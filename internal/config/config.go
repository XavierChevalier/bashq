// Package config reads .bashq.yaml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the configuration file bashq looks for in the current
// directory and its parents.
const FileName = ".bashq.yaml"

// Config is the content of .bashq.yaml.
type Config struct {
	// Exclude lists globs of paths not to analyse (see docs/configuration.md).
	Exclude []string `yaml:"exclude"`
	// Rules enables, disables and tunes the rules, by name.
	Rules map[string]Rule `yaml:"rules"`
	// Duplication tunes the duplicated-code detection.
	Duplication Duplication `yaml:"duplication"`
	// ShellCheck tunes the ShellCheck integration.
	ShellCheck ShellCheck `yaml:"shellcheck"`
	// Baseline is the baseline file, relative to the configuration file.
	Baseline string `yaml:"baseline"`

	// Dir is the directory of the configuration file ("" when none was
	// found): relative paths in it are relative to Dir.
	Dir string `yaml:"-"`
	// path is the configuration file, for error messages.
	path string
}

// Rule configures one rule.
type Rule struct {
	// Enabled turns the rule on or off; nil keeps its default.
	Enabled *bool `yaml:"enabled"`
	// Max is the threshold of a measure rule; 0 keeps the default.
	Max int `yaml:"max"`
}

// Duplication configures the duplicated-code detection.
type Duplication struct {
	MinUnits int `yaml:"min-units"`
	MinLines int `yaml:"min-lines"`
}

// ShellCheck configures the ShellCheck integration.
type ShellCheck struct {
	// Enabled is "auto" (when shellcheck is on the PATH), "always" (fail
	// without it) or "never".
	Enabled string `yaml:"enabled"`
	// Severity is the lowest ShellCheck level reported: error, warning,
	// info or style.
	Severity string `yaml:"severity"`
	// Args are passed to shellcheck as they are (e.g. ["-x", "-e", "SC1091"]).
	Args []string `yaml:"args"`
	// Path is the shellcheck binary; default "shellcheck" from the PATH.
	Path string `yaml:"path"`
}

// Default returns the configuration used without a .bashq.yaml.
func Default() *Config {
	c := &Config{}
	c.applyDefaults()
	return c
}

func (c *Config) applyDefaults() {
	if c.Rules == nil {
		c.Rules = map[string]Rule{}
	}
	if c.ShellCheck.Enabled == "" {
		c.ShellCheck.Enabled = "auto"
	}
	if c.ShellCheck.Severity == "" {
		c.ShellCheck.Severity = "warning"
	}
	if c.ShellCheck.Path == "" {
		c.ShellCheck.Path = "shellcheck"
	}
}

// Load reads the configuration file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	// An empty file is a valid configuration: all defaults.
	if err := decoder.Decode(c); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	c.Dir = filepath.Dir(abs)
	c.path = path
	c.applyDefaults()
	return c, c.Validate()
}

// Find returns the configuration of the closest .bashq.yaml in dir or its
// parents, or the default configuration when there is none.
func Find(dir string) (*Config, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for {
		path := filepath.Join(abs, FileName)
		if _, err := os.Stat(path); err == nil {
			return Load(path)
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return Default(), nil
		}
		abs = parent
	}
}

// Validate checks the values bashq cannot check while decoding.
func (c *Config) Validate() error {
	path := c.path
	if path == "" {
		path = "configuration"
	}
	switch c.ShellCheck.Enabled {
	case "auto", "always", "never":
	default:
		return fmt.Errorf("%s: shellcheck.enabled is %q, want auto, always or never", path, c.ShellCheck.Enabled)
	}
	switch c.ShellCheck.Severity {
	case "error", "warning", "info", "style":
	default:
		return fmt.Errorf("%s: shellcheck.severity is %q, want error, warning, info or style", path, c.ShellCheck.Severity)
	}
	return nil
}

// BaselinePath returns the baseline file, absolute, or "".
func (c *Config) BaselinePath() string {
	if c.Baseline == "" || filepath.IsAbs(c.Baseline) || c.Dir == "" {
		return c.Baseline
	}
	return filepath.Join(c.Dir, c.Baseline)
}
