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
		exp.KindIntDiv:            rename("DIV"),
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
			var unit any = e.Arg("unit")
			if u := asExpression(unit); u != nil && u.IsString() {
				unit = u.Name()
			}
			return g.funcCall("DATE_TRUNC", []any{e.Arg("this"), unit, e.Arg("zone")}, "(", ")", true)
		},
		exp.KindTimeToStr: (*Generator).bigqueryTimeToStrSQL,
		exp.KindTryCast: func(g *Generator, e exp.Expression) string {
			return g.castSQLWithPrefix(e, "SAFE_")
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
