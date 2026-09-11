#!/usr/bin/env node

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { probeLocalFundsProducerContent } from "../mcp/duckdb-workbench-runtime.mjs";
import { isDuckdbDiagnosticError } from "../mcp/duckdb-diagnostic.mjs";
import { writeCaseProjectBinding } from "./host-runtime-context-fixture.mjs";

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(SCRIPT_DIR, "../../..");
const CASE_ID = "case-fpc1-golden";
const EXPECTED_PRODUCER_CONTENT_ID = "fpc1_97231a8c6e4830c4183f29e76bb1d4bad8f5f2a48f307284b167ce0b1e2d77a0";
const EXPECTED_PRODUCER_MANIFEST_SHA256 = "ffc9fa3f37d6035fbc3bf265a3c3a6c8fcaf97b63d153d3ced4ff6775cd04fd1";
const RAW_ARTIFACT_MANIFEST_SHA256 = "1".repeat(64);
const BACKEND_PYTHON = process.env.ANALYTIX_BACKEND_PYTHON
  || path.join(REPO_ROOT, "backend", ".venv", process.platform === "win32" ? "Scripts/python.exe" : "bin/python");

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd || REPO_ROOT,
    env: { ...process.env, ...options.env },
    input: options.input,
    encoding: "utf8",
    maxBuffer: 16 * 1024 * 1024
  });
  if (result.status !== 0) {
    throw new Error(`${command} failed (${result.status}): ${String(result.stderr || result.stdout).trim()}`);
  }
  return String(result.stdout || "").trim();
}

function sha256File(filePath) {
  return crypto.createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

if (!fs.existsSync(BACKEND_PYTHON)) {
  throw new Error(`Formal backend Python is unavailable: ${BACKEND_PYTHON}`);
}

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-fpc1-cross-language-"));
process.on("exit", () => fs.rmSync(tempRoot, { recursive: true, force: true }));
const projectRoot = path.join(tempRoot, "project");
const casesRoot = path.join(tempRoot, "cases");
const dbPath = path.join(casesRoot, CASE_ID, "case.duckdb");
fs.mkdirSync(path.dirname(dbPath), { recursive: true });
writeCaseProjectBinding(projectRoot, CASE_ID);

run(BACKEND_PYTHON, ["-c", String.raw`
import os
import duckdb

db_path = os.environ["ANALYTIX_FPC1_DB"]
case_id = os.environ["ANALYTIX_FPC1_CASE_ID"]
if duckdb.__version__ != "1.5.4":
    raise RuntimeError("fixture requires DuckDB 1.5.4")
con = duckdb.connect(db_path)
con.execute("CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT)")
con.execute("INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 7)")
con.execute("""
CREATE TABLE fc_transaction_norm(
  id BIGINT, case_id TEXT, txn_ts TIMESTAMP, acct_no TEXT, amount DOUBLE,
  clean_amount TEXT, counterparty_acct TEXT, counterparty_name TEXT,
  counterparty_bank TEXT, currency TEXT, dc_flag TEXT, summary TEXT,
  remark TEXT, file_id TEXT, row_no BIGINT,
  clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER
)
""")
con.execute("""
INSERT INTO fc_transaction_norm VALUES
  (1, ?, TIMESTAMP '2026-01-01 01:02:03', '6222020000000000001', 12.5,
   '12.50', 'CP-1', '对手一', '银行甲', 'CNY', '进', '工资入账', '',
   'file-1', 1001, 0, 0, 0),
  (2, ?, TIMESTAMP '2026-01-02 01:02:03', '6222020000000000001', -0.0,
   '0.00', 'CP-2', '对手二', '银行乙', 'CNY', '出', '转出', 'POS',
   'file-1', 1002, 0, 0, 0)
""", [case_id, case_id])
con.execute("""
CREATE TABLE import_file_log(
  file_id TEXT, case_id TEXT, kind TEXT, sha256 TEXT,
  rows_imported_norm BIGINT, status TEXT, cleaned_status TEXT
)
""")
con.execute("""
INSERT INTO import_file_log VALUES (
  'file-1', ?, 'fc_transaction',
  'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  2, '已完成', 'done'
)
""", [case_id])
con.close()
`], {
  env: {
    ANALYTIX_FPC1_DB: dbPath,
    ANALYTIX_FPC1_CASE_ID: CASE_ID
  }
});

const typedRequest = {
  request_id: "fpc1-typed",
  command: "funds.materialize_txn_daily_v1",
  case_id: CASE_ID,
  db_path: dbPath,
  payload: {
    caseId: CASE_ID,
    sourceRevision: 7,
    sourceRowCount: 2,
    sourceMaxTxnTs: "2026-01-02 01:02:03",
    sourceMaxId: 2,
    rawArtifactManifestSha256: RAW_ARTIFACT_MANIFEST_SHA256
  }
};
const dataEngineManifest = path.join(REPO_ROOT, "tools/data_engine/Cargo.toml");
run("cargo", ["build", "--quiet", "--manifest-path", dataEngineManifest]);
const cargoMetadata = JSON.parse(run("cargo", [
  "metadata", "--format-version", "1", "--no-deps", "--manifest-path", dataEngineManifest
]));
const dataEngineBinary = path.join(
  cargoMetadata.target_directory,
  "debug",
  process.platform === "win32" ? "analytix-data-engine.exe" : "analytix-data-engine"
);
const typedStdout = process.platform === "darwin"
  ? run(BACKEND_PYTHON, ["-c", String.raw`
import os
import subprocess
import sys

descriptor_three = os.open(os.devnull, os.O_RDONLY)
owner_reader, owner_writer = os.pipe()
if descriptor_three != 3 or owner_reader != 4:
    raise RuntimeError("test launcher descriptor allocation is invalid")
os.close(descriptor_three)

child = subprocess.Popen(
    [os.environ["ANALYTIX_FPC1_DATA_ENGINE_BINARY"]],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    env={
        "ANALYTIX_NATIVE_LAUNCH_NONCE": "a" * 64,
        "ANALYTIX_NATIVE_PROTOCOL_VERSION": "analytix-native-v1",
    },
    pass_fds=(owner_reader,),
)
os.close(owner_reader)
stdout, stderr = child.communicate(
    (os.environ["ANALYTIX_FPC1_TYPED_REQUEST_JSON"] + "\n").encode("utf-8")
)
os.close(owner_writer)
sys.stdout.buffer.write(stdout)
sys.stderr.buffer.write(stderr)
raise SystemExit(child.returncode)
`], {
    env: {
      ANALYTIX_FPC1_DATA_ENGINE_BINARY: dataEngineBinary,
      ANALYTIX_FPC1_TYPED_REQUEST_JSON: JSON.stringify(typedRequest)
    }
  })
  : run(dataEngineBinary, [], {
    input: `${JSON.stringify(typedRequest)}\n`,
    env: {
      ANALYTIX_NATIVE_LAUNCH_NONCE: "a".repeat(64),
      ANALYTIX_NATIVE_PROTOCOL_VERSION: "analytix-native-v1"
    }
  });
const typedEnvelope = JSON.parse(typedStdout.split(/\r?\n/u).filter(Boolean).at(-1));
assert.equal(
  typedEnvelope.ok,
  true,
  `typed data-engine request failed: ${String(typedEnvelope.error?.code || "unknown")}/${String(typedEnvelope.error?.message || "unknown")}`
);
assert.equal(typedEnvelope.data.caseId, CASE_ID);
const materialized = { producer_content_id: typedEnvelope.data.producerContentId };
assert.match(materialized.producer_content_id, /^fpc1_[a-f0-9]{64}$/u);
assert.equal(materialized.producer_content_id, EXPECTED_PRODUCER_CONTENT_ID);
assert.doesNotMatch(materialized.producer_content_id, /^dsv2_/u);
assert.equal(typedEnvelope.data.materializationIdentitySchemaVersion, 2);
assert.equal(typedEnvelope.data.rawArtifactManifestSha256, RAW_ARTIFACT_MANIFEST_SHA256);
assert.match(typedEnvelope.data.producerContentManifestSha256, /^[a-f0-9]{64}$/u);
assert.match(typedEnvelope.data.rawSourceManifestSha256, /^[a-f0-9]{64}$/u);
assert.ok(typedEnvelope.data.producerContentManifestByteLength > 0);
assert.ok(typedEnvelope.data.rawSourceManifestByteLength > 0);
run("go", [
  "test", "-count=1", "-tags", "analytix_funds_cross_language",
  "./internal/adapters/outbound/nativecomponenthost", "-run", "^TestRustTypedFundsProducerBytesMatchGoCanonicalV1$"
], {
  cwd: path.join(REPO_ROOT, "packages/runtime-go"),
  env: { ANALYTIX_FPC1_TYPED_RESULT_JSON: JSON.stringify(typedEnvelope.data) }
});
const ownerLockPath = `${dbPath}.owner.lock`;
assert.equal(fs.existsSync(ownerLockPath), true);
fs.rmSync(ownerLockPath);

const env = {
  ...process.env,
  ANALYTIX_BACKEND_PYTHON: BACKEND_PYTHON,
  ANALYTIX_FUNDS_DUCKDB_PYTHON: BACKEND_PYTHON,
  ANALYTIX_CASE_PROJECT_ROOT: projectRoot,
  ANALYTIX_DATA_ANALYSIS_CASES_ROOT: casesRoot
};
const beforeProbeHash = sha256File(dbPath);
const beforeProbeMtime = fs.statSync(dbPath).mtimeMs;
const probe = await probeLocalFundsProducerContent({ case_id: CASE_ID }, { env });
assert.equal(probe.producer_content_contract, "analytix.funds-producer-content-manifest/v1");
assert.equal(probe.producer_content_id, materialized.producer_content_id);
assert.match(probe.producer_manifest_sha256, /^[a-f0-9]{64}$/u);
assert.equal(probe.producer_manifest_sha256, EXPECTED_PRODUCER_MANIFEST_SHA256);
assert.equal(probe.producer_manifest_schema_version, 1);
assert.equal(probe.host_dataset_snapshot_authority, "unavailable");
assert.equal(probe.fact_ready, false);
assert.equal(probe.read_only, true);
assert.equal(sha256File(dbPath), beforeProbeHash, "Python MCP verification must not modify the case DB");
assert.equal(fs.statSync(dbPath).mtimeMs, beforeProbeMtime, "Python MCP verification must remain read-only");

run(BACKEND_PYTHON, ["-c", String.raw`
import os
import duckdb
con = duckdb.connect(os.environ["ANALYTIX_FPC1_DB"])
con.execute("UPDATE analysis_txn_detail_idx SET amount=99 WHERE id=1")
con.close()
`], { env: { ANALYTIX_FPC1_DB: dbPath } });
await assert.rejects(
  () => probeLocalFundsProducerContent({ case_id: CASE_ID }, { env }),
  (error) => isDuckdbDiagnosticError(error),
  "same-count materialized content tampering must fail closed"
);

const result = {
  status: "ok",
  contract: "FundsProducerContentManifestV1",
  producer_content_id: probe.producer_content_id,
  producer_manifest_sha256: probe.producer_manifest_sha256,
  assertions: [
    "RustProducerAndPythonMcpConsumerAgreeExactly",
    "RustTypedFundsProducerBytesMatchGoCanonicalV1",
    "ProducerContentIdNeverUsesHostDatasetSnapshotPrefix",
    "PythonMcpConsumerIsReadOnly",
    "SameCountContentTamperFailsClosed",
    "ProducerContentAloneNeverAuthorizesFacts"
  ]
};

if (process.argv.includes("--json")) {
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
} else {
  console.log("FundsProducerContentManifestV1 cross-language contract passed.");
}
