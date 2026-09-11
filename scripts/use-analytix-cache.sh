#!/bin/zsh
# Source this file before local macOS development commands:
#   source ./scripts/use-analytix-cache.sh

if [[ "$ZSH_EVAL_CONTEXT" != *:file ]]; then
  print -u2 'Source this file so its cache environment applies to your command shell.'
  exit 64
fi

analytix_cache_helper_source="${(%):-%N}"
analytix_cache_helper_path="${analytix_cache_helper_source:A}"
analytix_cache_storage_library="${analytix_cache_helper_path:h}/analytix-cache-storage.zsh"

analytix_cache_preflight() {
emulate -L zsh
unsetopt xtrace

analytix_cache_mount='/Volumes/AnalytixCache'
analytix_cache_root="$analytix_cache_mount/development-v3"
analytix_cache_backing_mount='/Volumes/DataSSD'
analytix_cache_image='/Volumes/DataSSD/analytix/AnalytixCache-v3.sparsebundle'
analytix_damaged_v2_cache_image='/Volumes/DataSSD/analytix/AnalytixCache-v2.sparsebundle'
analytix_legacy_cache_image='/Volumes/DataSSD/analytix/AnalytixCache.sparsebundle'
analytix_cache_backing_volume_uuid='DE25E209-CF5A-3364-B7D8-EF992B64F732'
analytix_cache_volume_uuid='090478FC-CE2B-4E25-98F1-667C3252DD42'
analytix_apfs_volume_type='41504653-0000-11AA-AA11-00306543ECAC'
analytix_storage_profiler='/usr/sbin/system_profiler'

analytix_cache_fail() {
  printf '%s\n' "$1" >&2
}

analytix_cache_mount_entry() {
  local target="$1"
  /sbin/mount | /usr/bin/awk -v target="$target" '
    $2 == "on" && $3 == target { print }
  '
}

analytix_cache_image_volume() {
  local target="$1"
  print -rn -- "$analytix_cache_hdiutil_info" |
    /usr/bin/awk -v target="$target" -v apfs="$analytix_apfs_volume_type" '
    /^image-path[[:space:]]*:/ {
      path=$0
      sub(/^image-path[[:space:]]*:[[:space:]]*/, "", path)
      selected=(path == target)
      next
    }
    selected &&
    $1 ~ /^\/dev\/disk[0-9]+s[0-9]+$/ &&
    $2 == apfs {
      print $1
    }
  '
}

analytix_cache_image_attachment_count() {
  local target="$1"
  print -rn -- "$analytix_cache_hdiutil_info" |
    /usr/bin/awk -v target="$target" '
    BEGIN { count=0 }
    /^image-path[[:space:]]*:/ {
      path=$0
      sub(/^image-path[[:space:]]*:[[:space:]]*/, "", path)
      if (path == target) count++
    }
    END { print count }
  '
}

if [ ! -f "$analytix_cache_storage_library" ] ||
  [ -L "$analytix_cache_storage_library" ] ||
  [ "${analytix_cache_storage_library:A}" != "$analytix_cache_storage_library" ] ||
  ! source "$analytix_cache_storage_library"; then
  analytix_cache_fail 'Cannot load the Analytix cache storage-metadata parser.'
  return 1 2>/dev/null || exit 1
fi

if [ ! -x "$analytix_storage_profiler" ] ||
  ! analytix_cache_storage_info=$(
    LC_ALL=C "$analytix_storage_profiler" SPStorageDataType -json -detailLevel mini \
      -timeout 15 2>/dev/null
  ) || [ -z "$analytix_cache_storage_info" ]; then
  analytix_cache_fail 'Cannot resolve read-only host storage metadata for the Analytix cache.'
  return 1 2>/dev/null || exit 1
fi
if ! analytix_cache_backing_index=$(
  analytix_cache_storage_index "$analytix_cache_backing_mount"
) || ! analytix_cache_backing_bsd_name=$(
  analytix_cache_storage_field "$analytix_cache_backing_index" bsd_name
) || ! analytix_cache_actual_backing_uuid=$(
  analytix_cache_storage_field "$analytix_cache_backing_index" volume_uuid
) || ! analytix_cache_backing_writable=$(
  analytix_cache_storage_field "$analytix_cache_backing_index" writable
) || ! analytix_cache_backing_mount_line=$(
  analytix_cache_mount_entry "$analytix_cache_backing_mount"
); then
  analytix_cache_fail 'Cannot resolve the Analytix cache backing-volume authority.'
  return 1 2>/dev/null || exit 1
fi
if [[ ! "$analytix_cache_backing_bsd_name" =~ '^disk[0-9]+s[0-9]+$' ]] ||
  [ "$analytix_cache_actual_backing_uuid" != "$analytix_cache_backing_volume_uuid" ] ||
  [ "$analytix_cache_backing_writable" != 'yes' ] ||
  [[ "$analytix_cache_backing_mount_line" == *$'\n'* ]] ||
  [[ "$analytix_cache_backing_mount_line" != "/dev/$analytix_cache_backing_bsd_name on $analytix_cache_backing_mount ("* ]] ||
  [[ "$analytix_cache_backing_mount_line" == *read-only* ]] ||
  [[ "$analytix_cache_backing_mount_line" == *", ro,"* ]] ||
  [[ "$analytix_cache_backing_mount_line" == *", ro)"* ]] ||
  [ ! -w "$analytix_cache_backing_mount" ]; then
  analytix_cache_fail 'Analytix cache backing-volume identity is not authoritative.'
  return 1 2>/dev/null || exit 1
fi
if ! analytix_cache_backing_device_id=$(
  /usr/bin/stat -f '%d' "$analytix_cache_backing_mount"
); then
  analytix_cache_fail 'Cannot resolve the Analytix cache backing-volume filesystem identity.'
  return 1 2>/dev/null || exit 1
fi
analytix_cache_bundle="$analytix_cache_image"
if [ ! -d "$analytix_cache_bundle" ] ||
  [ -L "$analytix_cache_bundle" ] ||
  [ "${analytix_cache_bundle:A}" != "$analytix_cache_bundle" ]; then
  analytix_cache_fail "Analytix cache sparsebundle identity is unsafe: $analytix_cache_bundle"
  return 1 2>/dev/null || exit 1
fi
if ! analytix_cache_bundle_device_id=$(
  /usr/bin/stat -f '%d' "$analytix_cache_bundle"
) || [ "$analytix_cache_bundle_device_id" != "$analytix_cache_backing_device_id" ]; then
  analytix_cache_fail "Analytix cache sparsebundle escaped the trusted backing volume: $analytix_cache_bundle"
  return 1 2>/dev/null || exit 1
fi

for analytix_retired_cache_image in \
  "$analytix_damaged_v2_cache_image" \
  "$analytix_legacy_cache_image"; do
  if [ ! -e "$analytix_retired_cache_image" ] &&
    [ ! -L "$analytix_retired_cache_image" ]; then
    continue
  fi
  if [ ! -d "$analytix_retired_cache_image" ] ||
    [ -L "$analytix_retired_cache_image" ] ||
    [ "${analytix_retired_cache_image:A}" != "$analytix_retired_cache_image" ]; then
    analytix_cache_fail "Retired Analytix cache sparsebundle identity is unsafe: $analytix_retired_cache_image"
    return 1 2>/dev/null || exit 1
  fi
  if ! analytix_cache_bundle_device_id=$(
    /usr/bin/stat -f '%d' "$analytix_retired_cache_image"
  ) || [ "$analytix_cache_bundle_device_id" != "$analytix_cache_backing_device_id" ]; then
    analytix_cache_fail "Retired Analytix cache sparsebundle escaped the trusted backing volume: $analytix_retired_cache_image"
    return 1 2>/dev/null || exit 1
  fi
done

if ! analytix_cache_hdiutil_info=$(/usr/bin/hdiutil info); then
  analytix_cache_fail 'Cannot resolve attached Analytix cache images.'
  return 1 2>/dev/null || exit 1
fi
for analytix_damaged_cache_image in \
  "$analytix_damaged_v2_cache_image" \
  "$analytix_legacy_cache_image"; do
  analytix_damaged_cache_attachments="$(
    analytix_cache_image_attachment_count "$analytix_damaged_cache_image"
  )"
  if [ "$analytix_damaged_cache_attachments" != '0' ]; then
    analytix_cache_fail "Damaged Analytix cache image is attached: $analytix_damaged_cache_image."
    return 1 2>/dev/null || exit 1
  fi
done

analytix_cache_active_attachments="$(
  analytix_cache_image_attachment_count "$analytix_cache_image"
)"
if [ "$analytix_cache_active_attachments" != '1' ]; then
  analytix_cache_fail 'Trusted v3 Analytix cache image attachment is ambiguous.'
  return 1 2>/dev/null || exit 1
fi

if ! analytix_cache_index=$(analytix_cache_storage_index "$analytix_cache_mount") ||
  ! analytix_cache_bsd_name=$(analytix_cache_storage_field "$analytix_cache_index" bsd_name) ||
  ! analytix_cache_filesystem=$(analytix_cache_storage_field "$analytix_cache_index" file_system) ||
  ! analytix_cache_writable=$(analytix_cache_storage_field "$analytix_cache_index" writable) ||
  ! analytix_cache_owners=$(analytix_cache_storage_field "$analytix_cache_index" ignore_ownership) ||
  ! analytix_cache_actual_uuid=$(analytix_cache_storage_field "$analytix_cache_index" volume_uuid) ||
  ! analytix_cache_protocol=$(
    analytix_cache_storage_field "$analytix_cache_index" physical_drive.protocol
  ) || ! analytix_cache_mount_line=$(analytix_cache_mount_entry "$analytix_cache_mount"); then
  analytix_cache_fail 'Cannot resolve the Analytix development cache authority.'
  return 1 2>/dev/null || exit 1
fi
analytix_cache_device="/dev/$analytix_cache_bsd_name"
if [[ ! "$analytix_cache_bsd_name" =~ '^disk[0-9]+s[0-9]+$' ]] ||
  [ "$analytix_cache_protocol" != 'Disk Image' ]; then
  analytix_cache_fail 'Analytix development cache device identity is unsafe.'
  return 1 2>/dev/null || exit 1
fi
if [ "${analytix_cache_filesystem:u}" != 'APFS' ]; then
  analytix_cache_fail "Analytix development cache must use APFS: $analytix_cache_filesystem."
  return 1 2>/dev/null || exit 1
fi
if [ "$analytix_cache_writable" != 'yes' ]; then
  analytix_cache_fail "Analytix development cache is not writable: $analytix_cache_mount."
  return 1 2>/dev/null || exit 1
fi
if [ "$analytix_cache_owners" != 'no' ]; then
  analytix_cache_fail "Analytix development cache ownership is disabled: $analytix_cache_mount."
  return 1 2>/dev/null || exit 1
fi
if [ "$analytix_cache_actual_uuid" != "$analytix_cache_volume_uuid" ]; then
  analytix_cache_fail 'Analytix development cache volume identity is not authoritative.'
  return 1 2>/dev/null || exit 1
fi
if [ -z "$analytix_cache_mount_line" ] ||
  [[ "$analytix_cache_mount_line" == *$'\n'* ]] ||
  [[ "$analytix_cache_mount_line" != "$analytix_cache_device on $analytix_cache_mount (apfs,"* ]] ||
  [[ "$analytix_cache_mount_line" == *noowners* ]] ||
  [[ "$analytix_cache_mount_line" == *read-only* ]] ||
  [[ "$analytix_cache_mount_line" == *", ro,"* ]] ||
  [[ "$analytix_cache_mount_line" == *", ro)"* ]]; then
  analytix_cache_fail 'Analytix development cache mount flags are unsafe.'
  return 1 2>/dev/null || exit 1
fi
if [ ! -d "$analytix_cache_mount" ] ||
  [ -L "$analytix_cache_mount" ] ||
  [ "${analytix_cache_mount:A}" != "$analytix_cache_mount" ] ||
  [ ! -w "$analytix_cache_mount" ]; then
  analytix_cache_fail 'Analytix development cache mount path is not canonical and writable.'
  return 1 2>/dev/null || exit 1
fi
analytix_cache_expected_device="$(analytix_cache_image_volume "$analytix_cache_image")"
if [ "$analytix_cache_expected_device" != "$analytix_cache_device" ]; then
  analytix_cache_fail 'Analytix development cache is not mounted from the trusted v3 sparsebundle.'
  return 1 2>/dev/null || exit 1
fi
analytix_cache_device_uuid=$(
  /System/Library/Filesystems/apfs.fs/Contents/Resources/apfs.util \
    -k "$analytix_cache_device" 2>/dev/null || true
)
if [ "$analytix_cache_device_uuid" != "$analytix_cache_volume_uuid" ]; then
  analytix_cache_fail 'Analytix development cache APFS identity changed during validation.'
  return 1 2>/dev/null || exit 1
fi
if ! analytix_cache_mount_device_id=$(
  /usr/bin/stat -f '%d' "$analytix_cache_mount"
) || ! analytix_cache_effective_uid=$(/usr/bin/id -u); then
  analytix_cache_fail 'Cannot resolve the Analytix development cache filesystem identity.'
  return 1 2>/dev/null || exit 1
fi

analytix_cache_validate_directory() {
  local target="$1"
  local required="$2"
  if [ -L "$target" ] ||
    { [ -e "$target" ] && [ ! -d "$target" ]; }; then
    analytix_cache_fail "Analytix development cache path is not a real directory: $target"
    return 1
  fi
  if [ ! -e "$target" ]; then
    [ "$required" != 'true' ]
    return
  fi
  if ! analytix_cache_directory_device_id=$(
    /usr/bin/stat -f '%d' "$target"
  ) || [ "$analytix_cache_directory_device_id" != "$analytix_cache_mount_device_id" ]; then
    analytix_cache_fail "Analytix development cache path escaped the trusted volume: $target"
    return 1
  fi
  if ! analytix_cache_directory_uid=$(
    /usr/bin/stat -f '%u' "$target"
  ) || [ "$analytix_cache_directory_uid" != "$analytix_cache_effective_uid" ]; then
    analytix_cache_fail "Analytix development cache path ownership is unsafe: $target"
    return 1
  fi
  if [ "${target:A}" != "$target" ] ||
    [ ! -w "$target" ] ||
    [ "$(/usr/bin/stat -f '%Lp' "$target")" != '700' ]; then
    analytix_cache_fail "Analytix development cache path is not canonical, private, and writable: $target"
    return 1
  fi
}

analytix_cache_validate_private_tree() {
  local target="$1"
  local regular_mode="$2"
  local entry entry_mode entry_device entry_uid entry_nlink
  local -a entries
  [ -d "$target" ] || return 0
  entries=("$target"/**/*(DN))
  for entry in "${entries[@]}"; do
    if [ -L "$entry" ] || { [ ! -d "$entry" ] && [ ! -f "$entry" ]; } ||
      [ "${entry:A}" != "$entry" ]; then
      analytix_cache_fail "Analytix private cache output has an unsafe identity: $entry"
      return 1
    fi
    if ! entry_device=$(/usr/bin/stat -f '%d' "$entry") ||
      [ "$entry_device" != "$analytix_cache_mount_device_id" ] ||
      ! entry_uid=$(/usr/bin/stat -f '%u' "$entry") ||
      [ "$entry_uid" != "$analytix_cache_effective_uid" ]; then
      analytix_cache_fail "Analytix private cache output escaped its owner or device: $entry"
      return 1
    fi
    entry_mode=$(/usr/bin/stat -f '%Lp' "$entry") || return 1
    if [ -d "$entry" ]; then
      if [ "$entry_mode" != '700' ]; then
        analytix_cache_fail "Analytix private cache output directory must use mode 0700: $entry"
        return 1
      fi
    elif [ "$regular_mode" = 'evidence' ]; then
      if [ "$entry_mode" != '600' ]; then
        analytix_cache_fail "Analytix cache evidence file must use mode 0600: $entry"
        return 1
      fi
    elif [ "$regular_mode" = 'native' ]; then
      if ! entry_nlink=$(/usr/bin/stat -f '%l' "$entry") ||
        [ "$entry_nlink" != '1' ]; then
        analytix_cache_fail "Analytix native cache file must have one link: $entry"
        return 1
      fi
      if [[ "$entry" == */analytix-native-development-build.json ]] ||
        [[ "$entry" == *.build.lock/owner.json ]] ||
        [[ "$entry" == *.build.lock.retired/owner.json ]] ||
        [[ "$entry" == *.build.lock.released/owner.json ]]; then
        if [ "$entry_mode" != '600' ]; then
          analytix_cache_fail "Analytix native cache metadata must use mode 0600: $entry"
          return 1
        fi
      elif [ "$entry_mode" != '700' ]; then
        analytix_cache_fail "Analytix native cache binary must use mode 0700: $entry"
        return 1
      fi
    elif [ "$entry_mode" != '600' ] && [ "$entry_mode" != '700' ]; then
      analytix_cache_fail "Analytix cache output file must use mode 0600 or executable mode 0700: $entry"
      return 1
    fi
  done
}

analytix_cache_directories=(
  "$analytix_cache_root"
  "$analytix_cache_root/npm"
  "$analytix_cache_root/go-build"
  "$analytix_cache_root/go-mod"
  "$analytix_cache_root/pip"
  "$analytix_cache_root/uv"
  "$analytix_cache_root/python-bytecode"
  "$analytix_cache_root/mypy"
  "$analytix_cache_root/ruff"
  "$analytix_cache_root/cargo-home"
  "$analytix_cache_root/cargo-target"
  "$analytix_cache_root/ccache"
  "$analytix_cache_root/sccache"
  "$analytix_cache_root/xwin"
  "$analytix_cache_root/corepack"
  "$analytix_cache_root/electron"
  "$analytix_cache_root/electron-builder"
  "$analytix_cache_root/playwright"
  "$analytix_cache_root/node-compile"
  "$analytix_cache_root/tmp"
  "$analytix_cache_root/xdg"
  "$analytix_cache_root/bin"
  "$analytix_cache_root/evidence"
  "$analytix_cache_root/native-components-development"
)

# Validate every existing namespace before the first mkdir or chmod. An unsafe
# later entry must not leave earlier entries partially remediated.
for analytix_cache_directory in "${analytix_cache_directories[@]}"; do
  if ! analytix_cache_validate_directory "$analytix_cache_directory" 'false'; then
    return 1 2>/dev/null || exit 1
  fi
done
if ! analytix_cache_validate_private_tree "$analytix_cache_root/bin" 'output' ||
  ! analytix_cache_validate_private_tree "$analytix_cache_root/evidence" 'evidence' ||
  ! analytix_cache_validate_private_tree \
    "$analytix_cache_root/native-components-development" 'native'; then
  return 1 2>/dev/null || exit 1
fi

# This policy intentionally persists in the calling shell. Sourcing the helper
# must never widen file creation to 0022; non-executable outputs default to
# owner-only mode, while writers of evidence still must request 0600 exactly.
umask 077

for analytix_cache_directory in "${analytix_cache_directories[@]}"; do
  if [ ! -e "$analytix_cache_directory" ] &&
    ! /bin/mkdir -m 700 "$analytix_cache_directory"; then
    printf '%s\n' "Cannot create Analytix development cache directory: $analytix_cache_directory" >&2
    return 1 2>/dev/null || exit 1
  fi
done
for analytix_cache_directory in "${analytix_cache_directories[@]}"; do
  if ! analytix_cache_validate_directory "$analytix_cache_directory" 'true'; then
    return 1 2>/dev/null || exit 1
  fi
done

export ANALYTIX_DEV_CACHE_ROOT="$analytix_cache_root"
# npm/Electron cache hints belong to the orchestration shell. Keep one
# canonical npm spelling so case-insensitive hermetic child checks stay
# deterministic.
export npm_config_cache="$analytix_cache_root/npm"
export XDG_CACHE_HOME="$analytix_cache_root/xdg"
export PIP_CACHE_DIR="$analytix_cache_root/pip"
export UV_CACHE_DIR="$analytix_cache_root/uv"
export PYTHONPYCACHEPREFIX="$analytix_cache_root/python-bytecode"
export MYPY_CACHE_DIR="$analytix_cache_root/mypy"
export RUFF_CACHE_DIR="$analytix_cache_root/ruff"
# These compiler cache hints support direct local go/cargo commands. Native
# build authorities accept only this exact layout and project the hints away
# before constructing their own isolated child environments.
export GOCACHE="$analytix_cache_root/go-build"
export GOMODCACHE="$analytix_cache_root/go-mod"
export GOTMPDIR="$analytix_cache_root/tmp"
export CARGO_HOME="$analytix_cache_root/cargo-home"
export CARGO_TARGET_DIR="$analytix_cache_root/cargo-target"
export CCACHE_DIR="$analytix_cache_root/ccache"
export SCCACHE_DIR="$analytix_cache_root/sccache"
export XWIN_CACHE_DIR="$analytix_cache_root/xwin"
export COREPACK_HOME="$analytix_cache_root/corepack"
export ELECTRON_CACHE="$analytix_cache_root/electron"
export ELECTRON_BUILDER_CACHE="$analytix_cache_root/electron-builder"
export PLAYWRIGHT_BROWSERS_PATH="$analytix_cache_root/playwright"
export NODE_COMPILE_CACHE="$analytix_cache_root/node-compile"
export TMPDIR="$analytix_cache_root/tmp"

}

analytix_cache_cleanup() {
emulate -L zsh
unsetopt xtrace
unfunction analytix_cache_preflight
unset -f analytix_cache_fail analytix_cache_storage_count 2>/dev/null
unset -f analytix_cache_storage_index analytix_cache_storage_field 2>/dev/null
unset -f analytix_cache_mount_entry analytix_cache_image_volume 2>/dev/null
unset -f analytix_cache_image_attachment_count 2>/dev/null
unset -f analytix_cache_validate_directory analytix_cache_validate_private_tree 2>/dev/null
unset analytix_cache_mount analytix_cache_root analytix_cache_backing_mount
unset analytix_cache_image analytix_damaged_v2_cache_image analytix_legacy_cache_image
unset analytix_cache_backing_volume_uuid analytix_cache_volume_uuid
unset analytix_apfs_volume_type analytix_storage_profiler analytix_cache_storage_info
unset analytix_cache_helper_source analytix_cache_helper_path analytix_cache_storage_library
unset analytix_cache_backing_index analytix_cache_backing_bsd_name
unset analytix_cache_backing_mount_line
unset analytix_cache_actual_backing_uuid analytix_cache_backing_writable
unset analytix_cache_backing_device_id analytix_cache_bundle analytix_retired_cache_image
unset analytix_cache_bundle_device_id
unset analytix_cache_hdiutil_info analytix_damaged_cache_image
unset analytix_damaged_cache_attachments analytix_cache_active_attachments
unset analytix_cache_index analytix_cache_bsd_name analytix_cache_device
unset analytix_cache_filesystem analytix_cache_writable
unset analytix_cache_owners analytix_cache_actual_uuid analytix_cache_expected_device
unset analytix_cache_protocol analytix_cache_device_uuid
unset analytix_cache_mount_line analytix_cache_mount_device_id analytix_cache_effective_uid
unset analytix_cache_directories analytix_cache_directory
unset analytix_cache_directory_device_id analytix_cache_directory_uid
}

analytix_cache_preflight
analytix_cache_preflight_status=$?
analytix_cache_cleanup
unfunction analytix_cache_cleanup
if [ "$analytix_cache_preflight_status" != '0' ]; then
  unset analytix_cache_preflight_status
  return 1 2>/dev/null || exit 1
fi
unset analytix_cache_preflight_status
