package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// roundTripCase asserts that sql parses+generates back to want under dialect - the same
// semantics as corpus_test.go's roundTrip, reused here for the DDL-tail parity gaps this
// slice closes (parity_gaps.txt gaps 85, 86, 164, 172, 179, 180, 181, oracle-verified against
// .reference/sqlglot-v30.12.0).
func roundTripCase(t *testing.T, dialect, sql, want string) {
	t.Helper()
	expression := parseOneDialect(t, sql, dialect)
	got, err := generateSQL(t, expression, dialect)
	if err != nil {
		t.Fatalf("Generate(%q) error: %v", sql, err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch:\n  sql:  %s\n  got:  %s\n  want: %s\n  ast:  %s", sql, got, want, expression.ToS())
	}
}

// TestCreateFunctionDDLTailGaps closes parity_gaps.txt 85/172/179/180: CREATE FUNCTION's
// RETURNS/LANGUAGE/SQL SECURITY/CALLED ON NULL INPUT/IMMUTABLE/SET properties, and a UDF
// signature's DEFAULT value.
func TestCreateFunctionDDLTailGaps(t *testing.T) {
	// gap 85 (mysql): RETURNS/LANGUAGE/SQL SECURITY properties parsed before AS, MySQL's
	// VARCHAR-without-size -> TEXT canonicalization, and a bare SELECT body (no AS in the
	// source - _parse_user_defined_function_expression's parseStatement fallback).
	roundTripCase(t, "mysql",
		"CREATE FUNCTION f () RETURNS VARCHAR LANGUAGE SQL SQL SECURITY INVOKER SELECT 'abc'",
		"CREATE FUNCTION f() RETURNS TEXT LANGUAGE SQL SQL SECURITY INVOKER AS SELECT 'abc'")

	// gap 172 (postgres): a UDF signature with no RETURNS/AS/body at all, and a parameter's
	// DEFAULT value (character varying DEFAULT NULL::character varying -> CAST(NULL AS
	// VARCHAR)).
	roundTripCase(t, "postgres",
		"CREATE OR REPLACE FUNCTION function_name (input_a character varying DEFAULT NULL::character varying)",
		"CREATE OR REPLACE FUNCTION function_name(input_a VARCHAR DEFAULT CAST(NULL AS VARCHAR))")

	// gap 179 (postgres): a string-literal AS body, with LANGUAGE/IMMUTABLE/CALLED ON NULL
	// INPUT properties parsed AFTER it (parseTailProperties' third pass). Also exercises the
	// upstream-faithful ambiguity where bare `integer, integer` parameters (no name) are
	// parsed as plain Identifiers, not ColumnDefs (verified against the pinned oracle).
	roundTripCase(t, "postgres",
		"CREATE FUNCTION add(integer, integer) RETURNS integer AS 'select $1 + $2;' LANGUAGE SQL IMMUTABLE CALLED ON NULL INPUT",
		"CREATE FUNCTION add(integer, integer) RETURNS INT LANGUAGE SQL IMMUTABLE CALLED ON NULL INPUT AS 'select $1 + $2;'")

	// gap 180 (postgres): postgres's SET override (-> exp.SetConfigProperty wrapping the
	// existing exp.Set/SetItem nodes), canonicalizing SET ... TO ... to SET ... = ....
	roundTripCase(t, "postgres",
		"CREATE FUNCTION x(INT) RETURNS INT SET search_path TO 'public'",
		"CREATE FUNCTION x(INT) RETURNS INT SET search_path = 'public'")

	// Sibling case (not itself a tracked gap, but exercises the same SET property path):
	// when anything beyond the SET item list remains, upstream's own _parse_set degrades to
	// an embedded Command capturing the rest of the statement (verified against the pinned
	// oracle) - so the trailing LANGUAGE/IMMUTABLE properties never become separate
	// structured nodes here, and "TO" is preserved verbatim (no "=" canonicalization).
	roundTripCase(t, "postgres",
		"CREATE FUNCTION add(INT, INT) RETURNS INT SET search_path TO 'public' AS 'select $1 + $2;' LANGUAGE SQL IMMUTABLE",
		"CREATE FUNCTION add(INT, INT) RETURNS INT SET search_path TO 'public' AS 'select $1 + $2;' LANGUAGE SQL IMMUTABLE")
}

// TestCreateFunctionUnsupportedBodyDegradesToCommand guards the documented divergence: a
// dollar-quoted (heredoc) UDF body isn't modeled (no exp.Heredoc Kind), so the whole CREATE
// must degrade to a Command rather than mis-render the body as a plain string literal.
func TestCreateFunctionUnsupportedBodyDegradesToCommand(t *testing.T) {
	sql := "CREATE FUNCTION pymax(a INT, b INT) RETURNS INT LANGUAGE plpython3u AS $$\n  if a > b:\n    return a\n  return b\n$$"
	roundTripCase(t, "postgres", sql, sql)
}

// TestCreateFunctionParameterModes ports PostgresParser's two-lookahead mode
// disambiguation and raw InOutColumnConstraint attachment.
func TestCreateFunctionParameterModes(t *testing.T) {
	create := parseOneDialect(t, "CREATE FUNCTION foo(IN a INT DEFAULT 0, OUT b INT, INOUT c INT, VARIADIC d INT[])", "postgres")
	if create.Kind() != exp.KindCreate {
		t.Fatalf("kind = %v, want Create:\n%s", create.Kind(), create.ToS())
	}
	parameters := expressionsForArg(exprArg(t, create, "this"), "expressions")
	if len(parameters) != 4 {
		t.Fatalf("parameter count = %d, want 4:\n%s", len(parameters), create.ToS())
	}
	want := []struct {
		input, output, variadic bool
	}{{true, false, false}, {false, true, false}, {true, true, false}, {false, false, true}}
	for i, parameter := range parameters {
		constraints := expressionsForArg(parameter, "constraints")
		if len(constraints) == 0 || constraints[0].Kind() != exp.KindInOutColumnConstraint {
			t.Fatalf("parameter %d mode constraint mismatch:\n%s", i, create.ToS())
		}
		mode := constraints[0]
		if mode.Arg("input_") != want[i].input || mode.Arg("output") != want[i].output || mode.Arg("variadic") != want[i].variadic {
			t.Fatalf("parameter %d mode flags mismatch:\n%s", i, create.ToS())
		}
	}
	assertToSContains(t, create,
		"InOutColumnConstraint(input_=True, output=False, variadic=False)",
		"InOutColumnConstraint(input_=False, output=False, variadic=True)",
	)

	// MODE TYPE means the keyword is a parameter name, not a mode.
	namedOut := parseOneDialect(t, "CREATE FUNCTION foo(out INT)", "postgres")
	parameter := expressionsForArg(exprArg(t, namedOut, "this"), "expressions")[0]
	if parameter.Name() != "out" || len(expressionsForArg(parameter, "constraints")) != 0 {
		t.Fatalf("out identifier disambiguation mismatch:\n%s", namedOut.ToS())
	}
}

// TestCreateFunctionPlainParameters guards that, absent any parameter-mode keyword, this
// slice's base parseFunctionParameter now builds a real structured UserDefinedFunction
// (previously this degraded to Command too).
func TestCreateFunctionPlainParameters(t *testing.T) {
	create := parseOneDialect(t, "CREATE FUNCTION foo(a INT)", "postgres")
	if create.Kind() != exp.KindCreate {
		t.Fatalf("kind = %v, want Create:\n%s", create.Kind(), create.ToS())
	}
	udf := exprArg(t, create, "this")
	if udf.Kind() != exp.KindUserDefinedFunction {
		t.Fatalf("this kind mismatch:\n%s", create.ToS())
	}
	params := expressionsForArg(udf, "expressions")
	if len(params) != 1 || params[0].Kind() != exp.KindColumnDef {
		t.Fatalf("params mismatch:\n%s", create.ToS())
	}
}

// TestCreateIndexDDLTailGap closes parity_gaps.txt 164: a full postgres CREATE INDEX with
// USING <method>(<columns>) and a WHERE predicate.
func TestCreateIndexDDLTailGap(t *testing.T) {
	sql := "\n            CREATE INDEX index_ci_builds_on_commit_id_and_artifacts_expireatandidpartial\n            ON public.ci_builds\n            USING btree (commit_id, artifacts_expire_at, id)\n            WHERE (\n                ((type)::text = 'Ci::Build'::text)\n                AND ((retried = false) OR (retried IS NULL))\n                AND ((name)::text = ANY (ARRAY[\n                    ('sast'::character varying)::text,\n                    ('dependency_scanning'::character varying)::text,\n                    ('sast:container'::character varying)::text,\n                    ('container_scanning'::character varying)::text,\n                    ('dast'::character varying)::text\n                ]))\n            )\n            "
	want := "CREATE INDEX index_ci_builds_on_commit_id_and_artifacts_expireatandidpartial ON public.ci_builds USING btree(commit_id, artifacts_expire_at, id) WHERE ((CAST((type) AS TEXT) = CAST('Ci::Build' AS TEXT)) AND ((retried = FALSE) OR (retried IS NULL)) AND (CAST((name) AS TEXT) = ANY(ARRAY[CAST((CAST('sast' AS VARCHAR)) AS TEXT), CAST((CAST('dependency_scanning' AS VARCHAR)) AS TEXT), CAST((CAST('sast:container' AS VARCHAR)) AS TEXT), CAST((CAST('container_scanning' AS VARCHAR)) AS TEXT), CAST((CAST('dast' AS VARCHAR)) AS TEXT)])))"
	roundTripCase(t, "postgres", sql, want)

	create := parseOneDialect(t, sql, "postgres")
	index := exprArg(t, create, "this")
	if create.Arg("kind") != "INDEX" || index.Kind() != exp.KindIndex {
		t.Fatalf("kind mismatch:\n%s", create.ToS())
	}
}

// TestCreateIndexOpclassAndAnonymous exercises the parseOpclass/parseIndexedColumn sibling
// cases (CREATE INDEX columns with an operator-class name, and an anonymous CREATE INDEX
// with no name at all).
func TestCreateIndexOpclassAndAnonymous(t *testing.T) {
	for _, sql := range []string{
		"CREATE INDEX foo ON bar.baz USING btree(col1 varchar_pattern_ops ASC, col2)",
		"CREATE INDEX index_issues_on_title_trigram ON public.issues USING gin(title public.gin_trgm_ops)",
		"CREATE INDEX IF NOT EXISTS ON t(c)",
		"CREATE INDEX et_vid_idx ON et(vid) INCLUDE (fid)",
	} {
		roundTripCase(t, "postgres", sql, sql)
	}
}

// TestCreateTriggerDDLTailGap closes parity_gaps.txt 181: EXECUTE PROCEDURE canonicalizes to
// EXECUTE FUNCTION on generation regardless of which keyword the source used.
func TestCreateTriggerDDLTailGap(t *testing.T) {
	roundTripCase(t, "postgres",
		"CREATE TRIGGER proc_trigger BEFORE INSERT ON users FOR EACH ROW EXECUTE PROCEDURE LOG_CHANGES()",
		"CREATE TRIGGER proc_trigger BEFORE INSERT ON users FOR EACH ROW EXECUTE FUNCTION LOG_CHANGES()")

	create := parseOneDialect(t, "CREATE TRIGGER proc_trigger BEFORE INSERT ON users FOR EACH ROW EXECUTE PROCEDURE LOG_CHANGES()", "postgres")
	if create.Kind() != exp.KindCreate || create.Arg("kind") != "TRIGGER" {
		t.Fatalf("kind mismatch:\n%s", create.ToS())
	}
	props := exprArg(t, create, "properties")
	trig := expressionsForArg(props, "expressions")
	if len(trig) != 1 || trig[0].Kind() != exp.KindTriggerProperties {
		t.Fatalf("trigger properties mismatch:\n%s", create.ToS())
	}
	if trig[0].Arg("timing") != "BEFORE" {
		t.Fatalf("timing mismatch:\n%s", create.ToS())
	}
}

// TestCreateConstraintTrigger exercises the CREATE CONSTRAINT TRIGGER form (constraint=true
// on TriggerProperties, re-adding the "CONSTRAINT " kind prefix at generation time) plus the
// fuller REFERENCING/DEFERRABLE/WHEN/multi-event feature set.
func TestCreateConstraintTrigger(t *testing.T) {
	for _, sql := range []string{
		"CREATE CONSTRAINT TRIGGER check_fk AFTER INSERT ON orders FROM customers FOR EACH ROW EXECUTE FUNCTION CHECK_CUSTOMER_EXISTS()",
		"CREATE CONSTRAINT TRIGGER deferred_check AFTER INSERT ON orders DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION CHECK_ORDER()",
		"CREATE TRIGGER complex_when BEFORE UPDATE ON accounts FOR EACH ROW WHEN (OLD.balance <> NEW.balance) EXECUTE FUNCTION LOG_CHANGES()",
		"CREATE TRIGGER track_changes AFTER UPDATE ON accounts REFERENCING OLD TABLE AS old_data NEW TABLE AS new_data FOR EACH ROW EXECUTE FUNCTION LOG_CHANGES()",
		"CREATE TRIGGER all_events AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON audit FOR EACH STATEMENT EXECUTE FUNCTION LOG_CHANGES()",
	} {
		roundTripCase(t, "postgres", sql, sql)
	}

	create := parseOneDialect(t, "CREATE CONSTRAINT TRIGGER check_fk AFTER INSERT ON orders FROM customers FOR EACH ROW EXECUTE FUNCTION CHECK_CUSTOMER_EXISTS()", "postgres")
	props := exprArg(t, create, "properties")
	trig := expressionsForArg(props, "expressions")
	if len(trig) != 1 || trig[0].Arg("constraint") != true {
		t.Fatalf("constraint=true mismatch:\n%s", create.ToS())
	}
}

// TestCreateTableDDLTailGap closes parity_gaps.txt 86: a column's bare (no-paren)
// CURRENT_TIMESTAMP DEFAULT/ON UPDATE value, and the mysql table-tail
// CHARSET/ROW_FORMAT properties (with CHARSET canonicalizing to CHARACTER SET on
// generation).
func TestCreateTableDDLTailGap(t *testing.T) {
	roundTripCase(t, "mysql",
		"CREATE TABLE t (c DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP) DEFAULT CHARSET=utf8 ROW_FORMAT=DYNAMIC",
		"CREATE TABLE t (c DATETIME DEFAULT CURRENT_TIMESTAMP() ON UPDATE CURRENT_TIMESTAMP()) DEFAULT CHARACTER SET=utf8 ROW_FORMAT=DYNAMIC")

	// The property-only tail: no ON UPDATE / no-paren-function involved.
	roundTripCase(t, "mysql",
		"CREATE TABLE z (a INT) ENGINE=InnoDB AUTO_INCREMENT=1 CHARACTER SET=utf8 COLLATE=utf8_bin COMMENT='x'",
		"CREATE TABLE z (a INT) ENGINE=InnoDB AUTO_INCREMENT=1 CHARACTER SET=utf8 COLLATE=utf8_bin COMMENT='x'")
}
