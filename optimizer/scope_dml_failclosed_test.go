package optimizer

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestDMLScopeBuildScopeNilOnOmittedRoot covers the fail-open a review found: when the DML
// root's source set is incomplete (a FROM source that is not a Table/Subquery — here a VALUES
// derived table), _traverseDML omits the DML-root scope (complete-or-none). BuildScope must then
// return nil, not the last retained CHILD scope (which lacks the target + sources) — otherwise a
// consumer treating a non-nil BuildScope as the statement scope fails open.
func TestDMLScopeBuildScopeNilOnOmittedRoot(t *testing.T) {
	e := parseDMLDialect(t, "UPDATE t SET x = (SELECT y FROM child) FROM (VALUES (1)) AS v(id)", "postgres")

	// No DML-root scope is emitted (complete-or-none: VALUES source is rejected).
	for _, s := range traverseScope(e) {
		if isDMLRootScope(s) {
			t.Fatalf("traverseScope emitted a DML-root scope despite a rejected FROM source")
		}
	}
	// BuildScope must be nil (not a partial child scope missing the target).
	if bs := buildScope(e); bs != nil {
		var keys []string
		for k := range bs.Sources {
			keys = append(keys, k)
		}
		t.Fatalf("BuildScope returned a non-nil partial scope when the DML root was omitted; sources=%v", keys)
	}
}

// TestDMLScopeNestedJoinOnJoinFailsClosed covers the second fail-open: a Join node carrying its
// own `joins` (u.joins=[joinV], joinV.joins=[joinW]) — not a shape current parsers produce, but
// possible via direct AST construction. The source walk recurses only join.This(), so joinW's
// source `w` would be silently dropped, emitting a partial DML-root scope. It must instead be
// rejected fail-closed (the DML root is omitted).
func TestDMLScopeNestedJoinOnJoinFailsClosed(t *testing.T) {
	tbl := func(n string) exp.Expression { return exp.Table(exp.Args{"this": exp.ToIdentifier(n)}) }
	joinW := exp.Join(exp.Args{"this": tbl("w"), "kind": "CROSS"})
	joinV := exp.Join(exp.Args{"this": tbl("v"), "kind": "CROSS", "joins": []exp.Expression{joinW}})
	u := tbl("u")
	u.Set("joins", []exp.Expression{joinV})

	del := parseDMLDialect(t, "DELETE FROM target", "postgres")
	del.Set("using", []exp.Expression{u})

	for _, s := range traverseScope(del) {
		if isDMLRootScope(s) {
			t.Fatalf("emitted a DML-root scope for a joins-on-Join shape (would silently drop w)")
		}
	}
	if bs := buildScope(del); bs != nil {
		t.Fatalf("BuildScope returned non-nil for the fail-closed joins-on-Join case")
	}
}
