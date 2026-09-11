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

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-duckdb-cancel-"));
process.on("exit", () => fs.rmSync(tempRoot, { recursive: true, force: true }));

const projectRoot = path.join(tempRoot, "case-project");
const casesRoot = path.join(tempRoot, "cases");
const caseId = "case_cancel_contract";
fs.mkdirSync(projectRoot, { recursive: true });
fs.mkdirSync(path.join(casesRoot, caseId), { recursive: true });
fs.writeFileSync(path.join(casesRoot, caseId, "case.duckdb"), "not-a-real-database\n", "utf8");
const binding = writeCaseProjectBinding(projectRoot, caseId);

const startedMarker = path.join(tempRoot, "python-started");
const terminatedMarker = path.join(tempRoot, "python-terminated");
const fakePython = path.join(tempRoot, "fake-python");
fs.writeFileSync(fakePython, `#!/usr/bin/env node
const fs = require("node:fs");
fs.writeFileSync(${JSON.stringify(startedMarker)}, "started\\n", "utf8");
process.on("SIGTERM", () => {
  fs.writeFileSync(${JSON.stringify(terminatedMarker)}, "terminated\\n", "utf8");
  process.exit(143);
});
setInterval(() => {}, 1_000);
`, { mode: 0o755 });

const env = {
  ...process.env,
  ANALYTIX_CASE_PROJECT_ROOT: projectRoot,
  ANALYTIX_DATA_ANALYSIS_CASES_ROOT: casesRoot,
  ANALYTIX_FUNDS_DUCKDB_PYTHON: fakePython,
  ANALYTIX_BACKEND_PYTHON: fakePython,
  PYTHON: fakePython
};
const authority = hostFactRuntimeContext({
  ...binding,
  caseId,
  toolName: "run_case_sql",
  args: {
    sql: "SELECT account_key FROM analysis_account_dim LIMIT 1",
    purpose: "cancellation contract"
  },
  serverVersion: "duckdb-cancellation-contract"
});

async function waitForFile(filePath, timeoutMs = 5_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (fs.existsSync(filePath)) return;
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
  throw new Error(`Timed out waiting for ${path.basename(filePath)}`);
}

const controller = new AbortController();
const execution = executeLocalDuckdbWorkbenchSkill("run_case_sql", {
  case_id: caseId,
  purpose: "cancellation contract",
  sql: "SELECT account_key FROM analysis_account_dim LIMIT 1",
  _analytix: authority
}, { env, signal: controller.signal });

await waitForFile(startedMarker);
controller.abort(new Error("contract cancellation"));
await assert.rejects(execution, /contract cancellation/u);
await waitForFile(terminatedMarker);

const history = await executeLocalDuckdbWorkbenchSkill("inspect_workbench_history", {
  case_id: caseId,
  limit: 20,
  _analytix: authority
}, { env });
assert.equal(history.data.history_available, false);
assert.deepEqual(history.data.entries, []);

console.log("DuckdbCancellationTerminatesPythonRunner: passed");
console.log("DuckdbCancellationDoesNotWriteWorkbenchHistory: passed");
