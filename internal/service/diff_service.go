package service

import (
	"context"
	"errors"

	"artifact-resolver/internal/diff"
	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/store"
)

// DiffOutput 是版本依赖差异对比的结果。
type DiffOutput struct {
	Artifact string                  `json:"artifact"`
	From     string                  `json:"from"`
	To       string                  `json:"to"`
	Changes  []diff.DependencyChange `json:"changes"`
}

// CompareVersionDependencies 对比同一制品两个版本的依赖声明差异。
func (s *Service) CompareVersionDependencies(ctx context.Context, name, fromVersion, toVersion string) (DiffOutput, error) {
	if fromVersion == "" || toVersion == "" {
		return DiffOutput{}, newAPIError(errcode.CodeInvalidArgument, "from and to query params are required")
	}
	_, fromV, err := s.getVersion(ctx, name, fromVersion)
	if err != nil {
		return DiffOutput{}, err
	}
	if _, _, err := s.getVersion(ctx, name, toVersion); err != nil {
		return DiffOutput{}, err
	}

	fromDeps, err := s.st.ListDependencies(fromV.ID)
	if err != nil {
		return DiffOutput{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	toV, err := s.st.GetVersion(fromV.ArtifactID, toVersion)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return DiffOutput{}, newAPIError(errcode.CodeNotFound, "version not found")
		}
		return DiffOutput{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	toDeps, err := s.st.ListDependencies(toV.ID)
	if err != nil {
		return DiffOutput{}, newAPIError(errcode.CodeInternal, err.Error())
	}

	return DiffOutput{
		Artifact: name,
		From:     fromVersion,
		To:       toVersion,
		Changes:  diff.Dependencies(fromDeps, toDeps),
	}, nil
}
