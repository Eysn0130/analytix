import { redactAgentPayload } from "./agent-context-hygiene.mjs";
import { queryString } from "./backend-api-client.mjs";
import { throwIfAborted } from "./abort-runtime.mjs";
import {
  arrayOf,
  clampInt,
  dataOf,
  intOrUndefined,
  numberOrUndefined,
  objectOf,
  pruneEmpty,
  text
} from "./runtime-normalizers.mjs";

export const CASE_PIPELINE_RUNTIME_VERSION = "0.14.3";

export const SKILL_SURFACE_STATE_V1_SCHEMA = Object.freeze({
  oneOf: [
    {
      type: "object",
      properties: {
        version: { type: "string", const: "SkillSurfaceStateV1" },
        state: { type: "string", const: "available" },
        count: { type: "integer", minimum: 0, maximum: 80 },
        items: {
          type: "array",
          maxItems: 80,
          items: {
            type: "object",
            properties: {
              id: { type: "string", minLength: 1 },
              name: { type: "string", minLength: 1 },
              title: { type: "string", minLength: 1 },
              visibility: { type: "string", minLength: 1 },
              enabled: { type: "boolean" }
            },
            required: ["id", "name", "title", "visibility", "enabled"],
            additionalProperties: false
          }
        }
      },
      required: ["version", "state", "count", "items"],
      additionalProperties: false
    },
    {
      type: "object",
      properties: {
        version: { type: "string", const: "SkillSurfaceStateV1" },
        state: { type: "string", const: "unavailable" },
        blocker_code: { type: "string", const: "skills_source_unavailable" }
      },
      required: ["version", "state", "blocker_code"],
      additionalProperties: false
    },
    {
      type: "object",
      properties: {
        version: { type: "string", const: "SkillSurfaceStateV1" },
        state: { type: "string", const: "invalid" },
        blocker_code: { type: "string", const: "skills_response_schema_invalid" }
      },
      required: ["version", "state", "blocker_code"],
      additionalProperties: false
    }
  ]
});

const SKILL_RESPONSE_ROOT_KEYS = new Set(["data"]);
const SKILL_RESPONSE_DATA_KEYS = new Set(["items"]);
const SKILL_ITEM_KEYS = new Set(["id", "name", "title", "visibility", "enabled"]);

const IMPORT_COUNT_FIELDS = [
  "size",
  "rows_total",
  "rows_imported",
  "rows_imported_raw",
  "rows_imported_norm",
  "rows_dedup",
  "rows_error",
  "rows_skipped_non_data"
];
const IMPORT_REQUIRED_COUNT_FIELDS = [
  "rows_total",
  "rows_imported_norm",
  "rows_dedup",
  "rows_error"
];

function hasOwn(source, key) {
  return Object.prototype.hasOwnProperty.call(objectOf(source), key);
}

function optionalCount(value) {
  const number = intOrUndefined(value);
  return number !== undefined && number >= 0 ? number : undefined;
}

function firstOwnedCount(source, keys) {
  const item = objectOf(source);
  for (const key of keys) {
    if (!hasOwn(item, key)) continue;
    return optionalCount(item[key]);
  }
  return undefined;
}

function firstKnownCount(...values) {
  for (const value of values) {
    if (value === undefined) continue;
    const count = optionalCount(value);
    if (count !== undefined) return count;
    return undefined;
  }
  return undefined;
}

function completeCountSum(rows, key) {
  const records = arrayOf(rows).map(objectOf);
  if (!records.length) return undefined;
  const counts = records.map((row) => optionalCount(row[key]));
  if (counts.some((value) => value === undefined)) return undefined;
  return counts.reduce((sum, value) => sum + value, 0);
}

function externalText(value) {
  if (typeof value !== "string") return undefined;
  return value.trim() || undefined;
}

function reportedStatus(value) {
  const status = externalText(value);
  if (!status) return undefined;
  return /^(?:ok|ready|passed|success|succeeded|complete|completed)$/iu.test(status)
    ? "reported_success_unverified"
    : status;
}

function sanitizePipelineExternal(value, key = "", depth = 0) {
  if (depth > 6) return undefined;
  if (Array.isArray(value)) {
    return value
      .slice(0, 100)
      .map((item) => sanitizePipelineExternal(item, "", depth + 1))
      .filter((item) => item !== undefined);
  }
  if (value && typeof value === "object") {
    const output = {};
    for (const [childKey, item] of Object.entries(value)) {
      const next = sanitizePipelineExternal(item, childKey, depth + 1);
      if (next !== undefined) output[childKey] = next;
    }
    return Object.keys(output).length ? output : undefined;
  }
  const fieldKey = String(key || "").replace(/([a-z\d])([A-Z])/gu, "$1_$2").toLowerCase();
  if ((/(?:^|_)(?:count|rows?|size|items|groups|files|tx|txn)(?:_|$)/u.test(fieldKey)
    || /^(?:step|total|page|page_size|limit|offset|invalid|failed|reversal|account_invalid|account_info_filled|affected)$/u.test(fieldKey))
    && !/^(?:items|groups|files)$/u.test(fieldKey)) {
    return optionalCount(value);
  }
  if (/(?:^|_)(?:amount|turnover|inflow|outflow|balance)(?:_|$)/u.test(fieldKey)) {
    return numberOrUndefined(value);
  }
  if (/(?:^|_)(?:ok|ready|complete|completed|passed|success|succeeded|safe_to_answer)$/u.test(fieldKey)) {
    return value === false ? false : undefined;
  }
  if (/(?:^|_)status$/u.test(fieldKey)) return reportedStatus(value);
  if (typeof value === "number") return Number.isFinite(value) ? value : undefined;
  if (typeof value === "boolean") return value;
  if (typeof value === "string") return value.trim() ? value : undefined;
  return undefined;
}

function sanitizedExternalObject(value) {
  return redactAgentPayload(objectOf(sanitizePipelineExternal(objectOf(value))));
}

function sanitizedExternalRecords(value) {
  return redactAgentPayload(
    arrayOf(sanitizePipelineExternal(arrayOf(value))).filter(
      (item) => item && typeof item === "object" && !Array.isArray(item)
    )
  );
}

function externalRecordsShapeValid(value) {
  return Array.isArray(value)
    && value.every((item) => item && typeof item === "object" && !Array.isArray(item));
}

function importRowCountsStatus(row) {
  const source = objectOf(row);
  return IMPORT_REQUIRED_COUNT_FIELDS.every((key) => optionalCount(source[key]) !== undefined)
    ? "all_reported_counts_present"
    : "partial_or_invalid";
}

export function compactImportRow(row) {
  const source = objectOf(row);
  return pruneEmpty({
    title: source.title || source.file_name || source.filename,
    kind: source.kind || source.file_kind || source.type,
    status: reportedStatus(source.status),
    time: source.time || source.created_at || source.imported_at,
    size: optionalCount(source.size),
    rows_total: optionalCount(source.rows_total),
    rows_imported: optionalCount(source.rows_imported),
    rows_imported_raw: optionalCount(source.rows_imported_raw),
    rows_imported_norm: optionalCount(source.rows_imported_norm),
    rows_dedup: optionalCount(source.rows_dedup),
    rows_error: optionalCount(source.rows_error),
    rows_skipped_non_data: optionalCount(source.rows_skipped_non_data),
    error: source.error || source.message,
    reported_counts_status: importRowCountsStatus(source),
    coverage_complete: false,
    safe_for_case_conclusion: false,
    boundary: "Reported import counters are not dataset-snapshot coverage or proof of no anomaly."
  }) || {};
}

export function compactCaseDashboardStats(stats) {
  const source = objectOf(stats);
  if (!Object.keys(source).length) {
    return undefined;
  }
  return pruneEmpty({
    semantic: "Analytix case dashboard metadata only; not analysis coverage.",
    analysis_coverage_tool: "get_case_scope_map or get_scope_coverage",
    task_counter: firstOwnedCount(source, ["tasks", "task_count"]),
    dashboard_account_counter: firstOwnedCount(source, ["accounts", "account_count"]),
    dashboard_person_counter: firstOwnedCount(source, ["persons", "person_count"]),
    dashboard_transaction_counter: firstOwnedCount(source, ["tx", "txn_count", "transaction_count"]),
    dashboard_sub_counter: firstOwnedCount(source, ["sub", "sub_count"]),
    reported_counts_status: [
      firstOwnedCount(source, ["tasks", "task_count"]),
      firstOwnedCount(source, ["accounts", "account_count"]),
      firstOwnedCount(source, ["persons", "person_count"]),
      firstOwnedCount(source, ["tx", "txn_count", "transaction_count"]),
      firstOwnedCount(source, ["sub", "sub_count"])
    ].every((value) => value !== undefined)
      ? "all_reported_counts_present"
      : "partial_or_invalid",
    coverage_complete: false,
    safe_for_case_conclusion: false
  });
}

export function compactCaseRecord(record, options = {}) {
  const source = objectOf(record);
  const imports = arrayOf(source.imports);
  const includeImports = options.includeImports === true;
  const importSummary = imports.length ? summarizeImportFiles(imports) : undefined;
  return pruneEmpty({
    case_id: source.case_id || source.caseId,
    case_name: source.case_name || source.name,
    case_number: source.case_number || source.caseNo,
    source: source.source,
    owner: source.owner,
    note: source.note,
    case_type: source.case_type,
    tags: arrayOf(source.tags),
    status: reportedStatus(source.status),
    duckdb_ready: source.duckdb_ready === false ? false : undefined,
    backend_status: reportedStatus(source.backend_status),
    import_health: reportedStatus(source.import_health),
    size_label: source.size_label,
    size_bytes: optionalCount(source.size_bytes),
    created_at: source.created_at,
    updated_at: source.updated_at,
    coverage_note: "Do not use case_dashboard_stats as account/holder/transaction coverage. For ranking or report-grade totals, use get_case_scope_map.coverage or get_scope_coverage.",
    case_dashboard_stats: compactCaseDashboardStats(source.stats),
    import_summary: importSummary,
    imports: includeImports ? imports.map(compactImportRow) : undefined
  }) || {};
}

function hasOnlyKeys(value, allowedKeys) {
  return value
    && typeof value === "object"
    && !Array.isArray(value)
    && Object.keys(value).every((key) => allowedKeys.has(key));
}

function invalidSkillsSurfaceState() {
  return {
    version: "SkillSurfaceStateV1",
    state: "invalid",
    blocker_code: "skills_response_schema_invalid"
  };
}

export function unavailableSkillsSurfaceState() {
  return {
    version: "SkillSurfaceStateV1",
    state: "unavailable",
    blocker_code: "skills_source_unavailable"
  };
}

export function compactSkillsRecord(payload) {
  if (!hasOnlyKeys(payload, SKILL_RESPONSE_ROOT_KEYS)
    || Object.keys(payload).length !== 1
    || !hasOnlyKeys(payload.data, SKILL_RESPONSE_DATA_KEYS)
    || Object.keys(payload.data).length !== 1
    || !Array.isArray(payload.data.items)
    || payload.data.items.length > 80) {
    return invalidSkillsSurfaceState();
  }

  const items = [];
  const seenIds = new Set();
  for (const rawItem of payload.data.items) {
    if (!hasOnlyKeys(rawItem, SKILL_ITEM_KEYS)
      || Object.keys(rawItem).length !== SKILL_ITEM_KEYS.size
      || ![rawItem.id, rawItem.name, rawItem.title, rawItem.visibility]
        .every((value) => typeof value === "string" && value.trim())
      || typeof rawItem.enabled !== "boolean") {
      return invalidSkillsSurfaceState();
    }
    const id = rawItem.id.trim();
    if (seenIds.has(id)) return invalidSkillsSurfaceState();
    seenIds.add(id);
    items.push({
      id,
      name: rawItem.name.trim(),
      title: rawItem.title.trim(),
      visibility: rawItem.visibility.trim(),
      enabled: rawItem.enabled
    });
  }

  return {
    version: "SkillSurfaceStateV1",
    state: "available",
    count: items.length,
    items
  };
}

export function summarizeImportFiles(items) {
  const sourceListValid = Array.isArray(items);
  const sourceItems = arrayOf(items);
  const records = sourceItems.filter((item) => item && typeof item === "object" && !Array.isArray(item));
  const recordsShapeValid = sourceListValid && records.length === sourceItems.length;
  const summary = {
    file_count: recordsShapeValid ? records.length : undefined,
    source_item_count: sourceListValid ? sourceItems.length : undefined,
    invalid_item_count: sourceListValid ? sourceItems.length - records.length : undefined,
    reported_counts_status:
      recordsShapeValid
        && records.length > 0
        && records.every((row) => importRowCountsStatus(row) === "all_reported_counts_present")
        ? "all_reported_counts_present"
        : records.length > 0
          ? "partial_or_invalid"
          : "unavailable",
    coverage_complete: false,
    safe_for_case_conclusion: false,
    zero_result_verified: false,
    boundary: "Import counters describe returned records only; they do not prove complete case or snapshot coverage.",
    by_kind: {},
    by_file_type: {},
    by_status: {}
  };

  if (recordsShapeValid) {
    for (const field of IMPORT_COUNT_FIELDS) {
      const total = completeCountSum(records, field);
      if (total !== undefined) {
        summary[field === "size" ? "total_size" : field] = total;
      }
    }

    for (const [bucket, rawKey] of [
      ["by_kind", "kind"],
      ["by_file_type", "file_type"],
      ["by_status", "status"]
    ]) {
      const grouped = new Map();
      for (const item of records) {
        const rawValue = rawKey === "status" ? reportedStatus(item[rawKey]) : text(item[rawKey]);
        const key = rawValue || "unknown";
        if (!grouped.has(key)) grouped.set(key, []);
        grouped.get(key).push(item);
      }
      for (const [key, bucketRows] of grouped) {
        const target = { count: bucketRows.length };
        for (const field of ["rows_total", "rows_imported_norm", "rows_error"]) {
          const total = completeCountSum(bucketRows, field);
          if (total !== undefined) target[field] = total;
        }
        summary[bucket][key] = target;
      }
    }
  }
  return pruneEmpty(summary) || {};
}

export function summarizeCleaningSteps(items) {
  const sourceListValid = Array.isArray(items);
  const sourceItems = arrayOf(items);
  const records = sourceItems.filter((item) => item && typeof item === "object" && !Array.isArray(item));
  const recordsShapeValid = sourceListValid && records.length === sourceItems.length;
  const steps = records.map((item) => pruneEmpty({
    step: optionalCount(item.step),
    key: text(item.key),
    title: text(item.title),
    kind: text(item.kind),
    description: text(item.description),
    status: reportedStatus(item.status),
    ready: item.ready === false ? false : undefined
  }) || {});
  return pruneEmpty({
    step_count: recordsShapeValid ? steps.length : undefined,
    source_item_count: sourceListValid ? sourceItems.length : undefined,
    invalid_item_count: sourceListValid ? sourceItems.length - records.length : undefined,
    reported_counts_status: steps.length ? "withheld_without_host_receipt" : "unavailable",
    coverage_complete: false,
    safe_for_case_conclusion: false,
    zero_result_verified: false,
    fact_answer_allowed: false,
    publication_readiness: "blocked",
    boundary: "Cleaning counters are withheld until a same-case host EvidenceReceipt authorizes each claim.",
    steps
  }) || {};
}

export function firstPositive(...values) {
  for (const value of values) {
    const next = optionalCount(value);
    if (next !== undefined && next > 0) {
      return next;
    }
  }
  return undefined;
}

export function derivePipelineScopeSummary(importOverview, cleaningOverview, statsMeta, statsTreeByName, statsTreeByCard) {
  void statsTreeByName;
  void statsTreeByCard;
  const importWarnings = arrayOf(importOverview?.warnings).filter(Boolean);
  const cleaningWarnings = arrayOf(cleaningOverview?.warnings).filter(Boolean);
  const statsMetaData = objectOf(statsMeta?.meta);
  const importSnapshotId = externalText(importOverview?.reported_dataset_snapshot_id);
  const cleaningSnapshotId = externalText(cleaningOverview?.reported_dataset_snapshot_id);
  const statsSnapshotId = externalText(
    statsMeta?.reported_dataset_snapshot_id || statsMetaData.dataset_snapshot_id
  );
  const snapshotIdsReportedConsistent = Boolean(
    importSnapshotId
      && cleaningSnapshotId
      && statsSnapshotId
      && importSnapshotId === cleaningSnapshotId
      && importSnapshotId === statsSnapshotId
  );
  return pruneEmpty({
    case_id: text(cleaningOverview?.case_id || importOverview?.case_id || statsMeta?.case_id),
    import_files_availability: "host_receipt_required",
    import_warnings: importWarnings,
    cleaning_warnings: cleaningWarnings,
    funds_status: reportedStatus(statsMetaData.funds_status),
    snapshot_binding_status: snapshotIdsReportedConsistent
      ? "reported_match_unverified"
      : "unavailable_or_mismatched",
    coverage_complete: false,
    safe_for_case_conclusion: false,
    zero_result_verified: false,
    fact_answer_allowed: false,
    publication_readiness: "blocked",
    boundary: "Pipeline counters are operational diagnostics only until same-case dataset-snapshot evidence is verified."
  }) || {};
}

function compactCleaningJobBoundary(value) {
  const source = objectOf(value);
  return pruneEmpty({
    job_id: externalText(source.job_id),
    case_id: externalText(source.case_id),
    status: reportedStatus(source.status),
    progress: optionalCount(source.progress),
    result_semantic_status: "blocked",
    result_blocker: "host_evidence_receipt_required",
    fact_answer_allowed: false,
    created_at: externalText(source.created_at),
    updated_at: externalText(source.updated_at)
  }) || {};
}

export function compactTreeGroups(groups, args = {}) {
  const limitGroups = clampInt(args.limit_groups || args.tree_limit_groups, 100, 1, 1000);
  const limitItems = clampInt(args.limit_items || args.tree_limit_items, 20, 0, 1000);
  const includeItems = args.include_items !== false && limitItems > 0;
  const rawGroups = arrayOf(groups);
  const groupRecords = rawGroups.filter((group) => group && typeof group === "object" && !Array.isArray(group));
  const normalized = groupRecords.map((group) => {
    const rawItems = arrayOf(group.items);
    const itemRecords = rawItems.filter((item) => item && typeof item === "object" && !Array.isArray(item));
    const itemsListComplete = Array.isArray(group.items) && itemRecords.length === rawItems.length;
    const itemKeys = itemRecords.map((item) => text(item.id)).filter(Boolean);
    return pruneEmpty({
      id: text(group.id),
      title: text(group.title),
      meta: text(group.meta),
      extra: text(group.extra),
      item_count: itemsListComplete ? itemRecords.length : undefined,
      source_item_count: Array.isArray(group.items) ? rawItems.length : undefined,
      invalid_item_count: Array.isArray(group.items) ? rawItems.length - itemRecords.length : undefined,
      items_shape_valid: itemsListComplete,
      item_keys: itemKeys,
      items: includeItems
        ? itemRecords.slice(0, limitItems).map((item) => pruneEmpty({
            id: text(item.id),
            title: text(item.title),
            sub: text(item.sub)
          }) || {})
        : []
    }) || {};
  });
  const sourceListComplete = Array.isArray(groups)
    && groupRecords.length === rawGroups.length
    && normalized.every((group) => group.items_shape_valid === true);
  return pruneEmpty({
    total_groups: sourceListComplete ? normalized.length : undefined,
    total_items: sourceListComplete
      ? normalized.reduce((sum, group) => sum + group.item_count, 0)
      : undefined,
    source_group_count: Array.isArray(groups) ? rawGroups.length : undefined,
    invalid_group_count: Array.isArray(groups) ? rawGroups.length - groupRecords.length : undefined,
    returned_groups: sourceListComplete ? Math.min(normalized.length, limitGroups) : undefined,
    item_return_limit_per_group: includeItems ? limitItems : 0,
    response_shape_valid: sourceListComplete,
    coverage_complete: false,
    safe_for_case_conclusion: false,
    zero_result_verified: false,
    publication_readiness: "blocked",
    boundary: "Tree groups describe the returned response only; an empty tree does not prove no accounts or relationships.",
    groups: normalized.slice(0, limitGroups)
  }) || {};
}

export function createCasePipelineRuntime({ resolveCase, httpJson, httpPostData }) {
  if (typeof resolveCase !== "function" || typeof httpJson !== "function" || typeof httpPostData !== "function") {
    throw new Error("createCasePipelineRuntime requires resolveCase/httpJson/httpPostData");
  }

  async function getImportOverview(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const pageSize = clampInt(args.page_size, 50, 1, 500);
    const view = text(args.view) === "recycle" ? "recycle" : "active";
    const [filesPayload, jobsPayload] = await Promise.all([
      httpJson(`/import/files?${queryString({ case_id: resolved.case_id, view })}`, { signal }).catch((error) => {
        throwIfAborted(signal);
        return { error: error.message, data: {} };
      }),
      args.include_jobs === false
        ? Promise.resolve({ data: { items: [], page: {} } })
        : httpJson(`/import/jobs?${queryString({ case_id: resolved.case_id, page: 1, page_size: pageSize })}`, { signal }).catch(
            (error) => {
              throwIfAborted(signal);
              return { error: error.message, data: {} };
            }
          )
    ]);
    const filesData = dataOf(filesPayload);
    const jobsData = dataOf(jobsPayload);
    const items = arrayOf(filesData.items);
    const filesShapeValid = externalRecordsShapeValid(filesData.items);
    const jobsChecked = args.include_jobs !== false;
    const jobsShapeValid = !jobsChecked || externalRecordsShapeValid(jobsData.items);
    const summary = summarizeImportFiles(filesData.items);
    const warnings = [
      filesPayload.error,
      jobsPayload.error,
      !filesPayload.error && !filesShapeValid ? "import_files_invalid_response_shape" : undefined,
      jobsChecked && !jobsPayload.error && !jobsShapeValid ? "import_jobs_invalid_response_shape" : undefined
    ].filter(Boolean);
    return {
      case_id: resolved.case_id,
      source: resolved.source,
      view,
      reported_dataset_snapshot_id: externalText(
        filesData.dataset_snapshot_id || filesPayload.dataset_snapshot_id
      ),
      summary,
      files: items.map(compactImportRow),
      latest_jobs: sanitizedExternalRecords(jobsData.items),
      jobs_page: sanitizedExternalObject(jobsData.page),
      response_list_shape_valid: Boolean(
        !filesPayload.error && filesShapeValid && summary.invalid_item_count === 0
      ),
      jobs_checked: jobsChecked,
      jobs_list_shape_valid: Boolean(jobsChecked && !jobsPayload.error && jobsShapeValid),
      coverage_complete: false,
      safe_for_case_conclusion: false,
      zero_result_verified: false,
      publication_readiness: "blocked",
      source_status: warnings.length ? "unavailable_or_invalid" : "reported_records_only",
      boundary: "Import inventory is operational metadata until same-case dataset-snapshot evidence is verified.",
      warnings: redactAgentPayload(warnings)
    };
  }

  async function getCleaningOverview(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const pageSize = clampInt(args.page_size, 50, 1, 500);
    const logLimit = clampInt(args.log_limit, 200, 1, 1000);
    const [jobsPayload, historyPayload, stepsPayload, logsPayload] = await Promise.all([
      httpJson(`/cleaning/jobs?${queryString({ case_id: resolved.case_id, page: 1, page_size: pageSize })}`, { signal }).catch(
        (error) => {
          throwIfAborted(signal);
          return { error: error.message, data: {} };
        }
      ),
      httpJson(`/cleaning/history?${queryString({ case_id: resolved.case_id, limit: pageSize })}`, { signal }).catch((error) => {
        throwIfAborted(signal);
        return { error: error.message, data: {} };
      }),
      httpJson(`/cleaning/steps?${queryString({ case_id: resolved.case_id })}`, { signal }).catch((error) => {
        throwIfAborted(signal);
        return { error: error.message, data: {} };
      }),
      args.include_logs
        ? httpJson(`/cleaning/logs?${queryString({ case_id: resolved.case_id, limit: logLimit })}`, { signal }).catch((error) => {
            throwIfAborted(signal);
            return { error: error.message, data: {} };
          })
        : Promise.resolve({ data: { items: [] } })
    ]);
    const jobsData = dataOf(jobsPayload);
    const historyData = dataOf(historyPayload);
    const stepsData = dataOf(stepsPayload);
    const latestJob = arrayOf(jobsData.items)[0] || null;
    const latestHistory = arrayOf(historyData.items)[0] || null;
    const stepSummary = summarizeCleaningSteps(stepsData.items);
    const logsData = dataOf(logsPayload);
    const jobsShapeValid = externalRecordsShapeValid(jobsData.items);
    const historyShapeValid = externalRecordsShapeValid(historyData.items);
    const stepsShapeValid = externalRecordsShapeValid(stepsData.items);
    const logsChecked = args.include_logs === true;
    const logsShapeValid = !logsChecked || externalRecordsShapeValid(logsData.items);
    const warnings = [
      jobsPayload.error,
      historyPayload.error,
      stepsPayload.error,
      logsPayload.error,
      !jobsPayload.error && !jobsShapeValid ? "cleaning_jobs_invalid_response_shape" : undefined,
      !historyPayload.error && !historyShapeValid ? "cleaning_history_invalid_response_shape" : undefined,
      !stepsPayload.error && !stepsShapeValid ? "cleaning_steps_invalid_response_shape" : undefined,
      logsChecked && !logsPayload.error && !logsShapeValid ? "cleaning_logs_invalid_response_shape" : undefined
    ].filter(Boolean);
    return {
      case_id: resolved.case_id,
      source: resolved.source,
      reported_dataset_snapshot_id: externalText(
        jobsData.dataset_snapshot_id
          || historyData.dataset_snapshot_id
          || stepsData.dataset_snapshot_id
          || jobsPayload.dataset_snapshot_id
      ) || undefined,
      latest_job: compactCleaningJobBoundary(latestJob),
      latest_history: undefined,
      jobs: arrayOf(jobsData.items).map(compactCleaningJobBoundary),
      jobs_page: undefined,
      step_summary: redactAgentPayload(stepSummary),
      logs: [],
      jobs_list_shape_valid: Boolean(!jobsPayload.error && jobsShapeValid),
      history_list_shape_valid: Boolean(!historyPayload.error && historyShapeValid),
      steps_list_shape_valid: Boolean(!stepsPayload.error && stepsShapeValid),
      logs_checked: logsChecked,
      logs_list_shape_valid: Boolean(logsChecked && !logsPayload.error && logsShapeValid),
      coverage_complete: false,
      safe_for_case_conclusion: false,
      zero_result_verified: false,
      fact_answer_allowed: false,
      publication_readiness: "blocked",
      source_status: warnings.length ? "unavailable_or_invalid" : "reported_records_only",
      boundary: "Cleaning diagnostics do not establish complete case coverage or absence of invalid records.",
      warnings: redactAgentPayload(warnings)
    };
  }

  async function getStatsMeta(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const data = await httpPostData("/analysis/stats/v2/meta", { case_id: resolved.case_id }, { signal });
    return {
      case_id: resolved.case_id,
      source: resolved.source,
      reported_dataset_snapshot_id: externalText(data?.dataset_snapshot_id),
      meta: {
        funds_status: reportedStatus(data?.funds_status),
        fact_answer_allowed: false,
        result_blocker: "host_evidence_receipt_required"
      },
      coverage_complete: false,
      safe_for_case_conclusion: false,
      zero_result_verified: false,
      publication_readiness: "blocked",
      boundary: "Statistics metadata is an unverified source report until bound to a current dataset snapshot."
    };
  }

  async function getStatsTree(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const tab = text(args.tab) === "byCard" ? "byCard" : "byName";
    await httpPostData("/analysis/stats/v2/tree", {
      case_id: resolved.case_id,
      tab
    }, { signal });
    return {
      case_id: resolved.case_id,
      source: resolved.source,
      tab,
      groups: [],
      coverage_complete: false,
      safe_for_case_conclusion: false,
      zero_result_verified: false,
      fact_answer_allowed: false,
      publication_readiness: "blocked",
      blocker: "host_evidence_receipt_required"
    };
  }

  async function getCaseDataPipelineOverview(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const [importOverview, cleaningOverview, statsMeta, statsTreeByName, statsTreeByCard] = await Promise.all([
      getImportOverview({ ...args, case_id: resolved.case_id, include_jobs: true }, { signal }),
      getCleaningOverview({ ...args, case_id: resolved.case_id, include_logs: false }, { signal }),
      getStatsMeta({ ...args, case_id: resolved.case_id }, { signal }).catch((error) => {
        throwIfAborted(signal);
        return { error: error.message };
      }),
      args.include_tree === false
        ? Promise.resolve(null)
        : getStatsTree({
            ...args,
            case_id: resolved.case_id,
            tab: "byName",
            limit_groups: clampInt(args.tree_limit_groups, 100, 1, 500),
            limit_items: clampInt(args.tree_limit_items, 20, 0, 500),
            include_items: true
          }, { signal }).catch((error) => {
            throwIfAborted(signal);
            return { error: error.message };
          }),
      args.include_tree === false
        ? Promise.resolve(null)
        : getStatsTree({
            ...args,
            case_id: resolved.case_id,
            tab: "byCard",
            limit_groups: clampInt(args.tree_limit_groups, 100, 1, 500),
            limit_items: clampInt(args.tree_limit_items, 20, 0, 500),
            include_items: true
          }, { signal }).catch((error) => {
            throwIfAborted(signal);
            return { error: error.message };
          })
    ]);
    return {
      case_id: resolved.case_id,
      source: resolved.source,
      case: compactCaseRecord(resolved.case),
      coverage_complete: false,
      safe_for_case_conclusion: false,
      zero_result_verified: false,
      publication_readiness: "blocked",
      boundary: "Pipeline overview is boundary-only and cannot publish case facts without verified same-snapshot evidence.",
      data_scope_summary: derivePipelineScopeSummary(
        importOverview,
        cleaningOverview,
        statsMeta,
        statsTreeByName,
        statsTreeByCard
      ),
      import_overview: importOverview,
      cleaning_overview: cleaningOverview,
      stats_meta: statsMeta,
      stats_tree_by_name: statsTreeByName,
      stats_tree_by_card: statsTreeByCard
    };
  }

  return {
    compactCaseRecord,
    compactSkillsRecord,
    getImportOverview,
    getCleaningOverview,
    getStatsMeta,
    getStatsTree,
    getCaseDataPipelineOverview
  };
}
