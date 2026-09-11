#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
config_helper="${script_dir}/install-config-helper.mjs"
server_name="analytix-computer-use"
command_name="analytix-computer-use"
opencode_config_dir="${XDG_CONFIG_HOME:-${HOME}/.config}/opencode"

usage() {
  cat <<'EOF'
用法: ./scripts/install-opencode-mcp.sh

将 analytix-computer-use stdio MCP entry 安装到 opencode 配置。
设置 OPENCODE_CONFIG_PATH 可直接覆盖 primary config file。
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -n "${OPENCODE_CONFIG_PATH:-}" ]]; then
  primary_config_path="${OPENCODE_CONFIG_PATH}"
  secondary_config_path=""
elif [[ -f "${opencode_config_dir}/opencode.json" ]]; then
  primary_config_path="${opencode_config_dir}/opencode.json"
  secondary_config_path="${opencode_config_dir}/config.json"
elif [[ -f "${opencode_config_dir}/config.json" ]]; then
  primary_config_path="${opencode_config_dir}/config.json"
  secondary_config_path="${opencode_config_dir}/opencode.json"
else
  primary_config_path="${opencode_config_dir}/opencode.json"
  secondary_config_path="${opencode_config_dir}/config.json"
fi

node "${config_helper}" opencode-mcp "${primary_config_path}" "${secondary_config_path}" "${server_name}" "${command_name}"
