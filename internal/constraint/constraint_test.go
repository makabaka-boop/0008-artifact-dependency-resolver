package constraint

import (
	"testing"

	"artifact-resolver/internal/semver"
)

func TestParseAndMatch(t *testing.T) {
	cases := []struct {
		raw     string
		matches []string
		reject  []string
	}{
		{"=1.2.3", []string{"1.2.3"}, []string{"1.2.4", "2.0.0"}},
		{">1.2.3", []string{"1.2.4", "2.0.0"}, []string{"1.2.3", "1.2.2"}},
		{">=1.2.3", []string{"1.2.3", "2.0.0"}, []string{"1.2.2"}},
		{"<2.0.0", []string{"1.9.9"}, []string{"2.0.0", "2.0.1"}},
		{"<=2.0.0", []string{"2.0.0", "1.0.0"}, []string{"2.0.1"}},
		{"^1.2.3", []string{"1.9.9", "1.2.3"}, []string{"2.0.0", "1.2.2"}},
		{"~1.2.3", []string{"1.2.9", "1.2.3"}, []string{"1.3.0", "2.0.0"}},
		{"1.2.3", []string{"1.2.3"}, []string{"1.2.4"}},
		{">=1.0.0 <2.0.0", []string{"1.5.0"}, []string{"2.0.0", "0.9.0"}},
		{"<1.0.0 || >=2.0.0", []string{"0.9.0", "2.0.0"}, []string{"1.5.0"}},
	}
	for _, c := range cases {
		cs, err := Parse(c.raw)
		if err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", c.raw, err)
			continue
		}
		for _, m := range c.matches {
			v, _ := semver.Parse(m)
			if !cs.Match(v) {
				t.Errorf("constraint %q should match %s", c.raw, m)
			}
		}
		for _, r := range c.reject {
			v, _ := semver.Parse(r)
			if cs.Match(v) {
				t.Errorf("constraint %q should NOT match %s", c.raw, r)
			}
		}
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []string{"", "||", "1.2", "foo", ">=", ">=1.", "||1.2.3"}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("Parse(%q) expected error", c)
		}
	}
}
