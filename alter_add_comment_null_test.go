package sqlglot_test

import (
	"testing"

	sqlglot "github.com/ridi-oss/sqlglot-go"
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/generator"
)

// Repeated `ADD CONSTRAINT` actions and `COMMENT … IS NULL` parse structurally and round-trip. See
// DEVIATIONS "Grammar extensions beyond upstream".
func TestAlterAddConstraintRepeatedAndCommentIsNull(t *testing.T) {
	cases := []struct {
		dialect string
		sql     string
		kind    exp.Kind
	}{
		{"postgres", "ALTER TABLE t ADD CONSTRAINT a CHECK (x >= 1), ADD CONSTRAINT b CHECK (x >= 0)", exp.KindAlter},
		{"postgres", "ALTER TABLE t ADD CONSTRAINT a FOREIGN KEY (x) REFERENCES u (id), ADD CONSTRAINT b UNIQUE (y)", exp.KindAlter},
		{"postgres", "ALTER TABLE t ADD CONSTRAINT a CHECK (x > 1), ADD COLUMN c INT", exp.KindAlter},
		{"postgres", "ALTER TABLE t ADD CONSTRAINT a CHECK (x > 1), CONSTRAINT b CHECK (x > 2)", exp.KindAlter},
		{"mysql", "ALTER TABLE t ADD CONSTRAINT a CHECK (x >= 1), ADD CONSTRAINT b CHECK (x >= 0)", exp.KindAlter},
		{"postgres", "COMMENT ON TABLE t IS NULL", exp.KindComment},
		{"postgres", "COMMENT ON COLUMN t.c IS NULL", exp.KindComment},
	}
	for _, c := range cases {
		t.Run(c.dialect+"/"+c.sql, func(t *testing.T) {
			e, err := sqlglot.ParseOne(c.sql, c.dialect)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if e.Kind() != c.kind {
				t.Fatalf("kind = %v, want %v:\n%s", exp.ClassName(e.Kind()), exp.ClassName(c.kind), e.ToS())
			}
			out, err := sqlglot.Generate(e, c.dialect, generator.Options{})
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			if out != c.sql {
				t.Fatalf("round-trip = %q, want %q", out, c.sql)
			}
		})
	}
	e, err := sqlglot.ParseOne("ALTER TABLE t ADD CONSTRAINT a CHECK (x >= 1), ADD CONSTRAINT b CHECK (x >= 0)", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	actions, _ := e.Arg("actions").([]exp.Expression)
	if len(actions) != 2 || actions[0].Kind() != exp.KindAddConstraint || actions[1].Kind() != exp.KindAddConstraint {
		t.Fatalf("want two AddConstraint actions:\n%s", e.ToS())
	}
	c, err := sqlglot.ParseOne("COMMENT ON TABLE t IS NULL", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := c.Arg("expression").(exp.Expression); body == nil || body.Kind() != exp.KindNull {
		t.Fatalf("want Null expression:\n%s", c.ToS())
	}
}
