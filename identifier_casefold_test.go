package sqlglot_test

// ORIGINAL (non-ported) regression test — NOT a 1:1 port of an upstream sqlglot
// test. It pins an intentional BEHAVIORAL DEVIATION from upstream: sqlglot-go
// case-folds unquoted identifiers ASCII-only (A-Z <-> a-z, bytes >= 0x80 left
// untouched), whereas Python sqlglot folds full-Unicode via str.lower()/upper()
// (dialects/dialect.py v30.12.0:1042-1050,1055-1064). ASCII-only matches how real
// engines fold on multibyte encodings — e.g. PostgreSQL downcase_identifier
// (src/backend/parser/scansup.c) — so `CAFÉ` -> `cafÉ`, never `café`.
// See dialects/dialect.go asciiLower/asciiUpper + DEVIATIONS.md.

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
)

// normalizeVia runs the exact downstream path (parse -> normalize_identifiers ->
// generate) that a lineage consumer relies on to key off the normalized identity.
func normalizeVia(t *testing.T, sql, dialect string) string {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q, %q): %v", sql, dialect, err)
	}
	got, err := sqlglot.Generate(optimizer.NormalizeIdentifiers(e, dialect), dialect, generator.Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return got
}

// TestIdentifierCaseFoldASCIIOnly_Path exercises the end-to-end normalization
// path per dialect (the blast radius of the fix).
func TestIdentifierCaseFoldASCIIOnly_Path(t *testing.T) {
	cases := []struct{ name, dialect, in, want string }{
		// PostgreSQL (Lowercase): unquoted ASCII folds, non-ASCII É is left alone.
		{"pg unquoted mixed", "postgres", "SELECT CAFÉ FROM t", "SELECT cafÉ FROM t"},
		{"pg unquoted ascii", "postgres", "SELECT FOO FROM t", "SELECT foo FROM t"},
		// Quoted identifiers are never folded.
		{"pg quoted", "postgres", `SELECT "CAFÉ" FROM t`, `SELECT "CAFÉ" FROM t`},
		// Already in normal form: folding is a no-op (cafÉ must NOT become café,
		// and must NOT be treated as case-sensitive / needing quotes).
		{"pg idempotent", "postgres", "SELECT cafÉ FROM t", "SELECT cafÉ FROM t"},
		// MySQL (CaseSensitive): no folding at all.
		{"mysql no-op", "mysql", "SELECT CAFÉ FROM t", "SELECT CAFÉ FROM t"},
		// Presto/Trino/Athena/Hive (CaseInsensitive): fold to lower, ASCII-only.
		{"presto", "presto", "SELECT CAFÉ FROM t", "SELECT cafÉ FROM t"},
		{"trino", "trino", "SELECT CAFÉ FROM t", "SELECT cafÉ FROM t"},
		{"athena", "athena", "SELECT CAFÉ FROM t", "SELECT cafÉ FROM t"},
		{"hive", "hive", "SELECT CAFÉ FROM t", "SELECT cafÉ FROM t"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeVia(t, c.in, c.dialect); got != c.want {
				t.Fatalf("normalize %q (%s) = %q, want %q", c.in, c.dialect, got, c.want)
			}
		})
	}
}

func ident(this string, quoted bool) exp.Expression {
	return exp.Identifier(exp.Args{"this": this, "quoted": quoted})
}

func mustDialect(t *testing.T, name string) *dialects.Dialect {
	t.Helper()
	d, err := dialects.GetOrRaise(name)
	if err != nil {
		t.Fatalf("GetOrRaise(%q): %v", name, err)
	}
	return d
}

// TestNormalizeIdentifier_ASCIIFold unit-tests the dialect method directly,
// including the Uppercase strategy that no shipped dialect currently selects
// (constructed from Base()), so both fold directions are covered.
func TestNormalizeIdentifier_ASCIIFold(t *testing.T) {
	pg := mustDialect(t, "postgres") // Lowercase
	upper := dialects.Base()
	upper.NormalizationStrategy = dialects.Uppercase

	cases := []struct {
		name   string
		d      *dialects.Dialect
		this   string
		quoted bool
		want   string
	}{
		{"pg unquoted mixed -> ascii lower, É kept", pg, "CAFÉ", false, "cafÉ"},
		{"pg unquoted ascii", pg, "FOO", false, "foo"},
		{"pg quoted untouched", pg, "CAFÉ", true, "CAFÉ"},
		{"pg idempotent", pg, "cafÉ", false, "cafÉ"},
		{"upper unquoted mixed -> ascii upper, é kept", upper, "café", false, "CAFé"},
		{"upper unquoted ascii", upper, "foo", false, "FOO"},
		{"upper idempotent", upper, "CAFé", false, "CAFé"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.d.NormalizeIdentifier(ident(c.this, c.quoted))
			if name := got.Name(); name != c.want {
				t.Fatalf("NormalizeIdentifier(%q quoted=%v) = %q, want %q", c.this, c.quoted, name, c.want)
			}
		})
	}
}

// TestMySQLIdentifierFold pins the two non-upstream MySQL strategies (DEVIATIONS.md §1.2):
// they fold with MySQL's exact .tolower map (Unicode-simple, accent-PRESERVING — CAFÉ->café,
// NIÑO->niño, but Ñ!=N), regardless of quoting, and MYSQL_CASE_SENSITIVE_TABLE_NAMES keeps
// database/table names case-sensitive while folding everything else. Exercised through the
// settings-string form of GetOrRaise (the whole overridable-settings surface).
func TestMySQLIdentifierFold(t *testing.T) {
	const tbl = "mysql, normalization_strategy = mysql_case_sensitive_table_names"
	const ci = "mysql, normalization_strategy = mysql_case_insensitive"
	cases := []struct{ name, dialect, in, want string }{
		// role-aware (lower_case_table_names=0): column folds, db/table stay case-sensitive.
		{"tbl: column folds, table/db kept", tbl, "SELECT Col FROM MyDB.MyTable", "SELECT col FROM MyDB.MyTable"},
		// MySQL Unicode fold, accent-preserving.
		{"tbl: CAFÉ->café (accent kept)", tbl, "SELECT CAFÉ FROM t", "SELECT café FROM t"},
		{"tbl: NIÑO->niño (Ñ!=N)", tbl, "SELECT NIÑO FROM t", "SELECT niño FROM t"},
		// quoted column folds its case but stays quoted on output.
		{"tbl: quoted column folds, stays quoted", tbl, "SELECT `CAFÉ` FROM t", "SELECT `café` FROM t"},
		// mysql_case_insensitive folds table/db too.
		{"ci: table also folds", ci, "SELECT Col FROM MyDB.MyTable", "SELECT col FROM mydb.mytable"},
		{"ci: ß stays ß (no ss)", ci, "SELECT STRAßE FROM t", "SELECT straße FROM t"},
		// MySQL default (upstream CASE_SENSITIVE) is unchanged — no folding.
		{"default mysql: unchanged", "mysql", "SELECT CAFÉ FROM T", "SELECT CAFÉ FROM T"},
		// Postgres stays ASCII-only (proves the MySQL fold is dialect-scoped): CAFÉ->cafÉ.
		{"postgres still ASCII-only", "postgres", "SELECT CAFÉ FROM t", "SELECT cafÉ FROM t"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeVia(t, c.in, c.dialect); got != c.want {
				t.Fatalf("normalize %q (%s) = %q, want %q", c.in, c.dialect, got, c.want)
			}
		})
	}
}

// TestGetOrRaiseNormalizationSettings pins the upstream-style settings string form
// ("dialect, normalization_strategy = X") that makes the strategy overridable.
func TestGetOrRaiseNormalizationSettings(t *testing.T) {
	ok := map[string]dialects.NormalizationStrategy{
		"mysql, normalization_strategy=mysql_case_insensitive":             dialects.MySQLCaseInsensitive,
		"mysql, normalization_strategy = mysql_case_sensitive_table_names": dialects.MySQLCaseSensitiveTableNames,
		"postgres, normalization_strategy=case_sensitive":                  dialects.CaseSensitive,
	}
	for spec, want := range ok {
		d, err := dialects.GetOrRaise(spec)
		if err != nil {
			t.Fatalf("GetOrRaise(%q): %v", spec, err)
		}
		if d.NormalizationStrategy != want {
			t.Errorf("GetOrRaise(%q) strategy = %v, want %v", spec, d.NormalizationStrategy, want)
		}
	}
	// bare name is unchanged (mysql default = upstream CASE_SENSITIVE).
	if d, _ := dialects.GetOrRaise("mysql"); d.NormalizationStrategy != dialects.CaseSensitive {
		t.Errorf("bare mysql strategy = %v, want CaseSensitive", d.NormalizationStrategy)
	}
	// errors: unknown strategy value, unknown setting key, unknown dialect.
	for _, bad := range []string{
		"mysql, normalization_strategy = nonsense",
		"mysql, lower_case_table_names = 1",
		"notadialect",
	} {
		if _, err := dialects.GetOrRaise(bad); err == nil {
			t.Errorf("GetOrRaise(%q): expected error, got nil", bad)
		}
	}
}

// TestCaseSensitive_ASCIIOnly pins that "case sensitivity" (does folding change
// it → must be quoted to preserve) is decided ASCII-only: an identifier differing
// only by non-ASCII case is already normal form and is NOT case-sensitive.
func TestCaseSensitive_ASCIIOnly(t *testing.T) {
	pg := mustDialect(t, "postgres") // Lowercase: unsafe == has ASCII A-Z
	upper := dialects.Base()
	upper.NormalizationStrategy = dialects.Uppercase // unsafe == has ASCII a-z

	cases := []struct {
		name string
		d    *dialects.Dialect
		text string
		want bool
	}{
		{"pg has ascii upper", pg, "CAFÉ", true},
		{"pg ascii upper only", pg, "Foo", true},
		{"pg non-ascii upper only is safe", pg, "cafÉ", false},
		{"pg all lower", pg, "foo", false},
		{"upper has ascii lower", upper, "café", true},
		{"upper non-ascii lower only is safe", upper, "CAFé", false},
		{"upper all upper", upper, "FOO", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.d.CaseSensitive(c.text); got != c.want {
				t.Fatalf("CaseSensitive(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}
