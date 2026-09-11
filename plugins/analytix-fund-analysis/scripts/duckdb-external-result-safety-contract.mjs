#!/usr/bin/env node

import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import {
  executeLocalDuckdbWorkbenchSkill,
  notebookTruncatedLabel
} from "../mcp/duckdb-workbench-runtime.mjs";
import {
  hostFactRuntimeContext,
  writeCaseProjectBinding
} from "./host-runtime-context-fixture.mjs";

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-duckdb-result-safety-"));
const projectRoot = path.join(tempRoot, "project");
const casesRoot = path.join(tempRoot, "cases");
const caseId = "case_external_result_safety";
const caseRoot = path.join(casesRoot, caseId);
const dbPath = path.join(caseRoot, "case.duckdb");
const runnerPath = path.join(tempRoot, "fake-duckdb-runner.mjs");
const runnerConfigPath = path.join(tempRoot, "fake-duckdb-runner.json");
const runnerSqlLogPath = path.join(tempRoot, "fake-duckdb-sql.jsonl");

fs.mkdirSync(caseRoot, { recursive: true });
fs.writeFileSync(dbPath, "not-a-real-duckdb", "utf8");
const binding = writeCaseProjectBinding(projectRoot, caseId);
const datasetSnapshotId = `dsv1_${"a".repeat(64)}`;
const factualDatasetSnapshotId = `dsv2_${"b".repeat(64)}`;

fs.writeFileSync(runnerPath, `#!/usr/bin/env node
import fs from "node:fs";
let input = "";
for await (const chunk of process.stdin) input += chunk;
const payload = JSON.parse(input || "{}");
const sql = String(payload.sql || "");
const config = JSON.parse(fs.readFileSync(${JSON.stringify(runnerConfigPath)}, "utf8"));
fs.appendFileSync(${JSON.stringify(runnerSqlLogPath)}, JSON.stringify({ mode: config.mode, sql }) + "\\n", "utf8");
const mode = config.mode || "valid-count-zero";
const snapshot = payload.snapshot_case_id ? {
  snapshot_contract: config.snapshot_contract || "analytix_duckdb_dataset_snapshot_v1",
  snapshot_factual_ready: config.snapshot_factual_ready === true,
  snapshot_blocker: config.snapshot_blocker ?? "legacy_exact_count_quarantine",
  snapshot_manifest_schema_version: config.snapshot_manifest_schema_version || 0,
  observed_dataset_snapshot_id: config.snapshot
} : {
  snapshot_contract: "",
  snapshot_factual_ready: false,
  snapshot_blocker: "",
  snapshot_manifest_schema_version: 0,
  observed_dataset_snapshot_id: ""
};
const result = (columns, records, truncated = false) => {
  const output = { columns, records, row_count: records.length, truncated, ...snapshot };
  if (mode === "snapshot-unknown-blocker") output.snapshot_blocker = "6222029912345678901";
  if (mode === "snapshot-factual-ready-v1") output.snapshot_factual_ready = true;
  if (mode === "snapshot-undeclared-root") output.raw_error = "6222029912345678901";
  return output;
};
if (mode === "malformed-json") {
  process.stdout.write("{");
  process.exit(0);
}
if (mode === "nonfinite-json") {
  process.stdout.write('{"columns":["row_count"],"records":[{"row_count":NaN}],"row_count":1,"truncated":false}');
  process.exit(0);
}
if (mode === "missing-wrapper-count") {
  process.stdout.write(JSON.stringify({ columns: ["row_count"], records: [{ row_count: 0 }], truncated: false }));
  process.exit(0);
}
if (mode === "mismatched-wrapper-count") {
  process.stdout.write(JSON.stringify({ columns: ["row_count"], records: [], row_count: 1, truncated: false }));
  process.exit(0);
}
if (payload.policy_explain_only || /^EXPLAIN\\b/iu.test(sql.trim())) {
  process.stdout.write(JSON.stringify(result(["explain_key"], [{ explain_key: "plan" }])));
  process.exit(0);
}
if (sql.includes("information_schema.tables")) {
  process.stdout.write(JSON.stringify(result(
    ["table_schema", "table_name", "table_type"],
    [{ table_schema: "main", table_name: "analysis_txn_detail_idx", table_type: "BASE TABLE" }],
    mode === "scope-catalog-truncated"
  )));
  process.exit(0);
}
if (sql.includes("information_schema.columns")) {
  process.stdout.write(JSON.stringify(result(
    ["table_name", "column_name", "data_type", "is_nullable", "ordinal_position"],
    [{ table_name: "analysis_txn_detail_idx", column_name: "id", data_type: "BIGINT", is_nullable: "YES", ordinal_position: 1 }]
  )));
  process.exit(0);
}
if (sql.includes("amount_present_rows")) {
  const invalid = mode === "scope-detail-invalid";
  const amountCounts = mode === "scope-amount-partial"
    ? { total_rows: 4, amount_present_rows: 2, amount_missing_rows: 1, amount_parse_failed_rows: 1 }
    : (mode === "scope-amount-inconsistent"
      ? { total_rows: 4, amount_present_rows: 4, amount_missing_rows: 1, amount_parse_failed_rows: 0 }
      : { total_rows: 0, amount_present_rows: 0, amount_missing_rows: 0, amount_parse_failed_rows: 0 });
  const record = {
    ...amountCounts,
    account_count: 0,
    holder_count: 0,
    counterparty_key_count: 0,
    counterparty_name_count: 0,
    turnover_amount: null,
    inflow_amount: null,
    outflow_amount: null,
    first_txn_at: null,
    last_txn_at: null
  };
  if (invalid) record.total_rows = false;
  process.stdout.write(JSON.stringify(result(Object.keys(record), [record])));
  process.exit(0);
}
if (sql.includes("AS requested_rows") && sql.includes("AS direction_covered_rows")) {
  const total = mode === "ranking-complete-zero" ? 0 : 1;
  const cashConflict = mode.startsWith("ranking-cash-conflict-");
  const record = {
    requested_rows: total,
    amount_covered_rows: mode === "ranking-missing-amount" ? 0 : total,
    direction_covered_rows: mode === "ranking-missing-direction" ? 0 : total,
    timestamp_covered_rows: total,
    grouping_covered_rows: total,
    cash_covered_rows: mode === "ranking-missing-cash" || cashConflict ? 0 : total,
    cash_conflict_rows: cashConflict ? total : 0,
    success_covered_rows: mode === "ranking-missing-success" ? 0 : total,
    date_covered_rows: total,
    source_file_covered_rows: total,
    account_scope_covered_rows: total,
    holder_scope_covered_rows: total,
    id_scope_covered_rows: total,
    source_holder_covered_rows: total
  };
  process.stdout.write(JSON.stringify(result(Object.keys(record), [record])));
  process.exit(0);
}
if (/SELECT\\s+COUNT\\(\\*\\)\\s+AS\\s+row_count/iu.test(sql)) {
  let value = 0;
  if (mode === "count-missing") {
    process.stdout.write(JSON.stringify(result(["row_count"], [{}])));
    process.exit(0);
  }
  if (mode === "count-invalid" || mode === "scope-table-count-invalid") {
    value = config.value;
  }
  if (mode === "scope-amount-partial" || mode === "scope-amount-inconsistent") {
    value = 4;
  }
  process.stdout.write(JSON.stringify(result(["row_count"], [{ row_count: value }])));
  process.exit(0);
}
if (sql.includes("WITH grouped AS")) {
  const columns = ["rank", "account_key", "txn_count", "inflow_total", "outflow_total", "turnover_total", "net_flow", "max_single_amount", "account_count"];
  if (mode === "ranking-invalid") {
    process.stdout.write(JSON.stringify(result(columns, [{ rank: 1, account_key: "a", txn_count: 1, inflow_total: 10, outflow_total: null, turnover_total: 10, net_flow: 10, max_single_amount: 10, account_count: false }])));
  } else if (mode === "ranking-real-zero") {
    process.stdout.write(JSON.stringify(result(columns, [{ rank: 1, account_key: "a", txn_count: 1, inflow_total: 0, outflow_total: null, turnover_total: 0, net_flow: 0, max_single_amount: 0, account_count: 1 }])));
  } else {
    process.stdout.write(JSON.stringify(result(columns, [], mode === "ranking-incomplete-empty")));
  }
  process.exit(0);
}
if (mode === "run-invalid-count-fact") {
  process.stdout.write(JSON.stringify(result(["txn_count"], [{ txn_count: false }])));
  process.exit(0);
}
if (mode === "run-count-zero") {
  process.stdout.write(JSON.stringify(result(["row_count"], [{ row_count: 0 }])));
  process.exit(0);
}
process.stdout.write(JSON.stringify(result(["id"], [])));
`, "utf8");
fs.chmodSync(runnerPath, 0o755);

const baseEnv = {
  ...process.env,
  ANALYTIX_CASE_PROJECT_ROOT: projectRoot,
  ANALYTIX_WORKSPACE_ROOT: projectRoot,
  ANALYTIX_DATA_ANALYSIS_CASES_ROOT: casesRoot,
  ANALYTIX_FUNDS_DUCKDB_PYTHON: runnerPath,
  ANALYTIX_BACKEND_PYTHON: runnerPath,
  PYTHON: runnerPath
};

async function execute(mode, skill, args, value) {
  const rankingSkill = ["rank_accounts", "rank_holders", "rank_counterparties"].includes(skill);
  const rankingLegacySnapshot = mode === "ranking-legacy-snapshot";
  fs.writeFileSync(runnerConfigPath, JSON.stringify({
    mode,
    snapshot: rankingSkill && !rankingLegacySnapshot ? factualDatasetSnapshotId : datasetSnapshotId,
    ...(rankingSkill ? {
      snapshot_contract: rankingLegacySnapshot
        ? "analytix_duckdb_dataset_snapshot_v1"
        : "analytix_duckdb_dataset_snapshot_v2",
      snapshot_factual_ready: !rankingLegacySnapshot,
      snapshot_blocker: rankingLegacySnapshot ? "legacy_exact_count_quarantine" : "",
      snapshot_manifest_schema_version: rankingLegacySnapshot ? 0 : 2
    } : {}),
    ...(value === undefined ? {} : { value })
  }));
  const input = { case_id: caseId, ...args };
  if (skill === "run_case_sql" || rankingSkill) {
    input._analytix = hostFactRuntimeContext({
      ...binding,
      caseId,
      toolName: skill,
      args,
      overrides: { datasetSnapshotId: rankingSkill ? factualDatasetSnapshotId : datasetSnapshotId }
    });
  }
  return executeLocalDuckdbWorkbenchSkill(skill, input, { env: baseEnv });
}

const countArgs = { table_name: "analysis_txn_detail_idx" };
for (const [label, mode, value] of [
  ["missing", "count-missing", undefined],
  ["blank", "count-invalid", ""],
  ["boolean", "count-invalid", false],
  ["object", "count-invalid", {}],
  ["null", "count-invalid", null],
  ["unsafe integer", "count-invalid", Number.MAX_SAFE_INTEGER + 1],
  ["nonfinite", "nonfinite-json", undefined],
  ["malformed", "malformed-json", undefined],
  ["missing wrapper count", "missing-wrapper-count", undefined],
  ["mismatched wrapper count", "mismatched-wrapper-count", undefined],
  ["unknown snapshot blocker", "snapshot-unknown-blocker", undefined],
  ["legacy snapshot readiness upgrade", "snapshot-factual-ready-v1", undefined],
  ["undeclared runner root field", "snapshot-undeclared-root", undefined]
]) {
  await assert.rejects(
    execute(mode, "count_case_rows", countArgs, value),
    undefined,
    `${label} COUNT result must fail closed`
  );
}

const verifiedZeroCount = await execute("valid-count-zero", "count_case_rows", countArgs);
assert.equal(verifiedZeroCount.data.row_count, 0);
assert.equal(verifiedZeroCount.data.zero_result_status, "unresolved");
assert.equal(verifiedZeroCount.data.result_completeness.status, "partial");
assert.equal(verifiedZeroCount.data.result_completeness.factual_snapshot_ready, false);
assert.equal(verifiedZeroCount.data.result_completeness.result_integrity_validated, true);
assert.equal(verifiedZeroCount.data.query_scope.case_id, caseId);
assert.deepEqual(verifiedZeroCount.data.query_scope.source_scope, ["analysis_txn_detail_idx"]);
assert.ok(verifiedZeroCount.data.query_scope.query_hash);

await assert.rejects(
  execute("ranking-invalid", "rank_accounts", { limit: 10 }),
  undefined,
  "malformed ranking summary count must fail closed"
);

await assert.rejects(
  executeLocalDuckdbWorkbenchSkill(
    "rank_accounts",
    { case_id: caseId, limit: 10 },
    { env: baseEnv }
  ),
  /frozen same-turn dataset snapshot authority/u,
  "ranking fallback without frozen host authority must fail closed"
);

for (const [mode, args, expectedCode] of [
  ["ranking-missing-amount", { limit: 10 }, "result_contract_invalid"],
  ["ranking-missing-direction", { limit: 10 }, "result_contract_invalid"],
  ["ranking-missing-cash", { limit: 10, cash_filter: "non_cash_only" }, "result_contract_invalid"],
  ["ranking-cash-conflict-explicit-false", { limit: 10, cash_filter: "non_cash_only" }, "result_contract_invalid"],
  ["ranking-cash-conflict-explicit-true", { limit: 10, cash_filter: "cash_only" }, "result_contract_invalid"],
  ["ranking-missing-success", { limit: 10, success_filter: "success_only" }, "result_contract_invalid"],
  ["ranking-legacy-snapshot", { limit: 10 }, "dataset_snapshot_mismatch"]
]) {
  await assert.rejects(
    execute(mode, "rank_accounts", args),
    (error) => error?.duckdbDiagnostic?.code === expectedCode,
    `${mode} must fail closed before returning ranking facts`
  );
}

for (const [mode, args] of [
  ["ranking-complete-zero", { limit: 10 }],
  ["ranking-incomplete-empty", { limit: 10 }],
  ["ranking-real-zero", { limit: 10, cash_filter: "non_cash_only", success_filter: "success_only" }]
]) {
  await assert.rejects(
    execute(mode, "rank_accounts", args),
    (error) => error?.duckdbDiagnostic?.code === "result_contract_invalid",
    `${mode} must not turn a local dsv2 digest into factual ranking output`
  );
}

assert.equal(notebookTruncatedLabel(undefined), "unknown");
assert.equal(notebookTruncatedLabel(null), "unknown");
assert.equal(notebookTruncatedLabel(false), "false");
assert.equal(notebookTruncatedLabel(true), "true");

await assert.rejects(
  execute("scope-detail-invalid", "get_scope_coverage", {}),
  undefined,
  "malformed scope aggregate must fail closed"
);
const partialCoverage = await execute("scope-table-count-invalid", "get_scope_coverage", {}, false);
assert.equal(partialCoverage.data.coverage_status, "partial");
assert.equal(partialCoverage.data.result_completeness.status, "partial");
assert.equal(partialCoverage.data.zero_result_status, "unresolved");
assert.notEqual(partialCoverage.data.zero_result_status, "candidate_no_hit");
const truncatedCoverage = await execute("scope-catalog-truncated", "get_scope_coverage", {});
assert.equal(truncatedCoverage.data.coverage_status, "partial");
assert.equal(truncatedCoverage.data.zero_result_status, "unresolved");

const verifiedZeroCoverage = await execute("scope-zero", "get_scope_coverage", {});
assert.equal(verifiedZeroCoverage.data.coverage_status, "partial");
assert.equal(verifiedZeroCoverage.data.transaction_detail.row_count, 0);
assert.equal(verifiedZeroCoverage.data.zero_result_status, "unresolved");
assert.equal(verifiedZeroCoverage.data.result_completeness.status, "partial");
for (const field of ["turnover_amount", "inflow_amount", "outflow_amount"]) {
  assert.equal(Object.prototype.hasOwnProperty.call(verifiedZeroCoverage.data.transaction_detail, field), false);
}

const amountPartialCoverage = await execute("scope-amount-partial", "get_scope_coverage", {});
assert.equal(amountPartialCoverage.data.coverage_status, "partial");
assert.equal(amountPartialCoverage.data.transaction_detail.total_rows, 4);
assert.equal(amountPartialCoverage.data.transaction_detail.amount_present_rows, 2);
assert.equal(amountPartialCoverage.data.transaction_detail.amount_missing_rows, 1);
assert.equal(amountPartialCoverage.data.transaction_detail.amount_parse_failed_rows, 1);
assert.equal(amountPartialCoverage.data.transaction_detail.amount_coverage_rate, 0.5);
assert.equal(amountPartialCoverage.data.transaction_detail.amount_coverage_complete, false);
assert.equal(amountPartialCoverage.data.result_completeness.amount_coverage_complete, false);
assert.equal(amountPartialCoverage.data.validation_summary.coverage_complete, false);
assert.ok(amountPartialCoverage.warnings.some((warning) => warning.code === "AMOUNT_COVERAGE_PARTIAL"));

await assert.rejects(
  execute("scope-amount-inconsistent", "get_scope_coverage", {}),
  /inconsistent amount quality counts/u,
  "amount coverage count mismatch must fail closed"
);

await assert.rejects(
  execute("run-invalid-count-fact", "run_case_sql", {
    purpose: "count malformed result",
    sql: "SELECT COUNT(*) AS txn_count FROM analysis_precision",
    result_mode: "aggregate"
  }),
  undefined,
  "malformed run_case_sql integer facts must fail closed"
);
const verifiedRunCountZero = await execute("run-count-zero", "run_case_sql", {
  purpose: "count zero result",
  sql: "SELECT COUNT(*) AS row_count FROM analysis_precision",
  result_mode: "aggregate"
});
assert.equal(verifiedRunCountZero.data.records[0].row_count, 0);
assert.equal(verifiedRunCountZero.data.evidence_card.zero_result_status, "unresolved");
assert.equal(verifiedRunCountZero.data.evidence_card.support_status, "unsupported");
assert.equal(verifiedRunCountZero.data.evidence_card.fact_answer_allowed, false);
assert.equal(verifiedRunCountZero.data.evidence_card.result_completeness.status, "partial");
assert.deepEqual(verifiedRunCountZero.data.evidence_card.query_scope.source_scope, ["analysis_precision"]);

const verifiedEmptyRows = await execute("run-empty", "run_case_sql", {
  purpose: "bounded empty result",
  sql: "SELECT account_no FROM analysis_precision WHERE 1 = 0",
  result_mode: "evidence_table"
});
assert.equal(verifiedEmptyRows.data.row_count, 0);
assert.equal(verifiedEmptyRows.data.evidence_card.zero_result_status, "unresolved");
assert.equal(verifiedEmptyRows.data.evidence_card.support_status, "unsupported");
assert.equal(verifiedEmptyRows.data.evidence_card.result_completeness.status, "partial");

process.stdout.write(`${JSON.stringify({
  status: "ok",
  checks: [
    "COUNT external values fail closed unless strict and complete",
    "legacy zero count remains unresolved without a factual snapshot",
    "local dsv2 output never authorizes ranking facts without Go host authority",
    "ranking malformed counts cannot become exhausted or no-hit",
    "empty or incomplete local rankings remain blocked before zero/no-hit synthesis",
    "missing notebook truncation stays unknown instead of false",
    "missing or conflicting cash/success cannot bypass the local snapshot quarantine",
    "scope coverage remains partial when catalog or table counts are incomplete",
    "missing or unparseable amounts force partial coverage and inconsistent counts fail closed",
    "coverage null amounts remain absent instead of zero",
    "run_case_sql malformed facts fail closed and legacy zero stays unresolved"
  ]
}, null, 2)}\n`);
