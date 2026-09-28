package parser_test

import (
	"errors"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	sqlerrors "github.com/ridi-oss/sqlglot-go/errors"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// DEVIATIONS §1.21: PostgreSQL has no UNNEST ... WITH OFFSET; only WITH ORDINALITY exists.
func TestPostgresUnnestWithOffsetRejected(t *testing.T) {
	for _, sql := range []string{
		"SELECT * FROM unnest(ARRAY[1,2]) WITH OFFSET AS n",
		"SELECT * FROM unnest(ARRAY[1,2]) WITH OFFSET",
		"SELECT * FROM unnest(ARRAY[1,2]) AS u WITH OFFSET n",
		"SELECT * FROM unnest(ARRAY[1,2]) AS u(v) WITH OFFSET",
		"SELECT * FROM unnest(ARRAY[1,2]) WITH OFFSET LIMIT 1",
	} {
		_, err := sqlglot.ParseOne(sql, "postgres")
		var parseErr *sqlerrors.ParseError
		if !errors.As(err, &parseErr) || len(parseErr.Errors) == 0 || parseErr.Errors[0]["highlight"] != "WITH" {
			t.Errorf("postgres %q: want parse error at WITH, got %v", sql, err)
		}
	}
	// Other dialects keep upstream's shared grammar and fold the name into the alias.
	for _, tc := range []struct{ sql, want string }{
		{"SELECT * FROM unnest(ARRAY[1,2]) WITH OFFSET AS n", "SELECT * FROM UNNEST(ARRAY(1, 2)) WITH ORDINALITY"},
		{"SELECT * FROM unnest(ARRAY[1,2]) AS u WITH OFFSET n", "SELECT * FROM UNNEST(ARRAY(1, 2)) WITH ORDINALITY AS u(n)"},
		{"SELECT * FROM unnest(ARRAY[1,2]) AS u(v) WITH OFFSET", "SELECT * FROM UNNEST(ARRAY(1, 2)) WITH ORDINALITY AS u(v, offset)"},
	} {
		for _, dialect := range []string{"mysql", ""} {
			e := parseOneDialect(t, tc.sql, dialect)
			if got, err := sqlglot.Generate(e, dialect, generator.Options{}); err != nil || got != tc.want {
				t.Errorf("%s %q: got %q, %v; want %q", dialect, tc.sql, got, err, tc.want)
			}
		}
	}
}

// WITH ORDINALITY keeps upstream's shape: a surplus alias column becomes the Identifier offset
// (parser.py:5205-5207) and folds back on generation.
func TestUnnestOrdinalityAliasShape(t *testing.T) {
	for _, tc := range []struct {
		sql, offset string
		columns     int
	}{
		{"SELECT * FROM UNNEST(a) WITH ORDINALITY AS u(v, n)", "n", 1},
		{"SELECT * FROM UNNEST(a, b) WITH ORDINALITY AS u(x, y, n)", "n", 2},
		{"SELECT * FROM UNNEST(a) WITH ORDINALITY AS u(v)", "", 1},
		{"SELECT * FROM UNNEST(a) WITH ORDINALITY", "", 0},
	} {
		for _, dialect := range []string{"postgres", "", "mysql"} {
			e := parseOneDialect(t, tc.sql, dialect)
			unnest := e.FindAll(exp.KindUnnest)[0]
			offset := unnest.Arg("offset")
			if tc.offset == "" {
				if offset != true {
					t.Fatalf("%s %s: offset = %v, want true", dialect, tc.sql, offset)
				}
			} else if id, ok := offset.(exp.Expression); !ok || id.Name() != tc.offset {
				t.Fatalf("%s %s: offset = %v, want Identifier %s", dialect, tc.sql, offset, tc.offset)
			}
			if alias, ok := unnest.Arg("alias").(exp.Expression); ok {
				if columns, _ := alias.Arg("columns").([]exp.Expression); len(columns) != tc.columns {
					t.Fatalf("%s %s: alias columns = %d, want %d", dialect, tc.sql, len(columns), tc.columns)
				}
			}
			if got, err := sqlglot.Generate(e, dialect, generator.Options{}); err != nil || got != tc.sql {
				t.Fatalf("%s %s: got %q, %v", dialect, tc.sql, got, err)
			}
		}
	}
}
