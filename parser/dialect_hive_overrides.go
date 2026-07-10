package parser

import (
	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

// This overlay intentionally covers only the Hive CREATE properties, functions, transforms,
// and type behavior in this slice. Hive's ALTER CHANGE, _parse_partition_and_order,
// _parse_parameter, _to_prop_eq, and CURRENT_TIME-removal overrides remain out of scope.
func init() {
	registerDialectParserOverrides("hive", dialectParserOverrideSet{
		FunctionParsers: map[string]parserOverrideFunc{
			"PERCENTILE": func(p *Parser) exp.Expression {
				return p.parseHiveQuantileFunction(exp.KindQuantile)
			},
			"PERCENTILE_APPROX": func(p *Parser) exp.Expression {
				return p.parseHiveQuantileFunction(exp.KindApproxQuantile)
			},
		},
		NoParenFunctionParsers: map[string]parserOverrideFunc{
			"TRANSFORM": (*Parser).parseHiveTransform,
		},
		PropertyParsers: map[string]propertyParserFunc{
			"SERDEPROPERTIES": func(p *Parser, _ bool) exp.Expression {
				return exp.SerdeProperties(exp.Args{"expressions": p.parseWrappedProperties()})
			},
			"USING": func(p *Parser, _ bool) exp.Expression {
				return p.parseHiveUsingProperty()
			},
		},
		TypeParser: (*Parser).parseHiveTypes,
	})
}

func (p *Parser) parseHiveTransform() exp.Expression {
	if !p.match(tokens.L_PAREN, false) {
		p.retreat(p.index - 1)
		return nil
	}

	args := p.parseWrappedCsv(func() exp.Expression { return p.parseLambda(false) })
	rowFormatBefore := p.parseRowFormat(true)

	var recordWriter exp.Expression
	if p.matchTextSeq("RECORDWRITER") {
		recordWriter = p.parseString()
	}

	if !p.match(tokens.USING) {
		return exp.FromArgList(exp.KindTransform, args)
	}

	commandScript := p.parseString()
	p.match(tokens.ALIAS)
	schema := p.parseSchema(nil)
	rowFormatAfter := p.parseRowFormat(true)

	var recordReader exp.Expression
	if p.matchTextSeq("RECORDREADER") {
		recordReader = p.parseString()
	}

	return p.expression(exp.QueryTransform(exp.Args{
		"expressions":       args,
		"command_script":    commandScript,
		"schema":            schema,
		"row_format_before": rowFormatBefore,
		"record_writer":     recordWriter,
		"row_format_after":  rowFormatAfter,
		"record_reader":     recordReader,
	}), nil, nil)
}

func (p *Parser) parseHiveQuantileFunction(kind exp.Kind) exp.Expression {
	var firstArg exp.Expression
	if p.match(tokens.DISTINCT) {
		firstArg = p.expression(exp.Distinct(exp.Args{
			"expressions": []exp.Expression{p.parseLambda(false)},
		}), nil, nil)
	} else {
		p.match(tokens.ALL)
		firstArg = p.parseLambda(false)
	}

	args := []exp.Expression{firstArg}
	if p.match(tokens.COMMA) {
		args = append(args, p.parseFunctionArgs(false)...)
	}
	return exp.FromArgList(kind, args)
}

func (p *Parser) parseHiveTypes(checkFunc, schema, allowIdentifiers, withCollation bool) exp.Expression {
	this := p.parseTypesBase(checkFunc, schema, allowIdentifiers, withCollation)
	if this == nil || schema {
		return this
	}

	for _, node := range this.Walk() {
		if node.Kind() != exp.KindDataType {
			continue
		}
		switch node.Arg("this") {
		case exp.DTypeChar, exp.DTypeVarchar:
			node.Set("this", exp.DTypeText)
			node.Set("expressions", nil)
		}
	}
	return this
}

var hiveUsingPropertyKinds = map[string]bool{
	"ARCHIVE": true,
	"FILE":    true,
	"JAR":     true,
}

func (p *Parser) parseHiveUsingProperty() exp.Expression {
	if p.matchTexts(hiveUsingPropertyKinds) {
		kind := stringsUpper(p.prev.Text)
		return exp.UsingProperty(exp.Args{
			"this": p.parseString(),
			"kind": kind,
		})
	}

	return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
		return p.expression(exp.FileFormatProperty(exp.Args{"this": this}), nil, nil)
	})
}
