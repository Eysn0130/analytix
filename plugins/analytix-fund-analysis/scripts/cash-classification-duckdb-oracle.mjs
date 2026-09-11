#!/usr/bin/env node

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  compileLocalDuckdbRankingSql,
  runPythonDuckdb
} from "../mcp/duckdb-workbench-runtime.mjs";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const REPO_ROOT = path.resolve(SCRIPT_DIR, "../../..");
const FIXTURE_PATH = path.join(REPO_ROOT, "testdata", "cash-classification-vectors-v1.json");

function text(value) {
  return String(value == null ? "" : value).trim();
}

function pythonCandidates() {
  return [
    process.env.ANALYTIX_FUNDS_DUCKDB_PYTHON,
    process.env.ANALYTIX_BACKEND_PYTHON,
    process.env.PYTHON,
    path.join(REPO_ROOT, "backend", ".venv", process.platform === "win32" ? "Scripts/python.exe" : "bin/python"),
    path.join(REPO_ROOT, ".venv", process.platform === "win32" ? "Scripts/python.exe" : "bin/python"),
    "python3",
    "python"
  ].map(text).filter(Boolean);
}

function createRealDuckdbFixture(dbPath) {
  const script = String.raw`
import json
import os

import duckdb

with open(os.environ["ANALYTIX_CASH_VECTOR_FIXTURE"], "r", encoding="utf-8") as handle:
    fixture = json.load(handle)

db_path = os.environ["ANALYTIX_CASH_VECTOR_DB"]
os.makedirs(os.path.dirname(db_path), exist_ok=True)
con = duckdb.connect(db_path)
con.execute("""
CREATE TABLE analysis_txn_detail_idx (
    vector_id VARCHAR,
    id BIGINT,
    amount DOUBLE,
    dc_val VARCHAR,
    txn_time VARCHAR,
    acct_key VARCHAR,
    acct_no VARCHAR,
    card_no VARCHAR,
    account_open_name VARCHAR,
    opener_id_no VARCHAR,
    cp_key VARCHAR,
    counterparty_name VARCHAR,
    cp_name_pick VARCHAR,
    cp_name VARCHAR,
    stats_name_key VARCHAR,
    counterparty_acct VARCHAR,
    cp_raw VARCHAR,
    cash_flag VARCHAR,
    is_success VARCHAR,
    txn_day VARCHAR,
    file_id VARCHAR,
    summary VARCHAR,
    txn_type VARCHAR,
    remark VARCHAR,
    voucher_type VARCHAR
)
""")
for index, vector in enumerate(fixture["vectors"], start=1):
    vector_id = vector["id"]
    fields = vector["fields"]
    con.execute(
        "INSERT INTO analysis_txn_detail_idx VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
        [
            vector_id,
            index,
            0.0,
            "进",
            "2026-07-21 12:00:00",
            vector_id,
            "acct-" + str(index),
            "card-" + str(index),
            "holder-" + str(index),
            "id-" + str(index),
            "cp-" + str(index),
            "counterparty-" + str(index),
            "counterparty-" + str(index),
            "counterparty-" + str(index),
            "counterparty-" + str(index),
            "cp-acct-" + str(index),
            "cp-raw-" + str(index),
            vector["wire_token"],
            "true",
            "2026-07-21",
            "file-cash-vectors-v1",
            fields.get("summary"),
            fields.get("txn_type"),
            fields.get("remark"),
            fields.get("voucher_type"),
        ],
    )
con.close()
`;
  const failures = [];
  for (const python of [...new Set(pythonCandidates())]) {
    if (python.includes(path.sep) && !fs.existsSync(python)) continue;
    const result = spawnSync(python, ["-c", script], {
      env: {
        ...process.env,
        ANALYTIX_CASH_VECTOR_FIXTURE: FIXTURE_PATH,
        ANALYTIX_CASH_VECTOR_DB: dbPath
      },
      encoding: "utf8",
      maxBuffer: 2 * 1024 * 1024
    });
    if (result.status === 0) return python;
    failures.push(text(result.stderr || result.stdout || `${python} exited ${result.status}`));
  }
  throw new Error(`CashClassificationVectorsV1 DuckDB setup failed: ${failures.slice(-2).join(" | ")}`);
}

function scopeCoverageSql(sql, predicate) {
  const source = "FROM analysis_txn_detail_idx";
  assert.equal(sql.split(source).length, 2, "coverage SQL must have one authoritative source table");
  return sql.replace(source, `${source} WHERE ${predicate}`);
}

function strictAggregate(result) {
  assert.equal(result.row_count, 1);
  assert.equal(result.records.length, 1);
  assert.equal(result.truncated, false);
  return result.records[0];
}

const fixture = JSON.parse(fs.readFileSync(FIXTURE_PATH, "utf8"));
assert.equal(fixture.contract, "CashClassificationVectorsV1");
assert.equal(fixture.version, 1);
assert.equal(new Set(fixture.vectors.map((vector) => vector.id)).size, fixture.vectors.length);
assert.deepEqual(
  new Set(fixture.vectors.map((vector) => vector.expected)),
  new Set(["cash", "non_cash", "unknown", "conflict"])
);

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-cash-classification-v1-"));
try {
  const dbPath = path.join(tempRoot, "case.duckdb");
  const allSql = compileLocalDuckdbRankingSql("rank_accounts", {
    limit: 200,
    cash_filter: "all"
  });
  const cashSql = compileLocalDuckdbRankingSql("rank_accounts", {
    limit: 200,
    cash_filter: "cash_only"
  });
  const nonCashSql = compileLocalDuckdbRankingSql("rank_accounts", {
    limit: 200,
    cash_filter: "non_cash_only"
  });
  const realPython = createRealDuckdbFixture(dbPath);
  const realEnv = {
    ...process.env,
    ANALYTIX_FUNDS_DUCKDB_PYTHON: realPython,
    ANALYTIX_BACKEND_PYTHON: realPython,
    PYTHON: realPython
  };
  const executeRealSql = (sql, rowLimit = 500) => runPythonDuckdb(
    { db_path: dbPath, sql, row_limit: rowLimit },
    realEnv
  );

  const expectedCashIds = fixture.vectors
    .filter((vector) => vector.expected === "cash")
    .map((vector) => vector.id)
    .sort();
  const expectedNonCashIds = fixture.vectors
    .filter((vector) => vector.expected === "non_cash")
    .map((vector) => vector.id)
    .sort();
  const cashRows = await executeRealSql(cashSql.sql);
  const nonCashRows = await executeRealSql(nonCashSql.sql);
  assert.deepEqual(cashRows.records.map((record) => record.account_key).sort(), expectedCashIds);
  assert.deepEqual(nonCashRows.records.map((record) => record.account_key).sort(), expectedNonCashIds);

  for (const vector of fixture.vectors) {
    assert.match(vector.id, /^[a-z0-9_]+$/u);
    const record = strictAggregate(await executeRealSql(
      scopeCoverageSql(allSql.coverageSql, `vector_id = '${vector.id}'`),
      1
    ));
    assert.equal(record.requested_rows, 1, vector.id);
    assert.equal(
      record.cash_covered_rows,
      vector.expected === "cash" || vector.expected === "non_cash" ? 1 : 0,
      vector.id
    );
    assert.equal(record.cash_conflict_rows, vector.expected === "conflict" ? 1 : 0, vector.id);
  }

  const emptyRecord = strictAggregate(await executeRealSql(
    scopeCoverageSql(allSql.coverageSql, "FALSE"),
    1
  ));
  for (const [field, expected] of Object.entries(fixture.empty_dataset_expected_counts)) {
    assert.equal(emptyRecord[field], expected, `empty dataset ${field}`);
  }
  console.log(`CashClassificationVectorsV1 PASS (${fixture.vectors.length} vectors, real DuckDB)`);
} finally {
  fs.rmSync(tempRoot, { recursive: true, force: true });
}
