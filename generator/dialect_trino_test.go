package generator_test

import (
	"strings"
	"testing"
)

// Pinned v30.17.0 tests/dialects/test_trino.py and generators/trino.py.
func TestTrinoGeneratorParity(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"trino", "SELECT LISTAGG(DISTINCT col, ',') WITHIN GROUP (ORDER BY col ASC) FROM tbl", "SELECT LISTAGG(DISTINCT col, ',') WITHIN GROUP (ORDER BY col ASC) FROM tbl"},
		{"trino", "SELECT LISTAGG(col) WITHIN GROUP (ORDER BY col DESC) FROM tbl", "SELECT LISTAGG(col, ',') WITHIN GROUP (ORDER BY col DESC) FROM tbl"},
		{"trino", "SELECT LISTAGG(col, '; ' ON OVERFLOW ERROR) WITHIN GROUP (ORDER BY col ASC) FROM tbl", "SELECT LISTAGG(col, '; ' ON OVERFLOW ERROR) WITHIN GROUP (ORDER BY col ASC) FROM tbl"},
		{"trino", "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE '...' WITH COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl", "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE '...' WITH COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl"},
		{"trino", "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE '...' WITHOUT COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl", "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE '...' WITHOUT COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl"},
		{"trino", "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE WITH COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl", "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE WITH COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl"},
		{"trino", "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE WITHOUT COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl", "SELECT LISTAGG(col, '; ' ON OVERFLOW TRUNCATE WITHOUT COUNT) WITHIN GROUP (ORDER BY col ASC) FROM tbl"},
		{"trino", "SELECT TRIM('!' FROM '!foo!')", "SELECT TRIM('!' FROM '!foo!')"},
		{"trino", "SELECT TRIM('!foo!', '!')", "SELECT TRIM('!' FROM '!foo!')"},
		{"trino", "SELECT TRIM(BOTH '$' FROM '$var$')", "SELECT TRIM(BOTH '$' FROM '$var$')"},
		{"trino", "SELECT TRIM(TRAILING 'ER' FROM UPPER('worker'))", "SELECT TRIM(TRAILING 'ER' FROM UPPER('worker'))"},
		{"trino", "SELECT VERSION()", "SELECT VERSION()"},
		{"trino", "SELECT TIMESTAMP '2012-10-31 01:00 +2'", "SELECT CAST('2012-10-31 01:00 +2' AS TIMESTAMP WITH TIME ZONE)"},
		{"trino", "SELECT TIMESTAMP '2012-10-31 01:00 -2'", "SELECT CAST('2012-10-31 01:00 -2' AS TIMESTAMP WITH TIME ZONE)"},
		{"trino", "CREATE TABLE \"foo\" (\"a\" VARCHAR, \"b\" INTEGER, \"c\" DATE) WITH (PARTITIONED_BY=ARRAY['a', 'b'])", "CREATE TABLE \"foo\" (\"a\" VARCHAR, \"b\" INTEGER, \"c\" DATE) WITH (PARTITIONED_BY=ARRAY['a', 'b'])"},
		{"trino", "CREATE TABLE \"foo\" (\"a\" VARCHAR, \"b\" INTEGER, \"c\" DATE) WITH (PARTITIONING=ARRAY['a', 'bucket(4, b)', 'month(c)'])", "CREATE TABLE \"foo\" (\"a\" VARCHAR, \"b\" INTEGER, \"c\" DATE) WITH (PARTITIONING=ARRAY['a', 'bucket(4, b)', 'month(c)'])"},
		{"trino", "CREATE TABLE foo (a VARCHAR, b INTEGER, c DATE) WITH (PARTITIONED_BY=ARRAY['a', 'b'])", "CREATE TABLE foo (a VARCHAR, b INTEGER, c DATE) WITH (PARTITIONED_BY=ARRAY['a', 'b'])"},
		{"trino", "CREATE TABLE foo (a VARCHAR, b INTEGER, c DATE) WITH (PARTITIONING=ARRAY['a', 'bucket(4, b)', 'month(c)'])", "CREATE TABLE foo (a VARCHAR, b INTEGER, c DATE) WITH (PARTITIONING=ARRAY['a', 'bucket(4, b)', 'month(c)'])"},
		{"trino", "SELECT ROW(1, 'x')", "SELECT ROW(1, 'x')"},
	}
	for _, tc := range cases {
		if strings.HasPrefix(tc.sql, "SELECT") {
			t.Run("athena/"+tc.sql, func(t *testing.T) {
				if got := roundTrip(t, "athena", tc.sql); got != tc.want {
					t.Fatalf("got %s; want %s", got, tc.want)
				}
			})
		}
		t.Run(tc.dialect+"/"+tc.sql, func(t *testing.T) {
			if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}
