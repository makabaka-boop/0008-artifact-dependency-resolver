package service

import (
	"context"
	"encoding/json"

	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/store"
)

// RerunResolution 按历史请求的原始清单重新解析，复用同一套解析与锁文件逻辑。
// 重跑生成的是一条新的解析请求记录，不再触碰原历史记录的终态，因此不注册
// 兜底清理器，避免把原成功记录误判为 failed。
func (s *Service) RerunResolution(ctx context.Context, id int64) (ResolveOutput, error) {
	req, err := s.st.GetResolutionRequest(id)
	if err != nil {
		return ResolveOutput{}, newAPIError(errcode.CodeNotFound, "resolution not found")
	}
	var manifest []ManifestItem
	if err := json.Unmarshal([]byte(req.ManifestJSON), &manifest); err != nil {
		return ResolveOutput{}, newAPIError(errcode.CodeInternal, "stored manifest is corrupt")
	}

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
