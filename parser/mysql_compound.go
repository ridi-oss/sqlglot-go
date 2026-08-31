package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// MySQL compound-statement grammar (reference manual §15.6; verified against real MySQL
// 8.0.46). Upstream sqlglot v30.17.0 parses none of it — grammar extension, DEVIATIONS
// §1.18 and testdata/upstream_extensions.jsonl. These parse only INSIDE a routine body
// (compoundDepth > 0, entered via parseBlock), so the words stay ordinary identifiers in
// plain SQL: `SELECT if FROM loop` is untouched at top level.
//
// Grammar summary (§15.6.1-15.6.6):
//
//	compound_stmt: [label:] BEGIN [stmts] END
//	             | DECLARE name[, name]... type [DEFAULT expr]
//	             | DECLARE handler_action HANDLER FOR condition[, condition]... stmt
//	             | DECLARE name CONDITION FOR condition_value
//	             | DECLARE name CURSOR FOR select
//	             | IF cond THEN stmts [ELSEIF cond THEN stmts]... [ELSE stmts] END IF
//	             | CASE [expr] WHEN val THEN stmts... [ELSE stmts] END CASE
//	             | [label:] LOOP stmts END LOOP [label]
//	             | [label:] REPEAT stmts UNTIL cond END REPEAT [label]
//	             | [label:] WHILE cond DO stmts END WHILE [label]
//
// Statements inside a body are `;`-separated, so each lives in its own chunk; the body
// parsers below consume chunks via nextBodyChunk until their own terminator.

// compoundWords are the procedural statement leaders, dispatched by leading text at a
// statement position inside a compound body.
var compoundWords = map[string]bool{
	"IF": true, "LOOP": true, "REPEAT": true, "WHILE": true, "DECLARE": true,
	"ITERATE": true, "LEAVE": true, "DO": true,
	"SIGNAL": true, "RESIGNAL": true, "GET": true,
}

// parseCompoundStatement dispatches one statement inside a routine body: a procedural
// construct, a labeled construct, or an ordinary statement via parseStatement. Returns
// done=true when the token stream hit the enclosing block's own terminator (END...),
// leaving the terminator unconsumed for the caller.
func (p *Parser) parseCompoundStatement() exp.Expression {
	// [label:] prefix — valid ONLY on BEGIN/LOOP/REPEAT/WHILE (§15.6.2; real MySQL 1064
	// on a labeled SELECT or IF).
	if p.curr.TokenType == tokens.VAR && p.next.TokenType == tokens.COLON {
		label := p.curr.Text
		p.advance(2)
		if p.curr.TokenType == tokens.BEGIN && stringsUpper(p.curr.Text) == "BEGIN" {
			return p.parseMySQLBeginBlock(false, label)
		}
		if p.curr.TokenType == tokens.VAR {
			switch stringsUpper(p.curr.Text) {
			case "LOOP":
				return p.parseLoopBlock(label)
			case "REPEAT":
				return p.parseRepeatBlock(label)
			case "WHILE":
				return p.parseWhileDoBlock(label)
			}
		}
		p.raiseError("Label " + label + " is only valid on BEGIN/LOOP/REPEAT/WHILE")
		p.checkErrors()
		return nil
	}
	if p.curr.TokenType == tokens.BEGIN && stringsUpper(p.curr.Text) == "BEGIN" {
		return p.parseMySQLBeginBlock(false, "")
	}
	if p.curr.TokenType == tokens.CASE {
		return p.parseCaseBlock()
	}
	if p.curr.TokenType == tokens.VAR && compoundWords[stringsUpper(p.curr.Text)] {
		switch stringsUpper(p.curr.Text) {
		case "IF":
			return p.parseIfBlock()
		case "LOOP":
			return p.parseLoopBlock("")
		case "REPEAT":
			return p.parseRepeatBlock("")
		case "WHILE":
			return p.parseWhileDoBlock("")
		case "DECLARE":
			return p.parseDeclare()
		case "ITERATE", "LEAVE":
			// ITERATE/LEAVE <label> (§15.6.5.3-4): keyword + bare label.
			kind := stringsUpper(p.curr.Text)
			p.advance()
			label := p.parseIdVar(false, nil)
			if label == nil {
				p.raiseError("Expected label after " + kind)
				p.checkErrors()
				return nil
			}
			return p.expression(exp.Command(exp.Args{"this": kind, "expression": " " + label.Name()}), nil, nil)
		case "DO":
			// DO <expr>[, <expr>] (§15.7.4.3): evaluate and discard. Real expressions —
			// an expression CASE consumes its own END rather than stopping the statement.
			start := p.curr
			p.advance()
			if p.parseCsv(p.parseDisjunction) == nil {
				p.raiseError("Expected expression after DO")
				p.checkErrors()
				return nil
			}
			text := p.findSQL(start, p.prev)
			return p.expression(exp.Command(exp.Args{"this": "DO", "expression": string([]rune(text)[len([]rune(start.Text)):])}), nil, nil)
		case "SIGNAL", "RESIGNAL", "GET":
			// SIGNAL/RESIGNAL (§15.6.7.1-2) and GET DIAGNOSTICS (§15.6.7.3): condition-
			// handling statements, kept verbatim to the end of their `;` chunk. Upstream
			// errors on all three.
			kind := stringsUpper(p.curr.Text)
			start := p.curr
			for p.curr.IsValid() && p.curr.TokenType != tokens.END {
				p.advance()
			}
			if p.curr.TokenType == tokens.END {
				p.raiseError("Unexpected END in " + kind + " statement")
				p.checkErrors()
				return nil
			}
			text := p.findSQL(start, p.prev)
			return p.expression(exp.Command(exp.Args{"this": kind, "expression": string([]rune(text)[len([]rune(start.Text)):])}), nil, nil)
		}
	}
	return p.parseStatement()
}

// endsCompound reports whether the current position is the enclosing construct's
// terminator: END [IF|LOOP|REPEAT|WHILE|CASE] — always a chunk-leading END inside a body.
func (p *Parser) endsCompound() bool {
	return p.curr.TokenType == tokens.END
}

// parseBodyUntilEnd parses `;`-separated statements until a chunk-leading END, which it
// leaves unconsumed. ELSE/ELSEIF/WHEN/UNTIL chunk leaders also stop it (the caller owns
// them). A batch that runs out of chunks first is an unterminated body: parse error.
func (p *Parser) parseBodyUntilEnd(stops ...string) []exp.Expression {
	var body []exp.Expression
	for {
		if p.endsCompound() {
			return body
		}
		if p.curr.TokenType == tokens.ELSE {
			for _, s := range stops {
				if s == "ELSE" {
					return body
				}
			}
		}
		if p.curr.TokenType == tokens.VAR || p.curr.TokenType == tokens.WHEN {
			word := stringsUpper(p.curr.Text)
			for _, s := range stops {
				if s == word {
					return body
				}
			}
		}
		stmt := p.parseCompoundStatement()
		if stmt != nil {
			body = append(body, stmt)
		}
		if p.index < p.tokensSize {
			p.raiseError("Invalid expression / Unexpected token")
			p.checkErrors()
		}
		if !p.nextBodyChunk() {
			p.raiseError("Unterminated block: routine body ended without END")
			p.checkErrors()
			return body
		}
	}
}

// nextBodyChunk advances to the next `;`-separated chunk inside a routine body. False at
// batch exhaustion.
func (p *Parser) nextBodyChunk() bool {
	if p.chunkIndex >= len(p.chunks) {
		return false
	}
	p.advanceChunk()
	return true
}

// matchEnd consumes `END <word>` (and an optional trailing label) for the given construct.
func (p *Parser) matchEnd(word, label string) {
	if !p.match(tokens.END) {
		p.raiseError("Expected END " + word)
		p.checkErrors()
		return
	}
	closesWord := (p.curr.TokenType == tokens.VAR && stringsUpper(p.curr.Text) == word) ||
		(word == "CASE" && p.curr.TokenType == tokens.CASE)
	if !closesWord {
		p.raiseError("Expected END " + word)
		p.checkErrors()
		return
	}
	p.advance()
	// Optional trailing label: must equal the opening label (real MySQL 1310).
	if p.curr.TokenType == tokens.VAR && !compoundWords[stringsUpper(p.curr.Text)] {
		if p.curr.Text != label {
			p.raiseError("End-label " + p.curr.Text + " without match")
			p.checkErrors()
			return
		}
		p.advance()
	}
}

// parseIfBlock: IF cond THEN stmts [ELSEIF cond THEN stmts]... [ELSE stmts] END IF
// (§15.6.5.2). Builds exp.IfBlock{this, true, false} with ELSEIF chains nested in false.
func (p *Parser) parseIfBlock() exp.Expression {
	p.advance() // IF
	return p.parseIfBlockTail()
}

func (p *Parser) parseIfBlockTail() exp.Expression {
	condition := p.parseDisjunction()
	if !p.match(tokens.THEN) {
		p.raiseError("Expected THEN in IF statement")
		p.checkErrors()
		return nil
	}
	trueBlock := p.expression(exp.Block(exp.Args{
		"expressions": p.parseBodyUntilEnd("ELSE", "ELSEIF"),
	}), nil, nil)
	var falseBlock exp.Expression
	if p.curr.TokenType == tokens.VAR && stringsUpper(p.curr.Text) == "ELSEIF" {
		p.advance()
		falseBlock = p.parseIfBlockTail()
		return p.expression(exp.IfBlock(exp.Args{
			"this": condition, "true": trueBlock, "false": falseBlock,
		}), nil, nil)
	}
	if p.match(tokens.ELSE) {
		falseBlock = p.expression(exp.Block(exp.Args{
			"expressions": p.parseBodyUntilEnd(),
		}), nil, nil)
	}
	p.matchEnd("IF", "")
	return p.expression(exp.IfBlock(exp.Args{
		"this": condition, "true": trueBlock, "false": falseBlock,
	}), nil, nil)
}

// parseLoopBlock: LOOP stmts END LOOP [label] (§15.6.5.5).
func (p *Parser) parseLoopBlock(label string) exp.Expression {
	p.advance() // LOOP
	body := p.parseBodyUntilEnd()
	p.matchEnd("LOOP", label)
	return p.expression(exp.LoopBlock(exp.Args{"expressions": body, "label": labelArg(label)}), nil, nil)
}

// parseWhileDoBlock: WHILE cond DO stmts END WHILE [label] (§15.6.5.8).
func (p *Parser) parseWhileDoBlock(label string) exp.Expression {
	p.advance() // WHILE
	condition := p.parseDisjunction()
	if p.curr.TokenType != tokens.VAR || stringsUpper(p.curr.Text) != "DO" {
		p.raiseError("Expected DO in WHILE statement")
		p.checkErrors()
		return nil
	}
	p.advance()
	body := p.expression(exp.Block(exp.Args{"expressions": p.parseBodyUntilEnd()}), nil, nil)
	p.matchEnd("WHILE", label)
	return p.expression(exp.WhileBlock(exp.Args{"this": condition, "body": body, "label": labelArg(label)}), nil, nil)
}

// parseRepeatBlock: REPEAT stmts UNTIL cond END REPEAT [label] (§15.6.5.6).
func (p *Parser) parseRepeatBlock(label string) exp.Expression {
	p.advance() // REPEAT
	body := p.parseBodyUntilEnd("UNTIL")
	if p.curr.TokenType != tokens.VAR || stringsUpper(p.curr.Text) != "UNTIL" {
		p.raiseError("Expected UNTIL in REPEAT statement")
		p.checkErrors()
		return nil
	}
	p.advance()
	until := p.parseDisjunction()
	p.matchEnd("REPEAT", label)
	return p.expression(exp.RepeatBlock(exp.Args{"expressions": body, "until": until, "label": labelArg(label)}), nil, nil)
}

// parseCaseBlock: CASE [expr] WHEN val THEN stmts... [ELSE stmts] END CASE (§15.6.5.1).
// Distinguished from an expression CASE by context: dispatched only at a statement
// position inside a compound body. Each WHEN arm's THEN body is a statement list.
func (p *Parser) parseCaseBlock() exp.Expression {
	p.advance() // CASE
	var operand exp.Expression
	if p.curr.TokenType != tokens.WHEN {
		operand = p.parseDisjunction()
	}
	var whens []exp.Expression
	for p.match(tokens.WHEN) {
		when := p.parseDisjunction()
		if !p.match(tokens.THEN) {
			p.raiseError("Expected THEN in CASE statement")
			p.checkErrors()
			return nil
		}
		arm := p.expression(exp.Block(exp.Args{
			"expressions": p.parseBodyUntilEnd("ELSE", "WHEN"),
		}), nil, nil)
		whens = append(whens, p.expression(exp.If(exp.Args{"this": when, "true": arm}), nil, nil))
	}
	if len(whens) == 0 {
		p.raiseError("Expected WHEN in CASE statement")
		p.checkErrors()
		return nil
	}
	var elseBlock exp.Expression
	if p.match(tokens.ELSE) {
		elseBlock = p.expression(exp.Block(exp.Args{
			"expressions": p.parseBodyUntilEnd(),
		}), nil, nil)
	}
	p.matchEnd("CASE", "")
	return p.expression(exp.CaseBlock(exp.Args{
		"this": operand, "whens": whens, "else_": elseBlock,
	}), nil, nil)
}

// parseDeclare: DECLARE name[, name]... type [DEFAULT expr] | DECLARE ... HANDLER FOR
// condition[, condition]... stmt | DECLARE name CONDITION FOR ... | DECLARE name CURSOR
// FOR select (§15.6.4, §15.6.7.2-3). Handler/condition/cursor forms keep their verbatim
// text as a Command body child (their semantics are beyond an AST consumer's needs); the
// variable form builds exp.Declare{DeclareItem...} like upstream T-SQL.
func (p *Parser) parseDeclare() exp.Expression {
	start := p.curr
	p.advance() // DECLARE
	// Handler form: DECLARE CONTINUE|EXIT|UNDO HANDLER FOR conditions <statement>.
	if p.curr.TokenType == tokens.VAR {
		switch stringsUpper(p.curr.Text) {
		case "CONTINUE", "EXIT", "UNDO":
			p.advance()
			if p.curr.TokenType != tokens.VAR || stringsUpper(p.curr.Text) != "HANDLER" {
				p.raiseError("Expected HANDLER after handler action")
				p.checkErrors()
				return nil
			}
			p.advance()
			if !p.match(tokens.FOR) {
				p.raiseError("Expected FOR in handler declaration")
				p.checkErrors()
				return nil
			}
			// conditions: SQLSTATE ['VALUE'] '..' | errno | name | SQLWARNING |
			// NOT FOUND | SQLEXCEPTION, comma-separated.
			for {
				p.parseHandlerCondition()
				if !p.match(tokens.COMMA) {
					break
				}
			}
			handlerStmt := p.parseCompoundStatement()
			if handlerStmt == nil {
				p.raiseError("Expected handler statement")
				p.checkErrors()
				return nil
			}
			return p.expression(exp.Command(exp.Args{
				"this":       "DECLARE",
				"expression": p.findSQL(start, p.prev)[len([]rune(start.Text)):],
			}), nil, nil)
		}
	}
	// Name-led forms.
	names := []exp.Expression{p.parseIdVar(false, nil)}
	if names[0] == nil {
		p.raiseError("Expected name in DECLARE")
		p.checkErrors()
		return nil
	}
	if p.curr.TokenType == tokens.VAR {
		switch stringsUpper(p.curr.Text) {
		case "CONDITION", "CURSOR":
			// Verbatim Command through the chunk end (a cursor's SELECT parses fine but
			// its structure isn't needed; a condition's value is a literal).
			for p.curr.IsValid() {
				p.advance()
			}
			return p.expression(exp.Command(exp.Args{
				"this":       "DECLARE",
				"expression": p.findSQL(start, p.prev)[len([]rune(start.Text)):],
			}), nil, nil)
		}
	}
	for p.match(tokens.COMMA) {
		name := p.parseIdVar(false, nil)
		if name == nil {
			p.raiseError("Expected name in DECLARE list")
			p.checkErrors()
			return nil
		}
		names = append(names, name)
	}
	// checkFunc=false: `DECLARE x DECIMAL(10,2)` must keep the parameterized type
	// (checkFunc retreats past modifiers unless a string literal follows).
	kind := p.parseTypes(false, false, true, true)
	if kind == nil {
		p.raiseError("Expected type in DECLARE")
		p.checkErrors()
		return nil
	}
	var dflt exp.Expression
	if p.match(tokens.DEFAULT) {
		dflt = p.parseDisjunction()
	}
	items := make([]exp.Expression, 0, len(names))
	for _, name := range names {
		items = append(items, p.expression(exp.DeclareItem(exp.Args{
			"this": name, "kind": kind, "default": dflt,
		}), nil, nil))
	}
	return p.expression(exp.Declare(exp.Args{"expressions": items}), nil, nil)
}

// parseHandlerCondition consumes one handler condition (§15.6.7.2).
func (p *Parser) parseHandlerCondition() {
	switch {
	case p.curr.TokenType == tokens.VAR && stringsUpper(p.curr.Text) == "SQLSTATE":
		p.advance()
		if p.curr.TokenType == tokens.VAR && stringsUpper(p.curr.Text) == "VALUE" {
			p.advance()
		}
		if !p.match(tokens.STRING, false) {
			p.raiseError("Expected SQLSTATE value string")
			p.checkErrors()
			return
		}
		p.advance()
	case p.curr.TokenType == tokens.NUMBER:
		p.advance()
	case p.curr.TokenType == tokens.NOT:
		p.advance() // NOT FOUND
		p.advance()
	case p.curr.TokenType == tokens.VAR || p.curr.TokenType == tokens.IDENTIFIER:
		p.advance() // SQLWARNING | SQLEXCEPTION | condition_name (quoted or bare)
	default:
		p.raiseError("Expected handler condition")
		p.checkErrors()
	}
}

// labelArg maps an absent label to nil so unlabeled constructs carry no label arg.
func labelArg(label string) any {
	if label == "" {
		return nil
	}
	return label
}
