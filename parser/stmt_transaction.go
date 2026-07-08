package parser

import (
	"strings"

	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.BEGIN] = (*Parser).parseTransaction
	statementParsers[tokens.COMMIT] = (*Parser).parseCommitOrRollback
	statementParsers[tokens.ROLLBACK] = (*Parser).parseCommitOrRollback
	statementParsers[tokens.END] = (*Parser).parseEndTransaction
}

// parseEndTransaction ports the Postgres STATEMENT_PARSERS override
// (parsers/postgres.py:182): `{TokenType.END: lambda self: self._parse_commit_or_rollback()}`.
// The shared statementParsers map is dialect-agnostic (there is no per-dialect override
// table yet - ROADMAP 5b), so the dialect gate lives here: only Postgres treats a leading
// END as a transaction terminator (END -> COMMIT). For base/MySQL, END is an ordinary
// identifier/keyword, so retreat past the token parseStatement consumed and fall back to
// the normal expression path - keeping e.g. `END AND CHAIN` as `END AND CHAIN`.
func (p *Parser) parseEndTransaction() exp.Expression {
	if p.dialect.Name != "postgres" {
		p.retreat(p.index - 1)
		return p.parseExpressionStatement()
	}
	return p.parseCommitOrRollback()
}

// transactionOrWorkTexts mirrors the ("TRANSACTION", "WORK") literal tuple matched inline
// by parser.py:8666,8684 (_parse_transaction / _parse_commit_or_rollback).
var transactionOrWorkTexts = map[string]bool{"TRANSACTION": true, "WORK": true}

// parseTransaction ports parser.py:8662-8680 (_parse_transaction). modes is a permissive,
// comma-separated list of space-joined VAR|NOT runs so it also handles Postgres's
// `BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE, ISOLATION LEVEL SERIALIZABLE` and
// `DEFERRABLE, DEFERRABLE` forms without needing dedicated grammar for each mode.
func (p *Parser) parseTransaction() exp.Expression {
	var this any
	if p.matchTexts(transactionKind) {
		this = p.prev.Text
	}

	p.matchTexts(transactionOrWorkTexts)

	var modes []string
	for {
		var mode []string
		for p.match(tokens.VAR) || p.match(tokens.NOT) {
			mode = append(mode, p.prev.Text)
		}
		if len(mode) > 0 {
			modes = append(modes, strings.Join(mode, " "))
		}
		if !p.match(tokens.COMMA) {
			break
		}
	}

	return p.expression(exp.Transaction(exp.Args{"this": this, "modes": modes}), nil, nil)
}

// parseCommitOrRollback ports parser.py:8682-8700 (_parse_commit_or_rollback). p.prev is
// the leading COMMIT/ROLLBACK token: parseStatement (parser.go:388-392) advances past it
// before dispatching here, matching upstream's use of self._prev.
func (p *Parser) parseCommitOrRollback() exp.Expression {
	var chain any
	var savepoint exp.Expression
	isRollback := p.prev.TokenType == tokens.ROLLBACK

	p.matchTexts(transactionOrWorkTexts)

	if p.matchTextSeq("TO") {
		p.matchTextSeq("SAVEPOINT")
		savepoint = p.parseIdVar(true, nil)
	}

	if p.match(tokens.AND) {
		chain = !p.matchTextSeq("NO")
		p.matchTextSeq("CHAIN")
	}

	if isRollback {
		return p.expression(exp.Rollback(exp.Args{"savepoint": savepoint}), nil, nil)
	}
	return p.expression(exp.Commit(exp.Args{"chain": chain}), nil, nil)
}
