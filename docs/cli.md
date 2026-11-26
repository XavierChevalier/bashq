[Documentation](README.md) › Command line

# Command line

```text
bashq <command> [options] [paths...]
```

Paths are files and directories. They default to the current directory.
[Which files are analysed](configuration.md#exclude).

| Command | Does |
| --- | --- |
| [`check`](#bashq-check) | applies the rules, fails on issues the baseline does not tolerate |
| [`metrics`](#bashq-metrics) | measures every function |
| [`explain`](#bashq-explain) | shows where a function's cognitive complexity comes from |
| [`duplicates`](#bashq-duplicates) | lists duplicated blocks |
| [`baseline`](#bashq-baseline) | writes the baseline from the current issues |
| [`rules`](#bashq-rules) | lists the rules and their settings |
| [`init`](#bashq-init) | writes a `.bashq.yaml` with the defaults |
| `version` | prints `bashq <version>` |

`bashq <command> -h` prints the options of a command.

## Options of every analysing command

| Option | Meaning |
| --- | --- |
| `--config <file>` | configuration file (default: the closest `.bashq.yaml`, else the defaults) |
| `--exclude <glob>` | skip matching paths; repeatable, adds to the configuration's `exclude` |

## bashq check

```text
bashq check [options] [paths...]
```

The quality gate. It measures the scripts, applies the [rules](rules.md),
runs ShellCheck, compares the result with the [baseline](baseline.md), and
reports the issues left.

| Option | Meaning |
| --- | --- |
| `--format <format>` | `text` (default), `json`, `sarif`, `sonar`, `github` |
| `--output <file>` | write the report to a file instead of the standard output |
| `--rule <rule>` | run only this rule; repeatable, or comma-separated |
| `--max <rule>=<n>` | override a threshold; repeatable |
| `--baseline <file>` | baseline to compare with (default: the configuration's `baseline`) |
| `--no-baseline` | ignore the baseline: report every issue |
| `--no-ratchet` | do not fail on baseline rows that tolerate more than needed |
| `--shellcheck <mode>` | `auto`, `always` or `never` (default: the configuration's, `auto`) |

### Formats

| Format | For |
| --- | --- |
| `text` | people: `path:line[:col] severity message [rule]`, then a summary. Colours on a terminal, unless `NO_COLOR` is set |
| `json` | scripts: `{version, files, functions, shellcheck, baselined, findings: [...]}` |
| `sarif` | [SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html): GitHub code scanning, GitLab, IDEs. Each result has a stable `partialFingerprints` keyed on rule, file and function |
| `sonar` | SonarQube's [generic issue format](https://docs.sonarsource.com/sonarqube-community-build/analyzing-source-code/importing-external-issues/generic-issue-import-format/), for `sonar.externalIssuesReportPaths` |
| `github` | GitHub Actions [workflow commands](https://docs.github.com/actions/reference/workflow-commands-for-github-actions): annotations on the pull request, no extra permission |

A JSON finding:

```json
{
  "rule": "cognitive-complexity",
  "path": "examples/deploy/deploy.sh",
  "line": 12,
  "end_line": 43,
  "function": "deploy::main",
  "value": 17,
  "max": 15,
  "severity": "warning",
  "message": "deploy::main has a cognitive complexity of 17 (max 15)"
}
```

Duplication findings add `related` (the other copy). Findings over their
baseline row add `allowed` (the value the row tolerates).

## bashq metrics

```text
bashq metrics [options] [paths...]
```

Measures every function, without rules or baseline. It always exits 0.
Files that cannot be parsed are listed on the standard error.

| Option | Meaning |
| --- | --- |
| `--format <format>` | `text` (default), `json`, `tsv`, `prometheus` |
| `--sort <measure>` | `cognitive` (default), `cyclomatic`, `lines`, `nesting`, highest first; `name` for source order |
| `--top <n>` | only the first n functions (`text`, `tsv`) |

| Format | Holds |
| --- | --- |
| `text` | a table of functions, then totals: files, lines, code lines, functions, complexity, duplicated lines and density |
| `tsv` | `path line function cognitive cyclomatic lines nesting`, with a header |
| `json` | every file with its line counts, duplicated lines, functions and their increments; parse errors; duplicated blocks |
| `prometheus` | gauges per function and per file, see [CI and dashboards](ci.md#dashboards) |

The `text` and `tsv` tables list the code outside functions as `(top
level)` when it costs something, or when the file has no function.

## bashq explain

```text
bashq explain [--min <n>] <file>[:<function>] [<function>...]
```

Prints the function with the cost of each line in the margin and the
increments as comments ([example](metrics.md#reading-a-score)). Without a
function name, it explains every function of the file whose cognitive
complexity is at least `--min` (default 1). Use `(top level)` for the code
outside functions.

## bashq duplicates

```text
bashq duplicates [--min-units <n>] [--min-lines <n>] [paths...]
```

Lists every duplicated block (`lines, units: first copy = second copy`),
then the total. The options override the [configuration](configuration.md#duplication).

## bashq baseline

```text
bashq baseline [options] [paths...]
```

Runs the check without a baseline and writes every issue to the baseline
file: `--baseline <file>`, else the configuration's `baseline`, else
`.bashq-baseline.tsv`. It takes the options of `check` that select issues
(`--rule`, `--max`, `--shellcheck`, `--exclude`, `--config`).

## bashq rules

Lists every rule, whether it is on, its max, and what it reports, with the
configuration applied.

## bashq init

Writes `.bashq.yaml` with every default spelled out. `--force` overwrites
an existing one.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | no issue (or a command that does not check) |
| 1 | `check` found issues the baseline does not tolerate |
| 2 | bad usage, bad configuration, a path that does not exist, ShellCheck missing with `always` or failing |

## Environment

| Variable | Effect |
| --- | --- |
| `NO_COLOR` | no colours in text output |
| `PATH` | where `shellcheck` is looked up, unless `shellcheck.path` is set |
