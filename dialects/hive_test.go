package dialects_test

import (
	"testing"

	"github.com/ridi/sqlglot-go/dialects"
	"github.com/ridi/sqlglot-go/tokens"
)

func TestHiveConfigAndTokenizer(t *testing.T) {
	d, err := dialects.GetOrRaise("hive")
	if err != nil {
		t.Fatalf("GetOrRaise(hive): %v", err)
	}
	if d.Name != "hive" {
		t.Fatalf("Name = %q, want hive", d.Name)
	}
	if d.NormalizationStrategy != dialects.CaseInsensitive {
		t.Fatalf("NormalizationStrategy = %v, want CaseInsensitive", d.NormalizationStrategy)
	}
	if !d.AliasPostTablesample || d.SupportsUserDefinedTypes || !d.SafeDivision || !d.AlterTableSupportsCascade {
		t.Fatalf("dialect flags: AliasPostTablesample=%v SupportsUserDefinedTypes=%v SafeDivision=%v AlterTableSupportsCascade=%v, want true/false/true/true",
			d.AliasPostTablesample, d.SupportsUserDefinedTypes, d.SafeDivision, d.AlterTableSupportsCascade)
	}
	if d.ValuesFollowedByParen || !d.AlterTablePartitions {
		t.Fatalf("parser flags: ValuesFollowedByParen=%v AlterTablePartitions=%v, want false/true", d.ValuesFollowedByParen, d.AlterTablePartitions)
	}
	if d.StrictCast || !d.LogDefaultsToLn || !d.JoinsHaveEqualPrecedence || !d.AddJoinOnTrue {
		t.Fatalf("parser behavior flags: StrictCast=%v LogDefaultsToLn=%v JoinsHaveEqualPrecedence=%v AddJoinOnTrue=%v, want false/true/true/true",
			d.StrictCast, d.LogDefaultsToLn, d.JoinsHaveEqualPrecedence, d.AddJoinOnTrue)
	}
	if d.RegexpExtractDefaultGroup != 1 || !d.RegexpExtractPositionOverflowReturnsNull {
		t.Fatalf("regexp flags: default group=%d overflow returns null=%v, want 1/true",
			d.RegexpExtractDefaultGroup, d.RegexpExtractPositionOverflowReturnsNull)
	}
	if d.QuoteStart != "'" || d.QuoteEnd != "'" || d.IdentifierStart != "`" || d.IdentifierEnd != "`" {
		t.Fatalf("delimiters = quote %q/%q identifier %q/%q", d.QuoteStart, d.QuoteEnd, d.IdentifierStart, d.IdentifierEnd)
	}

	cfg := d.TokenizerConfig
	if cfg.Quotes["'"] != "'" || cfg.Quotes[`"`] != `"` {
		t.Fatalf("Hive quotes = %#v, want both single and double quotes", cfg.Quotes)
	}
	if cfg.Identifiers['`'] != "`" {
		t.Fatalf("Hive identifiers = %#v, want backticks", cfg.Identifiers)
	}
	if !cfg.StringEscapes['\\'] || !cfg.IdentifiersCanStartWithDigit {
		t.Fatalf("tokenizer flags: backslash escape=%v IdentifiersCanStartWithDigit=%v, want true/true", cfg.StringEscapes['\\'], cfg.IdentifiersCanStartWithDigit)
	}
	if cfg.SingleTokens['$'] != tokens.PARAMETER {
		t.Fatalf("Hive '$' token = %v, want PARAMETER", cfg.SingleTokens['$'])
	}

	toks, err := d.NewTokenizer().Tokenize("SELECT `a`, \"b\", 1abc, $x")
	if err != nil {
		t.Fatalf("Tokenize(hive identifiers/strings/parameter): %v", err)
	}
	wantTypes := []tokens.TokenType{
		tokens.SELECT,
		tokens.IDENTIFIER,
		tokens.COMMA,
		tokens.STRING,
		tokens.COMMA,
		tokens.VAR,
		tokens.COMMA,
		tokens.PARAMETER,
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
	if toks[1].Text != "a" || toks[3].Text != "b" || toks[5].Text != "1abc" || toks[8].Text != "x" {
		t.Fatalf("unexpected Hive token text: %s", tokens.ReprTokens(toks))
	}
}

func TestHiveNumericLiteralTokenTriples(t *testing.T) {
	d := dialects.Hive()
	toks, err := d.NewTokenizer().Tokenize("1L 2S 3Y 4D 5F 6BD")
	if err != nil {
		t.Fatalf("Tokenize(hive numeric literals): %v", err)
	}

	cases := []struct {
		number, suffix string
		typeToken      tokens.TokenType
	}{
		{"1", "L", tokens.BIGINT},
		{"2", "S", tokens.SMALLINT},
		{"3", "Y", tokens.TINYINT},
		{"4", "D", tokens.DOUBLE},
		{"5", "F", tokens.FLOAT},
		{"6", "BD", tokens.DECIMAL},
	}
	if len(toks) != len(cases)*3 {
		t.Fatalf("token count = %d, want %d: %s", len(toks), len(cases)*3, tokens.ReprTokens(toks))
	}
	for i, tc := range cases {
		triple := toks[i*3 : i*3+3]
		want := []tokens.TokenType{tokens.NUMBER, tokens.DCOLON, tc.typeToken}
		for j, tokenType := range want {
			if triple[j].TokenType != tokenType {
				t.Fatalf("numeric literal %d token %d = %s, want %s: %s", i, j, triple[j].TokenType, tokenType, tokens.ReprTokens(toks))
			}
		}
		if triple[0].Text != tc.number || triple[1].Text != "::" || triple[2].Text != tc.suffix {
			t.Fatalf("numeric literal %d text = %q/%q/%q, want %q/::/%q", i, triple[0].Text, triple[1].Text, triple[2].Text, tc.number, tc.suffix)
		}
	}
}

func TestHiveKeywordOverlay(t *testing.T) {
	d := dialects.Hive()
	cases := []struct {
		sql  string
		want tokens.TokenType
	}{
		{"ADD ARCHIVE", tokens.COMMAND},
		{"ADD ARCHIVES", tokens.COMMAND},
		{"ADD FILE", tokens.COMMAND},
		{"ADD FILES", tokens.COMMAND},
		{"ADD JAR", tokens.COMMAND},
		{"ADD JARS", tokens.COMMAND},
		{"MINUS", tokens.EXCEPT},
		{"MSCK REPAIR", tokens.COMMAND},
		{"REFRESH", tokens.REFRESH},
		{"TIMESTAMP AS OF", tokens.TIMESTAMP_SNAPSHOT},
		{"VERSION AS OF", tokens.VERSION_SNAPSHOT},
		{"SERDEPROPERTIES", tokens.SERDE_PROPERTIES},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			toks, err := d.NewTokenizer().Tokenize(tc.sql)
			if err != nil {
				t.Fatalf("Tokenize(hive %q): %v", tc.sql, err)
			}
			if len(toks) != 1 || toks[0].TokenType != tc.want {
				t.Fatalf("tokens = %s, want one %s token", tokens.ReprTokens(toks), tc.want)
			}
		})
	}
}

func TestHiveTokenizerOverlayDoesNotLeak(t *testing.T) {
	for _, tc := range []struct {
		name string
		new  func() *dialects.Dialect
	}{
		{name: "base", new: dialects.Base},
		{name: "presto", new: dialects.Presto},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.new()

			backticks, err := d.NewTokenizer().Tokenize("`x`")
			if err != nil {
				t.Fatalf("Tokenize(%s backticks): %v", tc.name, err)
			}
			if len(backticks) != 3 || backticks[0].TokenType != tokens.UNKNOWN || backticks[1].TokenType != tokens.VAR || backticks[2].TokenType != tokens.UNKNOWN {
				t.Fatalf("%s backticks leaked Hive identifier syntax: %s", tc.name, tokens.ReprTokens(backticks))
			}

			doubleQuoted, err := d.NewTokenizer().Tokenize(`"x"`)
			if err != nil {
				t.Fatalf("Tokenize(%s double quote): %v", tc.name, err)
			}
			if len(doubleQuoted) != 1 || doubleQuoted[0].TokenType != tokens.IDENTIFIER {
				t.Fatalf("%s double quote should remain an identifier: %s", tc.name, tokens.ReprTokens(doubleQuoted))
			}

			numeric, err := d.NewTokenizer().Tokenize("1L")
			if err != nil {
				t.Fatalf("Tokenize(%s numeric suffix): %v", tc.name, err)
			}
			if len(numeric) != 2 || numeric[0].TokenType != tokens.NUMBER || numeric[1].TokenType != tokens.VAR {
				t.Fatalf("%s numeric suffix leaked Hive typed-literal tokens: %s", tc.name, tokens.ReprTokens(numeric))
			}

			serde, err := d.NewTokenizer().Tokenize("SERDEPROPERTIES")
			if err != nil {
				t.Fatalf("Tokenize(%s SERDEPROPERTIES): %v", tc.name, err)
			}
			if len(serde) != 1 || serde[0].TokenType != tokens.VAR {
				t.Fatalf("%s SERDEPROPERTIES leaked Hive keyword: %s", tc.name, tokens.ReprTokens(serde))
			}
		})
	}
}
