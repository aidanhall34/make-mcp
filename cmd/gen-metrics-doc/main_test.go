package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestMethodToType(t *testing.T) {
	tests := []struct {
		method string
		want   MetricType
	}{
		{"Int64Counter", TypeCounter},
		{"Float64Counter", TypeCounter},
		{"Int64UpDownCounter", TypeGauge},
		{"Float64UpDownCounter", TypeGauge},
		{"Int64Gauge", TypeGauge},
		{"Float64Gauge", TypeGauge},
		{"Int64Histogram", TypeHistogram},
		{"Float64Histogram", TypeHistogram},
		{"unknown", TypeHistogram},
	}
	for _, tc := range tests {
		if got := methodToType(tc.method); got != tc.want {
			t.Errorf("methodToType(%q) = %q, want %q", tc.method, got, tc.want)
		}
	}
}

func TestMergeKeys(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		new      []string
		want     []string
	}{
		{"both nil", nil, nil, nil},
		{"empty existing", nil, []string{"a", "b"}, []string{"a", "b"}},
		{"no overlap", []string{"a"}, []string{"b"}, []string{"a", "b"}},
		{"full overlap", []string{"a", "b"}, []string{"a", "b"}, []string{"a", "b"}},
		{"partial overlap", []string{"a", "b"}, []string{"b", "c"}, []string{"a", "b", "c"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeKeys(tc.existing, tc.new)
			if len(got) != len(tc.want) {
				t.Fatalf("mergeKeys = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("mergeKeys[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestMetricDefLabelsOrNone(t *testing.T) {
	tests := []struct {
		labels []string
		want   string
	}{
		{nil, "_(none)_"},
		{[]string{}, "_(none)_"},
		{[]string{"status"}, "`status`"},
		{[]string{"status", "tool"}, "`status`, `tool`"},
	}
	for _, tc := range tests {
		m := MetricDef{Labels: tc.labels}
		if got := m.LabelsOrNone(); got != tc.want {
			t.Errorf("LabelsOrNone(%v) = %q, want %q", tc.labels, got, tc.want)
		}
	}
}

func TestMetricDefDescriptionOrDash(t *testing.T) {
	if (MetricDef{}).DescriptionOrDash() != "—" {
		t.Error("empty description should return em-dash")
	}
	m := MetricDef{Description: "Some description."}
	if got := m.DescriptionOrDash(); got != "Some description." {
		t.Errorf("DescriptionOrDash() = %q, want %q", got, "Some description.")
	}
}

func TestCollectAttrKeys(t *testing.T) {
	src := `package p

import "go.opentelemetry.io/otel/attribute"

func f() {
	_ = attribute.String("status", v)
	_ = attribute.Int64("count", 0)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatal("no func found in parsed source")
	}
	var keys []string
	for _, stmt := range fn.Body.List {
		exprStmt := stmt.(*ast.AssignStmt)
		keys = append(keys, collectAttrKeys(exprStmt.Rhs[0])...)
	}

	want := []string{"status", "count"}
	if len(keys) != len(want) {
		t.Fatalf("collectAttrKeys = %v, want %v", keys, want)
	}
	for i := range keys {
		if keys[i] != want[i] {
			t.Errorf("keys[%d] = %q, want %q", i, keys[i], want[i])
		}
	}
}

func TestExtractInstrumentField(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`package p; func f() { Metrics().MyField.Add(ctx, 1) }`, "MyField"},
		{`package p; func f() { m.MyField.Record(ctx, 1.0) }`, "MyField"},
		{`package p; func f() { telemetry.Metrics().MyField.Add(ctx, 1) }`, "MyField"},
		{`package p; func f() { something.Else(ctx) }`, ""},
		{`package p; func f() { m.MyField.NotAddOrRecord(ctx) }`, ""},
	}
	for _, tc := range tests {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p.go", tc.src, 0)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		fn := f.Decls[0].(*ast.FuncDecl)
		stmt := fn.Body.List[0].(*ast.ExprStmt)
		call := stmt.X.(*ast.CallExpr)
		got := extractInstrumentField(call.Fun)
		if got != tc.want {
			t.Errorf("extractInstrumentField(%q) = %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestMetricDefUnitOrDash(t *testing.T) {
	if (MetricDef{}).UnitOrDash() != "—" {
		t.Error("empty unit should return em-dash")
	}
	m := MetricDef{Unit: "s"}
	if got := m.UnitOrDash(); got != "s" {
		t.Errorf("UnitOrDash() = %q, want %q", got, "s")
	}
}

func TestNormalizeNewlines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single trailing newline preserved", "hello\n", "hello\n"},
		{"multiple trailing newlines collapsed to one", "hello\n\n\n", "hello\n"},
		{"triple blank lines collapsed to double", "a\n\n\n\nb", "a\n\nb\n"},
		{"no trailing newline gets one added", "hello", "hello\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(normalizeNewlines([]byte(tc.input)))
			if got != tc.want {
				t.Errorf("normalizeNewlines(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestBuildTemplateData_MissingFile(t *testing.T) {
	_, err := buildTemplateData("nonexistent_file.go", "")
	if err == nil {
		t.Error("expected error for missing metrics file, got nil")
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()

	// Minimal valid Go source with an initMetrics function.
	metricsGo := `package telemetry

import "go.opentelemetry.io/otel/metric"

type Instruments struct {
	Calls metric.Int64Counter
}

func initMetrics(meter metric.Meter) Instruments {
	var m Instruments
	if m.Calls, _ = meter.Int64Counter("myapp_calls_total"); true {
	}
	return m
}
`
	metricsFile := filepath.Join(dir, "metrics.go")
	if err := os.WriteFile(metricsFile, []byte(metricsGo), 0o644); err != nil {
		t.Fatalf("write metrics.go: %v", err)
	}

	tmplSrc := "# Metrics\n{{range .Counters}}- {{.Name}}\n{{end}}\n"
	tmplFile := filepath.Join(dir, "metrics.md.tmpl")
	if err := os.WriteFile(tmplFile, []byte(tmplSrc), 0o644); err != nil {
		t.Fatalf("write tmpl: %v", err)
	}

	outFile := filepath.Join(dir, "Metrics.md")

	t.Run("generates outdated file", func(t *testing.T) {
		err := run(metricsFile, "", tmplFile, outFile)
		if !errors.Is(err, errOutdated) {
			t.Fatalf("expected errOutdated, got %v", err)
		}
		if _, statErr := os.Stat(outFile); statErr != nil {
			t.Fatalf("output file not created: %v", statErr)
		}
	})

	t.Run("up to date returns nil", func(t *testing.T) {
		err := run(metricsFile, "", tmplFile, outFile)
		if err != nil {
			t.Fatalf("expected nil for up-to-date file, got %v", err)
		}
	})

	t.Run("missing template file", func(t *testing.T) {
		err := run(metricsFile, "", filepath.Join(dir, "no.tmpl"), outFile)
		if err == nil {
			t.Error("expected error for missing template, got nil")
		}
	})

	t.Run("invalid template", func(t *testing.T) {
		badTmpl := filepath.Join(dir, "bad.tmpl")
		if err := os.WriteFile(badTmpl, []byte("{{.Invalid}"), 0o644); err != nil {
			t.Fatalf("write bad tmpl: %v", err)
		}
		err := run(metricsFile, "", badTmpl, outFile)
		if err == nil {
			t.Error("expected error for invalid template, got nil")
		}
	})
}

func TestExtractLabelsFromFunc_VarAttrs(t *testing.T) {
	src := `package p

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func f(m struct{ Calls metric.Int64Counter }) {
	attrs := metric.WithAttributes(attribute.String("env", "prod"))
	m.Calls.Add(ctx, 1, attrs)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			fn = fd
			break
		}
	}
	result := extractLabelsFromFunc(fn, nil)
	keys, ok := result["Calls"]
	if !ok {
		t.Fatal("expected label keys for Calls, got none")
	}
	if len(keys) != 1 || keys[0] != "env" {
		t.Errorf("Calls labels = %v, want [env]", keys)
	}
}

func TestExtractMetricDefs(t *testing.T) {
	src := `package p

import "go.opentelemetry.io/otel/metric"

func initMetrics(meter metric.Meter) {
	var m struct {
		Hits   metric.Int64Counter
		Dur    metric.Float64Histogram
		Active metric.Int64UpDownCounter
	}
	if m.Hits, _ = meter.Int64Counter("app_hits_total",
		metric.WithDescription("Number of hits."),
	); true {}
	if m.Dur, _ = meter.Float64Histogram("app_duration_seconds",
		metric.WithUnit("s"),
	); true {}
	if m.Active, _ = meter.Int64UpDownCounter("app_active"); true {}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			fn = fd
			break
		}
	}
	orderedFields, fieldMap := extractMetricDefs(fn)
	if len(orderedFields) != 3 {
		t.Fatalf("orderedFields = %v, want 3 entries", orderedFields)
	}

	hits := fieldMap["Hits"]
	if hits.Name != "app_hits_total" || hits.Type != TypeCounter || hits.Description != "Number of hits." {
		t.Errorf("Hits = %+v", hits)
	}
	dur := fieldMap["Dur"]
	if dur.Name != "app_duration_seconds" || dur.Type != TypeHistogram || dur.Unit != "s" {
		t.Errorf("Dur = %+v", dur)
	}
	active := fieldMap["Active"]
	if active.Name != "app_active" || active.Type != TypeGauge {
		t.Errorf("Active = %+v", active)
	}
}

// TestBuildTemplateData_RealMetrics is an integration test that parses the
// actual telemetry package and verifies key properties of the parsed output.
func TestBuildTemplateData_RealMetrics(t *testing.T) {
	data, err := buildTemplateData(
		"../../pkg/telemetry/metrics.go",
		"../../cmd/mcp-server/main.go",
	)
	if err != nil {
		t.Fatalf("buildTemplateData: %v", err)
	}

	total := len(data.Counters) + len(data.Histograms) + len(data.Gauges)
	if total != 27 {
		t.Errorf("total metrics = %d, want 27 (counters=%d histograms=%d gauges=%d)",
			total, len(data.Counters), len(data.Histograms), len(data.Gauges))
	}

	// ConnectedClients must be a Gauge with a "transport" label.
	var foundGauge bool
	for _, m := range data.Gauges {
		if m.Name == "make_mcp_connected_clients" {
			foundGauge = true
			if len(m.Labels) != 1 || m.Labels[0] != "transport" {
				t.Errorf("connected_clients labels = %v, want [transport]", m.Labels)
			}
		}
	}
	if !foundGauge {
		t.Error("make_mcp_connected_clients not found in Gauges")
	}

	// Tool invocations counter must have tool, risk, status labels.
	var foundInvocations bool
	for _, m := range data.Counters {
		if m.Name == "make_mcp_tool_invocations_total" {
			foundInvocations = true
			wantLabels := map[string]bool{"tool": true, "risk": true, "status": true}
			for _, l := range m.Labels {
				delete(wantLabels, l)
			}
			if len(wantLabels) > 0 {
				t.Errorf("tool_invocations_total missing labels: %v (got %v)", wantLabels, m.Labels)
			}
		}
	}
	if !foundInvocations {
		t.Error("make_mcp_tool_invocations_total not found in Counters")
	}

	// Latency histogram must have a description and unit.
	for _, m := range data.Histograms {
		if m.Name == "make_mcp_tools_list_request_latency_seconds" {
			if m.Description == "" {
				t.Error("latency histogram has no description")
			}
			if m.Unit != "s" {
				t.Errorf("latency histogram unit = %q, want %q", m.Unit, "s")
			}
		}
	}

	// tools_listed_total must carry risk and MCP tool-hint labels.
	wantListedLabels := map[string]bool{
		"risk":        true,
		"read_only":   true,
		"destructive": true,
		"idempotent":  true,
		"open_world":  true,
	}
	var foundListed bool
	for _, m := range data.Counters {
		if m.Name == "make_mcp_tools_listed_total" {
			foundListed = true
			remaining := make(map[string]bool)
			for k, v := range wantListedLabels {
				remaining[k] = v
			}
			for _, l := range m.Labels {
				delete(remaining, l)
			}
			if len(remaining) > 0 {
				t.Errorf("tools_listed_total missing labels: %v (got %v)", remaining, m.Labels)
			}
		}
	}
	if !foundListed {
		t.Error("make_mcp_tools_listed_total not found in Counters")
	}

	// tools_registered must carry risk and MCP tool-hint labels.
	wantRegisteredLabels := map[string]bool{
		"risk":        true,
		"read_only":   true,
		"destructive": true,
		"idempotent":  true,
		"open_world":  true,
	}
	var foundRegistered bool
	for _, m := range data.Gauges {
		if m.Name == "make_mcp_tools_registered" {
			foundRegistered = true
			remaining := make(map[string]bool)
			for k, v := range wantRegisteredLabels {
				remaining[k] = v
			}
			for _, l := range m.Labels {
				delete(remaining, l)
			}
			if len(remaining) > 0 {
				t.Errorf("tools_registered missing labels: %v (got %v)", remaining, m.Labels)
			}
		}
	}
	if !foundRegistered {
		t.Error("make_mcp_tools_registered not found in Gauges")
	}
}
