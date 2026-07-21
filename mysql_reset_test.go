package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// MySQL `RESET …` administrative statements must degrade to a raw exp.Command rather than
// mis-parse as an Alias (`RESET AS MASTER`), which is a semantically wrong structural claim.
// See DEVIATIONS §1.6. Upstream MySQL produces the bogus Alias; Postgres already produces a
// Command, which this brings MySQL in line with.
func TestMySQLResetDegradesToCommand(t *testing.T) {
	for _, sql := range []string{
		"RESET MASTER",
		"RESET REPLICA",
		"RESET BINARY LOGS AND GTIDS", // valid MySQL 8.4; previously parse-errored
		"RESET QUERY CACHE",
	} {
		t.Run(sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindCommand {
				t.Fatalf("kind = %v, want Command:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
			if this := e.Text("this"); this != "RESET" {
				t.Fatalf("Command this = %q, want RESET", this)
			}
			out, gerr := sqlglot.Generate(e, "mysql", generator.Options{})
			if gerr != nil {
				t.Fatalf("generate: %v", gerr)
			}
			if out != sql {
				t.Fatalf("round-trip = %q, want %q", out, sql)
			}
		})
	}
}

// The COMMAND mapping must not steal `reset` from ordinary identifier positions — it is a
// non-reserved word in MySQL.
func TestMySQLResetStaysUsableAsIdentifier(t *testing.T) {
	for _, sql := range []string{
		"SELECT reset FROM t",
		"SELECT 1 AS reset",
		"SELECT reset.id FROM t AS reset",
	} {
		t.Run(sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindSelect {
				t.Fatalf("kind = %v, want Select:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
		})
	}
}
