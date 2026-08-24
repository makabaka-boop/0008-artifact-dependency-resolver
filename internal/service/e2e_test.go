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

// TestResolveIndependentArtifacts verifies that every top-level manifest item is
// resolved and persisted when the items have no dependencies on one another.
func TestResolveIndependentArtifacts(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "independent.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	s := New(st)
	ctx := context.Background()

	for _, name := range []string{"alpha", "beta"} {
		if _, err := s.CreateArtifact(ctx, name, ""); err != nil {
			t.Fatalf("create artifact %s: %v", name, err)
		}
		if _, err := s.CreateVersion(ctx, name, "1.0.0"); err != nil {
			t.Fatalf("create version %s@1.0.0: %v", name, err)
		}
		if _, err := s.PublishVersion(ctx, name, "1.0.0"); err != nil {
			t.Fatalf("publish %s@1.0.0: %v", name, err)
		}
	}

	out, err := s.Resolve(ctx, []ManifestItem{
		{Name: "alpha", Constraint: "^1.0.0"},
		{Name: "beta", Constraint: "^1.0.0"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if out.Status != "succeeded" {
		t.Fatalf("resolve status = %s, diagnostics = %v", out.Status, out.Diagnostics)
	}
	if out.LockfileRef == "" {
		t.Fatal("resolve did not return a lockfile reference")
	}
	if len(out.Graph) != 2 {
		t.Fatalf("resolve graph has %d nodes, want alpha and beta", len(out.Graph))
	}
	graphNames := map[string]bool{}
	for _, node := range out.Graph {
		graphNames[node.Name] = true
		if node.Version != "1.0.0" {
			t.Errorf("graph node %s resolved to %s, want 1.0.0", node.Name, node.Version)
		}
	}
	for _, name := range []string{"alpha", "beta"} {
		if !graphNames[name] {
			t.Errorf("resolve graph is missing %s", name)
		}
	}

	req, nodes, err := s.GetResolution(ctx, out.RequestID)
	if err != nil {
		t.Fatalf("get resolution: %v", err)
	}
	if req.Status != "succeeded" {
		t.Fatalf("history status = %s", req.Status)
	}
	if len(nodes) != 2 {
		t.Fatalf("history has %d nodes, want alpha and beta", len(nodes))
	}
	alpha, err := st.GetArtifactByName("alpha")
	if err != nil {
		t.Fatalf("lookup alpha: %v", err)
	}
	beta, err := st.GetArtifactByName("beta")
	if err != nil {
		t.Fatalf("lookup beta: %v", err)
	}
	nodeArtifacts := map[int64]bool{}
	for _, node := range nodes {
		nodeArtifacts[node.ArtifactID] = true
		if node.VersionID == 0 {
			t.Errorf("history node for artifact %d has no version", node.ArtifactID)
		}
	}
	if !nodeArtifacts[alpha.ID] || !nodeArtifacts[beta.ID] {
		t.Fatalf("history nodes contain artifact IDs %v, want alpha=%d and beta=%d", nodeArtifacts, alpha.ID, beta.ID)
	}
}
