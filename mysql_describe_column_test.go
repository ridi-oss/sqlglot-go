package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// MySQL `{DESCRIBE|DESC|EXPLAIN} tbl_name [col_name | wild]` (a column/wildcard-filtered table
// describe) parses to a structured Describe with this=Table — instead of degrading to Command —
// so a consumer keying on this.Kind() classifies it as a table-describe rather than fail-closed.
// This is an extension beyond pinned upstream, which parse-errors the form (see
// testdata/upstream_extensions.jsonl "mysql-describe-column"). Verified against MySQL 8.4.

func TestMySQLDescribeColumn(t *testing.T) {
	cases := []struct {
		sql     string
		wantSQL string
	}{
		{"DESCRIBE users id", "DESCRIBE users id"},
		{"DESC users id", "DESCRIBE users id"},    // DESC leader normalizes to DESCRIBE
		{"EXPLAIN users id", "DESCRIBE users id"}, // EXPLAIN is a MySQL DESCRIBE synonym
		{"DESCRIBE mydb.users id", "DESCRIBE mydb.users id"},
		{"DESCRIBE `users` `id`", "DESCRIBE `users` `id`"},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindDescribe {
				t.Fatalf("kind = %v, want Describe:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
			// The security-relevant invariant: the target is a Table, so it classifies as a
			// table-describe (not a query-explain).
			this, ok := e.Arg("this").(exp.Expression)
			if !ok || this.Kind() != exp.KindTable {
				t.Fatalf("this = %v, want Table:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
			if e.Arg("column") == nil {
				t.Fatalf("column not captured:\n%s", e.ToS())
			}
			out, gerr := sqlglot.Generate(e, "mysql", generator.Options{})
			if gerr != nil {
				t.Fatalf("generate: %v", gerr)
			}
			if out != tc.wantSQL {
				t.Fatalf("round-trip = %q, want %q", out, tc.wantSQL)
			}
		})
	}
}

func TestMySQLDescribeWildcard(t *testing.T) {
	e, err := sqlglot.ParseOne(`DESCRIBE users 'i%'`, "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	col, ok := e.Arg("column").(exp.Expression)
	if !ok || col.Kind() != exp.KindLiteral || col.Arg("is_string") != true {
		t.Fatalf("column = %v, want string Literal:\n%s", exp.ClassName(e.Kind()), e.ToS())
	}
	if got := col.Text("this"); got != "i%" {
		t.Fatalf("wildcard = %q, want i%%", got)
	}
	if out, _ := sqlglot.Generate(e, "mysql", generator.Options{}); out != `DESCRIBE users 'i%'` {
		t.Fatalf("round-trip = %q", out)
	}
}

func TestMySQLDescribeColumnBoundaries(t *testing.T) {
	// Bare describe is unchanged (no regression); trailing junk past a single col/wild fails
	// closed to Command; the column form is MySQL-gated; and a statement target never has a
	// trailing token grabbed as a column.
	t.Run("bare describe still structured", func(t *testing.T) {
		e, _ := sqlglot.ParseOne("DESCRIBE users", "mysql")
		if e.Kind() != exp.KindDescribe || e.Arg("column") != nil {
			t.Fatalf("bare describe changed:\n%s", e.ToS())
		}
	})
	t.Run("trailing junk fails closed", func(t *testing.T) {
		e, err := sqlglot.ParseOne("DESCRIBE users id extra", "mysql")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if e.Kind() != exp.KindCommand {
			t.Fatalf("kind = %v, want Command (fail-closed):\n%s", exp.ClassName(e.Kind()), e.ToS())
		}
	})
	// The column slot is a single identifier token, NOT the general expression grammar. The
	// dangerous forms — anything that could carry its own table reads or widen scope behind
	// this=Table — must fail closed to Command: a parenthesized subquery (the bypass vector), a
	// function call, a cast, a qualified/multi-part column, a bracket subscript, and a bare
	// numeric literal. This is what real MySQL rejects at the col_name position too.
	t.Run("non-identifier column slot fails closed", func(t *testing.T) {
		for _, sql := range []string{
			"DESCRIBE users (SELECT 1 FROM secret)", // subquery — the bypass vector
			"DESCRIBE users (SELECT * FROM secret WHERE 1 = 1)",
			"DESCRIBE users concat(a, b)", // function call
			"DESCRIBE users CAST(1 AS SIGNED)",
			"DESCRIBE users id.name", // qualified column
			"DESCRIBE users id.a.b",  // multi-part
			"DESCRIBE users id[1]",   // bracket subscript
			"DESCRIBE users 1",       // numeric literal
		} {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			if e.Kind() != exp.KindCommand {
				t.Fatalf("%q: kind = %v, want Command (fail-closed):\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
			}
		}
	})

	// col_name matches MySQL exactly: a non-reserved unquoted name or ANY backtick-quoted name is
	// accepted (stays this=Table); an unquoted reserved word is rejected (MySQL rejects it too).
	t.Run("non-reserved and quoted column names accepted", func(t *testing.T) {
		for _, sql := range []string{
			"DESCRIBE users comment", // non-reserved keyword-ish name
			"DESCRIBE users type",
			"DESCRIBE users `NULL`",  // quoted reserved word is a valid column name
			"DESCRIBE users `order`", // quoted reserved word
		} {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			this, _ := e.Arg("this").(exp.Expression)
			if e.Kind() != exp.KindDescribe || this == nil || this.Kind() != exp.KindTable {
				t.Fatalf("%q: want Describe with this=Table:\n%s", sql, e.ToS())
			}
		}
	})
	t.Run("unquoted reserved word fails closed", func(t *testing.T) {
		for _, sql := range []string{"DESCRIBE users NULL", "DESCRIBE users order", "DESCRIBE users select"} {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			if e.Kind() != exp.KindCommand {
				t.Fatalf("%q: kind = %v, want Command:\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
			}
		}
	})
	// A leading ANALYZE/FORMAT modifier is the query-explain form (`EXPLAIN ANALYZE TABLE t` is an
	// explain of the `TABLE t` query — a row-reading scan — not a metadata describe), and no
	// trailing PARTITION/AS JSON is valid after a col. All must fail closed to Command rather than
	// misclassify a query-explain as a table-describe or admit a statement MySQL rejects.
	t.Run("modifier and trailing-clause forms fail closed", func(t *testing.T) {
		for _, sql := range []string{
			"EXPLAIN ANALYZE TABLE t",         // query-explain of `TABLE t`
			"EXPLAIN FORMAT=JSON TABLE t",     // query-explain of `TABLE t`
			"EXPLAIN ANALYZE users id",        // ANALYZE modifier + col is invalid
			"EXPLAIN FORMAT=JSON users id",    // FORMAT modifier + col is invalid
			"DESCRIBE users id PARTITION(p1)", // no PARTITION after a col
			"DESCRIBE users id AS JSON",       // no AS JSON after a col
			"DESCRIBE users 'i%' AS JSON",
		} {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			if e.Kind() != exp.KindCommand {
				t.Fatalf("%q: kind = %v, want Command (fail-closed):\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
			}
		}
	})
	for _, dialect := range []string{"postgres", ""} {
		t.Run("column form is mysql-gated/"+dialect, func(t *testing.T) {
			e, err := sqlglot.ParseOne("DESCRIBE x id", dialect)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindCommand {
				t.Fatalf("dialect %q kind = %v, want Command:\n%s", dialect, exp.ClassName(e.Kind()), e.ToS())
			}
		})
	}
	t.Run("statement target is not a column-describe", func(t *testing.T) {
		// `SELECT 1 foo` binds foo as a SELECT alias; this stays a query-explain (this=Select),
		// and the column-grab gate (this must be a Table) does not fire.
		e, err := sqlglot.ParseOne("DESCRIBE SELECT 1 foo", "mysql")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		this, _ := e.Arg("this").(exp.Expression)
		if this == nil || this.Kind() != exp.KindSelect {
			t.Fatalf("this = %v, want Select:\n%s", exp.ClassName(e.Kind()), e.ToS())
		}
		if e.Arg("column") != nil {
			t.Fatalf("column wrongly grabbed for a statement target:\n%s", e.ToS())
		}
	})
}
