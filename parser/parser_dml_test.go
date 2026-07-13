package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func exprArg(t *testing.T, expression exp.Expression, key string) exp.Expression {
	t.Helper()
	child, ok := expression.Arg(key).(exp.Expression)
	if !ok || child == nil {
		t.Fatalf("%v missing expression arg %q:\n%s", expression.Kind(), key, expression.ToS())
	}
	return child
}

func TestParseInsert(t *testing.T) {
	insert := parseOne(t, "INSERT INTO t VALUES (1, 2)")
	if insert.Kind() != exp.KindInsert {
		t.Fatalf("kind = %v, want Insert:\n%s", insert.Kind(), insert.ToS())
	}
	if exprArg(t, insert, "this").Kind() != exp.KindTable {
		t.Fatalf("insert target should be Table:\n%s", insert.ToS())
	}
	if exprArg(t, insert, "expression").Kind() != exp.KindValues {
		t.Fatalf("insert expression should be Values:\n%s", insert.ToS())
	}

	insert = parseOne(t, "INSERT INTO t (a, b) VALUES (1, 2)")
	schema := exprArg(t, insert, "this")
	if schema.Kind() != exp.KindSchema || len(schema.Expressions()) != 2 {
		t.Fatalf("insert schema target mismatch:\n%s", insert.ToS())
	}

	insert = parseOne(t, "INSERT INTO t SELECT * FROM s")
	if exprArg(t, insert, "expression").Kind() != exp.KindSelect {
		t.Fatalf("insert select expression mismatch:\n%s", insert.ToS())
	}

	insert = parseOne(t, "INSERT INTO t VALUES (1) ON CONFLICT (id) DO NOTHING")
	conflict := exprArg(t, insert, "conflict")
	if conflict.Kind() != exp.KindOnConflict || conflict.Arg("action").(exp.Expression).Name() != "DO NOTHING" {
		t.Fatalf("on conflict mismatch:\n%s", insert.ToS())
	}

	insert = parseOne(t, "INSERT INTO t VALUES (1) ON DUPLICATE KEY UPDATE a = 1")
	conflict = exprArg(t, insert, "conflict")
	if conflict.Arg("duplicate") != true || len(conflict.Expressions()) != 1 || conflict.Expressions()[0].Kind() != exp.KindEQ {
		t.Fatalf("on duplicate mismatch:\n%s", insert.ToS())
	}

	insert = parseOne(t, "INSERT INTO t SELECT 1 RETURNING id")
	if exprArg(t, insert, "returning").Kind() != exp.KindReturning {
		t.Fatalf("returning mismatch:\n%s", insert.ToS())
	}

	insert = parseOne(t, "WITH x AS (SELECT 1) INSERT INTO t SELECT * FROM x")
	if insert.Kind() != exp.KindInsert || insert.Arg("with_") == nil {
		t.Fatalf("CTE insert mismatch:\n%s", insert.ToS())
	}
}

func TestParseUpdateDeleteMerge(t *testing.T) {
	update := parseOne(t, "UPDATE t SET a = 1 WHERE b = 2")
	if update.Kind() != exp.KindUpdate || len(update.Expressions()) != 1 || exprArg(t, update, "where").Kind() != exp.KindWhere {
		t.Fatalf("update mismatch:\n%s", update.ToS())
	}

	update = parseOne(t, "UPDATE t SET a = 1 FROM s")
	if exprArg(t, update, "from_").Kind() != exp.KindFrom {
		t.Fatalf("update FROM mismatch:\n%s", update.ToS())
	}

	deleteExpr := parseOne(t, "DELETE FROM t WHERE a = 1")
	if deleteExpr.Kind() != exp.KindDelete || exprArg(t, deleteExpr, "this").Kind() != exp.KindTable || exprArg(t, deleteExpr, "where").Kind() != exp.KindWhere {
		t.Fatalf("delete mismatch:\n%s", deleteExpr.ToS())
	}

	deleteExpr = parseOne(t, "DELETE FROM a USING b WHERE a.x = b.x")
	if using := expressionsForArg(deleteExpr, "using"); len(using) != 1 {
		t.Fatalf("delete USING len = %d, want 1:\n%s", len(using), deleteExpr.ToS())
	}

	deleteExpr = parseOne(t, "DELETE t1, t2 FROM t1 JOIN t2")
	if tables := expressionsForArg(deleteExpr, "tables"); len(tables) != 2 {
		t.Fatalf("delete tables len = %d, want 2:\n%s", len(tables), deleteExpr.ToS())
	}

	merge := parseOne(t, "MERGE INTO t USING s ON t.id = s.id WHEN MATCHED THEN UPDATE SET a = 1 WHEN NOT MATCHED THEN INSERT VALUES (1)")
	if merge.Kind() != exp.KindMerge {
		t.Fatalf("merge kind mismatch:\n%s", merge.ToS())
	}
	whens := exprArg(t, merge, "whens")
	whenExprs := whens.Expressions()
	if whens.Kind() != exp.KindWhens || len(whenExprs) != 2 {
		t.Fatalf("merge whens mismatch:\n%s", merge.ToS())
	}
	if whenExprs[0].Kind() != exp.KindWhen || whenExprs[0].Arg("matched") != true || exprArg(t, whenExprs[0], "then").Kind() != exp.KindUpdate {
		t.Fatalf("merge matched WHEN mismatch:\n%s", merge.ToS())
	}
	if whenExprs[1].Kind() != exp.KindWhen || whenExprs[1].Arg("matched") != false || exprArg(t, whenExprs[1], "then").Kind() != exp.KindInsert {
		t.Fatalf("merge not matched WHEN mismatch:\n%s", merge.ToS())
	}
}
