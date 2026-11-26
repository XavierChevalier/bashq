[Documentation](README.md) › CI and dashboards

# CI and dashboards

## GitHub Actions

Annotations on the pull request, without extra permission:

```yaml
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
      - run: go install github.com/XavierChevalier/bashq/cmd/bashq@v0.2.0 # x-release-please-version
      # ShellCheck is preinstalled on GitHub's Ubuntu runners.
      - run: bashq check --shellcheck always --format github
      - if: always()
        run: bashq metrics --top 20 >>"${GITHUB_STEP_SUMMARY}"
```

`--format github` prints one workflow command per issue. GitHub shows each
one on the line in the "Files changed" tab, and the job fails on exit code 1.

Pin a version (`@v0.1.0`) rather than `@latest`, so that a new release with
different scores does not fail an unrelated pull request.

### Code scanning (SARIF)

GitHub code scanning keeps a history of the issues and closes them when
they are fixed. It is free on public repositories. Private ones need GitHub
Advanced Security.

```yaml
    permissions:
      contents: read
      security-events: write
    steps:
      # … checkout, install as above
      - run: bashq check --format sarif --output bashq.sarif
        continue-on-error: true
      - uses: github/codeql-action/upload-sarif@v4
        with:
          sarif_file: bashq.sarif
          category: bashq
```

## GitLab CI, Gitea Actions, Jenkins…

Everywhere else: run `bashq check` and let its exit code fail the job.
Gitea Actions runs the GitHub workflow above unchanged. GitLab reads SARIF
as a [code quality report](https://docs.gitlab.com/ci/testing/code_quality/)
only through a converter, so plain text in the job log is the simplest
option there.

```yaml
# .gitlab-ci.yml
bashq:
  image: golang:1.26
  script:
    - apt-get update && apt-get install -y shellcheck
    - go install github.com/XavierChevalier/bashq/cmd/bashq@v0.2.0 # x-release-please-version
    - bashq check --shellcheck always
```

## Before a commit

A git hook, `.git/hooks/pre-commit`:

```bash
#!/usr/bin/env bash
mapfile -t files < <(git diff --cached --name-only --diff-filter=ACM | grep -E '\.(sh|bash)$')
((${#files[@]})) || exit 0
exec bashq check "${files[@]}"
```

With named files only, the baseline rows of the other files are left alone
([Baseline](baseline.md#what-fails-the-check)).

## SonarQube

SonarQube Community Build does not analyse shell, but it imports external
issues. Send bashq's issues to it:

```bash
bashq check --format sonar --output bashq-sonar.json || true
sonar-scanner -Dsonar.externalIssuesReportPaths=bashq-sonar.json
```

The issues show up on the files, with their rule and message. SonarQube
does not let imported rules into its quality profiles, and it does not
import measures, so complexity and duplication do not reach its dashboards.
[bashq, SonarQube and ShellCheck](comparison.md) explains when that is worth
it.

## Dashboards

`bashq metrics --format prometheus` writes the measures in the Prometheus
text format:

```text
# HELP bashq_function_cognitive_complexity Cognitive complexity of a bash function.
# TYPE bashq_function_cognitive_complexity gauge
bashq_function_cognitive_complexity{path="lib/net.sh",function="net::fetch"} 12
…
bashq_file_duplicated_lines{path="lib/net.sh"} 0
```

| Metric | Labels |
| --- | --- |
| `bashq_function_cognitive_complexity` | `path`, `function` |
| `bashq_function_cyclomatic_complexity` | `path`, `function` |
| `bashq_function_lines` | `path`, `function` |
| `bashq_function_max_nesting` | `path`, `function` |
| `bashq_file_lines` | `path` |
| `bashq_file_code_lines` | `path` |
| `bashq_file_cognitive_complexity` | `path` |
| `bashq_file_duplicated_lines` | `path` |
| `bashq_file_parse_errors` | `path` |

Two ways to get them into Prometheus:

- **node-exporter textfile collector**: a cron job on the machine that holds
  the repository writes them where node-exporter reads them:

  ```bash
  cd /srv/repo && git pull -q &&
    bashq metrics --format prometheus >/var/lib/node_exporter/bashq.prom.tmp &&
    mv /var/lib/node_exporter/bashq.prom.tmp /var/lib/node_exporter/bashq.prom
  ```

- **Pushgateway**, from CI after each merge:

  ```bash
  bashq metrics --format prometheus |
    curl --fail --data-binary @- "${PUSHGATEWAY_URL}/metrics/job/bashq/repo/${REPO}"
  ```

Queries for a Grafana panel:

```promql
# Total cognitive complexity, over time
sum(bashq_file_cognitive_complexity)

# The ten most complex functions
topk(10, bashq_function_cognitive_complexity)

# Duplication density, in %
100 * sum(bashq_file_duplicated_lines) / sum(bashq_file_lines)
```

Every function is a series. A repository with thousands of functions makes
thousands of series, which is fine for one Prometheus but worth knowing
before you push the metrics of many repositories.
