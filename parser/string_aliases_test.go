package parser_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// TestStringAliasesFlag pins the per-dialect STRING_ALIASES port (parser.py:1780;
// parsers/mysql.py:302). A trailing string constant is folded into an identifier alias in
// parseAlias only when the dialect sets StringAliases: base/postgres leave it false and fail
// closed on `SELECT 1 'x'` (matching upstream base + postgres, both of which raise, and the real
// engines - PostgreSQL rejects it, MySQL accepts it), while MySQL folds it to a quoted alias.
func TestStringAliasesFlag(t *testing.T) {
	// base ("") and postgres: STRING_ALIASES = False -> a trailing string is never absorbed as an
	// alias, so the leftover string is an unexpected token and parsing fails closed.
	for _, dialect := range []string{"", "postgres"} {
		if _, err := sqlglot.ParseOne("SELECT 1 'x'", dialect); err == nil {
			t.Fatalf("ParseOne(%q, %q): expected an error (STRING_ALIASES=false), got nil", "SELECT 1 'x'", dialect)
		}
	}

	// MySQL: STRING_ALIASES = True -> the trailing string folds into a quoted identifier alias.
	mysqlExpr := parseOneDialect(t, "SELECT 1 'x'", "mysql")
	got, err := generateSQL(t, mysqlExpr, "mysql")
	if err != nil {
		t.Fatalf("generate mysql: %v", err)
	}
	if want := "SELECT 1 AS `x`"; got != want {
		t.Fatalf("mysql SELECT 1 'x' = %q, want %q", got, want)
	}

	// The postgres `<type-name> 'string'` space typed-literal (pg-user-type-typed-literal) is
	// recognized at the primary-expression level, BEFORE alias parsing, so gating the alias by the
	// flag must not regress it: it still folds into a Cast, for both a schema-qualified name and a
	// bare non-reserved type-name keyword.
	for _, tc := range []struct {
		sql  string
		want string
	}{
		{"SELECT public.foo 'x'", "SELECT CAST('x' AS public.foo)"},
		{"SELECT current_schema 'x'", "SELECT CAST('x' AS current_schema)"},
	} {
		castExpr := parseOneDialect(t, tc.sql, "postgres")
		proj := castExpr.Expressions()[0]
		if proj.Kind() != exp.KindCast {
			t.Fatalf("postgres %q projection kind = %v, want KindCast:\n%s", tc.sql, proj.Kind(), castExpr.ToS())
		}
		out, err := generateSQL(t, castExpr, "postgres")
		if err != nil {
			t.Fatalf("generate postgres %q: %v", tc.sql, err)
		}
		if out != tc.want {
			t.Fatalf("postgres %q = %q, want %q", tc.sql, out, tc.want)
		}
	}

}

// TestStringTableIdentifiersFlag pins the port-introduced StringTableIdentifiers gate on a bare
// string used as a table NAME (`FROM 'foo'`, _parse_table_part parser.py:4668) or table ALIAS
// (`FROM t 'x'`, _parse_table_alias parser.py:4111). Upstream calls both UNCONDITIONALLY (no flag),
// but real PostgreSQL 17.6 and MySQL 8.0.33 BOTH reject either form, so postgres/mysql gate it off
// (a §1 correctness deviation - follow the engine); base keeps upstream's permissive accept. This is
// orthogonal to StringAliases: MySQL accepts the projection `SELECT 1 'x'` yet rejects the table
// forms below.
func TestStringTableIdentifiersFlag(t *testing.T) {
	tableStringForms := []string{"SELECT * FROM t 'x'", "SELECT * FROM 'foo'"}

	// postgres + mysql: real engines reject a bare string in table-name/alias position -> fail closed.
	for _, dialect := range []string{"postgres", "mysql"} {
		for _, sql := range tableStringForms {
			if _, err := sqlglot.ParseOne(sql, dialect); err == nil {
				t.Errorf("ParseOne(%q, %q): expected an error (StringTableIdentifiers=false, real engine rejects), got nil", sql, dialect)
			}
		}
	}

	// base: no real-engine reference, so it stays at upstream's unconditional accept.
	for _, sql := range tableStringForms {
		if _, err := sqlglot.ParseOne(sql, ""); err != nil {
			t.Errorf("base %q should still parse (StringTableIdentifiers defaults true, matching upstream): %v", sql, err)
		}
	}

	// The gate must not disturb ordinary identifier/aliased tables in postgres/mysql.
	for _, dialect := range []string{"postgres", "mysql", ""} {
		for _, sql := range []string{"SELECT * FROM t", "SELECT * FROM t AS x", "SELECT * FROM t x"} {
			if _, err := sqlglot.ParseOne(sql, dialect); err != nil {
				t.Errorf("%q [%s]: ordinary table ref should parse: %v", sql, dialect, err)
			}
		}
	}

	// Carve-out: an *explicit* AS + string table alias (`FROM t AS 'x'`) is NOT gated (it goes
	// through parseIdVar's advanceAny path, matching upstream) - a separate pre-existing quirk, like
	// the projection `SELECT 1 AS 'x'` case. Documented in DEVIATIONS §1.9.
	for _, dialect := range []string{"postgres", "mysql", ""} {
		if _, err := sqlglot.ParseOne("SELECT * FROM t AS 'x'", dialect); err != nil {
			t.Errorf("`SELECT * FROM t AS 'x'` [%s]: explicit AS+string alias is out of scope, want accept: %v", dialect, err)
		}
	}
}

// The presto/trino/hive/athena partial dialects inherit StringTableIdentifiers=true through Base()
// (they never override it), so they keep upstream's permissive accept - pinned here so a future edit
// to one of their constructors can't silently flip the field to the zero value.
func TestStringTableIdentifiersPartialDialectsAccept(t *testing.T) {
	for _, dialect := range []string{"presto", "trino", "hive", "athena"} {
		for _, sql := range []string{"SELECT * FROM 'foo'", "SELECT * FROM t 'x'"} {
			if _, err := sqlglot.ParseOne(sql, dialect); err != nil {
				t.Errorf("%q [%s]: partial dialect should still accept (StringTableIdentifiers inherits true): %v", sql, dialect, err)
			}
		}
	}
}

// CREATE-family DDL and GRANT wrap their structured parse in tryParse and degrade to a raw-text
// Command on any internal failure, so a rejected string table name there surfaces as a fail-closed
// Command (not a hard parse error, unlike SELECT/DML). Pinned so the fail-closed shape is explicit;
// see DEVIATIONS §1.9.
func TestStringTableIdentifiersDDLDegradesToCommand(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql"} {
		for _, sql := range []string{
			"CREATE TABLE 'foo' (a INT)",
			"CREATE VIEW 'foo' AS SELECT 1",
			"GRANT SELECT ON 'foo' TO bob",
		} {
			e, err := sqlglot.ParseOne(sql, dialect)
			if err != nil {
				// A hard error is also acceptable fail-closed; only a structured (Create/Grant) node
				// carrying the invalid string table would be a leak.
				continue
			}
			if e.Kind() != exp.KindCommand {
				t.Errorf("%q [%s]: Kind = %s, want Command (fail-closed) or a parse error\n%s", sql, dialect, exp.ClassName(e.Kind()), e.ToS())
			}
		}
		// A valid identifier-named CREATE still parses structurally (the gate must not over-reject).
		if e, err := sqlglot.ParseOne("CREATE TABLE foo (a INT)", dialect); err != nil || e.Kind() != exp.KindCreate {
			t.Errorf("CREATE TABLE foo [%s]: want structured Create, got err=%v", dialect, err)
		}
	}
}
