package deployment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent"
	entdeployment "github.com/whg517/fleet/internal/store/ent/deployment"
	entservice "github.com/whg517/fleet/internal/store/ent/service"
	enttemplateversion "github.com/whg517/fleet/internal/store/ent/templateversion"
)

// EntStore adapts *ent.Client to the DeploymentStore interface.
type EntStore struct {
	client *ent.Client
}

// NewEntStore creates a new EntStore for the Deployment entity.
func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) NewDeploymentCreate() *ent.DeploymentCreate {
	return s.client.Deployment.Create()
}

func (s *EntStore) SaveDeployment(ctx context.Context, d *ent.DeploymentCreate) (*ent.Deployment, error) {
	return d.Save(ctx)
}

func (s *EntStore) GetDeployment(ctx context.Context, id string) (*ent.Deployment, error) {
	return s.client.Deployment.Get(ctx, id)
}

func (s *EntStore) UpdateDeploymentOne(id string) *ent.DeploymentUpdateOne {
	return s.client.Deployment.UpdateOneID(id)
}

func (s *EntStore) SaveDeploymentUpdate(ctx context.Context, upd *ent.DeploymentUpdateOne) (*ent.Deployment, error) {
	return upd.Save(ctx)
}

func (s *EntStore) ListDeployments(ctx context.Context, limit, offset int, orgID, serviceID, environmentID, status string) ([]*ent.Deployment, int, error) {
	q := s.client.Deployment.Query()
	if orgID != "" {
		q = q.Where(entdeployment.OrgIDEQ(orgID))
	}
	if serviceID != "" {
		q = q.Where(entdeployment.ServiceIDEQ(serviceID))
	}
	if environmentID != "" {
		q = q.Where(entdeployment.EnvironmentIDEQ(environmentID))
	}
	if status != "" {
		q = q.Where(entdeployment.StatusEQ(entdeployment.Status(status)))
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
	deployments, err := q.Order(entdeployment.ByCreatedAt()).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return deployments, total, nil
}

// --- LookupStore Implementation ---

// LookupEntStore adapts *ent.Client to the LookupStore interface.
type LookupEntStore struct {
	client *ent.Client
}

// NewLookupEntStore creates a new LookupEntStore.
func NewLookupEntStore(client *ent.Client) *LookupEntStore {
	return &LookupEntStore{client: client}
}

func (s *LookupEntStore) GetServiceByID(ctx context.Context, id string) (*ent.Service, error) {
	return s.client.Service.Get(ctx, id)
}

func (s *LookupEntStore) GetEnvironmentByID(ctx context.Context, id string) (*ent.Environment, error) {
	return s.client.Environment.Get(ctx, id)
}

func (s *LookupEntStore) GetTemplateVersionByID(ctx context.Context, id string) (*ent.TemplateVersion, error) {
	return s.client.TemplateVersion.Get(ctx, id)
}

// --- Service Implementation ---

// ServiceImpl implements the Service interface.
type ServiceImpl struct {
	store      DeploymentStore
	lookup     LookupStore
	argocd     ArgoCDClient
	logger     *zap.Logger
}

// NewService creates a new deployment service.
func NewService(store DeploymentStore, lookup LookupStore, argocd ArgoCDClient, logger *zap.Logger) *ServiceImpl {
	return &ServiceImpl{
		store:  store,
		lookup: lookup,
		argocd: argocd,
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

func validateCreateReq(req CreateDeploymentReq) error {
	if strings.TrimSpace(req.ServiceID) == "" {
		return fmt.Errorf("%w: service_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.EnvironmentID) == "" {
		return fmt.Errorf("%w: environment_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.TemplateVersionID) == "" {
		return fmt.Errorf("%w: template_version_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.Version) == "" {
		return fmt.Errorf("%w: version (image tag) is required", ErrInvalidInput)
	}
	if len(req.Version) > 256 {
		return fmt.Errorf("%w: version must be at most 256 characters", ErrInvalidInput)
	}
	return nil
}

// generateArgocdAppName generates a deterministic Argo CD Application name.
// Format: fleet-{service_id_short}-{env_id_short}
func generateArgocdAppName(serviceID, environmentID string) string {
	// Use first 8 chars of each ID for a concise but unique name
	svcShort := serviceID
	if len(svcShort) > 8 {
		svcShort = svcShort[:8]
	}
	envShort := environmentID
	if len(envShort) > 8 {
		envShort = envShort[:8]
	}
	return fmt.Sprintf("fleet-%s-%s", svcShort, envShort)
}

// Create creates a new deployment.
func (s *ServiceImpl) Create(ctx context.Context, req CreateDeploymentReq) (*Deployment, error) {
	if err := validateCreateReq(req); err != nil {
		return nil, err
	}

	// Validate service exists and is active
	svc, err := s.lookup.GetServiceByID(ctx, req.ServiceID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: service not found", ErrInvalidInput)
		}
		return nil, fmt.Errorf("lookup service: %w", err)
	}
	if svc.Status == entservice.StatusArchived {
		return nil, fmt.Errorf("%w: service is archived", ErrInvalidInput)
	}

	// Validate environment exists
	env, err := s.lookup.GetEnvironmentByID(ctx, req.EnvironmentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: environment not found", ErrInvalidInput)
		}
		return nil, fmt.Errorf("lookup environment: %w", err)
	}

	// Validate template version exists and is active
	tv, err := s.lookup.GetTemplateVersionByID(ctx, req.TemplateVersionID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: template version not found", ErrInvalidInput)
		}
		return nil, fmt.Errorf("lookup template version: %w", err)
	}
	if tv.Status == enttemplateversion.StatusArchived {
		return nil, fmt.Errorf("%w: template version is archived", ErrInvalidInput)
	}

	// Generate Argo CD Application name
	argocdAppName := generateArgocdAppName(req.ServiceID, req.EnvironmentID)

	// Create Argo CD Application
	// In MVP, we use the template version's repo/chart info.
	// The actual repo URL would come from the Template/Registry, but for now
	// we use a simplified approach.
	err = s.argocd.CreateApplication(ctx, ArgoCDAppReq{
		Name:      argocdAppName,
		Namespace: env.NamespacePattern,
		RepoURL:   "", // Would be resolved from Template/Registry in full impl
		Chart:     "", // Would be resolved from Template
		TargetRev: tv.Version,
		Values:    req.ValuesOverride,
	})
	if err != nil {
		s.logger.Error("failed to create Argo CD application",
			zap.String("name", argocdAppName),
			zap.Error(err),
		)
		return nil, fmt.Errorf("%w: %v", ErrArgoCDUnavailable, err)
	}

	// Save Deployment record
	deploymentID := uuid.NewString()

	builder := s.store.NewDeploymentCreate().
		SetID(deploymentID).
		SetServiceID(req.ServiceID).
		SetEnvironmentID(req.EnvironmentID).
		SetClusterID(env.ClusterID).
		SetTemplateVersionID(req.TemplateVersionID).
		SetVersion(req.Version).
		SetStatus(entdeployment.StatusDeploying).
		SetArgocdAppName(argocdAppName)

	if req.OrgID != "" {
		builder.SetOrgID(req.OrgID)
	}
	if req.ValuesOverride != nil {
		builder.SetValuesOverride(req.ValuesOverride)
	}
	if req.CreatedBy != "" {
		builder.SetCreatedBy(req.CreatedBy)
	}

	d, err := s.store.SaveDeployment(ctx, builder)
	if err != nil {
		// Best-effort cleanup of Argo CD app if DB save fails
		_ = s.argocd.DeleteApplication(ctx, argocdAppName)
		if ent.IsConstraintError(err) {
			errMsg := err.Error()
			if strings.Contains(errMsg, "FOREIGN KEY constraint failed") {
				return nil, fmt.Errorf("%w: referenced entity not found", ErrInvalidInput)
			}
			return nil, ErrDeploymentAlreadyExists
		}
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	s.logger.Info("deployment created",
		zap.String("id", d.ID),
		zap.String("argocd_app", argocdAppName),
		zap.String("service_id", req.ServiceID),
		zap.String("version", req.Version),
	)

	return toDomainDeployment(d), nil
}

// Get returns a deployment by ID.
func (s *ServiceImpl) Get(ctx context.Context, id string) (*Deployment, error) {
	d, err := s.store.GetDeployment(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	return toDomainDeployment(d), nil
}

// List returns a filtered, paginated list of deployments.
func (s *ServiceImpl) List(ctx context.Context, filter DeploymentFilter) (*DeploymentListResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	offset := (page - 1) * pageSize

	deployments, total, err := s.store.ListDeployments(ctx, pageSize, offset, filter.OrgID, filter.ServiceID, filter.EnvironmentID, filter.Status)
	if err != nil {
		return nil, fmt.Errorf("query deployments: %w", err)
	}

	result := make([]*Deployment, 0, len(deployments))
	for _, d := range deployments {
		result = append(result, toDomainDeployment(d))
	}

	return &DeploymentListResult{
		Deployments: result,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

// GetStatus fetches the latest sync/health status from Argo CD and updates the record.
func (s *ServiceImpl) GetStatus(ctx context.Context, id string) (*Deployment, error) {
	d, err := s.store.GetDeployment(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("get deployment: %w", err)
	}

	if d.ArgocdAppName == "" {
		return nil, fmt.Errorf("%w: deployment has no Argo CD application", ErrInvalidInput)
	}

	status, err := s.argocd.GetApplication(ctx, d.ArgocdAppName)
	if err != nil {
		s.logger.Error("failed to get Argo CD application status",
			zap.String("name", d.ArgocdAppName),
			zap.Error(err),
		)
		return nil, fmt.Errorf("%w: %v", ErrArgoCDUnavailable, err)
	}

	// Map Argo CD status to deployment status
	newStatus := mapArgocdStatus(status.SyncStatus, status.HealthStatus)

	upd := s.store.UpdateDeploymentOne(id).
		SetSyncStatus(status.SyncStatus).
		SetHealthStatus(status.HealthStatus).
		SetStatus(entdeployment.Status(newStatus))

	// Set completed_at if the deployment reached a terminal state
	if isTerminalStatus(newStatus) && d.CompletedAt == nil {
		now := time.Now()
		upd.SetCompletedAt(now)
	}

	updated, err := s.store.SaveDeploymentUpdate(ctx, upd)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("update deployment status: %w", err)
	}

	return toDomainDeployment(updated), nil
}

// Rollback rolls back to the previous healthy deployment for the same service+environment.
func (s *ServiceImpl) Rollback(ctx context.Context, id string) (*Deployment, error) {
	// Get the current deployment
	current, err := s.store.GetDeployment(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("get deployment for rollback: %w", err)
	}

	if current.ArgocdAppName == "" {
		return nil, fmt.Errorf("%w: deployment has no Argo CD application", ErrInvalidInput)
	}

	// Find the previous healthy deployment for the same service+environment
	deployments, _, err := s.store.ListDeployments(ctx, 50, 0, "", current.ServiceID, current.EnvironmentID, "")
	if err != nil {
		return nil, fmt.Errorf("list deployments for rollback: %w", err)
	}

	var previousHealthy *ent.Deployment
	for _, d := range deployments {
		// Skip the current deployment
		if d.ID == id {
			continue
		}
		// Find the most recent healthy one with a different version
		if d.Status == entdeployment.StatusHealthy && d.Version != current.Version {
			previousHealthy = d
			break
		}
	}

	if previousHealthy == nil {
		return nil, ErrNoHealthyVersion
	}

	// Perform rollback via Argo CD
	err = s.argocd.RollbackApplication(ctx, current.ArgocdAppName, previousHealthy.Version)
	if err != nil {
		s.logger.Error("failed to rollback Argo CD application",
			zap.String("name", current.ArgocdAppName),
			zap.String("target_version", previousHealthy.Version),
			zap.Error(err),
		)
		return nil, fmt.Errorf("%w: %v", ErrArgoCDUnavailable, err)
	}

	// Update current deployment status to deploying
	upd := s.store.UpdateDeploymentOne(id).
		SetStatus(entdeployment.StatusDeploying).
		SetSyncStatus("").
		SetHealthStatus("")

	updated, err := s.store.SaveDeploymentUpdate(ctx, upd)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("update deployment after rollback: %w", err)
	}

	s.logger.Info("deployment rollback initiated",
		zap.String("id", id),
		zap.String("rolled_back_to", previousHealthy.Version),
	)

	return toDomainDeployment(updated), nil
}

// --- Helpers ---

// mapArgocdStatus maps Argo CD sync/health statuses to a deployment status.
func mapArgocdStatus(syncStatus, healthStatus string) string {
	switch {
	case syncStatus == "Failed" || syncStatus == "Error":
		return string(StatusFailed)
	case healthStatus == "Healthy" && syncStatus == "Synced":
		return string(StatusHealthy)
	case healthStatus == "Healthy":
		return string(StatusHealthy)
	case healthStatus == "Degraded":
		return string(StatusDegraded)
	case syncStatus == "OutOfSync":
		return string(StatusDeploying)
	default:
		return string(StatusDeploying)
	}
}

// isTerminalStatus returns true if the deployment has reached a final state.
func isTerminalStatus(status string) bool {
	switch status {
	case string(StatusHealthy), string(StatusFailed), string(StatusDegraded), string(StatusCancelled):
		return true
	default:
		return false
	}
}

func toDomainDeployment(d *ent.Deployment) *Deployment {
	return &Deployment{
		ID:                d.ID,
		OrgID:             d.OrgID,
		ServiceID:         d.ServiceID,
		EnvironmentID:     d.EnvironmentID,
		ClusterID:         d.ClusterID,
		TemplateVersionID: d.TemplateVersionID,
		Version:           d.Version,
		ValuesOverride:    d.ValuesOverride,
		Status:            DeploymentStatus(string(d.Status)),
		ArgocdAppName:     d.ArgocdAppName,
		SyncStatus:        d.SyncStatus,
		HealthStatus:      d.HealthStatus,
		CreatedBy:         d.CreatedBy,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
		CompletedAt:       d.CompletedAt,
	}
}
