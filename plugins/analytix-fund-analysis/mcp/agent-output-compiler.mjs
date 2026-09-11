import {
  redactAgentPayload,
  stripAgentOpaqueRefs
} from "./agent-context-hygiene.mjs";
import { translateUserVisibleText } from "./user-facing-language.mjs";
import {
  compactAccountStatsFact,
  compactCaseGraphForText,
  compactFlowGraphForText,
  compactKeyFactsForText,
  compactToolPayloadForAgent,
  compactWarningList,
  minimalCoverageFact
} from "./agent-payload-compiler.mjs";
import {
  PUBLICATION_RECEIPT_REQUIRED_TEXT,
  reportPublicationBlockedPayload,
  requiresPublicationBlock
} from "./report-publication-guard.mjs";
import {
  arrayOf,
  intOrUndefined,
  numberOrUndefined,
  objectOf,
  pruneEmpty,
  text,
  uniqueTexts
} from "./runtime-normalizers.mjs";

export const AGENT_OUTPUT_COMPILER_VERSION = "0.16.9";
export const DEFAULT_AGENT_TEXT_BUDGET = 20_000;

const COMPETING_PAIR_RANK_KEY_PATTERN = /^(?:rank_candidate|rank_cross_check)_/u;
const EXTERNAL_COUNT_KEY_PATTERN = /(?:^|_)(?:count|row_count|txn_count|account_count|counterparty_count)(?:_|$)/iu;
const EXTERNAL_AMOUNT_KEY_PATTERN = /(?:^|_)(?:amount|money|turnover|inflow|outflow|balance)(?:_|$)/iu;
const EXTERNAL_RATIO_KEY_PATTERN = /(?:^|_)(?:ratio|share|percent|percentage)(?:_|$)/iu;

function optionalCount(value) {
  const number = intOrUndefined(value);
  return number !== undefined && number >= 0 && Number.isSafeInteger(number) ? number : undefined;
}

function isSupportedStatus(value) {
  return /^(?:supported|verified|已有数据支持|已有流水支持|已支持|已核验)$/iu.test(text(value));
}

function optionalMoneyNumber(value) {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const source = objectOf(value);
    for (const key of ["yuan", "amount_yuan", "amount", "value"]) {
      if (Object.prototype.hasOwnProperty.call(source, key)) {
        return numberOrUndefined(source[key]);
      }
    }
    return undefined;
  }
  return numberOrUndefined(value);
}

function optionalNonNegativeMoney(value) {
  const number = optionalMoneyNumber(value);
  return number !== undefined && number >= 0 ? number : undefined;
}

function firstOptionalValue(parser, ...values) {
  for (const value of values) {
    if (value === undefined) continue;
    return parser(value);
  }
  return undefined;
}

function externalNumericValue(key, value) {
  if (EXTERNAL_COUNT_KEY_PATTERN.test(key)) return optionalCount(value);
  if (EXTERNAL_AMOUNT_KEY_PATTERN.test(key)) return optionalMoneyNumber(value);
  if (EXTERNAL_RATIO_KEY_PATTERN.test(key)) return numberOrUndefined(value);
  return null;
}

function exactCountWorkbenchSlice(compact) {
  const source = objectOf(compact);
  const workbench = objectOf(objectOf(source.key_facts).workbench);
  if (text(source.tool || workbench.tool) !== "count_case_rows") return undefined;
  return pruneEmpty({
    tool: text(workbench.tool),
    table_name: text(workbench.table_name),
    row_count: workbench.row_count,
    returned_row_count: workbench.returned_row_count,
    result_kind: text(workbench.result_kind),
    exact_count_contract: text(workbench.exact_count_contract),
    boundary: workbench.boundary
  });
}

function exactCountContractMatches(contract, count) {
  const value = text(contract);
  const formattedCount = count.toLocaleString("en-US");
  return Boolean(
    value
      && value.includes(`精确记录数为 ${formattedCount} 条`)
      && value.includes(`原始整数 ${count}`)
      && value.includes("不得四舍五入")
      && value.includes("不得改写为")
  );
}

function exactCountBoundaryText(count) {
  if (count === 0) {
    return "当前受控计数显式返回 0 条（未验证未命中）；空结果不等于全案为零、不存在、无关联或无异常；宿主证据门通过前不得作为案件事实或直接答复。";
  }
  const formattedCount = count.toLocaleString("en-US");
  return `当前受控计数精确记录数为 ${formattedCount} 条（原始整数 ${count}）；不得四舍五入、不得改写为“约/大约/左右/万条”概数；宿主证据门通过前不得作为案件事实或直接答复。`;
}

function projectExactCountEvidence(source, compactEvidence) {
  const payload = objectOf(source);
  const rawWorkbench = objectOf(objectOf(payload.key_facts).workbench);
  const evidence = { ...objectOf(compactEvidence) };
  if (text(payload.tool || rawWorkbench.tool) !== "count_case_rows") {
    return { applies: false, compactEvidence: evidence };
  }

  const count = optionalCount(rawWorkbench.row_count);
  const returnedCount = optionalCount(rawWorkbench.returned_row_count);
  const complete = count !== undefined
    && returnedCount === count
    && text(rawWorkbench.result_kind) === "row_count_only"
    && exactCountContractMatches(rawWorkbench.exact_count_contract, count);
  const workbench = { ...objectOf(evidence.workbench) };
  for (const key of [
    "answer_card_complete",
    "answer_ready",
    "coverage_complete",
    "fact_answer_allowed",
    "safeToAnswer",
    "safe_to_answer",
    "verified_no_hit",
    "verified_no_hit_allowed",
    "zero_result_status"
  ]) {
    delete workbench[key];
  }
  evidence.workbench = pruneEmpty({
    ...workbench,
    row_count: complete ? count : undefined,
    returned_row_count: complete ? count : undefined,
    exact_count_contract: complete ? exactCountBoundaryText(count) : undefined,
    exact_count_contract_status: complete ? "valid_boundary_only" : "invalid_or_incomplete",
    fact_answer_allowed: false,
    verified_no_hit_allowed: false,
    coverage_complete: false,
    exact_count_boundary: "Only a host-verified EvidenceReceipt may make this count publishable; caller readiness and a zero value are never sufficient."
  }) || {};
  return {
    applies: true,
    complete,
    zeroResult: complete && count === 0,
    status: complete ? "valid_boundary_only" : "invalid_or_incomplete",
    compactEvidence: evidence
  };
}

function canonicalSemanticStatus(...values) {
  let fallback = "";
  let sawOK = false;
  let sawVerifiedNoHit = false;
  let sawPartial = false;
  let sawFailed = false;
  let sawBlocked = false;
  for (const value of values) {
    const raw = text(value);
    if (!raw) continue;
    fallback ||= raw;
    if (/blocked|unavailable|阻断|不可用/iu.test(raw)) sawBlocked = true;
    else if (/failed|failure|error|unsafe|失败|错误|不安全/iu.test(raw)) sawFailed = true;
    else if (/partial|incomplete|truncated|部分|不完整|已截断/iu.test(raw)) sawPartial = true;
    else if (/verified[_ -]?no[_ -]?hit/iu.test(raw)) sawVerifiedNoHit = true;
    else if (/^(?:ok|success|succeeded|complete|completed|ready)$/iu.test(raw)) sawOK = true;
  }
  if (sawBlocked) return "blocked";
  if (sawFailed) return "failed";
  if (sawPartial) return "partial";
  if (sawVerifiedNoHit) return "verified_no_hit";
  if (sawOK) return "ok";
  return fallback;
}

function stableSnapshotId(value) {
  const id = text(value);
  return /^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$/u.test(id) ? id : "";
}

function preserveStableBoundaryMetadata(rawValue, scrubbedValue) {
  const raw = objectOf(rawValue);
  const scrubbed = { ...objectOf(scrubbedValue) };
  const rawPack = objectOf(objectOf(raw.key_facts).full_case_package);
  const scrubbedKeyFacts = objectOf(scrubbed.key_facts);
  const scrubbedPack = objectOf(scrubbedKeyFacts.full_case_package);
  const rawCoverage = objectOf(rawPack.coverage);
  const scrubbedCoverage = objectOf(scrubbedPack.coverage);
  const semanticStatus = canonicalSemanticStatus(raw.semantic_status, rawPack.semantic_status);
  const coverageStatus = canonicalSemanticStatus(raw.coverage_status, rawPack.coverage_status);
  const sourceCoverageComplete = firstBooleanValue(
    raw.source_coverage_complete,
    rawPack.source_coverage_complete,
    rawCoverage.source_coverage_complete
  );
  const partialCoverage = firstBooleanValue(
    raw.partial_coverage,
    rawPack.partial_coverage,
    rawCoverage.partial_coverage
  );
  const sourceDatasetSnapshotId = stableSnapshotId(
    raw.source_dataset_snapshot_id
      || rawPack.source_dataset_snapshot_id
      || rawCoverage.source_dataset_snapshot_id
  );
  if (semanticStatus) scrubbed.semantic_status = semanticStatus;
  if (coverageStatus) scrubbed.coverage_status = coverageStatus;
  if (sourceCoverageComplete !== undefined) scrubbed.source_coverage_complete = sourceCoverageComplete;
  if (partialCoverage !== undefined) scrubbed.partial_coverage = partialCoverage;
  if (sourceDatasetSnapshotId) scrubbed.source_dataset_snapshot_id = sourceDatasetSnapshotId;
  if (Object.keys(scrubbedPack).length) {
    scrubbed.key_facts = {
      ...scrubbedKeyFacts,
      full_case_package: {
        ...scrubbedPack,
        semantic_status: semanticStatus || undefined,
        coverage_status: coverageStatus || undefined,
        source_coverage_complete: sourceCoverageComplete,
        partial_coverage: partialCoverage,
        source_dataset_snapshot_id: sourceDatasetSnapshotId || undefined,
        coverage: Object.keys(scrubbedCoverage).length
          ? {
              ...scrubbedCoverage,
              semantic_status: semanticStatus || undefined,
              coverage_status: coverageStatus || undefined,
              source_coverage_complete: sourceCoverageComplete,
              partial_coverage: partialCoverage,
              source_dataset_snapshot_id: sourceDatasetSnapshotId || undefined
            }
          : undefined
      }
    };
  }
  return scrubbed;
}

function firstBooleanValue(...values) {
  for (const value of values) {
    if (typeof value === "boolean") return value;
  }
  return undefined;
}

function fullCaseFactNumbers(pack) {
  const source = objectOf(pack);
  const coverage = objectOf(source.coverage);
  const scope = objectOf(source.scope_stats);
  const values = [];
  for (const [record, keys] of [
    [coverage, ["txn_total", "txn_analyzed", "account_count", "selected_account_count"]],
    [scope, [
      "txn_count",
      "account_count",
      "counterparty_count",
      "in_txn_count",
      "out_txn_count",
      "directed_txn_count",
      "undirected_txn_count",
      "source_file_count"
    ]]
  ]) {
    for (const key of keys) {
      if (!Object.prototype.hasOwnProperty.call(record, key)) continue;
      const value = optionalCount(record[key]);
      if (value !== undefined) values.push(value);
    }
  }
  for (const key of ["in_amount", "out_amount", "turnover"]) {
    if (!Object.prototype.hasOwnProperty.call(scope, key)) continue;
    const value = optionalMoneyNumber(scope[key]);
    if (value !== undefined) values.push(value);
  }
  return values;
}

function uniqueEvidenceGaps(values, limit = 10) {
  const output = [];
  const seen = new Set();
  for (const value of arrayOf(values)) {
    if (value == null || value === "") continue;
    const key = typeof value === "string" ? value : JSON.stringify(value);
    if (seen.has(key)) continue;
    seen.add(key);
    output.push(value);
    if (output.length >= limit) break;
  }
  return output;
}

function fullCaseBoundaryState(source, compactEvidence = {}) {
  const payload = objectOf(source);
  const validation = objectOf(payload.validation_state);
  const evidence = objectOf(compactEvidence);
  const pack = compactFullCaseDigest(
    evidence.full_case_package
      || objectOf(payload.key_facts).full_case_package
      || objectOf(payload.support_facts).full_case_package
  );
  if (!Object.keys(pack).length) {
    return { applies: false, pack: {} };
  }
  const coverage = objectOf(pack.coverage);
  const semanticStatus = canonicalSemanticStatus(
    pack.semantic_status,
    payload.semantic_status,
    validation.semantic_status,
    payload.status,
    validation.status
  );
  const reportedCoverageComplete = firstBooleanValue(
    pack.source_coverage_complete,
    coverage.source_coverage_complete,
    payload.source_coverage_complete,
    validation.source_coverage_complete
  );
  const partialCoverage = semanticStatus === "partial"
    || pack.partial_coverage === true
    || coverage.partial_coverage === true
    || payload.partial_coverage === true
    || validation.partial_coverage === true
    || reportedCoverageComplete === false
    || /partial|incomplete|truncated|部分|不完整|已截断/iu.test(text(
      pack.coverage_status
        || coverage.coverage_status
        || payload.coverage_status
        || validation.coverage_status
    ));
  const rowFields = ["top_accounts", "top_holders", "top_destinations", "top_sources", "top_outflows"];
  const presentRowFields = rowFields.filter((key) => Object.prototype.hasOwnProperty.call(pack, key));
  const hasReturnedRows = rowFields.some((key) => arrayOf(pack[key]).length > 0);
  const explicitEmptyCollections = presentRowFields.length > 0
    && presentRowFields.every((key) => arrayOf(pack[key]).length === 0);
  const factNumbers = fullCaseFactNumbers(pack);
  const explicitZero = factNumbers.some((value) => value === 0);
  const hasNonZero = factNumbers.some((value) => value !== 0);
  const inferredEmptyResult = !hasNonZero
    && !hasReturnedRows
    && (explicitZero || explicitEmptyCollections);
  const emptyResult = text(pack.empty_result_status || validation.empty_result_status) === "unverified_empty_result"
    || inferredEmptyResult;
  const evidenceGaps = uniqueEvidenceGaps([
    ...arrayOf(pack.evidence_gaps),
    ...arrayOf(coverage.evidence_gaps),
    ...arrayOf(payload.evidence_gaps),
    ...arrayOf(validation.evidence_gaps)
  ]);
  return {
    applies: true,
    pack,
    semanticStatus,
    reportedCoverageComplete,
    partialCoverage,
    emptyResult,
    boundaryOnly: partialCoverage || emptyResult || pack.boundary_only === true || validation.boundary_only === true,
    reportedDatasetSnapshotId: stableSnapshotId(
      payload.source_dataset_snapshot_id
        || validation.source_dataset_snapshot_id
        || pack.source_dataset_snapshot_id
        || coverage.source_dataset_snapshot_id
    ),
    evidenceGaps
  };
}

function suppressBoundaryZeroFacts(value, depth = 0) {
  if (depth > 10) return undefined;
  if (typeof value === "number") {
    return Number.isFinite(value) && value !== 0 ? value : undefined;
  }
  if (typeof value === "string") {
    if (/^[+-]?0+(?:\.0+)?$/u.test(value.trim())) return undefined;
    return value;
  }
  if (Array.isArray(value)) {
    const rows = value
      .map((item) => suppressBoundaryZeroFacts(item, depth + 1))
      .filter((item) => item !== undefined);
    return rows.length ? rows : undefined;
  }
  if (!value || typeof value !== "object") return value;
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    const next = suppressBoundaryZeroFacts(item, depth + 1);
    if (next !== undefined) output[key] = next;
  }
  return Object.keys(output).length ? output : undefined;
}

function projectFullCaseBoundaryEvidence(compactEvidence, state) {
  if (!state.applies || !state.boundaryOnly) return compactEvidence;
  const evidence = { ...objectOf(compactEvidence) };
  const safePack = objectOf(suppressBoundaryZeroFacts(state.pack));
  evidence.full_case_package = pruneEmpty({
    ...safePack,
    semantic_status: state.semanticStatus,
    coverage_status: state.partialCoverage ? "partial" : safePack.coverage_status,
    source_coverage_complete: state.reportedCoverageComplete,
    partial_coverage: state.partialCoverage || undefined,
    source_dataset_snapshot_id: state.reportedDatasetSnapshotId || undefined,
    evidence_gaps: state.evidenceGaps,
    coverage_complete: false,
    fact_answer_allowed: false,
    verified_no_hit_allowed: false,
    empty_result_status: state.emptyResult ? "unverified_empty_result" : undefined,
    boundary_only: true
  }) || {};
  return evidence;
}

const NARRATIVE_SUPPORT_KEYS = new Set([
  "answer_card_complete",
  "answer_constraints",
  "answer_draft",
  "answer_ready",
  "answer_sections",
  "answer_text",
  "case_id",
  "context_compiler",
  "description",
  "delivery_state",
  "evidence_status",
  "interpretation",
  "long_description",
  "max_additional_tools",
  "message",
  "no_more_tools_needed",
  "prompt",
  "question",
  "recommended_next_action",
  "short_description",
  "summary",
  "support_material_only",
  "text",
  "title",
  "validation_state",
  "why_this_query"
]);

const SUPPORT_CARD_EVIDENCE_KEYS = [
  "card_type",
  "intent",
  "fact_source",
  "facts",
  "holder_name",
  "holder_names",
  "holder_name_variants",
  "via_holder_name",
  "account_key",
  "date_start",
  "date_end",
  "source_scope",
  "coverage",
  "import_lineage_summary",
  "statistical_period",
  "first_txn_at",
  "last_txn_at",
  "raw_detail_amount",
  "raw_detail_count",
  "effective_stat_basis",
  "detail_table_role",
  "high_confidence_same_fact_effective_amount",
  "high_confidence_same_fact_count",
  "full_period_raw_amount",
  "full_period_raw_count",
  "full_period_effective_amount",
  "full_period_effective_count",
  "full_period_first_txn_at",
  "full_period_last_txn_at",
  "full_period_effective_first_txn_at",
  "full_period_effective_last_txn_at",
  "duplicate_or_unsupported_amount",
  "confirmed_same_fact_duplicate_amount",
  "confirmed_same_fact_duplicate_count",
  "unresolved_duplicate_or_unsupported_amount",
  "duplicate_review_status",
  "duplicate_review_explanation",
  "principal_amount_after_fee_exclusion",
  "small_amount_candidate_amount",
  "small_amount_candidate_count",
  "detail_rank_difference_amount",
  "focus_cluster_effective_share",
  "outside_focus_cluster_effective_amount",
  "outside_focus_cluster_effective_count",
  "source_rank_effective_amount",
  "source_rank_duplicate_amount",
  "detail_query_auxiliary_only",
  "focus_cluster",
  "focus_cluster_role",
  "source_account_count",
  "source_accounts",
  "counterparty_account_count",
  "counterparty_accounts",
  "account_clusters",
  "source_account_clusters",
  "date_counterparty_clusters",
  "time_clusters",
  "top_transactions",
  "focus_cluster_transactions",
  "expanded_same_name_scope",
  "large_transactions",
  "duplicate_groups",
  "dedup_key_safety",
  "source_to_via_summary",
  "source_to_via_date_window_summary",
  "source_to_via_core_account_summary",
  "strict_rows",
  "excluded_near_names",
  "cannot_confirm",
  "match_scope",
  "same_row_counted_once",
  "subject_account_count",
  "subject_accounts",
  "buckets",
  "stock_detail_visible",
  "total_txn_count",
  "total_amount",
  "company_subject_count",
  "personal_subject_count",
  "summary_missing_txn_count",
  "summary_missing_amount",
  "nature_buckets",
  "classification_rules",
  "supported_edges",
  "boundary_edges",
  "downstream_edges",
  "product_or_asset_edges",
  "terminal_classification",
  "continuation_targets",
  "required_subpoena_fields",
  "required_evidence",
  "holder_scope",
  "ready_reason",
  "stop_reason",
  "registered_accounts",
  "account_stats",
  "top_accounts",
  "top_holders",
  "top_counterparties",
  "counterparty_rankings",
  "metric_rank_facts",
  "investigative_findings",
  "investigative_focus",
  "hypothesis_cards",
  "investigation_plan_lanes",
  "verified_claims",
  "corrected_claims",
  "claim_review",
  "facts"
];

function supportEvidenceValue(value, depth = 0, key = "") {
  if (depth > 6) return undefined;
  const normalizedNumeric = externalNumericValue(key, value);
  if (normalizedNumeric !== null) return normalizedNumeric;
  if (Array.isArray(value)) {
    const rows = value
      .slice(0, 20)
      .map((item) => supportEvidenceValue(item, depth + 1))
      .filter((item) => item !== undefined);
    return rows.length ? rows : undefined;
  }
  if (!value || typeof value !== "object") {
    if (typeof value === "string") {
      const item = /(?:^|_)(?:status|state)$/iu.test(key) || shouldPreserveSqlIdentifiers(value)
        ? value
        : translateUserVisibleText(value);
      return item || undefined;
    }
    return value === undefined || value === null ? undefined : value;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    if (COMPETING_PAIR_RANK_KEY_PATTERN.test(key)) continue;
    if (NARRATIVE_SUPPORT_KEYS.has(key) || /(?:draft|instruction|heading|prose)$/iu.test(key)) continue;
    const next = supportEvidenceValue(item, depth + 1, key);
    if (next === undefined) continue;
    if (Array.isArray(next) && !next.length) continue;
    if (next && typeof next === "object" && !Array.isArray(next) && !Object.keys(next).length) continue;
    output[key] = next;
  }
  const graphEdge = Object.prototype.hasOwnProperty.call(value, "edge_status")
    && ["edge_id", "txn_id", "from", "to", "from_label", "to_label"]
      .some((field) => Object.prototype.hasOwnProperty.call(value, field));
  if (graphEdge && isSupportedStatus(value.edge_status) && optionalMoneyNumber(value.amount) === undefined) {
    output.edge_status = "needs_evidence";
    output.missing_fields = uniqueTexts([...arrayOf(output.missing_fields), "amount"], 8);
  }
  return Object.keys(output).length ? output : undefined;
}

function supportText(item) {
  if (item && typeof item === "object") {
    const source = objectOf(item);
    return text(source.message || source.code || source.text || source.claim || source.statement || source.reason || source.boundary || source.label);
  }
  return text(item);
}

function supportRiskTexts(payload, card) {
  const cardType = text(objectOf(card).card_type);
  const validationState = text(objectOf(card).validation_state);
  const risks = uniqueTexts([
    ...arrayOf(payload.warnings),
    ...arrayOf(card.warnings),
    ...arrayOf(card.unsupported_claims),
    ...arrayOf(card.unsupported_flows),
    ...arrayOf(card.forbidden_as_facts),
    ...arrayOf(card.missing_source_boundaries),
    ...arrayOf(card.quality_guards)
  ].map(supportText).filter(Boolean), 16);
  if (cardType === "pair_amount_review_card" && /full_period_pair_amount/u.test(validationState)) {
    return risks.filter((item) => !/对手方排名|候选重复金额|折叠额外行/u.test(item));
  }
  return risks;
}

function supportGuidanceText(item) {
  if (item && typeof item === "object") {
    const source = objectOf(item);
    return text(source.reason || source.why_this_query || source.text || source.label || source.summary || source.boundary);
  }
  return text(item);
}

function supportGuidanceItems(payload, card) {
  return [
    ...arrayOf(payload.next_actions).map((item) => ({ item, source_field: "payload.next_actions" })),
    ...arrayOf(card.next_queries).map((item) => ({ item, source_field: "answer_card.next_queries" })),
    ...arrayOf(card.next_review_actions).map((item) => ({ item, source_field: "answer_card.next_review_actions" })),
    ...arrayOf(objectOf(card.context_compiler).next_queries).map((item) => ({ item, source_field: "context_compiler.next_queries" })),
    ...arrayOf(objectOf(card.context_compiler).stop_conditions).map((item) => ({ item, source_field: "context_compiler.stop_conditions" }))
  ];
}

function supportQueryGuidance(payload, card) {
  return uniqueTexts(supportGuidanceItems(payload, card).map(({ item }) => supportGuidanceText(item)).filter(Boolean), 12);
}

function inferredInvestigationActionType(direction, explicitType = "") {
  const source = `${explicitType} ${direction}`;
  if (/补证|调取|回单|流水|开户|凭证|材料|取证/u.test(source)) return "补证取证";
  if (/穿透|下一跳|下游|去向|承接|链路|trace|hop/iu.test(source)) return "资金穿透";
  if (/串并|关联|团伙|共同|设备|柜员|网点|凭证|关系/u.test(source)) return "关系串并";
  if (/重复|同事实|换卡|补卡|去重|质量|缺失|空户名|口径/u.test(source)) return "质量复核";
  if (/范围|账户|主体|户名|归属|边界/u.test(source)) return "范围核验";
  if (/报告|结论|定性|认定|成稿|复核/u.test(source)) return "结论复核";
  return "继续核查";
}

function supportSuggestedNextDirections(payload, card) {
  const seen = new Set();
  const output = [];
  for (const { item, source_field: sourceField } of supportGuidanceItems(payload, card)) {
    const source = objectOf(item);
    const direction = supportGuidanceText(item);
    if (!direction) continue;
    const key = direction.replace(/\s+/gu, " ").trim();
    if (seen.has(key)) continue;
    seen.add(key);
    output.push(pruneEmpty({
      direction,
      action_type: text(source.action_type || source.type || source.lane || source.category) || inferredInvestigationActionType(direction),
      target: text(source.target || source.object || source.subject || source.account || source.counterparty) || undefined,
      evidence_basis: text(source.evidence_basis || source.basis || source.fact_basis || source.source_basis)
        || `来源字段：${sourceField}`,
      boundary: text(source.boundary || source.stop_condition || source.condition)
        || "仅作为下一步核查方向；未补齐证据前不得升级为已查明事实。"
    }) || {});
    if (output.length >= 8) break;
  }
  return output;
}

function supportCardEvidence(card) {
  const selected = {};
  for (const key of SUPPORT_CARD_EVIDENCE_KEYS) {
    if (!Object.prototype.hasOwnProperty.call(card, key)) continue;
    if (key === "facts") {
      selected[key] = diagnosticFactsEvidence(card[key]);
      continue;
    }
    if (key === "supported_edges") {
      selected[key] = arrayOf(card[key]).map(objectOf).filter((row) => (
        isSupportedStatus(row.edge_status || row.support_status)
        && optionalMoneyNumber(row.amount) !== undefined
      ));
      continue;
    }
    selected[key] = card[key];
  }
  return supportEvidenceValue(selected) || {};
}

function publicPairAmountSupportKey(key) {
  return ({
    focus_cluster_effective_share: "major_concentrated_transfer_share",
    outside_focus_cluster_effective_amount: "other_verified_transfer_amount",
    outside_focus_cluster_effective_count: "other_verified_transfer_count",
    focus_cluster: "major_concentrated_transfer",
    focus_cluster_role: "major_concentrated_transfer_role",
    account_clusters: "receiver_account_groups",
    source_account_clusters: "payer_account_groups",
    date_counterparty_clusters: "date_receiver_account_groups",
    time_clusters: "time_concentrations",
    focus_cluster_transactions: "major_concentrated_transfer_rows"
  })[key] || key;
}

function diagnosticFactsEvidence(value) {
  const rows = arrayOf(value).map((fact) => {
    const source = objectOf(fact);
    const amountKeyed = Object.prototype.hasOwnProperty.call(source, "amount");
    const countKeyed = Object.prototype.hasOwnProperty.call(source, "count");
    const amount = amountKeyed ? optionalMoneyNumber(source.amount) : undefined;
    const count = countKeyed ? optionalCount(source.count) : undefined;
    const numericFieldsInvalid = (!amountKeyed && !countKeyed)
      || (amountKeyed && amount === undefined)
      || (countKeyed && count === undefined);
    const declaredStatus = text(source.support_status || source.supportStatus);
    return pruneEmpty({
      fact_id: text(source.fact_id || source.factId),
      label: text(source.label),
      statement: numericFieldsInvalid ? undefined : text(source.answer_text || source.answerText),
      holder: text(source.holder),
      counterparty: text(source.counterparty),
      amount,
      count,
      support_status: numericFieldsInvalid ? "needs_evidence" : declaredStatus,
      time_range: text(source.time_range || source.timeRange)
    }) || {};
  }).filter((fact) => Object.keys(fact).length);
  return rows.length ? rows : undefined;
}

function scalarSupportFacts(card) {
  const facts = supportCardEvidence(card);
  const output = {};
  for (const [key, value] of Object.entries(facts)) {
    if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
      output[publicPairAmountSupportKey(key)] = value;
    }
  }
  return output;
}

function amountSum(rows) {
  const values = arrayOf(rows)
    .map((row) => optionalMoneyNumber(objectOf(row).amount))
    .filter((value) => value !== undefined);
  if (!values.length) return undefined;
  return Number(values.reduce((sum, value) => sum + value, 0).toFixed(2));
}

function rowTime(row) {
  const source = objectOf(row);
  return text(source.txn_time || source.trade_time || source.time || source.date);
}

function rowDate(row) {
  return rowTime(row).slice(0, 10);
}

function rowCueText(row) {
  const source = objectOf(row);
  return uniqueTexts([
    source.summary,
    source.remark,
    source.txn_type,
    source.business_category_label,
    source.business_category
  ].map(text).filter(Boolean), 5).join(" / ");
}

function compactRowForReading(row) {
  const source = objectOf(row);
  return pruneEmpty({
    txn_time: rowTime(source),
    payer_account: text(source.payer_account),
    receiver_account: text(source.receiver_account || source.counterparty_account || source.counterparty_acct),
    amount: optionalMoneyNumber(source.amount),
    cue: rowCueText(source) || undefined
  });
}

const ROW_CUE_RULES = [
  { label: "还款/借贷用途线索", pattern: /还款|借款|借贷|贷款|本息|利息/u },
  { label: "人行/跨行汇款渠道线索", pattern: /人行|汇款|跨行|IBPS|ibps|C20002001S/u },
  { label: "终端/批处理渠道线索", pattern: /自助终端|终端|联机批处理|批处理|批量/u },
  { label: "金融产品线索", pattern: /理财|基金|证券|申购|赎回|产品|保险/u },
  { label: "现金线索", pattern: /现金|取现|ATM/u },
  { label: "平台/商户线索", pattern: /支付宝|微信|财付通|商户|平台|POS/u }
];

function rowCueLabels(row) {
  const cue = rowCueText(row);
  return ROW_CUE_RULES.filter((rule) => rule.pattern.test(cue)).map((rule) => rule.label);
}

function summarizeRowCueBuckets(rows, limit = 8) {
  const buckets = new Map();
  for (const row of arrayOf(rows)) {
    const labels = rowCueLabels(row);
    for (const label of labels) {
      const current = buckets.get(label) || {
        label,
        count: 0,
        amount_sum: 0,
        amount_aggregation_valid: true,
        amount_valid_count: 0,
        amount_missing_count: 0,
        examples: []
      };
      current.count += 1;
      const amount = optionalMoneyNumber(objectOf(row).amount);
      if (amount !== undefined) {
        const nextAmountSum = current.amount_sum + amount;
        if (current.amount_aggregation_valid && Number.isFinite(nextAmountSum)) {
          current.amount_sum = nextAmountSum;
        } else {
          current.amount_aggregation_valid = false;
        }
        current.amount_valid_count += 1;
      } else {
        current.amount_missing_count += 1;
      }
      if (current.examples.length < 3) current.examples.push(compactRowForReading(row));
      buckets.set(label, current);
    }
  }
  return [...buckets.values()]
    .map((item) => ({
      label: item.label,
      count: item.count,
      amount: item.amount_missing_count === 0 && item.amount_aggregation_valid
        ? Number(item.amount_sum.toFixed(2))
        : undefined,
      coverage_status: item.amount_missing_count > 0
        ? "partial_missing_amount"
        : (item.amount_aggregation_valid ? "complete" : "invalid_non_finite_total"),
      amount_valid_count: item.amount_valid_count,
      amount_missing_count: item.amount_missing_count,
      examples: item.examples
    }))
    .sort((a, b) => Number(b.coverage_status === "complete") - Number(a.coverage_status === "complete")
      || (numberOrUndefined(b.amount) ?? 0) - (numberOrUndefined(a.amount) ?? 0)
      || (optionalCount(b.count) ?? 0) - (optionalCount(a.count) ?? 0))
    .slice(0, limit)
    .map((item) => pruneEmpty(item));
}

function summarizeLowValueRows(rows) {
  const lowRows = arrayOf(rows).filter((row) => {
    const amount = optionalMoneyNumber(objectOf(row).amount);
    return amount !== undefined && amount > 0 && amount <= 100;
  });
  if (!lowRows.length) return undefined;
  return pruneEmpty({
    count: lowRows.length,
    amount: amountSum(lowRows),
    examples: lowRows.slice(0, 3).map(compactRowForReading),
    interpretation_required: "低额行仍属于当前明细表的一部分，用于固定首末时间、终端/渠道线索或重复差异边界；不得因为金额小而从20笔以内表格中删除。"
  });
}

function compactPairAmountSupportFacts(card, { transactionLimit = 20, clusterLimit = 6 } = {}) {
  const source = objectOf(card);
  if (text(source.card_type) !== "pair_amount_review_card") return supportCardEvidence(source);
  const effectiveCount = firstOptionalValue(optionalCount,
    source.full_period_effective_count,
    source.high_confidence_same_fact_count);
  const returnedTopTransactions = arrayOf(source.top_transactions).slice(0, transactionLimit);
  const focusCluster = objectOf(source.focus_cluster);
  const focusShare = numberOrUndefined(focusCluster.share_of_effective_amount);
  const focusDate = text(focusCluster.date || text(focusCluster.first_txn_at).slice(0, 10));
  const receiverName = text(source.via_holder_name || source.counterparty_name || source.receiver_name);
  const focusReceiverAccount = text(focusCluster.counterparty_acct || focusCluster.receiver_account);
  const fullAmount = firstOptionalValue(optionalMoneyNumber,
    source.full_period_effective_amount,
    source.high_confidence_same_fact_effective_amount);
  const focusAmount = optionalMoneyNumber(focusCluster.effective_amount);
  const focusCount = optionalCount(focusCluster.effective_count);
  const outsideAmountKeyed = Object.prototype.hasOwnProperty.call(source, "outside_focus_cluster_effective_amount");
  const outsideCountKeyed = Object.prototype.hasOwnProperty.call(source, "outside_focus_cluster_effective_count");
  const outsideFocusAmount = fullAmount !== undefined && focusAmount !== undefined
    ? (outsideAmountKeyed
        ? optionalNonNegativeMoney(source.outside_focus_cluster_effective_amount)
        : optionalNonNegativeMoney(Number((fullAmount - focusAmount).toFixed(2))))
    : undefined;
  const outsideFocusCount = effectiveCount !== undefined && focusCount !== undefined
    ? (outsideCountKeyed
        ? optionalCount(source.outside_focus_cluster_effective_count)
        : optionalCount(effectiveCount - focusCount))
    : undefined;
  const sourceAccounts = arrayOf(source.source_accounts).slice(0, 20);
  const counterpartyAccounts = arrayOf(source.counterparty_accounts).slice(0, 20);
  const focusReceiverAccountNorm = focusReceiverAccount.replace(/\D/gu, "");
  const outsideFocusTransactions = returnedTopTransactions.filter((row) => {
    const rowTime = text(row.txn_time || row.trade_time || row.time || row.date);
    const rowReceiverAccount = text(row.receiver_account || row.counterparty_account || row.counterparty_acct || row.to_account).replace(/\D/gu, "");
    const sameFocusDate = focusDate && rowTime.startsWith(focusDate);
    const sameFocusReceiver = focusReceiverAccountNorm && rowReceiverAccount === focusReceiverAccountNorm;
    return !(sameFocusDate && sameFocusReceiver);
  });
  const verifiedOutsideFocusTransactions = outsideFocusTransactions
    .filter((row) => optionalMoneyNumber(objectOf(row).amount) !== undefined);
  const scalarFacts = scalarSupportFacts(source);
  delete scalarFacts.other_verified_transfer_amount;
  delete scalarFacts.other_verified_transfer_count;
  const firstTxnAt = text(source.full_period_effective_first_txn_at || source.first_txn_at);
  const lastTxnAt = text(source.full_period_effective_last_txn_at || source.last_txn_at);
  const payerName = text(source.holder_name || source.source_holder_name || source.payer_name);
  const payerAccountCount = Object.prototype.hasOwnProperty.call(source, "source_account_count")
    ? optionalCount(source.source_account_count)
    : (sourceAccounts.length || undefined);
  const receiverAccountCount = Object.prototype.hasOwnProperty.call(source, "counterparty_account_count")
    ? optionalCount(source.counterparty_account_count)
    : (counterpartyAccounts.length || undefined);
  return pruneEmpty({
    ...scalarFacts,
    pair_amount_fact_pack: supportEvidenceValue({
      payer_name: payerName || undefined,
      receiver_name: receiverName || undefined,
      first_txn_at: firstTxnAt || undefined,
      last_txn_at: lastTxnAt || undefined,
      payer_account_count: payerAccountCount,
      payer_accounts: sourceAccounts,
      receiver_account_count: receiverAccountCount,
      receiver_accounts: counterpartyAccounts,
      effective_amount: fullAmount,
      effective_count: effectiveCount,
      raw_detail_amount: optionalMoneyNumber(source.raw_detail_amount),
      raw_detail_count: optionalCount(source.raw_detail_count),
      duplicate_or_unsupported_amount: optionalMoneyNumber(source.duplicate_or_unsupported_amount),
      confirmed_same_fact_duplicate_amount: optionalMoneyNumber(source.confirmed_same_fact_duplicate_amount),
      confirmed_same_fact_duplicate_count: optionalCount(source.confirmed_same_fact_duplicate_count),
      unresolved_duplicate_or_unsupported_amount: optionalMoneyNumber(source.unresolved_duplicate_or_unsupported_amount),
      duplicate_review_status: text(source.duplicate_review_status) || undefined,
      duplicate_review_explanation: text(source.duplicate_review_explanation) || undefined,
      other_verified_transfer_amount: outsideFocusAmount,
      other_verified_transfer_count: outsideFocusCount,
      row_cue_buckets: summarizeRowCueBuckets(returnedTopTransactions),
      low_value_rows: summarizeLowValueRows(returnedTopTransactions)
    }),
    pair_amount_structure_review: supportEvidenceValue({
      full_pair: {
        first_txn_at: text(source.full_period_effective_first_txn_at || source.first_txn_at),
        last_txn_at: text(source.full_period_effective_last_txn_at || source.last_txn_at),
        effective_amount: fullAmount,
        effective_count: effectiveCount,
        payer_account_count: optionalCount(source.source_account_count),
        receiver_account_count: optionalCount(source.counterparty_account_count)
      },
      major_concentrated_transfer: focusAmount !== undefined && focusCount !== undefined ? {
        date: focusDate || undefined,
        receiver_account: focusReceiverAccount || undefined,
        first_txn_at: text(focusCluster.first_txn_at),
        last_txn_at: text(focusCluster.last_txn_at),
        effective_amount: focusAmount,
        effective_count: focusCount,
        share_of_effective_amount: focusShare,
        role: "异常集中转移线索"
      } : undefined,
      other_verified_transfers: outsideFocusAmount !== undefined
        && outsideFocusCount !== undefined
        && (outsideFocusAmount !== 0 || outsideFocusCount !== 0) ? {
        effective_amount: outsideFocusAmount,
        effective_count: outsideFocusCount,
        returned_row_count: verifiedOutsideFocusTransactions.length || undefined,
        row_cue_buckets: summarizeRowCueBuckets(verifiedOutsideFocusTransactions)
      } : undefined,
      duplicate_review: {
        raw_detail_amount: optionalMoneyNumber(source.raw_detail_amount),
        raw_detail_count: optionalCount(source.raw_detail_count),
        duplicate_or_unsupported_amount: optionalMoneyNumber(source.duplicate_or_unsupported_amount),
        confirmed_same_fact_duplicate_amount: optionalMoneyNumber(source.confirmed_same_fact_duplicate_amount),
        confirmed_same_fact_duplicate_count: optionalCount(source.confirmed_same_fact_duplicate_count),
        unresolved_duplicate_or_unsupported_amount: optionalMoneyNumber(source.unresolved_duplicate_or_unsupported_amount),
        duplicate_review_status: text(source.duplicate_review_status) || undefined,
        duplicate_review_explanation: text(source.duplicate_review_explanation) || undefined
      }
    }),
    statistical_period: supportEvidenceValue(source.statistical_period),
    source_accounts: supportEvidenceValue(sourceAccounts),
    counterparty_accounts: supportEvidenceValue(counterpartyAccounts),
    top_transactions: supportEvidenceValue(returnedTopTransactions),
    other_verified_transfer_rows: supportEvidenceValue(verifiedOutsideFocusTransactions),
    major_concentrated_transfer_rows: supportEvidenceValue(arrayOf(source.focus_cluster_transactions)
      .filter((row) => optionalMoneyNumber(objectOf(row).amount) !== undefined)
      .slice(0, Math.min(transactionLimit, 5))),
    receiver_account_groups: supportEvidenceValue(arrayOf(source.account_clusters).slice(0, clusterLimit)),
    payer_account_groups: supportEvidenceValue(arrayOf(source.source_account_clusters).slice(0, clusterLimit)),
    date_receiver_account_groups: supportEvidenceValue(arrayOf(source.date_counterparty_clusters).slice(0, clusterLimit)),
    time_concentrations: supportEvidenceValue(arrayOf(source.time_clusters).slice(0, clusterLimit)),
    duplicate_groups: supportEvidenceValue(arrayOf(source.duplicate_groups).slice(0, Math.min(clusterLimit, 4))),
    expanded_same_name_scope: supportEvidenceValue(source.expanded_same_name_scope),
    dedup_key_safety: supportEvidenceValue(source.dedup_key_safety)
  }) || scalarFacts;
}

function fallbackSupportFactsForBudget(card) {
  const source = objectOf(card);
  if (text(source.card_type) === "pair_amount_review_card") {
    return compactPairAmountSupportFacts(source, { transactionLimit: 20, clusterLimit: 3 });
  }
  return supportCardEvidence(source);
}

function pairFactReadiness(facts) {
  const source = objectOf(facts);
  const pack = objectOf(source.pair_amount_fact_pack);
  const review = objectOf(source.pair_amount_structure_review);
  const fullPair = objectOf(review.full_pair);
  const amount = firstOptionalValue(optionalMoneyNumber,
    pack.effective_amount,
    fullPair.effective_amount,
    source.full_period_effective_amount,
    source.high_confidence_same_fact_effective_amount);
  const count = firstOptionalValue(optionalCount,
    pack.effective_count,
    fullPair.effective_count,
    source.full_period_effective_count,
    source.high_confidence_same_fact_count);
  const firstTxnAt = text(pack.first_txn_at || fullPair.first_txn_at || source.first_txn_at);
  const lastTxnAt = text(pack.last_txn_at || fullPair.last_txn_at || source.last_txn_at);
  const completeRange = Boolean(firstTxnAt && lastTxnAt);
  const completeNumeric = amount !== undefined && count !== undefined;
  const zeroResult = completeNumeric && (amount === 0 || count === 0);
  return {
    amount,
    count,
    completeRange,
    completeNumeric,
    zeroResult,
    factsEnough: completeRange && completeNumeric && !zeroResult
  };
}

function hasEvidenceBearingValue(value, key = "") {
  if (/^(?:card_type|intent|fact_source|case_id|holder_name|via_holder_name|source_holder|title|summary|boundary|status|type|category|label)$/iu.test(key)) {
    return false;
  }
  const normalizedNumeric = externalNumericValue(key, value);
  if (normalizedNumeric !== null) return normalizedNumeric !== undefined;
  if (Array.isArray(value)) {
    return value.some((item) => hasEvidenceBearingValue(item));
  }
  if (value && typeof value === "object") {
    const source = objectOf(value);
    const graphEdge = Object.prototype.hasOwnProperty.call(source, "edge_status")
      && ["edge_id", "txn_id", "from", "to", "from_label", "to_label"]
        .some((field) => Object.prototype.hasOwnProperty.call(source, field));
    if (graphEdge) {
      return isSupportedStatus(source.edge_status) && optionalMoneyNumber(source.amount) !== undefined;
    }
    return Object.entries(value).some(([childKey, item]) => hasEvidenceBearingValue(item, childKey));
  }
  if (typeof value === "number") return Number.isFinite(value);
  if (typeof value === "boolean" || typeof value !== "string") return false;
  if (!value.trim()) return false;
  return /(?:name|holder|account|counterparty|date|time|period|scope|direction|status|type|category|label)(?:_|$)/iu.test(key);
}

function hasStrictNumericValue(value) {
  if (Array.isArray(value)) return value.some(hasStrictNumericValue);
  if (value && typeof value === "object") return Object.values(value).some(hasStrictNumericValue);
  return numberOrUndefined(value) !== undefined;
}

function buildSupportEvidenceEnvelope(payload) {
  const rawSource = objectOf(payload);
  const source = preserveStableBoundaryMetadata(
    rawSource,
    stripAgentOpaqueRefs(rawSource) || {}
  );
  const card = objectOf(source.answer_card);
  const supportFacts = text(card.card_type) === "pair_amount_review_card"
    ? compactPairAmountSupportFacts(card, { transactionLimit: 20, clusterLimit: 10 })
    : supportCardEvidence(card);
  const pairState = text(card.card_type) === "pair_amount_review_card"
    ? pairFactReadiness(supportFacts)
    : null;
  const exactCountState = projectExactCountEvidence(
    source,
    supportEvidenceValue(source.key_facts) || {}
  );
  const fullCaseState = fullCaseBoundaryState(source, exactCountState.compactEvidence);
  const boundaryOnly = fullCaseState.applies && fullCaseState.boundaryOnly;
  const compactEvidence = projectFullCaseBoundaryEvidence(
    exactCountState.compactEvidence,
    fullCaseState
  );
  const usableFactsPresent = hasEvidenceBearingValue(supportFacts);
  const requiredFactsPresent = card.required_facts_present === true
    && usableFactsPresent
    && (!pairState || pairState.factsEnough);
  return {
    support_only: true,
    final_answer_owned_by_focused_skill: true,
    source_map: {
      tool: text(source.tool) || undefined,
      fact_source: text(card.fact_source) || undefined,
      source_scope: supportEvidenceValue(card.source_scope),
      current_case_ref: text(card.case_id) || undefined
    },
    validation_state: {
      status: text(source.status) || "ok",
      semantic_status: fullCaseState.applies ? fullCaseState.semanticStatus : undefined,
      required_facts_present: requiredFactsPresent,
      answer_card_complete: card.answer_card_complete === true && requiredFactsPresent,
      fact_answer_allowed: exactCountState.applies || boundaryOnly ? false : undefined,
      verified_no_hit_allowed: exactCountState.applies || boundaryOnly || pairState?.zeroResult
        ? false
        : undefined,
      coverage_complete: exactCountState.applies || boundaryOnly
        ? false
        : fullCaseState.reportedCoverageComplete,
      source_coverage_complete: fullCaseState.applies
        ? fullCaseState.reportedCoverageComplete
        : undefined,
      partial_coverage: fullCaseState.partialCoverage || undefined,
      boundary_only: boundaryOnly || undefined,
      empty_result_status: fullCaseState.emptyResult ? "unverified_empty_result" : undefined,
      host_verified_no_hit_receipt_present: fullCaseState.emptyResult ? false : undefined,
      source_dataset_snapshot_id: fullCaseState.reportedDatasetSnapshotId || undefined,
      evidence_gaps: fullCaseState.evidenceGaps?.length ? fullCaseState.evidenceGaps : undefined,
      exact_count_contract_status: exactCountState.applies ? exactCountState.status : undefined,
      report_grade_blocked: card.report_grade_blocked,
      write_blocked: card.write_blocked,
      validation_state: text(card.validation_state) || undefined
    },
    support_facts: supportFacts,
    compact_evidence: compactEvidence,
    known_risks: supportRiskTexts(source, card),
    query_guidance: supportQueryGuidance(source, card),
    suggested_next_directions: supportSuggestedNextDirections(source, card),
    artifact_provenance: {
      fact_source: text(card.fact_source) || undefined,
      source_scope: supportEvidenceValue(card.source_scope)
    }
  };
}

function renderCaseSourceBlockerForAgent(card, payload = {}) {
  const source = objectOf(card);
  const blocker = Object.keys(source).length
    ? source
    : objectOf(objectOf(payload).case_source_blocker);
  const explicitCaseId = text(blocker.explicit_case_id || payload.case_id);
  const invalidExplicit = /INVALID_CASE_SOURCE/iu.test(text(blocker.error_code))
    || /explicit_case_lookup|显式案件编号核验/iu.test(text(blocker.failed_stage))
    || /指定.*案件编号.*不存在/u.test(text(blocker.reason));
  const rawReason = text(blocker.reason)
    .replace(/^.*?重试后仍未完成[：:]\s*/u, "")
    .replace(/^.*?failed after transient retry[：:]\s*/iu, "")
    .replace(/Explicit\s+Analytix\s+(?:case_id|案件编号)\s+does\s+not\s+exist[：:]?\s*/iu, "指定案件编号不存在：")
    .replace(/Unable\s+to\s+resolve\s+active\s+Analytix\s+case\s+via\s+backend\s+active-case\s+API[：:]?\s*/iu, "当前会话未定位到 analytix 案件项目工作区：")
    .replace(/Unable\s+to\s+resolve\s+analytix\s+case\s+project\s+context[：:]?\s*/iu, "当前会话未定位到 analytix 案件项目工作区：")
    .replace(/Unable\s+to\s+validate\s+analytix\s+case\s+project\s+via\s+backend\s+case\s+API[：:]?\s*/iu, "当前案件项目 DuckDB 绑定未通过校验：")
    .replace(/\.\s*Open\s+the\s+核验环节\s+from\s+an\s+analytix\s+case\s+project\s+workspace\.?/iu, "。请从 analytix 案件项目工作区发起会话。");
  const reason = invalidExplicit
    ? `指定案件编号不存在${explicitCaseId ? `：${explicitCaseId}` : ""}。`
    : (rawReason ? translateUserVisibleText(rawReason) : "当前会话未定位到 analytix 案件项目工作区。");
  const stage = invalidExplicit ? "显式案件编号核验" : "当前案件确认";
  const recoveryActions = uniqueTexts([
    "从 analytix 左侧案件项目打开目标案件项目，或确认当前会话 workspace 是该案件项目目录。",
    "确认案件项目目录内存在 .analytix/case-project.json，且其中 caseId 对应 data-analysis/cases/<caseId>/case.duckdb。",
    "数据服务暂不可用时，改用当前案件 DuckDB 工作台工具核验，例如 get_scope_coverage、count_case_rows 或 run_case_sql。",
    "如已输入案件编号，只核验该编号是否存在，不得尝试其他编号。",
    "案件项目绑定恢复后，重新执行案件事实核验；不得改用其他案件、本机散落数据库、历史输出或测试样例作答。"
  ], 6).map(translateUserVisibleText).filter(Boolean);
  const lines = [
    "研判状态: 当前证据不足",
    `当前案件来源未就绪: ${reason}`,
    `来源确认环节: ${stage}`,
    "来源链路: analytix 案件项目工作区 -> .analytix/case-project.json -> 案件隔离 DuckDB -> 案件事实核验环节"
  ];
  if (explicitCaseId) {
    lines.push(`显式案件编号: ${explicitCaseId}；该编号不存在时不得尝试其他编号。`);
  }
  if (recoveryActions.length) {
    lines.push("恢复动作:");
    recoveryActions.slice(0, 5).forEach((item) => lines.push(`- ${item}`));
  }
  lines.push("禁止动作: 不得跳到其他案件项目、不得搜索本机数据库文件、不得从历史输出或测试样例猜案件编号；不得连续尝试其他编号。");
  lines.push("核验意见: 当前只能返回案件项目来源未就绪原因和恢复动作；不能给金额、排名、资金链或报告级结论。");
  return translateUserVisibleText(lines.join("\n"));
}

function renderCurrentCaseSupportText(payload = {}) {
  const source = objectOf(payload);
  const toolName = text(source.tool || source.skill_id);
  const caseRecord = objectOf(source.case || objectOf(source.data).case);
  const isCurrentCase =
    toolName === "get_current_case" ||
    ["active_case", "case-project", "case_project"].includes(text(source.source));
  if (!isCurrentCase || !Object.keys(caseRecord).length) return "";
  const lines = ["案件事实摘录: 当前案件项目确认"];
  const caseName = text(caseRecord.case_name || caseRecord.name);
  const caseNumber = text(caseRecord.case_number || caseRecord.caseNo);
  const status = text(caseRecord.status);
  if (caseName) lines.push(`当前案件: ${caseName}`);
  if (caseNumber) lines.push(`案件编号: ${caseNumber}`);
  if (status) {
    const statusLabel = /^(ready|active|ok)$/iu.test(status)
      ? "已就绪"
      : translateUserVisibleText(status);
    pushLine(lines, `案件状态: ${statusLabel}`);
  }
  pushLine(lines, "来源边界: 当前案件身份仅用于定位本轮分析上下文；金额、账户归属和资金链路仍以确定性核验材料为准。");
  return lines.join("\n");
}

function minimalAgentCard(compact) {
  const facts = objectOf(compact.key_facts);
  return pruneEmpty({
    tool: compact.tool,
    status: compact.status,
    answer_card: compact.answer_card,
    key_facts: pruneEmpty({
      coverage: minimalCoverageFact(facts.coverage || facts.holder_coverage || {}),
      holder_scope: facts.holder_scope,
      account_key: facts.account_key,
      account_stats: facts.account_stats
        ? compactAccountStatsFact(facts.account_stats)
        : undefined,
      holder_account_stats: facts.holder_account_stats || facts.account_stats
        ? compactAccountStatsFact(facts.holder_account_stats || facts.account_stats)
        : undefined,
      top_accounts: arrayOf(facts.top_accounts).slice(0, 2),
      top_holders: arrayOf(facts.top_holders).slice(0, 2),
      top_counterparties: arrayOf(facts.top_counterparties).slice(0, 2),
      counterparty_rankings: arrayOf(facts.counterparty_rankings).slice(0, 2),
      rankings: arrayOf(facts.rankings).slice(0, 20),
      top_outflows: arrayOf(facts.top_outflows).slice(0, 2),
      mandatory_review_cards: arrayOf(facts.mandatory_review_cards).slice(0, 2),
      hypothesis_cards: arrayOf(facts.hypothesis_cards).slice(0, 2),
      flow_graph: facts.flow_graph ? { ...compactFlowGraphForText(facts.flow_graph, 2), edges: arrayOf(facts.flow_graph.edges).slice(0, 2) } : undefined,
      full_case_package: facts.full_case_package ? compactFullCaseDigest(facts.full_case_package) : undefined,
      workbench: exactCountWorkbenchSlice(compact)
    }) || undefined,
    warnings: arrayOf(compact.warnings).slice(0, 3),
    next_actions: arrayOf(compact.next_actions).slice(0, 2),
    compacted_for_agent_context: true
  }) || {};
}

function boundedSupportEnvelope(compact, textBudget) {
  const fullEnvelope = buildSupportEvidenceEnvelope(compact);
  let envelope = fullEnvelope;
  if (JSON.stringify(envelope).length <= textBudget) return envelope;
  envelope = buildSupportEvidenceEnvelope(pruneEmpty({
    tool: compact.tool,
    status: compact.status,
    answer_card: supportCardEvidence(objectOf(compact.answer_card)),
    key_facts: pruneEmpty({
      ...objectOf(compactKeyFactsForText(compact.key_facts || {})),
      workbench: exactCountWorkbenchSlice(compact)
    }),
    warnings: arrayOf(compact.warnings).slice(0, 4),
    next_actions: arrayOf(compact.next_actions).slice(0, 2)
  }) || {});
  if (JSON.stringify(envelope).length <= textBudget) return envelope;
  envelope = buildSupportEvidenceEnvelope(minimalAgentCard(compact));
  if (JSON.stringify(envelope).length <= textBudget) return envelope;
  const fallback = {
    support_only: true,
    final_answer_owned_by_focused_skill: true,
    source_map: {
      ...objectOf(fullEnvelope.source_map),
      tool: text(objectOf(fullEnvelope.source_map).tool || compact.tool) || undefined
    },
    validation_state: {
      ...objectOf(fullEnvelope.validation_state),
      status: text(objectOf(fullEnvelope.validation_state).status || compact.status) || "ok",
      support_payload_truncated: true
    },
    support_facts: fallbackSupportFactsForBudget(objectOf(compact.answer_card)),
    compact_evidence: projectExactCountEvidence(compact, {}).compactEvidence,
    known_risks: uniqueTexts([
      ...arrayOf(fullEnvelope.known_risks),
      "支持材料超过上下文预算；完整载荷未进入模型，留存状态须由宿主证据登记确认。"
    ], 8),
    query_guidance: arrayOf(fullEnvelope.query_guidance).slice(0, 6),
    suggested_next_directions: arrayOf(fullEnvelope.suggested_next_directions).slice(0, 4),
    artifact_provenance: {
      ...objectOf(fullEnvelope.artifact_provenance)
    }
  };
  if (JSON.stringify(fallback).length <= textBudget) return fallback;
  return {
    support_only: true,
    final_answer_owned_by_focused_skill: true,
    source_map: objectOf(fallback.source_map),
    validation_state: {
      ...objectOf(fallback.validation_state),
      support_payload_truncated: true
    },
    support_facts: text(objectOf(compact.answer_card).card_type) === "pair_amount_review_card"
      ? compactPairAmountSupportFacts(objectOf(compact.answer_card), { transactionLimit: 20, clusterLimit: 1 })
      : scalarSupportFacts(objectOf(compact.answer_card)),
    compact_evidence: projectExactCountEvidence(compact, {}).compactEvidence,
    known_risks: arrayOf(fallback.known_risks).slice(0, 4),
    query_guidance: arrayOf(fallback.query_guidance).slice(0, 4),
    suggested_next_directions: arrayOf(fallback.suggested_next_directions).slice(0, 3),
    artifact_provenance: objectOf(fallback.artifact_provenance)
  };
}

function moneyLabel(value) {
  const number = optionalMoneyNumber(value);
  if (number === undefined) return "";
  const yuanText = `${number.toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })} 元`;
  if (Math.abs(number) >= 10000) {
    const wanText = (number / 10000).toLocaleString("zh-CN", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    });
    return `${yuanText}（${wanText} 万元）`;
  }
  return yuanText;
}

function compactListLabel(items, limit = 8) {
  const values = arrayOf(items).map(text).filter(Boolean);
  if (!values.length) return "";
  const head = values.slice(0, limit).join("、");
  return values.length > limit ? `${head}等 ${values.length} 项` : head;
}

function ratioLabel(value) {
  const number = numberOrUndefined(value);
  if (number === undefined) return "";
  return `${(number * 100).toFixed(2)}%`;
}

function shouldPreserveSqlIdentifiers(value) {
  const raw = text(value);
  return /\b(?:SQL-safe|sql_name|sql_identifier|column_name|table_name|SELECT|FROM|WHERE|GROUP BY|ORDER BY|LIMIT|account_open_name|analysis_txn_detail_idx|counterparty_name|cp_name_pick|dc_val|txn_time|txn_ts)\b/iu.test(raw);
}

function cleanSupportLine(value) {
  const raw = text(value);
  const translated = shouldPreserveSqlIdentifiers(raw) ? raw : translateUserVisibleText(raw);
  return translated
    .replace(/\b(?:case_id|workflow|delivery_state|answer_card_complete|max_additional_tools|support_only|final_answer_owned_by_focused_skill)\b/giu, "")
    .replace(/\b(?:MCP|debug|tool|skill)\b/giu, "")
    .replace(/本卡/gu, "本次核验")
    .replace(/\s{2,}/gu, " ")
    .trim();
}

function pushLine(lines, value) {
  const line = cleanSupportLine(value);
  if (line) lines.push(line);
}

function rowAmount(row) {
  const source = objectOf(row);
  return moneyLabel(source.amount ?? source.amount_yuan ?? source.value);
}

function renderTransactionRows(rows, limit = 20) {
  const selected = arrayOf(rows).slice(0, limit).map(objectOf);
  if (!selected.length) return [];
  const lines = [
    "重点交易候选（当前流水返回，供成稿引用）:",
    "| 时间 | 付款账户 | 收款账户 | 金额 | 摘要/类型 |",
    "| --- | --- | --- | --- | --- |"
  ];
  for (const row of selected) {
    const rowText = [
      text(row.txn_time || row.trade_time || row.time || row.date) || "-",
      text(row.payer_account || row.from_account || row.account_key) || "-",
      text(row.receiver_account || row.counterparty_account || row.counterparty_acct || row.to_account) || "-",
      rowAmount(row) || "-",
      rowCueText(row) || "-"
    ].map((item) => text(item).replace(/\|/gu, "/"));
    lines.push(`| ${rowText.join(" | ")} |`);
  }
  return lines;
}

function renderCueBuckets(buckets, limit = 5) {
  return arrayOf(buckets).slice(0, limit).map((bucket) => {
    const source = objectOf(bucket);
    const label = text(source.label || source.category || source.title);
    const count = firstOptionalValue(optionalCount, source.count, source.txn_count);
    const amountValue = firstOptionalValue(optionalMoneyNumber, source.amount, source.amount_total, source.turnover_total);
    const amount = amountValue === undefined ? "" : moneyLabel(amountValue);
    if (!label) return "";
    return [label, count === undefined ? "" : `${count} 笔`, amount].filter(Boolean).join("，");
  }).filter(Boolean);
}

function countLabel(...values) {
  const count = firstOptionalValue(optionalCount, ...values);
  return count === undefined ? "" : String(count);
}

function amountLabelFrom(source, ...keys) {
  const record = objectOf(source);
  for (const key of keys) {
    if (!Object.prototype.hasOwnProperty.call(record, key)) continue;
    return moneyLabel(record[key]);
  }
  return "";
}

function periodLabelFrom(source) {
  const record = objectOf(source);
  const first = text(record.first_txn_at || record.date_min);
  const last = text(record.last_txn_at || record.date_max);
  if (first && last && first !== last) return `${first} 至 ${last}`;
  return first || last || "";
}

function cellText(value) {
  return text(value).replace(/\|/gu, "/") || "-";
}

function renderSubjectAccountRows(rows, limit = 8) {
  const selected = arrayOf(rows).slice(0, limit).map(objectOf);
  if (!selected.length) return [];
  const lines = [
    "重点账户:",
    "| 账户 | 笔数 | 流入 | 流出 | 往来 | 最大单笔 | 期间 |",
    "| --- | --- | --- | --- | --- | --- | --- |"
  ];
  for (const row of selected) {
    const account = text(row.account_display || row.account_key || row.account_no || row.card_no || row.acct_no);
    const rowText = [
      account || text(row.display_name || row.holder_name) || "-",
      countLabel(row.txn_count),
      amountLabelFrom(row, "inflow", "inflow_total", "in_amount"),
      amountLabelFrom(row, "outflow", "outflow_total", "out_amount"),
      amountLabelFrom(row, "turnover", "turnover_total"),
      amountLabelFrom(row, "max_single", "max_single_amount"),
      periodLabelFrom(row)
    ].map(cellText);
    lines.push(`| ${rowText.join(" | ")} |`);
  }
  return lines;
}

function renderSubjectCounterpartyRows(rows, limit = 8) {
  const selected = arrayOf(rows).slice(0, limit).map(objectOf);
  if (!selected.length) return [];
  const lines = [
    "重点对手方:",
    "| 对手方 | 对手账号 | 笔数 | 流入 | 流出 | 往来 | 期间 |",
    "| --- | --- | --- | --- | --- | --- | --- |"
  ];
  for (const row of selected) {
    const rowText = [
      text(row.display_name || row.counterparty_name || row.holder_name || row.counterparty_key) || "-",
      text(row.counterparty_account || row.account_display || row.account_key) || "-",
      countLabel(row.txn_count),
      amountLabelFrom(row, "inflow", "inflow_total", "in_amount"),
      amountLabelFrom(row, "outflow", "outflow_total", "out_amount"),
      amountLabelFrom(row, "turnover", "turnover_total"),
      periodLabelFrom(row)
    ].map(cellText);
    lines.push(`| ${rowText.join(" | ")} |`);
  }
  return lines;
}

function renderSubjectDossierSupportText(envelope) {
  const facts = {
    ...objectOf(envelope.support_facts),
    ...objectOf(envelope.compact_evidence)
  };
  const scope = objectOf(facts.holder_scope || facts.scope);
  const coverage = objectOf(facts.coverage || facts.holder_coverage);
  const stats = objectOf(facts.account_stats || facts.holder_account_stats);
  const accountRows = arrayOf(facts.holder_top_accounts || facts.top_accounts);
  const counterpartyRows = arrayOf(facts.counterparty_rankings || facts.top_counterparties);
  const lines = [];
  pushLine(lines, "案件事实摘录: 主体资金画像");
  const holderName = text(scope.holder_name || facts.holder_name);
  if (holderName) pushLine(lines, `研判对象: ${holderName}`);
  if (accountRows.length || counterpartyRows.length) {
    pushLine(lines, "首答材料: 本事实材料已包含账户范围、资金收支、重点账户和重点对手方；除非用户另行指定统计范围或质疑结果，应直接形成主体画像。");
  }
  const accountCount = countLabel(scope.account_count, stats.account_count, coverage.selected_account_count);
  const accountPreview = compactListLabel(scope.account_keys_preview, 12);
  if (accountCount || accountPreview) {
    const accountParts = [
      accountCount ? `已纳入核验的登记账户 ${accountCount} 个` : "已纳入核验的登记账户",
      accountPreview ? `账号 ${accountPreview}` : "",
      scope.account_keys_truncated ? "另有登记账户需结合开户资料逐号补齐" : ""
    ].filter(Boolean);
    pushLine(lines, `账户范围: ${accountParts.join("；")}`);
  }
  const leadCount = countLabel(scope.candidate_account_count);
  if (leadCount) {
    pushLine(lines, `待补证账户线索: ${leadCount} 个；需补开户资料、身份关联或交易凭证后才能认定为该主体控制账户。`);
  }
  const period = periodLabelFrom({ first_txn_at: stats.first_txn_at || coverage.date_min, last_txn_at: stats.last_txn_at || coverage.date_max });
  const txnCount = countLabel(stats.txn_count, coverage.txn_analyzed, coverage.txn_total);
  if (period || txnCount) {
    pushLine(lines, `统计期间: ${period || "当前材料尚未固定起止时间"}；交易 ${txnCount || "尚未固定"} 笔`);
  }
  const scaleFacts = [
    amountLabelFrom(stats, "inflow") ? `流入 ${amountLabelFrom(stats, "inflow")}` : "",
    amountLabelFrom(stats, "outflow") ? `流出 ${amountLabelFrom(stats, "outflow")}` : "",
    amountLabelFrom(stats, "turnover") ? `往来 ${amountLabelFrom(stats, "turnover")}` : "",
    amountLabelFrom(stats, "net_flow") ? `净额 ${amountLabelFrom(stats, "net_flow")}` : "",
    amountLabelFrom(stats, "max_single") ? `最大单笔 ${amountLabelFrom(stats, "max_single")}` : ""
  ].filter(Boolean);
  if (scaleFacts.length) {
    pushLine(lines, `资金规模: ${scaleFacts.join("；")}`);
  }
  for (const line of renderSubjectAccountRows(accountRows, 8)) {
    pushLine(lines, line);
  }
  for (const line of renderSubjectCounterpartyRows(counterpartyRows, 8)) {
    pushLine(lines, line);
  }
  const featureClues = [
    amountLabelFrom(stats, "max_single") ? `存在大额单笔交易，最大单笔 ${amountLabelFrom(stats, "max_single")}` : "",
    accountRows.length ? "重点账户之间金额差异明显，需结合入账来源、出账去向和余额承接判断账户角色。" : "",
    counterpartyRows.length ? "重点对手方可作为追踪资金来源、去向和关系强度的优先核查对象。" : ""
  ].filter(Boolean);
  if (featureClues.length) {
    pushLine(lines, `异常特征: ${featureClues.join("；")}`);
  }
  pushLine(lines, "核验意见: 已登记账户可作为本次画像基础；当前材料尚未取得或尚未固定的身份、联系方式、用途、产品、余额承接和最终受益人信息，只能列为证据缺口或待补证明事项。");
  pushLine(lines, "事实用途: 账户范围、流入流出、重点账户和重点对手方已具备首答材料；普通主体画像可直接组织经侦研判，不再改用本地文件、历史材料或另行自定义核算补事实。");
  pushLine(lines, "表述边界: 避免内部摘要、内部摘录、轮次、返回或展开类表述；缺口统一写“当前材料尚未取得”“当前材料尚未固定”或“需补证”。重点对手方明细不足时，不重复排行，写成证据缺口和补证对象。");
  pushLine(lines, "固定缺口写法: 账号未完整列明时写“另有登记账户需结合开户资料逐号补齐”；重点对手方未完整固定时写“当前材料尚未固定重点对手方明细”。");
  pushLine(lines, "需补材料: 开户资料、银行回单、收付款双方流水、用途凭证、产品申赎确认、余额承接记录和与重点对手方的身份关系材料。");
  const risks = arrayOf(envelope.known_risks).map(cleanSupportLine).filter(Boolean).slice(0, 6);
  if (risks.length) {
    pushLine(lines, `边界与风险: ${risks.join("；")}`);
  }
  return lines.join("\n");
}

function renderSubjectReadyGuardText(envelope) {
  const facts = {
    ...objectOf(envelope.support_facts),
    ...objectOf(envelope.compact_evidence)
  };
  const lines = [];
  pushLine(lines, "案件事实摘录: 主体画像已具备首答材料");
  const holderName = text(facts.holder_name || objectOf(facts.holder_scope).holder_name);
  if (holderName) pushLine(lines, `研判对象: ${holderName}`);
  pushLine(lines, text(facts.ready_reason) || "同一主体画像事实已在本轮返回。");
  pushLine(lines, text(facts.stop_reason) || "账户范围、资金收支、重点账户和重点对手方已具备首答材料。");
  pushLine(lines, "事实用途: 前序主体画像和两方金额事实可支撑经侦研判；用户未挑战统计范围或未明确要求自定义核算时，不再重复画像核验。");
  const risks = arrayOf(envelope.known_risks).map(cleanSupportLine).filter(Boolean).slice(0, 4);
  if (risks.length) {
    pushLine(lines, `边界与风险: ${risks.join("；")}`);
  }
  return lines.join("\n");
}

function compactFullCaseDigest(fullCasePackage) {
  const pack = objectOf(fullCasePackage);
  if (!Object.keys(pack).length) return {};
  return pruneEmpty({
    package_type: pack.package_type,
    semantic_status: pack.semantic_status,
    coverage_status: pack.coverage_status,
    source_coverage_complete: pack.source_coverage_complete,
    partial_coverage: pack.partial_coverage,
    source_dataset_snapshot_id: pack.source_dataset_snapshot_id,
    coverage_complete: pack.coverage_complete,
    fact_answer_allowed: pack.fact_answer_allowed,
    verified_no_hit_allowed: pack.verified_no_hit_allowed,
    empty_result_status: pack.empty_result_status,
    boundary_only: pack.boundary_only,
    coverage: pack.coverage,
    scope_stats: pack.scope_stats,
    field_quality: pack.field_quality,
    candidate_accounts: pack.candidate_accounts,
    top_accounts: arrayOf(pack.top_accounts).slice(0, 5),
    top_holders: arrayOf(pack.top_holders).slice(0, 5),
    top_destinations: arrayOf(pack.top_destinations).slice(0, 5),
    top_sources: arrayOf(pack.top_sources).slice(0, 5),
    top_outflows: arrayOf(pack.top_outflows).slice(0, 5),
    report_artifact: objectOf(pack.report_artifact),
    report_path: text(pack.report_path),
    manifest_path: text(pack.manifest_path),
    detector_coverage: arrayOf(pack.detector_coverage).slice(0, 20),
    hypothesis_cards: arrayOf(pack.hypothesis_cards).slice(0, 5),
    mandatory_review_cards: arrayOf(pack.mandatory_review_cards).slice(0, 5),
    evidence_gaps: arrayOf(pack.evidence_gaps).slice(0, 6),
    next_actions: arrayOf(pack.next_actions).slice(0, 6),
    analysis_outline: arrayOf(pack.analysis_outline).slice(0, 10),
    report_validation_status: pack.report_validation_status
  }) || {};
}

function renderFullCaseRows(title, rows, limit = 5) {
  const selected = arrayOf(rows).slice(0, limit);
  if (!selected.length) return [];
  const lines = [`${title}:`];
  for (const row of selected) {
    const item = objectOf(row);
    const name = text(item.display_name || item.name || item.holder_name || item.account_display || item.account_key || item.counterparty_name || item.counterparty_key) || "待核对象";
    const amount = amountLabelFrom(item, "turnover")
      || amountLabelFrom(item, "amount")
      || amountLabelFrom(item, "amount_yuan");
    const txnCount = countLabel(item.txn_count);
    const parts = [
      item.rank ? `第${item.rank}` : "",
      name,
      amount,
      amountLabelFrom(item, "inflow") ? `入账 ${amountLabelFrom(item, "inflow")}` : "",
      amountLabelFrom(item, "outflow") ? `出账 ${amountLabelFrom(item, "outflow")}` : "",
      txnCount ? `${txnCount} 笔` : "",
      periodLabelFrom(item)
    ].filter(Boolean);
    if (parts.length) lines.push(`- ${parts.join("，")}`);
  }
  return lines;
}

function fullCaseCardLabel(value) {
  const raw = text(value);
  if (/subject_top_outflow/iu.test(raw)) return "主体大额出账线索";
  if (/missing_business_correction/iu.test(raw)) return "缺失主体/对手方业务分类线索";
  if (/duplicate|same_fact/iu.test(raw)) return "同事实/重复放大风险";
  if (/cash/iu.test(raw)) return "现金断点线索";
  if (/financial|investment/iu.test(raw)) return "理财/资产端线索";
  return translateUserVisibleText(raw);
}

function cleanFullCaseText(value) {
  return translateUserVisibleText(cleanSupportLine(value))
    .replace(/主体账户集合|账户组/gu, "相关账户")
    .replace(/收款端/gu, "收款账户或入账环节")
    .replace(/clean_duplicate\s*=\s*0/giu, "清洗标记不能单独证明无重复")
    .replace(/clean_duplicate/giu, "清洗标记")
    .replace(/规范明细/gu, "已清洗流水")
    .replace(/交易明细/gu, "流水记录")
    .replace(/账户维表/gu, "开户账户清单")
    .replace(/字段覆盖/gu, "信息覆盖")
    .replace(/原始导入行数/gu, "导入记录数")
    .replace(/\bfinancial_product\b/giu, "理财/金融产品")
    .replace(/\btransfer_unclassified\b/giu, "普通转账/未分类")
    .replace(/\bcounterparty\b/giu, "对手方")
    .replace(/\bholder\b/giu, "主体")
    .replace(/\bprobe_type\s*=\s*['"]?investigative_patterns['"]?/giu, "专题线索核验")
    .replace(/\bAgent\b/gu, "研判人员")
    .replace(/种子交易号/gu, "线索交易号")
    .replace(/业务分类\s+/gu, "业务分类为")
    .replace(/\bTop\b/gu, "重点")
    .replace(/研判人员\s+提出新的侦查词、人名、通道或业务假设时，用\s+专题线索核验\s+做下一轮确定性验证。/gu, "围绕新的侦查词、人名、通道或业务假设继续做确定性核验。")
    .replace(/重点\s+出账/gu, "重点出账");
}

function renderFullCaseCards(title, rows, limit = 6) {
  const selected = arrayOf(rows).slice(0, limit);
  if (!selected.length) return [];
  const lines = [`${title}:`];
  for (const row of selected) {
    const item = objectOf(row);
    const label = fullCaseCardLabel(item.title || item.label || item.category || item.card_type) || "待核线索";
    const amount = amountLabelFrom(item, "turnover", "outflow", "inflow", "max_single");
    const txnCount = countLabel(item.txn_count);
    const accountCount = countLabel(item.account_count);
    const parts = [
      label,
      txnCount ? `${txnCount} 笔` : "",
      accountCount ? `${accountCount} 个账户` : "",
      /^0(?:\.00)?\s*元/u.test(amount) ? "" : amount,
      cleanFullCaseText(item.evidence_statement || item.claim_guard)
    ].filter(Boolean);
    if (parts.length) lines.push(`- ${parts.join("，")}`);
  }
  return lines;
}

function fullCaseActionText(value) {
  if (value == null) return "";
  if (typeof value !== "object") return cleanSupportLine(value);
  const source = objectOf(value);
  return cleanSupportLine(
    source.reason
      || source.text
      || source.label
      || source.action
      || source.summary
      || source.target_name
      || source.target
  );
}

function nonZeroCountLabel(...values) {
  for (const value of values) {
    const count = optionalCount(value);
    if (count !== undefined && count !== 0) return String(count);
  }
  return "";
}

function nonZeroAmountLabelFrom(source, ...keys) {
  const record = objectOf(source);
  for (const key of keys) {
    const value = optionalMoneyNumber(record[key]);
    if (value !== undefined && value !== 0) return moneyLabel(value);
  }
  return "";
}

function renderBoundaryFullCaseRows(title, rows, limit = 5) {
  const selected = arrayOf(rows).slice(0, limit);
  if (!selected.length) return [];
  const lines = [`${title}:`];
  for (const row of selected) {
    const item = objectOf(row);
    const name = text(
      item.display_name
        || item.name
        || item.holder_name
        || item.account_display
        || item.account_key
        || item.counterparty_name
        || item.counterparty_key
    ) || "待核对象";
    const amount = nonZeroAmountLabelFrom(item, "turnover", "amount", "amount_yuan");
    const inflow = nonZeroAmountLabelFrom(item, "inflow");
    const outflow = nonZeroAmountLabelFrom(item, "outflow");
    const txnCount = nonZeroCountLabel(item.txn_count);
    const parts = [
      item.rank ? `第${item.rank}` : "",
      name,
      amount,
      inflow ? `入账 ${inflow}` : "",
      outflow ? `出账 ${outflow}` : "",
      txnCount ? `${txnCount} 笔` : "",
      periodLabelFrom(item)
    ].filter(Boolean);
    if (parts.length) lines.push(`- ${parts.join("，")}`);
  }
  return lines;
}

function renderFullCaseBoundaryText(envelope, state) {
  const pack = objectOf(state.pack);
  const coverage = objectOf(pack.coverage);
  const scope = objectOf(pack.scope_stats);
  const lines = [state.emptyResult
    ? "案件核验边界: 未验证空结果"
    : "案件核验边界: 部分范围资金材料"];
  if (state.semanticStatus) {
    lines.push(`语义状态: ${state.semanticStatus}`);
  }
  if (state.emptyResult) {
    pushLine(lines, "核验结果: 当前工具结果未返回可验证的案件事实字段。");
    pushLine(lines, "未验证空结果: 当前返回了空集合或显式零值；没有宿主签发且在权威 registry 中有效的 VerifiedNoHit Receipt，不得解释为全案零笔、零账户、零金额、不存在、无关联或无异常。");
  }
  if (state.partialCoverage) {
    pushLine(lines, "覆盖完整性: 范围不完整；当前结果只能作为边界输出，不得升级为全案事实或正式报告结论。");
  } else {
    pushLine(lines, "覆盖完整性: 来源自报的完整性不能代替宿主 VerifiedNoHit Receipt。");
  }
  if (state.reportedDatasetSnapshotId) {
    lines.push(`来源报告的数据快照: ${state.reportedDatasetSnapshotId}（仅作范围诊断，不构成宿主授权）`);
  }
  if (state.partialCoverage && !state.emptyResult) {
    const period = periodLabelFrom({
      first_txn_at: scope.first_txn_at || coverage.date_min,
      last_txn_at: scope.last_txn_at || coverage.date_max
    });
    const txnCount = nonZeroCountLabel(scope.txn_count, coverage.txn_analyzed, coverage.txn_total);
    const accountCount = nonZeroCountLabel(scope.account_count, coverage.account_count);
    const counterpartyCount = nonZeroCountLabel(scope.counterparty_count);
    const returnedScopeParts = [
      "已返回范围",
      period ? `期间 ${period}` : "",
      txnCount ? `${txnCount} 笔` : "",
      accountCount ? `${accountCount} 个账户` : "",
      counterpartyCount ? `${counterpartyCount} 个对手方` : ""
    ].filter(Boolean);
    if (returnedScopeParts.length > 1) pushLine(lines, returnedScopeParts.join("；"));
    const inflow = nonZeroAmountLabelFrom(scope, "in_amount") || nonZeroAmountLabelFrom(coverage, "inflow_total");
    const outflow = nonZeroAmountLabelFrom(scope, "out_amount") || nonZeroAmountLabelFrom(coverage, "outflow_total");
    const turnover = nonZeroAmountLabelFrom(scope, "turnover") || nonZeroAmountLabelFrom(coverage, "turnover_total");
    const returnedAmountParts = [
      "已返回金额",
      inflow ? `入账 ${inflow}` : "",
      outflow ? `出账 ${outflow}` : "",
      turnover ? `往来总额 ${turnover}` : ""
    ].filter(Boolean);
    if (returnedAmountParts.length > 1) pushLine(lines, returnedAmountParts.join("；"));
    for (const line of renderBoundaryFullCaseRows("部分范围重点账户", pack.top_accounts, 5)) pushLine(lines, line);
    for (const line of renderBoundaryFullCaseRows("部分范围重点主体", pack.top_holders, 5)) pushLine(lines, line);
    for (const line of renderBoundaryFullCaseRows("部分范围出账去向", pack.top_destinations, 5)) pushLine(lines, line);
  }
  const gaps = arrayOf(state.evidenceGaps).map(fullCaseActionText).filter(Boolean).slice(0, 6);
  if (gaps.length) pushLine(lines, `证据缺口: ${gaps.join("；")}`);
  const risks = arrayOf(envelope.known_risks).map(cleanFullCaseText).filter(Boolean).slice(0, 6);
  if (risks.length) pushLine(lines, `边界与风险: ${risks.join("；")}`);
  pushLine(lines, "核验意见: fact_answer_allowed=false；只能说明本轮已查范围、缺失数据和补证建议，不得生成全案事实或正式报告。");
  return lines.join("\n");
}

function renderFullCaseSupportText(envelope) {
  const facts = {
    ...objectOf(envelope.support_facts),
    ...objectOf(envelope.compact_evidence)
  };
  const pack = compactFullCaseDigest(facts.full_case_package);
  const artifact = objectOf(
    objectOf(facts.report_artifact).report_path || objectOf(facts.report_artifact).path
      ? facts.report_artifact
      : (objectOf(objectOf(facts.workbench).report_artifact).report_path || objectOf(objectOf(facts.workbench).report_artifact).path
          ? objectOf(facts.workbench).report_artifact
          : pack.report_artifact)
  );
  const coverage = objectOf(pack.coverage);
  const scope = objectOf(pack.scope_stats);
  const boundaryState = fullCaseBoundaryState(envelope, facts);
  if (boundaryState.boundaryOnly) {
    return renderFullCaseBoundaryText(envelope, boundaryState);
  }
  const lines = ["案件事实材料: 全案资金研判"];
  const hasReturnedFacts = [coverage, scope]
    .some((value) => hasEvidenceBearingValue(value))
    || hasStrictNumericValue(objectOf(pack.field_quality))
    || [pack.top_accounts, pack.top_holders, pack.top_destinations, pack.top_sources, pack.top_outflows]
      .some((value) => arrayOf(value).some((row) => hasEvidenceBearingValue(row)));
  if (!hasReturnedFacts) {
    pushLine(lines, "核验结果: 当前工具结果未返回可验证的案件事实字段；不能把缺失值写成 0、未发现、无关联或全案结论。");
    const gaps = arrayOf(pack.evidence_gaps).map(fullCaseActionText).filter(Boolean).slice(0, 6);
    if (gaps.length) pushLine(lines, `证据缺口: ${gaps.join("；")}`);
    pushLine(lines, "下一步: 恢复同案数据源并取得完整范围、数据快照和宿主签发的有效证据回执后重新核验。");
    return lines.join("\n");
  }
  pushLine(lines, "材料用途: 本材料用于全案研判；用户未明确要求生成报告时，不直接出具正式报告。");
  pushLine(lines, "事实覆盖: 仅限本轮实际返回且可核验的字段；未返回或部分覆盖事项写证据缺口，不得升级为全案结论。");
  const period = periodLabelFrom({
    first_txn_at: scope.first_txn_at || coverage.date_min,
    last_txn_at: scope.last_txn_at || coverage.date_max
  });
  const txnCount = countLabel(scope.txn_count, coverage.txn_analyzed, coverage.txn_total);
  const accountCount = countLabel(scope.account_count, coverage.account_count);
  const counterpartyCount = countLabel(scope.counterparty_count);
  const inflow = amountLabelFrom(scope, "in_amount") || amountLabelFrom(coverage, "inflow_total");
  const outflow = amountLabelFrom(scope, "out_amount") || amountLabelFrom(coverage, "outflow_total");
  const turnover = amountLabelFrom(scope, "turnover") || amountLabelFrom(coverage, "turnover_total");
  pushLine(lines, [
    "全案范围",
    period ? `期间 ${period}` : "",
    txnCount ? `${txnCount} 笔` : "",
    accountCount ? `${accountCount} 个账户` : "",
    counterpartyCount ? `${counterpartyCount} 个对手方` : ""
  ].filter(Boolean).join("；"));
  pushLine(lines, [
    "资金总量",
    inflow ? `入账 ${inflow}` : "",
    outflow ? `出账 ${outflow}` : "",
    turnover ? `往来总额 ${turnover}` : "",
    countLabel(scope.in_txn_count) ? `入账 ${countLabel(scope.in_txn_count)} 笔` : "",
    countLabel(scope.out_txn_count) ? `出账 ${countLabel(scope.out_txn_count)} 笔` : ""
  ].filter(Boolean).join("；"));
  const fieldQuality = objectOf(pack.field_quality);
  const qualityBits = [
    ratioLabel(fieldQuality.counterparty) ? `对手方信息覆盖约 ${ratioLabel(fieldQuality.counterparty)}` : "",
    ratioLabel(fieldQuality.ip_addr) ? `IP 覆盖约 ${ratioLabel(fieldQuality.ip_addr)}` : "",
    ratioLabel(fieldQuality.mac_addr) ? `MAC 覆盖约 ${ratioLabel(fieldQuality.mac_addr)}` : "",
    ratioLabel(fieldQuality.cash_flag) ? `现金标记覆盖约 ${ratioLabel(fieldQuality.cash_flag)}` : ""
  ].filter(Boolean);
  if (qualityBits.length) pushLine(lines, `数据质量: ${qualityBits.join("；")}`);
  for (const line of renderFullCaseRows("重点账户", pack.top_accounts, 5)) pushLine(lines, line);
  for (const line of renderFullCaseRows("重点主体", pack.top_holders, 5)) pushLine(lines, line);
  for (const line of renderFullCaseRows("主要资金流向表（出账去向）", pack.top_destinations, 5)) pushLine(lines, line);
  if (text(artifact.report_path || artifact.path)) {
    const files = arrayOf(artifact.files).map(objectOf).filter((file) => text(file.path));
    pushLine(lines, [
      "报告交付",
      `报告路径 ${text(artifact.report_path || artifact.path)}`,
      text(artifact.manifest_path) ? `清单路径 ${text(artifact.manifest_path)}` : "",
      text(artifact.inspection_status) ? `检查状态 ${text(artifact.inspection_status)}` : ""
    ].filter(Boolean).join("；"));
    for (const file of files.slice(0, 2)) {
      pushLine(lines, [
        "文件检查",
        text(file.path),
        optionalCount(file.size) ? `size=${optionalCount(file.size)}` : "",
        text(file.sha256) ? `sha256=${text(file.sha256).slice(0, 16)}` : "",
        text(file.inspection_status) ? `status=${text(file.inspection_status)}` : ""
      ].filter(Boolean).join("；"));
    }
  }
  const detectors = new Set(arrayOf(pack.detector_coverage).map(text));
  const lanes = [
    detectors.has("cash_counter_service") ? "现金同存同取/柜面现金线索" : "",
    detectors.has("investment_clues") ? "理财/证券/基金等资产端线索" : "",
    detectors.has("time_amount_structuring") ? "时间金额集中或拆分特征" : "",
    detectors.has("group_flow") || detectors.has("internal_seed_flow") ? "资金链路与接续流向线索" : "",
    detectors.has("source_destination_top10") ? "来源去向 Top 对手方" : "",
    detectors.has("case_reconciliation") ? "数据一致性核验" : ""
  ].filter(Boolean);
  if (lanes.length) pushLine(lines, `已覆盖分析道: ${lanes.join("；")}`);
  const cards = [
    ...arrayOf(pack.mandatory_review_cards),
    ...arrayOf(pack.hypothesis_cards)
  ];
  for (const line of renderFullCaseCards("异常特征与专题线索", cards, 6)) pushLine(lines, line);
  const gaps = arrayOf(pack.evidence_gaps).map(cleanFullCaseText).filter(Boolean).slice(0, 6);
  if (gaps.length) pushLine(lines, `证据缺口: ${gaps.join("；")}`);
  const risks = arrayOf(envelope.known_risks).map(cleanFullCaseText).filter(Boolean).slice(0, 6);
  if (risks.length) pushLine(lines, `边界与风险: ${risks.join("；")}`);
  const nextActions = arrayOf(pack.next_actions).map((item) => cleanFullCaseText(fullCaseActionText(item))).filter(Boolean).slice(0, 6);
  if (nextActions.length) pushLine(lines, `下一步核查建议（续调清单）: ${nextActions.join("；")}`);
  else pushLine(lines, "下一步核查建议（续调清单）: 围绕大额主体、重点账户、资金链路、现金/理财/支付端点和缺失对手方继续补证。");
  pushLine(lines, "核验意见: 当前材料仅支持上列实际返回字段；未覆盖范围不得形成全案结论。异常特征和下一步核查建议应围绕事实缺口组织，候选线索写需复核或需补证，不得升级为最终定性。");
  return lines.join("\n");
}

function renderPairAmountSupportText(envelope) {
  const facts = objectOf(envelope.support_facts);
  const pack = objectOf(facts.pair_amount_fact_pack);
  const review = objectOf(facts.pair_amount_structure_review);
  const fullPair = objectOf(review.full_pair);
  const focus = objectOf(review.major_concentrated_transfer || facts.major_concentrated_transfer || facts.focus_cluster);
  const outside = objectOf(review.other_verified_transfers || facts.other_verified_transfers || facts.outside_focus_cluster);
  const duplicate = objectOf(review.duplicate_review);
  const pairState = pairFactReadiness(facts);
  const lines = [];
  pushLine(lines, "案件事实摘录: 两方转账金额核验");
  const firstTxnAt = text(pack.first_txn_at || fullPair.first_txn_at || facts.first_txn_at);
  const lastTxnAt = text(pack.last_txn_at || fullPair.last_txn_at || facts.last_txn_at);
  if (firstTxnAt || lastTxnAt) {
    pushLine(lines, `统计期间: ${firstTxnAt || "未见起始时间"} 至 ${lastTxnAt || "未见截止时间"}`);
  }
  const effectiveAmount = pairState.amount === undefined ? "" : moneyLabel(pairState.amount);
  const effectiveCount = pairState.count;
  if (effectiveAmount || effectiveCount !== undefined) {
    pushLine(lines, pairState.zeroResult
      ? `显式零值（未验证未命中）: 金额 ${effectiveAmount || "未返回金额"}；笔数 ${effectiveCount === undefined ? "未返回" : effectiveCount}；仍需完整范围和宿主回执。`
      : `已有流水支持金额: ${effectiveAmount || "未返回金额"}；笔数: ${effectiveCount === undefined ? "未返回" : effectiveCount}`);
  }
  const payerAccounts = compactListLabel(pack.payer_accounts || facts.source_accounts);
  const receiverAccounts = compactListLabel(pack.receiver_accounts || facts.counterparty_accounts);
  if (payerAccounts) pushLine(lines, `付款账户: ${payerAccounts}`);
  if (receiverAccounts) pushLine(lines, `收款账户: ${receiverAccounts}`);
  const rawAmountValue = firstOptionalValue(optionalMoneyNumber, pack.raw_detail_amount, duplicate.raw_detail_amount, facts.raw_detail_amount);
  const rawAmount = rawAmountValue === undefined ? "" : moneyLabel(rawAmountValue);
  const rawCount = firstOptionalValue(optionalCount, pack.raw_detail_count, duplicate.raw_detail_count, facts.raw_detail_count);
  const duplicateAmountValue = firstOptionalValue(optionalMoneyNumber, pack.duplicate_or_unsupported_amount, duplicate.duplicate_or_unsupported_amount, facts.duplicate_or_unsupported_amount);
  const duplicateAmount = duplicateAmountValue === undefined ? "" : moneyLabel(duplicateAmountValue);
  const confirmedDuplicateAmountValue = firstOptionalValue(optionalMoneyNumber, pack.confirmed_same_fact_duplicate_amount, duplicate.confirmed_same_fact_duplicate_amount, facts.confirmed_same_fact_duplicate_amount);
  const confirmedDuplicateAmount = confirmedDuplicateAmountValue === undefined ? "" : moneyLabel(confirmedDuplicateAmountValue);
  const confirmedDuplicateCount = firstOptionalValue(optionalCount, pack.confirmed_same_fact_duplicate_count, duplicate.confirmed_same_fact_duplicate_count, facts.confirmed_same_fact_duplicate_count);
  const unresolvedDuplicateAmountValue = firstOptionalValue(optionalMoneyNumber, pack.unresolved_duplicate_or_unsupported_amount, duplicate.unresolved_duplicate_or_unsupported_amount, facts.unresolved_duplicate_or_unsupported_amount);
  const unresolvedDuplicateAmount = unresolvedDuplicateAmountValue === undefined ? "" : moneyLabel(unresolvedDuplicateAmountValue);
  const duplicateReviewStatus = text(pack.duplicate_review_status || duplicate.duplicate_review_status || facts.duplicate_review_status);
  if (rawAmount || rawCount !== undefined || duplicateAmount) {
    if (/confirmed_same_holder_cross_account_same_fact/u.test(duplicateReviewStatus) && confirmedDuplicateAmount) {
      pushLine(lines, `初筛流水记录与复核后有效转账差异: 初筛流水记录 ${rawAmount || "未见金额"} / ${rawCount === undefined ? "未见" : rawCount} 笔；已确定不同卡号/账号重复记录 ${confirmedDuplicateAmount}${confirmedDuplicateCount === undefined ? "" : ` / ${confirmedDuplicateCount} 笔重复行`}，不作为新增转账。`);
      pushLine(lines, "确认重复记录处理: 已确认重复记录，已剔除，不作为新增转账。");
      pushLine(lines, "差异说明: 本次需同时说明依据、统计范围、已有流水支持金额、差异原因，以及当前不能认定的去向、性质、控制关系或最终用途。");
      pushLine(lines, "核验意见: 该差异原因为已确认重复记录，已剔除，不作为新增转账。");
    } else if (/mixed_confirmed_duplicate_and_unresolved_difference/u.test(duplicateReviewStatus) && confirmedDuplicateAmount) {
      pushLine(lines, `初筛流水记录与复核后有效转账差异: 初筛流水记录 ${rawAmount || "未见金额"} / ${rawCount === undefined ? "未见" : rawCount} 笔；已确定不同卡号/账号重复记录 ${confirmedDuplicateAmount}${confirmedDuplicateCount === undefined ? "" : ` / ${confirmedDuplicateCount} 笔重复行`}；另有 ${unresolvedDuplicateAmount || "未见金额"} 尚需按交易事实继续核验。`);
    } else {
      pushLine(lines, `初筛流水记录与复核后有效转账差异: 初筛流水记录 ${rawAmount || "未见金额"} / ${rawCount === undefined ? "未见" : rawCount} 笔；疑似重复或证据不足金额 ${duplicateAmount || "未见"}`);
    }
  }
  const focusAmountValue = optionalMoneyNumber(focus.effective_amount);
  const focusAmount = focusAmountValue === undefined ? "" : moneyLabel(focusAmountValue);
  const focusCount = optionalCount(focus.effective_count);
  if (focusAmount && focusCount !== undefined) {
    pushLine(lines, `主要集中转账: ${text(focus.date) || text(focus.first_txn_at).slice(0, 10) || "未返回日期"}；收款账户 ${text(focus.receiver_account) || "未返回"}；${focusAmount} / ${focusCount} 笔；占比 ${ratioLabel(focus.share_of_effective_amount) || "未返回"}`);
    pushLine(lines, `重点交易说明: 用户未明示限定该日期或该收款账号时，复核后有效转账使用完整两方金额 ${effectiveAmount || "未返回金额"} / ${effectiveCount === undefined ? "未返回" : effectiveCount} 笔；主要集中转账 ${focusAmount} 只是异常线索，不得替代完整两方总额。`);
  }
  const outsideAmountValue = firstOptionalValue(optionalMoneyNumber, pack.other_verified_transfer_amount, pack.outside_focus_cluster_effective_amount, outside.effective_amount);
  const outsideCount = firstOptionalValue(optionalCount, pack.other_verified_transfer_count, pack.outside_focus_cluster_effective_count, outside.effective_count);
  if (outsideAmountValue !== undefined && outsideCount !== undefined && (outsideAmountValue !== 0 || outsideCount !== 0)) {
    pushLine(lines, `集中交易以外逐笔往来: ${moneyLabel(outsideAmountValue)} / ${outsideCount} 笔`);
  }
  const cueBuckets = renderCueBuckets(pack.row_cue_buckets || outside.row_cue_buckets);
  if (cueBuckets.length) {
    pushLine(lines, `备注/类型线索: ${cueBuckets.join("；")}`);
  }
  if (pairState.factsEnough) {
    pushLine(lines, "事实用途: 当前完整期间、金额和笔数已返回，可用于金额核对与重点交易整理；案件意义、去向和关系仍须遵守证据边界。");
  } else {
    pushLine(lines, "事实边界: 当前期间、金额或笔数不完整，或仅有零值但没有可验证的完整范围与宿主回执；不得写成事实已足够、无往来或已核验未命中。");
  }
  pushLine(lines, "补证线索: 回单、收款侧流水、余额承接、用途材料等补证动作应单独列明；不要只把取证事项塞进“当前不能认定”。");
  pushLine(lines, "交易明细列示: 已返回交易20笔以内应逐笔列明；未返回部分写差异范围、补证目的和当前不能认定事项，不得用折叠占位行替代具体账户、时间、金额，也不得重复核验同一事实。");
  for (const line of renderTransactionRows(facts.top_transactions, 20)) {
    pushLine(lines, line);
  }
  const risks = arrayOf(envelope.known_risks).map(cleanSupportLine).filter(Boolean).slice(0, 6);
  if (risks.length) {
    pushLine(lines, `边界与风险: ${risks.join("；")}`);
  }
  return lines.join("\n");
}

function flowPeriodLabel(row) {
  const source = objectOf(row);
  const first = text(source.first_txn_at || source.txn_time);
  const last = text(source.last_txn_at || source.txn_time);
  if (first && last && first !== last) return `${first} 至 ${last}`;
  return first || last || "-";
}

function flowStatusLabel(status) {
  const value = text(status);
  if (!value || isSupportedStatus(value)) return "";
  if (/needs[_ -]?review/iu.test(value)) return "需复核";
  if (/needs[_ -]?evidence/iu.test(value)) return "需补证";
  if (/candidate/iu.test(value)) return "线索";
  if (/partial/iu.test(value)) return "部分可见";
  return "需核验";
}

function flowCueText(row) {
  const source = objectOf(row);
  const status = isSupportedStatus(source.edge_status) && optionalMoneyNumber(source.amount) === undefined
    ? "needs_evidence"
    : source.edge_status;
  return [
    text(source.summary),
    text(source.category),
    flowStatusLabel(status),
    text(source.source_boundary || source.boundary)
  ].filter(Boolean).join(" / ");
}

function renderFlowRows(title, rows, limit = 8) {
  const supportedRowsOnly = /可证实|可解释/u.test(title);
  const selected = arrayOf(rows)
    .map(objectOf)
    .filter((row) => !supportedRowsOnly || (
      isSupportedStatus(row.edge_status)
      && optionalMoneyNumber(row.amount) !== undefined
    ))
    .slice(0, limit);
  if (!selected.length) return [];
  const lines = [
    `${title}:`,
    "| 时间/期间 | 付款/主体 | 收款/对手 | 金额 | 笔数/说明 |",
    "| --- | --- | --- | --- | --- |"
  ];
  for (const row of selected) {
    const from = text(row.from_label || row.source_holder || row.holder_name);
    const to = text(row.to_label || row.counterparty_name || row.counterparty_account);
    const txnCount = optionalCount(row.txn_count);
    const rowText = [
      flowPeriodLabel(row),
      from || "-",
      to || "-",
      moneyLabel(row.amount) || "-",
      [
        txnCount === undefined ? "" : `${txnCount} 笔`,
        flowCueText(row)
      ].filter(Boolean).join("；") || "-"
    ].map((item) => text(item).replace(/\|/gu, "/"));
    lines.push(`| ${rowText.join(" | ")} |`);
  }
  return lines;
}

function renderFundFlowSupportText(envelope) {
  const facts = {
    ...objectOf(envelope.support_facts),
    ...objectOf(envelope.compact_evidence)
  };
  const pack = objectOf(facts.fund_flow_fact_pack);
  const graphScopeStats = objectOf(facts.flow_graph_scope_stats);
  const sourceSummary = objectOf(pack.source_to_via_summary || facts.source_to_via_summary || graphScopeStats.source_to_via_summary);
  const sourceDateWindow = objectOf(pack.source_to_via_date_window_summary || facts.source_to_via_date_window_summary || graphScopeStats.source_to_via_date_window_summary);
  const sourceScope = objectOf(pack.source_scope_stats || facts.source_scope_stats);
  const viaScope = objectOf(pack.via_scope_stats || facts.via_scope_stats);
  const holder = text(pack.source_holder || sourceSummary.from_holder || facts.holder_name);
  const via = text(pack.via_holder || sourceSummary.to_holder);
  const lines = [];
  pushLine(lines, "案件事实摘录: 资金下游去向核验");
  if (holder || via) {
    pushLine(lines, `核验对象: ${holder || "来源主体"} -> ${via || "续查对象"}`);
  }
  const sourceFirst = text(sourceSummary.first_txn_at);
  const sourceLast = text(sourceSummary.last_txn_at);
  const sourceAmountValue = optionalMoneyNumber(sourceSummary.amount);
  const sourceAmount = sourceAmountValue === undefined ? "" : moneyLabel(sourceAmountValue);
  const sourceCount = optionalCount(sourceSummary.txn_count);
  if (sourceAmount && sourceCount !== undefined && sourceAmountValue !== 0 && sourceCount !== 0) {
    pushLine(lines, `已支持上游链路: ${holder || "来源主体"} -> ${via || "续查对象"}；${sourceAmount} / ${sourceCount} 笔；期间 ${sourceFirst || "未见起"} 至 ${sourceLast || "未见止"}`);
  } else if (hasEvidenceBearingValue(sourceSummary)) {
    pushLine(lines, "上游链路边界: 金额、笔数或完整范围未同时通过核验；不得写成已支持链路、零往来或未命中。");
  }
  const cluster = objectOf(sourceSummary.largest_date_cluster);
  const clusterAmount = optionalMoneyNumber(cluster.amount);
  const clusterCount = optionalCount(cluster.txn_count);
  if (text(cluster.date) && clusterAmount !== undefined && clusterCount !== undefined) {
    pushLine(lines, `上游集中日: ${text(cluster.date)}；${moneyLabel(clusterAmount)} / ${clusterCount} 笔`);
  }
  const windowAmountValue = optionalMoneyNumber(sourceDateWindow.amount);
  const windowAmount = windowAmountValue === undefined ? "" : moneyLabel(windowAmountValue);
  const windowCount = optionalCount(sourceDateWindow.txn_count);
  const windowFirst = text(sourceDateWindow.first_txn_at || sourceDateWindow.first_txn_time || sourceDateWindow.min_txn_time || pack.date_start || facts.date_start);
  const windowLast = text(sourceDateWindow.last_txn_at || sourceDateWindow.last_txn_time || sourceDateWindow.max_txn_time || pack.date_end || facts.date_end);
  const windowCluster = objectOf(sourceDateWindow.largest_date_cluster);
  if (windowAmount && windowCount !== undefined && windowAmountValue !== 0 && windowCount !== 0) {
    const windowRange = windowFirst || windowLast ? `${windowFirst || "未见起"} 至 ${windowLast || "未见止"}` : "指定时间范围";
    const windowClusterAmount = optionalMoneyNumber(windowCluster.amount);
    const windowClusterCount = optionalCount(windowCluster.txn_count);
    const clusterText = text(windowCluster.date) && windowClusterAmount !== undefined && windowClusterCount !== undefined
      ? `；集中日 ${text(windowCluster.date)} ${moneyLabel(windowClusterAmount)} / ${windowClusterCount} 笔`
      : "";
    pushLine(lines, `时间窗口核验: ${holder || "来源主体"} -> ${via || "续查对象"}；${windowRange}；${windowAmount} / ${windowCount} 笔${clusterText}`);
  } else if (hasEvidenceBearingValue(sourceDateWindow)) {
    pushLine(lines, "时间窗口边界: 金额、笔数或完整范围未同时通过核验；只能说明已检查范围和缺口。");
  }
  if (hasEvidenceBearingValue(sourceScope)) {
    pushLine(lines, `来源主体出账覆盖: ${countLabel(sourceScope.txn_count) || "尚未固定"} 笔；${countLabel(sourceScope.account_count) || "尚未固定"} 个账户；${countLabel(sourceScope.counterparty_count) || "尚未固定"} 个对手；${text(sourceScope.first_txn_at) || "未见起"} 至 ${text(sourceScope.last_txn_at) || "未见止"}`);
  }
  if (hasEvidenceBearingValue(viaScope)) {
    pushLine(lines, `续查对象出账覆盖: ${countLabel(viaScope.txn_count) || "尚未固定"} 笔；${countLabel(viaScope.account_count) || "尚未固定"} 个账户；${countLabel(viaScope.counterparty_count) || "尚未固定"} 个对手；${text(viaScope.first_txn_at) || "未见起"} 至 ${text(viaScope.last_txn_at) || "未见止"}`);
  }
  for (const line of renderFlowRows("上游可证实交易链路", pack.source_edges, 8)) {
    pushLine(lines, line);
  }
  for (const line of renderFlowRows("续查对象后续主要出账", pack.downstream_outflows, 10)) {
    pushLine(lines, line);
  }
  for (const line of renderFlowRows("可解释下游交易链路", pack.downstream_edges, 8)) {
    pushLine(lines, line);
  }
  for (const line of renderFlowRows("待核资金断点", pack.review_edges, 8)) {
    pushLine(lines, line);
  }
  const supportedGraphRows = [...arrayOf(pack.source_edges), ...arrayOf(pack.downstream_edges)]
    .map(objectOf)
    .filter((row) => isSupportedStatus(row.edge_status) && optionalMoneyNumber(row.amount) !== undefined);
  pushLine(lines, supportedGraphRows.length
    ? `事实覆盖: 仅覆盖上表 ${supportedGraphRows.length} 条金额字段完整的已支持交易边；不得扩写未核验链路。`
    : "事实边界: 当前没有金额字段完整的已支持交易边；不得把线索、空值或畸形字段写成资金事实。");
  pushLine(lines, "图谱明细边界: 下游线索十条以内已列明时逐条列明具体时间、金额、对象；不得用残余链路标题、残余笔数概括或折叠占位行替代。");
  pushLine(lines, "表述边界: 避免本次摘要、本轮、未返回、未展开或内部摘要类句式；缺字段写“现有材料尚未固定”“需复核”或“需补证”。把以“本轮”开头的材料句改成“现有材料”，把剩余去向概括句改成具体资金断点或待补证事项。");
  pushLine(lines, "图谱核验意见: 可见资金流向图至少标明一处“需复核”，用于说明余额承接、产品赎回、最终受益人或缺失对象仍需证明。");
  const risks = arrayOf(envelope.known_risks).map(cleanSupportLine).filter(Boolean).slice(0, 6);
  if (risks.length) {
    pushLine(lines, `边界与风险: ${risks.join("；")}`);
  }
  pushLine(lines, "边界标签: 对不能直接证明余额承接、产品赎回、最终受益人或缺失对象的事项，在可见答案中写“需复核”或“需补证”。");
  pushLine(lines, "不能认定: 后续出账和理财/申购线索未做逐笔余额承接前，不能直接写成来源主体原资金最终流向。");
  pushLine(lines, "补证方向: 补收款账户流水、银行回单、开户信息、账户归集、产品合同或申购赎回确认，再对指定交易继续穿透。");
  return lines.join("\n");
}

function renderSourceCounterpartySupportText(envelope) {
  const facts = {
    ...objectOf(envelope.support_facts),
    ...objectOf(envelope.compact_evidence)
  };
  const lines = [];
  pushLine(lines, "案件事实摘录: 一跳金额核验");
  const holder = text(facts.holder_name);
  const via = text(facts.via_holder_name);
  if (holder || via) {
    pushLine(lines, `核验对象: ${holder || "来源主体"} -> ${via || "对手方"}`);
  }
  for (const fact of arrayOf(facts.facts).map(objectOf)) {
    if (text(fact.support_status) && !isSupportedStatus(fact.support_status)) continue;
    const answerText = text(fact.statement || fact.answer_text || fact.answerText || fact.label);
    if (answerText) pushLine(lines, answerText);
  }
  const dateWindow = objectOf(facts.source_to_via_date_window_summary);
  if (
    !lines.some((line) => /时间窗口核验/u.test(line)) &&
    (dateWindow.amount || dateWindow.txn_count || dateWindow.first_txn_at || dateWindow.last_txn_at)
  ) {
    const first = text(dateWindow.first_txn_at || dateWindow.first_txn_time || dateWindow.min_txn_time || facts.date_start);
    const last = text(dateWindow.last_txn_at || dateWindow.last_txn_time || dateWindow.max_txn_time || facts.date_end);
    const range = first || last ? `${first || "未见起"} 至 ${last || "未见止"}` : "指定时间范围";
    const dateWindowCount = optionalCount(dateWindow.txn_count);
    pushLine(lines, `时间窗口核验: ${holder || "来源主体"} -> ${via || "对手方"}；${range}；${moneyLabel(dateWindow.amount) || "未返回金额"} / ${dateWindowCount === undefined ? "未返回" : dateWindowCount} 笔`);
  }
  const unsupported = arrayOf(facts.unsupported_flows).map(cleanSupportLine).filter(Boolean).slice(0, 5);
  if (unsupported.length) pushLine(lines, `边界与风险: ${unsupported.join("；")}`);
  pushLine(lines, "补证方向: 对一跳金额、重点账户和后续去向分别补银行回单、收款侧流水、开户资料和余额承接材料。");
  return lines.join("\n");
}

function renderDestinationOutflowSupportText(envelope) {
  const facts = {
    ...objectOf(envelope.support_facts),
    ...objectOf(envelope.compact_evidence)
  };
  const lines = [];
  pushLine(lines, "案件事实摘录: 资金下游续查");
  const holder = text(facts.holder_name);
  const via = text(facts.via_holder_name);
  if (holder || via) {
    pushLine(lines, `核验对象: ${holder || "来源主体"} -> ${via || "续查对象"}`);
  }
  for (const fact of arrayOf(facts.facts).map(objectOf)) {
    if (text(fact.support_status) && !isSupportedStatus(fact.support_status)) continue;
    const statement = text(fact.statement || fact.answer_text || fact.answerText || fact.label);
    if (statement) pushLine(lines, statement);
  }
  for (const pathLine of arrayOf(facts.continuation_paths).map(cleanSupportLine).filter(Boolean).slice(0, 5)) {
    pushLine(lines, `续查路径: ${pathLine}`);
  }
  const targets = arrayOf(facts.continuation_targets).map(objectOf).slice(0, 5);
  for (const target of targets) {
    const targetName = text(target.target);
    const reason = text(target.reason);
    if (targetName || reason) pushLine(lines, `补调对象: ${targetName || "待定对象"}；${reason || "补充可复核材料"}`);
  }
  const risks = arrayOf(envelope.known_risks).map(cleanSupportLine).filter(Boolean).slice(0, 6);
  if (risks.length) pushLine(lines, `边界与风险: ${risks.join("；")}`);
  pushLine(lines, "暂不能认定: 未做逐笔余额承接前，后续出账、理财/资产线索不能直接证明来源主体原资金已穿透。");
  pushLine(lines, "表述边界: 下游续查避免内部摘要、轮次、返回或展开类表述；缺口写“当前材料尚未固定逐笔明细”“需复核”或“需补证”。");
  return lines.join("\n");
}

const GENERIC_FACT_LABELS = {
  holder_name: "主体",
  account_key: "账户",
  total_amount: "金额",
  table_name: "表名",
  row_count: "记录数",
  exact_count_contract: "精确计数答复约束",
  result_kind: "结果类型",
  total_txn_count: "笔数",
  txn_count: "笔数",
  account_count: "账户数",
  counterparty_count: "对手方数",
  first_txn_at: "首笔时间",
  last_txn_at: "末笔时间",
  date_start: "起始日期",
  date_end: "截止日期",
  fact_source: "事实来源",
  source: "来源",
  case_name: "案件名称",
  case_identity: "案件身份",
  match_scope: "匹配范围",
  status: "状态",
  boundary: "边界",
  verified_claims: "已核验结论",
  corrected_claims: "需纠正结论",
  unsupported_claims: "证据不足结论",
  unsupported_flows: "证据不足资金流",
  missing_source_boundaries: "来源缺口",
  next_review_actions: "复核动作",
  strict_rows: "严格命中",
  excluded_near_names: "近名排除",
  cannot_confirm: "不能确认",
  buckets: "分组事实",
  nature_buckets: "性质分组",
  supported_edges: "可证实链路",
  boundary_edges: "待补证链路",
  top_outflows: "主要出账去向",
  top_accounts: "重点账户",
  top_holders: "重点主体",
  top_counterparties: "重点对手方",
  counterparty_rankings: "对手方排行",
  rankings: "排行",
  source_to_via_summary: "两方金额汇总",
  source_to_via_date_window_summary: "时间窗口核验",
  source_to_via_core_account_summary: "重点账户汇总",
  fund_flow_fact_pack: "资金下游事实包",
  account_stats: "账户统计",
  answer_delivery_notes: "核验说明",
  sample_notice: "样本边界",
  result_preview: "结果预览",
  execution_status: "执行状态",
  runtime_status: "运行状态",
  evidence_status: "证据状态",
  sql_policy_summary: "专项核算策略",
  evidence_card_summary: "证据卡摘要",
  metric_scope_summary: "指标范围",
  validation_summary_text: "验证摘要",
  visible_redaction_guard: "最终答复改写约束",
  source_hash: "来源指纹",
  returned_row_count: "返回行数",
  result_mode: "结果模式",
  error_code: "错误代码",
  error_message: "错误原因",
  reason: "原因",
  recovery_actions: "恢复动作",
  recommended_workbench_tools: "推荐工具",
  warning_messages: "提示"
};

function genericRecordSeed(record) {
  return objectOf(record.seed_txn || record.seed_transaction || record.seed || record.txn);
}

function genericRecordName(record) {
  const seed = genericRecordSeed(record);
  return text(
    record.display_name
      || record.holder_name
      || record.counterparty_name
      || record.counterparty_key
      || record.account_open_name
      || record.account_key
      || record.claim
      || record.text
      || record.statement
      || record.summary
      || record.bucket
      || record.title
      || seed.display_name
      || seed.holder_name
      || seed.counterparty_name
      || seed.counterparty_key
      || seed.account_open_name
      || seed.account_key
      || seed.summary
  );
}

function genericRecordAmountLabel(record) {
  const seed = genericRecordSeed(record);
  const metric = text(record.metric || record.order_metric || record.metric_mode || seed.metric);
  let amount;
  if (metric === "inflow") {
    amount = firstOptionalValue(optionalMoneyNumber, record.inflow, record.inflow_total, record.metric_value, seed.inflow, seed.inflow_total, seed.metric_value);
  } else if (metric === "outflow") {
    amount = firstOptionalValue(optionalMoneyNumber, record.outflow, record.outflow_total, record.metric_value, seed.outflow, seed.outflow_total, seed.metric_value);
  } else if (metric === "turnover") {
    amount = firstOptionalValue(optionalMoneyNumber, record.turnover, record.turnover_total, record.total_amount, record.metric_value, seed.turnover, seed.turnover_total, seed.total_amount, seed.metric_value);
  } else if (metric === "max_single_amount") {
    amount = firstOptionalValue(optionalMoneyNumber, record.max_single, record.max_single_amount, record.metric_value, seed.max_single, seed.max_single_amount, seed.metric_value);
  } else {
    amount = firstOptionalValue(optionalMoneyNumber,
      record.amount,
      record.total_amount,
      record.metric_value,
      record.turnover,
      record.turnover_total,
      record.outflow,
      record.outflow_total,
      record.inflow,
      record.inflow_total,
      record.net_flow,
      seed.amount,
      seed.total_amount,
      seed.metric_value,
      seed.turnover,
      seed.turnover_total,
      seed.outflow,
      seed.outflow_total,
      seed.inflow,
      seed.inflow_total,
      seed.net_flow);
  }
  return amount === undefined ? "" : moneyLabel(amount);
}

function genericRecordCountLabel(record) {
  const seed = genericRecordSeed(record);
  const count = firstOptionalValue(optionalCount,
    record.txn_count,
    record.total_txn_count,
    record.count,
    seed.txn_count,
    seed.total_txn_count,
    seed.count);
  return count === undefined ? "" : `${count} 笔`;
}

function genericRecordTimeLabel(record) {
  const seed = genericRecordSeed(record);
  return text(record.txn_time || record.first_txn_at || record.last_txn_at || seed.txn_time || seed.first_txn_at || seed.last_txn_at);
}

function genericScalarLabel(value) {
  if (typeof value === "number") {
    return Number.isFinite(value) ? String(value) : "";
  }
  if (typeof value === "boolean") return value ? "true" : "false";
  if (value && typeof value === "object") return "";
  return text(value);
}

function genericMetricLabel(value) {
  const raw = text(value);
  if (raw === "turnover") return "交易总额/往来总额（入账+出账）";
  if (raw === "inflow") return "入账金额";
  if (raw === "outflow") return "出账金额";
  if (raw === "txn_count") return "交易笔数";
  if (raw === "max_single_amount") return "最大单笔金额";
  return genericScalarLabel(value);
}

function genericArrayDisplayRows(key, item) {
  if (key === "rankings" && arrayOf(item).length === 1 && Array.isArray(objectOf(item[0]).rows)) {
    return arrayOf(objectOf(item[0]).rows);
  }
  if (key === "supported_edges" || key === "drawable_edges" || key === "mermaid_edges") {
    return arrayOf(item).filter((row) => {
      const source = objectOf(row);
      return isSupportedStatus(source.edge_status || source.support_status)
        && optionalMoneyNumber(source.amount) !== undefined;
    });
  }
  if (/(?:^|_)edges$/iu.test(key)) {
    return arrayOf(item).filter((row) => {
      const source = objectOf(row);
      return !(isSupportedStatus(source.edge_status || source.support_status)
        && optionalMoneyNumber(source.amount) === undefined);
    });
  }
  return arrayOf(item);
}

function genericArrayDisplayLimit(key, rows) {
  if (/^(?:result_preview|rankings|counterparty_rankings|top_counterparties|top_accounts|top_holders|top_outflows)$/iu.test(key)) {
    return Math.min(Math.max(arrayOf(rows).length, 5), 50);
  }
  return 5;
}

function genericRecordKeyValueLabel(record, limit = 8) {
  const source = objectOf(record);
  return Object.entries(source)
    .filter(([key]) => !/^(?:case_id|.*_id|.*_refs?|.*_state|support_|final_answer_|context_compiler|answer_)/iu.test(key))
    .slice(0, limit)
    .map(([key, value]) => {
      const valueText = key === "metric"
        ? genericMetricLabel(value)
        : /(?:amount|money|flow|turnover|total|balance)/iu.test(key)
        ? (moneyLabel(value) || genericScalarLabel(value))
        : genericScalarLabel(value);
      return valueText ? `${key}=${valueText}` : "";
    })
    .filter(Boolean)
    .join("，");
}

function renderGenericFactRows(value, depth = 0, limit = 18) {
  if (depth > 2 || limit <= 0) return [];
  const source = objectOf(value);
  const lines = [];
  for (const [key, item] of Object.entries(source)) {
    if (lines.length >= limit) break;
    if (/^(?:case_id|.*_id|.*_refs?|.*_state|support_|final_answer_|context_compiler|answer_)/iu.test(key)) continue;
    const label = GENERIC_FACT_LABELS[key] || "";
    if (!label && (item == null || typeof item !== "object")) continue;
    if (Array.isArray(item)) {
      const displayRows = genericArrayDisplayRows(key, item);
      const rowLimit = genericArrayDisplayLimit(key, displayRows);
      const rows = displayRows.slice(0, rowLimit).map((row) => {
        if (!row || typeof row !== "object") return genericScalarLabel(row);
        const record = objectOf(row);
        const narrativeRow = [
          record.rank ? `第 ${record.rank}` : "",
          genericRecordName(record),
          text(record.from_label || record.from_account) && text(record.to_label || record.to_account)
            ? `${text(record.from_label || record.from_account)} -> ${text(record.to_label || record.to_account)}`
            : "",
          genericRecordAmountLabel(record),
          genericRecordCountLabel(record),
          genericRecordTimeLabel(record)
        ].filter(Boolean).join("，");
        return narrativeRow || genericRecordKeyValueLabel(record);
      }).filter(Boolean);
      if (rows.length) lines.push(`${label || "列表"}: ${rows.join("；")}`);
      continue;
    }
    if (item && typeof item === "object") {
      lines.push(...renderGenericFactRows(item, depth + 1, limit - lines.length));
      continue;
    }
    const valueText = /(?:amount|money|flow|turnover)/iu.test(key) ? (moneyLabel(item) || text(item)) : text(item);
    if (label && valueText) lines.push(`${label}: ${valueText}`);
  }
  return lines;
}

function suggestedNextDirectionSummary(envelope, limit = 4) {
  const rows = arrayOf(envelope.suggested_next_directions)
    .map((item) => {
      const source = objectOf(item);
      const direction = cleanSupportLine(source.direction);
      if (!direction) return "";
      const actionType = cleanSupportLine(source.action_type);
      const target = cleanSupportLine(source.target);
      return [
        actionType ? `${actionType}` : "",
        target ? `对象 ${target}` : "",
        direction
      ].filter(Boolean).join("，");
    })
    .filter(Boolean)
    .slice(0, limit);
  return uniqueTexts(rows, limit).join("；");
}

function diagnosticSupportLine(value) {
  return text(value)
    .replace(/(?:\/Users|\/var)\/[^\s"'，。；、)）]+/gu, "本地路径已隐藏")
    .replace(/\bDuckDB\b/giu, "受控查询引擎")
    .replace(/\b_duckdb\b/giu, "受控查询引擎")
    .replace(/\s{2,}/gu, " ")
    .trim();
}

function renderWorkbenchDiagnosticSupportText(envelope) {
  const facts = {
    ...objectOf(envelope.support_facts),
    ...objectOf(envelope.compact_evidence)
  };
  const workbench = objectOf(facts.workbench);
  const diagnosis = objectOf(workbench.diagnosis);
  const candidateColumns = arrayOf(diagnosis.candidate_columns).map(objectOf);
  const sqlIdentifiers = uniqueTexts([
    ...candidateColumns.flatMap((item) => [text(item.table_name), text(item.column_name)]),
    ...arrayOf(diagnosis.candidate_bindings).map(text),
    text(diagnosis.missing_identifier)
  ].filter(Boolean), 24);
  const lines = [
    "案件事实摘录: 专项资金核算诊断",
    "用途: 仅供下一次受控工具调用修正字段或表名；最终答复不得展示这些技术标识符。"
  ];
  const status = diagnosticSupportLine(workbench.execution_status || workbench.runtime_status || objectOf(envelope.validation_state).status);
  if (status) lines.push(`诊断状态: ${status}`);
  const errorClass = diagnosticSupportLine(workbench.error_class || diagnosis.error_class);
  if (errorClass) lines.push(`错误类别: ${errorClass}`);
  if (sqlIdentifiers.length) {
    lines.push(`SQL-safe identifiers for next tool call only: ${sqlIdentifiers.join(", ")}`);
  }
  const diagnostics = arrayOf(diagnosis.diagnostics).map(diagnosticSupportLine).filter(Boolean).slice(0, 3);
  if (diagnostics.length) {
    lines.push(`诊断摘要: ${diagnostics.join("；")}`);
  }
  const correctiveActions = arrayOf(diagnosis.corrective_actions).map(diagnosticSupportLine).filter(Boolean).slice(0, 4);
  if (correctiveActions.length) {
    lines.push(`修正动作: ${correctiveActions.join("；")}`);
  }
  const guard = diagnosticSupportLine(workbench.visible_redaction_guard)
    || "最终答复改写约束：只写业务证据、金额、笔数、对象、时间范围和核验边界；不得写数据库、表名、字段名、查询编号或运行态状态词。";
  lines.push(guard);
  return lines.filter(Boolean).join("\n");
}

function renderSupportEnvelopeForAgent(envelope) {
  const cardType = text(objectOf(envelope.support_facts).card_type);
  if (cardType === "subject_dossier_ready_guard_card") {
    return renderSubjectReadyGuardText(envelope);
  }
  if (cardType === "pair_amount_review_card" || objectOf(envelope.support_facts).pair_amount_fact_pack) {
    return renderPairAmountSupportText(envelope);
  }
  if (cardType === "source_to_counterparty_amount_card") {
    return renderSourceCounterpartySupportText(envelope);
  }
  if (cardType === "destination_outflow_card") {
    return renderDestinationOutflowSupportText(envelope);
  }
  const mergedFacts = {
    ...objectOf(envelope.support_facts),
    ...objectOf(envelope.compact_evidence)
  };
  const sourceTool = text(objectOf(envelope.source_map).tool);
  if (sourceTool === "diagnose_case_sql" || sourceTool === "explain_case_sql") {
    return renderWorkbenchDiagnosticSupportText(envelope);
  }
  if (sourceTool === "run_full_case_analysis" || Object.keys(objectOf(mergedFacts.full_case_package)).length) {
    return renderFullCaseSupportText(envelope);
  }
  if (Object.keys(objectOf(mergedFacts.fund_flow_fact_pack)).length) {
    return renderFundFlowSupportText(envelope);
  }
  if (
    sourceTool === "analyze_holder_full"
    || (
      Object.keys(objectOf(mergedFacts.holder_scope || mergedFacts.scope)).length
      && (
        Object.keys(objectOf(mergedFacts.account_stats || mergedFacts.holder_account_stats)).length
        || arrayOf(mergedFacts.holder_top_accounts || mergedFacts.top_accounts).length
        || arrayOf(mergedFacts.counterparty_rankings || mergedFacts.top_counterparties).length
      )
    )
  ) {
    return renderSubjectDossierSupportText(envelope);
  }
  const lines = ["案件事实摘录: 当前核验结果"];
  const facts = mergedFacts;
  for (const line of renderGenericFactRows(facts)) {
    pushLine(lines, line);
  }
  const risks = arrayOf(envelope.known_risks).map(cleanSupportLine).filter(Boolean).slice(0, 6);
  if (risks.length) pushLine(lines, `边界与风险: ${risks.join("；")}`);
  const nextDirectionSummary = suggestedNextDirectionSummary(envelope);
  if (nextDirectionSummary) {
    pushLine(lines, `下一步核查方向: ${nextDirectionSummary}`);
  }
  if (lines.length === 1) {
    pushLine(lines, "当前核验未返回可直接成稿的事实字段；请按已知边界继续核验或向用户说明来源缺口。");
  }
  return lines.join("\n");
}

export function createAgentOutputCompiler({ env = {}, textBudget = DEFAULT_AGENT_TEXT_BUDGET }) {
  void env;
  function agentReadableToolText(compact) {
    const guardPayload = redactAgentPayload(compact);
    if (requiresPublicationBlock(guardPayload)) {
      return PUBLICATION_RECEIPT_REQUIRED_TEXT;
    }
    const rawPayload = objectOf(guardPayload);
    const payload = preserveStableBoundaryMetadata(
      rawPayload,
      stripAgentOpaqueRefs(rawPayload) || {}
    );
    const card = objectOf(payload.answer_card);
    if (text(card.card_type) === "case_source_blocker") {
      return renderCaseSourceBlockerForAgent(card, payload);
    }
    return renderSupportEnvelopeForAgent(boundedSupportEnvelope(payload, textBudget));
  }

  function compactToolText(payload, args = {}) {
    const redactedPayload = redactAgentPayload(payload);
    if (requiresPublicationBlock(redactedPayload, args)) {
      return PUBLICATION_RECEIPT_REQUIRED_TEXT;
    }
    const debugRequested = objectOf(args).include_debug === true;
    const currentCaseText = renderCurrentCaseSupportText(redactedPayload);
    if (currentCaseText) return currentCaseText;
    const compact = compactToolPayloadForAgent(redactedPayload);
    if (debugRequested) {
      compact.warnings = compactWarningList(compact.warnings, [{
        code: "DEBUG_PAYLOAD_DISABLED",
        severity: "info",
        message: "生产默认不向模型展开完整 JSON；如需开发排障，请在受控环境设置 ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD=true。"
      }]);
    }
    return agentReadableToolText(compact);
  }

  function compactStructuredContent(payload, args = {}) {
    const redactedPayload = redactAgentPayload(payload);
    if (requiresPublicationBlock(redactedPayload, args)) {
      return reportPublicationBlockedPayload(payload);
    }
    const compact = compactToolPayloadForAgent(redactedPayload);
    if (objectOf(args).include_debug === true) {
      compact.warnings = compactWarningList(compact.warnings, [{
        code: "DEBUG_PAYLOAD_DISABLED",
        severity: "info",
        message: "生产默认不向模型展开完整 JSON；如需开发排障，请在受控环境设置 ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD=true。"
      }]);
    }
    return boundedSupportEnvelope(compact, textBudget);
  }

  return {
    agentReadableToolText,
    compactStructuredContent,
    compactToolText,
    minimalAgentCard
  };
}
