#!/usr/bin/env bash

set -euo pipefail

# install-bridge.sh prefers a released bridge binary, then falls back to local
# build only when necessary. This keeps default installs fast and portable while
# still allowing source-only environments to recover.

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/panefleet/runtime/paths.sh"

REPO_ROOT="${PANEFLEET_ROOT:-$(panefleet_find_repo_root_from "${BASH_SOURCE[0]}")}"
STATE_HOME="$(panefleet_user_state_home)"
OUTPUT_BIN="${PANEFLEET_AGENT_BRIDGE_BIN:-$STATE_HOME/panefleet/bin/panefleet-agent-bridge}"
BRIDGE_REPO="${PANEFLEET_BRIDGE_REPO:-alnah/panefleet}"
INSTALL_MODE="${PANEFLEET_BRIDGE_INSTALL_MODE:-auto}"

bridge_ready() {
  [[ -x "$OUTPUT_BIN" ]]
}

is_safe_tar_path() {
  local entry="$1"
  local cleaned="${entry#./}"

  if [[ -z "$cleaned" ]]; then
    return 1
  fi
  if [[ "$cleaned" == /* ]]; then
    return 1
  fi
  if [[ "$cleaned" == *'..'* ]]; then
    case "$cleaned" in
    *'/../'* | '../'* | *'/..' | '..')
      return 1
      ;;
    esac
  fi
  return 0
}

extract_bridge_entry() {
  local archive="$1"
  local selected=""
  local selected_count=0
  local entry cleaned

  while IFS= read -r entry; do
    [[ -z "$entry" ]] && continue
    cleaned="${entry#./}"
    [[ "$cleaned" == */ ]] && continue
    if ! is_safe_tar_path "$cleaned"; then
      continue
    fi
    if [[ "$(basename "$cleaned")" == "panefleet-agent-bridge" ]]; then
      selected="$entry"
      selected_count=$((selected_count + 1))
    fi
  done < <(tar -tzf "$archive")

  if ((selected_count != 1)); then
    printf 'panefleet: archive must contain exactly one panefleet-agent-bridge entry\n' >&2
    return 1
  fi

  printf '%s\n' "$selected"
}

install_bridge_atomically() {
  local source_bin="$1"
  local output_dir tmp_out backup=""

  output_dir="$(dirname "$OUTPUT_BIN")"
  if [[ -L "$output_dir" ]]; then
    printf 'panefleet: refusing to install bridge through symlinked output directory %s\n' "$output_dir" >&2
    return 1
  fi
  if [[ -L "$OUTPUT_BIN" ]]; then
    printf 'panefleet: refusing to install bridge through symlinked output path %s\n' "$OUTPUT_BIN" >&2
    return 1
  fi

  mkdir -p "$output_dir"
  chmod 700 "$output_dir" 2>/dev/null || true
  tmp_out="$(mktemp "${output_dir}/.panefleet-agent-bridge.XXXXXX")"
  cp "$source_bin" "$tmp_out"
  chmod 755 "$tmp_out"

  if [[ -f "$OUTPUT_BIN" ]]; then
    backup="$(mktemp "${output_dir}/.panefleet-agent-bridge.bak.XXXXXX")"
    cp "$OUTPUT_BIN" "$backup"
  fi

  if ! mv -f "$tmp_out" "$OUTPUT_BIN"; then
    rm -f "$tmp_out"
    if [[ -n "$backup" && -f "$backup" ]]; then
      mv -f "$backup" "$OUTPUT_BIN" 2>/dev/null || true
    fi
    printf 'panefleet: failed to install bridge binary atomically\n' >&2
    return 1
  fi

  rm -f "$backup" 2>/dev/null || true
  return 0
}

# normalize_os and normalize_arch restrict assets to known release targets.
# Failing early avoids silent installs of incompatible binaries.
normalize_os() {
  case "$(uname -s)" in
  Darwin) printf 'darwin' ;;
  Linux) printf 'linux' ;;
  *)
    printf 'unsupported operating system: %s\n' "$(uname -s)" >&2
    return 1
    ;;
  esac
}

normalize_arch() {
  case "$(uname -m)" in
  x86_64 | amd64) printf 'amd64' ;;
  arm64 | aarch64) printf 'arm64' ;;
  *)
    printf 'unsupported architecture: %s\n' "$(uname -m)" >&2
    return 1
    ;;
  esac
}

bridge_asset_name() {
  local os="$1"
  local arch="$2"

  printf 'panefleet-agent-bridge_%s_%s.tar.gz' "$os" "$arch"
}

# exact_checkout_tag lets source checkouts consume matching release assets.
# When unavailable, the installer can still fall back to latest/build paths.
exact_checkout_tag() {
  if ! command -v git >/dev/null 2>&1; then
    return 0
  fi

  git -C "$REPO_ROOT" describe --tags --exact-match 2>/dev/null || true
}

# download_bridge performs a verified single-purpose download/unpack flow.
# It never pipes remote content to a shell and checks asset contents explicitly.
download_bridge() {
  local version="$1"
  local os="$2"
  local arch="$3"
  local asset url tmpdir archive extracted_entry extracted_bin

  if ! command -v curl >/dev/null 2>&1; then
    printf 'curl is required to download a prebuilt panefleet bridge.\n' >&2
    return 1
  fi

  if ! command -v tar >/dev/null 2>&1; then
    printf 'tar is required to unpack a prebuilt panefleet bridge.\n' >&2
    return 1
  fi

  asset="$(bridge_asset_name "$os" "$arch")"
  if [[ -n "$version" ]]; then
    url="https://github.com/${BRIDGE_REPO}/releases/download/${version}/${asset}"
  else
    url="https://github.com/${BRIDGE_REPO}/releases/latest/download/${asset}"
  fi

  tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/panefleet-bridge.XXXXXX")"
  archive="${tmpdir}/${asset}"
  if ! curl -fsSL --retry 2 "$url" -o "$archive"; then
    rm -rf "$tmpdir"
    return 1
  fi

  if ! extracted_entry="$(extract_bridge_entry "$archive")"; then
    rm -rf "$tmpdir"
    return 1
  fi

  extracted_bin="${tmpdir}/panefleet-agent-bridge"
  if ! tar -xOf "$archive" "$extracted_entry" >"$extracted_bin"; then
    rm -rf "$tmpdir"
    printf 'panefleet: failed to extract panefleet-agent-bridge from archive\n' >&2
    return 1
  fi

  if ! install_bridge_atomically "$extracted_bin"; then
    rm -rf "$tmpdir"
    return 1
  fi

  rm -rf "$tmpdir"
  printf 'Installed prebuilt bridge %s\n' "$OUTPUT_BIN"
}

# build_bridge remains a fallback to preserve operability without release assets.
build_bridge() {
  if ! command -v go >/dev/null 2>&1; then
    return 1
  fi

  PANEFLEET_ROOT="$REPO_ROOT" PANEFLEET_AGENT_BRIDGE_BIN="$OUTPUT_BIN" "${SCRIPT_DIR}/build-agent-bridge.sh"
}

# main enforces an explicit fallback order to keep behavior deterministic:
# exact-tag download -> local build -> latest download.
main() {
  local os arch version

  case "$INSTALL_MODE" in
  auto | build | download | force-build | force-download) ;;
  *)
    printf 'unknown PANEFLEET_BRIDGE_INSTALL_MODE: %s\n' "$INSTALL_MODE" >&2
    exit 1
    ;;
  esac

  if bridge_ready; then
    case "$INSTALL_MODE" in
    auto | build | download)
      printf 'Bridge already installed %s\n' "$OUTPUT_BIN"
      return
      ;;
    esac
  fi

  os="$(normalize_os)"
  arch="$(normalize_arch)"
  version="${PANEFLEET_BRIDGE_VERSION:-$(exact_checkout_tag)}"

  case "$INSTALL_MODE" in
  force-build)
    if ! build_bridge; then
      printf 'panefleet: Go is required to build the bridge from source.\n' >&2
      exit 1
    fi
    return
    ;;
  force-download)
    if ! download_bridge "$version" "$os" "$arch"; then
      printf 'panefleet: failed to download a prebuilt bridge for %s/%s.\n' "$os" "$arch" >&2
      exit 1
    fi
    return
    ;;
  build)
    if ! build_bridge; then
      printf 'panefleet: Go is required to build the bridge from source.\n' >&2
      exit 1
    fi
    return
    ;;
  download)
    if ! download_bridge "$version" "$os" "$arch"; then
      printf 'panefleet: failed to download a prebuilt bridge for %s/%s.\n' "$os" "$arch" >&2
      exit 1
    fi
    return
    ;;
  esac

  if [[ -n "$version" ]] && download_bridge "$version" "$os" "$arch"; then
    return
  fi

  if build_bridge; then
    return
  fi

  if download_bridge "" "$os" "$arch"; then
    return
  fi

  printf 'panefleet: could not install the bridge automatically.\n' >&2
  printf 'Try one of:\n' >&2
  printf '  1. install Go and rerun this command\n' >&2
  printf '  2. publish/download a release asset for %s/%s from %s\n' "$os" "$arch" "$BRIDGE_REPO" >&2
  exit 1
}

main "$@"
