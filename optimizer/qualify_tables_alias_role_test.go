package optimizer_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
)

// TestQualifyTablesDefaultAliasRelationRole is a regression test for the role-aware fold of the
// default alias that QualifyTables injects for an unaliased table.
//
// Under the port's non-upstream MySQLCaseSensitiveTableNames strategy (models MySQL
// lower_case_table_names=0), a table alias is relation-level and must stay case-sensitive so it
// matches the case-preserved column qualifier elsewhere in the query. The bug: the alias was folded
// as a parentless identifier BEFORE being attached under the TableAlias, so the role-aware strategy
// (which reads the identifier's parent) misclassified it as column-level and lowercased it —
// `FROM Users` became `... AS users` while `SELECT Users.rrn` stayed `Users`, so scope resolution
// could no longer bind Users.rrn and lineage came back empty. The fix normalizes the alias AFTER
// attaching it, giving it the TableAlias parent.
func TestQualifyTablesDefaultAliasRelationRole(t *testing.T) {
	cases := []struct {
		name     string
		strategy dialects.NormalizationStrategy
		want     string // full generated SQL after NormalizeIdentifiers + QualifyTables(db="app")
	}{
		{
			// The regression: the injected alias stays "Users" and matches the qualifier "Users".
			name:     "role-aware lctn=0 preserves the table alias case",
			strategy: dialects.MySQLCaseSensitiveTableNames,
			want:     "SELECT Users.rrn FROM app.Users AS Users",
		},
		{
			// Case-sensitive folds nothing, so the alias is trivially preserved too.
			name:     "case-sensitive preserves everything",
			strategy: dialects.CaseSensitive,
			want:     "SELECT Users.rrn FROM app.Users AS Users",
		},
		{
			// Case-insensitive folds every identifier, so table, qualifier, and alias all lowercase
			// consistently — the alias still matches the qualifier.
			name:     "case-insensitive folds table, qualifier, and alias alike",
			strategy: dialects.MySQLCaseInsensitive,
			want:     "SELECT users.rrn FROM app.users AS users",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := dialects.MySQL()
			d.NormalizationStrategy = tc.strategy

			e, err := sqlglot.ParseOne("SELECT Users.rrn FROM Users", "mysql")
			if err != nil {
				t.Fatalf("ParseOne: %v", err)
			}
			// Mirror the Qualify pipeline order — normalize identifiers, then qualify tables with a
			// fixed schema-level qualifier — using the SAME role-aware dialect throughout (the
			// single-per-connection-Dialect design the downstream consumer wants to rely on).
			e = optimizer.NormalizeIdentifiers(e, d)
			e = optimizer.QualifyTables(e, "app", nil, d, false, nil, nil, nil)

			got, err := sqlglot.Generate(e, "mysql", generator.Options{})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != tc.want {
				t.Fatalf("QualifyTables alias fold mismatch:\n  got:  %s\n  want: %s", got, tc.want)
			}
		})
	}
}
