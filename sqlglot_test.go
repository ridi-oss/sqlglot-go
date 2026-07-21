package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

func TestTranspileEmpty(t *testing.T) {
	got, err := sqlglot.Transpile("", "", "", generator.Options{})
	if err != nil {
		t.Fatalf("Transpile empty error: %v", err)
	}
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("Transpile empty = %#v, want []string{\"\"}", got)
	}
}

// The identity.sql round-trip (formerly TestIdentity) now lives in TestCorpus
// (corpus_test.go), which covers both the base-dialect corpus (Scope A,
// identity.sql) and the per-dialect validate_identity corpus (Scope B,
// dialect_identity.jsonl) through one parse->generate->compare core, so
// identity.sql is exercised exactly once.

// TestDialectTypeArgs guards the polymorphic dialect contract of the top-level API: the
// dialect argument is a dialects.DialectType (nil | string | *dialects.Dialect), not just a
// string. This locks the widened signatures so a future refactor cannot silently narrow them
// back to string and force every caller (agents included) onto the string form.
func TestDialectTypeArgs(t *testing.T) {
	// Dialect-neutral SQL so the nil (base) form parses too.
	const sql = "SELECT a FROM t"

	mysql, err := dialects.GetOrRaise("mysql")
	if err != nil {
		t.Fatalf("GetOrRaise(mysql): %v", err)
	}

	cases := []struct {
		name    string
		dialect dialects.DialectType
	}{
		{"string", "mysql"},
		{"pointer", mysql},
		{"nil", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Every top-level entry point accepts every DialectType form (nil == base).
			e, err := sqlglot.ParseOne(sql, tc.dialect)
			if err != nil {
				t.Fatalf("ParseOne(%v): %v", tc.name, err)
			}
			if _, err := sqlglot.Generate(e, tc.dialect, generator.Options{}); err != nil {
				t.Fatalf("Generate(%v): %v", tc.name, err)
			}
			if _, err := sqlglot.Tokenize(sql, tc.dialect); err != nil {
				t.Fatalf("Tokenize(%v): %v", tc.name, err)
			}
			stmts, err := sqlglot.Parse(sql, tc.dialect)
			if err != nil || len(stmts) != 1 {
				t.Fatalf("Parse(%v) = %d stmts, err %v", tc.name, len(stmts), err)
			}
			if _, err := sqlglot.ParseInto("INT", tc.dialect, exp.KindDataType); err != nil {
				t.Fatalf("ParseInto(%v): %v", tc.name, err)
			}
		})
	}

	// A pre-resolved *Dialect threaded through Transpile behaves identically to its name,
	// including MySQL-specific quoting.
	const mysqlSQL = "SELECT `a` FROM `t`"
	viaName, err := sqlglot.Transpile(mysqlSQL, "mysql", "mysql", generator.Options{})
	if err != nil {
		t.Fatalf("Transpile(name): %v", err)
	}
	viaPtr, err := sqlglot.Transpile(mysqlSQL, mysql, mysql, generator.Options{})
	if err != nil {
		t.Fatalf("Transpile(ptr): %v", err)
	}
	if len(viaName) != 1 || len(viaPtr) != 1 || viaName[0] != viaPtr[0] {
		t.Fatalf("Transpile string vs *Dialect diverged: %q vs %q", viaName, viaPtr)
	}
	if viaName[0] != mysqlSQL {
		t.Fatalf("Transpile round-trip = %q, want %q", viaName[0], mysqlSQL)
	}
}

// TestDialectTypeBuilderArgs guards the polymorphic dialect contract of the expressions
// builders and GenerateOptions.Dialect, which the import-cycle fix widened from string to
// DialectType. Each builder accepts a *dialects.Dialect (and nil), and a *Dialect produces the
// same output as its name. If a refactor narrows any of these back to string, this stops
// compiling.
func TestDialectTypeBuilderArgs(t *testing.T) {
	mysql, err := dialects.GetOrRaise("mysql")
	if err != nil {
		t.Fatalf("GetOrRaise(mysql): %v", err)
	}

	// gen renders an expression under mysql for string-vs-*Dialect parity comparisons.
	gen := func(e exp.Expression) string {
		out, err := e.SQL(exp.GenerateOptions{Dialect: "mysql"})
		if err != nil {
			t.Fatalf("SQL: %v", err)
		}
		return out
	}

	// Every builder's dialect arg is a DialectType: the *Dialect form must accept the value and
	// produce the same node as the equivalent "mysql" string.
	if gen(exp.MaybeParse("1 + 1", mysql, false)) != gen(exp.MaybeParse("1 + 1", "mysql", false)) {
		t.Fatal("MaybeParse: *Dialect vs string diverged")
	}
	if gen(exp.Condition("a AND b", mysql, false)) != gen(exp.Condition("a AND b", "mysql", false)) {
		t.Fatal("Condition: *Dialect vs string diverged")
	}
	tblPtr, err := exp.ToTable("db.t", mysql, false, nil)
	if err != nil {
		t.Fatalf("ToTable(*Dialect): %v", err)
	}
	tblStr, err := exp.ToTable("db.t", "mysql", false, nil)
	if err != nil {
		t.Fatalf("ToTable(string): %v", err)
	}
	if gen(tblPtr) != gen(tblStr) {
		t.Fatal("ToTable: *Dialect vs string diverged")
	}
	dtPtr, err := exp.DataTypeBuild("VARCHAR(10)", mysql, false, false, nil)
	if err != nil || dtPtr == nil {
		t.Fatalf("DataTypeBuild(*Dialect): %v", err)
	}
	dtFromStr, err := exp.DataTypeFromStr("VARCHAR(10)", mysql, false, nil)
	if err != nil || dtFromStr == nil {
		t.Fatalf("DataTypeFromStr(*Dialect): %v", err)
	}

	// ParseIdentifier: IdentifierName as a string and dialect as nil / string / *Dialect, plus an
	// already-built Identifier Expression passed straight through.
	if e := exp.ParseIdentifier("x", nil); e == nil {
		t.Fatal("ParseIdentifier(string, nil) = nil")
	}
	if gen(exp.ParseIdentifier("a b", mysql)) != gen(exp.ParseIdentifier("a b", "mysql")) {
		t.Fatal("ParseIdentifier: *Dialect vs string diverged")
	}
	ident := exp.ToIdentifier("y")
	if got := exp.ParseIdentifier(ident, mysql); got != ident {
		t.Fatal("ParseIdentifier(Expression) should pass the node through unchanged")
	}

	// GenerateOptions.Dialect takes a DialectType: rendering with the *Dialect must match the
	// string form.
	e, err := sqlglot.ParseOne("SELECT `a` FROM `t`", mysql)
	if err != nil {
		t.Fatalf("ParseOne: %v", err)
	}
	viaPtr, err := e.SQL(exp.GenerateOptions{Dialect: mysql})
	if err != nil {
		t.Fatalf("SQL(Dialect: *Dialect): %v", err)
	}
	viaStr, err := e.SQL(exp.GenerateOptions{Dialect: "mysql"})
	if err != nil {
		t.Fatalf("SQL(Dialect: string): %v", err)
	}
	if viaPtr != viaStr {
		t.Fatalf("SQL *Dialect vs string diverged: %q vs %q", viaPtr, viaStr)
	}
}
