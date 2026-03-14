#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmux source-file "${repo_root}/panefleet.tmux"
tmux display-message "Sourced ${repo_root}/panefleet.tmux"
