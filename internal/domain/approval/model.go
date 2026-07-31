package approval

import (
	"errors"
	"time"
)

var (
	// ErrApprovalNotFound is returned when an approval does not exist.
	ErrApprovalNotFound = errors.New("approval not found")
	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
	// ErrApprovalNotPending is returned when an approval is not in a pending state.
	ErrApprovalNotPending = errors.New("approval is not pending")
	// ErrDeploymentNotFound is returned when a deployment does not exist.
	ErrDeploymentNotFound = errors.New("deployment not found")
)

// ApprovalStatus defines the lifecycle state of an Approval.
type ApprovalStatus string

const (
	StatusPending   ApprovalStatus = "pending"
	StatusApproved  ApprovalStatus = "approved"
	StatusRejected  ApprovalStatus = "rejected"
	StatusTimeout   ApprovalStatus = "timeout"
	StatusCancelled ApprovalStatus = "cancelled"
)

// Approval represents an approval request for a deployment.
type Approval struct {
	ID            string         `json:"id"`
	OrgID         string         `json:"org_id,omitempty"`
	DeploymentID  string         `json:"deployment_id"`
	ServiceID     string         `json:"service_id"`
	EnvironmentID string         `json:"environment_id"`
	RequesterID   string         `json:"requester_id"`
	ApproverID    string         `json:"approver_id,omitempty"`
	Status        ApprovalStatus `json:"status"`
	TimeoutAt     time.Time      `json:"timeout_at"`
	DecidedAt     *time.Time     `json:"decided_at,omitempty"`
	Comment       string         `json:"comment,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// RequestApprovalReq is the request payload for creating an approval.
type RequestApprovalReq struct {
	OrgID         string `json:"org_id,omitempty"`
	DeploymentID  string `json:"-"`
	ServiceID     string `json:"-"`
	EnvironmentID string `json:"-"`
	RequesterID   string `json:"-"`
}

// ApproveReq is the request payload for approving a deployment.
type ApproveReq struct {
	ApproverID string `json:"-"`
	Comment    string `json:"comment"`
}

// RejectReq is the request payload for rejecting a deployment.
type RejectReq struct {
	ApproverID string `json:"-"`
	Comment    string `json:"comment"`
}

// ApprovalFilter is used for filtering and paginating approvals.
type ApprovalFilter struct {
	OrgID         string
	ServiceID     string
	EnvironmentID string
	DeploymentID  string
	Status        string
	Page          int
	PageSize      int
}

// ApprovalListResult holds a paginated list of approvals.
type ApprovalListResult struct {
	Approvals []*Approval
	Total     int
	Page      int
	PageSize  int
}
