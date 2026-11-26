// Command bashq measures and checks the quality of bash scripts: cognitive
// and cyclomatic complexity, length, nesting, duplicated code and
// ShellCheck findings, behind a quality gate with a ratcheting baseline.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/XavierChevalier/bashq/internal/check"
	"github.com/XavierChevalier/bashq/internal/config"
	"github.com/XavierChevalier/bashq/internal/report"
)

// version is set at release time (-ldflags "-X main.version=1.2.3").
var version = ""

// Exit codes.
const (
	exitOK       = 0
	exitFindings = 1
	exitError    = 2
)

// errUsage marks errors that are the caller's: the usage is printed.
var errUsage = errors.New("usage")

const usage = `bashq measures and checks the quality of bash scripts.

Usage:
  bashq <command> [options] [paths...]

Commands:
  check       apply the rules and fail on new issues (the quality gate)
  metrics     measure every function: complexity, length, nesting
  explain     show where a function's cognitive complexity comes from
  duplicates  list duplicated blocks of code
  baseline    accept the current issues as known debt
  rules       list the rules and their settings
  init        write a .bashq.yaml with the defaults
  version     print the version

Paths default to the current directory. Run "bashq <command> -h" for the
options of a command. Documentation: https://github.com/XavierChevalier/bashq
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitError
	}
	commands := map[string]func([]string, io.Writer, io.Writer) (int, error){
		"check":      cmdCheck,
		"metrics":    cmdMetrics,
		"explain":    cmdExplain,
		"duplicates": cmdDuplicates,
		"baseline":   cmdBaseline,
		"rules":      cmdRules,
		"init":       cmdInit,
		"version": func([]string, io.Writer, io.Writer) (int, error) {
			fmt.Fprintln(stdout, "bashq", versionString())
			return exitOK, nil
		},
	}
	name := args[0]
	switch name {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return exitOK
	case "-v", "--version":
		name = "version"
	}
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(stderr, "bashq: unknown command %q\n\n%s", name, usage)
		return exitError
	}
	code, err := cmd(args[1:], stdout, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err != nil {
		if !errors.Is(err, errUsage) {
			fmt.Fprintf(stderr, "bashq %s: %v\n", name, err)
		}
		return exitError
	}
	return code
}

func versionString() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}

// common are the options every analysing command takes.
type common struct {
	fs         *flag.FlagSet
	configPath string
	exclude    listFlag
	paths      []string
}

func newCommon(name, synopsis string, stderr io.Writer) *common {
	c := &common{fs: flag.NewFlagSet("bashq "+name, flag.ContinueOnError)}
	c.fs.SetOutput(stderr)
	c.fs.StringVar(&c.configPath, "config", "", "configuration `file` (default: the closest "+config.FileName+")")
	c.fs.Var(&c.exclude, "exclude", "skip paths matching `glob` (repeatable, adds to the configuration)")
	c.fs.Usage = func() {
		out := c.fs.Output()
		fmt.Fprintf(out, "Usage:\n  bashq %s\n\nOptions:\n", synopsis)
		c.fs.PrintDefaults()
	}
	return c
}

// parse parses args and loads the configuration.
func (c *common) parse(args []string) (*config.Config, error) {
	if err := c.fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, err
		}
		return nil, errUsage
	}
	c.paths = c.fs.Args()
	if len(c.paths) == 0 {
		c.paths = []string{"."}
	}
	var cfg *config.Config
	var err error
	if c.configPath != "" {
		cfg, err = config.Load(c.configPath)
	} else {
		cfg, err = config.Find(".")
	}
	if err != nil {
		return nil, err
	}
	cfg.Exclude = append(cfg.Exclude, c.exclude...)
	return cfg, nil
}

// listFlag is a repeatable flag; commas also separate values.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*l = append(*l, part)
		}
	}
	return nil
}

// checkFlags are the options of check and baseline.
type checkFlags struct {
	*common
	rules      listFlag
	maxes      listFlag
	baseline   string
	noBaseline bool
	noRatchet  bool
	shellcheck string
}

func newCheckFlags(name, synopsis string, stderr io.Writer) *checkFlags {
	f := &checkFlags{common: newCommon(name, synopsis, stderr)}
	f.fs.Var(&f.rules, "rule", "run only this `rule` (repeatable; bashq rules lists them)")
	f.fs.Var(&f.maxes, "max", "override a threshold: `rule=N` (repeatable)")
	f.fs.StringVar(&f.baseline, "baseline", "", "baseline `file` (default: baseline in the configuration)")
	f.fs.StringVar(&f.shellcheck, "shellcheck", "", "run ShellCheck: `auto`, always or never (default: the configuration's, auto)")
	return f
}

// options turns the flags into check options.
func (f *checkFlags) options(cfg *config.Config) (check.Options, error) {
	for _, m := range f.maxes {
		name, value, ok := strings.Cut(m, "=")
		n, err := strconv.Atoi(value)
		if !ok || err != nil || n < 1 {
			return check.Options{}, fmt.Errorf("--max %q: want rule=N, N ≥ 1", m)
		}
		rc := cfg.Rules[name]
		rc.Max = n
		cfg.Rules[name] = rc
	}
	if f.shellcheck != "" {
		cfg.ShellCheck.Enabled = f.shellcheck
		if err := cfg.Validate(); err != nil {
			return check.Options{}, err
		}
	}
	baseline := cfg.BaselinePath()
	if f.baseline != "" {
		baseline = f.baseline
	}
	if f.noBaseline {
		baseline = ""
	}
	return check.Options{
		Config: cfg, Paths: f.paths, Only: f.rules,
		Baseline: baseline, Ratchet: !f.noRatchet,
	}, nil
}

func cmdCheck(args []string, stdout, stderr io.Writer) (int, error) {
	f := newCheckFlags("check", "check [options] [paths...]", stderr)
	format := f.fs.String("format", "text", "output `format`: "+strings.Join(report.FindingFormats, ", "))
	output := f.fs.String("output", "", "write the report to `file` instead of the standard output")
	f.fs.BoolVar(&f.noBaseline, "no-baseline", false, "ignore the baseline: report every issue")
	f.fs.BoolVar(&f.noRatchet, "no-ratchet", false, "do not fail on baseline rows that tolerate more than needed")
	cfg, err := f.parse(args)
	if err != nil {
		return exitError, err
	}
	opts, err := f.options(cfg)
	if err != nil {
		return exitError, err
	}
	r, err := check.Run(opts)
	if err != nil {
		return exitError, err
	}
	style := report.StyleFor(os.Stdout)
	if *output != "" {
		file, err := os.Create(*output)
		if err != nil {
			return exitError, err
		}
		defer file.Close()
		stdout, style = file, report.Style{}
	}
	if err := report.Findings(stdout, *format, r, versionString(), style); err != nil {
		return exitError, err
	}
	if len(r.New) > 0 {
		return exitFindings, nil
	}
	return exitOK, nil
}

func cmdBaseline(args []string, stdout, stderr io.Writer) (int, error) {
	f := newCheckFlags("baseline", "baseline [options] [paths...]", stderr)
	cfg, err := f.parse(args)
	if err != nil {
		return exitError, err
	}
	opts, err := f.options(cfg)
	if err != nil {
		return exitError, err
	}
	path := opts.Baseline
	if path == "" {
		path = ".bashq-baseline.tsv"
	}
	opts.Baseline = ""
	r, err := check.Run(opts)
	if err != nil {
		return exitError, err
	}
	b := check.NewBaseline(r.All)
	file, err := os.Create(path)
	if err != nil {
		return exitError, err
	}
	if err := b.Write(file); err != nil {
		file.Close()
		return exitError, err
	}
	if err := file.Close(); err != nil {
		return exitError, err
	}
	fmt.Fprintf(stdout, "wrote %s: %d %s for %d %s\n", path, len(b), plural(len(b), "row", "rows"), len(r.All), plural(len(r.All), "issue", "issues"))
	if cfg.Baseline == "" && f.baseline == "" {
		fmt.Fprintf(stdout, "add it to %s so that check uses it:\n  baseline: %s\n", config.FileName, path)
	}
	return exitOK, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
