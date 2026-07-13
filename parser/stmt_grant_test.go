package parser_test

import (
	"strings"
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// TestGrantRevokeStructured ports parser.py:9690-9784 (_parse_grant/_parse_revoke and
// their helpers) plus the structured identity cases from test_postgres.py:1777 test_grant
// / :1813 test_revoke. The GRANT/REVOKE grammar ported here isn't dialect-gated, so the
// same cases (plus a handful of base-dialect ones) round-trip for the base dialect too.
func TestGrantRevokeStructured(t *testing.T) {
	cases := []struct {
		dialect string
		sql     string
	}{
		{"", "GRANT DELETE ON SCHEMA finance TO bob"},
		{"", "GRANT SELECT ON TABLE tbl TO user"},
		{"", "GRANT SELECT ON nation TO alice WITH GRANT OPTION"},
		// Exercises _parse_grant_principal's ROLE/GROUP kind (parser.py:9709).
		{"", "GRANT SELECT ON orders TO ROLE PUBLIC"},
		{"", "GRANT SELECT, INSERT ON FUNCTION tbl TO user"},
		{"", "REVOKE DELETE ON SCHEMA finance FROM bob CASCADE"},
		{"", "REVOKE GRANT OPTION FOR SELECT ON nation FROM alice"},
		{"", "REVOKE INSERT ON TABLE orders FROM user RESTRICT"},
		{"", "REVOKE SELECT ON TABLE tbl FROM user"},
		{"", "REVOKE SELECT ON orders FROM ROLE PUBLIC"},
		{"", "REVOKE SELECT, INSERT ON FUNCTION tbl FROM user"},

		{"postgres", "GRANT SELECT ON TABLE users TO role1"},
		{"postgres", "GRANT INSERT, DELETE ON TABLE orders TO user1"},
		{"postgres", "GRANT SELECT ON employees TO manager WITH GRANT OPTION"},
		{"postgres", "GRANT USAGE ON SCHEMA finance TO user2"},
		{"postgres", "GRANT ALL PRIVILEGES ON DATABASE mydb TO PUBLIC"},
		{"postgres", "GRANT CREATE ON SCHEMA public TO developer"},
		{"postgres", "GRANT CONNECT ON DATABASE testdb TO readonly_user"},
		{"postgres", "GRANT TEMPORARY ON DATABASE testdb TO temp_user"},
		{"postgres", "GRANT TRIGGER ON orders TO audit_role"},
		{"postgres", "GRANT REFERENCES ON products TO foreign_key_user"},
		{"postgres", "GRANT TRUNCATE ON logs TO admin_role"},
		{"postgres", "GRANT UPDATE(salary) ON employees TO hr_manager"},
		{"postgres", "GRANT SELECT(id, name), UPDATE(email) ON customers TO customer_service"},

		{"postgres", "REVOKE SELECT ON TABLE users FROM role1"},
		{"postgres", "REVOKE INSERT, DELETE ON TABLE orders FROM user1"},
		{"postgres", "REVOKE USAGE ON SCHEMA finance FROM user2"},
		{"postgres", "REVOKE ALL PRIVILEGES ON DATABASE mydb FROM PUBLIC"},
		{"postgres", "REVOKE CREATE ON SCHEMA public FROM developer"},
		{"postgres", "REVOKE CONNECT ON DATABASE testdb FROM readonly_user"},
		{"postgres", "REVOKE TEMPORARY ON DATABASE testdb FROM temp_user"},
		{"postgres", "REVOKE TRIGGER ON orders FROM audit_role"},
		{"postgres", "REVOKE REFERENCES ON products FROM foreign_key_user"},
		{"postgres", "REVOKE TRUNCATE ON logs FROM admin_role"},
		{"postgres", "REVOKE USAGE ON SCHEMA finance FROM user2 CASCADE"},
		{"postgres", "REVOKE SELECT ON TABLE orders FROM user1 RESTRICT"},
		{"postgres", "REVOKE GRANT OPTION FOR SELECT ON employees FROM manager"},
		{"postgres", "REVOKE GRANT OPTION FOR SELECT ON employees FROM manager RESTRICT"},
		{"postgres", "REVOKE UPDATE(salary) ON employees FROM hr_manager"},
		{"postgres", "REVOKE SELECT(id, name), UPDATE(email) ON customers FROM customer_service"},
	}

	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			root := parseOneDialect(t, tc.sql, tc.dialect)
			wantKind := exp.KindGrant
			if strings.HasPrefix(tc.sql, "REVOKE") {
				wantKind = exp.KindRevoke
			}
			if root.Kind() != wantKind {
				t.Fatalf("kind = %v, want %v:\n%s", root.Kind(), wantKind, root.ToS())
			}
			got, err := generateSQL(t, root, tc.dialect)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != tc.sql {
				t.Fatalf("round-trip = %q, want %q", got, tc.sql)
			}
		})
	}
}

// TestGrantRevokeFunctionSecurableNormalizesName ports test_postgres.py:1798-1801/1837-1840:
// the securable is parsed via parseTableParts' function-call branch (parser.py:4664-4670),
// so an unknown-function name is uppercased by the generic Anonymous-function fallback.
func TestGrantRevokeFunctionSecurableNormalizesName(t *testing.T) {
	root := parseOneDialect(t, "GRANT EXECUTE ON FUNCTION calculate_bonus(integer) TO analyst", "postgres")
	if root.Kind() != exp.KindGrant {
		t.Fatalf("kind = %v, want Grant:\n%s", root.Kind(), root.ToS())
	}
	got, err := generateSQL(t, root, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "GRANT EXECUTE ON FUNCTION CALCULATE_BONUS(integer) TO analyst"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}

	revoke := parseOneDialect(t, "REVOKE EXECUTE ON FUNCTION calculate_bonus(integer) FROM analyst", "postgres")
	if revoke.Kind() != exp.KindRevoke {
		t.Fatalf("kind = %v, want Revoke:\n%s", revoke.Kind(), revoke.ToS())
	}
	got, err = generateSQL(t, revoke, "postgres")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if want := "REVOKE EXECUTE ON FUNCTION CALCULATE_BONUS(integer) FROM analyst"; got != want {
		t.Fatalf("round-trip = %q, want %q", got, want)
	}
}

// TestGrantRevokeDegradesToCommand ports the Command-fallback cases from
// test_postgres.py:1803-1811/1842-1850 (role grants + ALL TABLES IN SCHEMA, which aren't
// a parseable securable/principal shape) and test_mysql.py:1569-1597 test_grant/test_revoke
// (MySQL's role lists, user@host principals, and db.*/*.* securables aren't parseable
// yet either), dropping the check_command_warning log assertion which this port doesn't
// implement.
func TestGrantRevokeDegradesToCommand(t *testing.T) {
	cases := []struct {
		dialect string
		sql     string
	}{
		{"postgres", "GRANT INSERT, DELETE ON ALL TABLES IN SCHEMA myschema TO user1"},
		{"postgres", "GRANT developer_role TO john"},
		{"postgres", "GRANT admin_role TO mary WITH ADMIN OPTION"},
		{"postgres", "REVOKE INSERT, DELETE ON ALL TABLES IN SCHEMA myschema FROM user1"},
		{"postgres", "REVOKE developer_role FROM john"},
		{"postgres", "REVOKE admin_role FROM mary"},

		{"mysql", "GRANT 'role1', 'role2' TO 'user1'@'localhost', 'user2'@'localhost'"},
		{"mysql", "GRANT SELECT ON world.* TO 'role3'"},
		{"mysql", "GRANT SELECT ON db2.invoice TO 'jeffrey'@'localhost'"},
		{"mysql", "GRANT INSERT ON `d%`.* TO u"},
		{"mysql", "GRANT ALL ON test.* TO ''@'localhost'"},
		{"mysql", "GRANT SELECT (col1), INSERT (col1, col2) ON mydb.mytbl TO 'someuser'@'somehost'"},
		{"mysql", "GRANT SELECT, INSERT, UPDATE ON *.* TO u2"},
		{"mysql", "REVOKE 'role1', 'role2' FROM 'user1'@'localhost', 'user2'@'localhost'"},
		{"mysql", "REVOKE SELECT ON world.* FROM 'role3'"},
		{"mysql", "REVOKE SELECT ON db2.invoice FROM 'jeffrey'@'localhost'"},
		{"mysql", "REVOKE INSERT ON `d%`.* FROM u"},
		{"mysql", "REVOKE ALL ON test.* FROM ''@'localhost'"},
		{"mysql", "REVOKE SELECT (col1), INSERT (col1, col2) ON mydb.mytbl FROM 'someuser'@'somehost'"},
		{"mysql", "REVOKE SELECT, INSERT, UPDATE ON *.* FROM u2"},
	}

	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			root := parseOneDialect(t, tc.sql, tc.dialect)
			if root.Kind() != exp.KindCommand {
				t.Fatalf("kind = %v, want Command (degrade):\n%s", root.Kind(), root.ToS())
			}
			got, err := generateSQL(t, root, tc.dialect)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != tc.sql {
				t.Fatalf("round-trip = %q, want %q", got, tc.sql)
			}
		})
	}
}
