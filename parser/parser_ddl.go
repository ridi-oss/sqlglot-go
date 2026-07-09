package parser

import (
	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

var rParenCommaSet = map[tokens.TokenType]bool{tokens.R_PAREN: true, tokens.COMMA: true}

func (p *Parser) parseCreate() exp.Expression {
	start := p.prev
	// Attempt a structured CREATE under error isolation, then degrade to a raw Command
	// whenever it can't be parsed cleanly: an unsupported creatable (FUNCTION/INDEX/...),
	// a deferred column constraint or property (1c), or trailing junk after the body.
	// tryParse runs the body at IMMEDIATE error level, so a partial parse (e.g.
	// parseSchema/matchRParen hitting an unparsed `NOT NULL`) panics-and-retreats instead
	// of leaving a stale "Expecting )" that would later poison checkErrors. This keeps the
	// documented graceful degradation (plan §5 Q#1) clean.
	if structured := p.tryParse(func() exp.Expression { return p.parseCreateStructured(start) }, false); structured != nil {
		return structured
	}
	return p.parseAsCommand(start)
}

// parseCreateStructured parses a CREATE into an exp.Create, or returns nil (signalling the
// caller to degrade to a Command) when the statement isn't a structured creatable we
// support or carries trailing tokens we don't yet parse. Under tryParse it may also raise
// on a malformed/deferred body, which tryParse converts to nil.
func (p *Parser) parseCreateStructured(start tokens.Token) exp.Expression {
	replace := start.TokenType == tokens.REPLACE || p.matchPair(tokens.OR, tokens.REPLACE, true) || p.matchPair(tokens.OR, tokens.ALTER, true)
	refresh := p.matchPair(tokens.OR, tokens.REFRESH, true)
	unique := p.match(tokens.UNIQUE)
	if !p.matchSet(creatables) {
		return nil
	}
	createToken := p.prev
	ctt := createToken.TokenType
	concurrently := p.matchTextSeq("CONCURRENTLY")
	exists := p.parseExists(true)

	var this exp.Expression
	var expression exp.Expression
	var noSchemaBinding any
	if dbCreatables[ctt] {
		tableParts := p.parseTableParts(true, ctt == tokens.SCHEMA, false, false)
		p.match(tokens.COMMA)
		this = p.parseSchema(tableParts)
		hasAlias := p.match(tokens.ALIAS)
		expression = p.parseDDLSelect()
		if expression == nil && hasAlias {
			expression = p.tryParse(func() exp.Expression { return p.parseTableParts(false, false, false, false) }, false)
		}
		if ctt == tokens.VIEW && p.matchTextSeq("WITH", "NO", "SCHEMA", "BINDING") {
			noSchemaBinding = true
		}
	} else {
		return nil
	}
	if p.curr.IsValid() && !p.matchSet(rParenCommaSet, false) {
		return nil
	}
	kind := stringsUpper(createToken.Text)
	return p.expression(exp.Create(exp.Args{
		"this":              this,
		"kind":              kind,
		"replace":           replace,
		"refresh":           refresh,
		"unique":            unique,
		"expression":        expression,
		"exists":            exists,
		"no_schema_binding": noSchemaBinding,
		"concurrently":      concurrently,
	}), nil, nil)
}

func (p *Parser) parseSchema(this exp.Expression) exp.Expression {
	index := p.index
	if !p.match(tokens.L_PAREN) {
		return this
	}
	if p.matchSet(selectStartTokens) {
		p.retreat(index)
		return this
	}
	args := p.parseCsv(func() exp.Expression {
		if c := p.parseConstraint(); c != nil {
			return c
		}
		return p.parseFieldDef()
	})
	p.matchRParen(nil)
	return p.expression(exp.Schema(exp.Args{"this": this, "expressions": args}), nil, nil)
}

func (p *Parser) parseFieldDef() exp.Expression {
	return p.parseColumnDef(p.parseField(true, nil, false))
}

func (p *Parser) parseColumnDef(this exp.Expression) exp.Expression {
	if this != nil && this.Kind() == exp.KindColumn {
		this = this.This()
	}
	// schema=true mirrors upstream _parse_column_def (parser.py:7255): it enables the
	// fixed-size-array form (e.g. `col INT[3]`) in column definitions.
	kind := p.parseTypes(false, true, true, false)

	// `<name> FOR ORDINALITY` (parser.py:7257): an XMLTABLE/column-def ordinality marker. The
	// flag isn't rendered by columndef_sql (generator.py:1169), so it round-trips lossily to just
	// `<name>`, matching upstream.
	if p.matchPair(tokens.FOR, tokens.ORDINALITY, true) {
		return p.expression(exp.ColumnDef(exp.Args{"this": this, "ordinality": true}), nil, nil)
	}

	constraints := []exp.Expression{}

	// `<kind> AS (<expr>) [STORED|VIRTUAL]`: the computed-column branch at parser.py:
	// 7283-7302. Gated on WRAPPED_TRANSFORM_COLUMN_CONSTRAINT (=True for base/mysql/
	// postgres; only RisingWave, out of scope, overrides False), which requires the AS to
	// be immediately followed by "(" - so `col INT AS some_expr` (no parens) is left for
	// parseColumnConstraint's own dispatch instead. The two upstream sibling branches
	// (ALIAS/MATERIALIZED-prefixed and IN/OUT parameter constraints, parser.py:7262-7281)
	// are Oracle/T-SQL/procedure-only and aren't exercised by the base/mysql/postgres
	// corpus, so they're omitted here (documented divergence).
	if kind != nil && p.matchPair(tokens.ALIAS, tokens.L_PAREN, false) {
		p.advance() // consume AS, leaving `(` as the current token for parseDisjunction below
		// `this` MUST be parsed before the STORED/VIRTUAL match: upstream (parser.py:7295-7299)
		// evaluates this=self._parse_disjunction() before persisted=self._match_texts(...), and
		// parseDisjunction consumes the `(<expr>)` so the storage keyword (if present) becomes the
		// current token. Computing `persisted` first (Go arg-map evaluation order) would test the
		// still-current `(` and leave STORED/VIRTUAL unconsumed, degrading the CREATE to a Command.
		this := p.parseDisjunction()
		persisted := p.matchTexts(map[string]bool{"STORED": true, "VIRTUAL": true}) && stringsUpper(p.prev.Text) == "STORED"
		constraints = append(constraints, p.expression(exp.ColumnConstraint(exp.Args{
			"kind": p.expression(exp.ComputedColumnConstraint(exp.Args{
				"this":      this,
				"persisted": persisted,
			}), nil, nil),
		}), nil, nil))
	}

	for {
		constraint := p.parseColumnConstraint()
		if constraint == nil {
			break
		}
		constraints = append(constraints, constraint)
	}
	if kind == nil && len(constraints) == 0 {
		return this
	}

	// Trailing FIRST/AFTER <col> position (parser.py:7313-7316), e.g. `ADD COLUMN k INT
	// FIRST` / `ADD COLUMN k INT AFTER m`.
	var position exp.Expression
	if p.matchTexts(map[string]bool{"FIRST": true, "AFTER": true}) {
		// pos must be captured before parseColumn() below, which advances p.prev past the
		// FIRST/AFTER keyword itself (mirrors upstream's `pos = self._prev.text` capture
		// preceding its own _parse_column() call, parser.py:7314-7316).
		pos := p.prev.Text
		position = p.expression(exp.ColumnPosition(exp.Args{"this": p.parseColumn(), "position": pos}), nil, nil)
	}

	return p.expression(exp.ColumnDef(exp.Args{"this": this, "kind": kind, "constraints": constraints, "position": position}), nil, nil)
}

// parseConstraint ports _parse_constraint (parser.py:7462-7468): a named `CONSTRAINT <id>
// (<unnamed constraints>)` clause, or (when CONSTRAINT isn't present) an unnamed
// schema-level constraint drawn from schemaUnnamedConstraints (CHECK/FOREIGN KEY/PRIMARY
// KEY/UNIQUE, +mysql's FULLTEXT/INDEX/KEY/SPATIAL).
func (p *Parser) parseConstraint() exp.Expression {
	if !p.match(tokens.CONSTRAINT) {
		return p.parseUnnamedConstraint(p.schemaUnnamedConstraints())
	}
	return p.expression(exp.Constraint(exp.Args{
		"this":        p.parseIdVar(true, nil),
		"expressions": p.parseUnnamedConstraints(),
	}), nil, nil)
}

// parseUnnamedConstraints ports _parse_unnamed_constraints (parser.py:7470-7478): the body
// of a named CONSTRAINT, a CSV of unnamed constraints (matched against every registered
// CONSTRAINT_PARSERS key, not just the schema-level subset) or plain function calls.
func (p *Parser) parseUnnamedConstraints() []exp.Expression {
	var constraints []exp.Expression
	for {
		constraint := p.parseUnnamedConstraint(nil)
		if constraint == nil {
			constraint = p.parseFunction(nil, false, true, false)
		}
		if constraint == nil {
			break
		}
		constraints = append(constraints, constraint)
	}
	return constraints
}

// parseUnnamedConstraint ports _parse_unnamed_constraint (parser.py:7480-7498). constraints
// filters which texts are eligible (nil means "any CONSTRAINT_PARSERS key", mirroring
// `constraints or self.CONSTRAINT_PARSERS`).
func (p *Parser) parseUnnamedConstraint(constraints map[string]bool) exp.Expression {
	index := p.index
	keys := constraints
	if keys == nil {
		keys = p.constraintParserKeys()
	}
	if p.match(tokens.IDENTIFIER, false) || !p.matchTexts(keys) {
		return nil
	}
	key := stringsUpper(p.prev.Text)
	fn, ok := p.constraintParsers()[key]
	if !ok {
		p.raiseError("No parser found for schema constraint " + key + ".")
		return nil
	}
	result := fn(p)
	if result == nil {
		p.retreat(index)
	}
	return result
}

// parseColumnConstraint ports _parse_column_constraint (parser.py:7443-7460): an optional
// `CONSTRAINT <id>` name, followed by a CONSTRAINT_PARSERS-dispatched kind.
// PROCEDURE_OPTIONS (parser.py:1636) is empty for base/mysql/postgres (only T-SQL overrides
// it), so procedure_option_follows is always false and is omitted here.
func (p *Parser) parseColumnConstraint() exp.Expression {
	var this exp.Expression
	if p.match(tokens.CONSTRAINT) {
		this = p.parseIdVar(true, nil)
	}
	if p.matchTexts(p.constraintParserKeys()) {
		key := stringsUpper(p.prev.Text)
		var constraint exp.Expression
		if fn := p.constraintParsers()[key]; fn != nil {
			constraint = fn(p)
		}
		if constraint == nil {
			p.retreat(p.index - 1)
			return nil
		}
		return p.expression(exp.ColumnConstraint(exp.Args{"this": this, "kind": constraint}), nil, nil)
	}
	return this
}

func (p *Parser) parseAsCommand(start tokens.Token) exp.Expression {
	for p.curr.IsValid() {
		p.advance()
	}
	text := p.findSQL(start, p.prev)
	runes := []rune(text)
	size := len([]rune(start.Text))
	return p.expression(exp.Command(exp.Args{"this": string(runes[:size]), "expression": string(runes[size:])}), nil, nil)
}

func (p *Parser) parseDDLSelect() exp.Expression {
	return p.parseQueryModifiers(p.parseSetOperations(p.parseSelect(true, false, false)))
}
