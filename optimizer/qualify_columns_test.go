package optimizer_test

import (
	"fmt"
	"strings"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
	"github.com/ridi/sqlglot-go/schema"
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
	if strings.Contains(upperSQL, "LATERAL VIEW") || strings.Contains(upperSQL, "UNNEST(") || strings.Contains(upperSQL, "EXPLODE(") {
		return "UDTF/UNNEST resolver path is slice 4c"
	}
	if strings.Contains(upperSQL, "STRUCT(") {
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
