package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// roundTripAffix parses sql under dialect and asserts the generated SQL round-trips
// byte-for-byte to want, verified against the pinned Python reference (v30.12.0).
func roundTripAffix(t *testing.T, dialect, sql, want string) exp.Expression {
	t.Helper()
	root := parseOneDialect(t, sql, dialect)
	got, err := generateSQL(t, root, dialect)
	if err != nil {
		t.Fatalf("[%s] Generate(%q): %v", dialect, sql, err)
	}
	if got != want {
		t.Fatalf("[%s] round-trip = %q, want %q:\n%s", dialect, got, want, root.ToS())
	}
	return root
}

// TestBitwiseShiftAndCoalesce covers _parse_bitwise's `<<`/`>>` two-token shift operators
// (parser.py:6084-6091, matchPair since LT LT / GT GT are two adjacent single-char tokens) and
// the `??` Coalesce shorthand (parser.py:6080-6083), plus mixed precedence with the pre-existing
// `|`/`&`/`^` bitwise operators (parity_gaps.txt cases 73-75).
func TestBitwiseShiftAndCoalesce(t *testing.T) {
	shl := parseOne(t, "SELECT x << 1").Expressions()[0]
	if shl.Kind() != exp.KindBitwiseLeftShift {
		t.Fatalf("x << 1: kind = %v, want BitwiseLeftShift:\n%s", shl.Kind(), shl.ToS())
	}
	roundTripAffix(t, "", "x << 1", "x << 1")

	shr := parseOne(t, "SELECT x >> 1").Expressions()[0]
	if shr.Kind() != exp.KindBitwiseRightShift {
		t.Fatalf("x >> 1: kind = %v, want BitwiseRightShift:\n%s", shr.Kind(), shr.ToS())
	}
	roundTripAffix(t, "", "x >> 1", "x >> 1")

	// x >> 1 | 1 & 1 ^ 1 -> BitwiseXor(BitwiseAnd(BitwiseOr(BitwiseRightShift(x,1),1),1),1):
	// left-associative, all at the same _parse_bitwise loop precedence.
	root := roundTripAffix(t, "", "x >> 1 | 1 & 1 ^ 1", "x >> 1 | 1 & 1 ^ 1")
	xorNode := root
	if xorNode.Kind() != exp.KindBitwiseXor {
		t.Fatalf("outermost kind = %v, want BitwiseXor:\n%s", xorNode.Kind(), root.ToS())
	}
	andNode := exprArg(t, xorNode, "this")
	if andNode.Kind() != exp.KindBitwiseAnd {
		t.Fatalf("BitwiseXor.this kind = %v, want BitwiseAnd:\n%s", andNode.Kind(), root.ToS())
	}
	orNode := exprArg(t, andNode, "this")
	if orNode.Kind() != exp.KindBitwiseOr {
		t.Fatalf("BitwiseAnd.this kind = %v, want BitwiseOr:\n%s", orNode.Kind(), root.ToS())
	}
	if exprArg(t, orNode, "this").Kind() != exp.KindBitwiseRightShift {
		t.Fatalf("BitwiseOr.this kind = %v, want BitwiseRightShift:\n%s", orNode.Kind(), root.ToS())
	}

	// DQMARK -> Coalesce(this, expressions=[...]).
	coalesce := parseOne(t, "SELECT x ?? 1").Expressions()[0]
	if coalesce.Kind() != exp.KindCoalesce {
		t.Fatalf("x ?? 1: kind = %v, want Coalesce:\n%s", coalesce.Kind(), coalesce.ToS())
	}
	roundTripAffix(t, "", "x ?? 1", "COALESCE(x, 1)")
}

// TestUnaryBitwiseNotSqrtCbrt covers UNARY_PARSERS' TILDE/PIPE_SLASH/DPIPE_SLASH entries
// (parser.py:1113-1120): `~x` -> BitwiseNot in every dialect (parity_gaps.txt cases 69, 231),
// and postgres-only `|/ x` -> Sqrt / `||/ x` -> Cbrt (cases 242, 243). Postgres also remaps the
// `~` lexeme itself from TILDE to RLIKE (since `~` doubles as the binary REGEXP-LIKE operator
// there), so postgres's BitwiseNot must come through the RLIKE-in-prefix-position branch
// instead, exercised separately below.
func TestUnaryBitwiseNotSqrtCbrt(t *testing.T) {
	for _, dialect := range []string{"", "mysql", "postgres"} {
		root := roundTripAffix(t, dialect, "SELECT ~x", "SELECT ~x")
		proj := root.Expressions()[0]
		if proj.Kind() != exp.KindBitwiseNot {
			t.Fatalf("[%s] ~x: kind = %v, want BitwiseNot:\n%s", dialect, proj.Kind(), root.ToS())
		}
		if this := exprArg(t, proj, "this"); this.Kind() != exp.KindColumn || this.Name() != "x" {
			t.Fatalf("[%s] ~x: this should be Column(x):\n%s", dialect, root.ToS())
		}
	}

	sqrt := roundTripAffix(t, "postgres", "|/ x", "SQRT(x)")
	if sqrt.Kind() != exp.KindSqrt {
		t.Fatalf("|/ x: kind = %v, want Sqrt:\n%s", sqrt.Kind(), sqrt.ToS())
	}

	cbrt := roundTripAffix(t, "postgres", "||/ x", "CBRT(x)")
	if cbrt.Kind() != exp.KindCbrt {
		t.Fatalf("||/ x: kind = %v, want Cbrt:\n%s", cbrt.Kind(), cbrt.ToS())
	}
}

// TestCollateOperator covers _parse_term's COLLATE handling (parser.py:908-913,6107-6115),
// including the post-build normalization that collapses a single-part column operand into a
// bare Var/Identifier while leaving a qualified one (e.g. `pg_catalog."default"`) as a Column
// (parity_gaps.txt cases 51, 220-adjacent).
func TestCollateOperator(t *testing.T) {
	root := roundTripAffix(t, "", "SELECT a FROM x WHERE a COLLATE 'utf8_general_ci' = 'b'",
		"SELECT a FROM x WHERE a COLLATE 'utf8_general_ci' = 'b'")
	where := exprArg(t, root, "where")
	eq := exprArg(t, where, "this")
	if eq.Kind() != exp.KindEQ {
		t.Fatalf("kind = %v, want EQ:\n%s", eq.Kind(), root.ToS())
	}
	collate := exprArg(t, eq, "this")
	if collate.Kind() != exp.KindCollate {
		t.Fatalf("EQ.this kind = %v, want Collate:\n%s", collate.Kind(), root.ToS())
	}
	// The collation operand here is a quoted STRING literal, not a Column, so the
	// single-part-column normalization does not apply and it stays a Literal.
	if expr := exprArg(t, collate, "expression"); expr.Kind() != exp.KindLiteral {
		t.Fatalf("Collate.expression kind = %v, want Literal:\n%s", expr.Kind(), root.ToS())
	}

	// Unquoted identifier collation operand -> normalized to Var.
	varRoot := roundTripAffix(t, "", "SELECT a COLLATE utf8_bin", "SELECT a COLLATE utf8_bin")
	varCollate := varRoot.Expressions()[0]
	if got := exprArg(t, varCollate, "expression"); got.Kind() != exp.KindVar || got.Name() != "utf8_bin" {
		t.Fatalf("Collate.expression kind = %v, want Var(utf8_bin):\n%s", got.Kind(), varRoot.ToS())
	}

	// Quoted identifier collation operand -> kept as the Identifier itself (not wrapped back
	// into a Column), still round-tripping its quoting.
	quotedRoot := roundTripAffix(t, "", `SELECT a COLLATE "utf8_bin"`, `SELECT a COLLATE "utf8_bin"`)
	quotedCollate := quotedRoot.Expressions()[0]
	if got := exprArg(t, quotedCollate, "expression"); got.Kind() != exp.KindIdentifier || got.Name() != "utf8_bin" {
		t.Fatalf("Collate.expression kind = %v, want Identifier(utf8_bin):\n%s", got.Kind(), quotedRoot.ToS())
	}

	// A qualified (multi-part) collation operand is left as a Column, e.g. Postgres'
	// `pg_catalog."default"` (parity_gaps.txt case 220's COLLATE half).
	qualifiedRoot := roundTripAffix(t, "postgres", `SELECT a COLLATE pg_catalog."default"`, `SELECT a COLLATE pg_catalog."default"`)
	qualifiedCollate := qualifiedRoot.Expressions()[0]
	if got := exprArg(t, qualifiedCollate, "expression"); got.Kind() != exp.KindColumn {
		t.Fatalf("Collate.expression kind = %v, want Column:\n%s", got.Kind(), qualifiedRoot.ToS())
	}
}

// TestMySQLConjunctionDisjunctionAliases covers mysql's CONJUNCTION/DISJUNCTION extensions
// (parsers/mysql.py:72-81): `&&` -> And, `XOR` -> Xor, and (since DPIPE_IS_STRING_CONCAT=False
// frees `||` up) `||` -> Or (parity_gaps.txt cases 146, 147, 156).
func TestMySQLConjunctionDisjunctionAliases(t *testing.T) {
	and := roundTripAffix(t, "mysql", "SELECT 1 && 0", "SELECT 1 AND 0").Expressions()[0]
	if and.Kind() != exp.KindAnd {
		t.Fatalf("1 && 0: kind = %v, want And:\n%s", and.Kind(), and.ToS())
	}

	xor := roundTripAffix(t, "mysql", "SELECT 1 XOR 0", "SELECT 1 XOR 0").Expressions()[0]
	if xor.Kind() != exp.KindXor {
		t.Fatalf("1 XOR 0: kind = %v, want Xor:\n%s", xor.Kind(), xor.ToS())
	}

	or := roundTripAffix(t, "mysql", "SELECT a || b", "SELECT a OR b").Expressions()[0]
	if or.Kind() != exp.KindOr {
		t.Fatalf("a || b: kind = %v, want Or:\n%s", or.Kind(), or.ToS())
	}

	// Base dialect keeps `||` as string concatenation (DPipeIsStringConcat=true there),
	// unaffected by the mysql-only DISJUNCTION extension.
	dpipe := parseOne(t, "SELECT a || b").Expressions()[0]
	if dpipe.Kind() != exp.KindDPipe {
		t.Fatalf("base a || b: kind = %v, want DPipe:\n%s", dpipe.Kind(), dpipe.ToS())
	}
}
