[Documentation](README.md) › bashq, SonarQube and ShellCheck

# bashq, SonarQube and ShellCheck

bashq exists because the free tools stop short for bash. This page lists
what each one does, as of November 2026, so you can pick the right one.

## What each tool covers

| | ShellCheck | SonarQube Community Build (free) | SonarQube Server / Cloud (paid) | bashq |
| --- | --- | --- | --- | --- |
| Analyses bash / sh | ✅ | ❌ no Shell analyzer | ✅ about 20 rules | ✅ |
| Bugs and pitfalls (quoting, `cd` without check…) | ✅ hundreds of checks | ❌ | ✅ its own rules, not ShellCheck's | ✅ by running ShellCheck |
| Cognitive complexity per function | ❌ | ❌ | not documented for Shell | ✅ |
| Cyclomatic complexity, length, nesting | ❌ | ❌ | not documented for Shell | ✅ |
| Duplicated code | ❌ | ❌ for shell | not documented for Shell | ✅ |
| Fail only on new issues | ❌ | main branch only | ✅ new code period, pull requests | ✅ baseline with ratchet |
| History and dashboards | ❌ | ✅ for the languages it analyses | ✅ | via Prometheus and Grafana |
| Runs as | a binary | a server (Java, a database) and a scanner | a server or SaaS | a binary |

Sources:
[SonarQube Community Build languages](https://docs.sonarsource.com/sonarqube-community-build/analyzing-source-code/languages/overview/)
(Shell is marked for Cloud and Server only),
[Shell analyzer](https://docs.sonarsource.com/sonarqube-server/analyzing-source-code/languages/shell/),
[Shell support announcement](https://community.sonarsource.com/t/support-for-shell-sh-bash-is-available-in-beta-on-sonarqube-cloud/149807)
(21 rules at launch, no ShellCheck because of its license),
[generic issue import](https://docs.sonarsource.com/sonarqube-community-build/analyzing-source-code/importing-external-issues/generic-issue-import-format/).

## ShellCheck

ShellCheck finds bugs: unquoted expansions, `cd` without a check, `read`
without `-r`, and hundreds more. bashq does not reimplement any of that. It
runs ShellCheck and merges its findings into the same report and the same
baseline.

What ShellCheck does not do is measure: it has no notion of a function that
is too long, too nested, or copied elsewhere. That is the part bashq adds.

## SonarQube

SonarQube is the reference for what bashq does: cognitive complexity, the
quality gate on new code, duplication. For bash, though:

- **Community Build, the free edition, does not analyse shell at all.** It
  can import issues computed by another tool (`bashq check --format
  sonar`), but not measures, and imported rules stay out of its quality
  profiles. You get a list of issues on a server, not a gate or a
  complexity dashboard.
- **The paid editions** analyse bash with Sonar's own rules. Their
  documentation lists no complexity or duplication measure for shell.
- The third-party
  [sonar-shellcheck](https://github.com/sbaudoin/sonar-shellcheck) plugin
  states that it is no longer maintained, and it is tested only up to
  SonarQube 9.

If your organisation already runs SonarQube for other languages, import
bashq's issues into it ([how](ci.md#sonarqube)), and keep `bashq check` as
the gate in CI.

## When to use what

- **Only bugs, no gate**: ShellCheck alone.
- **A quality gate for bash**, in any CI, with no server: bashq, which runs
  ShellCheck for you.
- **Bash issues in an existing SonarQube**: bashq's `sonar` format, plus
  `bashq check` in CI for the gate.
- **History of the measures**: `bashq metrics --format prometheus` into
  Prometheus and Grafana ([Dashboards](ci.md#dashboards)).
