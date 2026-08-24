package store

import (
	"database/sql"
	"errors"

	"artifact-resolver/internal/model"
)

// CreateResolutionRequest 创建 pending 状态的解析请求。
func (s *Store) CreateResolutionRequest(requestRef, manifestJSON string) (model.ResolutionRequest, error) {
	r := model.ResolutionRequest{
		RequestRef:   requestRef,
		ManifestJSON: manifestJSON,
		Status:       model.ResPending,
		CreatedAt:    nowTime(),
	}
	res, err := s.db.Exec(
		`INSERT INTO resolution_requests(request_ref, manifest_json, status, created_at) VALUES (?,?,?,?)`,
		r.RequestRef, r.ManifestJSON, string(r.Status), fs(r.CreatedAt),
	)
	if err != nil {
		return model.ResolutionRequest{}, err
	}
	r.ID, _ = res.LastInsertId()
	return r, nil
}

// FinishResolutionRequest 把请求置为终态并写入结果。
func (s *Store) FinishResolutionRequest(id int64, status model.ResolutionStatus, errCode, errMsg, graphJSON string) error {
	fin := nowTime()
	_, err := s.db.Exec(
		`UPDATE resolution_requests SET status=?, error_code=?, error_message=?, graph_json=?, finished_at=? WHERE id=?`,
		string(status), errCode, errMsg, graphJSON, fs(fin), id,
	)
	return err
}

// GetResolutionRequest 按 ID 查询请求。
func (s *Store) GetResolutionRequest(id int64) (model.ResolutionRequest, error) {
	var r model.ResolutionRequest
	var errCode, errMsg, graph, fin sql.NullString
	var ca string
	err := s.db.QueryRow(
		`SELECT id, request_ref, manifest_json, status, error_code, error_message, graph_json, created_at, finished_at
		 FROM resolution_requests WHERE id = ?`, id,
	).Scan(&r.ID, &r.RequestRef, &r.ManifestJSON, &r.Status, &errCode, &errMsg, &graph, &ca, &fin)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ResolutionRequest{}, ErrNotFound
	}
	if err != nil {
		return model.ResolutionRequest{}, err
	}
	r.ErrorCode = errCode.String
	r.ErrorMessage = errMsg.String
	r.GraphJSON = graph.String
	r.CreatedAt = parseTime(ca)
	if fin.Valid {
		t := parseTime(fin.String)
		r.FinishedAt = &t
	}
	return r, nil
}

// ListResolutionRequests 分页列出请求，可按状态过滤。
func (s *Store) ListResolutionRequests(limit, offset int, status string) ([]model.ResolutionRequest, error) {
	q := `SELECT id, request_ref, manifest_json, status, error_code, error_message, graph_json, created_at, finished_at
	      FROM resolution_requests`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ResolutionRequest
	for rows.Next() {
		var r model.ResolutionRequest
		var errCode, errMsg, graph, fin sql.NullString
		var ca string
		if err := rows.Scan(&r.ID, &r.RequestRef, &r.ManifestJSON, &r.Status, &errCode, &errMsg, &graph, &ca, &fin); err != nil {
			return nil, err
		}
		r.ErrorCode = errCode.String
		r.ErrorMessage = errMsg.String
		r.GraphJSON = graph.String
		r.CreatedAt = parseTime(ca)
		if fin.Valid {
			t := parseTime(fin.String)
			r.FinishedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SaveResolutionNodes 保存解析结果明细。
func (s *Store) SaveResolutionNodes(requestID int64, nodes []model.ResolutionNode) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, n := range nodes {
		n.RequestID = requestID
		if _, err := tx.Exec(
			`INSERT INTO resolution_nodes(request_id, artifact_id, version_id, depth, reason) VALUES (?,?,?,?,?)`,
			n.RequestID, n.ArtifactID, n.VersionID, n.Depth, n.Reason,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListResolutionNodes 列出某请求的结果节点。
func (s *Store) ListResolutionNodes(requestID int64) ([]model.ResolutionNode, error) {
	rows, err := s.db.Query(
		`SELECT id, request_id, artifact_id, version_id, depth, reason
		 FROM resolution_nodes WHERE request_id = ? ORDER BY depth, artifact_id`, requestID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ResolutionNode
	for rows.Next() {
		var n model.ResolutionNode
		if err := rows.Scan(&n.ID, &n.RequestID, &n.ArtifactID, &n.VersionID, &n.Depth, &n.Reason); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
