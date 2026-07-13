package generator

import "github.com/ridi/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindTransaction] = (*Generator).transactionSQL
	dispatch[expressions.KindCommit] = (*Generator).commitSQL
	dispatch[expressions.KindRollback] = (*Generator).rollbackSQL
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
