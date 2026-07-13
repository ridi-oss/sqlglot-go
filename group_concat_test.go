package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
)

// TestGroupConcat ports the MySQL GROUP_CONCAT cases from upstream
// tests/dialects/test_mysql.py::test_string_agg — the MySQL FUNCTION_PARSERS["GROUP_CONCAT"]
// entry (parsers/mysql.py:156 -> _parse_group_concat) plus the MySQL generator
// (generators/mysql.py:174: GROUP_CONCAT({this} SEPARATOR {sep or "','"})). Multiple value
// expressions collapse into a CONCAT; an ORDER BY becomes an exp.Order; the separator
// defaults to ',' when absent. Postgres renders the same node as STRING_AGG (pre-existing
// path) — included here to confirm the AST transpiles, not just round-trips.
func TestGroupConcat(t *testing.T) {
	gen := func(t *testing.T, sql, read, write string) string {
		t.Helper()
		e, err := sqlglot.ParseOne(sql, read)
		if err != nil {
			t.Fatalf("ParseOne(%q, read=%q): %v", sql, read, err)
		}
		out, err := sqlglot.Generate(e, write, generator.Options{})
		if err != nil {
			t.Fatalf("Generate(%q, write=%q): %v", sql, write, err)
		}
		return out
	}

	type wr struct{ write, want string }
	cases := []struct {
		sql    string
		writes []wr
	}{
		{"GROUP_CONCAT(x SEPARATOR ',')", []wr{{"mysql", "GROUP_CONCAT(x SEPARATOR ',')"}}},
		// Separator defaults to ',' when absent.
		{"GROUP_CONCAT(x)", []wr{{"mysql", "GROUP_CONCAT(x SEPARATOR ',')"}}},
		{"GROUP_CONCAT(DISTINCT x ORDER BY y DESC)", []wr{
			{"mysql", "GROUP_CONCAT(DISTINCT x ORDER BY y DESC SEPARATOR ',')"},
			{"postgres", "STRING_AGG(DISTINCT x, ',' ORDER BY y DESC NULLS LAST)"},
		}},
		// Separator can be a non-string field (an identifier), not only a literal.
		{"GROUP_CONCAT(x ORDER BY y SEPARATOR z)", []wr{
			{"mysql", "GROUP_CONCAT(x ORDER BY y SEPARATOR z)"},
			{"postgres", "STRING_AGG(x, z ORDER BY y NULLS FIRST)"},
		}},
		{"GROUP_CONCAT(DISTINCT x ORDER BY y DESC SEPARATOR '')", []wr{
			{"mysql", "GROUP_CONCAT(DISTINCT x ORDER BY y DESC SEPARATOR '')"},
		}},
		// Multiple value expressions collapse into a CONCAT.
		{"GROUP_CONCAT(a, b, c SEPARATOR ',')", []wr{
			{"mysql", "GROUP_CONCAT(CONCAT(a, b, c) SEPARATOR ',')"},
			{"postgres", "STRING_AGG(a || b || c, ',')"},
		}},
		{"GROUP_CONCAT(a, b, c SEPARATOR '')", []wr{
			{"mysql", "GROUP_CONCAT(CONCAT(a, b, c) SEPARATOR '')"},
		}},
		{"GROUP_CONCAT(DISTINCT a, b, c SEPARATOR '')", []wr{
			{"mysql", "GROUP_CONCAT(DISTINCT CONCAT(a, b, c) SEPARATOR '')"},
		}},
		{"GROUP_CONCAT(a, b, c ORDER BY d SEPARATOR '')", []wr{
			{"mysql", "GROUP_CONCAT(CONCAT(a, b, c) ORDER BY d SEPARATOR '')"},
		}},
		{"GROUP_CONCAT(DISTINCT a, b, c ORDER BY d SEPARATOR '')", []wr{
			{"mysql", "GROUP_CONCAT(DISTINCT CONCAT(a, b, c) ORDER BY d SEPARATOR '')"},
		}},
	}
	for _, tc := range cases {
		for _, w := range tc.writes {
			if got := gen(t, tc.sql, "mysql", w.write); got != w.want {
				t.Errorf("write=%q %q\n  got  %q\n  want %q", w.write, tc.sql, got, w.want)
			}
		}
	}
}
