// Command gen-metrics-doc parses pkg/telemetry/metrics.go using the Go AST and
// generates docs/metrics.md from docs/metrics.md.tmpl.
//
// Run from the repository root:
//
//	go run ./cmd/gen-metrics-doc
//
// The command exits 1 if the output file was out of date and has been
// regenerated. This matches the behaviour of scripts/gen-wiki-sidebar.sh so
// it can be wired into the pre-commit hook.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

// MetricType is the instrument type string used in documentation.
type MetricType string

const (
	TypeCounter   MetricType = "Counter"
	TypeHistogram MetricType = "Histogram"
	TypeGauge     MetricType = "Gauge"
)

// MetricDef holds documentation metadata for a single metric instrument.
type MetricDef struct {
	Name        string
	Type        MetricType
	Description string
	Unit        string
	Labels      []string
}

// DescriptionOrDash returns the description, or an em-dash when absent.
func (m MetricDef) DescriptionOrDash() string {
	if m.Description == "" {
		return "—"
	}
	return m.Description
}

// UnitOrDash returns the unit string, or an em-dash for unitless metrics.
func (m MetricDef) UnitOrDash() string {
	if m.Unit == "" {
		return "—"
	}
	return m.Unit
}

// LabelsOrNone returns a comma-separated, backtick-wrapped list of label keys,
// or the italicised string "(none)" when there are no labels.
func (m MetricDef) LabelsOrNone() string {
	if len(m.Labels) == 0 {
		return "_(none)_"
	}
	parts := make([]string, len(m.Labels))
	for i, l := range m.Labels {
		parts[i] = "`" + l + "`"
	}
	return strings.Join(parts, ", ")
}

type templateData struct {
	Counters   []MetricDef
	Histograms []MetricDef
	Gauges     []MetricDef
}

// errOutdated is returned by run when the output file was regenerated.
var errOutdated = errors.New("output was regenerated")

func main() {
	err := run(
		"pkg/telemetry/metrics.go",
		"cmd/mcp-server/main.go",
		"docs/metrics.md.tmpl",
		"wiki/Metrics.md",
	)
	switch {
	case errors.Is(err, errOutdated):
		fmt.Fprintln(os.Stderr, "wiki/Metrics.md was outdated and has been regenerated.")
		fmt.Fprintln(os.Stderr, "Please stage the changes and commit again.")
		os.Exit(1)
	case err != nil:
		log.Fatal(err)
	}
}

func run(metricsFile, extraFile, tmplFile, outFile string) error {
	data, err := buildTemplateData(metricsFile, extraFile)
	if err != nil {
		return err
	}

	tmplBytes, err := os.ReadFile(tmplFile)
	if err != nil {
		return fmt.Errorf("read template %s: %w", tmplFile, err)
	}

	tmpl, err := template.New("metrics").Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	generated := normalizeNewlines(buf.Bytes())

	existing, _ := os.ReadFile(outFile)
	if bytes.Equal(existing, generated) {
		fmt.Printf("%s is up to date.\n", outFile)
		return nil
	}

	if err := os.MkdirAll("docs", 0o755); err != nil {
		return fmt.Errorf("create docs dir: %w", err)
	}
	if err := os.WriteFile(outFile, generated, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outFile, err)
	}
	return errOutdated
}

// buildTemplateData parses the given Go source files and returns structured
// metric data ready for template rendering.
func buildTemplateData(metricsFile, extraFile string) (templateData, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, metricsFile, nil, 0)
	if err != nil {
		return templateData{}, fmt.Errorf("parse %s: %w", metricsFile, err)
	}

	var orderedFields []string
	fieldToMetric := map[string]MetricDef{}
	fieldToLabels := map[string][]string{}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if fn.Name.Name == "initMetrics" {
			orderedFields, fieldToMetric = extractMetricDefs(fn)
			continue
		}
		for field, keys := range extractLabelsFromFunc(fn) {
			fieldToLabels[field] = mergeKeys(fieldToLabels[field], keys)
		}
	}

	if extraFile != "" {
		if ef, err := parser.ParseFile(fset, extraFile, nil, 0); err == nil {
			for _, decl := range ef.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				for field, keys := range extractLabelsFromFunc(fn) {
					fieldToLabels[field] = mergeKeys(fieldToLabels[field], keys)
				}
			}
		}
	}

	var counters, histograms, gauges []MetricDef
	for _, fieldName := range orderedFields {
		def := fieldToMetric[fieldName]
		labels := append([]string(nil), fieldToLabels[fieldName]...)
		sort.Strings(labels)
		def.Labels = labels
		switch def.Type {
		case TypeCounter:
			counters = append(counters, def)
		case TypeHistogram:
			histograms = append(histograms, def)
		case TypeGauge:
			gauges = append(gauges, def)
		}
	}

	return templateData{Counters: counters, Histograms: histograms, Gauges: gauges}, nil
}

// extractMetricDefs walks the initMetrics function body and returns field names
// in declaration order alongside a map of field name -> MetricDef.
func extractMetricDefs(fn *ast.FuncDecl) ([]string, map[string]MetricDef) {
	var orderedFields []string
	result := map[string]MetricDef{}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		assign, ok := ifStmt.Init.(*ast.AssignStmt)
		if !ok || assign.Tok != token.ASSIGN {
			return true
		}
		if len(assign.Lhs) < 1 || len(assign.Rhs) != 1 {
			return true
		}

		// LHS[0]: m.FieldName
		lhsSel, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		fieldName := lhsSel.Sel.Name

		// RHS: meter.Method("name", options...)
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		callSel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		callerIdent, ok := callSel.X.(*ast.Ident)
		if !ok || callerIdent.Name != "meter" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		def := MetricDef{
			Name: strings.Trim(lit.Value, `"`),
			Type: methodToType(callSel.Sel.Name),
		}

		for _, arg := range call.Args[1:] {
			optCall, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			optSel, ok := optCall.Fun.(*ast.SelectorExpr)
			if !ok || len(optCall.Args) == 0 {
				continue
			}
			optLit, ok := optCall.Args[0].(*ast.BasicLit)
			if !ok || optLit.Kind != token.STRING {
				continue
			}
			switch optSel.Sel.Name {
			case "WithDescription":
				def.Description = strings.Trim(optLit.Value, `"`)
			case "WithUnit":
				def.Unit = strings.Trim(optLit.Value, `"`)
			}
		}

		if _, exists := result[fieldName]; !exists {
			orderedFields = append(orderedFields, fieldName)
		}
		result[fieldName] = def
		return true
	})

	return orderedFields, result
}

// extractLabelsFromFunc returns a map of Instruments field name -> label keys
// for all metric Add/Record calls found in fn.
//
// It handles both inline attribute.String calls and the common pattern of
// assigning metric.WithAttributes(...) to a local variable first.
func extractLabelsFromFunc(fn *ast.FuncDecl) map[string][]string {
	// First pass: collect local variable assignments whose RHS contains
	// attribute keys (e.g. attrs := metric.WithAttributes(attribute.String(...))).
	varAttrs := map[string][]string{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE {
			return true
		}
		if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		if keys := collectAttrKeys(assign.Rhs[0]); len(keys) > 0 {
			varAttrs[ident.Name] = keys
		}
		return true
	})

	// Second pass: find Add/Record calls and collect the label keys for each
	// instrument, expanding any variable references found in the first pass.
	result := map[string][]string{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		field := extractInstrumentField(call.Fun)
		if field == "" {
			return true
		}

		var keys []string
		for _, arg := range call.Args {
			keys = append(keys, collectAttrKeys(arg)...)
			if ident, ok := arg.(*ast.Ident); ok {
				keys = append(keys, varAttrs[ident.Name]...)
			}
		}

		if len(keys) > 0 {
			result[field] = mergeKeys(result[field], keys)
		}
		return true
	})

	return result
}

// collectAttrKeys recursively finds all attribute package call keys in expr.
// It matches any call of the form attribute.X("key", ...) where X is any
// exported function (String, Int64, Float64, Bool, etc.).
func collectAttrKeys(expr ast.Expr) []string {
	var keys []string
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "attribute" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		keys = append(keys, strings.Trim(lit.Value, `"`))
		return true
	})
	return keys
}

// extractInstrumentField returns the Instruments struct field name from the Fun
// expression of an Add or Record call. Returns an empty string if the
// expression does not match the expected pattern.
//
// Recognised patterns:
//
//	Metrics().FieldName.Add(...)         – local Metrics() call
//	m.FieldName.Add(...)                  – local variable
//	telemetry.Metrics().FieldName.Add(...) – cross-package call
func extractInstrumentField(fun ast.Expr) string {
	sel1, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if sel1.Sel.Name != "Add" && sel1.Sel.Name != "Record" {
		return ""
	}
	sel2, ok := sel1.X.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	switch sel2.X.(type) {
	case *ast.CallExpr, *ast.Ident:
		return sel2.Sel.Name
	}
	return ""
}

// methodToType maps an OTel meter method name to the documentation MetricType.
func methodToType(method string) MetricType {
	switch method {
	case "Int64Counter", "Float64Counter":
		return TypeCounter
	case "Int64UpDownCounter", "Float64UpDownCounter":
		return TypeGauge
	default:
		return TypeHistogram
	}
}

// normalizeNewlines collapses 3+ consecutive newlines to 2 and ensures the
// output ends with exactly one newline.
var multiBlank = regexp.MustCompile(`\n{3,}`)

func normalizeNewlines(b []byte) []byte {
	b = multiBlank.ReplaceAll(b, []byte("\n\n"))
	b = bytes.TrimRight(b, "\n")
	return append(b, '\n')
}

// mergeKeys returns a deduplicated union of existing and newKeys, preserving
// insertion order.
func mergeKeys(existing, newKeys []string) []string {
	seen := make(map[string]bool, len(existing)+len(newKeys))
	for _, k := range existing {
		seen[k] = true
	}
	result := append([]string(nil), existing...)
	for _, k := range newKeys {
		if !seen[k] {
			seen[k] = true
			result = append(result, k)
		}
	}
	return result
}
