(() => {
  const BANK_ACCOUNT_FIELDS = new Set([
    "account", "account_key", "account_no", "account_number", "acct_card_no", "acct_no", "bank_account",
    "card_no", "counterparty_account", "counterparty_acct", "fc_account", "fc_sub_account",
    "parent_acct", "retrieval_account", "sub_acct", "subject_account", "source_account_no", "target_account_no",
    "交易卡号", "查询卡号", "本方卡号", "银行卡号",
    "银行账号", "账卡号", "卡号", "交易账号", "查询账号", "查询帐号", "本方账号", "账户账号", "账户号",
    "账号", "帐号", "交易对手账卡号", "交易对方账卡号", "交易对方帐卡号", "交易对方账号",
    "交易对方卡号", "对方账号", "对方卡号", "对手账号", "对手卡号", "冻结账号",
  ]);
  const IDENTITY_FIELDS = new Set([
    "agent_id_no", "counterparty_id_no", "id_or_acct_no", "id_no", "identity_no", "identity_number", "license_no", "local_tax_no",
    "nat_tax_no", "opener_id_no", "代办人证件号码", "对手身份证号", "交易对方证件号码", "交易对方证件号",
    "对方证件号码", "对方证件号", "对手证件号", "开户人证件号码", "身份证号", "身份证号码", "证件号",
    "证件号码", "证照号码",
  ]);
  const PHONE_FIELDS = new Set([
    "contact_phone", "home_phone", "mobile", "mobile_no", "org_phone", "phone", "phone_no", "telephone",
    "手机", "手机号", "手机号码", "电话", "电话号码", "联系电话",
  ]);
  const IP_FIELDS = new Set(["ip", "ip_addr", "ip_address", "ip地址"]);
  const MAC_FIELDS = new Set(["imei", "imei地址", "mac", "mac_addr", "mac_address", "mac地址", "mac或imei地址", "mac_imei地址"]);
  const SAFE_NUMERIC_FIELDS = new Set([
    "amount", "available_balance", "balance", "counterparty_balance", "count", "row_count", "total_amount",
    "total_count", "txn_count",
  ]);
  const SAFE_TIME_FIELDS = new Set([
    "close_date", "created_at", "date", "date_end", "date_start", "end_date", "feedback_time", "first_time",
    "last_time", "last_txn_time", "open_time", "send_time", "start_date", "store_time", "timestamp", "txn_day",
    "txn_time", "updated_at",
  ]);
  const SAFE_FREE_TEXT_FIELDS = new Set(["description", "memo", "note", "remark"]);
  const IDENTIFIER_FIELDS = new Set(["id", "item_id", "node_id", "source_id", "target_id"]);
  const FORMAT_CHARACTERS = /\p{Cf}/gu;
  const IDENTIFIER_SEPARATORS = /[\s\-‐‑‒–—―_/\\.·•]+/gu;

  function normalizeFieldName(fieldName) {
    return String(fieldName == null ? "" : fieldName)
      .normalize("NFKC")
      .replace(FORMAT_CHARACTERS, "")
      .trim()
      .replace(/([a-z0-9])([A-Z])/g, "$1_$2")
      .toLowerCase()
      .replace(/[\s./\\-]+/g, "_")
      .replace(/^_+|_+$/g, "");
  }

  function fieldNameMatches(normalized, names) {
    return names.has(normalized);
  }

  function canonicalizeDetectionText(value) {
    return Array.from(String(value == null ? "" : value).replace(FORMAT_CHARACTERS, ""))
      .map((char) => {
        const compatible = char.normalize("NFKC");
        return /^[0-9A-Za-z]$/.test(compatible) || /^[-_/\\.]$/.test(compatible) ? compatible : char;
      })
      .join("")
      .trim();
  }

  function compactIdentifier(value) {
    return value.replace(IDENTIFIER_SEPARATORS, "");
  }

  function maskSignificant(value, leading, trailing) {
    const chars = Array.from(value);
    const indexes = chars.map((char, index) => (/^[\p{L}\p{N}]$/u.test(char) ? index : -1)).filter((index) => index >= 0);
    if (!indexes.length) return value ? "*".repeat(chars.length) : "";
    let visibleLeading = Math.max(0, Math.floor(leading));
    let visibleTrailing = Math.max(0, Math.floor(trailing));
    if (indexes.length <= visibleLeading + visibleTrailing) {
      visibleLeading = indexes.length > 1 ? 1 : 0;
      visibleTrailing = indexes.length > 1 ? 1 : 0;
    }
    const hiddenStart = visibleLeading;
    const hiddenEnd = indexes.length - visibleTrailing;
    if (hiddenEnd <= hiddenStart) {
      chars[indexes[Math.floor(indexes.length / 2)]] = "*";
      return chars.join("");
    }
    for (let index = hiddenStart; index < hiddenEnd; index += 1) chars[indexes[index]] = "*";
    return chars.join("");
  }

  function maskIp(value) {
    const ipv4 = value.split(".");
    if (ipv4.length === 4 && ipv4.every((part) => /^\d{1,3}$/.test(part) && Number(part) >= 0 && Number(part) <= 255)) {
      return `${ipv4[0]}.${ipv4[1]}.*.*`;
    }
    if (value.includes(":")) {
      let visible = 0;
      return value.split(":").map((part) => {
        if (!part) return "";
        visible += 1;
        return visible <= 2 ? part : "*";
      }).join(":");
    }
    return maskSignificant(value, 2, 0);
  }

  function maskMac(value) {
    const separator = value.includes("-") ? "-" : ":";
    const parts = value.split(separator);
    if (parts.length === 6 && parts.every((part) => /^[0-9A-Fa-f]{2}$/.test(part))) {
      return parts.slice(0, 3).concat(["**", "**", "**"]).join(separator);
    }
    return maskSignificant(value, 4, 0);
  }

  function resolveKind(fieldName) {
    const normalized = normalizeFieldName(fieldName);
    if (!normalized) return null;
    if (fieldNameMatches(normalized, BANK_ACCOUNT_FIELDS)) return "bank-account";
    if (fieldNameMatches(normalized, IDENTITY_FIELDS)) return "identity-number";
    if (fieldNameMatches(normalized, PHONE_FIELDS)) return "phone-number";
    if (fieldNameMatches(normalized, MAC_FIELDS)) return "mac-address";
    if (fieldNameMatches(normalized, IP_FIELDS)) return "ip-address";
    return null;
  }

  function projectRestricted(value, kind) {
    const text = canonicalizeDetectionText(value);
    if (!text) return "";
    if (text.includes("*")) {
      const visible = Array.from(text).filter((char) => /^[\p{L}\p{N}]$/u.test(char)).length;
      const safeVisibleLimit = kind === "bank-account" ? 8 : kind === "identity-number" || kind === "phone-number" ? 7 : 8;
      if (visible <= safeVisibleLimit) return text;
    }
    if (kind === "bank-account") return maskSignificant(text, 4, 4);
    if (kind === "identity-number") return maskSignificant(text, 3, 4);
    if (kind === "phone-number") return maskSignificant(text, 3, 4);
    if (kind === "ip-address") return maskIp(text);
    if (kind === "mac-address") return maskMac(text);
    return "";
  }

  function replaceContextualBankAccounts(text) {
    const projected = text.replace(
      /((?:银行)?(?:账号|帐号|账户|卡号|账卡号)\s*[:：]?\s*)(\p{N}(?:[\s\-‐‑‒–—―_/\\.·•]*\p{N}){7,})/gu,
      (_match, prefix, pii) => `${prefix}${projectRestricted(pii, "bank-account")}`
    );
    return projected.replace(
      /((?:银行)?(?:账号|帐号|账户|卡号|账卡号)\s*[:：]?\s*)(\d+(?:\.\d+)?[eE][+-]?\d+)/gu,
      (_match, prefix, pii) => `${prefix}${projectRestricted(pii, "bank-account")}`
    );
  }

  function scientificNumberRanges(text) {
    return Array.from(text.matchAll(/[+-]?\d+(?:\.\d+)?[eE][+-]?\d+/g)).map((match) => ({
      start: match.index,
      end: match.index + match[0].length,
    }));
  }

  function projectDetectedInternal(value, allowBareBankAccount) {
    const original = String(value == null ? "" : value).trim();
    if (!original) return "";
    const text = canonicalizeDetectionText(original);
    const compact = compactIdentifier(text);
    if (/^(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}$/.test(text)) return projectRestricted(text, "mac-address");
    if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(text) || (text.includes(":") && /^[0-9A-Fa-f:]+$/.test(text))) {
      return projectRestricted(text, "ip-address");
    }
    if (/^\d{17}[0-9Xx]$/.test(compact)) return projectRestricted(text, "identity-number");
    if (/^1\d{10}$/.test(compact)) return projectRestricted(text, "phone-number");
    if (allowBareBankAccount && /^\p{N}{12,64}$/u.test(compact)) return projectRestricted(text, "bank-account");

    let projected = text;
    projected = projected.replace(
      /(^|[^0-9A-Fa-f])((?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2})(?=$|[^0-9A-Fa-f])/g,
      (_match, prefix, pii) => `${prefix}${projectRestricted(pii, "mac-address")}`
    );
    projected = projected.replace(
      /(^|[^0-9])(\d{1,3}(?:\.\d{1,3}){3})(?=$|[^0-9])/g,
      (_match, prefix, pii) => `${prefix}${projectRestricted(pii, "ip-address")}`
    );
    projected = projected.replace(
      /(^|[^0-9A-Za-z])(\d(?:[\s\-‐‑‒–—―_/\\.·•]*\d){16}[\s\-‐‑‒–—―_/\\.·•]*[0-9Xx])(?=$|[^0-9A-Za-z])/gu,
      (_match, prefix, pii) => `${prefix}${projectRestricted(pii, "identity-number")}`
    );
    projected = projected.replace(
      /(^|[^0-9])(1(?:[\s\-‐‑‒–—―_/\\.·•]*\d){10})(?=$|[^0-9])/gu,
      (_match, prefix, pii) => `${prefix}${projectRestricted(pii, "phone-number")}`
    );
    if (allowBareBankAccount) {
      const scientificRanges = scientificNumberRanges(projected);
      projected = projected.replace(
        /(^|[^\p{N}])(\p{N}(?:[\s\-‐‑‒–—―_/\\.·•]*\p{N}){11,63})(?=$|[^\p{N}])/gu,
        (match, prefix, pii, offset) => {
          const start = offset + prefix.length;
          const end = start + pii.length;
          if (scientificRanges.some((range) => start < range.end && end > range.start)) return match;
          return `${prefix}${projectRestricted(pii, "bank-account")}`;
        }
      );
    }
    return replaceContextualBankAccounts(projected);
  }

  function projectDetected(value) {
    return projectDetectedInternal(value, true);
  }

  function isPlausibleTimeValue(value) {
    if (!value) return true;
    if (/^\d{4}-\d{2}-\d{2}(?:[T\s]\d{2}:\d{2}(?::\d{2}(?:\.\d{1,9})?)?(?:Z|[+-]\d{2}:?\d{2})?)?$/.test(value)) {
      return true;
    }
    const compact = value.replace(/[^0-9]/g, "");
    if (compact.length !== 8 && compact.length !== 14) return false;
    const year = Number(compact.slice(0, 4));
    const month = Number(compact.slice(4, 6));
    const day = Number(compact.slice(6, 8));
    if (year < 1900 || year > 2200 || month < 1 || month > 12 || day < 1 || day > 31) return false;
    if (compact.length === 14) {
      const hour = Number(compact.slice(8, 10));
      const minute = Number(compact.slice(10, 12));
      const second = Number(compact.slice(12, 14));
      if (hour > 23 || minute > 59 || second > 59) return false;
    }
    return true;
  }

  function projectField(fieldName, value) {
    const kind = resolveKind(fieldName);
    if (kind) return projectRestricted(value, kind);
    const normalized = normalizeFieldName(fieldName);
    if (fieldNameMatches(normalized, SAFE_TIME_FIELDS)) {
      const text = canonicalizeDetectionText(value);
      return isPlausibleTimeValue(text) ? text : projectDetected(text);
    }
    if (fieldNameMatches(normalized, SAFE_NUMERIC_FIELDS)) return projectDetected(value);
    if (fieldNameMatches(normalized, SAFE_FREE_TEXT_FIELDS)) return projectDetected(value);
    if (fieldNameMatches(normalized, IDENTIFIER_FIELDS)) {
      const text = canonicalizeDetectionText(value);
      if (/^\d{8,}$/.test(compactIdentifier(text))) return projectRestricted(text, "bank-account");
    }
    return projectDetected(value);
  }

  window.__ANALYTIX_ORDINARY_PII_PROJECTION__ = Object.freeze({
    normalizeFieldName,
    resolveKind,
    projectRestricted,
    projectField,
    projectDetected,
  });
})();
