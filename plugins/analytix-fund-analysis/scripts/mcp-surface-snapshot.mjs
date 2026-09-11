#!/usr/bin/env node

import { spawn } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE,
  FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
  FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI,
  fixedFundsBoundaryPrompt,
  fixedFundsBoundaryPrompts,
  fixedFundsBoundaryResourceRead,
  fixedFundsBoundaryResources,
} from "../mcp/source-unavailable-boundary.mjs";
import {
  PRODUCTION_MCP_ENTRY_CLOSURE_FILES,
} from "./production-mcp-entry-closure-contract.mjs";

const SCRIPT_PATH = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(SCRIPT_PATH);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");
const SNAPSHOT_SCHEMA_VERSION = "FundsMcpSurfaceSnapshotV2";
const PREFERRED_PROTOCOL_VERSION = "2025-11-25";
const EXPECTED_SERVER_NAME = "analytix_funds";
const FUNDS_COUNT_PROJECTION_META_KEY_V2 = "analytixFundsCountProjectionV2";
const FUNDS_COUNT_PROJECTION_DIGEST_DOMAIN_V2 = Buffer.from(
  "analytix.host.funds-count-projection/digest/v2\0",
  "utf8",
);
const EXPECTED_BOUNDARY_SHA256 =
  "ec0b9315229cce159cd97fb9da3c74f2d33b8385c674e1d9950d9a6ac185eceb";
const RAW_RESPONSE_SHA256 = Object.freeze({
  tools_list: "e1031fd7243813a51c78d36edfa6fecf17eddf35da21ff0a26d2a2babe5d2049",
  resources_list: "c9dba6e61810342f00df65808fed5e13964066b68b7bc841cd2b6b45aaf04232",
  resource_templates_list: "87854a2460a872ff5a0a898621e75c3133f9673bb5a98357a2a80681437e693d",
  resource_read: "c9767bf97eed313a211c4ba5405f0b27fbbf5c13e939db7d28e79ad505b82e88",
  prompts_list: "9a66e267a47e50339a460c1afb644e9592dba1db6e038ee6301db09240fbbad0",
  prompt_get: "bea7dd7d664c1945e9da25361c998c767a00784568b1a087b06d3506faf7b226",
});
const REQUIRED_RELEASE_BLOCKERS = Object.freeze([
  "production_mcp_disabled",
  "production_mcp_p0_quarantine",
  FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE,
]);
const SOURCE_BINDING_FILES = Object.freeze([
  ".codex-plugin/plugin.json",
  ".mcp.json",
  "scripts/production-mcp-entry-closure.json",
  ...PRODUCTION_MCP_ENTRY_CLOSURE_FILES,
]);

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function sha256(value) {
  if (value === undefined) return "";
  return crypto.createHash("sha256").update(value).digest("hex");
}

function goJsonString(value) {
  const encoded = JSON.stringify(value);
  if (encoded === undefined) return undefined;
  return encoded.replace(/[<>&\u2028\u2029]/gu, (character) => ({
    "<": "\\u003c",
    ">": "\\u003e",
    "&": "\\u0026",
    "\u2028": "\\u2028",
    "\u2029": "\\u2029",
  })[character]);
}

function canonicalJson(value) {
  if (value === null) return "null";
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(",")}]`;
  if (typeof value === "object") {
    return `{${Object.keys(value).sort().map((key) =>
      `${goJsonString(key)}:${canonicalJson(value[key])}`).join(",")}}`;
  }
  return goJsonString(value);
}

function fileSha256(filePath) {
  return sha256(fs.readFileSync(filePath));
}

function sameJson(left, right) {
  return JSON.stringify(left) === JSON.stringify(right);
}

function hasExactKeys(value, keys) {
  const record = objectOf(value);
  return sameJson(Object.keys(record).sort(), [...keys].sort());
}

function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

function refreshTranscript(snapshot) {
  snapshot.transcript_sha256 = sha256(JSON.stringify(snapshot.mcp));
  return snapshot;
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function parseArgs(argv) {
  const options = {
    failOnViolation: false,
    json: false,
    output: "",
    selfTestContract: false,
    timeoutMs: 20_000,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--backend-url") next();
    else if (arg.startsWith("--backend-url=")) {
      // Compatibility only. V2 never reads backend process state.
    } else if (arg === "--output") options.output = next();
    else if (arg.startsWith("--output=")) options.output = arg.slice("--output=".length);
    else if (arg === "--timeout-ms") options.timeoutMs = Number(next()) || options.timeoutMs;
    else if (arg.startsWith("--timeout-ms=")) options.timeoutMs = Number(arg.slice("--timeout-ms=".length)) || options.timeoutMs;
    else if (arg === "--fail-on-violation") options.failOnViolation = true;
    else if (arg === "--self-test-contract") options.selfTestContract = true;
    else if (arg === "--json") options.json = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  if (!Number.isFinite(options.timeoutMs) || options.timeoutMs < 1_000) {
    options.timeoutMs = 20_000;
  }
  options.output = text(options.output)
    ? path.resolve(REPO_ROOT, options.output)
    : path.join(REPO_ROOT, "output", "analytix-fund-analysis", "evidence", "mcp-surface-snapshot.json");
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/mcp-surface-snapshot.mjs [options]

Capture a source-bound snapshot of the production funds MCP P0 quarantine.
The snapshot proves the containment contract, never publication readiness.

Options:
  --backend-url <url>    Accepted for CLI compatibility; V2 performs no backend access.
  --output <file>        Snapshot path. Default: output/analytix-fund-analysis/evidence/mcp-surface-snapshot.json.
  --timeout-ms <ms>      MCP request timeout. Default: 20000.
  --fail-on-violation    Exit non-zero when the quarantine surface contract is invalid.
  --self-test-contract   Run mutation tests against the current live snapshot.
  --json                 Print machine-readable output.
`);
}

export function buildCurrentFundsMcpSourceBinding(pluginRoot = PLUGIN_ROOT) {
  const normalizedFiles = SOURCE_BINDING_FILES.map((relativePath) => ({
    path: relativePath,
    sha256: fileSha256(path.join(pluginRoot, relativePath)),
  }));
  const byPath = Object.fromEntries(normalizedFiles.map((entry) => [entry.path, entry.sha256]));
  const mcpConfig = readJson(path.join(pluginRoot, ".mcp.json"));
  const fundsConfig = objectOf(objectOf(mcpConfig.mcpServers).analytix_funds);
  const aggregateMaterial = normalizedFiles
    .map((entry) => `${entry.path}\0${entry.sha256}`)
    .join("\n");
  return {
    manifest_sha256: byPath[".codex-plugin/plugin.json"],
    mcp_config_sha256: byPath[".mcp.json"],
    closure_contract_sha256: byPath["scripts/production-mcp-entry-closure.json"],
    closure_files: PRODUCTION_MCP_ENTRY_CLOSURE_FILES.map((relativePath) => ({
      path: relativePath,
      sha256: byPath[relativePath],
    })),
    aggregate_sha256: sha256(aggregateMaterial),
    production_mcp_disabled: fundsConfig.disabled === true,
  };
}

class JsonRpcRequestError extends Error {
  constructor(error) {
    super(text(error?.message) || "MCP JSON-RPC request failed");
    this.name = "JsonRpcRequestError";
    this.code = Number(error?.code);
    this.data = error?.data;
  }
}

class McpClient {
  constructor({ env = {}, homeDir, timeoutMs }) {
    this.env = env;
    this.homeDir = homeDir;
    this.timeoutMs = timeoutMs;
    this.buffer = "";
    this.child = null;
    this.nextId = 1;
    this.pending = new Map();
    this.stderr = "";
  }

  start() {
    this.child = spawn(process.execPath, ["./mcp/server.mjs"], {
      cwd: PLUGIN_ROOT,
      env: {
        HOME: this.homeDir,
        PATH: process.env.PATH,
        TMPDIR: process.env.TMPDIR,
        ...this.env,
      },
      stdio: ["pipe", "pipe", "pipe"],
    });
    this.child.stdout.on("data", (chunk) => this.onStdout(chunk));
    this.child.stderr.on("data", (chunk) => {
      this.stderr += chunk.toString();
    });
    this.child.once("error", (error) => this.rejectPending(error));
    this.child.once("exit", (code, signal) => {
      this.rejectPending(new Error(
        `MCP exited code=${code ?? ""} signal=${signal ?? ""} ${this.stderr.slice(-300)}`,
      ));
    });
  }

  rejectPending(error) {
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timer);
      pending.reject(error);
    }
    this.pending.clear();
  }

  onStdout(chunk) {
    this.buffer += chunk.toString();
    let newlineIndex = this.buffer.indexOf("\n");
    while (newlineIndex >= 0) {
      const line = this.buffer.slice(0, newlineIndex).trim();
      this.buffer = this.buffer.slice(newlineIndex + 1);
      if (line) this.onMessage(line);
      newlineIndex = this.buffer.indexOf("\n");
    }
  }

  onMessage(line) {
    let message;
    try {
      message = JSON.parse(line);
    } catch {
      return;
    }
    if (!Object.hasOwn(message, "id") || !this.pending.has(message.id)) return;
    const pending = this.pending.get(message.id);
    this.pending.delete(message.id);
    clearTimeout(pending.timer);
    if (message.error) pending.reject(new JsonRpcRequestError(message.error));
    else pending.resolve(message.result);
  }

  request(method, params = {}, timeoutMs = this.timeoutMs) {
    if (!this.child || !this.child.stdin.writable) {
      return Promise.reject(new Error("MCP server is not running"));
    }
    const id = this.nextId++;
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

  async stop() {
    const child = this.child;
    if (!child || child.exitCode !== null || child.signalCode !== null) return;
    await new Promise((resolve) => {
      let settled = false;
      const finish = () => {
        if (settled) return;
        settled = true;
        clearTimeout(killTimer);
        resolve();
      };
      const killTimer = setTimeout(() => {
        try {
          child.kill("SIGKILL");
        } catch {
          finish();
        }
      }, 500);
      child.once("exit", finish);
      try {
        child.stdin.end();
      } catch {
        try {
          child.kill();
        } catch {
          finish();
        }
      }
    });
  }
}

function sortedResources(value) {
  return arrayOf(value?.resources)
    .map((resource) => ({
      uri: text(resource?.uri),
      name: text(resource?.name),
      description: text(resource?.description),
      mimeType: text(resource?.mimeType),
    }))
    .sort((left, right) => left.uri.localeCompare(right.uri));
}

function sortedPrompts(value) {
  return arrayOf(value?.prompts)
    .map((prompt) => ({
      name: text(prompt?.name),
      description: text(prompt?.description),
      arguments: arrayOf(prompt?.arguments).map((argument) => ({
        name: text(argument?.name),
        description: text(argument?.description),
        required: argument?.required === true,
      })),
    }))
    .sort((left, right) => left.name.localeCompare(right.name));
}

function promptText(result) {
  return arrayOf(result?.messages)
    .map((message) => text(message?.content?.text || message?.content))
    .filter(Boolean)
    .join("\n");
}

function resourceText(result) {
  return arrayOf(result?.contents)
    .map((content) => text(content?.text))
    .filter(Boolean)
    .join("\n");
}

async function captureError(request) {
  try {
    await request();
    return { code: 0, message: "request unexpectedly succeeded" };
  } catch (error) {
    return {
      code: Number(error?.code),
      message: text(error?.message).slice(0, 240),
    };
  }
}

function sourceProbeProjection() {
  const datasetManifestDigest = sha256("mcp-surface:dataset-manifest");
  const projection = {
    schemaVersion: 2,
    purpose: "analytix.host.funds-count-projection/v2",
    turnSecurityContextDigest: sha256("mcp-surface:turn-security-context"),
    datasetSnapshotId: `dsv2_${datasetManifestDigest}`,
    datasetSelectionDigest: sha256("mcp-surface:dataset-selection"),
    datasetRecordDigest: sha256("mcp-surface:dataset-record"),
    datasetManifestDigest,
    fundsProducerContentId: `fpc2_${sha256("mcp-surface:funds-producer-content")}`,
    fundsProducerContentManifestSha256: sha256(
      "mcp-surface:funds-producer-content-manifest",
    ),
    detailContentSha256: sha256("mcp-surface:detail-content"),
    tableName: "analysis_txn_detail_idx",
    rowCount: "0",
    projectionDigest: "",
  };
  projection.projectionDigest = crypto.createHash("sha256")
    .update(FUNDS_COUNT_PROJECTION_DIGEST_DOMAIN_V2)
    .update(canonicalJson(projection), "utf8")
    .digest("hex");
  return projection;
}

async function captureSurface(client, { includeNegativeChecks = false } = {}) {
  const initialize = await client.request("initialize", {
    protocolVersion: PREFERRED_PROTOCOL_VERSION,
    capabilities: {},
    clientInfo: { name: "analytix-mcp-surface-snapshot", version: "2.0.0" },
  });
  client.notify("notifications/initialized", {});
  const toolList = await client.request("tools/list");
  const resourceList = await client.request("resources/list");
  const promptList = await client.request("prompts/list");
  const resourceTemplateList = await client.request("resources/templates/list");
  const tools = arrayOf(toolList?.tools)
    .map((tool) => text(tool?.name))
    .filter(Boolean)
    .sort();
  const resources = sortedResources(resourceList);
  const prompts = sortedPrompts(promptList);
  const surface = {
    protocol_version: text(initialize?.protocolVersion),
    server_info: objectOf(initialize?.serverInfo),
    tool_count: tools.length,
    tools,
    resource_count: resources.length,
    resources,
    resource_template_count: arrayOf(resourceTemplateList?.resourceTemplates).length,
    resource_templates: arrayOf(resourceTemplateList?.resourceTemplates),
    prompt_count: prompts.length,
    prompts,
    raw_observations: {
      initialize,
      tools_list: toolList,
      resources_list: resourceList,
      resource_templates_list: resourceTemplateList,
      prompts_list: promptList,
    },
  };
  if (!includeNegativeChecks) return surface;

  const resourceRead = await client.request("resources/read", {
    uri: FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI,
  });
  const promptGet = await client.request("prompts/get", {
    name: FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
  });
  const probeProjection = sourceProbeProjection();
  return {
    ...surface,
    raw_observations: {
      ...surface.raw_observations,
      resource_read: resourceRead,
      prompt_get: promptGet,
    },
    resource_read: {
      uri: text(resourceRead?.contents?.[0]?.uri),
      mimeType: text(resourceRead?.contents?.[0]?.mimeType),
      text_sha256: sha256(resourceText(resourceRead)),
    },
    prompt_get: {
      name: FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
      description: text(promptGet?.description),
      text_sha256: sha256(promptText(promptGet)),
    },
    error_checks: {
      unknown_resource: await captureError(() => client.request("resources/read", {
        uri: "skill://analytix-fund-analysis/SKILL.md",
      })),
      unknown_prompt: await captureError(() => client.request("prompts/get", {
        name: "unknown-p0-prompt",
        arguments: {},
      })),
      prompt_arguments_rejected: await captureError(() => client.request("prompts/get", {
        name: FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
        arguments: { case_id: "must-not-be-accepted" },
      })),
      unadvertised_tool: await captureError(() => client.request("tools/call", {
        name: "run_full_case_analysis",
        arguments: {},
      })),
    },
    source_probe: await client.request("analytix/sourceProbe", {
      _meta: { [FUNDS_COUNT_PROJECTION_META_KEY_V2]: probeProjection },
    }),
  };
}

function expectedResourceRead() {
  const result = fixedFundsBoundaryResourceRead(FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI);
  return {
    uri: text(result?.contents?.[0]?.uri),
    mimeType: text(result?.contents?.[0]?.mimeType),
    text_sha256: sha256(resourceText(result)),
  };
}

function expectedPromptGet() {
  const result = fixedFundsBoundaryPrompt(FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME);
  return {
    name: FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
    description: text(result?.description),
    text_sha256: sha256(promptText(result)),
  };
}

function expectedError(error, message) {
  return Number(error?.code) === -32602 && text(error?.message) === message;
}

function evaluateMcpSurfaceSnapshotV2(snapshot, currentBinding) {
  const payload = objectOf(snapshot);
  const plugin = objectOf(payload.plugin);
  const mcp = objectOf(payload.mcp);
  const hostile = objectOf(mcp.hostile_discovery);
  const raw = objectOf(mcp.raw_observations);
  const sourceProbe = objectOf(mcp.source_probe);
  const errors = objectOf(mcp.error_checks);
  const manifest = readJson(path.join(PLUGIN_ROOT, ".codex-plugin", "plugin.json"));
  const expectedResources = sortedResources({ resources: fixedFundsBoundaryResources() });
  const expectedPrompts = sortedPrompts({ prompts: fixedFundsBoundaryPrompts() });
  const expectedInitialize = {
    protocolVersion: PREFERRED_PROTOCOL_VERSION,
    capabilities: {
      tools: {},
      resources: {},
      prompts: {},
      experimental: {
        analytixEvidenceAuthority: {
          version: 2,
          methods: ["analytix/sourceProbe", "analytix/evidenceRead"],
        },
      },
    },
    serverInfo: { name: EXPECTED_SERVER_NAME, version: manifest.version },
  };
  const expectedRawSurface = {
    initialize: expectedInitialize,
    tools_list: raw.tools_list,
    resources_list: { resources: fixedFundsBoundaryResources() },
    resource_templates_list: { resourceTemplates: [] },
    prompts_list: { prompts: fixedFundsBoundaryPrompts() },
  };
  const expectedSurface = {
    protocol_version: PREFERRED_PROTOCOL_VERSION,
    server_info: { name: EXPECTED_SERVER_NAME, version: manifest.version },
    tool_count: 2,
    tools: ["analyze_account_flows", "count_case_rows"],
    resource_count: expectedResources.length,
    resources: expectedResources,
    resource_template_count: 0,
    resource_templates: [],
    prompt_count: expectedPrompts.length,
    prompts: expectedPrompts,
    raw_observations: expectedRawSurface,
  };
  const checks = {
    top_level_shape: hasExactKeys(payload, [
      "schema_version",
      "generated_at",
      "plugin",
      "source_binding",
      "transcript_sha256",
      "mcp",
    ]),
    schema_version_v2: payload.schema_version === SNAPSHOT_SCHEMA_VERSION,
    generated_at_valid: Number.isFinite(Date.parse(text(payload.generated_at))),
    plugin_identity_current: sameJson(plugin, {
      name: manifest.name,
      version: manifest.version,
    }),
    source_binding_current: sameJson(objectOf(payload.source_binding), currentBinding),
    transcript_integrity:
      text(payload.transcript_sha256) === sha256(JSON.stringify(mcp)),
    production_mcp_disabled: currentBinding.production_mcp_disabled === true,
    mcp_shape: hasExactKeys(mcp, [
      "protocol_version",
      "server_info",
      "tool_count",
      "tools",
      "resource_count",
      "resources",
      "resource_template_count",
      "resource_templates",
      "prompt_count",
      "prompts",
      "raw_observations",
      "resource_read",
      "prompt_get",
      "error_checks",
      "source_probe",
      "hostile_discovery",
    ]),
    protocol_current: text(mcp.protocol_version) === expectedSurface.protocol_version,
    server_identity_current: sameJson(objectOf(mcp.server_info), expectedSurface.server_info),
    raw_observation_shape: hasExactKeys(raw, [
      "initialize",
      "tools_list",
      "resources_list",
      "resource_templates_list",
      "prompts_list",
      "resource_read",
      "prompt_get",
    ]),
    initialize_response_exact: sameJson(objectOf(raw.initialize), expectedInitialize),
    raw_tools_list_golden:
      sha256(JSON.stringify(raw.tools_list)) === RAW_RESPONSE_SHA256.tools_list,
    raw_resources_list_golden:
      sha256(JSON.stringify(raw.resources_list)) === RAW_RESPONSE_SHA256.resources_list,
    raw_resource_templates_list_golden:
      sha256(JSON.stringify(raw.resource_templates_list))
      === RAW_RESPONSE_SHA256.resource_templates_list,
    raw_resource_read_golden:
      sha256(JSON.stringify(raw.resource_read)) === RAW_RESPONSE_SHA256.resource_read,
    raw_prompts_list_golden:
      sha256(JSON.stringify(raw.prompts_list)) === RAW_RESPONSE_SHA256.prompts_list,
    raw_prompt_get_golden:
      sha256(JSON.stringify(raw.prompt_get)) === RAW_RESPONSE_SHA256.prompt_get,
    exact_host_capture_tools:
      Number(mcp.tool_count) === expectedSurface.tool_count
      && sameJson(arrayOf(mcp.tools), expectedSurface.tools),
    fixed_resource_surface:
      Number(mcp.resource_count) === expectedSurface.resource_count
      && sameJson(arrayOf(mcp.resources), expectedSurface.resources),
    no_resource_templates:
      Number(mcp.resource_template_count) === 0
      && sameJson(arrayOf(mcp.resource_templates), []),
    fixed_resource_body: sameJson(objectOf(mcp.resource_read), expectedResourceRead()),
    fixed_boundary_golden:
      text(objectOf(mcp.resource_read).text_sha256) === EXPECTED_BOUNDARY_SHA256
      && text(objectOf(mcp.prompt_get).text_sha256) === EXPECTED_BOUNDARY_SHA256,
    fixed_prompt_surface:
      Number(mcp.prompt_count) === expectedSurface.prompt_count
      && sameJson(arrayOf(mcp.prompts), expectedSurface.prompts),
    fixed_prompt_body: sameJson(objectOf(mcp.prompt_get), expectedPromptGet()),
    negative_check_shape: hasExactKeys(errors, [
      "unknown_resource",
      "unknown_prompt",
      "prompt_arguments_rejected",
      "unadvertised_tool",
    ]),
    unknown_resource_rejected:
      hasExactKeys(errors.unknown_resource, ["code", "message"])
      && expectedError(errors.unknown_resource, "Unknown P0 boundary resource"),
    unknown_prompt_rejected:
      hasExactKeys(errors.unknown_prompt, ["code", "message"])
      && expectedError(errors.unknown_prompt, "Unknown P0 boundary prompt"),
    prompt_arguments_rejected:
      hasExactKeys(errors.prompt_arguments_rejected, ["code", "message"])
      && expectedError(
        errors.prompt_arguments_rejected,
        "P0 boundary prompt does not accept arguments",
      ),
    unadvertised_tool_rejected:
      hasExactKeys(errors.unadvertised_tool, ["code", "message"])
      && expectedError(errors.unadvertised_tool, "Unknown or unadvertised tool"),
    hostile_env_cannot_expand_tools:
      Number(hostile.tool_count) === expectedSurface.tool_count
      && sameJson(arrayOf(hostile.tools), expectedSurface.tools),
    hostile_env_surface_unchanged: sameJson(hostile, expectedSurface),
    source_probe_shape: hasExactKeys(sourceProbe, [
      "schemaVersion",
      "purpose",
      "serverName",
      "serverVersion",
      "projectionDigest",
      "ready",
      "readOnly",
    ]),
    source_probe_projection_bound:
      sourceProbe.schemaVersion === 2
      && text(sourceProbe.purpose) === "analytix.funds-source-probe/v2"
      && sourceProbe.ready === true
      && sourceProbe.readOnly === true
      && text(sourceProbe.projectionDigest) === sourceProbeProjection().projectionDigest
      && text(sourceProbe.serverName) === EXPECTED_SERVER_NAME
      && text(sourceProbe.serverVersion) === text(manifest.version),
  };
  const blockers = Object.entries(checks)
    .filter(([, passed]) => !passed)
    .map(([name]) => name);
  return { checks, blockers, ok: blockers.length === 0 };
}

export function validateFundsMcpSurfaceSnapshotV2(
  snapshot,
  { currentBinding = buildCurrentFundsMcpSourceBinding() } = {},
) {
  const raw = evaluateMcpSurfaceSnapshotV2(snapshot, currentBinding);
  return {
    contract_ready: raw.blockers.length === 0,
    publication_ready: false,
    release_blockers: [...REQUIRED_RELEASE_BLOCKERS],
    checks: raw.checks,
    blockers: raw.blockers,
  };
}

async function buildSnapshot(options) {
  const manifest = readJson(path.join(PLUGIN_ROOT, ".codex-plugin", "plugin.json"));
  const sourceBindingBefore = buildCurrentFundsMcpSourceBinding();
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-funds-mcp-snapshot-"));
  const baselineHome = path.join(tempRoot, "baseline-home");
  const hostileHome = path.join(tempRoot, "hostile-home");
  fs.mkdirSync(baselineHome, { recursive: true });
  fs.mkdirSync(hostileHome, { recursive: true });
  const baselineClient = new McpClient({ homeDir: baselineHome, timeoutMs: options.timeoutMs });
  const hostileClient = new McpClient({
    homeDir: hostileHome,
    timeoutMs: options.timeoutMs,
    env: {
      ANALYTIX_FUNDS_DISCOVERY_MODE: "full",
      ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS: "1",
      ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD: "1",
    },
  });
  baselineClient.start();
  hostileClient.start();
  try {
    const mcp = await captureSurface(baselineClient, { includeNegativeChecks: true });
    mcp.hostile_discovery = await captureSurface(hostileClient);
    const sourceBinding = buildCurrentFundsMcpSourceBinding();
    if (!sameJson(sourceBindingBefore, sourceBinding)) {
      throw new Error("funds MCP source changed while the snapshot was being captured");
    }
    const snapshot = {
      schema_version: SNAPSHOT_SCHEMA_VERSION,
      generated_at: new Date().toISOString(),
      plugin: { name: manifest.name, version: manifest.version },
      source_binding: sourceBinding,
      transcript_sha256: sha256(JSON.stringify(mcp)),
      mcp,
    };
    return snapshot;
  } finally {
    await Promise.all([baselineClient.stop(), hostileClient.stop()]);
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

function contractMutationCases(snapshot) {
  const oldSameVersion = {
    generated_at: snapshot.generated_at,
    plugin: clone(snapshot.plugin),
    mcp: { tool_count: 30, tools: ["count_case_rows"] },
    ok: true,
  };
  const hashMismatch = clone(snapshot);
  hashMismatch.source_binding.aggregate_sha256 = "0".repeat(64);
  const nonemptyTools = clone(snapshot);
  nonemptyTools.mcp.tools.push("run_full_case_analysis");
  nonemptyTools.mcp.tools.sort();
  nonemptyTools.mcp.tool_count += 1;
  nonemptyTools.mcp.raw_observations.tools_list.tools.push({
    name: "run_full_case_analysis",
  });
  const extraResource = clone(snapshot);
  extraResource.mcp.resources.push({
    uri: "analytix://funds/stale-resource",
    name: "stale",
    description: "stale",
    mimeType: "text/plain",
  });
  extraResource.mcp.resource_count += 1;
  extraResource.mcp.raw_observations.resources_list.resources.push({
    uri: "analytix://funds/stale-resource",
    name: "stale",
    description: "stale",
    mimeType: "text/plain",
  });
  const promptArguments = clone(snapshot);
  promptArguments.mcp.prompts[0].arguments.push({
    name: "case_id",
    description: "must not exist",
    required: false,
  });
  promptArguments.mcp.raw_observations.prompts_list.prompts[0].arguments.push({
    name: "case_id",
    description: "must not exist",
    required: false,
  });
  const boundaryDrift = clone(snapshot);
  boundaryDrift.mcp.resource_read.text_sha256 = "f".repeat(64);
  const publicationForgery = clone(snapshot);
  publicationForgery.publication_ready = true;
  return [
    ["old_same_version_snapshot", oldSameVersion],
    ["source_binding_hash_mismatch", hashMismatch],
    ["nonempty_tools", refreshTranscript(nonemptyTools)],
    ["extra_resource", refreshTranscript(extraResource)],
    ["prompt_arguments", refreshTranscript(promptArguments)],
    ["boundary_body_drift", refreshTranscript(boundaryDrift)],
    ["publication_ready_forgery", publicationForgery],
  ];
}

async function selfTestContract(options) {
  const snapshot = await buildSnapshot(options);
  const baseline = validateFundsMcpSurfaceSnapshotV2(snapshot);
  const results = contractMutationCases(snapshot).map(([name, mutation]) => {
    const validation = validateFundsMcpSurfaceSnapshotV2(mutation);
    return {
      name,
      ok: validation.contract_ready === false,
      blockers: validation.blockers,
    };
  });
  return {
    schema_version: SNAPSHOT_SCHEMA_VERSION,
    baseline_contract_ready: baseline.contract_ready,
    mutation_count: results.length,
    results,
    ok: baseline.contract_ready && results.every((result) => result.ok),
  };
}

function printSummary(snapshot, validation, outputPath, json) {
  const summary = {
    schema_version: snapshot.schema_version,
    contract_ready: validation.contract_ready,
    publication_ready: validation.publication_ready,
    output: outputPath,
    tool_count: snapshot.mcp.tool_count,
    resource_count: snapshot.mcp.resource_count,
    prompt_count: snapshot.mcp.prompt_count,
    release_blockers: validation.release_blockers,
    contract_blockers: validation.blockers,
  };
  if (json) console.log(JSON.stringify(summary, null, 2));
  else {
    console.log(`[mcp-surface] contract_ready=${summary.contract_ready}`);
    console.log(`[mcp-surface] publication_ready=${summary.publication_ready}`);
    console.log(`[mcp-surface] output=${summary.output}`);
    console.log(`[mcp-surface] tools=${summary.tool_count} resources=${summary.resource_count} prompts=${summary.prompt_count}`);
    console.log(`[mcp-surface] release_blockers=${summary.release_blockers.join(",")}`);
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.selfTestContract) {
    const result = await selfTestContract(options);
    if (options.json) console.log(JSON.stringify(result, null, 2));
    else {
      console.log(`[mcp-surface-contract] ok=${result.ok}`);
      for (const testCase of result.results) {
        console.log(`  ${testCase.ok ? "ok" : "fail"} ${testCase.name}`);
      }
    }
    if (!result.ok) process.exitCode = 1;
    return;
  }
  const snapshot = await buildSnapshot(options);
  const validation = validateFundsMcpSurfaceSnapshotV2(snapshot);
  fs.mkdirSync(path.dirname(options.output), { recursive: true });
  fs.writeFileSync(options.output, `${JSON.stringify(snapshot, null, 2)}\n`);
  printSummary(snapshot, validation, options.output, options.json);
  if (options.failOnViolation && !validation.contract_ready) process.exitCode = 1;
}

if (process.argv[1] && path.resolve(process.argv[1]) === SCRIPT_PATH) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
