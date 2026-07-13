package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestParseLateralView covers parseQueryModifiers' post-joins LATERAL loop
// (parser.py:4180-4185): a bare LATERAL VIEW clause trailing a table source must
// attach to the query's "laterals" arg, not get dropped. parseLateral itself
// (parser/parser_pivot.go:8) and the lateralSQL generator (generator/sql.go:1823)
// were already complete; only the attach point in parseQueryModifiers was missing.
func TestParseLateralView(t *testing.T) {
	sql := "SELECT a, b FROM t LATERAL VIEW EXPLODE(scores) v AS score"
	root := parseOne(t, sql)

	lateral := root.Find(exp.KindLateral)
	if lateral == nil {
		t.Fatalf("expected a Lateral node in the tree:\n%s", root.ToS())
	}
	if lateral.Arg("view") != true {
		t.Fatalf("Lateral.view should be true:\n%s", root.ToS())
	}
	alias := exprArg(t, lateral, "alias")
	if alias.Kind() != exp.KindTableAlias {
		t.Fatalf("Lateral.alias should be TableAlias, got %v:\n%s", alias.Kind(), root.ToS())
	}

	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}

// TestStraightJoin covers the parseTableAlias guard that keeps tokens.STRAIGHT_JOIN
// from ever being consumed as a table alias, in any dialect (upstream base/mysql
// exclude it from TABLE_ALIAS_TOKENS despite it being an ID_VAR_TOKENS member;
// parsers/base.py:16-17).
func TestStraightJoin(t *testing.T) {
	baseSQL := "SELECT * FROM a STRAIGHT_JOIN b"
	root := parseOne(t, baseSQL)
	joins := expressionsForArg(root, "joins")
	if len(joins) != 1 || joins[0].Kind() != exp.KindJoin || joins[0].Arg("kind") != "STRAIGHT_JOIN" {
		t.Fatalf("expected a single STRAIGHT_JOIN join:\n%s", root.ToS())
	}
	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != baseSQL {
		t.Fatalf("round-trip = %q, want %q", got, baseSQL)
	}

	mysqlSQL := "SELECT e.* FROM e STRAIGHT_JOIN p ON e.x = p.y"
	mysqlRoot := parseOneDialect(t, mysqlSQL, "mysql")
	mysqlJoins := expressionsForArg(mysqlRoot, "joins")
	if len(mysqlJoins) != 1 || mysqlJoins[0].Kind() != exp.KindJoin || mysqlJoins[0].Arg("kind") != "STRAIGHT_JOIN" {
		t.Fatalf("expected a single STRAIGHT_JOIN join:\n%s", mysqlRoot.ToS())
	}
	got, err = generateSQL(t, mysqlRoot, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != mysqlSQL {
		t.Fatalf("round-trip = %q, want %q", got, mysqlSQL)
	}
}
