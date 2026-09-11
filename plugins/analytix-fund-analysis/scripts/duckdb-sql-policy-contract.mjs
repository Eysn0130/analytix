#!/usr/bin/env node

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import {
  executeLocalDuckdbWorkbenchSkill,
  probeLocalDuckdbDatasetSnapshot
} from "../mcp/duckdb-workbench-runtime.mjs";
import { analyzeDuckdbReadOnlySql } from "../mcp/duckdb-sql-policy.mjs";
import {
  hostFactRuntimeContext,
  writeCaseProjectBinding
} from "./host-runtime-context-fixture.mjs";

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-duckdb-policy-"));
process.on("exit", () => fs.rmSync(tempRoot, { recursive: true, force: true }));

const projectRoot = path.join(tempRoot, "case-project");
const casesRoot = path.join(tempRoot, "cases");
const caseId = "case_duckdb_policy_contract";
const dbPath = path.join(casesRoot, caseId, "case.duckdb");
fs.mkdirSync(path.dirname(dbPath), { recursive: true });
const binding = writeCaseProjectBinding(projectRoot, caseId);

const setup = spawnSync("python3", ["-c", String.raw`
import duckdb
import hashlib
import json
import os

def update_framed(digest, value):
    encoded = value.encode("utf-8")
    digest.update(len(encoded).to_bytes(8, "big"))
    digest.update(encoded)

def columns(con, table):
    return [
        str(row[0])
        for row in con.execute(
            "SELECT column_name FROM information_schema.columns WHERE table_schema='main' AND table_name=? ORDER BY ordinal_position",
            [table],
        ).fetchall()
    ]

def update_rows(con, digest, sql, params=None):
    cursor = con.execute(sql, params or [])
    while True:
        rows = cursor.fetchmany(2048)
        if not rows:
            break
        for row in rows:
            update_framed(digest, str(row[0]))

def normalized_source_signature(con, case_id):
    digest = hashlib.sha256()
    update_framed(digest, "analytix.txn-daily-normalized-source/v12")
    update_framed(digest, case_id)
    for column in columns(con, "fc_transaction_norm"):
        update_framed(digest, column)
    update_rows(
        con,
        digest,
        "SELECT to_json(r) FROM fc_transaction_norm r WHERE r.case_id=? ORDER BY TRY_CAST(r.id AS BIGINT), to_json(r)",
        [case_id],
    )
    return digest.hexdigest()

def materialization_result_signature(con, case_id):
    digest = hashlib.sha256()
    update_framed(digest, "analytix.txn-daily-result/v12")
    update_framed(digest, case_id)
    for table, order_by in (
        ("analysis_txn_daily_agg", "r.acct_key, r.txn_day, r.cp_key, r.dc_val"),
        ("analysis_txn_detail_idx", "TRY_CAST(r.id AS BIGINT), r.id"),
        ("analysis_txn_keyword_idx", "TRY_CAST(r.txn_row_id AS BIGINT), r.txn_row_id, r.kind, r.token, r.token_order"),
        ("analysis_account_dim", "r.account_key"),
    ):
        update_framed(digest, table)
        for column in columns(con, table):
            update_framed(digest, column)
        update_rows(con, digest, "SELECT to_json(r) FROM " + table + " r ORDER BY " + order_by + ", to_json(r)")
    return digest.hexdigest()

db_path = os.environ["ANALYTIX_POLICY_DB"]
case_id = os.environ["ANALYTIX_POLICY_CASE"]
con = duckdb.connect(db_path)
con.execute("CREATE TABLE analysis_txn_detail_idx(id BIGINT, amount DOUBLE, amount_source_present BIGINT, amount_parse_failed BIGINT, dc_val TEXT, integrity_marker TEXT)")
con.execute("INSERT INTO analysis_txn_detail_idx VALUES (1, 1.0, 1, 0, '进', 'baseline')")
con.execute("CREATE TABLE analysis_txn_daily_agg(acct_key TEXT, txn_day DATE, cp_key TEXT, dc_val TEXT, txn_count BIGINT, amount_source_present_count BIGINT, amount_valid_count BIGINT, amount_missing_count BIGINT, amount_parse_failed_count BIGINT, amt_sum DOUBLE, integrity_marker TEXT)")
con.execute("INSERT INTO analysis_txn_daily_agg VALUES ('acct-1', DATE '2026-01-01', 'cp-1', '进', 1, 1, 1, 0, 0, 1.0, 'baseline')")
con.execute("CREATE TABLE analysis_txn_keyword_idx(txn_row_id BIGINT, kind TEXT, token TEXT, token_order BIGINT, integrity_marker TEXT)")
con.execute("INSERT INTO analysis_txn_keyword_idx VALUES (1, 'summary', 'keyword', 0, 'baseline')")
con.execute("CREATE TABLE analysis_account_dim(account_key TEXT, integrity_marker TEXT)")
con.execute("INSERT INTO analysis_account_dim VALUES ('acct-1', 'baseline')")
con.execute("CREATE TABLE analysis_payload(id BIGINT, payload TEXT)")
con.execute("INSERT INTO analysis_payload VALUES (1, repeat('x', 9 * 1024 * 1024))")
con.execute("CREATE TABLE analysis_precision(account_no VARCHAR, amount DECIMAL(38,2), amount_minor HUGEINT)")
con.execute("INSERT INTO analysis_precision VALUES ('6222021234567890123', 123456789012345.67, 12345678901234567)")
con.execute("INSERT INTO analysis_precision VALUES ('0000123456789012345', 0.01, 1)")
con.execute("CREATE TABLE analysis_numeric_identifier(account_no BIGINT)")
con.execute("INSERT INTO analysis_numeric_identifier VALUES (6222021234567890123)")
con.execute("CREATE TABLE fc_transaction_norm(id BIGINT, case_id TEXT, txn_ts TIMESTAMP, file_id TEXT, clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER)")
con.execute("INSERT INTO fc_transaction_norm VALUES (1, ?, NULL, 'file_policy_1', 0, 0, 0)", [case_id])
con.execute("CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT)")
con.execute("INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 11)")
con.execute("CREATE TABLE import_file_log(file_id TEXT, case_id TEXT, kind TEXT, sha256 TEXT, rows_imported_norm BIGINT, status TEXT, cleaned_status TEXT)")
con.execute("INSERT INTO import_file_log VALUES ('file_policy_1', ?, 'fc_transaction', ?, 1, '已完成', 'done')", [case_id, 'a' * 64])
identity_manifest = {
    "acceptedRowCount": 1,
    "caseId": case_id,
    "normalizedSourceSha256": normalized_source_signature(con, case_id),
    "rawSources": [{
        "cleanedStatus": "done",
        "fileId": "file_policy_1",
        "rowsImportedNorm": 1,
        "sha256": "a" * 64,
        "status": "已完成",
    }],
    "rejectedRowCount": 0,
    "schemaVersion": 2,
    "sourceMaxId": 1,
    "sourceMaxTxnTimestamp": "",
    "sourceRevision": 11,
    "sourceRowCount": 1,
}
canonical = json.dumps(identity_manifest, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
source_signature = hashlib.sha256(canonical.encode("utf-8")).hexdigest()
materialization_identity = "txn_daily_snapshot:v12:" + source_signature
result_signature = materialization_result_signature(con, case_id)
con.execute("CREATE TABLE analysis_materialization_meta(agg_name TEXT PRIMARY KEY, agg_version BIGINT, case_id TEXT, identity_schema_version BIGINT, source_revision BIGINT, source_row_count BIGINT, source_max_txn_ts TIMESTAMP, source_max_id BIGINT, source_signature TEXT, result_signature TEXT, row_count BIGINT)")
con.execute("INSERT INTO analysis_materialization_meta VALUES (?, 12, ?, 2, 11, 1, NULL, 1, ?, ?, 1)", [materialization_identity, case_id, source_signature, result_signature])
con.execute("CREATE VIEW analysis_external AS SELECT file FROM glob('/etc/*')")
con.execute("CREATE MACRO lower(value) AS (SELECT file FROM glob('/etc/*') LIMIT 1)")
con.close()
`], {
  env: { ...process.env, ANALYTIX_POLICY_DB: dbPath, ANALYTIX_POLICY_CASE: caseId },
  encoding: "utf8",
  maxBuffer: 1024 * 1024
});
assert.equal(setup.status, 0, setup.stderr || setup.stdout);

const env = {
  ...process.env,
  ANALYTIX_CASE_PROJECT_ROOT: projectRoot,
  ANALYTIX_DATA_ANALYSIS_CASES_ROOT: casesRoot
};
const snapshot = await probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env });

function authorityFor(args) {
  return hostFactRuntimeContext({
    ...binding,
    caseId,
    toolName: "run_case_sql",
    args,
    overrides: { datasetSnapshotId: snapshot.observed_dataset_snapshot_id }
  });
}

async function runCaseSql(args) {
  return executeLocalDuckdbWorkbenchSkill("run_case_sql", {
    case_id: caseId,
    ...args,
    _analytix: authorityFor(args)
  }, { env });
}

async function rejectsDuckdbCode(promise, expectedCode) {
  await assert.rejects(promise, (error) => {
    assert.equal(error?.duckdbDiagnostic?.code, expectedCode, error?.message || String(error));
    assert.equal(error?.duckdbDiagnostic?.retryable, false);
    const serialized = JSON.stringify({ message: error?.message, diagnostic: error?.duckdbDiagnostic });
    for (const forbidden of ["6222021234567890123", dbPath, "analysis_secret_accounts", "reasoning_content"]) {
      assert.ok(!serialized.includes(forbidden), `typed DuckDB failure leaked ${forbidden}: ${serialized}`);
    }
    return true;
  });
}

function mutatePolicyDb(sql) {
  const mutation = spawnSync("python3", ["-c", String.raw`
import duckdb
import os

con = duckdb.connect(os.environ["ANALYTIX_POLICY_DB"])
try:
    con.execute(os.environ["ANALYTIX_POLICY_MUTATION"])
finally:
    con.close()
`], {
    env: {
      ...process.env,
      ANALYTIX_POLICY_DB: dbPath,
      ANALYTIX_POLICY_MUTATION: sql
    },
    encoding: "utf8",
    maxBuffer: 1024 * 1024
  });
  assert.equal(mutation.status, 0, mutation.stderr || mutation.stdout);
}

const safe = await runCaseSql({
  purpose: "policy safe control",
  sql: "SELECT account_no FROM analysis_precision ORDER BY account_no LIMIT 1",
  row_limit: 1
});
assert.equal(safe.data.records[0].account_no, "0000123456789012345");
assert.equal(safe.data.observed_dataset_snapshot_id, snapshot.observed_dataset_snapshot_id);
assert.equal(safe.data.sql_policy.static_guard.policy_engine, "tokenized_duckdb_readonly_scope_guard_v3");

const intrinsicFunctions = await runCaseSql({
  purpose: "safe parser intrinsic and internal macro control",
  sql: "SELECT COALESCE(NULLIF(account_no, ''), 'missing') AS account_label FROM analysis_precision ORDER BY account_no LIMIT 1",
  row_limit: 1
});
assert.equal(intrinsicFunctions.data.records[0].account_label, "0000123456789012345");

const identifierAggregate = await runCaseSql({
  purpose: "identifier aggregate names are not identifiers",
  sql: "SELECT COUNT(*) AS account_count FROM analysis_numeric_identifier",
  row_limit: 1
});
assert.equal(identifierAggregate.data.records[0].account_count, 1);

const scalarSubquery = await runCaseSql({
  purpose: "SQL grammar before scalar subqueries is not a function",
  sql: "SELECT (SELECT COUNT(*) FROM analysis_precision) AS row_count FROM analysis_precision LIMIT 1",
  row_limit: 1
});
assert.equal(scalarSubquery.data.records[0].row_count, 2);

const precise = await runCaseSql({
  purpose: "canonical account and money precision",
  sql: "SELECT account_no, amount, amount_minor FROM analysis_precision ORDER BY account_no",
  row_limit: 2
});
assert.deepEqual(precise.data.records, [
  { account_no: "0000123456789012345", amount: "0.01", amount_minor: 1 },
  { account_no: "6222021234567890123", amount: "123456789012345.67", amount_minor: "12345678901234567" }
]);
assert.deepEqual(precise.data.column_types, ["VARCHAR", "DECIMAL(38,2)", "HUGEINT"]);

await rejectsDuckdbCode(
  runCaseSql({
    purpose: "numeric account identifiers must not be coerced",
    sql: "SELECT account_no FROM analysis_numeric_identifier LIMIT 1"
  }),
  "result_contract_invalid"
);

await rejectsDuckdbCode(
  runCaseSql({
    purpose: "comment literal external access attack",
    sql: "SELECT '/*' AS marker, (SELECT file FROM glob('/etc/*') LIMIT 1) AS leaked_name, '*/' AS closer FROM analysis_precision LIMIT 1"
  }),
  "sql_policy_rejected"
);

for (const functionName of ["glob", "read_blob", "read_csv", "parquet_scan", "extension_scan"]) {
  assert.throws(
    () => analyzeDuckdbReadOnlySql(`SELECT * FROM ${functionName}('/etc/*'), analysis_precision`),
    /table function|not allowlisted/iu
  );
}

await rejectsDuckdbCode(
  runCaseSql({
    purpose: "persistent macro must not shadow builtin authority",
    sql: "SELECT lower('safe') AS value FROM analysis_precision LIMIT 1"
  }),
  "sql_policy_rejected"
);

await rejectsDuckdbCode(
  runCaseSql({
    purpose: "allowed-name view must not regain external access",
    sql: "SELECT file FROM analysis_external LIMIT 1"
  }),
  "sql_bind_rejected"
);

await rejectsDuckdbCode(
  runCaseSql({
    purpose: "bounded rows still require bounded bytes",
    sql: "SELECT payload FROM analysis_payload LIMIT 1",
    row_limit: 1
  }),
  "resource_limit_exceeded"
);

for (const sql of [
  "SELECT SUM(amount) AS amount FROM analysis_txn_detail_idx",
  "SELECT SUM(amt_sum) AS amount FROM analysis_txn_daily_agg",
  "SELECT id FROM analysis_txn_detail_idx LIMIT 1"
]) {
  await rejectsDuckdbCode(
    runCaseSql({ purpose: "unscoped amount fact table must be blocked", sql }),
    "amount_coverage_incomplete"
  );
}

for (const tableName of [
  "analysis_txn_daily_agg",
  "analysis_txn_detail_idx",
  "analysis_txn_keyword_idx",
  "analysis_account_dim"
]) {
  mutatePolicyDb(`UPDATE ${tableName} SET integrity_marker='tampered'`);
  await rejectsDuckdbCode(
    runCaseSql({
      purpose: `same-count ${tableName} tamper must fail before execution`,
      sql: "SELECT account_no FROM analysis_precision LIMIT 1"
    }),
    "dataset_snapshot_mismatch"
  );
  mutatePolicyDb(`UPDATE ${tableName} SET integrity_marker='baseline'`);
}

const mutation = spawnSync("python3", ["-c", String.raw`
import duckdb
import os
con = duckdb.connect(os.environ["ANALYTIX_POLICY_DB"])
con.execute("UPDATE import_file_log SET sha256=? WHERE file_id='file_policy_1'", ['b' * 64])
con.close()
`], {
  env: { ...process.env, ANALYTIX_POLICY_DB: dbPath },
  encoding: "utf8",
  maxBuffer: 1024 * 1024
});
assert.equal(mutation.status, 0, mutation.stderr || mutation.stdout);

await rejectsDuckdbCode(
  runCaseSql({
    purpose: "stale frozen snapshot must fail before execution",
    sql: "SELECT account_no FROM analysis_precision LIMIT 1"
  }),
  "dataset_snapshot_mismatch"
);

console.log("DuckdbSqlCommentLiteralCannotHideExternalAccess: passed");
console.log("DuckdbSqlFunctionAllowlistRejectsGlobReadBlobAndExtensionScan: passed");
console.log("DuckdbSqlResultByteLimit: passed");
console.log("DuckdbSqlExecutionRequiresFrozenSnapshot: passed");
console.log("DuckdbSqlFrozenSnapshotRejectsFourTableSameCountTamper: passed");
console.log("DuckdbCanonicalAccountAndMoneyPreserveExactBytes: passed");
console.log("DuckdbNumericAccountIdentifierFailsClosed: passed");
console.log("DuckdbUnscopedAmountFactSqlRequiresHostCoverageExecutor: passed");
