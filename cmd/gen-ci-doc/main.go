// Command gen-ci-doc parses .github/workflows/*.yml and
// .github/rulesets/main.json, then generates wiki/GitHub CI.md from
// wiki/tmpl/GitHub CI.md.tmpl.
//
// Run from the repository root:
//
//	go run ./cmd/gen-ci-doc
//
// The command exits 1 if the output file was out of date and has been
// regenerated. This matches the behaviour of gen-metrics-doc so both can be
// wired into the same pre-commit hook.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// WorkflowFile holds the parsed metadata for a single GitHub Actions workflow.
type WorkflowFile struct {
	FileName string        // e.g. "ci.yml"
	Name     string        // workflow `name:` field
	Trigger  string        // human-readable summary of the `on:` field
	Jobs     []WorkflowJob // jobs in declaration order
}

// WorkflowJob holds parsed metadata for a single job within a workflow.
type WorkflowJob struct {
	ID    string         // job key in the YAML jobs map
	Name  string         // job `name:` field (may contain template expressions)
	Needs []string       // values of the `needs:` field
	Steps []WorkflowStep // steps in declaration order
}

// WorkflowStep holds parsed metadata for a single step within a job.
type WorkflowStep struct {
	Name string // step `name:` field
	Uses string // step `uses:` field (action reference)
	Run  string // first non-empty line of the `run:` field
}

// BranchProtection holds the relevant fields parsed from the GitHub ruleset JSON.
type BranchProtection struct {
	Rules          []BranchRule
	RequiredChecks []string // status-check contexts, in declaration order
}

// BranchRule is a single parsed ruleset rule with a human-readable description.
type BranchRule struct {
	Rule    string
	Setting string
}

type templateData struct {
	WorkflowFiles    []WorkflowFile
	CIGraph          string // pre-rendered execution graph (markdown)
	ReleaseGraph     string // pre-rendered execution graph (markdown)
	BranchProtection BranchProtection
}

// errOutdated is returned by run when the output file was regenerated.
var errOutdated = errors.New("output was regenerated")

func main() {
	err := run(
		".github/workflows",
		".github/rulesets/main.json",
		"wiki/tmpl/GitHub CI.md.tmpl",
		"wiki/GitHub CI.md",
	)
	switch {
	case errors.Is(err, errOutdated):
		fmt.Fprintln(os.Stderr, "wiki/GitHub CI.md was outdated and has been regenerated.")
		fmt.Fprintln(os.Stderr, "Please stage the changes and commit again.")
		os.Exit(1)
	case err != nil:
		log.Fatal(err)
	}
}

func run(workflowDir, rulesetFile, tmplFile, outFile string) error {
	data, err := buildTemplateData(workflowDir, rulesetFile)
	if err != nil {
		return err
	}

	tmplBytes, err := os.ReadFile(tmplFile)
	if err != nil {
		return fmt.Errorf("read template %s: %w", tmplFile, err)
	}

	tmpl, err := template.New("ci").Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	generated := normalizeNewlines(buf.Bytes())

	existing, _ := os.ReadFile(outFile)
	fmt.Printf("%s\n\n\n", generated)
	fmt.Printf("%s\n\n", existing)
	if bytes.Equal(existing, generated) {
		fmt.Printf("%s is up to date.\n", outFile)
		return nil
	}

	if err := os.WriteFile(outFile, generated, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outFile, err)
	}
	return errOutdated
}

func buildTemplateData(workflowDir, rulesetFile string) (templateData, error) {
	wfs, err := parseWorkflowDir(workflowDir)
	if err != nil {
		return templateData{}, err
	}

	var ciJobs, releaseJobs []WorkflowJob
	for _, wf := range wfs {
		switch wf.FileName {
		case "ci.yml":
			ciJobs = wf.Jobs
		case "release.yml":
			releaseJobs = wf.Jobs
		}
	}

	bp, err := parseRuleset(rulesetFile)
	if err != nil {
		return templateData{}, err
	}

	return templateData{
		WorkflowFiles:    wfs,
		CIGraph:          renderMermaidGraph(ciJobs),
		ReleaseGraph:     renderMermaidGraph(releaseJobs),
		BranchProtection: bp,
	}, nil
}

// parseWorkflowDir reads all *.yml files from dir, returning orchestrators
// (ci.yml, release.yml) first, then reusable (_*.yml) alphabetically.
func parseWorkflowDir(dir string) ([]WorkflowFile, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", dir, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := filepath.Base(entries[i]), filepath.Base(entries[j])
		wa, wb := workflowSortWeight(a), workflowSortWeight(b)
		if wa != wb {
			return wa < wb
		}
		return a < b
	})

	result := make([]WorkflowFile, 0, len(entries))
	for _, path := range entries {
		wf, err := parseWorkflowFile(path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		result = append(result, wf)
	}
	return result, nil
}

// workflowSortWeight returns a sort key so orchestrators appear before reusable
// workflows in the table.
func workflowSortWeight(name string) int {
	switch name {
	case "ci.yml":
		return 0
	case "release.yml":
		return 1
	default:
		return 2
	}
}

// parseWorkflowFile decodes a GitHub Actions YAML workflow file into a
// WorkflowFile. It uses yaml.v3 Node decoding to capture head comments so that
// "# @ description:" annotations are extracted alongside the parsed fields.
func parseWorkflowFile(path string) (WorkflowFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowFile{}, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return WorkflowFile{}, fmt.Errorf("unmarshal: %w", err)
	}
	if len(doc.Content) == 0 {
		return WorkflowFile{FileName: filepath.Base(path)}, nil
	}

	root := doc.Content[0] // DocumentNode → root MappingNode
	wf := WorkflowFile{FileName: filepath.Base(path)}

	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		val := root.Content[i+1]
		switch key.Value {
		case "name":
			wf.Name = val.Value
		case "on":
			wf.Trigger = parseTrigger(val)
		case "jobs":
			wf.Jobs = parseJobs(val)
		}
	}
	return wf, nil
}

// parseTrigger converts a YAML on: node to a compact, human-readable string.
func parseTrigger(node *yaml.Node) string {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value
	case yaml.SequenceNode:
		parts := make([]string, 0, len(node.Content))
		for _, n := range node.Content {
			parts = append(parts, n.Value)
		}
		return strings.Join(parts, ", ")
	case yaml.MappingNode:
		parts := make([]string, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			event := node.Content[i].Value
			qualifier := branchQualifier(node.Content[i+1])
			if qualifier != "" {
				parts = append(parts, event+" "+qualifier)
			} else {
				parts = append(parts, event)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// branchQualifier extracts a compact branch-filter annotation from a trigger
// sub-mapping, e.g. "(branches: main)" or "(branches-ignore: main)".
func branchQualifier(node *yaml.Node) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		v := node.Content[i+1]
		switch k {
		case "branches":
			return "(branches: " + yamlSeqJoin(v) + ")"
		case "branches-ignore":
			return "(branches-ignore: " + yamlSeqJoin(v) + ")"
		}
	}
	return ""
}

func yamlSeqJoin(node *yaml.Node) string {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value
	case yaml.SequenceNode:
		parts := make([]string, 0, len(node.Content))
		for _, n := range node.Content {
			parts = append(parts, n.Value)
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// parseJobs parses the jobs: mapping node into an ordered slice of WorkflowJob.
func parseJobs(node *yaml.Node) []WorkflowJob {
	jobs := make([]WorkflowJob, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		jobKey := node.Content[i]
		jobDef := node.Content[i+1]
		job := WorkflowJob{
			ID: jobKey.Value,
		}
		for j := 0; j+1 < len(jobDef.Content); j += 2 {
			k := jobDef.Content[j]
			v := jobDef.Content[j+1]
			switch k.Value {
			case "name":
				job.Name = v.Value
			case "needs":
				job.Needs = parseNeeds(v)
			case "steps":
				job.Steps = parseSteps(v)
			}
		}
		jobs = append(jobs, job)
	}
	return jobs
}

// parseNeeds converts a needs: node (scalar or sequence) into a string slice.
func parseNeeds(node *yaml.Node) []string {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Value == "" {
			return nil
		}
		return []string{node.Value}
	case yaml.SequenceNode:
		needs := make([]string, 0, len(node.Content))
		for _, n := range node.Content {
			needs = append(needs, n.Value)
		}
		return needs
	}
	return nil
}

// parseSteps converts a steps: sequence node into an ordered slice of WorkflowStep.
func parseSteps(node *yaml.Node) []WorkflowStep {
	steps := make([]WorkflowStep, 0, len(node.Content))
	for _, item := range node.Content {
		step := WorkflowStep{}
		for j := 0; j+1 < len(item.Content); j += 2 {
			k := item.Content[j]
			v := item.Content[j+1]
			switch k.Value {
			case "name":
				step.Name = v.Value
			case "uses":
				step.Uses = v.Value
			case "run":
				step.Run = firstLine(v.Value)
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// mermaidID converts a job ID into a valid Mermaid node identifier by
// replacing hyphens with underscores.
func mermaidID(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}

// renderMermaidGraph renders a fenced Mermaid flowchart from job dependency
// edges. Each job becomes a node labelled with its name (or ID if no name is
// set); needs: relationships become directed edges.
func renderMermaidGraph(jobs []WorkflowJob) string {
	if len(jobs) == 0 {
		return "_no jobs found_\n"
	}

	jobSet := make(map[string]bool, len(jobs))
	for _, j := range jobs {
		jobSet[j.ID] = true
	}

	var sb strings.Builder
	sb.WriteString("```mermaid\ngraph TD\n")

	for _, j := range jobs {
		label := j.ID
		if j.Name != "" {
			label = j.Name
		}
		fmt.Fprintf(&sb, "    %s[\"%s\"]\n", mermaidID(j.ID), label)
	}

	for _, j := range jobs {
		for _, dep := range j.Needs {
			if jobSet[dep] {
				fmt.Fprintf(&sb, "    %s --> %s\n", mermaidID(dep), mermaidID(j.ID))
			}
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}

// --- Ruleset parsing ---

type rulesetJSON struct {
	BypassActors []struct {
		ActorID    int    `json:"actor_id"`
		ActorType  string `json:"actor_type"`
		BypassMode string `json:"bypass_mode"`
	} `json:"bypass_actors"`
	Rules []struct {
		Type       string `json:"type"`
		Parameters *struct {
			DismissStaleReviewsOnPush      bool `json:"dismiss_stale_reviews_on_push"`
			RequireLastPushApproval        bool `json:"require_last_push_approval"`
			RequiredApprovingReviewCount   int  `json:"required_approving_review_count"`
			RequiredReviewThreadResolution bool `json:"required_review_thread_resolution"`
			RequiredStatusChecks           []struct {
				Context string `json:"context"`
			} `json:"required_status_checks"`
		} `json:"parameters,omitempty"`
	} `json:"rules"`
}

func parseRuleset(path string) (BranchProtection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return BranchProtection{}, fmt.Errorf("read ruleset %s: %w", path, err)
	}
	var rs rulesetJSON
	if err := json.Unmarshal(data, &rs); err != nil {
		return BranchProtection{}, fmt.Errorf("unmarshal ruleset: %w", err)
	}

	var bp BranchProtection
	for _, r := range rs.Rules {
		switch r.Type {
		case "deletion":
			bp.Rules = append(bp.Rules, BranchRule{"Deletion", "Blocked"})
		case "non_fast_forward":
			bp.Rules = append(bp.Rules, BranchRule{"Force push", "Blocked"})
		case "required_linear_history":
			bp.Rules = append(bp.Rules, BranchRule{"Required linear history", "Yes"})
		case "pull_request":
			if p := r.Parameters; p != nil {
				bp.Rules = append(bp.Rules, BranchRule{
					"Required approving reviews",
					fmt.Sprintf("%d", p.RequiredApprovingReviewCount),
				})
				bp.Rules = append(bp.Rules, BranchRule{
					"Dismiss stale reviews on push",
					boolYesNo(p.DismissStaleReviewsOnPush),
				})
				bp.Rules = append(bp.Rules, BranchRule{
					"Require last-push approval",
					boolYesNo(p.RequireLastPushApproval),
				})
				bp.Rules = append(bp.Rules, BranchRule{
					"Required conversation resolution",
					boolYesNo(p.RequiredReviewThreadResolution),
				})
			}
		case "required_status_checks":
			if p := r.Parameters; p != nil {
				for _, sc := range p.RequiredStatusChecks {
					bp.RequiredChecks = append(bp.RequiredChecks, sc.Context)
				}
			}
		}
	}
	return bp, nil
}

func boolYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// normalizeNewlines collapses 3+ consecutive newlines to 2 and ensures the
// output ends with exactly one newline.
var multiBlank = regexp.MustCompile(`\n{3,}`)

func normalizeNewlines(b []byte) []byte {
	b = multiBlank.ReplaceAll(b, []byte("\n\n"))
	b = bytes.TrimRight(b, "\n")
	return append(b, '\n')
}
