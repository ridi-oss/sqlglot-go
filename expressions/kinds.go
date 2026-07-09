package expressions

type Kind int

const (
	KindExpression Kind = iota
	KindColumn
	KindLiteral
	KindIdentifier
	KindStar
	KindAlias
	KindAliases
	KindDot
	KindNull
	KindBoolean
	KindVar
	KindParen
	KindNeg
	KindNot
	KindAdd
	KindSub
	KindMul
	KindDiv
	KindMod
	KindEQ
	KindNEQ
	KindNullSafeEQ
	KindGT
	KindGTE
	KindLT
	KindLTE
	KindAnd
	KindOr
	KindBitwiseAnd
	KindBitwiseOr
	KindBitwiseXor
	KindDPipe
	KindBetween
	KindIs
	KindLike
	KindILike
	// KindSimilarTo/KindEscape port exp.SimilarTo/exp.Escape (expressions/core.py:2226,2162):
	// Postgres `x SIMILAR TO y [ESCAPE z]`, a Binary/Predicate pair like Like/ILike above.
	KindSimilarTo
	KindEscape
	KindIn
	KindSelect
	KindFrom
	KindJoin
	KindTable
	KindTableAlias
	KindWhere
	KindGroup
	KindOrder
	KindLimit
	KindOffset
	KindHint
	KindBlock
	KindPlaceholder
	KindTuple
	KindWith
	KindCTE
	KindRecursiveWithSearch
	KindUnion
	KindExcept
	KindIntersect
	KindSubquery
	KindHaving
	KindQualify
	KindCube
	KindRollup
	KindGroupingSets
	KindOrdered
	KindDistinct
	KindWindow
	KindWindowSpec
	KindFilter
	KindLimitOptions
	KindFetch
	KindCase
	KindIf
	KindExists
	KindAny
	KindAll
	KindNullSafeNEQ
	KindAnonymous
	KindAbs
	KindAvg
	KindSum
	KindSqrt
	KindLn
	KindExp
	KindMin
	KindMax
	KindRound
	KindLog
	KindPow
	KindStddev
	KindStddevPop
	KindStddevSamp
	KindVariance
	KindVariancePop
	KindDay
	KindMonth
	KindYear
	KindQuarter
	KindApproxDistinct
	KindHll
	KindCountIf
	KindQuantile
	KindCount
	KindCoalesce
	KindGreatest
	KindLeast
	KindInsert
	KindUpdate
	KindDelete
	KindMerge
	KindWhen
	KindWhens
	KindOnConflict
	KindReturning
	KindInto
	KindCreate
	KindSchema
	KindCommand
	KindPivot
	KindLateral
	KindValues
	KindColumnDef
	KindDataType
	KindDataTypeParam
	KindCast
	KindTryCast
	KindCastToStrType
	KindExtract
	KindStrPosition
	KindSubstring
	KindTrim
	KindCeil
	KindFloor
	KindGroupConcat
	KindUnnest
	KindArray
	KindBracket
	KindLock
	KindPreWhere
	KindCluster
	KindDistribute
	KindSort
	KindWithinGroup
	KindIgnoreNulls
	KindRespectNulls
	KindPivotAny
	KindPivotAlias
	KindInterval
	KindIntervalSpan
	KindJSONExtract
	KindJSONExtractScalar
	KindJSONBExtract
	KindJSONBExtractScalar
	KindJSONCast
	// KindJSONTable/KindJSONColumnDef/KindJSONSchema/KindFormatJson port the JSON_TABLE
	// family from expressions/query.py:2022,2030,2041 (JSONTable itself lives in
	// expressions/json.py:206).
	KindJSONTable
	KindJSONColumnDef
	KindJSONSchema
	KindFormatJson
	KindArrayAgg
	KindArraySize
	KindArrayContains
	KindInitcap
	KindSplit
	KindRegexpLike
	KindRegexpSplit
	KindStructExtract
	KindStandardHash
	KindHex
	KindMD5
	KindStPoint
	KindStDistance
	KindGenerateSeries
	KindDate
	KindAddMonths
	KindDateAdd
	KindDateDiff
	KindJoinHint
	KindTableColumn
	KindFinal
	// The 19 Kinds below close the statement-level parse gaps (SET/SHOW/USE/DESCRIBE/
	// EXPLAIN/BEGIN/COMMIT/ROLLBACK/KILL/LOAD DATA/ANALYZE/GRANT/REVOKE/COMMENT ON/
	// TRUNCATE, plus PRAGMA): ddl.py:104,121,157,173,181,191,306,385,389,393,426;
	// query.py:589,609,619,1879; properties.py:16,20; dml.py:281. All are plain
	// Expression subclasses (no Func/Condition/... mixins), so none get traitsOf rows.
	KindSet
	KindSetItem
	KindShow
	KindUse
	KindKill
	KindDescribe
	KindLoadData
	KindTransaction
	KindCommit
	KindRollback
	KindGrant
	KindRevoke
	KindGrantPrivilege
	KindGrantPrincipal
	KindComment
	KindTruncateTable
	KindPartition
	KindAnalyze
	KindPragma
	// Supporting expression/property nodes ported alongside the statement families to
	// close their remaining round-trip gaps: Parameter (mysql `@var` user variables,
	// core.py:1833), RawString (postgres `$$...$$` / raw string literals, query.py:490),
	// and FileFormatProperty (the `FORMAT=<fmt>` option on DESCRIBE, properties.py:176).
	KindParameter
	KindRawString
	KindFileFormatProperty
	// The DDL constraint/alter/drop node family below closes the CREATE-table constraint,
	// ALTER TABLE/VIEW/INDEX, and DROP round-trip gaps: expressions/constraints.py (column
	// constraints, PRIMARY KEY, FOREIGN KEY, indexes) and expressions/ddl.py (ALTER/DROP
	// statements), plus ColumnPosition/AddPartition/DropPartition from expressions/query.py
	// (:498,1941,1949). ColumnConstraintKind (core.py:1605) is itself a plain-Expression
	// marker mixin, so none of these get traitsOf rows.
	KindColumnConstraint
	KindConstraint
	KindPrimaryKey
	KindPrimaryKeyColumnConstraint
	KindForeignKey
	KindReference
	KindNotNullColumnConstraint
	KindUniqueColumnConstraint
	KindCheckColumnConstraint
	KindDefaultColumnConstraint
	KindCollateColumnConstraint
	KindCommentColumnConstraint
	KindCharacterSetColumnConstraint
	KindAutoIncrementColumnConstraint
	KindOnUpdateColumnConstraint
	KindGeneratedAsIdentityColumnConstraint
	KindGeneratedAsRowColumnConstraint
	KindComputedColumnConstraint
	KindCaseSpecificColumnConstraint
	KindNotForReplicationColumnConstraint
	KindZeroFillColumnConstraint
	KindInvisibleColumnConstraint
	KindIndexColumnConstraint
	KindIndexConstraintOption
	KindIndexParameters
	KindColumnPrefix
	KindColumnPosition
	KindAlter
	KindDrop
	KindAlterColumn
	KindModifyColumn
	KindAlterIndex
	KindRenameColumn
	KindRenameIndex
	KindAlterRename
	KindAlterSet
	KindDropPrimaryKey
	KindAddConstraint
	KindDropPartition
	KindAddPartition
	// KindLambda ports exp.Lambda (query.py:646): a plain Expression (no Func/Condition
	// mixin, mirroring e.g. KindTableAlias), so it gets no traitsOf row below.
	KindLambda
	// KindReplace ports exp.Replace (string.py:113), a Func/Condition subclass with no
	// dedicated generator method upstream: base has no replace_sql, so functionFallbackSQL
	// renders it (mirroring e.g. KindInitcap).
	KindReplace
	// KindLogicalOr ports exp.LogicalOr (aggregate.py:171-172): LOGICAL_OR/BOOL_OR/
	// BOOLOR_AGG all parse to it; postgres/mysql rename it on generation (see
	// generator/aggregate.go logicalOrSQL).
	KindLogicalOr
	// KindAtTimeZone/KindPseudoType/KindObjectIdentifier close the TYPE/CAST/`::`/
	// AT TIME ZONE round-trip parity gap: exp.AtTimeZone (core.py:2267, a plain
	// Expression - `x AT TIME ZONE zone`, parsed by _parse_at_time_zone), and
	// exp.PseudoType/exp.ObjectIdentifier (datatypes.py:439,443 - both DataType
	// subclasses with arg_types={"this": True}, but "this" holds the raw uppercased
	// token text rather than a DType, so they get their own Kinds/dispatch rather than
	// reusing KindDataType). None of the three have a Condition/Predicate/... mixin
	// upstream, so none get a traitsOf row below.
	KindAtTimeZone
	KindPseudoType
	KindObjectIdentifier
	// KindStruct is exp.Struct (array.py:355, `class Struct(Expression, Func)`,
	// is_var_len_args=True): the constructor produced by _parse_types' inline
	// values-suffix, e.g. STRUCT<a INT, b STRING>(1, 'foo') -> CAST(STRUCT(1, 'foo')
	// AS STRUCT<a INT, b TEXT>). Renders through the generic TraitFunc fallback
	// (STRUCT(...)), matching upstream struct_sql for non-named-field constructors.
	KindStruct
	// KindJSONObject/KindJSONObjectAgg/KindJSONKeyValue/KindOnCondition/KindJSONValue port
	// the JSON_OBJECT/JSON_OBJECTAGG/JSON_VALUE function family: exp.JSONObject/
	// exp.JSONObjectAgg (expressions/json.py:160-177, both Func subclasses - JSONObjectAgg is
	// also an AggFunc), exp.JSONKeyValue/exp.OnCondition/exp.JSONValue (expressions/query.py:
	// 2026,577,2045 - all three are plain Expression subclasses with no Func/Condition mixin,
	// so none of them get a traitsOf row below; JSONObject/JSONObjectAgg do).
	KindJSONObject
	KindJSONObjectAgg
	KindJSONKeyValue
	KindOnCondition
	KindJSONValue
	// KindChr ports exp.Chr (string.py:23-26, `class Chr(Expression, Func)`,
	// arg_types={"expressions": True, "charset": False}, is_var_len_args=True,
	// _sql_names=["CHR", "CHAR"]): CHR(...)/CHAR(...) with an optional trailing
	// `USING <charset>` clause. Parsed via functionParsers (parseChar), not
	// FromArgList, since the trailing USING clause needs special grammar.
	KindChr
	// The six Kinds below port the STR_TO_*/TIME_STR_TO_* temporal family (expressions/
	// temporal.py:452-473): all are `class X(Expression, Func)`, so each gets a
	// TraitCondition|TraitFunc row below (none is a bare `pass` Expression). TimeStrToDate
	// and TimeStrToUnix are themselves `pass` bodies (temporal.py:464,472), i.e. they inherit
	// arg_types={"this": True} from Expression (core.py:85) rather than declaring their own
	// dict, unlike their four siblings.
	KindStrToDate
	KindStrToTime
	KindStrToUnix
	KindTimeStrToDate
	KindTimeStrToTime
	KindTimeStrToUnix
	// KindXMLElement/KindXMLTable port exp.XMLElement/exp.XMLTable (Expression, Func)
	// (functions.py:439,453). KindXMLNamespace ports exp.XMLNamespace (query.py:2081-2082),
	// a plain Expression (no Func/Condition mixin), so it gets no traitsOf row below
	// (mirroring e.g. KindTableAlias/KindLambda).
	KindXMLElement
	KindXMLTable
	KindXMLNamespace
	// KindPathColumnConstraint ports exp.PathColumnConstraint (constraints.py:185,
	// `class PathColumnConstraint(Expression, ColumnConstraintKind): pass`), the `PATH '<p>'`
	// column constraint XMLTABLE's COLUMNS clause uses. Like upstream (parser.py:1391), "PATH"
	// is registered in the single shared constraintParsers map (parser/parser_constraints.go),
	// so it is reached generically through parseColumnConstraint's loop and composes with any
	// following constraints. Like ColumnConstraintKind's other `pass` subclasses (e.g.
	// KindCollateColumnConstraint), it has no Func/Condition mixin, so it gets no traitsOf
	// row below.
	KindPathColumnConstraint
)

type Trait uint32

const (
	TraitCondition Trait = 1 << iota
	TraitPredicate
	TraitBinary
	TraitConnector
	TraitFunc
	TraitAggFunc
	TraitUnary
	TraitQuery
	TraitDDL
	TraitDML
	TraitUDTF
	TraitDerivedTable
	TraitSetOperation
)

type argSpec struct {
	Key      string
	Required bool
}

var defaultArgTypes = []argSpec{{Key: "this", Required: true}}

var queryModifierArgTypes = []argSpec{
	{"match", false},
	{"laterals", false},
	{"joins", false},
	{"connect", false},
	{"pivots", false},
	{"prewhere", false},
	{"where", false},
	{"group", false},
	{"having", false},
	{"qualify", false},
	{"windows", false},
	{"distribute", false},
	{"sort", false},
	{"cluster", false},
	{"order", false},
	{"limit", false},
	{"offset", false},
	{"locks", false},
	{"sample", false},
	{"settings", false},
	{"format", false},
	{"options", false},
	{"for_", false},
}

func withQueryModifiers(prefix ...argSpec) []argSpec {
	out := append([]argSpec{}, prefix...)
	return append(out, queryModifierArgTypes...)
}

var argTypes = map[Kind][]argSpec{
	KindExpression: defaultArgTypes,
	KindColumn:     {{"this", true}, {"table", false}, {"db", false}, {"catalog", false}, {"join_mark", false}},
	KindLiteral:    {{"this", true}, {"is_string", true}},
	KindIdentifier: {{"this", true}, {"quoted", false}, {"global_", false}, {"temporary", false}},
	KindStar:       {{"except_", false}, {"replace", false}, {"rename", false}, {"ilike", false}},
	KindAlias:      {{"this", true}, {"alias", false}},
	KindAliases:    {{"this", true}, {"expressions", true}},
	KindDot:        {{"this", true}, {"expression", true}},
	KindNull:       {},
	KindBoolean:    defaultArgTypes,
	KindVar:        defaultArgTypes,
	KindParen:      defaultArgTypes,
	KindNeg:        defaultArgTypes,
	KindNot:        defaultArgTypes,
	KindAdd:        {{"this", true}, {"expression", true}},
	KindSub:        {{"this", true}, {"expression", true}},
	KindMul:        {{"this", true}, {"expression", true}},
	KindDiv:        {{"this", true}, {"expression", true}, {"typed", false}, {"safe", false}},
	KindMod:        {{"this", true}, {"expression", true}},
	KindEQ:         {{"this", true}, {"expression", true}},
	KindNEQ:        {{"this", true}, {"expression", true}},
	KindNullSafeEQ: {{"this", true}, {"expression", true}},
	KindGT:         {{"this", true}, {"expression", true}},
	KindGTE:        {{"this", true}, {"expression", true}},
	KindLT:         {{"this", true}, {"expression", true}},
	KindLTE:        {{"this", true}, {"expression", true}},
	KindAnd:        {{"this", true}, {"expression", true}},
	KindOr:         {{"this", true}, {"expression", true}},
	KindBitwiseAnd: {{"this", true}, {"expression", true}, {"padside", false}},
	KindBitwiseOr:  {{"this", true}, {"expression", true}, {"padside", false}},
	KindBitwiseXor: {{"this", true}, {"expression", true}, {"padside", false}},
	KindDPipe:      {{"this", true}, {"expression", true}, {"safe", false}},
	KindBetween:    {{"this", true}, {"low", true}, {"high", true}, {"symmetric", false}},
	KindIs:         {{"this", true}, {"expression", true}},
	KindLike:       {{"this", true}, {"expression", true}, {"negate", false}},
	KindILike:      {{"this", true}, {"expression", true}, {"negate", false}},
	// SimilarTo/Escape mirror Binary's default arg_types (parser.py:1623-1624): no "negate"
	// arg (unlike Like/ILike), so NOT ... is wrapped in exp.Not by parseRange.
	KindSimilarTo:           {{"this", true}, {"expression", true}},
	KindEscape:              {{"this", true}, {"expression", true}},
	KindIn:                  {{"this", true}, {"expressions", false}, {"query", false}, {"unnest", false}, {"field", false}, {"is_global", false}},
	KindSelect:              withQueryModifiers(argSpec{"with_", false}, argSpec{"kind", false}, argSpec{"expressions", false}, argSpec{"hint", false}, argSpec{"distinct", false}, argSpec{"into", false}, argSpec{"from_", false}, argSpec{"operation_modifiers", false}, argSpec{"exclude", false}),
	KindFrom:                defaultArgTypes,
	KindJoin:                {{"this", true}, {"on", false}, {"side", false}, {"kind", false}, {"using", false}, {"method", false}, {"global_", false}, {"hint", false}, {"match_condition", false}, {"directed", false}, {"expressions", false}, {"pivots", false}},
	KindTable:               {{"this", false}, {"alias", false}, {"db", false}, {"catalog", false}, {"laterals", false}, {"joins", false}, {"pivots", false}, {"hints", false}, {"system_time", false}, {"version", false}, {"format", false}, {"pattern", false}, {"ordinality", false}, {"when", false}, {"only", false}, {"partition", false}, {"changes", false}, {"rows_from", false}, {"sample", false}, {"indexed", false}},
	KindTableAlias:          {{"this", false}, {"columns", false}},
	KindWhere:               defaultArgTypes,
	KindGroup:               {{"expressions", false}, {"grouping_sets", false}, {"cube", false}, {"rollup", false}, {"totals", false}, {"all", false}},
	KindOrder:               {{"this", false}, {"expressions", true}, {"siblings", false}},
	KindLimit:               {{"this", false}, {"expression", true}, {"offset", false}, {"limit_options", false}, {"expressions", false}},
	KindOffset:              {{"this", false}, {"expression", true}, {"expressions", false}},
	KindHint:                {{"expressions", true}},
	KindBlock:               {{"expressions", true}},
	KindPlaceholder:         {{"this", false}, {"kind", false}, {"widget", false}, {"jdbc", false}},
	KindTuple:               {{"expressions", false}},
	KindWith:                {{"expressions", true}, {"recursive", false}, {"search", false}},
	KindCTE:                 {{"this", true}, {"alias", true}, {"scalar", false}, {"materialized", false}, {"key_expressions", false}},
	KindRecursiveWithSearch: {{"kind", true}, {"this", true}, {"expression", true}, {"using", false}},
	KindUnion:               withQueryModifiers(argSpec{"with_", false}, argSpec{"this", true}, argSpec{"expression", true}, argSpec{"distinct", false}, argSpec{"by_name", false}, argSpec{"side", false}, argSpec{"kind", false}, argSpec{"on", false}),
	KindExcept:              withQueryModifiers(argSpec{"with_", false}, argSpec{"this", true}, argSpec{"expression", true}, argSpec{"distinct", false}, argSpec{"by_name", false}, argSpec{"side", false}, argSpec{"kind", false}, argSpec{"on", false}),
	KindIntersect:           withQueryModifiers(argSpec{"with_", false}, argSpec{"this", true}, argSpec{"expression", true}, argSpec{"distinct", false}, argSpec{"by_name", false}, argSpec{"side", false}, argSpec{"kind", false}, argSpec{"on", false}),
	KindSubquery:            withQueryModifiers(argSpec{"this", true}, argSpec{"alias", false}, argSpec{"with_", false}),
	KindHaving:              {{"this", true}},
	KindQualify:             {{"this", true}},
	KindCube:                {{"expressions", false}},
	KindRollup:              {{"expressions", false}},
	KindGroupingSets:        {{"expressions", true}},
	KindOrdered:             {{"this", true}, {"desc", false}, {"nulls_first", true}, {"with_fill", false}},
	KindDistinct:            {{"expressions", false}, {"on", false}},
	KindWindow:              {{"this", true}, {"partition_by", false}, {"order", false}, {"spec", false}, {"alias", false}, {"over", false}, {"first", false}},
	KindWindowSpec:          {{"kind", false}, {"start", false}, {"start_side", false}, {"end", false}, {"end_side", false}, {"exclude", false}},
	KindFilter:              {{"this", true}, {"expression", true}},
	KindLimitOptions:        {{"percent", false}, {"rows", false}, {"with_ties", false}},
	KindFetch:               {{"direction", false}, {"count", false}, {"limit_options", false}},
	KindCase:                {{"this", false}, {"ifs", true}, {"default", false}},
	KindIf:                  {{"this", true}, {"true", true}, {"false", false}},
	KindExists:              {{"this", true}, {"expression", false}},
	KindAny:                 {{"this", true}},
	KindAll:                 {{"this", true}},
	KindNullSafeNEQ:         {{"this", true}, {"expression", true}},
	KindAnonymous:           {{"this", true}, {"expressions", false}},
	KindAbs:                 defaultArgTypes,
	KindAvg:                 defaultArgTypes,
	KindSum:                 defaultArgTypes,
	KindSqrt:                defaultArgTypes,
	KindLn:                  defaultArgTypes,
	KindExp:                 defaultArgTypes,
	KindMin:                 {{"this", true}, {"expressions", false}},
	KindMax:                 {{"this", true}, {"expressions", false}},
	KindRound:               {{"this", true}, {"decimals", false}, {"truncate", false}, {"casts_non_integer_decimals", false}},
	KindLog:                 {{"this", true}, {"expression", false}},
	KindPow:                 {{"this", true}, {"expression", true}},
	KindStddev:              defaultArgTypes,
	KindStddevPop:           defaultArgTypes,
	KindStddevSamp:          defaultArgTypes,
	KindVariance:            defaultArgTypes,
	KindVariancePop:         defaultArgTypes,
	KindDay:                 defaultArgTypes,
	KindMonth:               defaultArgTypes,
	KindYear:                defaultArgTypes,
	KindQuarter:             defaultArgTypes,
	KindApproxDistinct:      {{"this", true}, {"accuracy", false}},
	KindHll:                 {{"this", true}, {"expressions", false}},
	KindCountIf:             defaultArgTypes,
	KindQuantile:            {{"this", true}, {"quantile", true}},
	KindCount:               {{"this", false}, {"expressions", false}, {"big_int", false}},
	KindCoalesce:            {{"this", true}, {"expressions", false}, {"is_nvl", false}, {"is_null", false}},
	KindGreatest:            {{"this", true}, {"expressions", false}, {"ignore_nulls", true}},
	KindLeast:               {{"this", true}, {"expressions", false}, {"ignore_nulls", true}},
	KindInsert:              {{"hint", false}, {"with_", false}, {"is_function", false}, {"this", false}, {"expression", false}, {"conflict", false}, {"returning", false}, {"overwrite", false}, {"exists", false}, {"alternative", false}, {"where", false}, {"ignore", false}, {"by_name", false}, {"stored", false}, {"partition", false}, {"settings", false}, {"source", false}, {"default", false}},
	KindUpdate:              {{"with_", false}, {"this", false}, {"expressions", false}, {"from_", false}, {"where", false}, {"returning", false}, {"order", false}, {"limit", false}, {"options", false}, {"hint", false}},
	KindDelete:              {{"with_", false}, {"this", false}, {"using", false}, {"where", false}, {"returning", false}, {"order", false}, {"limit", false}, {"tables", false}, {"cluster", false}, {"hint", false}},
	KindMerge:               {{"this", true}, {"using", true}, {"on", false}, {"using_cond", false}, {"whens", true}, {"with_", false}, {"returning", false}},
	KindWhen:                {{"matched", true}, {"source", false}, {"condition", false}, {"then", true}},
	KindWhens:               {{"expressions", true}},
	KindOnConflict:          {{"duplicate", false}, {"expressions", false}, {"action", false}, {"conflict_keys", false}, {"index_predicate", false}, {"constraint", false}, {"where", false}},
	KindReturning:           {{"expressions", true}, {"into", false}},
	KindInto:                {{"this", false}, {"temporary", false}, {"unlogged", false}, {"bulk_collect", false}, {"expressions", false}},
	KindCreate:              {{"with_", false}, {"this", true}, {"kind", true}, {"expression", false}, {"exists", false}, {"properties", false}, {"replace", false}, {"refresh", false}, {"unique", false}, {"indexes", false}, {"no_schema_binding", false}, {"begin", false}, {"clone", false}, {"concurrently", false}, {"clustered", false}},
	KindSchema:              {{"this", false}, {"expressions", false}},
	KindCommand:             {{"this", true}, {"expression", false}},
	KindPivot:               {{"this", false}, {"alias", false}, {"expressions", false}, {"fields", false}, {"unpivot", false}, {"using", false}, {"group", false}, {"columns", false}, {"include_nulls", false}, {"default_on_null", false}, {"into", false}, {"with_", false}, {"identify_pivot_strings", false}, {"prefixed_pivot_columns", false}, {"pivot_column_naming", false}},
	KindLateral:             {{"this", true}, {"view", false}, {"outer", false}, {"alias", false}, {"cross_apply", false}, {"ordinality", false}},
	KindValues:              {{"expressions", true}, {"alias", false}, {"order", false}, {"limit", false}, {"offset", false}},
	KindColumnDef:           {{"this", true}, {"kind", false}, {"constraints", false}, {"exists", false}, {"position", false}, {"default", false}, {"output", false}, {"ordinality", false}},
	KindDataType:            {{"this", true}, {"expressions", false}, {"nested", false}, {"values", false}, {"kind", false}, {"nullable", false}, {"collate", false}},
	KindDataTypeParam:       {{"this", true}, {"expression", false}},
	KindCast:                {{"this", true}, {"to", true}, {"format", false}, {"safe", false}, {"action", false}, {"default", false}},
	KindTryCast:             {{"this", true}, {"to", true}, {"format", false}, {"safe", false}, {"action", false}, {"default", false}, {"requires_string", false}, {"null_on_text_overflow", false}, {"probe_date_format", false}},
	KindCastToStrType:       {{"this", true}, {"to", true}},
	KindExtract:             {{"this", true}, {"expression", true}},
	KindStrPosition:         {{"this", true}, {"substr", true}, {"position", false}, {"occurrence", false}, {"clamp_position", false}},
	KindSubstring:           {{"this", true}, {"start", false}, {"length", false}, {"zero_start", false}},
	KindTrim:                {{"this", true}, {"expression", false}, {"position", false}, {"collation", false}},
	KindCeil:                {{"this", true}, {"decimals", false}, {"to", false}},
	KindFloor:               {{"this", true}, {"decimals", false}, {"to", false}},
	KindGroupConcat:         {{"this", true}, {"separator", false}, {"on_overflow", false}},
	KindUnnest:              {{"expressions", true}, {"alias", false}, {"offset", false}, {"explode_array", false}},
	KindArray:               {{"expressions", false}, {"bracket_notation", false}, {"struct_name_inheritance", false}},
	KindBracket:             {{"this", true}, {"expressions", true}, {"offset", false}, {"safe", false}, {"returns_list_for_maps", false}, {"json_access", false}},
	KindLock:                {{"update", true}, {"expressions", false}, {"wait", false}, {"key", false}},
	KindPreWhere:            defaultArgTypes,
	KindCluster:             {{"expressions", true}},
	KindDistribute:          {{"this", false}, {"expressions", true}, {"siblings", false}},
	KindSort:                {{"this", false}, {"expressions", true}, {"siblings", false}},
	KindWithinGroup:         {{"this", true}, {"expression", false}},
	KindIgnoreNulls:         defaultArgTypes,
	KindRespectNulls:        defaultArgTypes,
	KindPivotAny:            {{"this", false}},
	KindPivotAlias:          {{"this", true}, {"alias", false}},
	KindInterval:            {{"this", false}, {"unit", false}},
	KindIntervalSpan:        {{"this", true}, {"expression", true}},
	KindJSONExtract:         {{"this", true}, {"expression", true}, {"only_json_types", false}, {"expressions", false}, {"variant_extract", false}, {"json_query", false}, {"option", false}, {"quote", false}, {"on_condition", false}, {"requires_json", false}, {"emits", false}},
	KindJSONExtractScalar:   {{"this", true}, {"expression", true}, {"only_json_types", false}, {"expressions", false}, {"json_type", false}, {"scalar_only", false}},
	KindJSONBExtract:        {{"this", true}, {"expression", true}},
	KindJSONBExtractScalar:  {{"this", true}, {"expression", true}, {"json_type", false}},
	KindJSONCast:            {{"this", true}, {"to", true}, {"format", false}, {"safe", false}, {"action", false}, {"default", false}},
	// JSONTable/JSONColumnDef/JSONSchema/FormatJson port JSON_TABLE(...) (parser.py:8166;
	// expressions/json.py:206, expressions/query.py:2022-2042).
	KindJSONTable:     {{"this", true}, {"schema", true}, {"path", false}, {"error_handling", false}, {"empty_handling", false}},
	KindJSONColumnDef: {{"this", false}, {"kind", false}, {"path", false}, {"nested_schema", false}, {"ordinality", false}, {"format_json", false}},
	KindJSONSchema:    {{"expressions", true}},
	KindFormatJson:    defaultArgTypes,
	// JSONObject/JSONObjectAgg/JSONKeyValue/OnCondition/JSONValue port the JSON_OBJECT/
	// JSON_OBJECTAGG/JSON_VALUE family (expressions/json.py:160-177, expressions/query.py:
	// 577,2026,2045).
	KindJSONObject:     {{"expressions", false}, {"null_handling", false}, {"unique_keys", false}, {"return_type", false}, {"encoding", false}},
	KindJSONObjectAgg:  {{"expressions", false}, {"null_handling", false}, {"unique_keys", false}, {"return_type", false}, {"encoding", false}},
	KindJSONKeyValue:   {{"this", true}, {"expression", true}},
	KindOnCondition:    {{"error", false}, {"empty", false}, {"null", false}},
	KindJSONValue:      {{"this", true}, {"path", true}, {"returning", false}, {"on_condition", false}},
	KindArrayAgg:       {{"this", true}, {"nulls_excluded", false}},
	KindArraySize:      {{"this", true}, {"expression", false}},
	KindArrayContains:  {{"this", true}, {"expression", true}, {"ensure_variant", false}, {"check_null", false}},
	KindInitcap:        {{"this", true}, {"expression", false}},
	KindSplit:          {{"this", true}, {"expression", true}, {"limit", false}, {"null_returns_null", false}, {"empty_delimiter_returns_whole", false}},
	KindRegexpLike:     {{"this", true}, {"expression", true}, {"flag", false}, {"full_match", false}},
	KindRegexpSplit:    {{"this", true}, {"expression", true}, {"limit", false}},
	KindStructExtract:  {{"this", true}, {"expression", true}},
	KindStandardHash:   {{"this", true}, {"expression", false}},
	KindHex:            {{"this", true}, {"case", false}},
	KindMD5:            defaultArgTypes,
	KindStPoint:        {{"this", true}, {"expression", true}, {"z", false}, {"m", false}},
	KindStDistance:     {{"this", true}, {"expression", true}, {"use_spheroid", false}},
	KindGenerateSeries: {{"start", true}, {"end", true}, {"step", false}, {"is_end_exclusive", false}},
	KindDate:           {{"this", false}, {"zone", false}, {"expressions", false}},
	KindAddMonths:      {{"this", true}, {"expression", true}, {"preserve_end_of_month", false}},
	KindDateAdd:        {{"this", true}, {"expression", true}, {"unit", false}},
	KindDateDiff:       {{"this", true}, {"expression", true}, {"unit", false}, {"zone", false}, {"big_int", false}, {"date_part_boundary", false}},
	KindJoinHint:       {{"this", true}, {"expressions", true}},
	KindTableColumn:    defaultArgTypes,
	KindFinal:          defaultArgTypes,
	// Set/SetItem/Show/.../Pragma (ddl.py, query.py, properties.py, dml.py; see the Kind
	// block comment above for exact line numbers).
	KindSet:            {{"expressions", false}, {"unset", false}, {"tag", false}},
	KindSetItem:        {{"this", false}, {"expressions", false}, {"kind", false}, {"collate", false}, {"global_", false}},
	KindShow:           {{"this", true}, {"history", false}, {"terse", false}, {"target", false}, {"offset", false}, {"starts_with", false}, {"limit", false}, {"from_", false}, {"like", false}, {"where", false}, {"db", false}, {"scope", false}, {"scope_kind", false}, {"full", false}, {"mutex", false}, {"query", false}, {"channel", false}, {"global_", false}, {"log", false}, {"position", false}, {"types", false}, {"privileges", false}, {"for_table", false}, {"for_group", false}, {"for_user", false}, {"for_role", false}, {"into_outfile", false}, {"json", false}, {"iceberg", false}},
	KindUse:            {{"this", false}, {"expressions", false}, {"kind", false}},
	KindKill:           {{"this", true}, {"kind", false}},
	KindDescribe:       {{"this", true}, {"style", false}, {"kind", false}, {"properties", false}, {"expressions", false}, {"partition", false}, {"format", false}, {"as_json", false}},
	KindLoadData:       {{"this", true}, {"local", false}, {"overwrite", false}, {"temp", false}, {"inpath", false}, {"files", false}, {"partition", false}, {"input_format", false}, {"serde", false}},
	KindTransaction:    {{"this", false}, {"modes", false}, {"mark", false}},
	KindCommit:         {{"chain", false}, {"this", false}, {"durability", false}},
	KindRollback:       {{"savepoint", false}, {"this", false}},
	KindGrant:          {{"privileges", true}, {"kind", false}, {"securable", true}, {"principals", true}, {"grant_option", false}},
	KindRevoke:         {{"privileges", true}, {"kind", false}, {"securable", true}, {"principals", true}, {"grant_option", false}, {"cascade", false}},
	KindGrantPrivilege: {{"this", true}, {"expressions", false}},
	KindGrantPrincipal: {{"this", true}, {"kind", false}},
	KindComment:        {{"this", true}, {"kind", true}, {"expression", true}, {"exists", false}, {"materialized", false}},
	KindTruncateTable:  {{"expressions", true}, {"is_database", false}, {"exists", false}, {"only", false}, {"cluster", false}, {"identity", false}, {"option", false}, {"partition", false}},
	KindPartition:      {{"expressions", true}, {"subpartition", false}},
	KindAnalyze:        {{"kind", false}, {"this", false}, {"options", false}, {"mode", false}, {"partition", false}, {"expression", false}, {"properties", false}},
	KindPragma:         defaultArgTypes,
	// Parameter/RawString/FileFormatProperty arg lists transcribed from core.py:1834
	// ({"this": True, "expression": False}), query.py:491 ({"this": True}), and
	// properties.py:177 ({"this": False, "expressions": False, "hive_format": False}).
	KindParameter:          {{"this", true}, {"expression", false}},
	KindRawString:          {{"this", true}},
	KindFileFormatProperty: {{"this", false}, {"expressions", false}, {"hive_format", false}},
	// ColumnConstraint/Constraint/PrimaryKey/ForeignKey/Reference family (constraints.py:8-237).
	KindColumnConstraint:           {{"this", false}, {"kind", true}},
	KindConstraint:                 {{"this", true}, {"expressions", true}},
	KindPrimaryKey:                 {{"this", false}, {"expressions", true}, {"options", false}, {"include", false}},
	KindPrimaryKeyColumnConstraint: {{"desc", false}, {"options", false}},
	KindForeignKey:                 {{"expressions", false}, {"reference", false}, {"delete", false}, {"update", false}, {"options", false}},
	KindReference:                  {{"this", true}, {"expressions", false}, {"options", false}},
	KindNotNullColumnConstraint:    {{"allow_null", false}},
	KindUniqueColumnConstraint:     {{"this", false}, {"index_type", false}, {"on_conflict", false}, {"nulls", false}, {"options", false}},
	KindCheckColumnConstraint:      {{"this", true}, {"enforced", false}},
	// Pure `pass` ColumnConstraintKind subclasses (constraints.py:32,52,68,72,84,155) inherit
	// Expression's default arg_types ({"this": True}, core.py:85), since ColumnConstraintKind
	// itself is `pass` (core.py:1605).
	KindDefaultColumnConstraint:      defaultArgTypes,
	KindCollateColumnConstraint:      defaultArgTypes,
	KindCommentColumnConstraint:      defaultArgTypes,
	KindCharacterSetColumnConstraint: defaultArgTypes,
	// AutoIncrementColumnConstraint inherits {"this": True} upstream too, but _parse_auto_increment
	// (parser.py:7347) returns a BARE exp.AutoIncrementColumnConstraint() via the raw constructor,
	// bypassing validate_expression - so the missing required `this` is never flagged upstream. This
	// port always routes node construction through p.expression (which validates), so we make `this`
	// optional here to match the effective upstream behavior; the generator ignores `this` anyway
	// (autoincrementcolumnconstraint_sql just emits the AUTO_INCREMENT token).
	KindAutoIncrementColumnConstraint:       {{"this", false}},
	KindOnUpdateColumnConstraint:            defaultArgTypes,
	KindGeneratedAsIdentityColumnConstraint: {{"this", false}, {"expression", false}, {"on_null", false}, {"start", false}, {"increment", false}, {"minvalue", false}, {"maxvalue", false}, {"cycle", false}, {"order", false}},
	KindGeneratedAsRowColumnConstraint:      {{"start", false}, {"hidden", false}},
	KindComputedColumnConstraint:            {{"this", true}, {"persisted", false}, {"not_null", false}, {"data_type", false}},
	KindCaseSpecificColumnConstraint:        {{"not_", true}},
	// Explicit arg_types = {} classes (constraints.py:37,41,144): no args at all.
	KindNotForReplicationColumnConstraint: {},
	KindZeroFillColumnConstraint:          {},
	KindInvisibleColumnConstraint:         {},
	KindIndexColumnConstraint:             {{"this", false}, {"expressions", false}, {"kind", false}, {"index_type", false}, {"options", false}, {"expression", false}, {"granularity", false}},
	KindIndexConstraintOption:             {{"key_block_size", false}, {"using", false}, {"parser", false}, {"comment", false}, {"visible", false}, {"engine_attr", false}, {"secondary_engine_attr", false}},
	KindIndexParameters:                   {{"using", false}, {"include", false}, {"columns", false}, {"with_storage", false}, {"partition_by", false}, {"tablespace", false}, {"where", false}, {"on", false}},
	KindColumnPrefix:                      {{"this", true}, {"expression", true}},
	// ColumnPosition/AddPartition/DropPartition (query.py:498,1941,1949).
	KindColumnPosition: {{"this", false}, {"position", true}},
	// Alter/Drop/AlterColumn/... family (ddl.py:241-401).
	KindAlter:        {{"this", false}, {"kind", true}, {"actions", true}, {"exists", false}, {"only", false}, {"options", false}, {"cluster", false}, {"not_valid", false}, {"check", false}, {"cascade", false}, {"iceberg", false}},
	KindDrop:         {{"this", false}, {"kind", false}, {"expressions", false}, {"exists", false}, {"temporary", false}, {"materialized", false}, {"cascade", false}, {"restrict", false}, {"constraints", false}, {"purge", false}, {"cluster", false}, {"concurrently", false}, {"sync", false}, {"iceberg", false}},
	KindAlterColumn:  {{"this", true}, {"dtype", false}, {"collate", false}, {"using", false}, {"default", false}, {"drop", false}, {"comment", false}, {"allow_null", false}, {"visible", false}, {"rename_to", false}},
	KindModifyColumn: {{"this", true}, {"rename_from", false}},
	KindAlterIndex:   {{"this", true}, {"visible", true}},
	KindRenameColumn: {{"this", true}, {"to", true}, {"exists", false}},
	KindRenameIndex:  {{"this", true}, {"to", true}},
	// AlterRename is `pass` (ddl.py:283): inherits Expression's default {"this": True}.
	KindAlterRename:    defaultArgTypes,
	KindAlterSet:       {{"expressions", false}, {"option", false}, {"tablespace", false}, {"access_method", false}, {"file_format", false}, {"copy_options", false}, {"tag", false}, {"location", false}, {"serde", false}},
	KindDropPrimaryKey: {},
	KindAddConstraint:  {{"expressions", true}},
	KindDropPartition:  {{"expressions", true}, {"exists", false}},
	KindAddPartition:   {{"this", true}, {"exists", false}, {"location", false}},
	// Lambda (query.py:646/647) and Replace (string.py:113/114).
	KindLambda:  {{"this", true}, {"expressions", true}, {"colon", false}},
	KindReplace: {{"this", true}, {"expression", true}, {"replacement", false}},
	// LogicalOr (aggregate.py:171) has no arg_types override, i.e. inherits the base
	// Expression default (mirroring e.g. KindStddev above).
	KindLogicalOr: defaultArgTypes,
	// AtTimeZone (core.py:2267/2268) and PseudoType/ObjectIdentifier (datatypes.py:
	// 439-444).
	KindAtTimeZone:       {{"this", true}, {"zone", true}},
	KindPseudoType:       defaultArgTypes,
	KindObjectIdentifier: defaultArgTypes,
	// Struct (array.py:356): arg_types = {"expressions": False}.
	KindStruct: {{"expressions", false}},
	// Chr (string.py:24): arg_types = {"expressions": True, "charset": False}.
	KindChr: {{"expressions", true}, {"charset", false}},
	// StrToDate/StrToTime/StrToUnix/TimeStrToTime declare their own arg_types dicts
	// (temporal.py:452-469, key order preserved); TimeStrToDate/TimeStrToUnix are `pass`
	// (temporal.py:464,472) and so fall back to Expression's default {"this": True} - spelled
	// out explicitly here (defaultArgTypes) rather than omitted, matching e.g. KindPragma above.
	KindStrToDate:     {{"this", true}, {"format", false}, {"safe", false}},
	KindStrToTime:     {{"this", true}, {"format", true}, {"zone", false}, {"safe", false}, {"target_type", false}},
	KindStrToUnix:     {{"this", false}, {"format", false}},
	KindTimeStrToDate: defaultArgTypes,
	KindTimeStrToTime: {{"this", true}, {"zone", false}},
	KindTimeStrToUnix: defaultArgTypes,
	// XMLElement/XMLTable (functions.py:439,453) and XMLNamespace (query.py:2081-2082,
	// `pass` -> inherits Expression's default {"this": True}, mirroring e.g. KindLambda's
	// sibling KindReplace comment style above).
	KindXMLElement:   {{"this", true}, {"expressions", false}, {"evalname", false}},
	KindXMLTable:     {{"this", true}, {"namespaces", false}, {"passing", false}, {"columns", false}, {"by_ref", false}},
	KindXMLNamespace: defaultArgTypes,
	// PathColumnConstraint (constraints.py:185): `pass` -> inherits {"this": True}.
	KindPathColumnConstraint: defaultArgTypes,
}

var traitsOf = map[Kind]Trait{
	KindColumn:             TraitCondition,
	KindLiteral:            TraitCondition,
	KindNull:               TraitCondition,
	KindBoolean:            TraitCondition,
	KindPlaceholder:        TraitCondition,
	KindParameter:          TraitCondition,
	KindRawString:          TraitCondition,
	KindDot:                TraitCondition | TraitBinary,
	KindAdd:                TraitCondition | TraitBinary,
	KindSub:                TraitCondition | TraitBinary,
	KindMul:                TraitCondition | TraitBinary,
	KindDiv:                TraitCondition | TraitBinary,
	KindMod:                TraitCondition | TraitBinary,
	KindEQ:                 TraitCondition | TraitBinary | TraitPredicate,
	KindNEQ:                TraitCondition | TraitBinary | TraitPredicate,
	KindNullSafeEQ:         TraitCondition | TraitBinary | TraitPredicate,
	KindGT:                 TraitCondition | TraitBinary | TraitPredicate,
	KindGTE:                TraitCondition | TraitBinary | TraitPredicate,
	KindLT:                 TraitCondition | TraitBinary | TraitPredicate,
	KindLTE:                TraitCondition | TraitBinary | TraitPredicate,
	KindLike:               TraitCondition | TraitBinary | TraitPredicate,
	KindILike:              TraitCondition | TraitBinary | TraitPredicate,
	KindSimilarTo:          TraitCondition | TraitBinary | TraitPredicate,
	KindEscape:             TraitCondition | TraitBinary,
	KindIs:                 TraitCondition | TraitBinary | TraitPredicate,
	KindBetween:            TraitPredicate,
	KindIn:                 TraitPredicate,
	KindAnd:                TraitCondition | TraitBinary | TraitConnector | TraitFunc,
	KindOr:                 TraitCondition | TraitBinary | TraitConnector | TraitFunc,
	KindBitwiseAnd:         TraitCondition | TraitBinary,
	KindBitwiseOr:          TraitCondition | TraitBinary,
	KindBitwiseXor:         TraitCondition | TraitBinary,
	KindDPipe:              TraitCondition | TraitBinary,
	KindParen:              TraitCondition | TraitUnary,
	KindNeg:                TraitCondition | TraitUnary,
	KindNot:                TraitCondition | TraitUnary,
	KindSelect:             TraitQuery,
	KindUnion:              TraitQuery | TraitSetOperation,
	KindExcept:             TraitQuery | TraitSetOperation,
	KindIntersect:          TraitQuery | TraitSetOperation,
	KindSubquery:           TraitQuery | TraitDerivedTable,
	KindNullSafeNEQ:        TraitCondition | TraitBinary | TraitPredicate,
	KindCase:               TraitCondition | TraitFunc,
	KindIf:                 TraitCondition | TraitFunc,
	KindExists:             TraitCondition | TraitPredicate | TraitFunc,
	KindAny:                TraitCondition | TraitPredicate,
	KindAll:                TraitCondition | TraitPredicate,
	KindAnonymous:          TraitCondition | TraitFunc,
	KindAbs:                TraitCondition | TraitFunc,
	KindSqrt:               TraitCondition | TraitFunc,
	KindLn:                 TraitCondition | TraitFunc,
	KindExp:                TraitCondition | TraitFunc,
	KindRound:              TraitCondition | TraitFunc,
	KindLog:                TraitCondition | TraitFunc,
	KindPow:                TraitCondition | TraitFunc,
	KindDay:                TraitCondition | TraitFunc,
	KindMonth:              TraitCondition | TraitFunc,
	KindYear:               TraitCondition | TraitFunc,
	KindQuarter:            TraitCondition | TraitFunc,
	KindCoalesce:           TraitCondition | TraitFunc,
	KindGreatest:           TraitCondition | TraitFunc,
	KindLeast:              TraitCondition | TraitFunc,
	KindWindow:             TraitCondition,
	KindAvg:                TraitCondition | TraitFunc | TraitAggFunc,
	KindSum:                TraitCondition | TraitFunc | TraitAggFunc,
	KindMin:                TraitCondition | TraitFunc | TraitAggFunc,
	KindMax:                TraitCondition | TraitFunc | TraitAggFunc,
	KindStddev:             TraitCondition | TraitFunc | TraitAggFunc,
	KindStddevPop:          TraitCondition | TraitFunc | TraitAggFunc,
	KindStddevSamp:         TraitCondition | TraitFunc | TraitAggFunc,
	KindVariance:           TraitCondition | TraitFunc | TraitAggFunc,
	KindVariancePop:        TraitCondition | TraitFunc | TraitAggFunc,
	KindApproxDistinct:     TraitCondition | TraitFunc | TraitAggFunc,
	KindHll:                TraitCondition | TraitFunc | TraitAggFunc,
	KindCountIf:            TraitCondition | TraitFunc | TraitAggFunc,
	KindQuantile:           TraitCondition | TraitFunc | TraitAggFunc,
	KindCount:              TraitCondition | TraitFunc | TraitAggFunc,
	KindCast:               TraitCondition | TraitFunc,
	KindTryCast:            TraitCondition | TraitFunc,
	KindCastToStrType:      TraitCondition | TraitFunc,
	KindExtract:            TraitCondition | TraitFunc,
	KindStrPosition:        TraitCondition | TraitFunc,
	KindSubstring:          TraitCondition | TraitFunc,
	KindTrim:               TraitCondition | TraitFunc,
	KindCeil:               TraitCondition | TraitFunc,
	KindFloor:              TraitCondition | TraitFunc,
	KindArray:              TraitCondition | TraitFunc,
	KindUnnest:             TraitCondition | TraitFunc | TraitUDTF | TraitDerivedTable,
	KindBracket:            TraitCondition,
	KindGroupConcat:        TraitCondition | TraitFunc | TraitAggFunc,
	KindJSONExtract:        TraitCondition | TraitBinary | TraitFunc,
	KindJSONExtractScalar:  TraitCondition | TraitBinary | TraitFunc,
	KindJSONBExtract:       TraitCondition | TraitBinary | TraitFunc,
	KindJSONBExtractScalar: TraitCondition | TraitBinary | TraitFunc,
	KindJSONCast:           TraitCondition | TraitFunc,
	// JSONTable(Expression, Func) (expressions/json.py:206); JSONColumnDef/JSONSchema/
	// FormatJson are plain Expression subclasses with no Func/Condition mixin, so they
	// get no trait bits (omitted below, matching e.g. KindTableAlias/KindSchema).
	KindJSONTable: TraitCondition | TraitFunc,
	// JSONObject(Expression, Func) and JSONObjectAgg(Expression, AggFunc)
	// (expressions/json.py:160,171); JSONKeyValue/OnCondition/JSONValue are plain Expression
	// subclasses with no Func/Condition mixin, so they get no trait bits (omitted below,
	// matching the JSONTable comment above).
	KindJSONObject:     TraitCondition | TraitFunc,
	KindJSONObjectAgg:  TraitCondition | TraitFunc | TraitAggFunc,
	KindArrayAgg:       TraitCondition | TraitFunc | TraitAggFunc,
	KindArraySize:      TraitCondition | TraitFunc,
	KindArrayContains:  TraitCondition | TraitBinary | TraitFunc,
	KindInitcap:        TraitCondition | TraitFunc,
	KindSplit:          TraitCondition | TraitFunc,
	KindRegexpLike:     TraitCondition | TraitBinary | TraitFunc,
	KindRegexpSplit:    TraitCondition | TraitFunc,
	KindStructExtract:  TraitCondition | TraitFunc,
	KindStandardHash:   TraitCondition | TraitFunc,
	KindHex:            TraitCondition | TraitFunc,
	KindMD5:            TraitCondition | TraitFunc,
	KindStPoint:        TraitCondition | TraitFunc,
	KindStDistance:     TraitCondition | TraitFunc,
	KindGenerateSeries: TraitCondition | TraitFunc,
	KindDate:           TraitCondition | TraitFunc,
	KindAddMonths:      TraitCondition | TraitFunc,
	KindDateAdd:        TraitCondition | TraitFunc,
	KindDateDiff:       TraitCondition | TraitFunc,
	KindCreate:         TraitDDL,
	KindInsert:         TraitDDL | TraitDML,
	KindUpdate:         TraitDML,
	KindDelete:         TraitDML,
	KindMerge:          TraitDML,
	KindCTE:            TraitDerivedTable,
	KindLateral:        TraitUDTF | TraitDerivedTable,
	KindValues:         TraitUDTF | TraitDerivedTable,
	// KindLambda (query.py:646) is a plain Expression, so it gets no row here (matching e.g.
	// KindTableAlias above).
	KindReplace:   TraitCondition | TraitFunc,
	KindLogicalOr: TraitCondition | TraitFunc | TraitAggFunc,
	// Struct is `class Struct(Expression, Func)` and Func(Condition) (core.py:1641),
	// so it carries both mixins - same as KindArray above.
	KindStruct: TraitCondition | TraitFunc,
	// Chr is `class Chr(Expression, Func)` (string.py:23), same mixins as KindStruct above.
	KindChr: TraitCondition | TraitFunc,
	// StrToDate/StrToTime/StrToUnix/TimeStrToDate/TimeStrToTime/TimeStrToUnix are all
	// `class X(Expression, Func)` (temporal.py:452-473), so every one gets both mixins - this
	// also covers TimeStrToDate/TimeStrToUnix, whose `pass` bodies affect only arg_types
	// (above), not the class bases.
	KindStrToDate:     TraitCondition | TraitFunc,
	KindStrToTime:     TraitCondition | TraitFunc,
	KindStrToUnix:     TraitCondition | TraitFunc,
	KindTimeStrToDate: TraitCondition | TraitFunc,
	KindTimeStrToTime: TraitCondition | TraitFunc,
	KindTimeStrToUnix: TraitCondition | TraitFunc,
	// XMLElement/XMLTable are `class X(Expression, Func)` (functions.py:439,453), so both
	// carry TraitFunc like KindStruct above. XMLNamespace has no row (see the Kind block
	// comment above).
	KindXMLElement: TraitCondition | TraitFunc,
	KindXMLTable:   TraitCondition | TraitFunc,
}

var primitive = map[Kind]bool{
	KindLiteral:    true,
	KindIdentifier: true,
	KindVar:        true,
	KindBoolean:    true,
}

var hashRaw = map[Kind]bool{
	KindLiteral:    true,
	KindIdentifier: true,
}

var className = map[Kind]string{
	KindExpression:                          "Expression",
	KindColumn:                              "Column",
	KindLiteral:                             "Literal",
	KindIdentifier:                          "Identifier",
	KindStar:                                "Star",
	KindAlias:                               "Alias",
	KindAliases:                             "Aliases",
	KindDot:                                 "Dot",
	KindNull:                                "Null",
	KindBoolean:                             "Boolean",
	KindVar:                                 "Var",
	KindParen:                               "Paren",
	KindNeg:                                 "Neg",
	KindNot:                                 "Not",
	KindAdd:                                 "Add",
	KindSub:                                 "Sub",
	KindMul:                                 "Mul",
	KindDiv:                                 "Div",
	KindMod:                                 "Mod",
	KindEQ:                                  "EQ",
	KindNEQ:                                 "NEQ",
	KindNullSafeEQ:                          "NullSafeEQ",
	KindGT:                                  "GT",
	KindGTE:                                 "GTE",
	KindLT:                                  "LT",
	KindLTE:                                 "LTE",
	KindAnd:                                 "And",
	KindOr:                                  "Or",
	KindBitwiseAnd:                          "BitwiseAnd",
	KindBitwiseOr:                           "BitwiseOr",
	KindBitwiseXor:                          "BitwiseXor",
	KindDPipe:                               "DPipe",
	KindBetween:                             "Between",
	KindIs:                                  "Is",
	KindLike:                                "Like",
	KindILike:                               "ILike",
	KindSimilarTo:                           "SimilarTo",
	KindEscape:                              "Escape",
	KindIn:                                  "In",
	KindSelect:                              "Select",
	KindFrom:                                "From",
	KindJoin:                                "Join",
	KindTable:                               "Table",
	KindTableAlias:                          "TableAlias",
	KindWhere:                               "Where",
	KindGroup:                               "Group",
	KindOrder:                               "Order",
	KindLimit:                               "Limit",
	KindOffset:                              "Offset",
	KindHint:                                "Hint",
	KindBlock:                               "Block",
	KindPlaceholder:                         "Placeholder",
	KindTuple:                               "Tuple",
	KindWith:                                "With",
	KindCTE:                                 "CTE",
	KindRecursiveWithSearch:                 "RecursiveWithSearch",
	KindUnion:                               "Union",
	KindExcept:                              "Except",
	KindIntersect:                           "Intersect",
	KindSubquery:                            "Subquery",
	KindHaving:                              "Having",
	KindQualify:                             "Qualify",
	KindCube:                                "Cube",
	KindRollup:                              "Rollup",
	KindGroupingSets:                        "GroupingSets",
	KindOrdered:                             "Ordered",
	KindDistinct:                            "Distinct",
	KindWindow:                              "Window",
	KindWindowSpec:                          "WindowSpec",
	KindFilter:                              "Filter",
	KindLimitOptions:                        "LimitOptions",
	KindFetch:                               "Fetch",
	KindCase:                                "Case",
	KindIf:                                  "If",
	KindExists:                              "Exists",
	KindAny:                                 "Any",
	KindAll:                                 "All",
	KindNullSafeNEQ:                         "NullSafeNEQ",
	KindAnonymous:                           "Anonymous",
	KindAbs:                                 "Abs",
	KindAvg:                                 "Avg",
	KindSum:                                 "Sum",
	KindSqrt:                                "Sqrt",
	KindLn:                                  "Ln",
	KindExp:                                 "Exp",
	KindMin:                                 "Min",
	KindMax:                                 "Max",
	KindRound:                               "Round",
	KindLog:                                 "Log",
	KindPow:                                 "Pow",
	KindStddev:                              "Stddev",
	KindStddevPop:                           "StddevPop",
	KindStddevSamp:                          "StddevSamp",
	KindVariance:                            "Variance",
	KindVariancePop:                         "VariancePop",
	KindDay:                                 "Day",
	KindMonth:                               "Month",
	KindYear:                                "Year",
	KindQuarter:                             "Quarter",
	KindApproxDistinct:                      "ApproxDistinct",
	KindHll:                                 "Hll",
	KindCountIf:                             "CountIf",
	KindQuantile:                            "Quantile",
	KindCount:                               "Count",
	KindCoalesce:                            "Coalesce",
	KindGreatest:                            "Greatest",
	KindLeast:                               "Least",
	KindInsert:                              "Insert",
	KindUpdate:                              "Update",
	KindDelete:                              "Delete",
	KindMerge:                               "Merge",
	KindWhen:                                "When",
	KindWhens:                               "Whens",
	KindOnConflict:                          "OnConflict",
	KindReturning:                           "Returning",
	KindInto:                                "Into",
	KindCreate:                              "Create",
	KindSchema:                              "Schema",
	KindCommand:                             "Command",
	KindPivot:                               "Pivot",
	KindLateral:                             "Lateral",
	KindValues:                              "Values",
	KindColumnDef:                           "ColumnDef",
	KindDataType:                            "DataType",
	KindDataTypeParam:                       "DataTypeParam",
	KindCast:                                "Cast",
	KindTryCast:                             "TryCast",
	KindCastToStrType:                       "CastToStrType",
	KindExtract:                             "Extract",
	KindStrPosition:                         "StrPosition",
	KindSubstring:                           "Substring",
	KindTrim:                                "Trim",
	KindCeil:                                "Ceil",
	KindFloor:                               "Floor",
	KindGroupConcat:                         "GroupConcat",
	KindUnnest:                              "Unnest",
	KindArray:                               "Array",
	KindBracket:                             "Bracket",
	KindLock:                                "Lock",
	KindPreWhere:                            "PreWhere",
	KindCluster:                             "Cluster",
	KindDistribute:                          "Distribute",
	KindSort:                                "Sort",
	KindWithinGroup:                         "WithinGroup",
	KindIgnoreNulls:                         "IgnoreNulls",
	KindRespectNulls:                        "RespectNulls",
	KindPivotAny:                            "PivotAny",
	KindPivotAlias:                          "PivotAlias",
	KindInterval:                            "Interval",
	KindIntervalSpan:                        "IntervalSpan",
	KindJSONExtract:                         "JSONExtract",
	KindJSONExtractScalar:                   "JSONExtractScalar",
	KindJSONBExtract:                        "JSONBExtract",
	KindJSONBExtractScalar:                  "JSONBExtractScalar",
	KindJSONCast:                            "JSONCast",
	KindJSONTable:                           "JSONTable",
	KindJSONColumnDef:                       "JSONColumnDef",
	KindJSONSchema:                          "JSONSchema",
	KindFormatJson:                          "FormatJson",
	KindArrayAgg:                            "ArrayAgg",
	KindArraySize:                           "ArraySize",
	KindArrayContains:                       "ArrayContains",
	KindInitcap:                             "Initcap",
	KindSplit:                               "Split",
	KindRegexpLike:                          "RegexpLike",
	KindRegexpSplit:                         "RegexpSplit",
	KindStructExtract:                       "StructExtract",
	KindStandardHash:                        "StandardHash",
	KindHex:                                 "Hex",
	KindMD5:                                 "MD5",
	KindStPoint:                             "StPoint",
	KindStDistance:                          "StDistance",
	KindGenerateSeries:                      "GenerateSeries",
	KindDate:                                "Date",
	KindAddMonths:                           "AddMonths",
	KindDateAdd:                             "DateAdd",
	KindDateDiff:                            "DateDiff",
	KindJoinHint:                            "JoinHint",
	KindTableColumn:                         "TableColumn",
	KindFinal:                               "Final",
	KindSet:                                 "Set",
	KindSetItem:                             "SetItem",
	KindShow:                                "Show",
	KindUse:                                 "Use",
	KindKill:                                "Kill",
	KindDescribe:                            "Describe",
	KindLoadData:                            "LoadData",
	KindTransaction:                         "Transaction",
	KindCommit:                              "Commit",
	KindRollback:                            "Rollback",
	KindGrant:                               "Grant",
	KindRevoke:                              "Revoke",
	KindGrantPrivilege:                      "GrantPrivilege",
	KindGrantPrincipal:                      "GrantPrincipal",
	KindComment:                             "Comment",
	KindTruncateTable:                       "TruncateTable",
	KindPartition:                           "Partition",
	KindAnalyze:                             "Analyze",
	KindPragma:                              "Pragma",
	KindParameter:                           "Parameter",
	KindRawString:                           "RawString",
	KindFileFormatProperty:                  "FileFormatProperty",
	KindColumnConstraint:                    "ColumnConstraint",
	KindConstraint:                          "Constraint",
	KindPrimaryKey:                          "PrimaryKey",
	KindPrimaryKeyColumnConstraint:          "PrimaryKeyColumnConstraint",
	KindForeignKey:                          "ForeignKey",
	KindReference:                           "Reference",
	KindNotNullColumnConstraint:             "NotNullColumnConstraint",
	KindUniqueColumnConstraint:              "UniqueColumnConstraint",
	KindCheckColumnConstraint:               "CheckColumnConstraint",
	KindDefaultColumnConstraint:             "DefaultColumnConstraint",
	KindCollateColumnConstraint:             "CollateColumnConstraint",
	KindCommentColumnConstraint:             "CommentColumnConstraint",
	KindCharacterSetColumnConstraint:        "CharacterSetColumnConstraint",
	KindAutoIncrementColumnConstraint:       "AutoIncrementColumnConstraint",
	KindOnUpdateColumnConstraint:            "OnUpdateColumnConstraint",
	KindGeneratedAsIdentityColumnConstraint: "GeneratedAsIdentityColumnConstraint",
	KindGeneratedAsRowColumnConstraint:      "GeneratedAsRowColumnConstraint",
	KindComputedColumnConstraint:            "ComputedColumnConstraint",
	KindCaseSpecificColumnConstraint:        "CaseSpecificColumnConstraint",
	KindNotForReplicationColumnConstraint:   "NotForReplicationColumnConstraint",
	KindZeroFillColumnConstraint:            "ZeroFillColumnConstraint",
	KindInvisibleColumnConstraint:           "InvisibleColumnConstraint",
	KindIndexColumnConstraint:               "IndexColumnConstraint",
	KindIndexConstraintOption:               "IndexConstraintOption",
	KindIndexParameters:                     "IndexParameters",
	KindColumnPrefix:                        "ColumnPrefix",
	KindColumnPosition:                      "ColumnPosition",
	KindAlter:                               "Alter",
	KindDrop:                                "Drop",
	KindAlterColumn:                         "AlterColumn",
	KindModifyColumn:                        "ModifyColumn",
	KindAlterIndex:                          "AlterIndex",
	KindRenameColumn:                        "RenameColumn",
	KindRenameIndex:                         "RenameIndex",
	KindAlterRename:                         "AlterRename",
	KindAlterSet:                            "AlterSet",
	KindDropPrimaryKey:                      "DropPrimaryKey",
	KindAddConstraint:                       "AddConstraint",
	KindDropPartition:                       "DropPartition",
	KindAddPartition:                        "AddPartition",
	KindLambda:                              "Lambda",
	KindReplace:                             "Replace",
	KindLogicalOr:                           "LogicalOr",
	KindAtTimeZone:                          "AtTimeZone",
	KindPseudoType:                          "PseudoType",
	KindObjectIdentifier:                    "ObjectIdentifier",
	KindStruct:                              "Struct",
	KindJSONObject:                          "JSONObject",
	KindJSONObjectAgg:                       "JSONObjectAgg",
	KindJSONKeyValue:                        "JSONKeyValue",
	KindOnCondition:                         "OnCondition",
	KindJSONValue:                           "JSONValue",
	KindChr:                                 "Chr",
	KindStrToDate:                           "StrToDate",
	KindStrToTime:                           "StrToTime",
	KindStrToUnix:                           "StrToUnix",
	KindTimeStrToDate:                       "TimeStrToDate",
	KindTimeStrToTime:                       "TimeStrToTime",
	KindTimeStrToUnix:                       "TimeStrToUnix",
	KindXMLElement:                          "XMLElement",
	KindXMLTable:                            "XMLTable",
	KindXMLNamespace:                        "XMLNamespace",
	KindPathColumnConstraint:                "PathColumnConstraint",
}

// varLenArgs is the authoritative is_var_len_args=True set (mirroring the upstream Func
// class attribute): the final arg_type is a variadic list. It drives both FromArgList's
// var-len packing and the error_messages arg-count check (Node.ErrorMessages). Count/
// Coalesce/Greatest/Least set it upstream too; they build via custom builders (never
// FromArgList) but must still be recognized here so the over-arity check skips them.
var varLenArgs = map[Kind]bool{
	KindMax:               true,
	KindMin:               true,
	KindHll:               true,
	KindAnonymous:         true,
	KindCount:             true,
	KindCoalesce:          true,
	KindGreatest:          true,
	KindLeast:             true,
	KindArray:             true,
	KindStruct:            true,
	KindJSONExtract:       true,
	KindJSONExtractScalar: true,
	KindDate:              true,
	KindChr:               true,
}

// ArgKeys returns the arg keys of a Kind in class-declaration order (mirrors
// Python's iteration over Expr.arg_types). Used by the generator's function fallback.
func ArgKeys(k Kind) []string {
	specs := argTypesFor(k)
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Key
	}
	return out
}

// ClassName returns the PascalCase class name for a Kind (e.g. "StddevPop").
func ClassName(k Kind) string { return className[k] }
