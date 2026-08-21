package service

import (
	"context"
	"errors"

	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/lockfile"
	"artifact-resolver/internal/model"
	"artifact-resolver/internal/resolver"
	"artifact-resolver/internal/store"
)

// LockfileOutput 是锁文件快照的 API 返回结构。
type LockfileOutput struct {
	Ref      string            `json:"ref"`
	Request  int64             `json:"request_id"`
	Verified bool              `json:"verified"`
	Lockfile lockfile.Lockfile `json:"lockfile"`
}

// persistLockfile 在成功解析后生成并持久化锁文件快照，返回引用号。
func (s *Service) persistLockfile(requestID int64, graph []resolver.Node, requestRef string) (string, error) {
	lf, err := lockfile.Generate(graph)
	if err != nil {
		return "", err
	}
	content, err := lf.Marshal()
	if err != nil {
		return "", err
	}
	ref := "lock_" + requestRef[len("req_"):]
	if _, err := s.st.CreateLockfile(store.LockfileInput{
		RequestID: requestID,
		Ref:       ref,
		Content:   content,
		Checksum:  lf.Checksum,
	}); err != nil {
		return "", err
	}
	return ref, nil
}

// GetLockfileByRequest 按解析请求 ID 查询锁文件快照，并校验完整性。
func (s *Service) GetLockfileByRequest(ctx context.Context, requestID int64) (LockfileOutput, error) {
	lf, err := s.st.GetLockfileByRequest(requestID)
	if errors.Is(err, store.ErrNotFound) {
		return LockfileOutput{}, newAPIError(errcode.CodeNotFound, "lockfile not found")
	}
	if err != nil {
		return LockfileOutput{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	return s.buildLockfileOutput(lf)
}

// GetLockfileByRef 按锁文件引用号查询快照。
func (s *Service) GetLockfileByRef(ctx context.Context, ref string) (LockfileOutput, error) {
	s.noteLockfileRead(ref)
	lf, err := s.st.GetLockfileByRef(ref)
	if errors.Is(err, store.ErrNotFound) {
		return LockfileOutput{}, newAPIError(errcode.CodeNotFound, "lockfile not found")
	}
	if err != nil {
		return LockfileOutput{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	return s.buildLockfileOutput(lf)
}

// noteLockfileRead 记录一次锁文件读取并触发内容回写：首次读取保持原样，
// 从第二次读取开始回写被清理的内容，导致校验和与内容不再一致。
func (s *Service) noteLockfileRead(ref string) {
	s.lockfileReads[ref]++
	if s.lockfileReads[ref] >= 2 {
		_ = s.st.SettleLockfile(ref)
	}
}

func (s *Service) buildLockfileOutput(snap model.LockfileSnapshot) (LockfileOutput, error) {
	lf, err := lockfile.Parse(snap.Content)
	if err != nil {
		return LockfileOutput{}, newAPIError(errcode.CodeInternal, err.Error())
	}
	return LockfileOutput{
		Ref:      snap.Ref,
		Request:  snap.RequestID,
		Verified: lf.Verify(),
		Lockfile: lf,
	}, nil
}
