package generator

import (
	"strings"

	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// BigQuery overrides follow generators/bigquery.py:248-733.
func init() {
	registerDialectTypeMapping("bigquery", map[exp.DType]string{
		exp.DTypeBigDecimal: "BIGNUMERIC", exp.DTypeBigInt: "INT64", exp.DTypeBinary: "BYTES",
		exp.DTypeBlob: "BYTES", exp.DTypeBoolean: "BOOL", exp.DTypeChar: "STRING",
		exp.DTypeDecimal: "NUMERIC", exp.DTypeDouble: "FLOAT64", exp.DTypeFloat: "FLOAT64",
		exp.DTypeInt: "INT64", exp.DTypeNChar: "STRING", exp.DTypeNVarchar: "STRING",
		exp.DTypeSmallInt: "INT64", exp.DTypeText: "STRING", exp.DTypeTimestamp: "DATETIME",
		exp.DTypeTimestampNtz: "DATETIME", exp.DTypeTimestampTz: "TIMESTAMP",
		exp.DTypeTimestampLtz: "TIMESTAMP", exp.DTypeTinyInt: "INT64", exp.DTypeRowVersion: "BYTES",
		exp.DTypeUUID: "STRING", exp.DTypeVarBinary: "BYTES", exp.DTypeVarchar: "STRING",
		exp.DTypeVariant: "ANY TYPE",
	})
	registerDialectPropertyLocations("bigquery", map[exp.Kind]propertyLocation{
		exp.KindPartitionedByProperty: propertyLocationPostSchema,
		exp.KindVolatileProperty:      propertyLocationUnsupported,
	})
	rename := prestoRename
	call := prestoCall
	registerDialectDispatch("bigquery", map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindDeclare:        (*Generator).declareDefaultSQL,
		exp.KindDeclareItem:    (*Generator).declareItemDefaultSQL,
		exp.KindAIEmbed:        rename("EMBED"),
		exp.KindAIGenerate:     rename("GENERATE"),
		exp.KindAISimilarity:   rename("SIMILARITY"),
		exp.KindApproxTopK:     rename("APPROX_TOP_COUNT"),
		exp.KindApproxDistinct: rename("APPROX_COUNT_DISTINCT"),
		exp.KindArgMax:         call("MAX_BY", "this", "expression"),
		exp.KindArgMin:         call("MIN_BY", "this", "expression"),
		exp.KindArray:          (*Generator).bigqueryArraySQL,
		exp.KindArrayContains:  (*Generator).bigqueryArrayContainsSQL,
		exp.KindBitwiseAndAgg:  rename("BIT_AND"),
		exp.KindBitwiseOrAgg:   rename("BIT_OR"),
		exp.KindBitwiseXorAgg:  rename("BIT_XOR"),
		exp.KindBitwiseCount:   rename("BIT_COUNT"),
		exp.KindByteLength:     rename("BYTE_LENGTH"),
		exp.KindCollateProperty: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("default")) {
				return "DEFAULT COLLATE " + g.sqlKey(e, "this")
			}
			return "COLLATE " + g.sqlKey(e, "this")
		},
		exp.KindCommit:  func(*Generator, exp.Expression) string { return "COMMIT TRANSACTION" },
		exp.KindCountIf: rename("COUNTIF"),
		exp.KindDateAdd: bigqueryDateAddInterval("DATE", "ADD"),
		exp.KindDateDiff: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE_DIFF", []any{e.Arg("this"), e.Arg("expression"), unitToVarValue(asExpression(e.Arg("unit")))}, "(", ")", true)
		},
		exp.KindDateFromParts:  rename("DATE"),
		exp.KindDateSub:        bigqueryDateAddInterval("DATE", "SUB"),
		exp.KindDatetimeAdd:    bigqueryDateAddInterval("DATETIME", "ADD"),
		exp.KindDatetimeSub:    bigqueryDateAddInterval("DATETIME", "SUB"),
		exp.KindGenerateSeries: call("GENERATE_ARRAY", "start", "end", "step"),
		exp.KindHex: func(g *Generator, e exp.Expression) string {
			return g.funcCall("UPPER", []any{g.funcCall("TO_HEX", []any{g.sqlKey(e, "this")}, "(", ")", true)}, "(", ")", true)
		},
		exp.KindIf: func(g *Generator, e exp.Expression) string {
			falseValue := e.Arg("false")
			if !truthy(falseValue) {
				falseValue = "NULL"
			}
			return g.funcCall("IF", []any{e.Arg("this"), e.Arg("true"), falseValue}, "(", ")", true)
		},
		exp.KindILike: func(g *Generator, e exp.Expression) string {
			return g.gen(exp.New(exp.KindLike, exp.Args{
				"this":       exp.New(exp.KindLower, exp.Args{"this": e.Arg("this")}),
				"expression": exp.New(exp.KindLower, exp.Args{"this": e.Arg("expression")}),
				"negate":     e.Arg("negate"),
			}))
		},
		exp.KindIntDiv: rename("DIV"),
		// generator.py:2024-2028 with HEX_FUNC = "TO_HEX" and HEX_LOWERCASE = True.
		exp.KindLowerHex:          call("TO_HEX", "this"),
		exp.KindInt64:             rename("INT64"),
		exp.KindJSONBool:          rename("BOOL"),
		exp.KindJSONExtract:       (*Generator).bigqueryJSONExtractSQL,
		exp.KindJSONExtractArray:  (*Generator).bigqueryJSONExtractSQL,
		exp.KindJSONExtractScalar: (*Generator).bigqueryJSONExtractSQL,
		exp.KindJSONFormat: func(g *Generator, e exp.Expression) string {
			name := "TO_JSON_STRING"
			if truthy(e.Arg("to_json")) {
				name = "TO_JSON"
			}
			return g.funcCall(name, []any{e.Arg("this"), e.Arg("options")}, "(", ")", true)
		},
		exp.KindJSONKeysAtDepth: rename("JSON_KEYS"),
		exp.KindJSONValueArray:  rename("JSON_VALUE_ARRAY"),
		exp.KindLevenshtein: func(g *Generator, e exp.Expression) string {
			var maxDist any
			if m := e.Arg("max_dist"); truthy(m) {
				maxDist = exp.New(exp.KindKwarg, exp.Args{"this": exp.Var(exp.Args{"this": "max_distance"}), "expression": m})
			}
			return g.funcCall("EDIT_DISTANCE", []any{e.Arg("this"), e.Arg("expression"), maxDist}, "(", ")", true)
		},
		exp.KindMax: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("expressions")) {
				return rename("GREATEST")(g, e)
			}
			return rename("MAX")(g, e)
		},
		exp.KindMD5: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TO_HEX", []any{g.funcCall("MD5", []any{e.Arg("this")}, "(", ")", true)}, "(", ")", true)
		},
		exp.KindMD5Digest: rename("MD5"),
		exp.KindMin: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("expressions")) {
				return rename("LEAST")(g, e)
			}
			return rename("MIN")(g, e)
		},
		exp.KindMod: (*Generator).bigqueryModSQL,
		// generator.py:4620-4625 with LOG_BASE_FIRST = False.
		exp.KindLog: func(g *Generator, e exp.Expression) string {
			if truthy(e.Arg("expression")) {
				return g.funcCall("LOG", []any{e.Arg("expression"), e.Arg("this")}, "(", ")", true)
			}
			return g.funcCall("LOG", []any{e.Arg("this")}, "(", ")", true)
		},
		exp.KindNormalize: func(g *Generator, e exp.Expression) string {
			name := "NORMALIZE"
			if truthy(e.Arg("is_casefold")) {
				name = "NORMALIZE_AND_CASEFOLD"
			}
			return g.funcCall(name, []any{e.Arg("this"), e.Arg("form")}, "(", ")", true)
		},
		exp.KindPartitionedByProperty: func(g *Generator, e exp.Expression) string { return "PARTITION BY " + g.sqlKey(e, "this") },
		exp.KindRegexpExtract:         call("REGEXP_EXTRACT", "this", "expression", "position", "occurrence"),
		exp.KindRegexpExtractAll:      call("REGEXP_EXTRACT_ALL", "this", "expression"),
		exp.KindRegexpLike:            rename("REGEXP_CONTAINS"),
		exp.KindRollback:              func(*Generator, exp.Expression) string { return "ROLLBACK TRANSACTION" },
		exp.KindParseTime: func(g *Generator, e exp.Expression) string {
			return g.funcCall("PARSE_TIME", []any{bigqueryFormatTime(g, e, false), e.Arg("this")}, "(", ")", true)
		},
		exp.KindParseDatetime: func(g *Generator, e exp.Expression) string {
			return g.funcCall("PARSE_DATETIME", []any{bigqueryFormatTime(g, e, false), e.Arg("this")}, "(", ")", true)
		},
		exp.KindSHA:        rename("SHA1"),
		exp.KindSHA1Digest: rename("SHA1"),
		exp.KindSHA2Digest: func(g *Generator, e exp.Expression) string {
			length := e.Text("length")
			if length == "" {
				length = "256"
			}
			return g.funcCall("SHA"+length, []any{e.Arg("this")}, "(", ")", true)
		},
		exp.KindStabilityProperty: func(_ *Generator, e exp.Expression) string {
			if e.Name() == "IMMUTABLE" {
				return "DETERMINISTIC"
			}
			return "NOT DETERMINISTIC"
		},
		exp.KindStrToDate:          (*Generator).bigqueryStrToDatetimeSQL,
		exp.KindStrToTime:          (*Generator).bigqueryStrToDatetimeSQL,
		exp.KindSessionUser:        func(*Generator, exp.Expression) string { return "SESSION_USER()" },
		exp.KindTimeAdd:            bigqueryDateAddInterval("TIME", "ADD"),
		exp.KindTimeFromParts:      rename("TIME"),
		exp.KindTimestampFromParts: rename("DATETIME"),
		exp.KindTimeSub:            bigqueryDateAddInterval("TIME", "SUB"),
		exp.KindTimestampAdd:       bigqueryDateAddInterval("TIMESTAMP", "ADD"),
		exp.KindTimestampDiff:      rename("TIMESTAMP_DIFF"),
		exp.KindTimestampSub:       bigqueryDateAddInterval("TIMESTAMP", "SUB"),
		exp.KindTransaction:        func(*Generator, exp.Expression) string { return "BEGIN TRANSACTION" },
		exp.KindTsOrDsToTime:       rename("TIME"),
		exp.KindTsOrDsToDatetime:   rename("DATETIME"),
		exp.KindTsOrDsToTimestamp:  rename("TIMESTAMP"),
		exp.KindUnhex:              rename("FROM_HEX"),
		exp.KindUnixDate:           rename("UNIX_DATE"),
		exp.KindUnixToTime:         (*Generator).bigqueryUnixToTimeSQL,
		exp.KindUuid:               func(*Generator, exp.Expression) string { return "GENERATE_UUID()" },
		exp.KindVariancePop:        rename("VAR_POP"),
		exp.KindSafeDivide:         rename("SAFE_DIVIDE"),
		exp.KindWeekStart: func(g *Generator, e exp.Expression) string {
			if this := asExpression(e.Arg("this")); this != nil && strings.ToUpper(this.Name()) == "SUNDAY" {
				return "WEEK"
			}
			return g.funcCall("WEEK", []any{e.Arg("this")}, "(", ")", true)
		},
		exp.KindDateTrunc: func(g *Generator, e exp.Expression) string {
			unit := e.Arg("unit")
			if u := asExpression(unit); u != nil && u.IsString() {
				unit = u.Name()
			}
			return g.funcCall("DATE_TRUNC", []any{e.Arg("this"), unit, e.Arg("zone")}, "(", ")", true)
		},
		exp.KindTimeToStr: (*Generator).bigqueryTimeToStrSQL,
		exp.KindCast:      func(g *Generator, e exp.Expression) string { return g.bigqueryCastSQL(e, "") },
		exp.KindTryCast: func(g *Generator, e exp.Expression) string {
			return g.bigqueryCastSQL(e, "SAFE_")
		},
		exp.KindContains: func(g *Generator, e exp.Expression) string {
			this, expr := asExpression(e.Arg("this")), asExpression(e.Arg("expression"))
			if this != nil && expr != nil && this.Kind() == exp.KindLower && expr.Kind() == exp.KindLower {
				this, expr = this.This(), expr.This()
			}
			return g.funcCall("CONTAINS_SUBSTR", []any{this, expr, e.Arg("json_scope")}, "(", ")", true)
		},
	})
}

// bigqueryDateAddInterval ports date_add_interval_sql (dialects/dialect.py:1691-1697).
func bigqueryDateAddInterval(dataType, kind string) func(*Generator, exp.Expression) string {
	return func(g *Generator, e exp.Expression) string {
		interval := exp.Interval(exp.Args{"this": e.Arg("expression"), "unit": unitToVarValue(asExpression(e.Arg("unit")))})
		return dataType + "_" + kind + "(" + g.sqlKey(e, "this") + ", " + g.gen(interval) + ")"
	}
}

// bigqueryArraySQL ports inline_array_unless_query (dialects/dialect.py:1262-1266).
func (g *Generator) bigqueryArraySQL(e exp.Expression) string {
	exprs := listFromValue(e.Arg("expressions"))
	if len(exprs) > 0 {
		if elem := asExpression(exprs[0]); elem != nil && containsQuery(elem) {
			return g.funcCall("ARRAY", []any{elem}, "(", ")", true)
		}
	}
	return g.inlineArraySQL(e)
}

func containsQuery(e exp.Expression) bool { return e.Find(exp.TraitQuery) != nil }

// generators/bigquery.py:168-175.
func (g *Generator) bigqueryArrayContainsSQL(e exp.Expression) string {
	left := g.sqlKey(e, "this")
	right := g.sqlKey(e, "expression")
	return "EXISTS(SELECT 1 FROM UNNEST(" + left + ") AS _col WHERE _col = " + right + ")"
}

// generators/bigquery.py:595-602.
func (g *Generator) bigqueryModSQL(e exp.Expression) string {
	unparen := func(key string) any {
		v := asExpression(e.Arg(key))
		if v != nil && v.Kind() == exp.KindParen {
			return unnestParen(v)
		}
		return v
	}
	return g.funcCall("MOD", []any{unparen("this"), unparen("expression")}, "(", ")", true)
}

func unnestParen(e exp.Expression) exp.Expression {
	for e != nil && e.Kind() == exp.KindParen {
		e = e.This()
	}
	return e
}

// generators/bigquery.py:231-245.
func (g *Generator) bigqueryJSONExtractSQL(e exp.Expression) string {
	name, _ := e.MetaGet("name").(string)
	if name == "" {
		name = g.sqlName(e.Kind())
	}
	return g.funcCall(strings.ToUpper(name), g.fallbackArgs(e), "(", ")", true)
}

func bigqueryFormatTime(g *Generator, e exp.Expression, cast bool) any {
	format := asExpression(e.Arg("format"))
	if format == nil || !format.IsString() {
		return format
	}
	return exp.LiteralString(dialects.BigQueryFormatTime(format.Name(), cast))
}

// generators/bigquery.py:206-219.
func (g *Generator) bigqueryStrToDatetimeSQL(e exp.Expression) string {
	this := g.sqlKey(e, "this")
	dtype := "TIMESTAMP"
	if e.Kind() == exp.KindStrToDate {
		dtype = "DATE"
	}
	if truthy(e.Arg("safe")) {
		return "SAFE_CAST(" + this + " AS " + dtype + " FORMAT " + g.gen(bigqueryFormatTime(g, e, true)) + ")"
	}
	return g.funcCall("PARSE_"+dtype, []any{bigqueryFormatTime(g, e, false), e.Arg("this"), e.Arg("zone")}, "(", ")", true)
}

// generators/bigquery.py:189-203.
func (g *Generator) bigqueryUnixToTimeSQL(e exp.Expression) string {
	scale := asExpression(e.Arg("scale"))
	switch {
	case scale == nil || scale.Name() == "0":
		return g.funcCall("TIMESTAMP_SECONDS", []any{e.Arg("this")}, "(", ")", true)
	case scale.Name() == "3":
		return g.funcCall("TIMESTAMP_MILLIS", []any{e.Arg("this")}, "(", ")", true)
	case scale.Name() == "6":
		return g.funcCall("TIMESTAMP_MICROS", []any{e.Arg("this")}, "(", ")", true)
	}
	div := exp.New(exp.KindDiv, exp.Args{"this": e.Arg("this"), "expression": exp.New(exp.KindAnonymous, exp.Args{"this": "POW", "expressions": []exp.Expression{exp.LiteralNumber("10"), scale}})})
	return g.funcCall("TIMESTAMP_SECONDS", []any{exp.New(exp.KindCast, exp.Args{"this": div, "to": exp.DataType(exp.Args{"this": exp.DTypeBigInt})})}, "(", ")", true)
}

// generators/bigquery.py:629-643.
func (g *Generator) bigqueryTimeToStrSQL(e exp.Expression) string {
	this := asExpression(e.Arg("this"))
	funcName := "FORMAT_DATE"
	timeExpr := e
	if this != nil {
		switch this.Kind() {
		case exp.KindTsOrDsToDatetime:
			funcName = "FORMAT_DATETIME"
		case exp.KindTsOrDsToTimestamp:
			funcName = "FORMAT_TIMESTAMP"
		case exp.KindTsOrDsToTime:
			funcName = "FORMAT_TIME"
		}
		switch this.Kind() {
		case exp.KindTsOrDsToDatetime, exp.KindTsOrDsToTimestamp, exp.KindTsOrDsToTime, exp.KindTsOrDsToDate:
			timeExpr = this
		}
	}
	return g.funcCall(funcName, []any{bigqueryFormatTime(g, e, false), timeExpr.Arg("this"), e.Arg("zone")}, "(", ")", true)
}

// generators/bigquery.py:604-612: `dataset.table`.name keeps the quoted table path.
func (g *Generator) bigqueryColumnParts(e exp.Expression) string {
	parts := e.Parts()
	names := make([]string, 0, len(parts))
	for _, part := range parts[:len(parts)-1] {
		names = append(names, part.Name())
	}
	path := g.gen(exp.Identifier(exp.Args{"this": strings.Join(names, "."), "quoted": true}))
	return path + "." + g.sqlKey(e, "this")
}

// generators/bigquery.py:614-627: `x.y` and `x`.`y` can resolve differently, so a table
// written as one quoted path is generated as one.
func (g *Generator) bigqueryTableParts(e exp.Expression) string {
	parts := e.Parts()
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		names = append(names, part.Name())
	}
	return g.gen(exp.Identifier(exp.Args{"this": strings.Join(names, "."), "quoted": true}))
}

func init() {
	// generator.py:3860-3868 jsonarray_sql.
	dispatch[exp.KindJSONArray] = func(g *Generator, e exp.Expression) string {
		suffix := ""
		if nh := e.Text("null_handling"); nh != "" {
			suffix += " " + nh
		}
		if rt := g.sqlKey(e, "return_type"); rt != "" {
			suffix += " RETURNING " + rt
		}
		if truthy(e.Arg("strict")) {
			suffix += " STRICT"
		}
		args := append([]any{}, listFromValue(e.Arg("expressions"))...)
		return g.funcCall("JSON_ARRAY", args, "(", suffix+")", true)
	}
}

// Base generator methods for nodes the BigQuery grammar introduces (generator.py).
func init() {
	dispatch[exp.KindLowerHex] = func(g *Generator, e exp.Expression) string {
		return g.funcCall("LOWER", []any{g.funcCall("HEX", []any{e.Arg("this")}, "(", ")", true)}, "(", ")", true)
	}
	dispatch[exp.KindPseudocolumn] = (*Generator).columnSQL
	dispatch[exp.KindKwarg] = func(g *Generator, e exp.Expression) string { return g.binary(e, "=>") }
	dispatch[exp.KindExport] = func(g *Generator, e exp.Expression) string {
		connection := g.sqlKey(e, "connection")
		if connection != "" {
			connection = "WITH CONNECTION " + connection + " "
		}
		return "EXPORT DATA " + connection + g.sqlKey(e, "options") + " AS " + g.sqlKey(e, "this")
	}
	dispatch[exp.KindForIn] = func(g *Generator, e exp.Expression) string {
		return "FOR " + g.sqlKey(e, "this") + " DO " + g.sqlKey(e, "expression")
	}
	dispatch[exp.KindHavingMax] = func(g *Generator, e exp.Expression) string {
		kind := "MIN"
		if truthy(e.Arg("max")) {
			kind = "MAX"
		}
		return g.sqlKey(e, "this") + " HAVING " + kind + " " + g.sqlKey(e, "expression")
	}
	dispatch[exp.KindSafeFunc] = func(g *Generator, e exp.Expression) string { return "SAFE." + g.sqlKey(e, "this") }
	dispatch[exp.KindNetFunc] = func(g *Generator, e exp.Expression) string { return "NET." + g.sqlKey(e, "this") }
	for kind, name := range map[exp.Kind]string{
		exp.KindPredict: "PREDICT", exp.KindGenerateText: "GENERATE_TEXT", exp.KindGenerateTable: "GENERATE_TABLE",
		exp.KindGenerateBool: "GENERATE_BOOL", exp.KindGenerateInt: "GENERATE_INT", exp.KindGenerateDouble: "GENERATE_DOUBLE",
		exp.KindMLTranslate: "TRANSLATE", exp.KindMLForecast: "FORECAST",
	} {
		name := name
		dispatch[kind] = func(g *Generator, e exp.Expression) string { return g.mlSQL(e, name) }
	}
	dispatch[exp.KindGenerateEmbedding] = func(g *Generator, e exp.Expression) string {
		if truthy(e.Arg("is_text")) {
			return g.mlSQL(e, "GENERATE_TEXT_EMBEDDING")
		}
		return g.mlSQL(e, "GENERATE_EMBEDDING")
	}
	dispatch[exp.KindAIForecast] = func(g *Generator, e exp.Expression) string {
		return g.funcCall("FORECAST", g.tableArgs(e, "this", "data_col", "timestamp_col", "model", "id_cols", "horizon",
			"forecast_end_timestamp", "confidence_level", "output_historical_time_series", "context_window"), "(", ")", true)
	}
	dispatch[exp.KindFeaturesAtTime] = func(g *Generator, e exp.Expression) string {
		return g.funcCall("FEATURES_AT_TIME", g.tableArgs(e, "this", "time", "num_rows", "ignore_feature_nulls"), "(", ")", true)
	}
	dispatch[exp.KindVectorSearch] = func(g *Generator, e exp.Expression) string {
		args := g.tableArgs(e, "this", "column_to_search", "query_table", "query_column_to_search", "top_k", "distance_type", "options")
		if q := asExpression(e.Arg("query_table")); q != nil && q.Kind() == exp.KindTable {
			args[2] = "TABLE " + g.gen(q)
		}
		return g.funcCall("VECTOR_SEARCH", args, "(", ")", true)
	}
}

// tableArgs collects args for a table-valued function call, prefixing a Table `this` with TABLE.
func (g *Generator) tableArgs(e exp.Expression, keys ...string) []any {
	args := make([]any, len(keys))
	for i, key := range keys {
		args[i] = e.Arg(key)
	}
	if this := asExpression(e.Arg("this")); this != nil && this.Kind() == exp.KindTable {
		args[0] = "TABLE " + g.gen(this)
	}
	return args
}

// mlSQL ports _ml_sql (generator.py:5054-5066).
func (g *Generator) mlSQL(e exp.Expression, name string) string {
	var exprSQL any
	if expr := asExpression(e.Arg("expression")); expr != nil {
		exprSQL = g.gen(expr)
		if expr.Kind() == exp.KindTable {
			exprSQL = "TABLE " + g.gen(expr)
		}
	}
	var params any
	if p := g.sqlKey(e, "params_struct"); p != "" {
		params = p
	}
	return g.funcCall(name, []any{"MODEL " + g.sqlKey(e, "this"), exprSQL, params}, "(", ")", true)
}

// generators/bigquery.py:320 (remove_precision_parameterized_types) and 716-727: inline
// typed array literals such as ARRAY<INT64>[1, 2] round-trip as written.
func (g *Generator) bigqueryCastSQL(e exp.Expression, safePrefix string) string {
	for _, dt := range e.FindAll(exp.KindDataType) {
		var kept []exp.Expression
		for _, param := range listFromValue(dt.Arg("expressions")) {
			if p := asExpression(param); p != nil && p.Kind() != exp.KindDataTypeParam {
				kept = append(kept, p)
			}
		}
		dt.Set("expressions", kept)
	}
	if this := asExpression(e.Arg("this")); this != nil && this.Kind() == exp.KindArray {
		elems := listFromValue(this.Arg("expressions"))
		var first exp.Expression
		if len(elems) > 0 {
			first = asExpression(elems[0])
		}
		if first == nil || first.Find(exp.TraitQuery) == nil {
			return g.sqlKey(e, "to") + g.gen(this)
		}
	}
	return g.castSQLWithPrefix(e, safePrefix)
}

// bigqueryUnqualifyUnnest ports transforms.unqualify_unnest (transforms.py:278-294): drop the
// table qualifier that qualify_columns points at an UNNEST alias, which BigQuery cannot name.
func bigqueryUnqualifyUnnest(sel exp.Expression) {
	aliases := map[string]bool{}
	for _, unnest := range sel.WalkWithPrune(true, func(n exp.Expression) bool { return n != sel && n.Is(exp.TraitQuery) }) {
		if unnest.Kind() != exp.KindUnnest || unnest.Parent() == nil {
			continue
		}
		if k := unnest.Parent().Kind(); k == exp.KindFrom || k == exp.KindJoin {
			if alias := unnest.Alias(); alias != "" {
				aliases[alias] = true
			}
		}
	}
	if len(aliases) == 0 {
		return
	}
	for _, column := range sel.FindAll(exp.KindColumn) {
		parts := column.Parts()
		if len(parts) == 0 {
			continue
		}
		if leftmost := parts[0]; leftmost.ArgKey() != "this" && aliases[leftmost.Name()] {
			leftmost.Pop()
		}
	}
}

// generators/bigquery.py:668-696, without the annotation-driven struct-subscript rewrite.
func (g *Generator) bigqueryBracketSQL(e exp.Expression) string {
	sql := g.expressions(exprsOptions{expression: e, flat: true})
	switch offset := e.Arg("offset"); offset {
	case 0:
		sql = "OFFSET(" + sql + ")"
	case 1:
		sql = "ORDINAL(" + sql + ")"
	case nil:
	default:
		g.unsupported("Unsupported array offset")
	}
	if truthy(e.Arg("safe")) {
		sql = "SAFE_" + sql
	}
	return g.sqlKey(e, "this") + "[" + sql + "]"
}
