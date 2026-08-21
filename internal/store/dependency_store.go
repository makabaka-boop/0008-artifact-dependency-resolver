package store

import (
	"artifact-resolver/internal/model"
)

// DependencyReplacement 记录一次全量替换的原始快照与已写入项。
type DependencyReplacement struct {
	FromVersionID int64
	Previous      []model.DependencyTarget
	InsertedIDs   []int64
}

// BeginDependencyReplacement 保存当前依赖并清空版本的依赖集合。
func (s *Store) BeginDependencyReplacement(fromVersionID int64) (*DependencyReplacement, error) {
	previous, err := s.ListDependencies(fromVersionID)
	if err != nil {
		return nil, err
	}
	if err := s.DeleteDependenciesFor(fromVersionID); err != nil {
		return nil, err
	}
	return &DependencyReplacement{
		FromVersionID: fromVersionID,
		Previous:      previous,
		InsertedIDs:   make([]int64, 0),
	}, nil
}

// RecordInserted 记录批处理中已经成功写入的依赖。
func (r *DependencyReplacement) RecordInserted(dependencyID int64) {
	r.InsertedIDs = append(r.InsertedIDs, dependencyID)
}

// RollbackDependencyReplacement 清理本批写入并恢复替换前快照。
func (s *Store) RollbackDependencyReplacement(replacement *DependencyReplacement) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range replacement.InsertedIDs {
		if _, err := tx.Exec(
			`DELETE FROM dependencies WHERE id = ? AND from_version_id = ?`,
			id, replacement.FromVersionID,
		); err != nil {
			return err
		}
	}
	for _, d := range replacement.Previous {
		if _, err := tx.Exec(
			`INSERT INTO dependencies(id, from_version_id, to_artifact_id, "constraint", created_at) VALUES (?,?,?,?,?)`,
			d.ID, d.FromVersionID, d.ToArtifactID, d.Constraint, fs(d.CreatedAt),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CreateDependency 为版本声明一条依赖约束。
func (s *Store) CreateDependency(fromVersionID, toArtifactID int64, constraint string) (model.Dependency, error) {
	d := model.Dependency{
		FromVersionID: fromVersionID,
		ToArtifactID:  toArtifactID,
		Constraint:    constraint,
		CreatedAt:     nowTime(),
	}
	res, err := s.db.Exec(
		`INSERT INTO dependencies(from_version_id, to_artifact_id, "constraint", created_at) VALUES (?,?,?,?)`,
		d.FromVersionID, d.ToArtifactID, d.Constraint, fs(d.CreatedAt),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Dependency{}, ErrDuplicateDependency
		}
		return model.Dependency{}, err
	}
	d.ID, _ = res.LastInsertId()
	return d, nil
}

// DeleteDependenciesFor 删除某版本的全部依赖（用于全量替换）。
func (s *Store) DeleteDependenciesFor(fromVersionID int64) error {
	_, err := s.db.Exec(`DELETE FROM dependencies WHERE from_version_id = ?`, fromVersionID)
	return err
}

// ListDependencies 列出某版本声明的依赖，携带目标制品名。
func (s *Store) ListDependencies(fromVersionID int64) ([]model.DependencyTarget, error) {
	rows, err := s.db.Query(
		`SELECT d.id, d.from_version_id, d.to_artifact_id, d."constraint", d.created_at, a.name
		 FROM dependencies d JOIN artifacts a ON a.id = d.to_artifact_id
		 WHERE d.from_version_id = ? ORDER BY a.name`, fromVersionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DependencyTarget
	for rows.Next() {
		var dt model.DependencyTarget
		var ca string
		if err := rows.Scan(&dt.ID, &dt.FromVersionID, &dt.ToArtifactID, &dt.Constraint, &ca, &dt.ToArtifactName); err != nil {
			return nil, err
		}
		dt.CreatedAt = parseTime(ca)
		out = append(out, dt)
	}
	return out, rows.Err()
}
