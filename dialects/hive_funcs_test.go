package dialects_test

import (
	"sort"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	exp "github.com/ridi/sqlglot-go/expressions"
)

var hiveOverlayKeys = []string{
	"BASE64", "COLLECT_LIST", "COLLECT_SET", "DATE_ADD", "DATE_FORMAT", "DATE_SUB",
	"DATEDIFF", "DAY", "FIRST", "FIRST_VALUE", "FROM_UNIXTIME", "GET_JSON_OBJECT",
	"LAST", "LAST_VALUE", "LOG", "MAP", "MONTH", "NAMED_STRUCT", "REGEXP_EXTRACT",
	"REGEXP_EXTRACT_ALL", "SEQUENCE", "SIZE", "SPLIT", "STR_TO_MAP", "TO_DATE",
	"TO_JSON", "TRUNC", "UNBASE64", "UNIX_TIMESTAMP", "YEAR",
}

func parseHiveFunction(t *testing.T, sql string) exp.Expression {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, "hive")
	if err != nil {
		t.Fatalf("ParseOne(%q, hive): %v", sql, err)
	}
	return expression
}

func hiveFunctionArg(t *testing.T, expression exp.Expression, key string) exp.Expression {
	t.Helper()
	child, ok := expression.Arg(key).(exp.Expression)
	if !ok || child == nil {
		t.Fatalf("%v missing expression arg %q:\n%s", expression.Kind(), key, expression.ToS())
	}
	return child
}

func TestHiveFunctionsOverlayKeys(t *testing.T) {
	hive, err := dialects.GetOrRaise("hive")
	if err != nil {
		t.Fatalf("GetOrRaise(hive): %v", err)
	}

	gotKeys := make([]string, 0, len(hive.Functions))
	for key, builder := range hive.Functions {
		if builder == nil {
			t.Errorf("hive.Functions[%q] = nil, want a builder", key)
		}
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	wantKeys := append([]string(nil), hiveOverlayKeys...)
	sort.Strings(wantKeys)
	if len(gotKeys) != len(wantKeys) {
		t.Fatalf("Hive overlay keys = %v, want %v", gotKeys, wantKeys)
	}
	for i, want := range wantKeys {
		if gotKeys[i] != want {
			t.Fatalf("Hive overlay keys = %v, want %v", gotKeys, wantKeys)
		}
	}

	base := dialects.Base()
	for _, key := range hiveOverlayKeys {
		if base.Functions[key] != nil {
			t.Errorf("base.Functions[%q] should be unset (Hive overlay leaked to base)", key)
		}
	}
}

func TestHiveFunctionsParseToKind(t *testing.T) {
	cases := []struct {
		sql  string
		kind exp.Kind
	}{
		{"BASE64(x)", exp.KindToBase64},
		{"COLLECT_LIST(x)", exp.KindArrayAgg},
		{"COLLECT_SET(x)", exp.KindArrayUniqueAgg},
		{"DATE_ADD(x, 1)", exp.KindTsOrDsAdd},
		{"DATE_FORMAT(x, 'yyyy-MM-dd')", exp.KindTimeToStr},
		{"DATE_SUB(x, 1)", exp.KindTsOrDsAdd},
		{"DATEDIFF(a, b)", exp.KindDateDiff},
		{"DAY(x)", exp.KindDay},
		{"FIRST(x)", exp.KindFirst},
		{"FIRST_VALUE(x)", exp.KindFirstValue},
		{"FROM_UNIXTIME(x)", exp.KindUnixToStr},
		{"GET_JSON_OBJECT(x, '$.a')", exp.KindJSONExtractScalar},
		{"LAST(x)", exp.KindLast},
		{"LAST_VALUE(x)", exp.KindLastValue},
		{"LOG(x)", exp.KindLn},
		{"MAP(a, b)", exp.KindVarMap},
		{"MAP(*)", exp.KindStarMap},
		{"MONTH(x)", exp.KindMonth},
		{"NAMED_STRUCT('a', x)", exp.KindStruct},
		{"REGEXP_EXTRACT(x, '(a)')", exp.KindRegexpExtract},
		{"REGEXP_EXTRACT_ALL(x, '(a)')", exp.KindRegexpExtractAll},
		{"SEQUENCE(a, b)", exp.KindGenerateSeries},
		{"SIZE(x)", exp.KindArraySize},
		{"SPLIT(x, ',')", exp.KindRegexpSplit},
		{"STR_TO_MAP(x)", exp.KindStrToMap},
		{"TO_DATE(x)", exp.KindTsOrDsToDate},
		{"TO_JSON(x)", exp.KindJSONFormat},
		{"TRUNC(x, 'MM')", exp.KindTimestampTrunc},
		{"UNBASE64(x)", exp.KindFromBase64},
		{"UNIX_TIMESTAMP(x)", exp.KindStrToUnix},
		{"YEAR(x)", exp.KindYear},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			expression := parseHiveFunction(t, tc.sql)
			if expression.Kind() != tc.kind {
				t.Fatalf("kind = %v, want %v:\n%s", expression.Kind(), tc.kind, expression.ToS())
			}
		})
	}
}

func TestHiveCollectListAndDateShapes(t *testing.T) {
	collect := parseHiveFunction(t, "COLLECT_LIST(x)")
	if collect.Kind() != exp.KindArrayAgg || collect.Arg("nulls_excluded") != true {
		t.Fatalf("COLLECT_LIST should be ArrayAgg(nulls_excluded=true):\n%s", collect.ToS())
	}

	dateSub := parseHiveFunction(t, "DATE_SUB(d, n)")
	if dateSub.Kind() != exp.KindTsOrDsAdd || dateSub.This() == nil || dateSub.This().Name() != "d" {
		t.Fatalf("DATE_SUB TsOrDsAdd target mismatch:\n%s", dateSub.ToS())
	}
	multiplier := hiveFunctionArg(t, dateSub, "expression")
	if multiplier.Kind() != exp.KindMul || multiplier.This() == nil || multiplier.This().Name() != "n" {
		t.Fatalf("DATE_SUB delta should be n * -1:\n%s", dateSub.ToS())
	}
	negativeOne := hiveFunctionArg(t, multiplier, "expression")
	if negativeOne.Kind() != exp.KindNeg || negativeOne.This() == nil || negativeOne.This().Kind() != exp.KindLiteral || negativeOne.This().Name() != "1" {
		t.Fatalf("DATE_SUB multiplier should end in Neg(Literal(1)):\n%s", dateSub.ToS())
	}

	dateDiff := parseHiveFunction(t, "DATEDIFF(a, b)")
	if dateDiff.Kind() != exp.KindDateDiff {
		t.Fatalf("DATEDIFF kind = %v, want DateDiff:\n%s", dateDiff.Kind(), dateDiff.ToS())
	}
	left := dateDiff.This()
	right := hiveFunctionArg(t, dateDiff, "expression")
	if left == nil || left.Kind() != exp.KindTsOrDsToDate || left.This() == nil || left.This().Name() != "a" {
		t.Fatalf("DATEDIFF left operand should be TsOrDsToDate(a):\n%s", dateDiff.ToS())
	}
	if right.Kind() != exp.KindTsOrDsToDate || right.This() == nil || right.This().Name() != "b" {
		t.Fatalf("DATEDIFF right operand should be TsOrDsToDate(b):\n%s", dateDiff.ToS())
	}

	for _, tc := range []struct {
		sql  string
		kind exp.Kind
	}{
		{"DAY(x)", exp.KindDay},
		{"MONTH(x)", exp.KindMonth},
		{"YEAR(x)", exp.KindYear},
	} {
		expression := parseHiveFunction(t, tc.sql)
		if expression.Kind() != tc.kind || expression.This() == nil || expression.This().Kind() != exp.KindTsOrDsToDate || expression.This().This() == nil || expression.This().This().Name() != "x" {
			t.Fatalf("%s should wrap x in TsOrDsToDate:\n%s", tc.sql, expression.ToS())
		}
	}
}

func TestHiveFirstLastIgnoreNulls(t *testing.T) {
	for _, tc := range []struct {
		name          string
		kind          exp.Kind
		supportsFalse bool
	}{
		{"FIRST", exp.KindFirst, true},
		{"FIRST_VALUE", exp.KindFirstValue, false},
		{"LAST", exp.KindLast, true},
		{"LAST_VALUE", exp.KindLastValue, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ignored := parseHiveFunction(t, tc.name+"(x, TRUE)")
			if ignored.Kind() != exp.KindIgnoreNulls || ignored.This() == nil || ignored.This().Kind() != tc.kind || ignored.This().This() == nil || ignored.This().This().Name() != "x" {
				t.Fatalf("%s(..., TRUE) should be IgnoreNulls(%v):\n%s", tc.name, tc.kind, ignored.ToS())
			}
			if tc.supportsFalse {
				included := parseHiveFunction(t, tc.name+"(x, FALSE)")
				if included.Kind() != tc.kind || included.This() == nil || included.This().Name() != "x" {
					t.Fatalf("%s(..., FALSE) should stay %v:\n%s", tc.name, tc.kind, included.ToS())
				}
			}
		})
	}
}

func TestHiveMapAndNamedStructShapes(t *testing.T) {
	starMap := parseHiveFunction(t, "MAP(*)")
	if starMap.Kind() != exp.KindStarMap || starMap.This() == nil || starMap.This().Kind() != exp.KindStar {
		t.Fatalf("MAP(*) should be StarMap(Star):\n%s", starMap.ToS())
	}

	varMap := parseHiveFunction(t, "MAP(a, b, c, d)")
	if varMap.Kind() != exp.KindVarMap {
		t.Fatalf("MAP pairs should be VarMap:\n%s", varMap.ToS())
	}
	keys := hiveFunctionArg(t, varMap, "keys")
	values := hiveFunctionArg(t, varMap, "values")
	if keys.Kind() != exp.KindArray || values.Kind() != exp.KindArray {
		t.Fatalf("VarMap keys/values should be Array nodes:\n%s", varMap.ToS())
	}
	if len(keys.Expressions()) != 2 || keys.Expressions()[0].Name() != "a" || keys.Expressions()[1].Name() != "c" {
		t.Fatalf("VarMap keys mismatch:\n%s", varMap.ToS())
	}
	if len(values.Expressions()) != 2 || values.Expressions()[0].Name() != "b" || values.Expressions()[1].Name() != "d" {
		t.Fatalf("VarMap values mismatch:\n%s", varMap.ToS())
	}

	named := parseHiveFunction(t, "NAMED_STRUCT('a', x, 'b', 2)")
	if named.Kind() != exp.KindStruct || len(named.Expressions()) != 2 {
		t.Fatalf("NAMED_STRUCT should contain two fields:\n%s", named.ToS())
	}
	for i, want := range []struct {
		name, value string
	}{
		{"a", "x"},
		{"b", "2"},
	} {
		field := named.Expressions()[i]
		if field.Kind() != exp.KindPropertyEQ || field.This() == nil || field.This().Kind() != exp.KindIdentifier || field.This().Name() != want.name {
			t.Fatalf("NAMED_STRUCT field %d key mismatch:\n%s", i, named.ToS())
		}
		if value := hiveFunctionArg(t, field, "expression"); value.Name() != want.value {
			t.Fatalf("NAMED_STRUCT field %d value = %q, want %q:\n%s", i, value.Name(), want.value, named.ToS())
		}
	}
}

func TestHiveRegexpAndTimeFormatShapes(t *testing.T) {
	regexpExtract := parseHiveFunction(t, "REGEXP_EXTRACT(x, '(a)')")
	if regexpExtract.Kind() != exp.KindRegexpExtract || regexpExtract.Arg("null_if_pos_overflow") != true {
		t.Fatalf("REGEXP_EXTRACT should set null_if_pos_overflow=true:\n%s", regexpExtract.ToS())
	}
	group := hiveFunctionArg(t, regexpExtract, "group")
	if group.Kind() != exp.KindLiteral || group.Name() != "1" {
		t.Fatalf("REGEXP_EXTRACT should inject group 1:\n%s", regexpExtract.ToS())
	}

	regexpExtractAll := parseHiveFunction(t, "REGEXP_EXTRACT_ALL(x, '(a)')")
	if regexpExtractAll.Kind() != exp.KindRegexpExtractAll || regexpExtractAll.Arg("null_if_pos_overflow") != nil {
		t.Fatalf("REGEXP_EXTRACT_ALL should omit null_if_pos_overflow:\n%s", regexpExtractAll.ToS())
	}
	group = hiveFunctionArg(t, regexpExtractAll, "group")
	if group.Kind() != exp.KindLiteral || group.Name() != "1" {
		t.Fatalf("REGEXP_EXTRACT_ALL should inject group 1:\n%s", regexpExtractAll.ToS())
	}

	dateFormat := parseHiveFunction(t, "DATE_FORMAT(x, 'yyyy-MM-dd HH:mm:ss')")
	if dateFormat.Kind() != exp.KindTimeToStr || dateFormat.This() == nil || dateFormat.This().Kind() != exp.KindTimeStrToTime || dateFormat.This().This() == nil || dateFormat.This().This().Name() != "x" {
		t.Fatalf("DATE_FORMAT should be TimeToStr(TimeStrToTime(x)):\n%s", dateFormat.ToS())
	}
	if format := hiveFunctionArg(t, dateFormat, "format"); format.Name() != "%Y-%m-%d %H:%M:%S" {
		t.Fatalf("DATE_FORMAT converted format = %q, want %%Y-%%m-%%d %%H:%%M:%%S:\n%s", format.Name(), dateFormat.ToS())
	}

	fromUnix := parseHiveFunction(t, "FROM_UNIXTIME(x)")
	if fromUnix.Kind() != exp.KindUnixToStr || hiveFunctionArg(t, fromUnix, "format").Name() != "%Y-%m-%d %H:%M:%S" {
		t.Fatalf("FROM_UNIXTIME should inject Hive's default time format:\n%s", fromUnix.ToS())
	}

	toDate := parseHiveFunction(t, "TO_DATE(x, 'yyyy-MM-dd')")
	if toDate.Kind() != exp.KindTsOrDsToDate || toDate.Arg("safe") != true || hiveFunctionArg(t, toDate, "format").Name() != "%Y-%m-%d" {
		t.Fatalf("TO_DATE should be safe and convert its format:\n%s", toDate.ToS())
	}
}

func TestHiveTimestampTruncUnitCaseSensitivity(t *testing.T) {
	cases := []struct {
		unit string
		want string
	}{
		{unit: "d", want: "D"},
		{unit: "ms", want: "MS"},
		{unit: "q", want: "Q"},
		{unit: "D", want: "DAY"},
		{unit: "MS", want: "MILLISECOND"},
		{unit: "MM", want: "MM"},
	}
	for _, tc := range cases {
		t.Run(tc.unit, func(t *testing.T) {
			expression := parseHiveFunction(t, "TRUNC(x, '"+tc.unit+"')")
			unit := hiveFunctionArg(t, expression, "unit")
			if unit.Kind() != exp.KindVar || unit.Name() != tc.want {
				t.Fatalf("TRUNC unit %q = %v(%q), want Var(%q):\n%s", tc.unit, unit.Kind(), unit.Name(), tc.want, expression.ToS())
			}
		})
	}
}

func TestHiveUnixTimestampAndLogShapes(t *testing.T) {
	current := parseHiveFunction(t, "UNIX_TIMESTAMP()")
	if current.Kind() != exp.KindStrToUnix || current.This() == nil || current.This().Kind() != exp.KindCurrentTimestamp {
		t.Fatalf("UNIX_TIMESTAMP() should use CurrentTimestamp:\n%s", current.ToS())
	}
	if format := hiveFunctionArg(t, current, "format"); format.Name() != "%Y-%m-%d %H:%M:%S" {
		t.Fatalf("UNIX_TIMESTAMP() default format = %q:\n%s", format.Name(), current.ToS())
	}

	formatted := parseHiveFunction(t, "UNIX_TIMESTAMP(x, 'yyyy-MM-dd')")
	if formatted.Kind() != exp.KindStrToUnix || formatted.This() == nil || formatted.This().Name() != "x" || hiveFunctionArg(t, formatted, "format").Name() != "%Y-%m-%d" {
		t.Fatalf("UNIX_TIMESTAMP explicit format mismatch:\n%s", formatted.ToS())
	}

	ln := parseHiveFunction(t, "LOG(x)")
	if ln.Kind() != exp.KindLn || ln.This() == nil || ln.This().Name() != "x" {
		t.Fatalf("one-argument Hive LOG should be Ln:\n%s", ln.ToS())
	}
	log := parseHiveFunction(t, "LOG(2, x)")
	if log.Kind() != exp.KindLog || log.This() == nil || log.This().Name() != "2" || hiveFunctionArg(t, log, "expression").Name() != "x" {
		t.Fatalf("two-argument Hive LOG should preserve base/expression order:\n%s", log.ToS())
	}
}
