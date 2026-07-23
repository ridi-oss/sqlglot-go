package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// SAVEPOINT / RELEASE SAVEPOINT transaction-control statements parse to exp.Savepoint. Pinned
// upstream models neither — it mis-parses `SAVEPOINT s` as an Alias (`SAVEPOINT AS s`) and
// parse-errors `RELEASE SAVEPOINT s`. Verified against PostgreSQL 17.6 and MySQL 8.0.33; see
// DEVIATIONS §1.8 and ledger id release-savepoint.
func TestSavepoint(t *testing.T) {
	cases := []struct {
		sql      string
		dialects []string
		name     string // savepoint name (identifier text)
		kind     string // "" for SAVEPOINT create, "RELEASE" for release
		want     string // round-trip output
	}{
		{"SAVEPOINT s", []string{"postgres", "mysql", "base"}, "s", "", "SAVEPOINT s"},
		// Unreserved keyword names are accepted by both engines.
		{"SAVEPOINT commit", []string{"postgres", "mysql", "base"}, "commit", "", "SAVEPOINT commit"},
		{"RELEASE SAVEPOINT s", []string{"postgres", "mysql", "base"}, "s", "RELEASE", "RELEASE SAVEPOINT s"},
		// Bare RELEASE (SAVEPOINT keyword omitted) is Postgres-only; it normalizes on output to the
		// explicit SAVEPOINT spelling (which Postgres also accepts).
		{"RELEASE s", []string{"postgres"}, "s", "RELEASE", "RELEASE SAVEPOINT s"},
		// Postgres: a lone `RELEASE savepoint` / `RELEASE "SAVEPOINT"` releases a savepoint literally
		// NAMED savepoint/SAVEPOINT — the optional SAVEPOINT keyword only applies when a distinct name
		// follows it.
		{"RELEASE savepoint", []string{"postgres"}, "savepoint", "RELEASE", "RELEASE SAVEPOINT savepoint"},
		{`RELEASE "SAVEPOINT"`, []string{"postgres"}, "SAVEPOINT", "RELEASE", `RELEASE SAVEPOINT "SAVEPOINT"`},
	}
	for _, tc := range cases {
		for _, dialect := range tc.dialects {
			e, err := sqlglot.ParseOne(tc.sql, dialect)
			if err != nil {
				t.Errorf("%q [%s]: parse: %v", tc.sql, dialect, err)
				continue
			}
			if e.Kind() != exp.KindSavepoint {
				t.Errorf("%q [%s]: Kind = %s, want Savepoint\n%s", tc.sql, dialect, exp.ClassName(e.Kind()), e.ToS())
				continue
			}
			if e.This() == nil || e.This().Name() != tc.name {
				t.Errorf("%q [%s]: savepoint name = %q, want %q\n%s", tc.sql, dialect, nameOf(e.This()), tc.name, e.ToS())
			}
			if kind, _ := e.Arg("kind").(string); kind != tc.kind {
				t.Errorf("%q [%s]: kind = %q, want %q", tc.sql, dialect, kind, tc.kind)
			}
			if got, _ := sqlglot.Generate(e, dialect, generator.Options{}); got != tc.want {
				t.Errorf("%q [%s]: round-trip = %q, want %q", tc.sql, dialect, got, tc.want)
			}
		}
	}
}

// Quoted savepoint names follow the dialect's identifier quoting: Postgres `"x"` is an identifier,
// but MySQL `"x"` is a string literal (rejected as a name) while “ `x` “ is the identifier form.
func TestSavepointQuotedNames(t *testing.T) {
	if got, _ := spRoundTrip(t, `SAVEPOINT "my sp"`, "postgres"); got != `SAVEPOINT "my sp"` {
		t.Errorf(`postgres SAVEPOINT "my sp" -> %q`, got)
	}
	if got, _ := spRoundTrip(t, "SAVEPOINT `my sp`", "mysql"); got != "SAVEPOINT `my sp`" {
		t.Errorf("mysql SAVEPOINT `my sp` -> %q", got)
	}
}

// A string/number savepoint name is rejected by both engines, so it must NOT parse to a Savepoint.
// (Postgres folds `SAVEPOINT 'x'` to a user-type typed literal Cast — still fail-closed, since the
// type does not exist — while `SAVEPOINT 1` is a parse error; either way it is not a Savepoint.)
func TestSavepointInvalidNameNotSavepoint(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql", "base"} {
		for _, sql := range []string{"SAVEPOINT 'foo'", "SAVEPOINT 1"} {
			if e, err := sqlglot.ParseOne(sql, dialect); err == nil && e.Kind() == exp.KindSavepoint {
				t.Errorf("%q [%s]: parsed as Savepoint, want fail-closed (invalid name)", sql, dialect)
			}
		}
	}
}

// The leading statement comment is preserved on the Savepoint root (the custom text dispatch must
// capture it just like the standard BEGIN/COMMIT/ROLLBACK statement dispatch does).
func TestSavepointPreservesLeadingComment(t *testing.T) {
	e, err := sqlglot.ParseOne("/* lead */ SAVEPOINT s", "postgres")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cs := e.Comments()
	if len(cs) != 1 || cs[0] != " lead " {
		t.Errorf("leading comment = %v, want [\" lead \"]\n%s", cs, e.ToS())
	}
}

// Bare `RELEASE <name>` (SAVEPOINT keyword omitted) is not a savepoint statement in MySQL/base —
// real MySQL requires the SAVEPOINT keyword — so it stays the exact pre-existing fall-through parse
// (an Alias of `RELEASE` aliased to the name), NOT a Savepoint.
func TestReleaseWithoutSavepointKeywordFallsThrough(t *testing.T) {
	cases := []struct{ dialect, want string }{
		{"mysql", "`RELEASE` AS s"},
		{"base", "RELEASE AS s"},
	}
	for _, tc := range cases {
		e, err := sqlglot.ParseOne("RELEASE s", tc.dialect)
		if err != nil {
			t.Errorf("`RELEASE s` [%s]: parse: %v", tc.dialect, err)
			continue
		}
		if e.Kind() != exp.KindAlias {
			t.Errorf("`RELEASE s` [%s]: Kind = %s, want Alias (fall-through)\n%s", tc.dialect, exp.ClassName(e.Kind()), e.ToS())
		}
		if got, _ := sqlglot.Generate(e, tc.dialect, generator.Options{}); got != tc.want {
			t.Errorf("`RELEASE s` [%s]: round-trip = %q, want %q", tc.dialect, got, tc.want)
		}
	}
}

// `savepoint` / `release` remain usable as ordinary identifiers (the text dispatch only fires at the
// leading position), and `ROLLBACK TO SAVEPOINT` stays an exp.Rollback, unchanged.
func TestSavepointDoesNotDisturbIdentifiersOrRollback(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql"} {
		for _, sql := range []string{"SELECT savepoint FROM t", "SELECT release AS r FROM t"} {
			e, err := sqlglot.ParseOne(sql, dialect)
			if err != nil {
				t.Errorf("%q [%s]: parse: %v", sql, dialect, err)
				continue
			}
			if e.Kind() != exp.KindSelect {
				t.Errorf("%q [%s]: Kind = %s, want Select", sql, dialect, exp.ClassName(e.Kind()))
			}
		}
		e, err := sqlglot.ParseOne("ROLLBACK TO SAVEPOINT s", dialect)
		if err != nil {
			t.Errorf("ROLLBACK TO SAVEPOINT s [%s]: parse: %v", dialect, err)
			continue
		}
		if e.Kind() != exp.KindRollback {
			t.Errorf("ROLLBACK TO SAVEPOINT s [%s]: Kind = %s, want Rollback", dialect, exp.ClassName(e.Kind()))
		}
	}
}

func nameOf(e exp.Expression) string {
	if e == nil {
		return "<nil>"
	}
	return e.Name()
}

func spRoundTrip(t *testing.T, sql, dialect string) (string, exp.Expression) {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("%q [%s]: parse: %v", sql, dialect, err)
	}
	out, err := sqlglot.Generate(e, dialect, generator.Options{})
	if err != nil {
		t.Fatalf("%q [%s]: generate: %v", sql, dialect, err)
	}
	return out, e
}
