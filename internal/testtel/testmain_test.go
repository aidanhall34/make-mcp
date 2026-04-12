package testtel_test

import (
	"os"
	"testing"

	"github.com/aidanhall34/make-mcp/internal/covercheck"
)

func TestMain(m *testing.M) {
	// testtel cannot use testtel.RunMain (circular dependency on covercheck).
	// RunMain and Init's OTLP path require a live endpoint / testing.M constructor,
	// so they are legitimately untestable in unit tests.  The threshold reflects
	// the achievable coverage for the remaining logic.
	os.Exit(covercheck.Report(m, 0.48))
}
