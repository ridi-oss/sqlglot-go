package generator

import "github.com/ridi/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindAnalyze] = (*Generator).analyzeSQL
	dispatch[expressions.KindAnalyzeHistogram] = (*Generator).analyzeHistogramSQL
	dispatch[expressions.KindAnalyzeWith] = (*Generator).analyzeWithSQL
	dispatch[expressions.KindUsingData] = (*Generator).usingDataSQL
}

// analyzeSQL ports generator.py:5947-5962 (analyze_sql).
func (g *Generator) analyzeSQL(e expressions.Expression) string {
	options := g.expressions(exprsOptions{expression: e, key: "options", sep: " "})
	if options != "" {
		options = " " + options
	}
	kind := g.sqlKey(e, "kind")
	if kind != "" {
		kind = " " + kind
	}
	this := g.sqlKey(e, "this")
	if this != "" {
		this = " " + this
	}
	mode := g.sqlKey(e, "mode")
	if mode != "" {
		mode = " " + mode
	}
	properties := g.sqlKey(e, "properties")
	if properties != "" {
		properties = " " + properties
	}
	partition := g.sqlKey(e, "partition")
	if partition != "" {
		partition = " " + partition
	}
	innerExpression := g.sqlKey(e, "expression")
	if innerExpression != "" {
		innerExpression = " " + innerExpression
	}
	return "ANALYZE" + options + kind + this + partition + mode + innerExpression + properties
}

// analyzeHistogramSQL ports generator.py:5922-5929 (analyzehistogram_sql).
func (g *Generator) analyzeHistogramSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	columns := g.expressions(exprsOptions{expression: e})
	innerExpression := g.sqlKey(e, "expression")
	if innerExpression != "" {
		innerExpression = " " + innerExpression
	}
	updateOptions := g.sqlKey(e, "update_options")
	if updateOptions != "" {
		updateOptions = " " + updateOptions + " UPDATE"
	}
	return this + " HISTOGRAM ON " + columns + innerExpression + updateOptions
}

// analyzeWithSQL ports generator.py:142's AnalyzeWith transform.
func (g *Generator) analyzeWithSQL(e expressions.Expression) string {
	return g.expressions(exprsOptions{expression: e, prefix: "WITH ", sep: " "})
}

// usingDataSQL ports generator.py:279's UsingData transform.
func (g *Generator) usingDataSQL(e expressions.Expression) string {
	return "USING DATA " + g.sqlKey(e, "this")
}
