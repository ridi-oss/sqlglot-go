package parser

import (
	"fmt"
	"testing"

	"github.com/ridi/sqlglot-go/dialects"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func parseFirstWithDialect(d *dialects.Dialect, sql string) (exp.Expression, error) {
	rawTokens, err := d.NewTokenizer().Tokenize(sql)
	if err != nil {
		return nil, err
	}

	expressions, err := New(d).Parse(rawTokens, sql)
	if err != nil {
		return nil, err
	}
	if len(expressions) == 0 || expressions[0] == nil {
		return nil, fmt.Errorf("parse returned no first expression for %q", sql)
	}
	return expressions[0], nil
}

func TestDialectParserOverrideSeam(t *testing.T) {
	const testName = "dialect_parser_override_seam_test"

	registerDialectParserOverrides(testName, dialectParserOverrideSet{
		FunctionParsers: map[string]parserOverrideFunc{
			"SEAM_FUNC": func(p *Parser) exp.Expression {
				left := p.parseBitwise()
				if !p.match(tokens.USING) {
					p.raiseError("Expected USING in SEAM_FUNC")
				}
				right := p.parseBitwise()
				return p.expression(exp.EQ(exp.Args{"this": left, "expression": right}), nil, nil)
			},
		},
		// Disabling SEAM_FUNC too proves an overlay callback wins over disablement for the same key.
		DisabledFunctionParsers: map[string]bool{"SEAM_FUNC": true, "TRIM": true},
		StatementParsers: map[tokens.TokenType]parserOverrideFunc{
			tokens.USING: func(p *Parser) exp.Expression { return p.parseAsCommand(p.prev) },
		},
		NoParenFunctionParsers: map[string]parserOverrideFunc{
			"SEAM_PREFIX": func(p *Parser) exp.Expression {
				return p.expression(exp.Variadic(exp.Args{"this": p.parseBitwise()}), nil, nil)
			},
		},
		NoParenFunctions: map[tokens.TokenType]func(exp.Args) exp.Expression{
			tokens.CURRENT_CATALOG: exp.CurrentDate,
			tokens.CURRENT_DATE:    exp.CurrentTime,
		},
		// TEMPORARY is deliberately also a base property key. Replacing it proves the overlay
		// wins over the shared callback, and placing it before TABLE proves dialect property keys
		// participate in CREATE's pre-creatable-token property pass.
		PropertyParsers: map[string]propertyParserFunc{
			"TEMPORARY": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.ExternalProperty(nil), nil, nil)
			},
		},
		TypeParser: func(p *Parser, checkFunc, schema, allowIdentifiers, withCollation bool) exp.Expression {
			dtype := p.parseTypesBase(checkFunc, schema, allowIdentifiers, withCollation)
			if dtype != nil && dtype.Kind() == exp.KindDataType && dtype.Arg("this") == exp.DTypeInt {
				dtype.Set("this", exp.DTypeText)
			}
			return dtype
		},
	})
	t.Cleanup(func() { delete(dialectParserOverrides, testName) })

	d := dialects.Base()
	d.Name = testName
	// Give the throwaway dialect an ordinary builder for the same name so the structured EQ
	// result below also proves the parser callback takes precedence over dialect function builders.
	d.Functions = map[string]func([]exp.Expression) exp.Expression{
		"SEAM_FUNC": func(args []exp.Expression) exp.Expression {
			return exp.Variadic(exp.Args{"this": args[0]})
		},
	}

	customCreate, err := parseFirstWithDialect(d, "CREATE TEMPORARY TABLE x")
	if err != nil {
		t.Fatalf("parse custom TEMPORARY property: %v", err)
	}
	if customCreate.Kind() != exp.KindCreate {
		t.Fatalf("custom TEMPORARY CREATE kind = %v, want Create:\n%s", customCreate.Kind(), customCreate.ToS())
	}
	customProperties, ok := customCreate.Arg("properties").(exp.Expression)
	if !ok || customProperties == nil || len(customProperties.Expressions()) != 1 || customProperties.Expressions()[0].Kind() != exp.KindExternalProperty {
		t.Fatalf("custom TEMPORARY callback should replace the base callback before TABLE:\n%s", customCreate.ToS())
	}

	for _, tc := range []struct {
		name string
		new  func() *dialects.Dialect
	}{
		{name: "base", new: dialects.Base},
		{name: "mysql", new: dialects.MySQL},
		{name: "postgres", new: dialects.Postgres},
		{name: "presto", new: dialects.Presto},
		{name: "hive", new: dialects.Hive},
		{name: "trino", new: dialects.Trino},
		{name: "athena", new: dialects.Athena},
	} {
		create, err := parseFirstWithDialect(tc.new(), "CREATE TEMPORARY TABLE x")
		if err != nil {
			t.Fatalf("parse %s TEMPORARY property: %v", tc.name, err)
		}
		properties, ok := create.Arg("properties").(exp.Expression)
		if create.Kind() != exp.KindCreate || !ok || properties == nil || len(properties.Expressions()) != 1 || properties.Expressions()[0].Kind() != exp.KindTemporaryProperty {
			t.Fatalf("%s TEMPORARY property changed by custom overlay:\n%s", tc.name, create.ToS())
		}
	}

	customCast, err := parseFirstWithDialect(d, "CAST(x AS INT)")
	if err != nil {
		t.Fatalf("parse custom CAST type: %v", err)
	}
	customType, ok := customCast.Arg("to").(exp.Expression)
	if customCast.Kind() != exp.KindCast || !ok || customType == nil || customType.Kind() != exp.KindDataType || customType.Arg("this") != exp.DTypeText {
		t.Fatalf("custom TypeParser should rewrite INT to TEXT:\n%s", customCast.ToS())
	}
	for _, tc := range []struct {
		name string
		new  func() *dialects.Dialect
	}{
		{name: "base", new: dialects.Base},
		{name: "mysql", new: dialects.MySQL},
		{name: "postgres", new: dialects.Postgres},
		{name: "presto", new: dialects.Presto},
		{name: "trino", new: dialects.Trino},
		{name: "athena", new: dialects.Athena},
	} {
		cast, err := parseFirstWithDialect(tc.new(), "CAST(x AS INT)")
		if err != nil {
			t.Fatalf("parse %s CAST type: %v", tc.name, err)
		}
		dtype, ok := cast.Arg("to").(exp.Expression)
		if cast.Kind() != exp.KindCast || !ok || dtype == nil || dtype.Kind() != exp.KindDataType || dtype.Arg("this") != exp.DTypeInt {
			t.Fatalf("%s CAST type changed by custom TypeParser:\n%s", tc.name, cast.ToS())
		}
	}

	seamFunc, err := parseFirstWithDialect(d, "SEAM_FUNC(a USING b)")
	if err != nil {
		t.Fatalf("parse SEAM_FUNC: %v", err)
	}
	if seamFunc.Kind() != exp.KindEQ {
		t.Fatalf("SEAM_FUNC kind = %v, want EQ:\n%s", seamFunc.Kind(), seamFunc.ToS())
	}
	if left := seamFunc.This(); left == nil || left.Name() != "a" {
		t.Fatalf("SEAM_FUNC left operand mismatch:\n%s", seamFunc.ToS())
	}
	right, ok := seamFunc.Arg("expression").(exp.Expression)
	if !ok || right == nil || right.Name() != "b" {
		t.Fatalf("SEAM_FUNC right operand mismatch:\n%s", seamFunc.ToS())
	}

	for _, tc := range []struct {
		name string
		new  func() *dialects.Dialect
	}{
		{name: "base", new: dialects.Base},
		{name: "mysql", new: dialects.MySQL},
		{name: "postgres", new: dialects.Postgres},
		{name: "presto", new: dialects.Presto},
		{name: "hive", new: dialects.Hive},
		{name: "trino", new: dialects.Trino},
		{name: "athena", new: dialects.Athena},
	} {
		if expression, err := parseFirstWithDialect(tc.new(), "SEAM_FUNC(a USING b)"); err == nil {
			t.Fatalf("%s unexpectedly gained SEAM_FUNC grammar:\n%s", tc.name, expression.ToS())
		}
	}

	trim, err := parseFirstWithDialect(d, "TRIM(x)")
	if err != nil {
		t.Fatalf("parse disabled TRIM: %v", err)
	}
	if trim.Kind() != exp.KindAnonymous || trim.Name() != "TRIM" {
		t.Fatalf("disabled TRIM mismatch:\n%s", trim.ToS())
	}

	var builtInTrim exp.Expression
	for _, tc := range []struct {
		name string
		new  func() *dialects.Dialect
	}{
		{name: "base", new: dialects.Base},
		{name: "mysql", new: dialects.MySQL},
		{name: "postgres", new: dialects.Postgres},
		{name: "hive", new: dialects.Hive},
		{name: "trino", new: dialects.Trino},
		{name: "athena", new: dialects.Athena},
	} {
		got, err := parseFirstWithDialect(tc.new(), "TRIM(x)")
		if err != nil {
			t.Fatalf("parse %s TRIM: %v", tc.name, err)
		}
		if got.Kind() != exp.KindTrim {
			t.Fatalf("%s TRIM kind = %v, want Trim:\n%s", tc.name, got.Kind(), got.ToS())
		}
		if builtInTrim == nil {
			builtInTrim = got
		} else if !builtInTrim.Equal(got) {
			t.Fatalf("%s TRIM AST differs from base:\nbase: %s\n%s: %s", tc.name, builtInTrim.ToS(), tc.name, got.ToS())
		}
	}

	prefix, err := parseFirstWithDialect(d, "SEAM_PREFIX x")
	if err != nil {
		t.Fatalf("parse SEAM_PREFIX: %v", err)
	}
	if prefix.Kind() != exp.KindVariadic || prefix.This() == nil || prefix.This().Name() != "x" {
		t.Fatalf("SEAM_PREFIX mismatch:\n%s", prefix.ToS())
	}

	bare, err := parseFirstWithDialect(d, "CURRENT_CATALOG")
	if err != nil {
		t.Fatalf("parse token-keyed no-paren override: %v", err)
	}
	if bare.Kind() != exp.KindCurrentDate {
		t.Fatalf("token-keyed no-paren override kind = %v, want CurrentDate:\n%s", bare.Kind(), bare.ToS())
	}
	overriddenGlobal, err := parseFirstWithDialect(d, "CURRENT_DATE")
	if err != nil {
		t.Fatalf("parse token-keyed override of global no-paren function: %v", err)
	}
	if overriddenGlobal.Kind() != exp.KindCurrentTime {
		t.Fatalf("dialect token-keyed override did not win over global CURRENT_DATE:\n%s", overriddenGlobal.ToS())
	}
	for _, tc := range []struct {
		name string
		new  func() *dialects.Dialect
	}{
		{name: "base", new: dialects.Base},
		{name: "mysql", new: dialects.MySQL},
		{name: "postgres", new: dialects.Postgres},
		{name: "presto", new: dialects.Presto},
		{name: "hive", new: dialects.Hive},
		{name: "trino", new: dialects.Trino},
		{name: "athena", new: dialects.Athena},
	} {
		other, err := parseFirstWithDialect(tc.new(), "CURRENT_CATALOG")
		if err != nil {
			t.Fatalf("parse %s CURRENT_CATALOG: %v", tc.name, err)
		}
		if other.Kind() == exp.KindCurrentDate {
			t.Fatalf("%s gained the custom token-keyed no-paren override:\n%s", tc.name, other.ToS())
		}
		otherDate, err := parseFirstWithDialect(tc.new(), "CURRENT_DATE")
		if err != nil {
			t.Fatalf("parse %s CURRENT_DATE: %v", tc.name, err)
		}
		if otherDate.Kind() == exp.KindCurrentTime {
			t.Fatalf("%s gained the custom CURRENT_DATE override:\n%s", tc.name, otherDate.ToS())
		}
	}

	command, err := parseFirstWithDialect(d, "USING RESOURCE foo")
	if err != nil {
		t.Fatalf("parse USING statement: %v", err)
	}
	if command.Kind() != exp.KindCommand {
		t.Fatalf("USING statement kind = %v, want Command:\n%s", command.Kind(), command.ToS())
	}

	describe, err := parseFirstWithDialect(d, "DESCRIBE USING RESOURCE foo")
	if err != nil {
		t.Fatalf("parse DESCRIBE USING statement: %v", err)
	}
	if describe.Kind() != exp.KindDescribe {
		t.Fatalf("DESCRIBE USING kind = %v, want Describe:\n%s", describe.Kind(), describe.ToS())
	}
	if this := describe.This(); this == nil || this.Kind() != exp.KindCommand {
		t.Fatalf("DESCRIBE USING child is not Command:\n%s", describe.ToS())
	}
}

func TestParserOverrideNameIsIndependentFromUnderlyingDialect(t *testing.T) {
	const overrideName = "parser_override_name_test"
	registerDialectParserOverrides(overrideName, dialectParserOverrideSet{
		FunctionParsers: map[string]parserOverrideFunc{
			"ROUTED_FUNC": func(p *Parser) exp.Expression {
				return p.expression(exp.Variadic(exp.Args{"this": p.parseBitwise()}), nil, nil)
			},
		},
	})
	t.Cleanup(func() { delete(dialectParserOverrides, overrideName) })

	parseWith := func(t *testing.T, p *Parser, d *dialects.Dialect, sql string) exp.Expression {
		t.Helper()
		rawTokens, err := d.NewTokenizer().Tokenize(sql)
		if err != nil {
			t.Fatalf("tokenize %q: %v", sql, err)
		}
		expressions, err := p.Parse(rawTokens, sql)
		if err != nil {
			t.Fatalf("parse %q: %v", sql, err)
		}
		if len(expressions) != 1 || expressions[0] == nil {
			t.Fatalf("parse %q returned %#v", sql, expressions)
		}
		return expressions[0]
	}

	mysql := dialects.MySQL()
	mysqlParser := newWithErrorLevelAndOverrideName(mysql, sqlerrors.IMMEDIATE, overrideName)
	if mysqlParser.parserOverrideKey() != overrideName || mysqlParser.dialect.Name != "mysql" {
		t.Fatalf("override key/dialect = %q/%q, want %q/mysql", mysqlParser.parserOverrideKey(), mysqlParser.dialect.Name, overrideName)
	}
	if routed := parseWith(t, mysqlParser, mysql, "ROUTED_FUNC(x)"); routed.Kind() != exp.KindVariadic {
		t.Fatalf("custom override-name lookup missed ROUTED_FUNC:\n%s", routed.ToS())
	}
	// JSON_VALUE is a package-global fallback with a MySQL-only compatibility gate. It must still
	// see the real underlying dialect even though callback lookup uses the custom override key.
	if jsonValue := parseWith(t, mysqlParser, mysql, "JSON_VALUE(doc, '$.x')"); jsonValue.Kind() != exp.KindJSONValue {
		t.Fatalf("real MySQL compatibility gate was replaced by override-name lookup:\n%s", jsonValue.ToS())
	}

	base := dialects.Base()
	baseParser := newWithErrorLevelAndOverrideName(base, sqlerrors.IMMEDIATE, overrideName)
	if routed := parseWith(t, baseParser, base, "ROUTED_FUNC(x)"); routed.Kind() != exp.KindVariadic {
		t.Fatalf("base parser missed custom override-name lookup:\n%s", routed.ToS())
	}
	if jsonValue := parseWith(t, baseParser, base, "JSON_VALUE(doc, '$.x')"); jsonValue.Kind() != exp.KindAnonymous {
		t.Fatalf("custom override key bypassed the real base dialect compatibility gate:\n%s", jsonValue.ToS())
	}
}

func TestDialectParserOverrideRejectsNilNoParenFunction(t *testing.T) {
	const name = "nil_no_paren_function_test"
	defer func() {
		if recover() == nil {
			t.Fatal("registerDialectParserOverrides accepted a nil token-keyed no-paren function")
		}
	}()
	registerDialectParserOverrides(name, dialectParserOverrideSet{
		NoParenFunctions: map[tokens.TokenType]func(exp.Args) exp.Expression{
			tokens.CURRENT_CATALOG: nil,
		},
	})
}

func TestAthenaRouterCopiesLimitsAndPreservesErrors(t *testing.T) {
	const sql = "SELECT 1"
	athena := dialects.Athena()
	rawTokens, err := athena.NewTokenizer().Tokenize(sql)
	if err != nil {
		t.Fatalf("tokenize Athena query: %v", err)
	}

	p := NewWithErrorLevel(athena, sqlerrors.RAISE)
	p.maxNodes = 0
	if _, err := p.Parse(rawTokens, sql); err == nil {
		t.Fatal("Athena sub-parser did not inherit maxNodes=0")
	}
	if len(p.Errors()) == 0 {
		t.Fatal("Athena wrapper did not preserve sub-parser errors")
	}
}
