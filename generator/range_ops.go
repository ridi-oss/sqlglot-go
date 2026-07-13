package generator

import (
	"strings"

	"github.com/ridi/sqlglot-go/expressions"
)

// range-ops cluster: generator methods for the Kinds parseRange/parseIs/columnOperators
// build (parser.go). Most upstream classes are `class X(Expression, Binary, ...)` with a
// one-line TRANSFORMS entry `lambda self, e: self.binary(e, "OP")` (generator.py:135-230);
// those are ported as one-liners below. Dialect-specific ones (RegexpLike/RegexpILike/
// JSONBContains only render as infix operators on postgres - generators/postgres.py:326,
// 343-344 - and fall back to a plain function call elsewhere, matching upstream's lack of a
// base TRANSFORMS entry for them) get their own dialect-gated method.
func (g *Generator) globSQL(e expressions.Expression) string     { return g.binary(e, "GLOB") }
func (g *Generator) overlapsSQL(e expressions.Expression) string { return g.binary(e, "OVERLAPS") }
func (g *Generator) adjacentSQL(e expressions.Expression) string { return g.binary(e, "-|-") }
func (g *Generator) arrayContainsAllSQL(e expressions.Expression) string {
	return g.binary(e, "@>")
}
func (g *Generator) arrayContainedBySQL(e expressions.Expression) string {
	return g.binary(e, "<@")
}
func (g *Generator) arrayOverlapsSQL(e expressions.Expression) string { return g.binary(e, "&&") }
func (g *Generator) jsonbContainsAllTopKeysSQL(e expressions.Expression) string {
	return g.binary(e, "?&")
}
func (g *Generator) jsonbContainsAnyTopKeysSQL(e expressions.Expression) string {
	return g.binary(e, "?|")
}
func (g *Generator) jsonbDeleteAtPathSQL(e expressions.Expression) string {
	return g.binary(e, "#-")
}
func (g *Generator) jsonbPathExistsSQL(e expressions.Expression) string { return g.binary(e, "@?") }

// operatorSQL ports generator.py:229 `exp.Operator: lambda self, e: self.binary(e, "")` -
// binary() itself renders the "OPERATOR(...)" prefix from the "operator" arg
// (generator.py:4566-4568, mirrored by Go's g.binary in generator/sql.go).
func (g *Generator) operatorSQL(e expressions.Expression) string { return g.binary(e, "") }

// regexpLikeSQL ports RegexpLike rendering: postgres renders the `~` infix operator
// (generators/postgres.py:343); base/mysql have no TRANSFORMS entry so they keep the
// existing functionFallbackSQL rendering ("REGEXP_LIKE(x, y)") this dispatch entry used to
// get implicitly (KindRegexpLike had no dispatch row before this file).
func (g *Generator) regexpLikeSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		return g.binary(e, "~")
	}
	return g.functionFallbackSQL(e)
}

// regexpILikeSQL mirrors regexpLikeSQL for `~*` / RegexpILike (generators/postgres.py:344).
func (g *Generator) regexpILikeSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		return g.binary(e, "~*")
	}
	return g.functionFallbackSQL(e)
}

// jsonbContainsSQL ports JSONBContains rendering: postgres renders the `?` infix operator
// (generators/postgres.py:326); base/mysql have no TRANSFORMS entry upstream, so they fall
// back to the `_sql_names = ["JSONB_CONTAINS"]` function call (json.py:47-48) - hardcoded
// here rather than via sqlNameOverrides (generator/name.go, out of this part's file scope)
// since camelToSnake("JSONBContains") would otherwise split every capital letter.
func (g *Generator) jsonbContainsSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		return g.binary(e, "?")
	}
	return g.funcCall("JSONB_CONTAINS", g.fallbackArgs(e), "(", ")", true)
}

// jsonArrayContainsSQL ports JSONArrayContains rendering: mysql renders `this MEMBER
// OF(expression)` (generators/mysql.py:685-686); base/postgres have no TRANSFORMS entry
// upstream, so they fall back to the `_sql_names = ["JSON_ARRAY_CONTAINS"]` function call
// (json.py:36-37), again hardcoded for the same camelToSnake reason as jsonbContainsSQL.
func (g *Generator) jsonArrayContainsSQL(e expressions.Expression) string {
	if g.dialect.Name == "mysql" {
		return g.sqlKey(e, "this") + " MEMBER OF(" + g.sqlKey(e, "expression") + ")"
	}
	return g.funcCall("JSON_ARRAY_CONTAINS", g.fallbackArgs(e), "(", ")", true)
}

// jsonSQL ports generator.py:5533 json_sql: `JSON[ this][ WITH|WITHOUT][ UNIQUE KEYS]`,
// where "this" holds the bare IS_JSON_PREDICATE_KIND text (VALUE/SCALAR/ARRAY/OBJECT) and
// "with_" is tri-state (nil = neither WITH nor WITHOUT was written, vs. explicit true/false).
func (g *Generator) jsonSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	withSQL := ""
	if w := e.Arg("with_"); w != nil {
		if boolValue(w) {
			withSQL = " WITH"
		} else {
			withSQL = " WITHOUT"
		}
	}
	uniqueSQL := ""
	if boolValue(e.Arg("unique")) {
		uniqueSQL = " UNIQUE KEYS"
	}
	return "JSON" + this + withSQL + uniqueSQL
}

// matchAgainstSQL ports generator.py:3729 matchagainst_sql (mysql `MATCH(...)
// AGAINST(...)` full-text search - the base/default rendering, kept here since exp.
// MatchAgainst has no dedicated dispatch entry upstream either) and its postgres override
// (generators/postgres.py:459-463), which instead renders postgres' `x @@ y` full-text
// match operator: `{expr} @@ {this}` per element of "expressions", OR-joined and
// parenthesized when there's more than one.
func (g *Generator) matchAgainstSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		this := g.sqlKey(e, "this")
		list, _ := e.Arg("expressions").([]expressions.Expression)
		parts := make([]string, len(list))
		for i, expr := range list {
			parts[i] = g.gen(expr) + " @@ " + this
		}
		sql := strings.Join(parts, " OR ")
		if len(parts) > 1 {
			return "(" + sql + ")"
		}
		return sql
	}

	modifier := g.sqlKey(e, "modifier")
	if modifier != "" {
		modifier = " " + modifier
	}
	return g.funcCall("MATCH", listFromValue(e.Arg("expressions")), "(", ")", true) +
		" AGAINST(" + g.sqlKey(e, "this") + modifier + ")"
}

func init() {
	dispatch[expressions.KindGlob] = (*Generator).globSQL
	dispatch[expressions.KindOverlaps] = (*Generator).overlapsSQL
	dispatch[expressions.KindRegexpLike] = (*Generator).regexpLikeSQL
	dispatch[expressions.KindRegexpILike] = (*Generator).regexpILikeSQL
	dispatch[expressions.KindAdjacent] = (*Generator).adjacentSQL
	dispatch[expressions.KindArrayContainsAll] = (*Generator).arrayContainsAllSQL
	dispatch[expressions.KindArrayContainedBy] = (*Generator).arrayContainedBySQL
	dispatch[expressions.KindArrayOverlaps] = (*Generator).arrayOverlapsSQL
	dispatch[expressions.KindJSONBContains] = (*Generator).jsonbContainsSQL
	dispatch[expressions.KindJSONBContainsAllTopKeys] = (*Generator).jsonbContainsAllTopKeysSQL
	dispatch[expressions.KindJSONBContainsAnyTopKeys] = (*Generator).jsonbContainsAnyTopKeysSQL
	dispatch[expressions.KindJSONBDeleteAtPath] = (*Generator).jsonbDeleteAtPathSQL
	dispatch[expressions.KindJSONBPathExists] = (*Generator).jsonbPathExistsSQL
	dispatch[expressions.KindJSON] = (*Generator).jsonSQL
	dispatch[expressions.KindOperator] = (*Generator).operatorSQL
	dispatch[expressions.KindMatchAgainst] = (*Generator).matchAgainstSQL
	dispatch[expressions.KindJSONArrayContains] = (*Generator).jsonArrayContainsSQL
}
