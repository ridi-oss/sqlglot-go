package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestCopyStructured ports the postgres COPY identity cases from
// test_postgres.py:911-921 (testdata/parity_gaps.txt:246-247,283): _parse_copy
// (parser.py:9616-9649) / copy_sql (generator.py:5390-5416).
func TestCopyStructured(t *testing.T) {
	cases := []string{
		"COPY tbl (col1, col2) FROM 'file' WITH (FORMAT format, HEADER MATCH, FREEZE TRUE)",
		"COPY tbl (col1, col2) TO 'file' WITH (FORMAT format, HEADER MATCH, FREEZE TRUE)",
		"COPY (SELECT * FROM t) TO 'file' WITH (FORMAT format, HEADER MATCH, FREEZE TRUE)",
	}

	for _, sql := range cases {
		t.Run(sql, func(t *testing.T) {
			root := parseOneDialect(t, sql, "postgres")
			if root.Kind() != exp.KindCopy {
				t.Fatalf("kind = %v, want Copy:\n%s", root.Kind(), root.ToS())
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

// TestCopyFromToKind exercises the kind := match(FROM) || !matchTextSeq("TO") logic
// (parser.py:9631) directly on the parsed Copy node's "kind" arg.
func TestCopyFromToKind(t *testing.T) {
	from := parseOneDialect(t, "COPY tbl FROM 'file'", "postgres")
	if from.Kind() != exp.KindCopy {
		t.Fatalf("kind = %v, want Copy:\n%s", from.Kind(), from.ToS())
	}
	if kind, _ := from.Arg("kind").(bool); !kind {
		t.Fatalf("kind arg = %#v, want true (FROM):\n%s", from.Arg("kind"), from.ToS())
	}

	to := parseOneDialect(t, "COPY tbl TO 'file'", "postgres")
	if to.Kind() != exp.KindCopy {
		t.Fatalf("kind = %v, want Copy:\n%s", to.Kind(), to.ToS())
	}
	if kind, _ := to.Arg("kind").(bool); kind {
		t.Fatalf("kind arg = %#v, want false (TO):\n%s", to.Arg("kind"), to.ToS())
	}
}

// TestCopyUnwrappedParams covers _parse_wrapped(_parse_copy_parameters, optional=True)
// (parser.py:9646): _parse_wrapped always invokes the parse method, so an UNWRAPPED (no
// parens) parameter list parses too. Verified against sqlglot==30.12.0: each input parses to
// a Copy and canonicalizes to the wrapped `WITH (...)` form. The last case (a bare trailing
// token with no WITH) still becomes a Copy with a single unwrapped CopyParameter — it does
// NOT degrade to Command (that only happens when tokens remain after the param list).
func TestCopyUnwrappedParams(t *testing.T) {
	cases := []struct{ in, want string }{
		{"COPY tbl FROM 'file' WITH FORMAT csv", "COPY tbl FROM 'file' WITH (FORMAT csv)"},
		{"COPY tbl FROM 'file' FORMAT csv", "COPY tbl FROM 'file' WITH (FORMAT csv)"},
		{"COPY tbl FROM 'file' WITH CSV HEADER", "COPY tbl FROM 'file' WITH (CSV HEADER)"},
		{"COPY tbl FROM 'file' garbage_trailer", "COPY tbl FROM 'file' WITH (garbage_trailer)"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			root := parseOneDialect(t, c.in, "postgres")
			if root.Kind() != exp.KindCopy {
				t.Fatalf("kind = %v, want Copy:\n%s", root.Kind(), root.ToS())
			}
			got, err := generateSQL(t, root, "postgres")
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != c.want {
				t.Fatalf("round-trip = %q, want %q", got, c.want)
			}
		})
	}
}

// TestCopyDegradesToCommand exercises the trailing-token fallback in parseCopy
// (parser.py:9644-9645/parser_ddl.go:240 parseAsCommand): a shape this slice's structured
// grammar can't fully consume (here, an unbalanced trailing ")") leaves tokens after the
// parameter list, so the whole statement round-trips via the raw Command escape hatch —
// matching sqlglot==30.12.0, which also falls back to Command for this input.
func TestCopyDegradesToCommand(t *testing.T) {
	sql := "COPY tbl FROM 'file' WITH (FORMAT csv))"
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
}

// TestCopyFormatAs covers the `FORMAT AS AVRO|JSON` copy-parameter branch (parser.py:
// 9573-9579). Unlike the COPY_INTO_VARLEN/FILE_FORMAT branches above it, this one is NOT
// dialect-gated, so postgres reaches it. Without the branch the generic path drops the AS
// and rewrites `FORMAT AS JSON 'x'` into the wrong `FORMAT JSON, x`. Each case is an identity
// round-trip in sqlglot 30.12.0.
func TestCopyFormatAs(t *testing.T) {
	for _, sql := range []string{
		"COPY tbl FROM 'file' WITH (FORMAT AS JSON)",
		"COPY tbl FROM 'file' WITH (FORMAT AS JSON 'x')",
		"COPY tbl FROM 'file' WITH (FORMAT AS AVRO)",
	} {
		t.Run(sql, func(t *testing.T) {
			root := parseOneDialect(t, sql, "postgres")
			if root.Kind() != exp.KindCopy {
				t.Fatalf("kind = %v, want Copy:\n%s", root.Kind(), root.ToS())
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

// TestCopyCredentialsDegradesToCommand covers the single-option credentials clause
// (`CREDENTIALS = (A='b')`). These Snowflake/Redshift-only clauses render faithfully only via
// upstream's unported _parse_property grammar, so parseCopy degrades any COPY carrying one to
// a raw Command (exact round-trip) rather than emit a lossy structural rewrite. This makes the
// single-option case consistent with the multi-option case in TestCopyCredentialsTerminates.
func TestCopyCredentialsDegradesToCommand(t *testing.T) {
	sql := "COPY tbl FROM 'file' CREDENTIALS = (A='b')"
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
}

// TestCopyCredentialsTerminates is a regression guard for the parseCopyWrappedOptions /
// parseCopyParameters no-progress guards: a comma-separated CREDENTIALS list is a shape this
// port doesn't fully model (no _parse_property grammar), and before the fix its loop spun
// forever on the comma token. It must now terminate — degrading to Command and round-tripping
// the raw input is the acceptable fallback (upstream models this via SequenceProperties, which
// isn't ported). The go test -timeout would fire if this ever regresses to an infinite loop.
func TestCopyCredentialsTerminates(t *testing.T) {
	sql := "COPY tbl FROM 'file' CREDENTIALS = (A='b', C='d')"
	root := parseOneDialect(t, sql, "postgres")
	got, err := generateSQL(t, root, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != sql {
		t.Fatalf("round-trip = %q, want %q", got, sql)
	}
}
