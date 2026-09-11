package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// parsers/hive.py supplies Hive function, property, and grammar overrides.
func init() {
	registerDialectParserOverrides("hive", hiveParserOverrideSet())
	// Athena's Hive-routed statements (ledger athena-show-*, athena-rename-partition) — standalone
	// Hive keeps upstream's Command/parse-error behavior.
	athenaHive := hiveParserOverrideSet()
	athenaHive.StatementParsers = map[tokens.TokenType]parserOverrideFunc{
		tokens.SHOW:  (*Parser).parseHiveShow,
		tokens.ALTER: (*Parser).parseHiveAlter,
	}
	registerDialectParserOverrides("athena-hive", athenaHive)
}

func hiveParserOverrideSet() dialectParserOverrideSet {
	return dialectParserOverrideSet{
		// parsers/hive.py:126-128: CURRENT_TIME is an ordinary identifier in Hive.
		DisabledNoParenFunctions: map[tokens.TokenType]bool{tokens.CURRENT_TIME: true},
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
	}
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
