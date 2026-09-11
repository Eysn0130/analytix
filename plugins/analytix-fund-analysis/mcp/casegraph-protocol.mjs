import { withAnswerCardProtocol } from "./answer-card-protocol.mjs";

export const CASEGRAPH_PROTOCOL_VERSION = "0.14.4-casegraph-fundgraph-protocol";

export const CASEGRAPH_REQUIRED_NODE_EDGE_NAMES = [
  "主体节点",
  "账户节点",
  "户名节点",
  "对手方节点",
  "交易边",
  "同事实族",
  "规则命中",
  "理财/资产端/现金断点",
  "缺失回单",
  "evidence pack",
  "claim support"
];

export const FUNDGRAPH_REQUIRED_FIELDS = [
  "edge_id",
  "edge_status",
  "txn_id",
  "txn_time",
  "amount",
  "direction",
  "from",
  "to",
  "source_tool",
  "boundary"
];

const CASEGRAPH_STOP_CONDITIONS = [
  "没有交易级可证实资金边时停止画图，只输出补调对象和证据缺口。",
  "线索候选、部分可见、字段缺失、需补证或证据不足的资金链路不得升级为确定性证据。",
  "质量门未过时停止正式报告，只输出普通研判或需复核草稿；继续追查可选择目标语义工具。"
];

const CASEGRAPH_ANSWER_CONSTRAINTS = [
  "casegraph 只承载确定性事实底座和最小续查路线，不替代开放式研判。",
  "候选账户、空户名、缺失对手方、现金断点和缺失回单只能作为线索或证据缺口。",
  "报告结论支持依据必须回到证据包、确定性事实、可回放计算或交付材料。",
  "插件生成的 fact_id、edge_id、query_id 或 evidence_id 不是宿主 EvidenceReceipt；没有宿主 registry membership 时，聚合、图边、claim 和 citation 只能保持 candidate/unresolved。"
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function uniqueTexts(values, limit = 24) {
  const seen = new Set();
  const output = [];
  for (const value of values.map(text).filter(Boolean)) {
    if (seen.has(value)) continue;
    seen.add(value);
    output.push(value);
    if (output.length >= limit) break;
  }
  return output;
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

function edgeAmountText(edge) {
  const amount = objectOf(edge).amount;
  const number = edgeAmountNumber(amount);
  return number === undefined ? "" : `${number.toFixed(2)} 元`;
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

function edgeAmountNumber(value) {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const source = objectOf(value);
    return edgeAmountNumber(source.yuan ?? source.amount_yuan ?? source.amount ?? source.value);
  }
  return finiteNumber(value);
}

function edgeCountNumber(value) {
  const number = finiteNumber(value);
  return number !== undefined && Number.isInteger(number) && number >= 0 ? number : undefined;
}

function nonNegativeInteger(value) {
  const number = finiteNumber(value);
  return number !== undefined && Number.isInteger(number) && number >= 0 ? number : undefined;
}

function negativeEvidenceStatus(value, fallback = "needs_evidence") {
  const status = text(value);
  return /^(?:candidate|unresolved|unsupported|refuted|rejected|missing|partial|boundary|forbidden|needs?_evidence|needs?_review)$/u.test(status)
    ? status
    : fallback;
}

function normalizeFundGraphEdge(edge) {
  const source = objectOf(edge);
  const next = { ...source };
  delete next.evidence_receipt_ids;
  delete next.citations;
  delete next.evidence_ids;
  delete next.support_status;
  next.host_registry_verified = false;
  next.citation_status = "unresolved";
  next.publication_allowed = false;
  next.fact_answer_allowed = false;
  const missingFields = new Set(arrayOf(source.missing_fields).map(text).filter(Boolean));

  if (edgeAmountNumber(source.amount) === undefined) {
    delete next.amount;
    missingFields.add("amount");
  }

  for (const field of ["txn_count", "count"]) {
    if (!Object.prototype.hasOwnProperty.call(source, field)) continue;
    const count = edgeCountNumber(source[field]);
    if (count === undefined) {
      delete next[field];
      missingFields.add(field);
    } else {
      next[field] = count;
    }
  }

  if (missingFields.size) {
    next.missing_fields = [...missingFields];
    next.edge_status = negativeEvidenceStatus(source.edge_status);
    next.boundary = text(source.boundary)
      || "金额或笔数字段缺失、空白或无效时，该链路不能升级为可绘制资金边或已支持事实。";
  }
  return next;
}

function edgeSourceRefs(edge, defaults = {}) {
  const source = objectOf(edge);
  const sourceRefs = objectOf(source.source_refs);
  const evidenceRefs = objectOf(source.evidence_refs);
  const next = pruneEmpty({
    query_ids: uniqueTexts([
      ...arrayOf(defaults.query_ids),
      ...arrayOf(sourceRefs.query_ids),
      ...arrayOf(evidenceRefs.query_ids),
      text(source.source_tool),
      "build_fund_flow_graph"
    ], 16),
    path_ids: uniqueTexts([
      ...arrayOf(defaults.path_ids),
      ...arrayOf(sourceRefs.path_ids),
      ...arrayOf(evidenceRefs.path_ids),
      text(source.edge_id)
    ], 8),
    report_ids: uniqueTexts([
      ...arrayOf(defaults.report_ids),
      ...arrayOf(sourceRefs.report_ids),
      ...arrayOf(evidenceRefs.report_ids)
    ], 8),
    audit_ref: {
      ...objectOf(defaults.audit_ref),
      ...objectOf(sourceRefs.audit_ref),
      ...objectOf(evidenceRefs.audit_ref),
      host_registry_verified: false
    },
    detail_ref: {
      ...objectOf(defaults.detail_ref),
      ...objectOf(sourceRefs.detail_ref),
      ...objectOf(evidenceRefs.detail_ref)
    },
    artifact_id: text(defaults.artifact_id || sourceRefs.artifact_id || evidenceRefs.artifact_id || source.artifact_id)
  });
  return next || {};
}

function evidencePackFromEdge(edge, index) {
  const source = objectOf(edge);
  const refs = edgeSourceRefs(source, {
    report_ids: [`evidence_pack.edge.${index + 1}`]
  });
  return pruneEmpty({
    pack_id: `evidence_pack.edge.${index + 1}`,
    evidence_type: "candidate_transaction_edge",
    support_status: "candidate",
    host_registry_verified: false,
    citation_status: "unresolved",
    edge_id: source.edge_id,
    txn_id: source.txn_id,
    txn_time: source.txn_time,
    amount: source.amount,
    direction: source.direction,
    from: source.from,
    from_label: source.from_label,
    to: source.to,
    to_label: source.to_label,
    source_tool: source.source_tool,
    source_refs: refs,
    mermaid_allowed: true,
    publication_allowed: false,
    boundary: source.boundary || "该交易边仅可用于未发布预览；宿主 registry 未核验同案、同轮、同快照 EvidenceReceipt 前，不得发布或推断下游箭头。"
  }) || {};
}

function claimSupportFromEdge(edge, index) {
  const source = objectOf(edge);
  const refs = edgeSourceRefs(source, {
    report_ids: [`claim_support.edge.${index + 1}`]
  });
  return pruneEmpty({
    claim_id: `claim_support.edge.${index + 1}`,
    category: "flow_graph_edge",
    support_status: "unresolved",
    host_registry_verified: false,
    citation_status: "unresolved",
    claim_text: `${text(source.from_label || source.from)} -> ${text(source.to_label || source.to)} ${edgeAmountText(source)}`.trim(),
    fact_refs: uniqueTexts([source.edge_id, source.txn_id], 8),
    source_refs: refs,
    allowed_report_use: "未经宿主 EvidenceReceipt registry 逐边核验，不得写入正式报告；只能保留为待核交易候选，且不得扩写为最终资金归属、现金去向或法律定性。"
  }) || {};
}

function evidenceBoundaryFromEdge(edge, index) {
  const source = objectOf(edge);
  return pruneEmpty({
    boundary_id: `evidence_boundary.edge.${index + 1}`,
    edge_id: source.edge_id,
    support_status: negativeEvidenceStatus(source.edge_status),
    missing_fields: arrayOf(source.missing_fields).map(text).filter(Boolean),
    boundary: source.boundary || "关键字段缺失时，该链路不能升级为资金流向图箭头或已复核报告结论。",
    source_refs: edgeSourceRefs(source, {
      report_ids: [`evidence_boundary.edge.${index + 1}`]
    }),
    next_review_action: "补齐交易号、交易时间、金额、方向、两端主体和来源凭证后，再升级为交易级可证实资金边。"
  }) || {};
}

function edgeTokens(edge) {
  const source = objectOf(edge);
  return uniqueTexts([
    source.edge_id,
    source.txn_id,
    ...arrayOf(source.fact_refs),
    ...arrayOf(objectOf(source.source_refs).evidence_ids),
    ...arrayOf(objectOf(source.source_refs).path_ids)
  ], 24);
}

function supportEntryStatusAllowsEdge(entry) {
  const source = objectOf(entry);
  const status = text(source.support_status || source.edge_status);
  return !status || status === "supported";
}

function matchingSupportedEdge(entry, supportedEdges) {
  if (!supportEntryStatusAllowsEdge(entry)) return null;
  const entryTokenSet = new Set(edgeTokens(entry));
  if (!entryTokenSet.size) return null;
  return supportedEdges.find((edge) => edgeTokens(edge).some((token) => entryTokenSet.has(token))) || null;
}

function entryKey(entry, fallback) {
  const source = objectOf(entry);
  const factRefs = arrayOf(source.fact_refs).map(text).filter(Boolean);
  const edgeFactRef = factRefs.find((ref) => /^edge\./u.test(ref));
  const txnFactRef = factRefs.find((ref) => /^txn\./u.test(ref));
  return text(source.edge_id || source.txn_id || edgeFactRef || txnFactRef || source.pack_id || source.claim_id || source.boundary_id) || fallback;
}

function mergeEntriesByKey(entries, prefix) {
  const seen = new Set();
  const output = [];
  arrayOf(entries).forEach((entry, index) => {
    const source = objectOf(entry);
    if (!Object.keys(source).length) return;
    const key = entryKey(source, `${prefix}.${index + 1}`);
    if (seen.has(key)) return;
    seen.add(key);
    output.push(source);
  });
  return output;
}

function supportedEvidencePackEntry(entry, supportedEdges, index) {
  const edge = matchingSupportedEdge(entry, supportedEdges);
  if (!edge) return null;
  const source = objectOf(entry);
  const safe = evidencePackFromEdge(edge, index);
  return pruneEmpty({
    ...safe,
    pack_id: text(source.pack_id) || safe.pack_id,
    mermaid_allowed: true,
    publication_allowed: false,
    support_status: "candidate",
    host_registry_verified: false,
    citation_status: "unresolved",
    boundary: text(source.boundary) || safe.boundary
  }) || null;
}

function supportedClaimEntry(entry, supportedEdges, index) {
  const edge = matchingSupportedEdge(entry, supportedEdges);
  if (!edge) return null;
  const source = objectOf(entry);
  const safe = claimSupportFromEdge(edge, index);
  return pruneEmpty({
    ...safe,
    claim_id: text(source.claim_id) || safe.claim_id,
    support_status: "unresolved",
    host_registry_verified: false,
    citation_status: "unresolved",
    allowed_report_use: text(source.allowed_report_use) || safe.allowed_report_use
  }) || null;
}

function unsupportedSupportBoundary(entry, index, category) {
  const source = objectOf(entry);
  return pruneEmpty({
    boundary_id: `evidence_boundary.${category}.${index + 1}`,
    edge_id: source.edge_id,
    claim_id: source.claim_id,
    support_status: negativeEvidenceStatus(source.support_status || source.edge_status, "needs_review"),
    missing_fields: uniqueTexts([
      ...arrayOf(source.missing_fields),
      "supported_transaction_edge"
    ], 8),
    boundary: `${category} 未匹配到本轮返回的交易级可证实资金边；只能作为核验意见，不作为资金流向图或报告结论支持依据。`,
    source_refs: edgeSourceRefs(source, {
      report_ids: [`evidence_boundary.${category}.${index + 1}`]
    }),
    next_review_action: "先补资金来源去向核验或交易级可证实资金边，再决定是否画图或写入报告研判结论。"
  }) || {};
}

function componentSupportStatus(status) {
  const value = text(status);
  if (/^(?:ready|candidate|supported|verified|passed|complete|ok)/u.test(value)) return "candidate";
  if (/^roadmap_ready/u.test(value)) return "lead";
  if (/^needs/u.test(value)) return "needs_evidence";
  if (/need|required|missing|gap/u.test(value)) return "needs_evidence";
  if (/partial|review/u.test(value)) return "needs_review";
  return "unresolved";
}

function componentSourceRefs(component, index, refs = {}, options = {}) {
  const source = objectOf(component);
  const parent = objectOf(refs);
  const settings = objectOf(options);
  const packId = `casegraph.component.${index + 1}`;
  return pruneEmpty({
    query_ids: uniqueTexts([
      ...arrayOf(parent.query_ids),
      "get_casegraph",
      "casegraph_minimum_roadmap"
    ], 16),
    path_ids: uniqueTexts([
      ...arrayOf(parent.path_ids),
      packId
    ], 12),
    report_ids: uniqueTexts([
      ...arrayOf(parent.report_ids),
      `casegraph.component.${index + 1}`
    ], 12),
    audit_ref: {
      ...objectOf(parent.audit_ref),
      case_id: text(settings.caseId || settings.case_id),
      component: text(source.name),
      component_status: text(source.status),
      protocol_version: CASEGRAPH_PROTOCOL_VERSION,
      host_registry_verified: false
    },
    detail_ref: objectOf(parent.detail_ref),
    artifact_id: text(parent.artifact_id)
  }) || {};
}

function componentNextReviewAction(component) {
  const source = objectOf(component);
  const name = text(source.name);
  if (/交易边/u.test(name)) return "补齐逐笔交易号、时间、金额、方向和端点后，才能升级为交易级可证实资金边或资金流向图箭头。";
  if (/理财|资产|现金/u.test(name)) return "补调理财认申购/赎回、现金取现凭证和后续入账明细；未闭合前只能写资产转换线索或现金断点。";
  if (/缺失回单/u.test(name)) return "补齐回单、凭证、来源文件或对手方身份后，再升级报告事实。";
  if (/claim support/u.test(name)) return "报告级文本先走 validate_report_claims，把 unsupported/corrected/verified 分类后再写入。";
  if (/规则命中/u.test(name)) return "先复核 source audit、data quality、account identity、counterparty gap 和 report gate，再输出报告级结论。";
  return "补齐 required_facts 与 source_refs 后，再把该组件升级为确定性事实。";
}

function componentAllowedReportUse(component, supportStatus) {
  const source = objectOf(component);
  if (supportStatus === "candidate") {
    return `${text(source.name)} 只能写为待核组件候选；宿主 EvidenceReceipt registry 未核验前，不得写成案件事实、最终资金去向、账户归属确认或法律定性。`;
  }
  if (supportStatus === "lead") {
    return `${text(source.name)} 只能写为路线图/续查方向；不得写成已核实事实。`;
  }
  return `${text(source.name)} 只能写为证据缺口或 next_review_action。`;
}

export function buildCasegraphComponentEvidence({
  graph = {},
  scopeMap = {},
  evidenceRefs = {},
  caseId = ""
} = {}) {
  const source = objectOf(graph);
  const roadmap = objectOf(source.minimum_roadmap).minimum_nodes_and_edges
    ? objectOf(source.minimum_roadmap)
    : casegraphMinimumRoadmap(source, scopeMap);
  const components = arrayOf(roadmap.minimum_nodes_and_edges);
  const evidencePack = [];
  const claimSupport = [];
  const evidenceBoundaries = [];
  const statusCounts = {};

  components.forEach((component, index) => {
    const item = objectOf(component);
    const supportStatus = componentSupportStatus(item.status);
    statusCounts[supportStatus] = Number(statusCounts[supportStatus] || 0) + 1;
    const sourceRefs = componentSourceRefs(item, index, evidenceRefs, { caseId });
    const pack = pruneEmpty({
      pack_id: `casegraph.component.${index + 1}`,
      fact_id: `casegraph.component.fact.${index + 1}`,
      evidence_type: "casegraph_component_status",
      component: item.name,
      component_status: item.status,
      support_status: supportStatus,
      host_registry_verified: false,
      citation_status: "unresolved",
      required_facts: arrayOf(item.required_facts).map(text).filter(Boolean),
      source_tool: "get_casegraph",
      source_refs: sourceRefs,
      boundary: item.boundary
    }) || {};
    if (Object.keys(pack).length) evidencePack.push(pack);
    const claim = pruneEmpty({
      claim_id: `casegraph.component.claim.${index + 1}`,
      category: "casegraph_component_boundary",
      support_status: supportStatus === "candidate" ? "unresolved" : supportStatus,
      host_registry_verified: false,
      citation_status: "unresolved",
      claim_text: `${text(item.name)}：${text(item.status)}；${text(item.boundary)}`.trim(),
      fact_refs: uniqueTexts([pack.fact_id, pack.pack_id, item.name, item.status], 8),
      source_refs: sourceRefs,
      allowed_report_use: componentAllowedReportUse(item, supportStatus)
    }) || {};
    if (Object.keys(claim).length) claimSupport.push(claim);
    if (supportStatus !== "supported") {
      const boundary = pruneEmpty({
        boundary_id: `casegraph.component.boundary.${index + 1}`,
        component: item.name,
        support_status: supportStatus,
        missing_fields: arrayOf(item.required_facts).map(text).filter(Boolean),
        boundary: item.boundary,
        source_refs: sourceRefs,
        next_review_action: componentNextReviewAction(item)
      }) || {};
      if (Object.keys(boundary).length) evidenceBoundaries.push(boundary);
    }
  });

  return {
    evidence_pack: evidencePack,
    claim_support: claimSupport,
    evidence_boundaries: evidenceBoundaries,
    component_status_counts: statusCounts
  };
}

function mergedGraphSourceRefs(edges) {
  const rows = arrayOf(edges).map(objectOf);
  return pruneEmpty({
    query_ids: uniqueTexts([
      ...rows.flatMap((edge) => arrayOf(objectOf(edge.source_refs).query_ids)),
      ...rows.map((edge) => text(edge.source_tool)),
      "build_fund_flow_graph"
    ], 16),
    path_ids: uniqueTexts([
      ...rows.flatMap((edge) => arrayOf(objectOf(edge.source_refs).path_ids)),
      ...rows.map((edge) => text(edge.edge_id))
    ], 16),
    report_ids: ["fund_flow_graph"]
  }) || {};
}

export function casegraphMinimumRoadmap(graph = {}, scopeMap = {}) {
  const source = objectOf(graph);
  const nodes = arrayOf(source.nodes);
  const edges = arrayOf(source.edges);
  const gates = arrayOf(source.mandatory_gates || source.gates || scopeMap.mandatory_gates);
  const topAccounts = arrayOf(source.top_accounts);
  const topHolders = arrayOf(source.top_holders);
  const topCounterparties = arrayOf(source.top_counterparties);
  const hasValidGate = gates.some((gate) => {
    const item = objectOf(gate);
    return Boolean(text(item.gate || item.id || item.name))
      && /^(?:passed|needs_evidence|needs_review|blocked|failed|partial|unavailable)$/u.test(text(item.status));
  });
  const nodeSignal = (node) => text(objectOf(node).id || objectOf(node).kind || objectOf(node).label);
  const edgeSignal = (edge) => text(objectOf(edge).kind || objectOf(edge).label || objectOf(edge).relation);
  const hasQualityNode = nodes.some((node) => /quality|source_audit|duplicate|report/u.test(nodeSignal(node)));
  const hasDuplicateNode = nodes.some((node) => /duplicate|同事实|换卡/u.test(nodeSignal(node)));
  const hasReportNode = nodes.some((node) => /report|claim|报告/u.test(nodeSignal(node)));
  const hasTransactionEdge = edges.some((edge) => /normalized|analysis|validated|transaction|交易|资金/u.test(edgeSignal(edge)));

  return pruneEmpty({
    protocol_version: CASEGRAPH_PROTOCOL_VERSION,
    purpose: "casegraph is a compact fact substrate for rank facts, trace facts, and claim facts; it is not a raw graph-wandering tool.",
    minimum_nodes_and_edges: [
      {
        name: "主体节点",
        status: topHolders.length ? "candidate_from_rank_facts" : "needs_rank_or_scope_fact",
        required_facts: ["holder_name", "id_no_when_available", "name_only_group_is_diagnostic"],
        boundary: "姓名组只能辅助研判；没有证件或账户归集时不得写成报告级同一主体。"
      },
      {
        name: "账户节点",
        status: topAccounts.length ? "candidate_from_rank_facts" : "needs_account_rank_fact",
        required_facts: ["account_key", "holder", "direct_or_candidate_status", "quality_flags"],
        boundary: "候选账户只能是线索，不能写成名下账户。"
      },
      {
        name: "户名节点",
        status: topHolders.length ? "candidate_from_holder_facts" : "needs_holder_rank_fact",
        required_facts: ["account_open_name", "holder_group", "id_no_when_available", "name_only_boundary"],
        boundary: "户名归组是研判线索；无证件、无开户或无交易承接时不得等同为同一主体。"
      },
      {
        name: "对手方节点",
        status: topCounterparties.length ? "candidate_from_rank_facts" : "needs_counterparty_rank_fact",
        required_facts: ["counterparty_name_or_account", "blank_or_missing_class", "person_unit_account_only_class", "match_confidence"],
        boundary: "空户名、账号型终点和未匹配终点不得写成最终流向。"
      },
      {
        name: "交易边",
        status: hasTransactionEdge ? "candidate_scope_edges_trace_edges_need_flow_tool" : "needs_supported_trace_fact",
        required_facts: ["direction", "amount", "time", "source_account", "counterparty", "edge_status", "source_scope"],
        boundary: "只有交易级可证实资金边才能画箭头；未返回逐笔边时不得画资金流向图。"
      },
      {
        name: "同事实族",
        status: hasDuplicateNode ? "candidate_as_review_risk" : "needs_duplicate_family_review",
        required_facts: ["duplicate_txn_id", "same_holder_candidate", "card_replacement_candidate", "natural_within_account_duplicate"],
        boundary: "同事实/换卡候选只能防止重复放大，未经确认不得扣减总额。"
      },
      {
        name: "规则命中",
        status: hasValidGate || hasQualityNode ? "candidate_from_gates" : "needs_quality_gate",
        required_facts: ["data_quality_gate", "account_identity_gate", "counterparty_gap_gate", "report_gate", "claim_review_gate"],
        boundary: "任一质量门 needs_review 时，报告级措辞必须降级或阻断。"
      },
      {
        name: "理财/资产端/现金断点",
        status: "roadmap_ready_probe_required",
        required_facts: ["wealth_management_subscription", "redemption", "loan_or_repayment", "cash_break", "missing_counterparty_break"],
        boundary: "理财、转存、贷款、还款和现金断点是资产转换或续查线索，不等于现金去向。"
      },
      {
        name: "缺失回单",
        status: "roadmap_ready_source_gap_required",
        required_facts: ["missing_receipt", "missing_voucher", "source_audit_gap", "counterparty_identity_gap"],
        boundary: "缺失回单和来源缺口只能写成补证方向，不得补写成已核实交易目的或最终归属。"
      },
      {
        name: "evidence pack",
        status: "roadmap_ready_evidence_pack_required",
        required_facts: ["bounded_row_sample", "deterministic_aggregate", "supported_flow_edge", "source_boundary", "missing_source_boundary"],
        boundary: "evidence pack / 证据包必须来自确定性聚合、受限样本或支持边，不能来自模型自造叙事。"
      },
      {
        name: "claim support",
        status: hasReportNode ? "candidate_as_report_gate" : "needs_validate_report_claims_for_report_tasks",
        required_facts: ["verified_claim", "corrected_claim", "unsupported_claim", "forbidden_phrasing", "next_review_action"],
        boundary: "verified/corrected/unsupported/boundary/forbidden 必须来自确定性事实族，不得只复述报告原文。"
      }
    ],
    fact_packs: [
      "rank facts: top_accounts、top_holders、top_counterparties 必须携带 metric、scope 和排序边界。",
      "资金追踪事实：一跳/下一跳必须携带交易级可证实资金边、停止条件和未支持边界。",
      "研判结论事实：已核实、需纠正、未支持、边界和禁用表述必须落回确定性事实族和证据包。"
    ],
    release_gates: [
      "普通范围、排行、追踪和结论支持问题应由案件关系图加目标事实核验给出最小路线和下一步边界。",
      "报告任务必须能说明研判结论为已核实、需纠正、未支持或禁用表述的原因，且不得绘制证据不足的资金边。",
      "开放式侦查只能产出 candidate、lead、missing、downgraded 或 forbidden 状态的假设。",
      "任何 candidate/supported 业务标签都不能替代宿主 EvidenceReceipt registry membership；正式事实发布仍由宿主 Final Evidence Gate 决定。"
    ],
    stop_conditions: CASEGRAPH_STOP_CONDITIONS
  }) || {};
}

export function withCasegraphAnswerCardProtocol(card, options = {}) {
  const source = objectOf(card);
  const optionGateCount = Object.prototype.hasOwnProperty.call(options, "blockingGateCount")
    ? nonNegativeInteger(options.blockingGateCount)
    : undefined;
  const sourceGateCount = Object.prototype.hasOwnProperty.call(source, "blocking_gate_count")
    ? nonNegativeInteger(source.blocking_gate_count)
    : undefined;
  const blockingGateCount = optionGateCount ?? sourceGateCount;
  const gateCountKnown = blockingGateCount !== undefined;
  const merged = {
    ...source,
    host_registry_verified: false,
    publication_ready: false,
    casegraph_protocol: {
      version: CASEGRAPH_PROTOCOL_VERSION,
      required_nodes_and_edges: CASEGRAPH_REQUIRED_NODE_EDGE_NAMES,
      evidence_pack_required: true,
      claim_support_required_for_reports: true,
      host_evidence_receipt_required: true,
      host_registry_verified: false,
      mermaid_rule: "只有宿主 registry 已核验 EvidenceReceipt 的交易级资金边才能成为可发布资金流向图箭头。"
    },
    answer_constraints: uniqueTexts([
      ...arrayOf(source.answer_constraints),
      ...CASEGRAPH_ANSWER_CONSTRAINTS
    ], 16),
    stop_conditions: uniqueTexts([
      ...arrayOf(source.stop_conditions),
      ...CASEGRAPH_STOP_CONDITIONS
    ], 16)
  };
  delete merged.evidence_receipt_ids;
  delete merged.citations;
  if (gateCountKnown) merged.blocking_gate_count = blockingGateCount;
  else delete merged.blocking_gate_count;
  return withAnswerCardProtocol(merged, {
    recommendedNextAction: text(options.recommendedNextAction) || (
      !gateCountKnown || blockingGateCount > 0
        ? "answer_boundary_then_resolve_quality_gate"
        : "answer_now_or_choose_targeted_semantic_tool"
    ),
    maxAdditionalTools: Object.prototype.hasOwnProperty.call(options, "maxAdditionalTools")
      ? Number(options.maxAdditionalTools)
      : (!gateCountKnown || blockingGateCount > 0 ? 1 : 0),
    requiredFactsPresent: false,
    unsupportedFlowsPresent: Object.prototype.hasOwnProperty.call(options, "unsupportedFlowsPresent")
      ? Boolean(options.unsupportedFlowsPresent)
      : false
  });
}

export function withFundGraphProtocol(flowGraph, options = {}) {
  const source = objectOf(flowGraph);
  const edges = arrayOf(source.edges).map((edge) => {
    const next = normalizeFundGraphEdge(edge);
    const refs = edgeSourceRefs(next);
    return Object.keys(refs).length ? { ...next, source_refs: refs } : next;
  });
  const supportedEdges = edges.filter((edge) => text(objectOf(edge).edge_status) === "supported");
  const incompleteEdges = edges.filter((edge) => text(objectOf(edge).edge_status) !== "supported");
  const edgeStatusCounts = edges.reduce((counts, edge) => {
    const status = text(objectOf(edge).edge_status) || "unknown";
    counts[status] = Number(counts[status] || 0) + 1;
    return counts;
  }, {});
  const providedEvidencePack = arrayOf(source.evidence_pack).map(objectOf).filter((entry) => Object.keys(entry).length);
  const providedClaimSupport = arrayOf(source.claim_support).map(objectOf).filter((entry) => Object.keys(entry).length);
  const acceptedEvidencePack = providedEvidencePack
    .map((entry, index) => supportedEvidencePackEntry(entry, supportedEdges, index))
    .filter((item) => item && Object.keys(item).length);
  const acceptedClaimSupport = providedClaimSupport
    .map((entry, index) => supportedClaimEntry(entry, supportedEdges, index))
    .filter((item) => item && Object.keys(item).length);
  const rejectedEvidencePack = providedEvidencePack
    .filter((entry) => !supportedEvidencePackEntry(entry, supportedEdges, 0))
    .map((entry, index) => unsupportedSupportBoundary(entry, index, "evidence_pack"));
  const rejectedClaimSupport = providedClaimSupport
    .filter((entry) => !supportedClaimEntry(entry, supportedEdges, 0))
    .map((entry, index) => unsupportedSupportBoundary(entry, index, "claim_support"));
  const evidencePack = mergeEntriesByKey([
    ...acceptedEvidencePack,
    ...supportedEdges.map(evidencePackFromEdge).filter((item) => Object.keys(item).length)
  ], "evidence_pack");
  const claimSupport = mergeEntriesByKey([
    ...acceptedClaimSupport,
    ...supportedEdges.map(claimSupportFromEdge).filter((item) => Object.keys(item).length)
  ], "claim_support");
  const evidenceBoundaries = mergeEntriesByKey([
    ...arrayOf(source.evidence_boundaries).map(objectOf),
    ...incompleteEdges.map(evidenceBoundaryFromEdge).filter((item) => Object.keys(item).length),
    ...rejectedEvidencePack,
    ...rejectedClaimSupport
  ], "evidence_boundary");
  const providedGraphSourceRefs = objectOf(source.source_refs);
  const sanitizedGraphSourceRefs = Object.keys(providedGraphSourceRefs).length
    ? edgeSourceRefs({}, providedGraphSourceRefs)
    : {};
  const graphSourceRefs = Object.keys(sanitizedGraphSourceRefs).length
    ? sanitizedGraphSourceRefs
    : mergedGraphSourceRefs(edges);
  return {
    ...source,
    edges,
    source_refs: graphSourceRefs,
    graph_protocol: {
      version: CASEGRAPH_PROTOCOL_VERSION,
      required_edge_fields: FUNDGRAPH_REQUIRED_FIELDS,
      evidence_pack_fields: ["pack_id", "evidence_type", "support_status", "edge_id", "txn_id", "amount", "source_refs"],
      claim_support_fields: ["claim_id", "category", "support_status", "claim_text", "fact_refs", "source_refs"],
      supported_edge_only: true,
      host_evidence_receipt_required: true,
      host_registry_verified: false,
      mermaid_rule: "资金流向图只绘制交易级可证实资金边。",
      unsupported_flow_rule: "字段缺失、聚合对手方、现金断点、缺回单和未匹配端点属于补证缺口，不画成图谱箭头。",
      evidence_pack_rule: "每个金额、笔数、账户、户名、对手方和资金边都必须能追溯到确定性候选；只有宿主 registry membership 可将其升级为 EvidenceReceipt 支持事实。",
      claim_support_rule: "没有宿主 EvidenceReceipt registry membership 时，交易边或确定性聚合只能产生 unresolved claim，不得签发 supported claim/citation。"
    },
    node_count: arrayOf(source.nodes).length,
    edge_count: edges.length,
    supported_edge_count: supportedEdges.length,
    incomplete_edge_count: incompleteEdges.length,
    edge_status_counts: Object.keys(edgeStatusCounts).length ? edgeStatusCounts : undefined,
    host_registry_verified: false,
    publishable_edge_count: 0,
    fact_answer_allowed: false,
    drawable_edges: supportedEdges,
    review_edges: incompleteEdges,
    mermaid_edges: supportedEdges,
    evidence_pack: evidencePack,
    evidence_boundaries: evidenceBoundaries,
    claim_support: claimSupport,
    next_review_actions: uniqueTexts([
      ...arrayOf(source.next_review_actions),
      ...evidenceBoundaries.map((item) => objectOf(item).next_review_action)
    ], 12),
    boundaries: uniqueTexts([
      ...arrayOf(source.boundaries),
      "只绘制本轮返回的资金边，不凭叙述记忆补箭头。",
      "只有交易级可证实资金边才能成为资金流向图箭头；证据不足的链路仍是边界或补证方向。",
      "缺回单、现金断点和聚合对手方是证据缺口，不是最终去向。"
    ], 8),
    mermaid_hint: text(source.mermaid_hint) || "只能根据交易级可证实资金边绘制资金流向图。",
    protocol_options: pruneEmpty({
      requested_intent: options.intent,
      evidence_pack_required: true,
      claim_support_required_for_reports: true
    })
  };
}
