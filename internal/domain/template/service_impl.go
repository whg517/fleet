package template

import (
	"context"
	"fmt"
	neturl "net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent"
	enttemplate "github.com/whg517/fleet/internal/store/ent/template"
	enttemplateversion "github.com/whg517/fleet/internal/store/ent/templateversion"
)

// TemplateStore abstracts the Ent client operations the service needs.
type TemplateStore interface {
	// Template operations
	NewTemplateCreate() *ent.TemplateCreate
	SaveTemplate(ctx context.Context, t *ent.TemplateCreate) (*ent.Template, error)
	GetTemplate(ctx context.Context, id string) (*ent.Template, error)
	UpdateTemplateOne(id string) *ent.TemplateUpdateOne
	SaveTemplateUpdate(ctx context.Context, upd *ent.TemplateUpdateOne) (*ent.Template, error)
	ListTemplates(ctx context.Context, limit, offset int, orgID, typ, source, status, nameContains string) ([]*ent.Template, int, error)
	// Template version operations
	NewVersionCreate() *ent.TemplateVersionCreate
	SaveVersion(ctx context.Context, v *ent.TemplateVersionCreate) (*ent.TemplateVersion, error)
	ListVersions(ctx context.Context, templateID string, limit, offset int) ([]*ent.TemplateVersion, int, error)
	GetVersionByTemplateAndVersion(ctx context.Context, templateID, version string) (*ent.TemplateVersion, error)
	UpdateVersionOne(id string) *ent.TemplateVersionUpdateOne
	SaveVersionUpdate(ctx context.Context, upd *ent.TemplateVersionUpdateOne) (*ent.TemplateVersion, error)
}

// EntStore adapts *ent.Client to the TemplateStore interface.
type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) NewTemplateCreate() *ent.TemplateCreate {
	return s.client.Template.Create()
}

func (s *EntStore) SaveTemplate(ctx context.Context, t *ent.TemplateCreate) (*ent.Template, error) {
	return t.Save(ctx)
}

func (s *EntStore) GetTemplate(ctx context.Context, id string) (*ent.Template, error) {
	return s.client.Template.Get(ctx, id)
}

func (s *EntStore) UpdateTemplateOne(id string) *ent.TemplateUpdateOne {
	return s.client.Template.UpdateOneID(id)
}

func (s *EntStore) SaveTemplateUpdate(ctx context.Context, upd *ent.TemplateUpdateOne) (*ent.Template, error) {
	return upd.Save(ctx)
}

func (s *EntStore) ListTemplates(ctx context.Context, limit, offset int, orgID, typ, source, status, nameContains string) ([]*ent.Template, int, error) {
	q := s.client.Template.Query()
	if orgID != "" {
		q = q.Where(enttemplate.OrgIDEQ(orgID))
	}
	if typ != "" {
		q = q.Where(enttemplate.TypeEQ(enttemplate.Type(typ)))
	}
	if source != "" {
		q = q.Where(enttemplate.SourceEQ(enttemplate.Source(source)))
	}
	if status != "" {
		q = q.Where(enttemplate.StatusEQ(enttemplate.Status(status)))
	}
	if nameContains != "" {
		q = q.Where(enttemplate.NameContainsFold(nameContains))
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
	templates, err := q.Order(enttemplate.ByCreatedAt()).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return templates, total, nil
}

func (s *EntStore) NewVersionCreate() *ent.TemplateVersionCreate {
	return s.client.TemplateVersion.Create()
}

func (s *EntStore) SaveVersion(ctx context.Context, v *ent.TemplateVersionCreate) (*ent.TemplateVersion, error) {
	return v.Save(ctx)
}

func (s *EntStore) ListVersions(ctx context.Context, templateID string, limit, offset int) ([]*ent.TemplateVersion, int, error) {
	q := s.client.TemplateVersion.Query().
		Where(enttemplateversion.TemplateIDEQ(templateID))

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
	versions, err := q.Order(enttemplateversion.ByCreatedAt()).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return versions, total, nil
}

func (s *EntStore) GetVersionByTemplateAndVersion(ctx context.Context, templateID, version string) (*ent.TemplateVersion, error) {
	return s.client.TemplateVersion.Query().
		Where(
			enttemplateversion.TemplateIDEQ(templateID),
			enttemplateversion.VersionEQ(version),
		).
		Only(ctx)
}

func (s *EntStore) UpdateVersionOne(id string) *ent.TemplateVersionUpdateOne {
	return s.client.TemplateVersion.UpdateOneID(id)
}

func (s *EntStore) SaveVersionUpdate(ctx context.Context, upd *ent.TemplateVersionUpdateOne) (*ent.TemplateVersion, error) {
	return upd.Save(ctx)
}

// --- Service Implementation ---

// ServiceImpl implements the Service interface.
type ServiceImpl struct {
	store  TemplateStore
	logger *zap.Logger
}

// NewService creates a new template management service.
func NewService(store TemplateStore, logger *zap.Logger) *ServiceImpl {
	return &ServiceImpl{
		store:  store,
		logger: logger,
	}
}

// semverRegex matches semantic version strings (e.g., 1.2.3, 1.2.3-beta.1).
var semverRegex = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

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

func isValidURL(s string) bool {
	u, err := neturl.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func isValidSemver(v string) bool {
	return semverRegex.MatchString(v)
}

func validateCreateReq(req CreateTemplateReq) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if len(req.Name) > 128 {
		return fmt.Errorf("%w: name must be at most 128 characters", ErrInvalidInput)
	}
	if len(req.Description) > 1024 {
		return fmt.Errorf("%w: description must be at most 1024 characters", ErrInvalidInput)
	}
	if req.Repo != "" && !isValidURL(req.Repo) {
		return fmt.Errorf("%w: repo must be a valid URL", ErrInvalidInput)
	}
	// Validate type
	switch req.Type {
	case TypeBuild, TypeDeployK8s, TypeDeployVM:
	default:
		return fmt.Errorf("%w: invalid type %q (must be build, deploy_k8s, or deploy_vm)", ErrInvalidInput, req.Type)
	}
	// Validate source
	switch req.Source {
	case SourcePlatform, SourceExternalOCI:
	default:
		return fmt.Errorf("%w: invalid source %q (must be platform or external_oci)", ErrInvalidInput, req.Source)
	}
	return nil
}

func validatePublishVersionReq(req PublishVersionReq) error {
	if !isValidSemver(req.Version) {
		return fmt.Errorf("%w: version must be valid semver (e.g., 1.2.3)", ErrInvalidInput)
	}
	if len(req.Changelog) > 4096 {
		return fmt.Errorf("%w: changelog must be at most 4096 characters", ErrInvalidInput)
	}
	return nil
}

// Create registers a new template.
func (s *ServiceImpl) Create(ctx context.Context, req CreateTemplateReq) (*Template, error) {
	if err := validateCreateReq(req); err != nil {
		return nil, err
	}

	templateID := uuid.NewString()

	builder := s.store.NewTemplateCreate().
		SetID(templateID).
		SetName(req.Name).
		SetType(enttemplate.Type(req.Type)).
		SetSource(enttemplate.Source(req.Source))

	if req.OrgID != "" {
		builder.SetOrgID(req.OrgID)
	}
	if req.RegistryID != "" {
		builder.SetRegistryID(req.RegistryID)
	}
	if req.Repo != "" {
		builder.SetRepo(req.Repo)
	}
	if req.Description != "" {
		builder.SetDescription(req.Description)
	}

	t, err := s.store.SaveTemplate(ctx, builder)
	if err != nil {
		if ent.IsConstraintError(err) {
			errMsg := err.Error()
			if strings.Contains(errMsg, "FOREIGN KEY constraint failed") {
				return nil, fmt.Errorf("%w: organization not found", ErrInvalidInput)
			}
			return nil, ErrTemplateAlreadyExists
		}
		return nil, fmt.Errorf("create template: %w", err)
	}

	s.logger.Info("template created",
		zap.String("id", t.ID),
		zap.String("name", t.Name),
		zap.String("type", string(t.Type)),
	)

	return toDomainTemplate(t), nil
}

// List returns a paginated, filtered list of templates.
func (s *ServiceImpl) List(ctx context.Context, filter TemplateFilter) (*TemplateListResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	offset := (page - 1) * pageSize

	templates, total, err := s.store.ListTemplates(ctx, pageSize, offset, filter.OrgID, filter.Type, filter.Source, filter.Status, filter.Name)
	if err != nil {
		return nil, fmt.Errorf("query templates: %w", err)
	}

	result := make([]*Template, 0, len(templates))
	for _, t := range templates {
		result = append(result, toDomainTemplate(t))
	}

	return &TemplateListResult{
		Templates: result,
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
	}, nil
}

// Get returns a single template by ID.
func (s *ServiceImpl) Get(ctx context.Context, id string) (*Template, error) {
	t, err := s.store.GetTemplate(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTemplateNotFound
		}
		return nil, fmt.Errorf("get template: %w", err)
	}
	return toDomainTemplate(t), nil
}

// GetWithVersions returns a template with all its active versions loaded.
func (s *ServiceImpl) GetWithVersions(ctx context.Context, id string) (*Template, []*TemplateVersion, error) {
	t, err := s.store.GetTemplate(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, ErrTemplateNotFound
		}
		return nil, nil, fmt.Errorf("get template: %w", err)
	}

	versions, _, err := s.store.ListVersions(ctx, id, 100, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("list template versions: %w", err)
	}

	domainVersions := make([]*TemplateVersion, 0, len(versions))
	for _, v := range versions {
		if v.Status == enttemplateversion.StatusActive {
			domainVersions = append(domainVersions, toDomainVersion(v))
		}
	}

	return toDomainTemplate(t), domainVersions, nil
}

// Update modifies an existing template.
// Archived templates cannot be updated.
func (s *ServiceImpl) Update(ctx context.Context, id string, req UpdateTemplateReq) (*Template, error) {
	// Check current status first
	existing, err := s.store.GetTemplate(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTemplateNotFound
		}
		return nil, fmt.Errorf("get template for update: %w", err)
	}
	if existing.Status == enttemplate.StatusArchived {
		return nil, fmt.Errorf("%w: cannot update archived template", ErrInvalidInput)
	}

	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", ErrInvalidInput)
		}
		if len(*req.Name) > 128 {
			return nil, fmt.Errorf("%w: name must be at most 128 characters", ErrInvalidInput)
		}
	}
	if req.Description != nil && len(*req.Description) > 1024 {
		return nil, fmt.Errorf("%w: description must be at most 1024 characters", ErrInvalidInput)
	}
	if req.Repo != nil && *req.Repo != "" && !isValidURL(*req.Repo) {
		return nil, fmt.Errorf("%w: repo must be a valid URL", ErrInvalidInput)
	}

	upd := s.store.UpdateTemplateOne(id)
	if req.Name != nil {
		upd.SetName(*req.Name)
	}
	if req.Description != nil {
		upd.SetDescription(*req.Description)
	}
	if req.Repo != nil {
		upd.SetRepo(*req.Repo)
	}
	if req.RegistryID != nil {
		upd.SetRegistryID(*req.RegistryID)
	}

	updated, err := s.store.SaveTemplateUpdate(ctx, upd)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTemplateNotFound
		}
		if ent.IsConstraintError(err) {
			return nil, ErrTemplateAlreadyExists
		}
		return nil, fmt.Errorf("update template: %w", err)
	}

	s.logger.Info("template updated", zap.String("id", id))
	return toDomainTemplate(updated), nil
}

// Delete archives a template (soft delete — sets status to archived).
func (s *ServiceImpl) Delete(ctx context.Context, id string) error {
	_, err := s.store.GetTemplate(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrTemplateNotFound
		}
		return fmt.Errorf("get template for archive: %w", err)
	}

	upd := s.store.UpdateTemplateOne(id).SetStatus(enttemplate.StatusArchived)
	if _, err := s.store.SaveTemplateUpdate(ctx, upd); err != nil {
		if ent.IsNotFound(err) {
			return ErrTemplateNotFound
		}
		return fmt.Errorf("archive template: %w", err)
	}

	s.logger.Info("template archived", zap.String("id", id))
	return nil
}

// PublishVersion creates a new immutable version for a template.
func (s *ServiceImpl) PublishVersion(ctx context.Context, templateID string, req PublishVersionReq) (*TemplateVersion, error) {
	if err := validatePublishVersionReq(req); err != nil {
		return nil, err
	}

	// Verify template exists and is active
	t, err := s.store.GetTemplate(ctx, templateID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTemplateNotFound
		}
		return nil, fmt.Errorf("get template for publish: %w", err)
	}
	if t.Status == enttemplate.StatusArchived {
		return nil, fmt.Errorf("%w: cannot publish version for archived template", ErrInvalidInput)
	}

	versionID := uuid.NewString()

	builder := s.store.NewVersionCreate().
		SetID(versionID).
		SetTemplateID(templateID).
		SetVersion(req.Version)

	if req.Digest != "" {
		builder.SetDigest(req.Digest)
	}
	if req.ValuesSchema != nil {
		builder.SetValuesSchema(req.ValuesSchema)
	}
	if req.Changelog != "" {
		builder.SetChangelog(req.Changelog)
	}

	v, err := s.store.SaveVersion(ctx, builder)
	if err != nil {
		if ent.IsConstraintError(err) {
			// Unique constraint: (template_id, version) already exists
			return nil, ErrVersionAlreadyExists
		}
		return nil, fmt.Errorf("publish version: %w", err)
	}

	s.logger.Info("template version published",
		zap.String("template_id", templateID),
		zap.String("version", req.Version),
	)

	return toDomainVersion(v), nil
}

// ListVersions returns a paginated list of versions for a template.
func (s *ServiceImpl) ListVersions(ctx context.Context, templateID string, page, pageSize int) (*VersionListResult, error) {
	page, pageSize = normalizePage(page, pageSize)
	offset := (page - 1) * pageSize

	versions, total, err := s.store.ListVersions(ctx, templateID, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("list template versions: %w", err)
	}

	result := make([]*TemplateVersion, 0, len(versions))
	for _, v := range versions {
		result = append(result, toDomainVersion(v))
	}

	return &VersionListResult{
		Versions: result,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// ArchiveVersion archives a specific version of a template.
func (s *ServiceImpl) ArchiveVersion(ctx context.Context, templateID, version string) error {
	v, err := s.store.GetVersionByTemplateAndVersion(ctx, templateID, version)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrVersionNotFound
		}
		return fmt.Errorf("get template version: %w", err)
	}

	upd := s.store.UpdateVersionOne(v.ID).SetStatus(enttemplateversion.StatusArchived)
	if _, err := s.store.SaveVersionUpdate(ctx, upd); err != nil {
		if ent.IsNotFound(err) {
			return ErrVersionNotFound
		}
		return fmt.Errorf("archive template version: %w", err)
	}

	s.logger.Info("template version archived",
		zap.String("template_id", templateID),
		zap.String("version", version),
	)

	return nil
}

// --- Helpers ---

func toDomainTemplate(t *ent.Template) *Template {
	return &Template{
		ID:          t.ID,
		OrgID:       t.OrgID,
		Name:        t.Name,
		Type:        TemplateType(string(t.Type)),
		Source:      TemplateSource(string(t.Source)),
		RegistryID:  t.RegistryID,
		Repo:        t.Repo,
		Description: t.Description,
		Status:      TemplateStatus(string(t.Status)),
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func toDomainVersion(v *ent.TemplateVersion) *TemplateVersion {
	return &TemplateVersion{
		ID:           v.ID,
		TemplateID:   v.TemplateID,
		Version:      v.Version,
		Digest:       v.Digest,
		ValuesSchema: v.ValuesSchema,
		Changelog:    v.Changelog,
		Status:       TemplateStatus(string(v.Status)),
		CreatedAt:    v.CreatedAt,
	}
}
