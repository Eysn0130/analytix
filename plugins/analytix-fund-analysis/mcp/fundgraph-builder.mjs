import crypto from "node:crypto";
import { withFundGraphProtocol } from "./casegraph-protocol.mjs";

export const FUNDGRAPH_BUILDER_VERSION = "0.14.4-fundgraph-builder";

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function stableHash(value) {
  return crypto
    .createHash("sha256")
    .update(JSON.stringify(value || {}, (_key, item) => (typeof item === "bigint" ? String(item) : item)))
    .digest("hex");
}

function pruneEmpty(value) {
  if (Array.isArray(value)) {
    return value.map(pruneEmpty).filter((item) => item !== undefined);
  }
  if (!value || typeof value !== "object") {
    return value === "" || value === null ? undefined : value;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    const next = pruneEmpty(item);
    if (next !== undefined) output[key] = next;
  }
  return Object.keys(output).length ? output : undefined;
}

function uniqueTexts(values, limit = 8) {
  const seen = new Set();
  const output = [];
  for (const value of arrayOf(values)) {
    const item = text(value);
    if (!item || seen.has(item)) continue;
    seen.add(item);
    output.push(item);
    if (output.length >= limit) break;
  }
  return output;
}

function finiteNumber(value) {
  if (value == null || (typeof value === "string" && !value.trim())) return undefined;
  if (typeof value !== "number" && typeof value !== "string") return undefined;
  if (typeof value === "string" && !/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/iu.test(value.trim())) {
    return undefined;
  }
  const number = Number(typeof value === "string" ? value.trim() : value);
  return Number.isFinite(number) ? number : undefined;
}

function positiveInt(value) {
  const number = finiteNumber(value);
  return number !== undefined && Number.isInteger(number) && number > 0 ? number : undefined;
}

function compactMoneyFact(value) {
  if (value && typeof value === "object") {
    const source = objectOf(value);
    const candidate = source.yuan ?? source.amount_yuan ?? source.amount ?? source.value;
    if (candidate !== undefined && candidate !== null && candidate !== "") {
      return compactMoneyFact(candidate);
    }
  }
  const number = finiteNumber(value);
  if (number === undefined) return undefined;
  return {
    yuan: Number(number.toFixed(2)),
    wan: Number((number / 10000).toFixed(6)),
    text: `${number.toFixed(2)} 元（${(number / 10000).toFixed(6)} 万元）`
  };
}

function weakEndpointLabel(value) {
  const item = text(value);
  if (!item) return true;
  return /^(unknown|未知|未匹配|缺失|空|无|none|null)$/iu.test(item)
    || /(对手.*缺失|字段缺失|未匹配|unknown|needs[_ -]?review|partial|candidate)/iu.test(item);
}

function sameEndpointNeedsReview(fromLabel, toLabel) {
  const from = text(fromLabel);
  const to = text(toLabel);
  return Boolean(from && to && from === to);
}

function compactSeedTxn(seedTxn) {
  const seed = objectOf(seedTxn);
  return pruneEmpty({
    txn_id: seed.txn_id,
    txn_time: seed.txn_time,
    account_key: seed.account_key,
    account_open_name: seed.account_open_name,
    counterparty_key: seed.counterparty_key,
    counterparty_name: seed.counterparty_name,
    amount: compactMoneyFact(seed.amount),
    direction: text(seed.direction),
    summary: seed.summary,
    txn_type: seed.txn_type,
    business_category: seed.business_category,
    business_category_label: seed.business_category_label,
    file_id: seed.file_id
  }) || {};
}

function upsertGraphNode(nodes, node) {
  const id = text(node.id);
  if (!id) return;
  if (!nodes.has(id)) nodes.set(id, node);
}

function flowEdgeFromSeed({ seed, fromLabel, toLabel, sourceTool, rank, boundary = "" }) {
  const txnId = text(seed.txn_id);
  const txnTime = text(seed.txn_time);
  const rawAmount = seed.amount?.yuan ?? seed.amount;
  const amount = finiteNumber(rawAmount);
  const direction = text(seed.direction).toLowerCase();
  const normalizedRank = positiveInt(rank);
  const fromId = text(seed.account_key) || `holder:${fromLabel}`;
  const targetLabel = text(toLabel || seed.counterparty_name || seed.counterparty_key);
  const toId = text(seed.counterparty_key) || `counterparty:${targetLabel || "unknown"}`;
  const missing = [];
  if (!txnId) missing.push("txn_id");
  if (!txnTime) missing.push("txn_time");
  if (amount === undefined) missing.push("amount");
  if (amount !== undefined && amount <= 0) missing.push("positive_amount");
  if (!/^(?:in|out)$/u.test(direction)) missing.push("direction");
  if (weakEndpointLabel(targetLabel)) missing.push("counterparty_endpoint");
  if (sameEndpointNeedsReview(fromLabel || seed.account_open_name, targetLabel)) {
    missing.push("same_name_endpoint_review");
  }
  const hasEndpointReviewGap = missing.includes("counterparty_endpoint")
    || missing.includes("same_name_endpoint_review");
  return pruneEmpty({
    edge_id: `edge-${stableHash({ txnId, txnTime, amount, fromId, toId, rank: normalizedRank })}`,
    edge_status: missing.length ? (hasEndpointReviewGap ? "needs_review" : "needs_evidence") : "candidate",
    missing_fields: missing,
    rank: normalizedRank,
    from: fromId,
    from_label: fromLabel || seed.account_open_name,
    to: toId,
    to_label: targetLabel,
    txn_id: txnId,
    txn_time: txnTime,
    amount: amount !== undefined ? compactMoneyFact(amount) : undefined,
    direction: /^(?:in|out)$/u.test(direction) ? direction : undefined,
    summary: seed.summary,
    txn_type: seed.txn_type,
    business_category: seed.business_category,
    business_category_label: seed.business_category_label,
    source_tool: sourceTool,
    host_registry_verified: false,
    boundary: boundary || "该笔仅为当前流水生成的交易候选；未经宿主 EvidenceReceipt registry 同案、同轮、同快照核验，不得发布为一跳事实或延伸推断后续去向。"
  }) || {};
}

export function seedRowsFromTopOutflows(rows) {
  return arrayOf(rows).map((row) => {
    const source = objectOf(row);
    return {
      rank: positiveInt(source.rank),
      seed: compactSeedTxn(source.seed_txn || source),
      terminal_category: source.terminal_category,
      terminal_category_label: source.terminal_category_label,
      raw: source
    };
  });
}

export function seedMatchesName(seed, expectedName) {
  const needle = text(expectedName);
  if (!needle) return true;
  const values = [
    seed.counterparty_name,
    seed.counterparty_key,
    seed.business_category_label,
    seed.summary,
    seed.txn_type
  ].map(text);
  return values.some((value) => value.includes(needle));
}

export function summarizeSeedTransfers(seedRows, { fromLabel = "", toLabel = "" } = {}) {
  const seeds = arrayOf(seedRows)
    .map((row) => objectOf(row).seed || row)
    .map(objectOf)
    .map((seed) => ({
      ...seed,
      normalized_amount: finiteNumber(seed.amount?.yuan ?? seed.amount)
    }))
    .filter((seed) => seed.normalized_amount !== undefined && seed.normalized_amount >= 0);
  if (!seeds.length) return undefined;
  const total = seeds.reduce((sum, seed) => sum + seed.normalized_amount, 0);
  const times = seeds.map((seed) => text(seed.txn_time)).filter(Boolean).sort();
  const byDate = new Map();
  for (const seed of seeds) {
    const date = text(seed.txn_time).slice(0, 10) || "unknown";
    const current = byDate.get(date) || { date, txn_count: 0, amount: 0 };
    current.txn_count += 1;
    current.amount += seed.normalized_amount;
    byDate.set(date, current);
  }
  const largestDateCluster = [...byDate.values()]
    .sort((a, b) => b.amount - a.amount || String(b.date).localeCompare(String(a.date)))
    .shift();
  return pruneEmpty({
    from_holder: fromLabel,
    to_holder: toLabel,
    source_accounts: uniqueTexts(seeds.map((seed) => seed.account_key), 6),
    counterparty_accounts: uniqueTexts(seeds.map((seed) => seed.counterparty_key), 6),
    txn_count: seeds.length,
    amount: compactMoneyFact(total),
    support_status: "candidate",
    host_registry_verified: false,
    verified_no_hit: false,
    first_txn_at: times[0],
    last_txn_at: times[times.length - 1],
    largest_date_cluster: largestDateCluster
      ? {
          date: largestDateCluster.date,
          txn_count: largestDateCluster.txn_count,
          amount: compactMoneyFact(largestDateCluster.amount)
        }
      : undefined,
    dedupe_basis: "按后端 Top 出账 seed 的有效 txn_id 去重后汇总；无有效 txn_id 的行不做跨账户盲合并。",
    boundary: "该汇总用于避免同一主体换卡/补卡重复放大；最终穿透仍需逐笔余额承接或收款侧明细核验。"
  }) || undefined;
}

export function buildFundGraphFromSeedRows({ sourceSeeds = [], viaSeeds = [], topN = 20, holderName = "", viaName = "" } = {}) {
  const nodes = new Map();
  const edges = [];
  for (const item of arrayOf(sourceSeeds).slice(0, topN)) {
    const seed = objectOf(item).seed || {};
    const fromLabel = seed.account_open_name || holderName || seed.account_key;
    const toLabel = seed.counterparty_name || viaName || seed.counterparty_key;
    upsertGraphNode(nodes, { id: seed.account_key || `holder:${fromLabel}`, label: fromLabel, kind: "source_account_or_holder" });
    upsertGraphNode(nodes, { id: seed.counterparty_key || `counterparty:${toLabel}`, label: toLabel, kind: viaName ? "intermediate_counterparty" : "counterparty" });
    edges.push(flowEdgeFromSeed({ seed, fromLabel, toLabel, sourceTool: "trace_subject_top_outflows", rank: item.rank }));
  }
  for (const item of arrayOf(viaSeeds).slice(0, topN)) {
    const seed = objectOf(item).seed || {};
    const fromLabel = seed.account_open_name || viaName || seed.account_key;
    const toLabel = seed.counterparty_name || seed.counterparty_key;
    upsertGraphNode(nodes, { id: seed.account_key || `holder:${fromLabel}`, label: fromLabel, kind: "intermediate_account_or_holder" });
    upsertGraphNode(nodes, { id: seed.counterparty_key || `counterparty:${toLabel}`, label: toLabel, kind: "downstream_counterparty" });
    edges.push(flowEdgeFromSeed({ seed, fromLabel, toLabel, sourceTool: "trace_subject_top_outflows", rank: item.rank }));
  }
  const incompleteEdges = edges.filter((edge) => edge.edge_status !== "supported");
  const flowGraph = withFundGraphProtocol({
    graph_version: "fund-flow-graph-v1",
    nodes: [...nodes.values()],
    edges,
    boundaries: [
      "绘制资金流向图前，先列出可证实资金链路事实，包括状态、时间、金额、付款账户/主体、收款对象、交易号状态和边界。",
      "来源范围为当前案件已清洗明细或分析索引事实；清洗重复标记不等于已排除全部重复，同事实、换卡、补卡风险仍应写为需复核边界。",
      "只能绘制对象完整且可证实的交易资金链路；不得凭叙述记忆补画箭头。",
      "聚合排行或对手方总额不是交易链路，除非有交易号和交易时间支撑。",
      "未匹配、缺失或同名待核对象只能作为下一步追查对象和边界行，不能画成资金流向图的确定性箭头。"
    ],
    mermaid_hint: "只有对象完整、无复核缺口且可证实的交易资金链路，才能进入资金流向图。"
  }, { intent: "flow_graph" });
  return { flowGraph, edges, incompleteEdges };
}
