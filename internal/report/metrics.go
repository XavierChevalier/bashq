package report

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/XavierChevalier/bashq/internal/check"
	"github.com/XavierChevalier/bashq/metrics"
)

// MetricFormats are the formats `bashq metrics --format` accepts.
var MetricFormats = []string{"text", "json", "tsv", "prometheus"}

// SortKeys are the measures `bashq metrics --sort` accepts.
var SortKeys = []string{"cognitive", "cyclomatic", "lines", "nesting", "name"}

// Row is one function of one file.
type Row struct {
	Path string
	*metrics.Function
}

// Rows lists every function (and every top level with code), sorted by key,
// highest first, and keeps the first top (all when top is 0).
func Rows(a *check.Analysis, key string, top int) ([]Row, error) {
	var rows []Row
	for _, f := range a.Files {
		for _, fn := range f.Functions {
			rows = append(rows, Row{f.Path, fn})
		}
		if f.TopLevel.Cognitive > 0 || len(f.Functions) == 0 {
			rows = append(rows, Row{f.Path, f.TopLevel})
		}
	}
	measure := map[string]func(Row) int{
		"cognitive":  func(r Row) int { return r.Cognitive },
		"cyclomatic": func(r Row) int { return r.Cyclomatic },
		"lines":      func(r Row) int { return r.Lines },
		"nesting":    func(r Row) int { return r.MaxNesting },
	}
	if key == "name" {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Path != rows[j].Path {
				return rows[i].Path < rows[j].Path
			}
			return rows[i].Line < rows[j].Line
		})
	} else if m, ok := measure[key]; ok {
		sort.SliceStable(rows, func(i, j int) bool { return m(rows[i]) > m(rows[j]) })
	} else {
		return nil, fmt.Errorf("unknown sort key %q (want %s)", key, strings.Join(SortKeys, ", "))
	}
	if top > 0 && len(rows) > top {
		rows = rows[:top]
	}
	return rows, nil
}

// Metrics renders the measures of an analysis.
func Metrics(w io.Writer, format string, a *check.Analysis, rows []Row) error {
	switch format {
	case "text":
		return metricsText(w, a, rows)
	case "json":
		return metricsJSON(w, a)
	case "tsv":
		return metricsTSV(w, rows)
	case "prometheus":
		return prometheus(w, a)
	}
	return fmt.Errorf("unknown format %q (want %s)", format, strings.Join(MetricFormats, ", "))
}

func metricsText(w io.Writer, a *check.Analysis, rows []Row) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	// No colour in the table: escape codes would break tabwriter's widths.
	fmt.Fprintln(tw, "cognitive\tcyclomatic\tlines\tnesting\t\t")
	for _, r := range rows {
		fmt.Fprintf(tw, "%d\t%d\t%d\t%d\t\t%s %s:%d\n", r.Cognitive, r.Cyclomatic, r.Lines, r.MaxNesting, r.Name, r.Path, r.Line)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	var lines, code, functions, cognitive, duplicated int
	for _, f := range a.Files {
		lines += f.Lines
		code += f.CodeLines
		functions += len(f.Functions)
		cognitive += f.Cognitive
	}
	for _, n := range a.DuplicatedLines {
		duplicated += n
	}
	density := 0.0
	if lines > 0 {
		density = 100 * float64(duplicated) / float64(lines)
	}
	_, err := fmt.Fprintf(w, "\n%d files, %d lines (%d of code), %d functions, cognitive complexity %d, %d duplicated lines (%.1f %%)\n",
		len(a.Files), lines, code, functions, cognitive, duplicated, density)
	return err
}

func metricsJSON(w io.Writer, a *check.Analysis) error {
	type file struct {
		*metrics.File
		DuplicatedLines int `json:"duplicated_lines"`
	}
	type fileError struct {
		Path  string `json:"path"`
		Line  int    `json:"line,omitempty"`
		Error string `json:"error"`
	}
	out := struct {
		Files      []file      `json:"files"`
		Errors     []fileError `json:"errors"`
		Duplicates any         `json:"duplicates"`
	}{Files: []file{}, Errors: []fileError{}, Duplicates: a.Blocks}
	if a.Blocks == nil {
		out.Duplicates = []any{}
	}
	for _, f := range a.Files {
		out.Files = append(out.Files, file{f, a.DuplicatedLines[f.Path]})
	}
	for _, e := range a.Errors {
		out.Errors = append(out.Errors, fileError{e.Path, e.Line, e.Err.Error()})
	}
	return encode(w, out)
}

func metricsTSV(w io.Writer, rows []Row) error {
	if _, err := fmt.Fprintln(w, "path\tline\tfunction\tcognitive\tcyclomatic\tlines\tnesting"); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "%s\t%d\t%s\t%d\t%d\t%d\t%d\n", r.Path, r.Line, r.Name, r.Cognitive, r.Cyclomatic, r.Lines, r.MaxNesting); err != nil {
			return err
		}
	}
	return nil
}

// prometheus writes the Prometheus text exposition format: a node-exporter
// textfile, a Pushgateway body, or anything that scrapes it.
func prometheus(w io.Writer, a *check.Analysis) error {
	type sample struct {
		labels string
		value  int
	}
	families := []struct {
		name, help string
		samples    []sample
	}{
		{name: "bashq_function_cognitive_complexity", help: "Cognitive complexity of a bash function."},
		{name: "bashq_function_cyclomatic_complexity", help: "Cyclomatic complexity of a bash function."},
		{name: "bashq_function_lines", help: "Lines of a bash function."},
		{name: "bashq_function_max_nesting", help: "Deepest nesting of control structures in a bash function."},
		{name: "bashq_file_lines", help: "Lines of a script."},
		{name: "bashq_file_code_lines", help: "Lines of code of a script (neither blank nor comment)."},
		{name: "bashq_file_cognitive_complexity", help: "Cognitive complexity of a script, all functions included."},
		{name: "bashq_file_duplicated_lines", help: "Lines of a script that belong to a duplicated block."},
		{name: "bashq_file_parse_errors", help: "1 when a script cannot be parsed."},
	}
	for _, f := range a.Files {
		path := "path=" + label(f.Path)
		for _, fn := range append(f.Functions, f.TopLevel) {
			labels := path + ",function=" + label(fn.Name)
			families[0].samples = append(families[0].samples, sample{labels, fn.Cognitive})
			families[1].samples = append(families[1].samples, sample{labels, fn.Cyclomatic})
			families[2].samples = append(families[2].samples, sample{labels, fn.Lines})
			families[3].samples = append(families[3].samples, sample{labels, fn.MaxNesting})
		}
		families[4].samples = append(families[4].samples, sample{path, f.Lines})
		families[5].samples = append(families[5].samples, sample{path, f.CodeLines})
		families[6].samples = append(families[6].samples, sample{path, f.Cognitive})
		families[7].samples = append(families[7].samples, sample{path, a.DuplicatedLines[f.Path]})
		families[8].samples = append(families[8].samples, sample{path, 0})
	}
	for _, e := range a.Errors {
		families[8].samples = append(families[8].samples, sample{"path=" + label(e.Path), 1})
	}
	for _, fam := range families {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", fam.name, fam.help, fam.name)
		for _, s := range fam.samples {
			if _, err := fmt.Fprintf(w, "%s{%s} %d\n", fam.name, s.labels, s.value); err != nil {
				return err
			}
		}
	}
	return nil
}

// label quotes a Prometheus label value: only \, " and newlines are escaped.
func label(v string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v) + `"`
}

// Explain prints a function's source with the cost of each line in the
// margin, the way SonarQube annotates cognitive complexity.
func Explain(w io.Writer, path string, src []byte, fn *metrics.Function, s Style) error {
	lines := strings.Split(string(src), "\n")
	perLine := map[int][]metrics.Increment{}
	for _, inc := range fn.Increments {
		perLine[inc.Line] = append(perLine[inc.Line], inc)
	}
	fmt.Fprintf(w, "%s %s: cognitive complexity %s, cyclomatic %d, %d lines, nesting %d\n\n",
		s.Bold(fn.Name), s.Dim(fmt.Sprintf("%s:%d", path, fn.Line)), s.Bold(strconv.Itoa(fn.Cognitive)), fn.Cyclomatic, fn.Lines, fn.MaxNesting)
	for n := fn.Line; n <= fn.EndLine && n <= len(lines); n++ {
		incs := perLine[n]
		cost, notes := 0, make([]string, 0, len(incs))
		for _, inc := range incs {
			cost += inc.Cost
			notes = append(notes, inc.String())
		}
		gutter := "   "
		if cost > 0 {
			gutter = fmt.Sprintf("+%-2d", cost)
		}
		line := fmt.Sprintf("%4d %s │ %s", n, s.Yellow(gutter), lines[n-1])
		if len(notes) > 0 {
			line += "  " + s.Dim("# "+strings.Join(notes, ", "))
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
