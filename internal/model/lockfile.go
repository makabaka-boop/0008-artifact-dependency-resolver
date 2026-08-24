package model

import "time"

// LockfileEntry 表示锁文件中一个被钉住的依赖条目，用于复现同一次解析结果。
type LockfileEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Depth   int    `json:"depth"`
	Reason  string `json:"reason"`
}

// LockfileSnapshot 持久化一份可复现的锁文件快照，绑定到某次解析请求。
type LockfileSnapshot struct {
	ID        int64     `json:"id"`
	RequestID int64     `json:"request_id"`
	Ref       string    `json:"ref"`
	Content   string    `json:"content"`
	Checksum  string    `json:"checksum"`
	CreatedAt time.Time `json:"created_at"`
}
