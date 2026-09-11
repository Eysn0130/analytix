#!/usr/bin/env node

import assert from "node:assert/strict";
import path from "node:path";
import { PassThrough } from "node:stream";
import { fileURLToPath } from "node:url";
import { startJsonRpcStdioRuntime } from "../mcp/jsonrpc-stdio-runtime.mjs";
import { createMcpRequestHandlerRuntime } from "../mcp/mcp-request-handler-runtime.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");

function captureLines(stream) {
  let buffer = "";
  const lines = [];
  const waiters = [];
  stream.setEncoding("utf8");
  stream.on("data", (chunk) => {
    buffer += chunk;
    let newline = buffer.indexOf("\n");
    while (newline >= 0) {
      const line = buffer.slice(0, newline);
      buffer = buffer.slice(newline + 1);
      if (line) lines.push(JSON.parse(line));
      newline = buffer.indexOf("\n");
    }
    while (waiters.length && lines.length >= waiters[0].count) waiters.shift().resolve();
  });
  async function waitFor(count, timeoutMs = 2000) {
    if (lines.length >= count) return;
    let timer;
    await new Promise((resolve, reject) => {
      timer = setTimeout(() => reject(new Error(`timed out waiting for ${count} JSON-RPC responses; got ${lines.length}`)), timeoutMs);
      waiters.push({ count, resolve: () => {
        clearTimeout(timer);
        resolve();
      } });
    });
  }
  return { lines, waitFor };
}

function send(input, message) {
  const body = typeof message === "string" ? message : JSON.stringify(message);
  input.write(`${body}\n`);
}

function responseById(lines, id) {
  return lines.find((line) => line.id === id);
}

async function lifecycleContract() {
  const input = new PassThrough();
  const output = new PassThrough();
  const capture = captureLines(output);
  const handler = createMcpRequestHandlerRuntime({
    serverName: "analytix_funds",
    serverVersion: "0.17.0-test",
    pluginRoot,
    callTool: async () => ({ status: "blocked" }),
    toolResult: () => ({
      content: [],
      structuredContent: {
        transportStatus: "success", semanticStatus: "blocked", safeToAnswer: false, isError: true,
        blocker: "test", partialCoverage: {}, data: {}, candidateEvidenceReceipts: []
      },
      isError: true
    }),
    env: {}
  });
  const runtime = startJsonRpcStdioRuntime({ input, output, ...handler });

  send(input, { jsonrpc: "2.0", id: 1, method: "tools/list" });
  send(input, { jsonrpc: "2.0", id: 2, method: "ping" });
  send(input, `{"jsonrpc":"1.0","id":3,"method":"ping"}`);
  send(input, `{"jsonrpc":"1.0","jsonrpc":"2.0","id":4,"method":"ping"}`);
  send(input, `{"jsonrpc":"2.0","id":5,"method":"ping"`);
  await capture.waitFor(5);
  assert.equal(responseById(capture.lines, 1).error.code, -32600, "FundsMcpRejectsPreInitializeOperation");
  assert.deepEqual(responseById(capture.lines, 2).result, {}, "FundsMcpPingWorksInEveryPhase");
  assert.equal(capture.lines.filter((line) => line.id === null && line.error.code === -32600).length, 2, "FundsMcpStrictJsonRpcEnvelope");
  assert.equal(capture.lines.filter((line) => line.id === null && line.error.code === -32700).length, 1, "FundsMcpStrictJsonRpcEnvelope parse error");

  send(input, {
    jsonrpc: "2.0", id: 6, method: "initialize",
    params: { protocolVersion: "2025-11-25", capabilities: {}, clientInfo: { name: "lifecycle-contract", version: "1.0.0" } }
  });
  await capture.waitFor(6);
  assert.equal(responseById(capture.lines, 6).result.protocolVersion, "2025-11-25", "FundsMcpNegotiates2025_11_25And2025_06_18");
  send(input, { jsonrpc: "2.0", id: 7, method: "tools/list" });
  await capture.waitFor(7);
  assert.equal(responseById(capture.lines, 7).error.code, -32600, "FundsMcpRequiresInitializedNotification");
  send(input, { jsonrpc: "2.0", method: "notifications/initialized", params: {} });
  send(input, { jsonrpc: "2.0", id: 8, method: "tools/list" });
  await capture.waitFor(8);
  assert(Array.isArray(responseById(capture.lines, 8).result.tools), "initialized notification did not activate tools/list");
  send(input, { jsonrpc: "2.0", id: 9, method: "unsupported/method" });
  send(input, { jsonrpc: "2.0", id: 10, method: "tools/call", params: { name: "hidden_tool", arguments: {} } });
  send(input, {
    jsonrpc: "2.0", id: 11, method: "initialize",
    params: { protocolVersion: "2025-11-25", capabilities: {}, clientInfo: { name: "duplicate", version: "1.0.0" } }
  });
  await capture.waitFor(11);
  assert.equal(responseById(capture.lines, 9).error.code, -32601, "FundsMcpUnknownMethodIs32601");
  assert.equal(responseById(capture.lines, 10).error.code, -32602, "FundsMcpUnknownOrHiddenToolIs32602");
  assert.equal(responseById(capture.lines, 11).error.code, -32600, "FundsMcpDuplicateInitializeRejected");
  await runtime.close();
  input.destroy();
  output.destroy();
}

async function cancellationAndBoundsContract() {
  const input = new PassThrough();
  const output = new PassThrough();
  const capture = captureLines(output);
  const runtime = startJsonRpcStdioRuntime({
    input,
    output,
    limits: { maxConcurrentRequests: 1, maxQueuedRequests: 1, maxOutstandingRequestBytes: 1024, maxFrameBytes: 512 },
    handleRequest: async (message, { signal }) => {
      if (message.method === "ping") return {};
      await new Promise((resolve, reject) => {
        const timer = setTimeout(resolve, 500);
        signal.addEventListener("abort", () => {
          clearTimeout(timer);
          reject(signal.reason);
        }, { once: true });
      });
      return { late: true };
    },
    handleNotification: async (message, { cancelRequest }) => {
      if (message.method === "notifications/cancelled") cancelRequest(message.params?.requestId, message.params?.reason);
    }
  });
  send(input, { jsonrpc: "2.0", id: "slow", method: "slow" });
  send(input, { jsonrpc: "2.0", id: "queued", method: "slow" });
  send(input, { jsonrpc: "2.0", id: "busy", method: "slow" });
  await capture.waitFor(1);
  assert.equal(responseById(capture.lines, "busy").error.code, -32603, "FundsMcpConcurrencyAndQueueAreBounded");
  send(input, { jsonrpc: "2.0", method: "notifications/cancelled", params: { requestId: "slow", reason: "test" } });
  send(input, { jsonrpc: "2.0", method: "notifications/cancelled", params: { requestId: "queued", reason: "test" } });
  send(input, { jsonrpc: "2.0", id: "slow", method: "ping" });
  send(input, { jsonrpc: "2.0", id: "ping", method: "ping" });
  await capture.waitFor(3);
  assert.deepEqual(responseById(capture.lines, "ping").result, {});
  assert.equal(responseById(capture.lines, "slow").error.code, -32600, "CancelledRequestIdCannotBeReusedUntilOriginalSettles");
  assert.equal(responseById(capture.lines, "slow").result, undefined, "FundsMcpCancellationSuppressesLateResponse");
  assert.equal(responseById(capture.lines, "queued"), undefined, "cancelled queued request emitted a response");

  send(input, Buffer.alloc(513, 0x61).toString("utf8"));
  await capture.waitFor(4);
  assert.equal(capture.lines.at(-1).error.code, -32600, "FundsMcpOversizedFrameIsBounded");
  await runtime.close();
  input.destroy();
  output.destroy();
}

await lifecycleContract();
await cancellationAndBoundsContract();
console.log("mcp stdio lifecycle contract passed");
