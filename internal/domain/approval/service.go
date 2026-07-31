package approval

import (
	"context"
	"time"

	"github.com/whg517/fleet/internal/store/ent"
)

// ApprovalStore abstracts Ent client operations for the Approval entity.
type ApprovalStore interface {
	NewApprovalCreate() *ent.ApprovalCreate
	SaveApproval(ctx context.Context, a *ent.ApprovalCreate) (*ent.Approval, error)
	GetApproval(ctx context.Context, id string) (*ent.Approval, error)
	GetApprovalByDeploymentID(ctx context.Context, deploymentID string) (*ent.Approval, error)
	ListApprovals(ctx context.Context, limit, offset int, orgID, serviceID, environmentID, status string) ([]*ent.Approval, int, error)
	UpdateApprovalOne(id string) *ent.ApprovalUpdateOne
	SaveApprovalUpdate(ctx context.Context, upd *ent.ApprovalUpdateOne) (*ent.Approval, error)
	ListExpiredApprovals(ctx context.Context, before time.Time) ([]*ent.Approval, error)
}

// LookupStore provides read-only access to related entities needed for validation.
type LookupStore interface {
	GetDeploymentByID(ctx context.Context, id string) (*ent.Deployment, error)
	GetServiceByID(ctx context.Context, id string) (*ent.Service, error)
	GetEnvironmentByID(ctx context.Context, id string) (*ent.Environment, error)
}

// DeploymentTrigger abstracts deployment lifecycle actions that the approval service
// can invoke after an approval decision (approve → trigger, reject/timeout → cancel).
type DeploymentTrigger interface {
	TriggerDeployment(ctx context.Context, deploymentID string) error
	CancelDeployment(ctx context.Context, deploymentID string) error
}

// Service defines the approval management operations.
type Service interface {
	// RequestApproval creates a pending approval for a deployment.
	RequestApproval(ctx context.Context, deploymentID string, req RequestApprovalReq) (*Approval, error)

	// Approve approves a pending approval and triggers the deployment.
	Approve(ctx context.Context, deploymentID string, req ApproveReq) (*Approval, error)

	// Reject rejects a pending approval and cancels the deployment.
	Reject(ctx context.Context, deploymentID string, req RejectReq) (*Approval, error)

	// Get returns the approval for a given deployment.
	Get(ctx context.Context, deploymentID string) (*Approval, error)

	// List returns a filtered, paginated list of approvals.
	List(ctx context.Context, filter ApprovalFilter) (*ApprovalListResult, error)

	// CheckTimeouts finds pending approvals past their timeout and marks them as timeout.
	CheckTimeouts(ctx context.Context) (int, error)
}
