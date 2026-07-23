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
	// The 20 Kinds below close the statement-level parse gaps (SET/SHOW/USE/DESCRIBE/
	// EXPLAIN/BEGIN/COMMIT/ROLLBACK/KILL/LOAD DATA/ANALYZE/GRANT/REVOKE/COMMENT ON/
	// TRUNCATE, plus PRAGMA): ddl.py:104,121,157,173,181,191,306,385,389,393,426;
	// query.py:589,609,619,1879; properties.py:16,20; dml.py:281. KindSavepoint (below,
	// after KindRollback) is the port's own transaction-control extension (no upstream node).
	// All are plain Expression subclasses (no Func/Condition/... mixins), so none get traitsOf rows.
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
	// KindSavepoint models `SAVEPOINT name` / `RELEASE [SAVEPOINT] name` transaction-control
	// statements. Upstream has no such node (it mis-parses SAVEPOINT as an Alias and parse-errors
	// RELEASE SAVEPOINT); see DEVIATIONS.
	KindSavepoint
	// KindReset models Postgres `RESET { name | ALL }` (reset a run-time parameter to its default).
	// Upstream tokenizes RESET as a raw Command; see DEVIATIONS.
	KindReset
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
	// KindTableSample ports exp.TableSample (query.py:1723-1733): the `TABLESAMPLE (...)`
	// modifier attached to a table source's "sample" arg (already-wired KindTable arg key,
	// tableSQL/subquerySQL). A plain Expression (no Func/Condition mixin), so it gets no
	// traitsOf row below.
	KindTableSample
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
	// KindCache/KindUncache port exp.Cache/exp.Uncache (core.py:1583-1594): Spark's
	// `CACHE [LAZY] TABLE x [OPTIONS(k = v)] [AS <query>]` / `UNCACHE TABLE [IF EXISTS] x`.
	// Both are plain Expression subclasses (no Func/Condition/... mixin), so neither
	// gets a traitsOf row below.
	KindCache
	KindUncache
	// KindCopy/KindCopyParameter/KindCredentials port exp.Copy/exp.CopyParameter/
	// exp.Credentials (dml.py:166-174, 162-163, 177-184): the `COPY ... FROM/TO ...`
	// statement (parser.py:9616-9649, generator.py:5393-5419). Copy(Expression, DML) gets
	// a traitsOf row (TraitDML); CopyParameter and Credentials are plain Expression
	// subclasses with no mixin, so they get none (mirroring e.g. KindTableAlias).
	KindCopy
	KindCopyParameter
	KindCredentials
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
	// The affix/connector-operator cluster below closes the COLLATE / unary bitwise-not /
	// PIPE_SLASH-CBRT / bitwise-shift / mysql XOR parity gaps, ported from _parse_term,
	// UNARY_PARSERS, _parse_bitwise, and CONJUNCTION/DISJUNCTION (parser.py:908-913,
	// 1113-1120, 6064-6117; parsers/mysql.py:72-81): exp.Collate (functions.py:171,
	// `class Collate(Expression, Binary, Func): pass`), exp.BitwiseNot (core.py:2242,
	// `class BitwiseNot(Unary): pass`), exp.Cbrt (math.py:130, `class Cbrt(Expression,
	// Func): pass`), exp.BitwiseLeftShift/exp.BitwiseRightShift (core.py:2102-2112, both
	// `class ...(Expression, Binary)` with an extra "requires_int128" arg unused by the
	// base parser), and exp.Xor (core.py:2312, `class Xor(Expression, Connector, Func)`
	// with an extra "round_input" arg, likewise unset by the base/mysql parser).
	KindCollate
	KindBitwiseNot
	KindCbrt
	KindBitwiseLeftShift
	KindBitwiseRightShift
	KindXor
	// FROM-clause / DML tail cluster: exp.Directory (dml.py:187, Hive/Spark `INSERT
	// OVERWRITE [LOCAL] DIRECTORY '...'`), exp.RowFormatDelimitedProperty/
	// RowFormatSerdeProperty/SerdeProperties (properties.py:405,418,452, the Hive `ROW
	// FORMAT ...` clause used by Directory), and exp.WithTableHint/IndexTableHint
	// (query.py:923,927 - T-SQL `WITH (...)` / MySQL `USE|FORCE|IGNORE INDEX` table
	// hints, parsed by _parse_table_hints and threaded through Table's existing "hints"
	// arg). None of these have a Condition/Predicate/... mixin upstream, so none get a
	// traitsOf row below.
	KindDirectory
	KindRowFormatDelimitedProperty
	KindRowFormatSerdeProperty
	KindSerdeProperties
	KindWithTableHint
	KindIndexTableHint

	// ddl cluster: CREATE FUNCTION/PROCEDURE/INDEX/TRIGGER structured parsing, plus the
	// small PROPERTY_PARSERS subset (parser.py:1227-1341) needed to close the DDL-tail
	// parity gaps (RETURNS/LANGUAGE/SQL SECURITY/CALLED ON NULL INPUT/STRICT/IMMUTABLE-
	// STABLE-DETERMINISTIC/SET .../CHARACTER SET/CHARSET/ROW_FORMAT). None of these carry a
	// Condition/Func/... mixin upstream (all `class X(Expression)` or `class X(Property)`,
	// and Property itself is a plain Expression subclass), so none get a traitsOf row below -
	// mirroring the existing KindTableAlias/KindLambda pattern.
	//
	// KindProperties/KindUserDefinedFunction (ddl.py / properties.py): the generic property
	// list container, and the UDF signature node (`name(args...)`).
	KindProperties
	KindUserDefinedFunction
	// KindReturnsProperty/KindLanguageProperty/KindSqlSecurityProperty/
	// KindCalledOnNullInputProperty/KindStrictProperty/KindStabilityProperty/
	// KindSetConfigProperty/KindCharacterSetProperty/KindRowFormatProperty (properties.py:
	// 74,222,393,397,401,456ish,464,480) - the small set of CREATE FUNCTION/TABLE tail
	// properties this slice supports; the other ~90 PROPERTY_PARSERS entries are exotic
	// dialect-specific storage/model properties out of scope (documented divergence,
	// mirroring constraintParsers' analogous omission list in parser_constraints.go).
	KindReturnsProperty
	KindLanguageProperty
	KindSqlSecurityProperty
	KindCalledOnNullInputProperty
	KindStrictProperty
	KindStabilityProperty
	KindSetConfigProperty
	KindCharacterSetProperty
	KindRowFormatProperty
	// KindReturn ports exp.Return (query.py:882, `pass` - default {"this": True}): the
	// `RETURN <expr>` wrapper _parse_create can apply to a UDF/procedure body
	// (parser.py:2469-2470). Reachable only via the BEGIN/RETURN body path, which this port
	// otherwise defers (no exp.Block/heredoc support) - kept for completeness/robustness
	// rather than any target-gap SQL actually needing it.
	KindReturn
	// KindIndex/KindOpclass port exp.Index (query.py:558-566) and exp.Opclass (core.py:1817-
	// 1818, `col opclass_name` inside an index column list, e.g. `USING gin(title
	// public.gin_trgm_ops)`). KindIndexParameters (already ported) is Index's "params" arg.
	KindIndex
	KindOpclass
	// KindTriggerProperties/KindTriggerExecute/KindTriggerEvent/KindTriggerReferencing port
	// exp.TriggerProperties/TriggerExecute/TriggerEvent/TriggerReferencing (ddl.py:76-101):
	// CREATE TRIGGER's structured body (timing/events/ON table/FROM/DEFERRABLE/REFERENCING/
	// FOR EACH .../WHEN/EXECUTE FUNCTION|PROCEDURE).
	KindTriggerProperties
	KindTriggerExecute
	KindTriggerEvent
	KindTriggerReferencing

	// dialect-funcs cluster: canonical Kinds closing the mysql CURTIME/CURDATE/
	// DAY_OF_MONTH/DAY_OF_WEEK/DAY_OF_YEAR/WEEK_OF_YEAR/LCASE/UCASE/DATABASE/TRUNC
	// spelling gaps and the postgres CHAR_LENGTH/CHARACTER_LENGTH/OVERLAY/VARIADIC gaps:
	// exp.CurrentDate/CurrentTime (temporal.py:25,33), exp.DayOfMonth/DayOfWeek/DayOfYear/
	// WeekOfYear (temporal.py:209,213,221,269), exp.CurrentSchema (functions.py:285),
	// exp.Lower/Upper (string.py:85,254), exp.Length (string.py:69), exp.TimeStrToUnix
	// (temporal.py:472), exp.Trunc (math.py:188), exp.Overlay (string.py:101), and
	// exp.Variadic (query.py:2099 - a plain `pass` Expression, no Func/Condition mixin,
	// so it gets no traitsOf row below, mirroring KindLambda/KindTableAlias).
	KindCurrentDate
	KindCurrentTime
	KindCurrentSchema
	KindDayOfMonth
	KindDayOfWeek
	KindDayOfYear
	KindWeekOfYear
	KindLower
	KindUpper
	KindLength
	KindTrunc
	KindOverlay
	KindVariadic
	// slice-strings cluster: KindSlice is exp.Slice (expressions/core.py:2017-2018, a plain
	// Expression) - the `start:end:step` triple inside a bracket subscript, e.g. x[1:2].
	// KindNational is exp.National (expressions/query.py:585-586, is_primitive=True) - a
	// national-character string literal, e.g. N'abc'.
	KindSlice
	KindNational

	// range-ops cluster: binary/range operators owned by parseRange/parseIs/
	// columnOperators (parser.go). Mirrors core.py/array.py/json.py/string.py/query.py
	// Binary subclasses used by RANGE_PARSERS + COLUMN_OPERATORS[PLACEHOLDER].
	//
	// KindGlob (core.py:2166, `class Glob(Expression, Binary, Predicate)`).
	KindGlob
	// KindOverlaps (core.py:2122, `class Overlaps(Expression, Binary)`).
	KindOverlaps
	// KindRegexpILike (string.py:448, `class RegexpILike(Expression, Binary, Func)`):
	// the `~*` postgres operator / IRLIKE token. KindRegexpLike (RLIKE token / `~`)
	// already exists above (function-registry cluster) and is reused as-is.
	KindRegexpILike
	// KindAdjacent (core.py:2234, `class Adjacent(Expression, Binary)`): postgres
	// range `-|-` operator.
	KindAdjacent
	// KindArrayContainsAll/KindArrayContainedBy/KindArrayOverlaps (array.py:113,117,131,
	// all `Expression, Binary, Func`): postgres `@>`/`<@`/`&&` array operators.
	KindArrayContainsAll
	KindArrayContainedBy
	KindArrayOverlaps
	// KindJSONBContains/KindJSONBContainsAllTopKeys/KindJSONBContainsAnyTopKeys/
	// KindJSONBDeleteAtPath/KindJSONBPathExists (json.py:49,53,57,61,65, all
	// `Expression, Binary, Func` except JSONBPathExists which also mixes in
	// Predicate): postgres `?`/`?&`/`?|`/`#-`/`@?` jsonb operators.
	KindJSONBContains
	KindJSONBContainsAllTopKeys
	KindJSONBContainsAnyTopKeys
	KindJSONBDeleteAtPath
	KindJSONBPathExists
	// KindJSON (query.py:1965, `class JSON(Expression)`, no Condition/Func mixin):
	// the `x IS JSON [VALUE|SCALAR|ARRAY|OBJECT] [WITH|WITHOUT] [UNIQUE KEYS]`
	// predicate kind, built by parseIs and rendered by json_sql.
	KindJSON
	// KindOperator (core.py:2222, `class Operator(Expression, Binary)`): postgres
	// `x OPERATOR(schema.op) y` explicit-operator syntax (_parse_operator).
	KindOperator
	// KindMatchAgainst (string.py:89, `class MatchAgainst(Expression, Func)`, no
	// Binary mixin): mysql `MATCH(...) AGAINST(...)` full-text search, reused here
	// as the target of postgres' `x @@ y` operator (parsers/postgres.py RANGE_PARSERS
	// TokenType.DAT builds MatchAgainst(this=rhs, expressions=[lhs]), NOT a dedicated
	// JSONB-path-match node - v30.12.0 has no exp.JSONBPathMatch).
	KindMatchAgainst
	// KindJSONArrayContains (json.py:38, `class JSONArrayContains(Expression, Binary,
	// Predicate, Func)`): mysql `x MEMBER OF(y)`, built by parsers/mysql.py's
	// RANGE_PARSERS[MEMBER_OF] (there is no dedicated exp.MemberOf class upstream).
	KindJSONArrayContains
	// KindSoundex (string.py:144, `class Soundex(Expression, Func)`): mysql
	// `x SOUNDS LIKE y` desugars to EQ(Soundex(x), Soundex(y)) in
	// parsers/mysql.py's RANGE_PARSERS[SOUNDS_LIKE] (there is no dedicated
	// exp.SoundsLike class upstream).
	KindSoundex

	// residual-tail cluster: byte/hex/bit-string literals, session parameters, the `:=`
	// assignment operator, and the postgres distance operators.
	//
	// KindBitString/KindHexString/KindByteString (query.py:471-491, all `Expression,
	// Condition` with `is_primitive = True`): BIT_STRING/HEX_STRING/BYTE_STRING token
	// literals (`b'101'`/`0b101`, `x'FF'`/`0xFF`, postgres `e'...'`). KindRawString
	// already exists (slice-strings cluster) and covers HEREDOC_STRING/RAW_STRING.
	KindBitString
	KindHexString
	KindByteString
	// KindSessionParameter (core.py:1837, `class SessionParameter(Expression, Condition)`):
	// mysql `@@GLOBAL.max_connections` / bare `@@x` session-variable reference.
	KindSessionParameter
	// KindPropertyEQ (core.py:2150, `class PropertyEQ(Expression, Binary)`): the ASSIGNMENT
	// `:=` operator (parser.py:881-883), e.g. mysql `SELECT @var1 := 1`.
	KindPropertyEQ
	// KindDistance/KindDistanceNd (core.py:2154/2158, both `class X(Expression, Binary)`):
	// the FACTOR-level postgres `<->`/`<<->>` distance operators (parser.py:917-918).
	KindDistance
	KindDistanceNd
	// KindLag/KindLead (aggregate.py:150-163, both `class X(Expression, AggFunc)`): the
	// LAG/LEAD window functions. Previously unregistered (fell through to Anonymous), which
	// meant a parenthesized call like `(LEAD(x)) OVER (...)` never tripped parseParen's
	// `this.This().Is(exp.TraitAggFunc)` window-reparse gate (Anonymous has no AggFunc
	// trait) - registering them here is what actually closes that gap, not a parser change.
	KindLag
	KindLead
	// KindConcat (string.py:29-31, `class Concat(Expression, Func)`, is_var_len_args=True):
	// built by both _parse_primary's adjacent-string-literal rewrite (`'a' 'b'` -> Concat,
	// parser.py:6871-6885) and the CONCAT(...) function builder (parser.py:345-349, ported as
	// parser.parseConcat - dialect-aware, since safe/coalesce depend on the dialect). CONCAT_WS
	// stays Anonymous: it needs a separate exp.ConcatWs node, out of scope here.
	KindConcat

	// fidelity-properties/constraints/query cluster: CREATE/ALTER properties, column
	// constraints, partition bounds, and ANALYZE options needed by the DDL fidelity slice.
	// Upstream defines these as plain Expression, Property, or ColumnConstraintKind subclasses
	// (properties.py:12-553, constraints.py:76-198, query.py:589-594,1879-1938), so none
	// receive traitsOf, primitive, or varLenArgs rows below.
	KindProperty
	KindAlgorithmProperty
	KindAutoIncrementProperty
	KindCollateProperty
	KindDefinerProperty
	KindEngineProperty
	KindInheritsProperty
	KindLikeProperty
	KindLockProperty
	KindLockingProperty
	KindMaterializedProperty
	KindNoPrimaryIndexProperty
	KindOnCommitProperty
	KindPartitionedByProperty
	KindPartitionByRangeProperty
	KindPartitionByListProperty
	KindPartitionList
	KindPartitionBoundSpec
	KindPartitionedOfProperty
	KindSchemaCommentProperty
	KindSqlReadWriteProperty
	KindTemporaryProperty
	KindUnloggedProperty
	KindWithDataProperty
	KindPartitionRange
	KindAnalyzeHistogram
	KindAnalyzeWith
	KindUsingData
	KindCompressColumnConstraint
	KindDateFormatColumnConstraint
	KindExcludeColumnConstraint
	KindInlineLengthColumnConstraint
	KindTitleColumnConstraint
	KindUppercaseColumnConstraint
	KindWithOperator
	KindInOutColumnConstraint
	// presto cluster: Func-trait Kinds referenced by the Presto FUNCTIONS overlay
	// (dialects/presto.go). Appended at the end of the iota block so existing Kind
	// values are not renumbered (values aren't serialized - additive-safe). Each cites
	// its upstream class in the parallel tables below and in expressions/presto_nodes.go.
	KindAnyValue         // aggregate.py:17
	KindApproxQuantile   // aggregate.py:234
	KindArrayUniqueAgg   // aggregate.py:83
	KindDayOfWeekIso     // temporal.py:217
	KindDecode           // string.py:285
	KindEncode           // string.py:289
	KindJSONFormat       // json.py:145
	KindLevenshtein      // string.py:74
	KindMD5Digest        // string.py:540
	KindSHA2             // string.py:562
	KindStrToMap         // string.py:203
	KindTimeToUnix       // temporal.py:484
	KindUnixToTime       // temporal.py:532
	KindUnhex            // string.py:405
	KindArraySlice       // array.py:85
	KindCurrentTimestamp // temporal.py:37
	// KindUnicodeString is exp.UnicodeString (query.py:494, `U&'...'`), a primitive
	// Condition (NOT a Func) - it has no functionFallbackSQL path, so the generator
	// supplies a dedicated dispatch method (mirrors KindNational's `N'...'`).
	KindUnicodeString // query.py:494
	// trino cluster: minimal AST surface required by the Trino/Athena parser and
	// generator deltas. These mirror functions.py:249-250,317-318,
	// query.py:1961-1962,2062-2066, core.py:1596-1597, and array.py:146-147.
	KindCurrentCatalog           // functions.py:249-250
	KindCurrentVersion           // functions.py:317-318
	KindJSONExtractQuote         // query.py:2062-2066
	KindOverflowTruncateBehavior // query.py:1961-1962
	KindRefresh                  // core.py:1596-1597
	KindArrayFirst               // array.py:146-147
	// hive cluster: property, transform, map, and function Kinds referenced by the
	// Hive parser and FUNCTIONS overlay. Appended so existing Kind values remain stable.
	KindExternalProperty       // properties.py:168-169
	KindClusteredByProperty    // properties.py:230-231
	KindLocationProperty       // properties.py:262-263
	KindStorageHandlerProperty // properties.py:488-489
	KindUsingProperty          // properties.py:492-494
	KindInputOutputFormat      // query.py:878-879
	KindQueryTransform         // properties.py:422-431
	KindTransform              // array.py:214-215
	KindToBase64               // string.py:325-326
	KindFromBase64             // string.py:301-302
	KindTsOrDsAdd              // temporal.py:145-147
	KindTsOrDsToDate           // temporal.py:492-493
	KindFirst                  // aggregate.py:125-126
	KindFirstValue             // aggregate.py:129-130
	KindLast                   // aggregate.py:155-156
	KindLastValue              // aggregate.py:159-160
	KindRegexpExtract          // string.py:421-430
	KindRegexpExtractAll       // string.py:433-441
	KindTimestampTrunc         // temporal.py:190-191
	KindUnixToStr              // temporal.py:528-529
	KindTimeToStr              // temporal.py:476-477
	KindStarMap                // array.py:331-332
	KindVarMap                 // array.py:339-341
	// Niladic system-value functions completing NO_PAREN_FUNCTIONS (parser.py:431-438 +
	// parsers/base.py:8-13, parsers/postgres.py:145-152, parsers/mysql.py:55-59). CurrentDate/
	// CurrentTime/CurrentTimestamp/CurrentSchema/CurrentCatalog already exist above.
	KindCurrentUser    // functions.py:309-310
	KindCurrentRole    // functions.py:277-278
	KindSessionUser    // functions.py:325-326
	KindLocaltime      // temporal.py:49-50
	KindLocaltimestamp // temporal.py:53-54
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
	KindColumn:     {{"this", true}, {"table", false}, {"schema", false}, {"catalog", false}, {"join_mark", false}},
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
	KindTable:               {{"this", false}, {"alias", false}, {"schema", false}, {"catalog", false}, {"laterals", false}, {"joins", false}, {"pivots", false}, {"hints", false}, {"system_time", false}, {"version", false}, {"format", false}, {"pattern", false}, {"ordinality", false}, {"when", false}, {"only", false}, {"partition", false}, {"changes", false}, {"rows_from", false}, {"sample", false}, {"indexed", false}},
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
	// The pinned .reference/sqlglot-v30.12.0/sqlglot/expressions/dml.py:195-215
	// Insert has no "replace" arg. The mysql-replace extension appends it so
	// existing Insert arg ordering stays stable.
	KindInsert:     {{"hint", false}, {"with_", false}, {"is_function", false}, {"this", false}, {"expression", false}, {"conflict", false}, {"returning", false}, {"overwrite", false}, {"exists", false}, {"alternative", false}, {"where", false}, {"ignore", false}, {"by_name", false}, {"stored", false}, {"partition", false}, {"settings", false}, {"source", false}, {"default", false}, {"replace", false}},
	KindUpdate:     {{"with_", false}, {"this", false}, {"expressions", false}, {"from_", false}, {"where", false}, {"returning", false}, {"order", false}, {"limit", false}, {"options", false}, {"hint", false}},
	KindDelete:     {{"with_", false}, {"this", false}, {"using", false}, {"where", false}, {"returning", false}, {"order", false}, {"limit", false}, {"tables", false}, {"cluster", false}, {"hint", false}},
	KindMerge:      {{"this", true}, {"using", true}, {"on", false}, {"using_cond", false}, {"whens", true}, {"with_", false}, {"returning", false}},
	KindWhen:       {{"matched", true}, {"source", false}, {"condition", false}, {"then", true}},
	KindWhens:      {{"expressions", true}},
	KindOnConflict: {{"duplicate", false}, {"expressions", false}, {"action", false}, {"conflict_keys", false}, {"index_predicate", false}, {"constraint", false}, {"where", false}},
	KindReturning:  {{"expressions", true}, {"into", false}},
	KindInto:       {{"this", false}, {"temporary", false}, {"unlogged", false}, {"bulk_collect", false}, {"expressions", false}, {"kind", false}, {"charset", false}, {"columns", false}, {"fields_terminated", false}, {"optionally_enclosed", false}, {"enclosed", false}, {"escaped", false}, {"lines_starting", false}, {"lines_terminated", false}},
	// Arg order matches the order _parse_create sets kwargs (parser.py:2627-2643),
	// which is what upstream repr() reflects (args are dict-insertion-ordered), NOT
	// the Create.arg_types declaration order (ddl.py:40). with_ is never set by the
	// parser (it lives inside the CTAS expression), so its position is inert here.
	KindCreate:             {{"with_", false}, {"this", true}, {"kind", true}, {"replace", false}, {"refresh", false}, {"unique", false}, {"expression", false}, {"exists", false}, {"properties", false}, {"indexes", false}, {"no_schema_binding", false}, {"begin", false}, {"clone", false}, {"concurrently", false}, {"clustered", false}},
	KindSchema:             {{"this", false}, {"expressions", false}},
	KindCommand:            {{"this", true}, {"expression", false}},
	KindPivot:              {{"this", false}, {"alias", false}, {"expressions", false}, {"fields", false}, {"unpivot", false}, {"using", false}, {"group", false}, {"columns", false}, {"include_nulls", false}, {"default_on_null", false}, {"into", false}, {"with_", false}, {"identify_pivot_strings", false}, {"prefixed_pivot_columns", false}, {"pivot_column_naming", false}},
	KindLateral:            {{"this", true}, {"view", false}, {"outer", false}, {"alias", false}, {"cross_apply", false}, {"ordinality", false}},
	KindValues:             {{"expressions", true}, {"alias", false}, {"order", false}, {"limit", false}, {"offset", false}},
	KindColumnDef:          {{"this", true}, {"kind", false}, {"constraints", false}, {"exists", false}, {"position", false}, {"default", false}, {"output", false}, {"ordinality", false}},
	KindDataType:           {{"this", true}, {"expressions", false}, {"nested", false}, {"values", false}, {"kind", false}, {"nullable", false}, {"collate", false}},
	KindDataTypeParam:      {{"this", true}, {"expression", false}},
	KindCast:               {{"this", true}, {"to", true}, {"format", false}, {"safe", false}, {"action", false}, {"default", false}},
	KindTryCast:            {{"this", true}, {"to", true}, {"format", false}, {"safe", false}, {"action", false}, {"default", false}, {"requires_string", false}, {"null_on_text_overflow", false}, {"probe_date_format", false}},
	KindCastToStrType:      {{"this", true}, {"to", true}},
	KindExtract:            {{"this", true}, {"expression", true}},
	KindStrPosition:        {{"this", true}, {"substr", true}, {"position", false}, {"occurrence", false}, {"clamp_position", false}},
	KindSubstring:          {{"this", true}, {"start", false}, {"length", false}, {"zero_start", false}},
	KindTrim:               {{"this", true}, {"expression", false}, {"position", false}, {"collation", false}},
	KindCeil:               {{"this", true}, {"decimals", false}, {"to", false}},
	KindFloor:              {{"this", true}, {"decimals", false}, {"to", false}},
	KindGroupConcat:        {{"this", true}, {"separator", false}, {"on_overflow", false}},
	KindUnnest:             {{"expressions", true}, {"alias", false}, {"offset", false}, {"explode_array", false}},
	KindArray:              {{"expressions", false}, {"bracket_notation", false}, {"struct_name_inheritance", false}},
	KindBracket:            {{"this", true}, {"expressions", true}, {"offset", false}, {"safe", false}, {"returns_list_for_maps", false}, {"json_access", false}},
	KindLock:               {{"update", true}, {"expressions", false}, {"wait", false}, {"key", false}},
	KindPreWhere:           defaultArgTypes,
	KindCluster:            {{"expressions", true}},
	KindDistribute:         {{"this", false}, {"expressions", true}, {"siblings", false}},
	KindSort:               {{"this", false}, {"expressions", true}, {"siblings", false}},
	KindWithinGroup:        {{"this", true}, {"expression", false}},
	KindIgnoreNulls:        defaultArgTypes,
	KindRespectNulls:       defaultArgTypes,
	KindPivotAny:           {{"this", false}},
	KindPivotAlias:         {{"this", true}, {"alias", false}},
	KindInterval:           {{"this", false}, {"unit", false}},
	KindIntervalSpan:       {{"this", true}, {"expression", true}},
	KindJSONExtract:        {{"this", true}, {"expression", true}, {"only_json_types", false}, {"expressions", false}, {"variant_extract", false}, {"json_query", false}, {"option", false}, {"quote", false}, {"on_condition", false}, {"requires_json", false}, {"emits", false}},
	KindJSONExtractScalar:  {{"this", true}, {"expression", true}, {"only_json_types", false}, {"expressions", false}, {"json_type", false}, {"scalar_only", false}},
	KindJSONBExtract:       {{"this", true}, {"expression", true}},
	KindJSONBExtractScalar: {{"this", true}, {"expression", true}, {"json_type", false}},
	KindJSONCast:           {{"this", true}, {"to", true}, {"format", false}, {"safe", false}, {"action", false}, {"default", false}},
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
	KindSetItem:        {{"this", false}, {"expressions", false}, {"kind", false}, {"collate", false}, {"global_", false}, {"scope", false}},
	KindShow:           {{"this", true}, {"history", false}, {"terse", false}, {"target", false}, {"offset", false}, {"starts_with", false}, {"limit", false}, {"from_", false}, {"like", false}, {"where", false}, {"db", false}, {"scope", false}, {"scope_kind", false}, {"full", false}, {"mutex", false}, {"query", false}, {"channel", false}, {"global_", false}, {"log", false}, {"position", false}, {"types", false}, {"privileges", false}, {"for_table", false}, {"for_group", false}, {"for_user", false}, {"for_role", false}, {"into_outfile", false}, {"json", false}, {"iceberg", false}},
	KindUse:            {{"this", false}, {"expressions", false}, {"kind", false}},
	KindKill:           {{"this", true}, {"kind", false}},
	KindDescribe:       {{"this", true}, {"style", false}, {"kind", false}, {"properties", false}, {"expressions", false}, {"wrapped", false}, {"partition", false}, {"format", false}, {"as_json", false}, {"column", false}},
	KindLoadData:       {{"this", true}, {"local", false}, {"overwrite", false}, {"temp", false}, {"inpath", false}, {"files", false}, {"partition", false}, {"input_format", false}, {"serde", false}},
	KindTransaction:    {{"this", false}, {"modes", false}, {"mark", false}},
	KindCommit:         {{"chain", false}, {"this", false}, {"durability", false}},
	KindRollback:       {{"savepoint", false}, {"this", false}},
	KindSavepoint:      {{"this", true}, {"kind", false}},
	KindReset:          {{"this", false}},
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
	// Arg order matches _parse_unique's constructor kwargs (parser.py:7511-7517:
	// nulls, this, index_type, on_conflict, options) — the order repr() reflects —
	// rather than the class arg_types declaration order (constraints.py:168).
	KindUniqueColumnConstraint: {{"nulls", false}, {"this", false}, {"index_type", false}, {"on_conflict", false}, {"options", false}},
	KindCheckColumnConstraint:  {{"this", true}, {"enforced", false}},
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
	// Arg order matches _parse_index_params' constructor kwargs (parser.py: using,
	// columns, include, partition_by, where, with_storage, tablespace, on) — the
	// order repr() reflects — rather than the class arg_types declaration order.
	KindIndexParameters: {{"using", false}, {"columns", false}, {"include", false}, {"partition_by", false}, {"where", false}, {"with_storage", false}, {"tablespace", false}, {"on", false}},
	KindColumnPrefix:    {{"this", true}, {"expression", true}},
	// ColumnPosition/AddPartition/DropPartition (query.py:498,1941,1949).
	KindColumnPosition: {{"this", false}, {"position", true}},
	// Alter/Drop/AlterColumn/... family (ddl.py:241-401).
	// Arg order matches _parse_alter's constructor kwargs (parser.py: this, kind,
	// exists, actions, ...) — the order repr() reflects — rather than the class
	// arg_types declaration order (ddl.py: this, kind, actions, exists, ...).
	KindAlter:        {{"this", false}, {"kind", true}, {"exists", false}, {"actions", true}, {"only", false}, {"options", false}, {"cluster", false}, {"not_valid", false}, {"check", false}, {"cascade", false}, {"iceberg", false}},
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
	// TableSample (query.py:1724-1733): all nine keys optional.
	KindTableSample: {{"expressions", false}, {"method", false}, {"bucket_numerator", false}, {"bucket_denominator", false}, {"bucket_field", false}, {"percent", false}, {"rows", false}, {"size", false}, {"seed", false}},
	// Cache/Uncache (core.py:1583-1594).
	KindCache:   {{"this", true}, {"lazy", false}, {"options", false}, {"expression", false}},
	KindUncache: {{"this", true}, {"exists", false}},
	// Copy/CopyParameter/Credentials (dml.py:166-174, 162-163, 177-184).
	KindCopy:          {{"this", true}, {"kind", true}, {"files", false}, {"credentials", false}, {"format", false}, {"params", false}},
	KindCopyParameter: {{"this", true}, {"expression", false}, {"expressions", false}},
	KindCredentials:   {{"credentials", false}, {"encryption", false}, {"storage", false}, {"iam_role", false}, {"region", false}},
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
	// Affix/connector-operator cluster (see the Kind block comment above for exact
	// upstream line numbers). Collate inherits Binary's {"this": True, "expression": True}
	// (functions.py:171-172); BitwiseNot inherits Unary's default {"this": True}
	// (core.py:2242-2243).
	KindCollate:           {{"this", true}, {"expression", true}},
	KindBitwiseNot:        defaultArgTypes,
	KindCbrt:              defaultArgTypes,
	KindBitwiseLeftShift:  {{"this", true}, {"expression", true}, {"requires_int128", false}},
	KindBitwiseRightShift: {{"this", true}, {"expression", true}, {"requires_int128", false}},
	KindXor:               {{"this", true}, {"expression", true}, {"round_input", false}},
	// FROM-clause / DML tail cluster (dml.py:187/188, properties.py:405-419/452-453,
	// query.py:923/924,927/928).
	KindDirectory:                  {{"this", true}, {"local", false}, {"row_format", false}},
	KindRowFormatDelimitedProperty: {{"fields", false}, {"escaped", false}, {"collection_items", false}, {"map_keys", false}, {"lines", false}, {"null", false}, {"serde", false}},
	KindRowFormatSerdeProperty:     {{"this", true}, {"serde_properties", false}},
	KindSerdeProperties:            {{"expressions", true}, {"with_", false}},
	KindWithTableHint:              {{"expressions", true}},
	KindIndexTableHint:             {{"this", true}, {"expressions", false}, {"target", false}},
	// ddl cluster (see the Kind block comment above for the upstream class/line references).
	KindProperties:                {{"expressions", true}},
	KindUserDefinedFunction:       {{"this", true}, {"expressions", false}, {"wrapped", false}},
	KindReturnsProperty:           {{"this", false}, {"is_table", false}, {"table", false}, {"null", false}},
	KindLanguageProperty:          {{"this", true}},
	KindSqlSecurityProperty:       {{"this", false}},
	KindCalledOnNullInputProperty: {},
	KindStrictProperty:            {},
	KindStabilityProperty:         {{"this", true}},
	KindSetConfigProperty:         {{"this", true}},
	KindCharacterSetProperty:      {{"this", true}, {"default", false}},
	KindRowFormatProperty:         {{"this", true}},
	KindReturn:                    defaultArgTypes,
	KindIndex:                     {{"this", false}, {"table", false}, {"unique", false}, {"primary", false}, {"amp", false}, {"params", false}},
	KindOpclass:                   {{"this", true}, {"expression", true}},
	KindTriggerProperties:         {{"table", true}, {"timing", true}, {"events", true}, {"execute", true}, {"constraint", false}, {"referenced_table", false}, {"deferrable", false}, {"initially", false}, {"referencing", false}, {"for_each", false}, {"when", false}},
	KindTriggerExecute:            defaultArgTypes,
	KindTriggerEvent:              {{"this", true}, {"columns", false}},
	KindTriggerReferencing:        {{"old", false}, {"new", false}},
	// dialect-funcs cluster (see the Kind block comment above for exact upstream lines).
	KindCurrentDate:   {{"this", false}},
	KindCurrentTime:   {{"this", false}},
	KindCurrentSchema: {{"this", false}},
	KindDayOfMonth:    defaultArgTypes,
	KindDayOfWeek:     defaultArgTypes,
	KindDayOfYear:     defaultArgTypes,
	KindWeekOfYear:    defaultArgTypes,
	KindLower:         defaultArgTypes,
	KindUpper:         defaultArgTypes,
	KindLength:        {{"this", true}, {"binary", false}, {"encoding", false}},
	KindTrunc:         {{"this", true}, {"decimals", false}, {"fractions_supported", false}},
	KindOverlay:       {{"this", true}, {"expression", true}, {"from_", true}, {"for_", false}},
	KindVariadic:      defaultArgTypes,
	// slice-strings cluster: Slice (core.py:2017-2018) - this/expression/step all optional.
	// National (query.py:585-586) has no arg_types override, i.e. inherits the base
	// Expression default (mirroring e.g. KindLogicalOr above).
	KindSlice:    {{"this", false}, {"expression", false}, {"step", false}},
	KindNational: defaultArgTypes,
	// range-ops cluster (parser.go parseRange/parseIs/columnOperators). All the plain
	// Binary subclasses below inherit Binary's {"this": True, "expression": True}
	// verbatim (core.py:1623/1624); only ArrayOverlaps/JSON/Operator/MatchAgainst/
	// JSONArrayContains override with extra fields.
	KindGlob:                    {{"this", true}, {"expression", true}},
	KindOverlaps:                {{"this", true}, {"expression", true}},
	KindRegexpILike:             {{"this", true}, {"expression", true}, {"flag", false}},
	KindAdjacent:                {{"this", true}, {"expression", true}},
	KindArrayContainsAll:        {{"this", true}, {"expression", true}},
	KindArrayContainedBy:        {{"this", true}, {"expression", true}},
	KindArrayOverlaps:           {{"this", true}, {"expression", true}, {"null_safe", false}},
	KindJSONBContains:           {{"this", true}, {"expression", true}},
	KindJSONBContainsAllTopKeys: {{"this", true}, {"expression", true}},
	KindJSONBContainsAnyTopKeys: {{"this", true}, {"expression", true}},
	KindJSONBDeleteAtPath:       {{"this", true}, {"expression", true}},
	KindJSONBPathExists:         {{"this", true}, {"expression", true}},
	KindJSON:                    {{"this", false}, {"with_", false}, {"unique", false}},
	KindOperator:                {{"this", true}, {"operator", true}, {"expression", true}},
	KindMatchAgainst:            {{"this", true}, {"expressions", true}, {"modifier", false}},
	KindJSONArrayContains:       {{"this", true}, {"expression", true}, {"json_type", false}},
	KindSoundex:                 defaultArgTypes,
	// residual-tail cluster: see the KindBitString..KindDistanceNd const-block comment
	// above. BitString has no arg_types override (defaultArgTypes); HexString/ByteString
	// add their own optional flag (query.py:480-487); SessionParameter/PropertyEQ/
	// Distance/DistanceNd transcribed from core.py:1837-1838, 2150-2159.
	KindBitString:        defaultArgTypes,
	KindHexString:        {{"this", true}, {"is_integer", false}},
	KindByteString:       {{"this", true}, {"is_bytes", false}},
	KindSessionParameter: {{"this", true}, {"kind", false}},
	KindPropertyEQ:       {{"this", true}, {"expression", true}},
	KindDistance:         {{"this", true}, {"expression", true}},
	KindDistanceNd:       {{"this", true}, {"expression", true}},
	// KindLag/KindLead (aggregate.py:150-163): {"this": True, "offset": False, "default": False}.
	KindLag:  {{"this", true}, {"offset", false}, {"default", false}},
	KindLead: {{"this", true}, {"offset", false}, {"default", false}},
	// KindConcat (string.py:29-31): {"expressions": True, "safe": False, "coalesce": False}.
	KindConcat: {{"expressions", true}, {"safe", false}, {"coalesce", false}},
	// Fidelity properties (properties.py:12-553), constraints (constraints.py:76-198),
	// and query nodes (query.py:589-594,1879-1938). Pass-through subclasses inherit
	// Expression's default {"this": True}; explicit empty dictionaries remain empty.
	KindProperty:                     {{"this", true}, {"value", true}},
	KindAlgorithmProperty:            {{"this", true}},
	KindAutoIncrementProperty:        {{"this", true}},
	KindCollateProperty:              {{"this", true}, {"default", false}},
	KindDefinerProperty:              {{"this", true}},
	KindEngineProperty:               {{"this", true}},
	KindInheritsProperty:             {{"expressions", true}},
	KindLikeProperty:                 {{"this", true}, {"expressions", false}},
	KindLockProperty:                 {{"this", true}},
	KindLockingProperty:              {{"this", false}, {"kind", true}, {"for_or_in", false}, {"lock_type", true}, {"override", false}},
	KindMaterializedProperty:         {{"this", false}},
	KindNoPrimaryIndexProperty:       {},
	KindOnCommitProperty:             {{"delete", false}},
	KindPartitionedByProperty:        {{"this", true}},
	KindPartitionByRangeProperty:     {{"partition_expressions", true}, {"create_expressions", true}},
	KindPartitionByListProperty:      {{"partition_expressions", true}, {"create_expressions", true}},
	KindPartitionList:                {{"this", true}, {"expressions", true}},
	KindPartitionBoundSpec:           {{"this", false}, {"expression", false}, {"from_expressions", false}, {"to_expressions", false}},
	KindPartitionedOfProperty:        {{"this", true}, {"expression", true}},
	KindSchemaCommentProperty:        {{"this", true}},
	KindSqlReadWriteProperty:         {{"this", true}},
	KindTemporaryProperty:            {{"this", false}},
	KindUnloggedProperty:             {},
	KindWithDataProperty:             {{"no", true}, {"statistics", false}},
	KindPartitionRange:               {{"this", true}, {"expression", false}, {"expressions", false}},
	KindAnalyzeHistogram:             {{"this", true}, {"expressions", true}, {"expression", false}, {"update_options", false}},
	KindAnalyzeWith:                  {{"expressions", true}},
	KindUsingData:                    defaultArgTypes,
	KindCompressColumnConstraint:     {{"this", false}},
	KindDateFormatColumnConstraint:   defaultArgTypes,
	KindExcludeColumnConstraint:      defaultArgTypes,
	KindInlineLengthColumnConstraint: defaultArgTypes,
	KindTitleColumnConstraint:        defaultArgTypes,
	KindUppercaseColumnConstraint:    {},
	KindWithOperator:                 {{"this", true}, {"op", true}},
	KindInOutColumnConstraint:        {{"input_", false}, {"output", false}, {"variadic", false}},
	// presto cluster: arg_types mirrored 1:1 from the upstream classes (dict order drives
	// FromArgList positional mapping). See expressions/presto_nodes.go for per-Kind citations.
	KindAnyValue:         defaultArgTypes,                                                                                                                      // aggregate.py:17 (pass -> {"this": True})
	KindApproxQuantile:   {{"this", true}, {"quantile", true}, {"accuracy", false}, {"weight", false}, {"error_tolerance", false}},                             // aggregate.py:234
	KindArrayUniqueAgg:   defaultArgTypes,                                                                                                                      // aggregate.py:83 (pass)
	KindDayOfWeekIso:     defaultArgTypes,                                                                                                                      // temporal.py:217 (pass)
	KindDecode:           {{"this", true}, {"charset", true}, {"replace", false}},                                                                              // string.py:285
	KindEncode:           {{"this", true}, {"charset", true}},                                                                                                  // string.py:289
	KindJSONFormat:       {{"this", false}, {"options", false}, {"is_json", false}, {"to_json", false}},                                                        // json.py:145
	KindLevenshtein:      {{"this", true}, {"expression", false}, {"ins_cost", false}, {"del_cost", false}, {"sub_cost", false}, {"max_dist", false}},          // string.py:74
	KindMD5Digest:        {{"this", true}, {"expressions", false}},                                                                                             // string.py:540 (is_var_len_args)
	KindSHA2:             {{"this", true}, {"length", false}},                                                                                                  // string.py:562
	KindStrToMap:         {{"this", true}, {"pair_delim", false}, {"key_value_delim", false}, {"duplicate_resolution_callback", false}},                        // string.py:203
	KindTimeToUnix:       defaultArgTypes,                                                                                                                      // temporal.py:484 (pass)
	KindUnixToTime:       {{"this", true}, {"scale", false}, {"zone", false}, {"hours", false}, {"minutes", false}, {"format", false}, {"target_type", false}}, // temporal.py:532
	KindUnhex:            {{"this", true}, {"expression", false}},                                                                                              // string.py:405
	KindArraySlice:       {{"this", true}, {"start", true}, {"end", false}, {"step", false}, {"zero_based", false}},                                            // array.py:85
	KindCurrentTimestamp: {{"this", false}, {"sysdate", false}},                                                                                                // temporal.py:37
	KindCurrentUser:      {{"this", false}},                                                                                                                    // functions.py:309
	KindCurrentRole:      {},                                                                                                                                   // functions.py:277 (empty arg_types)
	KindSessionUser:      {},                                                                                                                                   // functions.py:325 (empty arg_types)
	KindLocaltime:        {{"this", false}},                                                                                                                    // temporal.py:49
	KindLocaltimestamp:   {{"this", false}},                                                                                                                    // temporal.py:53
	KindUnicodeString:    {{"this", true}, {"escape", false}},                                                                                                  // query.py:494
	// trino cluster: arg_types mirror the pinned declarations exactly. In particular,
	// CurrentCatalog and CurrentVersion have genuinely empty arg maps, not optional `this`.
	KindCurrentCatalog:           {},                                      // functions.py:249-250
	KindCurrentVersion:           {},                                      // functions.py:317-318
	KindJSONExtractQuote:         {{"option", true}, {"scalar", false}},   // query.py:2062-2066
	KindOverflowTruncateBehavior: {{"this", false}, {"with_count", true}}, // query.py:1961-1962
	KindRefresh:                  {{"this", true}, {"kind", true}},        // core.py:1596-1597
	KindArrayFirst:               {{"this", true}, {"expression", false}}, // array.py:146-147
	// hive cluster: arg_types preserve upstream declaration order because FromArgList
	// maps function arguments positionally using these rows.
	KindExternalProperty:       {{"this", false}},
	KindClusteredByProperty:    {{"expressions", true}, {"sorted_by", false}, {"buckets", true}},
	KindLocationProperty:       {{"this", true}},
	KindStorageHandlerProperty: {{"this", true}},
	KindUsingProperty:          {{"this", true}, {"kind", true}},
	KindInputOutputFormat:      {{"input_format", false}, {"output_format", false}},
	KindQueryTransform:         {{"expressions", true}, {"command_script", true}, {"schema", false}, {"row_format_before", false}, {"record_writer", false}, {"row_format_after", false}, {"record_reader", false}},
	KindTransform:              {{"this", true}, {"expression", true}},
	KindToBase64:               defaultArgTypes,
	KindFromBase64:             defaultArgTypes,
	KindTsOrDsAdd:              {{"this", true}, {"expression", true}, {"unit", false}, {"return_type", false}},
	KindTsOrDsToDate:           {{"this", true}, {"format", false}, {"safe", false}},
	KindFirst:                  {{"this", true}, {"expression", false}},
	KindFirstValue:             defaultArgTypes,
	KindLast:                   {{"this", true}, {"expression", false}},
	KindLastValue:              defaultArgTypes,
	KindRegexpExtract:          {{"this", true}, {"expression", true}, {"position", false}, {"occurrence", false}, {"parameters", false}, {"group", false}, {"null_if_pos_overflow", false}},
	KindRegexpExtractAll:       {{"this", true}, {"expression", true}, {"group", false}, {"parameters", false}, {"position", false}, {"occurrence", false}},
	KindTimestampTrunc:         {{"this", true}, {"unit", true}, {"zone", false}, {"input_type_preserved", false}},
	KindUnixToStr:              {{"this", true}, {"format", false}},
	KindTimeToStr:              {{"this", true}, {"format", true}, {"culture", false}, {"zone", false}},
	KindStarMap:                defaultArgTypes,
	KindVarMap:                 {{"keys", true}, {"values", true}},
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
	// Copy is `class Copy(Expression, DML)` (dml.py:166); CopyParameter/Credentials are
	// plain Expression subclasses with no mixin, so they get no row here (matching e.g.
	// KindTableAlias above).
	KindCopy: TraitDML,
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
	// Affix/connector-operator cluster (see the Kind block comment above). Collate mirrors
	// DPipe's Binary+Func mixins; BitwiseNot mirrors Neg/Not's Unary mixin;
	// BitwiseLeftShift/BitwiseRightShift mirror BitwiseAnd's plain Binary mixin (no Func);
	// Xor mirrors And/Or's Connector+Func mixins exactly.
	KindCollate:           TraitCondition | TraitBinary | TraitFunc,
	KindBitwiseNot:        TraitCondition | TraitUnary,
	KindCbrt:              TraitCondition | TraitFunc,
	KindBitwiseLeftShift:  TraitCondition | TraitBinary,
	KindBitwiseRightShift: TraitCondition | TraitBinary,
	KindXor:               TraitCondition | TraitBinary | TraitConnector | TraitFunc,
	// dialect-funcs cluster (see the Kind block comment above for exact upstream lines).
	// KindVariadic is deliberately omitted: exp.Variadic is a plain `pass` Expression with
	// no Func/Condition mixin upstream (query.py:2099), so it gets no trait bits, matching
	// e.g. KindTableAlias/KindLambda above.
	KindCurrentDate:   TraitCondition | TraitFunc,
	KindCurrentTime:   TraitCondition | TraitFunc,
	KindCurrentSchema: TraitCondition | TraitFunc,
	KindDayOfMonth:    TraitCondition | TraitFunc,
	KindDayOfWeek:     TraitCondition | TraitFunc,
	KindDayOfYear:     TraitCondition | TraitFunc,
	KindWeekOfYear:    TraitCondition | TraitFunc,
	KindLower:         TraitCondition | TraitFunc,
	KindUpper:         TraitCondition | TraitFunc,
	KindLength:        TraitCondition | TraitFunc,
	KindTrunc:         TraitCondition | TraitFunc,
	KindOverlay:       TraitCondition | TraitFunc,
	// slice-strings cluster: both KindSlice (`class Slice(Expression)`, core.py:2017) and
	// KindNational (`class National(Expression)`, query.py:585) are plain Expression subclasses
	// with no Condition mixin upstream, so neither gets a traitsOf row (matching e.g. KindLambda
	// above). National is still a primitive value (primitive[KindNational]=true below), which is
	// what its `N'...'` literal round-trip relies on - the Condition mixin is not.
	// range-ops cluster: see the KindGlob..KindSoundex const-block comment above for
	// the exact upstream class each mirrors. KindJSON (query.py:1965, plain
	// Expression) gets no row, matching e.g. KindTableAlias.
	KindGlob:                    TraitCondition | TraitBinary | TraitPredicate,
	KindOverlaps:                TraitCondition | TraitBinary,
	KindRegexpILike:             TraitCondition | TraitBinary | TraitFunc,
	KindAdjacent:                TraitCondition | TraitBinary,
	KindArrayContainsAll:        TraitCondition | TraitBinary | TraitFunc,
	KindArrayContainedBy:        TraitCondition | TraitBinary | TraitFunc,
	KindArrayOverlaps:           TraitCondition | TraitBinary | TraitFunc,
	KindJSONBContains:           TraitCondition | TraitBinary | TraitFunc,
	KindJSONBContainsAllTopKeys: TraitCondition | TraitBinary | TraitFunc,
	KindJSONBContainsAnyTopKeys: TraitCondition | TraitBinary | TraitFunc,
	KindJSONBDeleteAtPath:       TraitCondition | TraitBinary | TraitFunc,
	KindJSONBPathExists:         TraitCondition | TraitBinary | TraitPredicate | TraitFunc,
	KindOperator:                TraitCondition | TraitBinary,
	KindMatchAgainst:            TraitCondition | TraitFunc,
	KindJSONArrayContains:       TraitCondition | TraitBinary | TraitPredicate | TraitFunc,
	KindSoundex:                 TraitCondition | TraitFunc,
	// residual-tail cluster: BitString/HexString/ByteString are `Expression, Condition`
	// (query.py:471-491); SessionParameter is `Expression, Condition` (core.py:1837);
	// PropertyEQ/Distance/DistanceNd are `Expression, Binary` and Binary IS-A Condition
	// (core.py:1623), so they get the same TraitCondition|TraitBinary combo as Add/Sub.
	KindBitString:        TraitCondition,
	KindHexString:        TraitCondition,
	KindByteString:       TraitCondition,
	KindSessionParameter: TraitCondition,
	KindPropertyEQ:       TraitCondition | TraitBinary,
	KindDistance:         TraitCondition | TraitBinary,
	KindDistanceNd:       TraitCondition | TraitBinary,
	KindLag:              TraitCondition | TraitFunc | TraitAggFunc,
	KindLead:             TraitCondition | TraitFunc | TraitAggFunc,
	KindConcat:           TraitCondition | TraitFunc,
	// presto cluster: Func(Condition) subclasses get TraitCondition|TraitFunc (so they
	// render via generator.functionFallbackSQL); AggFunc subclasses add TraitAggFunc
	// (core.py:1574/1641/1694). KindUnicodeString is Expression,Condition (query.py:494) -
	// NO TraitFunc; its dispatch is supplied by the generator, so it gets only TraitCondition.
	KindAnyValue:         TraitCondition | TraitFunc | TraitAggFunc,
	KindApproxQuantile:   TraitCondition | TraitFunc | TraitAggFunc,
	KindArrayUniqueAgg:   TraitCondition | TraitFunc | TraitAggFunc,
	KindDayOfWeekIso:     TraitCondition | TraitFunc,
	KindDecode:           TraitCondition | TraitFunc,
	KindEncode:           TraitCondition | TraitFunc,
	KindJSONFormat:       TraitCondition | TraitFunc,
	KindLevenshtein:      TraitCondition | TraitFunc,
	KindMD5Digest:        TraitCondition | TraitFunc,
	KindSHA2:             TraitCondition | TraitFunc,
	KindStrToMap:         TraitCondition | TraitFunc,
	KindTimeToUnix:       TraitCondition | TraitFunc,
	KindUnixToTime:       TraitCondition | TraitFunc,
	KindUnhex:            TraitCondition | TraitFunc,
	KindArraySlice:       TraitCondition | TraitFunc,
	KindCurrentTimestamp: TraitCondition | TraitFunc,
	KindCurrentUser:      TraitCondition | TraitFunc,
	KindCurrentRole:      TraitCondition | TraitFunc,
	KindSessionUser:      TraitCondition | TraitFunc,
	KindLocaltime:        TraitCondition | TraitFunc,
	KindLocaltimestamp:   TraitCondition | TraitFunc,
	KindUnicodeString:    TraitCondition,
	// trino cluster: only the Func subclasses carry traits. JSONExtractQuote,
	// OverflowTruncateBehavior, and Refresh are plain Expression subclasses upstream.
	KindCurrentCatalog: TraitCondition | TraitFunc,
	KindCurrentVersion: TraitCondition | TraitFunc,
	KindArrayFirst:     TraitCondition | TraitFunc,
	// hive cluster: Func subclasses are Conditions with TraitFunc; aggregate
	// subclasses additionally carry TraitAggFunc. Property/query nodes have no traits.
	KindTransform:        TraitCondition | TraitFunc,
	KindToBase64:         TraitCondition | TraitFunc,
	KindFromBase64:       TraitCondition | TraitFunc,
	KindTsOrDsAdd:        TraitCondition | TraitFunc,
	KindTsOrDsToDate:     TraitCondition | TraitFunc,
	KindFirst:            TraitCondition | TraitFunc | TraitAggFunc,
	KindFirstValue:       TraitCondition | TraitFunc | TraitAggFunc,
	KindLast:             TraitCondition | TraitFunc | TraitAggFunc,
	KindLastValue:        TraitCondition | TraitFunc | TraitAggFunc,
	KindRegexpExtract:    TraitCondition | TraitFunc,
	KindRegexpExtractAll: TraitCondition | TraitFunc,
	KindTimestampTrunc:   TraitCondition | TraitFunc,
	KindUnixToStr:        TraitCondition | TraitFunc,
	KindTimeToStr:        TraitCondition | TraitFunc,
	KindStarMap:          TraitCondition | TraitFunc,
	KindVarMap:           TraitCondition | TraitFunc,
}

var primitive = map[Kind]bool{
	KindLiteral:    true,
	KindIdentifier: true,
	KindVar:        true,
	KindBoolean:    true,
	// slice-strings cluster: National is_primitive=True (query.py:585-586).
	KindNational: true,
	// presto cluster: UnicodeString is_primitive=True (query.py:494), like National -
	// its `U&'...'` literal round-trip relies on the primitive flag.
	KindUnicodeString: true,
	// residual-tail cluster: BitString/HexString/ByteString is_primitive=True (query.py:
	// 471-487).
	KindBitString:  true,
	KindHexString:  true,
	KindByteString: true,
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
	KindSavepoint:                           "Savepoint",
	KindReset:                               "Reset",
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
	KindTableSample:                         "TableSample",
	KindAtTimeZone:                          "AtTimeZone",
	KindPseudoType:                          "PseudoType",
	KindObjectIdentifier:                    "ObjectIdentifier",
	KindStruct:                              "Struct",
	KindCache:                               "Cache",
	KindUncache:                             "Uncache",
	KindCopy:                                "Copy",
	KindCopyParameter:                       "CopyParameter",
	KindCredentials:                         "Credentials",
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
	KindCollate:                             "Collate",
	KindBitwiseNot:                          "BitwiseNot",
	KindCbrt:                                "Cbrt",
	KindBitwiseLeftShift:                    "BitwiseLeftShift",
	KindBitwiseRightShift:                   "BitwiseRightShift",
	KindXor:                                 "Xor",
	KindDirectory:                           "Directory",
	KindRowFormatDelimitedProperty:          "RowFormatDelimitedProperty",
	KindRowFormatSerdeProperty:              "RowFormatSerdeProperty",
	KindSerdeProperties:                     "SerdeProperties",
	KindWithTableHint:                       "WithTableHint",
	KindIndexTableHint:                      "IndexTableHint",
	KindProperties:                          "Properties",
	KindUserDefinedFunction:                 "UserDefinedFunction",
	KindReturnsProperty:                     "ReturnsProperty",
	KindLanguageProperty:                    "LanguageProperty",
	KindSqlSecurityProperty:                 "SqlSecurityProperty",
	KindCalledOnNullInputProperty:           "CalledOnNullInputProperty",
	KindStrictProperty:                      "StrictProperty",
	KindStabilityProperty:                   "StabilityProperty",
	KindSetConfigProperty:                   "SetConfigProperty",
	KindCharacterSetProperty:                "CharacterSetProperty",
	KindRowFormatProperty:                   "RowFormatProperty",
	KindReturn:                              "Return",
	KindIndex:                               "Index",
	KindOpclass:                             "Opclass",
	KindTriggerProperties:                   "TriggerProperties",
	KindTriggerExecute:                      "TriggerExecute",
	KindTriggerEvent:                        "TriggerEvent",
	KindTriggerReferencing:                  "TriggerReferencing",
	KindCurrentDate:                         "CurrentDate",
	KindCurrentTime:                         "CurrentTime",
	KindCurrentSchema:                       "CurrentSchema",
	KindDayOfMonth:                          "DayOfMonth",
	KindDayOfWeek:                           "DayOfWeek",
	KindDayOfYear:                           "DayOfYear",
	KindWeekOfYear:                          "WeekOfYear",
	KindLower:                               "Lower",
	KindUpper:                               "Upper",
	KindLength:                              "Length",
	KindTrunc:                               "Trunc",
	KindOverlay:                             "Overlay",
	KindVariadic:                            "Variadic",
	KindSlice:                               "Slice",
	KindNational:                            "National",
	KindGlob:                                "Glob",
	KindOverlaps:                            "Overlaps",
	KindRegexpILike:                         "RegexpILike",
	KindAdjacent:                            "Adjacent",
	KindArrayContainsAll:                    "ArrayContainsAll",
	KindArrayContainedBy:                    "ArrayContainedBy",
	KindArrayOverlaps:                       "ArrayOverlaps",
	KindJSONBContains:                       "JSONBContains",
	KindJSONBContainsAllTopKeys:             "JSONBContainsAllTopKeys",
	KindJSONBContainsAnyTopKeys:             "JSONBContainsAnyTopKeys",
	KindJSONBDeleteAtPath:                   "JSONBDeleteAtPath",
	KindJSONBPathExists:                     "JSONBPathExists",
	KindJSON:                                "JSON",
	KindOperator:                            "Operator",
	KindMatchAgainst:                        "MatchAgainst",
	KindJSONArrayContains:                   "JSONArrayContains",
	KindSoundex:                             "Soundex",
	KindBitString:                           "BitString",
	KindHexString:                           "HexString",
	KindByteString:                          "ByteString",
	KindSessionParameter:                    "SessionParameter",
	KindPropertyEQ:                          "PropertyEQ",
	KindDistance:                            "Distance",
	KindDistanceNd:                          "DistanceNd",
	KindLag:                                 "Lag",
	KindLead:                                "Lead",
	KindConcat:                              "Concat",
	KindProperty:                            "Property",
	KindAlgorithmProperty:                   "AlgorithmProperty",
	KindAutoIncrementProperty:               "AutoIncrementProperty",
	KindCollateProperty:                     "CollateProperty",
	KindDefinerProperty:                     "DefinerProperty",
	KindEngineProperty:                      "EngineProperty",
	KindInheritsProperty:                    "InheritsProperty",
	KindLikeProperty:                        "LikeProperty",
	KindLockProperty:                        "LockProperty",
	KindLockingProperty:                     "LockingProperty",
	KindMaterializedProperty:                "MaterializedProperty",
	KindNoPrimaryIndexProperty:              "NoPrimaryIndexProperty",
	KindOnCommitProperty:                    "OnCommitProperty",
	KindPartitionedByProperty:               "PartitionedByProperty",
	KindPartitionByRangeProperty:            "PartitionByRangeProperty",
	KindPartitionByListProperty:             "PartitionByListProperty",
	KindPartitionList:                       "PartitionList",
	KindPartitionBoundSpec:                  "PartitionBoundSpec",
	KindPartitionedOfProperty:               "PartitionedOfProperty",
	KindSchemaCommentProperty:               "SchemaCommentProperty",
	KindSqlReadWriteProperty:                "SqlReadWriteProperty",
	KindTemporaryProperty:                   "TemporaryProperty",
	KindUnloggedProperty:                    "UnloggedProperty",
	KindWithDataProperty:                    "WithDataProperty",
	KindPartitionRange:                      "PartitionRange",
	KindAnalyzeHistogram:                    "AnalyzeHistogram",
	KindAnalyzeWith:                         "AnalyzeWith",
	KindUsingData:                           "UsingData",
	KindCompressColumnConstraint:            "CompressColumnConstraint",
	KindDateFormatColumnConstraint:          "DateFormatColumnConstraint",
	KindExcludeColumnConstraint:             "ExcludeColumnConstraint",
	KindInlineLengthColumnConstraint:        "InlineLengthColumnConstraint",
	KindTitleColumnConstraint:               "TitleColumnConstraint",
	KindUppercaseColumnConstraint:           "UppercaseColumnConstraint",
	KindWithOperator:                        "WithOperator",
	KindInOutColumnConstraint:               "InOutColumnConstraint",
	// presto cluster: class names match the upstream Presto FUNCTIONS canonical class.
	KindAnyValue:         "AnyValue",
	KindApproxQuantile:   "ApproxQuantile",
	KindArrayUniqueAgg:   "ArrayUniqueAgg",
	KindDayOfWeekIso:     "DayOfWeekIso",
	KindDecode:           "Decode",
	KindEncode:           "Encode",
	KindJSONFormat:       "JSONFormat",
	KindLevenshtein:      "Levenshtein",
	KindMD5Digest:        "MD5Digest",
	KindSHA2:             "SHA2",
	KindStrToMap:         "StrToMap",
	KindTimeToUnix:       "TimeToUnix",
	KindUnixToTime:       "UnixToTime",
	KindUnhex:            "Unhex",
	KindArraySlice:       "ArraySlice",
	KindCurrentTimestamp: "CurrentTimestamp",
	KindCurrentUser:      "CurrentUser",
	KindCurrentRole:      "CurrentRole",
	KindSessionUser:      "SessionUser",
	KindLocaltime:        "Localtime",
	KindLocaltimestamp:   "Localtimestamp",
	KindUnicodeString:    "UnicodeString",
	// trino cluster: exact upstream PascalCase class names.
	KindCurrentCatalog:           "CurrentCatalog",
	KindCurrentVersion:           "CurrentVersion",
	KindJSONExtractQuote:         "JSONExtractQuote",
	KindOverflowTruncateBehavior: "OverflowTruncateBehavior",
	KindRefresh:                  "Refresh",
	KindArrayFirst:               "ArrayFirst",
	// hive cluster: exact upstream class names.
	KindExternalProperty:       "ExternalProperty",
	KindClusteredByProperty:    "ClusteredByProperty",
	KindLocationProperty:       "LocationProperty",
	KindStorageHandlerProperty: "StorageHandlerProperty",
	KindUsingProperty:          "UsingProperty",
	KindInputOutputFormat:      "InputOutputFormat",
	KindQueryTransform:         "QueryTransform",
	KindTransform:              "Transform",
	KindToBase64:               "ToBase64",
	KindFromBase64:             "FromBase64",
	KindTsOrDsAdd:              "TsOrDsAdd",
	KindTsOrDsToDate:           "TsOrDsToDate",
	KindFirst:                  "First",
	KindFirstValue:             "FirstValue",
	KindLast:                   "Last",
	KindLastValue:              "LastValue",
	KindRegexpExtract:          "RegexpExtract",
	KindRegexpExtractAll:       "RegexpExtractAll",
	KindTimestampTrunc:         "TimestampTrunc",
	KindUnixToStr:              "UnixToStr",
	KindTimeToStr:              "TimeToStr",
	KindStarMap:                "StarMap",
	KindVarMap:                 "VarMap",
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
	KindConcat:            true,
	// presto cluster: MD5Digest is_var_len_args=True (string.py:540).
	KindMD5Digest: true,
	// hive cluster: VarMap is_var_len_args=True (array.py:339-341).
	KindVarMap: true,
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
