package sqlglot_test

import (
	"reflect"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	"github.com/ridi/sqlglot-go/generator"
)

type executableCommentToken struct {
	text     string
	comments []string
}

func executableCommentTokens(t *testing.T, sql, dialect string) []executableCommentToken {
	t.Helper()
	toks, err := sqlglot.Tokenize(sql, dialect)
	if err != nil {
		t.Fatalf("Tokenize(%q, %q): %v", sql, dialect, err)
	}
	out := make([]executableCommentToken, len(toks))
	for i, tok := range toks {
		out[i] = executableCommentToken{text: tok.Text, comments: append([]string(nil), tok.Comments...)}
	}
	return out
}

func executableCommentSQL(t *testing.T, sql, dialect string) string {
	t.Helper()
	expression, err := sqlglot.ParseOne(sql, dialect)
	if err != nil {
		t.Fatalf("ParseOne(%q, %q): %v", sql, dialect, err)
	}
	out, err := sqlglot.Generate(expression, dialect, generator.Options{})
	if err != nil {
		t.Fatalf("Generate(ParseOne(%q), %q): %v", sql, dialect, err)
	}
	return out
}

// TestMySQLVersionExecutableComments is an original NON-UPSTREAM opt-in behavioral-extension
// regression test. See DEVIATIONS.md §1.5.
func TestMySQLVersionExecutableComments(t *testing.T) {
	// mysql_version is the MYSQL_VERSION_ID integer (80033 == MySQL 8.0.33). The bare-integer
	// input + the "newer gate" case below (/*!80034 must stay INACTIVE) is the regression for the
	// mis-parse where a bare integer was read as a major version (80033 -> 800330000), which wrongly
	// activated near-boundary gates like /*!80034.
	versioned := "mysql, mysql_version=80033"

	t.Run("tokenize default remains comment metadata", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			sql  string
			want []executableCommentToken
		}{
			{
				name: "bare executable comment",
				sql:  "SELECT 1 /*! + 100 */",
				want: []executableCommentToken{{text: "SELECT"}, {text: "1", comments: []string{"! + 100 "}}},
			},
			{
				name: "gated executable comment",
				sql:  "SELECT 1 /*!50000 + 100 */",
				want: []executableCommentToken{{text: "SELECT"}, {text: "1", comments: []string{"!50000 + 100 "}}},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := executableCommentTokens(t, tc.sql, "mysql"); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("tokens = %#v, want %#v", got, tc.want)
				}
			})
		}
	})

	t.Run("tokenize versioned activation", func(t *testing.T) {
		active := []struct {
			name string
			sql  string
		}{
			{"bare executable comment", "SELECT 1 /*! + 100 */"},
			{"older gate", "SELECT 1 /*!50000 + 100 */"},
			{"exact gate", "SELECT 1 /*!80033 + 100 */"},
		}
		wantActive := []executableCommentToken{{text: "SELECT"}, {text: "1"}, {text: "+"}, {text: "100"}}
		for _, tc := range active {
			t.Run(tc.name, func(t *testing.T) {
				if got := executableCommentTokens(t, tc.sql, versioned); !reflect.DeepEqual(got, wantActive) {
					t.Fatalf("tokens = %#v, want %#v", got, wantActive)
				}
			})
		}

		for _, tc := range []struct {
			name    string
			sql     string
			comment string
		}{
			{"newer gate", "SELECT 1 /*!80034 + 100 */", "!80034 + 100 "},
			{"maximum gate", "SELECT 1 /*!99999 + 100 */", "!99999 + 100 "},
		} {
			t.Run(tc.name, func(t *testing.T) {
				want := []executableCommentToken{{text: "SELECT"}, {text: "1", comments: []string{tc.comment}}}
				if got := executableCommentTokens(t, tc.sql, versioned); !reflect.DeepEqual(got, want) {
					t.Fatalf("tokens = %#v, want %#v", got, want)
				}
			})
		}
	})

	t.Run("parse and generate", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			sql     string
			dialect string
			want    string
		}{
			{"active arithmetic", "SELECT 1 /*!50000 + 100 */", versioned, "SELECT 1 + 100"},
			{"inactive arithmetic", "SELECT 1 /*!99999 + 100 */", versioned, "SELECT 1 /* !99999 + 100 */"},
			{"default remains sanitized comment", "SELECT 1 /*!50000 + 100 */", "mysql", "SELECT 1 /* !50000 + 100 */"},
			{"active hidden column", "SELECT 1 /*!50000, rrn */ FROM t", versioned, "SELECT 1, rrn FROM t"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := executableCommentSQL(t, tc.sql, tc.dialect); got != tc.want {
					t.Fatalf("generated SQL = %q, want %q", got, tc.want)
				}
			})
		}
	})
}

func TestMySQLVersionIsInertOutsideMySQL(t *testing.T) {
	const sql = "SELECT 1 /*!50000 + 100 */"
	for _, dialect := range []string{
		"base, mysql_version=80033",
		"postgres, mysql_version=80033",
	} {
		t.Run(dialect, func(t *testing.T) {
			wantTokens := []executableCommentToken{{text: "SELECT"}, {text: "1", comments: []string{"!50000 + 100 "}}}
			if got := executableCommentTokens(t, sql, dialect); !reflect.DeepEqual(got, wantTokens) {
				t.Fatalf("tokens = %#v, want %#v", got, wantTokens)
			}
			if got, want := executableCommentSQL(t, sql, dialect), "SELECT 1 /* !50000 + 100 */"; got != want {
				t.Fatalf("generated SQL = %q, want %q", got, want)
			}
		})
	}
}

func TestUnknownDialectSettingStillErrors(t *testing.T) {
	const dialect = "mysql, executable_comments=true"
	if _, err := sqlglot.Tokenize("SELECT 1", dialect); err == nil {
		t.Errorf("Tokenize with unknown dialect setting: expected error, got nil")
	}
	if _, err := sqlglot.ParseOne("SELECT 1", dialect); err == nil {
		t.Errorf("ParseOne with unknown dialect setting: expected error, got nil")
	}
}
