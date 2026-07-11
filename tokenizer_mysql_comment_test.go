package sqlglot_test

import (
	"testing"

	sqlglot "github.com/sjincho/sqlglot-go"
	"github.com/sjincho/sqlglot-go/generator"
)

// TestMySQLDashDashComment locks in the MySQL `--` rule (DEVIATIONS §1): `--` only
// starts a line comment when followed by whitespace/control or EOF; otherwise it is two
// `-` operators (`1--2` == `1 - -2`). Upstream sqlglot mis-tokenizes this (drops `--2`
// as a comment); we match the real engine. Postgres keeps the standard unconditional
// `--` comment. This is the correctness property proxy-monster's grant-hash normalizer
// relies on when it rebuilds over the shared tokenizer.
func TestMySQLDashDashComment(t *testing.T) {
	tokenTexts := func(t *testing.T, sql, dialect string) []string {
		t.Helper()
		toks, err := sqlglot.Tokenize(sql, dialect)
		if err != nil {
			t.Fatalf("Tokenize(%q, %q): %v", sql, dialect, err)
		}
		out := make([]string, 0, len(toks))
		for _, tk := range toks {
			out = append(out, tk.Text)
		}
		return out
	}

	cases := []struct {
		name    string
		dialect string
		sql     string
		want    []string
	}{
		{"mysql arithmetic no space", "mysql", "SELECT 1--2", []string{"SELECT", "1", "-", "-", "2"}},
		{"mysql comment with space", "mysql", "SELECT 1-- 2", []string{"SELECT", "1"}},
		{"mysql comment with newline", "mysql", "SELECT 1--\n2", []string{"SELECT", "1", "2"}},
		{"mysql comment with tab", "mysql", "SELECT 1--\t2", []string{"SELECT", "1"}},
		{"mysql trailing marker at EOF", "mysql", "SELECT 1 --", []string{"SELECT", "1"}},
		{"mysql triple dash", "mysql", "SELECT a---b", []string{"SELECT", "a", "-", "-", "-", "b"}},
		{"mysql normal comment mid-query", "mysql", "SELECT 1 -- c\nFROM t", []string{"SELECT", "1", "FROM", "t"}},
		// Postgres is unchanged: `--` is always a comment (standard SQL), so `--2` is dropped.
		{"postgres unconditional comment", "postgres", "SELECT 1--2", []string{"SELECT", "1"}},
		{"postgres comment with space", "postgres", "SELECT 1-- 2", []string{"SELECT", "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenTexts(t, tc.sql, tc.dialect)
			if len(got) != len(tc.want) {
				t.Fatalf("tokens = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("tokens = %v, want %v", got, tc.want)
				}
			}
		})
	}

	// The tokens must also parse+generate as arithmetic, matching real MySQL (1--2 = 3).
	for _, tc := range []struct{ sql, want string }{
		{"SELECT 1--2", "SELECT 1 - -2"},
		{"SELECT 5--3", "SELECT 5 - -3"},
	} {
		e, err := sqlglot.ParseOne(tc.sql, "mysql")
		if err != nil {
			t.Fatalf("ParseOne(%q, mysql): %v", tc.sql, err)
		}
		out, err := sqlglot.Generate(e, "mysql", generator.Options{})
		if err != nil {
			t.Fatalf("Generate(%q): %v", tc.sql, err)
		}
		if out != tc.want {
			t.Fatalf("round-trip %q = %q, want %q", tc.sql, out, tc.want)
		}
	}
}
