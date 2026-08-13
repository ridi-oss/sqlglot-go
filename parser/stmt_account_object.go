package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// Account-management object statements — MySQL CREATE/ALTER/DROP {USER|ROLE}.
//
// Grammar extension beyond upstream: pinned sqlglot v30.12.0 leaves every one of these as a raw
// exp.Command (USER and ROLE both lex as bare VAR tokens, and neither is a CREATABLE / ALTERABLE, so
// the structured Create/Alter/Drop parsers decline and degrade to Command). A consumer then sees only
// the leading keyword ("CREATE"/"ALTER"/"DROP"), which is shared with the schema-object forms, so it
// cannot tell USER/ROLE apart.
//
// These build a STRUCTURED root (Create/Alter/Drop) carrying the object type as the canonical `kind`
// arg — the same discriminator TABLE uses — so a consumer classifies by root Kind + kind. The
// statement body (the user/role list, IF [NOT] EXISTS, IDENTIFIED BY / RANDOM PASSWORD / WITH plugin /
// REQUIRE / resource-option clauses) is not modeled: it is captured verbatim and re-emitted
// byte-for-byte, exactly as the Command fallback did. Registered in testdata/upstream_extensions.jsonl
// (tripwire) and DEVIATIONS.md.

// mysqlAccountObjectKind reports the account-management object keyword at the current position (the
// token immediately after CREATE/ALTER/DROP), or "" if it is not one allowed. Both USER and ROLE lex
// as bare VAR tokens (neither is in MySQL's tokenizer keyword map). It requires exactly a VAR — so a
// quoted identifier (a table named `user`) or a string literal (`'USER'`) is not mistaken for the
// keyword — and a following token, since every valid form has a body (a user/role name) after the
// object word; a bare `CREATE USER` fails closed to Command. It does not consume. MySQL-only.
func (p *Parser) mysqlAccountObjectKind(allowUser, allowRole bool) string {
	if p.dialect.Name != "mysql" || p.curr.TokenType != tokens.VAR || !p.next.IsValid() {
		return ""
	}
	switch stringsUpper(p.curr.Text) {
	case "USER":
		if allowUser {
			return "USER"
		}
	case "ROLE":
		if allowRole {
			return "ROLE"
		}
	}
	return ""
}

// parseAccountObjectStatement consumes the remainder of the statement and builds a nodeKind
// (KindCreate/KindAlter/KindDrop) root with `kind` set to the canonical object type ("USER"/"ROLE").
// The whole statement is preserved verbatim inside a Command child — held as `this` for Create/Drop and
// as the sole `actions` element for Alter, which requires a non-empty `actions`. start is the leading
// CREATE/ALTER/DROP token; the generator keys on the Command child to emit it. Comments: the verbatim
// findSQL text already carries any inline comments, so the child is built bare (not via p.expression);
// the statement's leading comments attach to the root. The round-trip therefore matches the plain
// Command fallback exactly — including its normalization of a trailing/mid-statement comment.
func (p *Parser) parseAccountObjectStatement(start tokens.Token, nodeKind exp.Kind, kind string) exp.Expression {
	for p.curr.IsValid() {
		p.advance()
	}
	body := exp.Command(exp.Args{"this": p.findSQL(start, p.prev)})

	args := exp.Args{"kind": kind}
	if nodeKind == exp.KindAlter {
		args["actions"] = []exp.Expression{body}
	} else {
		args["this"] = body
	}
	return p.expression(exp.New(nodeKind, args), nil, nil)
}
