package dialects_test

import (
	"testing"

	"github.com/ridi/sqlglot-go/dialects"
	"github.com/ridi/sqlglot-go/tokens"
)

func athenaTokens(t *testing.T, sql string) []tokens.Token {
	t.Helper()
	tokenizer := dialects.Athena().NewTokenizer()
	got, err := tokenizer.Tokenize(sql)
	if err != nil {
		t.Fatalf("Tokenize(athena %q): %v", sql, err)
	}
	return got
}

func hasHiveTokenStream(got []tokens.Token) bool {
	return len(got) > 0 && got[0].TokenType == tokens.HIVE_TOKEN_STREAM
}

func TestAthenaIsOuterBaseDialect(t *testing.T) {
	d, err := dialects.GetOrRaise("AtHeNa")
	if err != nil {
		t.Fatalf("GetOrRaise(athena): %v", err)
	}
	base := dialects.Base()
	if d.Name != "athena" {
		t.Fatalf("Name = %q, want athena", d.Name)
	}
	if d.IndexOffset != base.IndexOffset ||
		d.NormalizationStrategy != base.NormalizationStrategy ||
		d.SupportsUserDefinedTypes != base.SupportsUserDefinedTypes {
		t.Fatalf("Athena should retain outer base flags: athena=%+v base=%+v", d, base)
	}
	if d.TokenizerFactory == nil {
		t.Fatal("Athena TokenizerFactory = nil, want classify-and-re-tokenize factory")
	}
}

func TestAthenaTokenizerClassifierBranches(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		hive bool
	}{
		{name: "fewer than two tokens", sql: "SELECT", hive: false},
		{name: "single compound MSCK token stays Trino", sql: "MSCK REPAIR", hive: false},
		{name: "describe", sql: "DESCRIBE t", hive: true},
		{name: "show", sql: "SHOW TABLES", hive: true},
		{name: "MSCK REPAIR text", sql: "MSCK REPAIR TABLE t", hive: true},
		{name: "alter database", sql: "ALTER DATABASE d SET LOCATION 'x'", hive: true},
		{name: "create external", sql: "CREATE EXTERNAL TABLE t (x INT)", hive: true},
		{name: "drop schema", sql: "DROP SCHEMA s", hive: true},
		{name: "view exclusion", sql: "DROP VIEW v", hive: false},
		{name: "SELECT in remaining DDL tokens", sql: "CREATE TABLE t AS SELECT 1", hive: false},
		{name: "DDL without SELECT", sql: "CREATE TABLE t (x INT)", hive: true},
		{name: "non-DDL", sql: "SELECT x", hive: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := athenaTokens(t, tc.sql)
			if hive := hasHiveTokenStream(got); hive != tc.hive {
				t.Fatalf("Hive routing = %v, want %v: %s", hive, tc.hive, tokens.ReprTokens(got))
			}
			count := 0
			for _, token := range got {
				if token.TokenType == tokens.HIVE_TOKEN_STREAM {
					count++
				}
			}
			wantCount := 0
			if tc.hive {
				wantCount = 1
			}
			if count != wantCount {
				t.Fatalf("HIVE_TOKEN_STREAM count = %d, want %d: %s", count, wantCount, tokens.ReprTokens(got))
			}
		})
	}
}

func TestAthenaRetokenizesWithChosenEngine(t *testing.T) {
	hiveSQL := "CREATE TABLE `t` (c STRING) LOCATION \"s3://bucket/path\""
	hiveTokens := athenaTokens(t, hiveSQL)
	if !hasHiveTokenStream(hiveTokens) {
		t.Fatalf("Hive DDL has no sentinel: %s", tokens.ReprTokens(hiveTokens))
	}
	foundBacktickIdentifier := false
	foundDoubleQuotedString := false
	for _, token := range hiveTokens[1:] {
		if token.TokenType == tokens.IDENTIFIER && token.Text == "t" {
			foundBacktickIdentifier = true
		}
		if token.TokenType == tokens.STRING && token.Text == "s3://bucket/path" {
			foundDoubleQuotedString = true
		}
	}
	if !foundBacktickIdentifier || !foundDoubleQuotedString {
		t.Fatalf("Hive re-tokenization did not accept backticks/double-quoted strings: %s", tokens.ReprTokens(hiveTokens))
	}

	trinoTokens := athenaTokens(t, `SELECT "a" FROM "t"`)
	if hasHiveTokenStream(trinoTokens) {
		t.Fatalf("Trino query unexpectedly routed to Hive: %s", tokens.ReprTokens(trinoTokens))
	}
	wantTypes := []tokens.TokenType{tokens.SELECT, tokens.IDENTIFIER, tokens.FROM, tokens.IDENTIFIER}
	if len(trinoTokens) != len(wantTypes) {
		t.Fatalf("Trino query token count = %d, want %d: %s", len(trinoTokens), len(wantTypes), tokens.ReprTokens(trinoTokens))
	}
	for i, want := range wantTypes {
		if trinoTokens[i].TokenType != want {
			t.Fatalf("Trino query token %d = %s, want %s: %s", i, trinoTokens[i].TokenType, want, tokens.ReprTokens(trinoTokens))
		}
	}
}

func TestAthenaUnloadIsCommandOnlyInAthena(t *testing.T) {
	sql := `UNLOAD (SELECT * FROM "t") TO 's3://x'`
	athena := athenaTokens(t, sql)
	if len(athena) != 2 || athena[0].TokenType != tokens.COMMAND || athena[0].Text != "UNLOAD" || athena[1].TokenType != tokens.STRING {
		t.Fatalf("Athena UNLOAD tokens = %s, want COMMAND plus packed STRING", tokens.ReprTokens(athena))
	}

	trino, err := dialects.Trino().NewTokenizer().Tokenize(sql)
	if err != nil {
		t.Fatalf("Tokenize(trino UNLOAD): %v", err)
	}
	if len(trino) == 0 || trino[0].TokenType != tokens.VAR || trino[0].Text != "UNLOAD" {
		t.Fatalf("standalone Trino UNLOAD leaked Athena COMMAND mapping: %s", tokens.ReprTokens(trino))
	}
}

func TestAthenaTokenizerDoesNotMutateSourceDialects(t *testing.T) {
	base := dialects.Base()
	presto := dialects.Presto()
	trino := dialects.Trino()
	hive := dialects.Hive()

	_ = athenaTokens(t, "CREATE TABLE `t` (x BIGINT)")

	for name, d := range map[string]*dialects.Dialect{
		"base":   base,
		"presto": presto,
		"trino":  trino,
		"hive":   hive,
	} {
		if _, ok := d.TokenizerConfig.Keywords["UNLOAD"]; ok {
			t.Errorf("%s tokenizer received Athena-only UNLOAD", name)
		}
	}
	if _, ok := base.TokenizerConfig.Identifiers['`']; ok {
		t.Fatal("base tokenizer received Athena's merged backtick identifier")
	}
	if trino.TokenizerConfig.StringEscapes['\\'] {
		t.Fatal("Trino tokenizer received Athena's merged Hive string escape")
	}
	if _, ok := trino.TokenizerConfig.NumericLiterals["L"]; ok {
		t.Fatal("Trino tokenizer received Athena's merged Hive numeric literal")
	}
	if hive.TokenizerConfig.Quotes[`"`] != `"` {
		t.Fatal("Hive double-quote string configuration changed")
	}
}

func TestAthenaLeadingCommentsDoNotAffectRouting(t *testing.T) {
	got := athenaTokens(t, "-- lead\nCREATE TABLE `t` (x INT)")
	if !hasHiveTokenStream(got) {
		t.Fatalf("leading comment prevented Hive routing: %s", tokens.ReprTokens(got))
	}
	if len(got) < 2 || got[1].TokenType != tokens.CREATE || len(got[1].Comments) != 1 || got[1].Comments[0] != " lead" {
		t.Fatalf("leading comment was not retained on Hive CREATE: %s", tokens.ReprTokens(got))
	}
}

func TestAthenaSemicolonBatchUsesOneGlobalRoute(t *testing.T) {
	hiveBatch := athenaTokens(t, `SHOW TABLES; SELECT "x" FROM "t"`)
	if !hasHiveTokenStream(hiveBatch) {
		t.Fatalf("SHOW batch should route wholly through Hive: %s", tokens.ReprTokens(hiveBatch))
	}
	foundHiveString := false
	for _, token := range hiveBatch[1:] {
		if token.TokenType == tokens.STRING && token.Text == "x" {
			foundHiveString = true
		}
	}
	if !foundHiveString {
		t.Fatalf("second statement was not re-tokenized through the global Hive route: %s", tokens.ReprTokens(hiveBatch))
	}

	trinoBatch := athenaTokens(t, "CREATE TABLE `t` (x INT); SELECT \"x\" FROM \"t\"")
	if hasHiveTokenStream(trinoBatch) {
		t.Fatalf("DDL batch containing a later SELECT should route wholly through Trino: %s", tokens.ReprTokens(trinoBatch))
	}
	foundBacktickUnknown := false
	foundTrinoIdentifier := false
	for _, token := range trinoBatch {
		if token.TokenType == tokens.UNKNOWN && token.Text == "`" {
			foundBacktickUnknown = true
		}
		if token.TokenType == tokens.IDENTIFIER && token.Text == "x" {
			foundTrinoIdentifier = true
		}
	}
	if !foundBacktickUnknown || !foundTrinoIdentifier {
		t.Fatalf("batch did not retain the one global Trino route: %s", tokens.ReprTokens(trinoBatch))
	}
}
