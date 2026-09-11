import { numericValue } from "./diagnostic-fact-helpers.mjs";
import {
  arrayOf,
  booleanOrUndefined,
  intOrUndefined,
  numberOrUndefined,
  objectOf,
  pruneEmpty,
  text,
  uniqueTexts
} from "./runtime-normalizers.mjs";

export const FRONTDOOR_FACT_SUMMARIES_VERSION = "0.14.4";

export function moneyText(value) {
  if (value && typeof value === "object") {
    return moneyText(value.yuan ?? value.amount_yuan ?? value.amount ?? value.value);
  }
  const amount = numberOrUndefined(value);
  if (amount !== undefined) {
    return `${amount.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} 元`;
  }
  return "";
}

function countOrUndefined(value) {
  const number = intOrUndefined(value);
  return number !== undefined && number >= 0 ? number : undefined;
}

export function isNamedCounterparty(row, expectedName) {
  const item = objectOf(row);
  const expected = text(expectedName);
  if (!expected) return false;
  return [
    item.display_name,
    item.counterparty_name,
    item.holder_name,
    item.account_name
  ].map(text).some((value) => value === expected);
}

export function firstTxnDateForCounterparty(rankResult, expectedName) {
  if (!rankCoverageAllowsCandidate(rankResult)) return "";
  const rows = arrayOf(objectOf(rankResult).data?.rankings);
  const row = rows.find((item) => isNamedCounterparty(item, expectedName));
  const raw = text(objectOf(row).first_txn_at);
  return raw ? raw.slice(0, 10) : "";
}

function rankCoverageAllowsCandidate(rankResult) {
  const envelope = objectOf(rankResult);
  const data = objectOf(envelope.data);
  if (text(data.coverage_status) !== "complete") return false;
  const input = objectOf(envelope.input);
  const filters = objectOf(data.filters);
  const inputHasDedupe = Object.prototype.hasOwnProperty.call(input, "dedupe_same_holder_same_fact");
  const filtersHaveDedupe = Object.prototype.hasOwnProperty.call(filters, "dedupe_same_holder_same_fact");
  const inputDedupe = input.dedupe_same_holder_same_fact;
  const filtersDedupe = filters.dedupe_same_holder_same_fact;
  if ((inputHasDedupe && typeof inputDedupe !== "boolean") || (filtersHaveDedupe && typeof filtersDedupe !== "boolean")) {
    return false;
  }
  if (inputHasDedupe && filtersHaveDedupe && inputDedupe !== filtersDedupe) return false;
  const dedupeRequested = inputDedupe === true || filtersDedupe === true;
  if (!dedupeRequested) return true;
  const readiness = objectOf(data.same_fact_dedupe);
  return readiness.contract === "SameFactDedupeCoverageV1"
    && readiness.key_version === "analytix.same-fact-dedupe-key/v2"
    && readiness.requested === true
    && readiness.scope_authorized === true
    && readiness.applied === true
    && readiness.coverage_status === "complete"
    && (readiness.blocker == null || readiness.blocker === "");
}

export function counterpartySummaryFromRank(rankResult, expectedName, { holderName = "", dateStart = "", dateEnd = "", scopeLabel = "" } = {}) {
  if (!rankCoverageAllowsCandidate(rankResult)) return {};
  const rows = arrayOf(objectOf(rankResult).data?.rankings);
  const row = objectOf(rows.find((item) => isNamedCounterparty(item, expectedName)));
  if (!Object.keys(row).length) return {};
  const cluster = objectOf(row.largest_date_cluster);
  const txnCount = countOrUndefined(row.txn_count);
  const amount = numberOrUndefined(objectOf(row.outflow).yuan ?? row.outflow_total);
  if (txnCount === undefined || amount === undefined) {
    return {};
  }
  return pruneEmpty({
    from_holder: holderName,
    to_holder: expectedName,
    scope_label: scopeLabel,
    date_start: dateStart,
    date_end: dateEnd,
    txn_count: txnCount,
    amount,
    source_accounts: uniqueTexts(row.source_accounts, 8),
    source_account_count: countOrUndefined(row.source_account_count ?? row.account_count),
    source_accounts_truncated: booleanOrUndefined(row.source_accounts_truncated),
    duplicate_source_accounts: uniqueTexts(row.duplicate_source_accounts, 8),
    duplicate_row_count: countOrUndefined(row.duplicate_row_count),
    duplicate_amount: numberOrUndefined(row.duplicate_amount),
    first_txn_at: text(row.first_txn_at),
    last_txn_at: text(row.last_txn_at),
    largest_date_cluster: pruneEmpty({
      date: text(cluster.date),
      txn_count: countOrUndefined(cluster.txn_count),
      amount: numberOrUndefined(cluster.amount)
    }) || undefined,
    dedupe_basis: "同一主体相关账户内按有效 txn_id+时间+方向+金额+余额+对手折叠；按对手户名聚合。",
    boundary: "这是主体到续查对象的确定性对手方聚合范围；最大集中日期用于选择续查窗口，最终穿透仍需逐笔余额承接。"
  }) || {};
}

export function counterpartyAccountSummaryFromRank(rankResult, expectedName, { holderName = "", dateStart = "", dateEnd = "", scopeLabel = "" } = {}) {
  if (!rankCoverageAllowsCandidate(rankResult)) return {};
  const rows = arrayOf(objectOf(rankResult).data?.rankings);
  const row = objectOf(rows.find((item) => isNamedCounterparty(item, expectedName)));
  if (!Object.keys(row).length) return {};
  const cluster = objectOf(row.largest_date_cluster);
  const account = text(row.counterparty_account || row.account_key || row.account_no || row.card_no);
  const txnCount = countOrUndefined(row.txn_count);
  const amount = numberOrUndefined(objectOf(row.outflow).yuan ?? row.outflow_total ?? row.outflow_yuan);
  if (txnCount === undefined || amount === undefined) {
    return {};
  }
  return pruneEmpty({
    from_holder: holderName,
    to_holder: expectedName,
    scope_label: scopeLabel,
    date_start: dateStart,
    date_end: dateEnd,
    txn_count: txnCount,
    amount,
    source_accounts: uniqueTexts(row.source_accounts, 8),
    source_account_count: countOrUndefined(row.source_account_count ?? row.account_count),
    source_accounts_truncated: booleanOrUndefined(row.source_accounts_truncated),
    duplicate_source_accounts: uniqueTexts(row.duplicate_source_accounts, 8),
    duplicate_row_count: countOrUndefined(row.duplicate_row_count),
    duplicate_amount: numberOrUndefined(row.duplicate_amount),
    counterparty_accounts: account ? [account] : [],
    largest_date_cluster: pruneEmpty({
      date: text(cluster.date),
      txn_count: countOrUndefined(cluster.txn_count),
      amount: numberOrUndefined(cluster.amount)
    }) || undefined,
    dedupe_basis: "同一主体相关账户内按有效 txn_id+时间+方向+金额+余额+对手折叠，再按收款账号聚合。",
    boundary: "这是重点收款账户聚合支持，不等于后续资金已形成闭环；下游仍需逐笔或一跳事实支持。"
  }) || {};
}

export function duplicateCandidateRiskText(summary) {
  const item = objectOf(summary);
  const groupCount = countOrUndefined(item.same_holder_group_count);
  const extraRows = countOrUndefined(item.same_holder_extra_rows);
  const candidateAmount = numberOrUndefined(numericValue(item.same_holder_candidate_duplicate_amount));
  if (groupCount === undefined && extraRows === undefined && candidateAmount === undefined) {
    return "当前结果未返回同事实候选组数、额外行数或候选差额；不得补成 0，也不能据此排除重复反馈风险。";
  }
  if (groupCount || extraRows || candidateAmount) {
    return `同一主体相关账户内同事实候选 ${groupCount ?? "未返回"} 组、额外 ${extraRows ?? "未返回"} 行，候选差额 ${candidateAmount === undefined ? "未返回" : moneyText(candidateAmount)}；候选差额仅提示换卡/补卡/银行重复反馈风险，未经回单或账户映射确认不得从统计总额中扣减。`;
  }
  return "当前结果显式返回同事实候选计数为 0；该值仍需同案、同轮、同快照回执和完整查询范围核验，不能仅凭 0 排除重复反馈风险。";
}
