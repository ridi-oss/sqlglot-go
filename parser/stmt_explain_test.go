package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

func assertPostgresExplainShape(t *testing.T, expression exp.Expression) {
	t.Helper()
	if expression.Kind() != exp.KindDescribe {
		t.Fatalf("kind = %v, want Describe:\n%s", expression.Kind(), expression.ToS())
	}
	if kind := expression.Text("kind"); kind != "EXPLAIN" {
		t.Fatalf("kind arg = %q, want EXPLAIN:\n%s", kind, expression.ToS())
	}
	if this := exprArg(t, expression, "this"); this.Kind() != exp.KindSelect {
		t.Fatalf("this kind = %v, want Select:\n%s", this.Kind(), expression.ToS())
	}
}

func assertExplainWrapped(t *testing.T, describe exp.Expression, want bool) {
	t.Helper()
	wrapped := describe.Arg("wrapped")
	if want {
		if value, ok := wrapped.(bool); !ok || !value {
			t.Fatalf("wrapped = %#v, want true:\n%s", wrapped, describe.ToS())
		}
		return
	}
	if wrapped != nil && wrapped != false {
		t.Fatalf("wrapped = %#v, want false or nil:\n%s", wrapped, describe.ToS())
	}
}

func TestParsePostgresExplain(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wrapped bool
	}{
		{in: "EXPLAIN SELECT 1", want: "EXPLAIN SELECT 1"},
		{in: "EXPLAIN (ANALYZE, FORMAT JSON) SELECT a FROM t", want: "EXPLAIN (ANALYZE, FORMAT JSON) SELECT a FROM t", wrapped: true},
		{in: "EXPLAIN ANALYZE SELECT 1", want: "EXPLAIN ANALYZE SELECT 1"},
		{in: "EXPLAIN VERBOSE SELECT 1", want: "EXPLAIN VERBOSE SELECT 1"},
		{in: "EXPLAIN ANALYZE VERBOSE SELECT 1", want: "EXPLAIN ANALYZE VERBOSE SELECT 1"},
		{
			in:      "EXPLAIN (ANALYZE TRUE, VERBOSE FALSE, COSTS ON, SETTINGS OFF, GENERIC_PLAN 1, BUFFERS 0, WAL TRUE, TIMING FALSE, SUMMARY ON, MEMORY OFF) SELECT 1",
			want:    "EXPLAIN (ANALYZE TRUE, VERBOSE FALSE, COSTS ON, SETTINGS OFF, GENERIC_PLAN 1, BUFFERS 0, WAL TRUE, TIMING FALSE, SUMMARY ON, MEMORY OFF) SELECT 1",
			wrapped: true,
		},
		{in: "EXPLAIN (SERIALIZE NONE) SELECT 1", want: "EXPLAIN (SERIALIZE NONE) SELECT 1", wrapped: true},
		{in: "EXPLAIN (SERIALIZE TEXT) SELECT 1", want: "EXPLAIN (SERIALIZE TEXT) SELECT 1", wrapped: true},
		{in: "EXPLAIN (SERIALIZE BINARY) SELECT 1", want: "EXPLAIN (SERIALIZE BINARY) SELECT 1", wrapped: true},
		{in: "EXPLAIN (FORMAT TEXT) SELECT 1", want: "EXPLAIN (FORMAT TEXT) SELECT 1", wrapped: true},
		{in: "EXPLAIN (FORMAT XML) SELECT 1", want: "EXPLAIN (FORMAT XML) SELECT 1", wrapped: true},
		{in: "EXPLAIN (FORMAT JSON) SELECT 1", want: "EXPLAIN (FORMAT JSON) SELECT 1", wrapped: true},
		{in: "EXPLAIN (FORMAT YAML) SELECT 1", want: "EXPLAIN (FORMAT YAML) SELECT 1", wrapped: true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			describe := parseOneDialect(t, tc.in, "postgres")
			assertPostgresExplainShape(t, describe)
			assertExplainWrapped(t, describe, tc.wrapped)

			got, err := generateSQL(t, describe, "postgres")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != tc.want {
				t.Fatalf("round-trip = %q, want %q", got, tc.want)
			}

			reparsed := parseOneDialect(t, got, "postgres")
			assertPostgresExplainShape(t, reparsed)
			assertExplainWrapped(t, reparsed, tc.wrapped)
		})
	}
}

func TestParsePostgresExplainOptionStructure(t *testing.T) {
	ledger := parseOneDialect(t, "EXPLAIN (ANALYZE, FORMAT JSON) SELECT a FROM t", "postgres")
	assertPostgresExplainShape(t, ledger)
	parameters := expressionsForArg(ledger, "expressions")
	if len(parameters) != 2 {
		t.Fatalf("option count = %d, want 2:\n%s", len(parameters), ledger.ToS())
	}
	if parameters[0].Kind() != exp.KindCopyParameter || parameters[0].Name() != "ANALYZE" {
		t.Fatalf("first option = %v %q, want CopyParameter ANALYZE:\n%s", parameters[0].Kind(), parameters[0].Name(), ledger.ToS())
	}
	if parameters[0].Arg("expression") != nil {
		t.Fatalf("ANALYZE expression = %#v, want nil:\n%s", parameters[0].Arg("expression"), ledger.ToS())
	}
	if parameters[1].Kind() != exp.KindCopyParameter || parameters[1].Name() != "FORMAT" {
		t.Fatalf("second option = %v %q, want CopyParameter FORMAT:\n%s", parameters[1].Kind(), parameters[1].Name(), ledger.ToS())
	}
	if value := exprArg(t, parameters[1], "expression").Text("this"); value != "JSON" {
		t.Fatalf("FORMAT expression = %q, want JSON:\n%s", value, ledger.ToS())
	}

	const sql = "EXPLAIN (ANALYZE TRUE, VERBOSE FALSE, COSTS ON, SETTINGS OFF, GENERIC_PLAN 1, BUFFERS 0, SERIALIZE BINARY, WAL TRUE, TIMING FALSE, SUMMARY ON, MEMORY OFF, FORMAT YAML) SELECT 1"
	describe := parseOneDialect(t, sql, "postgres")
	assertPostgresExplainShape(t, describe)
	parameters = expressionsForArg(describe, "expressions")
	want := []struct {
		name  string
		value string
	}{
		{name: "ANALYZE", value: "TRUE"},
		{name: "VERBOSE", value: "FALSE"},
		{name: "COSTS", value: "ON"},
		{name: "SETTINGS", value: "OFF"},
		{name: "GENERIC_PLAN", value: "1"},
		{name: "BUFFERS", value: "0"},
		{name: "SERIALIZE", value: "BINARY"},
		{name: "WAL", value: "TRUE"},
		{name: "TIMING", value: "FALSE"},
		{name: "SUMMARY", value: "ON"},
		{name: "MEMORY", value: "OFF"},
		{name: "FORMAT", value: "YAML"},
	}
	if len(parameters) != len(want) {
		t.Fatalf("option count = %d, want %d:\n%s", len(parameters), len(want), describe.ToS())
	}
	for i, expected := range want {
		parameter := parameters[i]
		if parameter.Kind() != exp.KindCopyParameter {
			t.Fatalf("option %d kind = %v, want CopyParameter:\n%s", i, parameter.Kind(), describe.ToS())
		}
		if name := parameter.Name(); name != expected.name {
			t.Fatalf("option %d name = %q, want %q:\n%s", i, name, expected.name, describe.ToS())
		}
		if value := exprArg(t, parameter, "expression").Text("this"); value != expected.value {
			t.Fatalf("option %s value = %q, want %q:\n%s", expected.name, value, expected.value, describe.ToS())
		}
	}
}

func TestParsePostgresExplainFailsClosed(t *testing.T) {
	for _, sql := range []string{
		"EXPLAIN () SELECT 1",
		"EXPLAIN (UNKNOWN TRUE) SELECT 1",
		"EXPLAIN (FORMAT) SELECT 1",
		"EXPLAIN (SERIALIZE JSON) SELECT 1",
		"EXPLAIN (ANALYZE TRUE FORMAT JSON) SELECT 1",
		"EXPLAIN (ANALYZE, FORMAT JSON,) SELECT 1",
		`EXPLAIN ("ANALYZE") SELECT 1`,
		`EXPLAIN (ANALYZE "TRUE") SELECT 1`,
		"EXPLAIN (ANALYZE 'TRUE') SELECT 1",
		"EXPLAIN (FORMAT 'JSON') SELECT 1",
		"EXPLAIN VERBOSE ANALYZE SELECT 1",
		"EXPLAIN ANALYZE ANALYZE SELECT 1",
		"EXPLAIN VERBOSE VERBOSE SELECT 1",
	} {
		t.Run(sql, func(t *testing.T) {
			root := parseOneDialect(t, sql, "postgres")
			if root.Kind() != exp.KindCommand {
				t.Fatalf("kind = %v, want Command:\n%s", root.Kind(), root.ToS())
			}
			got, err := generateSQL(t, root, "postgres")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != sql {
				t.Fatalf("round-trip = %q, want %q", got, sql)
			}
		})
	}
}

func TestPostgresExplainDialectGuardrails(t *testing.T) {
	t.Run("base explain remains command", func(t *testing.T) {
		const sql = "EXPLAIN SELECT 1"
		root := parseOne(t, sql)
		if root.Kind() != exp.KindCommand {
			t.Fatalf("kind = %v, want Command:\n%s", root.Kind(), root.ToS())
		}
		got, err := generateSQL(t, root, "")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	})

	t.Run("mysql explain remains describe", func(t *testing.T) {
		const sql = "EXPLAIN ANALYZE SELECT * FROM t"
		const want = "DESCRIBE ANALYZE SELECT * FROM t"
		describe := parseOneDialect(t, sql, "mysql")
		if describe.Kind() != exp.KindDescribe {
			t.Fatalf("kind = %v, want Describe:\n%s", describe.Kind(), describe.ToS())
		}
		got, err := generateSQL(t, describe, "mysql")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != want {
			t.Fatalf("round-trip = %q, want %q", got, want)
		}
	})

	t.Run("postgres describe remains unmarked", func(t *testing.T) {
		const sql = "DESCRIBE x"
		describe := parseOneDialect(t, sql, "postgres")
		if describe.Kind() != exp.KindDescribe {
			t.Fatalf("kind = %v, want Describe:\n%s", describe.Kind(), describe.ToS())
		}
		if kind := describe.Text("kind"); kind == "EXPLAIN" {
			t.Fatalf("kind arg = %q, want old unmarked Describe path:\n%s", kind, describe.ToS())
		}
		got, err := generateSQL(t, describe, "postgres")
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if got != sql {
			t.Fatalf("round-trip = %q, want %q", got, sql)
		}
	})
}
