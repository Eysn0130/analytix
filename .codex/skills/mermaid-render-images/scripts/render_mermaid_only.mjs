#!/usr/bin/env node
import fs from 'node:fs'
import path from 'node:path'

function parseArgs(argv) {
  const options = {
    input: '',
    output: '',
    basename: 'Mermaid图',
    curve: 'basis',
    transparentLabels: true,
    mermaidSrc: './mermaid.min.js'
  }
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index]
    switch (arg) {
      case '--input':
        options.input = argv[++index] || ''
        break
      case '--output':
        options.output = argv[++index] || ''
        break
      case '--basename':
        options.basename = argv[++index] || options.basename
        break
      case '--curve':
        options.curve = argv[++index] || options.curve
        break
      case '--transparent-labels':
        options.transparentLabels = String(argv[++index] || 'true').toLowerCase() !== 'false'
        break
      case '--mermaid-src':
        options.mermaidSrc = argv[++index] || options.mermaidSrc
        break
      case '-h':
      case '--help':
        usage()
        process.exit(0)
        break
      default:
        throw new Error(`Unknown option: ${arg}`)
    }
  }
  if (!options.input || !options.output) {
    usage()
    process.exit(2)
  }
  return options
}

function usage() {
  console.log(`Usage: render_mermaid_only.mjs --input INPUT.md --output OUT.html [--basename NAME] [--curve basis] [--transparent-labels true|false] [--mermaid-src SRC]`)
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

function extractMermaidBlocks(markdown, curve) {
  const blocks = []
  const regex = /```mermaid\s*\n([\s\S]*?)\n```/g
  let match
  while ((match = regex.exec(markdown)) !== null) {
    blocks.push(normalizeMermaidCurve(match[1].trim(), curve))
  }
  return blocks
}

function normalizeMermaidCurve(diagram, curve) {
  if (!curve) return diagram
  if (diagram.includes('"curve"')) {
    return diagram.replace(/"curve"\s*:\s*"[^"]+"/g, `"curve": "${curve}"`)
  }
  return `%%{init: {"flowchart": {"curve": "${curve}"}} }%%\n${diagram}`
}

const options = parseArgs(process.argv.slice(2))
const inputPath = path.resolve(options.input)
const outputPath = path.resolve(options.output)
const markdown = fs.readFileSync(inputPath, 'utf8')
const diagrams = extractMermaidBlocks(markdown, options.curve)

if (diagrams.length === 0) {
  throw new Error(`No Mermaid code blocks found in ${inputPath}`)
}

const labelBackground = options.transparentLabels ? 'transparent' : '#ffffff'
const cards = diagrams.map((diagram, index) => `
  <section class="mermaid-card" data-diagram-index="${index + 1}">
    <div class="mermaid-actions" aria-hidden="true">
      <svg viewBox="0 0 16 16"><path d="M1.8 5.8V2.6c0-.4.3-.8.8-.8h3.2M10.2 1.8h3.2c.4 0 .8.3.8.8v3.2M14.2 10.2v3.2c0 .4-.3.8-.8.8h-3.2M5.8 14.2H2.6c-.4 0-.8-.3-.8-.8v-3.2" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
      <svg viewBox="0 0 16 16"><path d="M5.8 4.8V3c0-.7.5-1.2 1.2-1.2h5.2c.7 0 1.2.5 1.2 1.2v5.2c0 .7-.5 1.2-1.2 1.2h-1.8M3.8 6.6H9c.7 0 1.2.5 1.2 1.2V13c0 .7-.5 1.2-1.2 1.2H3.8c-.7 0-1.2-.5-1.2-1.2V7.8c0-.7.5-1.2 1.2-1.2Z" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>
    </div>
    <div class="mermaid-source">${escapeHtml(diagram)}</div>
  </section>
`).join('\n')

const html = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(options.basename)}</title>
  <style>
    html,
    body {
      margin: 0;
      background: #fff;
      color: #111827;
      font-family: Arial, "PingFang SC", "Noto Sans SC", "Microsoft YaHei", sans-serif;
    }

    body { padding: 28px; }

    .page {
      display: flex;
      flex-direction: column;
      gap: 28px;
      width: max-content;
      min-width: 1320px;
    }

    .mermaid-card {
      position: relative;
      box-sizing: border-box;
      width: 1320px;
      overflow: visible;
      border: 1px solid rgba(17, 24, 39, 0.10);
      border-radius: 18px;
      background: #fff;
      padding: 48px 34px 28px;
      box-shadow: 0 1px 2px rgba(17, 24, 39, 0.04);
    }

    .mermaid-card.is-wide { width: 1720px; }

    .mermaid-actions {
      position: absolute;
      top: 15px;
      right: 24px;
      display: flex;
      align-items: center;
      gap: 14px;
      color: #6b7280;
    }

    .mermaid-actions svg {
      width: 18px;
      height: 18px;
    }

    .mermaid-source {
      display: flex;
      justify-content: center;
      min-height: 120px;
    }

    .mermaid-source > svg {
      display: block;
      max-width: none !important;
      height: auto !important;
    }

    .edgeLabel p,
    .edgeLabel span {
      background: ${labelBackground} !important;
      color: #111827 !important;
    }

    .edgeLabel rect,
    .labelBkg {
      fill: ${labelBackground} !important;
      background: ${labelBackground} !important;
    }
  </style>
</head>
<body>
  <main class="page">
${cards}
  </main>
  <script src="${escapeHtml(options.mermaidSrc)}"></script>
  <script>
    window.__MERMAID_RENDER_READY = false;
    window.addEventListener('load', async () => {
      try {
        mermaid.initialize({
          startOnLoad: false,
          securityLevel: 'loose',
          theme: 'default',
          themeVariables: {
            fontFamily: 'Arial, "PingFang SC", "Noto Sans SC", "Microsoft YaHei", sans-serif',
            edgeLabelBackground: '${labelBackground}',
            lineColor: '#111827',
            primaryTextColor: '#111827'
          },
          flowchart: {
            htmlLabels: true,
            curve: '${options.curve}'
          }
        });

        const blocks = Array.from(document.querySelectorAll('.mermaid-source'));
        for (let index = 0; index < blocks.length; index += 1) {
          const block = blocks[index];
          const source = block.textContent || '';
          const result = await mermaid.render(\`mermaid-render-images-\${index + 1}\`, source);
          block.innerHTML = result.svg;
          const svg = block.querySelector('svg');
          const viewBox = svg?.getAttribute('viewBox') || '';
          const width = Number(viewBox.split(/\\s+/)[2] || 0);
          if (width > 1250) block.closest('.mermaid-card')?.classList.add('is-wide');
        }
      } catch (error) {
        console.error(error);
      } finally {
        await document.fonts.ready;
        window.__MERMAID_RENDER_READY = true;
      }
    });
  </script>
</body>
</html>
`

fs.mkdirSync(path.dirname(outputPath), { recursive: true })
fs.writeFileSync(outputPath, html, 'utf8')
console.log(outputPath)
