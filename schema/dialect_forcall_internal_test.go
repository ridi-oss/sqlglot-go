package schema

import (
	"testing"

	"github.com/ridi-oss/sqlglot-go/dialects"
)

// TestDialectForCallPreservesDialect proves the schema threads the resolved *Dialect verbatim
// rather than round-tripping through a canonical string. SettingsString deliberately omits
// mysql_version, so the old CanonicalString-at-the-boundary path silently dropped it; this test
// asserts pointer identity (the exact instance survives), which fails if the reduction returns.
func TestDialectForCallPreservesDialect(t *testing.T) {
	versioned, err := dialects.GetOrRaise("mysql, mysql_version=80035")
	if err != nil {
		t.Fatalf("GetOrRaise(versioned): %v", err)
	}
	if versioned.MySQLVersion == nil {
		t.Fatal("precondition: MySQLVersion should be set")
	}
	// Confirm the failure mode we are guarding against: a SettingsString round-trip drops the
	// version, so anything that reduced a *Dialect to its canonical string would lose it.
	if rt, _ := dialects.GetOrRaise(versioned.SettingsString()); rt.MySQLVersion != nil {
		t.Fatal("precondition: SettingsString round-trip unexpectedly preserved mysql_version")
	}

	m, err := NewMappingSchema(nil, versioned, true)
	if err != nil {
		t.Fatalf("NewMappingSchema: %v", err)
	}

	// nil per-call arg falls back to the schema's own resolved dialect — same instance, version intact.
	got, err := m.dialectForCall(nil)
	if err != nil {
		t.Fatalf("dialectForCall(nil): %v", err)
	}
	if got != m.dialect || got.MySQLVersion == nil {
		t.Fatal("dialectForCall(nil) did not return the schema's version-preserving dialect")
	}

	// A per-call *Dialect override is threaded verbatim (pointer identity), not string-reduced.
	other, err := dialects.GetOrRaise("mysql, mysql_version=50727")
	if err != nil {
		t.Fatalf("GetOrRaise(other): %v", err)
	}
	got, err = m.dialectForCall(other)
	if err != nil {
		t.Fatalf("dialectForCall(other): %v", err)
	}
	if got != other || got.MySQLVersion == nil || *got.MySQLVersion != *other.MySQLVersion {
		t.Fatal("dialectForCall(*Dialect) did not preserve the exact versioned instance")
	}
}
