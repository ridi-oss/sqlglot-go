package parser_test

import (
	"strings"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestParseCastAndTypes(t *testing.T) {
	cast := parseOne(t, "CAST(x AS INT)")
	if cast.Kind() != exp.KindCast || !exp.IsType(exprArg(t, cast, "to"), exp.DTypeInt) {
		t.Fatalf("CAST type mismatch:\n%s", cast.ToS())
	}

	cast = parseOne(t, "CAST(x AS DECIMAL(10, 2))")
	to := exprArg(t, cast, "to")
	if !exp.IsType(to, exp.DTypeDecimal) || len(to.Expressions()) != 2 || to.Expressions()[0].Kind() != exp.KindDataTypeParam {
		t.Fatalf("DECIMAL params mismatch:\n%s", cast.ToS())
	}

	cast = parseOne(t, "x::int")
	if cast.Kind() != exp.KindCast || exprArg(t, cast, "this").Kind() != exp.KindColumn || !exp.IsType(exprArg(t, cast, "to"), exp.DTypeInt) {
		t.Fatalf("dcolon cast mismatch:\n%s", cast.ToS())
	}

	for _, sql := range []string{"TRY_CAST(x AS INT)", "SAFE_CAST(x AS INT)"} {
		tryCast := parseOne(t, sql)
		if tryCast.Kind() != exp.KindTryCast || tryCast.Arg("safe") != true {
			t.Fatalf("%s mismatch:\n%s", sql, tryCast.ToS())
		}
	}
}

func TestParseBracketAndArray(t *testing.T) {
	proj := parseOne(t, "SELECT a[0] FROM t").Expressions()[0]
	if proj.Kind() != exp.KindBracket || exprArg(t, proj, "this").Kind() != exp.KindColumn {
		t.Fatalf("bracket mismatch:\n%s", proj.ToS())
	}

	proj = parseOne(t, "SELECT a[0].b FROM t").Expressions()[0]
	if proj.Kind() != exp.KindDot || exprArg(t, proj, "this").Kind() != exp.KindBracket {
		t.Fatalf("bracket dot mismatch:\n%s", proj.ToS())
	}

	proj = parseOne(t, "SELECT [1, 2]").Expressions()[0]
	if proj.Kind() != exp.KindArray || len(proj.Expressions()) != 2 {
		t.Fatalf("array literal mismatch:\n%s", proj.ToS())
	}
}

func TestParseSpecialFunctions(t *testing.T) {
	cases := []struct {
		sql  string
		kind exp.Kind
	}{
		{"EXTRACT(DAY FROM x)", exp.KindExtract},
		{"SUBSTRING(x FROM 1 FOR 2)", exp.KindSubstring},
		{"TRIM(BOTH ' ' FROM x)", exp.KindTrim},
		{"POSITION(a IN b)", exp.KindStrPosition},
		{"CEIL(x)", exp.KindCeil},
		{"FLOOR(x, 2)", exp.KindFloor},
		{"STRING_AGG(x, ',')", exp.KindGroupConcat},
	}
	for _, tc := range cases {
		expression := parseOne(t, tc.sql)
		if expression.Kind() != tc.kind {
			t.Fatalf("%s kind = %v, want %v:\n%s", tc.sql, expression.Kind(), tc.kind, expression.ToS())
		}
	}
	trim := parseOne(t, "TRIM(BOTH ' ' FROM x)")
	if trim.Arg("position") != "BOTH" {
		t.Fatalf("trim position mismatch:\n%s", trim.ToS())
	}

	// EXTRACT's first arg alternatives must short-circuit (mirror Python `or`): when
	// the function/literal alternative matches, the FROM keyword must survive so the
	// FROM branch is taken (LOCAL fix for eager firstExpression / parseVarOrString).
	for _, sql := range []string{"EXTRACT(foo() FROM x)", "EXTRACT('lit' FROM x)", "EXTRACT(DAY FROM x)"} {
		extract := parseOne(t, sql)
		if extract.Kind() != exp.KindExtract {
			t.Fatalf("%s kind mismatch:\n%s", sql, extract.ToS())
		}
		if expr := exprArg(t, extract, "expression"); expr.Kind() != exp.KindColumn || expr.Name() != "x" {
			t.Fatalf("%s FROM operand mismatch:\n%s", sql, extract.ToS())
		}
	}
}

func TestNestedTypes(t *testing.T) {
	to := exprArg(t, parseOne(t, "CAST(x AS ARRAY<INT>)"), "to")
	if !exp.IsType(to, exp.DTypeArray) || len(to.Expressions()) != 1 || !exp.IsType(to.Expressions()[0], exp.DTypeInt) {
		t.Fatalf("ARRAY type mismatch:\n%s", to.ToS())
	}

	to = exprArg(t, parseOne(t, "CAST(x AS STRUCT<a INT, b STRING>)"), "to")
	if !exp.IsType(to, exp.DTypeStruct) || len(to.Expressions()) != 2 || to.Expressions()[0].Kind() != exp.KindColumnDef {
		t.Fatalf("STRUCT type mismatch:\n%s", to.ToS())
	}

	to = exprArg(t, parseOne(t, "CAST(x AS MAP<STRING, INT>)"), "to")
	if !exp.IsType(to, exp.DTypeMap) || len(to.Expressions()) != 2 {
		t.Fatalf("MAP type mismatch:\n%s", to.ToS())
	}

	to = exprArg(t, parseOne(t, "CAST(x AS MAP[STRING=>INT])"), "to")
	if !exp.IsType(to, exp.DTypeMap) || len(to.Expressions()) != 2 {
		t.Fatalf("MAP bracket type mismatch:\n%s", to.ToS())
	}

	to = exprArg(t, parseOne(t, "CAST(x AS ARRAY<STRING COLLATE utf8>)"), "to")
	if !exp.IsType(to, exp.DTypeArray) || len(to.Expressions()) != 1 || to.Expressions()[0].Arg("collate") == nil {
		t.Fatalf("COLLATE nested type mismatch:\n%s", to.ToS())
	}

	to = exprArg(t, parseOne(t, "CAST(x AS INT UNSIGNED)"), "to")
	if !exp.IsType(to, exp.DTypeUInt) {
		t.Fatalf("UNSIGNED type mismatch:\n%s", to.ToS())
	}

	to = exprArg(t, parseOne(t, "CAST(x AS TIMESTAMP WITH TIME ZONE)"), "to")
	if !exp.IsType(to, exp.DTypeTimestampTz) {
		t.Fatalf("TIMESTAMP WITH TIME ZONE mismatch:\n%s", to.ToS())
	}

	to = exprArg(t, parseOne(t, "CAST(x AS INT[])"), "to")
	if !exp.IsType(to, exp.DTypeArray) || len(to.Expressions()) != 1 || !exp.IsType(to.Expressions()[0], exp.DTypeInt) {
		t.Fatalf("array suffix type mismatch:\n%s", to.ToS())
	}

	to = exprArg(t, parseOne(t, "CAST(x AS NULLABLE(INT))"), "to")
	if !exp.IsType(to, exp.DTypeInt) || to.Arg("nullable") != true {
		t.Fatalf("NULLABLE type mismatch:\n%s", to.ToS())
	}

	// Top-level CAST(... AS <type> COLLATE ...) must parse (with_collation=True),
	// not hard-error on the COLLATE token (LOCAL fix; parser.py:7863).
	to = exprArg(t, parseOne(t, "CAST(x AS VARCHAR COLLATE utf8)"), "to")
	if !exp.IsType(to, exp.DTypeVarchar) || to.Arg("collate") == nil {
		t.Fatalf("top-level COLLATE cast mismatch:\n%s", to.ToS())
	}
}

// A fixed-size array column definition (`col INT[3]`) must parse into a structured
// Create with an ARRAY DataType carrying values, not degrade to a Command. This
// exercises parseTypes' schema=true path via _parse_column_def (LOCAL fix).
func TestFixedSizeArrayColumn(t *testing.T) {
	create := parseOne(t, "CREATE TABLE t (col INT[3])")
	if create.Kind() != exp.KindCreate {
		t.Fatalf("fixed-size array column should parse to Create:\n%s", create.ToS())
	}
	col := exprArg(t, create, "this").Expressions()[0]
	kind := exprArg(t, col, "kind")
	if !exp.IsType(kind, exp.DTypeArray) || len(expressionsForArg(kind, "values")) != 1 {
		t.Fatalf("fixed-size array type mismatch:\n%s", kind.ToS())
	}
	if len(kind.Expressions()) != 1 || !exp.IsType(kind.Expressions()[0], exp.DTypeInt) {
		t.Fatalf("fixed-size array element type mismatch:\n%s", kind.ToS())
	}
}

func TestIntervalType(t *testing.T) {
	to := exprArg(t, parseOne(t, "CAST(x AS INTERVAL DAY)"), "to")
	interval := exprArg(t, to, "this")
	if interval.Kind() != exp.KindInterval {
		t.Fatalf("INTERVAL type inner kind = %v, want Interval:\n%s", interval.Kind(), to.ToS())
	}
	unit := exprArg(t, interval, "unit")
	if unit.Kind() != exp.KindVar || unit.Name() != "DAY" {
		t.Fatalf("INTERVAL unit mismatch:\n%s", interval.ToS())
	}
}

func TestIntervalLiteral(t *testing.T) {
	for _, sql := range []string{"INTERVAL '1' DAY", "INTERVAL 1 DAY", "INTERVAL '1 day'"} {
		interval := parseOne(t, sql)
		if interval.Kind() != exp.KindInterval {
			t.Fatalf("%s: kind = %v, want Interval:\n%s", sql, interval.Kind(), interval.ToS())
		}
		unit := exprArg(t, interval, "unit")
		if unit.Kind() != exp.KindVar {
			t.Fatalf("%s: unit kind = %v, want Var:\n%s", sql, unit.Kind(), interval.ToS())
		}
	}

	interval := parseOne(t, "INTERVAL '1' DAY TO SECOND")
	unit := exprArg(t, interval, "unit")
	if unit.Kind() != exp.KindIntervalSpan {
		t.Fatalf("INTERVAL span unit kind = %v, want IntervalSpan:\n%s", unit.Kind(), interval.ToS())
	}

	// Numeric interval literals canonicalize to a string literal via Python-style str():
	// integers drop no digits, integer-valued decimals keep ".0", and Neg keeps its sign.
	for _, tc := range []struct{ sql, want string }{
		{"INTERVAL 1 DAY", "1"},
		{"INTERVAL 1.0 DAY", "1.0"},
		{"INTERVAL 1.5 DAY", "1.5"},
		{"INTERVAL -1 DAY", "-1"},
	} {
		iv := parseOne(t, tc.sql)
		if got := exprArg(t, iv, "this").Name(); got != tc.want {
			t.Fatalf("%s: interval value = %q, want %q:\n%s", tc.sql, got, tc.want, iv.ToS())
		}
	}
}

// TestParseLiteralDcolonCast ports the parseAtom/parseType round-trip gap: a NUMBER/STRING
// literal immediately followed by a column operator (`::`, `.`, `[`, ...) must NOT be
// consumed directly by parseAtom (parser.py:6560-6583's COLUMN_OPERATORS/COLUMN_POSTFIX_TOKENS
// guard), so it instead flows through parseColumn -> parseColumnOps and gets the cast/bracket
// handling. Before this port, `1::int` (a bare literal head) failed to parse entirely.
func TestParseLiteralDcolonCast(t *testing.T) {
	for _, sql := range []string{"1::int", "'x'::int"} {
		cast := parseOne(t, sql)
		if cast.Kind() != exp.KindCast || exprArg(t, cast, "this").Kind() != exp.KindLiteral || !exp.IsType(exprArg(t, cast, "to"), exp.DTypeInt) {
			t.Fatalf("%s: literal dcolon cast mismatch:\n%s", sql, cast.ToS())
		}
		got, err := generateSQL(t, cast, "")
		if err != nil {
			t.Fatalf("%s: Generate: %v", sql, err)
		}
		if want := "CAST(" + sql[:strings.Index(sql, "::")] + " AS INT)"; got != want {
			t.Fatalf("%s: round-trip = %q, want %q", sql, got, want)
		}
	}
}

// TestParseBareTypeExpression ports the parseType top-level DataType round-trip (parser.py:
// 6155-6217): a bare `ARRAY<...>`/`STRUCT<...>` type expression (no CAST/`::` wrapper) parses
// to a DataType, and STRUCT<int INT> exercises the double-type-token guard in parseStructTypes
// (parser.py:6526-6534: the field name "int" is itself a TYPE_TOKEN).
func TestParseBareTypeExpression(t *testing.T) {
	for _, sql := range []string{"ARRAY<TEXT>", "ARRAY<STRUCT<INT, DOUBLE, ARRAY<INT>>>", "STRUCT<int INT>"} {
		dtype := parseOne(t, sql)
		if dtype.Kind() != exp.KindDataType {
			t.Fatalf("%s: kind = %v, want DataType:\n%s", sql, dtype.Kind(), dtype.ToS())
		}
		got, err := generateSQL(t, dtype, "")
		if err != nil {
			t.Fatalf("%s: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%s: round-trip = %q, want %q", sql, got, sql)
		}
	}
}

// TestParseInlineConstructor covers _parse_types' values-suffix (parser.py:6375-6381,
// 6436-6438): ARRAY<INT>[1, 2] / STRUCT<a INT>(1, 'foo') build a Cast of an Array/Struct
// constructor to the nested type, returned through parseType's KindCast fast-path.
func TestParseInlineConstructor(t *testing.T) {
	arr := parseOne(t, "ARRAY<INT>[1, 2]")
	if arr.Kind() != exp.KindCast {
		t.Fatalf("array constructor kind = %v, want Cast:\n%s", arr.Kind(), arr.ToS())
	}
	if inner := exprArg(t, arr, "this"); inner.Kind() != exp.KindArray {
		t.Fatalf("array constructor `this` = %v, want Array:\n%s", inner.Kind(), arr.ToS())
	}
	if to := exprArg(t, arr, "to"); !exp.IsType(to, exp.DTypeArray) {
		t.Fatalf("array constructor `to` not ARRAY:\n%s", arr.ToS())
	}

	st := parseOne(t, "STRUCT<a INT, b STRING>(1, 'foo')")
	if st.Kind() != exp.KindCast {
		t.Fatalf("struct constructor kind = %v, want Cast:\n%s", st.Kind(), st.ToS())
	}
	if inner := exprArg(t, st, "this"); inner.Kind() != exp.KindStruct {
		t.Fatalf("struct constructor `this` = %v, want Struct:\n%s", inner.Kind(), st.ToS())
	}
}

// TestParseAtTimeZone ports _parse_at_time_zone (parser.py:6553-6558), wired into
// _parse_factor (parser.py:6119-6121): `<this> AT TIME ZONE <zone>`, right-nesting on repeat,
// and available for any operand (column, function call, or a parenthesized CAST).
func TestParseAtTimeZone(t *testing.T) {
	atz := parseOne(t, "x AT TIME ZONE 'UTC'")
	if atz.Kind() != exp.KindAtTimeZone || exprArg(t, atz, "this").Kind() != exp.KindColumn {
		t.Fatalf("AT TIME ZONE mismatch:\n%s", atz.ToS())
	}
	if zone := exprArg(t, atz, "zone"); zone.Kind() != exp.KindLiteral || zone.Name() != "UTC" {
		t.Fatalf("AT TIME ZONE zone mismatch:\n%s", atz.ToS())
	}

	chained := parseOne(t, "CURRENT_DATE AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Tokyo'")
	if chained.Kind() != exp.KindAtTimeZone {
		t.Fatalf("chained AT TIME ZONE kind = %v, want AtTimeZone:\n%s", chained.Kind(), chained.ToS())
	}
	inner := exprArg(t, chained, "this")
	if inner.Kind() != exp.KindAtTimeZone {
		t.Fatalf("chained AT TIME ZONE should nest, got inner kind %v:\n%s", inner.Kind(), chained.ToS())
	}

	for _, sql := range []string{
		"x AT TIME ZONE 'UTC'",
		"CURRENT_DATE AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Tokyo'",
		"CURRENT_DATE AT TIME ZONE zone_column",
		"CAST('2025-11-20 00:00:00+00' AS TIMESTAMP) AT TIME ZONE 'Africa/Cairo'",
	} {
		root := parseOne(t, sql)
		got, err := generateSQL(t, root, "")
		if err != nil {
			t.Fatalf("%s: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%s: round-trip = %q, want %q", sql, got, sql)
		}
	}

	// MySQL has no AT TIME ZONE syntax: it parses (so the zone doesn't leak into trailing
	// unconsumed tokens) but the generator drops the zone (generators/mysql.py:796-798).
	mysqlATZ := parseOneDialect(t, "SELECT foo AT TIME ZONE 'UTC'", "mysql")
	got, err := generateSQL(t, mysqlATZ, "mysql")
	if err != nil {
		t.Fatalf("mysql AT TIME ZONE: Generate: %v", err)
	}
	if want := "SELECT foo"; got != want {
		t.Fatalf("mysql AT TIME ZONE round-trip = %q, want %q", got, want)
	}
}

// TestParsePseudoTypeAndObjectIdentifier ports the PSEUDO_TYPE/OBJECT_IDENTIFIER branches of
// parseTypes (parser.py:6273-6277): postgres pseudo-types (CSTRING) and object identifier
// types (OID/REGCLASS/...) round-trip as their own node kinds, not KindDataType.
func TestParsePseudoTypeAndObjectIdentifier(t *testing.T) {
	cstring := exprArg(t, parseOneDialect(t, "x::cstring", "postgres"), "to")
	if cstring.Kind() != exp.KindPseudoType || cstring.Name() != "CSTRING" {
		t.Fatalf("PSEUDO_TYPE mismatch:\n%s", cstring.ToS())
	}

	regclass := exprArg(t, parseOneDialect(t, "x::regclass", "postgres"), "to")
	if regclass.Kind() != exp.KindObjectIdentifier || regclass.Name() != "REGCLASS" {
		t.Fatalf("OBJECT_IDENTIFIER mismatch:\n%s", regclass.ToS())
	}

	for _, sql := range []string{"x::cstring", "x::oid", "x::regclass", "x::regtype"} {
		root := parseOneDialect(t, sql, "postgres")
		got, err := generateSQL(t, root, "postgres")
		if err != nil {
			t.Fatalf("%s: Generate: %v", sql, err)
		}
		want := "CAST(x AS " + strings.ToUpper(sql[strings.Index(sql, "::")+2:]) + ")"
		if got != want {
			t.Fatalf("%s: round-trip = %q, want %q", sql, got, want)
		}
	}
}

// TestParseUserDefinedType ports the identifier-fallback half of parseTypes (parser.py:
// 6253-6271) plus parseUserDefinedType (parser.py:6231-6237, and the postgres override at
// parsers/postgres.py:339-347): a type name that isn't a TYPE_TOKEN falls back to a
// user-defined DataType. Base joins dotted parts into a plain string "kind"; postgres instead
// builds an Identifier/Dot chain so quoting round-trips exactly. MySQL has
// SUPPORTS_USER_DEFINED_TYPES=false, so it must error instead of silently misparsing.
func TestParseUserDefinedType(t *testing.T) {
	base := exprArg(t, parseOne(t, "CAST(x AS FOO)"), "to")
	if !exp.IsType(base, exp.DTypeUserDefined) || base.Arg("kind") != "FOO" {
		t.Fatalf("base UDT fallback mismatch (want plain-string kind):\n%s", base.ToS())
	}

	for _, sql := range []string{
		`CAST(5 AS "MyType")`,
		`CAST(5 AS "MySchema"."MyType")`,
		`CAST(5 AS "MyCatalog"."MySchema"."MyType")`,
		`CAST(5 AS MySchema."MyType")`,
		`CAST(5 AS MySchema.MyType)`,
		`CAST(5 AS MyType)`,
	} {
		root := parseOneDialect(t, sql, "postgres")
		to := exprArg(t, root, "to")
		if !exp.IsType(to, exp.DTypeUserDefined) {
			t.Fatalf("%s: to.this = %v, want USERDEFINED:\n%s", sql, to.Arg("this"), root.ToS())
		}
		kind := to.Arg("kind")
		if _, isExpr := kind.(exp.Expression); !isExpr {
			t.Fatalf("%s: postgres UDT kind should be an Identifier/Dot expression, got %#v:\n%s", sql, kind, root.ToS())
		}
		got, err := generateSQL(t, root, "postgres")
		if err != nil {
			t.Fatalf("%s: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%s: round-trip = %q, want %q", sql, got, sql)
		}
	}

	if _, err := sqlglot.ParseOne("CAST(x AS FOO)", "mysql"); err == nil {
		t.Fatalf("mysql CAST to an unrecognized type name should error (SUPPORTS_USER_DEFINED_TYPES=false)")
	}
}
