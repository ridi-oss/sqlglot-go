package parser

import (
	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

func (p *Parser) parseAthenaExplainStatement() exp.Expression {
	start := p.prev
	if e := p.tryParse(p.parseAthenaExplain, false); e != nil {
		return e
	}
	return p.parseAsCommand(start)
}

func (p *Parser) parseAthenaUnloadStatement() exp.Expression {
	start := p.prev
	if e := p.tryParse(p.parseAthenaUnload, false); e != nil {
		return e
	}
	return p.parseAsCommand(start)
}

func (p *Parser) parseHiveShow() exp.Expression {
	start := p.prev
	if e := p.tryParse(p.parseHiveShowStructured, false); e != nil {
		return e
	}
	return p.parseAsCommand(start)
}

func (p *Parser) parseHiveShowStructured() exp.Expression {
	if !p.curr.IsValid() || p.curr.TokenType == tokens.STRING || p.curr.TokenType == tokens.IDENTIFIER {
		return nil
	}
	name := stringsUpper(p.curr.Text)
	p.advance()
	args := exp.Args{}
	target := false
	switch name {
	case "COLUMNS":
		if !p.matchTexts(map[string]bool{"FROM": true, "IN": true}) {
			return nil
		}
		args["from_"] = stringsUpper(p.prev.Text)
		target = true
	case "CREATE":
		if !p.matchTexts(map[string]bool{"TABLE": true, "VIEW": true}) {
			return nil
		}
		name += " " + stringsUpper(p.prev.Text)
		target = true
	case "PARTITIONS", "TBLPROPERTIES":
		target = true
	case "DATABASES", "SCHEMAS", "TABLES", "VIEWS":
	default:
		return nil
	}
	args["this"] = name
	if target {
		table := p.parseTableParts(true, false, false, false)
		if table.This() == nil || table.This().Kind() != exp.KindIdentifier || table.Name() == "" || table.Arg("pivots") != nil {
			return nil
		}
		args["target"] = table
	}
	if name == "COLUMNS" && p.matchTexts(map[string]bool{"FROM": true, "IN": true}) {
		// SHOW COLUMNS {FROM|IN} table {FROM|IN} database (table must then be unqualified).
		table := args["target"].(exp.Expression)
		if table.Arg("schema") != nil {
			return nil
		}
		db := p.parseAthenaDBReference()
		if db == nil {
			return nil
		}
		args["db"] = db
	}
	if name == "TABLES" || name == "VIEWS" {
		if p.matchTextSeq("IN") {
			db := p.parseAthenaDBReference()
			if db == nil {
				return nil
			}
			args["db"] = db
		}
	}
	switch name {
	case "DATABASES", "SCHEMAS", "VIEWS":
		if p.matchTextSeq("LIKE") {
			pattern := p.parseString()
			if pattern == nil {
				return nil
			}
			args["like"] = pattern
		}
	case "TABLES":
		args["like"] = p.parseString()
	case "TBLPROPERTIES":
		if p.match(tokens.L_PAREN) {
			key := p.parseString()
			if key == nil || !p.match(tokens.R_PAREN) {
				return nil
			}
			args["like"] = key
		}
	}
	if p.curr.IsValid() {
		return nil
	}
	return p.expression(exp.Show(args), nil, nil)
}

// parseAthenaDBReference parses `[catalog.]database` (name lands in "schema"); a third part
// would be dropped silently by parseTableParts, so more than one dot fails closed.
func (p *Parser) parseAthenaDBReference() exp.Expression {
	start := p.index
	db := p.parseTableParts(true, true, false, false)
	dots := 0
	for i := start; i < p.index; i++ {
		if p.tokens[i].TokenType == tokens.DOT {
			dots++
		}
	}
	if db == nil || db.This() != nil || dots > 1 || db.Arg("pivots") != nil {
		return nil
	}
	schema := asExpressionArg(db, "schema")
	if schema == nil || schema.Kind() != exp.KindIdentifier || schema.Name() == "" {
		return nil
	}
	if catalog := db.Arg("catalog"); catalog != nil {
		c, ok := catalog.(exp.Expression)
		if !ok || c.Kind() != exp.KindIdentifier || c.Name() == "" {
			return nil
		}
	}
	return db
}

func asExpressionArg(e exp.Expression, key string) exp.Expression {
	v, _ := e.Arg(key).(exp.Expression)
	return v
}

func (p *Parser) parseAthenaExplain() exp.Expression {
	var options []exp.Expression
	var style any
	analyze := p.matchTextSeq("ANALYZE")
	if analyze {
		style = "ANALYZE"
	}
	wrapped := p.match(tokens.L_PAREN)
	if wrapped {
		for {
			if p.curr.TokenType == tokens.STRING || p.curr.TokenType == tokens.IDENTIFIER {
				return nil
			}
			name := stringsUpper(p.curr.Text)
			var values map[string]bool
			switch {
			case name == "FORMAT" && analyze:
				values = map[string]bool{"TEXT": true, "JSON": true}
			case name == "FORMAT":
				values = map[string]bool{"TEXT": true, "GRAPHVIZ": true, "JSON": true}
			case name == "TYPE" && !analyze:
				values = map[string]bool{"LOGICAL": true, "DISTRIBUTED": true, "VALIDATE": true, "IO": true}
			default:
				return nil
			}
			p.advance()
			if p.curr.TokenType == tokens.STRING || p.curr.TokenType == tokens.IDENTIFIER || !p.matchTexts(values) {
				return nil
			}
			options = append(options, p.postgresExplainOption(name, stringsUpper(p.prev.Text)))
			if p.match(tokens.R_PAREN) {
				break
			}
			if !p.match(tokens.COMMA) {
				return nil
			}
		}
	}
	inner := p.parseStatement()
	if !athenaExplainTarget(inner) || p.curr.IsValid() {
		return nil
	}
	return p.expression(exp.Describe(exp.Args{"this": inner, "kind": "EXPLAIN", "style": style, "expressions": options, "wrapped": wrapped}), nil, nil)
}

func athenaExplainTarget(e exp.Expression) bool {
	if e == nil {
		return false
	}
	switch e.Kind() {
	case exp.KindSelect, exp.KindUnion, exp.KindIntersect, exp.KindExcept, exp.KindInsert:
		return e.Find(exp.KindCommand) == nil
	case exp.KindCreate:
		return e.Text("kind") == "TABLE" && e.Expr() != nil && e.Find(exp.KindCommand) == nil
	}
	return false
}

// https://docs.aws.amazon.com/athena/latest/ug/unload.html: format is required.
var athenaUnloadProperties = map[string]bool{"format": true, "compression": true, "compression_level": true, "field_delimiter": true, "partitioned_by": true}
var athenaUnloadFormats = map[string]bool{"ORC": true, "PARQUET": true, "AVRO": true, "JSON": true, "TEXTFILE": true}

func (p *Parser) parseAthenaUnload() exp.Expression {
	if !p.match(tokens.L_PAREN, false) {
		return nil
	}
	query := p.parseSelect(true, false, false)
	if query == nil || query.Kind() != exp.KindSubquery || !athenaExplainTarget(query.This()) {
		return nil
	}
	if !p.matchTextSeq("TO") {
		return nil
	}
	file := p.parseString()
	if file == nil || !p.matchTextSeq("WITH") || !p.match(tokens.L_PAREN) {
		return nil
	}
	var params []exp.Expression
	seen := map[string]bool{}
	for {
		// `partitioned_by` tokenizes as the PARTITION_BY keyword, so match property names by text.
		if p.curr.TokenType == tokens.STRING || p.curr.TokenType == tokens.IDENTIFIER || !p.curr.IsValid() {
			return nil
		}
		name := p.curr
		key := stringsLower(name.Text)
		if !athenaUnloadProperties[key] || seen[key] {
			return nil
		}
		p.advance()
		if !p.match(tokens.EQ) {
			return nil
		}
		seen[key] = true
		value := p.parseDisjunction()
		if value == nil {
			return nil
		}
		if key == "format" && (!value.IsString() || !athenaUnloadFormats[stringsUpper(value.Name())]) {
			return nil
		}
		params = append(params, p.expression(exp.CopyParameter(exp.Args{"this": exp.Var(exp.Args{"this": name.Text}), "expression": value}), nil, nil))
		if p.match(tokens.R_PAREN) {
			break
		}
		if !p.match(tokens.COMMA) {
			return nil
		}
	}
	if !seen["format"] || p.curr.IsValid() {
		return nil
	}
	return p.expression(exp.Unload(exp.Args{"this": query, "files": []exp.Expression{file}, "params": params}), nil, nil)
}

func (p *Parser) parseHiveAlter() exp.Expression {
	start := p.prev
	if e := p.tryParse(func() exp.Expression {
		e := p.parseAlter()
		if e == nil {
			return nil
		}
		rename := e.Find(exp.KindAlterRename)
		if rename != nil && rename.This() != nil && rename.This().Kind() == exp.KindPartition {
			if e.This() == nil {
				return nil
			}
			partition, _ := e.This().Arg("partition").(exp.Expression)
			if !athenaPartitionAssignments(partition) {
				return nil
			}
		}
		return e
	}, false); e != nil {
		return e
	}
	return p.parseAsCommand(start)
}

func athenaPartitionAssignments(partition exp.Expression) bool {
	if partition == nil || len(partition.Expressions()) == 0 {
		return false
	}
	for _, item := range partition.Expressions() {
		if item.Kind() != exp.KindEQ || item.This() == nil || item.This().Kind() != exp.KindColumn || item.Arg("expression") == nil {
			return false
		}
	}
	return true
}
