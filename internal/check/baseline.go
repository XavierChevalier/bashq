package check

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// BaselineHeader starts every baseline file.
const BaselineHeader = "# bashq baseline: rule, path, function, tolerated value (measure rules) or count. Regenerate with `bashq baseline`."

// Key identifies a baseline row.
type Key struct {
	Rule     string
	Path     string
	Function string
}

// Baseline is the debt a project accepts: per rule, file and function, the
// highest value (measure rules) or the number of findings (count rules)
// that does not fail the check.
type Baseline map[Key]int

func keyOf(f Finding) Key {
	return Key{Rule: f.Rule, Path: f.Path, Function: f.Function}
}

// NewBaseline records findings as accepted debt.
func NewBaseline(findings []Finding) Baseline {
	b := Baseline{}
	for _, f := range findings {
		if f.Rule == "baseline-stale" || f.Rule == "parse-error" {
			continue
		}
		k := keyOf(f)
		if r, _ := Lookup(f.Rule); r.Kind == Measure {
			b[k] = max(b[k], f.Value)
		} else {
			b[k]++
		}
	}
	return b
}

// ReadBaseline reads a baseline file; a missing file is an empty baseline.
func ReadBaseline(path string) (Baseline, error) {
	b := Baseline{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	n := 0
	for scanner.Scan() {
		n++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			return nil, fmt.Errorf("%s:%d: want 4 tab-separated fields (rule, path, function, value), got %d", path, n, len(fields))
		}
		value, err := strconv.Atoi(fields[3])
		if err != nil || value < 1 {
			return nil, fmt.Errorf("%s:%d: value %q is not a positive integer", path, n, fields[3])
		}
		b[Key{Rule: fields[0], Path: fields[1], Function: fields[2]}] = value
	}
	return b, scanner.Err()
}

// Write writes the baseline, sorted, so that its diffs are readable.
func (b Baseline) Write(w io.Writer) error {
	keys := make([]Key, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		x, y := keys[i], keys[j]
		if x.Path != y.Path {
			return x.Path < y.Path
		}
		if x.Function != y.Function {
			return x.Function < y.Function
		}
		return x.Rule < y.Rule
	})
	if _, err := fmt.Fprintln(w, BaselineHeader); err != nil {
		return err
	}
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", k.Rule, k.Path, k.Function, b[k]); err != nil {
			return err
		}
	}
	return nil
}

// Outcome is the result of comparing findings with a baseline.
type Outcome struct {
	// New are the findings the baseline does not tolerate.
	New []Finding
	// Baselined counts the findings the baseline tolerates.
	Baselined int
}

// Apply compares findings with the baseline.
//
//   - A measure finding passes while its value is at or under its row.
//   - Count findings pass while there are at most as many as the row says,
//     for that rule in that function; if there are more, all of them are
//     reported (bashq cannot tell which one is new).
//   - With ratchet, a row that tolerates more than the code needs (the
//     value went down, the finding is gone) is a baseline-stale finding:
//     the baseline only shrinks. Rows of rules that did not run are left
//     alone, and so are rows of paths that were not analysed.
func (b Baseline) Apply(findings []Finding, s *Settings, analysed map[string]bool, ratchet bool) Outcome {
	var out Outcome
	actual := NewBaseline(findings)
	groups := map[Key][]Finding{}
	for _, f := range findings {
		groups[keyOf(f)] = append(groups[keyOf(f)], f)
	}

	for _, f := range findings {
		k := keyOf(f)
		allowed, ok := b[k]
		if ok && actual[k] <= allowed {
			out.Baselined++
			continue
		}
		if ok {
			f.Allowed = allowed
		}
		out.New = append(out.New, f)
	}

	if ratchet {
		for k, allowed := range b {
			if !s.Enabled(k.Rule) || !analysed[k.Path] || actual[k] >= allowed {
				continue
			}
			msg := fmt.Sprintf("baseline tolerates %s at %d, the code is now at %d: lower the row (bashq baseline)", k.Rule, allowed, actual[k])
			if actual[k] == 0 {
				msg = fmt.Sprintf("baseline tolerates %s, the code no longer needs it: remove the row (bashq baseline)", k.Rule)
			}
			line := 1
			if g := groups[k]; len(g) > 0 {
				line = g[0].Line
			}
			out.New = append(out.New, Finding{
				Rule: "baseline-stale", Location: Location{Path: k.Path, Line: line}, Function: k.Function,
				Value: 1, Severity: "error", Message: fmt.Sprintf("%s: %s", functionLabel(k.Function), msg),
			})
		}
	}
	Sort(out.New)
	return out
}

func functionLabel(name string) string {
	if name == "" {
		return "file"
	}
	return name
}
