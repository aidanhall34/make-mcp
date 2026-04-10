package main

import (
	"os"
	"testing"
)

func TestStringSlice(t *testing.T) {
	var s stringSlice
	if got := s.String(); got != "" {
		t.Errorf("empty String() = %q, want %q", got, "")
	}
	if err := s.Set("a"); err != nil {
		t.Fatalf("Set(a) error = %v", err)
	}
	if err := s.Set("b"); err != nil {
		t.Fatalf("Set(b) error = %v", err)
	}
	if got := s.String(); got != "a, b" {
		t.Errorf("String() = %q, want %q", got, "a, b")
	}
}

func TestRun_FlagParseError(t *testing.T) {
	if err := run([]string{"--unknown-flag-xyz"}); err == nil {
		t.Error("expected error for unknown flag, got nil")
	}
}

func TestRun_Help(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Errorf("run(--help) = %v, want nil", err)
	}
}

func TestRun_NoMakefiles(t *testing.T) {
	if err := run([]string{}); err == nil {
		t.Error("expected error for no makefiles, got nil")
	}
}

func TestRun_InvalidConfigFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(": invalid: yaml: [[[")
	f.Close()

	if err := run([]string{"--config", f.Name()}); err == nil {
		t.Error("expected error for invalid config file, got nil")
	}
}

func TestRun_NonExistentMakefile(t *testing.T) {
	if err := run([]string{"--makefile", "/nonexistent/Makefile"}); err == nil {
		t.Error("expected error for nonexistent makefile, got nil")
	}
}

func TestRun_NoAnnotationsFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "Makefile")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello:\n\t@echo hi\n")
	f.Close()

	if err := run([]string{"--makefile", f.Name()}); err != nil {
		t.Errorf("run(no-annotation makefile) = %v, want nil", err)
	}
}

func TestRun_ValidMakefile(t *testing.T) {
	// testdata/Makefile is valid and contains annotated recipes.
	if err := run([]string{"--makefile", "../../testdata/Makefile"}); err != nil {
		t.Errorf("run(testdata/Makefile) = %v, want nil", err)
	}
}

func TestRun_ConfigWithMakefiles(t *testing.T) {
	cfg := "makefiles:\n  - ../../testdata/Makefile\n"
	f, err := os.CreateTemp(t.TempDir(), "*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(cfg)
	f.Close()

	if err := run([]string{"--config", f.Name()}); err != nil {
		t.Errorf("run(config with makefiles) = %v, want nil", err)
	}
}

func TestRun_DelimiterFlag(t *testing.T) {
	// Delimiter flag overrides the config; the testdata file uses "@" so a
	// different delimiter should find zero annotated recipes.
	f, err := os.CreateTemp(t.TempDir(), "Makefile")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello:\n\t@echo hi\n")
	f.Close()

	if err := run([]string{"--makefile", f.Name(), "--delimiter", "##"}); err != nil {
		t.Errorf("run(custom delimiter) = %v, want nil", err)
	}
}
