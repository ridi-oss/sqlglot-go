package generator_test

// Round-trip checks for varianceSQL/variancePopSQL (generator/rename.go), which port
// rename_func("VAR_SAMP")/rename_func("VAR_POP") applied to exp.Variance/exp.VariancePop
// (generators/postgres.py:383-384). Cases are drawn from testdata/dialect_identity.jsonl.

import "testing"

func TestVarianceSQLPostgres(t *testing.T) {
	cases := []struct{ sql, want string }{
		{"SELECT VARIANCE(x)", "SELECT VAR_SAMP(x)"},
		{"SELECT VAR_SAMP(x)", "SELECT VAR_SAMP(x)"},
		{"SELECT VAR_POP(x)", "SELECT VAR_POP(x)"},
		{"SELECT VARIANCE_POP(x)", "SELECT VAR_POP(x)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, "postgres", tc.sql); got != tc.want {
			t.Errorf("postgres %q ->\n  got  %q\n  want %q", tc.sql, got, tc.want)
		}
	}
}

// TestVarianceSQLFallback guards that registering the KindVariance/KindVariancePop
// dispatch entries leaves base and mysql on the default names (VARIANCE/VARIANCE_POP) via
// functionFallbackSQL, matching pre-dispatch-entry behavior for those dialects.
func TestVarianceSQLFallback(t *testing.T) {
	cases := []struct{ dialect, sql, want string }{
		{"", "SELECT VARIANCE(x)", "SELECT VARIANCE(x)"},
		{"", "SELECT VAR_POP(x)", "SELECT VARIANCE_POP(x)"},
		{"mysql", "SELECT VARIANCE(x)", "SELECT VARIANCE(x)"},
		{"mysql", "SELECT VAR_POP(x)", "SELECT VARIANCE_POP(x)"},
	}
	for _, tc := range cases {
		if got := roundTrip(t, tc.dialect, tc.sql); got != tc.want {
			t.Errorf("%s %q ->\n  got  %q\n  want %q", tc.dialect, tc.sql, got, tc.want)
		}
	}
}
