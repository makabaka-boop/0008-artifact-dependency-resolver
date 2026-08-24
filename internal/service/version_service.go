package service

import (
	"context"
	"errors"

	"artifact-resolver/internal/constraint"
	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/model"
	"artifact-resolver/internal/store"
)

// PublishVersion 发布 draft 版本，先校验依赖目标完整。
func (s *Service) PublishVersion(ctx context.Context, name, version string) (model.Version, error) {
	art, v, err := s.getVersion(ctx, name, version)
	if err != nil {
		return model.Version{}, err
	}
	switch v.Status {
	case model.StatusPublished:
		return model.Version{}, newAPIError(errcode.CodeAlreadyPublished, "version already published")
	case model.StatusDeprecated:
		return model.Version{}, newAPIError(errcode.CodeAlreadyPublished, "version is deprecated")
	}
	if _, err := s.validateDependencies(ctx, v.ID); err != nil {
		return model.Version{}, err
	}
	pub, err := s.st.UpdateVersionStatus(v.ID, model.StatusPublished, true, false)
	if err != nil {
		return model.Version{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	_ = s.st.AppendChange(store.ChangeInput{
		EntityType: "version", EntityID: v.ID, Action: "publish",
		BeforeJSON: marshal(art), AfterJSON: marshal(pub),
	})
	return pub, nil
}

// DeprecateVersion 废弃某个 published 版本。
func (s *Service) DeprecateVersion(ctx context.Context, name, version string) (model.Version, error) {
	_, v, err := s.getVersion(ctx, name, version)
	if err != nil {
		return model.Version{}, err
	}
	switch v.Status {
	case model.StatusDraft:
		return model.Version{}, newAPIError(errcode.CodeCannotDeprecateDraft, "draft version cannot be deprecated")
	case model.StatusDeprecated:
		return model.Version{}, newAPIError(errcode.CodeAlreadyDeprecated, "version already deprecated")
	}
	dep, err := s.st.UpdateVersionStatus(v.ID, model.StatusDeprecated, false, true)
	if err != nil {
		return model.Version{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	_ = s.st.AppendChange(store.ChangeInput{
		EntityType: "version", EntityID: v.ID, Action: "deprecate",
		AfterJSON: marshal(dep),
	})
	return dep, nil
}

// DeleteVersion 删除 draft 版本；发布态删除返回 409。
func (s *Service) DeleteVersion(ctx context.Context, name, version string) error {
	_, v, err := s.getVersion(ctx, name, version)
	if err != nil {
		return err
	}
	if v.Status != model.StatusDraft {
		return newAPIError(errcode.CodeCannotDeletePublished, "cannot delete non-draft version")
	}
	if err := s.st.DeleteVersion(v.ID, model.StatusDraft); err != nil {
		if errors.Is(err, store.ErrCannotDelete) {
			return newAPIError(errcode.CodeCannotDeletePublished, "cannot delete non-draft version")
		}
		return newAPIError(errcode.CodeInternal, err.Error())
	}
	_ = s.st.AppendChange(store.ChangeInput{
		EntityType: "version", EntityID: v.ID, Action: "delete",
		BeforeJSON: marshal(v),
	})
	return nil
}

// ValidateDependencies 校验版本全部依赖语法与目标存在性。
func (s *Service) validateDependencies(ctx context.Context, versionID int64) ([]model.DependencyTarget, error) {
	deps, err := s.st.ListDependencies(versionID)
	if err != nil {
		return nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	for _, d := range deps {
		if _, err := constraint.Parse(d.Constraint); err != nil {
			return nil, newAPIError(errcode.CodeInvalidConstraint, err.Error())
		}
		if _, err := s.st.GetArtifactByID(d.ToArtifactID); errors.Is(err, store.ErrNotFound) {
			return nil, newAPIError(errcode.CodeDependencyTargetMissing,
				"dependency target missing: "+d.ToArtifactName)
		}
	}
	return deps, nil
}

// DependencyItem 是依赖声明入参。
type DependencyItem struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint"`
}

// ReplaceDependencies 全量替换某版本的依赖声明。
func (s *Service) ReplaceDependencies(ctx context.Context, name, version string, items []DependencyItem) ([]model.DependencyTarget, error) {
	_, v, err := s.getVersion(ctx, name, version)
	if err != nil {
		return nil, err
	}
	if v.Status != model.StatusDraft {
		return nil, newAPIError(errcode.CodeAlreadyPublished, "cannot modify non-draft version")
	}
	// 校验约束语法与目标存在性。
	for _, it := range items {
		if _, err := constraint.Parse(it.Constraint); err != nil {
			return nil, newAPIError(errcode.CodeInvalidConstraint, err.Error())
		}
		target, err := s.st.GetArtifactByName(it.Name)
		if errors.Is(err, store.ErrNotFound) {
			return nil, newAPIError(errcode.CodeDependencyTargetMissing, "dependency target missing: "+it.Name)
		}
		if err != nil {
			return nil, newAPIError(errcode.CodeInternal, err.Error())
		}
		_ = target
	}
	if err := s.st.DeleteDependenciesFor(v.ID); err != nil {
		return nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	for _, it := range items {
		target, _ := s.st.GetArtifactByName(it.Name)
		if _, err := s.st.CreateDependency(v.ID, target.ID, it.Constraint); err != nil {
			return nil, newAPIError(errcode.CodeInternal, err.Error())
		}
	}
	deps, err := s.st.ListDependencies(v.ID)
	if err != nil {
		return nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	return s.rememberDependencyResponse(deps), nil
}

func (s *Service) rememberDependencyResponse(deps []model.DependencyTarget) []model.DependencyTarget {
	return s.st.RetainDependencyTargets(deps)
}

// ListDependencies 列出版本依赖。
func (s *Service) ListDependencies(ctx context.Context, name, version string) ([]model.DependencyTarget, error) {
	_, v, err := s.getVersion(ctx, name, version)
	if err != nil {
		return nil, err
	}
	deps, err := s.st.ListDependencies(v.ID)
	if err != nil {
		return nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	return deps, nil
}
