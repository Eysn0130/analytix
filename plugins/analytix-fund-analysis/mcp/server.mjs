#!/usr/bin/env node

import { startJsonRpcStdioRuntime } from "./jsonrpc-stdio-runtime.mjs";
import { createMcpRequestHandlerRuntime } from "./mcp-request-handler-runtime.mjs";

const SERVER_NAME = "analytix_funds";
const SERVER_VERSION = "0.16.16";

// The production entrypoint exposes the host-projected count canary and the
// provider-safe account-flow call shape plus fixed resources/prompts. Direct
// account-flow dispatch terminates at a host-authority-required boundary. This
// entrypoint deliberately does not import backend clients, DuckDB, Python,
// report writers, or any path-resolving runtime: the Go host owns the exact
// callback-scoped DSV2/native execution and evidence effects.
const { handleRequest, handleNotification, close } = createMcpRequestHandlerRuntime({
  serverName: SERVER_NAME,
  serverVersion: SERVER_VERSION
});

startJsonRpcStdioRuntime({ handleRequest, handleNotification, close });
