package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"artifact-resolver/internal/store"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st)
}

func TestCreateArtifactNameConflict(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	if _, err := s.CreateArtifact(ctx, "mylib", "desc"); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := s.CreateArtifact(ctx, "mylib", "desc2")
	if err == nil {
		t.Fatal("expected name conflict")
	}
	if ae, ok := err.(*APIError); !ok || ae.Code != "NAME_CONFLICT" {
		t.Fatalf("expected NAME_CONFLICT, got %v", err)
	}
}

func TestVersionPublishAndDelete(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	if _, err := s.CreateArtifact(ctx, "mylib", "desc"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.CreateVersion(ctx, "mylib", "1.0.0"); err != nil {
		t.Fatalf("create version: %v", err)
	}
	// 重复版本。
	if _, err := s.CreateVersion(ctx, "mylib", "1.0.0"); err == nil {
		t.Fatal("expected version exists")
	}
	// 发布。
	v, err := s.PublishVersion(ctx, "mylib", "1.0.0")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if v.Status != "published" {
		t.Fatalf("expected published, got %s", v.Status)
	}
	// 删除已发布版本应失败。
	if err := s.DeleteVersion(ctx, "mylib", "1.0.0"); err == nil {
		t.Fatal("expected cannot delete published")
	}
}

func TestInvalidSemver(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	if _, err := s.CreateArtifact(ctx, "mylib", "desc"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.CreateVersion(ctx, "mylib", "not-semver"); err == nil {
		t.Fatal("expected invalid version error")
	}
}

func TestDependencyTargetMissing(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	if _, err := s.CreateArtifact(ctx, "mylib", "desc"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.CreateVersion(ctx, "mylib", "1.0.0"); err != nil {
		t.Fatalf("create version: %v", err)
	}
	_, err := s.ReplaceDependencies(ctx, "mylib", "1.0.0", []DependencyItem{{Name: "ghost", Constraint: "^1.0.0"}})
	if err == nil {
		t.Fatal("expected dependency target missing")
	}
	if ae, ok := err.(*APIError); !ok || ae.Code != "DEPENDENCY_TARGET_MISSING" {
		t.Fatalf("expected DEPENDENCY_TARGET_MISSING, got %v", err)
	}
}

func TestResolveFailureDoesNotContaminateNextResolution(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	app, err := s.CreateArtifact(ctx, "app", "application")
	if err != nil {
		t.Fatalf("create app artifact: %v", err)
	}
	appVersion, err := s.CreateVersion(ctx, "app", "1.0.0")
	if err != nil {
		t.Fatalf("create app version: %v", err)
	}
	if _, err := s.PublishVersion(ctx, "app", "1.0.0"); err != nil {
		t.Fatalf("publish app version: %v", err)
	}

	missing, err := s.Resolve(ctx, []ManifestItem{{Name: "ghost", Constraint: "^1.0.0"}})
	if err != nil {
		t.Fatalf("resolve missing artifact manifest: %v", err)
	}
	if missing.Status != "failed" {
		t.Fatalf("first resolution status = %q, want failed", missing.Status)
	}
	if len(missing.Diagnostics) != 1 || missing.Diagnostics[0].Type != "MISSING" || !strings.Contains(missing.Diagnostics[0].Message, "ghost") {
		t.Fatalf("first resolution diagnostics = %#v, want one MISSING diagnostic naming ghost", missing.Diagnostics)
	}

	resolved, err := s.Resolve(ctx, []ManifestItem{{Name: "app", Constraint: "^1.0.0"}})
	if err != nil {
		t.Fatalf("resolve valid app manifest: %v", err)
	}
	if resolved.Status != "succeeded" {
		t.Errorf("second resolution status = %q, want succeeded", resolved.Status)
	}
	if len(resolved.Diagnostics) != 0 {
		t.Errorf("second resolution diagnostics = %#v, want none", resolved.Diagnostics)
	}
	if resolved.LockfileRef == "" {
		t.Error("second resolution lockfile_ref is empty, want a generated reference")
	}
	if len(resolved.Graph) != 1 || resolved.Graph[0].Name != "app" || resolved.Graph[0].Version != "1.0.0" {
		t.Errorf("second resolution graph = %#v, want app@1.0.0 only", resolved.Graph)
	}
	if resolved.RequestID == missing.RequestID {
		t.Errorf("second resolution reused request id %d", resolved.RequestID)
	}

	record, nodes, err := s.GetResolution(ctx, resolved.RequestID)
	if err != nil {
		t.Fatalf("get second resolution %d: %v", resolved.RequestID, err)
	}
	recordJSON, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal second resolution record: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(recordJSON, &fields); err != nil {
		t.Fatalf("decode second resolution record: %v", err)
	}
	stringField := func(name string) string {
		canonical := func(value string) string {
			return strings.ToLower(strings.ReplaceAll(value, "_", ""))
		}
		for key, value := range fields {
			if canonical(key) == canonical(name) {
				text, _ := value.(string)
				return text
			}
		}
		return ""
	}
	if got := stringField("status"); got != "succeeded" {
		t.Errorf("persisted second resolution status = %q, want succeeded", got)
	}
	if got := stringField("error_code"); got != "" {
		t.Errorf("persisted second resolution error_code = %q, want empty", got)
	}
	if got := stringField("error_message"); got != "" {
		t.Errorf("persisted second resolution error_message = %q, want empty", got)
	}
	if encoded := string(recordJSON); strings.Contains(encoded, "ghost") || strings.Contains(encoded, "MISSING") {
		t.Errorf("persisted second resolution contains stale diagnostics: %s", encoded)
	}
	if len(nodes) != 1 {
		t.Errorf("persisted second resolution nodes = %#v, want app@1.0.0", nodes)
	} else if nodes[0].ArtifactID != app.ID || nodes[0].VersionID != appVersion.ID || nodes[0].Depth != 0 {
		t.Errorf("persisted second resolution node = %#v, want app@1.0.0 at depth 0", nodes[0])
	}
}
