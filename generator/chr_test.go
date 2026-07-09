package generator_test

// Round-trip checks for chrSQL (generator/string_ext.go), which ports chr_sql
// (generator.py:6190-6194) plus MySQL's CHR->CHAR rename (generators/mysql.py:160). Cases are
// drawn from testdata/dialect_identity.jsonl (mysql CHAR/CONVERT entries), confirmed against
// the pinned oracle:
//
//	PYTHONPATH=.reference/sqlglot-v30.12.0 python3 -c \
//	  "import sqlglot; print(sqlglot.transpile(\"SELECT CHAR(65 USING BINARY)\", read='mysql', write='mysql')[0])"
//	SELECT CHAR(65 USING BINARY)

import "testing"

func TestChrSQL(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"mysql", "CHAR(0)", "CHAR(0)"},
		{"mysql", "CHAR(77, 121, 83, 81, '76')", "CHAR(77, 121, 83, 81, '76')"},
		{"mysql", "CHAR(77, 77.3, '77.3' USING utf8mb4)", "CHAR(77, 77.3, '77.3' USING utf8mb4)"},
		{"mysql", "SELECT CHAR(65 USING BINARY)", "SELECT CHAR(65 USING BINARY)"},
		{"mysql", "SELECT CHAR(65 USING `binary`)", "SELECT CHAR(65 USING binary)"},
		{"mysql", "SELECT CHAR(65 USING `my charset`)", "SELECT CHAR(65 USING `my charset`)"},
		// NOTE: dialect_identity.jsonl:227 "SELECT CHARSET(CHAR(100 USING utf8))" is deliberately
		// NOT covered here - CHARSET(...) is a separate, still-unimplemented function (a
		// different parity gap, parity_gaps.txt:153) outside this slice's CHR/CONVERT scope.
		{"mysql", "CONVERT('a' USING binary)", "CAST('a' AS CHAR CHARACTER SET binary)"},
		{"mysql", "SELECT CONVERT(`col` USING `utf8mb4`)", "SELECT CAST(`col` AS CHAR CHARACTER SET utf8mb4)"},
		{"mysql", "SELECT CONVERT(x USING `binary`)", "SELECT CAST(x AS CHAR CHARACTER SET binary)"},
		{"mysql", "SELECT CONVERT(x USING `my charset`)", "SELECT CAST(x AS CHAR CHARACTER SET `my charset`)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}

// TestChrSQLBaseName guards the base/postgres CHR spelling (no MySQL CHAR rename).
func TestChrSQLBaseName(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"", "SELECT CHR(65)", "SELECT CHR(65)"},
		{"postgres", "SELECT CHR(65)", "SELECT CHR(65)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}
