package service

import (
	"context"
	"encoding/json"
	"errors"

	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/model"
	"artifact-resolver/internal/semver"
	"artifact-resolver/internal/store"
)

// APIError 是带错误码的业务错误。
type APIError struct {
	Code    errcode.Code
	Message string
	Status  int
}

func (e *APIError) Error() string { return e.Message }

func newAPIError(code errcode.Code, msg string) *APIError {
	return &APIError{Code: code, Message: msg, Status: code.HTTPStatus()}
}

// Service 编排业务逻辑。
type Service struct {
	st            *store.Store
	lockfileReads map[string]int
}

// New 创建服务。
func New(st *store.Store) *Service {
	return &Service{st: st, lockfileReads: map[string]int{}}
}

// Ping 检查数据库。
func (s *Service) Ping(ctx context.Context) error { return s.st.Ping(ctx) }

// CreateArtifact 创建制品。
func (s *Service) CreateArtifact(ctx context.Context, name, desc string) (model.Artifact, error) {
	if !model.ValidArtifactName(name) {
		return model.Artifact{}, newAPIError(errcode.CodeInvalidArgument, "invalid artifact name")
	}
	a, err := s.st.CreateArtifact(store.ArtifactInput{Name: name, Description: desc})
	if errors.Is(err, store.ErrNameConflict) {
		return model.Artifact{}, newAPIError(errcode.CodeNameConflict, "artifact name already exists")
	}
	if err != nil {
		return model.Artifact{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	_ = s.st.AppendChange(store.ChangeInput{
		EntityType: "artifact", EntityID: a.ID, Action: "create",
		AfterJSON: marshal(a),
	})
	return a, nil
}

// GetArtifact 按名称查询制品。
func (s *Service) GetArtifact(ctx context.Context, name string) (model.Artifact, error) {
	a, err := s.st.GetArtifactByName(name)
	if errors.Is(err, store.ErrNotFound) {
		return model.Artifact{}, newAPIError(errcode.CodeNotFound, "artifact not found")
	}
	if err != nil {
		return model.Artifact{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	return a, nil
}

// ListArtifacts 分页列表。
func (s *Service) ListArtifacts(ctx context.Context, limit, offset int) ([]model.Artifact, error) {
	as, err := s.st.ListArtifacts(limit, offset)
	if err != nil {
		return nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	return as, nil
}

// CreateVersion 创建 draft 版本。
func (s *Service) CreateVersion(ctx context.Context, name, version string) (model.Version, error) {
	art, err := s.st.GetArtifactByName(name)
	if errors.Is(err, store.ErrNotFound) {
		return model.Version{}, newAPIError(errcode.CodeNotFound, "artifact not found")
	}
	if err != nil {
		return model.Version{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	if _, err := semver.Parse(version); err != nil {
		return model.Version{}, newAPIError(errcode.CodeInvalidVersion, err.Error())
	}
	v, err := s.st.CreateVersion(store.VersionInput{ArtifactID: art.ID, Version: version})
	if errors.Is(err, store.ErrVersionExists) {
		return model.Version{}, newAPIError(errcode.CodeVersionExists, "version already exists")
	}
	if err != nil {
		return model.Version{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	_ = s.st.AppendChange(store.ChangeInput{
		EntityType: "version", EntityID: v.ID, Action: "create",
		AfterJSON: marshal(v),
	})
	return v, nil
}

// ListVersions 列出制品版本。
func (s *Service) ListVersions(ctx context.Context, name string) ([]model.Version, error) {
	art, err := s.st.GetArtifactByName(name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, newAPIError(errcode.CodeNotFound, "artifact not found")
	}
	if err != nil {
		return nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	vs, err := s.st.ListVersions(art.ID)
	if err != nil {
		return nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	return vs, nil
}

func (s *Service) getVersion(ctx context.Context, name, version string) (model.Artifact, model.Version, error) {
	art, err := s.st.GetArtifactByName(name)
	if errors.Is(err, store.ErrNotFound) {
		return model.Artifact{}, model.Version{}, newAPIError(errcode.CodeNotFound, "artifact not found")
	}
	if err != nil {
		return model.Artifact{}, model.Version{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	v, err := s.st.GetVersion(art.ID, version)
	if errors.Is(err, store.ErrNotFound) {
		return art, model.Version{}, newAPIError(errcode.CodeNotFound, "version not found")
	}
	if err != nil {
		return art, model.Version{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	return art, v, nil
}

func marshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
