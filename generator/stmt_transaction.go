package generator

import "github.com/ridi-oss/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindTransaction] = (*Generator).transactionSQL
	dispatch[expressions.KindCommit] = (*Generator).commitSQL
	dispatch[expressions.KindRollback] = (*Generator).rollbackSQL
	dispatch[expressions.KindSavepoint] = (*Generator).savepointSQL
	dispatch[expressions.KindReset] = (*Generator).resetSQL
}

// resetSQL renders Postgres `RESET <name>` (name captured verbatim, so `ALL` and special phrases like
// `TIME ZONE` round-trip). Upstream has no Reset node (it Commands RESET), so there is no upstream
// generator to port. See DEVIATIONS.
func (g *Generator) resetSQL(e expressions.Expression) string {
	return "RESET " + e.Text("this")
}

// savepointSQL renders the transaction-control savepoint statements: `SAVEPOINT <name>` and, when
// the kind arg is "RELEASE", `RELEASE SAVEPOINT <name>` (the bare Postgres `RELEASE <name>` spelling
// normalizes to the explicit SAVEPOINT keyword, which Postgres also accepts). Upstream has no
// savepoint node, so there is no upstream generator to port. See DEVIATIONS.
func (g *Generator) savepointSQL(e expressions.Expression) string {
	name := g.sqlKey(e, "this")
	if kind, _ := e.Arg("kind").(string); kind == "RELEASE" {
		return "RELEASE SAVEPOINT " + name
	}
	return "SAVEPOINT " + name
}

// transactionSQL ports generator.py:4140-4143 (transaction_sql). Note upstream never
// renders "this" (the sqlite-only DEFERRED/IMMEDIATE/EXCLUSIVE transaction kind) here.
func (g *Generator) transactionSQL(e expressions.Expression) string {
	modes := g.expressions(exprsOptions{expression: e, key: "modes", flat: true})
	if modes != "" {
		modes = " " + modes
	}
	return "BEGIN" + modes
}

// commitSQL ports generator.py:4145-4150 (commit_sql).
func (g *Generator) commitSQL(e expressions.Expression) string {
	chain := ""
	if v, ok := e.Arg("chain").(bool); ok {
		if v {
			chain = " AND CHAIN"
		} else {
			chain = " AND NO CHAIN"
		}
	}
	return "COMMIT" + chain
}

// rollbackSQL ports generator.py:4152-4156 (rollback_sql).
func (g *Generator) rollbackSQL(e expressions.Expression) string {
	savepoint := g.sqlKey(e, "savepoint")
	if savepoint != "" {
		savepoint = " TO " + savepoint
	}
	return "ROLLBACK" + savepoint
}
