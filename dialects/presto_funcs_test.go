package dialects_test

// Tests for the Presto per-dialect Dialect.Functions overlay (dialects/presto.go), ported 1:1
// from parsers/presto.py:74-135 with the slice's deferral policy applied (see the presto.go
// header comment + ROADMAP known-divergences). This slice ports the PARSER + TOKENIZER only; the
// Presto generator TRANSFORMS/TYPE_MAPPING are out of scope, so structured functions whose
// canonical class-name differs from the Presto spelling round-trip to the canonical name (the
// same "transform" situation dialect_funcs_test.go already locks in for mysql MOD -> `%` and
// postgres CHARACTER_LENGTH -> LENGTH). dialectRoundTrip is defined in dialect_funcs_test.go.

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	exp "github.com/ridi/sqlglot-go/expressions"
)

// prestoOverlayKeys is the exact key set registered by dialects.Presto().Functions (the
// non-deferred subset of parsers/presto.py:74-135).
var prestoOverlayKeys = []string{
	"ARBITRARY", "APPROX_DISTINCT", "APPROX_PERCENTILE",
	"BITWISE_AND", "BITWISE_NOT", "BITWISE_OR", "BITWISE_XOR",
	"CARDINALITY", "CONTAINS", "DATE_ADD", "DATE_DIFF",
	"DAY_OF_WEEK", "DOW", "DOY", "ELEMENT_AT", "FROM_HEX",
	"FROM_UNIXTIME", "FROM_UTF8", "JSON_FORMAT", "LEVENSHTEIN_DISTANCE",
	"NOW", "REPLACE", "ROW", "SEQUENCE", "SET_AGG", "SPLIT_TO_MAP",
	"STRPOS", "SLICE", "TO_UNIXTIME", "TO_UTF8", "MD5", "SHA256", "SHA512", "WEEK",
}

// prestoDeferredKeys are the FUNCTIONS entries deliberately left Anonymous this slice (they need
// build_formatted_time / date_trunc_to_time / build_regexp_extract helpers not yet ported), so
// they must NOT appear in the overlay.
var prestoDeferredKeys = []string{
	"DATE_FORMAT", "DATE_PARSE", "DATE_TRUNC", "TO_CHAR",
	"REGEXP_EXTRACT", "REGEXP_EXTRACT_ALL", "REGEXP_REPLACE",
}

// TestPrestoFunctionsOverlayKeys guards the exact key set of dialects.Presto().Functions: every
// non-deferred entry is present, every deferred entry is absent, and none of them leaked into the
// base overlay (they are presto-only additions).
func TestPrestoFunctionsOverlayKeys(t *testing.T) {
	presto, err := dialects.GetOrRaise("presto")
	if err != nil {
		t.Fatalf("GetOrRaise(presto): %v", err)
	}
	for _, key := range prestoOverlayKeys {
		if presto.Functions[key] == nil {
			t.Errorf("presto.Functions[%q] = nil, want a builder", key)
		}
	}
	for _, key := range prestoDeferredKeys {
		if presto.Functions[key] != nil {
			t.Errorf("presto.Functions[%q] should be unset (deferred: stays Anonymous this slice)", key)
		}
	}

	base, err := dialects.GetOrRaise("")
	if err != nil {
		t.Fatalf("GetOrRaise(base): %v", err)
	}
	for _, key := range prestoOverlayKeys {
		if base.Functions[key] != nil {
			t.Errorf("base.Functions[%q] should be unset (presto-only overlay leaked to base)", key)
		}
	}
}

// TestPrestoFunctionsParseToKind proves the parser overlay does its job: each Presto function
// spelling parses to a node of the expected sqlglot Kind (the real point of this parser slice,
// independent of generator rendering). The reorder/injection closures are additionally pinned by
// TestPrestoFunctionsRoundTrip below.
func TestPrestoFunctionsParseToKind(t *testing.T) {
	cases := []struct {
		sql  string
		kind exp.Kind
	}{
		{"SELECT ARBITRARY(x)", exp.KindAnyValue},
		{"SELECT APPROX_DISTINCT(x)", exp.KindApproxDistinct},
		{"SELECT APPROX_PERCENTILE(x, 0.5, 100)", exp.KindApproxQuantile},
		{"SELECT APPROX_PERCENTILE(x, w, 0.5, 100)", exp.KindApproxQuantile},
		{"SELECT BITWISE_AND(a, b)", exp.KindBitwiseAnd},
		{"SELECT BITWISE_NOT(a)", exp.KindBitwiseNot},
		{"SELECT BITWISE_OR(a, b)", exp.KindBitwiseOr},
		{"SELECT BITWISE_XOR(a, b)", exp.KindBitwiseXor},
		{"SELECT CARDINALITY(x)", exp.KindArraySize},
		{"SELECT CONTAINS(x, 1)", exp.KindArrayContains},
		{"SELECT DATE_ADD('day', 1, ts)", exp.KindDateAdd},
		{"SELECT DATE_DIFF('day', a, b)", exp.KindDateDiff},
		{"SELECT DAY_OF_WEEK(x)", exp.KindDayOfWeekIso},
		{"SELECT DOW(x)", exp.KindDayOfWeekIso},
		{"SELECT DOY(x)", exp.KindDayOfYear},
		{"SELECT ELEMENT_AT(m, k)", exp.KindBracket},
		{"SELECT FROM_HEX(x)", exp.KindUnhex},
		{"SELECT FROM_UNIXTIME(x)", exp.KindUnixToTime},
		{"SELECT FROM_UNIXTIME(x, z)", exp.KindUnixToTime},
		{"SELECT FROM_UNIXTIME(x, h, m)", exp.KindUnixToTime},
		{"SELECT FROM_UTF8(x)", exp.KindDecode},
		{"SELECT JSON_FORMAT(x)", exp.KindJSONFormat},
		{"SELECT LEVENSHTEIN_DISTANCE(a, b)", exp.KindLevenshtein},
		{"SELECT NOW()", exp.KindCurrentTimestamp},
		{"SELECT REPLACE(a, b, c)", exp.KindReplace},
		{"SELECT REPLACE(a, b)", exp.KindReplace},
		{"SELECT ROW(a, b)", exp.KindStruct},
		{"SELECT SEQUENCE(a, b)", exp.KindGenerateSeries},
		{"SELECT SET_AGG(x)", exp.KindArrayUniqueAgg},
		{"SELECT SPLIT_TO_MAP(a, b, c)", exp.KindStrToMap},
		{"SELECT STRPOS(a, b, 2)", exp.KindStrPosition},
		{"SELECT SLICE(a, 1, 2)", exp.KindArraySlice},
		{"SELECT TO_UNIXTIME(x)", exp.KindTimeToUnix},
		{"SELECT TO_UTF8(x)", exp.KindEncode},
		{"SELECT MD5(x)", exp.KindMD5Digest},
		{"SELECT SHA256(x)", exp.KindSHA2},
		{"SELECT SHA512(x)", exp.KindSHA2},
		{"SELECT WEEK(x)", exp.KindWeekOfYear},
	}
	for _, tc := range cases {
		expression, err := sqlglot.ParseOne(tc.sql, "presto")
		if err != nil {
			t.Errorf("ParseOne(%q, presto): %v", tc.sql, err)
			continue
		}
		if len(expression.FindAll(tc.kind)) == 0 {
			t.Errorf("presto %q did not parse to a %v node", tc.sql, tc.kind)
		}
	}
}

// TestPrestoFunctionsRoundTrip pins the actual presto->presto rendering of the overlay, covering
// each custom closure's argument handling: the DATE_ADD/DATE_DIFF unit/expression/this reorder
// (parsers/presto.py:85-90), APPROX_PERCENTILE's 3/4-arg weight reorder, FROM_UNIXTIME's 2/3-arg
// zone-vs-hours/minutes split, FROM_UTF8/TO_UTF8's injected "utf-8" charset, ELEMENT_AT's Bracket
// lowering, STRPOS's occurrence arg, and NOW()'s CurrentTimestamp. The output SQL is the canonical
// class-name (not the Presto spelling) because the Presto generator is out of scope this slice -
// same as dialect_funcs_test.go's mysql MOD/postgres CHARACTER_LENGTH transform assertions. The
// SHA2/MD5Digest/JSONFormat render their upstream _sql_names via the generator's sqlNameOverrides
// (JSON_FORMAT / MD5_DIGEST / SHA2), matching .reference presto-read base-write; they are pinned
// below alongside the closures.
func TestPrestoFunctionsRoundTrip(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT APPROX_DISTINCT(x)", "SELECT APPROX_DISTINCT(x)"},
		{"SELECT JSON_FORMAT(x)", "SELECT JSON_FORMAT(x)"},
		{"SELECT MD5(x)", "SELECT MD5_DIGEST(x)"},
		{"SELECT SHA256(x)", "SELECT SHA2(x, 256)"},
		{"SELECT SHA512(x)", "SELECT SHA2(x, 512)"},
		{"SELECT ARBITRARY(x)", "SELECT ANY_VALUE(x)"},
		{"SELECT CARDINALITY(x)", "SELECT ARRAY_LENGTH(x)"},
		{"SELECT CONTAINS(x, 1)", "SELECT ARRAY_CONTAINS(x, 1)"},
		{"SELECT APPROX_PERCENTILE(x, 0.5, 100)", "SELECT APPROX_QUANTILE(x, 0.5, 100)"},
		{"SELECT APPROX_PERCENTILE(x, w, 0.5, 100)", "SELECT APPROX_QUANTILE(x, 0.5, 100, w)"},
		{"SELECT BITWISE_AND(a, b)", "SELECT a & b"},
		{"SELECT BITWISE_OR(a, b)", "SELECT a | b"},
		{"SELECT BITWISE_XOR(a, b)", "SELECT a ^ b"},
		{"SELECT BITWISE_NOT(a)", "SELECT ~a"},
		{"SELECT DATE_ADD('day', 1, ts)", "SELECT DATE_ADD(ts, 1, 'day')"},
		{"SELECT DATE_DIFF('day', a, b)", "SELECT DATEDIFF(b, a, 'day')"},
		{"SELECT DAY_OF_WEEK(x)", "SELECT DAYOFWEEK_ISO(x)"},
		{"SELECT DOW(x)", "SELECT DAYOFWEEK_ISO(x)"},
		{"SELECT DOY(x)", "SELECT DAY_OF_YEAR(x)"},
		{"SELECT ELEMENT_AT(m, k)", "SELECT m[k]"},
		{"SELECT FROM_HEX(x)", "SELECT UNHEX(x)"},
		{"SELECT FROM_UNIXTIME(x, z)", "SELECT UNIX_TO_TIME(x, z)"},
		{"SELECT FROM_UNIXTIME(x, h, m)", "SELECT UNIX_TO_TIME(x, h, m)"},
		{"SELECT FROM_UTF8(x)", "SELECT DECODE(x, 'utf-8')"},
		{"SELECT LEVENSHTEIN_DISTANCE(a, b)", "SELECT LEVENSHTEIN(a, b)"},
		{"SELECT NOW()", "SELECT CURRENT_TIMESTAMP()"},
		{"SELECT REPLACE(a, b, c)", "SELECT REPLACE(a, b, c)"},
		{"SELECT ROW(a, b)", "SELECT STRUCT(a, b)"},
		{"SELECT SEQUENCE(a, b)", "SELECT GENERATE_SERIES(a, b)"},
		{"SELECT SET_AGG(x)", "SELECT ARRAY_UNIQUE_AGG(x)"},
		{"SELECT SPLIT_TO_MAP(a, b, c)", "SELECT STR_TO_MAP(a, b, c)"},
		{"SELECT STRPOS(a, b, 2)", "SELECT STR_POSITION(a, b, 2)"},
		{"SELECT SLICE(a, 1, 2)", "SELECT ARRAY_SLICE(a, 1, 2)"},
		{"SELECT TO_UNIXTIME(x)", "SELECT TIME_TO_UNIX(x)"},
		{"SELECT TO_UTF8(x)", "SELECT ENCODE(x, 'utf-8')"},
		{"SELECT WEEK(x)", "SELECT WEEK_OF_YEAR(x)"},
	}
	for _, tc := range cases {
		if got := dialectRoundTrip(t, "presto", tc.sql); got != tc.want {
			t.Errorf("presto %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestPrestoDeferredFunctionsStayAnonymous confirms the deferred FUNCTIONS entries fall through to
// Anonymous and round-trip verbatim (fail-closed until the missing helpers are ported).
func TestPrestoDeferredFunctionsStayAnonymous(t *testing.T) {
	cases := []string{
		"SELECT DATE_FORMAT(x, '%Y')",
		"SELECT DATE_PARSE(x, '%Y')",
		"SELECT DATE_TRUNC('day', x)",
		"SELECT TO_CHAR(x, 'y')",
		"SELECT REGEXP_EXTRACT(x, y)",
		"SELECT REGEXP_EXTRACT_ALL(x, y)",
		"SELECT REGEXP_REPLACE(x, y, z)",
	}
	for _, sql := range cases {
		expression, err := sqlglot.ParseOne(sql, "presto")
		if err != nil {
			t.Errorf("ParseOne(%q, presto): %v", sql, err)
			continue
		}
		if len(expression.FindAll(exp.KindAnonymous)) == 0 {
			t.Errorf("presto %q should stay Anonymous (deferred), but parsed to a structured node", sql)
		}
		if got := dialectRoundTrip(t, "presto", sql); got != sql {
			t.Errorf("presto deferred %q should round-trip verbatim, got %q", sql, got)
		}
	}
}

// TestPrestoCastRowStruct verifies the ROW -> STRUCT token remap (dialects/presto.py:62) reaches
// the STRUCT-token CAST path (parser_types.go:144-146): CAST(x AS ROW(a INT, b VARCHAR)) builds a
// struct type.
func TestPrestoCastRowStruct(t *testing.T) {
	got := dialectRoundTrip(t, "presto", "SELECT CAST(x AS ROW(a INT, b VARCHAR))")
	want := "SELECT CAST(x AS STRUCT<a INT, b VARCHAR>)"
	if got != want {
		t.Errorf("presto CAST(... AS ROW(...)) ->\n  got  %q\n  want %q", got, want)
	}
}

// TestPrestoUnicodeStringRoundTrip verifies a U&'...' literal (dialects/presto.py:46-50, tokenized
// as UNICODE_STRING and parsed via the primary-parser UNICODE_STRING entry) round-trips.
func TestPrestoUnicodeStringRoundTrip(t *testing.T) {
	got := dialectRoundTrip(t, "presto", "SELECT U&'abc'")
	want := "SELECT U&'abc'"
	if got != want {
		t.Errorf("presto U&'abc' -> %q, want %q", got, want)
	}
}
