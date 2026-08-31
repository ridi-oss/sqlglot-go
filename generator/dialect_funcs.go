package generator

import "github.com/ridi-oss/sqlglot-go/expressions"

// dayOfMonthSQL/dayOfWeekSQL/dayOfYearSQL/weekOfYearSQL port generators/mysql.py:170-173's
// `_remove_ts_or_ds_to_date(rename_func("DAYOFMONTH"))` etc. This port never wraps the "this"
// arg in a TsOrDsToDate node (unimplemented Kind, out of this part's scope), so the
// _remove_ts_or_ds_to_date unwrap step upstream performs is a no-op here and is elided; only
// the MySQL rename survives. Base and postgres keep the class's default name (DAY_OF_MONTH/
// DAY_OF_WEEK/DAY_OF_YEAR/WEEK_OF_YEAR, from DayOfMonth._sql_names[0] etc., temporal.py:
// 209-270) via functionFallbackSQL, matching pre-dispatch-entry behavior for those dialects.
func (g *Generator) dayOfMonthSQL(e expressions.Expression) string {
	return g.mysqlRenameFunc(e, "DAYOFMONTH")
}

func (g *Generator) dayOfWeekSQL(e expressions.Expression) string {
	return g.mysqlRenameFunc(e, "DAYOFWEEK")
}

func (g *Generator) dayOfYearSQL(e expressions.Expression) string {
	return g.mysqlRenameFunc(e, "DAYOFYEAR")
}

func (g *Generator) weekOfYearSQL(e expressions.Expression) string {
	return g.mysqlRenameFunc(e, "WEEKOFYEAR")
}

// truncSQL ports generators/mysql.py:222 `exp.Trunc: rename_func("TRUNCATE")`. Base/postgres
// keep the class's default name (Trunc._sql_names[0] = "TRUNC", math.py:188-190) via
// functionFallbackSQL.
func (g *Generator) truncSQL(e expressions.Expression) string {
	return g.mysqlRenameFunc(e, "TRUNCATE")
}

// mysqlRenameFunc is the shared "rename this Kind's default spelling to name, but only under
// MySQL" helper backing the four simple rename_func-style overrides above: it gathers args
// the same way the pre-override functionFallbackSQL rendering did (fallbackArgs, ArgKeys
// order, flattening/nil-skipping), so switching a Kind between the fallback and this path
// never changes which arguments are emitted - only the function name and dialect gating do.
func (g *Generator) mysqlRenameFunc(e expressions.Expression, name string) string {
	if g.dialect.Name != "mysql" {
		return g.functionFallbackSQL(e)
	}
	return g.funcCall(name, g.fallbackArgs(e), "(", ")", true)
}

// currentSchemaSQL ports generators/mysql.py:807-809 (`@unsupported_args("this");
// currentschema_sql: return self.func("SCHEMA")`): MySQL's DATABASE()/SCHEMA() both parse to
// exp.CurrentSchema (dialects/mysql.go's d.Functions) and always render back as the
// zero-arg SCHEMA() call, regardless of the (upstream-unsupported) "this" arg. Base has no
// override (postgres's own CURRENT_SCHEMA override is out of this part's scope - postgres
// never registers a parse-side path to this Kind, see dialects/postgres.go), so it keeps the
// default name (CURRENT_SCHEMA, from the class name, functions.py:285) via
// functionFallbackSQL.
func (g *Generator) currentSchemaSQL(e expressions.Expression) string {
	switch g.dialect.Name {
	case "mysql":
		// generators/mysql.py:807-809 `@unsupported_args("this"); return self.func("SCHEMA")`.
		return g.funcCall("SCHEMA", nil, "(", ")", true)
	case "postgres":
		// generators/postgres.py:527-528 `@unsupported_args("this"); return "CURRENT_SCHEMA"` —
		// bare, no parens, dropping any precision arg. Reached now that postgres builds
		// CurrentSchema from the bare CURRENT_SCHEMA keyword (its NoParenFunctions override).
		return "CURRENT_SCHEMA"
	}
	// Base/presto/trino/hive have no currentschema_sql override, so they keep the parenthesized
	// functionFallback (CURRENT_SCHEMA()), matching pinned upstream's default generator (base
	// generator.py has no CurrentSchema entry at all).
	return g.functionFallbackSQL(e)
}

// niladicBareDialects are the dialects whose generator renders CurrentTimestamp/CurrentUser as the
// bare keyword (no parens): postgres (generators/postgres.py:302-303) and the Presto family —
// presto/trino/athena (generators/presto.py:289-290). base/mysql/hive have no such override, so
// they render the parenthesized function-fallback form (CURRENT_TIMESTAMP()/CURRENT_USER()).
var niladicBareDialects = map[string]bool{"postgres": true, "presto": true, "trino": true, "athena": true}

// currentTimestampSQL renders CurrentTimestamp bare under postgres/presto/trino/athena, else via
// the parenthesized function fallback (CURRENT_TIMESTAMP([...])). Since the shared parser now
// resolves the bare CURRENT_TIMESTAMP keyword for every dialect via the base NO_PAREN_FUNCTIONS
// map, this per-dialect gating is what keeps each dialect's round-trip byte-identical to upstream.
func (g *Generator) currentTimestampSQL(e expressions.Expression) string {
	if niladicBareDialects[g.dialect.Name] {
		return "CURRENT_TIMESTAMP"
	}
	return g.functionFallbackSQL(e)
}

// currentUserSQL renders CurrentUser bare under postgres/presto/trino/athena; base/mysql/hive
// render CURRENT_USER() via the function fallback.
func (g *Generator) currentUserSQL(e expressions.Expression) string {
	if niladicBareDialects[g.dialect.Name] {
		return "CURRENT_USER"
	}
	return g.functionFallbackSQL(e)
}

// currentCatalogSQL renders the bare CURRENT_CATALOG keyword (generator.py:171
// `exp.CurrentCatalog: lambda *_: "CURRENT_CATALOG"`), dialect-agnostic.
func (g *Generator) currentCatalogSQL(_ expressions.Expression) string {
	return "CURRENT_CATALOG"
}

// sessionUserSQL renders bare SESSION_USER (generator.py:172); mysql renders the parenthesized
// SESSION_USER() (generators/mysql.py:204). In this port only postgres builds SessionUser from a
// bare keyword (its NoParenFunctions override); the mysql arm covers the SESSION_USER() call form.
func (g *Generator) sessionUserSQL(_ expressions.Expression) string {
	if g.dialect.Name == "mysql" {
		return "SESSION_USER()"
	}
	return "SESSION_USER"
}

// localtimeSQL / localtimestampSQL port generator.py:6174-6180: bare LOCALTIME/LOCALTIMESTAMP
// when there is no precision arg, else the parenthesized form. Dialect-agnostic (postgres and
// mysql both wire these into NO_PAREN_FUNCTIONS).
func (g *Generator) localtimeSQL(e expressions.Expression) string {
	if g.sqlKey(e, "this") != "" {
		return g.funcCall("LOCALTIME", []any{e.Arg("this")}, "(", ")", true)
	}
	return "LOCALTIME"
}

func (g *Generator) localtimestampSQL(e expressions.Expression) string {
	if g.sqlKey(e, "this") != "" {
		return g.funcCall("LOCALTIMESTAMP", []any{e.Arg("this")}, "(", ")", true)
	}
	return "LOCALTIMESTAMP"
}

// strPositionSQL ports dialects/dialect.py:1281-1321 `strposition_sql`, specialized to the
// one caller this part wires up: generators/mysql.py:198-200 `exp.StrPosition:
// strposition_sql(self, e, func_name="LOCATE", supports_position=True)`. func_name="LOCATE"
// swaps the argument order (substr, this) relative to StrPosition's own arg_types order
// (dialect.py:1306); supports_position=True appends the "position" arg (if any) as a third
// LOCATE argument instead of transpiling it into a Substring-wrapped offset (the
// transpile_position branch, dialect.py:1299-1301,1316-1319, is therefore dead for this
// caller and is elided). occurrence is unsupported by LOCATE and silently dropped, matching
// upstream's own "not supports_occurrence" warn-and-continue path (formatArgs would drop a
// nil occurrence anyway, but INSTR - the only mysql-side builder of this Kind, dialects/
// mysql.go - never sets it). Base/postgres keep the default name (STR_POSITION, from the
// class name, string.py:33-38) via functionFallbackSQL, matching pre-dispatch-entry behavior
// (e.g. testdata/identity.sql's STR_POSITION(haystack, needle[, pos]) cases).
func (g *Generator) strPositionSQL(e expressions.Expression) string {
	// opaque_functions: only POSITION(a IN b) parses to StrPosition, so render that grammar
	// back verbatim — never the STR_POSITION/LOCATE canonicalization.
	if g.dialect.OpaqueFunctions {
		return "POSITION(" + g.sqlKey(e, "substr") + " IN " + g.sqlKey(e, "this") + ")"
	}
	if g.dialect.Name != "mysql" {
		return g.functionFallbackSQL(e)
	}
	return g.funcCall("LOCATE", []any{e.Arg("substr"), e.Arg("this"), e.Arg("position")}, "(", ")", true)
}

// overlaySQL ports overlay_sql (generator.py:5746-5753): OVERLAY(<this> PLACING <expression>
// FROM <from_>[ FOR <for_>]). Not dialect-gated upstream (no per-dialect override in
// generators/mysql.py or generators/postgres.py), so this applies uniformly - closing
// parity_gaps.txt gaps 196-197 (postgres is the only dialect that currently parses OVERLAY,
// see parser/parser_functions.go's parseOverlay, but the rendering itself is dialect-agnostic
// like upstream's).
func (g *Generator) overlaySQL(e expressions.Expression) string {
	this := g.sqlKey(e, "this")
	expr := g.sqlKey(e, "expression")
	fromSQL := g.sqlKey(e, "from_")
	forSQL := g.sqlKey(e, "for_")
	if forSQL != "" {
		forSQL = " FOR " + forSQL
	}
	return "OVERLAY(" + this + " PLACING " + expr + " FROM " + fromSQL + forSQL + ")"
}

// variadicSQL ports generator.py:286 `exp.Variadic: lambda self, e: f"VARIADIC
// {self.sql(e, 'this')}"`. Not dialect-gated upstream, and reachable in this port only via
// postgres's noParenFunctionParsers["VARIADIC"] entry (parser/parser_functions.go), but the
// rendering itself is dialect-agnostic like upstream's.
func (g *Generator) variadicSQL(e expressions.Expression) string {
	return "VARIADIC " + g.sqlKey(e, "this")
}

// currentDateSQL ports currentdate_sql (generator.py:4098-4100): `CURRENT_DATE(<zone>)` when
// a zone/"this" arg is present, else bare `CURRENT_DATE` (no parens). This is a base Generator
// method (not dialect-gated), so mysql's CURDATE() -> CurrentDate and any bare CURRENT_DATE
// keyword (parser's NO_PAREN_FUNCTIONS) both render without the empty parens the generic
// functionFallbackSQL would emit. CurrentTime/CurrentSchema deliberately keep that
// parenthesized fallback (upstream has no currenttime_sql/currentschema base override).
func (g *Generator) currentDateSQL(e expressions.Expression) string {
	if zone := g.sqlKey(e, "this"); zone != "" {
		return "CURRENT_DATE(" + zone + ")"
	}
	return "CURRENT_DATE"
}

func init() {
	dispatch[expressions.KindDayOfMonth] = (*Generator).dayOfMonthSQL
	dispatch[expressions.KindDayOfWeek] = (*Generator).dayOfWeekSQL
	dispatch[expressions.KindDayOfYear] = (*Generator).dayOfYearSQL
	dispatch[expressions.KindWeekOfYear] = (*Generator).weekOfYearSQL
	dispatch[expressions.KindTrunc] = (*Generator).truncSQL
	dispatch[expressions.KindCurrentSchema] = (*Generator).currentSchemaSQL
	dispatch[expressions.KindStrPosition] = (*Generator).strPositionSQL
	dispatch[expressions.KindOverlay] = (*Generator).overlaySQL
	dispatch[expressions.KindVariadic] = (*Generator).variadicSQL
	dispatch[expressions.KindCurrentDate] = (*Generator).currentDateSQL
	dispatch[expressions.KindCurrentTimestamp] = (*Generator).currentTimestampSQL
	dispatch[expressions.KindCurrentUser] = (*Generator).currentUserSQL
	dispatch[expressions.KindCurrentCatalog] = (*Generator).currentCatalogSQL
	dispatch[expressions.KindSessionUser] = (*Generator).sessionUserSQL
	dispatch[expressions.KindLocaltime] = (*Generator).localtimeSQL
	dispatch[expressions.KindLocaltimestamp] = (*Generator).localtimestampSQL
}
