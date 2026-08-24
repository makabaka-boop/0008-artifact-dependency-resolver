// Package lockfile 生成并校验可复现的依赖锁文件快照。
// 一次成功解析会为每个制品钉住唯一版本，规格化的条目排序保证内容确定性，
// 校验和可用于检测快照内容是否被篡改。
package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"artifact-resolver/internal/model"
	"artifact-resolver/internal/resolver"
)

// LockfileVersion 是当前锁文件格式的版本号。
const LockfileVersion = 1

// Lockfile 表示一份锁文件，包含版本化条目与自校验和。
type Lockfile struct {
	Version  int                   `json:"lockfile_version"`
	Entries  []model.LockfileEntry `json:"entries"`
	Checksum string                `json:"checksum"`
}

// Generate 将解析结果图钉为确定性的锁文件并计算校验和。
func Generate(nodes []resolver.Node) (Lockfile, error) {
	entries := make([]model.LockfileEntry, 0, len(nodes))
	for _, n := range nodes {
		entries = append(entries, model.LockfileEntry{
			Name:    n.Name,
			Version: n.Version,
			Depth:   n.Depth,
			Reason:  n.Reason,
		})
	}
	// 按制品名稳定排序，保证同一结果图总是产出同一份锁文件。
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	sum, err := checksumOf(entries)
	if err != nil {
		return Lockfile{}, err
	}
	return Lockfile{Version: LockfileVersion, Entries: entries, Checksum: sum}, nil
}

// Marshal 将锁文件序列化为规范 JSON 字符串。
func (l Lockfile) Marshal() (string, error) {
	b, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Parse 反序列化锁文件内容。
func Parse(content string) (Lockfile, error) {
	var l Lockfile
	if err := json.Unmarshal([]byte(content), &l); err != nil {
		return Lockfile{}, fmt.Errorf("invalid lockfile: %w", err)
	}
	if l.Version != LockfileVersion {
		return Lockfile{}, fmt.Errorf("unsupported lockfile version %d", l.Version)
	}
	return l, nil
}

// Verify 校验锁文件条目未被篡改（重新计算校验和并比较）。
func (l Lockfile) Verify() bool {
	sum, err := checksumOf(l.Entries)
	if err != nil {
		return false
	}
	return sum == l.Checksum
}

// checksumOf 计算条目序列的 SHA-256 摘要（十六进制）。
func checksumOf(entries []model.LockfileEntry) (string, error) {
	b, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
