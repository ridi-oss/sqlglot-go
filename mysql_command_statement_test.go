package sqlglot_test

import (
	"strings"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// roundTrips parses sql as MySQL, asserts the root Kind, and asserts a byte-identical round-trip.
func assertMySQLKindRoundTrip(t *testing.T, sql string, want exp.Kind) exp.Expression {
	t.Helper()
	e, err := sqlglot.ParseOne(sql, "mysql")
	if err != nil {
		t.Fatalf("parse %q: %v", sql, err)
	}
	if e.Kind() != want {
		t.Fatalf("%q kind = %v, want %v:\n%s", sql, exp.ClassName(e.Kind()), exp.ClassName(want), e.ToS())
	}
	out, gerr := sqlglot.Generate(e, "mysql", generator.Options{})
	if gerr != nil {
		t.Fatalf("generate %q: %v", sql, gerr)
	}
	if out != sql {
		t.Fatalf("round-trip = %q, want %q", out, sql)
	}
	return e
}

// START REPLICA / START SLAVE / START GROUP_REPLICATION are MySQL replication control, NOT a
// transaction. Pinned upstream maps START->BEGIN and eats the verb as a transaction mode, yielding
// Transaction{modes:[REPLICA]} -> `BEGIN REPLICA` — which real MySQL 8.0 rejects with a syntax
// error (1064), the tell that upstream's parse is wrong. Degrading to Command round-trips
// faithfully and, for the downstream consumer, keeps a connect-only principal from starting
// replication via a statement that looks like a session-level transaction. See DEVIATIONS §1.
func TestMySQLStartReplicationIsNotTransaction(t *testing.T) {
	for _, sql := range []string{
		"START REPLICA",
		"START SLAVE",
		"START GROUP_REPLICATION",
		"START REPLICA USER = 'r'",
	} {
		t.Run(sql, func(t *testing.T) {
			e := assertMySQLKindRoundTrip(t, sql, exp.KindCommand)
			if this := e.Text("this"); this != "START" {
				t.Fatalf("Command this = %q, want START", this)
			}
		})
	}
}

// START TRANSACTION / BEGIN and their real transaction modes must stay a Transaction — the fix
// above must divert only the replication verbs.
func TestMySQLStartTransactionStillTransaction(t *testing.T) {
	for _, sql := range []string{
		"BEGIN",
		"START TRANSACTION",
		"START TRANSACTION READ ONLY",
		"START TRANSACTION WITH CONSISTENT SNAPSHOT",
	} {
		t.Run(sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindTransaction {
				t.Fatalf("kind = %v, want Transaction:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
		})
	}
}

// The replication divert is MySQL-only (DEVIATIONS §1.13 scope): Postgres and Presto also map
// START->BEGIN but have no such statements, so `START REPLICA` there must keep upstream's behavior
// (a Transaction), not leak into a Command — those are the dialects where the guard could misfire.
func TestStartReplicationDivertIsMySQLOnly(t *testing.T) {
	for _, dialect := range []string{"postgres", "presto"} {
		t.Run("dialect="+dialect, func(t *testing.T) {
			e, err := sqlglot.ParseOne("START REPLICA", dialect)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindTransaction {
				t.Fatalf("kind = %v, want Transaction (divert must be MySQL-only):\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
		})
	}
}

// MySQL admin statements whose leading keyword is a bare VAR must degrade to a raw Command rather
// than be mis-coerced into an expression node (Alias `STOP AS REPLICA`, Column `RESTART`). None is
// a plausible bare identifier-statement, and Command fails closed downstream. See DEVIATIONS §1.
func TestMySQLCommandLeadersDegradeToCommand(t *testing.T) {
	cases := []struct{ sql, this string }{
		{"STOP REPLICA", "STOP"},
		{"STOP SLAVE", "STOP"},
		{"STOP GROUP_REPLICATION", "STOP"},
		{"FLUSH TABLES", "FLUSH"},
		{"FLUSH PRIVILEGES", "FLUSH"},
		{"FLUSH LOGS", "FLUSH"},
		{"UNLOCK INSTANCE", "UNLOCK"},
		{"XA RECOVER", "XA"},
		{"XA START 'x'", "XA"}, // upstream parse-errors these; unifying under XA -> Command is stricter
		{"XA COMMIT 'x'", "XA"},
		{"BINLOG 'abc'", "BINLOG"},
		{"HELP 'contents'", "HELP"},
		{"RESTART", "RESTART"},
		{"SHUTDOWN", "SHUTDOWN"},
	}
	for _, c := range cases {
		t.Run(c.sql, func(t *testing.T) {
			e := assertMySQLKindRoundTrip(t, c.sql, exp.KindCommand)
			if this := e.Text("this"); this != c.this {
				t.Fatalf("Command this = %q, want %q", this, c.this)
			}
		})
	}
}

// The command-leader/TABLE dispatch fires only at a TOP-LEVEL statement (mixed case included), so
// these words stay usable as ordinary identifiers/values elsewhere — both in an expression position
// and, crucially, at a nested parseStatement entry such as a SET assignment RHS, where committing
// would swallow the rest of the statement.
func TestMySQLCommandLeadersStayUsableAsIdentifiers(t *testing.T) {
	for _, sql := range []string{
		"SELECT stop FROM t",
		"SELECT flush, help FROM t",
		"SELECT xa, binlog, unlock FROM t",
		"SELECT restart FROM shutdown",
		"SELECT * FROM t WHERE stop = 1",
	} {
		t.Run(sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindSelect {
				t.Fatalf("kind = %v, want Select:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
		})
	}
}

// Regression for the nested-parseStatement mis-fire: a command leader or TABLE as a SET assignment
// value must NOT be diverted to a Command/Select. The tell is a multi-value SET keeping every item —
// before the top-level gate, `SET x = stop` committed a Command that swallowed the trailing `, y = 1`.
func TestMySQLCommandLeadersNotDivertedInNestedStatement(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SET x = stop", "SET x = stop"},
		{"SET x = stop, y = 1", "SET x = stop, y = 1"},
		{"SET x = restart, y = 2", "SET x = restart, y = 2"},
		{"SET x = flush", "SET x = flush"},
		{"SET @v = help", "SET @v = help"},
		// TABLE as a value falls through to upstream's (non-Select) handling, not a diverted Select.
		{"SET x = TABLE users", "SET x = `TABLE` AS users"},
	}
	for _, c := range cases {
		t.Run(c.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(c.sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindSet {
				t.Fatalf("kind = %v, want Set:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
			out, gerr := sqlglot.Generate(e, "mysql", generator.Options{})
			if gerr != nil {
				t.Fatalf("generate: %v", gerr)
			}
			if out != c.want {
				t.Fatalf("round-trip = %q, want %q", out, c.want)
			}
		})
	}
}

// A comment on these raw admin Commands must never be DUPLICATED on round-trip (the raw source slice
// already carries an inline comment; attaching it again would double it). Leading comments on the
// text-dispatch leaders are dropped — immaterial for fail-closed admin statements — the guarantee is
// exactly-once, never doubled.
func TestMySQLCommandStatementDoesNotDuplicateComment(t *testing.T) {
	for _, sql := range []string{
		"STOP /* c */ REPLICA",
		"FLUSH /* c */ LOGS",
		"/* c */ STOP REPLICA",
		"/* c */ RESTART",
	} {
		t.Run(sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			out, gerr := sqlglot.Generate(e, "mysql", generator.Options{})
			if gerr != nil {
				t.Fatalf("generate: %v", gerr)
			}
			if strings.Count(out, "/*") > 1 {
				t.Fatalf("comment duplicated: %q -> %q", sql, out)
			}
		})
	}
}

// MySQL 8.0.19+ `TABLE tbl` is exactly `SELECT * FROM tbl` (verified on MySQL 8.0.46: identical
// rows). Pinned upstream mis-parses it as an Alias (“ `TABLE` AS users“); the port builds a real
// Select so a consumer gets the same column lineage as the SELECT form. See DEVIATIONS §1.
func TestMySQLTableStatementIsSelectStar(t *testing.T) {
	for _, tc := range []struct{ table, selectStar string }{
		{"TABLE users", "SELECT * FROM users"},
		{"TABLE db.users", "SELECT * FROM db.users"},
		{"TABLE `weird table`", "SELECT * FROM `weird table`"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			table, sql := tc.table, tc.selectStar
			tableExpr, err := sqlglot.ParseOne(table, "mysql")
			if err != nil {
				t.Fatalf("parse %q: %v", table, err)
			}
			if tableExpr.Kind() != exp.KindSelect {
				t.Fatalf("%q kind = %v, want Select:\n%s", table, exp.ClassName(tableExpr.Kind()), tableExpr.ToS())
			}
			selectExpr, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse %q: %v", sql, err)
			}
			// Same AST as the explicit SELECT * form — the whole point (lineage parity).
			if tableExpr.ToS() != selectExpr.ToS() {
				t.Fatalf("TABLE form AST != SELECT * form:\nTABLE:\n%s\nSELECT:\n%s", tableExpr.ToS(), selectExpr.ToS())
			}
			out, gerr := sqlglot.Generate(tableExpr, "mysql", generator.Options{})
			if gerr != nil {
				t.Fatalf("generate: %v", gerr)
			}
			if out != sql {
				t.Fatalf("round-trip = %q, want %q", out, sql)
			}
		})
	}
}

// Only the bare `TABLE tbl_name` form is modeled. A trailing ORDER BY/LIMIT and bare TABLE fail
// closed with a parse error; a non-identifier operand (table function, placeholder) is rejected so
// it does not fake a Select. CREATE/DROP TABLE and a TABLE keyword inside DDL keep their own parsers.
func TestMySQLTableStatementScope(t *testing.T) {
	for _, sql := range []string{
		"TABLE users ORDER BY id", // trailer unmodeled -> unconsumed tokens
		"TABLE users LIMIT 1",
		"TABLE",     // bare TABLE: no operand
		"TABLE f()", // table function: not a table name
		"TABLE ?",   // placeholder: not a table name
	} {
		t.Run("fails/"+sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err == nil {
				// Fail-closed means: never a bogus Select for these invalid operands/trailers.
				if e.Kind() == exp.KindSelect {
					t.Fatalf("%q: expected fail-closed, got Select:\n%s", sql, e.ToS())
				}
			}
		})
	}
	for _, tc := range []struct {
		sql  string
		want exp.Kind
	}{
		{"CREATE TABLE t (a INT)", exp.KindCreate},
		{"DROP TABLE t", exp.KindDrop},
		{"SELECT table_name FROM t", exp.KindSelect}, // TABLE only as an identifier substring
	} {
		t.Run("unchanged/"+tc.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != tc.want {
				t.Fatalf("kind = %v, want %v", exp.ClassName(e.Kind()), exp.ClassName(tc.want))
			}
		})
	}
}
