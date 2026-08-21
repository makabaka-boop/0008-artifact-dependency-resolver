package service

import (
	"context"
	"testing"
)

// TestLockfileRefReadsRemainVerified 覆盖 lockfile.Generate → 持久化 → 多次
// lockfile_ref 读取的完整链路。正确行为是：任何一次成功解析产生的锁文件都只
// 包含真实的依赖条目，校验和与内容一致，因此无论读取多少次，Verify 都应通过，
// 且返回的 content 不应因读取而被改写损坏（条目数保持一致）。
func TestLockfileRefReadsRemainVerified(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	// 构造 A→B→C 单层传递依赖：app 依赖 libb，libb 依赖 libc。
	for _, name := range []string{"app", "libb", "libc"} {
		if _, err := s.CreateArtifact(ctx, name, ""); err != nil {
			t.Fatalf("create artifact %s: %v", name, err)
		}
	}
	for _, name := range []string{"app", "libb", "libc"} {
		if _, err := s.CreateVersion(ctx, name, "1.0.0"); err != nil {
			t.Fatalf("create version %s@1.0.0: %v", name, err)
		}
	}
	if _, err := s.ReplaceDependencies(ctx, "app", "1.0.0", []DependencyItem{
		{Name: "libb", Constraint: "^1.0.0"},
	}); err != nil {
		t.Fatalf("replace app deps: %v", err)
	}
	if _, err := s.ReplaceDependencies(ctx, "libb", "1.0.0", []DependencyItem{
		{Name: "libc", Constraint: "^1.0.0"},
	}); err != nil {
		t.Fatalf("replace libb deps: %v", err)
	}
	for _, name := range []string{"libc", "libb", "app"} {
		if _, err := s.PublishVersion(ctx, name, "1.0.0"); err != nil {
			t.Fatalf("publish %s@1.0.0: %v", name, err)
		}
	}

	// 解析得到锁文件引用。
	out, err := s.Resolve(ctx, []ManifestItem{{Name: "app", Constraint: "^1.0.0"}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if out.Status != "succeeded" {
		t.Fatalf("resolve status = %s: %v", out.Status, out.Diagnostics)
	}
	if out.LockfileRef == "" {
		t.Fatal("expected non-empty lockfile ref on successful resolve")
	}
	if len(out.Graph) == 0 {
		t.Fatal("expected at least one non-empty node in dependency graph")
	}
	foundNonEmpty := false
	for _, n := range out.Graph {
		if n.Name != "" {
			foundNonEmpty = true
			break
		}
	}
	if !foundNonEmpty {
		t.Fatal("expected dependency graph to contain a non-empty Name node")
	}

	// 首次通过引用读取：Verify 应通过。
	first, err := s.GetLockfileByRef(ctx, out.LockfileRef)
	if err != nil {
		t.Fatalf("first GetLockfileByRef: %v", err)
	}
	if !first.Verified {
		t.Fatalf("first read: expected verified=true, got false, checksum=%s", first.Lockfile.Checksum)
	}
	firstCount := len(first.Lockfile.Entries)
	if firstCount == 0 {
		t.Fatal("first read: expected non-empty entries")
	}

	// 第二次读取同一引用：内容不应被改写，Verify 仍应通过，条目数应保持一致。
	second, err := s.GetLockfileByRef(ctx, out.LockfileRef)
	if err != nil {
		t.Fatalf("second GetLockfileByRef: %v", err)
	}
	if !second.Verified {
		t.Fatalf("second read: expected verified=true, got false (content corrupted)")
	}
	if len(second.Lockfile.Entries) != firstCount {
		t.Fatalf("second read: expected %d entries matching first read, got %d (content mutated)", firstCount, len(second.Lockfile.Entries))
	}
}
