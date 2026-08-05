package expressions

import "testing"

func TestCommandKeyword(t *testing.T) {
	cases := []struct {
		name string
		this any
		want string
	}{
		// parseAsCommand path keeps `this` verbatim (may be any source case, single token).
		{"verbatim upper", "GRANT", "GRANT"},
		{"verbatim lower", "grant", "GRANT"},
		{"mixed case", "Flush", "FLUSH"},
		{"leading/trailing space", "  reset  ", "RESET"},
		// parseCommand path stores an already-upper multi-word COMMAND token (MySQL LOCK TABLES).
		{"multi-word", "LOCK TABLES", "LOCK TABLES"},
		{"multi-word collapse", "LOCK   TABLES", "LOCK TABLES"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := Command(Args{"this": c.this, "expression": " remainder ignored"})
			if got := cmd.(*Node).Keyword(); got != c.want {
				t.Fatalf("Keyword() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestKeywordNonCommandIsEmpty(t *testing.T) {
	// Keyword is Command-only; a non-Command node whose `this` happens to be a string must not be
	// mistaken for a command keyword.
	show := Show(Args{"this": ShowWarnings})
	if got := show.(*Node).Keyword(); got != "" {
		t.Fatalf("Show.Keyword() = %q, want \"\"", got)
	}
}

func TestIntoAccessor(t *testing.T) {
	into := Into(Args{"kind": IntoOutfile})
	sel := newNode(KindSelect, Args{"into": into})
	got := sel.Into()
	if got == nil {
		t.Fatal("Into() = nil, want the Into node")
	}
	if got.Text("kind") != IntoOutfile {
		t.Fatalf("Into().kind = %q, want %q", got.Text("kind"), IntoOutfile)
	}

	// Absent INTO reads as nil, not a typed-nil or panic.
	if bare := newNode(KindSelect, Args{}).Into(); bare != nil {
		t.Fatalf("Into() with no INTO = %v, want nil", bare)
	}
}
