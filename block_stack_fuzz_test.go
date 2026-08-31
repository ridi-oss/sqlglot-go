package sqlglot_test

import (
	"math/rand"
	"strings"
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
)

// TestBlockExtentInvariantFuzz asserts the single property the degraded-CREATE extent
// machinery must uphold for a per-statement consumer (DEVIATIONS §1.18): a statement
// FOLLOWING a routine definition is never absorbed into the routine's span. Generated
// bodies deliberately mix real block structure with the confusable material that broke
// three prior designs — identifiers named begin/end/if/loop/case/while, labels, routine
// characteristics, nested blocks, expression CASE/IF() calls. Any parse outcome is
// acceptable EXCEPT a first statement whose span contains the sentinel trailer.
func TestBlockExtentInvariantFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(20260831))

	headers := []string{
		"CREATE PROCEDURE p()",
		"CREATE PROCEDURE p() DETERMINISTIC",
		"CREATE PROCEDURE p() COMMENT 'c'",
		"CREATE FUNCTION f() RETURNS INT",
		"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW",
	}
	stmts := []string{
		"SELECT 1",
		"SET @a = 1",
		"SELECT begin FROM t",
		"SELECT loop FROM t",
		"SET @a = if",
		"SET @b = end",
		"SELECT 1 AS `case`",
		"SET @a = IF(1, 2, 3)",
		"SET @a = REPEAT('x', 3)",
		"SET @a = CASE WHEN 1 THEN 2 ELSE 3 END",
		"DECLARE begin INT",
		"CREATE TABLE IF NOT EXISTS u (i INT)",
		"SELECT CASE WHEN a = 1 THEN 'x' ELSE 'y' END FROM t",
		"INSERT INTO log SELECT begin begin FROM src",
		"INSERT INTO begin SELECT 1",
		"SELECT case FROM (SELECT when FROM src) s",
		"SET @a = CASE end WHEN 1 THEN 2 ELSE 3 END",
		"DECLARE CONTINUE HANDLER FOR SQLSTATE '23000' SET @h = 1",
		"DO 1",
	}
	// wrap produces a block form around a body given a fresh nesting budget.
	var wrap func(depth int) string
	wrap = func(depth int) string {
		body := stmts[rng.Intn(len(stmts))]
		if depth > 0 && rng.Intn(2) == 0 {
			body = wrap(depth - 1)
		}
		switch rng.Intn(8) {
		case 0:
			return "BEGIN " + body + "; END"
		case 1:
			return "IF @a THEN " + body + "; END IF"
		case 2:
			return "LOOP " + body + "; END LOOP"
		case 3:
			return "lbl: LOOP " + body + "; END LOOP lbl"
		case 4:
			return "CASE @x WHEN 1 THEN " + body + "; END CASE"
		case 5:
			return "WHILE @x DO " + body + "; END WHILE"
		case 6:
			return "REPEAT " + body + "; UNTIL @x END REPEAT"
		default:
			return "BEGIN " + body + "; " + stmts[rng.Intn(len(stmts))] + "; END"
		}
	}

	const sentinel = "DROP TABLE fuzz_sentinel"
	for i := 0; i < 3000; i++ {
		sql := headers[rng.Intn(len(headers))] + " " + wrap(2) + "; " + sentinel
		statements, err := sqlglot.Parse(sql, "mysql")
		if err != nil {
			continue // fail-closed is always acceptable
		}
		if len(statements) == 0 || statements[0] == nil {
			continue
		}
		if span, ok := statements[0].SpanText(); ok && strings.Contains(span, sentinel) {
			t.Fatalf("MERGE: trailing statement absorbed into routine span\nsql:  %s\nspan: %s", sql, span)
		}
		// Escape (the dual fail-open): body text surfacing as extra top-level statements.
		// A successful parse of `<routine>; <sentinel>` is exactly those two statements.
		if len(statements) != 2 {
			t.Fatalf("ESCAPE: %d statements from routine+sentinel\nsql: %s", len(statements), sql)
		}
	}

	// Unterminated bodies (the closing END missing): the routine is invalid (real MySQL
	// 1064), and the sentinel written INSIDE it must never execute as its own statement.
	for i := 0; i < 1000; i++ {
		open := []string{"BEGIN ", "IF @a THEN ", "lbl: LOOP ", "WHILE @x DO ", "CASE @x WHEN 1 THEN "}[rng.Intn(5)]
		sql := headers[rng.Intn(len(headers))] + " " + open + stmts[rng.Intn(len(stmts))] + "; " + sentinel
		statements, err := sqlglot.Parse(sql, "mysql")
		if err != nil {
			continue
		}
		for _, s := range statements[1:] {
			if s == nil {
				continue
			}
			if span, ok := s.SpanText(); ok && strings.Contains(span, sentinel) {
				t.Fatalf("ESCAPE: unterminated body emitted its content as a top-level statement\nsql: %s", sql)
			}
		}
	}
}
