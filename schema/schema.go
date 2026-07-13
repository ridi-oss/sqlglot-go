package schema

import (
	"fmt"
	"strings"

	"github.com/ridi/sqlglot-go/dialects"
	sqlerrors "github.com/ridi/sqlglot-go/errors"
	exp "github.com/ridi/sqlglot-go/expressions"
)

type Schema interface {
	AddTable(table any, columnMapping any, dialect string, normalize *bool, matchDepth bool) error
	ColumnNames(table any, onlyVisible bool, dialect string, normalize *bool) ([]string, error)
	GetColumnType(table any, column any, dialect string, normalize *bool) (exp.Expression, error)
	HasColumn(table any, column any, dialect string, normalize *bool) (bool, error)
	GetUDFType(udf any, dialect string, normalize *bool) (exp.Expression, error)
	SupportedTableArgs() []string
	Empty() bool
	Dialect() *dialects.Dialect
	Find(table exp.Expression, raiseOnMissing bool, ensureDataTypes bool) (*Mapping, error)
}

func EnsureSchema(s any, dialect any, normalize bool) (Schema, error) {
	if schema, ok := s.(Schema); ok {
		return schema, nil
	}
	mapping, err := asMapping(s)
	if err != nil {
		return nil, err
	}
	return NewMappingSchema(mapping, dialect, normalize)
}

func asMapping(s any) (*Mapping, error) {
	switch v := s.(type) {
	case nil:
		return NewMapping(), nil
	case *Mapping:
		return v, nil
	default:
		return nil, fmt.Errorf("Invalid schema mapping provided: %T", s)
	}
}

func ensureColumnMapping(mapping any) (*Mapping, error) {
	switch v := mapping.(type) {
	case nil:
		return NewMapping(), nil
	case *Mapping:
		return v, nil
	case string:
		out := NewMapping()
		if strings.TrimSpace(v) == "" {
			return out, nil
		}
		for _, item := range strings.Split(v, ",") {
			parts := strings.SplitN(item, ":", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("Invalid mapping provided: %T", mapping)
			}
			out.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
		return out, nil
	case []string:
		out := NewMapping()
		for _, item := range v {
			out.Set(strings.TrimSpace(item), nil)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("Invalid mapping provided: %T", mapping)
	}
}

func dictDepth(value any) int {
	mapping, ok := value.(*Mapping)
	if !ok || mapping == nil {
		return 0
	}
	if mapping.Len() == 0 {
		return 1
	}
	return 1 + dictDepth(mapping.first())
}

func flattenSchema(m *Mapping, depth int) [][]string {
	return flattenSchemaWithKeys(m, depth, nil)
}

func flattenSchemaWithKeys(m *Mapping, depth int, keys []string) [][]string {
	var tables [][]string
	if m == nil {
		return tables
	}
	for _, key := range m.Keys() {
		value, _ := m.Get(key)
		_, isMapping := value.(*Mapping)
		path := append(append([]string(nil), keys...), key)
		if depth == 1 || !isMapping {
			tables = append(tables, path)
		} else if depth >= 2 {
			tables = append(tables, flattenSchemaWithKeys(value.(*Mapping), depth-1, path)...)
		}
	}
	return tables
}

func nestedGet(d *Mapping, names, keys []string, raiseOnMissing bool) (any, error) {
	var result any = d
	for i, key := range keys {
		mapping, ok := result.(*Mapping)
		if !ok || mapping == nil {
			if raiseOnMissing {
				return nil, unknownPathError(names, keys, i)
			}
			return nil, nil
		}
		value, ok := mapping.Get(key)
		if !ok || value == nil {
			if raiseOnMissing {
				return nil, unknownPathError(names, keys, i)
			}
			return nil, nil
		}
		result = value
	}
	return result, nil
}

func unknownPathError(names, keys []string, index int) error {
	name := ""
	if index < len(names) {
		name = names[index]
	}
	if name == "this" {
		name = "table"
	}
	key := ""
	if index < len(keys) {
		key = keys[index]
	}
	return sqlerrors.NewSchemaError("Unknown %s: %s", name, key)
}

func nestedSet(d *Mapping, keys []string, value any) *Mapping {
	if d == nil {
		d = NewMapping()
	}
	if len(keys) == 0 {
		return d
	}
	if len(keys) == 1 {
		d.Set(keys[0], value)
		return d
	}
	subd := d
	for _, key := range keys[:len(keys)-1] {
		child, ok := subd.Get(key)
		childMapping, isMapping := child.(*Mapping)
		if !ok || !isMapping || childMapping == nil {
			childMapping = NewMapping()
			subd.Set(key, childMapping)
		}
		subd = childMapping
	}
	subd.Set(keys[len(keys)-1], value)
	return d
}

func first(m *Mapping) any {
	if m == nil {
		return nil
	}
	return m.first()
}

type findCacheKey struct {
	hash            uint64
	ensureDataTypes bool
}

type MappingSchema struct {
	mapping            *Mapping
	mappingTrie        *trieNode
	visible            *Mapping
	normalize          bool
	dialectName        string
	dialect            *dialects.Dialect
	supportedTableArgs []string
	depthCache         int
	typeCache          map[string]exp.Expression
	findCache          map[findCacheKey]*Mapping
}

func NewMappingSchema(schema *Mapping, dialect any, normalize bool) (*MappingSchema, error) {
	// dialect is a DialectType-style value (nil | string | *dialects.Dialect). A *Dialect is
	// stored verbatim (so Dialect() hands the caller's instance — with all its fields — back to
	// the optimizer, mirroring upstream ensure_schema), while dialectName keeps a canonical
	// string form so the string-threaded per-name normalization still re-resolves the same
	// normalization strategy.
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		return nil, err
	}
	dialectName, err := dialects.CanonicalString(dialect)
	if err != nil {
		return nil, err
	}
	if schema == nil {
		schema = NewMapping()
	}
	m := &MappingSchema{
		visible:     NewMapping(),
		normalize:   normalize,
		dialectName: dialectName,
		dialect:     d,
		typeCache:   map[string]exp.Expression{},
		findCache:   map[findCacheKey]*Mapping{},
	}
	if normalize {
		schema, err = m.normalizeMapping(schema)
		if err != nil {
			return nil, err
		}
	}
	m.mapping = schema
	m.mappingTrie = newTrie(reverseEach(flattenSchema(m.mapping, m.depth())), nil)
	return m, nil
}

func (m *MappingSchema) Dialect() *dialects.Dialect { return m.dialect }

func (m *MappingSchema) Empty() bool { return m == nil || m.mapping == nil || m.mapping.Len() == 0 }

func (m *MappingSchema) SupportedTableArgs() []string {
	args, _ := m.supportedTableArgsInternal()
	return args
}

func (m *MappingSchema) depth() int {
	if !m.Empty() && m.depthCache == 0 {
		// Python's AbstractMappingSchema.__init__ virtually dispatches to MappingSchema.depth()
		// (dict_depth(mapping)-1). Go embedding would build the trie one level too deep, so the
		// merged struct computes the effective depth directly.
		m.depthCache = dictDepth(m.mapping) - 1
	}
	return m.depthCache
}

func (m *MappingSchema) supportedTableArgsInternal() ([]string, error) {
	if len(m.supportedTableArgs) == 0 && !m.Empty() {
		depth := m.depth()
		if depth == 0 {
			m.supportedTableArgs = []string{}
		} else if 1 <= depth && depth <= 3 {
			m.supportedTableArgs = append([]string(nil), exp.TablePartKeys[:depth]...)
		} else {
			return nil, sqlerrors.NewSchemaError("Invalid mapping shape. Depth: %d", depth)
		}
	}
	return append([]string(nil), m.supportedTableArgs...), nil
}

func tableParts(table exp.Expression) []string {
	parts := table.Parts()
	out := make([]string, 0, len(parts))
	for i := len(parts) - 1; i >= 0; i-- {
		out = append(out, parts[i].Name())
	}
	return out
}

func findInTrie(parts []string, trie *trieNode, raiseOnMissing bool) ([]string, error) {
	value, subtrie := inTrie(trie, parts)
	if value == TrieFailed {
		return nil, nil
	}
	resolved := append([]string(nil), parts...)
	if value == TriePrefix {
		possibilities := subtrie.flatten()
		if len(possibilities) == 1 {
			resolved = append(resolved, possibilities[0]...)
		} else {
			if raiseOnMissing {
				joinedParts := strings.Join(parts, ".")
				messages := make([]string, 0, len(possibilities))
				for _, possibility := range possibilities {
					messages = append(messages, strings.Join(possibility, "."))
				}
				return nil, sqlerrors.NewSchemaError("Ambiguous mapping for %s: %s.", joinedParts, strings.Join(messages, ", "))
			}
			return nil, nil
		}
	}
	return resolved, nil
}

func (m *MappingSchema) Find(table exp.Expression, raiseOnMissing bool, ensureDataTypes bool) (*Mapping, error) {
	if table == nil {
		return nil, nil
	}
	cacheKey := findCacheKey{hash: table.HashKey(), ensureDataTypes: ensureDataTypes}
	if schema, ok := m.findCache[cacheKey]; ok && schema != nil {
		return schema, nil
	}
	supported, err := m.supportedTableArgsInternal()
	if err != nil {
		return nil, err
	}
	parts := tableParts(table)
	if len(parts) > len(supported) {
		parts = parts[:len(supported)]
	}
	resolvedParts, err := findInTrie(parts, m.mappingTrie, raiseOnMissing)
	if err != nil {
		return nil, err
	}
	if resolvedParts == nil {
		return nil, nil
	}
	value, err := nestedGet(m.mapping, supported, reverseStrings(resolvedParts), raiseOnMissing)
	if err != nil {
		return nil, err
	}
	schema, _ := value.(*Mapping)
	if ensureDataTypes && schema != nil {
		converted := NewMapping()
		for _, key := range schema.Keys() {
			value, _ := schema.Get(key)
			if s, ok := value.(string); ok {
				value, err = m.toDataType(s, "")
				if err != nil {
					return nil, err
				}
			}
			converted.Set(key, value)
		}
		schema = converted
	}
	m.findCache[cacheKey] = schema
	return schema, nil
}

func (m *MappingSchema) normalizeMapping(schema *Mapping) (*Mapping, error) {
	normalizedMapping := NewMapping()
	flattened := flattenSchema(schema, dictDepth(schema)-1)
	errorMsg := "Table %s must match the schema's nesting level: %d."
	for _, keys := range flattened {
		columnsValue, err := nestedGet(schema, keys, keys, true)
		if err != nil {
			return nil, err
		}
		columns, ok := columnsValue.(*Mapping)
		if !ok {
			return nil, sqlerrors.NewSchemaError(errorMsg, strings.Join(keys[:len(keys)-1], "."), len(flattened[0]))
		}
		if columns.Len() == 0 {
			return nil, sqlerrors.NewSchemaError("Table %s must have at least one column", strings.Join(keys[:len(keys)-1], "."))
		}
		if _, ok := first(columns).(*Mapping); ok {
			inner := flattenSchema(columns, dictDepth(columns)-1)
			innerKeys := []string{}
			if len(inner) > 0 {
				innerKeys = inner[0]
			}
			return nil, sqlerrors.NewSchemaError(errorMsg, strings.Join(append(append([]string(nil), keys...), innerKeys...), "."), len(flattened[0]))
		}
		normalizedKeys := make([]string, 0, len(keys))
		for _, key := range keys {
			normalized, err := m.normalizeName(key, "", true, nil)
			if err != nil {
				return nil, err
			}
			normalizedKeys = append(normalizedKeys, normalized)
		}
		for _, columnName := range columns.Keys() {
			columnType, _ := columns.Get(columnName)
			normalizedColumn, err := m.normalizeName(columnName, "", false, nil)
			if err != nil {
				return nil, err
			}
			path := append(append([]string(nil), normalizedKeys...), normalizedColumn)
			nestedSet(normalizedMapping, path, columnType)
		}
	}
	return normalizedMapping, nil
}

func (m *MappingSchema) normalizeTable(table any, dialect string, normalize *bool) (exp.Expression, error) {
	dialectName := dialect
	if dialectName == "" {
		dialectName = m.dialectName
	}
	normalizeFlag := m.normalize
	if normalize != nil {
		normalizeFlag = *normalize
	}
	normalizedTable, err := maybeParseInto(table, dialectName, exp.KindTable, normalizeFlag)
	if err != nil {
		return nil, err
	}
	if normalizeFlag {
		for _, part := range normalizedTable.Parts() {
			if part.Kind() == exp.KindIdentifier {
				normalized, err := normalizeName(part, dialectName, true, normalizeFlag)
				if err != nil {
					return nil, err
				}
				part.Replace(normalized)
			}
		}
	}
	return normalizedTable, nil
}

func (m *MappingSchema) normalizeName(name any, dialect string, isTable bool, normalize *bool) (string, error) {
	normalizeFlag := m.normalize
	if normalize != nil {
		normalizeFlag = *normalize
	}
	dialectName := dialect
	if dialectName == "" {
		dialectName = m.dialectName
	}
	identifier, err := normalizeName(name, dialectName, isTable, normalizeFlag)
	if err != nil {
		return "", err
	}
	return identifier.Name(), nil
}

func normalizeName(identifier any, dialect string, isTable bool, normalize bool) (exp.Expression, error) {
	var id exp.Expression
	switch v := identifier.(type) {
	case string:
		id = exp.ParseIdentifier(v, dialect)
	case exp.Expression:
		id = v
	default:
		return nil, fmt.Errorf("invalid identifier: %T", identifier)
	}
	if id == nil {
		return nil, fmt.Errorf("invalid identifier: %v", identifier)
	}
	if !normalize {
		return id, nil
	}
	// schema.py:704: identifier.meta["is_table"] = is_table, consulted by a dialect's
	// normalize_identifier (only BigQuery reads it today; inert for base/mysql/pg).
	id.Meta()["is_table"] = isTable
	d, err := dialects.GetOrRaise(dialect)
	if err != nil {
		return nil, err
	}
	return d.NormalizeIdentifier(id), nil
}

func (m *MappingSchema) AddTable(table any, columnMapping any, dialect string, normalize *bool, matchDepth bool) error {
	normalizedTable, err := m.normalizeTable(table, dialect, normalize)
	if err != nil {
		return err
	}
	if matchDepth && !m.Empty() && len(normalizedTable.Parts()) != m.depth() {
		tableSQL, sqlErr := normalizedTable.SQL(exp.GenerateOptions{Dialect: m.dialectName})
		if sqlErr != nil {
			tableSQL = normalizedTable.Name()
		}
		return sqlerrors.NewSchemaError("Table %s must match the schema's nesting level: %d.", tableSQL, m.depth())
	}
	columnMapping, err = ensureColumnMapping(columnMapping)
	if err != nil {
		return err
	}
	normalizedColumnMapping := NewMapping()
	for _, key := range columnMapping.(*Mapping).Keys() {
		value, _ := columnMapping.(*Mapping).Get(key)
		normalizedKey, err := m.normalizeName(key, dialect, false, normalize)
		if err != nil {
			return err
		}
		normalizedColumnMapping.Set(normalizedKey, value)
	}
	schema, err := m.Find(normalizedTable, false, false)
	if err != nil {
		return err
	}
	if schema != nil && schema.Len() > 0 && normalizedColumnMapping.Len() == 0 {
		return nil
	}
	parts := tableParts(normalizedTable)
	nestedSet(m.mapping, reverseStrings(parts), normalizedColumnMapping)
	newTrie([][]string{parts}, m.mappingTrie)
	m.findCache = map[findCacheKey]*Mapping{}
	m.depthCache = 0
	m.supportedTableArgs = nil
	return nil
}

func (m *MappingSchema) ColumnNames(table any, onlyVisible bool, dialect string, normalize *bool) ([]string, error) {
	normalizedTable, err := m.normalizeTable(table, dialect, normalize)
	if err != nil {
		return nil, err
	}
	schema, err := m.Find(normalizedTable, true, false)
	if err != nil {
		return nil, err
	}
	if schema == nil {
		return []string{}, nil
	}
	if !onlyVisible || m.visible == nil || m.visible.Len() == 0 {
		return schema.Keys(), nil
	}
	visibleValue, err := nestedGet(m.visible, m.SupportedTableArgs(), reverseStrings(tableParts(normalizedTable)), true)
	if err != nil {
		return nil, err
	}
	visibleSet := map[string]bool{}
	if visible, ok := visibleValue.([]string); ok {
		for _, col := range visible {
			visibleSet[col] = true
		}
	}
	out := []string{}
	for _, col := range schema.Keys() {
		if visibleSet[col] {
			out = append(out, col)
		}
	}
	return out, nil
}

func (m *MappingSchema) GetColumnType(table any, column any, dialect string, normalize *bool) (exp.Expression, error) {
	normalizedTable, err := m.normalizeTable(table, dialect, normalize)
	if err != nil {
		return nil, err
	}
	columnName := column
	if columnExpr, ok := column.(exp.Expression); ok {
		if columnExpr.Kind() == exp.KindColumn {
			columnName = columnExpr.Arg("this")
		} else {
			columnName = columnExpr
		}
	}
	normalizedColumnName, err := m.normalizeName(columnName, dialect, false, normalize)
	if err != nil {
		return nil, err
	}
	tableSchema, err := m.Find(normalizedTable, false, false)
	if err != nil {
		return nil, err
	}
	if tableSchema != nil {
		columnType, _ := tableSchema.Get(normalizedColumnName)
		if dataType, ok := columnType.(exp.Expression); ok {
			return dataType, nil
		}
		if schemaType, ok := columnType.(string); ok {
			return m.toDataType(schemaType, dialect)
		}
	}
	return exp.DTypeUnknown.IntoExpr(nil), nil
}

func (m *MappingSchema) HasColumn(table any, column any, dialect string, normalize *bool) (bool, error) {
	normalizedTable, err := m.normalizeTable(table, dialect, normalize)
	if err != nil {
		return false, err
	}
	columnName := column
	if columnExpr, ok := column.(exp.Expression); ok {
		if columnExpr.Kind() == exp.KindColumn {
			columnName = columnExpr.Arg("this")
		} else {
			columnName = columnExpr
		}
	}
	normalizedColumnName, err := m.normalizeName(columnName, dialect, false, normalize)
	if err != nil {
		return false, err
	}
	tableSchema, err := m.Find(normalizedTable, false, false)
	if err != nil {
		return false, err
	}
	if tableSchema == nil {
		return false, nil
	}
	_, ok := tableSchema.Get(normalizedColumnName)
	return ok, nil
}

func (m *MappingSchema) GetUDFType(udf any, dialect string, normalize *bool) (exp.Expression, error) {
	// TODO(slice 5): port udf_mapping/udf_trie/find_udf/get_udf_type/_normalize_udf(s).
	return exp.DTypeUnknown.IntoExpr(nil), nil
}

func (m *MappingSchema) toDataType(schemaType, dialect string) (exp.Expression, error) {
	if cached := m.typeCache[schemaType]; cached != nil {
		return cached, nil
	}
	d := m.dialect
	dialectName := m.dialectName
	if dialect != "" {
		resolved, err := dialects.GetOrRaise(dialect)
		if err != nil {
			return nil, err
		}
		d = resolved
		dialectName = dialect
	}
	udt := d.SupportsUserDefinedTypes
	expr, err := exp.DataTypeBuild(schemaType, dialectName, udt, true, nil)
	if err != nil {
		inDialect := ""
		if dialectName != "" {
			inDialect = fmt.Sprintf(" in dialect %s", dialectName)
		}
		return nil, sqlerrors.NewSchemaError("Failed to build type '%s'%s.", schemaType, inDialect)
	}
	for _, id := range expr.FindAll(exp.KindIdentifier) {
		d.NormalizeIdentifier(id)
	}
	m.typeCache[schemaType] = expr
	return expr, nil
}

func maybeParseInto(sqlOrExpr any, dialect string, into exp.Kind, copyValue bool) (exp.Expression, error) {
	if expr, ok := sqlOrExpr.(exp.Expression); ok {
		if copyValue {
			return expr.Copy(), nil
		}
		return expr, nil
	}
	if exp.ParseIntoFunc == nil {
		return nil, fmt.Errorf("expressions.ParseIntoFunc is not configured")
	}
	return exp.ParseIntoFunc(fmt.Sprint(sqlOrExpr), dialect, into, false)
}

func reverseEach(in [][]string) [][]string {
	out := make([][]string, 0, len(in))
	for _, item := range in {
		out = append(out, reverseStrings(item))
	}
	return out
}

func reverseStrings(in []string) []string {
	out := make([]string, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}
