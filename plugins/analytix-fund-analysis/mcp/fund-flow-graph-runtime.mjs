import {
  compactWarningList,
  envelopeWarnings,
  evidenceRefsFromEnvelope,
  skillEnvelopeData,
  unwrapSkillEnvelope
} from "./agent-payload-compiler.mjs";
import { withAnswerCardProtocol } from "./answer-card-protocol.mjs";
import {
  buildFundGraphFromSeedRows,
  seedMatchesName,
  seedRowsFromTopOutflows,
  summarizeSeedTransfers
} from "./fundgraph-builder.mjs";
import { stableHash } from "./stable-hash.mjs";
import {
  executeLocalDuckdbWorkbenchSkill
} from "./duckdb-workbench-runtime.mjs";
import { throwIfAborted } from "./abort-runtime.mjs";
import {
  arrayOf,
  clampInt,
  pruneEmpty,
  text
} from "./runtime-normalizers.mjs";

export const FUND_FLOW_GRAPH_RUNTIME_VERSION = "0.15.128";

function moneyValue(value) {
  if (value && typeof value === "object") {
    const source = value;
    return moneyValue(source.yuan ?? source.amount_yuan ?? source.amount ?? source.value);
  }
  if (value == null || (typeof value === "string" && !value.trim())) return undefined;
  if (typeof value !== "number" && typeof value !== "string") return undefined;
  if (typeof value === "string" && !/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/iu.test(value.trim())) {
    return undefined;
  }
  const number = Number(typeof value === "string" ? value.trim() : value);
  return Number.isFinite(number) ? number : undefined;
}

function countValue(value) {
  if (value == null || (typeof value === "string" && !value.trim())) return undefined;
  const number = moneyValue(value);
  return number !== undefined && Number.isInteger(number) && number >= 0 ? number : undefined;
}

function firstMoneyFact(...values) {
  return values.find((value) => moneyValue(value) !== undefined);
}

function firstCount(...values) {
  for (const value of values) {
    const count = countValue(value);
    if (count !== undefined) return count;
  }
  return undefined;
}

function compactScopeStats(scope = {}) {
  const source = scope && typeof scope === "object" ? scope : {};
  return pruneEmpty({
    txn_count: firstCount(source.txn_count, source.directed_txn_count),
    account_count: firstCount(source.account_count, source.source_account_count),
    counterparty_count: firstCount(source.counterparty_count),
    first_txn_at: source.first_txn_at ?? source.min_txn_time,
    last_txn_at: source.last_txn_at ?? source.max_txn_time,
    amount: firstMoneyFact(source.out_amount, source.outflow_total, source.amount)
  }) || {};
}

function compactFlowEdgeRows(rows, limit = 8) {
  return arrayOf(rows).slice(0, limit).map((edge) => pruneEmpty({
    rank: edge.rank,
    from_label: text(edge.from_label),
    to_label: text(edge.to_label),
    txn_time: text(edge.txn_time),
    amount: firstMoneyFact(edge.amount),
    txn_count: firstCount(edge.txn_count, edge.count),
    edge_status: text(edge.edge_status),
    summary: text(edge.summary || edge.txn_type || edge.business_category_label),
    boundary: text(edge.boundary)
  }) || {}).filter((row) => Object.keys(row).length > 0);
}

function compactSeedOutflowRows(seedRows, limit = 10) {
  return arrayOf(seedRows).slice(0, limit).map((item) => {
    const row = item && typeof item === "object" ? item : {};
    const seed = row.seed && typeof row.seed === "object" ? row.seed : {};
    const raw = row.raw && typeof row.raw === "object" ? row.raw : {};
    const amount = firstMoneyFact(seed.amount, raw.amount, raw.outflow, raw.outflow_total, raw.turnover);
    const txnCount = firstCount(raw.txn_count, raw.count);
    const missingFields = [
      amount === undefined ? "amount" : "",
      txnCount === undefined ? "txn_count" : ""
    ].filter(Boolean);
    return pruneEmpty({
      rank: row.rank ?? raw.rank,
      counterparty_name: text(seed.counterparty_name || raw.counterparty_name || raw.display_name || raw.counterparty_key || seed.counterparty_key),
      counterparty_account: text(seed.counterparty_key || raw.counterparty_account || raw.counterparty_key),
      txn_time: text(seed.txn_time || raw.txn_time || raw.first_txn_at || raw.last_txn_at),
      first_txn_at: text(raw.first_txn_at || seed.txn_time),
      last_txn_at: text(raw.last_txn_at || seed.txn_time),
      amount,
      txn_count: txnCount,
      fact_status: missingFields.length ? "unresolved" : "unverified",
      missing_fields: missingFields,
      summary: text(seed.summary || raw.summary || raw.txn_type || raw.business_category_label),
      category: text(seed.business_category_label || raw.business_category_label || raw.terminal_category_label || raw.business_category),
      source_boundary: text(raw.boundary || seed.boundary)
    }) || {};
  }).filter((row) => Object.keys(row).length > 0 && (text(row.counterparty_name) || moneyValue(row.amount) !== undefined));
}

export function createFundFlowGraphRuntime({ resolveCase, executeSkill, env = {} }) {
  if (typeof resolveCase !== "function") {
    throw new TypeError("createFundFlowGraphRuntime requires resolveCase");
  }
  if (typeof executeSkill !== "function") {
    throw new TypeError("createFundFlowGraphRuntime requires executeSkill");
  }

  async function traceSubjectTopOutflows(payload, signal) {
    throwIfAborted(signal);
    try {
      const result = await executeSkill("trace_subject_top_outflows", payload, { signal });
      throwIfAborted(signal);
      return result;
    } catch (error) {
      throwIfAborted(signal);
      const result = await executeLocalDuckdbWorkbenchSkill("trace_subject_top_outflows", payload, { env, signal });
      throwIfAborted(signal);
      return {
        ...result,
        warnings: [
          ...arrayOf(result.warnings),
          {
            code: "FUND_FLOW_GRAPH_LOCAL_TRACE_FALLBACK",
            severity: "warning",
            message: `资金图上游 trace_subject_top_outflows 后端不可用，已改用当前案件 DuckDB 本地只读 fallback：${text(error?.message || error) || "未知错误"}`
          }
        ]
      };
    }
  }

  async function buildFundFlowGraph(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const caseId = resolved.case_id;
    const holderName = text(args.holder_name);
    const viaName = text(args.via_holder_name || args.counterparty_name);
    const topN = clampInt(args.top_n, 20, 1, 100);
    const sourceTraceTopN = viaName ? Math.max(topN, 50) : topN;
    const basePayload = {
      case_id: caseId,
      holder_name: holderName,
      id_no: text(args.id_no),
      account_keys: arrayOf(args.account_keys).map(text).filter(Boolean),
      date_start: text(args.date_start),
      date_end: text(args.date_end),
      include_candidate_accounts: false,
      top_n: sourceTraceTopN,
      trace_depth: 1,
      dedupe_seed_txn_id: true
    };
    const sourceResult = await traceSubjectTopOutflows(basePayload, signal);
    const sourceEnvelope = unwrapSkillEnvelope(sourceResult);
    const sourceData = skillEnvelopeData(sourceResult);
    const sourceRefs = evidenceRefsFromEnvelope(sourceEnvelope, sourceResult);
    const sourceSeeds = seedRowsFromTopOutflows(sourceData.top_outflows).filter((item) => seedMatchesName(item.seed, viaName));
    const sourceToViaSummary = viaName
      ? summarizeSeedTransfers(sourceSeeds, { fromLabel: holderName, toLabel: viaName })
      : undefined;

    let viaResult = null;
    let viaEnvelope = {};
    let viaRefs = {};
    let viaData = {};
    let viaSeeds = [];
    if (viaName) {
      viaResult = await traceSubjectTopOutflows({
        case_id: caseId,
        holder_name: viaName,
        date_start: text(args.date_start),
        date_end: text(args.date_end),
        include_candidate_accounts: false,
        top_n: topN,
        trace_depth: 1,
        dedupe_seed_txn_id: true
      }, signal);
      viaEnvelope = unwrapSkillEnvelope(viaResult);
      viaData = skillEnvelopeData(viaResult);
      viaRefs = evidenceRefsFromEnvelope(viaEnvelope, viaResult);
      viaSeeds = seedRowsFromTopOutflows(viaData.top_outflows);
    }

    const { flowGraph, edges, incompleteEdges } = buildFundGraphFromSeedRows({
      sourceSeeds,
      viaSeeds,
      topN,
      holderName,
      viaName
    });
    const sourceEdges = edges.slice(0, sourceSeeds.length);
    const downstreamEdges = edges.slice(sourceSeeds.length);
    const sourceDrawableEdges = sourceEdges.filter((edge) => text(edge.edge_status) === "supported");
    const downstreamDrawableEdges = downstreamEdges.filter((edge) => text(edge.edge_status) === "supported");
    const drawableEdgeCount = sourceDrawableEdges.length + downstreamDrawableEdges.length;
    const hasSupportedEdges = drawableEdgeCount > 0;
    const sourceEdgeRows = compactFlowEdgeRows(sourceDrawableEdges, 8);
    const downstreamEdgeRows = compactFlowEdgeRows(downstreamDrawableEdges, 10);
    const reviewEdgeRows = compactFlowEdgeRows(incompleteEdges, 12);
    const downstreamOutflowRows = compactSeedOutflowRows(viaSeeds, 12);
    const fundFlowFactPack = pruneEmpty({
      source_holder: holderName,
      via_holder: viaName,
      date_start: text(args.date_start),
      date_end: text(args.date_end),
      source_scope_stats: compactScopeStats(sourceData.scope_stats || {}),
      via_scope_stats: viaResult ? compactScopeStats(viaData.scope_stats || {}) : undefined,
      source_to_via_summary: sourceToViaSummary,
      source_edges: sourceEdgeRows,
      downstream_edges: downstreamEdgeRows,
      review_edges: reviewEdgeRows,
      downstream_outflows: downstreamOutflowRows,
      edge_count: edges.length,
      drawable_edge_count: drawableEdgeCount,
      supported_edge_count: drawableEdgeCount,
      incomplete_edge_count: incompleteEdges.length,
      current_answer_sufficiency: hasSupportedEdges
        ? "已返回当前问题可成稿的上游链路、后续出账线索和边界；普通后续去向题应先据此作答，并把未逐笔余额承接、产品赎回或最终受益人事项写成需复核/需补证。"
        : "未返回可成稿资金链路；只能说明证据缺口并建议指定交易或补调流水。"
    }) || {};
    const warnings = compactWarningList(
      envelopeWarnings(sourceEnvelope),
      envelopeWarnings(viaEnvelope),
      incompleteEdges.length
        ? [{ code: "FLOW_GRAPH_EDGE_FIELDS_MISSING", severity: "warning", message: `${incompleteEdges.length} 条边存在缺端点、同名待复核或关键字段缺口，不能作为最终箭头。` }]
        : []
    );
    return {
      tool: "build_fund_flow_graph",
      status: incompleteEdges.length ? "partial" : "ok",
      case_id: caseId,
      answer_card: withAnswerCardProtocol({
        title: "资金穿透图",
        summary: hasSupportedEdges
          ? "已按本轮可核验交易整理资金链路；图中只呈现端点完整的交易边，旁路线索和缺失端点另列待核事项。"
          : "本轮未形成可直接画入图中的交易边；可先列资金断点和补证事项，不绘制无依据箭头。",
        source_holder: holderName,
        via_holder: viaName,
        edge_count: edges.length,
        incomplete_edge_count: incompleteEdges.length,
        required_visible_boundaries: [
          "线索候选 / 需补证 / 部分可见 / 缺失端点只能列为候选或需复核边界",
          "证据缺口: 缺交易号、对象、回单、开户资料、收款侧流水或余额承接前不能升级为交易级可证实资金链路",
          "金额写法: 关键金额同时保留元和万元，例如 20,000,000.00 元（2,000.00 万元）"
        ]
      }, {
        recommendedNextAction: hasSupportedEdges ? "answer_now_from_supported_edges" : "answer_with_boundary_and_request_specific_trace_seed",
        maxAdditionalTools: 0,
        requiredFactsPresent: hasSupportedEdges,
        unsupportedFlowsPresent: incompleteEdges.length > 0 || !hasSupportedEdges
      }),
      key_facts: {
        graph_delivery_contract: {
          output_order: [
            "结论",
            "资金来源",
            "主要资金链路",
            "下游去向",
            "资金断点",
            "待补证事项"
          ],
          source_cleaning_boundary: "来源为当前案件规范明细/分析索引；同事实、换卡/补卡、重复交易号、缺失对手和金额集中均需在补证事项中简明说明。",
          edge_boundary: "图中只画对象完整且金额、时间、付款对象、收款对象均可核验的交易；缺对象、同名待核或聚合对象写入资金断点和待补证事项，并标明需复核/需补证。",
          next_review_focus: "后续补调银行回单、收款账户流水、开户信息、用途说明、产品凭证和余额承接；继续追一层时按确定性交易边选择种子。",
          final_answer_sections: [
            "结论",
            "资金来源",
            "主要资金链路",
            "下游去向",
            "资金断点",
            "待补证事项"
          ]
        },
        source_scope_stats: compactScopeStats(sourceData.scope_stats || {}),
        via_scope_stats: viaResult ? compactScopeStats(viaData.scope_stats || {}) : {},
        source_seed_count: sourceSeeds.length,
        downstream_seed_count: viaSeeds.length,
        source_to_via_summary: sourceToViaSummary,
        flow_graph: flowGraph,
        fund_flow_fact_pack: fundFlowFactPack
      },
      flow_graph: flowGraph,
      warnings,
      evidence_refs: pruneEmpty({
        query_ids: [...arrayOf(sourceRefs.query_ids), ...arrayOf(viaRefs.query_ids)].map(text).filter(Boolean).slice(0, 16),
        audit_ref: { graph_id: `fund-flow:${caseId}:${stableHash(edges)}` }
      }) || {},
      next_actions: [
        {
          reason: hasSupportedEdges
            ? "本工具已经返回可画图/可解释的确定性交易边；普通问答应先直接作答，不要再次扩大 top_n 或重复调用 build_fund_flow_graph。只有用户明确要求附件补调清单或指定某条交易继续穿透时，再进入校验/追踪。"
            : "本轮没有形成可绘制的确定性交易边；只能说明已检查范围、数值或端点缺口，并建议补调具体交易凭证。"
        }
      ],
      answer_contract: {
        stop_after_graph: "若已有资金链路，先基于本轮图谱事实作答；除非用户明确要求更多链路，否则不要重复扩大图谱范围。",
        output_order: "资金流向图按结论、资金来源、主要资金链路、下游去向、资金断点、待补证事项组织；旁路线索用待核列表表达，不强制固定审计标题。",
        source_cleaning_rule: "说明当前案件清洗明细或分析索引范围、重复/同事实/换卡边界、缺失对手方边界和需复核事项。",
        arrow_rule: "只绘制对象完整且达到交易级可证实资金边标准的资金链路；缺失对象、同名待核或聚合对手方只能作为下一步追查对象，不画成图谱箭头，并写需复核/需补证。"
      }
    };
  }

  return { buildFundFlowGraph };
}
