#!/usr/bin/env node

import assert from "node:assert/strict";
import childProcess from "node:child_process";
import crypto from "node:crypto";
import { AsyncResource, createHook } from "node:async_hooks";
import fs from "node:fs";
import fsPromises from "node:fs/promises";
import { syncBuiltinESMExports } from "node:module";
import os from "node:os";
import path from "node:path";

// These references are deliberately captured before the permanent JavaScript
// audit is installed. The calibration below proves the kernel sandbox and the
// writable-root hash inventory still detect the exact bypass class that a
// late monkey patch cannot see.
const preAuditWriteFileSync = fs.writeFileSync.bind(fs);
const preAuditWritevSync = fs.writevSync.bind(fs);
const preAuditSpawnSync = childProcess.spawnSync.bind(childProcess);
const preAuditSetTimeout = globalThis.setTimeout.bind(globalThis);

let createMcpRequestHandlerRuntime;
let McpRequestError;
let PRE_EXECUTION_BLOCKED_TOOL_NAMES;
let createMcpToolResultRuntime;
let createToolCallRuntime;
let compactSkillsRecord;
let unavailableSkillsSurfaceState;
let createAgentOutputCompiler;
let redactAgentPayload;
let createSafeSkillRuntime;
let requiresPublicationBlock;
let counterpartySummaryFromRank;
let renderDiagnosticCardForAgent;
let buildEvidenceLedger;
let withAnswerCardProtocol;
let withClaimReviewProtocol;
let buildFundGraphFromSeedRows;
let createDestinationDiagnosticRuntime;
let withFundGraphProtocol;
let createFundFlowGraphRuntime;
let createDuckdbDiagnostic;
let duckdbPythonChildEnv;
let runPythonDuckdb;
let tools;
let readResourceForAgent;
let resourceDefinitions;
let resourceListForAgent;
let hostFactRuntimeContext;
let writeCaseProjectBinding;
let FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI;
let fixedFundsUnavailableToolResult;

async function loadProductionModulesAfterAuditInstallation() {
  const [
    requestHandlerRuntime,
    toolResultRuntime,
    toolCallRuntimeModule,
    casePipelineRuntime,
    outputCompiler,
    contextHygiene,
    publicationGuard,
    factSummaries,
    cardRenderer,
    evidenceLedger,
    answerCardProtocol,
    claimVerifierProtocol,
    fundgraphBuilder,
    destinationDiagnostic,
    casegraphProtocol,
    flowGraphRuntime,
    duckdbDiagnostic,
    duckdbWorkbench,
    toolSchemas,
    progressiveResources,
    sourceUnavailableBoundary,
    safeSkillRuntime,
    hostRuntimeFixture,
  ] = await Promise.all([
    import("../mcp/mcp-request-handler-runtime.mjs"),
    import("../mcp/mcp-tool-result-runtime.mjs"),
    import("../mcp/tool-call-runtime.mjs"),
    import("../mcp/case-pipeline-runtime.mjs"),
    import("../mcp/agent-output-compiler.mjs"),
    import("../mcp/agent-context-hygiene.mjs"),
    import("../mcp/report-publication-guard.mjs"),
    import("../mcp/frontdoor-fact-summaries.mjs"),
    import("../mcp/card-renderer.mjs"),
    import("../mcp/evidence-ledger.mjs"),
    import("../mcp/answer-card-protocol.mjs"),
    import("../mcp/claim-verifier-protocol.mjs"),
    import("../mcp/fundgraph-builder.mjs"),
    import("../mcp/destination-diagnostic-runtime.mjs"),
    import("../mcp/casegraph-protocol.mjs"),
    import("../mcp/fund-flow-graph-runtime.mjs"),
    import("../mcp/duckdb-diagnostic.mjs"),
    import("../mcp/duckdb-workbench-runtime.mjs"),
    import("../mcp/tool-schemas.mjs"),
    import("../mcp/progressive-resources.mjs"),
    import("../mcp/source-unavailable-boundary.mjs"),
    import("../mcp/safe-skill-runtime.mjs"),
    import("./host-runtime-context-fixture.mjs"),
  ]);
  ({
    createMcpRequestHandlerRuntime,
    McpRequestError,
    PRE_EXECUTION_BLOCKED_TOOL_NAMES,
  } = requestHandlerRuntime);
  ({ createMcpToolResultRuntime } = toolResultRuntime);
  ({ createToolCallRuntime } = toolCallRuntimeModule);
  ({ compactSkillsRecord, unavailableSkillsSurfaceState } = casePipelineRuntime);
  ({ createAgentOutputCompiler } = outputCompiler);
  ({ redactAgentPayload } = contextHygiene);
  ({ requiresPublicationBlock } = publicationGuard);
  ({ counterpartySummaryFromRank } = factSummaries);
  ({ renderDiagnosticCardForAgent } = cardRenderer);
  ({ buildEvidenceLedger } = evidenceLedger);
  ({ withAnswerCardProtocol } = answerCardProtocol);
  ({ withClaimReviewProtocol } = claimVerifierProtocol);
  ({ buildFundGraphFromSeedRows } = fundgraphBuilder);
  ({ createDestinationDiagnosticRuntime } = destinationDiagnostic);
  ({ withFundGraphProtocol } = casegraphProtocol);
  ({ createFundFlowGraphRuntime } = flowGraphRuntime);
  ({ createDuckdbDiagnostic } = duckdbDiagnostic);
  ({ duckdbPythonChildEnv, runPythonDuckdb } = duckdbWorkbench);
  ({ tools } = toolSchemas);
  ({ readResourceForAgent, resourceDefinitions, resourceListForAgent } = progressiveResources);
  ({ FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI, fixedFundsUnavailableToolResult } = sourceUnavailableBoundary);
  ({ createSafeSkillRuntime } = safeSkillRuntime);
  ({ hostFactRuntimeContext, writeCaseProjectBinding } = hostRuntimeFixture);
}

const PLUGIN_ROOT = path.resolve(
  path.dirname(new URL(import.meta.url).pathname),
  "..",
);
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "..", "..");
const RAW_SENTINELS = [
  "6222021234567890123",
  "11010519491231002X",
  "13812345678",
  "AA:BB:CC:DD:EE:FF",
  "192.168.1.2",
  "2645472",
];
const FORBIDDEN_PROVIDER_RESOURCE_TEXT = [
  "江苏航案件分析",
  "江苏航宇建设工程有限公司",
  "38dc50241996",
  "2645472",
  "2,645,472",
  "current-case DuckDB report artifact fallback",
  "backend downtime should degrade to current-case DuckDB facts",
  "report requests can generate and inspect current-case DuckDB markdown/manifest artifacts locally",
  "current-case DuckDB fallback before returning a capability gap",
];
const FORBIDDEN_PROVIDER_RESOURCE_PATTERNS = [
  /(?:output\/analytix-fund-analysis|frontdoor-p0-oracle)\/[0-9]{4}-[0-9]{2}-[0-9]{2}[^\s)`]*/iu,
  /--case-(?:id|project-root)\s+[^\s)`]+/iu,
  /(?:current-case|local)\s+DuckDB\s+(?:report(?:\s+artifact)?|markdown\/manifest\s+artifact)\s+fallback/iu,
];

const FILESYSTEM_WRITE_METHODS = [
  "appendFile",
  "appendFileSync",
  "chmod",
  "chmodSync",
  "chown",
  "chownSync",
  "copyFile",
  "copyFileSync",
  "cp",
  "cpSync",
  "createWriteStream",
  "fchmod",
  "fchmodSync",
  "fchown",
  "fchownSync",
  "fdatasync",
  "fdatasyncSync",
  "ftruncate",
  "ftruncateSync",
  "fsync",
  "fsyncSync",
  "futimes",
  "futimesSync",
  "link",
  "linkSync",
  "lchown",
  "lchownSync",
  "lutimes",
  "lutimesSync",
  "mkdir",
  "mkdirSync",
  "mkdtemp",
  "mkdtempSync",
  "rename",
  "renameSync",
  "rm",
  "rmSync",
  "rmdir",
  "rmdirSync",
  "symlink",
  "symlinkSync",
  "truncate",
  "truncateSync",
  "unlink",
  "unlinkSync",
  "utimes",
  "utimesSync",
  "write",
  "writeSync",
  "writeFile",
  "writeFileSync",
  "writev",
  "writevSync",
];
const PROCESS_START_METHODS = [
  "exec",
  "execFile",
  "execFileSync",
  "execSync",
  "fork",
  "spawn",
  "spawnSync",
];
const FILE_HANDLE_WRITE_METHODS = [
  "appendFile",
  "chmod",
  "chown",
  "createWriteStream",
  "datasync",
  "sync",
  "truncate",
  "utimes",
  "write",
  "writeFile",
  "writev",
];
const ASYNC_SIDE_EFFECT_METHODS = [
  "queueMicrotask",
  "setImmediate",
  "setInterval",
  "setTimeout",
];
const ASYNC_EFFECT_RESOURCE_TYPES = new Set([
  "FSREQCALLBACK",
  "FSREQPROMISE",
  "GETADDRINFOREQWRAP",
  "Immediate",
  "MESSAGEPORT",
  "Microtask",
  "PIPECONNECTWRAP",
  "PIPEWRAP",
  "PROCESSWRAP",
  "SHUTDOWNWRAP",
  "TCPCONNECTWRAP",
  "TCPWRAP",
  "Timeout",
  "WORKER",
  "WRITEWRAP",
]);
const DARWIN_SANDBOX_CHILD_ENV = "ANALYTIX_FUNDS_P0_SANDBOX_CHILD_V1";
const DARWIN_SANDBOX_ROOT_ENV = "ANALYTIX_FUNDS_P0_SANDBOX_ROOT_V1";

const sideEffectAudit = {
  active: false,
  installed: false,
  attempts: null,
  asyncIds: null,
  asyncResources: null,
  root: "",
};

const sideEffectAsyncHook = createHook({
  init(asyncId, type, triggerAsyncId, resource) {
    if (!sideEffectAudit.active || !sideEffectAudit.asyncIds?.has(triggerAsyncId)) return;
    sideEffectAudit.asyncIds.add(asyncId);
    if (!ASYNC_EFFECT_RESOURCE_TYPES.has(type)) return;
    sideEffectAudit.attempts?.push({ method: `async_hooks:${type}`, target: "<async-resource>" });
    sideEffectAudit.asyncResources?.set(asyncId, { type, resource });
  },
});

function openFlagsCanWrite(flags) {
  if (typeof flags === "number") {
    const writeMask = fs.constants.O_WRONLY
      | fs.constants.O_RDWR
      | fs.constants.O_CREAT
      | fs.constants.O_TRUNC
      | fs.constants.O_APPEND;
    return (flags & writeMask) !== 0;
  }
  return /[+awx]/u.test(String(flags || ""));
}

function recordBlockedSideEffect(method, target, prefix) {
  const attempts = sideEffectAudit.attempts;
  if (Array.isArray(attempts)) {
    attempts.push({ method, target });
  }
  throw new Error(`${prefix}:${method}`);
}

function patchPermanentAuditMethod(target, method, {
  guard = () => true,
  prefix = "filesystem_write_forbidden",
  targetFor = (args) => typeof args[0] === "string" ? args[0] : "<non-string-target>",
} = {}) {
  if (!target || typeof target[method] !== "function") return;
  const original = target[method];
  if (original.__analytixP0AuditWrapper === true) return;
  const wrapped = function analytixP0AuditedSideEffect(...args) {
    if (!sideEffectAudit.active || !guard(args)) {
      return original.apply(this, args);
    }
    return recordBlockedSideEffect(method, targetFor(args), prefix);
  };
  Object.defineProperty(wrapped, "__analytixP0AuditWrapper", {
    value: true,
    configurable: false,
    enumerable: false,
    writable: false,
  });
  target[method] = wrapped;
}

async function installPermanentSideEffectAudit(root) {
  if (sideEffectAudit.installed) {
    assert.equal(sideEffectAudit.root, root, "side-effect audit root changed");
    return;
  }
  sideEffectAudit.root = root;
  for (const target of [fs, fs.promises, fsPromises]) {
    for (const method of FILESYSTEM_WRITE_METHODS) {
      patchPermanentAuditMethod(target, method);
    }
    patchPermanentAuditMethod(target, "open", {
      guard: (args) => openFlagsCanWrite(args[1]),
    });
    patchPermanentAuditMethod(target, "openSync", {
      guard: (args) => openFlagsCanWrite(args[1]),
    });
  }
  const readOnlyHandle = await fsPromises.open("/dev/null", "r");
  try {
    const fileHandlePrototype = Object.getPrototypeOf(readOnlyHandle);
    for (const method of FILE_HANDLE_WRITE_METHODS) {
      patchPermanentAuditMethod(fileHandlePrototype, method, {
        targetFor: () => "<file-handle>",
      });
    }
  } finally {
    await readOnlyHandle.close();
  }
  for (const method of PROCESS_START_METHODS) {
    patchPermanentAuditMethod(childProcess, method, {
      prefix: "subprocess_start_forbidden",
      targetFor: (args) => typeof args[0] === "string" ? args[0] : "<non-string-command>",
    });
  }
  for (const method of ASYNC_SIDE_EFFECT_METHODS) {
    patchPermanentAuditMethod(globalThis, method, {
      prefix: "async_side_effect_forbidden",
      targetFor: () => "<scheduled-callback>",
    });
  }
  syncBuiltinESMExports();
  sideEffectAsyncHook.enable();
  sideEffectAudit.installed = true;
}

function snapshotAuditRoot(root) {
  const records = [];
  const visit = (current, relativePath) => {
    const stat = fs.lstatSync(current);
    const record = {
      path: relativePath || ".",
      mode: stat.mode,
      size: stat.size,
      type: stat.isDirectory() ? "directory" : stat.isFile() ? "file" : stat.isSymbolicLink() ? "symlink" : "other",
    };
    if (stat.isFile()) {
      record.sha256 = crypto.createHash("sha256").update(fs.readFileSync(current)).digest("hex");
    } else if (stat.isSymbolicLink()) {
      record.target = fs.readlinkSync(current);
    }
    records.push(record);
    if (!stat.isDirectory()) return;
    for (const name of fs.readdirSync(current).sort()) {
      visit(path.join(current, name), relativePath ? path.join(relativePath, name) : name);
    }
  };
  visit(root, "");
  return records;
}

async function withBlockedPublicationSideEffectAudit(operation) {
  assert.equal(sideEffectAudit.installed, true, "side-effect audit is not installed");
  assert.equal(sideEffectAudit.active, false, "nested side-effect audit is forbidden");
  assert.equal(typeof operation, "function", "side-effect audit operation is required");
  const attempts = [];
  const before = snapshotAuditRoot(sideEffectAudit.root);
  const asyncIds = new Set();
  const asyncResources = new Map();
  const auditScope = new AsyncResource("AnalytixFundsP0SideEffectAudit");
  asyncIds.add(auditScope.asyncId());
  sideEffectAudit.attempts = attempts;
  sideEffectAudit.asyncIds = asyncIds;
  sideEffectAudit.asyncResources = asyncResources;
  sideEffectAudit.active = true;
  let result;
  let operationError = null;
  let auditError = null;
  try {
    try {
      result = await auditScope.runInAsyncScope(operation);
    } catch (error) {
      operationError = error;
    } finally {
      auditScope.emitDestroy();
    }
    for (const { type, resource } of asyncResources.values()) {
      if (type === "Timeout") clearTimeout(resource);
      if (type === "Immediate") clearImmediate(resource);
    }
    try {
      assert.deepEqual(
        attempts,
        [],
        "publication-blocked path attempted a filesystem mutation, subprocess start, or deferred side effect",
      );
      assert.deepEqual(
        snapshotAuditRoot(sideEffectAudit.root),
        before,
        "publication-blocked path changed the only sandbox-writable filesystem root",
      );
    } catch (error) {
      auditError = error;
    }
    if (operationError && auditError) {
      throw new AggregateError(
        [operationError, auditError],
        `${operationError?.message || operationError}; ${auditError?.message || auditError}`,
      );
    }
    if (auditError) throw auditError;
    if (operationError) throw operationError;
    return result;
  } finally {
    sideEffectAudit.active = false;
    sideEffectAudit.attempts = null;
    sideEffectAudit.asyncIds = null;
    sideEffectAudit.asyncResources = null;
  }
}

function assertNoRawSentinel(value, label) {
  const body = typeof value === "string" ? value : JSON.stringify(value);
  for (const sentinel of RAW_SENTINELS) {
    assert.equal(body.includes(sentinel), false, `${label} leaked ${sentinel}`);
  }
}

function assertCaseNeutralProviderResource(value, label) {
  const body = typeof value === "string" ? value : JSON.stringify(value);
  for (const forbidden of FORBIDDEN_PROVIDER_RESOURCE_TEXT) {
    assert.equal(
      body.includes(forbidden),
      false,
      `${label} exposed forbidden resource text: ${forbidden}`,
    );
  }
  for (const pattern of FORBIDDEN_PROVIDER_RESOURCE_PATTERNS) {
    assert.equal(
      pattern.test(body),
      false,
      `${label} exposed forbidden resource pattern: ${pattern}`,
    );
  }
}

const FUNDS_COUNT_PROJECTION_DIGEST_DOMAIN_V2 = Buffer.from(
  "analytix.host.funds-count-projection/digest/v2\0",
  "utf8",
);
const FUNDS_COUNT_PROJECTION_META_KEY_V2 = "analytixFundsCountProjectionV2";
const FUNDS_COUNT_TABLE_V2 = "analysis_txn_detail_idx";
const FUNDS_COUNT_PROJECTION_REQUIRED_KEYS_V2 = [
  "schemaVersion",
  "purpose",
  "turnSecurityContextDigest",
  "datasetSnapshotId",
  "datasetSelectionDigest",
  "datasetRecordDigest",
  "datasetManifestDigest",
  "fundsProducerContentId",
  "fundsProducerContentManifestSha256",
  "detailContentSha256",
  "tableName",
  "rowCount",
  "projectionDigest",
];

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

function fixtureDigest(label) {
  return crypto.createHash("sha256").update(label, "utf8").digest("hex");
}

function fundsCountProjectionDigestV2(projection) {
  return crypto.createHash("sha256")
    .update(FUNDS_COUNT_PROJECTION_DIGEST_DOMAIN_V2)
    .update(canonicalJson({ ...projection, projectionDigest: "" }), "utf8")
    .digest("hex");
}

function fundsCountProjectionV2(overrides = {}) {
  const datasetManifestDigest = fixtureDigest("p0-contract:dataset-manifest");
  const projection = {
    schemaVersion: 2,
    purpose: "analytix.host.funds-count-projection/v2",
    turnSecurityContextDigest: fixtureDigest("p0-contract:turn-security-context"),
    datasetSnapshotId: `dsv2_${datasetManifestDigest}`,
    datasetSelectionDigest: fixtureDigest("p0-contract:dataset-selection"),
    datasetRecordDigest: fixtureDigest("p0-contract:dataset-record"),
    datasetManifestDigest,
    fundsProducerContentId: `fpc2_${fixtureDigest("p0-contract:funds-producer-content")}`,
    fundsProducerContentManifestSha256: fixtureDigest(
      "p0-contract:funds-producer-content-manifest",
    ),
    detailContentSha256: fixtureDigest("p0-contract:detail-content"),
    tableName: FUNDS_COUNT_TABLE_V2,
    rowCount: "37",
    projectionDigest: "",
    ...overrides,
  };
  projection.projectionDigest = fundsCountProjectionDigestV2(projection);
  return projection;
}

function fundsCountToolV2Contract() {
  const sha256Schema = { type: "string", pattern: "^[a-f0-9]{64}$" };
  const projectionSchema = {
    type: "object",
    properties: {
      schemaVersion: { type: "integer", const: 2 },
      purpose: {
        type: "string",
        const: "analytix.host.funds-count-projection/v2",
      },
      turnSecurityContextDigest: { ...sha256Schema },
      datasetSnapshotId: {
        type: "string",
        pattern: "^dsv2_[a-f0-9]{64}$",
      },
      datasetSelectionDigest: { ...sha256Schema },
      datasetRecordDigest: { ...sha256Schema },
      datasetManifestDigest: { ...sha256Schema },
      fundsProducerContentId: {
        type: "string",
        pattern: "^fpc[12]_[a-f0-9]{64}$",
      },
      fundsProducerContentManifestSha256: { ...sha256Schema },
      detailContentSha256: { ...sha256Schema },
      tableName: { type: "string", const: FUNDS_COUNT_TABLE_V2 },
      rowCount: {
        type: "string",
        pattern: "^(0|[1-9][0-9]*)$",
        maxLength: 20,
      },
      projectionDigest: { ...sha256Schema },
    },
    required: [...FUNDS_COUNT_PROJECTION_REQUIRED_KEYS_V2],
    additionalProperties: false,
  };
  return {
    name: "count_case_rows",
    title: "当前不可变资金明细计数",
    description: "Returns only the host-projected row count for the exact immutable DSV2/FPC selection. The result is evidence material and never grants publication authority.",
    annotations: {
      title: "当前不可变资金明细计数",
      readOnlyHint: true,
      destructiveHint: false,
      idempotentHint: true,
      openWorldHint: false,
    },
    execution: { taskSupport: "forbidden" },
    inputSchema: {
      type: "object",
      properties: {
        table_name: { type: "string", const: FUNDS_COUNT_TABLE_V2 },
      },
      required: ["table_name"],
      additionalProperties: false,
    },
    outputSchema: {
      type: "object",
      properties: {
        schemaVersion: { type: "integer", const: 2 },
        purpose: {
          type: "string",
          const: "analytix.funds-count-tool-outcome/v2",
        },
        semanticStatus: { type: "string", const: "success" },
        data: projectionSchema,
      },
      required: ["schemaVersion", "purpose", "semanticStatus", "data"],
      additionalProperties: false,
    },
  };
}

function fundsAccountFlowToolV1Contract() {
  return {
    name: "analyze_account_flows",
    title: "账户资金流入流出分析",
    description: "Requests a bounded account-flow analysis for one host-resolved case-scoped account or card alias and inclusive time range. The Go host captures the call and owns all snapshot, identity, query, evidence, claim, and publication effects.",
    annotations: {
      title: "账户资金流入流出分析",
      readOnlyHint: true,
      destructiveHint: false,
      idempotentHint: true,
      openWorldHint: false,
    },
    execution: { taskSupport: "forbidden" },
    inputSchema: {
      type: "object",
      properties: {
        subject_alias: {
          type: "string",
          pattern: "^(acct|card):(?:[1-9][0-9]{0,8}|[1-3][0-9]{9}|4[01][0-9]{8}|42[0-8][0-9]{7}|429[0-3][0-9]{6}|4294[0-8][0-9]{5}|42949[0-5][0-9]{4}|429496[0-6][0-9]{3}|4294967[0-1][0-9]{2}|42949672[0-8][0-9]|429496729[0-5])$",
          minLength: 6,
          maxLength: 15,
        },
        start_inclusive: {
          type: "string",
          pattern: "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$",
          minLength: 27,
          maxLength: 27,
        },
        end_inclusive: {
          type: "string",
          pattern: "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$",
          minLength: 27,
          maxLength: 27,
        },
        evidence_row_limit: { type: "integer", minimum: 1, maximum: 512 },
      },
      required: [
        "subject_alias",
        "start_inclusive",
        "end_inclusive",
        "evidence_row_limit",
      ],
      additionalProperties: false,
    },
    outputSchema: {
      type: "object",
      properties: {
        schemaVersion: { type: "integer", const: 1 },
        purpose: {
          type: "string",
          const: "analytix.funds-account-flow-host-capture/v1",
        },
        semanticStatus: { type: "string", const: "host_authority_required" },
        hostCaptureRequired: { type: "boolean", const: true },
        factAnswerAllowed: { type: "boolean", const: false },
      },
      required: [
        "schemaVersion",
        "purpose",
        "semanticStatus",
        "hostCaptureRequired",
        "factAnswerAllowed",
      ],
      additionalProperties: false,
    },
  };
}

function assertFundsCountProjectionV2IsPathAndPiiFree(projection, label) {
  assert.deepEqual(
    Object.keys(projection).sort(),
    [...FUNDS_COUNT_PROJECTION_REQUIRED_KEYS_V2].sort(),
    `${label} must contain only the V2 projection fields`,
  );
  const body = JSON.stringify(projection);
  assert.doesNotMatch(
    body,
    /(?:\/Users\/|\/Volumes\/|file:|workspace|projectRoot|account(?:No|Number)?|bank(?:No|Number)?|card(?:No|Number)?|identity|phone)/iu,
    `${label} exposed a path or direct PII field`,
  );
  assertNoRawSentinel(projection, label);
}

function assertClosedExternalSchema(schema, label) {
  assert(
    schema && typeof schema === "object" && !Array.isArray(schema),
    `${label} must be an object schema`,
  );
  const variants = Array.isArray(schema.oneOf) ? schema.oneOf : [];
  assert(
    typeof schema.type === "string" || variants.length > 0,
    `${label}.type or .oneOf is required`,
  );
  for (const [index, variant] of variants.entries()) {
    assertClosedExternalSchema(variant, `${label}.oneOf[${index}]`);
  }
  if (schema.type === "object") {
    assert.equal(
      schema.additionalProperties,
      false,
      `${label} must reject additional properties`,
    );
    assert(
      schema.properties &&
        typeof schema.properties === "object" &&
        !Array.isArray(schema.properties),
      `${label}.properties must be an object`,
    );
    for (const [key, child] of Object.entries(schema.properties)) {
      assertClosedExternalSchema(child, `${label}.properties.${key}`);
    }
  }
  if (schema.type === "array") {
    assertClosedExternalSchema(schema.items, `${label}.items`);
  }
}

function productionSourceFiles(root) {
  const files = [];
  const visit = (current) => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const target = path.join(current, entry.name);
      if (entry.isDirectory()) {
        visit(target);
      } else if (entry.isFile() && /\.(?:mjs|py|rs)$/u.test(entry.name)) {
        files.push(target);
      }
    }
  };
  visit(root);
  return files;
}

async function expectMcpError(promise, code) {
  try {
    await promise;
    assert.fail(`expected MCP error ${code}`);
  } catch (error) {
    assert.equal(
      error instanceof McpRequestError,
      true,
      "error must be McpRequestError",
    );
    assert.equal(error.code, code);
  }
}

function outcomeRuntime() {
  return createMcpToolResultRuntime({
    serverName: "analytix_funds",
    serverVersion: "p0-test",
    compactToolText: () => "must-not-be-used",
    compactStructuredContent: () => ({ must_not_be_used: true }),
  });
}

function createHandler({
  env = {},
  toolResult = outcomeRuntime().toolResult,
  callTool: callToolOverride,
} = {}) {
  let calls = 0;
  const handler = createMcpRequestHandlerRuntime({
    serverName: "analytix_funds",
    serverVersion: "p0-test",
    pluginRoot: PLUGIN_ROOT,
    env,
    callTool: async (name, args) => {
      calls += 1;
      if (callToolOverride) return callToolOverride(name, args);
      return {
        status: "ok",
        amount: 2645472,
        account_no: RAW_SENTINELS[0],
        id_no: RAW_SENTINELS[1],
        phone: RAW_SENTINELS[2],
        mac_addr: RAW_SENTINELS[3],
        ip_addr: RAW_SENTINELS[4],
        evidence_receipt_ids: ["fake-receipt"],
      };
    },
    toolResult,
  });
  return { ...handler, calls: () => calls };
}

async function initializeHandler(handler, protocolVersion = "2025-11-25") {
  const initialized = await handler.handleRequest({
    method: "initialize",
    params: {
      protocolVersion,
      capabilities: {},
      clientInfo: { name: "p0-contract", version: "1.0.0" },
    },
  });
  await handler.handleNotification({
    method: "notifications/initialized",
    params: {},
  });
  return initialized;
}

function toolCallRuntime(overrides = {}) {
  const counters = {
    funds: 0,
    execute: 0,
    resolve: 0,
  };
  const runtime = createToolCallRuntime({
    fundsInvestigate: async (args) => {
      counters.funds += 1;
      return { status: "ok", args };
    },
    getCaseGraph: async () => ({ status: "ok" }),
    buildFundFlowGraph: async () => ({ status: "ok" }),
    resolveCase: async () => {
      counters.resolve += 1;
      return { case_id: "case-test", source: "test", case: {} };
    },
    compactCaseRecord: (value) => value,
    compactSkillsRecord: (value) => value,
    httpJson: async () => ({}),
    getImportOverview: async () => ({}),
    getCleaningOverview: async () => ({}),
    getCaseDataPipelineOverview: async () => ({}),
    getCaseScopeMap: async () => ({}),
    getStatsMeta: async () => ({}),
    getStatsTree: async () => ({}),
    queryStatsRows: async () => ({}),
    queryStatsTxnRows: async () => ({}),
    queryAccountTxnRows: async () => ({}),
    getAnalysisDashboard: async () => ({}),
    exportCleanedCaseData: async () => {
      throw new Error("export must not execute");
    },
    executeSkill: async (_skillId, args) => {
      counters.execute += 1;
      return { status: "ok", data: { write_report: args.write_report } };
    },
    env: {
      ANALYTIX_FUNDS_DISABLE_LOCAL_DUCKDB_FALLBACK: "1",
    },
    ...overrides,
  });
  return { ...runtime, counters };
}

function canonicalSandboxRootFromEnvironment() {
  const configured = String(process.env[DARWIN_SANDBOX_ROOT_ENV] || "").trim();
  if (!path.isAbsolute(configured) || configured === path.parse(configured).root) {
    throw new Error("p0_side_effect_sandbox_root_invalid");
  }
  const canonical = fs.realpathSync(configured);
  const stat = fs.lstatSync(canonical);
  if (!stat.isDirectory() || stat.isSymbolicLink() || (stat.mode & 0o077) !== 0) {
    throw new Error("p0_side_effect_sandbox_root_invalid");
  }
  return canonical;
}

function sandboxLiteral(value) {
  return JSON.stringify(String(value));
}

function runDarwinSandboxController() {
  const rawRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-funds-p0-sandbox-"));
  const root = fs.realpathSync(rawRoot);
  const scriptPath = path.resolve(new URL(import.meta.url).pathname);
  const nodePath = path.resolve(process.execPath);
  const profile = [
    "(version 1)",
    "(allow default)",
    `(deny file-write* (require-not (subpath ${sandboxLiteral(root)})))`,
    `(deny process-exec (require-not (require-any (literal ${sandboxLiteral(nodePath)}) (literal "/usr/bin/env") (subpath ${sandboxLiteral(root)}))))`,
  ].join(" ");
  let result;
  try {
    result = childProcess.spawnSync(
      "/usr/bin/sandbox-exec",
      ["-p", profile, nodePath, scriptPath],
      {
        env: {
          ...process.env,
          [DARWIN_SANDBOX_CHILD_ENV]: "1",
          [DARWIN_SANDBOX_ROOT_ENV]: root,
        },
        stdio: "inherit",
        timeout: 120_000,
        windowsHide: true,
      },
    );
  } finally {
    fs.rmSync(rawRoot, { recursive: true, force: true });
  }
  if (result?.error || result?.signal || result?.status !== 0) {
    throw new Error("p0_side_effect_sandbox_child_failed");
  }
  return { delegated: true };
}

async function run(authorityTempRoot) {
  const forbiddenCalibrationPath = path.join(
    authorityTempRoot,
    `analytix-funds-forbidden-write-${process.pid}.txt`,
  );
  const kernelDeniedPath = path.join(
    path.dirname(authorityTempRoot),
    `analytix-funds-kernel-denied-${process.pid}.txt`,
  );
  await assert.rejects(
    () => withBlockedPublicationSideEffectAudit(() =>
      Promise.resolve(preAuditWriteFileSync(kernelDeniedPath, "must-not-exist"))),
    (error) => error?.code === "EPERM",
    "the Darwin sandbox must deny a pre-audit write reference outside the sole writable root",
  );
  assert.equal(fs.existsSync(kernelDeniedPath), false);
  const kernelSpawnResult = await withBlockedPublicationSideEffectAudit(() =>
    Promise.resolve(preAuditSpawnSync("/usr/bin/true")));
  assert.equal(
    kernelSpawnResult.error?.code,
    "EPERM",
    "the Darwin sandbox must deny a pre-audit process reference before exec",
  );
  assert.equal(kernelSpawnResult.status, null);
  await assert.rejects(
    () => withBlockedPublicationSideEffectAudit(() =>
      fsPromises.writeFile(forbiddenCalibrationPath, "must-not-exist")),
    /filesystem_write_forbidden:writeFile/u,
    "filesystem side-effect audit must reject an attempted write on an arbitrary path",
  );
  assert.equal(fs.existsSync(forbiddenCalibrationPath), false);
  await assert.rejects(
    () => withBlockedPublicationSideEffectAudit(() =>
      Promise.resolve(childProcess.spawn(process.execPath, ["--version"]))),
    /subprocess_start_forbidden:spawn/u,
    "side-effect audit must reject subprocess creation before the process starts",
  );
  const preCapturedWriteFileSync = fs.writeFileSync;
  const preCapturedSpawn = childProcess.spawn;
  await assert.rejects(
    () => withBlockedPublicationSideEffectAudit(() =>
      Promise.resolve(preCapturedWriteFileSync(forbiddenCalibrationPath, "must-not-exist"))),
    /filesystem_write_forbidden:writeFileSync/u,
    "pre-captured filesystem functions must remain governed while the audit is active",
  );
  await assert.rejects(
    () => withBlockedPublicationSideEffectAudit(() =>
      Promise.resolve(preCapturedSpawn(process.execPath, ["--version"]))),
    /subprocess_start_forbidden:spawn/u,
    "pre-captured process functions must remain governed while the audit is active",
  );
  const descriptorCalibrationPath = path.join(authorityTempRoot, "preopened-fd.txt");
  fs.writeFileSync(descriptorCalibrationPath, "baseline");
  const descriptor = fs.openSync(descriptorCalibrationPath, "r+");
  try {
    await assert.rejects(
      () => withBlockedPublicationSideEffectAudit(() =>
        Promise.resolve(fs.writevSync(descriptor, [Buffer.from("mutated")], 0))),
      /filesystem_write_forbidden:writevSync/u,
      "pre-opened numeric descriptors must not bypass writev interception",
    );
  } finally {
    fs.closeSync(descriptor);
  }
  const rawDescriptor = fs.openSync(descriptorCalibrationPath, "r+");
  try {
    await assert.rejects(
      () => withBlockedPublicationSideEffectAudit(() =>
        Promise.resolve(preAuditWritevSync(rawDescriptor, [Buffer.from("changed!")], 0))),
      /changed the only sandbox-writable filesystem root/u,
      "the full-hash inventory must detect a pre-audit fd writer inside the allowed test root",
    );
  } finally {
    fs.closeSync(rawDescriptor);
    fs.writeFileSync(descriptorCalibrationPath, "baseline");
  }
  const fileHandleCalibrationPath = path.join(authorityTempRoot, "preopened-file-handle.txt");
  fs.writeFileSync(fileHandleCalibrationPath, "baseline");
  const fileHandle = await fsPromises.open(fileHandleCalibrationPath, "r+");
  try {
    await assert.rejects(
      () => withBlockedPublicationSideEffectAudit(() => fileHandle.writeFile("mutated")),
      /filesystem_write_forbidden:writeFile/u,
      "pre-opened FileHandle instances must not bypass prototype interception",
    );
  } finally {
    await fileHandle.close();
  }
  await assert.rejects(
    () => withBlockedPublicationSideEffectAudit(() => {
      globalThis.setTimeout(() => undefined, 0);
      return Promise.resolve();
    }),
    /async_side_effect_forbidden:setTimeout/u,
    "publication-blocked paths must not defer work until after their response",
  );
  await assert.rejects(
    () => withBlockedPublicationSideEffectAudit(() => {
      preAuditSetTimeout(() => {
        try {
          preCapturedWriteFileSync(forbiddenCalibrationPath, "must-not-exist");
        } catch {
          // The callback should be cancelled from the async-resource ledger;
          // this guard would still consume the expected audit rejection.
        }
      }, 5_000);
      return Promise.resolve();
    }),
    /publication-blocked path attempted/u,
    "the async-resource ledger must detect and cancel callbacks using a pre-captured timer without a timing window",
  );
  assert.equal(fs.existsSync(forbiddenCalibrationPath), false);
  const rejectingMutationPath = path.join(authorityTempRoot, "rejecting-operation-mutation.txt");
  await assert.rejects(
    () => withBlockedPublicationSideEffectAudit(() => {
      preAuditWriteFileSync(rejectingMutationPath, "must-be-detected");
      throw new Error("operation_rejected_after_mutation");
    }),
    (error) => error instanceof AggregateError
      && error.message.includes("operation_rejected_after_mutation")
      && error.message.includes("changed the only sandbox-writable filesystem root"),
    "an operation rejection must not bypass the side-effect inventory assertion",
  );
  fs.rmSync(rejectingMutationPath, { force: true });

  const authorityProjectRoot = path.join(authorityTempRoot, "case-project");
  fs.mkdirSync(authorityProjectRoot, { recursive: true });
  const authorityBinding = writeCaseProjectBinding(
    authorityProjectRoot,
    "case_p0_contract",
  );
  const authorityEnv = { ANALYTIX_CASE_PROJECT_ROOT: authorityProjectRoot };
  const authorityFor = (name, args = {}, overrides = {}) =>
    hostFactRuntimeContext({
      ...authorityBinding,
      caseId: "case_p0_contract",
      toolName: name,
      args,
      serverVersion: "p0-test",
      overrides,
    });
  const providerResourceList = resourceListForAgent();
  assert.equal(providerResourceList.length, resourceDefinitions.length);
  assert.equal(
    new Set(resourceDefinitions.map((item) => item.uri)).size,
    resourceDefinitions.length,
    "resource URIs must be unique",
  );
  const developmentOnlyResourceSuffixes = [
    "/plugin-benchmark.md",
    "/mature-plugin-drift-table.md",
    "/top-pluginization-plan.md",
    "/blueprint-implementation-matrix.md",
    "/casegraph-roadmap.md",
  ];
  for (const suffix of developmentOnlyResourceSuffixes) {
    assert.equal(
      providerResourceList.some((item) => item.uri.endsWith(suffix)),
      false,
      `${suffix} must not be provider-readable`,
    );
  }
  for (const definition of resourceDefinitions) {
    const listedResource = providerResourceList.find(
      (item) => item.uri === definition.uri,
    );
    assert(
      listedResource,
      `${definition.uri} must be reachable through resources/list`,
    );
    assert.equal(
      Object.prototype.hasOwnProperty.call(listedResource, "relativePath"),
      false,
      `${definition.uri} must not expose its local path in resources/list`,
    );
    assertCaseNeutralProviderResource(
      listedResource,
      `${definition.uri} resources/list`,
    );

    const readResult = readResourceForAgent(PLUGIN_ROOT, definition.uri);
    assert.equal(
      readResult.contents.length,
      1,
      `${definition.uri} must have exactly one provider-readable content item`,
    );
    assert.equal(readResult.contents[0].uri, definition.uri);
    assert.equal(readResult.contents[0].mimeType, definition.mimeType);
    assert.equal(typeof readResult.contents[0].text, "string");
    assertCaseNeutralProviderResource(
      readResult,
      `${definition.uri} resources/read`,
    );
    assert.equal(
      readResult.contents[0].text.includes(
        "run_full_case_analysis(write_report=false)",
      ),
      false,
      `${definition.uri} must not instruct the provider to invoke the P0-hidden full-case tool`,
    );
    for (const suffix of developmentOnlyResourceSuffixes) {
      const fileName = suffix.slice(1);
      assert.equal(
        readResult.contents[0].text.includes(fileName),
        false,
        `${definition.uri} must not route the provider to development-only ${fileName}`,
      );
    }
  }

  const providerBoundary = createHandler({ env: authorityEnv });
  const initializeParams = (protocolVersion) => ({
    protocolVersion,
    capabilities: {},
    clientInfo: { name: "p0-contract", version: "1.0.0" },
  });
  const preferredInitialize = await providerBoundary.handleRequest({
    method: "initialize",
    params: initializeParams("2025-11-25"),
  });
  await providerBoundary.handleNotification({
    method: "notifications/initialized",
    params: {},
  });
  const compatibleBoundary = createHandler({ env: authorityEnv });
  const compatibleInitialize = await compatibleBoundary.handleRequest({
    method: "initialize",
    params: initializeParams("2025-06-18"),
  });
  const legacyBoundary = createHandler({ env: authorityEnv });
  const legacyInitialize = await legacyBoundary.handleRequest({
    method: "initialize",
    params: initializeParams("2024-11-05"),
  });
  assert.equal(
    preferredInitialize.protocolVersion,
    "2025-11-25",
    "FundsNegotiatesSupportedMCPVersion preferred mismatch",
  );
  assert.equal(
    compatibleInitialize.protocolVersion,
    "2025-06-18",
    "FundsNegotiatesSupportedMCPVersion compatible mismatch",
  );
  assert.equal(
    legacyInitialize.protocolVersion,
    "2025-11-25",
    "legacy client must be offered a fact-capable revision, never a false 2024 contract",
  );
  await expectMcpError(
    createHandler({ env: authorityEnv }).handleRequest({
      method: "initialize",
      params: { protocolVersion: "2025-11-25" },
    }),
    -32602,
  );
  const mcpResourceList = await providerBoundary.handleRequest({
    method: "resources/list",
  });
  assert.deepEqual(
    mcpResourceList.resources.map((item) => item.uri),
    [FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI],
    "production quarantine must expose only the fixed boundary resource",
  );
  assertCaseNeutralProviderResource(mcpResourceList, "MCP resources/list");
  const mcpReadResult = await providerBoundary.handleRequest({
    method: "resources/read",
    params: { uri: FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI },
  });
  assert.equal(mcpReadResult.contents[0].uri, FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI);
  assertCaseNeutralProviderResource(mcpReadResult, "P0 boundary MCP resources/read");
  const promptList = await providerBoundary.handleRequest({
    method: "prompts/list",
  });
  assert(
    promptList.prompts.some(
      (item) => item.name === "prepare-investigation-report",
    ),
    "P0 report boundary prompt must remain discoverable",
  );
  await expectMcpError(providerBoundary.handleRequest({
    method: "prompts/get",
    params: {
      name: "prepare-investigation-report",
      arguments: { question: "请生成正式报告" },
    },
  }), -32602);
  const reportPrompt = await providerBoundary.handleRequest({
    method: "prompts/get",
    params: { name: "prepare-investigation-report", arguments: {} },
  });
  const reportPromptText = reportPrompt.messages?.[0]?.content?.text || "";
  assert(reportPromptText.includes("当前不查询案件数据"));
  assert(reportPromptText.includes("不生成分析草稿、正式报告、附件或文件"));
  assert(reportPromptText.includes("同案证据核验与人工复核"));
  assert.equal(
    reportPromptText.includes("组织最终报告思路"),
    false,
    "P0 prompt must not instruct report drafting",
  );
  assert.equal(
    providerBoundary.calls(),
    0,
    "resource/prompt reads must not execute a case tool",
  );

  const listed = await providerBoundary.handleRequest({ method: "tools/list" });
  const listedNames = listed.tools.map((tool) => tool.name);
  const expectedFundsCountTool = fundsCountToolV2Contract();
  const expectedFundsAccountFlowTool = fundsAccountFlowToolV1Contract();
  assert.deepEqual(
    listed.tools,
    [expectedFundsCountTool, expectedFundsAccountFlowTool],
    "production must advertise exactly the count canary and host-captured account-flow tool",
  );
  assertClosedExternalSchema(
    listed.tools[0].inputSchema,
    "count_case_rows.inputSchema",
  );
  assertClosedExternalSchema(
    listed.tools[0].outputSchema,
    "count_case_rows.outputSchema",
  );
  assertClosedExternalSchema(
    listed.tools[1].inputSchema,
    "analyze_account_flows.inputSchema",
  );
  assertClosedExternalSchema(
    listed.tools[1].outputSchema,
    "analyze_account_flows.outputSchema",
  );
  for (const blockedName of PRE_EXECUTION_BLOCKED_TOOL_NAMES) {
    assert.equal(
      listedNames.includes(blockedName),
      false,
      `${blockedName} must not be advertised`,
    );
  }

  const skillSurfaceCases = [
    {
      label: "strict empty success",
      request: async () => ({ data: { items: [] } }),
      expected: {
        version: "SkillSurfaceStateV1",
        state: "available",
        count: 0,
        items: [],
      },
    },
    {
      label: "missing success payload",
      request: async () => undefined,
      expected: {
        version: "SkillSurfaceStateV1",
        state: "invalid",
        blocker_code: "skills_response_schema_invalid",
      },
    },
    {
      label: "malformed success payload",
      request: async () => ({ data: { items: [] }, extra: true }),
      expected: {
        version: "SkillSurfaceStateV1",
        state: "invalid",
        blocker_code: "skills_response_schema_invalid",
      },
    },
    {
      label: "non-2xx response",
      request: async () => {
        const error = new Error("private HTTP response body");
        error.status = 503;
        throw error;
      },
      expected: unavailableSkillsSurfaceState(),
    },
    {
      label: "transport exception",
      request: async () => {
        throw new Error("private transport exception");
      },
      expected: unavailableSkillsSurfaceState(),
    },
  ];
  for (const skillSurfaceCase of skillSurfaceCases) {
    const statusRuntime = toolCallRuntime({
      compactSkillsRecord,
      httpJson: async (pathname) => pathname.startsWith("/skills?")
        ? skillSurfaceCase.request()
        : { data: { case_id: "case-test" } },
    });
    const status = await statusRuntime.callTool("get_case_status", {});
    assert.deepEqual(status.skills, skillSurfaceCase.expected, skillSurfaceCase.label);
    if (status.skills.state !== "available") {
      assert.equal(
        Object.hasOwn(status.skills, "count") || Object.hasOwn(status.skills, "items"),
        false,
        `${skillSurfaceCase.label} must not impersonate an available empty inventory`,
      );
    }
    assert.doesNotMatch(
      JSON.stringify(status.skills),
      /private HTTP response body|private transport exception/u,
      `${skillSurfaceCase.label} must not reflect transport details`,
    );
  }

  const operationalInstructionChecks = [
    [
      "skills/full-case-analysis/SKILL.md",
      ["P0 隔离期不得调用或模拟", "只返回：当前全案事实发布能力不可用"],
    ],
    [
      "skills/full-case-analysis/agents/openai.yaml",
      ["P0 隔离期", "不得输出研判结论"],
    ],
    [
      "skills/report-builder/SKILL.md",
      ["Active P0 Containment Gate", "不得生成报告草稿"],
    ],
    [
      "skills/report-builder/agents/openai.yaml",
      ["当前报告链处于 P0 隔离期", "不生成报告草稿"],
    ],
    [
      "skills/case-workbench/SKILL.md",
      ["P0 containment", "hidden and rejected before execution"],
    ],
    ["references/anti-patterns.md", ["P0 full-case/report bypass"]],
  ];
  for (const [relativePath, markers] of operationalInstructionChecks) {
    const body = fs.readFileSync(path.join(PLUGIN_ROOT, relativePath), "utf8");
    for (const marker of markers) {
      assert(
        body.includes(marker),
        `${relativePath} is missing P0 operational marker: ${marker}`,
      );
    }
    assert.equal(
      body.includes("run_full_case_analysis(write_report=false)"),
      false,
      `${relativePath} must not instruct the hidden full-case dry run`,
    );
  }

  const agentUiBody = fs.readFileSync(
    path.join(PLUGIN_ROOT, "scripts/agent-ui-ab-run.mjs"),
    "utf8",
  );
  assert.equal(
    agentUiBody.includes("固定为 run_full_case_analysis"),
    false,
    "agent UI evaluation must not require the P0-hidden full-case tool",
  );
  assert.equal(
    agentUiBody.includes("run_full_case_analysis(write_report=false)"),
    false,
    "agent UI evaluation must not prescribe a hidden full-case dry run",
  );
  const goldenQaBody = fs.readFileSync(
    path.join(PLUGIN_ROOT, "scripts/golden-qa.mjs"),
    "utf8",
  );
  assert.equal(
    goldenQaBody.includes('executeSkill(options, "run_full_case_analysis"'),
    false,
    "golden QA must not execute the P0-hidden full-case tool",
  );
  assert.equal(
    goldenQaBody.includes("run_full_case_analysis(write_report=false)"),
    false,
    "golden QA must not prescribe a hidden full-case dry run",
  );

  const goldenFixture = JSON.parse(
    fs.readFileSync(
      path.join(
        PLUGIN_ROOT,
        "scripts/eval-fixtures/golden-answer-set.json",
      ),
      "utf8",
    ),
  );
  const fixtureTasks = new Map(
    goldenFixture.tasks.map((task) => [task.id, task]),
  );
  const preExecutionBoundaryTaskIds = [
    "full_report_gate",
    "replay_report_materialize",
    "replay_case_delivery_pack",
    "full_case_analysis_tree",
    "report_builder_generation",
  ];
  for (const taskId of preExecutionBoundaryTaskIds) {
    const task = fixtureTasks.get(taskId);
    assert(task, `missing P0 boundary fixture ${taskId}`);
    assert.match(
      String(task.standard_logic || ""),
      /P0 containment/iu,
      `${taskId} must assert host-enforced P0 containment`,
    );
    assert.deepEqual(
      task.expected_tools,
      [],
      `${taskId} must not expect any tool before the capability boundary`,
    );
  }
  const notebookFixture = fixtureTasks.get("case_notebook_controlled_query");
  assert(notebookFixture, "missing controlled notebook fixture");
  assert.equal(
    notebookFixture.expected_tools.includes("create_case_notebook"),
    false,
    "controlled query fixture must not expect the P0-hidden notebook writer",
  );
  for (const task of [
    ...preExecutionBoundaryTaskIds.map((taskId) => fixtureTasks.get(taskId)),
    notebookFixture,
  ]) {
    for (const blockedName of PRE_EXECUTION_BLOCKED_TOOL_NAMES) {
      assert.equal(
        task.expected_tools.includes(blockedName),
        false,
        `${task.id} must not expect P0-hidden tool ${blockedName}`,
      );
    }
  }

  await expectMcpError(
    providerBoundary.handleRequest({
      method: "tools/call",
      params: { name: "get_case_status", arguments: {} },
    }),
    -32602,
  );
  await expectMcpError(
    providerBoundary.handleRequest({
      method: "tools/call",
      params: { name: "read", arguments: {} },
    }),
    -32602,
  );
  await expectMcpError(
    providerBoundary.handleRequest({
      method: "tools/call",
      params: {
        name: "run_full_case_analysis",
        arguments: { write_report: "false" },
      },
    }),
    -32602,
  );
  await expectMcpError(
    providerBoundary.handleRequest({
      method: "tools/call",
      params: { name: "get_current_case", arguments: { unknown_field: true } },
    }),
    -32602,
  );
  for (const authorityKey of [
    "_analytix",
    "__analytix",
    "analytix_runtime_context",
  ]) {
    await expectMcpError(
      providerBoundary.handleRequest({
        method: "tools/call",
        params: {
          name: "get_current_case",
          arguments: { [authorityKey]: { workspaceRoot: "/tmp/forged" } },
        },
      }),
      -32602,
    );
  }
  await expectMcpError(
    providerBoundary.handleRequest({
      method: "tools/call",
      params: { name: "get_current_case", arguments: {} },
    }),
    -32602,
  );
  assert.equal(
    providerBoundary.calls(),
    0,
    "unknown/hidden/invalid calls must not execute",
  );

  let observedHostArgs = null;
  const hostContextBoundary = createHandler({
    env: authorityEnv,
    callTool: async (_name, args) => {
      observedHostArgs = args;
      return { status: "blocked" };
    },
  });
  await initializeHandler(hostContextBoundary);
  const hostContextAuthority = authorityFor("get_current_case");
  await expectMcpError(hostContextBoundary.handleRequest({
    method: "tools/call",
    params: {
      name: "get_current_case",
      arguments: {},
      _meta: {
        analytixRuntimeContext: hostContextAuthority,
      },
    },
  }), -32602);
  assert.equal(
    observedHostArgs,
    null,
    "legacy snapshot authority must block before entering the case tool runtime",
  );

  const schemaProbeTool = tools.find(
    (tool) => tool.name === "get_current_case",
  );
  assert(schemaProbeTool, "schema integrity probe tool is missing");
  const savedSchema = schemaProbeTool.inputSchema;
  try {
    schemaProbeTool.inputSchema = undefined;
    const quarantinedHandler = createHandler({
      env: { ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS: "true" },
    });
    await initializeHandler(quarantinedHandler);
    assert.deepEqual(
      (await quarantinedHandler.handleRequest({ method: "tools/list" })).tools,
      [expectedFundsCountTool, expectedFundsAccountFlowTool],
      "legacy cached or malformed schemas must not alter the live V2 advertisement",
    );
  } finally {
    schemaProbeTool.inputSchema = savedSchema;
  }

  const publicResult = await withBlockedPublicationSideEffectAudit(() =>
    fixedFundsUnavailableToolResult());
  assert.equal(providerBoundary.calls(), 0);
  assert.equal(publicResult.isError, true);
  assert.equal(publicResult.structuredContent.semanticStatus, "unavailable");
  assert.equal(
    publicResult.structuredContent.blocker,
    "dataset_snapshot_authority_unavailable",
  );
  assert.equal(publicResult.structuredContent.safeToAnswer, false);
  assert.deepEqual(publicResult.structuredContent.data, {});
  assert.deepEqual(
    publicResult.structuredContent.candidateEvidenceReceipts,
    [],
  );
  assert.equal(
    publicResult._meta.analytix_evidence_ledger.host_registry_verified,
    false,
  );
  assertNoRawSentinel(publicResult, "public ToolOutcome");

  const fundsProjection = fundsCountProjectionV2();
  assertFundsCountProjectionV2IsPathAndPiiFree(
    fundsProjection,
    "valid host funds-count projection",
  );
  const forgedProjection = {
    ...fundsProjection,
    rowCount: "38",
  };
  const mismatchedProjection = fundsCountProjectionV2({
    datasetSnapshotId: `dsv2_${fixtureDigest("p0-contract:mismatched-snapshot")}`,
  });
  const invalidProjectionMetaCases = [
    ["missing projection", undefined],
    [
      "legacy runtime context",
      {
        analytixRuntimeContext: authorityFor(
          "count_case_rows",
          { table_name: FUNDS_COUNT_TABLE_V2 },
        ),
      },
    ],
    [
      "V2 projection mixed with legacy runtime context",
      {
        [FUNDS_COUNT_PROJECTION_META_KEY_V2]: fundsProjection,
        analytixRuntimeContext: authorityFor(
          "count_case_rows",
          { table_name: FUNDS_COUNT_TABLE_V2 },
        ),
      },
    ],
    [
      "forged projection digest",
      { [FUNDS_COUNT_PROJECTION_META_KEY_V2]: forgedProjection },
    ],
    [
      "mismatched snapshot projection",
      { [FUNDS_COUNT_PROJECTION_META_KEY_V2]: mismatchedProjection },
    ],
  ];
  const invalidProjectionRequests = [
    [
      "tools/call",
      (meta) => ({
        method: "tools/call",
        params: {
          name: "count_case_rows",
          arguments: { table_name: FUNDS_COUNT_TABLE_V2 },
          ...(meta === undefined ? {} : { _meta: meta }),
        },
      }),
    ],
    [
      "analytix/sourceProbe",
      (meta) => ({
        method: "analytix/sourceProbe",
        params: meta === undefined ? {} : { _meta: meta },
      }),
    ],
    [
      "analytix/evidenceRead",
      (meta) => ({
        method: "analytix/evidenceRead",
        params: {
          tool: "count_case_rows",
          tableName: FUNDS_COUNT_TABLE_V2,
          noFilter: true,
          ...(meta === undefined ? {} : { _meta: meta }),
        },
      }),
    ],
  ];
  for (const [metaLabel, meta] of invalidProjectionMetaCases) {
    for (const [methodLabel, request] of invalidProjectionRequests) {
      await expectMcpError(
        withBlockedPublicationSideEffectAudit(() =>
          providerBoundary.handleRequest(request(meta))),
        -32602,
      );
      assert.equal(
        providerBoundary.calls(),
        0,
        `${methodLabel} ${metaLabel} must fail before any plugin data source executes`,
      );
    }
  }

  const fundsProjectionMeta = {
    [FUNDS_COUNT_PROJECTION_META_KEY_V2]: fundsProjection,
  };
  const countToolResult = await withBlockedPublicationSideEffectAudit(() =>
    providerBoundary.handleRequest({
      method: "tools/call",
      params: {
        name: "count_case_rows",
        arguments: { table_name: FUNDS_COUNT_TABLE_V2 },
        _meta: fundsProjectionMeta,
      },
    }));
  assert.deepEqual(countToolResult, {
    content: [],
    structuredContent: {
      schemaVersion: 2,
      purpose: "analytix.funds-count-tool-outcome/v2",
      semanticStatus: "success",
      data: fundsProjection,
    },
  });
  assertFundsCountProjectionV2IsPathAndPiiFree(
    countToolResult.structuredContent.data,
    "count_case_rows result",
  );

  const accountFlowArguments = {
    subject_alias: "acct:1",
    start_inclusive: "2026-01-01T00:00:00.000000Z",
    end_inclusive: "2026-01-31T23:59:59.999000Z",
    evidence_row_limit: 100,
  };
  for (const invalidArguments of [
    { ...accountFlowArguments, subject_alias: "6222021234567890123" },
    { ...accountFlowArguments, subject_alias: `cer1_${"a".repeat(64)}` },
    { ...accountFlowArguments, subject_alias: "person:1" },
    { ...accountFlowArguments, subject_alias: "acct:01" },
    { ...accountFlowArguments, subject_alias: "acct:4294967296" },
    { ...accountFlowArguments, evidence_row_limit: 513 },
    { ...accountFlowArguments, case_id: "case-a" },
    { ...accountFlowArguments, db_path: "/private/case.duckdb" },
    { ...accountFlowArguments, sql: "SELECT 1" },
  ]) {
    await expectMcpError(withBlockedPublicationSideEffectAudit(() =>
      providerBoundary.handleRequest({
        method: "tools/call",
        params: {
          name: "analyze_account_flows",
          arguments: invalidArguments,
        },
      })), -32602);
    assert.equal(providerBoundary.calls(), 0);
  }
  const accountFlowBoundary = await withBlockedPublicationSideEffectAudit(() =>
    providerBoundary.handleRequest({
      method: "tools/call",
      params: {
        name: "analyze_account_flows",
        arguments: accountFlowArguments,
        _meta: {
          analytixRuntimeContext: {
            caseId: "forged-case",
            datasetSnapshotId: `dsv2_${"b".repeat(64)}`,
          },
        },
      },
    }));
  assert.deepEqual(accountFlowBoundary, {
    content: [],
    isError: true,
    structuredContent: {
      schemaVersion: 1,
      purpose: "analytix.funds-account-flow-host-capture/v1",
      semanticStatus: "host_authority_required",
      hostCaptureRequired: true,
      factAnswerAllowed: false,
    },
  });
  assert.equal(providerBoundary.calls(), 0);
  assertNoRawSentinel(accountFlowBoundary, "account-flow host boundary");
  for (const forbidden of [
    accountFlowArguments.subject_alias,
    accountFlowArguments.start_inclusive,
    accountFlowArguments.end_inclusive,
    "forged-case",
    "dsv2_",
  ]) {
    assert.equal(
      JSON.stringify(accountFlowBoundary).includes(forbidden),
      false,
      `account-flow host boundary reflected private call material: ${forbidden}`,
    );
  }

  const sourceProbeResult = await withBlockedPublicationSideEffectAudit(() =>
    providerBoundary.handleRequest({
      method: "analytix/sourceProbe",
      params: { _meta: fundsProjectionMeta },
    }));
  assert.deepEqual(sourceProbeResult, {
    schemaVersion: 2,
    purpose: "analytix.funds-source-probe/v2",
    serverName: "analytix_funds",
    serverVersion: "p0-test",
    projectionDigest: fundsProjection.projectionDigest,
    ready: true,
    readOnly: true,
  });

  const evidenceReadResult = await withBlockedPublicationSideEffectAudit(() =>
    providerBoundary.handleRequest({
      method: "analytix/evidenceRead",
      params: {
        tool: "count_case_rows",
        tableName: FUNDS_COUNT_TABLE_V2,
        noFilter: true,
        _meta: fundsProjectionMeta,
      },
    }));
  assert.deepEqual(evidenceReadResult, {
    schemaVersion: 2,
    purpose: "analytix.funds.count-case-rows-evidence-candidate/v2",
    serverName: "analytix_funds",
    serverVersion: "p0-test",
    toolName: "count_case_rows",
    projection: fundsProjection,
    paginationComplete: true,
    readOnly: true,
  });
  assert.deepEqual(
    evidenceReadResult.projection,
    countToolResult.structuredContent.data,
    "tool and evidenceRead must bind the exact same immutable host projection",
  );
  assert.equal(
    sourceProbeResult.projectionDigest,
    evidenceReadResult.projection.projectionDigest,
    "sourceProbe and evidenceRead must bind the exact same immutable host projection",
  );
  assert.equal(
    providerBoundary.calls(),
    0,
    "valid V2 projection reads must never execute a plugin-owned data source",
  );
  assert.equal(
    JSON.stringify({ countToolResult, sourceProbeResult, evidenceReadResult })
      .includes("analytixRuntimeContext"),
    false,
    "V2 results must not restore the legacy analytixRuntimeContext carrier",
  );

  for (const [code, marker] of [
    ["database_unavailable", "当前案件数据源不可用"],
    ["dataset_snapshot_mismatch", "旧快照结果已被拒绝"],
  ]) {
    const diagnosticResult = outcomeRuntime().toolResult({
      status: "failed",
      duckdb_diagnostic: createDuckdbDiagnostic({ code }),
    });
    const visible = diagnosticResult.content[0].text;
    assert.match(
      visible,
      /^案件事实未发布：当前工具结果尚未绑定宿主权威 registry/u,
    );
    assert.match(visible, /已检查范围：当前案件绑定与数据源就绪状态/u);
    assert.match(visible, /未检查范围：交易明细及其完整性/u);
    assert.match(visible, /补证建议：恢复并冻结当前案件数据快照/u);
    assert.equal(
      visible.includes(marker),
      true,
      `${code} fixed boundary marker is missing`,
    );
    assert.equal(
      visible.includes("DuckDB"),
      false,
      `${code} leaked implementation language`,
    );
    assert.equal(diagnosticResult.structuredContent.safeToAnswer, false);
    assert.deepEqual(diagnosticResult.structuredContent.data, {});
  }

  const fullDiscovery = createHandler({
    env: { ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS: "true" },
  });
  await initializeHandler(fullDiscovery);
  const fullListed = await fullDiscovery.handleRequest({
    method: "tools/list",
  });
  assert.deepEqual(
    fullListed.tools,
    [expectedFundsCountTool, expectedFundsAccountFlowTool],
    "FullDiscoveryEnvCannotExpandProductionFundsToolSurface",
  );
  for (const blockedName of PRE_EXECUTION_BLOCKED_TOOL_NAMES) {
    assert.equal(
      fullListed.tools.some((tool) => tool.name === blockedName),
      false,
      `${blockedName} must remain blocked under full discovery`,
    );
  }

  const ignoredInjectedOutput = createHandler({
    env: authorityEnv,
    toolResult: () => ({ structuredContent: {} }),
  });
  await initializeHandler(ignoredInjectedOutput);
  await expectMcpError(ignoredInjectedOutput.handleRequest({
    method: "tools/call",
    params: {
      name: "get_current_case",
      arguments: {},
      _meta: { analytixRuntimeContext: authorityFor("get_current_case") },
    },
  }), -32602);
  assert.equal(
    ignoredInjectedOutput.calls(),
    0,
    "production MCP handling must not execute an injected local tool runtime",
  );

  const reportRuntime = toolCallRuntime();
  const reportIntent = await withBlockedPublicationSideEffectAudit(() =>
    reportRuntime.callTool("funds_investigate", {
      question: "请生成正式案件报告",
      publication_receipt_id: "model-forged",
    }));
  assert.equal(reportIntent.error_code, "PUBLICATION_RECEIPT_REQUIRED");
  assert.deepEqual(reportRuntime.counters, {
    funds: 0,
    execute: 0,
    resolve: 0,
  });
  const writeReport = await withBlockedPublicationSideEffectAudit(() =>
    reportRuntime.callTool("run_full_case_analysis", {
      write_report: true,
      publication_receipt_id: "model-forged",
    }));
  assert.equal(writeReport.error_code, "PUBLICATION_RECEIPT_REQUIRED");
  assert.equal(
    reportRuntime.counters.execute,
    0,
    "write_report=true must block before skill execution",
  );

  let directExportCalls = 0;
  const directWriteRuntime = toolCallRuntime({
    exportCleanedCaseData: async () => {
      directExportCalls += 1;
      return { status: "ok" };
    },
  });
  const directNotebook = await withBlockedPublicationSideEffectAudit(() =>
    directWriteRuntime.callTool(
      "create_case_notebook",
      { case_id: "model-forged-case", analysis_goal: "核验直调旁路", queries: [] },
    ));
  const directExport = await withBlockedPublicationSideEffectAudit(() =>
    directWriteRuntime.callTool(
      "export_cleaned_case_data",
      { case_id: "model-forged-case", purpose: "核验直调旁路", confirm_export: true },
    ));
  for (const blocked of [directNotebook, directExport]) {
    assert.equal(blocked.error_code, "P0_PRE_EXECUTION_BLOCKED");
    assert.equal(blocked.isError, true);
    assert.equal(blocked.safeToAnswer, false);
    assert.deepEqual(blocked.data, {});
    assert.equal(
      Object.prototype.hasOwnProperty.call(blocked, "case_id"),
      false,
      "blocked direct call must not echo a provider-supplied case identity",
    );
  }
  assert.equal(
    directWriteRuntime.counters.execute,
    0,
    "create_case_notebook direct call must block before skill execution",
  );
  assert.equal(
    directExportCalls,
    0,
    "export_cleaned_case_data direct call must block before export execution",
  );

  const pluginManifest = JSON.parse(
    fs.readFileSync(path.join(PLUGIN_ROOT, ".codex-plugin", "plugin.json"), "utf8"),
  );
  assert.equal(
    pluginManifest.interface.capabilities.includes("Write"),
    false,
    "P0-contained plugin must not request a package-level Write capability",
  );
  const pluginMcpConfig = JSON.parse(
    fs.readFileSync(path.join(PLUGIN_ROOT, ".mcp.json"), "utf8"),
  );
  assert.equal(
    pluginMcpConfig.mcpServers.analytix_funds.disabled,
    true,
    "P0-contained funds MCP must remain disabled until the Go host owns executable and dataset authority",
  );
  assert.equal(
    pluginMcpConfig.mcpServers.analytix_funds.default_tools_approval_mode,
    "prompt",
    "P0-contained plugin tools must default to explicit approval",
  );

  const tempRoot = fs.mkdtempSync(path.join(authorityTempRoot, "write-report-"));
  try {
    const before = fs.readdirSync(tempRoot);
    const analysisRuntime = toolCallRuntime({
      env: {
        HOME: tempRoot,
        ANALYTIX_FUNDS_ARTIFACT_DIR: path.join(tempRoot, "artifacts"),
        ANALYTIX_FUNDS_DISABLE_LOCAL_DUCKDB_FALLBACK: "1",
      },
    });
    const dryRunBlocked = await withBlockedPublicationSideEffectAudit(() =>
      analysisRuntime.callTool(
        "run_full_case_analysis",
        { write_report: false },
      ));
    assert.equal(dryRunBlocked.error_code, "PUBLICATION_RECEIPT_REQUIRED");
    assert.equal(
      analysisRuntime.counters.execute,
      0,
      "write_report=false must block before backend execution during P0",
    );
    assert.deepEqual(
      fs.readdirSync(tempRoot),
      before,
      "write_report=false must not create a file in the controlled roots",
    );

    let failedBackendCalls = 0;
    const failedReportRuntime = toolCallRuntime({
      executeSkill: async () => {
        failedBackendCalls += 1;
        throw new Error("fetch failed");
      },
      env: {
        HOME: tempRoot,
        ANALYTIX_FUNDS_ARTIFACT_DIR: path.join(tempRoot, "artifacts"),
        ANALYTIX_FUNDS_DISABLE_LOCAL_DUCKDB_FALLBACK: "0",
      },
    });
    const failedReport = await withBlockedPublicationSideEffectAudit(() =>
      failedReportRuntime.callTool(
        "run_full_case_analysis",
        { write_report: false },
      ));
    assert.equal(failedBackendCalls, 0);
    assert.equal(
      failedReport.error_code,
      "PUBLICATION_RECEIPT_REQUIRED",
      "report analysis must block before backend or local fallback",
    );
    assert.deepEqual(
      fs.readdirSync(tempRoot),
      before,
      "report backend failure must not create a fallback report",
    );

    const failedReview = await withBlockedPublicationSideEffectAudit(() =>
      failedReportRuntime.callTool(
        "validate_report_claims",
        {
          case_id: "case-test",
          report_text: "金额 100 元",
          facts: { amount: 100 },
          claims: [
            { claim_id: "claim-1", text: "金额 100 元", fact_refs: ["amount"] },
          ],
        },
      ));
    assert.equal(
      failedBackendCalls,
      1,
      "claim review failure fixture should be the only backend call",
    );
    assert.equal(failedReview.status, "blocked");
    assert.equal(failedReview.blocker, "report_claim_validation_unavailable");
    assert.deepEqual(failedReview.answer_card.verified_claims, []);
    assert.equal(failedReview.answer_card.write_blocked, true);
    assert.equal(
      JSON.stringify(failedReview).includes("本地确定性 claim review fallback"),
      false,
    );
  } finally {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }

  const noCacheRuntime = toolCallRuntime();
  await noCacheRuntime.callTool("funds_investigate", {
    question: "核验主体甲",
  });
  await noCacheRuntime.callTool("funds_investigate", {
    question: "核验主体乙",
  });
  await noCacheRuntime.callTool("funds_investigate", {
    question: "核验主体甲",
  });
  assert.equal(
    noCacheRuntime.counters.funds,
    3,
    "A/B/A must execute live three times",
  );

  const reportBody = {
    tool: "run_full_case_analysis",
    report: {
      content_md: `# 江苏航案件\n账号 ${RAW_SENTINELS[0]}，金额 ${RAW_SENTINELS[5]} 元`,
    },
  };
  assert.equal(
    requiresPublicationBlock(reportBody),
    true,
    "report body must require PublicationReceipt",
  );

  const pii = {
    account_no: RAW_SENTINELS[0],
    id_no: RAW_SENTINELS[1],
    phone: RAW_SENTINELS[2],
    mac_addr: RAW_SENTINELS[3],
    ip_addr: RAW_SENTINELS[4],
  };
  assertNoRawSentinel(redactAgentPayload(pii), "PII projection");
  const accountAliasSentinel = "1234567890";
  const ibanSentinel = "DE89370400440532013000";
  const accountAliases = {
    bank_account: accountAliasSentinel,
    bankAccount: accountAliasSentinel,
    payer_account: accountAliasSentinel,
    receiver_account: accountAliasSentinel,
    from_account: accountAliasSentinel,
    to_account: accountAliasSentinel,
    counterparty_account: accountAliasSentinel,
    accounts: [accountAliasSentinel, ibanSentinel],
    source_accounts: [{ value: accountAliasSentinel, iban: ibanSentinel }],
  };
  const redactedAccountAliases = JSON.stringify(
    redactAgentPayload(accountAliases),
  );
  for (const forbidden of [accountAliasSentinel, ibanSentinel]) {
    assert.equal(
      redactedAccountAliases.includes(forbidden),
      false,
      `account field alias leaked ${forbidden}`,
    );
  }
  const freeTextPii =
    "姓名 张三，账号 1234567890，手机号 138 1234 5678，座机 010-12345678，住址 北京市朝阳区示例路88号。";
  const redactedFreeTextPii = redactAgentPayload(freeTextPii);
  for (const forbidden of [
    "张三",
    "1234567890",
    "138 1234 5678",
    "010-12345678",
    "北京市朝阳区示例路88号",
  ]) {
    assert.equal(
      redactedFreeTextPii.includes(forbidden),
      false,
      `free-text PII leaked ${forbidden}`,
    );
  }
  const ordinaryPiiCorpus = JSON.parse(fs.readFileSync(
    path.join(REPO_ROOT, "packages", "runtime", "src", "conformance", "fixtures", "ordinary-pii-projection-v1.json"),
    "utf8",
  ));
  assert.equal(ordinaryPiiCorpus.schemaVersion, 1, "ordinary PII corpus schema version");
  assert.equal(ordinaryPiiCorpus.syntheticOnly, true, "ordinary PII corpus must remain synthetic-only");
  const canonicalDigits = (value) => String(value || "").normalize("NFKC").replace(/[^0-9]/gu, "");
  for (const testCase of ordinaryPiiCorpus.untrustedTextCases) {
    const projected = redactAgentPayload(testCase.input);
    if (testCase.expected === "preserve") {
      assert.equal(projected, testCase.input, `${testCase.id} must preserve a safe measure`);
      continue;
    }
    assert.notEqual(projected, testCase.input, `${testCase.id} must be projected`);
    if (testCase.sensitiveCanonical) {
      assert.equal(
        canonicalDigits(projected).includes(testCase.sensitiveCanonical),
        false,
        `${testCase.id} retained the complete account digits`,
      );
    }
  }
  for (const testCase of ordinaryPiiCorpus.typedFieldCases) {
    const projected = redactAgentPayload({ [testCase.fieldName]: testCase.value })[testCase.fieldName];
    if (testCase.expected === "preserve") {
      assert.equal(projected, testCase.value, `${testCase.id} must preserve a trusted measure field`);
      continue;
    }
    assert.notEqual(projected, testCase.value, `${testCase.id} must project a typed account field`);
    if (testCase.sensitiveCanonical) {
      assert.equal(
        canonicalDigits(projected).includes(testCase.sensitiveCanonical),
        false,
        `${testCase.id} retained the complete typed account digits`,
      );
    }
  }
  const workbenchRecipeSource = fs.readFileSync(
    path.join(PLUGIN_ROOT, "mcp", "duckdb-workbench-runtime.mjs"),
    "utf8",
  );
  const oracleSource = fs.readFileSync(
    path.join(PLUGIN_ROOT, "scripts", "mcp-return-oracle.mjs"),
    "utf8",
  );
  for (const requiredBoundary of [
    "case_db_binding_mismatch",
    "case_db_identity_invalid",
    "dataset_snapshot_manifest_v2_unavailable",
    "--summary-json",
    "--no-write",
  ]) {
    assert.equal(
      oracleSource.includes(requiredBoundary),
      true,
      `oracle must retain ${requiredBoundary} boundary`,
    );
  }
  assert.equal(
    oracleSource.includes("&& /^dsv2_[a-f0-9]{64}$/u.test(datasetSnapshotId)"),
    true,
    "oracle must require a factual DatasetSnapshotManifestV2 before business-data checks",
  );
  for (const [name, source] of [["workbench", workbenchRecipeSource], ["oracle", oracleSource]]) {
    assert.equal(
      source.includes("project_or_business"),
      false,
      `${name} must not upgrade a transaction keyword into a project/business fact`,
    );
    assert.equal(
      source.includes("transaction_text_keyword_hit"),
      true,
      `${name} must retain only a literal keyword-hit lead`,
    );
  }
  const reasoningSentinels = [
    "private-reasoning-content",
    "private-thinking-content",
    "private-analysis-content",
    "private-tagged-thought",
  ];
  const reasoningProjection = redactAgentPayload({
    visible: "safe",
    reasoning_content: reasoningSentinels[0],
    thinking: reasoningSentinels[1],
    analysis: reasoningSentinels[2],
    nested: { reasoningContent: reasoningSentinels[0], safe: "visible" },
    text: `public <think>${reasoningSentinels[3]}</think> boundary`,
  });
  const reasoningProjectionText = JSON.stringify(reasoningProjection);
  for (const forbidden of reasoningSentinels) {
    assert.equal(reasoningProjectionText.includes(forbidden), false, `reasoning projection leaked ${forbidden}`);
  }
  const safeReasoningRuntime = createSafeSkillRuntime({
    executeSkill: async () => ({
      status: "ok",
      reasoning_content: reasoningSentinels[0],
      data: { thinking: reasoningSentinels[1], visible: "safe" },
      warnings: [{ code: "SYNTHETIC", message: `<analysis>${reasoningSentinels[2]}</analysis>` }],
    }),
  });
  const safeReasoningResult = await safeReasoningRuntime.safeSkill("synthetic_reasoning_probe", {});
  const safeReasoningText = JSON.stringify(safeReasoningResult);
  for (const forbidden of reasoningSentinels) {
    assert.equal(safeReasoningText.includes(forbidden), false, `safe skill runtime leaked ${forbidden}`);
  }
  const fixedErrorRuntime = createSafeSkillRuntime({
    executeSkill: async () => { throw new Error("private-upstream-error-reasoning"); },
  });
  const fixedErrorResult = await fixedErrorRuntime.safeSkill("synthetic_failure_probe", {});
  assert.equal(
    JSON.stringify(fixedErrorResult).includes("private-upstream-error-reasoning"),
    false,
    "safe skill runtime leaked an upstream error payload",
  );
  let deep = pii;
  for (let index = 0; index < 16; index += 1) deep = { nested: deep };
  assertNoRawSentinel(redactAgentPayload(deep), "deep PII projection");
  const compiler = createAgentOutputCompiler({
    env: { ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD: "true" },
  });
  assertNoRawSentinel(
    compiler.compactToolText({ status: "ok", ...pii }, { include_debug: true }),
    "debug text",
  );
  assertNoRawSentinel(
    compiler.compactStructuredContent(
      { status: "ok", ...pii },
      { include_debug: true },
    ),
    "debug structured content",
  );

  const missingSummary = counterpartySummaryFromRank(
    {
      data: { rankings: [{ display_name: "乙方" }] },
    },
    "乙方",
  );
  assert.deepEqual(
    missingSummary,
    {},
    "an incomplete rank row must not create a truthy fact object that downstream code can upgrade",
  );
  assert.equal(JSON.stringify(missingSummary).includes('"amount":0'), false);
  const missingCardText = renderDiagnosticCardForAgent({
    card_type: "source_to_counterparty_amount_card",
    holder_name: "甲方",
    via_holder_name: "乙方",
    source_to_via_summary: {},
  });
  assert.equal(
    /0\.00\s*元|0\s*笔/u.test(missingCardText),
    false,
    "missing numeric fields must not render as zero",
  );
  assert(
    /未返回|缺失|不得写成 0/u.test(missingCardText),
    "missing card must be boundary-only",
  );
  const missingAmountGraph = buildFundGraphFromSeedRows({
    holderName: "甲方",
    sourceSeeds: [
      {
        rank: 1,
        seed: {
          txn_id: "txn-missing-amount",
          txn_time: "2026-07-10T00:00:00Z",
          account_key: "acct-a",
          account_open_name: "甲方",
          counterparty_key: "acct-b",
          counterparty_name: "乙方",
        },
      },
    ],
  });
  assert.equal(
    missingAmountGraph.edges[0].missing_fields.includes("amount"),
    true,
  );
  assert.equal(
    Object.prototype.hasOwnProperty.call(missingAmountGraph.edges[0], "amount"),
    false,
  );
  assert.equal(
    JSON.stringify(missingAmountGraph).includes("0.00 元"),
    false,
    "missing graph amount must not become zero",
  );

  const supportedEdgeBase = {
    edge_id: "edge-numeric-contract",
    edge_status: "supported",
    txn_id: "txn-numeric-contract",
    txn_time: "2026-07-10T00:00:00Z",
    direction: "out",
    from: "acct-a",
    from_label: "甲方",
    to: "acct-b",
    to_label: "乙方",
    source_tool: "trace_subject_top_outflows",
  };
  for (const [label, amount] of [
    ["missing", undefined],
    ["blank", "  "],
    ["invalid", "not-a-number"],
    ["boolean", false],
    ["array", []],
  ]) {
    const graph = withFundGraphProtocol({
      supported_edge_count: 99,
      edges: [{ ...supportedEdgeBase, amount }],
    });
    assert.equal(
      graph.supported_edge_count,
      0,
      `${label} amount must not remain supported`,
    );
    assert.equal(
      graph.drawable_edges.length,
      0,
      `${label} amount must not be drawable`,
    );
    assert.equal(
      graph.evidence_pack.length,
      0,
      `${label} amount must not issue evidence pack`,
    );
    assert.equal(
      graph.claim_support.length,
      0,
      `${label} amount must not issue claim support`,
    );
    assert.equal(graph.edges[0].edge_status, "needs_evidence");
    assert.equal(graph.edges[0].missing_fields.includes("amount"), true);
    assert.equal(
      Object.prototype.hasOwnProperty.call(graph.edges[0], "amount"),
      false,
    );
  }
  for (const [label, txnCount] of [
    ["blank", ""],
    ["invalid", "many"],
    ["boolean", false],
  ]) {
    const graph = withFundGraphProtocol({
      edges: [{ ...supportedEdgeBase, amount: 25, txn_count: txnCount }],
    });
    assert.equal(
      graph.supported_edge_count,
      0,
      `${label} count must not remain supported`,
    );
    assert.equal(
      graph.drawable_edges.length,
      0,
      `${label} count must not be drawable`,
    );
    assert.equal(graph.edges[0].missing_fields.includes("txn_count"), true);
    assert.equal(
      Object.prototype.hasOwnProperty.call(graph.edges[0], "txn_count"),
      false,
    );
  }
  const explicitZeroGraph = withFundGraphProtocol({
    edges: [{ ...supportedEdgeBase, amount: 0, txn_count: 0 }],
  });
  assert.equal(
    explicitZeroGraph.edges[0].amount,
    0,
    "explicit zero amount must be preserved",
  );
  assert.equal(
    explicitZeroGraph.edges[0].txn_count,
    0,
    "explicit zero count must be preserved",
  );
  assert.equal(
    explicitZeroGraph.supported_edge_count,
    1,
    "explicit zero must remain distinct from missing numeric data",
  );

  const numericFlowRuntime = createFundFlowGraphRuntime({
    resolveCase: async () => ({ case_id: "case-numeric" }),
    executeSkill: async (_skillId, payload) =>
      payload.holder_name === "甲方"
        ? {
            data: {
              scope_stats: {
                txn_count: "",
                account_count: "invalid",
                counterparty_count: 0,
                out_amount: " ",
              },
              top_outflows: [
                {
                  rank: 1,
                  txn_count: "",
                  seed_txn: {
                    txn_id: "txn-source-missing",
                    txn_time: "2026-07-10T00:00:00Z",
                    account_key: "acct-a",
                    account_open_name: "甲方",
                    counterparty_key: "acct-b",
                    counterparty_name: "乙方",
                  },
                },
              ],
            },
          }
        : {
            data: {
              scope_stats: {
                txn_count: 0,
                account_count: 0,
                counterparty_count: 0,
                out_amount: 0,
              },
              top_outflows: [
                {
                  rank: 1,
                  txn_count: "",
                  seed_txn: {
                    txn_id: "txn-downstream-missing",
                    txn_time: "2026-07-10T01:00:00Z",
                    account_key: "acct-b",
                    account_open_name: "乙方",
                    counterparty_key: "acct-c",
                    counterparty_name: "丙方",
                  },
                },
                {
                  rank: 2,
                  txn_count: 0,
                  seed_txn: {
                    txn_id: "txn-downstream-zero",
                    txn_time: "2026-07-10T02:00:00Z",
                    account_key: "acct-b",
                    account_open_name: "乙方",
                    counterparty_key: "acct-d",
                    counterparty_name: "丁方",
                    amount: 0,
                  },
                },
                {
                  rank: 3,
                  txn_count: "invalid",
                  seed_txn: {
                    txn_id: "txn-downstream-invalid",
                    txn_time: "2026-07-10T03:00:00Z",
                    account_key: "acct-b",
                    account_open_name: "乙方",
                    counterparty_key: "acct-e",
                    counterparty_name: "戊方",
                    amount: "invalid",
                  },
                },
              ],
            },
          },
  });
  const numericFlow = await numericFlowRuntime.buildFundFlowGraph({
    holder_name: "甲方",
    via_holder_name: "乙方",
  });
  assert.equal(numericFlow.flow_graph.supported_edge_count, 0);
  assert.equal(numericFlow.flow_graph.drawable_edges.length, 0);
  assert.equal(numericFlow.answer_card.required_facts_present, false);
  assert.equal(numericFlow.answer_card.fact_answer_allowed, false);
  const numericRows =
    numericFlow.key_facts.fund_flow_fact_pack.downstream_outflows;
  const missingNumericRow = numericRows.find(
    (row) => row.counterparty_name === "丙方",
  );
  assert.equal(
    Object.prototype.hasOwnProperty.call(missingNumericRow, "amount"),
    false,
  );
  assert.equal(
    Object.prototype.hasOwnProperty.call(missingNumericRow, "txn_count"),
    false,
  );
  assert.deepEqual(missingNumericRow.missing_fields, ["amount", "txn_count"]);
  assert.equal(missingNumericRow.fact_status, "unresolved");
  const explicitZeroRow = numericRows.find(
    (row) => row.counterparty_name === "丁方",
  );
  assert.equal(explicitZeroRow.amount.yuan, 0);
  assert.equal(explicitZeroRow.txn_count, 0);
  assert.equal(explicitZeroRow.fact_status, "unverified");
  const invalidNumericRow = numericRows.find(
    (row) => row.counterparty_name === "戊方",
  );
  assert.equal(
    Object.prototype.hasOwnProperty.call(invalidNumericRow, "amount"),
    false,
  );
  assert.equal(
    Object.prototype.hasOwnProperty.call(invalidNumericRow, "txn_count"),
    false,
  );
  assert.deepEqual(invalidNumericRow.missing_fields, ["amount", "txn_count"]);
  assert.deepEqual(numericFlow.key_facts.source_scope_stats, {
    counterparty_count: 0,
  });
  assert.deepEqual(numericFlow.key_facts.via_scope_stats, {
    txn_count: 0,
    account_count: 0,
    counterparty_count: 0,
    amount: 0,
  });
  const destinationRuntime = createDestinationDiagnosticRuntime({
    moneyText: (value) => `${Number(value).toFixed(2)} 元`,
  });
  const missingDestinationCard =
    destinationRuntime.buildDestinationOutflowDiagnosticCard({
      caseId: "case-test",
      holderName: "甲方",
      traceResult: { data: { scope_stats: {} } },
      counterpartyRankResult: {
        data: { rankings: [{ display_name: "乙方" }] },
      },
    });
  const missingDestinationBody = JSON.stringify(missingDestinationCard);
  assert.equal(
    /0\.00 元|"amount":0|"count":0|"txn_count":0/u.test(missingDestinationBody),
    false,
  );
  assert.equal(
    missingDestinationCard.facts.some(
      (fact) => fact.support_status === "unsupported"
        && fact.fact_answer_allowed === false
        && fact.publication_readiness === "blocked",
    ),
    true,
  );

  const ledger = buildEvidenceLedger({
    facts: [
      {
        fact_id: "fake",
        amount: 1,
        support_status: "supported",
        evidence_ids: ["fake"],
      },
    ],
  });
  assert(ledger.entries.length > 0);
  assert.match(
    ledger.ledger_id,
    /^evidence-ledger:[a-f0-9]{64}$/u,
    "Evidence Ledger identity must use a complete SHA-256",
  );
  assert(
    ledger.entries.every((entry) =>
      /^el-[a-f0-9]{64}$/u.test(String(entry.entry_id || "")),
    ),
    "Evidence Ledger entry identity must use a complete SHA-256",
  );
  assert(
    ledger.entries.every((entry) => entry.support_status !== "supported"),
    "Evidence Ledger must default to unsupported",
  );
  const perClaimLedger = buildEvidenceLedger({
    evidence_refs: {
      evidence_ids: ["parent-evidence-must-not-propagate"],
      query_ids: ["parent-query-must-not-propagate"],
    },
    facts: [
      {
        fact_id: "claim-with-exact-evidence",
        amount: 1,
        source_refs: {
          evidence_ids: ["evidence-for-claim-one"],
          query_ids: ["query-for-claim-one"],
        },
      },
      {
        fact_id: "claim-without-evidence",
        amount: 2,
      },
    ],
  });
  const exactClaimEntry = perClaimLedger.entries.find(
    (entry) => entry.fact_id === "claim-with-exact-evidence",
  );
  const unsupportedSiblingEntry = perClaimLedger.entries.find(
    (entry) => entry.fact_id === "claim-without-evidence",
  );
  assert.deepEqual(
    exactClaimEntry?.source_refs,
    {
      query_ids: ["query-for-claim-one"],
      evidence_ids: ["evidence-for-claim-one"],
    },
    "EvidenceLedgerDoesNotInheritParentCitationAcrossClaims: an exact claim may keep only its own citation set",
  );
  assert.deepEqual(
    unsupportedSiblingEntry?.source_refs || {},
    {},
    "EvidenceLedgerDoesNotInheritParentCitationAcrossClaims: a sibling without exact citations must remain unbound",
  );
  const answerCard = withAnswerCardProtocol({
    facts: [{ support_status: "supported" }],
  });
  assert.equal(answerCard.answer_card_complete, true);
  assert.equal(answerCard.fact_answer_allowed, false);
  assert.equal(answerCard.required_facts_present, false);
  const claimCard = withClaimReviewProtocol({
    verified_claims: [
      { claim_id: "c1", text: "金额 1 元", evidence_ids: ["fake"] },
    ],
  });
  assert.equal(claimCard.claim_support_index[0].support_status, "unsupported");

  const casegraphSource = fs.readFileSync(
    path.join(PLUGIN_ROOT, "mcp", "casegraph-runtime.mjs"),
    "utf8",
  );
  const flowSource = fs.readFileSync(
    path.join(PLUGIN_ROOT, "mcp", "fund-flow-graph-runtime.mjs"),
    "utf8",
  );
  assert.equal(
    casegraphSource.includes("detailRefForPayload"),
    false,
    "casegraph must not write implicit detail artifacts",
  );
  assert.equal(
    flowSource.includes("detailRefForPayload"),
    false,
    "fund flow must not write implicit detail artifacts",
  );
  assert.equal(
    fs.existsSync(path.join(PLUGIN_ROOT, "mcp", "mcp-artifact-store.mjs")),
    false,
    "production MCP must not retain the unbound auto-write artifact store",
  );
  const frontdoorSource = `${fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", "frontdoor-routing.mjs"), "utf8")}\n${fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", "frontdoor-runtime.mjs"), "utf8")}`;
  assert.equal(
    /\bactive\b.{0,40}(?:cache|key)|(?:cache|key).{0,40}\bactive\b/iu.test(
      frontdoorSource,
    ),
    false,
    "active must not be a factual cache key",
  );
  assert.equal(
    /responseCache|reuseCached|rememberFrontdoor|duplicateSuppression/u.test(
      frontdoorSource,
    ),
    false,
    "frontdoor factual response cache must not exist",
  );
  const productionMcpSource = fs
    .readdirSync(path.join(PLUGIN_ROOT, "mcp"))
    .filter((name) => name.endsWith(".mjs"))
    .map((name) => fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", name), "utf8"))
    .join("\n");
  assert.equal(
    /persistMcpArtifact|detailRefForPayload|stored_in_local_audit_artifact_not_model_context/u.test(
      productionMcpSource,
    ),
    false,
    "production MCP must not expose the retired unbound artifact writer",
  );
  assert.equal(
    productionMcpSource.includes("江苏航案件"),
    false,
    "production report code must not contain a hard-coded case name",
  );
  const forbiddenMaterializationAlias = ["txn", "daily", "active"].join("_");
  const productionRoots = [
    path.join(REPO_ROOT, "backend", "app"),
    path.join(REPO_ROOT, "tools", "analysis_compute", "src"),
    path.join(PLUGIN_ROOT, "mcp"),
  ];
  for (const productionRoot of productionRoots) {
    for (const sourceFile of productionSourceFiles(productionRoot)) {
      const source = fs.readFileSync(sourceFile, "utf8");
      assert.equal(
        source.includes(forbiddenMaterializationAlias),
        false,
        `${path.relative(REPO_ROOT, sourceFile)} retained a mutable factual materialization alias`,
      );
    }
  }

  const environmentProbe = path.join(
    authorityTempRoot,
    "duckdb-python-environment-probe.mjs",
  );
  fs.writeFileSync(
    environmentProbe,
    `#!/usr/bin/env node
const record = {
  "analytix_api_token": process.env.ANALYTIX_API_TOKEN ?? null,
  "openai_api_key": process.env.OPENAI_API_KEY ?? null,
  "anthropic_api_key": process.env.ANTHROPIC_API_KEY ?? null,
  "deepseek_api_key": process.env.DEEPSEEK_API_KEY ?? null,
  "provider_api_key": process.env.ANALYTIX_PROVIDER_API_KEY ?? null,
  python_no_user_site: process.env.PYTHONNOUSERSITE ?? null,
  python_utf8: process.env.PYTHONUTF8 ?? null
};
process.stdout.write(JSON.stringify({
  columns: Object.keys(record),
  records: [record],
  row_count: 1,
  truncated: false,
  snapshot_contract: "",
  snapshot_factual_ready: false,
  snapshot_blocker: "",
  snapshot_manifest_schema_version: 0,
  observed_dataset_snapshot_id: ""
}));
`,
    { mode: 0o755 },
  );
  const hostileParentEnv = {
    ...process.env,
    ANALYTIX_FUNDS_DUCKDB_PYTHON: environmentProbe,
    ANALYTIX_BACKEND_PYTHON: environmentProbe,
    PYTHON: environmentProbe,
    ANALYTIX_API_TOKEN: "<redacted>",
    OPENAI_API_KEY: "<redacted>",
    ANTHROPIC_API_KEY: "<redacted>",
    DEEPSEEK_API_KEY: "<redacted>",
    ANALYTIX_PROVIDER_API_KEY: "<redacted>",
  };
  const childEnv = duckdbPythonChildEnv(hostileParentEnv);
  for (const forbiddenName of [
    "ANALYTIX_API_TOKEN",
    "OPENAI_API_KEY",
    "ANTHROPIC_API_KEY",
    "DEEPSEEK_API_KEY",
    "ANALYTIX_PROVIDER_API_KEY",
    "ANALYTIX_FUNDS_DUCKDB_PYTHON",
    "PYTHON",
  ]) {
    assert.equal(
      Object.prototype.hasOwnProperty.call(childEnv, forbiddenName),
      false,
      `DuckDB Python child env inherited ${forbiddenName}`,
    );
  }
  const environmentResult = await runPythonDuckdb(
    { sql: "SELECT 1", row_limit: 1 },
    hostileParentEnv,
  );
  assert.deepEqual(environmentResult.records, [
    {
      analytix_api_token: null,
      openai_api_key: null,
      anthropic_api_key: null,
      deepseek_api_key: null,
      provider_api_key: null,
      python_no_user_site: "1",
      python_utf8: "1",
    },
  ]);

  return {
    status: "ok",
    checks: [
      "production advertises exactly count_case_rows and analyze_account_flows with closed schemas while every legacy fact tool remains undiscoverable",
      "missing, legacy, forged, mismatched, and mixed host projections fail closed before plugin data execution",
      "tool, sourceProbe, and evidenceRead bind the same path-free and PII-free immutable host projection",
      "SkillSurfaceStateV1 distinguishes strict available-empty, invalid payload, and unavailable transport without fabricating skill inventory",
      "FundsNegotiatesSupportedMCPVersion and rejects malformed initialize params",
      "resources/list and resources/read expose only case-neutral P0 publication guidance",
      "unknown, hidden, invalid, and invalid-output paths",
      "public ToolOutcome contains no facts, PII, or candidate receipts",
      "write tools blocked before execution",
      "report intent and body require PublicationReceipt",
      "Darwin kernel sandbox plus pre-import guards, async-resource ledger, and full-hash inventory prove audited publication-blocked paths have zero filesystem, process, fd, FileHandle, or deferred side effects",
      "A/B/A executes live without factual response cache",
      "PII projection including debug and deep nesting",
      "missing numeric fields remain unresolved",
      "fund graph amount/count never default or self-upgrade",
      "Evidence/Claim/Answer defaults remain unsupported",
      "all production layers contain no mutable txn-daily factual alias",
      "DuckDB Python child receives only the minimum environment allowlist",
      "casegraph and flow create no implicit detail artifacts",
    ],
  };
}

async function main() {
  if (process.platform !== "darwin") {
    throw new Error("p0_side_effect_kernel_sandbox_unavailable");
  }
  if (process.env[DARWIN_SANDBOX_CHILD_ENV] !== "1") {
    return runDarwinSandboxController();
  }
  const authorityTempRoot = canonicalSandboxRootFromEnvironment();
  await installPermanentSideEffectAudit(authorityTempRoot);
  await loadProductionModulesAfterAuditInstallation();
  return run(authorityTempRoot);
}

main()
  .then((result) =>
    result?.delegated
      ? undefined
      : process.stdout.write(`${JSON.stringify(result, null, 2)}\n`),
  )
  .catch((error) => {
    process.stderr.write(`${error?.stack || error}\n`);
    process.exitCode = 1;
  });
