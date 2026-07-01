package runtime

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeProjectStorageLifecycleAndProjection(t *testing.T) {
	root := runtimeDevTestRoot(t, "project-storage-lifecycle")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")
	projectPath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}

	service := newRuntimeService()
	opened, err := service.OpenProject(context.Background(), RuntimeOpenProjectRequest{Path: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Project.ID == "" || opened.Project.Path != projectPath || !opened.Project.Current {
		t.Fatalf("opened project = %#v", opened.Project)
	}
	firstID := opened.Project.ID

	reopened, err := service.OpenProject(context.Background(), RuntimeOpenProjectRequest{Path: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Project.ID != firstID {
		t.Fatalf("same active path created new project id: %s != %s", reopened.Project.ID, firstID)
	}

	projectSession, err := service.CreateSession(context.Background(), RuntimeSessionCreateRequest{
		Title:     "Project chat",
		Scope:     "project",
		ProjectID: firstID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projectSession.Session.ProjectID != firstID || projectSession.Session.Scope != "project" {
		t.Fatalf("project session = %#v", projectSession.Session)
	}
	standalone, err := service.CreateSession(context.Background(), RuntimeSessionCreateRequest{
		Title: "Standalone chat",
		Scope: "standalone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if standalone.Session.ProjectID != "" || standalone.Session.Scope != "standalone" {
		t.Fatalf("standalone session = %#v", standalone.Session)
	}

	beforeDraft, err := service.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.NewChat(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	afterDraft, err := service.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(afterDraft.Sessions) != len(beforeDraft.Sessions) {
		t.Fatalf("draft NewChat created a persisted session: before=%d after=%d", len(beforeDraft.Sessions), len(afterDraft.Sessions))
	}

	projection, err := service.SidebarProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if projection.CurrentProjectID != firstID || len(projection.Projects) != 1 {
		t.Fatalf("projection projects = %#v current=%q", projection.Projects, projection.CurrentProjectID)
	}
	if len(projection.Sessions) != 2 {
		t.Fatalf("projection sessions = %#v", projection.Sessions)
	}
}

func TestRuntimeProjectSoftDeleteAllowsSamePathNewID(t *testing.T) {
	root := runtimeDevTestRoot(t, "project-soft-delete")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")
	projectPath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}

	service := newRuntimeService()
	opened, err := service.OpenProject(context.Background(), RuntimeOpenProjectRequest{Path: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	store, err := service.projectStore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(context.Background(), `UPDATE runtime_settings SET value = 'soft' WHERE key = 'delete_mode'`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RemoveProject(context.Background(), RuntimeProjectActionRequest{ProjectID: opened.Project.ID}); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.Get(context.Background(), opened.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.DeletedAt == 0 {
		t.Fatalf("soft delete did not set deleted_at: %#v", deleted)
	}
	reopened, err := service.OpenProject(context.Background(), RuntimeOpenProjectRequest{Path: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Project.ID == opened.Project.ID {
		t.Fatalf("soft-deleted same path reused old project id %s", reopened.Project.ID)
	}
}

func TestRuntimeProjectHardDeletePurgesDBOnly(t *testing.T) {
	root := runtimeDevTestRoot(t, "project-hard-delete")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)
	writeRuntimeDevModelConfig(t, root, "http://127.0.0.1:1")
	projectPath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(projectPath, "keep.txt")
	if err := os.WriteFile(userFile, []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := newRuntimeService()
	opened, err := service.OpenProject(context.Background(), RuntimeOpenProjectRequest{Path: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	sessionResp, err := service.CreateSession(context.Background(), RuntimeSessionCreateRequest{
		Title:     "Project chat",
		Scope:     "project",
		ProjectID: opened.Project.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := service.projectStore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RemoveProject(context.Background(), RuntimeProjectActionRequest{ProjectID: opened.Project.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Fatalf("hard delete touched user project file: %v", err)
	}
	if _, err := store.Get(context.Background(), opened.Project.ID); err == nil {
		t.Fatal("hard-deleted project record still exists")
	}
	var count int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sessions WHERE id = ?`, sessionResp.Session.ID).Scan(&count); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("hard-deleted session still exists: count=%d", count)
	}
}
