package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.USE] = (*Parser).parseUse
	statementParsers[tokens.KILL] = (*Parser).parseKill
	statementParsers[tokens.DESC] = (*Parser).parseDescribe
	statementParsers[tokens.DESCRIBE] = (*Parser).parseDescribe
	statementParsers[tokens.LOAD] = (*Parser).parseLoad
}

// killTargets mirrors the two literal texts matched inline by upstream
// _parse_kill (parser.py:3551): `KILL CONNECTION ...` / `KILL QUERY ...`.
var killTargets = map[string]bool{"CONNECTION": true, "QUERY": true}

// parseUse ports _parse_use (parser.py:3735-3741).
func (p *Parser) parseUse() exp.Expression {
	kind := p.parseVarFromOptions(usables, false)
	this := p.parseTable(false, false, nil, false, false, false, false)
	return p.expression(exp.Use(exp.Args{"kind": kind, "this": this}), nil, nil)
}

// parseKill ports _parse_kill (parser.py:3550-3553).
func (p *Parser) parseKill() exp.Expression {
	var kind exp.Expression
	if p.matchTexts(killTargets) {
		kind = exp.Var(exp.Args{"this": p.prev.Text})
	}
	this := p.parsePrimary()
	return p.expression(exp.Kill(exp.Args{"this": this, "kind": kind}), nil, nil)
}

// parseLoad ports _parse_load (parser.py:3653-3678). The `FROM FILES (...)`
// branch (files=...) is not ported: no corpus case exercises it, and Hive's
// LOAD DATA doesn't otherwise combine INPATH with FROM FILES.
func (p *Parser) parseLoad() exp.Expression {
	if !p.matchTextSeq("DATA") {
		return p.parseAsCommand(p.prev)
	}

	local := p.matchTextSeq("LOCAL")
	p.matchTextSeq("INPATH")
	inpath := p.parseString()
	overwrite := p.match(tokens.OVERWRITE)
	var temp any
	if p.match(tokens.INTO) {
		temp = p.match(tokens.TEMPORARY)
		p.match(tokens.TABLE)
	}

	this := p.parseTable(true, false, nil, false, false, false, false)
	partition := p.parsePartition()
	var inputFormat exp.Expression
	if p.matchTextSeq("INPUTFORMAT") {
		inputFormat = p.parseString()
	}
	var serde exp.Expression
	if p.matchTextSeq("SERDE") {
		serde = p.parseString()
	}

	return p.expression(exp.LoadData(exp.Args{
		"this":         this,
		"local":        local,
		"overwrite":    overwrite,
		"temp":         temp,
		"inpath":       inpath,
		"partition":    partition,
		"input_format": inputFormat,
		"serde":        serde,
	}), nil, nil)
}

// parseDescribe ports _parse_describe (parser.py:3416-3445), wrapped in the
// parseCreate degrade idiom (parser_ddl.go:10-23): attempt a structured
// DESCRIBE under error isolation, then fall back to a raw Command whenever
// the body can't be parsed cleanly or leaves trailing tokens.
func (p *Parser) parseDescribe() exp.Expression {
	start := p.prev
	if structured := p.tryParse(func() exp.Expression { return p.parseDescribeStructured() }, false); structured != nil {
		return structured
	}
	return p.parseAsCommand(start)
}

// parseDescribeStructured is the tryParse body for parseDescribe; it returns nil
// (signalling the caller to degrade to Command) for any leftover trailing tokens.
func (p *Parser) parseDescribeStructured() exp.Expression {
	var kind any
	if p.matchSet(creatables) {
		kind = p.prev.Text
	}
	var style any
	if p.matchTexts(describeStyles) {
		style = stringsUpper(p.prev.Text)
	}
	if p.match(tokens.DOT) {
		style = nil
		p.retreat(p.index - 2)
	}

	// Ports `format = self._parse_property() if self._match(TokenType.FORMAT, advance=False)`
	// (parser.py:3425). FORMAT is in PROPERTY_PARSERS as
	// _parse_property_assignment(exp.FileFormatProperty) (parser.py:1270), i.e.
	// `FORMAT [=] [AS] <fmt>`, e.g. mysql `DESCRIBE FORMAT=JSON UPDATE ...`. Parsing it
	// structurally (rather than degrading to Command) lets the generator normalize the
	// leader to DESCRIBE for the EXPLAIN/DESC aliases (mysql tokenizes EXPLAIN as DESCRIBE).
	var format exp.Expression
	if p.match(tokens.FORMAT) {
		p.match(tokens.EQ)
		p.match(tokens.ALIAS)
		format = p.expression(exp.FileFormatProperty(exp.Args{"this": p.parseUnquotedField()}), nil, nil)
	}

	var this exp.Expression
	if p.statementParser(p.curr.TokenType) != nil {
		this = p.parseStatement()
	} else {
		this = p.parseDescribeThis()
	}

	properties := p.parseProperties()
	var expressions []exp.Expression
	if properties != nil {
		expressions = properties.Expressions()
	}
	partition := p.parsePartition()
	asJSON := p.matchTextSeq("AS", "JSON")

	if p.curr.IsValid() {
		return nil
	}

	return p.expression(exp.Describe(exp.Args{
		"this":        this,
		"style":       style,
		"kind":        kind,
		"expressions": expressions,
		"partition":   partition,
		"format":      format,
		"as_json":     asJSON,
	}), nil, nil)
}

// parseDescribeThis ports the `this = self._parse_table(schema=True)` branch
// of _parse_describe (parser.py:3430). Divergence: the shared parseTable's
// own subquery/SELECT fallback (parser.py:4844-4862, "if subquery :=
// self._parse_select(table=True, ...)") is gated on schema=false in this
// port (parser.go:646: "if !schema && !isDBReference"), so a schema=true
// caller can never reach it. DESCRIBE needs that fallback too (e.g.
// `DESCRIBE SELECT 1`, mysql `EXPLAIN ANALYZE SELECT * FROM t` once EXPLAIN
// is tokenized as DESCRIBE), so try it directly here first: parseSelect
// consumes nothing and returns nil when curr isn't SELECT/(select)/VALUES,
// so this is a no-op for the ordinary table-name case.
func (p *Parser) parseDescribeThis() exp.Expression {
	if this := p.parseSelect(false, true, true, true); this != nil {
		return this
	}
	return p.parseTable(true, false, nil, false, false, false, false)
}
