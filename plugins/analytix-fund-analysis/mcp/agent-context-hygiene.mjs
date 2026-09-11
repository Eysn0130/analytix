import { translateUserVisibleText } from "./user-facing-language.mjs";

export const AGENT_CONTEXT_HYGIENE_VERSION = "0.14.4-agent-context-hygiene";

export const AGENT_CONTEXT_HYGIENE_GUARDS = [
  "strip_internal_query_ids",
  "strip_audit_ref",
  "strip_artifact_id",
  "strip_evidence_refs",
  "redact_local_paths",
  "mask_restricted_pii",
  "strip_reasoning_fields_and_markup",
  "depth_limit_fails_closed",
  "model_context_no_opaque_refs"
];

const REASONING_FIELD_KEYS = new Set([
  "analysis",
  "chain_of_thought",
  "chainofthought",
  "cot",
  "internal_analysis",
  "internal_reasoning",
  "internal_thinking",
  "reasoning",
  "reasoning_content",
  "reasoningcontent",
  "thought",
  "thoughts",
  "thinking",
  "thinking_content",
  "thinkingcontent",
]);

const AGENT_OPAQUE_REF_KEYS = new Set([
  "audit_ref",
  "detail_ref",
  "artifact_id",
  "artifact_status",
  "evidence_refs",
  "evidence_ref",
  "evidence_ledger",
  "query_ids",
  "query_id",
  "evidence_ids",
  "evidence_id",
  "path_ids",
  "path_id",
  "report_ids",
  "report_id",
  "edge_id",
  "scope_id",
  "owner_scope_id",
  "resolved_scope_id",
  "temp_scope_id",
  "casegraph_id"
]);

function text(value) {
  return String(value || "").trim();
}

function pruneEmpty(value) {
  if (Array.isArray(value)) {
    return value.map(pruneEmpty).filter((item) => item !== undefined);
  }
  if (!value || typeof value !== "object") {
    return value === "" || value === null ? undefined : value;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    const next = pruneEmpty(item);
    if (next !== undefined) output[key] = next;
  }
  return Object.keys(output).length ? output : undefined;
}

export function redactLocalPathText(value) {
  return String(value || "").replace(
    /(?:[A-Za-z]:\\(?:[^\\/:*?"<>|\r\n]+\\)+[^\\/:*?"<>|\r\n]*|\/(?:Users|Volumes|private|var|tmp|opt|home)\/[^\s"'<>，,;；)）\]]+)/gu,
    (match) => {
      const normalized = match.replace(/\\/gu, "/").replace(/\/+$/u, "");
      const leaf = normalized.split("/").filter(Boolean).at(-1) || "path";
      return `[local-path]/${leaf}`;
    }
  );
}

function maskedDigits(value, label) {
  if (label === "account") return "[account-redacted]";
  const digits = String(value || "").replace(/\D/gu, "");
  return `[${label}-redacted:${digits.slice(-4) || "hidden"}]`;
}

function stripReasoningMarkup(value) {
  let projected = String(value || "");
  for (const tag of ["analysis", "reasoning", "think", "thinking"]) {
    projected = projected
      .replace(new RegExp(`<${tag}\\b[^>]*>[\\s\\S]*?<\\/${tag}\\s*>`, "giu"), "[reasoning-redacted]")
      .replace(new RegExp(`<${tag}\\b[^>]*>[\\s\\S]*$`, "giu"), "[reasoning-redacted]")
      .replace(new RegExp(`<\\/${tag}\\s*>`, "giu"), "");
  }
  return projected;
}

export function redactRestrictedPiiText(value) {
  const pathRedacted = stripReasoningMarkup(redactLocalPathText(value));
  const detectionText = pathRedacted.normalize("NFKC").replace(/\p{Cf}/gu, "");
  const projected = detectionText
    .replace(/\b(?:[0-9A-F]{2}[:-]){5}[0-9A-F]{2}\b/giu, "[mac-redacted]")
    .replace(/\b(?:25[0-5]|2[0-4]\d|1?\d?\d)(?:\.(?:25[0-5]|2[0-4]\d|1?\d?\d)){3}\b/gu, "[ip-redacted]")
    .replace(/\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/giu, "[email-redacted]")
    .replace(/((?:(?:银行)?(?:账号|帐号|账户|银行卡号|卡号|账卡号)|(?:bank\s*)?(?:account|acct|card)(?:[\s_-]*(?:id|key|no|number))?)\s*[:：=]?\s*)(\d(?:[\s\-‐‑‒–—―_/\\.·•*]*\d){7,})/giu, (_match, label) => `${label}[account-redacted]`)
    .replace(/((?:(?:bank|payer|receiver|from|to|counterparty|source|target|destination|beneficiary)[_\s-]*)?(?:account|acct|card)(?:[_\s-]*(?:id|key|no|number))?\s*[:=]\s*)(?:[A-Z]{0,2}[A-Z0-9][ A-Z0-9-]{4,34}[A-Z0-9])/giu, (_match, label) => `${label}[account-redacted]`)
    .replace(/((?:身份证号?|证件号)\s*[:：]?\s*)\d(?:[\s\-‐‑‒–—―_/\\.·•]*\d){16}[\s\-‐‑‒–—―_/\\.·•]*[\dXx]/gu, (_match, label) => `${label}[id-redacted]`)
    .replace(/((?:手机号|联系电话|电话|座机)\s*[:：]?\s*)0\d{2,3}[\s\-‐‑‒–—―_/\\.·•]*\d{7,8}/gu, (_match, label) => `${label}[phone-redacted]`)
    .replace(/((?:手机号|联系电话|电话)\s*[:：]?\s*)1[3-9](?:[\s\-‐‑‒–—―_/\\.·•]*\d){9}/gu, (_match, label) => `${label}[phone-redacted]`)
    .replace(/((?:姓名)\s*[:：]?\s*)[\p{Script=Han}·]{2,12}/gu, (_match, label) => `${label}[name-redacted]`)
    .replace(/((?:住址|家庭地址|联系地址|地址)\s*[:：]?\s*)[^，。,；;\n]{4,80}/gu, (_match, label) => `${label}[address-redacted]`)
    .replace(/(?<!\d)\d(?:[\s\-‐‑‒–—―_/\\.·•]*\d){16}[\s\-‐‑‒–—―_/\\.·•]*[\dXx](?!\d)/gu, (match) => maskedDigits(match, "id"))
    .replace(/(?<!\d)1[3-9](?:[\s\-‐‑‒–—―_/\\.·•]*\d){9}(?!\d)/gu, (match) => maskedDigits(match, "phone"))
    .replace(/(?<!\d)0\d{2,3}[\s\-‐‑‒–—―_/\\.·•]*\d{7,8}(?!\d)/gu, (match) => maskedDigits(match, "phone"))
    .replace(/(?<!\d)\d(?:[\s\-‐‑‒–—―_/\\.·•]*\d){11,}(?!\d)/gu, (match) => maskedDigits(match, "account"));
  return projected === detectionText ? pathRedacted : projected;
}

function normalizedSensitiveFieldKey(key) {
  return String(key || "")
    .normalize("NFKC")
    .replace(/\p{Cf}/gu, "")
    .replace(/([a-z0-9])([A-Z])/gu, "$1_$2")
    .replace(/[^\p{L}\p{N}]+/gu, "_")
    .replace(/^_+|_+$/gu, "")
    .toLowerCase();
}

function isAccountFieldKey(key) {
  const normalized = normalizedSensitiveFieldKey(key);
  if (/^(?:银行卡号|银行账号|账卡号|卡号|账号|帐号|账户号|账户)$/u.test(normalized)) return true;
  return /^(?:(?:bank|payer|payee|receiver|sender|from|to|counterparty|source|target|destination|beneficiary|debit|credit)_)?(?:account|accounts|acct|accts|card|cards)(?:_(?:id|ids|key|keys|no|nos|number|numbers|list|values))?$/u.test(normalized);
}

function isReasoningFieldKey(key) {
  return REASONING_FIELD_KEYS.has(normalizedSensitiveFieldKey(key));
}

function redactAccountFieldValue(value, depth) {
  if (value == null || value === "" || typeof value === "boolean") return value;
  if (Array.isArray(value)) {
    return value.map((item) => redactAccountFieldValue(item, depth + 1));
  }
  if (value && typeof value === "object") {
    if (depth > 12) return "[redacted-depth-limit]";
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, redactAccountFieldValue(item, depth + 1)])
    );
  }
  return "[account-redacted]";
}

function redactSensitiveField(key, value, depth) {
  if (value == null || value === "") return value;
  if (/(?:id_?no|identity|身份证|证件号)/iu.test(key)) return maskedDigits(value, "id");
  if (/(?:phone|mobile|telephone|手机号|电话)/iu.test(key)) return maskedDigits(value, "phone");
  if (/(?:mac(?:_?addr)?)/iu.test(key)) return "[mac-redacted]";
  if (/(?:ip(?:_?addr)?)/iu.test(key)) return "[ip-redacted]";
  if (/(?:address|addr|住址|地址)/iu.test(key)) return "[address-redacted]";
  if (isAccountFieldKey(key)) return redactAccountFieldValue(value, depth + 1);
  return redactAgentPayload(value, depth + 1);
}

export function humanizeAgentText(value) {
  return redactRestrictedPiiText(translateUserVisibleText(text(value)
    .replace(/\bSAME_FACT_CROSS_ACCOUNT_DUPLICATE_CANDIDATES\b/gu, "同事实跨账户重复候选")
    .replace(/\bSAME_FACT_DUPLICATE_FAMILIES\b/gu, "同事实重复候选族")
    .replace(/\bSAME_HOLDER_SAME_FACT_DUPLICATE_FAMILIES\b/gu, "同户名同事实重复候选族")
    .replace(/同事实候选族\s+same_fact_[A-Za-z0-9_]+\s*/gu, "同事实候选族")
    .replace(/\bsame_fact_[A-Za-z0-9_]+\b/gu, "同事实候选族")
    .replace(/\bq_[0-9a-f]{8,}\b/giu, "")
    .replace(/\bmcp-full-[0-9a-f]{8,}\b/giu, "")
    .replace(/\bcasegraph:[\w:.-]+\b/giu, "")
    .replace(/\b(?:audit_ref|detail_ref|artifact_id|evidence_ref|evidence_refs|query_id|query_ids)\s*[:：]\s*[\w:.,;/-]*/giu, "")
    .replace(/引用分类\s*query_id/giu, "引用对应的分类事实")
    .replace(/query_id/giu, "内部查询编号")
    .replace(/\s{2,}/gu, " ")
    .replace(/\s+([，。；、,.])/gu, "$1")
    .trim()));
}

function shouldPreserveAgentCodeText(value) {
  const raw = text(value);
  return /\b(?:SQL-safe|sql_name|sql_identifier|column_name|table_name|SELECT|FROM|WHERE|GROUP BY|ORDER BY|LIMIT|account_open_name|analysis_txn_detail_idx|counterparty_name|cp_name_pick|dc_val|txn_time|txn_ts)\b/iu.test(raw);
}

export function redactAgentPayload(value, depth = 0) {
  if (depth > 12) {
    return "[redacted-depth-limit]";
  }
  if (typeof value === "string") {
    return redactRestrictedPiiText(value);
  }
  if (Array.isArray(value)) {
    return value.map((item) => redactAgentPayload(item, depth + 1));
  }
  if (!value || typeof value !== "object") {
    return value;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    if (isReasoningFieldKey(key)) continue;
    output[key] = redactSensitiveField(key, item, depth);
  }
  return output;
}

export function stripAgentOpaqueRefs(value, depth = 0) {
  if (depth > 12) {
    return "[redacted-depth-limit]";
  }
  if (typeof value === "string") {
    const scrubbed = shouldPreserveAgentCodeText(value)
      ? redactRestrictedPiiText(value).trim()
      : humanizeAgentText(value).trim();
    return scrubbed || undefined;
  }
  if (Array.isArray(value)) {
    return pruneEmpty(value.map((item) => stripAgentOpaqueRefs(item, depth + 1))) || [];
  }
  if (!value || typeof value !== "object") {
    return value;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    if (AGENT_OPAQUE_REF_KEYS.has(key) || isReasoningFieldKey(key)) {
      continue;
    }
    const next = stripAgentOpaqueRefs(item, depth + 1);
    if (next !== undefined) {
      output[key] = next;
    }
  }
  return Object.keys(output).length ? output : undefined;
}

export function hasOpaqueAgentRefs(value) {
  return /\b(?:q_[0-9a-f]{8,}|audit_ref|detail_ref|artifact_id|evidence_refs?|query_ids?|mcp-full-[0-9a-f]{8,})\b/iu.test(
    JSON.stringify(value || "")
  );
}
