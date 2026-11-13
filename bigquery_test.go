package sqlglot_test

import (
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
