package tokens

import "strings"

// decodeUnicodeEscapes decodes the SQL-standard Unicode escapes used by Postgres
// `U&'...'` string literals and `U&"..."` quoted identifiers, with the default escape
// character '\'. It resolves:
//
//	\XXXX      -> code point U+XXXX      (4 hex digits)
//	\+XXXXXX   -> code point U+XXXXXX    (6 hex digits)
//	\\         -> a literal backslash
//	a UTF-16 surrogate pair (a high surrogate immediately followed by a low surrogate, each
//	written in EITHER the 4-digit or the 6-digit form) -> the combined supplementary code point
//
// so `inf\006Frmation_schema` decodes to `information_schema` and `\D835\+00DD0D` (mixed widths)
// decodes to U+1D50D, matching PostgreSQL.
//
// It returns ok=false for any sequence PostgreSQL itself rejects, so the caller fails CLOSED
// (a parse error) rather than fabricating a value that diverges from what the server executes:
//   - a code point of 0 (NUL) or greater than U+10FFFF,
//   - an unpaired surrogate (a high surrogate not immediately followed by a low surrogate, or a
//     lone low surrogate),
//   - a backslash not introducing one of the forms above (malformed/incomplete escape).
//
// Delimiter doubling (`”` / `""`) is already resolved by extractString before this runs. The
// default escape '\' is assumed because a trailing custom `UESCAPE 'c'` clause is not consumed by
// the tokenizer, so those rare forms fail closed downstream rather than decode against the wrong
// escape character.
func decodeUnicodeEscapes(s string) (string, bool) {
	if !strings.ContainsRune(s, '\\') {
		return s, true
	}
	rs := []rune(s)
	n := len(rs)
	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < n {
		if rs[i] != '\\' {
			b.WriteRune(rs[i])
			i++
			continue
		}
		// `\\` is a literal backslash, not a code-point escape.
		if i+1 < n && rs[i+1] == '\\' {
			b.WriteRune('\\')
			i += 2
			continue
		}
		cp, next, ok := readCodePoint(rs, i)
		if !ok {
			return "", false
		}
		switch {
		case cp >= 0xD800 && cp <= 0xDBFF:
			// High surrogate: must be immediately followed by a low surrogate (either width).
			lo, next2, ok2 := readCodePoint(rs, next)
			if !ok2 || lo < 0xDC00 || lo > 0xDFFF {
				return "", false
			}
			b.WriteRune(rune(0x10000 + (cp-0xD800)*0x400 + (lo - 0xDC00)))
			i = next2
		case cp >= 0xDC00 && cp <= 0xDFFF:
			return "", false // unpaired low surrogate
		case cp == 0 || cp > 0x10FFFF:
			return "", false // NUL or out of range
		default:
			b.WriteRune(rune(cp))
			i = next
		}
	}
	return b.String(), true
}

// readCodePoint reads a single `\XXXX` or `\+XXXXXX` escape whose backslash is at rs[i], returning
// the code point and the index just past it. It returns ok=false when rs[i] is not a backslash
// introducing a hex escape (including the `\\` literal-backslash form, which the caller handles).
func readCodePoint(rs []rune, i int) (cp int, next int, ok bool) {
	if i >= len(rs) || rs[i] != '\\' {
		return 0, i, false
	}
	if i+1 < len(rs) && rs[i+1] == '+' {
		if v, hexOK := readHex(rs, i+2, 6); hexOK {
			return v, i + 8, true
		}
		return 0, i, false
	}
	if v, hexOK := readHex(rs, i+1, 4); hexOK {
		return v, i + 5, true
	}
	return 0, i, false
}

// readHex reads exactly count hex digits starting at rs[start], returning their value and whether
// all count digits were present and valid.
func readHex(rs []rune, start, count int) (int, bool) {
	if start+count > len(rs) {
		return 0, false
	}
	v := 0
	for j := range count {
		d := hexDigit(rs[start+j])
		if d < 0 {
			return 0, false
		}
		v = v*16 + d
	}
	return v, true
}

// hexDigit returns the value of a hex digit rune, or -1 if r is not [0-9A-Fa-f].
func hexDigit(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10
	}
	return -1
}
