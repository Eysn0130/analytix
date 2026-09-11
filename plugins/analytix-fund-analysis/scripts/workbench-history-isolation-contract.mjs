#!/usr/bin/env node

import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { executeLocalDuckdbWorkbenchSkill } from "../mcp/duckdb-workbench-runtime.mjs";
import {
  hostFactRuntimeContext,
  writeCaseProjectBinding
} from "./host-runtime-context-fixture.mjs";

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-workbench-history-isolation-"));
process.on("exit", () => fs.rmSync(tempRoot, { recursive: true, force: true }));
const projectRoot = path.join(tempRoot, "case-project");
fs.mkdirSync(projectRoot, { recursive: true });
const caseId = "case_history_isolation";
const binding = writeCaseProjectBinding(projectRoot, caseId);
const env = { ...process.env, ANALYTIX_CASE_PROJECT_ROOT: projectRoot };

function authority(overrides = {}) {
  return hostFactRuntimeContext({
    ...binding,
    caseId,
    toolName: "case_sql_recipes",
    args: {},
    serverVersion: "history-isolation-contract",
    overrides
  });
}

async function recipes(context) {
  return executeLocalDuckdbWorkbenchSkill("case_sql_recipes", {
    case_id: caseId,
    _analytix: context
  }, { env });
}

async function history(context) {
  return executeLocalDuckdbWorkbenchSkill("inspect_workbench_history", {
    case_id: caseId,
    limit: 20,
    ...(context ? { _analytix: context } : {})
  }, { env });
}

const contextA = authority({
  threadId: "thread_history_a",
  turnId: "turn_history_a",
  contextDigest: "1".repeat(64)
});
const contextB = authority({
  threadId: "thread_history_b",
  turnId: "turn_history_b",
  contextDigest: "2".repeat(64)
});
const epochB = { ...contextA, contextEpoch: contextA.contextEpoch + 1, contextDigest: "3".repeat(64) };
const snapshotB = { ...contextA, datasetSnapshotId: `dsv1_${"4".repeat(64)}`, contextDigest: "5".repeat(64) };

const diagnoseArgs = { purpose: "diagnose without SQL" };
const diagnoseContext = hostFactRuntimeContext({
  ...binding,
  caseId,
  toolName: "diagnose_case_sql",
  args: diagnoseArgs,
  serverVersion: "history-isolation-contract",
  overrides: {
    threadId: "thread_diagnose_unknown",
    turnId: "turn_diagnose_unknown",
    contextDigest: "6".repeat(64)
  }
});
const diagnoseWithoutSql = await executeLocalDuckdbWorkbenchSkill("diagnose_case_sql", {
  case_id: caseId,
  ...diagnoseArgs,
  _analytix: diagnoseContext
}, { env });
const diagnoseAudit = diagnoseWithoutSql.data.audit_events[0];
assert.equal(
  Object.prototype.hasOwnProperty.call(diagnoseAudit, "slow_path_likely"),
  false,
  "missing SQL/plan observation must not become slow_path_likely=false"
);

await recipes(contextA);
const historyA = await history(contextA);
assert.equal(historyA.data.history_available, true);
assert.equal(historyA.data.entries.length, 1);
assert.equal(historyA.data.entries[0].skill_id, "case_sql_recipes");
assert.match(historyA.data.entries[0].query_id, /^local-duckdb:case_sql_recipes:[a-f0-9]{64}$/u);
for (const key of ["truncated", "slow_path_likely"]) {
  assert.equal(
    Object.prototype.hasOwnProperty.call(historyA.data.entries[0], key),
    false,
    `missing ${key} must remain unknown`
  );
}
for (const key of [
  "readonly",
  "current_case_only",
  "cleaned_analysis_scope_only",
  "parser_binder_validated",
  "raw_rows_exposed"
]) {
  assert.equal(
    Object.prototype.hasOwnProperty.call(historyA.data.entries[0].validation_state, key),
    false,
    `missing validation_state.${key} must remain unknown`
  );
}

for (const isolatedContext of [contextB, epochB, snapshotB]) {
  const isolated = await history(isolatedContext);
  assert.equal(isolated.data.history_available, false);
  assert.deepEqual(isolated.data.entries, []);
}
const authorityMissing = await history(null);
assert.equal(authorityMissing.data.history_available, false);
assert.deepEqual(authorityMissing.data.entries, []);

await recipes(contextB);
const historyBAfterWrite = await history(contextB);
assert.equal(historyBAfterWrite.data.entries.length, 1);
const historyAAfterB = await history(contextA);
assert.equal(historyAAfterB.data.entries.length, 1);
for (const entry of [...historyBAfterWrite.data.entries, ...historyAAfterB.data.entries]) {
  assert.equal(Object.prototype.hasOwnProperty.call(entry, "authority_key"), false);
  if (entry.query_digest) assert.match(entry.query_digest, /^[a-f0-9]{64}$/u);
  const serialized = JSON.stringify(entry);
  assert.equal(serialized.includes("thread_history_"), false);
  assert.equal(serialized.includes("dsv1_"), false);
}

console.log("WorkbenchHistoryNeverCrossesThreadEpochOrSnapshot: passed");
console.log("FullSha256RequiredForEvidenceIdentity: passed");
