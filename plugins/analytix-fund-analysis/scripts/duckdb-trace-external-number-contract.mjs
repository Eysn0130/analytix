#!/usr/bin/env node

import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { executeLocalDuckdbWorkbenchSkill } from "../mcp/duckdb-workbench-runtime.mjs";

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-duckdb-trace-number-"));
const projectRoot = path.join(tempRoot, "project");
const casesRoot = path.join(tempRoot, "cases");
const caseId = "case_trace_number_contract";
const caseRoot = path.join(casesRoot, caseId);
const dbPath = path.join(caseRoot, "case.duckdb");
const runnerPath = path.join(tempRoot, "fake-duckdb-runner.mjs");
const runnerConfigPath = path.join(tempRoot, "fake-duckdb-runner.json");

fs.mkdirSync(path.join(projectRoot, ".analytix"), { recursive: true });
fs.mkdirSync(caseRoot, { recursive: true });
fs.writeFileSync(dbPath, "not-a-real-duckdb", "utf8");
fs.writeFileSync(path.join(projectRoot, ".analytix", "case-project.json"), JSON.stringify({
  version: 1,
  source: "analytix-data-analysis",
  caseId,
  workspaceRoot: projectRoot
}, null, 2));

fs.writeFileSync(runnerPath, `#!/usr/bin/env node
import fs from "node:fs";
let input = "";
for await (const chunk of process.stdin) input += chunk;
const payload = JSON.parse(input || "{}");
const sql = String(payload.sql || "");
const config = JSON.parse(fs.readFileSync(${JSON.stringify(runnerConfigPath)}, "utf8"));
const mode = config.mode || "subject-valid";
const result = (columns, records, truncated = false) => ({
  columns,
  records,
  row_count: records.length,
  truncated,
  snapshot_contract: "",
  snapshot_factual_ready: false,
  snapshot_blocker: "",
  snapshot_manifest_schema_version: 0,
  observed_dataset_snapshot_id: ""
});
const subjectScope = () => {
  const record = mode === "subject-empty-zero"
    ? {
        txn_count: 0,
        source_account_count: 0,
        counterparty_count: 0,
        outflow_total: 0,
        first_txn_at: null,
        last_txn_at: null,
        missing_counterparty_name_count: 0,
        missing_counterparty_account_count: 0
      }
    : {
        txn_count: 2,
        source_account_count: 1,
        counterparty_count: 1,
        outflow_total: 1000,
        first_txn_at: "2026-01-01 09:00:00",
        last_txn_at: "2026-01-02 09:00:00",
        missing_counterparty_name_count: 0,
        missing_counterparty_account_count: 0
      };
  return result(Object.keys(record), [record]);
};
const subjectRank = () => {
  const columns = [
    "rank", "counterparty_key", "display_name", "counterparty_account", "txn_count",
    "source_account_count", "outflow_total", "max_single_amount", "first_txn_at", "last_txn_at",
    "target_account_in_case", "total_counterparty_count", "seed_txn_id", "seed_txn_time",
    "seed_account_key", "seed_holder_name", "seed_counterparty_key", "seed_counterparty_account",
    "seed_amount", "seed_summary", "seed_txn_type", "seed_file_id"
  ];
  if (mode === "subject-empty-zero") return result(columns, []);
  const record = {
    rank: 1,
    counterparty_key: "cp-1",
    display_name: "测试对手公司",
    counterparty_account: "acct-cp-1",
    txn_count: 2,
    source_account_count: 1,
    outflow_total: 1000,
    max_single_amount: 600,
    first_txn_at: "2026-01-01 09:00:00",
    last_txn_at: "2026-01-02 09:00:00",
    target_account_in_case: 0,
    total_counterparty_count: 1,
    seed_txn_id: "seed-1",
    seed_txn_time: "2026-01-02 09:00:00",
    seed_account_key: "source-1",
    seed_holder_name: "测试主体",
    seed_counterparty_key: "cp-1",
    seed_counterparty_account: "acct-cp-1",
    seed_amount: 600,
    seed_summary: "测试出账",
    seed_txn_type: "transfer",
    seed_file_id: "file-1"
  };
  if (mode === "subject-invalid-rank") record.rank = false;
  if (mode === "subject-zero-rank") record.rank = 0;
  if (mode === "subject-missing-count") delete record.txn_count;
  if (mode === "subject-missing-amount") delete record.outflow_total;
  if (mode === "subject-zero-amount") record.outflow_total = 0;
  if (mode === "subject-missing-coverage") delete record.target_account_in_case;
  return result(Object.keys(record), [record]);
};
const seedResult = () => {
  const record = {
    txn_id: "seed-next-1",
    txn_time: "2026-02-01 10:00:00",
    txn_ts: "2026-02-01 10:00:00",
    acct_key: "source-next-1",
    account_open_name: "测试主体",
    counterparty_acct: "receiver-1",
    counterparty_name: "收款主体",
    cp_key: "receiver-key-1",
    amount: 500,
    summary: "下一跳种子",
    txn_type: "transfer",
    file_id: "file-next-1"
  };
  if (mode === "next-missing-seed-amount") delete record.amount;
  if (mode === "next-zero-seed-amount") record.amount = 0;
  return result(Object.keys(record), [record]);
};
const nextHopResult = () => {
  const columns = [
    "rank", "txn_id", "txn_time", "txn_ts", "acct_key", "account_open_name",
    "counterparty_acct", "counterparty_name", "cp_key", "amount", "summary", "txn_type", "file_id"
  ];
  if (mode === "next-empty") return result(columns, []);
  const record = {
    rank: 1,
    txn_id: "next-1",
    txn_time: "2026-02-01 11:00:00",
    txn_ts: "2026-02-01 11:00:00",
    acct_key: "receiver-1",
    account_open_name: "收款主体",
    counterparty_acct: "terminal-1",
    counterparty_name: "下游主体",
    cp_key: "terminal-key-1",
    amount: 200,
    summary: "下一跳出账",
    txn_type: "transfer",
    file_id: "file-next-2"
  };
  if (mode === "next-missing-hop-amount") delete record.amount;
  if (mode === "next-zero-hop-amount") record.amount = 0;
  if (mode === "next-zero-hop-rank") record.rank = 0;
  return result(Object.keys(record), [record]);
};
if (sql.includes("missing_counterparty_name_count") && sql.includes("COUNT(*) AS txn_count")) {
  process.stdout.write(JSON.stringify(subjectScope()));
} else if (sql.includes("seed_rows AS") && sql.includes("total_counterparty_count")) {
  process.stdout.write(JSON.stringify(subjectRank()));
} else if (sql.includes("ORDER BY txn_ts DESC, amount DESC") && sql.includes("LIMIT 1")) {
  process.stdout.write(JSON.stringify(seedResult()));
} else if (sql.includes("ROW_NUMBER() OVER (ORDER BY txn_ts ASC")) {
  process.stdout.write(JSON.stringify(nextHopResult()));
} else {
  process.stdout.write(JSON.stringify(result(["id"], [])));
}
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

async function execute(mode, skillId, args = {}) {
  fs.writeFileSync(runnerConfigPath, JSON.stringify({ mode }));
  return executeLocalDuckdbWorkbenchSkill(skillId, { case_id: caseId, ...args }, {
    env: baseEnv
  });
}

function assertNoVerifiedNoHit(value, message) {
  assert.equal(JSON.stringify(value).includes("verified_no_hit"), false, message);
}

const validSubject = await execute("subject-valid", "trace_subject_top_outflows", {
  holder_name: "测试主体",
  top_n: 10
});
assert.equal(validSubject.data.top_outflows[0].rank, 1);
assert.equal(validSubject.data.top_outflows[0].txn_count, 2);
assert.equal(validSubject.data.top_outflows[0].outflow_total, 1000);
assert.equal(validSubject.data.top_outflows[0].seed_txn.amount, 600);
assert.equal(validSubject.data.top_outflows[0].target_account_match_status, "unverified");
assert.equal(Object.hasOwn(validSubject.data.top_outflows[0], "target_account_in_case"), false);
assert.equal(validSubject.data.support_status, "partial");
assert.equal(validSubject.data.validation_summary.fact_answer_allowed, false);
assertNoVerifiedNoHit(validSubject, "bounded Top outflow rows must not become verified no-hit");

for (const mode of [
  "subject-invalid-rank",
  "subject-zero-rank",
  "subject-missing-count",
  "subject-missing-amount",
  "subject-zero-amount",
  "subject-missing-coverage"
]) {
  await assert.rejects(
    execute(mode, "trace_subject_top_outflows", { holder_name: "测试主体", top_n: 10 }),
    undefined,
    `${mode} must fail closed instead of projecting zero`
  );
}

const zeroSubject = await execute("subject-empty-zero", "trace_subject_top_outflows", {
  holder_name: "测试主体",
  top_n: 10
});
assert.equal(zeroSubject.data.scope_stats.txn_count, 0);
assert.equal(zeroSubject.data.scope_stats.outflow_total, 0);
assert.equal(zeroSubject.data.scope_stats.support_status, "unverified");
assert.equal(zeroSubject.data.support_status, "unverified");
assert.equal(zeroSubject.data.zero_result_status, "unresolved");
assert.equal(zeroSubject.data.validation_summary.coverage_complete, false);
assert.equal(zeroSubject.data.validation_summary.fact_answer_allowed, false);
assertNoVerifiedNoHit(zeroSubject, "explicit zero trace scope must remain boundary-only");

const validNextHop = await execute("next-valid", "trace_fund_next_hop", {
  seed_txn_id: "seed-next-1",
  amount_tolerance: 0.1
});
assert.equal(validNextHop.data.seed_txn.amount, 500);
assert.equal(validNextHop.data.next_hops[0].rank, 1);
assert.equal(validNextHop.data.next_hops[0].amount, 200);
assert.equal(validNextHop.data.support_status, "candidate_next_hop");
assert.equal(validNextHop.data.validation_summary.fact_answer_allowed, false);
assertNoVerifiedNoHit(validNextHop, "next-hop candidates must not become verified no-hit");

for (const mode of [
  "next-missing-seed-amount",
  "next-zero-seed-amount",
  "next-missing-hop-amount",
  "next-zero-hop-amount",
  "next-zero-hop-rank"
]) {
  await assert.rejects(
    execute(mode, "trace_fund_next_hop", { seed_txn_id: "seed-next-1" }),
    undefined,
    `${mode} must fail closed instead of projecting zero`
  );
}

const emptyNextHop = await execute("next-empty", "trace_fund_next_hop", {
  seed_txn_id: "seed-next-1"
});
assert.equal(emptyNextHop.data.next_hops.length, 0);
assert.equal(emptyNextHop.data.support_status, "unverified");
assert.equal(emptyNextHop.data.zero_result_status, "unresolved");
assert.equal(emptyNextHop.data.validation_summary.receiver_account_match_status, "unresolved");
assert.equal(Object.hasOwn(emptyNextHop.data.validation_summary, "receiver_account_matched_in_case"), false);
assertNoVerifiedNoHit(emptyNextHop, "empty next-hop result must remain a current-data boundary");

const traceFund = await execute("next-valid", "trace_fund", { seed_txn_id: "seed-next-1" });
assert.equal(traceFund.data.trace_paths[0].support_status, "candidate_next_hop");
assert.equal(traceFund.data.validation_summary.fact_answer_allowed, false);

await assert.rejects(
  execute("next-valid", "trace_fund_next_hop", { seed_txn_id: "seed-next-1", amount_tolerance: "invalid" }),
  undefined,
  "invalid trace tolerance must not become zero"
);

process.stdout.write(`${JSON.stringify({
  status: "ok",
  checks: [
    "subject Top outflow rank/count/amount/coverage fail closed",
    "explicit zero subject scope remains unverified and unresolved",
    "terminal account match zero is not projected as a verified false fact",
    "seed and next-hop amounts/ranks fail closed",
    "empty next-hop results remain boundary-only",
    "trace_fund preserves candidate-only support"
  ]
}, null, 2)}\n`);
