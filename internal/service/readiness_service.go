package service

import (
	"context"

	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/model"
	"artifact-resolver/internal/resolver"
)

// ReadinessBlocker 描述一个阻断发布就绪的依赖问题。
type ReadinessBlocker struct {
	Name    string `json:"name"`
	Reason  string `json:"reason"`
	Details string `json:"details,omitempty"`
}

// ReadinessOutput 是发布就绪检查的结果。
type ReadinessOutput struct {
	Artifact string             `json:"artifact"`
	Version  string             `json:"version"`
	Ready    bool               `json:"ready"`
	Blockers []ReadinessBlocker `json:"blockers"`
}

// CheckReadiness 校验某版本的全部依赖是否可解析，并列出阻断项。
// 未声明任何依赖的版本视为就绪。
func (s *Service) CheckReadiness(ctx context.Context, name, version string) (ReadinessOutput, error) {
	_, v, err := s.getVersion(ctx, name, version)
	if err != nil {
		return ReadinessOutput{}, err
	}
	if v.Status != model.StatusDraft {
		return ReadinessOutput{}, newAPIError(errcode.CodeAlreadyPublished, "readiness check only applies to draft versions")
	}

	deps, err := s.st.ListDependencies(v.ID)
	if err != nil {
		return ReadinessOutput{}, newAPIError(errcode.CodeInternal, err.Error())
	}

	out := ReadinessOutput{Artifact: name, Version: version, Ready: true, Blockers: []ReadinessBlocker{}}
	if len(deps) == 0 {
		return out, nil
	}

	manifest := make([]resolver.ManifestItem, 0, len(deps))
	for _, d := range deps {
		manifest = append(manifest, resolver.ManifestItem{Name: d.ToArtifactName, Constraint: d.Constraint})
	}

	result := s.readinessResolver.Resolve(manifest)
	if result.Status == "failed" {
		out.Ready = false
		for _, d := range result.Diagnostics {
			out.Blockers = append(out.Blockers, ReadinessBlocker{
				Reason:  d.Message,
				Details: d.Details,
			})
		}
	}
	return out, nil
}
