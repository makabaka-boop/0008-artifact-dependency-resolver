// Package diff 计算同一制品两个版本之间依赖声明的差异。
package diff

import (
	"sort"

	"artifact-resolver/internal/model"
)

// ChangeKind 描述一条依赖发生的变化类型。
type ChangeKind string

const (
	// Added 表示目标版本新增的依赖。
	Added ChangeKind = "added"
	// Removed 表示目标版本删除的依赖。
	Removed ChangeKind = "removed"
	// Changed 表示同一制品约束发生变化。
	Changed ChangeKind = "changed"
)

// DependencyChange 描述单条依赖差异。
type DependencyChange struct {
	Name string     `json:"name"`
	Kind ChangeKind `json:"kind"`
	From string     `json:"from,omitempty"` // 旧约束，仅 changed/removed 时有意义。
	To   string     `json:"to,omitempty"`   // 新约束，仅 changed/added 时有意义。
}

// Dependencies 对比 from 与 to 两组依赖声明，返回确定性排序的差异列表。
func Dependencies(from, to []model.DependencyTarget) []DependencyChange {
	fromMap := make(map[string]string, len(from))
	for _, d := range from {
		fromMap[d.ToArtifactName] = d.Constraint
	}
	toMap := make(map[string]string, len(to))
	for _, d := range to {
		toMap[d.ToArtifactName] = d.Constraint
	}

	var out []DependencyChange
	for name, cstr := range toMap {
		if prev, ok := fromMap[name]; !ok {
			out = append(out, DependencyChange{Name: name, Kind: Added, To: cstr})
		} else if prev != cstr {
			out = append(out, DependencyChange{Name: name, Kind: Changed, From: prev, To: cstr})
		}
	}
	for name, prev := range fromMap {
		if _, ok := toMap[name]; !ok {
			out = append(out, DependencyChange{Name: name, Kind: Removed, From: prev})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return kindOrder(out[i].Kind) < kindOrder(out[j].Kind)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func kindOrder(k ChangeKind) int {
	switch k {
	case Added:
		return 0
	case Removed:
		return 1
	default:
		return 2
	}
}
