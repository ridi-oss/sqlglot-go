package generator

import "github.com/sjincho/sqlglot-go/expressions"

func init() {
	dispatch[expressions.KindAnalyze] = (*Generator).analyzeSQL
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
