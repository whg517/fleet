package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/yaml"

	entcluster "github.com/whg517/fleet/internal/store/ent/cluster"
	"github.com/whg517/fleet/internal/store/ent/environment"
	"github.com/whg517/fleet/internal/store/ent"
	"github.com/whg517/fleet/internal/infra/secrets"
)

// ClusterStore abstracts the Ent client operations the service needs.
// This allows mocking in tests without a real database.
type ClusterStore interface {
	// Cluster operations
	NewClusterCreate() *ent.ClusterCreate
	SaveCluster(ctx context.Context, c *ent.ClusterCreate) (*ent.Cluster, error)
	GetCluster(ctx context.Context, id string) (*ent.Cluster, error)
	UpdateClusterOne(id string) *ent.ClusterUpdateOne
	SaveClusterUpdate(ctx context.Context, upd *ent.ClusterUpdateOne) (*ent.Cluster, error)
	DeleteCluster(ctx context.Context, id string) error
	ListClusters(ctx context.Context, limit, offset int, orgID, status string) ([]*ent.Cluster, int, error)
	CountClusterEnvironments(ctx context.Context, clusterID string) (int, error)
	// Environment operations
	NewEnvironmentCreate() *ent.EnvironmentCreate
	SaveEnvironment(ctx context.Context, e *ent.EnvironmentCreate) (*ent.Environment, error)
	ListEnvironments(ctx context.Context, clusterID string) ([]*ent.Environment, error)
	CountEnvironmentsByName(ctx context.Context, clusterID string, name environment.Name) (int, error)
}

// EntStore adapts *ent.Client to the ClusterStore interface.
type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) NewClusterCreate() *ent.ClusterCreate {
	return s.client.Cluster.Create()
}

func (s *EntStore) SaveCluster(ctx context.Context, c *ent.ClusterCreate) (*ent.Cluster, error) {
	return c.Save(ctx)
}

func (s *EntStore) GetCluster(ctx context.Context, id string) (*ent.Cluster, error) {
	return s.client.Cluster.Get(ctx, id)
}

func (s *EntStore) UpdateClusterOne(id string) *ent.ClusterUpdateOne {
	return s.client.Cluster.UpdateOneID(id)
}

func (s *EntStore) SaveClusterUpdate(ctx context.Context, upd *ent.ClusterUpdateOne) (*ent.Cluster, error) {
	return upd.Save(ctx)
}

func (s *EntStore) DeleteCluster(ctx context.Context, id string) error {
	return s.client.Cluster.DeleteOneID(id).Exec(ctx)
}

func (s *EntStore) ListClusters(ctx context.Context, limit, offset int, orgID, status string) ([]*ent.Cluster, int, error) {
	q := s.client.Cluster.Query()
	if orgID != "" {
		q = q.Where(entcluster.OrgIDEQ(orgID))
	}
	if status != "" {
		q = q.Where(entcluster.StatusEQ(entcluster.Status(status)))
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
	clusters, err := q.Order(entcluster.ByCreatedAt()).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return clusters, total, nil
}

func (s *EntStore) CountClusterEnvironments(ctx context.Context, clusterID string) (int, error) {
	return s.client.Environment.Query().Where(environment.ClusterIDEQ(clusterID)).Count(ctx)
}

func (s *EntStore) NewEnvironmentCreate() *ent.EnvironmentCreate {
	return s.client.Environment.Create()
}

func (s *EntStore) SaveEnvironment(ctx context.Context, e *ent.EnvironmentCreate) (*ent.Environment, error) {
	return e.Save(ctx)
}

func (s *EntStore) ListEnvironments(ctx context.Context, clusterID string) ([]*ent.Environment, error) {
	return s.client.Environment.Query().
		Where(environment.ClusterIDEQ(clusterID)).
		Order(environment.ByCreatedAt()).
		All(ctx)
}

func (s *EntStore) CountEnvironmentsByName(ctx context.Context, clusterID string, name environment.Name) (int, error) {
	return s.client.Environment.Query().
		Where(environment.ClusterIDEQ(clusterID), environment.NameEQ(name)).
		Count(ctx)
}

// --- Service Implementation ---

// ServiceImpl implements the Service interface.
type ServiceImpl struct {
	store  ClusterStore
	dek    []byte
	logger *zap.Logger
}

// NewService creates a new cluster service.
func NewService(store ClusterStore, dek []byte, logger *zap.Logger) *ServiceImpl {
	return &ServiceImpl{
		store:  store,
		dek:    dek,
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

func validateCreateReq(req CreateClusterReq) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.APIServer) == "" {
		return fmt.Errorf("%w: api_server is required", ErrInvalidInput)
	}
	if strings.TrimSpace(req.Kubeconfig) == "" {
		return fmt.Errorf("%w: kubeconfig is required", ErrInvalidInput)
	}
	return nil
}

// Create registers a new cluster.
func (s *ServiceImpl) Create(ctx context.Context, req CreateClusterReq) (*Cluster, error) {
	if err := validateCreateReq(req); err != nil {
		return nil, err
	}

	encrypted, err := secrets.Encrypt(s.dek, []byte(req.Kubeconfig))
	if err != nil {
		s.logger.Error("failed to encrypt kubeconfig", zap.Error(err))
		return nil, fmt.Errorf("failed to store kubeconfig: %w", err)
	}

	clusterID := uuid.NewString()

	labels := req.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	builder := s.store.NewClusterCreate().
		SetID(clusterID).
		SetName(req.Name).
		SetAPIServer(req.APIServer).
		SetKubeconfigEncrypted(encrypted).
		SetLabels(labels)

	if req.OrgID != "" {
		builder.SetOrgID(req.OrgID)
	}

	c, err := s.store.SaveCluster(ctx, builder)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrClusterAlreadyExists
		}
		return nil, fmt.Errorf("create cluster: %w", err)
	}

	s.logger.Info("cluster created",
		zap.String("id", c.ID),
		zap.String("name", c.Name),
	)

	return toDomainCluster(c), nil
}

// List returns a paginated, filtered list of clusters.
//
// Note: Label filtering is performed in application memory because the ent
// store layer does not yet support JSON predicate queries on the labels field.
// When label filters are active, we fetch all matching org/status clusters
// (without SQL pagination), filter in memory, then manually paginate the
// result set. This is acceptable for the expected cluster count (< 1000).
// TODO: push label filtering into the store layer once ent JSON predicates
// are wired.
func (s *ServiceImpl) List(ctx context.Context, filter ClusterFilter) (*ClusterListResult, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)

	var result []*Cluster
	var total int

	if len(filter.Labels) > 0 {
		// Label filter active: fetch all (no pagination), filter in memory, then paginate.
		clusters, _, err := s.store.ListClusters(ctx, 0, 0, filter.OrgID, filter.Status)
		if err != nil {
			return nil, fmt.Errorf("query clusters: %w", err)
		}

		filtered := make([]*Cluster, 0, len(clusters))
		for _, c := range clusters {
			if matchLabels(c.Labels, filter.Labels) {
				filtered = append(filtered, toDomainCluster(c))
			}
		}

		total = len(filtered)
		offset := (page - 1) * pageSize
		if offset >= total {
			result = []*Cluster{}
		} else if offset+pageSize >= total {
			result = filtered[offset:]
		} else {
			result = filtered[offset : offset+pageSize]
		}
	} else {
		// No label filter: use SQL pagination directly.
		clusters, t, err := s.store.ListClusters(ctx, pageSize, (page-1)*pageSize, filter.OrgID, filter.Status)
		if err != nil {
			return nil, fmt.Errorf("query clusters: %w", err)
		}
		result = make([]*Cluster, 0, len(clusters))
		for _, c := range clusters {
			result = append(result, toDomainCluster(c))
		}
		total = t
	}

	return &ClusterListResult{
		Clusters: result,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// Get returns a single cluster by ID.
func (s *ServiceImpl) Get(ctx context.Context, id string) (*Cluster, error) {
	c, err := s.store.GetCluster(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrClusterNotFound
		}
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	return toDomainCluster(c), nil
}

// Update modifies an existing cluster.
func (s *ServiceImpl) Update(ctx context.Context, id string, req UpdateClusterReq) (*Cluster, error) {
	upd := s.store.UpdateClusterOne(id)
	if req.Name != nil {
		upd.SetName(*req.Name)
	}
	if req.APIServer != nil {
		upd.SetAPIServer(*req.APIServer)
	}
	if req.Status != nil {
		upd.SetStatus(entcluster.Status(*req.Status))
	}
	if req.Labels != nil {
		upd.SetLabels(*req.Labels)
	}
	if req.Kubeconfig != nil {
		encrypted, encErr := secrets.Encrypt(s.dek, []byte(*req.Kubeconfig))
		if encErr != nil {
			return nil, fmt.Errorf("failed to encrypt kubeconfig: %w", encErr)
		}
		upd.SetKubeconfigEncrypted(encrypted)
	}

	updated, err := s.store.SaveClusterUpdate(ctx, upd)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrClusterNotFound
		}
		return nil, fmt.Errorf("update cluster: %w", err)
	}

	s.logger.Info("cluster updated", zap.String("id", id))
	return toDomainCluster(updated), nil
}

// Delete removes a cluster after checking for active environments.
func (s *ServiceImpl) Delete(ctx context.Context, id string) error {
	_, err := s.store.GetCluster(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrClusterNotFound
		}
		return fmt.Errorf("get cluster for delete: %w", err)
	}

	envCount, err := s.store.CountClusterEnvironments(ctx, id)
	if err != nil {
		return fmt.Errorf("check environments: %w", err)
	}
	if envCount > 0 {
		return ErrClusterHasEnvironments
	}

	if err := s.store.DeleteCluster(ctx, id); err != nil {
		return fmt.Errorf("delete cluster: %w", err)
	}

	s.logger.Info("cluster deleted", zap.String("id", id))
	return nil
}

// TestConnection validates the cluster's kubeconfig by calling the Kubernetes API.
func (s *ServiceImpl) TestConnection(ctx context.Context, id string) error {
	c, err := s.store.GetCluster(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrClusterNotFound
		}
		return fmt.Errorf("get cluster: %w", err)
	}

	if len(c.KubeconfigEncrypted) == 0 {
		return ErrInvalidKubeconfig
	}

	plaintext, err := secrets.Decrypt(s.dek, c.KubeconfigEncrypted)
	if err != nil {
		return fmt.Errorf("decrypt kubeconfig: %w", err)
	}

	restConfig, err := buildRESTConfig(string(plaintext), c.APIServer)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidKubeconfig, err)
	}

	// Set a 10-second timeout on the REST client itself.
	restConfig.Timeout = 10 * time.Second

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create discovery client: %w", err)
	}

	_, err = discoveryClient.ServerVersion()
	if err != nil {
		return fmt.Errorf("kubernetes API unreachable: %w", err)
	}

	s.logger.Info("cluster connection test passed", zap.String("id", id))
	return nil
}

// CreateEnvironment creates a new environment for a cluster.
func (s *ServiceImpl) CreateEnvironment(ctx context.Context, clusterID string, req CreateEnvReq) (*Environment, error) {
	c, err := s.store.GetCluster(ctx, clusterID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrClusterNotFound
		}
		return nil, fmt.Errorf("get cluster: %w", err)
	}

	envName := environment.Name(req.Name)
	if err := environment.NameValidator(envName); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	existing, err := s.store.CountEnvironmentsByName(ctx, clusterID, envName)
	if err != nil {
		return nil, fmt.Errorf("check existing environment: %w", err)
	}
	if existing > 0 {
		return nil, ErrEnvironmentAlreadyExists
	}

	overrides := req.ConfigOverrides
	if overrides == nil {
		overrides = map[string]any{}
	}

	envID := uuid.NewString()

	builder := s.store.NewEnvironmentCreate().
		SetID(envID).
		SetName(envName).
		SetClusterID(clusterID).
		SetApprovalRequired(req.ApprovalRequired).
		SetConfigOverrides(overrides)

	if c.OrgID != "" {
		builder.SetOrgID(c.OrgID)
	}
	if req.NamespacePattern != "" {
		builder.SetNamespacePattern(req.NamespacePattern)
	}
	if req.ApproverRole != "" {
		builder.SetApproverRole(req.ApproverRole)
	}

	env, err := s.store.SaveEnvironment(ctx, builder)
	if err != nil {
		return nil, fmt.Errorf("create environment: %w", err)
	}

	s.logger.Info("environment created",
		zap.String("id", env.ID),
		zap.String("cluster_id", clusterID),
		zap.String("name", req.Name),
	)

	return toDomainEnvironment(env), nil
}

// ListEnvironments returns all environments for a cluster.
func (s *ServiceImpl) ListEnvironments(ctx context.Context, clusterID string) ([]*Environment, error) {
	envs, err := s.store.ListEnvironments(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}

	result := make([]*Environment, 0, len(envs))
	for _, e := range envs {
		result = append(result, toDomainEnvironment(e))
	}
	return result, nil
}

// --- Helpers ---

func matchLabels(clusterLabels, filterLabels map[string]string) bool {
	if len(filterLabels) == 0 {
		return true
	}
	for k, v := range filterLabels {
		if clusterLabels[k] != v {
			return false
		}
	}
	return true
}

func toDomainCluster(c *ent.Cluster) *Cluster {
	return &Cluster{
		ID:        c.ID,
		OrgID:     c.OrgID,
		Name:      c.Name,
		APIServer: c.APIServer,
		Labels:    c.Labels,
		Status:    string(c.Status),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

func toDomainEnvironment(e *ent.Environment) *Environment {
	return &Environment{
		ID:               e.ID,
		OrgID:            e.OrgID,
		Name:             string(e.Name),
		ClusterID:        e.ClusterID,
		NamespacePattern: e.NamespacePattern,
		ApprovalRequired: e.ApprovalRequired,
		ApproverRole:     e.ApproverRole,
		ConfigOverrides:  e.ConfigOverrides,
		CreatedAt:        e.CreatedAt,
		UpdatedAt:        e.UpdatedAt,
	}
}

// buildRESTConfig parses a kubeconfig YAML and returns a k8s rest.Config.
func buildRESTConfig(kubeconfigYAML, apiServerOverride string) (*rest.Config, error) {
	parsedConfig := api.Config{}
	if err := yaml.Unmarshal([]byte(kubeconfigYAML), &parsedConfig); err != nil {
		return nil, fmt.Errorf("parse kubeconfig yaml: %w", err)
	}

	loader := clientcmd.NewNonInteractiveClientConfig(
		parsedConfig,
		parsedConfig.CurrentContext,
		&clientcmd.ConfigOverrides{},
		nil,
	)

	restConfig, err := loader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	if apiServerOverride != "" {
		restConfig.Host = apiServerOverride
	}

	return restConfig, nil
}
