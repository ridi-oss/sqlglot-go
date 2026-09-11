package parser_test

import (
	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"testing"
)

func TestPrestoParserDrift(t *testing.T) {
	cases := []struct {
		sql, want  string
		kind       exp.Kind
		arg, value string
	}{
		{"SELECT LOCALTIME", "SELECT LOCALTIME", exp.KindLocaltime, "", ""},
		{"SELECT LOCALTIMESTAMP", "SELECT LOCALTIMESTAMP", exp.KindLocaltimestamp, "", ""},
		{"SELECT DATE_FORMAT(x, '%Y-%m-%d %H:%i:%S')", "SELECT DATE_FORMAT(x, '%Y-%m-%d %T')", exp.KindTimeToStr, "format", "%Y-%m-%d %H:%M:%S"},
		{"SELECT DATE_PARSE(x, '%Y-%m-%d %H:%i:%S')", "SELECT DATE_PARSE(x, '%Y-%m-%d %T')", exp.KindStrToTime, "format", "%Y-%m-%d %H:%M:%S"},
		{"SELECT DATE_FORMAT(x, '%M %c %e %h %i %s %u %k %l %T %W')", "SELECT DATE_FORMAT(x, '%M %c %e %h %i %s %u %k %l %T %W')", exp.KindTimeToStr, "format", "%B %-m %-d %I %M %S %W %-H %-I %H:%M:%S %A"},
		{"SELECT DATE_TRUNC('day', CAST(x AS DATE))", "SELECT DATE_TRUNC('DAY', CAST(x AS DATE))", exp.KindDateTrunc, "unit", "DAY"},
		{"SELECT DATE_TRUNC('Q', x)", "SELECT DATE_TRUNC('QUARTER', x)", exp.KindTimestampTrunc, "unit", "QUARTER"},
		{"SELECT DATE_TRUNC('q', x)", "SELECT DATE_TRUNC('Q', x)", exp.KindTimestampTrunc, "unit", "Q"},
		{"SELECT DATE_TRUNC(u, x)", "SELECT DATE_TRUNC('U', x)", exp.KindTimestampTrunc, "unit", "U"},
		{"SELECT DATE_FORMAT(x, f)", "SELECT DATE_FORMAT(x, f)", exp.KindTimeToStr, "", ""},
		{"SELECT DATE_FORMAT(x, '')", "SELECT DATE_FORMAT(x, 'None')", exp.KindTimeToStr, "format", "None"},
		{"SELECT DATE_TRUNC('day', x)", "SELECT DATE_TRUNC('DAY', x)", exp.KindTimestampTrunc, "unit", "DAY"},
		{"SELECT REGEXP_EXTRACT(x, '(a)')", "SELECT REGEXP_EXTRACT(x, '(a)')", exp.KindRegexpExtract, "group", "0"},
		{"SELECT REGEXP_EXTRACT(x, '(a)', 0)", "SELECT REGEXP_EXTRACT(x, '(a)')", exp.KindRegexpExtract, "group", "0"},
		{"SELECT REGEXP_EXTRACT(x, '(a)', 1)", "SELECT REGEXP_EXTRACT(x, '(a)', 1)", exp.KindRegexpExtract, "group", "1"},
		{"SELECT REGEXP_EXTRACT_ALL(x, '(a)')", "SELECT REGEXP_EXTRACT_ALL(x, '(a)')", exp.KindRegexpExtractAll, "group", "0"},
		{"SELECT SHA256(x)", "SELECT SHA256(x)", exp.KindSHA2Digest, "length", "256"},
		{"SELECT SHA512(x)", "SELECT SHA512(x)", exp.KindSHA2Digest, "length", "512"},
		{"SELECT SHA2(CAST(x AS VARCHAR), 512)", "SELECT LOWER(TO_HEX(SHA512(TO_UTF8(CAST(x AS VARCHAR)))))", exp.KindSHA2, "length", "512"},
		{"SELECT FIRST(x, 2)", "SELECT ARBITRARY(x, 2)", exp.KindFirst, "", ""},
		{"SELECT LAST(y) FROM t GROUP BY z", "SELECT ARBITRARY(y) FROM t GROUP BY z", exp.KindLast, "", ""},
		{"SELECT LOCALTIME()", "SELECT LOCALTIME", exp.KindLocaltime, "", ""},
		{"SELECT LOCALTIME(3)", "SELECT LOCALTIME(3)", exp.KindLocaltime, "this", "3"},
		{"SELECT CURRENT_TIMESTAMP(3)", "SELECT CURRENT_TIMESTAMP", exp.KindCurrentTimestamp, "this", "3"},
		{"SELECT CURRENT_USER()", "SELECT CURRENT_USER", exp.KindCurrentUser, "", ""},
	}
	for _, dialect := range []string{"presto", "trino", "athena"} {
		for _, tc := range cases {
			t.Run(dialect+"/"+tc.sql, func(t *testing.T) {
				e, err := sqlglot.ParseOne(tc.sql, dialect)
				if err != nil {
					t.Fatal(err)
				}
				nodes := e.FindAll(tc.kind)
				if len(nodes) != 1 {
					t.Fatalf("want %v: %s", tc.kind, e.ToS())
				}
				if tc.arg != "" {
					arg, ok := nodes[0].Arg(tc.arg).(exp.Expression)
					if !ok || arg.Name() != tc.value {
						t.Fatalf("%s: %s", tc.arg, nodes[0].ToS())
					}
				}
				if tc.kind == exp.KindRegexpExtract && nodes[0].Arg("null_if_pos_overflow") != true {
					t.Fatalf("missing overflow flag: %s", nodes[0].ToS())
				}
				got, err := sqlglot.Generate(e, dialect, generator.Options{})
				if err != nil {
					t.Fatal(err)
				}
				if got != tc.want {
					t.Fatalf("got %s; want %s", got, tc.want)
				}
			})
		}
		for _, tc := range []struct{ format, want string }{{"dd", "%d"}, {"hh", "%H"}, {"hh24", "%H"}, {"mi", "%i"}, {"mm", "%m"}, {"ss", "%s"}, {"yy", "%y"}, {"yyyy", "%Y"}} {
			sql := "SELECT TO_CHAR(ts, '" + tc.format + "')"
			t.Run(dialect+"/"+sql, func(t *testing.T) {
				e, err := sqlglot.ParseOne(sql, dialect)
				if err != nil {
					t.Fatal(err)
				}
				got, err := sqlglot.Generate(e, dialect, generator.Options{})
				if err != nil {
					t.Fatal(err)
				}
				want := "SELECT DATE_FORMAT(ts, '" + tc.want + "')"
				if got != want {
					t.Fatalf("got %s; want %s", got, want)
				}
			})
		}
	}
}

func TestPrestoVersionDrift(t *testing.T) {
	cases := []struct{ sql, want, alias string }{
		{"SELECT * FROM t FOR TIMESTAMP AS OF CAST('2020-01-01' AS TIMESTAMP)", "SELECT * FROM t FOR TIMESTAMP AS OF CAST('2020-01-01' AS TIMESTAMP)", ""},
		{"SELECT * FROM t FOR VERSION AS OF 123", "SELECT * FROM t FOR VERSION AS OF 123", ""},
		{"SELECT * FROM t FOR VERSION AS OF 123 AS bar", "SELECT * FROM t FOR VERSION AS OF 123 AS bar", "bar"},
		{"SELECT * FROM t FOR VERSION AS OF 123 bar", "SELECT * FROM t FOR VERSION AS OF 123 AS bar", "bar"},
		{"SELECT * FROM t FOR SYSTEM_TIME AS OF TIMESTAMP '2020-01-01'", "SELECT * FROM t FOR TIMESTAMP AS OF CAST('2020-01-01' AS TIMESTAMP)", ""},
	}
	for _, dialect := range []string{"presto", "trino", "athena"} {
		for _, tc := range cases {
			t.Run(dialect+"/"+tc.sql, func(t *testing.T) {
				e, err := sqlglot.ParseOne(tc.sql, dialect)
				if err != nil {
					t.Fatal(err)
				}
				table := e.Arg("from_").(exp.Expression).This()
				version, _ := table.Arg("version").(exp.Expression)
				if version == nil || version.Kind() != exp.KindVersion {
					t.Fatalf("missing Version on table: %s", table.ToS())
				}
				if alias, _ := table.Arg("alias").(exp.Expression); (alias == nil) != (tc.alias == "") || (alias != nil && alias.Name() != tc.alias) {
					t.Fatalf("alias: %s", table.ToS())
				}
				got, err := sqlglot.Generate(e, dialect, generator.Options{})
				if err != nil || got != tc.want {
					t.Fatalf("got %q, %v; want %q", got, err, tc.want)
				}
			})
		}
	}
}

func TestPrestoUnicodeEscapeDrift(t *testing.T) {
	for _, dialect := range []string{"presto", "trino", "athena"} {
		sql := "SELECT U&'a#0041' UESCAPE '#'"
		e, err := sqlglot.ParseOne(sql, dialect)
		if err != nil {
			t.Fatal(err)
		}
		got, err := sqlglot.Generate(e, dialect, generator.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if got != sql {
			t.Errorf("%s: got %s", dialect, got)
		}
	}
}
