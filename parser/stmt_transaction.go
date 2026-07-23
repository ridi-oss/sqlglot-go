package parser

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.BEGIN] = (*Parser).parseTransaction
	statementParsers[tokens.COMMIT] = (*Parser).parseCommitOrRollback
	statementParsers[tokens.ROLLBACK] = (*Parser).parseCommitOrRollback
	statementParsers[tokens.END] = (*Parser).parseEndTransaction
}

// parseEndTransaction ports the Postgres STATEMENT_PARSERS override
// (parsers/postgres.py:182): `{TokenType.END: lambda self: self._parse_commit_or_rollback()}`.
// The parser-side dialect override registry now supports statement callbacks, but its production
// overlays remain empty for this infrastructure-only slice. Retain the pre-existing END entry in
// the base singleton and this Postgres-only fallback gate for zero behavior change: Postgres treats
// a leading END as a transaction terminator (END -> COMMIT), while base/MySQL retreat past the token
// parseStatement consumed and use the normal expression path, preserving `END AND CHAIN`.
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
		// MySQL's `START TRANSACTION WITH CONSISTENT SNAPSHOT` — WITH lexes as its own token,
		// so it isn't caught by the VAR run below. Gated to MySQL: real MySQL 8.0.33 accepts it,
		// but real PostgreSQL 17.6 rejects it, and pinned upstream errors on it in both. Kept as a
		// single mode string so it round-trips as one unit.
		if p.dialect.Name == "mysql" && p.matchTextSeq("WITH", "CONSISTENT", "SNAPSHOT") {
			modes = append(modes, "WITH CONSISTENT SNAPSHOT")
		} else {
			var mode []string
			// tokens.ONLY is also consumed (in addition to upstream's VAR|NOT) so Postgres
			// `START TRANSACTION READ ONLY` parses: postgres lexes ONLY as a dedicated token
			// (for `TABLE ONLY`), not VAR, but here it is just a transaction-mode word.
			for p.match(tokens.VAR) || p.match(tokens.NOT) || p.match(tokens.ONLY) {
				mode = append(mode, p.prev.Text)
			}
			if len(mode) > 0 {
				modes = append(modes, strings.Join(mode, " "))
			}
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

// parseSavepointStatement recognizes the ANSI transaction-control statements `SAVEPOINT <name>`
// and `RELEASE [SAVEPOINT] <name>`, returning nil (consuming nothing) when the leading token is not
// one of these so parseStatement falls through to its normal expression path. Pinned upstream models
// neither: it mis-parses `SAVEPOINT s` as an Alias (`SAVEPOINT AS s`) and parse-errors
// `RELEASE SAVEPOINT s`. SAVEPOINT/RELEASE are ordinary VAR tokens (not statement keywords), so this
// is dispatched by leading text, keeping them usable as identifiers everywhere else. The bare
// `RELEASE <name>` spelling (SAVEPOINT keyword omitted) is Postgres-only — real MySQL requires the
// SAVEPOINT keyword — so for mysql/base a bare RELEASE is left to the normal path (fails closed).
// `ROLLBACK TO [SAVEPOINT] <name>` is unaffected (already an exp.Rollback). See DEVIATIONS.
func (p *Parser) parseSavepointStatement() exp.Expression {
	if p.curr.TokenType != tokens.VAR {
		return nil
	}
	switch stringsUpper(p.curr.Text) {
	case "SAVEPOINT":
		comments := p.curr.Comments
		start := p.index
		p.advance()
		name := p.parseSavepointName()
		if name == nil {
			p.retreat(start)
			return nil
		}
		return p.expression(exp.Savepoint(exp.Args{"this": name}), nil, comments)
	case "RELEASE":
		comments := p.curr.Comments
		start := p.index
		p.advance()
		// The SAVEPOINT keyword is optional in Postgres (`RELEASE [SAVEPOINT] name`). It is the
		// keyword only when it is an *unquoted* VAR "SAVEPOINT" immediately followed by a name
		// token; a lone `RELEASE savepoint` or `RELEASE "SAVEPOINT"` releases a savepoint literally
		// named `savepoint`/`SAVEPOINT` (verified on real PG 17.6). matchTextSeq is deliberately
		// NOT used here — it matches a quoted identifier `"SAVEPOINT"` by text, which would swallow
		// the name. When the keyword is absent the bare form is Postgres-only (real MySQL/base
		// require the SAVEPOINT keyword), so mysql/base fall through to the normal path.
		if p.curr.TokenType == tokens.VAR && stringsUpper(p.curr.Text) == "SAVEPOINT" && p.savepointNameAhead(p.next) {
			p.advance()
		} else if p.dialect.Name != "postgres" {
			p.retreat(start)
			return nil
		}
		name := p.parseSavepointName()
		if name == nil {
			p.retreat(start)
			return nil
		}
		return p.expression(exp.Savepoint(exp.Args{"this": name, "kind": "RELEASE"}), nil, comments)
	}
	return nil
}

// parseSavepointName parses a savepoint identifier: an unquoted identifier (including unreserved
// keyword names such as `commit`/`rollback` that both engines accept) or a dialect-quoted identifier
// (Postgres `"x"`, MySQL “ `x` “). It rejects string literals (`'x'`), numbers, and — for MySQL —
// a double-quoted `"x"` (which lexes as a STRING there), matching what real PostgreSQL 17.6 and
// MySQL 8.0.33 accept as savepoint names, rather than accept-and-normalize an invalid name. Uses
// parseIdVar with anyToken=false so STRING/NUMBER tokens (absent from idVarTokens) fail closed.
func (p *Parser) parseSavepointName() exp.Expression {
	return p.parseIdVar(false, nil)
}

// savepointNameAhead reports whether tok can start a savepoint name (see parseSavepointName): a
// quoted IDENTIFIER or any idVarTokens member (VAR + the unreserved keyword tokens). Used to decide
// whether a VAR "SAVEPOINT" after RELEASE is the optional keyword (a name follows) or itself the
// savepoint name (nothing name-like follows).
func (p *Parser) savepointNameAhead(tok tokens.Token) bool {
	return tok.TokenType == tokens.IDENTIFIER || idVarTokens[tok.TokenType]
}
