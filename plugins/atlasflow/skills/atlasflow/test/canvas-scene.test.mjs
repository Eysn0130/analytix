// MIT: Copyright (c) 2026 tt-a1i; (c) 2025 Cocoon AI. See ../LICENSE.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';
import { renderCanvasScene, canvasSceneToPlan } from '../renderers/architecture/canvas-scene.mjs';
import { renderArchitecturePlan } from '../renderers/architecture/render-plan.mjs';
import { renderLocalDocument } from '../renderers/shared/local-document.mjs';

function fixture() {
  const style = { fill: '#eeeeee', stroke: '#123456', strokeWidth: 3, dash: 'dashed', shape: 'ellipse' };
  const edgeStyle = { ...style, shape: 'line' };
  return {
    schemaVersion: 1,
    facts: {
      nodes: [
        { id: '1:node.a', label: 'Original fact', attributes: { amount: '12.34', active: true, unset: null }, sources: Array.from({ length: 9 }, (_, i) => ({ sourceId: `src-${i}`, locator: `row:${i}`, note: 'retained' })), assumption: false },
        { id: 'node-b', label: 'Concept', attributes: {}, sources: [], assumption: true }
      ],
      edges: [
        { id: 'edge:1', from: '1:node.a', to: 'node-b', label: 'Fact relation one', relation: 'transfer', attributes: { amount: 25 }, sources: [{ sourceId: 'src-edge' }], assumption: false },
        { id: 'edge:2', from: '1:node.a', to: 'node-b', label: 'Fact relation two', relation: 'hypothesis', attributes: {}, sources: [], assumption: true }
      ]
    },
    presentation: {
      nodes: [
        { id: '1:node.a', layout: { x: -200.5, y: -70.25, width: 100, height: 60 }, displayLabel: 'Display A', style },
        { id: 'node-b', layout: { x: 30, y: 100, width: 110, height: 80 }, displayLabel: 'Display B', style: { ...style, shape: 'rectangle', dash: 'solid' } }
      ],
      edges: [
        { id: 'edge:1', points: [{ x: -150, y: -40 }, { x: -300.25, y: -10.5 }, { x: 40, y: 100 }], displayLabel: 'Relation A', style: edgeStyle },
        { id: 'edge:2', points: [], displayLabel: 'Relation B', style: { ...edgeStyle, stroke: '#654321' } }
      ]
    }
  };
}
const decodeText = value => value.replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');

test('Canvas uses the shared architecture kernel with complete IDs, facts and exact routes', () => {
  const scene = fixture(), original = structuredClone(scene);
  const result = renderCanvasScene(scene);
  assert.deepEqual(scene, original);
  assert.deepEqual(result.scene, original);
  const embedded = /<metadata id="canvas-scene">([\s\S]*?)<\/metadata>/.exec(result.svg)[1];
  assert.deepEqual(JSON.parse(decodeText(embedded)), scene);
  assert.equal(result.scene.facts.nodes[0].sources.length, 9);
  assert.match(result.svg, /data-canvas-node-id="1:node.a"/);
  for (const edge of scene.facts.edges) assert.match(result.svg, new RegExp(`data-canvas-edge-id="${edge.id}"`));
  assert.deepEqual(result.layout.connections[0].points, [[-150, -40], [-300.25, -10.5], [40, 100]]);
  assert.equal(result.layout.components[0].x, -200.5);
  assert.ok(result.layout.viewBox[0] < -300.25);
  assert.match(result.svg, /<ellipse[^>]*fill="#eeeeee" stroke="#123456" stroke-width="3" stroke-dasharray="6 4"/);
  assert.match(result.svg, /<title>Original fact<\/title>/);
  assert.match(result.svg, />Display A<\/text>/);
  const plan = canvasSceneToPlan(scene).plan;
  const shared = renderArchitecturePlan(plan, { canvas: true });
  assert.deepEqual(shared.layout.connections, result.layout.connections);
});

test('Canvas includes every label and supports empty scenes, rounding and overlapping layouts', () => {
  const empty = { schemaVersion: 1, facts: { nodes: [], edges: [] }, presentation: { nodes: [], edges: [] } };
  const result = renderCanvasScene(empty);
  assert.deepEqual(result.layout.viewBox, [0, 0, 320, 240]);
  assert.deepEqual(result.scene, empty);
  const scene = fixture();
  scene.presentation.nodes[0].displayLabel = '长标签'.repeat(100);
  scene.presentation.nodes[0].style.shape = 'rounded';
  scene.presentation.nodes[1].layout = { ...scene.presentation.nodes[0].layout };
  scene.presentation.edges[1].displayLabel = '';
  const overlapping = renderCanvasScene(scene);
  assert.ok(overlapping.svg.includes(scene.presentation.nodes[0].displayLabel));
  assert.ok(overlapping.layout.viewBox[2] > 4000);
  assert.match(overlapping.svg, /rx="6"/);
});

test('Canvas treats script, paths, CSS and template syntax as escaped text, never executable input', () => {
  const scene = fixture();
  const malicious = '</metadata><script src="https://evil.invalid/x"></script><img onerror="readFile()"> url(https://evil.invalid) $&';
  scene.facts.nodes[0].label = malicious;
  scene.facts.nodes[0].sources[0].note = malicious;
  scene.presentation.nodes[0].displayLabel = malicious;
  const result = renderCanvasScene(scene, { title: malicious });
  assert.deepEqual(result.scene, scene);
  assert.ok(!/<script\b|<img\b|<link\b|<iframe\b|<foreignObject\b/i.test(result.html));
  assert.ok(result.html.includes('&lt;script src=&quot;https://evil.invalid/x&quot;&gt;'));
  assert.match(result.html, /script-src 'none'/);
  assert.match(result.html, /connect-src 'none'/);
  // Check the actual resource policy and stylesheet content, not a blacklist
  // of host substrings (which is neither URL parsing nor a resource boundary).
  assert.match(result.html, /font-src 'none'/);
  const styles = [...result.html.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/gi)];
  assert.ok(styles.length > 0);
  for (const [, css] of styles) assert.doesNotMatch(css, /@import|@font-face|url\s*\(/i);
});

test('Canvas type/resource checks reject lossy, incomplete and active values before execution', () => {
  const changes = [
    scene => { scene.facts.edges[0].to = 'unknown'; },
    scene => { scene.presentation.nodes.pop(); },
    scene => { scene.presentation.edges[1].id = 'edge:1'; },
    scene => { scene.presentation.edges[0].points = [{ x: 0, y: 0 }]; },
    scene => { scene.presentation.nodes[0].layout.x = Infinity; },
    scene => { scene.presentation.nodes[0].style.fill = 'url(file:///private)'; },
    scene => { scene.facts.nodes[0].attributes.nested = {}; },
    scene => { scene.facts.nodes[0].label = '\ud800'; },
    scene => { scene.path = '/tmp/renderer-controlled'; },
    scene => { scene.facts.nodes = new Array(257).fill(scene.facts.nodes[0]); }
  ];
  for (const change of changes) { const scene = fixture(); change(scene); assert.throws(() => renderCanvasScene(scene)); }
  let executed = false;
  const scene = fixture();
  Object.defineProperty(scene.facts.nodes[0], 'label', { enumerable: true, get() { executed = true; return 'getter'; } });
  assert.throws(() => renderCanvasScene(scene));
  assert.equal(executed, false);
  const executable = fixture(); executable.toJSON = () => { executed = true; return {}; };
  assert.throws(() => renderCanvasScene(executable));
  assert.equal(executed, false);
});

test('local document refuses active SVG and external styles/resources', () => {
  for (const svg of ['<svg><script>alert(1)</script></svg>', '<svg><image href="file:///private"/></svg>', '<svg onload="x()"></svg>', '<svg><style>@import "https://evil.invalid";</style></svg>', '<svg><path fill="url(https://evil.invalid)"/></svg>']) {
    assert.throws(() => renderLocalDocument({ svg, title: 'Canvas' }));
  }
});

test('local document rejects external resources regardless of a familiar host in the URL', () => {
  const resources = [
    'https://fonts.googleapis.com.evil.invalid/a',
    'https://evil.invalid/fonts.gstatic.com/a',
    'https://fonts.googleapis.com@evil.invalid/a',
    '//evil.invalid/a'
  ];
  for (const resource of resources) {
    assert.throws(() => renderLocalDocument({ svg: `<svg><style>@import "${resource}";</style></svg>`, title: 'Canvas' }));
    assert.throws(() => renderLocalDocument({ svg: `<svg><path fill="url(${resource})"/></svg>`, title: 'Canvas' }));
  }
});

test('pure module import graph has no CLI, filesystem, process, network or dynamic-code dependency', () => {
  const entry = new URL('../renderers/architecture/canvas-scene.mjs', import.meta.url);
  const visited = new Set();
  function inspect(url) {
    if (visited.has(url.href)) return;
    visited.add(url.href);
    const code = fs.readFileSync(fileURLToPath(url), 'utf8');
    assert.doesNotMatch(code, /(?:from\s*|import\s*\()['"](?:node:|https?:)/);
    assert.doesNotMatch(code, /\b(?:eval|Function|fetch|require)\s*\(|\bprocess\s*\./);
    for (const [, specifier] of code.matchAll(/^(?:import|export)\s+[\w*{},\s]+\s+from\s*['"]([^'"]+)['"]/gm)) {
      assert.ok(specifier.startsWith('.'));
      assert.ok(!/(?:cli|validator)\.mjs$/.test(specifier));
      inspect(new URL(specifier, url));
    }
  }
  inspect(entry);
  assert.ok(visited.size >= 6);
});
