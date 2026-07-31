package approval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"entgo.io/ent/dialect/sql"
	"github.com/whg517/fleet/internal/store/ent"
	entapproval "github.com/whg517/fleet/internal/store/ent/approval"
	entdeployment "github.com/whg517/fleet/internal/store/ent/deployment"
)

// Approval timeout duration.
const approvalTimeout = 24 * time.Hour

// EntStore adapts *ent.Client to the ApprovalStore interface.
type EntStore struct {
	client *ent.Client
}

// NewEntStore creates a new EntStore for the Approval entity.
func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) NewApprovalCreate() *ent.ApprovalCreate {
	return s.client.Approval.Create()
}

func (s *EntStore) SaveApproval(ctx context.Context, a *ent.ApprovalCreate) (*ent.Approval, error) {
	return a.Save(ctx)
}

func (s *EntStore) GetApproval(ctx context.Context, id string) (*ent.Approval, error) {
	return s.client.Approval.Get(ctx, id)
}

func (s *EntStore) GetApprovalByDeploymentID(ctx context.Context, deploymentID string) (*ent.Approval, error) {
	approvals, err := s.client.Approval.Query().
		Where(entapproval.DeploymentIDEQ(deploymentID)).
		Order(entapproval.ByCreatedAt(sql.OrderDesc())).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(approvals) == 0 {
		return nil, &ent.NotFoundError{}
	}
	return approvals[0], nil
}

func (s *EntStore) ListApprovals(ctx context.Context, limit, offset int, orgID, serviceID, environmentID, status string) ([]*ent.Approval, int, error) {
	q := s.client.Approval.Query()
	if orgID != "" {
		q = q.Where(entapproval.OrgIDEQ(orgID))
	}
	if serviceID != "" {
		q = q.Where(entapproval.ServiceIDEQ(serviceID))
	}
	if environmentID != "" {
		q = q.Where(entapproval.EnvironmentIDEQ(environmentID))
	}
	if status != "" {
		q = q.Where(entapproval.StatusEQ(entapproval.Status(status)))
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
	approvals, err := q.Order(entapproval.ByCreatedAt(sql.OrderDesc())).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return approvals, total, nil
}

func (s *EntStore) UpdateApprovalOne(id string) *ent.ApprovalUpdateOne {
	return s.client.Approval.UpdateOneID(id)
}

func (s *EntStore) SaveApprovalUpdate(ctx context.Context, upd *ent.ApprovalUpdateOne) (*ent.Approval, error) {
	return upd.Save(ctx)
}

func (s *EntStore) ListExpiredApprovals(ctx context.Context, before time.Time) ([]*ent.Approval, error) {
	return s.client.Approval.Query().
		Where(
			entapproval.StatusEQ(entapproval.StatusPending),
			entapproval.TimeoutAtLT(before),
		).
		All(ctx)
}

// --- LookupEntStore Implementation ---

// LookupEntStore adapts *ent.Client to the LookupStore interface.
type LookupEntStore struct {
	client *ent.Client
}

// NewLookupEntStore creates a new LookupEntStore.
func NewLookupEntStore(client *ent.Client) *LookupEntStore {
	return &LookupEntStore{client: client}
}

func (s *LookupEntStore) GetDeploymentByID(ctx context.Context, id string) (*ent.Deployment, error) {
	return s.client.Deployment.Get(ctx, id)
}

func (s *LookupEntStore) GetServiceByID(ctx context.Context, id string) (*ent.Service, error) {
	return s.client.Service.Get(ctx, id)
}

func (s *LookupEntStore) GetEnvironmentByID(ctx context.Context, id string) (*ent.Environment, error) {
	return s.client.Environment.Get(ctx, id)
}

// --- Service Implementation ---

// ServiceImpl implements the Service interface.
type ServiceImpl struct {
	store    ApprovalStore
	lookup   LookupStore
	trigger  DeploymentTrigger
	logger   *zap.Logger
}

// NewService creates a new approval service.
func NewService(store ApprovalStore, lookup LookupStore, trigger DeploymentTrigger, logger *zap.Logger) *ServiceImpl {
	return &ServiceImpl{
		store:  store,
		lookup: lookup,
		trigger: trigger,
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

// RequestApproval creates a pending approval for a deployment.
func (s *ServiceImpl) RequestApproval(ctx context.Context, deploymentID string, req RequestApprovalReq) (*Approval, error) {
	if strings.TrimSpace(deploymentID) == "" {
		return nil, fmt.Errorf("%w: deployment_id is required", ErrInvalidInput)
	}

	// Validate deployment exists
	dep, err := s.lookup.GetDeploymentByID(ctx, deploymentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: deployment not found", ErrDeploymentNotFound)
		}
		return nil, fmt.Errorf("lookup deployment: %w", err)
	}

	// Use deployment's service/environment if not provided
	serviceID := req.ServiceID
	if serviceID == "" {
		serviceID = dep.ServiceID
	}
	environmentID := req.EnvironmentID
	if environmentID == "" {
		environmentID = dep.EnvironmentID
	}

	approvalID := uuid.NewString()
	now := time.Now()
	timeoutAt := now.Add(approvalTimeout)

	builder := s.store.NewApprovalCreate().
		SetID(approvalID).
		SetDeploymentID(deploymentID).
		SetServiceID(serviceID).
		SetEnvironmentID(environmentID).
		SetRequesterID(req.RequesterID).
		SetStatus(entapproval.StatusPending).
		SetTimeoutAt(timeoutAt)

	if req.OrgID != "" {
		builder.SetOrgID(req.OrgID)
	} else if dep.OrgID != "" {
		builder.SetOrgID(dep.OrgID)
	}

	a, err := s.store.SaveApproval(ctx, builder)
	if err != nil {
		return nil, fmt.Errorf("create approval: %w", err)
	}

	s.logger.Info("approval requested",
		zap.String("id", a.ID),
		zap.String("deployment_id", deploymentID),
		zap.String("service_id", serviceID),
		zap.String("environment_id", environmentID),
	)

	return toDomainApproval(a), nil
}

// Approve approves a pending approval and triggers the deployment.
func (s *ServiceImpl) Approve(ctx context.Context, deploymentID string, req ApproveReq) (*Approval, error) {
	a, err := s.store.GetApprovalByDeploymentID(ctx, deploymentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("get approval: %w", err)
	}

	if a.Status != entapproval.StatusPending {
		return nil, fmt.Errorf("%w: current status is %s", ErrApprovalNotPending, a.Status)
	}

	now := time.Now()
	upd := s.store.UpdateApprovalOne(a.ID).
		SetStatus(entapproval.StatusApproved).
		SetApproverID(req.ApproverID).
		SetDecidedAt(now)

	if req.Comment != "" {
		upd.SetComment(req.Comment)
	}

	updated, err := s.store.SaveApprovalUpdate(ctx, upd)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("update approval: %w", err)
	}

	// Trigger the actual deployment
	if s.trigger != nil {
		if err := s.trigger.TriggerDeployment(ctx, deploymentID); err != nil {
			s.logger.Error("failed to trigger deployment after approval",
				zap.String("deployment_id", deploymentID),
				zap.String("approval_id", a.ID),
				zap.Error(err),
			)
			// Don't fail the approval — it's already approved.
			// The deployment can be retried separately.
		}
	}

	s.logger.Info("approval approved",
		zap.String("id", a.ID),
		zap.String("deployment_id", deploymentID),
		zap.String("approver_id", req.ApproverID),
	)

	return toDomainApproval(updated), nil
}

// Reject rejects a pending approval and cancels the deployment.
func (s *ServiceImpl) Reject(ctx context.Context, deploymentID string, req RejectReq) (*Approval, error) {
	a, err := s.store.GetApprovalByDeploymentID(ctx, deploymentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("get approval: %w", err)
	}

	if a.Status != entapproval.StatusPending {
		return nil, fmt.Errorf("%w: current status is %s", ErrApprovalNotPending, a.Status)
	}

	now := time.Now()
	upd := s.store.UpdateApprovalOne(a.ID).
		SetStatus(entapproval.StatusRejected).
		SetApproverID(req.ApproverID).
		SetDecidedAt(now)

	if req.Comment != "" {
		upd.SetComment(req.Comment)
	}

	updated, err := s.store.SaveApprovalUpdate(ctx, upd)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("update approval: %w", err)
	}

	s.logger.Info("approval rejected",
		zap.String("id", a.ID),
		zap.String("deployment_id", deploymentID),
		zap.String("approver_id", req.ApproverID),
	)

	return toDomainApproval(updated), nil
}

// Get returns the approval for a given deployment.
func (s *ServiceImpl) Get(ctx context.Context, deploymentID string) (*Approval, error) {
	a, err := s.store.GetApprovalByDeploymentID(ctx, deploymentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrApprovalNotFound
		}
		return nil, fmt.Errorf("get approval: %w", err)
	}
	return toDomainApproval(a), nil
}

// List returns a filtered, paginated list of approvals.
func (s *ServiceImpl) List(ctx context.Context, filter ApprovalFilter) (*ApprovalListResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	offset := (page - 1) * pageSize

	approvals, total, err := s.store.ListApprovals(ctx, pageSize, offset, filter.OrgID, filter.ServiceID, filter.EnvironmentID, filter.Status)
	if err != nil {
		return nil, fmt.Errorf("query approvals: %w", err)
	}

	result := make([]*Approval, 0, len(approvals))
	for _, a := range approvals {
		result = append(result, toDomainApproval(a))
	}

	return &ApprovalListResult{
		Approvals: result,
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
	}, nil
}

// CheckTimeouts finds pending approvals past their timeout and marks them as timeout.
func (s *ServiceImpl) CheckTimeouts(ctx context.Context) (int, error) {
	now := time.Now()
	expired, err := s.store.ListExpiredApprovals(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("list expired approvals: %w", err)
	}

	count := 0
	for _, a := range expired {
		upd := s.store.UpdateApprovalOne(a.ID).
			SetStatus(entapproval.StatusTimeout).
			SetDecidedAt(now)

		_, err := s.store.SaveApprovalUpdate(ctx, upd)
		if err != nil {
			s.logger.Error("failed to timeout approval",
				zap.String("id", a.ID),
				zap.Error(err),
			)
			continue
		}
		count++

		s.logger.Info("approval timed out",
			zap.String("id", a.ID),
			zap.String("deployment_id", a.DeploymentID),
		)
	}

	if count > 0 {
		s.logger.Info("batch timeout processed", zap.Int("count", count))
	}

	return count, nil
}

// --- Helpers ---

func toDomainApproval(a *ent.Approval) *Approval {
	return &Approval{
		ID:            a.ID,
		OrgID:         a.OrgID,
		DeploymentID:  a.DeploymentID,
		ServiceID:     a.ServiceID,
		EnvironmentID: a.EnvironmentID,
		RequesterID:   a.RequesterID,
		ApproverID:    a.ApproverID,
		Status:        ApprovalStatus(string(a.Status)),
		TimeoutAt:     a.TimeoutAt,
		DecidedAt:     a.DecidedAt,
		Comment:       a.Comment,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}

// Status constants for external use (e.g. deployment update).
var (
	_ = entdeployment.StatusCancelled // ensure deployment package is imported
)
