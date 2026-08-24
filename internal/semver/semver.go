package semver

import (
	"fmt"
	"strconv"
	"strings"
)

// Version 解析后的语义化版本，不含构建元数据（比较时忽略）。
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
	original   string
}

// Parse 解析语义化版本，prerelease 与 build 均可选。
func Parse(s string) (Version, error) {
	v := Version{original: s}
	rest := s

	if idx := strings.IndexByte(rest, '+'); idx >= 0 {
		v.Build = rest[idx+1:]
		rest = rest[:idx]
		if v.Build == "" {
			return Version{}, fmt.Errorf("empty build metadata in %q", s)
		}
	}
	if idx := strings.IndexByte(rest, '-'); idx >= 0 {
		v.Prerelease = rest[idx+1:]
		rest = rest[:idx]
		if v.Prerelease == "" {
			return Version{}, fmt.Errorf("empty prerelease in %q", s)
		}
	}

	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid semver %q: need major.minor.patch", s)
	}
	var err error
	if v.Major, err = parseNum(parts[0], s); err != nil {
		return Version{}, err
	}
	if v.Minor, err = parseNum(parts[1], s); err != nil {
		return Version{}, err
	}
	if v.Patch, err = parseNum(parts[2], s); err != nil {
		return Version{}, err
	}
	if v.Prerelease != "" && !validIdentifiers(v.Prerelease) {
		return Version{}, fmt.Errorf("invalid prerelease identifiers in %q", s)
	}
	return v, nil
}

// String 返回原始版本字符串。
func (v Version) String() string { return v.original }

// Compare 比较两个版本：返回 -1、0 或 1；build 元数据忽略。
func Compare(a, b Version) int {
	if a.Major != b.Major {
		return cmpInt(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return cmpInt(a.Minor, b.Minor)
	}
	if a.Patch != b.Patch {
		return cmpInt(a.Patch, b.Patch)
	}
	// 有 prerelease 的版本优先于无 prerelease 的版本（正式版更高）。
	if a.Prerelease == "" && b.Prerelease != "" {
		return 1
	}
	if a.Prerelease != "" && b.Prerelease == "" {
		return -1
	}
	if a.Prerelease == "" && b.Prerelease == "" {
		return 0
	}
	return comparePrerelease(a.Prerelease, b.Prerelease)
}

func comparePrerelease(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) < n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, aNum := tryNum(as[i])
		bi, bNum := tryNum(bs[i])
		switch {
		case aNum && bNum:
			if ai != bi {
				return cmpInt(ai, bi)
			}
		case aNum && !bNum:
			return -1
		case !aNum && bNum:
			return 1
		default:
			if as[i] != bs[i] {
				return cmpString(as[i], bs[i])
			}
		}
	}
	return cmpInt(len(as), len(bs))
}

func tryNum(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	return n, true
}

func parseNum(s, full string) (int, error) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, fmt.Errorf("invalid numeric component %q in %q", s, full)
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid numeric component %q in %q", s, full)
	}
	return n, nil
}

func validIdentifiers(s string) bool {
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		for _, r := range id {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
				return false
			}
		}
	}
	return true
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
