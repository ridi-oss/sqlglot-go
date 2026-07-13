package optimizer_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
	"github.com/ridi/sqlglot-go/schema"
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
