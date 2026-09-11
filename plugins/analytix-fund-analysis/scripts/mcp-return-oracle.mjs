#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  resolveCaseDuckdbPath,
  resolveCaseProjectContext
} from "../mcp/case-project-context.mjs";
import { createAgentOutputCompiler } from "../mcp/agent-output-compiler.mjs";
import { createDestinationDiagnosticRuntime } from "../mcp/destination-diagnostic-runtime.mjs";
import { compactToolPayloadForAgent } from "../mcp/agent-payload-compiler.mjs";
import {
  executeLocalDuckdbWorkbenchSkill,
  probeLocalDuckdbDatasetSnapshot
} from "../mcp/duckdb-workbench-runtime.mjs";
import { moneyText } from "../mcp/frontdoor-fact-summaries.mjs";
import { createToolCallRuntime } from "../mcp/tool-call-runtime.mjs";
import { hostFactRuntimeContext } from "./host-runtime-context-fixture.mjs";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");

const PYTHON_SCRIPT = String.raw`
import datetime
import decimal
import json
import sys
import duckdb

def normalize(value):
    if isinstance(value, (datetime.datetime, datetime.date, datetime.time)):
        try:
            return value.isoformat(sep=" ")
        except TypeError:
            return value.isoformat()
    if isinstance(value, decimal.Decimal):
        return float(value)
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    if isinstance(value, list):
        return [normalize(item) for item in value]
    if isinstance(value, tuple):
        return [normalize(item) for item in value]
    if isinstance(value, dict):
        return {str(key): normalize(item) for key, item in value.items()}
    return value

payload = json.loads(sys.stdin.read())
limit = max(1, min(int(payload.get("row_limit") or 100), int(payload.get("max_limit") or 5000)))
con = duckdb.connect(payload["db_path"], read_only=True)
try:
    cursor = con.execute(payload["sql"])
    columns = [desc[0] for desc in cursor.description or []]
    rows = cursor.fetchmany(limit + 1)
finally:
    con.close()

records = [
    {columns[index]: normalize(value) for index, value in enumerate(row)}
    for row in rows[:limit]
]
print(json.dumps({
    "columns": columns,
    "records": records,
    "row_count": len(records),
    "truncated": len(rows) > limit
}, ensure_ascii=False))
`;

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function timestamp() {
  return new Date().toISOString().replace(/[:.]/gu, "-");
}

function defaultOutputDir() {
  return path.join(REPO_ROOT, "output", "analytix-fund-analysis", "mcp-return-oracle", timestamp());
}

function defaultCaseProjectRoot(env = process.env) {
  const explicit = text(env.ANALYTIX_CASE_PROJECT_ROOT || env.ANALYTIX_WORKSPACE_ROOT);
  if (explicit) return explicit;
  const localOracleProject = path.join(REPO_ROOT, "output", "analytix-fund-analysis", "oracle-case-project");
  if (fs.existsSync(path.join(localOracleProject, ".analytix", "case-project.json"))) {
    return localOracleProject;
  }
  return "";
}

function defaultCaseDb(env = process.env) {
  const explicit = text(env.ANALYTIX_FUNDS_ORACLE_CASE_DB || env.ANALYTIX_FUNDS_CASE_DB || env.ANALYTIX_CASE_DB);
  if (explicit) return explicit;
  const caseProjectRoot = defaultCaseProjectRoot(env);
  if (caseProjectRoot) {
    const context = resolveCaseProjectContext({ _analytix: { workspaceRealPath: caseProjectRoot } }, env);
    if (context?.case_id) return resolveCaseDuckdbPath(context.case_id, env);
  }
  const caseId = text(env.ANALYTIX_FUNDS_ORACLE_CASE_ID || env.ANALYTIX_CASE_ID);
  return caseId ? resolveCaseDuckdbPath(caseId, env) : "";
}

function parseArgs(argv) {
  const options = {
    json: false,
    summaryJson: false,
    noWrite: false,
    selfTestSecurityBoundary: false,
    caseDb: "",
    caseId: "",
    caseProjectRoot: defaultCaseProjectRoot(),
    outputDir: "",
    failOnGaps: false,
    requireCaseDb: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--json") options.json = true;
    else if (arg === "--summary-json") options.summaryJson = true;
    else if (arg === "--no-write") options.noWrite = true;
    else if (arg === "--self-test-security-boundary") options.selfTestSecurityBoundary = true;
    else if (arg === "--case-db") options.caseDb = next();
    else if (arg.startsWith("--case-db=")) options.caseDb = arg.slice("--case-db=".length);
    else if (arg === "--case-id") options.caseId = next();
    else if (arg.startsWith("--case-id=")) options.caseId = arg.slice("--case-id=".length);
    else if (arg === "--case-project-root") options.caseProjectRoot = path.resolve(next());
    else if (arg.startsWith("--case-project-root=")) options.caseProjectRoot = path.resolve(arg.slice("--case-project-root=".length));
    else if (arg === "--output-dir") options.outputDir = next();
    else if (arg.startsWith("--output-dir=")) options.outputDir = arg.slice("--output-dir=".length);
    else if (arg === "--fail-on-gaps") options.failOnGaps = true;
    else if (arg === "--require-case-db") options.requireCaseDb = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  const defaultEnv = { ...process.env };
  if (options.caseProjectRoot) {
    defaultEnv.ANALYTIX_CASE_PROJECT_ROOT = options.caseProjectRoot;
    defaultEnv.ANALYTIX_WORKSPACE_ROOT = options.caseProjectRoot;
  }
  if (!options.caseDb) options.caseDb = defaultCaseDb(defaultEnv);
  if (options.summaryJson !== options.noWrite) {
    throw new Error("--summary-json and --no-write must be used together");
  }
  if (options.summaryJson && options.json) {
    throw new Error("--summary-json cannot be combined with --json");
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node plugins/analytix-fund-analysis/scripts/mcp-return-oracle.mjs [options]

Deterministically validates Analytix Workbench MCP return values against a
read-only DuckDB oracle. It does not launch Analytix and does not call an LLM.

Options:
  --case-project-root <dir>
                         analytix case-project workspace containing .analytix/case-project.json.
                         Defaults to ANALYTIX_CASE_PROJECT_ROOT / ANALYTIX_WORKSPACE_ROOT.
  --case-db <path>       Real case DuckDB path. Default: ANALYTIX_FUNDS_ORACLE_CASE_DB,
                         or the DuckDB resolved from --case-project-root / ANALYTIX_DATA_ANALYSIS_CASES_ROOT.
                         If unset, the oracle is skipped unless --require-case-db is used.
  --case-id <id>         Case id. Default: parent directory name of --case-db.
  --output-dir <dir>     Write JSON/Markdown evidence to this directory.
  --fail-on-gaps         Exit non-zero when any oracle check fails.
  --require-case-db      Treat a missing case DB as failure instead of skipped.
  --summary-json         Print only bounded status/count fields; requires --no-write.
  --no-write             Do not create persistent oracle evidence; requires --summary-json.
  --self-test-security-boundary
                         Test canonical case binding and bounded no-write output in a temporary root.
  --json                 Print JSON to stdout.
`);
}

function pythonCandidates(env = process.env) {
  return [
    env.ANALYTIX_FUNDS_DUCKDB_PYTHON,
    env.ANALYTIX_BACKEND_PYTHON,
    env.PYTHON,
    path.join(REPO_ROOT, ".venv", process.platform === "win32" ? "Scripts/python.exe" : "bin/python"),
    "python3",
    "python"
  ].map(text).filter(Boolean);
}

function runDuckdbOracle(caseDb, sql, { rowLimit = 100, maxLimit = 5000 } = {}) {
  const input = JSON.stringify({ db_path: caseDb, sql, row_limit: rowLimit, max_limit: maxLimit });
  const errors = [];
  for (const python of pythonCandidates()) {
    if (python.includes(path.sep) && !fs.existsSync(python)) continue;
    const result = spawnSync(python, ["-c", PYTHON_SCRIPT], {
      input,
      encoding: "utf8",
      timeout: 60_000,
      maxBuffer: 20 * 1024 * 1024
    });
    if (result.status === 0) return JSON.parse(result.stdout || "{}");
    errors.push(text(result.stderr || result.stdout || `${python} exited ${result.status}`));
  }
  throw new Error(`DuckDB oracle failed: ${errors.slice(-2).join(" | ")}`);
}

function boundaryError(code) {
  const error = new Error(code);
  error.code = code;
  return error;
}

function existingRegularFileIdentity(filePath) {
  const resolved = path.resolve(filePath);
  let realPath;
  let stat;
  try {
    realPath = fs.realpathSync(resolved);
    stat = fs.lstatSync(resolved, { bigint: true });
  } catch {
    throw boundaryError("case_db_unavailable");
  }
  if (realPath !== resolved || !stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1n) {
    throw boundaryError("case_db_identity_invalid");
  }
  return Object.freeze({
    path: resolved,
    dev: stat.dev,
    ino: stat.ino,
    size: stat.size,
    mtimeNs: stat.mtimeNs,
    ctimeNs: stat.ctimeNs
  });
}

function sameFileIdentity(left, right) {
  return left.path === right.path
    && left.dev === right.dev
    && left.ino === right.ino
    && left.size === right.size
    && left.mtimeNs === right.mtimeNs
    && left.ctimeNs === right.ctimeNs;
}

function resolveOracleCaseBinding(options, env = process.env) {
  const configuredRoot = text(options.caseProjectRoot || defaultCaseProjectRoot(env));
  if (!configuredRoot) throw boundaryError("case_project_binding_required");
  let workspaceRealPath;
  try {
    workspaceRealPath = fs.realpathSync(path.resolve(configuredRoot));
  } catch {
    throw boundaryError("case_project_binding_unavailable");
  }
  const boundEnv = {
    ...env,
    ANALYTIX_CASE_PROJECT_ROOT: workspaceRealPath,
    ANALYTIX_WORKSPACE_ROOT: workspaceRealPath
  };
  // A caller-controlled cases-root override cannot redefine which database is
  // authoritative for a project binding. The canonical resolver must select
  // the existing same-case database from the Analytix-owned data roots.
  delete boundEnv.ANALYTIX_DATA_ANALYSIS_CASES_ROOT;
  const projectContext = resolveCaseProjectContext({
    _analytix: { workspaceRealPath }
  }, boundEnv);
  if (!projectContext || projectContext.workspace_root !== workspaceRealPath) {
    throw boundaryError("case_project_binding_invalid");
  }
  if (text(options.caseId) && text(options.caseId) !== projectContext.case_id) {
    throw boundaryError("case_id_binding_mismatch");
  }
  let authoritativePath;
  try {
    authoritativePath = path.resolve(resolveCaseDuckdbPath(projectContext.case_id, boundEnv));
  } catch {
    throw boundaryError("case_db_binding_unavailable");
  }
  const candidatePath = path.resolve(text(options.caseDb) || authoritativePath);
  if (candidatePath !== authoritativePath) throw boundaryError("case_db_binding_mismatch");
  if (!fs.existsSync(authoritativePath)) {
    return {
      projectContext,
      caseId: projectContext.case_id,
      caseDb: authoritativePath,
      caseDbIdentity: null,
      env: boundEnv
    };
  }
  const caseDbIdentity = existingRegularFileIdentity(authoritativePath);
  return {
    projectContext,
    caseId: projectContext.case_id,
    caseDb: authoritativePath,
    caseDbIdentity,
    env: boundEnv
  };
}

function assertOracleCaseBindingUnchanged(binding) {
  if (!binding?.caseDbIdentity) throw boundaryError("case_db_identity_missing");
  const observed = existingRegularFileIdentity(binding.caseDb);
  if (!sameFileIdentity(binding.caseDbIdentity, observed)) {
    throw boundaryError("case_db_identity_changed");
  }
}

function sqlString(value) {
  return `'${text(value).replace(/'/gu, "''")}'`;
}

function allowedTableName(tableName) {
  const simple = text(tableName).split(".").pop().toLowerCase();
  return /^analysis_[a-z0-9_]*$/u.test(simple) || /^fc_[a-z0-9_]*_norm$/u.test(simple);
}

function stableHash(value) {
  return crypto
    .createHash("sha256")
    .update(JSON.stringify(value || {}, (_key, item) => (typeof item === "bigint" ? String(item) : item)))
    .digest("hex");
}

function normalizeRecord(record) {
  return Object.fromEntries(Object.entries(objectOf(record)).sort(([left], [right]) => left.localeCompare(right)));
}

function equivalentValue(left, right) {
  if (typeof left === "number" || typeof right === "number") {
    const l = Number(left);
    const r = Number(right);
    if (Number.isNaN(l) || Number.isNaN(r)) return Object.is(left, right);
    return Math.abs(l - r) <= Math.max(1e-6, Math.abs(r) * 1e-9);
  }
  return JSON.stringify(left) === JSON.stringify(right);
}

function compareRecords(actual, expected, fields) {
  for (const field of fields) {
    if (!equivalentValue(objectOf(actual)[field], objectOf(expected)[field])) {
      throw new Error(`${field} mismatch: MCP=${JSON.stringify(objectOf(actual)[field])}, DuckDB=${JSON.stringify(objectOf(expected)[field])}`);
    }
  }
}

function compareRecordArrays(actualRows, expectedRows, fields) {
  if (actualRows.length !== expectedRows.length) {
    throw new Error(`row count mismatch: MCP=${actualRows.length}, DuckDB=${expectedRows.length}`);
  }
  for (let index = 0; index < expectedRows.length; index += 1) {
    compareRecords(actualRows[index], expectedRows[index], fields);
  }
}

function caseProjectArgs(caseProjectRoot, datasetSnapshotId) {
  const workspaceRealPath = text(caseProjectRoot) ? fs.realpathSync(path.resolve(caseProjectRoot)) : "";
  if (!workspaceRealPath) return {};
  const projectContext = resolveCaseProjectContext({}, {
    ...process.env,
    ANALYTIX_CASE_PROJECT_ROOT: workspaceRealPath
  });
  if (!projectContext) return {};
  return {
    workspaceRealPath,
    caseId: projectContext.case_id,
    caseBindingHash: projectContext.case_binding_hash,
    datasetSnapshotId: text(datasetSnapshotId),
    threadId: "thread_mcp_return_oracle",
    turnId: "turn_mcp_return_oracle",
    issuedAt: new Date(Date.now() - 30_000).toISOString()
  };
}

function runtimeForCase(caseId, caseDb, caseProjectRoot = "") {
  const caseRoot = path.dirname(path.dirname(path.resolve(caseDb)));
  const env = {
    ...process.env,
    ANALYTIX_CASE_PROJECT_ROOT: text(caseProjectRoot) || process.env.ANALYTIX_CASE_PROJECT_ROOT,
    ANALYTIX_WORKSPACE_ROOT: text(caseProjectRoot) || process.env.ANALYTIX_WORKSPACE_ROOT,
    ANALYTIX_DATA_ANALYSIS_CASES_ROOT: process.env.ANALYTIX_DATA_ANALYSIS_CASES_ROOT || caseRoot
  };
  return createToolCallRuntime({
    env,
    executeSkill: (name, args) => executeLocalDuckdbWorkbenchSkill(name, args, { env })
  });
}

function fallbackRuntimeForCase(caseDb, caseProjectRoot = "") {
  const caseRoot = path.dirname(path.dirname(path.resolve(caseDb)));
  const env = {
    ...process.env,
    ANALYTIX_CASE_PROJECT_ROOT: text(caseProjectRoot) || process.env.ANALYTIX_CASE_PROJECT_ROOT,
    ANALYTIX_WORKSPACE_ROOT: text(caseProjectRoot) || process.env.ANALYTIX_WORKSPACE_ROOT,
    ANALYTIX_DATA_ANALYSIS_CASES_ROOT: process.env.ANALYTIX_DATA_ANALYSIS_CASES_ROOT || caseRoot
  };
  return createToolCallRuntime({ env });
}

async function callTool(runtime, name, args, runtimeArgs = {}) {
  const callArgs = { ...args };
  let hostArgs = {};
  if (runtimeArgs.workspaceRealPath && runtimeArgs.caseId && runtimeArgs.caseBindingHash && runtimeArgs.datasetSnapshotId) {
    const base = hostFactRuntimeContext({
      workspaceRealPath: runtimeArgs.workspaceRealPath,
      caseId: runtimeArgs.caseId,
      caseBindingHash: runtimeArgs.caseBindingHash,
      toolName: name,
      args: callArgs,
      overrides: {
        threadId: text(runtimeArgs.threadId) || "thread_mcp_return_oracle",
        turnId: text(runtimeArgs.turnId) || "turn_mcp_return_oracle",
        contextEpoch: 1,
        datasetSnapshotId: runtimeArgs.datasetSnapshotId,
        issuedAt: runtimeArgs.issuedAt
      }
    });
    hostArgs = { _analytix: base };
  }
  const result = await runtime.callTool(name, { ...callArgs, ...hostArgs });
  return {
    envelope: result,
    response: objectOf(result.response || result)
  };
}

function listAllowedTables(caseDb) {
  const tables = runDuckdbOracle(caseDb, `
    SELECT table_name
    FROM information_schema.tables
    WHERE table_schema NOT IN ('information_schema', 'pg_catalog')
      AND (lower(table_name) LIKE 'analysis_%' OR regexp_matches(lower(table_name), '^fc_[a-z0-9_]*_norm$'))
    ORDER BY table_name
  `, { rowLimit: 5000, maxLimit: 5000 }).records.map((row) => text(row.table_name));
  const counts = new Map();
  for (const table of tables) {
    const count = runDuckdbOracle(caseDb, `SELECT COUNT(*) AS row_count FROM "${table}"`, { rowLimit: 1 }).records[0]?.row_count;
    counts.set(table, Number(count || 0));
  }
  return { tables, counts };
}

function writeEvidence(outputDir, payload) {
  const resolved = path.resolve(REPO_ROOT, outputDir || defaultOutputDir());
  fs.mkdirSync(resolved, { recursive: true });
  const jsonPath = path.join(resolved, "mcp-return-oracle.json");
  const mdPath = path.join(resolved, "mcp-return-oracle.md");
  fs.writeFileSync(jsonPath, `${JSON.stringify(payload, null, 2)}\n`);
  fs.writeFileSync(mdPath, renderMarkdown(payload));
  return { output_dir: resolved, json_path: jsonPath, markdown_path: mdPath };
}

function renderMarkdown(payload) {
  const lines = [
    "# Analytix MCP Return Oracle",
    "",
    `- Status: ${payload.status}`,
    `- Case: ${payload.case_id}`,
    `- Case DB: ${payload.case_db}`,
    `- Generated: ${payload.generated_at}`,
    "",
    "| Check | Status | Detail |",
    "| --- | --- | --- |"
  ];
  for (const result of payload.results || []) {
    lines.push(`| ${result.id} | ${result.status} | ${text(result.summary || result.error).replace(/\|/gu, "\\|")} |`);
  }
  if (arrayOf(payload.findings).length) {
    lines.push("", "## Findings");
    for (const finding of payload.findings) {
      lines.push(`- ${finding.severity}: ${finding.message}`);
    }
  }
  lines.push("");
  return `${lines.join("\n")}\n`;
}

const INVESTIGATION_SCENARIO_RECIPES = [
  "case_overview_from_daily_agg",
  "holder_account_scope_overview",
  "account_transaction_overview",
  "holder_top_counterparties",
  "one_hop_amount_by_parties",
  "keyword_evidence_hits",
  "large_amount_cash_review",
  "fast_in_out_pattern_review",
  "shared_device_contact_address_review",
  "abnormal_time_high_frequency_review",
  "fund_usage_keyword_classifier",
  "account_role_candidate_classification",
  "fund_path_evidence_support_check",
  "same_fact_duplicate_review"
];

const INVESTIGATION_SCENARIO_QUERIES = [
  {
    id: "duplicate_cleaning_flags",
    purpose: "oracle cleaning duplicate/reversal/invalid flag summary",
    resultMode: "aggregate",
    rowLimit: 1,
    sql: `
      SELECT
        COUNT(*) AS total_rows,
        SUM(CASE WHEN clean_duplicate = 1 THEN 1 ELSE 0 END) AS clean_duplicate_rows,
        SUM(CASE WHEN clean_reversal = 1 THEN 1 ELSE 0 END) AS clean_reversal_rows,
        SUM(CASE WHEN clean_invalid = 1 THEN 1 ELSE 0 END) AS clean_invalid_rows
      FROM fc_transaction_norm
    `
  },
  {
    id: "holder_account_opening_overview",
    purpose: "oracle holder account opening, status and balance overview",
    resultMode: "evidence_table",
    rowLimit: 10,
    sql: `
      SELECT
        account_open_name AS holder_name,
        COUNT(DISTINCT COALESCE(NULLIF(acct_no_norm, ''), NULLIF(card_no_norm, ''))) AS account_count,
        COUNT(DISTINCT acct_status) AS status_count,
        SUM(COALESCE(balance_val, 0)) AS balance_sum,
        SUM(COALESCE(available_balance_val, 0)) AS available_balance_sum,
        MIN(open_time_ts) AS first_open_time,
        MAX(open_time_ts) AS last_open_time
      FROM fc_account_norm
      GROUP BY account_open_name
      ORDER BY account_count DESC, balance_sum DESC, holder_name
      LIMIT 10
    `
  },
  {
    id: "account_transaction_structure",
    purpose: "oracle account transaction overview and counterparty breadth",
    resultMode: "evidence_table",
    rowLimit: 10,
    sql: `
      SELECT
        agg.acct_key,
        MAX(dim.open_name) AS holder_name,
        SUM(agg.txn_count) AS txn_count,
        SUM(CASE WHEN agg.dc_val = '进' THEN agg.amt_sum ELSE 0 END) AS in_amount,
        SUM(CASE WHEN agg.dc_val = '出' THEN agg.amt_sum ELSE 0 END) AS out_amount,
        COUNT(DISTINCT agg.cp_key) AS counterparty_count,
        MIN(agg.first_ts) AS first_txn_ts,
        MAX(agg.last_ts) AS last_txn_ts
      FROM analysis_txn_daily_agg agg
      LEFT JOIN analysis_account_dim dim ON dim.account_key = agg.acct_key
      GROUP BY agg.acct_key
      ORDER BY txn_count DESC, (COALESCE(in_amount, 0) + COALESCE(out_amount, 0)) DESC, agg.acct_key
      LIMIT 10
    `
  },
  {
    id: "large_amount_and_cash",
    purpose: "oracle large amount and cash clue summary",
    resultMode: "aggregate",
    rowLimit: 1,
    sql: `
      SELECT
        COUNT(*) AS txn_count,
        SUM(amount) AS amount_sum,
        SUM(CASE WHEN amount >= 100000 THEN 1 ELSE 0 END) AS large_txn_count,
        SUM(CASE WHEN amount >= 100000 THEN amount ELSE 0 END) AS large_amount_sum,
        SUM(CASE WHEN cash_flag IN ('01', '1') THEN 1 ELSE 0 END) AS cash_txn_count,
        SUM(CASE WHEN cash_flag IN ('01', '1') THEN amount ELSE 0 END) AS cash_amount_sum
      FROM analysis_txn_detail_idx
    `
  },
  {
    id: "fast_in_out_daily_pattern",
    purpose: "oracle fast-in-fast-out daily candidate pattern",
    resultMode: "aggregate",
    rowLimit: 1,
    sql: `
      WITH daily AS (
        SELECT
          acct_key,
          txn_day,
          SUM(CASE WHEN dc_val = '进' THEN amt_sum ELSE 0 END) AS in_amount,
          SUM(CASE WHEN dc_val = '出' THEN amt_sum ELSE 0 END) AS out_amount,
          SUM(txn_count) AS txn_count
        FROM analysis_txn_daily_agg
        GROUP BY acct_key, txn_day
      )
      SELECT
        COUNT(*) AS candidate_day_count,
        SUM(in_amount) AS in_amount_sum,
        SUM(out_amount) AS out_amount_sum,
        MAX(txn_count) AS max_daily_txn_count
      FROM daily
      WHERE in_amount > 0 AND out_amount >= in_amount * 0.8
    `
  },
  {
    id: "device_contact_address_association",
    purpose: "oracle shared device/contact/address association coverage",
    resultMode: "aggregate",
    rowLimit: 1,
    sql: `
      SELECT
        (SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE ip_addr IS NOT NULL AND ip_addr <> '') AS ip_rows,
        (SELECT COUNT(*) FROM analysis_txn_detail_idx WHERE mac_addr IS NOT NULL AND mac_addr <> '') AS mac_rows,
        (SELECT COUNT(*) FROM (
          SELECT ip_addr FROM analysis_txn_detail_idx
          WHERE ip_addr IS NOT NULL AND ip_addr <> ''
          GROUP BY ip_addr HAVING COUNT(DISTINCT acct_key) > 1
        )) AS shared_ip_count,
        (SELECT COUNT(*) FROM (
          SELECT mac_addr FROM analysis_txn_detail_idx
          WHERE mac_addr IS NOT NULL AND mac_addr <> ''
          GROUP BY mac_addr HAVING COUNT(DISTINCT acct_key) > 1
        )) AS shared_mac_count,
        (SELECT COUNT(*) FROM fc_person_contact_norm WHERE contact_phone IS NOT NULL AND contact_phone <> '') AS contact_rows,
        (SELECT COUNT(*) FROM fc_person_address_norm WHERE home_addr IS NOT NULL AND home_addr <> '') AS address_rows
    `
  },
  {
    id: "time_high_frequency_pattern",
    purpose: "oracle night and high-frequency transaction pattern",
    resultMode: "aggregate",
    rowLimit: 1,
    sql: `
      WITH hourly AS (
        SELECT
          acct_key,
          DATE_TRUNC('hour', txn_ts) AS hour_bucket,
          COUNT(*) AS txn_count,
          SUM(amount) AS amount_sum
        FROM analysis_txn_detail_idx
        WHERE txn_ts IS NOT NULL
        GROUP BY acct_key, DATE_TRUNC('hour', txn_ts)
      )
      SELECT
        COUNT(*) AS high_frequency_hour_count,
        MAX(txn_count) AS max_hourly_txn_count,
        SUM(CASE WHEN EXTRACT(hour FROM hour_bucket) BETWEEN 0 AND 5 THEN txn_count ELSE 0 END) AS night_txn_count,
        SUM(CASE WHEN EXTRACT(hour FROM hour_bucket) BETWEEN 0 AND 5 THEN amount_sum ELSE 0 END) AS night_amount_sum,
        SUM(CASE WHEN strftime(hour_bucket, '%w') IN ('0', '6') THEN txn_count ELSE 0 END) AS weekend_txn_count,
        SUM(CASE WHEN strftime(hour_bucket, '%w') IN ('0', '6') THEN amount_sum ELSE 0 END) AS weekend_amount_sum
      FROM hourly
      WHERE txn_count >= 20
        OR EXTRACT(hour FROM hour_bucket) BETWEEN 0 AND 5
        OR strftime(hour_bucket, '%w') IN ('0', '6')
    `
  },
  {
    id: "fund_usage_keyword_classifier",
    purpose: "oracle literal transaction-text keyword-hit support without purpose classification",
    resultMode: "evidence_table",
    rowLimit: 10,
    sql: `
      WITH tagged AS (
        SELECT
          CASE WHEN regexp_matches(
            COALESCE(summary, '') || ' ' || COALESCE(remark, '') || ' ' || COALESCE(txn_type, '') || ' ' || COALESCE(merchant_name, ''),
            '车|汽车|购车|车辆|停车|房|购房|物业|装修|不动产|理财|基金|证券|股票|保险|投资|贷款|还款|借款|利息|工程|项目|货款|材料|现金|ATM|取现|存现'
          ) OR cash_flag IN ('01', '1') THEN 'transaction_text_keyword_hit' ELSE 'no_keyword_hit' END AS lead_status,
          amount
        FROM analysis_txn_detail_idx
      )
      SELECT lead_status, COUNT(*) AS txn_count, SUM(amount) AS amount_sum
      FROM tagged
      WHERE lead_status = 'transaction_text_keyword_hit'
      GROUP BY lead_status
      ORDER BY amount_sum DESC, lead_status
      LIMIT 10
    `
  },
  {
    id: "account_role_candidates",
    purpose: "oracle key/transfer/accumulation/terminal account role candidates",
    resultMode: "evidence_table",
    rowLimit: 10,
    sql: `
      SELECT
        node_key,
        display_name,
        txn_count,
        total_amount,
        in_amount,
        out_amount,
        amount_share,
        CASE
          WHEN in_amount > 0 AND out_amount / in_amount >= 0.8 AND txn_count >= 20 THEN 'transfer_hub_candidate'
          WHEN in_amount > out_amount * 3 THEN 'accumulation_candidate'
          WHEN out_amount > in_amount * 3 THEN 'terminal_outflow_candidate'
          WHEN amount_share >= 0.1 THEN 'key_account_candidate'
          ELSE 'review_candidate'
        END AS role_candidate,
        quality_label
      FROM analysis_key_node_features
      ORDER BY key_score DESC, total_amount DESC, node_key
      LIMIT 10
    `
  },
  {
    id: "fund_path_and_evidence_integrity",
    purpose: "oracle fund-flow graph/path/evidence support integrity",
    resultMode: "aggregate",
    rowLimit: 1,
    sql: `
      SELECT
        (SELECT COUNT(*) FROM analysis_relation_edge) AS edge_count,
        (SELECT COUNT(*) FROM analysis_relation_edge edge
          LEFT JOIN analysis_entity_node src ON src.entity_id = edge.src_entity_id
          LEFT JOIN analysis_entity_node dst ON dst.entity_id = edge.dst_entity_id
          WHERE src.entity_id IS NULL OR dst.entity_id IS NULL) AS missing_endpoint_edge_count,
        (SELECT COUNT(*) FROM analysis_trace_path_hop) AS trace_hop_count,
        (SELECT COUNT(*) FROM analysis_trace_path_hop hop
          LEFT JOIN analysis_trace_path path ON path.path_id = hop.path_id
          LEFT JOIN analysis_txn_detail_idx txn ON CAST(txn.id AS VARCHAR) = CAST(hop.txn_id AS VARCHAR)
            OR CAST(txn.txn_id AS VARCHAR) = CAST(hop.txn_id AS VARCHAR)
          WHERE path.path_id IS NULL OR txn.id IS NULL) AS missing_trace_support_count,
        (SELECT COUNT(*) FROM analysis_evidence_ref
          WHERE ref_table IS NULL OR ref_pk IS NULL OR ref_table = '') AS malformed_evidence_ref_count
    `
  }
];

async function runCheck(id, fn) {
  const startedAt = Date.now();
  try {
    const details = await fn();
    return {
      id,
      status: "passed",
      elapsed_ms: Date.now() - startedAt,
      summary: details?.summary || "passed",
      details
    };
  } catch (error) {
    return {
      id,
      status: "failed",
      elapsed_ms: Date.now() - startedAt,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}

async function runOracle(options) {
  let binding;
  try {
    binding = resolveOracleCaseBinding(options, process.env);
  } catch (error) {
    const reason = text(error?.code) || "case_project_binding_invalid";
    return {
      status: "failed",
      generated_at: new Date().toISOString(),
      case_id: "",
      case_db: "",
      reason,
      results: [],
      passed_count: 0,
      failed_count: 1,
      dataset_snapshot_v2_ready: false,
      findings: [{ severity: "p0", message: reason }]
    };
  }
  const { projectContext, caseDb, caseId } = binding;
  if (!binding.caseDbIdentity) {
    return {
      status: options.requireCaseDb ? "failed" : "skipped",
      generated_at: new Date().toISOString(),
      case_id: caseId,
      case_db: "",
      reason: "case_db_unavailable",
      results: [],
      passed_count: 0,
      failed_count: options.requireCaseDb ? 1 : 0,
      dataset_snapshot_v2_ready: false,
      findings: options.requireCaseDb
        ? [{ severity: "p0", message: "case_db_unavailable" }]
        : []
    };
  }

  const runtimeEnv = {
    ...binding.env,
    ANALYTIX_CASE_PROJECT_ROOT: options.caseProjectRoot || projectContext?.workspace_root,
    ANALYTIX_WORKSPACE_ROOT: options.caseProjectRoot || projectContext?.workspace_root,
    ANALYTIX_DATA_ANALYSIS_CASES_ROOT: path.dirname(path.dirname(caseDb))
  };
  let snapshotProbe;
  try {
    snapshotProbe = await probeLocalDuckdbDatasetSnapshot({ case_id: caseId }, { env: runtimeEnv });
  } catch {
    snapshotProbe = null;
  }
  const datasetSnapshotId = text(snapshotProbe?.observed_dataset_snapshot_id);
  const datasetSnapshotV2Ready = snapshotProbe?.ready === true
    && snapshotProbe?.read_only === true
    && snapshotProbe?.snapshot_contract === "analytix_duckdb_dataset_snapshot_v2"
    && snapshotProbe?.snapshot_manifest_schema_version === 2
    && !text(snapshotProbe?.blocker)
    && /^dsv2_[a-f0-9]{64}$/u.test(datasetSnapshotId);
  if (!datasetSnapshotV2Ready) {
    assertOracleCaseBindingUnchanged(binding);
    return {
      status: "failed",
      generated_at: new Date().toISOString(),
      case_id: caseId,
      case_db: "",
      reason: "dataset_snapshot_manifest_v2_unavailable",
      results: [],
      passed_count: 0,
      failed_count: 1,
      dataset_snapshot_v2_ready: false,
      findings: [{ severity: "p0", message: "dataset_snapshot_manifest_v2_unavailable" }]
    };
  }
  const { tables: allowedTables, counts } = listAllowedTables(caseDb);
  const runtime = runtimeForCase(caseId, caseDb, options.caseProjectRoot || projectContext?.workspace_root);
  const fallbackRuntime = fallbackRuntimeForCase(caseDb, options.caseProjectRoot || projectContext?.workspace_root);
  const runtimeArgs = caseProjectArgs(options.caseProjectRoot || projectContext?.workspace_root, datasetSnapshotId);
  const { compactToolText } = createAgentOutputCompiler({ env: process.env });
  const coreTables = [
    "analysis_txn_detail_idx",
    "analysis_txn_daily_agg",
    "analysis_account_dim"
  ].filter((table) => allowedTables.includes(table));
  const graphEvidenceTables = [
    {
      table: "analysis_entity_node",
      required_columns: [
        "entity_id",
        "case_id",
        "entity_type",
        "entity_key",
        "display_name",
        "first_seen_at",
        "last_seen_at"
      ]
    },
    {
      table: "analysis_relation_edge",
      required_columns: [
        "edge_id",
        "case_id",
        "src_entity_id",
        "dst_entity_id",
        "relation_type",
        "txn_count",
        "amount_sum",
        "first_seen_at",
        "last_seen_at"
      ]
    },
    {
      table: "analysis_evidence_ref",
      required_columns: [
        "evidence_id",
        "case_id",
        "evidence_type",
        "ref_table",
        "ref_pk",
        "title",
        "payload_json",
        "created_at"
      ]
    },
    {
      table: "analysis_trace_path",
      required_columns: [
        "path_id",
        "trace_id",
        "case_id",
        "path_index",
        "hop_count",
        "path_score",
        "evidence_ids_json",
        "detail_json"
      ]
    },
    {
      table: "analysis_trace_path_hop",
      required_columns: [
        "path_hop_id",
        "path_id",
        "hop_index",
        "txn_id",
        "src_entity_id",
        "dst_entity_id",
        "txn_time",
        "amount",
        "direction"
      ]
    },
    {
      table: "analysis_txn_daily_agg",
      required_columns: [
        "txn_day",
        "acct_key",
        "cp_key",
        "dc_val",
        "txn_count",
        "amt_sum",
        "first_ts",
        "last_ts",
        "open_name",
        "cp_name"
      ]
    }
  ];
  const results = [];

  results.push(await runCheck("inspect_case_schema_matches_duckdb", async () => {
    const { envelope, response } = await callTool(runtime, "inspect_case_schema", { case_id: caseId, table_limit: 50 }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error(`unexpected status: ${envelope.status}/${response.status}`);
    const mcpTables = arrayOf(response.data?.tables);
    if (mcpTables.length !== Math.min(50, allowedTables.length)) {
      throw new Error(`table_count mismatch: MCP=${mcpTables.length}, DuckDB=${allowedTables.length}`);
    }
    for (const table of mcpTables) {
      const tableName = text(table.table_name);
      if (!allowedTableName(tableName)) throw new Error(`disallowed table exposed: ${tableName}`);
      if (Number(table.row_count) !== Number(counts.get(tableName))) {
        throw new Error(`${tableName} row_count mismatch: MCP=${table.row_count}, DuckDB=${counts.get(tableName)}`);
      }
      for (const required of ["sql_name", "sql_identifier", "display_name", "role", "column_count"]) {
        if (!Object.prototype.hasOwnProperty.call(table, required)) throw new Error(`${tableName} missing ${required}`);
      }
      for (const column of arrayOf(table.columns)) {
        for (const required of ["sql_name", "sql_identifier", "display_name", "type", "ordinal"]) {
          if (!Object.prototype.hasOwnProperty.call(column, required)) {
            throw new Error(`${tableName}.${text(column.name)} missing ${required}`);
          }
        }
      }
    }
    return {
      summary: `${mcpTables.length} allowed cleaned/analysis tables matched DuckDB counts`,
      table_count: mcpTables.length,
      table_hash: stableHash(mcpTables.map((table) => [table.table_name, table.row_count]))
    };
  }));

  results.push(await runCheck("graph_and_evidence_tables_match_duckdb", async () => {
    const missing = graphEvidenceTables
      .map((item) => item.table)
      .filter((table) => !allowedTables.includes(table));
    if (missing.length) throw new Error(`required graph/evidence tables missing: ${missing.join(", ")}`);
    const { envelope, response } = await callTool(runtime, "inspect_case_schema", { case_id: caseId, table_limit: 100 }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error(`unexpected status: ${envelope.status}/${response.status}`);
    const tablesByName = new Map(arrayOf(response.data?.tables).map((table) => [text(table.table_name), table]));
    const checked = [];
    for (const requiredTable of graphEvidenceTables) {
      const table = tablesByName.get(requiredTable.table);
      if (!table) throw new Error(`${requiredTable.table} missing from MCP schema`);
      if (Number(table.row_count) !== Number(counts.get(requiredTable.table))) {
        throw new Error(`${requiredTable.table} row_count mismatch: MCP=${table.row_count}, DuckDB=${counts.get(requiredTable.table)}`);
      }
      const columnNames = new Set(arrayOf(table.columns)
        .map((column) => text(column.sql_name || column.column_name || column.name))
        .filter(Boolean));
      const missingColumns = requiredTable.required_columns.filter((columnName) => !columnNames.has(columnName));
      if (missingColumns.length) {
        throw new Error(`${requiredTable.table} missing columns: ${missingColumns.join(", ")}`);
      }
      const countCheck = await callTool(runtime, "count_case_rows", { case_id: caseId, table_name: requiredTable.table }, runtimeArgs);
      if (countCheck.envelope.status !== "ok" || countCheck.response.status !== "ok") {
        throw new Error(`${requiredTable.table} count_case_rows unexpected status`);
      }
      if (Number(countCheck.response.data?.row_count) !== Number(counts.get(requiredTable.table))) {
        throw new Error(`${requiredTable.table} count_case_rows mismatch: MCP=${countCheck.response.data?.row_count}, DuckDB=${counts.get(requiredTable.table)}`);
      }
      checked.push({
        table: requiredTable.table,
        row_count: Number(counts.get(requiredTable.table)),
        required_columns: requiredTable.required_columns
      });
    }
    return {
      summary: `${checked.length} graph/evidence/chart base tables matched DuckDB counts and required columns`,
      checked
    };
  }));

  results.push(await runCheck("graph_integrity_health_matches_duckdb", async () => {
    const edgeHealth = runDuckdbOracle(caseDb, `
      SELECT
        COUNT(DISTINCT edge.edge_id) AS edge_count,
        COUNT(DISTINCT CASE
          WHEN NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.src_entity_id)
          THEN edge.edge_id END) AS missing_src_count,
        COUNT(DISTINCT CASE
          WHEN NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.dst_entity_id)
          THEN edge.edge_id END) AS missing_dst_count,
        COUNT(DISTINCT CASE
          WHEN NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.src_entity_id)
            OR NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.dst_entity_id)
          THEN edge.edge_id END) AS missing_endpoint_edge_count,
        COUNT(DISTINCT CASE WHEN txn_count IS NULL OR txn_count <= 0 THEN edge.edge_id END) AS non_positive_txn_edges,
        COUNT(DISTINCT CASE WHEN amount_sum IS NULL THEN edge.edge_id END) AS null_amount_edges
      FROM analysis_relation_edge edge
    `, { rowLimit: 1 }).records[0] || {};
    const missingByRelation = runDuckdbOracle(caseDb, `
      SELECT
        edge.relation_type,
        COUNT(DISTINCT edge.edge_id) AS edge_count,
        COUNT(DISTINCT CASE
          WHEN NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.src_entity_id)
            OR NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.dst_entity_id)
          THEN edge.edge_id END) AS missing_endpoint_edge_count
      FROM analysis_relation_edge edge
      GROUP BY edge.relation_type
      HAVING COUNT(DISTINCT CASE
        WHEN NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.src_entity_id)
          OR NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.dst_entity_id)
        THEN edge.edge_id END) > 0
      ORDER BY missing_endpoint_edge_count DESC, edge_count DESC
      LIMIT 10
    `, { rowLimit: 10 }).records;
    const traceHealth = runDuckdbOracle(caseDb, `
      SELECT
        COUNT(DISTINCT hop.path_hop_id) AS hop_count,
        COUNT(DISTINCT CASE
          WHEN NOT EXISTS (SELECT 1 FROM analysis_trace_path path WHERE path.path_id = hop.path_id)
          THEN hop.path_hop_id END) AS missing_path_count,
        COUNT(DISTINCT CASE
          WHEN NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = hop.src_entity_id)
          THEN hop.path_hop_id END) AS missing_src_count,
        COUNT(DISTINCT CASE
          WHEN NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = hop.dst_entity_id)
          THEN hop.path_hop_id END) AS missing_dst_count,
        COUNT(DISTINCT CASE
          WHEN NOT EXISTS (
            SELECT 1
            FROM analysis_txn_detail_idx txn
            WHERE CAST(hop.txn_id AS VARCHAR) = CAST(txn.id AS VARCHAR)
              OR CAST(hop.txn_id AS VARCHAR) = CAST(txn.txn_id AS VARCHAR)
          )
          THEN hop.path_hop_id END) AS missing_txn_count
      FROM analysis_trace_path_hop hop
    `, { rowLimit: 1 }).records[0] || {};
    const evidenceRefHealth = runDuckdbOracle(caseDb, `
      SELECT
        COUNT(*) AS ref_count,
        SUM(CASE WHEN target.table_name IS NULL THEN 1 ELSE 0 END) AS missing_target_table_count,
        SUM(CASE
          WHEN regexp_matches(lower(ref.ref_table), '^analysis_[a-z0-9_]*$')
            OR regexp_matches(lower(ref.ref_table), '^fc_[a-z0-9_]*_norm$')
          THEN 0 ELSE 1 END) AS disallowed_ref_table_count
      FROM analysis_evidence_ref ref
      LEFT JOIN information_schema.tables target
        ON lower(target.table_name) = lower(ref.ref_table)
        AND target.table_schema NOT IN ('information_schema', 'pg_catalog')
    `, { rowLimit: 1 }).records[0] || {};
    const edgeCount = Number(edgeHealth.edge_count || 0);
    const missingEndpointEdges = Number(edgeHealth.missing_endpoint_edge_count || 0);
    const traceHopCount = Number(traceHealth.hop_count || 0);
    const findings = [];
    if (edgeCount && missingEndpointEdges) {
      findings.push({
        severity: "p1",
        message: `analysis_relation_edge has ${missingEndpointEdges}/${edgeCount} edges whose src_entity_id or dst_entity_id does not join analysis_entity_node; graph output must label this as a graph-index quality boundary until rebuilt.`
      });
    }
    if (Number(traceHealth.missing_path_count || 0) || Number(traceHealth.missing_txn_count || 0)) {
      throw new Error(`analysis_trace_path_hop has missing path/transaction references: path=${traceHealth.missing_path_count || 0}, txn=${traceHealth.missing_txn_count || 0}`);
    }
    if (Number(evidenceRefHealth.missing_target_table_count || 0) || Number(evidenceRefHealth.disallowed_ref_table_count || 0)) {
      throw new Error(`analysis_evidence_ref has invalid target tables: missing=${evidenceRefHealth.missing_target_table_count || 0}, disallowed=${evidenceRefHealth.disallowed_ref_table_count || 0}`);
    }
    return {
      summary: `${edgeCount} graph edges, ${missingEndpointEdges} endpoint-join gaps; ${traceHopCount} trace hops checked`,
      edge_endpoint_health: {
        ...edgeHealth,
        missing_endpoint_ratio: edgeCount ? missingEndpointEdges / edgeCount : 0
      },
      missing_endpoint_by_relation: missingByRelation,
      trace_path_health: traceHealth,
      evidence_ref_health: evidenceRefHealth,
      findings
    };
  }));

  results.push(await runCheck("visible_artifact_evidence_contract_matches_duckdb", async () => {
    const sql = `
      WITH visible_transfer_edges AS (
        SELECT
          edge.edge_id,
          edge.src_entity_id,
          edge.dst_entity_id,
          edge.txn_count,
          edge.amount_sum,
          edge.first_seen_at,
          edge.last_seen_at
        FROM analysis_relation_edge edge
        WHERE edge.relation_type = 'account_to_account_transfer'
        ORDER BY edge.amount_sum DESC NULLS LAST, edge.txn_count DESC NULLS LAST, edge.edge_id
        LIMIT 50
      ),
      edge_refs AS (
        SELECT CAST(ref_pk AS VARCHAR) AS edge_id, COUNT(*) AS ref_count
        FROM analysis_evidence_ref
        WHERE lower(ref_table) = 'analysis_relation_edge'
        GROUP BY CAST(ref_pk AS VARCHAR)
      ),
      evidence_table_rows AS (
        SELECT id, txn_id, txn_ts, acct_key, cp_key, amount
        FROM analysis_txn_detail_idx
        WHERE amount IS NOT NULL
        ORDER BY amount DESC NULLS LAST, id
        LIMIT 50
      ),
      chart_buckets AS (
        SELECT txn_day, acct_key, dc_val, txn_count, amt_sum
        FROM analysis_txn_daily_agg
        WHERE txn_count IS NOT NULL AND amt_sum IS NOT NULL
        ORDER BY amt_sum DESC NULLS LAST, txn_count DESC NULLS LAST, acct_key, txn_day
        LIMIT 50
      )
      SELECT
        (SELECT COUNT(*) FROM visible_transfer_edges) AS visible_transfer_edge_count,
        (SELECT SUM(CASE WHEN src.entity_id IS NULL OR dst.entity_id IS NULL THEN 1 ELSE 0 END)
          FROM visible_transfer_edges edge
          LEFT JOIN analysis_entity_node src ON src.entity_id = edge.src_entity_id
          LEFT JOIN analysis_entity_node dst ON dst.entity_id = edge.dst_entity_id) AS drawable_edge_missing_endpoint_count,
        (SELECT SUM(CASE WHEN edge.txn_count IS NULL OR edge.txn_count <= 0 OR edge.amount_sum IS NULL OR edge.amount_sum <= 0 THEN 1 ELSE 0 END)
          FROM visible_transfer_edges edge) AS drawable_edge_missing_metric_count,
        (SELECT SUM(CASE WHEN COALESCE(refs.ref_count, 0) <= 0 THEN 1 ELSE 0 END)
          FROM visible_transfer_edges edge
          LEFT JOIN edge_refs refs ON refs.edge_id = CAST(edge.edge_id AS VARCHAR)) AS drawable_edge_missing_evidence_ref_count,
        (SELECT SUM(COALESCE(refs.ref_count, 0))
          FROM visible_transfer_edges edge
          LEFT JOIN edge_refs refs ON refs.edge_id = CAST(edge.edge_id AS VARCHAR)) AS drawable_edge_evidence_ref_count,
        (SELECT COUNT(*) FROM evidence_table_rows) AS evidence_table_row_count,
        (SELECT SUM(CASE WHEN id IS NULL OR amount IS NULL OR amount <= 0 OR acct_key IS NULL OR acct_key = '' OR txn_ts IS NULL THEN 1 ELSE 0 END)
          FROM evidence_table_rows) AS evidence_table_missing_core_field_count,
        (SELECT COUNT(*) FROM chart_buckets) AS chart_bucket_count,
        (SELECT SUM(CASE WHEN txn_day IS NULL OR acct_key IS NULL OR acct_key = '' OR dc_val IS NULL OR dc_val = '' OR txn_count IS NULL OR amt_sum IS NULL THEN 1 ELSE 0 END)
          FROM chart_buckets) AS chart_bucket_missing_core_field_count
    `;
    const oracle = runDuckdbOracle(caseDb, sql, { rowLimit: 1 }).records[0] || {};
    const { envelope, response } = await callTool(runtime, "run_case_sql", {
      case_id: caseId,
      purpose: "oracle visible graph/table/chart artifact support contract",
      sql,
      row_limit: 1,
      result_mode: "aggregate"
    }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error(`unexpected artifact contract status: ${envelope.status}/${response.status}`);
    const row = objectOf(arrayOf(response.data?.records)[0]);
    const fields = [
      "visible_transfer_edge_count",
      "drawable_edge_missing_endpoint_count",
      "drawable_edge_missing_metric_count",
      "drawable_edge_missing_evidence_ref_count",
      "drawable_edge_evidence_ref_count",
      "evidence_table_row_count",
      "evidence_table_missing_core_field_count",
      "chart_bucket_count",
      "chart_bucket_missing_core_field_count"
    ];
    compareRecords(row, oracle, fields);
    const visibleTransferEdgeCount = Number(row.visible_transfer_edge_count || 0);
    const requiredPositive = [
      "evidence_table_row_count",
      "chart_bucket_count"
    ];
    for (const field of requiredPositive) {
      if (Number(row[field] || 0) <= 0) throw new Error(`${field} must be positive for visible artifact support`);
    }
    const zeroGapFields = [
      "drawable_edge_missing_endpoint_count",
      "drawable_edge_missing_metric_count",
      "drawable_edge_missing_evidence_ref_count",
      "evidence_table_missing_core_field_count",
      "chart_bucket_missing_core_field_count"
    ];
    for (const field of zeroGapFields) {
      if (Number(row[field] || 0) !== 0) throw new Error(`visible artifact contract has non-zero ${field}: ${row[field]}`);
    }
    if (visibleTransferEdgeCount > 0 && Number(row.drawable_edge_evidence_ref_count || 0) <= 0) {
      throw new Error("drawable fund-flow edges must carry evidence refs when graph edges exist");
    }
    if (response.data?.validation_state?.readonly !== true) throw new Error("artifact contract readonly validation missing");
    if (!response.data?.evidence_card?.source_hash) throw new Error("artifact contract evidence source_hash missing");
    return {
      summary: visibleTransferEdgeCount > 0
        ? `${row.visible_transfer_edge_count} drawable fund-flow edges, ${row.evidence_table_row_count} evidence rows, and ${row.chart_bucket_count} chart buckets matched DuckDB with zero support gaps`
        : `fund-flow graph edges are unavailable for this case and must be presented as a visual-evidence boundary; ${row.evidence_table_row_count} evidence rows and ${row.chart_bucket_count} chart buckets matched DuckDB`,
      row: normalizeRecord(row),
      source_hash: response.data.source_hash
    };
  }));

  results.push(await runCheck("count_case_rows_matches_duckdb", async () => {
    const checked = [];
    for (const table of coreTables) {
      const { envelope, response } = await callTool(runtime, "count_case_rows", { case_id: caseId, table_name: table }, runtimeArgs);
      if (envelope.status !== "ok" || response.status !== "ok") throw new Error(`${table} unexpected status`);
      if (Number(response.data?.row_count) !== Number(counts.get(table))) {
        throw new Error(`${table} count mismatch: MCP=${response.data?.row_count}, DuckDB=${counts.get(table)}`);
      }
      checked.push({ table, row_count: response.data.row_count });
    }
    return { summary: `${checked.length} core table counts matched`, checked };
  }));

  results.push(await runCheck("rank_counterparties_top10_visible_matches_duckdb", async () => {
    if (!allowedTables.includes("analysis_txn_detail_idx")) throw new Error("analysis_txn_detail_idx missing");
    const holder = text(runDuckdbOracle(caseDb, `
      SELECT account_open_name AS holder_name
      FROM analysis_txn_detail_idx
      WHERE account_open_name IS NOT NULL AND TRIM(account_open_name) <> ''
      GROUP BY account_open_name
      ORDER BY SUM(amount) DESC, COUNT(*) DESC, account_open_name
      LIMIT 1
    `, { rowLimit: 1 }).records[0]?.holder_name);
    if (!holder) throw new Error("no holder seed available for top10 ranking oracle");
    const expectedRows = runDuckdbOracle(caseDb, `
      WITH grouped AS (
        SELECT
          COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '__unknown__') AS counterparty_key,
          COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS display_name,
          MIN(counterparty_acct) AS counterparty_account,
          'name' AS counterparty_group_mode,
          COUNT(DISTINCT acct_key) AS source_account_count,
          COUNT(*) AS txn_count,
          SUM(CASE WHEN dc_val = '进' THEN amount ELSE 0 END) AS inflow_total,
          SUM(CASE WHEN dc_val = '出' THEN amount ELSE 0 END) AS outflow_total,
          SUM(amount) AS turnover_total,
          SUM(CASE WHEN dc_val = '进' THEN amount ELSE -amount END) AS net_flow,
          MAX(amount) AS max_single_amount,
          MIN(txn_time) AS first_txn_at,
          MAX(txn_time) AS last_txn_at
        FROM analysis_txn_detail_idx
        WHERE account_open_name = ${sqlString(holder)}
          AND dc_val = '出'
          AND COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') <> '空户名'
          AND COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') <> account_open_name
        GROUP BY counterparty_key, display_name
      )
      SELECT
        ROW_NUMBER() OVER (ORDER BY outflow_total DESC, txn_count DESC) AS rank,
        *
      FROM grouped
      ORDER BY outflow_total DESC, txn_count DESC
      LIMIT 10
    `, { rowLimit: 10 }).records;
    if (expectedRows.length < 10) throw new Error(`top10 oracle requires at least 10 rows, got ${expectedRows.length}`);
    const { envelope, response } = await callTool(runtime, "rank_counterparties", {
      case_id: caseId,
      holder_name: holder,
      direction_mode: "out",
      metric: "outflow",
      success_filter: "all",
      cash_filter: "all",
      counterparty_group_mode: "name",
      limit: 10
    }, runtimeArgs);
    if (response.status !== "ok") throw new Error(`unexpected rank status: ${envelope.status}/${response.status}`);
    const actualRows = arrayOf(response.data?.rankings);
    if (actualRows.length !== 10) throw new Error(`rank_counterparties returned ${actualRows.length} rows instead of 10`);
    if (Number(response.data?.requested_limit || 0) !== 10 || Number(response.data?.returned_count || 0) !== 10) {
      throw new Error(`ranking limit metadata mismatch: requested=${response.data?.requested_limit}, returned=${response.data?.returned_count}`);
    }
    compareRecordArrays(actualRows, expectedRows, [
      "rank",
      "display_name",
      "txn_count",
      "outflow_total",
      "turnover_total",
      "first_txn_at",
      "last_txn_at"
    ]);
    const toolText = compactToolText(envelope, {});
    const rankMarkers = [...toolText.matchAll(/第\s+\d+/gu)].length;
    if (!toolText.includes("第 10") || rankMarkers < 10) {
      throw new Error(`visible compact text did not expose all top10 rows; rank markers=${rankMarkers}; preview=${toolText.slice(0, 1200)}`);
    }
    const { response: defaultResponse } = await callTool(runtime, "rank_counterparties", {
      case_id: caseId,
      holder_name: holder,
      direction_mode: "out",
      metric: "outflow",
      success_filter: "all",
      cash_filter: "all",
      limit: 10
    }, runtimeArgs);
    if (defaultResponse.status !== "ok") throw new Error(`unexpected default rank status: ${defaultResponse.status}`);
    if (text(defaultResponse.data?.counterparty_group_mode) !== "name") {
      throw new Error(`default counterparty_group_mode must be name, got ${defaultResponse.data?.counterparty_group_mode}`);
    }
    compareRecordArrays(arrayOf(defaultResponse.data?.rankings), expectedRows, [
      "rank",
      "display_name",
      "txn_count",
      "outflow_total",
      "turnover_total",
      "first_txn_at",
      "last_txn_at"
    ]);
    return {
      summary: `Top 10 outflow counterparties for ${holder} matched DuckDB, defaulted to name grain, and compact text exposed rank 1-10`,
      holder_name: holder,
      top10_hash: stableHash(actualRows.map((row) => [row.rank, row.display_name, row.outflow_total, row.txn_count]))
    };
  }));

  results.push(await runCheck("case_project_title_fragment_does_not_filter_counterparty_rank", async () => {
    const caseTitleFragment = text(path.basename(options.caseProjectRoot || projectContext?.workspace_root || "")).replace(/(?:资金)?案件(?:项目)?分析$/u, "");
    if (!caseTitleFragment) throw new Error("case project title fragment unavailable");
    const expectedRows = runDuckdbOracle(caseDb, `
      WITH grouped AS (
        SELECT
          COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '__unknown__') AS counterparty_key,
          COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS display_name,
          MIN(counterparty_acct) AS counterparty_account,
          'name' AS counterparty_group_mode,
          COUNT(DISTINCT acct_key) AS source_account_count,
          COUNT(*) AS txn_count,
          SUM(CASE WHEN dc_val = '进' THEN amount ELSE 0 END) AS inflow_total,
          SUM(CASE WHEN dc_val = '出' THEN amount ELSE 0 END) AS outflow_total,
          SUM(amount) AS turnover_total,
          SUM(CASE WHEN dc_val = '进' THEN amount ELSE -amount END) AS net_flow,
          MAX(amount) AS max_single_amount,
          MIN(txn_time) AS first_txn_at,
          MAX(txn_time) AS last_txn_at
        FROM analysis_txn_detail_idx
        WHERE dc_val = '出'
          AND COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') <> '空户名'
          AND COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') <> COALESCE(NULLIF(TRIM(account_open_name), ''), '__source__')
        GROUP BY counterparty_key, display_name
      )
      SELECT
        ROW_NUMBER() OVER (ORDER BY outflow_total DESC, txn_count DESC) AS rank,
        *
      FROM grouped
      ORDER BY outflow_total DESC, txn_count DESC
      LIMIT 10
    `, { rowLimit: 10 }).records;
    const { response } = await callTool(runtime, "rank_counterparties", {
      case_id: caseId,
      holder_name: caseTitleFragment,
      match_mode: "contains",
      direction_mode: "out",
      metric: "outflow",
      success_filter: "all",
      cash_filter: "all",
      limit: 10
    }, runtimeArgs);
    if (response.status !== "ok") throw new Error(`unexpected case-title guard rank status: ${response.status}`);
    compareRecordArrays(arrayOf(response.data?.rankings), expectedRows, [
      "rank",
      "display_name",
      "txn_count",
      "outflow_total",
      "turnover_total",
      "first_txn_at",
      "last_txn_at"
    ]);
    return {
      summary: `Case title fragment ${caseTitleFragment} was treated as current-case scope, not a holder filter`,
      case_title_fragment: caseTitleFragment,
      top10_hash: stableHash(expectedRows.map((row) => [row.rank, row.display_name, row.outflow_total, row.txn_count]))
    };
  }));

  results.push(await runCheck("flow_intent_overrides_turnover_counterparty_rank", async () => {
    const expectedRows = runDuckdbOracle(caseDb, `
      WITH grouped AS (
        SELECT
          COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '__unknown__') AS counterparty_key,
          COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS display_name,
          MIN(counterparty_acct) AS counterparty_account,
          'name' AS counterparty_group_mode,
          COUNT(DISTINCT acct_key) AS source_account_count,
          COUNT(*) AS txn_count,
          SUM(CASE WHEN dc_val = '进' THEN amount ELSE 0 END) AS inflow_total,
          SUM(CASE WHEN dc_val = '出' THEN amount ELSE 0 END) AS outflow_total,
          SUM(amount) AS turnover_total,
          SUM(CASE WHEN dc_val = '进' THEN amount ELSE -amount END) AS net_flow,
          MAX(amount) AS max_single_amount,
          MIN(txn_time) AS first_txn_at,
          MAX(txn_time) AS last_txn_at
        FROM analysis_txn_detail_idx
        WHERE dc_val = '出'
          AND COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') <> '空户名'
          AND COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') <> COALESCE(NULLIF(TRIM(account_open_name), ''), '__source__')
        GROUP BY counterparty_key, display_name
      )
      SELECT
        ROW_NUMBER() OVER (ORDER BY outflow_total DESC, txn_count DESC) AS rank,
        *
      FROM grouped
      ORDER BY outflow_total DESC, txn_count DESC
      LIMIT 10
    `, { rowLimit: 10 }).records;
    const wrongMetricArgs = {
      case_id: caseId,
      question: "当前案件分析中资金流向前十位对手方是谁？",
      metric: "turnover",
      top_n: 10
    };
    const { response: directResponse } = await callTool(runtime, "rank_counterparties", wrongMetricArgs, runtimeArgs);
    if (directResponse.status !== "ok") throw new Error(`unexpected direct flow-intent rank status: ${directResponse.status}`);
    if (text(directResponse.data?.metric) !== "outflow" || text(directResponse.data?.direction_mode) !== "out") {
      throw new Error(`direct flow intent did not normalize metric/direction: metric=${directResponse.data?.metric}, direction=${directResponse.data?.direction_mode}`);
    }
    compareRecordArrays(arrayOf(directResponse.data?.rankings), expectedRows, [
      "rank",
      "display_name",
      "txn_count",
      "outflow_total",
      "turnover_total",
      "first_txn_at",
      "last_txn_at"
    ]);

    const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-funds-oracle-"));
    const threadId = "thr_oracle_flow_intent";
    const turnId = "turn_oracle_flow_intent";
    const threadDir = path.join(tempRoot, "threads", threadId);
    fs.mkdirSync(threadDir, { recursive: true });
    fs.writeFileSync(path.join(threadDir, "events.jsonl"), `${JSON.stringify({
      kind: "item_created",
      threadId,
      turnId,
      item: {
        kind: "user_message",
        text: "当前案件分析中资金流向前十位对手方是谁？"
      }
    })}\n`);
    const previousDataDir = process.env.ANALYTIX_DATA_DIR;
    process.env.ANALYTIX_DATA_DIR = tempRoot;
    try {
      const contextRuntime = runtimeForCase(caseId, caseDb, options.caseProjectRoot || projectContext?.workspace_root);
      const contextRuntimeArgs = {
        ...runtimeArgs,
        threadId,
        turnId
      };
      const { response: contextResponse } = await callTool(contextRuntime, "rank_counterparties", {
        case_id: caseId,
        metric: "turnover",
        top_n: 10
      }, contextRuntimeArgs);
      if (contextResponse.status !== "ok") throw new Error(`unexpected context flow-intent rank status: ${contextResponse.status}`);
      if (text(contextResponse.data?.metric) !== "outflow" || text(contextResponse.data?.direction_mode) !== "out") {
        throw new Error(`context flow intent did not normalize metric/direction: metric=${contextResponse.data?.metric}, direction=${contextResponse.data?.direction_mode}`);
      }
      compareRecordArrays(arrayOf(contextResponse.data?.rankings), expectedRows, [
        "rank",
        "display_name",
        "txn_count",
        "outflow_total",
        "turnover_total",
        "first_txn_at",
        "last_txn_at"
      ]);
    } finally {
      if (previousDataDir === undefined) delete process.env.ANALYTIX_DATA_DIR;
      else process.env.ANALYTIX_DATA_DIR = previousDataDir;
      fs.rmSync(tempRoot, { recursive: true, force: true });
    }
    return {
      summary: "Flow/destination front-door intent normalized turnover tool args to outflow Top10",
      top10_hash: stableHash(expectedRows.map((row) => [row.rank, row.display_name, row.outflow_total, row.txn_count]))
    };
  }));

  results.push(await runCheck("destination_card_top10_survives_self_counterparty_filter", async () => {
    const holder = text(runDuckdbOracle(caseDb, `
      SELECT account_open_name AS holder_name
      FROM analysis_txn_detail_idx
      WHERE account_open_name IS NOT NULL AND TRIM(account_open_name) <> ''
      GROUP BY account_open_name
      ORDER BY SUM(amount) DESC, COUNT(*) DESC, account_open_name
      LIMIT 1
    `, { rowLimit: 1 }).records[0]?.holder_name);
    if (!holder) throw new Error("no holder seed available for destination top10 oracle");
    const { response } = await callTool(runtime, "rank_counterparties", {
      case_id: caseId,
      holder_name: holder,
      direction_mode: "out",
      metric: "outflow",
      success_filter: "all",
      cash_filter: "all",
      counterparty_group_mode: "name",
      limit: 20
    }, runtimeArgs);
    if (response.status !== "ok") throw new Error(`unexpected rank status: ${response.status}`);
    const { buildDestinationOutflowDiagnosticCard } = createDestinationDiagnosticRuntime({ moneyText });
    const card = buildDestinationOutflowDiagnosticCard({
      caseId,
      holderName: holder,
      topN: 10,
      traceResult: { data: { scope_stats: { txn_count: 0, outflow_total: 0 } } },
      counterpartyRankResult: response,
      calls: [response]
    });
    const terminalFacts = arrayOf(card.facts).filter((fact) => text(fact.fact_id).startsWith("destination.terminal.rank"));
    if (terminalFacts.length !== 10) {
      throw new Error(`destination card returned ${terminalFacts.length} terminal facts after self-counterparty filtering instead of 10`);
    }
    if (!text(terminalFacts[9].answer_text || terminalFacts[9].answerText).includes("10.")) {
      throw new Error("destination card tenth fact is missing visible rank 10 label");
    }
    return {
      summary: `Destination card returned 10 non-self terminal facts for ${holder} after fetching buffered rank rows`,
      holder_name: holder,
      tenth_fact: text(terminalFacts[9].answer_text || terminalFacts[9].answerText)
    };
  }));

  results.push(await runCheck("top_outflows_payload_respects_requested_topn", async () => {
    const topOutflows = Array.from({ length: 12 }, (_item, index) => ({
      rank: index + 1,
      seed_txn: {
        txn_id: `oracle-top-outflow-${index + 1}`,
        amount: 1000 - index,
        account_open_name: "oracle holder",
        counterparty_name: `oracle counterparty ${index + 1}`,
        txn_time: `2026-06-${String(index + 1).padStart(2, "0")}`
      },
      terminal_category: "direct_counterparty",
      terminal_category_label: "直接对手",
      downstream_candidates: []
    }));
    const compact = compactToolPayloadForAgent({
      response: {
        skill_id: "trace_subject_top_outflows",
        status: "ok",
        data: {
          case_id: caseId,
          top_n: 10,
          requested_limit: 10,
          scope_stats: { txn_count: 12, outflow_total: 12000 },
          top_outflows: topOutflows
        }
      }
    });
    const rows = arrayOf(compact.key_facts?.top_outflows);
    if (rows.length !== 10) {
      throw new Error(`top_outflows compact payload returned ${rows.length} rows instead of requested top_n=10`);
    }
    if (Number(rows[9]?.rank || 0) !== 10) {
      throw new Error("top_outflows compact payload lost the tenth rank row");
    }
    return {
      summary: "top_outflows key facts preserve the requested Top 10 rows instead of fixed 4/6/8 truncation",
      returned_count: rows.length,
      tenth_rank: rows[9]?.rank
    };
  }));

  results.push(await runCheck("trace_subject_top_outflows_local_fallback_matches_duckdb", async () => {
    const holder = text(runDuckdbOracle(caseDb, `
      SELECT account_open_name AS holder_name
      FROM analysis_txn_detail_idx
      WHERE account_open_name IS NOT NULL AND TRIM(account_open_name) <> ''
      GROUP BY account_open_name
      ORDER BY SUM(amount) DESC, COUNT(*) DESC, account_open_name
      LIMIT 1
    `, { rowLimit: 1 }).records[0]?.holder_name);
    if (!holder) throw new Error("no holder seed available for trace fallback oracle");
    const expectedScope = runDuckdbOracle(caseDb, `
      SELECT
        COUNT(*) AS txn_count,
        COUNT(DISTINCT acct_key) AS source_account_count,
        COUNT(DISTINCT COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), NULLIF(TRIM(counterparty_acct), ''), '__unknown__')) AS counterparty_count,
        SUM(amount) AS outflow_total,
        MIN(txn_time) AS first_txn_at,
        MAX(txn_time) AS last_txn_at,
        SUM(CASE WHEN counterparty_name IS NULL OR TRIM(counterparty_name) = '' THEN 1 ELSE 0 END) AS missing_counterparty_name_count,
        SUM(CASE WHEN counterparty_acct IS NULL OR TRIM(counterparty_acct) = '' THEN 1 ELSE 0 END) AS missing_counterparty_account_count
      FROM analysis_txn_detail_idx
      WHERE account_open_name = ${sqlString(holder)}
        AND dc_val = '出'
        AND amount IS NOT NULL
    `, { rowLimit: 1 }).records[0];
    const expectedRows = runDuckdbOracle(caseDb, `
      WITH scoped AS (
        SELECT
          id,
          txn_id,
          txn_time,
          txn_ts,
          acct_key,
          account_open_name,
          cp_key,
          counterparty_acct,
          COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), NULLIF(TRIM(counterparty_acct), ''), '__unknown__') AS counterparty_key,
          COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS display_name,
          amount
        FROM analysis_txn_detail_idx
        WHERE account_open_name = ${sqlString(holder)}
          AND dc_val = '出'
          AND amount IS NOT NULL
      ),
      account_lookup AS (
        SELECT DISTINCT COALESCE(NULLIF(TRIM(acct_no), ''), NULLIF(TRIM(card_no), ''), acct_key) AS account_lookup
        FROM analysis_txn_detail_idx
        WHERE COALESCE(NULLIF(TRIM(acct_no), ''), NULLIF(TRIM(card_no), ''), acct_key) IS NOT NULL
      ),
      grouped AS (
        SELECT
          counterparty_key,
          display_name,
          MIN(counterparty_acct) AS counterparty_account,
          COUNT(*) AS txn_count,
          COUNT(DISTINCT acct_key) AS source_account_count,
          SUM(amount) AS outflow_total,
          MAX(amount) AS max_single_amount,
          MIN(txn_time) AS first_txn_at,
          MAX(txn_time) AS last_txn_at,
          MAX(CASE WHEN counterparty_acct IN (SELECT account_lookup FROM account_lookup) THEN 1 ELSE 0 END) AS target_account_in_case
        FROM scoped
        GROUP BY counterparty_key, display_name
      )
      SELECT
        ROW_NUMBER() OVER (ORDER BY outflow_total DESC, txn_count DESC) AS rank,
        *
      FROM grouped
      ORDER BY outflow_total DESC, txn_count DESC
      LIMIT 10
    `, { rowLimit: 10 }).records;
    if (expectedRows.length < 10) throw new Error(`trace fallback oracle requires at least 10 top outflows, got ${expectedRows.length}`);
    const { envelope, response } = await callTool(fallbackRuntime, "trace_subject_top_outflows", {
      case_id: caseId,
      holder_name: holder,
      top_n: 10
    }, runtimeArgs);
    if (envelope.status !== "ok" || envelope.backend_status !== "local_duckdb_fallback" || response.status !== "ok") {
      throw new Error(`unexpected trace fallback status: envelope=${envelope.status}/${envelope.backend_status}, response=${response.status}`);
    }
    const actualRows = arrayOf(response.data?.top_outflows);
    if (actualRows.length !== 10) throw new Error(`trace fallback returned ${actualRows.length} rows instead of 10`);
    compareRecords(response.data?.scope_stats, expectedScope, [
      "txn_count",
      "source_account_count",
      "counterparty_count",
      "outflow_total",
      "first_txn_at",
      "last_txn_at",
      "missing_counterparty_name_count",
      "missing_counterparty_account_count"
    ]);
    compareRecordArrays(actualRows, expectedRows, [
      "rank",
      "counterparty_key",
      "display_name",
      "counterparty_account",
      "txn_count",
      "source_account_count",
      "outflow_total",
      "max_single_amount",
      "first_txn_at",
      "last_txn_at"
    ]);
    for (let index = 0; index < expectedRows.length; index += 1) {
      const actualTarget = actualRows[index]?.target_account_in_case === true;
      const expectedTarget = Number(expectedRows[index]?.target_account_in_case || 0) > 0;
      if (actualTarget !== expectedTarget) {
        throw new Error(`target_account_in_case mismatch at rank ${index + 1}: MCP=${actualTarget}, DuckDB=${expectedTarget}`);
      }
    }
    return {
      summary: `trace_subject_top_outflows local fallback returned DuckDB-matched Top 10 facts for ${holder}`,
      holder_name: holder,
      top10_hash: stableHash(actualRows.map((row) => [row.rank, row.display_name, row.outflow_total, row.txn_count])),
      self_review_count: actualRows.filter((row) => text(row.terminal_category) === "self_counterparty_review").length
    };
  }));

  results.push(await runCheck("trace_fund_next_hop_local_fallback_matches_duckdb", async () => {
    const seed = runDuckdbOracle(caseDb, `
      WITH account_lookup AS (
        SELECT DISTINCT COALESCE(NULLIF(TRIM(acct_no), ''), NULLIF(TRIM(card_no), ''), acct_key) AS account_lookup
        FROM analysis_txn_detail_idx
      )
      SELECT
        t.txn_id,
        t.txn_ts,
        t.txn_time,
        t.account_open_name,
        t.counterparty_name,
        t.counterparty_acct,
        t.amount
      FROM analysis_txn_detail_idx t
      JOIN account_lookup a ON t.counterparty_acct = a.account_lookup
      WHERE t.dc_val = '出'
        AND t.amount IS NOT NULL
        AND t.counterparty_acct IS NOT NULL
        AND TRIM(t.counterparty_acct) <> ''
      ORDER BY t.amount DESC, t.txn_ts DESC, t.id DESC
      LIMIT 1
    `, { rowLimit: 1 }).records[0];
    const seedTxnId = text(seed?.txn_id);
    const receiverAccount = text(seed?.counterparty_acct);
    const seedTxnTs = text(seed?.txn_ts);
    if (!seedTxnId || !receiverAccount || !seedTxnTs) throw new Error("no in-case next-hop seed available");
    const expectedRows = runDuckdbOracle(caseDb, `
      SELECT
        ROW_NUMBER() OVER (ORDER BY txn_ts ASC, amount DESC, id DESC) AS rank,
        txn_id,
        txn_time,
        acct_key AS from_account_key,
        account_open_name AS from_holder_name,
        cp_key AS counterparty_key,
        COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS counterparty_name,
        counterparty_acct AS counterparty_account,
        amount,
        summary,
        txn_type,
        file_id
      FROM analysis_txn_detail_idx
      WHERE (acct_key = ${sqlString(receiverAccount)} OR acct_no = ${sqlString(receiverAccount)} OR card_no = ${sqlString(receiverAccount)})
        AND dc_val = '出'
        AND txn_ts >= TIMESTAMP ${sqlString(seedTxnTs)}
        AND txn_ts <= TIMESTAMP ${sqlString(seedTxnTs)} + INTERVAL 43200 MINUTE
        AND amount IS NOT NULL
      ORDER BY txn_ts ASC, amount DESC, id DESC
      LIMIT 10
    `, { rowLimit: 10 }).records;
    if (!expectedRows.length) throw new Error(`next-hop oracle seed ${seedTxnId} has no visible downstream rows`);
    const { envelope, response } = await callTool(fallbackRuntime, "trace_fund_next_hop", {
      case_id: caseId,
      seed_txn_id: seedTxnId,
      top_n: 10,
      time_window_minutes: 43200
    }, runtimeArgs);
    if (envelope.status !== "ok" || envelope.backend_status !== "local_duckdb_fallback" || response.status !== "ok") {
      throw new Error(`unexpected next-hop fallback status: envelope=${envelope.status}/${envelope.backend_status}, response=${response.status}`);
    }
    const actualRows = arrayOf(response.data?.next_hops);
    compareRecordArrays(actualRows, expectedRows, [
      "rank",
      "txn_id",
      "txn_time",
      "from_account_key",
      "from_holder_name",
      "counterparty_key",
      "counterparty_name",
      "counterparty_account",
      "amount",
      "summary",
      "txn_type",
      "file_id"
    ]);
    return {
      summary: `trace_fund_next_hop local fallback matched DuckDB for seed ${seedTxnId}`,
      seed_txn_id: seedTxnId,
      receiver_account: receiverAccount,
      returned_count: actualRows.length,
      next_hop_hash: stableHash(actualRows.map((row) => [row.rank, row.txn_id, row.amount, row.counterparty_name]))
    };
  }));

  results.push(await runCheck("build_fund_flow_graph_local_fallback_uses_duckdb_trace", async () => {
    const holder = text(runDuckdbOracle(caseDb, `
      SELECT account_open_name AS holder_name
      FROM analysis_txn_detail_idx
      WHERE account_open_name IS NOT NULL AND TRIM(account_open_name) <> ''
      GROUP BY account_open_name
      ORDER BY SUM(amount) DESC, COUNT(*) DESC, account_open_name
      LIMIT 1
    `, { rowLimit: 1 }).records[0]?.holder_name);
    if (!holder) throw new Error("no holder seed available for local graph fallback oracle");
    const { envelope } = await callTool(fallbackRuntime, "build_fund_flow_graph", {
      case_id: caseId,
      holder_name: holder,
      top_n: 10
    }, runtimeArgs);
    if (!["ok", "partial"].includes(text(envelope.status)) || envelope.backend_status !== "local_duckdb_fallback") {
      throw new Error(`unexpected graph fallback status: ${envelope.status}/${envelope.backend_status}`);
    }
    const flowGraph = objectOf(envelope.flow_graph || envelope.key_facts?.flow_graph);
    const edges = arrayOf(flowGraph.edges);
    if (edges.length !== 10) throw new Error(`graph fallback returned ${edges.length} edges instead of 10`);
    const queryIds = arrayOf(envelope.evidence_refs?.query_ids).map(text).filter(Boolean);
    if (!queryIds.length) throw new Error("graph fallback did not carry trace query ids");
    const supportedCount = edges.filter((edge) => text(edge.edge_status) === "supported").length;
    const needsReviewCount = edges.filter((edge) => text(edge.edge_status) !== "supported").length;
    if (supportedCount + needsReviewCount !== edges.length) throw new Error("graph fallback edge status counts do not close");
    return {
      summary: `build_fund_flow_graph local fallback produced 10 DuckDB-backed graph edges for ${holder}`,
      holder_name: holder,
      supported_count: supportedCount,
      needs_review_count: needsReviewCount,
      graph_hash: stableHash(edges.map((edge) => [edge.rank, edge.from_label, edge.to_label, edge.amount?.yuan, edge.edge_status]))
    };
  }));

  results.push(await runCheck("empty_feature_tables_are_reported_as_boundaries", async () => {
    const featureTables = [
      "analysis_txn_feature",
      "analysis_account_feature"
    ].filter((table) => allowedTables.includes(table));
    const checked = [];
    for (const table of featureTables) {
      const { envelope, response } = await callTool(runtime, "count_case_rows", { case_id: caseId, table_name: table }, runtimeArgs);
      if (envelope.status !== "ok" || response.status !== "ok") throw new Error(`${table} unexpected status`);
      if (Number(response.data?.row_count) !== Number(counts.get(table))) {
        throw new Error(`${table} count mismatch: MCP=${response.data?.row_count}, DuckDB=${counts.get(table)}`);
      }
      checked.push({ table, row_count: Number(response.data?.row_count || 0) });
    }
    const emptyTables = checked.filter((row) => Number(row.row_count || 0) === 0).map((row) => row.table);
    const keyNodeRows = Number(counts.get("analysis_key_node_features") || 0);
    const accountRoleEvidenceUnavailable = emptyTables.length && keyNodeRows <= 0;
    const { envelope, response } = await callTool(runtime, "case_sql_recipes", {
      case_id: caseId,
      category: "account_role",
      limit: 50
    }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error("unexpected account-role recipe status");
    const accountRoleRecipe = arrayOf(response.data?.recipes).find((recipe) => text(recipe.recipe_id) === "account_role_candidate_classification");
    if (!accountRoleRecipe) throw new Error("account role recipe missing");
    if (!arrayOf(accountRoleRecipe.preferred_tables).includes("analysis_key_node_features")) {
      throw new Error("account role recipe must prefer analysis_key_node_features when feature tables are empty");
    }
    return {
      summary: emptyTables.length
        ? `${emptyTables.join(", ")} empty and treated as evidence boundaries; account-role fallback has ${keyNodeRows} rows${accountRoleEvidenceUnavailable ? " (account-role evidence unavailable for this case)" : ""}`
        : `${checked.length} feature tables are populated and counts matched`,
      checked,
      key_node_feature_rows: keyNodeRows,
      empty_feature_tables: emptyTables,
      account_role_evidence_unavailable: accountRoleEvidenceUnavailable
    };
  }));

  results.push(await runCheck("case_sql_recipes_cover_investigation_scenarios", async () => {
    const { envelope, response } = await callTool(runtime, "case_sql_recipes", {
      case_id: caseId,
      limit: 100
    }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error("unexpected recipe status");
    const recipes = arrayOf(response.data?.recipes);
    const recipeIds = new Set(recipes.map((recipe) => text(recipe.recipe_id)));
    const missing = INVESTIGATION_SCENARIO_RECIPES.filter((recipeId) => !recipeIds.has(recipeId));
    if (missing.length) throw new Error(`scenario recipes missing: ${missing.join(", ")}`);
    for (const recipe of recipes.filter((item) => INVESTIGATION_SCENARIO_RECIPES.includes(text(item.recipe_id)))) {
      if (!arrayOf(recipe.preferred_tables).length) throw new Error(`${recipe.recipe_id} missing preferred_tables`);
      if (!arrayOf(recipe.required_preflight).length) throw new Error(`${recipe.recipe_id} missing required_preflight`);
      if (!text(recipe.validation_gate)) throw new Error(`${recipe.recipe_id} missing validation_gate`);
      if (!text(recipe.sql_template).toLowerCase().includes("select")) throw new Error(`${recipe.recipe_id} missing SQL template`);
    }
    return {
      summary: `${INVESTIGATION_SCENARIO_RECIPES.length} reviewed scenario recipes are available`,
      recipe_ids: INVESTIGATION_SCENARIO_RECIPES
    };
  }));

  results.push(await runCheck("run_case_sql_aggregate_matches_duckdb", async () => {
    const sql = "SELECT COUNT(*) AS row_count, SUM(amount) AS amount_sum FROM analysis_txn_detail_idx";
    const oracle = runDuckdbOracle(caseDb, sql, { rowLimit: 1 }).records[0];
    const { envelope, response } = await callTool(runtime, "run_case_sql", {
      case_id: caseId,
      purpose: "oracle aggregate over transaction detail index",
      sql,
      row_limit: 1,
      result_mode: "aggregate"
    }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error(`unexpected status: ${envelope.status}/${response.status}`);
    const row = response.data?.records?.[0];
    compareRecords(row, oracle, ["row_count", "amount_sum"]);
    if (response.data?.validation_state?.readonly !== true) throw new Error("readonly validation_state missing");
    if (response.data?.validation_state?.cleaned_analysis_scope_only !== true) throw new Error("cleaned_analysis_scope_only validation missing");
    if (response.data?.validation_state?.sql_policy?.policy_engine !== "duckdb_extract_statement_and_explain_v2") {
      throw new Error("DuckDB parser/binder SQL policy missing from validation_state");
    }
    if (response.data?.sql_policy?.static_guard?.policy_engine !== "tokenized_duckdb_readonly_scope_guard_v3") {
      throw new Error("static read-only scope guard missing from sql_policy");
    }
    if (response.data?.sql_policy?.executed_user_sql !== false) {
      throw new Error("parser/binder guard must not execute user SQL");
    }
    if (!response.data?.evidence_card?.source_hash) throw new Error("evidence_card.source_hash missing");
    return {
      summary: "aggregate row_count/amount_sum matched DuckDB and carried evidence_card plus DuckDB parser/binder policy",
      row: normalizeRecord(row),
      source_hash: response.data.source_hash
    };
  }));

  results.push(await runCheck("run_case_sql_row_limit_and_truncation", async () => {
    const sql = "SELECT id, amount FROM analysis_txn_detail_idx ORDER BY id";
    const oracle = runDuckdbOracle(caseDb, sql, { rowLimit: 3 }).records;
    const { envelope, response } = await callTool(runtime, "run_case_sql", {
      case_id: caseId,
      purpose: "oracle bounded row limit over transaction detail index",
      sql,
      row_limit: 3,
      result_mode: "evidence_table"
    }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error("unexpected row-limit status");
    const records = arrayOf(response.data?.records);
    if (records.length !== 3) throw new Error(`row_limit not enforced: ${records.length}`);
    for (let index = 0; index < oracle.length; index += 1) {
      compareRecords(records[index], oracle[index], ["id", "amount"]);
    }
    if (response.data?.truncated !== true) throw new Error("truncated flag should be true for limited detail query");
    return { summary: "row_limit=3 matched first DuckDB rows and marked truncation", records: records.length };
  }));

  results.push(await runCheck("run_case_sql_supports_investigation_scenarios", async () => {
    const checked = [];
    for (const scenario of INVESTIGATION_SCENARIO_QUERIES) {
      const sql = text(scenario.sql);
      const oracle = runDuckdbOracle(caseDb, sql, {
        rowLimit: scenario.rowLimit,
        maxLimit: Math.max(100, scenario.rowLimit + 1)
      });
      const { envelope, response } = await callTool(runtime, "run_case_sql", {
        case_id: caseId,
        purpose: scenario.purpose,
        sql,
        row_limit: scenario.rowLimit,
        result_mode: scenario.resultMode
      }, runtimeArgs);
      if (envelope.status !== "ok" || response.status !== "ok") throw new Error(`${scenario.id} unexpected MCP status`);
      const actualRows = arrayOf(response.data?.records);
      const expectedRows = arrayOf(oracle.records);
      const fields = arrayOf(oracle.columns);
      compareRecordArrays(actualRows, expectedRows, fields);
      if (scenario.id === "fund_path_and_evidence_integrity") {
        const row = objectOf(actualRows[0]);
        const gapFields = [
          "missing_endpoint_edge_count",
          "missing_trace_support_count",
          "malformed_evidence_ref_count"
        ];
        for (const field of gapFields) {
          if (Number(row[field] || 0) !== 0) throw new Error(`${scenario.id} has non-zero ${field}: ${row[field]}`);
        }
      }
      if (response.data?.validation_state?.readonly !== true) throw new Error(`${scenario.id} readonly validation missing`);
      if (!response.data?.evidence_card?.source_hash) throw new Error(`${scenario.id} evidence source_hash missing`);
      if (response.data?.raw_rows_exposed !== false) throw new Error(`${scenario.id} raw row exposure flag missing`);
      checked.push({
        id: scenario.id,
        rows: actualRows.length,
        fields
      });
    }
    return {
      summary: `${checked.length} investigation scenarios matched DuckDB through run_case_sql`,
      checked
    };
  }));

  results.push(await runCheck("profile_case_schema_matches_column_stats", async () => {
    const sql = "SELECT COUNT(*) AS row_count, SUM(CASE WHEN amount IS NULL THEN 1 ELSE 0 END) AS null_count, MIN(amount) AS min_value, MAX(amount) AS max_value FROM analysis_txn_detail_idx";
    const oracle = runDuckdbOracle(caseDb, sql, { rowLimit: 1 }).records[0];
    const { envelope, response } = await callTool(runtime, "profile_case_schema", {
      case_id: caseId,
      tables: ["analysis_txn_detail_idx"],
      table_limit: 1,
      column_limit: 80
    }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error("unexpected profile status");
    const profile = arrayOf(response.data?.profiles)[0];
    const amount = arrayOf(profile?.columns).find((column) => text(column.column_name) === "amount");
    if (!amount) throw new Error("amount column profile missing");
    compareRecords(amount, oracle, ["row_count", "null_count", "min_value", "max_value"]);
    if (amount.value_samples_exposed !== false) throw new Error("profile exposed value samples");
    return { summary: "amount column row/null/min/max matched DuckDB without sample values", amount_profile: amount };
  }));

  results.push(await runCheck("preview_case_rows_is_filtered_limited_and_masked", async () => {
    const seed = runDuckdbOracle(caseDb, "SELECT amount FROM analysis_txn_detail_idx WHERE amount IS NOT NULL LIMIT 1", { rowLimit: 1 }).records[0];
    const amount = Number(seed?.amount);
    if (!Number.isFinite(amount)) throw new Error("no numeric amount seed available");
    const whereSql = `amount = ${amount}`;
    const { envelope, response } = await callTool(runtime, "preview_case_rows", {
      case_id: caseId,
      purpose: "oracle filtered sample privacy projection",
      table_name: "analysis_txn_detail_idx",
      columns: ["acct_key", "amount"],
      where_sql: whereSql,
      row_limit: 5
    }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error("unexpected preview status");
    const rows = arrayOf(response.data?.records);
    if (!rows.length || rows.length > 5) throw new Error(`preview row_count out of bounds: ${rows.length}`);
    if (response.data?.sample_only !== true || response.data?.sample_cannot_support_totals !== true) {
      throw new Error("preview sample boundary flags missing");
    }
    if (text(rows[0].acct_key) && !text(rows[0].acct_key).includes("*")) {
      throw new Error("sensitive acct_key was not masked");
    }
    for (const row of rows) {
      if (!equivalentValue(row.amount, amount)) throw new Error("preview where_sql amount filter not respected");
    }
    return { summary: "filtered preview stayed <=5 rows, preserved amount filter, masked acct_key", row_count: rows.length };
  }));

  results.push(await runCheck("explain_and_diagnose_do_not_return_case_facts", async () => {
    const explain = await callTool(runtime, "explain_case_sql", {
      case_id: caseId,
      purpose: "oracle explain count query",
      sql: "SELECT COUNT(*) AS row_count FROM analysis_txn_detail_idx"
    }, runtimeArgs);
    if (explain.envelope.status !== "ok" || explain.response.status !== "ok") throw new Error("unexpected explain status");
    if (!arrayOf(explain.response.data?.plan_rows).length) throw new Error("explain plan rows missing");
    if (explain.response.data?.validation_summary?.returns_detail_rows !== false) throw new Error("explain detail-row boundary missing");
    if (!arrayOf(explain.response.data?.sql_shape?.base_tables).includes("analysis_txn_detail_idx")) {
      throw new Error("explain sql_shape did not include base table");
    }
    if (!Object.prototype.hasOwnProperty.call(objectOf(explain.response.data?.slow_path_diagnostics), "slow_path_likely")) {
      throw new Error("explain slow_path_diagnostics missing");
    }
    if (explain.response.data?.sql_policy?.policy_engine !== "duckdb_extract_statement_and_explain_v2") {
      throw new Error("explain sql_policy did not include DuckDB parser/binder guard");
    }
    const diagnose = await callTool(runtime, "diagnose_case_sql", {
      case_id: caseId,
      purpose: "oracle invalid column diagnosis",
      sql: "SELECT not_a_column FROM analysis_txn_detail_idx"
    }, runtimeArgs);
    if (diagnose.envelope.status !== "ok" || diagnose.response.status !== "ok") throw new Error("unexpected diagnose status");
    if (diagnose.response.data?.execution_status !== "blocked") throw new Error("invalid column diagnosis should be blocked");
    if (!JSON.stringify(diagnose.response.data?.diagnostics || []).includes("not_a_column")) {
      throw new Error("diagnostics did not mention invalid column");
    }
    if (!arrayOf(diagnose.response.data?.diagnosis?.candidate_columns).some((column) => text(column.column_name) === "amount")) {
      throw new Error("diagnose candidate_columns did not include semantic amount column");
    }
    const diagnoseText = compactToolText(diagnose.envelope, {});
    if (!diagnoseText.includes("候选") && !diagnoseText.includes("candidate")) {
      throw new Error(`diagnose compact text did not expose repair candidates; preview=${diagnoseText.slice(0, 1000)}`);
    }
    for (const requiredIdentifier of ["analysis_txn_detail_idx", "account_open_name", "counterparty_name"]) {
      if (!diagnoseText.includes(requiredIdentifier)) {
        throw new Error(`diagnose compact text translated or omitted SQL-safe identifier ${requiredIdentifier}; preview=${diagnoseText.slice(0, 1000)}`);
      }
    }
    if (!arrayOf(diagnose.response.data?.sql_shape?.base_tables).includes("analysis_txn_detail_idx")) {
      throw new Error("diagnose sql_shape did not include base table");
    }
    const diagnoseAuditEvents = arrayOf(diagnose.response.data?.audit_events).map(objectOf);
    if (!diagnoseAuditEvents.some((event) => text(event.event) === "case_sql_diagnose:blocked" && text(event.fact_policy) === "diagnose_results_are_not_case_amount_or_flow_facts")) {
      throw new Error("diagnose audit event missing or fact policy unsafe");
    }
    return { summary: "explain returned plan only; diagnose blocked invalid column with diagnostics and audit event" };
  }));

  results.push(await runCheck("inspect_workbench_history_returns_safe_recent_digests", async () => {
    const { envelope, response } = await callTool(runtime, "inspect_workbench_history", {
      case_id: caseId,
      limit: 20
    }, runtimeArgs);
    if (envelope.status !== "ok" || response.status !== "ok") throw new Error("unexpected history status");
    if (response.data?.history_available !== true) throw new Error("history should be available after oracle tool calls");
    const entries = arrayOf(response.data?.entries);
    if (!entries.length) throw new Error("history entries missing");
    const skillIds = new Set(entries.map((entry) => text(entry.skill_id)));
    for (const requiredSkill of ["run_case_sql", "explain_case_sql", "diagnose_case_sql"]) {
      if (!skillIds.has(requiredSkill)) throw new Error(`history missing ${requiredSkill}`);
    }
    for (const entry of entries) {
      if (Object.prototype.hasOwnProperty.call(entry, "sql")) throw new Error("history leaked raw sql field");
      if (Object.prototype.hasOwnProperty.call(entry, "records")) throw new Error("history leaked records field");
      if (!text(entry.query_id)) throw new Error("history entry missing query_id");
      if (entry.query_digest && !/^[a-f0-9]{64}$/u.test(text(entry.query_digest))) throw new Error("history query_digest must be a complete SHA-256");
      if (!objectOf(entry.validation_state).execution_status) throw new Error("history validation_state missing execution_status");
      if (text(entry.skill_id) === "run_case_sql" && objectOf(entry.validation_state).parser_binder_validated !== true) {
        throw new Error("history run_case_sql entry missing parser/binder validation marker");
      }
    }
    const serialized = JSON.stringify(entries);
    if (serialized.includes("SELECT ") || serialized.includes(caseDb) || serialized.includes("/Users/")) {
      throw new Error("history leaked SQL text or local filesystem path");
    }
    return {
      summary: `${entries.length} safe workbench history digests returned without SQL text or rows`,
      skills: [...skillIds].sort()
    };
  }));

  results.push(await runCheck("workbench_security_blocks_write_raw_external_and_unfiltered_preview", async () => {
    const cases = [
      ["raw_table", "run_case_sql", { case_id: caseId, purpose: "block raw", sql: "SELECT COUNT(*) AS n FROM fc_transaction_raw" }],
      ["ddl", "run_case_sql", { case_id: caseId, purpose: "block ddl", sql: "DROP TABLE analysis_txn_detail_idx" }],
      ["external", "run_case_sql", { case_id: caseId, purpose: "block external", sql: "SELECT * FROM read_csv('/tmp/x.csv')" }],
      ["invalid_column_parser_guard", "run_case_sql", { case_id: caseId, purpose: "block invalid column before execution", sql: "SELECT not_a_column FROM analysis_txn_detail_idx" }],
      ["unfiltered_preview", "preview_case_rows", { case_id: caseId, purpose: "block unfiltered preview", table_name: "analysis_txn_detail_idx", where_sql: "1=1" }]
    ];
    const blocked = [];
    for (const [label, tool, args] of cases) {
      const { envelope } = await callTool(runtime, tool, args, runtimeArgs);
      if (envelope.status !== "blocked") throw new Error(`${label} was not blocked: ${envelope.status}`);
      blocked.push(label);
    }
    return { summary: `${blocked.length} unsafe workbench calls blocked before data return`, blocked };
  }));

  const failed = results.filter((result) => result.status === "failed");
  const findings = [
    ...failed.map((result) => ({
      severity: "p0",
      message: `${result.id}: ${result.error}`
    })),
    ...results.flatMap((result) => arrayOf(objectOf(result.details).findings).map((finding) => ({
      severity: text(finding.severity) || "advisory",
      message: `${result.id}: ${text(finding.message)}`
    }))).filter((finding) => finding.message)
  ];
  assertOracleCaseBindingUnchanged(binding);
  return {
    status: failed.length ? "failed" : "ok",
    generated_at: new Date().toISOString(),
    case_id: caseId,
    case_db: "[host-bound]",
    allowed_table_count: allowedTables.length,
    core_tables: Object.fromEntries(coreTables.map((table) => [table, counts.get(table)])),
    results,
    passed_count: results.filter((result) => result.status === "passed").length,
    failed_count: failed.length,
    dataset_snapshot_v2_ready: true,
    findings
  };
}

function boundedOracleSummary(payload) {
  const status = ["ok", "failed", "skipped"].includes(text(payload?.status))
    ? text(payload.status)
    : "failed";
  const boundedCount = (value) => Number.isSafeInteger(Number(value)) && Number(value) >= 0
    ? Number(value)
    : 0;
  return {
    status,
    allowed_table_count: boundedCount(payload?.allowed_table_count),
    passed_count: boundedCount(payload?.passed_count),
    failed_count: boundedCount(payload?.failed_count),
    finding_count: arrayOf(payload?.findings).length,
    dataset_snapshot_v2_ready: payload?.dataset_snapshot_v2_ready === true
  };
}

function runSecurityBoundarySelfTest() {
  const temporaryRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-funds-oracle-boundary-"));
  const root = fs.realpathSync(temporaryRoot);
  try {
    const workspace = path.join(root, "workspace");
    const dataHome = path.join(root, "data-analysis");
    const caseId = "case_oracle_boundary";
    const caseDb = path.join(dataHome, "cases", caseId, "case.duckdb");
    fs.mkdirSync(path.join(workspace, ".analytix"), { recursive: true });
    fs.mkdirSync(path.dirname(caseDb), { recursive: true });
    fs.writeFileSync(path.join(workspace, ".analytix", "case-project.json"), JSON.stringify({
      version: 1,
      workspaceRoot: workspace,
      caseId,
      source: "analytix-data-analysis",
      updatedAt: "2026-07-23T00:00:00.000Z"
    }));
    fs.writeFileSync(caseDb, "synthetic-oracle-boundary");
    const env = {
      ...process.env,
      ANALYTIX_DATA_ANALYSIS_DIR: dataHome
    };
    const options = {
      caseProjectRoot: workspace,
      caseDb,
      caseId,
      requireCaseDb: true
    };
    const binding = resolveOracleCaseBinding(options, env);
    assertOracleCaseBindingUnchanged(binding);

    const expectCode = (label, expectedCode, fn) => {
      try {
        fn();
      } catch (error) {
        if (text(error?.code) === expectedCode) return label;
        throw error;
      }
      throw new Error(`${label} did not fail closed`);
    };
    const cases = [
      expectCode("different database", "case_db_binding_mismatch", () => resolveOracleCaseBinding({
        ...options,
        caseDb: path.join(root, "other.duckdb")
      }, env)),
      expectCode("different case", "case_id_binding_mismatch", () => resolveOracleCaseBinding({
        ...options,
        caseId: "case_other_boundary"
      }, env))
    ];
    const hardlink = path.join(path.dirname(caseDb), "case-hardlink.duckdb");
    fs.linkSync(caseDb, hardlink);
    cases.push(expectCode("hard-linked database", "case_db_identity_invalid", () => resolveOracleCaseBinding(options, env)));
    fs.unlinkSync(hardlink);
    assertOracleCaseBindingUnchanged(resolveOracleCaseBinding(options, env));

    const summary = boundedOracleSummary({
      status: "failed",
      allowed_table_count: 0,
      passed_count: 0,
      failed_count: 1,
      findings: [{ severity: "p0", message: root }],
      dataset_snapshot_v2_ready: false,
      case_id: caseId,
      case_db: caseDb
    });
    if (JSON.stringify(summary).includes(root) || Object.hasOwn(summary, "case_id") || Object.hasOwn(summary, "case_db")) {
      throw new Error("bounded summary leaked case identity or local path");
    }
    return {
      status: "ok",
      checked_count: cases.length + 2,
      mismatch_rejected: true,
      hardlink_rejected: true,
      bounded_summary: true
    };
  } finally {
    fs.rmSync(temporaryRoot, { recursive: true, force: true });
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.selfTestSecurityBoundary) {
    const result = runSecurityBoundarySelfTest();
    console.log(JSON.stringify(result));
    return;
  }
  const payload = await runOracle(options);
  if (!options.noWrite) {
    const evidence = writeEvidence(options.outputDir, payload);
    payload.output = {
      output_dir: path.relative(REPO_ROOT, evidence.output_dir),
      json_path: path.relative(REPO_ROOT, evidence.json_path),
      markdown_path: path.relative(REPO_ROOT, evidence.markdown_path)
    };
  }
  if (options.summaryJson) {
    console.log(JSON.stringify(boundedOracleSummary(payload)));
  } else if (options.json) {
    console.log(JSON.stringify(payload, null, 2));
  } else {
    console.log(`MCP return oracle: ${payload.status}`);
    console.log(`  passed: ${payload.passed_count || 0}`);
    console.log(`  failed: ${payload.failed_count || 0}`);
    if (payload.output) console.log(`  output: ${payload.output.json_path}`);
  }
  if ((payload.status === "failed" || (payload.status === "skipped" && options.requireCaseDb)) && options.failOnGaps) {
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exitCode = 1;
});
