package approval

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent"
	entapproval "github.com/whg517/fleet/internal/store/ent/approval"
	"github.com/whg517/fleet/internal/store/ent/enttest"
	enttemplateversion "github.com/whg517/fleet/internal/store/ent/templateversion"

	modernsqlite "modernc.org/sqlite"
)

func init() {
	sql.Register("sqlite3", &sqliteFKDriver{inner: &modernsqlite.Driver{}})
}

type sqliteFKDriver struct {
	inner driver.Driver
}

func (d *sqliteFKDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	if execCtx, ok := conn.(driver.ExecerContext); ok {
		_, _ = execCtx.ExecContext(context.Background(), "PRAGMA foreign_keys = ON", nil)
	}
	return conn, nil
}

// mockTrigger is a mock implementation of DeploymentTrigger for testing.
type mockTrigger struct {
	calls    []string
	failWith error
}

func (m *mockTrigger) TriggerDeployment(_ context.Context, deploymentID string) error {
	m.calls = append(m.calls, deploymentID)
	return m.failWith
}

// newTestService creates a test approval service with SQLite in-memory DB.
func newTestService(t *testing.T) (*ServiceImpl, *ent.Client, *mockTrigger) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", fmt.Sprintf("file:%s?mode=memory&_fk=1&_pragma=foreign_keys(1)", t.Name()))
	store := NewEntStore(client)
	lookup := NewLookupEntStore(client)
	trigger := &mockTrigger{}
	svc := NewService(store, lookup, trigger, zap.NewNop())

	t.Cleanup(func() { _ = client.Close() })

	return svc, client, trigger
}

// seedPrerequisites creates a Service, Environment, Cluster, Template, TemplateVersion,
// Organization, and a Deployment so that approval operations have valid references.
func seedPrerequisites(t *testing.T, client *ent.Client, approvalRequired bool) (svcID, envID, deploymentID string) {
	t.Helper()
	ctx := context.Background()

	client.Organization.Create().
		SetID("org-1").
		SetName("test-org").
		SetSlug("test-org").
		SaveX(ctx)

	cl := client.Cluster.Create().
		SetID("cluster-1").
		SetName("test-cluster").
		SetAPIServer("https://10.0.0.1:6443").
		SaveX(ctx)

	svc := client.Service.Create().
		SetID("svc-1").
		SetName("payment-api").
		SaveX(ctx)

	env := client.Environment.Create().
		SetID("env-1").
		SetName("pre").
		SetClusterID(cl.ID).
		SetNamespacePattern("default").
		SetApprovalRequired(approvalRequired).
		SaveX(ctx)

	tmpl := client.Template.Create().
		SetID("tmpl-1").
		SetName("deploy-template").
		SetType("deploy_k8s").
		SetSource("platform").
		SetRepo("https://charts.example.com").
		SaveX(ctx)

	tv := client.TemplateVersion.Create().
		SetID("tv-1").
		SetTemplateID(tmpl.ID).
		SetVersion("1.0.0").
		SetStatus(enttemplateversion.StatusActive).
		SaveX(ctx)

	dep := client.Deployment.Create().
		SetID("dep-1").
		SetServiceID(svc.ID).
		SetEnvironmentID(env.ID).
		SetClusterID(cl.ID).
		SetTemplateVersionID(tv.ID).
		SetVersion("v1.0.0").
		SetStatus("pending").
		SaveX(ctx)

	return svc.ID, env.ID, dep.ID
}

// --- Tests ---

func TestRequestApproval_Success(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	_, envID, depID := seedPrerequisites(t, client, true)

	a, err := svc.RequestApproval(ctx, depID, RequestApprovalReq{
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	if a.ID == "" {
		t.Error("expected non-empty ID")
	}
	if a.DeploymentID != depID {
		t.Errorf("DeploymentID: got %q, want %q", a.DeploymentID, depID)
	}
	if a.Status != StatusPending {
		t.Errorf("Status: got %q, want %q", a.Status, StatusPending)
	}
	if a.RequesterID != "user-1" {
		t.Errorf("RequesterID: got %q, want %q", a.RequesterID, "user-1")
	}
	// Timeout should be ~24h from now
	if a.TimeoutAt.Before(time.Now().Add(23 * time.Hour)) {
		t.Errorf("TimeoutAt too early: got %v", a.TimeoutAt)
	}
	_ = envID
}

func TestRequestApproval_DeploymentNotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.RequestApproval(ctx, "nonexistent-dep", RequestApprovalReq{
		RequesterID: "user-1",
	})
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Errorf("expected ErrDeploymentNotFound, got %v", err)
	}
}

func TestRequestApproval_EmptyDeploymentID(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.RequestApproval(ctx, "", RequestApprovalReq{
		RequesterID: "user-1",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestApprove_Success(t *testing.T) {
	svc, client, trigger := newTestService(t)
	ctx := context.Background()

	_, _, depID := seedPrerequisites(t, client, true)

	// Create an approval first
	_, err := svc.RequestApproval(ctx, depID, RequestApprovalReq{
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	// Approve it
	a, err := svc.Approve(ctx, depID, ApproveReq{
		ApproverID: "admin-1",
		Comment:    "looks good",
	})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}

	if a.Status != StatusApproved {
		t.Errorf("Status: got %q, want %q", a.Status, StatusApproved)
	}
	if a.ApproverID != "admin-1" {
		t.Errorf("ApproverID: got %q, want %q", a.ApproverID, "admin-1")
	}
	if a.Comment != "looks good" {
		t.Errorf("Comment: got %q, want %q", a.Comment, "looks good")
	}
	if a.DecidedAt == nil {
		t.Error("expected non-nil DecidedAt")
	}

	// Verify trigger was called
	if len(trigger.calls) != 1 {
		t.Fatalf("expected 1 trigger call, got %d", len(trigger.calls))
	}
	if trigger.calls[0] != depID {
		t.Errorf("trigger deploymentID: got %q, want %q", trigger.calls[0], depID)
	}
}

func TestApprove_NotPending(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	_, _, depID := seedPrerequisites(t, client, true)

	// Create and approve
	_, err := svc.RequestApproval(ctx, depID, RequestApprovalReq{
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	_, err = svc.Approve(ctx, depID, ApproveReq{
		ApproverID: "admin-1",
	})
	if err != nil {
		t.Fatalf("Approve first: %v", err)
	}

	// Try to approve again
	_, err = svc.Approve(ctx, depID, ApproveReq{
		ApproverID: "admin-2",
	})
	if !errors.Is(err, ErrApprovalNotPending) {
		t.Errorf("expected ErrApprovalNotPending, got %v", err)
	}
}

func TestApprove_ApprovalNotFound(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	seedPrerequisites(t, client, true)

	// No approval created — try to approve directly
	_, err := svc.Approve(ctx, "dep-1", ApproveReq{
		ApproverID: "admin-1",
	})
	if !errors.Is(err, ErrApprovalNotFound) {
		t.Errorf("expected ErrApprovalNotFound, got %v", err)
	}
}

func TestReject_Success(t *testing.T) {
	svc, client, trigger := newTestService(t)
	ctx := context.Background()

	_, _, depID := seedPrerequisites(t, client, true)

	// Create an approval
	_, err := svc.RequestApproval(ctx, depID, RequestApprovalReq{
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	// Reject it
	a, err := svc.Reject(ctx, depID, RejectReq{
		ApproverID: "admin-1",
		Comment:    "not ready",
	})
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}

	if a.Status != StatusRejected {
		t.Errorf("Status: got %q, want %q", a.Status, StatusRejected)
	}
	if a.ApproverID != "admin-1" {
		t.Errorf("ApproverID: got %q, want %q", a.ApproverID, "admin-1")
	}
	if a.Comment != "not ready" {
		t.Errorf("Comment: got %q, want %q", a.Comment, "not ready")
	}
	if a.DecidedAt == nil {
		t.Error("expected non-nil DecidedAt")
	}

	// Verify trigger was NOT called
	if len(trigger.calls) != 0 {
		t.Fatalf("expected 0 trigger calls, got %d", len(trigger.calls))
	}
}

func TestReject_NotPending(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	_, _, depID := seedPrerequisites(t, client, true)

	_, err := svc.RequestApproval(ctx, depID, RequestApprovalReq{
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	_, err = svc.Reject(ctx, depID, RejectReq{
		ApproverID: "admin-1",
	})
	if err != nil {
		t.Fatalf("Reject first: %v", err)
	}

	// Try to reject again
	_, err = svc.Reject(ctx, depID, RejectReq{
		ApproverID: "admin-2",
	})
	if !errors.Is(err, ErrApprovalNotPending) {
		t.Errorf("expected ErrApprovalNotPending, got %v", err)
	}
}

func TestGet_Success(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	_, _, depID := seedPrerequisites(t, client, true)

	created, err := svc.RequestApproval(ctx, depID, RequestApprovalReq{
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	got, err := svc.Get(ctx, depID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: got %q, want %q", got.ID, created.ID)
	}
}

func TestGet_NotFound(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	seedPrerequisites(t, client, true)

	_, err := svc.Get(ctx, "dep-1")
	if !errors.Is(err, ErrApprovalNotFound) {
		t.Errorf("expected ErrApprovalNotFound, got %v", err)
	}
}

func TestList_Pagination(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client, true)

	// Create multiple deployments + approvals
	for i := 0; i < 3; i++ {
		dep := client.Deployment.Create().
			SetID(fmt.Sprintf("dep-list-%d", i)).
			SetServiceID(svcID).
			SetEnvironmentID(envID).
			SetClusterID("cluster-1").
			SetTemplateVersionID("tv-1").
			SetVersion(fmt.Sprintf("v1.%d.0", i)).
			SetStatus("pending").
			SaveX(ctx)

		_, err := svc.RequestApproval(ctx, dep.ID, RequestApprovalReq{
			RequesterID: "user-1",
		})
		if err != nil {
			t.Fatalf("RequestApproval %d: %v", i, err)
		}
	}

	// Page 1, size 2
	result, err := svc.List(ctx, ApprovalFilter{
		Page:     1,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Approvals) != 2 {
		t.Errorf("Page 1: got %d approvals, want 2", len(result.Approvals))
	}
	if result.Total != 3 {
		t.Errorf("Total: got %d, want 3", result.Total)
	}

	// Page 2, size 2
	result, err = svc.List(ctx, ApprovalFilter{
		Page:     2,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(result.Approvals) != 1 {
		t.Errorf("Page 2: got %d approvals, want 1", len(result.Approvals))
	}
}

func TestList_FilterByStatus(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client, true)

	// Create one pending and one approved
	dep1 := client.Deployment.Create().
		SetID("dep-f1").
		SetServiceID(svcID).
		SetEnvironmentID(envID).
		SetClusterID("cluster-1").
		SetTemplateVersionID("tv-1").
		SetVersion("v1.0.0").
		SetStatus("pending").
		SaveX(ctx)

	_, err := svc.RequestApproval(ctx, dep1.ID, RequestApprovalReq{
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("RequestApproval 1: %v", err)
	}

	dep2 := client.Deployment.Create().
		SetID("dep-f2").
		SetServiceID(svcID).
		SetEnvironmentID(envID).
		SetClusterID("cluster-1").
		SetTemplateVersionID("tv-1").
		SetVersion("v2.0.0").
		SetStatus("pending").
		SaveX(ctx)

	_, err = svc.RequestApproval(ctx, dep2.ID, RequestApprovalReq{
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("RequestApproval 2: %v", err)
	}

	// Approve the second one
	_, err = svc.Approve(ctx, dep2.ID, ApproveReq{
		ApproverID: "admin-1",
	})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Filter by pending
	result, err := svc.List(ctx, ApprovalFilter{
		Status:   "pending",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("List pending: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Pending total: got %d, want 1", result.Total)
	}

	// Filter by approved
	result, err = svc.List(ctx, ApprovalFilter{
		Status:   "approved",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("List approved: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Approved total: got %d, want 1", result.Total)
	}
}

func TestCheckTimeouts_ExpiredApprovals(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client, true)

	// Create an approval with a past timeout
	dep := client.Deployment.Create().
		SetID("dep-timeout").
		SetServiceID(svcID).
		SetEnvironmentID(envID).
		SetClusterID("cluster-1").
		SetTemplateVersionID("tv-1").
		SetVersion("v1.0.0").
		SetStatus("pending").
		SaveX(ctx)

	// Create approval manually with expired timeout
	pastTime := time.Now().Add(-1 * time.Hour)
	client.Approval.Create().
		SetID("appr-expired").
		SetDeploymentID(dep.ID).
		SetServiceID(svcID).
		SetEnvironmentID(envID).
		SetRequesterID("user-1").
		SetStatus(entapproval.StatusPending).
		SetTimeoutAt(pastTime).
		SaveX(ctx)

	// Also create a non-expired one
	dep2 := client.Deployment.Create().
		SetID("dep-notimeout").
		SetServiceID(svcID).
		SetEnvironmentID(envID).
		SetClusterID("cluster-1").
		SetTemplateVersionID("tv-1").
		SetVersion("v2.0.0").
		SetStatus("pending").
		SaveX(ctx)

	_, err := svc.RequestApproval(ctx, dep2.ID, RequestApprovalReq{
		RequesterID: "user-1",
	})
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}

	// Run CheckTimeouts
	count, err := svc.CheckTimeouts(ctx)
	if err != nil {
		t.Fatalf("CheckTimeouts: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 timeout, got %d", count)
	}

	// Verify the expired approval is now timed out
	a, err := svc.Get(ctx, dep.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if a.Status != StatusTimeout {
		t.Errorf("Status: got %q, want %q", a.Status, StatusTimeout)
	}
	if a.DecidedAt == nil {
		t.Error("expected non-nil DecidedAt after timeout")
	}

	// Verify the non-expired one is still pending
	a2, err := svc.Get(ctx, dep2.ID)
	if err != nil {
		t.Fatalf("Get 2: %v", err)
	}
	if a2.Status != StatusPending {
		t.Errorf("Status: got %q, want %q", a2.Status, StatusPending)
	}
}

func TestCheckTimeouts_NoExpiredApprovals(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	count, err := svc.CheckTimeouts(ctx)
	if err != nil {
		t.Fatalf("CheckTimeouts: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 timeouts, got %d", count)
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
