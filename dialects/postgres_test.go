package dialects_test

import (
	"testing"

	sqlglot "github.com/sjincho/sqlglot-go"
	"github.com/sjincho/sqlglot-go/dialects"
	"github.com/sjincho/sqlglot-go/generator"
	"github.com/sjincho/sqlglot-go/optimizer"
	"github.com/sjincho/sqlglot-go/schema"
	"github.com/sjincho/sqlglot-go/tokens"
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

func TestPostgresIdentityRoundTrips(t *testing.T) {
	cases := []identityCase{
		{name: "quoted identifiers", dialect: "postgres", sql: "SELECT \"Foo\" FROM \"Bar\""},
		{name: "generic select", dialect: "postgres", sql: "SELECT a, b FROM t WHERE a = 1"},
		{name: "cte", dialect: "postgres", sql: "WITH x AS (SELECT 1 AS a) SELECT a FROM x"},
		{name: "union", dialect: "postgres", sql: "SELECT a FROM x UNION SELECT b FROM y"},
		{name: "intersect", dialect: "postgres", sql: "SELECT a FROM x INTERSECT SELECT a FROM y"},
		{name: "except", dialect: "postgres", sql: "SELECT a FROM x EXCEPT SELECT a FROM y"},
		{name: "lateral unnest", dialect: "postgres", sql: "SELECT * FROM r CROSS JOIN LATERAL UNNEST(ARRAY[1]) AS s(location)"},
		{name: "dcolon cast", dialect: "postgres", sql: "SELECT x::INT", want: "SELECT CAST(x AS INT)"},
		{name: "int4range cast", dialect: "postgres", sql: "CAST(x AS INT4RANGE)"},
		{name: "int4multirange cast", dialect: "postgres", sql: "CAST(x AS INT4MULTIRANGE)"},
		{name: "int8range cast", dialect: "postgres", sql: "CAST(x AS INT8RANGE)"},
		{name: "int8multirange cast", dialect: "postgres", sql: "CAST(x AS INT8MULTIRANGE)"},
		{name: "numrange cast", dialect: "postgres", sql: "CAST(x AS NUMRANGE)"},
		{name: "nummultirange cast", dialect: "postgres", sql: "CAST(x AS NUMMULTIRANGE)"},
		{name: "tsrange cast", dialect: "postgres", sql: "CAST(x AS TSRANGE)"},
		{name: "tsmultirange cast", dialect: "postgres", sql: "CAST(x AS TSMULTIRANGE)"},
		{name: "tstzrange cast", dialect: "postgres", sql: "CAST(x AS TSTZRANGE)"},
		{name: "tstzmultirange cast", dialect: "postgres", sql: "CAST(x AS TSTZMULTIRANGE)"},
		{name: "daterange cast", dialect: "postgres", sql: "CAST(x AS DATERANGE)"},
		{name: "datemultirange cast", dialect: "postgres", sql: "CAST(x AS DATEMULTIRANGE)"},
		{name: "byte literal", deferredReason: "byte literal parser and generator rendering — slice 5b", category: "param/literal rendering"},
		{name: "dollar heredoc literal", deferredReason: "heredoc literal parser and generator rendering — slice 5b", category: "param/literal rendering"},
		{name: "positional parameter", deferredReason: "Postgres parameter rendering — slice 5b", category: "param/literal rendering"},
		{name: "json path operator", deferredReason: "Postgres operator parser/generator wiring — slice 5b", category: "parser operator"},
		{name: "only table source", dialect: "postgres", sql: "SELECT * FROM ONLY t1"},
		{name: "similar to escape", dialect: "postgres", sql: "SELECT '%' SIMILAR TO '^%' ESCAPE '^'"},
		{name: "interval phrase", deferredReason: "Postgres interval rendering override — slice 5b", category: "generator TRANSFORM/TYPE_MAPPING"},
		{name: "json_to_recordset", dialect: "postgres", sql: `SELECT * FROM JSON_TO_RECORDSET(z) AS y("rank" INT)`},
		// Keyword-like alias column: the alias column list uses parseIdVar(any_token=true),
		// mirroring _parse_function_parameter -> _parse_id_var() (parser.py:7111-7112,8507).
		{name: "json_to_recordset keyword column", dialect: "postgres", sql: `SELECT * FROM JSON_TO_RECORDSET(z) AS y(from INT)`},
		// Table-valued function sources (test_postgres.py:742-775): a function call is a
		// valid FROM/JOIN source, becoming exp.Table's "this".
		{name: "generate_series source", dialect: "postgres", sql: "SELECT * FROM GENERATE_SERIES(a, b)"},
		{name: "generate_series cross join", dialect: "postgres", sql: "SELECT * FROM t CROSS JOIN GENERATE_SERIES(2, 4)"},
		{name: "generate_series cross join alias", dialect: "postgres", sql: "SELECT * FROM t CROSS JOIN GENERATE_SERIES(2, 4) AS s"},
		// WITH ORDINALITY on a function source (test_postgres.py:143-151); the alias
		// (if any) binds after the keyword.
		{name: "with ordinality", dialect: "postgres", sql: "SELECT * FROM JSON_ARRAY_ELEMENTS('[1,true, [2,false]]') WITH ORDINALITY"},
		{name: "with ordinality alias", dialect: "postgres", sql: "SELECT * FROM JSON_ARRAY_ELEMENTS('[1,true, [2,false]]') WITH ORDINALITY AS kv_json"},
		{name: "with ordinality alias columns", dialect: "postgres", sql: "SELECT * FROM JSON_ARRAY_ELEMENTS('[1,true, [2,false]]') WITH ORDINALITY AS kv_json(a, b)"},
	}
	runIdentityCases(t, "test_postgres validate_identity", cases)
}

func TestPostgresValidateAllKeys(t *testing.T) {
	cases := []identityCase{
		{name: "dcolon cast postgres key", dialect: "postgres", sql: "SELECT x::INT", want: "SELECT CAST(x AS INT)"},
		{name: "dcolon cast spark key", deferredReason: "cross-dialect: needs spark", category: "cross-dialect"},
	}
	runIdentityCases(t, "test_postgres validate_all in-scope keys", cases)
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
