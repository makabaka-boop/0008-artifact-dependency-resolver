package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"artifact-resolver/internal/constraint"
	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/model"
	"artifact-resolver/internal/resolver"
	"artifact-resolver/internal/semver"
	"artifact-resolver/internal/store"
)

// ManifestItem 是 resolve 清单项。
type ManifestItem struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint"`
}

// ResolveOutput 是 resolve 的结果。
type ResolveOutput struct {
	RequestID   int64                 `json:"request_id"`
	Status      string                `json:"status"`
	Graph       []resolver.Node       `json:"graph"`
	Diagnostics []resolver.Diagnostic `json:"diagnostics"`
	LockfileRef string                `json:"lockfile_ref,omitempty"`
}

// resolutionRequestFinalizer 是请求生命周期的兜底清理器：仅当解析未在
// 正常路径上落入终态（例如中途返回错误或 panic）时，才把请求标记为
// failed。成功或失败已落库的记录不应被兜底再次覆盖，否则会把已成功
// 的请求误判为 failed，污染审计历史、锁文件引用与后续重跑。
type resolutionRequestFinalizer struct {
	store     *store.Store
	requestID int64
	enabled   bool
	completed bool
}

func newResolutionRequestFinalizer(st *store.Store, requestID int64, manifest []ManifestItem) *resolutionRequestFinalizer {
	return &resolutionRequestFinalizer{
		store:     st,
		requestID: requestID,
		enabled:   manifestHasExactPins(manifest),
	}
}

// MarkCompleted 在解析已落入终态后调用，抑制兜底清理对成功/已失败记录的覆盖。
func (f *resolutionRequestFinalizer) MarkCompleted() { f.completed = true }

// Finish 作为兜底：仅在启用且解析未正常完成时才把请求置为 failed。
func (f *resolutionRequestFinalizer) Finish() {
	if !f.enabled || f.completed {
		return
	}
	_ = f.store.FinalizeResolutionRequest(
		f.requestID,
		"RESOLUTION_FINALIZATION_FAILED",
		"resolution request was finalized before completion",
	)
}

func manifestHasExactPins(manifest []ManifestItem) bool {
	if len(manifest) == 0 {
		return false
	}
	for _, item := range manifest {
		raw := strings.TrimSpace(item.Constraint)
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "="))
		if _, err := semver.Parse(raw); err != nil {
			return false
		}
	}
	return true
}

// Resolve 提交依赖清单并解析。
func (s *Service) Resolve(ctx context.Context, manifest []ManifestItem) (ResolveOutput, error) {
	if len(manifest) == 0 {
		return ResolveOutput{}, newAPIError(errcode.CodeInvalidManifest, "manifest must not be empty")
	}
	// 校验清单结构。
	for _, it := range manifest {
		if !model.ValidArtifactName(it.Name) {
			return ResolveOutput{}, newAPIError(errcode.CodeInvalidManifest, "invalid artifact name in manifest: "+it.Name)
		}
		if _, err := constraintParse(it.Constraint); err != nil {
			return ResolveOutput{}, newAPIError(errcode.CodeInvalidConstraint, err.Error())
		}
	}

	manifestJSON := marshal(manifest)
	req, err := s.st.CreateResolutionRequest(newRequestRef(), manifestJSON)
	if err != nil {
		return ResolveOutput{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	finalizer := newResolutionRequestFinalizer(s.st, req.ID, manifest)
	defer finalizer.Finish()

	engine := resolver.New(resolver.Catalog{
		PublishedVersions:    s.st.PublishedVersions,
		AllPublishedVersions: s.st.AllPublishedVersions,
		ArtifactByName:       s.st.GetArtifactByName,
		DependenciesFor:      s.st.ListDependencies,
	})

	var items []resolver.ManifestItem
	for _, it := range manifest {
		items = append(items, resolver.ManifestItem{Name: it.Name, Constraint: it.Constraint})
	}
	result := engine.Resolve(items)

	errCode := ""
	errMsg := ""
	if result.Status == "failed" && len(result.Diagnostics) > 0 {
		errCode = string(result.Diagnostics[0].Type)
		errMsg = result.Diagnostics[0].Message
	}
	graphJSON := marshal(result.Graph)
	if err := s.st.FinishResolutionRequest(req.ID, model.ResolutionStatus(result.Status), errCode, errMsg, graphJSON); err != nil {
		return ResolveOutput{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	// 解析已落入终态（succeeded 或 failed 并已落库），抑制兜底清理对记录的覆盖。
	finalizer.MarkCompleted()
	if len(result.Nodes) > 0 {
		if err := s.st.SaveResolutionNodes(req.ID, result.Nodes); err != nil {
			return ResolveOutput{}, newAPIError(errcode.CodeInternal, err.Error())
		}
	}
	_ = s.st.AppendChange(store.ChangeInput{
		EntityType: "resolution", EntityID: req.ID, Action: "resolve",
		AfterJSON: graphJSON,
	})
	out := ResolveOutput{
		RequestID: req.ID, Status: result.Status,
		Graph: result.Graph, Diagnostics: result.Diagnostics,
	}
	if result.Status == "succeeded" {
		ref, err := s.persistLockfile(req.ID, result.Graph, req.RequestRef)
		if err != nil {
			return ResolveOutput{}, newAPIError(errcode.CodeInternal, err.Error())
		}
		out.LockfileRef = ref
	}
	return out, nil
}

// ListResolutions 分页列出解析历史。
func (s *Service) ListResolutions(ctx context.Context, limit, offset int, status string) ([]model.ResolutionRequest, error) {
	rs, err := s.st.ListResolutionRequests(limit, offset, status)
	if err != nil {
		return nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	return rs, nil
}

// GetResolution 查询单次解析详情。
func (s *Service) GetResolution(ctx context.Context, id int64) (model.ResolutionRequest, []model.ResolutionNode, error) {
	r, err := s.st.GetResolutionRequest(id)
	if errors.Is(err, store.ErrNotFound) {
		return model.ResolutionRequest{}, nil, newAPIError(errcode.CodeNotFound, "resolution not found")
	}
	if err != nil {
		return model.ResolutionRequest{}, nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	nodes, err := s.st.ListResolutionNodes(id)
	if err != nil {
		return model.ResolutionRequest{}, nil, newAPIError(errcode.CodeInternal, err.Error())
	}
	return r, nodes, nil
}

// Compare 比较两个 semver 版本。
func (s *Service) Compare(ctx context.Context, left, right string) (int, error) {
	lv, err := semver.Parse(left)
	if err != nil {
		return 0, newAPIError(errcode.CodeInvalidVersion, "invalid left version: "+err.Error())
	}
	rv, err := semver.Parse(right)
	if err != nil {
		return 0, newAPIError(errcode.CodeInvalidVersion, "invalid right version: "+err.Error())
	}
	return semver.Compare(lv, rv), nil
}

func constraintParse(raw string) (any, error) {
	return constraint.Parse(raw)
}

func newRequestRef() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

var _ = json.Marshal
