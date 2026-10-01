#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
esac
case "$os" in
  mingw*|msys*|cygwin*) os=windows ;;
  darwin) os=darwin ;;
  linux) os=linux ;;
esac

suffix=""
if [[ "$os" == "windows" ]]; then
  suffix=".exe"
fi
bin="$here/bin/astack-$os-$arch$suffix"

host_matches() {
  local gobin=$1
  local host
  host=$("$gobin" env GOHOSTOS 2>/dev/null || true)
  [[ -z "$host" || "$host" == "$os" ]]
}

find_go() {
  local cands=()
  if command -v go >/dev/null 2>&1; then
    cands+=("$(command -v go)")
  fi
  local p
  for p in \
    /opt/homebrew/bin/go \
    /usr/local/go/bin/go \
    /usr/local/bin/go \
    "$HOME/go/bin/go" \
    "$HOME/.local/bin/go" \
    /usr/lib/go/bin/go
  do
    cands+=("$p")
  done
  for p in "${cands[@]}"; do
    [[ -e "$p" ]] || continue
    if host_matches "$p"; then
      printf '%s\n' "$p"
      return 0
    fi
  done
  return 1
}

needs_build() {
  local target=$1
  [[ -e "$target" ]] || return 0
  local f
  for f in "$here"/*.go "$here/agents.json" "$here/go.mod"; do
    [[ -e "$f" ]] || continue
    if [[ "$f" -nt "$target" ]]; then
      return 0
    fi
  done
  return 1
}

if needs_build "$bin"; then
  if go_bin=$(find_go); then
    mkdir -p "$here/bin"
    GOOS="$os" GOARCH="$arch" "$go_bin" build -o "$bin" "$here"
  fi
fi

if [[ -e "$bin" ]]; then
  exec "$bin" "$@"
fi

if go_bin=$(find_go); then
  exec "$go_bin" run "$here" "$@"
fi

echo "astack: no dispatcher binary; conductor should edit in this chat. not rerouting." >&2
exit 3
