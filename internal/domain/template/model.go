package template

import (
	"errors"
	"time"
)

var (
	// ErrTemplateNotFound is returned when a template does not exist.
	ErrTemplateNotFound = errors.New("template not found")
	// ErrTemplateAlreadyExists is returned when a template name already exists within the org.
	ErrTemplateAlreadyExists = errors.New("template already exists")
	// ErrVersionNotFound is returned when a template version does not exist.
	ErrVersionNotFound = errors.New("template version not found")
	// ErrVersionAlreadyExists is returned when a version (semver) already exists for the template.
	ErrVersionAlreadyExists = errors.New("template version already exists")
	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
)

// TemplateType defines the kind of template.
type TemplateType string

const (
	TypeBuild     TemplateType = "build"
	TypeDeployK8s TemplateType = "deploy_k8s"
	TypeDeployVM  TemplateType = "deploy_vm"
)

// TemplateSource defines where the template originates from.
type TemplateSource string

const (
	SourcePlatform    TemplateSource = "platform"
	SourceExternalOCI TemplateSource = "external_oci"
)

// TemplateStatus defines the lifecycle state of a template.
type TemplateStatus string

const (
	StatusActive   TemplateStatus = "active"
	StatusArchived TemplateStatus = "archived"
)

// Template represents a deployment or build template (Helm Chart, Ansible Role, etc.).
type Template struct {
	ID          string         `json:"id"`
	OrgID       string         `json:"org_id,omitempty"`
	Name        string         `json:"name"`
	Type        TemplateType   `json:"type"`
	Source      TemplateSource `json:"source"`
	RegistryID  string         `json:"registry_id,omitempty"`
	Repo        string         `json:"repo,omitempty"`
	Description string         `json:"description,omitempty"`
	Status      TemplateStatus `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// TemplateVersion represents an immutable, published version of a template.
type TemplateVersion struct {
	ID           string         `json:"id"`
	TemplateID   string         `json:"template_id"`
	Version      string         `json:"version"` // semver
	Digest       string         `json:"digest,omitempty"`
	ValuesSchema map[string]any `json:"values_schema,omitempty"`
	Changelog    string         `json:"changelog,omitempty"`
	Status       TemplateStatus `json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
}

// CreateTemplateReq is the request payload for creating a new template.
type CreateTemplateReq struct {
	OrgID       string         `json:"org_id,omitempty"`
	Name        string         `json:"name"`
	Type        TemplateType   `json:"type"`
	Source      TemplateSource `json:"source"`
	RegistryID  string         `json:"registry_id,omitempty"`
	Repo        string         `json:"repo,omitempty"`
	Description string         `json:"description,omitempty"`
}

// UpdateTemplateReq is the request payload for updating a template.
// All fields are optional (pointer) — only provided fields are updated.
type UpdateTemplateReq struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Repo        *string `json:"repo,omitempty"`
	RegistryID  *string `json:"registry_id,omitempty"`
}

// PublishVersionReq is the request payload for publishing a new template version.
type PublishVersionReq struct {
	Version      string         `json:"version"`
	Digest       string         `json:"digest,omitempty"`
	ValuesSchema map[string]any `json:"values_schema,omitempty"`
	Changelog    string         `json:"changelog,omitempty"`
}

// TemplateFilter is used for filtering and paginating the template list.
type TemplateFilter struct {
	OrgID    string
	Type     string
	Source   string
	Status   string
	Name     string // fuzzy match (LIKE %name%)
	Page     int
	PageSize int
}

// TemplateListResult holds a paginated list of templates.
type TemplateListResult struct {
	Templates []*Template
	Total     int
	Page      int
	PageSize  int
}

// VersionListResult holds a paginated list of template versions.
type VersionListResult struct {
	Versions []*TemplateVersion
	Total    int
	Page     int
	PageSize int
}
