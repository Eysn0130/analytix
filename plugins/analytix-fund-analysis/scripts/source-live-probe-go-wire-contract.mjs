#!/usr/bin/env node

import fs from "node:fs";
import {
  CASE_SOURCE_PROBE_METHOD,
  createMcpRequestHandlerRuntime
} from "../mcp/mcp-request-handler-runtime.mjs";

export const SOURCE_LIVE_PROBE_GO_WIRE_CONTRACT_VERSION = "0.16.16";

const params = JSON.parse(fs.readFileSync(0, "utf8"));
const runtimeContext = params?._meta?.analytixRuntimeContext;
const serverVersion = SOURCE_LIVE_PROBE_GO_WIRE_CONTRACT_VERSION;
const handler = createMcpRequestHandlerRuntime({
  serverName: "analytix_funds",
  serverVersion,
  pluginRoot: new URL("..", import.meta.url).pathname,
  callTool: async () => {
    throw new Error("source probe must not invoke plugin-local execution");
  },
  toolResult: () => {
    throw new Error("source probe must not use provider-facing toolResult");
  },
  env: {}
});

await handler.handleRequest({
  method: "initialize",
  params: {
    protocolVersion: "2025-11-25",
    capabilities: {},
    clientInfo: { name: "go-wire-contract", version: "1.0.0" }
  }
});
await handler.handleNotification({ method: "notifications/initialized", params: {} });
const result = await handler.handleRequest({
  jsonrpc: "2.0",
  id: 1,
  method: CASE_SOURCE_PROBE_METHOD,
  params
});
process.stdout.write(`${JSON.stringify(result)}\n`);
