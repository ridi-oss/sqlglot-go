package expressions

import "strings"

// Statement discriminators — the consumer-facing contract for classifying a parsed statement.
//
// A parsed statement is discriminated at two levels: the node Kind (Kind() / ClassName — "is this a
// Select, a Show, a Command?"), and, for some node kinds, a string arg that sub-types within the
// class (Into.kind, Create/Alter/Drop.kind, SetItem.kind, Show.this).
//
// These string discriminators are CANONICAL: upper-cased and trimmed, with a single stable spelling
// per concept (the parser folds every alias to one label — e.g. SHOW SLAVE STATUS -> "REPLICA
// STATUS"). Read one with Text(key) and compare it directly; no re-normalization (ToUpper/TrimSpace)
// is needed or expected. The contract is enforced by TestStatementDiscriminatorContract (root
// package). Two boundaries:
//
//   - Show.this is canonical only for MySQL SHOW. A Postgres SHOW carries the verbatim
//     config-parameter name instead (e.g. "search_path", "timezone") — lower-case and NOT a label
//     from the Show* set below. A consumer keys on the MySQL labels; a Postgres name never collides
//     with one.
//   - Create/Alter/Drop.kind is canonical when the statement is structured, but WHICH object types
//     are structured is verb- and dialect-dependent (DROP TRIGGER is a structured Drop, CREATE
//     TRIGGER is a Command; Postgres structures CREATE TYPE, MySQL does not). An object type the
//     parser does not structure degrades to a Command, classified via Keyword(). Because that set is
//     verb/dialect-dependent and grows as the port structures more DDL, the object-type vocabulary is
//     intentionally NOT exported as constants — a consumer maps the (canonical) keyword itself.
//
// The Command node is the deliberate exception to canonicalization: it is the raw-text fallback for a
// statement sqlglot-go does not structure, and its this/expression args are kept VERBATIM for
// round-trip fidelity. Read a Command's leading keyword through Keyword() (which normalizes it);
// never key on a Command's untokenized remainder (expression).

// Into file-write kinds (Into.kind) — MySQL server-side SELECT ... INTO writes.
const (
	IntoOutfile  = "OUTFILE"
	IntoDumpfile = "DUMPFILE"
)

// SetItemTransaction is the SetItem.kind for `SET TRANSACTION`. Every SetItem.kind is
// canonical-uppercase; only the form a consumer keys on directly is named here.
const SetItemTransaction = "TRANSACTION"

// SHOW targets (Show.this) — the MySQL SHOW forms distinguished from generic schema-metadata SHOWs.
// Each is the canonical label the parser folds every spelling to. See the Show.this boundary in the
// package doc above: Postgres SHOW carries a verbatim config-parameter name instead, not one of these.
const (
	ShowWarnings       = "WARNINGS"
	ShowErrors         = "ERRORS"
	ShowGrants         = "GRANTS"
	ShowCreateUser     = "CREATE USER"
	ShowProcesslist    = "PROCESSLIST"
	ShowEngine         = "ENGINE"
	ShowBinlogEvents   = "BINLOG EVENTS"
	ShowRelaylogEvents = "RELAYLOG EVENTS"
	ShowMasterStatus   = "MASTER STATUS"
	ShowBinaryLogs     = "BINARY LOGS"
	ShowReplicaStatus  = "REPLICA STATUS"
	ShowReplicas       = "REPLICAS"
)

// Into returns the INTO clause of a query (the `into` arg) as an Expression, or nil when absent — a
// typed read of an arg that would otherwise be fetched and type-asserted by hand.
func (n *Node) Into() Expression { return asExpression(n.args["into"]) }

// Keyword returns the normalized value of a Command node's `this` — upper-cased, trimmed, with
// internal whitespace collapsed to single spaces — so it is stable across both Command builder paths
// (one upper-cases `this`, the other keeps it verbatim) and across multi-word command tokens (MySQL
// `LOCK TABLES`). For every statement sqlglot-go fails closed on, `this` is the leading keyword of
// the fallback, which is the statement's leading keyword. It returns "" for a node that is not a
// Command. Read only this; a Command's untokenized remainder is never a reliable discriminator and
// must not be parsed by consumers.
func (n *Node) Keyword() string {
	if n.kind != KindCommand {
		return ""
	}
	return strings.Join(strings.Fields(strings.ToUpper(n.Text("this"))), " ")
}
