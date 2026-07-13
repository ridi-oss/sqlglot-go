package tokens

import (
	stderrors "errors"
	"strings"
	"testing"

	sqlerrors "github.com/ridi/sqlglot-go/errors"
)

func mustTokenize(t *testing.T, tokenizer *Tokenizer, sql string) []Token {
	t.Helper()
	tokens, err := tokenizer.Tokenize(sql)
	if err != nil {
		t.Fatalf("Tokenize(%q) error: %v", sql, err)
	}
	return tokens
}

func TestSpaceKeywords(t *testing.T) {
	for _, tc := range []struct {
		sql    string
		length int
	}{
		{"group bys", 2},
		{" group bys", 2},
		{" group bys ", 2},
		{"group by)", 2},
		{"group bys)", 3},
		{"group \r", 1},
	} {
		tokens := mustTokenize(t, NewTokenizer(), tc.sql)
		if !strings.Contains(strings.ToUpper(tokens[0].Text), "GROUP") {
			t.Fatalf("Tokenize(%q)[0].Text = %q", tc.sql, tokens[0].Text)
		}
		if len(tokens) != tc.length {
			t.Fatalf("Tokenize(%q) length = %d, want %d", tc.sql, len(tokens), tc.length)
		}
	}
}

func TestCommentAttachment(t *testing.T) {
	tokenizer := NewTokenizer()
	cases := []struct {
		sql      string
		comments []string
	}{
		{"/*comment*/ foo", []string{"comment"}},
		{"/*comment*/ foo --test", []string{"comment", "test"}},
		{"--comment\nfoo --test", []string{"comment", "test"}},
		{"foo --comment", []string{"comment"}},
		{"foo", []string{}},
		{"foo /*comment 1*/ /*comment 2*/", []string{"comment 1", "comment 2"}},
		{"foo\n-- comment", []string{" comment"}},
		{"1 /*/2 */", []string{"/2 "}},
		{"1\n/*comment*/;", []string{"comment"}},
	}
	for _, tc := range cases {
		got := mustTokenize(t, tokenizer, tc.sql)[0].Comments
		if !sameStrings(got, tc.comments) {
			t.Fatalf("Tokenize(%q)[0].Comments = %#v, want %#v", tc.sql, got, tc.comments)
		}
	}
}

func TestTokenLineCol(t *testing.T) {
	tokens := mustTokenize(t, NewTokenizer(), "SELECT /*\nline break\n*/\n'x\n y',\nx")

	assertPos(t, tokens[0], 1, 6)
	assertPos(t, tokens[1], 5, 3)
	assertPos(t, tokens[2], 5, 4)
	assertPos(t, tokens[3], 6, 1)

	tokens = mustTokenize(t, NewTokenizer(), "SELECT .")
	assertPos(t, tokens[1], 1, 8)

	if got := mustTokenize(t, NewTokenizer(), "'''abc'")[0].Start; got != 0 {
		t.Fatalf("start = %d, want 0", got)
	}
	if got := mustTokenize(t, NewTokenizer(), "'''abc'")[0].End; got != 6 {
		t.Fatalf("end = %d, want 6", got)
	}
	if got := mustTokenize(t, NewTokenizer(), "'abc'")[0].Start; got != 0 {
		t.Fatalf("start = %d, want 0", got)
	}

	tokens = mustTokenize(t, NewTokenizer(), "SELECT\r\n  1,\r\n  2")
	assertPos(t, tokens[0], 1, 6)
	assertPos(t, tokens[1], 2, 3)
	assertPos(t, tokens[2], 2, 4)
	assertPos(t, tokens[3], 3, 3)

	tokens = mustTokenize(t, NewTokenizer(), "  SELECT\n    100")
	assertPos(t, tokens[0], 1, 8)
	assertPos(t, tokens[1], 2, 7)
}

func TestCRLF(t *testing.T) {
	tokens := mustTokenize(t, NewTokenizer(), "SELECT a\r\nFROM b")
	assertTokenPairs(t, tokens, []tokenPair{{SELECT, "SELECT"}, {VAR, "a"}, {FROM, "FROM"}, {VAR, "b"}})

	for _, simpleQuery := range []string{"SELECT 1\r\n", "\r\nSELECT 1"} {
		tokens = mustTokenize(t, NewTokenizer(), simpleQuery)
		assertTokenPairs(t, tokens, []tokenPair{{SELECT, "SELECT"}, {NUMBER, "1"}})
	}
}

func TestCommand(t *testing.T) {
	tokens := mustTokenize(t, NewTokenizer(), "SHOW;")
	if tokens[0].TokenType != SHOW || tokens[1].TokenType != SEMICOLON {
		t.Fatalf("SHOW; tokens = %s", ReprTokens(tokens))
	}

	tokens = mustTokenize(t, NewTokenizer(), "EXECUTE")
	if tokens[0].TokenType != EXECUTE || len(tokens) != 1 {
		t.Fatalf("EXECUTE tokens = %s", ReprTokens(tokens))
	}

	tokens = mustTokenize(t, NewTokenizer(), "FETCH;SHOW;")
	want := []TokenType{FETCH, SEMICOLON, SHOW, SEMICOLON}
	for i, tokenType := range want {
		if tokens[i].TokenType != tokenType {
			t.Fatalf("token %d = %s, want %s", i, tokens[i].TokenType, tokenType)
		}
	}
}

func TestErrorMsg(t *testing.T) {
	_, err := NewTokenizer().Tokenize("select /*")
	if err == nil {
		t.Fatal("expected TokenError")
	}
	var tokenErr *sqlerrors.TokenError
	if !stderrors.As(err, &tokenErr) {
		t.Fatalf("error = %T, want *TokenError", err)
	}
	if !strings.Contains(err.Error(), "Error tokenizing 'select /'") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestJinja(t *testing.T) {
	t.Skip("TODO: BigQuery dialect out of M1 scope per ROADMAP.md")
}

func TestPartialTokenList(t *testing.T) {
	tokenizer := NewTokenizer()
	_, err := tokenizer.Tokenize("foo 'bar")
	if err == nil {
		t.Fatal("expected TokenError")
	}
	if !strings.Contains(err.Error(), "Error tokenizing 'foo 'ba'") {
		t.Fatalf("error = %q", err.Error())
	}

	partialTokens := tokenizer.Tokens()
	if len(partialTokens) != 1 {
		t.Fatalf("partial token count = %d, want 1", len(partialTokens))
	}
	if partialTokens[0].TokenType != VAR || partialTokens[0].Text != "foo" {
		t.Fatalf("partial token = %s", partialTokens[0].String())
	}
}

func TestUnicodeIdentifiers(t *testing.T) {
	tokens := mustTokenize(t, NewTokenizer(), "SELECT café FROM t")
	for _, token := range tokens {
		if token.TokenType == VAR {
			if token.Text != "café" {
				t.Fatalf("first VAR text = %q, want café", token.Text)
			}
			return
		}
	}
	t.Fatal("no VAR token found")
}

func TestTokenRepr(t *testing.T) {
	got := ReprTokens(mustTokenize(t, NewTokenizer(), "foo"))
	want := "[<Token token_type: TokenType.VAR, text: foo, line: 1, col: 3, start: 0, end: 2, comments: []>]"
	if got != want {
		t.Fatalf("repr = %q, want %q", got, want)
	}
}

type tokenPair struct {
	tokenType TokenType
	text      string
}

func assertTokenPairs(t *testing.T, got []Token, want []tokenPair) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d: %s", len(got), len(want), ReprTokens(got))
	}
	for i, expected := range want {
		if got[i].TokenType != expected.tokenType || got[i].Text != expected.text {
			t.Fatalf("token %d = (%s, %q), want (%s, %q)", i, got[i].TokenType, got[i].Text, expected.tokenType, expected.text)
		}
	}
}

func assertPos(t *testing.T, token Token, line, col int) {
	t.Helper()
	if token.Line != line || token.Col != col {
		t.Fatalf("%s position = (%d, %d), want (%d, %d)", token.String(), token.Line, token.Col, line, col)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
