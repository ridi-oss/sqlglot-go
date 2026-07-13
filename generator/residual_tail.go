package generator

import (
	"fmt"

	"github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
)

// bitstringSQL ports bitstring_sql (generator.py:1568-1572): re-quote a bit-string literal
// using the dialect's BIT_START/BIT_END, or fall back to its decimal integer value if the
// dialect has no bit-string family at all (base).
func (g *Generator) bitstringSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	if g.dialect.BitStart != "" {
		return g.dialect.BitStart + this + g.dialect.BitEnd
	}
	return parseIntBaseToDecimalString(this, 2)
}

// hexstringSQL ports hexstring_sql (generator.py:1574-1597). The upstream signature also
// takes an optional binary_function_repr param, used only by dialects outside base/mysql/
// postgres to transpile a BINARY/BLOB-typed hex string into e.g. UNHEX(...); no in-scope
// caller ever supplies one, so it's omitted here (every conditional below that upstream
// gates on `binary_function_repr` collapses to its "not supplied" branch).
func (g *Generator) hexstringSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	isIntegerType := boolValue(e.Arg("is_integer"))

	// (is_integer_type and not HEX_STRING_IS_INTEGER_TYPE) or (not HEX_START and not
	// binary_function_repr). No in-scope parser sets is_integer (and base/mysql/postgres all
	// have HEX_STRING_IS_INTEGER_TYPE=false), but a hand-built HexString{is_integer:true} must
	// still render as its integer value even when the write dialect has a HEX_START, so port the
	// full condition rather than collapsing it to `HEX_START == ""`.
	if (isIntegerType && !g.dialect.HexStringIsIntegerType) || g.dialect.HexStart == "" {
		return parseIntBaseToDecimalString(this, 16)
	}

	if !isIntegerType && g.dialect.HexStringIsIntegerType {
		g.unsupported("Unsupported transpilation from BINARY/BLOB hex string")
	}

	return g.dialect.HexStart + this + g.dialect.HexEnd
}

// bytestringSQL ports bytestring_sql (generator.py:1599-1628): re-quote a byte-string
// literal using the dialect's BYTE_START/BYTE_END (escape_backslash=False, so a literal
// backslash - e.g. postgres `e'\176'`'s octal escape - round-trips unchanged instead of
// being doubled), otherwise fall back to a plain string literal (mysql, whose regular
// strings support backslash escapes) or "unsupported" (base).
func (g *Generator) bytestringSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")

	if g.dialect.ByteStart != "" {
		escaped := g.escapeStrOpts(this, escapeStrOptions{
			delimiter:        g.dialect.ByteEnd,
			escapedDelimiter: g.escapedByteQuoteEnd,
			isByteString:     true,
		})
		isBytes := boolValue(e.Arg("is_bytes"))
		delimited := g.dialect.ByteStart + escaped + g.dialect.ByteEnd

		// Neither cast-wrap branch is reachable for base/mysql/postgres: no in-scope parser sets
		// is_bytes, and BYTE_STRING_IS_BYTES_TYPE is false for all of them (see its doc). Kept for
		// 1:1 shape with bytestring_sql. Upstream casts exp.cast(delimited_byte_string, ...), which
		// maybe-parses the rendered literal back into a ByteString; a fresh ByteString{this} node is
		// that same re-parse, so cast it directly (casting a LiteralString would re-quote it as a
		// plain string, e.g. CAST('e''abc''' AS BYTEA) instead of CAST(e'abc' AS BYTEA)).
		if isBytes && !g.dialect.ByteStringIsBytesType {
			reparsed := expressions.ByteString(expressions.Args{"this": this})
			return g.gen(expressions.Cast(expressions.Args{"this": reparsed, "to": expressions.DTypeBinary.IntoExpr(nil)}))
		}
		if !isBytes && g.dialect.ByteStringIsBytesType {
			reparsed := expressions.ByteString(expressions.Args{"this": this})
			return g.gen(expressions.Cast(expressions.Args{"this": reparsed, "to": expressions.DTypeVarchar.IntoExpr(nil)}))
		}
		return delimited
	}

	if g.dialect.TokenizerConfig.StringEscapes['\\'] {
		return g.gen(expressions.LiteralString(this))
	}

	g.unsupported("Byte strings are not supported for " + g.dialect.Name)
	return ""
}

// parseIntBaseToDecimalString mirrors Python's `int(this, base)`, used by bitstring_sql/
// hexstring_sql's integer fallback (arbitrary-precision, since a bit/hex literal can exceed 64
// bits) when the write dialect has no bit/hex family. It reuses the one CPython-int parser
// (tokens.ParseIntPython) the tokenizer validates literals with, so the accepted forms match
// exactly. An invalid payload panics, matching CPython's ValueError - e.g. transpiling a bare
// `0x_FF` (payload "_FF", validated as the full "0x_FF") to base, where int("_FF", 16) raises;
// Generate recovers the panic into an error rather than dropping the literal.
func parseIntBaseToDecimalString(text string, base int) string {
	n, ok := tokens.ParseIntPython(text, base)
	if !ok {
		panic(fmt.Errorf("invalid literal for int() with base %d: '%s'", base, text))
	}
	return n.String()
}

// sessionParameterSQL ports sessionparameter_sql (generator.py:3410-3415): mysql
// `@@GLOBAL.max_connections` / bare `@@x`.
func (g *Generator) sessionParameterSQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	kind := e.Text("kind")
	if kind != "" {
		kind += "."
	}
	return "@@" + kind + this
}

// propertyEQSQL ports propertyeq_sql (generator.py:4408-4409): the `:=` ASSIGNMENT
// operator, e.g. mysql `@var1 := 1`.
func (g *Generator) propertyEQSQL(e expressions.Expression) string { return g.binary(e, ":=") }

// distanceSQL/distanceNdSQL port distance_sql/distancend_sql (generator.py:4396-4400): the
// postgres `<->`/`<<->>` distance operators.
func (g *Generator) distanceSQL(e expressions.Expression) string   { return g.binary(e, "<->") }
func (g *Generator) distanceNdSQL(e expressions.Expression) string { return g.binary(e, "<<->>") }

// concatSQL ports concat_sql (generator.py:3667-3682). Same-dialect the adjacent-string-literal
// Concat this port builds carries coalesce=dialect.CONCAT_COALESCE, so neither branch below
// fires; but both are reachable across dialects (e.g. transpiling `'a' 'b'` base->postgres) and
// via hand-built ASTs, so port them faithfully.
func (g *Generator) concatSQL(e expressions.Expression) string {
	if g.dialect.ConcatCoalesce && !boolValue(e.Arg("coalesce")) {
		// The dialect's CONCAT coalesces NULLs to '', but this expression does not: transpile to
		// `||`, which propagates NULL instead (concat_to_dpipe_sql, dialect.py:1804-1805). Fires
		// when a coalesce=false Concat (e.g. built by a non-coalescing read dialect) is generated
		// for postgres (CONCAT_COALESCE=true).
		return g.concatToDPipe(e)
	}
	// SUPPORTS_SINGLE_ARG_CONCAT defaults true and isn't overridden by base/mysql/postgres, so
	// the single-arg passthrough (generator.py:3678-3680) never triggers here.
	return g.funcCall("CONCAT", g.convertConcatArgs(e), "(", ")", true)
}

// concatToDPipe ports concat_to_dpipe_sql (dialect.py:1804-1805): left-fold the CONCAT args
// into a `a || b || c` DPipe chain.
func (g *Generator) concatToDPipe(e expressions.Expression) string {
	exprs := listFromValue(e.Arg("expressions"))
	if len(exprs) == 0 {
		return ""
	}
	acc := asExpression(exprs[0])
	for _, next := range exprs[1:] {
		acc = expressions.DPipe(expressions.Args{"this": acc, "expression": asExpression(next)})
	}
	return g.gen(acc)
}

// convertConcatArgs ports convert_concat_args (generator.py:3636-3665) for the Concat shape
// (ConcatWs and the STRICT_STRING_CONCAT `safe`->cast(TEXT) branch are out of scope for
// base/mysql/postgres, which don't set those flags). When the write dialect's CONCAT does NOT
// coalesce NULLs but the expression asks it to (coalesce=true), each non-string arg is wrapped
// in a COALESCE with an empty-string default. Upstream annotate_types the arg to decide (skipping
// an already-string or ARRAY-typed arg); this port defers full annotate_types (ROADMAP 4c) and
// wraps every arg that isn't a statically-known string literal - which matches upstream for the
// reachable cases (an unknown column annotates to a non-string type and is wrapped; a string
// literal is left alone).
func (g *Generator) convertConcatArgs(e expressions.Expression) []any {
	args := listFromValue(e.Arg("expressions"))
	if g.dialect.ConcatCoalesce || !boolValue(e.Arg("coalesce")) {
		return args
	}
	wrapped := make([]any, len(args))
	for i, a := range args {
		arg := asExpression(a)
		if arg == nil || arg.IsString() {
			wrapped[i] = a
			continue
		}
		wrapped[i] = expressions.Coalesce(expressions.Args{
			"this":        arg,
			"expressions": []expressions.Expression{expressions.LiteralString("")},
		})
	}
	return wrapped
}

// arraySQL ports postgres's array_sql override (generators/postgres.py:502-509): the
// bracket-notation ARRAY[...] literal, or ARRAY(<subquery>) when the sole element is a
// query. Base/mysql have no array_sql override upstream (mysql's only difference is an
// added "Arrays are not supported" warning before it falls back to the same rendering
// functionFallbackSQL already gives it, generators/mysql.py:648-650 - not modeled, since no
// in-scope corpus case exercises an mysql/base Array node), so they fall through to
// functionFallbackSQL here too, which renders the identical ARRAY(a, b, c) paren form.
func (g *Generator) arraySQL(e expressions.Expression) string {
	if g.dialect.Name != "postgres" {
		return g.functionFallbackSQL(e)
	}
	exprs := listFromValue(e.Arg("expressions"))
	funcName := g.normalizeFunc("ARRAY")
	if len(exprs) > 0 {
		if elem := asExpression(exprs[0]); elem != nil && elem.Is(expressions.TraitQuery) {
			return g.funcCall(funcName, []any{elem}, "(", ")", false)
		}
	}
	return funcName + g.inlineArraySQL(e)
}

func init() {
	dispatch[expressions.KindArray] = (*Generator).arraySQL
	dispatch[expressions.KindConcat] = (*Generator).concatSQL
	dispatch[expressions.KindBitString] = (*Generator).bitstringSQL
	dispatch[expressions.KindHexString] = (*Generator).hexstringSQL
	dispatch[expressions.KindByteString] = (*Generator).bytestringSQL
	dispatch[expressions.KindSessionParameter] = (*Generator).sessionParameterSQL
	dispatch[expressions.KindPropertyEQ] = (*Generator).propertyEQSQL
	dispatch[expressions.KindDistance] = (*Generator).distanceSQL
	dispatch[expressions.KindDistanceNd] = (*Generator).distanceNdSQL
}
