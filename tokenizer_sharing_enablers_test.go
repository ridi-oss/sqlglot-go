package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
)

// TestTokenizeErrorsOnUnterminated (V1): Tokenize must return an error — never a silently
// truncated stream — on an unterminated construct, so a consumer can fail closed.
func TestTokenizeErrorsOnUnterminated(t *testing.T) {
	cases := []struct{ dialect, sql string }{
		{"mysql", "SELECT 'unterminated"},
		{"mysql", "SELECT /* unterminated"},
		{"mysql", "SELECT `unterminated"},
		{"postgres", `SELECT "unterminated`},
		{"postgres", "SELECT $tag$unterminated"},
	}
	for _, c := range cases {
		if _, err := sqlglot.Tokenize(c.sql, c.dialect); err == nil {
			t.Errorf("Tokenize(%q, %q): expected an error on unterminated input, got nil", c.sql, c.dialect)
		}
	}
}

// TestTokenByteExactSpan (V2): Start/End are inclusive rune offsets into the original source, and
// slicing the source by them recovers the verbatim lexeme for every token kind — including string
// literals, whose Text is the decoded value (quotes/escapes resolved), not the raw source.
func TestTokenByteExactSpan(t *testing.T) {
	src := `SELECT 'a''b', "Id", 42, x FROM t`
	runes := []rune(src)
	toks, err := sqlglot.Tokenize(src, "postgres")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	for _, tk := range toks {
		if tk.Start < 0 || tk.End >= len(runes) || tk.Start > tk.End {
			t.Fatalf("token %q has out-of-range span [%d,%d] (len=%d)", tk.Text, tk.Start, tk.End, len(runes))
		}
	}
	// The string literal 'a''b': Text is decoded ("a'b"), but the source span is the raw "'a''b'".
	var foundString bool
	for _, tk := range toks {
		if tk.Text == "a'b" {
			foundString = true
			if got := string(runes[tk.Start : tk.End+1]); got != "'a''b'" {
				t.Fatalf("string literal source span = %q, want %q", got, "'a''b'")
			}
		}
	}
	if !foundString {
		t.Fatal("did not find the decoded string token a'b")
	}
}

// TestIsReservedKeyword (A2): the case-safe reserved-keyword accessor. Reserved words fold; a
// non-reserved keyword that can be a case-sensitive identifier (e.g. COMMENT) does not.
func TestIsReservedKeyword(t *testing.T) {
	my := dialects.MySQL()
	// Reserved in MySQL 8.0 (INDEX is reserved) — case-insensitive.
	for _, w := range []string{"SELECT", "select", "Match", "GROUP", "LATERAL", "INDEX", "index"} {
		if !my.IsReservedKeyword(w) {
			t.Errorf("MySQL.IsReservedKeyword(%q) = false, want true (case-insensitive)", w)
		}
	}
	// Non-reserved keywords — these can be case-sensitive identifiers, so must NOT fold.
	for _, w := range []string{"COMMENT", "format", "STATUS"} {
		if my.IsReservedKeyword(w) {
			t.Errorf("MySQL.IsReservedKeyword(%q) = true, want false (non-reserved, can be an identifier)", w)
		}
	}
	// Dialects without a reserved set return false.
	if dialects.Postgres().IsReservedKeyword("SELECT") {
		t.Error("Postgres.IsReservedKeyword should be false (no reserved set populated)")
	}
}
