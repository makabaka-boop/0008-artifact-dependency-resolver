package semver

import "sort"

// ByVersion 实现按语义化版本升序排序。
type ByVersion []Version

func (s ByVersion) Len() int      { return len(s) }
func (s ByVersion) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s ByVersion) Less(i, j int) bool {
	return Compare(s[i], s[j]) < 0
}

// Sort 升序排序版本切片。
func Sort(versions []Version) { sort.Sort(ByVersion(versions)) }
