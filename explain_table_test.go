package sqlglot_test

import (
	"strings"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// EXPLAIN-form consistency (extensions mysql-explain-modifier-table / pg-explain-table): every
// EXPLAIN variant — plain, ANALYZE, FORMAT, and the `TABLE t` SELECT-shorthand target — parses to
// exp.Describe in both dialects, and the TABLE keyword survives the round trip (a query-explain of
// a table scan, not a metadata DESCRIBE; engine-verified on MySQL 8.0/8.4 + PG 16/17).
func TestExplainFormsUnified(t *testing.T) {
	for _, tt := range []struct{ dialect, sql, want string }{
		{"mysql", "EXPLAIN SELECT * FROM users", "DESCRIBE SELECT * FROM users"},
		{"mysql", "EXPLAIN ANALYZE SELECT * FROM users", "DESCRIBE ANALYZE SELECT * FROM users"},
		{"mysql", "EXPLAIN TABLE users", "DESCRIBE TABLE users"},
		{"mysql", "EXPLAIN ANALYZE TABLE users", "DESCRIBE ANALYZE TABLE users"},
		{"mysql", "EXPLAIN FORMAT=JSON SELECT * FROM users", "DESCRIBE FORMAT=JSON SELECT * FROM users"},
		{"mysql", "EXPLAIN FORMAT=JSON TABLE users", "DESCRIBE FORMAT=JSON TABLE users"},
		{"mysql", "EXPLAIN ANALYZE FORMAT=TREE TABLE users", "DESCRIBE ANALYZE FORMAT=TREE TABLE users"},
		// Lowercase source spelling must not leak into the kind discriminator (upstream
		// parser.py:2399 upper()s the creatable kind) — and the generated form must reparse
		// to the same shape.
		{"mysql", "explain analyze table users", "DESCRIBE ANALYZE TABLE users"},
		{"mysql", "explain table users", "DESCRIBE TABLE users"},
		{"postgres", "EXPLAIN SELECT * FROM users", "EXPLAIN SELECT * FROM users"},
		{"postgres", "EXPLAIN ANALYZE SELECT * FROM users", "EXPLAIN ANALYZE SELECT * FROM users"},
		{"postgres", "EXPLAIN TABLE users", "EXPLAIN TABLE users"},
		{"postgres", "EXPLAIN ANALYZE TABLE users", "EXPLAIN ANALYZE TABLE users"},
		{"postgres", "EXPLAIN (ANALYZE, FORMAT JSON) TABLE users", "EXPLAIN (ANALYZE, FORMAT JSON) TABLE users"},
	} {
		e, err := sqlglot.ParseOne(tt.sql, tt.dialect)
		if err != nil {
			t.Errorf("[%s] %q: %v", tt.dialect, tt.sql, err)
			continue
		}
		if e.Kind() != exp.KindDescribe {
			t.Errorf("[%s] %q: kind = %v, want Describe: %s", tt.dialect, tt.sql, exp.ClassName(e.Kind()), e.ToS())
			continue
		}
		// Per-dialect discriminators (see DEVIATIONS): PG keeps kind=EXPLAIN and the TABLE
		// shorthand shows as a Table target; MySQL marks TABLE targets kind=TABLE.
		if tt.dialect == "postgres" && e.Text("kind") != "EXPLAIN" {
			t.Errorf("[postgres] %q: kind = %q, want EXPLAIN", tt.sql, e.Text("kind"))
		}
		if strings.Contains(tt.sql, "TABLE ") {
			if e.This() == nil || e.This().Kind() != exp.KindTable {
				t.Errorf("[%s] %q: target not a Table: %s", tt.dialect, tt.sql, e.ToS())
			}
			if tt.dialect == "mysql" && e.Text("kind") != "TABLE" {
				t.Errorf("[mysql] %q: kind = %q, want TABLE", tt.sql, e.Text("kind"))
			}
		}
		out, err := sqlglot.Generate(e, tt.dialect, generator.Options{})
		if err != nil || out != tt.want {
			t.Errorf("[%s] %q -> %q (want %q, err %v)", tt.dialect, tt.sql, out, tt.want, err)
		}
	}
}

// The TABLE target inside a Describe is a real exp.Table for the analyzer to scope, and a
// qualified name keeps its parts.
func TestExplainTableTargetShape(t *testing.T) {
	for _, tt := range []struct{ dialect, sql string }{
		{"mysql", "EXPLAIN ANALYZE TABLE sch.users"},
		{"postgres", "EXPLAIN (ANALYZE) TABLE sch.users"},
	} {
		e, err := sqlglot.ParseOne(tt.sql, tt.dialect)
		if err != nil {
			t.Fatalf("[%s] %q: %v", tt.dialect, tt.sql, err)
		}
		target := e.This()
		if target == nil || target.Kind() != exp.KindTable || target.Name() != "users" || target.SchemaName() != "sch" {
			t.Errorf("[%s] %q: target not Table sch.users:\n%s", tt.dialect, tt.sql, e.ToS())
		}
	}
	// Garbage / unrepresentable forms fail closed in BOTH dialects: trailing tokens, PG
	// inheritance scope markers (ONLY, trailing *) the Table node cannot carry.
	for _, tt := range []struct{ dialect, sql string }{
		{"mysql", "EXPLAIN ANALYZE TABLE t garbage garbage"},
		{"mysql", "EXPLAIN TABLE TABLE t"},
		{"postgres", "EXPLAIN TABLE t garbage"},
		{"postgres", "EXPLAIN (ANALYZE) TABLE t garbage"},
		{"postgres", "EXPLAIN TABLE ONLY t"},
		{"postgres", "EXPLAIN TABLE t *"},
		{"mysql", "EXPLAIN ANALYZE TABLE t PARTITION(p0)"}, // TABLE stmt has no partition selection (engine 1064)
		{"mysql", "EXPLAIN ANALYZE TABLE t AS JSON"},       // no metadata clauses on a query-explain
	} {
		e, err := sqlglot.ParseOne(tt.sql, tt.dialect)
		if err != nil {
			t.Fatalf("[%s] %q: %v", tt.dialect, tt.sql, err)
		}
		if e.Kind() != exp.KindCommand {
			t.Errorf("[%s] %q should degrade to Command: %s", tt.dialect, tt.sql, e.ToS())
		}
	}
}

// A plain MySQL metadata describe must NOT be painted with the TABLE query-explain marker —
// the query-explain vs metadata-describe distinction is the consumer's classification line.
func TestDescribeMetadataFormUnmarked(t *testing.T) {
	e, err := sqlglot.ParseOne("DESCRIBE users", "mysql")
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind() != exp.KindDescribe || e.Text("kind") != "" {
		t.Fatalf("DESCRIBE users: kind = %q, want empty (metadata form): %s", e.Text("kind"), e.ToS())
	}
	out, err := sqlglot.Generate(e, "mysql", generator.Options{})
	if err != nil || out != "DESCRIBE users" {
		t.Fatalf("metadata describe grew a keyword: %q %v", out, err)
	}
}
