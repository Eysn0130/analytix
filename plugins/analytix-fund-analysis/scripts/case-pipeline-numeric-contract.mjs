#!/usr/bin/env node

import assert from "node:assert/strict";
import {
  SKILL_SURFACE_STATE_V1_SCHEMA,
  compactCaseDashboardStats,
  compactCaseRecord,
  compactImportRow,
  compactSkillsRecord,
  compactTreeGroups,
  createCasePipelineRuntime,
  derivePipelineScopeSummary,
  firstPositive,
  summarizeCleaningSteps,
  summarizeImportFiles,
  unavailableSkillsSurfaceState
} from "../mcp/case-pipeline-runtime.mjs";

const invalidValues = [
  ["undefined", undefined],
  ["null", null],
  ["empty", ""],
  ["blank", " \t"],
  ["false", false],
  ["true", true],
  ["array", []],
  ["array-zero", [0]],
  ["object", {}],
  ["nan", Number.NaN],
  ["infinity", Number.POSITIVE_INFINITY]
];

const importCountFields = [
  "size",
  "rows_total",
  "rows_imported",
  "rows_imported_raw",
  "rows_imported_norm",
  "rows_dedup",
  "rows_error",
  "rows_skipped_non_data"
];

function hasOwn(value, key) {
  return Object.prototype.hasOwnProperty.call(value, key);
}

function assertBoundaryOnly(value, label) {
  assert.equal(value.coverage_complete, false, `${label} must not claim complete coverage`);
  assert.equal(value.safe_for_case_conclusion, false, `${label} must not permit case conclusions`);
  assert.equal(value.zero_result_verified, false, `${label} must not turn zero into verified no-hit`);
  assert.equal(value.publication_readiness, "blocked", `${label} must remain publication-blocked`);
}

const validEmptySkillSurface = compactSkillsRecord({ data: { items: [] } });
assert.deepEqual(validEmptySkillSurface, {
  version: "SkillSurfaceStateV1",
  state: "available",
  count: 0,
  items: []
});
const validSkillSurface = compactSkillsRecord({
  data: {
    items: [{
      id: "skill-a",
      name: "skill-a",
      title: "Skill A",
      visibility: "internal",
      enabled: false
    }]
  }
});
assert.deepEqual(validSkillSurface, {
  version: "SkillSurfaceStateV1",
  state: "available",
  count: 1,
  items: [{
    id: "skill-a",
    name: "skill-a",
    title: "Skill A",
    visibility: "internal",
    enabled: false
  }]
});

const invalidSkillSurfacePayloads = [
  undefined,
  null,
  {},
  { data: {} },
  { data: { items: [] }, extra: true },
  { data: { items: [], extra: true } },
  { data: { skills: [] } },
  { data: { items: false } },
  { data: { items: [{}] } },
  {
    data: {
      items: [{
        id: "skill-a",
        name: "skill-a",
        title: "Skill A",
        visibility: "internal",
        enabled: false,
        extra: true
      }]
    }
  },
  {
    data: {
      items: [{
        id: "skill-a",
        name: "skill-a",
        title: "Skill A",
        visibility: "internal",
        enabled: "false"
      }]
    }
  },
  {
    data: {
      items: Array.from({ length: 81 }, (_, index) => ({
        id: `skill-${index}`,
        name: `skill-${index}`,
        title: `Skill ${index}`,
        visibility: "internal",
        enabled: false
      }))
    }
  },
  {
    data: {
      items: [
        { id: "skill-a", name: "skill-a", title: "Skill A", visibility: "internal", enabled: false },
        { id: "skill-a", name: "skill-b", title: "Skill B", visibility: "internal", enabled: true }
      ]
    }
  }
];
for (const payload of invalidSkillSurfacePayloads) {
  assert.deepEqual(compactSkillsRecord(payload), {
    version: "SkillSurfaceStateV1",
    state: "invalid",
    blocker_code: "skills_response_schema_invalid"
  });
}
assert.deepEqual(unavailableSkillsSurfaceState(), {
  version: "SkillSurfaceStateV1",
  state: "unavailable",
  blocker_code: "skills_source_unavailable"
});
for (const state of [
  compactSkillsRecord(undefined),
  unavailableSkillsSurfaceState()
]) {
  assert.equal(hasOwn(state, "count"), false, `${state.state} skill state must not impersonate an empty inventory`);
  assert.equal(hasOwn(state, "items"), false, `${state.state} skill state must not impersonate an empty inventory`);
}
assert.equal(SKILL_SURFACE_STATE_V1_SCHEMA.oneOf.length, 3);
for (const variant of SKILL_SURFACE_STATE_V1_SCHEMA.oneOf) {
  assert.equal(variant.additionalProperties, false, "every SkillSurfaceStateV1 variant must be exact-root");
}
assert.equal(
  SKILL_SURFACE_STATE_V1_SCHEMA.oneOf[0].properties.items.items.additionalProperties,
  false,
  "SkillSurfaceStateV1 items must reject unknown fields"
);

for (const [label, value] of invalidValues) {
  const rawImport = Object.fromEntries(importCountFields.map((field) => [field, value]));
  rawImport.status = "ready";
  const compactImport = compactImportRow(rawImport);
  for (const field of importCountFields) {
    assert.equal(hasOwn(compactImport, field), false, `compact import/${label} must omit ${field}`);
  }
  assert.equal(compactImport.status, "reported_success_unverified");
  assert.equal(compactImport.reported_counts_status, "partial_or_invalid");
  assert.equal(compactImport.coverage_complete, false);
  assert.equal(compactImport.safe_for_case_conclusion, false);

  const dashboard = compactCaseDashboardStats({
    tasks: value,
    accounts: value,
    persons: value,
    tx: value,
    sub: value
  });
  for (const key of [
    "task_counter",
    "dashboard_account_counter",
    "dashboard_person_counter",
    "dashboard_transaction_counter",
    "dashboard_sub_counter"
  ]) {
    assert.equal(hasOwn(dashboard, key), false, `dashboard/${label} must omit ${key}`);
  }
  assert.equal(dashboard.reported_counts_status, "partial_or_invalid");

  const importSummary = summarizeImportFiles([rawImport]);
  assert.equal(importSummary.file_count, 1, `import/${label} may count the returned record itself`);
  for (const key of [
    "total_size",
    "rows_total",
    "rows_imported",
    "rows_imported_raw",
    "rows_imported_norm",
    "rows_dedup",
    "rows_error",
    "rows_skipped_non_data"
  ]) {
    assert.equal(hasOwn(importSummary, key), false, `import/${label} must omit invalid aggregate ${key}`);
  }
  assert.equal(importSummary.reported_counts_status, "partial_or_invalid");
  assert.equal(importSummary.coverage_complete, false);
  assert.equal(importSummary.safe_for_case_conclusion, false);
  assert.equal(importSummary.zero_result_verified, false);

  const cleaningSummary = summarizeCleaningSteps([{
    step: value,
    affected_rows: value,
    status: "passed",
    ready: true
  }]);
  assert.equal(cleaningSummary.step_count, 1, `cleaning/${label} may count the returned step record itself`);
  for (const key of [
    "hit_step_count",
    "affected_rows_total",
    "transaction_step_affected",
    "account_step_affected"
  ]) {
    assert.equal(hasOwn(cleaningSummary, key), false, `cleaning/${label} must omit ${key}`);
  }
  assert.equal(cleaningSummary.steps[0].status, "reported_success_unverified");
  assert.equal(hasOwn(cleaningSummary.steps[0], "ready"), false);
  assert.equal(cleaningSummary.reported_counts_status, "withheld_without_host_receipt");
  assert.equal(cleaningSummary.coverage_complete, false);
  assert.equal(cleaningSummary.fact_answer_allowed, false);

  const derived = derivePipelineScopeSummary(
    {
      case_id: "case-a",
      reported_dataset_snapshot_id: "snapshot-a",
      response_list_shape_valid: true,
      summary: {
        file_count: value,
        reported_counts_status: "all_reported_counts_present"
      },
      warnings: []
    },
    {
      case_id: "case-a",
      reported_dataset_snapshot_id: "snapshot-a",
      latest_job: { cleaned_rows: value, summary: { invalid: value, failed: value } },
      latest_history: {},
      warnings: []
    },
    {
      case_id: "case-a",
      reported_dataset_snapshot_id: "snapshot-a",
      meta: { dataset_snapshot_id: "snapshot-a", funds_status: "ready" }
    },
    { total_groups: value, total_items: value },
    { total_groups: value, total_items: value }
  );
  for (const key of [
    "file_count",
    "cleaned_rows",
    "invalid_rows",
    "failed_rows",
    "holder_group_count",
    "account_count_by_name_tree",
    "bank_group_count",
    "account_count_by_card_tree"
  ]) {
    assert.equal(hasOwn(derived, key), false, `derived/${label} must omit ${key}`);
  }
  assert.equal(derived.funds_status, "reported_success_unverified");
  assert.equal(hasOwn(derived, "import_files_available"), false);
  assert.equal(derived.import_files_availability, "host_receipt_required");
  assert.equal(derived.snapshot_binding_status, "reported_match_unverified");
  assertBoundaryOnly(derived, `derived/${label}`);

  assert.equal(firstPositive(value), undefined, `firstPositive/${label} must not synthesize zero`);
}

const zeroImport = Object.fromEntries(importCountFields.map((field) => [field, 0]));
zeroImport.kind = "transactions";
zeroImport.file_type = "csv";
zeroImport.status = "success";
const compactZeroImport = compactImportRow(zeroImport);
for (const field of importCountFields) {
  assert.equal(compactZeroImport[field], 0, `explicit import zero must survive for ${field}`);
}
assert.equal(compactZeroImport.status, "reported_success_unverified");
assert.equal(compactZeroImport.coverage_complete, false);
assert.equal(compactZeroImport.safe_for_case_conclusion, false);

const zeroImportSummary = summarizeImportFiles([zeroImport]);
assert.equal(zeroImportSummary.file_count, 1);
assert.equal(zeroImportSummary.total_size, 0);
for (const field of importCountFields.filter((field) => field !== "size")) {
  assert.equal(zeroImportSummary[field], 0, `explicit aggregate zero must survive for ${field}`);
}
assert.equal(zeroImportSummary.reported_counts_status, "all_reported_counts_present");
assert.equal(zeroImportSummary.by_status.reported_success_unverified.rows_total, 0);
assert.equal(zeroImportSummary.coverage_complete, false);
assert.equal(zeroImportSummary.safe_for_case_conclusion, false);
assert.equal(zeroImportSummary.zero_result_verified, false);

const partialImportSummary = summarizeImportFiles([
  zeroImport,
  { ...zeroImport, rows_total: "invalid", rows_imported_norm: null }
]);
assert.equal(hasOwn(partialImportSummary, "rows_total"), false);
assert.equal(hasOwn(partialImportSummary, "rows_imported_norm"), false);
assert.equal(partialImportSummary.rows_error, 0, "independently complete aggregates may preserve explicit zero");
assert.equal(partialImportSummary.reported_counts_status, "partial_or_invalid");

const emptyImportSummary = summarizeImportFiles([]);
assert.equal(emptyImportSummary.file_count, 0, "an explicit empty response may report zero returned files");
assert.equal(hasOwn(emptyImportSummary, "rows_total"), false);
assert.equal(emptyImportSummary.coverage_complete, false);
assert.equal(emptyImportSummary.zero_result_verified, false);
const malformedImportList = summarizeImportFiles(false);
assert.equal(hasOwn(malformedImportList, "file_count"), false);
assert.equal(malformedImportList.reported_counts_status, "unavailable");
const invalidImportItemList = summarizeImportFiles([false]);
assert.equal(hasOwn(invalidImportItemList, "file_count"), false);
assert.equal(invalidImportItemList.invalid_item_count, 1);
const mixedInvalidImportItemList = summarizeImportFiles([zeroImport, false]);
assert.equal(hasOwn(mixedInvalidImportItemList, "file_count"), false);
assert.equal(hasOwn(mixedInvalidImportItemList, "rows_total"), false);
assert.equal(hasOwn(mixedInvalidImportItemList, "by_kind"), false);

const zeroCleaningSummary = summarizeCleaningSteps([{ step: 0, affected_rows: 0, status: "ok" }]);
assert.equal(zeroCleaningSummary.step_count, 1);
for (const key of [
  "hit_step_count",
  "affected_rows_total",
  "transaction_step_affected",
  "account_step_affected"
]) {
  assert.equal(hasOwn(zeroCleaningSummary, key), false, `cleaning zero must omit ${key}`);
}
assert.equal(zeroCleaningSummary.steps[0].status, "reported_success_unverified");
assert.equal(zeroCleaningSummary.reported_counts_status, "withheld_without_host_receipt");
assert.equal(zeroCleaningSummary.coverage_complete, false);
assert.equal(zeroCleaningSummary.safe_for_case_conclusion, false);
assert.equal(zeroCleaningSummary.zero_result_verified, false);
assert.equal(zeroCleaningSummary.fact_answer_allowed, false);
const emptyCleaningSummary = summarizeCleaningSteps([]);
assert.equal(emptyCleaningSummary.step_count, 0);
assert.equal(hasOwn(emptyCleaningSummary, "affected_rows_total"), false);
assert.equal(emptyCleaningSummary.zero_result_verified, false);
const malformedCleaningList = summarizeCleaningSteps(false);
assert.equal(hasOwn(malformedCleaningList, "step_count"), false);

assert.equal(firstPositive(0, "0", null), undefined);
assert.equal(firstPositive(undefined, "2", 3), 2);

const zeroDerived = derivePipelineScopeSummary(
  {
    case_id: "case-zero",
    reported_dataset_snapshot_id: "snapshot-zero",
    response_list_shape_valid: true,
    summary: { file_count: 0, reported_counts_status: "all_reported_counts_present" },
    warnings: []
  },
  {
    case_id: "case-zero",
    reported_dataset_snapshot_id: "snapshot-zero",
    latest_job: { cleaned_rows: 0, summary: { invalid: 0, failed: 0, reversal: 0 } },
    warnings: []
  },
  {
    case_id: "case-zero",
    reported_dataset_snapshot_id: "snapshot-zero",
    meta: { dataset_snapshot_id: "snapshot-zero", funds_status: "passed" }
  },
  { total_groups: 0, total_items: 0 },
  { total_groups: 0, total_items: 0 }
);
for (const key of [
  "file_count",
  "cleaned_rows",
  "invalid_rows",
  "failed_rows",
  "reversal_rows",
  "holder_group_count",
  "account_count_by_name_tree",
  "bank_group_count",
  "account_count_by_card_tree"
]) {
  assert.equal(hasOwn(zeroDerived, key), false, `unverified derived zero must be withheld for ${key}`);
}
assert.equal(zeroDerived.funds_status, "reported_success_unverified");
assert.equal(hasOwn(zeroDerived, "import_files_available"), false);
assert.equal(zeroDerived.import_files_availability, "host_receipt_required");
assert.equal(zeroDerived.snapshot_binding_status, "reported_match_unverified");
assert.equal(hasOwn(zeroDerived, "dataset_snapshot_id"), false);
assertBoundaryOnly(zeroDerived, "zero-derived");

const malformedTree = compactTreeGroups(false);
assert.equal(hasOwn(malformedTree, "total_groups"), false);
assert.equal(hasOwn(malformedTree, "total_items"), false);
assert.equal(malformedTree.response_shape_valid, false);
assertBoundaryOnly(malformedTree, "malformed-tree");
const emptyTree = compactTreeGroups([]);
assert.equal(emptyTree.total_groups, 0);
assert.equal(emptyTree.total_items, 0);
assert.equal(emptyTree.response_shape_valid, true);
assertBoundaryOnly(emptyTree, "empty-tree");
const malformedTreeItems = compactTreeGroups([{ id: "group-a", items: false }]);
assert.equal(hasOwn(malformedTreeItems, "total_items"), false);
assert.equal(malformedTreeItems.response_shape_valid, false);
const explicitEmptyTreeItems = compactTreeGroups([{ id: "group-a", items: [] }]);
assert.equal(explicitEmptyTreeItems.total_groups, 1);
assert.equal(explicitEmptyTreeItems.total_items, 0);
assert.equal(explicitEmptyTreeItems.zero_result_verified, false);

const caseRecord = compactCaseRecord({
  case_id: "case-zero",
  status: "ready",
  duckdb_ready: true,
  backend_status: "passed",
  import_health: "success",
  size_bytes: false,
  imports: [zeroImport]
}, { includeImports: true });
assert.equal(caseRecord.status, "reported_success_unverified");
assert.equal(caseRecord.backend_status, "reported_success_unverified");
assert.equal(caseRecord.import_health, "reported_success_unverified");
assert.equal(hasOwn(caseRecord, "duckdb_ready"), false);
assert.equal(hasOwn(caseRecord, "size_bytes"), false);
assert.equal(caseRecord.import_summary.rows_total, 0);
assert.equal(caseRecord.import_summary.coverage_complete, false);

const failingRuntime = createCasePipelineRuntime({
  resolveCase: async () => ({ case_id: "case-fail", source: "test", case: {} }),
  httpJson: async () => {
    throw new Error("source offline");
  },
  httpPostData: async () => {
    throw new Error("source offline");
  }
});
const failedImportOverview = await failingRuntime.getImportOverview({ include_jobs: true });
assert.equal(hasOwn(failedImportOverview.summary, "file_count"), false);
assert.equal(hasOwn(failedImportOverview.summary, "rows_total"), false);
assert.equal(failedImportOverview.response_list_shape_valid, false);
assert.equal(failedImportOverview.source_status, "unavailable_or_invalid");
assertBoundaryOnly(failedImportOverview, "failed-import-overview");
const failedCleaningOverview = await failingRuntime.getCleaningOverview({ include_logs: true });
assert.equal(hasOwn(failedCleaningOverview.step_summary, "step_count"), false);
assert.equal(hasOwn(failedCleaningOverview.step_summary, "affected_rows_total"), false);
assert.equal(failedCleaningOverview.steps_list_shape_valid, false);
assert.equal(failedCleaningOverview.source_status, "unavailable_or_invalid");
assertBoundaryOnly(failedCleaningOverview, "failed-cleaning-overview");

const validRuntime = createCasePipelineRuntime({
  resolveCase: async () => ({
    case_id: "case-zero",
    source: "test",
    case: { case_id: "case-zero", status: "ready", stats: { tx: false } }
  }),
  httpJson: async (path) => {
    if (path.startsWith("/import/files?")) {
      return { data: { items: [zeroImport], dataset_snapshot_id: "snapshot-zero" } };
    }
    if (path.startsWith("/import/jobs?")) {
      return { data: { items: [{ status: "passed", cleaned_rows: false }], page: { total: false } } };
    }
    if (path.startsWith("/cleaning/jobs?")) {
      return {
        data: {
          dataset_snapshot_id: "snapshot-zero",
          items: [{ status: "success", cleaned_rows: false, summary: { invalid: false } }],
          page: { total: false }
        }
      };
    }
    if (path.startsWith("/cleaning/history?")) {
      return { data: { items: [{ status: "ready", file_count: false }] } };
    }
    if (path.startsWith("/cleaning/steps?")) {
      return {
        data: {
          dataset_snapshot_id: "snapshot-zero",
          items: [{ step: 0, affected_rows: 0, status: "passed", ready: true }]
        }
      };
    }
    if (path.startsWith("/cleaning/logs?")) {
      return { data: { items: [{ status: "ok", affected_rows: false }] } };
    }
    throw new Error(`unexpected GET ${path}`);
  },
  httpPostData: async (path) => {
    if (path === "/analysis/stats/v2/meta") {
      return {
        dataset_snapshot_id: "snapshot-zero",
        funds_status: "ready",
        txn_count: false,
        rowCount: false,
        turnover: " ",
        cleaned_rows: "7",
        source_ready: true,
        safeToAnswer: "true",
        success: true
      };
    }
    if (path === "/analysis/stats/v2/tree") {
      return { groups: [] };
    }
    throw new Error(`unexpected POST ${path}`);
  }
});

const validImportOverview = await validRuntime.getImportOverview({ include_jobs: true });
assert.equal(validImportOverview.summary.rows_total, 0);
assert.equal(validImportOverview.summary.zero_result_verified, false);
assert.equal(validImportOverview.files[0].status, "reported_success_unverified");
assert.equal(validImportOverview.latest_jobs[0].status, "reported_success_unverified");
assert.equal(hasOwn(validImportOverview.latest_jobs[0], "cleaned_rows"), false);
assert.equal(hasOwn(validImportOverview.jobs_page, "total"), false);
assert.equal(validImportOverview.response_list_shape_valid, true);
assertBoundaryOnly(validImportOverview, "valid-import-overview");

const validCleaningOverview = await validRuntime.getCleaningOverview({ include_logs: true });
assert.equal(hasOwn(validCleaningOverview.step_summary, "affected_rows_total"), false);
assert.equal(validCleaningOverview.step_summary.zero_result_verified, false);
assert.equal(validCleaningOverview.step_summary.fact_answer_allowed, false);
assert.equal(validCleaningOverview.latest_job.status, "reported_success_unverified");
assert.equal(hasOwn(validCleaningOverview.latest_job, "cleaned_rows"), false);
assert.equal(validCleaningOverview.steps_list_shape_valid, true);
assertBoundaryOnly(validCleaningOverview, "valid-cleaning-overview");

const validStatsMeta = await validRuntime.getStatsMeta();
assert.equal(validStatsMeta.meta.funds_status, "reported_success_unverified");
assert.equal(hasOwn(validStatsMeta.meta, "cleaned_rows"), false);
assert.equal(validStatsMeta.meta.fact_answer_allowed, false);
assert.equal(hasOwn(validStatsMeta.meta, "txn_count"), false);
assert.equal(hasOwn(validStatsMeta.meta, "rowCount"), false);
assert.equal(hasOwn(validStatsMeta.meta, "turnover"), false);
assert.equal(hasOwn(validStatsMeta.meta, "source_ready"), false);
assert.equal(hasOwn(validStatsMeta.meta, "safeToAnswer"), false);
assert.equal(hasOwn(validStatsMeta.meta, "success"), false);
assertBoundaryOnly(validStatsMeta, "valid-stats-meta");

const validTree = await validRuntime.getStatsTree();
assert.equal(hasOwn(validTree, "total_groups"), false);
assert.equal(hasOwn(validTree, "total_items"), false);
assert.equal(validTree.fact_answer_allowed, false);
assertBoundaryOnly(validTree, "valid-tree");

const pipelineOverview = await validRuntime.getCaseDataPipelineOverview();
assert.equal(hasOwn(pipelineOverview.data_scope_summary, "import_files_available"), false);
assert.equal(pipelineOverview.data_scope_summary.import_files_availability, "host_receipt_required");
assert.equal(pipelineOverview.data_scope_summary.snapshot_binding_status, "reported_match_unverified");
assertBoundaryOnly(pipelineOverview.data_scope_summary, "pipeline-scope");
assertBoundaryOnly(pipelineOverview, "pipeline-overview");
assert.doesNotMatch(
  JSON.stringify(pipelineOverview),
  /"(?:status|funds_status|backend_status|import_health)":"(?:ok|ready|passed|success|succeeded|complete|completed)"/iu,
  "upstream success labels must not survive as authoritative readiness"
);

process.stdout.write(`${JSON.stringify({
  status: "ok",
  checks: [
    "invalid external import, cleaning, dedupe, and stats numerics never become zero",
    "partial aggregates are omitted instead of summing malformed rows",
    "explicit zero remains visible but never proves no-hit or complete coverage",
    "source failures do not become empty successful inventories",
    "skill source failures and malformed responses remain explicit unavailable or invalid states",
    "only a strict data.items response can represent an available zero-skill inventory",
    "reported ready, passed, and success statuses are downgraded",
    "tree and pipeline outputs remain publication-blocked without host evidence"
  ]
}, null, 2)}\n`);
