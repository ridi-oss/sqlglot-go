package sqlglot_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	sqlglot "github.com/ridi/sqlglot-go"
	exp "github.com/ridi/sqlglot-go/expressions"
	"github.com/ridi/sqlglot-go/generator"
)

// These monotonic floors are pinned to the 95-row Python oracle. Two Postgres
// CREATE FUNCTION rows contain a nested Command in upstream itself; add new
// oracle rows by raising the applicable floor, never by lowering either one.
const (
	minFidelityCases       = 95
	minCommandFreeFidelity = 93
	// maxASTDivergences caps the rows whose Go AST legitimately differs from the
	// pinned Python repr because of a scoped, documented node deferral (see
	// ast_divergence below). This is a CEILING — it must not grow without a
	// consciously updated rationale, so the gate can't be dodged by marking rows.
	maxASTDivergences = 2
)

type fidelityCase struct {
	Dialect      string `json:"dialect"`
	UpstreamNode string `json:"upstream_node"`
	SQL          string `json:"sql"`
	Want         string `json:"want"`
	WantAST      string `json:"want_ast"`
	// CommandException documents a row whose pinned upstream tree itself contains a
	// nested Command (relaxes the no-Command-descendant assertion for that row).
	CommandException string `json:"command_exception,omitempty"`
	// ASTDivergence documents a row where Go's AST legitimately differs from the
	// pinned Python repr due to a deliberately-deferred node (e.g. MySQL date
	// functions are not wrapped in the unimplemented TsOrDsToDate node — see
	// generator/dialect_funcs.go). WantAST stays the honest Python oracle; the
	// exact ToS() match is relaxed, but root Kind, generated SQL, and Command-
	// freeness are still asserted, so the DDL structure is still fully validated.
	ASTDivergence string `json:"ast_divergence,omitempty"`
}

func loadFidelityCases(t *testing.T) []fidelityCase {
	t.Helper()
	data, err := os.ReadFile("testdata/fidelity_cases.txt")
	if err != nil {
		t.Fatalf("read fidelity fixture: %v", err)
	}

	var cases []fidelityCase
	for lineNumber, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var testCase fidelityCase
		if err := json.Unmarshal([]byte(line), &testCase); err != nil {
			t.Fatalf("parse fidelity fixture line %d: %v", lineNumber+1, err)
		}
		cases = append(cases, testCase)
	}
	return cases
}

func fidelityDialect(t *testing.T, dialect string) string {
	t.Helper()
	switch dialect {
	case "base":
		return ""
	case "mysql", "postgres":
		return dialect
	default:
		t.Fatalf("unsupported fidelity dialect %q", dialect)
		return ""
	}
}

func fidelityRootKind(t *testing.T, upstreamNode string) exp.Kind {
	t.Helper()
	switch upstreamNode {
	case "Alter":
		return exp.KindAlter
	case "Analyze":
		return exp.KindAnalyze
	case "Comment":
		return exp.KindComment
	case "Create":
		return exp.KindCreate
	default:
		t.Fatalf("unsupported upstream node %q", upstreamNode)
		return exp.KindExpression
	}
}

func TestFidelity(t *testing.T) {
	cases := loadFidelityCases(t)
	if len(cases) < minFidelityCases {
		t.Fatalf("fidelity fixture shrank: cases=%d, want >= %d", len(cases), minFidelityCases)
	}

	seen := make(map[string]struct{}, len(cases))
	commandFree := 0
	astDivergences := 0
	for index, testCase := range cases {
		key := testCase.Dialect + "\x00" + testCase.SQL
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate fidelity case at row %d: dialect=%q sql=%q", index+1, testCase.Dialect, testCase.SQL)
		}
		seen[key] = struct{}{}

		expectedCommand := strings.Contains(testCase.WantAST, "Command(")
		if testCase.CommandException != "" {
			if !expectedCommand {
				t.Fatalf("row %d has command_exception but pinned want_ast contains no Command(: %s", index+1, testCase.CommandException)
			}
		} else {
			if expectedCommand {
				t.Fatalf("row %d pinned want_ast contains Command( without a command_exception", index+1)
			}
			commandFree++
		}

		if testCase.ASTDivergence != "" {
			astDivergences++
			// A divergence row must still carry the honest Python oracle in want_ast;
			// only the exact ToS() match is relaxed, never the SQL/Command checks.
			if strings.TrimSpace(testCase.WantAST) == "" {
				t.Fatalf("row %d has ast_divergence but empty want_ast: %s", index+1, testCase.ASTDivergence)
			}
		}
	}
	if commandFree < minCommandFreeFidelity {
		t.Fatalf("command-free fidelity coverage shrank: cases=%d, want >= %d", commandFree, minCommandFreeFidelity)
	}
	if astDivergences > maxASTDivergences {
		t.Fatalf("ast_divergence rows grew to %d, want <= %d (each must be a consciously documented, evidence-backed node deferral)", astDivergences, maxASTDivergences)
	}

	for index, testCase := range cases {
		testCase := testCase
		t.Run(fmt.Sprintf("%03d_%s_%s", index+1, testCase.Dialect, testCase.UpstreamNode), func(t *testing.T) {
			dialect := fidelityDialect(t, testCase.Dialect)
			expression, err := sqlglot.ParseOne(testCase.SQL, dialect)
			if err != nil {
				t.Fatalf("ParseOne(%q): %v", testCase.SQL, err)
			}
			if wantKind := fidelityRootKind(t, testCase.UpstreamNode); expression.Kind() != wantKind {
				t.Errorf("root Kind=%d, want %d (%s)", expression.Kind(), wantKind, testCase.UpstreamNode)
			}

			gotAST := expression.ToS()
			if testCase.ASTDivergence == "" {
				if gotAST != testCase.WantAST {
					t.Errorf("ToS() mismatch\ngot:\n%s\nwant:\n%s", gotAST, testCase.WantAST)
				}
			} else if gotAST == testCase.WantAST {
				// The divergence has closed (the deferred node was implemented): drop
				// the ast_divergence marker so the row reverts to a strict oracle check.
				t.Errorf("row now matches the Python oracle exactly; remove its ast_divergence marker: %s", testCase.ASTDivergence)
			}

			command := expression.Find(exp.KindCommand)
			if testCase.CommandException == "" {
				if command != nil {
					t.Errorf("unexpected Command descendant:\n%s", command.ToS())
				}
			} else if command == nil {
				t.Errorf("validated oracle exception missing its upstream Command descendant: %s", testCase.CommandException)
			}

			got, err := sqlglot.Generate(expression, dialect, generator.Options{})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got != testCase.Want {
				t.Errorf("Generate()=%q, want %q", got, testCase.Want)
			}
		})
	}
}

type auditPythonResult struct {
	UpstreamNode     string   `json:"upstream_node"`
	TopLevelCommand  bool     `json:"top_level_command"`
	RecursiveCommand bool     `json:"recursive_command"`
	CommandPaths     []string `json:"command_paths"`
	CommandASTs      []string `json:"command_asts"`
}

type auditInput struct {
	Dialect string `json:"dialect"`
	SQL     string `json:"sql"`
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func commandPathsAndASTs(expression exp.Expression) ([]string, []string) {
	var paths, asts []string
	for _, node := range expression.Walk() {
		if node.Kind() != exp.KindCommand {
			continue
		}
		var parts []string
		for current := node; current.Parent() != nil; current = current.Parent() {
			part := current.ArgKey()
			if current.Index() >= 0 {
				part += fmt.Sprintf("[%d]", current.Index())
			}
			parts = append(parts, part)
		}
		for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
			parts[left], parts[right] = parts[right], parts[left]
		}
		paths = append(paths, strings.Join(parts, "."))
		asts = append(asts, node.ToS())
	}
	return paths, asts
}

func TestWriteFidelityAudit(t *testing.T) {
	path := os.Getenv("SQLGLOT_FIDELITY_AUDIT_PATH")
	if path == "" {
		t.Skip("set SQLGLOT_FIDELITY_AUDIT_PATH to write the audit")
	}

	records := append(loadIdentityCorpus(t), loadDialectCorpus(t)...)
	var passing []corpusRecord
	var expressions []exp.Expression
	for _, record := range records {
		expression, err := sqlglot.ParseOne(record.Sql, record.Dialect)
		if err != nil {
			continue
		}
		got, err := sqlglot.Generate(expression, record.Dialect, generator.Options{Pretty: record.Pretty})
		if err != nil || got != record.Want {
			continue
		}
		passing = append(passing, record)
		expressions = append(expressions, expression)
	}

	inputs := make([]auditInput, len(passing))
	for i, record := range passing {
		inputs[i] = auditInput{Dialect: record.Dialect, SQL: record.Sql}
	}
	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}
	python := `
import json, sys
import sqlglot
from sqlglot import exp

def commands_with_paths(root):
    found = []
    def visit(node, path):
        if isinstance(node, exp.Command):
            found.append((path, repr(node)))
        for key, value in node.args.items():
            if isinstance(value, exp.Expression):
                visit(value, path + [key])
            elif isinstance(value, list):
                for index, item in enumerate(value):
                    if isinstance(item, exp.Expression):
                        visit(item, path + [f"{key}[{index}]"])
    visit(root, [])
    return found

out = []
for row in json.load(sys.stdin):
    dialect = None if row["dialect"] == "" else row["dialect"]
    expression = sqlglot.parse_one(row["sql"], read=dialect)
    commands = commands_with_paths(expression)
    out.append({
        "upstream_node": expression.__class__.__name__,
        "top_level_command": isinstance(expression, exp.Command),
        "recursive_command": bool(commands),
        "command_paths": [".".join(path) for path, _ in commands],
        "command_asts": [ast for _, ast in commands],
    })
json.dump(out, sys.stdout, ensure_ascii=False, separators=(",", ":"))
`
	cmd := exec.Command("python3", "-c", python)
	cmd.Env = append(os.Environ(), "PYTHONPATH=.reference/sqlglot-v30.12.0")
	cmd.Stdin = bytes.NewReader(inputJSON)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run Python oracle: %v", err)
	}
	var pythonResults []auditPythonResult
	if err := json.Unmarshal(output, &pythonResults); err != nil {
		t.Fatalf("decode Python oracle: %v", err)
	}
	if len(pythonResults) != len(passing) {
		t.Fatalf("Python results=%d, passing records=%d", len(pythonResults), len(passing))
	}

	var contents strings.Builder
	encoder := json.NewEncoder(&contents)
	encoder.SetEscapeHTML(false)
	summary := map[string]any{
		"type":                      "summary",
		"corpus_records":            len(records),
		"passing_records":           len(passing),
		"go_top_level_commands":     0,
		"go_recursive_commands":     0,
		"python_top_level_commands": 0,
		"python_recursive_commands": 0,
		"go_only_commands":          0,
		"python_only_commands":      0,
		"command_path_mismatches":   0,
	}
	for i, record := range passing {
		paths, asts := commandPathsAndASTs(expressions[i])
		pythonResult := pythonResults[i]
		goTop := expressions[i].Kind() == exp.KindCommand
		goRecursive := expressions[i].Find(exp.KindCommand) != nil
		row := map[string]any{
			"type":                     "case",
			"dialect":                  record.Dialect,
			"sql":                      record.Sql,
			"want":                     record.Want,
			"go_top_level_command":     goTop,
			"go_recursive_command":     goRecursive,
			"go_command_paths":         paths,
			"go_command_asts":          asts,
			"python_upstream_node":     pythonResult.UpstreamNode,
			"python_top_level_command": pythonResult.TopLevelCommand,
			"python_recursive_command": pythonResult.RecursiveCommand,
			"python_command_paths":     pythonResult.CommandPaths,
			"python_command_asts":      pythonResult.CommandASTs,
		}
		if goTop {
			summary["go_top_level_commands"] = summary["go_top_level_commands"].(int) + 1
		}
		if goRecursive {
			summary["go_recursive_commands"] = summary["go_recursive_commands"].(int) + 1
		}
		if pythonResult.TopLevelCommand {
			summary["python_top_level_commands"] = summary["python_top_level_commands"].(int) + 1
		}
		if pythonResult.RecursiveCommand {
			summary["python_recursive_commands"] = summary["python_recursive_commands"].(int) + 1
		}
		if goRecursive && !pythonResult.RecursiveCommand {
			summary["go_only_commands"] = summary["go_only_commands"].(int) + 1
		}
		if !goRecursive && pythonResult.RecursiveCommand {
			summary["python_only_commands"] = summary["python_only_commands"].(int) + 1
		}
		if !equalStrings(paths, pythonResult.CommandPaths) {
			summary["command_path_mismatches"] = summary["command_path_mismatches"].(int) + 1
		}
		if err := encoder.Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := encoder.Encode(summary); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d passing records and summary to %s: %+v", len(passing), path, summary)
}
