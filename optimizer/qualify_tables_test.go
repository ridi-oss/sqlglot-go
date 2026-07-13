package optimizer_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
	"github.com/ridi/sqlglot-go/schema"
)

type failOnSearchPathLookupSchema struct {
	schema.Schema
}

func (failOnSearchPathLookupSchema) SupportedTableArgs() []string {
	panic("SupportedTableArgs called outside search-path mode")
}

func (failOnSearchPathLookupSchema) Find(exp.Expression, bool, bool) (*schema.Mapping, error) {
	panic("Find called outside search-path mode")
}

type errorOnSearchPathFindSchema struct {
	schema.Schema
	err error
}

func (errorOnSearchPathFindSchema) SupportedTableArgs() []string {
	return []string{"this", "db"}
}

func (s errorOnSearchPathFindSchema) Find(exp.Expression, bool, bool) (*schema.Mapping, error) {
	return nil, s.err
}

func TestQualifyTablesDirect(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		db      any
		catalog any
		want    string
	}{
		{
			name:    "pivot cte",
			sql:     "WITH cte AS (SELECT * FROM t) SELECT * FROM cte PIVOT(SUM(c) FOR v IN ('x', 'y'))",
			db:      "db",
			catalog: "catalog",
			want:    "WITH cte AS (SELECT * FROM catalog.db.t AS t) SELECT * FROM cte AS cte PIVOT(SUM(c) FOR v IN ('x', 'y')) AS _0",
		},
		{
			name:    "pivot cte explicit alias",
			sql:     "WITH cte AS (SELECT * FROM t) SELECT * FROM cte PIVOT(SUM(c) FOR v IN ('x', 'y')) AS pivot_alias",
			db:      "db",
			catalog: "catalog",
			want:    "WITH cte AS (SELECT * FROM catalog.db.t AS t) SELECT * FROM cte AS cte PIVOT(SUM(c) FOR v IN ('x', 'y')) AS pivot_alias",
		},
		{
			name:    "catalog without db",
			sql:     "select a from b",
			catalog: "catalog",
			want:    "SELECT a FROM b AS b",
		},
		{
			name: "quoted db",
			sql:  "select a from b",
			db:   `"DB"`,
			want: `SELECT a FROM "DB".b AS b`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expression, err := sqlglot.ParseOne(tt.sql, "")
			if err != nil {
				t.Fatalf("ParseOne: %v", err)
			}
			result := optimizer.QualifyTables(expression, tt.db, tt.catalog, "", false, nil, nil, nil)
			got, err := sqlglot.Generate(result, "", generator.Options{})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != tt.want {
				t.Fatalf("QualifyTables() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQualifyTablesSearchPath(t *testing.T) {
	mapping, err := schema.NewMappingSchema(schema.M(
		"s1", schema.M(
			"t1", schema.M("c", "INT"),
			"shared", schema.M("c", "INT"),
		),
		"s2", schema.M(
			"t2", schema.M("c", "INT"),
			"shared", schema.M("c", "INT"),
		),
	), nil, true)
	if err != nil {
		t.Fatalf("NewMappingSchema: %v", err)
	}
	if got := mapping.SupportedTableArgs(); len(got) != 2 || got[0] != "this" || got[1] != "db" {
		t.Fatalf("SupportedTableArgs() = %v, want [this db]", got)
	}

	qualify := func(t *testing.T, sql string, s any, searchPath []string, db, catalog any) exp.Expression {
		t.Helper()
		expression, err := sqlglot.ParseOne(sql, "")
		if err != nil {
			t.Fatalf("ParseOne: %v", err)
		}
		return optimizer.Qualify(expression, optimizer.QualifyOpts{
			Schema:     s,
			SearchPath: searchPath,
			DB:         db,
			Catalog:    catalog,
		})
	}

	soleTable := func(t *testing.T, expression exp.Expression) exp.Expression {
		t.Helper()
		tables := expression.FindAll(exp.KindTable)
		if len(tables) != 1 {
			t.Fatalf("table count = %d, want 1", len(tables))
		}
		return tables[0]
	}

	assertParts := func(t *testing.T, table exp.Expression, wantDB, wantCatalog string) {
		t.Helper()
		if got := table.DbName(); got != wantDB {
			t.Errorf("table %q db = %q, want %q", table.Name(), got, wantDB)
		}
		if got := table.CatalogName(); got != wantCatalog {
			t.Errorf("table %q catalog = %q, want %q", table.Name(), got, wantCatalog)
		}
	}

	for _, tt := range []struct {
		name       string
		sql        string
		searchPath []string
		catalog    any
		wantDB     string
	}{
		{name: "first candidate", sql: "SELECT * FROM t1", searchPath: []string{"s1", "s2"}, wantDB: "s1"},
		{name: "second candidate", sql: "SELECT * FROM t2", searchPath: []string{"s1", "s2"}, wantDB: "s2"},
		{name: "shared first candidate", sql: "SELECT * FROM shared", searchPath: []string{"s1", "s2"}, wantDB: "s1"},
		{name: "shared reversed path", sql: "SELECT * FROM shared", searchPath: []string{"s2", "s1"}, wantDB: "s2"},
		{name: "empty candidate is skipped", sql: "SELECT * FROM t2", searchPath: []string{"", "s2"}, wantDB: "s2"},
		{name: "catalog is not added", sql: "SELECT * FROM t1", searchPath: []string{"s1", "s2"}, catalog: "cat", wantDB: "s1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			table := soleTable(t, qualify(t, tt.sql, mapping, tt.searchPath, nil, tt.catalog))
			assertParts(t, table, tt.wantDB, "")
		})
	}

	t.Run("missing table remains unqualified", func(t *testing.T) {
		table := soleTable(t, qualify(t, "SELECT * FROM missing", mapping, []string{"s1", "s2"}, nil, nil))
		assertParts(t, table, "", "")
	})

	t.Run("missing table does not fall back to db or catalog", func(t *testing.T) {
		table := soleTable(t, qualify(t, "SELECT * FROM missing", mapping, []string{"s1", "s2"}, "legacy", "cat"))
		assertParts(t, table, "", "")
	})

	t.Run("only empty candidates do not fall back to db or catalog", func(t *testing.T) {
		table := soleTable(t, qualify(t, "SELECT * FROM t1", mapping, []string{"", ""}, "legacy", "cat"))
		assertParts(t, table, "", "")
	})

	t.Run("explicit db is preserved", func(t *testing.T) {
		table := soleTable(t, qualify(t, "SELECT * FROM s2.t1", mapping, []string{"s1", "s2"}, nil, nil))
		assertParts(t, table, "s2", "")
	})

	t.Run("cte reference is excluded", func(t *testing.T) {
		expression := qualify(t, "WITH cte AS (SELECT * FROM t2) SELECT * FROM cte", mapping, []string{"s1", "s2"}, nil, nil)
		tables := expression.FindAll(exp.KindTable)
		if len(tables) != 2 {
			t.Fatalf("table count = %d, want 2", len(tables))
		}
		byName := make(map[string]exp.Expression, len(tables))
		for _, table := range tables {
			byName[table.Name()] = table
		}
		physical := byName["t2"]
		if physical == nil {
			t.Fatal("physical table t2 not found")
		}
		cte := byName["cte"]
		if cte == nil {
			t.Fatal("cte table reference not found")
		}
		assertParts(t, physical, "s2", "")
		assertParts(t, cte, "", "")
	})

	t.Run("non-query root", func(t *testing.T) {
		table := soleTable(t, qualify(t, "DELETE FROM t1", mapping, []string{"s1", "s2"}, nil, nil))
		assertParts(t, table, "s1", "")
	})

	t.Run("flat schema does not prove db-qualified lookup", func(t *testing.T) {
		flat := schema.M("t1", schema.M("c", "INT"))
		table := soleTable(t, qualify(t, "SELECT * FROM t1", flat, []string{"s1"}, nil, nil))
		assertParts(t, table, "", "")
	})

	t.Run("unset schema does not prove db-qualified lookup", func(t *testing.T) {
		table := soleTable(t, qualify(t, "SELECT * FROM t1", nil, []string{"s1"}, nil, nil))
		assertParts(t, table, "", "")
	})

	t.Run("schema lookup error panics", func(t *testing.T) {
		lookupErr := errors.New("search-path lookup failed")
		defer func() {
			if got := recover(); got != lookupErr {
				t.Fatalf("panic = %v, want %v", got, lookupErr)
			}
		}()
		qualify(t, "SELECT * FROM t1", errorOnSearchPathFindSchema{err: lookupErr}, []string{"s1"}, nil, nil)
	})

	t.Run("empty search path preserves legacy qualification", func(t *testing.T) {
		expression := qualify(t, "SELECT * FROM t1", mapping, nil, "legacy", "cat")
		table := soleTable(t, expression)
		assertParts(t, table, "legacy", "cat")
		got, err := sqlglot.Generate(expression, "", generator.Options{})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if want := "SELECT * FROM cat.legacy.t1 AS t1"; got != want {
			t.Fatalf("Qualify() = %q, want %q", got, want)
		}
	})

	t.Run("empty search path does not inspect schema", func(t *testing.T) {
		expression := qualify(t, "SELECT * FROM t1", failOnSearchPathLookupSchema{}, nil, "legacy", "cat")
		assertParts(t, soleTable(t, expression), "legacy", "cat")
	})

	// The search-path probe is a 2-part Table{this, db} (no catalog), yet it must still resolve
	// against a 3-level catalog.schema.table mapping — stamping db by proven existence while
	// leaving catalog for the caller to supply. A depth-3 lineage consumer depends on this match;
	// if s.Find required the catalog level, searchPathMode would never stamp and the consumer
	// would silently over-deny. Locks the schema/db-only division documented in the PR review.
	t.Run("depth-3 schema matches db-only probe", func(t *testing.T) {
		depth3, err := schema.NewMappingSchema(schema.M(
			"cat", schema.M(
				"public", schema.M(
					"users", schema.M("rrn", "INT"),
				),
				"analytics", schema.M(
					"events", schema.M("id", "INT"),
				),
			),
		), nil, true)
		if err != nil {
			t.Fatalf("NewMappingSchema: %v", err)
		}
		if got := depth3.SupportedTableArgs(); len(got) != 3 || got[0] != "this" || got[1] != "db" || got[2] != "catalog" {
			t.Fatalf("SupportedTableArgs() = %v, want [this db catalog]", got)
		}

		searchPath := []string{"public", "analytics"}
		// First candidate schema in the path proves the table -> db stamped, catalog untouched.
		assertParts(t, soleTable(t, qualify(t, "SELECT * FROM users", depth3, searchPath, nil, nil)), "public", "")
		// Second candidate schema (search-path order is honored on a 3-level mapping too).
		assertParts(t, soleTable(t, qualify(t, "SELECT * FROM events", depth3, searchPath, nil, nil)), "analytics", "")
		// Unknown table stays fail-closed: neither db nor catalog is stamped.
		assertParts(t, soleTable(t, qualify(t, "SELECT * FROM missing", depth3, searchPath, nil, nil)), "", "")
	})
}

func TestQualifyTablesFixtures(t *testing.T) {
	for i, pair := range loadSQLFixturePairs(t, "qualify_tables.sql") {
		title := pair.Meta["title"]
		if title == "" {
			title = fmt.Sprintf("%d, %s", i+1, pair.SQL)
		}
		t.Run(title, func(t *testing.T) {
			if !dialectInScope(pair.Meta) {
				t.Skipf("deferred dialect: %s", pair.Meta["dialect"])
			}
			if reason := deferredQualifyTablesFixture(pair); reason != "" {
				t.Skipf("deferred: %s", reason)
			}

			dialect := pair.Meta["dialect"]
			expression, err := sqlglot.ParseOne(pair.SQL, dialect)
			if err != nil {
				t.Fatalf("ParseOne: %v", err)
			}
			result := optimizer.QualifyTables(expression, "db", "c", dialect, stringToBool(pair.Meta["canonicalize_table_aliases"]), nil, nil, nil)
			got, err := sqlglot.Generate(result, dialect, generator.Options{})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != pair.Expected {
				t.Fatalf("QualifyTables() = %q, want %q", got, pair.Expected)
			}
		})
	}
}

func deferredQualifyTablesFixture(pair sqlFixturePair) string {
	if pair.Meta["dialect"] == "postgres" {
		return "postgres function table-source needs GENERATE_SERIES/JSONB_TO_RECORDSET FUNCTIONS override — slice 5b"
	}
	switch pair.Meta["title"] {
	case "nested joins":
		return "parser does not yet accept chained join ON syntax"
	case "table with ordinality":
		return "parser does not yet accept function table sources with ordinality"
	case "alter table":
		return "ALTER TABLE parser support is deferred"
	}
	if strings.HasPrefix(pair.SQL, "COPY INTO ") {
		return "COPY INTO parser support is deferred"
	}
	return ""
}

func TestQualifyTablesCopiesTypedAliasColumns(t *testing.T) {
	original := exp.ColumnDef(exp.Args{
		"this": exp.ToIdentifier("rank", true),
		"kind": exp.DTypeInt.IntoExpr(nil),
	})
	table := exp.Table(exp.Args{
		"this": exp.Anonymous(exp.Args{
			"this":        "JSON_TO_RECORDSET",
			"expressions": []exp.Expression{exp.Column_("z", nil, nil, nil, nil)},
		}),
		"alias": exp.TableAlias(exp.Args{
			"this":    exp.ToIdentifier("y"),
			"columns": []exp.Expression{original},
		}),
	})
	expression := exp.Select(exp.Args{
		"expressions": []exp.Expression{exp.Star(exp.Args{})},
		"from_":       exp.From(exp.Args{"this": table}),
	})

	optimizer.QualifyTables(expression, nil, nil, "", true, nil, nil, nil)

	alias := expression.Find(exp.KindTable).Arg("alias").(exp.Expression)
	newColumn := alias.(*exp.Node).ExpressionsFor("columns")[0]
	if newColumn.Kind() != exp.KindColumnDef {
		t.Fatalf("alias column kind = %v, want ColumnDef", newColumn.Kind())
	}
	if original == newColumn {
		t.Fatalf("alias column was reused, want a copy")
	}
	originalSQL, err := sqlglot.Generate(original, "", generator.Options{})
	if err != nil {
		t.Fatalf("Generate original: %v", err)
	}
	newSQL, err := sqlglot.Generate(newColumn, "", generator.Options{})
	if err != nil {
		t.Fatalf("Generate copied: %v", err)
	}
	if originalSQL != newSQL {
		t.Fatalf("copied alias column SQL = %q, want %q", newSQL, originalSQL)
	}
}
