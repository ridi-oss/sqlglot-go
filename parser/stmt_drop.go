package parser

import (
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/tokens"
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
		cluster = p.parseOnProperty()
		if cluster == nil {
			// `DROP INDEX <idx> ON <table>`: the ON-clause target is an exp.OnProperty (not
			// ported; parseOnProperty matches ON but returns nil, leaving <table> unconsumed).
			// parseDrop isn't wrapped in tryParse, so the leftover would otherwise surface as a
			// hard "Unexpected token" error at the batch level. Degrade to a raw Command (guide:
			// "leftover/unmatched -> parseAsCommand(start)"); parseAsCommand is source-position
			// based, so it re-captures the whole statement and round-trips byte-identically.
			// Scoped to the ON branch so parseDrop stays reusable as a CSV sub-parser inside
			// ALTER ... DROP (e.g. `DROP COLUMN c, DROP PRIMARY KEY`), which never has an ON.
			return p.parseAsCommand(start)
		}
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
