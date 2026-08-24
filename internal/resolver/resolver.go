package resolver

import (
	"artifact-resolver/internal/model"
)

// ManifestItem 是解析清单中的一项。
type ManifestItem struct {
	Name       string
	Constraint string
}

// Diagnostic 描述一次解析失败的原因。
type Diagnostic struct {
	Type    string `json:"type"` // CYCLE | CONFLICT | MISSING
	Message string `json:"message"`
	Details string `json:"details"`
	// Candidates 为诊断明细增强：列出一一被淘汰的候选版本及逐条原因。
	Candidates []CandidateElimination `json:"candidates,omitempty"`
}

// CandidateElimination 记录某个候选版本为何未被选中。
type CandidateElimination struct {
	Version string `json:"version"`
	Reason  string `json:"reason"`
}

// Node 是解析结果图中的一个节点。
type Node struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Depth   int    `json:"depth"`
	Reason  string `json:"reason"`
}

// Result 是解析器输出。
type Result struct {
	Status      string       `json:"status"` // succeeded | failed
	Graph       []Node       `json:"graph"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Nodes       []model.ResolutionNode
}

// Catalog 提供给解析器的版本与依赖数据视图。
type Catalog struct {
	// PublishedVersions 返回某制品所有可参与常规解析的已发布且未废弃版本。
	PublishedVersions func(artifactID int64) ([]model.Version, error)
	// AllPublishedVersions 返回某制品所有已发布版本（含 deprecated），用于显式 pin。
	AllPublishedVersions func(artifactID int64) ([]model.Version, error)
	// ArtifactByName 返回制品。
	ArtifactByName func(name string) (model.Artifact, error)
	// DependenciesFor 返回某版本的依赖。
	DependenciesFor func(versionID int64) ([]model.DependencyTarget, error)
}
