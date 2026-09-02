package optimizer_test

import (
	"strings"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
	"github.com/ridi-oss/sqlglot-go/schema"
)

// Qualify-level tests for implicit (system) columns via NewMappingSchemaWithImplicit — PG
// semantics, engine-verified on postgres:16/17: explicit refs resolve like normal columns
// (including qualified and aggregate/lineage positions); implicit columns are excluded from
// star expansion, t.*, and NATURAL JOIN / USING candidate sets; and they do NOT propagate
// through derived scopes (a subquery's star output no longer carries them, so an outer ctid
// fails resolution exactly like the engine).

func implicitSchema(t *testing.T) *schema.MappingSchema {
	t.Helper()
	s, err := schema.NewMappingSchemaWithImplicit(
		schema.M(
			"users", schema.M("id", "INT", "name", "TEXT", "ctid", "TID", "xmin", "XID"),
			"orders", schema.M("id", "INT", "uid", "INT", "ctid", "TID", "xmin", "XID"),
		),
		schema.M("users", []string{"ctid", "xmin"}, "orders", []string{"ctid", "xmin"}), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func qualifyImplicit(t *testing.T, sql string) (string, error) {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, "postgres")
	if err != nil {
		t.Fatalf("ParseOne(%q): %v", sql, err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "postgres"
	opts.Schema = implicitSchema(t)
	opts.QuoteIdentifiers = false
	opts.Identify = false
	var out string
	err = func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(error); ok {
					err = e
				} else {
					t.Fatalf("non-error panic: %v", r)
				}
			}
		}()
		qualified := optimizer.Qualify(e, opts)
		out, err = sqlglot.Generate(qualified, "postgres", generator.Options{})
		return err
	}()
	return out, err
}

func TestImplicitColumnsQualify(t *testing.T) {
	for _, tt := range []struct{ name, sql, want string }{
		{"explicit ref resolves", "SELECT ctid FROM users",
			"SELECT users.ctid AS ctid FROM users AS users"},
		{"qualified ref resolves", "SELECT u.ctid FROM users AS u",
			"SELECT u.ctid AS ctid FROM users AS u"},
		{"star excludes implicit", "SELECT * FROM users",
			"SELECT users.id AS id, users.name AS name FROM users AS users"},
		{"t.* excludes implicit", "SELECT users.* FROM users",
			"SELECT users.id AS id, users.name AS name FROM users AS users"},
		{"explicit inner projection re-exports", "SELECT ctid FROM (SELECT ctid FROM users) AS t",
			"SELECT t.ctid AS ctid FROM (SELECT users.ctid AS ctid FROM users AS users) AS t"},
		{"NATURAL JOIN excludes implicit from candidates", "SELECT * FROM users NATURAL JOIN orders",
			"SELECT COALESCE(users.id, orders.id) AS id, users.name AS name, orders.uid AS uid FROM users AS users JOIN orders AS orders ON users.id = orders.id"},
		{"dedup DELETE lineage", "DELETE FROM users WHERE ctid IN (SELECT MIN(ctid) FROM users GROUP BY name)",
			"DELETE FROM users WHERE ctid IN (SELECT MIN(users.ctid) AS _col_0 FROM users AS users GROUP BY users.name)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := qualifyImplicit(t, tt.sql)
			if err != nil {
				t.Fatalf("qualify error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestImplicitColumnsDerivedScopeFailsClosed(t *testing.T) {
	// PG: system columns do not propagate through derived scopes — the subquery's star output no
	// longer includes ctid, so the outer reference must fail resolution (live PG errors too).
	_, err := qualifyImplicit(t, "SELECT ctid FROM (SELECT * FROM users) AS t")
	if err == nil || !strings.Contains(err.Error(), "ctid") {
		t.Errorf("want unresolved-ctid error, got %v", err)
	}
}

func TestImplicitColumnsUsingExcluded(t *testing.T) {
	// USING with an implicit column must fail (PG: column ctid specified in USING clause does not
	// exist), never silently match the system column.
	_, err := qualifyImplicit(t, "SELECT * FROM users JOIN orders USING (ctid)")
	if err == nil || !strings.Contains(err.Error(), "join") {
		t.Errorf("USING (ctid) must fail the join resolution, got %v", err)
	}
}

func TestImplicitColumnsUsingUnknownSourceFailsClosed(t *testing.T) {
	// USING on an implicit column must fail even when the opposite source's schema is unknown —
	// the tolerant upstream guard would otherwise rewrite the engine-invalid USING into a
	// valid-looking ON comparison of hidden columns. Both source orders.
	for _, sql := range []string{
		"SELECT * FROM unknown_table JOIN users USING (ctid)",
		"SELECT * FROM users JOIN unknown_table USING (ctid)",
	} {
		if _, err := qualifyImplicit(t, sql); err == nil || !strings.Contains(err.Error(), "join") {
			t.Errorf("%q must fail the join resolution, got %v", sql, err)
		}
	}
}

func TestImplicitColumnsPositionalAliases(t *testing.T) {
	// PG positional table aliases bind to ROW columns only; system columns keep their names
	// (live-verified: SELECT u.ctid FROM t AS u(a, b) resolves). Mapping order puts ctid between
	// the row columns to prove the zip skips it.
	s, err := schema.NewMappingSchemaWithImplicit(
		schema.M("users", schema.M("id", "INT", "ctid", "TID", "name", "TEXT")),
		schema.M("users", []string{"ctid"}), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	qualify := func(sql string) (string, error) {
		e, err := sqlglot.ParseOne(sql, "postgres")
		if err != nil {
			t.Fatal(err)
		}
		opts := optimizer.DefaultQualifyOpts()
		opts.Dialect = "postgres"
		opts.Schema = s
		opts.QuoteIdentifiers = false
		opts.Identify = false
		var out string
		err = func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = r.(error)
				}
			}()
			out, err = sqlglot.Generate(optimizer.Qualify(e, opts), "postgres", generator.Options{})
			return err
		}()
		return out, err
	}
	got, err := qualify("SELECT a, b FROM users AS u(a, b)")
	if err != nil || got != "SELECT u.a AS a, u.b AS b FROM users AS u(a, b)" {
		t.Errorf("row aliases: %q %v", got, err)
	}
	got, err = qualify("SELECT u.ctid FROM users AS u(a, b)")
	if err != nil || got != "SELECT u.ctid AS ctid FROM users AS u(a, b)" {
		t.Errorf("system column under alias list: %q %v", got, err)
	}
}

func TestImplicitColumnsDedupDeleteLineage(t *testing.T) {
	// Beyond generated SQL: assert the DML scope actually binds ctid to the users source.
	e, err := sqlglot.ParseOne("DELETE FROM users WHERE ctid IN (SELECT MIN(ctid) FROM users GROUP BY name)", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "postgres"
	opts.Schema = implicitSchema(t)
	opts.QuoteIdentifiers = false
	opts.Identify = false
	report := map[exp.Expression]optimizer.ResolvedSource{}
	opts.ResolutionReport = report
	qualified := optimizer.Qualify(e, opts)
	// The OUTER (DML root) ctid specifically must bind to users — the inner MIN(ctid) resolving
	// is not enough evidence. The outer WHERE's column lives directly under the Delete root,
	// outside its subquery.
	where := qualified.Arg("where")
	whereExpr, _ := where.(exp.Expression)
	if whereExpr == nil {
		t.Fatalf("no where clause: %s", qualified.ToS())
	}
	outerBound := false
	for _, col := range whereExpr.FindAll(exp.KindColumn) {
		if col.Name() != "ctid" {
			continue
		}
		inSubquery := false
		for p := col.Parent(); p != nil && p != whereExpr; p = p.Parent() {
			if p.Kind() == exp.KindSelect {
				inSubquery = true
				break
			}
		}
		if !inSubquery {
			outerBound = true
		}
	}
	if !outerBound {
		t.Errorf("outer DELETE ctid not present/bound: %s", qualified.ToS())
	}
	// And the inner aggregate's ctid must be qualified to users (lineage bucket evidence).
	innerBound := false
	for _, scope := range optimizer.TraverseScope(qualified) {
		for _, col := range scope.Expression.FindAll(exp.KindColumn) {
			if col.Name() == "ctid" && col.Text("table") == "users" {
				innerBound = true
			}
		}
	}
	if !innerBound {
		t.Errorf("no scope binds ctid to users: %s", qualified.ToS())
	}
}

func TestImplicitColumnsNestedWithSearchPath(t *testing.T) {
	// Regression (review finding): a schema.table mapping + SearchPath stamps only the schema
	// part; the visible lookup must still find the full-path entry, or star leaks ctid.
	s, err := schema.NewMappingSchemaWithImplicit(
		schema.M("sch", schema.M("users", schema.M("id", "INT", "ctid", "TID"))),
		schema.M("sch", schema.M("users", []string{"ctid"})), "postgres", true)
	if err != nil {
		t.Fatal(err)
	}
	e, err := sqlglot.ParseOne("SELECT * FROM users", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "postgres"
	opts.Schema = s
	opts.SearchPath = []string{"sch"}
	opts.QuoteIdentifiers = false
	opts.Identify = false
	out := optimizer.Qualify(e, opts)
	got, err := sqlglot.Generate(out, "postgres", generator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "ctid") {
		t.Errorf("star leaked implicit column through SearchPath qualification: %s", got)
	}
}
