package parser_test

// TestParseSlice/TestParseNational port the AST shapes checked by the pinned oracle for
// the slice-strings cluster (parseSlice/parseBracketKeyValue/parseString's National
// branch, parser/parser_types.go), closing testdata/parity_gaps.txt cases 5-9 and 35:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(repr(sqlglot.parse_one('x[1:2]')))"

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestParseSlice(t *testing.T) {
	bracket := parseOne(t, "x[1:2]")
	if bracket.Kind() != exp.KindBracket || len(bracket.Expressions()) != 1 {
		t.Fatalf("bracket mismatch:\n%s", bracket.ToS())
	}
	slice := bracket.Expressions()[0]
	if slice.Kind() != exp.KindSlice {
		t.Fatalf("expected Slice, got %v:\n%s", slice.Kind(), bracket.ToS())
	}
	if this, ok := slice.Arg("this").(exp.Expression); !ok || this == nil || this.Text("this") != "1" {
		t.Fatalf("this mismatch:\n%s", slice.ToS())
	}
	if end, ok := slice.Arg("expression").(exp.Expression); !ok || end == nil || end.Text("this") != "2" {
		t.Fatalf("expression mismatch:\n%s", slice.ToS())
	}
	if slice.Arg("step") != nil {
		t.Fatalf("expected nil step:\n%s", slice.ToS())
	}

	// x[-4:-1]: the DASH+COLON special-case doesn't fire here (the second COLON only
	// appears immediately after a bare "-", not after a full "-1" literal), so both ends
	// parse as ordinary Neg literals.
	bracket = parseOne(t, "x[-4:-1]")
	slice = bracket.Expressions()[0]
	this, _ := slice.Arg("this").(exp.Expression)
	end, _ := slice.Arg("expression").(exp.Expression)
	if this == nil || this.Kind() != exp.KindNeg || end == nil || end.Kind() != exp.KindNeg {
		t.Fatalf("x[-4:-1] this/expression mismatch:\n%s", slice.ToS())
	}

	for _, sql := range []string{"x[1:]", "x[:2]", "x[:]"} {
		bracket = parseOne(t, sql)
		if bracket.Kind() != exp.KindBracket || len(bracket.Expressions()) != 1 {
			t.Fatalf("%s: bracket mismatch:\n%s", sql, bracket.ToS())
		}
		if bracket.Expressions()[0].Kind() != exp.KindSlice {
			t.Fatalf("%s: expected Slice, got %v:\n%s", sql, bracket.Expressions()[0].Kind(), bracket.ToS())
		}
	}

	got, err := generateSQL(t, parseOne(t, "x[1:]"), "")
	if err != nil || got != "x[1:]" {
		t.Fatalf("x[1:] round-trip = %q, err=%v", got, err)
	}
	got, err = generateSQL(t, parseOne(t, "x[:]"), "")
	if err != nil || got != "x[:]" {
		t.Fatalf("x[:] round-trip = %q, err=%v", got, err)
	}
}

func TestParseNational(t *testing.T) {
	national := parseOne(t, "N'abc'")
	if national.Kind() != exp.KindNational || national.Text("this") != "abc" {
		t.Fatalf("National mismatch:\n%s", national.ToS())
	}
	if !national.IsPrimitive() {
		t.Fatalf("National should be primitive:\n%s", national.ToS())
	}
	got, err := generateSQL(t, national, "")
	if err != nil || got != "N'abc'" {
		t.Fatalf("round-trip = %q, err=%v", got, err)
	}

	// Lowercase n-prefix parses the same as uppercase N (compileConfig registers both
	// "n'" and "N'" as NATIONAL_STRING format-string prefixes).
	national = parseOne(t, "n'abc'")
	if national.Kind() != exp.KindNational {
		t.Fatalf("lowercase n-prefix mismatch:\n%s", national.ToS())
	}
	got, err = generateSQL(t, national, "")
	if err != nil || got != "N'abc'" {
		t.Fatalf("lowercase n-prefix round-trip = %q, err=%v", got, err)
	}

	for _, dialect := range []string{"", "mysql", "postgres"} {
		sql := "SELECT N'abc'"
		got, err := generateSQL(t, parseOneDialect(t, sql, dialect), dialect)
		if err != nil || got != sql {
			t.Fatalf("%s %q round-trip = %q, err=%v", dialect, sql, got, err)
		}
	}
}
