# bashq documentation

bashq measures bash code (complexity, length, nesting, duplication), merges
in ShellCheck's findings, and gates CI on new issues only.

## Start here

1. [Installation](installation.md): a binary, `go install`, or from source; ShellCheck.
2. [Getting started](getting-started.md): measure a project, understand a
   score, fix it, baseline the rest, gate in CI.

## Guides

| Page | Read it to |
| --- | --- |
| [Baseline and ratchet](baseline.md) | adopt bashq on existing code: accept today's debt, fail on what gets worse, pay it off |
| [CI and dashboards](ci.md) | run bashq in GitHub Actions (annotations, SARIF), other CIs, before a commit; send issues to SonarQube and measures to Prometheus and Grafana |
| [bashq, SonarQube and ShellCheck](comparison.md) | see what each tool covers for bash, and when to use which |

## Reference

| Page | Contains |
| --- | --- |
| [Command line](cli.md) | every command, option, format and exit code |
| [Metrics](metrics.md) | how cognitive and cyclomatic complexity, length, nesting, lines and duplication are counted |
| [Rules](rules.md) | what each rule reports, its default, how to fix it |
| [Configuration](configuration.md) | every key of `.bashq.yaml` |
| [How it works](how-it-works.md) | the pipeline, and the Go API (`metrics`, `dup`) |
| [Troubleshooting](troubleshooting.md) | parse errors, surprising scores, baseline mismatches |
| [Contributing](../CONTRIBUTING.md) | checks, commit messages, releases |
| [Changelog](../CHANGELOG.md) | what changed in each release |

## Conventions in this documentation

- `$` starts a command you type; the lines under it are its output.
- Outputs are shown without colours (`NO_COLOR=1`).
- Paths like `metrics/walker.go` are relative to the bashq repository.
