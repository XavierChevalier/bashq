#!/usr/bin/env bash
# Deploys a release of the app to a list of hosts.
set -euo pipefail

readonly RELEASES=/srv/app/releases

deploy::usage() {
  echo "usage: deploy.sh [-n] <version> <host>..." >&2
}

# Uploads, unpacks and switches every host to <version>; -n prints instead.
deploy::main() {
  local dry_run=false
  if [[ "${1:-}" == "-n" ]]; then
    dry_run=true
    shift
  fi
  [[ $# -ge 2 ]] || { deploy::usage; return 2; }
  local version="$1"
  shift

  for host in "$@"; do
    if ! ssh "${host}" true; then
      echo "skip ${host}: unreachable" >&2
      continue
    fi
    if [[ "${dry_run}" == true ]]; then
      echo "would deploy ${version} to ${host}"
    else
      for attempt in 1 2 3; do
        if scp "app-${version}.tar.gz" "${host}:${RELEASES}/"; then
          break
        elif [[ "${attempt}" -eq 3 ]]; then
          echo "upload to ${host} failed" >&2
          return 1
        else
          sleep $((attempt * 2))
        fi
      done
      deploy::switch "${host}" "${version}"
    fi
  done
}

# Points the current symlink of <host> to <version> and restarts the app.
deploy::switch() {
  local host="$1" version="$2"
  ssh "${host}" "
    set -e
    cd ${RELEASES}
    tar -xzf app-${version}.tar.gz
    ln -sfn ${RELEASES}/app-${version} /srv/app/current
  "
  ssh "${host}" sudo systemctl restart app
  local status
  status="$(ssh "${host}" systemctl is-active app)"
  if [[ "${status}" != active ]]; then
    echo "${host}: app is ${status} after the switch" >&2
    journalctl_tail "${host}"
    return 1
  fi
  echo "${host}: app ${version} is active"
  curl --fail --silent --max-time 5 "http://${host}:8080/health" >/dev/null
  echo "${host}: health check passed"
}

journalctl_tail() {
  ssh "$1" journalctl -u app -n 20 --no-pager >&2
}

deploy::main "$@"
