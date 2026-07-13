package parser_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestParseCommandFallback ports test_parser.py:186 test_command: a batch of statements
// where some fall back to a raw exp.Command and others parse structurally, verifying the
// two coexist within a single Parse call. Upstream's own case ("SET x = 1; ADD JAR
// s3://a; SELECT 1", read="hive") isn't reproducible here as-is: SET's structured parser
// and hive itself belong to a sibling statement-family part / are out of this port's
// dialect scope (AGENTS.md: base + MySQL + Postgres only), and fnd must not reference any
// family symbol. So this substitutes two genuine base-dialect Commands-set leaders (CALL,
// EXPLAIN - both TokenType.COMMAND, tokens.go KEYWORDS) for SET/ADD, preserving the
// upstream test's actual point: Command-fallback statements interleave freely with
// structurally-parsed ones in the same batch. The _warn_unsupported log assertion is
// dropped, per the guide (this port has no logger).
func TestParseCommandFallback(t *testing.T) {
	expressions, err := sqlglot.Parse("CALL foo(1, 2); EXPLAIN SELECT 1; SELECT 2", "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(expressions) != 3 {
		t.Fatalf("len(expressions) = %d, want 3", len(expressions))
	}

	if expressions[0].Kind() != exp.KindCommand {
		t.Fatalf("expressions[0].Kind() = %v, want Command:\n%s", expressions[0].Kind(), expressions[0].ToS())
	}
	if this := expressions[0].Arg("this"); this != "CALL" {
		t.Fatalf("expressions[0].this = %#v, want \"CALL\"", this)
	}
	if got := generateOrFatal(t, expressions[0]); got != "CALL foo(1, 2)" {
		t.Fatalf("expressions[0].sql() = %q, want %q", got, "CALL foo(1, 2)")
	}

	if expressions[1].Kind() != exp.KindCommand {
		t.Fatalf("expressions[1].Kind() = %v, want Command:\n%s", expressions[1].Kind(), expressions[1].ToS())
	}
	if this := expressions[1].Arg("this"); this != "EXPLAIN" {
		t.Fatalf("expressions[1].this = %#v, want \"EXPLAIN\"", this)
	}
	if got := generateOrFatal(t, expressions[1]); got != "EXPLAIN SELECT 1" {
		t.Fatalf("expressions[1].sql() = %q, want %q", got, "EXPLAIN SELECT 1")
	}

	if expressions[2].Kind() != exp.KindSelect {
		t.Fatalf("expressions[2].Kind() = %v, want Select:\n%s", expressions[2].Kind(), expressions[2].ToS())
	}
	if got := generateOrFatal(t, expressions[2]); got != "SELECT 2" {
		t.Fatalf("expressions[2].sql() = %q, want %q", got, "SELECT 2")
	}
}

// generateOrFatal is a single-expression sql() helper local to this file (the shared
// generateSQL/parseOneDialect helpers in parser_helpers_test.go only cover the ParseOne
// path; this test needs sqlglot.Parse's multi-statement batch).
func generateOrFatal(t *testing.T, expression exp.Expression) string {
	t.Helper()
	got, err := generateSQL(t, expression, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return got
}

// TestParseCommandMySQLLockTables asserts mysql's `LOCK TABLES ...`/`UNLOCK TABLES ...`
// round-trip via the generic Commands-branch + parseCommand: both keywords are mapped to
// TokenType.COMMAND by mysql's multi-word KEYWORDS override (dialects/mysql.go), so the
// tokenizer packs the remainder into one STRING token (tokens/tokenizer_core.go:256-266)
// and parseCommand's `this` captures the full two-word leader.
func TestParseCommandMySQLLockTables(t *testing.T) {
	for _, sql := range []string{
		"LOCK TABLES t READ",
		"UNLOCK TABLES",
	} {
		expression := parseOneDialect(t, sql, "mysql")
		if expression.Kind() != exp.KindCommand {
			t.Fatalf("%q: Kind() = %v, want Command:\n%s", sql, expression.Kind(), expression.ToS())
		}
		got, err := generateSQL(t, expression, "mysql")
		if err != nil {
			t.Fatalf("%q: Generate: %v", sql, err)
		}
		if got != sql {
			t.Fatalf("%q: round-trip = %q", sql, got)
		}
	}
}

// TestParsePragma asserts the base dialect's `PRAGMA <expr>` round-trips via the
// statementParsers[tokens.PRAGMA] -> parsePragma -> exp.Pragma -> pragmaSQL path.
func TestParsePragma(t *testing.T) {
	sql := "PRAGMA quick_check"
	expression := parseOneDialect(t, sql, "")
	if expression.Kind() != exp.KindPragma {
		t.Fatalf("Kind() = %v, want Pragma:\n%s", expression.Kind(), expression.ToS())
	}
	got, err := generateSQL(t, expression, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}
