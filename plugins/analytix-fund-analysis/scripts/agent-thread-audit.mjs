#!/usr/bin/env node

import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const UUID_PATTERN = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/iu;
const ROLLOUT_FILE_PATTERN = /^rollout-.*-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.jsonl$/iu;
const DEFAULT_MAX_FILES = 50_000;
const LARGE_PAYLOAD_BYTES = 8_000;
const INTERNAL_FIELD_PATTERN = /\b(?:support_only|final_answer_owned_by_focused_skill|answer_card_complete|delivery_state|workflow|case_id|raw_transaction_candidates|direct\/candidate|supported seed|funds_investigate|MCP|Workbench|debug|artifact_provenance|query_guidance|known_risks|deterministic transaction seed|do not infer further hops|another returned transaction edge)\b/iu;
const LOCAL_CASE_GUESS_PATTERN = /\b(?:find|rg|ls|grep)\b[^\n]*(?:\.duckdb|case_id|Downloads\/case|sessions|rollout)/iu;
const INSTRUCTION_READ_PATTERN = /(?:SKILL\.md|skills\/|references\/(?:focused-skill-shared|tool-availability|command-router|anti-patterns|analytix-workflow-context|top-pluginization-plan|blueprint-execution-plan)\.(?:md|json))/iu;
const JSON_LIKE_PATTERN = /^\s*(?:\{|\[|"\s*\{|\[\{)/u;

function text(value) {
  return String(value == null ? "" : value);
}

function trimmed(value) {
  return text(value).trim();
}

function parseArgs(argv) {
  const options = {
    id: "",
    files: [],
    roots: [],
    recent: 0,
    query: "",
    includeSystemCodex: false,
    maxFiles: DEFAULT_MAX_FILES,
    json: false,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--id" || arg === "--thread-id" || arg === "--deeplink") options.id = extractThreadId(next());
    else if (arg === "--file") options.files.push(path.resolve(next()));
    else if (arg === "--files") options.files.push(...parseCsv(next()).map((file) => path.resolve(file)));
    else if (arg === "--roots") options.roots.push(...parseCsv(next()));
    else if (arg === "--recent") options.recent = Math.max(1, Number(next()) || 20);
    else if (arg === "--query") options.query = trimmed(next());
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
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/agent-thread-audit.mjs [options]

Read-only audit for real Analytixagent rollout JSONL files. It locates threads
under Analytix-owned runtime homes by default, then counts model-visible tool
payload bytes, repeated JSON/support output, internal-field leakage, tool count,
and final-answer investigative quality markers.

Options:
  --file <path>          Audit a specific rollout JSONL file. Repeatable.
  --files <csv>          Audit multiple rollout JSONL files.
  --id <id|url>          Locate and audit a thread/session id or deeplink.
  --query <text>         Audit recent rollouts containing the text.
  --recent <n>           Audit n most recent rollouts when no file/id/query is set.
  --roots <csv>          Additional runtime roots or sessions directories.
  --include-system-codex Also search system Codex as read-only fallback.
  --max-files <n>        Maximum rollout files to inspect. Default: ${DEFAULT_MAX_FILES}.
  --json                 Print machine-readable JSON.
`);
}

function parseCsv(value) {
  return trimmed(value).split(",").map((item) => item.trim()).filter(Boolean);
}

function extractThreadId(value) {
  const match = trimmed(value).match(UUID_PATTERN);
  return match ? match[0] : "";
}

function unique(values) {
  return [...new Set(values.map((item) => trimmed(item)).filter(Boolean))];
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
    if (entry.isFile() && ROLLOUT_FILE_PATTERN.test(entry.name)) files.push(entryPath);
    else if (entry.isDirectory() && !shouldPruneDirectory(entry.name)) collectRolloutFiles(entryPath, options, files);
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

function safeReadFile(file, maxBytes = 80 * 1024 * 1024) {
  const stat = fs.statSync(file);
  if ((stat.size || 0) > maxBytes) {
    throw new Error(`rollout too large for audit: ${file}`);
  }
  return fs.readFileSync(file, "utf8");
}

function fileIncludes(file, needle, maxBytes) {
  try {
    return safeReadFile(file, maxBytes).includes(needle);
  } catch {
    return false;
  }
}

function filesForOptions(options) {
  if (options.files.length) return unique(options.files).filter(isFile);
  const roots = defaultRoots(options.roots, options);
  const files = unique(roots.flatMap((root) => collectRolloutFiles(root, options))).sort((a, b) => statMtimeMs(b) - statMtimeMs(a));
  if (options.id) {
    return files.filter((file) => rolloutIdFromPath(file) === options.id || fileIncludes(file, options.id, 4 * 1024 * 1024));
  }
  if (options.query) {
    return files.filter((file) => fileIncludes(file, options.query, 8 * 1024 * 1024)).slice(0, options.recent || 20);
  }
  return files.slice(0, options.recent || 20);
}

function contentText(content) {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .map((item) => text(item?.text || item?.input_text || item?.output_text))
    .filter(Boolean)
    .join("\n");
}

function maybeJsonText(value) {
  const body = trimmed(value);
  if (!JSON_LIKE_PATTERN.test(body)) return containsEscapedJsonBlob(body);
  try {
    JSON.parse(body);
    return true;
  } catch {
    return containsEscapedJsonBlob(body);
  }
}

function containsEscapedJsonBlob(value) {
  const body = text(value);
  return /(?:\[\{"type":"text","text":"\{\\")|(?:\\?"(?:support_only|final_answer_owned_by_focused_skill|case_id|delivery_state|raw_transaction_candidates|query_guidance)\\?")/u.test(body);
}

function canonicalOutputPayload(value) {
  return text(value)
    .replace(/^Wall time:[\s\S]*?\nOutput:\n/u, "")
    .replace(/^Chunk ID:[\s\S]*?\nOutput:\n/u, "")
    .replace(/Wall time: [\d.]+ seconds/gu, "Wall time: <duration> seconds")
    .replace(/Original token count: \d+/gu, "Original token count: <n>")
    .trim();
}

function stableHash(value) {
  return crypto.createHash("sha256").update(value).digest("hex").slice(0, 16);
}

function extractOutputText(payload) {
  const output = payload?.output;
  if (typeof output === "string") return output;
  if (Array.isArray(output)) return contentText(output);
  return JSON.stringify(output || {});
}

function payloadText(payload) {
  try {
    return JSON.stringify(payload || {});
  } catch {
    return "";
  }
}

function mcpResultContentText(result) {
  const payload = result && typeof result === "object" ? result : {};
  const ok = payload.Ok && typeof payload.Ok === "object" ? payload.Ok : {};
  const directContent = contentText(payload.content);
  const okContent = contentText(ok.content);
  return [directContent, okContent].filter(Boolean).join("\n");
}

function answerQuality(finalAnswer) {
  const body = text(finalAnswer);
  const markers = {
    conclusion_first: /(?:合计|已有流水支持|有效金额|结论|可作为当前)/u.test(body.slice(0, 260)),
    period: /(?:\d{4}[-年]\d{1,2}|期间|统计期间|至)/u.test(body),
    transaction_count: /\d+\s*笔/u.test(body),
    amount: /(?:\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?)\s*(?:元|万元)/u.test(body),
    top_or_detail_table: /\|[^\n]*金额[^\n]*\|/u.test(body),
    anomaly_features: /(?:异常|集中|大额|多账户|换卡|短时间|分散|长期关系|账户更换)/u.test(body),
    relation_or_downstream: /(?:关系|角色|下游|去向|承接|分流|用途|实际控制|资金来源)/u.test(body),
    cannot_determine: /(?:暂不能认定|不能直接认定|当前不能确认|证据不足|不宜重复计入)/u.test(body),
    next_evidence: /(?:补证|调取|回单|余额连续|开户资料|持仓|赎回|证明力|下一步)/u.test(body),
    no_internal_terms: !INTERNAL_FIELD_PATTERN.test(body),
  };
  return {
    markers,
    marker_count: Object.values(markers).filter(Boolean).length,
    ok: markers.conclusion_first
      && markers.period
      && markers.transaction_count
      && markers.amount
      && markers.top_or_detail_table
      && markers.anomaly_features
      && markers.relation_or_downstream
      && markers.cannot_determine
      && markers.next_evidence
      && markers.no_internal_terms,
  };
}

function auditFile(file) {
  const source = safeReadFile(file);
  const summary = {
    thread_id: rolloutIdFromPath(file),
    rollout_file_id: rolloutIdFromPath(file),
    path: file,
    deeplink: `codex://threads/${rolloutIdFromPath(file)}`,
    analytix_deeplink: `analytix-agent://threads/${rolloutIdFromPath(file)}`,
    cwd: "",
    created_at: "",
    updated_at: new Date(statMtimeMs(file)).toISOString(),
    turns: [],
    user_messages: [],
    final_answer: "",
    final_answer_preview: "",
    tool_calls: [],
    model_visible_outputs: [],
    mcp_results: [],
    shell_commands: [],
    counters: {},
    failure_attribution: [],
  };
  const outputHashes = new Map();
  for (const line of source.split(/\r?\n/u)) {
    const trimmedLine = line.trim();
    if (!trimmedLine || !trimmedLine.startsWith("{")) continue;
    let record = null;
    try {
      record = JSON.parse(trimmedLine);
    } catch {
      continue;
    }
    const payload = record?.payload && typeof record.payload === "object" ? record.payload : {};
    if (record.type === "session_meta") {
      summary.thread_id = trimmed(payload.id) || summary.thread_id;
      summary.cwd = trimmed(payload.cwd);
      summary.created_at = trimmed(payload.timestamp || record.timestamp);
    }
    if (record.type === "event_msg" && trimmed(payload.turn_id || payload.turnId)) {
      const turnId = trimmed(payload.turn_id || payload.turnId);
      if (!summary.turns.includes(turnId)) summary.turns.push(turnId);
    }
    if (record.type === "event_msg" && payload.type === "user_message" && trimmed(payload.message)) {
      summary.user_messages.push(trimmed(payload.message));
    }
    if (record.type === "response_item" && payload.type === "message") {
      const role = trimmed(payload.role);
      const body = contentText(payload.content);
      if (role === "user" && body) summary.user_messages.push(body);
      if (role === "assistant" && body) summary.final_answer = body;
    }
    if (record.type === "event_msg" && payload.type === "agent_message" && trimmed(payload.message)) {
      summary.final_answer = trimmed(payload.message);
    }
    if (record.type === "response_item" && payload.type === "function_call") {
      const argumentsText = typeof payload.arguments === "string" ? payload.arguments : JSON.stringify(payload.arguments || {});
      const item = {
        name: trimmed(payload.name),
        namespace: trimmed(payload.namespace),
        call_id: trimmed(payload.call_id),
        arguments_bytes: Buffer.byteLength(argumentsText, "utf8"),
        arguments_hash: stableHash(argumentsText),
        arguments_internal_hit: INTERNAL_FIELD_PATTERN.test(argumentsText),
      };
      summary.tool_calls.push(item);
      if (item.name === "exec_command") {
        summary.shell_commands.push({
          call_id: item.call_id,
          arguments_preview: argumentsText.slice(0, 500),
          instruction_read: INSTRUCTION_READ_PATTERN.test(argumentsText),
          local_case_guess_hit: LOCAL_CASE_GUESS_PATTERN.test(argumentsText),
        });
      }
    }
    if (record.type === "event_msg" && payload.type === "mcp_tool_call_end") {
      const body = payloadText(payload.result);
      const visibleContent = mcpResultContentText(payload.result);
      const contentInternalHit = INTERNAL_FIELD_PATTERN.test(visibleContent);
      const hiddenInternalBridgeHit = !contentInternalHit && INTERNAL_FIELD_PATTERN.test(body);
      summary.mcp_results.push({
        call_id: trimmed(payload.call_id),
        tool: trimmed(payload.invocation?.tool || payload.invocation?.name),
        bytes: Buffer.byteLength(body, "utf8"),
        large_json: body.length >= LARGE_PAYLOAD_BYTES && maybeJsonText(body),
        internal_hit: contentInternalHit,
        hidden_internal_bridge_hit: hiddenInternalBridgeHit,
        visible_content_hash: visibleContent ? stableHash(canonicalOutputPayload(visibleContent)) : "",
        preview: body.slice(0, 400),
      });
    }
    if (record.type === "response_item" && payload.type === "function_call_output") {
      const body = extractOutputText(payload);
      const normalized = canonicalOutputPayload(body);
      const hash = body.length >= 1000 ? stableHash(normalized) : "";
      if (hash) outputHashes.set(hash, (outputHashes.get(hash) || 0) + 1);
      summary.model_visible_outputs.push({
        call_id: trimmed(payload.call_id),
        bytes: Buffer.byteLength(body, "utf8"),
        large: body.length >= LARGE_PAYLOAD_BYTES,
        json_like: maybeJsonText(body),
        internal_hit: INTERNAL_FIELD_PATTERN.test(body),
        hash,
        preview: body.slice(0, 700),
      });
    }
  }
  summary.deeplink = `codex://threads/${summary.thread_id}`;
  summary.analytix_deeplink = `analytix-agent://threads/${summary.thread_id}`;
  summary.final_answer_preview = summary.final_answer.slice(0, 1500);
  const instructionReadCallIds = new Set(summary.shell_commands.filter((item) => item.instruction_read).map((item) => item.call_id));
  for (const item of summary.model_visible_outputs) {
    item.instruction_read = instructionReadCallIds.has(item.call_id);
  }
  const repeatedHashes = [...outputHashes.entries()].filter(([, count]) => count > 1);
  const repeatedJsonOutputs = summary.model_visible_outputs
    .filter((item) => item.hash && repeatedHashes.some(([hash]) => hash === item.hash));
  const nonInstructionOutputs = summary.model_visible_outputs.filter((item) => !item.instruction_read);
  const instructionOutputs = summary.model_visible_outputs.filter((item) => item.instruction_read);
  const nonInstructionToolCalls = summary.tool_calls.filter((item) => !instructionReadCallIds.has(item.call_id));
  const repeatedToolCallSignatures = [...nonInstructionToolCalls.reduce((map, item) => {
    const key = `${item.namespace}/${item.name}/${item.arguments_hash}`;
    map.set(key, (map.get(key) || 0) + 1);
    return map;
  }, new Map()).values()].filter((count) => count > 1);
  const repeatedMcpResultSignatures = [...summary.mcp_results.reduce((map, item) => {
    if (!item.tool || !item.visible_content_hash) return map;
    const key = `${item.tool}/${item.visible_content_hash}`;
    map.set(key, (map.get(key) || 0) + 1);
    return map;
  }, new Map()).values()].filter((count) => count > 1);
  const counters = {
    line_count: source.split(/\r?\n/u).filter(Boolean).length,
    file_bytes: Buffer.byteLength(source, "utf8"),
    turn_count: summary.turns.length,
    user_message_count: summary.user_messages.length,
    tool_call_count: summary.tool_calls.length,
    mcp_tool_call_count: summary.mcp_results.length,
    model_visible_output_count: summary.model_visible_outputs.length,
    model_visible_payload_bytes_total: summary.model_visible_outputs.reduce((sum, item) => sum + item.bytes, 0),
    model_visible_payload_bytes_max: Math.max(0, ...summary.model_visible_outputs.map((item) => item.bytes)),
    mcp_payload_bytes_total: summary.mcp_results.reduce((sum, item) => sum + item.bytes, 0),
    mcp_payload_bytes_max: Math.max(0, ...summary.mcp_results.map((item) => item.bytes)),
    large_model_visible_output_count: nonInstructionOutputs.filter((item) => item.large).length,
    large_instruction_read_output_count: instructionOutputs.filter((item) => item.large).length,
    json_like_model_visible_output_count: nonInstructionOutputs.filter((item) => item.json_like).length,
    repeated_model_visible_json_count: repeatedJsonOutputs.length,
    internal_field_leak_output_count: nonInstructionOutputs.filter((item) => item.internal_hit).length,
    internal_field_instruction_read_count: instructionOutputs.filter((item) => item.internal_hit).length,
    internal_field_leak_mcp_result_count: summary.mcp_results.filter((item) => item.internal_hit).length,
    hidden_mcp_internal_bridge_count: summary.mcp_results.filter((item) => item.hidden_internal_bridge_hit).length,
    shell_command_count: summary.shell_commands.length,
    instruction_read_command_count: summary.shell_commands.filter((item) => item.instruction_read).length,
    local_case_guess_command_count: summary.shell_commands.filter((item) => item.local_case_guess_hit).length,
    repeated_tool_call_count: repeatedToolCallSignatures.reduce((sum, count) => sum + count - 1, 0),
    repeated_mcp_result_count: repeatedMcpResultSignatures.reduce((sum, count) => sum + count - 1, 0),
    over_tool_call_for_simple_pair_amount: summary.user_messages.some((item) => /转给.*多少钱/u.test(item)) && nonInstructionToolCalls.length > 3,
  };
  summary.counters = counters;
  summary.final_answer_quality = answerQuality(summary.final_answer);
  if (counters.large_model_visible_output_count > 0) {
    summary.failure_attribution.push({
      code: "MODEL_VISIBLE_LARGE_PAYLOAD",
      finding: "工具结果以大 payload 进入模型可见 function_call_output。",
      likely_files: ["mcp/agent-output-compiler.mjs", "mcp/card-renderer.mjs", "mcp/mcp-tool-result-runtime.mjs"],
    });
  }
  if (counters.json_like_model_visible_output_count > 0) {
    summary.failure_attribution.push({
      code: "MODEL_VISIBLE_JSON",
      finding: "模型可见 tool output 仍像 JSON/转义 JSON，容易诱导复述低质材料。",
      likely_files: ["mcp/agent-output-compiler.mjs", "mcp/card-renderer.mjs"],
    });
  }
  if (counters.internal_field_leak_output_count > 0 || counters.internal_field_leak_mcp_result_count > 0) {
    summary.failure_attribution.push({
      code: "INTERNAL_FIELD_LEAKAGE",
      finding: "support/debug/control 字段进入工具输出或 MCP result。",
      likely_files: ["mcp/agent-output-compiler.mjs", "mcp/agent-payload-compiler.mjs", "mcp/frontdoor-runtime.mjs"],
    });
  }
  if (counters.over_tool_call_for_simple_pair_amount) {
    summary.failure_attribution.push({
      code: "OVER_TOOL_CALL_SIMPLE_AMOUNT",
      finding: "简单两方金额问题调用超过 3 个工具，存在挤牙膏和路线漂移风险。",
      likely_files: ["skills/pair-amount-investigation/SKILL.md", "mcp/frontdoor-routing.mjs"],
    });
  }
  if (counters.repeated_tool_call_count > 0 || counters.repeated_mcp_result_count > 0) {
    summary.failure_attribution.push({
      code: "REPEATED_TOOL_CALL",
      finding: "同一工具以相同参数或相同事实结果重复调用，存在延迟和上下文污染风险。",
      likely_files: ["skills/*/SKILL.md", "mcp/frontdoor-routing.mjs", "mcp/tool-call-runtime.mjs"],
    });
  }
  if (counters.local_case_guess_command_count > 0) {
    summary.failure_attribution.push({
      code: "LOCAL_CASE_GUESS",
      finding: "会话出现本地目录/历史文件猜案命令。",
      likely_files: ["mcp/backend-api-client.mjs", "references/tool-availability.md", "skills/analytix-fund-analysis/SKILL.md"],
    });
  }
  return summary;
}

function printHuman(result) {
  console.log(`agent thread audit: ${result.ok ? "ok" : "failed"}; files=${result.files.length}`);
  for (const item of result.files) {
    console.log("");
    console.log(`thread: ${item.thread_id}`);
    console.log(`path: ${item.path}`);
    console.log(`tools=${item.counters.tool_call_count} model_output_bytes=${item.counters.model_visible_payload_bytes_total} max=${item.counters.model_visible_payload_bytes_max}`);
    console.log(`json_outputs=${item.counters.json_like_model_visible_output_count} large_outputs=${item.counters.large_model_visible_output_count} internal_leaks=${item.counters.internal_field_leak_output_count}`);
    console.log(`answer_quality=${item.final_answer_quality.ok ? "ok" : "needs_review"} markers=${item.final_answer_quality.marker_count}/10`);
    if (item.failure_attribution.length) {
      console.log(`failures: ${item.failure_attribution.map((entry) => entry.code).join(", ")}`);
    }
  }
}

try {
  const options = parseArgs(process.argv.slice(2));
  const files = filesForOptions(options);
  const result = {
    ok: files.length > 0,
    searched: {
      include_system_codex: Boolean(options.includeSystemCodex),
      roots: defaultRoots(options.roots, options),
    },
    files: files.map((file) => auditFile(file)),
  };
  if (options.json) console.log(JSON.stringify(result, null, 2));
  else printHuman(result);
  process.exitCode = result.ok ? 0 : 1;
} catch (error) {
  const payload = { ok: false, error: error instanceof Error ? error.message : String(error) };
  if (process.argv.includes("--json")) console.log(JSON.stringify(payload, null, 2));
  else console.error(payload.error);
  process.exitCode = 1;
}
