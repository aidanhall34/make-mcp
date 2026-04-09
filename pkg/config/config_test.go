package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/config"
)

func TestDefault(t *testing.T) {
	cfg := config.Default()
	if cfg.Delimiter != config.DefaultDelimiter {
		t.Errorf("default delimiter: got %q, want %q", cfg.Delimiter, config.DefaultDelimiter)
	}
	if len(cfg.Makefiles) != 0 {
		t.Errorf("default makefiles: got %v, want empty", cfg.Makefiles)
	}
	if cfg.Transport != config.DefaultTransport {
		t.Errorf("default transport: got %q, want %q", cfg.Transport, config.DefaultTransport)
	}
	if cfg.Listen != config.DefaultListenAddress {
		t.Errorf("default listen: got %q, want %q", cfg.Listen, config.DefaultListenAddress)
	}
	if cfg.Timeouts.Low != 10*time.Minute || cfg.Timeouts.Medium != 10*time.Minute || cfg.Timeouts.High != 10*time.Minute {
		t.Errorf("default timeouts: got %+v, want all 10m", cfg.Timeouts)
	}
}

func TestLoadFile_Valid(t *testing.T) {
	content := `
delimiter: "##"
makefiles:
  - ./makefile
  - ./other/Makefile
transport: http
listen: 127.0.0.1:9999
strict: true
timeouts:
  low: 1m
  medium: 2m
  high: 3m
`
	f, err := os.CreateTemp(t.TempDir(), "make-mcp-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := config.LoadFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Delimiter != "##" {
		t.Errorf("delimiter: got %q, want %q", cfg.Delimiter, "##")
	}
	if len(cfg.Makefiles) != 2 {
		t.Errorf("makefiles count: got %d, want 2", len(cfg.Makefiles))
	}
	if cfg.Makefiles[0] != "./makefile" {
		t.Errorf("makefiles[0]: got %q, want %q", cfg.Makefiles[0], "./makefile")
	}
	if cfg.Transport != "http" {
		t.Errorf("transport: got %q, want %q", cfg.Transport, "http")
	}
	if cfg.Listen != "127.0.0.1:9999" {
		t.Errorf("listen: got %q, want %q", cfg.Listen, "127.0.0.1:9999")
	}
	if !cfg.Strict {
		t.Error("strict: got false, want true")
	}
	if cfg.Timeouts.Low != time.Minute || cfg.Timeouts.Medium != 2*time.Minute || cfg.Timeouts.High != 3*time.Minute {
		t.Errorf("timeouts: got %+v", cfg.Timeouts)
	}
}

func TestLoadFile_OnlyDelimiter(t *testing.T) {
	content := `delimiter: "mcp"`
	f, err := os.CreateTemp(t.TempDir(), "make-mcp-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()

	cfg, err := config.LoadFile(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Delimiter != "mcp" {
		t.Errorf("delimiter: got %q, want %q", cfg.Delimiter, "mcp")
	}
	if len(cfg.Makefiles) != 0 {
		t.Errorf("makefiles: got %v, want empty", cfg.Makefiles)
	}
}

func TestLoadFile_Missing(t *testing.T) {
	_, err := config.LoadFile("/nonexistent/path/make-mcp.yml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "make-mcp-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(": invalid: yaml: [[[")
	f.Close()

	_, err = config.LoadFile(f.Name())
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestMerge_OverrideWins(t *testing.T) {
	base := config.Config{
		Delimiter: "@",
		Makefiles: []string{"./a"},
		Transport: "stdio",
		Listen:    "127.0.0.1:9378",
		Strict:    false,
		Timeouts: config.Timeouts{
			Low:    time.Minute,
			Medium: 2 * time.Minute,
			High:   3 * time.Minute,
		},
	}
	override := config.Config{
		Delimiter: "##",
		Makefiles: []string{"./b", "./c"},
		Transport: "http",
		Listen:    "127.0.0.1:9999",
		Strict:    true,
		Timeouts: config.Timeouts{
			Low:    4 * time.Minute,
			Medium: 5 * time.Minute,
			High:   6 * time.Minute,
		},
	}

	result := config.Merge(base, override)
	if result.Delimiter != "##" {
		t.Errorf("delimiter: got %q, want %q", result.Delimiter, "##")
	}
	if len(result.Makefiles) != 2 || result.Makefiles[0] != "./b" {
		t.Errorf("makefiles: got %v, want [./b ./c]", result.Makefiles)
	}
	if result.Transport != "http" {
		t.Errorf("transport: got %q, want %q", result.Transport, "http")
	}
	if result.Listen != "127.0.0.1:9999" {
		t.Errorf("listen: got %q, want %q", result.Listen, "127.0.0.1:9999")
	}
	if !result.Strict {
		t.Error("strict: got false, want true")
	}
	if result.Timeouts.Low != 4*time.Minute || result.Timeouts.Medium != 5*time.Minute || result.Timeouts.High != 6*time.Minute {
		t.Errorf("timeouts: got %+v", result.Timeouts)
	}
}

func TestMerge_BasePreservedWhenOverrideEmpty(t *testing.T) {
	base := config.Config{
		Delimiter: "@",
		Makefiles: []string{"./a"},
		Transport: "stdio",
		Listen:    "127.0.0.1:9378",
		Timeouts: config.Timeouts{
			Low:    time.Minute,
			Medium: 2 * time.Minute,
			High:   3 * time.Minute,
		},
	}
	override := config.Config{}

	result := config.Merge(base, override)
	if result.Delimiter != "@" {
		t.Errorf("delimiter: got %q, want %q", result.Delimiter, "@")
	}
	if len(result.Makefiles) != 1 || result.Makefiles[0] != "./a" {
		t.Errorf("makefiles: got %v, want [./a]", result.Makefiles)
	}
	if result.Transport != "stdio" {
		t.Errorf("transport: got %q, want %q", result.Transport, "stdio")
	}
	if result.Listen != "127.0.0.1:9378" {
		t.Errorf("listen: got %q, want %q", result.Listen, "127.0.0.1:9378")
	}
	if result.Timeouts.Low != time.Minute || result.Timeouts.Medium != 2*time.Minute || result.Timeouts.High != 3*time.Minute {
		t.Errorf("timeouts: got %+v", result.Timeouts)
	}
}

func TestMerge_DoesNotMutateBase(t *testing.T) {
	base := config.Config{Delimiter: "@", Makefiles: []string{"./a"}}
	override := config.Config{Delimiter: "##"}

	_ = config.Merge(base, override)
	if base.Delimiter != "@" {
		t.Errorf("Merge mutated base: delimiter is now %q", base.Delimiter)
	}
}
