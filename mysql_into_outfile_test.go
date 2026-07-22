package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	sqlerrors "github.com/ridi-oss/sqlglot-go/errors"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/parser"
)

// MySQL `SELECT ... INTO {OUTFILE|DUMPFILE} '/path' [CHARACTER SET cs] [export_options]` is a
// server-side file write. It parses to a Select carrying an `into` arg → an Into node whose
// `kind` is "OUTFILE"/"DUMPFILE" (unset for a normal INTO table/var) and whose `this` is the
// path Literal, so a consumer can detect the file-write (and its target) structurally rather
// than scanning raw SQL. This is an extension beyond pinned upstream, which parse-errors the
// form (see testdata/upstream_extensions.jsonl "mysql-into-outfile"/"mysql-into-dumpfile").
// Verified syntactically against MySQL 8.0.33 (each form reaches runtime secure-file-priv, i.e.
// parses cleanly).

// intoArg returns the Into node hanging off a Select's `into` arg.
func intoArg(t *testing.T, e exp.Expression) exp.Expression {
	t.Helper()
	into, ok := e.Arg("into").(exp.Expression)
	if !ok || into == nil {
		t.Fatalf("no into arg:\n%s", e.ToS())
	}
	if into.Kind() != exp.KindInto {
		t.Fatalf("into kind = %v, want Into:\n%s", exp.ClassName(into.Kind()), e.ToS())
	}
	return into
}

func TestMySQLIntoOutfileRoundTrip(t *testing.T) {
	// Identity round-trips at the canonical trailing position, with the full export-options grammar.
	cases := []string{
		"SELECT * FROM t INTO OUTFILE '/tmp/x'",
		"SELECT * FROM t INTO DUMPFILE '/tmp/x'",
		"SELECT 1 FROM t INTO OUTFILE '/tmp/x' CHARACTER SET utf8",
		"SELECT 1 FROM t INTO OUTFILE '/tmp/x' CHARACTER SET 'utf8'", // quoted charset name
		"SELECT 1 FROM t INTO OUTFILE '/tmp/x' FIELDS TERMINATED BY ','",
		"SELECT 1 FROM t INTO OUTFILE '/tmp/x' COLUMNS TERMINATED BY ','",
		"SELECT 1 FROM t INTO OUTFILE '/tmp/x' FIELDS ENCLOSED BY '\"'",
		"SELECT 1 FROM t INTO OUTFILE '/tmp/x' LINES TERMINATED BY '\\n'",
		"SELECT 1 FROM t INTO OUTFILE '/tmp/x' FIELDS TERMINATED BY x'2c'",     // hex-string value
		"SELECT 1 FROM t INTO OUTFILE '/tmp/x' FIELDS TERMINATED BY b'101100'", // bit-string value
		`SELECT a, b FROM t WHERE a > 1 INTO OUTFILE '/tmp/x' FIELDS TERMINATED BY ',' OPTIONALLY ENCLOSED BY '"' ESCAPED BY '\\' LINES STARTING BY 'y' TERMINATED BY '\n'`,
	}
	for _, sql := range cases {
		t.Run(sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != exp.KindSelect {
				t.Fatalf("kind = %v, want Select:\n%s", exp.ClassName(e.Kind()), e.ToS())
			}
			out, gerr := sqlglot.Generate(e, "mysql", generator.Options{})
			if gerr != nil {
				t.Fatalf("generate: %v", gerr)
			}
			if out != sql {
				t.Fatalf("round-trip = %q, want %q", out, sql)
			}
		})
	}
}

func TestMySQLIntoOutfileShape(t *testing.T) {
	// The contract a consumer keys on: Into.kind marks the write kind, Into.this is the path Literal.
	cases := []struct {
		sql, wantKind, wantPath string
	}{
		{"SELECT * FROM t INTO OUTFILE '/var/lib/mysql-files/out.csv'", "OUTFILE", "/var/lib/mysql-files/out.csv"},
		{"SELECT * FROM t INTO DUMPFILE '/var/lib/mysql-files/blob.bin'", "DUMPFILE", "/var/lib/mysql-files/blob.bin"},
	}
	for _, tc := range cases {
		t.Run(tc.wantKind, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			into := intoArg(t, e)
			if got := into.Text("kind"); got != tc.wantKind {
				t.Fatalf("kind = %q, want %q:\n%s", got, tc.wantKind, e.ToS())
			}
			path, ok := into.Arg("this").(exp.Expression)
			if !ok || path.Kind() != exp.KindLiteral || path.Arg("is_string") != true {
				t.Fatalf("this = %v, want string Literal:\n%s", exp.ClassName(into.Kind()), e.ToS())
			}
			if got := path.Text("this"); got != tc.wantPath {
				t.Fatalf("path = %q, want %q", got, tc.wantPath)
			}
			// The Into node is reachable by a structural walk over the whole statement.
			found := e.FindAll(exp.KindInto)
			if len(found) != 1 {
				t.Fatalf("FindAll(Into) = %d nodes, want 1:\n%s", len(found), e.ToS())
			}
		})
	}
}

func TestMySQLIntoOutfileExportOptions(t *testing.T) {
	// The full FIELDS/LINES grammar lands in dedicated args (not smuggled into `this`).
	e, err := sqlglot.ParseOne(
		`SELECT a FROM t INTO OUTFILE '/tmp/x' CHARACTER SET utf8mb4 FIELDS TERMINATED BY ',' OPTIONALLY ENCLOSED BY '"' ESCAPED BY '\\' LINES STARTING BY '>' TERMINATED BY '\n'`,
		"mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	into := intoArg(t, e)
	if into.Arg("optionally_enclosed") != true {
		t.Fatalf("optionally_enclosed not set:\n%s", into.ToS())
	}
	for _, key := range []string{"charset", "fields_terminated", "enclosed", "escaped", "lines_starting", "lines_terminated"} {
		if into.Arg(key) == nil {
			t.Fatalf("export option %q not captured:\n%s", key, into.ToS())
		}
	}
	// COLUMNS is the FIELDS synonym; it flips the `columns` marker.
	e2, err := sqlglot.ParseOne("SELECT a FROM t INTO OUTFILE '/tmp/x' COLUMNS TERMINATED BY ','", "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if intoArg(t, e2).Arg("columns") != true {
		t.Fatalf("COLUMNS did not set the columns marker:\n%s", e2.ToS())
	}
}

func TestMySQLIntoOutfilePositionNormalizes(t *testing.T) {
	// MySQL accepts INTO before FROM too; it parses to the same shape and normalizes to the
	// canonical trailing position on output (semantically identical, valid MySQL).
	e, err := sqlglot.ParseOne("SELECT * INTO OUTFILE '/tmp/x' FROM t", "mysql")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := intoArg(t, e).Text("kind"); got != "OUTFILE" {
		t.Fatalf("kind = %q, want OUTFILE", got)
	}
	out, _ := sqlglot.Generate(e, "mysql", generator.Options{})
	if want := "SELECT * FROM t INTO OUTFILE '/tmp/x'"; out != want {
		t.Fatalf("normalized = %q, want %q", out, want)
	}
}

func TestMySQLIntoOutfileBoundaries(t *testing.T) {
	// A committed file-write with no path fails closed (hard parse error), never a silent
	// degrade to a plain SELECT with the INTO dropped.
	t.Run("missing path fails closed", func(t *testing.T) {
		for _, sql := range []string{"SELECT 1 INTO OUTFILE", "SELECT 1 INTO DUMPFILE"} {
			if _, err := sqlglot.ParseOne(sql, "mysql"); err == nil {
				t.Fatalf("%q: parsed without error, want fail-closed", sql)
			}
		}
	})
	// DUMPFILE takes no export options (MySQL rejects them); trailing option tokens fail closed.
	t.Run("dumpfile rejects export options", func(t *testing.T) {
		if _, err := sqlglot.ParseOne("SELECT 1 INTO DUMPFILE '/x' FIELDS TERMINATED BY ','", "mysql"); err == nil {
			t.Fatal("DUMPFILE with export options parsed without error, want fail-closed")
		}
	})
	// The file-write grammar is MySQL-gated: other dialects do not treat OUTFILE specially.
	t.Run("mysql-gated", func(t *testing.T) {
		for _, dialect := range []string{"postgres", ""} {
			if _, err := sqlglot.ParseOne("SELECT 1 INTO OUTFILE '/x'", dialect); err == nil {
				t.Fatalf("dialect %q: parsed OUTFILE without error, want gated off", dialect)
			}
		}
	})
	// A plain INTO table/var keeps kind unset (unchanged from before the extension).
	t.Run("plain into keeps kind unset", func(t *testing.T) {
		e, err := sqlglot.ParseOne("SELECT * INTO foo FROM bar", "mysql")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := intoArg(t, e).Text("kind"); got != "" {
			t.Fatalf("plain INTO kind = %q, want empty:\n%s", got, e.ToS())
		}
	})
}

func TestMySQLIntoOutfileOptionOrderAndValues(t *testing.T) {
	// MySQL accepts FIELDS/LINES sub-options in ANY order (last wins on repetition); the parser
	// captures them into the same args and the generator re-emits a canonical order — semantically
	// identical, valid MySQL (verified against MySQL 8.0.33). Reordered/repeated input therefore
	// normalizes rather than round-tripping byte-for-byte.
	cases := []struct{ in, want string }{
		{"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS ENCLOSED BY '\"' TERMINATED BY ','",
			"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS TERMINATED BY ',' ENCLOSED BY '\"'"},
		{"SELECT 1 FROM t INTO OUTFILE '/x' LINES TERMINATED BY '\\n' STARTING BY '>'",
			"SELECT 1 FROM t INTO OUTFILE '/x' LINES STARTING BY '>' TERMINATED BY '\\n'"},
		{"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS TERMINATED BY ',' TERMINATED BY ';'", // repetition: last wins
			"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS TERMINATED BY ';'"},
		{"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS TERMINATED BY 0x2c", // 0x hex literal normalizes to x'2c'
			"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS TERMINATED BY x'2c'"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			e, err := sqlglot.ParseOne(tc.in, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			out, _ := sqlglot.Generate(e, "mysql", generator.Options{})
			if out != tc.want {
				t.Fatalf("normalized = %q, want %q", out, tc.want)
			}
		})
	}
}

func TestMySQLIntoOutfileExportOptionsFailClosed(t *testing.T) {
	// Every export-option introducer requires its operand, and a bare FIELDS/COLUMNS/LINES with no
	// sub-option is rejected — all fail closed rather than silently dropping the malformed clause.
	// Real MySQL 8.0.33 rejects each of these with a syntax error.
	for _, sql := range []string{
		"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS",                              // bare FIELDS
		"SELECT 1 FROM t INTO OUTFILE '/x' COLUMNS",                             // bare COLUMNS
		"SELECT 1 FROM t INTO OUTFILE '/x' LINES",                               // bare LINES
		"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS TERMINATED BY",                // dangling value
		"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS ENCLOSED BY",                  // dangling value
		"SELECT 1 FROM t INTO OUTFILE '/x' LINES STARTING BY",                   // dangling value
		"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS OPTIONALLY",                   // OPTIONALLY without ENCLOSED BY
		"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS OPTIONALLY TERMINATED BY ','", // OPTIONALLY misplaced
		"SELECT 1 FROM t INTO OUTFILE '/x' CHARACTER SET",                       // missing charset
		"SELECT 1 FROM t INTO OUTFILE '/x' CHARACTER SET 5",                     // charset is not a name (number)
		"SELECT 1 FROM t INTO OUTFILE '/x' CHARACTER SET NULL",                  // charset is not a name (NULL)
		"SELECT 1 FROM t INTO OUTFILE '/x' CHARACTER SET (utf8mb4)",             // charset parenthesized
		"SELECT 1 FROM t INTO OUTFILE '/x' CHARACTER SET ?",                     // charset placeholder
		"SELECT 1 FROM t INTO OUTFILE '/x' CHARACTER SET @cs",                   // charset session var
		"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS TERMINATED BY 5",              // non-string value (number)
		"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS TERMINATED BY N','",           // national string (MySQL rejects here)
		"SELECT 1 FROM t INTO OUTFILE '/x' FIELDS TERMINATED BY 'a' 'b'",        // adjacent strings (MySQL rejects; no CONCAT fold)
		"SELECT 1 FROM t INTO OUTFILE ?",                                        // placeholder path (not a Literal)
		"SELECT 1 FROM t INTO OUTFILE @p",                                       // parameter path
	} {
		if _, err := sqlglot.ParseOne(sql, "mysql"); err == nil {
			t.Fatalf("%q: parsed without error, want fail-closed", sql)
		}
	}
}

func TestMySQLIntoOutfileTrailingClause(t *testing.T) {
	// A trailing file-write INTO must be the last clause — MySQL rejects a query modifier after it
	// (1064) rather than silently reordering it ahead of the INTO on regeneration. A locking clause
	// (FOR UPDATE / LOCK IN SHARE MODE) is the one thing MySQL allows on either side. Verified 8.0.33.
	for _, sql := range []string{
		"SELECT * FROM t INTO OUTFILE '/x' ORDER BY 1",
		"SELECT * FROM t INTO OUTFILE '/x' LIMIT 1",
		"SELECT * FROM t INTO OUTFILE '/x' WHERE a > 1",
		"SELECT * FROM t INTO OUTFILE '/x' GROUP BY a",
	} {
		if _, err := sqlglot.ParseOne(sql, "mysql"); err == nil {
			t.Fatalf("%q: parsed without error, want fail-closed (modifier after trailing INTO)", sql)
		}
	}
	// FOR UPDATE after the trailing INTO is accepted (and normalizes ahead of the INTO on output).
	e, err := sqlglot.ParseOne("SELECT * FROM t INTO OUTFILE '/x' FOR UPDATE", "mysql")
	if err != nil {
		t.Fatalf("FOR UPDATE after INTO: unexpected error: %v", err)
	}
	if n := len(e.FindAll(exp.KindInto)); n != 1 {
		t.Fatalf("FOR UPDATE after INTO: FindAll(Into) = %d, want 1", n)
	}
}

func TestMySQLIntoOutfileMisplaced(t *testing.T) {
	// A file-write INTO must be at the end of a top-level query expression. MySQL rejects it inside
	// a subquery/derived table and on a non-final UNION branch (error 3954); the parser must fail
	// closed too rather than accept invalid MySQL (detection is not lost either way). Verified
	// against MySQL 8.0.33.
	for _, sql := range []string{
		"SELECT * FROM (SELECT 1 INTO OUTFILE '/x') x",             // inside a derived table
		"SELECT 1 INTO OUTFILE '/x' UNION SELECT 2",                // first (non-final) UNION branch
		"SELECT 1 UNION SELECT 2 INTO OUTFILE '/x' UNION SELECT 3", // middle (non-final) UNION branch
		"SELECT (SELECT 1 INTO OUTFILE '/x')",                      // inside a scalar subquery
		"UPDATE t SET id = (SELECT 1 INTO OUTFILE '/x')",           // nested in UPDATE
		"DELETE FROM t WHERE id IN (SELECT 1 INTO OUTFILE '/x')",   // nested in DELETE
		"INSERT INTO t VALUES ((SELECT 1 INTO OUTFILE '/x'))",      // nested in INSERT VALUES
		"EXPLAIN UPDATE t SET id = (SELECT 1 INTO OUTFILE '/x')",   // nested under EXPLAIN of a non-SELECT
	} {
		if _, err := sqlglot.ParseOne(sql, "mysql"); err == nil {
			t.Fatalf("%q: parsed without error, want fail-closed (misplaced INTO)", sql)
		}
	}
	// EXPLAIN of a SELECT whose top-level query has the INTO is valid (the explained query root).
	if e, err := sqlglot.ParseOne("EXPLAIN SELECT 1 INTO OUTFILE '/x'", "mysql"); err != nil {
		t.Fatalf("EXPLAIN SELECT INTO OUTFILE: unexpected error: %v", err)
	} else if n := len(e.FindAll(exp.KindInto)); n != 1 {
		t.Fatalf("EXPLAIN SELECT INTO OUTFILE: FindAll(Into) = %d, want 1", n)
	}
	// The check must NOT over-reject a top-level INTO that merely has nested subqueries without
	// their own file-write — these are valid MySQL (verified against MySQL 8.0.33).
	for _, sql := range []string{
		"WITH x AS (SELECT 1 AS a) SELECT a FROM x INTO OUTFILE '/x'", // CTE + top-level INTO
		"SELECT (SELECT 1) INTO OUTFILE '/x'",                         // scalar subquery, no inner write
		"SELECT * FROM (SELECT 1 AS a) t INTO OUTFILE '/x'",           // derived table + top-level INTO
	} {
		e, err := sqlglot.ParseOne(sql, "mysql")
		if err != nil {
			t.Fatalf("%q: unexpected error, want accepted: %v", sql, err)
		}
		if n := len(e.FindAll(exp.KindInto)); n != 1 {
			t.Fatalf("%q: FindAll(Into) = %d, want 1", sql, n)
		}
	}
}

func TestMySQLIntoOutfileQuotedKeywordIsPlainInto(t *testing.T) {
	// A backtick-quoted `outfile`/`dumpfile` is an ordinary INTO target name (a variable), which
	// MySQL accepts — NOT the file-write keyword. It must parse as a plain INTO (kind unset), not
	// hard-error as a malformed file-write (a regression the naive text match would introduce).
	for _, sql := range []string{
		"SELECT 1 INTO `outfile` FROM t",
		"SELECT 1 INTO `dumpfile` FROM t",
	} {
		e, err := sqlglot.ParseOne(sql, "mysql")
		if err != nil {
			t.Fatalf("%q: parse: %v", sql, err)
		}
		if got := intoArg(t, e).Text("kind"); got != "" {
			t.Fatalf("%q: kind = %q, want empty (plain INTO):\n%s", sql, got, e.ToS())
		}
	}
}

func TestMySQLIntoOutfileSetOperation(t *testing.T) {
	// A file-write INTO on the last branch of a UNION applies to the whole query expression and is
	// hoisted to the set-operation node so it renders at the trailing position (after ORDER BY),
	// producing valid MySQL and staying detectable via FindAll(KindInto). Verified against MySQL 8.0.33.
	cases := []string{
		"SELECT 1 UNION SELECT 2 INTO OUTFILE '/x'",
		"SELECT 1 AS a UNION SELECT 2 ORDER BY a INTO OUTFILE '/x'",
	}
	for _, sql := range cases {
		t.Run(sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(sql, "mysql")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			found := e.FindAll(exp.KindInto)
			if len(found) != 1 {
				t.Fatalf("FindAll(Into) = %d, want 1:\n%s", len(found), e.ToS())
			}
			if found[0].Text("kind") != "OUTFILE" {
				t.Fatalf("hoisted into kind wrong:\n%s", e.ToS())
			}
			out, _ := sqlglot.Generate(e, "mysql", generator.Options{})
			if out != sql {
				t.Fatalf("round-trip = %q, want %q", out, sql)
			}
		})
	}
}

func TestMySQLIntoOutfileDetectionSurvivesLenientErrorLevel(t *testing.T) {
	// SECURITY: even at the lenient WARN/IGNORE error levels (reachable via the public
	// parser.NewWithErrorLevel), a malformed committed file-write must NEVER silently degrade to a
	// benign SELECT — the Into node with its kind is always produced, so FindAll(KindInto) still
	// flags the write. (At the default IMMEDIATE level, used by sqlglot.ParseOne/Parse, it is a
	// hard parse error, covered elsewhere.)
	mysql, err := dialects.GetOrRaise("mysql")
	if err != nil {
		t.Fatalf("dialect: %v", err)
	}
	const sql = "SELECT secret FROM t INTO OUTFILE"
	for _, level := range []sqlerrors.ErrorLevel{sqlerrors.WARN, sqlerrors.IGNORE} {
		p := parser.NewWithErrorLevel(mysql, level)
		toks, terr := mysql.NewTokenizer().Tokenize(sql)
		if terr != nil {
			t.Fatalf("tokenize: %v", terr)
		}
		res, perr := p.Parse(toks, sql)
		if perr != nil {
			t.Fatalf("level %v: unexpected error: %v", level, perr)
		}
		if len(res) != 1 {
			t.Fatalf("level %v: got %d statements, want 1", level, len(res))
		}
		if n := len(res[0].FindAll(exp.KindInto)); n != 1 {
			t.Fatalf("level %v: FindAll(Into) = %d, want 1 (file-write must stay detectable):\n%s",
				level, n, res[0].ToS())
		}
	}
}

func TestMySQLIntoOutfileExecutableComment(t *testing.T) {
	// A MySQL executable comment `/*! ... */` is stripped by default (matching upstream, for ALL
	// such content — DEVIATIONS §1.5), so a file-write hidden in one is invisible by default. With
	// `mysql_version` set — the documented, opt-in way to see executable-comment SQL — the body is
	// tokenized and the file-write parses to a detectable Into node. This confirms the extension
	// composes with the existing mysql_version feature rather than needing its own comment handling.
	const sql = "SELECT 1 /*! INTO OUTFILE '/tmp/ec' */"
	def, err := sqlglot.ParseOne(sql, "mysql")
	if err != nil {
		t.Fatalf("default parse: %v", err)
	}
	if n := len(def.FindAll(exp.KindInto)); n != 0 {
		t.Fatalf("default: FindAll(Into) = %d, want 0 (comment stripped)", n)
	}
	act, err := sqlglot.ParseOne(sql, "mysql, mysql_version=80035")
	if err != nil {
		t.Fatalf("activated parse: %v", err)
	}
	found := act.FindAll(exp.KindInto)
	if len(found) != 1 {
		t.Fatalf("activated: FindAll(Into) = %d, want 1:\n%s", len(found), act.ToS())
	}
	if found[0].Text("kind") != "OUTFILE" {
		t.Fatalf("activated: into kind wrong:\n%s", act.ToS())
	}
}
