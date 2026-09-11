#!/usr/bin/env node

import { spawn, spawnSync } from "node:child_process";
import fs from "node:fs";
import http from "node:http";
import https from "node:https";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY,
  FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
  FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI,
} from "../mcp/source-unavailable-boundary.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");
const ACTIVE_CASE_LOCK_STALE_MS = 10 * 60 * 1000;

const P0_REJECTED_FACT_TOOL = "get_current_case";

const TEXT_AUDIT_EXTENSIONS = new Set([
  ".json",
  ".md",
  ".mjs",
  ".js",
  ".cjs",
  ".yaml",
  ".yml",
  ".txt",
]);

const PACKAGE_LEAK_PATTERNS = [
  ["/Users/sun/Downloads", "Analytix_new"].join("/"),
  ["/Users/sun", ".codex"].join("/"),
  ["ANALYTIX_HUB_ADMIN", "AUTOMATION_TOKEN"].join("_"),
  ["HUB", "SSH", "KEY"].join("_"),
  ["HUB", "SSH", "TARGET"].join("_"),
];

const SECRET_TEXT_PATTERNS = [
  /(?:password|passwd|pwd)\s*[:=]\s*["']?[^"'\s]{8,}/iu,
  /(?:api[_-]?key|secret|token)\s*[:=]\s*["']?[A-Za-z0-9._~+/=-]{20,}/iu,
  /-----BEGIN (?:RSA |OPENSSH |EC |DSA )?PRIVATE KEY-----/u,
];

const PROCESS_ENV_ALLOWLIST = [
  "PATH",
  "Path",
  "PATHEXT",
  "SystemRoot",
  "WINDIR",
  "COMSPEC",
  "HOME",
  "USERPROFILE",
  "TMPDIR",
  "TEMP",
  "TMP",
];

function text(value) {
  return String(value || "").trim();
}

function parseArgs(argv) {
  const options = {
    backendUrl: "",
    caseId: "",
    caseProjectRoot: text(process.env.ANALYTIX_CASE_PROJECT_ROOT || process.env.ANALYTIX_WORKSPACE_ROOT),
    deep: false,
    reportDryRun: false,
    reportWriteSmoke: false,
    json: false,
    maxAccounts: 1,
    timeoutMs: 30_000,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--case-id") {
      options.caseId = text(argv[index + 1]);
      index += 1;
    } else if (arg.startsWith("--case-id=")) {
      options.caseId = text(arg.slice("--case-id=".length));
    } else if (arg === "--case-project-root") {
      options.caseProjectRoot = path.resolve(text(argv[index + 1]));
      index += 1;
    } else if (arg.startsWith("--case-project-root=")) {
      options.caseProjectRoot = path.resolve(text(arg.slice("--case-project-root=".length)));
    } else if (arg === "--backend-url") {
      options.backendUrl = text(argv[index + 1]);
      index += 1;
    } else if (arg.startsWith("--backend-url=")) {
      options.backendUrl = text(arg.slice("--backend-url=".length));
    } else if (arg === "--deep") {
      options.deep = true;
    } else if (arg === "--report-dry-run") {
      options.reportDryRun = true;
    } else if (arg === "--report-write-smoke") {
      options.reportWriteSmoke = true;
    } else if (arg === "--max-accounts") {
      options.maxAccounts = Number(argv[index + 1] || options.maxAccounts);
      index += 1;
    } else if (arg.startsWith("--max-accounts=")) {
      options.maxAccounts = Number(arg.slice("--max-accounts=".length));
    } else if (arg === "--json") {
      options.json = true;
    } else if (arg === "--timeout-ms") {
      options.timeoutMs = Number(argv[index + 1] || options.timeoutMs);
      index += 1;
    } else if (arg.startsWith("--timeout-ms=")) {
      options.timeoutMs = Number(arg.slice("--timeout-ms=".length));
    } else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }
  if (!Number.isFinite(options.timeoutMs) || options.timeoutMs < 1_000) {
    options.timeoutMs = 30_000;
  }
  if (!Number.isFinite(options.maxAccounts) || options.maxAccounts < 1) {
    options.maxAccounts = 1;
  }
  options.maxAccounts = Math.floor(options.maxAccounts);
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/check-health.mjs [options]

Options:
  --backend-url <url>   Analytix backend base URL. Defaults to .mcp.json env or http://127.0.0.1:18731.
  --case-project-root <dir>
                       analytix case-project workspace containing .analytix/case-project.json.
                       Defaults to ANALYTIX_CASE_PROJECT_ROOT / ANALYTIX_WORKSPACE_ROOT.
  --case-id <id>        Run current-case-project smoke checks. Must match the case-project binding.
  --deep                Also run scan_case_risks. This can take tens of seconds.
  --report-dry-run      Verify the P0-hidden full-case tool rejects write_report=false before execution.
  --report-write-smoke  Verify the P0-hidden full-case tool rejects write_report=true before execution.
  --max-accounts <n>    Account cap for report smoke checks. Default: 1.
  --timeout-ms <ms>     Per-request timeout. Default: 30000.
  --json                Print machine-readable JSON.
`);
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function normalizeBaseUrl(value) {
  return (text(value) || "http://127.0.0.1:18731").replace(/\/+$/u, "");
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function activeCaseLockPath(options = {}) {
  const caseSlug = (text(options.caseId) || "active")
    .replace(/[^A-Za-z0-9_.-]+/gu, "_")
    .slice(0, 80) || "active";
  const lockDir = path.join(os.tmpdir(), "analytix-fund-analysis");
  fs.mkdirSync(lockDir, { recursive: true });
  return path.join(lockDir, `check-health-${caseSlug}.lock`);
}

function processIsAlive(pid) {
  const numericPid = Number(pid || 0);
  if (!Number.isInteger(numericPid) || numericPid <= 0) {
    return false;
  }
  try {
    process.kill(numericPid, 0);
    return true;
  } catch (error) {
    return text(error?.code) === "EPERM";
  }
}

function readLock(lockPath) {
  try {
    return JSON.parse(fs.readFileSync(lockPath, "utf8"));
  } catch {
    return {};
  }
}

function staleActiveCaseLock(lockPath) {
  const lock = readLock(lockPath);
  const createdAt = Number(lock.created_at_ms || 0);
  return !processIsAlive(lock.pid)
    || !createdAt
    || Date.now() - createdAt > ACTIVE_CASE_LOCK_STALE_MS;
}

async function acquireActiveCaseLock(options = {}) {
  const lockPath = activeCaseLockPath(options);
  const deadline = Date.now() + Math.min(Math.max(Number(options.timeoutMs || 0), 5_000), 60_000);
  for (;;) {
    try {
      const fd = fs.openSync(lockPath, "wx");
      try {
        fs.writeFileSync(fd, JSON.stringify({
          pid: process.pid,
          case_id: text(options.caseId),
          created_at_ms: Date.now(),
          reason: "serialized active-case health check"
        }));
      } finally {
        fs.closeSync(fd);
      }
      return { lockPath };
    } catch (error) {
      if (text(error?.code) !== "EEXIST") {
        throw error;
      }
      if (staleActiveCaseLock(lockPath)) {
        fs.rmSync(lockPath, { force: true });
        continue;
      }
      if (Date.now() >= deadline) {
        throw new Error(`active-case health lock timed out: ${lockPath}`);
      }
      await sleep(200);
    }
  }
}

function releaseActiveCaseLock(lock) {
  const lockPath = text(lock?.lockPath);
  if (lockPath) {
    fs.rmSync(lockPath, { force: true });
  }
}

function hasExpectedCaseMissingError(error) {
  const message = error instanceof Error ? error.message : String(error || "");
  return /CASE_NOT_FOUND|case project|case-project/i.test(message);
}

function hasExpectedHiddenFullCaseError(error) {
  const message = error instanceof Error ? error.message : String(error || "");
  return /"code"\s*:\s*-32602[^\n]*Unknown or unadvertised tool/iu.test(message);
}

function hasExpectedInvalidParamsError(error, messagePattern) {
  const message = error instanceof Error ? error.message : String(error || "");
  return /"code"\s*:\s*-32602/iu.test(message) && messagePattern.test(message);
}

function exactStrings(values) {
  return [...values].map(text).filter(Boolean).sort();
}

async function runP0IsolationHealth(report, serverConfig, options) {
  if (serverConfig.disabled === true) {
    report.pass("mcp disabled", "production manifest disables process resolution");
  } else {
    report.fail("mcp disabled", "analytix_funds must remain disabled until host source authority is complete");
    return;
  }

  const backendUrl = normalizeBaseUrl(
    options.backendUrl || process.env.ANALYTIX_API_BASE_URL || serverConfig.env?.ANALYTIX_API_BASE_URL,
  );
  const isolatedConfig = {
    ...serverConfig,
    env: {
      ...(serverConfig.env && typeof serverConfig.env === "object" ? serverConfig.env : {}),
      ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS: "true",
    },
  };
  let mcp = null;
  try {
    // This explicit child is a protocol conformance probe only. The production
    // manifest above remains disabled, and the hostile discovery flag proves
    // that environment input cannot restore dormant fact tools.
    mcp = new McpClient(isolatedConfig, backendUrl, options.timeoutMs);
    mcp.start();
    const init = await mcp.request("initialize", {
      protocolVersion: "2025-11-25",
      capabilities: {},
      clientInfo: { name: "analytix-funds-p0-health", version: "1" },
    });
    mcp.notify("notifications/initialized", {});
    const serverInfo = init?.serverInfo || {};
    report.pass("mcp initialize", `${text(serverInfo.name) || "analytix_funds"} ${text(serverInfo.version) || ""}`.trim());

    const toolList = await mcp.request("tools/list", {});
    const toolNames = exactStrings(
      (Array.isArray(toolList?.tools) ? toolList.tools : []).map((tool) => tool?.name),
    );
    if (toolNames.length === 0) {
      report.pass("mcp tools", "exactly zero provider-callable tools, including with full-discovery env set");
    } else {
      report.fail("mcp tools", `P0 quarantine advertised tools: ${toolNames.join(", ")}`);
    }

    const resourceList = await mcp.request("resources/list", {});
    const resourceUris = exactStrings(
      (Array.isArray(resourceList?.resources) ? resourceList.resources : []).map((item) => item?.uri),
    );
    if (JSON.stringify(resourceUris) !== JSON.stringify([FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI])) {
      report.fail("mcp resources", `unexpected resource inventory: ${resourceUris.join(", ") || "none"}`);
    } else {
      const resource = await mcp.request("resources/read", { uri: FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI });
      const body = String(resource?.contents?.[0]?.text || "");
      if (body.includes(FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY)) {
        report.pass("mcp resources", "exactly one fixed source-unavailable boundary");
      } else {
        report.fail("mcp resources", "fixed resource omitted the authoritative boundary text");
      }
    }

    const promptList = await mcp.request("prompts/list", {});
    const promptNames = exactStrings(
      (Array.isArray(promptList?.prompts) ? promptList.prompts : []).map((prompt) => prompt?.name),
    );
    if (JSON.stringify(promptNames) !== JSON.stringify([FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME])) {
      report.fail("mcp prompts", `unexpected prompt inventory: ${promptNames.join(", ") || "none"}`);
    } else {
      const prompt = await mcp.request("prompts/get", {
        name: FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
        arguments: {},
      });
      const body = String(prompt?.messages?.[0]?.content?.text || "");
      if (body.includes(FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY)) {
        report.pass("mcp prompts", "exactly one argument-free fixed boundary prompt");
      } else {
        report.fail("mcp prompts", "fixed prompt omitted the authoritative boundary text");
      }
    }

    try {
      await mcp.request("prompts/get", {
        name: FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
        arguments: { case_id: "must-not-be-consumed" },
      });
      report.fail("mcp prompt arguments", "P0 boundary prompt accepted case input");
    } catch (error) {
      if (hasExpectedInvalidParamsError(error, /does not accept arguments/iu)) {
        report.pass("mcp prompt arguments", "case input rejected before boundary rendering");
      } else {
        throw error;
      }
    }

    try {
      await mcp.callTool(P0_REJECTED_FACT_TOOL, {});
      report.fail("mcp fact-tool quarantine", `${P0_REJECTED_FACT_TOOL} unexpectedly executed`);
    } catch (error) {
      if (hasExpectedHiddenFullCaseError(error)) {
        report.pass("mcp fact-tool quarantine", "unknown or dormant fact tool rejected before backend access");
      } else {
        throw error;
      }
    }

    if (options.caseId || options.deep) {
      report.pass("case smoke quarantine", "case/deep fact smoke intentionally blocked while the source authority is unavailable");
    }
    if (options.reportDryRun || options.reportWriteSmoke) {
      try {
        await mcp.callTool("run_full_case_analysis", {
          case_id: options.caseId || "must-not-be-consumed",
          write_report: Boolean(options.reportWriteSmoke),
        });
        report.fail("report quarantine", "dormant report tool unexpectedly executed");
      } catch (error) {
        if (hasExpectedHiddenFullCaseError(error)) {
          report.pass("report quarantine", "report tool rejected before analysis, draft generation, or file access");
        } else {
          throw error;
        }
      }
    }
  } catch (error) {
    report.fail("mcp isolation smoke", error instanceof Error ? error.message : String(error));
  } finally {
    if (mcp) mcp.stop();
  }
}

function caseProjectRuntimeArgs(options) {
  const workspaceRealPath = text(options.caseProjectRoot);
  return workspaceRealPath
    ? {
        _analytix: {
          workspaceRealPath,
          source: "check-health"
        }
      }
    : {};
}

function structuredCaseId(result) {
  const structured = result?.structuredContent && typeof result.structuredContent === "object"
    ? result.structuredContent
    : {};
  const structuredId = text(structured.case_id || structured.caseId);
  if (structuredId) {
    return structuredId;
  }
  const metaLedger = objectOf(objectOf(result?._meta).analytix_evidence_ledger);
  const metaCaseId = text(metaLedger.case_id);
  if (metaCaseId) {
    return metaCaseId;
  }
  const contentText = Array.isArray(result?.content)
    ? result.content.map((item) => text(item?.text)).filter(Boolean).join("\n")
    : "";
  const match = contentText.match(/\bcase_id\s*[=:：]\s*([A-Za-z0-9_.-]+)/iu)
    || contentText.match(/案件ID\s*[：:\s]+([A-Za-z0-9_.-]+)/u)
    || contentText.match(/案件编号\s*[：:\s]*([A-Za-z0-9_.-]+)/u);
  return text(match?.[1]);
}

function summarizeResult(result) {
  const structured = result?.structuredContent && typeof result.structuredContent === "object"
    ? result.structuredContent
    : {};
  if (Object.keys(structured).length) {
    const caseId = text(structured.case_id || structured.caseId);
    const source = text(structured.source);
    return [caseId ? `案件编号=${caseId}` : "", source ? `source=${source}` : ""].filter(Boolean).join(" ");
  }
  const content = Array.isArray(result?.content) ? result.content : [];
  const head = content.map((item) => text(item?.text)).filter(Boolean).join(" ").slice(0, 160);
  return head || "ok";
}

function contentTextLength(result) {
  const content = Array.isArray(result?.content) ? result.content : [];
  return content.map((item) => text(item?.text)).join("\n").length;
}

function contentText(result) {
  const content = Array.isArray(result?.content) ? result.content : [];
  return content.map((item) => text(item?.text)).join("\n");
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function assertNoOpaqueRefs(report, name, result) {
  const structured = result?.structuredContent && typeof result.structuredContent === "object"
    ? result.structuredContent
    : {};
  const combined = `${contentText(result)}\n${JSON.stringify(structured)}`;
  const patterns = [
    /\bq_[0-9a-f]{8,}\b/iu,
    /"?(?:audit_ref|detail_ref|artifact_id|evidence_refs|query_ids|path_ids|report_ids)"?\s*:/iu,
    /(?:证据|审计引用|DuckDB 查询|路径引用|报告引用|快照引用)\s*[:：]\s*(?:q_|casegraph:|fund-flow:|mcp-full-|[\w.-]{8,})/iu,
  ];
  const hit = patterns.find((pattern) => pattern.test(combined));
  if (hit) {
    report.fail(name, `agent-visible output contains opaque reference pattern ${hit}`);
  } else {
    report.pass(name, "no opaque q/audit/artifact refs");
  }
}

function assertCompactText(report, name, result, limit = 6_000) {
  const length = contentTextLength(result);
  if (length > limit) {
    report.fail(name, `compact text too large: ${length} chars > ${limit}`);
  } else {
    report.pass(name, `compact text ${length} chars`);
  }
}

function assertCompactStructuredContent(report, name, result, limit = 6_000) {
  const structured = result?.structuredContent && typeof result.structuredContent === "object"
    ? result.structuredContent
    : {};
  const length = JSON.stringify(structured).length;
  const hasRawPayload = Boolean(structured.response || structured.payload);
  if (hasRawPayload) {
    report.fail(name, "structuredContent contains raw response/payload");
  } else if (length > limit) {
    report.fail(name, `structuredContent too large: ${length} chars > ${limit}`);
  } else {
    report.pass(name, `structuredContent ${length} chars`);
  }
}

function supportEnvelope(result) {
  return { body: contentText(result) };
}

function assertSupportOwnerEnvelope(report, name, result) {
  const envelope = supportEnvelope(result);
  const body = text(envelope.body);
  if (/^\s*[[{]/u.test(body)) {
    report.fail(name, "ordinary agent-readable output must be concise readable support text, not JSON");
  } else if (/support_only|final_answer_owned_by_focused_skill|case_id|delivery_state|answer_card_complete/u.test(body)) {
    report.fail(name, "ordinary agent-readable output exposes internal support fields");
  } else if (!/案件事实摘录|研判状态:/u.test(body)) {
    report.fail(name, "ordinary agent-readable output must retain a visible fact/source excerpt");
  } else {
    report.pass(name, "agent-readable support text keeps internals hidden");
  }
}

function assertEvidenceLedger(report, name, result, requiredSurfaces = []) {
  const ledger = objectOf(objectOf(result?._meta).analytix_evidence_ledger);
  const coverage = objectOf(ledger.coverage_summary);
  const surfaceCounts = objectOf(coverage.surface_counts);
  const entries = arrayOf(ledger.entries);
  const missingSurfaces = requiredSurfaces.filter((surface) => !Number(surfaceCounts[surface] || 0));
  const untraceableCount = Number(coverage.untraceable_required_surface_entry_count || 0);
  const missingEntryFields = entries.some((entry) => {
    const source = objectOf(entry);
    return !text(source.entry_id) || !text(source.fact_id) || !text(source.source_path);
  });
  const artifactId = text(ledger.artifact_id);
  const artifactStatus = text(ledger.artifact_status);
  if (!text(ledger.ledger_id) || !entries.length) {
    report.fail(name, "missing MCP _meta.analytix_evidence_ledger entries");
  } else if (missingSurfaces.length) {
    report.fail(name, `missing evidence surfaces: ${missingSurfaces.join(", ")}`);
  } else if (missingEntryFields) {
    report.fail(name, "ledger entries must carry entry_id, fact_id, and source_path");
  } else if (!artifactId || !artifactStatus) {
    report.fail(name, "ledger must include local artifact detail ref in MCP _meta");
  } else if (!Number(surfaceCounts.source_refs || 0)) {
    report.fail(name, "ledger entries must include source_refs surface");
  } else if (untraceableCount > 0) {
    report.fail(name, `ledger has ${untraceableCount} required-surface entries without source_refs`);
  } else {
    report.pass(name, `entries=${entries.length}; surfaces=${requiredSurfaces.join(",") || "present"}; artifact=${artifactStatus}`);
  }
}

function assertOwnerLedgerBoundary(report, name, result, required = false) {
  const ledgerJson = JSON.stringify(objectOf(objectOf(result?._meta).analytix_evidence_ledger));
  const hasSupportOwner = ledgerJson.includes("support_owner");
  const hasFocusedOwner = ledgerJson.includes("focused_owner");
  if (required && (!hasSupportOwner || !hasFocusedOwner)) {
    report.fail(name, "missing support_owner/focused_owner ledger boundary");
  } else if (hasSupportOwner || hasFocusedOwner) {
    report.pass(name, "support_owner/focused_owner ledger boundary present");
  } else {
    report.pass(name, "support_owner/focused_owner not required for this tool");
  }
}

function assertClaimVerifierRiskGate(report, name, result) {
  const output = contentText(result);
  const ledgerJson = JSON.stringify(objectOf(objectOf(result?._meta).analytix_evidence_ledger));
  const metaJson = JSON.stringify(objectOf(result?._meta));
  const answerCardJson = JSON.stringify(objectOf(result?.answer_card));
  const requiredSignals = [
    ["amount_without_fact_anchor", /amount_without_fact_anchor|42,?000,?000|金额 claim 未绑定事实|金额表述/u],
    ["unsupported_mermaid_or_arrow_flow", /unsupported_mermaid_or_arrow_flow|Mermaid|flowchart|箭头|没有确定性资金边/u],
    ["forbidden_legal_phrasing", /forbidden_legal_phrasing|已查明涉黑资金|确定违法所得|只能写线索\/需复核/u],
    ["candidate_account_as_owned_account", /candidate_account_as_owned_account|候选账户.*(?:名下|归属)|候选账户归属/u],
    ["cash_or_asset_destination_without_supported_flow", /cash_or_asset_destination_without_supported_flow|现金.*去向|理财.*supported transaction edge|资产端/u],
    ["missing_counterparty_or_cash_break_as_verified", /missing_counterparty_or_cash_break_as_verified|对手方.*(?:缺失|为空)|缺失对手方|收付款方缺失|现金断点|未匹配端点/u],
    ["claim_source_boundary", /claim_without_source_anchor|fact_refs\/source_refs|未声明 fact_refs|未声明事实来源|未绑定事实引用|报告级 claim 需绑定事实来源/u],
    ["report_not_ready", /write_blocked=true|阻断最终报告|不能出具正式报告|正式报告暂不出具|报告存在未绑定事实引用|复核或来源边界仍有缺口/u],
  ];
  const missing = requiredSignals
    .filter(([label, pattern]) => !pattern.test(output) && !pattern.test(answerCardJson) && !ledgerJson.includes(label) && !metaJson.includes(label) && !answerCardJson.includes(label))
    .map(([label]) => label);
  const missingLedgerSignals = [
    "claim_support_index",
    "claim_text_risk_scan"
  ].filter((marker) => !ledgerJson.includes(marker));
  if (missing.length) {
    report.fail(name, `missing claim verifier risk signals: ${missing.join(", ")}`);
  } else if (missingLedgerSignals.length) {
    report.fail(name, `missing internal claim support ledger signals: ${missingLedgerSignals.join(", ")}`);
  } else {
    report.pass(name, "amount, Mermaid/arrow flow, legal phrasing, report-not-ready, and internal claim support refs are gated");
  }
}

function assertClaimVerifierSupportedGate(report, name, result) {
  const output = contentText(result);
  const missing = [
    ["supported_claim", /supported_amount|金额来自 facts|fact_refs=known\.amount/u],
    ["readable_fact_excerpt", /案件事实摘录|研判状态:/u],
    ["no_json_support_fields", /support_only|final_answer_owned_by_focused_skill|case_id|delivery_state/u.test(output) ? "" : "ok"],
    ["object_text_normalized", /\[object Object\]/u.test(output) ? "" : "ok"],
  ].filter(([, marker]) => {
    if (marker === "ok") return false;
    if (!marker) return true;
    return !marker.test(output);
  }).map(([label]) => label);
  if (missing.length) {
    report.fail(name, `claim verifier supported-claim output is not normalized: ${missing.join(", ")}`);
  } else {
    report.pass(name, "supported claim fact refs remain normalized support for the focused owner");
  }
}

function isSemanticGapResult(result) {
  const output = contentText(result);
  const metaJson = JSON.stringify(objectOf(result?._meta));
  const answerCardJson = JSON.stringify(objectOf(result?.answer_card));
  if (/deterministic_tool_runtime/u.test(output + answerCardJson + metaJson)) return false;
  return /semantic_gap|capability_gap|SEMANTIC_TOOL_UNAVAILABLE|语义事实核验环节|语义事实工具|暂不可用/u.test(output + metaJson);
}

function assertSemanticGapWorkbenchBoundary(report, name, result) {
  const output = contentText(result);
  const metaJson = JSON.stringify(objectOf(result?._meta));
  const combined = output + metaJson;
  const missing = [
    ["semantic_gap", /semantic_gap|capability_gap|语义事实核验环节|语义事实工具|暂不可用/u],
    ["duckdb_workbench_available", /DuckDB|Workbench|run_case_sql|diagnose_case_sql|inspect_case_schema|count_case_rows/u],
    ["no_fabrication_boundary", /不得编造|不能编造|不支持任何案件金额|不支持.*链路|不支持.*报告结论/u],
  ].filter(([, pattern]) => !pattern.test(combined)).map(([label]) => label);
  if (missing.length) {
    report.fail(name, `semantic gap fallback missing boundary fields: ${missing.join(", ")}`);
  } else {
    report.pass(name, "semantic gap keeps DuckDB workbench recovery path and no-fabrication boundary");
  }
}

class HealthReport {
  constructor() {
    this.results = [];
  }

  add(name, status, detail = "") {
    this.results.push({ name, status, detail: text(detail) });
  }

  pass(name, detail = "") {
    this.add(name, "pass", detail);
  }

  warn(name, detail = "") {
    this.add(name, "warn", detail);
  }

  fail(name, detail = "") {
    this.add(name, "fail", detail);
  }

  get failed() {
    return this.results.some((result) => result.status === "fail");
  }

  print(json = false) {
    if (json) {
      console.log(JSON.stringify({
        ok: !this.failed,
        results: this.results,
      }, null, 2));
      return;
    }
    for (const result of this.results) {
      const marker = result.status === "pass" ? "PASS" : result.status === "warn" ? "WARN" : "FAIL";
      console.log(`[${marker}] ${result.name}${result.detail ? ` - ${result.detail}` : ""}`);
    }
  }
}

function requestJson(url, timeoutMs, method = "GET", payload = null) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const client = parsed.protocol === "https:" ? https : http;
    const body = payload == null ? "" : JSON.stringify(payload);
    const headers = body
      ? {
          "content-type": "application/json",
          "content-length": String(Buffer.byteLength(body)),
        }
      : {};
    const req = client.request(parsed, { method, headers }, (res) => {
      const chunks = [];
      res.on("data", (chunk) => chunks.push(Buffer.from(chunk)));
      res.on("end", () => {
        const body = Buffer.concat(chunks).toString("utf8");
        if (Number(res.statusCode || 0) < 200 || Number(res.statusCode || 0) >= 300) {
          reject(new Error(`HTTP ${res.statusCode}: ${body.slice(0, 200)}`));
          return;
        }
        try {
          resolve(JSON.parse(body));
        } catch (_) {
          resolve({ body });
        }
      });
    });
    req.setTimeout(timeoutMs, () => req.destroy(new Error("request timed out")));
    req.on("error", reject);
    if (body) req.write(body);
    req.end();
  });
}

function moneyNumber(value) {
  if (value && typeof value === "object") {
    return moneyNumber(value.yuan ?? value.amount_yuan ?? value.amount ?? value.value);
  }
  const normalized = String(value ?? "").replace(/,/gu, "").trim();
  const number = Number(normalized);
  return Number.isFinite(number) ? number : 0;
}

function approxEqual(actual, expected, tolerance = 0.5) {
  return Math.abs(Number(actual || 0) - Number(expected || 0)) <= tolerance;
}

function rankRowsFromSkillResponse(response) {
  return arrayOf(objectOf(objectOf(response).data).data?.rankings);
}

function isZhangJinzhiNameRow(row) {
  const item = objectOf(row);
  return [
    item.display_name,
    item.counterparty_name,
    item.holder_name,
    item.account_name,
  ].map(text).some((value) => value === "合成主体乙");
}

async function detectLiuZhangGoldenScenario(backendUrl, caseId, timeoutMs) {
  const commonInput = {
    case_id: caseId,
    holder_name: "合成主体甲",
    direction_mode: "out",
    metric: "outflow",
    success_filter: "all",
    cash_filter: "all",
    dedupe_same_holder_same_fact: true,
    limit: 20,
  };
  const requestTimeout = Math.max(timeoutMs, 60_000);
  try {
    const byName = await requestJson(
      `${backendUrl}/api/v1/skills/rank_counterparties:execute`,
      requestTimeout,
      "POST",
      { input: { ...commonInput, counterparty_group_mode: "name" } }
    );
    const nameRow = objectOf(rankRowsFromSkillResponse(byName).find(isZhangJinzhiNameRow));
    const nameAmount = moneyNumber(nameRow.outflow ?? nameRow.outflow_total);
    const nameTxnCount = Number(nameRow.txn_count || 0);
    const byAccount = await requestJson(
      `${backendUrl}/api/v1/skills/rank_counterparties:execute`,
      requestTimeout,
      "POST",
      { input: { ...commonInput, counterparty_group_mode: "account" } }
    );
    const accountRows = rankRowsFromSkillResponse(byAccount);
    const accountRow = objectOf(accountRows.find((row) =>
      text(objectOf(row).counterparty_account || objectOf(row).account_key || objectOf(row).display_name)
        .includes("9000000000000000015")
    ));
    const accountAmount = moneyNumber(accountRow.outflow ?? accountRow.outflow_total ?? accountRow.outflow_yuan);
    const accountTxnCount = Number(accountRow.txn_count || 0);
    const supported = approxEqual(nameAmount, 25_831_013)
      && nameTxnCount === 16
      && approxEqual(accountAmount, 21_000_000)
      && accountTxnCount === 5;
    return {
      supported,
      unavailable: false,
      detail: supported
        ? "case contains 合成主体甲->合成主体乙 fixed golden fact family"
        : `case lacks fixed golden fact family: name=${nameAmount}/${nameTxnCount}, account=${accountAmount}/${accountTxnCount}`,
    };
  } catch (error) {
    return {
      supported: false,
      unavailable: true,
      detail: `scenario preflight unavailable: ${error instanceof Error ? error.message : String(error)}`,
    };
  }
}

function extractServerVersion(source) {
  const match = source.match(/SERVER_VERSION\s*=\s*["']([^"']+)["']/u);
  return text(match?.[1]);
}

function collectTextFiles(root) {
  const files = [];
  const ignored = new Set([".git", "assets", "evidence", "node_modules"]);
  function walk(current) {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      if (ignored.has(entry.name)) {
        continue;
      }
      const entryPath = path.join(current, entry.name);
      if (entry.isDirectory()) {
        walk(entryPath);
        continue;
      }
      if (entry.isFile() && TEXT_AUDIT_EXTENSIONS.has(path.extname(entry.name))) {
        files.push(entryPath);
      }
    }
  }
  walk(root);
  return files;
}

function auditPackageText(root) {
  const findings = [];
  const developmentOnlyTextFiles = new Set([
    "references/top-pluginization-plan.md",
    "references/blueprint-execution-plan.md",
    "references/blueprint-implementation-matrix.md",
    "skills/analytix-fund-analysis/references/top-pluginization-plan.md",
    "skills/analytix-fund-analysis/references/blueprint-execution-plan.md",
    "skills/analytix-fund-analysis/references/blueprint-implementation-matrix.md",
  ]);
  for (const filePath of collectTextFiles(root)) {
    const relativePath = path.relative(root, filePath);
    if (developmentOnlyTextFiles.has(relativePath)) {
      continue;
    }
    const body = fs.readFileSync(filePath, "utf8");
    for (const pattern of PACKAGE_LEAK_PATTERNS) {
      if (body.includes(pattern)) {
        findings.push(`${relativePath} contains ${pattern}`);
      }
    }
    for (const pattern of SECRET_TEXT_PATTERNS) {
      if (pattern.test(body)) {
        findings.push(`${relativePath} contains secret-like assignment`);
      }
    }
  }
  return findings;
}

function mcpChildEnv(serverConfig, backendUrl) {
  const env = {};
  for (const key of PROCESS_ENV_ALLOWLIST) {
    if (process.env[key] !== undefined) {
      env[key] = process.env[key];
    }
  }
  const allowedMcpEnv = new Set(Array.isArray(serverConfig.env_vars) ? serverConfig.env_vars.map(text).filter(Boolean) : []);
  allowedMcpEnv.add("ANALYTIX_API_BASE_URL");
  for (const [key, value] of Object.entries(serverConfig.env && typeof serverConfig.env === "object" ? serverConfig.env : {})) {
    if (allowedMcpEnv.has(key)) {
      env[key] = String(value);
    }
  }
  for (const key of allowedMcpEnv) {
    if (process.env[key] !== undefined) {
      env[key] = process.env[key];
    }
  }
  env.ANALYTIX_API_BASE_URL = backendUrl;
  return env;
}

class McpClient {
  constructor(serverConfig, backendUrl, timeoutMs) {
    this.serverConfig = serverConfig;
    this.backendUrl = backendUrl;
    this.timeoutMs = timeoutMs;
    this.nextId = 1;
    this.pending = new Map();
    this.buffer = "";
    this.stderr = "";
    this.child = null;
  }

  start() {
    const cwd = path.resolve(pluginRoot, text(this.serverConfig.cwd) || ".");
    const command = text(this.serverConfig.command);
    const args = Array.isArray(this.serverConfig.args) ? this.serverConfig.args.map(String) : [];
    if (!command) {
      throw new Error("MCP command is empty");
    }
    this.child = spawn(command, args, {
      cwd,
      env: mcpChildEnv(this.serverConfig, this.backendUrl),
      stdio: ["pipe", "pipe", "pipe"],
    });
    this.child.stdout.on("data", (chunk) => this.onStdout(chunk));
    this.child.stderr.on("data", (chunk) => {
      this.stderr += chunk.toString();
    });
    this.child.once("exit", (code, signal) => {
      const error = new Error(`MCP server exited code=${code ?? ""} signal=${signal ?? ""} ${this.stderr.slice(-500)}`);
      for (const pending of this.pending.values()) {
        clearTimeout(pending.timer);
        pending.reject(error);
      }
      this.pending.clear();
    });
  }

  onStdout(chunk) {
    this.buffer += chunk.toString();
    let newlineIndex = this.buffer.indexOf("\n");
    while (newlineIndex >= 0) {
      const line = this.buffer.slice(0, newlineIndex).trim();
      this.buffer = this.buffer.slice(newlineIndex + 1);
      if (line) {
        this.onMessage(line);
      }
      newlineIndex = this.buffer.indexOf("\n");
    }
  }

  onMessage(line) {
    let message;
    try {
      message = JSON.parse(line);
    } catch (_) {
      return;
    }
    if (!Object.prototype.hasOwnProperty.call(message, "id") || !this.pending.has(message.id)) {
      return;
    }
    const pending = this.pending.get(message.id);
    this.pending.delete(message.id);
    clearTimeout(pending.timer);
    if (message.error) {
      pending.reject(new Error(JSON.stringify(message.error)));
      return;
    }
    pending.resolve(message.result);
  }

  request(method, params = {}, timeoutMs = this.timeoutMs) {
    if (!this.child || !this.child.stdin.writable) {
      return Promise.reject(new Error("MCP server is not running"));
    }
    const id = this.nextId;
    this.nextId += 1;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        this.notify("notifications/cancelled", { requestId: id, reason: "client_timeout" });
        reject(new Error(`${method} timed out after ${timeoutMs}ms`));
      }, timeoutMs);
      this.pending.set(id, { resolve, reject, timer });
      this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`);
    });
  }

  notify(method, params = {}) {
    if (this.child && this.child.stdin.writable) {
      this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", method, params })}\n`);
    }
  }

  callTool(name, args = {}, timeoutMs = this.timeoutMs) {
    return this.request("tools/call", { name, arguments: args }, timeoutMs);
  }

  stop() {
    if (!this.child) {
      return;
    }
    try {
      this.child.stdin.end();
    } catch (_) {
      // ignore shutdown errors
    }
    setTimeout(() => {
      try {
        this.child.kill();
      } catch (_) {
        // ignore shutdown errors
      }
    }, 200);
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const report = new HealthReport();
  const pluginJsonPath = path.join(pluginRoot, ".codex-plugin", "plugin.json");
  const mcpJsonPath = path.join(pluginRoot, ".mcp.json");
  const serverPath = path.join(pluginRoot, "mcp", "server.mjs");
  const skillPath = path.join(pluginRoot, "skills", "analytix-fund-analysis", "SKILL.md");
  const commandRouterPath = path.join(pluginRoot, "references", "command-router.md");
  const commandMetadataPath = path.join(pluginRoot, "references", "command-metadata.json");
  const antiPatternsPath = path.join(pluginRoot, "references", "anti-patterns.md");

  let pluginManifest = null;
  let mcpManifest = null;
  let serverSource = "";
  try {
    for (const [label, filePath] of [
      ["plugin.json", pluginJsonPath],
      [".mcp.json", mcpJsonPath],
      ["server.mjs", serverPath],
      ["SKILL.md", skillPath],
      ["command-router.md", commandRouterPath],
      ["command-metadata.json", commandMetadataPath],
      ["anti-patterns.md", antiPatternsPath],
    ]) {
      if (!fs.existsSync(filePath)) {
        throw new Error(`${label} missing at ${filePath}`);
      }
    }
    pluginManifest = readJson(pluginJsonPath);
    mcpManifest = readJson(mcpJsonPath);
    serverSource = fs.readFileSync(serverPath, "utf8");
    report.pass("plugin files", `version=${text(pluginManifest.version)}`);
  } catch (error) {
    report.fail("plugin files", error instanceof Error ? error.message : String(error));
  }

  const serverVersion = extractServerVersion(serverSource);
  if (serverVersion && text(pluginManifest?.version) === serverVersion) {
    report.pass("version alignment", `plugin.json=${text(pluginManifest.version)} server=${serverVersion}`);
  } else {
    report.fail("version alignment", `plugin.json=${text(pluginManifest?.version) || "missing"} server=${serverVersion || "missing"}`);
  }

  const packageLeaks = auditPackageText(pluginRoot);
  if (packageLeaks.length) {
    report.fail("package text audit", packageLeaks.slice(0, 5).join("; "));
  } else {
    report.pass("package text audit", "no local repo path or deployment secret markers");
  }

  const serverConfig = mcpManifest?.mcpServers?.analytix_funds || {};
  if (text(serverConfig.cwd) === ".") {
    report.pass("mcp cwd", "cwd=.");
  } else {
    report.fail("mcp cwd", "analytix_funds must set cwd to . so relative args resolve from the plugin root");
  }

  const check = spawnSync(process.execPath, ["--check", serverPath], {
    cwd: pluginRoot,
    encoding: "utf8",
  });
  if (check.status === 0) {
    report.pass("server syntax", "node --check passed");
  } else {
    report.fail("server syntax", (check.stderr || check.stdout || "").slice(0, 500));
  }

  await runP0IsolationHealth(report, serverConfig, options);
  report.print(options.json);
  process.exit(report.failed ? 1 : 0);

  const backendUrl = normalizeBaseUrl(options.backendUrl || process.env.ANALYTIX_API_BASE_URL || serverConfig.env?.ANALYTIX_API_BASE_URL);
  let backendReachable = false;
  try {
    const health = await requestJson(`${backendUrl}/health`, Math.min(options.timeoutMs, 5_000));
    backendReachable = true;
    report.pass("backend health", `service=${text(health.service || health.name || "analytix-backend")}`);
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    if (options.deep || options.activateCase) {
      report.fail("backend health", detail);
    } else {
      report.warn("backend health", detail);
    }
  }

  let mcp = null;
  let activeCaseLock = null;
  try {
    if (options.caseId) {
      activeCaseLock = await acquireActiveCaseLock(options);
      report.pass("serialized active-case health check", `lock=${activeCaseLock.lockPath}`);
    }
    mcp = new McpClient(serverConfig, backendUrl, options.timeoutMs);
    mcp.start();
    const init = await mcp.request("initialize", {
      protocolVersion: "2025-11-25",
      capabilities: {},
      clientInfo: { name: "analytix-fund-analysis-health", version: "0.0.1" },
    });
    mcp.notify("notifications/initialized", {});
    const serverInfo = init?.serverInfo || {};
    report.pass("mcp initialize", `${text(serverInfo.name) || "analytix_funds"} ${text(serverInfo.version) || ""}`.trim());

    const list = await mcp.request("tools/list", {});
    const toolNames = new Set((Array.isArray(list?.tools) ? list.tools : []).map((tool) => text(tool.name)).filter(Boolean));
    // This legacy deep-health branch is unreachable after the P0 isolation
    // runner exits above; retain it only as historical diagnostic scaffolding.
    // eslint-disable-next-line no-undef
    const missing = EXPECTED_DEFAULT_TOOLS.filter((name) => !toolNames.has(name));
    // eslint-disable-next-line no-undef
    const reportHeavyVisible = REPORT_HEAVY_TOOLS.filter((name) => toolNames.has(name));
    if (missing.length) {
      report.fail("mcp tools", `missing ${missing.join(", ")}`);
    } else {
      const reportNote = reportHeavyVisible.length ? `; report-heavy visible=${reportHeavyVisible.join(",")}` : "; report-heavy hidden by default";
      report.pass("mcp tools", `${toolNames.size} tools${reportNote}`);
    }

    try {
      const resourceList = await mcp.request("resources/list", {});
      const resources = Array.isArray(resourceList?.resources) ? resourceList.resources : [];
      const resourceUris = new Set(resources.map((item) => text(item?.uri)).filter(Boolean));
      const requiredUris = [
        "skill://analytix-fund-analysis/SKILL.md",
        "skill://analytix-fund-analysis/references/command-metadata.json",
        "skill://analytix-fund-analysis/references/anti-patterns.md",
      ];
      const missingResources = requiredUris.filter((uri) => !resourceUris.has(uri));
      const evalResources = ["skill://analytix-fund-analysis/references/golden-eval-rubric.md"]
        .filter((uri) => resourceUris.has(uri));
      if (missingResources.length) {
        report.fail("mcp resources", `missing ${missingResources.join(", ")}`);
      } else if (evalResources.length) {
        report.fail("mcp resources", `eval-only resources exposed: ${evalResources.join(", ")}`);
      } else {
        const skillResource = await mcp.request("resources/read", { uri: "skill://analytix-fund-analysis/SKILL.md" });
        const body = String(skillResource?.contents?.[0]?.text || "");
        if (body.includes("Agent Freedom Rule") && body.includes("semantic fact toolbox") && body.includes("funds_investigate") && body.includes("navigator")) {
          report.pass("mcp resources", `${resources.length} resources; root skill readable`);
        } else {
          report.fail("mcp resources", "root skill resource is missing expected semantic-toolbox/navigator guidance");
        }
      }
    } catch (error) {
      report.fail("mcp resources", error instanceof Error ? error.message : String(error));
    }

    try {
      const promptList = await mcp.request("prompts/list", {});
      const promptNames = new Set((Array.isArray(promptList?.prompts) ? promptList.prompts : [])
        .map((prompt) => text(prompt?.name))
        .filter(Boolean));
      // eslint-disable-next-line no-undef
      const missingPrompts = EXPECTED_PROMPTS.filter((name) => !promptNames.has(name));
      if (missingPrompts.length) {
        report.fail("mcp prompts", `missing ${missingPrompts.join(", ")}`);
      } else {
        const prompt = await mcp.request("prompts/get", {
          name: "explore-case-database",
          arguments: { question: "核验当前案件资金数据", case_id: "health_check_case" }
        });
        const body = String(prompt?.messages?.[0]?.content?.text || "");
        if (body.includes("schema") && body.includes("analysis_*") && body.includes("公安经侦")) {
          report.pass("mcp prompts", `${promptNames.size} prompts; workflow prompt readable`);
        } else {
          report.fail("mcp prompts", "explore-case-database prompt is missing schema/analysis/public-security guidance");
        }
      }
    } catch (error) {
      report.fail("mcp prompts", error instanceof Error ? error.message : String(error));
    }

    const runtimeArgs = caseProjectRuntimeArgs(options);
    try {
      const currentCase = await mcp.callTool("get_current_case", runtimeArgs);
      let currentCaseId = structuredCaseId(currentCase);
      let activeCaseDetail = "";
      if (options.caseId && currentCaseId && currentCaseId !== options.caseId) {
        report.fail("case project", `expected case_id=${options.caseId} got ${currentCaseId || "missing"}`);
      } else {
        report.pass("case project", `${summarizeResult(currentCase)}${activeCaseDetail}`);
      }
    } catch (error) {
      if (hasExpectedCaseMissingError(error)) {
        if (options.caseId) {
          report.pass("case project", "explicit --case-id smoke checks current case project");
        } else {
          report.warn("case project", "no current case project; pass --case-project-root or run from a case project workspace");
        }
      } else {
        throw error;
      }
    }

    if (options.caseId) {
      const caseArgs = { ...runtimeArgs, case_id: options.caseId };
      const status = await mcp.callTool("get_case_status", caseArgs);
      report.pass("case status", summarizeResult(status));
      const pipeline = await mcp.callTool("get_case_data_pipeline_overview", {
        ...caseArgs,
        include_tree: false,
      }, Math.max(options.timeoutMs, 30_000));
      report.pass("case pipeline", summarizeResult(pipeline));
      const scopeMap = await mcp.callTool("get_case_scope_map", {
        ...caseArgs,
        include_tree_items: false,
        include_schema_columns: false,
      }, Math.max(options.timeoutMs, 90_000));
      report.pass("case scope map", summarizeResult(scopeMap));
      const casegraph = await mcp.callTool("get_casegraph", {
        ...caseArgs,
        include_rankings: true,
      }, Math.max(options.timeoutMs, 120_000));
      assertCompactText(report, "casegraph compact output", casegraph);
      assertCompactStructuredContent(report, "casegraph compact structured output", casegraph);
      assertNoOpaqueRefs(report, "casegraph opaque refs", casegraph);
      assertEvidenceLedger(report, "casegraph evidence ledger", casegraph, [
        "amount",
        "count",
        "source_refs",
      ]);
      const fixedScenario = backendReachable
        ? await detectLiuZhangGoldenScenario(backendUrl, options.caseId, options.timeoutMs)
        : { supported: false, unavailable: true, detail: "backend unavailable" };
      if (fixedScenario.unavailable) {
        report.fail("fixed golden scenario preflight", fixedScenario.detail);
      } else {
        report.pass("fixed golden scenario preflight", fixedScenario.detail);
      }
      if (fixedScenario.supported) {
        const frontDoor = await mcp.callTool("funds_investigate", {
          ...caseArgs,
          intent: "holder_analysis",
          question: "对合成主体甲名下账户进行分析",
          holder_name: "合成主体甲",
          max_cards: 4,
        }, Math.max(options.timeoutMs, 180_000));
        assertCompactText(report, "frontdoor compact output", frontDoor);
        assertCompactStructuredContent(report, "frontdoor compact structured output", frontDoor);
        assertNoOpaqueRefs(report, "frontdoor opaque refs", frontDoor);
        assertSupportOwnerEnvelope(report, "frontdoor focused-owner contract", frontDoor);
      assertEvidenceLedger(report, "frontdoor evidence ledger", frontDoor, [
        "count",
        "account",
        "holder",
        "context_compiler_directive",
        "source_refs",
      ]);
      } else {
        report.pass("frontdoor golden scenario", "skipped because selected case lacks 合成主体甲->合成主体乙 fixed fact family");
      }
      const rankingFrontDoor = await mcp.callTool("funds_investigate", {
        ...caseArgs,
        intent: "ranking",
        question: "资金体量最大的户名是谁，他出账给谁的钱最多，梳理出前十位。",
        holder_name: "河南颍淮建工有限公司",
        metric: "outflow",
        direction_mode: "out",
        counterparty_group_mode: "name",
        top_n: 10,
      }, Math.max(options.timeoutMs, 180_000));
      assertCompactText(report, "ranking frontdoor compact output", rankingFrontDoor);
      assertCompactStructuredContent(report, "ranking frontdoor compact structured output", rankingFrontDoor);
      assertNoOpaqueRefs(report, "ranking frontdoor opaque refs", rankingFrontDoor);
      assertSupportOwnerEnvelope(report, "ranking frontdoor focused-owner contract", rankingFrontDoor);
      assertEvidenceLedger(report, "ranking frontdoor evidence ledger", rankingFrontDoor, [
        "amount",
        "count",
        "account",
        "holder",
        "counterparty",
        "context_compiler_directive",
        "source_refs",
      ]);
      if (fixedScenario.supported) {
        const destinationFrontDoor = await mcp.callTool("funds_investigate", {
          ...caseArgs,
          intent: "destination",
          question: "合成主体甲自 2025-01-01 起转给合成主体乙多少钱？完整数据范围内同名收款人合计是多少？转给合成主体乙后，合成主体乙又把钱给了谁？",
          holder_name: "合成主体甲",
          via_holder_name: "合成主体乙",
          date_start: "2025-01-01",
          top_n: 20,
        }, Math.max(options.timeoutMs, 180_000));
        assertCompactText(report, "destination frontdoor compact output", destinationFrontDoor);
        assertCompactStructuredContent(report, "destination frontdoor compact structured output", destinationFrontDoor);
        assertNoOpaqueRefs(report, "destination frontdoor opaque refs", destinationFrontDoor);
        assertSupportOwnerEnvelope(report, "destination frontdoor focused-owner contract", destinationFrontDoor);
	        assertEvidenceLedger(report, "destination frontdoor evidence ledger", destinationFrontDoor, [
	          "amount",
	          "count",
	          "account",
	          "holder",
	          "counterparty",
	          "context_compiler_directive",
	          "source_refs",
	        ]);
	        assertOwnerLedgerBoundary(report, "destination frontdoor support_owner/focused_owner ledger", destinationFrontDoor, true);
        const destinationText = contentText(destinationFrontDoor);
        const mentionsDuplicate42m = /42,?000,?000(?:\.00)?/u.test(destinationText) || /4200(?:\.0+)?\s*万/u.test(destinationText);
        const duplicate42mIsGuarded = /重复放大|错误口径|不能写成(?:已查明)?事实|不得写成事实/u.test(destinationText);
        if (mentionsDuplicate42m && !duplicate42mIsGuarded) {
          report.fail("destination frontdoor no duplicate-amplified 42M", "合成主体甲->合成主体乙 must not be reported as duplicate-amplified 42M");
        } else {
          report.pass("destination frontdoor no duplicate-amplified 42M", mentionsDuplicate42m ? "42M appears only as guarded duplicate-amplification warning" : "42M duplicate-amplified figure absent");
        }
        if (/25,?885,?013(?:\.00)?/u.test(destinationText) || /2588\.5013/u.test(destinationText)) {
          report.pass("destination frontdoor Zhang Jinzhi full-period name total", "合成主体乙 name-scope total present");
        } else {
          report.fail("destination frontdoor Zhang Jinzhi full-period name total", "missing 25,885,013.00 effective pair amount");
        }
        if (/21,?000,?000(?:\.00)?/u.test(destinationText) || /2100(?:\.0+)?\s*万/u.test(destinationText)) {
          report.pass("destination frontdoor Zhang Jinzhi core transaction concentration", "2025 core 21M cluster present");
        } else {
          report.fail("destination frontdoor Zhang Jinzhi core transaction concentration", "missing 21,000,000.00 core txn-id-deduped cluster");
        }
        if (/时间窗口核验|时间窗口一跳核验|核心窗口口径|2025-01-01\s*以来|source_to_via_date_window_summary/u.test(destinationText)) {
          report.pass("destination frontdoor date-window summary", "date-window one-hop summary present");
        } else {
          report.fail("destination frontdoor date-window summary", `missing date-window one-hop summary; preview=${destinationText.slice(0, 1200)}`);
        }
        const flowGraph = await mcp.callTool("build_fund_flow_graph", {
          ...caseArgs,
          holder_name: "合成主体甲",
          via_holder_name: "合成主体乙",
          date_start: "2025-01-01",
          top_n: 10,
        }, Math.max(options.timeoutMs, 120_000));
        assertCompactText(report, "flowgraph compact output", flowGraph);
        assertCompactStructuredContent(report, "flowgraph compact structured output", flowGraph);
        assertNoOpaqueRefs(report, "flowgraph opaque refs", flowGraph);
        assertSupportOwnerEnvelope(report, "flowgraph focused-owner contract", flowGraph);
        assertEvidenceLedger(report, "flowgraph evidence ledger", flowGraph, [
          "amount",
          "count",
          "holder",
          "counterparty",
          "flow_or_transaction_edge",
          "source_refs",
        ]);
        const flowGraphDebug = await mcp.callTool("build_fund_flow_graph", {
          ...caseArgs,
          holder_name: "合成主体甲",
          via_holder_name: "合成主体乙",
          date_start: "2025-01-01",
          top_n: 10,
          include_debug: true,
        }, Math.max(options.timeoutMs, 120_000));
        assertCompactText(report, "flowgraph debug-disabled compact output", flowGraphDebug);
        assertCompactStructuredContent(report, "flowgraph debug-disabled compact structured output", flowGraphDebug);
        assertNoOpaqueRefs(report, "flowgraph debug-disabled opaque refs", flowGraphDebug);
        assertSupportOwnerEnvelope(report, "flowgraph debug-disabled focused-owner contract", flowGraphDebug);
        assertEvidenceLedger(report, "flowgraph debug-disabled evidence ledger", flowGraphDebug, [
          "amount",
          "count",
          "holder",
          "counterparty",
          "flow_or_transaction_edge",
          "source_refs",
        ]);
      } else {
        report.pass("destination and flowgraph golden scenario", "skipped because selected case lacks 合成主体甲->合成主体乙 fixed fact family");
      }
      const reconciliation = await mcp.callTool("get_case_reconciliation", caseArgs, Math.max(options.timeoutMs, 30_000));
      report.pass("case reconciliation", summarizeResult(reconciliation));
      const dataQuality = await mcp.callTool("audit_case_data_quality", {
        ...caseArgs,
        example_limit: Math.max(1, Math.min(options.maxAccounts, 12)),
      }, Math.max(options.timeoutMs, 60_000));
      report.pass("data quality audit", summarizeResult(dataQuality));
      if (fixedScenario.supported) {
        const duplicateFamilies = await mcp.callTool("resolve_duplicate_families", {
          ...caseArgs,
          holder_name: "合成主体甲",
          scope_mode: "same_holder_accounts",
          limit: Math.max(1, Math.min(options.maxAccounts, 12)),
        }, Math.max(options.timeoutMs, 90_000));
        report.pass("duplicate family resolver", summarizeResult(duplicateFamilies));
      } else {
        report.pass("duplicate family golden scenario", "skipped because selected case lacks 合成主体甲->合成主体乙 fixed fact family");
      }
      const coverage = await mcp.callTool("get_scope_coverage", caseArgs, Math.max(options.timeoutMs, 30_000));
      report.pass("scope coverage", summarizeResult(coverage));
      const scopeCompare = await mcp.callTool("compare_analysis_scopes", caseArgs, Math.max(options.timeoutMs, 60_000));
      report.pass("scope compare", summarizeResult(scopeCompare));
      const accountRank = await mcp.callTool("rank_accounts", {
        ...caseArgs,
        metric: "turnover",
        limit: Math.max(1, Math.min(options.maxAccounts, 12)),
      }, Math.max(options.timeoutMs, 60_000));
      report.pass("rank accounts", summarizeResult(accountRank));
      const holderRank = await mcp.callTool("rank_holders", {
        ...caseArgs,
        metric: "turnover",
        limit: Math.max(1, Math.min(options.maxAccounts, 12)),
      }, Math.max(options.timeoutMs, 60_000));
      report.pass("rank holders", summarizeResult(holderRank));
      const counterpartyRank = await mcp.callTool("rank_counterparties", {
        ...caseArgs,
        metric: "turnover",
        limit: Math.max(1, Math.min(options.maxAccounts, 12)),
      }, Math.max(options.timeoutMs, 60_000));
      report.pass("rank counterparties", summarizeResult(counterpartyRank));
      if (fixedScenario.supported) {
        const ownerScope = await mcp.callTool("resolve_owner_scope", {
          ...caseArgs,
          holder_name: "合成主体甲",
          include_candidate_accounts: true,
          limit: Math.max(1, Math.min(options.maxAccounts, 12)),
        }, Math.max(options.timeoutMs, 60_000));
        report.pass("owner scope", summarizeResult(ownerScope));
        const discoveryProbe = await mcp.callTool("run_discovery_scan", {
          ...caseArgs,
          holder_name: "合成主体甲",
          include_candidate_accounts: true,
          limit: Math.max(1, Math.min(options.maxAccounts, 12)),
        }, Math.max(options.timeoutMs, 60_000));
        report.pass("discovery probe", summarizeResult(discoveryProbe));
        const destinationProbe = await mcp.callTool("trace_holder_destinations", {
          ...caseArgs,
          holder_name: "合成主体甲",
          include_candidate_accounts: true,
          limit: Math.max(1, Math.min(options.maxAccounts, 12)),
        }, Math.max(options.timeoutMs, 60_000));
        report.pass("destination probe", summarizeResult(destinationProbe));
        const topOutflows = await mcp.callTool("trace_subject_top_outflows", {
          ...caseArgs,
          holder_name: "合成主体甲",
          include_candidate_accounts: true,
          date_start: "2025-01-01",
          top_n: Math.max(1, Math.min(options.maxAccounts, 12)),
        }, Math.max(options.timeoutMs, 90_000));
        report.pass("top outflow trace", summarizeResult(topOutflows));
        const missingBusiness = await mcp.callTool("classify_missing_counterparty_business", {
          ...caseArgs,
          holder_name: "合成主体甲",
          include_candidate_accounts: true,
          missing_kind: "both",
          limit: Math.max(1, Math.min(options.maxAccounts, 12)),
        }, Math.max(options.timeoutMs, 60_000));
        report.pass("missing business classification", summarizeResult(missingBusiness));
      } else {
        report.pass("holder investigation golden scenario", "skipped because selected case lacks 合成主体甲->合成主体乙 fixed fact family");
      }
      const continuationQa = await mcp.callTool("validate_continuation_list", {
        ...caseArgs,
        rows: [],
        strict_db_match: true,
      }, Math.max(options.timeoutMs, 60_000));
      report.pass("continuation list qa", summarizeResult(continuationQa));
      const claimReview = await mcp.callTool("validate_report_claims", {
        ...caseArgs,
        claims: [
          {
            id: "duplicate_amount_guard",
            text: "合成主体甲转给合成主体乙 42,000,000 元。"
          },
          {
            id: "unsupported_mermaid_guard",
            text: "```mermaid\nflowchart LR\n合成主体甲 --> 合成主体乙 --> 理财 --> 合成主体丁\n```"
          },
          {
            id: "forbidden_legal_guard",
            text: "已查明涉黑资金，确定违法所得最终资金归属。"
          },
          {
            id: "candidate_owner_guard",
            text: "候选账户已查明确认为合成主体甲名下账户。"
          },
          {
            id: "cash_asset_destination_guard",
            text: "合成主体乙资金最终流向理财和现金去向已确认。"
          },
          {
            id: "missing_counterparty_break_guard",
            text: "对手方为空但资金闭环已确认。"
          }
        ],
        strict_report_text: true
      }, Math.max(options.timeoutMs, 60_000));
      report.pass("claim verifier smoke", summarizeResult(claimReview));
      assertCompactStructuredContent(report, "claim verifier compact structured output", claimReview);
      assertNoOpaqueRefs(report, "claim verifier opaque refs", claimReview);
      if (isSemanticGapResult(claimReview)) {
        assertSemanticGapWorkbenchBoundary(report, "claim verifier semantic gap boundary", claimReview);
      } else {
        assertClaimVerifierRiskGate(report, "claim verifier risk gate", claimReview);
        assertEvidenceLedger(report, "claim verifier evidence ledger", claimReview, [
          "report_claim",
          "context_compiler_directive",
          "source_refs",
        ]);
      }
      const supportedClaimReview = await mcp.callTool("validate_report_claims", {
        ...caseArgs,
        claims: [
          {
            claim_id: "supported_amount",
            text: "金额来自 facts 100 元。",
            fact_refs: ["known.amount"]
          }
        ],
        facts: {
          known: {
            amount: 100
          }
        },
        report_text: "金额 100 元。",
        strict_report_text: true
      }, Math.max(options.timeoutMs, 60_000));
      if (isSemanticGapResult(supportedClaimReview)) {
        assertSemanticGapWorkbenchBoundary(report, "claim verifier supported semantic gap boundary", supportedClaimReview);
      } else {
        assertClaimVerifierSupportedGate(report, "claim verifier supported gate", supportedClaimReview);
      }
      const plan = await mcp.callTool("plan_case_analysis", {
        ...caseArgs,
        analysis_goal: "smoke top20 outflow continuation",
        include_risk_scan: false,
        max_lanes: 4,
      }, Math.max(options.timeoutMs, 90_000));
      report.pass("analysis plan", summarizeResult(plan));
      if (fixedScenario.supported) {
        const investigationLab = await mcp.callTool("run_investigation_lab", {
          ...caseArgs,
          analysis_goal: "smoke old-thread-style hypothesis lab",
          holder_name: "合成主体甲",
          include_candidate_accounts: true,
          focus_keywords: ["合成主体乙", "理财"],
          date_start: "2025-01-01",
          max_hypotheses: Math.max(3, Math.min(options.maxAccounts + 2, 8)),
        }, Math.max(options.timeoutMs, 120_000));
        report.pass("investigation lab", summarizeResult(investigationLab));
      } else {
        report.pass("investigation lab golden scenario", "skipped because selected case lacks 合成主体甲->合成主体乙 fixed fact family");
      }
      const tree = await mcp.callTool("get_stats_tree", {
        ...caseArgs,
        limit_groups: 1,
        limit_items: 1,
        include_items: true,
      }, Math.max(options.timeoutMs, 30_000));
      report.pass("stats tree", summarizeResult(tree));
    }

    if (options.caseId && options.deep) {
      const risks = await mcp.callTool("scan_case_risks", {
        ...runtimeArgs,
        case_id: options.caseId,
        include_graph: false,
      }, Math.max(options.timeoutMs, 60_000));
      report.pass("risk scan", summarizeResult(risks));
    }

    if (options.reportDryRun) {
      try {
        const dryRun = await mcp.callTool("run_full_case_analysis", {
          ...runtimeArgs,
          case_id: options.caseId || "p0-health-check",
          max_accounts: options.maxAccounts,
          include_internal_playbooks: false,
          write_report: false,
        }, Math.max(options.timeoutMs, 30_000));
        report.fail("report dry-run", `P0-hidden tool unexpectedly executed: ${summarizeResult(dryRun)}`);
      } catch (error) {
        if (hasExpectedHiddenFullCaseError(error)) {
          report.pass("report dry-run", "write_report=false rejected as unknown/unadvertised before execution");
        } else {
          throw error;
        }
      }
    }

    if (options.reportWriteSmoke) {
      try {
        const writeSmoke = await mcp.callTool("run_full_case_analysis", {
          ...runtimeArgs,
          case_id: options.caseId || "p0-health-check",
          max_accounts: options.maxAccounts,
          include_internal_playbooks: false,
          write_report: true,
        }, Math.max(options.timeoutMs, 30_000));
        report.fail("report publication-gate smoke", `P0-hidden tool unexpectedly executed: ${summarizeResult(writeSmoke)}`);
      } catch (error) {
        if (hasExpectedHiddenFullCaseError(error)) {
          report.pass("report publication-gate smoke", "write_report=true rejected as unknown/unadvertised before execution");
        } else {
          throw error;
        }
      }
    }
  } catch (error) {
    report.fail("mcp smoke", error instanceof Error ? error.message : String(error));
  } finally {
    if (mcp) {
      mcp.stop();
    }
    releaseActiveCaseLock(activeCaseLock);
  }

  report.print(options.json);
  process.exit(report.failed ? 1 : 0);
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack : String(error));
  process.exit(1);
});
