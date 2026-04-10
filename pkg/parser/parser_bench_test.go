package parser_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aidanhall34/make-mcp/internal/testtel"
	"github.com/aidanhall34/make-mcp/pkg/parser"
)

func generateMakefile(n int) string {
	var sb strings.Builder
	for i := range n {
		fmt.Fprintf(&sb, `
# @ name: Tool %d
# @ description: Does something useful for tool number %d.
# @ risk: low
# @ param: input string | The input value for tool %d
# @ output: Result from tool %d
# @ output-type: text/plain
tool-%d:
	@echo "tool %d"

`, i, i, i, i, i, i)
	}
	return sb.String()
}

func BenchmarkParseMakefile_Small(b *testing.B) {
	_ = testtel.Start(b)
	input := generateMakefile(5)
	opts := parser.ParseOptions{Delimiter: "@"}
	b.ResetTimer()
	for b.Loop() {
		parser.ParseMakefile(strings.NewReader(input), opts) //nolint:errcheck
	}
}

func BenchmarkParseMakefile_Large(b *testing.B) {
	_ = testtel.Start(b)
	input := generateMakefile(50)
	opts := parser.ParseOptions{Delimiter: "@"}
	b.ResetTimer()
	for b.Loop() {
		parser.ParseMakefile(strings.NewReader(input), opts) //nolint:errcheck
	}
}
