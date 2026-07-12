package tokens

import (
	"reflect"
	"testing"
)

func TestMySQLExecutableCommentsActivateByVersion(t *testing.T) {
	version := 80033
	cfg := BaseConfig()
	cfg.MySQLExecutableComments = true
	cfg.MySQLVersion = &version

	for _, tc := range []struct {
		name string
		sql  string
		want []tokenPair
	}{
		{
			name: "bare",
			sql:  "/*! SELECT 1 */",
			want: []tokenPair{{SELECT, "SELECT"}, {NUMBER, "1"}},
		},
		{
			name: "lower gate",
			sql:  "/*!50000 SELECT 1 */",
			want: []tokenPair{{SELECT, "SELECT"}, {NUMBER, "1"}},
		},
		{
			name: "equal gate",
			sql:  "/*!80033 SELECT 1 */",
			want: []tokenPair{{SELECT, "SELECT"}, {NUMBER, "1"}},
		},
		{
			name: "fewer than five digits are body",
			sql:  "/*!123 SELECT 1 */",
			want: []tokenPair{{NUMBER, "123"}, {SELECT, "SELECT"}, {NUMBER, "1"}},
		},
		{
			name: "sixth digit is body",
			sql:  "/*!800330 SELECT 1 */",
			want: []tokenPair{{NUMBER, "0"}, {SELECT, "SELECT"}, {NUMBER, "1"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tokens, err := NewTokenizerCore(cfg).Tokenize(tc.sql)
			if err != nil {
				t.Fatalf("Tokenize(%q): %v", tc.sql, err)
			}
			assertTokenPairs(t, tokens, tc.want)
			assertNoExecutableWrapperTokens(t, tokens)
		})
	}
}

func TestMySQLExecutableCommentsLeaveNewerGatesInactive(t *testing.T) {
	version := 80033
	cfg := BaseConfig()
	cfg.MySQLExecutableComments = true
	cfg.MySQLVersion = &version

	for _, gate := range []string{"80034", "99999"} {
		sql := "/*!" + gate + " SELECT 1 */ SELECT 2"
		tokens, err := NewTokenizerCore(cfg).Tokenize(sql)
		if err != nil {
			t.Fatalf("Tokenize(%q): %v", sql, err)
		}
		assertTokenPairs(t, tokens, []tokenPair{{SELECT, "SELECT"}, {NUMBER, "2"}})
		wantComment := "!" + gate + " SELECT 1 "
		if !reflect.DeepEqual(tokens[0].Comments, []string{wantComment}) {
			t.Fatalf("Tokenize(%q) comments = %#v, want %#v", sql, tokens[0].Comments, []string{wantComment})
		}
	}
}

func TestMySQLExecutableCommentsRequireVersion(t *testing.T) {
	cfg := BaseConfig()
	cfg.MySQLExecutableComments = true
	cfg.MySQLVersion = nil

	sql := "/*! SELECT 1 */ SELECT 2"
	tokens, err := NewTokenizerCore(cfg).Tokenize(sql)
	if err != nil {
		t.Fatalf("Tokenize(%q): %v", sql, err)
	}
	assertTokenPairs(t, tokens, []tokenPair{{SELECT, "SELECT"}, {NUMBER, "2"}})
	if !reflect.DeepEqual(tokens[0].Comments, []string{"! SELECT 1 "}) {
		t.Fatalf("comments = %#v, want executable wrapper preserved as an ordinary comment", tokens[0].Comments)
	}
}

func TestMySQLExecutableCommentPositionsUseOriginalSource(t *testing.T) {
	version := 80033
	cfg := BaseConfig()
	cfg.MySQLExecutableComments = true
	cfg.MySQLVersion = &version

	sql := "π /*!50000SELECT café,\n  β\n*/ FROM t"
	tokens, err := NewTokenizerCore(cfg).Tokenize(sql)
	if err != nil {
		t.Fatalf("Tokenize(%q): %v", sql, err)
	}
	assertTokenPairs(t, tokens, []tokenPair{
		{VAR, "π"},
		{SELECT, "SELECT"},
		{VAR, "café"},
		{COMMA, ","},
		{VAR, "β"},
		{FROM, "FROM"},
		{VAR, "t"},
	})

	runes := []rune(sql)
	for i, want := range []struct {
		text       string
		start, end int
		line, col  int
	}{
		{text: "SELECT", start: 10, end: 15, line: 1, col: 16},
		{text: "café", start: 17, end: 20, line: 1, col: 21},
		{text: ",", start: 21, end: 21, line: 1, col: 22},
		{text: "β", start: 25, end: 25, line: 2, col: 3},
	} {
		token := tokens[i+1]
		if got := string(runes[token.Start : token.End+1]); got != want.text {
			t.Errorf("token %d source lexeme = %q, want %q (%s)", i+1, got, want.text, token.String())
		}
		if token.Start != want.start || token.End != want.end || token.Line != want.line || token.Col != want.col {
			t.Errorf("token %d position = start %d end %d line %d col %d, want %d %d %d %d", i+1, token.Start, token.End, token.Line, token.Col, want.start, want.end, want.line, want.col)
		}
	}
}

func TestMySQLExecutableCommentsPreservePendingComments(t *testing.T) {
	version := 80033
	cfg := BaseConfig()
	cfg.MySQLExecutableComments = true
	cfg.MySQLVersion = &version

	tokens, err := NewTokenizerCore(cfg).Tokenize("/* outer */ /*!50000 SELECT 1 */")
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	assertTokenPairs(t, tokens, []tokenPair{{SELECT, "SELECT"}, {NUMBER, "1"}})
	if !reflect.DeepEqual(tokens[0].Comments, []string{" outer "}) {
		t.Fatalf("first spliced token comments = %#v, want pending outer comment", tokens[0].Comments)
	}

	tokens, err = NewTokenizerCore(cfg).Tokenize("/* outer */ /*!50000*/ SELECT 1")
	if err != nil {
		t.Fatalf("Tokenize empty body: %v", err)
	}
	assertTokenPairs(t, tokens, []tokenPair{{SELECT, "SELECT"}, {NUMBER, "1"}})
	if !reflect.DeepEqual(tokens[0].Comments, []string{" outer "}) {
		t.Fatalf("token after empty body comments = %#v, want pending outer comment", tokens[0].Comments)
	}
}

func TestMySQLExecutableCommentsDoNotChangeOrdinaryCommentsOrHints(t *testing.T) {
	version := 80033
	cfg := BaseConfig()
	cfg.MySQLExecutableComments = true
	cfg.MySQLVersion = &version

	for _, sql := range []string{
		"SELECT /* ordinary */ 1",
		"SELECT /*+ INDEX(t) */ * FROM t",
	} {
		got, err := NewTokenizerCore(cfg).Tokenize(sql)
		if err != nil {
			t.Fatalf("active Tokenize(%q): %v", sql, err)
		}
		want, err := NewTokenizerCore(BaseConfig()).Tokenize(sql)
		if err != nil {
			t.Fatalf("baseline Tokenize(%q): %v", sql, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Tokenize(%q) = %s, want unchanged %s", sql, ReprTokens(got), ReprTokens(want))
		}
	}
}

func TestMySQLExecutableCommentErrorsAreSurfaced(t *testing.T) {
	version := 80033
	cfg := BaseConfig()
	cfg.MySQLExecutableComments = true
	cfg.MySQLVersion = &version

	for _, sql := range []string{
		"/*!50000 SELECT",
		"/*!50000 'unterminated */",
	} {
		if _, err := NewTokenizerCore(cfg).Tokenize(sql); err == nil {
			t.Fatalf("Tokenize(%q) succeeded, want tokenizer error", sql)
		}
	}
}

func assertNoExecutableWrapperTokens(t *testing.T, tokenList []Token) {
	t.Helper()
	for _, token := range tokenList {
		switch token.TokenType {
		case NOT, STAR, SLASH:
			t.Fatalf("executable wrapper leaked into token stream: %s", ReprTokens(tokenList))
		}
		if token.TokenType == NUMBER && len(token.Text) == 5 {
			t.Fatalf("five-digit executable gate leaked into token stream: %s", ReprTokens(tokenList))
		}
	}
}
