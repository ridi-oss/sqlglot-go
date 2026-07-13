package generator

import "github.com/ridi/sqlglot-go/expressions"

// substringSQL ports _substring_sql (generators/postgres.py:106-114). Postgres always
// renders the ANSI SUBSTRING(<this> FROM <start> FOR <length>) form, and always in that
// FROM-then-FOR key order, regardless of how the source SQL ordered the two clauses -
// parser/parser_functions.go:67-90 (parseSubstring) already normalizes a bare `FOR n`
// into start=1, so no parser change is needed here (parity_gaps.txt: SUBSTRING('Thomas'
// FOR 3 FROM 2) -> SUBSTRING('Thomas' FROM 2 FOR 3); SUBSTRING('afafa' for 1) ->
// SUBSTRING('afafa' FROM 1 FOR 1)).
//
// Every other dialect (including base and mysql) keeps the comma-form via
// functionFallbackSQL, which is also what KindSubstring rendered before this dispatch
// entry existed (falling through to the TraitFunc fallback in genWithComment), so base and
// mysql SUBSTR/SUBSTRING identity fixtures are unaffected.
func (g *Generator) substringSQL(e expressions.Expression) string {
	if g.dialect.Name != "postgres" {
		return g.functionFallbackSQL(e)
	}

	this := g.sqlKey(e, "this")
	start := g.sqlKey(e, "start")
	length := g.sqlKey(e, "length")

	fromPart := ""
	if start != "" {
		fromPart = " FROM " + start
	}
	forPart := ""
	if length != "" {
		forPart = " FOR " + length
	}

	return "SUBSTRING(" + this + fromPart + forPart + ")"
}

func init() {
	dispatch[expressions.KindSubstring] = (*Generator).substringSQL
}
