#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';

function fail(message) {
  console.error(message);
  process.exit(2);
}

function text(value, fallback = '') {
  const next = String(value ?? '').trim();
  return next || fallback;
}

function id(value, fallback) {
  const raw = text(value, fallback).replace(/[^a-zA-Z0-9_-]/g, '_');
  const normalized = raw.replace(/_+/g, '_').replace(/^_+|_+$/g, '');
  return /^[a-zA-Z]/.test(normalized) ? normalized : `n_${normalized || fallback}`;
}

function nodeType(kind) {
  const value = text(kind).toLowerCase();
  if (/(person|人员|subject|主体|company|公司|case|案件|document|公文)/u.test(value)) return 'external';
  if (/(account|账户|bank|资金|equity|股权|database|库)/u.test(value)) return 'database';
  if (/(evidence|证据|security|审核|法制|审批)/u.test(value)) return 'security';
  if (/(process|流程|workflow|stage|环节)/u.test(value)) return 'backend';
  return 'backend';
}

function variant(kind, confidence) {
  const value = text(kind).toLowerCase();
  if (/(primary|main|主|重点|核心|资金)/u.test(value)) return 'emphasis';
  if (/(evidence|security|证据|审核|敏感|合规)/u.test(value)) return 'security';
  if (Number(confidence) > 0 && Number(confidence) < 0.75) return 'dashed';
  return 'default';
}

function inferType(input) {
  const explicit = text(input.diagramType || input.diagram_type).toLowerCase();
  if (explicit) return explicit;
  const graphType = text(input.graphType || input.graph_type || input.intent || input.domain).toLowerCase();
  if (/(fund|资金|flow|流向|data)/u.test(graphType)) return 'dataflow';
  if (/(evidence|证据|official|公文|公安|procedure|流程|workflow)/u.test(graphType)) return 'workflow';
  if (/(status|state|状态|生命周期|lifecycle)/u.test(graphType)) return 'lifecycle';
  return 'architecture';
}

function architecture(input) {
  const nodes = Array.isArray(input.nodes) ? input.nodes : [];
  const edges = Array.isArray(input.edges) ? input.edges : [];
  return {
    schema_version: 1,
    diagram_type: 'architecture',
    meta: {
      title: text(input.title, 'AtlasFlow Domain Diagram'),
      subtitle: text(input.subtitle || input.domain || input.graphType)
    },
    layout: { mode: 'grid', cols: Math.min(7, Math.max(3, nodes.length || 3)) },
    components: nodes.map((node, index) => ({
      id: id(node.id, `node_${index + 1}`),
      type: nodeType(node.kind || node.type),
      label: text(node.label || node.name, `Node ${index + 1}`),
      ...(text(node.sublabel || node.subtitle) ? { sublabel: text(node.sublabel || node.subtitle) } : {}),
      row: Math.floor(index / 4),
      col: index % 4
    })),
    connections: edges.map((edge) => ({
      from: id(edge.from, 'from'),
      to: id(edge.to, 'to'),
      ...(text(edge.label || edge.kind) ? { label: text(edge.label || edge.kind) } : {}),
      variant: variant(edge.kind, edge.confidence)
    })),
    cards: cards(input)
  };
}

function workflow(input) {
  const nodes = Array.isArray(input.nodes) ? input.nodes : [];
  const edges = Array.isArray(input.edges) ? input.edges : [];
  const compact = nodes.length > 4;
  const defaultCols = compact ? [0, 1, 2, 3, 4, 5] : [0, 1, 3, 5];
  const lanes = Array.isArray(input.lanes) && input.lanes.length > 0
    ? input.lanes.map((lane, index) => ({ id: id(lane.id, `lane_${index + 1}`), label: text(lane.label, `Lane ${index + 1}`) }))
    : [{ id: 'main', label: text(input.domain, 'Main Flow') }];
  return {
    schema_version: 1,
    diagram_type: 'workflow',
    meta: {
      title: text(input.title, 'AtlasFlow Workflow'),
      subtitle: text(input.subtitle || input.domain || input.graphType)
    },
    lanes,
    mainPath: nodes.map((node, index) => id(node.id, `node_${index + 1}`)),
    nodes: nodes.map((node, index) => ({
      id: id(node.id, `node_${index + 1}`),
      lane: id(node.lane, lanes[Math.min(index, lanes.length - 1)]?.id || 'main'),
      col: Math.min(5, Number.isFinite(Number(node.col)) ? Number(node.col) : defaultCols[index] ?? 5),
      type: nodeType(node.kind || node.type),
      label: text(node.label || node.name, `Step ${index + 1}`),
      ...(compact && !node.width ? { width: 60 } : {}),
      ...(text(node.sublabel || node.subtitle) ? { sublabel: text(node.sublabel || node.subtitle) } : {})
    })),
    edges: edges.map((edge) => ({
      from: id(edge.from, 'from'),
      to: id(edge.to, 'to'),
      ...(text(edge.label || edge.kind) ? { label: text(edge.label || edge.kind) } : {}),
      ...(compact && !edge.route ? { route: 'bottom-channel' } : {}),
      ...(compact && lanes.length === 1 && !edge.channelY ? { channelY: 164 } : {}),
      variant: variant(edge.kind, edge.confidence)
    })),
    cards: cards(input)
  };
}

function dataflow(input) {
  const nodes = Array.isArray(input.nodes) ? input.nodes : [];
  const edges = Array.isArray(input.edges) ? input.edges : [];
  const stages = Array.isArray(input.stages) && input.stages.length >= 2
    ? input.stages.map((stage) => ({ label: text(stage.label || stage) }))
    : [{ label: '来源' }, { label: '中转' }, { label: '去向' }];
  return {
    schema_version: 1,
    diagram_type: 'dataflow',
    meta: {
      title: text(input.title, 'AtlasFlow Data Flow'),
      subtitle: text(input.subtitle || input.domain || input.graphType)
    },
    stages,
    nodes: nodes.map((node, index) => ({
      id: id(node.id, `node_${index + 1}`),
      type: nodeType(node.kind || node.type),
      label: text(node.label || node.name, `Node ${index + 1}`),
      stage: Math.min(stages.length - 1, Number.isFinite(Number(node.stage)) ? Number(node.stage) : index % stages.length),
      row: Math.min(4, Number.isFinite(Number(node.row)) ? Number(node.row) : Math.floor(index / stages.length)),
      ...(text(node.sublabel || node.subtitle) ? { sublabel: text(node.sublabel || node.subtitle) } : {})
    })),
    flows: edges.map((edge) => ({
      from: id(edge.from, 'from'),
      to: id(edge.to, 'to'),
      label: text(edge.label || edge.amount || edge.kind, 'flow'),
      ...(text(edge.classification) ? { classification: text(edge.classification) } : {}),
      variant: variant(edge.kind, edge.confidence)
    })),
    cards: cards(input)
  };
}

function lifecycle(input) {
  const nodes = Array.isArray(input.nodes) ? input.nodes : [];
  const edges = Array.isArray(input.edges) ? input.edges : [];
  return {
    schema_version: 1,
    diagram_type: 'lifecycle',
    meta: {
      title: text(input.title, 'AtlasFlow Lifecycle'),
      subtitle: text(input.subtitle || input.domain || input.graphType)
    },
    lanes: [
      { id: 'main', label: '主要阶段' },
      { id: 'waiting', label: '等待/补正' },
      { id: 'terminal', label: '终态' }
    ],
    states: nodes.map((node, index) => ({
      id: id(node.id, `state_${index + 1}`),
      type: index === 0 ? 'start' : index === nodes.length - 1 ? 'success' : 'active',
      label: text(node.label || node.name, `State ${index + 1}`),
      lane: text(node.lane, index === nodes.length - 1 ? 'terminal' : 'main'),
      col: Math.min(4, Number.isFinite(Number(node.col)) ? Number(node.col) : index),
      ...(text(node.step) ? { step: text(node.step) } : {})
    })),
    transitions: edges.map((edge) => ({
      from: id(edge.from, 'from'),
      to: id(edge.to, 'to'),
      ...(text(edge.label || edge.kind) ? { label: text(edge.label || edge.kind) } : {}),
      variant: variant(edge.kind, edge.confidence)
    })),
    cards: cards(input)
  };
}

function cards(input) {
  const result = [];
  if (Array.isArray(input.evidence) && input.evidence.length > 0) {
    result.push({
      dot: 'emerald',
      title: '证据来源',
      items: input.evidence.slice(0, 5).map((item) => text(item.source || item.id || item, 'evidence'))
    });
  }
  if (input.privacy) {
    result.push({
      dot: 'amber',
      title: '隐私处理',
      items: [JSON.stringify(input.privacy)]
    });
  }
  return result;
}

const [inputPath, outputPath] = process.argv.slice(2);
if (!inputPath || !outputPath) fail('Usage: node scripts/domain-ir-to-atlasflow.mjs <input.domain.json> <output.atlasflow.json>');

const input = JSON.parse(fs.readFileSync(inputPath, 'utf8'));
const type = inferType(input);
const output = type === 'dataflow'
  ? dataflow(input)
  : type === 'workflow'
    ? workflow(input)
    : type === 'lifecycle'
      ? lifecycle(input)
      : architecture(input);

fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(output, null, 2)}\n`, 'utf8');
console.log(JSON.stringify({ ok: true, type, output: outputPath }, null, 2));
