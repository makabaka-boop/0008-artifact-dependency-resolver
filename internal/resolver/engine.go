package resolver

import (
	"context"
	"fmt"
	"sort"

	"artifact-resolver/internal/constraint"
	"artifact-resolver/internal/model"
	"artifact-resolver/internal/semver"
)

// Engine 执行依赖解析。
type Engine struct {
	cat Catalog
}

// New 创建解析引擎。
func New(cat Catalog) *Engine { return &Engine{cat: cat} }

type selectedVersion struct {
	version    model.Version
	artifactID int64
}

// ResolveContext 解析依赖清单，返回结果图或诊断。
func (e *Engine) ResolveContext(ctx context.Context, manifest []ManifestItem) Result {
	selected := map[string]selectedVersion{}
	explicit := map[string]string{}
	var graph []Node
	var nodes []model.ResolutionNode
	var diags []Diagnostic
	// 用于环检测的递归栈。
	var stack []string
	inStack := map[string]bool{}

	// 为每个清单项建立显式 pin 映射（精确版本 = 视为显式 pin）。
	for _, item := range manifest {
		explicit[item.Name] = item.Constraint
	}

	var resolveArtifact func(name, cstr string, depth int, parent string)
	resolveArtifact = func(name, cstr string, depth int, parent string) {
		if ctx.Err() != nil {
			return
		}
		if inStack[name] {
			// 检出循环。
			cycle := append([]string{}, stack[1:]...)
			cycle = append(cycle, name)
			diags = append(diags, Diagnostic{
				Type:    "CYCLE",
				Message: fmt.Sprintf("dependency cycle detected: %s", joinPath(cycle)),
				Details: joinPath(cycle),
			})
			return
		}
		if _, ok := selected[name]; ok {
			return // 已选定，跳过。
		}

		art, err := e.artifactByName(ctx, name)
		if err != nil {
			diags = append(diags, Diagnostic{
				Type:    "MISSING",
				Message: fmt.Sprintf("artifact %q not found", name),
				Details: fmt.Sprintf("required by %s", parentRef(parent)),
			})
			return
		}

		cst, err := constraint.Parse(cstr)
		if err != nil {
			diags = append(diags, Diagnostic{
				Type:    "MISSING",
				Message: fmt.Sprintf("invalid constraint %q for %q", cstr, name),
				Details: err.Error(),
			})
			return
		}

		isExplicit := explicit[name] != ""
		candidates, err := e.candidateVersionsContext(ctx, art, isExplicit)
		if err != nil {
			diags = append(diags, Diagnostic{Type: "MISSING", Message: err.Error(), Details: name})
			return
		}

		chosen, eliminations := e.selectVersion(candidates, cst, isExplicit)
		if chosen == nil {
			diags = append(diags, Diagnostic{
				Type:       "MISSING",
				Message:    fmt.Sprintf("no version of %q satisfies %q", name, cstr),
				Details:    fmt.Sprintf("candidates: %d", len(candidates)),
				Candidates: eliminations,
			})
			return
		}

		selected[name] = selectedVersion{version: *chosen, artifactID: art.ID}
		reason := fmt.Sprintf("selected for constraint %q", cstr)
		if parent != "" {
			reason = fmt.Sprintf("transitively required by %s with %q", parent, cstr)
		}
		graph = append(graph, Node{Name: name, Version: chosen.Version, Depth: depth, Reason: reason})
		nodes = append(nodes, model.ResolutionNode{
			ArtifactID: art.ID, VersionID: chosen.ID, Depth: depth, Reason: reason,
		})

		// 递归展开传递依赖。
		stack = append(stack, name)
		inStack[name] = true
		deps, _ := e.dependenciesFor(ctx, chosen.ID)
		for _, d := range deps {
			resolveArtifact(d.ToArtifactName, d.Constraint, depth+1, name)
		}
		inStack[name] = false
		stack = stack[:len(stack)-1]
	}

	// 顶层按清单顺序解析。
	for _, item := range manifest {
		resolveArtifact(item.Name, item.Constraint, 0, "")
	}

	// 冲突检测：同一制品在清单中重复出现不同约束。
	diags = append(diags, e.detectConflicts(manifest)...)

	sort.SliceStable(graph, func(i, j int) bool {
		if graph[i].Depth != graph[j].Depth {
			return graph[i].Depth < graph[j].Depth
		}
		return graph[i].Name < graph[j].Name
	})
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].Depth < nodes[j].Depth })

	if len(diags) > 0 {
		return Result{Status: "failed", Graph: graph, Diagnostics: diags, Nodes: nodes}
	}
	return Result{Status: "succeeded", Graph: graph, Diagnostics: diags, Nodes: nodes}
}

func (e *Engine) artifactByName(ctx context.Context, name string) (model.Artifact, error) {
	if e.cat.ArtifactByNameContext != nil {
		return e.cat.ArtifactByNameContext(ctx, name)
	}
	return e.cat.ArtifactByName(name)
}

func (e *Engine) candidateVersionsContext(ctx context.Context, art model.Artifact, explicit bool) ([]model.Version, error) {
	if explicit && e.cat.AllPublishedVersionsContext != nil {
		return e.cat.AllPublishedVersionsContext(ctx, art.ID)
	}
	if !explicit && e.cat.PublishedVersionsContext != nil {
		return e.cat.PublishedVersionsContext(ctx, art.ID)
	}
	return e.candidateVersions(art, explicit)
}

func (e *Engine) dependenciesFor(ctx context.Context, versionID int64) ([]model.DependencyTarget, error) {
	if e.cat.DependenciesForContext != nil {
		return e.cat.DependenciesForContext(ctx, versionID)
	}
	return e.cat.DependenciesFor(versionID)
}

func (e *Engine) candidateVersions(art model.Artifact, explicit bool) ([]model.Version, error) {
	if explicit {
		return e.cat.AllPublishedVersions(art.ID)
	}
	return e.cat.PublishedVersions(art.ID)
}

func (e *Engine) selectVersion(cands []model.Version, cst constraint.Constraint, explicit bool) (*model.Version, []CandidateElimination) {
	var best *model.Version
	var elims []CandidateElimination
	for i := range cands {
		v := cands[i]
		sv, err := semver.Parse(v.Version)
		if err != nil {
			elims = append(elims, CandidateElimination{Version: v.Version, Reason: "invalid semver: " + err.Error()})
			continue
		}
		if !cst.Match(sv) {
			elims = append(elims, CandidateElimination{Version: v.Version, Reason: "does not match constraint"})
			continue
		}
		if best == nil {
			best = &cands[i]
			continue
		}
		bv, _ := semver.Parse(best.Version)
		if semver.Compare(sv, bv) > 0 {
			elims = append(elims, CandidateElimination{Version: best.Version, Reason: "lower version, superseded by " + v.Version})
			best = &cands[i]
		} else {
			elims = append(elims, CandidateElimination{Version: v.Version, Reason: "lower version, superseded by " + best.Version})
		}
	}
	return best, elims
}

// detectConflicts 检测清单中同一制品被声明了互斥约束。
func (e *Engine) detectConflicts(manifest []ManifestItem) []Diagnostic {
	var out []Diagnostic
	seen := map[string]string{}
	for _, item := range manifest {
		if prev, ok := seen[item.Name]; ok && prev != item.Constraint {
			out = append(out, Diagnostic{
				Type:    "CONFLICT",
				Message: fmt.Sprintf("artifact %q has conflicting constraints %q and %q", item.Name, prev, item.Constraint),
				Details: item.Name,
			})
		} else {
			seen[item.Name] = item.Constraint
		}
	}
	return out
}

func joinPath(path []string) string {
	out := ""
	for i, p := range path {
		if i > 0 {
			out += " -> "
		}
		out += p
	}
	return out
}

func parentRef(parent string) string {
	if parent == "" {
		return "manifest"
	}
	return parent
}
