package sqlglot_test

import (
	"strings"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Tests for the NON-UPSTREAM opt-in `opaque_functions` dialect setting (DEVIATIONS): name-lookup
// builtin calls parse as exp.Anonymous with the name as written and round-trip verbatim; keyword-
// grammar forms keep their structured nodes and render form-faithfully.

func opaqueRoundTrip(t *testing.T, dialect, sql string) (exp.Expression, string) {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q, %q): %v", sql, dialect, err)
	}
	out, err := sqlglot.Generate(e, dialect, generator.Options{})
	if err != nil {
		t.Fatalf("Generate(%q, %q): %v", sql, dialect, err)
	}
	return e, out
}

func firstCall(t *testing.T, root exp.Expression, kinds ...exp.Kind) exp.Expression {
	t.Helper()
	for _, k := range kinds {
		if n := root.Find(k); n != nil {
			return n
		}
	}
	t.Fatalf("no node of kinds %v in:\n%s", kinds, root.ToS())
	return nil
}

func TestOpaqueFunctionsVerbatim(t *testing.T) {
	// Every name-lookup call — builtin-recognized or UDF, any spelling/case, qualified or not —
	// parses Anonymous and generates back byte-identical.
	for _, tt := range []struct{ dialect, sql string }{
		{"postgres, opaque_functions=true", "SELECT if(a > 1, 1, 2)"},
		{"postgres, opaque_functions=true", "SELECT ifnull(a, 0) FROM t"},
		{"postgres, opaque_functions=true", "SELECT my_udf(a, b)"},
		{"postgres, opaque_functions=true", "SELECT pg_catalog.abs(x)"},
		{"postgres, opaque_functions=true", "SELECT lower('X'), ABS(-1), Coalesce(a, b)"},
		{"postgres, opaque_functions=true", "SELECT ceil(x)"},
		{"postgres, opaque_functions=true", "SELECT now(), version()"},
		{"postgres, opaque_functions=true", "SELECT date_part('year', c)"},
		{"postgres, opaque_functions=true", "SELECT substring('hello', 1, 3)"},
		{"postgres, opaque_functions=true", "SELECT trim('  x  ')"},
		{"postgres, opaque_functions=true", `SELECT "My_Func"(x)`},
		{"postgres, opaque_functions=true", "SELECT unnest(arr)"},
		{"mysql, opaque_functions=true", "SELECT ifnull(a, 0)"},
		{"mysql, opaque_functions=true", "SELECT char(65)"},
		{"mysql, opaque_functions=true", "SELECT substr('hello', 1, 3)"},
		{"mysql, opaque_functions=true", "SELECT SUBSTRING('hello', 1, 3)"},
		{"mysql, opaque_functions=true", "SELECT ucase(x), dayofmonth(d)"},
		// Nested grammar/opaque calls must not clobber the outer call's as-written name
		// (regression: opaqueFuncName captured at form-split parser entry).
		{"mysql, opaque_functions=true", "SELECT SUBSTRING(CAST(x AS CHAR), 1, 3)"},
		{"mysql, opaque_functions=true", "SELECT CHAR(CAST(65 AS UNSIGNED))"},
		{"postgres, opaque_functions=true", "SELECT position(CAST(a AS CHAR), b)"},
		{"postgres, opaque_functions=true", "SELECT substring(TRIM(BOTH 'x' FROM s), 1, 2)"},
		{"postgres, opaque_functions=true", "SELECT trim(substring(a, 1, 2))"},
		{"postgres, opaque_functions=true", "SELECT ceil(CAST(x AS DOUBLE PRECISION))"},
		// Quoted name colliding with a grammar parser stays a quoted Anonymous call.
		{"postgres, opaque_functions=true", `SELECT "TRIM"(x)`},
		// COLLATE inside an argument is consumed by the argument grammar, call stays opaque.
		{"postgres, opaque_functions=true", `SELECT trim(x COLLATE "C")`},
		// STRING_AGG comma form (with or without in-paren ORDER BY) is a name-lookup call.
		{"postgres, opaque_functions=true", "SELECT string_agg(x, ',')"},
		{"postgres, opaque_functions=true", "SELECT string_agg(x, ',' ORDER BY y)"},
		{", opaque_functions=true", "SELECT if(a > 1, 'x', 'y')"}, // base dialect + settings
	} {
		e, out := opaqueRoundTrip(t, tt.dialect, tt.sql)
		if out != tt.sql {
			t.Errorf("[%s] %q round-tripped as %q, want verbatim", tt.dialect, tt.sql, out)
		}
		if n := e.Find(exp.KindAnonymous); n == nil {
			t.Errorf("[%s] %q: no Anonymous node:\n%s", tt.dialect, tt.sql, e.ToS())
		}
		// Idempotency: a second parse∘generate cycle is a fixpoint.
		if _, out2 := opaqueRoundTrip(t, tt.dialect, out); out2 != out {
			t.Errorf("[%s] not idempotent: %q -> %q", tt.dialect, out, out2)
		}
	}
}

func TestOpaqueFunctionsGrammarFormsStructured(t *testing.T) {
	// Keyword-grammar forms keep their structured Kinds and render their source grammar form
	// (form-faithful; keyword case may normalize but the form is a fixpoint).
	for _, tt := range []struct {
		dialect, sql, want string
		kind               exp.Kind
	}{
		{"postgres, opaque_functions=true", "SELECT CAST(x AS INT)", "SELECT CAST(x AS INT)", exp.KindCast},
		{"postgres, opaque_functions=true", "SELECT EXTRACT(YEAR FROM ts)", "SELECT EXTRACT(YEAR FROM ts)", exp.KindExtract},
		{"postgres, opaque_functions=true", "SELECT TRIM(BOTH 'x' FROM s)", "SELECT TRIM(BOTH 'x' FROM s)", exp.KindTrim},
		{"postgres, opaque_functions=true", "SELECT SUBSTRING('hello' FROM 1 FOR 3)", "SELECT SUBSTRING('hello' FROM 1 FOR 3)", exp.KindSubstring},
		{"postgres, opaque_functions=true", "SELECT POSITION('l' IN 'hello')", "SELECT POSITION('l' IN 'hello')", exp.KindStrPosition},
		{"mysql, opaque_functions=true", "SELECT SUBSTRING('hello' FROM 1 FOR 3)", "SELECT SUBSTRING('hello' FROM 1 FOR 3)", exp.KindSubstring},
		{"mysql, opaque_functions=true", "SELECT CHAR(65 USING utf8)", "SELECT CHAR(65 USING utf8)", exp.KindChr},
		{"mysql, opaque_functions=true", "SELECT CEIL(x TO unit)", "SELECT CEIL(x TO unit)", exp.KindCeil},
		// Keyword TRIM without remove-chars keeps its grammar form (never TRIM(s)/LTRIM(s)).
		{"postgres, opaque_functions=true", "SELECT TRIM(BOTH FROM s)", "SELECT TRIM(BOTH FROM s)", exp.KindTrim},
		{"postgres, opaque_functions=true", "SELECT TRIM(LEADING FROM s)", "SELECT TRIM(LEADING FROM s)", exp.KindTrim},
		{"postgres, opaque_functions=true", "SELECT TRIM(FROM s)", "SELECT TRIM(FROM s)", exp.KindTrim},
		// CHAR(... USING cs) renders CHAR on every dialect under the flag — never CHR.
		{"postgres, opaque_functions=true", "SELECT CHAR(65 USING utf8)", "SELECT CHAR(65 USING utf8)", exp.KindChr},
		// WITHIN GROUP is keyword grammar and stays structured.
		{"postgres, opaque_functions=true", "SELECT STRING_AGG(x, ',') WITHIN GROUP (ORDER BY y)", "SELECT STRING_AGG(x, ',') WITHIN GROUP (ORDER BY y)", exp.KindGroupConcat},
	} {
		e, out := opaqueRoundTrip(t, tt.dialect, tt.sql)
		if out != tt.want {
			t.Errorf("[%s] %q generated %q, want %q", tt.dialect, tt.sql, out, tt.want)
		}
		firstCall(t, e, tt.kind)
		if _, out2 := opaqueRoundTrip(t, tt.dialect, out); out2 != out {
			t.Errorf("[%s] not idempotent: %q -> %q", tt.dialect, out, out2)
		}
	}
}

func TestOpaqueFunctionsAggregateWindowGrammar(t *testing.T) {
	// Wrapper grammar (DISTINCT args, OVER, FILTER) still applies around the opaque call.
	for _, sql := range []string{
		"SELECT count(DISTINCT x) FROM t",
		"SELECT count(*) FROM t",
		"SELECT array_agg(x ORDER BY y) FROM t",
		"SELECT sum(x) OVER (PARTITION BY y) FROM t",
		"SELECT count(x) FILTER(WHERE x > 0) FROM t",
	} {
		e, out := opaqueRoundTrip(t, "postgres, opaque_functions=true", sql)
		if out != sql {
			t.Errorf("%q round-tripped as %q, want verbatim", sql, out)
		}
		if n := e.Find(exp.KindAnonymous); n == nil {
			t.Errorf("%q: call did not parse Anonymous:\n%s", sql, e.ToS())
		}
	}
}

func TestOpaqueFunctionsNiladicsAndIfKeyword(t *testing.T) {
	// Bare no-paren niladics keep their structured nodes; IF(...) is opaque but the MySQL
	// keyword IF ... THEN statement form is untouched.
	e, out := opaqueRoundTrip(t, "postgres, opaque_functions=true", "SELECT CURRENT_DATE, now()")
	if out != "SELECT CURRENT_DATE, now()" {
		t.Errorf("niladics: got %q", out)
	}
	firstCall(t, e, exp.KindCurrentDate)

	e, _ = opaqueRoundTrip(t, "postgres, opaque_functions=true", "SELECT if(a, b, c)")
	call := firstCall(t, e, exp.KindAnonymous)
	if call.Arg("this") != "if" {
		t.Errorf("IF( name = %v, want as-written \"if\"", call.Arg("this"))
	}
	if e.Find(exp.KindIf) != nil {
		t.Errorf("IF(...) built a KindIf node under opaque_functions:\n%s", e.ToS())
	}
}

func TestOpaqueFunctionsSettingPlumbing(t *testing.T) {
	// Settings-string resolution in any position; SettingsString round-trip; default off.
	d, err := dialects.GetOrRaise("mysql, mysql_version=80036, opaque_functions=true, mysql_ansi_quotes=true")
	if err != nil {
		t.Fatal(err)
	}
	if !d.OpaqueFunctions {
		t.Fatal("opaque_functions=true not applied")
	}
	if !strings.Contains(d.SettingsString(), "opaque_functions=true") {
		t.Errorf("SettingsString() = %q, want opaque_functions=true", d.SettingsString())
	}
	if d2, _ := dialects.GetOrRaise("mysql, opaque_functions=true, opaque_functions=false"); d2.OpaqueFunctions {
		t.Error("last-value-wins violated for duplicated opaque_functions")
	}
	if d3, _ := dialects.GetOrRaise("postgres"); d3.OpaqueFunctions {
		t.Error("default must be off")
	}
	// Default-off behavior unchanged: builtin recognition still active.
	e, err := sqlglot.ParseOne("SELECT ifnull(a, 0)", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if e.Find(exp.KindCoalesce) == nil {
		t.Errorf("flag-off IFNULL should still build Coalesce:\n%s", e.ToS())
	}
}

func TestOpaqueFunctionsMySQLValuesPreserved(t *testing.T) {
	// MySQL VALUES(col) in ON DUPLICATE KEY UPDATE keeps its Anonymous shape + col arg.
	sql := "INSERT INTO t (a) VALUES (1) ON DUPLICATE KEY UPDATE a = VALUES(a)"
	e, out := opaqueRoundTrip(t, "mysql, opaque_functions=true", sql)
	if out != sql {
		t.Errorf("round-tripped as %q", out)
	}
	found := false
	for _, n := range e.FindAll(exp.KindAnonymous) {
		if name, _ := n.Arg("this").(string); strings.EqualFold(name, "VALUES") {
			found = true
		}
	}
	if !found {
		t.Errorf("no Anonymous(VALUES) node:\n%s", e.ToS())
	}
}
