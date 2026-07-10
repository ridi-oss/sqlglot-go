package parser

import (
	"strings"

	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

// Upstream keeps parser callbacks in per-dialect class tables: the base statement,
// no-parentheses-function, and function tables are in .reference/sqlglot-v30.12.0/sqlglot/parser.py:
// 1081-1111, 1470-1477, and 1488-1521; Presto removes TRIM at .reference/sqlglot-v30.12.0/sqlglot/
// parsers/presto.py:137; and Athena adds a statement parser at .reference/sqlglot-v30.12.0/sqlglot/
// parsers/athena.py:17-21. This registry applies the same dependency-inversion principle as
// expressions.ParseIntoFunc (expressions/builders.go:14-15, wired at sqlglot.go:108-114), but
// cannot copy that mechanism literally. ParseIntoFunc uses leaf-package types, while these
// callbacks must resume an in-flight *Parser. Their typed callback table therefore remains in the
// parser package and is selected through the dialect's plain-string Name, avoiding a dialects ->
// parser import cycle.
type parserOverrideFunc = func(*Parser) exp.Expression
type typeParserOverrideFunc = func(*Parser, bool, bool, bool, bool) exp.Expression

type dialectParserOverrideSet struct {
	FunctionParsers         map[string]parserOverrideFunc
	DisabledFunctionParsers map[string]bool
	StatementParsers        map[tokens.TokenType]parserOverrideFunc
	NoParenFunctionParsers  map[string]parserOverrideFunc
	PropertyParsers         map[string]propertyParserFunc
	TypeParser              typeParserOverrideFunc
}

var dialectParserOverrides = map[string]dialectParserOverrideSet{}

// registerDialectParserOverrides configures an override set during package initialization. A nil
// callback is not a removal marker: function removals must use DisabledFunctionParsers explicitly.
func registerDialectParserOverrides(name string, overrides dialectParserOverrideSet) {
	name = strings.ToLower(name)
	if _, exists := dialectParserOverrides[name]; exists {
		panic("parser: duplicate parser overrides for dialect " + name)
	}

	for _, callback := range overrides.FunctionParsers {
		if callback == nil {
			panic("parser: nil function parser override for dialect " + name)
		}
	}
	for _, callback := range overrides.StatementParsers {
		if callback == nil {
			panic("parser: nil statement parser override for dialect " + name)
		}
	}
	for _, callback := range overrides.NoParenFunctionParsers {
		if callback == nil {
			panic("parser: nil no-parentheses function parser override for dialect " + name)
		}
	}
	for _, callback := range overrides.PropertyParsers {
		if callback == nil {
			panic("parser: nil property parser override for dialect " + name)
		}
	}

	dialectParserOverrides[name] = overrides
}

func (p *Parser) functionParser(name string) parserOverrideFunc {
	dialectName := strings.ToLower(p.dialect.Name)
	overrides := dialectParserOverrides[dialectName]
	if parser := overrides.FunctionParsers[name]; parser != nil {
		return parser
	}
	if overrides.DisabledFunctionParsers[name] {
		return nil
	}

	parser := functionParsers[name]
	if parser == nil {
		return nil
	}

	// Keep the current base-singleton compatibility gates on the fallback only. An overlay above
	// may intentionally add or override any of these names for another dialect.
	switch name {
	case "VALUES":
		if !p.dialect.ValuesIsFunction {
			return nil
		}
	case "SUBSTR", "JSON_VALUE":
		if dialectName != "mysql" {
			return nil
		}
	case "DATE_PART":
		if dialectName != "postgres" {
			return nil
		}
	}
	return parser
}

func (p *Parser) statementParser(tokenType tokens.TokenType) parserOverrideFunc {
	dialectName := strings.ToLower(p.dialect.Name)
	if parser := dialectParserOverrides[dialectName].StatementParsers[tokenType]; parser != nil {
		return parser
	}
	return statementParsers[tokenType]
}

// propertyParser mirrors functionParser and statementParser: the selected dialect overlay wins,
// while the shared PROPERTY_PARSERS singleton remains the fallback for every dialect.
func (p *Parser) propertyParser(name string) propertyParserFunc {
	name = strings.ToUpper(name)
	dialectName := strings.ToLower(p.dialect.Name)
	if parser := dialectParserOverrides[dialectName].PropertyParsers[name]; parser != nil {
		return parser
	}
	return propertyParsers[name]
}

// propertyParserKeys returns a fresh union because matchTexts needs every overlay key even when the
// same dialect continues to inherit the rest of the shared registry.
func (p *Parser) propertyParserKeys() map[string]bool {
	dialectName := strings.ToLower(p.dialect.Name)
	overlay := dialectParserOverrides[dialectName].PropertyParsers
	keys := make(map[string]bool, len(propertyParsers)+len(overlay))
	for name := range propertyParsers {
		keys[name] = true
	}
	for name := range overlay {
		keys[name] = true
	}
	return keys
}

func (p *Parser) typeParserOverride() typeParserOverrideFunc {
	return dialectParserOverrides[strings.ToLower(p.dialect.Name)].TypeParser
}

func (p *Parser) noParenFunctionParserFor(name string) parserOverrideFunc {
	dialectName := strings.ToLower(p.dialect.Name)
	if parser := dialectParserOverrides[dialectName].NoParenFunctionParsers[name]; parser != nil {
		return parser
	}

	parser := noParenFunctionParsers[name]
	if parser == nil || (name == "VARIADIC" && dialectName != "postgres") {
		return nil
	}
	return parser
}
