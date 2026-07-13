package optimizer_test

import (
	"fmt"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
)

func TestIsolateTableSelectsFixtures(t *testing.T) {
	schema := optimizerTestSchema()
	for i, pair := range loadSQLFixturePairs(t, "isolate_table_selects.sql") {
		title := pair.Meta["title"]
		if title == "" {
			title = fmt.Sprintf("%d, %s", i+1, pair.SQL)
		}
		t.Run(title, func(t *testing.T) {
			if !dialectInScope(pair.Meta) {
				t.Skipf("deferred dialect: %s", pair.Meta["dialect"])
			}

			dialect := pair.Meta["dialect"]
			expression, err := sqlglot.ParseOne(pair.SQL, dialect)
			if err != nil {
				t.Fatalf("ParseOne: %v", err)
			}
			result := optimizer.IsolateTableSelects(expression, schema, dialect)
			got, err := sqlglot.Generate(result, dialect, generator.Options{})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != pair.Expected {
				t.Fatalf("IsolateTableSelects() = %q, want %q", got, pair.Expected)
			}
		})
	}
}
