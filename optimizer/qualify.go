package optimizer

import (
	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/schema"
)

type QualifyOpts struct {
	Dialect                   string
	DB                        any
	Catalog                   any
	Schema                    any
	ExpandAliasRefs           bool
	ExpandStars               bool
	InferSchema               *bool
	IsolateTables             bool
	QualifyColumns            bool
	AllowPartialQualification bool
	ValidateQualifyColumns    bool
	QuoteIdentifiers          bool
	Identify                  bool
	CanonicalizeTableAliases  bool
	OnQualify                 func(exp.Expression)
	SQL                       string
}

func DefaultQualifyOpts() QualifyOpts {
	return QualifyOpts{
		ExpandAliasRefs:        true,
		ExpandStars:            true,
		QualifyColumns:         true,
		ValidateQualifyColumns: true,
		QuoteIdentifiers:       true,
		Identify:               true,
	}
}

func Qualify(expression exp.Expression, opts QualifyOpts) exp.Expression {
	s, err := schema.EnsureSchema(opts.Schema, opts.Dialect, true)
	if err != nil {
		panic(err)
	}

	// TODO(slice 4c): store_original_column_identifiers needs Node meta.
	expression = NormalizeIdentifiers(expression, opts.Dialect)
	expression = QualifyTables(expression, opts.DB, opts.Catalog, opts.Dialect, opts.CanonicalizeTableAliases, opts.OnQualify)

	if opts.IsolateTables {
		expression = IsolateTableSelects(expression, s, opts.Dialect)
	}

	if opts.QualifyColumns {
		expression = QualifyColumns(expression, s, opts.ExpandAliasRefs, opts.ExpandStars, opts.InferSchema, opts.AllowPartialQualification, opts.Dialect)
	}

	if opts.QuoteIdentifiers {
		expression = QuoteIdentifiers(expression, opts.Dialect, opts.Identify)
	}

	if opts.ValidateQualifyColumns {
		ValidateQualifyColumns(expression, opts.SQL)
	}

	return expression
}
