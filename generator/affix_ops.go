package generator

import "github.com/ridi/sqlglot-go/expressions"

// collateSQL ports generator.py:4102-4105 collate_sql. Upstream gates the alternate
// function-call form (`COLLATE(this, expr)`) behind COLLATE_IS_FUNC, which is False for base
// and not overridden by mysql/postgres (only e.g. duckdb sets it True, out of scope here), so
// this always renders the binary `this COLLATE expression` form.
func (g *Generator) collateSQL(e expressions.Expression) string { return g.binary(e, "COLLATE") }

// bitwiseNotSQL ports generator.py:4063-4064 bitwisenot_sql: a plain `~this` prefix, no
// dedicated dialect override in base/mysql/postgres.
func (g *Generator) bitwiseNotSQL(e expressions.Expression) string {
	return "~" + g.sqlKey(e, "this")
}

// bitwiseLeftShiftSQL/bitwiseRightShiftSQL port generator.py:4060-4061/4069-4070.
func (g *Generator) bitwiseLeftShiftSQL(e expressions.Expression) string  { return g.binary(e, "<<") }
func (g *Generator) bitwiseRightShiftSQL(e expressions.Expression) string { return g.binary(e, ">>") }

// xorSQL ports generator.py:4020-4021 xor_sql: like and_sql/or_sql it forwards to the shared
// connectorSQL tree-flattening helper, whose per-node op lookup (connectorOp, generator/sql.go)
// dispatches KindAnd/KindOr/KindXor generically. This renders `this XOR expression` for a
// top-level Xor and, because exp.Xor carries TraitConnector (matching upstream
// `class Xor(Expression, Connector, Func)`), correctly renders XOR even when nested inside an
// outer AND/OR chain (e.g. mysql `a AND b XOR c`).
func (g *Generator) xorSQL(e expressions.Expression) string { return g.connectorSQL(e, "XOR", nil) }

func init() {
	dispatch[expressions.KindCollate] = (*Generator).collateSQL
	dispatch[expressions.KindBitwiseNot] = (*Generator).bitwiseNotSQL
	dispatch[expressions.KindBitwiseLeftShift] = (*Generator).bitwiseLeftShiftSQL
	dispatch[expressions.KindBitwiseRightShift] = (*Generator).bitwiseRightShiftSQL
	dispatch[expressions.KindXor] = (*Generator).xorSQL
	// KindCbrt intentionally has no dispatch entry: upstream defines no cbrt_sql override in
	// base/mysql/postgres, so it falls through to functionFallbackSQL (CBRT(...)), matching
	// e.g. KindReplace/KindSqrt above.
}
