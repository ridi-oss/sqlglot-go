package optimizer_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
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
		// lctn=0: INFORMATION_SCHEMA is a virtual schema MySQL matches case-insensitively regardless of
		// lctn (live-verified on MySQL 8.0.46), so its schema name AND the table names under it fold
		// even though relation-level names are otherwise preserved here.
		{"lctn0 information_schema folds schema+table", sensitive,
			"SELECT TABLE_NAME FROM Information_Schema.Tables",
			"SELECT table_name FROM information_schema.tables"},
		{"lctn0 information_schema mixed case folds", sensitive,
			"SELECT * FROM INFORMATION_SCHEMA.SCHEMATA",
			"SELECT * FROM information_schema.schemata"},
		// lctn=0: an information_schema-qualified column folds its db + table qualifier too.
		{"lctn0 information_schema column qualifier folds", sensitive,
			"SELECT Information_Schema.Tables.Table_Name FROM Information_Schema.Tables",
			"SELECT information_schema.tables.table_name FROM information_schema.tables"},
		// lctn=0: performance_schema / mysql / sys are ordinary on-disk DBs — case-SENSITIVE, preserved.
		{"lctn0 performance_schema stays case-sensitive", sensitive,
			"SELECT EVENT_NAME FROM Performance_Schema.Accounts",
			"SELECT event_name FROM Performance_Schema.Accounts"},
		// lctn=1/2: every identifier folds.
		{"lctn1/2 folds all", insensitive,
			"SELECT Users.RRN FROM Users", "SELECT users.rrn FROM users"},
		{"lctn1/2 folds CTE name too", insensitive,
			"WITH Users AS (SELECT rrn FROM other) SELECT rrn FROM Users",
			"WITH users AS (SELECT rrn FROM other) SELECT rrn FROM users"},
		// lctn=1/2: information_schema folds like everything else (no special-casing needed).
		{"lctn1/2 information_schema folds", insensitive,
			"SELECT * FROM Information_Schema.Tables", "SELECT * FROM information_schema.tables"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := norm(t, tc.dialect, tc.sql); got != tc.expected {
				t.Fatalf("NormalizeIdentifiers(%q)\n  = %q\n  want %q", tc.sql, got, tc.expected)
			}
		})
	}
}
