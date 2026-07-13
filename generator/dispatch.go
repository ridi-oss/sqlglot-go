package generator

import "github.com/ridi/sqlglot-go/expressions"

// dispatch is declared with an empty-map var initializer (rather than left nil and only
// ever assigned inside init()) so it's guaranteed to be a valid, non-nil map as soon as
// package-level var initialization completes and BEFORE any init() func runs (Go var
// initializers always precede init() funcs, regardless of file). That lets each statement-
// family part register its own entries from its own func init() via plain key assignment,
// safely, regardless of init() run order across files (plan's "seam" for parallel,
// disjoint-file family parts). Embedding the entries directly in this initializer instead
// (a map literal with (*Generator).negSQL etc. as values) is not viable: those methods
// transitively call sqlKey -> gen, which reads dispatch, and Go's package dependency
// analysis flags that as an initialization cycle - so the entries are populated by
// assignment in init() instead.
var dispatch = map[expressions.Kind]func(*Generator, expressions.Expression) string{}

func init() {
	dispatch[expressions.KindExpression] = (*Generator).expressionSQL
	dispatch[expressions.KindColumn] = (*Generator).columnSQL
	dispatch[expressions.KindLiteral] = (*Generator).literalSQL
	dispatch[expressions.KindIdentifier] = (*Generator).identifierSQL
	dispatch[expressions.KindStar] = (*Generator).starSQL
	dispatch[expressions.KindAlias] = (*Generator).aliasSQL
	dispatch[expressions.KindAliases] = (*Generator).aliasesSQL
	dispatch[expressions.KindDot] = (*Generator).dotSQL
	dispatch[expressions.KindNull] = (*Generator).nullSQL
	dispatch[expressions.KindBoolean] = (*Generator).booleanSQL
	dispatch[expressions.KindVar] = (*Generator).varSQL
	dispatch[expressions.KindParen] = (*Generator).parenSQL
	dispatch[expressions.KindNeg] = (*Generator).negSQL
	dispatch[expressions.KindNot] = (*Generator).notSQL
	dispatch[expressions.KindAdd] = (*Generator).addSQL
	dispatch[expressions.KindSub] = (*Generator).subSQL
	dispatch[expressions.KindMul] = (*Generator).mulSQL
	dispatch[expressions.KindDiv] = (*Generator).divSQL
	dispatch[expressions.KindMod] = (*Generator).modSQL
	dispatch[expressions.KindEQ] = (*Generator).eqSQL
	dispatch[expressions.KindNEQ] = (*Generator).neqSQL
	dispatch[expressions.KindNullSafeEQ] = (*Generator).nullSafeEQSQL
	dispatch[expressions.KindGT] = (*Generator).gtSQL
	dispatch[expressions.KindGTE] = (*Generator).gteSQL
	dispatch[expressions.KindLT] = (*Generator).ltSQL
	dispatch[expressions.KindLTE] = (*Generator).lteSQL
	dispatch[expressions.KindAnd] = (*Generator).andSQL
	dispatch[expressions.KindOr] = (*Generator).orSQL
	dispatch[expressions.KindBitwiseAnd] = (*Generator).bitwiseAndSQL
	dispatch[expressions.KindBitwiseOr] = (*Generator).bitwiseOrSQL
	dispatch[expressions.KindBitwiseXor] = (*Generator).bitwiseXorSQL
	dispatch[expressions.KindDPipe] = (*Generator).dpipeSQL
	dispatch[expressions.KindBetween] = (*Generator).betweenSQL
	dispatch[expressions.KindIs] = (*Generator).isSQL
	dispatch[expressions.KindLike] = (*Generator).likeSQL
	dispatch[expressions.KindILike] = (*Generator).ilikeSQL
	dispatch[expressions.KindSimilarTo] = (*Generator).similarToSQL
	dispatch[expressions.KindEscape] = (*Generator).escapeSQL
	dispatch[expressions.KindIn] = (*Generator).inSQL
	dispatch[expressions.KindSelect] = (*Generator).selectSQL
	dispatch[expressions.KindFrom] = (*Generator).fromSQL
	dispatch[expressions.KindJoin] = (*Generator).joinSQL
	dispatch[expressions.KindTable] = (*Generator).tableSQL
	dispatch[expressions.KindTableColumn] = (*Generator).tableColumnSQL
	dispatch[expressions.KindTableAlias] = (*Generator).tableAliasSQL
	dispatch[expressions.KindWhere] = (*Generator).whereSQL
	dispatch[expressions.KindGroup] = (*Generator).groupSQL
	dispatch[expressions.KindOrder] = (*Generator).orderSQL
	dispatch[expressions.KindLimit] = (*Generator).limitSQL
	dispatch[expressions.KindOffset] = (*Generator).offsetSQL
	dispatch[expressions.KindHint] = (*Generator).hintSQL
	dispatch[expressions.KindBlock] = (*Generator).blockSQL
	dispatch[expressions.KindPlaceholder] = (*Generator).placeholderSQL
	dispatch[expressions.KindTuple] = (*Generator).tupleSQL
	dispatch[expressions.KindWith] = (*Generator).withSQL
	dispatch[expressions.KindCTE] = (*Generator).cteSQL
	dispatch[expressions.KindRecursiveWithSearch] = (*Generator).recursiveWithSearchSQL
	dispatch[expressions.KindUnion] = (*Generator).setOperationsSQL
	dispatch[expressions.KindExcept] = (*Generator).setOperationsSQL
	dispatch[expressions.KindIntersect] = (*Generator).setOperationsSQL
	dispatch[expressions.KindSubquery] = (*Generator).subquerySQL
	dispatch[expressions.KindHaving] = (*Generator).havingSQL
	dispatch[expressions.KindQualify] = (*Generator).qualifySQL
	dispatch[expressions.KindCube] = (*Generator).cubeSQL
	dispatch[expressions.KindRollup] = (*Generator).rollupSQL
	dispatch[expressions.KindGroupingSets] = (*Generator).groupingSetsSQL
	dispatch[expressions.KindOrdered] = (*Generator).orderedSQL
	dispatch[expressions.KindDistinct] = (*Generator).distinctSQL
	dispatch[expressions.KindWindow] = (*Generator).windowSQL
	dispatch[expressions.KindWindowSpec] = (*Generator).windowSpecSQL
	dispatch[expressions.KindFilter] = (*Generator).filterSQL
	dispatch[expressions.KindLimitOptions] = (*Generator).limitOptionsSQL
	dispatch[expressions.KindFetch] = (*Generator).fetchSQL
	dispatch[expressions.KindCase] = (*Generator).caseSQL
	dispatch[expressions.KindIf] = (*Generator).ifSQL
	dispatch[expressions.KindExists] = (*Generator).existsSQL
	dispatch[expressions.KindAny] = (*Generator).anySQL
	dispatch[expressions.KindAll] = (*Generator).allSQL
	dispatch[expressions.KindNullSafeNEQ] = (*Generator).nullSafeNEQSQL
	dispatch[expressions.KindAnonymous] = (*Generator).anonymousSQL
	dispatch[expressions.KindLog] = (*Generator).logSQL
	dispatch[expressions.KindInsert] = (*Generator).insertSQL
	dispatch[expressions.KindUpdate] = (*Generator).updateSQL
	dispatch[expressions.KindDelete] = (*Generator).deleteSQL
	dispatch[expressions.KindMerge] = (*Generator).mergeSQL
	dispatch[expressions.KindWhen] = (*Generator).whenSQL
	dispatch[expressions.KindWhens] = (*Generator).whensSQL
	dispatch[expressions.KindOnConflict] = (*Generator).onConflictSQL
	dispatch[expressions.KindReturning] = (*Generator).returningSQL
	dispatch[expressions.KindCreate] = (*Generator).createSQL
	dispatch[expressions.KindSchema] = (*Generator).schemaSQL
	dispatch[expressions.KindCommand] = (*Generator).commandSQL
	dispatch[expressions.KindPivot] = (*Generator).pivotSQL
	dispatch[expressions.KindLateral] = (*Generator).lateralSQL
	dispatch[expressions.KindValues] = (*Generator).valuesSQL
	dispatch[expressions.KindColumnDef] = (*Generator).columnDefSQL
	dispatch[expressions.KindDataType] = (*Generator).dataTypeSQL
	dispatch[expressions.KindDataTypeParam] = (*Generator).dataTypeParamSQL
	dispatch[expressions.KindCast] = (*Generator).castSQL
	dispatch[expressions.KindTryCast] = (*Generator).tryCastSQL
	dispatch[expressions.KindExtract] = (*Generator).extractSQL
	dispatch[expressions.KindTrim] = (*Generator).trimSQL
	dispatch[expressions.KindCeil] = (*Generator).ceilFloorSQL
	dispatch[expressions.KindFloor] = (*Generator).ceilFloorSQL
	dispatch[expressions.KindUnnest] = (*Generator).unnestSQL
	dispatch[expressions.KindBracket] = (*Generator).bracketSQL
	dispatch[expressions.KindLock] = (*Generator).lockSQL
	dispatch[expressions.KindPreWhere] = (*Generator).preWhereSQL
	dispatch[expressions.KindCluster] = (*Generator).clusterSQL
	dispatch[expressions.KindDistribute] = (*Generator).distributeSQL
	dispatch[expressions.KindSort] = (*Generator).sortSQL
	dispatch[expressions.KindWithinGroup] = (*Generator).withinGroupSQL
	dispatch[expressions.KindIgnoreNulls] = (*Generator).ignoreNullsSQL
	dispatch[expressions.KindRespectNulls] = (*Generator).respectNullsSQL
	dispatch[expressions.KindPivotAny] = func(g *Generator, e expressions.Expression) string {
		return "ANY" + g.sqlKey(e, "this")
	}
	dispatch[expressions.KindPivotAlias] = (*Generator).pivotAliasSQL
	dispatch[expressions.KindInterval] = (*Generator).intervalSQL
	dispatch[expressions.KindIntervalSpan] = func(g *Generator, e expressions.Expression) string {
		return g.sqlKey(e, "this") + " TO " + g.sqlKey(e, "expression")
	}
	dispatch[expressions.KindJSONCast] = (*Generator).jsonCastSQL
	dispatch[expressions.KindJSONTable] = (*Generator).jsonTableSQL
	dispatch[expressions.KindJSONColumnDef] = (*Generator).jsonColumnDefSQL
	dispatch[expressions.KindJSONSchema] = (*Generator).jsonSchemaSQL
	dispatch[expressions.KindFormatJson] = (*Generator).formatJSONSQL
	dispatch[expressions.KindArrayAgg] = (*Generator).arrayAggSQL
	dispatch[expressions.KindArraySize] = (*Generator).arraySizeSQL
	dispatch[expressions.KindInitcap] = (*Generator).initcapSQL
	dispatch[expressions.KindHex] = (*Generator).hexSQL
	dispatch[expressions.KindDateAdd] = (*Generator).dateAddSQL
	dispatch[expressions.KindPartition] = (*Generator).partitionSQL
	dispatch[expressions.KindPragma] = (*Generator).pragmaSQL
	dispatch[expressions.KindParameter] = (*Generator).parameterSQL
	dispatch[expressions.KindRawString] = (*Generator).rawStringSQL
	dispatch[expressions.KindFileFormatProperty] = (*Generator).fileFormatPropertySQL
	dispatch[expressions.KindAtTimeZone] = (*Generator).atTimeZoneSQL
	dispatch[expressions.KindPseudoType] = (*Generator).pseudoTypeSQL
	dispatch[expressions.KindObjectIdentifier] = (*Generator).objectIdentifierSQL
}
