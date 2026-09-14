// MIT: Copyright (c) 2026 tt-a1i; (c) 2025 Cocoon AI. See ../../LICENSE.
import { esc } from './utils.mjs';

// Only the fixed renderer's SVG belongs here. No template, resource URL,
// JavaScript, stylesheet or filesystem capability comes from Scene input.
export function renderLocalDocument({ svg, title }) {
  if (typeof svg !== 'string' || typeof title !== 'string' || title.length > 1024 || new TextEncoder().encode(svg).length > (32 << 20) ||
      !/^\s*<svg\b/.test(svg) || !/<\/svg>\s*$/.test(svg)) throw new Error('Canvas local output is invalid.');
  const allowedTags = new Set(['svg', 'metadata', 'style', 'defs', 'marker', 'polygon', 'pattern', 'path', 'rect', 'ellipse', 'text', 'title', 'g']);
  const tags = [...svg.matchAll(/<[^>]*>/g)].map(match => match[0]);
  for (const tag of tags) {
    if (/^<!--[\s\S]*-->$/.test(tag)) continue;
    const name = /^<\/?([a-z][a-z0-9]*)\b/i.exec(tag)?.[1];
    if (!name || !allowedTags.has(name)) throw new Error('Canvas local output contains an unsupported resource.');
    // Inspect actual attributes, not escaped text inside aria-label/title.
    const attributes = /([A-Za-z_:][A-Za-z0-9_.:-]*)\s*=\s*("[^"]*"|'[^']*')/g;
    const allowedAttributes = new Set(['xmlns', 'viewBox', 'role', 'aria-label', 'id', 'markerWidth', 'markerHeight', 'refX', 'refY', 'orient', 'points', 'class', 'patternUnits', 'd', 'x', 'y', 'cx', 'cy', 'width', 'height', 'rx', 'ry', 'fill', 'stroke', 'stroke-width', 'stroke-dasharray', 'marker-end', 'font-size', 'font-weight', 'text-anchor', 'data-canvas-node-id', 'data-canvas-edge-id']);
    for (const attribute of tag.matchAll(attributes)) {
      if (!allowedAttributes.has(attribute[1]) ||
          (['fill', 'stroke', 'marker-end'].includes(attribute[1]) && /url\(\s*(?!#[A-Za-z0-9._:-]+\))/i.test(attribute[2]))) throw new Error('Canvas local output contains an unsupported attribute.');
    }
    if (tag.replace(/^<\/?[A-Za-z][A-Za-z0-9]*/, '').replace(attributes, '').replace(/\/?\s*>$/, '').trim()) throw new Error('Canvas local output has malformed attributes.');
  }
  for (const match of svg.matchAll(/<style>([\s\S]*?)<\/style>/g)) {
    if (/@|\\|url\s*\(/i.test(match[1])) throw new Error('Canvas local stylesheet contains an unsupported resource.');
  }
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; img-src 'none'; font-src 'none'; connect-src 'none'; base-uri 'none'; form-action 'none'">
<title>${esc(title)}</title><style>body{margin:0;background:#fff;color:#111827;font-family:ui-monospace,monospace}main{padding:24px}svg{display:block;width:100%;height:auto}h1{font-size:20px}footer{font-size:12px;color:#475569}</style></head>
<body><main><h1>${esc(title)}</h1>${svg}<footer>Built with AtlasFlow</footer></main></body></html>`;
}
