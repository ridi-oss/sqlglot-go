package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// parseSetItemPassword parses MySQL `SET PASSWORD [FOR user] = value` into
// SetItem{kind:"PASSWORD", this:<user|nil>, expressions:[<value>]}, so a consumer reads
// kind="PASSWORD" (an account mutation) structurally instead of scanning the raw Command tail.
// Beyond pinned upstream, which Commands the FOR form (and parses the bare `SET PASSWORD = x` as a
// plain assignment). `PASSWORD` is already consumed by the SET dispatch. Fails closed (returns nil
// → the statement degrades to Command) when a required part is missing.
// setPasswordAssignDelimiters is the strict `=` (or `:=`) assignment delimiter for SET PASSWORD —
// deliberately excluding `TO`, which MySQL only accepts before the `RANDOM` keyword.
var setPasswordAssignDelimiters = map[string]bool{"=": true, ":=": true}

func (p *Parser) parseSetItemPassword() exp.Expression {
	var user exp.Expression
	// matchUnquotedTextSeq (not matchTextSeq) so a quoted `FOR`/`RANDOM` — a backtick identifier, never
	// the keyword — cannot be laundered into the account-mutation form (MySQL rejects those).
	if p.matchUnquotedTextSeq("FOR") {
		user = p.parseMySQLUserSpec()
		if user == nil {
			return nil
		}
	}
	// MySQL 8.0 accepts exactly two value grammars — `= '<auth_string>'` or `TO RANDOM`; anything
	// else (a bare/identifier value, `= RANDOM`, `TO '<string>'`, `= PASSWORD(...)`) is invalid and
	// fails closed rather than be normalized into a valid mutation.
	var value exp.Expression
	switch {
	// The delimiter must be an unquoted `=`/`:=`; a backtick-quoted `` `=` `` is an identifier, never
	// the operator (MySQL rejects it), so exclude IDENTIFIER — matchTexts already excludes STRING but
	// would otherwise match the quoted identifier by text and launder it into a valid `= '…'` mutation.
	case p.curr.TokenType != tokens.IDENTIFIER && p.matchTexts(setPasswordAssignDelimiters):
		// The auth string must be a plain string literal `'…'`; a placeholder (`= ?`), number, or
		// bare identifier is invalid MySQL and fails closed (parseString would otherwise fall back to
		// a Placeholder for `?`).
		if p.curr.TokenType != tokens.STRING {
			return nil
		}
		value = p.parseString()
		if value == nil {
			return nil
		}
	case p.matchUnquotedTextSeq("TO", "RANDOM"):
		value = p.expression(exp.Var(exp.Args{"this": "RANDOM"}), nil, nil)
	default:
		return nil
	}
	return p.expression(exp.SetItem(exp.Args{
		"kind":        "PASSWORD",
		"this":        user,
		"expressions": []exp.Expression{value},
	}), nil, nil)
}

// parseShowCreateUser parses MySQL `SHOW CREATE USER <user>` into Show{this:"CREATE USER",
// target:<user spec>}. Unlike the generic showParser it is STRICT: a user target is required, and
// no trailing clause is allowed (CREATE USER accepts none — MySQL rejects `SHOW CREATE USER 'u'
// LIKE …`/`FROM …`/`LIMIT …` and a bare `SHOW CREATE USER`). Returning nil makes parseShow fail
// closed to a raw Command. The "CREATE USER" keyword is already consumed by the dispatch.
func (p *Parser) parseShowCreateUser() exp.Expression {
	user := p.parseMySQLUserSpec()
	if user == nil {
		return nil
	}
	// No clause may follow (only a statement terminator / end of input).
	if p.curr.IsValid() && p.curr.TokenType != tokens.SEMICOLON {
		return nil
	}
	return p.expression(exp.Show(exp.Args{"this": "CREATE USER", "target": user}), nil, nil)
}

// mysqlUserHostTokens are the tokens a MySQL host part (`user@<host>`) may be: an identifier/backtick,
// a string, or a number/IP-shaped token (`'%'`, localhost, 123). It excludes operators, so a bare
// unquoted `%` (`u@%`, an operator token) fails closed — only `'u'@'%'` is valid MySQL.
var mysqlUserHostTokens = map[tokens.TokenType]bool{
	tokens.VAR: true, tokens.IDENTIFIER: true, tokens.STRING: true, tokens.NUMBER: true,
}

// parseMySQLUserSpec parses a MySQL user `name[@host]` (or `CURRENT_USER[()]`), preserving the exact
// source spelling — quoting included — as a Var so the clause round-trips byte-for-byte. It fails
// closed (returns nil) for a missing name / leading `@host`, empty parens on a non-CURRENT_USER name
// (`foo()`), or a host on CURRENT_USER (`CURRENT_USER@'h'`). The account NAME uses the permissive
// any-token path: MySQL accepts most non-reserved keywords as an unquoted account name (`session`,
// `begin`, …), and this port does not model MySQL's full reserved-word table, so a reserved keyword
// name may be over-accepted here — a FAIL-SAFE fidelity gap, since a consumer gates on the statement
// kind (PASSWORD / CREATE USER), not the account name. Verified against MySQL 8.0.33. See DEVIATIONS.
func (p *Parser) parseMySQLUserSpec() exp.Expression {
	// A user spec starts with a name, never the `@` host separator (reject a leading `@'host'`) and
	// never a bare number (`SHOW CREATE USER 123` is invalid MySQL — a number is not an account name).
	if p.curr.TokenType == tokens.PARAMETER || p.curr.TokenType == tokens.NUMBER {
		return nil
	}
	start := p.curr
	isCurrentUser := p.curr.TokenType == tokens.CURRENT_USER
	if p.parseIdVar(true, nil) == nil {
		return nil
	}
	end := p.prev
	// Empty parens are valid ONLY for CURRENT_USER (`CURRENT_USER()`); `foo()` is not a MySQL user.
	if p.match(tokens.L_PAREN) {
		if !isCurrentUser || !p.match(tokens.R_PAREN) {
			return nil
		}
		end = p.prev
	}
	if p.match(tokens.PARAMETER) { // the '@' between user and host
		// CURRENT_USER takes no host (`CURRENT_USER@'h'` is invalid MySQL).
		if isCurrentUser || !mysqlUserHostTokens[p.curr.TokenType] {
			return nil
		}
		p.advance()
		end = p.prev
	}
	return p.expression(exp.Var(exp.Args{"this": p.findSQL(start, end)}), nil, nil)
}
