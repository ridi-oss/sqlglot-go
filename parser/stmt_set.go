package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
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
	mysqlSetParsers["PASSWORD"] = (*Parser).parseSetItemPassword
	mysqlSetTrie = newTrie(setParserKeys(mysqlSetParsers))

	// postgresSetParsers extends the base table with Postgres's SET special-forms, which pinned
	// upstream degrades to a raw Command — structuring them into `Set{SetItem{kind: ...}}` lets a
	// consumer read `SetItem.kind` to tell a privileged SET (ROLE, SESSION AUTHORIZATION) from a
	// benign one (TIME ZONE, NAMES, CONSTRAINTS, SESSION CHARACTERISTICS) without string-scanning.
	// Grammar extension beyond upstream; see dialect_postgres_set.go + DEVIATIONS.
	postgresSetParsers = make(map[string]func(*Parser) exp.Expression, len(setParsers)+6)
	for k, v := range setParsers {
		postgresSetParsers[k] = v
	}
	postgresSetParsers["ROLE"] = (*Parser).parseSetItemRole
	postgresSetParsers["TIME ZONE"] = (*Parser).parseSetItemTimeZone
	postgresSetParsers["NAMES"] = (*Parser).parseSetItemNamesPostgres
	postgresSetParsers["CONSTRAINTS"] = (*Parser).parseSetItemConstraints
	// `SESSION AUTHORIZATION` / `SESSION CHARACTERISTICS` can't be dispatch keys: findParser
	// returns on the first terminal, and `SESSION` is already one (the base assignment scope), so
	// they're handled inside parseSetItemAssignment's SESSION branch instead.
	postgresSetTrie = newTrie(setParserKeys(postgresSetParsers))
}

// setParsers/setTrie port the base SET_PARSERS/SET_TRIE (parser.py:1553-1558, 1855).
var (
	setParsers map[string]func(*Parser) exp.Expression
	setTrie    wordTrie

	mysqlSetParsers map[string]func(*Parser) exp.Expression
	mysqlSetTrie    wordTrie

	postgresSetParsers map[string]func(*Parser) exp.Expression
	postgresSetTrie    wordTrie
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
	items := p.parseCsv(p.parseSetItem)
	// Trailing tokens, or no item parsed at all (e.g. a special form whose required value was
	// missing so its parser returned nil, leaving an empty list), fail closed to a raw Command.
	// Postgres SET is single-item at top level (a comma-list is a mysql feature, or belongs
	// inside a value/CONSTRAINTS/TRANSACTION list), so a multi-item postgres SET — the only way a
	// special form gets comma-combined with another item, which real Postgres rejects — also fails
	// closed rather than admit SQL the server does not accept.
	// MySQL `SET PASSWORD` is a standalone statement that cannot be comma-combined with other SET
	// items in either position (real MySQL rejects `SET PASSWORD = x, @y = 1`), so a multi-item list
	// containing a PASSWORD item also fails closed.
	passwordInMultiItem := false
	if len(items) > 1 {
		for _, item := range items {
			if item != nil && item.Text("kind") == "PASSWORD" {
				passwordInMultiItem = true
				break
			}
		}
	}
	if p.curr.IsValid() || len(items) == 0 || (p.dialect.Name == "postgres" && len(items) > 1) || passwordInMultiItem {
		p.retreat(index)
		return p.parseAsCommand(start)
	}
	return p.expression(exp.Set(exp.Args{
		"expressions": items,
		"unset":       false,
		"tag":         false,
	}), nil, nil)
}

// parseSetItem ports _parse_set_item (parser.py:9261-9263): dispatch through SET_PARSERS/
// SET_TRIE (mysql's table extends the base one with PERSIST/PERSIST_ONLY/CHARACTER SET/
// CHARSET/NAMES), falling back to a plain assignment.
func (p *Parser) parseSetItem() exp.Expression {
	parsers, trie := setParsers, setTrie
	switch p.dialect.Name {
	case "mysql":
		parsers, trie = mysqlSetParsers, mysqlSetTrie
	case "postgres":
		parsers, trie = postgresSetParsers, postgresSetTrie
	}
	if parse := p.findParser(parsers, trie); parse != nil {
		return parse(p)
	}
	return p.parseSetItemAssignment(nil)
}

// isSetAssignmentDelimiterAhead reports whether the current token is a real SET assignment delimiter
// (=/:=/TO). It excludes STRING and quoted IDENTIFIER tokens whose TEXT merely collides with a
// delimiter word — e.g. a role literally named "to" (`SET SESSION ROLE "to"`, valid Postgres) lexes
// as an IDENTIFIER, distinct from the TO keyword and the =/:= operators. Without the token-type
// guard the delimiter peek would misfire on such a name and either crash or, worse, silently
// misclassify a privileged `SET [SESSION|LOCAL] ROLE <name>` as a benign assignment, dropping the
// role. Used only to disambiguate the privileged ROLE / SESSION AUTHORIZATION forms from the
// GUC-alias assignment.
func (p *Parser) isSetAssignmentDelimiterAhead() bool {
	if p.curr.TokenType == tokens.STRING || p.curr.TokenType == tokens.IDENTIFIER {
		return false
	}
	return setAssignmentDelimiters[stringsUpper(p.curr.Text)]
}

// isUnquotedKeywordAhead reports whether the current token is an unquoted keyword/word, i.e. NOT a
// quoted identifier or a string. Used to gate the scoped ROLE / SESSION AUTHORIZATION keyword match:
// a quoted `SET SESSION "role" …` must not be mistaken for the ROLE keyword (matchTextSeq matches by
// text and would otherwise grab the quoted identifier), which would wrongly structure an
// engine-invalid statement as the privileged form.
func (p *Parser) isUnquotedKeywordAhead() bool {
	return p.curr.TokenType != tokens.IDENTIFIER && p.curr.TokenType != tokens.STRING
}

// parseSetItemAssignment ports _parse_set_item_assignment (parser.py:9232-9250). kind is
// `string | nil`, mirroring Python's `str | None`.
func (p *Parser) parseSetItemAssignment(kind any) exp.Expression {
	index := p.index

	if kindStr, ok := kind.(string); ok && (kindStr == "GLOBAL" || kindStr == "SESSION") && p.matchUnquotedTextSeq("TRANSACTION") {
		return p.parseSetTransaction(kindStr == "GLOBAL")
	}

	// Postgres SCOPED privileged forms: `SET [SESSION|LOCAL] ROLE r` and
	// `SET [SESSION|LOCAL] SESSION AUTHORIZATION u`. The scope word (SESSION/LOCAL) was consumed as
	// `kind` by the dispatch; the form label is carried in the returned SetItem's own `kind` (ROLE /
	// SESSION AUTHORIZATION — the SAME kind as the bare form, so a consumer reads the privilege the
	// same way), with the scope preserved separately in `scope`. Beyond pinned upstream, which
	// Commands these. A bare `SET SESSION AUTHORIZATION` (SESSION as the form-start, no scope) is not
	// matched here — its follower is a lone AUTHORIZATION, not `SESSION AUTHORIZATION` — and falls to
	// the block below.
	if kindStr, ok := kind.(string); ok && (kindStr == "SESSION" || kindStr == "LOCAL") && p.dialect.Name == "postgres" && p.isUnquotedKeywordAhead() {
		// SECURITY: `role` and `session_authorization` are also plain GUCs, so `SET SESSION role =
		// attacker` / `SET LOCAL SESSION AUTHORIZATION = x` are ASSIGNMENTS (privilege escalation via
		// the GUC alias), NOT the `ROLE <name>` / `SESSION AUTHORIZATION <name>` privileged forms. A
		// following assignment delimiter (=/:=/TO) means it is the GUC assignment — retreat and let
		// the assignment path build the EQ so a consumer can read the LHS var name. Only the
		// no-delimiter form is the privileged special form. The `isUnquotedKeywordAhead` gate ensures
		// a *quoted* `SET SESSION "role" …` is NOT mistaken for the ROLE keyword (a quoted identifier
		// is never the keyword — real Postgres rejects that form; matchTextSeq matches by text and
		// would otherwise grab it). See DEVIATIONS + the GUC-alias caveat.
		if p.matchUnquotedTextSeq("ROLE") {
			if p.isSetAssignmentDelimiterAhead() {
				p.retreat(index)
			} else {
				item := p.parseSetItemRole()
				if item == nil {
					p.retreat(index)
					return nil
				}
				item.Set("scope", kindStr)
				return item
			}
		} else if p.matchUnquotedTextSeq("SESSION", "AUTHORIZATION") {
			if p.isSetAssignmentDelimiterAhead() {
				p.retreat(index)
			} else {
				item := p.parseSetItemSessionAuthorization()
				if item == nil {
					p.retreat(index)
					return nil
				}
				item.Set("scope", kindStr)
				return item
			}
		}
	}

	// Postgres `SET SESSION AUTHORIZATION ...` / `SET SESSION CHARACTERISTICS AS TRANSACTION ...`
	// — the `SESSION` shadows these longer forms in the dispatch trie, so disambiguate here on the
	// word that follows (an ordinary `SET SESSION x = v` continues to the assignment path below).
	if kindStr, ok := kind.(string); ok && kindStr == "SESSION" && p.dialect.Name == "postgres" {
		// matchUnquotedTextSeq: a quoted `SET SESSION "AUTHORIZATION" x` is not the privileged form —
		// the quoted identifier is never the keyword (real Postgres rejects it), so it falls through to
		// the assignment path / fails closed rather than being structured as SESSION AUTHORIZATION.
		if p.matchUnquotedTextSeq("AUTHORIZATION") {
			if item := p.parseSetItemSessionAuthorization(); item != nil {
				return item
			}
			p.retreat(index)
			return nil
		}
		if p.matchUnquotedTextSeq("CHARACTERISTICS") {
			if item := p.parseSetSessionCharacteristics(); item != nil {
				return item
			}
			p.retreat(index)
			return nil
		}
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
