package cluster

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent/enttest"
	"github.com/whg517/fleet/internal/store/ent"
	entcluster "github.com/whg517/fleet/internal/store/ent/cluster"
	"github.com/whg517/fleet/internal/store/ent/environment"

	modernsqlite "modernc.org/sqlite"
)

func init() {
	// Register modernc.org/sqlite under the "sqlite3" name that ent expects.
	sql.Register("sqlite3", &sqliteFKDriver{inner: &modernsqlite.Driver{}})
}

// validKubeconfig is a minimal valid kubeconfig YAML for testing.
const validKubeconfig = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`

func newTestService(t *testing.T) (*ServiceImpl, *ent.Client) {
	t.Helper()

	// Use a unique file name per test to avoid shared state.
	// Use ":memory:" for a private in-memory database.
	client := enttest.Open(t, "sqlite3", fmt.Sprintf("file:%s?mode=memory&_fk=1&_pragma=foreign_keys(1)", t.Name()))

	dek, _ := hex.DecodeString("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	store := NewEntStore(client)
	svc := NewService(store, dek, zap.NewNop())

	t.Cleanup(func() { _ = client.Close() })

	return svc, client
}

func TestCreateCluster_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cl, err := svc.Create(ctx, CreateClusterReq{
		Name:       "prod-cluster",
		APIServer:  "https://10.0.0.1:6443",
		Kubeconfig: validKubeconfig,
		Labels:     map[string]string{"env": "prod", "region": "us-east-1"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if cl.ID == "" {
		t.Error("expected non-empty ID")
	}
	if cl.Name != "prod-cluster" {
		t.Errorf("Name: got %q, want %q", cl.Name, "prod-cluster")
	}
	if cl.Status != "active" {
		t.Errorf("Status: got %q, want %q", cl.Status, "active")
	}
	if cl.Labels["env"] != "prod" {
		t.Errorf("Labels[env]: got %q", cl.Labels["env"])
	}
}

func TestCreateCluster_ValidationErrors(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tests := []struct {
		name string
		req  CreateClusterReq
	}{
		{"empty name", CreateClusterReq{APIServer: "https://x", Kubeconfig: "y"}},
		{"empty api_server", CreateClusterReq{Name: "x", Kubeconfig: "y"}},
		{"empty kubeconfig", CreateClusterReq{Name: "x", APIServer: "https://x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tt.req)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestGetCluster_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Get(ctx, "nonexistent")
	if err != ErrClusterNotFound {
		t.Errorf("Get: got %v, want ErrClusterNotFound", err)
	}
}

func TestGetCluster_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cl, err := svc.Create(ctx, CreateClusterReq{
		Name:       "test",
		APIServer:  "https://10.0.0.1:6443",
		Kubeconfig: validKubeconfig,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get(ctx, cl.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != cl.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, cl.ID)
	}
	if got.Name != cl.Name {
		t.Errorf("Name mismatch: got %q, want %q", got.Name, cl.Name)
	}
}

func TestListClusters_Pagination(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create 5 clusters
	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, CreateClusterReq{
			Name:       "cluster-" + string(rune('a'+i)),
			APIServer:  "https://10.0.0.1:6443",
			Kubeconfig: validKubeconfig,
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// Page 1, size 2
	result, err := svc.List(ctx, ClusterFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Clusters) != 2 {
		t.Errorf("Page 1: got %d clusters, want 2", len(result.Clusters))
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}

	// Page 3, size 2 (should have 1)
	result, err = svc.List(ctx, ClusterFilter{Page: 3, PageSize: 2})
	if err != nil {
		t.Fatalf("List page 3: %v", err)
	}
	if len(result.Clusters) != 1 {
		t.Errorf("Page 3: got %d clusters, want 1", len(result.Clusters))
	}
}

func TestListClusters_LabelFilter(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateClusterReq{
		Name:       "a",
		APIServer:  "https://a",
		Kubeconfig: validKubeconfig,
		Labels:     map[string]string{"team": "infra"},
	})
	_, _ = svc.Create(ctx, CreateClusterReq{
		Name:       "b",
		APIServer:  "https://b",
		Kubeconfig: validKubeconfig,
		Labels:     map[string]string{"team": "app"},
	})

	result, err := svc.List(ctx, ClusterFilter{Page: 1, PageSize: 10, Labels: map[string]string{"team": "infra"}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(result.Clusters))
	}
	if result.Clusters[0].Name != "a" {
		t.Errorf("Name: got %q, want %q", result.Clusters[0].Name, "a")
	}
}

func TestUpdateCluster_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cl, _ := svc.Create(ctx, CreateClusterReq{
		Name:       "orig",
		APIServer:  "https://orig",
		Kubeconfig: validKubeconfig,
	})

	newName := "updated"
	updated, err := svc.Update(ctx, cl.ID, UpdateClusterReq{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "updated" {
		t.Errorf("Name: got %q, want %q", updated.Name, "updated")
	}
}

func TestUpdateCluster_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	name := "x"
	_, err := svc.Update(ctx, "nonexistent", UpdateClusterReq{Name: &name})
	if err != ErrClusterNotFound {
		t.Errorf("Update: got %v, want ErrClusterNotFound", err)
	}
}

func TestUpdateCluster_Kubeconfig(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	cl, _ := svc.Create(ctx, CreateClusterReq{
		Name:       "test",
		APIServer:  "https://x",
		Kubeconfig: validKubeconfig,
	})

	newKubeconfig := "updated-kubeconfig-data"
	_, err := svc.Update(ctx, cl.ID, UpdateClusterReq{Kubeconfig: &newKubeconfig})
	if err != nil {
		t.Fatalf("Update kubeconfig: %v", err)
	}

	// Verify the stored encrypted kubeconfig changed
	c, _ := client.Cluster.Get(ctx, cl.ID)
	if string(c.KubeconfigEncrypted) == validKubeconfig {
		t.Error("kubeconfig was not re-encrypted")
	}
}

func TestDeleteCluster_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cl, _ := svc.Create(ctx, CreateClusterReq{
		Name:       "to-delete",
		APIServer:  "https://x",
		Kubeconfig: validKubeconfig,
	})

	if err := svc.Delete(ctx, cl.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := svc.Get(ctx, cl.ID)
	if err != ErrClusterNotFound {
		t.Errorf("expected ErrClusterNotFound after delete, got %v", err)
	}
}

func TestDeleteCluster_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if err := svc.Delete(ctx, "nonexistent"); err != ErrClusterNotFound {
		t.Errorf("Delete: got %v, want ErrClusterNotFound", err)
	}
}

func TestDeleteCluster_HasEnvironments(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	cl, _ := svc.Create(ctx, CreateClusterReq{
		Name:       "with-env",
		APIServer:  "https://x",
		Kubeconfig: validKubeconfig,
	})

	// Manually insert an environment to block deletion
	_, err := client.Environment.Create().
		SetID(uuid.NewString()).
		SetName(environment.NameDev).
		SetClusterID(cl.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create environment: %v", err)
	}

	err = svc.Delete(ctx, cl.ID)
	if err != ErrClusterHasEnvironments {
		t.Errorf("Delete: got %v, want ErrClusterHasEnvironments", err)
	}
}

func TestCreateEnvironment_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cl, _ := svc.Create(ctx, CreateClusterReq{
		Name:       "test",
		APIServer:  "https://x",
		Kubeconfig: validKubeconfig,
	})

	env, err := svc.CreateEnvironment(ctx, cl.ID, CreateEnvReq{
		Name:             "dev",
		NamespacePattern: "team-.*",
		ApprovalRequired: true,
		ApproverRole:     "admin",
	})
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	if env.ID == "" {
		t.Error("expected non-empty ID")
	}
	if env.Name != "dev" {
		t.Errorf("Name: got %q, want %q", env.Name, "dev")
	}
	if env.ClusterID != cl.ID {
		t.Errorf("ClusterID: got %q, want %q", env.ClusterID, cl.ID)
	}
	if !env.ApprovalRequired {
		t.Error("ApprovalRequired: got false, want true")
	}
}

func TestCreateEnvironment_ClusterNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.CreateEnvironment(ctx, "nonexistent", CreateEnvReq{Name: "dev"})
	if err != ErrClusterNotFound {
		t.Errorf("CreateEnvironment: got %v, want ErrClusterNotFound", err)
	}
}

func TestCreateEnvironment_InvalidName(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cl, _ := svc.Create(ctx, CreateClusterReq{
		Name:       "test",
		APIServer:  "https://x",
		Kubeconfig: validKubeconfig,
	})

	_, err := svc.CreateEnvironment(ctx, cl.ID, CreateEnvReq{Name: "staging"})
	if err == nil {
		t.Fatal("expected error for invalid environment name")
	}
}

func TestCreateEnvironment_Duplicate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cl, _ := svc.Create(ctx, CreateClusterReq{
		Name:       "test",
		APIServer:  "https://x",
		Kubeconfig: validKubeconfig,
	})

	_, _ = svc.CreateEnvironment(ctx, cl.ID, CreateEnvReq{Name: "dev"})

	_, err := svc.CreateEnvironment(ctx, cl.ID, CreateEnvReq{Name: "dev"})
	if err != ErrEnvironmentAlreadyExists {
		t.Errorf("CreateEnvironment duplicate: got %v, want ErrEnvironmentAlreadyExists", err)
	}
}

func TestListEnvironments(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	cl, _ := svc.Create(ctx, CreateClusterReq{
		Name:       "test",
		APIServer:  "https://x",
		Kubeconfig: validKubeconfig,
	})

	_, _ = svc.CreateEnvironment(ctx, cl.ID, CreateEnvReq{Name: "dev"})
	_, _ = svc.CreateEnvironment(ctx, cl.ID, CreateEnvReq{Name: "test"})

	envs, err := svc.ListEnvironments(ctx, cl.ID)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	if len(envs) != 2 {
		t.Errorf("got %d environments, want 2", len(envs))
	}
}

func TestNormalizePage(t *testing.T) {
	tests := []struct {
		page, pageSize, wantPage, wantPageSize int
	}{
		{0, 0, 1, 20},
		{-1, -5, 1, 20},
		{1, 10, 1, 10},
		{5, 200, 5, 100},
		{3, 50, 3, 50},
	}

	for _, tt := range tests {
		page, pageSize := normalizePage(tt.page, tt.pageSize)
		if page != tt.wantPage || pageSize != tt.wantPageSize {
			t.Errorf("normalizePage(%d, %d): got (%d, %d), want (%d, %d)",
				tt.page, tt.pageSize, page, pageSize, tt.wantPage, tt.wantPageSize)
		}
	}
}

func TestMatchLabels(t *testing.T) {
	tests := []struct {
		name         string
		cluster      map[string]string
		filter       map[string]string
		wantMatch    bool
	}{
		{"no filter", map[string]string{"a": "b"}, nil, true},
		{"exact match", map[string]string{"a": "b"}, map[string]string{"a": "b"}, true},
		{"no match", map[string]string{"a": "b"}, map[string]string{"a": "c"}, false},
		{"missing key", map[string]string{"a": "b"}, map[string]string{"c": "d"}, false},
		{"multi match", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "2"}, true},
		{"multi partial", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "3"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchLabels(tt.cluster, tt.filter); got != tt.wantMatch {
				t.Errorf("matchLabels: got %v, want %v", got, tt.wantMatch)
			}
		})
	}
}

func TestListClusters_StatusFilter(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	cl, _ := svc.Create(ctx, CreateClusterReq{
		Name:       "test",
		APIServer:  "https://x",
		Kubeconfig: validKubeconfig,
	})

	// Mark one as inactive
	_, _ = client.Cluster.UpdateOneID(cl.ID).SetStatus(entcluster.StatusInactive).Save(ctx)

	result, err := svc.List(ctx, ClusterFilter{Page: 1, PageSize: 10, Status: "inactive"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(result.Clusters))
	}
	if result.Clusters[0].Status != "inactive" {
		t.Errorf("Status: got %q, want %q", result.Clusters[0].Status, "inactive")
	}
}
