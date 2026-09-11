#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  export_mermaid_images.sh INPUT.md [--out-dir DIR] [--basename NAME] [--curve basis|linear|natural|cardinal|step] [--transparent-labels true|false]

Exports each Mermaid code block in INPUT.md as a styled PNG and writes a Mermaid-only HTML file.

Defaults:
  --out-dir             directory containing INPUT.md
  --basename            Mermaid图
  --curve               basis
  --transparent-labels  true
EOF
}

if [[ $# -lt 1 ]]; then
  usage
  exit 2
fi

input=""
out_dir=""
basename="Mermaid图"
curve="basis"
transparent_labels="true"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out-dir)
      out_dir="${2:?missing value for --out-dir}"
      shift 2
      ;;
    --basename)
      basename="${2:?missing value for --basename}"
      shift 2
      ;;
    --curve)
      curve="${2:?missing value for --curve}"
      shift 2
      ;;
    --transparent-labels)
      transparent_labels="${2:?missing value for --transparent-labels}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$input" ]]; then
        echo "Only one input Markdown file is supported." >&2
        exit 2
      fi
      input="$1"
      shift
      ;;
  esac
done

if [[ -z "$input" ]]; then
  echo "Missing input Markdown file." >&2
  usage >&2
  exit 2
fi

input="$(python3 -c 'import os,sys; print(os.path.abspath(os.path.expanduser(sys.argv[1])))' "$input")"
if [[ ! -f "$input" ]]; then
  echo "Input file not found: $input" >&2
  exit 1
fi

if [[ -z "$out_dir" ]]; then
  out_dir="$(dirname "$input")"
else
  out_dir="$(python3 -c 'import os,sys; print(os.path.abspath(os.path.expanduser(sys.argv[1])))' "$out_dir")"
fi
mkdir -p "$out_dir"

skill_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/mermaid-render-images.XXXXXX")"
cleanup() {
  if [[ -n "${server_pid:-}" ]]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$work_dir"
}
trap cleanup EXIT

html_path="$work_dir/mermaid_only.html"

find_mermaid_script() {
  local start="$1"
  local dir
  dir="$(cd "$(dirname "$start")" && pwd)"
  while [[ "$dir" != "/" ]]; do
    local candidate="$dir/node_modules/mermaid/dist/mermaid.min.js"
    if [[ -f "$candidate" ]]; then
      echo "$candidate"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  dir="$(pwd)"
  while [[ "$dir" != "/" ]]; do
    local candidate="$dir/node_modules/mermaid/dist/mermaid.min.js"
    if [[ -f "$candidate" ]]; then
      echo "$candidate"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  return 1
}

mermaid_src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"
if mermaid_file="$(find_mermaid_script "$input")"; then
  cp "$mermaid_file" "$work_dir/mermaid.min.js"
  mermaid_src="./mermaid.min.js"
fi

node "$skill_dir/scripts/render_mermaid_only.mjs" \
  --input "$input" \
  --output "$html_path" \
  --basename "$basename" \
  --curve "$curve" \
  --transparent-labels "$transparent_labels" \
  --mermaid-src "$mermaid_src" >/dev/null

cp "$html_path" "$out_dir/${basename}.html"
find "$out_dir" -maxdepth 1 -name "${basename}_*.png" -delete

port="$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(('127.0.0.1', 0))
    print(sock.getsockname()[1])
PY
)"

python3 -m http.server "$port" --bind 127.0.0.1 --directory "$work_dir" >"$work_dir/server.log" 2>&1 &
server_pid=$!

for _ in {1..80}; do
  if python3 - <<'PY' "$port" >/dev/null 2>&1
import socket, sys
with socket.create_connection(('127.0.0.1', int(sys.argv[1])), timeout=0.2):
    pass
PY
  then
    break
  fi
  sleep 0.1
done

if ! python3 - <<'PY' "$port" >/dev/null 2>&1
import socket, sys
with socket.create_connection(('127.0.0.1', int(sys.argv[1])), timeout=0.2):
    pass
PY
then
  echo "Failed to start local preview server." >&2
  cat "$work_dir/server.log" >&2 || true
  exit 1
fi

pwcli="${CODEX_HOME:-$HOME/.codex}/skills/playwright/scripts/playwright_cli.sh"
if [[ -x "$pwcli" ]]; then
  pw_cmd=("$pwcli")
else
  if ! command -v npx >/dev/null 2>&1; then
    echo "npx is required when the bundled Playwright CLI wrapper is unavailable." >&2
    exit 1
  fi
  pw_cmd=(npx --yes --package @playwright/cli playwright-cli)
fi

export PLAYWRIGHT_CLI_SESSION="mermaid-render-images-$$"
url="http://127.0.0.1:$port/mermaid_only.html"
capture_script="$work_dir/capture_mermaid_images.js"
python3 - <<'PY' "$skill_dir/scripts/capture_mermaid_images.js" "$capture_script" "$out_dir" "$basename"
import json
import sys
template_path, output_path, out_dir, basename = sys.argv[1:5]
text = open(template_path, encoding='utf-8').read()
text = text.replace('__MERMAID_RENDER_OUT_DIR__', json.dumps(out_dir, ensure_ascii=False))
text = text.replace('__MERMAID_RENDER_BASENAME__', json.dumps(basename, ensure_ascii=False))
open(output_path, 'w', encoding='utf-8').write(text)
PY

"${pw_cmd[@]}" open "$url" >/dev/null
"${pw_cmd[@]}" run-code --filename "$capture_script" >/dev/null
"${pw_cmd[@]}" close >/dev/null 2>&1 || true

if ! find "$out_dir" -maxdepth 1 -name "${basename}_*.png" | grep -q .; then
  echo "No PNG files were generated." >&2
  exit 1
fi

echo "$out_dir/${basename}.html"
find "$out_dir" -maxdepth 1 -name "${basename}_*.png" | sort
