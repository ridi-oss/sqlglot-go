package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// parseRefresh ports _parse_refresh (parser.py:8702-8714). REFRESH without TABLE or MATERIALIZED
// VIEW is structured only for a string literal; every other unsupported form preserves upstream's
// Command fallback starting at the last consumed token.
func (p *Parser) parseRefresh() exp.Expression {
	var kind string
	if p.match(tokens.TABLE) {
		kind = "TABLE"
	} else if p.matchTextSeq("MATERIALIZED", "VIEW") {
		kind = "MATERIALIZED VIEW"
	}

	this := p.parseString()
	if this == nil {
		this = p.parseTable(false, false, nil, false, false, false, false)
	}
	if kind == "" && (this == nil || this.Kind() != exp.KindLiteral) {
		return p.parseAsCommand(p.prev)
	}

	return p.expression(exp.Refresh(exp.Args{
		"this": this,
		"kind": kind,
	}), nil, nil)
}
