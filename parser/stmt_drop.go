package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

func init() {
	statementParsers[tokens.DROP] = (*Parser).parseDrop
}

// parseDrop ports _parse_drop (parser.py:2307-2351). creatables (sets.go) already covers
// COLUMN/CONSTRAINT/FOREIGN_KEY/FUNCTION/INDEX/PROCEDURE/TRIGGER/TYPE/DB_CREATABLES
// (DATABASE/.../TABLE/VIEW/...); a MATERIALIZED prefix is a separate boolean flag (not part
// of "kind"), matching upstream's `materialized = self._match_text_seq("MATERIALIZED")`
// preceding the kind match. CREATABLE_KIND_MAPPING is empty for base/mysql/postgres
// (dialects/dialect.py), so kind is used verbatim.
func (p *Parser) parseDrop() exp.Expression {
	start := p.prev
	temporary := p.match(tokens.TEMPORARY)
	materialized := p.matchTextSeq("MATERIALIZED")
	iceberg := p.matchTextSeq("ICEBERG")

	var kind string
	if p.matchSet(creatables) {
		kind = stringsUpper(p.prev.Text)
	}
	if kind == "" || (iceberg && kind != "TABLE") {
		// Grammar extension: MySQL DROP {USER|ROLE} — structure as a Drop root carrying the object
		// kind, body (IF EXISTS + name list) kept verbatim (see stmt_account_object.go). Only the plain
		// form (no TEMPORARY/MATERIALIZED/ICEBERG prefix) qualifies; anything else degrades to Command.
		if !temporary && !materialized && !iceberg {
			if accountKind := p.mysqlAccountObjectKind(true, true); accountKind != "" {
				return p.parseAccountObjectStatement(start, exp.KindDrop, accountKind)
			}
		}
		return p.parseAsCommand(start)
	}

	concurrently := p.matchTextSeq("CONCURRENTLY")
	ifExists := p.parseExists(false)

	var this exp.Expression
	if kind == "COLUMN" {
		this = p.parseColumn()
	} else {
		this = p.parseTableParts(true, kind == "SCHEMA", false, false)
	}

	var cluster exp.Expression
	if p.match(tokens.ON) {
		// `DROP INDEX <idx> ON <table>` (MySQL): the ON-clause target is an OnProperty carrying the
		// table (upstream Drop.cluster, parser.py:2325). divergence: upstream's shared _parse_on_property
		// parses only a single id (_parse_schema(_parse_id_var), parser.py:3345), so it rejects the
		// db-qualified `ON db.tbl` that real MySQL 8.0.46 accepts. Parse the full table parts here —
		// scoped to DROP so CREATE/ALTER keep upstream's ON handling — so the qualifier survives. The
		// missing-name case fails closed via parseTableParts' own raiseError. See DEVIATIONS §1.
		cluster = p.expression(exp.OnProperty(exp.Args{"this": p.parseTableParts(false, false, false, false)}), nil, nil)
	}

	var expressions []exp.Expression
	if p.match(tokens.L_PAREN, false) {
		expressions = p.parseWrappedCsv(func() exp.Expression { return p.parseTypes(false, false, true, false) })
	}

	var cascadeOrRestrict string
	if p.matchTextSeq("CASCADE") {
		cascadeOrRestrict = "CASCADE"
	} else if p.matchTextSeq("RESTRICT") {
		cascadeOrRestrict = "RESTRICT"
	}

	return p.expression(exp.Drop(exp.Args{
		"exists":       ifExists,
		"this":         this,
		"expressions":  expressions,
		"kind":         kind,
		"temporary":    temporary,
		"materialized": materialized,
		"cascade":      cascadeOrRestrict == "CASCADE",
		"restrict":     cascadeOrRestrict == "RESTRICT",
		"constraints":  p.matchTextSeq("CONSTRAINTS"),
		"purge":        p.matchTextSeq("PURGE"),
		"cluster":      cluster,
		"concurrently": concurrently,
		"sync":         p.matchTextSeq("SYNC"),
		"iceberg":      iceberg,
	}), nil, nil)
}
