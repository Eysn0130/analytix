export const FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE =
  "dataset_snapshot_authority_unavailable";

export const FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY =
  "案件事实未发布：当前资金数据源尚未完成同一案件、同一轮次和同一数据快照的宿主核验；本轮未接受任何金额、笔数、账户、主体、时间范围或关系结论。";

export const FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI =
  "analytix://funds/source-unavailable";

export const FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME =
  "prepare-investigation-report";

const FUNDS_SOURCE_UNAVAILABLE_GUIDANCE = [
  FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY,
  "当前不查询案件数据，不生成分析草稿、正式报告、附件或文件。",
  "当前只能说明能力缺口、未检查范围和补证动作；完成同案证据核验与人工复核后方可重新申请发布。"
].join("\n");

export function fixedFundsBoundaryResources() {
  return [{
    uri: FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI,
    name: "资金事实来源不可用边界",
    description: "固定能力边界；不包含案件事实、敏感个人信息或报告内容。",
    mimeType: "text/plain"
  }];
}

export function fixedFundsBoundaryResourceRead(uri) {
  if (uri !== FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI) return null;
  return {
    contents: [{
      uri: FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI,
      mimeType: "text/plain",
      text: FUNDS_SOURCE_UNAVAILABLE_GUIDANCE
    }]
  };
}

export function fixedFundsBoundaryPrompts() {
  return [{
    name: FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
    description: "返回固定资金来源不可用边界；不接受案件文本或敏感个人信息参数。",
    arguments: []
  }];
}

export function fixedFundsBoundaryPrompt(name) {
  if (name !== FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME) return null;
  return {
    description: "资金来源不可用固定边界",
    messages: [{
      role: "user",
      content: { type: "text", text: FUNDS_SOURCE_UNAVAILABLE_GUIDANCE }
    }]
  };
}

export function fixedFundsUnavailableOutcome() {
  return {
    transportStatus: "success",
    semanticStatus: "unavailable",
    reportedSemanticStatus: "unavailable",
    safeToAnswer: false,
    isError: true,
    blocker: FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE,
    partialCoverage: {},
    data: {},
    candidateEvidenceReceipts: []
  };
}

export function fixedFundsUnavailableToolResult() {
  const outcome = fixedFundsUnavailableOutcome();
  return {
    content: [{ type: "text", text: FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY }],
    structuredContent: outcome,
    isError: true,
    _meta: {
      analytix_tool_outcome: outcome,
      analytix_evidence_ledger: {
        status: "unsupported",
        host_registry_verified: false,
        receipt_count: 0
      }
    }
  };
}

export function fixedFundsScopeUnavailableBoundary(mandatoryGates) {
  return {
    version: "case_scope_map_boundary_v1",
    semantic_status: "unavailable",
    blocker: FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE,
    safe_to_answer: false,
    fact_answer_allowed: false,
    publication_readiness: "blocked",
    host_registry_verified: false,
    coverage_complete: false,
    zero_result_verified: false,
    raw_rows_exposed: false,
    checked_scope: "none_verified",
    mandatory_gates: mandatoryGates,
    next_actions: [
      "establish_host_source_authority",
      "retry_after_current_run_live_probe"
    ]
  };
}
