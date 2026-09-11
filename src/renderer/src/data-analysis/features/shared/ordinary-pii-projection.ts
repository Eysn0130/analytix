export type RestrictedPiiKind = "bank-account" | "identity-number" | "phone-number" | "ip-address" | "mac-address";

const BANK_ACCOUNT_FIELD_NAMES = new Set([
  "account",
  "account_key",
  "account_no",
  "account_number",
  "acct_card_no",
  "acct_no",
  "bank_account",
  "card_no",
  "counterparty_account",
  "counterparty_acct",
  "fc_account",
  "fc_sub_account",
  "parent_acct",
  "retrieval_account",
  "sub_acct",
  "subject_account",
  "source_account_no",
  "target_account_no",
  "交易卡号",
  "查询卡号",
  "本方卡号",
  "银行卡号",
  "银行账号",
  "账卡号",
  "卡号",
  "交易账号",
  "查询账号",
  "查询帐号",
  "本方账号",
  "账户账号",
  "账户号",
  "账户",
  "账号",
  "帐号",
  "交易对手账卡号",
  "交易对方账卡号",
  "交易对方帐卡号",
  "交易对方账号",
  "交易对方卡号",
  "对方账号",
  "对方卡号",
  "对手账号",
  "对手卡号",
  "冻结账号",
]);

const IDENTITY_FIELD_NAMES = new Set([
  "agent_id_no",
  "counterparty_id_no",
  "id_or_acct_no",
  "id_no",
  "identity_no",
  "identity_number",
  "license_no",
  "local_tax_no",
  "nat_tax_no",
  "opener_id_no",
  "代办人证件号码",
  "对手身份证号",
  "交易对方证件号码",
  "交易对方证件号",
  "对方证件号码",
  "对方证件号",
  "对手证件号",
  "开户人证件号码",
  "身份证号",
  "身份证号码",
  "证件号",
  "证件号码",
  "证照号码",
]);

const PHONE_FIELD_NAMES = new Set([
  "contact_phone",
  "home_phone",
  "mobile",
  "mobile_no",
  "org_phone",
  "phone",
  "phone_no",
  "telephone",
  "手机",
  "手机号",
  "手机号码",
  "电话",
  "电话号码",
  "联系电话",
]);

const IP_FIELD_NAMES = new Set(["ip", "ip_addr", "ip_address", "ip地址"]);
const MAC_FIELD_NAMES = new Set([
  "imei",
  "imei地址",
  "mac",
  "mac_addr",
  "mac_address",
  "mac地址",
  "mac或imei地址",
  "mac_imei地址",
]);

const SAFE_NUMERIC_FIELD_NAMES = new Set([
  "amount",
  "available_balance",
  "balance",
  "counterparty_balance",
  "count",
  "row_count",
  "total_amount",
  "total_count",
  "txn_count",
]);

const SAFE_TIME_FIELD_NAMES = new Set([
  "close_date",
  "created_at",
  "date",
  "date_end",
  "date_start",
  "end_date",
  "feedback_time",
  "first_time",
  "last_time",
  "last_txn_time",
  "open_time",
  "send_time",
  "start_date",
  "store_time",
  "timestamp",
  "txn_day",
  "txn_time",
  "updated_at",
]);

const SAFE_FREE_TEXT_FIELD_NAMES = new Set(["description", "memo", "note", "remark"]);

const IDENTIFIER_FIELD_NAMES = new Set([
  "id",
  "item_id",
  "node_id",
  "source_id",
  "target_id",
]);

const UNCONTROLLED_EXPORT_BLOCK_REASON = "当前没有可验证的受控敏感信息导出授权，普通完整导出已安全阻止。";
const FORMAT_CHARACTERS = /\p{Cf}/gu;
const IDENTIFIER_SEPARATORS = /[\s\-‐‑‒–—―_/\\.·•]+/gu;

export function normalizeOrdinaryPresentationFieldName(fieldName: unknown): string {
  return String(fieldName ?? "")
    .normalize("NFKC")
    .replace(FORMAT_CHARACTERS, "")
    .trim()
    .replace(/([a-z0-9])([A-Z])/g, "$1_$2")
    .toLowerCase()
    .replace(/[\s./\\-]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

function fieldNameMatches(normalized: string, names: ReadonlySet<string>): boolean {
  return names.has(normalized);
}

function canonicalizeDetectionText(value: unknown): string {
  return Array.from(String(value ?? "").replace(FORMAT_CHARACTERS, ""))
    .map((char) => {
      const compatible = char.normalize("NFKC");
      return /^[0-9A-Za-z]$/.test(compatible) || /^[-_/\\.]$/.test(compatible) ? compatible : char;
    })
    .join("")
    .trim();
}

function compactIdentifier(value: string): string {
  return value.replace(IDENTIFIER_SEPARATORS, "");
}

function maskSignificantCharacters(value: string, leading: number, trailing: number): string {
  const chars = Array.from(value);
  const significantIndexes = chars
    .map((char, index) => (/^[\p{L}\p{N}]$/u.test(char) ? index : -1))
    .filter((index) => index >= 0);
  if (significantIndexes.length === 0) {
    return value ? "*".repeat(chars.length) : "";
  }

  let visibleLeading = Math.max(0, Math.floor(leading));
  let visibleTrailing = Math.max(0, Math.floor(trailing));
  if (significantIndexes.length <= visibleLeading + visibleTrailing) {
    visibleLeading = significantIndexes.length > 1 ? 1 : 0;
    visibleTrailing = significantIndexes.length > 1 ? 1 : 0;
  }
  const hiddenStart = visibleLeading;
  const hiddenEnd = significantIndexes.length - visibleTrailing;
  if (hiddenEnd <= hiddenStart) {
    chars[significantIndexes[Math.floor(significantIndexes.length / 2)]] = "*";
    return chars.join("");
  }
  for (let index = hiddenStart; index < hiddenEnd; index += 1) {
    chars[significantIndexes[index]] = "*";
  }
  return chars.join("");
}

function maskIpAddress(value: string): string {
  const ipv4Parts = value.split(".");
  if (
    ipv4Parts.length === 4 &&
    ipv4Parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) >= 0 && Number(part) <= 255)
  ) {
    return `${ipv4Parts[0]}.${ipv4Parts[1]}.*.*`;
  }
  if (value.includes(":")) {
    const parts = value.split(":");
    let visibleGroups = 0;
    return parts
      .map((part) => {
        if (!part) {
          return "";
        }
        visibleGroups += 1;
        return visibleGroups <= 2 ? part : "*";
      })
      .join(":");
  }
  return maskSignificantCharacters(value, 2, 0);
}

function maskMacAddress(value: string): string {
  const separator = value.includes("-") ? "-" : ":";
  const parts = value.split(separator);
  if (parts.length === 6 && parts.every((part) => /^[0-9A-Fa-f]{2}$/.test(part))) {
    return [...parts.slice(0, 3), "**", "**", "**"].join(separator);
  }
  return maskSignificantCharacters(value, 4, 0);
}

export function resolveRestrictedPiiKind(fieldName: unknown): RestrictedPiiKind | null {
  const normalized = normalizeOrdinaryPresentationFieldName(fieldName);
  if (!normalized) {
    return null;
  }
  if (fieldNameMatches(normalized, BANK_ACCOUNT_FIELD_NAMES)) {
    return "bank-account";
  }
  if (fieldNameMatches(normalized, IDENTITY_FIELD_NAMES)) {
    return "identity-number";
  }
  if (fieldNameMatches(normalized, PHONE_FIELD_NAMES)) {
    return "phone-number";
  }
  if (fieldNameMatches(normalized, MAC_FIELD_NAMES)) {
    return "mac-address";
  }
  if (fieldNameMatches(normalized, IP_FIELD_NAMES)) {
    return "ip-address";
  }
  return null;
}

function isSafeNumericField(fieldName: unknown): boolean {
  return fieldNameMatches(normalizeOrdinaryPresentationFieldName(fieldName), SAFE_NUMERIC_FIELD_NAMES);
}

function isSafeTimeField(fieldName: unknown): boolean {
  return fieldNameMatches(normalizeOrdinaryPresentationFieldName(fieldName), SAFE_TIME_FIELD_NAMES);
}

function isIdentifierField(fieldName: unknown): boolean {
  return fieldNameMatches(normalizeOrdinaryPresentationFieldName(fieldName), IDENTIFIER_FIELD_NAMES);
}

function isSafeFreeTextField(fieldName: unknown): boolean {
  return fieldNameMatches(normalizeOrdinaryPresentationFieldName(fieldName), SAFE_FREE_TEXT_FIELD_NAMES);
}

export function projectOrdinaryRestrictedPii(value: unknown, kind: RestrictedPiiKind): string {
  // A JavaScript number cannot be authoritative PII: leading zeroes are lost
  // and account/identity values beyond 2^53 may already be rounded. Ordinary
  // surfaces therefore emit only a typed placeholder. Controlled artifacts
  // must render the original evidence string after authorization instead.
  if (typeof value === "number") {
    if (kind === "bank-account") return "[ACCOUNT]";
    if (kind === "identity-number") return "[IDENTITY]";
    if (kind === "phone-number") return "[PHONE]";
    if (kind === "ip-address") return "[IP]";
    return "[DEVICE]";
  }
  const text = canonicalizeDetectionText(value);
  if (!text) {
    return "";
  }
  if (text.includes("*")) {
    const hidden = Array.from(text).filter((char) => char === "*").length;
    const visible = Array.from(text).filter((char) => /^[\p{L}\p{N}]$/u.test(char)).length;
    const safeVisibleLimit = kind === "bank-account" ? 8 : kind === "identity-number" || kind === "phone-number" ? 7 : 8;
    if (hidden >= 4 && visible <= safeVisibleLimit) {
      return text;
    }
  }
  switch (kind) {
    case "bank-account":
      return maskSignificantCharacters(text, 4, 4);
    case "identity-number":
      return maskSignificantCharacters(text, 3, 4);
    case "phone-number":
      return maskSignificantCharacters(text, 3, 4);
    case "ip-address":
      return maskIpAddress(text);
    case "mac-address":
      return maskMacAddress(text);
  }
}

function replaceContextualBankAccounts(text: string): string {
  const projected = text.replace(
    /((?:(?:银行)?(?:账号|帐号|账户|卡号|账卡号)|(?:bank\s*)?(?:account|acct|card)(?:[\s_-]*(?:id|no|number))?)\s*[:：=]?\s*)(\p{N}(?:[\s\-‐‑‒–—―_/\\.·•*]*\p{N}){7,})/giu,
    (_match, prefix: string, pii: string) => `${prefix}${projectOrdinaryRestrictedPii(pii, "bank-account")}`
  );
  return projected.replace(
    /((?:银行)?(?:账号|帐号|账户|卡号|账卡号)\s*[:：]?\s*)(\d+(?:\.\d+)?[eE][+-]?\d+)/gu,
    (_match, prefix: string, pii: string) => `${prefix}${projectOrdinaryRestrictedPii(pii, "bank-account")}`
  );
}

function scientificNumberRanges(text: string): Array<{ start: number; end: number }> {
  return Array.from(text.matchAll(/[+-]?\d+(?:\.\d+)?[eE][+-]?\d+/g)).map((match) => ({
    start: match.index,
    end: match.index + match[0].length,
  }));
}

function projectDetectedOrdinaryRestrictedPiiInternal(value: unknown, allowBareBankAccount: boolean): string {
  const original = String(value ?? "").trim();
  if (!original) {
    return "";
  }
  const text = canonicalizeDetectionText(original);
  const compact = compactIdentifier(text);
  if (/^(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}$/.test(text)) {
    return projectOrdinaryRestrictedPii(text, "mac-address");
  }
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(text) || (text.includes(":") && /^[0-9A-Fa-f:]+$/.test(text))) {
    return projectOrdinaryRestrictedPii(text, "ip-address");
  }
  if (/^\d{17}[0-9Xx]$/.test(compact)) {
    return projectOrdinaryRestrictedPii(text, "identity-number");
  }
  if (/^1\d{10}$/.test(compact)) {
    return projectOrdinaryRestrictedPii(text, "phone-number");
  }
  if (allowBareBankAccount && /^\p{N}{12,64}$/u.test(compact)) {
    return projectOrdinaryRestrictedPii(text, "bank-account");
  }

  let projected = text;
  projected = projected.replace(
    /(^|[^0-9A-Fa-f])((?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2})(?=$|[^0-9A-Fa-f])/g,
    (_match, prefix: string, pii: string) => `${prefix}${projectOrdinaryRestrictedPii(pii, "mac-address")}`
  );
  projected = projected.replace(
    /(^|[^0-9])(\d{1,3}(?:\.\d{1,3}){3})(?=$|[^0-9])/g,
    (_match, prefix: string, pii: string) => `${prefix}${projectOrdinaryRestrictedPii(pii, "ip-address")}`
  );
  projected = projected.replace(
    /(^|[^0-9A-Za-z])(\d(?:[\s\-‐‑‒–—―_/\\.·•]*\d){16}[\s\-‐‑‒–—―_/\\.·•]*[0-9Xx])(?=$|[^0-9A-Za-z])/gu,
    (_match, prefix: string, pii: string) => `${prefix}${projectOrdinaryRestrictedPii(pii, "identity-number")}`
  );
  projected = projected.replace(
    /(^|[^0-9])(1(?:[\s\-‐‑‒–—―_/\\.·•]*\d){10})(?=$|[^0-9])/gu,
    (_match, prefix: string, pii: string) => `${prefix}${projectOrdinaryRestrictedPii(pii, "phone-number")}`
  );
  if (allowBareBankAccount) {
    const scientificRanges = scientificNumberRanges(projected);
    projected = projected.replace(
      /(^|[^\p{N}])(\p{N}(?:[\s\-‐‑‒–—―_/\\.·•]*\p{N}){11,})(?=$|[^\p{N}])/gu,
      (match, prefix: string, pii: string, offset: number) => {
        const start = offset + prefix.length;
        const end = start + pii.length;
        if (scientificRanges.some((range) => start < range.end && end > range.start)) {
          return match;
        }
        return `${prefix}${projectOrdinaryRestrictedPii(pii, "bank-account")}`;
      }
    );
  }
  return replaceContextualBankAccounts(projected);
}

export function projectDetectedOrdinaryRestrictedPii(value: unknown): string {
  return projectDetectedOrdinaryRestrictedPiiInternal(value, true);
}

export function projectOrdinaryFieldValue(fieldName: unknown, value: unknown): string {
  const kind = resolveRestrictedPiiKind(fieldName);
  if (kind) {
    return projectOrdinaryRestrictedPii(value, kind);
  }
  if (isSafeTimeField(fieldName)) {
    const text = canonicalizeDetectionText(value);
    if (isPlausibleOrdinaryTimeValue(text)) {
      return text;
    }
    return projectDetectedOrdinaryRestrictedPii(text);
  }
  if (isSafeNumericField(fieldName)) {
    return projectDetectedOrdinaryRestrictedPii(value);
  }
  if (isSafeFreeTextField(fieldName)) {
    return projectDetectedOrdinaryRestrictedPii(value);
  }
  if (isIdentifierField(fieldName)) {
    const text = canonicalizeDetectionText(value);
    const compact = compactIdentifier(text);
    if (/^\d{8,}$/.test(compact)) {
      return projectOrdinaryRestrictedPii(text, "bank-account");
    }
  }
  return projectDetectedOrdinaryRestrictedPii(value);
}

function isPlausibleOrdinaryTimeValue(value: string): boolean {
  if (!value) {
    return true;
  }
  if (/^\d{4}-\d{2}-\d{2}(?:[T\s]\d{2}:\d{2}(?::\d{2}(?:\.\d{1,9})?)?(?:Z|[+-]\d{2}:?\d{2})?)?$/.test(value)) {
    return true;
  }
  const compact = value.replace(/[^0-9]/g, "");
  if (![8, 14].includes(compact.length)) {
    return false;
  }
  const year = Number(compact.slice(0, 4));
  const month = Number(compact.slice(4, 6));
  const day = Number(compact.slice(6, 8));
  if (year < 1900 || year > 2200 || month < 1 || month > 12 || day < 1 || day > 31) {
    return false;
  }
  if (compact.length === 14) {
    const hour = Number(compact.slice(8, 10));
    const minute = Number(compact.slice(10, 12));
    const second = Number(compact.slice(12, 14));
    if (hour > 23 || minute > 59 || second > 59) {
      return false;
    }
  }
  return true;
}

export function projectOrdinaryClipboardValue(fieldName: unknown, value: unknown): string {
  return projectOrdinaryFieldValue(fieldName, value);
}

function projectStructuredKey(key: string): string {
  const text = canonicalizeDetectionText(key);
  const compact = compactIdentifier(text);
  if (/^\d{8,}$/.test(compact)) {
    return `account:${projectOrdinaryRestrictedPii(text, "bank-account")}`;
  }
  return projectOrdinaryFieldValue("", text);
}

function projectStructuredValue(value: unknown, fieldName: unknown, ancestors: WeakSet<object>): unknown {
  const kind = resolveRestrictedPiiKind(fieldName);
  if (kind && (typeof value === "string" || typeof value === "number")) {
    return projectOrdinaryRestrictedPii(value, kind);
  }
  if (typeof value === "string") {
    return projectOrdinaryFieldValue(fieldName, value);
  }
  if (typeof value === "number") {
    if (isSafeTimeField(fieldName) && isPlausibleOrdinaryTimeValue(String(value))) {
      return value;
    }
    if ((isIdentifierField(fieldName) && Math.abs(value) >= 10_000_000) || Math.abs(value) >= 100_000_000_000) {
      return projectOrdinaryRestrictedPii(String(value), "bank-account");
    }
  }
  if (!value || typeof value !== "object") {
    return value;
  }
  if (ancestors.has(value)) {
    return "[Circular]";
  }

  ancestors.add(value);
  let projected: unknown;
  if (Array.isArray(value)) {
    projected = value.map((item) => projectStructuredValue(item, fieldName, ancestors));
  } else if (value instanceof Date) {
    projected = value.toISOString();
  } else {
    projected = Object.fromEntries(
      Object.entries(value).map(([key, item]) => [projectStructuredKey(key), projectStructuredValue(item, key, ancestors)])
    );
  }
  ancestors.delete(value);
  return projected;
}

export function projectOrdinaryStructuredValue(value: unknown): unknown {
  return projectStructuredValue(value, "", new WeakSet<object>());
}

export function protectOrdinaryCsvFormula(value: unknown): string {
  const text = String(value ?? "");
  let firstContentIndex = 0;
  while (firstContentIndex < text.length) {
    const char = text[firstContentIndex] || "";
    const code = text.charCodeAt(firstContentIndex);
    if (code <= 0x20 || code === 0x85 || /\p{Cf}/u.test(char)) {
      firstContentIndex += 1;
      continue;
    }
    break;
  }
  return "=+-@".includes(text[firstContentIndex] || "") ? `'${text}` : text;
}

export function projectOrdinaryCsvField(fieldName: unknown, value: unknown): string {
  return protectOrdinaryCsvFormula(projectOrdinaryFieldValue(fieldName, value));
}

export function uncontrolledRestrictedPiiExportBlockReason(): string {
  return UNCONTROLLED_EXPORT_BLOCK_REASON;
}
