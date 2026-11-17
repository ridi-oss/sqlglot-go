package optimizer_test

import (
	"fmt"
	"strings"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	sqlerrors "github.com/ridi-oss/sqlglot-go/errors"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
	"github.com/ridi-oss/sqlglot-go/schema"
)

func TestQualifyColumnsFixtures(t *testing.T) {
	for i, pair := range loadSQLFixturePairs(t, "qualify_columns.sql") {
		title := pair.Meta["title"]
		if title == "" {
			title = fmt.Sprintf("%d, %s", i+1, pair.SQL)
		}
		t.Run(title, func(t *testing.T) {
			if !dialectInScope(pair.Meta) {
				t.Skipf("deferred dialect: %s", pair.Meta["dialect"])
			}
			if reason := deferredQualifyColumnsFixture(pair); reason != "" {
				t.Skipf("deferred: %s", reason)
			}

			dialect := pair.Meta["dialect"]
			expression, err := sqlglot.ParseOne(pair.SQL, dialect)
			if err != nil {
				t.Fatalf("ParseOne: %v", err)
			}
			opts := optimizer.DefaultQualifyOpts()
			opts.Schema = optimizerTestSchema()
			opts.Dialect = dialect
			opts.InferSchema = boolPtr(true)
			opts.Identify = false
			if _, ok := pair.Meta["validate_qualify_columns"]; ok {
				opts.ValidateQualifyColumns = stringToBool(pair.Meta["validate_qualify_columns"])
			}
			if _, ok := pair.Meta["allow_partial_qualification"]; ok {
				opts.AllowPartialQualification = stringToBool(pair.Meta["allow_partial_qualification"])
			}

			result := optimizer.Qualify(expression, opts)
			got, err := sqlglot.Generate(result, dialect, generator.Options{})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != pair.Expected {
				t.Fatalf("Qualify() = %q, want %q", got, pair.Expected)
			}
			assertASTInvariants(t, result)
		})
	}
}

// TestExpandStarsFullStarLeak locks in upstream's id(table)-keyed EXCEPT/RENAME/REPLACE
// behavior (qualify_columns.py:778-780,846): a modifier on a leading full `*` LEAKS into a
// later bare `*` (they share the stable selected_sources key), while qualified stars (x.*)
// never leak (fresh per-occurrence key). No in-scope fixture has two full stars, so this is
// the only guard against a regression to per-selection reset.
func TestExpandStarsFullStarLeak(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SELECT * EXCEPT(a), * FROM x", "SELECT x.b AS b, x.b AS b FROM x AS x"},
		{"SELECT * RENAME(a AS z1), * FROM x", "SELECT x.a AS z1, x.b AS b, x.a AS z1, x.b AS b FROM x AS x"},
		{"SELECT x.* EXCEPT(a), x.* FROM x", "SELECT x.b AS b, x.a AS a, x.b AS b FROM x AS x"},
	}
	for _, tc := range cases {
		expression, err := sqlglot.ParseOne(tc.in, "")
		if err != nil {
			t.Fatalf("ParseOne(%q): %v", tc.in, err)
		}
		opts := optimizer.DefaultQualifyOpts()
		opts.Schema = optimizerTestSchema()
		opts.InferSchema = boolPtr(true)
		opts.Identify = false
		result := optimizer.Qualify(expression, opts)
		got, err := sqlglot.Generate(result, "", generator.Options{})
		if err != nil {
			t.Fatalf("Generate(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("Qualify(%q) =\n  %q\nwant\n  %q", tc.in, got, tc.want)
		}
		assertASTInvariants(t, result)
	}
}

func TestQualifyColumnsInvalid(t *testing.T) {
	for i, sql := range loadSQLFixtures(t, "qualify_columns__invalid.sql") {
		t.Run(fmt.Sprintf("%d", i+1), func(t *testing.T) {
			expression, err := sqlglot.ParseOne(sql, "")
			if err != nil {
				t.Fatalf("ParseOne: %v", err)
			}
			expectOptimizeOrSchemaError(t, func() {
				result := optimizer.QualifyColumns(expression, optimizerTestSchema(), true, true, nil, false, "")
				optimizer.ValidateQualifyColumns(result, sql)
			})
		})
	}

	t.Run("join context fallback unresolved", func(t *testing.T) {
		sql := `
			SELECT
			INLINE_VIEW.a AS ACCOUNT
			FROM (
			(
				SELECT
				a
				FROM table1
			) inline_view
			LEFT JOIN table2
				ON a = table2.id
			)
			LEFT JOIN table3
			ON inline_view.a = table3.a
		`
		expression, err := sqlglot.ParseOne(sql, "")
		if err != nil {
			t.Fatalf("ParseOne: %v", err)
		}
		mappingSchema, err := schema.NewMappingSchema(schema.NewMapping(), "", true)
		if err != nil {
			t.Fatalf("NewMappingSchema: %v", err)
		}
		if err := mappingSchema.AddTable("table3", []string{"a"}, "", nil, false); err != nil {
			t.Fatalf("AddTable: %v", err)
		}
		recovered := expectOptimizeOrSchemaError(t, func() {
			result := optimizer.QualifyColumns(expression, mappingSchema, true, true, nil, false, "")
			optimizer.ValidateQualifyColumns(result, sql)
		})
		if !strings.Contains(fmt.Sprint(recovered), "Column 'a' could not be resolved") {
			t.Fatalf("error = %v, want Column 'a' could not be resolved", recovered)
		}
	})
}

func expectOptimizeOrSchemaError(t *testing.T, fn func()) (recovered any) {
	t.Helper()
	defer func() {
		recovered = recover()
		if recovered == nil {
			t.Fatalf("expected OptimizeError or SchemaError")
		}
		switch recovered.(type) {
		case *sqlerrors.OptimizeError, *sqlerrors.SchemaError:
			return
		default:
			t.Fatalf("unexpected panic: %T %v", recovered, recovered)
		}
	}()
	fn()
	return nil
}

func deferredQualifyColumnsFixture(pair sqlFixturePair) string {
	upperSQL := strings.ToUpper(pair.SQL)
	bigquery := pair.Meta["dialect"] == "bigquery"
	if strings.Contains(upperSQL, "LATERAL VIEW") || (!bigquery && strings.Contains(upperSQL, "UNNEST(")) || strings.Contains(upperSQL, "EXPLODE(") {
		return "UDTF/UNNEST resolver path is slice 4c"
	}
	if !bigquery && strings.Contains(upperSQL, "STRUCT(") {
		return "STRUCT/PropertyEQ is slice 4c"
	}
	if strings.Contains(upperSQL, "READ_CSV(") || strings.Contains(upperSQL, "READ_PARQUET(") || strings.Contains(upperSQL, "ROWS FROM") {
		return "function table source is slice 4c"
	}
	if strings.Contains(upperSQL, "AGGREGATE(") || strings.Contains(pair.SQL, "->") {
		return "lambda scoping is slice 4c"
	}
	if strings.HasPrefix(upperSQL, "SELECT * FROM UNNEST") {
		return "UNNEST scalar source is slice 4c"
	}
	if strings.Contains(upperSQL, "FOO(BAR)") || strings.Contains(upperSQL, "FOO(BAR, BAZ)") {
		return "function table source is slice 4c"
	}
	return ""
}

// An unqualified column binds as Postgres 17 and MySQL 8.4 bind it (DEVIATIONS §1.27): to a source
// visible at its position, to an outer query when none has it, and to nothing when two do.
func TestQualifyColumnsJoinVisibility(t *testing.T) {
	M := schema.M
	c := func(names ...string) *schema.Mapping {
		m := schema.NewMapping()
		for _, name := range names {
			m.Set(name, "INT")
		}
		return m
	}
	cases := []struct {
		schema *schema.Mapping
		sql    string
		want   string
	}{
		// Two visible sources have the name: no outer query may claim it.
		{M("o", c("x"), "a", c("x"), "b", c("x", "id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b ON b.id = x)", "Ambiguous column"},
		{M("o", c("x"), "a", c("x"), "b", c("x", "id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b ON TRUE WHERE x > 0)", "Ambiguous column"},
		{M("o", c("x"), "a", c("x"), "b", c("x", "id")), "SELECT (SELECT x FROM a JOIN b ON TRUE) FROM o", "Ambiguous column"},
		{M("a", c("id"), "b", c("id"), "x", c("k"), "y", c("k2")), "SELECT 1 FROM a JOIN b ON EXISTS (SELECT 1 FROM x JOIN y ON x.k = id)", "Ambiguous column"},
		// An ON sees the items joined up to it, not later ones.
		{M("o", c("ox"), "a", c("id"), "b", c("id"), "c", c("ox")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b ON a.id = b.id AND ox > 0 JOIN c ON TRUE)", "AND o.ox > 0"},
		{M("a", c("ax", "id"), "b", c("bx"), "c", c("id", "ax")), "SELECT 1 FROM (a JOIN b ON b.bx = ax) JOIN c ON c.id = a.id", "b.bx = a.ax"},
		{M("o", c("ox"), "a", c("id"), "b", c("bx"), "c", c("ox")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM (a JOIN b ON a.id = ox) JOIN c ON TRUE)", "a.id = o.ox"},
		// A nested join's ON sees only its own group.
		{M("o", c("x"), "a", c("x"), "b", c("id"), "c", c("id"), "d", c("x")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN (b JOIN c ON x > 0 JOIN d ON TRUE) ON TRUE)", "ON o.x > 0"},
		{M("a", c("id"), "b", c("bx"), "c", c("cx", "id"), "d", c("dx")), "SELECT 1 FROM a JOIN (b JOIN (c JOIN d ON c.cx = d.dx AND id > 0) ON TRUE) ON TRUE", "AND c.id > 0"},
		{M("o", c("ax"), "a", c("id", "ax"), "b", c("id"), "d", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a LEFT JOIN b JOIN d ON b.id = ax ON a.id = b.id)", "b.id = o.ax"},
		// A subquery in an ON sees what the ON sees.
		{M("o", c("x"), "a", c("x"), "b", c("y"), "c", c("y"), "d", c("y")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b ON EXISTS (SELECT 1 FROM c JOIN d ON x > 0))", "ON a.x > 0"},
		{M("o", c("ax"), "a", c("id", "ax"), "b", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b ON (SELECT ax) > 0 JOIN o AS t ON TRUE)", "SELECT a.ax"},
		// A derived table sees no sibling; a LATERAL one sees the items before it.
		{M("o", c("ax"), "a", c("id", "ax"), "b", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM (SELECT b.id FROM b WHERE b.id = ax) AS d JOIN a ON a.id = d.id)", "b.id = o.ax"},
		{M("o", c("ax"), "a", c("id", "ax"), "b", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN LATERAL (SELECT b.id FROM b WHERE b.id = ax) AS d ON TRUE)", "b.id = a.ax"},
		{M("o", c("p"), "a", c("p"), "b", c("id"), "c", c("id"), "d", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM ((a JOIN b ON TRUE) JOIN c ON p > 0) JOIN d ON TRUE)", "ON a.p > 0"},
		// A table function sees the items before it; a plain VALUES item sees none.
		{M("o", c("p"), "a", c("p")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a CROSS JOIN GENERATE_SERIES(1, p) AS g)", "GENERATE_SERIES(1, a.p)"},
		{M("o", c("p"), "a", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a CROSS JOIN GENERATE_SERIES(1, p) AS g)", "GENERATE_SERIES(1, o.p)"},
		{M("a", c("id")), "SELECT 1 FROM a CROSS JOIN GENERATE_SERIES(1, p) AS g", "could not be resolved"},
		{M("o", c("p"), "a", c("p")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN (VALUES (p)) AS v(x) ON TRUE)", "o.p"},
		{M("o", c("ax"), "a", c("id", "ax"), "b", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN (SELECT b.id FROM b WHERE b.id = ax) AS d ON TRUE)", "b.id = o.ax"},
		// HAVING reads the outer query's column, and an unknown name fails.
		{M("o", c("p"), "a", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY a.id HAVING p > 0)", "HAVING o.p > 0"},
		{M("a", c("id")), "SELECT 1 FROM a GROUP BY a.id HAVING nope > 0", "could not be resolved"},
		// `s.f` names an outer table, though two local sources have a column `s`.
		{M("s", c("f"), "x", c("id"), "a", c("s"), "b", c("s")), "SELECT 1 FROM s WHERE EXISTS (SELECT 1 FROM x WHERE EXISTS (SELECT 1 FROM a JOIN b ON TRUE WHERE s.f > 0))", "s.f > 0"},
		{M("s", c("f", "id"), "a", c("s", "id"), "b", c("s", "id")), "UPDATE s SET f = (SELECT MAX(a.id) FROM a JOIN b ON a.id = b.id WHERE a.id = s.f)", "a.id = s.f"},
		// `o.q` names the table two queries out, not the local column `o`.
		{M("o", c("q"), "b", c("id", "o"), "g", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM (b CROSS JOIN g) JOIN LATERAL (SELECT o.q AS n) AS l ON TRUE)", "SELECT o.q AS n"},
		{M("o", c("q"), "b", c("id", "o"), "g", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM b CROSS JOIN g JOIN LATERAL (SELECT o.q AS n) AS l ON TRUE)", "SELECT o.q AS n"},
		// A lateral item reads the column USING merged; a third table with the name is ambiguous.
		{M("a", c("p"), "b", c("p")), "SELECT 1 FROM (a JOIN b USING (p)) CROSS JOIN LATERAL (SELECT p) AS v", "COALESCE(a.p, b.p)"},
		{M("a", c("p"), "b", c("p")), "SELECT 1 FROM a JOIN b USING (p) CROSS JOIN LATERAL (SELECT p) AS v", "COALESCE(a.p, b.p)"},
		{M("a", c("p"), "b", c("p"), "g", c("p")), "SELECT 1 FROM (a JOIN b USING (p)) JOIN g ON TRUE CROSS JOIN LATERAL (SELECT p) AS v", "Ambiguous column"},
		// LATERAL (VALUES ...) binds like any lateral item, subqueries included.
		{M("o", c("p"), "a", c("p")), "SELECT 1 FROM o, a, LATERAL (VALUES (p)) AS v(x)", "Ambiguous column"},
		{M("o", c("p"), "a", c("p")), "SELECT 1 FROM o, LATERAL (VALUES (p)) AS v(x), a", "o.p"},
		{M("a", c("p"), "b", c("id")), "SELECT 1 FROM a JOIN LATERAL (VALUES ((SELECT p))) AS v(x) ON TRUE JOIN b ON TRUE", "SELECT a.p"},
		{M("a", c("id"), "b", c("id")), "SELECT 1 FROM a JOIN LATERAL (VALUES ((SELECT p))) AS v(x) ON TRUE", "could not be resolved"},
		{M("o", c("p"), "a", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN LATERAL (VALUES ((SELECT p))) AS v(x) ON TRUE)", "SELECT o.p"},
		{M("a", c("p"), "b", c("p")), "SELECT 1 FROM a, b, LATERAL (VALUES ((SELECT p))) AS v(x)", "Ambiguous column"},
		// A lateral item in a join group does not see the items joined after the group.
		{M("o", c("id"), "a", c("id"), "b", c("p")), "SELECT 1 FROM o JOIN (a JOIN LATERAL (SELECT p) AS v(x) ON TRUE) ON TRUE JOIN b ON TRUE", "could not be resolved"},
		{M("o", c("p"), "a", c("id"), "b", c("p")), "SELECT 1 FROM o JOIN (a JOIN LATERAL (VALUES (p)) AS v(x) ON TRUE) ON TRUE JOIN b ON TRUE", "o.p"},
		{M("o", c("p"), "a", c("id"), "b", c("p")), "SELECT 1 FROM o JOIN (a JOIN UNNEST(ARRAY[p]) AS v(x) ON TRUE) ON TRUE JOIN b ON TRUE", "o.p"},
		{M("o", c("x"), "a", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN ((SELECT 1 AS x)) AS d ON x = a.id)", "ON d.x = a.id"},
		{M("o", c("p"), "a", c("p"), "b", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN (b CROSS JOIN GENERATE_SERIES(1, p) AS g) ON TRUE)", "GENERATE_SERIES(1, a.p)"},
		// A visible match wins even when an item before the comma also has the name.
		{M("o", c("p"), "a", c("p"), "b", c("p", "id"), "c", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a, b JOIN c ON p > 0)", "ON b.p > 0"},
		{M("o", c("ax"), "m", c("id"), "x", c("k")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM m WHERE EXISTS (SELECT x.k AS ax FROM x WHERE x.k = ax))", "x.k = o.ax"},
		// WHERE cannot see a select alias.
		{M("o", c("ax"), "x", c("k")), "SELECT 1 FROM o WHERE EXISTS (SELECT x.k AS ax FROM x WHERE x.k = ax)", "x.k = o.ax"},
		// JOIN binds tighter than a comma: the ON does not see `a`.
		{M("o", c("p"), "a", c("p"), "b", c("id"), "c", c("id")), "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a, b JOIN c ON p > 0)", "ON o.p > 0"},
	}
	for _, d := range []string{"postgres", "mysql"} {
		for _, tc := range cases {
			e, err := sqlglot.ParseOne(tc.sql, d)
			if err != nil {
				t.Fatal(err)
			}
			opts := optimizer.DefaultQualifyOpts()
			opts.Dialect = d
			opts.Schema = tc.schema
			got := func() (out string) {
				defer func() {
					if r := recover(); r != nil {
						out = fmt.Sprint(r)
					}
				}()
				out, _ = sqlglot.Generate(optimizer.Qualify(e, opts), d, generator.Options{})
				return strings.NewReplacer(`"`, "", "`", "").Replace(out)
			}()
			if !strings.Contains(got, tc.want) {
				t.Errorf("%s: %s\n got %s\nwant %s", d, tc.sql, got, tc.want)
			}
		}
	}

	// MySQL's bare JOIN is a join, not a comma: the next ON sees `a`.
	e, err := sqlglot.ParseOne("SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b JOIN c ON p > 0)", "mysql")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "mysql"
	opts.Schema = M("o", c("p"), "a", c("p"), "b", c("id"), "c", c("id"))
	if got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), "mysql", generator.Options{}); !strings.Contains(got, "ON `a`.`p` > 0") {
		t.Errorf("mysql bare join: %s", got)
	}

	// A semi-joined table is visible only in its own ON.
	e, err = sqlglot.ParseOne("SELECT a.x FROM a LEFT SEMI JOIN b ON a.id = b.id JOIN c ON x = c.id", "hive")
	if err != nil {
		t.Fatal(err)
	}
	opts.Dialect = "hive"
	opts.Schema = M("a", c("id", "x"), "b", c("id", "x"), "c", c("id"))
	if got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), "hive", generator.Options{}); !strings.Contains(got, "ON `a`.`x` = `c`.`id`") {
		t.Errorf("hive semi join: %s", got)
	}

	e, err = sqlglot.ParseOne("SELECT 1 FROM a LATERAL VIEW EXPLODE(arr) t AS x JOIN b ON b.id = x", "hive")
	if err != nil {
		t.Fatal(err)
	}
	opts.Schema = M("a", c("arr"), "b", c("id"))
	if got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), "hive", generator.Options{}); !strings.Contains(got, "`b`.`id` = `t`.`x`") {
		t.Errorf("hive lateral view: %s", got)
	}

	// Without a schema, inference picks among the visible sources only.
	e, err = sqlglot.ParseOne("SELECT 1 FROM a CROSS JOIN GENERATE_SERIES(1, n) AS g JOIN b ON TRUE", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := sqlglot.Generate(optimizer.Qualify(e, optimizer.DefaultQualifyOpts()), "postgres", generator.Options{}); !strings.Contains(got, `GENERATE_SERIES(1, "a"."n")`) {
		t.Errorf("schema-less table function: %s", got)
	}

	// A PIVOT reads the item it is attached to.
	e, err = sqlglot.ParseOne("SELECT 1 FROM b JOIN a PIVOT(SUM(p) FOR id IN (1, 2)) AS pv ON TRUE", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	opts.Dialect = "bigquery"
	opts.Schema = M("a", c("p", "id", "k"), "b", c("p", "id"))
	if got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), "bigquery", generator.Options{}); !strings.Contains(got, "PIVOT(SUM(`a`.`p`) FOR `a`.`id`") {
		t.Errorf("bigquery pivot: %s", got)
	}

	// A joined pivot: a sibling's ON does not bind to the pivot, and a pass-through column does.
	for sql, want := range map[string]string{
		"SELECT 1 FROM a PIVOT(SUM(p) FOR id IN (1, 2)) AS pv JOIN b ON p > 0": "Ambiguous column",
		"SELECT k FROM a PIVOT(SUM(p) FOR id IN (1, 2)) AS pv JOIN b ON TRUE":  "SELECT `pv`.`k`",
	} {
		e, err = sqlglot.ParseOne(sql, "bigquery")
		if err != nil {
			t.Fatal(err)
		}
		got := func() (out string) {
			defer func() {
				if r := recover(); r != nil {
					out = fmt.Sprint(r)
				}
			}()
			out, _ = sqlglot.Generate(optimizer.Qualify(e, opts), "bigquery", generator.Options{})
			return out
		}()
		if !strings.Contains(got, want) {
			t.Errorf("%s: got %s", sql, got)
		}
	}

	// Schema inference never overrides a name two known sources have.
	e, err = sqlglot.ParseOne("SELECT 1 FROM a JOIN b ON TRUE JOIN u ON x = 1 JOIN v ON TRUE", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	infer := optimizer.DefaultQualifyOpts()
	infer.Dialect = "postgres"
	infer.Schema = M("a", c("x"), "b", c("x"))
	inferSchema := true
	infer.InferSchema = &inferSchema
	func() {
		defer func() {
			if recover() == nil {
				t.Error("inferred a source over an ambiguous name")
			}
		}()
		optimizer.Qualify(e, infer)
	}()

	// BigQuery joins left to right (its comma parses as a CROSS JOIN), so the ON sees `a`.
	e, err = sqlglot.ParseOne("SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a, b JOIN c ON p > 0)", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	opts.Schema = M("o", c("p"), "a", c("p"), "b", c("id"), "c", c("id"))
	if got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), "bigquery", generator.Options{}); !strings.Contains(got, "ON `a`.`p` > 0") {
		t.Errorf("bigquery comma join: %s", got)
	}

	// HAVING cannot name a select alias in Postgres or Trino; it can in MySQL.
	for d, want := range map[string]string{"postgres": "HAVING o.p > 0", "athena": "HAVING o.p > 0", "trino": "HAVING o.p > 0", "mysql": "HAVING a.q > 0"} {
		e, err = sqlglot.ParseOne("SELECT 1 FROM o WHERE EXISTS (SELECT a.q AS p FROM a GROUP BY a.q HAVING p > 0)", d)
		if err != nil {
			t.Fatal(err)
		}
		opts.Dialect = d
		opts.Schema = M("o", c("p"), "a", c("q"))
		got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), d, generator.Options{})
		if got = strings.NewReplacer(`"`, "", "`", "").Replace(got); !strings.Contains(got, want) {
			t.Errorf("%s having alias: %s", d, got)
		}
	}
	opts.Dialect = "bigquery"

	// Outside an aggregate, MySQL's HAVING sees a FROM column only through the select list or
	// GROUP BY; otherwise the name is the outer query's. Postgres sees every FROM column.
	having := M("o", c("p", "id"), "a", c("id", "p"), "b", c("p"), "g", c("id"), "h", c("id", "p"))
	for _, tc := range [][3]string{
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b ON TRUE GROUP BY a.id HAVING p > 0)", "HAVING o.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY a.id, a.p HAVING p > 0)", "HAVING a.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT a.p FROM a GROUP BY a.id HAVING p > 0)", "HAVING a.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY a.id HAVING MAX(p) > 0)", "MAX(a.p)"},
		{"mysql", "SELECT 1 FROM a GROUP BY a.id HAVING p > 0", "could not be resolved"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY a.id HAVING EXISTS (SELECT 1 FROM g GROUP BY g.id HAVING p > 0))", "HAVING o.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY a.id HAVING EXISTS (SELECT 1 FROM g HAVING MAX(p) > 0))", "MAX(a.p)"},
		{"mysql", "SELECT * FROM a HAVING p > 0", "HAVING a.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT g.id, h.* FROM g JOIN h ON TRUE HAVING p > 0)", "HAVING h.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT a.p + 1 AS x FROM a HAVING p > 0)", "HAVING o.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT a.p AS x FROM a HAVING p > 0)", "HAVING a.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT COUNT(*) FROM a HAVING ST_Collect(p) IS NOT NULL)", "(a.p)"},
		{"mysql", "SELECT COUNT(*) AS n FROM a JOIN h ON a.id = h.id GROUP BY a.id HAVING id > 0", "HAVING a.id > 0"},
		{"mysql", "SELECT COUNT(*) AS Cnt FROM a HAVING cnt > 0", "HAVING COUNT(*) > 0"},
		{"mysql", "SELECT COUNT(*) AS cnt FROM a HAVING CNT > 0", "HAVING COUNT(*) > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT COUNT(*) AS P FROM a HAVING p > 0)", "HAVING COUNT(*) > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN h USING (id) HAVING id > 0)", "HAVING o.id > 0"},
		// An aggregate's argument reads a FROM column before a select alias.
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT a.id AS p FROM a HAVING MAX(p) > 0)", "MAX(a.p)"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT g.id AS p FROM g HAVING MAX(p) > 0)", "MAX(g.id)"},
		{"mysql, opaque_functions=true", "SELECT 1 FROM o WHERE EXISTS (SELECT a.id AS p FROM a HAVING max(p) > 0)", "max(a.p)"},
		// A subquery of the HAVING sees every FROM column unless the query groups.
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a HAVING EXISTS (SELECT 1 FROM g WHERE g.id = p))", "g.id = a.p"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a HAVING EXISTS (SELECT 1 FROM g GROUP BY g.id HAVING p > 0))", "HAVING a.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT MAX(a.id) FROM a HAVING EXISTS (SELECT 1 FROM g GROUP BY g.id HAVING p > 0))", "HAVING o.p > 0"},
		{"mysql", "SELECT SUM(a.id) OVER () AS n FROM a HAVING (SELECT p) > 0", "SELECT a.p"},
		{"mysql", "SELECT a.p FROM a JOIN h ON a.id = h.id HAVING (SELECT MAX(g.id) FROM g HAVING MAX(g.id) > p) > 0", "> a.p"},
		// A subquery of the HAVING reads a select alias in its own HAVING, elsewhere only when FROM lacks the name.
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT a.id AS p FROM a HAVING EXISTS (SELECT 1 FROM g GROUP BY g.id HAVING p > 0))", "HAVING p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT g.id AS p FROM g HAVING EXISTS (SELECT 1 FROM g AS z WHERE z.id = p))", "z.id = p)"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT a.id AS p FROM a HAVING EXISTS (SELECT 1 FROM g WHERE g.id = p))", "g.id = a.p"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT a.id AS p FROM a HAVING EXISTS (SELECT 1 FROM g HAVING MAX(p) > 0))", "MAX(a.p)"},
		// The name's table: a GROUP BY column, then an unaliased select column, then the column behind an alias.
		{"mysql", "SELECT a.p AS x FROM a JOIN h ON a.id = h.p GROUP BY h.p HAVING p > 0", "HAVING h.p > 0"},
		{"mysql", "SELECT h.p AS x FROM a JOIN h ON TRUE GROUP BY a.p HAVING p > 0", "HAVING a.p > 0"},
		{"mysql", "SELECT a.id AS aid, h.id FROM a LEFT JOIN h ON a.id = h.id HAVING id IS NULL", "HAVING h.id IS NULL"},
		{"mysql", "SELECT a.p FROM a JOIN h ON TRUE HAVING p > 0", "HAVING a.p > 0"},
		{"mysql", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY (a.p) HAVING p > 0)", "HAVING a.p > 0"},
		// With USING: the selected column, or the merged GROUP BY key.
		{"mysql", "SELECT a.p AS x FROM a RIGHT JOIN h USING (p) HAVING p IS NULL", "HAVING a.p IS NULL"},
		{"mysql", "SELECT COUNT(*) AS n FROM a JOIN h USING (p) GROUP BY p HAVING p > 0", "HAVING COALESCE(a.p, h.p) > 0"},
		{"mysql, opaque_functions=true", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN h USING (id) HAVING my_fn(id) > 0)", "Ambiguous column"},
		{"mysql", "SELECT a.p FROM a JOIN h USING (p) HAVING MAX(p) > 0", "MAX(COALESCE(a.p, h.p))"},
		{"mysql", "SELECT a.id AS p FROM a JOIN h USING (p) HAVING MAX(p) > 0", "MAX(COALESCE(a.p, h.p))"},
		{"mysql", "SELECT a.id AS p FROM a JOIN h USING (p) HAVING p > 0", "HAVING a.id > 0"},
		{"mysql", "SELECT a.p FROM a JOIN h USING (p) GROUP BY h.p HAVING (SELECT p) > 0", "SELECT COALESCE(a.p, h.p)"},
		// The outer select list has the name too.
		{"mysql", "SELECT p FROM a WHERE EXISTS (SELECT g.id FROM g GROUP BY g.id HAVING g.id = p)", "g.id = a.p"},
		{"bigquery", "SELECT p FROM a WHERE EXISTS (SELECT g.id FROM g QUALIFY g.id = p)", "g.id = a.p"},
		// A GROUP BY alias two sources have as a column still expands.
		{"mysql", "SELECT DATE(a.p) AS p, COUNT(*) AS n FROM a JOIN h ON TRUE GROUP BY p", "GROUP BY DATE(a.p)"},
		{"mysql, opaque_functions=true", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY a.id HAVING max(p) > 0)", "max(a.p)"},
		{"mysql, opaque_functions=true", "SELECT 1 FROM a GROUP BY a.id HAVING abs(max(p)) > 0", "max(a.p)"},
		{"mysql, opaque_functions=true", "SELECT 1 FROM a GROUP BY a.id HAVING my_agg(p) > 0", "my_agg(a.p)"},
		{"mysql, opaque_functions=true", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY a.id HAVING my_fn(p) > 0)", "Ambiguous column"},
		{"mysql, opaque_functions=true", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY a.id HAVING p > 0)", "HAVING o.p > 0"},
		{"postgres", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a GROUP BY a.id HAVING p > 0)", "HAVING a.p > 0"},
		{"postgres", "SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b ON TRUE GROUP BY a.id HAVING p > 0)", "Ambiguous column"},
	} {
		e, err = sqlglot.ParseOne(tc[1], tc[0])
		if err != nil {
			t.Fatal(err)
		}
		opts.Dialect = tc[0]
		opts.Schema = having
		got := func() (out string) {
			defer func() {
				if r := recover(); r != nil {
					out = fmt.Sprint(r)
				}
			}()
			out, _ = sqlglot.Generate(optimizer.Qualify(e, opts), tc[0], generator.Options{})
			return strings.NewReplacer(`"`, "", "`", "").Replace(out)
		}()
		if !strings.Contains(got, tc[2]) {
			t.Errorf("%s: %s\n got %s\nwant %s", tc[0], tc[1], got, tc[2])
		}
	}
	opts.Dialect = "bigquery"

	// A struct field on a root two visible sources have is ambiguous, not the outer query's.
	st := func() *schema.Mapping {
		m := schema.NewMapping()
		m.Set("s", "STRUCT<f INT64>")
		return m
	}
	e, err = sqlglot.ParseOne("SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b ON TRUE WHERE s.f > 0)", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	opts.Schema = M("o", st(), "a", st(), "b", st())
	func() {
		defer func() {
			if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "Ambiguous column") {
				t.Errorf("outer query claimed an ambiguous struct root: %v", r)
			}
		}()
		optimizer.Qualify(e, opts)
	}()

	// A mark left by a failed Qualify does not outlive it.
	e, err = sqlglot.ParseOne("SELECT 1 FROM o WHERE EXISTS (SELECT 1 FROM a JOIN b ON TRUE WHERE p > 0)", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	opts.Schema = M("o", c("p"), "a", c("p"), "b", c("p"))
	func() {
		defer func() { _ = recover() }()
		optimizer.Qualify(e, opts)
	}()
	opts.Schema = M("o", c("p"), "a", c("p"), "b", c("id"))
	if got, _ := sqlglot.Generate(optimizer.Qualify(e, opts), "bigquery", generator.Options{}); !strings.Contains(got, "WHERE `a`.`p` > 0") {
		t.Errorf("stale ambiguous mark: %s", got)
	}
}
