package generator

import "github.com/ridi-oss/sqlglot-go/expressions"

// parseJSONSQL ports parsejson_sql (generator.py:5548-5552) with the per-dialect
// PARSE_JSON_NAME: base/hive PARSE_JSON, presto JSON_PARSE, mysql none (bare value), and
// postgres's CAST(x AS JSON) transform (generators/postgres.py:353).
func (g *Generator) parseJSONSQL(e expressions.Expression) string {
	switch {
	case g.dialect.Name == "mysql":
		return g.sqlKey(e, "this")
	case g.dialect.Name == "postgres":
		return "CAST(" + g.sqlKey(e, "this") + " AS JSON)"
	case g.isDialect("presto"):
		return g.funcCall("JSON_PARSE", []any{e.Arg("this"), e.Arg("expression")}, "(", ")", true)
	}
	return g.funcCall("PARSE_JSON", []any{e.Arg("this"), e.Arg("expression")}, "(", ")", true)
}

// regexpReplaceSQL ports regexpreplace_sql (generator.py) / regexp_replace_sql (dialect.py:1868-1871).
func (g *Generator) regexpReplaceSQL(e expressions.Expression) string {
	return g.funcCall("REGEXP_REPLACE", []any{e.Arg("this"), e.Arg("expression"), e.Arg("replacement"), e.Arg("position"), e.Arg("occurrence"), e.Arg("modifiers")}, "(", ")", true)
}

func init() {
	dispatch[expressions.KindParseJSON] = (*Generator).parseJSONSQL
	dispatch[expressions.KindRegexpReplace] = (*Generator).regexpReplaceSQL
}
