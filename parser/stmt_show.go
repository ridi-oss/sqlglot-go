package parser

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// parseShow ports parser.py:9226-9230 Parser._parse_show. Upstream dialects override
// SHOW_PARSERS/SHOW_TRIE at the class level (mysql.py:170-223, 240), so _find_parser is a no-op
// for base/Postgres (both leave SHOW_PARSERS empty) and always degrades to _parse_as_command.
// This port has one shared Parser (no per-dialect subclass), so showParsers/showTrie hold only
// MySQL's table and the MySQL gate below stands in for that class-level override: outside MySQL,
// SHOW always falls straight through to the raw-text Command fallback, matching upstream exactly.
func (p *Parser) parseShow() exp.Expression {
	if p.dialect.Name == "mysql" {
		start := p.prev
		if parse := p.findParser(showParsers, showTrie); parse != nil {
			// A sub-parser returning nil means the form is malformed for that keyword (e.g.
			// `SHOW CREATE USER` with no user, or trailing clauses it does not accept) — fail closed
			// to a raw Command rather than a half-built Show. The generic parseShowMySQL never
			// returns nil, so this only affects sub-parsers that opt into strict validation.
			if result := parse(p); result != nil {
				return result
			}
			return p.parseAsCommand(start)
		}
		return p.parseAsCommand(start)
	}
	if p.dialect.Name == "postgres" {
		return p.parsePostgresShow(p.prev)
	}
	return p.parseAsCommand(p.prev)
}

// parsePostgresShow structures Postgres `SHOW { ALL | <name> }` into Show{this:<name>} — a grammar
// extension (pinned upstream Commands all Postgres SHOW). The parameter (a GUC name like
// `search_path`, a dotted `ext.var`, `ALL`, or a special phrase like `TIME ZONE`) is parsed and
// captured verbatim by the shared parsePostgresConfigParam. A `SHOW` with no name, an unknown
// multi-word form, or one followed by trailing tokens (`SHOW search_path extra`) fails closed to a
// raw Command.
func (p *Parser) parsePostgresShow(showTok tokens.Token) exp.Expression {
	if name, ok := p.parsePostgresConfigParam(true); ok {
		return p.expression(exp.Show(exp.Args{"this": name}), nil, nil)
	}
	return p.parseAsCommand(showTok)
}

// showParser mirrors parsers/mysql.py:47-51 _show_parser: a closure factory that binds a SHOW
// sub-parser to a fixed `this` label plus the target/full/global_ shape it should parse with.
// target follows upstream's bool|str union: false means no target, true means a bare target (no
// prefix keywords to match first), and a non-empty string is the prefix keyword sequence (e.g.
// "FROM", "FOR") to match before parsing the target id.
func showParser(this string, target any, full any, global_ any) func(*Parser) exp.Expression {
	return func(p *Parser) exp.Expression {
		return p.parseShowMySQL(this, target, full, global_)
	}
}

// showParsers mirrors parsers/mysql.py:170-223 MySQLParser.SHOW_PARSERS. Several keys normalize
// to a different write-key on round-trip (MASTER LOGS -> BINARY LOGS, SCHEMAS -> DATABASES,
// SLAVE HOSTS -> REPLICAS, SLAVE STATUS -> REPLICA STATUS, STORAGE ENGINES -> ENGINES) because
// upstream binds them to the same `this` label as their canonical spelling.
var showParsers = map[string]func(*Parser) exp.Expression{
	"BINARY LOGS":      showParser("BINARY LOGS", false, nil, nil),
	"MASTER LOGS":      showParser("BINARY LOGS", false, nil, nil),
	"BINLOG EVENTS":    showParser("BINLOG EVENTS", false, nil, nil),
	"CHARACTER SET":    showParser("CHARACTER SET", false, nil, nil),
	"CHARSET":          showParser("CHARACTER SET", false, nil, nil),
	"COLLATION":        showParser("COLLATION", false, nil, nil),
	"FULL COLUMNS":     showParser("COLUMNS", "FROM", true, nil),
	"COLUMNS":          showParser("COLUMNS", "FROM", nil, nil),
	"CREATE DATABASE":  showParser("CREATE DATABASE", true, nil, nil),
	"CREATE EVENT":     showParser("CREATE EVENT", true, nil, nil),
	"CREATE FUNCTION":  showParser("CREATE FUNCTION", true, nil, nil),
	"CREATE PROCEDURE": showParser("CREATE PROCEDURE", true, nil, nil),
	"CREATE TABLE":     showParser("CREATE TABLE", true, nil, nil),
	"CREATE TRIGGER":   showParser("CREATE TRIGGER", true, nil, nil),
	// CREATE USER: MySQL `SHOW CREATE USER <user>` — beyond pinned upstream, whose MySQL
	// SHOW_PARSERS omits it (it Commands the statement). Modeled like the sibling CREATE * forms so
	// a consumer reads Show.this = "CREATE USER" instead of scanning the raw Command tail.
	"CREATE USER":       (*Parser).parseShowCreateUser,
	"CREATE VIEW":       showParser("CREATE VIEW", true, nil, nil),
	"DATABASES":         showParser("DATABASES", false, nil, nil),
	"SCHEMAS":           showParser("DATABASES", false, nil, nil),
	"ENGINE":            showParser("ENGINE", true, nil, nil),
	"STORAGE ENGINES":   showParser("ENGINES", false, nil, nil),
	"ENGINES":           showParser("ENGINES", false, nil, nil),
	"ERRORS":            showParser("ERRORS", false, nil, nil),
	"EVENTS":            showParser("EVENTS", false, nil, nil),
	"FUNCTION CODE":     showParser("FUNCTION CODE", true, nil, nil),
	"FUNCTION STATUS":   showParser("FUNCTION STATUS", false, nil, nil),
	"GRANTS":            showParser("GRANTS", "FOR", nil, nil),
	"INDEX":             showParser("INDEX", "FROM", nil, nil),
	"MASTER STATUS":     showParser("MASTER STATUS", false, nil, nil),
	"OPEN TABLES":       showParser("OPEN TABLES", false, nil, nil),
	"PLUGINS":           showParser("PLUGINS", false, nil, nil),
	"PROCEDURE CODE":    showParser("PROCEDURE CODE", true, nil, nil),
	"PROCEDURE STATUS":  showParser("PROCEDURE STATUS", false, nil, nil),
	"PRIVILEGES":        showParser("PRIVILEGES", false, nil, nil),
	"FULL PROCESSLIST":  showParser("PROCESSLIST", false, true, nil),
	"PROCESSLIST":       showParser("PROCESSLIST", false, nil, nil),
	"PROFILE":           showParser("PROFILE", false, nil, nil),
	"PROFILES":          showParser("PROFILES", false, nil, nil),
	"RELAYLOG EVENTS":   showParser("RELAYLOG EVENTS", false, nil, nil),
	"REPLICAS":          showParser("REPLICAS", false, nil, nil),
	"SLAVE HOSTS":       showParser("REPLICAS", false, nil, nil),
	"REPLICA STATUS":    showParser("REPLICA STATUS", false, nil, nil),
	"SLAVE STATUS":      showParser("REPLICA STATUS", false, nil, nil),
	"GLOBAL STATUS":     showParser("STATUS", false, nil, true),
	"SESSION STATUS":    showParser("STATUS", false, nil, nil),
	"STATUS":            showParser("STATUS", false, nil, nil),
	"TABLE STATUS":      showParser("TABLE STATUS", false, nil, nil),
	"FULL TABLES":       showParser("TABLES", false, true, nil),
	"TABLES":            showParser("TABLES", false, nil, nil),
	"TRIGGERS":          showParser("TRIGGERS", false, nil, nil),
	"GLOBAL VARIABLES":  showParser("VARIABLES", false, nil, true),
	"SESSION VARIABLES": showParser("VARIABLES", false, nil, nil),
	"VARIABLES":         showParser("VARIABLES", false, nil, nil),
	"WARNINGS":          showParser("WARNINGS", false, nil, nil),
}

// showTrie mirrors parsers/mysql.py:240 SHOW_TRIE = new_trie(key.split(" ") for key in
// SHOW_PARSERS), built from showParsers' keys via the shared word-trie helper (fnd).
var showTrie = newTrie(showParserKeys())

func showParserKeys() []string {
	keys := make([]string, 0, len(showParsers))
	for key := range showParsers {
		keys = append(keys, key)
	}
	return keys
}

// parseShowMySQL ports parsers/mysql.py:418-502 MySQLParser._parse_show_mysql verbatim.
func (p *Parser) parseShowMySQL(this string, target any, full any, global_ any) exp.Expression {
	json := p.matchTextSeq("JSON")

	var targetID exp.Expression
	switch t := target.(type) {
	case string:
		p.matchTextSeq(strings.Fields(t)...)
		targetID = p.parseIdVar(true, nil)
	case bool:
		if t {
			targetID = p.parseIdVar(true, nil)
		}
	}

	index := p.index
	var log exp.Expression
	if p.matchTextSeq("IN") {
		log = p.parseString()
		if log == nil {
			p.retreat(index)
		}
	}

	var position, db exp.Expression
	if this == "BINLOG EVENTS" || this == "RELAYLOG EVENTS" {
		if p.matchTextSeq("FROM") {
			position = p.parseShowNumber()
		}
	} else {
		if p.match(tokens.FROM) || p.matchTextSeq("IN") {
			db = p.parseIdVar(true, nil)
		} else if p.match(tokens.DOT) {
			db = targetID
			targetID = p.parseIdVar(true, nil)
		}
	}

	var channel exp.Expression
	if p.matchTextSeq("FOR", "CHANNEL") {
		channel = p.parseIdVar(true, nil)
	}

	var like exp.Expression
	if p.matchTextSeq("LIKE") {
		like = p.parseString()
	}
	where := p.parseWhere(false)

	var types []exp.Expression
	var query, offset, limit exp.Expression
	if this == "PROFILE" {
		types = p.parseCsv(func() exp.Expression { return p.parseVarFromOptions(profileTypes, true) })
		if p.matchTextSeq("FOR", "QUERY") {
			query = p.parseShowNumber()
		}
		if p.matchTextSeq("OFFSET") {
			offset = p.parseShowNumber()
		}
		if p.matchTextSeq("LIMIT") {
			limit = p.parseShowNumber()
		}
	} else {
		offset, limit = p.parseOldstyleLimit()
	}

	var mutex any
	if p.matchTextSeq("MUTEX") {
		mutex = true
	}
	if p.matchTextSeq("STATUS") {
		mutex = false
	}

	var forTable, forGroup, forUser, forRole, intoOutfile exp.Expression
	if p.matchTextSeq("FOR", "TABLE") {
		forTable = p.parseIdVar(true, nil)
	}
	if p.matchTextSeq("FOR", "GROUP") {
		forGroup = p.parseString()
	}
	if p.matchTextSeq("FOR", "USER") {
		forUser = p.parseString()
	}
	if p.matchTextSeq("FOR", "ROLE") {
		forRole = p.parseString()
	}
	if p.matchTextSeq("INTO", "OUTFILE") {
		intoOutfile = p.parseString()
	}

	return p.expression(exp.Show(exp.Args{
		"this":         this,
		"target":       targetID,
		"full":         full,
		"log":          log,
		"position":     position,
		"db":           db,
		"channel":      channel,
		"like":         like,
		"where":        where,
		"types":        types,
		"query":        query,
		"offset":       offset,
		"limit":        limit,
		"mutex":        mutex,
		"for_table":    forTable,
		"for_group":    forGroup,
		"for_user":     forUser,
		"for_role":     forRole,
		"into_outfile": intoOutfile,
		"json":         json,
		"global_":      global_,
	}), nil, nil)
}

// parseOldstyleLimit ports parsers/mysql.py:504-517 MySQLParser._parse_oldstyle_limit: MySQL's
// `LIMIT [offset,] count` form used by SHOW statements that predate the PROFILE-style OFFSET/
// LIMIT pair. Returns (offset, limit), mirroring the Python tuple order.
func (p *Parser) parseOldstyleLimit() (offset, limit exp.Expression) {
	if p.matchTextSeq("LIMIT") {
		parts := p.parseCsv(p.parseShowNumber)
		if len(parts) == 1 {
			limit = parts[0]
		} else if len(parts) == 2 {
			offset = parts[0]
			limit = parts[1]
		}
	}
	return offset, limit
}

// parseShowNumber is a SHOW-local stand-in for upstream's more general _parse_number
// (parser.py:8531-8534): SHOW's numeric fields (position/query/offset/limit) are always plain
// integer literals, so a NUMBER token dispatched through parsePrimary (NUMBER -> Literal) covers
// every case this grammar needs without pulling in the full NUMERIC_PARSERS/placeholder fallback.
func (p *Parser) parseShowNumber() exp.Expression {
	if p.curr.TokenType != tokens.NUMBER {
		return nil
	}
	return p.parsePrimary()
}

func init() {
	statementParsers[tokens.SHOW] = (*Parser).parseShow
}
