package main

import (
	"go/ast"
	"go/parser"
	"go/token"
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
	if total != 11 {
		t.Errorf("total metrics = %d, want 11 (counters=%d histograms=%d gauges=%d)",
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
}
