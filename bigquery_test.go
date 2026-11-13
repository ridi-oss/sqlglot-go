package sqlglot_test

import (
	"fmt"
	"strings"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
	"github.com/ridi-oss/sqlglot-go/schema"
)

func bigquerySchema() *schema.Mapping {
	M := schema.M
	return M(
		"my-proj", M(
			"analytics_123", M("events_*", M("user_pseudo_id", "STRING", "event_name", "STRING",
				"event_params", "ARRAY<STRUCT<key STRING, value STRUCT<string_value STRING, int_value INT64>>>")),
			"crm", M(
				"users", M("user_id", "INT64", "email", "STRING"),
				"orders", M("user_id", "INT64", "created_at", "TIMESTAMP", "internal_note", "STRING"),
			),
			"shop", M("purchases", M("name", "STRING", "items", "ARRAY<STRUCT<sku STRING, qty INT64>>",
				"buyer", "STRUCT<address STRUCT<country STRING>>")),
			"ds", M("Users", M("email", "STRING")),
		),
		"other-proj", M("dflt", M("t", M("x", "INT64"))),
	)
}

func bigqueryQualify(t *testing.T, sql string, opts optimizer.QualifyOpts) string {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, "bigquery")
	if err != nil {
		t.Fatalf("parse %q: %v", sql, err)
	}
	opts.Dialect = "bigquery"
	opts.Schema = bigquerySchema()
	got, err := sqlglot.Generate(optimizer.Qualify(e, opts), "bigquery", generator.Options{})
	if err != nil {
		t.Fatalf("generate %q: %v", sql, err)
	}
	return got
}

func TestBigQueryTableNameParts(t *testing.T) {
	cases := []struct{ sql, catalog, schema, table string }{
		{"SELECT * FROM my-proj-123.ds.t", "my-proj-123", "ds", "t"},
		{"SELECT * FROM `my-proj.ds.t`", "my-proj", "ds", "t"},
		{"SELECT * FROM `my-proj`.ds.`t`", "my-proj", "ds", "t"},
		{"SELECT * FROM `my-proj.ds.events_*`", "my-proj", "ds", "events_*"},
		{"SELECT * FROM ds.events_*", "", "ds", "events_*"},
		{"SELECT * FROM `region-us`.INFORMATION_SCHEMA.JOBS", "", "region-us", "INFORMATION_SCHEMA.JOBS"},
		{"SELECT * FROM p.ds.INFORMATION_SCHEMA.COLUMNS", "p", "ds", "INFORMATION_SCHEMA.COLUMNS"},
	}
	for _, c := range cases {
		e, err := sqlglot.ParseOne(c.sql, "bigquery")
		if err != nil {
			t.Fatalf("parse %q: %v", c.sql, err)
		}
		table := e.Find(exp.KindTable)
		if got := []string{table.Text("catalog"), table.Text("schema"), table.Name()}; got[0] != c.catalog || got[1] != c.schema || got[2] != c.table {
			t.Errorf("%q parts = %q, want [%s %s %s]", c.sql, got, c.catalog, c.schema, c.table)
		}
	}
}

func TestBigQueryFoldIdentifierName(t *testing.T) {
	d := dialects.BigQuery()
	if got := d.FoldIdentifierName("MyTable", true); got != "MyTable" {
		t.Errorf("table part folded to %q", got)
	}
	if got := d.FoldIdentifierName("MyCol", false); got != "mycol" {
		t.Errorf("column folded to %q, want mycol", got)
	}
}

func TestBigQueryQualifyDefaults(t *testing.T) {
	opts := optimizer.DefaultQualifyOpts()
	opts.Catalog = "my-proj"
	opts.DefaultSchema = "dflt"
	opts.DefaultSchemaCatalog = "other-proj"
	for sql, want := range map[string]string{
		// Unqualified -> defaultDataset (its own project); dataset-qualified -> job project.
		"SELECT x FROM t":                   "SELECT `t`.`x` AS `x` FROM `other-proj`.`dflt`.`t` AS `t`",
		"SELECT email FROM crm.users":       "SELECT `users`.`email` AS `email` FROM `my-proj`.`crm`.`users` AS `users`",
		"SELECT x FROM `other-proj.dflt.t`": "SELECT `t`.`x` AS `x` FROM `other-proj.dflt.t` AS `t`",
	} {
		if got := bigqueryQualify(t, sql, opts); got != want {
			t.Errorf("Qualify(%q)\n got: %s\nwant: %s", sql, got, want)
		}
	}
}

// DEVIATIONS §1.22: unqualified table names keep their case; CTE references still fold.
func TestBigQueryUnqualifiedTableNameCase(t *testing.T) {
	opts := optimizer.DefaultQualifyOpts()
	opts.Catalog = "my-proj"
	opts.DefaultSchema = "ds"
	for sql, want := range map[string]string{
		"SELECT email FROM Users":                       "SELECT `users`.`email` AS `email` FROM `my-proj`.`ds`.`Users` AS `users`",
		"WITH Cte AS (SELECT 1 AS x) SELECT x FROM cte": "WITH `cte` AS (SELECT 1 AS `x`) SELECT `cte`.`x` AS `x` FROM `cte` AS `cte`",
		"SELECT Email FROM `my-proj`.ds.Users AS U":     "SELECT `u`.`email` AS `email` FROM `my-proj`.`ds`.`Users` AS `u`",
	} {
		if got := bigqueryQualify(t, sql, opts); got != want {
			t.Errorf("Qualify(%q)\n got: %s\nwant: %s", sql, got, want)
		}
	}
}

// Expected outputs are pinned upstream v30.17.0 qualify() results.
func TestBigQueryQualifyMatchesUpstream(t *testing.T) {
	opts := optimizer.DefaultQualifyOpts()
	opts.Catalog = "my-proj"
	opts.DefaultSchema = "ds"
	for sql, want := range map[string]string{
		"SELECT user_pseudo_id, (SELECT value.string_value FROM UNNEST(event_params) WHERE key = 'page_location') AS page FROM `my-proj.analytics_123.events_*` WHERE _TABLE_SUFFIX BETWEEN '20261101' AND '20261107' AND event_name = 'page_view'": "SELECT `events_*`.`user_pseudo_id` AS `user_pseudo_id`, (SELECT `value`.`string_value` AS `string_value` FROM UNNEST(`events_*`.`event_params`) WHERE `key` = 'page_location') AS `page` FROM `my-proj.analytics_123.events_*` AS `events_*` WHERE `_table_suffix` BETWEEN '20261101' AND '20261107' AND `events_*`.`event_name` = 'page_view'",
		"SELECT item.sku, off FROM shop.purchases p, UNNEST(p.items) AS item WITH OFFSET AS off": "SELECT `item`.`sku` AS `sku`, `off` AS `off` FROM `my-proj`.`shop`.`purchases` AS `p` CROSS JOIN UNNEST(`p`.`items`) AS `item` WITH OFFSET AS off",
		"SELECT o.* EXCEPT (internal_note) FROM crm.orders o":                                    "SELECT `o`.`user_id` AS `user_id`, `o`.`created_at` AS `created_at` FROM `my-proj`.`crm`.`orders` AS `o`",
		"SELECT p.buyer.address.* FROM shop.purchases AS p":                                      "SELECT `p`.`buyer`.`address`.`country` AS `country` FROM `my-proj`.`shop`.`purchases` AS `p`",
		"SELECT * FROM shop.purchases AS p, p.items AS i":                                        "SELECT `p`.`name` AS `name`, `p`.`items` AS `items`, `p`.`buyer` AS `buyer`, `i` AS `i`, `sku` AS `sku`, `qty` AS `qty` FROM `my-proj`.`shop`.`purchases` AS `p` CROSS JOIN UNNEST(`p`.`items`) AS `i`",
	} {
		if got := bigqueryQualify(t, sql, opts); got != want {
			t.Errorf("Qualify(%q)\n got: %s\nwant: %s", sql, got, want)
		}
	}
}

func TestBigQueryPseudocolumn(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT event_name FROM `my-proj.analytics_123.events_*` WHERE _TABLE_SUFFIX > '1'", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = bigquerySchema()
	q := optimizer.Qualify(e, opts)
	if p := q.Find(exp.KindPseudocolumn); p == nil || p.Name() != "_table_suffix" {
		t.Fatalf("no _table_suffix Pseudocolumn in %s", q.ToS())
	}
}

func TestBigQueryRoundTrip(t *testing.T) {
	for sql, want := range map[string]string{
		`SELECT r'\d+'`:                        `SELECT '\\d+'`,
		`SELECT r"""a\"b"""`:                   `SELECT 'a\\"b'`,
		"SELECT SAFE.SUBSTR('foo', 0, -2)":     "SELECT SAFE.SUBSTR('foo', 0, -2)",
		"SELECT x FROM t WHERE x IN UNNEST(a)": "SELECT x FROM t WHERE x IN UNNEST(a)",
		"SELECT 1 UNION ALL SELECT 2":          "SELECT 1 UNION ALL SELECT 2",
		"SELECT 1 UNION DISTINCT SELECT 2":     "SELECT 1 UNION DISTINCT SELECT 2",
	} {
		e, err := sqlglot.ParseOne(sql, "bigquery")
		if err != nil {
			t.Fatalf("parse %q: %v", sql, err)
		}
		if got, _ := sqlglot.Generate(e, "bigquery", generator.Options{}); got != want {
			t.Errorf("%q -> %q, want %q", sql, got, want)
		}
	}
	// BigQuery set operations must spell out DISTINCT or ALL.
	if _, err := sqlglot.ParseOne("SELECT 1 UNION SELECT 2", "bigquery"); err == nil {
		t.Error("bare UNION parsed")
	}
}

func TestBigQueryASTShapes(t *testing.T) {
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = bigquerySchema()
	opts.Catalog = "my-proj"

	// GROUP BY keeps a Column (marked shadow only on an alias collision), not a bare Identifier.
	e, err := sqlglot.ParseOne("SELECT COUNT(*) FROM crm.users GROUP BY email", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	group := optimizer.Qualify(e, opts).Find(exp.KindGroup)
	if c := group.Find(exp.KindColumn); c == nil || c.Text("table") != "users" {
		t.Errorf("GROUP BY lost its qualified column: %s", group.ToS())
	}

	// A bare date part is a Var unit, never a column reference.
	e, err = sqlglot.ParseOne("SELECT TIMESTAMP_TRUNC(created_at, DAY), DATE_DIFF(created_at, created_at, hour) FROM crm.orders", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	q := optimizer.Qualify(e, opts)
	for _, sel := range q.Selects() {
		if unit, _ := sel.This().Arg("unit").(exp.Expression); unit == nil || unit.Kind() != exp.KindVar {
			t.Errorf("unit is not a Var: %s", sel.ToS())
		}
	}

	// A renamed struct in a derived table expands to the fields of the source column it renames.
	got := bigqueryQualify(t, "SELECT c.s.* FROM (SELECT buyer AS s FROM shop.purchases) AS c", opts)
	want := "SELECT `c`.`s`.`address` AS `address` FROM (SELECT `purchases`.`buyer` AS `s` FROM `my-proj`.`shop`.`purchases` AS `purchases`) AS `c`"
	if got != want {
		t.Errorf("renamed struct star\n got: %s\nwant: %s", got, want)
	}
}

func TestSHA1KeepsNameOutsideBigQuery(t *testing.T) {
	for _, d := range []string{"presto", "trino", "athena", "bigquery"} {
		e, err := sqlglot.ParseOne("SELECT SHA1(x)", d)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := sqlglot.Generate(e, d, generator.Options{}); got != "SELECT SHA1(x)" {
			t.Errorf("%s: %s", d, got)
		}
	}
}

// Callers that normalize before Qualify (proxy-monster does) keep the table-name case too.
func TestBigQueryNormalizeKeepsTableNameCase(t *testing.T) {
	M := schema.M
	s := M("p", M("Sales", M("Users", M("id", "INT64", "email", "STRING"), "users", M("id", "INT64"))))
	e, err := sqlglot.ParseOne("SELECT email FROM Users", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	e = optimizer.NormalizeIdentifiers(e, "bigquery")
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = s
	opts.Catalog = "p"
	opts.DefaultSchema = "Sales"
	got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), "bigquery", generator.Options{})
	if want := "SELECT `users`.`email` AS `email` FROM `p`.`Sales`.`Users` AS `users`"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// DEVIATIONS §1.24: _DBT_MAX_PARTITION is an ordinary column with a source.
func TestBigQueryDbtMaxPartitionIsAColumn(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT _DBT_MAX_PARTITION FROM t", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = schema.M("t", schema.M("_dbt_max_partition", "DATE"))
	q := optimizer.Qualify(e, opts)
	if q.Find(exp.KindPseudocolumn) != nil {
		t.Fatalf("became a Pseudocolumn: %s", q.ToS())
	}
	if c := q.Find(exp.KindColumn); c == nil || c.Text("table") != "t" {
		t.Errorf("not a qualified column: %s", q.ToS())
	}
}

// NormalizeIdentifiers alone: a CTE reference folds, a same-named table in that CTE's own body
// (or before the CTE is defined) is the physical table and keeps its case.
func TestBigQueryNormalizeCTEVisibility(t *testing.T) {
	for sql, want := range map[string]string{
		"SELECT F(X).Field.g(), Sales.upper(X)":                                    "SELECT F(x).field.g(), Sales.upper(x)",
		"WITH Foo AS (SELECT 1) SELECT * FROM FOO":                                 "WITH foo AS (SELECT 1) SELECT * FROM foo",
		"WITH Users AS (SELECT * FROM Users) SELECT * FROM Users":                  "WITH users AS (SELECT * FROM Users) SELECT * FROM users",
		"WITH a AS (SELECT * FROM B), B AS (SELECT 1) SELECT * FROM a, B":          "WITH a AS (SELECT * FROM B), b AS (SELECT 1) SELECT * FROM a CROSS JOIN b",
		"WITH RECURSIVE R AS (SELECT 1 UNION ALL SELECT * FROM R) SELECT * FROM R": "WITH RECURSIVE r AS (SELECT 1 UNION ALL SELECT * FROM r) SELECT * FROM r",
	} {
		e, err := sqlglot.ParseOne(sql, "bigquery")
		if err != nil {
			t.Fatal(err)
		}
		got, _ := sqlglot.Generate(optimizer.NormalizeIdentifiers(e, "bigquery"), "bigquery", generator.Options{})
		if got != want {
			t.Errorf("%s\n got: %s\nwant: %s", sql, got, want)
		}
	}
}

// DEVIATIONS §1.25: a FROM path whose first name is an enclosing query's source is an implicit
// UNNEST of that source's column; a CTE body does not see the outer query.
func TestBigQueryCorrelatedImplicitUnnest(t *testing.T) {
	for sql, want := range map[string]string{
		"SELECT (SELECT STRING_AGG(x) FROM Public.tags AS x) FROM Sales.Users AS Public":                                          "SELECT (SELECT STRING_AGG(x) FROM UNNEST(Public.tags) AS x) FROM Sales.Users AS Public",
		"SELECT * FROM Sales.Users AS u WHERE EXISTS(SELECT 1 FROM u.tags AS t WHERE t = 'a')":                                    "SELECT * FROM Sales.Users AS u WHERE EXISTS(SELECT 1 FROM UNNEST(u.tags) AS t WHERE t = 'a')",
		"SELECT * FROM Sales.Users AS u WHERE EXISTS(SELECT 1 FROM Other.t)":                                                      "SELECT * FROM Sales.Users AS u WHERE EXISTS(SELECT 1 FROM Other.t)",
		"WITH c AS (SELECT * FROM ds.t) SELECT * FROM c, x AS ds":                                                                 "WITH c AS (SELECT * FROM ds.t) SELECT * FROM c CROSS JOIN x AS ds",
		"SELECT (WITH c AS (SELECT * FROM o.arr) SELECT 1 FROM c) FROM t AS o":                                                    "SELECT (WITH c AS (SELECT * FROM o.arr) SELECT 1 FROM c) FROM t AS o",
		"DELETE FROM Sales.Users AS u WHERE EXISTS(SELECT 1 FROM U.tags AS t)":                                                    "DELETE FROM Sales.Users AS u WHERE EXISTS(SELECT 1 FROM UNNEST(U.tags) AS t)",
		"UPDATE Sales.Users AS u SET id = (SELECT COUNT(t) FROM u.tags AS t) WHERE TRUE":                                          "UPDATE Sales.Users AS u SET id = (SELECT COUNT(t) FROM UNNEST(u.tags) AS t) WHERE TRUE",
		"SELECT * FROM ds.users AS o, (SELECT * FROM o.arr) AS x":                                                                 "SELECT * FROM ds.users AS o CROSS JOIN (SELECT * FROM o.arr) AS x",
		"SELECT (SELECT 1 FROM ds.other JOIN o.arr AS a ON TRUE) FROM ds.t AS o":                                                  "SELECT (SELECT 1 FROM ds.other JOIN UNNEST(o.arr) AS a ON TRUE) FROM ds.t AS o",
		"SELECT ARRAY(SELECT x FROM o.arr AS x) FROM ds.t AS o":                                                                   "SELECT ARRAY(SELECT x FROM UNNEST(o.arr) AS x) FROM ds.t AS o",
		"SELECT 1 FROM Sales.Users AS u UNION ALL SELECT 1 FROM u.tags":                                                           "SELECT 1 FROM Sales.Users AS u UNION ALL SELECT 1 FROM u.tags",
		"MERGE Sales.Users AS u USING (SELECT t FROM u.tags AS t) AS s ON FALSE WHEN NOT MATCHED THEN INSERT ROW":                 "MERGE INTO Sales.Users AS u USING (SELECT t FROM u.tags AS t) AS s ON FALSE WHEN NOT MATCHED THEN INSERT ROW",
		"MERGE Sales.Users AS u USING ds.s AS s ON EXISTS(SELECT 1 FROM u.tags) WHEN MATCHED THEN DELETE":                         "MERGE INTO Sales.Users AS u USING ds.s AS s ON EXISTS(SELECT 1 FROM UNNEST(u.tags) AS tags) WHEN MATCHED THEN DELETE",
		"MERGE Sales.Users AS u USING ds.s AS s ON FALSE WHEN NOT MATCHED AND EXISTS(SELECT 1 FROM u.tags) THEN INSERT ROW":       "MERGE INTO Sales.Users AS u USING ds.s AS s ON FALSE WHEN NOT MATCHED AND EXISTS(SELECT 1 FROM u.tags) THEN INSERT ROW",
		"MERGE Sales.Users AS u USING ds.s AS s ON FALSE WHEN NOT MATCHED BY SOURCE AND EXISTS(SELECT 1 FROM u.tags) THEN DELETE": "MERGE INTO Sales.Users AS u USING ds.s AS s ON FALSE WHEN NOT MATCHED BY SOURCE AND EXISTS(SELECT 1 FROM UNNEST(u.tags) AS tags) THEN DELETE",
		"SELECT t FROM Sales.Users AS u, UNNEST(ARRAY(SELECT x FROM u.tags AS x)) AS t":                                           "SELECT t FROM Sales.Users AS u CROSS JOIN UNNEST(ARRAY(SELECT x FROM UNNEST(u.tags) AS x)) AS t",
		"SELECT 1 FROM ds.a JOIN ds.b ON (SELECT COUNT(*) FROM later.tags) > 0 JOIN ds.c AS later ON TRUE":                        "SELECT 1 FROM ds.a JOIN ds.b ON (SELECT COUNT(*) FROM later.tags) > 0 JOIN ds.c AS later ON TRUE",
		"SELECT 1 FROM ds.c AS c JOIN ds.b ON (SELECT COUNT(*) FROM c.tags) > 0":                                                  "SELECT 1 FROM ds.c AS c JOIN ds.b ON (SELECT COUNT(*) FROM UNNEST(c.tags) AS tags) > 0",
		"SELECT x FROM Sales.Users AS u JOIN u.profile.nested.tags AS x ON TRUE":                                                  "SELECT x FROM Sales.Users AS u JOIN UNNEST(u.profile.nested.tags) AS x ON TRUE",
		"SELECT (SELECT 1 FROM u.a.b.c) FROM Sales.Users AS u":                                                                    "SELECT (SELECT 1 FROM UNNEST(u.a.b.c) AS c) FROM Sales.Users AS u",
		"SELECT 1 FROM ds.t AS u JOIN ds.f((SELECT s FROM u.tags LIMIT 1)) AS x ON TRUE":                                          "SELECT 1 FROM ds.t AS u JOIN ds.f((SELECT s FROM u.tags LIMIT 1)) AS x ON TRUE",
		"DELETE ds.t AS t WHERE EXISTS(SELECT 1 FROM t.arr)":                                                                      "DELETE ds.t AS t WHERE EXISTS(SELECT 1 FROM UNNEST(t.arr) AS arr)",
		"SELECT * FROM (SELECT x FROM a.tags AS x) AS t, ds.a AS a":                                                               "SELECT * FROM (SELECT x FROM a.tags AS x) AS t CROSS JOIN ds.a AS a",
		"SELECT * FROM ds.a AS a, UNNEST(ARRAY(SELECT x FROM c.tags AS x)) AS t, ds.c AS c":                                       "SELECT * FROM ds.a AS a CROSS JOIN UNNEST(ARRAY(SELECT x FROM c.tags AS x)) AS t CROSS JOIN ds.c AS c",
		"SELECT * FROM ds.a AS a, UNNEST(ARRAY(SELECT y FROM x.tags AS y)) AS x":                                                  "SELECT * FROM ds.a AS a CROSS JOIN UNNEST(ARRAY(SELECT y FROM x.tags AS y)) AS x",
		"SELECT 1 FROM ds.a JOIN ds.b AS b ON (SELECT COUNT(*) FROM b.tags) > 0":                                                  "SELECT 1 FROM ds.a JOIN ds.b AS b ON (SELECT COUNT(*) FROM UNNEST(b.tags) AS tags) > 0",
		"MERGE Sales.Users AS u USING ds.s AS s ON FALSE WHEN NOT MATCHED BY SOURCE AND EXISTS(SELECT 1 FROM s.arr) THEN DELETE":  "MERGE INTO Sales.Users AS u USING ds.s AS s ON FALSE WHEN NOT MATCHED BY SOURCE AND EXISTS(SELECT 1 FROM s.arr) THEN DELETE",
		"UPDATE ds.t AS t SET x = 1 FROM (SELECT * FROM t.arr) AS s WHERE TRUE":                                                   "UPDATE ds.t AS t SET x = 1 FROM (SELECT * FROM t.arr) AS s WHERE TRUE",
		"UPDATE ds.t AS t SET x = 1 FROM ds.a AS a WHERE EXISTS(SELECT 1 FROM a.arr)":                                             "UPDATE ds.t AS t SET x = 1 FROM ds.a AS a WHERE EXISTS(SELECT 1 FROM UNNEST(a.arr) AS arr)",
		"DELETE FROM ds.t AS u USING (SELECT * FROM u.arr) AS s WHERE TRUE":                                                       "DELETE FROM ds.t AS u USING (SELECT * FROM u.arr) AS s WHERE TRUE",
		"SELECT (SELECT 1 FROM `u.tags`) FROM Sales.Users AS u":                                                                   "SELECT (SELECT 1 FROM `u.tags`) FROM Sales.Users AS u",
		"SELECT t FROM Sales.Users AS u JOIN u.tags AS t ON TRUE":                                                                 "SELECT t FROM Sales.Users AS u JOIN UNNEST(u.tags) AS t ON TRUE",
		"SELECT * FROM ds.users AS u JOIN u.f() AS x ON TRUE":                                                                     "SELECT * FROM ds.users AS u JOIN u.f() AS x ON TRUE",
		"SELECT * FROM Sales.Users AS u JOIN u.tags AS t USING (x)":                                                               "SELECT * FROM Sales.Users AS u JOIN UNNEST(u.tags) AS t USING (x)",
		"SELECT * FROM ds.x AS Sales JOIN Sales.Users ON TRUE":                                                                    "SELECT * FROM ds.x AS Sales JOIN UNNEST(Sales.Users) AS Users ON TRUE",
		"SELECT * FROM Sales.Users JOIN Sales.Orders ON TRUE":                                                                     "SELECT * FROM Sales.Users JOIN Sales.Orders ON TRUE",
		"SELECT * FROM ds.x AS Sales RIGHT JOIN Sales.Users AS u ON TRUE":                                                         "SELECT * FROM ds.x AS Sales RIGHT JOIN Sales.Users AS u ON TRUE",
		"SELECT * FROM ds.x AS Sales FULL JOIN Sales.Users AS u ON TRUE":                                                          "SELECT * FROM ds.x AS Sales FULL JOIN Sales.Users AS u ON TRUE",
		"SELECT t FROM Sales.Users JOIN users.tags AS t ON TRUE":                                                                  "SELECT t FROM Sales.Users JOIN UNNEST(users.tags) AS t ON TRUE",
		"SELECT t FROM UNNEST(a) AS u JOIN u.tags AS t ON TRUE":                                                                   "SELECT t FROM UNNEST(a) AS u JOIN UNNEST(u.tags) AS t ON TRUE",
		"SELECT i FROM Sales.Users AS u, u.orders JOIN orders.items AS i ON TRUE":                                                 "SELECT i FROM Sales.Users AS u CROSS JOIN UNNEST(u.orders) AS orders JOIN UNNEST(orders.items) AS i ON TRUE",
		"SELECT t FROM Sales.Users AS u JOIN `u.tags` AS t ON TRUE":                                                               "SELECT t FROM Sales.Users AS u JOIN `u.tags` AS t ON TRUE",
		"SELECT t FROM Sales.Users AS u LEFT JOIN u.tags AS t ON t <> 'x'":                                                        "SELECT t FROM Sales.Users AS u LEFT JOIN UNNEST(u.tags) AS t ON t <> 'x'",
		"SELECT * FROM Sales.Users AS u, `u.tags`":                                                                                "SELECT * FROM Sales.Users AS u CROSS JOIN `u.tags`",
		"SELECT * FROM Sales.Users AS u WHERE EXISTS(SELECT 1 FROM ds.a, (SELECT (SELECT 1 FROM u.tbl) FROM ds.b) AS d)":          "SELECT * FROM Sales.Users AS u WHERE EXISTS(SELECT 1 FROM ds.a CROSS JOIN (SELECT (SELECT 1 FROM UNNEST(u.tbl) AS tbl) FROM ds.b) AS d)",
		"SELECT (SELECT 1 FROM o.items) FROM Sales.Users AS u, u.orders AS o":                                                     "SELECT (SELECT 1 FROM UNNEST(o.items) AS items) FROM Sales.Users AS u CROSS JOIN UNNEST(u.orders) AS o",
		"DELETE Sales.Target AS t FROM Sales.Target AS t JOIN Sales.Users AS u ON t.id = u.id WHERE EXISTS(SELECT 1 FROM u.tags)": "DELETE Sales.Target AS t FROM Sales.Target AS t JOIN Sales.Users AS u ON t.id = u.id WHERE EXISTS(SELECT 1 FROM UNNEST(u.tags) AS tags)",
	} {
		statements, err := sqlglot.Parse(sql, "bigquery")
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := sqlglot.Generate(statements[0], "bigquery", generator.Options{}); got != want {
			t.Errorf("%s\n got: %s\nwant: %s", sql, got, want)
		}
	}

	// Lineage: the subquery reads Users.tags.
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = schema.M("Sales", schema.M("Users", schema.M("id", "INT64", "tags", "ARRAY<STRING>")))
	e, err := sqlglot.ParseOne("SELECT (SELECT STRING_AGG(x) FROM Public.tags AS x) FROM Sales.Users AS Public", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	q := optimizer.Qualify(e, opts)
	got, _ := sqlglot.Generate(q, "bigquery", generator.Options{})
	if want := "SELECT (SELECT STRING_AGG(`x`) AS `_col_0` FROM UNNEST(`public`.`tags`) AS `x`) AS `_col_0` FROM `Sales`.`Users` AS `public`"; got != want {
		t.Errorf("qualify\n got: %s\nwant: %s", got, want)
	}
	// The unnested column is tags of the outer source public, which is Sales.Users.
	column := q.Find(exp.KindUnnest).Find(exp.KindColumn)
	if column.Name() != "tags" || column.Text("table") != "public" {
		t.Errorf("unnest column: %s", column.ToS())
	}
	if source := q.Find(exp.KindFrom).This(); source.AliasOrName() != "public" || source.Name() != "Users" {
		t.Errorf("outer source: %s", source.ToS())
	}
}

// Date parts and NORMALIZE forms are keywords, never column references.
func TestBigQueryDatePartsAreVars(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT TIME_TRUNC(t, HOUR), TIME_DIFF(a, b, MINUTE), NORMALIZE(x, NFKC), NORMALIZE_AND_CASEFOLD(x, NFD) FROM z", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	for _, sel := range e.Selects() {
		arg := sel.Arg("unit")
		if arg == nil {
			arg = sel.Arg("form")
		}
		if v, _ := arg.(exp.Expression); v == nil || v.Kind() != exp.KindVar {
			t.Errorf("not a Var: %s", sel.ToS())
		}
	}
}

// MySQL TIMESTAMPDIFF takes the unit first; both operands stay columns.
func TestMySQLTimestampDiffKeepsColumns(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT TIMESTAMPDIFF(MINUTE, a, b) FROM t", "mysql")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(e.FindAll(exp.KindColumn)); got != 2 {
		t.Errorf("found %d columns: %s", got, e.ToS())
	}
	if got, _ := sqlglot.Generate(e, "mysql", generator.Options{}); got != "SELECT TIMESTAMPDIFF(MINUTE, a, b) FROM t" {
		t.Errorf("round trip: %s", got)
	}
}

// u.s.arr after u reads column s of u, then field arr, even when another source is named s.
func TestBigQueryRangeVariablePathShape(t *testing.T) {
	for _, sql := range []string{
		"SELECT x FROM ds.users AS u JOIN ds.other AS s ON TRUE JOIN u.s.arr AS x ON TRUE",
		"SELECT x FROM ds.users AS u, u.s.arr AS x",
	} {
		e, err := sqlglot.ParseOne(sql, "bigquery")
		if err != nil {
			t.Fatal(err)
		}
		column := e.Find(exp.KindUnnest).Find(exp.KindColumn)
		if column == nil || column.Text("table") != "u" || column.Name() != "s" || column.Text("schema") != "" {
			t.Errorf("%s: unnest reads %s", sql, e.Find(exp.KindUnnest).ToS())
		}
	}
}

// The implicit alias of a converted path is its last name, so `tags` binds the element.
func TestBigQueryImplicitUnnestAlias(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT tags FROM Sales.Users AS u JOIN u.tags ON TRUE", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = schema.M("Sales", schema.M("Users", schema.M("tags", "ARRAY<STRING>")))
	got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), "bigquery", generator.Options{})
	if want := "SELECT `tags` AS `tags` FROM `Sales`.`Users` AS `u` JOIN UNNEST(`u`.`tags`) AS `tags` ON TRUE"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// A non-date-part argument of a date-unit function name stays a column, and MySQL/Hive have no
// builtin NORMALIZE: real column reads stay reads.
func TestUDFArgumentsStayColumns(t *testing.T) {
	for _, c := range []struct{ dialect, sql string }{
		{"mysql", "SELECT NORMALIZE(a, secret), TIME_DIFF(a, secret), TIME_TRUNC(a, secret), DATETIME_TRUNC(a, secret), LAST_DAY(a, secret) FROM t"},
		{"postgres", "SELECT DATE_DIFF(a, b, d) FROM t"},
		{"hive", "SELECT NORMALIZE(a, secret) FROM t"},
		{"postgres", "SELECT DATE_DIFF(a, b, secret) FROM t"},
		{"bigquery", "SELECT DATE_DIFF(a, b, secret) FROM t"},
		{"postgres", "SELECT TIMESTAMPDIFF(MONTH, a, secret) FROM t"},
	} {
		e, err := sqlglot.ParseOne(c.sql, c.dialect)
		if err != nil {
			t.Fatal(err)
		}
		reads := 0
		for _, col := range e.FindAll(exp.KindColumn) {
			if col.Name() == "secret" {
				reads++
			}
		}
		if c.dialect == "postgres" && strings.Contains(c.sql, ", d)") {
			if len(e.FindAll(exp.KindColumn)) != 3 {
				t.Errorf("%s: abbreviation d became a unit: %s", c.sql, e.ToS())
			}
			continue
		}
		if want := strings.Count(c.sql, "secret"); reads != want {
			t.Errorf("%s %s: %d column reads of secret, want %d", c.dialect, c.sql, reads, want)
		}
	}
}

// In MySQL the BigQuery date functions are plain calls (user functions): every argument, including
// one named like a date part, stays a column, and the call stays Anonymous for function checks.
func TestMySQLBigQueryDateNamesArePlainCalls(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT TIME_TRUNC(a, day), DATETIME_DIFF(a, b, day), TIME_DIFF(a, b), TIMESTAMP_ADD(a, b) FROM t", "mysql")
	if err != nil {
		t.Fatal(err)
	}
	for _, sel := range e.Selects() {
		if sel.Kind() != exp.KindAnonymous {
			t.Errorf("not a plain call: %s", sel.ToS())
		}
	}
	if got := len(e.FindAll(exp.KindColumn)); got != 9 {
		t.Errorf("found %d column reads, want 9: %s", got, e.ToS())
	}
}

// A JOIN's ON binds only to sources up to that join: a later UNNEST alias does not shadow it.
func TestBigQueryOnBindsBeforeLaterUnnest(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT 1 FROM ds.t AS t JOIN (SELECT 1 AS id) AS b ON secret IS NOT NULL CROSS JOIN UNNEST([1]) AS secret", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = schema.M("ds", schema.M("t", schema.M("secret", "STRING")))
	q := optimizer.Qualify(e, opts)
	on := q.Find(exp.KindJoin).Arg("on").(exp.Expression)
	if c := on.Find(exp.KindColumn); c == nil || c.Text("table") != "t" {
		t.Errorf("ON column bound to %s, want t", on.ToS())
	}
}

// Two UNNESTs with the same alias stay ambiguous rather than picking one at random.
func TestBigQueryDuplicateUnnestAliasAmbiguous(t *testing.T) {
	for i := 0; i < 20; i++ {
		e, err := sqlglot.ParseOne("SELECT x FROM ds.t AS t, UNNEST(t.a) AS x, UNNEST(t.b) AS x", "bigquery")
		if err != nil {
			t.Fatal(err)
		}
		opts := optimizer.DefaultQualifyOpts()
		opts.Dialect = "bigquery"
		opts.Schema = schema.M("ds", schema.M("t", schema.M("a", "ARRAY<INT64>", "b", "ARRAY<INT64>")))
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "could not be resolved") {
					t.Fatalf("want an unresolved-column error, got %v", r)
				}
			}()
			optimizer.Qualify(e, opts)
		}()
	}
}

// Every BigQuery date part is a unit keyword, never a column reference.
func TestBigQueryAllDatePartsAreVars(t *testing.T) {
	for _, part := range []string{"MICROSECOND", "MILLISECOND", "SECOND", "MINUTE", "HOUR", "DAY", "DAYOFWEEK", "DAYOFYEAR", "WEEK", "ISOWEEK", "MONTH", "QUARTER", "YEAR", "ISOYEAR", "DATE", "DATETIME", "TIME", "NANOSECOND", "EPOCH", "DECADE", "CENTURY", "MILLENNIUM"} {
		e, err := sqlglot.ParseOne("SELECT DATE_DIFF(a, b, "+part+"), DATE_TRUNC(a, "+part+") FROM t", "bigquery")
		if err != nil {
			t.Fatal(err)
		}
		for _, sel := range e.Selects() {
			if sel.Find(exp.KindColumn) != nil && len(sel.FindAll(exp.KindColumn)) > 2 {
				t.Errorf("%s: unit became a column: %s", part, sel.ToS())
			}
		}
		if got := len(e.FindAll(exp.KindColumn)); got != 3 {
			t.Errorf("%s: %d columns, want 3 (a, b, a)", part, got)
		}
	}
	// A non-date-part bare name stays a column in DATE_TRUNC too.
	e, _ := sqlglot.ParseOne("SELECT DATE_TRUNC(x, secret) FROM t", "bigquery")
	if len(e.FindAll(exp.KindColumn)) != 2 {
		t.Errorf("DATE_TRUNC dropped secret: %s", e.ToS())
	}
}

// The BigQuery-only date names are plain calls in MySQL: all 14 stay Anonymous.
func TestMySQLAllBigQueryDateNamesArePlainCalls(t *testing.T) {
	for _, name := range []string{"TIME_DIFF", "TIME_TRUNC", "TIMESTAMP_DIFF", "DATETIME_DIFF", "DATETIME_TRUNC", "DATETIME_ADD", "DATETIME_SUB", "TIME_ADD", "TIME_SUB", "TIMESTAMP_ADD", "TIMESTAMP_SUB", "LAST_DAY", "DATE_SUB"} {
		e, err := sqlglot.ParseOne("SELECT "+name+"(a, b) FROM t", "mysql")
		if err != nil {
			t.Fatal(err)
		}
		if sel := e.Selects()[0]; sel.Kind() != exp.KindAnonymous {
			t.Errorf("%s is %s, want Anonymous", name, exp.ClassName(sel.Kind()))
		}
	}
}

// Outside BigQuery an UNNEST alias does not shadow a same-named column: x stays ambiguous.
func TestNonBigQueryKeepsAmbiguity(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT x FROM t, UNNEST(t.arr) AS u(x)", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "postgres"
	opts.Schema = schema.M("t", schema.M("x", "INT", "arr", "INT[]"))
	defer func() {
		if recover() == nil {
			t.Error("ambiguous x resolved")
		}
	}()
	optimizer.Qualify(e, opts)
}

// With the column in two sources before the join, a later UNNEST of that name does not win: the
// ON column is ambiguous (BigQuery rejects it too), never bound to the UNNEST.
func TestBigQueryOnAmbiguousNotLaterUnnest(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT 1 FROM ds.t AS t JOIN ds.b AS b ON secret IS NOT NULL CROSS JOIN UNNEST([1]) AS secret", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = schema.M("ds", schema.M("t", schema.M("secret", "STRING"), "b", schema.M("secret", "STRING")))
	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "could not be resolved") {
			t.Errorf("want an unresolved-column error, got %v", r)
		}
	}()
	optimizer.Qualify(e, opts)
}

// Columns in parenthesized joins still resolve in an ON.
func TestBigQueryParenthesizedJoinOn(t *testing.T) {
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = schema.M("ds", schema.M("a", schema.M("acol", "INT64"), "b", schema.M("bcol", "INT64"), "c", schema.M("ccol", "INT64")))
	for _, sql := range []string{
		"SELECT 1 FROM (ds.a AS a JOIN ds.b AS b ON acol = bcol) JOIN ds.c AS c ON ccol = bcol",
		"SELECT 1 FROM ds.a AS a JOIN (ds.b AS b JOIN ds.c AS c ON ccol = bcol) ON acol = bcol",
	} {
		e, err := sqlglot.ParseOne(sql, "bigquery")
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: %v", sql, r)
				}
			}()
			optimizer.Qualify(e, opts)
		}()
	}
}

// Athena/Presto DATE_TRUNC with a column unit keeps the column read.
func TestAthenaDateTruncColumnUnit(t *testing.T) {
	for _, d := range []string{"athena", "presto", "trino"} {
		e, err := sqlglot.ParseOne("SELECT DATE_TRUNC(unit_col, ts) FROM t", d)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(e.FindAll(exp.KindColumn)); got != 2 {
			t.Errorf("%s: %d columns, want 2: %s", d, got, e.ToS())
		}
		opts := optimizer.DefaultQualifyOpts()
		opts.Dialect = d
		opts.Schema = schema.M("t", schema.M("unit_col", "VARCHAR", "ts", "TIMESTAMP"))
		for _, col := range optimizer.Qualify(e, opts).FindAll(exp.KindColumn) {
			if col.Text("table") != "t" {
				t.Errorf("%s: %s bound to %q", d, col.Name(), col.Text("table"))
			}
		}
	}
	e, err := sqlglot.ParseOne("SELECT TRUNC(ts, unit_col) FROM t", "hive")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(e.FindAll(exp.KindColumn)); got != 2 {
		t.Errorf("hive: %d columns, want 2: %s", got, e.ToS())
	}
	if got, _ := sqlglot.Generate(e, "hive", generator.Options{}); got != "SELECT TRUNC(ts, unit_col) FROM t" {
		t.Errorf("hive: generated %s", got)
	}
	// Trino and Hive take the unit as a string, so even a bare date-part name is a column.
	for _, c := range [][2]string{
		{"trino", "SELECT DATE_TRUNC(day, ts) FROM t"},
		{"athena", "SELECT DATE_TRUNC(day, CAST(ts AS DATE)) FROM t"},
		{"hive", "SELECT TRUNC(ts, month) FROM t"},
		{"trino", "SELECT DATE_ADD(month, 1, ts) FROM t"},
		{"trino", "SELECT DATE_DIFF(day, ts, ts) FROM t"},
		{"postgres", "SELECT DATE_PART(unit_col, ts) FROM t"},
	} {
		e, err := sqlglot.ParseOne(c[1], c[0])
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := sqlglot.Generate(e, c[0], generator.Options{}); got != c[1] {
			t.Errorf("%s: generated %s", c[1], got)
		}
		if got := len(e.FindAll(exp.KindColumn)); got < 2 {
			t.Errorf("%s: bare date-part column: %d columns, want 2: %s", c[1], got, e.ToS())
		}
	}
	e, err = sqlglot.ParseOne("SELECT TRUNC(ts, CONCAT(fmt, '')) FROM t", "hive")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := sqlglot.Generate(e, "hive", generator.Options{}); got != "SELECT TRUNC(ts, CONCAT(fmt, '')) FROM t" {
		t.Errorf("hive: generated %s", got)
	}
	e, err = sqlglot.ParseOne("SELECT TIMESTAMP_TRUNC(ts, WEEK(MONDAY)) FROM t", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	for d, want := range map[string]string{"trino": "SELECT DATE_TRUNC('WEEK', ts) FROM t", "hive": "SELECT TRUNC(ts, 'WEEK') FROM t"} {
		if got, _ := sqlglot.Generate(e, d, generator.Options{}); got != want {
			t.Errorf("WEEK(MONDAY) to %s: %s", d, got)
		}
	}
	for _, d := range []string{"trino", "bigquery"} {
		e, err := sqlglot.ParseOne(`SELECT DATE_TRUNC("day", ts) FROM t`, d)
		if d == "bigquery" {
			e, err = sqlglot.ParseOne("SELECT DATE_TRUNC(ts, `day`) FROM t", d)
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := len(e.FindAll(exp.KindColumn)); got != 2 {
			t.Errorf("%s: quoted unit: %d columns, want 2: %s", d, got, e.ToS())
		}
	}
}

// In an ON, a column never binds to an UNNEST alias that also names an earlier column, in any
// join nesting: it binds to the table or fails.
func TestBigQueryOnNeverShadowedByUnnest(t *testing.T) {
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery"
	opts.Schema = schema.M("ds", schema.M(
		"t", schema.M("secret", "STRING", "id", "INT64", "arr", "ARRAY<INT64>"),
		"b", schema.M("id", "INT64"),
		"c", schema.M("ccol", "INT64"),
		"s", schema.M("secret", "STRUCT<foo STRING>"),
	))
	for _, sql := range []string{
		"SELECT 1 FROM ds.s AS t JOIN ds.b AS b ON secret.foo IS NOT NULL CROSS JOIN UNNEST([STRUCT('x' AS foo)]) AS secret",
		"SELECT 1 FROM ds.t AS t JOIN ds.b AS b ON secret IS NOT NULL JOIN (ds.c AS c CROSS JOIN UNNEST([1]) AS secret) ON TRUE",
		"SELECT 1 FROM (ds.t AS t JOIN ds.b AS b ON secret IS NOT NULL) CROSS JOIN UNNEST([1]) AS secret",
		"SELECT 1 FROM ds.t AS t JOIN ds.b AS b ON (SELECT secret) IS NOT NULL CROSS JOIN UNNEST([1]) AS secret",
		"SELECT x FROM ds.t AS t CROSS JOIN UNNEST(ARRAY(SELECT secret)) AS x CROSS JOIN UNNEST([1]) AS secret",
		"SELECT 1 FROM ds.t PIVOT(MAX(secret) FOR id IN (1)) AS p CROSS JOIN UNNEST([1]) AS secret",
		"SELECT 1 FROM ds.t UNPIVOT(v FOR n IN (secret)) AS p CROSS JOIN UNNEST([1]) AS secret",
	} {
		e, err := sqlglot.ParseOne(sql, "bigquery")
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				r := fmt.Sprint(recover())
				if r != "<nil>" && !strings.Contains(r, "could not be resolved") && !strings.Contains(r, "Ambiguous column") {
					t.Errorf("%s: unexpected panic %v", sql, r)
				}
			}()
			q := optimizer.Qualify(e, opts)
			reads := 0
			for _, col := range q.FindAll(exp.KindColumn) {
				if col.Name() != "secret" {
					continue
				}
				reads++
				if col.Text("table") != "t" {
					t.Errorf("%s: secret bound to %q", sql, col.Text("table"))
				}
			}
			if reads == 0 {
				t.Errorf("%s: read of secret dropped", sql)
			}
		}()
	}

	// An ON can still use an earlier UNNEST alias that collides with nothing.
	e, err := sqlglot.ParseOne("SELECT 1 FROM ds.t AS t, UNNEST(t.arr) AS x JOIN ds.b AS b ON x = b.id", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	optimizer.Qualify(e, opts)

	// A subquery in the SELECT list sees every FROM item, so the UNNEST alias still shadows there.
	opts.Schema = schema.M("ds", schema.M("t", schema.M("hits", "ARRAY<STRUCT<page STRING, cds ARRAY<STRUCT<index INT64, value STRING>>>>")))
	e, err = sqlglot.ParseOne("SELECT (SELECT cd.value FROM UNNEST(hits.cds) AS cd WHERE cd.index = 1) AS v FROM ds.t AS t, UNNEST(hits) AS hits", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), "bigquery", generator.Options{})
	if !strings.Contains(got, "UNNEST(`hits`.`cds`)") {
		t.Errorf("GA360 shape: %s", got)
	}
}
