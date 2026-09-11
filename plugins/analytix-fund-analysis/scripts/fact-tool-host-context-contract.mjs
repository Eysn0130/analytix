#!/usr/bin/env node

import assert from "node:assert/strict";
import crypto from "node:crypto";

import {
  createMcpRequestHandlerRuntime,
  FUNDS_ACCOUNT_FLOW_HOST_CAPTURE_OUTPUT_SCHEMA_V1,
  FUNDS_ACCOUNT_FLOW_TOOL_INPUT_SCHEMA_V1,
  McpRequestError,
  PRE_EXECUTION_BLOCKED_TOOL_NAMES
} from "../mcp/mcp-request-handler-runtime.mjs";

const FUNDS_COUNT_PROJECTION_DIGEST_DOMAIN_V2 = Buffer.from(
  "analytix.host.funds-count-projection/digest/v2\0",
  "utf8"
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
  "projectionDigest"
];

async function activate(handler) {
  await handler.handleRequest({
    method: "initialize",
    params: {
      protocolVersion: "2025-11-25",
      capabilities: {},
      clientInfo: { name: "fact-host-context-contract", version: "1.0.0" }
    }
  });
  await handler.handleNotification({ method: "notifications/initialized", params: {} });
}

async function expectMcpError(operation, code = -32602) {
  await assert.rejects(
    operation,
    (error) => error instanceof McpRequestError && error.code === code
  );
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
  const datasetManifestDigest = fixtureDigest("fact-contract:dataset-manifest");
  const projection = {
    schemaVersion: 2,
    purpose: "analytix.host.funds-count-projection/v2",
    turnSecurityContextDigest: fixtureDigest("fact-contract:turn-security-context"),
    datasetSnapshotId: `dsv2_${datasetManifestDigest}`,
    datasetSelectionDigest: fixtureDigest("fact-contract:dataset-selection"),
    datasetRecordDigest: fixtureDigest("fact-contract:dataset-record"),
    datasetManifestDigest,
    fundsProducerContentId: `fpc2_${fixtureDigest("fact-contract:funds-producer-content")}`,
    fundsProducerContentManifestSha256: fixtureDigest(
      "fact-contract:funds-producer-content-manifest"
    ),
    detailContentSha256: fixtureDigest("fact-contract:detail-content"),
    tableName: FUNDS_COUNT_TABLE_V2,
    rowCount: "37",
    projectionDigest: "",
    ...overrides
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
        const: "analytix.host.funds-count-projection/v2"
      },
      turnSecurityContextDigest: { ...sha256Schema },
      datasetSnapshotId: {
        type: "string",
        pattern: "^dsv2_[a-f0-9]{64}$"
      },
      datasetSelectionDigest: { ...sha256Schema },
      datasetRecordDigest: { ...sha256Schema },
      datasetManifestDigest: { ...sha256Schema },
      fundsProducerContentId: {
        type: "string",
        pattern: "^fpc[12]_[a-f0-9]{64}$"
      },
      fundsProducerContentManifestSha256: { ...sha256Schema },
      detailContentSha256: { ...sha256Schema },
      tableName: { type: "string", const: FUNDS_COUNT_TABLE_V2 },
      rowCount: {
        type: "string",
        pattern: "^(0|[1-9][0-9]*)$",
        maxLength: 20
      },
      projectionDigest: { ...sha256Schema }
    },
    required: [...FUNDS_COUNT_PROJECTION_REQUIRED_KEYS_V2],
    additionalProperties: false
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
      openWorldHint: false
    },
    execution: { taskSupport: "forbidden" },
    inputSchema: {
      type: "object",
      properties: {
        table_name: { type: "string", const: FUNDS_COUNT_TABLE_V2 }
      },
      required: ["table_name"],
      additionalProperties: false
    },
    outputSchema: {
      type: "object",
      properties: {
        schemaVersion: { type: "integer", const: 2 },
        purpose: {
          type: "string",
          const: "analytix.funds-count-tool-outcome/v2"
        },
        semanticStatus: { type: "string", const: "success" },
        data: projectionSchema
      },
      required: ["schemaVersion", "purpose", "semanticStatus", "data"],
      additionalProperties: false
    }
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
      openWorldHint: false
    },
    execution: { taskSupport: "forbidden" },
    inputSchema: FUNDS_ACCOUNT_FLOW_TOOL_INPUT_SCHEMA_V1,
    outputSchema: FUNDS_ACCOUNT_FLOW_HOST_CAPTURE_OUTPUT_SCHEMA_V1
  };
}

function assertPathAndPiiFreeProjection(projection, label) {
  assert.deepEqual(
    Object.keys(projection).sort(),
    [...FUNDS_COUNT_PROJECTION_REQUIRED_KEYS_V2].sort(),
    `${label} must contain only the V2 projection fields`
  );
  const body = JSON.stringify(projection);
  assert.doesNotMatch(
    body,
    /(?:\/Users\/|\/Volumes\/|file:|workspace|projectRoot|account(?:No|Number)?|bank(?:No|Number)?|card(?:No|Number)?|identity|phone)/iu,
    `${label} exposed a path or direct PII field`
  );
}

let executions = 0;
function createHandler(env = {}) {
  return createMcpRequestHandlerRuntime({
    serverName: "analytix_funds",
    serverVersion: "host-context-contract",
    env,
    callTool: async () => {
      executions += 1;
      throw new Error("plugin-owned data source must not execute");
    }
  });
}

const handler = createHandler();
await activate(handler);
const expectedTool = fundsCountToolV2Contract();
const expectedAccountFlowTool = fundsAccountFlowToolV1Contract();
const listed = (await handler.handleRequest({ method: "tools/list" })).tools;
assert.deepEqual(
  listed,
  [expectedTool, expectedAccountFlowTool],
  "production must advertise exactly the count canary and host-captured account-flow tool"
);

const blockedLegacyToolNames = new Set([
  ...PRE_EXECUTION_BLOCKED_TOOL_NAMES,
  "get_current_case",
  "rank_accounts",
  "run_case_sql",
  "validate_report_claims"
]);
for (const blockedName of blockedLegacyToolNames) {
  assert.equal(
    listed.some((tool) => tool.name === blockedName),
    false,
    `${blockedName} must remain undiscoverable`
  );
}

const fullDiscoveryHandler = createHandler({ ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS: "true" });
await activate(fullDiscoveryHandler);
assert.deepEqual(
  (await fullDiscoveryHandler.handleRequest({ method: "tools/list" })).tools,
  [expectedTool, expectedAccountFlowTool],
  "FullDiscoveryEnvCannotExpandProductionFundsToolSurface"
);

for (const toolName of blockedLegacyToolNames) {
  await expectMcpError(() => handler.handleRequest({
    method: "tools/call",
    params: { name: toolName, arguments: {} }
  }));
  await expectMcpError(() => handler.handleRequest({
    method: "tools/call",
    params: {
      name: toolName,
      arguments: {},
      _meta: {
        analytixRuntimeContext: {
          version: 1,
          toolName
        }
      }
    }
  }));
}

const projection = fundsCountProjectionV2();
assertPathAndPiiFreeProjection(projection, "valid host projection");
const forgedProjection = {
  ...projection,
  rowCount: "38"
};
const mismatchedProjection = fundsCountProjectionV2({
  datasetSnapshotId: `dsv2_${fixtureDigest("fact-contract:mismatched-snapshot")}`
});
const invalidProjectionMetaCases = [
  ["missing projection", undefined],
  [
    "legacy runtime context",
    {
      analytixRuntimeContext: {
        version: 1,
        datasetSnapshotId: "dsv1_legacy"
      }
    }
  ],
  [
    "V2 projection mixed with legacy runtime context",
    {
      [FUNDS_COUNT_PROJECTION_META_KEY_V2]: projection,
      analytixRuntimeContext: {
        version: 1,
        datasetSnapshotId: "dsv1_legacy"
      }
    }
  ],
  [
    "forged projection digest",
    { [FUNDS_COUNT_PROJECTION_META_KEY_V2]: forgedProjection }
  ],
  [
    "mismatched snapshot projection",
    { [FUNDS_COUNT_PROJECTION_META_KEY_V2]: mismatchedProjection }
  ]
];
const invalidProjectionRequests = [
  [
    "tools/call",
    (meta) => ({
      method: "tools/call",
      params: {
        name: "count_case_rows",
        arguments: { table_name: FUNDS_COUNT_TABLE_V2 },
        ...(meta === undefined ? {} : { _meta: meta })
      }
    })
  ],
  [
    "analytix/sourceProbe",
    (meta) => ({
      method: "analytix/sourceProbe",
      params: meta === undefined ? {} : { _meta: meta }
    })
  ],
  [
    "analytix/evidenceRead",
    (meta) => ({
      method: "analytix/evidenceRead",
      params: {
        tool: "count_case_rows",
        tableName: FUNDS_COUNT_TABLE_V2,
        noFilter: true,
        ...(meta === undefined ? {} : { _meta: meta })
      }
    })
  ]
];
for (const [metaLabel, meta] of invalidProjectionMetaCases) {
  for (const [methodLabel, request] of invalidProjectionRequests) {
    await expectMcpError(() => handler.handleRequest(request(meta)));
    assert.equal(
      executions,
      0,
      `${methodLabel} ${metaLabel} must fail before plugin data execution`
    );
  }
}

await expectMcpError(() => handler.handleRequest({
  method: "tools/call",
  params: {
    name: "count_case_rows",
    arguments: { table_name: "analysis_txn_summary" },
    _meta: { [FUNDS_COUNT_PROJECTION_META_KEY_V2]: projection }
  }
}));
assert.equal(executions, 0, "mismatched table input must fail before plugin data execution");

const accountFlowArguments = {
  subject_alias: "acct:1",
  start_inclusive: "2026-01-01T00:00:00.000000Z",
  end_inclusive: "2026-01-31T23:59:59.999000Z",
  evidence_row_limit: 100
};
for (const [label, invalidArguments] of [
  ["full account instead of entity alias", {
    ...accountFlowArguments,
    subject_alias: "6222021234567890123"
  }],
  ["authority reference instead of entity alias", { ...accountFlowArguments, subject_alias: `cer1_${"a".repeat(64)}` }],
  ["unsupported alias prefix", { ...accountFlowArguments, subject_alias: "person:1" }],
  ["non-canonical alias ordinal", { ...accountFlowArguments, subject_alias: "acct:01" }],
  ["overflow alias ordinal", { ...accountFlowArguments, subject_alias: "acct:4294967296" }],
  ["caller case authority", { ...accountFlowArguments, case_id: "case-a" }],
  ["caller dataset authority", { ...accountFlowArguments, datasetSnapshotId: `dsv2_${"a".repeat(64)}` }],
  ["database path", { ...accountFlowArguments, db_path: "/private/case.duckdb" }],
  ["arbitrary SQL", { ...accountFlowArguments, sql: "SELECT * FROM secret" }],
  ["unbounded evidence", { ...accountFlowArguments, evidence_row_limit: 513 }]
]) {
  await expectMcpError(() => handler.handleRequest({
    method: "tools/call",
    params: {
      name: "analyze_account_flows",
      arguments: invalidArguments
    }
  }));
  assert.equal(executions, 0, `${label} must fail before plugin data execution`);
}

const accountFlowBoundary = await handler.handleRequest({
  method: "tools/call",
  params: {
    name: "analyze_account_flows",
    arguments: accountFlowArguments,
    _meta: {
      analytixRuntimeContext: {
        caseId: "forged-case",
        datasetSnapshotId: `dsv2_${"b".repeat(64)}`
      }
    }
  }
});
assert.deepEqual(accountFlowBoundary, {
  content: [],
  isError: true,
  structuredContent: {
    schemaVersion: 1,
    purpose: "analytix.funds-account-flow-host-capture/v1",
    semanticStatus: "host_authority_required",
    hostCaptureRequired: true,
    factAnswerAllowed: false
  }
});
assert.equal(executions, 0, "direct Node account-flow dispatch must perform zero data effects");
for (const forbidden of [
  accountFlowArguments.subject_alias,
  accountFlowArguments.start_inclusive,
  accountFlowArguments.end_inclusive,
  "forged-case",
  "dsv2_"
]) {
  assert.equal(
    JSON.stringify(accountFlowBoundary).includes(forbidden),
    false,
    `direct account-flow boundary reflected private call material: ${forbidden}`
  );
}

const projectionMeta = {
  [FUNDS_COUNT_PROJECTION_META_KEY_V2]: projection
};
const toolResult = await handler.handleRequest({
  method: "tools/call",
  params: {
    name: "count_case_rows",
    arguments: { table_name: FUNDS_COUNT_TABLE_V2 },
    _meta: projectionMeta
  }
});
assert.deepEqual(toolResult, {
  content: [],
  structuredContent: {
    schemaVersion: 2,
    purpose: "analytix.funds-count-tool-outcome/v2",
    semanticStatus: "success",
    data: projection
  }
});

const sourceProbe = await handler.handleRequest({
  method: "analytix/sourceProbe",
  params: { _meta: projectionMeta }
});
assert.deepEqual(sourceProbe, {
  schemaVersion: 2,
  purpose: "analytix.funds-source-probe/v2",
  serverName: "analytix_funds",
  serverVersion: "host-context-contract",
  projectionDigest: projection.projectionDigest,
  ready: true,
  readOnly: true
});

const evidenceRead = await handler.handleRequest({
  method: "analytix/evidenceRead",
  params: {
    tool: "count_case_rows",
    tableName: FUNDS_COUNT_TABLE_V2,
    noFilter: true,
    _meta: projectionMeta
  }
});
assert.deepEqual(evidenceRead, {
  schemaVersion: 2,
  purpose: "analytix.funds.count-case-rows-evidence-candidate/v2",
  serverName: "analytix_funds",
  serverVersion: "host-context-contract",
  toolName: "count_case_rows",
  projection,
  paginationComplete: true,
  readOnly: true
});
assert.deepEqual(
  toolResult.structuredContent.data,
  evidenceRead.projection,
  "tool and evidenceRead must return the exact same immutable host projection"
);
assert.equal(
  sourceProbe.projectionDigest,
  evidenceRead.projection.projectionDigest,
  "sourceProbe and evidenceRead must bind the exact same immutable host projection"
);
assertPathAndPiiFreeProjection(toolResult.structuredContent.data, "tool result");
assertPathAndPiiFreeProjection(evidenceRead.projection, "evidence read");
assert.equal(
  JSON.stringify({ toolResult, sourceProbe, evidenceRead })
    .includes("analytixRuntimeContext"),
  false,
  "V2 result surfaces must not restore the legacy analytixRuntimeContext carrier"
);
assert.equal(executions, 0, "V2 host projection handling must not execute plugin data");

await expectMcpError(
  () => handler.handleRequest({ method: "tools/list", params: { cursor: "forged" } })
);

console.log("ProductionAdvertisesExactFundsCountProjectionV2Tool: passed");
console.log("FundsCountProjectionV2RejectsMissingForgedMismatchedAndLegacyAuthority: passed");
console.log("FundsCountProjectionV2ToolProbeAndEvidenceRemainHostOnly: passed");
console.log("FundsAccountFlowCatalogIsProviderSafeAndDirectDispatchRequiresHostAuthority: passed");
