#!/usr/bin/env node

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { execFileSync, spawn } from "node:child_process";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import {
  buildPromptSuite,
  expectedAnalytixToolBudgetForTask,
  isReportTaskId,
  scoreAnswers,
  TASKS
} from "./model-ab-eval.mjs";
import {
  MARKETPLACE_NAME as HUB_MARKETPLACE,
  PLUGIN_CACHE_FILES,
  PLUGIN_NAME as FUND_PLUGIN_NAME,
  RUNTIME_SKILL_FILES,
} from "./runtime-cache-contract.mjs";

const require = createRequire(import.meta.url);
const { AgentRuntimeDirectClient } = require("../../../apps/desktop/src/main/agent-runtime-client");

const DEFAULT_BACKEND_URL = "http://127.0.0.1:18731";
const DEFAULT_CASE_ID = "be6a1df3d3c4";
const DEFAULT_CWD = "/Users/sun/Documents/New project";
const DEFAULT_WS_URL = "auto";
const FUND_PLUGIN_ID = `${FUND_PLUGIN_NAME}@${HUB_MARKETPLACE}`;

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(__dirname, "..");
const REPO_ROOT = path.resolve(__dirname, "../../..");

const DEFAULT_TASK_IDS = TASKS.map((task) => task.id);

const DEFAULT_MODES = ["plugin_disabled", "skill_only", "plugin_enabled_passive", "skill_mcp", "full_plugin"];
const PHASE4_FRONTDOOR_ALLOWED_TOOLS = new Set(["funds_investigate", "validate_report_claims"]);
const DEFAULT_MODE_LABELS = {
  plugin_disabled: "无插件 Codex",
  skill_only: "仅短 skill",
  skill_mcp: "skill + MCP diagnostic",
  full_plugin: "完整插件",
  plugin_enabled_passive: "插件开启-自然触发",
  plugin_enabled_commanded: "插件开启-命令入口",
};

function text(value) {
  return String(value == null ? "" : value).trim();
}

function redactAuthSensitiveText(value) {
  return text(value)
    .replace(/\b(Bearer\s+)[A-Za-z0-9._~+/=-]{16,}/giu, "$1[redacted]")
    .replace(/\bsk-[A-Za-z0-9_-]{16,}\b/gu, "[redacted-api-key]")
    .replace(/\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b/gu, "[redacted-jwt]")
    .replace(/\b((?:access|refresh|id|auth|session|api)[_-]?token|authorization|api[_-]?key)\s*[:=]\s*["']?[^"',\s}]+/giu, "$1=[redacted]");
}

function parseCsv(value) {
  return text(value)
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseRefreshRuns(value) {
  return new Set(parseCsv(value).map((item) => item.toLowerCase()));
}

function parseModelSpec(value) {
  const [model, provider, effort] = text(value).split(":");
  if (!model) return null;
  return {
    model,
    provider: provider || providerForModel(model),
    effort: effort || defaultEffortForModel(model),
  };
}

function modeUsesAnalytixMcp(mode) {
  return ["plugin_enabled_passive", "plugin_enabled_commanded", "skill_mcp", "full_plugin"].includes(mode);
}

function modeRequiresDirectDataCheck(mode) {
  return ["plugin_disabled", "skill_only"].includes(mode);
}

function turnTimeoutMsForMode(options, mode) {
  const baselineTimeoutMs = Number(options.baselineTimeoutMs || 0);
  if (modeRequiresDirectDataCheck(mode) && baselineTimeoutMs > 0) {
    return baselineTimeoutMs;
  }
  return Number(options.timeoutMs || 0);
}

function providerForModel(model) {
  if (/^deepseek/i.test(model)) return "deepseek";
  if (/^qwen/i.test(model)) return "dashscope";
  return "openai";
}

function defaultEffortForModel(model) {
  return /^deepseek/i.test(model) ? "xhigh" : "xhigh";
}

function parseArgs(argv) {
  const options = {
    backendUrl: DEFAULT_BACKEND_URL,
    caseId: DEFAULT_CASE_ID,
    caseDb: text(process.env.ANALYTIX_FUNDS_ORACLE_CASE_DB || process.env.ANALYTIX_FUNDS_CASE_DB || process.env.ANALYTIX_CASE_DB),
    cwd: DEFAULT_CWD,
    wsUrl: DEFAULT_WS_URL,
    taskIds: DEFAULT_TASK_IDS,
    modes: DEFAULT_MODES,
    models: [parseModelSpec("gpt-5.5:openai:xhigh")],
    timeoutMs: 600_000,
    baselineTimeoutMs: 0,
    pollMs: 5_000,
    output: "",
    resume: false,
    maxNewRuns: 0,
    refreshRuns: new Set(),
    selfTestResume: false,
    reinstallAtEnd: true,
    json: false,
    allowInvalidRuns: false,
    phase4Profile: "default",
    phase4OracleSnapshot: "",
    phase4OracleCards: {},
    spawnAppServer: false,
    appServerBinaryPath: "",
    appServerBinaryArgs: ["app-server"],
    appServerCodexHome: "",
    appServerReadyTimeoutMs: 90_000,
    spawnBackend: false,
    backendLog: "",
    backendReadyTimeoutMs: 30_000,
    backendKeepAliveMs: 20_000,
    backendAutoRestart: true,
    backendRestartTimer: null,
    managedBackendStartPromise: null,
    authPreflight: true,
    authPreflightTimeoutMs: 60_000,
    sandboxMode: "read-only",
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--backend-url") options.backendUrl = next();
    else if (arg === "--case-id") options.caseId = next();
    else if (arg === "--case-db") options.caseDb = next();
    else if (arg === "--cwd") options.cwd = next();
    else if (arg === "--ws-url") options.wsUrl = next();
    else if (arg === "--tasks") options.taskIds = parseCsv(next());
    else if (arg === "--modes") options.modes = parseCsv(next());
    else if (arg === "--model") options.models.push(parseModelSpec(next()));
    else if (arg === "--models") options.models = parseCsv(next()).map(parseModelSpec).filter(Boolean);
    else if (arg === "--timeout-ms") options.timeoutMs = Number(next()) || options.timeoutMs;
    else if (arg === "--baseline-timeout-ms") options.baselineTimeoutMs = Number(next()) || options.baselineTimeoutMs;
    else if (arg === "--poll-ms") options.pollMs = Number(next()) || options.pollMs;
    else if (arg === "--output") options.output = next();
    else if (arg === "--resume") options.resume = true;
    else if (arg === "--max-new-runs") options.maxNewRuns = Math.max(0, Math.trunc(Number(next()) || 0));
    else if (arg === "--refresh-runs") options.refreshRuns = parseRefreshRuns(next());
    else if (arg === "--self-test-resume") options.selfTestResume = true;
    else if (arg === "--no-reinstall-at-end") options.reinstallAtEnd = false;
    else if (arg === "--allow-invalid-runs") options.allowInvalidRuns = true;
    else if (arg === "--phase4-profile") options.phase4Profile = next() || "default";
    else if (arg === "--phase4-oracle-snapshot") options.phase4OracleSnapshot = next();
    else if (arg === "--spawn-app-server") options.spawnAppServer = true;
    else if (arg === "--app-server-binary") options.appServerBinaryPath = next();
    else if (arg === "--app-server-args") options.appServerBinaryArgs = parseCsv(next());
    else if (arg === "--app-server-codex-home") options.appServerCodexHome = next();
    else if (arg === "--app-server-ready-timeout-ms") options.appServerReadyTimeoutMs = Number(next()) || options.appServerReadyTimeoutMs;
    else if (arg === "--spawn-backend") options.spawnBackend = true;
    else if (arg === "--backend-log") options.backendLog = next();
    else if (arg === "--backend-ready-timeout-ms") options.backendReadyTimeoutMs = Number(next()) || options.backendReadyTimeoutMs;
    else if (arg === "--backend-keepalive-ms") options.backendKeepAliveMs = Number(next()) || options.backendKeepAliveMs;
    else if (arg === "--skip-auth-preflight") options.authPreflight = false;
    else if (arg === "--auth-preflight-timeout-ms") options.authPreflightTimeoutMs = Number(next()) || options.authPreflightTimeoutMs;
    else if (arg === "--sandbox-mode") options.sandboxMode = next();
    else if (arg === "--json") options.json = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  options.backendUrl = text(options.backendUrl).replace(/\/+$/u, "");
  options.wsUrl = resolveWsUrl(options.wsUrl);
  options.models = options.models.filter(Boolean);
  if (!options.models.length) {
    throw new Error("at least one --model is required");
  }
  if (!options.output) {
    const stamp = new Date().toISOString().replace(/[:.]/gu, "-");
    options.output = path.join(REPO_ROOT, "output", "analytix-fund-analysis", "diagnostics", "agent-ui-ab", `agent-ui-ab-${stamp}.json`);
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/agent-ui-ab-run.mjs [options]

Runs diagnostic analytixagent UI/runtime turns with and without the Analytix fund plugin.
This runner is diagnostic only; release readiness is controlled by functional closure evidence.

Options:
  --ws-url <url|auto>       Agent app-server WebSocket URL. Default: ${DEFAULT_WS_URL}
  --spawn-app-server        Start the Analytix-owned analytix-agent app-server for this run.
  --app-server-binary <path>
                            Binary used with --spawn-app-server. Default: analytixagent/src/<platform>/analytix-agent.
  --app-server-args <csv>   App-server args used with --spawn-app-server. Default: app-server.
  --app-server-codex-home <path>
                            Runtime home used with --spawn-app-server. Default: Analytix agent runtime home.
  --app-server-ready-timeout-ms <n>
                            App-server health readiness timeout. Default: 90000.
  --skip-auth-preflight    Skip the default tiny model-auth preflight before quality tasks.
  --auth-preflight-timeout-ms <n>
                            Auth preflight timeout. Default: 60000.
  --sandbox-mode <mode>     Eval turn sandbox: read-only or danger-full-access. Default: read-only.
  --backend-url <url>       Analytix backend URL. Default: ${DEFAULT_BACKEND_URL}
  --spawn-backend           Start a local Analytix backend for this eval run and stop it at exit.
  --backend-log <file>      Log file for --spawn-backend.
  --backend-ready-timeout-ms <n>
                            Backend readiness timeout. Default: 30000.
  --backend-keepalive-ms <n>
                            Backend health ping interval for --spawn-backend. Default: 20000.
  --case-id <id>            Case id. Default: ${DEFAULT_CASE_ID}
  --case-db <path>          Explicit DuckDB path for legacy direct-data baselines. Default: ANALYTIX_FUNDS_ORACLE_CASE_DB / ANALYTIX_FUNDS_CASE_DB / ANALYTIX_CASE_DB.
  --cwd <path>              Thread cwd. Default: ${DEFAULT_CWD}
  --tasks <ids>             Comma-separated model-ab-eval task ids.
  --modes <ids>             Comma-separated modes. Default: ${DEFAULT_MODES.join(",")}
  --models <specs>          Comma-separated model:provider:effort specs.
  --model <spec>            Add one model:provider:effort spec.
  --timeout-ms <n>          Turn timeout. Default: 600000.
  --baseline-timeout-ms <n> Turn timeout only for plugin_disabled/skill_only baselines. Default: same as --timeout-ms.
  --output <file>           Output JSON file.
  --resume                  Reuse existing records from --output and skip completed model/task/mode turns.
  --max-new-runs <n>        Stop after n new turns so long 10x5 runs can be resumed in batches.
  --refresh-runs <task:mode[,task:mode]>
                            With --resume, drop selected completed rows before skipping so one row can be rerun without losing the full matrix.
  --self-test-resume        Run deterministic resume/checkpoint tests without starting app-server.
  --allow-invalid-runs      Continue after empty/error turns. Default aborts to avoid scoring auth/runtime failures.
  --phase4-profile <name>   Diagnostic profile label, e.g. frontdoor_report_only or default.
  --phase4-oracle-snapshot <file>
                            Inject A0 rendered oracle cards by task for readability-only runs.
  --json                    Print JSON result.
`);
}

function resolveWsUrl(value) {
  const normalized = text(value) || DEFAULT_WS_URL;
  if (normalized !== "auto") return normalized;
  const fromEnv = text(process.env.ANALYTIX_AGENT_APP_SERVER_WS_URL || process.env.ANALYTIX_CODEX_APP_SERVER_WS_URL);
  if (fromEnv) return fromEnv;
  try {
    const output = execFileSync("lsof", ["-nP", "-iTCP", "-sTCP:LISTEN"], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    });
    const ports = output
      .split(/\r?\n/u)
      .filter((line) => /^analytix-/u.test(line) && line.includes("127.0.0.1:"))
      .map((line) => {
        const match = line.match(/127\.0\.0\.1:(\d+)\s+\(LISTEN\)/u);
        return match ? Number(match[1]) : 0;
      })
      .filter(Boolean);
    if (ports.length) {
      return `ws://127.0.0.1:${ports.at(-1)}`;
    }
  } catch {
    // Fall through to the historical development port.
  }
  return "ws://127.0.0.1:54386";
}

function normalizedSandboxMode(value) {
  const mode = text(value).toLowerCase();
  if (mode === "danger-full-access" || mode === "dangerfullaccess" || mode === "full-access") {
    return "danger-full-access";
  }
  return "read-only";
}

function threadSandboxMode(options = {}) {
  return normalizedSandboxMode(options.sandboxMode);
}

function turnSandboxPolicy(options = {}) {
  return threadSandboxMode(options) === "danger-full-access"
    ? { type: "dangerFullAccess" }
    : { type: "readOnly", networkAccess: false };
}

function codexDesktopPlatformKey(platform = process.platform, arch = process.arch) {
  if (platform === "darwin") {
    return arch === "x64" ? "mac-x64" : "mac-arm64";
  }
  if (platform === "win32") {
    return "win";
  }
  if (platform === "linux") {
    return arch === "arm64" ? "linux-arm64" : "linux-x64";
  }
  return `${platform}-${arch}`;
}

function defaultAppServerBinaryPath() {
  const fromEnv = text(process.env.ANALYTIX_AGENT_APP_SERVER_BINARY || process.env.ANALYTIX_CODEX_APP_SERVER_BINARY);
  if (fromEnv) return fromEnv;
  return path.join(
    REPO_ROOT,
    "analytixagent",
    "src",
    codexDesktopPlatformKey(),
    process.platform === "win32" ? "analytix-agent.exe" : "analytix-agent"
  );
}

function shellQuote(value) {
  return `'${String(value).replace(/'/gu, "'\\''")}'`;
}

function localBackendEndpoint(options) {
  let parsed;
  try {
    parsed = new URL(options.backendUrl);
  } catch {
    throw new Error(`--spawn-backend requires an absolute --backend-url, got ${options.backendUrl}`);
  }
  const host = parsed.hostname;
  if (!["127.0.0.1", "localhost", "::1"].includes(host)) {
    throw new Error(`--spawn-backend only supports localhost backend URLs, got ${options.backendUrl}`);
  }
  const port = Number(parsed.port || (parsed.protocol === "https:" ? 443 : 80));
  if (!Number.isFinite(port) || port <= 0) {
    throw new Error(`--spawn-backend cannot determine backend port from ${options.backendUrl}`);
  }
  return {
    host: host === "::1" ? "::1" : "127.0.0.1",
    port,
  };
}

async function backendHealthOk(backendUrl, timeoutMs = 2_000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(`${backendUrl}/health`, { signal: controller.signal });
    return response.ok;
  } catch {
    return false;
  } finally {
    clearTimeout(timer);
  }
}

function tailFileSafe(file, maxChars = 4000) {
  try {
    const raw = fs.readFileSync(file, "utf8");
    return raw.slice(Math.max(0, raw.length - maxChars));
  } catch {
    return "";
  }
}

async function waitForBackendReady(options, handle) {
  const deadline = Date.now() + Math.max(1_000, options.backendReadyTimeoutMs);
  while (Date.now() < deadline) {
    if (await backendHealthOk(options.backendUrl)) {
      return true;
    }
    if (handle?.exited) {
      break;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  return false;
}

async function startManagedBackend(options) {
  if (!options.spawnBackend) return null;
  if (await backendHealthOk(options.backendUrl)) {
    return { owned: false, status: "already_running" };
  }
  const endpoint = localBackendEndpoint(options);
  const logPath = path.resolve(
    text(options.backendLog)
      || path.join(REPO_ROOT, "output", "analytix-fund-analysis", "diagnostics", "agent-ui-ab", `agent-ui-ab-backend-${endpoint.port}.log`)
  );
  fs.mkdirSync(path.dirname(logPath), { recursive: true });
  const output = fs.openSync(logPath, "a");
  const pythonBin = path.join(REPO_ROOT, ".venv", "bin", "python");
  const backendDir = path.join(REPO_ROOT, "backend");
  const script = [
    "set -euo pipefail",
    `cd ${shellQuote(REPO_ROOT)}`,
    "set -a",
    '[ -f .env ] && source .env || true',
    '[ -f .env.local ] && source .env.local || true',
    '[ -f .env.desktop.local ] && source .env.desktop.local || true',
    "set +a",
    `cd ${shellQuote(backendDir)}`,
    `exec ${shellQuote(pythonBin)} -m uvicorn app.main:app --host ${shellQuote(endpoint.host)} --port ${endpoint.port}`,
  ].join("\n");
  const child = spawn("bash", ["-lc", script], {
    cwd: REPO_ROOT,
    env: { ...process.env },
    stdio: ["ignore", output, output],
  });
  const handle = {
    owned: true,
    child,
    logPath,
    output,
    stopping: false,
    exited: false,
    exitCode: null,
  };
  child.once("exit", (code, signal) => {
    handle.exited = true;
    handle.exitCode = signal || code;
    if (handle.output != null) {
      try {
        fs.closeSync(handle.output);
      } catch {
        // Best effort cleanup for eval infrastructure only.
      }
      handle.output = null;
    }
    if (options.backendAutoRestart && !handle.stopping && !options.backendStopping) {
      options.backendRestartTimer = setTimeout(() => {
        options.backendRestartTimer = null;
        if (options.backendStopping) return;
        ensureManagedBackendReady(options)
          .catch((error) => {
            console.error(`[agent-ui-ab] managed backend autorestart failed: ${text(error?.message || error)}`);
          });
      }, 250);
      options.backendRestartTimer.unref?.();
    }
  });
  const ready = await waitForBackendReady(options, handle);
  if (!ready) {
    await stopManagedBackend(handle);
    const tail = tailFileSafe(logPath);
    throw new Error(`managed backend did not become ready at ${options.backendUrl}; log=${logPath}\n${tail}`);
  }
  console.error(`[agent-ui-ab] managed backend ready ${options.backendUrl} log=${logPath}`);
  return handle;
}

async function ensureManagedBackendReady(options) {
  if (!options.spawnBackend) return null;
  if (options.managedBackendStartPromise) {
    return await options.managedBackendStartPromise;
  }
  if (await backendHealthOk(options.backendUrl)) {
    return options.managedBackend || { owned: false, status: "already_running" };
  }
  options.managedBackendStartPromise = (async () => {
    try {
      if (await backendHealthOk(options.backendUrl)) {
        return options.managedBackend || { owned: false, status: "already_running" };
      }
      if (options.managedBackend?.owned) {
        await stopManagedBackend(options.managedBackend);
      }
      options.managedBackend = await startManagedBackend(options);
      return options.managedBackend;
    } finally {
      options.managedBackendStartPromise = null;
    }
  })();
  return await options.managedBackendStartPromise;
}

function startManagedBackendKeepAlive(options) {
  if (!options.spawnBackend || options.backendKeepAliveTimer || options.backendKeepAliveMs <= 0) return;
  options.backendKeepAliveTimer = setInterval(() => {
    ensureManagedBackendReady(options).catch((error) => {
      console.error(`[agent-ui-ab] managed backend keepalive failed: ${text(error?.message || error)}`);
    });
  }, Math.max(1_000, options.backendKeepAliveMs));
  options.backendKeepAliveTimer.unref?.();
}

async function stopManagedBackend(handle) {
  if (!handle?.owned) return;
  const child = handle.child;
  if (child && !handle.exited) {
    handle.stopping = true;
    child.kill("SIGTERM");
    await new Promise((resolve) => {
      const timer = setTimeout(() => {
        if (!handle.exited) child.kill("SIGKILL");
        resolve();
      }, 5_000);
      child.once("exit", () => {
        clearTimeout(timer);
        resolve();
      });
    });
  }
  if (handle.output != null) {
    try {
      fs.closeSync(handle.output);
    } catch {
      // Best effort cleanup for eval infrastructure only.
    }
  }
}

async function stopManagedBackendRuntime(options) {
  if (options) options.backendStopping = true;
  if (options?.backendKeepAliveTimer) {
    clearInterval(options.backendKeepAliveTimer);
    options.backendKeepAliveTimer = null;
  }
  if (options?.backendRestartTimer) {
    clearTimeout(options.backendRestartTimer);
    options.backendRestartTimer = null;
  }
  await stopManagedBackend(options?.managedBackend);
  if (options) options.managedBackend = null;
}

async function postJson(url, body, timeoutMs) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body || {}),
      signal: controller.signal,
    });
    const raw = await response.text();
    if (!response.ok) {
      throw new Error(`${response.status} ${response.statusText}: ${raw}`);
    }
    return raw ? JSON.parse(raw) : null;
  } finally {
    clearTimeout(timer);
  }
}

async function activateCase(options) {
  const caseId = text(options.caseId);
  const activate = async () => await postJson(
    `${options.backendUrl}/api/v1/cases/${encodeURIComponent(caseId)}/activate`,
    {},
    Math.min(options.timeoutMs, 60_000)
  );
  await ensureManagedBackendReady(options);
  try {
    return await activate();
  } catch (error) {
    if (!options.spawnBackend || !/fetch failed|ECONNREFUSED|ECONNRESET|AbortError/iu.test(text(error?.message || error))) {
      throw error;
    }
    console.error(`[agent-ui-ab] backend activate failed; restarting managed backend: ${text(error?.message || error)}`);
    if (options.managedBackend?.owned) {
      await stopManagedBackend(options.managedBackend);
    }
    options.managedBackend = await startManagedBackend(options);
    return await activate();
  }
}

function createClient(options) {
  options.runtimeLogs = [];
  const processCwd = clientProcessCwd(options);
  const shellPwdKey = ["P", "WD"].join("");
  const recordRuntimeLog = (...args) => {
    const line = redactAuthSensitiveText(args.map((item) => text(item)).filter(Boolean).join(" "));
    if (line) {
      options.runtimeLogs.push(line);
      if (options.runtimeLogs.length > 200) {
        options.runtimeLogs.splice(0, options.runtimeLogs.length - 200);
      }
    }
    return line;
  };
  const clientOptions = {
    cwd: processCwd,
    codexHome: options.spawnAppServer
      ? path.resolve(text(options.appServerCodexHome) || analytixRuntimeHome())
      : undefined,
    binaryPath: options.spawnAppServer
      ? path.resolve(text(options.appServerBinaryPath) || defaultAppServerBinaryPath())
      : undefined,
    binaryArgs: options.spawnAppServer ? options.appServerBinaryArgs : undefined,
    env: {
      ...process.env,
      ANALYTIX_API_BASE_URL: options.backendUrl,
      ANALYTIX_DISABLE_TELEMETRY: "1",
      CODEX_DISABLE_ANALYTICS: "1",
      CODEX_DISABLE_TELEMETRY: "1",
      DO_NOT_TRACK: "1",
      [shellPwdKey]: processCwd,
      INIT_CWD: processCwd,
      OTEL_SDK_DISABLED: "true",
      SENTRY_DSN: "",
    },
    requestTimeoutMs: options.timeoutMs,
    readyTimeoutMs: Math.max(1_000, Number(options.appServerReadyTimeoutMs || 90_000)),
    logger: {
      log() {},
      info() {},
      warn(...args) {
        const line = recordRuntimeLog(...args);
        if (line) console.warn(line);
      },
      error(...args) {
        const line = recordRuntimeLog(...args);
        if (line) console.error(line);
      },
    },
  };
  const client = new AgentRuntimeDirectClient(clientOptions);
  if (!options.spawnAppServer) {
    client.wsUrl = options.wsUrl;
    const healthUrl = options.wsUrl.replace(/^ws:/u, "http:").replace(/^wss:/u, "https:").replace(/\/+$/u, "");
    client.healthUrl = `${healthUrl}/healthz`;
  }
  return client;
}

function clientProcessCwd(options) {
  const candidate = path.resolve(text(options.cwd) || REPO_ROOT);
  try {
    if (fs.existsSync(candidate) && fs.statSync(candidate).isDirectory()) {
      return candidate;
    }
  } catch {
    // Fall back to the repo root only when the requested eval cwd is invalid.
  }
  return REPO_ROOT;
}

async function connectClient(client) {
  if (client.binaryPath) {
    if (!fs.existsSync(client.binaryPath) || !fs.statSync(client.binaryPath).isFile()) {
      throw new Error(`agent app-server binary not found: ${client.binaryPath}; pass --app-server-binary or set ANALYTIX_AGENT_APP_SERVER_BINARY`);
    }
    await client.start();
    return client;
  }
  await client.connectWebSocket();
  await client.initialize();
  client.ready = true;
  return client;
}

async function stopClientRuntime(client) {
  if (!client) return;
  const proc = client.proc;
  client.stop();
  if (!proc || typeof proc.once !== "function") return;
  if (proc.exitCode !== null || proc.signalCode) return;
  await new Promise((resolve) => {
    let settled = false;
    const finish = () => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve();
    };
    const timer = setTimeout(() => {
      try {
        proc.kill("SIGKILL");
      } catch {
        // Best effort cleanup for eval-owned app-server processes only.
      }
      finish();
    }, 3_000);
    proc.once("exit", finish);
  });
}

async function ensurePluginState(client, enabled) {
  if (enabled) {
    const list = await client.pluginList({});
    const entry = findFundPluginEntry(list);
    if (entry?.installed && entry?.enabled) {
      return { status: "already_installed", plugin_id: entry.id, source: entry.source };
    }
    return await client.pluginInstall({
      pluginName: FUND_PLUGIN_NAME,
      marketplacePath: entry?.marketplacePath || defaultHubMarketplacePath(),
    });
  }
  try {
    return await client.pluginUninstall({ pluginId: FUND_PLUGIN_ID });
  } catch (error) {
    return { status: "ignored", error: text(error?.message || error) };
  }
}

function defaultHubMarketplacePath() {
  return path.join(
    defaultAnalytixRuntimeHome(),
    ".cache",
    "analytix-hub-plugins",
    "marketplaces",
    HUB_MARKETPLACE,
    ".agents",
    "plugins",
    "marketplace.json"
  );
}

function defaultAnalytixRuntimeHome() {
  return path.join(os.homedir(), ".analytix");
}

function analytixRuntimeHome() {
  const explicit = text(process.env.ANALYTIX_AGENT_RUNTIME_HOME);
  if (explicit) return explicit;
  const codexHome = text(process.env.CODEX_HOME);
  if (/Analytix/u.test(codexHome)) return codexHome;
  return defaultAnalytixRuntimeHome();
}

function sameFileText(leftPath, rightPath) {
  if (!fs.existsSync(leftPath) || !fs.existsSync(rightPath)) return false;
  return fs.readFileSync(leftPath, "utf8") === fs.readFileSync(rightPath, "utf8");
}

function pluginManifest() {
  return JSON.parse(fs.readFileSync(path.join(PLUGIN_ROOT, ".codex-plugin", "plugin.json"), "utf8"));
}

function gitOutput(args) {
  try {
    return execFileSync("git", args, {
      cwd: REPO_ROOT,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
  } catch {
    return "";
  }
}

function repoRelativePath(filePath) {
  const resolved = path.resolve(text(filePath) || ".");
  return path.relative(REPO_ROOT, resolved).replace(/\\/gu, "/");
}

function gitStatusPorcelainLines() {
  try {
    return execFileSync("git", ["status", "--porcelain=v1", "--untracked-files=all"], {
      cwd: REPO_ROOT,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).split(/\r?\n/u).filter((line) => line.trim());
  } catch {
    return [];
  }
}

function porcelainStatusPath(line) {
  const raw = String(line == null ? "" : line).slice(3);
  const renamedIndex = raw.lastIndexOf(" -> ");
  const value = renamedIndex >= 0 ? raw.slice(renamedIndex + 4) : raw;
  return value.replace(/^"|"$/gu, "");
}

function isAllowedEvalOutputStatus(line, options) {
  const outputRel = repoRelativePath(options.output);
  const statusPath = porcelainStatusPath(line);
  return statusPath === outputRel || statusPath.startsWith(`${outputRel}.`);
}

function worktreeMutationGuardEnabled(options) {
  return threadSandboxMode(options) === "read-only";
}

function worktreeMutationLines(options) {
  if (!worktreeMutationGuardEnabled(options)) return [];
  return gitStatusPorcelainLines().filter((line) => !isAllowedEvalOutputStatus(line, options));
}

function worktreeMutationFailure(options, phase) {
  const lines = worktreeMutationLines(options);
  if (!lines.length) return null;
  const files = lines.map((line) => porcelainStatusPath(line)).filter(Boolean);
  const preview = files.slice(0, 8).join(", ");
  const suffix = files.length > 8 ? ", ..." : "";
  const reason = `read-only eval ${phase} detected worktree mutation outside output artifact: ${preview}${suffix}`;
  return {
    reason,
    status: "worktree_mutation_detected",
    route_violations: [reason],
    changed_files: files,
  };
}

function serverVersion() {
  const source = fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", "server.mjs"), "utf8");
  const match = source.match(/SERVER_VERSION\s*=\s*"([^"]+)"/u);
  return match ? match[1] : "";
}

function releaseIdentity() {
  const manifestVersion = text(pluginManifest().version);
  const tag = manifestVersion ? `analytix-fund-analysis-v${manifestVersion}` : "";
  const tagsAtHead = gitOutput(["tag", "--points-at", "HEAD"]).split(/\r?\n/u).map(text).filter(Boolean);
  return {
    manifest_version: manifestVersion,
    server_version: serverVersion(),
    head: gitOutput(["rev-parse", "--short", "HEAD"]),
    tags_at_head: tagsAtHead,
    release_tag: tag,
    release_tag_at_head: Boolean(tag && tagsAtHead.includes(tag)),
  };
}

function installedPluginRoots(runtimeHome, version) {
  const roots = [];
  const legacyRoot = path.join(runtimeHome, "plugins", "cache", HUB_MARKETPLACE, FUND_PLUGIN_NAME, version);
  if (fs.existsSync(legacyRoot)) {
    roots.push(legacyRoot);
  }
  const marketplacePluginRoot = path.join(
    runtimeHome,
    ".cache",
    `${HUB_MARKETPLACE}-plugins`,
    "marketplaces",
    HUB_MARKETPLACE,
    "plugins",
    FUND_PLUGIN_NAME
  );
  if (fs.existsSync(marketplacePluginRoot)) {
    for (const entry of fs.readdirSync(marketplacePluginRoot, { withFileTypes: true })) {
      if (!entry.isDirectory()) continue;
      if (entry.name === version || entry.name.startsWith(`${version}-`)) {
        roots.push(path.join(marketplacePluginRoot, entry.name));
      }
    }
  }
  return [...new Set(roots.map((item) => path.resolve(item)))];
}

function runtimeCachePreflight(options) {
  if (!options.modes.some(modeUsesAnalytixMcp) || options.phase4OracleSnapshot) return null;
  const manifest = pluginManifest();
  const version = text(manifest.version);
  const runtimeHome = analytixRuntimeHome();
  const expectedLegacyRoot = path.join(runtimeHome, "plugins", "cache", HUB_MARKETPLACE, FUND_PLUGIN_NAME, version);
  const installedRoots = version ? installedPluginRoots(runtimeHome, version) : [];
  const installedRoot = installedRoots[0];
  if (!version || !installedRoot) {
    return {
      status: "runtime_cache_missing",
      reason: `Analytix-owned runtime cache missing ${FUND_PLUGIN_NAME}@${version || "unknown"} at ${expectedLegacyRoot} or Hub hashed marketplace cache`,
      route_violations: ["plugin quality run cannot start without installed Analytix-owned plugin cache"],
    };
  }
  const driftFiles = PLUGIN_CACHE_FILES.filter((relativePath) =>
    !sameFileText(path.join(PLUGIN_ROOT, relativePath), path.join(installedRoot, relativePath))
  );
  const runtimeSkillDrift = RUNTIME_SKILL_FILES.filter((relativePath) =>
    !sameFileText(
      path.join(PLUGIN_ROOT, "skills", FUND_PLUGIN_NAME, relativePath),
      path.join(runtimeHome, "skills", FUND_PLUGIN_NAME, relativePath)
    )
  );
  if (!driftFiles.length && !runtimeSkillDrift.length) return null;
  const detail = [
    driftFiles.length ? `cache differs from workspace: ${driftFiles.slice(0, 8).join(", ")}${driftFiles.length > 8 ? ", ..." : ""}` : "",
    runtimeSkillDrift.length ? `runtime required skill differs from workspace: ${runtimeSkillDrift.slice(0, 8).join(", ")}${runtimeSkillDrift.length > 8 ? ", ..." : ""}` : "",
  ].filter(Boolean).join("; ");
  return {
    status: "runtime_cache_drift",
    reason: `${detail}; sync/reinstall Analytix-owned runtime cache before legacy Agent UI diagnostic or functional closure run`,
    route_violations: ["plugin quality run would use stale Analytix-owned runtime cache"],
  };
}

function runtimeCachePreflightPassed(options) {
  const required = options.modes.some(modeUsesAnalytixMcp) && !options.phase4OracleSnapshot;
  if (!required) {
    return {
      status: "not_required",
      required: false,
    };
  }
  const manifest = pluginManifest();
  const version = text(manifest.version);
  const runtimeHome = analytixRuntimeHome();
  return {
    status: "passed",
    required: true,
    plugin_name: FUND_PLUGIN_NAME,
    plugin_version: version,
    runtime_home: runtimeHome,
    installed_plugin_roots: installedPluginRoots(runtimeHome, version),
  };
}

function findFundPluginEntry(list) {
  const marketplaces = Array.isArray(list?.marketplaces) ? list.marketplaces : [];
  for (const marketplace of marketplaces) {
    const marketplacePath = text(marketplace?.path);
    const plugins = Array.isArray(marketplace?.plugins) ? marketplace.plugins : [];
    const plugin = plugins.find((item) => item?.id === FUND_PLUGIN_ID || item?.name === FUND_PLUGIN_NAME);
    if (plugin) {
      return { ...plugin, marketplacePath };
    }
  }
  return null;
}

function threadFromResult(result) {
  return result?.thread || result?.data?.thread || result?.data || result || {};
}

function turnsFromThreadRead(result) {
  const thread = threadFromResult(result);
  return Array.isArray(thread.turns) ? thread.turns : [];
}

function allItemsFromThreadRead(result) {
  return turnsFromThreadRead(result).flatMap((turn) => Array.isArray(turn.items) ? turn.items : []);
}

function finalAnswerFromThreadRead(result) {
  const items = allItemsFromThreadRead(result);
  const final = [...items].reverse().find((item) => item?.type === "agentMessage" && item?.phase === "final_answer");
  if (final) return text(final.text || final.content);
  const any = [...items].reverse().find((item) => item?.type === "agentMessage");
  return text(any?.text || any?.content);
}

function compactToolInput(item) {
  const input = item?.arguments || item?.args || item?.input || item?.params || item?.rawInput || null;
  if (!input || typeof input !== "object") return input ? text(input).slice(0, 500) : null;
  const json = JSON.stringify(input, (_key, value) => {
    if (typeof value === "string" && value.length > 500) return `${value.slice(0, 500)}...`;
    return value;
  });
  return json.length > 1500 ? `${json.slice(0, 1500)}...` : JSON.parse(json);
}

function toolCallsFromThreadRead(result) {
  return allItemsFromThreadRead(result)
    .filter((item) => item?.type === "mcpToolCall")
    .map((item, index) => ({
      index,
      server: text(item.server),
      tool: text(item.tool),
      status: text(item.status),
      duration_ms: Number(item.durationMs || 0),
      input: compactToolInput(item),
      result_text_chars: Array.isArray(item.result?.content)
        ? item.result.content.reduce((sum, part) => sum + text(part?.text).length, 0)
        : 0,
    }));
}

function toolInputText(input, key) {
  if (!input || typeof input !== "object") return "";
  return text(input[key] || input.arguments?.[key] || input.input?.[key]);
}

function toolInputArray(input, key) {
  if (!input || typeof input !== "object") return [];
  const value = input[key] || input.arguments?.[key] || input.input?.[key];
  return Array.isArray(value) ? value.map(text).filter(Boolean) : [];
}

function frontdoorCallSignature(call, taskId) {
  const input = call.input && typeof call.input === "object" ? call.input : {};
  return JSON.stringify({
    task_id: taskId,
    case_id: toolInputText(input, "case_id") || toolInputText(input, "caseId"),
    intent: toolInputText(input, "intent") || "auto",
    holder_name: toolInputText(input, "holder_name"),
    via_holder_name: toolInputText(input, "via_holder_name") || toolInputText(input, "counterparty_name"),
    question: toolInputText(input, "question").replace(/\s+/gu, " ").slice(0, 260),
    focus_keywords: toolInputArray(input, "focus_keywords").sort(),
  });
}

function analyzeToolSequence(toolCalls, { mode, taskId, task = null, phase4Profile, pluginExpected = true }) {
  const analytixCalls = toolCalls.filter((item) => item.server === "analytix_funds");
  const toolCounts = new Map();
  const repeated = [];
  const repeatedFundsByIntent = [];
  const fundsSignatures = new Map();
  for (const call of analytixCalls) {
    const key = call.tool;
    const count = (toolCounts.get(key) || 0) + 1;
    toolCounts.set(key, count);
    if (count > 1) repeated.push({ tool: key, occurrence: count, index: call.index });
    if (call.tool === "funds_investigate") {
      const signature = frontdoorCallSignature(call, taskId);
      const seen = fundsSignatures.get(signature);
      if (seen) {
        repeatedFundsByIntent.push({
          tool: call.tool,
          occurrence: seen.count + 1,
          first_index: seen.first_index,
          index: call.index,
          reason: "same case/task/intent funds_investigate repeated after prior navigator support; expected the current lead owner or a targeted semantic fact tool"
        });
        seen.count += 1;
      } else {
        fundsSignatures.set(signature, { first_index: call.index, count: 1 });
      }
    }
  }
  const firstAnalytix = analytixCalls[0] || null;
  const firstFundsIndex = analytixCalls.findIndex((item) => item.tool === "funds_investigate");
  const afterFirstFunds = firstFundsIndex >= 0 ? analytixCalls.slice(firstFundsIndex + 1) : [];
  const frontdoorOnly = phase4Profile === "frontdoor_report_only";
  const hiddenToolCalls = frontdoorOnly
    ? analytixCalls.filter((item) => !PHASE4_FRONTDOOR_ALLOWED_TOOLS.has(item.tool))
    : [];
  const reportTask = isReportTaskId(taskId);
  const budgetTask = task && typeof task === "object"
    ? { ...task, id: text(task.id || taskId) }
    : { id: taskId };
  const budgetLimit = modeUsesAnalytixMcp(mode)
    ? expectedAnalytixToolBudgetForTask(budgetTask, undefined, { pluginExpected })
    : null;
  const callsAfterFirst = afterFirstFunds.map((item) => {
    const allowed = reportTask && item.tool === "validate_report_claims";
    return {
      index: item.index,
      tool: item.tool,
      reason: allowed
        ? "allowed report-grade follow-up after navigator card"
        : "semantic follow-up after navigator card; verify it is a continuation, rescope, or hypothesis task"
    };
  });
  return {
    phase4_profile: phase4Profile,
    called_tools: analytixCalls.map((item) => item.tool),
    repeated_same_intent: repeated,
    repeated_funds_investigate_same_case_task_intent: repeatedFundsByIntent,
    used_semantic_tool_before_navigator: analytixCalls.length > 0 && firstAnalytix?.tool !== "funds_investigate",
    bypassed_funds_investigate: false,
    hidden_tool_calls: hiddenToolCalls.map((item) => ({ index: item.index, tool: item.tool })),
    continued_after_first_answer_card: afterFirstFunds.length > 0,
    calls_after_first_funds_investigate: callsAfterFirst,
    first_card_continue_reason: callsAfterFirst.length
      ? callsAfterFirst.map((item) => item.reason).join("; ")
      : "no additional analytix_funds call after first funds_investigate",
    answer_card_trimmed: false,
    facts_given_but_model_missed: null,
    budget_limit: budgetLimit,
    over_budget: budgetLimit == null ? false : analytixCalls.length > budgetLimit,
  };
}

function commandExecutionsFromThreadRead(result) {
  return allItemsFromThreadRead(result)
    .flatMap((item) => {
      if (item?.type === "commandExecution") {
        return [{
          status: text(item.status),
          command: text(item.command || item.cmd || item.title),
        }];
      }
      if (item?.type !== "function_call" || text(item.name) !== "exec_command") {
        return [];
      }
      let args = {};
      try {
        args = JSON.parse(text(item.arguments));
      } catch (_) {
        args = {};
      }
      return [{
        status: text(item.status || "called"),
        command: text(args.cmd || args.command || item.arguments),
      }];
    });
}

function listSessionFiles(root) {
  const output = [];
  if (!root || !fs.existsSync(root)) return output;
  const stack = [root];
  while (stack.length) {
    const current = stack.pop();
    let entries = [];
    try {
      entries = fs.readdirSync(current, { withFileTypes: true });
    } catch (_) {
      continue;
    }
    for (const entry of entries) {
      const fullPath = path.join(current, entry.name);
      if (entry.isDirectory()) {
        stack.push(fullPath);
      } else if (entry.isFile() && entry.name.endsWith(".jsonl")) {
        output.push(fullPath);
      }
    }
  }
  return output;
}

function sessionFileForThread(runtimeHome, threadId) {
  const sessionsRoot = path.join(text(runtimeHome), "sessions");
  const wanted = text(threadId);
  if (!wanted || !fs.existsSync(sessionsRoot)) return "";
  return listSessionFiles(sessionsRoot).find((filePath) => path.basename(filePath).includes(wanted)) || "";
}

function commandExecutionsFromRuntimeSession(runtimeHome, threadId) {
  const sessionPath = sessionFileForThread(runtimeHome, threadId);
  if (!sessionPath) return [];
  const commands = [];
  for (const line of fs.readFileSync(sessionPath, "utf8").split(/\r?\n/u)) {
    if (!line.trim()) continue;
    let record = null;
    try {
      record = JSON.parse(line);
    } catch (_) {
      continue;
    }
    const payload = record?.payload || {};
    if (payload?.type !== "function_call" || text(payload.name) !== "exec_command") {
      continue;
    }
    let args = {};
    try {
      args = JSON.parse(text(payload.arguments));
    } catch (_) {
      args = {};
    }
    commands.push({
      status: text(payload.status || "called"),
      command: text(args.cmd || args.command || payload.arguments),
      source: "runtime_session_jsonl",
    });
  }
  return commands;
}

function isDirectCaseDataCommand(command) {
  const value = text(command);
  return /(?:^|\s)duckdb(?:\s|$)|duckdb\.connect|case\.duckdb|\/cases\/[^/\s]+\/case\.duckdb|fc_transaction|analysis_txn|analysis_account_dim/iu.test(value);
}

function infrastructureFailureReason(record) {
  const answer = text(record?.answer);
  const toolCalls = Array.isArray(record?.tool_calls) ? record.tool_calls : [];
  const commandExecutions = Array.isArray(record?.command_executions) ? record.command_executions : [];
  const status = text(record?.status);
  const error = redactAuthSensitiveText(record?.error);
  const runtimeAuthErrors = Array.isArray(record?.runtime_log_auth_errors)
    ? record.runtime_log_auth_errors.map(redactAuthSensitiveText)
    : [];
  const runtimeTelemetryEvents = Array.isArray(record?.runtime_log_telemetry_events)
    ? record.runtime_log_telemetry_events.map(redactAuthSensitiveText)
    : [];
  if (runtimeTelemetryEvents.length) {
    return `telemetry/runtime failure: ${runtimeTelemetryEvents[0]}`;
  }
  if (runtimeAuthErrors.length) {
    return `auth/runtime failure: ${runtimeAuthErrors[0]}`;
  }
  if (error && /401|unauthorized|token|refresh token|sign in|login|auth/iu.test(error)) {
    return "auth/runtime failure: model turn could not authenticate";
  }
  if (!answer && toolCalls.length === 0 && commandExecutions.length === 0) {
    return "empty turn: no final answer, no tool calls, and no command executions; likely auth/model/runtime failure";
  }
  if (["error", "failed", "cancelled", "canceled", "timeout"].includes(status) && !answer) {
    return `turn ${status}: no final answer to score`;
  }
  return "";
}

function runtimeAuthErrors(runtimeLogs = []) {
  const errors = [];
  const push = (message) => {
    const compact = redactAuthSensitiveText(message).replace(/\s+/gu, " ").slice(0, 240);
    if (compact && !errors.includes(compact)) {
      errors.push(compact);
    }
  };
  for (const line of runtimeLogs) {
    const message = redactAuthSensitiveText(line);
    if (!message) continue;
    if (/401 Unauthorized|token_expired|refresh_token_reused|refresh token|sign in again|log out and sign in|access token could not be refreshed/iu.test(message)) {
      if (/refresh_token_reused/iu.test(message)) {
        push("refresh_token_reused: refresh token already used; log out and sign in again");
      } else if (/token_expired/iu.test(message)) {
        push("token_expired: access token expired; sign in again");
      } else if (/access token could not be refreshed|refresh token was already used|log out and sign in/iu.test(message)) {
        push("refresh token already used; log out and sign in again");
      } else {
        push(message.replace(/^\[agent-runtime-app-server\]\s*/u, ""));
      }
    }
    if (errors.length >= 8) break;
  }
  return errors;
}

function runtimeTelemetryEvents(runtimeLogs = []) {
  const events = [];
  const push = (message) => {
    const compact = redactAuthSensitiveText(message).replace(/\s+/gu, " ").slice(0, 240);
    if (compact && !events.includes(compact)) {
      events.push(compact);
    }
  };
  for (const line of runtimeLogs) {
    const message = redactAuthSensitiveText(line);
    if (!message) continue;
    if (/codex_analytics::client|analytics-events\/events|backend-api\/codex\/analytics|failed to send events|events failed/iu.test(message)) {
      push(message.replace(/^\[agent-runtime-app-server\]\s*/u, ""));
    }
    if (events.length >= 8) break;
  }
  return events;
}

function writeInvalidPreflightResult(options, startedAt, { reason, status, route_violations, runtime_log_auth_errors, runtime_log_telemetry_events, preflight }) {
  const safeReason = redactAuthSensitiveText(reason);
  const safeRouteViolations = Array.isArray(route_violations) ? route_violations.map(redactAuthSensitiveText) : [];
  const safeRuntimeAuthErrors = Array.isArray(runtime_log_auth_errors) ? runtime_log_auth_errors.map(redactAuthSensitiveText) : [];
  const safeRuntimeTelemetryEvents = Array.isArray(runtime_log_telemetry_events)
    ? runtime_log_telemetry_events.map(redactAuthSensitiveText)
    : [];
  const answers = [{
    task_id: "runtime_preflight",
    case_id: options.caseId,
    mode: "runtime",
    status: "error",
    error: safeReason,
    infrastructure_error: safeReason,
    valid_for_scoring: false,
    answer: "",
    answer_chars: 0,
    tool_calls: [],
    tool_sequence: [],
    tool_sequence_analysis: analyzeToolSequence([], {
      mode: "runtime",
      taskId: "runtime_preflight",
      phase4Profile: options.phase4Profile,
    }),
    command_executions: [],
    route_violations: safeRouteViolations,
    runtime_log_auth_errors: safeRuntimeAuthErrors,
    runtime_log_telemetry_events: safeRuntimeTelemetryEvents,
  }];
  const scoring = scoreAnswers({ answers });
  const result = {
    run_status: "invalid",
    valid_for_plugin_quality_score: false,
    abort: {
      reason: safeReason,
      status,
      route_violations: answers[0].route_violations,
    },
    invalid_answers: scoring.invalid_answers || [],
    case_id: options.caseId,
    case_ids: [options.caseId],
    cwd: options.cwd,
    client_process_cwd: clientProcessCwd(options),
    ws_url: options.wsUrl,
    backend_url: options.backendUrl,
    app_server_binary_path: text(options.appServerBinaryPath || defaultAppServerBinaryPath()),
    app_server_ready_timeout_ms: Number(options.appServerReadyTimeoutMs || 0),
    timeout_ms: Number(options.timeoutMs || 0),
    baseline_timeout_ms: Number(options.baselineTimeoutMs || 0),
    phase4_profile: options.phase4Profile,
    oracle_assisted: Boolean(options.phase4OracleSnapshot),
    not_for_plugin_quality_score: Boolean(options.phase4OracleSnapshot),
    phase4_oracle_snapshot: options.phase4OracleSnapshot,
    release_identity: releaseIdentity(),
    started_at: startedAt,
    completed_at: new Date().toISOString(),
    runtime_cache_preflight: options.runtimeCachePreflight || null,
    preflight: preflight || null,
    answers,
    scoring,
  };
  fs.mkdirSync(path.dirname(options.output), { recursive: true });
  fs.writeFileSync(options.output, `${JSON.stringify(result, null, 2)}\n`, "utf8");
  if (options.json) {
    console.log(JSON.stringify(result, null, 2));
  } else {
    console.log(`wrote ${options.output}`);
    console.log(JSON.stringify(scoring.by_mode, null, 2));
  }
  return result;
}

function loadPhase4OracleCards(filePath) {
  const resolved = text(filePath);
  if (!resolved) return {};
  const payload = JSON.parse(fs.readFileSync(resolved, "utf8"));
  const cards = {};
  for (const item of Array.isArray(payload?.tasks) ? payload.tasks : []) {
    const taskId = text(item.task_id);
    const rendered = text(item.rendered_text);
    if (taskId && rendered) cards[taskId] = rendered;
  }
  return cards;
}

const RUN_KEY_SEPARATOR = "\u001f";

function runKey(parts = {}) {
  return [
    text(parts.case_id || parts.caseId),
    text(parts.task_id || parts.taskId),
    text(parts.mode),
    text(parts.model),
    text(parts.provider),
    text(parts.effort),
  ].join(RUN_KEY_SEPARATOR);
}

function expectedRunKey(options, task, mode, modelSpec) {
  return runKey({
    caseId: text(task.case_id || options.caseId),
    taskId: task.id,
    mode,
    model: modelSpec.model,
    provider: modelSpec.provider,
    effort: modelSpec.effort,
  });
}

function answerRunKey(answer) {
  return runKey(answer);
}

function refreshRunId(parts = {}) {
  return `${text(parts.task_id || parts.taskId).toLowerCase()}:${text(parts.mode).toLowerCase()}`;
}

function shouldRefreshRun(options, answer) {
  return options.refreshRuns instanceof Set && options.refreshRuns.has(refreshRunId(answer));
}

function selectedRunKeys(options, tasks) {
  const keys = new Set();
  for (const modelSpec of options.models) {
    for (const task of tasks) {
      for (const mode of options.modes) {
        keys.add(expectedRunKey(options, task, mode, modelSpec));
      }
    }
  }
  return keys;
}

function loadResumeAnswers(options, tasks) {
  const outputPath = text(options.output);
  if (!options.resume || !outputPath || !fs.existsSync(outputPath)) {
    return { enabled: Boolean(options.resume), loaded_count: 0, reused_count: 0, ignored_count: 0, answers: [] };
  }
  const selectedKeys = selectedRunKeys(options, tasks);
  const taskMap = new Map(tasks.map((task) => [text(task.id), task]));
  const payload = JSON.parse(fs.readFileSync(outputPath, "utf8"));
  const previousAnswers = Array.isArray(payload?.answers) ? payload.answers : [];
  const answersByKey = new Map();
  let ignoredCount = 0;
  for (const answer of previousAnswers) {
    const key = answerRunKey(answer);
    const task = taskMap.get(text(answer?.task_id));
    const commandExecutions = Array.isArray(answer?.command_executions) ? answer.command_executions : [];
    if (!key || !selectedKeys.has(key)) {
      ignoredCount += 1;
      continue;
    }
    if (shouldRefreshRun(options, answer)) {
      ignoredCount += 1;
      continue;
    }
    if (modeRequiresDirectDataCheck(answer?.mode) && task?.pluginExpected !== false && commandExecutions.length === 0) {
      ignoredCount += 1;
      continue;
    }
    answersByKey.set(key, answer);
  }
  return {
    enabled: true,
    source: outputPath,
    loaded_count: previousAnswers.length,
    reused_count: answersByKey.size,
    ignored_count: ignoredCount,
    answers: [...answersByKey.values()],
  };
}

function invalidAnswerSummaries(authPreflight, answers, scoring, aborted) {
  let invalidAnswers = [
    ...(authPreflight?.ok === false
      ? [{
          task_id: "runtime_preflight",
          mode: "runtime",
          status: authPreflight.status,
          reason: authPreflight.reason,
          route_violations: Array.isArray(authPreflight.route_violations) ? authPreflight.route_violations : [],
        }]
      : []),
    ...answers
      .filter((item) => text(item.infrastructure_error))
      .map((item) => ({
        task_id: text(item.task_id),
        mode: text(item.mode),
        status: text(item.status),
        reason: text(item.infrastructure_error),
        route_violations: Array.isArray(item.route_violations) ? item.route_violations : [],
      }))
  ];
  if (!invalidAnswers.length && Array.isArray(scoring.invalid_answers) && scoring.invalid_answers.length) {
    invalidAnswers = scoring.invalid_answers;
  }
  if (aborted && !invalidAnswers.length) {
    invalidAnswers = [{
      task_id: text(aborted.task_id),
      mode: text(aborted.mode),
      status: text(aborted.status),
      reason: text(aborted.reason),
      route_violations: Array.isArray(aborted.route_violations) ? aborted.route_violations : [],
    }];
  }
  return invalidAnswers;
}

function writeRunResult(options, {
  runStatus,
  validForPluginQualityScore,
  startedAt,
  authPreflight,
  answers,
  scoring,
  aborted,
  paused,
  resumeState,
  expectedRunCount,
}) {
  const caseIds = [...new Set(answers.map((item) => text(item.case_id)).filter(Boolean))];
  const selectedKeys = new Set(answers.map(answerRunKey).filter(Boolean));
  const guardLines = worktreeMutationLines(options);
  const result = {
    run_status: runStatus,
    valid_for_plugin_quality_score: Boolean(validForPluginQualityScore),
    abort: aborted || null,
    pause: paused || null,
    invalid_answers: Array.isArray(scoring.invalid_answers) ? scoring.invalid_answers : [],
    case_id: caseIds.length === 1 ? caseIds[0] : "mixed",
    case_ids: caseIds,
    cwd: options.cwd,
    client_process_cwd: clientProcessCwd(options),
    ws_url: options.wsUrl,
    backend_url: options.backendUrl,
    app_server_binary_path: text(options.appServerBinaryPath || defaultAppServerBinaryPath()),
    app_server_ready_timeout_ms: Number(options.appServerReadyTimeoutMs || 0),
    timeout_ms: Number(options.timeoutMs || 0),
    baseline_timeout_ms: Number(options.baselineTimeoutMs || 0),
    phase4_profile: options.phase4Profile,
    oracle_assisted: Boolean(options.phase4OracleSnapshot),
    not_for_plugin_quality_score: Boolean(options.phase4OracleSnapshot) || runStatus !== "completed",
    phase4_oracle_snapshot: options.phase4OracleSnapshot,
    release_identity: releaseIdentity(),
    runtime_cache_preflight: options.runtimeCachePreflight || null,
    auth_preflight: authPreflight,
    worktree_mutation_guard: {
      enabled: worktreeMutationGuardEnabled(options),
      allowed_output_path: repoRelativePath(options.output),
      dirty_count: guardLines.length,
      dirty_files: guardLines.map((line) => porcelainStatusPath(line)).filter(Boolean),
    },
    resume: {
      enabled: Boolean(options.resume),
      loaded_count: Number(resumeState?.loaded_count || 0),
      reused_count: Number(resumeState?.reused_count || 0),
      ignored_count: Number(resumeState?.ignored_count || 0),
      max_new_runs: Number(options.maxNewRuns || 0),
      refresh_runs: [...(options.refreshRuns || [])],
    },
    progress: {
      expected_runs: expectedRunCount,
      completed_runs: selectedKeys.size,
      remaining_runs: Math.max(0, expectedRunCount - selectedKeys.size),
    },
    started_at: startedAt,
    completed_at: new Date().toISOString(),
    answers,
    scoring,
  };
  fs.mkdirSync(path.dirname(options.output), { recursive: true });
  const tmpPath = `${options.output}.${process.pid}.tmp`;
  fs.writeFileSync(tmpPath, `${JSON.stringify(result, null, 2)}\n`, "utf8");
  fs.renameSync(tmpPath, options.output);
  return result;
}

function assertSelfTest(condition, message) {
  if (!condition) {
    throw new Error(`resume self-test failed: ${message}`);
  }
}

function fakeAnswer(options, task, mode, modelSpec, overrides = {}) {
  return {
    task_id: task.id,
    case_id: text(task.case_id || options.caseId),
    title: task.title,
    mode,
    model: modelSpec.model,
    provider: modelSpec.provider,
    effort: modelSpec.effort,
    status: "completed",
    answer: "self-test answer",
    answer_chars: 16,
    tool_calls: [],
    tool_sequence: [],
    command_executions: [],
    route_violations: [],
    ...overrides,
  };
}

function runResumeSelfTest(options) {
  const taskMap = new Map(TASKS.map((task) => [task.id, task]));
  const tasks = ["top_account", "data_quality"].map((taskId) => taskMap.get(taskId)).filter(Boolean);
  const modelSpec = options.models[0];
  assertSelfTest(tasks.length === 2, "expected fixture tasks are present");
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-agent-ui-resume-"));
  const outputPath = path.join(tmpRoot, "agent-ui-ab-resume.json");
  const selfTestOptions = {
    ...options,
    output: outputPath,
    resume: true,
    taskIds: tasks.map((task) => task.id),
    modes: ["skill_mcp", "full_plugin"],
    models: [modelSpec],
    maxNewRuns: 3,
    refreshRuns: new Set(),
  };
  const firstAnswer = fakeAnswer(selfTestOptions, tasks[0], "skill_mcp", modelSpec, { answer: "old duplicate" });
  const replacementAnswer = fakeAnswer(selfTestOptions, tasks[0], "skill_mcp", modelSpec, { answer: "new duplicate wins" });
  const secondAnswer = fakeAnswer(selfTestOptions, tasks[0], "full_plugin", modelSpec);
  const ignoredAnswer = fakeAnswer(selfTestOptions, tasks[1], "plugin_disabled", modelSpec);
  const staleBaselineAnswer = fakeAnswer(selfTestOptions, tasks[1], "skill_only", modelSpec, { command_executions: [] });
  fs.writeFileSync(outputPath, `${JSON.stringify({ answers: [firstAnswer, secondAnswer, ignoredAnswer, staleBaselineAnswer, replacementAnswer] }, null, 2)}\n`, "utf8");
  const resumeState = loadResumeAnswers(selfTestOptions, tasks);
  assertSelfTest(resumeState.loaded_count === 5, "loaded_count tracks previous output answers");
  assertSelfTest(resumeState.reused_count === 2, "duplicate run keys collapse to one reusable answer");
  assertSelfTest(resumeState.ignored_count === 2, "answers outside selected modes or stale commandless baselines are ignored");
  assertSelfTest(resumeState.answers.some((answer) => answer.answer === "new duplicate wins"), "latest duplicate answer is retained");
  const refreshState = loadResumeAnswers({
    ...selfTestOptions,
    refreshRuns: new Set([`${tasks[0].id}:skill_mcp`]),
  }, tasks);
  assertSelfTest(refreshState.reused_count === 1, "refresh-runs drops only selected completed row");
  assertSelfTest(refreshState.ignored_count === 4, "refresh-runs keeps unselected matrix rows ignored without losing reusable rows");
  const extractedCommands = commandExecutionsFromThreadRead({
    turns: [{
      items: [{
        type: "function_call",
        name: "exec_command",
        status: "completed",
        arguments: JSON.stringify({ cmd: "python3 - <<'PY'\nprint('ok')\nPY" }),
      }],
    }],
  });
  assertSelfTest(extractedCommands.length === 1, "exec_command function_call is counted as command execution");
  assertSelfTest(extractedCommands[0].command.includes("print('ok')"), "exec_command arguments are decoded");
  const sessionThreadId = "019e-self-test";
  const sessionDir = path.join(tmpRoot, "sessions", "2026", "05", "30");
  fs.mkdirSync(sessionDir, { recursive: true });
  fs.writeFileSync(
    path.join(sessionDir, `rollout-2026-05-30T00-00-00-${sessionThreadId}.jsonl`),
    `${JSON.stringify({ payload: { type: "function_call", name: "exec_command", arguments: JSON.stringify({ cmd: "python3 - <<'PY'\nprint('session ok')\nPY" }) } })}\n`,
    "utf8"
  );
  const sessionCommands = commandExecutionsFromRuntimeSession(tmpRoot, sessionThreadId);
  assertSelfTest(sessionCommands.length === 1, "runtime session exec_command fallback is counted");
  assertSelfTest(sessionCommands[0].source === "runtime_session_jsonl", "runtime session fallback source is recorded");
  assertSelfTest(isDirectCaseDataCommand("python3 - <<'PY'\nimport duckdb\nduckdb.connect('/tmp/cases/case-1/case.duckdb')\nPY"), "direct case-data command is detected");
  assertSelfTest(!isDirectCaseDataCommand("sed -n '1,40p' /tmp/skills/analytix-fund-analysis/SKILL.md"), "skill file reads are not treated as direct case-data reads");
  const result = writeRunResult(selfTestOptions, {
    runStatus: "in_progress",
    validForPluginQualityScore: false,
    startedAt: "2026-05-30T00:00:00.000Z",
    authPreflight: { ok: true, self_test: true },
    answers: resumeState.answers,
    scoring: { results: [], by_mode: {}, invalid_answers: [] },
    aborted: null,
    paused: { reason: "self-test checkpoint" },
    resumeState,
    expectedRunCount: selfTestOptions.models.length * tasks.length * selfTestOptions.modes.length,
  });
  const written = JSON.parse(fs.readFileSync(outputPath, "utf8"));
  assertSelfTest(written.progress.expected_runs === 4, "expected run count is persisted");
  assertSelfTest(written.progress.completed_runs === 2, "completed run count is persisted");
  assertSelfTest(written.progress.remaining_runs === 2, "remaining run count is persisted");
  assertSelfTest(written.resume.max_new_runs === 3, "max_new_runs is persisted");
  assertSelfTest(!fs.readdirSync(tmpRoot).some((entry) => entry.endsWith(".tmp")), "atomic temp file is renamed away");
  fs.rmSync(tmpRoot, { recursive: true, force: true });
  return {
    ok: true,
    loaded_count: resumeState.loaded_count,
    reused_count: resumeState.reused_count,
    ignored_count: resumeState.ignored_count,
    refresh_reused_count: refreshState.reused_count,
    refresh_ignored_count: refreshState.ignored_count,
    extracted_command_count: extractedCommands.length,
    session_command_count: sessionCommands.length,
    direct_case_data_guard: true,
    expected_runs: result.progress.expected_runs,
    completed_runs: result.progress.completed_runs,
    remaining_runs: result.progress.remaining_runs,
  };
}

function isTerminalTurnStatus(status) {
  return ["completed", "failed", "cancelled", "canceled", "interrupted"].includes(text(status));
}

async function waitForTurn(client, threadId, turnId, options) {
  const startedAt = Date.now();
  let lastRead = null;
  while (Date.now() - startedAt < options.timeoutMs) {
    try {
      lastRead = await client.threadRead({ threadId, includeTurns: true });
    } catch (error) {
      if (/not materialized yet|includeTurns is unavailable before first user message|rollout .* is empty/iu.test(text(error?.message || error))) {
        await new Promise((resolve) => setTimeout(resolve, options.pollMs));
        continue;
      }
      throw error;
    }
    const turns = turnsFromThreadRead(lastRead);
    const targetTurn = turns.find((turn) => text(turn.id || turn.turnId || turn.turn_id) === turnId) || turns.at(-1);
    if (targetTurn && isTerminalTurnStatus(targetTurn.status)) {
      return { read: lastRead, turn: targetTurn, elapsed_ms: Date.now() - startedAt };
    }
    await new Promise((resolve) => setTimeout(resolve, options.pollMs));
  }
  return { read: lastRead, turn: null, elapsed_ms: Date.now() - startedAt, timeout: true };
}

async function runAuthPreflight(client, options) {
  if (!options.authPreflight) {
    return { ok: true, skipped: true, reason: "auth preflight skipped by --skip-auth-preflight" };
  }
  const modelSpec = options.models[0];
  const runtimeLogOffset = options.runtimeLogs.length;
  const routeViolations = [];
  const preflightOptions = {
    ...options,
    timeoutMs: Math.min(Number(options.authPreflightTimeoutMs) || 60_000, Number(options.timeoutMs) || 60_000),
    pollMs: Math.min(Number(options.pollMs) || 1_000, 1_000),
  };
  const startedAt = Date.now();
  try {
    const threadStart = await client.threadStart({
      cwd: options.cwd,
      model: modelSpec.model,
      modelProvider: modelSpec.provider,
      approvalPolicy: "never",
      approvalsReviewer: "user",
      sandbox: threadSandboxMode(options),
      developerInstructions: [
        "这是 Analytix eval harness 的模型登录态预检。",
        "不要调用任何工具，不要读取文件，不要修改环境。",
        "只回复 auth_preflight_ok。",
      ].join("\n"),
      persistExtendedHistory: false,
      // The runner polls via threadRead(includeTurns=true); analytixagent rejects
      // that on ephemeral threads, so use a short-lived persisted thread here.
      ephemeral: false,
    });
    const thread = threadFromResult(threadStart);
    const threadId = text(thread.id || thread.sessionId);
    if (!threadId) throw new Error("auth preflight thread/start returned no thread id");
    const turnStart = await client.turnStart({
      threadId,
      input: [{ type: "text", text: "只回复 auth_preflight_ok。", text_elements: [] }],
      cwd: options.cwd,
      approvalPolicy: "never",
      approvalsReviewer: "user",
      sandboxPolicy: turnSandboxPolicy(options),
      model: modelSpec.model,
      modelProvider: modelSpec.provider,
      effort: modelSpec.effort,
      serviceTier: null,
      summary: "none",
      personality: null,
      collaborationMode: null,
    });
    const turnId = text(turnStart?.turn?.id || turnStart?.turnId || turnStart?.turn_id);
    const waited = await waitForTurn(client, threadId, turnId, preflightOptions);
    if (waited.timeout) {
      try {
        await client.turnInterrupt({ threadId, turnId });
      } catch (error) {
        routeViolations.push(`auth preflight interrupt failed: ${text(error?.message || error)}`);
      }
    }
    const recentAuthErrors = runtimeAuthErrors(options.runtimeLogs.slice(runtimeLogOffset));
    const recentTelemetryEvents = runtimeTelemetryEvents(options.runtimeLogs.slice(runtimeLogOffset));
    if (recentTelemetryEvents.length) {
      return {
        ok: false,
        status: "telemetry_preflight_failed",
        reason: `telemetry/runtime failure before first eval task: ${recentTelemetryEvents[0]}`,
        runtime_log_auth_errors: recentAuthErrors,
        runtime_log_telemetry_events: recentTelemetryEvents,
        route_violations: ["agent telemetry event attempted before first eval task", ...routeViolations],
        elapsed_ms: Date.now() - startedAt,
      };
    }
    if (recentAuthErrors.length) {
      return {
        ok: false,
        status: "auth_preflight_failed",
        reason: `auth/runtime failure before first eval task: ${recentAuthErrors[0]}`,
        runtime_log_auth_errors: recentAuthErrors,
        route_violations: ["agent auth preflight failed before first eval task", ...routeViolations],
        elapsed_ms: Date.now() - startedAt,
      };
    }
    const answer = finalAnswerFromThreadRead(waited.read);
    const toolCalls = toolCallsFromThreadRead(waited.read);
    if (toolCalls.length) {
      routeViolations.push("auth preflight unexpectedly used tools");
    }
    const status = text(waited.turn?.status || (waited.timeout ? "timeout" : "unknown"));
    if (!answer || ["error", "failed", "cancelled", "canceled", "timeout"].includes(status)) {
      return {
        ok: false,
        status: "auth_preflight_empty",
        reason: `auth/runtime failure before first eval task: preflight turn ${status || "unknown"} produced no usable answer`,
        runtime_log_auth_errors: [],
        route_violations: ["agent auth preflight produced no usable model answer", ...routeViolations],
        elapsed_ms: Date.now() - startedAt,
      };
    }
    return {
      ok: true,
      status,
      elapsed_ms: Date.now() - startedAt,
      answer_chars: answer.length,
      tool_calls: toolCalls.length,
      route_violations: routeViolations,
    };
  } catch (error) {
    const recentAuthErrors = runtimeAuthErrors(options.runtimeLogs.slice(runtimeLogOffset));
    const recentTelemetryEvents = runtimeTelemetryEvents(options.runtimeLogs.slice(runtimeLogOffset));
    const message = text(error?.message || error);
    if (recentTelemetryEvents.length) {
      return {
        ok: false,
        status: "telemetry_preflight_error",
        reason: `telemetry/runtime failure before first eval task: ${recentTelemetryEvents[0]}`,
        runtime_log_auth_errors: recentAuthErrors,
        runtime_log_telemetry_events: recentTelemetryEvents,
        route_violations: ["agent telemetry event attempted before first eval task", ...routeViolations],
        elapsed_ms: Date.now() - startedAt,
      };
    }
    const authReason = recentAuthErrors[0] || (/401|unauthorized|token|refresh token|sign in|login|auth/iu.test(message)
      ? "model turn could not authenticate"
      : message);
    return {
      ok: false,
      status: "auth_preflight_error",
      reason: `auth/runtime failure before first eval task: ${authReason}`,
      runtime_log_auth_errors: recentAuthErrors,
      route_violations: ["agent auth preflight failed before first eval task", ...routeViolations],
      elapsed_ms: Date.now() - startedAt,
    };
  }
}

function modeDeveloperInstructions(mode, options = {}) {
  const phase4Line = options.phase4Profile === "frontdoor_report_only"
    ? "Phase4 诊断 profile: frontdoor_report_only 仅用于旧前门对照；新默认路线应使用语义事实工具箱，不能把此 profile 当发布标准。"
    : "";
  if (mode === "plugin_disabled") {
    return [
      "这是 Analytix 涉案资金研判插件关闭的 A/B 基线。",
      "不要调用 analytix_funds MCP 或 Analytix 涉案资金研判插件。",
      "必须先用通用 SQL/Python/notebook 路径读取提供的案件数据源并完成至少一次确定性复核；未实际执行数据复核时，不得给出金额结论。",
      "允许用原生 Codex 的侦查意图驱动方式自由探索，但必须说明口径、来源和不确定性。",
      "不要修改任何系统 Codex 或 analytixagent 代码。",
      phase4Line,
    ].join("\n");
  }
  if (mode === "skill_only") {
    return [
      "这是仅注入 Analytix 涉案资金研判短 skill 纪律、但不启用 analytix_funds MCP 的 A/B 基线。",
      "不要调用 analytix_funds MCP 或 Analytix 涉案资金研判插件。",
      "必须先用通用 SQL/Python/notebook 路径读取提供的案件数据源并完成至少一次确定性复核；未实际执行数据复核时，不得给出金额结论。",
      "按短 skill 纪律回答：事实、统计特征、可疑线索、需复核边界分开；候选账户不能写成确认归属；不得输出法律性质最终定性。",
      "不要修改任何系统 Codex 或 analytixagent 代码。",
      phase4Line,
    ].join("\n");
  }
  if (mode === "plugin_enabled_passive") {
    return [
      "这是 Analytix 涉案资金研判插件已启用但不强制入口的 A/B 实测。",
      "保持原生 Codex 的侦查意图驱动和自由探索方式；如果问题涉及涉案资金、账户、户名、对手方、链路、报告复核，应自然判断是否调用可见的 Analytix 涉案资金研判工具。",
      "如果问题明确不是案件/资金研判，完整完成普通任务；普通改写、公式、脱敏或会议事项要保留用户原始数字、日期、时间、符号和请求格式。",
      "不要输出 q_xxx、audit_ref、artifact_id、detail_ref、evidence_refs 等内部编号；金额、链路和交付状态只能来自当前案件已执行的语义事实工具或受控 Workbench；不能把来源不足、未执行的计算、来源明细或未生成的产物写成事实。",
      "不要用 shell/cat/sed 打开插件 SKILL.md、references、生成元数据或本地插件文档；本轮规则已注入，案件事实必须来自当前案件 source envelope 内的语义事实工具或受控 Workbench 执行结果。",
      "输出必须是实质研判结果，不能把工具说明复述成说明书。",
      "不要修改任何系统 Codex 或 analytixagent 代码。",
      phase4Line,
    ].join("\n");
  }
  if (mode === "skill_mcp") {
    return [
      "这是 Analytix 涉案资金研判 skill + MCP 诊断基线，不是发布质量证明中心。",
      "把 Analytix 涉案资金研判插件当作事实发动机，而不是替代你的侦查思考。",
      "自然语言资金问题优先选择最小充分语义事实工具；只有工具选择不明确时才调用 funds_investigate navigator。",
      "answer_card_complete/max_additional_tools 只约束本轮同一问题，不阻断继续追一层、改口径、指定新对象或新假设。",
      "如果工具文本含内部协议、诊断标签或写作模板，最终用户回答必须转写成自然研判结果，不得逐字泄漏内部语。",
      "不要输出 q_xxx、audit_ref、artifact_id、detail_ref、evidence_refs 等内部编号；金额、链路和交付状态只能来自当前案件已执行的语义事实工具或受控 Workbench；不能把来源不足、未执行的计算、来源明细或未生成的产物写成事实。",
      "不要用 shell/cat/sed 打开插件 SKILL.md、references、生成元数据或本地插件文档；本轮规则已注入，案件事实必须来自当前案件 source envelope 内的语义事实工具或受控 Workbench 执行结果。",
      "不要把工具输出复述成说明书；用事实、口径边界、可疑点和下一步补证组织答案。",
      "不要修改任何系统 Codex 或 analytixagent 代码。",
      phase4Line,
    ].join("\n");
  }
  return [
    "这是 Analytix 涉案资金研判完整插件路线的 A/B 实测。",
    "把 Analytix 涉案资金研判插件当作事实发动机，而不是替代你的侦查思考。",
    "自然语言资金问题优先选择最小充分语义事实工具；只有工具选择不明确时才调用 funds_investigate navigator。",
    "answer_card_complete/max_additional_tools 只约束本轮同一问题，不阻断继续追一层、改口径、指定新对象或新假设。",
    "如果工具文本含内部协议、诊断标签或写作模板，最终用户回答必须转写成自然研判结果，不得逐字泄漏内部语。",
    "只有正式报告/报告级段落才调用 validate_report_claims；普通问答、后续去向、Mermaid 图谱题不要为了复核而重复调用前门或 claim 校验。",
    "如果问题明确不是案件/资金研判，完整完成普通任务；普通改写、公式、脱敏或会议事项要保留用户原始数字、日期、时间、符号和请求格式。",
    "图谱只能使用工具返回的确定性资金边。",
    "不要输出 q_xxx、audit_ref、artifact_id、detail_ref、evidence_refs 等内部编号；金额、链路和交付状态只能来自当前案件已执行的语义事实工具或受控 Workbench；不能把来源不足、未执行的计算、来源明细或未生成的产物写成事实。",
    "不要用 shell/cat/sed 打开插件 SKILL.md、references、生成元数据或本地插件文档；本轮规则已注入，案件事实必须来自当前案件 source envelope 内的语义事实工具或受控 Workbench 执行结果。",
    "不要把工具输出复述成说明书；用事实、口径边界、可疑点和下一步补证组织答案。",
    "不要修改任何系统 Codex 或 analytixagent 代码。",
    phase4Line,
  ].join("\n");
}

function legacyDirectCaseDataLine(options = {}) {
  const caseDb = text(options.caseDb);
  if (caseDb) {
    return `案件数据源：${caseDb}；必须先执行实际 SQL/Python 复核，不能凭记忆或上下文直接作答。`;
  }
  return "案件数据源：未显式配置；legacy direct-data baseline 必须通过 --case-db 或 ANALYTIX_FUNDS_ORACLE_CASE_DB / ANALYTIX_FUNDS_CASE_DB / ANALYTIX_CASE_DB 提供案件库后，才能作出案件事实或金额结论。";
}

function buildRunPrompt(taskPrompt, mode, options, task = {}) {
  const localDbLine = modeRequiresDirectDataCheck(mode)
    ? legacyDirectCaseDataLine(options)
    : "使用当前选中案件；事实必须来自当前案件 source envelope 内的语义事实工具或受控 Workbench 执行结果。";
  const pluginMode = !modeRequiresDirectDataCheck(mode) && mode !== "plugin_disabled" && mode !== "skill_only";
  const pluginExpected = task.pluginExpected !== false;
  const reportTask = isReportTaskId(task.id);
  const taskRouteLine = pluginMode && pluginExpected ? taskSpecificRouteLine(task) : "";
  const budgetLine = pluginMode
    ? !pluginExpected
      ? "本任务不是涉案资金研判任务：不要调用 analytix_funds、funds_investigate 或 validate_report_claims；直接用普通文字能力回答。"
      : reportTask
      ? "本任务是报告级复核/报告材料化，但正式报告链当前处于 P0 隔离：不得调用或模拟 run_full_case_analysis，也不得生成报告草稿。只允许说明当前发布能力缺口、已检查范围、缺失的宿主证据/覆盖范围和补证动作。"
      : "本任务不是正式报告正文复核：选择最小充分语义事实工具作答；显式排行/Top/最大账户题只调用一个 rank_* 工具，rank 结果已包含普通排行所需覆盖数和边界，成功后不要再调用 get_casegraph、get_scope_coverage、audit、profile 或 navigator；只有工具选择不明确时才用 funds_investigate navigator。不要调用 validate_report_claims。"
    : "";
  const protocolLine = pluginMode
    ? "协议遵循：工具内部协议、诊断标签、report gate 或 write_blocked 只能作为内部判断依据，最终用户回答要转写成事实结论、口径边界、证据缺口和下一步补证。"
    : "";
  const oracleCard = text(options.phase4OracleCards?.[task.id]);
  const oracleBlock = oracleCard
    ? [
        "Phase4 oracle-assisted readability only：以下支持材料来自本地 eval fixture，经生产同款 renderer 渲染；本轮只测试模型是否能把支持事实转成专业研判，不证明生产插件能力。",
        "请把它当作语义事实工具刚返回的支持材料。普通题由对应 lead owner 基于材料输出实质研判；继续追一层、调整统计范围或新假设可选择新的语义工具；报告级题才调用 validate_report_claims。",
        oracleCard
      ].join("\n")
    : "";
  return [
    oracleBlock,
    oracleBlock ? "" : "",
    taskPrompt,
    "",
    localDbLine,
    budgetLine,
    taskRouteLine,
    protocolLine,
    "输出要求：只给实质研判结果；关键金额写清楚口径；保留工具返回的自然审计短语，例如同一户名边界、不同排序口径；区分事实、统计特征、可疑线索、需复核边界；不要输出内部工具编号。",
  ].filter((line) => line !== "").join("\n");
}

function taskSpecificRouteLine(task = {}) {
  const id = text(task.id);
  if (id === "bare_analytix_entry_status") {
    return "focused skill 路由：裸 /analytix 必须进入 index/entry-status；只调用 get_current_case/get_case_scope_map 做入口状态和可用 lane，不得触发 full-case-analysis、report-builder 或 claim-review。";
  }
  if (id === "case_context_card") {
    return "focused skill 路由：当前案件上下文必须进入 case-context；允许读取 current case、scope map、coverage、casegraph，但只输出上下文卡和下一步 focused skill，不得跑全案分析或报告。";
  }
  if (id === "account_dossier_card") {
    return "focused skill 路由：某卡/某账户资金研判进入 account-dossier；允许组合 analyze_account_full、rank_counterparties、hypothesis_probe，但答案应按账户资金画像交付：账户基本情况、登记/开户信息、资金流入、资金流出、重点对手方、账户角色、异常特征、核验意见和核查建议，不得只答一个金额或 Top。";
  }
  if (id === "subject_dossier_person") {
    return "focused skill 路由：某人/某公司资金研判进入 subject-dossier；首轮最多组合 analyze_holder_full、rank_accounts、rank_counterparties 三次语义事实工具，成功后不要追加 analyze_account_full、casegraph、scope/schema/audit、run_case_sql、tracing 或 lab。若联系电话/住址/IP/MAC/单位/法人等字段未返回，写成证据缺口和补调对象。最终答案按公安经侦主体资金画像口吻组织：账户基本情况、登记账户清单、重点账户表、资金流入、资金流出、重点对手方、异常特征、可疑用途或去向、核验意见和核查建议；待核账户线索不能写成已确认归属。";
  }
  if (id === "full_case_analysis_tree") {
    return "focused skill 路由：全案分析/完整研判/跑分析树进入 full-case-analysis；P0 隔离期不得调用或模拟 run_full_case_analysis，也不得拼接全案事实，只输出当前全案能力缺口、已检查范围、缺失 lane 和补证动作。";
  }
  if (id === "graph_visualization_supported_edges") {
    return "focused skill 路由：图谱可视化进入 graph-visualization；先用 get_casegraph/build_fund_flow_graph 获取图谱事实。图中只画端点完整、金额时间明确、付款端和收款端可核验的交易；旁路线索、缺端点、同名待核或聚合对象列入待核列表和补证事项，不得画成确定箭头。最终答案按结论、资金来源、主要资金链路、下游去向、资金断点和待补证事项组织。";
  }
  if (id === "case_notebook_controlled_query") {
    return "focused skill 路由：明确 notebook/custom 口径必须进入 case-workbench；只允许当前案件、只读、限行的受控查询。P0 隔离期不得调用 create_case_notebook 或声称已生成可回放 artifact；若用户要求正式 notebook，只说明 artifact 能力缺口和补证/解封条件。";
  }
  if (id === "report_builder_generation") {
    return "focused skill 路由：明确生成报告才进入 report-builder；当前报告链处于 P0 隔离，不得调用或模拟 run_full_case_analysis，不得生成待复核或正式报告草稿。只输出发布能力缺口、缺失证据/覆盖/PublicationReceipt 和补证动作。";
  }
  if (id === "case_3c72_report_claim_review") {
    return "focused skill 路由：既有报告 claim review 进入 claim-review；先调用 validate_report_claims，不得先跑全案分析。若验证支持已给纠正值、未支持资金流、来源边界或 max_additional_tools=0，由 claim-review owner 写复核结论并保留逐项状态表；只有提交的具体金额/路径 claim 仍标为 needs data 时，才允许一次 claim-scoped run_case_sql over cleaned/analysis 表复核。回答必须列出纠正值、两方金额的原明细/高置信去重/集中日期差异、重复放大禁用，并使用可见标题 `需纠正`、`未获支持的研判结论`、`未支持/不能确认资金流`、`降级/线索`、`来源边界`、`正式报告暂不出具`、`禁用表述`、`复核动作`。必须明确报告自述的未完成分析和追踪动作，因此聚合片段/Mermaid/路径没有确定性资金边时只能降级。";
  }
  if (id === "replay_multisubject_trace") {
    return "旧线程 replay 路由：本轮最多两个工具，先用 hypothesis_probe 做开放式多主体假设卡，再用 trace_subject_top_outflows 生成补调/下一步队列；不要追加 casegraph、rank、profile、audit、navigator 或报告门禁。";
  }
  if (id === "replay_rescope_recompute") {
    return "旧线程 replay 路由：普通排行改口径可用 rank_counterparties 按本轮口径重新读金额/笔数/时间窗；若用户转成 Pair Amount、金额对错或 competing amount 质疑，rank_counterparties 只能做候选定位，必须按 Amount Challenge 继续做当前案件 targeted 聚合。不要先查 coverage，不要调用 funds_investigate、rank_holders、resolve_duplicate_families 或旧结论。";
  }
  if (id === "replay_negative_search") {
    return "旧线程 replay 路由：否定性搜索必须调用一次 hypothesis_probe；即使用户没有给出精确姓名/单位，也要用工具确认当前可检字段边界，再说明未命中只限本次字段范围。";
  }
  if (id === "replay_continue_one_more_hop") {
    return "旧线程 replay 路由：继续追一层最多两个工具；没有明确 seed_txn_id/account_id 时先用 build_fund_flow_graph 找 supported edge/可续查边界，再仅在返回明确 seed 时调用 trace_fund_next_hop。不要先让 trace_fund_next_hop 空 seed 失败，也不要追加 navigator/rank/audit。";
  }
  if (id === "replay_report_materialize") {
    return "旧线程 replay 路由：旧报告材料和旧线程状态不能恢复发布权。P0 隔离期不得调用或模拟 run_full_case_analysis，不得生成报告草稿；只输出当前来源、证据、覆盖和 PublicationReceipt 缺口。";
  }
  if (id === "replay_amount_challenge") {
    return "旧线程 replay 路由：金额质疑纠偏不得停在 rank_counterparties；rank_counterparties 只能做候选定位，非必需。若用户质疑最终金额或 competing amount 口径，继续用 inspect_case_schema/run_case_sql 做当前案件、清洗/analysis scope、只读限行 targeted 聚合，必要时可补 scope/data-quality/duplicate/coverage 校验；不要把普通 1-2 工具提示当作硬停止。最终分列 raw/effective/dedup、时间窗、holder/account scope、counterparty grain、核心账户或交易号口径，写明支持金额、不可支持金额、差异原因、验证状态、核验意见和下一步核验，并保留字面短语 `不能写成事实`。";
  }
  return "";
}

function autoRespondServerRequest(client, request, context) {
  const method = text(request?.method);
  const params = request?.params || {};
  context.server_requests.push({
    id: text(request?.id),
    method,
    server_name: text(params.serverName || params.server_name),
    message: redactAuthSensitiveText(params.message),
  });
  try {
    if (method === "mcpServer/elicitation/request") {
      const serverName = text(params.serverName || params.server_name);
      if (!modeUsesAnalytixMcp(context.mode) && serverName === "analytix_funds") {
        context.route_violations.push(`${context.mode} route attempted analytix_funds MCP`);
        client.respondServerRequest(request.id, {
          action: "decline",
          content: { confirmed: false },
          _meta: null,
        });
        return;
      }
      client.respondServerRequest(request.id, {
        action: "accept",
        content: { confirmed: true },
        _meta: null,
      });
      return;
    }
    if (method === "item/commandExecution/requestApproval") {
      const command = text(params.command || params.cmd || params.title || params.item?.command || params.item?.cmd);
      if (threadSandboxMode(context.options || {}) === "read-only" && command && !isReadonlyEvalCommand(command)) {
        context.route_violations.push(`eval turn requested non-read-only command execution: ${redactAuthSensitiveText(command).slice(0, 240)}`);
        client.respondServerRequest(request.id, { decision: "decline" });
        return;
      }
      client.respondServerRequest(request.id, { decision: "accept" });
      return;
    }
    if (method === "item/fileChange/requestApproval") {
      context.route_violations.push("eval turn attempted file change; release-quality runs are read-only");
      client.respondServerRequest(request.id, { decision: "decline" });
      return;
    }
    if (method === "item/permissions/requestApproval") {
      context.route_violations.push("eval turn requested elevated permissions; release-quality runs are read-only");
      client.respondServerRequest(request.id, { permissions: {}, scope: "turn" });
      return;
    }
    client.respondServerRequest(request.id, { decision: "accept" });
  } catch (error) {
    context.route_violations.push(`server request response failed: ${text(error?.message || error)}`);
  }
}

function isReadonlyEvalCommand(command) {
  const value = text(command);
  if (!value) return true;
  if (/[><|;&`]/u.test(value)) return false;
  return /^(?:cat|sed)\s/u.test(value);
}

async function runOne(client, options, modelSpec, task, mode) {
  const pluginEnabled = modeUsesAnalytixMcp(mode);
  const caseId = text(task.case_id || options.caseId);
  const caseName = text(task.case_name);
  const taskOptions = { ...options, caseId };
  const turnTimeoutMs = turnTimeoutMsForMode(options, mode);
  const waitOptions = { ...options, timeoutMs: turnTimeoutMs };
  await activateCase(taskOptions);
  const pluginState = await ensurePluginState(client, pluginEnabled);
  const promptSuite = buildPromptSuite({ caseId, holderName: "合成主体甲", dateStart: "2025-01-01" });
  const promptRecord = promptSuite.find((item) => item.task_id === task.id);
  const modePrompt = promptRecord?.prompts?.find((item) => item.mode === mode)?.prompt || task.prompt;
  const runPrompt = buildRunPrompt(modePrompt, mode, taskOptions, task);
  const context = {
    mode,
    task_id: task.id,
    case_id: caseId,
    model: modelSpec.model,
    provider: modelSpec.provider,
    effort: modelSpec.effort,
    server_requests: [],
    route_violations: [],
    source_delivery_warnings: [],
    notifications: [],
    options,
  };
  const onServerRequest = (request) => autoRespondServerRequest(client, request, context);
  const onNotification = (notification) => {
    const item = notification?.params?.item;
    if (item?.type === "mcpToolCall" || item?.type === "commandExecution" || notification?.method === "turn/completed") {
      context.notifications.push({
        method: text(notification.method),
        type: text(item?.type),
        server: text(item?.server),
        tool: text(item?.tool),
        status: text(item?.status || notification?.params?.turn?.status),
      });
    }
  };
  client.on("server-request", onServerRequest);
  client.on("notification", onNotification);
  try {
    const threadStart = await client.threadStart({
      cwd: options.cwd,
      model: modelSpec.model,
      modelProvider: modelSpec.provider,
      approvalPolicy: "never",
      approvalsReviewer: "user",
      sandbox: threadSandboxMode(options),
      developerInstructions: modeDeveloperInstructions(mode, options),
      persistExtendedHistory: true,
      ephemeral: false,
    });
    const thread = threadFromResult(threadStart);
    const threadId = text(thread.id || thread.sessionId);
    if (!threadId) throw new Error("thread/start returned no thread id");
    const turnStart = await client.turnStart({
      threadId,
      input: [{ type: "text", text: runPrompt, text_elements: [] }],
      cwd: options.cwd,
      approvalPolicy: "never",
      approvalsReviewer: "user",
      sandboxPolicy: turnSandboxPolicy(options),
      model: modelSpec.model,
      modelProvider: modelSpec.provider,
      effort: modelSpec.effort,
      serviceTier: null,
      summary: "none",
      personality: null,
      collaborationMode: null,
    });
    const turnId = text(turnStart?.turn?.id || turnStart?.turnId || turnStart?.turn_id);
    const waited = await waitForTurn(client, threadId, turnId, waitOptions);
    if (waited.timeout) {
      try {
        await client.turnInterrupt({ threadId, turnId });
        waited.read = await client.threadRead({ threadId, includeTurns: true });
        const turns = turnsFromThreadRead(waited.read);
        waited.turn = turns.find((turn) => text(turn.id || turn.turnId || turn.turn_id) === turnId) || turns.at(-1) || null;
      } catch (error) {
        context.route_violations.push(`timeout interrupt failed: ${text(error?.message || error)}`);
      }
    }
    const answer = finalAnswerFromThreadRead(waited.read);
    const toolCalls = toolCallsFromThreadRead(waited.read);
    const pluginExpected = task.pluginExpected !== false;
    const tool_sequence_analysis = analyzeToolSequence(toolCalls, {
      mode,
      taskId: task.id,
      task,
      phase4Profile: options.phase4Profile,
      pluginExpected,
    });
    const threadReadCommandExecutions = commandExecutionsFromThreadRead(waited.read);
    const commandExecutions = threadReadCommandExecutions.length
      ? threadReadCommandExecutions
      : commandExecutionsFromRuntimeSession(text(options.appServerCodexHome) || analytixRuntimeHome(), threadId);
    const directCaseDataCommandExecutions = commandExecutions.filter((item) => isDirectCaseDataCommand(item.command));
    const toolResultTextChars = toolCalls.reduce((sum, item) => sum + Number(item.result_text_chars || 0), 0);
    const usedAnalytixFunds = toolCalls.some((item) => item.server === "analytix_funds");
    if (pluginEnabled && pluginExpected && !usedAnalytixFunds) {
      context.route_violations.push("plugin mode finished without analytix_funds tool call");
    }
    if (pluginEnabled && tool_sequence_analysis.over_budget) {
      context.route_violations.push(`tool budget exceeded: expected at most ${tool_sequence_analysis.budget_limit} analytix_funds tool call(s)`);
    }
    if (pluginEnabled && !pluginExpected && usedAnalytixFunds) {
      context.route_violations.push("passive non-funds task used analytix_funds tool call");
    }
    if (!pluginEnabled && usedAnalytixFunds) {
      context.route_violations.push(`${mode} answer used analytix_funds tool call`);
    }
    if (pluginEnabled && directCaseDataCommandExecutions.length) {
      context.source_delivery_warnings.push(`plugin mode used direct case-data command execution; verify source envelope, validation state, and delivery boundary: ${directCaseDataCommandExecutions.length}`);
    }
    if (modeRequiresDirectDataCheck(mode) && pluginExpected && commandExecutions.length === 0) {
      context.route_violations.push(`${mode} produced a conclusion without direct data-source command execution`);
    }
    if (
      mode === "full_plugin" &&
      isReportTaskId(task.id) &&
      !toolCalls.some((item) => item.tool === "validate_report_claims")
    ) {
      context.route_violations.push("full_plugin report task finished without validate_report_claims");
    }
    if (
      pluginEnabled &&
      !isReportTaskId(task.id) &&
      toolCalls.some((item) => item.tool === "validate_report_claims")
    ) {
      context.route_violations.push("non-report task continued into validate_report_claims after a complete frontdoor card");
    }
    return {
      task_id: task.id,
      case_id: caseId,
      case_name: caseName,
      title: task.title,
      mode,
      mode_label: DEFAULT_MODE_LABELS[mode] || mode,
      model: modelSpec.model,
      provider: modelSpec.provider,
      effort: modelSpec.effort,
      phase4_profile: options.phase4Profile,
      oracle_assisted: Boolean(options.phase4OracleCards?.[task.id]),
      not_for_plugin_quality_score: Boolean(options.phase4OracleCards?.[task.id]),
      thread_id: threadId,
      turn_id: turnId,
      status: waited.turn?.status || (waited.timeout ? "timeout" : "unknown"),
      elapsed_ms: waited.elapsed_ms,
      turn_timeout_ms: turnTimeoutMs,
      plugin_state: pluginState,
      answer,
      answer_chars: answer.length,
      cost_proxy: {
        prompt_chars: runPrompt.length,
        answer_chars: answer.length,
        tool_result_text_chars: toolResultTextChars,
        context_chars_proxy: runPrompt.length + answer.length + toolResultTextChars,
        elapsed_ms: waited.elapsed_ms,
        tool_calls: toolCalls.length,
        command_executions: commandExecutions.length,
      },
      tool_calls: toolCalls,
      tool_sequence: toolCalls,
      tool_sequence_analysis,
      command_executions: commandExecutions,
      direct_case_data_command_executions: directCaseDataCommandExecutions,
      server_requests: context.server_requests,
      source_delivery_warnings: context.source_delivery_warnings.map(redactAuthSensitiveText),
      route_violations: context.route_violations.map(redactAuthSensitiveText),
    };
  } catch (error) {
    return {
      task_id: task.id,
      case_id: caseId,
      case_name: caseName,
      title: task.title,
      mode,
      mode_label: DEFAULT_MODE_LABELS[mode] || mode,
      model: modelSpec.model,
      provider: modelSpec.provider,
      effort: modelSpec.effort,
      phase4_profile: options.phase4Profile,
      oracle_assisted: Boolean(options.phase4OracleCards?.[task.id]),
      not_for_plugin_quality_score: Boolean(options.phase4OracleCards?.[task.id]),
      status: "error",
      error: redactAuthSensitiveText(error?.stack || error?.message || error),
      answer: "",
      answer_chars: 0,
      tool_calls: [],
      tool_sequence: [],
      tool_sequence_analysis: analyzeToolSequence([], {
        mode,
        taskId: task.id,
        phase4Profile: options.phase4Profile,
        pluginExpected: task.pluginExpected !== false,
      }),
      command_executions: [],
      server_requests: context.server_requests,
      source_delivery_warnings: context.source_delivery_warnings.map(redactAuthSensitiveText),
      route_violations: context.route_violations.map(redactAuthSensitiveText),
    };
  } finally {
    client.off("server-request", onServerRequest);
    client.off("notification", onNotification);
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  options.phase4OracleCards = loadPhase4OracleCards(options.phase4OracleSnapshot);
  if (options.selfTestResume) {
    const result = runResumeSelfTest(options);
    if (options.json) {
      console.log(JSON.stringify(result, null, 2));
    } else {
      console.log(`[agent-ui-ab] resume self-test ok ${JSON.stringify(result)}`);
    }
    return;
  }
  const startedAt = new Date().toISOString();
  const preflightMutation = worktreeMutationFailure(options, "preflight");
  if (preflightMutation) {
    writeInvalidPreflightResult(options, startedAt, {
      reason: preflightMutation.reason,
      status: preflightMutation.status,
      route_violations: preflightMutation.route_violations,
    });
    process.exitCode = 2;
    return;
  }
  const runtimePreflight = runtimeCachePreflight(options);
  if (runtimePreflight) {
    writeInvalidPreflightResult(options, startedAt, runtimePreflight);
    process.exitCode = 2;
    return;
  }
  options.runtimeCachePreflight = runtimeCachePreflightPassed(options);
  try {
    options.managedBackend = await startManagedBackend(options);
    startManagedBackendKeepAlive(options);
  } catch (error) {
    writeInvalidPreflightResult(options, startedAt, {
      reason: `managed backend unavailable: ${text(error?.message || error)}`,
      status: "backend_connection_error",
      route_violations: ["managed backend failed before first turn"],
    });
    process.exitCode = 2;
    return;
  }
  const client = createClient(options);
  try {
    await connectClient(client);
    options.wsUrl = client.wsUrl || options.wsUrl;
  } catch (error) {
    const reason = `agent app-server unavailable: ${text(error?.message || error)}`;
    writeInvalidPreflightResult(options, startedAt, {
      reason,
      status: "connection_error",
      route_violations: ["agent app-server connection failed before first turn"],
    });
    await stopClientRuntime(client);
    await stopManagedBackendRuntime(options);
    process.exitCode = 2;
    return;
  }
  const authPreflight = await runAuthPreflight(client, options);
  if (!authPreflight.ok && !options.allowInvalidRuns) {
    writeInvalidPreflightResult(options, startedAt, {
      reason: authPreflight.reason,
      status: authPreflight.status,
      route_violations: authPreflight.route_violations,
      runtime_log_auth_errors: authPreflight.runtime_log_auth_errors,
      runtime_log_telemetry_events: authPreflight.runtime_log_telemetry_events,
      preflight: authPreflight,
    });
    await stopClientRuntime(client);
    await stopManagedBackendRuntime(options);
    process.exitCode = 2;
    return;
  }
  const taskMap = new Map(TASKS.map((task) => [task.id, task]));
  const tasks = options.taskIds.map((taskId) => {
    const task = taskMap.get(taskId);
    if (!task) throw new Error(`unknown task id: ${taskId}`);
    return task;
  });
  const resumeState = loadResumeAnswers(options, tasks);
  const answers = [...resumeState.answers];
  const completedRunKeys = new Set(answers.map(answerRunKey).filter(Boolean));
  const expectedRunCount = options.models.length * tasks.length * options.modes.length;
  let newRunCount = 0;
  let aborted = null;
  let paused = null;
  try {
    runLoop:
    for (const modelSpec of options.models) {
      for (const task of tasks) {
        for (const mode of options.modes) {
          const key = expectedRunKey(options, task, mode, modelSpec);
          if (completedRunKeys.has(key)) {
            console.error(`[agent-ui-ab] resume skip ${modelSpec.model}/${modelSpec.effort} ${task.id} ${mode}`);
            continue;
          }
          console.error(`[agent-ui-ab] ${modelSpec.model}/${modelSpec.effort} ${task.id} ${mode}`);
          const answer = await runOne(client, options, modelSpec, task, mode);
          answer.runtime_log_auth_errors = runtimeAuthErrors(options.runtimeLogs);
          answer.runtime_log_telemetry_events = runtimeTelemetryEvents(options.runtimeLogs);
          const mutationFailure = worktreeMutationFailure(options, `after ${task.id}/${mode}`);
          if (mutationFailure) {
            answer.worktree_mutation_guard = mutationFailure;
            answer.route_violations = [
              ...(Array.isArray(answer.route_violations) ? answer.route_violations : []),
              ...mutationFailure.route_violations,
            ];
            answer.infrastructure_error = mutationFailure.reason;
            answer.valid_for_scoring = false;
          }
          const infrastructureFailure = answer.infrastructure_error || infrastructureFailureReason(answer);
          if (infrastructureFailure) {
            answer.infrastructure_error = infrastructureFailure;
            answer.valid_for_scoring = false;
          }
          answers.push(answer);
          completedRunKeys.add(answerRunKey(answer));
          newRunCount += 1;
          writeRunResult(options, {
            runStatus: "in_progress",
            validForPluginQualityScore: false,
            startedAt,
            authPreflight,
            answers,
            scoring: scoreAnswers({ answers }),
            aborted: null,
            paused: {
              reason: "progress checkpoint after answer",
              new_run_count: newRunCount,
            },
            resumeState,
            expectedRunCount,
          });
          if (infrastructureFailure && !options.allowInvalidRuns) {
            aborted = {
              reason: infrastructureFailure,
              task_id: task.id,
              mode,
              model: modelSpec.model,
              status: answer.status,
              route_violations: answer.route_violations || [],
            };
            console.error(`[agent-ui-ab] aborting invalid run: ${infrastructureFailure}`);
            break runLoop;
          }
          if (options.maxNewRuns > 0 && newRunCount >= options.maxNewRuns) {
            paused = {
              reason: `max_new_runs reached: ${options.maxNewRuns}`,
              new_run_count: newRunCount,
              next_action: "rerun with --resume and the same --output to continue",
            };
            console.error(`[agent-ui-ab] paused: ${paused.reason}`);
            break runLoop;
          }
        }
      }
    }
  } finally {
    if (options.reinstallAtEnd) {
      try {
        await ensurePluginState(client, true);
      } catch (error) {
        console.error(`[agent-ui-ab] reinstall failed: ${text(error?.message || error)}`);
      }
    }
  }
  const finalMutationFailure = worktreeMutationFailure(options, "final");
  if (finalMutationFailure && !aborted) {
    aborted = {
      reason: finalMutationFailure.reason,
      task_id: "worktree_guard",
      mode: "runtime",
      status: finalMutationFailure.status,
      route_violations: finalMutationFailure.route_violations,
    };
  }
  const scoring = scoreAnswers({ answers });
  const invalidAnswers = invalidAnswerSummaries(authPreflight, answers, scoring, aborted);
  const invalidRun = Boolean(!authPreflight.ok || aborted || invalidAnswers.length || scoring.invalid_run);
  if (invalidRun) {
    scoring.invalid_run = true;
    scoring.abort_reason = aborted?.reason || invalidAnswers[0]?.reason || "invalid infrastructure turn";
    scoring.invalid_answers = invalidAnswers;
    scoring.records_scored = 0;
    scoring.by_mode = {};
    scoring.results = [];
  }
  const completedRunCount = new Set(answers.map(answerRunKey).filter(Boolean)).size;
  const completed = completedRunCount >= expectedRunCount;
  if (completed && paused?.reason?.startsWith("max_new_runs reached:")) {
    paused = null;
  }
  const result = writeRunResult(options, {
    runStatus: invalidRun ? "invalid" : completed ? "completed" : "in_progress",
    validForPluginQualityScore: !invalidRun && completed && !options.phase4OracleSnapshot,
    startedAt,
    authPreflight,
    answers,
    scoring,
    aborted,
    paused,
    resumeState,
    expectedRunCount,
  });
  if (options.json) {
    console.log(JSON.stringify(result, null, 2));
  } else {
    console.log(`wrote ${options.output}`);
    console.log(JSON.stringify(scoring.by_mode, null, 2));
    if (paused) {
      console.log(`[agent-ui-ab] ${paused.next_action}`);
    }
  }
  await stopClientRuntime(client);
  await stopManagedBackendRuntime(options);
  if (aborted && !options.allowInvalidRuns) {
    process.exitCode = 2;
  }
}

main()
  .then(() => {
    process.exit(process.exitCode || 0);
  })
  .catch((error) => {
    console.error(error instanceof Error ? error.stack || error.message : String(error));
    process.exit(1);
  });
