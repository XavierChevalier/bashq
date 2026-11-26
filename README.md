# bashq

[![CI](https://github.com/XavierChevalier/bashq/actions/workflows/ci.yml/badge.svg)](https://github.com/XavierChevalier/bashq/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/XavierChevalier/bashq)](https://github.com/XavierChevalier/bashq/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

Code quality for bash: complexity, duplication and a quality gate, like
SonarQube, for the language SonarQube's free edition does not analyse.

- **Measure** every function: cognitive complexity (SonarSource's metric,
  adapted to bash), cyclomatic complexity, length, nesting depth. Plus lines
  of code and comments for every file.
- **Explain** a score line by line with `bashq explain`, the way SonarQube
  annotates a function.
- **Find copies**: blocks of code duplicated within a script or across
  scripts, whatever their indentation, quoting style or comments.
- **Gate** with `bashq check`. It applies thresholds, flags lines over 100
  characters and loops with no bound (`while true`), merges in ShellCheck's
  findings, and fails on new issues only: a baseline keeps today's debt, and
  that baseline can only shrink.
- **Report** as text, JSON, SARIF (GitHub code scanning), GitHub
  annotations, SonarQube's generic issues, or Prometheus metrics for a
  Grafana dashboard.

Everything is computed on the syntax tree of [mvdan/sh](https://github.com/mvdan/sh),
the parser behind shfmt. Heredocs, quotes, comments and the awk or jq
programs inside a script cannot fool it. One static binary, no runtime. A
200-script repository takes under 0.1 s, plus ShellCheck when it runs.

## Install

```bash
go install github.com/XavierChevalier/bashq/cmd/bashq@latest
bashq version
```

Or download a binary for Linux or macOS from the
[releases](https://github.com/XavierChevalier/bashq/releases/latest).
ShellCheck is optional: bashq runs it when it is on the `PATH`. All the
ways to install: [Installation](docs/installation.md).

## Quick start

[`examples/deploy`](examples/deploy) holds two scripts that deploy and roll
back an app. Check them:

```console
$ bashq check examples/deploy
examples/deploy/deploy.sh:12 warning deploy::main has a cognitive complexity of 17 (max 15) [cognitive-complexity]
examples/deploy/rollback.sh:8 warning 23 lines duplicate examples/deploy/deploy.sh:46-68 [duplication]

✗ 2 issues: cognitive-complexity 1, duplication 1 (2 files, 6 functions)
```

Where does the 17 come from?

```console
$ bashq explain examples/deploy/deploy.sh:deploy::main
deploy::main examples/deploy/deploy.sh:12: cognitive complexity 17, cyclomatic 9, 32 lines, nesting 4

  12     │ deploy::main() {
  ...
  22 +1  │   for host in "$@"; do  # +1 for
  23 +2  │     if ! ssh "${host}" true; then  # +2 if (nesting 1)
  ...
  27 +2  │     if [[ "${dry_run}" == true ]]; then  # +2 if (nesting 1)
  28     │       echo "would deploy ${version} to ${host}"
  29 +1  │     else  # +1 else
  30 +3  │       for attempt in 1 2 3; do  # +3 for (nesting 2)
  31 +4  │         if scp "app-${version}.tar.gz" "${host}:${RELEASES}/"; then  # +4 if (nesting 3)
```

Each `if` and loop costs 1, plus 1 for every structure it sits in. The
upload retry loop alone costs 9, because it is nested three levels deep.
Extract it into a function of its own and both functions drop under 15.

All the measures, highest first:

```console
$ bashq metrics examples/deploy
  cognitive  cyclomatic  lines  nesting
         17           9     32        4  deploy::main examples/deploy/deploy.sh:12
          1           2     20        1  deploy::switch examples/deploy/deploy.sh:46
          1           2     20        1  rollback::switch examples/deploy/rollback.sh:8
  ...

2 files, 104 lines (86 of code), 6 functions, cognitive complexity 19, 46 duplicated lines (44.2 %)
```

On an existing project, accept today's debt once and fail only on what gets
worse:

```bash
bashq init       # writes .bashq.yaml
bashq baseline   # writes .bashq-baseline.tsv; add "baseline: .bashq-baseline.tsv" to .bashq.yaml
bashq check      # passes; fails on a new issue, a function that got worse, or a row to lower
```

## Documentation

Start with **[Getting started](docs/getting-started.md)**: a project from its
first check to a quality gate in CI. Everything else is in the
**[documentation index](docs/README.md)**:

| Guides | Reference |
| --- | --- |
| [Installation](docs/installation.md) | [Command line](docs/cli.md): every command, option and exit code |
| [Getting started](docs/getting-started.md) | [Metrics](docs/metrics.md): how each measure is counted |
| [Baseline and ratchet](docs/baseline.md) | [Rules](docs/rules.md): what each rule reports and how to fix it |
| [CI and dashboards](docs/ci.md) | [Configuration](docs/configuration.md): `.bashq.yaml` |
| [bashq, SonarQube and ShellCheck](docs/comparison.md) | [How it works](docs/how-it-works.md), and the Go API |
| [Troubleshooting](docs/troubleshooting.md) | [Changelog](CHANGELOG.md) · [Contributing](CONTRIBUTING.md) |

## Companion

[bashx](https://github.com/XavierChevalier/bashx) tests bash: unit tests,
coverage and mutation testing. bashq measures the code those tests cover.

## License

[MIT](LICENSE)
