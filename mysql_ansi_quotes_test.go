package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// MySQL `sql_mode=ANSI_QUOTES` — an opt-in dialect setting (`mysql_ansi_quotes=true`) that makes `"`
// a quoted-IDENTIFIER delimiter instead of a string literal, exactly as the real server does. This
// is security-relevant for a name-based analyzer: under ANSI_QUOTES `SELECT "card_number"` reads the
// COLUMN card_number, so an analyzer parsing in default mode (where `"…"` is a string) would miss the
// column entirely (a masking bypass / data leak). Verified against MySQL 8.0.33. See DEVIATIONS.
func TestMySQLAnsiQuotes(t *testing.T) {
	const ansi = "mysql, mysql_ansi_quotes=true"

	// Default MySQL: `"x"` is a STRING literal.
	def, err := sqlglot.ParseOne(`SELECT "card_number" FROM t`, "mysql")
	if err != nil {
		t.Fatalf("default parse: %v", err)
	}
	if got := def.Expressions()[0]; got.Kind() != exp.KindLiteral {
		t.Errorf(`default mysql: "card_number" should be a Literal (string), got %s`, exp.ClassName(got.Kind()))
	}

	// ANSI_QUOTES: `"x"` is a quoted IDENTIFIER — parses to a Column, and round-trips to a backtick
	// identifier (valid under ANSI_QUOTES, and universally valid across sql_modes).
	aq, err := sqlglot.ParseOne(`SELECT "card_number" FROM t`, ansi)
	if err != nil {
		t.Fatalf("ansi parse: %v", err)
	}
	proj := aq.Expressions()[0]
	if proj.Kind() != exp.KindColumn {
		t.Fatalf(`ANSI_QUOTES: "card_number" should be a Column (identifier), got %s\n%s`, exp.ClassName(proj.Kind()), aq.ToS())
	}
	if proj.Name() != "card_number" {
		t.Errorf("ANSI_QUOTES: column name = %q, want card_number", proj.Name())
	}
	if out, _ := sqlglot.Generate(aq, ansi, generator.Options{}); out != "SELECT `card_number` FROM t" {
		t.Errorf("ANSI_QUOTES round-trip = %q, want SELECT `card_number` FROM t", out)
	}

	// Under ANSI_QUOTES, a single-quoted string is still a string, a backtick is still an identifier,
	// and the two compose (`"col" = 'val'` → Column = Literal).
	pred := mustParse(t, `SELECT * FROM t WHERE "col" = 'val'`, ansi)
	where, _ := pred.Arg("where").(exp.Expression)
	if where == nil {
		t.Fatalf("no WHERE\n%s", pred.ToS())
	}
	eq := where.This()
	if eq == nil || eq.Kind() != exp.KindEQ {
		t.Fatalf("WHERE is not an EQ\n%s", pred.ToS())
	}
	if lhs := eq.This(); lhs == nil || lhs.Kind() != exp.KindColumn {
		t.Errorf(`ANSI_QUOTES: "col" LHS should be a Column, got %v`, eq.This())
	}
	if rhs, _ := eq.Arg("expression").(exp.Expression); rhs == nil || rhs.Kind() != exp.KindLiteral {
		t.Errorf("ANSI_QUOTES: 'val' RHS should be a Literal (string), got %v", eq.Arg("expression"))
	}

	// `mysql_ansi_quotes=false` is the default (off) behavior.
	off := mustParse(t, `SELECT "x"`, "mysql, mysql_ansi_quotes=false")
	if got := off.Expressions()[0]; got.Kind() != exp.KindLiteral {
		t.Errorf(`mysql_ansi_quotes=false: "x" should stay a Literal, got %s`, exp.ClassName(got.Kind()))
	}

	// A non-boolean value is rejected.
	if _, err := sqlglot.ParseOne("SELECT 1", "mysql, mysql_ansi_quotes=maybe"); err == nil {
		t.Errorf("mysql_ansi_quotes=maybe should error (non-boolean)")
	}
}

// Regression (dual review): moving `"` out of the string Quotes must also drop it from StringEscapes.
// MySQL adds `"` to StringEscapes for `""`-doubling inside a `"`-string (default mode); under
// ANSI_QUOTES there are no `"`-strings, so a stale escape entry makes a `"` before the closing `'`
// wrongly consume the terminator, failing to tokenize a perfectly valid single-quoted string. MySQL
// 8.0.33 accepts all of these under ANSI_QUOTES (`"` is just a literal char inside a `'`-string).
func TestMySQLAnsiQuotesStringEscape(t *testing.T) {
	const ansi = "mysql, mysql_ansi_quotes=true"
	for _, sql := range []string{
		`SELECT '\n"'`,  // backslash-n then a `"` before the closing quote (the codex/Sol repro)
		`SELECT 'a"b'`,  // a `"` mid-string
		`SELECT 'a''"'`, // doubled apostrophe forces the slow path, then a trailing `"`
		`SELECT '"'`,    // a lone `"`
	} {
		e := mustParse(t, sql, ansi)
		if got := e.Expressions()[0]; got.Kind() != exp.KindLiteral {
			t.Errorf("%q [ANSI_QUOTES]: want a string Literal, got %s\n%s", sql, exp.ClassName(got.Kind()), e.ToS())
		}
	}
}

// Regression (dual review): the setting must be last-value-wins. Because a `true` value activates the
// tokenizer change, a later `mysql_ansi_quotes=false` must undo it — the change is applied once, from
// the FINAL flag value, not eagerly on the first `true`. Mirrors real MySQL restoring string semantics
// when ANSI_QUOTES is removed from sql_mode.
func TestMySQLAnsiQuotesLastValueWins(t *testing.T) {
	// true then false -> off -> `"x"` is a string.
	off := mustParse(t, `SELECT "x"`, "mysql, mysql_ansi_quotes=true, mysql_ansi_quotes=false")
	if got := off.Expressions()[0]; got.Kind() != exp.KindLiteral {
		t.Errorf(`true,false: "x" should be a Literal (last value wins), got %s`, exp.ClassName(got.Kind()))
	}
	// false then true -> on -> `"x"` is a column.
	on := mustParse(t, `SELECT "x"`, "mysql, mysql_ansi_quotes=false, mysql_ansi_quotes=true")
	if got := on.Expressions()[0]; got.Kind() != exp.KindColumn {
		t.Errorf(`false,true: "x" should be a Column (last value wins), got %s`, exp.ClassName(got.Kind()))
	}
}

func mustParse(t *testing.T, sql string, dialect string) exp.Expression {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("%q [%s]: parse: %v", sql, dialect, err)
	}
	return e
}
