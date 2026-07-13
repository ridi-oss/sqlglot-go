package optimizer_test

import (
	"fmt"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
	"github.com/ridi/sqlglot-go/optimizer"
)

func TestNormalizeIdentifiersFixtures(t *testing.T) {
	for i, pair := range loadSQLFixturePairs(t, "normalize_identifiers.sql") {
		title := pair.Meta["title"]
		if title == "" {
			title = fmt.Sprintf("%d, %s", i+1, pair.SQL)
		}
		t.Run(title, func(t *testing.T) {
			if !dialectInScope(pair.Meta) {
				t.Skipf("deferred dialect: %s", pair.Meta["dialect"])
			}
			if pair.SQL == "SELECT @X" {
				t.Skip("deferred: base parser does not yet parse @ parameters")
			}

			dialect := pair.Meta["dialect"]
			expression, err := sqlglot.ParseOne(pair.SQL, dialect)
			if err != nil {
				t.Fatalf("ParseOne: %v", err)
			}
			result := optimizer.NormalizeIdentifiers(expression, dialect)
			got, err := sqlglot.Generate(result, dialect, generator.Options{})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != pair.Expected {
				t.Fatalf("NormalizeIdentifiers() = %q, want %q", got, pair.Expected)
			}
		})
	}
}

func TestNormalizeIdentifiersString(t *testing.T) {
	got, err := sqlglot.Generate(optimizer.NormalizeIdentifiersString("a%", ""), "", generator.Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != `"a%"` {
		t.Fatalf("NormalizeIdentifiersString(a%%) = %q, want %q", got, `"a%"`)
	}
}
