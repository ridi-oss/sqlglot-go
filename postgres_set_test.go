package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Postgres SET special-forms parse into Set{SetItem{kind: ...}} — a grammar extension beyond
// pinned upstream, which degrades each to a raw Command. A consumer reads SetItem.kind to tell a
// privileged SET (ROLE, SESSION AUTHORIZATION) from a benign one (TIME ZONE, NAMES, CONSTRAINTS,
// SESSION CHARACTERISTICS) without string-scanning. Verified against PostgreSQL 17.6.

func setItemKind(t *testing.T, e exp.Expression) string {
	t.Helper()
	if e.Kind() != exp.KindSet {
		t.Fatalf("root = %v, want Set:\n%s", exp.ClassName(e.Kind()), e.ToS())
	}
	items := e.Expressions()
	if len(items) != 1 {
		t.Fatalf("Set items = %d, want 1:\n%s", len(items), e.ToS())
	}
	return items[0].Text("kind")
}

func TestPostgresSetSpecialForms(t *testing.T) {
	cases := []struct {
		sql      string
		wantKind string
	}{
		{"SET ROLE admin", "ROLE"},
		{"SET ROLE NONE", "ROLE"},
		{"SET ROLE 'admin'", "ROLE"},
		{"SET SESSION AUTHORIZATION bob", "SESSION AUTHORIZATION"},
		{"SET SESSION AUTHORIZATION DEFAULT", "SESSION AUTHORIZATION"},
		{"SET TIME ZONE 'UTC'", "TIME ZONE"},
		{"SET TIME ZONE LOCAL", "TIME ZONE"},
		{"SET TIME ZONE DEFAULT", "TIME ZONE"},
		{"SET TIME ZONE INTERVAL '+00:00' HOUR TO MINUTE", "TIME ZONE"},
		{"SET TIME ZONE 7", "TIME ZONE"},
		{"SET TIME ZONE -5", "TIME ZONE"},  // signed numeric offset
		{"SET TIME ZONE UTC", "TIME ZONE"}, // bare zone name
		{"SET NAMES 'utf8'", "NAMES"},
		{"SET NAMES DEFAULT", "NAMES"},
		{"SET CONSTRAINTS ALL DEFERRED", "CONSTRAINTS"},
		{"SET CONSTRAINTS ALL IMMEDIATE", "CONSTRAINTS"},
		{"SET CONSTRAINTS a, b DEFERRED", "CONSTRAINTS"},
		{`SET CONSTRAINTS "ALL" DEFERRED`, "CONSTRAINTS"},      // quoted "ALL" is a constraint name
		{"SET CONSTRAINTS public.foo DEFERRED", "CONSTRAINTS"}, // schema-qualified name
		{"SET SESSION CHARACTERISTICS AS TRANSACTION ISOLATION LEVEL SERIALIZABLE", "SESSION CHARACTERISTICS"},
		{"SET SESSION CHARACTERISTICS AS TRANSACTION READ WRITE", "SESSION CHARACTERISTICS"},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if kind := setItemKind(t, e); kind != tc.wantKind {
				t.Fatalf("SetItem kind = %q, want %q:\n%s", kind, tc.wantKind, e.ToS())
			}
			out, gerr := sqlglot.Generate(e, "postgres", generator.Options{})
			if gerr != nil {
				t.Fatalf("generate: %v", gerr)
			}
			if out != tc.sql {
				t.Fatalf("round-trip = %q, want %q", out, tc.sql)
			}
		})
	}
}

// The discriminator a consumer keys on: privileged vs benign, readable straight off SetItem.kind.
func TestPostgresSetKindDiscriminator(t *testing.T) {
	privileged := map[string]bool{"ROLE": true, "SESSION AUTHORIZATION": true}
	for _, tc := range []struct {
		sql      string
		wantPriv bool
	}{
		{"SET ROLE admin", true},
		{"SET SESSION AUTHORIZATION bob", true},
		{"SET TIME ZONE 'UTC'", false},
		{"SET NAMES 'utf8'", false},
		{"SET CONSTRAINTS ALL DEFERRED", false},
		{"SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY", false},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := privileged[setItemKind(t, e)]; got != tc.wantPriv {
				t.Fatalf("privileged=%v, want %v for %q", got, tc.wantPriv, tc.sql)
			}
		})
	}
}

// Postgres also exposes `role` and `session_authorization` as ordinary GUCs, so the assignment
// spellings perform the same privilege change but carry a scope-only/blank kind — the privileged
// signal is the assignment's LHS variable name, NOT the SetItem.kind. A consumer must deny on the
// LHS name too (see DEVIATIONS). This test guards that the LHS name is structurally reachable.
func TestPostgresSetGUCAliasLHSReachable(t *testing.T) {
	for _, tc := range []struct {
		sql     string
		wantVar string
	}{
		{"SET SESSION role = attacker", "role"},
		{"SET session_authorization = attacker", "session_authorization"},
		{"SET LOCAL session_authorization = attacker", "session_authorization"},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "postgres")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindSet || len(e.Expressions()) != 1 {
				t.Fatalf("want single-item Set:\n%s", e.ToS())
			}
			assign, ok := e.Expressions()[0].Arg("this").(exp.Expression)
			if !ok || assign.Kind() != exp.KindEQ {
				t.Fatalf("SetItem.this not an EQ assignment:\n%s", e.ToS())
			}
			lhs, ok := assign.Arg("this").(exp.Expression)
			if !ok || lhs.Name() != tc.wantVar {
				t.Fatalf("LHS var = %q, want %q:\n%s", func() string {
					if lhs != nil {
						return lhs.Name()
					}
					return "<nil>"
				}(), tc.wantVar, e.ToS())
			}
		})
	}
}

func TestPostgresSetFailClosedAndRegressions(t *testing.T) {
	// Ordinary assignments and the existing TRANSACTION form are unchanged.
	t.Run("assignments unchanged", func(t *testing.T) {
		for _, sql := range []string{
			"SET search_path = public",
			"SET SESSION search_path = public",
			"SET LOCAL search_path TO public",
			"SET TRANSACTION ISOLATION LEVEL SERIALIZABLE",
		} {
			e, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			if e.Kind() != exp.KindSet {
				t.Fatalf("%q: kind = %v, want Set:\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
			}
		}
	})

	// A special form missing its required value fails closed to Command (never a degenerate Set),
	// as do the SESSION/LOCAL-scoped variants this extension deliberately does not model.
	t.Run("malformed and unmodeled forms fail closed", func(t *testing.T) {
		for _, sql := range []string{
			"SET ROLE",                                              // missing role
			"SET TIME ZONE",                                         // missing value
			"SET SESSION AUTHORIZATION",                             // missing user
			"SET CONSTRAINTS ALL",                                   // missing mode
			"SET SESSION TIME ZONE 'UTC'",                           // scoped TIME ZONE stays unmodeled (benign)
			"SET CONSTRAINTS ALL GARBAGE",                           // mode not DEFERRED/IMMEDIATE
			"SET SESSION CHARACTERISTICS READ ONLY",                 // missing AS TRANSACTION
			"SET SESSION CHARACTERISTICS AS TRANSACTION",            // missing mode
			"SET SESSION CHARACTERISTICS AS TRANSACTION DEFERRABLE", // unmodeled characteristic (no crash)
			"SET NAMES 'utf8' COLLATE 'x'",                          // COLLATE is not valid Postgres SET NAMES
			"SET NAMES utf8",                                        // unquoted charset is invalid Postgres
			"SET TIME ZONE 'UTC', ROLE admin",                       // comma-combined multi-item (Postgres rejects)
			"SET a = 1, b = 2",                                      // multi-item Postgres SET
		} {
			e, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			if e.Kind() != exp.KindCommand {
				t.Fatalf("%q: kind = %v, want Command (fail-closed):\n%s", sql, exp.ClassName(e.Kind()), e.ToS())
			}
		}
	})

	// The special forms are Postgres-only; base/MySQL leave them as Command.
	t.Run("special forms are postgres-gated", func(t *testing.T) {
		for _, dialect := range []string{"", "mysql"} {
			e, err := sqlglot.ParseOne("SET ROLE admin", dialect)
			if err != nil {
				t.Fatalf("parse [%s]: %v", dialect, err)
			}
			if e.Kind() != exp.KindCommand {
				t.Fatalf("[%s] SET ROLE admin = %v, want Command:\n%s", dialect, exp.ClassName(e.Kind()), e.ToS())
			}
		}
	})
}
