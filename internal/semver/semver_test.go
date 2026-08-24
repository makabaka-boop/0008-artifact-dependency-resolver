package semver

import "testing"

func TestParseValid(t *testing.T) {
	cases := []string{"1.2.3", "1.2.3-alpha", "1.2.3-alpha.1", "1.2.3+build.5", "1.2.3-alpha+build", "0.0.1"}
	for _, c := range cases {
		if _, err := Parse(c); err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", c, err)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []string{"", "1", "1.2", "1.2.3.4", "01.2.3", "1.2", "a.b.c", "1.2.3-", "1.2.3+"}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("Parse(%q) expected error, got nil", c)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.4", -1},
		{"1.2.4", "1.2.3", 1},
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "2.0.0", -1},
		{"1.2.3-alpha", "1.2.3", -1},
		{"1.2.3", "1.2.3-alpha", 1},
		{"1.2.3-alpha", "1.2.3-beta", -1},
		{"1.2.3-alpha.1", "1.2.3-alpha.2", -1},
		{"1.2.3-alpha.10", "1.2.3-alpha.2", 1},
		{"1.2.3+build1", "1.2.3+build2", 0},
	}
	for _, c := range cases {
		a, _ := Parse(c.a)
		b, _ := Parse(c.b)
		if got := Compare(a, b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSortStable(t *testing.T) {
	versions := []Version{}
	for _, s := range []string{"1.2.4", "1.2.3", "2.0.0", "1.2.3-alpha"} {
		v, _ := Parse(s)
		versions = append(versions, v)
	}
	Sort(versions)
	want := []string{"1.2.3-alpha", "1.2.3", "1.2.4", "2.0.0"}
	for i, w := range want {
		if versions[i].String() != w {
			t.Errorf("sorted[%d] = %s, want %s", i, versions[i].String(), w)
		}
	}
}
