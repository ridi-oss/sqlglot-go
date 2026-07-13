package generator

import "github.com/ridi/sqlglot-go/expressions"

// groupConcatSQL ports groupconcat_sql (dialects/dialect.py:2423-2467), dialect-gated per
// its only override in this codebase's scope, generators/postgres.py:313-315: postgres
// renders exp.GroupConcat as STRING_AGG(..., within_group=False); base and mysql keep the
// pre-dispatch-entry GROUP_CONCAT(...) rendering via functionFallbackSQL (this dispatch
// entry didn't exist before - Kind.Is(TraitFunc) already routed there automatically, see
// genWithComment's fallback branch).
//
// The within_group=True branch upstream keeps for e.g. Oracle/Snowflake's LISTAGG (which
// wraps the call in exp.WithinGroup instead of splicing the ORDER BY into the arg list) is
// out of this part's scope: postgres is the only caller wired up below and it always passes
// within_group=False, so that branch is elided.
func (g *Generator) groupConcatSQL(e expressions.Expression) string {
	if g.dialect.Name == "mysql" {
		// generators/mysql.py:174-175: GROUP_CONCAT({this} SEPARATOR {sep or "','"}).
		// `this` already encodes any DISTINCT/CONCAT/ORDER BY shape built by
		// parser.parseGroupConcat, so it renders itself; the separator defaults to a comma
		// literal when GroupConcat has no explicit "separator" arg.
		separator := g.sqlKey(e, "separator")
		if separator == "" {
			separator = "','"
		}
		return "GROUP_CONCAT(" + g.sqlKey(e, "this") + " SEPARATOR " + separator + ")"
	}
	if g.dialect.Name != "postgres" {
		return g.functionFallbackSQL(e)
	}

	this := asExpression(e.Arg("this"))

	// separator defaults to a comma literal (sep="," upstream) when GroupConcat has no
	// explicit "separator" arg.
	separatorExpr := asExpression(e.Arg("separator"))
	if separatorExpr == nil {
		separatorExpr = expressions.LiteralString(",")
	}
	separator := g.gen(separatorExpr)

	// STRING_AGG(x, ',' ORDER BY y LIMIT n) parses "this" as exp.Limit(this=Order(this=x,
	// ...)); pop the Limit's own "this" out so `limit` renders as a trailing LIMIT clause
	// below instead of nesting inside the arg list.
	var limit expressions.Expression
	if this != nil && this.Kind() == expressions.KindLimit && this.This() != nil {
		limit = this
		this = this.This().Pop()
	}

	// `this` may itself be the Order node (STRING_AGG(x, ',' ORDER BY y)) or wrap DISTINCT
	// under an Order (STRING_AGG(DISTINCT x, ',' ORDER BY y)); either way, pop the Order's
	// own "this" out so it renders as a trailing ORDER BY clause below.
	var order expressions.Expression
	if this != nil {
		order = this.Find(expressions.KindOrder)
	}
	if order != nil && order.This() != nil {
		this = order.This().Pop()
	}

	args := g.formatArgs([]any{this, separator}, ", ")

	modifiers := g.gen(limit)
	if order != nil {
		modifiers = g.gen(order) + modifiers
	}

	finalArgs := args
	if modifiers != "" {
		finalArgs = args + modifiers
	}
	return g.funcCall("STRING_AGG", []any{finalArgs}, "(", ")", true)
}

// logicalOrSQL renders exp.LogicalOr under its dialect-specific rename: postgres ->
// BOOL_OR (generators/postgres.py:332), mysql -> MAX (generators/mysql.py:180); base has no
// override, so it keeps the class's default name (LogicalOr._sql_names[0] = "LOGICAL_OR",
// aggregate.py:172) via functionFallbackSQL. The renamed branches gather their args via the
// shared fallbackArgs helper (the same path varianceSQL/variancePopSQL use), so all three
// dialect renames collect arguments identically.
func (g *Generator) logicalOrSQL(e expressions.Expression) string {
	switch g.dialect.Name {
	case "postgres":
		return g.funcCall("BOOL_OR", g.fallbackArgs(e), "(", ")", true)
	case "mysql":
		return g.funcCall("MAX", g.fallbackArgs(e), "(", ")", true)
	default:
		return g.functionFallbackSQL(e)
	}
}

func init() {
	dispatch[expressions.KindGroupConcat] = (*Generator).groupConcatSQL
	dispatch[expressions.KindLogicalOr] = (*Generator).logicalOrSQL
}
