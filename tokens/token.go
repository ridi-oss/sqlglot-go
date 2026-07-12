package tokens

import (
	"fmt"
	"strconv"
	"strings"
)

// Token is one lexeme from Tokenize. Text is the DECODED value — for a string/quoted identifier
// the surrounding quotes are stripped and escapes resolved, so Text is NOT the raw source. To
// recover the exact source lexeme (e.g. for byte-exact hashing), slice the original source by
// Start/End, not Text. Start and End are INCLUSIVE rune offsets into the original source string
// passed to Tokenize (the tokenizer operates on []rune(sql)); string([]rune(sql)[Start:End+1])
// yields the verbatim lexeme, and this holds for every token kind (STRING, quoted identifier,
// NUMBER, keyword, VAR, operator). Ordinary comments are attached to Comments rather than emitted
// as tokens.
type Token struct {
	TokenType TokenType
	Text      string
	Line      int
	Col       int
	Start     int // inclusive rune offset into the original source
	End       int // inclusive rune offset into the original source
	Comments  []string
}

var SentinelNone = Token{TokenType: SENTINEL}

func Number(number int) Token {
	return NewToken(NUMBER, strconv.Itoa(number))
}

func StringToken(s string) Token {
	return NewToken(STRING, s)
}

func IdentifierToken(identifier string) Token {
	return NewToken(IDENTIFIER, identifier)
}

func VarToken(v string) Token {
	return NewToken(VAR, v)
}

func NewToken(tokenType TokenType, text string) Token {
	return Token{TokenType: tokenType, Text: text, Line: 1, Col: 1, Start: 0, End: 0, Comments: []string{}}
}

func NewTokenFull(tokenType TokenType, text string, line, col, start, end int, comments []string) Token {
	if comments == nil {
		comments = []string{}
	}
	return Token{TokenType: tokenType, Text: text, Line: line, Col: col, Start: start, End: end, Comments: comments}
}

func (t Token) IsValid() bool {
	return t.TokenType != SENTINEL
}

func (t Token) String() string {
	return fmt.Sprintf(
		"<Token token_type: %s, text: %s, line: %d, col: %d, start: %d, end: %d, comments: %s>",
		t.TokenType,
		t.Text,
		t.Line,
		t.Col,
		t.Start,
		t.End,
		pythonStringList(t.Comments),
	)
}

func ReprTokens(tokens []Token) string {
	parts := make([]string, len(tokens))
	for i, token := range tokens {
		parts[i] = token.String()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func pythonStringList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = pyStringRepr(value)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func pyStringRepr(s string) string {
	quote := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, "\"") {
		quote = "\""
	}
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\t", "\\t")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	if quote == "'" {
		escaped = strings.ReplaceAll(escaped, "'", "\\'")
	} else {
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	}
	return quote + escaped + quote
}
