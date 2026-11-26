[Documentation](README.md) › Getting started

# Getting started

From a first look at a project to a quality gate in CI, in five steps. The
examples use [`examples/deploy`](../examples/deploy), two scripts that
deploy an app and roll it back. Run them from a clone of bashq, or follow
along on your own scripts.

## 1. Measure

```console
$ bashq metrics examples/deploy
  cognitive  cyclomatic  lines  nesting
         17           9     32        4  deploy::main examples/deploy/deploy.sh:12
          1           2     20        1  deploy::switch examples/deploy/deploy.sh:46
          1           2     20        1  rollback::switch examples/deploy/rollback.sh:8
          0           1      3        0  deploy::usage examples/deploy/deploy.sh:7
          0           1      3        0  journalctl_tail examples/deploy/deploy.sh:67
          0           1      3        0  journalctl_tail examples/deploy/rollback.sh:29

2 files, 104 lines (86 of code), 6 functions, cognitive complexity 19, 46 duplicated lines (44.2 %)
```

The functions come sorted by cognitive complexity, highest first. On a big
project, `--top 20` shows where to look first.

## 2. Check

```console
$ bashq check examples/deploy
examples/deploy/deploy.sh:12 warning deploy::main has a cognitive complexity of 17 (max 15) [cognitive-complexity]
examples/deploy/rollback.sh:8 warning 23 lines duplicate examples/deploy/deploy.sh:46-68 [duplication]

✗ 2 issues: cognitive-complexity 1, duplication 1 (2 files, 6 functions)
$ echo $?
1
```

Two issues, so the check fails with exit code 1. With ShellCheck installed,
its findings show up here too, as `SC…` rules.

## 3. Understand and fix

```console
$ bashq explain examples/deploy/deploy.sh:deploy::main
…
  30 +3  │       for attempt in 1 2 3; do  # +3 for (nesting 2)
  31 +4  │         if scp "app-${version}.tar.gz" "${host}:${RELEASES}/"; then  # +4 if (nesting 3)
  32     │           break
  33 +1  │         elif [[ "${attempt}" -eq 3 ]]; then  # +1 elif
…
```

The upload retry loop costs 9 of the 17 because it sits three levels deep.
Move it into a function:

```bash
# Uploads the release archive to <host>, three attempts.
deploy::upload() {
  local host="$1" version="$2"
  for attempt in 1 2 3; do
    if scp "app-${version}.tar.gz" "${host}:${RELEASES}/"; then
      return 0
    fi
    sleep $((attempt * 2))
  done
  echo "upload to ${host} failed" >&2
  return 1
}
```

`deploy::main` then calls `deploy::upload "${host}" "${version}" || return 1`
and drops to 8, while `deploy::upload` is 3. The duplication goes the same
way: `rollback.sh` sources a library holding `switch` instead of carrying a
copy of it.

## 4. Configure and baseline

A real project has more debt than one afternoon can pay. Write a
configuration, then accept the current issues:

```console
$ bashq init
wrote .bashq.yaml
$ bashq baseline
wrote .bashq-baseline.tsv: 2 rows for 2 issues
add it to .bashq.yaml so that check uses it:
  baseline: .bashq-baseline.tsv
```

Uncomment `baseline: .bashq-baseline.tsv` in `.bashq.yaml`, then:

```console
$ bashq check
✓ No issues (2 files, 6 functions; 2 baselined)
```

From now on the check fails on a new issue, on a function that gets worse,
and on a baseline row that tolerates more than the code needs, so the
baseline only shrinks. [Baseline and ratchet](baseline.md) has the details.

Commit both files.

## 5. Gate in CI

```yaml
# .github/workflows/quality.yml
name: Quality
on: [push, pull_request]
permissions:
  contents: read
jobs:
  bashq:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: stable
      - run: go install github.com/XavierChevalier/bashq/cmd/bashq@v0.1.0 # x-release-please-version
      - run: bashq check --shellcheck always --format github
```

Issues show up as annotations on the pull request.
[CI and dashboards](ci.md) covers SARIF, other CIs, SonarQube and Grafana.
