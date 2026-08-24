package store

import (
	"artifact-resolver/internal/model"
)

// ChangeInput 描述一次变更记录。
type ChangeInput struct {
	EntityType string
	EntityID   int64
	Action     string
	BeforeJSON string
	AfterJSON  string
}

// AppendChange 追加一条变更记录。
func (s *Store) AppendChange(in ChangeInput) error {
	c := model.ChangeRecord{
		EntityType: in.EntityType,
		EntityID:   in.EntityID,
		Action:     in.Action,
		BeforeJSON: in.BeforeJSON,
		AfterJSON:  in.AfterJSON,
		CreatedAt:  nowTime(),
	}
	_, err := s.db.Exec(
		`INSERT INTO change_records(entity_type, entity_id, action, before_json, after_json, created_at)
		 VALUES (?,?,?,?,?,?)`,
		c.EntityType, c.EntityID, c.Action, c.BeforeJSON, c.AfterJSON, fs(c.CreatedAt),
	)
	return err
}

// ListChanges 列出某实体的变更记录。
func (s *Store) ListChanges(entityType string, entityID int64) ([]model.ChangeRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, entity_type, entity_id, action, before_json, after_json, created_at
		 FROM change_records WHERE entity_type = ? AND entity_id = ? ORDER BY id DESC`,
		entityType, entityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ChangeRecord
	for rows.Next() {
		var c model.ChangeRecord
		var ca string
		if err := rows.Scan(&c.ID, &c.EntityType, &c.EntityID, &c.Action, &c.BeforeJSON, &c.AfterJSON, &ca); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(ca)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListChangesPage 分页列出变更记录，可选择性按实体类型与 ID 过滤。
// entityType 为空表示不过滤类型；entityID < 0 表示不过滤 ID。
func (s *Store) ListChangesPage(limit, offset int, entityType string, entityID int64) ([]model.ChangeRecord, error) {
	q := `SELECT id, entity_type, entity_id, action, before_json, after_json, created_at FROM change_records`
	var args []any
	var conds []string
	if entityType != "" {
		conds = append(conds, `entity_type = ?`)
		args = append(args, entityType)
	}
	if entityID >= 0 {
		conds = append(conds, `entity_id = ?`)
		args = append(args, entityID)
	}
	if len(conds) > 0 {
		q += ` WHERE ` + conds[0]
		for _, c := range conds[1:] {
			q += ` AND ` + c
		}
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ChangeRecord
	for rows.Next() {
		var c model.ChangeRecord
		var ca string
		if err := rows.Scan(&c.ID, &c.EntityType, &c.EntityID, &c.Action, &c.BeforeJSON, &c.AfterJSON, &ca); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(ca)
		out = append(out, c)
	}
	return out, rows.Err()
}
