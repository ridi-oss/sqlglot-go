package schema_test

import (
	"strings"
	"testing"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/schema"
)

const (
	mysqlLctn0 = "mysql, normalization_strategy=mysql_case_sensitive_table_names"
	mysqlLctn1 = "mysql, normalization_strategy=mysql_case_insensitive"
)

func mysqlTable(catalog, db, table string) exp.Expression {
	return exp.Table(exp.Args{
		"this":    exp.ToIdentifier(table),
		"db":      exp.ToIdentifier(db),
		"catalog": exp.ToIdentifier(catalog),
	})
}

func mustMappingSchema(t *testing.T, mapping *schema.Mapping, dialect string) *schema.MappingSchema {
	t.Helper()
	s, err := schema.NewMappingSchema(mapping, dialect, true)
	if err != nil {
		t.Fatalf("NewMappingSchema(%q): %v", dialect, err)
	}
	return s
}

func columnNames(t *testing.T, s schema.Schema, table exp.Expression) []string {
	t.Helper()
	got, err := s.ColumnNames(table, false, "", nil)
	if err != nil {
		t.Fatalf("ColumnNames: %v", err)
	}
	return got
}

// catalog.schema.table mapping built key-by-key so insertion order (and exact casing) is controlled.
func mapping3(catalog, db, table string, columns [][2]string) *schema.Mapping {
	cols := schema.NewMapping()
	for _, c := range columns {
		cols.Set(c[0], c[1])
	}
	tbl := schema.NewMapping()
	tbl.Set(table, cols)
	sch := schema.NewMapping()
	sch.Set(db, tbl)
	cat := schema.NewMapping()
	cat.Set(catalog, sch)
	return cat
}

// Regression for the reported bug: bulk NewMappingSchema(normalize=true) under lctn=0
// (mysql_case_sensitive_table_names) must PRESERVE table/schema-name case (columns still fold), just
// like the query-side normalization does. Before the fix it folded relation keys to lowercase because
// each key was normalized as a detached (parentless) identifier and misread as a column.
func TestBulkMappingSchemaLctn0PreservesRelationCase(t *testing.T) {
	s := mustMappingSchema(t, mapping3("def", "App", "Users",
		[][2]string{{"ID", "BIGINT"}, {"RRN", "VARCHAR"}}), mysqlLctn0)

	// The table's own exact casing resolves; columns are folded (case-insensitive on every platform).
	if got := columnNames(t, s, mysqlTable("def", "App", "Users")); !equalStrings(got, []string{"id", "rrn"}) {
		t.Fatalf("def.App.Users columns = %v, want [id rrn]", got)
	}
	// A lowercased relation spelling must NOT resolve — lctn=0 is case-sensitive for table/schema names.
	if got := columnNames(t, s, mysqlTable("def", "app", "users")); len(got) != 0 {
		t.Fatalf("def.app.users unexpectedly resolved to %v (relation names must stay case-sensitive)", got)
	}
}

// INFORMATION_SCHEMA is case-insensitive regardless of lctn (live-verified on MySQL 8.0.46), so a bulk
// mapping stores its schema+table names folded, and a query in ANY case resolves — unlike ordinary
// schemas which stay case-sensitive under lctn=0.
func TestBulkMappingSchemaLctn0InformationSchemaFolds(t *testing.T) {
	s := mustMappingSchema(t, mapping3("def", "information_schema", "SCHEMATA",
		[][2]string{{"SCHEMA_NAME", "VARCHAR"}}), mysqlLctn0)

	for _, tbl := range []exp.Expression{
		mysqlTable("def", "information_schema", "SCHEMATA"),
		mysqlTable("def", "information_schema", "schemata"),
		mysqlTable("def", "INFORMATION_SCHEMA", "Schemata"),
		mysqlTable("def", "Information_Schema", "schemata"),
	} {
		if got := columnNames(t, s, tbl); !equalStrings(got, []string{"schema_name"}) {
			t.Fatalf("information_schema lookup %v columns = %v, want [schema_name]", tbl, got)
		}
	}
}

// Non-ASCII: İ (U+0130) folds to i under MySQL's utf8mb3_general_ci, so İNFORMATION_SCHEMA is
// recognized as the virtual schema (and its table folds) — Go's strings.EqualFold would miss this,
// which is why the match uses MySQLLower, not EqualFold.
func TestBulkMappingSchemaLctn0InformationSchemaNonASCII(t *testing.T) {
	s := mustMappingSchema(t, mapping3("def", "İNFORMATION_SCHEMA", "TABLES",
		[][2]string{{"TABLE_NAME", "VARCHAR"}}), mysqlLctn0)
	if got := columnNames(t, s, mysqlTable("def", "information_schema", "tables")); !equalStrings(got, []string{"table_name"}) {
		t.Fatalf("İNFORMATION_SCHEMA.TABLES should fold to information_schema.tables; lookup = %v", got)
	}
}

// A non-information_schema system DB (performance_schema/mysql/sys) is an ordinary on-disk database:
// case-SENSITIVE under lctn=0, so it must NOT fold.
func TestBulkMappingSchemaLctn0PerformanceSchemaStaysCaseSensitive(t *testing.T) {
	s := mustMappingSchema(t, mapping3("def", "performance_schema", "Accounts",
		[][2]string{{"USER", "VARCHAR"}}), mysqlLctn0)

	if got := columnNames(t, s, mysqlTable("def", "performance_schema", "Accounts")); !equalStrings(got, []string{"user"}) {
		t.Fatalf("performance_schema.Accounts = %v, want [user]", got)
	}
	if got := columnNames(t, s, mysqlTable("def", "performance_schema", "accounts")); len(got) != 0 {
		t.Fatalf("performance_schema.accounts unexpectedly resolved to %v (must stay case-sensitive)", got)
	}
}

// Kind-1 injectivity: two distinct raw spellings that fold to one normalized key must fail closed
// instead of silently merging. Covers table, schema, and column levels, plus postgres (Fix C is not
// gated to MySQL).
func TestBulkMappingSchemaFoldCollisionFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		dialect string
		mapping *schema.Mapping
		wantErr string
	}{
		{
			name:    "table collision (lctn1)",
			dialect: mysqlLctn1,
			mapping: func() *schema.Mapping {
				sch := schema.NewMapping()
				u1 := schema.NewMapping()
				u1.Set("id", "BIGINT")
				u2 := schema.NewMapping()
				u2.Set("id", "BIGINT")
				sch.Set("Users", u1)
				sch.Set("users", u2)
				db := schema.NewMapping()
				db.Set("app", sch)
				cat := schema.NewMapping()
				cat.Set("def", db)
				return cat
			}(),
			wantErr: "duplicate normalized table",
		},
		{
			name:    "schema collision (lctn1)",
			dialect: mysqlLctn1,
			mapping: func() *schema.Mapping {
				t1 := schema.NewMapping()
				t1.Set("id", "BIGINT")
				s1 := schema.NewMapping()
				s1.Set("users", t1)
				t2 := schema.NewMapping()
				t2.Set("id", "BIGINT")
				s2 := schema.NewMapping()
				s2.Set("orders", t2)
				db := schema.NewMapping()
				db.Set("App", s1)
				db.Set("APP", s2)
				cat := schema.NewMapping()
				cat.Set("def", db)
				return cat
			}(),
			wantErr: "duplicate normalized schema",
		},
		{
			name:    "column collision (lctn0 still folds columns)",
			dialect: mysqlLctn0,
			mapping: mapping3("def", "App", "Users", [][2]string{{"Email", "VARCHAR"}, {"email", "VARCHAR"}}),
			wantErr: "duplicate normalized column",
		},
		{
			name:    "postgres column collision also fails closed",
			dialect: "postgres",
			mapping: mapping3("ridi", "public", "users", [][2]string{{"RRN", "VARCHAR"}, {"rrn", "VARCHAR"}}),
			wantErr: "duplicate normalized column",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := schema.NewMappingSchema(tc.mapping, tc.dialect, true)
			if err == nil {
				t.Fatalf("NewMappingSchema succeeded, want error %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
