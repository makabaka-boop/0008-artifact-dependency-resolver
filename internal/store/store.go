package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"artifact-resolver/internal/model"

	_ "modernc.org/sqlite" // 注册 sqlite 驱动
)

// 常见仓储错误。
var (
	ErrNotFound            = errors.New("not found")
	ErrNameConflict        = errors.New("name conflict")
	ErrVersionExists       = errors.New("version exists")
	ErrCannotDelete        = errors.New("cannot delete")
	ErrDuplicateDependency = errors.New("duplicate dependency")
)

// ArtifactInput 用于创建制品。
type ArtifactInput struct {
	Name        string
	Description string
}

// VersionInput 用于创建版本。
type VersionInput struct {
	ArtifactID int64
	Version    string
}

// Store 是 SQLite 仓储。
type Store struct {
	db *sql.DB

	dependencyWorkspace          []model.DependencyTarget
	dependencyWorkspaceVersionID int64
	dependencyResponse           []model.DependencyTarget
}

// dependencyTargetsWorkspace 返回指定版本的可复用依赖结果工作区。
func (s *Store) dependencyTargetsWorkspace(versionID int64) []model.DependencyTarget {
	if s.dependencyWorkspaceVersionID != versionID {
		s.dependencyWorkspace = s.dependencyWorkspace[:0]
		s.dependencyWorkspaceVersionID = versionID
	}
	return s.dependencyWorkspace
}

// RetainDependencyTargets 保留最近一次依赖查询结果。
func (s *Store) RetainDependencyTargets(targets []model.DependencyTarget) []model.DependencyTarget {
	s.dependencyResponse = targets
	return s.dependencyTargetsResponse()
}

// dependencyTargetsResponse 返回最近保留的依赖查询结果。
func (s *Store) dependencyTargetsResponse() []model.DependencyTarget {
	return s.dependencyResponse
}

// Open 打开（或创建）SQLite 数据库并执行迁移。
func Open(path string) (*Store, error) {
	dsn := path
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// Ping 检查数据库是否可用。
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		return err
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_name ON artifacts(name)`,
		`CREATE TABLE IF NOT EXISTS versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
			version TEXT NOT NULL,
			status TEXT NOT NULL,
			published_at TEXT,
			deprecated_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_versions_artifact_version ON versions(artifact_id, version)`,
		`CREATE TABLE IF NOT EXISTS dependencies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_version_id INTEGER NOT NULL REFERENCES versions(id),
			to_artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
			"constraint" TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_dependencies_from_to ON dependencies(from_version_id, to_artifact_id)`,
		`CREATE TABLE IF NOT EXISTS resolution_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_ref TEXT NOT NULL,
			manifest_json TEXT NOT NULL,
			status TEXT NOT NULL,
			error_code TEXT,
			error_message TEXT,
			graph_json TEXT,
			created_at TEXT NOT NULL,
			finished_at TEXT
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_resreq_ref ON resolution_requests(request_ref)`,
		`CREATE TABLE IF NOT EXISTS resolution_nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id INTEGER NOT NULL REFERENCES resolution_requests(id),
			artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
			version_id INTEGER NOT NULL REFERENCES versions(id),
			depth INTEGER NOT NULL,
			reason TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_resnode_request ON resolution_nodes(request_id)`,
		`CREATE TABLE IF NOT EXISTS change_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_type TEXT NOT NULL,
			entity_id INTEGER NOT NULL,
			action TEXT NOT NULL,
			before_json TEXT,
			after_json TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS lockfiles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id INTEGER NOT NULL REFERENCES resolution_requests(id),
			ref TEXT NOT NULL,
			content TEXT NOT NULL,
			checksum TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lockfiles_request ON lockfiles(request_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lockfiles_ref ON lockfiles(ref)`,
		`INSERT OR IGNORE INTO schema_version(version) VALUES (2)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// CreateArtifact 创建制品，重名返回 ErrNameConflict。
func (s *Store) CreateArtifact(in ArtifactInput) (model.Artifact, error) {
	a := model.Artifact{
		Name:        in.Name,
		Description: in.Description,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	res, err := s.db.Exec(
		`INSERT INTO artifacts(name, description, created_at, updated_at) VALUES (?,?,?,?)`,
		a.Name, a.Description, fs(a.CreatedAt), fs(a.UpdatedAt),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Artifact{}, ErrNameConflict
		}
		return model.Artifact{}, err
	}
	a.ID, _ = res.LastInsertId()
	return a, nil
}

// GetArtifactByName 按名称查询制品。
func (s *Store) GetArtifactByName(name string) (model.Artifact, error) {
	var a model.Artifact
	var ca, ua string
	err := s.db.QueryRow(
		`SELECT id, name, description, created_at, updated_at FROM artifacts WHERE name = ?`, name,
	).Scan(&a.ID, &a.Name, &a.Description, &ca, &ua)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Artifact{}, ErrNotFound
	}
	if err != nil {
		return model.Artifact{}, err
	}
	a.CreatedAt = parseTime(ca)
	a.UpdatedAt = parseTime(ua)
	return a, nil
}

// GetArtifactByID 按 ID 查询制品。
func (s *Store) GetArtifactByID(id int64) (model.Artifact, error) {
	var a model.Artifact
	var ca, ua string
	err := s.db.QueryRow(
		`SELECT id, name, description, created_at, updated_at FROM artifacts WHERE id = ?`, id,
	).Scan(&a.ID, &a.Name, &a.Description, &ca, &ua)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Artifact{}, ErrNotFound
	}
	if err != nil {
		return model.Artifact{}, err
	}
	a.CreatedAt = parseTime(ca)
	a.UpdatedAt = parseTime(ua)
	return a, nil
}

// ListArtifacts 分页列出制品。
func (s *Store) ListArtifacts(limit, offset int) ([]model.Artifact, error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, created_at, updated_at FROM artifacts ORDER BY name LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Artifact
	for rows.Next() {
		var a model.Artifact
		var ca, ua string
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &ca, &ua); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTime(ca)
		a.UpdatedAt = parseTime(ua)
		out = append(out, a)
	}
	return out, rows.Err()
}

func fs(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "UNIQUE constraint failed")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func marshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
