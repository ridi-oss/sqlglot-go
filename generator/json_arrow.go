package generator

import "github.com/ridi/sqlglot-go/expressions"

// jsonExtractArgs returns the positional argument list for the *function* rendering of an
// exp.JSONExtract/JSONExtractScalar node: `this`, `expression`, then the variadic `expressions`
// tail. The tail is populated only for JSONExtract parsed from the 3+-arg function form
// (jsonExtractFunction, expressions/functions.go) - e.g. JSON_EXTRACT(a, '$.b', '$.c') - because
// upstream build_extract_json_with_path (parser.py:104-118) drops the tail for JSONExtractScalar.
// Arrow-created nodes and scalar nodes only set this/expression, so the tail is empty and this
// collapses to the two-operand list the operator branches also use.
func jsonExtractArgs(e expressions.Expression) []any {
	return append([]any{e.Arg("this"), e.Arg("expression")}, listFromValue(e.Arg("expressions"))...)
}

// This file ports the JSON `->`/`->>`/`#>`/`#>>` rendering for exp.JSONExtract/
// JSONExtractScalar/JSONBExtract/JSONBExtractScalar. Upstream drives this via json_extract_
// segments (dialect.py:2133-2158), which renders the operator form by walking an exp.JSONPath's
// segments. This port never builds a JSONPath (the RHS stays the raw literal/expression parsed
// at parser/parser.go's columnOperators ARROW/DARROW - see that file's comment for why), so
// json_extract_segments' "path isn't a JSONPath" fallback (rename_func) would always fire; these
// methods instead implement each dialect's TRANSFORMS entry directly against the simplified
// only_json_types signal, which is equivalent for every corpus case (see ROADMAP's JSONPath
// deferral note).

// jsonExtractSQL ports the base/mysql default (functionFallbackSQL - neither dialect overrides
// exp.JSONExtract in TRANSFORMS), postgres's _json_extract_sql("JSON_EXTRACT_PATH", "->")
// (generators/postgres.py:322), and Trino's JSON_QUERY-specific override
// (generators/trino.py:50-68). The Postgres branch stays first so its existing extraction
// spelling remains authoritative even if a caller supplies a JSONExtract with json_query set.
func (g *Generator) jsonExtractSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		if boolValue(e.Arg("only_json_types")) {
			return g.binary(e, "->")
		}
		return g.funcCall("JSON_EXTRACT_PATH", jsonExtractArgs(e), "(", ")", true)
	}
	if (g.dialect.Name == "trino" || g.dialect.Name == "athena") && boolValue(e.Arg("json_query")) {
		path := g.sqlKey(e, "expression")

		option := g.sqlKey(e, "option")
		if option != "" {
			option = " " + option
		}

		quote := g.sqlKey(e, "quote")
		if quote != "" {
			quote = " " + quote
		}

		onCondition := g.sqlKey(e, "on_condition")
		if onCondition != "" {
			onCondition = " " + onCondition
		}

		return g.funcCall("JSON_QUERY", []any{e.Arg("this"), path + option + quote + onCondition}, "(", ")", true)
	}
	return g.funcCall("JSON_EXTRACT", jsonExtractArgs(e), "(", ")", true)
}

// jsonExtractScalarSQL ports base's default (functionFallbackSQL), mysql's
// exp.JSONExtractScalar: arrow_json_extract_sql (generators/mysql.py:178, dialect.py:1210-1215),
// and postgres's _json_extract_sql("JSON_EXTRACT_PATH_TEXT", "->>") (generators/postgres.py:323)
// - postgres's own TRANSFORMS entry takes precedence over the generic JSON_TYPE_REQUIRED_FOR_
// EXTRACTION-gated arrow_json_extract_sql helper mysql shares, exactly mirroring upstream's
// per-dialect TRANSFORMS dict precedence.
func (g *Generator) jsonExtractScalarSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		if boolValue(e.Arg("only_json_types")) {
			return g.binary(e, "->>")
		}
		return g.funcCall("JSON_EXTRACT_PATH_TEXT", jsonExtractArgs(e), "(", ")", true)
	}
	if g.dialect.JSONTypeRequiredForExtraction {
		return g.arrowJSONExtractScalarSQL(e)
	}
	return g.funcCall("JSON_EXTRACT_SCALAR", jsonExtractArgs(e), "(", ")", true)
}

// arrowJSONExtractScalarSQL ports arrow_json_extract_sql (dialect.py:1210-1215): a string-
// literal `this` is wrapped in CAST(... AS JSON) before rendering the `->>` operator form.
func (g *Generator) arrowJSONExtractScalarSQL(e expressions.Expression) string {
	if this := asExpression(e.Arg("this")); this != nil && this.Kind() == expressions.KindLiteral && this.IsString() {
		cast := expressions.Cast(expressions.Args{
			"this": this,
			"to":   expressions.DataType(expressions.Args{"this": expressions.DTypeJSON}),
		})
		e.Set("this", cast)
	}
	return g.binary(e, "->>")
}

// jsonbExtractSQL ports base/mysql's default (functionFallbackSQL - JSONB_EXTRACT) and
// postgres's `lambda self, e: self.binary(e, "#>")` (generators/postgres.py:324).
func (g *Generator) jsonbExtractSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		return g.binary(e, "#>")
	}
	return g.funcCall("JSONB_EXTRACT", []any{e.Arg("this"), e.Arg("expression")}, "(", ")", true)
}

// jsonbExtractScalarSQL ports base/mysql's default (functionFallbackSQL - JSONB_EXTRACT_SCALAR)
// and postgres's `lambda self, e: self.binary(e, "#>>")` (generators/postgres.py:325).
func (g *Generator) jsonbExtractScalarSQL(e expressions.Expression) string {
	if g.dialect.Name == "postgres" {
		return g.binary(e, "#>>")
	}
	return g.funcCall("JSONB_EXTRACT_SCALAR", []any{e.Arg("this"), e.Arg("expression")}, "(", ")", true)
}

func init() {
	dispatch[expressions.KindJSONExtract] = (*Generator).jsonExtractSQL
	dispatch[expressions.KindJSONExtractScalar] = (*Generator).jsonExtractScalarSQL
	dispatch[expressions.KindJSONBExtract] = (*Generator).jsonbExtractSQL
	dispatch[expressions.KindJSONBExtractScalar] = (*Generator).jsonbExtractScalarSQL
}
