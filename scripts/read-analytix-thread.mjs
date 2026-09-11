#!/usr/bin/env node

import { createHash } from "node:crypto";

const THREAD_ID_PATTERN = /^[A-Za-z0-9_-]{3,256}$/u;
const HIGH_RISK_AUTHORITY_KEYS = new Set([
  "acceptedfinal",
  "acceptedfinaldigest",
  "acceptedfinalview",
  "factfinalwitnessadmission",
  "publicationsnapshotproof",
  "publicationsnapshotproofdigest"
]);

function usage() {
  return [
    "Usage: node scripts/read-analytix-thread.mjs <thread-id-or-link> [--json] [--include-tools] [--runtime-url <url>]",
    "",
    "Reads the Go runtime public thread projection. Direct JSONL export is intentionally unsupported.",
    "ANALYTIX_RUNTIME_TOKEN is read from the environment; secrets are never accepted on argv.",
    "Tool output is limited to lifecycle metadata. Full PII requires a separately authorized controlled artifact.",
    "",
    "Examples:",
    "  ANALYTIX_RUNTIME_URL=http://127.0.0.1:8901 node scripts/read-analytix-thread.mjs thr_pievnjnx",
    "  node scripts/read-analytix-thread.mjs analytix://threads/thr_pievnjnx --json --runtime-url http://127.0.0.1:8901"
  ].join("\n");
}

function text(value) {
  return String(value == null ? "" : value).trim();
}

function parseArgs(argv) {
  const options = {
    input: "",
    json: false,
    includeTools: false,
    runtimeUrl: text(process.env.ANALYTIX_RUNTIME_URL)
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--json") {
      options.json = true;
      continue;
    }
    if (arg === "--include-tools") {
      options.includeTools = true;
      continue;
    }
    if (arg === "--runtime-url") {
      options.runtimeUrl = text(argv[index + 1]);
      index += 1;
      if (!options.runtimeUrl) throw new Error("Missing value for --runtime-url.");
      continue;
    }
    if (arg.startsWith("--runtime-url=")) {
      options.runtimeUrl = text(arg.slice("--runtime-url=".length));
      continue;
    }
    if (arg.startsWith("--")) throw new Error(`Unexpected argument: ${arg}`);
    if (!options.input) {
      options.input = arg;
      continue;
    }
    throw new Error(`Unexpected argument: ${arg}`);
  }
  if (!options.input) throw new Error("Missing thread id or thread link.");
  if (!options.runtimeUrl) throw new Error("Missing --runtime-url or ANALYTIX_RUNTIME_URL.");
  return options;
}

function threadIdFromInput(input) {
  const raw = text(input);
  if (THREAD_ID_PATTERN.test(raw)) return raw;
  if (/\b(?:events|messages|metadata)\.jsonl$/u.test(raw)) {
    throw new Error("Direct thread JSONL export is forbidden; use the authenticated Go runtime projection.");
  }
  try {
    const parsed = new URL(raw);
    if ((parsed.protocol === "codex:" || parsed.protocol === "analytix:") && parsed.hostname === "threads") {
      const id = decodeURIComponent(parsed.pathname.replace(/^\/+|\/+$/gu, "").split("/")[0] || "");
      if (THREAD_ID_PATTERN.test(id)) return id;
    }
  } catch {
    // Fall through to the bounded thread-id matcher.
  }
  const match = raw.match(/\bthr_[A-Za-z0-9_-]+\b/u);
  if (match && THREAD_ID_PATTERN.test(match[0])) return match[0];
  throw new Error(`Unable to parse analytix thread id from: ${raw}`);
}

function runtimeEndpoint(baseUrl, threadId) {
  const parsed = new URL(baseUrl);
  if (parsed.protocol !== "http:" || (parsed.hostname !== "127.0.0.1" && parsed.hostname !== "[::1]")) {
    throw new Error("Runtime URL must use http with a literal loopback address.");
  }
  if (parsed.username || parsed.password) {
    throw new Error("Runtime URL must not contain credentials.");
  }
  if (!parsed.port || !/^\d{1,5}$/u.test(parsed.port) || Number(parsed.port) < 1 || Number(parsed.port) > 65535) {
    throw new Error("Runtime URL must contain an explicit valid port.");
  }
  if (parsed.pathname !== "" && parsed.pathname !== "/") {
    throw new Error("Runtime URL must not contain a path.");
  }
  parsed.search = "";
  parsed.hash = "";
  parsed.pathname = `/v1/threads/${encodeURIComponent(threadId)}`;
  return parsed;
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function maskLongIdentifier(match) {
  const compact = match.replace(/[^0-9A-Za-z]/gu, "");
  const suffix = compact.slice(-4);
  return `<redacted-identifier:${suffix}>`;
}

function maskOrdinaryText(value) {
  return text(value)
    .replace(/\b[A-Z]{2}[0-9]{2}(?:[ -]?[A-Z0-9]){11,30}\b/giu, maskLongIdentifier)
    .replace(/(?<![0-9A-Za-z])(?:[0-9][ -]?){9,33}[0-9](?![0-9A-Za-z])/gu, maskLongIdentifier);
}

function safeIdentifier(value, fallback = "") {
  const normalized = text(value);
  return /^[A-Za-z0-9_.:/-]{1,256}$/u.test(normalized) ? normalized : fallback;
}

function normalizedKey(value) {
  return text(value).toLowerCase().replace(/[^a-z0-9]/gu, "");
}

function containsHighRiskAuthorityMarker(value, depth = 0) {
  if (depth > 64) return true;
  if (!value || typeof value !== "object") return false;
  if (Array.isArray(value)) {
    return value.some((entry) => containsHighRiskAuthorityMarker(entry, depth + 1));
  }
  return Object.entries(value).some(([key, entry]) =>
    HIGH_RISK_AUTHORITY_KEYS.has(normalizedKey(key)) ||
    containsHighRiskAuthorityMarker(entry, depth + 1)
  );
}

function publicTranscript(thread, includeTools, withholdUserText) {
  const transcript = [];
  let omittedAssistantCount = 0;
  for (const turn of Array.isArray(thread.turns) ? thread.turns : []) {
    if (!turn || !safeIdentifier(turn.id)) continue;
    for (const item of Array.isArray(turn.items) ? turn.items : []) {
      if (!withholdUserText && item?.kind === "user_message" && item.role === "user" && item.threadId === thread.id && item.turnId === turn.id) {
        transcript.push({ kind: "user", turnId: turn.id, text: maskOrdinaryText(item.displayText || item.text) });
        continue;
      }
      if (item?.kind === "assistant_text") {
        // Bearer authentication proves the client to the server, not the
        // server to this standalone process. Until a protected launcher pin
        // or equivalent process attestation is supplied independently of the
        // HTTP response, response-provided signing keys cannot authorize any
        // assistant text for export.
        omittedAssistantCount += 1;
        continue;
      }
      if (!includeTools || (item?.kind !== "tool_call" && item?.kind !== "tool_result")) continue;
      const toolName = safeIdentifier(item.toolName);
      const callId = safeIdentifier(item.callId);
      if (!toolName || !callId || item.threadId !== thread.id || item.turnId !== turn.id) continue;
      transcript.push({
        kind: item.kind,
        turnId: turn.id,
        toolName,
        callId,
        status: safeIdentifier(item.status),
        ...(item.kind === "tool_result" ? { isError: item.isError === true } : {})
      });
    }
  }
  return { transcript, omittedAssistantCount };
}

function projectThread(thread, includeTools) {
  if (!thread || typeof thread !== "object" || !THREAD_ID_PATTERN.test(text(thread.id))) {
    throw new Error("Runtime returned an invalid thread projection.");
  }
  const workspace = text(thread.workspace);
  const withholdUserText = containsHighRiskAuthorityMarker(thread);
  const { transcript, omittedAssistantCount } = publicTranscript(thread, includeTools, withholdUserText);
  return {
    schemaVersion: 2,
    source: "analytix-go-runtime-public-projection",
    threadId: thread.id,
    thread: {
      title: withholdUserText ? "(withheld)" : maskOrdinaryText(thread.title) || "(untitled)",
      workspaceHash: !withholdUserText && workspace ? sha256(workspace) : "",
      status: safeIdentifier(thread.status)
    },
    transcript,
    omittedAssistantCount
  };
}

async function readThread(options) {
  const threadId = threadIdFromInput(options.input);
  const endpoint = runtimeEndpoint(options.runtimeUrl, threadId);
  const token = text(process.env.ANALYTIX_RUNTIME_TOKEN);
  const headers = { accept: "application/json" };
  if (token) headers.authorization = `Bearer ${token}`;
  const response = await fetch(endpoint, {
    headers,
    redirect: "error",
    signal: AbortSignal.timeout(10_000)
  });
  if (!response.ok) throw new Error(`Runtime thread read failed with HTTP ${response.status}.`);
  const thread = await response.json();
  if (thread?.id !== threadId) throw new Error("Runtime returned a mismatched thread projection.");
  return projectThread(thread, options.includeTools);
}

function renderText(result, includeTools) {
  const lines = [
    `Thread: ${result.threadId}`,
    `Title: ${result.thread.title}`,
    `Workspace hash: ${result.thread.workspaceHash || "(unknown)"}`,
    ""
  ];
  for (const item of result.transcript) {
    if (item.kind === "user") {
      lines.push(`User: ${item.text}`);
    } else if (item.kind === "assistant") {
      lines.push(`Assistant: ${item.text}`);
    } else if (includeTools && item.kind === "tool_call") {
      lines.push(`Tool call: ${item.toolName} (${item.status || "unknown"})`);
    } else if (includeTools && item.kind === "tool_result") {
      lines.push(`Tool result: ${item.toolName} (${item.isError ? "error" : item.status || "completed"})`);
    }
  }
  if (result.omittedAssistantCount > 0) {
    lines.push("", `Omitted unaccepted assistant items: ${result.omittedAssistantCount}`);
  }
  return lines.join("\n");
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const result = await readThread(options);
  console.log(options.json ? JSON.stringify(result, null, 2) : renderText(result, options.includeTools));
}

main().catch((error) => {
  console.error(error?.message || String(error));
  console.error("");
  console.error(usage());
  process.exit(1);
});
