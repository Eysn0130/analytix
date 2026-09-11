import { redactAgentPayload } from "./agent-context-hygiene.mjs";
import {
  compactCaseGraphFromScopeMap,
  compactRankingRows,
  compactWarningList,
  envelopeWarnings,
  evidenceRefsFromEnvelope,
  skillEnvelopeData,
  unwrapSkillEnvelope
} from "./agent-payload-compiler.mjs";
import {
  buildCasegraphComponentEvidence,
  casegraphMinimumRoadmap,
  withCasegraphAnswerCardProtocol
} from "./casegraph-protocol.mjs";
import { stableHash } from "./stable-hash.mjs";
import { throwIfAborted } from "./abort-runtime.mjs";
import {
  arrayOf,
  clampInt,
  intOrUndefined,
  numberOrUndefined,
  objectOf,
  pruneEmpty,
  text,
  uniqueTexts
} from "./runtime-normalizers.mjs";

export const CASEGRAPH_RUNTIME_VERSION = "0.14.4";

const RANK_FACT_BOUNDARY = "Deterministic aggregate candidate only. Until the host verifies same-context EvidenceReceipt registry membership, it is not a publishable fact, ownership confirmation, final flow destination, or legal characterization.";
const HOST_RECEIPT_BOUNDARY = "The plugin cannot verify host EvidenceReceipt registry membership. Rank aggregates and generated refs remain candidate/unresolved until the host Final Evidence Gate verifies the same case, turn, epoch, and dataset snapshot.";

const RANK_LABELS = {
  account: "账户排行",
  holder: "户名排行",
  counterparty: "对手方排行"
};

function rankFactEntity(row, rankKind) {
  const source = objectOf(row);
  if (rankKind === "account") {
    return text(source.account_display || source.account_key || source.display_name || source.holder_name);
  }
  if (rankKind === "holder") {
    return text(source.holder_name || source.display_name || source.account_key);
  }
  return text(source.display_name || source.counterparty_key || source.counterparty_account || source.holder_name);
}

function nonNegativeInt(value) {
  const number = intOrUndefined(value);
  return number !== undefined && number >= 0 ? number : undefined;
}

function positiveInt(value) {
  const number = intOrUndefined(value);
  return number !== undefined && number > 0 ? number : undefined;
}

function compactRankMoney(value) {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const source = objectOf(value);
    return compactRankMoney(source.yuan ?? source.amount_yuan ?? source.amount ?? source.value);
  }
  const number = numberOrUndefined(value);
  if (number === undefined) return undefined;
  return {
    yuan: Number(number.toFixed(2)),
    wan: Number((number / 10_000).toFixed(6)),
    text: `${number.toFixed(2)} 元（${(number / 10_000).toFixed(6)} 万元）`
  };
}

function rankMetricAmount(row, metric) {
  const source = objectOf(row);
  return compactRankMoney(
    source[metric]
      ?? source.metric_value
      ?? source.turnover
      ?? source.inflow
      ?? source.outflow
      ?? source.net_flow
  );
}

function rankScopeFromPayload(rankPayload = {}, caseId = "") {
  const payload = objectOf(rankPayload);
  return pruneEmpty({
    case_id: text(caseId || payload.case_id),
    metric: text(payload.metric) || "turnover",
    holder_name: text(payload.holder_name),
    id_no_provided: Boolean(text(payload.id_no)),
    account_key_count: arrayOf(payload.account_keys).length,
    success_filter: text(payload.success_filter),
    cash_filter: text(payload.cash_filter),
    limit: positiveInt(payload.limit)
  }) || {};
}

function rankFactId(row, rankKind, index) {
  const source = objectOf(row);
  return `casegraph.rank.${rankKind}.${stableHash({
    rank: positiveInt(source.rank),
    source_index: index + 1,
    account_key: source.account_key,
    holder_name: source.holder_name,
    counterparty_key: source.counterparty_key,
    counterparty_account: source.counterparty_account,
    display_name: source.display_name
  })}`;
}

function rankFactSourceRefs(row, {
  evidenceRefs = {},
  factId = "",
  rankKind = "rank",
  sourceTool = "",
  reportId = ""
} = {}) {
  const source = objectOf(row);
  const refs = objectOf(evidenceRefs);
  return pruneEmpty({
    query_ids: uniqueTexts([
      ...arrayOf(refs.query_ids),
      text(sourceTool),
      `casegraph_${rankKind}_rank`
    ], 16),
    path_ids: uniqueTexts([
      ...arrayOf(refs.path_ids),
      text(factId)
    ], 12),
    report_ids: uniqueTexts([
      ...arrayOf(refs.report_ids),
      text(reportId),
      `casegraph.rank.${rankKind}`
    ], 12),
    audit_ref: {
      ...objectOf(refs.audit_ref),
      rank_kind: rankKind,
      host_registry_verified: false
    },
    detail_ref: objectOf(refs.detail_ref),
    artifact_id: text(refs.artifact_id)
  }) || {};
}

function rankClaimText(pack, rankKind) {
  const source = objectOf(pack);
  const label = rankFactEntity(source, rankKind) || RANK_LABELS[rankKind] || "排行事实";
  const metric = text(source.metric) || "turnover";
  const amountText = text(objectOf(source.amount).text);
  const rank = positiveInt(source.rank);
  const txnCount = nonNegativeInt(source.txn_count);
  return [
    rank
      ? `${RANK_LABELS[rankKind] || "排行"} ${label} 按 ${metric} 排名第 ${rank}`
      : `${RANK_LABELS[rankKind] || "排行"} ${label} 的 ${metric} 排行位置未决`,
    amountText ? `${metric} ${amountText}` : "",
    txnCount !== undefined ? `${txnCount} 笔（显式 0 仅表示候选聚合值，不构成 verified no-hit）` : ""
  ].filter(Boolean).join("，");
}

export function buildCasegraphRankEvidence({
  rows = [],
  rankKind = "rank",
  sourceTool = "",
  evidenceRefs = {},
  rankPayload = {},
  caseId = ""
} = {}) {
  const metric = text(objectOf(rankPayload).metric) || "turnover";
  const scope = rankScopeFromPayload(rankPayload, caseId);
  const annotatedRows = arrayOf(rows).map((row, index) => {
    const source = objectOf(row);
    const factId = rankFactId(source, rankKind, index);
    const rank = positiveInt(source.rank);
    const entity = rankFactEntity(source, rankKind);
    const amount = rankMetricAmount(source, metric);
    const missingFields = [
      !rank ? "rank" : "",
      !entity ? "rank_entity" : "",
      !amount ? "metric_amount" : ""
    ].filter(Boolean);
    const supportStatus = missingFields.length ? "unresolved" : "candidate";
    const sourceRefs = rankFactSourceRefs(source, {
      evidenceRefs,
      factId,
      rankKind,
      sourceTool,
      reportId: `casegraph.rank.${rankKind}.${index + 1}`
    });
    return pruneEmpty({
      ...source,
      fact_id: factId,
      rank_kind: rankKind,
      rank,
      account_count: nonNegativeInt(source.account_count),
      txn_count: nonNegativeInt(source.txn_count),
      inflow: compactRankMoney(source.inflow),
      outflow: compactRankMoney(source.outflow),
      turnover: compactRankMoney(source.turnover),
      net_flow: compactRankMoney(source.net_flow),
      max_single: compactRankMoney(source.max_single),
      metric_value: compactRankMoney(source.metric_value),
      support_status: supportStatus,
      missing_fields: missingFields,
      host_registry_verified: false,
      citation_status: "unresolved",
      publication_allowed: false,
      fact_answer_allowed: false,
      evidence_receipt_ids: undefined,
      citations: undefined,
      evidence_ids: undefined,
      metric,
      scope,
      source_tool: sourceTool,
      source_refs: sourceRefs,
      boundary: source.boundary || RANK_FACT_BOUNDARY
    }) || {};
  });
  const evidencePack = annotatedRows.map((row, index) => {
    const source = objectOf(row);
    return pruneEmpty({
      pack_id: `casegraph.rank.${rankKind}.${index + 1}`,
      evidence_type: `casegraph_${rankKind}_rank_fact`,
      fact_id: source.fact_id,
      support_status: source.support_status,
      host_registry_verified: false,
      citation_status: "unresolved",
      rank_kind: rankKind,
      rank: positiveInt(source.rank),
      metric: source.metric || metric,
      scope: source.scope,
      account_key: source.account_key,
      account_display: source.account_display,
      holder_name: source.holder_name,
      counterparty_key: source.counterparty_key,
      counterparty_account: source.counterparty_account,
      counterparty_name: source.display_name,
      display_name: source.display_name,
      account_count: source.account_count,
      txn_count: source.txn_count,
      amount: rankMetricAmount(source, metric),
      inflow: source.inflow,
      outflow: source.outflow,
      net_flow: source.net_flow,
      max_single: source.max_single,
      first_txn_at: source.first_txn_at,
      last_txn_at: source.last_txn_at,
      source_tool: source.source_tool,
      source_refs: source.source_refs,
      missing_fields: source.missing_fields,
      boundary: source.boundary
    }) || {};
  }).filter((item) => Object.keys(item).length);
  const claimSupport = evidencePack.map((pack, index) => {
    const source = objectOf(pack);
    return pruneEmpty({
      claim_id: `casegraph.claim.${rankKind}.${index + 1}`,
      category: "rank_fact",
      support_status: "unresolved",
      host_registry_verified: false,
      citation_status: "unresolved",
      claim_text: rankClaimText(source, rankKind),
      fact_refs: uniqueTexts([
        source.fact_id,
        source.pack_id,
        source.account_key,
        source.holder_name,
        source.counterparty_key,
        source.counterparty_account
      ], 12),
      source_refs: source.source_refs,
      allowed_report_use: "未经宿主 EvidenceReceipt registry 逐字段核验，只能写为待核聚合候选或证据缺口；不得写成案件事实、名下账户确认、最终资金去向或法律定性。"
    }) || {};
  }).filter((item) => Object.keys(item).length);
  return {
    rows: annotatedRows,
    evidence_pack: evidencePack,
    claim_support: claimSupport
  };
}

function normalizedCasegraphGates(value) {
  if (!Array.isArray(value) || value.length === 0) {
    return [{
      gate: "casegraph_source_gate",
      status: "needs_evidence",
      reason: "案件 gate 未返回；缺失或空 gate 列表不能等同于全部通过。"
    }];
  }
  return value.map((gate, index) => {
    const source = objectOf(gate);
    const gateName = text(source.gate || source.id || source.name);
    const reportedStatus = text(source.status);
    const validStatus = Boolean(gateName)
      && /^(?:passed|needs_evidence|needs_review|blocked|failed|partial|unavailable)$/u.test(reportedStatus);
    return pruneEmpty({
      ...source,
      gate: gateName || `casegraph_gate_${index + 1}`,
      status: validStatus ? reportedStatus : "needs_evidence",
      reported_status: validStatus ? undefined : reportedStatus,
      reason: validStatus
        ? source.reason
        : "gate status 缺失或非法；不能默认为 passed/ready。"
    }) || {};
  });
}

export function createCaseGraphRuntime({
  resolveCase,
  executeSkill,
  getCaseScopeMap,
  compactCaseRecord
}) {
  if (typeof resolveCase !== "function") {
    throw new TypeError("createCaseGraphRuntime requires resolveCase");
  }
  if (typeof executeSkill !== "function") {
    throw new TypeError("createCaseGraphRuntime requires executeSkill");
  }
  if (typeof getCaseScopeMap !== "function") {
    throw new TypeError("createCaseGraphRuntime requires getCaseScopeMap");
  }
  if (typeof compactCaseRecord !== "function") {
    throw new TypeError("createCaseGraphRuntime requires compactCaseRecord");
  }

  async function casegraphChildSkill(skillId, input, signal) {
    throwIfAborted(signal);
    try {
      const result = await executeSkill(skillId, input, { signal });
      throwIfAborted(signal);
      const envelope = unwrapSkillEnvelope(result);
      return {
        ok: true,
        skill_id: skillId,
        input: redactAgentPayload(input),
        envelope: redactAgentPayload(envelope),
        data: redactAgentPayload(skillEnvelopeData(result)),
        warnings: redactAgentPayload(envelopeWarnings(envelope)),
        evidence_refs: redactAgentPayload(evidenceRefsFromEnvelope(envelope, result))
      };
    } catch (error) {
      throwIfAborted(signal);
      return {
        ok: false,
        skill_id: skillId,
        input: redactAgentPayload(input),
        envelope: {},
        data: {},
        warnings: [
          {
            code: "CASEGRAPH_CHILD_TOOL_FAILED",
            severity: "warning",
            message: "上游语义技能执行失败；未发布其原始错误文本。"
          }
        ],
        evidence_refs: {}
      };
    }
  }

  async function getCaseGraph(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const caseId = resolved.case_id;
    const includeRankings = args.include_rankings !== false;
    const scopeMap = await getCaseScopeMap({
      ...args,
      case_id: caseId,
      include_tree_items: false,
      include_schema_columns: false,
      include_child_details: false,
      child_timeout_ms: clampInt(args.child_timeout_ms, 25_000, 1_000, 90_000)
    }, { signal });
    const rankPayload = {
      case_id: caseId,
      holder_name: text(args.holder_name),
      id_no: text(args.id_no),
      account_keys: arrayOf(args.account_keys).map(text).filter(Boolean),
      metric: "turnover",
      success_filter: "all",
      cash_filter: "all",
      limit: 5
    };
    const [accounts, holders, counterparties] = includeRankings
      ? await Promise.all([
          casegraphChildSkill("rank_accounts", rankPayload, signal),
          casegraphChildSkill("rank_holders", rankPayload, signal),
          casegraphChildSkill("rank_counterparties", rankPayload, signal)
        ])
      : [{ data: {}, warnings: [], evidence_refs: {} }, { data: {}, warnings: [], evidence_refs: {} }, { data: {}, warnings: [], evidence_refs: {} }];

    const gates = normalizedCasegraphGates(scopeMap.mandatory_gates);
    const blockingGates = gates.filter((item) => text(item.status) !== "passed");
    const graph = compactCaseGraphFromScopeMap(scopeMap);
    graph.mandatory_gates = gates;
    const accountRanks = buildCasegraphRankEvidence({
      rows: compactRankingRows(accounts.data.rankings || [], 5),
      rankKind: "account",
      sourceTool: "rank_accounts",
      evidenceRefs: accounts.evidence_refs,
      rankPayload,
      caseId
    });
    const holderRanks = buildCasegraphRankEvidence({
      rows: compactRankingRows(holders.data.rankings || [], 5),
      rankKind: "holder",
      sourceTool: "rank_holders",
      evidenceRefs: holders.evidence_refs,
      rankPayload,
      caseId
    });
    const counterpartyRanks = buildCasegraphRankEvidence({
      rows: compactRankingRows(counterparties.data.rankings || [], 5),
      rankKind: "counterparty",
      sourceTool: "rank_counterparties",
      evidenceRefs: counterparties.evidence_refs,
      rankPayload,
      caseId
    });
    graph.top_accounts = accountRanks.rows;
    graph.top_holders = holderRanks.rows;
    graph.top_counterparties = counterpartyRanks.rows;
    graph.case_identity = compactCaseRecord(resolved.case);
    graph.evidence_pack = [
      ...accountRanks.evidence_pack,
      ...holderRanks.evidence_pack,
      ...counterpartyRanks.evidence_pack
    ];
    graph.claim_support = [
      ...accountRanks.claim_support,
      ...holderRanks.claim_support,
      ...counterpartyRanks.claim_support
    ];
    graph.boundaries = [
      "casegraph v1 is a compact deterministic context graph; it is not a raw DuckDB export.",
      "Use rankings and gates to choose the next 1-3 tools, not to replace free investigative reasoning.",
      "Candidate duplicates/card replacement families are review leads; do not deduct totals before same-holder/person confirmation.",
      HOST_RECEIPT_BOUNDARY
    ];
    graph.host_registry_verified = false;
    graph.fact_answer_allowed = false;
    graph.minimum_roadmap = casegraphMinimumRoadmap(graph, scopeMap);

    const warnings = compactWarningList(scopeMap.warnings, accounts.warnings, holders.warnings, counterparties.warnings);
    const evidenceRefs = pruneEmpty({
      query_ids: [
        ...arrayOf(scopeMap.child_summaries?.coverage?.query_ids),
        ...arrayOf(accounts.evidence_refs.query_ids),
        ...arrayOf(holders.evidence_refs.query_ids),
        ...arrayOf(counterparties.evidence_refs.query_ids)
      ].map(text).filter(Boolean).slice(0, 16),
      audit_ref: {
        casegraph_id: `casegraph:${caseId}:${stableHash(graph)}`,
        scope_map_version: scopeMap.map_version || "v1"
      }
    }) || {};
    graph.source_refs = evidenceRefs;
    const componentEvidence = buildCasegraphComponentEvidence({
      graph,
      scopeMap,
      evidenceRefs,
      caseId
    });
    graph.evidence_pack = [
      ...arrayOf(graph.evidence_pack),
      ...arrayOf(componentEvidence.evidence_pack)
    ];
    graph.claim_support = [
      ...arrayOf(graph.claim_support),
      ...arrayOf(componentEvidence.claim_support)
    ];
    graph.evidence_boundaries = arrayOf(componentEvidence.evidence_boundaries);
    graph.component_status_counts = objectOf(componentEvidence.component_status_counts);

    return {
      tool: "get_casegraph",
      status: blockingGates.length ? "partial" : "ok",
      case_id: caseId,
      answer_card: withCasegraphAnswerCardProtocol({
        title: "Analytix casegraph v1",
        summary: blockingGates.length
          ? "口径图谱已建立，但存在需先复核的质量门。"
          : "口径图谱已建立，可进入少量高价值事实工具或自由侦查假设验证。",
        case_name: text(resolved.case?.case_name || resolved.case?.name),
        blocking_gate_count: blockingGates.length,
        next_policy: blockingGates.length
          ? "先处理 needs_review gate，再做报告级结论。"
          : "按用户意图选择 rank/profile/trace/hypothesis/quality/validation 语义事实工具；funds_investigate 只作模糊自然语言 navigator。"
      }, {
        blockingGateCount: blockingGates.length,
        recommendedNextAction: blockingGates.length ? "answer_boundary_then_resolve_quality_gate" : "answer_now_or_choose_targeted_semantic_tool",
        maxAdditionalTools: blockingGates.length ? 1 : 2,
        requiredFactsPresent: false
      }),
      key_facts: {
        coverage: graph.coverage,
        gates,
        top_accounts: graph.top_accounts,
        top_holders: graph.top_holders,
        top_counterparties: graph.top_counterparties,
        casegraph_roadmap: graph.minimum_roadmap,
        casegraph_evidence_pack: graph.evidence_pack,
        claim_support: graph.claim_support,
        allowed_next_tools: scopeMap.agent_usage?.allowed_next_tools || scopeMap.allowed_next_tools || []
      },
      casegraph: graph,
      warnings,
      evidence_refs: evidenceRefs,
      next_actions: [
        {
          reason: "若用户问 Top、最大、交易量或对手方排行，直接做对应排名核验，并保留统计范围和覆盖边界。"
        },
        {
          reason: "开放式可疑点、继续追一层或图谱题进入对应专项核验；待核线索、部分可见、字段缺失和需复核事项只能写成线索或证据缺口。"
        }
      ]
    };
  }

  return { getCaseGraph };
}
