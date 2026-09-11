#!/usr/bin/env node

import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {
  CASE_SOURCE_PROBE_METHOD,
  caseSourceProbeResult,
  createMcpRequestHandlerRuntime,
  FUNDS_EVIDENCE_READ_METHOD,
  fundsCountEvidenceCandidate,
  McpRequestError
} from "../mcp/mcp-request-handler-runtime.mjs";
import {
  hostFactRuntimeContext,
  hostSourceProbeRuntimeContext,
  writeCaseProjectBinding
} from "./host-runtime-context-fixture.mjs";

export const SOURCE_LIVE_PROBE_CONTRACT_VERSION = "0.16.16";

async function activate(handler) {
  await handler.handleRequest({
    method: "initialize",
    params: { protocolVersion: "2025-11-25", capabilities: {}, clientInfo: { name: "source-probe-contract", version: "1.0.0" } }
  });
  await handler.handleNotification({ method: "notifications/initialized", params: {} });
}

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-source-probe-contract-"));
process.on("exit", () => fs.rmSync(tempRoot, { recursive: true, force: true }));
const projectRoot = path.join(tempRoot, "case-project");
fs.mkdirSync(projectRoot, { recursive: true });
const binding = writeCaseProjectBinding(projectRoot, "case_1234");
const runtimeContext = hostFactRuntimeContext({
  ...binding,
  caseId: "case_1234",
  toolName: "count_case_rows",
  args: { table_name: "analysis_txn_detail_idx" },
  serverVersion: SOURCE_LIVE_PROBE_CONTRACT_VERSION
});
const sourceProbeContext = hostSourceProbeRuntimeContext(runtimeContext);
const goNativeProbeContext = {
  version: runtimeContext.version,
  workspaceRealPath: runtimeContext.workspaceRealPath,
  threadId: runtimeContext.threadId,
  turnId: runtimeContext.turnId,
  caseId: runtimeContext.caseId,
  caseBindingHash: runtimeContext.caseBindingHash,
  datasetSnapshotId: runtimeContext.datasetSnapshotId,
  contextEpoch: runtimeContext.contextEpoch,
  contextDigest: runtimeContext.contextDigest
};
assert.deepEqual(sourceProbeContext, goNativeProbeContext);

const pipeline = {
  case_id: "case_1234",
  observed_dataset_snapshot_id: runtimeContext.datasetSnapshotId,
  snapshot_contract: "analytix_duckdb_dataset_snapshot_v1",
  ready: true,
  read_only: true
};

const ready = caseSourceProbeResult(pipeline, runtimeContext, "analytix_funds", SOURCE_LIVE_PROBE_CONTRACT_VERSION);
assert.equal(ready.ready, false);
assert.equal(ready.datasetSnapshotId, "");
assert.equal(ready.blocker, "dataset_snapshot_authority_unavailable");
assert.equal(ready.caseId, "case_1234");

const partial = caseSourceProbeResult({
  ...pipeline,
  observed_dataset_snapshot_id: ""
}, runtimeContext, "analytix_funds", SOURCE_LIVE_PROBE_CONTRACT_VERSION);
assert.equal(partial.ready, false);
assert.equal(partial.datasetSnapshotId, "");
assert.equal(partial.blocker, "dataset_snapshot_authority_unavailable");

const mismatch = caseSourceProbeResult({
  ...pipeline,
  snapshot_contract: "untrusted_echo_v1"
}, runtimeContext, "analytix_funds", SOURCE_LIVE_PROBE_CONTRACT_VERSION);
assert.equal(mismatch.ready, false);
assert.equal(mismatch.blocker, "dataset_snapshot_authority_unavailable");

const bareV2 = caseSourceProbeResult({
  ...pipeline,
  snapshot_contract: "analytix_duckdb_dataset_snapshot_v2",
  observed_dataset_snapshot_id: `dsv2_${"2".repeat(64)}`
}, {
  ...runtimeContext,
  datasetSnapshotId: `dsv2_${"2".repeat(64)}`
}, "analytix_funds", SOURCE_LIVE_PROBE_CONTRACT_VERSION);
assert.equal(bareV2.ready, false);
assert.equal(bareV2.datasetSnapshotId, "");
assert.equal(bareV2.blocker, "dataset_snapshot_authority_unavailable");

const snapshotMismatch = caseSourceProbeResult({
  ...pipeline,
  observed_dataset_snapshot_id: `dsv1_${"1".repeat(64)}`
}, runtimeContext, "analytix_funds", SOURCE_LIVE_PROBE_CONTRACT_VERSION);
assert.equal(snapshotMismatch.ready, false);
assert.equal(snapshotMismatch.datasetSnapshotId, "");
assert.equal(snapshotMismatch.blocker, "dataset_snapshot_mismatch");

let sourceProbeToolCalls = 0;
const pythonMarker = path.join(tempRoot, "forbidden-python-marker");
const maliciousPython = path.join(tempRoot, "malicious-python");
fs.writeFileSync(
  maliciousPython,
  "#!/bin/sh\nprintf invoked > \"$ANALYTIX_FUNDS_PROBE_MARKER\"\nexit 99\n",
  { encoding: "utf8", mode: 0o700 }
);
const handler = createMcpRequestHandlerRuntime({
  serverName: "analytix_funds",
  serverVersion: SOURCE_LIVE_PROBE_CONTRACT_VERSION,
  pluginRoot: new URL("..", import.meta.url).pathname,
  callTool: async () => {
    sourceProbeToolCalls += 1;
    throw new Error("source probe must not invoke plugin-local execution");
  },
  toolResult: () => {
    throw new Error("source probe must not use provider-facing toolResult");
  },
  env: {
    ANALYTIX_FUNDS_DUCKDB_PYTHON: maliciousPython,
    ANALYTIX_FUNDS_PROBE_MARKER: pythonMarker
  }
});
await activate(handler);

const probed = await handler.handleRequest({
  jsonrpc: "2.0",
  id: 1,
  method: CASE_SOURCE_PROBE_METHOD,
  params: { _meta: { analytixRuntimeContext: goNativeProbeContext } }
});
assert.equal(probed.ready, false);
assert.equal(probed.datasetSnapshotId, "");
assert.equal(probed.blocker, "dataset_snapshot_authority_unavailable");
assert.equal(sourceProbeToolCalls, 0, "source probe must fail before plugin-local tool or process execution");
assert.equal(fs.existsSync(pythonMarker), false, "source probe must never execute an env-selected Python marker");

await assert.rejects(
  () => handler.handleRequest({
    jsonrpc: "2.0",
    id: 11,
    method: CASE_SOURCE_PROBE_METHOD,
    params: {
      _meta: {
        analytixRuntimeContext: {
          ...goNativeProbeContext,
          datasetSnapshotId: undefined
        }
      }
    }
  }),
  (error) => error instanceof McpRequestError
    && error.code === -32602
    && error.message.includes("datasetSnapshotId")
);

const countPayload = {
  skill_id: "count_case_rows",
  version: SOURCE_LIVE_PROBE_CONTRACT_VERSION,
  status: "ok",
  data: {
    case_id: runtimeContext.caseId,
    table_name: "analysis_txn_detail_idx",
    requested_table_name: "analysis_txn_detail_idx",
    where_applied: false,
    row_count: 2645472,
    result_kind: "row_count_only",
    observed_dataset_snapshot_id: runtimeContext.datasetSnapshotId,
    snapshot_contract: "analytix_duckdb_dataset_snapshot_v1",
    result_completeness: { status: "complete" }
  }
};
assert.throws(
  () => fundsCountEvidenceCandidate(countPayload, runtimeContext, "analytix_funds", SOURCE_LIVE_PROBE_CONTRACT_VERSION),
  (error) => error instanceof McpRequestError
    && error.code === -32603
    && error.message.includes("registry-backed DatasetSnapshotManifestV2")
);

let evidenceToolCalls = 0;
const evidenceHandler = createMcpRequestHandlerRuntime({
  serverName: "analytix_funds",
  serverVersion: SOURCE_LIVE_PROBE_CONTRACT_VERSION,
  pluginRoot: new URL("..", import.meta.url).pathname,
  callTool: async (name, args) => {
    evidenceToolCalls += 1;
    assert.equal(name, "count_case_rows");
    assert.deepEqual(Object.keys(args).sort(), ["_analytix", "case_id", "table_name"]);
    assert.equal(args.table_name, "analysis_txn_detail_idx");
    return countPayload;
  },
  toolResult: () => {
    throw new Error("host-native evidence read must not use provider-facing toolResult");
  },
  env: {}
});
await activate(evidenceHandler);
await assert.rejects(
  () => evidenceHandler.handleRequest({
    jsonrpc: "2.0",
    id: 3,
    method: FUNDS_EVIDENCE_READ_METHOD,
    params: {
      tool: "count_case_rows",
      tableName: "analysis_txn_detail_idx",
      noFilter: true,
      _meta: { analytixRuntimeContext: runtimeContext }
    }
  }),
  (error) => error instanceof McpRequestError
    && error.code === -32603
    && error.message.includes("registry-backed DatasetSnapshotManifestV2")
);
assert.equal(evidenceToolCalls, 0, "legacy evidenceRead must block before executing count_case_rows");
await assert.rejects(
  () => evidenceHandler.handleRequest({
    jsonrpc: "2.0",
    id: 4,
    method: FUNDS_EVIDENCE_READ_METHOD,
    params: {
      tool: "count_case_rows",
      tableName: "analysis_txn_detail_idx",
      noFilter: true,
      whereSql: "1=1",
      _meta: { analytixRuntimeContext: runtimeContext }
    }
  }),
  (error) => error instanceof McpRequestError && error.code === -32602
);

await assert.rejects(
  () => handler.handleRequest({
    jsonrpc: "2.0",
    id: 2,
    method: "tools/call",
    params: { name: CASE_SOURCE_PROBE_METHOD, arguments: {} }
  }),
  (error) => error instanceof McpRequestError && error.code === -32602
);

console.log("source live probe contract: passed");
