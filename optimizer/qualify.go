package optimizer

import (
	exp "github.com/sjincho/sqlglot-go/expressions"
	"github.com/sjincho/sqlglot-go/schema"
)

type QualifyOpts struct {
	// Dialect is a DialectType-style value: nil (base), a string (bare name or the
	// "name, normalization_strategy=..." settings form), or a *dialects.Dialect. Mirrors
	// upstream sqlglot's polymorphic dialect argument; DB/Catalog/Schema are likewise any.
	Dialect any
	DB      any
	Catalog any
	// SearchPath is an ordered, opt-in database search path. Its zero value preserves
	// upstream-compatible fixed DB/Catalog qualification.
	SearchPath                []string
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
	// ResolutionReport receives source classifications; nil disables all report work.
	ResolutionReport map[exp.Expression]ResolvedSource
	SQL              string
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
	// opts.Dialect is a DialectType-style value (nil | string | *dialects.Dialect), threaded
	// through unchanged so a *Dialect instance reaches the schema (Dialect()) and the passes
	// that read dialect fields — mirroring upstream qualify.py:78, which resolves once and
	// passes the same instance down.
	dialect := opts.Dialect

	s, err := schema.EnsureSchema(opts.Schema, dialect, true)
	if err != nil {
		panic(err)
	}

	// TODO(slice 4c): store_original_column_identifiers needs Node meta.
	expression = NormalizeIdentifiers(expression, dialect)
	expression = QualifyTables(expression, opts.DB, opts.Catalog, dialect, opts.CanonicalizeTableAliases, opts.OnQualify, opts.SearchPath, s)

	if opts.IsolateTables {
		expression = IsolateTableSelects(expression, s, dialect)
	}

	if opts.QualifyColumns {
		expression = qualifyColumns(expression, s, opts.ExpandAliasRefs, opts.ExpandStars, opts.InferSchema, opts.AllowPartialQualification, dialect, opts.ResolutionReport)
	} else if opts.ResolutionReport != nil {
		populateResolutionReport(expression, opts.ResolutionReport)
	}

	if opts.QuoteIdentifiers {
		expression = QuoteIdentifiers(expression, dialect, opts.Identify)
	}

	if opts.ValidateQualifyColumns {
		ValidateQualifyColumns(expression, opts.SQL)
	}

	return expression
}
