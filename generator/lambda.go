package generator

import "github.com/ridi/sqlglot-go/expressions"

// lambdaSQL ports lambda_sql (generator.py:2861-2864): the flat, comma-joined parameter
// list - parenthesized when there's more than one - followed by " -> " and the lambda
// body. Every dialect in this codebase's scope (base/mysql/postgres) uses this directly
// (no dialect calls lambda_sql with a non-default arrow_sep/wrap, so those upstream
// parameters aren't ported).
func (g *Generator) lambdaSQL(e expressions.Expression) string {
	args := g.expressions(exprsOptions{expression: e, flat: true})
	// Wrap the parameter list in parens iff there's more than one parameter (upstream
	// lambda_sql: `f"({args})" if wrap and len(expression.expressions) > 1`); wrap defaults
	// to True and no base/mysql/postgres caller overrides it.
	if len(listFromValue(e.Arg("expressions"))) > 1 {
		args = "(" + args + ")"
	}
	return args + " -> " + g.sqlKey(e, "this")
}

func init() {
	dispatch[expressions.KindLambda] = (*Generator).lambdaSQL
}
