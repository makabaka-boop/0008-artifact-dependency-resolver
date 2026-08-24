package store

import (
	"database/sql"
	"errors"

	"artifact-resolver/internal/model"
)

// CreateLockfile 持久化一份锁文件快照，同一解析请求只保留一份。
func (s *Store) CreateLockfile(in LockfileInput) (model.LockfileSnapshot, error) {
	lf := model.LockfileSnapshot{
		RequestID: in.RequestID,
		Ref:       in.Ref,
		Content:   in.Content,
		Checksum:  in.Checksum,
		CreatedAt: nowTime(),
	}
	res, err := s.db.Exec(
		`INSERT INTO lockfiles(request_id, ref, content, checksum, created_at) VALUES (?,?,?,?,?)`,
		lf.RequestID, lf.Ref, lf.Content, lf.Checksum, fs(lf.CreatedAt),
	)
	if err != nil {
		return model.LockfileSnapshot{}, err
	}
	lf.ID, _ = res.LastInsertId()
	return lf, nil
}

// LockfileInput 用于创建锁文件快照。
type LockfileInput struct {
	RequestID int64
	Ref       string
	Content   string
	Checksum  string
}

// GetLockfileByRequest 按解析请求 ID 查询锁文件快照。
func (s *Store) GetLockfileByRequest(requestID int64) (model.LockfileSnapshot, error) {
	return scanLockfile(s.db.QueryRow(
		`SELECT id, request_id, ref, content, checksum, created_at
		 FROM lockfiles WHERE request_id = ?`, requestID,
	))
}

// GetLockfileByRef 按业务引用号查询锁文件快照。
func (s *Store) GetLockfileByRef(ref string) (model.LockfileSnapshot, error) {
	return scanLockfile(s.db.QueryRow(
		`SELECT id, request_id, ref, content, checksum, created_at
		 FROM lockfiles WHERE ref = ?`, ref,
	))
}

func scanLockfile(row rowScanner) (model.LockfileSnapshot, error) {
	var lf model.LockfileSnapshot
	var ca string
	err := row.Scan(&lf.ID, &lf.RequestID, &lf.Ref, &lf.Content, &lf.Checksum, &ca)
	if errors.Is(err, sql.ErrNoRows) {
		return model.LockfileSnapshot{}, ErrNotFound
	}
	if err != nil {
		return model.LockfileSnapshot{}, err
	}
	lf.CreatedAt = parseTime(ca)
	return lf, nil
}
