package service

import (
	"context"
	"encoding/json"

	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/store"
)

// RerunResolution 按历史请求的原始清单重新解析，复用同一套解析与锁文件逻辑。
func (s *Service) RerunResolution(ctx context.Context, id int64) (ResolveOutput, error) {
	req, err := s.st.GetResolutionRequest(id)
	if err != nil {
		return ResolveOutput{}, newAPIError(errcode.CodeNotFound, "resolution not found")
	}
	var manifest []ManifestItem
	if err := json.Unmarshal([]byte(req.ManifestJSON), &manifest); err != nil {
		return ResolveOutput{}, newAPIError(errcode.CodeInternal, "stored manifest is corrupt")
	}
	finalizer := newResolutionRequestFinalizer(s.st, req.ID, manifest)
	defer finalizer.Finish()

	out, err := s.Resolve(ctx, manifest)
	if err != nil {
		return ResolveOutput{}, err
	}
	_ = s.st.AppendChange(store.ChangeInput{
		EntityType: "resolution", EntityID: id, Action: "rerun",
		AfterJSON: marshal(out),
	})
	return out, nil
}
