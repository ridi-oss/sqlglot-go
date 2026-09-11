package generator

import "github.com/ridi-oss/sqlglot-go/expressions"

// matchRecognizeMeasureSQL ports matchrecognizemeasure_sql (generator.py:3234-3240).
func (g *Generator) matchRecognizeMeasureSQL(e expressions.Expression) string {
	frame := g.sqlKey(e, "window_frame")
	if frame != "" {
		frame += " "
	}
	return frame + g.sqlKey(e, "this")
}

// matchRecognizeSQL ports matchrecognize_sql (generator.py:3242-3272).
func (g *Generator) matchRecognizeSQL(e expressions.Expression) string {
	partition := g.partitionBySQL(e)
	order := g.sqlKey(e, "order")
	measures := g.expressions(exprsOptions{expression: e, key: "measures"})
	if measures != "" {
		measures = g.seg("MEASURES" + g.seg(measures))
	}
	rows := g.sqlKey(e, "rows")
	if rows != "" {
		rows = g.seg(rows)
	}
	after := g.sqlKey(e, "after")
	if after != "" {
		after = g.seg(after)
	}
	pattern := g.sqlKey(e, "pattern")
	if pattern != "" {
		pattern = g.seg("PATTERN (" + pattern + ")")
	}
	var definitions []any
	for _, definition := range listFromValue(e.Arg("define")) {
		if d := asExpression(definition); d != nil {
			definitions = append(definitions, g.sqlKey(d, "alias")+" AS "+g.sqlKey(d, "this"))
		}
	}
	define := g.expressions(exprsOptions{sqls: definitions})
	if define != "" {
		define = g.seg("DEFINE" + g.seg(define))
	}
	body := partition + order + measures + rows + after + pattern + define
	alias := g.sqlKey(e, "alias")
	if alias != "" {
		alias = " " + alias
	}
	return g.seg("MATCH_RECOGNIZE") + " " + g.wrap(body) + alias
}

func init() {
	dispatch[expressions.KindMatchRecognize] = (*Generator).matchRecognizeSQL
	dispatch[expressions.KindMatchRecognizeMeasure] = (*Generator).matchRecognizeMeasureSQL
}
