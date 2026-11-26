# Contributing

## Checks to run before a pull request

```bash
gofmt -l .                       # prints nothing
go vet ./...
go test -race ./...
go test ./metrics -run '^$' -fuzz FuzzAnalyze -fuzztime 30s   # optional
```

CI runs the same checks, and more:

| Job | Checks |
| --- | --- |
| `lint` | `gofmt`, `go vet`, `go.mod` tidy, the GoReleaser configuration |
| `test (ubuntu-latest, macos-latest)` | the whole suite with the race detector; coverage in the job summary |
| `fuzz` | one minute of fuzzing of the analyzer: no panic, increments that add up |
| `example` | `bashq check` with ShellCheck on `examples/deploy`: exactly its two known issues |
| `PR title` | the title follows Conventional Commits (below) |

## Where things are

| Path | Holds |
| --- | --- |
| `metrics/` | the measures: the syntax-tree walker (`walker.go`) and line counts |
| `dup/` | duplicated-code detection |
| `internal/discover/` | which files are scripts, exclude globs |
| `internal/config/` | `.bashq.yaml` |
| `internal/check/` | rules, analysis pipeline, baseline, ShellCheck |
| `internal/report/` | every output format |
| `cmd/bashq/` | the command line, and its golden tests in `testdata/` |
| `examples/deploy/` | the example of the README and the docs; the CLI tests run on it |

## Tests

- Every change to a measure comes with a case in `metrics/metrics_test.go`:
  the bash snippet and the expected score, named after the behaviour
  (`"test || return is a hidden if"`). Update [docs/metrics.md](docs/metrics.md)
  in the same pull request: it is the specification.
- The command line is tested end to end on `examples/deploy`, against the
  files in `cmd/bashq/testdata/*.golden`. After an intended output change:

  ```bash
  go test ./cmd/bashq -update
  git diff cmd/bashq/testdata   # review every change
  ```

  The docs quote these outputs: update them too.
- ShellCheck is faked in tests (`internal/check/check_test.go`), since its
  output changes between versions.

## Commit messages and pull request titles

Releases are computed from them, so they follow
[Conventional Commits](https://www.conventionalcommits.org):

```text
<type>: <what changes, in the imperative>
```

| Type | For | Next version (from 0.x) | In the changelog |
| --- | --- | --- | --- |
| `feat` | a new feature | minor: 0.1.0 → 0.2.0 | Features |
| `fix` | a bug fix | patch: 0.1.0 → 0.1.1 | Bug Fixes |
| `perf` | faster, same behaviour | patch | Performance |
| `docs` | documentation only | patch | Documentation |
| `refactor`, `test`, `ci`, `chore` | everything else | none on its own | hidden |

A change to how a measure is counted changes users' scores and can fail
their checks: it is a `feat!` (or a `BREAKING CHANGE:` footer), even when it
fixes a miscount. Before 1.0 it bumps the minor version; from 1.0 on, the
major one.

Pull requests are merged with **squash**: the pull request title becomes the
commit message on `main`, which is why the `PR title` check enforces the
format.

## Releases

Nothing to do by hand but merging:

1. Every push to `main` runs the `Release` workflow. It opens or updates a
   pull request named `chore: release X.Y.Z`, which:
   - adds the `X.Y.Z` section to `CHANGELOG.md`, from the commits since the
     last release;
   - sets the version in `version.txt`, `.release-please-manifest.json`
     and the version shown in the docs.
2. Merging that pull request:
   - tags `vX.Y.Z`;
   - publishes the GitHub release, with the changelog section as its notes;
   - builds the binaries with GoReleaser (Linux and macOS, amd64 and
     arm64, version stamped in) and attaches them with `checksums.txt`.

`go install github.com/XavierChevalier/bashq/cmd/bashq@vX.Y.Z` works as soon
as the tag exists.

To force a version, add a commit with a `Release-As: 1.0.0` footer:

```bash
git commit --allow-empty -m "chore: release 1.0.0" -m "Release-As: 1.0.0"
```

### Repository settings the workflow needs

Set once, in [Settings › Actions › General](https://github.com/XavierChevalier/bashq/settings/actions):

- **Workflow permissions**: *Read and write permissions*.
- **Allow GitHub Actions to create and approve pull requests**: checked
  (release-please opens the release pull request).

And in [Settings › General › Pull Requests](https://github.com/XavierChevalier/bashq/settings):
*Allow squash merging* checked, with *Default to pull request title*.

Optional: the release pull request is opened with the workflow's own token,
and GitHub does not start CI for it. To get CI on it too, create a
fine-grained token with *Contents* and *Pull requests* read and write on this
repository, and save it as the `RELEASE_PLEASE_TOKEN` secret in
[Settings › Secrets › Actions](https://github.com/XavierChevalier/bashq/settings/secrets/actions).
