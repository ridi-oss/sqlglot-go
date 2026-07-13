package optimizer_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
)

// MySQL identifier normalization is role-aware under lower_case_table_names=0
// (mysql_case_sensitive_table_names): relation-level identifiers — table/db names, column QUALIFIERS,
// and table-alias/CTE names — are case-SENSITIVE and preserved, while column-level identifiers — leaf
// column names, CTE output-column lists, and column aliases — fold with MySQLLower. Under lctn=1/2
// (mysql_case_insensitive) every identifier folds. These match MySQL 8.4 exactly: `SELECT users.rrn
// FROM Users` errors (qualifiers are case-sensitive), and a mixed-case CTE binds by exact case.
func TestNormalizeIdentifiersMySQLStrategies(t *testing.T) {
	norm := func(t *testing.T, dialect, sql string) string {
		t.Helper()
		expr, err := sqlglot.ParseOne(sql, dialect)
		if err != nil {
			t.Fatalf("ParseOne(%q): %v", sql, err)
		}
		got, err := sqlglot.Generate(optimizer.NormalizeIdentifiers(expr, dialect), dialect, generator.Options{})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return got
	}

	const sensitive = "mysql, normalization_strategy=mysql_case_sensitive_table_names"
	const insensitive = "mysql, normalization_strategy=mysql_case_insensitive"

	cases := []struct {
		name     string
		dialect  string
		sql      string
		expected string
	}{
		// lctn=0: qualifier + table name are case-sensitive (preserved); the leaf column folds. The
		// qualifier MUST stay `Users` so it keeps matching the preserved table `Users` — folding it to
		// `users` made a qualified column against an unaliased mixed-case table drop from lineage.
		{"lctn0 qualifier+table preserved, leaf folds", sensitive,
			"SELECT Users.RRN FROM Users", "SELECT Users.rrn FROM Users"},
		// lctn=0: the CTE name is case-sensitive; preserving it on BOTH the definition and the reference
		// keeps the reference bound to the CTE — folding only the definition made the reference miss the
		// CTE and resolve a same-spelled physical table (a wrong-relation bind).
		{"lctn0 CTE name preserved", sensitive,
			"WITH Users AS (SELECT rrn FROM other) SELECT rrn FROM Users",
			"WITH Users AS (SELECT rrn FROM other) SELECT rrn FROM Users"},
		// lctn=0: a CTE output-column list is column-level and folds, matching its consumer (round-3).
		{"lctn0 CTE output-column folds", sensitive,
			"WITH cte(Secret) AS (SELECT rrn FROM t) SELECT secret FROM cte",
			"WITH cte(secret) AS (SELECT rrn FROM t) SELECT secret FROM cte"},
		// lctn=0: a column alias is column-level and folds.
		{"lctn0 column alias folds", sensitive,
			"SELECT rrn AS Foo FROM t", "SELECT rrn AS foo FROM t"},
		// lctn=1/2: every identifier folds.
		{"lctn1/2 folds all", insensitive,
			"SELECT Users.RRN FROM Users", "SELECT users.rrn FROM users"},
		{"lctn1/2 folds CTE name too", insensitive,
			"WITH Users AS (SELECT rrn FROM other) SELECT rrn FROM Users",
			"WITH users AS (SELECT rrn FROM other) SELECT rrn FROM users"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := norm(t, tc.dialect, tc.sql); got != tc.expected {
				t.Fatalf("NormalizeIdentifiers(%q)\n  = %q\n  want %q", tc.sql, got, tc.expected)
			}
		})
	}
}
