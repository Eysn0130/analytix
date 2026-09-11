import { stableHash } from "./stable-hash.mjs";
import {
  arrayOf,
  objectOf,
  pruneEmpty,
  text
} from "./runtime-normalizers.mjs";

export const DIAGNOSTIC_FACT_HELPERS_VERSION = "0.14.3";

export function numericValue(value) {
  if (value && typeof value === "object") {
    const source = objectOf(value);
    return numericValue(source.yuan ?? source.amount_yuan ?? source.amount ?? source.value);
  }
  const normalized = String(value ?? "").replace(/[,，\s]/gu, "");
  if (!normalized) return null;
  const next = Number(normalized);
  return Number.isFinite(next) ? next : null;
}

export function amountFromDiagnosticRow(row, base) {
  const source = objectOf(row);
  for (const candidate of [
    source[`${base}_yuan`],
    source[`${base}_total`],
    source[base],
    objectOf(source[base]).yuan,
    source.amount_yuan,
    source.amount,
    source.value
  ]) {
    const value = numericValue(candidate);
    if (value != null) return value;
  }
  return undefined;
}

export function diagnosticRowName(row, keys) {
  const source = objectOf(row);
  for (const key of keys) {
    const value = text(source[key]);
    if (value) return value;
  }
  return "";
}

export function diagnosticRowTimeRange(row) {
  const source = objectOf(row);
  const first = text(source.first_txn_at || source.first_time || source.date_min);
  const last = text(source.last_txn_at || source.last_time || source.date_max);
  return first || last ? `${first}~${last}` : "";
}

export function diagnosticCounterpartyName(row) {
  const source = objectOf(row);
  const key = diagnosticRowName(source, ["counterparty_key", "display_name", "counterparty_name", "holder_name"]);
  const account = diagnosticRowName(source, ["counterparty_account", "counterparty_acct_norm"]);
  if (key.startsWith("__name__:")) return key.slice("__name__:".length);
  if (key === "__unknown__" || key === "unknown") return "空户名";
  if (text(source.counterparty_type) === "account_only" && account) return `空户名/账号:${account}`;
  if (text(source.counterparty_group_mode) === "name" && account && /^\d{8,}$/u.test(key)) return `空户名/账号:${account}`;
  return key;
}

export function diagnosticSourceHash(supportQueryName, sourcePayload) {
  return stableHash({
    support_query_name: supportQueryName,
    data: objectOf(sourcePayload).data || objectOf(sourcePayload).result || sourcePayload
  });
}

export function makeDiagnosticFact(options = {}) {
  const source = objectOf(options);
  const {
    factId,
    label
  } = source;
  return pruneEmpty({
    fact_id: factId,
    label,
    support_status: "unsupported",
    fact_answer_allowed: false,
    publication_readiness: "blocked",
    blocker: "host_evidence_receipt_required"
  }) || {};
}

export function diagnosticSkillWarnings(results) {
  return arrayOf(results)
    .filter((item) => item && item.ok === false)
    .map((item) => `${text(item.skill_id)} failed: ${text(objectOf(arrayOf(item.warnings)[0]).message || item.error || "unknown error")}`);
}

export function diagnosticCallSummaries(results) {
  return arrayOf(results).map((item) => pruneEmpty({
    tool: text(item.skill_id),
    status: item.ok === false ? "failed" : "ok",
    holder_name: text(item.input?.holder_name),
    direction_mode: text(item.input?.direction_mode),
    metric: text(item.input?.metric),
    group_mode: text(item.input?.counterparty_group_mode),
    date_start: text(item.input?.date_start),
    date_end: text(item.input?.date_end)
  })).filter(Boolean);
}

export function claimProbeSummary(probe) {
  return objectOf(objectOf(objectOf(probe).pattern_summary).claim_review_summary);
}

export function probePatternSummary(probe) {
  return objectOf(objectOf(probe).pattern_summary);
}

export function namedCounterpartySummary(probe, keyword) {
  const summaries = arrayOf(claimProbeSummary(probe).named_counterparty_summaries);
  return objectOf(summaries.find((item) => text(item.keyword) === text(keyword)));
}

export function financialLead(probe, matcher) {
  return objectOf(arrayOf(probePatternSummary(probe).financial_product_leads).find((item) => matcher(objectOf(item))));
}

export function compactFinancialProductLeads(probe, limit = 6) {
  void probe;
  void limit;
  return [];
}

export function duplicateGuard(probe, counterpartyName) {
  return objectOf(arrayOf(claimProbeSummary(probe).holder_to_counterparty_duplicate_guards).find((item) =>
    text(item.counterparty_name) === text(counterpartyName)
  ));
}

export function unitDriftCandidate(probe) {
  const candidates = arrayOf(probePatternSummary(probe).unit_drift_candidates).map(objectOf);
  return objectOf(
    candidates.find((item) => arrayOf(item.business_texts).join("|").includes("理财")) ||
    candidates.find((item) => numericValue(item.amount) != null) ||
    {}
  );
}

export function duplicateSeedCandidate(probe, fallbackTxnId = "") {
  const duplicateGroups = arrayOf(probePatternSummary(probe).duplicate_txn_id_groups).map(objectOf);
  const unit = unitDriftCandidate(probe);
  const unitTxnId = text(unit.txn_id);
  const matched = duplicateGroups.find((item) => text(item.txn_id) === unitTxnId);
  return matched ? objectOf(matched) : { txn_id: unitTxnId || text(fallbackTxnId) };
}

export function duplicateCandidateSummary(duplicateResult) {
  void duplicateResult;
  return {
    summary_status: "unresolved",
    fact_answer_allowed: false,
    publication_readiness: "blocked",
    blocker: "host_evidence_receipt_required"
  };
}
