package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestParseTransaction ports test_parser.py:205 test_transactions (the BEGIN/START ->
// "BEGIN", modes-list cases), restricted to base + MySQL + Postgres (the dialects this
// repo targets). Upstream's presto-only "START TRANSACTION READ WRITE, ISOLATION LEVEL
// SERIALIZABLE" case is replaced with an equivalent BEGIN TRANSACTION form, since bare
// START only resolves to a BEGIN token under MySQL here (dialects/mysql.go:81), not under
// presto (an unsupported dialect). Upstream's bigquery case (BEGIN parsing as a
// non-Transaction block) is out of scope for the same reason.
func TestParseTransaction(t *testing.T) {
	txn := parseOne(t, "BEGIN TRANSACTION")
	if txn.Kind() != exp.KindTransaction || txn.Arg("this") != nil {
		t.Fatalf("BEGIN TRANSACTION mismatch:\n%s", txn.ToS())
	}
	if modes, _ := txn.Arg("modes").([]string); len(modes) != 0 {
		t.Fatalf("BEGIN TRANSACTION modes should be empty: %v", modes)
	}
	if sql, err := generateSQL(t, txn, ""); err != nil || sql != "BEGIN" {
		t.Fatalf("BEGIN TRANSACTION sql = %q, err = %v, want \"BEGIN\"", sql, err)
	}

	txn = parseOneDialect(t, "START TRANSACTION", "mysql")
	if txn.Kind() != exp.KindTransaction || txn.Arg("this") != nil {
		t.Fatalf("START TRANSACTION mismatch:\n%s", txn.ToS())
	}
	if sql, err := generateSQL(t, txn, "mysql"); err != nil || sql != "BEGIN" {
		t.Fatalf("START TRANSACTION sql = %q, err = %v, want \"BEGIN\"", sql, err)
	}

	// The transaction kind (this) is parsed but never rendered by transaction_sql
	// (generator.py:4140-4143), matching upstream's own asymmetry.
	txn = parseOne(t, "BEGIN DEFERRED TRANSACTION")
	if txn.Kind() != exp.KindTransaction || txn.Arg("this") != "DEFERRED" {
		t.Fatalf("BEGIN DEFERRED TRANSACTION mismatch:\n%s", txn.ToS())
	}
	if sql, err := generateSQL(t, txn, ""); err != nil || sql != "BEGIN" {
		t.Fatalf("BEGIN DEFERRED TRANSACTION sql = %q, err = %v, want \"BEGIN\"", sql, err)
	}

	txn = parseOne(t, "BEGIN TRANSACTION READ WRITE, ISOLATION LEVEL SERIALIZABLE")
	modes, _ := txn.Arg("modes").([]string)
	if len(modes) != 2 || modes[0] != "READ WRITE" || modes[1] != "ISOLATION LEVEL SERIALIZABLE" {
		t.Fatalf("modes mismatch: %v\n%s", modes, txn.ToS())
	}
	if sql, err := generateSQL(t, txn, ""); err != nil || sql != "BEGIN READ WRITE, ISOLATION LEVEL SERIALIZABLE" {
		t.Fatalf("modes sql = %q, err = %v", sql, err)
	}
}

// TestParseCommitOrRollback ports the COMMIT/ROLLBACK half of parser.py:8682-8700
// (_parse_commit_or_rollback), including the postgres AND [NO] CHAIN suffix and the
// TO [SAVEPOINT] savepoint form.
func TestParseCommitOrRollback(t *testing.T) {
	commit := parseOne(t, "COMMIT")
	if commit.Kind() != exp.KindCommit || commit.Arg("chain") != nil {
		t.Fatalf("COMMIT mismatch:\n%s", commit.ToS())
	}

	commit = parseOne(t, "COMMIT WORK")
	if sql, err := generateSQL(t, commit, ""); err != nil || sql != "COMMIT" {
		t.Fatalf("COMMIT WORK sql = %q, err = %v", sql, err)
	}

	commit = parseOne(t, "COMMIT AND CHAIN")
	if commit.Arg("chain") != true {
		t.Fatalf("COMMIT AND CHAIN chain mismatch:\n%s", commit.ToS())
	}
	if sql, err := generateSQL(t, commit, ""); err != nil || sql != "COMMIT AND CHAIN" {
		t.Fatalf("COMMIT AND CHAIN sql = %q, err = %v", sql, err)
	}

	commit = parseOne(t, "COMMIT AND NO CHAIN")
	if commit.Arg("chain") != false {
		t.Fatalf("COMMIT AND NO CHAIN chain mismatch:\n%s", commit.ToS())
	}
	if sql, err := generateSQL(t, commit, ""); err != nil || sql != "COMMIT AND NO CHAIN" {
		t.Fatalf("COMMIT AND NO CHAIN sql = %q, err = %v", sql, err)
	}

	rollback := parseOne(t, "ROLLBACK")
	if rollback.Kind() != exp.KindRollback || rollback.Arg("savepoint") != nil {
		t.Fatalf("ROLLBACK mismatch:\n%s", rollback.ToS())
	}

	rollback = parseOne(t, "ROLLBACK TO b")
	if sp := exprArg(t, rollback, "savepoint"); sp.Name() != "b" {
		t.Fatalf("ROLLBACK TO b savepoint mismatch:\n%s", rollback.ToS())
	}
	if sql, err := generateSQL(t, rollback, ""); err != nil || sql != "ROLLBACK TO b" {
		t.Fatalf("ROLLBACK TO b sql = %q, err = %v", sql, err)
	}

	rollback = parseOne(t, "ROLLBACK TO SAVEPOINT sp1")
	if sp := exprArg(t, rollback, "savepoint"); sp.Name() != "sp1" {
		t.Fatalf("ROLLBACK TO SAVEPOINT sp1 savepoint mismatch:\n%s", rollback.ToS())
	}
	if sql, err := generateSQL(t, rollback, ""); err != nil || sql != "ROLLBACK TO sp1" {
		t.Fatalf("ROLLBACK TO SAVEPOINT sp1 sql = %q, err = %v", sql, err)
	}

	// AND NO CHAIN is parsed but never rendered on Rollback: upstream's Rollback
	// arg_types has no "chain" key (rollback_sql / generator.py:4152-4156 only renders
	// savepoint), so the chain suffix is silently dropped - matching upstream exactly.
	rollback = parseOne(t, "ROLLBACK WORK TO SAVEPOINT sp1 AND NO CHAIN")
	if sql, err := generateSQL(t, rollback, ""); err != nil || sql != "ROLLBACK TO sp1" {
		t.Fatalf("ROLLBACK WORK ... AND NO CHAIN sql = %q, err = %v", sql, err)
	}
}
