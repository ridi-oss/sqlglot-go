package parser

import (
	"strings"

	exp "github.com/ridi-oss/sqlglot-go/expressions"
	"github.com/ridi-oss/sqlglot-go/tokens"
)

// parseCommand ports _parse_command (parser.py:2184-2190): the generic fallback for any
// leading token in the dialect's tokenizer Commands set (CALL/EXPLAIN/OPTIMIZE/PREPARE/
// VACUUM plus EXECUTE/FETCH/SHOW/RENAME, per-dialect KEYWORDS/COMMANDS overrides -
// mysql's `LOCK TABLES`/`UNLOCK TABLES` for example). p.prev is the leading token
// (parseStatement advances past it before dispatching here, mirroring upstream's use of
// self._prev); the tokenizer has already packed the remainder of the statement into a
// single STRING token (tokens/tokenizer_core.go:256-266, mirroring the Commands/
// CommandPrefixTokens behavior of upstream's Rust tokenizer), so parseString picks it up
// whole and the round-trip is byte-identical. _warn_unsupported is omitted: this port has
// no logger (see ROADMAP's known-divergences ledger).
func (p *Parser) parseCommand() exp.Expression {
	comments := p.prevComments
	return p.expression(exp.Command(exp.Args{"this": stringsUpper(p.prev.Text), "expression": p.parseString()}), nil, comments)
}

// parsePragma ports the inline TokenType.PRAGMA STATEMENT_PARSERS entry (parser.py:1101):
// `PRAGMA <expr>`, e.g. sqlite's `PRAGMA quick_check`.
func (p *Parser) parsePragma() exp.Expression {
	return p.expression(exp.Pragma(exp.Args{"this": p.parseExpression()}), nil, nil)
}

// parsePartition ports _parse_partition (parser.py:3772-3781): `PARTITION(...)` /
// `SUBPARTITION(...)`, used by TRUNCATE/ANALYZE/... family parsers.
func (p *Parser) parsePartition() exp.Expression {
	if !p.matchTexts(partitionKeywords) {
		return nil
	}
	return p.expression(exp.Partition(exp.Args{
		"subpartition": stringsUpper(p.prev.Text) == "SUBPARTITION",
		"expressions":  p.parseWrappedCsv(p.parseDisjunction),
	}), nil, nil)
}

// parseProperties ports _parse_properties (parser.py:2879-2894). Property parsers have a
// singleton return contract in Go, so the few upstream entries which return a list use a
// temporary Properties wrapper; flattening it here preserves upstream's final AST shape.
func (p *Parser) parseProperties(before ...bool) exp.Expression {
	parseBefore := len(before) > 0 && before[0]
	properties := []exp.Expression{}
	for {
		var property exp.Expression
		if parseBefore {
			property = p.parsePropertyBefore()
		} else {
			property = p.parseProperty()
		}
		if property == nil {
			break
		}
		if property.Kind() == exp.KindProperties {
			properties = append(properties, property.Expressions()...)
		} else {
			properties = append(properties, property)
		}
	}
	if len(properties) == 0 {
		return nil
	}
	return p.expression(exp.Properties(exp.Args{"expressions": properties}), nil, nil)
}

// wordTrieNode is a node in a trie keyed by whole (uppercased) words - one dispatch-map
// key may be several words, e.g. SHOW_PARSERS' "COLUMNS FROM" - rather than by individual
// characters. It ports new_trie/in_trie (trie.py), which findParser below walks exactly
// as _find_parser (parser.py:9404-9426) does.
type wordTrieNode struct {
	children map[string]*wordTrieNode
	terminal bool
}

// wordTrie is the type family builders build (e.g. SET_TRIE, SHOW_TRIE) and pass to
// findParser.
type wordTrie = *wordTrieNode

// newTrie builds a wordTrie from a set of (possibly multi-word) dispatch keys, e.g.
// "SESSION" or "COLUMNS FROM" (ports `new_trie(key.split(" ") for key in X_PARSERS)`,
// parser.py:1854-1855 and per-dialect equivalents).
func newTrie(keys []string) wordTrie {
	root := &wordTrieNode{children: map[string]*wordTrieNode{}}
	for _, key := range keys {
		node := root
		for _, word := range strings.Split(key, " ") {
			next := node.children[word]
			if next == nil {
				next = &wordTrieNode{children: map[string]*wordTrieNode{}}
				node.children[word] = next
			}
			node = next
		}
		node.terminal = true
	}
	return root
}

// findParser ports _find_parser (parser.py:9404-9426): walks tokens one at a time (each
// token's uppercased text is itself split on spaces, matching upstream's `curr.split(" ")`
// for the rare multi-word-single-token case, e.g. a keyword whose canonical text embeds a
// space) through trie, returning the dispatch func keyed by the longest matching sequence
// of words in parsers, or nil (after retreating) if nothing matches. Unlike upstream,
// running out of tokens mid-walk can't panic here: an exhausted p.curr is the SENTINEL
// token (empty Text), which simply fails the next trie lookup and breaks the loop -
// a Go-necessitated, behavior-preserving divergence (upstream never actually hits this
// because no SET_PARSERS/SHOW_PARSERS key is a strict, unterminated prefix of another at
// end of input in the corpus).
func (p *Parser) findParser(parsers map[string]func(*Parser) exp.Expression, trie wordTrie) func(*Parser) exp.Expression {
	if !p.curr.IsValid() {
		return nil
	}
	index := p.index
	var this []string
	node := trie
	for {
		curr := stringsUpper(p.curr.Text)
		// A quoted identifier or string literal is never an (unquoted) dispatch keyword — real engines
		// reject a quoted keyword in this position (`SET "ROLE" x` / `SHOW "CREATE" USER` are syntax
		// errors) or read it as an ordinary name (`` `PASSWORD` = x`` is a variable assignment).
		// Matching it by text would launder those into a structured privileged statement, so treat it as
		// a non-match (failed) — the walk then fails closed to the assignment/Command fallback.
		failed := p.curr.TokenType == tokens.IDENTIFIER || p.curr.TokenType == tokens.STRING
		this = append(this, curr)
		p.advance()

		for _, word := range strings.Split(curr, " ") {
			if node == nil {
				failed = true
				break
			}
			next := node.children[word]
			if next == nil {
				failed = true
				break
			}
			node = next
		}
		if failed {
			break
		}
		if node.terminal {
			return parsers[strings.Join(this, " ")]
		}
	}
	p.retreat(index)
	return nil
}
