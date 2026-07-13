package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// parseXMLElement ports _parse_xml_element (parser.py:7977-7992):
// XMLELEMENT(NAME name [, expr, ...]) or XMLELEMENT(EVALNAME expr [, expr, ...]).
func (p *Parser) parseXMLElement() exp.Expression {
	var this exp.Expression
	var evalname any
	if p.matchTextSeq("EVALNAME") {
		evalname = true
		this = p.parseBitwise()
	} else {
		p.matchTextSeq("NAME")
		// _parse_id_var()'s default any_token=True (parser.py:7985): a string literal like
		// NAME 'foo' is accepted and folded to a quoted identifier ("foo").
		this = p.parseIdVar(true, nil)
	}

	var expressionsArg []exp.Expression
	if p.match(tokens.COMMA) {
		expressionsArg = p.parseCsv(p.parseBitwise)
	}

	return p.expression(exp.XMLElement(exp.Args{
		"this":        this,
		"expressions": expressionsArg,
		"evalname":    evalname,
	}), nil, nil)
}

// parseXMLTable ports _parse_xml_table (parser.py:7994-8019):
// XMLTABLE([XMLNAMESPACES(...),] '<xpath>' [PASSING [BY VALUE] col, ...]
//
//	[RETURNING SEQUENCE BY REF] [COLUMNS col_def, ...]).
func (p *Parser) parseXMLTable() exp.Expression {
	var namespaces []exp.Expression
	var passing []exp.Expression
	var columns []exp.Expression

	if p.matchTextSeq("XMLNAMESPACES", "(") {
		namespaces = p.parseXMLNamespace()
		p.matchTextSeq(")", ",")
	}

	this := p.parseString()

	if p.matchTextSeq("PASSING") {
		// The BY VALUE keywords are optional and are provided for semantic clarity.
		p.matchTextSeq("BY", "VALUE")
		passing = p.parseCsv(p.parseColumn)
	}

	byRef := p.matchTextSeq("RETURNING", "SEQUENCE", "BY", "REF")

	if p.matchTextSeq("COLUMNS") {
		// parseFieldDef handles the `<name> <type> [constraints...]` column form; the trailing
		// `PATH '<xpath>'` is one such constraint, registered in the shared constraintParsers table
		// (parser_constraints.go), so it composes with any following constraints exactly as
		// upstream's _parse_field_def does (parser.py:8013).
		columns = p.parseCsv(p.parseFieldDef)
	}

	return p.expression(exp.XMLTable(exp.Args{
		"this":       this,
		"namespaces": namespaces,
		"passing":    passing,
		"columns":    columns,
		"by_ref":     byRef,
	}), nil, nil)
}

// parseXMLNamespace ports _parse_xml_namespace (parser.py:8021-8033):
// DEFAULT '<uri>' | '<uri>' AS prefix [, ...].
func (p *Parser) parseXMLNamespace() []exp.Expression {
	var namespaces []exp.Expression
	for {
		var uri exp.Expression
		if p.match(tokens.DEFAULT) {
			uri = p.parseString()
		} else {
			uri = p.parseAlias(p.parseString(), false)
		}
		namespaces = append(namespaces, p.expression(exp.XMLNamespace(exp.Args{"this": uri}), nil, nil))
		if !p.match(tokens.COMMA) {
			break
		}
	}
	return namespaces
}
