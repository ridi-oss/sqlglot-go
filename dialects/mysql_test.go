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

func TestMySQLConfigAndTokenizer(t *testing.T) {
	d, err := dialects.GetOrRaise("mysql")
	if err != nil {
		t.Fatalf("GetOrRaise(mysql): %v", err)
	}
	if d.Name != "mysql" {
		t.Fatalf("Name = %q, want mysql", d.Name)
	}
	if d.NormalizationStrategy != dialects.CaseSensitive {
		t.Fatalf("NormalizationStrategy = %v, want CaseSensitive", d.NormalizationStrategy)
	}
	if d.DPipeIsStringConcat || d.SupportsUserDefinedTypes || !d.SafeDivision {
		t.Fatalf("dialect flags: DPipeIsStringConcat=%v SupportsUserDefinedTypes=%v SafeDivision=%v", d.DPipeIsStringConcat, d.SupportsUserDefinedTypes, d.SafeDivision)
	}
	if d.QuoteStart != "'" || d.QuoteEnd != "'" || d.IdentifierStart != "`" || d.IdentifierEnd != "`" {
		t.Fatalf("delimiters = quote %q/%q identifier %q/%q", d.QuoteStart, d.QuoteEnd, d.IdentifierStart, d.IdentifierEnd)
	}
	if !d.ValidIntervalUnits["SECOND_MICROSECOND"] || !d.ValidIntervalUnits["YEAR_MONTH"] {
		t.Fatalf("missing MySQL compound interval units")
	}

	toks, err := sqlglot.Tokenize("SELECT \"x\", `A`, b'1010', x'1F', 0b1010, 0x1F, @@foo # hi\nFROM t", "mysql")
	if err != nil {
		t.Fatalf("Tokenize(mysql): %v", err)
	}
	wantTypes := []tokens.TokenType{
		tokens.SELECT,
		tokens.STRING,
		tokens.COMMA,
		tokens.IDENTIFIER,
		tokens.COMMA,
		tokens.BIT_STRING,
		tokens.COMMA,
		tokens.HEX_STRING,
		tokens.COMMA,
		tokens.BIT_STRING,
		tokens.COMMA,
		tokens.HEX_STRING,
		tokens.COMMA,
		tokens.SESSION_PARAMETER,
		tokens.VAR,
		tokens.FROM,
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
	if toks[1].Text != "x" || toks[3].Text != "A" || toks[5].Text != "1010" || toks[7].Text != "1F" || toks[9].Text != "1010" || toks[11].Text != "1F" {
		t.Fatalf("unexpected token text: %s", tokens.ReprTokens(toks))
	}
	if len(toks[14].Comments) != 1 || toks[14].Comments[0] != " hi" {
		t.Fatalf("hash comment = %#v, want [\" hi\"]", toks[14].Comments)
	}
}

func TestMySQLProbeQualifyTraverseScope(t *testing.T) {
	expression, err := sqlglot.ParseOne("SELECT Foo, bar FROM MyTable", "mysql")
	if err != nil {
		t.Fatalf("ParseOne(mysql): %v", err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "mysql"
	opts.Schema = schema.M("MyTable", schema.M("Foo", "INT", "bar", "INT"))
	opts.InferSchema = boolPtr(true)
	qualified := optimizer.Qualify(expression, opts)
	got, err := sqlglot.Generate(qualified, "mysql", generator.Options{})
	if err != nil {
		t.Fatalf("Generate(mysql): %v", err)
	}
	want := "SELECT `MyTable`.`Foo` AS `Foo`, `MyTable`.`bar` AS `bar` FROM `MyTable` AS `MyTable`"
	if got != want {
		t.Fatalf("qualified mysql = %q, want %q", got, want)
	}
	if scopes := optimizer.TraverseScope(qualified); len(scopes) != 1 {
		t.Fatalf("TraverseScope(mysql) len = %d, want 1", len(scopes))
	}
}
