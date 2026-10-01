// MIT: Copyright (c) 2026 tt-a1i; (c) 2025 Cocoon AI. See ../../LICENSE.
import { esc } from './utils.mjs';

// Accessible name for the generated diagram SVG.
export function svgRootAttrs(meta, kind) {
  const name = meta.subtitle ? `${meta.title} — ${meta.subtitle}` : meta.title;
  const animation = meta.animation === 'trace' ? ' data-animation="trace"' : '';
  return `role="img" aria-label="${esc(`${name} (${kind})`)}"${animation}`;
}

export function animateAttr(meta, kind, step) {
  if (meta.animation !== 'trace') return '';
  const safeStep = Number.isFinite(step) && step >= 0 ? Math.floor(step) : 0;
  return ` data-animate="${kind}" style="--step:${safeStep}"`;
}
