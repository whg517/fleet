package config

import (
	"context"

	"github.com/whg517/fleet/internal/store/ent"
)

// ConfigStore abstracts Ent client operations for ConfigSnapshot.
type ConfigStore interface {
	NewConfigSnapshotCreate() *ent.ConfigSnapshotCreate
	SaveSnapshot(ctx context.Context, c *ent.ConfigSnapshotCreate) (*ent.ConfigSnapshot, error)
	ListSnapshots(ctx context.Context, limit, offset int, orgID, serviceID, environmentID string) ([]*ent.ConfigSnapshot, int, error)
	GetSnapshot(ctx context.Context, id string) (*ent.ConfigSnapshot, error)
}

// LookupStore provides read-only access to related entities needed for validation.
// This is a subset of the deployment.LookupStore, focused on what config needs.
type LookupStore interface {
	GetServiceByID(ctx context.Context, id string) (*ent.Service, error)
	GetEnvironmentByID(ctx context.Context, id string) (*ent.Environment, error)
	GetLatestDeployment(ctx context.Context, serviceID, environmentID string) (*ent.Deployment, error)
}

// Service defines the configuration management operations.
type Service interface {
	// UpdateValues modifies Helm values for a service+environment and triggers Argo CD update.
	UpdateValues(ctx context.Context, serviceID, environmentID string, req UpdateValuesReq) (*ConfigSnapshot, error)

	// GetValues returns the current effective Helm values for a service+environment.
	GetValues(ctx context.Context, serviceID, environmentID string) (map[string]any, error)

	// ListHistory returns a paginated list of config changes for a service+environment.
	ListHistory(ctx context.Context, serviceID, environmentID string, page, pageSize int) (*ConfigHistoryResult, error)

	// Diff compares two config snapshots by ID and returns the differences.
	Diff(ctx context.Context, serviceID, environmentID string, fromVer, toVer string) (*ConfigDiff, error)
}
