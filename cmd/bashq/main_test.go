package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const testConfig = "cmd/bashq/testdata/bashq.yaml"

// root is the repository root: the tests run from there.
var root, _ = filepath.Abs("../..")

// bashq runs the CLI from the repository root, on examples/deploy.
func bashq(t *testing.T, args ...string) (string, int) {
	t.Helper()
	t.Chdir(root)
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	if stderr.Len() > 0 && code != exitError {
		t.Logf("stderr: %s", stderr.String())
	}
	return stdout.String() + stderr.String(), code
}

// golden compares out with testdata/<name>.golden; -update rewrites it.
func golden(t *testing.T, name, out string) {
	t.Helper()
	path := filepath.Join("cmd", "bashq", "testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (go test ./cmd/bashq -update writes it)", err)
	}
	if out != string(want) {
		t.Errorf("output differs from %s (go test ./cmd/bashq -update rewrites it)\n--- got\n%s--- want\n%s", path, out, want)
	}
}

func TestGolden(t *testing.T) {
	cases := []struct {
		name string
		args []string
		code int
	}{
		{"check", []string{"check", "--config", testConfig, "examples/deploy"}, exitFindings},
		{"check-github", []string{"check", "--config", testConfig, "--format", "github", "examples/deploy"}, exitFindings},
		{"check-max", []string{"check", "--config", testConfig, "--max", "cognitive-complexity=20", "--rule", "cognitive-complexity", "examples/deploy"}, exitOK},
		{"metrics", []string{"metrics", "--config", testConfig, "examples/deploy"}, exitOK},
		{"metrics-tsv", []string{"metrics", "--config", testConfig, "--format", "tsv", "--sort", "name", "examples/deploy"}, exitOK},
		{"metrics-prometheus", []string{"metrics", "--config", testConfig, "--format", "prometheus", "examples/deploy/rollback.sh"}, exitOK},
		{"metrics-json", []string{"metrics", "--config", testConfig, "--format", "json", "examples/deploy/rollback.sh"}, exitOK},
		{"explain", []string{"explain", "examples/deploy/deploy.sh:deploy::main"}, exitOK},
		{"explain-file", []string{"explain", "--min", "2", "examples/deploy/deploy.sh"}, exitOK},
		{"duplicates", []string{"duplicates", "--config", testConfig, "examples/deploy"}, exitOK},
		{"rules", []string{"rules", "--config", testConfig}, exitOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := bashq(t, tc.args...)
			if code != tc.code {
				t.Errorf("exit code = %d, want %d\n%s", code, tc.code, out)
			}
			golden(t, tc.name, out)
		})
	}
}

func TestCheckJSONFormats(t *testing.T) {
	for _, format := range []string{"json", "sarif", "sonar"} {
		t.Run(format, func(t *testing.T) {
			out, code := bashq(t, "check", "--config", testConfig, "--format", format, "examples/deploy")
			if code != exitFindings {
				t.Fatalf("exit code = %d\n%s", code, out)
			}
			var doc map[string]any
			if err := json.Unmarshal([]byte(out), &doc); err != nil {
				t.Fatalf("not JSON: %v\n%s", err, out)
			}
			golden(t, "check-"+format, out)
		})
	}
}

func TestBaselineFlow(t *testing.T) {
	dir := t.TempDir()
	baseline := filepath.Join(dir, "baseline.tsv")
	out, code := bashq(t, "baseline", "--config", testConfig, "--baseline", baseline, "examples/deploy")
	if code != exitOK || !strings.Contains(out, "2 rows for 2 issues") {
		t.Fatalf("baseline: %d %s", code, out)
	}
	out, code = bashq(t, "check", "--config", testConfig, "--baseline", baseline, "examples/deploy")
	if code != exitOK || !strings.Contains(out, "2 baselined") {
		t.Errorf("check with baseline: %d %s", code, out)
	}
	out, code = bashq(t, "check", "--config", testConfig, "--baseline", baseline, "--no-baseline", "examples/deploy")
	if code != exitFindings {
		t.Errorf("--no-baseline: %d %s", code, out)
	}
	// A lower threshold does not make rows stale: the value is what counts.
	out, code = bashq(t, "check", "--config", testConfig, "--baseline", baseline, "--max", "cognitive-complexity=5", "examples/deploy")
	if code != exitOK {
		t.Errorf("lower max: %d %s", code, out)
	}
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"init"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("init: %d %s", code, stderr.String())
	}
	// The configuration it writes is valid and changes nothing.
	if code := run([]string{"rules"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("rules with the init configuration: %d %s", code, stderr.String())
	}
	if code := run([]string{"init"}, &stdout, &stderr); code != exitError || !strings.Contains(stderr.String(), "already exists") {
		t.Errorf("second init: %d %s", code, stderr.String())
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{nil, "Usage:"},
		{[]string{"nope"}, `unknown command "nope"`},
		{[]string{"check", "--format", "xml", "--config", testConfig, "examples/deploy"}, `unknown format "xml"`},
		{[]string{"check", "--rule", "nope", "--config", testConfig}, `unknown rule "nope"`},
		{[]string{"check", "--max", "cognitive-complexity", "--config", testConfig}, "want rule=N"},
		{[]string{"check", "--shellcheck", "maybe", "--config", testConfig}, "shellcheck.enabled"},
		{[]string{"check", "--config", testConfig, "does/not/exist"}, "no such file"},
		{[]string{"explain"}, "Usage:"},
		{[]string{"explain", "examples/deploy/deploy.sh:nope"}, `no function "nope"`},
		{[]string{"metrics", "--sort", "size", "--config", testConfig, "examples/deploy"}, `unknown sort key "size"`},
		{[]string{"check", "--bogus"}, "flag provided but not defined"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			all, code := bashq(t, tc.args...)
			if code != exitError || !strings.Contains(all, tc.want) {
				t.Errorf("exit %d, output:\n%s\nwant exit 2 and %q", code, all, tc.want)
			}
		})
	}
}

func TestVersionAndHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"version"}, &stdout, &stderr); code != exitOK || !strings.HasPrefix(stdout.String(), "bashq ") {
		t.Errorf("version: %d %q", code, stdout.String())
	}
	stdout.Reset()
	if code := run([]string{"--help"}, &stdout, &stderr); code != exitOK || !strings.Contains(stdout.String(), "Commands:") {
		t.Errorf("help: %d %q", code, stdout.String())
	}
	if code := run([]string{"check", "-h"}, &stdout, &stderr); code != exitOK {
		t.Errorf("check -h: %d", code)
	}
}
