package parser

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// pgConfigSpecialParams are Postgres's special multi-word run-time-parameter spellings that SHOW and
// RESET accept in place of a generic dotted name (real PG gram.y var_name productions: `TIME ZONE`,
// `TRANSACTION ISOLATION LEVEL`, `SESSION AUTHORIZATION`). Each is an exact alias of an underscore GUC
// — verified against PostgreSQL 17.6 (`SHOW SESSION AUTHORIZATION` == `SHOW session_authorization`,
// `SHOW TIME ZONE` == `SHOW timezone`, `SHOW TRANSACTION ISOLATION LEVEL` == `SHOW transaction_isolation`)
// — so each phrase canonicalizes to that GUC name: a consumer gets ONE identity per setting regardless
// of which spelling was used (the phrase or the underscore name). Longest phrases are listed first so a
// shorter prefix cannot shadow a longer valid phrase. Each word must be an UNQUOTED keyword token —
// Postgres treats a quoted `"time"` as a parameter name, not the TIME keyword.
var pgConfigSpecialParams = []struct {
	words     []string
	canonical string
}{
	{[]string{"TRANSACTION", "ISOLATION", "LEVEL"}, "transaction_isolation"},
	{[]string{"SESSION", "AUTHORIZATION"}, "session_authorization"},
	{[]string{"TIME", "ZONE"}, "timezone"},
}

// pgConfigNameTokens is the token set a generic Postgres config-parameter name component may be: an
// unquoted VAR, or a quoted IDENTIFIER (matched inside parseIdVar before this set is consulted). It
// deliberately EXCLUDES the reserved-keyword tokens that idVarTokens carries (NULL/TRUE/FALSE/DEFAULT/
// CURRENT_USER/SESSION_USER/…) — Postgres rejects those as a var_name (`SHOW NULL`/`RESET CURRENT_USER`
// → syntax error), and every real GUC name lexes as a plain VAR (or a quoted identifier), so this
// accepts all valid names while failing the reserved ones closed.
var pgConfigNameTokens = map[tokens.TokenType]bool{tokens.VAR: true}

// parsePostgresConfigParam parses a Postgres run-time-configuration parameter reference as used by
// SHOW and RESET: `{ ALL | <special phrase> | name[.name] }`, and returns a CANONICAL identity for it
// — case-folded and unquoted — so a consumer can gate on a privilege-relevant GUC (role,
// session_authorization, …) WITHOUT reimplementing Postgres's case/quote-insensitive GUC lookup (real
// PG: `RESET ROLE` == `reset role` == `RESET "RoLe"`; `SHOW "Search_Path"` == `SHOW search_path`).
// The special multi-word phrases canonicalize to their fixed uppercase spelling (matching SET's
// SetItem.kind), `ALL` to "ALL", and a generic dotted name to its ASCII-lowercased, unquoted spelling.
// The whole statement must end right after the parameter; a trailing token (`search_path extra`), a
// reserved word Postgres rejects as a var_name (`NULL`/`DEFAULT`/`CURRENT_USER`/…), or any non-name
// token fails closed. On failure it retreats and returns ("", false) so the caller degrades to a raw
// Command.
//
// showAll: SHOW treats a quoted `"all"` as the ALL form too (real PG: `SHOW "all"` lists every
// setting), whereas RESET treats `"all"` as an ordinary parameter named `all` — so only SHOW passes
// showAll=true.
func (p *Parser) parsePostgresConfigParam(showAll bool) (string, bool) {
	startIndex := p.index

	// Special multi-word phrases -> their canonical underscore-GUC name.
	for _, phrase := range pgConfigSpecialParams {
		if p.matchUnquotedTextSeq(phrase.words...) {
			if p.atStatementEnd() {
				return phrase.canonical, true
			}
			p.retreat(startIndex)
			return "", false
		}
	}
	// ALL (unquoted keyword) -> "ALL".
	if p.matchUnquotedTextSeq("ALL") {
		if p.atStatementEnd() {
			return "ALL", true
		}
		p.retreat(startIndex)
		return "", false
	}
	// Generic dotted GUC name: name[.name], each part a VAR or quoted identifier, folded to canonical
	// lowercase. Reserved-keyword tokens, operators, numbers, and strings all fail closed.
	first := p.parseConfigNameIdent()
	if first == "" {
		p.retreat(startIndex)
		return "", false
	}
	parts := []string{first}
	if p.match(tokens.DOT) {
		second := p.parseConfigNameIdent()
		if second == "" {
			p.retreat(startIndex)
			return "", false
		}
		parts = append(parts, second)
	}
	if !p.atStatementEnd() {
		p.retreat(startIndex)
		return "", false
	}
	name := strings.Join(parts, ".")
	// A single unquoted-or-quoted `all` collides with the ALL keyword on regeneration: `SHOW "all"`
	// (a parameter literally named `all`) would round-trip to `SHOW all` = the ALL form. For SHOW that
	// IS the ALL form (real PG: `SHOW "all"` lists every setting). For RESET it is NOT (`RESET "all"`
	// is an unrecognized parameter, `RESET all` resets everything), so regenerating it unquoted would
	// change the meaning — fail closed rather than launder it.
	if len(parts) == 1 && strings.EqualFold(name, "all") {
		if showAll {
			return "ALL", true
		}
		p.retreat(startIndex)
		return "", false
	}
	return name, true
}

// isPlainConfigIdent reports whether s is a plain lowercase identifier — `[a-z_][a-z0-9_$]*` — i.e. a
// config-name component that regenerates UNQUOTED with the same meaning. A quoted parameter name whose
// folded text is not plain (contains a space, `.`, or other punctuation — e.g. `"SESSION
// AUTHORIZATION"` or `"a.b"`) would, if emitted unquoted, parse as a different construct (the phrase
// form, or a two-part name), so the caller fails it closed instead of laundering it.
func isPlainConfigIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r == '_':
		case i > 0 && (r >= '0' && r <= '9' || r == '$'):
		default:
			return false
		}
	}
	return true
}

// parseConfigNameIdent consumes one component of a Postgres config-parameter name — an unquoted VAR or
// a quoted identifier — and returns its ASCII-lowercased spelling (Postgres folds GUC names
// case-insensitively, quoted or not, per DEVIATIONS §1.1). It returns "" (consuming nothing beyond
// what parseIdVar already retreated) when the current token is not a valid name component.
func (p *Parser) parseConfigNameIdent() string {
	id := p.parseIdVar(false, pgConfigNameTokens)
	if id == nil {
		return ""
	}
	name := stringsLower(id.Name())
	// A quoted name whose folded text is not a plain identifier (a space, `.`, or punctuation — e.g.
	// `"SESSION AUTHORIZATION"` or `"a.b"`) must NOT be accepted: emitting it unquoted would parse as a
	// different construct (the special phrase, or a two-part name). Fail closed instead of laundering.
	if !isPlainConfigIdent(name) {
		return ""
	}
	return name
}

// atStatementEnd reports whether the cursor is at a statement terminator or end of input.
func (p *Parser) atStatementEnd() bool {
	return !p.curr.IsValid() || p.curr.TokenType == tokens.SEMICOLON
}

// parseResetStatement recognizes Postgres `RESET { name | ALL }` (reset a run-time parameter to its
// default), returning nil (consuming nothing) when the leading token is not a Postgres RESET so
// parseStatement falls through to its normal path. RESET is a plain VAR token in this port's Postgres
// tokenizer (see dialects/postgres.go) — unlike pinned upstream, which Commands it, and unlike MySQL,
// whose `RESET MASTER`/`RESET REPLICA` stays a privileged raw Command. Once the leading VAR is a
// Postgres RESET this owns the statement: a valid parameter builds exp.Reset, anything else fails
// closed to a raw Command (never a mis-parsed expression). Grammar extension, ledger id pg-reset.
func (p *Parser) parseResetStatement() exp.Expression {
	if p.dialect.Name != "postgres" || p.curr.TokenType != tokens.VAR || stringsUpper(p.curr.Text) != "RESET" {
		return nil
	}
	resetTok := p.curr
	comments := p.curr.Comments
	p.advance() // consume RESET
	if name, ok := p.parsePostgresConfigParam(false); ok {
		return p.expression(exp.Reset(exp.Args{"this": name}), nil, comments)
	}
	return p.parseAsCommand(resetTok)
}
