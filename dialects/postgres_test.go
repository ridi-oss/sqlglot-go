package dialects_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
	"github.com/ridi/sqlglot-go/schema"
	"github.com/ridi/sqlglot-go/tokens"
)

func TestPostgresConfigAndTokenizer(t *testing.T) {
	d, err := dialects.GetOrRaise("postgres")
	if err != nil {
		t.Fatalf("GetOrRaise(postgres): %v", err)
	}
	if d.Name != "postgres" {
		t.Fatalf("Name = %q, want postgres", d.Name)
	}
	if d.NormalizationStrategy != dialects.Lowercase {
		t.Fatalf("NormalizationStrategy = %v, want Lowercase", d.NormalizationStrategy)
	}
	if d.IndexOffset != 1 || !d.TypedDivision || d.NullOrdering != "nulls_are_large" || !d.SupportsLimitAll || !d.TablesReferenceableAsColumns {
		t.Fatalf("dialect flags: IndexOffset=%d TypedDivision=%v NullOrdering=%q SupportsLimitAll=%v TablesReferenceableAsColumns=%v", d.IndexOffset, d.TypedDivision, d.NullOrdering, d.SupportsLimitAll, d.TablesReferenceableAsColumns)
	}
	if d.QuoteStart != "'" || d.QuoteEnd != "'" || d.IdentifierStart != "\"" || d.IdentifierEnd != "\"" {
		t.Fatalf("delimiters = quote %q/%q identifier %q/%q", d.QuoteStart, d.QuoteEnd, d.IdentifierStart, d.IdentifierEnd)
	}

	baseTokens, err := sqlglot.Tokenize("HSTORE", "")
	if err != nil {
		t.Fatalf("Tokenize(base HSTORE): %v", err)
	}
	if len(baseTokens) != 1 || baseTokens[0].TokenType != tokens.VAR {
		t.Fatalf("base HSTORE tokens = %s, want VAR", tokens.ReprTokens(baseTokens))
	}

	toks, err := sqlglot.Tokenize("SELECT HSTORE, /*+ hi */ 1, b'1010', x'1F', e'abc', $tag$abc$tag$, $1, x @@ y", "postgres")
	if err != nil {
		t.Fatalf("Tokenize(postgres): %v", err)
	}
	for _, tok := range toks {
		if tok.TokenType == tokens.HINT {
			t.Fatalf("postgres hint comment produced HINT token: %s", tokens.ReprTokens(toks))
		}
	}
	wantTypes := []tokens.TokenType{
		tokens.SELECT,
		tokens.HSTORE,
		tokens.COMMA,
		tokens.NUMBER,
		tokens.COMMA,
		tokens.BIT_STRING,
		tokens.COMMA,
		tokens.HEX_STRING,
		tokens.COMMA,
		tokens.BYTE_STRING,
		tokens.COMMA,
		tokens.HEREDOC_STRING,
		tokens.COMMA,
		tokens.PARAMETER,
		tokens.NUMBER,
		tokens.COMMA,
		tokens.VAR,
		tokens.DAT,
		tokens.VAR,
	}
	if len(toks) != len(wantTypes) {
		t.Fatalf("token count = %d, want %d: %s", len(toks), len(wantTypes), tokens.ReprTokens(toks))
	}
	for i, want := range wantTypes {
		if toks[i].TokenType != want {
			t.Fatalf("token %d type = %s, want %s: %s", i, toks[i].TokenType, want, tokens.ReprTokens(toks))
		}
	}
	if len(toks[2].Comments) != 1 || toks[2].Comments[0] != "+ hi " {
		t.Fatalf("postgres /*+ */ comment = %#v, want [\"+ hi \"]", toks[2].Comments)
	}
	if toks[5].Text != "1010" || toks[7].Text != "1F" || toks[9].Text != "abc" || toks[11].Text != "abc" {
		t.Fatalf("unexpected literal token text: %s", tokens.ReprTokens(toks))
	}
}

func TestPostgresProbeQualifyTraverseScope(t *testing.T) {
	expression, err := sqlglot.ParseOne("SELECT Foo, bar FROM MyTable", "postgres")
	if err != nil {
		t.Fatalf("ParseOne(postgres): %v", err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "postgres"
	opts.Schema = schema.M("MyTable", schema.M("Foo", "INT", "bar", "INT"))
	opts.InferSchema = boolPtr(true)
	qualified := optimizer.Qualify(expression, opts)
	got, err := sqlglot.Generate(qualified, "postgres", generator.Options{})
	if err != nil {
		t.Fatalf("Generate(postgres): %v", err)
	}
	want := "SELECT \"mytable\".\"foo\" AS \"foo\", \"mytable\".\"bar\" AS \"bar\" FROM \"mytable\" AS \"mytable\""
	if got != want {
		t.Fatalf("qualified postgres = %q, want %q", got, want)
	}
	if scopes := optimizer.TraverseScope(qualified); len(scopes) != 1 {
		t.Fatalf("TraverseScope(postgres) len = %d, want 1", len(scopes))
	}
}
