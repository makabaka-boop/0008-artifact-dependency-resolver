package service

import (
	"context"
	"path/filepath"
	"testing"

	"artifact-resolver/internal/store"
)

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
