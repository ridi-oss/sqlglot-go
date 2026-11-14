package optimizer

import (
	"strings"

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
	// DefaultSchemaCatalog, when set, is the catalog stamped on a table that received
	// DefaultSchema, while Catalog stays the catalog for tables that were already
	// schema-qualified. BigQuery needs both: `t` resolves to the job's defaultDataset
	// (its own project), `ds.t` to the job's project.
	DefaultSchemaCatalog exp.IdentifierName
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
	// EngineCatalog enables NON-UPSTREAM function/relation identity resolution (see
	// engine_catalog.go): with a non-nil catalog, CallReport/RelationReport (whichever are
	// non-nil) receive per-node classifications after the qualify passes run. Never rewrites
	// the AST. SearchPath doubles as the PG function search path.
	EngineCatalog  *EngineCatalog
	CallReport     map[exp.Expression]ResolvedCall
	RelationReport map[exp.Expression]ResolvedRelation
	SQL            string
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
	expression = qualifyTables(expression, opts.DefaultSchema, opts.Catalog, opts.DefaultSchemaCatalog, dialect, opts.CanonicalizeTableAliases, opts.OnQualify, opts.SearchPath, s)

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

	if opts.EngineCatalog != nil {
		ResolveEngineIdentities(expression, opts.EngineCatalog, dialect, opts.SearchPath, opts.CallReport, opts.RelationReport)
	}

	return expression
}

// markBigQueryTableNames keeps an unqualified BigQuery table name case-sensitive.
// divergence: upstream's normalize_identifier (dialects/bigquery.py:153-174) folds every
// unqualified name, treating it as a possible CTE; BigQuery table names are case-sensitive
// (DEVIATIONS §1.22). A name that matches a CTE visible from the table (case-insensitively) still folds.
func markBigQueryTableNames(expression exp.Expression) {
	for _, table := range expression.FindAll(exp.KindTable) {
		this := table.This()
		if this == nil || this.Kind() != exp.KindIdentifier || table.Arg("schema") != nil {
			continue
		}
		if !cteVisible(table, strings.ToLower(this.Name())) {
			this.Meta()["is_table"] = true
		}
	}
}

// cteVisible reports whether a WITH clause on an enclosing query defines name. Inside a CTE's
// own body only the earlier CTEs are visible, unless the WITH is RECURSIVE.
func cteVisible(node exp.Expression, name string) bool {
	var via exp.Expression
	for n := node.Parent(); n != nil; n = n.Parent() {
		if n.Kind() == exp.KindCTE {
			via = n
		}
		with := asExpression(n.Arg("with_"))
		if with == nil {
			continue
		}
		recursive := truthy(with.Arg("recursive"))
		for _, cte := range with.Expressions() {
			if cte == via && !recursive {
				break
			}
			if strings.ToLower(cte.AliasOrName()) == name {
				return true
			}
		}
		via = nil
	}
	return false
}
