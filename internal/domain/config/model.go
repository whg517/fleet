package config

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrConfigNotFound is returned when a config snapshot does not exist.
	ErrConfigNotFound = errors.New("config snapshot not found")
	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
	// ErrArgoCDUnavailable is returned when the Argo CD API call fails.
	ErrArgoCDUnavailable = errors.New("argo cd unavailable")
	// ErrNoActiveDeployment is returned when no active deployment exists for the service+environment.
	ErrNoActiveDeployment = errors.New("no active deployment for this service and environment")
)

// ConfigSnapshot represents a Helm values configuration snapshot.
type ConfigSnapshot struct {
	ID             string         `json:"id"`
	OrgID          string         `json:"org_id,omitempty"`
	ServiceID      string         `json:"service_id"`
	EnvironmentID  string         `json:"environment_id"`
	Values         map[string]any `json:"values"`
	PreviousValues map[string]any `json:"previous_values,omitempty"`
	ChangedBy      string         `json:"changed_by,omitempty"`
	ChangeReason   string         `json:"change_reason,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// UpdateValuesReq is the request payload for updating Helm values.
type UpdateValuesReq struct {
	ServiceID     string         `json:"-"`
	EnvironmentID string         `json:"-"`
	OrgID         string         `json:"org_id,omitempty"`
	Values        map[string]any `json:"values"`
	ChangedBy     string         `json:"-"`
	ChangeReason  string         `json:"change_reason,omitempty"`
}

// ConfigHistoryResult holds a paginated list of config snapshots.
type ConfigHistoryResult struct {
	Snapshots []*ConfigSnapshot `json:"data"`
	Total     int               `json:"total"`
	Page      int               `json:"page"`
	PageSize  int               `json:"page_size"`
}

// ConfigDiffEntry represents a single difference between two config versions.
type ConfigDiffEntry struct {
	Path     string `json:"path"`
	Type     string `json:"type"` // "added", "removed", "changed"
	OldValue any    `json:"old_value,omitempty"`
	NewValue any    `json:"new_value,omitempty"`
}

// ConfigDiff holds the result of diffing two config snapshots.
type ConfigDiff struct {
	FromSnapshotID string            `json:"from_snapshot_id"`
	ToSnapshotID   string            `json:"to_snapshot_id"`
	Entries        []ConfigDiffEntry `json:"entries"`
}

// ArgoCDClient abstracts Argo CD application update operations.
// We reuse the deployment.ArgoCDClient by parameterizing on this minimal interface.
type ArgoCDClient interface {
	UpdateApplication(ctx context.Context, name string, values map[string]any) error
}
