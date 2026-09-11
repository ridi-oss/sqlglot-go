package dialects_test

import (
	"reflect"
	"testing"

	"github.com/ridi-oss/sqlglot-go/dialects"
	"github.com/ridi-oss/sqlglot-go/tokens"
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
		d.NormalizationStrategy != dialects.CaseInsensitive ||
		d.SupportsUserDefinedTypes != base.SupportsUserDefinedTypes {
		t.Fatalf("Athena should retain base flags except case-insensitive normalization: athena=%+v base=%+v", d, base)
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

func TestAthenaUnloadKeywordIsAthenaOnly(t *testing.T) {
	sql := `UNLOAD (SELECT * FROM "t") TO 's3://x'`
	athena := athenaTokens(t, sql)
	if len(athena) < 3 || athena[0].TokenType != tokens.UNLOAD || athena[1].TokenType != tokens.L_PAREN || athena[2].TokenType != tokens.SELECT {
		t.Fatalf("Athena UNLOAD tokens = %s, want unpacked query tokens", tokens.ReprTokens(athena))
	}

	trino, err := dialects.Trino().NewTokenizer().Tokenize(sql)
	if err != nil {
		t.Fatalf("Tokenize(trino UNLOAD): %v", err)
	}
	if len(trino) == 0 || trino[0].TokenType != tokens.VAR || trino[0].Text != "UNLOAD" {
		t.Fatalf("standalone Trino UNLOAD leaked Athena UNLOAD mapping: %s", tokens.ReprTokens(trino))
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
	if !hive.TokenizerConfig.Commands[tokens.SHOW] || !trino.TokenizerConfig.Commands[tokens.COMMAND] {
		t.Fatal("Athena command unpacking mutated a source dialect")
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

func TestAthenaSemicolonBatchRoutesEachStatement(t *testing.T) {
	for _, sql := range []string{
		`SHOW TABLES; SELECT "$path" FROM t`,
		"CREATE TABLE `t` (x INT); SELECT \"x\" FROM \"t\"",
	} {
		got := athenaTokens(t, sql)
		if !hasHiveTokenStream(got) {
			t.Fatalf("DDL must route through Hive: %s", tokens.ReprTokens(got))
		}
		foundIdentifier := false
		for _, token := range got {
			if token.TokenType == tokens.UNKNOWN || token.TokenType == tokens.STRING && token.Text != "TABLES" {
				t.Fatalf("incorrect engine token: %s", token)
			}
			if token.TokenType == tokens.IDENTIFIER && (token.Text == "$path" || token.Text == "x") {
				foundIdentifier = true
			}
		}
		if !foundIdentifier {
			t.Fatalf("missing quoted query identifier: %s", tokens.ReprTokens(got))
		}
	}
	got := athenaTokens(t, `SHOW TABLES; SELECT 1; SHOW TABLES`)
	count := 0
	for i, token := range got {
		if token.TokenType == tokens.HIVE_TOKEN_STREAM {
			count++
			if i > 0 && got[i-1].TokenType != tokens.SEMICOLON {
				t.Fatalf("sentinel must lead a statement: %s", tokens.ReprTokens(got))
			}
		}
	}
	if count != 2 {
		t.Fatalf("sentinel count = %d, want 2", count)
	}
}

func TestAthenaTokenizerPreservesPositionsAndComments(t *testing.T) {
	for _, sql := range []string{
		"-- lead\nSELECT '한;글'; -- same line\nSELECT \"x;y\"; /* tail */",
		"SELECT 1; /* middle */ SELECT 2; SELECT 3",
		"SELECT 1;\r\n-- next\r\nSELECT 2;\rSELECT 3",
		";; SELECT 1; ; -- end",
		"SELECT 1;\n-- lead\nSELECT 2\n-- trailing",
		"  ", "-- comment only", "SELECT 1;   ",
	} {
		want, err := dialects.Trino().NewTokenizer().Tokenize(sql)
		if err != nil {
			t.Fatal(err)
		}
		got := athenaTokens(t, sql)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%q\ngot  %s\nwant %s", sql, tokens.ReprTokens(got), tokens.ReprTokens(want))
		}
	}
	for _, sql := range []string{
		"-- lead\nCREATE TABLE `한;글` (x INT); -- tail",
		`SELECT "a" FROM "t"`,
	} {
		got := athenaTokens(t, sql)
		d := dialects.Trino()
		if hasHiveTokenStream(got) {
			d = dialects.Hive()
			got = got[1:]
		}
		want, err := d.NewTokenizer().Tokenize(sql)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("single-statement tokens changed: got %s, want %s", tokens.ReprTokens(got), tokens.ReprTokens(want))
		}
	}
}

func TestAthenaMixedTokenizerPositions(t *testing.T) {
	const sql = "-- lead\nCREATE TABLE `한;글` (x INT); -- DDL tail\nSELECT 1; /* query tail */ CREATE TABLE `next` (x INT);"
	got := athenaTokens(t, sql)
	var unmarked []tokens.Token
	for _, token := range got {
		if token.TokenType != tokens.HIVE_TOKEN_STREAM {
			unmarked = append(unmarked, token)
		}
	}
	want, err := dialects.Hive().NewTokenizer().Tokenize(sql)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unmarked, want) {
		t.Fatalf("mixed statement positions/comments changed:\ngot %s\nwant %s", tokens.ReprTokens(unmarked), tokens.ReprTokens(want))
	}

	// Engines disagree on double quotes: the query chunk must be Trino-tokenized at exact offsets.
	const mixed = "CREATE TABLE `한;글` (x INT);\nSELECT \"$path\" FROM t; CREATE TABLE `n` (y INT)"
	runes := []rune(mixed)
	pathSeen := false
	for _, token := range athenaTokens(t, mixed) {
		if token.TokenType == tokens.HIVE_TOKEN_STREAM {
			continue
		}
		if token.Text == "$path" {
			pathSeen = true
			if token.TokenType != tokens.IDENTIFIER || string(runes[token.Start:token.End+1]) != `"$path"` || token.Line != 2 {
				t.Fatalf("query chunk not Trino-tokenized in place: %s", tokens.ReprTokens([]tokens.Token{token}))
			}
		}
		if token.TokenType == tokens.VAR || token.TokenType == tokens.SELECT || token.TokenType == tokens.CREATE {
			if got := string(runes[token.Start : token.End+1]); got != token.Text {
				t.Fatalf("token %q offsets point at %q", token.Text, got)
			}
		}
	}
	if !pathSeen {
		t.Fatal("\"$path\" token missing")
	}
}

func TestAthenaBoundaryDisagreementsAndHintComments(t *testing.T) {
	// Hive reads "…" as a backslash-escaped string; the classifier reads it as an identifier.
	got := athenaTokens(t, `CREATE TABLE t (x INT) LOCATION "a\";b\"c"; SELECT 1`)
	semis := 0
	for _, token := range got {
		if token.TokenType == tokens.SEMICOLON {
			semis++
		}
		if token.TokenType == tokens.STRING && token.Text != `a";b"c` {
			t.Fatalf("string split at an escaped quote: %s", tokens.ReprTokens(got))
		}
	}
	if semis != 1 {
		t.Fatalf("want one statement boundary, got %d: %s", semis, tokens.ReprTokens(got))
	}

	// Trino has no `/*+` hint; the trailing comment payload must be Trino's, not the classifier's.
	got = athenaTokens(t, "SELECT 1; /*+ keep */ SELECT 2")
	want, err := dialects.Trino().NewTokenizer().Tokenize("SELECT 1; /*+ keep */ SELECT 2")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hint comment payload changed:\ngot  %s\nwant %s", tokens.ReprTokens(got), tokens.ReprTokens(want))
	}
}
