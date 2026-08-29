package expressions

import "testing"

func TestNodeSpan(t *testing.T) {
	col := Column(Args{"this": ToIdentifier("a")})

	if _, _, ok := col.Span(); ok {
		t.Fatal("fresh node must have no span")
	}
	if _, ok := SpanText("SELECT a", col); ok {
		t.Fatal("SpanText must report no span")
	}

	col.SetSpan(7, 8)
	start, end, ok := col.Span()
	if !ok || start != 7 || end != 8 {
		t.Fatalf("Span() = %d,%d,%v; want 7,8,true", start, end, ok)
	}
	if text, ok := SpanText("SELECT a", col); !ok || text != "a" {
		t.Fatalf("SpanText = %q,%v; want \"a\",true", text, ok)
	}

	// Spans survive Copy and never affect equality or hashing.
	clone := col.Copy()
	if s, e, ok := clone.Span(); !ok || s != 7 || e != 8 {
		t.Fatalf("copy Span() = %d,%d,%v; want 7,8,true", s, e, ok)
	}
	plain := Column(Args{"this": ToIdentifier("a")})
	if !col.Equal(plain) || col.HashKey() != plain.HashKey() {
		t.Fatal("span must not affect Equal/HashKey")
	}

	// Out-of-range or inverted spans fail closed.
	col.SetSpan(5, 3)
	if _, ok := SpanText("SELECT a", col); ok {
		t.Fatal("inverted span must not resolve")
	}
	col.SetSpan(7, 99)
	if _, ok := SpanText("SELECT a", col); ok {
		t.Fatal("span past end of sql must not resolve")
	}
}

func TestSpanTextRuneOffsets(t *testing.T) {
	// Token offsets are rune offsets; multi-byte source before the span must not skew the slice.
	sql := "SELECT '데이터', x"
	col := Column(Args{"this": ToIdentifier("x")})
	col.SetSpan(14, 15)
	if text, ok := SpanText(sql, col); !ok || text != "x" {
		t.Fatalf("SpanText = %q,%v; want \"x\",true", text, ok)
	}
}
