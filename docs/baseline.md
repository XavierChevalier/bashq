[Documentation](README.md) › Baseline and ratchet

# Baseline and ratchet

An existing project fails most quality gates on day one. A baseline lets
you turn the gate on anyway: it records today's issues as known debt, and
`bashq check` then fails only on what gets worse.

SonarQube's version of this is "clean as you code": the quality gate looks
at new code only. bashq does the same with a file in the repository, so it
works in any CI and is reviewed like code.

## Create it

```bash
bashq baseline
```

This writes `.bashq-baseline.tsv`. Point `.bashq.yaml` at it, then commit
both:

```yaml
baseline: .bashq-baseline.tsv
```

`bashq baseline --baseline <file>` writes somewhere else. The rules,
thresholds and excludes are the ones `bashq check` uses, so build the
baseline with the same configuration.

## What a row holds

```text
# bashq baseline: rule, path, function, tolerated value (measure rules) or count. Regenerate with `bashq baseline`.
cognitive-complexity	examples/deploy/deploy.sh	deploy::main	17
duplication	examples/deploy/rollback.sh	rollback::switch	1
SC2086	lib/net.sh	net::fetch	2
```

Four tab-separated fields: rule, path, function, value. A row keys on the
function, not on a line, so edits elsewhere in the file do not break it.
Code outside functions is `(top level)`, and file-wide findings
(`file-length`) have an empty function.

| Rule kind | Value | A finding passes while |
| --- | --- | --- |
| measure (`cognitive-complexity`, `cyclomatic-complexity`, `function-length`, `nesting-depth`, `file-length`) | the measure tolerated | its measure is ≤ the value |
| count (`line-length`, `unbounded-loop`, `duplication`, `SC…`) | the number tolerated in that function | the function has ≤ that many |

The rows are sorted by path, then function, then rule, so the baseline's
diff in a pull request shows which debt it adds or pays off.

## What fails the check

| Change | Result |
| --- | --- |
| a new issue (no row) | fails: the issue is reported |
| a measure goes over its row (17 → 19) | fails: `(baseline allows 17)` is added to the message |
| a function gets one more ShellCheck finding of a code than its row | fails: all its findings of that code are reported, since bashq cannot tell which one is new |
| a measure goes down (17 → 12) | fails with [`baseline-stale`](rules.md#baseline-stale): lower the row |
| an issue disappears | fails with `baseline-stale`: remove the row |
| an unrelated threshold change | nothing: rows hold values, not thresholds |

The last two lines are the ratchet: the baseline can only shrink. Whoever
pays off debt runs `bashq baseline` and commits the smaller file, and that
debt cannot come back unnoticed. `--no-ratchet` turns this off for one run
(on a branch you are about to rebase, for example).

Rows are compared only when they can be:

- rows of a file that was not analysed (`bashq check lib/` leaves the rows
  of `bin/` alone) are skipped;
- rows of a rule that did not run (`--rule`, or ShellCheck not installed)
  are skipped too.

## Paying off debt

```bash
bashq explain lib/net.sh:net::fetch   # find the expensive lines
$EDITOR lib/net.sh                    # simplify
bashq check                           # fails: baseline-stale, the row can go down
bashq baseline                        # rewrite the baseline
git commit -am "refactor: simplify net::fetch"
```

## Without a baseline

`bashq check --no-baseline` reports every issue, as if there were no
baseline: the list of debt left.
