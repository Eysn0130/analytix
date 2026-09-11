if [[ "$ZSH_EVAL_CONTEXT" != *:file ]]; then
  print -u2 'Source this file from the Analytix cache preflight.'
  exit 64
fi

analytix_cache_storage_count() {
  local count
  count=$(print -rn -- "$analytix_cache_storage_info" |
    /usr/bin/plutil -extract SPStorageDataType raw -o - - 2>/dev/null) || return 1
  if [[ ! "$count" =~ '^[0-9]+$' ]] || [ "$count" -lt 1 ] || [ "$count" -gt 128 ]; then
    return 1
  fi
  print -rn -- "$count"
}

analytix_cache_storage_index() {
  local target="$1"
  local count index=0
  local mount_point match_index=''
  local match_count=0
  count=$(analytix_cache_storage_count) || return 1
  while [ "$index" -lt "$count" ]; do
    mount_point=$(print -rn -- "$analytix_cache_storage_info" |
      /usr/bin/plutil -extract "SPStorageDataType.$index.mount_point" raw -o - - 2>/dev/null) ||
      mount_point=''
    if [ "$mount_point" = "$target" ]; then
      match_index="$index"
      match_count=$((match_count + 1))
    fi
    index=$((index + 1))
  done
  if [ "$match_count" != '1' ]; then
    return 1
  fi
  print -rn -- "$match_index"
}

analytix_cache_storage_field() {
  local index="$1"
  local field="$2"
  print -rn -- "$analytix_cache_storage_info" |
    /usr/bin/plutil -extract "SPStorageDataType.$index.$field" raw -o - -
}
