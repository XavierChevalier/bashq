#!/usr/bin/env bash
# Rolls the app of one host back to a previous release.
set -euo pipefail

readonly RELEASES=/srv/app/releases

# Points the current symlink of <host> to <version> and restarts the app.
rollback::switch() {
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

rollback::switch "$1" "$2"
