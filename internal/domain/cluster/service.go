package cluster

import (
	"context"
)

// Service defines the cluster management operations.
type Service interface {
	Create(ctx context.Context, req CreateClusterReq) (*Cluster, error)
	List(ctx context.Context, filter ClusterFilter) (*ClusterListResult, error)
	Get(ctx context.Context, id string) (*Cluster, error)
	Update(ctx context.Context, id string, req UpdateClusterReq) (*Cluster, error)
	Delete(ctx context.Context, id string) error
	TestConnection(ctx context.Context, id string) error
	CreateEnvironment(ctx context.Context, clusterID string, req CreateEnvReq) (*Environment, error)
	ListEnvironments(ctx context.Context, clusterID string) ([]*Environment, error)
}
