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

run_install() {
  if [[ "$(id -u)" == "0" ]]; then
    "$@"
    return
  fi

  if command -v sudo >/dev/null 2>&1; then
    sudo "$@"
    return
  fi

  "$@"
}

packages=(tmux fzf ripgrep)
if [[ "$with_go" == "1" ]]; then
  packages+=(go)
fi

if command -v brew >/dev/null 2>&1; then
  brew install "${packages[@]}"
elif command -v apt-get >/dev/null 2>&1; then
  run_install apt-get update
  run_install apt-get install -y "${packages[@]}"
elif command -v dnf >/dev/null 2>&1; then
  run_install dnf install -y "${packages[@]}"
elif command -v pacman >/dev/null 2>&1; then
  run_install pacman -Sy --noconfirm "${packages[@]}"
else
  printf 'unsupported package manager. install manually: %s\n' "${packages[*]}" >&2
  exit 1
fi

printf 'Installed core dependencies: tmux, fzf, ripgrep\n'
if [[ "$with_go" == "1" ]]; then
  printf 'Installed optional build dependency: go\n'
else
  printf 'Optional build dependency not installed: go (use --with-go)\n'
fi
