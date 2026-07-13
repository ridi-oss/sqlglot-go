package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// parseMySQLPartitionProperty ports MySQLParser._parse_partition_property
// (parsers/mysql.py:573-601). The caller has already consumed "PARTITION BY".
func (p *Parser) parseMySQLPartitionProperty() exp.Expression {
	var buildProperty func(exp.Args) exp.Expression
	var parseValue func() exp.Expression

	switch {
	case p.matchTextSeq("RANGE"):
		buildProperty = exp.PartitionByRangeProperty
		parseValue = p.parseMySQLPartitionRangeValue
	case p.matchTextSeq("LIST"):
		buildProperty = exp.PartitionByListProperty
		parseValue = p.parseMySQLPartitionListValue
	default:
		return nil
	}

	partitionExpressions := p.parseWrappedCsv(p.parseAssignment)

	// The upstream parser returns partitionExpressions directly for Doris/StarRocks when
	// no `(PARTITION ...)` list follows. This MySQL-only contract returns one property node,
	// so an absent create list is left unsupported and the enclosing CREATE fails closed.
	if p.curr.TokenType != tokens.L_PAREN || stringsUpper(p.next.Text) != "PARTITION" {
		return nil
	}

	return p.expression(buildProperty(exp.Args{
		"partition_expressions": partitionExpressions,
		"create_expressions":    p.parseWrappedCsv(parseValue),
	}), nil, nil)
}

// parseMySQLPartitionRangeValue ports _parse_partition_range_value
// (parsers/mysql.py:603-620).
func (p *Parser) parseMySQLPartitionRangeValue() exp.Expression {
	p.matchTextSeq("PARTITION")
	name := p.parseIdVar(false, nil)

	if !p.matchTextSeq("VALUES", "LESS", "THAN") {
		return name
	}

	values := p.parseWrappedCsv(p.parseExpression)
	if len(values) == 1 && values[0] != nil && values[0].Kind() == exp.KindColumn && stringsUpper(values[0].Name()) == "MAXVALUE" {
		values = []exp.Expression{exp.Var(exp.Args{"this": "MAXVALUE"})}
	}

	partitionRange := p.expression(exp.PartitionRange(exp.Args{
		"this":        name,
		"expressions": values,
	}), nil, nil)
	return p.expression(exp.Partition(exp.Args{"expressions": []exp.Expression{partitionRange}}), nil, nil)
}

// parseMySQLPartitionListValue ports _parse_partition_list_value
// (parsers/mysql.py:622-628).
func (p *Parser) parseMySQLPartitionListValue() exp.Expression {
	p.matchTextSeq("PARTITION")
	name := p.parseIdVar(false, nil)
	p.matchTextSeq("VALUES", "IN")
	values := p.parseWrappedCsv(p.parseExpression)
	partitionList := p.expression(exp.PartitionList(exp.Args{
		"this":        name,
		"expressions": values,
	}), nil, nil)
	return p.expression(exp.Partition(exp.Args{"expressions": []exp.Expression{partitionList}}), nil, nil)
}
