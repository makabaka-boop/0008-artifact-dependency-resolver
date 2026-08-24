package service

import (
	"context"
	"path/filepath"
	"testing"

	"artifact-resolver/internal/store"
)

func TestExactVersionResolutionTerminalStateRemainsSucceededAcrossRerun(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "exact-version-rerun.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	s := New(st)
	ctx := context.Background()
	if _, err := s.CreateArtifact(ctx, "exact-app", ""); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if _, err := s.CreateVersion(ctx, "exact-app", "1.2.3"); err != nil {
		t.Fatalf("create version: %v", err)
	}
	if _, err := s.PublishVersion(ctx, "exact-app", "1.2.3"); err != nil {
		t.Fatalf("publish version: %v", err)
	}

	manifest := []ManifestItem{{Name: "exact-app", Constraint: "1.2.3"}}
	first, err := s.Resolve(ctx, manifest)
	if err != nil {
		t.Fatalf("resolve exact version: %v", err)
	}
	assertSuccessfulResolveOutput(t, "initial resolve", first)
	assertPersistedSuccessfulResolution(t, ctx, s, "initial resolution record", first.RequestID)

	rerun, err := s.RerunResolution(ctx, first.RequestID)
	if err != nil {
		t.Fatalf("rerun resolution %d: %v", first.RequestID, err)
	}
	assertSuccessfulResolveOutput(t, "rerun", rerun)
	assertPersistedSuccessfulResolution(t, ctx, s, "original record after rerun", first.RequestID)
	assertPersistedSuccessfulResolution(t, ctx, s, "rerun record", rerun.RequestID)
}

func assertSuccessfulResolveOutput(t *testing.T, stage string, out ResolveOutput) {
	t.Helper()
	if out.Status != "succeeded" {
		t.Errorf("%s status = %q, want succeeded; diagnostics = %#v", stage, out.Status, out.Diagnostics)
	}
	if out.RequestID <= 0 {
		t.Errorf("%s request ID = %d, want a positive ID", stage, out.RequestID)
	}
	if out.LockfileRef == "" {
		t.Errorf("%s lockfile reference is empty", stage)
	}
	if len(out.Graph) == 0 {
		t.Errorf("%s graph is empty", stage)
	}
}

func assertPersistedSuccessfulResolution(t *testing.T, ctx context.Context, s *Service, stage string, id int64) {
	t.Helper()
	req, nodes, err := s.GetResolution(ctx, id)
	if err != nil {
		t.Errorf("%s: get resolution %d: %v", stage, id, err)
		return
	}
	if req.Status != "succeeded" {
		t.Errorf("%s status = %q, want succeeded", stage, req.Status)
	}
	if req.ErrorCode != "" {
		t.Errorf("%s error code = %q, want empty", stage, req.ErrorCode)
	}
	if req.ErrorMessage != "" {
		t.Errorf("%s error message = %q, want empty", stage, req.ErrorMessage)
	}
	if req.GraphJSON == "" {
		t.Errorf("%s stored graph is empty", stage)
	}
	if len(nodes) == 0 {
		t.Errorf("%s resolution nodes are empty", stage)
	}
}

// TestEndToEnd 覆盖创建→发布→声明依赖→resolve→历史→compare 全流程。
func TestEndToEnd(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	s := New(st)
	ctx := context.Background()

	// 创建制品。
	for _, name := range []string{"app", "liba", "libb"} {
		if _, err := s.CreateArtifact(ctx, name, ""); err != nil {
			t.Fatalf("create artifact %s: %v", name, err)
		}
	}
	// 创建版本（draft）并声明依赖，然后发布。
	mustVersion := func(name, ver string) {
		t.Helper()
		if _, err := s.CreateVersion(ctx, name, ver); err != nil {
			t.Fatalf("create version %s@%s: %v", name, ver, err)
		}
	}
	mustVersion("liba", "1.0.0")
	mustVersion("libb", "1.0.0")
	mustVersion("app", "1.0.0")

	// 声明依赖（draft 阶段）。
	if _, err := s.ReplaceDependencies(ctx, "app", "1.0.0", []DependencyItem{
		{Name: "liba", Constraint: "^1.0.0"},
		{Name: "libb", Constraint: "^1.0.0"},
	}); err != nil {
		t.Fatalf("replace deps: %v", err)
	}

	// 发布全部版本。
	for _, p := range [][2]string{{"liba", "1.0.0"}, {"libb", "1.0.0"}, {"app", "1.0.0"}} {
		if _, err := s.PublishVersion(ctx, p[0], p[1]); err != nil {
			t.Fatalf("publish %s@%s: %v", p[0], p[1], err)
		}
	}

	// resolve。
	out, err := s.Resolve(ctx, []ManifestItem{{Name: "app", Constraint: "^1.0.0"}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if out.Status != "succeeded" {
		t.Fatalf("resolve status = %s: %v", out.Status, out.Diagnostics)
	}

	// 历史查询。
	req, nodes, err := s.GetResolution(ctx, out.RequestID)
	if err != nil {
		t.Fatalf("get resolution: %v", err)
	}
	if req.Status != "succeeded" {
		t.Fatalf("history status = %s", req.Status)
	}
	if len(nodes) == 0 {
		t.Fatal("expected resolution nodes")
	}

	// compare。
	c, err := s.Compare(ctx, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if c != -1 {
		t.Fatalf("compare(1.2.3,1.2.4) = %d, want -1", c)
	}
}
