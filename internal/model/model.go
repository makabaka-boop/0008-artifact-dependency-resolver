package model

import (
	"regexp"
	"time"
)

// 实体状态枚举。
type VersionStatus string

const (
	StatusDraft      VersionStatus = "draft"
	StatusPublished  VersionStatus = "published"
	StatusDeprecated VersionStatus = "deprecated"
)

type ResolutionStatus string

const (
	ResPending   ResolutionStatus = "pending"
	ResRunning   ResolutionStatus = "running"
	ResSucceeded ResolutionStatus = "succeeded"
	ResFailed    ResolutionStatus = "failed"
)

// Artifact 表示一个软件制品。
type Artifact struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Version 表示制品的某个语义化版本。
type Version struct {
	ID           int64         `json:"id"`
	ArtifactID   int64         `json:"artifact_id"`
	Version      string        `json:"version"`
	Status       VersionStatus `json:"status"`
	PublishedAt  *time.Time    `json:"published_at,omitempty"`
	DeprecatedAt *time.Time    `json:"deprecated_at,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// Dependency 表示版本对目标制品声明的约束。
type Dependency struct {
	ID            int64     `json:"id"`
	FromVersionID int64     `json:"from_version_id"`
	ToArtifactID  int64     `json:"to_artifact_id"`
	Constraint    string    `json:"constraint"`
	CreatedAt     time.Time `json:"created_at"`
}

// DependencyTarget 携带依赖目标制品名，方便查询结果返回。
type DependencyTarget struct {
	Dependency
	ToArtifactName string `json:"to_artifact_name"`
}

// ResolutionRequest 表示一次依赖解析请求。
type ResolutionRequest struct {
	ID           int64            `json:"id"`
	RequestRef   string           `json:"request_ref"`
	ManifestJSON string           `json:"manifest_json"`
	Status       ResolutionStatus `json:"status"`
	ErrorCode    string           `json:"error_code,omitempty"`
	ErrorMessage string           `json:"error_message,omitempty"`
	GraphJSON    string           `json:"graph_json,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	FinishedAt   *time.Time       `json:"finished_at,omitempty"`
}

// ResolutionNode 反规范化保存单个解析结果节点。
type ResolutionNode struct {
	ID         int64  `json:"id"`
	RequestID  int64  `json:"request_id"`
	ArtifactID int64  `json:"artifact_id"`
	VersionID  int64  `json:"version_id"`
	Depth      int    `json:"depth"`
	Reason     string `json:"reason"`
}

// ChangeRecord 记录关键动作的变更痕迹，append-only。
type ChangeRecord struct {
	ID         int64     `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   int64     `json:"entity_id"`
	Action     string    `json:"action"`
	BeforeJSON string    `json:"before_json,omitempty"`
	AfterJSON  string    `json:"after_json,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

var artifactNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9_-]*[a-z0-9])?)*$`)

// ValidArtifactName 校验制品名是否满足规范。
func ValidArtifactName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	return artifactNameRe.MatchString(name)
}
