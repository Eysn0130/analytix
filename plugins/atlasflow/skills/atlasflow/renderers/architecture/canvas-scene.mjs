// MIT: Copyright (c) 2026 tt-a1i; (c) 2025 Cocoon AI. See ../../LICENSE.
// Core/codec must validate the shared canvasSceneSchema before invoking this
// adapter. These checks bound rendering and defend its data-only input; they
// neither authorize source references nor replace the canonical Scene contract.
import { renderArchitecturePlan } from './render-plan.mjs';
import { esc, textUnits } from '../shared/utils.mjs';
import { labelPoint } from '../shared/geometry.mjs';
import { renderLocalDocument } from '../shared/local-document.mjs';

const MAX_INPUT = 4 << 20;
const MAX_OUTPUT = 32 << 20;
const encoder = new TextEncoder();
const idPattern = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/;
const color = /^#[0-9a-fA-F]{6}$/;
const fail = () => { throw new Error('Canvas render input is invalid or exceeds its resource budget.'); };
function text(value, maximum) {
  if (typeof value !== 'string' || value.includes('\0') || encoder.encode(value).length > maximum) fail();
  for (let i = 0; i < value.length; i++) {
    const unit = value.charCodeAt(i);
    if (unit >= 0xd800 && unit <= 0xdbff) {
      const low = value.charCodeAt(++i);
      if (!(low >= 0xdc00 && low <= 0xdfff)) fail();
    } else if (unit >= 0xdc00 && unit <= 0xdfff) fail();
  }
  return value;
}
function keys(value, required, optional = []) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) fail();
  const actual = Object.keys(value);
  if (required.some(key => !Object.hasOwn(value, key)) || actual.some(key => !required.includes(key) && !optional.includes(key))) fail();
}
function list(value, maximum) { if (!Array.isArray(value) || value.length > maximum) fail(); return value; }
function coordinate(value) { if (typeof value !== 'number' || !Number.isFinite(value) || Math.abs(value) > 1000000) fail(); return value; }
function identifier(value) { if (typeof value !== 'string' || !idPattern.test(value)) fail(); return value; }
function style(value, target) {
  keys(value, ['fill', 'stroke', 'strokeWidth', 'dash', 'shape']);
  if (typeof value.fill !== 'string' || typeof value.stroke !== 'string' || !color.test(value.fill) || !color.test(value.stroke) ||
      !Number.isInteger(value.strokeWidth) || value.strokeWidth < 1 || value.strokeWidth > 16 ||
      !['solid', 'dashed'].includes(value.dash) || !(target === 'edge' ? ['line'] : ['rectangle', 'rounded', 'ellipse']).includes(value.shape)) fail();
}
// Reject executable/getter/toJSON values before serialization can invoke them.
function plainData(value, depth = 0, budget = { tokens: 0, bytes: 0 }) {
  if (depth > 12 || ++budget.tokens > 524288) fail();
  if (typeof value === 'string') { text(value, MAX_INPUT); budget.bytes += encoder.encode(value).length; }
  else if (typeof value === 'number') { if (!Number.isFinite(value)) fail(); }
  else if (value !== null && typeof value !== 'boolean') {
    if (!value || typeof value !== 'object') fail();
    const array = Array.isArray(value), prototype = Object.getPrototypeOf(value);
    if (array ? prototype !== Array.prototype : prototype !== Object.prototype && prototype !== null) fail();
    if (array && value.length > 524288 - budget.tokens) fail();
    const ownKeys = Reflect.ownKeys(value);
    // JSON must neither execute inherited hooks nor drop extra array fields.
    // Reject holes before serialization can expand a sparse array without limit.
    if (array && ownKeys.length !== value.length + 1) fail();
    for (const key of ownKeys) {
      if (array && key === 'length') continue;
      if (typeof key !== 'string' || (array && (!/^(0|[1-9][0-9]*)$/.test(key) || Number(key) >= value.length))) fail();
      const descriptor = Object.getOwnPropertyDescriptor(value, key);
      if (!descriptor || !descriptor.enumerable || !Object.hasOwn(descriptor, 'value')) fail();
      budget.bytes += encoder.encode(key).length;
      plainData(descriptor.value, depth + 1, budget);
    }
  }
  if (budget.bytes > MAX_INPUT) fail();
}
function assertFact(fact, edge) {
  keys(fact, edge ? ['id', 'from', 'to', 'label', 'relation', 'attributes', 'sources', 'assumption'] : ['id', 'label', 'attributes', 'sources', 'assumption']);
  identifier(fact.id); text(fact.label, 1024);
  if (typeof fact.assumption !== 'boolean') fail();
  if (edge) { identifier(fact.from); identifier(fact.to); if (!text(fact.relation, 128)) fail(); }
  if (!fact.attributes || typeof fact.attributes !== 'object' || Array.isArray(fact.attributes) || Object.keys(fact.attributes).length > 32) fail();
  for (const [key, value] of Object.entries(fact.attributes)) {
    if (!/^[A-Za-z][A-Za-z0-9_-]{0,63}$/.test(key)) fail();
    if (typeof value === 'string') text(value, 2048);
    else if (typeof value === 'number') { if (!Number.isFinite(value) || Math.abs(value) > Number.MAX_SAFE_INTEGER) fail(); }
    else if (value !== null && typeof value !== 'boolean') fail();
  }
  const seen = new Set();
  for (const ref of list(fact.sources, 64)) {
    keys(ref, ['sourceId'], ['locator', 'note']); identifier(ref.sourceId);
    if (Object.hasOwn(ref, 'locator')) text(ref.locator, 1024);
    if (Object.hasOwn(ref, 'note')) text(ref.note, 2048);
    const key = `${ref.sourceId}\0${ref.locator ?? ''}`;
    if (seen.has(key)) fail(); seen.add(key);
  }
}

export function canvasSceneToPlan(input) {
  plainData(input);
  const serialized = JSON.stringify(input);
  if (encoder.encode(serialized).length > MAX_INPUT) fail();
  const scene = JSON.parse(serialized);
  keys(scene, ['schemaVersion', 'facts', 'presentation']); if (scene.schemaVersion !== 1) fail();
  keys(scene.facts, ['nodes', 'edges']); keys(scene.presentation, ['nodes', 'edges']);
  list(scene.facts.nodes, 256); list(scene.facts.edges, 1024); list(scene.presentation.nodes, 256); list(scene.presentation.edges, 1024);
  const nodes = new Map(), edges = new Map();
  for (const node of scene.facts.nodes) { assertFact(node, false); if (nodes.has(node.id)) fail(); nodes.set(node.id, node); }
  for (const edge of scene.facts.edges) { assertFact(edge, true); if (edges.has(edge.id) || nodes.has(edge.id) || !nodes.has(edge.from) || !nodes.has(edge.to)) fail(); edges.set(edge.id, edge); }
  if (scene.presentation.nodes.length !== nodes.size || scene.presentation.edges.length !== edges.size) fail();
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  const include = (x, y) => { minX = Math.min(minX, x); minY = Math.min(minY, y); maxX = Math.max(maxX, x); maxY = Math.max(maxY, y); };
  const seen = new Set();
  const components = scene.presentation.nodes.map(view => {
    keys(view, ['id', 'layout', 'displayLabel', 'style']);
    if (!nodes.has(view.id) || seen.has(view.id)) fail(); seen.add(view.id);
    keys(view.layout, ['x', 'y', 'width', 'height']);
    const { x, y, width, height } = view.layout;
    [x, y, width, height, x + width, y + height].forEach(coordinate); if (width <= 0 || height <= 0) fail();
    text(view.displayLabel, 1024); style(view.style, 'node');
    include(x - view.style.strokeWidth, y - view.style.strokeWidth); include(x + width + view.style.strokeWidth, y + height + view.style.strokeWidth);
    const labelWidth = textUnits(view.displayLabel) * 7;
    include(x + width / 2 - labelWidth / 2, y + height / 2 - 12); include(x + width / 2 + labelWidth / 2, y + height / 2 + 16);
    return { id: view.id, type: 'external', label: view.displayLabel, factLabel: nodes.get(view.id).label, pos: [x, y], size: [width, height], style: view.style };
  });
  const connections = scene.presentation.edges.map(view => {
    keys(view, ['id', 'points', 'displayLabel', 'style']);
    if (!edges.has(view.id) || seen.has(view.id)) fail(); seen.add(view.id);
    list(view.points, 32); if (view.points.length === 1) fail();
    const points = view.points.map(point => { keys(point, ['x', 'y']); coordinate(point.x); coordinate(point.y); include(point.x, point.y); return [point.x, point.y]; });
    text(view.displayLabel, 1024); style(view.style, 'edge');
    const fact = edges.get(view.id);
    return { id: view.id, from: fact.from, to: fact.to, label: view.displayLabel, factLabel: fact.label, points, style: view.style };
  });
  // Include labels for explicit and automatic routes after the common kernel
  // computes routing; this preliminary viewport also supports an empty scene.
  const viewport = components.length ? [minX - 24, minY - 24, maxX - minX + 48, maxY - minY + 48] : [0, 0, 320, 240];
  return { scene, plan: { schema_version: 1, diagram_type: 'architecture', meta: { title: 'Canvas', animation: 'none' }, components, connections, boundaries: [], cards: [], viewport } };
}

export function renderCanvasScene(input, { title = 'Canvas' } = {}) {
  text(title, 1024);
  const { scene, plan } = canvasSceneToPlan(input);
  plan.meta.title = title;
  let rendered = renderArchitecturePlan(plan, { canvas: true });
  // Display labels can be longer than nodes; preserve them instead of slicing.
  let [x, y, width, height] = plan.viewport;
  let right = x + width, bottom = y + height;
  for (let index = 0; index < rendered.layout.connections.length; index++) {
    const route = rendered.layout.connections[index].points;
    if (route.length < 2) continue;
    const padding = plan.connections[index].style.strokeWidth * 10;
    for (const point of route) { x = Math.min(x, point[0] - padding); y = Math.min(y, point[1] - padding); right = Math.max(right, point[0] + padding); bottom = Math.max(bottom, point[1] + padding); }
    const [lx, ly] = labelPoint(plan.connections[index], route);
    const half = Math.max(30, textUnits(plan.connections[index].label) * 4.8 + 10) / 2;
    x = Math.min(x, lx - half - 24); y = Math.min(y, ly - 34);
    right = Math.max(right, lx + half + 24); bottom = Math.max(bottom, ly + 28);
  }
  plan.viewport = [x, y, right - x, bottom - y];
  rendered = renderArchitecturePlan(plan, { canvas: true });
  const metadata = `<metadata id="canvas-scene">${esc(JSON.stringify(scene))}</metadata>`;
  const svg = rendered.svg.replace(/(<svg\b[^>]*>)/, (_match, start) => `${start}${metadata}`);
  if (encoder.encode(svg).length > MAX_OUTPUT) fail();
  return { svg, html: renderLocalDocument({ svg, title }), scene, layout: rendered.layout };
}
