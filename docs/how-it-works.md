[Documentation](README.md) › How it works

# How it works

```text
paths ─▶ discover ─▶ parse (mvdan/sh) ─┬─▶ metrics ─┐
                      in parallel      └─▶ dup ─────┼─▶ rules ─▶ baseline ─▶ report
                                       shellcheck ──┘
```

1. **Discover** (`internal/discover`): walk the paths, keep `.sh` and
   `.bash` files and files with a sh or bash shebang, and drop the excluded
   globs.
2. **Parse** each file with [mvdan.cc/sh/v3/syntax](https://pkg.go.dev/mvdan.cc/sh/v3/syntax),
   the parser of shfmt, in bash mode with comments kept. Files are parsed
   in parallel, one worker per CPU.
3. **Measure** (`metrics`): one walk of the syntax tree per function. The
   walker carries the nesting level and records each increment with its
   position, which is what `bashq explain` prints.
4. **Find duplicates** (`dup`): print every simple command back in canonical
   form, hash windows of consecutive units, then extend each match as far as
   both copies stay equal ([details](metrics.md#duplicated-lines)).
5. **ShellCheck** (`internal/check`): `shellcheck --format=json1` on batches
   of files, in parallel. Each comment becomes a finding attributed to the
   function around its line.
6. **Rules and baseline** (`internal/check`): compare the measures with the
   thresholds, then the findings with the baseline.
7. **Report** (`internal/report`): text, JSON, SARIF, Sonar, GitHub,
   Prometheus.

Nothing is cached, and nothing runs but ShellCheck. bashq never executes
the scripts it reads.

## Why a syntax tree

A line-based tool has to guess. Is `if` inside a string? Is this `}` the end
of the function or of a `${var}`? Is this line part of a heredoc, or of a
multi-line awk program in single quotes? Every guess is a false positive
for someone. The tree answers all of these exactly: a heredoc is a node, an
argument is a word, and a `case` pattern is not a command.

mvdan/sh is the parser shfmt uses on millions of scripts. It parses bash,
POSIX sh, mksh and bats. bashq uses bash mode, which accepts POSIX scripts.

## The Go API

The measures are a library. `metrics` and `dup` have no dependency on the
command line.

```go
import (
	"fmt"
	"os"

	"github.com/XavierChevalier/bashq/metrics"
)

func main() {
	src, _ := os.ReadFile("deploy.sh")
	file, err := metrics.Analyze("deploy.sh", src)
	if err != nil {
		panic(err) // a syntax error, with its position
	}
	for _, fn := range file.Functions {
		fmt.Printf("%s: cognitive %d, cyclomatic %d, %d lines\n",
			fn.Name, fn.Cognitive, fn.Cyclomatic, fn.Lines)
		for _, inc := range fn.Increments {
			fmt.Printf("  line %d: %s\n", inc.Line, inc)
		}
	}
}
```

| Package | Gives |
| --- | --- |
| [`metrics`](../metrics) | `Analyze(path, src)`, `Parse`, `Measure(path, src, tree)`; `File` (line counts, functions, top level) and `Function` (measures and increments) |
| [`dup`](../dup) | `Find(sources, Options)` → duplicated `Block`s; `DuplicatedLines(blocks)` per file |

The packages under `internal/` (discovery, configuration, rules, baseline,
reports) are the command line's. Their API may change in any release.
