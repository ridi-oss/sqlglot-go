package dialects_test

import (
	"testing"

	"github.com/ridi-oss/sqlglot-go/dialects"
)

// TestGetOrRaiseDialectType demonstrates the DialectType polymorphism of GetOrRaise: nil resolves
// to the base dialect, a string resolves by name, and a *Dialect is returned as the SAME instance
// (no lossy re-resolution) — the property the schema/optimizer boundaries rely on.
func TestGetOrRaiseDialectType(t *testing.T) {
	base, err := dialects.GetOrRaise(nil)
	if err != nil || base == nil {
		t.Fatalf("GetOrRaise(nil): %v", err)
	}

	byName, err := dialects.GetOrRaise("mysql")
	if err != nil || byName == nil || byName.Name != "mysql" {
		t.Fatalf("GetOrRaise(\"mysql\") = %v, err %v", byName, err)
	}

	// A *Dialect is handed back verbatim (pointer identity), preserving every field.
	if got, err := dialects.GetOrRaise(byName); err != nil || got != byName {
		t.Fatalf("GetOrRaise(*Dialect) = %p, want same instance %p (err %v)", got, byName, err)
	}
}

// TestCanonicalStringDialectType covers CanonicalString across every DialectType form.
func TestCanonicalStringDialectType(t *testing.T) {
	cases := []struct {
		name    string
		dialect dialects.DialectType
		want    string
	}{
		{"nil", nil, ""},
		{"string", "mysql", "mysql"},
	}
	for _, tc := range cases {
		got, err := dialects.CanonicalString(tc.dialect)
		if err != nil {
			t.Fatalf("CanonicalString(%s): %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("CanonicalString(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}

	// A *Dialect reduces to its SettingsString, which round-trips back through GetOrRaise.
	mysql, _ := dialects.GetOrRaise("mysql")
	got, err := dialects.CanonicalString(mysql)
	if err != nil {
		t.Fatalf("CanonicalString(*Dialect): %v", err)
	}
	if got != mysql.SettingsString() {
		t.Fatalf("CanonicalString(*Dialect) = %q, want %q", got, mysql.SettingsString())
	}
	rt, err := dialects.GetOrRaise(got)
	if err != nil || rt.Name != "mysql" {
		t.Fatalf("GetOrRaise(CanonicalString(*Dialect)) = %v, err %v", rt, err)
	}
}
