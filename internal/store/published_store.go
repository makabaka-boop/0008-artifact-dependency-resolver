package store

import (
	"artifact-resolver/internal/model"
)

// PublishedVersions 返回某制品所有已发布且未废弃的版本（常规解析候选）。
func (s *Store) PublishedVersions(artifactID int64) ([]model.Version, error) {
	rows, err := s.db.Query(
		`SELECT id, artifact_id, version, status, published_at, deprecated_at, created_at, updated_at
		 FROM versions WHERE artifact_id = ? AND status = 'published' AND deprecated_at IS NULL ORDER BY version`, artifactID,
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

// AllPublishedVersions 返回某制品所有已发布版本（含 deprecated，供显式 pin）。
func (s *Store) AllPublishedVersions(artifactID int64) ([]model.Version, error) {
	return s.listVersionsStatus(artifactID, "published")
}

func (s *Store) listVersionsStatus(artifactID int64, status string) ([]model.Version, error) {
	rows, err := s.db.Query(
		`SELECT id, artifact_id, version, status, published_at, deprecated_at, created_at, updated_at
		 FROM versions WHERE artifact_id = ? AND status = ? ORDER BY version`, artifactID, status,
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
