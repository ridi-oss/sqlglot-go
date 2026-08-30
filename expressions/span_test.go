package expressions

import "testing"

func TestNodeSpan(t *testing.T) {
	col := Column(Args{"this": ToIdentifier("a")})

	if _, _, ok := col.Span(); ok {
		t.Fatal("fresh node must have no span")
	}
	if _, ok := col.SpanText(); ok {
		t.Fatal("fresh node must have no span text")
	}

	col.SetSpan(7, 8)
	col.SetSpanText("a")
	start, end, ok := col.Span()
	if !ok || start != 7 || end != 8 {
		t.Fatalf("Span() = %d,%d,%v; want 7,8,true", start, end, ok)
	}
	if text, ok := col.SpanText(); !ok || text != "a" {
		t.Fatalf("SpanText() = %q,%v; want \"a\",true", text, ok)
	}

	// Spans survive Copy and never affect equality or hashing.
	clone := col.Copy()
	if s, e, ok := clone.Span(); !ok || s != 7 || e != 8 {
		t.Fatalf("copy Span() = %d,%d,%v; want 7,8,true", s, e, ok)
	}
	if text, ok := clone.SpanText(); !ok || text != "a" {
		t.Fatalf("copy SpanText() = %q,%v; want \"a\",true", text, ok)
	}
	plain := Column(Args{"this": ToIdentifier("a")})
	if !col.Equal(plain) || col.HashKey() != plain.HashKey() {
		t.Fatal("span must not affect Equal/HashKey")
	}
}
