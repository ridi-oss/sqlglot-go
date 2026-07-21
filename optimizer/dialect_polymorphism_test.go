package optimizer_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
	"github.com/ridi-oss/sqlglot-go/schema"
)

// TestDialectTypePolymorphism verifies that NormalizeIdentifiers and Qualify accept a
// DialectType-style value (nil | string | *dialects.Dialect) — mirroring upstream sqlglot's
// polymorphic dialect argument — and that a typed *Dialect carrying a normalization strategy
// produces exactly the same result as the equivalent "name, normalization_strategy=..."
// settings string. This is the N1 win: proxy can build a *Dialect once instead of hand-
// formatting a magic settings string.
func TestDialectTypePolymorphism(t *testing.T) {
	normalize := func(t *testing.T, sql string, dialect any) string {
		t.Helper()
		e, err := sqlglot.ParseOne(sql, "mysql")
		if err != nil {
			t.Fatalf("ParseOne(%q): %v", sql, err)
		}
		normalized := optimizer.NormalizeIdentifiers(e, dialect)
		out, err := sqlglot.Generate(normalized, "mysql", generator.Options{})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return out
	}

	const sql = "SELECT Col_A FROM Tbl"

	// Case-insensitive MySQL strategy: string form vs typed *Dialect must agree, and both must
	// fold the unquoted identifiers to lowercase.
	ciDialect := dialects.MySQL()
	ciDialect.NormalizationStrategy = dialects.MySQLCaseInsensitive
	fromString := normalize(t, sql, "mysql, normalization_strategy=mysql_case_insensitive")
	fromDialect := normalize(t, sql, ciDialect)
	if fromString != fromDialect {
		t.Fatalf("string vs *Dialect disagree:\n  string  %q\n  dialect %q", fromString, fromDialect)
	}
	if fromDialect != "SELECT col_a FROM tbl" {
		t.Fatalf("case-insensitive fold = %q, want %q", fromDialect, "SELECT col_a FROM tbl")
	}

	// Default MySQL (CASE_SENSITIVE): string "mysql", typed *Dialect, and nil-vs-base all leave
	// the identifiers unfolded, and string form == typed form.
	defString := normalize(t, sql, "mysql")
	defDialect := normalize(t, sql, dialects.MySQL())
	if defString != defDialect {
		t.Fatalf("default string vs *Dialect disagree:\n  string  %q\n  dialect %q", defString, defDialect)
	}
	if defDialect != "SELECT Col_A FROM Tbl" {
		t.Fatalf("case-sensitive (default) = %q, want unchanged", defDialect)
	}

	// SettingsString round-trips through GetOrRaise.
	if got := ciDialect.SettingsString(); got != "mysql, normalization_strategy=mysql_case_insensitive" {
		t.Fatalf("SettingsString() = %q", got)
	}
	rt, err := dialects.GetOrRaise(ciDialect.SettingsString())
	if err != nil {
		t.Fatalf("GetOrRaise(SettingsString): %v", err)
	}
	if rt.NormalizationStrategy != dialects.MySQLCaseInsensitive {
		t.Fatalf("round-trip strategy = %v, want MySQLCaseInsensitive", rt.NormalizationStrategy)
	}

	// Instance preservation: EnsureSchema must hand back the SAME *Dialect instance (not a
	// fresh re-resolution), so the passes that read dialect fields via Schema.Dialect() —
	// e.g. QualifyColumns' ForceEarlyAliasRefExpansion / TablesReferenceableAsColumns — see
	// the caller's instance and all its (possibly non-default) fields. This is the fix for a
	// review finding: the earlier settings-string round-trip discarded non-strategy state.
	s, err := schema.EnsureSchema(schema.NewMapping(), ciDialect, true)
	if err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	if s.Dialect() != ciDialect {
		t.Fatalf("EnsureSchema re-resolved the dialect (%p) instead of preserving the passed instance (%p)", s.Dialect(), ciDialect)
	}

	// Qualify-level polymorphism: a *Dialect and the equivalent settings string produce
	// identical output end-to-end (the "proxy builds a *Dialect once" path, previously only
	// covered at the NormalizeIdentifiers layer).
	qualifyOut := func(t *testing.T, dialect any) string {
		t.Helper()
		e, err := sqlglot.ParseOne("SELECT A FROM x", "mysql")
		if err != nil {
			t.Fatalf("ParseOne: %v", err)
		}
		opts := optimizer.DefaultQualifyOpts()
		opts.Schema = optimizerTestSchema()
		opts.Dialect = dialect
		out, err := sqlglot.Generate(optimizer.Qualify(e, opts), "mysql", generator.Options{})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return out
	}
	ciDialect2 := dialects.MySQL()
	ciDialect2.NormalizationStrategy = dialects.MySQLCaseInsensitive
	qFromDialect := qualifyOut(t, ciDialect2)
	qFromString := qualifyOut(t, "mysql, normalization_strategy=mysql_case_insensitive")
	if qFromDialect != qFromString {
		t.Fatalf("Qualify string vs *Dialect disagree:\n  string  %q\n  dialect %q", qFromString, qFromDialect)
	}
}

// TestDialectTypePolymorphismPasses covers the remaining passes whose dialect argument was
// widened to DialectType — QualifyColumns, QuoteIdentifiers, QualifyTables, IsolateTableSelects
// and NormalizeIdentifiersString. Each is exercised with the "mysql" string and the equivalent
// *dialects.Dialect and must produce identical output, demonstrating the string -> *Dialect
// polymorphism end to end.
func TestDialectTypePolymorphismPasses(t *testing.T) {
	mysqlStr := "mysql"
	mysqlPtr := dialects.MySQL()

	// A fresh parse per run: these passes mutate the expression in place.
	parse := func(t *testing.T) exp.Expression {
		t.Helper()
		e, err := sqlglot.ParseOne("SELECT a FROM t", "mysql")
		if err != nil {
			t.Fatalf("ParseOne: %v", err)
		}
		return e
	}
	gen := func(t *testing.T, e exp.Expression) string {
		t.Helper()
		out, err := sqlglot.Generate(e, "mysql", generator.Options{})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return out
	}
	sch := func(t *testing.T) schema.Schema {
		t.Helper()
		s, err := schema.EnsureSchema(schema.M("t", schema.M("a", "INT")), "mysql", true)
		if err != nil {
			t.Fatalf("EnsureSchema: %v", err)
		}
		return s
	}

	// parity runs fn(dialect) on a fresh parse for the string and *Dialect forms and compares.
	parity := func(t *testing.T, name string, fn func(dialect dialects.DialectType) exp.Expression) {
		t.Helper()
		fromStr := gen(t, fn(mysqlStr))
		fromPtr := gen(t, fn(mysqlPtr))
		if fromStr != fromPtr {
			t.Fatalf("%s string vs *Dialect disagree:\n  string  %q\n  dialect %q", name, fromStr, fromPtr)
		}
	}

	parity(t, "QualifyColumns", func(d dialects.DialectType) exp.Expression {
		return optimizer.QualifyColumns(parse(t), sch(t), true, true, nil, false, d)
	})
	parity(t, "QuoteIdentifiers", func(d dialects.DialectType) exp.Expression {
		return optimizer.QuoteIdentifiers(parse(t), d, true)
	})
	parity(t, "QualifyTables", func(d dialects.DialectType) exp.Expression {
		return optimizer.QualifyTables(parse(t), nil, nil, d, false, nil, nil, nil)
	})
	parity(t, "IsolateTableSelects", func(d dialects.DialectType) exp.Expression {
		return optimizer.IsolateTableSelects(parse(t), nil, d)
	})

	// NormalizeIdentifiersString: a settings-carrying *Dialect (case-insensitive MySQL) must fold
	// identically to its equivalent settings string, proving the strategy flows through the typed
	// value — not just that a *Dialect is accepted.
	ci := dialects.MySQL()
	ci.NormalizationStrategy = dialects.MySQLCaseInsensitive
	fromStr := gen(t, optimizer.NormalizeIdentifiersString("Col_A", "mysql, normalization_strategy=mysql_case_insensitive"))
	fromPtr := gen(t, optimizer.NormalizeIdentifiersString("Col_A", ci))
	if fromStr != fromPtr {
		t.Fatalf("NormalizeIdentifiersString string vs *Dialect disagree:\n  string  %q\n  dialect %q", fromStr, fromPtr)
	}
}
