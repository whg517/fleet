package argocd

import (
	"context"

	"go.uber.org/zap"

	configdomain "github.com/whg517/fleet/internal/domain/config"
	"github.com/whg517/fleet/internal/domain/deployment"
)

// Compile-time interface assertions.
var (
	_ deployment.ArgoCDClient   = (*DeploymentArgoCDAdapter)(nil)
	_ configdomain.ArgoCDClient = (*DeploymentArgoCDAdapter)(nil)
)

// DeploymentArgoCDAdapter wraps the raw Argo CD REST client and adapts it
// to the deployment.ArgoCDClient interface.
// The infra layer imports the domain layer to satisfy the interface,
// which is a standard Go pattern (interfaces are defined where they're used).
type DeploymentArgoCDAdapter struct {
	client *Client
}

// NewDeploymentArgoCDAdapter creates an adapter that satisfies the
// deployment.ArgoCDClient interface.
// Returns an error if baseURL is empty.
func NewDeploymentArgoCDAdapter(baseURL, token string, logger *zap.Logger) (*DeploymentArgoCDAdapter, error) {
	client, err := NewClient(baseURL, token, logger)
	if err != nil {
		return nil, err
	}
	return &DeploymentArgoCDAdapter{
		client: client,
	}, nil
}

// CreateApplication implements deployment.ArgoCDClient.
func (a *DeploymentArgoCDAdapter) CreateApplication(ctx context.Context, req deployment.ArgoCDAppReq) error {
	return a.client.CreateApplication(ctx, req.Name, req.Namespace, req.RepoURL, req.Chart, req.TargetRev, req.Values)
}

// GetApplication implements deployment.ArgoCDClient.
func (a *DeploymentArgoCDAdapter) GetApplication(ctx context.Context, name string) (*deployment.ArgoCDAppStatus, error) {
	status, err := a.client.GetApplication(ctx, name)
	if err != nil {
		return nil, err
	}
	return &deployment.ArgoCDAppStatus{
		SyncStatus:   status.SyncStatus,
		HealthStatus: status.HealthStatus,
	}, nil
}

// SyncApplication implements deployment.ArgoCDClient.
func (a *DeploymentArgoCDAdapter) SyncApplication(ctx context.Context, name string) error {
	return a.client.SyncApplication(ctx, name)
}

// RollbackApplication implements deployment.ArgoCDClient.
func (a *DeploymentArgoCDAdapter) RollbackApplication(ctx context.Context, name, revision string) error {
	return a.client.RollbackApplication(ctx, name, revision)
}

// DeleteApplication implements deployment.ArgoCDClient.
func (a *DeploymentArgoCDAdapter) DeleteApplication(ctx context.Context, name string) error {
	return a.client.DeleteApplication(ctx, name)
}

// UpdateApplication implements deployment.ArgoCDClient.
func (a *DeploymentArgoCDAdapter) UpdateApplication(ctx context.Context, name string, values map[string]any) error {
	return a.client.UpdateApplication(ctx, name, values)
}
