package generator

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// generators/presto.py:303; cross-dialect Select preprocessing is intentionally omitted.
func init() {
	table := map[exp.Kind]func(*Generator, exp.Expression) string{
		exp.KindApproxQuantile: prestoCall("APPROX_PERCENTILE", "this", "weight", "quantile", "accuracy"),
		exp.KindDateAdd:        (*Generator).prestoDateAddSQL,
		exp.KindDateDiff: func(g *Generator, e exp.Expression) string {
			return g.funcCall("DATE_DIFF", []any{prestoUnit(e), e.Arg("expression"), e.Arg("this")}, "(", ")", true)
		},
		exp.KindDecode:         prestoCall("FROM_UTF8", "this", "replace"),
		exp.KindEncode:         prestoCall("TO_UTF8", "this", "replace"),
		exp.KindUnixToTime:     prestoCall("FROM_UNIXTIME", "this", "zone", "hours", "minutes"),
		exp.KindGenerateSeries: prestoCall("SEQUENCE", "start", "end", "step"),
		exp.KindStrPosition:    prestoCall("STRPOS", "this", "substr", "occurrence"),
		exp.KindIf:             prestoCall("IF", "this", "true", "false"),
		exp.KindAtTimeZone:     prestoCall("AT_TIMEZONE", "this", "zone"),
		exp.KindDataType:       (*Generator).prestoDataTypeSQL,
		exp.KindStruct:         (*Generator).prestoStructSQL,
		exp.KindBracket:        (*Generator).prestoBracketSQL,
		exp.KindInterval:       (*Generator).prestoIntervalSQL,
		exp.KindTransaction:    (*Generator).prestoTransactionSQL,
		exp.KindGroupConcat: func(g *Generator, e exp.Expression) string {
			return g.funcCall("ARRAY_JOIN", []any{g.funcCall("ARRAY_AGG", []any{e.Arg("this")}, "(", ")", true), e.Arg("separator")}, "(", ")", true)
		},
		exp.KindSqlSecurityProperty:   func(g *Generator, e exp.Expression) string { return "SECURITY " + g.sqlKey(e, "this") },
		exp.KindSchemaCommentProperty: func(g *Generator, e exp.Expression) string { return "COMMENT " + g.sqlKey(e, "this") },
		exp.KindHex: func(g *Generator, e exp.Expression) string {
			return g.funcCall("TO_HEX", []any{e.Arg("this")}, "(", ")", true)
		},
		exp.KindSchema:             (*Generator).prestoSchemaSQL,
		exp.KindFileFormatProperty: func(g *Generator, e exp.Expression) string { return "format=" + g.gen(exp.LiteralString(e.Name())) },
		exp.KindSHA2:               (*Generator).prestoSHA2DigestSQL,
		exp.KindConcat:             (*Generator).prestoConcatSQL,
		exp.KindCreate:             (*Generator).prestoCreateSQL,
		exp.KindJSONFormat:         prestoCall("JSON_FORMAT", "this", "options"),
		exp.KindJSONExtract: func(g *Generator, e exp.Expression) string {
			args := []any{e.Arg("this"), e.Arg("expression")}
			for _, x := range e.Expressions() {
				args = append(args, x)
			}
			return g.funcCall("JSON_EXTRACT", args, "(", ")", true)
		},
		exp.KindILike: func(g *Generator, e exp.Expression) string {
			return "LOWER(" + g.sqlKey(e, "this") + ") LIKE LOWER(" + g.sqlKey(e, "expression") + ")"
		},
		exp.KindInitcap: func(g *Generator, e exp.Expression) string {
			// presto.py:50-58.
			if d := asExpression(e.Arg("expression")); d != nil && (!d.IsString() || d.Name() != initcapDefaultDelimiterChars) {
				g.unsupported("INITCAP does not support custom delimiters")
			}
			return "REGEXP_REPLACE(" + g.sqlKey(e, "this") + ", '(\\w)(\\w*)', x -> UPPER(x[1]) || LOWER(x[2]))"
		},
		exp.KindXor: func(g *Generator, e exp.Expression) string {
			a, b := g.sqlKey(e, "this"), g.sqlKey(e, "expression")
			return "(" + a + " AND (NOT " + b + ")) OR ((NOT " + a + ") AND " + b + ")"
		},
	}
	for kind, name := range map[exp.Kind]string{
		exp.KindAnyValue: "ARBITRARY", exp.KindArrayContains: "CONTAINS", exp.KindArrayUniqueAgg: "SET_AGG", exp.KindArraySlice: "SLICE", exp.KindArraySize: "CARDINALITY",
		exp.KindBitwiseAnd: "BITWISE_AND", exp.KindBitwiseOr: "BITWISE_OR", exp.KindBitwiseXor: "BITWISE_XOR", exp.KindBitwiseNot: "BITWISE_NOT", exp.KindBitwiseLeftShift: "BITWISE_LEFT_SHIFT", exp.KindBitwiseRightShift: "BITWISE_RIGHT_SHIFT",
		exp.KindDayOfWeekIso: "DAY_OF_WEEK", exp.KindLogicalOr: "BOOL_OR", exp.KindStrToMap: "SPLIT_TO_MAP", exp.KindTrunc: "TRUNCATE", exp.KindVariancePop: "VAR_POP", exp.KindLevenshtein: "LEVENSHTEIN_DISTANCE", exp.KindUnhex: "FROM_HEX", exp.KindTimeToUnix: "TO_UNIXTIME", exp.KindMD5Digest: "MD5", exp.KindSubstring: "SUBSTR",
	} {
		table[kind] = prestoRename(name)
	}
	for kind, name := range map[exp.Kind]string{exp.KindCurrentTime: "CURRENT_TIME", exp.KindCurrentTimestamp: "CURRENT_TIMESTAMP", exp.KindCurrentUser: "CURRENT_USER"} {
		table[kind] = func(_ *Generator, _ exp.Expression) string { return name }
	}
	registerDialectDispatch("presto", table)
	registerDialectTypeMapping("presto", map[exp.DType]string{exp.DTypeBinary: "VARBINARY", exp.DTypeBit: "BOOLEAN", exp.DTypeDatetime: "TIMESTAMP", exp.DTypeDatetime64: "TIMESTAMP", exp.DTypeFloat: "REAL", exp.DTypeHllSketch: "HYPERLOGLOG", exp.DTypeInt: "INTEGER", exp.DTypeStruct: "ROW", exp.DTypeText: "VARCHAR", exp.DTypeTimestampTz: "TIMESTAMP", exp.DTypeTimestampNtz: "TIMESTAMP", exp.DTypeTimeTz: "TIME"})
	registerDialectPropertyLocations("presto", map[exp.Kind]propertyLocation{exp.KindLocationProperty: propertyLocationUnsupported})
}

func prestoRename(name string) func(*Generator, exp.Expression) string {
	return func(g *Generator, e exp.Expression) string {
		return g.funcCall(name, g.fallbackArgs(e), "(", ")", true)
	}
}
func prestoCall(name string, keys ...string) func(*Generator, exp.Expression) string {
	return func(g *Generator, e exp.Expression) string {
		args := make([]any, 0, len(keys))
		for _, key := range keys {
			args = append(args, e.Arg(key))
		}
		return g.funcCall(name, args, "(", ")", true)
	}
}

// generators/presto.py:288-301 and generator.py datatype_sql.
func (g *Generator) prestoDataTypeSQL(e exp.Expression) string {
	if exp.IsType(e, exp.DTypeTimestampTz) || exp.IsType(e, exp.DTypeTimeTz) {
		return g.dataTypeSQL(e) + " WITH TIME ZONE"
	}
	if exp.IsType(e, exp.DTypeArray) || exp.IsType(e, exp.DTypeMap) || exp.IsType(e, exp.DTypeStruct) {
		c := e.Copy()
		c.Set("nested", false)
		return g.dataTypeSQL(c)
	}
	return g.dataTypeSQL(e)
}

// generators/presto.py:588-620. Named fields need their types (upstream annotate_types); the
// port infers them for literal values and reports anything else unsupported.
func (g *Generator) prestoStructSQL(e exp.Expression) string {
	values := []string{}
	schema := []string{}
	unknown := false
	for _, x := range e.Expressions() {
		if x.Kind() == exp.KindPropertyEQ {
			value := asExpression(x.Arg("expression"))
			if dtype := prestoLiteralType(value); dtype != "" {
				schema = append(schema, g.sqlKey(x, "this")+" "+dtype)
			} else {
				unknown = true
			}
			values = append(values, g.gen(value))
		} else {
			values = append(values, g.gen(x))
		}
	}
	if len(values) == 0 || len(schema) != len(values) {
		if unknown {
			g.unsupported("Cannot convert untyped key-value definitions (try annotate_types).")
		}
		return "ROW(" + strings.Join(values, ", ") + ")"
	}
	return "CAST(ROW(" + strings.Join(values, ", ") + ") AS ROW(" + strings.Join(schema, ", ") + "))"
}

func prestoLiteralType(e exp.Expression) string {
	if e == nil {
		return ""
	}
	switch e.Kind() {
	case exp.KindLiteral:
		if e.IsString() {
			return "VARCHAR"
		}
		if strings.ContainsAny(e.Name(), ".eE") {
			return "DOUBLE"
		}
		return "INTEGER"
	case exp.KindBoolean:
		return "BOOLEAN"
	case exp.KindNeg:
		return prestoLiteralType(e.This())
	}
	return ""
}

// generator.py:3678-3680 with SUPPORTS_SINGLE_ARG_CONCAT = False (presto.py:271).
func (g *Generator) prestoConcatSQL(e exp.Expression) string {
	if args := e.Expressions(); len(args) == 1 {
		return g.gen(args[0])
	}
	return g.concatSQL(e)
}

// presto.py:639-648: Presto has no CREATE VIEW column list.
func (g *Generator) prestoCreateSQL(e exp.Expression) string {
	if e.Text("kind") == "VIEW" {
		if schema := e.This(); schema != nil && schema.Kind() == exp.KindSchema && len(schema.Expressions()) > 0 {
			e = e.Copy()
			e.This().Set("expressions", nil)
		}
	}
	return g.createSQL(e)
}

// generators/presto.py:583-586; parser Bracket offsets already match Presto's one-based indexing.
func (g *Generator) prestoBracketSQL(e exp.Expression) string {
	if boolValue(e.Arg("safe")) {
		args := []any{e.Arg("this")}
		if xs := e.Expressions(); len(xs) > 0 {
			args = append(args, xs[0])
		}
		return g.funcCall("ELEMENT_AT", args, "(", ")", true)
	}
	return g.bracketSQL(e)
}

// generators/presto.py:622-625.
func (g *Generator) prestoIntervalSQL(e exp.Expression) string {
	if e.This() != nil && strings.HasPrefix(strings.ToUpper(e.Text("unit")), "WEEK") {
		return "(" + e.This().Name() + " * INTERVAL '7' DAY)"
	}
	return g.intervalSQL(e)
}

// generators/presto.py:627-630.
func (g *Generator) prestoTransactionSQL(e exp.Expression) string {
	modes := []string{}
	switch v := e.Arg("modes").(type) {
	case []string:
		modes = v
	case []any:
		for _, x := range v {
			modes = append(modes, g.gen(x))
		}
	case []exp.Expression:
		for _, x := range v {
			modes = append(modes, g.gen(x))
		}
	}
	if len(modes) > 0 {
		return "START TRANSACTION " + strings.Join(modes, ", ")
	}
	return "START TRANSACTION"
}

// generators/presto.py:67-89.
func (g *Generator) prestoSchemaSQL(e exp.Expression) string {
	if parent := e.Parent(); parent != nil && parent.Kind() == exp.KindPartitionedByProperty {
		// Partition column names go into ARRAY[] string literals unquoted.
		args := []string{}
		for _, x := range e.Expressions() {
			var name string
			switch {
			case x.Is(exp.TraitFunc) || x.Kind() == exp.KindProperty:
				name = g.gen(x)
			case x.Kind() == exp.KindIdentifier:
				name = x.Name()
			default:
				this := asExpression(x.Arg("this"))
				if this != nil && this.Kind() == exp.KindIdentifier {
					name = this.Name()
				} else {
					name = g.sqlKey(x, "this")
				}
			}
			args = append(args, g.gen(exp.LiteralString(name)))
		}
		return "ARRAY[" + strings.Join(args, ", ") + "]"
	}
	if parent := e.Parent(); parent != nil {
		// The partition columns must also exist on the table: fold PARTITIONED_BY (col TYPE, ...)
		// ColumnDefs into the table schema.
		for _, schema := range parent.FindAll(exp.KindSchema) {
			if schema == e {
				continue
			}
			if sp := schema.Parent(); sp != nil && sp.Kind() == exp.KindPartitionedByProperty {
				for _, def := range schema.FindAll(exp.KindColumnDef) {
					e.Append("expressions", def.Copy())
				}
			}
		}
	}
	return g.schemaSQL(e)
}

// generators/presto.py:147-169 _to_int: the interval is cast to BIGINT unless annotate_types
// proves it integer. Without the full annotator this mirrors its literal/arithmetic/known-
// function rules and casts everything else (columns, unknown calls), like upstream.
func (g *Generator) prestoDateAddSQL(e exp.Expression) string {
	interval := e.Arg("expression")
	if iv := asExpression(interval); iv != nil && !prestoIntegerTyped(iv) {
		interval = "CAST(" + g.gen(interval) + " AS BIGINT)"
	}
	return g.funcCall("DATE_ADD", []any{prestoUnit(e), interval, e.Arg("this")}, "(", ")", true)
}

func prestoIntegerTyped(e exp.Expression) bool {
	switch e.Kind() {
	case exp.KindLiteral:
		return !e.IsString() && !strings.ContainsAny(e.Name(), ".eE")
	case exp.KindNeg, exp.KindParen, exp.KindFloor:
		// FLOOR(x) types as BIGINT for any numeric x (annotate_types FLOOR rule).
		return e.Kind() == exp.KindFloor || prestoIntegerTyped(e.This())
	case exp.KindAdd, exp.KindSub, exp.KindMul, exp.KindMod:
		return prestoIntegerTyped(e.This()) && prestoIntegerTyped(asExpression(e.Arg("expression")))
	case exp.KindCoalesce:
		if !prestoIntegerTyped(e.This()) {
			return false
		}
		for _, x := range e.Expressions() {
			if !prestoIntegerTyped(x) {
				return false
			}
		}
		return true
	case exp.KindCast, exp.KindTryCast:
		to := asExpression(e.Arg("to"))
		if to == nil {
			return false
		}
		dt, ok := to.Arg("this").(exp.DType)
		return ok && exp.IntegerTypes[dt]
	case exp.KindLength, exp.KindCount, exp.KindDayOfWeekIso, exp.KindDayOfYear, exp.KindWeekOfYear, exp.KindDayOfMonth, exp.KindDayOfWeek, exp.KindTimeToUnix:
		return true
	}
	return false
}

// generators/presto.py:34-46; the Go parser represents SHA2Digest with KindSHA2.
func (g *Generator) prestoSHA2DigestSQL(e exp.Expression) string {
	length := e.Text("length")
	if length == "" {
		length = "256"
	}
	if length != "256" && length != "512" {
		g.unsupported("SHA" + length + " is not supported in Presto")
	}
	this := e.Arg("this")
	// The digest takes VARBINARY: a text-typed argument (a CAST to a text type, the only case
	// where upstream sees a type without annotate_types) is encoded first.
	if cast := asExpression(this); cast != nil && (cast.Kind() == exp.KindCast || cast.Kind() == exp.KindTryCast) {
		if to := asExpression(cast.Arg("to")); to != nil {
			if dt, ok := to.Arg("this").(exp.DType); ok && exp.TextTypes[dt] {
				this = g.funcCall("TO_UTF8", []any{cast}, "(", ")", true)
			}
		}
	}
	return g.funcCall("SHA"+length, []any{this}, "(", ")", true)
}

// dialects/dialect.py:2059-2066.
func prestoUnit(e exp.Expression) exp.Expression {
	unit := asExpression(e.Arg("unit"))
	if unit == nil {
		return exp.LiteralString("DAY")
	}
	if unit.Kind() == exp.KindLiteral || unit.Kind() == exp.KindVar {
		return exp.LiteralString(strings.ToUpper(unit.Name()))
	}
	return unit
}
