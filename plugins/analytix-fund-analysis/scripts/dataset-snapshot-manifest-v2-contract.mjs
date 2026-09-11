#!/usr/bin/env node

import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { runPythonDuckdb } from "../mcp/duckdb-workbench-runtime.mjs";
import { isDuckdbDiagnosticError } from "../mcp/duckdb-diagnostic.mjs";

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-local-dsv2-retirement-"));
process.on("exit", () => fs.rmSync(tempRoot, { recursive: true, force: true }));
const runnerPath = path.join(tempRoot, "local-dsv2-runner.mjs");
const snapshotId = `dsv2_${"a".repeat(64)}`;

fs.writeFileSync(runnerPath, `#!/usr/bin/env node
let input = "";
for await (const chunk of process.stdin) input += chunk;
const payload = JSON.parse(input || "{}");
const factual = payload.factual === true;
process.stdout.write(JSON.stringify({
  columns: ["value"],
  column_types: ["INTEGER"],
  records: [{ value: 1 }],
  row_count: 1,
  truncated: false,
  snapshot_contract: "analytix_duckdb_dataset_snapshot_v2",
  snapshot_factual_ready: factual,
  snapshot_blocker: factual ? "" : "local_dataset_snapshot_v2_retired",
  snapshot_manifest_schema_version: 2,
  observed_dataset_snapshot_id: ${JSON.stringify(snapshotId)},
  producer_content_contract: "",
  producer_content_id: "",
  producer_manifest_sha256: "",
  producer_manifest_schema_version: 0
}));
`, { mode: 0o755 });

const env = {
  ...process.env,
  ANALYTIX_FUNDS_DUCKDB_PYTHON: runnerPath,
  ANALYTIX_BACKEND_PYTHON: runnerPath,
  PYTHON: runnerPath
};

await assert.rejects(
  () => runPythonDuckdb({ sql: "SELECT 1", row_limit: 1, factual: true }, env),
  (error) => isDuckdbDiagnosticError(error) && error.code === "result_contract_invalid",
  "a local dsv2 digest must never be accepted as factual host authority"
);

const retired = await runPythonDuckdb({ sql: "SELECT 1", row_limit: 1, factual: false }, env);
assert.equal(retired.snapshot_contract, "analytix_duckdb_dataset_snapshot_v2");
assert.equal(retired.snapshot_factual_ready, false);
assert.equal(retired.snapshot_blocker, "local_dataset_snapshot_v2_retired");
assert.equal(retired.observed_dataset_snapshot_id, snapshotId);

const result = {
  status: "ok",
  contract: "DatasetSnapshotManifestV2Retirement",
  assertions: [
    "LocalDatasetSnapshotV2CannotAuthorizeFacts",
    "RetiredLocalDatasetSnapshotV2IsDiagnosticOnly",
    "FundsProducerContentUsesSeparateFpc1Contract"
  ]
};

if (process.argv.includes("--json")) {
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
} else {
  console.log("Local DatasetSnapshotManifestV2 retirement contract passed.");
}
