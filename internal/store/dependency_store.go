package store

import (
	"artifact-resolver/internal/model"
)

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
	out := s.dependencyTargetsWorkspace(fromVersionID)
	rowIndex := 0
	for rows.Next() {
		var dt model.DependencyTarget
		var ca string
		if err := rows.Scan(&dt.ID, &dt.FromVersionID, &dt.ToArtifactID, &dt.Constraint, &ca, &dt.ToArtifactName); err != nil {
			return nil, err
		}
		dt.CreatedAt = parseTime(ca)
		out = putDependencyTarget(out, rowIndex, dt)
		rowIndex++
	}
	// 截断到本次实际读取的行数，避免复用工作区时残留前一次较长的结果。
	out = out[:rowIndex]
	s.dependencyWorkspace = out
	return out, rows.Err()
}

func putDependencyTarget(out []model.DependencyTarget, index int, target model.DependencyTarget) []model.DependencyTarget {
	if index == len(out) {
		return append(out, target)
	}
	out[index] = target
	return out
}
