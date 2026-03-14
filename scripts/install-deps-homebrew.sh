#!/usr/bin/env bash

set -euo pipefail

if ! command -v brew >/dev/null 2>&1; then
  printf 'Homebrew is required: https://brew.sh\n' >&2
  exit 1
fi

brew install tmux fzf ripgrep go

printf 'Installed dependencies: tmux, fzf, ripgrep, go\n'
