[Documentation](README.md) › Rules

# Rules

`bashq check` applies these rules. `bashq rules` lists them with the
settings in force (defaults, then [`.bashq.yaml`](configuration.md)).

| Rule | Default | Reports |
| --- | --- | --- |
| [`cognitive-complexity`](#cognitive-complexity) | on, max 15 | a function too hard to understand |
| [`cyclomatic-complexity`](#cyclomatic-complexity) | off, max 20 | a function with too many paths |
| [`function-length`](#function-length) | on, max 70 | a function longer than a screen |
| [`nesting-depth`](#nesting-depth) | on, max 4 | control structures nested too deep |
| [`file-length`](#file-length) | on, max 1000 | a file too long |
| [`duplication`](#duplication) | on | a block of code that is a copy of another one |
| [`shellcheck`](#shellcheck) | on when installed | ShellCheck's findings, as `SC1234` rules |
| [`parse-error`](#parse-error) | always | a file that is not valid bash |
| [`baseline-stale`](#baseline-stale) | always, with a baseline | a baseline row to lower or remove |

A measure rule reports a value **over** its max: a function at exactly 15
passes `cognitive-complexity`.

## cognitive-complexity

A function's [cognitive complexity](metrics.md#cognitive-complexity) is over
the max. The default, 15, is SonarQube's usual threshold.

```text
deploy.sh:12 warning deploy::main has a cognitive complexity of 17 (max 15) [cognitive-complexity]
```

**Fix it.** Run `bashq explain <file>:<function>` and look for the biggest
increments. They are the deepest structures:

- return early instead of nesting: `[[ -f $f ]] || continue` rather than
  `if [[ -f $f ]]; then …; fi` around the whole loop body;
- move a nested loop or a `case` branch into a function of its own;
- replace an `if`/`elif` ladder on one value with a `case`, which costs 1
  in total.

## cyclomatic-complexity

A function's [cyclomatic complexity](metrics.md#cyclomatic-complexity) is
over the max. Off by default: cognitive complexity says more about
readability. Turn it on when you want a bound on the test cases a function
needs:

```yaml
rules:
  cyclomatic-complexity: {enabled: true, max: 10}
```

## function-length

A function is longer than the max, in lines, from its name to its closing
brace. The default of 70 lines is a screen, as in
[TigerStyle](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/TIGER_STYLE.md).

**Fix it.** Split along the comments: a comment that announces a step
("# Upload the archive") is usually the name of a function waiting to be
extracted.

## nesting-depth

A function opens more than the max control structures inside each other.
The line reported is the structure that goes over. The default of 4 allows a
loop in a loop in an `if` in a `case`.

**Fix it.** As for cognitive complexity: early returns, and inner loops in
functions of their own.

## file-length

A file has more lines than the max (default 1000, as in SonarQube).

**Fix it.** Split the file into libraries that the script `source`s.

## duplication

A block of at least 15 commands and keywords, spanning at least 10 lines,
appears elsewhere ([how copies are found](metrics.md#duplicated-lines)). The
finding is on the later copy and names the earlier one. SARIF and Sonar
reports carry it as a related location.

```text
rollback.sh:8 warning 23 lines duplicate deploy.sh:46-68 [duplication]
```

A block found in three places is reported once for each pair of copies.
`bashq duplicates` lists every block.

**Fix it.** Move the code to a function in a library both scripts source.

## shellcheck

When [ShellCheck](https://www.shellcheck.net) is installed, `bashq check`
runs it on the same files and reports each of its comments as a rule named
after the code: `SC2086`, `SC2164`… Each finding is attributed to the
function it is in, so the baseline can tolerate it per function.

| ShellCheck level | bashq severity |
| --- | --- |
| error | error |
| warning | warning |
| info, style | note |

ShellCheck runs with `--severity=warning` by default (errors and warnings
only), and with the [`shellcheck.args`](configuration.md#shellcheck) you
give it. Use those to exclude codes (`-e SC1091`) or follow sources (`-x`).
`# shellcheck disable=…` directives work as usual. `--shellcheck never`
turns it off for one run.

## parse-error

The file is not valid bash. The finding gives the position of the error.
The file is not measured, and the check fails until it is fixed: a baseline
cannot tolerate a parse error.

bashq parses every file as bash, including `#!/bin/sh` scripts: bash is a
superset of POSIX sh for parsing purposes. It does not parse zsh.

## baseline-stale

A [baseline](baseline.md) row tolerates more than the code needs. A function
got simpler, or a finding is gone. Run `bashq baseline` to rewrite the
baseline, and commit it with the improvement. This keeps the baseline
shrinking: debt paid off cannot come back unnoticed.

`--no-ratchet` turns this rule off for one run.
