package optimizer

import (
	"github.com/ridi-oss/sqlglot-go/dialects"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/schema"
)

type QualifyOpts struct {
	// Dialect selects the dialect. See dialects.DialectType for the accepted forms
	// (nil | string | *dialects.Dialect). Mirrors upstream sqlglot's polymorphic dialect argument.
	Dialect dialects.DialectType
	// DefaultSchema is the fixed schema-level qualifier stamped on unqualified tables — upstream's
	// `db=` kwarg (renamed here for the same reason as the Table/Column `schema` arg; see
	// DEVIATIONS.md §7). Named DefaultSchema, not Schema, because Schema below is the column-metadata
	// mapping (upstream's `schema=`), mirroring upstream's own two distinct qualify() arguments.
	// A name-or-identifier value; see expressions.IdentifierName.
	DefaultSchema exp.IdentifierName
	// Catalog is the fixed catalog-level qualifier; a name-or-identifier, see expressions.IdentifierName.
	Catalog exp.IdentifierName
	// SearchPath is an ordered, opt-in schema search path. Its zero value preserves
	// upstream-compatible fixed DefaultSchema/Catalog qualification.
	SearchPath []string
	// Schema is the column-metadata schema. See schema.SchemaType for the accepted forms.
	Schema                    schema.SchemaType
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
	expression = QualifyTables(expression, opts.DefaultSchema, opts.Catalog, dialect, opts.CanonicalizeTableAliases, opts.OnQualify, opts.SearchPath, s)

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
