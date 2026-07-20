package service

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent"
	entservice "github.com/whg517/fleet/internal/store/ent/service"
)

// ServiceStore abstracts the Ent client operations the service needs.
// This allows mocking in tests without a real database.
type ServiceStore interface {
	NewServiceCreate() *ent.ServiceCreate
	SaveService(ctx context.Context, s *ent.ServiceCreate) (*ent.Service, error)
	GetService(ctx context.Context, id string) (*ent.Service, error)
	UpdateServiceOne(id string) *ent.ServiceUpdateOne
	SaveServiceUpdate(ctx context.Context, upd *ent.ServiceUpdateOne) (*ent.Service, error)
	ListServices(ctx context.Context, limit, offset int, orgID, team, status, nameContains string) ([]*ent.Service, int, error)
}

// EntStore adapts *ent.Client to the ServiceStore interface.
type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) NewServiceCreate() *ent.ServiceCreate {
	return s.client.Service.Create()
}

func (s *EntStore) SaveService(ctx context.Context, sv *ent.ServiceCreate) (*ent.Service, error) {
	return sv.Save(ctx)
}

func (s *EntStore) GetService(ctx context.Context, id string) (*ent.Service, error) {
	return s.client.Service.Get(ctx, id)
}

func (s *EntStore) UpdateServiceOne(id string) *ent.ServiceUpdateOne {
	return s.client.Service.UpdateOneID(id)
}

func (s *EntStore) SaveServiceUpdate(ctx context.Context, upd *ent.ServiceUpdateOne) (*ent.Service, error) {
	return upd.Save(ctx)
}

func (s *EntStore) ListServices(ctx context.Context, limit, offset int, orgID, team, status, nameContains string) ([]*ent.Service, int, error) {
	q := s.client.Service.Query()
	if orgID != "" {
		q = q.Where(entservice.OrgIDEQ(orgID))
	}
	if team != "" {
		q = q.Where(entservice.TeamEQ(team))
	}
	if status != "" {
		q = q.Where(entservice.StatusEQ(entservice.Status(status)))
	}
	if nameContains != "" {
		q = q.Where(entservice.NameContainsFold(nameContains))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	services, err := q.Order(entservice.ByCreatedAt()).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return services, total, nil
}

// --- Service Implementation ---

// ServiceImpl implements the Service interface.
type ServiceImpl struct {
	store  ServiceStore
	logger *zap.Logger
}

// NewService creates a new service catalog service.
func NewService(store ServiceStore, logger *zap.Logger) *ServiceImpl {
	return &ServiceImpl{
		store:  store,
		logger: logger,
	}
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func validateCreateReq(req CreateServiceReq) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if len(req.Name) > 128 {
		return fmt.Errorf("%w: name must be at most 128 characters", ErrInvalidInput)
	}
	if len(req.Description) > 1024 {
		return fmt.Errorf("%w: description must be at most 1024 characters", ErrInvalidInput)
	}
	if len(req.Team) > 64 {
		return fmt.Errorf("%w: team must be at most 64 characters", ErrInvalidInput)
	}
	if req.GitRepo != "" && !isValidURL(req.GitRepo) {
		return fmt.Errorf("%w: git_repo must be a valid URL", ErrInvalidInput)
	}
	if len(req.Labels) > 50 {
		return fmt.Errorf("%w: labels must have at most 50 entries", ErrInvalidInput)
	}
	for k, v := range req.Labels {
		if len(k) > 64 || len(v) > 256 {
			return fmt.Errorf("%w: label key/value too long (key≤64, value≤256)", ErrInvalidInput)
		}
	}
	return nil
}

func isValidURL(s string) bool {
	u, err := neturl.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// Create registers a new service in the catalog.
func (s *ServiceImpl) Create(ctx context.Context, req CreateServiceReq) (*ServiceEntry, error) {
	if err := validateCreateReq(req); err != nil {
		return nil, err
	}

	serviceID := uuid.NewString()

	labels := req.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	builder := s.store.NewServiceCreate().
		SetID(serviceID).
		SetName(req.Name).
		SetLabels(labels)

	if req.OrgID != "" {
		builder.SetOrgID(req.OrgID)
	}
	if req.Team != "" {
		builder.SetTeam(req.Team)
	}
	if req.Description != "" {
		builder.SetDescription(req.Description)
	}
	if req.HarborProject != "" {
		builder.SetHarborProject(req.HarborProject)
	}
	if req.GitRepo != "" {
		builder.SetGitRepo(req.GitRepo)
	}

	sv, err := s.store.SaveService(ctx, builder)
	if err != nil {
		if ent.IsConstraintError(err) {
			errMsg := err.Error()
			// FK violation: org_id references a non-existent organization
			if strings.Contains(errMsg, "FOREIGN KEY constraint failed") {
				return nil, fmt.Errorf("%w: organization not found", ErrInvalidInput)
			}
			// Unique constraint: (org_id, name) already exists
			return nil, ErrServiceAlreadyExists
		}
		return nil, fmt.Errorf("create service: %w", err)
	}

	s.logger.Info("service created",
		zap.String("id", sv.ID),
		zap.String("name", sv.Name),
	)

	return toDomainService(sv), nil
}

// List returns a paginated, filtered list of services.
//
// Label filtering is performed in application memory (same pattern as cluster)
// because the ent store layer does not yet support JSON predicate queries on
// the labels field. When label filters are active, we fetch all matching
// services (without SQL pagination), filter in memory, then paginate.
func (s *ServiceImpl) List(ctx context.Context, filter ServiceFilter) (*ServiceListResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)

	var result []*ServiceEntry
	var total int

	// TODO: 当 ent 支持 JSON 谓词查询后，将 label 过滤下推到 SQL 层。
	// 当前方案在 label 过滤时全量拉取后内存过滤，设置上限防止滥用。
	const maxLabelScanLimit = 5000
	if len(filter.Labels) > 0 {
		services, _, err := s.store.ListServices(ctx, maxLabelScanLimit, 0, filter.OrgID, filter.Team, filter.Status, filter.Name)
		if err != nil {
			return nil, fmt.Errorf("query services: %w", err)
		}

		filtered := make([]*ServiceEntry, 0, len(services))
		for _, sv := range services {
			if matchLabels(sv.Labels, filter.Labels) {
				filtered = append(filtered, toDomainService(sv))
			}
		}

		total = len(filtered)
		offset := (page - 1) * pageSize
		if offset >= total {
			result = []*ServiceEntry{}
		} else if offset+pageSize >= total {
			result = filtered[offset:]
		} else {
			result = filtered[offset : offset+pageSize]
		}
	} else {
		offset := (page - 1) * pageSize
		services, t, err := s.store.ListServices(ctx, pageSize, offset, filter.OrgID, filter.Team, filter.Status, filter.Name)
		if err != nil {
			return nil, fmt.Errorf("query services: %w", err)
		}
		result = make([]*ServiceEntry, 0, len(services))
		for _, sv := range services {
			result = append(result, toDomainService(sv))
		}
		total = t
	}

	return &ServiceListResult{
		Services: result,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// Get returns a single service by ID.
func (s *ServiceImpl) Get(ctx context.Context, id string) (*ServiceEntry, error) {
	sv, err := s.store.GetService(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrServiceNotFound
		}
		return nil, fmt.Errorf("get service: %w", err)
	}
	return toDomainService(sv), nil
}

// Update modifies an existing service.
// Archived services cannot be updated.
func (s *ServiceImpl) Update(ctx context.Context, id string, req UpdateServiceReq) (*ServiceEntry, error) {
	// Check current status first
	existing, err := s.store.GetService(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrServiceNotFound
		}
		return nil, fmt.Errorf("get service for update: %w", err)
	}
	if existing.Status == entservice.StatusArchived {
		return nil, fmt.Errorf("%w: cannot update archived service", ErrInvalidInput)
	}

	upd := s.store.UpdateServiceOne(id)
	if req.Name != nil {
		upd.SetName(*req.Name)
	}
	if req.Team != nil {
		upd.SetTeam(*req.Team)
	}
	if req.Description != nil {
		upd.SetDescription(*req.Description)
	}
	if req.Labels != nil {
		upd.SetLabels(*req.Labels)
	}
	if req.HarborProject != nil {
		upd.SetHarborProject(*req.HarborProject)
	}
	if req.GitRepo != nil {
		upd.SetGitRepo(*req.GitRepo)
	}

	updated, err := s.store.SaveServiceUpdate(ctx, upd)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrServiceNotFound
		}
		if ent.IsConstraintError(err) {
			return nil, ErrServiceAlreadyExists
		}
		return nil, fmt.Errorf("update service: %w", err)
	}

	s.logger.Info("service updated", zap.String("id", id))
	return toDomainService(updated), nil
}

// Delete archives a service (soft delete — sets status to archived).
func (s *ServiceImpl) Delete(ctx context.Context, id string) error {
	upd := s.store.UpdateServiceOne(id).SetStatus(entservice.StatusArchived)
	_, err := s.store.SaveServiceUpdate(ctx, upd)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrServiceNotFound
		}
		return fmt.Errorf("archive service: %w", err)
	}

	s.logger.Info("service archived", zap.String("id", id))
	return nil
}

// --- Helpers ---

func matchLabels(serviceLabels, filterLabels map[string]string) bool {
	if len(filterLabels) == 0 {
		return true
	}
	for k, v := range filterLabels {
		if serviceLabels[k] != v {
			return false
		}
	}
	return true
}

func toDomainService(sv *ent.Service) *ServiceEntry {
	return &ServiceEntry{
		ID:            sv.ID,
		OrgID:         sv.OrgID,
		Name:          sv.Name,
		Team:          sv.Team,
		Description:   sv.Description,
		Labels:        sv.Labels,
		Status:        string(sv.Status),
		HarborProject: sv.HarborProject,
		GitRepo:       sv.GitRepo,
		CreatedAt:     sv.CreatedAt,
		UpdatedAt:     sv.UpdatedAt,
	}
}
