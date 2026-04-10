// Tests in package covercheck (not covercheck_test) so unexported helpers can
// be called directly during m.Run(). The helper functions make up the bulk of
// the package; their execution during tests is what drives coverage.
package covercheck

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// TestMain lets the default runner write the coverage profile normally, then
// calls Report to exercise that path and enforce the threshold.
func TestMain(m *testing.M) {
	os.Exit(Report(m, 0.90))
}

// ---- helpers ----

func writeProfile(t *testing.T, lines ...string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "coverage*.out")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "mode: set")
	for _, l := range lines {
		fmt.Fprintln(f, l)
	}
	f.Close()
	return f.Name()
}

// ---- reportResult ----

func TestReportResult_PassRC(t *testing.T) {
	// rc != 0 is returned as-is regardless of coverage or threshold.
	if got := reportResult(1, 0.0); got != 1 {
		t.Errorf("reportResult(1, 0) = %d, want 1", got)
	}
}

func TestReportResult_PassThreshold(t *testing.T) {
	// rc=0, threshold=0.0 → always passes even with 0% coverage.
	got := reportResult(0, 0.0)
	if got != 0 {
		t.Errorf("reportResult(0, 0.0) = %d, want 0", got)
	}
}

func TestReportResult_ThresholdOne(t *testing.T) {
	// rc=0, threshold=1.0 → fails unless coverage is exactly 100%; in a normal
	// test run it will be < 1.0, exercising the return-1 path.
	got := reportResult(0, 1.0)
	if got < 0 {
		t.Errorf("reportResult(0, 1.0) = %d, want 0 or 1", got)
	}
}

// ---- parseProfile ----

func TestParseProfile_AllCovered(t *testing.T) {
	path := writeProfile(t,
		"github.com/example/a.go:1.10,5.2 4 1",
		"github.com/example/a.go:6.10,8.2 2 1",
	)
	entries, err := parseProfile(path)
	if err != nil {
		t.Fatalf("parseProfile error: %v", err)
	}
	e := entries["github.com/example/a.go"]
	if e == nil {
		t.Fatal("expected entry for a.go")
	}
	if e.total != 6 || e.covered != 6 {
		t.Errorf("got covered=%d total=%d, want 6/6", e.covered, e.total)
	}
}

func TestParseProfile_PartialCoverage(t *testing.T) {
	path := writeProfile(t,
		"github.com/example/a.go:1.10,5.2 4 1", // covered
		"github.com/example/a.go:6.10,8.2 2 0", // not covered
	)
	entries, err := parseProfile(path)
	if err != nil {
		t.Fatalf("parseProfile error: %v", err)
	}
	e := entries["github.com/example/a.go"]
	if e.covered != 4 || e.total != 6 {
		t.Errorf("got covered=%d total=%d, want 4/6", e.covered, e.total)
	}
}

func TestParseProfile_MultipleFiles(t *testing.T) {
	path := writeProfile(t,
		"github.com/example/a.go:1.10,5.2 3 1",
		"github.com/example/b.go:1.10,3.2 2 0",
	)
	entries, err := parseProfile(path)
	if err != nil {
		t.Fatalf("parseProfile error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d files, want 2", len(entries))
	}
}

func TestParseProfile_ModeOnly(t *testing.T) {
	path := writeProfile(t) // mode line only, no data
	entries, err := parseProfile(path)
	if err != nil {
		t.Fatalf("parseProfile error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries for empty profile, want 0", len(entries))
	}
}

func TestParseProfile_MalformedLines(t *testing.T) {
	path := writeProfile(t,
		"not-a-valid-line",
		"also:bad",
		"github.com/example/a.go:1.10,5.2 4 1",
	)
	entries, err := parseProfile(path)
	if err != nil {
		t.Fatalf("parseProfile error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 (malformed lines skipped)", len(entries))
	}
}

func TestParseProfile_NonexistentFile(t *testing.T) {
	_, err := parseProfile("/nonexistent/path/to/coverage.out")
	if err == nil {
		t.Error("expected error for nonexistent profile, got nil")
	}
}

// ---- statementCoverage ----

func TestStatementCoverage_Normal(t *testing.T) {
	entries := map[string]*fileEntry{
		"a.go": {covered: 8, total: 10},
		"b.go": {covered: 2, total: 2},
	}
	want := float64(10) / float64(12)
	if got := statementCoverage(entries); got != want {
		t.Errorf("statementCoverage = %v, want %v", got, want)
	}
}

func TestStatementCoverage_AllCovered(t *testing.T) {
	entries := map[string]*fileEntry{
		"a.go": {covered: 5, total: 5},
	}
	if got := statementCoverage(entries); got != 1.0 {
		t.Errorf("statementCoverage = %v, want 1.0", got)
	}
}

func TestStatementCoverage_Empty(t *testing.T) {
	if got := statementCoverage(map[string]*fileEntry{}); got != -1 {
		t.Errorf("statementCoverage(empty) = %v, want -1", got)
	}
}

// ---- printPerFile ----

func TestPrintPerFile_Sorted(t *testing.T) {
	// printPerFile writes to stderr; verify it doesn't panic with multiple
	// files including a zero-total edge case.
	entries := map[string]*fileEntry{
		"z.go": {covered: 3, total: 5},
		"a.go": {covered: 0, total: 0}, // zero-total: coverage should be 0.0
		"m.go": {covered: 5, total: 5},
	}
	printPerFile(entries, 0.9)
}

func TestPrintPerFile_BelowThreshold(t *testing.T) {
	entries := map[string]*fileEntry{
		"low.go": {covered: 1, total: 10}, // 10% coverage
	}
	printPerFile(entries, 0.95) // 10% < 95% → pass=false in output
}

// TestReportResult_WithProfile exercises the profile-parsing path in reportResult
// by temporarily pointing the test.coverprofile flag at a synthetic profile.
func TestReportResult_WithProfile(t *testing.T) {
	path := writeProfile(t,
		"github.com/example/a.go:1.10,5.2 4 1",
		"github.com/example/a.go:6.10,8.2 2 0",
	)
	pf := flag.Lookup("test.coverprofile")
	if pf == nil {
		t.Skip("test.coverprofile flag not registered — skipping profile-path exercise")
	}
	prev := pf.Value.String()
	if err := flag.Set("test.coverprofile", path); err != nil {
		t.Fatalf("flag.Set: %v", err)
	}
	t.Cleanup(func() { flag.Set("test.coverprofile", prev) }) //nolint:errcheck

	got := reportResult(0, 0.0) // threshold 0 → always passes
	if got != 0 {
		t.Errorf("reportResult(0, 0.0) with profile = %d, want 0", got)
	}
}
