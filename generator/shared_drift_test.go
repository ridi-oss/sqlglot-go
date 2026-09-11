package generator_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Shared parser/generator seams touched by the Presto and Hive drift port (brace structs,
// ODBC `{fn ...}`, JSON typed literals, TABLESAMPLE modifiers); every dialect must match upstream.
func TestSharedParserDriftRoundTrip(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"", "SELECT {a: 1, 'b': 'c'}", "SELECT STRUCT(1 AS a, 'c' AS b)"},
		{"", "SELECT STRUCT(1 AS a)", "SELECT STRUCT(1 AS a)"},
		{"mysql", "SELECT {a: 1}", "SELECT STRUCT(1 AS a)"},
		{"postgres", "SELECT {a: 1}", "SELECT STRUCT(1 AS a)"},
		{"presto", "SELECT {a: 1}", "SELECT CAST(ROW(1) AS ROW(a INTEGER))"},
		{"hive", "SELECT {a, 1}", "SELECT STRUCT(a, 1)"},
		{"", "SELECT {fn CONCAT(a, b)}", "SELECT CONCAT(a, b)"},
		{"mysql", "SELECT {fn CONCAT(a, b)} AS x", "SELECT CONCAT(a, b) AS x"},
		{"presto", "SELECT U&'it''s'", "SELECT U&'it''s'"},
		// TYPE_LITERAL_PARSERS JSON (parser.py:1633) is global upstream; mysql renders the bare
		// value (generators/mysql.py:136), postgres a CAST, base PARSE_JSON.
		{"", "SELECT JSON '{\"a\": 1}'", "SELECT PARSE_JSON('{\"a\": 1}')"},
		{"mysql", "SELECT JSON '{\"a\": 1}'", "SELECT '{\"a\": 1}'"},
		{"postgres", "SELECT JSON '{\"a\": 1}'", "SELECT CAST('{\"a\": 1}' AS JSON)"},
		{"presto", "SELECT JSON '{\"a\": 1}'", "SELECT JSON_PARSE('{\"a\": 1}')"},
		// QUERY_MODIFIER_PARSERS TABLE_SAMPLE / USING SAMPLE (parser.py:1609-1610).
		{"", "SELECT * FROM t WHERE a = 1 TABLESAMPLE (10 PERCENT)", "SELECT * FROM t WHERE a = 1 TABLESAMPLE (10 PERCENT)"},
		{"", "SELECT * FROM t AS x USING SAMPLE 10", "SELECT * FROM t AS x TABLESAMPLE (10 ROWS)"},
		{"postgres", "SELECT * FROM t WHERE a = 1 TABLESAMPLE (10 PERCENT)", "SELECT * FROM t WHERE a = 1 TABLESAMPLE (10)"},
		{"postgres", "SELECT * FROM t JOIN u USING (a) WHERE 1 = 1", "SELECT * FROM t JOIN u USING (a) WHERE 1 = 1"},
		{"hive", "SELECT * FROM x.z y TABLESAMPLE (10 PERCENT) JOIN w ON TRUE", "SELECT * FROM x.z AS y JOIN w ON TRUE TABLESAMPLE (10 PERCENT)"},
		{"trino", "SELECT U&'Hello winter #2603 !' UESCAPE '#'", "SELECT U&'Hello winter #2603 !' UESCAPE '#'"},
	}
	for _, tc := range cases {
		t.Run(tc.dialect+"/"+tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, tc.dialect)
			if err != nil {
				t.Fatal(err)
			}
			got, err := sqlglot.Generate(e, tc.dialect, generator.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %s; want %s", got, tc.want)
			}
		})
	}
}
