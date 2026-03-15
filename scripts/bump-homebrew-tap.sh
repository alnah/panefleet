#!/usr/bin/env bash

set -euo pipefail

REPO_SLUG="${PANEFLEET_REPO_SLUG:-alnah/panefleet}"
TAP_REPO="${PANEFLEET_HOMEBREW_TAP_REPO:-alnah/homebrew-tap}"
FORMULA_PATH="${PANEFLEET_HOMEBREW_FORMULA_PATH:-Formula/panefleet.rb}"
RELEASE_TAG="${PANEFLEET_RELEASE_TAG:-${GITHUB_REF_NAME:-}}"
TAP_TOKEN="${PANEFLEET_HOMEBREW_TAP_TOKEN:-${HOMEBREW_TAP_TOKEN:-}}"

if [[ -z "${RELEASE_TAG}" ]]; then
  printf 'panefleet: release tag missing (set PANEFLEET_RELEASE_TAG or GITHUB_REF_NAME).\n' >&2
  exit 1
fi

if [[ -z "${TAP_TOKEN}" ]]; then
  printf 'panefleet: HOMEBREW_TAP_TOKEN not set; skipping Homebrew tap bump.\n'
  exit 0
fi

if ! command -v curl >/dev/null 2>&1; then
  printf 'panefleet: curl is required.\n' >&2
  exit 1
fi

if ! command -v git >/dev/null 2>&1; then
  printf 'panefleet: git is required.\n' >&2
  exit 1
fi

archive_url="https://github.com/${REPO_SLUG}/archive/refs/tags/${RELEASE_TAG}.tar.gz"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/panefleet-homebrew.XXXXXX")"
archive_path="${tmp_dir}/panefleet.tar.gz"
tap_dir="${tmp_dir}/tap"
trap 'rm -rf "${tmp_dir}"' EXIT

curl -fSL "${archive_url}" -o "${archive_path}"

if command -v sha256sum >/dev/null 2>&1; then
  archive_sha="$(sha256sum "${archive_path}" | awk '{print $1}')"
else
  archive_sha="$(shasum -a 256 "${archive_path}" | awk '{print $1}')"
fi

git clone "https://x-access-token:${TAP_TOKEN}@github.com/${TAP_REPO}.git" "${tap_dir}" >/dev/null 2>&1

formula_file="${tap_dir}/${FORMULA_PATH}"
if [[ ! -f "${formula_file}" ]]; then
  printf 'panefleet: formula file not found: %s\n' "${FORMULA_PATH}" >&2
  exit 1
fi

# shellcheck disable=SC2016
PANEFLEET_ARCHIVE_URL="${archive_url}" PANEFLEET_ARCHIVE_SHA="${archive_sha}" ruby -i -pe '
if $_ =~ /^  url "/
  $_ = "  url \"#{ENV.fetch("PANEFLEET_ARCHIVE_URL")}\"\n"
elsif $_ =~ /^  sha256 "/
  $_ = "  sha256 \"#{ENV.fetch("PANEFLEET_ARCHIVE_SHA")}\"\n"
end
' "${formula_file}"

(
  cd "${tap_dir}"
  if git diff --quiet -- "${FORMULA_PATH}"; then
    printf 'panefleet: Homebrew formula already up to date for %s.\n' "${RELEASE_TAG}"
    exit 0
  fi

  git config user.name "${GIT_AUTHOR_NAME:-panefleet-bot}"
  git config user.email "${GIT_AUTHOR_EMAIL:-panefleet-bot@users.noreply.github.com}"
  git add "${FORMULA_PATH}"
  git commit -m "chore(homebrew): bump panefleet ${RELEASE_TAG}" >/dev/null
  git push origin HEAD:main >/dev/null
)

printf 'panefleet: Homebrew tap updated for %s.\n' "${RELEASE_TAG}"
