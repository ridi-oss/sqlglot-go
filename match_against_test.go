package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
)

// TestMatchAgainst ports upstream tests/dialects/test_mysql.py::test_match_against — the
// base FUNCTION_PARSERS["MATCH"] entry (parser.py:1508 -> _parse_match_against). MATCH(...)
// AGAINST(...) parses in base + MySQL and renders the same; Postgres transforms it to the
// `@@` full-text form (generators/postgres.py matchAgainstSQL override).
func TestMatchAgainst(t *testing.T) {
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
		read   string
		writes []wr
	}{
		{
			sql:  "MATCH(col1, col2, col3) AGAINST('abc')",
			read: "mysql",
			writes: []wr{
				{"", "MATCH(col1, col2, col3) AGAINST('abc')"},
				{"mysql", "MATCH(col1, col2, col3) AGAINST('abc')"},
				// not quite correct because it's not ts_query (per upstream's own note).
				{"postgres", "(col1 @@ 'abc' OR col2 @@ 'abc' OR col3 @@ 'abc')"},
			},
		},
		// read as base too, to mirror upstream's read={"": ...} arm.
		{
			sql:    "MATCH(col1, col2, col3) AGAINST('abc')",
			read:   "",
			writes: []wr{{"", "MATCH(col1, col2, col3) AGAINST('abc')"}},
		},
		{
			sql:    "MATCH(col1, col2) AGAINST('abc' IN NATURAL LANGUAGE MODE)",
			read:   "mysql",
			writes: []wr{{"mysql", "MATCH(col1, col2) AGAINST('abc' IN NATURAL LANGUAGE MODE)"}},
		},
		{
			sql:    "MATCH(col1, col2) AGAINST('abc' IN NATURAL LANGUAGE MODE WITH QUERY EXPANSION)",
			read:   "mysql",
			writes: []wr{{"mysql", "MATCH(col1, col2) AGAINST('abc' IN NATURAL LANGUAGE MODE WITH QUERY EXPANSION)"}},
		},
		{
			sql:    "MATCH(col1, col2) AGAINST('abc' IN BOOLEAN MODE)",
			read:   "mysql",
			writes: []wr{{"mysql", "MATCH(col1, col2) AGAINST('abc' IN BOOLEAN MODE)"}},
		},
		{
			sql:    "MATCH(col1, col2) AGAINST('abc' WITH QUERY EXPANSION)",
			read:   "mysql",
			writes: []wr{{"mysql", "MATCH(col1, col2) AGAINST('abc' WITH QUERY EXPANSION)"}},
		},
		{
			sql:    "MATCH(a.b) AGAINST('abc')",
			read:   "mysql",
			writes: []wr{{"mysql", "MATCH(a.b) AGAINST('abc')"}},
		},
	}
	for _, tc := range cases {
		for _, w := range tc.writes {
			got := gen(t, tc.sql, tc.read, w.write)
			if got != w.want {
				t.Errorf("read=%q write=%q %q\n  got  %q\n  want %q", tc.read, w.write, tc.sql, got, w.want)
			}
		}
	}

	// A bare MATCH(x) with no AGAINST clause fails loud (required arg `this` missing),
	// exactly as upstream raises ParseError.
	if _, err := sqlglot.ParseOne("SELECT MATCH(x)", "mysql"); err == nil {
		t.Errorf("SELECT MATCH(x): expected a parse error (missing AGAINST), got nil")
	}
}
