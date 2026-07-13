package parser_test

// Parser-shape checks for parseChar/parseCharsetName/parseConvert's USING branch
// (parser/parser_functions.go), which port _parse_char/_parse_charset_name (parser.py:
// 7836-7851), MySQL's _parse_charset_name override (parsers/mysql.py:523-535), and the
// CONVERT(x USING <charset>) branch of _parse_convert (parser.py:7965-7975). Round-trip SQL
// coverage lives in generator/chr_test.go; this file guards the AST shape those round-trips
// depend on.

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func TestParseChar(t *testing.T) {
	expr := parseOneDialect(t, "SELECT CHAR(77, 121, 83, 81, '76')", "mysql").Expressions()[0]
	if expr.Kind() != exp.KindChr {
		t.Fatalf("kind = %v, want Chr:\n%s", expr.Kind(), expr.ToS())
	}
	if got, want := len(expressionsForArg(expr, "expressions")), 5; got != want {
		t.Fatalf("expressions count = %d, want %d:\n%s", got, want, expr.ToS())
	}
	if expr.Arg("charset") != nil {
		t.Fatalf("charset should be nil without USING:\n%s", expr.ToS())
	}
}

func TestParseCharUsingCharset(t *testing.T) {
	expr := parseOneDialect(t, "SELECT CHAR(65 USING utf8mb4)", "mysql").Expressions()[0]
	charset := exprArg(t, expr, "charset")
	if charset.Kind() != exp.KindVar || charset.Name() != "utf8mb4" {
		t.Fatalf("charset = %v %q, want Var(utf8mb4):\n%s", charset.Kind(), charset.Name(), expr.ToS())
	}
}

// TestParseCharsetNameMySQLQuoting guards MySQL's charset-name quoting rules: "safe"
// (identifier-shaped) quoted names unwrap to a bare Var (renders unquoted), while names that
// need quoting (e.g. containing a space) keep their Identifier node (preserves quoting).
func TestParseCharsetNameMySQLQuoting(t *testing.T) {
	safe := exprArg(t, parseOneDialect(t, "SELECT CHAR(65 USING `binary`)", "mysql").Expressions()[0], "charset")
	if safe.Kind() != exp.KindVar || safe.Name() != "binary" {
		t.Fatalf("safe charset = %v %q, want Var(binary):\n%s", safe.Kind(), safe.Name(), safe.ToS())
	}

	unsafe := exprArg(t, parseOneDialect(t, "SELECT CHAR(65 USING `my charset`)", "mysql").Expressions()[0], "charset")
	if unsafe.Kind() != exp.KindIdentifier || unsafe.Name() != "my charset" {
		t.Fatalf("unsafe charset = %v %q, want Identifier(\"my charset\"):\n%s", unsafe.Kind(), unsafe.Name(), unsafe.ToS())
	}

	bareBinary := exprArg(t, parseOneDialect(t, "SELECT CHAR(65 USING BINARY)", "mysql").Expressions()[0], "charset")
	if bareBinary.Kind() != exp.KindVar || bareBinary.Name() != "BINARY" {
		t.Fatalf("bare BINARY charset = %v %q, want Var(BINARY):\n%s", bareBinary.Kind(), bareBinary.Name(), bareBinary.ToS())
	}
}

// TestParseConvertUsing guards CONVERT(x USING <charset>) building a Cast whose `to` is the
// synthetic CHARACTER_SET data type (parser.py:7969), not a dedicated Convert node.
func TestParseConvertUsing(t *testing.T) {
	expr := parseOneDialect(t, "SELECT CONVERT(x USING utf8mb4)", "mysql").Expressions()[0]
	if expr.Kind() != exp.KindCast {
		t.Fatalf("kind = %v, want Cast:\n%s", expr.Kind(), expr.ToS())
	}
	to := exprArg(t, expr, "to")
	if to.Kind() != exp.KindDataType || to.Arg("this") != exp.DTypeCharacterSet {
		t.Fatalf("to = %v %v, want DataType(CHARACTER_SET):\n%s", to.Kind(), to.Arg("this"), expr.ToS())
	}
	kind := exprArg(t, to, "kind")
	if kind.Kind() != exp.KindVar || kind.Name() != "utf8mb4" {
		t.Fatalf("kind = %v %q, want Var(utf8mb4):\n%s", kind.Kind(), kind.Name(), expr.ToS())
	}
}
