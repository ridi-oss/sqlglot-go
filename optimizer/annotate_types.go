package optimizer

import (
	"github.com/ridi/sqlglot-go/dialects"
	"github.com/ridi/sqlglot-go/schema"
)

type TypeAnnotator struct {
	schema  schema.Schema
	dialect *dialects.Dialect
}

func NewTypeAnnotator(s schema.Schema) *TypeAnnotator {
	var d *dialects.Dialect
	if s != nil {
		d = s.Dialect()
	}
	if d == nil {
		d = dialects.Base()
	}
	return &TypeAnnotator{schema: s, dialect: d}
}

func (a *TypeAnnotator) AnnotateScope(scope *Scope) {
	// TODO(slice 4c): port full annotate_types.py:160-1051.
	// Deferred surface: _annotate_expression, EXPRESSION_METADATA,
	// BINARY_COERCIONS, _maybe_coerce, _get_setop_column_types, and
	// pivot column type inference. qualify_columns only reaches this method
	// when dialect.AnnotateAllScopes or dialect.SupportsStructStarExpansion is
	// enabled; both are false for the base/mysql/postgres slice 4b dialect path.
}
