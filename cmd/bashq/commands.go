package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/XavierChevalier/bashq/dup"
	"github.com/XavierChevalier/bashq/internal/check"
	"github.com/XavierChevalier/bashq/internal/config"
	"github.com/XavierChevalier/bashq/internal/discover"
	"github.com/XavierChevalier/bashq/internal/report"
	"github.com/XavierChevalier/bashq/metrics"
)

// analyze discovers and measures the scripts of a command.
func analyze(c *common, cfg *config.Config, dupOpts *dup.Options) (*check.Analysis, error) {
	paths, err := discover.Scripts(c.paths, cfg.Exclude)
	if err != nil {
		return nil, err
	}
	return check.Analyze(paths, dupOpts), nil
}

func dupOptions(cfg *config.Config) *dup.Options {
	return &dup.Options{MinUnits: cfg.Duplication.MinUnits, MinLines: cfg.Duplication.MinLines}
}

func cmdMetrics(args []string, stdout, stderr io.Writer) (int, error) {
	c := newCommon("metrics", "metrics [options] [paths...]", stderr)
	format := c.fs.String("format", "text", "output `format`: "+strings.Join(report.MetricFormats, ", "))
	sortKey := c.fs.String("sort", "cognitive", "sort functions by `measure`, highest first: "+strings.Join(report.SortKeys, ", "))
	top := c.fs.Int("top", 0, "show only the first `n` functions (text and tsv; 0: all)")
	cfg, err := c.parse(args)
	if err != nil {
		return exitError, err
	}
	a, err := analyze(c, cfg, dupOptions(cfg))
	if err != nil {
		return exitError, err
	}
	rows, err := report.Rows(a, *sortKey, *top)
	if err != nil {
		return exitError, err
	}
	if err := report.Metrics(stdout, *format, a, rows); err != nil {
		return exitError, err
	}
	for _, e := range a.Errors {
		fmt.Fprintf(stderr, "bashq metrics: %s:%d: %v\n", e.Path, e.Line, e.Err)
	}
	return exitOK, nil
}

func cmdExplain(args []string, stdout, stderr io.Writer) (int, error) {
	c := newCommon("explain", "explain [options] file[:function] [function...]", stderr)
	minCost := c.fs.Int("min", 1, "without a function name, explain the functions of cognitive complexity ≥ `n`")
	if _, err := c.parse(args); err != nil {
		return exitError, err
	}
	if len(c.fs.Args()) == 0 {
		c.fs.Usage()
		return exitError, errUsage
	}
	path, names := c.paths[0], c.paths[1:]
	if p, fn, ok := strings.Cut(path, ":"); ok {
		path, names = p, append([]string{fn}, names...)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return exitError, err
	}
	file, err := metrics.Analyze(path, src)
	if err != nil {
		return exitError, err
	}

	var fns []*metrics.Function
	for _, name := range names {
		fn := file.Function(name)
		if fn == nil {
			return exitError, fmt.Errorf("%s: no function %q", path, name)
		}
		fns = append(fns, fn)
	}
	if len(names) == 0 {
		for _, fn := range append(file.Functions, file.TopLevel) {
			if fn.Cognitive >= *minCost && fn.Cognitive > 0 {
				fns = append(fns, fn)
			}
		}
	}
	style := report.StyleFor(os.Stdout)
	for i, fn := range fns {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		if fn.Name == metrics.TopLevel {
			fmt.Fprintf(stdout, "%s: code outside functions, cognitive complexity %d: %s\n", path, fn.Cognitive, joinIncrements(fn))
			continue
		}
		if err := report.Explain(stdout, path, src, fn, style); err != nil {
			return exitError, err
		}
	}
	if len(fns) == 0 {
		fmt.Fprintf(stdout, "%s: no function with a cognitive complexity of %d or more\n", path, max(*minCost, 1))
	}
	return exitOK, nil
}

func joinIncrements(fn *metrics.Function) string {
	parts := make([]string, 0, len(fn.Increments))
	for _, inc := range fn.Increments {
		parts = append(parts, fmt.Sprintf("line %d %s", inc.Line, inc))
	}
	return strings.Join(parts, ", ")
}

func cmdDuplicates(args []string, stdout, stderr io.Writer) (int, error) {
	c := newCommon("duplicates", "duplicates [options] [paths...]", stderr)
	minUnits := c.fs.Int("min-units", 0, "smallest duplicated block, in commands and keywords (default: configuration, else 15)")
	minLines := c.fs.Int("min-lines", 0, "smallest duplicated block, in lines (default: configuration, else 10)")
	cfg, err := c.parse(args)
	if err != nil {
		return exitError, err
	}
	opts := dupOptions(cfg)
	if *minUnits > 0 {
		opts.MinUnits = *minUnits
	}
	if *minLines > 0 {
		opts.MinLines = *minLines
	}
	a, err := analyze(c, cfg, opts)
	if err != nil {
		return exitError, err
	}
	for _, b := range a.Blocks {
		fmt.Fprintf(stdout, "%d lines, %d units: %s:%d-%d = %s:%d-%d\n", b.Second.Lines(), b.Units,
			b.First.Path, b.First.StartLine, b.First.EndLine, b.Second.Path, b.Second.StartLine, b.Second.EndLine)
	}
	total := 0
	for _, n := range a.DuplicatedLines {
		total += n
	}
	fmt.Fprintf(stdout, "%d duplicated %s, %d duplicated lines in %d files\n", len(a.Blocks), plural(len(a.Blocks), "block", "blocks"), total, len(a.DuplicatedLines))
	return exitOK, nil
}

func cmdRules(args []string, stdout, stderr io.Writer) (int, error) {
	c := newCommon("rules", "rules [options]", stderr)
	cfg, err := c.parse(args)
	if err != nil {
		return exitError, err
	}
	settings, err := check.Resolve(cfg)
	if err != nil {
		return exitError, err
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RULE\tSTATE\tMAX\tDESCRIPTION")
	for _, r := range check.Catalog {
		state := "on"
		if !settings.Enabled(r.Name) {
			state = "off"
		}
		limit := "-"
		if r.Kind == check.Measure {
			limit = fmt.Sprint(settings.Max(r.Name))
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Name, state, limit, r.Summary)
	}
	return exitOK, tw.Flush()
}

const initConfig = `# bashq configuration: https://github.com/XavierChevalier/bashq/blob/main/docs/configuration.md

# Paths not to analyse (globs; a name without a slash matches anywhere).
exclude:
  - vendor
  - "**/testdata"

# Thresholds. A function over a max is an issue; enabled: false turns a rule off.
rules:
  cognitive-complexity:
    max: 15
  cyclomatic-complexity:
    enabled: false
    max: 20
  function-length:
    max: 70
  nesting-depth:
    max: 4
  file-length:
    max: 1000

# Duplicated blocks: at least min-units commands and keywords over min-lines lines.
duplication:
  min-units: 15
  min-lines: 10

shellcheck:
  enabled: auto      # auto (when installed), always, never
  severity: warning  # lowest level reported: error, warning, info, style
  args: ["-x"]

# Known debt, written by "bashq baseline": these issues do not fail the check.
# baseline: .bashq-baseline.tsv
`

func cmdInit(args []string, stdout, stderr io.Writer) (int, error) {
	c := newCommon("init", "init", stderr)
	force := c.fs.Bool("force", false, "overwrite an existing "+config.FileName)
	if _, err := c.parse(args); err != nil {
		return exitError, err
	}
	if _, err := os.Stat(config.FileName); err == nil && !*force {
		return exitError, fmt.Errorf("%s already exists (--force to overwrite it)", config.FileName)
	}
	if err := os.WriteFile(config.FileName, []byte(initConfig), 0o644); err != nil {
		return exitError, err
	}
	fmt.Fprintf(stdout, "wrote %s\n", config.FileName)
	return exitOK, nil
}
