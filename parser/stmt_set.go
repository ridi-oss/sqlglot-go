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
	// MySQL `SET ROLE …` / `SET DEFAULT ROLE … TO …` — privilege ops pinned upstream Commands; modeled
	// into SetItem{kind:"ROLE"|"DEFAULT ROLE"} so a consumer gates them like the Postgres SET ROLE form
	// (which shares kind="ROLE"). Grammar extension; see dialect_mysql_set_role.go + DEVIATIONS.
	mysqlSetParsers["ROLE"] = (*Parser).parseSetItemRoleMySQL
	mysqlSetParsers["DEFAULT ROLE"] = (*Parser).parseSetItemDefaultRoleMySQL
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
	// MySQL `SET PASSWORD`, `SET ROLE`, and `SET DEFAULT ROLE` are standalone statements that cannot be
	// comma-combined with other SET items in either position (real MySQL 8.0.33 rejects
	// `SET PASSWORD = x, @y = 1`, `SET NAMES utf8, ROLE admin`, `SET @x = 1, DEFAULT ROLE r TO u` — all
	// ERROR 1064), so a multi-item list containing any of them also fails closed to Command rather than
	// admit a structured multi-item Set the server would never execute.
	standaloneInMultiItem := false
	if len(items) > 1 {
		for _, item := range items {
			if item == nil {
				continue
			}
			switch item.Text("kind") {
			case "PASSWORD", "ROLE", "DEFAULT ROLE":
				standaloneInMultiItem = true
			}
		}
	}
	if p.curr.IsValid() || len(items) == 0 || (p.dialect.Name == "postgres" && len(items) > 1) || standaloneInMultiItem {
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
	// Capture the index BEFORE findParser (which consumes the dispatch keyword) so a dispatched
	// special-form parser that fails on a malformed tail can be retreated ATOMICALLY — the whole item,
	// keyword included, is left unconsumed. Without this, a failed sub-parser leaves the cursor
	// mid-item and the caller's CSV continues from there, dropping the consumed prefix (e.g.
	// `SET DEFAULT ROLE NONE, @x=1` -> the lossy `SET @x = 1`). Failing closed to nil makes the caller
	// see the item whole and degrade to Command, never a silently-truncated Set.
	index := p.index
	if parse := p.findParser(parsers, trie); parse != nil {
		if item := parse(p); item != nil {
			return item
		}
		p.retreat(index)
		return nil
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

	right := p.parseSetAssignmentValue()
	if right == nil {
		p.retreat(index)
		return nil
	}

	// Postgres allows a single GUC to take a comma-separated VALUE list — `SET search_path = a, b` /
	// `SET search_path TO "$user", public` — where the comma separates VALUES of the one variable.
	// (MySQL/base commas separate SET ITEMS, not values, so `SET a = 1, b = 2` is two items; that split
	// is done by the outer parseCsv, and only Postgres reads a value list here.) Pinned upstream Commands
	// the multi-value form; this structures it — a grammar extension (ledger id pg-set-multi-value), the
	// extra values collected in SetItem.expressions with the EQ (LHS + first value) staying in `this` so
	// a consumer reads the assignment's variable name off EQ→this exactly as for the single-value form.
	var rest []exp.Expression
	if p.dialect.Name == "postgres" && p.curr.TokenType == tokens.COMMA {
		// A comma after the first value means a Postgres single-GUC VALUE list (`SET search_path = a, b`).
		// Postgres `var_list` admits only simple `var_value`s — an identifier, string, number, boolean, or
		// signed number — so EVERY element, the first included, must be one. Anything else (an expression
		// `a + b`, a cast `a::text`, a subquery, the `DEFAULT` keyword, or a second `name = value`
		// assignment `SET a = 1, b = 2`) is a Postgres SYNTAX error and fails closed to Command rather than
		// structuring engine-invalid SQL. (`SET x = 1, 2` — all simple values — stays structured: Postgres
		// parses it and rejects only SEMANTICALLY, "SET x takes only one argument", which is beyond a
		// parser's GUC-type knowledge.) A dangling/leading/double comma also fails closed.
		// isPgSetListValue runs on the RAW (un-flattened) value so a DOTTED column (`a.b` / `a."b"`) is
		// seen and rejected — flattening it to a bare Var first would launder `SET search_path = a.b, c`
		// (a Postgres syntax error) into the valid-but-different `b, c`.
		if !isPgSetListValue(right) {
			p.retreat(index)
			return nil
		}
		for p.match(tokens.COMMA) {
			v := p.parseSetAssignmentValue()
			if v == nil || !isPgSetListValue(v) {
				p.retreat(index)
				return nil
			}
			rest = append(rest, flattenSetValue(v))
		}
	}
	right = flattenSetValue(right)

	this := p.expression(exp.EQ(exp.Args{"this": left, "expression": right}), nil, nil)
	args := exp.Args{"this": this, "kind": kind}
	if len(rest) > 0 {
		args["expressions"] = rest
	}
	return p.expression(exp.SetItem(args), nil, nil)
}

// parseSetAssignmentValue parses one RAW right-hand-side value of a `SET var = value` assignment (also
// each element of a Postgres comma value list). Mirrors the upstream RHS parse (a full statement — so a
// subquery `= (SELECT …)` works — else an id/var). It does NOT flatten here: the caller validates the raw
// value (a Postgres value list rejects a dotted column, which flattening would hide) before rendering it
// with flattenSetValue.
func (p *Parser) parseSetAssignmentValue() exp.Expression {
	v := p.parseStatement()
	if v == nil {
		v = p.parseIdVar(true, nil)
	}
	return v
}

// flattenSetValue renders a SET assignment value the way upstream does — an unquoted identifier
// (Column/Identifier) becomes a bare Var; a DOTTED `a.b` drops its qualifier (lossy but syntactically
// valid) — with one correctness fix (§1.12): a SINGLE-part QUOTED identifier is preserved verbatim so
// `SET search_path = "$user"` round-trips to VALID Postgres (upstream flattens it to the invalid bare
// `$user`, which real Postgres rejects with `syntax error at "$"`). Applied AFTER isPgSetListValue.
func flattenSetValue(v exp.Expression) exp.Expression {
	if v != nil && (v.Kind() == exp.KindColumn || v.Kind() == exp.KindIdentifier) && !isQuotedIdentifierExpr(v) {
		return exp.Var(exp.Args{"this": v.Name()})
	}
	return v
}

// isPgSetListValue reports whether e is a Postgres `var_value` — the only thing allowed as an element of
// a multi-value `SET var = v1, v2` list: an identifier (`Var`/quoted-identifier `Column`), a string or
// number `Literal`, a `Boolean` (TRUE/FALSE/ON), or a signed number (`Neg` of a Literal). It rejects an
// expression (`Add`, `Cast`, …), a subquery, a nested assignment (`EQ`), and the `DEFAULT` keyword (valid
// only as the sole value `SET x = DEFAULT`, never inside a list) — all Postgres SYNTAX errors in a list.
func isPgSetListValue(e exp.Expression) bool {
	switch e.Kind() {
	case exp.KindLiteral, exp.KindBoolean:
		return true
	case exp.KindColumn:
		// A SINGLE-part identifier only (`"$user"`); a dotted `a."b"` is not a valid Postgres var_value
		// (syntax error at `.`), so it must not slip into a value list. The unquoted `DEFAULT` keyword
		// (which parses raw as a single-part Column here, before flattenSetValue) is valid only as the
		// SOLE value, never in a list — reject it; but a QUOTED `"DEFAULT"` is an ordinary identifier
		// value that real Postgres accepts (`SET search_path = "DEFAULT", public`), so keep it.
		return e.Arg("table") == nil && (isQuotedIdentifierExpr(e) || stringsUpper(e.Name()) != "DEFAULT")
	case exp.KindVar, exp.KindIdentifier:
		// A bare Identifier is the raw shape parseIdVar returns for a reserved keyword value that
		// parseStatement can't build into a Column — e.g. `ON` (reserved via `JOIN … ON`), a valid
		// Postgres `opt_boolean_or_string` var_value (`SET x = ON, a` is accepted, semantic error only).
		// It carries no qualifier, so no dotted guard is needed; reject only the UNQUOTED DEFAULT keyword
		// (a quoted `"DEFAULT"` Identifier is a valid identifier name).
		return isQuotedIdentifierExpr(e) || stringsUpper(e.Name()) != "DEFAULT"
	case exp.KindNeg:
		inner, _ := e.Arg("this").(exp.Expression)
		return inner != nil && inner.Kind() == exp.KindLiteral
	}
	return false
}

// isQuotedIdentifierExpr reports whether e is (or wraps) a quoted identifier — a bare `Identifier` with
// quoted=true, or a single-part `Column` whose name Identifier is quoted (`"$user"`). Such a value must
// keep its quotes to round-trip to valid SQL (see parseSetAssignmentValue).
func isQuotedIdentifierExpr(e exp.Expression) bool {
	switch e.Kind() {
	case exp.KindIdentifier:
		quoted, _ := e.Arg("quoted").(bool)
		return quoted
	case exp.KindColumn:
		// Only a SINGLE-part column is a quoted identifier value. A dotted `a."b"` is NOT valid Postgres
		// (syntax error at `.`); leave it to the Var-name flattening below (matching upstream/origin-main:
		// lossy `a."b"` -> `b`, but syntactically valid) rather than preserving the invalid dotted form.
		if e.Arg("table") != nil {
			return false
		}
		if id, ok := e.Arg("this").(exp.Expression); ok && id.Kind() == exp.KindIdentifier {
			quoted, _ := id.Arg("quoted").(bool)
			return quoted
		}
	}
	return false
}

// parseSetTransaction ports _parse_set_transaction (parser.py:9252-9259). The mode set is
// dialect-scoped: Postgres additionally accepts `[NOT] DEFERRABLE` (pgTransactionCharacteristics), which
// MySQL/base reject — so this is NOT in the shared table (see sets_statements.go).
func (p *Parser) parseSetTransaction(global bool) exp.Expression {
	p.matchTextSeq("TRANSACTION")
	characteristics := transactionCharacteristics
	if p.dialect.Name == "postgres" {
		characteristics = pgTransactionCharacteristics
	}
	modes := p.parseCsv(func() exp.Expression {
		return p.parseVarFromOptions(characteristics, true)
	})
	return p.expression(exp.SetItem(exp.Args{
		"expressions": modes,
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
