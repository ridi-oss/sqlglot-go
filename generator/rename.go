package generator

import "github.com/ridi/sqlglot-go/expressions"

// varianceSQL ports rename_func("VAR_SAMP") applied to exp.Variance
// (generators/postgres.py:384). Base and mysql keep the default VARIANCE name via
// functionFallbackSQL, which is also what KindVariance rendered before this dispatch entry
// existed, so base/mysql VARIANCE(x) identity fixtures are unaffected.
func (g *Generator) varianceSQL(e expressions.Expression) string {
	if g.dialect.Name != "postgres" {
		return g.functionFallbackSQL(e)
	}
	return g.funcCall("VAR_SAMP", g.fallbackArgs(e), "(", ")", true)
}

// variancePopSQL ports rename_func("VAR_POP") applied to exp.VariancePop
// (generators/postgres.py:383). Base and mysql keep VARIANCE_POP via functionFallbackSQL.
func (g *Generator) variancePopSQL(e expressions.Expression) string {
	if g.dialect.Name != "postgres" {
		return g.functionFallbackSQL(e)
	}
	return g.funcCall("VAR_POP", g.fallbackArgs(e), "(", ")", true)
}

func init() {
	dispatch[expressions.KindVariance] = (*Generator).varianceSQL
	dispatch[expressions.KindVariancePop] = (*Generator).variancePopSQL
}
