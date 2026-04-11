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

	// Timeouts configures per-risk execution timeouts.
	Timeouts Timeouts `yaml:"timeouts"`
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
	if override.Timeouts.Low != 0 {
		out.Timeouts.Low = override.Timeouts.Low
	}
	if override.Timeouts.Medium != 0 {
		out.Timeouts.Medium = override.Timeouts.Medium
	}
	if override.Timeouts.High != 0 {
		out.Timeouts.High = override.Timeouts.High
	}
	return out
}
