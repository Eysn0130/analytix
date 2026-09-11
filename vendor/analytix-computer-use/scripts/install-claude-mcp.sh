#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
config_helper="${script_dir}/install-config-helper.mjs"
claude_config_path="${CLAUDE_CONFIG_PATH:-${HOME}/.claude.json}"
project_root="$(pwd -P)"
server_name="analytix-computer-use"
command_name="analytix-computer-use"

usage() {
  cat <<'EOF'
用法: ./scripts/install-claude-mcp.sh

将 analytix-computer-use stdio MCP entry 安装到当前项目的 ~/.claude.json。
脚本可重复执行：如果相同 MCP server entry 已存在，会保持文件不变。
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

node "${config_helper}" claude-mcp "${claude_config_path}" "${project_root}" "${server_name}" "${command_name}"
