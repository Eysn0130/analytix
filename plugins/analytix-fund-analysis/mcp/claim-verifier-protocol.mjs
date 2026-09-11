import { withAnswerCardProtocol } from "./answer-card-protocol.mjs";
import { validateInvestigationAnswerContract } from "./investigation-answer-contract.mjs";
import { intOrUndefined } from "./runtime-normalizers.mjs";

export const CLAIM_VERIFIER_VERSION = "0.15.6-claim-verifier-protocol";

export const CLAIM_REVIEW_REQUIRED_FIELDS = [
  "verified_claims",
  "corrected_claims",
  "unsupported_claims",
  "unsupported_flows",
  "missing_source_boundaries",
  "forbidden_phrasings",
  "next_review_actions"
];

export const CLAIM_REVIEW_INTERNAL_TRACE_FIELDS = [
  "claim_support_index",
  "source_refs",
  "fact_refs",
  "support_query_names"
];

const DEFAULT_ANSWER_SECTIONS = [
  "既有报告研判结论复核材料",
  "已复核事实",
  "需纠正",
  "未支持/不能确认",
  "来源边界",
  "禁用表述",
  "下一步复核"
];

const DEFAULT_STOP_CONDITIONS = [
  "正式报告暂不出具；需先完成报告研判结论复核和来源边界复核。",
  "本轮研判结论复核卡已经覆盖纠正口径、降级边界和补证动作时，应先作答，不再追加表结构、计算、排行、开放式深挖或全案分析。",
  "最终回答不得写复核已通过或可直接出具正式报告。",
  "未支持/不能确认小节必须写：没有确定性资金边、不能确认、证据不足、不得画资金流向图。",
  "没有确定性交易边的链路不得画资金流向图、流程图或箭头。"
];

const DEFAULT_UNSUPPORTED_FLOWS = [
  "没有确定性交易边的资金链路片段不能画成确定性资金流向图链路。",
  "未返回确定性交易边时，只能写线索、需复核或补调方向。"
];

const DEFAULT_MISSING_SOURCE_BOUNDARIES = [
  "报告中的聚合片段需回到确定性事实、可回放计算或交付附件，不能模型侧补成资金闭环。",
  "来源审计、数据质量、必需核验材料或报告事实结论复核任一未逐项通过时，只能输出待复核草稿和补证动作。"
];

const DEFAULT_FORBIDDEN_PHRASES = [
  "未触发报告阻断",
  "门禁通过",
  "可直接出具正式报告",
  "已查明涉黑资金",
  "确定违法所得",
  "实际控制/代持已坐实",
  "最终资金归属已确认"
];

const DEFAULT_NEXT_REVIEW_ACTIONS = [
  "正式报告级文本形成后，先做报告事实结论复核；同一报告文本不要重复复核，普通研判、继续追一层、改口径、新对象或新假设仍走对应语义事实工具。",
  "对未支持资金流先补确定性交易边，再决定是否画图。",
  "全案报告需逐项复核来源审计、数据质量、必需核验材料和报告事实结论复核；缺一项只能输出需复核草稿。"
];

const DEFAULT_ANSWER_CONSTRAINTS = [
  "报告复核回答第一行使用“既有报告研判结论复核材料”，并按每条研判结论给出已核验、需纠正、当前证据不足或需补证。",
  "报告结论必须分为已复核、需纠正、未获支持和需降级线索。",
  "每个金额、笔数、主体和链路必须回到确定性工具事实；证据不足时写需补调。",
  "报告级问题拿到本卡后先输出复核结论、纠正口径和补证动作；同一报告文本不要重复复核，新的追查目标仍可进入对应语义事实工具。"
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

const GUARDED_RISK_PHRASE_PATTERN = /(?:不能|不得|不可|不应|未|没有|尚未|需核实|需复核|待核|线索|风险|并非|不是|而非|不写作|不写成|禁止)/u;

function claimEntryText(item) {
  if (item && typeof item === "object") {
    const source = objectOf(item);
    return text(source.text || source.claim || source.statement || source.summary || source.reason || source.id || source.claim_id);
  }
  return text(item);
}

function claimTexts(value) {
  return arrayOf(value).map(claimEntryText).filter(Boolean);
}

function claimRows(value) {
  return arrayOf(value).map((item, index) => {
    const source = objectOf(item);
    return {
      source,
      id: text(source.claim_id || source.claimId || source.id || source.key) || `claim.${index + 1}`,
      text: claimEntryText(item)
    };
  }).filter((item) => text(item.text));
}

const CLAIM_AUTHORITY_ASSERTION_FIELDS = [
  "write_blocked",
  "report_gate_status",
  "fact_answer_allowed",
  "safeToAnswer",
  "safe_to_answer_current_task",
  "host_registry_verified",
  "registry_membership_verified",
  "evidence_registry_verified",
  "publication_receipt_verified",
  "publication_receipt",
  "publicationReceipt",
  "publication_receipt_id",
  "publicationReceiptId",
  "publication_receipts",
  "publicationReceipts",
  "publication_authorization",
  "publication_gate"
];

function stripClaimAuthorityAssertions(value) {
  const output = { ...objectOf(value) };
  for (const key of CLAIM_AUTHORITY_ASSERTION_FIELDS) delete output[key];
  const meta = { ...objectOf(output._meta) };
  for (const key of CLAIM_AUTHORITY_ASSERTION_FIELDS) delete meta[key];
  if (Object.keys(meta).length) output._meta = meta;
  else delete output._meta;
  return output;
}

function candidateClaimEntry(item, origin, index) {
  const source = objectOf(item);
  const claimText = claimEntryText(item);
  if (!claimText) return undefined;
  return pruneEmpty({
    ...stripClaimAuthorityAssertions(source),
    claim_id: text(source.claim_id || source.claimId || source.id || source.key) || `${origin}.${index + 1}`,
    text: claimText,
    asserted_support_status: text(source.asserted_support_status || source.support_status || source.status),
    status: "candidate",
    support_status: "candidate",
    candidate_origin: text(source.candidate_origin) || origin
  });
}

function candidateClaimsFrom(source = {}, claimReview = {}) {
  const groups = [
    ["backend_candidate", source.candidate_claims],
    ["backend_verified_assertion", source.verified_claims],
    ["backend_supported_assertion", source.supported_claims],
    ["backend_claim_review_supported_assertion", claimReview.supported],
    ["backend_claim_review_candidate", claimReview.candidate_supported]
  ];
  const output = [];
  const seen = new Set();
  for (const [origin, values] of groups) {
    for (const [index, item] of arrayOf(values).entries()) {
      const candidate = candidateClaimEntry(item, origin, index);
      if (!candidate) continue;
      const key = `${text(candidate.claim_id)}\u0000${text(candidate.text)}`;
      if (seen.has(key)) continue;
      seen.add(key);
      output.push(candidate);
      if (output.length >= 24) return output;
    }
  }
  return output;
}

function p0PublicationGate({ candidateClaims = [], combinedRisk = {} } = {}) {
  const riskPresent = [
    ...arrayOf(combinedRisk.unsupported_claims),
    ...arrayOf(combinedRisk.unsupported_flows),
    ...arrayOf(combinedRisk.missing_source_boundaries),
    ...arrayOf(combinedRisk.forbidden_phrasings)
  ].length > 0;
  return {
    contract_version: "ClaimPublicationGateP0",
    status: "blocked",
    write_blocked: true,
    report_gate_status: "publication_receipt_required",
    host_registry_verified: false,
    publication_receipt_verified: false,
    verified_claim_count: 0,
    candidate_claim_count: candidateClaims.length,
    blockers: uniqueTexts([
      candidateClaims.length ? "backend_claims_are_untrusted_candidates" : "no_host_verified_claims",
      riskPresent ? "claim_review_risk_or_source_boundary_present" : "claim_review_support_not_established",
      "host_registry_membership_unavailable",
      "publication_receipt_required"
    ], 8)
  };
}

function hasClaimSourceAnchor(item = {}) {
  void item;
  // Non-empty IDs are claims about evidence, not proof of host registry
  // membership. This plugin deliberately cannot promote them to anchors.
  return false;
}

function textWindows(value) {
  const body = text(value);
  if (!body) return [];
  const windows = body
    .split(/(?<=[。！？!?；;])|\n+/u)
    .map(text)
    .filter(Boolean);
  return windows.length ? windows : [body];
}

function hasUnguardedRisk(value, pattern, guardPattern = GUARDED_RISK_PHRASE_PATTERN) {
  return textWindows(value).some((line) => pattern.test(line) && !guardPattern.test(line));
}

const TABLE_AFTER_ANALYSIS_PATTERN = /(?:研判|异常|证明价值|证明|案件意义|资金意义|现有材料|暂不能认定|不能证明|证据缺口|补证|调取|核验意见|资金来源|资金去向|账户角色)/u;
const MARKDOWN_TABLE_LINE_PATTERN = /^\s*\|.+\|\s*$/u;
const MARKDOWN_TABLE_SEPARATOR_PATTERN = /^\s*\|?\s*:?-{3,}:?\s*(?:\|\s*:?-{3,}:?\s*)+\|?\s*$/u;
const MATERIAL_SECTION_HEADING_PATTERN = /^\s*(?:#{1,6}\s+|[一二三四五六七八九十]+[、.．]|（[一二三四五六七八九十]+）|\d+[.、．])/u;

function hasMarkdownTableWithoutAnalysis(value) {
  const lines = text(value).split(/\r?\n/u);
  let insideFence = false;
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index] || "";
    if (/^\s*```/u.test(line)) {
      insideFence = !insideFence;
      continue;
    }
    if (insideFence) continue;
    if (!MARKDOWN_TABLE_LINE_PATTERN.test(line)) continue;
    if (!MARKDOWN_TABLE_SEPARATOR_PATTERN.test(lines[index + 1] || "")) continue;
    let cursor = index + 2;
    while (cursor < lines.length && MARKDOWN_TABLE_LINE_PATTERN.test(lines[cursor] || "")) {
      cursor += 1;
    }
    while (cursor < lines.length && !text(lines[cursor])) {
      cursor += 1;
    }
    const afterLines = [];
    for (let scan = cursor; scan < lines.length && afterLines.length < 2; scan += 1) {
      const nextLine = text(lines[scan]);
      if (!nextLine) continue;
      afterLines.push(nextLine);
      if (MARKDOWN_TABLE_LINE_PATTERN.test(nextLine) || MATERIAL_SECTION_HEADING_PATTERN.test(nextLine)) break;
    }
    if (!afterLines.length) return true;
    const afterText = afterLines.join(" ");
    if (MATERIAL_SECTION_HEADING_PATTERN.test(afterLines[0]) || MARKDOWN_TABLE_LINE_PATTERN.test(afterLines[0])) return true;
    if (!TABLE_AFTER_ANALYSIS_PATTERN.test(afterText)) return true;
    index = cursor;
  }
  return false;
}

function factsPresent(value) {
  if (Array.isArray(value)) return value.length > 0;
  return Object.keys(objectOf(value)).length > 0;
}

function supportedEdgeCountFrom(value) {
  const source = objectOf(value);
  const explicit = intOrUndefined(source.supported_edge_count);
  if (explicit !== undefined && explicit >= 0) return explicit;
  return arrayOf(source.edges).filter((edge) => text(edge?.edge_status) === "supported").length;
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

function mergeObjects(...sources) {
  const output = {};
  for (const source of sources.map(objectOf)) {
    for (const [key, value] of Object.entries(source)) {
      if (value !== undefined && value !== null && value !== "") output[key] = value;
    }
  }
  return output;
}

function mergeReviewList(source, defaults, limit = 24) {
  return uniqueTexts([
    ...arrayOf(source).map(claimEntryText),
    ...defaults
  ], limit);
}

function claimEntryId(item, category, index) {
  const source = objectOf(item);
  return text(source.claim_id || source.id || source.key) || `${category}.${index + 1}`;
}

function sourceRefsForClaim(item, defaults = {}) {
  const source = objectOf(item);
  const sourceRefs = objectOf(source.source_refs);
  const evidenceRefs = objectOf(source.evidence_refs);
  const citations = objectOf(source.citations);
  const factRefs = [
    ...arrayOf(source.fact_refs),
    ...arrayOf(source.factRefs)
  ];
  const next = pruneEmpty({
    query_ids: uniqueTexts([
      ...arrayOf(defaults.query_ids),
      ...arrayOf(sourceRefs.query_ids),
      ...arrayOf(evidenceRefs.query_ids),
      ...arrayOf(citations.query_ids),
      ...arrayOf(source.query_ids),
      text(source.support_query_name)
    ], 24),
    evidence_ids: uniqueTexts([
      ...arrayOf(defaults.evidence_ids),
      ...arrayOf(sourceRefs.evidence_ids),
      ...arrayOf(evidenceRefs.evidence_ids),
      ...arrayOf(citations.evidence_ids),
      ...arrayOf(source.evidence_ids),
      ...factRefs
    ], 24),
    path_ids: uniqueTexts([
      ...arrayOf(defaults.path_ids),
      ...arrayOf(sourceRefs.path_ids),
      ...arrayOf(evidenceRefs.path_ids),
      ...arrayOf(citations.path_ids),
      ...arrayOf(source.path_ids)
    ], 24),
    report_ids: uniqueTexts([
      ...arrayOf(defaults.report_ids),
      ...arrayOf(sourceRefs.report_ids),
      ...arrayOf(evidenceRefs.report_ids),
      ...arrayOf(citations.report_ids),
      ...arrayOf(source.report_ids)
    ], 24),
    audit_ref: mergeObjects(defaults.audit_ref, sourceRefs.audit_ref, evidenceRefs.audit_ref, citations.audit_ref, source.audit_ref),
    detail_ref: mergeObjects(defaults.detail_ref, sourceRefs.detail_ref, source.detail_ref),
    artifact_id: text(defaults.artifact_id || sourceRefs.artifact_id || evidenceRefs.artifact_id || source.artifact_id)
  });
  return next || {};
}

function claimSupportEntries(category, items, supportStatus, defaultRefs = {}) {
  return arrayOf(items).map((item, index) => {
    const claimText = claimEntryText(item);
    if (!claimText) return undefined;
    const claimId = claimEntryId(item, category, index);
    return pruneEmpty({
      claim_id: claimId,
      category,
      claim_text: claimText,
      support_status: /^(?:unsupported|refuted|rejected|missing|partial|boundary|forbidden|review_action|needs?_evidence|needs?_review)$/u.test(text(objectOf(item).support_status))
        ? text(objectOf(item).support_status)
        : supportStatus,
      source_refs: sourceRefsForClaim(item, defaultRefs)
    });
  }).filter(Boolean);
}

function buildClaimSupportIndex(source = {}, textRisk = {}) {
  const card = objectOf(source);
  return [
    ...claimSupportEntries("candidate_claims", card.candidate_claims, "unsupported"),
    ...claimSupportEntries("verified_claims", card.verified_claims, "unsupported"),
    ...claimSupportEntries("corrected_claims", card.corrected_claims, "needs_review"),
    ...claimSupportEntries("unsupported_claims", card.unsupported_claims, "unsupported"),
    ...claimSupportEntries("unsupported_flows", card.unsupported_flows, "unsupported"),
    ...claimSupportEntries("missing_source_boundaries", card.missing_source_boundaries, "boundary"),
    ...claimSupportEntries("forbidden_phrasings", card.forbidden_phrasings, "forbidden"),
    ...claimSupportEntries("next_review_actions", card.next_review_actions, "review_action"),
    ...claimSupportEntries("text_risk_unsupported_claims", textRisk.unsupported_claims, "unsupported"),
    ...claimSupportEntries("text_risk_unsupported_flows", textRisk.unsupported_flows, "unsupported")
  ].slice(0, 80);
}

export function analyzeReportClaimText(reportText, options = {}) {
  const source = objectOf(options);
  const reportTextValue = text(reportText);
  const structuredClaimRows = [
    ...claimRows(source.claims),
    ...claimRows(source.report_claims)
  ];
  const claimRowList = structuredClaimRows.length || !reportTextValue
    ? structuredClaimRows
    : [{ source: {}, id: "report_text", text: reportTextValue }];
  const body = [
    reportTextValue,
    ...claimRowList.map((claim) => claim.text)
  ].filter(Boolean).join("\n");
  if (!body) return {};
  const hasFacts = factsPresent(source.facts);
  const explicitSupportedEdgeCount = intOrUndefined(source.supportedEdgeCount);
  const supportedEdgeCount = explicitSupportedEdgeCount !== undefined && explicitSupportedEdgeCount >= 0
    ? explicitSupportedEdgeCount
    : supportedEdgeCountFrom(source.flow_graph || source.flowGraph);
  const flowSyntaxPattern = /```mermaid|flowchart|graph\s+(?:TD|LR|RL|BT)|-->|→|=>/iu;
  const amountClaimPattern = /(?:\d{1,3}(?:[,，]\d{3})+|\d+)(?:\.\d+)?\s*(?:元|万元|万|亿元|亿)/iu;
  const legalOverreachPattern = /(?:已查明|确定|坐实).{0,24}(?:涉黑|违法所得|实际控制|代持|最终资金归属|资金归属)/u;
  const candidateOwnershipPattern = /(?:候选账户.{0,36}(?:确认|确定|已查明|已证实|坐实).{0,36}(?:名下|归属|实际控制|持有)|(?:确认|确定|已查明|已证实|坐实).{0,36}候选账户.{0,36}(?:名下|归属|实际控制|持有))/u;
  const cashAssetDestinationPattern = /(?:(?:资金|款项|赃款|涉案资金).{0,48}(?:最终)?(?:流向|去向|归属|进入|转入|归集至).{0,48}(?:现金|取现|理财|资产|房产|车辆)|(?:现金|取现|理财|资产|房产|车辆).{0,48}(?:最终去向|最终流向|归属|已查明|确定|确认|坐实))/u;
  const missingCounterpartyBreakPattern = /(?:(?:双空对手方|缺失对手方|未知对手方|空户名)|(?:对手方|收款方|付款方).{0,24}(?:为空|缺失|未知|未匹配|双空)).{0,48}(?:(?:已查明|确定|确认|坐实).{0,48}(?:流向|去向|归属|闭环)|(?:流向|去向|归属|闭环).{0,24}(?:已查明|确定|确认|坐实))/u;
  const hasFlowSyntax = flowSyntaxPattern.test(body);
  const hasAmountClaim = amountClaimPattern.test(body);
  const matchedForbidden = DEFAULT_FORBIDDEN_PHRASES.filter((phrase) => body.includes(phrase));
  const hasLegalOverreach = hasUnguardedRisk(body, legalOverreachPattern);
  const hasCandidateOwnershipOverreach = hasUnguardedRisk(body, candidateOwnershipPattern);
  const hasCashAssetDestinationWithoutFlow = supportedEdgeCount <= 0 && hasUnguardedRisk(body, cashAssetDestinationPattern);
  const hasMissingCounterpartyBreakAsVerified = supportedEdgeCount <= 0 && hasUnguardedRisk(body, missingCounterpartyBreakPattern);
  const hasTableWithoutAnalysis = hasMarkdownTableWithoutAnalysis(body);
  const unanchoredSensitiveClaimIds = claimRowList
    .filter((claim) => {
      const claimText = text(claim.text);
      if (hasClaimSourceAnchor(claim.source)) return false;
      return hasUnguardedRisk(claimText, amountClaimPattern)
        || hasUnguardedRisk(claimText, flowSyntaxPattern)
        || hasUnguardedRisk(claimText, legalOverreachPattern)
        || hasUnguardedRisk(claimText, candidateOwnershipPattern)
        || hasUnguardedRisk(claimText, cashAssetDestinationPattern)
        || hasUnguardedRisk(claimText, missingCounterpartyBreakPattern);
    })
    .map((claim) => text(claim.id))
    .slice(0, 8);
  const unsupportedClaims = [];
  const unsupportedFlows = [];
  const missingSourceBoundaries = [];
  const forbiddenPhrasings = [...matchedForbidden];
  const nextReviewActions = [];
  if (hasAmountClaim && !hasFacts) {
    unsupportedClaims.push("报告正文包含金额表述，但未提供可追溯事实或确定性计算，不能写成已核事实。");
    missingSourceBoundaries.push("金额、笔数、账户、户名、路径和研判结论需回到确定性事实、可回放计算或交付附件。");
    nextReviewActions.push("补充事实来源后重新复核；金额研判结论未绑定事实前只能写线索/需核实。");
  }
  if (hasFlowSyntax && supportedEdgeCount <= 0) {
    unsupportedFlows.push("报告正文包含资金流向图、流程图或箭头链路，但没有确定性交易边。");
    missingSourceBoundaries.push("未返回确定性交易边时，不得画资金流向图、流程图或箭头。");
    nextReviewActions.push("先补确定性交易边，再决定是否画图。");
  }
  if (hasCandidateOwnershipOverreach) {
    unsupportedClaims.push("候选账户、关联卡、疑似归属账户在未完成开户/身份/控制证据复核前，不能写成已查明名下账户或确定归属。");
    forbiddenPhrasings.push("candidate_account_as_owned_account");
    nextReviewActions.push("将候选账户归属表述降级为线索；补开户信息、证件号、账户控制和同主体证据后再复核。");
  }
  if (hasCashAssetDestinationWithoutFlow) {
    unsupportedFlows.push("现金、取现、理财、资产端或最终去向未绑定确定性交易边和证据包，不能写成确定性资金流向。");
    missingSourceBoundaries.push("资产形态转换、理财申赎、现金断点和最终归属需回到逐笔流水、回单、产品凭证或余额承接证据。");
    nextReviewActions.push("补调收款账户后续流水、现金取现凭证、理财/资产产品凭证和余额承接材料；未补证前只能写线索/需核实。");
  }
  if (hasMissingCounterpartyBreakAsVerified) {
    unsupportedClaims.push("对手方字段缺失、现金断点或未匹配端点时，不能把资金去向、归属或闭环写成已确认。");
    unsupportedFlows.push("对手方/收付款方缺失或未匹配时，不能把断点后的流向、去向、归属或闭环写成已确认。");
    missingSourceBoundaries.push("对手方字段缺失、现金断点或未匹配端点需作为证据缺口显式说明。");
    nextReviewActions.push("先补对手方账号/户名、银行回单、现金凭证或后续账户流水，再决定是否升级为确定性链路。");
  }
  if (hasLegalOverreach || matchedForbidden.length) {
    if (hasLegalOverreach && !forbiddenPhrasings.includes("forbidden_legal_phrasing")) {
      forbiddenPhrasings.push("forbidden_legal_phrasing");
    }
    unsupportedClaims.push("涉黑、违法所得、实际控制、代持、最终资金归属等只能写线索/需复核，不能写已查明或确定。");
    nextReviewActions.push("删除法律定性或改写为线索/需复核，并交由人工/法务证据链复核。");
  }
  if (hasTableWithoutAnalysis) {
    unsupportedClaims.push("报告正文包含金额表、Top 表、资金链路表或续调清单，但表后未接研判意见；不能用表格或附件目录替代异常特征、证明价值、证据缺口和补证动作。");
    missingSourceBoundaries.push("表格进入报告级材料时，需在表后写明异常特征、证明价值、仍不能认定事项和下一步调取材料。");
    nextReviewActions.push("为每张报告级表格补表后研判意见；无法说明证明价值的表格应降级为附件或待复核材料。");
  }
  if (unanchoredSensitiveClaimIds.length) {
    unsupportedClaims.push(`${unanchoredSensitiveClaimIds.join(", ")} 包含金额、链路、归属、法律定性或资产/现金去向表述，但未声明事实来源；不能写成已复核研判结论。`);
    missingSourceBoundaries.push("报告级研判结论需绑定事实来源，或作为未支持/需纠正结论降级输出。");
    nextReviewActions.push("为每条金额、路径、主体归属、资金边和法律敏感研判结论补事实来源后重新复核。");
  }
  return pruneEmpty({
    unsupported_claims: unsupportedClaims,
    unsupported_flows: unsupportedFlows,
    missing_source_boundaries: missingSourceBoundaries,
    forbidden_phrasings: forbiddenPhrasings,
    next_review_actions: nextReviewActions,
    risk_markers: {
      report_claim_text_risk_scan: true,
      amount_without_fact_anchor: hasAmountClaim && !hasFacts,
      unsupported_mermaid_or_arrow_flow: hasFlowSyntax && supportedEdgeCount <= 0,
      forbidden_legal_phrasing: hasLegalOverreach || matchedForbidden.length > 0,
      candidate_account_as_owned_account: hasCandidateOwnershipOverreach,
      cash_or_asset_destination_without_supported_flow: hasCashAssetDestinationWithoutFlow,
      missing_counterparty_or_cash_break_as_verified: hasMissingCounterpartyBreakAsVerified,
      table_without_analysis: hasTableWithoutAnalysis,
      claim_without_source_anchor: unanchoredSensitiveClaimIds.length > 0
    }
  }) || {};
}

function firstObject(...values) {
  for (const value of values) {
    const source = objectOf(value);
    if (Object.keys(source).length) return source;
  }
  return {};
}

function investigationAnswerCandidate(source = {}, options = {}) {
  const card = objectOf(source);
  const settings = objectOf(options);
  return firstObject(
    settings.finalInvestigationAnswer,
    settings.final_investigation_answer,
    card.final_investigation_answer,
    card.investigation_answer,
    card.finalAnswerContract,
    card.final_answer_contract,
    objectOf(card.report_contract).final_investigation_answer,
    objectOf(card.delivery_contract).final_investigation_answer
  );
}

export function analyzeInvestigationAnswerContract(answer, options = {}) {
  const source = objectOf(answer);
  if (!Object.keys(source).length) return {};

  const validation = validateInvestigationAnswerContract(source, options);
  const unsupportedClaims = [];
  const unsupportedFlows = [];
  const missingSourceBoundaries = [];
  const forbiddenPhrasings = [];
  const nextReviewActions = [];

  for (const error of arrayOf(validation.errors)) {
    const code = text(error.code);
    const path = text(error.path);
    const message = text(error.message);
    const reviewLine = [`FinalInvestigationAnswer`, path, message].filter(Boolean).join("：");
    if (/^FLOW_/u.test(code)) {
      unsupportedFlows.push(reviewLine);
    } else if (/(?:SOURCE|EVIDENCE|BOUNDARY|REFS|OBJECTIVE_SOURCE)/u.test(code)) {
      missingSourceBoundaries.push(reviewLine);
    } else {
      unsupportedClaims.push(reviewLine);
    }
    if (/(?:LEGAL|VISIBLE|FORBIDDEN)/u.test(code)) {
      forbiddenPhrasings.push(code);
    }
  }

  for (const action of arrayOf(validation.repair_actions)) {
    const instruction = text(objectOf(action).instruction);
    const actionPath = text(objectOf(action).path);
    if (instruction) {
      nextReviewActions.push([actionPath, instruction].filter(Boolean).join("："));
    }
  }

  return pruneEmpty({
    unsupported_claims: unsupportedClaims,
    unsupported_flows: unsupportedFlows,
    missing_source_boundaries: missingSourceBoundaries,
    forbidden_phrasings: forbiddenPhrasings,
    next_review_actions: nextReviewActions,
    audit_events: arrayOf(validation.audit_events),
    risk_markers: {
      investigation_answer_contract_checked: true,
      investigation_answer_contract_failed: validation.ok !== true,
      investigation_answer_contract_error_codes: uniqueTexts(arrayOf(validation.errors).map((error) => objectOf(error).code), 24),
      investigation_answer_audit_event_count: arrayOf(validation.audit_events).length,
      investigation_answer_audit_categories: uniqueTexts(arrayOf(validation.audit_events).map((event) => objectOf(event).category), 12),
      investigation_answer_contract_version: validation.contract_version
    }
  }) || {};
}

export function withClaimReviewProtocol(card, options = {}) {
  const source = objectOf(card);
  const claimReview = objectOf(source.claim_review);
  const sanitizedSource = stripClaimAuthorityAssertions(source);
  const sanitizedClaimReview = stripClaimAuthorityAssertions(claimReview);
  const candidateClaims = candidateClaimsFrom(source, claimReview);
  const hostAuthorityBoundary = "后端 claim、support 状态、citation 或 receipt 字符串仅是待核候选；当前插件没有宿主权威 registry membership 和 PublicationReceipt，正式报告必须保持阻断。";
  const textRisk = analyzeReportClaimText(options.reportText ?? source.report_text ?? source.reportText, {
    claims: source.claims,
    report_claims: source.report_claims,
    facts: source.facts,
    flow_graph: source.flow_graph,
    supportedEdgeCount: source.supported_edge_count
  });
  const contractRisk = analyzeInvestigationAnswerContract(investigationAnswerCandidate(source, options), options);
  const combinedRisk = {
    unsupported_claims: [
      ...arrayOf(textRisk.unsupported_claims),
      ...arrayOf(contractRisk.unsupported_claims)
    ],
    unsupported_flows: [
      ...arrayOf(textRisk.unsupported_flows),
      ...arrayOf(contractRisk.unsupported_flows)
    ],
    missing_source_boundaries: [
      ...arrayOf(textRisk.missing_source_boundaries),
      ...arrayOf(contractRisk.missing_source_boundaries),
      hostAuthorityBoundary
    ],
    forbidden_phrasings: [
      ...arrayOf(textRisk.forbidden_phrasings),
      ...arrayOf(contractRisk.forbidden_phrasings)
    ],
    next_review_actions: [
      ...arrayOf(textRisk.next_review_actions),
      ...arrayOf(contractRisk.next_review_actions),
      "由宿主逐 claim 校验同案 registry receipt 的字段、范围和 citation membership，并在原子发布后签发 PublicationReceipt。"
    ],
    audit_events: [
      ...arrayOf(contractRisk.audit_events)
    ].slice(0, 40),
    risk_markers: {
      ...objectOf(textRisk.risk_markers),
      ...objectOf(contractRisk.risk_markers),
      backend_claims_candidate_only: true,
      host_registry_membership_verified: false,
      publication_receipt_verified: false
    }
  };
  const publicationGate = p0PublicationGate({ candidateClaims, combinedRisk });
  const mergedBase = {
    ...sanitizedSource,
    card_type: "claim_review_card",
    intent: text(source.intent) || "claim_review",
    status: "blocked",
    semantic_status: "blocked",
    safeToAnswer: false,
    safe_to_answer_current_task: false,
    write_blocked: true,
    report_gate_status: "publication_receipt_required",
    host_registry_verified: false,
    registry_membership_verified: false,
    publication_receipt_verified: false,
    publication_gate: publicationGate,
    answer_sections: mergeReviewList(source.answer_sections, DEFAULT_ANSWER_SECTIONS, 12),
    stop_conditions: mergeReviewList(source.stop_conditions, DEFAULT_STOP_CONDITIONS, 16),
    candidate_claims: candidateClaims,
    verified_claims: [],
    supported_claims: [],
    corrected_claims: arrayOf(source.corrected_claims).map(claimEntryText).filter(Boolean),
    unsupported_claims: uniqueTexts([
      ...arrayOf(source.unsupported_claims).map(claimEntryText),
      ...arrayOf(combinedRisk.unsupported_claims)
    ], 24),
    unsupported_flows: mergeReviewList([
      ...arrayOf(source.unsupported_flows),
      ...arrayOf(combinedRisk.unsupported_flows)
    ], DEFAULT_UNSUPPORTED_FLOWS, 16),
    missing_source_boundaries: mergeReviewList([
      ...arrayOf(source.missing_source_boundaries),
      ...arrayOf(combinedRisk.missing_source_boundaries)
    ], DEFAULT_MISSING_SOURCE_BOUNDARIES, 16),
    forbidden_phrasings: mergeReviewList([
      ...arrayOf(source.forbidden_phrasings),
      ...arrayOf(combinedRisk.forbidden_phrasings)
    ], DEFAULT_FORBIDDEN_PHRASES, 16),
    next_review_actions: mergeReviewList([
      ...arrayOf(source.next_review_actions),
      ...arrayOf(combinedRisk.next_review_actions)
    ], DEFAULT_NEXT_REVIEW_ACTIONS, 16),
    answer_constraints: mergeReviewList(source.answer_constraints, DEFAULT_ANSWER_CONSTRAINTS, 18),
    claim_text_risk_scan: combinedRisk.risk_markers,
    investigation_answer_contract_review: contractRisk.risk_markers,
    _meta: {
      ...objectOf(sanitizedSource._meta),
      analytix_publication_gate: publicationGate,
      analytix_evidence_ledger: {
        ...objectOf(objectOf(sanitizedSource._meta).analytix_evidence_ledger),
        host_registry_verified: false,
        publication_receipt_verified: false
      },
      audit_events: [
        ...arrayOf(objectOf(sanitizedSource._meta).audit_events),
        ...arrayOf(combinedRisk.audit_events)
      ].slice(0, 40)
    },
    claim_review: {
      ...sanitizedClaimReview,
      supported: [],
      candidate_supported: candidateClaims.map(claimEntryText).filter(Boolean),
      corrected: arrayOf(claimReview.corrected).map(text).filter(Boolean),
      unsupported: mergeReviewList([
        ...arrayOf(claimReview.unsupported),
        ...arrayOf(combinedRisk.unsupported_claims),
        ...arrayOf(combinedRisk.unsupported_flows)
      ], DEFAULT_UNSUPPORTED_FLOWS, 12),
      downgraded: mergeReviewList(claimReview.downgraded, [
        "涉黑、违法所得、实际控制、代持等只能写线索/需复核，不能写已查明或确定。"
      ], 12)
    }
  };
  const merged = {
    ...mergedBase,
    claim_support_index: buildClaimSupportIndex({
      ...mergedBase,
      candidate_claims: candidateClaims,
      verified_claims: [],
      corrected_claims: arrayOf(source.corrected_claims).length ? source.corrected_claims : mergedBase.corrected_claims,
      unsupported_claims: [
        ...(arrayOf(source.unsupported_claims).length ? arrayOf(source.unsupported_claims) : arrayOf(mergedBase.unsupported_claims)),
        ...arrayOf(combinedRisk.unsupported_claims).map((item, index) => ({
          claim_id: `text_risk.unsupported_claim.${index + 1}`,
          category: "unsupported_claims",
          risk_marker: /FinalInvestigationAnswer/u.test(text(item)) ? "investigation_answer_contract" : "claim_text_risk_scan",
          text: item
        }))
      ],
      unsupported_flows: [
        ...(arrayOf(source.unsupported_flows).length ? arrayOf(source.unsupported_flows) : arrayOf(mergedBase.unsupported_flows)),
        ...arrayOf(combinedRisk.unsupported_flows).map((item, index) => ({
          claim_id: `text_risk.unsupported_flow.${index + 1}`,
          category: "unsupported_flows",
          risk_marker: /FinalInvestigationAnswer/u.test(text(item)) ? "investigation_answer_contract" : "claim_text_risk_scan",
          text: item
        }))
      ],
      missing_source_boundaries: [
        ...(arrayOf(source.missing_source_boundaries).length ? arrayOf(source.missing_source_boundaries) : arrayOf(mergedBase.missing_source_boundaries)),
        ...arrayOf(combinedRisk.missing_source_boundaries).map((item, index) => ({
          claim_id: `text_risk.missing_source_boundary.${index + 1}`,
          category: "missing_source_boundaries",
          risk_marker: /FinalInvestigationAnswer/u.test(text(item)) ? "investigation_answer_contract" : "claim_text_risk_scan",
          text: item
        }))
      ],
      forbidden_phrasings: [
        ...(arrayOf(source.forbidden_phrasings).length ? arrayOf(source.forbidden_phrasings) : arrayOf(mergedBase.forbidden_phrasings)),
        ...arrayOf(combinedRisk.forbidden_phrasings).map((item, index) => ({
          claim_id: `text_risk.forbidden_phrasing.${index + 1}`,
          category: "forbidden_phrasings",
          risk_marker: /LEGAL|VISIBLE|FORBIDDEN/u.test(text(item)) ? "investigation_answer_contract" : "claim_text_risk_scan",
          text: item
        }))
      ],
      next_review_actions: [
        ...(arrayOf(source.next_review_actions).length ? arrayOf(source.next_review_actions) : arrayOf(mergedBase.next_review_actions)),
        ...arrayOf(combinedRisk.next_review_actions).map((item, index) => ({
          claim_id: `text_risk.next_review_action.${index + 1}`,
          category: "next_review_actions",
          risk_marker: /evidence_refs|MCP|DuckDB|FinalInvestigationAnswer/u.test(text(item)) ? "investigation_answer_contract" : "claim_text_risk_scan",
          text: item
        }))
      ]
    }, combinedRisk)
  };
  return withAnswerCardProtocol(merged, {
    recommendedNextAction: "stop",
    maxAdditionalTools: 0,
    requiredFactsPresent: false,
    unsupportedFlowsPresent: true
  });
}
