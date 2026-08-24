package service

import (
	"context"
	"path/filepath"
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
