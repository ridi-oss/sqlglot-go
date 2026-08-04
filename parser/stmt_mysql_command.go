package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// mysqlCommandLeaders are MySQL statement-initial keywords that upstream leaves as bare VAR
// tokens, so its generic expression path mis-coerces them into an expression node rather than a
// statement — STOP REPLICA/SLAVE/GROUP_REPLICATION, FLUSH …, UNLOCK INSTANCE, XA …, BINLOG '…',
// HELP '…' become an Alias (`STOP AS REPLICA`) and bare RESTART/SHUTDOWN a Column. None is a
// plausible bare identifier-statement, so at statement start each is really a command with no
// structural model here; degrade the whole statement to a raw Command (fail-closed downstream)
// instead of an expression. divergence: matches real MySQL 8.0 (these are statements, not
// `<kw> AS <x>`); see DEVIATIONS §1.
//
// Scope notes: UNLOCK/LOCK TABLES already tokenize as a single COMMAND keyword (dialects/mysql.go)
// and are handled by the Commands-set path in parseStatement before this runs, so only the bare
// `UNLOCK` (UNLOCK INSTANCE) reaches here — both spellings end up Command. Making XA a leader also
// unifies XA START/END/PREPARE/COMMIT/ROLLBACK (upstream parse-errors) into Command. LOCK is
// deliberately absent: LOCK TABLES is already Command and LOCK INSTANCE FOR BACKUP fails closed.
var mysqlCommandLeaders = map[string]bool{
	"STOP":     true,
	"FLUSH":    true,
	"UNLOCK":   true,
	"XA":       true,
	"BINLOG":   true,
	"HELP":     true,
	"RESTART":  true,
	"SHUTDOWN": true,
}

// parseMysqlCommandStatement dispatches a MySQL command statement by its leading keyword text
// (see mysqlCommandLeaders), mirroring parseSavepointStatement/parseResetStatement: it is called
// only at statement start, so `SELECT stop FROM t` and other identifier uses of these words are
// untouched. Returns nil (consuming nothing) when the leading token is not one of these bare-VAR
// leaders, so parseStatement falls through to its normal expression path.
func (p *Parser) parseMysqlCommandStatement() exp.Expression {
	if p.dialect.Name != "mysql" || p.curr.TokenType != tokens.VAR {
		return nil
	}
	if !mysqlCommandLeaders[stringsUpper(p.curr.Text)] {
		return nil
	}
	start := p.curr
	p.advance()
	return p.parseAsCommand(start)
}
