package service

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"artifact-resolver/internal/resolver"
)

func graphNames(graph []resolver.Node) []string {
	names := make([]string, 0, len(graph))
	for _, n := range graph {
		names = append(names, n.Name)
	}
	return names
}

// TestResolveDoesNotLeakGraphAcrossRequests 覆盖：同一服务进程内两次连续 Resolve，
// 第二次解析只应反映本次清单（b），graph、resolution_nodes、锁文件快照均不应残留第一次的 a@1.0.0。
func TestResolveDoesNotLeakGraphAcrossRequests(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	for _, name := range []string{"a", "b"} {
		if _, err := s.CreateArtifact(ctx, name, ""); err != nil {
			t.Fatalf("create artifact %s: %v", name, err)
		}
	}
	mustVersion := func(name, ver string) {
		t.Helper()
		if _, err := s.CreateVersion(ctx, name, ver); err != nil {
			t.Fatalf("create version %s@%s: %v", name, ver, err)
		}
	}
	mustVersion("a", "1.0.0")
	mustVersion("b", "1.0.0")
	for _, p := range [][2]string{{"a", "1.0.0"}, {"b", "1.0.0"}} {
		if _, err := s.PublishVersion(ctx, p[0], p[1]); err != nil {
			t.Fatalf("publish %s@%s: %v", p[0], p[1], err)
		}
	}

	// 第一次解析：只包含 a。
	if _, err := s.Resolve(ctx, []ManifestItem{{Name: "a", Constraint: "^1.0.0"}}); err != nil {
		t.Fatalf("resolve a: %v", err)
	}

	// 第二次解析：只包含 b。
	out, err := s.Resolve(ctx, []ManifestItem{{Name: "b", Constraint: "^1.0.0"}})
	if err != nil {
		t.Fatalf("resolve b: %v", err)
	}

	// 图只应包含 b。
	names := graphNames(out.Graph)
	if len(names) != 1 || names[0] != "b" {
		t.Fatalf("second resolve graph = %v, want exactly [b] (residual a leaked)", names)
	}

	// resolution_nodes 只应包含 b（一条记录）。
	_, nodes, err := s.GetResolution(ctx, out.RequestID)
	if err != nil {
		t.Fatalf("get resolution: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("resolution_nodes count = %d, want 1 (residual a leaked)", len(nodes))
	}

	// 锁文件快照只应包含 b 一个条目。
	lf, err := s.GetLockfileByRequest(ctx, out.RequestID)
	if err != nil {
		t.Fatalf("get lockfile: %v", err)
	}
	if len(lf.Lockfile.Entries) != 1 {
		t.Fatalf("lockfile entries = %d, want 1 (residual a leaked): %+v", len(lf.Lockfile.Entries), lf.Lockfile.Entries)
	}
	if lf.Lockfile.Entries[0].Name != "b" || lf.Lockfile.Entries[0].Version != "1.0.0" {
		t.Fatalf("lockfile entry = %s@%s, want b@1.0.0", lf.Lockfile.Entries[0].Name, lf.Lockfile.Entries[0].Version)
	}
}

// TestPersistResolverStateDoesNotPolluteAudit 覆盖：先解析 a 使引擎累积图非空，
// 再触发 PersistResolverState 的空清单解析，落库的 resolve_state 审计记录不应混入 a。
func TestPersistResolverStateDoesNotPolluteAudit(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	for _, name := range []string{"a", "app"} {
		if _, err := s.CreateArtifact(ctx, name, ""); err != nil {
			t.Fatalf("create artifact %s: %v", name, err)
		}
	}
	if _, err := s.CreateVersion(ctx, "a", "1.0.0"); err != nil {
		t.Fatalf("create version a@1.0.0: %v", err)
	}
	if _, err := s.CreateVersion(ctx, "app", "1.0.0"); err != nil {
		t.Fatalf("create version app@1.0.0: %v", err)
	}
	if _, err := s.PublishVersion(ctx, "a", "1.0.0"); err != nil {
		t.Fatalf("publish a@1.0.0: %v", err)
	}

	// 先解析 a，让引擎累积图含 a。
	if _, err := s.Resolve(ctx, []ManifestItem{{Name: "a", Constraint: "^1.0.0"}}); err != nil {
		t.Fatalf("resolve a: %v", err)
	}

	// ReplaceDependencies 成功后由调用方触发 PersistResolverState。
	if _, err := s.ReplaceDependencies(ctx, "app", "1.0.0", nil); err != nil {
		t.Fatalf("replace dependencies: %v", err)
	}
	if err := s.PersistResolverState(ctx, "app", "1.0.0"); err != nil {
		t.Fatalf("persist resolver state: %v", err)
	}

	_, appV, err := s.getVersion(ctx, "app", "1.0.0")
	if err != nil {
		t.Fatalf("get version app@1.0.0: %v", err)
	}

	changes, err := s.ListChanges(ctx, 100, 0, "version", strconv.FormatInt(appV.ID, 10))
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}

	var stateGraph []resolver.Node
	found := false
	for _, c := range changes {
		if c.Action != "resolve_state" {
			continue
		}
		found = true
		if err := json.Unmarshal([]byte(c.AfterJSON), &stateGraph); err != nil {
			t.Fatalf("unmarshal resolve_state graph: %v", err)
		}
	}
	if !found {
		t.Fatal("expected resolve_state audit record")
	}
	if names := graphNames(stateGraph); len(names) != 0 {
		t.Fatalf("resolve_state audit graph = %v, want empty (residual a leaked into version audit)", names)
	}
}
