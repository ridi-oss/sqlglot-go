package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// prestoProjection returns the single projection of a SELECT, failing the test if the
// statement does not have exactly one.
func prestoProjection(t *testing.T, root exp.Expression) exp.Expression {
	t.Helper()
	projections := root.Expressions()
	if len(projections) != 1 {
		t.Fatalf("want exactly one projection, got %d:\n%s", len(projections), root.ToS())
	}
	return projections[0]
}

// TestPrestoDisablesTrimParser covers parsers/presto.py:137, where Presto rebuilds
// FUNCTION_PARSERS as the base table minus TRIM. Under Presto `TRIM(x)` therefore falls
// through to an Anonymous function, while base/mysql/postgres keep the dedicated exp.Trim
// node. Mirrors dialect_parser_overrides_test.go's disabled-TRIM assertion.
func TestPrestoDisablesTrimParser(t *testing.T) {
	presto := prestoProjection(t, parseOneDialect(t, "SELECT TRIM(x)", "presto"))
	if presto.Kind() != exp.KindAnonymous || presto.Name() != "TRIM" {
		t.Fatalf("presto TRIM should be Anonymous(TRIM):\n%s", presto.ToS())
	}

	for _, dialect := range []string{"", "mysql", "postgres"} {
		got := prestoProjection(t, parseOneDialect(t, "SELECT TRIM(x)", dialect))
		if got.Kind() != exp.KindTrim {
			t.Fatalf("%q TRIM kind = %v, want Trim:\n%s", dialect, got.Kind(), got.ToS())
		}
	}
}

// TestPrestoZoneAwareTimestamp covers ZONE_AWARE_TIMESTAMP_CONSTRUCTOR (parser.py:6186-6191,
// test_presto.py:21-24): under Presto a `TIMESTAMP '<literal>'` whose literal carries a time
// zone (TIME_ZONE_RE match) is promoted from TIMESTAMP to TIMESTAMPTZ before the Cast is
// built. The promotion is gated on the Presto-only dialect flag, so base/postgres keep
// TIMESTAMP.
func TestPrestoZoneAwareTimestamp(t *testing.T) {
	const zoned = "SELECT TIMESTAMP '2025-06-20 11:22:29 Europe/Prague'"

	cast := prestoProjection(t, parseOneDialect(t, zoned, "presto"))
	if cast.Kind() != exp.KindCast {
		t.Fatalf("want Cast, got %v:\n%s", cast.Kind(), cast.ToS())
	}
	to := exprArg(t, cast, "to")
	if to.Kind() != exp.KindDataType || to.Arg("this") != exp.DTypeTimestampTz {
		t.Fatalf("zoned literal should cast to TIMESTAMPTZ:\n%s", cast.ToS())
	}
	this := exprArg(t, cast, "this")
	if this.Kind() != exp.KindLiteral || this.Name() != "2025-06-20 11:22:29 Europe/Prague" {
		t.Fatalf("cast literal mismatch:\n%s", cast.ToS())
	}

	// A literal without a zone suffix is not promoted, even under Presto.
	plain := prestoProjection(t, parseOneDialect(t, "SELECT TIMESTAMP '2025-06-20 11:22:29'", "presto"))
	plainTo := exprArg(t, plain, "to")
	if plainTo.Arg("this") != exp.DTypeTimestamp {
		t.Fatalf("non-zoned literal should stay TIMESTAMP:\n%s", plain.ToS())
	}

	// The promotion is Presto-gated: base/postgres leave the flag false, so a zoned literal
	// stays TIMESTAMP there (zero base impact).
	for _, dialect := range []string{"", "postgres"} {
		other := prestoProjection(t, parseOneDialect(t, zoned, dialect))
		otherTo := exprArg(t, other, "to")
		if otherTo.Arg("this") != exp.DTypeTimestamp {
			t.Fatalf("%q should keep TIMESTAMP (flag off):\n%s", dialect, other.ToS())
		}
	}
}

// TestPrestoUnicodeString covers the UNICODE_STRING primary path: Presto's `U&'...'` literal
// parses to an exp.UnicodeString (query.py:494) and round-trips back to `U&'...'`.
func TestPrestoUnicodeString(t *testing.T) {
	root := parseOneDialect(t, "SELECT U&'Hello'", "presto")
	us := prestoProjection(t, root)
	if us.Kind() != exp.KindUnicodeString {
		t.Fatalf("want UnicodeString, got %v:\n%s", us.Kind(), us.ToS())
	}
	if us.Name() != "Hello" {
		t.Fatalf("unicode text = %q, want %q:\n%s", us.Name(), "Hello", us.ToS())
	}

	got, err := generateSQL(t, root, "presto")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "SELECT U&'Hello'"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestPrestoRowStruct covers Presto's `ROW(...)` type spelling (ROW -> STRUCT keyword), which
// casts to a STRUCT DataType carrying one ColumnDef per named field. Only the parsed shape is
// asserted: the STRUCT -> ROW generator TYPE_MAPPING is out of this slice's scope.
func TestPrestoRowStruct(t *testing.T) {
	cast := prestoProjection(t, parseOneDialect(t, "SELECT CAST(x AS ROW(a INT))", "presto"))
	if cast.Kind() != exp.KindCast {
		t.Fatalf("want Cast, got %v:\n%s", cast.Kind(), cast.ToS())
	}
	to := exprArg(t, cast, "to")
	if to.Kind() != exp.KindDataType || to.Arg("this") != exp.DTypeStruct {
		t.Fatalf("ROW should map to STRUCT:\n%s", cast.ToS())
	}
	fields := to.Expressions()
	if len(fields) != 1 || fields[0].Kind() != exp.KindColumnDef {
		t.Fatalf("STRUCT should carry one ColumnDef field:\n%s", cast.ToS())
	}
	if name := fields[0].This(); name == nil || name.Name() != "a" {
		t.Fatalf("STRUCT field name should be a:\n%s", cast.ToS())
	}
}

// TestPrestoQualifyAsAlias covers Presto dropping QUALIFY from KEYWORDS (parsers/presto.py:69,
// test_presto.py:13): `SELECT * FROM x qualify` then reads `qualify` as a bare table alias
// rather than a QUALIFY clause, round-tripping to `SELECT * FROM x AS qualify`.
func TestPrestoQualifyAsAlias(t *testing.T) {
	root := parseOneDialect(t, "SELECT * FROM x qualify", "presto")
	from := exprArg(t, root, "from_")
	table := from.This()
	if table == nil || table.Kind() != exp.KindTable {
		t.Fatalf("FROM should be a Table:\n%s", root.ToS())
	}
	alias := exprArg(t, table, "alias")
	if alias.Kind() != exp.KindTableAlias || alias.This() == nil || alias.This().Name() != "qualify" {
		t.Fatalf("table alias should be qualify:\n%s", root.ToS())
	}

	got, err := generateSQL(t, root, "presto")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "SELECT * FROM x AS qualify"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}
