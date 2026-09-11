#!/usr/bin/env node

import assert from "node:assert/strict";

import {
  CAPABILITY_REGISTRY_RUNTIME_VERSION
} from "../mcp/capability-registry-runtime.mjs";
import {
  createMcpRequestHandlerRuntime,
  FUNDS_ACCOUNT_FLOW_HOST_CAPTURE_OUTPUT_SCHEMA_V1,
  FUNDS_ACCOUNT_FLOW_TOOL_INPUT_SCHEMA_V1,
  FUNDS_COUNT_PROJECTION_V2_SCHEMA,
  MCP_TOOL_OUTCOME_OUTPUT_SCHEMA,
  McpRequestError,
  PRE_EXECUTION_BLOCKED_TOOL_NAMES,
  validateToolInput,
  validateToolOutput
} from "../mcp/mcp-request-handler-runtime.mjs";
import { fixedFundsUnavailableOutcome } from "../mcp/source-unavailable-boundary.mjs";
import { TOOL_DISCOVERY_POLICY_VERSION } from "../mcp/tool-discovery-policy.mjs";
import { TOOL_INPUT_SCHEMAS_VERSION } from "../mcp/tool-input-schemas.mjs";
import {
  DORMANT_ANALYTICAL_OUTPUT_SCHEMA_COUNT,
  DORMANT_ANALYTICAL_OUTPUT_UNCLOSED_TOOL_NAMES,
  QUARANTINED_FUNDS_OUTPUT_CONTRACT,
  TOOL_OUTPUT_CONTRACT_BY_NAME,
  TOOL_OUTPUT_SCHEMAS_VERSION
} from "../mcp/tool-output-schemas.mjs";
import {
  assertToolCatalogSchemas,
  normalizeExternalSchema,
  schemaValidationFailures,
  TOOL_SCHEMA_VALIDATION_VERSION
} from "../mcp/tool-schema-validation.mjs";
import {
  TOOL_SCHEMA_INVENTORY,
  TOOL_SCHEMAS_VERSION,
  tools
} from "../mcp/tool-schemas.mjs";

const EXPECTED_VERSION = "0.16.16";
for (const [name, version] of Object.entries({
  CAPABILITY_REGISTRY_RUNTIME_VERSION,
  TOOL_DISCOVERY_POLICY_VERSION,
  TOOL_INPUT_SCHEMAS_VERSION,
  TOOL_OUTPUT_SCHEMAS_VERSION,
  TOOL_SCHEMA_VALIDATION_VERSION,
  TOOL_SCHEMAS_VERSION
})) {
  assert.equal(version, EXPECTED_VERSION, `${name} must use the exact plugin version`);
}

assert.equal(tools.length, 62);
assert.deepEqual(TOOL_SCHEMA_INVENTORY, {
  toolCount: 62,
  inputSchemaCount: 62,
  outputSchemaCount: 62
});
assert.deepEqual(
  assertToolCatalogSchemas(tools, TOOL_OUTPUT_CONTRACT_BY_NAME),
  TOOL_SCHEMA_INVENTORY
);
assert.equal(Object.keys(TOOL_OUTPUT_CONTRACT_BY_NAME).length, tools.length);
assert.equal(DORMANT_ANALYTICAL_OUTPUT_SCHEMA_COUNT, 0);
assert.deepEqual(
  [...DORMANT_ANALYTICAL_OUTPUT_UNCLOSED_TOOL_NAMES].sort(),
  tools.map((tool) => tool.name).sort(),
  "all legacy analytical payloads must remain explicitly unclosed and unadvertised"
);
for (const tool of tools) {
  assert.equal(
    TOOL_OUTPUT_CONTRACT_BY_NAME[tool.name],
    QUARANTINED_FUNDS_OUTPUT_CONTRACT,
    `${tool.name} must have an explicit current-handler output contract`
  );
  validateToolOutput(tool, fixedFundsUnavailableOutcome());
}

assert.throws(
  () => normalizeExternalSchema({ type: "object", properties: {}, additionalProperties: true }),
  /additionalProperties must be false/u
);
for (const [schema, expectedMessage] of [
  [{ type: "string", enum: "only" }, /enum must be a non-empty array/u],
  [{ type: "number", minimum: "100" }, /minimum must be a finite number/u],
  [{ type: "array", items: { type: "string" }, maxItems: "0" }, /maxItems must be a non-negative integer/u],
  [{ type: "string", minLength: 2, maxLength: 1 }, /minLength must not exceed maxLength/u],
  [{ type: "object", properties: undefined, additionalProperties: false }, /properties must be an object/u],
  [{ type: "object", properties: {}, additionalProperties: false, required: undefined }, /required must contain non-empty strings/u],
  [{ type: "object", properties: { id: { type: "string" } }, additionalProperties: false, required: ["id", "id"] }, /required must not contain duplicate/u],
  [{ type: "array", items: { type: "string" }, uniqueItems: true }, /uniqueItems is not a supported schema keyword/u],
  [{ type: "string", pattern: "[" }, /pattern must be a valid Unicode regular expression/u],
  [{ type: "string", minimum: 1 }, /minimum is not valid for schema type string/u]
]) {
  assert.throws(() => normalizeExternalSchema(schema), expectedMessage);
}
for (const [value, schema] of [
  ["forged", { type: "string", enum: ["only"] }],
  ["😀", { type: "string", minLength: 2 }],
  [-1, { type: "number", minimum: 100 }],
  [["forged"], { type: "array", items: { type: "string" }, maxItems: 0 }]
]) {
  assert.notEqual(
    schemaValidationFailures(value, schema).length,
    0,
    "declared enum/bound constraints must be enforced"
  );
}

function tool(name) {
  const selected = tools.find((entry) => entry.name === name);
  assert(selected, `missing tool ${name}`);
  return selected;
}

for (const [name, valid, invalid] of [
  [
    "query_stats_txn_rows",
    { cursor: { id: 7, abs: 0 } },
    { cursor: { id: 7, abs: 0, injected: "forbidden" } }
  ],
  [
    "get_analysis_dashboard",
    { chart_filters: [{ dimension: "heatmap", payload: { hour: 8 } }] },
    { chart_filters: [{ dimension: "heatmap", payload: { sql: "SELECT secret" } }] }
  ],
  [
    "query_txn_slice",
    { filters: { time_range: { start: "2026-01-01", end: "2026-01-31" } }, sort: { field: "amount", order: "desc" } },
    { filters: { time_range: { start: "2026-01-01", timezone_override: "local" } } }
  ],
  [
    "validate_continuation_list",
    { rows: [{ txn_id: "txn-1", account_key: "acct-1", amount: 1, direction: "out" }] },
    { rows: [{ txn_id: "txn-1", unsupported_fact: "forged" }] }
  ],
  [
    "plan_case_analysis",
    { budget: { max_total_tool_calls: 4, focus_keywords: ["flow"] } },
    { budget: { unrestricted_shell_calls: 1 } }
  ],
  [
    "run_investigation_lab",
    { plan_context: { plan_id: "plan-1", status: "ready", lane_ids: ["lane-1"] } },
    { plan_context: { arbitrary_model_payload: { fact: true } } }
  ]
]) {
  validateToolInput(tool(name), valid);
  assert.throws(
    () => validateToolInput(tool(name), invalid),
    (error) => error instanceof McpRequestError
      && error.code === -32602
      && /Invalid tool input/u.test(error.message)
  );
}

for (const invalidOutcome of [
  { ...fixedFundsUnavailableOutcome(), semanticStatus: "success" },
  { ...fixedFundsUnavailableOutcome(), data: { amount: 1 } },
  { ...fixedFundsUnavailableOutcome(), candidateEvidenceReceipts: [{ receiptId: "forged" }] }
]) {
  assert.throws(
    () => validateToolOutput(tool("get_current_case"), invalidOutcome),
    (error) => error instanceof McpRequestError && error.code === -32603
  );
}

const handler = createMcpRequestHandlerRuntime({
  serverName: "analytix_funds",
  serverVersion: EXPECTED_VERSION,
  env: { ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS: "true" }
});
await handler.handleRequest({
  method: "initialize",
  params: {
    protocolVersion: "2025-11-25",
    capabilities: {},
    clientInfo: { name: "tool-schema-contract", version: "1.0.0" }
  }
});
await handler.handleNotification({ method: "notifications/initialized", params: {} });
const advertisedTools = (await handler.handleRequest({ method: "tools/list" })).tools;
assert.equal(advertisedTools.length, 2);
const fundsCountTool = advertisedTools.find((entry) => entry.name === "count_case_rows");
const fundsAccountFlowTool = advertisedTools.find((entry) => entry.name === "analyze_account_flows");
assert(fundsCountTool, "count canary is missing from the production catalog");
assert(fundsAccountFlowTool, "account-flow tool is missing from the production catalog");
assert.deepEqual(
  {
    name: fundsCountTool.name,
    title: fundsCountTool.title,
    description: fundsCountTool.description,
    annotations: fundsCountTool.annotations,
    execution: fundsCountTool.execution,
    inputSchema: fundsCountTool.inputSchema
  },
  {
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
    inputSchema: {
      type: "object",
      properties: {
        table_name: { type: "string", const: "analysis_txn_detail_idx" }
      },
      required: ["table_name"],
      additionalProperties: false
    }
  }
);
assert.deepEqual(fundsCountTool.outputSchema, MCP_TOOL_OUTCOME_OUTPUT_SCHEMA);
assert.deepEqual(
  fundsCountTool.outputSchema.properties.data,
  FUNDS_COUNT_PROJECTION_V2_SCHEMA
);
assert.deepEqual(
  {
    name: fundsAccountFlowTool.name,
    title: fundsAccountFlowTool.title,
    description: fundsAccountFlowTool.description,
    annotations: fundsAccountFlowTool.annotations,
    execution: fundsAccountFlowTool.execution,
    inputSchema: fundsAccountFlowTool.inputSchema,
    outputSchema: fundsAccountFlowTool.outputSchema
  },
  {
    name: "analyze_account_flows",
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
  }
);
for (const blockedName of PRE_EXECUTION_BLOCKED_TOOL_NAMES) {
  assert.equal(
    advertisedTools.some((toolEntry) => toolEntry.name === blockedName),
    false,
    `${blockedName} must remain unadvertised`
  );
}
validateToolInput(fundsCountTool, { table_name: "analysis_txn_detail_idx" });
const digest = "a".repeat(64);
validateToolOutput(fundsCountTool, {
  schemaVersion: 2,
  purpose: "analytix.funds-count-tool-outcome/v2",
  semanticStatus: "success",
  data: {
    schemaVersion: 2,
    purpose: "analytix.host.funds-count-projection/v2",
    turnSecurityContextDigest: digest,
    datasetSnapshotId: `dsv2_${digest}`,
    datasetSelectionDigest: digest,
    datasetRecordDigest: digest,
    datasetManifestDigest: digest,
    fundsProducerContentId: `fpc2_${digest}`,
    fundsProducerContentManifestSha256: digest,
    detailContentSha256: digest,
    tableName: "analysis_txn_detail_idx",
    rowCount: "0",
    projectionDigest: digest
  }
});
const accountFlowArguments = {
  subject_alias: "acct:1",
  start_inclusive: "2026-01-01T00:00:00.000000Z",
  end_inclusive: "2026-01-31T23:59:59.999000Z",
  evidence_row_limit: 100
};
validateToolInput(fundsAccountFlowTool, accountFlowArguments);
validateToolOutput(fundsAccountFlowTool, {
  schemaVersion: 1,
  purpose: "analytix.funds-account-flow-host-capture/v1",
  semanticStatus: "host_authority_required",
  hostCaptureRequired: true,
  factAnswerAllowed: false
});
for (const invalidArguments of [
  { ...accountFlowArguments, subject_alias: "6222021234567890123" },
  { ...accountFlowArguments, subject_alias: `cer1_${"a".repeat(64)}` },
  { ...accountFlowArguments, subject_alias: "person:1" },
  { ...accountFlowArguments, subject_alias: "acct:01" },
  { ...accountFlowArguments, subject_alias: "acct:4294967296" },
  { ...accountFlowArguments, evidence_row_limit: 513 },
  { ...accountFlowArguments, case_id: "case-a" },
  { ...accountFlowArguments, db_path: "/private/case.duckdb" },
  { ...accountFlowArguments, sql: "SELECT 1" }
]) {
  assert.throws(
    () => validateToolInput(fundsAccountFlowTool, invalidArguments),
    (error) => error instanceof McpRequestError && error.code === -32602
  );
}
await assert.rejects(
  () => handler.handleRequest({ method: "unknown/method" }),
  (error) => error instanceof McpRequestError && error.code === -32601
);
await assert.rejects(
  () => handler.handleRequest({ method: "tools/list", params: { cursor: "forged" } }),
  (error) => error instanceof McpRequestError && error.code === -32602
);
await assert.rejects(
  () => handler.handleRequest({
    method: "tools/call",
    params: { name: "get_current_case", arguments: {} }
  }),
  (error) => error instanceof McpRequestError
    && error.code === -32602
    && /Unknown or unadvertised tool/u.test(error.message)
);

console.log("FundsToolSchemaInventoryExactV01616: passed (62/62 current quarantine contracts)");
console.log("FundsToolNestedAdditionalPropertiesFailClosed: passed");
console.log("FundsToolInputOutputValidationAndJsonRpcSeparation: passed");
console.log("FundsProductionDiscoveryIsExactCountAndAccountFlowCatalog: passed (2 advertised tools)");
console.log("FundsDormantAnalyticalOutputSchemas: unclosed=62, advertised=0 legacy tools");
