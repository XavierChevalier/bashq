[Documentation](README.md) › Installation

# Installation

bashq is one static binary. [ShellCheck](https://www.shellcheck.net) is
optional: with it on the `PATH`, `bashq check` merges its findings.

## With Go

```bash
go install github.com/XavierChevalier/bashq/cmd/bashq@latest
bashq version
```

It needs Go ≥ 1.26 and installs to `$(go env GOPATH)/bin`, which must be on
your `PATH`. Pin a version in CI: `@v0.1.0`.

## A release binary

Each [release](https://github.com/XavierChevalier/bashq/releases/latest)
has archives for Linux and macOS (amd64, arm64), with a `checksums.txt`.

```bash
version=0.2.0 # x-release-please-version
os=linux arch=amd64
curl -fsSLO "https://github.com/XavierChevalier/bashq/releases/download/v${version}/bashq_${version}_${os}_${arch}.tar.gz"
curl -fsSLO "https://github.com/XavierChevalier/bashq/releases/download/v${version}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt
tar -xzf "bashq_${version}_${os}_${arch}.tar.gz" bashq
install bashq ~/.local/bin/
```

## From source

```bash
git clone https://github.com/XavierChevalier/bashq.git
cd bashq
go build -o bashq ./cmd/bashq
```

## ShellCheck

| System | Command |
| --- | --- |
| Debian, Ubuntu | `sudo apt-get install shellcheck` |
| Fedora | `sudo dnf install ShellCheck` |
| Alpine | `apk add shellcheck` |
| macOS | `brew install shellcheck` |
| GitHub Actions (Ubuntu) | preinstalled |

Without it, `bashq check` runs everything else and says so: the summary
lists `ShellCheck` only when it ran. Set
[`shellcheck.enabled: always`](configuration.md#shellcheck) in CI to fail
instead.

## Check it works

```console
$ bashq version
bashq 0.1.0
$ bashq check --no-baseline .
✓ No issues (12 files, 48 functions, ShellCheck)
```
