package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// parseSetItemRoleMySQL parses MySQL `SET ROLE { DEFAULT | NONE | ALL [EXCEPT role,...] | role,... }`
// into SetItem{kind:"ROLE", …} — a privilege operation a consumer gates like the Postgres `SET ROLE`
// form (which shares kind="ROLE"). Beyond pinned upstream, which Commands it; ledger id mysql-set-role.
// The `ROLE` keyword is already consumed by the SET dispatch. Fails closed (nil → Command) on a
// malformed tail. Shape:
//
//   - DEFAULT / NONE / ALL     -> this = Var(<keyword>)
//   - ALL EXCEPT r1, r2, …     -> this = Var("ALL"), except = true, expressions = [r1, r2, …]
//   - r1, r2, …                -> expressions = [r1, r2, …]
//
// A role is a MySQL account name `name[@host]` (parseMySQLRoleName — strict: rejects reserved keywords
// and CURRENT_USER as a name). DEFAULT/NONE/ALL/EXCEPT are matched UNQUOTED, so a quoted `SET ROLE 'ALL'`
// is a role literally named ALL, not the keyword (matches MySQL).
func (p *Parser) parseSetItemRoleMySQL() exp.Expression {
	// `SET ROLE = x` / `SET ROLE := x` is the generic MySQL assignment production — an attempt to set a
	// run-time variable literally named `role` (MySQL 8.0.33 parses it, erroring only semantically with
	// "Unknown system variable 'ROLE'"), NOT the privileged `SET ROLE <name>` form. Since ROLE is a
	// dispatch key, findParser reached here having consumed it; on a following `=`/`:=` delimiter, retreat
	// one token to re-expose `role` as the assignment LHS and parse the readable assignment — restoring
	// the pre-slice behavior and mirroring the Postgres precedent (parseSetItemRole). MySQL has NO `TO`
	// assignment delimiter, so `SET ROLE TO admin` (ERROR 1064) is deliberately not treated as an
	// assignment and falls closed to Command.
	if p.isMySQLSetAssignmentDelimiterAhead() {
		p.retreat(p.index - 1) // ROLE is a single-token dispatch key; re-expose it as the LHS.
		return p.parseSetItemAssignment(nil)
	}
	item := p.parseMySQLSetRoleForm()
	if item == nil {
		return nil
	}
	// SET ROLE is standalone: nothing may follow a complete form — not even a trailing comma. A dangling
	// comma after a keyword alternative (`SET ROLE ALL,` / `NONE,` / `DEFAULT,` -> ERROR 1064) is not a
	// role list, so it must fail closed rather than be silently eaten by the outer CSV and regenerated as
	// the different, valid `SET ROLE ALL`. (The list branches already fail inside parseMySQLRoleList on a
	// dangling comma, and a non-comma trailing token is caught by parseSet's leftover-token guard; this
	// closes the keyword-alternative branches, whose parse ends before the comma.)
	if p.curr.TokenType == tokens.COMMA {
		return nil
	}
	return item
}

// parseMySQLSetRoleForm parses the body of `SET ROLE …` after the ROLE keyword: one of the DEFAULT /
// NONE / ALL [EXCEPT …] keyword alternatives, or a role list. Returns nil (→ Command) on a malformed
// tail; the caller (parseSetItemRoleMySQL) additionally rejects a dangling comma after a complete form.
func (p *Parser) parseMySQLSetRoleForm() exp.Expression {
	switch {
	case p.matchUnquotedTextSeq("DEFAULT"):
		return p.mysqlRoleItem("ROLE", exp.Var(exp.Args{"this": "DEFAULT"}), false, nil, nil)
	case p.matchUnquotedTextSeq("NONE"):
		return p.mysqlRoleItem("ROLE", exp.Var(exp.Args{"this": "NONE"}), false, nil, nil)
	case p.matchUnquotedTextSeq("ALL"):
		if p.matchUnquotedTextSeq("EXCEPT") {
			roles, ok := p.parseMySQLRoleList()
			if !ok {
				return nil
			}
			return p.mysqlRoleItem("ROLE", exp.Var(exp.Args{"this": "ALL"}), true, roles, nil)
		}
		return p.mysqlRoleItem("ROLE", exp.Var(exp.Args{"this": "ALL"}), false, nil, nil)
	default:
		roles, ok := p.parseMySQLRoleList()
		if !ok {
			return nil
		}
		return p.mysqlRoleItem("ROLE", nil, false, roles, nil)
	}
}

// parseSetItemDefaultRoleMySQL parses MySQL `SET DEFAULT ROLE { NONE | ALL | role,... } TO user,...`
// into SetItem{kind:"DEFAULT ROLE", …} — the admin operation that sets a user's default roles (a
// distinct, also privilege-relevant statement from `SET ROLE`). The `DEFAULT ROLE` keyword pair is
// already consumed by the SET dispatch; the `TO user,...` clause is required. Ledger id
// mysql-set-default-role.
func (p *Parser) parseSetItemDefaultRoleMySQL() exp.Expression {
	var this exp.Expression
	var roles []exp.Expression
	switch {
	case p.matchUnquotedTextSeq("NONE"):
		this = exp.Var(exp.Args{"this": "NONE"})
	case p.matchUnquotedTextSeq("ALL"):
		this = exp.Var(exp.Args{"this": "ALL"})
	default:
		var ok bool
		roles, ok = p.parseMySQLRoleList()
		if !ok {
			return nil
		}
	}
	if !p.matchUnquotedTextSeq("TO") {
		return nil
	}
	users, ok := p.parseMySQLRoleList()
	if !ok {
		return nil
	}
	return p.mysqlRoleItem("DEFAULT ROLE", this, false, roles, users)
}

// mysqlRoleItem assembles the SetItem for a MySQL role statement (SET ROLE / SET DEFAULT ROLE).
func (p *Parser) mysqlRoleItem(kind string, this exp.Expression, except bool, roles, to []exp.Expression) exp.Expression {
	args := exp.Args{"kind": kind}
	if this != nil {
		args["this"] = this
	}
	if len(roles) > 0 {
		args["expressions"] = roles
	}
	if except {
		args["except"] = true
	}
	if len(to) > 0 {
		args["to"] = to
	}
	return p.expression(exp.SetItem(args), nil, nil)
}

// parseMySQLRoleList parses a comma-separated list of MySQL role/user names `name[@host]` (the roles
// in `SET ROLE …` / the `ALL EXCEPT` list, or the users in a `SET DEFAULT ROLE … TO` clause).
//
// It is STRICT, not the generic parseCsv: a comma MUST be followed by another element, so a leading,
// trailing, or doubled comma fails closed (ok=false) instead of being silently dropped. parseCsv
// tolerates a dangling separator (`admin,` -> [admin]), which would launder MySQL-invalid SQL
// (`SET ROLE admin,` is ERROR 1064 on MySQL 8.0.33) into a structured, executable role mutation — the
// exact quote-blind-laundering class a gating consumer must never see. Returns ok=false (never a
// partial list) whenever any element is absent or invalid.
func (p *Parser) parseMySQLRoleList() ([]exp.Expression, bool) {
	first := p.parseMySQLRoleName()
	if first == nil {
		return nil, false
	}
	roles := []exp.Expression{first}
	for p.match(tokens.COMMA) {
		next := p.parseMySQLRoleName()
		if next == nil {
			return nil, false
		}
		roles = append(roles, next)
	}
	return roles, true
}

// mysqlSetAssignmentDelimiters are MySQL's SET assignment delimiters — `=` and `:=` only. Unlike
// Postgres (setAssignmentDelimiters), MySQL has NO `TO` form (`SET x TO y` / `SET ROLE TO r` are both
// ERROR 1064 on 8.0.33), so `TO` must not be treated as an assignment delimiter here.
var mysqlSetAssignmentDelimiters = map[string]bool{"=": true, ":=": true}

// isMySQLSetAssignmentDelimiterAhead reports whether the current token is a MySQL `=`/`:=` assignment
// delimiter — used to disambiguate `SET ROLE = x` (a generic variable assignment) from the privileged
// `SET ROLE <name>` form. Mirrors isSetAssignmentDelimiterAhead but excludes `TO` (not a MySQL
// delimiter) and, like it, ignores STRING / quoted IDENTIFIER tokens whose text merely collides.
func (p *Parser) isMySQLSetAssignmentDelimiterAhead() bool {
	if p.curr.TokenType == tokens.STRING || p.curr.TokenType == tokens.IDENTIFIER {
		return false
	}
	return mysqlSetAssignmentDelimiters[stringsUpper(p.curr.Text)]
}

// mysqlNonRoleNameWords are words that cannot be an UNQUOTED role/user name in `SET ROLE` /
// `SET DEFAULT ROLE` even though they are NOT in the MySQL reserved-keyword table — so IsReservedKeyword
// misses them and they need this explicit guard. Two grammar categories from MySQL's `sql_yacc.yy`:
//
//   - the SET ROLE top-level alternatives `{ DEFAULT | NONE | ALL | role,… }` — a bare `NONE`/`ALL`/
//     `DEFAULT` is the keyword form, never a list name, so `SET ROLE admin, NONE` is ERROR 1064 (ALL and
//     DEFAULT are also in the reserved table; NONE is not);
//   - `role_keyword` deliberately EXCLUDES the `ident_keywords_ambiguous_1_roles_and_labels`
//     (EXECUTE, RESTART, SHUTDOWN) and `ident_keywords_ambiguous_3_roles` (EVENT, FILE, NONE, PROCESS,
//     PROXY, RELOAD, REPLICATION, RESOURCE, SUPER) categories — nonreserved keywords valid as an
//     identifier everywhere else (`SELECT 1 AS EVENT`) but a syntax error specifically in role-name
//     position, in both the role list and the `TO` target (`SET ROLE EVENT`,
//     `SET DEFAULT ROLE admin TO SUPER` -> ERROR 1064).
//
// A QUOTED name (`'NONE'`, `'EVENT'`) is a valid role name and bypasses this guard, matching MySQL.
// Verified exhaustively against MySQL 8.0.33: these are the only non-reserved words the server rejects
// as a role/user name (dual-review round 3, Codex, 757-keyword sweep both positions).
var mysqlNonRoleNameWords = map[string]bool{
	"NONE": true, "ALL": true, "DEFAULT": true,
	"EXECUTE": true, "RESTART": true, "SHUTDOWN": true,
	"EVENT": true, "FILE": true, "PROCESS": true, "PROXY": true,
	"RELOAD": true, "REPLICATION": true, "RESOURCE": true, "SUPER": true,
}

// isPlainMySQLName reports whether s is a single unquoted MySQL identifier word: one or more identifier
// characters (`[0-9A-Za-z$_]` or an extended rune >= U+0080), not all-digits, and no embedded
// whitespace. It is a text gate on the CURRENT token's text — the tokenizer produces a non-identifier
// text for every non-name lexeme (`?`, `<=`, `->`, a hex/bit literal's digits, a bare number) and a
// space-joined text for a MULTIWORD keyword token (`CHARACTER SET`, `GROUP BY`), so all of those return
// false. A quoted name never reaches here (its content may be anything).
func isPlainMySQLName(s string) bool {
	if s == "" {
		return false
	}
	hasNonDigit := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_', r == '$', r >= 0x80:
			hasNonDigit = true
		default:
			return false
		}
	}
	return hasNonDigit
}

// parseMySQLRoleName parses one MySQL role/user `name[@host]`, preserving the exact source spelling
// (quoting included) as a Var so the clause round-trips. An unquoted name must be a word MySQL accepts
// as an account name. Two authoritative checks: the pinned MySQL reserved-keyword table
// (Dialect.IsReservedKeyword) — MySQL forbids an unquoted RESERVED word as an account name but accepts
// nonreserved ones — plus mysqlNonRoleNameWords for the nonreserved words MySQL's `role_keyword` grammar
// nonetheless excludes. Together they are the right oracle in BOTH directions the tokenizer gets wrong:
//   - reserved words the tokenizer lexes as a plain VAR (`TO`, `ACCESSIBLE`, `ADD`, …) — rejected here,
//     matching MySQL's ERROR 1064, instead of laundering into a structured role mutation;
//   - nonreserved words the tokenizer lexes as a dedicated keyword token (`BEGIN`, `SESSION`,
//     `FORMAT`, …) — accepted here, matching MySQL, instead of a fail-safe degrade to Command.
//
// CURRENT_USER / TRUE / FALSE / NULL are reserved, so they are rejected too (`SET ROLE CURRENT_USER`,
// `… TO CURRENT_USER` -> ERROR 1064), as are the `role_keyword`-excluded nonreserved words (EVENT, SUPER,
// …) via mysqlNonRoleNameWords. A QUOTED name (backtick IDENTIFIER or 'string') may be any word, reserved
// included, matching MySQL. Fails closed (returns nil) on a leading `@host`, a bare number, or a
// non-adjacent host. Multi-token unquoted hosts (dotted/IP, `admin@127.0.0.1`) degrade to a single-token
// host and fail closed downstream — the FAIL-SAFE under-acceptance parseMySQLUserSpec documents
// (DEVIATIONS.md); a consumer denies the resulting Command.
func (p *Parser) parseMySQLRoleName() exp.Expression {
	if quoted := p.curr.TokenType == tokens.STRING || p.curr.TokenType == tokens.IDENTIFIER; !quoted {
		// Unquoted: the name must be a single plain-identifier word — reject anything the tokenizer
		// lexed as a placeholder (`?`), number, hex/bit literal (`x'6162'`), operator (`<=`, `->`), or a
		// MULTIWORD keyword token whose text spans two words (`CHARACTER SET`, `GROUP BY`) — all ERROR
		// 1064 on MySQL, none a valid account name. isPlainMySQLName (identifier chars only, not
		// all-digit) is the single-token gate; then the word must not be a MySQL reserved keyword nor a
		// role_keyword-excluded nonreserved word (NONE/ALL/DEFAULT + EVENT/SUPER/…).
		if !isPlainMySQLName(p.curr.Text) {
			return nil
		}
		if p.dialect.IsReservedKeyword(p.curr.Text) || mysqlNonRoleNameWords[stringsUpper(p.curr.Text)] {
			return nil
		}
	}
	// The name is exactly one token (quoted string/identifier, or the validated plain word); advance it
	// directly rather than via the permissive parseIdVar, which would also admit placeholders/operators.
	start := p.curr
	p.advance()
	end := p.prev
	if p.match(tokens.PARAMETER) { // the '@' between name and host
		// The host must be adjacent to the `@` — MySQL 8.0.33 rejects a gap after it
		// (`SET ROLE admin@ localhost` / `admin @ localhost` -> ERROR 1064), though it tolerates a
		// space BEFORE the `@` (`admin @localhost` -> parsed). Without this, findSQL would reconstruct
		// the spaced source verbatim and round-trip an engine-invalid statement back out.
		if !p.isConnected() || !mysqlUserHostTokens[p.curr.TokenType] {
			return nil
		}
		p.advance()
		end = p.prev
	}
	return p.expression(exp.Var(exp.Args{"this": p.findSQL(start, end)}), nil, nil)
}
