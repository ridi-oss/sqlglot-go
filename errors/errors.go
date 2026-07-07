package errors

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ansiUnderline = "\033[4m"
	ansiReset     = "\033[0m"

	ErrorMessageContextDefault = 100
)

type ErrorLevel int

const (
	IGNORE ErrorLevel = iota
	WARN
	RAISE
	IMMEDIATE
)

func (e ErrorLevel) String() string {
	switch e {
	case IGNORE:
		return "IGNORE"
	case WARN:
		return "WARN"
	case RAISE:
		return "RAISE"
	case IMMEDIATE:
		return "IMMEDIATE"
	default:
		return fmt.Sprintf("ErrorLevel(%d)", int(e))
	}
}

type SqlglotError struct{ Msg string }

func (e *SqlglotError) Error() string { return e.Msg }

type UnsupportedError struct{ Msg string }

func (e *UnsupportedError) Error() string { return e.Msg }

type OptimizeError struct{ Msg string }

func (e *OptimizeError) Error() string { return e.Msg }

type SchemaError struct{ Msg string }

func (e *SchemaError) Error() string { return e.Msg }

func NewSchemaError(format string, args ...any) *SchemaError {
	return &SchemaError{Msg: fmt.Sprintf(format, args...)}
}

type ExecuteError struct{ Msg string }

func (e *ExecuteError) Error() string { return e.Msg }

type TokenError struct{ Msg string }

func (e *TokenError) Error() string { return e.Msg }

type ParseError struct {
	Msg    string
	Errors []map[string]any
}

func (e *ParseError) Error() string { return e.Msg }

func NewParseError(message string, fields ...map[string]any) *ParseError {
	if len(fields) > 0 {
		return &ParseError{Msg: message, Errors: fields}
	}
	return &ParseError{
		Msg: message,
		Errors: []map[string]any{{
			"description":     nil,
			"line":            nil,
			"col":             nil,
			"start_context":   nil,
			"highlight":       nil,
			"end_context":     nil,
			"into_expression": nil,
		}},
	}
}

func NewParseErrorWithLocation(message, description string, line, col int, startContext, highlight, endContext string) *ParseError {
	return &ParseError{
		Msg: message,
		Errors: []map[string]any{{
			"description":     description,
			"line":            line,
			"col":             col,
			"start_context":   startContext,
			"highlight":       highlight,
			"end_context":     endContext,
			"into_expression": nil,
		}},
	}
}

type Position struct {
	Start int
	End   int
}

func HighlightSQL(sql string, positions [][2]int, ctxLen int) (formatted, startContext, highlight, endContext string) {
	if len(positions) == 0 {
		panic("positions must contain at least one (start, end) tuple")
	}

	sortedPositions := append([][2]int(nil), positions...)
	sort.Slice(sortedPositions, func(i, j int) bool { return sortedPositions[i][0] < sortedPositions[j][0] })

	runes := []rune(sql)
	firstHighlightStart := 0
	previousPartEnd := 0
	parts := make([]string, 0, len(sortedPositions)*3)

	if sortedPositions[0][0] > 0 {
		firstHighlightStart = sortedPositions[0][0]
		start := maxInt(0, firstHighlightStart-ctxLen)
		startContext = string(runes[start:firstHighlightStart])
		parts = append(parts, startContext)
		previousPartEnd = firstHighlightStart
	}

	for _, pos := range sortedPositions {
		start := clamp(pos[0], 0, len(runes))
		end := clamp(pos[1]+1, 0, len(runes))
		highlightStart := maxInt(start, previousPartEnd)
		highlightEnd := end
		if highlightStart >= highlightEnd {
			continue
		}
		if highlightStart > previousPartEnd {
			parts = append(parts, string(runes[previousPartEnd:highlightStart]))
		}
		parts = append(parts, ansiUnderline, string(runes[highlightStart:highlightEnd]), ansiReset)
		previousPartEnd = highlightEnd
	}

	if previousPartEnd < len(runes) {
		end := minInt(previousPartEnd+ctxLen, len(runes))
		endContext = string(runes[previousPartEnd:end])
		parts = append(parts, endContext)
	}

	formatted = strings.Join(parts, "")
	highlight = string(runes[firstHighlightStart:previousPartEnd])
	return formatted, startContext, highlight, endContext
}

func ConcatMessages(errs []error, maximum int) string {
	limit := minInt(len(errs), maximum)
	messages := make([]string, 0, limit+1)
	for _, err := range errs[:limit] {
		messages = append(messages, err.Error())
	}
	remaining := len(errs) - maximum
	if remaining > 0 {
		messages = append(messages, fmt.Sprintf("... and %d more", remaining))
	}
	return strings.Join(messages, "\n\n")
}

func MergeErrors(errs []*ParseError) []map[string]any {
	var merged []map[string]any
	for _, err := range errs {
		merged = append(merged, err.Errors...)
	}
	return merged
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
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
