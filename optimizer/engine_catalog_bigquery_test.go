package optimizer_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/optimizer"
)

func bigqueryEngineCatalog() *optimizer.EngineCatalog {
	return &optimizer.EngineCatalog{
		BuiltinFunctions: names("upper", "net.host", "keys.new_keyset"),
		UDFSchemas:       map[string]map[string]bool{"Sales": names("mask")},
		DefaultCatalog:   "p",
		CurrentDatabase:  "Sales",
		CatalogUserRelations: map[string]map[string]map[string]bool{
			"p": {"Sales": names("Users")},
		},
	}
}

func resolveBigQuery(t *testing.T, sql string) ([]optimizer.ResolvedCall, map[string]optimizer.ResolvedRelation) {
	t.Helper()
	statements, err := sqlglot.Parse(sql, "bigquery,opaque_functions=true")
	if err != nil {
		t.Fatalf("parse %q: %v", sql, err)
	}
	var calls []optimizer.ResolvedCall
	relations := map[string]optimizer.ResolvedRelation{}
	for _, statement := range statements {
		callReport := map[exp.Expression]optimizer.ResolvedCall{}
		relationReport := map[exp.Expression]optimizer.ResolvedRelation{}
		optimizer.ResolveEngineIdentities(statement, bigqueryEngineCatalog(), "bigquery", nil, callReport, relationReport)
		for _, call := range statement.FindAll(exp.KindAnonymous) {
			calls = append(calls, callReport[call])
		}
		for table, resolved := range relationReport {
			key, _ := sqlglot.Generate(table, "bigquery", generator.Options{})
			relations[key] = resolved
		}
	}
	return calls, relations
}

func TestBigQueryEngineCalls(t *testing.T) {
	cases := []struct {
		sql  string
		want optimizer.ResolvedCall
	}{
		{"SELECT UPPER(x)", optimizer.ResolvedCall{Kind: optimizer.CallBuiltin, Identity: "upper"}},
		{"SELECT SAFE.upper(x)", optimizer.ResolvedCall{Kind: optimizer.CallBuiltin, Identity: "upper"}},
		{"SELECT SAFE.nope(x)", optimizer.ResolvedCall{}},
		{"SELECT NET.HOST(x)", optimizer.ResolvedCall{Kind: optimizer.CallBuiltin, Identity: "net.host"}},
		{"SELECT KEYS.NEW_KEYSET('x')", optimizer.ResolvedCall{Kind: optimizer.CallBuiltin, Identity: "keys.new_keyset"}},
		{"SELECT Sales.mask(x)", optimizer.ResolvedCall{Kind: optimizer.CallUDF, Identity: "Sales.mask"}},
		// UDF names are case-sensitive: MASK is a different (unknown) function.
		{"SELECT Sales.MASK(x)", optimizer.ResolvedCall{}},
		{"SELECT UPPER(x)", optimizer.ResolvedCall{Kind: optimizer.CallBuiltin, Identity: "upper"}},
		{"SELECT p.Sales.MASK(x)", optimizer.ResolvedCall{}},
		{"SELECT * FROM Sales.MASK(1)", optimizer.ResolvedCall{}},
		{"SELECT p.Sales.mask(x)", optimizer.ResolvedCall{Kind: optimizer.CallUDF, Identity: "Sales.mask"}},
		{"SELECT `p.Sales.mask`(x)", optimizer.ResolvedCall{Kind: optimizer.CallUDF, Identity: "Sales.mask"}},
		{"SELECT other.Sales.mask(x)", optimizer.ResolvedCall{}},
		{"SELECT sales.mask(x)", optimizer.ResolvedCall{}},
		{"SELECT nope(x)", optimizer.ResolvedCall{}},
		{"SELECT upper(x).a", optimizer.ResolvedCall{Kind: optimizer.CallBuiltin, Identity: "upper"}},
		{"SELECT SAFE.Sales.mask(x)", optimizer.ResolvedCall{}},
		{"SELECT * FROM Sales.upper(1)", optimizer.ResolvedCall{}},
		{"SELECT * FROM Sales.mask(1)", optimizer.ResolvedCall{Kind: optimizer.CallUDF, Identity: "Sales.mask"}},
		{"SELECT * FROM p.Sales.mask(1)", optimizer.ResolvedCall{Kind: optimizer.CallUDF, Identity: "Sales.mask"}},
		{"SELECT * FROM upper(1)", optimizer.ResolvedCall{Kind: optimizer.CallBuiltin, Identity: "upper"}},
		{"SELECT * FROM a.p.Sales.mask(1)", optimizer.ResolvedCall{}},
		{"SELECT p.`Sales.mask`(x)", optimizer.ResolvedCall{}},
	}
	for _, c := range cases {
		calls, _ := resolveBigQuery(t, c.sql)
		if len(calls) != 1 || calls[0] != c.want {
			t.Errorf("%s: got %+v, want %+v", c.sql, calls, c.want)
		}
	}

	// A temp function defined under the same root is a UDF and shadows the builtin name set.
	statements, err := sqlglot.Parse("CREATE TEMP FUNCTION upper(x INT64) AS (x); SELECT upper(1)", "bigquery,opaque_functions=true")
	if err != nil {
		t.Fatal(err)
	}
	root := exp.New(exp.KindBlock, exp.Args{"expressions": statements})
	report := map[exp.Expression]optimizer.ResolvedCall{}
	optimizer.ResolveEngineIdentities(root, bigqueryEngineCatalog(), "bigquery", nil, report, nil)
	call := root.Find(exp.KindAnonymous)
	if want := (optimizer.ResolvedCall{Kind: optimizer.CallUDF, Identity: "upper"}); report[call] != want {
		t.Errorf("temp function: got %+v", report[call])
	}
}

func TestBigQueryEngineRelations(t *testing.T) {
	_, relations := resolveBigQuery(t, "WITH users AS (SELECT 1) SELECT * FROM Users, Sales.Users, p.Sales.Users, Sales.users")
	want := map[string]optimizer.ResolvedRelation{
		"Users":         {},
		"Sales.Users":   {Kind: optimizer.UserRelation, Identity: "p.Sales.Users"},
		"p.Sales.Users": {Kind: optimizer.UserRelation, Identity: "p.Sales.Users"},
		"Sales.users":   {},
	}
	for sql, w := range want {
		if got := relations[sql]; got != w {
			t.Errorf("%s: got %+v, want %+v", sql, got, w)
		}
	}
	_, relations = resolveBigQuery(t, "SELECT * FROM other.Sales.Users")
	if got := relations["other.Sales.Users"]; got.Kind != optimizer.RelationUnknown {
		t.Errorf("other project: got %+v", got)
	}
	_, relations = resolveBigQuery(t, "SELECT * FROM Users")
	if got := relations["Users"]; got.Identity != "p.Sales.Users" {
		t.Errorf("unqualified Users: got %+v", got)
	}
}

// Through NormalizeIdentifiers (as Qualify and proxy-monster run it), a 3-part UDF path keeps its case.
func TestBigQueryEngineCallsAfterNormalize(t *testing.T) {
	e, err := sqlglot.ParseOne("SELECT p.Sales.mask(x), Sales.mask(x), `p.Sales.mask`(x) FROM Users", "bigquery")
	if err != nil {
		t.Fatal(err)
	}
	e = optimizer.NormalizeIdentifiers(e, "bigquery")
	report := map[exp.Expression]optimizer.ResolvedCall{}
	optimizer.ResolveEngineIdentities(e, bigqueryEngineCatalog(), "bigquery", nil, report, nil)
	want := optimizer.ResolvedCall{Kind: optimizer.CallUDF, Identity: "Sales.mask"}
	calls := e.FindAll(exp.KindAnonymous)
	if len(calls) != 3 {
		t.Fatalf("found %d calls, want 3: %s", len(calls), e.ToS())
	}
	for _, call := range calls {
		if report[call] != want {
			t.Errorf("%s: got %+v", call.ToS(), report[call])
		}
	}
}

// UDF names are case-sensitive in BigQuery: temp MyF matches only MyF, and Upper does not
// shadow the builtin upper.
func TestBigQueryTempFunctionCase(t *testing.T) {
	for sql, want := range map[string]optimizer.ResolvedCall{
		"CREATE TEMP FUNCTION MyF(x INT64) AS (x); SELECT MyF(1)":     {Kind: optimizer.CallUDF, Identity: "MyF"},
		"CREATE TEMP FUNCTION MyF(x INT64) AS (x); SELECT myf(1)":     {},
		"CREATE TEMP FUNCTION Upper(x INT64) AS (x); SELECT upper(1)": {Kind: optimizer.CallBuiltin, Identity: "upper"},
	} {
		statements, err := sqlglot.Parse(sql, "bigquery,opaque_functions=true")
		if err != nil {
			t.Fatal(err)
		}
		root := exp.New(exp.KindBlock, exp.Args{"expressions": statements})
		report := map[exp.Expression]optimizer.ResolvedCall{}
		optimizer.ResolveEngineIdentities(root, bigqueryEngineCatalog(), "bigquery", nil, report, nil)
		if got := report[statements[1].Find(exp.KindAnonymous)]; got != want {
			t.Errorf("%s: got %+v, want %+v", sql, got, want)
		}
	}
}

// A temp UDF is still recognized after Qualify stamps the default dataset on it.
func TestBigQueryTempFunctionThroughQualify(t *testing.T) {
	statements, err := sqlglot.Parse("CREATE TEMP FUNCTION MyF(x STRING) AS (x); SELECT MyF(e) FROM Users", "bigquery,opaque_functions=true")
	if err != nil {
		t.Fatal(err)
	}
	root := exp.New(exp.KindBlock, exp.Args{"expressions": statements})
	opts := optimizer.DefaultQualifyOpts()
	opts.Dialect = "bigquery,opaque_functions=true"
	opts.DefaultSchema = "Sales"
	opts.Catalog = "p"
	opts.QualifyColumns = false
	opts.ValidateQualifyColumns = false
	opts.EngineCatalog = bigqueryEngineCatalog()
	opts.CallReport = map[exp.Expression]optimizer.ResolvedCall{}
	q := optimizer.Qualify(root, opts)
	if got := opts.CallReport[q.Find(exp.KindAnonymous)]; got != (optimizer.ResolvedCall{Kind: optimizer.CallUDF, Identity: "MyF"}) {
		t.Errorf("got %+v", got)
	}
}
