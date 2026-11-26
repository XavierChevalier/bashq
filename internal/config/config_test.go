package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(write(t, dir, `
exclude: [vendor]
rules:
  function-length: {max: 50}
  cyclomatic-complexity: {enabled: true}
duplication: {min-units: 20, min-lines: 12}
shellcheck: {enabled: never, args: ["-x"]}
baseline: quality/baseline.tsv
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Rules["function-length"].Max != 50 || !*c.Rules["cyclomatic-complexity"].Enabled {
		t.Errorf("rules = %+v", c.Rules)
	}
	if c.Duplication.MinUnits != 20 || c.ShellCheck.Enabled != "never" || c.ShellCheck.Severity != "warning" {
		t.Errorf("config = %+v", c)
	}
	if want := filepath.Join(dir, "quality", "baseline.tsv"); c.BaselinePath() != want {
		t.Errorf("baseline = %s, want %s: relative to the configuration file", c.BaselinePath(), want)
	}
}

func TestEmptyFileIsTheDefaults(t *testing.T) {
	c, err := Load(write(t, t.TempDir(), ""))
	if err != nil || c.ShellCheck.Enabled != "auto" || c.Rules == nil {
		t.Fatalf("config = %+v, err = %v", c, err)
	}
}

func TestErrors(t *testing.T) {
	cases := map[string]string{
		"rulez: {}\n":                        "field rulez not found",
		"shellcheck: {enabled: sometimes}\n": "shellcheck.enabled",
		"shellcheck: {severity: loud}\n":     "shellcheck.severity",
		"exclude: vendor: x\n":               "yaml",
	}
	for content, want := range cases {
		_, err := Load(write(t, t.TempDir(), content))
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), FileName) {
			t.Errorf("%q: err = %v, want %q and the file name", content, err, want)
		}
	}
}

func TestFindWalksUp(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "baseline: b.tsv\n")
	sub := filepath.Join(dir, "a", "b")
	os.MkdirAll(sub, 0o755)
	c, err := Find(sub)
	if err != nil || c.Baseline != "b.tsv" {
		t.Fatalf("config = %+v, err = %v", c, err)
	}
	c, err = Find(t.TempDir())
	if err != nil || c.Dir != "" || c.ShellCheck.Enabled != "auto" {
		t.Errorf("no file: config = %+v, err = %v", c, err)
	}
}
