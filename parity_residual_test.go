package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
)

// TestResidualParityFixes locks in the integrator residual-fix round: each case is
// oracle-verified against the pinned reference (sqlglot v30.12.0). The four cases that are
// also corpus rows (mysql PARTITION selection, mysql reserved-keyword quoting, postgres MERGE
// target unqualification, postgres date_add(current_date, ...)) are additionally guarded by
// TestCorpus + the raised minPass* floors; CURDATE and MERGE `UPDATE *` are NOT corpus rows,
// so this is their only regression guard.
func TestResidualParityFixes(t *testing.T) {
	cases := []struct {
		name    string
		dialect string
		sql     string
		want    string
	}{
		// dialect-funcs: CURDATE -> CurrentDate renders bare CURRENT_DATE (no parens); CURTIME
		// still keeps the parenthesized fallback (only currentdate_sql exists upstream).
		{"mysql_curdate", "mysql", "SELECT CURDATE()", "SELECT CURRENT_DATE"},
		{"mysql_curtime", "mysql", "SELECT CURTIME()", "SELECT CURRENT_TIME()"},
		// no-paren CURRENT_DATE keyword (NO_PAREN_FUNCTIONS): the corpus gap plus a couple of
		// base cases that must NOT regress (bare CURRENT_DATE, and current_date as a table name).
		{"pg_date_add_current_date", "postgres", "SELECT date_add(current_date, interval '7' day)", "SELECT CURRENT_DATE + INTERVAL '7 DAY'"},
		{"base_current_date", "", "SELECT CURRENT_DATE", "SELECT CURRENT_DATE"},
		{"base_current_date_at_tz", "", "SELECT CURRENT_DATE AT TIME ZONE 'UTC'", "SELECT CURRENT_DATE AT TIME ZONE 'UTC'"},
		{"base_current_date_table", "", "SELECT * FROM current_date", "SELECT * FROM current_date"},
		// from-dml: mysql FROM-clause PARTITION(...) selection (SUPPORTS_PARTITION_SELECTION);
		// base keeps it as a plain alias, matching upstream.
		{"mysql_partition_from", "mysql", "SELECT * FROM t1 PARTITION(p0)", "SELECT * FROM t1 PARTITION(p0)"},
		// generator: mysql reserved-keyword identifier quoting; base/postgres leave it unquoted.
		{"mysql_reserved_alias", "mysql", "SELECT 1 AS row", "SELECT 1 AS `row`"},
		{"base_reserved_alias", "", "SELECT 1 AS row", "SELECT 1 AS row"},
		{"pg_reserved_alias", "postgres", "SELECT 1 AS row", "SELECT 1 AS row"},
		// generator: postgres MERGE removes the target-table qualifier from WHEN update/insert
		// columns (merge_without_target_sql); base leaves them intact.
		{"pg_merge_unqualify", "postgres", "MERGE INTO x USING (SELECT id) AS y ON a = b WHEN MATCHED THEN UPDATE SET x.a = y.b WHEN NOT MATCHED THEN INSERT (a, b) VALUES (y.a, y.b)", "MERGE INTO x USING (SELECT id) AS y ON a = b WHEN MATCHED THEN UPDATE SET a = y.b WHEN NOT MATCHED THEN INSERT (a, b) VALUES (y.a, y.b)"},
		{"base_merge_keeps_qualifier", "", "MERGE INTO x USING (SELECT id) AS y ON a = b WHEN MATCHED THEN UPDATE SET x.a = y.b WHEN NOT MATCHED THEN INSERT (a, b) VALUES (y.a, y.b)", "MERGE INTO x USING (SELECT id) AS y ON a = b WHEN MATCHED THEN UPDATE SET x.a = y.b WHEN NOT MATCHED THEN INSERT (a, b) VALUES (y.a, y.b)"},
		// generator: MERGE `UPDATE *` renders bare (not `UPDATE SET *`) for every dialect.
		{"pg_merge_update_star", "postgres", "MERGE INTO x USING y ON a = b WHEN MATCHED THEN UPDATE *", "MERGE INTO x USING y ON a = b WHEN MATCHED THEN UPDATE *"},

		// --- second residual round (dialect-divergence review findings) ---
		// dialect-funcs: parenthesized CURRENT_DATE() builds a CurrentDate node too (it's a
		// base FUNCTION_BY_NAME entry), rendering bare CURRENT_DATE for every dialect.
		{"base_current_date_paren", "", "SELECT CURRENT_DATE()", "SELECT CURRENT_DATE"},
		{"mysql_current_date_paren", "mysql", "SELECT CURRENT_DATE()", "SELECT CURRENT_DATE"},
		{"pg_current_date_paren", "postgres", "SELECT CURRENT_DATE()", "SELECT CURRENT_DATE"},
		{"pg_date_add_current_date_paren", "postgres", "SELECT date_add(current_date(), interval '7' day)", "SELECT CURRENT_DATE + INTERVAL '7 DAY'"},
		{"base_current_date_zone", "", "SELECT CURRENT_DATE('UTC')", "SELECT CURRENT_DATE('UTC')"},
		// ddl: a bare CURRENT_DATE column-constraint default now builds CurrentDate (renders
		// bare); CURRENT_TIMESTAMP still keeps the parenthesized Anonymous form (no Kind yet).
		{"base_default_current_date", "", "CREATE TABLE t (c DATE DEFAULT CURRENT_DATE)", "CREATE TABLE t (c DATE DEFAULT CURRENT_DATE)"},
		{"mysql_default_current_date", "mysql", "CREATE TABLE t (c DATE DEFAULT CURRENT_DATE)", "CREATE TABLE t (c DATE DEFAULT CURRENT_DATE)"},
		{"mysql_default_current_timestamp", "mysql", "CREATE TABLE t (c DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP)", "CREATE TABLE t (c DATETIME DEFAULT CURRENT_TIMESTAMP() ON UPDATE CURRENT_TIMESTAMP())"},
		// dialect-funcs: base/postgres recognize the no-underscore day/week spellings +
		// LCASE/UCASE (base FUNCTION_BY_NAME), canonicalizing to DAY_OF_MONTH/LOWER; mysql keeps
		// its own spelling. Also guards the mysql.go dedup (DAY_OF_MONTH still -> DAYOFMONTH).
		{"base_dayofmonth", "", "SELECT DAYOFMONTH(x)", "SELECT DAY_OF_MONTH(x)"},
		{"pg_dayofmonth", "postgres", "SELECT DAYOFMONTH(x)", "SELECT DAY_OF_MONTH(x)"},
		{"mysql_dayofmonth", "mysql", "SELECT DAYOFMONTH(x)", "SELECT DAYOFMONTH(x)"},
		{"mysql_day_of_month_dedup", "mysql", "SELECT DAY_OF_MONTH(x)", "SELECT DAYOFMONTH(x)"},
		{"base_lcase", "", "SELECT LCASE(x)", "SELECT LOWER(x)"},
		// generator: SELECT ... INTO is inline only for postgres (SUPPORTS_SELECT_INTO); base/
		// mysql rewrite it to CTAS (UNLOGGED is dropped since base/mysql lack unlogged tables).
		{"base_select_into_ctas", "", "SELECT * INTO foo FROM bar", "CREATE TABLE foo AS SELECT * FROM bar"},
		{"mysql_select_into_ctas", "mysql", "SELECT * INTO foo FROM bar", "CREATE TABLE foo AS SELECT * FROM bar"},
		{"pg_select_into_inline", "postgres", "SELECT * INTO foo FROM bar", "SELECT * INTO foo FROM bar"},
		{"base_select_into_unlogged_ctas", "", "SELECT * INTO UNLOGGED foo FROM bar", "CREATE TABLE foo AS SELECT * FROM bar"},
		{"pg_select_into_unlogged_inline", "postgres", "SELECT * INTO UNLOGGED foo FROM bar", "SELECT * INTO UNLOGGED foo FROM bar"},
		// generator: postgres renders placeholders in pyformat (%(name)s / %s); base keeps :name.
		{"pg_placeholder_named", "postgres", "SELECT * FROM x LIMIT :my_limit", "SELECT * FROM x LIMIT %(my_limit)s"},
		{"pg_placeholder_select", "postgres", "SELECT :hello", "SELECT %(hello)s"},
		{"base_placeholder_named", "", "SELECT * FROM x LIMIT :my_limit", "SELECT * FROM x LIMIT :my_limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := sqlglot.ParseOne(tc.sql, tc.dialect)
			if err != nil {
				t.Fatalf("ParseOne(%q, %q) error: %v", tc.sql, tc.dialect, err)
			}
			got, err := sqlglot.Generate(expr, tc.dialect, generator.Options{})
			if err != nil {
				t.Fatalf("Generate error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("round-trip mismatch\n  dialect: %q\n  sql:  %s\n  got:  %s\n  want: %s", tc.dialect, tc.sql, got, tc.want)
			}
		})
	}
}

// TestReviewFindingsFixes locks in the integrator quality-pass round that closed the
// dialect-divergence review findings: the CONCAT(...) function and adjacent-string Concat, the
// byte/hex/bit-string int(text, base) validation (quoted sign/prefix payloads, bare
// underscore-after-prefix), postgres pyformat placeholder names (any_token), and VARIADIC.
// Every expected value is oracle-verified against the pinned reference (sqlglot v30.12.0). None
// of these are corpus rows: the corpus is same read==write and does not cover cross-dialect
// transpile, bare 0x / underscore literals, keyword/numeric placeholder names, VARIADIC as a
// function call, or hand-built ASTs setting the exposed optional flags. This is their only guard.
func TestReviewFindingsFixes(t *testing.T) {
	transpile := []struct {
		name, read, write, sql, want string
	}{
		// Adjacent string literals build a coalesce=false Concat under base; postgres CONCAT
		// coalesces NULLs to empty string, so it must transpile to the || operator
		// (concat_to_dpipe_sql) to preserve NULL propagation rather than emitting CONCAT(...).
		{"concat_base_to_pg_dpipe", "", "postgres", "SELECT 'a' 'b'", "SELECT 'a' || 'b'"},
		{"concat_pg_to_base", "postgres", "", "SELECT 'a' 'b'", "SELECT CONCAT('a', 'b')"},
		// Bare 0x / 0b are hex / bit strings for postgres too (has_hex/bit_strings derives from a
		// non-empty HEX_STRINGS / BIT_STRINGS, dialects/postgres.py:65-66).
		{"pg_bare_hex", "postgres", "postgres", "SELECT 0xFF", "SELECT x'FF'"},
		{"pg_bare_bit", "postgres", "postgres", "SELECT 0b101", "SELECT b'101'"},
		{"mysql_bare_hex", "mysql", "mysql", "SELECT 0xA_B", "SELECT x'A_B'"},
		// Underscores are valid digit separators in a bit / hex payload (Python int(text, base)).
		{"mysql_hex_underscore", "mysql", "mysql", "SELECT x'A_B'", "SELECT x'A_B'"},
		{"mysql_bit_underscore", "mysql", "mysql", "SELECT b'10_1'", "SELECT b'10_1'"},
		{"pg_hex_underscore", "postgres", "postgres", "SELECT x'A_B'", "SELECT x'A_B'"},
		// Base has no hex family: a hex literal renders as its integer value, underscores stripped
		// just as Python int(this, 16) accepts them (0xA_B -> 171).
		{"mysql_hex_to_base_int", "mysql", "", "SELECT x'A_B'", "SELECT 171"},
		// VARIADIC is a postgres-only no-paren operator; base / mysql parse VARIADIC(x) as an
		// ordinary function call instead of degrading to VARIADIC AS (x).
		{"base_variadic_func", "", "", "SELECT VARIADIC(x)", "SELECT VARIADIC(x)"},
		{"mysql_variadic_func", "mysql", "mysql", "SELECT VARIADIC(x)", "SELECT VARIADIC(x)"},
		{"pg_variadic_operator", "postgres", "postgres", "SELECT MLEAST(VARIADIC ARRAY[]::numeric[])", "SELECT MLEAST(VARIADIC CAST(ARRAY[] AS DECIMAL[]))"},
		// CONCAT(...) is a registered function (parser.parseConcat), not an Anonymous call, so it
		// transpiles like the adjacent-string Concat: base's coalesce=false CONCAT becomes `||`
		// under postgres, and postgres's coalesce=true CONCAT wraps each arg in COALESCE under
		// mysql. Same-dialect it stays CONCAT(...); a lowercase name is normalized to CONCAT.
		{"concat_fn_base_to_pg", "", "postgres", "SELECT CONCAT(a, b)", "SELECT a || b"},
		{"concat_fn_pg_to_mysql", "postgres", "mysql", "SELECT CONCAT(a, b)", "SELECT CONCAT(COALESCE(a, ''), COALESCE(b, ''))"},
		{"concat_fn_identity_base", "", "", "SELECT CONCAT(a, b)", "SELECT CONCAT(a, b)"},
		{"concat_fn_lowercased", "mysql", "mysql", "SELECT concat(a, b)", "SELECT CONCAT(a, b)"},
		// Quoted hex/bit payloads are validated by int(payload, base), which honors a leading sign
		// and a matching base prefix, so these round-trip verbatim (the delimited form re-emits the
		// payload without re-int-ing it); an all-invalid payload like x'GG' errors at tokenize time.
		{"hex_quoted_0x_prefix", "mysql", "mysql", "SELECT x'0xA'", "SELECT x'0xA'"},
		{"hex_quoted_plus_sign", "postgres", "postgres", "SELECT x'+A'", "SELECT x'+A'"},
		{"hex_quoted_minus_sign", "mysql", "mysql", "SELECT x'-FF'", "SELECT x'-FF'"},
		// Bare 0x_/0b_ (underscore right after the prefix) is valid: int("0x_FF", 16) parses, and
		// the stored payload keeps the underscore, so it renders as x'_FF' / b'_101'.
		{"bare_hex_underscore_prefix", "mysql", "mysql", "SELECT 0x_FF", "SELECT x'_FF'"},
		{"bare_bit_underscore_prefix", "postgres", "postgres", "SELECT 0b_101", "SELECT b'_101'"},
		// Postgres pyformat placeholder names use _parse_id_var(any_token=True): a non-reserved
		// keyword or a number is a valid name, not only a bare identifier.
		{"pg_placeholder_keyword_name", "postgres", "postgres", "SELECT %(from)s", "SELECT %(from)s"},
		{"pg_placeholder_numeric_name", "postgres", "postgres", "SELECT %(1)s", "SELECT %(1)s"},
	}
	for _, tc := range transpile {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sqlglot.Transpile(tc.sql, tc.read, tc.write, generator.Options{})
			if err != nil {
				t.Fatalf("Transpile(%q, %q, %q) error: %v", tc.sql, tc.read, tc.write, err)
			}
			if got[0] != tc.want {
				t.Fatalf("transpile mismatch\n  read:  %q\n  write: %q\n  sql:   %s\n  got:   %s\n  want:  %s", tc.read, tc.write, tc.sql, got[0], tc.want)
			}
		})
	}

	// An empty bit / hex string generated to the base dialect has no digits for int(payload, base),
	// which raises upstream (ValueError) rather than silently dropping the projection.
	t.Run("empty_hex_to_base_errors", func(t *testing.T) {
		if out, err := sqlglot.Transpile("SELECT x''", "mysql", "", generator.Options{}); err == nil {
			t.Fatalf("expected an error generating an empty hex string to base, got %q", out)
		}
	})

	// A bare 0x_FF validates the full text (int("0x_FF", 16)) but stores payload "_FF"; folding
	// that to base runs int("_FF", 16), which raises upstream - so it errors here too rather than
	// emitting an empty/invalid literal. (Upstream is asymmetric this way; we match it.)
	t.Run("bare_hex_underscore_prefix_to_base_errors", func(t *testing.T) {
		if out, err := sqlglot.Transpile("SELECT 0x_FF", "mysql", "", generator.Options{}); err == nil {
			t.Fatalf("expected an error folding 0x_FF to base, got %q", out)
		}
	})

	// A hand-built HexString whose payload is not a valid int(payload, 16) - a doubled underscore
	// here - errors when folded to base, matching CPython int("A__B", 16) raising ValueError
	// (rather than silently stripping the underscores as an earlier draft did).
	t.Run("invalid_hex_payload_to_base_errors", func(t *testing.T) {
		e := expressions.HexString(expressions.Args{"this": "A__B"})
		if out, err := sqlglot.Generate(e, "", generator.Options{}); err == nil {
			t.Fatalf("expected an error generating HexString{A__B} to base, got %q", out)
		}
	})

	// Hand-built ASTs exercising the exposed optional flags (is_integer / is_bytes / concat
	// coalesce) that no in-scope parser sets, so nothing else guards these generator branches.
	col := func(n string) expressions.Expression {
		return expressions.Column(expressions.Args{"this": expressions.Identifier(expressions.Args{"this": n})})
	}
	concat := func(coalesce bool, args ...expressions.Expression) expressions.Expression {
		return expressions.Concat(expressions.Args{"expressions": args, "coalesce": coalesce})
	}
	genCases := []struct {
		name    string
		dialect string
		expr    expressions.Expression
		want    string
	}{
		// is_integer hex renders as its integer value even where the write dialect has a HEX_START
		// (HEX_STRING_IS_INTEGER_TYPE is false for base/mysql/postgres).
		{"hex_is_integer_pg", "postgres", expressions.HexString(expressions.Args{"this": "FF", "is_integer": true}), "255"},
		{"hex_is_integer_mysql", "mysql", expressions.HexString(expressions.Args{"this": "FF", "is_integer": true}), "255"},
		// Folding a hex literal to base runs int(payload, 16), which honors a base prefix
		// (int("0xA", 16) == 10) - so a hand-built HexString{"0xA"} generates 10, not an error.
		{"hex_0x_prefix_fold_base", "", expressions.HexString(expressions.Args{"this": "0xA"}), "10"},
		// is_bytes byte string casts the re-quoted literal to BINARY (BYTEA in postgres), not a
		// plain string literal.
		{"byte_is_bytes_pg", "postgres", expressions.ByteString(expressions.Args{"this": "abc", "is_bytes": true}), "CAST(e'abc' AS BYTEA)"},
		// coalesce=true Concat to a non-coalescing dialect wraps each non-string arg in COALESCE;
		// string-literal args are left alone.
		{"concat_coalesce_cols_base", "", concat(true, col("a"), col("b")), "CONCAT(COALESCE(a, ''), COALESCE(b, ''))"},
		{"concat_coalesce_strlit_base", "", concat(true, expressions.LiteralString("x"), expressions.LiteralString("y")), "CONCAT('x', 'y')"},
		{"concat_coalesce_cols_pg_dpipe", "postgres", concat(false, col("a"), col("b")), "a || b"},
	}
	for _, tc := range genCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sqlglot.Generate(tc.expr, tc.dialect, generator.Options{})
			if err != nil {
				t.Fatalf("Generate(%q) error: %v", tc.dialect, err)
			}
			if got != tc.want {
				t.Fatalf("generate mismatch\n  dialect: %q\n  got:  %s\n  want: %s", tc.dialect, got, tc.want)
			}
		})
	}
}
