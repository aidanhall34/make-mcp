package config

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultDelimiter is the annotation delimiter used when none is specified.
const DefaultDelimiter = "@"

const (
	// DefaultTransport is the transport used when none is specified.
	DefaultTransport = "stdio"

	// DefaultListenAddress is the HTTP bind address when HTTP transport is enabled.
	DefaultListenAddress = "127.0.0.1:9378"
)

// Timeouts holds the per-risk execution timeouts for tool invocations.
type Timeouts struct {
	Low    time.Duration `yaml:"low"`
	Medium time.Duration `yaml:"medium"`
	High   time.Duration `yaml:"high"`
}

// TLSConfig holds TLS certificate paths for the HTTP server.
type TLSConfig struct {
	// Cert is the path to the PEM-encoded server certificate.
	Cert string `yaml:"cert"`
	// Key is the path to the PEM-encoded server private key.
	Key string `yaml:"key"`
	// CA is an optional path to a PEM-encoded CA certificate used to trust
	// self-signed certificates (e.g. a local Keycloak instance).
	CA string `yaml:"ca"`
}

// ResourcePath describes a named set of files, directories, or globs to expose
// as MCP resources.
type ResourcePath struct {
	// Name identifies this path group and is used when mapping OAuth groups to
	// resource access.
	Name string `yaml:"name"`
	// Description is an optional human-readable description applied to all
	// resources discovered by this path group.
	Description string `yaml:"description"`
	// Path is the list of file paths, directory paths, or glob patterns whose
	// matching files are exposed as resources.
	Path []string `yaml:"path"`
	// Recursive controls whether directory entries are expanded recursively. A
	// nil value inherits the top-level ResourcesConfig.Recursive default.
	Recursive *bool `yaml:"recursive"`
}

// ResourcesConfig controls which files are exposed as MCP resources.
type ResourcesConfig struct {
	// Recursive is the top-level default for whether directories are traversed
	// recursively. Individual ResourcePath entries may override this.
	Recursive bool `yaml:"recursive"`
	// Paths is the ordered list of named resource path groups.
	Paths []ResourcePath `yaml:"paths"`
}

// OAuthGroupBinding maps an OAuth/OIDC group name to the resource path groups
// that members of that group are permitted to read.
type OAuthGroupBinding struct {
	// Name is the OAuth/OIDC group or role claim value (e.g. "/admins").
	Name string `yaml:"name"`
	// Resources is the list of ResourcePath.Name values accessible to this group.
	Resources []string `yaml:"resources"`
}

// OAuthConfig configures OAuth 2.1 Bearer token validation for the HTTP server.
// OAuth is only supported with HTTP transport (not stdio).
type OAuthConfig struct {
	// Enabled activates Bearer token validation on the /mcp endpoint.
	Enabled bool `yaml:"enabled"`
	// Issuer is the expected "iss" claim in incoming JWTs.
	Issuer string `yaml:"issuer"`
	// Audience is the expected "aud" claim in incoming JWTs.
	Audience string `yaml:"audience"`
	// JWKSURI is the URL of the JSON Web Key Set used to validate token signatures.
	JWKSURI string `yaml:"jwks_uri"`
	// TLS configures the server certificate/key for HTTPS (required when OAuth
	// is enabled) and an optional CA for trusting the JWKS endpoint.
	TLS TLSConfig `yaml:"tls"`
	// Groups maps OAuth group/role names to the resource path groups they may read.
	Groups []OAuthGroupBinding `yaml:"groups"`
}

// Config holds all make-mcp configuration. It is populated from make-mcp.yml
// and/or CLI flags; CLI flags take precedence over the file.
type Config struct {
	// Delimiter is the literal string that must appear between the # comment
	// character and an annotation key. Default: "@".
	Delimiter string `yaml:"delimiter"`

	// Makefiles is the ordered list of Makefile paths to parse.
	Makefiles []string `yaml:"makefiles"`

	// Strict requires every supported annotation, including optional MCP tool hints.
	Strict bool `yaml:"strict"`

	// Transport selects which MCP transport(s) to enable: stdio, http, or both.
	Transport string `yaml:"transport"`

	// Listen is the HTTP bind address when HTTP transport is enabled.
	Listen string `yaml:"listen"`

	// LogPath is the destination for JSON logs. Defaults to stderr.
	LogPath string `yaml:"log_path"`

	// Debug enables debug-level logging. When true, tool call arguments,
	// stdout, and stderr are included in log output.
	Debug bool `yaml:"debug"`

	// Timeouts configures per-risk execution timeouts.
	Timeouts Timeouts `yaml:"timeouts"`

	// PageSize is the maximum number of items returned per page in tools/list
	// and resources/list responses. Zero disables pagination (all items returned).
	PageSize int `yaml:"page_size"`

	// Resources configures which files are exposed as MCP resources.
	Resources ResourcesConfig `yaml:"resources"`

	// OAuth configures OAuth 2.1 Bearer token validation (HTTP transport only).
	OAuth OAuthConfig `yaml:"oauth"`
}

// Default returns a Config populated with default values.
func Default() Config {
	return Config{
		Delimiter: DefaultDelimiter,
		Transport: DefaultTransport,
		Listen:    DefaultListenAddress,
		LogPath:   "stderr",
		Timeouts: Timeouts{
			Low:    10 * time.Minute,
			Medium: 10 * time.Minute,
			High:   10 * time.Minute,
		},
	}
}

// LoadFile reads a YAML config file at path and unmarshals it into a Config.
// Unknown keys cause an error.
func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// Merge returns a new Config where non-zero fields in override replace those
// in base. Use this to layer CLI flags on top of a file-loaded config.
func Merge(base, override Config) Config {
	out := base
	if override.Delimiter != "" {
		out.Delimiter = override.Delimiter
	}
	if len(override.Makefiles) > 0 {
		out.Makefiles = override.Makefiles
	}
	if override.Strict {
		out.Strict = true
	}
	if override.Transport != "" {
		out.Transport = override.Transport
	}
	if override.Listen != "" {
		out.Listen = override.Listen
	}
	if override.LogPath != "" {
		out.LogPath = override.LogPath
	}
	if override.Debug {
		out.Debug = true
	}
	if override.Timeouts.Low != 0 {
		out.Timeouts.Low = override.Timeouts.Low
	}
	if override.Timeouts.Medium != 0 {
		out.Timeouts.Medium = override.Timeouts.Medium
	}
	if override.Timeouts.High != 0 {
		out.Timeouts.High = override.Timeouts.High
	}
	if override.PageSize != 0 {
		out.PageSize = override.PageSize
	}
	if len(override.Resources.Paths) > 0 {
		out.Resources = override.Resources
	}
	if override.OAuth.Enabled {
		out.OAuth = override.OAuth
	}
	return out
}
