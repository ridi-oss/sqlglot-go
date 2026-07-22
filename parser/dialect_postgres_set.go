package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// constraintModes is the option set for `SET CONSTRAINTS ... { DEFERRED | IMMEDIATE }`; Postgres
// accepts only those two words in the mode slot (unlike TRANSACTION_KIND, which also has EXCLUSIVE).
var constraintModes = optionsType{"DEFERRED": nil, "IMMEDIATE": nil}

// Postgres SET special-forms, structured into `Set{SetItem{kind: ...}}` (a grammar extension
// beyond pinned upstream, which degrades each to a raw Command). The `kind` discriminator lets a
// consumer tell a privileged SET (`ROLE`, `SESSION AUTHORIZATION`) from a benign one
// (`TIME ZONE`, `NAMES`, `CONSTRAINTS`, `SESSION CHARACTERISTICS`) without string-scanning. The
// dispatch keyword is already consumed by findParser (parser_stmt_common.go) before these run.
//
// The optional `SESSION`/`LOCAL` scope prefix on these forms (e.g. `SET LOCAL ROLE r`) is NOT
// modeled here: it is intercepted by the base `SESSION`/`LOCAL` assignment parsers and, finding
// no `= value`, degrades to Command — fail-closed, and safe for the privileged forms. See
// DEVIATIONS "Grammar extensions beyond upstream".

// parseSetSpecialWord parses the value slot of a special SET form: a quoted string, otherwise a
// bare word (identifier / keyword like NONE / DEFAULT), rewritten to a Var so it round-trips bare.
func (p *Parser) parseSetSpecialWord() exp.Expression {
	if this := p.parseString(); this != nil {
		return this
	}
	return p.parseUnquotedField()
}

// parseSetItemRole ports `SET [SESSION|LOCAL] ROLE { role_name | NONE }` (privileged). Returns
// nil (→ fail closed to Command) when the required role value is absent.
func (p *Parser) parseSetItemRole() exp.Expression {
	this := p.parseSetSpecialWord()
	if this == nil {
		return nil
	}
	return p.expression(exp.SetItem(exp.Args{"this": this, "kind": "ROLE"}), nil, nil)
}

// parseSetItemSessionAuthorization ports `SET [SESSION|LOCAL] SESSION AUTHORIZATION
// { user_name | DEFAULT }` (privileged — changes the effective user).
func (p *Parser) parseSetItemSessionAuthorization() exp.Expression {
	this := p.parseSetSpecialWord()
	if this == nil {
		return nil
	}
	return p.expression(exp.SetItem(exp.Args{"this": this, "kind": "SESSION AUTHORIZATION"}), nil, nil)
}

// parseSetItemTimeZone ports `SET [SESSION|LOCAL] TIME ZONE { value | 'value' | LOCAL | DEFAULT |
// INTERVAL '...' ... }` (benign).
func (p *Parser) parseSetItemTimeZone() exp.Expression {
	this := p.parseInterval(true) // INTERVAL '+00:00' HOUR TO MINUTE (nil if not an INTERVAL)
	if this == nil {
		// A signed numeric offset, e.g. `SET TIME ZONE -5` (parsePrimary handles only the number).
		neg := p.match(tokens.DASH)
		if !neg {
			p.match(tokens.PLUS)
		}
		this = p.parsePrimary() // 'UTC', a number
		if this != nil && neg {
			this = p.expression(exp.Neg(exp.Args{"this": this}), nil, nil)
		}
	}
	if this == nil {
		this = p.parseUnquotedField() // LOCAL / DEFAULT (bare words)
	}
	if this == nil {
		return nil
	}
	return p.expression(exp.SetItem(exp.Args{"this": this, "kind": "TIME ZONE"}), nil, nil)
}

// parseSetItemConstraints ports `SET CONSTRAINTS { ALL | name [, ...] } { DEFERRED | IMMEDIATE }`
// (benign). The targets are held in `expressions`, the mode in `this`; see the CONSTRAINTS branch
// in setItemSQL.
func (p *Parser) parseSetItemConstraints() exp.Expression {
	var targets []exp.Expression
	// Unquoted `ALL` is the keyword (all constraints). A *quoted* `"ALL"` is a specific constraint
	// named ALL, so it must go through the name-list branch and keep its quotes on round-trip —
	// regenerating it as the bare keyword would broaden the statement from one constraint to every
	// constraint.
	if p.curr.TokenType != tokens.IDENTIFIER && p.matchTextSeq("ALL") {
		targets = []exp.Expression{exp.Var(exp.Args{"this": "ALL"})}
	} else {
		targets = p.parseCsv(p.parseConstraintName)
	}
	mode := p.parseVarFromOptions(constraintModes, false) // DEFERRED | IMMEDIATE only
	if len(targets) == 0 || mode == nil {
		return nil
	}
	return p.expression(exp.SetItem(exp.Args{"expressions": targets, "this": mode, "kind": "CONSTRAINTS"}), nil, nil)
}

// parseConstraintName parses one possibly schema-qualified constraint name (identifiers joined by
// dots, e.g. `public.foo`) — deliberately a bounded identifier chain, not the general expression
// grammar.
func (p *Parser) parseConstraintName() exp.Expression {
	this := p.parseIdVar(false, nil)
	for this != nil && p.match(tokens.DOT) {
		next := p.parseIdVar(false, nil)
		if next == nil {
			return nil
		}
		this = p.expression(exp.Dot(exp.Args{"this": this, "expression": next}), nil, nil)
	}
	return this
}

// parseSetSessionCharacteristics ports `SET SESSION CHARACTERISTICS AS TRANSACTION
// transaction_mode [, ...]` (benign). "SESSION CHARACTERISTICS" is already consumed; the
// remaining `AS TRANSACTION <modes>` is captured with the shared transaction-mode options, and
// rendered by the SESSION CHARACTERISTICS branch in setItemSQL.
func (p *Parser) parseSetSessionCharacteristics() exp.Expression {
	// `AS TRANSACTION` is mandatory in Postgres — bail (→ Command) if either word is missing.
	if !p.matchTextSeq("AS") || !p.matchTextSeq("TRANSACTION") {
		return nil
	}
	// raiseUnmatched=false so a characteristic outside the modeled set (e.g. DEFERRABLE, or
	// READ UNCOMMITTED — blocked upstream by a typo in the shared characteristics table) fails
	// closed to a Command via the empty-list check, rather than raising a hard parse error.
	characteristics := p.parseCsv(func() exp.Expression {
		return p.parseVarFromOptions(transactionCharacteristics, false)
	})
	if len(characteristics) == 0 {
		return nil
	}
	return p.expression(exp.SetItem(exp.Args{"expressions": characteristics, "kind": "SESSION CHARACTERISTICS"}), nil, nil)
}

// parseSetItemNamesPostgres ports `SET NAMES [ 'value' | DEFAULT ]` (benign — client_encoding).
// Unlike MySQL's SET NAMES, Postgres has no trailing `COLLATE` clause, and the value must be a
// string literal or the `DEFAULT` keyword (a bare `SET NAMES` is also valid). An unquoted charset
// like `SET NAMES utf8` is a syntax error in Postgres, so it is left for the leftover-token guard.
func (p *Parser) parseSetItemNamesPostgres() exp.Expression {
	this := p.parseString()
	if this == nil && p.matchTextSeq("DEFAULT") {
		this = exp.Var(exp.Args{"this": "DEFAULT"})
	}
	return p.expression(exp.SetItem(exp.Args{"this": this, "kind": "NAMES"}), nil, nil)
}
