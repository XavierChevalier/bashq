[Documentation](README.md) › Configuration

# Configuration

bashq reads `.bashq.yaml` from the current directory, or else from the
closest parent directory that has one. `--config <file>` picks another file.
Without a file, the defaults apply. `bashq init` writes a file with every
default spelled out.

```yaml
# Paths not to analyse.
exclude:
  - vendor
  - "**/testdata"
  - "scripts/generated/*.sh"

# Thresholds; enabled: false turns a rule off.
rules:
  cognitive-complexity:
    max: 15
  cyclomatic-complexity:
    enabled: true
    max: 10
  function-length:
    max: 70
  nesting-depth:
    max: 4
  file-length:
    enabled: false

duplication:
  min-units: 15
  min-lines: 10

shellcheck:
  enabled: auto
  severity: warning
  args: ["-x", "-e", "SC1091"]

baseline: .bashq-baseline.tsv
```

An unknown key, an unknown rule or a bad value is an error that names the
file: a typo never silently turns a rule off.

## exclude

Globs of paths not to analyse, in addition to `.git` and `node_modules`,
which are never walked.

| Glob | Matches |
| --- | --- |
| `vendor` | a file or directory named `vendor`, anywhere, and everything under it |
| `*.generated.sh` | that file name, anywhere |
| `scripts/*.sh` | `.sh` files directly in `scripts/` |
| `scripts/**/*.sh` | `.sh` files anywhere under `scripts/` |
| `**/testdata` | every `testdata` directory |

A glob without a slash matches a name anywhere. A glob with a slash matches
the path relative to each directory given on the command line. `--exclude
<glob>` adds one for a run.

Files named on the command line are always analysed. In a directory, bashq
analyses files ending in `.sh` or `.bash`, plus files whose first line is a
`sh` or `bash` shebang (`#!/bin/sh`, `#!/usr/bin/env bash`,
`#!/usr/bin/env -S bash -e`).

## rules

One entry per rule, by its name ([Rules](rules.md)):

| Key | Type | Meaning |
| --- | --- | --- |
| `enabled` | boolean | turn the rule on or off (default: the rule's) |
| `max` | integer ≥ 1 | the threshold of a measure rule: values over it are reported |

`--max <rule>=<n>` overrides a max for one run, and `--rule <rule>` runs only
the rules named, even ones that are off.

ShellCheck codes are not rules of their own here. Exclude them with
`shellcheck.args: ["-e", "SC2086"]` or with ShellCheck's own directives.

## duplication

| Key | Default | Meaning |
| --- | --- | --- |
| `min-units` | 15 | smallest block reported, in commands and keywords |
| `min-lines` | 10 | smallest block reported, in lines, in each copy |

[How duplicates are found](metrics.md#duplicated-lines).

## shellcheck

| Key | Default | Meaning |
| --- | --- | --- |
| `enabled` | `auto` | `auto`: run ShellCheck when it is on the `PATH`; `always`: fail without it; `never` |
| `severity` | `warning` | lowest level reported: `error`, `warning`, `info` or `style` |
| `args` | `[]` | extra arguments, given to every `shellcheck` run as they are |
| `path` | `shellcheck` | the binary to run |

Pin `always` in CI, so that a runner without ShellCheck fails instead of
passing with fewer checks. `--shellcheck <mode>` overrides `enabled` for one
run.

## baseline

The [baseline](baseline.md) file, relative to the directory of
`.bashq.yaml`. Without it, `bashq check` reports every issue.
`--baseline <file>` overrides it for one run, `--no-baseline` ignores it.

Baseline rows hold the paths as bashq prints them, relative to the current
directory. Run bashq from the directory that holds `.bashq.yaml`, in CI and
locally alike.
