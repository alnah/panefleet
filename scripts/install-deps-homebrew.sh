#!/usr/bin/env bash

set -euo pipefail

with_go="0"

while (( $# > 0 )); do
  case "$1" in
    --with-go)
      with_go="1"
      shift
      ;;
    *)
      printf 'unknown option: %s\n' "$1" >&2
      exit 1
      ;;
  esac
done

if ! command -v brew >/dev/null 2>&1; then
  printf 'Homebrew is required: https://brew.sh\n' >&2
  exit 1
fi

packages=(tmux fzf ripgrep)
if [[ "$with_go" == "1" ]]; then
  packages+=(go)
fi

brew install "${packages[@]}"

printf 'Installed core dependencies: tmux, fzf, ripgrep\n'
if [[ "$with_go" == "1" ]]; then
  printf 'Installed optional build dependency: go\n'
else
  printf 'Optional build dependency not installed: go (use --with-go)\n'
fi
