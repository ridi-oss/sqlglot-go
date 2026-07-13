package dialects_test

// Tests for the per-dialect Dialect.Functions overlay (dialects/dialect.go) and its two
// concrete dialect wirings (dialects/mysql.go, dialects/postgres.go): closes parity_gaps.txt
// gaps 103-108,110-111,113,114,159,160,177,178 (see AGENTS.md's "port 1:1" note - these are
// verified against .reference/sqlglot-v30.12.0's parsers/mysql.py, parsers/postgres.py, and
// expressions/temporal.py|string.py|functions.py).

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	"github.com/ridi/sqlglot-go/generator"
)

// dialectRoundTrip parses sql under dialect and regenerates it in that same dialect, failing
// the test on any parse/generate error (mirrors generator_test.roundTrip, duplicated here
// since that helper lives in an unexported test package this one doesn't import).
func dialectRoundTrip(t *testing.T, dialect, sql string) string {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q, %q) error: %v", sql, dialect, err)
	}
	got, err := sqlglot.Generate(expression, dialect, generator.Options{})
	if err != nil {
		t.Fatalf("Generate(%q, %q) error: %v", sql, dialect, err)
	}
	return got
}

// TestMySQLFunctionsOverlayKeys guards the exact key set of dialects.MySQL().Functions: only
// the genuinely mysql-only names remain here (parsers/mysql.py:106-166). The DayOf*/WeekOfYear
// and LCASE/UCASE spellings are base-scope upstream and now live in the shared
// exp.FunctionByName (expressions/functions.go), so base and postgres canonicalize them too;
// they are intentionally NOT in this overlay anymore (see TestMySQLFunctionsRoundTrip and the
// base_dayofmonth/base_lcase cases in parity_residual_test.go).
func TestMySQLFunctionsOverlayKeys(t *testing.T) {
	mysql, err := dialects.GetOrRaise("mysql")
	if err != nil {
		t.Fatalf("GetOrRaise(mysql): %v", err)
	}
	wantKeys := []string{"CURDATE", "CURTIME", "DATABASE", "SCHEMA", "INSTR", "TIME_STR_TO_UNIX"}
	for _, key := range wantKeys {
		if mysql.Functions[key] == nil {
			t.Errorf("mysql.Functions[%q] = nil, want a builder", key)
		}
	}
	// The base-scope names that moved to exp.FunctionByName must no longer be in the mysql
	// overlay (mysql resolves them through the merged base map instead).
	for _, key := range []string{"DAY_OF_MONTH", "DAY_OF_WEEK", "DAY_OF_YEAR", "WEEK_OF_YEAR", "LCASE", "UCASE"} {
		if mysql.Functions[key] != nil {
			t.Errorf("mysql.Functions[%q] should now be unset (moved to base exp.FunctionByName)", key)
		}
	}

	base, err := dialects.GetOrRaise("")
	if err != nil {
		t.Fatalf("GetOrRaise(base): %v", err)
	}
	for _, key := range wantKeys {
		if base.Functions[key] != nil {
			t.Errorf("base.Functions[%q] should be unset (mysql-only overlay)", key)
		}
	}
}

// TestPostgresFunctionsOverlayKeys guards dialects.Postgres().Functions: CHAR_LENGTH and
// CHARACTER_LENGTH only (functions.go:100-105's warning - LENGTH itself is deliberately left
// unregistered since it already round-trips via Anonymous).
func TestPostgresFunctionsOverlayKeys(t *testing.T) {
	postgres, err := dialects.GetOrRaise("postgres")
	if err != nil {
		t.Fatalf("GetOrRaise(postgres): %v", err)
	}
	for _, key := range []string{"CHAR_LENGTH", "CHARACTER_LENGTH"} {
		if postgres.Functions[key] == nil {
			t.Errorf("postgres.Functions[%q] = nil, want a builder", key)
		}
	}
	if postgres.Functions["LENGTH"] != nil {
		t.Errorf("postgres.Functions[\"LENGTH\"] should be unset (LENGTH already round-trips via Anonymous)")
	}
}

// TestMySQLFunctionsRoundTrip exercises the full merge path (parser/parser.go's
// mergedDialectFunctions plus the funcTokens gate for the DATABASE/MOD/SCHEMA keyword
// tokens) end to end via the public sqlglot API, one case per parity_gaps.txt row closed.
func TestMySQLFunctionsRoundTrip(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT CURTIME()", "SELECT CURRENT_TIME()"},
		{"SELECT DAY_OF_MONTH('2023-01-01')", "SELECT DAYOFMONTH('2023-01-01')"},
		{"SELECT DAY_OF_WEEK('2023-01-01')", "SELECT DAYOFWEEK('2023-01-01')"},
		{"SELECT DAY_OF_YEAR('2023-01-01')", "SELECT DAYOFYEAR('2023-01-01')"},
		{"SELECT INSTR('str', 'substr')", "SELECT LOCATE('substr', 'str')"},
		{"SELECT LCASE('foo')", "SELECT LOWER('foo')"},
		{"SELECT UCASE('foo')", "SELECT UPPER('foo')"},
		{"SELECT WEEK_OF_YEAR('2023-01-01')", "SELECT WEEKOFYEAR('2023-01-01')"},
		{"TIME_STR_TO_UNIX(x)", "UNIX_TIMESTAMP(x)"},
		{"TRUNC(3.14159, 2)", "TRUNCATE(3.14159, 2)"},
		{"DATABASE()", "SCHEMA()"},
		{"MOD(x, y)", "x % y"},
		// build_mod's Binary-operand Paren wrap (parser.py:126-127): MOD(a + 1, 7) must
		// round-trip with explicit parens, or `%`'s precedence would silently reassociate.
		{"SELECT MOD(a + 1, 7)", "SELECT (a + 1) % 7"},
	}
	for _, tc := range cases {
		if got := dialectRoundTrip(t, "mysql", tc.sql); got != tc.want {
			t.Errorf("mysql %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestPostgresFunctionsRoundTrip mirrors TestMySQLFunctionsRoundTrip for the postgres overlay.
func TestPostgresFunctionsRoundTrip(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"CHARACTER_LENGTH(x)", "LENGTH(x)"},
		{"CHAR_LENGTH(x)", "LENGTH(x)"},
		{"LENGTH(x)", "LENGTH(x)"},
	}
	for _, tc := range cases {
		if got := dialectRoundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestMODBaseAndPostgresUnaffectedByFuncTokensGate guards that the funcTokens gate extension
// added alongside the Functions overlay (parser/parser.go's parseFunctionCall) doesn't change
// MOD's behavior outside mysql: base and postgres never tokenize "MOD" as a dedicated
// keyword (only mysql's tokenizer config does), so they reach exp.FunctionByName["MOD"]
// (expressions/functions.go) through the ordinary VAR/funcTokens path, unaffected by the
// gate's extra dialect.Functions/FunctionByName lookup.
func TestMODBaseAndPostgresUnaffectedByFuncTokensGate(t *testing.T) {
	for _, dialect := range []string{"", "postgres"} {
		if got := dialectRoundTrip(t, dialect, "MOD(x, y)"); got != "x % y" {
			t.Errorf("%s MOD(x, y) -> %q, want %q", dialect, got, "x % y")
		}
	}
}
