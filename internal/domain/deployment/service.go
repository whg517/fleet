package deployment

import (
	"context"

	"github.com/whg517/fleet/internal/store/ent"
)

// ArgoCDAppReq contains the parameters for creating an Argo CD Application.
type ArgoCDAppReq struct {
	Name      string
	Namespace string
	RepoURL   string
	Chart     string
	TargetRev string
	Values    map[string]any
}

// ArgoCDAppStatus contains the sync and health status from Argo CD.
type ArgoCDAppStatus struct {
	SyncStatus   string
	HealthStatus string
}

// ArgoCDClient abstracts Argo CD API operations.
// Implementations may call the real Argo CD REST API or be mocked in tests.
type ArgoCDClient interface {
	CreateApplication(ctx context.Context, req ArgoCDAppReq) error
	GetApplication(ctx context.Context, name string) (*ArgoCDAppStatus, error)
	SyncApplication(ctx context.Context, name string) error
	RollbackApplication(ctx context.Context, name string, revision string) error
	DeleteApplication(ctx context.Context, name string) error
	UpdateApplication(ctx context.Context, name string, values map[string]any) error
}

// DeploymentStore abstracts the Ent client operations the service needs.
// This follows the same pattern as ServiceStore and TemplateStore.
type DeploymentStore interface {
	NewDeploymentCreate() *ent.DeploymentCreate
	SaveDeployment(ctx context.Context, d *ent.DeploymentCreate) (*ent.Deployment, error)
	GetDeployment(ctx context.Context, id string) (*ent.Deployment, error)
	UpdateDeploymentOne(id string) *ent.DeploymentUpdateOne
	SaveDeploymentUpdate(ctx context.Context, upd *ent.DeploymentUpdateOne) (*ent.Deployment, error)
	ListDeployments(ctx context.Context, limit, offset int, orgID, serviceID, environmentID, status string) ([]*ent.Deployment, int, error)
}

// LookupStore provides read-only access to related entities needed for validation.
type LookupStore interface {
	// GetServiceByID returns the service entity if it exists.
	GetServiceByID(ctx context.Context, id string) (*ent.Service, error)
	// GetEnvironmentByID returns the environment entity if it exists.
	GetEnvironmentByID(ctx context.Context, id string) (*ent.Environment, error)
	// GetTemplateVersionByID returns the template version entity if it exists.
	GetTemplateVersionByID(ctx context.Context, id string) (*ent.TemplateVersion, error)
	// GetTemplateByID returns the template entity if it exists.
	GetTemplateByID(ctx context.Context, id string) (*ent.Template, error)
}

// Service defines the deployment management operations.
type Service interface {
	// Create creates a new deployment by creating an Argo CD Application and recording the Deployment.
	// If the target environment requires approval, the deployment is saved as pending and no
	// Argo CD Application is created until the approval is approved.
	Create(ctx context.Context, req CreateDeploymentReq) (*Deployment, error)

	// Get returns a deployment by ID.
	Get(ctx context.Context, id string) (*Deployment, error)

	// List returns a filtered, paginated list of deployments.
	List(ctx context.Context, filter DeploymentFilter) (*DeploymentListResult, error)

	// GetStatus fetches the latest sync/health status from Argo CD and updates the record.
	GetStatus(ctx context.Context, id string) (*Deployment, error)

	// Rollback rolls back to the previous healthy deployment version.
	Rollback(ctx context.Context, id string) (*Deployment, error)

	// TriggerDeployment creates the Argo CD Application for a deployment that was previously
	// created in pending state (e.g. after approval). It transitions the deployment to deploying.
	TriggerDeployment(ctx context.Context, id string) error
}
