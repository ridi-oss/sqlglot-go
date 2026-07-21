package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Postgres `U&'...'` string literals and `U&"..."` quoted identifiers are decoded into the
// real code points they denote, so an AST consumer (e.g. a name-based authorization check)
// sees the actual string / identifier the server executes rather than the escaped spelling.
// This is beyond pinned upstream, which mis-tokenizes `U&'...'` as `U & '...'` and
// parse-errors the identifier form. See DEVIATIONS §1.

func genPG(t *testing.T, e exp.Expression) string {
	t.Helper()
	out, err := sqlglot.Generate(e, "postgres", generator.Options{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return out
}

func TestPostgresUnicodeString(t *testing.T) {
	cases := []struct {
		sql     string
		wantVal string
		wantSQL string
	}{
		{`SELECT U&'\0067\0072\0061\0064\0065'`, "grade", "SELECT 'grade'"},
		{`SELECT U&'\0073\0065\0074'`, "set", "SELECT 'set'"},
		{`SELECT u&'\0067rade'`, "grade", "SELECT 'grade'"}, // lowercase u& prefix
		{`SELECT U&'a''b\0063'`, "a'bc", "SELECT 'a''bc'"},  // delimiter doubling + escape
		{`SELECT U&'plain'`, "plain", "SELECT 'plain'"},
		{`SELECT U&'\+01F600'`, "\U0001F600", "SELECT '\U0001F600'"},         // six-hex astral
		{`SELECT U&'\D835\DD0D'`, "\U0001D50D", "SELECT '\U0001D50D'"},       // surrogate pair 4+4
		{`SELECT U&'\D835\+00DD0D'`, "\U0001D50D", "SELECT '\U0001D50D'"},    // surrogate pair 4+6
		{`SELECT U&'\+00D835\+00DD0D'`, "\U0001D50D", "SELECT '\U0001D50D'"}, // surrogate pair 6+6
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindSelect {
				t.Fatalf("root = %v, want Select:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
			lit := e.Expressions()[0]
			if lit.Kind() != exp.KindLiteral || lit.Arg("is_string") != true {
				t.Fatalf("expr = %v, want string Literal:\n%s", exp.ClassName(lit.Kind()), e.ToS())
			}
			if got := lit.Text("this"); got != tc.wantVal {
				t.Fatalf("decoded value = %q, want %q", got, tc.wantVal)
			}
			if got := genPG(t, e); got != tc.wantSQL {
				t.Fatalf("round-trip = %q, want %q", got, tc.wantSQL)
			}
		})
	}
}

func TestPostgresUnicodeIdentifier(t *testing.T) {
	t.Run("schema identifier resolves to the real name", func(t *testing.T) {
		// The headline: a schema spelled with escapes must surface as its decoded name so a
		// name-based check cannot be bypassed.
		e, err := sqlglot.ParseOne(`SELECT count(*) FROM U&"inf\006Frmation_schema".tables`, "postgres")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		table := e.Arg("from_").(exp.Expression).Arg("this").(exp.Expression)
		schema := table.Arg("schema").(exp.Expression)
		if schema.Kind() != exp.KindIdentifier || schema.Arg("quoted") != true {
			t.Fatalf("schema = %v, want quoted Identifier:\n%s", exp.ClassName(schema.Kind()), e.ToS())
		}
		if got := schema.Text("this"); got != "information_schema" {
			t.Fatalf("decoded schema = %q, want information_schema", got)
		}
		if got := genPG(t, e); got != `SELECT COUNT(*) FROM "information_schema".tables` {
			t.Fatalf("round-trip = %q", got)
		}
	})

	t.Run("function name resolves to the real name", func(t *testing.T) {
		e, err := sqlglot.ParseOne(`SELECT U&"set_confi\0067"('search_path', 'x', false)`, "postgres")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		fn := e.Expressions()[0]
		name := fn.Arg("this").(exp.Expression)
		if got := name.Text("this"); got != "set_config" {
			t.Fatalf("decoded function name = %q, want set_config:\n%s", got, e.ToS())
		}
	})

	t.Run("quoted identifier preserves case", func(t *testing.T) {
		e, err := sqlglot.ParseOne(`SELECT U&"Mixed\0043ase"`, "postgres")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		col := e.Expressions()[0].Arg("this").(exp.Expression)
		if got := col.Text("this"); got != "MixedCase" {
			t.Fatalf("decoded identifier = %q, want MixedCase", got)
		}
	})
}

func TestPostgresUnicodeFailClosedAndNoRegression(t *testing.T) {
	// A custom `UESCAPE 'c'` clause is not decoded (the tokenizer assumes the default '\'),
	// so such forms must fail closed (parse error), never silently decode against the wrong
	// escape character.
	t.Run("custom UESCAPE fails closed", func(t *testing.T) {
		if _, err := sqlglot.ParseOne(`SELECT U&'\0441' UESCAPE '!'`, "postgres"); err == nil {
			t.Fatal("expected a parse error for a custom UESCAPE clause, got none")
		}
	})

	// Escapes PostgreSQL itself rejects must fail closed (parse error), never silently decode
	// to a replacement/garbage value a name-based check could be fooled by. Verified against
	// PostgreSQL 17.6 (each errors: "invalid Unicode escape value" / "invalid Unicode surrogate").
	for _, sql := range []string{
		`SELECT U&'\+110000'`,  // beyond U+10FFFF
		`SELECT U&'\0000'`,     // NUL
		`SELECT U&'\D800'`,     // lone high surrogate
		`SELECT U&'\DC00'`,     // lone low surrogate
		`SELECT U&'a\g0z'`,     // malformed escape
		`SELECT U&"info\D800"`, // invalid escape in an identifier
	} {
		t.Run("invalid escape fails closed/"+sql, func(t *testing.T) {
			if _, err := sqlglot.ParseOne(sql, "postgres"); err == nil {
				t.Fatalf("expected a parse error for %q, got none", sql)
			}
		})
	}

	// Spaced `U & 'x'` is genuine bitwise-AND of a column named U — the U& binding requires the
	// prefix to be immediately followed by the quote, matching Postgres.
	t.Run("spaced form stays bitwise", func(t *testing.T) {
		e, err := sqlglot.ParseOne(`SELECT U & 'x'`, "postgres")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if e.Expressions()[0].Kind() != exp.KindBitwiseAnd {
			t.Fatalf("expr = %v, want BitwiseAnd:\n%s", exp.ClassName(e.Expressions()[0].Kind()), e.ToS())
		}
	})

	// Only Postgres wires U&; base/MySQL have no such syntax, so U& stays split (unchanged).
	for _, dialect := range []string{"", "mysql"} {
		t.Run("non-postgres unaffected/"+dialect, func(t *testing.T) {
			e, err := sqlglot.ParseOne(`SELECT U&'\0067'`, dialect)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Expressions()[0].Kind() != exp.KindBitwiseAnd {
				t.Fatalf("dialect %q expr = %v, want BitwiseAnd (no U& support):\n%s", dialect, exp.ClassName(e.Expressions()[0].Kind()), e.ToS())
			}
		})
	}
}
