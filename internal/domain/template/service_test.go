package template

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent"
	"github.com/whg517/fleet/internal/store/ent/enttest"

	modernsqlite "modernc.org/sqlite"
)

func init() {
	// Register modernc.org/sqlite under the "sqlite3" name that ent expects.
	sql.Register("sqlite3", &sqliteFKDriver{inner: &modernsqlite.Driver{}})
}

// sqliteFKDriver wraps modernc.org/sqlite to enable foreign keys by default.
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

func newTestService(t *testing.T) (*ServiceImpl, *ent.Client) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", fmt.Sprintf("file:%s?mode=memory&_fk=1&_pragma=foreign_keys(1)", t.Name()))
	store := NewEntStore(client)
	svc := NewService(store, zap.NewNop())

	t.Cleanup(func() { _ = client.Close() })

	return svc, client
}

// --- Template CRUD Tests ---

func TestCreateTemplate_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, err := svc.Create(ctx, CreateTemplateReq{
		Name:        "web-deploy",
		Type:        TypeDeployK8s,
		Source:      SourcePlatform,
		Description: "Standard web service Helm chart",
		Repo:        "https://charts.example.com/web",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if tmpl.ID == "" {
		t.Error("expected non-empty ID")
	}
	if tmpl.Name != "web-deploy" {
		t.Errorf("Name: got %q, want %q", tmpl.Name, "web-deploy")
	}
	if tmpl.Type != TypeDeployK8s {
		t.Errorf("Type: got %q, want %q", tmpl.Type, TypeDeployK8s)
	}
	if tmpl.Source != SourcePlatform {
		t.Errorf("Source: got %q, want %q", tmpl.Source, SourcePlatform)
	}
	if tmpl.Status != StatusActive {
		t.Errorf("Status: got %q, want %q", tmpl.Status, StatusActive)
	}
}

func TestCreateTemplate_ValidationErrors(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tests := []struct {
		name string
		req  CreateTemplateReq
	}{
		{"empty name", CreateTemplateReq{Name: "", Type: TypeDeployK8s, Source: SourcePlatform}},
		{"invalid type", CreateTemplateReq{Name: "test", Type: "invalid", Source: SourcePlatform}},
		{"invalid source", CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: "invalid"}},
		{"invalid repo URL", CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform, Repo: "not-a-url"}},
		{"name too long", CreateTemplateReq{Name: string(make([]byte, 129)), Type: TypeDeployK8s, Source: SourcePlatform}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tt.req)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.name)
			}
		})
	}
}

func TestCreateTemplate_DuplicateName(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	// Create an organization first to satisfy FK + unique constraint.
	org, err := client.Organization.Create().
		SetID("org-1").
		SetName("test-org").
		SetSlug("test-org").
		Save(ctx)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	_, err = svc.Create(ctx, CreateTemplateReq{OrgID: org.ID, Name: "dup-template", Type: TypeDeployK8s, Source: SourcePlatform})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = svc.Create(ctx, CreateTemplateReq{OrgID: org.ID, Name: "dup-template", Type: TypeDeployK8s, Source: SourcePlatform})
	if err != ErrTemplateAlreadyExists {
		t.Errorf("Create duplicate: got %v, want ErrTemplateAlreadyExists", err)
	}
}

func TestGetTemplate_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Get(ctx, "nonexistent")
	if err != ErrTemplateNotFound {
		t.Errorf("Get: got %v, want ErrTemplateNotFound", err)
	}
}

func TestGetTemplate_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	created, _ := svc.Create(ctx, CreateTemplateReq{Name: "test-tmpl", Type: TypeBuild, Source: SourcePlatform})

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, created.ID)
	}
	if got.Name != "test-tmpl" {
		t.Errorf("Name mismatch: got %q, want %q", got.Name, "test-tmpl")
	}
	if got.Type != TypeBuild {
		t.Errorf("Type mismatch: got %q, want %q", got.Type, TypeBuild)
	}
}

func TestListTemplates_Pagination(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, CreateTemplateReq{
			Name:   fmt.Sprintf("tmpl-%d", i),
			Type:   TypeDeployK8s,
			Source: SourcePlatform,
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// Page 1, size 2
	result, err := svc.List(ctx, TemplateFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Templates) != 2 {
		t.Errorf("Page 1: got %d templates, want 2", len(result.Templates))
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}

	// Page 3, size 2
	result, err = svc.List(ctx, TemplateFilter{Page: 3, PageSize: 2})
	if err != nil {
		t.Fatalf("List page 3: %v", err)
	}
	if len(result.Templates) != 1 {
		t.Errorf("Page 3: got %d templates, want 1", len(result.Templates))
	}
}

func TestListTemplates_TypeFilter(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateTemplateReq{Name: "a", Type: TypeBuild, Source: SourcePlatform})
	_, _ = svc.Create(ctx, CreateTemplateReq{Name: "b", Type: TypeDeployK8s, Source: SourcePlatform})

	result, err := svc.List(ctx, TemplateFilter{Page: 1, PageSize: 10, Type: string(TypeBuild)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(result.Templates))
	}
	if result.Templates[0].Type != TypeBuild {
		t.Errorf("Type: got %q, want %q", result.Templates[0].Type, TypeBuild)
	}
}

func TestListTemplates_SourceFilter(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateTemplateReq{Name: "a", Type: TypeDeployK8s, Source: SourcePlatform})
	_, _ = svc.Create(ctx, CreateTemplateReq{Name: "b", Type: TypeDeployK8s, Source: SourceExternalOCI})

	result, err := svc.List(ctx, TemplateFilter{Page: 1, PageSize: 10, Source: string(SourceExternalOCI)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(result.Templates))
	}
	if result.Templates[0].Source != SourceExternalOCI {
		t.Errorf("Source: got %q, want %q", result.Templates[0].Source, SourceExternalOCI)
	}
}

func TestListTemplates_NameSearch(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateTemplateReq{Name: "web-chart", Type: TypeDeployK8s, Source: SourcePlatform})
	_, _ = svc.Create(ctx, CreateTemplateReq{Name: "api-chart", Type: TypeDeployK8s, Source: SourcePlatform})

	result, err := svc.List(ctx, TemplateFilter{Page: 1, PageSize: 10, Name: "web"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(result.Templates))
	}
	if result.Templates[0].Name != "web-chart" {
		t.Errorf("Name: got %q, want %q", result.Templates[0].Name, "web-chart")
	}
}

func TestUpdateTemplate_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "orig", Type: TypeDeployK8s, Source: SourcePlatform})

	newName := "updated"
	newDesc := "updated description"
	updated, err := svc.Update(ctx, tmpl.ID, UpdateTemplateReq{Name: &newName, Description: &newDesc})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "updated" {
		t.Errorf("Name: got %q, want %q", updated.Name, "updated")
	}
	if updated.Description != "updated description" {
		t.Errorf("Description: got %q, want %q", updated.Description, "updated description")
	}
}

func TestUpdateTemplate_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	name := "x"
	_, err := svc.Update(ctx, "nonexistent", UpdateTemplateReq{Name: &name})
	if err != ErrTemplateNotFound {
		t.Errorf("Update: got %v, want ErrTemplateNotFound", err)
	}
}

func TestUpdateTemplate_ArchivedRejected(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "to-archive", Type: TypeDeployK8s, Source: SourcePlatform})

	if err := svc.Delete(ctx, tmpl.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	newName := "should-fail"
	_, err := svc.Update(ctx, tmpl.ID, UpdateTemplateReq{Name: &newName})
	if err == nil {
		t.Fatal("expected error updating archived template, got nil")
	}
}

func TestUpdateTemplate_ValidationErrors(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})

	emptyName := ""
	_, err := svc.Update(ctx, tmpl.ID, UpdateTemplateReq{Name: &emptyName})
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}

	badRepo := "not-a-url"
	_, err = svc.Update(ctx, tmpl.ID, UpdateTemplateReq{Repo: &badRepo})
	if err == nil {
		t.Error("expected error for invalid repo URL, got nil")
	}
}

func TestDeleteTemplate_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "to-archive", Type: TypeDeployK8s, Source: SourcePlatform})

	if err := svc.Delete(ctx, tmpl.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := svc.Get(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("Get after archive: %v", err)
	}
	if got.Status != StatusArchived {
		t.Errorf("Status: got %q, want %q", got.Status, StatusArchived)
	}
}

func TestDeleteTemplate_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if err := svc.Delete(ctx, "nonexistent"); err != ErrTemplateNotFound {
		t.Errorf("Delete: got %v, want ErrTemplateNotFound", err)
	}
}

// --- Version Management Tests ---

func TestPublishVersion_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})

	v, err := svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{
		Version:   "1.0.0",
		Changelog: "Initial release",
		Digest:    "sha256:abc123",
	})
	if err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}

	if v.ID == "" {
		t.Error("expected non-empty ID")
	}
	if v.Version != "1.0.0" {
		t.Errorf("Version: got %q, want %q", v.Version, "1.0.0")
	}
	if v.Digest != "sha256:abc123" {
		t.Errorf("Digest: got %q, want %q", v.Digest, "sha256:abc123")
	}
	if v.Status != StatusActive {
		t.Errorf("Status: got %q, want %q", v.Status, StatusActive)
	}
}

func TestPublishVersion_InvalidSemver(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})

	tests := []string{
		"1.0",       // missing patch
		"1.0.0.0",   // too many segments
		"v1.0.0",    // leading v
		"latest",    // not semver
		"1.0.0-",    // incomplete pre-release
		"",          // empty
	}

	for _, v := range tests {
		_, err := svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: v})
		if err == nil {
			t.Errorf("expected validation error for version %q, got nil", v)
		}
	}
}

func TestPublishVersion_Duplicate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})

	_, err := svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("first PublishVersion: %v", err)
	}

	_, err = svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: "1.0.0"})
	if err != ErrVersionAlreadyExists {
		t.Errorf("PublishVersion duplicate: got %v, want ErrVersionAlreadyExists", err)
	}
}

func TestPublishVersion_TemplateNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.PublishVersion(ctx, "nonexistent", PublishVersionReq{Version: "1.0.0"})
	if err != ErrTemplateNotFound {
		t.Errorf("PublishVersion: got %v, want ErrTemplateNotFound", err)
	}
}

func TestPublishVersion_ArchivedTemplate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})
	_ = svc.Delete(ctx, tmpl.ID)

	_, err := svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: "1.0.0"})
	if err == nil {
		t.Fatal("expected error publishing version for archived template, got nil")
	}
}

func TestListVersions_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})

	for _, v := range []string{"1.0.0", "1.1.0", "2.0.0"} {
		_, err := svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: v})
		if err != nil {
			t.Fatalf("PublishVersion %s: %v", v, err)
		}
	}

	result, err := svc.ListVersions(ctx, tmpl.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(result.Versions) != 3 {
		t.Fatalf("got %d versions, want 3", len(result.Versions))
	}
	if result.Total != 3 {
		t.Errorf("Total: got %d, want 3", result.Total)
	}
}

func TestListVersions_Pagination(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})

	for i := 1; i <= 5; i++ {
		_, err := svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{
			Version: fmt.Sprintf("1.0.%d", i),
		})
		if err != nil {
			t.Fatalf("PublishVersion %d: %v", i, err)
		}
	}

	result, err := svc.ListVersions(ctx, tmpl.ID, 1, 2)
	if err != nil {
		t.Fatalf("ListVersions page 1: %v", err)
	}
	if len(result.Versions) != 2 {
		t.Errorf("Page 1: got %d versions, want 2", len(result.Versions))
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}

	result, err = svc.ListVersions(ctx, tmpl.ID, 3, 2)
	if err != nil {
		t.Fatalf("ListVersions page 3: %v", err)
	}
	if len(result.Versions) != 1 {
		t.Errorf("Page 3: got %d versions, want 1", len(result.Versions))
	}
}

func TestArchiveVersion_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})
	_, _ = svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: "1.0.0"})

	if err := svc.ArchiveVersion(ctx, tmpl.ID, "1.0.0"); err != nil {
		t.Fatalf("ArchiveVersion: %v", err)
	}

	// Verify the version is archived
	result, _ := svc.ListVersions(ctx, tmpl.ID, 1, 10)
	if len(result.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(result.Versions))
	}
	if result.Versions[0].Status != StatusArchived {
		t.Errorf("Status: got %q, want %q", result.Versions[0].Status, StatusArchived)
	}
}

func TestArchiveVersion_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})

	err := svc.ArchiveVersion(ctx, tmpl.ID, "999.0.0")
	if err != ErrVersionNotFound {
		t.Errorf("ArchiveVersion: got %v, want ErrVersionNotFound", err)
	}
}

func TestGetWithVersions_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})
	_, _ = svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: "1.0.0"})
	_, _ = svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: "1.1.0"})

	got, versions, err := svc.GetWithVersions(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("GetWithVersions: %v", err)
	}
	if got.ID != tmpl.ID {
		t.Errorf("ID: got %q, want %q", got.ID, tmpl.ID)
	}
	if len(versions) != 2 {
		t.Errorf("Versions: got %d, want 2", len(versions))
	}
}

func TestGetWithVersions_FiltersArchived(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	tmpl, _ := svc.Create(ctx, CreateTemplateReq{Name: "test", Type: TypeDeployK8s, Source: SourcePlatform})
	_, _ = svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: "1.0.0"})
	_, _ = svc.PublishVersion(ctx, tmpl.ID, PublishVersionReq{Version: "2.0.0"})
	_ = svc.ArchiveVersion(ctx, tmpl.ID, "1.0.0")

	_, versions, err := svc.GetWithVersions(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("GetWithVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 active version, got %d", len(versions))
	}
	if versions[0].Version != "2.0.0" {
		t.Errorf("Version: got %q, want %q", versions[0].Version, "2.0.0")
	}
}

// --- Helper Tests ---

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

func TestIsValidSemver(t *testing.T) {
	valid := []string{
		"1.0.0", "0.0.1", "10.20.30", "1.0.0-beta.1", "1.0.0-alpha",
		"1.0.0-rc.1+build.1", "0.1.0", "1.0.0+build",
	}
	invalid := []string{
		"", "1", "1.0", "v1.0.0", "latest", "1.0.0.0",
		"1.0.0-", "01.0.0", "1.00.0",
	}

	for _, v := range valid {
		if !isValidSemver(v) {
			t.Errorf("expected %q to be valid semver", v)
		}
	}
	for _, v := range invalid {
		if isValidSemver(v) {
			t.Errorf("expected %q to be invalid semver", v)
		}
	}
}

func TestIsValidURL(t *testing.T) {
	valid := []string{
		"https://charts.example.com", "http://localhost:8080", "https://github.com/org/repo",
	}
	invalid := []string{
		"", "not-a-url", "ftp://", "://missing-scheme",
	}

	for _, u := range valid {
		if !isValidURL(u) {
			t.Errorf("expected %q to be valid URL", u)
		}
	}
	for _, u := range invalid {
		if isValidURL(u) {
			t.Errorf("expected %q to be invalid URL", u)
		}
	}
}
