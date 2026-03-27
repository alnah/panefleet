#!/usr/bin/env bash

set -euo pipefail

# install-deps.sh installs only the minimum runtime tools required by panefleet.
# It keeps dependency ownership explicit: core deps are automatic, optional Go is
# opt-in to avoid surprising users who only consume release binaries.

with_go="0"
missing_packages=()
package_manager=""

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

  printf 'elevation required to run: %s\n' "$*" >&2
  printf 'install-deps: run as root or install sudo\n' >&2
  return 1
}

# append_missing_package tracks missing commands first, then performs one package
# manager call, which keeps install output predictable and easier to audit.
append_missing_package() {
  local command_name="$1"
  local package_name="$2"

  if ! command -v "$command_name" >/dev/null 2>&1; then
    missing_packages+=("$package_name")
  fi
}

# detect_package_manager intentionally supports only common package managers.
# Unsupported systems fail fast with a manual list instead of guessing.
detect_package_manager() {
  if command -v brew >/dev/null 2>&1; then
    package_manager="brew"
  elif command -v apt-get >/dev/null 2>&1; then
    package_manager="apt-get"
  elif command -v dnf >/dev/null 2>&1; then
    package_manager="dnf"
  elif command -v pacman >/dev/null 2>&1; then
    package_manager="pacman"
  else
    package_manager=""
  fi
}

append_missing_package tmux tmux
append_missing_package fzf fzf
append_missing_package rg ripgrep
if [[ "$with_go" == "1" ]]; then
  append_missing_package go go
fi

if (( ${#missing_packages[@]} == 0 )); then
  printf 'Core dependencies already present: tmux, fzf, ripgrep\n'
  if [[ "$with_go" == "1" ]]; then
    printf 'Optional build dependency already present: go\n'
  else
    printf 'Optional build dependency not installed: go (use --with-go)\n'
  fi
  exit 0
fi

detect_package_manager
if [[ -z "$package_manager" ]]; then
  printf 'unsupported package manager. install manually: %s\n' "${missing_packages[*]}" >&2
  exit 1
fi

printf 'Installing missing system packages with %s: %s\n' "$package_manager" "${missing_packages[*]}"

if [[ "$package_manager" == "brew" ]]; then
  brew install "${missing_packages[@]}"
elif [[ "$package_manager" == "apt-get" ]]; then
  run_install apt-get update
  run_install apt-get install -y "${missing_packages[@]}"
elif [[ "$package_manager" == "dnf" ]]; then
  run_install dnf install -y "${missing_packages[@]}"
elif [[ "$package_manager" == "pacman" ]]; then
  run_install pacman -Sy --noconfirm "${missing_packages[@]}"
fi

printf 'Installed core dependencies: tmux, fzf, ripgrep\n'
if [[ "$with_go" == "1" ]]; then
  printf 'Installed optional build dependency: go\n'
else
  printf 'Optional build dependency not installed: go (use --with-go)\n'
fi
