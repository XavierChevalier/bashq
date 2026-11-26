package check

import (
	"github.com/XavierChevalier/bashq/dup"
	"github.com/XavierChevalier/bashq/internal/config"
	"github.com/XavierChevalier/bashq/internal/discover"
)

// Options configures a run.
type Options struct {
	Config *config.Config
	// Paths are the files and directories to analyse.
	Paths []string
	// Only runs these rules (all enabled rules when empty).
	Only []string
	// Baseline is the baseline file ("" for none).
	Baseline string
	// Ratchet reports baseline rows that tolerate more than needed.
	Ratchet bool
}

// Report is the outcome of a run.
type Report struct {
	Analysis *Analysis
	Settings *Settings
	// All are the findings before the baseline.
	All []Finding
	// New are the findings the baseline does not tolerate: the check fails
	// when there is one.
	New []Finding
	// Baselined counts the findings the baseline tolerates.
	Baselined int
	// ShellCheck tells whether ShellCheck ran.
	ShellCheck bool
}

// Run discovers the scripts, measures them, applies the rules, runs
// ShellCheck and compares the result with the baseline.
func Run(opts Options) (*Report, error) {
	settings, err := Resolve(opts.Config)
	if err != nil {
		return nil, err
	}
	if len(opts.Only) > 0 {
		if err := settings.Only(opts.Only); err != nil {
			return nil, err
		}
	}
	paths, err := discover.Scripts(opts.Paths, opts.Config.Exclude)
	if err != nil {
		return nil, err
	}

	var dupOpts *dup.Options
	if settings.Enabled("duplication") {
		dupOpts = &dup.Options{MinUnits: opts.Config.Duplication.MinUnits, MinLines: opts.Config.Duplication.MinLines}
	}
	analysis := Analyze(paths, dupOpts)
	report := &Report{Analysis: analysis, Settings: settings}
	report.All = analysis.Findings(settings)

	if settings.Enabled("shellcheck") {
		ok, err := ShellCheckAvailable(opts.Config.ShellCheck)
		if err != nil {
			return nil, err
		}
		if ok && len(analysis.Files) > 0 {
			found, err := analysis.ShellCheckFindings(opts.Config.ShellCheck)
			if err != nil {
				return nil, err
			}
			report.All = append(report.All, found...)
			Sort(report.All)
			report.ShellCheck = true
		} else {
			// Its baseline rows must not look stale.
			settings.Disable("shellcheck")
		}
	}

	baseline := Baseline{}
	if opts.Baseline != "" {
		if baseline, err = ReadBaseline(opts.Baseline); err != nil {
			return nil, err
		}
	}
	analysed := map[string]bool{}
	for _, p := range paths {
		analysed[p] = true
	}
	outcome := baseline.Apply(report.All, settings, analysed, opts.Ratchet)
	report.New, report.Baselined = outcome.New, outcome.Baselined
	return report, nil
}
