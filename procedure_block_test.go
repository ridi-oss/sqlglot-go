package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// CREATE PROCEDURE/FUNCTION bodies: BEGIN...END blocks (exp.Block + exp.EndStatement,
// parser.py:2114-2135,2463) and Postgres dollar-quoted heredoc bodies (exp.Heredoc,
// parser.py:9368-9370). Expected outputs verified against pinned upstream v30.12.0.
func TestCreateProcedureBody(t *testing.T) {
	cases := []struct {
		sql      string
		dialects []string
		rootKind exp.Kind
		want     string
	}{
		{"CREATE PROCEDURE p() BEGIN SELECT 1; END", []string{"base", "mysql", "postgres"},
			exp.KindCreate, "CREATE PROCEDURE p() AS BEGIN SELECT 1; END"},
		{"CREATE PROCEDURE p() BEGIN UPDATE t SET a = 1; SELECT 1; END", []string{"mysql"},
			exp.KindCreate, "CREATE PROCEDURE p() AS BEGIN UPDATE t SET a = 1; SELECT 1; END"},
		{"CREATE PROCEDURE p(IN x INT) BEGIN UPDATE t SET a = x; SELECT 1; END", []string{"postgres"},
			exp.KindCreate, "CREATE PROCEDURE p(IN x INT) AS BEGIN UPDATE t SET a = x; SELECT 1; END"},
		// The already-consumed AS spelling round-trips identically.
		{"CREATE PROCEDURE p() AS BEGIN SELECT 1; END", []string{"mysql"},
			exp.KindCreate, "CREATE PROCEDURE p() AS BEGIN SELECT 1; END"},
		// divergence (§1.18): a BEGIN-bodied FUNCTION takes the block path like PROCEDURE —
		// upstream's single-statement body path leaves the END as a dangling top-level
		// EndStatement fragment.
		{"CREATE FUNCTION f() RETURNS INT BEGIN RETURN 1; END", []string{"mysql"},
			exp.KindCreate, "CREATE FUNCTION f() RETURNS INT AS BEGIN RETURN 1; END"},
		// Postgres dollar-quoted bodies -> exp.Heredoc. divergence: a named tag is preserved
		// (pinned upstream drops it and always emits `$$`, which real PostgreSQL rejects when
		// the body itself contains `$$`); see DEVIATIONS §1 and TestHeredocTagPreserved.
		{"CREATE PROCEDURE p() LANGUAGE plpgsql AS $$ BEGIN UPDATE t SET a = 1; END $$", []string{"postgres"},
			exp.KindCreate, "CREATE PROCEDURE p() LANGUAGE plpgsql AS $$ BEGIN UPDATE t SET a = 1; END $$"},
		{"CREATE FUNCTION f() RETURNS INT AS $fn$ BEGIN RETURN 1; END $fn$ LANGUAGE plpgsql", []string{"postgres"},
			exp.KindCreate, "CREATE FUNCTION f() RETURNS INT LANGUAGE plpgsql AS $fn$ BEGIN RETURN 1; END $fn$"},
	}
	for _, tc := range cases {
		for _, dialect := range tc.dialects {
			e, err := sqlglot.ParseOne(tc.sql, dialect)
			if err != nil {
				t.Errorf("%q [%s]: parse: %v", tc.sql, dialect, err)
				continue
			}
			if e.Kind() != tc.rootKind {
				t.Errorf("%q [%s]: root Kind=%s, want %s", tc.sql, dialect, exp.ClassName(e.Kind()), exp.ClassName(tc.rootKind))
			}
			got, err := sqlglot.Generate(e, dialect, generator.Options{})
			if err != nil {
				t.Errorf("%q [%s]: generate: %v", tc.sql, dialect, err)
				continue
			}
			if got != tc.want {
				t.Errorf("%q [%s]:\ngot  %q\nwant %q", tc.sql, dialect, got, tc.want)
			}
		}
	}
}

// divergence test for DEVIATIONS §1.18: END terminates a routine's block, so a trailing
// statement is its OWN statement — upstream folds it into the procedure's Block (after
// EndStatement), but real PostgreSQL 16 (BEGIN ATOMIC, the engine that parses this form
// server-side) runs the trailing statement separately. Nested WHILE blocks are unaffected:
// only the routine body's own terminating END returns to the outer batch.
func TestBlockEndTerminates(t *testing.T) {
	cases := []struct {
		sql   string
		kinds []exp.Kind
	}{
		{"CREATE PROCEDURE p() BEGIN SELECT 1; END; SELECT 2",
			[]exp.Kind{exp.KindCreate, exp.KindSelect}},
		{"CREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END; UPDATE t SET a = 1",
			[]exp.Kind{exp.KindCreate, exp.KindUpdate}},
		{"CREATE PROCEDURE p() BEGIN WHILE (x < 3) BEGIN SELECT 1; END; END; SELECT 9",
			[]exp.Kind{exp.KindCreate, exp.KindSelect}},
		// Empty body (`BEGIN END`, valid MySQL): terminates the same way instead of
		// upstream's Column(END) mis-parse.
		{"CREATE PROCEDURE p() BEGIN END; SELECT 2",
			[]exp.Kind{exp.KindCreate, exp.KindSelect}},
		// A WHILE body without its own BEGIN is the single following statement, so it cannot
		// steal the routine's END.
		{"CREATE PROCEDURE p() BEGIN WHILE (x < 3) SELECT 1; END; SELECT 9",
			[]exp.Kind{exp.KindCreate, exp.KindSelect}},
		// Multiple trailers all stay separate statements.
		{"CREATE PROCEDURE p() BEGIN SELECT 1; END; SELECT 2; SELECT 3",
			[]exp.Kind{exp.KindCreate, exp.KindSelect, exp.KindSelect}},
		// Two routines in one batch.
		{"CREATE PROCEDURE p() BEGIN SELECT 1; END; CREATE PROCEDURE q() BEGIN SELECT 2; END",
			[]exp.Kind{exp.KindCreate, exp.KindCreate}},
	}
	for _, tc := range cases {
		statements, err := sqlglot.Parse(tc.sql, "mysql")
		if err != nil {
			t.Errorf("%q: parse: %v", tc.sql, err)
			continue
		}
		if len(statements) != len(tc.kinds) {
			t.Errorf("%q: got %d statements, want %d", tc.sql, len(statements), len(tc.kinds))
			continue
		}
		for i, want := range tc.kinds {
			if statements[i].Kind() != want {
				t.Errorf("%q stmt %d: Kind=%s, want %s", tc.sql, i, exp.ClassName(statements[i].Kind()), exp.ClassName(want))
			}
		}
		// The routine's Block must not smuggle anything past its EndStatement.
		block := statements[0].Find(exp.KindBlock)
		if block == nil {
			t.Errorf("%q: no Block in Create", tc.sql)
			continue
		}
		inner := block.Expressions()
		if len(inner) == 0 || inner[len(inner)-1].Kind() != exp.KindEndStatement {
			t.Errorf("%q: Block does not end with EndStatement: %s", tc.sql, block.ToS())
		}
	}
}

// divergence (§1.18): every routine form is ONE statement through its body's END — never a
// truncated definition plus a dangling top-level EndStatement fragment (upstream's shape for
// TRIGGER, BEGIN-bodied FUNCTION, and PG BEGIN ATOMIC). Trailing statements stay separate.
func TestRoutineBodyOneStatement(t *testing.T) {
	cases := []struct {
		sql     string
		dialect string
		spans   []string
	}{
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = 1; END", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = 1; END"}},
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = 1; END; SELECT 2", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = 1; END", "SELECT 2"}},
		// Body with a control-flow END pair: END IF must not close the BEGIN block early.
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN IF @a THEN SET @b = 1; END IF; SET @c = 2; END; SELECT 3", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN IF @a THEN SET @b = 1; END IF; SET @c = 2; END", "SELECT 3"}},
		{"CREATE FUNCTION f() RETURNS INT BEGIN RETURN 1; END", "mysql",
			[]string{"CREATE FUNCTION f() RETURNS INT BEGIN RETURN 1; END"}},
		{"CREATE FUNCTION f() RETURNS INT BEGIN RETURN 1; END; SELECT 2", "mysql",
			[]string{"CREATE FUNCTION f() RETURNS INT BEGIN RETURN 1; END", "SELECT 2"}},
		{"CREATE FUNCTION f() RETURNS int LANGUAGE SQL BEGIN ATOMIC SELECT 1; END; SELECT 2", "postgres",
			[]string{"CREATE FUNCTION f() RETURNS int LANGUAGE SQL BEGIN ATOMIC SELECT 1; END", "SELECT 2"}},
		// Bare procedural bodies without BEGIN: IF/LOOP/REPEAT/WHILE/CASE leaders open a
		// block ended by their `END <kw>` closer (valid MySQL, verified on 8.4).
		{"CREATE PROCEDURE p() IF 1 THEN SELECT 1; SELECT 2; END IF", "mysql",
			[]string{"CREATE PROCEDURE p() IF 1 THEN SELECT 1; SELECT 2; END IF"}},
		{"CREATE PROCEDURE p() LOOP SELECT 1; SELECT 2; END LOOP", "mysql",
			[]string{"CREATE PROCEDURE p() LOOP SELECT 1; SELECT 2; END LOOP"}},
		{"CREATE PROCEDURE p() IF 1 THEN SELECT 1; END IF; SELECT 99", "mysql",
			[]string{"CREATE PROCEDURE p() IF 1 THEN SELECT 1; END IF", "SELECT 99"}},
		{"CREATE PROCEDURE p() REPEAT SELECT 1; UNTIL x END REPEAT", "mysql",
			[]string{"CREATE PROCEDURE p() REPEAT SELECT 1; UNTIL x END REPEAT"}},
		// IF()/REPEAT() as ordinary function calls inside a degraded body don't open blocks.
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = IF(1, 2, 3); END; SELECT 9", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = IF(1, 2, 3); END", "SELECT 9"}},
		// Mixed nesting: IF > LOOP inside BEGIN, all balanced.
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN IF @a THEN LOOP SET @b = 1; END LOOP; END IF; END; SELECT 4", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN IF @a THEN LOOP SET @b = 1; END LOOP; END IF; END", "SELECT 4"}},
		// Parenthesized IF condition still opens the block (MySQL accepts it).
		{"CREATE PROCEDURE p() IF (1 = 1) THEN SELECT 1; END IF; SELECT 9", "mysql",
			[]string{"CREATE PROCEDURE p() IF (1 = 1) THEN SELECT 1; END IF", "SELECT 9"}},
		// MySQL `WHILE cond DO ... END WHILE` body: one statement, trailing SELECT separate
		// (previously the structured WhileBlock mis-parse absorbed the SELECT — fail-open).
		{"CREATE PROCEDURE p() WHILE FALSE DO SELECT 1; END WHILE; SELECT 9", "mysql",
			[]string{"CREATE PROCEDURE p() WHILE FALSE DO SELECT 1; END WHILE", "SELECT 9"}},
		// CREATE TRIGGER IF NOT EXISTS: the header IF is not a block opener.
		{"CREATE TRIGGER IF NOT EXISTS t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = 1; END; SELECT 2", "mysql",
			[]string{"CREATE TRIGGER IF NOT EXISTS t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = 1; END", "SELECT 2"}},
		// Bare identifiers named begin/if/loop and IF NOT EXISTS inside a degraded body:
		// not openers (position/next-token gates), so the batch splits cleanly.
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SELECT begin FROM t; END; DROP TABLE users; SELECT 2", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SELECT begin FROM t; END", "DROP TABLE users", "SELECT 2"}},
		// Round-3 shapes: adjacent begin identifiers, table named begin, DO/REPLACE first
		// statements, condition-name/SQLSTATE/errno handlers, identifier end inside CASE.
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN INSERT INTO log SELECT begin begin FROM src; END; DROP TABLE users; SELECT 1 end", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN INSERT INTO log SELECT begin begin FROM src; END", "DROP TABLE users", "SELECT 1 end"}},
		{"CREATE TRIGGER a BEFORE INSERT ON x FOR EACH ROW BEGIN INSERT INTO begin SELECT 1; END; CREATE TRIGGER b BEFORE UPDATE ON x FOR EACH ROW BEGIN DO 1; END", "mysql",
			[]string{"CREATE TRIGGER a BEFORE INSERT ON x FOR EACH ROW BEGIN INSERT INTO begin SELECT 1; END", "CREATE TRIGGER b BEFORE UPDATE ON x FOR EACH ROW BEGIN DO 1; END"}},
		{"CREATE TRIGGER tr BEFORE UPDATE ON x FOR EACH ROW BEGIN REPLACE INTO y VALUES (1); END; SELECT 9", "mysql",
			[]string{"CREATE TRIGGER tr BEFORE UPDATE ON x FOR EACH ROW BEGIN REPLACE INTO y VALUES (1); END", "SELECT 9"}},
		{"CREATE PROCEDURE p() BEGIN DECLARE CONTINUE HANDLER FOR SQLSTATE '23000' IF 1 THEN SELECT 1; END IF; END; SELECT 9", "mysql",
			[]string{"CREATE PROCEDURE p() BEGIN DECLARE CONTINUE HANDLER FOR SQLSTATE '23000' IF 1 THEN SELECT 1; END IF; END", "SELECT 9"}},
		{"CREATE PROCEDURE p() BEGIN DECLARE CONTINUE HANDLER FOR 1051 IF 1 THEN SELECT 1; END IF; END; SELECT 9", "mysql",
			[]string{"CREATE PROCEDURE p() BEGIN DECLARE CONTINUE HANDLER FOR 1051 IF 1 THEN SELECT 1; END IF; END", "SELECT 9"}},
		{"CREATE PROCEDURE p() BEGIN DECLARE CONTINUE HANDLER FOR no_table IF 1 THEN SELECT 1; END IF; END; SELECT 9", "mysql",
			[]string{"CREATE PROCEDURE p() BEGIN DECLARE CONTINUE HANDLER FOR no_table IF 1 THEN SELECT 1; END IF; END", "SELECT 9"}},
		{"CREATE PROCEDURE p() BEGIN SET @a = CASE end WHEN 1 THEN 2 ELSE 3 END; END; SELECT 9", "mysql",
			[]string{"CREATE PROCEDURE p() BEGIN SET @a = CASE end WHEN 1 THEN 2 ELSE 3 END; END", "SELECT 9"}},
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = if; END; SELECT 9", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = if; END", "SELECT 9"}},
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SELECT loop FROM t; END; SELECT 9", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SELECT loop FROM t; END", "SELECT 9"}},
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN CREATE TABLE IF NOT EXISTS u(i INT); END; SELECT 9", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN CREATE TABLE IF NOT EXISTS u(i INT); END", "SELECT 9"}},
		// Labeled blocks: `lbl:` is a statement-start gate.
		{"CREATE PROCEDURE p() BEGIN lbl: LOOP SELECT 1; END LOOP lbl; END; SELECT 5", "mysql",
			[]string{"CREATE PROCEDURE p() BEGIN lbl: LOOP SELECT 1; END LOOP lbl; END", "SELECT 5"}},
		// Routine characteristics before a bare body still gate the leader.
		{"CREATE PROCEDURE p() DETERMINISTIC IF 1 THEN SELECT 1; END IF; SELECT 6", "mysql",
			[]string{"CREATE PROCEDURE p() DETERMINISTIC IF 1 THEN SELECT 1; END IF", "SELECT 6"}},
		// Expression CASE inside a body pairs with its own plain END.
		{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = CASE WHEN 1 THEN 2 ELSE 3 END; END; SELECT 4", "mysql",
			[]string{"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = CASE WHEN 1 THEN 2 ELSE 3 END; END", "SELECT 4"}},
	}
	for _, tc := range cases {
		statements, err := sqlglot.Parse(tc.sql, tc.dialect)
		if err != nil {
			t.Errorf("%q [%s]: parse: %v", tc.sql, tc.dialect, err)
			continue
		}
		if len(statements) != len(tc.spans) {
			t.Errorf("%q [%s]: got %d statements, want %d", tc.sql, tc.dialect, len(statements), len(tc.spans))
			continue
		}
		for i, want := range tc.spans {
			if statements[i].Kind() == exp.KindEndStatement {
				t.Errorf("%q [%s] stmt %d: dangling top-level EndStatement", tc.sql, tc.dialect, i)
				continue
			}
			if text, ok := statements[i].SpanText(); !ok || text != want {
				t.Errorf("%q [%s] stmt %d: SpanText()=%q,%v, want %q", tc.sql, tc.dialect, i, text, ok, want)
			}
		}
	}
}

// The degraded-CREATE extent machinery must FAIL CLOSED on every ambiguity: an input where
// the block-depth count cannot establish the body's END (unterminated body, `begin`/`end`
// used as bare identifiers inflating/deflating the count) is a parse error — never a merge
// of later statements into the Command (fail-open for a per-statement consumer) and never
// truncated fragments. Bare top-level END is likewise an error under MySQL (real MySQL 8.0
// rejects it; error 1064).
func TestDegradedCreateFailsClosed(t *testing.T) {
	for _, sql := range []string{
		// PG CREATE RULE with a trailing alias IF and a later END IF (must not merge).
		// NOTE: dialect is mysql below; the PG variant is asserted separately.
		// Unterminated body: without the depth>0 check this merged the DROP into the Command.
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = 1; DROP TABLE users",

		// `end` as a bare value deflates it -> the residual top-level END chunk errors.
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = end; END; SELECT 2",
		// Empty trailing chunk while unbalanced: an error, not a slice-bounds panic.
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a=1;;",
		"SELECT 1; END",
		// Unterminated structured body (real MySQL error 1064): an error, not a silently
		// truncated Create ending at the last inner statement.
		"CREATE PROCEDURE p() BEGIN SELECT 1",
		"CREATE PROCEDURE p() BEGIN SELECT 1;",
		// Bare END as the whole batch (real MySQL error 1064): an error, not Column(END).
		"END",
		// A stray trailing END cannot rebalance the block stack (kind-matching): a bare
		// identifier miscount plus a later END must ERROR, never merge the DROP into the
		// Command (the fail-open direction the stack design exists to prevent).
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = if; END; DROP TABLE users; END",
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN CREATE TABLE IF NOT EXISTS u(i INT); END; SELECT 9; END",
		// Same-kind false-opener probes: an identifier/alias that COULD push a kind plus a
		// later matching `END <kw>` — must error, never balance and absorb the DROP.
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SELECT * FROM (SELECT 1) if; END IF; DROP TABLE users; END",
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN IF 1 THEN SELECT (1) if; END IF; END IF; DROP TABLE u; END",
		"CREATE PROCEDURE p() BEGIN DO IF(1,2,3); END IF; DROP TABLE users; END",
		"CREATE PROCEDURE p() BEGIN DO REPEAT(1,2); END REPEAT; DROP TABLE users; END",
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SELECT 1 AS case; END CASE; DROP TABLE users; END",
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SELECT case FROM t; END CASE; DROP TABLE u; END",
		// whenAhead across a subquery WHEN + broken expression CASE + PG RULE header IF:
		// all must error, never merge the DROP.
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW INSERT INTO log SELECT case FROM (SELECT when FROM src) s; DROP TABLE users; END CASE",
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = CASE WHEN 1 THEN 2; END; DROP TABLE users; END",
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = CASE WHEN 1 THEN 2; END CASE; DROP TABLE users; END",
	} {
		if _, err := sqlglot.Parse(sql, "mysql"); err == nil {
			t.Errorf("%q parsed; want error", sql)
		}
	}
}

// A routine body WITHOUT BEGIN (a valid single-statement routine) must not trip the
// unterminated-block error; and bare `END` stays dialect-correct: mysql errors (real MySQL
// 1064), postgres is COMMIT, base keeps upstream's Column.
func TestNoBeginBodyAndBareEndDialects(t *testing.T) {
	for _, dialect := range []string{"base", "mysql", "postgres"} {
		e, err := sqlglot.ParseOne("CREATE PROCEDURE p() SELECT 1", dialect)
		if err != nil {
			t.Errorf("[%s] no-BEGIN body: %v", dialect, err)
		} else if e.Kind() != exp.KindCreate {
			t.Errorf("[%s] no-BEGIN body: Kind=%s, want Create", dialect, exp.ClassName(e.Kind()))
		}
	}
	if e, err := sqlglot.ParseOne("END", "postgres"); err != nil || e.Kind() != exp.KindCommit {
		t.Errorf("[postgres] END: got %v/%v, want Commit", e, err)
	}
	if e, err := sqlglot.ParseOne("END", "base"); err != nil || e.Kind() != exp.KindColumn {
		t.Errorf("[base] END: got %v/%v, want Column", e, err)
	}
	if _, err := sqlglot.ParseOne("SELECT end", "mysql"); err != nil {
		t.Errorf("[mysql] SELECT end: %v", err)
	}
}

// Under postgres a bare END chunk is COMMIT wherever it appears in the batch — real PG 16
// executes `BEGIN; SELECT 1; END` with END closing the transaction; upstream returns a
// dangling EndStatement for the mid-batch position (DEVIATIONS §1.18).
func TestPostgresMidBatchEndIsCommit(t *testing.T) {
	statements, err := sqlglot.Parse("BEGIN; SELECT 1; END", "postgres")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []exp.Kind{exp.KindTransaction, exp.KindSelect, exp.KindCommit}
	if len(statements) != len(want) {
		t.Fatalf("got %d statements, want %d", len(statements), len(want))
	}
	for i, k := range want {
		if statements[i].Kind() != k {
			t.Errorf("stmt %d: Kind=%s, want %s", i, exp.ClassName(statements[i].Kind()), exp.ClassName(k))
		}
	}
}

// Keywords in qualified identifiers (`NEW.begin`) are not block tokens: the batch still
// splits at the real statement boundary.
func TestQualifiedKeywordNotBlockToken(t *testing.T) {
	statements, err := sqlglot.Parse("CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW SET NEW.begin = 1; DROP TABLE secret", "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(statements) != 2 || statements[1].Kind() != exp.KindDrop {
		t.Fatalf("got %d statements (last %s), want [Command, Drop]", len(statements), exp.ClassName(statements[len(statements)-1].Kind()))
	}
}

// A body statement the parser can't fully consume (MySQL DECLARE) degrades the whole
// definition to ONE verbatim Command — never a silently mangled structured AST.
func TestUnsupportedBodyStatementDegradesWhole(t *testing.T) {
	sql := "CREATE FUNCTION f() RETURNS INT BEGIN DECLARE x INT DEFAULT 1; RETURN x; END"
	e, err := sqlglot.ParseOne(sql, "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Kind() != exp.KindCommand {
		t.Fatalf("root Kind=%s, want Command", exp.ClassName(e.Kind()))
	}
	if got, _ := e.SpanText(); got != sql {
		t.Errorf("SpanText()=%q, want the whole definition", got)
	}
}

// A bare top-level ELSE chunk is a parse error (real MySQL/PG reject it) — never upstream's
// silent drop of every later statement (DEVIATIONS §1.18).
func TestTopLevelElseFailsClosed(t *testing.T) {
	for _, sql := range []string{
		"SELECT 1; ELSE; SELECT 2",
		"CREATE PROCEDURE p() BEGIN SELECT 1; END; ELSE; SELECT 2",
	} {
		if _, err := sqlglot.Parse(sql, "mysql"); err == nil {
			t.Errorf("%q parsed; want error", sql)
		}
	}
}

// A standalone `BEGIN SELECT 1; END` is not a valid statement in any target dialect (BEGIN
// starts a transaction); upstream parse-errors it too. Must stay fail-closed.
func TestBareBeginBlockFailsClosed(t *testing.T) {
	for _, dialect := range []string{"base", "mysql", "postgres"} {
		if _, err := sqlglot.ParseOne("BEGIN SELECT 1; END", dialect); err == nil {
			t.Errorf("[%s] bare BEGIN block parsed; want error", dialect)
		}
	}
}

// divergence test for DEVIATIONS §1: a named dollar-quote tag survives the round-trip even
// when the body contains `$$` — pinned upstream regenerates `$$ SELECT '$$'; $$`, which real
// PostgreSQL 17 rejects (the embedded `$$` closes the body early).
func TestHeredocTagPreserved(t *testing.T) {
	sql := "CREATE FUNCTION f() RETURNS INT AS $fn$ SELECT '$$'; $fn$ LANGUAGE plpgsql"
	e, err := sqlglot.ParseOne(sql, "postgres")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := sqlglot.Generate(e, "postgres", generator.Options{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want := "CREATE FUNCTION f() RETURNS INT LANGUAGE plpgsql AS $fn$ SELECT '$$'; $fn$"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := sqlglot.ParseOne(got, "postgres"); err != nil {
		t.Fatalf("regenerated SQL does not re-tokenize: %v", err)
	}
}

// A structured CREATE whose block parse stopped mid-statement degrades to Command IN PLACE
// (parser.py:2624): the Command spans through the chunk the parse stopped in — a block body
// may have consumed several — and parsing resumes at the next chunk, so trailing statements
// survive. Upstream yields [Command("...SELECT 1; ELSE"), Select] for this input.
func TestCreateProcedureDegradeSpansConsumedChunks(t *testing.T) {
	statements, err := sqlglot.Parse("CREATE PROCEDURE p() BEGIN SELECT 1; ELSE; SELECT 2", "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(statements) != 2 {
		t.Fatalf("got %d statements, want 2 (Command + Select)", len(statements))
	}
	if statements[0].Kind() != exp.KindCommand {
		t.Errorf("statements[0] Kind=%s, want Command", exp.ClassName(statements[0].Kind()))
	}
	if got, want := statements[0].Text("this")+statements[0].Text("expression"), "CREATE PROCEDURE p() BEGIN SELECT 1; ELSE"; got != want {
		t.Errorf("Command text %q, want %q", got, want)
	}
	if statements[1].Kind() != exp.KindSelect {
		t.Errorf("statements[1] Kind=%s, want Select", exp.ClassName(statements[1].Kind()))
	}
}

// WHILE statement AST parity (_parse_whileblock, parser.py:2278-2281); base generation is
// unsupported upstream too (generator.py:6208-6210), so only the AST shape is asserted.
func TestWhileBlockAST(t *testing.T) {
	e, err := sqlglot.ParseOne("WHILE (x < 3) SELECT 1", "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := "WhileBlock(\n" +
		"  this=LT(\n" +
		"    this=Column(\n" +
		"      this=Identifier(this=x, quoted=False)),\n" +
		"    expression=Literal(this=3, is_string=False)),\n" +
		"  body=Block(\n" +
		"    expressions=[\n" +
		"      Select(\n" +
		"        expressions=[\n" +
		"          Literal(this=1, is_string=False)])]))"
	if got := e.ToS(); got != want {
		t.Errorf("ToS() mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
