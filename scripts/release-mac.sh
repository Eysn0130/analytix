#!/usr/bin/env bash
set -euo pipefail

# macOS controlled publication: validate an already-built, signed, notarized,
# stapled artifact set before creating local release metadata or publishing it.
# Local packages use the explicitly non-publishable path below and never create
# a release tag, notes, index, upload, or publication receipt.
#
# Usage:
#   ./scripts/release-mac.sh --tag v0.1.3 --authority receipt.json --authority-public-key release.pub
#   ./scripts/release-mac.sh --local-nonpublishable --arch arm64
#   ./scripts/release-mac.sh --tag v0.1.3 --authority receipt.json --authority-public-key release.pub --r2
#   ./scripts/release-mac.sh --tag v0.1.3 --authority receipt.json --authority-public-key release.pub --beta --r2
#   ./scripts/release-mac.sh --tag v0.1.3 --authority receipt.json --authority-public-key release.pub --r2-upload-only
#
# Release notes default: summarize conventional commits since the previous tag.
#   --notes "..."        custom text only
#   --notes-file path    markdown file
#   --no-commit-notes    generic build info only (old behavior)
#
# Upload knob:
#   RELEASE_UPLOAD_CONCURRENCY=4    R2 upload concurrency
#
# After this completes, run on Windows (same version) if needed:
#   ./scripts/release-win.sh --tag v<RELEASE_VERSION from output> --r2 --r2-promote

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=lib/release-common.sh
source "${ROOT}/scripts/lib/release-common.sh"
release_load_local_env

PUBLISH=false
CUSTOM_NOTES=""
NOTES_FILE=""
RELEASE_NOTES_FROM_COMMITS=1
RELEASE_TAG=""
P12_PATH="${P12_PATH:-${CSC_LINK:-}}"
P12_PASSWORD="${P12_PASSWORD:-${CSC_KEY_PASSWORD:-}}"
P8_PATH="${P8_PATH:-${APPLE_API_KEY:-}}"
KEY_ID="${KEY_ID:-${APPLE_API_KEY_ID:-}}"
ISSUER="${ISSUER:-${APPLE_API_ISSUER:-}}"
RELEASE_CHANNEL="${RELEASE_CHANNEL:-stable}"
R2_UPLOAD="${R2_UPLOAD:-false}"
R2_PROMOTE="${R2_PROMOTE:-false}"
RELEASE_UPLOAD_CONCURRENCY="${RELEASE_UPLOAD_CONCURRENCY:-4}"
RELEASE_AUTHORITY_PATH="${ANALYTIX_RELEASE_AUTHORITY:-}"
RELEASE_AUTHORITY_PUBLIC_KEY="${ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY:-}"
LOCAL_NONPUBLISHABLE=false
LOCAL_NONPUBLISHABLE_ARCH=""
LOCAL_NONPUBLISHABLE_OUTPUT="${ANALYTIX_LOCAL_NONPUBLISHABLE_DIST_DIR:-${ROOT}/dist-local-nonpublishable}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --publish) PUBLISH=true; shift ;;
    --r2) R2_UPLOAD=true; R2_PROMOTE=true; shift ;;
    --r2-upload-only) R2_UPLOAD=true; R2_PROMOTE=false; shift ;;
    --r2-promote) R2_UPLOAD=true; R2_PROMOTE=true; shift ;;
    --tag) RELEASE_TAG="$2"; shift 2 ;;
    --authority) RELEASE_AUTHORITY_PATH="$2"; shift 2 ;;
    --authority-public-key) RELEASE_AUTHORITY_PUBLIC_KEY="$2"; shift 2 ;;
    --local-nonpublishable) LOCAL_NONPUBLISHABLE=true; shift ;;
    --arch) LOCAL_NONPUBLISHABLE_ARCH="$2"; shift 2 ;;
    --channel) RELEASE_CHANNEL="$2"; shift 2 ;;
    --stable) RELEASE_CHANNEL=stable; shift ;;
    --beta) RELEASE_CHANNEL=beta; shift ;;
    --notes) CUSTOM_NOTES="$2"; shift 2 ;;
    --notes-file) NOTES_FILE="$2"; shift 2 ;;
    --no-commit-notes) RELEASE_NOTES_FROM_COMMITS=0; shift ;;
    --help|-h)
      awk 'NR >= 4 { if ($0 !~ /^#/) exit; sub(/^# ?/, ""); print }' "$0"
      exit 0
      ;;
    *) die "Unknown flag: $1" ;;
  esac
done

[[ "$(uname -s)" == "Darwin" ]] || die "release-mac.sh must run on macOS."

build_local_nonpublishable() {
  local arch="${LOCAL_NONPUBLISHABLE_ARCH}"
  if [[ -z "${arch}" ]]; then
    case "$(uname -m)" in
      arm64|aarch64) arch=arm64 ;;
      x86_64|amd64) arch=x64 ;;
      *) die "Unsupported local macOS architecture: $(uname -m)" ;;
    esac
  fi
  [[ "${arch}" == "arm64" || "${arch}" == "x64" ]] \
    || die "--arch must be arm64 or x64 for --local-nonpublishable"
  [[ "${R2_UPLOAD}" == "false" && "${R2_PROMOTE}" == "false" && "${PUBLISH}" == "false" ]] \
    || die "--local-nonpublishable cannot upload, promote, or publish"
  [[ -z "${RELEASE_TAG}${RELEASE_AUTHORITY_PATH}${RELEASE_AUTHORITY_PUBLIC_KEY}" ]] \
    || die "--local-nonpublishable cannot accept a release tag or publication authority"
  [[ -z "${P12_PATH}${P12_PASSWORD}${P8_PATH}${KEY_ID}${ISSUER}" ]] \
    || die "--local-nonpublishable cannot accept Developer ID or notary credentials"

  release_check_prerequisites
  unset ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE ANALYTIX_RELEASE_BUILD MAC_SIGN \
    CSC_LINK CSC_NAME CSC_KEY_PASSWORD APPLE_API_KEY APPLE_API_KEY_BASE64 \
    APPLE_API_KEY_ID APPLE_API_ISSUER 2>/dev/null || true
  cyan "Building explicitly non-publishable local macOS ${arch} app -> ${LOCAL_NONPUBLISHABLE_OUTPUT}"
  node "${ROOT}/scripts/build-data-analysis-native-tools.cjs" \
    --development --platform darwin --arch "${arch}" \
    || die "Local non-publishable native development build failed"
  npm run build || die "Local non-publishable renderer/main build failed"
  ANALYTIX_DIST_DIR="${LOCAL_NONPUBLISHABLE_OUTPUT}" \
    npx --yes electron-builder@26.15.3 --config electron-builder.config.cjs \
      --publish never --mac --dir "--${arch}" \
    || die "Local non-publishable macOS package failed"
  green "Local non-publishable app is ready."
  cyan "  Output: ${LOCAL_NONPUBLISHABLE_OUTPUT}"
  cyan "  Authority: development-only packaged marker; no release tag, notes, index, upload, or publication receipt was created."
}

verify_formal_publication_authority() {
  [[ -n "${RELEASE_TAG}" ]] \
    || die "Formal macOS publication requires --tag vX.Y.Z; automatic version bump is not an authority."
  [[ -n "${RELEASE_AUTHORITY_PATH}" ]] \
    || die "release_publication_authority_missing"
  [[ -n "${RELEASE_AUTHORITY_PUBLIC_KEY}" ]] \
    || die "release_publication_authority_public_key_missing"
  command -v node >/dev/null 2>&1 || die "node not found — install Node.js >= 22"
  command -v git >/dev/null 2>&1 || die "git not found"
  RELEASE_CHANNEL="$(release_normalize_channel "${RELEASE_CHANNEL}")"
  node "${ROOT}/scripts/publish-r2.mjs" verify \
    --platform mac \
    --tag "${RELEASE_TAG}" \
    --channel "${RELEASE_CHANNEL}" \
    --dist "${ROOT}/dist" \
    --authority "${RELEASE_AUTHORITY_PATH}" \
    --public-key "${RELEASE_AUTHORITY_PUBLIC_KEY}" \
    || die "Formal macOS publication authority preflight failed before build, tag, notes, index, or upload"
}

if [[ "${LOCAL_NONPUBLISHABLE}" == "true" ]]; then
  build_local_nonpublishable
  exit 0
fi
[[ -z "${LOCAL_NONPUBLISHABLE_ARCH}" ]] \
  || die "--arch is only valid with --local-nonpublishable; formal publication consumes the authority-bound artifact set."

# This is intentionally the first formal-release operation. It is read-only
# and blocks before lock creation, artifact cleanup/build, tagging, notes,
# release-index writes, or any network request.
verify_formal_publication_authority
SIGNING=true
release_acquire_lock

cyan "Computing release version..."
RELEASE_BUMP=none
release_compute_version

cyan "  Base:    ${BASE_VERSION}"
cyan "  Latest:  ${LATEST_TAG:-<none>}"
cyan "  Next:    ${RELEASE_VERSION}  (tag: ${TAG_NAME})"
release_export_update_channel
release_export_app_version

release_ensure_tag_available

cyan "Using the exact prebuilt artifact set bound by the controlled publication authority."

release_write_meta_file

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

collect_optional() {
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
    yellow "  • ${label}: none"
    return
  fi

  for file in "${matched[@]}"; do
    ASSETS+=("${file}")
    green "  ✓ ${label}: ${file}"
  done
}

# artifactName: analytix-${version}-mac-${arch}.dmg|zip
collect "macOS arm64 dmg" "dist/analytix-*-mac-arm64.dmg"
collect "macOS x64 dmg" "dist/analytix-*-mac-x64.dmg"
collect "macOS arm64 zip" "dist/analytix-*-mac-arm64.zip"
collect "macOS x64 zip" "dist/analytix-*-mac-x64.zip"
collect_optional "macOS blockmap" "dist/analytix-*-mac-*.zip.blockmap"

NOTES_TMP=$(mktemp "${TMPDIR:-/tmp}/release-notes.XXXXXX")
NOTES_OUT="${ROOT}/dist/analytix-${RELEASE_VERSION}-release-notes.md"
release_write_notes_file "${NOTES_TMP}"
cp "${NOTES_TMP}" "${NOTES_OUT}"

release_create_local_tag
release_write_local_release_index "${NOTES_OUT}" "${ASSETS[@]}"

if [[ "${R2_UPLOAD}" == "true" ]]; then
  cyan "Uploading macOS asset metadata to R2 (${TAG_NAME})..."
  node "${ROOT}/scripts/publish-r2.mjs" upload --platform mac --tag "${TAG_NAME}" --channel "${RELEASE_CHANNEL}" \
    --authority "${RELEASE_AUTHORITY_PATH}" --public-key "${RELEASE_AUTHORITY_PUBLIC_KEY}" \
    || die "R2 upload failed for macOS assets"
fi

if [[ "${R2_PROMOTE}" == "true" ]]; then
  cyan "Promoting ${TAG_NAME} as R2 latest..."
  node "${ROOT}/scripts/publish-r2.mjs" promote --tag "${TAG_NAME}" --channel "${RELEASE_CHANNEL}" \
    --platforms mac --authority "${RELEASE_AUTHORITY_PATH}" --public-key "${RELEASE_AUTHORITY_PUBLIC_KEY}" \
    || die "R2 promote failed"
fi

release_maybe_publish_notice

rm -f "${NOTES_TMP}"

echo
green "macOS release ${TAG_NAME} ready locally."
cyan "  Meta: dist/.release-meta.env"
cyan "  Notes: ${NOTES_OUT}"
cyan "  Channel: ${RELEASE_CHANNEL}"
cyan "  Next on Windows: ./scripts/release-win.sh --tag ${TAG_NAME} --channel ${RELEASE_CHANNEL}"
