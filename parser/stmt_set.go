package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.SET] = (*Parser).parseSet

	setParsers = map[string]func(*Parser) exp.Expression{
		"GLOBAL":      func(p *Parser) exp.Expression { return p.parseSetItemAssignment("GLOBAL") },
		"LOCAL":       func(p *Parser) exp.Expression { return p.parseSetItemAssignment("LOCAL") },
		"SESSION":     func(p *Parser) exp.Expression { return p.parseSetItemAssignment("SESSION") },
		"TRANSACTION": func(p *Parser) exp.Expression { return p.parseSetTransaction(false) },
	}
	setTrie = newTrie(setParserKeys(setParsers))

	// mysqlSetParsers ports parsers/mysql.py:231-238 MySQLParser.SET_PARSERS: `**parser.
	// Parser.SET_PARSERS` plus PERSIST/PERSIST_ONLY/CHARACTER SET/CHARSET/NAMES.
	mysqlSetParsers = make(map[string]func(*Parser) exp.Expression, len(setParsers)+5)
	for k, v := range setParsers {
		mysqlSetParsers[k] = v
	}
	mysqlSetParsers["PERSIST"] = func(p *Parser) exp.Expression { return p.parseSetItemAssignment("PERSIST") }
	mysqlSetParsers["PERSIST_ONLY"] = func(p *Parser) exp.Expression { return p.parseSetItemAssignment("PERSIST_ONLY") }
	mysqlSetParsers["CHARACTER SET"] = func(p *Parser) exp.Expression { return p.parseSetItemCharset("CHARACTER SET") }
	mysqlSetParsers["CHARSET"] = func(p *Parser) exp.Expression { return p.parseSetItemCharset("CHARACTER SET") }
	mysqlSetParsers["NAMES"] = func(p *Parser) exp.Expression { return p.parseSetItemNames() }
	mysqlSetTrie = newTrie(setParserKeys(mysqlSetParsers))
}

// setParsers/setTrie port the base SET_PARSERS/SET_TRIE (parser.py:1553-1558, 1855).
var (
	setParsers map[string]func(*Parser) exp.Expression
	setTrie    wordTrie

	mysqlSetParsers map[string]func(*Parser) exp.Expression
	mysqlSetTrie    wordTrie
)

func setParserKeys(parsers map[string]func(*Parser) exp.Expression) []string {
	keys := make([]string, 0, len(parsers))
	for key := range parsers {
		keys = append(keys, key)
	}
	return keys
}

// parseSet ports _parse_set (parser.py:9265-9275). unset/tag are always false here: no
// dialect in this port's base/mysql/postgres scope wires TokenType.UNSET or a `tag=true`
// caller (those belong to other dialects' STATEMENT_PARSERS, out of scope). Degrades to a
// raw Command whenever the structured Set leaves trailing tokens - now only a fallback for
// shapes this port's parseSetItem/parseSetItemAssignment don't structurally model; mysql's
// `@`/`@@` user/system variable forms (`SET @x = 1`, `SET @@GLOBAL.x = 1`) parse
// structurally via Parameter/SessionParameter (residual-tail cluster).
func (p *Parser) parseSet() exp.Expression {
	start := p.prev
	index := p.index
	set := p.expression(exp.Set(exp.Args{
		"expressions": p.parseCsv(p.parseSetItem),
		"unset":       false,
		"tag":         false,
	}), nil, nil)
	if p.curr.IsValid() {
		p.retreat(index)
		return p.parseAsCommand(start)
	}
	return set
}

// parseSetItem ports _parse_set_item (parser.py:9261-9263): dispatch through SET_PARSERS/
// SET_TRIE (mysql's table extends the base one with PERSIST/PERSIST_ONLY/CHARACTER SET/
// CHARSET/NAMES), falling back to a plain assignment.
func (p *Parser) parseSetItem() exp.Expression {
	parsers, trie := setParsers, setTrie
	if p.dialect.Name == "mysql" {
		parsers, trie = mysqlSetParsers, mysqlSetTrie
	}
	if parse := p.findParser(parsers, trie); parse != nil {
		return parse(p)
	}
	return p.parseSetItemAssignment(nil)
}

// parseSetItemAssignment ports _parse_set_item_assignment (parser.py:9232-9250). kind is
// `string | nil`, mirroring Python's `str | None`.
func (p *Parser) parseSetItemAssignment(kind any) exp.Expression {
	index := p.index

	if kindStr, ok := kind.(string); ok && (kindStr == "GLOBAL" || kindStr == "SESSION") && p.matchTextSeq("TRANSACTION") {
		return p.parseSetTransaction(kindStr == "GLOBAL")
	}

	left := p.parsePrimary()
	if left == nil {
		left = p.parseColumn()
	}
	assignmentDelimiter := p.matchTexts(setAssignmentDelimiters)

	// SET_REQUIRES_ASSIGNMENT_DELIMITER (parser.py:1774) defaults true and isn't overridden
	// by mysql/postgres in this port's dialect scope, so it's inlined as a constant.
	const setRequiresAssignmentDelimiter = true
	if left == nil || (setRequiresAssignmentDelimiter && !assignmentDelimiter) {
		p.retreat(index)
		return nil
	}

	right := p.parseStatement()
	if right == nil {
		right = p.parseIdVar(true, nil)
	}
	if right != nil && (right.Kind() == exp.KindColumn || right.Kind() == exp.KindIdentifier) {
		right = exp.Var(exp.Args{"this": right.Name()})
	}

	this := p.expression(exp.EQ(exp.Args{"this": left, "expression": right}), nil, nil)
	return p.expression(exp.SetItem(exp.Args{"this": this, "kind": kind}), nil, nil)
}

// parseSetTransaction ports _parse_set_transaction (parser.py:9252-9259).
func (p *Parser) parseSetTransaction(global bool) exp.Expression {
	p.matchTextSeq("TRANSACTION")
	characteristics := p.parseCsv(func() exp.Expression {
		return p.parseVarFromOptions(transactionCharacteristics, true)
	})
	return p.expression(exp.SetItem(exp.Args{
		"expressions": characteristics,
		"kind":        "TRANSACTION",
		"global_":     global,
	}), nil, nil)
}

// parseSetItemCharset ports parsers/mysql.py:519-521 MySQLParser._parse_set_item_charset:
// `SET CHARACTER SET|CHARSET <charset>|DEFAULT`.
func (p *Parser) parseSetItemCharset(kind string) exp.Expression {
	this := p.parseString()
	if this == nil {
		this = p.parseUnquotedField()
	}
	return p.expression(exp.SetItem(exp.Args{"this": this, "kind": kind}), nil, nil)
}

// parseSetItemNames ports parsers/mysql.py:537-544 MySQLParser._parse_set_item_names:
// `SET NAMES <charset>|DEFAULT [COLLATE <collation>]`.
func (p *Parser) parseSetItemNames() exp.Expression {
	charset := p.parseString()
	if charset == nil {
		charset = p.parseUnquotedField()
	}
	var collate exp.Expression
	if p.matchTextSeq("COLLATE") {
		collate = p.parseString()
		if collate == nil {
			collate = p.parseUnquotedField()
		}
	}
	return p.expression(exp.SetItem(exp.Args{"this": charset, "collate": collate, "kind": "NAMES"}), nil, nil)
}

// parseUnquotedField ports _parse_unquoted_field (parser.py:2866-2871): parses a
// generic field and, when it resolved to an unquoted Identifier (e.g. a bare charset name
// or DEFAULT), rewrites it to a Var so it round-trips as a bare word.
func (p *Parser) parseUnquotedField() exp.Expression {
	field := p.parseField(false, nil, false)
	if field != nil && field.Kind() == exp.KindIdentifier {
		if quoted, _ := field.Arg("quoted").(bool); !quoted {
			field = exp.Var(exp.Args{"this": field.Name()})
		}
	}
	return field
}
