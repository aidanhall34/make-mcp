// Package covercheck enforces a minimum test-coverage threshold from within
// TestMain and emits one JSON object per Go source file plus a summary object.
//
// Usage:
//
//	func TestMain(m *testing.M) {
//	    os.Exit(covercheck.Report(m, 0.95))
//	}
package covercheck

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// Report runs all tests via m.Run, then (when the binary was built with
// -cover) emits coverage as JSON and enforces the threshold. Returns the
// integer exit code suitable for os.Exit.
//
// When a -coverprofile file is available its per-source-file statistics are
// printed first, one JSON object per line:
//
//	{"file":"pkg/foo/foo.go","coverage":0.91,"pass":true}
//
// A summary object is always printed last:
//
//	{"coverage":0.95,"threshold":0.95,"pass":true}
//
// Coverage is measured as statement coverage (matching go tool cover) when a
// -coverprofile is available, and falls back to block coverage via
// testing.Coverage() otherwise.
func Report(m *testing.M, threshold float64) int {
	return reportResult(m.Run(), threshold)
}

// reportResult contains all post-run logic. It is a separate function so that
// tests can call it directly during m.Run() and have the execution counted in
// the coverage profile.
func reportResult(rc int, threshold float64) int {
	if testing.CoverMode() == "" {
		return rc
	}

	// Prefer statement coverage from the profile file (matches go tool cover).
	// The profile is flushed by m.Run() before it returns, so it is readable.
	total := testing.Coverage() // fallback: block coverage
	if pf := flag.Lookup("test.coverprofile"); pf != nil && pf.Value.String() != "" {
		if entries, err := parseProfile(pf.Value.String()); err == nil {
			printPerFile(entries, threshold)
			if t := statementCoverage(entries); t >= 0 {
				total = t
			}
		}
	}

	pass := total >= threshold

	type summary struct {
		Coverage  float64 `json:"coverage"`
		Threshold float64 `json:"threshold"`
		Pass      bool    `json:"pass"`
	}
	enc, _ := json.Marshal(summary{Coverage: total, Threshold: threshold, Pass: pass})
	fmt.Fprintln(os.Stderr, string(enc))

	if rc == 0 && !pass {
		return 1
	}
	return rc
}

// fileEntry accumulates statement counts from a coverage profile.
type fileEntry struct {
	covered int
	total   int
}

func printPerFile(entries map[string]*fileEntry, threshold float64) {
	files := make([]string, 0, len(entries))
	for f := range entries {
		files = append(files, f)
	}
	sort.Strings(files)

	type fileReport struct {
		File     string  `json:"file"`
		Coverage float64 `json:"coverage"`
		Pass     bool    `json:"pass"`
	}

	for _, file := range files {
		e := entries[file]
		cov := 0.0
		if e.total > 0 {
			cov = float64(e.covered) / float64(e.total)
		}
		enc, _ := json.Marshal(fileReport{File: file, Coverage: cov, Pass: cov >= threshold})
		fmt.Fprintln(os.Stderr, string(enc))
	}
}

// statementCoverage returns weighted statement coverage across all files or -1
// if there are no coverable statements.
func statementCoverage(entries map[string]*fileEntry) float64 {
	var covered, total int
	for _, e := range entries {
		covered += e.covered
		total += e.total
	}
	if total == 0 {
		return -1
	}
	return float64(covered) / float64(total)
}

// parseProfile reads a Go coverage profile and returns per-file statement
// counts. The profile format is:
//
//	mode: set
//	<file>:<startLine>.<startCol>,<endLine>.<endCol> <numStmts> <count>
func parseProfile(path string) (map[string]*fileEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	entries := map[string]*fileEntry{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		// Split on the last colon to separate "file" from "line.col,line.col stmts count".
		colon := strings.LastIndex(line, ":")
		if colon < 0 {
			continue
		}
		file := line[:colon]
		rest := line[colon+1:]

		var sl, sc, el, ec, numStmts, count int
		if _, err := fmt.Sscanf(rest, "%d.%d,%d.%d %d %d", &sl, &sc, &el, &ec, &numStmts, &count); err != nil {
			continue
		}

		e := entries[file]
		if e == nil {
			e = &fileEntry{}
			entries[file] = e
		}
		e.total += numStmts
		if count > 0 {
			e.covered += numStmts
		}
	}
	return entries, scanner.Err()
}
