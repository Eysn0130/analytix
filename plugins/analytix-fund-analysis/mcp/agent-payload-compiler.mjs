import {
  humanizeAgentText,
  stripAgentOpaqueRefs
} from "./agent-context-hygiene.mjs";
import { translateUserVisibleText } from "./user-facing-language.mjs";
import {
  arrayOf,
  booleanOrUndefined,
  dataOf,
  hasOwn,
  intOrUndefined,
  numberOrUndefined,
  objectOf,
  pruneEmpty,
  text,
  uniqueTexts
} from "./runtime-normalizers.mjs";

export const AGENT_PAYLOAD_COMPILER_VERSION = "0.16.11";
export const AGENT_PAYLOAD_COMPILER_CONTRACT = [
  "Compact MCP output",
  "Do not infer totals",
  "edge_status='supported'"
];

export function envelopeStatus(envelope) {
  return text(envelope?.status) || (envelope ? "ok" : "error");
}

export function unwrapSkillEnvelope(payload) {
  const data = dataOf(payload);
  if (
    data &&
    typeof data === "object" &&
    Object.prototype.hasOwnProperty.call(data, "skill_id") &&
    Object.prototype.hasOwnProperty.call(data, "data")
  ) {
    return data;
  }
  return payload || {};
}

export function skillEnvelopeData(payload) {
  return dataOf(unwrapSkillEnvelope(payload));
}

export function envelopeWarnings(envelope) {
  return arrayOf(envelope?.warnings).map((item) => {
    if (item && typeof item === "object") {
      return {
        code: text(item.code),
        severity: text(item.severity) || "warning",
        message: text(item.message)
      };
    }
    return { code: "", severity: "warning", message: text(item) };
  }).filter((item) => item.code || item.message);
}

export function envelopeQueryIds(envelope) {
  return arrayOf(envelope?.citations?.query_ids).map(text).filter(Boolean);
}

export function evidenceRefsFromEnvelope(envelope, fallbackPayload = {}) {
  const citations = objectOf(envelope?.citations || fallbackPayload.citations);
  const auditRef = objectOf(envelope?.audit_ref || fallbackPayload.audit_ref);
  return pruneEmpty({
    query_ids: arrayOf(citations.query_ids).map(text).filter(Boolean).slice(0, 12),
    evidence_ids: arrayOf(citations.evidence_ids).map(text).filter(Boolean).slice(0, 12),
    path_ids: arrayOf(citations.path_ids).map(text).filter(Boolean).slice(0, 12),
    report_ids: arrayOf(citations.report_ids).map(text).filter(Boolean).slice(0, 12),
    audit_ref: Object.keys(auditRef).length ? auditRef : undefined
  }) || {};
}

export function compactMoneyFact(value) {
  if (value === undefined || value === null || value === "") {
    return undefined;
  }
  if (value && typeof value === "object") {
    const source = objectOf(value);
    const candidate = source.yuan ?? source.amount_yuan ?? source.amount ?? source.value;
    if (candidate !== undefined && candidate !== null && candidate !== "") {
      return compactMoneyFact(candidate);
    }
    return undefined;
  }
  const number = numberOrUndefined(value);
  if (number === undefined) {
    return undefined;
  }
  return {
    yuan: Number(number.toFixed(2)),
    wan: Number((number / 10000).toFixed(6)),
    text: `${number.toFixed(2)} 元（${(number / 10000).toFixed(6)} 万元）`
  };
}

function nonNegativeIntOrUndefined(value) {
  const number = intOrUndefined(value);
  return number !== undefined && number >= 0 ? number : undefined;
}

function strictNonNegativeIntOrUndefined(value) {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
    ? value
    : undefined;
}

function strictNonNegativeIntArrayOrUndefined(value) {
  if (!Array.isArray(value)) return undefined;
  const normalized = value.map(strictNonNegativeIntOrUndefined);
  return normalized.some((item) => item === undefined) ? undefined : normalized.slice(0, 5);
}

function firstNonNegativeInt(...values) {
  for (const value of values) {
    const number = nonNegativeIntOrUndefined(value);
    if (number !== undefined) return number;
  }
  return undefined;
}

export function sameFactEligibilityComplete(value) {
  const source = objectOf(value);
  const requested = nonNegativeIntOrUndefined(source.requested_row_count);
  const factEligible = nonNegativeIntOrUndefined(source.fact_eligible_row_count);
  const familyEligible = nonNegativeIntOrUndefined(source.family_eligible_row_count);
  const rejected = nonNegativeIntOrUndefined(source.rejected_row_count);
  const histogram = objectOf(source.rejection_reason_histogram);
  return text(source.contract) === "SameFactEligibilityV1"
    && text(source.key_version) === "analytix.same-fact-dedupe-key/v2"
    && text(source.coverage_status) === "complete"
    && !text(source.blocker)
    && requested !== undefined
    && requested > 0
    && factEligible === requested
    && familyEligible === requested
    && rejected === 0
    && Object.keys(histogram).length === 0;
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

function firstBooleanField(sources, keys) {
  for (const sourceValue of sources) {
    const source = objectOf(sourceValue);
    for (const key of keys) {
      if (!hasOwn(source, key)) continue;
      const value = booleanOrUndefined(source[key]);
      if (value !== undefined) return value;
    }
  }
  return undefined;
}

function firstTextField(sources, keys) {
  for (const sourceValue of sources) {
    const source = objectOf(sourceValue);
    for (const key of keys) {
      const value = text(source[key]);
      if (value) return value;
    }
  }
  return "";
}

function compactEvidenceGaps(...sources) {
  const output = [];
  const seen = new Set();
  for (const source of sources) {
    for (const value of arrayOf(source)) {
      const item = value && typeof value === "object"
        ? pruneEmpty({
            code: text(value.code),
            status: text(value.status),
            scope: value.scope,
            reason: value.reason,
            message: value.message,
            missing: value.missing,
            required_evidence: value.required_evidence
          })
        : text(value);
      if (!item) continue;
      const key = typeof item === "string" ? item : JSON.stringify(item);
      if (seen.has(key)) continue;
      seen.add(key);
      output.push(item);
      if (output.length >= 10) return output;
    }
  }
  return output;
}

function resultSafetyMetadata(source, response, data) {
  const localPackage = objectOf(data.full_case_package);
  const coverageEnvelope = objectOf(data.case_scope_coverage);
  const investigationLab = objectOf(data.investigation_lab);
  const coverage = objectOf(
    coverageEnvelope.coverage
      || investigationLab.coverage
      || localPackage.coverage
      || data.coverage
  );
  const validationState = objectOf(data.validation_state);
  const validationSummary = objectOf(data.validation_summary);
  const sourceMeta = objectOf(source.meta);
  const responseMeta = objectOf(response.meta);
  const dataMeta = objectOf(data.meta);
  const sources = [
    data,
    response,
    source,
    localPackage,
    coverageEnvelope,
    coverage,
    investigationLab,
    validationState,
    validationSummary,
    dataMeta,
    responseMeta,
    sourceMeta
  ];
  const semanticStatus = canonicalSemanticStatus(
    data.semantic_status,
    data.semanticStatus,
    response.semantic_status,
    response.semanticStatus,
    response.status,
    source.semantic_status,
    source.semanticStatus,
    source.status,
    localPackage.semantic_status,
    validationState.semantic_status,
    validationSummary.semantic_status
  );
  const reportedCoverageComplete = firstBooleanField(sources, [
    "coverage_complete",
    "coverageComplete"
  ]);
  const reportedPartialCoverage = firstBooleanField(sources, [
    "partial_coverage",
    "partialCoverage"
  ]);
  const rawCoverageStatus = firstTextField(sources, ["coverage_status", "coverageStatus"]);
  const partialCoverage = semanticStatus === "partial"
    || reportedPartialCoverage === true
    || reportedCoverageComplete === false
    || /partial|incomplete|truncated|部分|不完整|已截断/iu.test(rawCoverageStatus);
  return pruneEmpty({
    semantic_status: semanticStatus,
    coverage_status: partialCoverage ? "partial" : rawCoverageStatus,
    source_coverage_complete: partialCoverage ? false : reportedCoverageComplete,
    partial_coverage: partialCoverage || undefined,
    source_dataset_snapshot_id: firstTextField(sources, [
      "reported_dataset_snapshot_id",
      "dataset_snapshot_id",
      "datasetSnapshotId",
      "snapshot_id",
      "snapshotId"
    ]),
    evidence_gaps: compactEvidenceGaps(
      data.evidence_gaps,
      localPackage.evidence_gaps,
      coverageEnvelope.evidence_gaps,
      coverage.evidence_gaps,
      investigationLab.evidence_gaps,
      validationState.evidence_gaps,
      validationSummary.evidence_gaps,
      response.evidence_gaps,
      source.evidence_gaps
    )
  }) || {};
}

function attachStableSafetyMetadata(value, metadata) {
  const compact = objectOf(value);
  const safety = objectOf(metadata);
  if (!Object.keys(safety).length) return compact;
  const next = { ...compact };
  for (const key of [
    "semantic_status",
    "coverage_status",
    "source_coverage_complete",
    "partial_coverage",
    "source_dataset_snapshot_id"
  ]) {
    if (hasOwn(safety, key)) next[key] = safety[key];
  }
  if (arrayOf(safety.evidence_gaps).length) {
    next.evidence_gaps = safety.evidence_gaps;
  }
  const keyFacts = objectOf(next.key_facts);
  const fullCasePackage = objectOf(keyFacts.full_case_package);
  if (Object.keys(fullCasePackage).length) {
    next.key_facts = {
      ...keyFacts,
      full_case_package: pruneEmpty({
        ...fullCasePackage,
        ...safety,
        evidence_gaps: compactEvidenceGaps(fullCasePackage.evidence_gaps, safety.evidence_gaps)
      }) || {}
    };
  }
  return next;
}

function firstArrayField(source, keys) {
  const record = objectOf(source);
  for (const key of keys) {
    if (hasOwn(record, key) && Array.isArray(record[key])) {
      return { present: true, value: record[key] };
    }
  }
  return { present: false, value: [] };
}

function arrayLengthIfPresent(source, keys) {
  const field = firstArrayField(source, keys);
  return field.present ? field.value.length : undefined;
}

function exactCountAnswerContract(toolName, data, rowCount) {
  if (toolName !== "count_case_rows") return "";
  const integerCount = nonNegativeIntOrUndefined(rowCount);
  if (integerCount === undefined) return "";
  const businessLabel = text(data.business_table_label) || businessTableLabel(data.table_name) || "目标记录";
  return `${businessLabel}精确记录数为 ${integerCount.toLocaleString("en-US")} 条（原始整数 ${integerCount}）；最终回答必须写精确整数，不得四舍五入、不得改写为“约/大约/左右/万条”概数。`;
}

export function compactWarningList(...sources) {
  const output = [];
  const seen = new Set();
  for (const source of sources) {
    for (const item of arrayOf(source)) {
      if (!item) {
        continue;
      }
      const warning = typeof item === "object"
        ? {
            code: humanizeAgentText(item.code),
            severity: text(item.severity) || "warning",
            message: humanizeAgentText(item.message || item.reason || item.detail)
          }
        : { code: "", severity: "warning", message: humanizeAgentText(item) };
      const key = `${warning.code}::${warning.message}`;
      if ((warning.code || warning.message) && !seen.has(key)) {
        seen.add(key);
        output.push(warning);
      }
    }
  }
  return output.slice(0, 10);
}

export function compactNextActions(actions, limit = 5) {
  return arrayOf(actions)
    .map((item) => {
      const source = objectOf(item);
      return pruneEmpty({
        action_type: humanizeAgentText(source.action_type || source.type || source.lane || source.category),
        reason: humanizeAgentText(source.reason || source.why_this_query || source.text || source.label),
        evidence_basis: humanizeAgentText(source.evidence_basis || source.basis || source.fact_basis || source.source_basis),
        boundary: humanizeAgentText(source.boundary || source.stop_condition || source.condition),
        target: humanizeAgentText(source.target || source.object || source.subject || source.account || source.counterparty)
      }) || {};
    })
    .filter((item) => Object.keys(item).length)
    .slice(0, limit);
}

function compactWorkbenchFacts(toolName, source, response, data) {
  const card = objectOf(source.answer_card);
  const isWorkbench = toolName === "run_case_sql"
    || toolName === "count_case_rows"
    || toolName === "explain_case_sql"
    || toolName === "diagnose_case_sql"
    || toolName === "profile_case_schema"
    || toolName === "preview_case_rows"
    || toolName === "inspect_workbench_history"
    || toolName === "case_sql_recipes"
    || toolName === "create_case_notebook"
    || text(card.card_type) === "case_workbench_card"
    || text(data.workbench_contract).includes("controlled_case_workbench")
    || text(data.workbench_contract).includes("local_duckdb_readonly")
    || text(data.notebook_contract).includes("controlled_case_workbench")
    || text(data.explain_contract).includes("case_sql_explain")
    || text(data.diagnostic_contract).includes("case_sql_diagnostic")
    || text(data.profile_contract).includes("case_schema_profile")
    || text(data.preview_contract).includes("case_row_preview")
    || text(data.history_contract).includes("case_workbench_history")
    || text(data.recipe_contract).includes("case_sql_recipes");
  if (!isWorkbench) return {};
  const cellsField = firstArrayField(data, ["cells"]);
  const cells = cellsField.value;
  const executedCells = cells.filter((cell) => text(objectOf(cell).execution_status) === "executed");
  const executableCells = cells.filter((cell) => text(objectOf(cell).type) === "query" || text(objectOf(cell).query_request));
  const warningMessages = compactWarningList(envelopeWarnings(response), source.warnings, data.warnings)
    .map((item) => text(item.message || item.code))
    .filter(Boolean)
    .slice(0, 4);
  const executionStatus = text(data.execution_status);
  const evidenceStatus = text(data.evidence_status);
  const runtimeStatus = text(response.status || source.status);
  const resultPreview = compactWorkbenchResultPreview(data);
  const sampleNotice = text(data.sample_notice);
  const summary = objectOf(data.summary);
  const recordsField = firstArrayField(data, ["records", "rows", "result_rows", "resultRows", "rankings"]);
  const records = recordsField.value;
  const rankingsField = firstArrayField(data, ["rankings"]);
  const isRanking = text(data.execution_status) === "ranked" || text(data.rank_type).startsWith("rank_") || rankingsField.present;
  const rowLimit = firstNonNegativeInt(data.row_limit, data.max_rows_per_query, data.requested_limit, summary.limit);
  const explicitRowCount = firstNonNegativeInt(data.row_count, data.returned_count, summary.returned_count);
  const rowCount = explicitRowCount ?? (recordsField.present ? records.length : undefined);
  const requestedLimit = isRanking ? requestedRankingLimit(data, rowLimit ?? rowCount ?? 10) : undefined;
  const validationState = objectOf(data.validation_state);
  const validationSummary = objectOf(data.validation_summary);
  const sqlPolicy = objectOf(data.sql_policy || validationState.sql_policy);
  const evidenceCard = objectOf(data.evidence_card || source.evidence_card);
  const metricScope = objectOf(data.metric_scope || evidenceCard.metric_scope);
  const metricScopeRowCount = firstNonNegativeInt(metricScope.row_count, rowCount);
  const truncationState = booleanOrUndefined(metricScope.truncated)
    ?? booleanOrUndefined(data.truncated);
  const policyEngine = text(sqlPolicy.policy_engine || validationSummary.sql_policy_engine);
  const parserBinderPassed = sqlPolicy.status === "passed" || validationSummary.parser_binder_validated === true;
  const rawRowsExposed = consistentBooleanOrUndefined(
    data.raw_rows_exposed,
    validationState.raw_rows_exposed
  );
  const rawRowsBoundaryText = rawRowsExposed === true
    ? "已展开来源明细"
    : (rawRowsExposed === false ? "明确未向模型展开来源明细" : "来源明细展开状态未知");
  const metricScopeText = [
    text(metricScope.purpose || data.purpose) ? `用途=${text(metricScope.purpose || data.purpose)}` : "",
    text(metricScope.result_mode || data.result_mode) ? `结果形态=${safeWorkbenchText(metricScope.result_mode || data.result_mode)}` : "",
    metricScopeRowCount !== undefined ? `返回行数=${metricScopeRowCount}` : "",
    truncationState === true ? "结果已截断" : (truncationState === false ? "结果未截断" : "")
  ].filter(Boolean).join("；");
  const sqlPolicyText = [
    "当前案件只读",
    "仅限清洗后数据和经审核分析口径",
    policyEngine ? "策略校验已执行" : "",
    parserBinderPassed ? "受控查询预检通过" : (sqlPolicy.status ? `预检状态=${safeWorkbenchText(sqlPolicy.status)}` : ""),
    rowLimit !== undefined ? `最多返回 ${rowLimit} 行` : ""
  ].filter(Boolean).join("；");
  const evidenceCardText = [
    metricScopeText || "",
    rawRowsBoundaryText
  ].filter(Boolean).join("；");
  const validationStateText = [
    validationState.current_case_only === true || validationSummary.current_case_only === true ? "当前案件范围已确认" : "",
    validationState.readonly === true || validationSummary.readonly === true ? "只读核验" : "",
    validationState.cleaned_analysis_scope_only === true || text(validationSummary.allowed_tables || data.allowed_view_policy).includes("cleaned") ? "清洗后数据范围" : "",
    parserBinderPassed ? "查询预检通过" : "",
    rawRowsBoundaryText
  ].filter(Boolean).join("；");
  const validationTail = `验证状态 ${safeWorkbenchText(executionStatus || runtimeStatus || "unknown")}；可回放${executionStatus === "executed" || runtimeStatus === "ok" ? "执行记录已生成" : "执行记录未完成"}；返回行数=${rowCount === undefined ? "未知" : rowCount}。`;
  const evidenceTail = `核验意见 当前案件、只读、清洗后数据和经审核分析口径；最多返回 ${rowLimit || "受控"} 行；${rawRowsBoundaryText}；不写成法律结论、控制关系或交易路径结论。`;
  const capabilityTail = runtimeStatus === "ok" || executionStatus === "executed"
    ? "能力缺口：本轮未形成阻断；重复自定义口径可登记为后续语义核验能力产品化候选。"
    : "能力缺口：专项资金核算未返回已执行结果；不得编造未验证数值。";
  const visibleRedactionGuard = "最终答复改写约束：只写业务证据、金额、笔数、对象、时间范围和核验边界；不得写数据库、表名、字段名、查询编号或运行态状态词。";
  const reportArtifact = compactReportArtifact(data.report_artifact || {
    path: data.report_path,
    manifest_path: data.manifest_path,
    artifact_id: data.artifact_id,
    artifact_type: data.artifact_type
  });
  return pruneEmpty({
    mode: "controlled_case_workbench",
    tool: toolName,
    runtime_status: safeWorkbenchText(runtimeStatus),
    execution_status: safeWorkbenchText(executionStatus),
    evidence_status: safeWorkbenchText(evidenceStatus),
    sql_policy_summary: sqlPolicyText,
    evidence_card_summary: evidenceCardText,
    metric_scope_summary: metricScopeText,
    validation_summary_text: validationStateText,
    validation_status: validationTail,
    evidence_boundary_status: evidenceTail,
    capability_gap_status: capabilityTail,
    visible_redaction_guard: visibleRedactionGuard,
    answer_delivery_notes: [
      validationTail,
      evidenceTail,
      capabilityTail,
      visibleRedactionGuard,
      sampleNotice
    ].filter(Boolean),
    allowed_view_policy: data.allowed_view_policy ? "当前案件清洗后数据与经审核分析口径" : "",
    table_name: businessTableLabel(data.table_name),
    row_count: rowCount,
    exact_count_contract: exactCountAnswerContract(toolName, data, rowCount),
    result_kind: safeWorkbenchText(data.result_kind),
    where_applied: data.where_applied === true,
    source_scope: arrayOf(data.source_scope).map(businessTableLabel).filter(Boolean).slice(0, 4),
    base_tables: arrayOf(data.base_tables).map(businessTableLabel).filter(Boolean).slice(0, 8),
    error_class: safeWorkbenchText(data.error_class),
    diagnosis: compactCaseSqlDiagnosis(data, {
      preserveSqlIdentifiers: toolName === "diagnose_case_sql" || toolName === "explain_case_sql"
    }),
		plan_summary: compactCaseSqlPlan(data.plan_summary ?? data.plan),
    schema_profile: compactCaseSchemaProfile(data.tables || data.profiles),
    sample_notice: safeWorkbenchText(data.sample_notice),
    preview_records: compactCasePreviewRecords(data),
    history_items: compactWorkbenchHistory(data.items),
    recipes: compactCaseSqlRecipes(data.recipes),
    result_mode: safeWorkbenchText(data.result_mode),
    row_limit: rowLimit,
    requested_limit: requestedLimit,
    returned_row_count: rowCount,
    truncated: booleanOrUndefined(data.truncated),
    result_preview_complete: isRanking && requestedLimit
      ? (rowCount === undefined ? undefined : resultPreview.length >= Math.min(requestedLimit, rowCount))
      : undefined,
    result_preview_truncated: isRanking && requestedLimit
      ? (rowCount === undefined ? undefined : resultPreview.length < Math.min(requestedLimit, rowCount))
      : undefined,
    source_row_dump_exposed: rawRowsExposed,
    columns: arrayOf(data.columns).map(businessColumnLabel).filter(Boolean).slice(0, 12),
    result_preview: resultPreview,
    result_preview_boundary: resultPreview.length
      ? "当前案件小范围聚合或排行预览；可用于答复，不是来源明细展开。"
      : "",
    report_artifact: Object.keys(reportArtifact).length ? reportArtifact : undefined,
    report_path: text(reportArtifact.report_path),
    manifest_path: text(reportArtifact.manifest_path),
    artifact_inspection_status: text(reportArtifact.inspection_status),
    notebook_status: text(data.notebook_status),
    delivery_mode: text(data.delivery_mode),
    cell_count: cellsField.present ? cells.length : undefined,
    executable_cell_count: cellsField.present ? executableCells.length : undefined,
    executed_cell_count: cellsField.present ? executedCells.length : undefined,
    warning_messages: warningMessages,
    boundary: [
      "当前案件范围",
      "只读核验",
      "仅限清洗后数据和经审核分析口径",
      "来源明细不展开进答复上下文",
      "说明核验边界"
    ]
  }) || {};
}

function safeWorkbenchText(value) {
  return translateUserVisibleText(humanizeAgentText(value));
}

function compactWorkbenchResultPreview(data) {
  void data;
  // Raw/aggregate rows are not a public or model-context surface. A future
  // host receipt projection may expose verified claim fields explicitly.
  return [];
}

function compactWorkbenchRecord(record, columnNames = []) {
  const source = objectOf(record);
  const keys = uniqueTexts([...columnNames, ...Object.keys(source)], 16);
  const output = {};
  for (const key of keys) {
    if (!Object.prototype.hasOwnProperty.call(source, key)) continue;
    const value = compactWorkbenchScalar(source[key]);
    if (value !== undefined) {
      const label = businessColumnLabel(key) || safeWorkbenchText(key);
      output[label] = value;
    }
  }
  return output;
}

function compactWorkbenchScalar(value) {
  if (value === null || value === undefined) return undefined;
  if (typeof value === "number" || typeof value === "boolean") return value;
  const raw = text(value);
  if (!raw) return undefined;
  return raw.length > 160 ? `${raw.slice(0, 157)}...` : raw;
}

function compactCaseSqlDiagnosis(data, options = {}) {
  const source = objectOf(data);
  const diagnosis = objectOf(source.diagnosis);
  const preserveSqlIdentifiers = objectOf(options).preserveSqlIdentifiers === true;
  return pruneEmpty({
    error_class: preserveSqlIdentifiers ? text(source.error_class) : safeWorkbenchText(source.error_class),
    likely_cause: preserveSqlIdentifiers ? text(diagnosis.likely_cause) : safeWorkbenchText(diagnosis.likely_cause),
    diagnostics: arrayOf(source.diagnostics).map(preserveSqlIdentifiers ? text : safeWorkbenchText).filter(Boolean).slice(0, 6),
    missing_identifier: preserveSqlIdentifiers ? text(diagnosis.missing_identifier) : businessColumnLabel(diagnosis.missing_identifier),
    candidate_bindings: arrayOf(diagnosis.candidate_bindings).map(preserveSqlIdentifiers ? text : safeWorkbenchText).filter(Boolean).slice(0, 12),
    candidate_columns: arrayOf(diagnosis.candidate_columns).slice(0, 12).map((column) => {
      const item = objectOf(column);
      return pruneEmpty({
        table_name: preserveSqlIdentifiers ? text(item.table_name) : businessTableLabel(item.table_name),
        column_name: preserveSqlIdentifiers ? text(item.column_name) : businessColumnLabel(item.column_name),
        usage: preserveSqlIdentifiers ? text(item.usage) : safeWorkbenchText(item.usage)
      }) || {};
    }).filter((column) => Object.keys(column).length),
    recommended_sql_patterns: arrayOf(diagnosis.recommended_sql_patterns).map(preserveSqlIdentifiers ? text : () => "按可用字段修正后重新核算").filter(Boolean).slice(0, 3),
    corrective_actions: arrayOf(diagnosis.corrective_actions).map(preserveSqlIdentifiers ? text : safeWorkbenchText).filter(Boolean).slice(0, 8)
  }) || {};
}

function compactCaseSqlPlan(plan) {
	const source = objectOf(plan);
	return pruneEmpty({
		plan_kind: text(source.plan_kind),
		estimated_row_upper_bound: strictNonNegativeIntOrUndefined(source.estimated_row_upper_bound),
		estimated_rows: strictNonNegativeIntArrayOrUndefined(source.estimated_rows),
		full_table_scan_possible: consistentBooleanOrUndefined(
			source.full_table_scan_possible,
			source.full_scan_possible
		)
	}) || {};
}

function compactCaseSchemaProfile(tables) {
  return arrayOf(tables).slice(0, 8).map((table) => {
    const source = objectOf(table);
    const coverage = objectOf(source.field_coverage);
    const fields = {};
    for (const [key, value] of Object.entries(coverage).slice(0, 8)) {
      const item = objectOf(value);
      fields[businessColumnLabel(key) || safeWorkbenchText(key)] = pruneEmpty({
        column: businessColumnLabel(item.column),
        coverage_rate: numberOrUndefined(item.coverage_rate),
        distinct_count: nonNegativeIntOrUndefined(item.distinct_count)
      }) || {};
    }
    return pruneEmpty({
      table_name: businessTableLabel(source.table_name),
      role: safeWorkbenchText(source.role),
      row_count: nonNegativeIntOrUndefined(source.row_count),
      profiled_column_count: nonNegativeIntOrUndefined(source.profiled_column_count)
        ?? arrayLengthIfPresent(source, ["columns"]),
      columns: arrayOf(source.columns)
        .slice(0, 10)
        .map((column) => {
          const item = objectOf(column);
          return pruneEmpty({
            column_name: businessColumnLabel(item.column_name || item.sql_name || item.name),
            data_type: safeWorkbenchText(item.data_type),
            null_count: nonNegativeIntOrUndefined(item.null_count),
            distinct_count: nonNegativeIntOrUndefined(item.distinct_count)
          }) || {};
        })
        .filter((item) => Object.keys(item).length),
      field_coverage: fields
    }) || {};
  }).filter((item) => Object.keys(item).length);
}

function compactCasePreviewRecords(data) {
  const source = objectOf(data);
  if (source.raw_rows_exposed === true || source.privacy_projected !== true) return [];
  return arrayOf(source.records)
    .slice(0, 5)
    .map((record) => compactWorkbenchRecord(record, arrayOf(source.columns).map(text)))
    .filter((record) => Object.keys(record).length);
}

function compactWorkbenchHistory(items) {
  return arrayOf(items).slice(0, 8).map((item) => {
    const source = objectOf(item);
    return pruneEmpty({
      tool_name: businessWorkbenchToolLabel(source.tool_name),
      query_digest: source.query_digest ? "可回放记录" : "",
      purpose: humanizeAgentText(source.purpose),
      base_tables: arrayOf(source.base_tables).map(businessTableLabel).filter(Boolean).slice(0, 6),
      duration_ms: nonNegativeIntOrUndefined(source.duration_ms),
      row_count: nonNegativeIntOrUndefined(source.row_count),
      truncated: booleanOrUndefined(source.truncated),
      validation_state: safeWorkbenchText(source.validation_state),
      error_class: safeWorkbenchText(source.error_class),
      created_at: text(source.created_at)
    }) || {};
  }).filter((item) => Object.keys(item).length);
}

function compactCaseSqlRecipes(recipes) {
  return arrayOf(recipes).slice(0, 10).map((recipe) => {
    const source = objectOf(recipe);
    return pruneEmpty({
      recipe_id: source.recipe_id ? "可回放核算模板" : "",
      title: humanizeAgentText(source.title),
      category: safeWorkbenchText(source.category),
      required_parameters: arrayOf(source.required_parameters).map(safeWorkbenchText).filter(Boolean).slice(0, 8),
      validation_notes: arrayOf(source.validation_notes).map(humanizeAgentText).filter(Boolean).slice(0, 3),
      replayable: source.replayable === true
    }) || {};
  }).filter((item) => Object.keys(item).length);
}

const WORKBENCH_ANALYSIS_TABLE_PREFIXES = [
  "analysis_account_",
  "analysis_key_node_",
  "analysis_rule_",
  "analysis_signal_",
  "analysis_trace_",
  "analysis_txn_"
];

const WORKBENCH_COLUMN_HINT_RE = /^(case_id|id|txn_id|row_id|txn_time|txn_ts|txn_ts_val|txn_day|amount|amount_val|amt_sum|balance|balance_val|dc_|dc$|dc_flag|dc_norm|dc_final|dc_val|direction|direction_raw|cash_flag|cash_raw|is_success|success_raw|account|account_key|acct_|acct_no|card_no|card_no_norm|counterparty|cp_|summary|remark|txn_type|clean_duplicate|clean_reversal|clean_failed|file_id|log_id)/iu;

function isWorkbenchSchemaTable(name) {
  const tableName = text(name).toLowerCase();
  if (tableName.startsWith("fc_") && tableName.endsWith("_norm")) return true;
  return WORKBENCH_ANALYSIS_TABLE_PREFIXES.some((prefix) => tableName.startsWith(prefix));
}

function compactSchemaColumns(columns, limit = 28) {
  const names = arrayOf(columns)
    .map((column) => text(objectOf(column).name || column))
    .filter(Boolean);
  const hinted = names.filter((name) => WORKBENCH_COLUMN_HINT_RE.test(name));
  const selected = uniqueTexts([...hinted, ...names], limit);
  return selected.slice(0, limit);
}

function compactSchemaInventory(toolName, data) {
  if (toolName !== "inspect_case_schema" && !text(data.schema_contract)) return {};
  const tables = arrayOf(data.tables).map(objectOf);
  const workbenchTables = tables
    .filter((table) => isWorkbenchSchemaTable(table.table_name))
    .sort((left, right) => {
      const leftName = text(left.table_name);
      const rightName = text(right.table_name);
      const leftTxn = /(?:transaction|txn)/iu.test(leftName) ? 0 : 1;
      const rightTxn = /(?:transaction|txn)/iu.test(rightName) ? 0 : 1;
      if (leftTxn !== rightTxn) return leftTxn - rightTxn;
      return leftName.localeCompare(rightName);
    })
    .slice(0, 10)
    .map((table) => pruneEmpty({
      table_name: businessTableLabel(table.table_name),
      role: safeWorkbenchText(table.role || table.table_type),
      row_count: nonNegativeIntOrUndefined(table.row_count),
      column_count: nonNegativeIntOrUndefined(table.column_count),
      columns: compactSchemaColumns(table.columns, /(?:transaction|txn|rule)/iu.test(text(table.table_name)) ? 34 : 20)
        .map(businessColumnLabel)
        .filter(Boolean),
      columns_truncated: table.columns_truncated === true
    }) || {})
    .filter((table) => text(table.table_name));
  return pruneEmpty({
    schema_status: safeWorkbenchText(data.schema_status),
    table_count: nonNegativeIntOrUndefined(data.table_count),
    returned_table_count: nonNegativeIntOrUndefined(data.returned_table_count),
    include_columns: data.include_columns === true,
    allowed_view_policy: "当前案件清洗后数据与经审核分析口径",
    workbench_allowed_tables: workbenchTables,
    boundary: "只有这些清洗后数据和经审核分析口径适合专项资金核算；未批准来源和未列入口径不能作为安全假设。"
  }) || {};
}

export function compactEvidenceRefsForAgent(refs = {}, limit = 6) {
  const source = objectOf(refs);
  return pruneEmpty({
    query_ids: arrayOf(source.query_ids).map(text).filter(Boolean).slice(0, limit),
    evidence_ids: arrayOf(source.evidence_ids).map(text).filter(Boolean).slice(0, limit),
    path_ids: arrayOf(source.path_ids).map(text).filter(Boolean).slice(0, limit),
    report_ids: arrayOf(source.report_ids).map(text).filter(Boolean).slice(0, limit),
    audit_ref: source.audit_ref
  }) || {};
}

function compactLimit(value, fallback = 5, min = 1, max = 50) {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.max(min, Math.min(Math.trunc(number), max));
}

function requestedRankingLimit(data, fallback = 10) {
  const source = objectOf(data);
  const summary = objectOf(source.summary);
  const filters = objectOf(source.filters);
  return compactLimit(
    source.requested_limit
      || source.row_limit
      || summary.limit
      || filters.limit
      || source.limit,
    fallback,
    1,
    50
  );
}

function requestedTopOutflowLimit(data, fallback = 8) {
  const source = objectOf(data);
  const summary = objectOf(source.summary);
  const filters = objectOf(source.filters);
  return compactLimit(
    source.requested_limit
      || source.top_n
      || source.row_limit
      || summary.limit
      || summary.top_n
      || filters.limit
      || filters.top_n
      || source.limit,
    fallback,
    1,
    50
  );
}

function businessTableLabel(value) {
  const raw = text(value);
  if (!raw) return "";
  if (raw === "analysis_txn_detail_idx" || raw === "fc_transaction_norm" || raw === "fc_transaction") {
    return "清洗后的交易明细";
  }
  if (raw === "analysis_txn_daily_agg") return "交易日聚合";
  if (raw === "analysis_account_dim") return "账户维度";
  if (/^analysis_/iu.test(raw)) return "分析索引";
  if (/^fc_.*_norm$/iu.test(raw)) return "清洗后的业务表";
  if (/^fc_/iu.test(raw)) return "业务表";
  return humanizeAgentText(raw);
}

function businessColumnLabel(value) {
  const raw = text(value);
  if (!raw) return "";
  const normalized = raw.toLowerCase();
  if (/^(?:account_open_name|holder_name|payer_name|payee_name)$/u.test(normalized)) return "户名";
  if (/^(?:account_key|account_no|acct_no|card_no|card_no_norm|source_account|target_account)$/u.test(normalized)) return "账号";
  if (/^(?:counterparty_name|cp_name|cp_name_pick)$/u.test(normalized)) return "对手方名称";
  if (/^(?:counterparty_key|counterparty_account|counterparty_acct_norm|cp_key)$/u.test(normalized)) return "对手方账号";
  if (/^(?:amount|amount_val|amt_sum|metric_value|turnover|turnover_total|inflow|outflow|net_flow|balance|balance_val)$/u.test(normalized)) return "金额";
  if (/^(?:txn_time|txn_ts|txn_ts_val|txn_day|trade_time|date)$/u.test(normalized)) return "交易时间";
  if (/^(?:txn_id|row_id|id)$/u.test(normalized)) return "交易标识";
  if (/^(?:summary|remark|memo|purpose)$/u.test(normalized)) return "摘要/备注";
  if (/^(?:txn_type|business_category|terminal_category)$/u.test(normalized)) return "交易类型";
  if (/^(?:direction|direction_raw|dc|dc_flag|dc_norm|dc_final|dc_val)$/u.test(normalized)) return "收支方向";
  if (/^(?:txn_count|count|row_count)$/u.test(normalized)) return "笔数";
  if (/^(?:rank|rn)$/u.test(normalized)) return "序号";
  if (/^analysis_/iu.test(raw)) return "分析口径字段";
  if (/^fc_/iu.test(raw)) return "清洗字段";
  return safeWorkbenchText(raw);
}

function businessWorkbenchToolLabel(value) {
  const raw = text(value);
  if (!raw) return "";
  if (raw === "run_case_sql") return "专项资金核算";
  if (raw === "count_case_rows") return "精确记录计数";
  if (raw === "explain_case_sql") return "核算预检";
  if (raw === "diagnose_case_sql") return "核算诊断";
  if (raw === "profile_case_schema" || raw === "inspect_case_schema") return "字段范围核验";
  if (raw === "preview_case_rows") return "字段样例核验";
  if (raw === "inspect_workbench_history") return "可回放记录核验";
  if (raw === "case_sql_recipes") return "核算模板";
  if (raw === "create_case_notebook") return "可回放核算记录";
  return safeWorkbenchText(raw);
}

function displayAccountLabel(value) {
  const raw = text(value);
  if (!raw) return "";
  return `账户 ${raw}`;
}

export function compactRankingRows(rows, limit = 5) {
  return arrayOf(rows).slice(0, limit).map((row) => {
    const source = objectOf(row);
    return pruneEmpty({
      rank: nonNegativeIntOrUndefined(source.rank),
      account_key: source.account_key,
      account_display: displayAccountLabel(source.account_key || source.account_no || source.card_no || source.acct_no),
      holder_name: source.holder_name,
      counterparty_key: source.counterparty_key,
      counterparty_account: source.counterparty_account,
      display_name: source.display_name || source.account_open_name,
      account_count: nonNegativeIntOrUndefined(source.account_count),
      txn_count: nonNegativeIntOrUndefined(source.txn_count),
      inflow: compactMoneyFact(source.inflow_total),
      outflow: compactMoneyFact(source.outflow_total),
      turnover: compactMoneyFact(source.turnover_total),
      net_flow: compactMoneyFact(source.net_flow),
      max_single: compactMoneyFact(source.max_single_amount),
      metric: text(source.metric),
      metric_value: compactMoneyFact(source.metric_value),
      largest_date_cluster: source.largest_date_cluster,
      first_txn_at: source.first_txn_at,
      last_txn_at: source.last_txn_at
    }) || {};
  });
}

function compactAccountStatsTopAccounts(stats, limit = 8) {
  const rows = arrayOf(objectOf(stats).accounts)
    .map((row) => {
      const source = objectOf(row);
      const inflow = compactMoneyFact(source.inflow_total ?? source.in_amount ?? source.inflow)?.yuan;
      const outflow = compactMoneyFact(source.outflow_total ?? source.out_amount ?? source.outflow)?.yuan;
      const explicitTurnover = compactMoneyFact(source.turnover_total ?? source.turnover)?.yuan;
      const turnover = explicitTurnover ?? (
        inflow !== undefined && outflow !== undefined ? inflow + outflow : undefined
      );
      return {
        ...source,
        turnover_total: turnover,
        inflow_total: inflow,
        outflow_total: outflow
      };
    })
    .filter((row) => text(row.account_key || row.account_no || row.card_no || row.acct_no))
    .sort((left, right) => (
      (numberOrUndefined(right.turnover_total) ?? Number.NEGATIVE_INFINITY)
      - (numberOrUndefined(left.turnover_total) ?? Number.NEGATIVE_INFINITY)
    ))
    .map((row, index) => ({ ...row, rank: index + 1 }));
  return compactRankingRows(rows, limit);
}

export function compactSeedTxn(seedTxn) {
  const seed = objectOf(seedTxn);
  return pruneEmpty({
    txn_id: seed.txn_id,
    txn_time: seed.txn_time,
    account_key: seed.account_key,
    account_open_name: seed.account_open_name,
    counterparty_key: seed.counterparty_key,
    counterparty_name: seed.counterparty_name,
    amount: compactMoneyFact(seed.amount),
    direction: text(seed.direction || "out"),
    summary: seed.summary,
    txn_type: seed.txn_type,
    business_category: seed.business_category,
    business_category_label: seed.business_category_label,
    file_id: seed.file_id
  }) || {};
}

export function compactTopOutflowRows(rows, limit = 6) {
  return arrayOf(rows).slice(0, limit).map((row) => {
    const source = objectOf(row);
    const seed = compactSeedTxn(source.seed_txn || source);
    const terminalCategoryLabel = translateUserVisibleText(source.terminal_category_label);
    return pruneEmpty({
      rank: nonNegativeIntOrUndefined(source.rank),
      seed_txn: seed,
      terminal_category: source.terminal_category,
      terminal_category_label: terminalCategoryLabel,
      target_account_in_case: source.target_account_in_case,
      downstream_truncated: source.downstream_truncated,
      downstream_candidate_count: arrayLengthIfPresent(source, ["downstream_candidates"])
    }) || {};
  });
}

export function compactHypothesisCards(cards, limit = 8) {
  return arrayOf(cards).slice(0, limit).map((card) => {
    const source = objectOf(card);
    return pruneEmpty({
      card_type: source.card_type,
      title: source.title,
      summary: humanizeAgentText(source.summary || source.finding || source.reason),
      priority: source.priority,
      confidence: source.confidence,
      category: source.category,
      label: source.label,
      txn_count: nonNegativeIntOrUndefined(source.txn_count),
      account_count: nonNegativeIntOrUndefined(source.account_count),
      counterparty_count: nonNegativeIntOrUndefined(source.counterparty_count),
      inflow: compactMoneyFact(source.inflow_total),
      outflow: compactMoneyFact(source.outflow_total),
      turnover: compactMoneyFact(source.turnover_total ?? source.amount_total),
      max_single: compactMoneyFact(source.max_single_amount),
      first_txn_at: source.first_txn_at,
      last_txn_at: source.last_txn_at,
      must_cite: source.must_cite,
      evidence_statement: humanizeAgentText(source.evidence_statement),
      claim_guard: humanizeAgentText(source.claim_guard),
      source_tool: source.source_tool,
      next_actions: arrayOf(source.next_actions).map(humanizeAgentText).filter(Boolean).slice(0, 3)
    }) || {};
  });
}

export function compactProbeFindingCards(rows, limit = 8) {
  return arrayOf(rows).slice(0, limit).map((row) => {
    const source = objectOf(row);
    const label = text(source.label || source.category || source.title || "线索");
    const amount = compactMoneyFact(source.turnover_total ?? source.amount_total);
    return pruneEmpty({
      title: label,
      category: source.category,
      priority: source.priority,
      summary: [
        nonNegativeIntOrUndefined(source.txn_count) ? `${nonNegativeIntOrUndefined(source.txn_count)} 笔` : "",
        nonNegativeIntOrUndefined(source.account_count) ? `${nonNegativeIntOrUndefined(source.account_count)} 个账户` : "",
        nonNegativeIntOrUndefined(source.counterparty_count) ? `${nonNegativeIntOrUndefined(source.counterparty_count)} 个对手` : "",
        amount?.yuan ? `资金流量 ${amount.text}` : "",
        text(source.first_txn_at) || text(source.last_txn_at) ? `${text(source.first_txn_at) || "未知起"} 至 ${text(source.last_txn_at) || "未知止"}` : ""
      ].filter(Boolean).join("；"),
      txn_count: nonNegativeIntOrUndefined(source.txn_count),
      account_count: nonNegativeIntOrUndefined(source.account_count),
      counterparty_count: nonNegativeIntOrUndefined(source.counterparty_count),
      inflow: compactMoneyFact(source.inflow_total),
      outflow: compactMoneyFact(source.outflow_total),
      turnover: amount,
      max_single: compactMoneyFact(source.max_single_amount),
      first_txn_at: source.first_txn_at,
      last_txn_at: source.last_txn_at
    }) || {};
  });
}

export function compactFollowupCandidates(rows, limit = 8) {
  return arrayOf(rows).slice(0, limit).map((row) => {
    const source = objectOf(row);
    return pruneEmpty({
      priority: source.priority,
      target_name: source.target_name || source.name,
      target_account: source.target_account || source.account_key || source.account,
      amount: compactMoneyFact(source.amount_total ?? source.turnover_total),
      txn_count: nonNegativeIntOrUndefined(source.txn_count),
      reason: humanizeAgentText(source.reason || source.summary)
    }) || {};
  });
}

export function humanizeEvidenceStatement(value) {
  return humanizeAgentText(value);
}

export function compactOwnerScope(scope) {
  const source = objectOf(scope);
  const accountKeyRows = firstPresentArray(
    source.account_keys,
    source.direct_account_keys,
    source.expanded_account_keys
  );
  const accountKeys = accountKeyRows?.map(text).filter(Boolean);
  const previewRows = firstPresentArray(
    source.account_keys_preview,
    source.direct_account_keys_preview,
    source.expanded_account_keys_preview
  ) ?? accountKeys;
  return pruneEmpty({
    holder_name: source.holder_name || source.subject?.holder_name,
    id_no_provided: consistentBooleanOrUndefined(
      source.id_no_provided,
      source.subject?.id_no_provided
    ),
    account_count: firstNonNegativeInt(
      source.account_count,
      source.direct_account_keys_count,
      source.expanded_account_keys_count
    ) ?? (accountKeys?.length ? accountKeys.length : undefined),
    account_keys_preview: previewRows?.map(text).filter(Boolean).slice(0, 12),
    account_keys_truncated: consistentBooleanOrUndefined(
      source.account_keys_truncated,
      source.direct_account_keys_truncated,
      source.expanded_account_keys_truncated
    ),
    candidate_account_count: firstNonNegativeInt(source.candidate_accounts_count, source.candidate_account_keys_count),
    boundary: "direct accounts are deterministic scope; candidate accounts are leads, not ownership facts"
  }) || {};
}

function firstPresentArray(...values) {
  for (const value of values) {
    if (value === undefined) continue;
    return Array.isArray(value) ? value : undefined;
  }
  return undefined;
}

function consistentBooleanOrUndefined(...values) {
  let observed;
  for (const value of values) {
    if (value === undefined) continue;
    if (typeof value !== "boolean") return undefined;
    if (observed !== undefined && observed !== value) return undefined;
    observed = value;
  }
  return observed;
}

export function compactFieldQuality(value) {
  const source = objectOf(value);
  const entries = Object.entries(source).slice(0, 10).map(([key, item]) => {
    if (item && typeof item === "object") {
      const nested = objectOf(item);
      return [key, pruneEmpty({
        ratio: numberOrUndefined(nested.ratio ?? nested.coverage_ratio),
        count: nonNegativeIntOrUndefined(nested.count ?? nested.present_count),
        missing: nonNegativeIntOrUndefined(nested.missing ?? nested.missing_count)
      }) || text(JSON.stringify(nested).slice(0, 80))];
    }
    return [key, item];
  });
  return Object.fromEntries(entries);
}

export function compactCoverageFact(coverage) {
  const source = objectOf(coverage);
  return pruneEmpty({
    txn_total: nonNegativeIntOrUndefined(source.txn_total),
    txn_analyzed: nonNegativeIntOrUndefined(source.txn_analyzed),
    account_count: nonNegativeIntOrUndefined(source.account_count),
    selected_account_count: nonNegativeIntOrUndefined(source.selected_account_count)
      ?? arrayLengthIfPresent(source, ["selected_accounts"]),
    date_min: source.date_min,
    date_max: source.date_max,
    field_quality: compactFieldQuality(source.field_quality),
    coverage_status: text(source.coverage_status),
    source_coverage_complete: booleanOrUndefined(
      source.source_coverage_complete ?? source.coverage_complete
    ),
    partial_coverage: booleanOrUndefined(source.partial_coverage),
    source_dataset_snapshot_id: text(
      source.source_dataset_snapshot_id
        || source.reported_dataset_snapshot_id
        || source.dataset_snapshot_id
        || source.snapshot_id
    ),
    semantic_status: canonicalSemanticStatus(source.semantic_status),
    evidence_gaps: compactEvidenceGaps(source.evidence_gaps),
    warnings: arrayOf(source.warnings).slice(0, 6)
  }) || {};
}

export function minimalCoverageFact(coverage) {
  const source = compactCoverageFact(coverage);
  return pruneEmpty({
    txn_total: source.txn_total,
    txn_analyzed: source.txn_analyzed,
    account_count: source.account_count,
    selected_account_count: source.selected_account_count,
    date_min: source.date_min,
    date_max: source.date_max,
    coverage_status: source.coverage_status,
    source_coverage_complete: source.source_coverage_complete,
    partial_coverage: source.partial_coverage,
    source_dataset_snapshot_id: source.source_dataset_snapshot_id,
    semantic_status: source.semantic_status,
    evidence_gaps: arrayOf(source.evidence_gaps).slice(0, 4),
    warnings: arrayOf(source.warnings).slice(0, 2)
  }) || {};
}

export function compactAccountStatsFact(stats) {
  const source = objectOf(stats);
  const summary = objectOf(source.summary || source.group_summary || source.totals || source.metrics || source);
  const inflowValue = summary.inflow_total ?? summary.in_amount ?? summary.inflow ?? source.inflow_total ?? source.in_amount ?? source.inflow;
  const outflowValue = summary.outflow_total ?? summary.out_amount ?? summary.outflow ?? source.outflow_total ?? source.out_amount ?? source.outflow;
  const turnoverValue = summary.turnover_total ?? summary.turnover ?? source.turnover_total ?? source.turnover;
  const maxSingleValue = summary.max_single_amount ?? summary.max_single ?? source.max_single_amount ?? source.max_single;
  const hasStats = [
    summary.account_count,
    source.account_count,
    summary.txn_count,
    source.txn_count,
    inflowValue,
    outflowValue,
    turnoverValue,
    summary.net_flow,
    source.net_flow,
    maxSingleValue
  ].some((item) => item !== undefined && item !== null && item !== "");
  if (!hasStats) {
    return {};
  }
  const inflow = compactMoneyFact(inflowValue);
  const outflow = compactMoneyFact(outflowValue);
  const explicitTurnover = compactMoneyFact(turnoverValue);
  const turnover = explicitTurnover ?? (
    inflow && outflow ? compactMoneyFact(inflow.yuan + outflow.yuan) : undefined
  );
  return pruneEmpty({
    account_count: firstNonNegativeInt(summary.account_count, source.account_count),
    txn_count: firstNonNegativeInt(summary.txn_count, source.txn_count),
    inflow,
    outflow,
    turnover,
    net_flow: compactMoneyFact(summary.net_flow ?? source.net_flow),
    max_single: compactMoneyFact(maxSingleValue),
    first_txn_at: summary.first_txn_at || source.first_txn_at,
    last_txn_at: summary.last_txn_at || source.last_txn_at,
    data_keys: Object.keys(source).slice(0, 12)
  }) || {};
}

export function compactCaseGraphFromScopeMap(scopeMap) {
  const map = objectOf(scopeMap.map);
  const coverage = objectOf(scopeMap.coverage || scopeMap.map?.coverage);
  return pruneEmpty({
    case_id: scopeMap.case_id,
    graph_version: scopeMap.map_version || scopeMap.graph_version,
    nodes: arrayOf(map.nodes).slice(0, 12).map((node) => {
      const source = objectOf(node);
      return pruneEmpty({
        id: source.id,
        label: source.label,
        kind: source.kind,
        status: source.status,
        metrics: source.metrics
      }) || {};
    }),
    edges: arrayOf(map.edges).slice(0, 18),
    mandatory_gates: arrayOf(scopeMap.mandatory_gates || scopeMap.gates).slice(0, 10),
    allowed_next_tools: arrayOf(scopeMap.allowed_next_tools).slice(0, 10),
    coverage: pruneEmpty({
      txn_total: coverage.txn_total,
      txn_analyzed: coverage.txn_analyzed,
      account_count: nonNegativeIntOrUndefined(coverage.account_count),
      holder_group_count: nonNegativeIntOrUndefined(coverage.holder_group_count),
      date_min: coverage.date_min,
      date_max: coverage.date_max,
      field_quality: compactFieldQuality(coverage.field_quality)
    })
  }) || {};
}

export function compactCaseGraphForText(graph) {
  const source = objectOf(graph);
  return pruneEmpty({
    graph_version: source.graph_version,
    case_identity: source.case_identity,
    coverage: source.coverage,
    nodes: arrayOf(source.nodes).slice(0, 8),
    gates: arrayOf(source.mandatory_gates || source.gates).slice(0, 8),
    top_accounts: arrayOf(source.top_accounts).slice(0, 2),
    top_holders: arrayOf(source.top_holders).slice(0, 2),
    top_counterparties: arrayOf(source.top_counterparties).slice(0, 2),
    minimum_roadmap: source.minimum_roadmap,
    boundaries: arrayOf(source.boundaries).slice(0, 4)
  }) || {};
}

function firstByKind(items, kind) {
  if (Array.isArray(items)) {
    return items.find((item) => text(item?.kind) === kind) || {};
  }
  const source = objectOf(items);
  return objectOf(source[kind] || Object.values(source).find((item) => text(item?.kind) === kind));
}

export function compactDuplicateExample(item) {
  const source = objectOf(item);
  return pruneEmpty({
    holder_name: text(source.holder_name || arrayOf(source.holder_names)[0]),
    account_count: nonNegativeIntOrUndefined(source.account_count),
    row_count: nonNegativeIntOrUndefined(source.row_count),
    txn_time: text(source.txn_time || source.first_time),
    amount: compactMoneyFact(source.amount ?? source.max_amount),
    direction: text(source.direction),
    counterparty_name: text(source.counterparty_name),
    summary: text(source.summary || source.txn_type),
    boundary: "候选样本只用于复核，不得直接扣减统计。"
  }) || {};
}

function hasQaQualityFacts(qualityData = {}) {
  const quality = objectOf(qualityData);
  return Boolean(
    quality.import_lineage ||
    quality.cleaning_quality ||
    quality.account_identity_quality ||
    quality.table_counts
  );
}

function hasDuplicateFamilyFacts(duplicateData = {}) {
  const duplicate = objectOf(duplicateData);
  return Boolean(
    duplicate.summary ||
    duplicate.dedupe_policy ||
    duplicate.same_fact_eligibility ||
    duplicate.coverage_status ||
    duplicate.blocker ||
    arrayOf(duplicate.families).length
  );
}

export function compactQaReviewFacts(qualityData = {}, duplicateData = {}) {
  const quality = objectOf(qualityData);
  const duplicate = objectOf(duplicateData);
  const hasQuality = hasQaQualityFacts(quality);
  const hasDuplicate = hasDuplicateFamilyFacts(duplicate);
  if (!hasQuality && !hasDuplicate) {
    return undefined;
  }
  const importLineage = objectOf(quality.import_lineage);
  const importSummary = objectOf(importLineage.summary);
  const transactionImport = firstByKind(importLineage.by_kind, "fc_transaction");
  const cleaningQuality = objectOf(quality.cleaning_quality);
  const flags = objectOf(cleaningQuality.flags);
  const fieldQuality = objectOf(cleaningQuality.field_quality);
  const transactionTimeQuality = objectOf(cleaningQuality.transaction_time_quality);
  const duplicateSummary = objectOf(cleaningQuality.duplicate_summary);
  const duplicateExamples = objectOf(cleaningQuality.duplicate_examples);
  const accountQuality = objectOf(quality.account_identity_quality);
  const duplicateToolSummary = objectOf(duplicate.summary);
  const sameHolder = objectOf(duplicateToolSummary.same_holder_same_fact);
  const fullCase = objectOf(duplicateToolSummary.full_case_review_only);
  const policy = objectOf(duplicate.dedupe_policy);
  const qualitySameFactEligible = sameFactEligibilityComplete(cleaningQuality.same_fact_eligibility);
  const duplicateSameFactEligible = sameFactEligibilityComplete(duplicate.same_fact_eligibility);
  const eligibleDuplicateSummary = qualitySameFactEligible ? duplicateSummary : {};
  const eligibleDuplicateExamples = qualitySameFactEligible ? duplicateExamples : {};
  const eligibleSameHolder = duplicateSameFactEligible ? sameHolder : {};
  const eligibleFullCase = duplicateSameFactEligible ? fullCase : {};
  const hasImportLineage = Object.keys(importLineage).length > 0;
  const hasCleaningFlags = Object.keys(flags).length > 0;
  const hasTimeQuality = Object.keys(transactionTimeQuality).length > 0;
  const hasSameFactRisk = Object.keys(eligibleDuplicateSummary).length > 0
    || Object.keys(eligibleSameHolder).length > 0
    || Object.keys(eligibleFullCase).length > 0;
  const hasFieldGaps = Object.keys(flags).length > 0
    || Object.keys(accountQuality).length > 0
    || Object.keys(fieldQuality).length > 0;
  const unregisteredAccounts = firstArrayField(accountQuality, ["unregistered_accounts"]);
  const naturalDuplicateSamples = firstArrayField(eligibleDuplicateExamples, ["natural_duplicate_within_account"]);
  const crossAccountSamples = firstArrayField(eligibleDuplicateExamples, ["same_fact_cross_account"]);
  const sameHolderSamples = firstArrayField(duplicateSameFactEligible ? duplicate : {}, ["families"]);
  const hasSamples = [naturalDuplicateSamples, crossAccountSamples, sameHolderSamples]
    .some((field) => field.present);
  const hasPolicy = Object.keys(policy).length > 0;
  const abnormalEarlyRows = nonNegativeIntOrUndefined(transactionTimeQuality.abnormal_early_txn_time_rows);

  return pruneEmpty({
    import_lineage: hasImportLineage ? {
      all_file_logs: nonNegativeIntOrUndefined(importSummary.file_log_count),
      all_rows_total: nonNegativeIntOrUndefined(importSummary.rows_total),
      all_rows_imported_norm: nonNegativeIntOrUndefined(importSummary.rows_imported_norm),
      all_rows_dedup: nonNegativeIntOrUndefined(importSummary.rows_dedup),
      transaction_file_logs: nonNegativeIntOrUndefined(transactionImport.file_log_count),
      transaction_rows_total: nonNegativeIntOrUndefined(transactionImport.rows_total),
      transaction_rows_imported_norm: nonNegativeIntOrUndefined(transactionImport.rows_imported_norm),
      transaction_rows_dedup: nonNegativeIntOrUndefined(transactionImport.rows_dedup),
      transaction_rows_error: nonNegativeIntOrUndefined(transactionImport.rows_error),
      transaction_rows_skipped_non_data: nonNegativeIntOrUndefined(transactionImport.rows_skipped_non_data)
    } : undefined,
    cleaning_flags: hasCleaningFlags ? {
      txn_total: nonNegativeIntOrUndefined(flags.txn_total),
      clean_duplicate: nonNegativeIntOrUndefined(flags.clean_duplicate),
      clean_failed: nonNegativeIntOrUndefined(flags.clean_failed),
      clean_reversal: nonNegativeIntOrUndefined(flags.clean_reversal),
      clean_dc_inferred: nonNegativeIntOrUndefined(flags.clean_dc_inferred),
      clean_card_filled: nonNegativeIntOrUndefined(flags.clean_card_filled),
      clean_account_filled: nonNegativeIntOrUndefined(flags.clean_account_filled)
    } : undefined,
    time_quality: hasTimeQuality ? {
      txn_time_min: text(transactionTimeQuality.txn_time_min),
      txn_time_max: text(transactionTimeQuality.txn_time_max),
      abnormal_early_txn_time_rows: abnormalEarlyRows,
      missing_txn_time_rows: nonNegativeIntOrUndefined(transactionTimeQuality.missing_txn_time_rows),
      boundary: abnormalEarlyRows !== undefined && abnormalEarlyRows > 0
        ? "最早交易时间若早于 1900 年，只能写成解析时间质量边界，不能写成正常交易年份或真实发生时间。"
        : ""
    } : undefined,
    same_fact_risk: hasSameFactRisk ? {
      row_hash_duplicate_groups: nonNegativeIntOrUndefined(eligibleDuplicateSummary.row_hash_duplicate_groups),
      natural_within_account_groups: nonNegativeIntOrUndefined(eligibleDuplicateSummary.natural_duplicate_within_account_groups),
      natural_within_account_extra_rows: nonNegativeIntOrUndefined(eligibleDuplicateSummary.natural_duplicate_within_account_extra_rows),
      same_holder_group_count: firstNonNegativeInt(
        eligibleSameHolder.group_count,
        eligibleDuplicateSummary.same_holder_same_fact_groups
      ),
      same_holder_extra_rows: firstNonNegativeInt(
        eligibleSameHolder.extra_rows,
        eligibleDuplicateSummary.same_holder_same_fact_extra_rows
      ),
      same_holder_candidate_duplicate_amount: numberOrUndefined(
        eligibleSameHolder.candidate_duplicate_amount
          ?? eligibleDuplicateSummary.same_holder_same_fact_candidate_duplicate_amount
      ),
      full_case_group_count: nonNegativeIntOrUndefined(eligibleFullCase.group_count),
      full_case_extra_rows: nonNegativeIntOrUndefined(eligibleFullCase.extra_rows),
      full_case_cross_holder_group_count: firstNonNegativeInt(
        eligibleFullCase.cross_holder_group_count,
        eligibleDuplicateSummary.full_case_same_fact_cross_holder_groups
      ),
      cross_account_same_fact_groups: nonNegativeIntOrUndefined(eligibleDuplicateSummary.same_fact_cross_account_groups),
      cross_account_same_fact_extra_rows: nonNegativeIntOrUndefined(eligibleDuplicateSummary.same_fact_cross_account_extra_rows),
      same_txn_id_cross_account_groups: nonNegativeIntOrUndefined(eligibleDuplicateSummary.same_txn_id_cross_account_groups),
      same_txn_id_cross_account_extra_rows: nonNegativeIntOrUndefined(eligibleDuplicateSummary.same_txn_id_cross_account_extra_rows),
      full_case_blind_dedupe_allowed: booleanOrUndefined(policy.full_case_blind_dedupe_allowed),
      deduct_from_totals_allowed: booleanOrUndefined(policy.deduct_from_totals_allowed)
    } : undefined,
    field_gaps: hasFieldGaps ? {
      txn_blank_holder_rows: firstNonNegativeInt(flags.txn_blank_holder_rows, accountQuality.txn_blank_holder_rows),
      txn_blank_holder_account_count: firstNonNegativeInt(
        flags.txn_blank_holder_account_count,
        accountQuality.txn_blank_holder_account_count
      ),
      txn_blank_holder_resolved_account_count: nonNegativeIntOrUndefined(accountQuality.txn_blank_holder_resolved_account_count),
      txn_blank_holder_unregistered_account_count: nonNegativeIntOrUndefined(accountQuality.txn_blank_holder_unregistered_account_count),
      account_dim_account_count: nonNegativeIntOrUndefined(accountQuality.account_count),
      account_dim_unregistered_account_count: nonNegativeIntOrUndefined(accountQuality.unregistered_account_count),
      account_dim_unregistered_txn_count: nonNegativeIntOrUndefined(accountQuality.unregistered_txn_count),
      account_dim_unregistered_turnover: numberOrUndefined(accountQuality.unregistered_turnover_total),
      account_dim_unregistered_accounts: unregisteredAccounts.present ? unregisteredAccounts.value.slice(0, 5).map((item) => {
        const source = objectOf(item);
        return pruneEmpty({
          account_key: text(source.account_key),
          txn_count: nonNegativeIntOrUndefined(source.txn_count),
          turnover: compactMoneyFact(source.turnover_total)
        }) || {};
      }) : undefined,
      blank_counterparty_name_rows: nonNegativeIntOrUndefined(flags.blank_counterparty_name_rows),
      blank_counterparty_account_rows: nonNegativeIntOrUndefined(flags.blank_counterparty_account_rows),
      blank_counterparty_both_rows: nonNegativeIntOrUndefined(flags.blank_counterparty_both_rows),
      counterparty_name_present_rate: numberOrUndefined(fieldQuality.counterparty_name?.rate),
      counterparty_account_present_rate: numberOrUndefined(fieldQuality.counterparty_acct_norm?.rate)
    } : undefined,
    samples: hasSamples ? {
      natural_duplicate_within_account: naturalDuplicateSamples.present
        ? naturalDuplicateSamples.value.slice(0, 2).map(compactDuplicateExample)
        : undefined,
      same_fact_cross_account: crossAccountSamples.present
        ? crossAccountSamples.value.slice(0, 2).map(compactDuplicateExample)
        : undefined,
      same_holder_families: sameHolderSamples.present
        ? sameHolderSamples.value.slice(0, 2).map(compactDuplicateExample)
        : undefined
    } : undefined,
    policy: {
      status: hasPolicy ? text(policy.status) : "unknown",
      minimum_safe_scope: hasPolicy ? text(policy.minimum_safe_scope) : undefined,
      full_case_blind_dedupe_allowed: hasPolicy ? booleanOrUndefined(policy.full_case_blind_dedupe_allowed) : undefined,
      deduct_from_totals_allowed: hasPolicy ? booleanOrUndefined(policy.deduct_from_totals_allowed) : undefined,
      boundary: "清洗标记不能单独证明无重复；同事实/换卡候选未经同一人账户集合逐族确认，不得扣减统计，不能全流水去重。"
    }
  }) || {};
}

export function compactFlowEdgeForAgent(edge) {
  const source = objectOf(edge);
  return pruneEmpty({
    rank: nonNegativeIntOrUndefined(source.rank),
    edge_status: source.edge_status,
    from_label: source.from_label || source.from,
    from_account: source.from_account || source.from,
    to_label: source.to_label || source.to,
    to_account: source.to_account || source.to,
    txn_id: source.txn_id,
    txn_time: source.txn_time,
    amount: compactMoneyFact(source.amount?.yuan ?? source.amount),
    direction: source.direction,
    summary: source.summary,
    txn_type: source.txn_type,
    business_category: source.business_category,
    business_category_label: source.business_category_label,
    missing_fields: arrayOf(source.missing_fields).slice(0, 4),
    boundary: source.boundary
  }) || {};
}

export function compactFlowGraphForText(graph, limit = 10) {
  const source = objectOf(graph);
  const nodesField = firstArrayField(source, ["nodes"]);
  const edgesField = firstArrayField(source, ["edges"]);
  const drawableEdgesField = firstArrayField(source, ["drawable_edges"]);
  const reviewEdgesField = firstArrayField(source, ["review_edges"]);
  const edges = edgesField.value;
  const statusCounts = edgesField.present ? edges.reduce((counts, edge) => {
    const status = text(objectOf(edge).edge_status) || "unknown";
    counts[status] = hasOwn(counts, status) ? counts[status] + 1 : 1;
    return counts;
  }, {}) : undefined;
  const supportedEdges = drawableEdgesField.present
    ? drawableEdgesField.value
    : (edgesField.present ? edges.filter((edge) => text(objectOf(edge).edge_status) === "supported") : []);
  const boundaryEdges = reviewEdgesField.present
    ? reviewEdgesField.value
    : (edgesField.present ? edges.filter((edge) => text(objectOf(edge).edge_status) !== "supported") : []);
  const hasSupportedEdgeSource = drawableEdgesField.present || edgesField.present;
  const hasBoundaryEdgeSource = reviewEdgesField.present || edgesField.present;
  const edgePreview = supportedEdges.slice(0, limit).map(compactFlowEdgeForAgent);
  return pruneEmpty({
    graph_version: source.graph_version,
    node_count: nonNegativeIntOrUndefined(source.node_count)
      ?? (nodesField.present ? nodesField.value.length : undefined),
    edge_count: nonNegativeIntOrUndefined(source.edge_count)
      ?? (edgesField.present ? edges.length : undefined),
    supported_edge_count: nonNegativeIntOrUndefined(source.supported_edge_count)
      ?? (edgesField.present ? edges.filter((edge) => objectOf(edge).edge_status === "supported").length : undefined),
    incomplete_edge_count: nonNegativeIntOrUndefined(source.incomplete_edge_count)
      ?? (hasBoundaryEdgeSource ? boundaryEdges.length : undefined),
    edge_status_counts: Object.keys(objectOf(source.edge_status_counts)).length
      ? source.edge_status_counts
      : statusCounts,
    edge_preview: hasSupportedEdgeSource ? edgePreview : undefined,
    drawable_edge_preview: hasSupportedEdgeSource ? edgePreview : undefined,
    supported_edge_preview: hasSupportedEdgeSource
      ? supportedEdges.slice(0, limit).map(compactFlowEdgeForAgent)
      : undefined,
    boundary_edge_preview: hasBoundaryEdgeSource
      ? boundaryEdges.slice(0, limit).map(compactFlowEdgeForAgent)
      : undefined,
    evidence_boundaries: arrayOf(source.evidence_boundaries).slice(0, limit),
    boundaries: arrayOf(source.boundaries).slice(0, 3),
    drawing_rule: "Only draw arrows from drawable_edge_preview or drawable_edges. Do not add narrative-only arrows."
  }) || {};
}

function compactFieldCoverage(value) {
  const source = objectOf(value);
  return pruneEmpty({
    counterparty: source.counterparty,
    branch_name: source.branch_name,
    location: source.location,
    summary: source.summary,
    remark: source.remark,
    success_status: source.success_status,
    ip_addr: source.ip_addr,
    mac_addr: source.mac_addr,
    cash_flag: source.cash_flag,
    file_id: source.file_id
  }) || {};
}

function compactDetectorCoverage(value) {
  return Object.entries(objectOf(value))
    .filter(([, enabled]) => enabled === true)
    .map(([key]) => key)
    .slice(0, 24);
}

function compactSourceAuditFact(value) {
  const source = objectOf(value);
  return pruneEmpty({
    audit_status: source.audit_status,
    schema_status: source.schema_status,
    pipeline_counts: objectOf(source.pipeline_counts),
    missing_core_tables: arrayOf(source.missing_core_tables).slice(0, 6),
    potential_unindexed_sources: arrayOf(source.potential_unindexed_sources).slice(0, 6),
    schema_role_summary: arrayOf(source.schema_role_summary).slice(0, 8).map((row) => {
      const item = objectOf(row);
      return pruneEmpty({
        role: item.role,
        table_count: nonNegativeIntOrUndefined(item.table_count),
        row_count: nonNegativeIntOrUndefined(item.row_count)
      }) || {};
    }),
    warnings: arrayOf(source.warnings).slice(0, 6),
    raw_rows_exposed: source.raw_rows_exposed
  }) || {};
}

function compactCaseReconciliationFact(value) {
  const source = objectOf(value);
  const counts = objectOf(source.counts);
  const reconciliation = objectOf(source.reconciliation);
  const fieldQuality = objectOf(source.field_quality);
  return pruneEmpty({
    counts: pruneEmpty({
      fc_transaction_norm: nonNegativeIntOrUndefined(counts.fc_transaction_norm),
      analysis_txn_detail_idx: nonNegativeIntOrUndefined(counts.analysis_txn_detail_idx),
      analysis_rule_txn_idx: nonNegativeIntOrUndefined(counts.analysis_rule_txn_idx),
      analysis_txn_daily_agg: nonNegativeIntOrUndefined(counts.analysis_txn_daily_agg),
      analysis_account_dim: nonNegativeIntOrUndefined(counts.analysis_account_dim),
      import_file_log: nonNegativeIntOrUndefined(counts.import_file_log)
    }) || {},
    reconciliation,
    field_quality: pruneEmpty({
      detail_idx: fieldQuality.detail_idx,
      transaction_norm: fieldQuality.transaction_norm
    }) || {},
    all_transaction_indexes_matched: source.all_transaction_indexes_matched,
    warnings: arrayOf(source.warnings).slice(0, 6)
  }) || {};
}

function compactReportArtifact(value) {
  const source = objectOf(value);
  const files = arrayOf(source.files).map((file) => {
    const item = objectOf(file);
    return pruneEmpty({
      path: text(item.path),
      size: nonNegativeIntOrUndefined(item.size),
      sha256: text(item.sha256),
      inspection_status: text(item.inspection_status)
    }) || {};
  }).filter((item) => text(item.path));
  return pruneEmpty({
    report_path: text(source.report_path || source.path),
    manifest_path: text(source.manifest_path),
    artifact_id: text(source.artifact_id),
    artifact_type: text(source.artifact_type),
    inspection_status: text(source.inspection_status),
    files,
    table_headers: source.table_headers,
    key_numbers: source.key_numbers
  }) || {};
}

function compactLocalFullCasePackage(source, safetyMetadata = {}) {
  const localPackage = objectOf(source.full_case_package);
  if (!Object.keys(localPackage).length) return {};
  const coverage = objectOf(localPackage.coverage);
  const detail = objectOf(coverage.transaction_detail);
  const safety = objectOf(safetyMetadata);
  return pruneEmpty({
    package_type: "full_case_analysis_tree",
    case_id: source.case_id,
    semantic_status: safety.semantic_status,
    coverage_status: safety.coverage_status,
    source_coverage_complete: safety.source_coverage_complete,
    partial_coverage: safety.partial_coverage,
    source_dataset_snapshot_id: safety.source_dataset_snapshot_id,
    coverage: compactCoverageFact({
      txn_total: firstNonNegativeInt(detail.row_count, detail.txn_count),
      txn_analyzed: firstNonNegativeInt(detail.txn_count, detail.row_count),
      account_count: nonNegativeIntOrUndefined(detail.account_count),
      date_min: detail.first_txn_at,
      date_max: detail.last_txn_at,
      coverage_status: safety.coverage_status,
      source_coverage_complete: safety.source_coverage_complete,
      partial_coverage: safety.partial_coverage,
      source_dataset_snapshot_id: safety.source_dataset_snapshot_id,
      semantic_status: safety.semantic_status,
      evidence_gaps: compactEvidenceGaps(localPackage.evidence_gaps, safety.evidence_gaps)
    }),
    scope_stats: pruneEmpty({
      txn_count: firstNonNegativeInt(detail.txn_count, detail.row_count),
      account_count: nonNegativeIntOrUndefined(detail.account_count),
      counterparty_count: firstNonNegativeInt(detail.counterparty_name_count, detail.counterparty_key_count),
      first_txn_at: detail.first_txn_at,
      last_txn_at: detail.last_txn_at,
      in_amount: compactMoneyFact(detail.inflow_amount),
      out_amount: compactMoneyFact(detail.outflow_amount),
      turnover: compactMoneyFact(detail.turnover_amount)
    }) || {},
    top_holders: arrayOf(localPackage.holder_rankings).slice(0, 8),
    top_destinations: arrayOf(localPackage.destination_outflows).slice(0, 8),
    top_sources: arrayOf(localPackage.source_inflows).slice(0, 8),
    top_outflows: arrayOf(localPackage.top_holder_outflows).slice(0, 8),
    evidence_gaps: compactEvidenceGaps(localPackage.evidence_gaps, safety.evidence_gaps),
    report_validation_status: text(objectOf(source.validation_state).status || objectOf(source.validation_summary).status),
    same_turn_stop: "Use only fields actually returned by this current-turn package. Missing or partial fields remain evidence gaps and cannot be upgraded into whole-case conclusions or report publication."
  }) || {};
}

function compactFullCasePackage(data, safetyMetadata = {}) {
  const source = objectOf(data);
  const safety = objectOf(safetyMetadata);
  const localPackage = compactLocalFullCasePackage(source, safety);
  if (Object.keys(localPackage).length) return localPackage;
  const coverageEnvelope = objectOf(source.case_scope_coverage);
  const coverage = objectOf(coverageEnvelope.coverage || objectOf(source.investigation_lab).coverage);
  const scopeStats = objectOf(coverageEnvelope.scope_stats);
  const rankings = objectOf(source.rankings);
  const lab = objectOf(source.investigation_lab);
  const plan = objectOf(source.analysis_plan);
  return pruneEmpty({
    package_type: "full_case_analysis_tree",
    case_id: source.case_id || coverage.case_id || lab.case_id,
    semantic_status: safety.semantic_status,
    coverage_status: safety.coverage_status,
    source_coverage_complete: safety.source_coverage_complete,
    partial_coverage: safety.partial_coverage,
    source_dataset_snapshot_id: safety.source_dataset_snapshot_id,
    coverage: compactCoverageFact({
      ...coverage,
      coverage_status: safety.coverage_status || coverage.coverage_status,
      source_coverage_complete: hasOwn(safety, "source_coverage_complete")
        ? safety.source_coverage_complete
        : coverage.source_coverage_complete ?? coverage.coverage_complete,
      partial_coverage: hasOwn(safety, "partial_coverage")
        ? safety.partial_coverage
        : coverage.partial_coverage,
      source_dataset_snapshot_id: safety.source_dataset_snapshot_id
        || coverage.source_dataset_snapshot_id
        || coverage.reported_dataset_snapshot_id
        || coverage.dataset_snapshot_id,
      semantic_status: safety.semantic_status || coverage.semantic_status,
      evidence_gaps: compactEvidenceGaps(coverage.evidence_gaps, safety.evidence_gaps)
    }),
    scope_stats: pruneEmpty({
      txn_count: nonNegativeIntOrUndefined(scopeStats.txn_count),
      account_count: nonNegativeIntOrUndefined(scopeStats.account_count),
      counterparty_count: nonNegativeIntOrUndefined(scopeStats.counterparty_count),
      first_txn_at: scopeStats.first_txn_at || coverage.date_min,
      last_txn_at: scopeStats.last_txn_at || coverage.date_max,
      in_amount: compactMoneyFact(scopeStats.in_amount ?? coverage.inflow_total),
      out_amount: compactMoneyFact(scopeStats.out_amount ?? coverage.outflow_total),
      turnover: compactMoneyFact(scopeStats.turnover_total ?? coverage.turnover_total),
      in_txn_count: nonNegativeIntOrUndefined(scopeStats.in_txn_count),
      out_txn_count: nonNegativeIntOrUndefined(scopeStats.out_txn_count),
      directed_txn_count: firstNonNegativeInt(scopeStats.directed_txn_count, coverage.directed_transaction_rows),
      undirected_txn_count: firstNonNegativeInt(scopeStats.undirected_txn_count, coverage.undirected_transaction_rows),
      source_file_count: nonNegativeIntOrUndefined(coverage.source_file_count)
    }) || {},
    field_quality: compactFieldCoverage(coverage.field_quality || scopeStats.field_coverage),
    top_accounts: compactRankingRows(rankings.accounts, 8),
    top_holders: compactRankingRows(rankings.holders, 8),
    detector_coverage: compactDetectorCoverage(source.detector_coverage),
    candidate_accounts: pruneEmpty({
      count: Array.isArray(source.candidate_accounts)
        ? source.candidate_accounts.length
        : nonNegativeIntOrUndefined(objectOf(source.candidate_accounts).count),
      stats: source.candidate_account_stats
    }),
    plan_lanes: arrayOf(plan.lanes).slice(0, 8).map((lane) => {
      const item = objectOf(lane);
      return pruneEmpty({
        lane: item.lane || item.id || item.name,
        intent: item.intent || item.analysis_goal,
        status: item.status,
        allowed_tools: arrayOf(item.allowed_tools).slice(0, 6)
      }) || {};
    }),
    hypothesis_cards: compactHypothesisCards(lab.hypothesis_cards, 8),
    mandatory_review_cards: compactHypothesisCards(lab.mandatory_review_cards, 8),
    evidence_gaps: compactEvidenceGaps(lab.evidence_gaps, source.evidence_gaps, safety.evidence_gaps),
    next_actions: compactNextActions(lab.recommended_next_actions || source.recommended_next_actions, 8),
    source_audit: compactSourceAuditFact(source.source_audit),
    case_reconciliation: compactCaseReconciliationFact(source.case_reconciliation),
    report_validation_status: objectOf(source.report_validation).status,
    same_turn_stop: "Use only fields actually returned by this current-turn package. Missing or partial fields remain evidence gaps and cannot be upgraded into whole-case conclusions or report publication."
  }) || {};
}

function compactEvidencePackNode(node) {
  const source = objectOf(node);
  return pruneEmpty({
    display_name: source.display_name,
    node_key: source.node_key,
    txn_count: nonNegativeIntOrUndefined(source.txn_count),
    total_amount: compactMoneyFact(source.total_amount),
    in_amount: compactMoneyFact(source.in_amount),
    out_amount: compactMoneyFact(source.out_amount),
    amount_share: source.amount_share,
    quality_label: source.quality_label,
    reasons: arrayOf(source.reasons).map(humanizeAgentText).filter(Boolean).slice(0, 3)
  }) || {};
}

function compactEvidencePackSignal(signal) {
  const source = objectOf(signal);
  return pruneEmpty({
    title: source.title || source.signal_type,
    signal_type: source.signal_type,
    severity: source.severity,
    confidence: source.confidence,
    support_count: nonNegativeIntOrUndefined(source.support_count),
    summary: humanizeAgentText(source.summary),
    evidence_status: source.counter_evidence_present ? "needs_review" : "lead"
  }) || {};
}

function compactEvidencePackSample(row) {
  const source = objectOf(row);
  return pruneEmpty({
    txn_time: source.txn_time,
    amount: compactMoneyFact(source.amount),
    direction: source.direction,
    account_key: source.account_key,
    counterparty_name: source.counterparty_name,
    summary: humanizeAgentText(source.summary || source.remark),
    branch_name: source.branch_name,
    evidence_status: "representative_sample_not_total"
  }) || {};
}

function compactEvidencePack(data) {
  const source = objectOf(data);
  const coverage = objectOf(source.coverage);
  const stats = objectOf(source.global_stats);
  const quality = objectOf(source.data_quality || coverage.quality_summary);
  return pruneEmpty({
    package_type: "visual_evidence_delivery_pack",
    summary: humanizeAgentText(source.summary_text),
    scope: objectOf(source.scope),
    coverage: pruneEmpty({
      scope_type: coverage.scope_type,
      scope_source: coverage.scope_source,
      scanned_rows: nonNegativeIntOrUndefined(coverage.scanned_rows),
      total_case_rows: nonNegativeIntOrUndefined(coverage.total_case_rows),
      time_range: coverage.time_range,
      field_coverage: compactFieldCoverage(coverage.field_coverage),
      quality_label: coverage.quality_label || quality.label
    }),
    global_stats: pruneEmpty({
      txn_count: nonNegativeIntOrUndefined(stats.txn_count),
      account_count: nonNegativeIntOrUndefined(stats.account_count),
      counterparty_count: nonNegativeIntOrUndefined(stats.counterparty_count),
      in_txn_count: nonNegativeIntOrUndefined(stats.in_txn_count),
      out_txn_count: nonNegativeIntOrUndefined(stats.out_txn_count),
      in_amount: compactMoneyFact(stats.in_amount),
      out_amount: compactMoneyFact(stats.out_amount),
      net_amount: compactMoneyFact(stats.net_amount)
    }),
    key_nodes: arrayOf(source.key_nodes).slice(0, 8).map(compactEvidencePackNode),
    pattern_signals: arrayOf(source.pattern_signals).slice(0, 8).map(compactEvidencePackSignal),
    representative_samples: arrayOf(source.representative_samples).slice(0, 6).map(compactEvidencePackSample),
    table_inventory: [
      "证据表格: key_nodes / representative_samples, evidence status=fact_or_sample",
      "Top20 图表: key_nodes sorted by total_amount/amount_share; recommended forms=排行榜/柱状图; metric=amount, unit=yuan",
      "特征表: pattern_signals with severity/confidence/support_count; evidence status=lead/needs_review",
      "资金流向表: in/out/global_stats and representative direction; not a supported path and not a legal conclusion",
      "续调表: low quality fields, missing counterparties, candidate signals, 证据缺口",
      "附件目录: scope, coverage, quality notes, compact samples; appendix/workbook inventory",
      "看板建议: dashboard cards for scope, quality, Top20, pattern signals, and evidence requests",
      "表图边界: 排行榜/柱状/趋势/矩阵/热力/dashboard/appendix/workbook 不是交易路径，不得写成控制关系，不能写成法律结论",
      "next owner skill: fact owner, graph-visualization, claim-review, evidence-request, full-case-analysis only when the user changes task"
    ],
    visual_contract: {
      scope: "state scope/source and selected accounts or case scope",
      unit: "yuan/count/ratio",
      time_window: "use coverage.time_range",
      metric: "txn_count/in_amount/out_amount/total_amount/support_count",
      direction: "in/out/both; charts are not transaction paths",
      evidence_status: "fact/statistical_feature/lead/needs_review/unavailable"
    },
    same_turn_stop: "After get_case_scope_map, get_evidence_pack, and at most one targeted rank/trace/list tool, answer with inventory and gaps; do not run full-case analysis only to assemble visuals."
  }) || {};
}

export function compactKeyFactsForText(facts) {
  const source = objectOf(facts);
  return pruneEmpty({
    coverage: compactCoverageFact(source.coverage || {}),
    gates: arrayOf(source.gates).slice(0, 5).map((gate) => {
      const item = objectOf(gate);
      return pruneEmpty({ gate: item.gate, tool: item.tool, status: item.status, reason: item.reason }) || {};
    }),
    top_accounts: arrayOf(source.top_accounts).slice(0, 2),
    top_holders: arrayOf(source.top_holders).slice(0, 2),
    top_counterparties: arrayOf(source.top_counterparties).slice(0, 2),
    allowed_next_tools: arrayOf(source.allowed_next_tools).slice(0, 5),
    casegraph_roadmap: source.casegraph_roadmap || source.minimum_roadmap,
    casegraph_summary: source.casegraph
      ? {
          graph_version: source.casegraph.graph_version,
          node_count: arrayLengthIfPresent(source.casegraph, ["nodes"]),
          gate_count: arrayLengthIfPresent(source.casegraph, ["mandatory_gates", "gates"]),
          boundary_count: arrayLengthIfPresent(source.casegraph, ["boundaries"])
        }
      : undefined,
	    holder_scope: source.holder_scope,
	    holder_coverage: source.holder_coverage ? compactCoverageFact(source.holder_coverage) : undefined,
	    holder_account_stats: source.holder_account_stats ? compactAccountStatsFact(source.holder_account_stats) : undefined,
	    holder_top_accounts: arrayOf(source.holder_top_accounts).length ? compactRankingRows(source.holder_top_accounts, 8) : undefined,
	    account_key: source.account_key,
	    account_coverage: source.account_coverage ? compactCoverageFact(source.account_coverage) : undefined,
	    account_stats: source.account_stats ? compactAccountStatsFact(source.account_stats) : undefined,
	    counterparty_rankings: arrayOf(source.counterparty_rankings).slice(0, 8),
    via_counterparty_rankings: arrayOf(source.via_counterparty_rankings).slice(0, 8),
    via_counterparty_date_start: source.via_counterparty_date_start,
    via_financial_product_clues: arrayOf(source.via_financial_product_clues).slice(0, 6),
    top_outflows: arrayOf(source.top_outflows).slice(0, requestedTopOutflowLimit(source, arrayOf(source.top_outflows).length || 8)),
    source_to_via_summary: source.source_to_via_summary,
    source_to_via_date_window_summary: source.source_to_via_date_window_summary,
    source_to_via_core_account_summary: source.source_to_via_core_account_summary,
    fund_flow_fact_pack: source.fund_flow_fact_pack,
    graph_delivery_contract: source.graph_delivery_contract,
    flow_graph: source.flow_graph ? compactFlowGraphForText(source.flow_graph, 5) : undefined,
    flow_graph_answer: source.flow_graph_answer,
    flow_graph_scope_stats: source.flow_graph_scope_stats,
    mandatory_review_cards: arrayOf(source.mandatory_review_cards).slice(0, 4),
    hypothesis_cards: arrayOf(source.hypothesis_cards).slice(0, 4),
    plan_lanes: arrayOf(source.plan_lanes).slice(0, 4),
    workbench_gap: source.workbench_gap,
    qa_review: source.qa_review,
    source_scope_stats: source.source_scope_stats,
    via_scope_stats: source.via_scope_stats,
    source_seed_count: nonNegativeIntOrUndefined(source.source_seed_count),
    downstream_seed_count: nonNegativeIntOrUndefined(source.downstream_seed_count),
    rankings: arrayOf(source.rankings).slice(0, 20),
    full_case_package: source.full_case_package,
    evidence_pack: source.evidence_pack,
    cleaned_export: source.cleaned_export
  }) || {};
}

export function compactToolPayloadForAgent(payload) {
  const source = objectOf(payload);
  const response = source.response ? unwrapSkillEnvelope(source.response) : unwrapSkillEnvelope(source);
  const data = source.response ? skillEnvelopeData(source.response) : skillEnvelopeData(source);
  const toolName = text(source.tool || response.skill_id || source.skill_id || "analytix_funds");
  const status = text(response.status || source.status || "ok");
  const safetyMetadata = resultSafetyMetadata(source, response, data);
  const warnings = compactWarningList(envelopeWarnings(response), source.warnings, data.warnings);
  const keyFacts = {};
  const caseRecord = objectOf(data.case || source.case);
  const caseId = text(data.case_id || source.case_id || caseRecord.case_id || caseRecord.caseId);
  if (caseId || Object.keys(caseRecord).length) {
    keyFacts.case_identity = pruneEmpty({
      case_id: caseId,
      source: data.source || source.source,
      case_name: caseRecord.case_name || caseRecord.name,
      case_number: caseRecord.case_number || caseRecord.caseNo,
      status: caseRecord.status,
      boundary: "当前案件身份只用于定位分析上下文；报告级金额、账户归属和资金流向研判结论仍需确定性核验材料支持。"
    }) || {};
  }

  if (Array.isArray(data.rankings)) {
    const rankingFilters = objectOf(data.filters);
    const rankingScope = objectOf(data.scope);
    const groupSummary = objectOf(data.group_summary);
    const summary = objectOf(data.summary);
    const rankingRows = data.rankings;
    const rankingLimit = requestedRankingLimit(data, rankingRows.length || 10);
    const returnedCount = firstNonNegativeInt(data.returned_count, summary.returned_count);
    const visibleCount = firstNonNegativeInt(data.visible_count, summary.visible_count);
    const filteredCount = firstNonNegativeInt(data.filtered_count, summary.filtered_count);
    const exhaustedState = booleanOrUndefined(data.data_exhausted)
      ?? booleanOrUndefined(summary.data_exhausted);
    keyFacts.group_summary = groupSummary;
    keyFacts.ranking_context = pruneEmpty({
      source: "规范明细索引",
      scope_type: rankingScope.scope_type || "case",
      holder_name: rankingScope.holder_name,
      metric: text(rankingFilters.metric || rankingScope.metric || data.metric || "turnover"),
      direction_mode: rankingFilters.direction_mode,
      success_filter: rankingFilters.success_filter,
      cash_filter: rankingFilters.cash_filter,
      limit: firstNonNegativeInt(rankingFilters.limit, summary.limit) ?? rankingLimit,
      returned_count: returnedCount,
      preview_limit: rankingLimit,
      txn_count: nonNegativeIntOrUndefined(groupSummary.txn_count),
      account_count: nonNegativeIntOrUndefined(groupSummary.account_count),
      counterparty_count: nonNegativeIntOrUndefined(groupSummary.counterparty_count),
      first_txn_at: groupSummary.first_txn_at,
      last_txn_at: groupSummary.last_txn_at,
      boundary: "本卡只控制普通排行/Top 问题；若用户问 A 转给 B 多少钱、金额争议、去重/换卡/同事实等金额核验题，本卡仅是排行线索/线索候选，不能单独作为最终金额，必须补有来源支撑的定向聚合或专项资金核算，复核原明细、有效交易、去重依据、时间窗和核验意见。"
    }) || {};
    keyFacts.rankings = [{
      target: toolName,
      metric: text(data.filters?.metric || data.scope?.metric || data.metric || "turnover"),
      requested_limit: rankingLimit,
      resolved_limit: firstNonNegativeInt(data.resolved_limit, data.row_limit),
      returned_count: returnedCount,
      visible_count: visibleCount,
      filtered_count: filteredCount,
      data_exhausted: returnedCount === undefined ? undefined : exhaustedState,
      truncation_reason: text(data.truncation_reason || summary.truncation_reason),
      compact_text_row_count: firstNonNegativeInt(data.compact_text_row_count, summary.compact_text_row_count),
      final_answer_expected_min_rows: firstNonNegativeInt(
        data.final_answer_expected_min_rows,
        summary.final_answer_expected_min_rows
      ),
      rows: compactRankingRows(rankingRows, rankingLimit)
    }];
  }
  if (Array.isArray(data.top_outflows)) {
    const topOutflowRows = data.top_outflows;
    const topOutflowLimit = requestedTopOutflowLimit(data, topOutflowRows.length || 8);
    const returnedCount = nonNegativeIntOrUndefined(data.returned_count);
    const exhaustedState = booleanOrUndefined(data.data_exhausted);
    keyFacts.scope_stats = data.scope_stats || {};
    keyFacts.top_n_contract = pruneEmpty({
      requested_limit: firstNonNegativeInt(data.requested_limit, data.top_n),
      resolved_limit: firstNonNegativeInt(data.resolved_limit, data.row_limit),
      returned_count: returnedCount,
      visible_count: nonNegativeIntOrUndefined(data.visible_count),
      filtered_count: nonNegativeIntOrUndefined(data.filtered_count),
      data_exhausted: returnedCount === undefined ? undefined : exhaustedState,
      truncation_reason: text(data.truncation_reason),
      compact_text_row_count: nonNegativeIntOrUndefined(data.compact_text_row_count),
      final_answer_expected_min_rows: nonNegativeIntOrUndefined(data.final_answer_expected_min_rows)
    }) || {};
    keyFacts.top_outflows = compactTopOutflowRows(topOutflowRows, topOutflowLimit);
    keyFacts.terminal_summary = arrayOf(data.terminal_summary).slice(0, 6);
    keyFacts.followup_requests = arrayOf(data.followup_requests).slice(0, 6);
  }
  if (data.hypothesis_cards || data.mandatory_review_cards) {
    keyFacts.mandatory_review_cards = compactHypothesisCards(data.mandatory_review_cards || [], 8);
    keyFacts.hypothesis_cards = compactHypothesisCards(data.hypothesis_cards || [], 8);
    keyFacts.evidence_gaps = arrayOf(data.evidence_gaps).slice(0, 8);
  }
  if (data.findings || data.category_summary || data.pattern_summary) {
    keyFacts.hypothesis_cards = [
      ...arrayOf(keyFacts.hypothesis_cards),
      ...compactProbeFindingCards(data.findings || data.category_summary || [], 8)
    ];
    keyFacts.pattern_summary = objectOf(data.pattern_summary);
    if (text(data.interpretation)) {
      keyFacts.interpretation = humanizeAgentText(data.interpretation);
    }
  }
  if (data.followup_candidates) {
    keyFacts.followup_requests = compactFollowupCandidates(data.followup_candidates, 8);
  }
  if (data.import_lineage || data.cleaning_quality || data.account_identity_quality) {
    keyFacts.qa_review = compactQaReviewFacts(data, {});
  }
  if (data.summary && data.dedupe_policy) {
    keyFacts.qa_review = {
      ...objectOf(keyFacts.qa_review),
      ...compactQaReviewFacts({}, data)
    };
  }
  const workbenchFacts = compactWorkbenchFacts(toolName, source, response, data);
  if (Object.keys(workbenchFacts).length) {
    keyFacts.workbench = workbenchFacts;
  }
  const schemaInventory = compactSchemaInventory(toolName, data);
  if (Object.keys(schemaInventory).length) {
    keyFacts.schema_inventory = schemaInventory;
  }
  if (data.holder_scope || data.account_analysis) {
    keyFacts.holder_scope = compactOwnerScope(data.holder_scope || {});
    keyFacts.coverage = compactCoverageFact(data.account_analysis?.coverage || {});
    keyFacts.account_stats = compactAccountStatsFact(data.account_analysis?.account_stats || {});
    keyFacts.holder_top_accounts = compactAccountStatsTopAccounts(data.account_analysis?.account_stats || {}, 8);
    keyFacts.counterparty_rankings = compactRankingRows(data.account_analysis?.counterparty_rankings?.rankings || [], 6);
  }
  if (data.owner_scope_id || data.scope_id || data.account_keys || data.direct_account_keys) {
    keyFacts.scope = compactOwnerScope(data);
  }
  if (toolName === "run_full_case_analysis" || data.report || data.detector_coverage || data.investigation_lab) {
    keyFacts.full_case_package = compactFullCasePackage(data, safetyMetadata);
  }
  if (toolName === "get_evidence_pack" || data.pack_version || data.key_nodes || data.pattern_signals) {
    keyFacts.evidence_pack = compactEvidencePack(data);
  }
  if (source.map || source.mandatory_gates || source.allowed_next_tools) {
    Object.assign(keyFacts, compactCaseGraphFromScopeMap(source));
  }
  if (source.answer_card || source.key_facts || source.casegraph || source.flow_graph) {
    const frontDoorFacts = compactKeyFactsForText(source.key_facts || {});
    if (source.casegraph) {
      frontDoorFacts.casegraph_summary = {
        graph_version: source.casegraph.graph_version,
        node_count: arrayLengthIfPresent(source.casegraph, ["nodes"]),
        gate_count: arrayLengthIfPresent(source.casegraph, ["mandatory_gates", "gates"]),
        boundary_count: arrayLengthIfPresent(source.casegraph, ["boundaries"])
      };
    }
    if (source.flow_graph) {
      frontDoorFacts.flow_graph = compactFlowGraphForText(source.flow_graph, 5);
    }
    const mergedFacts = pruneEmpty({
      ...frontDoorFacts,
      ...keyFacts
    }) || {};
    return attachStableSafetyMetadata(stripAgentOpaqueRefs(pruneEmpty({
      tool: toolName,
      status: source.status || status,
      answer_card: source.answer_card,
      key_facts: mergedFacts,
      warnings: compactWarningList(source.warnings),
      next_actions: compactNextActions(source.next_actions)
    }) || {}) || {}, safetyMetadata);
  }

  return attachStableSafetyMetadata(stripAgentOpaqueRefs(pruneEmpty({
    tool: toolName,
    status,
    answer_card: {
      title: `${toolName} completed`,
      summary: "已返回本次核验可支持的事实摘要；是否足够作答取决于当前问题类型、来源边界和核验状态。原始明细是否留存须以宿主证据登记为准。",
      case_id: caseId,
      boundary: "不能从被省略的明细推断总额；金额、归属和资金边只以确定性事实或交易级可证实资金边为准。"
    },
    key_facts: Object.keys(keyFacts).length ? keyFacts : {
      data_keys: Object.keys(data).slice(0, 20),
      summary: data.summary || data.coverage || data.scope || data.group_summary || {}
    },
    warnings,
    next_actions: compactNextActions(response.next_actions || source.next_actions)
  }) || {}) || {}, safetyMetadata);
}
