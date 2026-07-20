package template

import "context"

// Service defines the template management operations.
type Service interface {
	// Create registers a new template.
	Create(ctx context.Context, req CreateTemplateReq) (*Template, error)

	// List returns a paginated, filtered list of templates.
	List(ctx context.Context, filter TemplateFilter) (*TemplateListResult, error)

	// Get returns a single template by ID, optionally with versions loaded.
	Get(ctx context.Context, id string) (*Template, error)

	// GetWithVersions returns a template with all its active versions loaded.
	GetWithVersions(ctx context.Context, id string) (*Template, []*TemplateVersion, error)

	// Update modifies an existing template.
	// Archived templates cannot be updated.
	Update(ctx context.Context, id string, req UpdateTemplateReq) (*Template, error)

	// Delete archives a template (soft delete — sets status to archived).
	Delete(ctx context.Context, id string) error

	// PublishVersion creates a new immutable version for a template.
	PublishVersion(ctx context.Context, templateID string, req PublishVersionReq) (*TemplateVersion, error)

	// ListVersions returns a paginated list of versions for a template.
	ListVersions(ctx context.Context, templateID string, page, pageSize int) (*VersionListResult, error)

	// ArchiveVersion archives a specific version of a template.
	ArchiveVersion(ctx context.Context, templateID, version string) error
}
