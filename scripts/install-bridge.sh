#!/usr/bin/env bash

set -euo pipefail

# install-bridge.sh prefers a released bridge binary, then falls back to local
# build only when necessary. This keeps default installs fast and portable while
# still allowing source-only environments to recover.

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${PANEFLEET_ROOT:-$(CDPATH='' cd -- "${SCRIPT_DIR}/.." && pwd)}"
STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"
OUTPUT_BIN="${PANEFLEET_AGENT_BRIDGE_BIN:-$STATE_HOME/panefleet/bin/panefleet-agent-bridge}"
BRIDGE_REPO="${PANEFLEET_BRIDGE_REPO:-alnah/panefleet}"
INSTALL_MODE="${PANEFLEET_BRIDGE_INSTALL_MODE:-auto}"

bridge_ready() {
  [[ -x "$OUTPUT_BIN" ]]
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
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
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
  local asset url tmpdir extracted

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
  if ! curl -fsSL --retry 2 "$url" -o "${tmpdir}/${asset}"; then
    rm -rf "$tmpdir"
    return 1
  fi

  tar -xzf "${tmpdir}/${asset}" -C "$tmpdir"
  extracted="$(find "$tmpdir" -type f -name 'panefleet-agent-bridge' | head -n 1)"
  if [[ -z "$extracted" || ! -f "$extracted" ]]; then
    rm -rf "$tmpdir"
    printf 'panefleet: downloaded archive did not contain panefleet-agent-bridge\n' >&2
    return 1
  fi

  mkdir -p "$(dirname "$OUTPUT_BIN")"
  cp "$extracted" "$OUTPUT_BIN"
  chmod 755 "$OUTPUT_BIN"
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
    auto|build|download|force-build|force-download)
      ;;
    *)
      printf 'unknown PANEFLEET_BRIDGE_INSTALL_MODE: %s\n' "$INSTALL_MODE" >&2
      exit 1
      ;;
  esac

  if bridge_ready; then
    case "$INSTALL_MODE" in
      auto|build|download)
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
