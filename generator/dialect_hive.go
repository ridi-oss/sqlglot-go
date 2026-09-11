package generator

import (
	"strconv"
	"strings"

	"github.com/ridi-oss/sqlglot-go/dialects"
	"github.com/ridi-oss/sqlglot-go/expressions"
)

// Hive overrides follow generators/hive.py:260-393 and 474-642.
func init() {
	registerDialectTypeMapping("hive", map[expressions.DType]string{
		expressions.DTypeBit: "BOOLEAN", expressions.DTypeBlob: "BINARY", expressions.DTypeDatetime: "TIMESTAMP", expressions.DTypeRowVersion: "BINARY", expressions.DTypeText: "STRING", expressions.DTypeTime: "TIMESTAMP", expressions.DTypeTimestampNtz: "TIMESTAMP", expressions.DTypeTimestampTz: "TIMESTAMP", expressions.DTypeUTinyInt: "SMALLINT", expressions.DTypeVarBinary: "BINARY",
	})
	registerDialectPropertyLocations("hive", map[expressions.Kind]propertyLocation{
		expressions.KindExternalProperty:       propertyLocationPostCreate,
		expressions.KindLocationProperty:       propertyLocationPostSchema,
		expressions.KindClusteredByProperty:    propertyLocationPostSchema,
		expressions.KindStorageHandlerProperty: propertyLocationPostSchema,
		expressions.KindUsingProperty:          propertyLocationPostExpression,
		expressions.KindFileFormatProperty:     propertyLocationPostSchema,
		expressions.KindPartitionedByProperty:  propertyLocationPostSchema,
		expressions.KindWithDataProperty:       propertyLocationUnsupported,
	})
	handlers := map[expressions.Kind]func(*Generator, expressions.Expression) string{
		expressions.KindProperties: (*Generator).hivePropertiesSQL,
		expressions.KindProperty: func(g *Generator, e expressions.Expression) string {
			return g.gen(expressions.LiteralString(e.Name())) + "=" + g.sqlKey(e, "value")
		},
		expressions.KindExternalProperty:       func(*Generator, expressions.Expression) string { return "EXTERNAL" },
		expressions.KindLocationProperty:       hivePrefix("LOCATION "),
		expressions.KindStorageHandlerProperty: hivePrefix("STORED BY "),
		expressions.KindSchemaCommentProperty:  hivePrefix("COMMENT "),
		expressions.KindPartitionedByProperty:  hivePrefix("PARTITIONED BY "),
		expressions.KindUsingProperty: func(g *Generator, e expressions.Expression) string {
			return "USING " + e.Text("kind") + " " + g.sqlKey(e, "this")
		},
		expressions.KindFileFormatProperty: func(g *Generator, e expressions.Expression) string {
			this := strings.ToUpper(e.Name())
			if t := e.This(); t != nil && t.Kind() == expressions.KindInputOutputFormat {
				this = g.gen(t)
			}
			return "STORED AS " + this
		},
		expressions.KindInputOutputFormat: func(g *Generator, e expressions.Expression) string {
			var p []string
			for _, a := range []struct{ k, p string }{{"input_format", "INPUTFORMAT "}, {"output_format", "OUTPUTFORMAT "}} {
				if s := g.sqlKey(e, a.k); s != "" {
					p = append(p, a.p+s)
				}
			}
			return strings.Join(p, " ")
		},
		expressions.KindClusteredByProperty: func(g *Generator, e expressions.Expression) string {
			s := "CLUSTERED BY (" + g.expressions(exprsOptions{expression: e, flat: true}) + ")"
			if v := g.expressions(exprsOptions{expression: e, key: "sorted_by", flat: true}); v != "" {
				s += " SORTED BY (" + v + ")"
			}
			return s + " INTO " + g.sqlKey(e, "buckets") + " BUCKETS"
		},
		expressions.KindAlterSet:    (*Generator).hiveAlterSetSQL,
		expressions.KindAlterColumn: (*Generator).hiveAlterColumnSQL,
		expressions.KindColumnDef: func(g *Generator, e expressions.Expression) string {
			s := g.columnDefSQL(e)
			if p := e.Parent(); p != nil && p.Kind() == expressions.KindDataType && expressions.IsType(p, expressions.DTypeStruct) {
				column := g.sqlKey(e, "this")
				s = strings.Replace(s, column+" ", column+": ", 1)
			}
			return s
		},
		expressions.KindDataType:       (*Generator).hiveDataTypeSQL,
		expressions.KindTryCast:        func(g *Generator, e expressions.Expression) string { return g.castSQLWithPrefix(e, "") },
		expressions.KindApproxDistinct: hiveCall("APPROX_COUNT_DISTINCT", "this"),
		expressions.KindRegexpLike:     func(g *Generator, e expressions.Expression) string { return g.binary(e, "RLIKE") },
		expressions.KindIgnoreNulls: func(g *Generator, e expressions.Expression) string {
			// hive.py:432-437: FIRST/LAST/FIRST_VALUE/LAST_VALUE take IGNORE NULLS as a TRUE argument.
			if this := e.This(); this != nil {
				switch this.Kind() {
				case expressions.KindFirst, expressions.KindLast, expressions.KindFirstValue, expressions.KindLastValue:
					return g.funcCall(g.sqlName(this.Kind()), []any{this.This(), expressions.Boolean(expressions.Args{"this": true})}, "(", ")", true)
				}
			}
			return g.ignoreNullsSQL(e)
		},
		expressions.KindStrToDate: func(g *Generator, e expressions.Expression) string {
			return "CAST(" + hiveStrToTimeInner(g, e) + " AS DATE)"
		},
		expressions.KindStrToTime: func(g *Generator, e expressions.Expression) string {
			return "CAST(" + hiveStrToTimeInner(g, e) + " AS TIMESTAMP)"
		},
		expressions.KindDateAdd:      (*Generator).hiveAddDateSQL,
		expressions.KindTsOrDsAdd:    (*Generator).hiveAddDateSQL,
		expressions.KindTsOrDsToDate: (*Generator).hiveToDateSQL,
		expressions.KindTimeToStr: func(g *Generator, e expressions.Expression) string {
			// hive.py:593-598: DATE_FORMAT(TimeStrToTime(x), f) renders the inner x directly.
			this := e.This()
			if this != nil && this.Kind() == expressions.KindTimeStrToTime {
				this = this.This()
			}
			return g.funcCall("DATE_FORMAT", []any{this, hiveFormatTimeLiteral(e)}, "(", ")", true)
		},
		expressions.KindUnixToStr: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("FROM_UNIXTIME", []any{e.This(), hiveNonDefaultTimeFormat(e)}, "(", ")", true)
		},
		expressions.KindStrToUnix: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("UNIX_TIMESTAMP", []any{e.This(), hiveNonDefaultTimeFormat(e)}, "(", ")", true)
		},
		expressions.KindArrayAgg: func(g *Generator, e expressions.Expression) string {
			this := e.This()
			if this != nil && this.Kind() == expressions.KindOrder {
				this = this.This()
			}
			return g.funcCall("COLLECT_LIST", []any{this}, "(", ")", true)
		},
		expressions.KindStruct: func(g *Generator, e expressions.Expression) string {
			args := []any{}
			for _, x := range e.Expressions() {
				if x.Kind() == expressions.KindPropertyEQ {
					g.unsupported("Hive does not support named structs.")
					args = append(args, x.Arg("expression"))
				} else {
					args = append(args, x)
				}
			}
			return g.funcCall("STRUCT", args, "(", ")", true)
		},
		expressions.KindVarMap:            (*Generator).hiveVarMapSQL,
		expressions.KindRegexpExtract:     (*Generator).hiveRegexpExtractSQL,
		expressions.KindRegexpExtractAll:  (*Generator).hiveRegexpExtractSQL,
		expressions.KindJSONExtract:       hiveCall("GET_JSON_OBJECT", "this", "expression"),
		expressions.KindJSONExtractScalar: hiveCall("GET_JSON_OBJECT", "this", "expression"),
		expressions.KindGenerateSeries:    hiveCall("SEQUENCE", "start", "end", "step"),
		expressions.KindArraySize:         hiveCall("SIZE", "this"),
		expressions.KindStrToMap:          hiveCall("STR_TO_MAP", "this", "pair_delim", "key_value_delim"),
		expressions.KindTimestampTrunc: func(g *Generator, e expressions.Expression) string {
			return g.funcCall("TRUNC", []any{e.This(), expressions.LiteralString(e.Text("unit"))}, "(", ")", true)
		},
	}
	for k, name := range map[expressions.Kind]string{expressions.KindAnyValue: "FIRST", expressions.KindFromBase64: "UNBASE64", expressions.KindToBase64: "BASE64", expressions.KindJSONFormat: "TO_JSON", expressions.KindQuantile: "PERCENTILE", expressions.KindApproxQuantile: "PERCENTILE_APPROX", expressions.KindRegexpSplit: "SPLIT", expressions.KindArrayUniqueAgg: "COLLECT_SET", expressions.KindTimeStrToDate: "TO_DATE", expressions.KindTimeStrToUnix: "UNIX_TIMESTAMP", expressions.KindTimeToUnix: "UNIX_TIMESTAMP", expressions.KindWeekOfYear: "WEEKOFYEAR", expressions.KindDayOfMonth: "DAYOFMONTH", expressions.KindDayOfWeek: "DAYOFWEEK", expressions.KindUnnest: "EXPLODE", expressions.KindStarMap: "MAP"} {
		name := name
		handlers[k] = func(g *Generator, e expressions.Expression) string {
			return g.funcCall(name, g.fallbackArgs(e), "(", ")", true)
		}
	}
	registerDialectDispatch("hive", handlers)
}

func hiveCall(name string, keys ...string) func(*Generator, expressions.Expression) string {
	return func(g *Generator, e expressions.Expression) string {
		args := make([]any, 0, len(keys))
		for _, key := range keys {
			args = append(args, e.Arg(key))
		}
		return g.funcCall(name, args, "(", ")", true)
	}
}

// hiveFormatTimeLiteral is Generator.format_time under Hive's INVERSE_TIME_MAPPING.
func hiveFormatTimeLiteral(e expressions.Expression) any {
	format := asExpression(e.Arg("format"))
	if format == nil || !format.IsString() {
		return format
	}
	return expressions.LiteralString(dialects.HiveFormatTime(format.Name()))
}

// hiveNonDefaultTimeFormat ports dialect.py:1622-1633 time_format("hive"): nil for TIME_FORMAT.
func hiveNonDefaultTimeFormat(e expressions.Expression) any {
	format := hiveFormatTimeLiteral(e)
	if lit, ok := format.(expressions.Expression); ok && lit != nil && lit.Name() == dialects.HiveTimeFormat {
		return nil
	}
	return format
}

// hiveStrToTimeInner ports the shared body of _str_to_date_sql/_str_to_time_sql (hive.py:199-209).
func hiveStrToTimeInner(g *Generator, e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if format, ok := hiveFormatTimeLiteral(e).(expressions.Expression); ok && format != nil {
		if name := format.Name(); name != dialects.HiveTimeFormat && name != dialects.HiveDateFormat {
			return "FROM_UNIXTIME(UNIX_TIMESTAMP(" + this + ", " + g.gen(format) + "))"
		}
	}
	return this
}

// hiveToDateSQL ports _to_date_sql (hive.py:211-219).
func (g *Generator) hiveToDateSQL(e expressions.Expression) string {
	if format, ok := hiveFormatTimeLiteral(e).(expressions.Expression); ok && format != nil {
		if name := format.Name(); name != dialects.HiveTimeFormat && name != dialects.HiveDateFormat {
			return g.funcCall("TO_DATE", []any{e.This(), format}, "(", ")", true)
		}
	}
	if p := e.Parent(); p != nil {
		switch p.Kind() {
		case expressions.KindDateDiff, expressions.KindDay, expressions.KindMonth, expressions.KindYear:
			return g.sqlKey(e, "this")
		}
	}
	return g.funcCall("TO_DATE", []any{e.This()}, "(", ")", true)
}

// hiveVarMapSQL ports var_map_sql (dialect.py:1521-1536).
func (g *Generator) hiveVarMapSQL(e expressions.Expression) string {
	keys, values := asExpression(e.Arg("keys")), asExpression(e.Arg("values"))
	if keys == nil || values == nil || keys.Kind() != expressions.KindArray || values.Kind() != expressions.KindArray {
		g.unsupported("Cannot convert array columns into map.")
		return g.funcCall("MAP", []any{keys, values}, "(", ")", true)
	}
	args := []any{}
	ks, vs := keys.Expressions(), values.Expressions()
	for i := 0; i < len(ks) && i < len(vs); i++ {
		args = append(args, ks[i], vs[i])
	}
	return g.funcCall("MAP", args, "(", ")", true)
}

// hiveRegexpExtractSQL ports regexp_extract_sql (dialect.py:1855-1865).
func (g *Generator) hiveRegexpExtractSQL(e expressions.Expression) string {
	group := asExpression(e.Arg("group"))
	if group != nil && group.Name() == strconv.Itoa(g.dialect.RegexpExtractDefaultGroup) {
		group = nil
	}
	return g.funcCall(g.sqlName(e.Kind()), []any{e.This(), e.Arg("expression"), group}, "(", ")", true)
}

func hivePrefix(prefix string) func(*Generator, expressions.Expression) string {
	return func(g *Generator, e expressions.Expression) string { return prefix + g.sqlKey(e, "this") }
}

// generators/hive.py:235 supplies WITH_PROPERTIES_PREFIX for generator.py:2019-2042.
func (g *Generator) hivePropertiesSQL(e expressions.Expression) string {
	locations := g.locateProperties(e)
	root := g.rootPropertiesSQL(g.propertiesExpression(locations[propertyLocationPostSchema], e.Parent()))
	with := g.renderProperties(g.propertiesExpression(locations[propertyLocationPostWith], e.Parent()), g.seg("TBLPROPERTIES", ""), ", ", "", true)
	if root != "" && with != "" && !g.pretty {
		with = " " + with
	}
	return root + with
}

// generators/hive.py:590-605.
func (g *Generator) hiveAlterSetSQL(e expressions.Expression) string {
	s := "SET"
	if v := g.sqlKey(e, "serde"); v != "" {
		s += " SERDE " + v
	}
	if v := g.expressions(exprsOptions{expression: e, flat: true}); v != "" {
		s += " " + v
	}
	if v := g.sqlKey(e, "location"); v != "" {
		s += " LOCATION " + v
	}
	if v := g.expressions(exprsOptions{expression: e, key: "file_format", flat: true, sep: " "}); v != "" {
		s += " FILEFORMAT " + v
	}
	if v := g.expressions(exprsOptions{expression: e, key: "tag", flat: true, sep: ""}); v != "" {
		s += " TAGS " + v
	}
	return s
}

// generators/hive.py:560-584.
func (g *Generator) hiveAlterColumnSQL(e expressions.Expression) string {
	if boolValue(e.Arg("exists")) {
		g.unsupported("ALTER COLUMN IF EXISTS is not supported by this dialect")
	}
	if truthy(e.Arg("default")) || truthy(e.Arg("drop")) || truthy(e.Arg("visible")) || e.Arg("allow_null") != nil {
		g.unsupported("Unsupported CHANGE COLUMN syntax")
	}
	this := g.sqlKey(e, "this")
	name := g.sqlKey(e, "rename_to")
	if name == "" {
		name = this
	}
	dtype := g.sqlKey(e, "dtype")
	if dtype == "" {
		g.unsupported("CHANGE COLUMN without a type is not supported")
	}
	s := "CHANGE COLUMN " + this + " " + name + " " + dtype
	if c := g.sqlKey(e, "comment"); c != "" {
		s += " COMMENT " + c
	}
	return s
}

// generators/hive.py:502-520.
func (g *Generator) hiveDataTypeSQL(e expressions.Expression) string {
	e = e.Copy()
	params := e.Expressions()
	switch e.Arg("this") {
	case expressions.DTypeVarchar, expressions.DTypeNVarchar, expressions.DTypeChar, expressions.DTypeNChar:
		if len(params) == 0 || params[0].Name() == "MAX" {
			e.Set("this", expressions.DTypeText)
			e.Set("expressions", nil)
		}
	case expressions.DTypeText:
		if len(params) > 0 {
			e.Set("this", expressions.DTypeVarchar)
		}
	case expressions.DTypeFloat:
		if len(params) > 0 {
			if n, err := strconv.Atoi(params[0].Name()); err == nil {
				if n > 32 {
					e.Set("this", expressions.DTypeDouble)
				}
				e.Set("expressions", nil)
			}
		}
	}
	if dtype, ok := e.Arg("this").(expressions.DType); ok && expressions.TemporalTypes[dtype] {
		e.Set("expressions", nil)
	}
	return g.dataTypeSQL(e)
}

// generators/hive.py:118-139.
func (g *Generator) hiveAddDateSQL(e expressions.Expression) string {
	unit := strings.ToUpper(e.Text("unit"))
	name := "DATE_ADD"
	mult := 1
	switch unit {
	case "WEEK":
		mult = 7
	case "MONTH":
		name = "ADD_MONTHS"
	case "QUARTER":
		name = "ADD_MONTHS"
		mult = 3
	case "YEAR":
		name = "ADD_MONTHS"
		mult = 12
	}
	inc := e.Expr()
	sql := g.gen(inc)
	if mult != 1 {
		if inc != nil && inc.Kind() == expressions.KindLiteral {
			if n, err := strconv.ParseFloat(inc.Name(), 64); err == nil {
				sql = strconv.FormatFloat(n*float64(mult), 'f', -1, 64)
			} else {
				g.unsupported("Invalid date increment")
			}
		} else {
			sql = "(" + sql + ") * " + strconv.Itoa(mult)
		}
	}
	return g.funcCall(name, []any{e.This(), sql}, "(", ")", true)
}
