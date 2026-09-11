#!/bin/zsh

# Canonical configured-host cache verification. This is a development-volume
# check, not packaged-product or release-signing evidence.

set -eu
setopt pipefail

analytix_cache_verify_fail() {
  print -u2 -- "[analytix-cache] $1"
  exit 1
}

analytix_cache_verify_script_dir="${0:A:h}"
if ! source "$analytix_cache_verify_script_dir/use-analytix-cache.sh"; then
  analytix_cache_verify_fail 'cache authority helper rejected the host'
fi

analytix_cache_verify_mount='/Volumes/AnalytixCache'
analytix_cache_verify_root='/Volumes/AnalytixCache/development-v3'
analytix_cache_verify_evidence="$analytix_cache_verify_root/evidence"
analytix_cache_verify_image='/Volumes/DataSSD/analytix/AnalytixCache-v3.sparsebundle'
analytix_cache_verify_volume_uuid='090478FC-CE2B-4E25-98F1-667C3252DD42'
analytix_cache_verify_apfs_volume_type='41504653-0000-11AA-AA11-00306543ECAC'
analytix_cache_verify_mount_job_label='com.analytix.mount-cache'
analytix_cache_verify_mount_job_plist='/Users/sun/Library/LaunchAgents/com.analytix.mount-cache.plist'
analytix_cache_verify_mount_job_restore='false'
analytix_cache_verify_probe=''

analytix_cache_verify_cleanup() {
  if [ -n "${analytix_cache_verify_probe:-}" ] &&
    [ -d "$analytix_cache_verify_probe" ] &&
    [ ! -L "$analytix_cache_verify_probe" ] &&
    [[ "${analytix_cache_verify_probe:A}" == "$analytix_cache_verify_root/tmp/"* ]] &&
    [ "$(/usr/bin/stat -f '%d' "$analytix_cache_verify_probe")" = "${analytix_cache_verify_device:-}" ] &&
    [ "$(/usr/bin/stat -f '%u' "$analytix_cache_verify_probe")" = "${analytix_cache_verify_uid:-}" ]; then
    /bin/rm -rf -- "$analytix_cache_verify_probe"
  fi
  if [ "${analytix_cache_verify_mount_job_restore:-false}" = 'true' ]; then
    if ! /bin/launchctl bootstrap "gui/$analytix_cache_verify_uid" \
      "$analytix_cache_verify_mount_job_plist"; then
      print -u2 -- '[analytix-cache] failed to restore the configured cache mount job'
    fi
  fi
}
trap analytix_cache_verify_cleanup EXIT

analytix_cache_verify_plist_field() {
  local field="$1"
  /usr/bin/plutil -extract "$field" raw -o - -
}

analytix_cache_verify_image_member() {
  local kind="$1"
  /usr/bin/hdiutil info | /usr/bin/awk \
    -v target="$analytix_cache_verify_image" \
    -v apfs="$analytix_cache_verify_apfs_volume_type" \
    -v kind="$kind" '
    /^image-path[[:space:]]*:/ {
      path=$0
      sub(/^image-path[[:space:]]*:[[:space:]]*/, "", path)
      selected=(path == target)
      next
    }
    selected && kind == "whole" &&
    $1 ~ /^\/dev\/disk[0-9]+$/ && $2 == "GUID_partition_scheme" {
      print $1
    }
    selected && kind == "volume" &&
    $1 ~ /^\/dev\/disk[0-9]+s[0-9]+$/ && $2 == apfs {
      print $1
    }
  '
}

if [ "${ANALYTIX_DEV_CACHE_ROOT:-}" != "$analytix_cache_verify_root" ] ||
  [ "${TMPDIR:-}" != "$analytix_cache_verify_root/tmp" ] ||
  [ "${GOCACHE:-}" != "$analytix_cache_verify_root/go-build" ] ||
  [ "${GOMODCACHE:-}" != "$analytix_cache_verify_root/go-mod" ] ||
  [ "${GOTMPDIR:-}" != "$analytix_cache_verify_root/tmp" ] ||
  [ "$(umask)" != '077' ]; then
  analytix_cache_verify_fail 'cache environment or owner-only umask is not authoritative'
fi

# diskutil verifyVolume is deliberately completed before this script creates
# its probe directory or evidence receipt. On this configured host Disk
# Arbitration can remount an otherwise valid disk image with noowners after a
# successful verification. Suspend only the exact task-owned mount job during
# that transition, then reattach only the already verified v3 image with
# owners enabled before any cache write.
analytix_cache_verify_uid=$(/usr/bin/id -u) ||
  analytix_cache_verify_fail 'cannot resolve cache owner identity'
analytix_cache_verify_mount_job_service="gui/$analytix_cache_verify_uid/$analytix_cache_verify_mount_job_label"
if analytix_cache_verify_mount_job_state=$(
  /bin/launchctl print "$analytix_cache_verify_mount_job_service" 2>/dev/null
); then
  if ! print -r -- "$analytix_cache_verify_mount_job_state" |
    /usr/bin/grep -Fq "path = $analytix_cache_verify_mount_job_plist"; then
    analytix_cache_verify_fail 'configured cache mount job identity is not authoritative'
  fi
  if ! /bin/launchctl bootout "$analytix_cache_verify_mount_job_service"; then
    analytix_cache_verify_fail 'cannot suspend the configured cache mount job'
  fi
  analytix_cache_verify_mount_job_restore='true'
fi

if ! /usr/sbin/diskutil verifyVolume "$analytix_cache_verify_mount"; then
  analytix_cache_verify_fail 'diskutil verifyVolume rejected the mounted cache'
fi

analytix_cache_verify_post_info=$(
  /usr/sbin/diskutil info -plist "$analytix_cache_verify_mount" 2>/dev/null
) || analytix_cache_verify_fail 'verified cache did not return to its exact mount point'
analytix_cache_verify_post_device=$(
  print -rn -- "$analytix_cache_verify_post_info" |
    analytix_cache_verify_plist_field DeviceIdentifier
) || analytix_cache_verify_fail 'cannot resolve post-verify cache device'
analytix_cache_verify_post_mount=$(
  print -rn -- "$analytix_cache_verify_post_info" |
    analytix_cache_verify_plist_field MountPoint
) || analytix_cache_verify_fail 'cannot resolve post-verify cache mount point'
analytix_cache_verify_post_filesystem=$(
  print -rn -- "$analytix_cache_verify_post_info" |
    analytix_cache_verify_plist_field FilesystemType
) || analytix_cache_verify_fail 'cannot resolve post-verify cache filesystem'
analytix_cache_verify_post_writable=$(
  print -rn -- "$analytix_cache_verify_post_info" |
    analytix_cache_verify_plist_field Writable
) || analytix_cache_verify_fail 'cannot resolve post-verify cache write authority'
analytix_cache_verify_post_owners=$(
  print -rn -- "$analytix_cache_verify_post_info" |
    analytix_cache_verify_plist_field GlobalPermissionsEnabled
) || analytix_cache_verify_fail 'cannot resolve post-verify cache ownership authority'
analytix_cache_verify_post_uuid=$(
  print -rn -- "$analytix_cache_verify_post_info" |
    analytix_cache_verify_plist_field VolumeUUID
) || analytix_cache_verify_fail 'cannot resolve post-verify cache volume identity'
analytix_cache_verify_post_whole="$(analytix_cache_verify_image_member whole)"
analytix_cache_verify_post_volume="$(analytix_cache_verify_image_member volume)"
analytix_cache_verify_post_mount_line=$(
  /sbin/mount | /usr/bin/awk \
    -v dev="/dev/$analytix_cache_verify_post_device" \
    -v target="$analytix_cache_verify_mount" '
      $1 == dev && $2 == "on" && $3 == target { print }
    '
)

if [ "$analytix_cache_verify_post_mount" != "$analytix_cache_verify_mount" ] ||
  [ "$analytix_cache_verify_post_filesystem" != 'apfs' ] ||
  [ "$analytix_cache_verify_post_writable" != 'true' ] ||
  [ "$analytix_cache_verify_post_uuid" != "$analytix_cache_verify_volume_uuid" ] ||
  [ "$analytix_cache_verify_post_volume" != "/dev/$analytix_cache_verify_post_device" ] ||
  [[ "$analytix_cache_verify_post_whole" != /dev/disk<-> ]] ||
  [ -z "$analytix_cache_verify_post_mount_line" ] ||
  [[ "$analytix_cache_verify_post_mount_line" != *"(apfs,"* ]] ||
  [[ "$analytix_cache_verify_post_mount_line" == *read-only* ]] ||
  [[ "$analytix_cache_verify_post_mount_line" == *", ro,"* ]] ||
  [[ "$analytix_cache_verify_post_mount_line" == *", ro)"* ]]; then
  analytix_cache_verify_fail 'post-verify cache identity or write authority changed'
fi

if [ "$analytix_cache_verify_post_owners" != 'true' ] ||
  [[ "$analytix_cache_verify_post_mount_line" == *noowners* ]]; then
  if [ "$analytix_cache_verify_post_owners" != 'false' ] ||
    [[ "$analytix_cache_verify_post_mount_line" != *noowners* ]]; then
    analytix_cache_verify_fail 'post-verify ownership state is ambiguous'
  fi
  analytix_cache_verify_open_pids=$(
    /usr/sbin/lsof -t +f -- "$analytix_cache_verify_mount" 2>/dev/null || true
  )
  if [ -n "$analytix_cache_verify_open_pids" ]; then
    analytix_cache_verify_fail 'cache was reopened before ownership could be restored'
  fi
  if ! /usr/bin/hdiutil detach "$analytix_cache_verify_post_whole" >/dev/null; then
    analytix_cache_verify_fail 'cannot detach the exact verified v3 cache image'
  fi
  if [ -n "$(analytix_cache_verify_image_member whole)" ] ||
    [ -n "$(analytix_cache_verify_image_member volume)" ]; then
    analytix_cache_verify_fail 'verified v3 cache image remained attached after detach'
  fi
  if ! /usr/bin/hdiutil attach \
    -owners on \
    -nobrowse \
    -noautoopen \
    -mountpoint "$analytix_cache_verify_mount" \
    "$analytix_cache_verify_image" >/dev/null; then
    analytix_cache_verify_fail 'cannot reattach the exact verified v3 cache image with owners enabled'
  fi
fi

if ! source "$analytix_cache_verify_script_dir/use-analytix-cache.sh"; then
  analytix_cache_verify_fail 'post-verify cache authority helper rejected the host'
fi

if [ "$analytix_cache_verify_mount_job_restore" = 'true' ]; then
  if ! /bin/launchctl bootstrap "gui/$analytix_cache_verify_uid" \
    "$analytix_cache_verify_mount_job_plist"; then
    analytix_cache_verify_fail 'cannot restore the configured cache mount job'
  fi
  analytix_cache_verify_mount_job_restore='false'
fi

analytix_cache_verify_device=$(/usr/bin/stat -f '%d' "$analytix_cache_verify_mount") ||
  analytix_cache_verify_fail 'cannot resolve cache filesystem identity'

analytix_cache_verify_probe=$(
  /usr/bin/mktemp -d "$TMPDIR/analytix-cache-verification.XXXXXXXX"
) || analytix_cache_verify_fail 'cannot allocate a probe on the trusted cache'

if [ "${analytix_cache_verify_probe:A}" != "$analytix_cache_verify_probe" ] ||
  [ "$(/usr/bin/stat -f '%Lp' "$analytix_cache_verify_probe")" != '700' ] ||
  [ "$(/usr/bin/stat -f '%d' "$analytix_cache_verify_probe")" != "$analytix_cache_verify_device" ] ||
  [ "$(/usr/bin/stat -f '%u' "$analytix_cache_verify_probe")" != "$analytix_cache_verify_uid" ]; then
  analytix_cache_verify_fail 'probe directory escaped the private cache namespace'
fi

for analytix_cache_verify_directory in \
  "$analytix_cache_verify_probe/home" \
  "$analytix_cache_verify_probe/tmp" \
  "$analytix_cache_verify_probe/xdg" \
  "$analytix_cache_verify_probe/go-build" \
  "$analytix_cache_verify_probe/go-mod" \
  "$analytix_cache_verify_probe/bin"; do
  /bin/mkdir -m 700 "$analytix_cache_verify_directory" ||
    analytix_cache_verify_fail "cannot create private probe directory: $analytix_cache_verify_directory"
done

analytix_cache_verify_source="$analytix_cache_verify_probe/main.go"
analytix_cache_verify_cold_log="$analytix_cache_verify_probe/cold.log"
analytix_cache_verify_warm_log="$analytix_cache_verify_probe/warm.log"
analytix_cache_verify_receipt_body="$analytix_cache_verify_probe/receipt"
for analytix_cache_verify_file in \
  "$analytix_cache_verify_source" \
  "$analytix_cache_verify_cold_log" \
  "$analytix_cache_verify_warm_log" \
  "$analytix_cache_verify_receipt_body"; do
  /usr/bin/install -m 600 /dev/null "$analytix_cache_verify_file" ||
    analytix_cache_verify_fail "cannot create mode-0600 probe output: $analytix_cache_verify_file"
done

/usr/bin/printf '%s\n' \
  'package main' \
  'import "fmt"' \
  'func main() { fmt.Println("analytix-cache-verification-v1") }' \
  >| "$analytix_cache_verify_source"

analytix_cache_verify_go_candidate="$(command -v go 2>/dev/null || true)"
if [ -z "$analytix_cache_verify_go_candidate" ]; then
  analytix_cache_verify_fail 'Go toolchain is unavailable for the cold/warm probe'
fi
analytix_cache_verify_go="${analytix_cache_verify_go_candidate:A}"
if [ ! -f "$analytix_cache_verify_go" ] || [ ! -x "$analytix_cache_verify_go" ]; then
  analytix_cache_verify_fail 'resolved Go toolchain is not an executable regular file'
fi

analytix_cache_verify_build() {
  local output="$1"
  local log="$2"
  /usr/bin/env -i \
    PATH="${analytix_cache_verify_go:h}:/usr/bin:/bin:/usr/sbin:/sbin" \
    HOME="$analytix_cache_verify_probe/home" \
    TMPDIR="$analytix_cache_verify_probe/tmp" \
    XDG_CACHE_HOME="$analytix_cache_verify_probe/xdg" \
    GOCACHE="$analytix_cache_verify_probe/go-build" \
    GOMODCACHE="$analytix_cache_verify_probe/go-mod" \
    GOTMPDIR="$analytix_cache_verify_probe/tmp" \
    GOENV=off \
    GOPROXY=off \
    GOSUMDB=off \
    GOTOOLCHAIN=local \
    GO111MODULE=off \
    CGO_ENABLED=0 \
    "$analytix_cache_verify_go" build -x -trimpath -buildvcs=false \
      -o "$output" "$analytix_cache_verify_source" >| "$log" 2>&1
}

analytix_cache_verify_cold="$analytix_cache_verify_probe/bin/probe-cold"
analytix_cache_verify_warm="$analytix_cache_verify_probe/bin/probe-warm"
analytix_cache_verify_signed="$analytix_cache_verify_probe/bin/probe-signed"

if /usr/bin/find -x "$analytix_cache_verify_probe/go-build" -mindepth 1 -print -quit |
  /usr/bin/grep -q .; then
  analytix_cache_verify_fail 'cold probe cache was not empty'
fi
analytix_cache_verify_build "$analytix_cache_verify_cold" "$analytix_cache_verify_cold_log" ||
  analytix_cache_verify_fail 'cold Go cache build failed'
if ! /usr/bin/find -x "$analytix_cache_verify_probe/go-build" -type f -print -quit |
  /usr/bin/grep -q .; then
  analytix_cache_verify_fail 'cold build did not populate its trusted cache'
fi
if ! /usr/bin/grep -Eq '/compile([[:space:]]|$)' "$analytix_cache_verify_cold_log"; then
  analytix_cache_verify_fail 'cold build did not expose a compiler action'
fi

analytix_cache_verify_build "$analytix_cache_verify_warm" "$analytix_cache_verify_warm_log" ||
  analytix_cache_verify_fail 'warm Go cache build failed'
if /usr/bin/grep -Eq '/compile([[:space:]]|$)' "$analytix_cache_verify_warm_log"; then
  analytix_cache_verify_fail 'warm build unexpectedly repeated a compiler action'
fi

for analytix_cache_verify_binary in \
  "$analytix_cache_verify_cold" \
  "$analytix_cache_verify_warm"; do
  if [ ! -f "$analytix_cache_verify_binary" ] ||
    [ -L "$analytix_cache_verify_binary" ] ||
    [ "$(/usr/bin/stat -f '%Lp' "$analytix_cache_verify_binary")" != '700' ] ||
    [ "$(/usr/bin/stat -f '%d' "$analytix_cache_verify_binary")" != "$analytix_cache_verify_device" ] ||
    [ "$(/usr/bin/stat -f '%u' "$analytix_cache_verify_binary")" != "$analytix_cache_verify_uid" ] ||
    [ "$($analytix_cache_verify_binary)" != 'analytix-cache-verification-v1' ]; then
    analytix_cache_verify_fail "built probe identity or execution failed: $analytix_cache_verify_binary"
  fi
done

analytix_cache_verify_sha256() {
  /usr/bin/shasum -a 256 "$1" | /usr/bin/awk '{print $1}'
}

analytix_cache_verify_cold_sha256="$(analytix_cache_verify_sha256 "$analytix_cache_verify_cold")"
analytix_cache_verify_warm_sha256="$(analytix_cache_verify_sha256 "$analytix_cache_verify_warm")"
if [ "$analytix_cache_verify_cold_sha256" != "$analytix_cache_verify_warm_sha256" ] ||
  ! /usr/bin/cmp -s "$analytix_cache_verify_cold" "$analytix_cache_verify_warm"; then
  analytix_cache_verify_fail 'cold and warm build outputs are not byte-identical'
fi

/bin/cp "$analytix_cache_verify_warm" "$analytix_cache_verify_signed" ||
  analytix_cache_verify_fail 'cannot create codesign probe copy'
/bin/chmod 700 "$analytix_cache_verify_signed" ||
  analytix_cache_verify_fail 'cannot protect codesign probe copy'
if ! /usr/bin/codesign --force --sign - --timestamp=none \
  --identifier com.analytix.cache-verification "$analytix_cache_verify_signed"; then
  analytix_cache_verify_fail 'ad-hoc codesign probe failed'
fi
if ! /usr/bin/codesign --verify --strict --verbose=2 "$analytix_cache_verify_signed" ||
  [ "$($analytix_cache_verify_signed)" != 'analytix-cache-verification-v1' ]; then
  analytix_cache_verify_fail 'signed probe verification or execution failed'
fi
analytix_cache_verify_signed_sha256="$(analytix_cache_verify_sha256 "$analytix_cache_verify_signed")"
analytix_cache_verify_cold_log_sha256="$(analytix_cache_verify_sha256 "$analytix_cache_verify_cold_log")"
analytix_cache_verify_warm_log_sha256="$(analytix_cache_verify_sha256 "$analytix_cache_verify_warm_log")"

# The probe can take long enough for an external mount transition to occur.
# Re-run the complete authority helper immediately before publishing a PASS
# receipt so a changed device, mount, image, ownership, or private namespace
# cannot inherit the earlier verification.
if ! source "$analytix_cache_verify_script_dir/use-analytix-cache.sh"; then
  analytix_cache_verify_fail 'final cache authority helper rejected the host'
fi

analytix_cache_verify_info=$(
  /usr/sbin/diskutil info -plist "$analytix_cache_verify_mount"
) || analytix_cache_verify_fail 'cannot read verified cache volume identity'
analytix_cache_verify_volume_uuid=$(
  print -rn -- "$analytix_cache_verify_info" |
    /usr/bin/plutil -extract VolumeUUID raw -o - -
) || analytix_cache_verify_fail 'cannot read verified cache volume UUID'
analytix_cache_verify_device_identifier=$(
  print -rn -- "$analytix_cache_verify_info" |
    /usr/bin/plutil -extract DeviceIdentifier raw -o - -
) || analytix_cache_verify_fail 'cannot read verified cache device identifier'

/usr/bin/printf '%s\n' \
  'schemaVersion=1' \
  'scope=configured-development-cache-only' \
  "volumeUUID=$analytix_cache_verify_volume_uuid" \
  "deviceIdentifier=$analytix_cache_verify_device_identifier" \
  "coldSha256=$analytix_cache_verify_cold_sha256" \
  "warmSha256=$analytix_cache_verify_warm_sha256" \
  "signedSha256=$analytix_cache_verify_signed_sha256" \
  "coldLogSha256=$analytix_cache_verify_cold_log_sha256" \
  "warmLogSha256=$analytix_cache_verify_warm_log_sha256" \
  'verifyVolume=pass' \
  'coldWarmCache=pass' \
  'codesign=pass' \
  'status=PASS' \
  >| "$analytix_cache_verify_receipt_body"

analytix_cache_verify_receipt=$(
  /usr/bin/mktemp "$analytix_cache_verify_evidence/cache-verification-v1.receipt.XXXXXXXX"
) || analytix_cache_verify_fail 'cannot allocate a private cache verification receipt'
/bin/chmod 600 "$analytix_cache_verify_receipt" ||
  analytix_cache_verify_fail 'cannot protect cache verification receipt'
/usr/bin/install -m 600 "$analytix_cache_verify_receipt_body" "$analytix_cache_verify_receipt" ||
  analytix_cache_verify_fail 'cannot publish cache verification receipt'

print -r -- "[analytix-cache] PASS receipt=$analytix_cache_verify_receipt"
