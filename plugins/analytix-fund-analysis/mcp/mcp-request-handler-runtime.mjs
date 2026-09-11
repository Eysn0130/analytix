import nodeCrypto from "node:crypto";
import { text } from "./runtime-normalizers.mjs";
import {
  PRE_EXECUTION_BLOCKED_TOOL_NAMES
} from "./pre-execution-tool-policy.mjs";
import {
  FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE,
  fixedFundsBoundaryPrompt,
  fixedFundsBoundaryPrompts,
  fixedFundsBoundaryResourceRead,
  fixedFundsBoundaryResources
} from "./source-unavailable-boundary.mjs";
import { schemaValidationFailures } from "./tool-schema-validation.mjs";

export { PRE_EXECUTION_BLOCKED_TOOL_NAMES } from "./pre-execution-tool-policy.mjs";
export {
  FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE,
  FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY
} from "./source-unavailable-boundary.mjs";

export const MCP_REQUEST_HANDLER_RUNTIME_VERSION = "0.16.16";
export const MCP_PREFERRED_PROTOCOL_VERSION = "2025-11-25";
export const MCP_SUPPORTED_PROTOCOL_VERSIONS = Object.freeze(["2025-11-25", "2025-06-18"]);

export const CASE_SOURCE_PROBE_METHOD = "analytix/sourceProbe";
export const FUNDS_EVIDENCE_READ_METHOD = "analytix/evidenceRead";
const PROVIDER_AUTHORITY_KEYS = new Set([
  "_analytix",
  "__analytix",
  "analytix_runtime_context"
]);

const SHA256_PATTERN = "^[a-f0-9]{64}$";
const DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE = FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE;
const FUNDS_COUNT_PROJECTION_META_KEY_V2 = "analytixFundsCountProjectionV2";
const FUNDS_COUNT_PROJECTION_PURPOSE_V2 = "analytix.host.funds-count-projection/v2";
const FUNDS_COUNT_PROJECTION_DIGEST_DOMAIN_V2 = Buffer.from(
  "analytix.host.funds-count-projection/digest/v2\0",
  "utf8"
);
const FUNDS_COUNT_TABLE_V2 = "analysis_txn_detail_idx";
const FUNDS_COUNT_TOOL_OUTCOME_PURPOSE_V2 = "analytix.funds-count-tool-outcome/v2";
const FUNDS_SOURCE_PROBE_PURPOSE_V2 = "analytix.funds-source-probe/v2";
const FUNDS_EVIDENCE_CANDIDATE_PURPOSE_V2 = "analytix.funds.count-case-rows-evidence-candidate/v2";
const FUNDS_ACCOUNT_FLOW_TOOL_NAME_V1 = "analyze_account_flows";
const FUNDS_ACCOUNT_FLOW_HOST_CAPTURE_PURPOSE_V1 = "analytix.funds-account-flow-host-capture/v1";
const FUNDS_ACCOUNT_FLOW_HOST_AUTHORITY_REQUIRED_V1 = "host_authority_required";
const FUNDS_ACCOUNT_FLOW_TIMESTAMP_PATTERN_V1 = "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$";

export const FUNDS_COUNT_PROJECTION_V2_SCHEMA = Object.freeze({
  type: "object",
  properties: {
    schemaVersion: { type: "integer", const: 2 },
    purpose: { type: "string", const: FUNDS_COUNT_PROJECTION_PURPOSE_V2 },
    turnSecurityContextDigest: { type: "string", pattern: SHA256_PATTERN },
    datasetSnapshotId: { type: "string", pattern: "^dsv2_[a-f0-9]{64}$" },
    datasetSelectionDigest: { type: "string", pattern: SHA256_PATTERN },
    datasetRecordDigest: { type: "string", pattern: SHA256_PATTERN },
    datasetManifestDigest: { type: "string", pattern: SHA256_PATTERN },
    fundsProducerContentId: { type: "string", pattern: "^fpc[12]_[a-f0-9]{64}$" },
    fundsProducerContentManifestSha256: { type: "string", pattern: SHA256_PATTERN },
    detailContentSha256: { type: "string", pattern: SHA256_PATTERN },
    tableName: { type: "string", const: FUNDS_COUNT_TABLE_V2 },
    rowCount: { type: "string", pattern: "^(0|[1-9][0-9]*)$", maxLength: 20 },
    projectionDigest: { type: "string", pattern: SHA256_PATTERN }
  },
  required: [
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
    "projectionDigest"
  ],
  additionalProperties: false
});

const FUNDS_COUNT_TOOL_INPUT_SCHEMA_V2 = Object.freeze({
  type: "object",
  properties: {
    table_name: { type: "string", const: FUNDS_COUNT_TABLE_V2 }
  },
  required: ["table_name"],
  additionalProperties: false
});

export const FUNDS_COUNT_TOOL_OUTCOME_SCHEMA_V2 = Object.freeze({
  type: "object",
  properties: {
    schemaVersion: { type: "integer", const: 2 },
    purpose: { type: "string", const: FUNDS_COUNT_TOOL_OUTCOME_PURPOSE_V2 },
    semanticStatus: { type: "string", const: "success" },
    data: FUNDS_COUNT_PROJECTION_V2_SCHEMA
  },
  required: ["schemaVersion", "purpose", "semanticStatus", "data"],
  additionalProperties: false
});

export const FUNDS_ACCOUNT_FLOW_TOOL_INPUT_SCHEMA_V1 = Object.freeze({
  type: "object",
  properties: {
    subject_alias: {
      type: "string",
      pattern: "^(acct|card):(?:[1-9][0-9]{0,8}|[1-3][0-9]{9}|4[01][0-9]{8}|42[0-8][0-9]{7}|429[0-3][0-9]{6}|4294[0-8][0-9]{5}|42949[0-5][0-9]{4}|429496[0-6][0-9]{3}|4294967[0-1][0-9]{2}|42949672[0-8][0-9]|429496729[0-5])$",
      minLength: 6,
      maxLength: 15
    },
    start_inclusive: {
      type: "string",
      pattern: FUNDS_ACCOUNT_FLOW_TIMESTAMP_PATTERN_V1,
      minLength: 27,
      maxLength: 27
    },
    end_inclusive: {
      type: "string",
      pattern: FUNDS_ACCOUNT_FLOW_TIMESTAMP_PATTERN_V1,
      minLength: 27,
      maxLength: 27
    },
    evidence_row_limit: { type: "integer", minimum: 1, maximum: 512 }
  },
  required: ["subject_alias", "start_inclusive", "end_inclusive", "evidence_row_limit"],
  additionalProperties: false
});

export const FUNDS_ACCOUNT_FLOW_HOST_CAPTURE_OUTPUT_SCHEMA_V1 = Object.freeze({
  type: "object",
  properties: {
    schemaVersion: { type: "integer", const: 1 },
    purpose: { type: "string", const: FUNDS_ACCOUNT_FLOW_HOST_CAPTURE_PURPOSE_V1 },
    semanticStatus: { type: "string", const: FUNDS_ACCOUNT_FLOW_HOST_AUTHORITY_REQUIRED_V1 },
    hostCaptureRequired: { type: "boolean", const: true },
    factAnswerAllowed: { type: "boolean", const: false }
  },
  required: [
    "schemaVersion",
    "purpose",
    "semanticStatus",
    "hostCaptureRequired",
    "factAnswerAllowed"
  ],
  additionalProperties: false
});

export class McpRequestError extends Error {
  constructor(code, message, payload) {
    super(message);
    this.name = "McpRequestError";
    this.code = code;
    this.payload = payload;
  }
}

export const MCP_TOOL_OUTCOME_OUTPUT_SCHEMA = FUNDS_COUNT_TOOL_OUTCOME_SCHEMA_V2;

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function requireOnlyKeys(value, allowedKeys, label) {
  const source = value === undefined ? {} : value;
  if (!source || typeof source !== "object" || Array.isArray(source)) {
    throw new McpRequestError(-32602, `${label} params must be an object`);
  }
  const allowed = new Set(allowedKeys);
  const unknown = Object.keys(source).find((key) => !allowed.has(key));
  if (unknown) throw new McpRequestError(-32602, `${label} does not allow param ${unknown}`);
  return source;
}

export function validateToolInput(tool, args = {}) {
  const name = tool.name;
  const failures = schemaValidationFailures(args, tool.inputSchema);
  if (failures.length) {
    throw new McpRequestError(-32602, `Invalid tool input for ${name}: ${failures.join("; ")}`);
  }
  return args;
}

export function validateToolOutput(tool, structuredContent) {
  const name = tool.name;
  const failures = schemaValidationFailures(structuredContent, tool.outputSchema);
  if (failures.length) {
    throw new McpRequestError(-32603, `Tool output failed schema validation for ${name}`, {
      failures: failures.slice(0, 20)
    });
  }
  return structuredContent;
}

function goJsonString(value) {
  const encoded = JSON.stringify(value);
  if (encoded === undefined) return undefined;
  return encoded.replace(/[<>&\u2028\u2029]/gu, (character) => ({
    "<": "\\u003c",
    ">": "\\u003e",
    "&": "\\u0026",
    "\u2028": "\\u2028",
    "\u2029": "\\u2029"
  })[character]);
}

function canonicalJson(value) {
  if (value === null) return "null";
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(",")}]`;
  if (typeof value === "object") {
    return `{${Object.keys(value).sort().map((key) => `${goJsonString(key)}:${canonicalJson(value[key])}`).join(",")}}`;
  }
  const encoded = goJsonString(value);
  if (encoded === undefined) throw new McpRequestError(-32602, "Tool arguments contain a non-JSON value");
  return encoded;
}

function fundsCountProjectionDigestV2(projection) {
  const digestRecord = { ...projection, projectionDigest: "" };
  return nodeCrypto.createHash("sha256")
    .update(FUNDS_COUNT_PROJECTION_DIGEST_DOMAIN_V2)
    .update(canonicalJson(digestRecord), "utf8")
    .digest("hex");
}

function trustedFundsCountProjectionV2(params, label) {
  const meta = params?._meta;
  if (!meta || typeof meta !== "object" || Array.isArray(meta)
    || Object.keys(meta).length !== 1
    || !Object.hasOwn(meta, FUNDS_COUNT_PROJECTION_META_KEY_V2)) {
    throw new McpRequestError(-32602, `${label} requires the exact host funds-count projection`);
  }
  const projection = meta[FUNDS_COUNT_PROJECTION_META_KEY_V2];
  const failures = schemaValidationFailures(
    projection,
    FUNDS_COUNT_PROJECTION_V2_SCHEMA,
    `$._meta.${FUNDS_COUNT_PROJECTION_META_KEY_V2}`
  );
  if (failures.length) {
    throw new McpRequestError(-32602, `Invalid host funds-count projection: ${failures.join("; ")}`);
  }
  let count;
  try {
    count = BigInt(projection.rowCount);
  } catch {
    throw new McpRequestError(-32602, "Host funds-count projection count is invalid");
  }
  if (count < 0n || count > 18446744073709551615n
    || String(count) !== projection.rowCount
    || projection.datasetSnapshotId !== `dsv2_${projection.datasetManifestDigest}`
    || projection.projectionDigest !== fundsCountProjectionDigestV2(projection)) {
    throw new McpRequestError(-32602, "Host funds-count projection integrity is invalid");
  }
  return { ...projection };
}

function negotiatedInitializeVersion(message) {
  const params = message?.params;
  if (!params || typeof params !== "object" || Array.isArray(params)) {
    throw new McpRequestError(-32602, "MCP initialize params must be an object");
  }
  const allowedKeys = new Set(["protocolVersion", "capabilities", "clientInfo", "_meta"]);
  if (Object.keys(params).some((key) => !allowedKeys.has(key))
    || !params.capabilities || typeof params.capabilities !== "object" || Array.isArray(params.capabilities)
    || !params.clientInfo || typeof params.clientInfo !== "object" || Array.isArray(params.clientInfo)
    || Object.keys(params.clientInfo).some((key) => !new Set(["name", "version", "title", "description", "websiteUrl", "icons"]).has(key))
    || !text(params.clientInfo.name) || !text(params.clientInfo.version)) {
    throw new McpRequestError(-32602, "MCP initialize params are incomplete or contain unknown fields");
  }
  const requested = text(params.protocolVersion);
  return MCP_SUPPORTED_PROTOCOL_VERSIONS.includes(requested) ? requested : MCP_PREFERRED_PROTOCOL_VERSION;
}

export function caseSourceProbeResult(payload, runtimeContext, serverName, serverVersion) {
  const source = objectOf(payload);
  const expectedCaseId = text(runtimeContext?.caseId);
  const expectedSnapshotId = text(runtimeContext?.datasetSnapshotId);
  const observedCaseId = text(source.case_id || source.caseId);
  const observedSnapshotId = text(source.observed_dataset_snapshot_id || source.observedDatasetSnapshotId);
  const caseMatches = Boolean(expectedCaseId && observedCaseId === expectedCaseId);
  let blocker = DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE;
  if (!caseMatches) blocker = "case_identity_mismatch";
  else if (observedSnapshotId && expectedSnapshotId && observedSnapshotId !== expectedSnapshotId) blocker = "dataset_snapshot_mismatch";
  return {
    version: 1,
    serverName: text(serverName),
    serverVersion: text(serverVersion),
    caseId: expectedCaseId,
    caseBindingHash: text(runtimeContext?.caseBindingHash),
    datasetSnapshotId: "",
    ready: false,
    readOnly: true,
    blocker,
    checkedAt: new Date().toISOString()
  };
}

export function fundsCountEvidenceCandidate(payload, runtimeContext, serverName, serverVersion) {
  void payload;
  void runtimeContext;
  void serverName;
  void serverVersion;
  throw new McpRequestError(
    -32603,
    "Funds evidence read is blocked until the host has verified a registry-backed DatasetSnapshotManifestV2 authority"
  );
}

function fundsSourceProbeResultV2(projection, serverName, serverVersion) {
  return {
    schemaVersion: 2,
    purpose: FUNDS_SOURCE_PROBE_PURPOSE_V2,
    serverName: text(serverName),
    serverVersion: text(serverVersion),
    projectionDigest: projection.projectionDigest,
    ready: true,
    readOnly: true
  };
}

function fundsCountToolResultV2(projection) {
  return {
    content: [],
    structuredContent: {
      schemaVersion: 2,
      purpose: FUNDS_COUNT_TOOL_OUTCOME_PURPOSE_V2,
      semanticStatus: "success",
      data: { ...projection }
    }
  };
}

function fundsCountEvidenceCandidateV2(projection, serverName, serverVersion) {
  return {
    schemaVersion: 2,
    purpose: FUNDS_EVIDENCE_CANDIDATE_PURPOSE_V2,
    serverName: text(serverName),
    serverVersion: text(serverVersion),
    toolName: "count_case_rows",
    projection: { ...projection },
    paginationComplete: true,
    readOnly: true
  };
}

function fundsAccountFlowHostCaptureRequiredV1() {
  return {
    content: [],
    isError: true,
    structuredContent: {
      schemaVersion: 1,
      purpose: FUNDS_ACCOUNT_FLOW_HOST_CAPTURE_PURPOSE_V1,
      semanticStatus: FUNDS_ACCOUNT_FLOW_HOST_AUTHORITY_REQUIRED_V1,
      hostCaptureRequired: true,
      factAnswerAllowed: false
    }
  };
}

export function createMcpRequestHandlerRuntime({
  serverName,
  serverVersion
}) {
  const advertisedTools = [{
    name: "count_case_rows",
    title: "当前不可变资金明细计数",
    description: "Returns only the host-projected row count for the exact immutable DSV2/FPC selection. The result is evidence material and never grants publication authority.",
    annotations: {
      title: "当前不可变资金明细计数",
      readOnlyHint: true,
      destructiveHint: false,
      idempotentHint: true,
      openWorldHint: false
    },
    execution: { taskSupport: "forbidden" },
    inputSchema: FUNDS_COUNT_TOOL_INPUT_SCHEMA_V2,
    outputSchema: FUNDS_COUNT_TOOL_OUTCOME_SCHEMA_V2
  }, {
    name: FUNDS_ACCOUNT_FLOW_TOOL_NAME_V1,
    title: "账户资金流入流出分析",
    description: "Requests a bounded account-flow analysis for one host-resolved case-scoped account or card alias and inclusive time range. The Go host captures the call and owns all snapshot, identity, query, evidence, claim, and publication effects.",
    annotations: {
      title: "账户资金流入流出分析",
      readOnlyHint: true,
      destructiveHint: false,
      idempotentHint: true,
      openWorldHint: false
    },
    execution: { taskSupport: "forbidden" },
    inputSchema: FUNDS_ACCOUNT_FLOW_TOOL_INPUT_SCHEMA_V1,
    outputSchema: FUNDS_ACCOUNT_FLOW_HOST_CAPTURE_OUTPUT_SCHEMA_V1
  }];
  const advertisedByName = new Map(advertisedTools.map((tool) => [tool.name, tool]));

  let lifecyclePhase = "pre_initialize";
  let negotiatedProtocolVersion = "";

  function requireOperational(method) {
    if (lifecyclePhase !== "operational") {
      throw new McpRequestError(-32600, `MCP lifecycle is not operational for ${method || "request"}`);
    }
  }

  function abortIfRequested(signal) {
    if (signal?.aborted) {
      throw new McpRequestError(-32800, "Request cancelled");
    }
  }

  async function handleRequest(message, { signal, requestId } = {}) {
    const method = text(message.method);
    if (lifecyclePhase === "closed") {
      throw new McpRequestError(-32600, "MCP connection is closed");
    }
    if (method === "ping") {
      requireOnlyKeys(message.params, [], "ping");
      return {};
    }
    if (method === "initialize") {
      if (lifecyclePhase !== "pre_initialize") {
        throw new McpRequestError(-32600, "MCP initialize may occur only once per connection");
      }
      negotiatedProtocolVersion = negotiatedInitializeVersion(message);
      lifecyclePhase = "awaiting_initialized";
      return {
        protocolVersion: negotiatedProtocolVersion,
        capabilities: {
          tools: {},
          resources: {},
          prompts: {},
          experimental: {
            analytixEvidenceAuthority: {
              version: 2,
              methods: [CASE_SOURCE_PROBE_METHOD, FUNDS_EVIDENCE_READ_METHOD]
            }
          }
        },
        serverInfo: {
          name: serverName,
          version: serverVersion
        }
      };
    }
    requireOperational(method);
    abortIfRequested(signal);
    if (method === "tools/list") {
      requireOnlyKeys(message.params, [], "tools/list");
      return { tools: advertisedTools };
    }
    if (method === "resources/list") {
      requireOnlyKeys(message.params, [], "resources/list");
      return { resources: fixedFundsBoundaryResources() };
    }
    if (method === "resources/read") {
      const params = requireOnlyKeys(message.params, ["uri"], "resources/read");
      const result = fixedFundsBoundaryResourceRead(text(params.uri));
      if (!result) throw new McpRequestError(-32602, "Unknown P0 boundary resource");
      return result;
    }
    if (method === "resources/templates/list") {
      requireOnlyKeys(message.params, [], "resources/templates/list");
      return { resourceTemplates: [] };
    }
    if (method === "prompts/list") {
      requireOnlyKeys(message.params, [], "prompts/list");
      return { prompts: fixedFundsBoundaryPrompts() };
    }
    if (method === "prompts/get") {
      const params = requireOnlyKeys(message.params, ["name", "arguments"], "prompts/get");
      const args = params.arguments === undefined ? {} : params.arguments;
      if (!args || typeof args !== "object" || Array.isArray(args) || Object.keys(args).length) {
        throw new McpRequestError(-32602, "P0 boundary prompt does not accept arguments");
      }
      const result = fixedFundsBoundaryPrompt(text(params.name));
      if (!result) throw new McpRequestError(-32602, "Unknown P0 boundary prompt");
      return result;
    }
    if (method === CASE_SOURCE_PROBE_METHOD) {
      const params = requireOnlyKeys(message.params, ["_meta"], "Case source probe");
      const projection = trustedFundsCountProjectionV2(params, "Case source probe");
      abortIfRequested(signal);
      return fundsSourceProbeResultV2(projection, serverName, serverVersion);
    }
    if (method === FUNDS_EVIDENCE_READ_METHOD) {
      const params = message.params || {};
      const allowedKeys = new Set(["tool", "tableName", "noFilter", "_meta"]);
      if (Object.keys(params).some((key) => !allowedKeys.has(key))
        || text(params.tool) !== "count_case_rows"
        || text(params.tableName) !== "analysis_txn_detail_idx"
        || params.noFilter !== true) {
        throw new McpRequestError(-32602, "Funds evidence read requires the exact unfiltered transaction-index count contract");
      }
      const projection = trustedFundsCountProjectionV2(params, "Funds evidence read");
      return fundsCountEvidenceCandidateV2(projection, serverName, serverVersion);
    }
    if (method === "tools/call") {
      const params = requireOnlyKeys(message.params, ["name", "arguments", "_meta"], "tools/call");
      const name = text(params.name);
      const tool = advertisedByName.get(name);
      if (!tool) throw new McpRequestError(-32602, "Unknown or unadvertised tool");
      if (params.arguments !== undefined && (!params.arguments || typeof params.arguments !== "object" || Array.isArray(params.arguments))) {
        throw new McpRequestError(-32602, `Invalid tool input for ${name}: arguments must be an object`);
      }
      const args = params.arguments || {};
      for (const key of PROVIDER_AUTHORITY_KEYS) {
        if (Object.hasOwn(args, key)) {
          throw new McpRequestError(-32602, `Invalid tool input for ${name}: provider authority key ${key} is not allowed`);
        }
      }
      const validatedArgs = validateToolInput(tool, args);
      if (name === FUNDS_ACCOUNT_FLOW_TOOL_NAME_V1) {
        const result = fundsAccountFlowHostCaptureRequiredV1();
        abortIfRequested(signal);
        validateToolOutput(tool, result.structuredContent);
        return result;
      }
      const projection = trustedFundsCountProjectionV2(params, `Fact tool ${name}`);
      if (projection.tableName !== validatedArgs.table_name) {
        throw new McpRequestError(-32602, "Fact tool table does not match the host funds-count projection");
      }
      const result = fundsCountToolResultV2(projection);
      abortIfRequested(signal);
      validateToolOutput(tool, result?.structuredContent);
      return result;
    }
    throw new McpRequestError(-32601, `Unsupported method: ${method || "(missing)"}`);
  }

  async function handleNotification(message, { cancelRequest } = {}) {
    if (lifecyclePhase === "closed") return;
    const method = text(message?.method);
    const params = message?.params === undefined ? {} : message.params;
    if (!params || typeof params !== "object" || Array.isArray(params)) return;
    if (method === "notifications/initialized") {
      if (lifecyclePhase === "awaiting_initialized" && Object.keys(params).length === 0) lifecyclePhase = "operational";
      return;
    }
    if (method === "notifications/cancelled") {
      const requestId = params.requestId;
      if (Object.keys(params).every((key) => key === "requestId" || key === "reason")
        && (typeof requestId === "string" || Number.isSafeInteger(requestId))) {
        cancelRequest?.(requestId, text(params.reason) || "client_cancelled");
      }
    }
  }

  async function close() {
    lifecyclePhase = "closed";
  }

  function sessionState() {
    return Object.freeze({ lifecyclePhase, negotiatedProtocolVersion });
  }

  return { handleRequest, handleNotification, close, sessionState };
}
