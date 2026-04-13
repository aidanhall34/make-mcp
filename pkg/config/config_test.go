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
debug: true
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
	if !cfg.Debug {
		t.Error("debug: got false, want true")
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

func TestLoadFile_UnknownKey(t *testing.T) {
	content := `delimiter: "##"
unknown_field: oops
`
	f, err := os.CreateTemp(t.TempDir(), "make-mcp-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()

	_, err = config.LoadFile(f.Name())
	if err == nil {
		t.Error("expected error for unknown key, got nil")
	}
}

func TestMerge_DebugEnabled(t *testing.T) {
	base := config.Config{Debug: false}
	override := config.Config{Debug: true}
	result := config.Merge(base, override)
	if !result.Debug {
		t.Error("Merge: debug override did not take effect")
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

func TestLoadFile_ResourcesBlock(t *testing.T) {
	content := `
makefiles:
  - ./makefile
resources:
  recursive: true
  paths:
    - name: "config-files"
      description: "Config files"
      path:
        - "./config/*.yml"
      recursive: false
    - name: "logs"
      path:
        - "./logs/"
`
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
	if !cfg.Resources.Recursive {
		t.Error("resources.recursive: want true")
	}
	if len(cfg.Resources.Paths) != 2 {
		t.Fatalf("resources.paths: got %d, want 2", len(cfg.Resources.Paths))
	}
	p0 := cfg.Resources.Paths[0]
	if p0.Name != "config-files" {
		t.Errorf("paths[0].name: got %q, want config-files", p0.Name)
	}
	if p0.Description != "Config files" {
		t.Errorf("paths[0].description: got %q, want 'Config files'", p0.Description)
	}
	if p0.Recursive == nil || *p0.Recursive {
		t.Errorf("paths[0].recursive: want *false, got %v", p0.Recursive)
	}
	if len(p0.Path) != 1 || p0.Path[0] != "./config/*.yml" {
		t.Errorf("paths[0].path: got %v", p0.Path)
	}
	p1 := cfg.Resources.Paths[1]
	if p1.Name != "logs" {
		t.Errorf("paths[1].name: got %q, want logs", p1.Name)
	}
	if p1.Recursive != nil {
		t.Errorf("paths[1].recursive: want nil (inherit), got %v", p1.Recursive)
	}
}

func TestLoadFile_OAuthBlock(t *testing.T) {
	content := `
makefiles:
  - ./makefile
oauth:
  enabled: true
  issuer: "https://localhost:8443/realms/make-mcp"
  audience: "make-mcp"
  jwks_uri: "https://localhost:8443/realms/make-mcp/protocol/openid-connect/certs"
  tls:
    cert: ./certs/server.crt
    key: ./certs/server.key
    ca: ./certs/ca.crt
  groups:
    - name: "admins"
      resources:
        - "config-files"
`
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
	if !cfg.OAuth.Enabled {
		t.Error("oauth.enabled: want true")
	}
	if cfg.OAuth.Issuer != "https://localhost:8443/realms/make-mcp" {
		t.Errorf("oauth.issuer: got %q", cfg.OAuth.Issuer)
	}
	if cfg.OAuth.Audience != "make-mcp" {
		t.Errorf("oauth.audience: got %q", cfg.OAuth.Audience)
	}
	if cfg.OAuth.JWKSURI != "https://localhost:8443/realms/make-mcp/protocol/openid-connect/certs" {
		t.Errorf("oauth.jwks_uri: got %q", cfg.OAuth.JWKSURI)
	}
	if cfg.OAuth.TLS.Cert != "./certs/server.crt" {
		t.Errorf("oauth.tls.cert: got %q", cfg.OAuth.TLS.Cert)
	}
	if cfg.OAuth.TLS.Key != "./certs/server.key" {
		t.Errorf("oauth.tls.key: got %q", cfg.OAuth.TLS.Key)
	}
	if cfg.OAuth.TLS.CA != "./certs/ca.crt" {
		t.Errorf("oauth.tls.ca: got %q", cfg.OAuth.TLS.CA)
	}
	if len(cfg.OAuth.Groups) != 1 {
		t.Fatalf("oauth.groups: got %d, want 1", len(cfg.OAuth.Groups))
	}
	g := cfg.OAuth.Groups[0]
	if g.Name != "admins" {
		t.Errorf("groups[0].name: got %q, want admins", g.Name)
	}
	if len(g.Resources) != 1 || g.Resources[0] != "config-files" {
		t.Errorf("groups[0].resources: got %v", g.Resources)
	}
}

func TestMerge_ResourcesOverride(t *testing.T) {
	base := config.Config{
		Resources: config.ResourcesConfig{
			Paths: []config.ResourcePath{{Name: "old"}},
		},
	}
	override := config.Config{
		Resources: config.ResourcesConfig{
			Recursive: true,
			Paths:     []config.ResourcePath{{Name: "new1"}, {Name: "new2"}},
		},
	}
	result := config.Merge(base, override)
	if len(result.Resources.Paths) != 2 {
		t.Fatalf("merge resources: got %d paths, want 2", len(result.Resources.Paths))
	}
	if result.Resources.Paths[0].Name != "new1" {
		t.Errorf("merge resources: paths[0].name got %q, want new1", result.Resources.Paths[0].Name)
	}
	if !result.Resources.Recursive {
		t.Error("merge resources: recursive should be true")
	}
}

func TestMerge_ResourcesBasePreservedWhenOverrideEmpty(t *testing.T) {
	base := config.Config{
		Resources: config.ResourcesConfig{
			Paths: []config.ResourcePath{{Name: "kept"}},
		},
	}
	result := config.Merge(base, config.Config{})
	if len(result.Resources.Paths) != 1 || result.Resources.Paths[0].Name != "kept" {
		t.Errorf("merge resources: base not preserved: %v", result.Resources.Paths)
	}
}

func TestMerge_OAuthEnabled(t *testing.T) {
	base := config.Config{OAuth: config.OAuthConfig{Enabled: false, Issuer: "old"}}
	override := config.Config{OAuth: config.OAuthConfig{Enabled: true, Issuer: "new"}}
	result := config.Merge(base, override)
	if !result.OAuth.Enabled {
		t.Error("merge oauth: enabled should be true")
	}
	if result.OAuth.Issuer != "new" {
		t.Errorf("merge oauth: issuer got %q, want new", result.OAuth.Issuer)
	}
}

func TestMerge_OAuthBasePreservedWhenOverrideDisabled(t *testing.T) {
	base := config.Config{OAuth: config.OAuthConfig{Enabled: true, Issuer: "kept"}}
	result := config.Merge(base, config.Config{})
	if !result.OAuth.Enabled {
		t.Error("merge oauth: base enabled should be preserved")
	}
	if result.OAuth.Issuer != "kept" {
		t.Errorf("merge oauth: base issuer should be preserved, got %q", result.OAuth.Issuer)
	}
}

func TestDefault_ZeroValueOAuthAndResources(t *testing.T) {
	cfg := config.Default()
	if cfg.OAuth.Enabled {
		t.Error("default oauth.enabled: want false")
	}
	if len(cfg.Resources.Paths) != 0 {
		t.Errorf("default resources.paths: want empty, got %v", cfg.Resources.Paths)
	}
	if cfg.PageSize != 0 {
		t.Errorf("default page_size: want 0, got %d", cfg.PageSize)
	}
}

func TestLoadFile_PageSize(t *testing.T) {
	content := `
makefiles:
  - ./makefile
page_size: 50
`
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
	if cfg.PageSize != 50 {
		t.Errorf("page_size: got %d, want 50", cfg.PageSize)
	}
}

func TestMerge_PageSizeOverride(t *testing.T) {
	base := config.Config{PageSize: 25}
	override := config.Config{PageSize: 100}
	result := config.Merge(base, override)
	if result.PageSize != 100 {
		t.Errorf("merge page_size: got %d, want 100", result.PageSize)
	}
}

func TestMerge_PageSizeBasePreservedWhenOverrideZero(t *testing.T) {
	base := config.Config{PageSize: 25}
	result := config.Merge(base, config.Config{})
	if result.PageSize != 25 {
		t.Errorf("merge page_size: base not preserved, got %d", result.PageSize)
	}
}
