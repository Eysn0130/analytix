const ESCAPE_MAP = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };

export function esc(value) {
  return String(value ?? '').replace(/[&<>"']/g, (c) => ESCAPE_MAP[c]);
}

export function renderDefinitions() {
  return `        <!-- Definitions -->
        <defs>
          <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
            <polygon points="0 0, 10 3.5, 0 7" class="m-default" />
          </marker>
          <marker id="arrowhead-emphasis" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
            <polygon points="0 0, 10 3.5, 0 7" class="m-emphasis" />
          </marker>
          <marker id="arrowhead-security" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
            <polygon points="0 0, 10 3.5, 0 7" class="m-security" />
          </marker>
          <marker id="arrowhead-dashed" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
            <polygon points="0 0, 10 3.5, 0 7" class="m-dashed" />
          </marker>
          <pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
            <path d="M 40 0 L 0 0 0 40" class="c-grid" stroke-width="0.5"/>
          </pattern>
        </defs>`;
}

export function renderCards(cards) {
  const list = Array.isArray(cards) ? cards : [];
  return `    <!-- Info Cards -->
    <div class="cards">
${list.map((card) => `      <div class="card">
        <div class="card-header">
          <div class="card-dot ${esc(card.dot)}"></div>
          <h3>${esc(card.title)}</h3>
        </div>
        <ul>
${card.items.map((item) => `          <li>&bull; ${esc(item)}</li>`).join('\n')}
        </ul>
      </div>`).join('\n\n')}
    </div>`;
}

const SVG_SLOT_RE = / {6}<!-- ATLASFLOW:SVG_SLOT_START -->[\s\S]*? {6}<!-- ATLASFLOW:SVG_SLOT_END -->/;
const CARDS_SLOT_RE = / {4}<!-- ATLASFLOW:CARDS_SLOT_START -->[\s\S]*? {4}<!-- ATLASFLOW:CARDS_SLOT_END -->/;

const TEMPLATE_PLACEHOLDERS = [
  '<title>[PROJECT NAME] Architecture Diagram</title>',
  '<h1>[PROJECT NAME] Architecture</h1>',
  '<p class="subtitle">[Subtitle description]</p>',
  '[Project Name] &bull; [Additional metadata]',
];

// `footer` is injected as raw HTML so callers can embed <kbd> hints;
// pass only trusted strings here, never user input.
export function applyTemplate(template, { title, subtitle, footer, svg, cards }) {
  if (!SVG_SLOT_RE.test(template)) {
    throw new Error('applyTemplate: template missing ATLASFLOW:SVG_SLOT sentinel');
  }
  if (!CARDS_SLOT_RE.test(template)) {
    throw new Error('applyTemplate: template missing ATLASFLOW:CARDS_SLOT sentinel');
  }
  for (const ph of TEMPLATE_PLACEHOLDERS) {
    if (!template.includes(ph)) {
      throw new Error(`applyTemplate: template missing placeholder ${JSON.stringify(ph)}`);
    }
  }
  // Function replacers: a literal `$&`, `$'`, `$\`` or `$$` in titles, labels,
  // or rendered SVG must not be interpreted as a replacement pattern.
  return template
    .replace(TEMPLATE_PLACEHOLDERS[0], () => `<title>${esc(title)} Diagram</title>`)
    .replace(TEMPLATE_PLACEHOLDERS[1], () => `<h1>${esc(title)}</h1>`)
    .replace(TEMPLATE_PLACEHOLDERS[2], () => `<p class="subtitle">${esc(subtitle ?? '')}</p>`)
    .replace(SVG_SLOT_RE, () => svg)
    .replace(CARDS_SLOT_RE, () => cards)
    .replace(TEMPLATE_PLACEHOLDERS[3], () => footer);
}

// CJK and other fullwidth glyphs render at roughly twice the advance width of
// ASCII in the monospace stacks the template uses. Includes the supplementary
// CJK extensions and emoji, which also render double-width.
const FULLWIDTH_RE = /[\u{1100}-\u{115F}\u{2E80}-\u{A4CF}\u{AC00}-\u{D7A3}\u{F900}-\u{FAFF}\u{FE30}-\u{FE4F}\u{FF00}-\u{FF60}\u{FFE0}-\u{FFE6}\u{3000}-\u{303F}\u{1F000}-\u{1FAFF}\u{20000}-\u{3FFFD}]/u;

export function textUnits(text) {
  let units = 0;
  for (const ch of String(text ?? '')) units += FULLWIDTH_RE.test(ch) ? 2 : 1;
  return units;
}
