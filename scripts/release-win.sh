#!/usr/bin/env bash
set -euo pipefail

# Windows controlled publication: validate an already-built, Authenticode-signed
# artifact set before writing local metadata, archiving, uploading, or promoting.
# Local packages use the explicit non-publishable path and never create release
# metadata, an installer/update feed, an archive, or a publication receipt.
#
# Usage:
#   ./scripts/release-win.sh --tag v0.1.1 --authority receipt.json --authority-public-key release.pub
#   ./scripts/release-win.sh --tag v0.1.1 --authority receipt.json --authority-public-key release.pub --r2 --r2-promote
#   ./scripts/release-win.sh --local-nonpublishable
#
# Native PowerShell (Git Bash not required):
#   .\scripts\release-win.ps1 -Tag v0.1.1 -Authority receipt.json -AuthorityPublicKey release.pub -R2 -PromoteR2

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=lib/release-common.sh
source "${ROOT}/scripts/lib/release-common.sh"
# Do not source release.local.env from the formal Windows entrypoint. A shell
# fragment is executable code and could mutate tags, metadata, or artifacts
# before the controlled publication authority is verified. Pass secrets and
# authority paths through the process environment or explicit flags instead.

PUBLISH=false
RELEASE_TAG=""
CUSTOM_NOTES=""
NOTES_FILE=""
RELEASE_NOTES_FROM_COMMITS=1
REQUESTED_RELEASE_CHANNEL="${RELEASE_CHANNEL:-stable}"
R2_UPLOAD="${R2_UPLOAD:-false}"
R2_PROMOTE="${R2_PROMOTE:-false}"
NO_CACHE=false
RELEASE_AUTHORITY_PATH="${ANALYTIX_RELEASE_AUTHORITY:-}"
RELEASE_AUTHORITY_PUBLIC_KEY="${ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY:-}"
LOCAL_NONPUBLISHABLE=false
LOCAL_NONPUBLISHABLE_OUTPUT="${ANALYTIX_LOCAL_NONPUBLISHABLE_WIN_DIST_DIR:-${ROOT}/dist-local-nonpublishable-win}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --publish) PUBLISH=true; shift ;;
    --tag) RELEASE_TAG="$2"; shift 2 ;;
    --authority) RELEASE_AUTHORITY_PATH="$2"; shift 2 ;;
    --authority-public-key) RELEASE_AUTHORITY_PUBLIC_KEY="$2"; shift 2 ;;
    --local-nonpublishable) LOCAL_NONPUBLISHABLE=true; shift ;;
    --channel) REQUESTED_RELEASE_CHANNEL="$2"; shift 2 ;;
    --stable) REQUESTED_RELEASE_CHANNEL=stable; shift ;;
    --beta) REQUESTED_RELEASE_CHANNEL=beta; shift ;;
    --r2) R2_UPLOAD=true; shift ;;
    --r2-promote) R2_UPLOAD=true; R2_PROMOTE=true; shift ;;
    --no-cache) NO_CACHE=true; shift ;;
    --help|-h)
      sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) die "Unknown flag: $1" ;;
  esac
done

WINDOWS_SIGNING_CERT="${WIN_CSC_LINK:-${CSC_LINK:-${ANALYTIX_WIN_SIGNING_CERT_PATH:-}}}"
WINDOWS_SIGNING_PASSWORD="${WIN_CSC_KEY_PASSWORD:-${CSC_KEY_PASSWORD:-${ANALYTIX_WIN_SIGNING_CERT_PASSWORD:-}}}"

require_windows_host() {
  case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*|Windows*) ;;
    *) die "release-win.sh must run on Windows (or MSYS/Git Bash on Windows)." ;;
  esac
}

build_local_nonpublishable() {
  [[ "${R2_UPLOAD}" == "false" && "${R2_PROMOTE}" == "false" && "${PUBLISH}" == "false" ]] \
    || die "--local-nonpublishable cannot upload, promote, or publish"
  [[ -z "${RELEASE_TAG}${RELEASE_AUTHORITY_PATH}${RELEASE_AUTHORITY_PUBLIC_KEY}" ]] \
    || die "--local-nonpublishable cannot accept a release tag or publication authority"
  [[ -z "${WINDOWS_SIGNING_CERT}${WINDOWS_SIGNING_PASSWORD}${ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1:-}" ]] \
    || die "--local-nonpublishable cannot accept signing credentials or a release signer pin"
  require_windows_host
  release_check_prerequisites
  LOCAL_NONPUBLISHABLE_OUTPUT="$(node -e 'process.stdout.write(require("node:path").resolve(process.argv[1]))' "${LOCAL_NONPUBLISHABLE_OUTPUT}")"
  case "${LOCAL_NONPUBLISHABLE_OUTPUT}" in
    "${ROOT}/dist"|"${ROOT}/dist/"*|"${ROOT}/dist-standard-win"|"${ROOT}/dist-standard-win/"*)
      die "Local non-publishable output must be isolated from formal release directories."
      ;;
  esac
  [[ ! -e "${LOCAL_NONPUBLISHABLE_OUTPUT}" ]] \
    || die "Local non-publishable output already exists: ${LOCAL_NONPUBLISHABLE_OUTPUT}"
  unset ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE ANALYTIX_RELEASE_BUILD \
    ANALYTIX_RELEASE_AUTHORITY ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY ANALYTIX_RELEASE_ENV \
    WIN_CSC_LINK CSC_LINK ANALYTIX_WIN_SIGNING_CERT_PATH \
    WIN_CSC_KEY_PASSWORD CSC_KEY_PASSWORD ANALYTIX_WIN_SIGNING_CERT_PASSWORD \
    ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1 CARGO_TARGET_DIR 2>/dev/null || true
  export ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA=1
  if $NO_CACHE; then
    export ANALYTIX_RELEASE_CACHE_DISABLE=1
  fi
  cyan "Building explicitly non-publishable local Windows x64 app -> ${LOCAL_NONPUBLISHABLE_OUTPUT}"
  node "${ROOT}/scripts/build-data-analysis-native-tools.cjs" \
    --development --platform win32 --arch x64 \
    || die "Local non-publishable native development build failed"
  npm run build || die "Local non-publishable renderer/main build failed"
  ANALYTIX_DIST_DIR="${LOCAL_NONPUBLISHABLE_OUTPUT}" \
    npx --yes electron-builder@26.15.3 --config electron-builder.config.cjs \
      --publish never --win --dir --x64 \
    || die "Local non-publishable Windows package failed"
  green "Local non-publishable Windows app is ready."
  cyan "  Output: ${LOCAL_NONPUBLISHABLE_OUTPUT}"
  cyan "  Authority: development-only packaged marker; no release tag, metadata, installer/update feed, archive, upload, or publication receipt was created."
}

verify_formal_publication_authority() {
  [[ "${RELEASE_TAG}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] \
    || die "Formal Windows publication requires --tag vX.Y.Z; local metadata is not an authority."
  [[ -n "${RELEASE_AUTHORITY_PATH}" ]] || die "release_publication_authority_missing"
  [[ -n "${RELEASE_AUTHORITY_PUBLIC_KEY}" ]] || die "release_publication_authority_public_key_missing"
  [[ -n "${WINDOWS_SIGNING_CERT}" && -n "${WINDOWS_SIGNING_PASSWORD}" ]] \
    || die "Official Windows release requires a signing certificate and password."
  [[ "${ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1:-}" =~ ^[A-Fa-f0-9]{40}$ ]] \
    || die "Official Windows release requires ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1 (40 hexadecimal characters)."
  [[ -z "${ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA:-}" ]] \
    || die "Official Windows release forbids ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA."
  command -v node >/dev/null 2>&1 || die "node not found — install Node.js >= 22"
  command -v git >/dev/null 2>&1 || die "git not found"
  RELEASE_CHANNEL="$(release_normalize_channel "${REQUESTED_RELEASE_CHANNEL}")"
  node "${ROOT}/scripts/publish-r2.mjs" verify \
    --platform win \
    --tag "${RELEASE_TAG}" \
    --channel "${RELEASE_CHANNEL}" \
    --dist "${ROOT}/dist-standard-win" \
    --authority "${RELEASE_AUTHORITY_PATH}" \
    --public-key "${RELEASE_AUTHORITY_PUBLIC_KEY}" \
    || die "Formal Windows publication authority preflight failed before build, tag, metadata, archive, or upload"
}

if [[ "${LOCAL_NONPUBLISHABLE}" == "true" ]]; then
  build_local_nonpublishable
  exit 0
fi
[[ "${NO_CACHE}" == "false" ]] \
  || die "--no-cache is only valid with --local-nonpublishable; formal publication consumes prebuilt authority-bound artifacts."

# This is intentionally the first formal-release operation. It is read-only
# and blocks before host lock creation, artifact cleanup/build, tagging,
# metadata/index/archive writes, or any network request.
verify_formal_publication_authority
require_windows_host
release_acquire_lock

export ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1="$(printf '%s' "${ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1}" | tr '[:lower:]' '[:upper:]')"
export WIN_CSC_LINK="${WINDOWS_SIGNING_CERT}"
export CSC_LINK="${WINDOWS_SIGNING_CERT}"
export ANALYTIX_WIN_SIGNING_CERT_PATH="${WINDOWS_SIGNING_CERT}"
export WIN_CSC_KEY_PASSWORD="${WINDOWS_SIGNING_PASSWORD}"
export CSC_KEY_PASSWORD="${WINDOWS_SIGNING_PASSWORD}"
export ANALYTIX_WIN_SIGNING_CERT_PASSWORD="${WINDOWS_SIGNING_PASSWORD}"
unset CARGO_TARGET_DIR 2>/dev/null || true

RELEASE_BUMP=none
release_compute_version
release_export_update_channel
release_export_app_version
cyan "Using the exact prebuilt Windows artifact set bound by the controlled publication authority."

cyan "Running Windows package audit..."
node "${ROOT}/scripts/audit-windows-package.cjs" "${RELEASE_VERSION}" \
  || die "Windows package audit failed"

VERIFY_DB_DIR="${TMPDIR:-/tmp}/analytix-data-engine-single-owner-release"
mkdir -p "${VERIFY_DB_DIR}"
cyan "Running data engine single-owner packaged gate..."
node "${ROOT}/scripts/verify-data-engine-single-owner.cjs" \
  --runtime-dir "${ROOT}/dist-standard-win/win-unpacked/resources/runtime" \
  --db-path "${VERIFY_DB_DIR}/case.duckdb" \
  --case-id release-gate \
  || die "Data engine single-owner packaged gate failed"

ASSETS=()
collect() {
  local label="$1"
  shift
  local matched=()
  local pattern file

  shopt -s nullglob
  for pattern in "$@"; do
    for file in ${pattern}; do
      [[ -f "${file}" ]] || continue
      matched+=("${file}")
    done
  done
  shopt -u nullglob

  if [[ ${#matched[@]} -eq 0 ]]; then
    red "  ✗ ${label}"
    die "Missing asset: ${label}"
  fi

  for file in "${matched[@]}"; do
    ASSETS+=("${file}")
    green "  ✓ ${label}: ${file}"
  done
}

collect "Standard Windows installer" "dist-standard-win/analytix-standard-*-x64.exe"
collect "Standard Windows blockmap" "dist-standard-win/analytix-standard-*-x64.exe.blockmap"
collect "Standard Windows update metadata" "dist-standard-win/latest.yml"

NOTES_OUT="${ROOT}/dist/analytix-${RELEASE_VERSION}-windows-release-notes.md"
release_write_notes_file "${NOTES_OUT}"
release_write_local_release_index "${NOTES_OUT}" "${ASSETS[@]}"

if [[ "${R2_UPLOAD}" == "true" ]]; then
  cyan "Uploading Windows asset metadata to R2 (${TAG_NAME})..."
  node "${ROOT}/scripts/publish-r2.mjs" upload --platform win --tag "${TAG_NAME}" --channel "${RELEASE_CHANNEL}" --dist "${ROOT}/dist-standard-win" \
    --authority "${RELEASE_AUTHORITY_PATH}" --public-key "${RELEASE_AUTHORITY_PUBLIC_KEY}" \
    || die "R2 upload failed for Windows assets"
fi

if [[ "${R2_PROMOTE}" == "true" ]]; then
  cyan "Promoting ${TAG_NAME} as R2 latest..."
  node "${ROOT}/scripts/publish-r2.mjs" promote --tag "${TAG_NAME}" --channel "${RELEASE_CHANNEL}" --platforms win \
    --authority "${RELEASE_AUTHORITY_PATH}" --public-key "${RELEASE_AUTHORITY_PUBLIC_KEY}" \
    || die "R2 promote failed"
fi

release_maybe_publish_notice

echo
green "Windows release ${TAG_NAME} ready locally."
cyan "  Channel: ${RELEASE_CHANNEL}"
