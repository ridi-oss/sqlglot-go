package sqlglot_test

import (
	"strings"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
)

// TestStatementDiscriminatorContract enforces the contract documented in
// expressions/statement_discriminators.go: the string discriminator args a consumer classifies a
// statement by are canonical (upper-cased, trimmed) — and, where a bounded vocabulary is exported,
// carry the single stable spelling that constant names — so a consumer compares them directly, with
// no re-normalization. It also pins the fold from alias spellings (SLAVE -> REPLICA, MASTER LOGS ->
// BINARY LOGS), that mixed-case input does not leak into the discriminator, and the two documented
// boundaries: Postgres Show.this is verbatim (not canonical), and object kinds are not exported.
func TestStatementDiscriminatorContract(t *testing.T) {
	// assertCanonical checks the general contract independent of any dedicated constant: a
	// discriminator equals its own upper/trim form (and, for these fixtures, is non-empty).
	assertCanonical := func(t *testing.T, label, got string) {
		t.Helper()
		if got == "" {
			t.Fatalf("%s: discriminator is empty", label)
		}
		if want := strings.ToUpper(strings.TrimSpace(got)); got != want {
			t.Fatalf("%s: discriminator %q is not canonical (want %q)", label, got, want)
		}
	}

	intoKind := func(t *testing.T, sql string) string {
		t.Helper()
		root, err := sqlglot.ParseOne(sql, "mysql")
		if err != nil {
			t.Fatalf("parse %q: %v", sql, err)
		}
		into := root.Into()
		if into == nil {
			t.Fatalf("%q: no INTO clause on root", sql)
		}
		return into.Text("kind")
	}

	textArg := func(t *testing.T, sql, dialect, key string) string {
		t.Helper()
		root, err := sqlglot.ParseOne(sql, dialect)
		if err != nil {
			t.Fatalf("parse %q: %v", sql, err)
		}
		return root.Text(key)
	}

	t.Run("Into.kind", func(t *testing.T) {
		for sql, want := range map[string]string{
			"SELECT * FROM t INTO OUTFILE '/tmp/x'":  exp.IntoOutfile,
			"SELECT * FROM t INTO DUMPFILE '/tmp/x'": exp.IntoDumpfile,
		} {
			got := intoKind(t, sql)
			assertCanonical(t, sql, got)
			if got != want {
				t.Fatalf("%q: Into.kind = %q, want %q", sql, got, want)
			}
		}
	})

	t.Run("Show.this", func(t *testing.T) {
		for sql, want := range map[string]string{
			"SHOW WARNINGS":             exp.ShowWarnings,
			"SHOW ERRORS":               exp.ShowErrors,
			"SHOW GRANTS":               exp.ShowGrants,
			"SHOW CREATE USER u":        exp.ShowCreateUser,
			"SHOW PROCESSLIST":          exp.ShowProcesslist,
			"SHOW ENGINE INNODB STATUS": exp.ShowEngine,
			"SHOW BINLOG EVENTS":        exp.ShowBinlogEvents,
			"SHOW RELAYLOG EVENTS":      exp.ShowRelaylogEvents,
			"SHOW MASTER STATUS":        exp.ShowMasterStatus,
			"SHOW BINARY LOGS":          exp.ShowBinaryLogs,
			"SHOW REPLICAS":             exp.ShowReplicas,
			// Alias spellings fold to the canonical label — the normalization the contract promises.
			"SHOW SLAVE STATUS": exp.ShowReplicaStatus,
			"SHOW SLAVE HOSTS":  exp.ShowReplicas,
			"SHOW MASTER LOGS":  exp.ShowBinaryLogs,
		} {
			root, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			if root.Kind() != exp.KindShow {
				t.Fatalf("%q: kind = %v, want Show", sql, exp.ClassName(root.Kind()))
			}
			got := root.Text("this")
			assertCanonical(t, sql, got)
			if got != want {
				t.Fatalf("%q: Show.this = %q, want %q", sql, got, want)
			}
		}
	})

	t.Run("Create/Alter/Drop.kind", func(t *testing.T) {
		// The object-type vocabulary is intentionally not exported as constants (it is
		// verb/dialect-dependent — see the package doc), so these assert the canonical guarantee for
		// a structured node's `kind` directly against the object keyword.
		cases := []struct {
			sql, dialect, want string
		}{
			{"CREATE TABLE t (a INT)", "mysql", "TABLE"},
			{"CREATE VIEW v AS SELECT 1", "mysql", "VIEW"},
			{"CREATE SEQUENCE seq", "postgres", "SEQUENCE"},
			{"DROP TABLE t", "mysql", "TABLE"},
			{"DROP VIEW v", "mysql", "VIEW"},
			{"ALTER TABLE t ADD COLUMN c INT", "mysql", "TABLE"},
			// Lower-case input must not leak into the discriminator.
			{"create table t (a int)", "mysql", "TABLE"},
			{"drop database d", "postgres", "DATABASE"},
		}
		for _, c := range cases {
			got := textArg(t, c.sql, c.dialect, "kind")
			assertCanonical(t, c.sql, got)
			if got != c.want {
				t.Fatalf("%q: kind = %q, want %q", c.sql, got, c.want)
			}
		}
	})

	// Postgres SHOW is the documented boundary: Show.this is the verbatim config-parameter name, NOT
	// a canonical uppercase label. Pin it so a future change cannot silently start upper-casing it
	// (which would break the documented contract and a consumer keying on the raw GUC name).
	t.Run("Show.this Postgres boundary (verbatim, not canonical)", func(t *testing.T) {
		for sql, want := range map[string]string{
			"SHOW search_path": "search_path",
			"SHOW TIME ZONE":   "timezone",
		} {
			root, err := sqlglot.ParseOne(sql, "postgres")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			if root.Kind() != exp.KindShow {
				t.Fatalf("%q: kind = %v, want Show", sql, exp.ClassName(root.Kind()))
			}
			if got := root.Text("this"); got != want {
				t.Fatalf("%q: Postgres Show.this = %q, want verbatim %q", sql, got, want)
			}
		}
	})

	t.Run("SetItem.kind", func(t *testing.T) {
		root, err := sqlglot.ParseOne("SET TRANSACTION ISOLATION LEVEL SERIALIZABLE", "mysql")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		items := root.FindAll(exp.KindSetItem)
		if len(items) == 0 {
			t.Fatal("no SetItem found")
		}
		var found bool
		for _, item := range items {
			kind := item.Text("kind")
			if kind == "" {
				continue
			}
			assertCanonical(t, "SET TRANSACTION", kind)
			if kind == exp.SetItemTransaction {
				found = true
			}
		}
		if !found {
			t.Fatalf("SET TRANSACTION: no SetItem.kind == %q", exp.SetItemTransaction)
		}
	})
}
