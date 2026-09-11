import crypto from "node:crypto";

export const DUCKDB_DIAGNOSTIC_VERSION = 1;

const CODES = new Set([
  "runner_unavailable",
  "runner_spawn_failed",
  "runner_timeout",
  "runner_cancelled",
  "runner_protocol_invalid",
  "sql_policy_rejected",
  "sql_parse_rejected",
  "sql_bind_rejected",
  "sql_execution_failed",
  "sql_fetch_failed",
  "database_unavailable",
  "database_locked",
  "dataset_snapshot_mismatch",
  "amount_coverage_incomplete",
  "result_contract_invalid",
  "resource_limit_exceeded"
]);

const STAGES = new Set([
  "runner_resolution",
  "runner_spawn",
  "policy",
  "parse",
  "bind",
  "execute",
  "fetch",
  "result_validation",
  "snapshot_validation"
]);

const PUBLIC_MESSAGES = Object.freeze({
  runner_unavailable: "当前运行环境没有可用的本地 DuckDB 只读执行器。",
  runner_spawn_failed: "本地 DuckDB 只读执行器启动失败。",
  runner_timeout: "本地 DuckDB 只读查询在宿主时限内未完成。",
  runner_cancelled: "本地 DuckDB 只读查询已取消。",
  runner_protocol_invalid: "本地 DuckDB 只读执行器返回了无效协议结果。",
  sql_policy_rejected: "SQL 未通过当前案件的只读正向安全策略。",
  sql_parse_rejected: "SQL 无法由当前 DuckDB 解析器安全解析。",
  sql_bind_rejected: "SQL 无法绑定到当前案件的允许数据范围。",
  sql_execution_failed: "当前案件的本地 DuckDB 只读查询执行失败。",
  sql_fetch_failed: "当前案件的本地 DuckDB 只读结果读取失败。",
  database_unavailable: "当前案件的本地 DuckDB 数据库不可用。",
  database_locked: "当前案件数据库正被写入方占用，未执行只读查询。",
  dataset_snapshot_mismatch: "当前案件数据快照已变化，查询结果未被接受。",
  amount_coverage_incomplete: "当前案件金额覆盖不完整，未执行可产生金额事实的查询。",
  result_contract_invalid: "本地 DuckDB 结果不满足宿主结果契约。",
  resource_limit_exceeded: "本地 DuckDB 查询超过宿主资源上限。"
});

const BLOCKERS = Object.freeze({
  runner_unavailable: "source_unavailable",
  runner_spawn_failed: "source_unavailable",
  runner_timeout: "timeout",
  runner_cancelled: "cancelled",
  database_unavailable: "source_unavailable",
  database_locked: "source_busy",
  dataset_snapshot_mismatch: "stale_dataset_snapshot",
  amount_coverage_incomplete: "partial_coverage"
});

function normalizedCode(value) {
  const code = String(value || "").trim().toLowerCase();
  return CODES.has(code) ? code : "sql_execution_failed";
}

function normalizedStage(value, code) {
  const stage = String(value || "").trim().toLowerCase();
  if (STAGES.has(stage)) return stage;
  if (code.startsWith("runner_")) return code === "runner_protocol_invalid" ? "result_validation" : "runner_spawn";
  if (code === "sql_policy_rejected") return "policy";
  if (code === "sql_parse_rejected") return "parse";
  if (code === "sql_bind_rejected" || code === "database_locked") return "bind";
  if (code === "database_unavailable") return "runner_resolution";
  if (code === "sql_fetch_failed") return "fetch";
  if (code === "dataset_snapshot_mismatch") return "snapshot_validation";
  if (code === "amount_coverage_incomplete") return "result_validation";
  if (code === "result_contract_invalid") return "result_validation";
  return "execute";
}

export function duckdbQueryHash(sql) {
  const source = String(sql || "");
  return source
    ? `sha256:${crypto.createHash("sha256").update(source, "utf8").digest("hex")}`
    : "";
}

export function createDuckdbDiagnostic({ code, stage, sql = "" } = {}) {
  const normalized = normalizedCode(code);
  return Object.freeze({
    version: DUCKDB_DIAGNOSTIC_VERSION,
    code: normalized,
    stage: normalizedStage(stage, normalized),
    retryable: normalized === "runner_unavailable",
    publicMessage: PUBLIC_MESSAGES[normalized],
    blocker: BLOCKERS[normalized] || "query_blocked",
    queryHash: duckdbQueryHash(sql)
  });
}

export class DuckdbDiagnosticError extends Error {
  constructor(input = {}) {
    const diagnostic = createDuckdbDiagnostic(input);
    super(diagnostic.publicMessage);
    this.name = "DuckdbDiagnosticError";
    this.code = diagnostic.code;
    this.duckdbDiagnostic = diagnostic;
  }
}

export function duckdbDiagnosticError(input = {}) {
  return new DuckdbDiagnosticError(input);
}

export function isDuckdbDiagnosticError(error) {
  return error instanceof DuckdbDiagnosticError
    && error.duckdbDiagnostic?.version === DUCKDB_DIAGNOSTIC_VERSION
    && CODES.has(error.duckdbDiagnostic.code);
}

export function duckdbDiagnosticFromError(error, { stage = "execute", sql = "" } = {}) {
  if (isDuckdbDiagnosticError(error)) return error.duckdbDiagnostic;
  const nativeCode = String(error?.code || "").trim().toUpperCase();
  if (nativeCode === "ENOENT" || nativeCode === "EACCES" || nativeCode === "ENOEXEC") {
    return createDuckdbDiagnostic({ code: "runner_unavailable", stage: "runner_resolution", sql });
  }
  if (error?.name === "AbortError") {
    return createDuckdbDiagnostic({ code: "runner_cancelled", stage: "execute", sql });
  }
  return createDuckdbDiagnostic({ code: "sql_execution_failed", stage, sql });
}

export function publicDuckdbDiagnostic(error, options = {}) {
  return duckdbDiagnosticFromError(error, options);
}

export function shouldRetryDuckdbRunner(error) {
  return duckdbDiagnosticFromError(error).code === "runner_unavailable";
}

export function assertDuckdbDiagnosticV1(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("DuckdbDiagnosticV1 must be an object.");
  }
  const expectedKeys = ["version", "code", "stage", "retryable", "publicMessage", "blocker", "queryHash"];
  if (Object.keys(value).sort().join("\0") !== expectedKeys.sort().join("\0")) {
    throw new Error("DuckdbDiagnosticV1 contains unknown or missing fields.");
  }
  const canonical = createDuckdbDiagnostic({ code: value.code, stage: value.stage });
  if (
    value.version !== DUCKDB_DIAGNOSTIC_VERSION
    || value.code !== canonical.code
    || value.stage !== canonical.stage
    || value.retryable !== canonical.retryable
    || value.publicMessage !== canonical.publicMessage
    || value.blocker !== canonical.blocker
    || (value.queryHash !== "" && !/^sha256:[a-f0-9]{64}$/u.test(value.queryHash))
  ) {
    throw new Error("DuckdbDiagnosticV1 is not canonical.");
  }
  return value;
}
