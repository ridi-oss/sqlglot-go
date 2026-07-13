package dialects_test

import (
	"testing"

	"github.com/ridi/sqlglot-go/dialects"
	"github.com/ridi/sqlglot-go/tokens"
)

func TestTrinoConfigAndTokenizer(t *testing.T) {
	d, err := dialects.GetOrRaise("TrInO")
	if err != nil {
		t.Fatalf("GetOrRaise(trino): %v", err)
	}
	if d.Name != "trino" {
		t.Fatalf("Name = %q, want trino", d.Name)
	}
	if d.SupportsUserDefinedTypes {
		t.Fatal("SupportsUserDefinedTypes = true, want false")
	}

	presto := dialects.Presto()
	if d.NormalizationStrategy != presto.NormalizationStrategy ||
		d.IndexOffset != presto.IndexOffset ||
		d.NullOrdering != presto.NullOrdering ||
		d.StrictStringConcat != presto.StrictStringConcat ||
		d.TypedDivision != presto.TypedDivision ||
		d.TablesampleSizeIsPercent != presto.TablesampleSizeIsPercent ||
		d.SupportsLimitAll != presto.SupportsLimitAll ||
		d.SupportsValuesDefault != presto.SupportsValuesDefault ||
		d.ValuesFollowedByParen != presto.ValuesFollowedByParen ||
		d.ZoneAwareTimestampConstructor != presto.ZoneAwareTimestampConstructor {
		t.Fatalf("Trino did not preserve inherited Presto flags: trino=%+v presto=%+v", d, presto)
	}
	if d.TokenizerConfig.HasHexStrings != presto.TokenizerConfig.HasHexStrings ||
		d.TokenizerConfig.NestedComments != presto.TokenizerConfig.NestedComments ||
		d.TokenizerConfig.FormatStrings["U&'"] != presto.TokenizerConfig.FormatStrings["U&'"] {
		t.Fatal("Trino did not preserve inherited Presto tokenizer configuration")
	}

	got, err := d.NewTokenizer().Tokenize("REFRESH")
	if err != nil {
		t.Fatalf("Tokenize(trino REFRESH): %v", err)
	}
	if len(got) != 1 || got[0].TokenType != tokens.REFRESH || got[0].Text != "REFRESH" {
		t.Fatalf("Trino REFRESH tokens = %s, want one REFRESH token", tokens.ReprTokens(got))
	}
}

func TestTrinoConstructorDoesNotMutateOtherDialects(t *testing.T) {
	base := dialects.Base()
	presto := dialects.Presto()
	hive := dialects.Hive()
	trino := dialects.Trino()

	for name, d := range map[string]*dialects.Dialect{
		"base":   base,
		"presto": presto,
		"hive":   hive,
	} {
		if d.Functions["VERSION"] != nil || d.Functions["ARRAY_FIRST"] != nil {
			t.Errorf("%s function registry received Trino-only backfills", name)
		}
		if _, ok := d.TokenizerConfig.Keywords["UNLOAD"]; ok {
			t.Errorf("%s tokenizer received Athena-only UNLOAD", name)
		}
	}
	if _, ok := base.TokenizerConfig.Keywords["REFRESH"]; ok {
		t.Fatal("base tokenizer received Trino REFRESH")
	}
	if _, ok := presto.TokenizerConfig.Keywords["REFRESH"]; ok {
		t.Fatal("Presto tokenizer received Trino REFRESH")
	}
	if hive.TokenizerConfig.Keywords["REFRESH"] != tokens.REFRESH {
		t.Fatal("Hive's own REFRESH mapping changed")
	}

	trino.Functions["MUTATED"] = trino.Functions["VERSION"]
	trino.TokenizerConfig.Keywords["MUTATED"] = tokens.COMMAND
	fresh := dialects.Trino()
	if fresh.Functions["MUTATED"] != nil {
		t.Fatal("a fresh Trino shared the previous Trino function map")
	}
	if _, ok := fresh.TokenizerConfig.Keywords["MUTATED"]; ok {
		t.Fatal("a fresh Trino shared the previous Trino keyword map")
	}
}
