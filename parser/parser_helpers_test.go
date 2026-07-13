package parser_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
)

func parseOne(t *testing.T, sql string) exp.Expression {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, "")
	if err != nil {
		t.Fatalf("ParseOne(%q) error: %v", sql, err)
	}
	return expression
}

func parseOneDialect(t *testing.T, sql, dialect string) exp.Expression {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q, %q) error: %v", sql, dialect, err)
	}
	return expression
}

func generateSQL(t *testing.T, expression exp.Expression, dialect string) (string, error) {
	t.Helper()
	return sqlglot.Generate(expression, dialect, generator.Options{})
}

func expressionsForArg(expression exp.Expression, key string) []exp.Expression {
	value := expression.Arg(key)
	if expressions, ok := value.([]exp.Expression); ok {
		return expressions
	}
	return nil
}
