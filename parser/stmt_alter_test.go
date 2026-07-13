package parser_test

import (
	"testing"

	exp "github.com/ridi/sqlglot-go/expressions"
)

// alterActions returns the parsed Alter's "actions" list (parser.py:8752-... family).
func alterActions(t *testing.T, alter exp.Expression) []exp.Expression {
	t.Helper()
	if alter.Kind() != exp.KindAlter {
		t.Fatalf("kind = %v, want Alter:\n%s", alter.Kind(), alter.ToS())
	}
	return expressionsForArg(alter, "actions")
}

// TestParseAlterAddColumn ports the ADD COLUMN family (parser.py:8716-8789), including the
// gaps at testdata/identity.sql:762-768.
func TestParseAlterAddColumn(t *testing.T) {
	alter := parseOne(t, "ALTER TABLE integers ADD COLUMN k INT")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindColumnDef {
		t.Fatalf("ADD COLUMN action mismatch:\n%s", alter.ToS())
	}
	if exprArg(t, actions[0], "kind").Kind() != exp.KindDataType {
		t.Fatalf("ADD COLUMN type mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE integers ADD COLUMN k INT FIRST")
	actions = alterActions(t, alter)
	pos := exprArg(t, actions[0], "position")
	if pos.Kind() != exp.KindColumnPosition || pos.Arg("position") != "FIRST" {
		t.Fatalf("FIRST position mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE integers ADD COLUMN k INT AFTER m")
	actions = alterActions(t, alter)
	pos = exprArg(t, actions[0], "position")
	if pos.Kind() != exp.KindColumnPosition || pos.Arg("position") != "AFTER" {
		t.Fatalf("AFTER position mismatch (this must be a Column, not just the position text):\n%s", alter.ToS())
	}
	if exprArg(t, pos, "this").Name() != "m" {
		t.Fatalf("AFTER target column mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE integers ADD COLUMN IF NOT EXISTS k INT")
	actions = alterActions(t, alter)
	if actions[0].Arg("exists") != true {
		t.Fatalf("IF NOT EXISTS mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE IF EXISTS integers ADD COLUMN k INT")
	if alter.Arg("exists") != true {
		t.Fatalf("ALTER TABLE IF EXISTS mismatch:\n%s", alter.ToS())
	}

	// BigQuery-style repeated ADD COLUMN (parser.py:8752-8789's csv-of-_parse_add_alteration
	// path, since ALTER_TABLE_ADD_REQUIRED_FOR_EACH_COLUMN=True for base/mysql/postgres):
	// each item re-matches its own ADD COLUMN.
	alter = parseOne(t, "ALTER TABLE mydataset.mytable ADD COLUMN A TEXT, ADD COLUMN IF NOT EXISTS B INT")
	actions = alterActions(t, alter)
	if len(actions) != 2 {
		t.Fatalf("repeated ADD COLUMN count mismatch: got %d, want 2:\n%s", len(actions), alter.ToS())
	}
	if actions[0].Kind() != exp.KindColumnDef || actions[0].This().Name() != "A" {
		t.Fatalf("first repeated ADD COLUMN mismatch:\n%s", alter.ToS())
	}
	if actions[1].Kind() != exp.KindColumnDef || actions[1].This().Name() != "B" || actions[1].Arg("exists") != true {
		t.Fatalf("second repeated ADD COLUMN mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterAddConstraint ports _parse_alter_table_add's ADD_CONSTRAINT_TOKENS branch
// (parser.py:8752-8789) through parseConstraint/parseUnnamedConstraint(s), covering the
// named-CONSTRAINT and bare PRIMARY KEY/FOREIGN KEY/UNIQUE forms from
// testdata/identity.sql:794-806.
func TestParseAlterAddConstraint(t *testing.T) {
	alter := parseOne(t, `ALTER TABLE "schema"."tablename" ADD CONSTRAINT "CHK_Name" CHECK (NOT "IdDwh" IS NULL)`)
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindAddConstraint {
		t.Fatalf("AddConstraint mismatch:\n%s", alter.ToS())
	}
	addConstraints := expressionsForArg(actions[0], "expressions")
	if len(addConstraints) != 1 || addConstraints[0].Kind() != exp.KindConstraint {
		t.Fatalf("named constraint mismatch:\n%s", alter.ToS())
	}
	if addConstraints[0].This().Name() != "CHK_Name" {
		t.Fatalf("constraint name mismatch:\n%s", alter.ToS())
	}
	namedBody := expressionsForArg(addConstraints[0], "expressions")
	if len(namedBody) != 1 || namedBody[0].Kind() != exp.KindCheckColumnConstraint {
		t.Fatalf("constraint body mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE persons ADD CONSTRAINT persons_pk PRIMARY KEY (first_name, last_name)")
	actions = alterActions(t, alter)
	pkBody := expressionsForArg(expressionsForArg(actions[0], "expressions")[0], "expressions")
	if pkBody[0].Kind() != exp.KindPrimaryKey || len(pkBody[0].Expressions()) != 2 {
		t.Fatalf("named PRIMARY KEY mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE pets ADD CONSTRAINT pets_persons_fk FOREIGN KEY (owner_first_name, owner_last_name) REFERENCES persons")
	actions = alterActions(t, alter)
	fkBody := expressionsForArg(expressionsForArg(actions[0], "expressions")[0], "expressions")
	if fkBody[0].Kind() != exp.KindForeignKey {
		t.Fatalf("named FOREIGN KEY mismatch:\n%s", alter.ToS())
	}
	if exprArg(t, fkBody[0], "reference").Kind() != exp.KindReference {
		t.Fatalf("FOREIGN KEY reference mismatch:\n%s", alter.ToS())
	}

	// Bare (unnamed) PRIMARY KEY/FOREIGN KEY via ADD_CONSTRAINT_TOKENS, straight to
	// exp.AddConstraint (no exp.Constraint wrapper).
	alter = parseOne(t, "ALTER TABLE a ADD PRIMARY KEY (x, y) NOT ENFORCED")
	actions = alterActions(t, alter)
	unnamed := expressionsForArg(actions[0], "expressions")
	if unnamed[0].Kind() != exp.KindPrimaryKey {
		t.Fatalf("unnamed PRIMARY KEY mismatch:\n%s", alter.ToS())
	}
	opts, _ := unnamed[0].Arg("options").([]string)
	if len(opts) != 1 || opts[0] != "NOT ENFORCED" {
		t.Fatalf("PRIMARY KEY options mismatch: %v\n%s", opts, alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE a ADD FOREIGN KEY (x, y) REFERENCES bla")
	actions = alterActions(t, alter)
	unnamed = expressionsForArg(actions[0], "expressions")
	if unnamed[0].Kind() != exp.KindForeignKey || len(unnamed[0].Expressions()) != 2 {
		t.Fatalf("unnamed FOREIGN KEY mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE s_ut ADD CONSTRAINT s_ut_uq UNIQUE hajo")
	actions = alterActions(t, alter)
	uqBody := expressionsForArg(expressionsForArg(actions[0], "expressions")[0], "expressions")
	if uqBody[0].Kind() != exp.KindUniqueColumnConstraint {
		t.Fatalf("named UNIQUE mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterDropColumn ports _parse_alter_table_drop's per-item DROP-retreat trick
// (parser.py:8848-8856) and test_drop_column (test_parser.py:984): find_all(exp.Table) == 1
// and find_all(exp.Column) == 1 for a single DROP COLUMN, i.e. the dropped column parses as
// a bare exp.Column, not wrapped in a Table.
func TestParseAlterDropColumn(t *testing.T) {
	alter := parseOne(t, "ALTER TABLE tbl DROP COLUMN col")
	if len(alter.FindAll(exp.KindTable)) != 1 {
		t.Fatalf("want exactly 1 Table:\n%s", alter.ToS())
	}
	if len(alter.FindAll(exp.KindColumn)) != 1 {
		t.Fatalf("want exactly 1 Column:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE integers DROP COLUMN k")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindDrop || actions[0].Arg("kind") != "COLUMN" {
		t.Fatalf("DROP COLUMN mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE integers DROP COLUMN IF EXISTS k")
	actions = alterActions(t, alter)
	if actions[0].Arg("exists") != true {
		t.Fatalf("DROP COLUMN IF EXISTS mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE integers DROP COLUMN k CASCADE")
	actions = alterActions(t, alter)
	if actions[0].Arg("cascade") != true {
		t.Fatalf("DROP COLUMN CASCADE mismatch:\n%s", alter.ToS())
	}

	// Each comma-separated DROP re-matches its own DROP keyword.
	alter = parseOne(t, "ALTER TABLE mydataset.mytable DROP COLUMN A, DROP COLUMN IF EXISTS B")
	actions = alterActions(t, alter)
	if len(actions) != 2 {
		t.Fatalf("repeated DROP COLUMN count mismatch: got %d, want 2:\n%s", len(actions), alter.ToS())
	}
	if actions[0].This().Name() != "A" || actions[1].This().Name() != "B" || actions[1].Arg("exists") != true {
		t.Fatalf("repeated DROP COLUMN mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterDropPartition ports _parse_alter_table_drop's PARTITION branch and
// _parse_drop_partition (parser.py:8747-8750,8848-8856).
func TestParseAlterDropPartition(t *testing.T) {
	alter := parseOne(t, "ALTER TABLE orders DROP PARTITION(dt = '2014-05-14', country = 'IN')")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindDropPartition {
		t.Fatalf("DROP PARTITION mismatch:\n%s", alter.ToS())
	}
	parts := expressionsForArg(actions[0], "expressions")
	if len(parts) != 1 || parts[0].Kind() != exp.KindPartition {
		t.Fatalf("DROP PARTITION body mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE orders DROP IF EXISTS PARTITION(dt = '2014-05-14', country = 'IN')")
	actions = alterActions(t, alter)
	if actions[0].Arg("exists") != true {
		t.Fatalf("DROP IF EXISTS PARTITION mismatch:\n%s", alter.ToS())
	}

	// A single DropPartition action absorbs every comma-separated PARTITION(...) (the inner
	// parseCsv(parsePartition) call consumes the whole list before the outer parseCsv can
	// see another comma).
	alter = parseOne(t, "ALTER TABLE orders DROP PARTITION(dt = '2014-05-14', country = 'IN'), PARTITION(dt = '2014-05-15', country = 'IN')")
	actions = alterActions(t, alter)
	if len(actions) != 1 {
		t.Fatalf("multi-PARTITION action count mismatch: got %d, want 1:\n%s", len(actions), alter.ToS())
	}
	parts = expressionsForArg(actions[0], "expressions")
	if len(parts) != 2 {
		t.Fatalf("multi-PARTITION expressions count mismatch: got %d, want 2:\n%s", len(parts), alter.ToS())
	}
}

// TestParseAlterColumn ports _parse_alter_table_alter (parser.py:8791-8825).
func TestParseAlterColumn(t *testing.T) {
	cases := []struct {
		sql     string
		key     string
		wantOK  func(action exp.Expression) bool
		wantMsg string
	}{
		{
			"ALTER TABLE integers ALTER COLUMN i SET DATA TYPE VARCHAR", "dtype",
			func(a exp.Expression) bool { return exprArg(t, a, "dtype").Kind() == exp.KindDataType },
			"SET DATA TYPE",
		},
		{
			"ALTER TABLE integers ALTER COLUMN i SET DEFAULT 10", "default",
			func(a exp.Expression) bool { return a.Arg("default") != nil },
			"SET DEFAULT",
		},
		{
			"ALTER TABLE integers ALTER COLUMN i DROP DEFAULT", "drop",
			func(a exp.Expression) bool { return a.Arg("drop") == true },
			"DROP DEFAULT",
		},
		{
			"ALTER TABLE ingredients ALTER COLUMN amount COMMENT 'tablespoons'", "comment",
			func(a exp.Expression) bool { return exprArg(t, a, "comment").Name() == "tablespoons" },
			"COMMENT",
		},
	}
	for _, tc := range cases {
		t.Run(tc.wantMsg, func(t *testing.T) {
			alter := parseOne(t, tc.sql)
			actions := alterActions(t, alter)
			if len(actions) != 1 || actions[0].Kind() != exp.KindAlterColumn {
				t.Fatalf("%s: kind mismatch:\n%s", tc.wantMsg, alter.ToS())
			}
			if !tc.wantOK(actions[0]) {
				t.Fatalf("%s: arg mismatch:\n%s", tc.wantMsg, alter.ToS())
			}
		})
	}

	// USING clause on SET DATA TYPE (dtype parsed before the trailing USING check). CONCAT is a
	// registered function (parser.parseConcat), so the USING expression is a Concat node.
	alter := parseOne(t, "ALTER TABLE integers ALTER COLUMN i SET DATA TYPE VARCHAR USING CONCAT(i, '_', j)")
	actions := alterActions(t, alter)
	if exprArg(t, actions[0], "using").Kind() != exp.KindConcat {
		t.Fatalf("SET DATA TYPE USING mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterRename ports _parse_alter_table_rename (parser.py:8858-8873).
func TestParseAlterRename(t *testing.T) {
	alter := parseOne(t, "ALTER TABLE table1 RENAME COLUMN c1 TO c2")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindRenameColumn {
		t.Fatalf("RENAME COLUMN mismatch:\n%s", alter.ToS())
	}
	if exprArg(t, actions[0], "this").Name() != "c1" || exprArg(t, actions[0], "to").Name() != "c2" {
		t.Fatalf("RENAME COLUMN names mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE table1 RENAME COLUMN IF EXISTS c1 TO c2")
	actions = alterActions(t, alter)
	if actions[0].Arg("exists") != true {
		t.Fatalf("RENAME COLUMN IF EXISTS mismatch:\n%s", alter.ToS())
	}

	alter = parseOne(t, "ALTER TABLE table1 RENAME TO table2")
	actions = alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindAlterRename {
		t.Fatalf("RENAME TO mismatch:\n%s", alter.ToS())
	}
	if exprArg(t, actions[0], "this").Name() != "table2" {
		t.Fatalf("RENAME TO target mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterViewAsSelect and TestParseAlterTableDelete ports ALTER_PARSERS["AS"]/
// ["DELETE"] (parser.py:1439,1442).
func TestParseAlterViewAsSelect(t *testing.T) {
	alter := parseOne(t, "ALTER VIEW view1 AS SELECT a, b, c FROM table1 UNION ALL SELECT a, b, c FROM table2")
	if alter.Kind() != exp.KindAlter || alter.Arg("kind") != "VIEW" {
		t.Fatalf("ALTER VIEW kind mismatch:\n%s", alter.ToS())
	}
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindUnion {
		t.Fatalf("ALTER VIEW AS SELECT UNION mismatch:\n%s", alter.ToS())
	}
}

func TestParseAlterTableDelete(t *testing.T) {
	alter := parseOne(t, "ALTER TABLE mydataset.mytable DELETE WHERE x = 1")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindDelete {
		t.Fatalf("ALTER ... DELETE mismatch:\n%s", alter.ToS())
	}
	if exprArg(t, actions[0], "where").Kind() != exp.KindWhere {
		t.Fatalf("ALTER ... DELETE WHERE mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterTableSetPostgres ports _parse_alter_table_set's postgres-reachable branches
// (parser.py:8875-8909), from testdata/parity_gaps.txt's postgres ALTER TABLE ... SET cases.
func TestParseAlterTableSetPostgres(t *testing.T) {
	cases := []struct {
		sql string
		key string
	}{
		{"ALTER TABLE t1 SET (fillfactor = 5, autovacuum_enabled = TRUE)", "expressions"},
		{"ALTER TABLE t1 SET ACCESS METHOD method", "access_method"},
		{"ALTER TABLE t1 SET LOGGED", "option"},
		{"ALTER TABLE t1 SET TABLESPACE tablespace", "tablespace"},
		{"ALTER TABLE t1 SET UNLOGGED", "option"},
		{"ALTER TABLE t1 SET WITHOUT CLUSTER", "option"},
		{"ALTER TABLE t1 SET WITHOUT OIDS", "option"},
	}
	for _, tc := range cases {
		t.Run(tc.sql, func(t *testing.T) {
			alter := parseOneDialect(t, tc.sql, "postgres")
			actions := alterActions(t, alter)
			if len(actions) != 1 || actions[0].Kind() != exp.KindAlterSet {
				t.Fatalf("kind mismatch:\n%s", alter.ToS())
			}
			if actions[0].Arg(tc.key) == nil {
				t.Fatalf("%s arg missing:\n%s", tc.key, alter.ToS())
			}
		})
	}

	alter := parseOneDialect(t, "ALTER TABLE t1 SET WITHOUT CLUSTER", "postgres")
	opt := exprArg(t, alterActions(t, alter)[0], "option")
	if opt.Kind() != exp.KindVar || opt.Name() != "WITHOUT CLUSTER" {
		t.Fatalf("WITHOUT CLUSTER option mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterMySQLIndexAttributes ports the mysql ADD INDEX/KEY/UNIQUE +
// ALTER INDEX VISIBLE/INVISIBLE + ADD COLUMN ... INVISIBLE gaps (parsers/mysql.py:
// 243-251,371-416,561-571).
func TestParseAlterMySQLIndexAttributes(t *testing.T) {
	alter := parseOneDialect(t, "ALTER TABLE t ADD INDEX `i` (`c`)", "mysql")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindAddConstraint {
		t.Fatalf("ADD INDEX mismatch:\n%s", alter.ToS())
	}
	idx := expressionsForArg(actions[0], "expressions")[0]
	if idx.Kind() != exp.KindIndexColumnConstraint || exprArg(t, idx, "this").Name() != "i" {
		t.Fatalf("ADD INDEX body mismatch:\n%s", alter.ToS())
	}

	alter = parseOneDialect(t, "ALTER TABLE t ADD UNIQUE `i` (`c`)", "mysql")
	actions = alterActions(t, alter)
	idx = expressionsForArg(actions[0], "expressions")[0]
	if idx.Kind() != exp.KindUniqueColumnConstraint {
		t.Fatalf("ADD UNIQUE mismatch:\n%s", alter.ToS())
	}

	alter = parseOneDialect(t, "ALTER TABLE t ALTER INDEX i INVISIBLE", "mysql")
	actions = alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindAlterIndex || actions[0].Arg("visible") != false {
		t.Fatalf("ALTER INDEX INVISIBLE mismatch:\n%s", alter.ToS())
	}

	alter = parseOneDialect(t, "ALTER TABLE t ALTER INDEX i VISIBLE", "mysql")
	actions = alterActions(t, alter)
	if actions[0].Arg("visible") != true {
		t.Fatalf("ALTER INDEX VISIBLE mismatch:\n%s", alter.ToS())
	}

	alter = parseOneDialect(t, "ALTER TABLE t ADD COLUMN c INT INVISIBLE", "mysql")
	actions = alterActions(t, alter)
	if actions[0].Kind() != exp.KindColumnDef {
		t.Fatalf("ADD COLUMN INVISIBLE mismatch:\n%s", alter.ToS())
	}
	invisConstraints := expressionsForArg(actions[0], "constraints")
	if len(invisConstraints) != 1 || exprArg(t, invisConstraints[0], "kind").Kind() != exp.KindInvisibleColumnConstraint {
		t.Fatalf("ADD COLUMN INVISIBLE constraint mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterMySQLChangeModify ports parsers/mysql.py:253-258,319-339
// (CHANGE/MODIFY COLUMN -> exp.ModifyColumn).
func TestParseAlterMySQLChangeModify(t *testing.T) {
	alter := parseOneDialect(t, "ALTER TABLE t CHANGE COLUMN a b BIGINT NOT NULL", "mysql")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindModifyColumn {
		t.Fatalf("CHANGE COLUMN mismatch:\n%s", alter.ToS())
	}
	if exprArg(t, actions[0], "rename_from").Name() != "a" {
		t.Fatalf("CHANGE COLUMN rename_from mismatch:\n%s", alter.ToS())
	}
	newDef := exprArg(t, actions[0], "this")
	if newDef.Kind() != exp.KindColumnDef || newDef.This().Name() != "b" {
		t.Fatalf("CHANGE COLUMN new def mismatch:\n%s", alter.ToS())
	}

	alter = parseOneDialect(t, "ALTER TABLE t MODIFY COLUMN c INT NOT NULL", "mysql")
	actions = alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindModifyColumn {
		t.Fatalf("MODIFY COLUMN mismatch:\n%s", alter.ToS())
	}
	if actions[0].Arg("rename_from") != nil {
		t.Fatalf("MODIFY COLUMN should have no rename_from:\n%s", alter.ToS())
	}

	alter = parseOneDialect(t, "ALTER TABLE t CHANGE COLUMN c d INT AFTER e", "mysql")
	actions = alterActions(t, alter)
	newDef = exprArg(t, actions[0], "this")
	if exprArg(t, newDef, "position").Arg("position") != "AFTER" {
		t.Fatalf("CHANGE COLUMN ... AFTER mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterMySQLRenameIndex ports parsers/mysql.py:306-312 (RENAME INDEX/KEY ->
// exp.RenameIndex).
func TestParseAlterMySQLRenameIndex(t *testing.T) {
	alter := parseOneDialect(t, "ALTER TABLE t RENAME INDEX a TO b", "mysql")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindRenameIndex {
		t.Fatalf("RENAME INDEX mismatch:\n%s", alter.ToS())
	}
	if exprArg(t, actions[0], "this").Name() != "a" || exprArg(t, actions[0], "to").Name() != "b" {
		t.Fatalf("RENAME INDEX names mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterMySQLDropPrimaryKey ports parsers/mysql.py:314-317 (DROP PRIMARY KEY ->
// exp.DropPrimaryKey) and its interleaving with regular DROP COLUMN/INDEX actions.
func TestParseAlterMySQLDropPrimaryKey(t *testing.T) {
	alter := parseOneDialect(t, "ALTER TABLE t DROP PRIMARY KEY", "mysql")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindDropPrimaryKey {
		t.Fatalf("DROP PRIMARY KEY mismatch:\n%s", alter.ToS())
	}

	alter = parseOneDialect(t, "ALTER TABLE t DROP COLUMN c, DROP PRIMARY KEY, DROP INDEX `i`", "mysql")
	actions = alterActions(t, alter)
	if len(actions) != 3 {
		t.Fatalf("mixed DROP actions count mismatch: got %d, want 3:\n%s", len(actions), alter.ToS())
	}
	if actions[0].Kind() != exp.KindDrop || actions[0].Arg("kind") != "COLUMN" {
		t.Fatalf("mixed DROP[0] mismatch:\n%s", alter.ToS())
	}
	if actions[1].Kind() != exp.KindDropPrimaryKey {
		t.Fatalf("mixed DROP[1] mismatch:\n%s", alter.ToS())
	}
	if actions[2].Kind() != exp.KindDrop || actions[2].Arg("kind") != "INDEX" {
		t.Fatalf("mixed DROP[2] mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterOnlySchemaCheckCascade ports the ONLY/WITH CHECK/cluster/cascade args on
// exp.Alter itself (parser.py:8934-8969), plus the ALTERABLES gate (INDEX/TABLE/VIEW/
// SESSION, parser.py:657-662).
func TestParseAlterOnlySchemaCheckCascade(t *testing.T) {
	alter := parseOneDialect(t, `ALTER TABLE ONLY "Album" ADD CONSTRAINT "FK_AlbumArtistId" FOREIGN KEY ("ArtistId") REFERENCES "Artist" ("ArtistId") ON DELETE CASCADE`, "postgres")
	if alter.Arg("only") != true {
		t.Fatalf("ONLY mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterMySQLProperties ports MySQL's AUTO_INCREMENT action and trailing
// ALGORITHM/LOCK options.
func TestParseAlterMySQLProperties(t *testing.T) {
	alter := parseOneDialect(t, "ALTER TABLE t1 ADD COLUMN x INT, ALGORITHM=INPLACE, LOCK=EXCLUSIVE", "mysql")
	actions := alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindColumnDef {
		t.Fatalf("ALTER actions mismatch:\n%s", alter.ToS())
	}
	options := expressionsForArg(alter, "options")
	if len(options) != 2 || options[0].Kind() != exp.KindAlgorithmProperty || options[1].Kind() != exp.KindLockProperty {
		t.Fatalf("ALTER options mismatch:\n%s", alter.ToS())
	}
	if options[0].Name() != "INPLACE" || options[1].Name() != "EXCLUSIVE" {
		t.Fatalf("ALTER option values mismatch:\n%s", alter.ToS())
	}

	alter = parseOneDialect(t, "ALTER TABLE t AUTO_INCREMENT=3000000000", "mysql")
	actions = alterActions(t, alter)
	if len(actions) != 1 || actions[0].Kind() != exp.KindAutoIncrementProperty || actions[0].Name() != "3000000000" {
		t.Fatalf("AUTO_INCREMENT action mismatch:\n%s", alter.ToS())
	}
}

// TestParseAlterUnsupportedSequenceStaysCommand keeps the genuinely unsupported ALTER target
// fail-closed.
func TestParseAlterUnsupportedSequenceStaysCommand(t *testing.T) {
	root := parseOne(t, "ALTER SEQUENCE foo RESTART")
	if root.Kind() != exp.KindCommand {
		t.Fatalf("kind = %v, want Command (degrade):\n%s", root.Kind(), root.ToS())
	}
}
