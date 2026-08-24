package constraint

import (
	"fmt"
	"strings"

	"artifact-resolver/internal/semver"
)

// Node 表示约束解析后的单个比较单元。
type Node struct {
	Op    string // =, >, >=, <, <=, ^, ~
	Major int
	Minor int
	Patch int
	Raw   string
}

// Constraint 表示一条约束，可含多个以空格连接的 AND 条件或以 || 连接的 OR 分支。
type Constraint struct {
	orGroups [][]Node
	raw      string
}

// Parse 解析约束字符串语法。
func Parse(raw string) (Constraint, error) {
	c := Constraint{raw: raw}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Constraint{}, fmt.Errorf("empty constraint")
	}
	ors := strings.Split(raw, "||")
	for _, orPart := range ors {
		orPart = strings.TrimSpace(orPart)
		if orPart == "" {
			return Constraint{}, fmt.Errorf("empty OR branch in %q", raw)
		}
		group := []Node{}
		fields := strings.Fields(orPart)
		for _, f := range fields {
			n, err := parseNode(f)
			if err != nil {
				return Constraint{}, err
			}
			group = append(group, n)
		}
		if len(group) == 0 {
			return Constraint{}, fmt.Errorf("empty AND branch in %q", raw)
		}
		c.orGroups = append(c.orGroups, group)
	}
	return c, nil
}

func parseNode(f string) (Node, error) {
	n := Node{Raw: f}
	i := 0
	for i < len(f) && (f[i] == '=' || f[i] == '>' || f[i] == '<' || f[i] == '^' || f[i] == '~') {
		i++
	}
	if i == 0 {
		// 裸版本等价于 =。
		n.Op = "="
	} else {
		n.Op = f[:i]
	}
	num := f[i:]
	dots := strings.Split(num, ".")
	if len(dots) != 3 {
		return Node{}, fmt.Errorf("invalid constraint version %q", f)
	}
	v, err := semver.Parse(strings.TrimPrefix(strings.TrimPrefix(num, "v"), "v"))
	_ = v
	if err != nil {
		return Node{}, fmt.Errorf("invalid constraint version %q", f)
	}
	// 逐段解析整数，允许 ^1.2.3 与 ~1.2.3 等写法。
	var major, minor, patch int
	if _, err := fmt.Sscanf(dots[0], "%d", &major); err != nil {
		return Node{}, fmt.Errorf("invalid major in %q", f)
	}
	if _, err := fmt.Sscanf(dots[1], "%d", &minor); err != nil {
		return Node{}, fmt.Errorf("invalid minor in %q", f)
	}
	if _, err := fmt.Sscanf(dots[2], "%d", &patch); err != nil {
		return Node{}, fmt.Errorf("invalid patch in %q", f)
	}
	n.Major, n.Minor, n.Patch = major, minor, patch
	return n, nil
}

// Match 判断版本是否满足约束。
func (c Constraint) Match(v semver.Version) bool {
	for _, group := range c.orGroups {
		groupOK := true
		for _, n := range group {
			if !matchNode(n, v) {
				groupOK = false
				break
			}
		}
		if groupOK {
			return true
		}
	}
	return false
}

func matchNode(n Node, v semver.Version) bool {
	switch n.Op {
	case "=":
		return v.Major == n.Major && v.Minor == n.Minor && v.Patch == n.Patch && v.Prerelease == ""
	case ">":
		return compareCore(v, n) > 0
	case ">=":
		return compareCore(v, n) >= 0
	case "<":
		return compareCore(v, n) < 0
	case "<=":
		return compareCore(v, n) <= 0
	case "^":
		return matchCaret(n, v)
	case "~":
		return matchTilde(n, v)
	default:
		return false
	}
}

func compareCore(v semver.Version, n Node) int {
	return semver.Compare(semver.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch},
		semver.Version{Major: n.Major, Minor: n.Minor, Patch: n.Patch})
}

func matchCaret(n Node, v semver.Version) bool {
	// ^x.y.z 等价于 >=x.y.z 且 <(x+1).0.0（x>0 时）。
	low := semver.Version{Major: n.Major, Minor: n.Minor, Patch: n.Patch}
	if semver.Compare(v, low) < 0 {
		return false
	}
	switch {
	case n.Major > 0:
		high := semver.Version{Major: n.Major + 1}
		return semver.Compare(v, high) < 0
	case n.Minor > 0:
		high := semver.Version{Major: 0, Minor: n.Minor + 1}
		return semver.Compare(v, high) < 0
	default:
		high := semver.Version{Major: 0, Minor: 0, Patch: n.Patch + 1}
		return semver.Compare(v, high) < 0
	}
}

func matchTilde(n Node, v semver.Version) bool {
	if v.Major != n.Major || v.Minor != n.Minor {
		return false
	}
	return v.Patch >= n.Patch
}
