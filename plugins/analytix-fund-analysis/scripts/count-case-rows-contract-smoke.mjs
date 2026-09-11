#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  executeLocalDuckdbWorkbenchSkill,
  probeLocalDuckdbDatasetSnapshot,
} from "../mcp/duckdb-workbench-runtime.mjs";
import { isDuckdbDiagnosticError } from "../mcp/duckdb-diagnostic.mjs";
import { createAgentOutputCompiler } from "../mcp/agent-output-compiler.mjs";
import { tools } from "../mcp/tool-schemas.mjs";
import {
  hostFactRuntimeContext,
  writeCaseProjectBinding,
} from "./host-runtime-context-fixture.mjs";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const REPO_ROOT = path.resolve(SCRIPT_DIR, "../../..");

function text(value) {
  return String(value == null ? "" : value).trim();
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function toolByName(name) {
  return tools.find((tool) => tool?.name === name) || null;
}

function assertReadOnlyMcpTool(name) {
  const tool = toolByName(name);
  assert(tool, `${name} should be present in tool schemas`);
  assert(
    tool.annotations?.readOnlyHint === true,
    `${name} must stay readOnlyHint=true for approvalPolicy=never frontdoor access`,
  );
  assert(
    tool.annotations?.destructiveHint === false,
    `${name} must stay destructiveHint=false`,
  );
  assert(
    tool.annotations?.openWorldHint === false,
    `${name} must stay openWorldHint=false`,
  );
}

function pythonCandidates(env = process.env) {
  return [
    env.ANALYTIX_FUNDS_DUCKDB_PYTHON,
    env.ANALYTIX_BACKEND_PYTHON,
    env.PYTHON,
    path.join(
      REPO_ROOT,
      ".venv",
      process.platform === "win32" ? "Scripts/python.exe" : "bin/python",
    ),
    "python3",
    "python",
  ]
    .map(text)
    .filter(Boolean);
}

function runPython(script, env = process.env) {
  const errors = [];
  for (const python of [...new Set(pythonCandidates(env))]) {
    if (python.includes(path.sep) && !fs.existsSync(python)) continue;
    const result = spawnSync(python, ["-c", script], {
      env,
      encoding: "utf8",
      maxBuffer: 1024 * 1024,
    });
    if (result.status === 0) return result.stdout;
    errors.push(
      text(
        result.stderr || result.stdout || `${python} exited ${result.status}`,
      ),
    );
  }
  throw new Error(`DuckDB smoke setup failed: ${errors.slice(-2).join(" | ")}`);
}

function createCaseDb(dbPath) {
  const script = `
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

db_path = os.environ["ANALYTIX_SMOKE_CASE_DB"]
case_id = os.environ["ANALYTIX_SMOKE_CASE_ID"]
os.makedirs(os.path.dirname(db_path), exist_ok=True)
con = duckdb.connect(db_path)
con.execute("CREATE OR REPLACE TABLE analysis_txn_detail_idx AS SELECT range AS id, 1.0::DOUBLE AS amount, 1::BIGINT AS amount_source_present, 0::BIGINT AS amount_parse_failed, CASE WHEN range % 2 = 0 THEN '进' ELSE '出' END::TEXT AS dc_val FROM range(3)")
con.execute("CREATE OR REPLACE TABLE analysis_txn_daily_agg AS SELECT ('acct-' || CAST(range AS VARCHAR))::TEXT AS acct_key, DATE '2026-01-01' + CAST(range AS INTEGER) AS txn_day, ('cp-' || CAST(range AS VARCHAR))::TEXT AS cp_key, CASE WHEN range % 2 = 0 THEN '进' ELSE '出' END::TEXT AS dc_val, 1::BIGINT AS txn_count, 1::BIGINT AS amount_source_present_count, 1::BIGINT AS amount_valid_count, 0::BIGINT AS amount_missing_count, 0::BIGINT AS amount_parse_failed_count, 1.0::DOUBLE AS amt_sum FROM range(3)")
con.execute("CREATE TABLE analysis_txn_keyword_idx(txn_row_id BIGINT, kind TEXT, token TEXT, token_order BIGINT)")
con.execute("INSERT INTO analysis_txn_keyword_idx VALUES (0, 'summary', 'keyword', 0)")
con.execute("CREATE TABLE analysis_account_dim(account_key TEXT, label TEXT)")
con.execute("INSERT INTO analysis_account_dim VALUES ('acct-0', 'account')")
con.execute("CREATE OR REPLACE TABLE analysis_precision AS SELECT range AS id FROM range(3)")
con.execute("CREATE OR REPLACE TABLE fc_transaction_norm AS SELECT range AS id, ? AS case_id, NULL::TIMESTAMP AS txn_ts, 'file_txn_1' AS file_id, CASE WHEN range=3 THEN 1 ELSE 0 END::INTEGER AS clean_invalid, 0::INTEGER AS clean_failed, 0::INTEGER AS clean_reversal FROM range(4)", [case_id])
con.execute("CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT)")
con.execute("INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 7)")
con.execute("CREATE TABLE import_file_log(file_id TEXT, case_id TEXT, kind TEXT, sha256 TEXT, rows_imported_norm BIGINT, status TEXT, cleaned_status TEXT)")
con.execute("INSERT INTO import_file_log VALUES ('file_txn_1', ?, 'fc_transaction', ?, 4, '已完成', 'done')", [case_id, 'a' * 64])
identity_manifest = {
    "acceptedRowCount": 3,
    "caseId": case_id,
    "normalizedSourceSha256": normalized_source_signature(con, case_id),
    "rawSources": [{
        "cleanedStatus": "done",
        "fileId": "file_txn_1",
        "rowsImportedNorm": 4,
        "sha256": "a" * 64,
        "status": "已完成",
    }],
    "rejectedRowCount": 1,
    "schemaVersion": 2,
    "sourceMaxId": 3,
    "sourceMaxTxnTimestamp": "",
    "sourceRevision": 7,
    "sourceRowCount": 4,
}
source_signature = hashlib.sha256(json.dumps(identity_manifest, ensure_ascii=False, separators=(",", ":"), sort_keys=True).encode("utf-8")).hexdigest()
materialization_identity = "txn_daily_snapshot:v12:" + source_signature
result_signature = materialization_result_signature(con, case_id)
con.execute("CREATE TABLE analysis_materialization_meta(agg_name TEXT PRIMARY KEY, agg_version BIGINT, case_id TEXT, identity_schema_version BIGINT, source_revision BIGINT, source_row_count BIGINT, source_max_txn_ts TIMESTAMP, source_max_id BIGINT, source_signature TEXT, result_signature TEXT, row_count BIGINT)")
con.execute("INSERT INTO analysis_materialization_meta VALUES (?, 12, ?, 2, 7, 4, NULL, 3, ?, ?, 3)", [materialization_identity, case_id, source_signature, result_signature])
con.close()
`;
  runPython(script, {
    ...process.env,
    ANALYTIX_SMOKE_CASE_DB: dbPath,
    ANALYTIX_SMOKE_CASE_ID: "case_count_smoke",
  });
}

function mutateCaseDb(dbPath, sql) {
  const script = `
import duckdb
import os

con = duckdb.connect(os.environ["ANALYTIX_SMOKE_CASE_DB"])
try:
    con.execute(os.environ["ANALYTIX_SMOKE_MUTATION"])
finally:
    con.close()
`;
  runPython(script, {
    ...process.env,
    ANALYTIX_SMOKE_CASE_DB: dbPath,
    ANALYTIX_SMOKE_MUTATION: sql,
  });
}

async function assertSnapshotRejected(probe, code, message) {
  let rejected = null;
  try {
    await probe();
  } catch (error) {
    rejected = error;
  }
  assert(isDuckdbDiagnosticError(rejected) && rejected.code === code, message);
}

async function main() {
  const json = process.argv.includes("--json");
  assertReadOnlyMcpTool("get_current_case");
  assertReadOnlyMcpTool("count_case_rows");
  const tempRoot = fs.mkdtempSync(
    path.join(os.tmpdir(), "analytix-count-case-rows-"),
  );
  const caseId = "case_count_smoke";
  const projectRoot = path.join(tempRoot, "project");
  const casesRoot = path.join(tempRoot, "data-analysis", "cases");
  const caseRoot = path.join(casesRoot, caseId);
  const dbPath = path.join(caseRoot, "case.duckdb");
  const binding = writeCaseProjectBinding(projectRoot, caseId);
  createCaseDb(dbPath);

  const env = {
    ...process.env,
    ANALYTIX_CASE_PROJECT_ROOT: projectRoot,
    ANALYTIX_DATA_ANALYSIS_CASES_ROOT: casesRoot,
  };
  const direct = await executeLocalDuckdbWorkbenchSkill(
    "count_case_rows",
    {
      case_id: caseId,
      table_name: "analysis_txn_detail_idx",
    },
    { env },
  );
  const snapshotProbe = await probeLocalDuckdbDatasetSnapshot(
    { case_id: caseId },
    { env },
  );
  const normalized = await executeLocalDuckdbWorkbenchSkill(
    "count_case_rows",
    {
      case_id: caseId,
      table_name: "fc_transaction_norm",
    },
    { env },
  );
  const filtered = await executeLocalDuckdbWorkbenchSkill(
    "count_case_rows",
    {
      case_id: caseId,
      table_name: "fc_transaction_norm",
      where_sql: "id < 2",
    },
    { env },
  );
  const zeroFiltered = await executeLocalDuckdbWorkbenchSkill(
    "count_case_rows",
    {
      case_id: caseId,
      table_name: "fc_transaction_norm",
      where_sql: "id < 0",
    },
    { env },
  );
  const withRunCaseAuthority = (args) => ({
    ...args,
    _analytix: hostFactRuntimeContext({
      ...binding,
      caseId,
      toolName: "run_case_sql",
      args,
      overrides: {
        datasetSnapshotId: snapshotProbe.observed_dataset_snapshot_id,
      },
    }),
  });
  const limitTopNArgs = {
    case_id: caseId,
    purpose: "Top 2 smoke",
    sql: "SELECT id FROM analysis_precision ORDER BY id LIMIT 2",
    row_limit: 5,
    result_mode: "evidence_table",
  };
  const limitTopN = await executeLocalDuckdbWorkbenchSkill(
    "run_case_sql",
    withRunCaseAuthority(limitTopNArgs),
    { env },
  );
  const rowNumberTopNArgs = {
    case_id: caseId,
    purpose: "前 2 条 row_number smoke",
    sql: [
      "WITH ranked AS (",
      "  SELECT id, ROW_NUMBER() OVER (ORDER BY id) AS rn",
      "  FROM analysis_precision",
      ")",
      "SELECT id, rn FROM ranked WHERE rn <= 2 ORDER BY rn",
    ].join("\n"),
    row_limit: 5,
    result_mode: "evidence_table",
  };
  const rowNumberTopN = await executeLocalDuckdbWorkbenchSkill(
    "run_case_sql",
    withRunCaseAuthority(rowNumberTopNArgs),
    { env },
  );

  assert(
    direct.data.table_name === "analysis_txn_detail_idx",
    "direct analysis table count should keep analysis table",
  );
  assert(
    direct.data.row_count === 3,
    "direct analysis table count should read analysis_txn_detail_idx rows",
  );
  assert(
    /^dsv1_[a-f0-9]{64}$/u.test(direct.data.observed_dataset_snapshot_id),
    "exact table count must observe the dataset snapshot in the same read transaction",
  );
  assert(
    direct.data.snapshot_contract === "analytix_duckdb_dataset_snapshot_v1",
    "exact table count must expose the snapshot contract",
  );
  assert(
    snapshotProbe.observed_dataset_snapshot_id ===
      direct.data.observed_dataset_snapshot_id,
    "source probe and count must bind the same observed snapshot contract",
  );
  mutateCaseDb(
    dbPath,
    "UPDATE analysis_materialization_meta SET agg_version=11 WHERE starts_with(agg_name, 'txn_daily_')",
  );
  await assertSnapshotRejected(
    () => probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env }),
    "dataset_snapshot_mismatch",
    "a v11 materialization must not satisfy the v12 evidence-lineage contract",
  );
  mutateCaseDb(
    dbPath,
    "UPDATE analysis_materialization_meta SET agg_version=12 WHERE starts_with(agg_name, 'txn_daily_')",
  );
  mutateCaseDb(
    dbPath,
    "INSERT INTO analysis_materialization_meta(agg_name, agg_version, source_revision) VALUES ('txn_daily_' || 'active:v8', 8, 7)",
  );
  await assertSnapshotRejected(
    () => probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env }),
    "dataset_snapshot_mismatch",
    "legacy factual materialization metadata must fail closed",
  );
  mutateCaseDb(
    dbPath,
    "DELETE FROM analysis_materialization_meta WHERE agg_version=8",
  );
  mutateCaseDb(
    dbPath,
    `UPDATE import_file_log SET sha256='${"b".repeat(64)}' WHERE file_id='file_txn_1'`,
  );
  await assertSnapshotRejected(
    () => probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env }),
    "dataset_snapshot_mismatch",
    "a raw-source identity mutation must invalidate the materialization identity",
  );
  mutateCaseDb(
    dbPath,
    `UPDATE import_file_log SET sha256='${"a".repeat(64)}' WHERE file_id='file_txn_1'`,
  );
  mutateCaseDb(
    dbPath,
    "UPDATE analysis_materialization_meta SET source_revision=6 WHERE starts_with(agg_name, 'txn_daily_')",
  );
  await assertSnapshotRejected(
    () => probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env }),
    "dataset_snapshot_mismatch",
    "stale materialization revision must block the dataset snapshot",
  );
  mutateCaseDb(
    dbPath,
    "UPDATE analysis_materialization_meta SET source_revision=7 WHERE starts_with(agg_name, 'txn_daily_')",
  );
  mutateCaseDb(
    dbPath,
    "UPDATE analysis_materialization_meta SET source_row_count=3 WHERE starts_with(agg_name, 'txn_daily_')",
  );
  await assertSnapshotRejected(
    () => probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env }),
    "dataset_snapshot_mismatch",
    "stale materialization source count must block the dataset snapshot",
  );
  mutateCaseDb(
    dbPath,
    "UPDATE analysis_materialization_meta SET source_row_count=4 WHERE starts_with(agg_name, 'txn_daily_')",
  );
  mutateCaseDb(dbPath, "DELETE FROM analysis_txn_detail_idx WHERE id=2");
  await assertSnapshotRejected(
    () => probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env }),
    "dataset_snapshot_mismatch",
    "source/detail row-count mismatch must block the dataset snapshot",
  );
  mutateCaseDb(dbPath, "INSERT INTO analysis_txn_detail_idx VALUES (2, 1.0, 1, 0, '进')");
  mutateCaseDb(
    dbPath,
    "UPDATE fc_transaction_norm SET clean_invalid=0 WHERE id=3",
  );
  await assertSnapshotRejected(
    () => probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env }),
    "dataset_snapshot_mismatch",
    "a formerly rejected row becoming accepted without index rematerialization must block the dataset snapshot",
  );
  mutateCaseDb(
    dbPath,
    "UPDATE fc_transaction_norm SET clean_invalid=1 WHERE id=3",
  );
  mutateCaseDb(
    dbPath,
    "UPDATE fc_transaction_norm SET clean_invalid=0, clean_failed=1 WHERE id=3",
  );
  await assertSnapshotRejected(
    () => probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env }),
    "dataset_snapshot_mismatch",
    "moving a rejection flag while preserving the rejected count must invalidate normalized source identity",
  );
  mutateCaseDb(
    dbPath,
    "UPDATE fc_transaction_norm SET clean_invalid=1, clean_failed=0 WHERE id=3",
  );
  mutateCaseDb(
    dbPath,
    "UPDATE import_file_log SET sha256='not-a-sha256' WHERE file_id='file_txn_1'",
  );
  await assertSnapshotRejected(
    () => probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env }),
    "dataset_snapshot_mismatch",
    "invalid raw-source SHA-256 must block the dataset snapshot",
  );
  mutateCaseDb(
    dbPath,
    `UPDATE import_file_log SET sha256='${"a".repeat(64)}' WHERE file_id='file_txn_1'`,
  );
  const restoredSnapshotProbe = await probeLocalDuckdbDatasetSnapshot(
    { case_id: caseId },
    { env },
  );
  assert(
    restoredSnapshotProbe.observed_dataset_snapshot_id ===
      snapshotProbe.observed_dataset_snapshot_id,
    "restored source authority must reproduce the deterministic dataset snapshot id",
  );
  assert(
    normalized.data.table_name === "analysis_txn_detail_idx",
    "unfiltered fc_transaction_norm total should normalize to analysis_txn_detail_idx",
  );
  assert(
    normalized.data.requested_table_name === "fc_transaction_norm",
    "normalized count should retain requested table name",
  );
  assert(
    normalized.data.table_name_normalization?.applied === true,
    "normalized count should carry normalization audit metadata",
  );
  assert(
    normalized.data.row_count === 3,
    "normalized total should use analysis_txn_detail_idx row count",
  );
  const compiler = createAgentOutputCompiler({ env });
  const normalizedAgentText = compiler.compactToolText(normalized);
  const normalizedStructured = compiler.compactStructuredContent({
    ...normalized,
    data: {
      ...normalized.data,
      answer_card_complete: true,
      fact_answer_allowed: true,
      safe_to_answer: true,
      verified_no_hit_allowed: true,
    },
  });
  for (const [label, unsafeCount] of [
    ["missing", undefined],
    ["null", null],
    ["blank", ""],
    ["boolean", false],
    ["object", {}],
  ]) {
    const unsafeData = {
      ...normalized.data,
      row_count: unsafeCount,
      returned_count: undefined,
      records: undefined,
      rows: undefined,
      result_rows: undefined,
      resultRows: undefined,
      rankings: undefined,
      summary: {
        ...normalized.data.summary,
        returned_count: undefined,
      },
    };
    const unsafeText = compiler.compactToolText({
      ...normalized,
      data: unsafeData,
    });
    assert(
      !unsafeText.includes("精确记录数为 0 条") &&
        !unsafeText.includes("原始整数 0"),
      `${label} count must not be compiled into a verified zero fact`,
    );
  }
  assert(
    normalizedAgentText.includes("精确计数答复约束"),
    "count_case_rows agent text should expose an exact-count answer contract",
  );
  assert(
    normalizedAgentText.includes("精确记录数为 3 条"),
    "count_case_rows agent text should preserve the exact integer count",
  );
  assert(
    normalizedAgentText.includes("不得四舍五入") &&
      normalizedAgentText.includes("不得改写为"),
    "count_case_rows agent text should forbid approximate count answers",
  );
  assert(
    normalizedAgentText.includes("宿主证据门通过前不得作为案件事实或直接答复"),
    "exact count must remain boundary-only before the host evidence gate",
  );
  assert(
    !normalizedAgentText.includes("最终回答必须写精确整数"),
    "caller exact-count wording must not instruct an ungated final answer",
  );
  assert(
    normalizedStructured.validation_state?.fact_answer_allowed === false,
    "caller fact_answer_allowed must be ignored",
  );
  assert(
    normalizedStructured.validation_state?.verified_no_hit_allowed === false,
    "caller verified_no_hit_allowed must be ignored",
  );
  assert(
    normalizedStructured.validation_state?.coverage_complete === false,
    "exact count must not imply complete case coverage",
  );
  assert(
    normalizedStructured.validation_state?.answer_card_complete === false,
    "exact count must not complete the answer card",
  );
  assert(
    normalizedStructured.validation_state?.exact_count_contract_status ===
      "valid_boundary_only",
    "complete exact-count structure must remain boundary-only",
  );
  assert(
    normalizedStructured.compact_evidence?.workbench?.row_count === 3,
    "boundary projection must preserve the exact nonzero integer",
  );
  assert(
    normalizedStructured.compact_evidence?.workbench?.fact_answer_allowed ===
      false,
    "workbench projection must block fact answering",
  );
  assert(
    filtered.data.table_name === "fc_transaction_norm",
    "filtered fc_transaction_norm count should keep requested table",
  );
  assert(
    filtered.data.table_name_normalization?.applied === false,
    "filtered fc_transaction_norm count should not normalize",
  );
  assert(
    filtered.data.row_count === 2,
    "filtered fc_transaction_norm count should apply the requested where_sql",
  );
  assert(
    zeroFiltered.data.row_count === 0,
    "zero filter should preserve an explicit zero result",
  );
  assert(
    zeroFiltered.data.zero_result_status === "unresolved",
    "legacy or unbound zero results must remain unresolved without a factual snapshot",
  );
  const zeroAgentText = compiler.compactToolText(zeroFiltered);
  const zeroStructured = compiler.compactStructuredContent(zeroFiltered);
  assert(
    zeroAgentText.includes("显式返回 0 条（未验证未命中）"),
    "zero count must be visible only as an unverified no-hit",
  );
  assert(
    zeroAgentText.includes("空结果不等于全案为零、不存在、无关联或无异常"),
    "zero count must retain the empty-result boundary",
  );
  assert(
    !zeroAgentText.includes("精确记录数为 0 条"),
    "zero count must not use the nonzero exact-fact wording",
  );
  assert(
    zeroStructured.compact_evidence?.workbench?.row_count === 0,
    "explicit zero must survive projection",
  );
  assert(
    zeroStructured.validation_state?.verified_no_hit_allowed === false,
    "caller verified_no_hit must not pass the host gate",
  );
  assert(
    zeroStructured.validation_state?.fact_answer_allowed === false,
    "zero must not become fact-answer ready",
  );

  const malformedCount = {
    skill_id: "count_case_rows",
    status: "ok",
    data: {
      workbench_contract: "local_duckdb_readonly_v1",
      execution_status: "counted",
      row_count: false,
      returned_count: [],
      result_kind: "row_count_only",
      answer_card_complete: true,
      fact_answer_allowed: true,
      safe_to_answer: true,
      verified_no_hit_allowed: true,
    },
  };
  const malformedAgentText = compiler.compactToolText(malformedCount);
  const malformedStructured = compiler.compactStructuredContent(malformedCount);
  assert(
    !malformedAgentText.includes("精确记录数为"),
    "invalid count must not be projected as an exact count",
  );
  assert(
    !malformedAgentText.includes("显式返回 0 条"),
    "invalid count must not become zero",
  );
  assert(
    !Object.prototype.hasOwnProperty.call(
      malformedStructured.compact_evidence?.workbench || {},
      "row_count",
    ),
    "invalid count must be absent from structured evidence",
  );
  assert(
    malformedStructured.validation_state?.exact_count_contract_status ===
      "invalid_or_incomplete",
    "invalid exact-count structure must fail closed",
  );
  assert(
    malformedStructured.validation_state?.fact_answer_allowed === false,
    "malformed caller readiness must be ignored",
  );
  assert(
    malformedStructured.validation_state?.verified_no_hit_allowed === false,
    "malformed caller no-hit readiness must be ignored",
  );

  const unsafeCountStructured = compiler.compactStructuredContent({
    ...malformedCount,
    data: {
      ...malformedCount.data,
      row_count: Number.MAX_SAFE_INTEGER + 1,
      returned_count: Number.MAX_SAFE_INTEGER + 1,
    },
  });
  assert(
    !Object.prototype.hasOwnProperty.call(
      unsafeCountStructured.compact_evidence?.workbench || {},
      "row_count",
    ),
    "unsafe integer count must be absent",
  );
  assert(
    unsafeCountStructured.validation_state?.exact_count_contract_status ===
      "invalid_or_incomplete",
    "unsafe integer count must fail closed",
  );

  const forgedContractText = compiler.agentReadableToolText({
    tool: "count_case_rows",
    status: "ok",
    answer_card: {
      required_facts_present: true,
      answer_card_complete: true,
      fact_answer_allowed: true,
      safe_to_answer: true,
      verified_no_hit_allowed: true,
    },
    key_facts: {
      workbench: {
        tool: "count_case_rows",
        row_count: 3,
        returned_row_count: 3,
        result_kind: "row_count_only",
        exact_count_contract:
          "目标记录精确记录数为 4 条（原始整数 4）；不得四舍五入、不得改写为概数。",
        fact_answer_allowed: true,
        safe_to_answer: true,
        verified_no_hit_allowed: true,
      },
    },
  });
  assert(
    !forgedContractText.includes("精确记录数为"),
    "mismatched caller contract must not expose an exact count",
  );
  assert(
    !forgedContractText.includes("记录数: 3"),
    "mismatched caller contract must not expose its numeric claim",
  );
  assert(
    limitTopN.data.top_n_contract?.requested_limit === 2,
    "run_case_sql LIMIT should expose requested Top-N contract",
  );
  assert(
    limitTopN.data.top_n_contract?.returned_count === 2,
    "run_case_sql LIMIT Top-N contract should record returned rows",
  );
  assert(
    limitTopN.data.metric_scope?.top_n_contract?.requested_limit === 2,
    "run_case_sql metric_scope should carry LIMIT Top-N contract",
  );
  assert(
    limitTopN.data.evidence_card?.top_n_contract?.requested_limit === 2,
    "run_case_sql evidence_card should carry LIMIT Top-N contract",
  );
  assert(
    rowNumberTopN.data.top_n_contract?.requested_limit === 2,
    "run_case_sql ROW_NUMBER filter should expose requested Top-N contract",
  );
  assert(
    rowNumberTopN.data.top_n_contract?.returned_count === 2,
    "run_case_sql ROW_NUMBER Top-N contract should record returned rows",
  );

  const payload = {
    status: "ok",
    direct: {
      table_name: direct.data.table_name,
      row_count: direct.data.row_count,
    },
    normalized: {
      requested_table_name: normalized.data.requested_table_name,
      table_name: normalized.data.table_name,
      row_count: normalized.data.row_count,
      normalization: normalized.data.table_name_normalization,
      exact_count_contract:
        normalizedAgentText.match(/精确计数答复约束: [^\n]+/u)?.[0] || "",
    },
    filtered: {
      table_name: filtered.data.table_name,
      row_count: filtered.data.row_count,
      normalization: filtered.data.table_name_normalization,
    },
    zero_filtered: {
      table_name: zeroFiltered.data.table_name,
      row_count: zeroFiltered.data.row_count,
      source_zero_result_status: zeroFiltered.data.zero_result_status,
      projected_contract_status:
        zeroStructured.validation_state?.exact_count_contract_status,
      fact_answer_allowed: zeroStructured.validation_state?.fact_answer_allowed,
      verified_no_hit_allowed:
        zeroStructured.validation_state?.verified_no_hit_allowed,
    },
    run_case_sql_top_n: {
      limit_contract: limitTopN.data.top_n_contract,
      row_number_contract: rowNumberTopN.data.top_n_contract,
    },
    read_only_tool_contract: {
      get_current_case: toolByName("get_current_case")?.annotations,
      count_case_rows: toolByName("count_case_rows")?.annotations,
    },
  };
  if (json) console.log(JSON.stringify(payload, null, 2));
  else console.log("count-case-rows contract smoke ok");
}

main().catch((error) => {
  console.error(error?.stack || error?.message || error);
  process.exit(1);
});
