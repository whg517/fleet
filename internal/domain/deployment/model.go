package deployment

import (
	"errors"
	"time"
)

var (
	// ErrDeploymentNotFound is returned when a deployment does not exist.
	ErrDeploymentNotFound = errors.New("deployment not found")
	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
	// ErrDeploymentAlreadyExists is returned when an active deployment already exists for the same service+environment.
	ErrDeploymentAlreadyExists = errors.New("deployment already exists for this service and environment")
	// ErrArgoCDUnavailable is returned when the Argo CD API call fails.
	ErrArgoCDUnavailable = errors.New("argo cd unavailable")
	// ErrNoHealthyVersion is returned when rollback cannot find a previous healthy deployment.
	ErrNoHealthyVersion = errors.New("no previous healthy version to rollback to")
)

// DeploymentStatus defines the lifecycle state of a Deployment.
type DeploymentStatus string

const (
	StatusPending    DeploymentStatus = "pending"
	StatusValidating DeploymentStatus = "validating"
	StatusDeploying  DeploymentStatus = "deploying"
	StatusHealthy    DeploymentStatus = "healthy"
	StatusDegraded   DeploymentStatus = "degraded"
	StatusFailed     DeploymentStatus = "failed"
	StatusCancelled  DeploymentStatus = "cancelled"
)

// Deployment represents a deploy action in the system.
type Deployment struct {
	ID                string           `json:"id"`
	OrgID             string           `json:"org_id,omitempty"`
	ServiceID         string           `json:"service_id"`
	EnvironmentID     string           `json:"environment_id"`
	ClusterID         string           `json:"cluster_id"`
	TemplateVersionID string           `json:"template_version_id"`
	Version           string           `json:"version"`
	ValuesOverride    map[string]any   `json:"values_override,omitempty"`
	Status            DeploymentStatus `json:"status"`
	ArgoCDAppName     string           `json:"argocd_app_name,omitempty"`
	SyncStatus        string           `json:"sync_status,omitempty"`
	HealthStatus      string           `json:"health_status,omitempty"`
	CreatedBy         string           `json:"created_by,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	CompletedAt       *time.Time       `json:"completed_at,omitempty"`
}

// CreateDeploymentReq is the request payload for creating a new deployment.
type CreateDeploymentReq struct {
	OrgID             string         `json:"org_id,omitempty"`
	ServiceID         string         `json:"service_id"`
	EnvironmentID     string         `json:"environment_id"`
	TemplateVersionID string         `json:"template_version_id"`
	Version           string         `json:"version"`
	ValuesOverride    map[string]any `json:"values_override,omitempty"`
	CreatedBy         string         `json:"-"`
}

// DeploymentFilter is used for filtering and paginating deployments.
type DeploymentFilter struct {
	OrgID         string
	ServiceID     string
	EnvironmentID string
	Status        string
	Page          int
	PageSize      int
}

// DeploymentListResult holds a paginated list of deployments.
type DeploymentListResult struct {
	Deployments []*Deployment
	Total       int
	Page        int
	PageSize    int
}
