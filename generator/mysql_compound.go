package generator

import (
	"strings"

	"github.com/ridi-oss/sqlglot-go/expressions"
)

// Renderers for the MySQL compound-statement grammar extension (parser/mysql_compound.go,
// DEVIATIONS §1.18). Upstream has no counterparts; shapes follow the reference manual §15.6.

func init() {
	dispatch[expressions.KindIfBlock] = (*Generator).ifBlockSQL
	dispatch[expressions.KindLoopBlock] = (*Generator).loopBlockSQL
	dispatch[expressions.KindRepeatBlock] = (*Generator).repeatBlockSQL
	dispatch[expressions.KindCaseBlock] = (*Generator).caseBlockSQL
	dispatch[expressions.KindDeclare] = (*Generator).declareSQL
	dispatch[expressions.KindDeclareItem] = (*Generator).declareItemSQL
}

// compoundBodySQL renders a statement list with each statement `;`-terminated (MySQL's
// stmt_list, §15.6.1), a trailing EndStatement child rendering as the closing END. A
// nested BEGIN block child (exp.Block, whose own EndStatement supplies its END) gets the
// BEGIN prefix and label decoration here.
func (g *Generator) compoundBodySQL(stmts []expressions.Expression) string {
	var b strings.Builder
	for _, s := range stmts {
		if s == nil {
			continue
		}
		if s.Kind() == expressions.KindEndStatement {
			b.WriteString("END")
			return b.String()
		}
		if s.Kind() == expressions.KindBlock {
			b.WriteString(g.labeledBlockSQL(s))
		} else {
			b.WriteString(g.gen(s))
		}
		b.WriteString("; ")
	}
	return strings.TrimSuffix(b.String(), " ")
}

// labeledBlockSQL renders a BEGIN block as a body statement: `[lbl: ]BEGIN <stmts>; END[ lbl]`
// (the block's own EndStatement child supplies the END).
func (g *Generator) labeledBlockSQL(e expressions.Expression) string {
	label := stringValue(e.Arg("label"))
	sql := "BEGIN " + g.gen(e)
	if label != "" {
		sql = label + ": " + sql + " " + label
	}
	return sql
}

func (g *Generator) bodyKeySQL(e expressions.Expression, key string) string {
	child := asExpression(e.Arg(key))
	if child == nil {
		return ""
	}
	if child.Kind() == expressions.KindBlock {
		return g.compoundBodySQL(child.Expressions())
	}
	return g.gen(child)
}

// IF cond THEN stmts; [ELSEIF cond THEN stmts;]... [ELSE stmts;] END IF (§15.6.5.2).
// ELSEIF chains live in the false child as a nested IfBlock.
func (g *Generator) ifBlockSQL(e expressions.Expression) string {
	return "IF " + g.ifBlockChainSQL(e) + " END IF"
}

func (g *Generator) ifBlockChainSQL(e expressions.Expression) string {
	sql := g.sqlKey(e, "this") + " THEN " + g.bodyKeySQL(e, "true")
	if f := asExpression(e.Arg("false")); f != nil {
		if f.Kind() == expressions.KindIfBlock {
			return sql + " ELSEIF " + g.ifBlockChainSQL(f)
		}
		return sql + " ELSE " + g.compoundBodySQL(f.Expressions())
	}
	return sql
}

// [lbl: ]LOOP stmts; END LOOP[ lbl] (§15.6.5.5).
func (g *Generator) loopBlockSQL(e expressions.Expression) string {
	return g.labelWrap(e, "LOOP "+g.compoundBodySQL(e.Expressions())+" END LOOP")
}

// [lbl: ]REPEAT stmts; UNTIL cond END REPEAT[ lbl] (§15.6.5.6).
func (g *Generator) repeatBlockSQL(e expressions.Expression) string {
	return g.labelWrap(e, "REPEAT "+g.compoundBodySQL(e.Expressions())+" UNTIL "+g.sqlKey(e, "until")+" END REPEAT")
}

// CASE [op] WHEN v THEN stmts;... [ELSE stmts;] END CASE (§15.6.5.1). Arms are exp.If
// nodes whose true child is a Block.
func (g *Generator) caseBlockSQL(e expressions.Expression) string {
	var b strings.Builder
	b.WriteString("CASE")
	if op := g.sqlKey(e, "this"); op != "" {
		b.WriteString(" " + op)
	}
	for _, when := range listFromValue(e.Arg("whens")) {
		w, _ := when.(expressions.Expression)
		if w == nil {
			continue
		}
		b.WriteString(" WHEN " + g.sqlKey(w, "this") + " THEN " + g.bodyKeySQL(w, "true"))
	}
	if els := asExpression(e.Arg("else_")); els != nil {
		b.WriteString(" ELSE " + g.compoundBodySQL(els.Expressions()))
	}
	b.WriteString(" END CASE")
	return b.String()
}

// DECLARE a[, b]... type [DEFAULT expr] (§15.6.4.1). Items share one type/default.
func (g *Generator) declareSQL(e expressions.Expression) string {
	items := e.Expressions()
	if len(items) == 0 {
		return "DECLARE"
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, g.sqlKey(it, "this"))
	}
	sql := "DECLARE " + strings.Join(names, ", ")
	if kind := g.sqlKey(items[0], "kind"); kind != "" {
		sql += " " + kind
	}
	if dflt := g.sqlKey(items[0], "default"); dflt != "" {
		sql += " DEFAULT " + dflt
	}
	return sql
}

func (g *Generator) declareItemSQL(e expressions.Expression) string {
	sql := g.sqlKey(e, "this")
	if kind := g.sqlKey(e, "kind"); kind != "" {
		sql += " " + kind
	}
	if dflt := g.sqlKey(e, "default"); dflt != "" {
		sql += " DEFAULT " + dflt
	}
	return sql
}

func (g *Generator) labelWrap(e expressions.Expression, sql string) string {
	if label := stringValue(e.Arg("label")); label != "" {
		return label + ": " + sql + " " + label
	}
	return sql
}
