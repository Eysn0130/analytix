#!/usr/bin/env node

import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const UUID_PATTERN = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/iu;
const ROLLOUT_FILE_PATTERN = /^rollout-.*-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.jsonl$/iu;
const DEFAULT_MAX_FILES = 50_000;

function text(value) {
  return String(value == null ? "" : value).trim();
}

function parseCsv(value) {
  return text(value).split(",").map((item) => item.trim()).filter(Boolean);
}

function parseArgs(argv) {
  const options = {
    id: "",
    query: "",
    recent: 0,
    roots: [],
    includeSystemCodex: false,
    maxFiles: DEFAULT_MAX_FILES,
    json: false,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--id" || arg === "--thread-id" || arg === "--deeplink") options.id = next();
    else if (arg === "--query") options.query = next();
    else if (arg === "--recent") options.recent = Math.max(1, Number(next()) || 20);
    else if (arg === "--roots") options.roots = parseCsv(next());
    else if (arg === "--include-system-codex") options.includeSystemCodex = true;
    else if (arg === "--max-files") options.maxFiles = Math.max(1, Number(next()) || DEFAULT_MAX_FILES);
    else if (arg === "--json") options.json = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  options.id = extractThreadId(options.id);
  return options;
}

function printHelp() {
  console.log(`Usage: node plugins/analytix-fund-analysis/scripts/agent-thread-locator.mjs [options]

Locate real Analytixagent/Codex rollout files after manual desktop UI testing.
This searches Analytix-owned runtime homes by default and does not mutate
runtime state.

Options:
  --id <id|url>       Thread/session id or codex:// / analytix-agent:// deeplink.
  --query <text>      Find recent rollouts containing this text.
  --recent <n>        List n most recent rollouts. Default when no id/query is 20.
  --roots <csv>       Additional runtime roots or sessions directories.
  --include-system-codex
                      Also search the system Codex home as a read-only fallback.
  --max-files <n>     Maximum rollout files to inspect. Default: ${DEFAULT_MAX_FILES}.
  --json              Print machine-readable JSON.
`);
}

function extractThreadId(value) {
  const raw = text(value);
  if (!raw) return "";
  const match = raw.match(UUID_PATTERN);
  return match ? match[0] : "";
}

function unique(values) {
  return [...new Set(values.map((item) => text(item)).filter(Boolean))];
}

function defaultRoots(extraRoots = [], options = {}) {
  const home = os.homedir();
  const analytixHome = path.join(home, ".analytix");
  const roots = [
    ...extraRoots,
    process.env.ANALYTIX_AGENT_RUNTIME_HOME,
    process.env.ANALYTIX_CODEX_HOME,
    analytixHome,
    path.join(analytixHome, "sessions"),
    path.join(analytixHome, "codex-runtime"),
  ];
  if (options.includeSystemCodex) {
    roots.push(process.env.CODEX_HOME, path.join(home, ".codex"));
  }
  return unique(roots);
}

function rootPriority(file) {
  const normalized = file.replace(/\\/gu, "/");
  if (normalized.includes("/.analytix/")) return 0;
  if (normalized.includes("/.codex/")) return 9;
  return 5;
}

function isDirectory(candidate) {
  try {
    return fs.statSync(candidate).isDirectory();
  } catch {
    return false;
  }
}

function isFile(candidate) {
  try {
    return fs.statSync(candidate).isFile();
  } catch {
    return false;
  }
}

function shouldPruneDirectory(name) {
  return [
    ".cache",
    ".git",
    "node_modules",
    "plugins",
    "target",
    "dist",
    "build",
    "managed-chrome",
  ].includes(name);
}

function collectRolloutFiles(root, options, files = []) {
  if (files.length >= options.maxFiles || !isDirectory(root)) return files;
  let entries = [];
  try {
    entries = fs.readdirSync(root, { withFileTypes: true });
  } catch {
    return files;
  }
  for (const entry of entries) {
    if (files.length >= options.maxFiles) break;
    const entryPath = path.join(root, entry.name);
    if (entry.isFile() && ROLLOUT_FILE_PATTERN.test(entry.name)) {
      files.push(entryPath);
    } else if (entry.isDirectory() && !shouldPruneDirectory(entry.name)) {
      collectRolloutFiles(entryPath, options, files);
    }
  }
  return files;
}

function rolloutIdFromPath(file) {
  return extractThreadId(path.basename(file));
}

function statMtimeMs(file) {
  try {
    return fs.statSync(file).mtimeMs || 0;
  } catch {
    return 0;
  }
}

function safeReadFile(file, maxBytes = 24 * 1024 * 1024) {
  try {
    const stat = fs.statSync(file);
    const size = stat.size || 0;
    if (size <= maxBytes) return fs.readFileSync(file, "utf8");
    const fd = fs.openSync(file, "r");
    try {
      const headSize = Math.floor(maxBytes / 3);
      const tailSize = maxBytes - headSize;
      const head = Buffer.alloc(headSize);
      const tail = Buffer.alloc(tailSize);
      fs.readSync(fd, head, 0, headSize, 0);
      fs.readSync(fd, tail, 0, tailSize, Math.max(0, size - tailSize));
      return `${head.toString("utf8")}\n\n__ANALYTIX_THREAD_LOCATOR_TRUNCATED__\n\n${tail.toString("utf8")}`;
    } finally {
      fs.closeSync(fd);
    }
  } catch {
    return "";
  }
}

function contentText(content) {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .map((item) => text(item?.text || item?.input_text || item?.output_text))
    .filter(Boolean)
    .join("\n");
}

function parseRollout(file, { includeContent = false } = {}) {
  const source = safeReadFile(file);
  const rolloutFileId = rolloutIdFromPath(file);
  const summary = {
    thread_id: rolloutFileId,
    rollout_file_id: rolloutFileId,
    path: file,
    deeplink: `codex://threads/${rolloutFileId}`,
    analytix_deeplink: `analytix-agent://threads/${rolloutFileId}`,
    cwd: "",
    model_provider: "",
    model: "",
    created_at: "",
    updated_at: new Date(statMtimeMs(file)).toISOString(),
    turn_ids: [],
    user_messages: [],
    final_answer: "",
    final_answer_preview: "",
  };
  for (const line of source.split(/\r?\n/u)) {
    const trimmed = line.trim();
    if (!trimmed || !trimmed.startsWith("{")) continue;
    let record = null;
    try {
      record = JSON.parse(trimmed);
    } catch {
      continue;
    }
    const payload = record?.payload && typeof record.payload === "object" ? record.payload : {};
    if (record.type === "session_meta") {
      summary.thread_id = text(payload.id) || summary.thread_id;
      summary.cwd = text(payload.cwd);
      summary.model_provider = text(payload.model_provider || payload.modelProvider);
      summary.model = text(payload.model);
      summary.created_at = text(payload.timestamp || record.timestamp);
    }
    if (record.type === "event_msg" && text(payload.turn_id || payload.turnId)) {
      const turnId = text(payload.turn_id || payload.turnId);
      if (!summary.turn_ids.includes(turnId)) summary.turn_ids.push(turnId);
    }
    if (record.type === "response_item" && payload.type === "message") {
      const role = text(payload.role);
      const body = contentText(payload.content);
      if (role === "user" && body) summary.user_messages.push(body);
      if (role === "assistant" && body) summary.final_answer = body;
    }
    if (record.type === "event_msg" && payload.type === "agent_message" && text(payload.message)) {
      summary.final_answer = text(payload.message);
    }
  }
  summary.deeplink = `codex://threads/${summary.thread_id}`;
  summary.analytix_deeplink = `analytix-agent://threads/${summary.thread_id}`;
  summary.final_answer_preview = summary.final_answer.slice(0, 1200);
  if (!includeContent) {
    summary.user_messages = summary.user_messages.slice(-5).map((item) => item.slice(0, 800));
  }
  return summary;
}

function matchesQuery(file, query) {
  const needle = text(query);
  if (!needle) return true;
  return safeReadFile(file).includes(needle);
}

function locate(options) {
  const roots = defaultRoots(options.roots, options);
  const files = unique(roots.flatMap((root) => collectRolloutFiles(root, options))).sort((a, b) => {
    const priorityDelta = rootPriority(a) - rootPriority(b);
    return priorityDelta || (statMtimeMs(b) - statMtimeMs(a));
  });
  if (options.id) {
    let direct = files.filter((file) => rolloutIdFromPath(file) === options.id);
    if (direct.length === 0) {
      direct = files.filter((file) => safeReadFile(file, 2 * 1024 * 1024).includes(options.id));
    }
    return {
      ok: direct.length > 0,
      mode: "id",
      searched_files: files.length,
      include_system_codex: Boolean(options.includeSystemCodex),
      roots,
      matches: direct.slice(0, 20).map((file) => parseRollout(file, { includeContent: true })),
    };
  }
  if (options.query) {
    const matches = files.filter((file) => matchesQuery(file, options.query));
    return {
      ok: matches.length > 0,
      mode: "query",
      searched_files: files.length,
      include_system_codex: Boolean(options.includeSystemCodex),
      roots,
      matches: matches.slice(0, options.recent || 20).map((file) => parseRollout(file)),
    };
  }
  const recent = files.slice(0, options.recent || 20);
  return {
    ok: recent.length > 0,
    mode: "recent",
    searched_files: files.length,
    include_system_codex: Boolean(options.includeSystemCodex),
    roots,
    matches: recent.map((file) => parseRollout(file)),
  };
}

function printHuman(result) {
  console.log(`agent thread locator: ${result.ok ? "found" : "no match"} (${result.mode}); searched ${result.searched_files} rollout files`);
  for (const item of result.matches) {
    console.log("");
    console.log(`thread: ${item.thread_id}`);
    if (item.rollout_file_id && item.rollout_file_id !== item.thread_id) {
      console.log(`rollout file id: ${item.rollout_file_id}`);
    }
    console.log(`deeplink: ${item.deeplink}`);
    console.log(`analytix deeplink: ${item.analytix_deeplink}`);
    console.log(`path: ${item.path}`);
    if (item.cwd) console.log(`cwd: ${item.cwd}`);
    if (item.created_at || item.updated_at) console.log(`time: ${item.created_at || item.updated_at}`);
    if (item.user_messages.length) console.log(`last user: ${item.user_messages.at(-1).replace(/\s+/gu, " ").slice(0, 220)}`);
    if (item.final_answer_preview) console.log(`final preview: ${item.final_answer_preview.replace(/\s+/gu, " ").slice(0, 360)}`);
  }
}

try {
  const options = parseArgs(process.argv.slice(2));
  const result = locate(options);
  if (options.json) console.log(JSON.stringify(result, null, 2));
  else printHuman(result);
  process.exitCode = result.ok ? 0 : 1;
} catch (error) {
  const payload = { ok: false, error: error instanceof Error ? error.message : String(error) };
  if (process.argv.includes("--json")) console.log(JSON.stringify(payload, null, 2));
  else console.error(payload.error);
  process.exitCode = 1;
}
