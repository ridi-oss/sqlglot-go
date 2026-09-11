package parser_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	sqlerrors "github.com/ridi-oss/sqlglot-go/errors"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	parserpkg "github.com/ridi-oss/sqlglot-go/parser"
	"github.com/ridi-oss/sqlglot-go/tokens"
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

func TestAthenaUnloadIsStructured(t *testing.T) {
	const sql = "UNLOAD (SELECT name1 FROM table1) TO 's3://bucket/path/' WITH (format = 'TEXTFILE')"
	command := parseOneDialect(t, sql, "athena")
	if command.Kind() != exp.KindUnload || command.Find(exp.KindSelect) == nil {
		t.Fatalf("Athena UNLOAD must contain a structured query:\n%s", command.ToS())
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
	if table.Name() != "table_name" || table.Text("schema") != "catalog" {
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

func TestAthenaBatchAccumulatesErrors(t *testing.T) {
	const sql = "SELECT ( ; SELECT ("
	d := dialects.Athena()
	raw, err := d.NewTokenizer().Tokenize(sql)
	if err != nil {
		t.Fatal(err)
	}
	for _, level := range []sqlerrors.ErrorLevel{sqlerrors.RAISE, sqlerrors.WARN, sqlerrors.IGNORE} {
		p := parserpkg.NewWithErrorLevel(d, level)
		_, err := p.Parse(raw, sql)
		if (err != nil) != (level == sqlerrors.RAISE) {
			t.Fatalf("%v: unexpected error %v", level, err)
		}
		first, second := false, false
		for _, e := range p.Errors() {
			for _, location := range e.Errors {
				col, _ := location["col"].(int)
				first = first || col < 10
				second = second || col > 10
			}
		}
		// RAISE stops at the first failing statement, like the base parser; WARN/IGNORE keep going.
		if !first || second != (level != sqlerrors.RAISE) {
			t.Fatalf("%v: unexpected error chunks (first=%v second=%v): %v", level, first, second, p.Errors())
		}
		valid, err := d.NewTokenizer().Tokenize("SELECT 1")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Parse(valid, "SELECT 1"); err != nil || len(p.Errors()) != 0 {
			t.Fatalf("parser reuse retained errors: %v, %v", err, p.Errors())
		}
	}
}

func TestHiveRenamePartitionStaysUpstream(t *testing.T) {
	const sql = "ALTER TABLE t PARTITION (ds = 'old') RENAME TO PARTITION (ds = 'new')"
	raw, err := dialects.Hive().NewTokenizer().Tokenize(sql)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parserpkg.NewWithErrorLevel(dialects.Hive(), sqlerrors.RAISE).Parse(raw, sql); err == nil {
		t.Fatal("standalone hive must keep upstream's parse error for RENAME TO PARTITION")
	}
	showRaw, err := dialects.Hive().NewTokenizer().Tokenize("SHOW TABLES")
	if err != nil {
		t.Fatal(err)
	}
	e, err := parserpkg.NewWithErrorLevel(dialects.Hive(), sqlerrors.RAISE).Parse(showRaw, "SHOW TABLES")
	if err != nil || len(e) != 1 || e[0].Kind() != exp.KindCommand {
		t.Fatalf("standalone hive SHOW must stay a Command: %v %v", e, err)
	}
}

func TestAthenaParseIntoMixedBatch(t *testing.T) {
	const sql = "`first`; \"second\""
	raw, err := dialects.Hive().NewTokenizer().Tokenize(sql)
	if err != nil {
		t.Fatal(err)
	}
	raw[2].TokenType = tokens.IDENTIFIER
	raw = append([]tokens.Token{tokens.NewToken(tokens.HIVE_TOKEN_STREAM, "")}, raw...)
	expressions, err := parserpkg.New(dialects.Athena()).ParseInto(raw, sql, exp.KindTable)
	if err != nil || len(expressions) != 2 {
		t.Fatalf("ParseInto = %v, %v", expressions, err)
	}
	for i, want := range []string{"`first`", `"second"`} {
		text, ok := expressions[i].SpanText()
		if !ok || text != want || expressions[i].Kind() != exp.KindTable || expressions[i].This().Arg("quoted") != true {
			t.Fatalf("statement %d = %s, span %q", i, expressions[i].ToS(), text)
		}
	}
}

func TestAthenaHiveRoutePreservesOpaqueFunctions(t *testing.T) {
	e := parseOneDialect(t, "CREATE TABLE t (x VARCHAR DEFAULT SUBSTR('abc', 1, 2))", "athena, opaque_functions=true")
	if e.Kind() != exp.KindCreate || e.Find(exp.KindAnonymous) == nil || e.Find(exp.KindSubstring) != nil {
		t.Fatalf("Hive route lost opaque_functions: %s", e.ToS())
	}
}
