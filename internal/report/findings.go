// Package report renders findings and measures: text for people, JSON,
// SARIF, SonarQube's generic issues and GitHub annotations for tools, TSV
// and Prometheus metrics for dashboards.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/XavierChevalier/bashq/internal/check"
)

// FindingFormats are the formats `bashq check --format` accepts.
var FindingFormats = []string{"text", "json", "sarif", "sonar", "github"}

// Style colours text output.
type Style struct{ Color bool }

// StyleFor colours a terminal unless NO_COLOR is set.
func StyleFor(f *os.File) Style {
	if os.Getenv("NO_COLOR") != "" {
		return Style{}
	}
	info, err := f.Stat()
	return Style{Color: err == nil && info.Mode()&os.ModeCharDevice != 0}
}

func (s Style) wrap(code, text string) string {
	if !s.Color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// Bold, Dim, Red, Green and Yellow colour text when Color is set.
func (s Style) Bold(t string) string   { return s.wrap("1", t) }
func (s Style) Dim(t string) string    { return s.wrap("2", t) }
func (s Style) Red(t string) string    { return s.wrap("31", t) }
func (s Style) Green(t string) string  { return s.wrap("32", t) }
func (s Style) Yellow(t string) string { return s.wrap("33", t) }

// Findings renders the outcome of a check.
func Findings(w io.Writer, format string, r *check.Report, version string, style Style) error {
	switch format {
	case "text":
		return text(w, r, style)
	case "json":
		return jsonFindings(w, r, version)
	case "sarif":
		return sarif(w, r, version)
	case "sonar":
		return sonar(w, r)
	case "github":
		return github(w, r)
	}
	return fmt.Errorf("unknown format %q (want %s)", format, strings.Join(FindingFormats, ", "))
}

func text(w io.Writer, r *check.Report, s Style) error {
	for _, f := range r.New {
		sev := s.Yellow(f.Severity)
		if f.Severity == "error" {
			sev = s.Red(f.Severity)
		}
		line := fmt.Sprintf("%s:%d", f.Path, f.Line)
		if f.Col > 0 {
			line += fmt.Sprintf(":%d", f.Col)
		}
		msg := f.Message
		if f.Allowed > 0 {
			msg += fmt.Sprintf(" (baseline allows %d)", f.Allowed)
		}
		fmt.Fprintf(w, "%s %s %s %s\n", s.Bold(line), sev, msg, s.Dim("["+f.Rule+"]"))
	}
	if len(r.New) > 0 {
		fmt.Fprintln(w)
	}
	functions := 0
	for _, f := range r.Analysis.Files {
		functions += len(f.Functions)
	}
	files := len(r.Analysis.Files) + len(r.Analysis.Errors)
	summary := fmt.Sprintf("%d %s, %d %s", files, plural(files, "file", "files"), functions, plural(functions, "function", "functions"))
	if r.ShellCheck {
		summary += ", ShellCheck"
	}
	if r.Baselined > 0 {
		summary += fmt.Sprintf("; %d baselined", r.Baselined)
	}
	if len(r.New) == 0 {
		_, err := fmt.Fprintf(w, "%s No issues (%s)\n", s.Green("✓"), summary)
		return err
	}
	counts := map[string]int{}
	for _, f := range r.New {
		counts[ruleGroup(f.Rule)]++
	}
	var parts []string
	for rule, n := range counts {
		parts = append(parts, fmt.Sprintf("%s %d", rule, n))
	}
	sort.Strings(parts)
	_, err := fmt.Fprintf(w, "%s %d %s: %s (%s)\n", s.Red("✗"), len(r.New), plural(len(r.New), "issue", "issues"), strings.Join(parts, ", "), summary)
	return err
}

func ruleGroup(rule string) string {
	if check.IsShellCheck(rule) {
		return "shellcheck"
	}
	return rule
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func jsonFindings(w io.Writer, r *check.Report, version string) error {
	findings := r.New
	if findings == nil {
		findings = []check.Finding{}
	}
	out := struct {
		Version    string          `json:"version"`
		Files      int             `json:"files"`
		Functions  int             `json:"functions"`
		ShellCheck bool            `json:"shellcheck"`
		Baselined  int             `json:"baselined"`
		Findings   []check.Finding `json:"findings"`
	}{Version: version, Files: len(r.Analysis.Files) + len(r.Analysis.Errors), ShellCheck: r.ShellCheck, Baselined: r.Baselined, Findings: findings}
	for _, f := range r.Analysis.Files {
		out.Functions += len(f.Functions)
	}
	return encode(w, out)
}

func encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// github prints workflow commands: GitHub Actions turns them into
// annotations on the pull request, with no extra permission.
func github(w io.Writer, r *check.Report) error {
	for _, f := range r.New {
		level := "warning"
		switch f.Severity {
		case "error":
			level = "error"
		case "note":
			level = "notice"
		}
		props := fmt.Sprintf("file=%s,line=%d", escapeProperty(f.Path), f.Line)
		if f.EndLine > f.Line {
			props += fmt.Sprintf(",endLine=%d", f.EndLine)
		}
		if f.Col > 0 {
			props += fmt.Sprintf(",col=%d", f.Col)
		}
		props += ",title=" + escapeProperty("bashq "+f.Rule)
		if _, err := fmt.Fprintf(w, "::%s %s::%s\n", level, props, escapeData(f.Message)); err != nil {
			return err
		}
	}
	return nil
}

func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}
