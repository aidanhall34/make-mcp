package config_test

import (
	"os"
	"testing"

	"github.com/aidanhall34/make-mcp/internal/testtel"
)

func TestMain(m *testing.M) {
	os.Exit(testtel.RunMain(m, 0.95))
}
