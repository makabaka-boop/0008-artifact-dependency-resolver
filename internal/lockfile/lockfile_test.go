package lockfile

import (
	"testing"

	"artifact-resolver/internal/resolver"
)

// TestGenerateNoDebugEntry 验证 Generate 返回的 Lockfile.Entries 只包含真实的
// 依赖节点，不含任何 Name 为空、Reason 为 debug-entry 的调试条目，且校验和与
// 条目内容一致（Verify 通过）。
func TestGenerateNoDebugEntry(t *testing.T) {
	nodes := []resolver.Node{
		{Name: "A", Version: "1.0.0", Depth: 0, Reason: "root"},
		{Name: "B", Version: "1.0.0", Depth: 1, Reason: "dep"},
		{Name: "C", Version: "1.0.0", Depth: 2, Reason: "transitive"},
	}
	lf, err := Generate(nodes)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// 返回的条目应与输入节点一一对应，不额外多出任何条目。
	if len(lf.Entries) != len(nodes) {
		t.Fatalf("expected %d entries matching nodes, got %d (debug-entry leaked)", len(nodes), len(lf.Entries))
	}

	// 不得存在 Name 为空、Reason 为 debug-entry 的调试条目。
	for _, e := range lf.Entries {
		if e.Name == "" && e.Reason == "debug-entry" {
			t.Fatal("generated entries must not contain a debug-entry with empty Name")
		}
	}

	// 校验和应与持久化的条目内容一致。
	if !lf.Verify() {
		t.Fatal("expected generated lockfile to pass Verify")
	}
}
