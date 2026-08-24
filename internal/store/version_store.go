package store

import (
	"database/sql"
	"errors"
	"time"

	"artifact-resolver/internal/model"
)

type timeT = time.Time

// CreateVersion 创建 draft 版本，重复返回 ErrVersionExists。
func (s *Store) CreateVersion(in VersionInput) (model.Version, error) {
	v := model.Version{
		ArtifactID: in.ArtifactID,
		Version:    in.Version,
		Status:     model.StatusDraft,
		CreatedAt:  nowTime(),
		UpdatedAt:  nowTime(),
	}
	res, err := s.db.Exec(
		`INSERT INTO versions(artifact_id, version, status, created_at, updated_at) VALUES (?,?,?,?,?)`,
		v.ArtifactID, v.Version, string(v.Status), fs(v.CreatedAt), fs(v.UpdatedAt),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Version{}, ErrVersionExists
		}
		return model.Version{}, err
	}
	v.ID, _ = res.LastInsertId()
	return v, nil
}

// GetVersion 按制品 ID 与版本字符串查询。
func (s *Store) GetVersion(artifactID int64, version string) (model.Version, error) {
	return s.scanVersion(s.db.QueryRow(
		`SELECT id, artifact_id, version, status, published_at, deprecated_at, created_at, updated_at
		 FROM versions WHERE artifact_id = ? AND version = ?`, artifactID, version,
	))
}

// GetVersionByID 按 ID 查询版本。
func (s *Store) GetVersionByID(id int64) (model.Version, error) {
	return s.scanVersion(s.db.QueryRow(
		`SELECT id, artifact_id, version, status, published_at, deprecated_at, created_at, updated_at
		 FROM versions WHERE id = ?`, id,
	))
}

// ListVersions 列出某制品下的所有版本，按版本字符串升序。
func (s *Store) ListVersions(artifactID int64) ([]model.Version, error) {
	rows, err := s.db.Query(
		`SELECT id, artifact_id, version, status, published_at, deprecated_at, created_at, updated_at
		 FROM versions WHERE artifact_id = ? ORDER BY version`, artifactID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Version
	for rows.Next() {
		v, err := scanVersionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateVersionStatus 更新版本状态及相关时间戳。
func (s *Store) UpdateVersionStatus(id int64, status model.VersionStatus, publish, deprecate bool) (model.Version, error) {
	v, err := s.GetVersionByID(id)
	if err != nil {
		return model.Version{}, err
	}
	v.Status = status
	v.UpdatedAt = nowTime()
	if publish {
		t := nowTime()
		v.PublishedAt = &t
	}
	if deprecate {
		t := nowTime()
		v.DeprecatedAt = &t
	}
	var pub, dep any
	if v.PublishedAt != nil {
		pub = fs(*v.PublishedAt)
	}
	if v.DeprecatedAt != nil {
		dep = fs(*v.DeprecatedAt)
	}
	_, err = s.db.Exec(
		`UPDATE versions SET status=?, published_at=?, deprecated_at=?, updated_at=? WHERE id=?`,
		string(v.Status), pub, dep, fs(v.UpdatedAt), id,
	)
	if err != nil {
		return model.Version{}, err
	}
	return v, nil
}

// DeleteVersion 删除 draft 版本；非 draft 由上层拦截，本层仍保证仅删 draft。
func (s *Store) DeleteVersion(id int64, checkStatus model.VersionStatus) error {
	res, err := s.db.Exec(`DELETE FROM versions WHERE id=? AND status=?`, id, string(checkStatus))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCannotDelete
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanVersion(row rowScanner) (model.Version, error) {
	var v model.Version
	var pub, dep, ca, ua sql.NullString
	err := row.Scan(&v.ID, &v.ArtifactID, &v.Version, &v.Status, &pub, &dep, &ca, &ua)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Version{}, ErrNotFound
	}
	if err != nil {
		return model.Version{}, err
	}
	if pub.Valid {
		t := parseTime(pub.String)
		v.PublishedAt = &t
	}
	if dep.Valid {
		t := parseTime(dep.String)
		v.DeprecatedAt = &t
	}
	v.CreatedAt = parseTime(ca.String)
	v.UpdatedAt = parseTime(ua.String)
	return v, nil
}

func scanVersionRow(rows *sql.Rows) (model.Version, error) {
	var v model.Version
	var pub, dep, ca, ua sql.NullString
	if err := rows.Scan(&v.ID, &v.ArtifactID, &v.Version, &v.Status, &pub, &dep, &ca, &ua); err != nil {
		return model.Version{}, err
	}
	if pub.Valid {
		t := parseTime(pub.String)
		v.PublishedAt = &t
	}
	if dep.Valid {
		t := parseTime(dep.String)
		v.DeprecatedAt = &t
	}
	v.CreatedAt = parseTime(ca.String)
	v.UpdatedAt = parseTime(ua.String)
	return v, nil
}

func nowTime() timeT { return time.Now().UTC() }
