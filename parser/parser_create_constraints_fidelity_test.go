package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func schemaExpressions(t *testing.T, create exp.Expression) []exp.Expression {
	t.Helper()
	if create.Kind() != exp.KindCreate {
		t.Fatalf("kind = %v, want Create:\n%s", create.Kind(), create.ToS())
	}
	schema := exprArg(t, create, "this")
	if schema.Kind() != exp.KindSchema {
		t.Fatalf("Create.this kind = %v, want Schema:\n%s", schema.Kind(), create.ToS())
	}
	return schema.Expressions()
}

func columnConstraintKinds(t *testing.T, column exp.Expression) []exp.Expression {
	t.Helper()
	if column.Kind() != exp.KindColumnDef {
		t.Fatalf("kind = %v, want ColumnDef:\n%s", column.Kind(), column.ToS())
	}
	constraints := expressionsForArg(column, "constraints")
	kinds := make([]exp.Expression, 0, len(constraints))
	for _, constraint := range constraints {
		if constraint.Kind() != exp.KindColumnConstraint {
			t.Fatalf("constraint kind = %v, want ColumnConstraint:\n%s", constraint.Kind(), column.ToS())
		}
		kinds = append(kinds, exprArg(t, constraint, "kind"))
	}
	return kinds
}

// TestCreateColumnConstraintFidelity ports tests/fixtures/identity.sql:602-604 and the
// corresponding fidelity worklist rows. Besides identity round-trips, it locks the exact
// ColumnConstraint -> constraint-kind nesting and COMPRESS's scalar/list distinction.
func TestCreateColumnConstraintFidelity(t *testing.T) {
	for _, sql := range []string{
		"CREATE TABLE foo (baz CHAR(4) CHARACTER SET LATIN UPPERCASE NOT CASESPECIFIC COMPRESS 'a')",
		"CREATE TABLE db.foo (id INT NOT NULL, valid_date DATE FORMAT 'YYYY-MM-DD', measurement INT COMPRESS)",
		"CREATE TABLE foo (baz DATE FORMAT 'YYYY/MM/DD' TITLE 'title' INLINE LENGTH 1 COMPRESS ('a', 'b'))",
	} {
		roundTripCase(t, "", sql, sql)
	}

	columns := schemaExpressions(t, parseOne(t,
		"CREATE TABLE foo (baz CHAR(4) CHARACTER SET LATIN UPPERCASE NOT CASESPECIFIC COMPRESS 'a')"))
	kinds := columnConstraintKinds(t, columns[0])
	wantKinds := []exp.Kind{
		exp.KindCharacterSetColumnConstraint,
		exp.KindUppercaseColumnConstraint,
		exp.KindCaseSpecificColumnConstraint,
		exp.KindCompressColumnConstraint,
	}
	if len(kinds) != len(wantKinds) {
		t.Fatalf("constraint count = %d, want %d:\n%s", len(kinds), len(wantKinds), columns[0].ToS())
	}
	for i, want := range wantKinds {
		if kinds[i].Kind() != want {
			t.Fatalf("constraint %d kind = %v, want %v:\n%s", i, kinds[i].Kind(), want, columns[0].ToS())
		}
	}
	if kinds[3].This() == nil || !kinds[3].This().IsString() || kinds[3].This().Name() != "a" {
		t.Fatalf("scalar COMPRESS.this mismatch:\n%s", kinds[3].ToS())
	}

	columns = schemaExpressions(t, parseOne(t,
		"CREATE TABLE foo (baz DATE FORMAT 'YYYY/MM/DD' TITLE 'title' INLINE LENGTH 1 COMPRESS ('a', 'b'))"))
	kinds = columnConstraintKinds(t, columns[0])
	wantKinds = []exp.Kind{
		exp.KindDateFormatColumnConstraint,
		exp.KindTitleColumnConstraint,
		exp.KindInlineLengthColumnConstraint,
		exp.KindCompressColumnConstraint,
	}
	if len(kinds) != len(wantKinds) {
		t.Fatalf("constraint count = %d, want %d:\n%s", len(kinds), len(wantKinds), columns[0].ToS())
	}
	for i, want := range wantKinds {
		if kinds[i].Kind() != want {
			t.Fatalf("constraint %d kind = %v, want %v:\n%s", i, kinds[i].Kind(), want, columns[0].ToS())
		}
	}
	compressed := expressionsForArg(kinds[3], "this")
	if len(compressed) != 2 || compressed[0].Name() != "a" || compressed[1].Name() != "b" {
		t.Fatalf("list COMPRESS.this mismatch:\n%s", kinds[3].ToS())
	}
}

func excludeConstraint(t *testing.T, create exp.Expression) exp.Expression {
	t.Helper()
	excludes := create.FindAll(exp.KindExcludeColumnConstraint)
	if len(excludes) != 1 {
		t.Fatalf("ExcludeColumnConstraint count = %d, want 1:\n%s", len(excludes), create.ToS())
	}
	exclude := excludes[0]
	params := exprArg(t, exclude, "this")
	if params.Kind() != exp.KindIndexParameters {
		t.Fatalf("Exclude.this kind = %v, want IndexParameters:\n%s", params.Kind(), create.ToS())
	}
	for i, column := range expressionsForArg(params, "columns") {
		if column.Kind() != exp.KindWithOperator {
			t.Fatalf("Exclude column %d kind = %v, want WithOperator:\n%s", i, column.Kind(), create.ToS())
		}
		if exprArg(t, column, "op").Kind() != exp.KindVar {
			t.Fatalf("WithOperator.op is not Var:\n%s", create.ToS())
		}
	}
	return exclude
}

// TestPostgresExcludeConstraintFidelity ports test_postgres.py:1215-1226 and 1453-1457.
// It covers named/unnamed EXCLUDE constraints, all reserved operator tokens, opclasses and
// ordering/null modifiers, INCLUDE, tablespace/WHERE, and generic WITH storage properties.
func TestPostgresExcludeConstraintFidelity(t *testing.T) {
	cases := []string{
		"CREATE TABLE t (vid INT NOT NULL, CONSTRAINT ht_vid_nid_fid_idx EXCLUDE (INT4RANGE(vid, nid) WITH &&, INT4RANGE(fid, fid, '[]') WITH &&))",
		"CREATE TABLE t (i INT, PRIMARY KEY (i), EXCLUDE USING gist(col varchar_pattern_ops DESC NULLS LAST WITH &&) WITH (sp1=1, sp2=2))",
		"CREATE TABLE t (i INT, EXCLUDE USING btree(INT4RANGE(vid, nid, '[]') ASC NULLS FIRST WITH &&) INCLUDE (col1, col2))",
		"CREATE TABLE t (i INT, EXCLUDE USING gin(col1 WITH &&, col2 WITH ||) USING INDEX TABLESPACE tablespace WHERE (id > 5))",
	}
	for _, sql := range cases {
		roundTripCase(t, "postgres", sql, sql)
		excludeConstraint(t, parseOneDialect(t, sql, "postgres"))
	}

	storageCreate := parseOneDialect(t, cases[1], "postgres")
	params := exprArg(t, excludeConstraint(t, storageCreate), "this")
	storage := expressionsForArg(params, "with_storage")
	if len(storage) != 2 {
		t.Fatalf("with_storage count = %d, want 2:\n%s", len(storage), storageCreate.ToS())
	}
	for i, property := range storage {
		if property.Kind() != exp.KindProperty {
			t.Fatalf("with_storage[%d] kind = %v, want Property:\n%s", i, property.Kind(), storageCreate.ToS())
		}
	}

	for _, op := range []string{"=", ">=", "<=", "<", ">", "&&", "||", "@>", "<@"} {
		sql := "CREATE TABLE circles (c circle, EXCLUDE USING gist(c WITH " + op + "))"
		roundTripCase(t, "postgres", sql, sql)
		exclude := excludeConstraint(t, parseOneDialect(t, sql, "postgres"))
		params := exprArg(t, exclude, "this")
		columns := expressionsForArg(params, "columns")
		if len(columns) != 1 || exprArg(t, columns[0], "op").Name() != op {
			t.Fatalf("operator %q nesting mismatch:\n%s", op, exclude.ToS())
		}
	}
}

// TestPostgresCreateLikeConstraintFidelity ports test_postgres.py:1227-1229. LIKE is a
// schema-level constraint; each INCLUDING/EXCLUDING option is a generic Property nested in
// LikeProperty.expressions.
func TestPostgresCreateLikeConstraintFidelity(t *testing.T) {
	sql := "CREATE TABLE A (LIKE B INCLUDING CONSTRAINT INCLUDING COMPRESSION EXCLUDING COMMENTS)"
	roundTripCase(t, "postgres", sql, sql)
	create := parseOneDialect(t, sql, "postgres")
	items := schemaExpressions(t, create)
	if len(items) != 1 || items[0].Kind() != exp.KindLikeProperty {
		t.Fatalf("schema LIKE nesting mismatch:\n%s", create.ToS())
	}
	options := items[0].Expressions()
	if len(options) != 3 {
		t.Fatalf("LIKE option count = %d, want 3:\n%s", len(options), create.ToS())
	}
	for i, option := range options {
		if option.Kind() != exp.KindProperty {
			t.Fatalf("LIKE option %d kind = %v, want Property:\n%s", i, option.Kind(), create.ToS())
		}
	}
}

// TestPostgresFunctionParameterModes ports test_postgres.py:1327-1352. Mode constraints are
// direct ColumnDef.constraints entries (not wrapped in ColumnConstraint), and MODE TYPE remains
// an ordinary parameter name while MODE NAME TYPE produces InOutColumnConstraint.
func TestPostgresFunctionParameterModes(t *testing.T) {
	cases := []struct {
		sql       string
		modeCount int
	}{
		{"CREATE FUNCTION foo(a INT)", 0},
		{"CREATE FUNCTION foo(IN a INT)", 1},
		{"CREATE FUNCTION foo(OUT a INT)", 1},
		{"CREATE FUNCTION foo(INOUT a INT)", 1},
		{"CREATE FUNCTION foo(VARIADIC a INT[])", 1},
		{"CREATE FUNCTION foo(out INT)", 0},
		{"CREATE FUNCTION foo(inout VARCHAR)", 0},
		{"CREATE FUNCTION foo(variadic INT[])", 0},
		{"CREATE FUNCTION foo(a INT, OUT b INT, INOUT c VARCHAR, VARIADIC d INT[])", 3},
		{"CREATE OR REPLACE FUNCTION foo(INOUT id UUID)", 1},
		{"CREATE OR REPLACE FUNCTION foo(id UUID, OUT created_at TIMESTAMPTZ)", 1},
		{"CREATE FUNCTION foo(OUT x INT DEFAULT 5)", 1},
		{"CREATE FUNCTION foo(INOUT y VARCHAR DEFAULT 'test')", 1},
		{"CREATE FUNCTION foo(IN a INT DEFAULT 0, OUT b INT)", 2},
		{"CREATE FUNCTION foo(OUT result INT, IN input INT DEFAULT 10)", 2},
	}
	for _, tc := range cases {
		roundTripCase(t, "postgres", tc.sql, tc.sql)
		create := parseOneDialect(t, tc.sql, "postgres")
		modes := create.FindAll(exp.KindInOutColumnConstraint)
		if len(modes) != tc.modeCount {
			t.Fatalf("%q: mode count = %d, want %d:\n%s", tc.sql, len(modes), tc.modeCount, create.ToS())
		}
		for _, mode := range modes {
			if mode.Parent() == nil || mode.Parent().Kind() != exp.KindColumnDef {
				t.Fatalf("%q: mode parent is not ColumnDef:\n%s", tc.sql, create.ToS())
			}
		}
	}
}
