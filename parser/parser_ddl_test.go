package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestParseCreate(t *testing.T) {
	create := parseOne(t, "CREATE TABLE t (a INT, b VARCHAR(10))")
	if create.Kind() != exp.KindCreate || create.Arg("kind") != "TABLE" {
		t.Fatalf("create table mismatch:\n%s", create.ToS())
	}
	schema := exprArg(t, create, "this")
	cols := schema.Expressions()
	if schema.Kind() != exp.KindSchema || len(cols) != 2 {
		t.Fatalf("create schema mismatch:\n%s", create.ToS())
	}
	if cols[0].Kind() != exp.KindColumnDef || !exp.IsType(exprArg(t, cols[0], "kind"), exp.DTypeInt) {
		t.Fatalf("first column type mismatch:\n%s", create.ToS())
	}
	if cols[1].Kind() != exp.KindColumnDef || !exp.IsType(exprArg(t, cols[1], "kind"), exp.DTypeVarchar) {
		t.Fatalf("second column type mismatch:\n%s", create.ToS())
	}

	create = parseOne(t, "CREATE TABLE t AS SELECT 1")
	if exprArg(t, create, "expression").Kind() != exp.KindSelect {
		t.Fatalf("CTAS expression mismatch:\n%s", create.ToS())
	}

	create = parseOne(t, "CREATE OR REPLACE VIEW v AS SELECT a FROM t")
	if create.Kind() != exp.KindCreate || create.Arg("kind") != "VIEW" || create.Arg("replace") != true {
		t.Fatalf("create or replace view mismatch:\n%s", create.ToS())
	}

	create = parseOne(t, "CREATE TABLE IF NOT EXISTS t (a INT)")
	if create.Arg("exists") != true {
		t.Fatalf("IF NOT EXISTS mismatch:\n%s", create.ToS())
	}

	create = parseOne(t, "CREATE TABLE t (a INT) ENGINE=InnoDB")
	if create.Kind() != exp.KindCreate {
		t.Fatalf("property-bearing CREATE mismatch:\n%s", create.ToS())
	}
	properties := expressionsForArg(exprArg(t, create, "properties"), "expressions")
	if len(properties) != 1 || properties[0].Kind() != exp.KindEngineProperty || properties[0].Name() != "InnoDB" {
		t.Fatalf("ENGINE property mismatch:\n%s", create.ToS())
	}

	// Column constraints are structured as of this DDL slice (parser_ddl.go/
	// parser_constraints.go): a CREATE that carries them now parses into a real
	// exp.Create/exp.ColumnDef/exp.ColumnConstraint tree instead of degrading to Command
	// (this replaces the earlier "degrades to Command" regression-trap assertion).
	create = parseOne(t, "CREATE TABLE t (a INT NOT NULL)")
	if create.Kind() != exp.KindCreate {
		t.Fatalf("kind = %v, want Create:\n%s", create.Kind(), create.ToS())
	}
	col := exprArg(t, create, "this").Expressions()[0]
	if col.Kind() != exp.KindColumnDef {
		t.Fatalf("column mismatch:\n%s", create.ToS())
	}
	constraints := expressionsForArg(col, "constraints")
	if len(constraints) != 1 || constraints[0].Kind() != exp.KindColumnConstraint {
		t.Fatalf("constraints mismatch:\n%s", create.ToS())
	}
	if exprArg(t, constraints[0], "kind").Kind() != exp.KindNotNullColumnConstraint {
		t.Fatalf("constraint kind mismatch:\n%s", create.ToS())
	}

	create = parseOne(t, "CREATE TABLE t (id INT PRIMARY KEY, name VARCHAR(50) NOT NULL)")
	if create.Kind() != exp.KindCreate {
		t.Fatalf("kind = %v, want Create:\n%s", create.Kind(), create.ToS())
	}
	pkCols := exprArg(t, create, "this").Expressions()
	if len(pkCols) != 2 || pkCols[0].Kind() != exp.KindColumnDef || pkCols[1].Kind() != exp.KindColumnDef {
		t.Fatalf("columns mismatch:\n%s", create.ToS())
	}
	pkConstraints := expressionsForArg(pkCols[0], "constraints")
	if len(pkConstraints) != 1 || exprArg(t, pkConstraints[0], "kind").Kind() != exp.KindPrimaryKeyColumnConstraint {
		t.Fatalf("primary key constraint mismatch:\n%s", create.ToS())
	}

	create = parseOne(t, "CREATE TABLE t (a INT DEFAULT 0)")
	if create.Kind() != exp.KindCreate {
		t.Fatalf("kind = %v, want Create:\n%s", create.Kind(), create.ToS())
	}
	defCol := exprArg(t, create, "this").Expressions()[0]
	defConstraints := expressionsForArg(defCol, "constraints")
	if len(defConstraints) != 1 || exprArg(t, defConstraints[0], "kind").Kind() != exp.KindDefaultColumnConstraint {
		t.Fatalf("default constraint mismatch:\n%s", create.ToS())
	}
}
