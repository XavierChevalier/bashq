// Package check turns measures into findings: rules with thresholds, the
// baseline that tolerates known debt, and ShellCheck's findings merged in.
package check

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/XavierChevalier/bashq/internal/config"
	"github.com/XavierChevalier/bashq/internal/discover"
)

// Kind tells how a rule's findings compare with a baseline.
type Kind int

const (
	// Measure rules report a value over a threshold. A baseline row holds
	// the value tolerated; the finding passes while the value stays at or
	// under it.
	Measure Kind = iota
	// Count rules report occurrences. A baseline row holds how many are
	// tolerated in one function. A count rule may have a max too:
	// line-length counts the lines wider than it.
	Count
)

// Rule describes a rule.
type Rule struct {
	Name           string
	Summary        string
	Kind           Kind
	Severity       string // error, warning or note (SARIF levels)
	DefaultEnabled bool
	// DefaultMax is the threshold of a rule that has one, 0 otherwise.
	DefaultMax int
}

// Catalog lists every rule, in the order `bashq rules` shows them.
var Catalog = []Rule{
	{Name: "cognitive-complexity", Summary: "a function is hard to understand: too many branches, nested too deep", Kind: Measure, Severity: "warning", DefaultEnabled: true, DefaultMax: 15},
	{Name: "cyclomatic-complexity", Summary: "a function has too many paths to test", Kind: Measure, Severity: "warning", DefaultEnabled: false, DefaultMax: 20},
	{Name: "function-length", Summary: "a function is too long to fit on a screen", Kind: Measure, Severity: "warning", DefaultEnabled: true, DefaultMax: 70},
	{Name: "nesting-depth", Summary: "control structures are nested too deep", Kind: Measure, Severity: "warning", DefaultEnabled: true, DefaultMax: 4},
	{Name: "file-length", Summary: "a file is too long", Kind: Measure, Severity: "warning", DefaultEnabled: true, DefaultMax: 1000},
	{Name: "line-length", Summary: "a line is too wide to read without scrolling", Kind: Count, Severity: "warning", DefaultEnabled: true, DefaultMax: 100},
	{Name: "unbounded-loop", Summary: "a loop has no upper bound: while true, while :", Kind: Count, Severity: "warning", DefaultEnabled: true},
	{Name: "duplication", Summary: "a block of code is a copy of another one", Kind: Count, Severity: "warning", DefaultEnabled: true},
	{Name: "shellcheck", Summary: "ShellCheck findings, one rule per SC code", Kind: Count, Severity: "warning", DefaultEnabled: true},
	{Name: "parse-error", Summary: "a file is not valid bash", Kind: Count, Severity: "error", DefaultEnabled: true},
	{Name: "baseline-stale", Summary: "a baseline row tolerates more than the code needs: lower it", Kind: Count, Severity: "error", DefaultEnabled: true},
}

// Lookup returns the rule named name. ShellCheck codes (SC2086) are the
// shellcheck rule.
func Lookup(name string) (Rule, bool) {
	if IsShellCheck(name) {
		name = "shellcheck"
	}
	for _, r := range Catalog {
		if r.Name == name {
			return r, true
		}
	}
	return Rule{}, false
}

// IsShellCheck reports whether a rule name is a ShellCheck code.
func IsShellCheck(name string) bool {
	return strings.HasPrefix(name, "SC") && len(name) > 2
}

// Settings are the rules as configured: enabled or not, and thresholds.
type Settings struct {
	enabled map[string]bool
	max     map[string]int
	exclude map[string][]*regexp.Regexp
}

// Resolve applies a configuration to the catalog defaults.
func Resolve(cfg *config.Config) (*Settings, error) {
	s := &Settings{enabled: map[string]bool{}, max: map[string]int{}, exclude: map[string][]*regexp.Regexp{}}
	for _, r := range Catalog {
		s.enabled[r.Name] = r.DefaultEnabled
		s.max[r.Name] = r.DefaultMax
	}
	names := make([]string, 0, len(cfg.Rules))
	for name := range cfg.Rules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rc := cfg.Rules[name]
		r, ok := Lookup(name)
		if !ok || r.Name != name {
			return nil, fmt.Errorf("unknown rule %q in the configuration (bashq rules lists them)", name)
		}
		if rc.Enabled != nil {
			s.enabled[name] = *rc.Enabled
		}
		for _, g := range rc.Exclude {
			s.exclude[name] = append(s.exclude[name], discover.Glob(g))
		}
		if rc.Max < 0 {
			return nil, fmt.Errorf("rule %s: max is %d, want a positive number", name, rc.Max)
		}
		if rc.Max > 0 {
			if r.DefaultMax == 0 {
				return nil, fmt.Errorf("rule %s has no max", name)
			}
			s.max[name] = rc.Max
		}
	}
	return s, nil
}

// Enabled reports whether a rule runs.
func (s *Settings) Enabled(name string) bool {
	if IsShellCheck(name) {
		name = "shellcheck"
	}
	return s.enabled[name]
}

// Excluded reports whether a rule's exclude globs match path.
func (s *Settings) Excluded(rule, path string) bool {
	if IsShellCheck(rule) {
		rule = "shellcheck"
	}
	path = filepath.ToSlash(filepath.Clean(path))
	for _, g := range s.exclude[rule] {
		if g.MatchString(path) {
			return true
		}
	}
	return false
}

// Max returns the threshold of a rule that has one.
func (s *Settings) Max(name string) int { return s.max[name] }

// Only runs the named rules, enabled by default or not, and disables the
// others. parse-error and baseline-stale always run.
func (s *Settings) Only(names []string) error {
	keep := map[string]bool{}
	for _, n := range names {
		if _, ok := Lookup(n); !ok {
			return fmt.Errorf("unknown rule %q (bashq rules lists them)", n)
		}
		if IsShellCheck(n) {
			// ShellCheck runs as a whole; filter codes with shellcheck.args.
			n = "shellcheck"
		}
		keep[n] = true
	}
	for name := range s.enabled {
		s.enabled[name] = keep[name] || name == "parse-error" || name == "baseline-stale"
	}
	return nil
}

// Disable turns a rule off, e.g. shellcheck when the binary is missing.
func (s *Settings) Disable(name string) { s.enabled[name] = false }
