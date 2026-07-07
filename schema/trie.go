package schema

type TrieResult int

const (
	TrieFailed TrieResult = iota
	TriePrefix
	TrieExists
)

type trieNode struct {
	children map[string]*trieNode
	order    []string
	terminal bool
}

func newTrie(keys [][]string, base *trieNode) *trieNode {
	trie := base
	if trie == nil {
		trie = &trieNode{}
	}
	for _, key := range keys {
		current := trie
		for _, part := range key {
			if current.children == nil {
				current.children = map[string]*trieNode{}
			}
			next := current.children[part]
			if next == nil {
				next = &trieNode{}
				current.children[part] = next
				current.order = append(current.order, part)
			}
			current = next
		}
		current.terminal = true
	}
	return trie
}

func inTrie(t *trieNode, key []string) (TrieResult, *trieNode) {
	if t == nil {
		t = &trieNode{}
	}
	if len(key) == 0 {
		return TrieFailed, t
	}
	current := t
	for _, part := range key {
		next := current.children[part]
		if next == nil {
			return TrieFailed, current
		}
		current = next
	}
	if current.terminal {
		return TrieExists, current
	}
	return TriePrefix, current
}

func (t *trieNode) flatten() [][]string {
	// Schema's string-key trie uses a terminal bool instead of trie.py's int(0)
	// sentinel, so a DFS over insertion-order children is equivalent to
	// flatten_schema(subtrie) without conflating sentinel keys with table parts.
	if t == nil {
		return nil
	}
	var out [][]string
	var walk func(*trieNode, []string)
	walk = func(node *trieNode, path []string) {
		if node.terminal {
			out = append(out, append([]string(nil), path...))
		}
		for _, key := range node.order {
			walk(node.children[key], append(path, key))
		}
	}
	walk(t, nil)
	return out
}
