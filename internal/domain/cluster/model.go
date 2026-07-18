package cluster

import (
	"errors"
	"time"
)

var (
	// ErrClusterNotFound is returned when a cluster does not exist.
	ErrClusterNotFound = errors.New("cluster not found")
	// ErrClusterAlreadyExists is returned when a cluster name already exists.
	ErrClusterAlreadyExists = errors.New("cluster already exists")
	// ErrInvalidKubeconfig is returned when kubeconfig is invalid.
	ErrInvalidKubeconfig = errors.New("invalid kubeconfig")
	// ErrClusterHasEnvironments is returned when attempting to delete a cluster with active environments.
	ErrClusterHasEnvironments = errors.New("cluster has active environments, cannot delete")
	// ErrEnvironmentAlreadyExists is returned when an environment already exists for the cluster.
	ErrEnvironmentAlreadyExists = errors.New("environment already exists for this cluster")
	// ErrEnvironmentNotFound is returned when an environment does not exist.
	ErrEnvironmentNotFound = errors.New("environment not found")
	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
)

// Cluster represents a Kubernetes cluster registration.
type Cluster struct {
	ID        string            `json:"id"`
	OrgID     string            `json:"org_id,omitempty"`
	Name      string            `json:"name"`
	APIServer string            `json:"api_server"`
	Labels    map[string]string `json:"labels,omitempty"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Environment represents a deployment environment within a cluster.
type Environment struct {
	ID              string         `json:"id"`
	OrgID           string         `json:"org_id,omitempty"`
	Name            string         `json:"name"`
	ClusterID       string         `json:"cluster_id"`
	NamespacePattern string        `json:"namespace_pattern,omitempty"`
	ApprovalRequired bool          `json:"approval_required"`
	ApproverRole    string         `json:"approver_role,omitempty"`
	ConfigOverrides map[string]any `json:"config_overrides,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// CreateClusterReq is the request payload for creating a cluster.
type CreateClusterReq struct {
	OrgID      string            `json:"org_id,omitempty"`
	Name       string            `json:"name"`
	APIServer  string            `json:"api_server"`
	Kubeconfig string            `json:"kubeconfig"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// UpdateClusterReq is the request payload for updating a cluster.
type UpdateClusterReq struct {
	Name       *string            `json:"name,omitempty"`
	APIServer  *string            `json:"api_server,omitempty"`
	Kubeconfig *string            `json:"kubeconfig,omitempty"`
	Labels     *map[string]string `json:"labels,omitempty"`
	Status     *string            `json:"status,omitempty"`
}

// ClusterFilter is used for filtering and paginating cluster list queries.
type ClusterFilter struct {
	OrgID    string
	Page     int
	PageSize int
	Labels   map[string]string
	Status   string
}

// CreateEnvReq is the request payload for creating an environment.
type CreateEnvReq struct {
	Name             string         `json:"name"`
	NamespacePattern string         `json:"namespace_pattern,omitempty"`
	ApprovalRequired bool           `json:"approval_required"`
	ApproverRole     string         `json:"approver_role,omitempty"`
	ConfigOverrides  map[string]any `json:"config_overrides,omitempty"`
}

// ClusterListResult holds a paginated list of clusters.
type ClusterListResult struct {
	Clusters  []*Cluster
	Total     int
	Page      int
	PageSize  int
}

// Pagination response helper.
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}
