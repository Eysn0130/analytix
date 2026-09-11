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
import { PUBLICATION_RECEIPT_REQUIRED_TEXT } from "../mcp/report-publication-guard.mjs";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");
const DEFAULT_THREADS_ROOT = path.join(os.homedir(), ".analytix", "data", "threads");
const DEFAULT_TRACE_ROOT = path.join(os.homedir(), "Library", "Application Support", "analytix", "traces");
const OUTPUT_ROOT = path.join(os.tmpdir(), "analytix-fund-analysis", "frontdoor-p0-oracle");
const ACCEPTED_FINAL_SIGNATURE_DOMAIN = Buffer.from("analytix.final-answer-authority/v1\0");
const ACCEPTED_FINAL_EVENT_DOMAIN = "analytix.accepted-final-event/v1\0";
const ED25519_SPKI_PREFIX = Buffer.from("302a300506032b6570032100", "hex");
const ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON = Object.freeze({
  success: "completed",
  source_unavailable: "completed",
  semantic_failure: "failed",
  provider_failure: "failed",
  cancel: "aborted",
  timeout: "failed",
  stream_abort: "failed",
  recovery: "completed",
  approval: "completed",
  user_input: "completed",
  resume: "completed",
  restart: "aborted",
  report_fallback: "failed",
  step_limit: "failed",
  background_completion: "completed",
  tool_failure: "failed",
  approval_denied: "completed",
  input_cancelled: "completed"
});
const PYTHON_DUCKDB = String.raw`
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
con = duckdb.connect(payload["db_path"], read_only=True)
try:
    cursor = con.execute(payload["sql"])
    columns = [desc[0] for desc in cursor.description or []]
    limit = max(1, min(int(payload.get("limit") or 100), 5000))
    rows = cursor.fetchmany(limit)
finally:
    con.close()
print(json.dumps({
    "columns": columns,
    "rows": [
        {columns[index]: normalize(value) for index, value in enumerate(row)}
        for row in rows
    ]
}, ensure_ascii=False))
`;

const INTERNAL_LEAK_PATTERNS = [
  /\bDuckDB\b/iu,
  /\banalysis_[a-z0-9_]+\b/iu,
  /\bfc_[a-z0-9_]+(?:_norm|_raw)?\b/iu,
  /\bMCP\b/iu,
  /\bmcp_analytix_funds\b/iu,
  /\bquery_id\b/iu,
  /\bsource_hash\b/iu,
  /\bevidence_card\b/iu,
  /\bvalidation_state\b/iu,
  /\bsql_policy\b/iu,
  /\bmetric_scope\b/iu,
  /\blocal-duckdb:/iu,
  /\bcase_id\b/iu,
  /案件(?:编号|ID)\s*[:：]?\s*[0-9a-f]{8,}/iu,
  /\bWorkbench\b/iu,
  /\bcontrolled\b/iu,
  /\bsource lane\b/iu,
  /\bfollow-up\b/iu
];

const REPORT_ARTIFACT_SIGNAL_PATTERNS = [
  { id: "absolute_report_file", pattern: /(?:\/Users\/|\/tmp\/|output\/)[^\s"']+\.(?:csv|docx|html|json|md|pdf|png|xlsx)\b/iu },
  { id: "report_file_name", pattern: /\b[^\s"']+\.(?:csv|docx|html|json|md|pdf|png|xlsx)\b/iu },
  { id: "report_path_field", pattern: /\b(?:artifact|file|manifest|report)[_-]?(?:id|path|uri|url)\b\s*[":=]/iu },
  { id: "localized_report_path_field", pattern: /(?:附件|报告|文件|清单)(?:路径|地址|编号)\s*["：:=]/u }
];

const REPORT_CASE_FACT_SIGNAL_PATTERNS = [
  { id: "amount_or_count", pattern: /\d[\d,]*(?:\.\d+)?\s*(?:亿元|万元|元|笔|条|户|个账户)/u },
  { id: "account_number", pattern: /(?:银行卡号|卡号|账号|账户)\s*[:：]?\s*(?:\d[ -]?){6,}/u },
  { id: "device_identifier", pattern: /\b(?:[0-9A-F]{2}:){5}[0-9A-F]{2}\b|\b(?:\d{1,3}\.){3}\d{1,3}\b/iu },
  { id: "case_fact_table", pattern: /\|\s*(?:主体|账户|户名|对手方|金额|笔数|流向|日期)\s*\|/u },
  { id: "high_risk_characterization", pattern: /串通投标|围标|行贿|利益输送|具备立案条件/u }
];

const TASKS = [
  {
    id: "A",
    title: "简单数据量",
    question: "交易明细表数据量多少",
    match: /交易明细表数据量多少/u,
    expectedTools: ["count_case_rows", "get_scope_coverage", "inspect_case_schema"],
    audit: auditTaskA
  },
  {
    id: "B",
    title: "资金交易最大的户名是谁",
    question: "资金交易最大的户名是谁",
    match: /资金交易最大的户名是谁/u,
    expectedTools: ["rank_holders", "run_case_sql"],
    audit: auditTaskB
  },
  {
    id: "C",
    title: "Top 10 去向",
    question: "江苏航案件分析中资金流向前十位对手方是谁",
    match: /(?=.*(?:资金流向|流向|去向))(?=.*(?:对手方|收款方|去向))(?=.*(?:前十位|前十|Top\s*10))/iu,
    expectedTools: ["rank_counterparties", "trace_subject_top_outflows", "run_case_sql"],
    audit: auditTaskC
  },
  {
    id: "D",
    title: "Pair Amount",
    question: "A 转给 B 多少钱",
    match: /(?:转给|打给|汇给).*(?:多少钱|多少|金额)/u,
    expectedTools: ["investigate_pair_amount", "run_case_sql"],
    audit: auditTaskD
  },
  {
    id: "E",
    title: "语义不足自定义口径",
    question: "按当前案件明细，统计 2024 年每月出账金额前十的户名，给出可复算口径",
    match: /2024\s*年每月出账金额前十/u,
    expectedTools: ["inspect_case_schema", "profile_case_schema", "case_sql_recipes", "explain_case_sql", "run_case_sql"],
    audit: auditTaskE
  },
  {
    id: "F",
    title: "图谱/下一跳",
    question: "对最大收款方继续看后续资金去向，能画出一跳资金流向吗",
    match: /(?:最大收款方|后续资金去向|一跳资金流向|下一跳)/u,
    expectedTools: ["trace_subject_top_outflows", "trace_fund_next_hop", "build_fund_flow_graph", "run_case_sql"],
    audit: auditTaskF
  },
  {
    id: "G",
    title: "报告发布隔离",
    question: "基于当前案件生成一份资金研判简报，并附主要资金流向表",
    match: /(?:资金研判简报|主要资金流向表|生成.*简报|附.*流向表)/u,
    expectedTools: [],
    audit: auditTaskG
  }
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function parseArgs(argv) {
  const options = {
    caseProjectRoot: "",
    caseId: "",
    threadId: "",
    threadsRoot: DEFAULT_THREADS_ROOT,
    outputDir: "",
    requireInstalledVersion: "",
    selfTestWorkspaceBinding: false,
    selfTestReplayNormalization: false,
    selfTestInstalledVersionScan: false,
    selfTestReportContainment: false,
    json: false,
    failOnGaps: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--case-project-root") options.caseProjectRoot = text(argv[++index]);
    else if (arg === "--case-id") options.caseId = text(argv[++index]);
    else if (arg === "--thread-id") options.threadId = text(argv[++index]);
    else if (arg === "--threads-root") options.threadsRoot = text(argv[++index]) || options.threadsRoot;
    else if (arg === "--output-dir") options.outputDir = text(argv[++index]);
    else if (arg.startsWith("--output-dir=")) options.outputDir = text(arg.slice("--output-dir=".length));
    else if (arg === "--require-installed-version") options.requireInstalledVersion = text(argv[++index]);
    else if (arg === "--self-test-workspace-binding") options.selfTestWorkspaceBinding = true;
    else if (arg === "--self-test-replay-normalization") options.selfTestReplayNormalization = true;
    else if (arg === "--self-test-installed-version-scan") options.selfTestInstalledVersionScan = true;
    else if (arg === "--self-test-report-containment") options.selfTestReportContainment = true;
    else if (arg === "--json") options.json = true;
    else if (arg === "--fail-on-gaps") options.failOnGaps = true;
    else if (arg === "--help" || arg === "-h") {
      console.log("Usage: node plugins/analytix-fund-analysis/scripts/frontdoor-p0-oracle.mjs --case-project-root <path> --case-id <id> [--thread-id <id>] [--threads-root <path>] [--output-dir <path>] [--require-installed-version <version>] [--json] [--fail-on-gaps]");
      console.log("       node plugins/analytix-fund-analysis/scripts/frontdoor-p0-oracle.mjs --self-test-workspace-binding [--json]");
      console.log("       node plugins/analytix-fund-analysis/scripts/frontdoor-p0-oracle.mjs --self-test-replay-normalization [--json]");
      console.log("       node plugins/analytix-fund-analysis/scripts/frontdoor-p0-oracle.mjs --self-test-installed-version-scan [--json]");
      console.log("       node plugins/analytix-fund-analysis/scripts/frontdoor-p0-oracle.mjs --self-test-report-containment [--json]");
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  if (!options.selfTestWorkspaceBinding && !options.selfTestReplayNormalization && !options.selfTestInstalledVersionScan && !options.selfTestReportContainment) {
    if (!options.caseProjectRoot) throw new Error("--case-project-root is required");
    if (!options.caseId) throw new Error("--case-id is required");
  }
  return options;
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function readJsonIfExists(filePath) {
  try {
    return readJson(filePath);
  } catch {
    return null;
  }
}

function readTextIfExists(filePath) {
  try {
    return fs.readFileSync(filePath, "utf8");
  } catch {
    return "";
  }
}

function parseJsonl(filePath) {
  const body = readTextIfExists(filePath);
  if (!body) return [];
  const rows = [];
  for (const line of body.split(/\r?\n/u)) {
    if (!line.trim()) continue;
    try {
      rows.push(JSON.parse(line));
    } catch {
      rows.push({ parse_error: true, raw: line.slice(0, 500) });
    }
  }
  return rows;
}

function sha256(value) {
  return crypto.createHash("sha256").update(value).digest("hex");
}

function goJSONStringify(value) {
  const encoded = JSON.stringify(value);
  if (encoded === undefined) throw new Error("value is not JSON serializable");
  return encoded
    .replace(/</gu, "\\u003c")
    .replace(/>/gu, "\\u003e")
    .replace(/&/gu, "\\u0026")
    .replace(/\u2028/gu, "\\u2028")
    .replace(/\u2029/gu, "\\u2029");
}

function goCanonicalJson(value) {
  if (value === null || typeof value !== "object") return goJSONStringify(value);
  if (Array.isArray(value)) return `[${value.map(goCanonicalJson).join(",")}]`;
  const record = objectOf(value);
  return `{${Object.keys(record).sort().map((key) => `${goJSONStringify(key)}:${goCanonicalJson(record[key])}`).join(",")}}`;
}

function acceptedFinalPublicationEventId(recordDigest, slot) {
  return sha256(`${ACCEPTED_FINAL_EVENT_DOMAIN}${recordDigest}\0${slot}`);
}

function acceptedFinalPublicationPayloadDigest(event) {
  const canonical = { ...event };
  delete canonical.seq;
  delete canonical.publicationPayloadDigest;
  return sha256(goCanonicalJson(canonical));
}

function hasExactKeys(value, required, optional = []) {
  const keys = Object.keys(objectOf(value)).sort();
  const allowed = new Set([...required, ...optional]);
  return required.every((key) => Object.prototype.hasOwnProperty.call(value, key))
    && keys.every((key) => allowed.has(key));
}

function isIsoTimestamp(value) {
  const body = text(value);
  return body.length > 0 && Number.isFinite(Date.parse(body)) && /(?:Z|[+-]\d{2}:\d{2})$/u.test(body);
}

const ACCEPTED_FINAL_VARIANTS = new Set([
  "EvidenceBackedAnswer",
  "PartialEvidenceAnswer",
  "VerifiedNoHitAnswer",
  "SourceUnavailableAnswer",
  "NeedsEvidenceAnswer",
  "GeneralGuidanceAnswer"
]);
const ACCEPTED_FINAL_RECORD_KEYS = [
  "schemaVersion", "authorityPurpose", "authorityAlgorithm", "authorityKeyId", "authorityPublicKey",
  "threadId", "turnId", "envelopeDigest", "contextDigest", "contextEpoch", "datasetSnapshotId",
  "variant", "terminalReason", "renderedTextSha256", "registrySequence", "registryStateDigest",
  "rendererVersion", "finalGateVersion", "verifierVersion", "privateRecordDigest", "acceptedAt",
  "authoritySignature", "recordDigest"
];
const ACCEPTED_FINAL_VIEW_KEYS = [
  "schemaVersion", "publicationState", "acceptedFinalDigest", "envelopeDigest", "contextDigest",
  "contextEpoch", "datasetSnapshotId", "variant", "terminalReason", "blockerCode", "coverageStatus",
  "checkedScopeDigest", "missingScopeCount", "claimCount", "claimTypes", "receiptMetadata",
  "noHitWording", "envelopeIssuedAt", "acceptedAt"
];

function acceptedFinalRecordIntegrity(record) {
  try {
    const publicKey = Buffer.from(text(record.authorityPublicKey), "base64url");
    const signature = Buffer.from(text(record.authoritySignature), "base64url");
    if (
      Number(record.schemaVersion) !== 3
      || text(record.authorityPurpose) !== "analytix.case-final/v1"
      || text(record.authorityAlgorithm) !== "Ed25519"
      || text(record.rendererVersion) !== "analytix.host-final-renderer/v1"
      || text(record.finalGateVersion) !== "analytix.final-evidence-gate/v2"
      || text(record.verifierVersion) !== "analytix.claim-verifier-policy/v1"
      || !hasExactKeys(record, ACCEPTED_FINAL_RECORD_KEYS, ["publicationSnapshotProofDigest"])
      || !ACCEPTED_FINAL_VARIANTS.has(text(record.variant))
      || !Number.isSafeInteger(record.contextEpoch)
      || record.contextEpoch <= 0
      || !Number.isSafeInteger(record.registrySequence)
      || record.registrySequence < 0
      || !isIsoTimestamp(record.acceptedAt)
      || publicKey.length !== 32
      || signature.length !== 64
      || sha256(publicKey) !== text(record.authorityKeyId)
      || !/^[0-9a-f]{64}$/u.test(text(record.recordDigest))
      || !Object.prototype.hasOwnProperty.call(ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON, text(record.terminalReason))
    ) {
      return false;
    }
    const factBearing = new Set(["EvidenceBackedAnswer", "PartialEvidenceAnswer", "VerifiedNoHitAnswer"])
      .has(text(record.variant));
    if (factBearing !== /^[0-9a-f]{64}$/u.test(text(record.publicationSnapshotProofDigest))) return false;
    const signingRecord = { ...record, authoritySignature: "", recordDigest: "" };
    const signingDigest = crypto.createHash("sha256").update(goJSONStringify(signingRecord)).digest();
    const signingBytes = Buffer.concat([ACCEPTED_FINAL_SIGNATURE_DOMAIN, signingDigest]);
    const key = crypto.createPublicKey({
      key: Buffer.concat([ED25519_SPKI_PREFIX, publicKey]),
      format: "der",
      type: "spki"
    });
    if (!crypto.verify(null, signingBytes, key, signature)) return false;
    return sha256(goJSONStringify({ ...record, recordDigest: "" })) === text(record.recordDigest);
  } catch {
    return false;
  }
}

function acceptedFinalViewMatchesRecord(record, view) {
  const receiptMetadata = objectOf(view.receiptMetadata);
  const citations = arrayOf(receiptMetadata.citations);
  const claimTypes = arrayOf(view.claimTypes);
  const receiptShapeValid = hasExactKeys(receiptMetadata, ["projection", "count", "setDigest", "citations"])
    && text(receiptMetadata.projection) === "masked_metadata_only"
    && Number.isSafeInteger(receiptMetadata.count)
    && receiptMetadata.count >= 0
    && receiptMetadata.count === citations.length
    && /^[0-9a-f]{64}$/u.test(text(receiptMetadata.setDigest))
    && citations.every((citation, index) =>
      hasExactKeys(objectOf(citation), ["handle", "label"])
      && /^cite_[0-9a-f]{64}$/u.test(text(citation.handle))
      && text(citation.label) === `evidence-${index + 1}`
    );
  const uniqueSortedClaimTypes = [...new Set(claimTypes.map(text))].sort();
  const sharedShapeValid = hasExactKeys(view, ACCEPTED_FINAL_VIEW_KEYS)
    && Number(view.schemaVersion) === 1
    && Number.isSafeInteger(view.contextEpoch)
    && view.contextEpoch > 0
    && Number.isSafeInteger(view.missingScopeCount)
    && view.missingScopeCount >= 0
    && Number.isSafeInteger(view.claimCount)
    && view.claimCount >= 0
    && uniqueSortedClaimTypes.length === claimTypes.length
    && uniqueSortedClaimTypes.every((claimType, index) => claimType === claimTypes[index])
    && ((view.claimCount === 0) === (claimTypes.length === 0))
    && receiptShapeValid
    && isIsoTimestamp(view.envelopeIssuedAt)
    && isIsoTimestamp(view.acceptedAt);
  const receiptCount = Number(receiptMetadata.count);
  const checkedScope = text(view.checkedScopeDigest);
  const variantShapeValid = {
    EvidenceBackedAnswer: text(view.coverageStatus) === "complete" && view.claimCount > 0 && receiptCount > 0
      && /^[0-9a-f]{64}$/u.test(checkedScope) && view.missingScopeCount === 0 && text(view.blockerCode) === "" && text(view.noHitWording) === "",
    PartialEvidenceAnswer: text(view.coverageStatus) === "partial" && view.claimCount > 0 && receiptCount > 0
      && /^[0-9a-f]{64}$/u.test(checkedScope) && view.missingScopeCount > 0 && text(view.blockerCode) === "" && text(view.noHitWording) === "",
    VerifiedNoHitAnswer: text(view.coverageStatus) === "complete" && view.claimCount === 0 && receiptCount > 0
      && /^[0-9a-f]{64}$/u.test(checkedScope) && view.missingScopeCount === 0 && text(view.blockerCode) === ""
      && text(view.noHitWording) === "not_found_in_checked_scope",
    SourceUnavailableAnswer: text(view.coverageStatus) === "unavailable" && view.claimCount === 0 && receiptCount === 0
      && text(view.blockerCode) !== "" && text(view.noHitWording) === "",
    NeedsEvidenceAnswer: text(view.coverageStatus) === "unverified" && view.claimCount === 0 && receiptCount === 0
      && view.missingScopeCount > 0 && text(view.noHitWording) === "",
    GeneralGuidanceAnswer: text(view.coverageStatus) === "guidance_only" && view.claimCount === 0 && receiptCount === 0
      && checkedScope === "" && view.missingScopeCount === 0 && text(view.blockerCode) === "" && text(view.noHitWording) === ""
  }[text(view.variant)] === true;
  return sharedShapeValid
    && variantShapeValid
    && text(view.publicationState) === "accepted"
    && text(view.acceptedFinalDigest) === text(record.recordDigest)
    && text(view.envelopeDigest) === text(record.envelopeDigest)
    && text(view.contextDigest) === text(record.contextDigest)
    && Number(view.contextEpoch) === Number(record.contextEpoch)
    && text(view.datasetSnapshotId) === text(record.datasetSnapshotId)
    && text(view.variant) === text(record.variant)
    && text(view.terminalReason) === text(record.terminalReason)
    && text(view.acceptedAt) === text(record.acceptedAt)
    && !/(?:rawReceiptId|receiptId|sourceRecordIds|reasoning)/u.test(JSON.stringify(view));
}

function containsPrivateReasoningContent(value, inheritedRuntimeText = true, inheritedUserText = false, seen = new Set()) {
  if (Array.isArray(value)) {
    return value.some((entry) => containsPrivateReasoningContent(entry, inheritedRuntimeText, inheritedUserText, seen));
  }
  if (typeof value === "string") {
    if (!inheritedRuntimeText) return false;
    return /<\/?think(?:\s[^>]*)?>|(?:assistant_reasoning|reasoning_content|thinking_content)\s*[:=]/iu.test(value);
  }
  if (!value || typeof value !== "object" || seen.has(value)) return false;
  seen.add(value);
  const record = objectOf(value);
  const normalizedKind = text(record.kind).toLowerCase();
  if (["assistant_reasoning", "assistant_reasoning_delta", "agent_reasoning"].includes(normalizedKind)) return true;
  const role = text(record.role).toLowerCase();
  const userText = inheritedUserText || role === "user" || normalizedKind === "user_message";
  const assistantText = role === "assistant"
    || ["assistant_text", "assistant_text_delta", "agent_message"].includes(normalizedKind);
  const runtimeText = userText ? false : inheritedRuntimeText || assistantText || Object.keys(record).length > 0;
  for (const [key, entry] of Object.entries(record)) {
    const normalizedKey = key.toLowerCase().replace(/[^a-z0-9]/gu, "");
    if (["reasoning", "assistantreasoning", "reasoningcontent", "assistantreasoningcontent", "thinking", "assistantthinking", "thinkingcontent", "assistantthinkingcontent"].includes(normalizedKey)) {
      return true;
    }
    if (containsPrivateReasoningContent(entry, runtimeText, userText, seen)) return true;
  }
  return false;
}

function fileStatSummary(filePath) {
  if (!filePath) return { exists: false, size: 0, mtime_ms: 0, mtime_iso: "" };
  try {
    const stat = fs.statSync(filePath);
    return {
      exists: true,
      size: stat.size,
      mtime_ms: stat.mtimeMs,
      mtime_iso: new Date(stat.mtimeMs).toISOString()
    };
  } catch {
    return { exists: false, size: 0, mtime_ms: 0, mtime_iso: "" };
  }
}

function normalizeFsPath(value) {
  const body = text(value);
  if (!body) return "";
  try {
    return fs.realpathSync.native(body);
  } catch {
    return path.resolve(body);
  }
}

function sameFsPath(left, right) {
  const leftPath = normalizeFsPath(left);
  const rightPath = normalizeFsPath(right);
  return Boolean(leftPath && rightPath && leftPath === rightPath);
}

function nowStamp() {
  return new Date().toISOString().replace(/[:.]/gu, "-");
}

function versionFromSource(filePath, regex) {
  const body = readTextIfExists(filePath);
  const match = body.match(regex);
  return match ? match[1] : "";
}

function collectVersionEvidence(requireInstalledVersion) {
  const manifestPath = path.join(PLUGIN_ROOT, ".codex-plugin", "plugin.json");
  const manifest = readJson(manifestPath);
  const source = {
    manifest_path: manifestPath,
    manifest_version: text(manifest.version),
    server_version: versionFromSource(path.join(PLUGIN_ROOT, "mcp", "server.mjs"), /SERVER_VERSION\s*=\s*"([^"]+)"/u),
    tool_schemas_version: versionFromSource(path.join(PLUGIN_ROOT, "mcp", "tool-schemas.mjs"), /TOOL_SCHEMAS_VERSION\s*=\s*"([^"]+)"/u),
    tool_call_runtime_version: versionFromSource(path.join(PLUGIN_ROOT, "mcp", "tool-call-runtime.mjs"), /TOOL_CALL_RUNTIME_VERSION\s*=\s*"([^"]+)"/u)
  };
  const installed = findInstalledPluginVersions();
  const matchingInstalled = requireInstalledVersion
    ? installed.filter((item) => item.version === requireInstalledVersion)
    : installed.filter((item) => item.version === source.manifest_version);
  return {
    source,
    installed,
    require_installed_version: requireInstalledVersion || "",
    installed_requirement_met: !requireInstalledVersion || matchingInstalled.length > 0,
    matching_installed: matchingInstalled
  };
}

function uniqueFsPaths(values) {
  const seen = new Set();
  const output = [];
  for (const value of values) {
    const normalized = normalizeFsPath(value);
    if (!normalized || seen.has(normalized)) continue;
    seen.add(normalized);
    output.push(normalized);
  }
  return output;
}

function installedPluginScanRuntimeHomes() {
  return uniqueFsPaths([
    process.env.ANALYTIX_AGENT_RUNTIME_HOME,
    process.env.ANALYTIX_AGENT_RUNTIME_CODEX_HOME,
    path.join(os.homedir(), ".analytix"),
    path.join(os.homedir(), "Library", "Application Support", "analytix")
  ]);
}

function installedPluginScanRoots() {
  const runtimeHomes = installedPluginScanRuntimeHomes();
  return uniqueFsPaths([
    ...runtimeHomes.map((runtimeHome) => path.join(runtimeHome, "plugins", "cache")),
    ...runtimeHomes.map((runtimeHome) => path.join(runtimeHome, ".cache", "analytix-hub-plugins", "marketplaces")),
    path.join(os.homedir(), ".codex", "plugins", "cache")
  ]);
}

function findInstalledPluginVersions(roots = installedPluginScanRoots()) {
  const output = [];
  for (const root of roots) {
    scanPluginManifests(root, 0, output);
  }
  return output;
}

function scanPluginManifests(dir, depth, output) {
  if (depth > 7 || !fs.existsSync(dir)) return;
  let entries = [];
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return;
  }
  const manifestPath = path.join(dir, ".codex-plugin", "plugin.json");
  if (fs.existsSync(manifestPath)) {
    try {
      const manifest = readJson(manifestPath);
      const id = text(manifest.id || manifest.name || manifest.slug);
      if (id === "analytix-fund-analysis") {
        const stat = fileStatSummary(manifestPath);
        output.push({
          path: dir,
          manifest_path: manifestPath,
          version: text(manifest.version),
          manifest_mtime_ms: stat.mtime_ms,
          manifest_mtime_iso: stat.mtime_iso,
          sha256: sha256(fs.readFileSync(manifestPath))
        });
      }
    } catch {
      // Ignore unreadable cache entries.
    }
  }
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    if (entry.name === "node_modules" || entry.name === ".git") continue;
    scanPluginManifests(path.join(dir, entry.name), depth + 1, output);
  }
}

function runDuckdb(dbPath, sql, limit = 100) {
  const result = spawnSync("python3", ["-c", PYTHON_DUCKDB], {
    input: JSON.stringify({ db_path: dbPath, sql, limit }),
    encoding: "utf8",
    timeout: 120_000,
    maxBuffer: 1024 * 1024 * 20
  });
  if (result.status !== 0) {
    throw new Error(`duckdb oracle failed: ${text(result.stderr) || text(result.stdout)}`);
  }
  return JSON.parse(result.stdout);
}

function firstRow(result) {
  return objectOf(arrayOf(result.rows)[0]);
}

function numberValue(value) {
  const next = Number(value);
  return Number.isFinite(next) ? next : 0;
}

function moneyText(value) {
  const next = Number(value);
  if (!Number.isFinite(next)) return "";
  return `${next.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} 元`;
}

function buildDuckdbCrossChecks(dbPath) {
  const count = firstRow(runDuckdb(dbPath, "SELECT COUNT(*) AS txn_count FROM analysis_txn_detail_idx", 1));
  const holderRank = runDuckdb(dbPath, `
    SELECT
      account_open_name AS holder_name,
      opener_id_no AS id_no,
      COUNT(*) AS txn_count,
      COUNT(DISTINCT acct_key) AS account_count,
      SUM(CASE WHEN dc_val = '进' THEN amount ELSE 0 END) AS inflow_total,
      SUM(CASE WHEN dc_val = '出' THEN amount ELSE 0 END) AS outflow_total,
      SUM(amount) AS turnover_total,
      MIN(txn_ts) AS first_txn_at,
      MAX(txn_ts) AS last_txn_at
    FROM analysis_txn_detail_idx
    WHERE account_open_name IS NOT NULL AND TRIM(account_open_name) <> '' AND amount IS NOT NULL
    GROUP BY account_open_name, opener_id_no
    ORDER BY turnover_total DESC, txn_count DESC
    LIMIT 10`, 10);
  const destinationTop10 = runDuckdb(dbPath, `
    WITH base AS (
      SELECT
        COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS counterparty_name,
        COALESCE(NULLIF(TRIM(account_open_name), ''), '__source__') AS source_holder_name,
        acct_key,
        amount,
        txn_ts
      FROM analysis_txn_detail_idx
      WHERE dc_val = '出'
        AND amount IS NOT NULL
    ), grouped AS (
      SELECT
        counterparty_name,
        COUNT(*) AS txn_count,
        COUNT(DISTINCT acct_key) AS source_account_count,
        SUM(amount) AS outflow_total,
        MIN(txn_ts) AS first_txn_at,
        MAX(txn_ts) AS last_txn_at
      FROM base
      WHERE counterparty_name <> '空户名'
        AND counterparty_name <> source_holder_name
      GROUP BY counterparty_name
    )
    SELECT ROW_NUMBER() OVER (ORDER BY outflow_total DESC, txn_count DESC) AS rank, *
    FROM grouped
    ORDER BY outflow_total DESC, txn_count DESC
    LIMIT 10`, 10);
  const pairSeed = firstRow(runDuckdb(dbPath, `
    WITH base AS (
      SELECT
        account_open_name AS payer_name,
        COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS receiver_name,
        amount,
        txn_ts
      FROM analysis_txn_detail_idx
      WHERE dc_val = '出'
        AND amount IS NOT NULL
        AND account_open_name IS NOT NULL
        AND TRIM(account_open_name) <> ''
    )
    SELECT
      payer_name,
      receiver_name,
      COUNT(*) AS raw_detail_count,
      SUM(amount) AS raw_detail_amount,
      MIN(txn_ts) AS first_txn_at,
      MAX(txn_ts) AS last_txn_at
    FROM base
    GROUP BY payer_name, receiver_name
    HAVING payer_name <> receiver_name AND receiver_name <> '空户名'
    ORDER BY raw_detail_amount DESC, raw_detail_count DESC
    LIMIT 1`, 1));
  const pairSql = `
    SELECT
      COUNT(*) AS raw_detail_count,
      SUM(amount) AS raw_detail_amount,
      COUNT(DISTINCT acct_key) AS payer_account_count,
      COUNT(DISTINCT counterparty_acct) AS receiver_account_count,
      MIN(txn_ts) AS first_txn_at,
      MAX(txn_ts) AS last_txn_at
    FROM analysis_txn_detail_idx
    WHERE dc_val = '出'
      AND account_open_name = ${sqlString(pairSeed.payer_name)}
      AND COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') = ${sqlString(pairSeed.receiver_name)}`;
  const pairAggregate = firstRow(runDuckdb(dbPath, pairSql, 1));
  const monthly2024 = runDuckdb(dbPath, `
    WITH monthly AS (
      SELECT
        strftime(txn_ts, '%Y-%m') AS month,
        account_open_name AS holder_name,
        COUNT(*) AS txn_count,
        SUM(amount) AS outflow_total
      FROM analysis_txn_detail_idx
      WHERE dc_val = '出'
        AND txn_ts >= TIMESTAMP '2024-01-01'
        AND txn_ts < TIMESTAMP '2025-01-01'
        AND account_open_name IS NOT NULL
        AND TRIM(account_open_name) <> ''
        AND amount IS NOT NULL
      GROUP BY month, holder_name
    ), ranked AS (
      SELECT ROW_NUMBER() OVER (PARTITION BY month ORDER BY outflow_total DESC, txn_count DESC) AS rank, *
      FROM monthly
    )
    SELECT * FROM ranked WHERE rank <= 10 ORDER BY month, rank`, 140);
  const maxReceiver = firstRow(destinationTop10);
  const nextHop = maxReceiver.counterparty_name ? runDuckdb(dbPath, `
    SELECT
      COALESCE(NULLIF(TRIM(counterparty_name), ''), NULLIF(TRIM(cp_name_pick), ''), NULLIF(TRIM(cp_name), ''), NULLIF(TRIM(stats_name_key), ''), '空户名') AS downstream_counterparty,
      COUNT(*) AS txn_count,
      SUM(amount) AS outflow_total,
      MIN(txn_ts) AS first_txn_at,
      MAX(txn_ts) AS last_txn_at
    FROM analysis_txn_detail_idx
    WHERE dc_val = '出'
      AND account_open_name = ${sqlString(maxReceiver.counterparty_name)}
      AND amount IS NOT NULL
    GROUP BY downstream_counterparty
    ORDER BY outflow_total DESC, txn_count DESC
    LIMIT 10`, 10) : { columns: [], rows: [] };
  return {
    core_table_counts: {
      analysis_txn_detail_idx: numberValue(count.txn_count)
    },
    holder_rank_turnover_top10: holderRank.rows,
    destination_outflow_top10: destinationTop10.rows,
    pair_amount_seed: pairSeed,
    pair_amount_sql: pairSql.replace(/\s+/gu, " ").trim(),
    pair_amount_aggregate: pairAggregate,
    monthly_2024_outflow_top10_by_holder: monthly2024.rows,
    next_hop_seed_receiver: maxReceiver.counterparty_name || "",
    next_hop_top_outflows: nextHop.rows
  };
}

function sqlString(value) {
  return `'${text(value).replace(/'/gu, "''")}'`;
}

function loadThread(threadId, caseProjectRoot = "", threadsRoot = DEFAULT_THREADS_ROOT) {
  const threadPath = path.join(threadsRoot, threadId, "thread.json");
  const threadDoc = objectOf(readJsonIfExists(threadPath));
  const workspace = text(threadDoc.workspace);
  const candidates = [
    {
      kind: "durable_thread",
      thread_id: threadId,
      thread_path: threadPath,
      messages_path: path.join(threadsRoot, threadId, "messages.jsonl"),
      events_path: path.join(threadsRoot, threadId, "events.jsonl")
    },
    {
      kind: "trace_replay",
      thread_id: threadId,
      messages_path: "",
      events_path: path.join(DEFAULT_TRACE_ROOT, `thread-${threadId}.jsonl`)
    }
  ];
  for (const candidate of candidates) {
    const messages = parseJsonl(candidate.messages_path);
    const events = parseJsonl(candidate.events_path);
    const items = normalizeThreadItems({ messages, events });
    if (items.length || events.length) {
      const messagesStat = fileStatSummary(candidate.messages_path);
      const eventsStat = fileStatSummary(candidate.events_path);
      const evidenceMtimeMs = Math.max(messagesStat.mtime_ms, eventsStat.mtime_ms);
      return {
        ...candidate,
        messages_count: messages.length,
        events_count: events.length,
        thread_workspace: workspace,
        thread_workspace_resolved: normalizeFsPath(workspace),
        thread_workspace_matches: caseProjectRoot ? sameFsPath(workspace, caseProjectRoot) : null,
        messages_stat: messagesStat,
        events_stat: eventsStat,
        evidence_mtime_ms: evidenceMtimeMs,
        evidence_mtime_iso: evidenceMtimeMs ? new Date(evidenceMtimeMs).toISOString() : "",
        events,
        items
      };
    }
  }
  return { thread_id: threadId, status: "missing", items: [] };
}

function normalizeThreadItems({ messages, events }) {
  const byId = new Map();
  for (const item of messages) {
    if (item.id) byId.set(item.id, item);
  }
  for (const event of events) {
    const item = objectOf(event.item);
    const id = text(item.id);
    const eventKind = text(event.kind || event.type || event.event);
    if (eventKind === "assistant_text_delta" || eventKind === "assistant_reasoning_delta") {
      continue;
    }
    if (!id) continue;
    const existing = byId.get(id);
    if (existing && text(existing.status) === "completed" && text(item.status) === "running") continue;
    byId.set(id, item);
  }
  return [...byId.values()].sort((left, right) =>
    text(left.createdAt).localeCompare(text(right.createdAt)) || text(left.id).localeCompare(text(right.id))
  );
}

function acceptedFinalPublication(item, events, threadId, turnId) {
  const expectedItemId = `item_${turnId}_assistant`;
  if (
    text(item.kind) !== "assistant_text"
    || text(item.role) !== "assistant"
    || text(item.status) !== "completed"
    || text(item.id) !== expectedItemId
    || text(item.threadId) !== threadId
    || text(item.turnId) !== turnId
    || !text(item.text)
    || containsPrivateReasoningContent(item.text)
  ) {
    return null;
  }
  const record = objectOf(item.acceptedFinal);
  const view = objectOf(item.acceptedFinalView);
  const digest = text(record.recordDigest);
  if (
    text(record.threadId) !== threadId
    || text(record.turnId) !== turnId
    || text(item.finishedAt) !== text(record.acceptedAt)
    || text(record.renderedTextSha256) !== sha256(text(item.text))
    || !acceptedFinalRecordIntegrity(record)
    || !acceptedFinalViewMatchesRecord(record, view)
  ) {
    return null;
  }
  const assistantEvents = arrayOf(events).filter((event) => {
    const eventItem = objectOf(event.item);
    const eventRecord = objectOf(eventItem.acceptedFinal);
    return text(event.kind || event.type || event.event) === "item_completed"
      && Number.isSafeInteger(event.seq)
      && event.seq >= 0
      && text(event.turnId) === turnId
      && text(event.threadId) === threadId
      && text(event.itemId) === expectedItemId
      && text(event.timestamp) === text(record.acceptedAt)
      && text(event.publicationSlot) === "assistant-final"
      && text(event.publicationCommitId) === digest
      && text(event.acceptedFinalDigest) === digest
      && text(event.publicationEventId) === acceptedFinalPublicationEventId(digest, "assistant-final")
      && text(event.publicationPayloadDigest) === acceptedFinalPublicationPayloadDigest(event)
      && text(eventItem.id) === text(item.id)
      && text(eventRecord.recordDigest) === digest
      && text(objectOf(eventItem.acceptedFinalView).acceptedFinalDigest) === digest
      && goCanonicalJson(eventItem) === goCanonicalJson(item);
  });
  if (assistantEvents.length !== 1) return null;
  const itemSequence = Number(assistantEvents[0].seq);
  const expectedTerminalStatus = ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON[text(record.terminalReason)];
  const terminalEvents = arrayOf(events).filter((event) => {
    const abortedFields = ["discard", "cancelled", "cancelledPendingGates"];
    const abortShapeValid = expectedTerminalStatus === "aborted"
      ? typeof event.discard === "boolean"
        && typeof event.cancelled === "boolean"
        && Number.isSafeInteger(event.cancelledPendingGates)
        && event.cancelledPendingGates >= 0
      : abortedFields.every((field) => !Object.prototype.hasOwnProperty.call(event, field));
    return /^(?:turn_completed|turn_failed|turn_aborted)$/u.test(text(event.kind || event.type || event.event))
      && text(event.kind || event.type || event.event) === `turn_${expectedTerminalStatus}`
      && Number.isSafeInteger(event.seq)
      && event.seq > itemSequence
      && text(event.turnId) === turnId
      && text(event.threadId) === threadId
      && text(event.status) === expectedTerminalStatus
      && text(event.terminalReason) === text(record.terminalReason)
      && text(event.timestamp) === text(record.acceptedAt)
      && text(event.publicationSlot) === "terminal"
      && text(event.publicationCommitId) === digest
      && text(event.acceptedFinalDigest) === digest
      && text(event.publicationEventId) === acceptedFinalPublicationEventId(digest, "terminal")
      && text(event.publicationPayloadDigest) === acceptedFinalPublicationPayloadDigest(event)
      && abortShapeValid;
  });
  if (terminalEvents.length !== 1) return null;
  return { digest, record };
}

function threadDiscoveryInfo(threadId, caseProjectRoot, threadsRoot = DEFAULT_THREADS_ROOT) {
  const threadPath = path.join(threadsRoot, threadId, "thread.json");
  const threadDoc = objectOf(readJsonIfExists(threadPath));
  const workspace = text(threadDoc.workspace);
  const turns = arrayOf(threadDoc.turns);
  const latestTurn = objectOf(turns[turns.length - 1]);
  const latestTurnStatus = text(latestTurn.status);
  const latestTurnDiscarded = latestTurn.discard === true || text(latestTurn.discard).toLowerCase() === "true";
  return {
    workspace,
    workspace_resolved: normalizeFsPath(workspace),
    case_project_match: caseProjectRoot ? sameFsPath(workspace, caseProjectRoot) : true,
    latest_turn_id: text(latestTurn.id),
    latest_turn_status: latestTurnStatus,
    latest_turn_discarded: latestTurnDiscarded,
    auto_selectable: !latestTurnDiscarded
  };
}

function matchedTaskIdsForMessagesFile(messagesPath) {
  const body = readTextIfExists(messagesPath);
  if (!body) return [];
  const matchedTaskIds = new Set();
  for (const line of body.split(/\r?\n/u)) {
    if (!line.includes("user_message")) continue;
    let item = null;
    try {
      item = JSON.parse(line);
    } catch {
      item = null;
    }
    if (!item || text(item.kind) !== "user_message") continue;
    const userText = text(item.text);
    if (!userText) continue;
    for (const task of TASKS) {
      if (matchedTaskIds.has(task.id)) continue;
      if (task.match.test(userText)) matchedTaskIds.add(task.id);
    }
    if (matchedTaskIds.size === TASKS.length) break;
  }
  return [...matchedTaskIds];
}

function autoDiscoverThreads(caseProjectRoot = "", threadsRoot = DEFAULT_THREADS_ROOT) {
  const output = [];
  if (fs.existsSync(threadsRoot)) {
    for (const entry of fs.readdirSync(threadsRoot, { withFileTypes: true })) {
      if (!entry.isDirectory()) continue;
      const threadId = entry.name;
      const messagesPath = path.join(threadsRoot, threadId, "messages.jsonl");
      const matchedTaskIds = matchedTaskIdsForMessagesFile(messagesPath);
      if (matchedTaskIds.length) {
        const stat = fs.statSync(messagesPath);
        output.push({
          thread_id: threadId,
          matched_task_ids: matchedTaskIds,
          mtime_ms: stat.mtimeMs,
          mtime_iso: new Date(stat.mtimeMs).toISOString(),
          ...threadDiscoveryInfo(threadId, caseProjectRoot, threadsRoot)
        });
      }
    }
  }
  return output.sort((left, right) => right.mtime_ms - left.mtime_ms);
}

function selectLatestThreadPerTask(discovered) {
  const selected = [];
  const selectedIds = new Set();
  for (const task of TASKS) {
    const match = discovered.find((item) =>
      item.case_project_match
      && item.auto_selectable !== false
      && arrayOf(item.matched_task_ids).includes(task.id)
    );
    if (!match || selectedIds.has(match.thread_id)) continue;
    selectedIds.add(match.thread_id);
    selected.push(match);
  }
  return selected;
}

function selectThreadEvidence(options, caseProjectRoot) {
  const threadsRoot = text(options.threadsRoot) || DEFAULT_THREADS_ROOT;
  if (options.threadId) {
    return {
      mode: "explicit",
      candidate_threads: [],
      excluded_threads: [],
      threads: [loadThread(options.threadId, caseProjectRoot, threadsRoot)]
    };
  }
  const discovered = autoDiscoverThreads(caseProjectRoot, threadsRoot);
  const selected = discovered.filter((item) => item.case_project_match && item.auto_selectable !== false);
  const selectedForLoad = selectLatestThreadPerTask(selected);
  const excluded = discovered.filter((item) => !item.case_project_match || item.auto_selectable === false);
  return {
    mode: "auto",
    candidate_threads: discovered,
    excluded_threads: excluded,
    threads: selectedForLoad.map((item) => loadThread(item.thread_id, caseProjectRoot, threadsRoot))
  };
}

function itemsForTask(thread, task) {
  const items = arrayOf(thread.items);
  const events = arrayOf(thread.events);
  const matches = items.filter((item) => item.kind === "user_message" && task.match.test(text(item.text)));
  const attempts = [];
  for (const user of matches) {
    const turnId = text(user.turnId);
    const sameTurn = items.filter((item) => text(item.turnId) === turnId);
    const sameTurnEvents = events.filter((event) => text(event.turnId || objectOf(event.item).turnId) === turnId);
    const assistantItems = sameTurn.filter((item) => item.kind === "assistant_text" && text(item.text));
    const acceptedAssistantItems = assistantItems
      .map((item) => ({ item, publication: acceptedFinalPublication(item, sameTurnEvents, thread.thread_id, turnId) }))
      .filter((candidate) => candidate.publication !== null);
    const acceptedAssistantIds = new Set(acceptedAssistantItems.map((candidate) => text(candidate.item.id)));
    const assistantDeltaSeen = sameTurnEvents.some((event) =>
      text(event.kind || event.type || event.event) === "assistant_text_delta"
    );
    const assistantReasoningSeen = sameTurn.some((item) => containsPrivateReasoningContent(item))
      || sameTurnEvents.some((event) => containsPrivateReasoningContent(event));
    attempts.push({
      thread_id: thread.thread_id,
      thread_kind: thread.kind,
      thread_workspace: text(thread.thread_workspace),
      thread_workspace_resolved: text(thread.thread_workspace_resolved),
      thread_workspace_matches: thread.thread_workspace_matches,
      thread_evidence_mtime_ms: Number(thread.evidence_mtime_ms || 0),
      thread_evidence_mtime_iso: text(thread.evidence_mtime_iso),
      thread_messages_mtime_ms: Number(objectOf(thread.messages_stat).mtime_ms || 0),
      thread_events_mtime_ms: Number(objectOf(thread.events_stat).mtime_ms || 0),
      turn_id: turnId,
      user_message: text(user.text),
      assistant_answer: acceptedAssistantItems
        .map((candidate) => text(candidate.item.text))
        .join("\n"),
      accepted_final_count: acceptedAssistantItems.length,
      accepted_final_digests: acceptedAssistantItems.map((candidate) => candidate.publication.digest),
      assistant_delta_seen: assistantDeltaSeen,
      unaccepted_assistant_text_seen: assistantDeltaSeen
        || assistantItems.some((item) => !acceptedAssistantIds.has(text(item.id))),
      assistant_reasoning_seen: assistantReasoningSeen,
      tool_calls: sameTurn
        .filter((item) => item.kind === "tool_call")
        .map((item) => ({
          tool_name: normalizedToolName(item.toolName),
          raw_tool_name: text(item.toolName),
          call_id: text(item.callId),
          args: objectOf(item.arguments)
        })),
      tool_results: sameTurn
        .filter((item) => item.kind === "tool_result")
        .map((item) => ({
          tool_name: normalizedToolName(item.toolName),
          raw_tool_name: text(item.toolName),
          call_id: text(item.callId),
          output: item.output
        }))
    });
  }
  return attempts;
}

function normalizedToolName(name) {
  return text(name)
    .replace(/^mcp__analytix_funds__/u, "")
    .replace(/^mcp_analytix_funds_/u, "");
}

function firstAttemptForTask(threads, task) {
  for (const thread of threads) {
    const attempts = itemsForTask(thread, task);
    if (attempts.length) return attempts[attempts.length - 1];
  }
  return null;
}

function buildThreadFreshness(versions) {
  const matchingInstalled = arrayOf(versions.matching_installed);
  const cutoff = Math.max(0, ...matchingInstalled.map((item) => Number(item.manifest_mtime_ms || 0)));
  return {
    required_installed_version: versions.require_installed_version || "",
    required_after_mtime_ms: cutoff,
    required_after_mtime_iso: cutoff ? new Date(cutoff).toISOString() : "",
    matching_installed: matchingInstalled.map((item) => ({
      path: item.path,
      version: item.version,
      manifest_mtime_ms: item.manifest_mtime_ms || 0,
      manifest_mtime_iso: item.manifest_mtime_iso || ""
    }))
  };
}

function applyThreadFreshness(task, freshness) {
  const cutoff = Number(freshness.required_after_mtime_ms || 0);
  const attempt = objectOf(task.frontdoor_attempt);
  const evidenceMtimeMs = Number(attempt.thread_evidence_mtime_ms || 0);
  const failures = [...arrayOf(task.blockers)];
  if (cutoff && task.frontdoor_attempt) {
    if (!evidenceMtimeMs) {
      failures.push("frontdoor_thread_evidence_mtime_missing_for_installed_version");
    } else if (evidenceMtimeMs < cutoff) {
      failures.push("frontdoor_thread_evidence_stale_before_installed_version");
    }
  }
  return {
    ...task,
    status: failures.length ? "failed" : task.status,
    blockers: failures,
    thread_freshness: {
      required_after_mtime_ms: cutoff,
      required_after_mtime_iso: freshness.required_after_mtime_iso,
      evidence_mtime_ms: evidenceMtimeMs,
      evidence_mtime_iso: attempt.thread_evidence_mtime_iso || "",
      fresh_enough: !cutoff || !task.frontdoor_attempt || evidenceMtimeMs >= cutoff
    }
  };
}

function applyThreadWorkspaceBinding(task, caseProjectRoot) {
  const attempt = objectOf(task.frontdoor_attempt);
  const failures = [...arrayOf(task.blockers)];
  if (caseProjectRoot && task.frontdoor_attempt) {
    if (!text(attempt.thread_workspace)) {
      failures.push("frontdoor_thread_workspace_missing");
    } else if (attempt.thread_workspace_matches !== true) {
      failures.push("frontdoor_thread_workspace_mismatch");
    }
  }
  return {
    ...task,
    status: failures.length ? "failed" : task.status,
    blockers: failures,
    thread_workspace_binding: {
      required_case_project_root: path.resolve(caseProjectRoot),
      required_case_project_root_resolved: normalizeFsPath(caseProjectRoot),
      thread_workspace: text(attempt.thread_workspace),
      thread_workspace_resolved: text(attempt.thread_workspace_resolved),
      matches: caseProjectRoot && task.frontdoor_attempt ? attempt.thread_workspace_matches === true : null
    }
  };
}

function assertSelfTest(condition, message) {
  if (!condition) throw new Error(`self-test failed: ${message}`);
}

function writeSyntheticThread(threadsRoot, { threadId, workspace, userText, turnStatus = "completed", discard = false }) {
  const dir = path.join(threadsRoot, threadId);
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, "thread.json"), JSON.stringify({
    id: threadId,
    workspace,
    status: "idle",
    turns: [{
      id: "turn_selftest",
      status: turnStatus,
      discard
    }]
  }), "utf8");
  fs.writeFileSync(path.join(dir, "messages.jsonl"), `${JSON.stringify({
    id: `${threadId}_user`,
    kind: "user_message",
    turnId: "turn_selftest",
    threadId,
    text: userText,
    createdAt: "2026-01-01T00:00:00.000Z"
  })}\n`, "utf8");
  fs.writeFileSync(path.join(dir, "events.jsonl"), "", "utf8");
}

function runWorkspaceBindingSelfTest({ json = false } = {}) {
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "frontdoor-p0-oracle-workspace-"));
  try {
    const caseRoot = path.join(tempRoot, "case-project");
    const otherRoot = path.join(tempRoot, "other-project");
    const threadsRoot = path.join(tempRoot, "threads");
    fs.mkdirSync(caseRoot, { recursive: true });
    fs.mkdirSync(otherRoot, { recursive: true });
    fs.mkdirSync(threadsRoot, { recursive: true });
    const task = TASKS[0];
    writeSyntheticThread(threadsRoot, {
      threadId: "thread_old_same_workspace",
      workspace: caseRoot,
      userText: task.question
    });
    const oldMessagesPath = path.join(threadsRoot, "thread_old_same_workspace", "messages.jsonl");
    fs.utimesSync(oldMessagesPath, new Date("2020-01-01T00:00:00.000Z"), new Date("2020-01-01T00:00:00.000Z"));
    writeSyntheticThread(threadsRoot, {
      threadId: "thread_good",
      workspace: caseRoot,
      userText: task.question
    });
    writeSyntheticThread(threadsRoot, {
      threadId: "thread_discarded_newer",
      workspace: caseRoot,
      userText: task.question,
      turnStatus: "aborted",
      discard: true
    });
    writeSyntheticThread(threadsRoot, {
      threadId: "thread_wrong",
      workspace: otherRoot,
      userText: task.question
    });
    writeSyntheticThread(threadsRoot, {
      threadId: "thread_missing_workspace",
      workspace: "",
      userText: task.question
    });

    const selection = selectThreadEvidence({ threadId: "", threadsRoot }, caseRoot);
    assertSelfTest(selection.candidate_threads.length === 5, "auto discovery should see all synthetic candidates");
    assertSelfTest(selection.threads.length === 1, "auto discovery should load only the latest matching thread for a task");
    assertSelfTest(selection.threads[0]?.thread_id === "thread_good", "latest matching non-discarded workspace thread should be selected");
    assertSelfTest(selection.excluded_threads.length === 3, "wrong, missing, and discarded workspace threads should be excluded");
    assertSelfTest(
      selection.excluded_threads.some((item) => item.thread_id === "thread_discarded_newer" && item.latest_turn_discarded === true),
      "discarded candidate should be excluded from auto selection"
    );
    assertSelfTest(
      selection.excluded_threads.some((item) => item.thread_id === "thread_wrong" && item.case_project_match === false),
      "wrong workspace candidate should be excluded"
    );
    assertSelfTest(
      selection.excluded_threads.some((item) => item.thread_id === "thread_missing_workspace" && item.case_project_match === false),
      "missing workspace candidate should be excluded"
    );

    const selectedAttempt = firstAttemptForTask(selection.threads, task);
    assertSelfTest(selectedAttempt?.thread_workspace_matches === true, "selected attempt should carry workspace match metadata");

    const wrongSelection = selectThreadEvidence({ threadId: "thread_wrong", threadsRoot }, caseRoot);
    const wrongAttempt = firstAttemptForTask(wrongSelection.threads, task);
    const wrongTask = applyThreadWorkspaceBinding({
      ...taskBase(task, wrongAttempt, {}),
      status: "ok",
      blockers: []
    }, caseRoot);
    assertSelfTest(
      wrongTask.blockers.includes("frontdoor_thread_workspace_mismatch"),
      "explicit wrong workspace thread should produce mismatch blocker"
    );

    const missingSelection = selectThreadEvidence({ threadId: "thread_missing_workspace", threadsRoot }, caseRoot);
    const missingAttempt = firstAttemptForTask(missingSelection.threads, task);
    const missingTask = applyThreadWorkspaceBinding({
      ...taskBase(task, missingAttempt, {}),
      status: "ok",
      blockers: []
    }, caseRoot);
    assertSelfTest(
      missingTask.blockers.includes("frontdoor_thread_workspace_missing"),
      "explicit missing workspace thread should produce missing-workspace blocker"
    );

    const result = {
      status: "ok",
      selected_thread_ids: selection.threads.map((thread) => thread.thread_id),
      excluded_thread_ids: selection.excluded_threads.map((thread) => thread.thread_id),
      wrong_workspace_blockers: wrongTask.blockers,
      missing_workspace_blockers: missingTask.blockers
    };
    if (json) console.log(JSON.stringify(result, null, 2));
    else console.log("frontdoor-p0-oracle workspace binding self-test ok");
    return result;
  } finally {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

function syntheticAcceptedFinalBundle({ threadId, turnId, answer, terminalReason = "success" }) {
  const acceptedAt = "2026-01-01T00:00:01.000Z";
  const { privateKey, publicKey } = crypto.generateKeyPairSync("ed25519");
  const publicDer = publicKey.export({ format: "der", type: "spki" });
  const publicRaw = publicDer.subarray(publicDer.length - 32);
  const record = {
    schemaVersion: 3,
    authorityPurpose: "analytix.case-final/v1",
    authorityAlgorithm: "Ed25519",
    authorityKeyId: sha256(publicRaw),
    authorityPublicKey: publicRaw.toString("base64url"),
    threadId,
    turnId,
    envelopeDigest: "1".repeat(64),
    contextDigest: "2".repeat(64),
    contextEpoch: 1,
    datasetSnapshotId: "dataset-self-test",
    variant: "GeneralGuidanceAnswer",
    terminalReason,
    renderedTextSha256: sha256(answer),
    registrySequence: 0,
    registryStateDigest: "3".repeat(64),
    rendererVersion: "analytix.host-final-renderer/v1",
    finalGateVersion: "analytix.final-evidence-gate/v2",
    verifierVersion: "analytix.claim-verifier-policy/v1",
    privateRecordDigest: "4".repeat(64),
    acceptedAt,
    authoritySignature: "",
    recordDigest: ""
  };
  const signingDigest = crypto.createHash("sha256")
    .update(goJSONStringify(record))
    .digest();
  record.authoritySignature = crypto.sign(
    null,
    Buffer.concat([ACCEPTED_FINAL_SIGNATURE_DOMAIN, signingDigest]),
    privateKey
  ).toString("base64url");
  record.recordDigest = sha256(goJSONStringify({ ...record, recordDigest: "" }));
  const view = {
    schemaVersion: 1,
    publicationState: "accepted",
    acceptedFinalDigest: record.recordDigest,
    envelopeDigest: record.envelopeDigest,
    contextDigest: record.contextDigest,
    contextEpoch: record.contextEpoch,
    datasetSnapshotId: record.datasetSnapshotId,
    variant: record.variant,
    terminalReason: record.terminalReason,
    blockerCode: "",
    coverageStatus: "guidance_only",
    checkedScopeDigest: "",
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: "masked_metadata_only",
      count: 0,
      setDigest: "5".repeat(64),
      citations: []
    },
    noHitWording: "",
    envelopeIssuedAt: acceptedAt,
    acceptedAt
  };
  const item = {
    id: `item_${turnId}_assistant`,
    turnId,
    threadId,
    role: "assistant",
    status: "completed",
    createdAt: acceptedAt,
    finishedAt: acceptedAt,
    kind: "assistant_text",
    text: answer,
    acceptedFinal: record,
    acceptedFinalView: view
  };
  const assistantEvent = {
    kind: "item_completed",
    threadId,
    turnId,
    itemId: item.id,
    item,
    publicationCommitId: record.recordDigest,
    publicationEventId: acceptedFinalPublicationEventId(record.recordDigest, "assistant-final"),
    publicationSlot: "assistant-final",
    acceptedFinalDigest: record.recordDigest,
    timestamp: acceptedAt,
    seq: 1
  };
  assistantEvent.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(assistantEvent);
  const terminalStatus = ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON[terminalReason];
  const terminalEvent = {
    kind: `turn_${terminalStatus}`,
    threadId,
    turnId,
    status: terminalStatus,
    acceptedFinalDigest: record.recordDigest,
    terminalReason,
    publicationCommitId: record.recordDigest,
    publicationEventId: acceptedFinalPublicationEventId(record.recordDigest, "terminal"),
    publicationSlot: "terminal",
    timestamp: acceptedAt,
    seq: 2
  };
  if (terminalStatus === "aborted") {
    terminalEvent.discard = true;
    terminalEvent.cancelled = terminalReason === "cancel";
    terminalEvent.cancelledPendingGates = 0;
  }
  terminalEvent.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(terminalEvent);
  return { item, events: [assistantEvent, terminalEvent] };
}

function runReplayNormalizationSelfTest({ json = false } = {}) {
  const threadId = "thread_replay_self_test";
  const turnId = "turn_1";
  const user = {
    id: "user_1",
    kind: "user_message",
    threadId,
    turnId,
    text: "资金交易最大的户名是谁",
    status: "completed",
    createdAt: "2026-01-01T00:00:00.000Z"
  };
  const accepted = syntheticAcceptedFinalBundle({ threadId, turnId, answer: "完整最终回答" });
  const acceptedItems = normalizeThreadItems({ messages: [user, accepted.item], events: accepted.events });
  const acceptedAttempt = firstAttemptForTask([{
    thread_id: threadId,
    items: acceptedItems,
    events: accepted.events
  }], TASKS[1]);
  assertSelfTest(acceptedAttempt?.assistant_answer === "完整最终回答", "only the complete host accepted final should be observable");
  assertSelfTest(acceptedAttempt?.accepted_final_count === 1, "complete accepted-final publication bundle should be counted exactly once");

  const injectedUser = {
    ...user,
    text: `${user.text}；原始数据包含 <think>不可信提示</think> 与 reasoning_content: 字段`
  };
  const injectedUserAttempt = firstAttemptForTask([{
    thread_id: threadId,
    items: normalizeThreadItems({ messages: [injectedUser, accepted.item], events: accepted.events }),
    events: accepted.events
  }], TASKS[1]);
  assertSelfTest(
    injectedUserAttempt?.assistant_reasoning_seen === false,
    "untrusted user-authored prompt-injection text must not be misclassified as host reasoning leakage"
  );

  const signedReasoningBundle = syntheticAcceptedFinalBundle({
    threadId,
    turnId,
    answer: "<think>private chain of thought</think>公开回答"
  });
  const signedReasoningAttempt = firstAttemptForTask([{
    thread_id: threadId,
    items: normalizeThreadItems({ messages: [user, signedReasoningBundle.item], events: signedReasoningBundle.events }),
    events: signedReasoningBundle.events
  }], TASKS[1]);
  assertSelfTest(
    signedReasoningAttempt?.assistant_answer === "",
    "even a signed bundle containing private reasoning text must remain quarantined"
  );

  const deltaEvents = [{
    kind: "assistant_text_delta",
    threadId,
    turnId,
    delta: "伪造金额 2,645,472 元",
    item: {
      id: "assistant_partial",
      kind: "assistant_text",
      threadId,
      turnId,
      text: "伪造金额 2,645,472 元",
      status: "running"
    }
  }];
  const deltaAttempt = firstAttemptForTask([{
    thread_id: threadId,
    items: normalizeThreadItems({ messages: [user], events: deltaEvents }),
    events: deltaEvents
  }], TASKS[1]);
  assertSelfTest(deltaAttempt?.assistant_answer === "", "delta-only replay must never become an answer fallback");
  assertSelfTest(deltaAttempt?.assistant_delta_seen === true, "delta-only replay must remain an explicit blocker");

  const unsignedItem = {
    id: `item_${turnId}_assistant`,
    kind: "assistant_text",
    role: "assistant",
    threadId,
    turnId,
    text: "无签名最终回答",
    status: "completed"
  };
  const unsignedAttempt = firstAttemptForTask([{
    thread_id: threadId,
    items: normalizeThreadItems({ messages: [user, unsignedItem], events: [] }),
    events: []
  }], TASKS[1]);
  assertSelfTest(unsignedAttempt?.assistant_answer === "", "completed text without accepted-final authority must remain quarantined");

  const reasoningEvents = [...accepted.events, {
    kind: "assistant_reasoning_delta",
    threadId,
    turnId,
    delta: "private chain of thought"
  }];
  const reasoningAttempt = firstAttemptForTask([{
    thread_id: threadId,
    items: acceptedItems,
    events: reasoningEvents
  }], TASKS[1]);
  const reasoningGate = passFail({ frontdoor_attempt: reasoningAttempt }, []);
  assertSelfTest(
    reasoningGate.blockers.includes("frontdoor_reasoning_persisted_or_replayed"),
    "reasoning persistence must fail the oracle even beside an accepted final"
  );

  const mismatchedEvents = accepted.events.map((event) => ({ ...event }));
  mismatchedEvents[1].acceptedFinalDigest = "9".repeat(64);
  const mismatchedAttempt = firstAttemptForTask([{
    thread_id: threadId,
    items: acceptedItems,
    events: mismatchedEvents
  }], TASKS[1]);
  assertSelfTest(mismatchedAttempt?.assistant_answer === "", "mismatched terminal publication must quarantine the candidate final");

  const attemptForBundle = (bundle) => firstAttemptForTask([{
    thread_id: threadId,
    items: normalizeThreadItems({ messages: [user, bundle.events[0].item], events: bundle.events }),
    events: bundle.events
  }], TASKS[1]);
  const acceptedTerminalReasons = [];
  for (const terminalReason of Object.keys(ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON)) {
    const bundle = syntheticAcceptedFinalBundle({
      threadId,
      turnId,
      answer: `终止路径 ${terminalReason}`,
      terminalReason
    });
    const attempt = attemptForBundle(bundle);
    assertSelfTest(
      attempt?.accepted_final_count === 1,
      `complete ${terminalReason} accepted-final bundle should pass normalization`
    );
    acceptedTerminalReasons.push(terminalReason);
  }

  const cloneBundle = () => JSON.parse(JSON.stringify(accepted));
  const recomputePayload = (event) => {
    event.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(event);
  };
  const mutations = [
    ["signature", (bundle) => {
      bundle.events[0].item.acceptedFinal.authoritySignature = "A".repeat(86);
      recomputePayload(bundle.events[0]);
    }],
    ["text_hash", (bundle) => {
      bundle.events[0].item.text = "被篡改回答";
      recomputePayload(bundle.events[0]);
    }],
    ["accepted_view", (bundle) => {
      bundle.events[0].item.acceptedFinalView.contextDigest = "8".repeat(64);
      recomputePayload(bundle.events[0]);
    }],
    ["assistant_event_id", (bundle) => {
      bundle.events[0].publicationEventId = "7".repeat(64);
      recomputePayload(bundle.events[0]);
    }],
    ["assistant_payload_digest", (bundle) => {
      bundle.events[0].publicationPayloadDigest = "6".repeat(64);
    }],
    ["assistant_sequence_type", (bundle) => {
      bundle.events[0].seq = "1";
    }],
    ["terminal_status", (bundle) => {
      bundle.events[1].kind = "turn_failed";
      bundle.events[1].status = "failed";
      recomputePayload(bundle.events[1]);
    }],
    ["terminal_reason", (bundle) => {
      bundle.events[1].terminalReason = "provider_failure";
      recomputePayload(bundle.events[1]);
    }],
    ["terminal_event_id", (bundle) => {
      bundle.events[1].publicationEventId = "5".repeat(64);
      recomputePayload(bundle.events[1]);
    }],
    ["terminal_payload_digest", (bundle) => {
      bundle.events[1].publicationPayloadDigest = "4".repeat(64);
    }],
    ["terminal_sequence_order", (bundle) => {
      bundle.events[1].seq = 1;
    }],
    ["duplicate_assistant_publication", (bundle) => {
      bundle.events.push(JSON.parse(JSON.stringify(bundle.events[0])));
    }]
  ];
  const rejectedMutations = [];
  for (const [label, mutate] of mutations) {
    const bundle = cloneBundle();
    mutate(bundle);
    const attempt = attemptForBundle(bundle);
    assertSelfTest(attempt?.assistant_answer === "", `${label} mutation must quarantine the candidate final`);
    rejectedMutations.push(label);
  }

  const result = {
    status: "ok",
    accepted_answer: acceptedAttempt.assistant_answer,
    delta_answer: deltaAttempt.assistant_answer,
    unsigned_answer: unsignedAttempt.assistant_answer,
    mismatched_answer: mismatchedAttempt.assistant_answer,
    reasoning_blockers: reasoningGate.blockers,
    user_prompt_injection_reasoning_seen: injectedUserAttempt.assistant_reasoning_seen,
    signed_reasoning_answer: signedReasoningAttempt.assistant_answer,
    accepted_terminal_reasons: acceptedTerminalReasons,
    rejected_mutations: rejectedMutations
  };
  if (json) console.log(JSON.stringify(result, null, 2));
  else console.log("frontdoor-p0-oracle replay normalization self-test ok");
  return result;
}

function writeSyntheticPluginInstall(root, version) {
  const manifestDir = path.join(root, ".codex-plugin");
  fs.mkdirSync(manifestDir, { recursive: true });
  fs.writeFileSync(path.join(manifestDir, "plugin.json"), JSON.stringify({
    name: "analytix-fund-analysis",
    version
  }, null, 2), "utf8");
}

function runInstalledVersionScanSelfTest({ json = false } = {}) {
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "frontdoor-p0-oracle-installed-"));
  try {
    const legacyRoot = path.join(
      tempRoot,
      "runtime",
      "plugins",
      "cache",
      "analytix-hub",
      "analytix-fund-analysis",
      "0.16.9"
    );
    const marketplaceRoot = path.join(
      tempRoot,
      "runtime",
      ".cache",
      "analytix-hub-plugins",
      "marketplaces",
      "analytix-hub",
      "plugins",
      "analytix-fund-analysis",
      "0.16.9-local-test"
    );
    writeSyntheticPluginInstall(legacyRoot, "0.16.9");
    writeSyntheticPluginInstall(marketplaceRoot, "0.16.9");

    const installed = findInstalledPluginVersions([
      path.join(tempRoot, "runtime", "plugins", "cache"),
      path.join(tempRoot, "runtime", ".cache", "analytix-hub-plugins", "marketplaces")
    ]);
    const matching = installed.filter((item) => item.version === "0.16.9");
    assertSelfTest(matching.length === 2, "direct and generated marketplace plugin installs should both be scanned");
    assertSelfTest(
      matching.some((item) => sameFsPath(item.path, legacyRoot)),
      "direct plugin cache install should be present"
    );
    assertSelfTest(
      matching.some((item) => sameFsPath(item.path, marketplaceRoot)),
      "generated marketplace plugin install should be present"
    );

    const result = {
      status: "ok",
      scanned_paths: matching.map((item) => item.path).sort()
    };
    if (json) console.log(JSON.stringify(result, null, 2));
    else console.log("frontdoor-p0-oracle installed version scan self-test ok");
    return result;
  } finally {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

function runReportContainmentSelfTest({ json = false } = {}) {
  const task = TASKS.find((item) => item.id === "G");
  assertSelfTest(Boolean(task), "report quarantine task should exist");

  const signedAttempt = (threadId, turnId, answer, toolCalls = []) => {
    const bundle = syntheticAcceptedFinalBundle({ threadId, turnId, answer });
    const user = {
      id: `${turnId}_user`, kind: "user_message", role: "user", status: "completed",
      threadId, turnId, text: task.question, createdAt: "2026-01-01T00:00:00.000Z"
    };
    const toolItems = toolCalls.map((call, index) => ({
      id: `${turnId}_tool_${index}`, kind: "tool_call", role: "assistant", status: "completed",
      threadId, turnId, toolName: call.tool_name, arguments: objectOf(call.arguments),
      callId: `${turnId}_call_${index}`, createdAt: "2026-01-01T00:00:00.500Z"
    }));
    return firstAttemptForTask([{
      thread_id: threadId,
      items: normalizeThreadItems({ messages: [user, ...toolItems, bundle.item], events: bundle.events }),
      events: bundle.events
    }], task);
  };

  const safe = auditTaskG(task, signedAttempt(
    "thread_report_safe",
    "turn_report_safe",
    PUBLICATION_RECEIPT_REQUIRED_TEXT
  ), {});
  assertSelfTest(safe.status === "ok", "fixed host boundary with no tools, artifacts, or facts should pass");
  assertSelfTest(safe.report_boundary_exact === true, "safe report response should match the fixed boundary exactly");
  assertSelfTest(safe.report_tool_call_count === 0, "safe report response should have zero tool calls");
  assertSelfTest(safe.report_artifact_signal_count === 0, "safe report response should have zero artifact signals");
  assertSelfTest(safe.report_case_fact_signal_count === 0, "safe report response should have zero case-fact signals");

  const legacy = auditTaskG(task, signedAttempt(
    "thread_report_legacy",
    "turn_report_legacy",
    "已生成正式报告：/tmp/case-report.pdf。共发现 2,645,472 条交易。",
    [{
      tool_name: "run_full_case_analysis",
      arguments: { write_report: true }
    }]
  ), {});
  assertSelfTest(legacy.status === "failed", "legacy report-tool/file oracle must not produce a release pass");
  assertSelfTest(legacy.blockers.includes("report_quarantine_requires_zero_tool_calls"), "legacy report tool call should block");
  assertSelfTest(legacy.blockers.includes("report_quarantine_requires_zero_artifacts"), "legacy report artifact should block");
  assertSelfTest(legacy.blockers.includes("report_quarantine_requires_zero_case_facts"), "legacy report facts should block");
  assertSelfTest(legacy.blockers.includes("report_quarantine_requires_fixed_host_boundary"), "legacy free-form report answer should block");
  assertSelfTest(legacy.frontdoor_attempt.assistant_answer === "", "unsafe legacy report answer should be quarantined from oracle evidence");

  const freeform = auditTaskG(task, signedAttempt(
    "thread_report_freeform",
    "turn_report_freeform",
    "当前无法生成报告，请稍后重试。"
  ), {});
  assertSelfTest(freeform.status === "failed", "non-canonical free-form refusal must not bypass the fixed host boundary");
  assertSelfTest(freeform.blockers.includes("report_quarantine_requires_fixed_host_boundary"), "free-form refusal should fail exact-boundary validation");

  const result = {
    status: "ok",
    contract: "ReportP0FixedBoundaryOnly",
    safe_status: safe.status,
    legacy_status: legacy.status,
    legacy_blockers: legacy.blockers,
    freeform_status: freeform.status,
    freeform_blockers: freeform.blockers
  };
  if (json) console.log(JSON.stringify(result, null, 2));
  else console.log("frontdoor-p0-oracle report containment self-test ok");
  return result;
}

function inferSqlPolicyFromCompactText(value) {
  const body = text(value);
  if (!body) return undefined;
  if (!/(专项核算策略|sql_policy|只读|read-?only|DuckDB\s+EXPLAIN|parser\/binder|预检|清洗\/分析|cleaned.*analysis)/iu.test(body)) {
    return undefined;
  }
  return {
    observed_in_compact_text: true,
    readonly: /(只读|read-?only)/iu.test(body),
    current_case_only: /(当前案件|current case)/iu.test(body),
    parser_binder_validated: /(parser\/binder|EXPLAIN|预检通过|parser_binder_validated=true)/iu.test(body),
    allowed_view_policy: /(清洗\/分析|cleaned.*analysis|cleaned_and_analysis_only)/iu.test(body) ? "cleaned_and_analysis_only" : "",
    excerpt: body.slice(0, 300)
  };
}

function inferEvidenceCardFromCompactText(value) {
  const body = text(value);
  if (!body) return undefined;
  if (!/(证据卡|evidence_card|source_hash|来源指纹|validated evidence|来源支撑)/iu.test(body)) {
    return undefined;
  }
  const sourceHash = body.match(/source_hash[=：:]\s*([a-f0-9]{12,64})/iu)?.[1]
    || body.match(/来源指纹[=：:]\s*([a-f0-9]{12,64})/iu)?.[1]
    || "";
  return {
    observed_in_compact_text: true,
    source_hash: sourceHash,
    excerpt: body.slice(0, 300)
  };
}

function inferValidationStateFromCompactText(value) {
  const body = text(value);
  if (!body) return undefined;
  if (!/(验证摘要|验证状态|validation_state|current_case_only|readonly|parser_binder_validated|执行状态:\s*executed|执行状态：\s*executed)/iu.test(body)) {
    return undefined;
  }
  return {
    observed_in_compact_text: true,
    current_case_only: /(current_case_only=true|当前案件|current case)/iu.test(body),
    readonly: /(readonly=true|只读|read-?only)/iu.test(body),
    parser_binder_validated: /(parser_binder_validated=true|parser\/binder|EXPLAIN|预检通过)/iu.test(body),
    excerpt: body.slice(0, 300)
  };
}

function collectToolResultFacts(attempt) {
  const facts = [];
  for (const result of arrayOf(attempt?.tool_results)) {
    const raw = result.output;
    const rawObject = objectOf(raw);
    const mcpResult = objectOf(rawObject.result);
    const structured = objectOf(mcpResult.structuredContent);
    const mcpContentText = arrayOf(mcpResult.content).map((item) => text(objectOf(item).text)).join("\n").trim();
    const directResultText = typeof rawObject.result === "string" ? rawObject.result : "";
    const outputText = mcpContentText || text(directResultText || rawObject.text || rawObject.output);
    facts.push({
      tool_name: result.tool_name,
      call_id: result.call_id,
      compact_text: outputText,
      structured,
      top_n_contract: deepFindFirst(raw, "top_n_contract"),
      sql_policy: deepFindFirst(raw, "sql_policy") || inferSqlPolicyFromCompactText(outputText),
      evidence_card: deepFindFirst(raw, "evidence_card") || inferEvidenceCardFromCompactText(outputText),
      validation_state: deepFindFirst(raw, "validation_state") || inferValidationStateFromCompactText(outputText),
      evidence_ledger: objectOf(objectOf(mcpResult._meta).analytix_evidence_ledger)
    });
  }
  return facts;
}

function topNContractFromAttempt(attempt) {
  const contracts = [];
  for (const call of arrayOf(attempt?.tool_calls)) {
    if (["rank_counterparties", "trace_subject_top_outflows", "run_case_sql"].includes(call.tool_name)) {
      contracts.push({
        tool_name: call.tool_name,
        limit: call.args.limit,
        top_n: call.args.top_n,
        requested_limit: call.args.requested_limit,
        source: "tool_args"
      });
    }
  }
  for (const fact of collectToolResultFacts(attempt)) {
    if (!["rank_counterparties", "trace_subject_top_outflows", "run_case_sql"].includes(fact.tool_name)) continue;
    const contract = objectOf(fact.top_n_contract);
    if (Number(contract.requested_limit || 0) > 0) {
      contracts.push({
        tool_name: fact.tool_name,
        limit: contract.resolved_limit,
        top_n: contract.resolved_limit,
        requested_limit: contract.requested_limit,
        returned_count: contract.returned_count,
        source: "tool_result_top_n_contract"
      });
    }
  }
  return contracts;
}

function deepFindFirst(value, key) {
  if (!value || typeof value !== "object") return undefined;
  if (Object.prototype.hasOwnProperty.call(value, key)) return value[key];
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = deepFindFirst(item, key);
      if (found !== undefined) return found;
    }
    return undefined;
  }
  for (const item of Object.values(value)) {
    const found = deepFindFirst(item, key);
    if (found !== undefined) return found;
  }
  return undefined;
}

function visibleNumbers(answer) {
  const body = text(answer);
  return [...body.matchAll(/(?<![\w.])(\d[\d,]*(?:\.\d+)?)(?:\s*(?:条|笔|元|万元|亿元|个|户|%))?/gu)]
    .map((match) => match[0])
    .slice(0, 80);
}

function visibleAmountValues(answer) {
  const body = text(answer);
  const values = [];
  for (const match of body.matchAll(/(?<![\w.])(\d[\d,]*(?:\.\d+)?)(?:\s*(亿元|万元|元))?/gu)) {
    const number = Number(text(match[1]).replace(/,/gu, ""));
    if (!Number.isFinite(number)) continue;
    const unit = text(match[2]);
    const multiplier = unit === "亿元" ? 100000000 : unit === "万元" ? 10000 : 1;
    values.push(number * multiplier);
  }
  return values;
}

function answerContainsAmount(answer, amount) {
  const expected = Number(amount);
  if (!Number.isFinite(expected)) return false;
  const compact = text(answer).replace(/[,\s]/gu, "");
  if (compact.includes(String(Math.round(expected)))) return true;
  const tolerance = Math.max(1, Math.abs(expected) * 0.001);
  return visibleAmountValues(answer).some((value) => Math.abs(value - expected) <= tolerance);
}

function answerContainsBusinessText(answer, value) {
  const expected = text(value);
  if (!expected) return true;
  const compactAnswer = text(answer).replace(/\s+/gu, "");
  const compactExpected = expected.replace(/\s+/gu, "");
  return compactAnswer.includes(compactExpected);
}

function topCounterpartyMismatches(answer, rows, limit = 10) {
  const body = text(answer);
  const mismatches = [];
  for (const row of arrayOf(rows).slice(0, limit)) {
    const rank = Number(row.rank || mismatches.length + 1);
    const name = text(row.counterparty_name || row.display_name || row.counterparty_key);
    if (name && !answerContainsBusinessText(body, name)) {
      mismatches.push(`rank_${rank}_name_expected_${name}`);
      continue;
    }
    const amount = numberValue(row.outflow_total ?? row.inflow_total ?? row.turnover_total ?? row.metric_value);
    if (Number.isFinite(amount) && !answerContainsAmount(body, amount)) {
      mismatches.push(`rank_${rank}_amount_expected_${amount.toFixed(2)}`);
    }
  }
  return mismatches;
}

function visibleRowCount(answer) {
  const lines = text(answer).split(/\r?\n/u).map((line) => line.trim()).filter(Boolean);
  const numbered = lines.filter((line) => /^(?:\d+[.、]|第\s*\d+|[-*]\s*\d+|[-*]\s*第\s*\d+)/u.test(line)).length;
  const tableRows = lines.filter((line) => /^\|.+\|$/u.test(line) && !/^\|\s*-+/u.test(line) && !/户名|对手方|金额|指标/u.test(line)).length;
  return Math.max(numbered, tableRows);
}

function leakageFindings(answer) {
  const body = text(answer);
  return INTERNAL_LEAK_PATTERNS
    .filter((pattern) => pattern.test(body))
    .map((pattern) => pattern.source);
}

function hasTool(attempt, names) {
  const set = new Set(names);
  return arrayOf(attempt?.tool_calls).some((call) => set.has(call.tool_name));
}

function toolNames(attempt) {
  return [...new Set(arrayOf(attempt?.tool_calls).map((call) => call.tool_name).filter(Boolean))];
}

function matchingSignalIds(value, definitions) {
  const body = text(value);
  return definitions
    .filter(({ pattern }) => pattern.test(body))
    .map(({ id }) => id);
}

function reportArtifactSignals(attempt) {
  return matchingSignalIds(JSON.stringify(attempt || {}), REPORT_ARTIFACT_SIGNAL_PATTERNS);
}

function reportCaseFactSignals(answer) {
  return matchingSignalIds(answer, REPORT_CASE_FACT_SIGNAL_PATTERNS);
}

function taskBase(task, attempt, crossChecks) {
  const toolResultFacts = collectToolResultFacts(attempt);
  const toolsUsed = toolNames(attempt);
  return {
    id: task.id,
    title: task.title,
    question: task.question,
    expected_tools: task.expectedTools,
    frontdoor_attempt: attempt ? {
      thread_id: attempt.thread_id,
      thread_kind: attempt.thread_kind,
      thread_workspace: attempt.thread_workspace,
      thread_workspace_resolved: attempt.thread_workspace_resolved,
      thread_workspace_matches: attempt.thread_workspace_matches,
      thread_evidence_mtime_ms: attempt.thread_evidence_mtime_ms,
      thread_evidence_mtime_iso: attempt.thread_evidence_mtime_iso,
      turn_id: attempt.turn_id,
      user_message: attempt.user_message,
      assistant_answer: attempt.assistant_answer,
      accepted_final_count: attempt.accepted_final_count,
      accepted_final_digests: attempt.accepted_final_digests,
      assistant_delta_seen: attempt.assistant_delta_seen,
      unaccepted_assistant_text_seen: attempt.unaccepted_assistant_text_seen,
      assistant_reasoning_seen: attempt.assistant_reasoning_seen,
      tool_calls: attempt.tool_calls,
      tool_result_summary: toolResultFacts.map((fact) => ({
        tool_name: fact.tool_name,
        call_id: fact.call_id,
        compact_text_sha256: sha256(fact.compact_text),
        compact_text_excerpt: fact.compact_text.slice(0, 500),
        has_sql_policy: Boolean(fact.sql_policy),
        has_evidence_card: Boolean(fact.evidence_card),
        has_validation_state: Boolean(fact.validation_state)
      }))
    } : null,
    source_selection_reason: toolsUsed.length
      ? `front-door selected ${toolsUsed.join(", ")}`
      : "no real front-door tool evidence found",
    lead_owner: inferLeadOwner(toolsUsed),
    mcp_tools_used: toolsUsed,
    workbench_used: toolsUsed.some((name) => /(?:run_case_sql|inspect_case_schema|count_case_rows|profile_case_schema|case_sql_recipes|explain_case_sql|diagnose_case_sql|create_case_notebook)/u.test(name)),
    sql_policy: toolResultFacts.map((fact) => fact.sql_policy).filter(Boolean),
    evidence_card: toolResultFacts.map((fact) => fact.evidence_card).filter(Boolean),
    validation_state: toolResultFacts.map((fact) => fact.validation_state).filter(Boolean),
    final_answer_observed: attempt ? {
      visible_numbers: visibleNumbers(attempt.assistant_answer),
      visible_top_rows: visibleRowCount(attempt.assistant_answer),
      leakage: leakageFindings(attempt.assistant_answer)
    } : null,
    duckdb_cross_check: crossChecks
  };
}

function inferLeadOwner(tools) {
  if (tools.includes("investigate_pair_amount")) return "pair-amount-investigation";
  if (tools.includes("rank_holders")) return "holder-ranking";
  if (tools.includes("rank_counterparties")) return "counterparty-ranking";
  if (tools.includes("trace_subject_top_outflows") || tools.includes("trace_fund_next_hop") || tools.includes("build_fund_flow_graph")) return "fund-tracing";
  if (tools.includes("run_case_sql")) return "controlled-case-workbench";
  if (tools.includes("run_full_case_analysis")) return "full-case-analysis";
  if (tools.length) return tools[0];
  return "unresolved";
}

function passFail(base, failures, extra = {}) {
  const attempt = objectOf(base.frontdoor_attempt);
  const authorityFailures = [];
  if (base.frontdoor_attempt) {
    if (Number(attempt.accepted_final_count) !== 1) {
      authorityFailures.push("frontdoor_requires_exactly_one_host_accepted_final");
    }
    if (attempt.assistant_delta_seen === true || attempt.unaccepted_assistant_text_seen === true) {
      authorityFailures.push("frontdoor_unaccepted_assistant_draft_persisted");
    }
    if (attempt.assistant_reasoning_seen === true) {
      authorityFailures.push("frontdoor_reasoning_persisted_or_replayed");
    }
  }
  const blockers = [...new Set([...failures, ...authorityFailures])];
  return {
    ...base,
    ...extra,
    status: blockers.length ? "failed" : "ok",
    blockers
  };
}

function auditTaskA(task, attempt, oracle) {
  const base = taskBase(task, attempt, {
    sql: "SELECT COUNT(*) AS txn_count FROM analysis_txn_detail_idx",
    result: oracle.core_table_counts
  });
  const failures = [];
  const expected = Number(oracle.core_table_counts.analysis_txn_detail_idx);
  if (!attempt) failures.push("frontdoor_thread_evidence_missing");
  if (attempt && !hasTool(attempt, ["count_case_rows", "get_scope_coverage", "inspect_case_schema"])) failures.push("count_question_did_not_use_allowed_data_tool");
  const answerDigits = text(attempt?.assistant_answer).replace(/[,\s]/gu, "");
  if (attempt && !answerDigits.includes(String(expected))) failures.push(`answer_count_mismatch_expected_${expected}`);
  if (attempt && /preview/iu.test(text(attempt.assistant_answer)) && !/不能支持总量|不支持总量|总量/u.test(text(attempt.assistant_answer))) failures.push("preview_used_without_total_boundary");
  for (const leak of leakageFindings(attempt?.assistant_answer)) failures.push(`user_visible_leakage:${leak}`);
  return passFail(base, failures);
}

function auditTaskB(task, attempt, oracle) {
  const base = taskBase(task, attempt, {
    metric: "turnover_total",
    sql: "holder turnover aggregate over analysis_txn_detail_idx",
    top: oracle.holder_rank_turnover_top10[0]
  });
  const failures = [];
  const top = objectOf(oracle.holder_rank_turnover_top10[0]);
  if (!attempt) failures.push("frontdoor_thread_evidence_missing");
  if (attempt && !hasTool(attempt, ["rank_holders", "run_case_sql"])) failures.push("holder_ranking_question_did_not_use_rank_holders_or_workbench");
  const answer = text(attempt?.assistant_answer);
  if (attempt && !answer.includes(text(top.holder_name))) failures.push(`top_holder_mismatch_expected_${text(top.holder_name)}`);
  if (attempt && !/(交易总额|总交易额|往来总额|总流水|周转额|流入\s*\+\s*流出|流入\+流出|汇入\s*\+\s*汇出|汇入\+汇出|入账\s*\+\s*出账|入账\+出账|流入金额.*流出金额.*(?:之和|合计)|流入.*流出.*(?:之和|合计)|汇入.*汇出.*(?:之和|合计)|turnover|累计交易)/iu.test(answer)) failures.push("metric_not_stated_for_largest_holder");
  if (attempt && !/(笔|账户|账号|户)/u.test(answer)) failures.push("answer_missing_count_or_account_scope");
  if (attempt && /(单笔最大|最大单笔)/u.test(answer)) failures.push("turnover_question_miswritten_as_max_single_transaction");
  for (const leak of leakageFindings(answer)) failures.push(`user_visible_leakage:${leak}`);
  return passFail(base, failures);
}

function auditTaskC(task, attempt, oracle) {
  const base = taskBase(task, attempt, {
    sql: "external outflow counterparty Top 10 aggregate",
    result: oracle.destination_outflow_top10
  });
  const failures = [];
  if (!attempt) failures.push("frontdoor_thread_evidence_missing");
  const topNContracts = topNContractFromAttempt(attempt);
  if (attempt && !topNContracts.some((item) => Number(item.limit || item.top_n || item.requested_limit) >= 10)) failures.push("requested_limit_10_not_preserved_in_tool_args");
  if (attempt && visibleRowCount(attempt.assistant_answer) < 10 && oracle.destination_outflow_top10.length >= 10) failures.push("final_answer_visible_rows_less_than_10");
  if (attempt) {
    for (const mismatch of topCounterpartyMismatches(attempt.assistant_answer, oracle.destination_outflow_top10, 10).slice(0, 5)) {
      failures.push(`top10_counterparty_content_mismatch:${mismatch}`);
    }
  }
  if (attempt && /默认返回前\s*5\s*位|前\s*5\s*位/u.test(attempt.assistant_answer)) failures.push("answer_mentions_default_top5");
  for (const leak of leakageFindings(attempt?.assistant_answer)) failures.push(`user_visible_leakage:${leak}`);
  return passFail(base, failures, { top_n_contracts: topNContracts });
}

function auditTaskD(task, attempt, oracle) {
  const base = taskBase(task, attempt, {
    seed_pair: oracle.pair_amount_seed,
    sql: oracle.pair_amount_sql,
    result: oracle.pair_amount_aggregate
  });
  const failures = [];
  if (!attempt) failures.push("frontdoor_thread_evidence_missing");
  if (attempt && !hasTool(attempt, ["investigate_pair_amount", "run_case_sql"])) failures.push("pair_amount_did_not_use_source_of_truth_tool");
  if (attempt && hasTool(attempt, ["rank_counterparties"]) && !hasTool(attempt, ["investigate_pair_amount", "run_case_sql"])) failures.push("rank_counterparties_used_as_final_pair_amount");
  const answer = text(attempt?.assistant_answer);
  const amount = numberValue(oracle.pair_amount_aggregate.raw_detail_amount);
  if (attempt && !answerContainsAmount(answer, amount)) failures.push("pair_amount_visible_amount_not_matching_sql_aggregate");
  if (attempt && !/(原始|明细|有效|去重|重复|时间|期间|账号|账户)/u.test(answer)) failures.push("pair_amount_answer_missing_scope_or_dedup_boundary");
  for (const leak of leakageFindings(answer)) failures.push(`user_visible_leakage:${leak}`);
  return passFail(base, failures);
}

function auditTaskE(task, attempt, oracle) {
  const base = taskBase(task, attempt, {
    sql: "2024 monthly holder outflow Top 10 with row_number partition",
    returned_rows: oracle.monthly_2024_outflow_top10_by_holder.length,
    first_rows: oracle.monthly_2024_outflow_top10_by_holder.slice(0, 10)
  });
  const failures = [];
  if (!attempt) failures.push("frontdoor_thread_evidence_missing");
  if (attempt && !hasTool(attempt, ["inspect_case_schema", "profile_case_schema"])) failures.push("custom_metric_missing_schema_or_profile_preflight");
  if (attempt && !hasTool(attempt, ["run_case_sql"])) failures.push("custom_metric_missing_controlled_workbench_sql");
  const facts = collectToolResultFacts(attempt);
  if (attempt && !facts.some((fact) => fact.sql_policy || fact.evidence_card || fact.validation_state)) failures.push("custom_metric_missing_sql_policy_or_evidence_card");
  for (const leak of leakageFindings(attempt?.assistant_answer)) failures.push(`user_visible_leakage:${leak}`);
  return passFail(base, failures);
}

function auditTaskF(task, attempt, oracle) {
  const base = taskBase(task, attempt, {
    seed_receiver: oracle.next_hop_seed_receiver,
    sql: "seed receiver outbound next-hop aggregate",
    result: oracle.next_hop_top_outflows
  });
  const failures = [];
  if (!attempt) failures.push("frontdoor_thread_evidence_missing");
  if (attempt && !hasTool(attempt, ["trace_subject_top_outflows", "trace_fund_next_hop", "build_fund_flow_graph", "run_case_sql"])) failures.push("next_hop_question_missing_trace_or_graph_tool");
  if (attempt && oracle.next_hop_top_outflows.length > 0 && !/(流向|去向|下游|一跳|资金断点|需复核|需补证)/u.test(text(attempt.assistant_answer))) failures.push("next_hop_answer_missing_investigative_graph_language");
  if (attempt && /candidate edge|候选边.*确认|confirmed edge/iu.test(text(attempt.assistant_answer))) failures.push("candidate_edge_written_as_confirmed");
  for (const leak of leakageFindings(attempt?.assistant_answer)) failures.push(`user_visible_leakage:${leak}`);
  return passFail(base, failures);
}

function auditTaskG(task, attempt, oracle) {
  const answer = text(attempt?.assistant_answer);
  const boundaryExact = answer === PUBLICATION_RECEIPT_REQUIRED_TEXT;
  const toolsUsed = toolNames(attempt);
  const artifactSignals = reportArtifactSignals(attempt);
  const caseFactSignals = reportCaseFactSignals(answer);
  const base = taskBase(task, attempt, {
    publication_boundary: "fixed_host_boundary_only",
    case_facts_allowed: false,
    artifacts_allowed: false,
    tool_calls_allowed: false
  });
  if (base.frontdoor_attempt) {
    base.frontdoor_attempt = {
      ...base.frontdoor_attempt,
      assistant_answer: boundaryExact ? answer : "",
      assistant_answer_sha256: sha256(answer),
      assistant_answer_quarantined: !boundaryExact,
      tool_calls: toolsUsed.map((toolName) => ({ tool_name: toolName })),
      tool_result_summary: []
    };
    base.final_answer_observed = {
      visible_numbers: [],
      visible_top_rows: 0,
      leakage: boundaryExact ? leakageFindings(answer) : []
    };
  }
  const failures = [];
  if (!attempt) failures.push("frontdoor_thread_evidence_missing");
  if (attempt && toolsUsed.length > 0) failures.push("report_quarantine_requires_zero_tool_calls");
  if (attempt && artifactSignals.length > 0) failures.push("report_quarantine_requires_zero_artifacts");
  if (attempt && caseFactSignals.length > 0) failures.push("report_quarantine_requires_zero_case_facts");
  if (attempt && !boundaryExact) failures.push("report_quarantine_requires_fixed_host_boundary");
  return passFail(base, failures, {
    report_boundary_exact: boundaryExact,
    report_tool_call_count: toolsUsed.length,
    report_artifact_signal_count: artifactSignals.length,
    report_artifact_signals: artifactSignals,
    report_case_fact_signal_count: caseFactSignals.length,
    report_case_fact_signals: caseFactSignals,
    inspected_artifacts: []
  });
}

function l5ReviewChecklist(tasks) {
  const answerBodies = tasks.map((task) => text(task.frontdoor_attempt?.assistant_answer)).join("\n\n");
  const leakage = leakageFindings(answerBodies);
  const failedTasks = tasks.filter((task) => task.status !== "ok");
  const reportTask = tasks.find((task) => task.id === "G");
  const missingWrittenAsZero = /(?:缺失|缺口|未取得|未固定|未覆盖)[^。\n]{0,24}(?:为|=|:|：)?\s*0\s*(?:条|笔|个|户|项|元)|没有数据[^。\n]{0,24}0\s*(?:条|笔|个|户|项|元)/u.test(answerBodies);
  const checks = [
    { id: "economic_investigation_material", passed: /结论|依据|异常|核查|补证|需复核/u.test(answerBodies), note: "是否像经侦研判而非工具说明" },
    { id: "has_conclusion_basis_abnormality_limits_actions", passed: /结论/u.test(answerBodies) && /依据|金额|笔/u.test(answerBodies) && /不能认定|需复核|补证|下一步/u.test(answerBodies), note: "结论、依据、异常、案件意义、不能认定、补证动作" },
    { id: "support_layer_hidden", passed: leakage.length === 0, note: leakage.join(", ") || "no forbidden support-layer terms detected" },
    { id: "candidate_not_confirmed", passed: !/候选.*确认|candidate.*confirmed/iu.test(answerBodies), note: "候选线索不得写成确认事实" },
    { id: "missing_not_zero", passed: !missingWrittenAsZero, note: "缺失数据不得写成零" },
    { id: "current_flow_not_ignored", passed: !/调取完整流水/u.test(answerBodies) || /当前案件|已有流水|现有流水/u.test(answerBodies), note: "不能忽略当前案件已有流水" },
    { id: "report_fixed_boundary_only", passed: reportTask?.report_boundary_exact === true, note: "P0 报告请求只能返回宿主固定能力边界" },
    { id: "report_zero_tool_calls", passed: reportTask?.report_tool_call_count === 0, note: "P0 报告请求不得调用隐藏或数据工具" },
    { id: "report_zero_artifacts", passed: reportTask?.report_artifact_signal_count === 0, note: "P0 报告请求不得生成、检查或返回文件/artifact" },
    { id: "report_zero_case_facts", passed: reportTask?.report_case_fact_signal_count === 0, note: "P0 报告请求不得编译或输出案件事实" }
  ];
  const failedChecks = checks.filter((check) => check.passed !== true).map((check) => check.id);
  return {
    status: failedTasks.length || leakage.length || failedChecks.length ? "failed" : "ok",
    checks,
    failed_task_ids: failedTasks.map((task) => task.id),
    failed_check_ids: failedChecks,
    leakage
  };
}

function writeEvidence(payload, outputDir = "") {
  const dir = outputDir
    ? path.resolve(REPO_ROOT, outputDir)
    : path.join(OUTPUT_ROOT, nowStamp());
  fs.mkdirSync(dir, { recursive: true });
  const jsonPath = path.join(dir, "frontdoor-p0-oracle.json");
  const mdPath = path.join(dir, "frontdoor-p0-oracle.md");
  fs.writeFileSync(jsonPath, JSON.stringify(payload, null, 2), "utf8");
  fs.writeFileSync(mdPath, renderMarkdown(payload), "utf8");
  return { dir, jsonPath, mdPath };
}

function renderMarkdown(payload) {
  const lines = [
    "# analytix-fund-analysis Frontdoor P0 Oracle",
    "",
    `- status: ${payload.status}`,
    `- case_id: ${payload.case.case_id}`,
    `- case_project_root: ${payload.case.case_project_root}`,
    `- case_duckdb_path: ${payload.case.case_duckdb_path}`,
    `- source_version: ${payload.versions.source.manifest_version}`,
    `- required_installed_version: ${payload.versions.require_installed_version || "(none)"}`,
    `- installed_requirement_met: ${payload.versions.installed_requirement_met}`,
    "",
    "## Task Results"
  ];
  for (const task of payload.tasks) {
    lines.push("");
    lines.push(`### ${task.id}. ${task.title} - ${task.status}`);
    if (task.blockers.length) {
      lines.push(`- blockers: ${task.blockers.join("; ")}`);
    }
    lines.push(`- thread: ${task.frontdoor_attempt?.thread_id || "(missing)"}`);
    lines.push(`- tools: ${task.mcp_tools_used.join(", ") || "(none)"}`);
    lines.push(`- lead_owner: ${task.lead_owner}`);
    lines.push(`- workbench_used: ${task.workbench_used}`);
    if (task.final_answer_observed) {
      lines.push(`- visible_rows: ${task.final_answer_observed.visible_top_rows}`);
      lines.push(`- leakage: ${task.final_answer_observed.leakage.join(", ") || "none"}`);
    }
  }
  lines.push("");
  lines.push("## L5 Review");
  lines.push(`- status: ${payload.l5_review.status}`);
  for (const check of payload.l5_review.checks) {
    lines.push(`- ${check.id}: ${check.passed ? "passed" : "failed"} - ${check.note}`);
  }
  if (payload.blockers.length) {
    lines.push("");
    lines.push("## Release Blockers");
    for (const blocker of payload.blockers) lines.push(`- ${blocker}`);
  }
  return `${lines.join("\n")}\n`;
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.selfTestWorkspaceBinding) {
    runWorkspaceBindingSelfTest({ json: options.json });
    return;
  }
  if (options.selfTestReplayNormalization) {
    runReplayNormalizationSelfTest({ json: options.json });
    return;
  }
  if (options.selfTestInstalledVersionScan) {
    runInstalledVersionScanSelfTest({ json: options.json });
    return;
  }
  if (options.selfTestReportContainment) {
    runReportContainmentSelfTest({ json: options.json });
    return;
  }
  const versions = collectVersionEvidence(options.requireInstalledVersion);
  const projectContext = resolveCaseProjectContext({}, {
    ...process.env,
    ANALYTIX_CASE_PROJECT_ROOT: options.caseProjectRoot
  });
  if (!projectContext) throw new Error("failed to resolve analytix case project context");
  if (projectContext.case_id !== options.caseId) {
    throw new Error(`case project binding mismatch: expected ${options.caseId}, got ${projectContext.case_id}`);
  }
  const caseDuckdbPath = resolveCaseDuckdbPath(options.caseId, process.env);
  if (!fs.existsSync(caseDuckdbPath)) throw new Error(`case DuckDB not found: ${caseDuckdbPath}`);
  const duckdb = buildDuckdbCrossChecks(caseDuckdbPath);
  const threadSelection = selectThreadEvidence(options, path.resolve(options.caseProjectRoot));
  const threads = threadSelection.threads;
  const threadFreshness = buildThreadFreshness(versions);
  const tasks = TASKS
    .map((task) => task.audit(task, firstAttemptForTask(threads, task), duckdb))
    .map((task) => applyThreadFreshness(task, threadFreshness))
    .map((task) => applyThreadWorkspaceBinding(task, path.resolve(options.caseProjectRoot)));
  const l5 = l5ReviewChecklist(tasks);
  const blockers = [
    ...(!versions.installed_requirement_met ? [`installed_version_requirement_not_met:${options.requireInstalledVersion}`] : []),
    ...tasks.flatMap((task) => task.blockers.map((blocker) => `${task.id}:${blocker}`)),
    ...(l5.status === "ok" ? [] : ["l5_review_failed"])
  ];
  const payload = {
    id: "analytix-fund-analysis-frontdoor-p0-oracle",
    generated_at: new Date().toISOString(),
    status: blockers.length ? "failed" : "ok",
    case: {
      case_id: options.caseId,
      case_project_root: path.resolve(options.caseProjectRoot),
      case_project_config: projectContext.case_project_config,
      case_project_binding: projectContext,
      case_duckdb_path: caseDuckdbPath,
      case_duckdb_size: fs.statSync(caseDuckdbPath).size
    },
    versions,
    thread_freshness: {
      ...threadFreshness,
      stale_task_ids: tasks
        .filter((task) => task.blockers.includes("frontdoor_thread_evidence_stale_before_installed_version"))
        .map((task) => task.id)
    },
    thread_collection: {
      requested_thread_id: options.threadId || "",
      selection_mode: threadSelection.mode,
      auto_discovered_candidate_count: threadSelection.candidate_threads.length,
      auto_discovered_selected_count: threads.length,
      auto_discovered_excluded_count: threadSelection.excluded_threads.length,
      excluded_thread_candidates: threadSelection.excluded_threads.slice(0, 50),
      discovered_threads: threads.map((thread) => ({
        thread_id: thread.thread_id,
        kind: thread.kind || "unknown",
        messages_count: thread.messages_count || 0,
        events_count: thread.events_count || 0,
        thread_workspace: thread.thread_workspace || "",
        thread_workspace_resolved: thread.thread_workspace_resolved || "",
        thread_workspace_matches: thread.thread_workspace_matches,
        evidence_mtime_ms: thread.evidence_mtime_ms || 0,
        evidence_mtime_iso: thread.evidence_mtime_iso || "",
        item_count: arrayOf(thread.items).length
      }))
    },
    duckdb_cross_checks: duckdb,
    tasks,
    l5_review: l5,
    blockers
  };
  const evidence = writeEvidence(payload, options.outputDir);
  payload.evidence = evidence;
  fs.writeFileSync(evidence.jsonPath, JSON.stringify(payload, null, 2), "utf8");
  fs.writeFileSync(evidence.mdPath, renderMarkdown(payload), "utf8");
  if (options.json) {
    console.log(JSON.stringify({
      status: payload.status,
      evidence,
      blockers,
      task_status: tasks.map((task) => ({ id: task.id, status: task.status, blockers: task.blockers }))
    }, null, 2));
  } else {
    console.log(`frontdoor-p0-oracle ${payload.status}: ${evidence.dir}`);
  }
  if (options.failOnGaps && blockers.length) process.exit(1);
}

main();
