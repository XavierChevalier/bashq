[Documentation](README.md) › Troubleshooting

# Troubleshooting

## `parse-error` on a script that runs fine

bashq parses as bash. Check the reported position:

- **zsh or ksh syntax** (`${(k)map}`, `${=var}`). Exclude the file: bashq
  does not measure zsh.
- **`floating point arithmetic is a zsh feature`** on an associative array
  literal: the parser reads `[key]=value` keys as arithmetic, so an
  unquoted key with a dot (`[run.sh]=x`) fails. Quote the key:
  `["run.sh"]=x`. Bash treats both the same.
- **Another bash construct the parser rejects.** That is a bug in bashq or in
  mvdan/sh: open an issue with the line.
- **The script is not shell** (a Python file named `.sh`). Exclude it, or
  give it its real extension.

A file with a parse error is not measured, and its error cannot be
baselined: fix or exclude it.

## A score looks wrong

Run `bashq explain <file>:<function>`. Each increment is listed with its
line and the reason. If one of them contradicts [Metrics](metrics.md), it is
a bug: open an issue with the function.

The usual surprises:

- `[[ -n $x ]] || return 1` costs 1 + nesting: a test is an `if`
  ([why](metrics.md#bash-specific-choices)).
- `cd "$d" || return 1` costs nothing: error handling is free.
- A function declared inside another one is measured on its own.

## A rule I disabled still runs

`--rule` runs the rules named, even disabled ones. Without `--rule`, check
the spelling in `.bashq.yaml`: an unknown rule is an error, but `enabled:
false` under the wrong rule disables that other rule. `bashq rules` shows
what is in force.

## `baseline-stale` after an unrelated change

A row is stale when the code needs less than it tolerates. Something got
simpler, or a function was renamed (its old row has nothing left to
tolerate, and the new name has no row). Run `bashq baseline` and commit the
diff, which shows exactly that.

## The baseline does not match in CI

Rows hold paths as bashq prints them, relative to the current directory.
Run `bashq check` from the directory holding `.bashq.yaml` in CI too, with
the same paths (`bashq check`, not `bashq check ./scripts` in one place and
`bashq check scripts` in another).

## `shellcheck not found on the PATH`

`shellcheck.enabled` is `always`, or `--shellcheck always` was given, and
the binary is missing. Install it ([how](installation.md#shellcheck)), set
`shellcheck.path`, or use `auto`.

## ShellCheck findings differ between machines

ShellCheck's checks change between versions. Pin it in CI (the version of
the runner image, or a fixed download), and baseline with the same version.

## `bashq check` is slow

The measures take milliseconds. ShellCheck takes the rest: it is the slow
part on big files. `bashq check --shellcheck never` shows the difference.
bashq already runs one ShellCheck per CPU. Excluding generated or vendored
scripts helps both.
