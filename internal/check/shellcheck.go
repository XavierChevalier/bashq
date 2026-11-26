package check

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/XavierChevalier/bashq/internal/config"
)

// shellCheckBatch is the most files given to one shellcheck run.
const shellCheckBatch = 200

// ErrNoShellCheck is returned when shellcheck is required and missing.
var ErrNoShellCheck = errors.New("shellcheck not found on the PATH (install it, or set shellcheck.enabled: never)")

// ShellCheckAvailable reports whether ShellCheck runs with this
// configuration: an error when it is required and missing.
func ShellCheckAvailable(cfg config.ShellCheck) (bool, error) {
	if cfg.Enabled == "never" {
		return false, nil
	}
	if _, err := exec.LookPath(cfg.Path); err != nil {
		if cfg.Enabled == "always" {
			return false, ErrNoShellCheck
		}
		return false, nil
	}
	return true, nil
}

type shellCheckComment struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	EndLine   int    `json:"endLine"`
	Column    int    `json:"column"`
	EndColumn int    `json:"endColumn"`
	Level     string `json:"level"`
	Code      int    `json:"code"`
	Message   string `json:"message"`
}

// ShellCheckFindings runs ShellCheck on the analysed files, in parallel
// batches, and returns its comments as findings, attributed to the function
// they are in.
func (a *Analysis) ShellCheckFindings(cfg config.ShellCheck) ([]Finding, error) {
	size := min(shellCheckBatch, max(1, (len(a.Files)+runtime.GOMAXPROCS(0)-1)/runtime.GOMAXPROCS(0)))
	var batches [][]string
	for start := 0; start < len(a.Files); start += size {
		var batch []string
		for _, f := range a.Files[start:min(start+size, len(a.Files))] {
			batch = append(batch, f.Path)
		}
		batches = append(batches, batch)
	}

	comments := make([][]shellCheckComment, len(batches))
	errs := make([]error, len(batches))
	var wg sync.WaitGroup
	for i, batch := range batches {
		wg.Go(func() { comments[i], errs[i] = runShellCheck(cfg, batch) })
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	var out []Finding
	for _, batch := range comments {
		for _, c := range batch {
			out = append(out, a.shellCheckFinding(c))
		}
	}
	return out, nil
}

func runShellCheck(cfg config.ShellCheck, paths []string) ([]shellCheckComment, error) {
	args := []string{"--format=json1", "--severity=" + cfg.Severity}
	args = append(args, cfg.Args...)
	args = append(args, "--")
	args = append(args, paths...)
	cmd := exec.Command(cfg.Path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	// shellcheck exits 1 when it found something: that is not a failure.
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return nil, fmt.Errorf("shellcheck: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	var report struct {
		Comments []shellCheckComment `json:"comments"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		return nil, fmt.Errorf("shellcheck: cannot read its json1 output: %v", err)
	}
	return report.Comments, nil
}

func (a *Analysis) shellCheckFinding(c shellCheckComment) Finding {
	f := Finding{
		Rule:     fmt.Sprintf("SC%d", c.Code),
		Location: Location{Path: c.File, Line: c.Line, Col: c.Column},
		Value:    1,
		Severity: shellCheckSeverity(c.Level),
		Message:  c.Message,
	}
	if c.EndLine > c.Line {
		f.EndLine = c.EndLine
	}
	if file := a.File(c.File); file != nil {
		f.Function = file.FunctionAt(c.Line).Name
	}
	return f
}

// shellCheckSeverity maps ShellCheck levels to SARIF levels.
func shellCheckSeverity(level string) string {
	switch level {
	case "error":
		return "error"
	case "warning":
		return "warning"
	default:
		return "note"
	}
}
