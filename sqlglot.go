package sqlglot

import (
	fmtstd "fmt"
	"strings"

	"github.com/ridi-oss/sqlglot-go/dialects"
	sqlerrors "github.com/ridi-oss/sqlglot-go/errors"
	"github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
	"github.com/ridi-oss/sqlglot-go/parser"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// Tokenize lexes sql under dialect into a token stream. It is a pure lexer, independent of the
// parser, so it tokenizes input the parser would reject — and it never silently truncates: an
// unterminated string, quoted identifier, block comment, or Postgres dollar-quote returns a
// non-nil error, which a consumer should treat as fail-closed. Ordinary comments are attached to a
// token's Comments (not emitted as stream tokens). For the byte-exact source lexeme of a token,
// slice the source by its Start/End offsets rather than reading Token.Text (see tokens.Token).
func Tokenize(sql string, dialect dialects.DialectType) ([]tokens.Token, error) {
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		return nil, err
	}
	return d.NewTokenizer().Tokenize(sql)
}

// Parse tokenizes and parses sql under dialect (see dialects.DialectType for the accepted
// forms: nil | string | *dialects.Dialect), returning one Expression per statement.
func Parse(sql string, dialect dialects.DialectType) ([]expressions.Expression, error) {
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		return nil, err
	}
	tokenizer := d.NewTokenizer()
	toks, err := tokenizer.Tokenize(sql)
	if err != nil {
		return nil, err
	}
	p := parser.New(d)
	return p.Parse(toks, sql)
}

// ParseOne parses a single statement from sql under dialect (see dialects.DialectType).
func ParseOne(sql string, dialect dialects.DialectType) (expressions.Expression, error) {
	res, err := Parse(sql, dialect)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 || res[0] == nil {
		return nil, sqlerrors.NewParseError(fmtstd.Sprintf("No expression was parsed from '%s'", sql))
	}
	if len(res) > 1 {
		return expressions.Block(expressions.Args{"expressions": res}), nil
	}
	return res[0], nil
}

func ParseInto(sql string, dialect dialects.DialectType, into expressions.Kind) (expressions.Expression, error) {
	return parseInto(sql, dialect, into, false)
}

func parseInto(sql string, dialect dialects.DialectType, into expressions.Kind, ignoreErrors bool) (expressions.Expression, error) {
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		return nil, err
	}
	toks, err := d.NewTokenizer().Tokenize(sql)
	if err != nil {
		return nil, err
	}
	level := sqlerrors.IMMEDIATE
	if ignoreErrors {
		level = sqlerrors.IGNORE
	}
	res, err := parser.NewWithErrorLevel(d, level).ParseInto(toks, sql, into)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 || res[0] == nil {
		return nil, sqlerrors.NewParseError(fmtstd.Sprintf("No expression was parsed from '%s'", sql))
	}
	if len(res) > 1 {
		return expressions.Block(expressions.Args{"expressions": res}), nil
	}
	return res[0], nil
}

// Generate renders e to SQL under dialect (see dialects.DialectType for the accepted forms).
func Generate(e expressions.Expression, dialect dialects.DialectType, opts generator.Options) (string, error) {
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		return "", err
	}
	return generator.New(d, opts).Generate(e)
}

// Transpile parses sql under the read dialect and re-renders each statement under the write
// dialect. read and write are each a dialects.DialectType (nil | string | *dialects.Dialect).
func Transpile(sql string, read dialects.DialectType, write dialects.DialectType, opts generator.Options) ([]string, error) {
	if strings.TrimSpace(sql) == "" {
		return []string{""}, nil
	}
	expressions, err := Parse(sql, read)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(expressions))
	for _, expression := range expressions {
		generated, err := Generate(expression, write, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, generated)
	}
	return out, nil
}

func init() {
	expressions.MaybeParseFunc = func(sql string, dialect expressions.DialectType) (expressions.Expression, error) {
		return ParseOne(sql, dialect)
	}
	expressions.ParseIntoFunc = func(sql string, dialect expressions.DialectType, into expressions.Kind, ignoreErrors bool) (expressions.Expression, error) {
		return parseInto(sql, dialect, into, ignoreErrors)
	}
	expressions.GenerateFunc = func(e expressions.Expression, opts expressions.GenerateOptions) (string, error) {
		return Generate(e, opts.Dialect, generator.Options{
			Pretty:             opts.Pretty,
			Identify:           opts.Identify,
			Normalize:          opts.Normalize,
			NormalizeFunctions: opts.NormalizeFunctions,
			LeadingComma:       opts.LeadingComma,
			MaxTextWidth:       opts.MaxTextWidth,
			Comments:           opts.Comments,
		})
	}
}
