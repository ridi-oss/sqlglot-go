package dialects_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	"github.com/ridi/sqlglot-go/tokens"
)

// TestPrestoConfigAndTokenizer mirrors TestPostgresConfigAndTokenizer: it pins the dialect flags
// ported from dialects/presto.py:18-35 (+ parsers/presto.py:60-61) and the Tokenizer config from
// dialects/presto.py:44-69 (HEX x'..' strings, U&'..' unicode strings, /*+ demoted to an ordinary
// comment, ROW remapped to the STRUCT token).
func TestPrestoConfigAndTokenizer(t *testing.T) {
	d, err := dialects.GetOrRaise("presto")
	if err != nil {
		t.Fatalf("GetOrRaise(presto): %v", err)
	}
	if d.Name != "presto" {
		t.Fatalf("Name = %q, want presto", d.Name)
	}
	if d.NormalizationStrategy != dialects.CaseInsensitive {
		t.Fatalf("NormalizationStrategy = %v, want CaseInsensitive", d.NormalizationStrategy)
	}
	if d.IndexOffset != 1 || !d.TypedDivision || d.NullOrdering != "nulls_are_last" || !d.SupportsLimitAll || !d.StrictStringConcat || !d.TablesampleSizeIsPercent {
		t.Fatalf("dialect flags: IndexOffset=%d TypedDivision=%v NullOrdering=%q SupportsLimitAll=%v StrictStringConcat=%v TablesampleSizeIsPercent=%v", d.IndexOffset, d.TypedDivision, d.NullOrdering, d.SupportsLimitAll, d.StrictStringConcat, d.TablesampleSizeIsPercent)
	}
	if d.SupportsValuesDefault || d.ValuesFollowedByParen || !d.ZoneAwareTimestampConstructor {
		t.Fatalf("parser flags: SupportsValuesDefault=%v ValuesFollowedByParen=%v ZoneAwareTimestampConstructor=%v, want false/false/true", d.SupportsValuesDefault, d.ValuesFollowedByParen, d.ZoneAwareTimestampConstructor)
	}
	// presto.py declares no Tokenizer QUOTES/IDENTIFIERS override, so the delimiters inherit
	// base ANSI '/".
	if d.QuoteStart != "'" || d.QuoteEnd != "'" || d.IdentifierStart != "\"" || d.IdentifierEnd != "\"" {
		t.Fatalf("delimiters = quote %q/%q identifier %q/%q", d.QuoteStart, d.QuoteEnd, d.IdentifierStart, d.IdentifierEnd)
	}
	if d.HexStart != "x'" || d.HexEnd != "'" {
		t.Fatalf("hex delimiters = %q/%q, want x'/'", d.HexStart, d.HexEnd)
	}

	// Base is unaffected by presto's UNICODE_STRINGS addition: U&'abc' must NOT tokenize as a
	// UNICODE_STRING under the base dialect (guards the additive-only contract).
	baseTokens, err := sqlglot.Tokenize("U&'abc'", "")
	if err != nil {
		t.Fatalf("Tokenize(base U&'abc'): %v", err)
	}
	for _, tok := range baseTokens {
		if tok.TokenType == tokens.UNICODE_STRING {
			t.Fatalf("base produced UNICODE_STRING (presto config leaked): %s", tokens.ReprTokens(baseTokens))
		}
	}

	toks, err := sqlglot.Tokenize("SELECT x'1F', U&'abc', /*+ hi */ 1, ROW(a)", "presto")
	if err != nil {
		t.Fatalf("Tokenize(presto): %v", err)
	}
	for _, tok := range toks {
		if tok.TokenType == tokens.HINT {
			t.Fatalf("presto /*+ */ produced HINT token (QUALIFY/hint pop failed): %s", tokens.ReprTokens(toks))
		}
	}
	wantTypes := []tokens.TokenType{
		tokens.SELECT,
		tokens.HEX_STRING,
		tokens.COMMA,
		tokens.UNICODE_STRING,
		tokens.COMMA,
		tokens.NUMBER,
		tokens.COMMA,
		tokens.STRUCT,
		tokens.L_PAREN,
		tokens.VAR,
		tokens.R_PAREN,
	}
	if len(toks) != len(wantTypes) {
		t.Fatalf("token count = %d, want %d: %s", len(toks), len(wantTypes), tokens.ReprTokens(toks))
	}
	for i, want := range wantTypes {
		if toks[i].TokenType != want {
			t.Fatalf("token %d type = %s, want %s: %s", i, toks[i].TokenType, want, tokens.ReprTokens(toks))
		}
	}
	if toks[1].Text != "1F" || toks[3].Text != "abc" {
		t.Fatalf("literal token text: hex=%q unicode=%q, want 1F/abc", toks[1].Text, toks[3].Text)
	}
	// ROW is remapped to TokenType.STRUCT (dialects/presto.py:62) but keeps its source text.
	if toks[7].Text != "ROW" {
		t.Fatalf("ROW->STRUCT token text = %q, want ROW", toks[7].Text)
	}
	// The /*+ hi */ block is an ordinary comment attached to the following COMMA, not a HINT.
	if len(toks[4].Comments) != 1 || toks[4].Comments[0] != "+ hi " {
		t.Fatalf("presto /*+ */ comment = %#v, want [\"+ hi \"]", toks[4].Comments)
	}
}
