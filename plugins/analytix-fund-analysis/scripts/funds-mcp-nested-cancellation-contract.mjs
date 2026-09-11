#!/usr/bin/env node

import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  hostFactRuntimeContext,
  writeCaseProjectBinding
} from "./host-runtime-context-fixture.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");
const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-funds-nested-cancel-"));
process.on("exit", () => fs.rmSync(tempRoot, { recursive: true, force: true }));

const projectRoot = path.join(tempRoot, "case-project");
const casesRoot = path.join(tempRoot, "cases");
const caseId = "case_nested_cancel";
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

const child = spawn(process.execPath, [path.join(pluginRoot, "mcp", "server.mjs")], {
  cwd: pluginRoot,
  env: {
    ...process.env,
    ANALYTIX_API_BASE_URL: "http://127.0.0.1:9",
    ANALYTIX_CASE_PROJECT_ROOT: projectRoot,
    ANALYTIX_DATA_ANALYSIS_CASES_ROOT: casesRoot,
    ANALYTIX_FUNDS_DUCKDB_PYTHON: fakePython,
    ANALYTIX_BACKEND_PYTHON: fakePython,
    PYTHON: fakePython
  },
  stdio: ["pipe", "pipe", "pipe"]
});

let stderr = "";
child.stderr.setEncoding("utf8");
child.stderr.on("data", (chunk) => { stderr += chunk; });
const messages = [];
let stdoutBuffer = "";
child.stdout.setEncoding("utf8");
child.stdout.on("data", (chunk) => {
  stdoutBuffer += chunk;
  let newline = stdoutBuffer.indexOf("\n");
  while (newline >= 0) {
    const line = stdoutBuffer.slice(0, newline).trim();
    stdoutBuffer = stdoutBuffer.slice(newline + 1);
    if (line) messages.push(JSON.parse(line));
    newline = stdoutBuffer.indexOf("\n");
  }
});

function send(message) {
  child.stdin.write(`${JSON.stringify(message)}\n`);
}

async function waitFor(predicate, label, timeoutMs = 8_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const value = predicate();
    if (value) return value;
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
  throw new Error(`Timed out waiting for ${label}; stderr=${stderr.slice(-500)}`);
}

try {
  send({
    jsonrpc: "2.0",
    id: 1,
    method: "initialize",
    params: {
      protocolVersion: "2025-11-25",
      capabilities: {},
      clientInfo: { name: "nested-cancellation-contract", version: "1.0.0" }
    }
  });
  await waitFor(() => messages.find((message) => message.id === 1), "initialize response");
  send({ jsonrpc: "2.0", method: "notifications/initialized", params: {} });

  const args = {
    case_id: caseId,
    intent: "destination",
    question: "甲方转给乙方多少钱？",
    holder_name: "甲方",
    via_holder_name: "乙方",
    top_n: 20
  };
  const authority = hostFactRuntimeContext({
    ...binding,
    caseId,
    toolName: "funds_investigate",
    args,
    serverVersion: "0.16.16"
  });
  send({
    jsonrpc: "2.0",
    id: 2,
    method: "tools/call",
    params: {
      name: "funds_investigate",
      arguments: args,
      _meta: { analytixRuntimeContext: authority }
    }
  });
  const blockedResponse = await waitFor(
    () => messages.find((message) => message.id === 2),
    "P0-quarantined funds response",
  );
  assert.deepEqual(blockedResponse.error, {
    code: -32602,
    message: "Unknown or unadvertised tool"
  });
  assert.equal(
    blockedResponse.result,
    undefined,
    "an unadvertised tool must not be normalized into an executable MCP result",
  );
  await new Promise((resolve) => setTimeout(resolve, 100));
  assert.equal(
    fs.existsSync(startedMarker),
    false,
    "P0-quarantined MCP entrypoint started nested DuckDB Python",
  );
  assert.equal(
    fs.existsSync(terminatedMarker),
    false,
    "a non-existent nested DuckDB process reported termination",
  );
  send({
    jsonrpc: "2.0",
    method: "notifications/cancelled",
    params: { requestId: 2, reason: "nested contract cancellation" }
  });
  send({ jsonrpc: "2.0", id: 3, method: "ping" });
  await waitFor(() => messages.find((message) => message.id === 3), "post-cancel ping");
  assert.deepEqual(messages.find((message) => message.id === 3)?.result, {});
  console.log("FundsMcpP0QuarantineRejectsUnadvertisedFrontdoorBeforeNestedDuckdbPython: passed");
  console.log("FundsMcpCancellationAfterBoundaryKeepsConnectionHealthy: passed");
} finally {
  child.stdin.end();
  await Promise.race([
    new Promise((resolve) => child.once("exit", resolve)),
    new Promise((resolve) => setTimeout(() => {
      child.kill("SIGTERM");
      resolve();
    }, 2_000))
  ]);
}
