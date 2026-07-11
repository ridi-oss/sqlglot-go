package expressions

import "testing"

// TestNodeMeta covers the ported Expression.meta / meta_get (core.py:991/996): lazy
// allocation, no-alloc reads, deep-copy on Copy (with copy/original independence), and the
// invariant that metadata never leaks into the node's repr.
func TestNodeMeta(t *testing.T) {
	id := Identifier(Args{"this": "x"})

	// MetaGet on an untouched node reads nothing and does not allocate the map.
	if got := id.MetaGet("is_table"); got != nil {
		t.Fatalf("MetaGet on fresh node = %v, want nil", got)
	}
	if id.(*Node).meta != nil {
		t.Fatal("MetaGet allocated the meta map; it must not")
	}

	// Meta() allocates and is mutable; MetaGet then reads the stored value.
	id.Meta()["is_table"] = true
	if got := id.MetaGet("is_table"); got != true {
		t.Fatalf("MetaGet(is_table) = %v, want true", got)
	}

	// Repr must not include metadata.
	if before := Identifier(Args{"this": "x"}).ToS(); before != id.ToS() {
		t.Fatalf("meta leaked into ToS(): %q vs %q", id.ToS(), before)
	}

	// Copy carries meta, but copy and original are independent maps.
	clone := id.Copy()
	if got := clone.MetaGet("is_table"); got != true {
		t.Fatalf("Copy did not carry meta: MetaGet(is_table) = %v", got)
	}
	clone.Meta()["is_table"] = false
	if got := id.MetaGet("is_table"); got != true {
		t.Fatalf("mutating the copy's meta changed the original: %v", got)
	}
}
