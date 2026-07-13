package parser

import (
	"strings"

	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.GRANT] = (*Parser).parseGrant
	statementParsers[tokens.REVOKE] = (*Parser).parseRevoke
}

// grantPrincipalKinds mirrors the ("ROLE", "GROUP") literal tuple matched inline by
// parser.py:9709 (_parse_grant_principal).
var grantPrincipalKinds = map[string]bool{"ROLE": true, "GROUP": true}

// revokeCascadeKinds mirrors the ("CASCADE", "RESTRICT") literal tuple matched inline by
// parser.py:9769 (_parse_revoke).
var revokeCascadeKinds = map[string]bool{"CASCADE": true, "RESTRICT": true}

// parseGrantPrivilege ports parser.py:9690-9706 (_parse_grant_privilege): keep consuming
// consecutive keyword tokens until a PRIVILEGE_FOLLOW_TOKENS boundary (ON / COMMA /
// L_PAREN) is met, joining them into a single Var (e.g. "ALL PRIVILEGES"), then an
// optional parenthesized column list (e.g. UPDATE(col1, col2)).
func (p *Parser) parseGrantPrivilege() exp.Expression {
	var privilegeParts []string
	for p.curr.IsValid() && !p.matchSet(privilegeFollowTokens, false) {
		privilegeParts = append(privilegeParts, stringsUpper(p.curr.Text))
		p.advance()
	}

	this := exp.Var(exp.Args{"this": strings.Join(privilegeParts, " ")})

	var expressions []exp.Expression
	if p.match(tokens.L_PAREN, false) {
		expressions = p.parseWrappedCsv(p.parseColumn)
	}

	return p.expression(exp.GrantPrivilege(exp.Args{"this": this, "expressions": expressions}), nil, nil)
}

// parseGrantPrincipal ports parser.py:9708-9715 (_parse_grant_principal).
func (p *Parser) parseGrantPrincipal() exp.Expression {
	var kind string
	if p.matchTexts(grantPrincipalKinds) {
		kind = stringsUpper(p.prev.Text)
	}

	principal := p.parseIdVar(true, nil)
	if principal == nil {
		return nil
	}

	return p.expression(exp.GrantPrincipal(exp.Args{"this": principal, "kind": kind}), nil, nil)
}

// parseGrantRevokeCommon ports parser.py:9717-9729 (_parse_grant_revoke_common): the
// privilege list and securable-object parsing shared by GRANT and REVOKE. The securable
// is parsed under tryParse because some dialects allow securables that aren't easily
// parseable yet (e.g. MySQL's "foo.*"/"*.*"), in which case the caller degrades to a
// Command.
func (p *Parser) parseGrantRevokeCommon() ([]exp.Expression, string, exp.Expression) {
	privileges := p.parseCsv(p.parseGrantPrivilege)

	p.match(tokens.ON)
	var kind string
	if p.matchSet(creatables) {
		kind = stringsUpper(p.prev.Text)
	}

	// Attempt to parse the securable e.g. MySQL allows names such as "foo.*", "*.*"
	// which are not easily parseable yet.
	securable := p.tryParse(func() exp.Expression { return p.parseTableParts(false, false, false, false) }, false)

	return privileges, kind, securable
}

// parseGrant ports parser.py:9731-9754 (_parse_grant): a GRANT that isn't a securable +
// TO principal-list we can parse structurally, or that carries trailing tokens after the
// (optional) WITH GRANT OPTION, degrades to a Command.
func (p *Parser) parseGrant() exp.Expression {
	start := p.prev

	privileges, kind, securable := p.parseGrantRevokeCommon()

	if securable == nil || !p.matchTextSeq("TO") {
		return p.parseAsCommand(start)
	}

	principals := p.parseCsv(p.parseGrantPrincipal)

	grantOption := p.matchTextSeq("WITH", "GRANT", "OPTION")

	if p.curr.IsValid() {
		return p.parseAsCommand(start)
	}

	return p.expression(exp.Grant(exp.Args{
		"privileges":   privileges,
		"kind":         kind,
		"securable":    securable,
		"principals":   principals,
		"grant_option": grantOption,
	}), nil, nil)
}

// parseRevoke ports parser.py:9756-9784 (_parse_revoke).
func (p *Parser) parseRevoke() exp.Expression {
	start := p.prev

	grantOption := p.matchTextSeq("GRANT", "OPTION", "FOR")

	privileges, kind, securable := p.parseGrantRevokeCommon()

	if securable == nil || !p.matchTextSeq("FROM") {
		return p.parseAsCommand(start)
	}

	principals := p.parseCsv(p.parseGrantPrincipal)

	var cascade string
	if p.matchTexts(revokeCascadeKinds) {
		cascade = stringsUpper(p.prev.Text)
	}

	if p.curr.IsValid() {
		return p.parseAsCommand(start)
	}

	return p.expression(exp.Revoke(exp.Args{
		"privileges":   privileges,
		"kind":         kind,
		"securable":    securable,
		"principals":   principals,
		"grant_option": grantOption,
		"cascade":      cascade,
	}), nil, nil)
}
