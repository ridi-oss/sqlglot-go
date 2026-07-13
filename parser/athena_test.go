package parser_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/dialects"
	exp "github.com/ridi/sqlglot-go/expressions"
	parserpkg "github.com/ridi/sqlglot-go/parser"
	"github.com/ridi/sqlglot-go/tokens"
)

func TestAthenaRoutesExternalDDLToHive(t *testing.T) {
	const sql = "CREATE EXTERNAL TABLE foo (id INT, val STRING) STORED AS PARQUET LOCATION 's3://foo' TBLPROPERTIES ('classification'='test')"
	athena := dialects.Athena()
	rawTokens, err := athena.NewTokenizer().Tokenize(sql)
	if err != nil {
		t.Fatalf("Athena tokenize external DDL: %v", err)
	}
	if len(rawTokens) == 0 || rawTokens[0].TokenType != tokens.HIVE_TOKEN_STREAM {
		t.Fatalf("external DDL did not receive HIVE_TOKEN_STREAM: %s", tokens.ReprTokens(rawTokens))
	}

	expressions, err := parserpkg.New(athena).Parse(rawTokens, sql)
	if err != nil {
		t.Fatalf("direct Athena parser external DDL: %v", err)
	}
	if len(expressions) != 1 || expressions[0] == nil {
		t.Fatalf("direct Athena parser returned %#v", expressions)
	}
	create := expressions[0]
	if create.Kind() != exp.KindCreate {
		t.Fatalf("Athena external DDL must be structured Create, never Command:\n%s", create.ToS())
	}
	if command := create.Find(exp.KindCommand); command != nil {
		t.Fatalf("Athena external DDL contains a nested Command:\n%s", create.ToS())
	}
	properties := createProperties(t, create)
	want := []exp.Kind{
		exp.KindExternalProperty,
		exp.KindFileFormatProperty,
		exp.KindLocationProperty,
		exp.KindProperty,
	}
	if len(properties) != len(want) {
		t.Fatalf("Athena external DDL property count = %d, want %d:\n%s", len(properties), len(want), create.ToS())
	}
	for i, kind := range want {
		if properties[i].Kind() != kind {
			t.Fatalf("Athena external DDL property %d = %v, want %v:\n%s", i, properties[i].Kind(), kind, create.ToS())
		}
	}
}

func TestAthenaRoutesQueriesAndSelectDDLToTrino(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		kind exp.Kind
	}{
		{
			name: "ctas",
			sql:  "CREATE TABLE foo WITH (format='parquet') AS SELECT * FROM a",
			kind: exp.KindCreate,
		},
		{
			name: "create view",
			sql:  "CREATE VIEW foo AS SELECT id FROM tbl",
			kind: exp.KindCreate,
		},
		{
			name: "select",
			sql:  "SELECT CURRENT_CATALOG",
			kind: exp.KindSelect,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			athena := dialects.Athena()
			rawTokens, err := athena.NewTokenizer().Tokenize(tc.sql)
			if err != nil {
				t.Fatalf("Athena tokenize: %v", err)
			}
			if len(rawTokens) > 0 && rawTokens[0].TokenType == tokens.HIVE_TOKEN_STREAM {
				t.Fatalf("%s was incorrectly routed to Hive: %s", tc.name, tokens.ReprTokens(rawTokens))
			}
			expressions, err := parserpkg.New(athena).Parse(rawTokens, tc.sql)
			if err != nil {
				t.Fatalf("direct Athena parse: %v", err)
			}
			if len(expressions) != 1 || expressions[0] == nil || expressions[0].Kind() != tc.kind {
				t.Fatalf("%s parse result = %#v, want one %v", tc.name, expressions, tc.kind)
			}
			if tc.name == "select" {
				projection := expressions[0].Expressions()[0]
				if projection.Kind() != exp.KindCurrentCatalog {
					t.Fatalf("Athena SELECT did not receive Trino CURRENT_CATALOG grammar:\n%s", expressions[0].ToS())
				}
			}
		})
	}
}

func TestAthenaUsingExternalFunctionIsParserLocal(t *testing.T) {
	const sql = "USING EXTERNAL FUNCTION some_function(input VARBINARY) RETURNS VARCHAR LAMBDA 'some-name' SELECT some_function(1)"
	command := parseOneDialect(t, sql, "athena")
	if command.Kind() != exp.KindCommand {
		t.Fatalf("Athena USING EXTERNAL FUNCTION kind = %v, want Command:\n%s", command.Kind(), command.ToS())
	}
	if got, err := generateSQL(t, command, "athena"); err != nil || got != sql {
		t.Fatalf("Athena USING command generation = %q, %v", got, err)
	}

	if expression, err := sqlglot.ParseOne(sql, "trino"); err == nil {
		t.Fatalf("standalone Trino unexpectedly gained Athena USING grammar:\n%s", expression.ToS())
	}
}

func TestAthenaUnloadIsCommand(t *testing.T) {
	const sql = "UNLOAD (SELECT name1 FROM table1) TO 's3://bucket/path/' WITH (format = 'TEXTFILE')"
	command := parseOneDialect(t, sql, "athena")
	if command.Kind() != exp.KindCommand {
		t.Fatalf("Athena UNLOAD kind = %v, want Command:\n%s", command.Kind(), command.ToS())
	}
	if got, err := generateSQL(t, command, "athena"); err != nil || got != sql {
		t.Fatalf("Athena UNLOAD generation = %q, %v", got, err)
	}
}

func TestAthenaDirectParseIntoHonorsHiveSentinel(t *testing.T) {
	const sql = "`catalog`.`table_name`"
	hiveTokens, err := dialects.Hive().NewTokenizer().Tokenize(sql)
	if err != nil {
		t.Fatalf("Hive tokenize table: %v", err)
	}
	rawTokens := append([]tokens.Token{tokens.NewToken(tokens.HIVE_TOKEN_STREAM, "")}, hiveTokens...)

	expressions, err := parserpkg.New(dialects.Athena()).ParseInto(rawTokens, sql, exp.KindTable)
	if err != nil {
		t.Fatalf("direct Athena ParseInto with Hive sentinel: %v", err)
	}
	if len(expressions) != 1 || expressions[0] == nil || expressions[0].Kind() != exp.KindTable {
		t.Fatalf("Athena ParseInto result = %#v, want one Table", expressions)
	}
	table := expressions[0]
	if table.Name() != "table_name" || table.Text("db") != "catalog" {
		t.Fatalf("Athena Hive-routed ParseInto table mismatch:\n%s", table.ToS())
	}
	if identifier := table.This(); identifier == nil || identifier.Arg("quoted") != true {
		t.Fatalf("Athena Hive-routed ParseInto lost backtick quoting:\n%s", table.ToS())
	}

	doubleSentinel := append([]tokens.Token{
		tokens.NewToken(tokens.HIVE_TOKEN_STREAM, ""),
		tokens.NewToken(tokens.HIVE_TOKEN_STREAM, ""),
	}, hiveTokens...)
	if _, err := parserpkg.New(dialects.Athena()).ParseInto(doubleSentinel, sql, exp.KindTable); err == nil {
		t.Fatal("Athena ParseInto stripped more than one HIVE_TOKEN_STREAM sentinel")
	}
}
