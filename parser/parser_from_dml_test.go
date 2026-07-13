package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestParseJoinNestedFallback covers the nested-join ON/USING fallback ported into
// parseJoin from _parse_join (parser.py:4520-4541): a trailing ON/USING that doesn't
// immediately follow a join's own `this` may belong to an *outer* join wrapping further
// joins nested on that `this` instead.
func TestParseJoinNestedFallback(t *testing.T) {
	sel := parseOne(t, "SELECT * FROM a JOIN b JOIN c USING (id) USING (id)")
	sql, err := generateSQL(t, sel, "")
	if err != nil || sql != "SELECT * FROM a JOIN b JOIN c USING (id) USING (id)" {
		t.Fatalf("USING..USING round-trip mismatch: sql=%q err=%v", sql, err)
	}
	joins := expressionsForArg(sel, "joins")
	if len(joins) != 1 {
		t.Fatalf("outer joins len = %d, want 1:\n%s", len(joins), sel.ToS())
	}
	outer := joins[0]
	if outerUsing := expressionsForArg(outer, "using"); len(outerUsing) != 1 {
		t.Fatalf("outer join USING mismatch:\n%s", sel.ToS())
	}
	innerTable := exprArg(t, outer, "this")
	nested := expressionsForArg(innerTable, "joins")
	if len(nested) != 1 {
		t.Fatalf("nested joins len = %d, want 1:\n%s", len(nested), sel.ToS())
	}
	if innerUsing := expressionsForArg(nested[0], "using"); len(innerUsing) != 1 {
		t.Fatalf("nested join USING mismatch:\n%s", sel.ToS())
	}

	sel = parseOne(t, "SELECT 1 FROM a JOIN b JOIN c ON b.id = c.id ON a.id = b.id")
	sql, err = generateSQL(t, sel, "")
	if err != nil || sql != "SELECT 1 FROM a JOIN b JOIN c ON b.id = c.id ON a.id = b.id" {
		t.Fatalf("ON..ON round-trip mismatch: sql=%q err=%v", sql, err)
	}

	// A join-starting keyword must never be swallowed as a bare alias (parseTable's
	// compensating fast-path-terminator check).
	sel = parseOne(t, "SELECT * FROM a STRAIGHT_JOIN b")
	sql, err = generateSQL(t, sel, "")
	if err != nil || sql != "SELECT * FROM a STRAIGHT_JOIN b" {
		t.Fatalf("STRAIGHT_JOIN round-trip mismatch: sql=%q err=%v", sql, err)
	}
}

// TestParseTablePartition covers the Hive/Spark PARTITION(...) table-partition selector
// (parseTable, ported from parser.py:4893-4895), reached via parseInsertTable's
// parsePartition=true.
func TestParseTablePartition(t *testing.T) {
	insert := parseOne(t, "INSERT OVERWRITE TABLE a.b PARTITION(ds = 'YYYY-MM-DD', hour = 'hh') SELECT x FROM y")
	table := exprArg(t, insert, "this")
	partition := exprArg(t, table, "partition")
	if partition.Kind() != exp.KindPartition || len(partition.Expressions()) != 2 {
		t.Fatalf("partition mismatch:\n%s", insert.ToS())
	}
	sql, err := generateSQL(t, insert, "")
	want := "INSERT OVERWRITE TABLE a.b PARTITION(ds = 'YYYY-MM-DD', hour = 'hh') SELECT x FROM y"
	if err != nil || sql != want {
		t.Fatalf("round-trip mismatch: sql=%q err=%v", sql, err)
	}

	insert = parseOne(t, "INSERT INTO a.b PARTITION(DAY = '2024-04-14') (col1, col2) SELECT x FROM y")
	schema := exprArg(t, insert, "this")
	if schema.Kind() != exp.KindSchema {
		t.Fatalf("expected Schema wrapping the partitioned table:\n%s", insert.ToS())
	}
	table = exprArg(t, schema, "this")
	if table.Arg("partition") == nil {
		t.Fatalf("expected partition on the inner Table:\n%s", insert.ToS())
	}
	sql, err = generateSQL(t, insert, "")
	want = "INSERT INTO a.b PARTITION(DAY = '2024-04-14') (col1, col2) SELECT x FROM y"
	if err != nil || sql != want {
		t.Fatalf("round-trip mismatch: sql=%q err=%v", sql, err)
	}
}

// TestParseInsertDirectory covers the Hive/Spark `INSERT OVERWRITE [LOCAL] DIRECTORY
// '<path>' [ROW FORMAT ...]` form (parseInsert's DIRECTORY branch, exp.Directory).
func TestParseInsertDirectory(t *testing.T) {
	insert := parseOne(t, "INSERT OVERWRITE LOCAL DIRECTORY 'x' SELECT 1")
	dir := exprArg(t, insert, "this")
	if dir.Kind() != exp.KindDirectory || dir.Arg("local") != true {
		t.Fatalf("directory mismatch:\n%s", insert.ToS())
	}

	insert = parseOne(t, "INSERT OVERWRITE LOCAL DIRECTORY 'x' ROW FORMAT DELIMITED FIELDS TERMINATED BY '1' "+
		"COLLECTION ITEMS TERMINATED BY '2' MAP KEYS TERMINATED BY '3' LINES TERMINATED BY '4' NULL DEFINED AS '5' SELECT 1")
	dir = exprArg(t, insert, "this")
	rowFormat := exprArg(t, dir, "row_format")
	if rowFormat.Kind() != exp.KindRowFormatDelimitedProperty {
		t.Fatalf("row format mismatch:\n%s", insert.ToS())
	}
	if rowFormat.Arg("fields") == nil || rowFormat.Arg("collection_items") == nil ||
		rowFormat.Arg("map_keys") == nil || rowFormat.Arg("lines") == nil || rowFormat.Arg("null") == nil {
		t.Fatalf("row format clauses incomplete:\n%s", insert.ToS())
	}
	rowFormatSQL, err := generateSQL(t, rowFormat, "")
	want := "ROW FORMAT DELIMITED FIELDS TERMINATED BY '1' COLLECTION ITEMS TERMINATED BY '2' " +
		"MAP KEYS TERMINATED BY '3' LINES TERMINATED BY '4' NULL DEFINED AS '5'"
	if err != nil || rowFormatSQL != want {
		t.Fatalf("row format round-trip mismatch: sql=%q err=%v", rowFormatSQL, err)
	}
	// NOTE: full-statement generation isn't asserted here for the overwrite+Directory
	// case: generator/sql.go's insertSQL still unconditionally renders overwrite=true as
	// " OVERWRITE TABLE"; upstream (insert_sql, generator.py:2265-2268) special-cases
	// isinstance(this, exp.Directory) to render " OVERWRITE" (no TABLE). That one-line
	// generator fix is outside this part's file scope (generator/sql.go) - see the
	// integrator report for the exact diff needed for the full round-trip to pass.
}

// TestParseTableHintsMySQL covers MySQL USE/FORCE/IGNORE INDEX table hints
// (parseTableHints, ported from _parse_table_hints, parser.py:4636-4662), and the
// compensating alias-guard that keeps FORCE/IGNORE/USE from being swallowed as a bare
// table alias under the mysql dialect (parsers/mysql.py:83-85).
func TestParseTableHintsMySQL(t *testing.T) {
	sel := parseOneDialect(t, "SELECT * FROM t1 USE INDEX (i1) IGNORE INDEX FOR ORDER BY (i2) ORDER BY a", "mysql")
	from := exprArg(t, sel, "from_")
	table := exprArg(t, from, "this")
	hints := expressionsForArg(table, "hints")
	if len(hints) != 2 {
		t.Fatalf("hints len = %d, want 2:\n%s", len(hints), sel.ToS())
	}
	if hints[0].Kind() != exp.KindIndexTableHint || hints[0].Arg("this") != "USE" {
		t.Fatalf("first hint mismatch:\n%s", sel.ToS())
	}
	if hints[1].Arg("target") != "ORDER BY" {
		t.Fatalf("second hint target mismatch:\n%s", sel.ToS())
	}
	sql, err := generateSQL(t, sel, "mysql")
	want := "SELECT * FROM t1 USE INDEX (i1) IGNORE INDEX FOR ORDER BY (i2) ORDER BY a"
	if err != nil || sql != want {
		t.Fatalf("round-trip mismatch: sql=%q err=%v", sql, err)
	}
}

// TestParseRowsFromPostgres covers Postgres's multi-function `ROWS FROM (...)` table
// source (parseTable, ported from parser.py:4867-4874).
func TestParseRowsFromPostgres(t *testing.T) {
	sel := parseOneDialect(t, "SELECT * FROM ROWS FROM (FUNC1(col1, col2))", "postgres")
	from := exprArg(t, sel, "from_")
	table := exprArg(t, from, "this")
	rowsFrom := expressionsForArg(table, "rows_from")
	if len(rowsFrom) != 1 {
		t.Fatalf("rows_from len = %d, want 1:\n%s", len(rowsFrom), sel.ToS())
	}
	sql, err := generateSQL(t, sel, "postgres")
	want := "SELECT * FROM ROWS FROM (FUNC1(col1, col2))"
	if err != nil || sql != want {
		t.Fatalf("round-trip mismatch: sql=%q err=%v", sql, err)
	}
}

// TestParseInsertUpdateDeleteHintArg is a regression check for the new p.parseHint()
// call sites added to parseInsert/parseUpdate/parseDelete (consuming the "hints" part's
// func (p *Parser) parseHint() exp.Expression): hint-less statements must keep parsing
// exactly as before, with "hint" staying nil.
func TestParseInsertUpdateDeleteHintArg(t *testing.T) {
	insert := parseOne(t, "INSERT INTO t VALUES (1)")
	if insert.Arg("hint") != nil {
		t.Fatalf("unexpected hint on hint-less INSERT:\n%s", insert.ToS())
	}
	update := parseOne(t, "UPDATE t SET a = 1")
	if update.Arg("hint") != nil {
		t.Fatalf("unexpected hint on hint-less UPDATE:\n%s", update.ToS())
	}
	deleteExpr := parseOne(t, "DELETE FROM t")
	if deleteExpr.Arg("hint") != nil {
		t.Fatalf("unexpected hint on hint-less DELETE:\n%s", deleteExpr.ToS())
	}
}
