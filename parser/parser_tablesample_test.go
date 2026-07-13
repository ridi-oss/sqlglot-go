package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestTableSample ports the base-dialect TABLESAMPLE identity cases (identity.sql:343-348)
// and postgres's TABLESAMPLE ... REPEATABLE case (test_postgres.py:43), checking both the
// parsed exp.TableSample args and the round-trip SQL.
func TestTableSample(t *testing.T) {
	t.Run("bucket", func(t *testing.T) {
		sql := "SELECT a FROM test TABLESAMPLE (BUCKET 1 OUT OF 5)"
		selectExpr := parseOneDialect(t, sql, "")
		table := exprArg(t, selectExpr, "from_").This()
		sample := exprArg(t, table, "sample")
		if sample.Kind() != exp.KindTableSample {
			t.Fatalf("kind = %v, want TableSample:\n%s", sample.Kind(), selectExpr.ToS())
		}
		if got := exprArg(t, sample, "bucket_numerator").Name(); got != "1" {
			t.Fatalf("bucket_numerator = %q, want 1:\n%s", got, selectExpr.ToS())
		}
		if got := exprArg(t, sample, "bucket_denominator").Name(); got != "5" {
			t.Fatalf("bucket_denominator = %q, want 5:\n%s", got, selectExpr.ToS())
		}
		if sample.Arg("bucket_field") != nil {
			t.Fatalf("bucket_field should be unset:\n%s", selectExpr.ToS())
		}
		got, err := generateSQL(t, selectExpr, "")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	})

	t.Run("bucket on column", func(t *testing.T) {
		sql := "SELECT a FROM test TABLESAMPLE (BUCKET 1 OUT OF 5 ON x)"
		selectExpr := parseOneDialect(t, sql, "")
		table := exprArg(t, selectExpr, "from_").This()
		sample := exprArg(t, table, "sample")
		// Matches upstream: _parse_field falls through to a bare Identifier for an unquoted
		// name that's neither a primary literal nor a function call.
		field := exprArg(t, sample, "bucket_field")
		if field.Kind() != exp.KindIdentifier || field.Name() != "x" {
			t.Fatalf("bucket_field kind = %v, want Identifier(x):\n%s", field.Kind(), selectExpr.ToS())
		}
		got, err := generateSQL(t, selectExpr, "")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	})

	t.Run("bucket on function", func(t *testing.T) {
		sql := "SELECT a FROM test TABLESAMPLE (BUCKET 1 OUT OF 5 ON RAND())"
		selectExpr := parseOneDialect(t, sql, "")
		table := exprArg(t, selectExpr, "from_").This()
		sample := exprArg(t, table, "sample")
		field := exprArg(t, sample, "bucket_field")
		if field.Kind() != exp.KindAnonymous || field.Name() != "RAND" {
			t.Fatalf("bucket_field should be Anonymous(RAND):\n%s", selectExpr.ToS())
		}
		got, err := generateSQL(t, selectExpr, "")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	})

	t.Run("percent", func(t *testing.T) {
		sql := "SELECT a FROM test TABLESAMPLE (0.1 PERCENT)"
		selectExpr := parseOneDialect(t, sql, "")
		table := exprArg(t, selectExpr, "from_").This()
		sample := exprArg(t, table, "sample")
		if got := exprArg(t, sample, "percent").Name(); got != "0.1" {
			t.Fatalf("percent = %q, want 0.1:\n%s", got, selectExpr.ToS())
		}
		if sample.Arg("size") != nil {
			t.Fatalf("size should be unset:\n%s", selectExpr.ToS())
		}
		got, err := generateSQL(t, selectExpr, "")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	})

	t.Run("rows", func(t *testing.T) {
		sql := "SELECT a FROM test TABLESAMPLE (100 ROWS)"
		selectExpr := parseOneDialect(t, sql, "")
		table := exprArg(t, selectExpr, "from_").This()
		sample := exprArg(t, table, "sample")
		if got := exprArg(t, sample, "size").Name(); got != "100" {
			t.Fatalf("size = %q, want 100:\n%s", got, selectExpr.ToS())
		}
		if sample.Arg("percent") != nil {
			t.Fatalf("percent should be unset:\n%s", selectExpr.ToS())
		}
		got, err := generateSQL(t, selectExpr, "")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	})

	// _parse_subquery (parser.py:4132-4146) attaches a trailing TABLESAMPLE to the Subquery
	// itself, so a derived-table source samples too. base keeps the ROWS keyword; postgres
	// (TABLESAMPLE_SIZE_IS_ROWS=False) drops it — both verified against sqlglot==30.12.0.
	t.Run("subquery source", func(t *testing.T) {
		sql := "SELECT * FROM (SELECT * FROM t) TABLESAMPLE (100 ROWS)"
		selectExpr := parseOneDialect(t, sql, "")
		subquery := exprArg(t, selectExpr, "from_").This()
		if subquery.Kind() != exp.KindSubquery {
			t.Fatalf("from source kind = %v, want Subquery:\n%s", subquery.Kind(), selectExpr.ToS())
		}
		sample := exprArg(t, subquery, "sample")
		if sample.Kind() != exp.KindTableSample {
			t.Fatalf("sample kind = %v, want TableSample:\n%s", sample.Kind(), selectExpr.ToS())
		}
		if got := exprArg(t, sample, "size").Name(); got != "100" {
			t.Fatalf("size = %q, want 100:\n%s", got, selectExpr.ToS())
		}
		got, err := generateSQL(t, selectExpr, "")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	})

	// test_postgres.py:43: SYSTEM (50) parses as percent (postgres TABLESAMPLE_SIZE_IS_PERCENT
	// = True), and REPEATABLE (55) is postgres's seed keyword.
	t.Run("postgres system repeatable", func(t *testing.T) {
		sql := "SELECT * FROM t TABLESAMPLE SYSTEM (50) REPEATABLE (55)"
		selectExpr := parseOneDialect(t, sql, "postgres")
		table := exprArg(t, selectExpr, "from_").This()
		sample := exprArg(t, table, "sample")
		if sample.Kind() != exp.KindTableSample {
			t.Fatalf("kind = %v, want TableSample:\n%s", sample.Kind(), selectExpr.ToS())
		}
		if got := exprArg(t, sample, "method").Name(); got != "SYSTEM" {
			t.Fatalf("method = %q, want SYSTEM:\n%s", got, selectExpr.ToS())
		}
		if got := exprArg(t, sample, "percent").Name(); got != "50" {
			t.Fatalf("percent = %q, want 50:\n%s", got, selectExpr.ToS())
		}
		if sample.Arg("size") != nil {
			t.Fatalf("size should be unset (postgres percent-by-default):\n%s", selectExpr.ToS())
		}
		if got := exprArg(t, sample, "seed").Name(); got != "55" {
			t.Fatalf("seed = %q, want 55:\n%s", got, selectExpr.ToS())
		}
		got, err := generateSQL(t, selectExpr, "postgres")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	})
}

// TestTableSampleAfterPivotedSubquery guards the parseTable subquery branch: a TABLESAMPLE
// trailing a PIVOT over a derived table must still attach as the subquery's `sample`.
// parseSubquery only gets to parse `sample` before pivots, so with a PIVOT in between the
// sample is parsed after pivots in the subquery branch instead (parser.py:4141-4143). This
// is an identity round-trip in sqlglot 30.12.0's default dialect.
func TestTableSampleAfterPivotedSubquery(t *testing.T) {
	sql := "SELECT * FROM (SELECT * FROM t) PIVOT(SUM(a) FOR b IN ('c')) TABLESAMPLE (1 ROWS)"
	root := parseOne(t, sql)
	subquery := exprArg(t, root, "from_").This()
	sample := exprArg(t, subquery, "sample")
	if sample.Kind() != exp.KindTableSample {
		t.Fatalf("subquery sample kind = %v, want TableSample:\n%s", sample.Kind(), root.ToS())
	}
	got, err := generateSQL(t, root, "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}
