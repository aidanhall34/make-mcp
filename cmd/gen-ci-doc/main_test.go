package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- parseTrigger ---

func TestParseTrigger_Scalar(t *testing.T) {
	src := "on: push\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	root := doc.Content[0]
	// root.Content[0] = "on" key, root.Content[1] = value node
	got := parseTrigger(root.Content[1])
	if got != "push" {
		t.Errorf("got %q, want %q", got, "push")
	}
}

func TestParseTrigger_Sequence(t *testing.T) {
	src := "on: [push, pull_request]\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	root := doc.Content[0]
	got := parseTrigger(root.Content[1])
	if got != "push, pull_request" {
		t.Errorf("got %q, want %q", got, "push, pull_request")
	}
}

func TestParseTrigger_MappingWithBranchesIgnore(t *testing.T) {
	src := "on:\n  push:\n    branches-ignore:\n      - main\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	root := doc.Content[0]
	got := parseTrigger(root.Content[1])
	if got != "push (branches-ignore: main)" {
		t.Errorf("got %q, want %q", got, "push (branches-ignore: main)")
	}
}

func TestParseTrigger_WorkflowCall(t *testing.T) {
	src := "on:\n  workflow_call:\n    secrets:\n      FOO:\n        required: false\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	root := doc.Content[0]
	got := parseTrigger(root.Content[1])
	if got != "workflow_call" {
		t.Errorf("got %q, want %q", got, "workflow_call")
	}
}

// --- parseNeeds ---

func TestParseNeeds_Scalar(t *testing.T) {
	src := "needs: build\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	root := doc.Content[0]
	got := parseNeeds(root.Content[1])
	if len(got) != 1 || got[0] != "build" {
		t.Errorf("got %v, want [build]", got)
	}
}

func TestParseNeeds_Sequence(t *testing.T) {
	src := "needs: [lint, test]\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	root := doc.Content[0]
	got := parseNeeds(root.Content[1])
	if len(got) != 2 || got[0] != "lint" || got[1] != "test" {
		t.Errorf("got %v, want [lint test]", got)
	}
}

// --- renderMermaidGraph ---

func TestRenderMermaidGraph_Empty(t *testing.T) {
	got := renderMermaidGraph(nil)
	if !strings.Contains(got, "no jobs") {
		t.Errorf("expected 'no jobs' message, got %q", got)
	}
}

func TestRenderMermaidGraph_SingleJob(t *testing.T) {
	jobs := []WorkflowJob{{ID: "build"}}
	got := renderMermaidGraph(jobs)
	if !strings.Contains(got, "```mermaid") {
		t.Errorf("missing mermaid fence in %q", got)
	}
	if !strings.Contains(got, "build") {
		t.Errorf("missing build node in %q", got)
	}
	if strings.Contains(got, "-->") {
		t.Errorf("single job should have no edges, got %q", got)
	}
}

func TestRenderMermaidGraph_WithDependencies(t *testing.T) {
	jobs := []WorkflowJob{
		{ID: "setup"},
		{ID: "lint", Needs: []string{"setup"}},
		{ID: "test", Needs: []string{"setup"}},
		{ID: "deploy", Needs: []string{"lint", "test"}},
	}
	got := renderMermaidGraph(jobs)
	if !strings.Contains(got, "setup --> lint") {
		t.Errorf("missing setup->lint edge in %q", got)
	}
	if !strings.Contains(got, "setup --> test") {
		t.Errorf("missing setup->test edge in %q", got)
	}
	if !strings.Contains(got, "lint --> deploy") {
		t.Errorf("missing lint->deploy edge in %q", got)
	}
	if !strings.Contains(got, "test --> deploy") {
		t.Errorf("missing test->deploy edge in %q", got)
	}
}

func TestRenderMermaidGraph_IgnoresExternalDeps(t *testing.T) {
	// "external-dep" is not in the jobs list; renderMermaidGraph should ignore it.
	jobs := []WorkflowJob{
		{ID: "a", Needs: []string{"external-dep"}},
	}
	got := renderMermaidGraph(jobs)
	if strings.Contains(got, "external") {
		t.Errorf("external dep should be ignored, got %q", got)
	}
}

func TestRenderMermaidGraph_HyphenatedIDs(t *testing.T) {
	jobs := []WorkflowJob{
		{ID: "detect-changes"},
		{ID: "build-image", Needs: []string{"detect-changes"}},
	}
	got := renderMermaidGraph(jobs)
	if !strings.Contains(got, "detect_changes") {
		t.Errorf("hyphens should be replaced with underscores in node IDs, got %q", got)
	}
	if !strings.Contains(got, "detect_changes --> build_image") {
		t.Errorf("missing edge with sanitized IDs in %q", got)
	}
}

// --- firstLine ---

func TestFirstLine_Simple(t *testing.T) {
	if got := firstLine("echo hello"); got != "echo hello" {
		t.Errorf("got %q", got)
	}
}

func TestFirstLine_Multiline(t *testing.T) {
	if got := firstLine("\necho hello\necho world"); got != "echo hello" {
		t.Errorf("got %q", got)
	}
}

func TestFirstLine_Empty(t *testing.T) {
	if got := firstLine(""); got != "" {
		t.Errorf("got %q", got)
	}
}

// --- workflowSortWeight ---

func TestWorkflowSortWeight(t *testing.T) {
	if workflowSortWeight("ci.yml") >= workflowSortWeight("release.yml") {
		t.Error("ci.yml should sort before release.yml")
	}
	if workflowSortWeight("release.yml") >= workflowSortWeight("_lint.yml") {
		t.Error("release.yml should sort before _lint.yml")
	}
}

// --- normalizeNewlines ---

func TestNormalizeNewlines(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello\n", "hello\n"},
		{"hello\n\n\n", "hello\n"},
		{"a\n\n\n\nb", "a\n\nb\n"},
		{"hello", "hello\n"},
	}
	for _, tc := range tests {
		got := string(normalizeNewlines([]byte(tc.in)))
		if got != tc.want {
			t.Errorf("normalizeNewlines(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- parseRuleset ---

func TestParseRuleset_MissingFile(t *testing.T) {
	_, err := parseRuleset(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseRuleset_InvalidJSON(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(f, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := parseRuleset(f)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseRuleset_BasicRules(t *testing.T) {
	src := `{
		"rules": [
			{"type": "deletion"},
			{"type": "non_fast_forward"},
			{"type": "required_linear_history"},
			{"type": "pull_request", "parameters": {
				"required_approving_review_count": 2,
				"dismiss_stale_reviews_on_push": true,
				"require_last_push_approval": false,
				"required_review_thread_resolution": true
			}},
			{"type": "required_status_checks", "parameters": {
				"required_status_checks": [
					{"context": "lint / lint"},
					{"context": "test / test"}
				]
			}}
		]
	}`
	f := filepath.Join(t.TempDir(), "ruleset.json")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	bp, err := parseRuleset(f)
	if err != nil {
		t.Fatalf("parseRuleset: %v", err)
	}
	if len(bp.RequiredChecks) != 2 {
		t.Errorf("required checks = %v, want 2", bp.RequiredChecks)
	}
	if bp.RequiredChecks[0] != "lint / lint" {
		t.Errorf("check 0 = %q", bp.RequiredChecks[0])
	}
	// Should have deletion + force-push + linear-history + 4 PR sub-rules
	if len(bp.Rules) != 7 {
		t.Errorf("rules count = %d, want 7: %v", len(bp.Rules), bp.Rules)
	}
}

// --- integration tests against the real repository files ---

func TestParseWorkflowDir_RealFiles(t *testing.T) {
	wfs, err := parseWorkflowDir("../../.github/workflows")
	if err != nil {
		t.Fatalf("parseWorkflowDir: %v", err)
	}
	if len(wfs) == 0 {
		t.Fatal("no workflow files parsed")
	}

	// ci.yml and release.yml must be present and sorted first.
	if wfs[0].FileName != "ci.yml" {
		t.Errorf("first workflow = %q, want ci.yml", wfs[0].FileName)
	}
	if wfs[1].FileName != "release.yml" {
		t.Errorf("second workflow = %q, want release.yml", wfs[1].FileName)
	}

	// Every workflow must have a non-empty trigger.
	for _, wf := range wfs {
		if wf.Trigger == "" {
			t.Errorf("workflow %s has empty trigger", wf.FileName)
		}
	}
}

func TestRenderMermaidGraph_RealCIWorkflow(t *testing.T) {
	wfs, err := parseWorkflowDir("../../.github/workflows")
	if err != nil {
		t.Fatalf("parseWorkflowDir: %v", err)
	}
	var ciJobs []WorkflowJob
	for _, wf := range wfs {
		if wf.FileName == "ci.yml" {
			ciJobs = wf.Jobs
			break
		}
	}
	if len(ciJobs) == 0 {
		t.Fatal("no jobs found in ci.yml")
	}
	got := renderMermaidGraph(ciJobs)
	if !strings.Contains(got, "```mermaid") {
		t.Errorf("missing mermaid fence in output: %s", got)
	}
	// detect-changes is a root node and must appear in the graph.
	if !strings.Contains(got, "detect_changes") {
		t.Errorf("detect-changes not found in graph: %s", got)
	}
}

func TestParseRuleset_RealFile(t *testing.T) {
	bp, err := parseRuleset("../../.github/rulesets/main.json")
	if err != nil {
		t.Fatalf("parseRuleset: %v", err)
	}
	if len(bp.RequiredChecks) == 0 {
		t.Error("no required checks parsed from real ruleset")
	}
	if len(bp.Rules) == 0 {
		t.Error("no rules parsed from real ruleset")
	}
}

// --- run() ---

func TestRun_GeneratesOutdatedFile(t *testing.T) {
	dir := t.TempDir()

	workflowsDir := filepath.Join(dir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ciYML := `name: CI
on:
  push:
    branches-ignore:
      - main
jobs:
  build:
    name: build
    steps:
      - name: Check out
        uses: actions/checkout@v4
`
	if err := os.WriteFile(filepath.Join(workflowsDir, "ci.yml"), []byte(ciYML), 0o644); err != nil {
		t.Fatal(err)
	}

	rulesetJSON := `{"rules": [{"type": "deletion"}]}`
	rulesetFile := filepath.Join(dir, "main.json")
	if err := os.WriteFile(rulesetFile, []byte(rulesetJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	tmplSrc := "# CI\n{{range .WorkflowFiles}}- {{.FileName}}\n{{end}}\n"
	tmplFile := filepath.Join(dir, "GitHub CI.md.tmpl")
	if err := os.WriteFile(tmplFile, []byte(tmplSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(dir, "GitHub CI.md")

	t.Run("generates outdated", func(t *testing.T) {
		err := run(workflowsDir, rulesetFile, tmplFile, outFile)
		if !errors.Is(err, errOutdated) {
			t.Fatalf("expected errOutdated, got %v", err)
		}
		content, _ := os.ReadFile(outFile)
		if !strings.Contains(string(content), "ci.yml") {
			t.Errorf("output missing ci.yml: %s", content)
		}
	})

	t.Run("up to date returns nil", func(t *testing.T) {
		if err := run(workflowsDir, rulesetFile, tmplFile, outFile); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("missing template returns error", func(t *testing.T) {
		err := run(workflowsDir, rulesetFile, filepath.Join(dir, "no.tmpl"), outFile)
		if err == nil {
			t.Error("expected error for missing template")
		}
	})

	t.Run("invalid template returns error", func(t *testing.T) {
		bad := filepath.Join(dir, "bad.tmpl")
		if err := os.WriteFile(bad, []byte("{{.Missing}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := run(workflowsDir, rulesetFile, bad, outFile); err == nil {
			t.Error("expected error for invalid template")
		}
	})
}
