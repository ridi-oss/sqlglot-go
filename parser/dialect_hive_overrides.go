package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// Upstream Hive inherits the base PROPERTY_PARSERS and overrides SERDEPROPERTIES and USING at
// /Users/sjcho/repos/sqlglot-go-hive/.reference/sqlglot-v30.12.0/sqlglot/parsers/hive.py:131-137.
// This port instead composes the generator-safe shared subset with Hive-only parser callbacks for
// the base entries at /Users/sjcho/repos/sqlglot-go-hive/.reference/sqlglot-v30.12.0/sqlglot/parser.py:
// 1243,1268,1287,1310,1326,1328,1337.
// This intentional temporary divergence preserves the no-generator slice's fail-closed invariant;
// the callbacks can return to the shared registry only in a future paired parser+generator slice.
// Hive's ALTER CHANGE, _parse_partition_and_order, _parse_parameter, _to_prop_eq, and
// CURRENT_TIME-removal overrides remain out of scope.
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
			"CLUSTERED": func(p *Parser, _ bool) exp.Expression {
				return p.parseClusteredBy()
			},
			"EXTERNAL": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.ExternalProperty(nil), nil, nil)
			},
			"LOCATION": func(p *Parser, _ bool) exp.Expression {
				return p.parsePropertyAssignment(func(this exp.Expression) exp.Expression {
					return p.expression(exp.LocationProperty(exp.Args{"this": this}), nil, nil)
				})
			},
			"ROW": func(p *Parser, _ bool) exp.Expression {
				return p.parseRow()
			},
			"SERDEPROPERTIES": func(p *Parser, _ bool) exp.Expression {
				return exp.SerdeProperties(exp.Args{"expressions": p.parseWrappedProperties()})
			},
			"STORED": func(p *Parser, _ bool) exp.Expression {
				return p.parseStored()
			},
			"TBLPROPERTIES": func(p *Parser, _ bool) exp.Expression {
				return p.expression(exp.Properties(exp.Args{"expressions": p.parseWrappedProperties()}), nil, nil)
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
