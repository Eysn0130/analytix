#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
config_helper="${script_dir}/install-config-helper.mjs"
server_name="analytix-computer-use"
command_name="analytix-computer-use"
scope="project"

usage() {
  cat <<'EOF'
用法: ./scripts/install-gemini-mcp.sh [--scope project|user]

将 analytix-computer-use stdio MCP entry 安装到 Gemini CLI 配置。
默认使用 project scope，会写入当前项目的 ./.gemini/settings.json。
设置 GEMINI_CONFIG_PATH 可直接覆盖目标文件。
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --scope)
      if [[ $# -lt 2 ]]; then
        echo "--scope 需要一个值" >&2
        usage >&2
        exit 1
      fi
      scope="$2"
      shift 2
      ;;
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

case "${scope}" in
  project)
    default_config_path="$(pwd -P)/.gemini/settings.json"
    ;;
  user)
    default_config_path="${HOME}/.gemini/settings.json"
    ;;
  *)
    echo "不支持的 Gemini scope: ${scope}" >&2
    usage >&2
    exit 1
    ;;
esac

config_path="${GEMINI_CONFIG_PATH:-${default_config_path}}"

node "${config_helper}" gemini-mcp "${config_path}" "${server_name}" "${command_name}"
