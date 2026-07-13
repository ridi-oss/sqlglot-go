package parser_test

import (
	"strings"
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// showRoundTrip parses sql under the mysql dialect, asserts it becomes an exp.Show, and asserts
// that regenerating it reproduces want exactly (want defaults to sql when empty).
func showRoundTrip(t *testing.T, sql, want string) exp.Expression {
	t.Helper()
	if want == "" {
		want = sql
	}
	show := parseOneDialect(t, sql, "mysql")
	if show.Kind() != exp.KindShow {
		t.Fatalf("SHOW %q should parse to exp.Show, got %v:\n%s", sql, show.Kind(), show.ToS())
	}
	got, err := generateSQL(t, show, "mysql")
	if err != nil {
		t.Fatalf("Generate(%q): %v", sql, err)
	}
	if got != want {
		t.Fatalf("round-trip %q = %q, want %q", sql, got, want)
	}
	return show
}

// TestShowSimple ports test_mysql.py:1229-1245 test_show_simple.
func TestShowSimple(t *testing.T) {
	cases := []struct{ key, writeKey string }{
		{"BINARY LOGS", "BINARY LOGS"},
		{"MASTER LOGS", "BINARY LOGS"},
		{"STORAGE ENGINES", "ENGINES"},
		{"ENGINES", "ENGINES"},
		{"EVENTS", "EVENTS"},
		{"MASTER STATUS", "MASTER STATUS"},
		{"PLUGINS", "PLUGINS"},
		{"PRIVILEGES", "PRIVILEGES"},
		{"PROFILES", "PROFILES"},
		{"REPLICAS", "REPLICAS"},
		{"SLAVE HOSTS", "REPLICAS"},
	}
	for _, c := range cases {
		show := showRoundTrip(t, "SHOW "+c.key, "SHOW "+c.writeKey)
		if show.Name() != c.writeKey {
			t.Fatalf("SHOW %s: name = %q, want %q", c.key, show.Name(), c.writeKey)
		}
	}
}

// TestShowEvents ports test_mysql.py:1247-1261 test_show_events.
func TestShowEvents(t *testing.T) {
	for _, key := range []string{"BINLOG", "RELAYLOG"} {
		show := showRoundTrip(t, "SHOW "+key+" EVENTS", "")
		if want := key + " EVENTS"; show.Name() != want {
			t.Fatalf("name = %q, want %q", show.Name(), want)
		}

		show = showRoundTrip(t, "SHOW "+key+" EVENTS IN 'log' FROM 1 LIMIT 2, 3", "")
		if got, want := show.Text("log"), "log"; got != want {
			t.Fatalf("log = %q, want %q", got, want)
		}
		if got, want := show.Text("position"), "1"; got != want {
			t.Fatalf("position = %q, want %q", got, want)
		}
		if got, want := show.Text("limit"), "3"; got != want {
			t.Fatalf("limit = %q, want %q", got, want)
		}
		if got, want := show.Text("offset"), "2"; got != want {
			t.Fatalf("offset = %q, want %q", got, want)
		}

		show = showRoundTrip(t, "SHOW "+key+" EVENTS LIMIT 1", "")
		if got, want := show.Text("limit"), "1"; got != want {
			t.Fatalf("limit = %q, want %q", got, want)
		}
		if show.Arg("offset") != nil {
			t.Fatalf("offset should be unset, got %#v", show.Arg("offset"))
		}
	}
}

// TestShowLikeOrWhere ports test_mysql.py:1263-1295 test_show_like_or_where.
func TestShowLikeOrWhere(t *testing.T) {
	cases := []struct{ key, writeKey string }{
		{"CHARSET", "CHARACTER SET"},
		{"CHARACTER SET", "CHARACTER SET"},
		{"COLLATION", "COLLATION"},
		{"DATABASES", "DATABASES"},
		{"SCHEMAS", "DATABASES"},
		{"FUNCTION STATUS", "FUNCTION STATUS"},
		{"PROCEDURE STATUS", "PROCEDURE STATUS"},
		{"GLOBAL STATUS", "GLOBAL STATUS"},
		{"SESSION STATUS", "STATUS"},
		{"STATUS", "STATUS"},
		{"GLOBAL VARIABLES", "GLOBAL VARIABLES"},
		{"SESSION VARIABLES", "VARIABLES"},
		{"VARIABLES", "VARIABLES"},
	}
	for _, c := range cases {
		expectedName := strings.TrimSpace(strings.TrimPrefix(c.writeKey, "GLOBAL"))

		show := showRoundTrip(t, "SHOW "+c.key, "SHOW "+c.writeKey)
		if show.Name() != expectedName {
			t.Fatalf("SHOW %s: name = %q, want %q", c.key, show.Name(), expectedName)
		}

		show = showRoundTrip(t, "SHOW "+c.key+" LIKE '%foo%'", "SHOW "+c.writeKey+" LIKE '%foo%'")
		if like := exprArg(t, show, "like"); like.Kind() != exp.KindLiteral {
			t.Fatalf("like should be a Literal:\n%s", show.ToS())
		}
		if got, want := show.Text("like"), "%foo%"; got != want {
			t.Fatalf("like = %q, want %q", got, want)
		}

		show = showRoundTrip(t, "SHOW "+c.key+" WHERE Column_name LIKE '%foo%'", "SHOW "+c.writeKey+" WHERE Column_name LIKE '%foo%'")
		if where := exprArg(t, show, "where"); where.Kind() != exp.KindWhere {
			t.Fatalf("where should be a Where:\n%s", show.ToS())
		}
	}
}

// TestShowColumns ports test_mysql.py:1297-1310 test_show_columns.
func TestShowColumns(t *testing.T) {
	show := showRoundTrip(t, "SHOW COLUMNS FROM tbl_name", "")
	if show.Name() != "COLUMNS" {
		t.Fatalf("name = %q, want COLUMNS", show.Name())
	}
	if got, want := show.Text("target"), "tbl_name"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if show.Arg("full") == true {
		t.Fatalf("full should be falsy, got %#v", show.Arg("full"))
	}

	show = showRoundTrip(t, "SHOW FULL COLUMNS FROM tbl_name FROM db_name LIKE '%foo%'", "")
	if got, want := show.Text("target"), "tbl_name"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if show.Arg("full") != true {
		t.Fatalf("full should be true, got %#v", show.Arg("full"))
	}
	if got, want := show.Text("db"), "db_name"; got != want {
		t.Fatalf("db = %q, want %q", got, want)
	}
	if got, want := show.Text("like"), "%foo%"; got != want {
		t.Fatalf("like = %q, want %q", got, want)
	}
}

// TestShowName ports test_mysql.py:1312-1327 test_show_name.
func TestShowName(t *testing.T) {
	for _, key := range []string{
		"CREATE DATABASE", "CREATE EVENT", "CREATE FUNCTION", "CREATE PROCEDURE",
		"CREATE TABLE", "CREATE TRIGGER", "CREATE VIEW", "FUNCTION CODE", "PROCEDURE CODE",
	} {
		show := showRoundTrip(t, "SHOW "+key+" foo", "")
		if show.Name() != key {
			t.Fatalf("name = %q, want %q", show.Name(), key)
		}
		if got, want := show.Text("target"), "foo"; got != want {
			t.Fatalf("target = %q, want %q", got, want)
		}
	}
}

// TestShowGrants ports test_mysql.py:1329-1333 test_show_grants.
func TestShowGrants(t *testing.T) {
	show := showRoundTrip(t, "SHOW GRANTS FOR foo", "")
	if show.Name() != "GRANTS" {
		t.Fatalf("name = %q, want GRANTS", show.Name())
	}
	if got, want := show.Text("target"), "foo"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

// TestShowEngine ports test_mysql.py:1335-1345 test_show_engine.
func TestShowEngine(t *testing.T) {
	show := showRoundTrip(t, "SHOW ENGINE foo STATUS", "")
	if show.Name() != "ENGINE" {
		t.Fatalf("name = %q, want ENGINE", show.Name())
	}
	if got, want := show.Text("target"), "foo"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if show.Arg("mutex") == true {
		t.Fatalf("mutex should be falsy, got %#v", show.Arg("mutex"))
	}

	show = showRoundTrip(t, "SHOW ENGINE foo MUTEX", "")
	if got, want := show.Text("target"), "foo"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if show.Arg("mutex") != true {
		t.Fatalf("mutex should be true, got %#v", show.Arg("mutex"))
	}
}

// TestShowErrors ports test_mysql.py:1347-1355 test_show_errors.
func TestShowErrors(t *testing.T) {
	for _, key := range []string{"ERRORS", "WARNINGS"} {
		show := showRoundTrip(t, "SHOW "+key, "")
		if show.Name() != key {
			t.Fatalf("name = %q, want %q", show.Name(), key)
		}

		show = showRoundTrip(t, "SHOW "+key+" LIMIT 2, 3", "")
		if got, want := show.Text("limit"), "3"; got != want {
			t.Fatalf("limit = %q, want %q", got, want)
		}
		if got, want := show.Text("offset"), "2"; got != want {
			t.Fatalf("offset = %q, want %q", got, want)
		}
	}
}

// TestShowIndex ports test_mysql.py:1357-1368 test_show_index.
func TestShowIndex(t *testing.T) {
	show := showRoundTrip(t, "SHOW INDEX FROM foo", "")
	if show.Name() != "INDEX" {
		t.Fatalf("name = %q, want INDEX", show.Name())
	}
	if got, want := show.Text("target"), "foo"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}

	show = showRoundTrip(t, "SHOW INDEX FROM foo FROM bar", "")
	if got, want := show.Text("db"), "bar"; got != want {
		t.Fatalf("db = %q, want %q", got, want)
	}

	showRoundTrip(t, "SHOW INDEX FROM bar.foo", "SHOW INDEX FROM foo FROM bar")
}

// TestShowDbLikeOrWhere ports test_mysql.py:1370-1392 test_show_db_like_or_where_sql.
func TestShowDbLikeOrWhere(t *testing.T) {
	for _, key := range []string{"OPEN TABLES", "TABLE STATUS", "TRIGGERS"} {
		show := showRoundTrip(t, "SHOW "+key, "")
		if show.Name() != key {
			t.Fatalf("name = %q, want %q", show.Name(), key)
		}

		show = showRoundTrip(t, "SHOW "+key+" FROM db_name", "")
		if show.Name() != key {
			t.Fatalf("name = %q, want %q", show.Name(), key)
		}
		if got, want := show.Text("db"), "db_name"; got != want {
			t.Fatalf("db = %q, want %q", got, want)
		}

		show = showRoundTrip(t, "SHOW "+key+" LIKE '%foo%'", "")
		if show.Name() != key {
			t.Fatalf("name = %q, want %q", show.Name(), key)
		}
		if got, want := show.Text("like"), "%foo%"; got != want {
			t.Fatalf("like = %q, want %q", got, want)
		}

		show = showRoundTrip(t, "SHOW "+key+" WHERE Column_name LIKE '%foo%'", "")
		if show.Name() != key {
			t.Fatalf("name = %q, want %q", show.Name(), key)
		}
		if where := exprArg(t, show, "where"); where.Kind() != exp.KindWhere {
			t.Fatalf("where should be a Where:\n%s", show.ToS())
		}
	}
}

// TestShowProcesslist ports test_mysql.py:1394-1402 test_show_processlist.
func TestShowProcesslist(t *testing.T) {
	show := showRoundTrip(t, "SHOW PROCESSLIST", "")
	if show.Name() != "PROCESSLIST" {
		t.Fatalf("name = %q, want PROCESSLIST", show.Name())
	}
	if show.Arg("full") == true {
		t.Fatalf("full should be falsy, got %#v", show.Arg("full"))
	}

	show = showRoundTrip(t, "SHOW FULL PROCESSLIST", "")
	if show.Name() != "PROCESSLIST" {
		t.Fatalf("name = %q, want PROCESSLIST", show.Name())
	}
	if show.Arg("full") != true {
		t.Fatalf("full should be true, got %#v", show.Arg("full"))
	}
}

// TestShowProfile ports test_mysql.py:1404-1419 test_show_profile.
func TestShowProfile(t *testing.T) {
	show := showRoundTrip(t, "SHOW PROFILE", "")
	if show.Name() != "PROFILE" {
		t.Fatalf("name = %q, want PROFILE", show.Name())
	}

	show = showRoundTrip(t, "SHOW PROFILE BLOCK IO", "")
	types := expressionsForArg(show, "types")
	if len(types) != 1 || types[0].Name() != "BLOCK IO" {
		t.Fatalf("types = %v, want [BLOCK IO]", types)
	}

	show = showRoundTrip(t, "SHOW PROFILE BLOCK IO, PAGE FAULTS FOR QUERY 1 OFFSET 2 LIMIT 3", "")
	types = expressionsForArg(show, "types")
	if len(types) != 2 || types[0].Name() != "BLOCK IO" || types[1].Name() != "PAGE FAULTS" {
		t.Fatalf("types = %v, want [BLOCK IO, PAGE FAULTS]", types)
	}
	if got, want := show.Text("query"), "1"; got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
	if got, want := show.Text("offset"), "2"; got != want {
		t.Fatalf("offset = %q, want %q", got, want)
	}
	if got, want := show.Text("limit"), "3"; got != want {
		t.Fatalf("limit = %q, want %q", got, want)
	}
}

// TestShowReplicaStatus ports test_mysql.py:1421-1431 test_show_replica_status.
func TestShowReplicaStatus(t *testing.T) {
	show := showRoundTrip(t, "SHOW REPLICA STATUS", "")
	if show.Name() != "REPLICA STATUS" {
		t.Fatalf("name = %q, want REPLICA STATUS", show.Name())
	}

	show = showRoundTrip(t, "SHOW SLAVE STATUS", "SHOW REPLICA STATUS")
	if show.Name() != "REPLICA STATUS" {
		t.Fatalf("name = %q, want REPLICA STATUS", show.Name())
	}

	show = showRoundTrip(t, "SHOW REPLICA STATUS FOR CHANNEL channel_name", "")
	if got, want := show.Text("channel"), "channel_name"; got != want {
		t.Fatalf("channel = %q, want %q", got, want)
	}
}

// TestShowTables ports test_mysql.py:1433-1451 test_show_tables.
func TestShowTables(t *testing.T) {
	show := showRoundTrip(t, "SHOW TABLES", "")
	if show.Name() != "TABLES" {
		t.Fatalf("name = %q, want TABLES", show.Name())
	}

	show = showRoundTrip(t, "SHOW FULL TABLES FROM db_name LIKE '%foo%'", "")
	if show.Arg("full") != true {
		t.Fatalf("full should be true, got %#v", show.Arg("full"))
	}
	if got, want := show.Text("db"), "db_name"; got != want {
		t.Fatalf("db = %q, want %q", got, want)
	}
	if got, want := show.Text("like"), "%foo%"; got != want {
		t.Fatalf("like = %q, want %q", got, want)
	}

	show = showRoundTrip(t, "SHOW TABLES IN test", "SHOW TABLES FROM test")
	if show.Name() != "TABLES" {
		t.Fatalf("name = %q, want TABLES", show.Name())
	}
	if got, want := show.Text("db"), "test"; got != want {
		t.Fatalf("db = %q, want %q", got, want)
	}

	show = showRoundTrip(t, "SHOW FULL TABLES IN test", "SHOW FULL TABLES FROM test")
	if show.Arg("full") != true {
		t.Fatalf("full should be true, got %#v", show.Arg("full"))
	}
	if got, want := show.Text("db"), "test"; got != want {
		t.Fatalf("db = %q, want %q", got, want)
	}
}

// TestShowDegradesToCommandOutsideMySQL covers the base/Postgres side of parseShow
// (parser.py:9226-9230): SHOW_PARSERS is only populated for MySQL in this port, so any dialect
// that doesn't gate on p.dialect.Name=="mysql" falls straight through to the raw-text Command
// fallback, matching upstream's base _parse_show -> _parse_as_command.
func TestShowDegradesToCommandOutsideMySQL(t *testing.T) {
	for _, dialect := range []string{"", "postgres"} {
		cmd := parseOneDialect(t, "SHOW TABLES", dialect)
		if cmd.Kind() != exp.KindCommand {
			t.Fatalf("dialect %q: SHOW should degrade to Command, got %v:\n%s", dialect, cmd.Kind(), cmd.ToS())
		}
		got, err := generateSQL(t, cmd, dialect)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != "SHOW TABLES" {
			t.Fatalf("round-trip = %q, want %q", got, "SHOW TABLES")
		}
	}
}

// TestShowUnmatchedMySQLDegradesToCommand covers the findParser-miss branch of parseShow: a SHOW
// variant that isn't one of MySQL's SHOW_PARSERS keys still degrades to Command instead of
// failing to parse.
func TestShowUnmatchedMySQLDegradesToCommand(t *testing.T) {
	cmd := parseOneDialect(t, "SHOW CREATE SEQUENCE foo", "mysql")
	if cmd.Kind() != exp.KindCommand {
		t.Fatalf("unmatched SHOW should degrade to Command, got %v:\n%s", cmd.Kind(), cmd.ToS())
	}
	got, err := generateSQL(t, cmd, "mysql")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "SHOW CREATE SEQUENCE foo"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}
