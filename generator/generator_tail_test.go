package generator_test

// Generator-side round-trip mop-up cases (window EXCLUDE, SELECT INTO UNLOGGED, the
// postgres ARRAY[...] literal's pretty line-wrap, mysql control-character string escaping,
// and postgres's ARRAY_LENGTH dimension argument). See interval_test.go for the
// INTERVAL/DATE_ADD-shaped cases in the same cluster. Every case is drawn from
// testdata/dialect_identity.jsonl / testdata/parity_gaps.txt and confirmed against the
// pinned oracle (comments below cite the specific invocation).

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
)

// prettyRoundTrip is roundTrip (generator_test.go) with Pretty:true, needed by the
// ARRAY[...] line-wrap case below (the wrap only triggers in pretty mode).
func prettyRoundTrip(t *testing.T, dialect, sql string) string {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q, %q) error: %v", sql, dialect, err)
	}
	generated, err := sqlglot.Generate(expression, dialect, generator.Options{Pretty: true})
	if err != nil {
		t.Fatalf("Generate(%q, %q, pretty) error: %v", sql, dialect, err)
	}
	return generated
}

// TestWindowSpecExcludePostgres covers windowSpecSQL's EXCLUDE rendering
// (generator/sql.go), gated on SUPPORTS_WINDOW_EXCLUDE (generator.py:512,
// generators/postgres.py:250: true for postgres, false/unsupported-warning for base and
// mysql). Confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.parse_one(
//	  'select count() OVER(partition by a order by a range offset preceding exclude current row)',
//	  read='postgres').sql(dialect='postgres'))"
//	SELECT COUNT() OVER (PARTITION BY a ORDER BY a range BETWEEN offset preceding AND CURRENT ROW EXCLUDE CURRENT ROW)
func TestWindowSpecExcludePostgres(t *testing.T) {
	sql := "select count() OVER(partition by a order by a range offset preceding exclude current row)"
	want := "SELECT COUNT() OVER (PARTITION BY a ORDER BY a range BETWEEN offset preceding AND CURRENT ROW EXCLUDE CURRENT ROW)"
	if got := roundTrip(t, "postgres", sql); got != want {
		t.Errorf("postgres window EXCLUDE ->\n  got  %q\n  want %q", got, want)
	}
}

// TestWindowSpecExcludeBaseUnsupported covers the other half of the same branch: base (and
// mysql) drop EXCLUDE with an unsupported warning rather than rendering it, matching
// upstream's SUPPORTS_WINDOW_EXCLUDE=False default. Confirmed against the pinned oracle:
//
//	>>> sqlglot.parse_one(
//	...   'select count() OVER(partition by a order by a range offset preceding exclude current row)'
//	... ).sql()
//	'SELECT COUNT() OVER (PARTITION BY a ORDER BY a range BETWEEN offset preceding AND CURRENT ROW)'
func TestWindowSpecExcludeBaseUnsupported(t *testing.T) {
	sql := "select count() OVER(partition by a order by a range offset preceding exclude current row)"
	want := "SELECT COUNT() OVER (PARTITION BY a ORDER BY a range BETWEEN offset preceding AND CURRENT ROW)"
	if got := roundTrip(t, "", sql); got != want {
		t.Errorf("base window EXCLUDE (dropped) ->\n  got  %q\n  want %q", got, want)
	}
}

// TestIntoUnlogged covers the new intoSQL dispatch (generator/into.go): KindInto had no
// generator entry at all before (any query with an INTO clause panicked with "Unsupported
// expression type Into"). Confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.parse_one(
//	  \"WITH t(c) AS (SELECT 1) SELECT * INTO UNLOGGED foo FROM (SELECT c AS c FROM t) AS temp\",
//	  read='postgres').sql(dialect='postgres'))"
//	WITH t(c) AS (SELECT 1) SELECT * INTO UNLOGGED foo FROM (SELECT c AS c FROM t) AS temp
func TestIntoUnlogged(t *testing.T) {
	sql := "WITH t(c) AS (SELECT 1) SELECT * INTO UNLOGGED foo FROM (SELECT c AS c FROM t) AS temp"
	if got := roundTrip(t, "postgres", sql); got != sql {
		t.Errorf("postgres INTO UNLOGGED ->\n  got  %q\n  want %q", got, sql)
	}
}

// TestArrayLiteralPrettyWrap covers arraySQL's postgres ARRAY[...] line-wrap
// (generator/residual_tail.go arraySQL -> inlineArraySQL): a wide array literal wraps one
// element per line in pretty mode, like upstream's inline_array_sql (dialects/dialect.py:
// 1218-1219), instead of a flat comma-join. Confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.parse_one(
//	  'ARRAY[x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x]',
//	  read='postgres').sql(dialect='postgres', pretty=True))"
func TestArrayLiteralPrettyWrap(t *testing.T) {
	sql := "ARRAY[x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x,x]"
	want := "ARRAY[\n" +
		"  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n" +
		"  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x,\n  x\n]"
	if got := prettyRoundTrip(t, "postgres", sql); got != want {
		t.Errorf("postgres ARRAY[...] pretty wrap ->\n  got  %q\n  want %q", got, want)
	}
}

// TestArrayLiteralFlatUnaffected guards arraySQL against regressing the many already-passing
// ARRAY[...] cases that don't need wrapping (few elements, or non-pretty output): they must
// render as a flat `ARRAY[a, b, c]` whenever pretty line-wrap wouldn't kick in anyway.
func TestArrayLiteralFlatUnaffected(t *testing.T) {
	cases := []string{
		"SELECT ARRAY[1, 2, 3]",
		"WIDTH_BUCKET(10, ARRAY[5, 15])",
	}
	for _, sql := range cases {
		if got := roundTrip(t, "postgres", sql); got != sql {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", sql, got, sql)
		}
	}
}

// TestArraySizeDimRequiredPostgres covers arraySizeSQL's ARRAY_SIZE_DIM_REQUIRED handling
// (generator/sql.go): postgres keeps the dimension argument to ARRAY_LENGTH (defaulting to
// 1 when omitted), unlike base which drops a "1" dimension and warns on anything else.
// Confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.parse_one('SELECT ARRAY_LENGTH(ARRAY[1, 2, 3], 1)',
//	  read='postgres').sql(dialect='postgres'))"
//	SELECT ARRAY_LENGTH(ARRAY[1, 2, 3], 1)
func TestArraySizeDimRequiredPostgres(t *testing.T) {
	sql := "SELECT ARRAY_LENGTH(ARRAY[1, 2, 3], 1)"
	if got := roundTrip(t, "postgres", sql); got != sql {
		t.Errorf("postgres ARRAY_LENGTH dim ->\n  got  %q\n  want %q", got, sql)
	}
}

// TestArraySizeDimDroppedBase guards the base branch of the same code (dim=1 still drops
// to the 1-arg form), confirming the postgres-only gate didn't change base's behavior. Base
// has no array_sql override upstream (only postgres does, generators/postgres.py:502-509),
// so a base exp.Array renders via the generic function-fallback paren form, not bracket
// notation - confirmed against the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.parse_one('SELECT ARRAY_LENGTH(ARRAY[1, 2, 3], 1)',
//	  read='').sql())"
//	SELECT ARRAY_LENGTH(ARRAY(1, 2, 3))
func TestArraySizeDimDroppedBase(t *testing.T) {
	got := roundTrip(t, "", "SELECT ARRAY_LENGTH(ARRAY[1, 2, 3], 1)")
	want := "SELECT ARRAY_LENGTH(ARRAY(1, 2, 3))"
	if got != want {
		t.Errorf("base ARRAY_LENGTH dim ->\n  got  %q\n  want %q", got, want)
	}
}

// TestMySQLEscapeControlChars covers escapeStr's control-character escaping
// (generator/sql.go escapedSequences): a literal control byte (e.g. a raw tab) in a mysql
// string round-trips as its backslash-letter escape sequence on output, matching upstream's
// ESCAPED_SEQUENCES table (dialects/dialect.py:66-90, 302-312). Confirmed against the
// pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.parse_one('\'\t\'', read='mysql').sql(dialect='mysql'))"
//	'\t'
func TestMySQLEscapeControlChars(t *testing.T) {
	sql := "'" + "\t" + "'"
	want := `'\t'`
	if got := roundTrip(t, "mysql", sql); got != want {
		t.Errorf("mysql control-char escape ->\n  got  %q\n  want %q", got, want)
	}
}
