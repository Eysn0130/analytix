#!/usr/bin/env node

import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import {
  assertDuckdbDiagnosticV1,
  createDuckdbDiagnostic
} from "../mcp/duckdb-diagnostic.mjs";
import {
  canonicalPythonCandidates,
  runPythonDuckdb
} from "../mcp/duckdb-workbench-runtime.mjs";

const root = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-duckdb-diagnostic-"));
process.on("exit", () => fs.rmSync(root, { recursive: true, force: true }));

function executable(name, body) {
  const target = path.join(root, name);
  fs.writeFileSync(target, `#!/bin/sh\n${body}\n`, { mode: 0o755 });
  return target;
}

const unavailableMarker = path.join(root, "unavailable.marker");
const successMarker = path.join(root, "success.marker");
const binderMarker = path.join(root, "binder.marker");
const success = executable("python-success", String.raw`
printf 'success\n' >> ${JSON.stringify(successMarker)}
cat >/dev/null
printf '%s' '{"columns":["value"],"records":[{"value":1}],"row_count":1,"truncated":false,"snapshot_contract":"","snapshot_factual_ready":false,"snapshot_blocker":"","snapshot_manifest_schema_version":0,"observed_dataset_snapshot_id":""}'
`);
const unavailable = executable("python-unavailable", String.raw`
printf 'unavailable\n' >> ${JSON.stringify(unavailableMarker)}
cat >/dev/null
printf '%s\n' "ModuleNotFoundError: No module named 'duckdb'" >&2
exit 1
`);
const binderFailure = executable("python-binder-failure", String.raw`
printf 'binder\n' >> ${JSON.stringify(binderMarker)}
cat >/dev/null
printf '%s\n' 'Binder Error: SOL_PRIVATE_TRACE_7C account 6222021234567890123 is absent' >&2
exit 1
`);

const alias = path.join(root, "python-success-alias");
fs.symlinkSync(success, alias);
const canonical = canonicalPythonCandidates({
  ...process.env,
  ANALYTIX_FUNDS_DUCKDB_PYTHON: success,
  ANALYTIX_BACKEND_PYTHON: alias,
  PYTHON: success
});
assert.equal(canonical.filter((candidate) => {
  try {
    return fs.realpathSync(candidate) === fs.realpathSync(success);
  } catch {
    return false;
  }
}).length, 1, "Python aliases must resolve to one runner identity");

const fallbackResult = await runPythonDuckdb({ sql: "SELECT 1", row_limit: 1 }, {
  ...process.env,
  ANALYTIX_FUNDS_DUCKDB_PYTHON: unavailable,
  ANALYTIX_BACKEND_PYTHON: success,
  PYTHON: success
});
assert.equal(fallbackResult.records[0].value, 1);
assert.equal(fs.readFileSync(unavailableMarker, "utf8"), "unavailable\n");
assert.equal(fs.readFileSync(successMarker, "utf8"), "success\n");

let binderError;
try {
  await runPythonDuckdb({
    sql: "SELECT account_no FROM analysis_secret_accounts",
    row_limit: 1
  }, {
    ...process.env,
    ANALYTIX_FUNDS_DUCKDB_PYTHON: binderFailure,
    ANALYTIX_BACKEND_PYTHON: success,
    PYTHON: success
  });
} catch (error) {
  binderError = error;
}
assert.ok(binderError, "binder failure must fail closed");
assert.equal(binderError.duckdbDiagnostic?.code, "sql_bind_rejected");
assert.equal(binderError.duckdbDiagnostic?.retryable, false);
assert.equal(fs.readFileSync(binderMarker, "utf8"), "binder\n");
assert.equal(fs.readFileSync(successMarker, "utf8"), "success\n", "binder rejection must not fall through");
const publicFailure = JSON.stringify({ message: binderError.message, diagnostic: binderError.duckdbDiagnostic });
for (const forbidden of ["SOL_PRIVATE_TRACE_7C", "6222021234567890123", "analysis_secret_accounts", "account_no"]) {
  assert.ok(!publicFailure.includes(forbidden), `public DuckDB failure leaked ${forbidden}: ${publicFailure}`);
}

const canonicalDiagnostic = createDuckdbDiagnostic({ code: "runner_timeout", stage: "execute", sql: "SELECT 1" });
assert.equal(assertDuckdbDiagnosticV1(canonicalDiagnostic), canonicalDiagnostic);
assert.throws(() => assertDuckdbDiagnosticV1({ ...canonicalDiagnostic, code: "invented_code" }), /not canonical/iu);
assert.throws(() => assertDuckdbDiagnosticV1({ ...canonicalDiagnostic, rawSql: "SELECT 1" }), /unknown or missing/iu);

console.log("DuckdbRunnerUnavailableFallsThroughOnce: passed");
console.log("DuckdbPythonAliasesDoNotDuplicateRunner: passed");
console.log("DuckdbBinderFailureExecutesOnce: passed");
console.log("DuckdbDiagnosticRejectsUnknownCode: passed");
console.log("DuckdbDiagnosticLeaksNoSqlPathAccountOrReasoning: passed");
