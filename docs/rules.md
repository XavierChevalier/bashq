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
| [`line-length`](#line-length) | on, max 100 | a line wider than the max, in characters |
| [`unbounded-loop`](#unbounded-loop) | on | a loop with no upper bound: `while true`, `while :` |
| [`duplication`](#duplication) | on | a block of code that is a copy of another one |
| [`shellcheck`](#shellcheck) | on when installed | ShellCheck's findings, as `SC1234` rules |
| [`parse-error`](#parse-error) | always | a file that is not valid bash |
| [`baseline-stale`](#baseline-stale) | always, with a baseline | a baseline row to lower or remove |

A rule with a max reports a value **over** it: a function at exactly 15
passes `cognitive-complexity`, a line of exactly 100 characters passes
`line-length`.

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

## line-length

A line is wider than the max, in characters (Unicode characters, not
bytes; a tab counts 1). The default of 100 is
[TigerStyle](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/TIGER_STYLE.md)'s:
a line you can read without scrolling, and two files side by side on one
screen. Each long line is a finding, attributed to the function it is in.

```text
deploy.sh:31:101 warning line is 117 characters long (max 100) [line-length]
```

Unlike the other measures, width is read on the text: every line counts,
comments, strings and heredoc bodies included, since a reader has to scroll
them all the same. Where long lines are the point (generated files,
templates, a heredoc that must stay as it is), exclude the paths:

```yaml
rules:
  line-length:
    exclude: ["templates", "*.generated.sh"]
```

`line-length` is a [count rule](baseline.md) with a max: a baseline row
holds the number of long lines tolerated in a function, not the widest one.
A row keyed on the widest line would let a function gain long lines as long
as each stays under the widest, and the ratchet would only notice when that
one line is shortened. A count fails on every new long line, and turns stale
as soon as one is fixed.

**Fix it.** Break the command at its arguments with `\`, one option per
line; move a long pipeline's stages to their own lines after `|`; give a
long string a variable, or build it with `printf`.

## unbounded-loop

A `while` or `until` loop whose condition is a constant that never ends it:
only a `break`, a `return` or an `exit` inside gets out.

```text
deploy.sh:40:3 warning loop with no upper bound: bound it (a max count or a deadline) or assert one inside [unbounded-loop]
```

The conditions reported are one command among `true`, `:`, `false` and
`(( n ))` with a number, negated with `!` or not, that keeps the loop
running: `while true`, `while :`, `while (( 1 ))`, `until false`,
`while ! false`. They are read on the syntax tree, so the same words in a
string, a heredoc or a comment are not loops. A condition that can change,
such as `while read -r line` or `while [[ $i -lt 3 ]]`, is a bound and is
not reported.

[TigerStyle](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/TIGER_STYLE.md)
puts a limit on everything: a loop that waits for a service, polls a job or
retries a download needs a number of tries or a deadline, or it hangs a
deploy forever the day the service does not come back. Every such loop is
reported, even one with a counter and a `break` inside: the bound belongs in
the condition, where a reader sees it. A daemon's main loop, unbounded on
purpose, goes in the [baseline](baseline.md) or the rule's `exclude`.

**Fix it.** Put the bound in the condition:

```bash
# Before
while true; do
  curl -fsS "$url" && break
  sleep 2
done

# After: 30 tries, then a clear error
tries=0
until curl -fsS "$url"; do
  tries=$((tries + 1))
  ((tries < 30)) || { echo "gave up on $url" >&2; return 1; }
  sleep 2
done
```

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
