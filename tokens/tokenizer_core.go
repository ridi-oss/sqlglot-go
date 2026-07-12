package tokens

import (
	"fmt"
	"math/big"
	"strings"
	"unicode"

	sqlerrors "github.com/sjincho/sqlglot-go/errors"
	"github.com/sjincho/sqlglot-go/trie"
)

var digitChars = map[rune]bool{
	'0': true, '1': true, '2': true, '3': true, '4': true,
	'5': true, '6': true, '7': true, '8': true, '9': true,
}

type TokenizerCore struct {
	config TokenizerConfig
	sql    []rune
	size   int
	tokens []Token

	start         int
	current       int
	line          int
	col           int
	comments      []string
	char          rune
	end           bool
	peek          rune
	prevTokenLine int

	singleTokens                     map[rune]TokenType
	keywords                         map[string]TokenType
	quotes                           map[string]string
	formatStrings                    map[string]FormatString
	identifiers                      map[rune]string
	commentsConfig                   map[string]string
	mysqlExecutableComments          bool
	mysqlVersion                     *int
	lineCommentRequiresSpace         map[string]bool
	stringEscapes                    map[rune]bool
	byteStringEscapes                map[rune]bool
	identifierEscapes                map[rune]bool
	escapeFollowChars                map[rune]bool
	commands                         map[TokenType]bool
	commandPrefixTokens              map[TokenType]bool
	nestedComments                   bool
	hintStart                        string
	tokensPrecedingHint              map[TokenType]bool
	hasBitStrings                    bool
	hasHexStrings                    bool
	numericLiterals                  map[string]string
	varSingleTokens                  map[rune]bool
	stringEscapesAllowedInRawStrings bool
	heredocTagIsIdentifier           bool
	heredocStringAlternative         TokenType
	keywordTrie                      *trie.Node
	numbersCanBeUnderscoreSeparated  bool
	numbersCanHaveDecimals           bool
	identifiersCanStartWithDigit     bool
	unescapedSequences               map[string]string
}

func NewTokenizerCore(cfg TokenizerConfig) *TokenizerCore {
	return &TokenizerCore{
		config:                           cfg,
		singleTokens:                     cfg.SingleTokens,
		keywords:                         cfg.Keywords,
		quotes:                           cfg.Quotes,
		formatStrings:                    cfg.FormatStrings,
		identifiers:                      cfg.Identifiers,
		commentsConfig:                   cfg.Comments,
		mysqlExecutableComments:          cfg.MySQLExecutableComments,
		mysqlVersion:                     cfg.MySQLVersion,
		lineCommentRequiresSpace:         cfg.LineCommentRequiresSpace,
		stringEscapes:                    cfg.StringEscapes,
		byteStringEscapes:                cfg.ByteStringEscapes,
		identifierEscapes:                cfg.IdentifierEscapes,
		escapeFollowChars:                cfg.EscapeFollowChars,
		commands:                         cfg.Commands,
		commandPrefixTokens:              cfg.CommandPrefixTokens,
		nestedComments:                   cfg.NestedComments,
		hintStart:                        cfg.HintStart,
		tokensPrecedingHint:              cfg.TokensPrecedingHint,
		hasBitStrings:                    cfg.HasBitStrings,
		hasHexStrings:                    cfg.HasHexStrings,
		numericLiterals:                  cfg.NumericLiterals,
		varSingleTokens:                  cfg.VarSingleTokens,
		stringEscapesAllowedInRawStrings: cfg.StringEscapesAllowedInRawStrings,
		heredocTagIsIdentifier:           cfg.HeredocTagIsIdentifier,
		heredocStringAlternative:         cfg.HeredocStringAlternative,
		keywordTrie:                      cfg.KeywordTrie,
		numbersCanBeUnderscoreSeparated:  cfg.NumbersCanBeUnderscoreSeparated,
		numbersCanHaveDecimals:           cfg.NumbersCanHaveDecimals,
		identifiersCanStartWithDigit:     cfg.IdentifiersCanStartWithDigit,
		unescapedSequences:               cfg.UnescapedSequences,
		line:                             1,
		prevTokenLine:                    -1,
	}
}

func (c *TokenizerCore) reset() {
	c.sql = nil
	c.size = 0
	c.tokens = nil
	c.start = 0
	c.current = 0
	c.line = 1
	c.col = 0
	c.comments = nil
	c.char = 0
	c.end = false
	c.peek = 0
	c.prevTokenLine = -1
}

func (c *TokenizerCore) Tokenize(sql string) (tokens []Token, err error) {
	c.reset()
	c.sql = []rune(sql)
	c.size = len(c.sql)

	defer func() {
		if r := recover(); r != nil {
			start := maxInt(c.current-50, 0)
			end := minInt(c.current+50, c.size-1)
			context := ""
			if end > start {
				context = string(c.sql[start:end])
			}
			err = &sqlerrors.TokenError{Msg: fmt.Sprintf("Error tokenizing '%s'", context)}
			tokens = nil
		}
	}()

	c.scan(false)
	return c.tokens, nil
}

func (c *TokenizerCore) scan(checkSemicolon bool) {
	for c.size > 0 && !c.end {
		current := c.current
		for current < c.size {
			char := c.sql[current]
			if char == ' ' || char == '\t' {
				current++
			} else {
				break
			}
		}

		offset := 1
		if current > c.current {
			offset = current - c.current
		}

		c.start = current
		c.advance(offset, false)

		if !unicode.IsSpace(c.char) {
			if digitChars[c.char] {
				c.scanNumber()
			} else if end, ok := c.identifiers[c.char]; ok {
				c.scanIdentifier(end)
			} else {
				c.scanKeywords()
			}
		}

		if checkSemicolon && c.peek == ';' {
			break
		}
	}

	if len(c.tokens) > 0 && len(c.comments) > 0 {
		c.tokens[len(c.tokens)-1].Comments = append(c.tokens[len(c.tokens)-1].Comments, c.comments...)
	}
}

func (c *TokenizerCore) chars(size int) string {
	if size == 1 {
		if c.char == 0 {
			return ""
		}
		return string(c.char)
	}
	start := c.current - 1
	end := start + size
	if start < 0 || end > c.size {
		return ""
	}
	return string(c.sql[start:end])
}

func (c *TokenizerCore) advance(i int, alnum bool) {
	char := c.char

	if char == '\n' || char == '\r' {
		if !(char == '\r' && c.peek == '\n') {
			c.col = i
			c.line++
		}
	} else {
		c.col += i
	}

	c.current += i
	c.end = c.current >= c.size
	c.char = c.sql[c.current-1]
	if c.end {
		c.peek = 0
	} else {
		c.peek = c.sql[c.current]
	}

	if alnum && isAlnum(c.char) {
		_col := c.col
		_current := c.current
		_end := c.end
		_peek := c.peek

		for isAlnum(_peek) {
			_col++
			_current++
			_end = _current >= c.size
			if _end {
				_peek = 0
			} else {
				_peek = c.sql[_current]
			}
		}

		c.col = _col
		c.current = _current
		c.end = _end
		c.peek = _peek
		c.char = c.sql[_current-1]
	}
}

func (c *TokenizerCore) text() string {
	return string(c.sql[c.start:c.current])
}

func (c *TokenizerCore) add(tokenType TokenType, text ...string) {
	c.prevTokenLine = c.line

	if len(c.comments) > 0 && tokenType == SEMICOLON && len(c.tokens) > 0 {
		c.tokens[len(c.tokens)-1].Comments = append(c.tokens[len(c.tokens)-1].Comments, c.comments...)
		c.comments = nil
	}

	tokenText := ""
	if len(text) > 0 {
		tokenText = text[0]
	} else {
		tokenText = c.text()
	}

	comments := append([]string(nil), c.comments...)
	c.tokens = append(c.tokens, NewTokenFull(tokenType, tokenText, c.line, c.col, c.start, c.current-1, comments))
	c.comments = nil

	if c.commands[tokenType] && c.peek != ';' && (len(c.tokens) == 1 || c.commandPrefixTokens[c.tokens[len(c.tokens)-2].TokenType]) {
		start := c.current
		tokenCount := len(c.tokens)
		c.scan(true)
		c.tokens = c.tokens[:tokenCount]
		text := strings.TrimSpace(string(c.sql[start:c.current]))
		if text != "" {
			c.add(STRING, text)
		}
	}
}

func (c *TokenizerCore) scanKeywords() {
	size := 0
	word := ""
	chars := string(c.char)
	char := c.char
	prevSpace := false
	skip := false
	node := c.keywordTrie
	singleToken := c.singleTokens[char] != 0

	for chars != "" {
		if !skip {
			upper := upperASCII(char)
			if node == nil || node.Children == nil || node.Children[upper] == nil {
				break
			}
			node = node.Children[upper]
			if node.Terminal {
				word = chars
			}
		}

		end := c.current + size
		size++

		if end < c.size {
			char = c.sql[end]
			if c.singleTokens[char] != 0 {
				singleToken = true
			}
			isSpace := unicode.IsSpace(char)
			if !isSpace || !prevSpace {
				if isSpace {
					char = ' '
				}
				chars += string(char)
				prevSpace = isSpace
				skip = false
			} else {
				skip = true
			}
		} else {
			char = 0
			break
		}
	}

	if word != "" {
		if c.scanString(word) {
			return
		}
		// divergence: some dialects (MySQL) only begin a line comment on a start like
		// `--` when it is immediately followed by whitespace/control or EOF; otherwise
		// `--` is two `-` operators (`1--2` == `1 - -2`). Upstream sqlglot mis-tokenizes
		// this (drops `--2` as a comment) — we match the real engine. See DEVIATIONS §1.4.
		// When suppressed, fall through so only the first rune is emitted as a single
		// token and the tokenizer re-scans the remainder.
		if !c.lineCommentSuppressed(word) {
			if c.scanComment(word) {
				return
			}
			if prevSpace || singleToken || char == 0 {
				c.advance(size-1, false)
				upperWord := strings.ToUpper(word)
				c.add(c.keywords[upperWord], upperWord)
				return
			}
		}
	}

	if tokenType := c.singleTokens[c.char]; tokenType != 0 {
		c.add(tokenType, string(c.char))
		return
	}

	c.scanVar()
}

// lineCommentSuppressed reports whether commentStart, though matched as a line-comment
// start, must NOT begin a comment here because the dialect requires a trailing
// whitespace/control char (or EOF) and the next char is neither. MySQL `--`: `1--2` is
// `1 - -2`, not `1` + comment. The word begins at c.current-1, so the char after it is at
// c.current-1+len(word). See DEVIATIONS §1.4.
func (c *TokenizerCore) lineCommentSuppressed(commentStart string) bool {
	if !c.lineCommentRequiresSpace[commentStart] {
		return false
	}
	after := c.current - 1 + len([]rune(commentStart))
	if after >= c.size {
		return false // EOF right after the marker — a valid (empty) comment (verified vs MySQL 8.4).
	}
	next := c.sql[after]
	// MySQL honors only ASCII whitespace/control after `--`; a non-ASCII space such as NBSP
	// (U+00A0) does NOT start a comment (verified vs MySQL 8.4: `SELECT 1--<NBSP>2` errors,
	// it is not `SELECT 1`). Restrict the comment trigger to ASCII, so a non-ASCII byte
	// leaves `--` as two `-` operators.
	if next < 0x80 && (unicode.IsSpace(next) || unicode.IsControl(next)) {
		return false // ASCII whitespace/control follows — it is a comment.
	}
	return true // not a comment marker here — fall through to `-` operators.
}

func (c *TokenizerCore) scanComment(commentStart string) bool {
	commentEnd, ok := c.commentsConfig[commentStart]
	if !ok {
		return false
	}

	commentStartLine := c.line
	commentStartSize := len([]rune(commentStart))

	if commentEnd != "" {
		c.advance(commentStartSize, false)

		activeMySQLComment := false
		if commentStart == "/*" && c.mysqlExecutableComments && c.mysqlVersion != nil && c.char == '!' {
			activeMySQLComment = true
			gate := -1
			if c.current+5 <= c.size {
				gate = 0
				for i := 0; i < 5; i++ {
					digit := c.sql[c.current+i]
					if digit < '0' || digit > '9' {
						gate = -1
						break
					}
					gate = gate*10 + int(digit-'0')
				}
			}

			if gate >= 0 {
				activeMySQLComment = gate <= *c.mysqlVersion
				if activeMySQLComment {
					c.advance(5, false)
				}
			}
		}

		bodyStart := c.current
		bodyStartLine := c.line
		bodyStartCol := c.col
		commentCount := 1
		commentEndSize := len([]rune(commentEnd))

		for !c.end {
			if c.chars(commentEndSize) == commentEnd {
				commentCount--
				if commentCount == 0 {
					break
				}
			}

			c.advance(1, true)

			if c.nestedComments && !c.end && c.chars(commentEndSize) == commentStart {
				c.advance(commentStartSize, false)
				commentCount++
			}
		}

		if activeMySQLComment {
			bodyEnd := c.current - 1
			bodyTokens, err := NewTokenizerCore(c.config).Tokenize(string(c.sql[bodyStart:bodyEnd]))
			if err != nil {
				panic(err)
			}
			for i := range bodyTokens {
				nestedLine := bodyTokens[i].Line
				bodyTokens[i].Start += bodyStart
				bodyTokens[i].End += bodyStart
				bodyTokens[i].Line += bodyStartLine - 1
				if nestedLine == 1 {
					bodyTokens[i].Col += bodyStartCol
				}
			}
			if len(bodyTokens) > 0 {
				if len(c.comments) > 0 {
					comments := append([]string(nil), c.comments...)
					bodyTokens[0].Comments = append(comments, bodyTokens[0].Comments...)
					c.comments = nil
				}
				c.tokens = append(c.tokens, bodyTokens...)
				c.prevTokenLine = bodyTokens[len(bodyTokens)-1].Line
			}
			c.advance(commentEndSize-1, false)
			return true
		}

		textRunes := []rune(c.text())
		end := len(textRunes) - commentEndSize + 1
		if end < commentStartSize {
			end = commentStartSize
		}
		c.comments = append(c.comments, string(textRunes[commentStartSize:end]))
		c.advance(commentEndSize-1, false)
	} else {
		peek := c.peek
		for !c.end && peek != '\n' && peek != '\r' {
			c.advance(1, true)
			peek = c.peek
		}
		textRunes := []rune(c.text())
		c.comments = append(c.comments, string(textRunes[commentStartSize:]))
	}

	if commentStart == c.hintStart && len(c.tokens) > 0 && c.tokensPrecedingHint[c.tokens[len(c.tokens)-1].TokenType] {
		c.add(HINT)
	}

	if commentStartLine == c.prevTokenLine && len(c.tokens) > 0 {
		c.tokens[len(c.tokens)-1].Comments = append(c.tokens[len(c.tokens)-1].Comments, c.comments...)
		c.comments = nil
		c.prevTokenLine = c.line
	}

	return true
}

func (c *TokenizerCore) scanNumber() {
	if c.char == '0' {
		peek := upperASCII(c.peek)
		if peek == 'B' {
			if c.hasBitStrings {
				c.scanBits()
			} else {
				c.add(NUMBER)
			}
			return
		} else if peek == 'X' {
			if c.hasHexStrings {
				c.scanHex()
			} else {
				c.add(NUMBER)
			}
			return
		}
	}

	decimal := false
	scientific := 0
	isUnderscoreSeparated := false
	numberText := ""
	numericLiteral := ""
	var numericType TokenType

	for {
		if digitChars[c.peek] {
			end := c.current + 1
			for end < c.size && digitChars[c.sql[end]] {
				end++
			}
			c.advance(end-c.current, false)
		} else if c.peek == '.' && !decimal {
			if (len(c.tokens) > 0 && c.tokens[len(c.tokens)-1].TokenType == PARAMETER) || !c.numbersCanHaveDecimals {
				break
			}
			decimal = true
			c.advance(1, false)
		} else if (c.peek == '-' || c.peek == '+') && scientific == 1 {
			if c.current+1 < c.size && digitChars[c.sql[c.current+1]] {
				scientific++
				c.advance(1, false)
			} else {
				break
			}
		} else if upperASCII(c.peek) == 'E' && scientific == 0 {
			scientific++
			c.advance(1, false)
		} else if c.peek == '_' && c.numbersCanBeUnderscoreSeparated {
			isUnderscoreSeparated = true
			c.advance(1, false)
		} else if isIdentifierChar(c.peek) {
			numberText = c.text()
			for c.peek != 0 && !unicode.IsSpace(c.peek) && c.singleTokens[c.peek] == 0 {
				numericLiteral += string(c.peek)
				c.advance(1, false)
			}
			if literal, ok := c.numericLiterals[strings.ToUpper(numericLiteral)]; ok {
				numericType = c.keywords[literal]
			}
			if numericType != 0 {
				break
			} else if c.identifiersCanStartWithDigit {
				c.add(VAR)
				return
			}
			c.advance(-len([]rune(numericLiteral)), false)
			break
		} else {
			break
		}
	}

	if numberText == "" {
		numberText = c.text()
	}
	if isUnderscoreSeparated {
		numberText = strings.ReplaceAll(numberText, "_", "")
	}

	c.add(NUMBER, numberText)
	if numericType != 0 {
		c.add(DCOLON, "::")
		c.add(numericType, numericLiteral)
	}
}

func (c *TokenizerCore) scanBits() {
	c.advance(1, false)
	value := c.extractValue()
	runes := []rune(value)
	payload := ""
	if len(runes) >= 2 {
		payload = string(runes[2:])
	}
	// Upstream validates the full text with int(value, 2) (e.g. int("0b_101", 2)): CPython
	// accepts the "0b" prefix and an underscore right after it but requires at least one digit,
	// so a bare "0b" (or "0b_") falls back to an identifier. The stored payload keeps the
	// post-prefix text, so `0b_101` -> b'_101', matching upstream's BIT_STRING token.
	if _, ok := ParseIntPython(value, 2); ok {
		c.add(BIT_STRING, payload)
	} else {
		c.add(IDENTIFIER)
	}
}

func (c *TokenizerCore) scanHex() {
	c.advance(1, false)
	value := c.extractValue()
	runes := []rune(value)
	payload := ""
	if len(runes) >= 2 {
		payload = string(runes[2:])
	}
	// See scanBits: validate the full int("0x...", 16), so a bare "0x" (empty payload) falls
	// back to an identifier while "0x_FF" tokenizes (payload "_FF" -> x'_FF').
	if _, ok := ParseIntPython(value, 16); ok {
		c.add(HEX_STRING, payload)
	} else {
		c.add(IDENTIFIER)
	}
}

func (c *TokenizerCore) extractValue() string {
	for {
		char := c.peek
		if char != 0 && !unicode.IsSpace(char) && c.singleTokens[char] == 0 {
			c.advance(1, true)
		} else {
			break
		}
	}
	return c.text()
}

func (c *TokenizerCore) scanString(start string) bool {
	base := 0
	tokenType := STRING
	end := ""

	if quoteEnd, ok := c.quotes[start]; ok {
		end = quoteEnd
	} else if format, ok := c.formatStrings[start]; ok {
		end = format.End
		tokenType = format.TokenType
		if tokenType == HEX_STRING {
			base = 16
		} else if tokenType == BIT_STRING {
			base = 2
		} else if tokenType == HEREDOC_STRING {
			c.advance(1, false)
			tag := ""
			if string(c.char) != end {
				tag = c.extractString(end, nil, true, !c.heredocTagIsIdentifier)
			}
			if tag != "" && c.heredocTagIsIdentifier && (c.end || isAllDigits(tag) || containsSpace(tag)) {
				if !c.end {
					c.advance(-1, false)
				}
				c.advance(-len([]rune(tag)), false)
				c.add(c.heredocStringAlternative)
				return true
			}
			end = start + tag + end
		}
	} else {
		return false
	}

	c.advance(len([]rune(start)), false)
	escapes := c.stringEscapes
	if tokenType == BYTE_STRING {
		escapes = c.byteStringEscapes
	}
	text := c.extractString(end, escapes, tokenType == RAW_STRING, true)

	// The quoted forms validate the payload directly via int(payload, base): x'0xA'/x'+A' are
	// accepted (int() honors a leading sign and matching base prefix), x'GG' is rejected. An
	// empty x''/b'' is left as-is (upstream does not int()-validate an empty payload).
	if base != 0 && text != "" {
		if _, ok := ParseIntPython(text, base); !ok {
			panic(&sqlerrors.TokenError{Msg: fmt.Sprintf("Numeric string contains invalid characters from %d:%d", c.line, c.start)})
		}
	}

	c.add(tokenType, text)
	return true
}

func (c *TokenizerCore) scanIdentifier(identifierEnd string) {
	c.advance(1, false)
	escapes := copyRuneSet(c.identifierEscapes)
	for _, r := range identifierEnd {
		escapes[r] = true
	}
	text := c.extractString(identifierEnd, escapes, false, true)
	c.add(IDENTIFIER, text)
}

func (c *TokenizerCore) scanVar() {
	for {
		peek := c.peek
		if peek == 0 || unicode.IsSpace(peek) {
			break
		}
		if !c.varSingleTokens[peek] && c.singleTokens[peek] != 0 {
			break
		}
		c.advance(1, true)
	}

	tokenType := c.keywords[strings.ToUpper(c.text())]
	if tokenType == 0 || (len(c.tokens) > 0 && c.tokens[len(c.tokens)-1].TokenType == PARAMETER) {
		tokenType = VAR
	}
	c.add(tokenType)
}

func (c *TokenizerCore) extractString(delimiter string, escapes map[rune]bool, rawString bool, raiseUnmatched bool) string {
	text := ""
	delimRunes := []rune(delimiter)
	delimSize := len(delimRunes)
	if escapes == nil {
		escapes = c.stringEscapes
	}

	if delimSize == 1 {
		pos := c.current - 1
		end := indexRuneFrom(c.sql, delimRunes[0], pos)
		if end != -1 && (end+1 >= c.size || c.sql[end+1] != delimRunes[0] || !escapes[delimRunes[0]]) && (!(len(c.unescapedSequences) > 0 || escapes['\\']) || indexRuneFrom(c.sql[:end], '\\', pos) == -1) {
			newlines := countRune(c.sql[pos:end], '\n')
			if newlines > 0 {
				c.line += newlines
				lastNewline := lastIndexRune(c.sql[pos:end], '\n')
				c.col = end - (pos + lastNewline)
			} else {
				c.col += end - pos
			}
			c.current = end + 1
			c.end = c.current >= c.size
			c.char = c.sql[end]
			if c.end {
				c.peek = 0
			} else {
				c.peek = c.sql[c.current]
			}
			return string(c.sql[pos:end])
		}
	}

	for {
		if !rawString && len(c.unescapedSequences) > 0 && c.peek != 0 && escapes[c.char] {
			key := string([]rune{c.char, c.peek})
			if unescaped, ok := c.unescapedSequences[key]; ok {
				c.advance(2, false)
				text += unescaped
				continue
			}
		}

		isValidCustomEscape := len(c.escapeFollowChars) > 0 && c.char == '\\' && !c.escapeFollowChars[c.peek]
		if (c.stringEscapesAllowedInRawStrings || !rawString) && escapes[c.char] && (c.peek == delimRunes[0] || escapes[c.peek] || isValidCustomEscape) && (!c.isQuote(c.char) || c.char == c.peek) {
			if c.peek == delimRunes[0] {
				text += string(c.peek)
			} else if isValidCustomEscape && c.char != c.peek {
				text += string(c.peek)
			} else {
				text += string(c.char) + string(c.peek)
			}

			if c.current+1 < c.size {
				c.advance(2, false)
			} else {
				panic(&sqlerrors.TokenError{Msg: fmt.Sprintf("Missing %s from %d:%d", delimiter, c.line, c.current)})
			}
		} else {
			if c.chars(delimSize) == delimiter {
				if delimSize > 1 {
					c.advance(delimSize-1, false)
				}
				break
			}

			if c.end {
				if !raiseUnmatched {
					return text + string(c.char)
				}
				panic(&sqlerrors.TokenError{Msg: fmt.Sprintf("Missing %s from %d:%d", delimiter, c.line, c.start)})
			}

			current := c.current - 1
			c.advance(1, true)
			text += string(c.sql[current : c.current-1])
		}
	}

	return text
}

func (c *TokenizerCore) isQuote(r rune) bool {
	_, ok := c.quotes[string(r)]
	return ok
}

func upperASCII(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - ('a' - 'A')
	}
	return r
}

func isAlnum(r rune) bool {
	return r != 0 && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

func isIdentifierChar(r rune) bool {
	return r != 0 && (unicode.IsLetter(r) || r == '_')
}

func copyRuneSet(in map[rune]bool) map[rune]bool {
	out := map[rune]bool{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func indexRuneFrom(haystack []rune, needle rune, start int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(haystack); i++ {
		if haystack[i] == needle {
			return i
		}
	}
	return -1
}

func countRune(values []rune, needle rune) int {
	count := 0
	for _, value := range values {
		if value == needle {
			count++
		}
	}
	return count
}

func lastIndexRune(values []rune, needle rune) int {
	for i := len(values) - 1; i >= 0; i-- {
		if values[i] == needle {
			return i
		}
	}
	return -1
}

// ParseIntPython parses value exactly the way CPython's int(value, base) does, for the bases
// sqlglot uses to validate BIT_STRING/HEX_STRING literals (2, 8, 16). It returns the
// arbitrary-precision value (a bit/hex literal can exceed 64 bits) and whether value is a valid
// literal. CPython accepts, in order: surrounding whitespace, an optional single +/- sign, an
// optional base prefix (0b/0o/0x matching the base), then one or more base digits with optional
// single '_' separators - each underscore between two digits or immediately after the base
// prefix, never leading, trailing, or doubled (int("A_B",16) and int("0x_FF",16) are valid;
// "_A"/"A_"/"A__B"/"+_A" are not).
//
// The tokenizer validates the quoted forms (x'..'/b'..') by int(payload, base) and the bare
// forms (0x../0b..) by int(fullText, base); the generator reuses it to fold a hex/bit literal to
// its integer value on a dialect with no hex/bit family. Only the digit scan runs on invalid
// input (no big.Int allocation until the literal is known valid).
func ParseIntPython(value string, base int) (*big.Int, bool) {
	s := strings.TrimSpace(value)
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	afterPrefix := false
	if len(s) >= 2 && s[0] == '0' {
		switch lowerASCII(s[1]) {
		case 'x':
			afterPrefix = base == 16
		case 'o':
			afterPrefix = base == 8
		case 'b':
			afterPrefix = base == 2
		}
		if afterPrefix {
			s = s[2:]
		}
	}
	if s == "" {
		return nil, false
	}
	digits := make([]byte, 0, len(s))
	prevUnderscore := false
	seenDigit := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '_' {
			// A '_' is valid only between two digits or immediately after the base prefix.
			if prevUnderscore || (!seenDigit && !(i == 0 && afterPrefix)) {
				return nil, false
			}
			prevUnderscore = true
			continue
		}
		d := digitValue(ch)
		if d < 0 || d >= base {
			return nil, false
		}
		digits = append(digits, ch)
		seenDigit = true
		prevUnderscore = false
	}
	if prevUnderscore || len(digits) == 0 {
		return nil, false
	}
	n := new(big.Int)
	if _, ok := n.SetString(string(digits), base); !ok {
		return nil, false
	}
	if neg {
		n.Neg(n)
	}
	return n, true
}

// digitValue returns the value of an ASCII digit for bases up to 36, or -1 if ch is not a digit.
func digitValue(ch byte) int {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0')
	case ch >= 'a' && ch <= 'z':
		return int(ch-'a') + 10
	case ch >= 'A' && ch <= 'Z':
		return int(ch-'A') + 10
	}
	return -1
}

func lowerASCII(ch byte) byte {
	if ch >= 'A' && ch <= 'Z' {
		return ch + ('a' - 'A')
	}
	return ch
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func containsSpace(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
