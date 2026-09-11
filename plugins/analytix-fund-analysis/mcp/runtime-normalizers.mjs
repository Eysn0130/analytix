export const RUNTIME_NORMALIZERS_VERSION = "0.16.12";
export const MCP_VALIDATION_ERROR_KEY = "__mcp_validation_error";

export function text(value) {
  return String(value || "").trim();
}

export function hasOwn(value, key) {
  return value && typeof value === "object" && Object.hasOwn(value, key);
}

export function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function preserveInternalRuntimeContext(source, target) {
  for (const key of ["_analytix", "__analytix", "analytix_runtime_context"]) {
    if (hasOwn(source, key)) {
      target[key] = source[key];
    }
  }
  return target;
}

export function normalizedTextList(value, limit = 12) {
  if (Array.isArray(value)) {
    return value.map((item) => text(item)).filter(Boolean).slice(0, limit);
  }
  const single = text(value);
  return single ? [single] : [];
}

export function numberOrUndefined(value) {
  if (value == null || (typeof value === "string" && !value.trim())) return undefined;
  if (typeof value !== "number" && typeof value !== "string") return undefined;
  if (typeof value === "string" && !/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/iu.test(value.trim())) {
    return undefined;
  }
  const number = Number(typeof value === "string" ? value.trim() : value);
  return Number.isFinite(number) ? number : undefined;
}

export function intOrUndefined(value) {
  const number = numberOrUndefined(value);
  return number !== undefined && Number.isInteger(number) ? number : undefined;
}

export function booleanOrUndefined(value) {
  return typeof value === "boolean" ? value : undefined;
}

export function validationWarning(code, message) {
  return {
    tool_status: "blocked",
    error: {
      code,
      message
    },
    warnings: [
      {
        code,
        message,
        severity: "blocking"
      }
    ]
  };
}

export function toleranceRatioValidationMessage(toolName, fieldName, value) {
  return `${toolName}.${fieldName} must be a relative ratio within [0,1], for example 0.03 means 3%; absolute yuan tolerance is not supported by this tool contract. Re-run with a ratio or omit the field to use the backend default. Received: ${value}`;
}

export function normalizePlanArgs(args) {
  const next = { ...objectOf(args) };
  const rawBudget = objectOf(next.budget);
  const focusItems = [
    ...normalizedTextList(next.focus_keywords),
    ...normalizedTextList(rawBudget.focus),
    ...normalizedTextList(rawBudget.focus_keywords)
  ];
  delete next.focus_keywords;
  const goal = text(next.analysis_goal) || "full_case";
  if (focusItems.length) {
    next.analysis_goal = `${goal}; focus: ${focusItems.join(", ")}`;
  }

  const budget = {};
  for (const key of [
    "max_tool_calls_per_lane",
    "max_evidence_rows_per_lane",
    "max_prompt_chars_per_lane",
    "max_total_tool_calls",
    "max_report_claims"
  ]) {
    const value = numberOrUndefined(rawBudget[key]);
    if (value !== undefined) {
      budget[key] = value;
    }
  }
  next.budget = budget;
  return next;
}

export function normalizeTraceNextHopArgs(args) {
  const next = { ...objectOf(args) };
  if (!hasOwn(next, "amount_tolerance") && hasOwn(next, "amount_tolerance_ratio")) {
    next.amount_tolerance = next.amount_tolerance_ratio;
  }
  if (!hasOwn(next, "amount_tolerance") && hasOwn(next, "tolerance_amount")) {
    next.amount_tolerance = next.tolerance_amount;
  }
  const ratio = numberOrUndefined(next.amount_tolerance);
  if (ratio !== undefined && ratio > 1) {
    next[MCP_VALIDATION_ERROR_KEY] = toleranceRatioValidationMessage("trace_fund_next_hop", "amount_tolerance", next.amount_tolerance);
  }
  delete next.amount_tolerance_ratio;
  delete next.amount_tolerance_abs;
  if (!hasOwn(next, "max_depth") && hasOwn(next, "depth")) {
    next.max_depth = next.depth;
  }
  delete next.tolerance_amount;
  delete next.include_return_flows;
  delete next.depth;
  return next;
}

export function normalizeTraceFundArgs(args) {
  const next = { ...objectOf(args) };
  const ratio = numberOrUndefined(next.tolerance_amount);
  if (ratio !== undefined && ratio > 1) {
    next[MCP_VALIDATION_ERROR_KEY] = toleranceRatioValidationMessage("trace_fund", "tolerance_amount", next.tolerance_amount);
  }
  return next;
}

const RANKING_SKILL_NAMES = new Set(["rank_accounts", "rank_holders", "rank_counterparties"]);

function normalizeRankingArgs(name, args) {
  const next = { ...objectOf(args) };
  const requestedLimit = next.limit ?? next.top_n ?? next.topN ?? next.requested_limit ?? next.requestedLimit;
  next.limit = clampInt(requestedLimit, 10, 1, 200);
  next.requested_limit = next.limit;
  delete next.top_n;
  delete next.topN;
  delete next.requestedLimit;
  if (name === "rank_counterparties") {
    const rawGroupMode = text(next.counterparty_group_mode ?? next.counterpartyGroupMode);
    next.counterparty_group_mode = rawGroupMode === "account" ? "account" : "name";
    next.include_self_counterparty = next.include_self_counterparty === true || next.includeSelfCounterparty === true;
    delete next.counterpartyGroupMode;
    delete next.includeSelfCounterparty;
  } else {
    delete next.counterparty_group_mode;
  }
  return next;
}

function normalizePairAmountArgs(args) {
  const next = { ...objectOf(args) };
  if (!hasOwn(next, "payer_name")) {
    const payerName = next.holder_name ?? next.from_name ?? next.source_name;
    if (text(payerName)) next.payer_name = payerName;
  }
  if (!hasOwn(next, "receiver_name")) {
    const receiverName = next.via_holder_name
      ?? next.counterparty_name
      ?? next.payee_name
      ?? next.receiver_holder_name
      ?? next.to_name
      ?? next.target_name;
    if (text(receiverName)) next.receiver_name = receiverName;
  }
  if (!hasOwn(next, "date_start") && hasOwn(next, "start_date")) {
    next.date_start = next.start_date;
  }
  if (!hasOwn(next, "date_end") && hasOwn(next, "end_date")) {
    next.date_end = next.end_date;
  }
  delete next.from_name;
  delete next.source_name;
  delete next.counterparty_name;
  delete next.payee_name;
  delete next.receiver_holder_name;
  delete next.to_name;
  delete next.target_name;
  delete next.start_date;
  delete next.end_date;
  return next;
}

function normalizeProbeArgs(args) {
  const next = { ...objectOf(args) };
  delete next.match_mode;
  return next;
}

function normalizeFullCaseArgs(args) {
  const source = objectOf(args);
  const next = {};
  for (const key of [
    "case_id",
    "account_keys",
    "include_internal_playbooks",
    "write_report",
    "force_refresh"
  ]) {
    if (hasOwn(source, key)) {
      next[key] = source[key];
    }
  }
  if (hasOwn(source, "max_accounts")) {
    next.max_accounts = clampInt(source.max_accounts, 12, 1, 12);
  }
  return preserveInternalRuntimeContext(source, next);
}

function normalizeEvidencePackArgs(args) {
  const source = objectOf(args);
  const next = {};
  for (const key of [
    "case_id",
    "account_keys",
    "txn_ids"
  ]) {
    if (hasOwn(source, key)) {
      next[key] = source[key];
    }
  }
  return preserveInternalRuntimeContext(source, next);
}

export function normalizeSkillArgs(name, args) {
  if (name === "plan_case_analysis") {
    return normalizePlanArgs(args);
  }
  if (RANKING_SKILL_NAMES.has(name)) {
    return normalizeRankingArgs(name, args);
  }
  if (name === "investigate_pair_amount") {
    return normalizePairAmountArgs(args);
  }
  if (name === "hypothesis_probe" || name === "run_discovery_scan" || name === "trace_holder_destinations" || name === "detect_cash_breakpoints" || name === "detect_financial_product_flows" || name === "detect_project_litigation_asset_leads" || name === "generate_followup_investigation_list") {
    return normalizeProbeArgs(args);
  }
  if (name === "run_full_case_analysis") {
    return normalizeFullCaseArgs(args);
  }
  if (name === "get_evidence_pack") {
    return normalizeEvidencePackArgs(args);
  }
  if (name === "trace_fund_next_hop") {
    return normalizeTraceNextHopArgs(args);
  }
  if (name === "trace_fund") {
    return normalizeTraceFundArgs(args);
  }
  if (name === "resolve_owner_scope") {
    const next = { ...objectOf(args) };
    delete next.match_mode;
    return next;
  }
  return { ...objectOf(args) };
}

export function dataOf(payload) {
  if (payload && typeof payload === "object" && Object.hasOwn(payload, "data")) {
    return payload.data || {};
  }
  return payload || {};
}

export function intValue(value, fallback = undefined) {
  const next = intOrUndefined(value);
  return next === undefined ? fallback : next;
}

export function clampInt(value, fallback, min, max) {
  const next = intValue(value, fallback);
  return Math.min(max, Math.max(min, next));
}

export function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

export function uniqueTexts(values, limit = 8) {
  const seen = new Set();
  const output = [];
  for (const value of arrayOf(values)) {
    const item = text(value);
    if (!item || seen.has(item)) continue;
    seen.add(item);
    output.push(item);
    if (output.length >= limit) break;
  }
  return output;
}

export function pruneEmpty(value) {
  if (Array.isArray(value)) {
    return value
      .map(pruneEmpty)
      .filter((item) => item !== undefined);
  }
  if (!value || typeof value !== "object") {
    return value === "" || value === null ? undefined : value;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    const next = pruneEmpty(item);
    if (next !== undefined) {
      output[key] = next;
    }
  }
  return Object.keys(output).length ? output : undefined;
}

export function sumNumberRows(rows, key) {
  const source = arrayOf(rows);
  if (!source.length) return undefined;
  let sum = 0;
  for (const row of source) {
    const next = numberOrUndefined(objectOf(row)[key]);
    if (next === undefined) return undefined;
    sum += next;
    if (!Number.isFinite(sum)) return undefined;
  }
  return sum;
}
