---
name: analytix-mermaid-export
description: Export Mermaid diagrams using Analytix's repository-pinned renderer when that project implementation is requested. General Mermaid image export uses the personal mermaid-render-images skill.
---

# Mermaid生图

Use this skill to turn Mermaid code blocks inside a Markdown file into report-ready PNG images.

## Quick Start

Run the bundled exporter:

```bash
.codex/skills/mermaid-render-images/scripts/export_mermaid_images.sh \
  "/absolute/path/to/report.md" \
  --out-dir "/Users/sun/Downloads" \
  --basename "资金研判报告_Mermaid图"
```

The exporter writes:

- `basename_01.png`, `basename_02.png`, ... for each Mermaid card.
- `basename_总览.png` for all cards together.
- `basename.html` as a standalone Mermaid-only HTML page.

## Options

- `--curve basis`: use smooth Bezier-style curved lines. This overrides `curve: "linear"` inside Mermaid init blocks during export only.
- `--curve linear`: preserve straight lines.
- `--transparent-labels true`: keep edge-label backgrounds transparent. This is the default.
- `--transparent-labels false`: use white label backgrounds when labels must mask crossing lines.
- `--out-dir DIR`: choose the output folder.
- `--basename NAME`: choose output filenames.

## Workflow

1. Verify the input `.md` contains fenced Mermaid blocks: ` ```mermaid `.
2. Run `scripts/export_mermaid_images.sh` with an absolute input path.
3. Prefer `--curve basis` for polished report images unless the user requests straight lines.
4. Preserve `--transparent-labels true` when the user asks for transparent line labels.
5. After export, report the output file paths and any validation notes.

## Requirements

- Requires `node`, `python3`, and `npx` or the bundled Playwright CLI wrapper at `~/.codex/skills/playwright/scripts/playwright_cli.sh`.
- Uses local `node_modules/mermaid/dist/mermaid.min.js` when available near the input file or current workspace. If unavailable, the generated HTML falls back to the public Mermaid CDN.

## Notes

- The exporter does not modify the source Markdown file.
- The exporter normalizes Mermaid `curve` settings only in the generated HTML, so report source content remains unchanged.
- The card style is designed to match Codex-style Mermaid rendering: white rounded card, top-right icons, rectangular nodes, and optional transparent edge labels.
