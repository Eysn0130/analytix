import {
  compactAccountStatsFact,
  compactCoverageFact,
  compactFlowGraphForText,
  compactHypothesisCards,
  compactOwnerScope,
  compactQaReviewFacts,
  compactRankingRows,
  compactTopOutflowRows,
  compactWarningList,
  sameFactEligibilityComplete
} from "./agent-payload-compiler.mjs";
import { withAnswerCardProtocol } from "./answer-card-protocol.mjs";
import {
  compactFinancialProductLeads
} from "./diagnostic-fact-helpers.mjs";
import {
  buildFrontdoorAnswerContract,
  buildFrontdoorNextActions
} from "./frontdoor-answer-contract.mjs";
import {
  counterpartyAccountSummaryFromRank,
  counterpartySummaryFromRank,
  duplicateCandidateRiskText,
  firstTxnDateForCounterparty,
  moneyText
} from "./frontdoor-fact-summaries.mjs";
import {
  directionModeForMetric,
  inferDateStartFromQuestion,
  inferRankingMetric,
  inferRankingMetrics,
  inferRankingTarget,
  inferSourceHolderFromQuestion,
  inferViaNameFromQuestion,
  isExplicitTransferAmountQuestion,
  isTopOutflowClassificationQuestion,
  isViaContinuationQuestion,
  rankingSkillForTarget
} from "./frontdoor-routing.mjs";
import {
  buildIntentAst,
  buildPlanDag,
  frontdoorWantsFlowGraph,
  inferFrontDoorIntent
} from "./intent-plan-protocol.mjs";
import { executeLocalDuckdbRunCaseSql } from "./duckdb-workbench-runtime.mjs";
import { stableHash } from "./stable-hash.mjs";
import { reportPublicationBlockedPayload } from "./report-publication-guard.mjs";
import { throwIfAborted } from "./abort-runtime.mjs";
import {
  arrayOf,
  clampInt,
  intOrUndefined,
  numberOrUndefined,
  objectOf,
  pruneEmpty,
  text
} from "./runtime-normalizers.mjs";

export const FRONTDOOR_RUNTIME_VERSION = "0.15.127";

const INTERNAL_RUNTIME_CONTEXT_KEYS = ["_analytix", "__analytix", "analytix_runtime_context"];

function hasOwn(value, key) {
  return value && typeof value === "object" && Object.prototype.hasOwnProperty.call(value, key);
}

function withInternalRuntimeContext(source, target = {}) {
  const next = { ...objectOf(target) };
  const contextSource = objectOf(source);
  for (const key of INTERNAL_RUNTIME_CONTEXT_KEYS) {
    if (hasOwn(contextSource, key)) {
      next[key] = contextSource[key];
    }
  }
  return next;
}

function optionalNumberValue(value) {
  if (value && typeof value === "object") {
    const source = objectOf(value);
    return optionalNumberValue(source.yuan ?? source.amount_yuan ?? source.amount ?? source.value);
  }
  return numberOrUndefined(value);
}

function optionalCountValue(value) {
  const next = intOrUndefined(value);
  return next !== undefined && next >= 0 ? next : undefined;
}

function explicitArray(source, key) {
  return hasOwn(source, key) && Array.isArray(source[key]) ? source[key] : undefined;
}

function requestedAmountYuanFromQuestion(question) {
  const q = text(question).replace(/[,，]/gu, "");
  const match = q.match(/(\d+(?:\.\d+)?)\s*(亿元|亿|万元|万|元)/u);
  if (!match) return undefined;
  const value = numberOrUndefined(match[1]);
  if (value === undefined) return undefined;
  const unit = text(match[2]);
  if (unit === "亿元" || unit === "亿") return value * 100000000;
  if (unit === "万元" || unit === "万") return value * 10000;
  return value;
}

function amountApproximatelyMatches(left, right) {
  const a = optionalNumberValue(left);
  const b = optionalNumberValue(right);
  if (a === undefined || b === undefined || a <= 0 || b <= 0) return false;
  return Math.abs(a - b) <= Math.max(1, Math.abs(b) * 0.001);
}

function roundMoney(value) {
  const amount = optionalNumberValue(value);
  if (amount === undefined) return undefined;
  return Math.round(amount * 100) / 100;
}

function nonNegativeDifference(left, right) {
  const a = optionalNumberValue(left);
  const b = optionalNumberValue(right);
  return a === undefined || b === undefined ? undefined : Math.max(0, roundMoney(a - b));
}

function sumKnown(left, right) {
  const a = optionalNumberValue(left);
  const b = optionalNumberValue(right);
  return a === undefined || b === undefined ? undefined : roundMoney(a + b);
}

function workbenchPayloadData(result) {
  const source = objectOf(result?.data || result);
  return arrayOf(source.records).length || text(source.execution_status)
    ? source
    : objectOf(source.data);
}

function firstRecord(result) {
  return objectOf(arrayOf(workbenchPayloadData(result).records)[0]);
}

function pairAmountWorkbenchExecuted(result) {
  const data = workbenchPayloadData(result);
  return result?.ok === true && data.execution_status === "executed" && Object.keys(firstRecord(data)).length > 0;
}

function pairAmountWorkbenchNeedsLocalFallback(result) {
  if (pairAmountWorkbenchExecuted(result)) return false;
  const diagnostic = JSON.stringify({
    data: result?.data,
    envelope: result?.envelope,
    warnings: result?.warnings
  });
  return /Structured SQL parser is unavailable|controlled case SQL fails closed|Structured .*parser.*fails closed|unbounded SELECT \* is not allowed/iu.test(diagnostic);
}

async function runLocalPairAmountWorkbench({ caseId, holderName, viaName, env, signal }) {
  throwIfAborted(signal);
  const input = {
    case_id: caseId,
    purpose: [
      "当前案件两方往来金额定向核验。",
      "后端 SQL parser 暂不可用时，使用插件本地只读 DuckDB 现场能力执行同一条受控聚合。",
      "仅使用清洗/分析范围，组合安全事实键，不能把占位交易号单独当作唯一事实键。"
    ].join(" "),
    sql: pairAmountSql({ holderName, viaName }),
    row_limit: 5,
    result_mode: "aggregate",
    allowed_view_policy: "cleaned_and_analysis_only"
  };
  try {
    const envelope = await executeLocalDuckdbRunCaseSql(input, { env, signal });
    throwIfAborted(signal);
    return {
      ok: true,
      skill_id: "run_case_sql",
      input,
      envelope,
      data: objectOf(envelope.data),
      warnings: arrayOf(envelope.warnings),
      evidence_refs: objectOf(envelope.citations)
    };
  } catch (error) {
    throwIfAborted(signal);
    return {
      ok: false,
      skill_id: "run_case_sql",
      input,
      envelope: {},
      data: {},
      warnings: [{
        code: "LOCAL_DUCKDB_WORKBENCH_FALLBACK_FAILED",
        severity: "warning",
        message: text(error?.message || error)
      }],
      evidence_refs: {}
    };
  }
}

function sqlStringLiteral(value) {
  return `'${text(value).replace(/'/gu, "''")}'`;
}

export function pairAmountSql({ holderName, viaName }) {
  const holder = sqlStringLiteral(holderName);
  const via = sqlStringLiteral(viaName);
  return `WITH pair_rows AS (
  SELECT
    id,
    acct_key,
    card_no,
    acct_no,
    account_open_name,
    opener_id_no,
    txn_id,
    txn_ts,
    txn_time,
    dc_val,
    amount,
    balance,
    cp_key,
    cp_raw,
    cp_name,
    cp_name_pick,
    stats_name_key,
    counterparty_acct,
    counterparty_name,
    counterparty_id_no,
    summary,
    remark,
    txn_type,
    is_success
  FROM analysis_txn_detail_idx
  WHERE account_open_name = ${holder}
    AND dc_val = '出'
    AND amount IS NOT NULL
    AND (
      cp_name = ${via}
      OR cp_name_pick = ${via}
      OR stats_name_key = ${via}
      OR counterparty_name = ${via}
    )
), base_keyed AS (
  SELECT
    id,
    amount,
    txn_ts,
    txn_time,
    counterparty_acct,
    coalesce(nullif(trim(cast(card_no AS varchar)), ''), nullif(trim(cast(acct_no AS varchar)), ''), nullif(trim(cast(acct_key AS varchar)), ''), '') AS account_key_norm,
    CASE
      WHEN txn_id IS NOT NULL
        AND length(trim(cast(txn_id AS varchar))) > 0
        AND lower(trim(cast(txn_id AS varchar))) NOT IN ('查无信息','无','未知','unknown','__unknown__','null','none','na','n/a','-','--')
      THEN trim(cast(txn_id AS varchar))
      ELSE ''
    END AS valid_txn_id,
    account_open_name,
    opener_id_no,
    dc_val,
    balance,
    cp_name,
    cp_name_pick,
    stats_name_key,
    counterparty_name,
    counterparty_id_no,
    summary,
    remark,
    txn_type,
    is_success
  FROM pair_rows
), natural_keyed AS (
  SELECT
    id,
    amount,
    txn_ts,
    txn_time,
    counterparty_acct,
    account_key_norm,
    valid_txn_id,
    account_open_name,
    opener_id_no,
    dc_val,
    balance,
    cp_name,
    cp_name_pick,
    stats_name_key,
    counterparty_name,
    counterparty_id_no,
    summary,
    remark,
    txn_type,
    is_success,
    concat_ws('|',
      'same_holder_cross_account_fact',
      coalesce(account_open_name, ''),
      coalesce(opener_id_no, ''),
      coalesce(dc_val, ''),
      coalesce(cast(txn_ts AS varchar), cast(txn_time AS varchar), ''),
      cast(round(abs(amount), 2) AS varchar),
      coalesce(cast(round(balance, 2) AS varchar), ''),
      coalesce(counterparty_acct, ''),
      coalesce(cp_name, cp_name_pick, stats_name_key, counterparty_name, ''),
      coalesce(counterparty_id_no, ''),
      valid_txn_id,
      coalesce(summary, ''),
      coalesce(remark, ''),
      coalesce(txn_type, ''),
      coalesce(cast(is_success AS varchar), '')
    ) AS same_holder_cross_account_fact_key
  FROM base_keyed
), confirmed_same_fact_keys AS (
  SELECT same_holder_cross_account_fact_key
  FROM natural_keyed
  WHERE length(trim(same_holder_cross_account_fact_key)) > 0
    AND length(trim(account_key_norm)) > 0
  GROUP BY same_holder_cross_account_fact_key
  HAVING count(*) > 1 AND count(DISTINCT account_key_norm) > 1
), keyed AS (
  SELECT
    n.id,
    n.amount,
    n.txn_ts,
    n.txn_time,
    n.counterparty_acct,
    n.account_key_norm,
    CASE
      WHEN c.same_holder_cross_account_fact_key IS NOT NULL
      THEN concat('confirmed_cross_account|', n.same_holder_cross_account_fact_key)
      WHEN length(n.valid_txn_id) > 0
      THEN concat_ws('|', 'txn', n.valid_txn_id, coalesce(cast(n.txn_ts AS varchar), cast(n.txn_time AS varchar), ''), cast(round(abs(n.amount), 2) AS varchar), coalesce(cast(round(n.balance, 2) AS varchar), ''), coalesce(n.counterparty_acct, ''), coalesce(n.cp_name, n.cp_name_pick, n.stats_name_key, n.counterparty_name, ''), coalesce(n.summary, ''), coalesce(n.remark, ''), coalesce(n.txn_type, ''), coalesce(cast(n.is_success AS varchar), ''))
      ELSE concat_ws('|', 'row', cast(n.id AS varchar), coalesce(cast(n.txn_ts AS varchar), cast(n.txn_time AS varchar), ''), cast(round(abs(n.amount), 2) AS varchar), coalesce(n.counterparty_acct, ''), coalesce(n.cp_name, n.cp_name_pick, n.stats_name_key, n.counterparty_name, ''))
    END AS safe_fact_key,
    CASE
      WHEN c.same_holder_cross_account_fact_key IS NOT NULL THEN 'confirmed_same_holder_cross_account_same_fact'
      WHEN length(n.valid_txn_id) > 0 THEN 'valid_transaction_id_fact_key'
      ELSE 'row_identity_when_joint_fact_key_incomplete'
    END AS dedupe_basis
  FROM natural_keyed n
  LEFT JOIN confirmed_same_fact_keys c
    ON n.same_holder_cross_account_fact_key = c.same_holder_cross_account_fact_key
), deduped AS (
  SELECT safe_fact_key, min(id) AS representative_id, count(*) AS row_count, max(amount) AS amount, min(txn_ts) AS first_ts, max(txn_ts) AS last_ts
  FROM keyed
  GROUP BY safe_fact_key
), date_clusters AS (
  SELECT substr(cast(txn_ts AS varchar), 1, 10) AS txn_date, count(*) AS raw_count, round(sum(amount), 2) AS raw_amount
  FROM pair_rows
  GROUP BY 1
), account_clusters AS (
  SELECT coalesce(counterparty_acct, '') AS counterparty_acct, count(*) AS raw_count, round(sum(amount), 2) AS raw_amount
  FROM pair_rows
  GROUP BY 1
), source_account_clusters AS (
  SELECT coalesce(card_no, acct_no, acct_key, '') AS payer_account, count(*) AS raw_count, round(sum(amount), 2) AS raw_amount
  FROM pair_rows
  GROUP BY 1
), duplicate_groups AS (
  SELECT
    safe_fact_key,
    min(dedupe_basis) AS dedupe_basis,
    count(DISTINCT account_key_norm) AS account_count,
    list(DISTINCT account_key_norm ORDER BY account_key_norm) AS payer_accounts,
    count(*) AS raw_count,
    round(sum(amount), 2) AS raw_amount,
    round(max(amount), 2) AS effective_amount,
    round(sum(amount) - max(amount), 2) AS duplicate_amount
  FROM keyed
  GROUP BY safe_fact_key
  HAVING count(*) > 1
), date_counterparty_keyed AS (
  SELECT
    substr(cast(txn_ts AS varchar), 1, 10) AS txn_date,
    coalesce(counterparty_acct, '') AS counterparty_acct,
    safe_fact_key,
    count(*) AS raw_count,
    round(sum(amount), 2) AS raw_amount,
    round(max(amount), 2) AS effective_amount,
    min(txn_ts) AS first_txn_at,
    max(txn_ts) AS last_txn_at
  FROM keyed
  GROUP BY 1, 2, 3
), date_counterparty_clusters AS (
  SELECT
    txn_date,
    counterparty_acct,
    sum(raw_count) AS raw_count,
    round(sum(raw_amount), 2) AS raw_amount,
    count(*) AS effective_count,
    round(sum(effective_amount), 2) AS effective_amount,
    min(first_txn_at) AS first_txn_at,
    max(last_txn_at) AS last_txn_at
  FROM date_counterparty_keyed
  GROUP BY 1, 2
), top_transaction_rows AS (
  SELECT
    id,
    acct_key,
    card_no,
    acct_no,
    account_open_name,
    txn_ts,
    txn_time,
    amount,
    counterparty_acct,
    cp_name,
    cp_name_pick,
    stats_name_key,
    counterparty_name,
    summary,
    remark,
    txn_type
  FROM pair_rows
  ORDER BY amount DESC, cast(txn_ts AS varchar), id
  LIMIT 10
), effective_top_transaction_rows AS (
  SELECT
    p.id,
    p.acct_key,
    p.card_no,
    p.acct_no,
    p.account_open_name,
    p.txn_ts,
    p.txn_time,
    p.amount,
    p.counterparty_acct,
    p.cp_name,
    p.cp_name_pick,
    p.stats_name_key,
    p.counterparty_name,
    p.summary,
    p.remark,
    p.txn_type
  FROM pair_rows p
  INNER JOIN deduped d ON p.id = d.representative_id
  ORDER BY p.amount DESC, cast(p.txn_ts AS varchar), p.id
  LIMIT 20
), top_transactions AS (
  SELECT list({
    txn_time: coalesce(cast(txn_ts AS varchar), cast(txn_time AS varchar), ''),
    payer_account: coalesce(card_no, acct_no, acct_key, ''),
    payer_name: coalesce(account_open_name, ''),
    receiver_account: coalesce(counterparty_acct, ''),
    receiver_name: coalesce(cp_name, cp_name_pick, stats_name_key, counterparty_name, ''),
    amount: round(amount, 2),
    summary: coalesce(summary, ''),
    remark: coalesce(remark, ''),
    txn_type: coalesce(txn_type, ''),
    evidence_status: 'raw_detail_support'
  } ORDER BY amount DESC) AS rows
  FROM top_transaction_rows
), effective_top_transactions AS (
  SELECT list({
    txn_time: coalesce(cast(txn_ts AS varchar), cast(txn_time AS varchar), ''),
    payer_account: coalesce(card_no, acct_no, acct_key, ''),
    payer_name: coalesce(account_open_name, ''),
    receiver_account: coalesce(counterparty_acct, ''),
    receiver_name: coalesce(cp_name, cp_name_pick, stats_name_key, counterparty_name, ''),
    amount: round(amount, 2),
    summary: coalesce(summary, ''),
    remark: coalesce(remark, ''),
    txn_type: coalesce(txn_type, ''),
    evidence_status: 'effective_fact_support'
  } ORDER BY amount DESC) AS rows
  FROM effective_top_transaction_rows
), small_amount_candidates AS (
  SELECT count(*) AS small_count, round(coalesce(sum(amount), 0), 2) AS small_amount
  FROM deduped
  WHERE amount > 0 AND amount <= 100
)
SELECT
  (SELECT count(*) FROM pair_rows) AS raw_detail_count,
  (SELECT round(coalesce(sum(amount), 0), 2) FROM pair_rows) AS raw_detail_amount,
  (SELECT count(*) FROM deduped) AS effective_fact_count,
  (SELECT round(coalesce(sum(amount), 0), 2) FROM deduped) AS effective_dedup_amount,
  (SELECT cast(min(txn_ts) AS varchar) FROM pair_rows) AS first_txn_at,
  (SELECT cast(max(txn_ts) AS varchar) FROM pair_rows) AS last_txn_at,
  (SELECT cast(min(first_ts) AS varchar) FROM deduped) AS effective_first_txn_at,
  (SELECT cast(max(last_ts) AS varchar) FROM deduped) AS effective_last_txn_at,
  (SELECT round(coalesce(sum(amount), 0), 2) FROM pair_rows) - (SELECT round(coalesce(sum(amount), 0), 2) FROM deduped) AS duplicate_or_unsupported_amount,
  (SELECT round(coalesce(sum(duplicate_amount), 0), 2) FROM duplicate_groups WHERE dedupe_basis = 'confirmed_same_holder_cross_account_same_fact') AS confirmed_same_fact_duplicate_amount,
  (SELECT coalesce(sum(raw_count - 1), 0) FROM duplicate_groups WHERE dedupe_basis = 'confirmed_same_holder_cross_account_same_fact') AS confirmed_same_fact_duplicate_extra_count,
  (SELECT small_count FROM small_amount_candidates) AS small_amount_candidate_count,
  (SELECT small_amount FROM small_amount_candidates) AS small_amount_candidate_amount,
  (SELECT round(coalesce(sum(amount), 0), 2) FROM deduped) - (SELECT small_amount FROM small_amount_candidates) AS principal_after_small_amount_exclusion,
  (SELECT list({date: txn_date, raw_count: raw_count, raw_amount: raw_amount} ORDER BY raw_amount DESC) FROM date_clusters) AS date_clusters,
  (SELECT list({counterparty_acct: counterparty_acct, raw_count: raw_count, raw_amount: raw_amount} ORDER BY raw_amount DESC) FROM account_clusters) AS counterparty_account_clusters,
  (SELECT list({payer_account: payer_account, raw_count: raw_count, raw_amount: raw_amount} ORDER BY raw_amount DESC) FROM source_account_clusters) AS source_account_clusters,
  (SELECT list({date: txn_date, counterparty_acct: counterparty_acct, raw_count: raw_count, raw_amount: raw_amount, effective_count: effective_count, effective_amount: effective_amount, first_txn_at: cast(first_txn_at AS varchar), last_txn_at: cast(last_txn_at AS varchar)} ORDER BY effective_amount DESC, txn_date DESC) FROM date_counterparty_clusters) AS date_counterparty_clusters,
  (SELECT list({safe_fact_key: safe_fact_key, dedupe_basis: dedupe_basis, account_count: account_count, payer_accounts: payer_accounts, raw_count: raw_count, raw_amount: raw_amount, effective_amount: effective_amount, duplicate_amount: duplicate_amount} ORDER BY raw_amount DESC) FROM duplicate_groups) AS duplicate_groups,
  (SELECT rows FROM top_transactions) AS top_transactions,
  (SELECT rows FROM effective_top_transactions) AS effective_top_transactions
FROM pair_rows
LIMIT 1`;
}

const NAME_QUICK_FACT_STOPWORDS = new Set([
  "案件",
  "资金",
  "流水",
  "交易",
  "关联",
  "有关",
  "关系",
  "是否",
  "有无",
  "有没有",
  "严格",
  "模糊",
  "排除",
  "对象",
  "不能",
  "确认",
  "输出",
  "给出"
]);

function uniqueStrings(values, limit = 12) {
  const seen = new Set();
  const output = [];
  for (const value of arrayOf(values).map(text).filter(Boolean)) {
    if (seen.has(value)) continue;
    seen.add(value);
    output.push(value);
    if (output.length >= limit) break;
  }
  return output;
}

function extractAccountKeysFromQuestion(question) {
  return uniqueStrings([...text(question).matchAll(/\b[A-Za-z0-9][A-Za-z0-9_-]{7,35}\b/gu)]
    .map((match) => match[0])
    .filter((item) => /\d/u.test(item))
    .filter((item) => !/^20\d{2}/u.test(item)), 6);
}

function extractLikelyNamesFromQuestion(question, focusKeywords = []) {
  const names = [];
  for (const keyword of arrayOf(focusKeywords)) {
    const value = text(keyword);
    if (/^[\u4e00-\u9fa5]{2,4}$/u.test(value) && !NAME_QUICK_FACT_STOPWORDS.has(value)) {
      names.push(value);
    }
  }
  const q = text(question);
  const prefix = q.split(/是否|有无|有没有|与案件|和案件|跟案件|同案件/u)[0] || q;
  for (const piece of prefix.split(/[、,，;；\s]|和|及|与|跟/u)) {
    const value = text(piece).replace(/[？?。！!：:]/gu, "");
    if (/^[\u4e00-\u9fa5]{2,4}$/u.test(value) && !NAME_QUICK_FACT_STOPWORDS.has(value)) {
      names.push(value);
    }
  }
  if (!names.length) {
    for (const match of q.matchAll(/[\u4e00-\u9fa5]{2,4}/gu)) {
      const value = text(match[0]);
      if (!NAME_QUICK_FACT_STOPWORDS.has(value)) names.push(value);
    }
  }
  return uniqueStrings(names, 6);
}

function inferHolderNameForTopic(question, current = "") {
  const holder = text(current);
  if (holder) return holder;
  const q = text(question);
  const normalized = q.replace(/^(?:请|帮我|帮忙|麻烦|协助|核算|查询|统计|梳理|分析|研判|复核|查看|看看|说明|给出)+/u, "");
  const match = normalized.match(/^([\u4e00-\u9fa5]{2,4})(?=(?:购买|买入|买|基金|证券|股票|三方存管|申购|赎回|的|在|有|相关|情况))/u)
    || q.match(/(?:核算|查询|统计|梳理|分析|研判|复核|查看|说明)?\s*([\u4e00-\u9fa5]{2,4})(?=(?:购买|买入|买|基金|证券|股票|三方存管|申购|赎回))/u);
  const inferred = text(match?.[1])
    .replace(/[购买彩票买申赎认]+$/u, "")
    .replace(/(?:购买|买入|基金|证券|股票|三方存管|申购|赎回|情况|专题)+$/u, "");
  if (/^[\u4e00-\u9fa5]{2,4}$/u.test(inferred) && !NAME_QUICK_FACT_STOPWORDS.has(inferred)) return inferred;
  return "";
}

function inferHolderNameForDossier(question, current = "") {
  const holder = text(current);
  if (holder) return holder;
  const q = text(question);
  const normalized = q.replace(/^(?:请|帮我|帮忙|麻烦|协助|核查|查询|统计|梳理|分析|研判|复核|查看|看看|说明|给出)+/u, "");
  const match = normalized.match(/^(?:对|围绕)?\s*([\u4e00-\u9fa5]{2,4})(?=(?:名下|账户|户名|资金画像|资金研判|进行研判|开展研判|的账户|相关账户))/u)
    || q.match(/(?:对|围绕|核查|研判|分析|查询|查看|梳理)\s*([\u4e00-\u9fa5]{2,4})(?=(?:名下|账户|户名|资金画像|资金研判|进行研判|开展研判|的账户|相关账户))/u);
  const inferred = text(match?.[1]).replace(/(?:名下|账户|户名|资金画像|资金研判|研判)+$/u, "");
  if (/^[\u4e00-\u9fa5]{2,4}$/u.test(inferred) && !NAME_QUICK_FACT_STOPWORDS.has(inferred)) return inferred;
  return "";
}

function targetRowsSql(names = [], tableName = "analysis_txn_detail_idx") {
  return uniqueStrings(names, 12)
    .map((name, index) => `SELECT DISTINCT ${sqlStringLiteral(name)} AS target, ${index + 1} AS target_order FROM ${tableName}`)
    .join("\n  UNION ALL ");
}

function nameAssociationQuickFactSql(names = []) {
  const targets = targetRowsSql(names, "analysis_txn_detail_idx") || "SELECT DISTINCT '' AS target, 1 AS target_order FROM analysis_txn_detail_idx";
  return `WITH target_rows AS (
  ${targets}
), name_rows AS (
  SELECT id, amount, account_open_name AS name FROM analysis_txn_detail_idx WHERE account_open_name IS NOT NULL AND account_open_name <> ''
  UNION ALL SELECT id, amount, cp_name AS name FROM analysis_txn_detail_idx WHERE cp_name IS NOT NULL AND cp_name <> ''
  UNION ALL SELECT id, amount, cp_name_pick AS name FROM analysis_txn_detail_idx WHERE cp_name_pick IS NOT NULL AND cp_name_pick <> ''
  UNION ALL SELECT id, amount, stats_name_key AS name FROM analysis_txn_detail_idx WHERE stats_name_key IS NOT NULL AND stats_name_key <> ''
  UNION ALL SELECT id, amount, counterparty_name AS name FROM analysis_txn_detail_idx WHERE counterparty_name IS NOT NULL AND counterparty_name <> ''
), exact_rows AS (
  SELECT DISTINCT t.target, t.target_order, n.id, n.amount
  FROM target_rows t JOIN name_rows n ON n.name = t.target
), fuzzy_distinct AS (
  SELECT DISTINCT t.target, n.name AS matched_name, n.id, n.amount
  FROM target_rows t JOIN name_rows n ON n.name <> t.target
  WHERE length(t.target) >= 3 AND length(n.name) = length(t.target)
    AND (
      (substr(n.name, 1, 1) = substr(t.target, 1, 1) AND substr(n.name, 2, 1) = substr(t.target, 2, 1))
      OR (substr(n.name, 1, 1) = substr(t.target, 1, 1) AND substr(n.name, 3, 1) = substr(t.target, 3, 1))
      OR (substr(n.name, 2, 1) = substr(t.target, 2, 1) AND substr(n.name, 3, 1) = substr(t.target, 3, 1))
    )
), fuzzy_rows AS (
  SELECT target, matched_name, count(*) AS txn_count, round(sum(amount), 2) AS amount_sample
  FROM fuzzy_distinct
  GROUP BY target, matched_name
), strict AS (
  SELECT t.target, t.target_order, count(e.id) AS strict_txn_count, round(coalesce(sum(e.amount), 0), 2) AS strict_amount
  FROM target_rows t LEFT JOIN exact_rows e ON e.target = t.target
  GROUP BY t.target, t.target_order
)
SELECT
  s.target,
  s.strict_txn_count,
  s.strict_amount,
  coalesce((SELECT list({matched_name: matched_name, txn_count: txn_count, amount_sample: amount_sample} ORDER BY txn_count DESC, matched_name) FROM fuzzy_rows f WHERE f.target = s.target), []) AS fuzzy_matches
FROM strict s
ORDER BY s.target_order`;
}

function financialProductTopicSql(holderName) {
  const holder = sqlStringLiteral(holderName);
  return `WITH subject_ids AS (
  SELECT DISTINCT id_no
  FROM analysis_account_dim
  WHERE open_name = ${holder} AND coalesce(id_no, '') <> ''
), subject_accounts AS (
  SELECT DISTINCT account_key, open_name, id_no, acct_type, bank_name
  FROM analysis_account_dim
  WHERE open_name = ${holder}
     OR (coalesce(id_no, '') <> '' AND id_no IN (SELECT id_no FROM subject_ids))
), product_rows AS (
  SELECT
    t.id,
    t.acct_key,
    t.dc_val,
    t.amount,
    concat_ws(' ', coalesce(t.cp_name, ''), coalesce(t.cp_name_pick, ''), coalesce(t.stats_name_key, ''), coalesce(t.counterparty_name, ''), coalesce(t.counterparty_bank, ''), coalesce(t.summary, ''), coalesce(t.remark, ''), coalesce(t.txn_type, ''), coalesce(t.merchant_name, '')) AS searchable
  FROM analysis_txn_detail_idx t
  JOIN subject_accounts a ON t.acct_key = a.account_key
), bucketed AS (
  SELECT
    CASE
      WHEN searchable LIKE '%证券%' OR searchable LIKE '%三方存管%' THEN '证券三方存管'
      WHEN searchable LIKE '%基金%' THEN '基金申赎'
      WHEN searchable LIKE '%申购%' OR searchable LIKE '%赎回%' THEN '理财/其他产品申赎线索'
      ELSE '其他'
    END AS bucket,
    dc_val,
    count(*) AS txn_count,
    round(sum(amount), 2) AS amount
  FROM product_rows
  WHERE searchable LIKE '%证券%' OR searchable LIKE '%三方存管%' OR searchable LIKE '%基金%' OR searchable LIKE '%申购%' OR searchable LIKE '%赎回%'
  GROUP BY bucket, dc_val
)
SELECT
  (SELECT count(*) FROM subject_accounts) AS subject_account_count,
  (SELECT list({account_key: account_key, open_name: open_name, id_no: id_no, acct_type: acct_type, bank_name: bank_name} ORDER BY account_key) FROM subject_accounts) AS subject_accounts,
  (SELECT list({bucket: bucket, direction: dc_val, txn_count: txn_count, amount: amount} ORDER BY bucket, dc_val) FROM bucketed) AS buckets
FROM subject_accounts
LIMIT 1`;
}

function companySubjectSqlExpression(columnSql) {
  return `(${columnSql} LIKE '%公司%' OR ${columnSql} LIKE '%有限%' OR ${columnSql} LIKE '%银行%' OR ${columnSql} LIKE '%法院%' OR ${columnSql} LIKE '%中心%' OR ${columnSql} LIKE '%合作社%' OR ${columnSql} LIKE '%店%' OR ${columnSql} LIKE '%厂%' OR ${columnSql} LIKE '%经营部%' OR ${columnSql} LIKE '%个体工商户%' OR ${columnSql} LIKE '%企业%' OR ${columnSql} LIKE '%单位%')`;
}

function companyToPersonRecomputeSql() {
  const cpDisplay = "coalesce(nullif(t.cp_name, ''), nullif(t.counterparty_name, ''), nullif(t.cp_name_pick, ''), nullif(t.stats_name_key, ''), '')";
  return `WITH holder_type AS (
  SELECT
    account_key,
    open_name,
    CASE WHEN ${companySubjectSqlExpression("open_name")} THEN '对公主体' ELSE '个人主体' END AS holder_type
  FROM analysis_account_dim
), company_out_person AS (
  SELECT
    t.id,
    t.amount,
    t.summary,
    t.remark,
    t.txn_type,
    h.open_name,
    ${cpDisplay} AS cp_display,
    CASE WHEN ${companySubjectSqlExpression(cpDisplay)} THEN '对公主体' ELSE '个人主体' END AS cp_type,
    concat_ws(' ', coalesce(t.summary, ''), coalesce(t.remark, ''), coalesce(t.txn_type, '')) AS purpose_text
  FROM analysis_txn_detail_idx t
  JOIN holder_type h ON t.acct_key = h.account_key
  WHERE t.dc_val = '出' AND h.holder_type = '对公主体'
), scoped AS (
  SELECT
    id,
    amount,
    open_name,
    cp_display,
    CASE WHEN coalesce(trim(summary), '') = '' THEN 1 ELSE 0 END AS summary_missing,
    CASE
      WHEN purpose_text LIKE '%薪资%' OR purpose_text LIKE '%工资%' OR purpose_text LIKE '%劳务%' OR purpose_text LIKE '%报销%' THEN '可识别薪资工资/劳务/报销'
      ELSE '无法识别性质'
    END AS nature_bucket
  FROM company_out_person
  WHERE cp_type = '个人主体'
), agg AS (
  SELECT nature_bucket, count(*) AS txn_count, round(sum(amount), 2) AS amount
  FROM scoped
  GROUP BY nature_bucket
), summary_missing AS (
  SELECT count(*) AS txn_count, round(coalesce(sum(amount), 0), 2) AS amount
  FROM scoped
  WHERE summary_missing = 1
)
SELECT
  (SELECT count(*) FROM scoped) AS total_txn_count,
  (SELECT round(coalesce(sum(amount), 0), 2) FROM scoped) AS total_amount,
  (SELECT count(DISTINCT open_name) FROM scoped) AS company_subject_count,
  (SELECT count(DISTINCT cp_display) FROM scoped) AS personal_subject_count,
  (SELECT txn_count FROM summary_missing) AS summary_missing_txn_count,
  (SELECT amount FROM summary_missing) AS summary_missing_amount,
  (SELECT list({bucket: nature_bucket, txn_count: txn_count, amount: amount} ORDER BY amount DESC) FROM agg) AS nature_buckets
FROM scoped
LIMIT 1`;
}

function pairAmountSummary(record) {
  const item = objectOf(record);
  return {
    raw_detail_count: optionalCountValue(item.raw_detail_count),
    raw_detail_amount: optionalNumberValue(item.raw_detail_amount),
    effective_fact_count: optionalCountValue(item.effective_fact_count),
    effective_dedup_amount: optionalNumberValue(item.effective_dedup_amount),
    first_txn_at: text(item.first_txn_at),
    last_txn_at: text(item.last_txn_at),
    effective_first_txn_at: text(item.effective_first_txn_at || item.first_txn_at),
    effective_last_txn_at: text(item.effective_last_txn_at || item.last_txn_at),
    duplicate_or_unsupported_amount: optionalNumberValue(item.duplicate_or_unsupported_amount),
    confirmed_same_fact_duplicate_amount: optionalNumberValue(item.confirmed_same_fact_duplicate_amount),
    confirmed_same_fact_duplicate_extra_count: optionalCountValue(item.confirmed_same_fact_duplicate_extra_count),
    small_amount_candidate_count: optionalCountValue(item.small_amount_candidate_count),
    small_amount_candidate_amount: optionalNumberValue(item.small_amount_candidate_amount),
    principal_after_small_amount_exclusion: optionalNumberValue(item.principal_after_small_amount_exclusion),
    date_clusters: explicitArray(item, "date_clusters")?.slice(0, 6),
    counterparty_account_clusters: explicitArray(item, "counterparty_account_clusters")?.slice(0, 6),
    source_account_clusters: explicitArray(item, "source_account_clusters")?.slice(0, 8),
    date_counterparty_clusters: explicitArray(item, "date_counterparty_clusters")?.slice(0, 8),
    duplicate_groups: explicitArray(item, "duplicate_groups")?.slice(0, 6),
    top_transactions: explicitArray(item, "top_transactions")?.slice(0, 20),
    effective_top_transactions: explicitArray(item, "effective_top_transactions")?.slice(0, 20)
  };
}

function pairSameFactEligibilityComplete(result, { caseId, holderName }) {
  if (result?.ok !== true) return false;
  const input = objectOf(result.input);
  const data = objectOf(result.data);
  const scope = objectOf(data.scope);
  return text(input.case_id) === text(caseId)
    && text(input.holder_name) === text(holderName)
    && text(input.scope_mode) === "same_holder_accounts"
    && text(data.coverage_status) === "complete"
    && !text(data.blocker)
    && text(scope.scope_mode) === "same_holder_accounts"
    && text(scope.holder_name) === text(holderName)
    && sameFactEligibilityComplete(data.same_fact_eligibility);
}

function dominantPairAmountFocusCluster(summary) {
  const totalEffective = optionalNumberValue(summary.effective_dedup_amount);
  if (totalEffective === undefined || totalEffective <= 0) return null;
  const clusters = arrayOf(summary.date_counterparty_clusters)
    .map((item) => objectOf(item))
    .filter((item) => optionalNumberValue(item.effective_amount) > 0
      && optionalCountValue(item.effective_count) > 0
      && text(item.date)
      && text(item.counterparty_acct))
    .sort((left, right) =>
      optionalNumberValue(right.effective_amount) - optionalNumberValue(left.effective_amount) ||
      text(right.date).localeCompare(text(left.date))
    );
  const focus = clusters[0];
  if (!focus) return null;
  const focusAmount = optionalNumberValue(focus.effective_amount);
  const share = focusAmount / totalEffective;
  const isDominant = share >= 0.7 || (focusAmount >= 1000000 && clusters.length === 1);
  if (!isDominant) return null;
  return {
    date: text(focus.date),
    counterparty_acct: text(focus.counterparty_acct),
    raw_count: optionalCountValue(focus.raw_count),
    raw_amount: optionalNumberValue(focus.raw_amount),
    effective_count: optionalCountValue(focus.effective_count),
    effective_amount: focusAmount,
    first_txn_at: text(focus.first_txn_at),
    last_txn_at: text(focus.last_txn_at),
    share_of_effective_amount: share
  };
}

function transactionMatchesFocus(row, focusCluster) {
  if (!focusCluster) return true;
  const source = objectOf(row);
  const txnTime = text(source.txn_time);
  const receiverAccount = text(source.receiver_account);
  return (!focusCluster.date || txnTime.startsWith(focusCluster.date))
    && (!focusCluster.counterparty_acct || receiverAccount === focusCluster.counterparty_acct);
}

function focusTransactions(summary, focusCluster, { effective = true } = {}) {
  const rows = arrayOf(effective ? summary.effective_top_transactions : summary.top_transactions)
    .filter((row) => transactionMatchesFocus(row, focusCluster));
  return rows.length ? rows.slice(0, 20) : arrayOf(effective ? summary.effective_top_transactions : summary.top_transactions).slice(0, 20);
}

function uniqueTransactionAccounts(rows, key) {
  return uniqueStrings(arrayOf(rows).map((row) => objectOf(row)[key]), 20);
}

function pairAmountControllingSummary(summary, sourceToViaSummary) {
  const rank = objectOf(sourceToViaSummary);
  const rankAmount = optionalNumberValue(rank.amount);
  const rankCount = optionalCountValue(rank.txn_count);
  const rankDuplicateAmount = optionalNumberValue(rank.duplicate_amount);
  const rankDuplicateRows = optionalCountValue(rank.duplicate_row_count);
  const rankCandidateAvailable = rankAmount > 0
    && rankCount > 0
    && Boolean(text(rank.first_txn_at))
    && Boolean(text(rank.last_txn_at));
  const focusCluster = dominantPairAmountFocusCluster(summary);
  const workbenchEffective = optionalNumberValue(summary.effective_dedup_amount);
  const workbenchCount = optionalCountValue(summary.effective_fact_count);
  const workbenchRawAmount = optionalNumberValue(summary.raw_detail_amount);
  const workbenchRawCount = optionalCountValue(summary.raw_detail_count);
  const hasWorkbenchControl = workbenchEffective > 0
    && workbenchCount > 0
    && workbenchRawAmount !== undefined
    && workbenchRawAmount >= workbenchEffective
    && workbenchRawCount !== undefined
    && workbenchRawCount >= workbenchCount
    && Boolean(text(summary.effective_first_txn_at || summary.first_txn_at))
    && Boolean(text(summary.effective_last_txn_at || summary.last_txn_at))
    && summary.source_account_clusters !== undefined
    && summary.counterparty_account_clusters !== undefined
    && summary.duplicate_groups !== undefined
    && summary.top_transactions !== undefined
    && summary.effective_top_transactions !== undefined;
  const hasRankControl = !hasWorkbenchControl && rankCandidateAvailable;
  const detailRankDisagrees = rankCandidateAvailable
    && hasWorkbenchControl
    && !amountApproximatelyMatches(workbenchEffective, rankAmount);
  const duplicateRowCount = hasRankControl
    ? rankDuplicateRows ?? nonNegativeDifference(workbenchRawCount, workbenchCount)
    : nonNegativeDifference(workbenchRawCount, workbenchCount);
  const effectiveAmount = hasRankControl ? rankAmount : workbenchEffective;
  const effectiveCount = hasRankControl ? rankCount : workbenchCount;
  const duplicateAmount = hasRankControl
    ? rankDuplicateAmount ?? nonNegativeDifference(workbenchRawAmount, workbenchEffective)
    : nonNegativeDifference(workbenchRawAmount, workbenchEffective);
  const rawAmount = hasRankControl
    ? rankDuplicateAmount === undefined ? undefined : sumKnown(rankAmount, rankDuplicateAmount)
    : workbenchRawAmount;
  const rawCount = hasRankControl
    ? duplicateRowCount === undefined ? undefined : sumKnown(rankCount, duplicateRowCount)
    : workbenchRawCount;
  const exposeSmallAmount = !detailRankDisagrees && optionalNumberValue(summary.small_amount_candidate_amount) > 0;
  const firstTxnAt = hasRankControl ? text(rank.first_txn_at) : text(summary.effective_first_txn_at || summary.first_txn_at);
  const lastTxnAt = hasRankControl ? text(rank.last_txn_at) : text(summary.effective_last_txn_at || summary.last_txn_at);
  const rawFirstTxnAt = text(summary.first_txn_at);
  const rawLastTxnAt = text(summary.last_txn_at);
  const detailRankDifferenceAmount = detailRankDisagrees ? roundMoney(workbenchEffective - rankAmount) : undefined;
  return {
    hasRankControl,
    hasWorkbenchControl,
    rankCandidateAvailable,
    workbenchDisagrees: detailRankDisagrees,
    detailRankDisagrees,
    focusCluster,
    effectiveAmount,
    effectiveCount,
    duplicateAmount,
    rawAmount,
    rawCount,
    duplicateRowCount,
    rankAmount,
    rankCount,
    rankDuplicateAmount,
    rankFirstTxnAt: text(rank.first_txn_at),
    rankLastTxnAt: text(rank.last_txn_at),
    detailRankDifferenceAmount,
    firstTxnAt,
    lastTxnAt,
    rawFirstTxnAt,
    rawLastTxnAt,
    fullPeriodRawAmount: workbenchRawAmount,
    fullPeriodRawCount: workbenchRawCount,
    fullPeriodEffectiveAmount: workbenchEffective,
    fullPeriodEffectiveCount: workbenchCount,
    exposeSmallAmount
  };
}

function buildPairAmountReviewCard({
  caseId,
  holderName,
  viaName,
  pairAmountWorkbench,
  pairSameFactReview,
  sourceToViaSummary,
  sourceToViaCoreAccountSummary,
  calls = []
}) {
  const workbenchData = workbenchPayloadData(pairAmountWorkbench);
  const workbenchOk = pairAmountWorkbenchExecuted(pairAmountWorkbench);
  const record = workbenchOk ? firstRecord(workbenchData) : {};
  const summary = pairAmountSummary(record);
  const controlling = pairAmountControllingSummary(summary, sourceToViaSummary);
  const sameFactReady = controlling.hasRankControl || pairSameFactEligibilityComplete(
    pairSameFactReview,
    { caseId, holderName }
  );
  const factsReady = sameFactReady
    && (controlling.hasRankControl || (workbenchOk && controlling.hasWorkbenchControl));
  if (!factsReady) {
    return withAnswerCardProtocol({
      card_type: "pair_amount_review_card",
      case_id: caseId,
      intent: "pair_amount_review",
      title: "特定双方资金往来核验边界",
      holder_name: holderName,
      via_holder_name: viaName,
      validation_state: "required_numeric_fields_missing",
      delivery_state: "blocked",
      facts: [],
      missing_source_boundaries: [
        sameFactReady
          ? "当前结果未同时返回可验证的金额与笔数；缺失值、空结果和部分字段不得补成 0、无往来或完整两方结论。"
          : "当前范围缺少同案、同主体且完整的 SameFactEligibilityV1 资格覆盖；重复差额、确认重复金额和有效去重金额均保持未解析。"
      ],
      warnings: diagnosticWarnings([pairAmountWorkbench, pairSameFactReview])
    }, {
      recommendedNextAction: "repair_workbench_source_then_retry",
      maxAdditionalTools: 1,
      requiredFactsPresent: false,
      unsupportedFlowsPresent: true
    });
  }
  const rawRows = focusTransactions(summary, null, { effective: false });
  const effectiveRows = focusTransactions(summary, null, { effective: true });
  const rawFocusRows = controlling.focusCluster
    ? focusTransactions(summary, controlling.focusCluster, { effective: false })
    : [];
  const effectiveFocusRows = controlling.focusCluster
    ? focusTransactions(summary, controlling.focusCluster, { effective: true })
    : [];
  const payerAccounts = uniqueStrings([
    ...arrayOf(summary.source_account_clusters).map((row) => objectOf(row).payer_account),
    ...uniqueTransactionAccounts(rawRows, "payer_account")
  ], 20);
  const receiverAccounts = uniqueStrings([
    ...arrayOf(summary.counterparty_account_clusters).map((row) => objectOf(row).counterparty_acct),
    ...uniqueTransactionAccounts(rawRows, "receiver_account")
  ], 20);
  const sourceAccountsDeclared = summary.source_account_clusters !== undefined || summary.top_transactions !== undefined;
  const counterpartyAccountsDeclared = summary.counterparty_account_clusters !== undefined || summary.top_transactions !== undefined;
  const duplicateGroups = summary.duplicate_groups;
  const confirmedDuplicateAmount = optionalNumberValue(summary.confirmed_same_fact_duplicate_amount);
  const confirmedDuplicateCount = optionalCountValue(summary.confirmed_same_fact_duplicate_extra_count);
  const unresolvedDuplicateAmount = nonNegativeDifference(controlling.duplicateAmount, confirmedDuplicateAmount);
  const duplicateReviewStatus = confirmedDuplicateAmount > 0 && unresolvedDuplicateAmount === 0
    ? "confirmed_same_holder_cross_account_same_fact"
    : (confirmedDuplicateAmount > 0 && unresolvedDuplicateAmount > 0
      ? "mixed_confirmed_duplicate_and_unresolved_difference"
      : (optionalNumberValue(controlling.duplicateAmount) > 0
        ? "unresolved_duplicate_or_unsupported_difference"
        : optionalNumberValue(controlling.duplicateAmount) === 0
          ? "zero_difference_in_checked_scope"
          : "unresolved"));
  const focusEffectiveAmount = optionalNumberValue(controlling.focusCluster?.effective_amount);
  const focusEffectiveCount = optionalCountValue(controlling.focusCluster?.effective_count);
  const effectiveAmount = optionalNumberValue(controlling.effectiveAmount);
  const effectiveCount = optionalCountValue(controlling.effectiveCount);
  return withAnswerCardProtocol({
	    card_type: "pair_amount_review_card",
	    case_id: caseId,
	    intent: "pair_amount_review",
	    support_owner: "pair-amount-investigation",
	    focused_owner: "pair-amount-investigation",
	    title: "特定双方资金往来核验证据记录",
    fact_source: controlling.hasRankControl
      ? "production_backend_counterparty_dedup_aggregate"
      : "production_backend_controlled_workbench",
    holder_name: holderName,
    via_holder_name: viaName,
    source_scope: ["当前案件清洗/分析索引", "对手方排行仅作交叉核验"],
    validation_state: controlling.hasRankControl
      ? (workbenchOk ? "counterparty_dedup_aggregate_with_detail_support" : "counterparty_dedup_aggregate_only")
      : (workbenchOk
        ? (controlling.focusCluster ? "full_period_pair_amount_with_focus_cluster" : "full_period_pair_amount")
        : "workbench_gap"),
    delivery_state: factsReady ? "mini_review_ready" : "blocked",
    rank_counterparties_use: controlling.hasRankControl
      ? "effective_amount_control_when_detail_unavailable"
      : (controlling.rankCandidateAvailable ? "ranking_candidate_cross_check_not_final_pair_amount" : "ranking_candidate_only_not_final_pair_amount"),
    effective_stat_basis: controlling.hasRankControl
      ? "本次转账涉及账户内按重复风险复核后的对手方聚合"
      : "当前案件清洗/分析明细中付款人与收款人的完整两方转账复核；主要集中转账只作异常线索",
    detail_table_role: controlling.workbenchDisagrees
      ? "专项明细复核控制完整两方明细金额、笔数和统计期间；对手方排行差异列为需复核，不得改写成重点日期集中交易。"
      : "专项明细复核用于列示完整两方明细范围内的重点交易、账户集中和时间集中情况。",
    statistical_period: controlling.firstTxnAt || controlling.lastTxnAt ? {
      first_txn_at: controlling.firstTxnAt || undefined,
      last_txn_at: controlling.lastTxnAt || undefined
    } : undefined,
    first_txn_at: controlling.firstTxnAt || undefined,
    last_txn_at: controlling.lastTxnAt || undefined,
    raw_detail_amount: controlling.rawAmount,
    raw_detail_count: controlling.rawCount,
    high_confidence_same_fact_effective_amount: controlling.effectiveAmount,
    high_confidence_same_fact_count: controlling.effectiveCount,
    full_period_raw_amount: controlling.fullPeriodRawAmount,
    full_period_raw_count: controlling.fullPeriodRawCount,
    full_period_effective_amount: controlling.fullPeriodEffectiveAmount,
    full_period_effective_count: controlling.fullPeriodEffectiveCount,
    full_period_first_txn_at: controlling.rawFirstTxnAt || controlling.firstTxnAt || undefined,
    full_period_last_txn_at: controlling.rawLastTxnAt || controlling.lastTxnAt || undefined,
    full_period_effective_first_txn_at: controlling.firstTxnAt || undefined,
    full_period_effective_last_txn_at: controlling.lastTxnAt || undefined,
    principal_amount_after_fee_exclusion: controlling.exposeSmallAmount
      ? summary.principal_after_small_amount_exclusion
      : undefined,
    small_amount_candidate_amount: controlling.exposeSmallAmount ? summary.small_amount_candidate_amount : undefined,
    duplicate_or_unsupported_amount: controlling.duplicateAmount,
    confirmed_same_fact_duplicate_amount: confirmedDuplicateAmount,
    confirmed_same_fact_duplicate_count: confirmedDuplicateCount,
    unresolved_duplicate_or_unsupported_amount: unresolvedDuplicateAmount,
    duplicate_review_status: duplicateReviewStatus,
    duplicate_review_explanation: confirmedDuplicateAmount > 0
      ? "已按同一交易事实重复规则扣除不同卡号/账号下的重复记录；该部分不作为新增转账，也不应再要求办案人员复核是否重复。"
      : undefined,
    rank_candidate_amount: controlling.hasRankControl ? undefined : controlling.rankAmount,
    rank_candidate_count: controlling.hasRankControl ? undefined : controlling.rankCount,
    rank_candidate_duplicate_amount: controlling.hasRankControl ? undefined : controlling.rankDuplicateAmount,
    rank_cross_check_amount: controlling.hasRankControl ? undefined : controlling.rankAmount,
    rank_cross_check_count: controlling.hasRankControl ? undefined : controlling.rankCount,
    rank_cross_check_first_txn_at: controlling.hasRankControl ? undefined : controlling.rankFirstTxnAt || undefined,
    rank_cross_check_last_txn_at: controlling.hasRankControl ? undefined : controlling.rankLastTxnAt || undefined,
    detail_rank_difference_amount: controlling.detailRankDifferenceAmount,
    source_rank_effective_amount: controlling.hasRankControl ? controlling.effectiveAmount : undefined,
    source_rank_duplicate_amount: controlling.hasRankControl ? controlling.duplicateAmount : undefined,
    detail_query_auxiliary_only: controlling.hasRankControl && controlling.workbenchDisagrees ? true : undefined,
    focus_cluster: controlling.focusCluster || undefined,
    focus_cluster_role: controlling.focusCluster
      ? "重点异常集中交易，普通未限定时间的两方金额题不得把它当作完整两方明细总额。"
      : undefined,
    focus_cluster_effective_share: focusEffectiveAmount !== undefined && effectiveAmount > 0
      ? Math.round((focusEffectiveAmount / effectiveAmount) * 1000000) / 1000000
      : undefined,
    outside_focus_cluster_effective_amount: focusEffectiveAmount !== undefined && effectiveAmount !== undefined
      ? nonNegativeDifference(effectiveAmount, focusEffectiveAmount)
      : undefined,
    outside_focus_cluster_effective_count: focusEffectiveCount !== undefined && effectiveCount !== undefined
      ? nonNegativeDifference(effectiveCount, focusEffectiveCount)
      : undefined,
    source_account_count: sourceAccountsDeclared ? payerAccounts.length : undefined,
    source_accounts: sourceAccountsDeclared ? payerAccounts : undefined,
    counterparty_account_count: counterpartyAccountsDeclared ? receiverAccounts.length : undefined,
    counterparty_accounts: counterpartyAccountsDeclared ? receiverAccounts : undefined,
    account_clusters: summary.counterparty_account_clusters,
    source_account_clusters: summary.source_account_clusters,
    date_counterparty_clusters: summary.date_counterparty_clusters,
    time_clusters: summary.date_clusters,
    top_transactions: summary.effective_top_transactions === undefined ? undefined : effectiveRows,
    raw_transaction_candidates: summary.top_transactions === undefined ? undefined : rawRows,
    focus_cluster_transactions: effectiveFocusRows.length ? effectiveFocusRows : undefined,
    expanded_same_name_scope: workbenchOk ? {
      effective_amount: summary.effective_dedup_amount,
      effective_count: summary.effective_fact_count,
      raw_amount: summary.raw_detail_amount,
      raw_count: summary.raw_detail_count,
      first_txn_at: controlling.firstTxnAt || undefined,
      last_txn_at: controlling.lastTxnAt || undefined,
      note: "未限定时间、账号或特定交易集合的自然问句，以当前案件清洗/分析明细中的两方完整明细为主；主要集中转账另列为异常特征和续查线索。"
    } : undefined,
    duplicate_groups: duplicateGroups?.map((group) => pruneEmpty({
      dedupe_basis: text(group.dedupe_basis),
      account_count: optionalCountValue(group.account_count),
      payer_accounts: explicitArray(objectOf(group), "payer_accounts") === undefined
        ? undefined
        : uniqueStrings(group.payer_accounts, 8),
      raw_count: optionalCountValue(group.raw_count),
      raw_amount: optionalNumberValue(group.raw_amount),
      effective_amount: optionalNumberValue(group.effective_amount),
      duplicate_amount: optionalNumberValue(group.duplicate_amount)
    }) || {}),
    facts: [],
    dedup_key_safety: {
      forbidden_single_keys: ["空值", "查无信息", "unknown", "无", "null", "重复占位交易号"],
      required_joint_fields: ["持有人姓名", "证件号", "方向", "时间", "金额", "余额", "对手账号", "解析对手户名", "摘要/备注/类型", "成功状态"],
      same_holder_cross_account_rule: "仅当一次选择里包含多个账号，且持有人姓名、证件号、方向、时间、金额、余额、对手、流水/摘要等交易事实完全一致时，才把不同卡号/账号下的重复记录计一次。"
    },
    warnings: [
      ...(controlling.hasRankControl ? diagnosticWarnings(calls) : diagnosticWarnings([pairAmountWorkbench])),
      ...(!workbenchOk && !controlling.hasRankControl ? [{
        code: "PAIR_AMOUNT_WORKBENCH_GAP",
        severity: "blocking",
        message: "金额核验专项资金核算未执行；排行记录只能作为定位线索。"
      }] : []),
      ...(controlling.workbenchDisagrees ? [{
        code: "PAIR_AMOUNT_DETAIL_AGGREGATE_DISAGREEMENT",
        severity: "medium",
        message: "专项明细全范围复核与对手方排行聚合存在差异；未限定时间的两方金额题以专项明细复核为主，排行差异列为需复核，不得缩成重点日期集中交易。"
      }] : []),
      ...(controlling.focusCluster ? [{
        code: "PAIR_AMOUNT_FOCUS_CLUSTER_NOT_TOTAL",
        severity: "medium",
        message: "存在重点日期/账号集中交易；该交易集合用于异常特征和下游续查，不得替代未限定自然问句的完整两方明细总额。"
      }] : [])
    ],
    unsupported_claims: [
      "不得把对手方排行金额写成两方往来最终核验金额。",
      "不得把重点日期集中转账写成未限定时间问题的完整两方明细总额。",
      "不得把空值、查无信息、unknown、无、null 或重复占位交易号作为唯一去重主键。",
      "没有收款侧流水、回单和余额承接时，不得写成最终流向或后续闭环。"
    ],
    next_queries: [
      {
        tool: "trace_fund_next_hop",
        why_this_query: "仅在用户要求下一跳或流向穿透时，对重点收款账户/集中日期做逐笔承接核验。"
      },
      {
        tool: "rank_counterparties",
        why_this_query: "仅在用户要求 Top 对手方对照或改口径时重新排行；本轮 Pair Amount 不重复排行。"
      }
    ]
  }, {
    recommendedNextAction: factsReady ? "answer_now" : "repair_workbench_source_then_retry",
    maxAdditionalTools: factsReady ? 0 : 1,
    requiredFactsPresent: factsReady,
    unsupportedFlowsPresent: true
  });
}

function buildNameAssociationQuickFactCard({ caseId, names = [], workbench }) {
  const workbenchData = workbenchPayloadData(workbench);
  const workbenchOk = workbench?.ok === true && workbenchData.execution_status === "executed";
  const rows = workbenchOk && explicitArray(workbenchData, "records")
    ? workbenchData.records.map(objectOf)
    : [];
  const completeRows = rows.length > 0 && rows.every((row) =>
    text(row.target)
      && optionalCountValue(row.strict_txn_count) !== undefined
      && optionalNumberValue(row.strict_amount) !== undefined
      && explicitArray(row, "fuzzy_matches") !== undefined
  );
  const positiveRows = completeRows && rows.some((row) => optionalCountValue(row.strict_txn_count) > 0);
  const factsReady = workbenchOk && completeRows && positiveRows;
  const excludedRows = rows.flatMap((row) => arrayOf(row.fuzzy_matches).map((match) => ({
    target: text(row.target),
    matched_name: text(objectOf(match).matched_name),
    txn_count: optionalCountValue(objectOf(match).txn_count),
    amount: optionalNumberValue(objectOf(match).amount_sample)
  }))).filter((row) => row.matched_name && row.txn_count !== undefined && row.amount !== undefined);
  const cannotConfirm = [];
  return withAnswerCardProtocol({
    card_type: "name_association_quick_fact_card",
    case_id: caseId,
    title: "姓名/主体关联快查",
    holder_names: uniqueStrings(names, 8),
    validation_state: factsReady ? "bounded_workbench_executed" : "required_numeric_fields_missing",
    delivery_state: factsReady ? "quick_fact_ready" : "blocked",
    strict_rows: rows.map((row) => pruneEmpty({
      target: text(row.target),
      strict_txn_count: optionalCountValue(row.strict_txn_count),
      strict_amount: optionalNumberValue(row.strict_amount)
    }) || {}),
    excluded_near_names: excludedRows,
    cannot_confirm: cannotConfirm,
    match_scope: ["本方户名", "解析对手户名", "统计对手户名", "原始对手户名"],
    same_row_counted_once: true,
    warnings: diagnosticWarnings([workbench])
  }, {
    recommendedNextAction: factsReady ? "answer_now" : "repair_workbench_source_then_retry",
    maxAdditionalTools: factsReady ? 0 : 1,
    requiredFactsPresent: factsReady,
    unsupportedFlowsPresent: cannotConfirm.length > 0 || excludedRows.length > 0
  });
}

function schemaHasStockDetail(schemaResult) {
  if (schemaResult?.ok !== true) return undefined;
  const source = objectOf(schemaResult?.data || schemaResult);
  const nested = objectOf(source.data);
  const declaredTables = explicitArray(source, "tables") ?? explicitArray(nested, "tables");
  if (declaredTables === undefined) return undefined;
  const tables = declaredTables.map(objectOf);
  return tables.some((table) => /股票|持仓|证券流水|stock|position|security_trade/iu.test(text(table.table_name)));
}

function buildFinancialProductTopicCard({ caseId, holderName, workbench, schema }) {
  const workbenchData = workbenchPayloadData(workbench);
  const workbenchOk = workbench?.ok === true && workbenchData.execution_status === "executed";
  const record = workbenchOk ? firstRecord(workbenchData) : {};
  const subjectAccountCount = optionalCountValue(record.subject_account_count);
  const rawAccounts = explicitArray(record, "subject_accounts");
  const rawBuckets = explicitArray(record, "buckets");
  const accounts = rawAccounts?.map(objectOf);
  const buckets = rawBuckets?.map((bucket) => pruneEmpty({
    bucket: text(objectOf(bucket).bucket),
    direction: text(objectOf(bucket).direction),
    txn_count: optionalCountValue(objectOf(bucket).txn_count),
    amount: optionalNumberValue(objectOf(bucket).amount)
  }) || {});
  const requiredFactsComplete = subjectAccountCount > 0
    && accounts?.some((account) => text(account.account_key))
    && buckets?.some((bucket) => optionalCountValue(bucket.txn_count) > 0 && optionalNumberValue(bucket.amount) !== undefined);
  const factsReady = workbenchOk && requiredFactsComplete;
  const stockVisible = schemaHasStockDetail(schema);
  return withAnswerCardProtocol({
    card_type: "financial_product_topic_card",
    case_id: caseId,
    title: `${holderName || "主体"}基金/证券/股票专项资金核算`,
    holder_name: holderName,
    validation_state: factsReady ? "bounded_workbench_executed" : "required_numeric_fields_missing",
    delivery_state: factsReady ? "topic_review_ready" : "blocked",
    subject_account_count: subjectAccountCount,
    subject_accounts: accounts,
    holder_name_variants: accounts === undefined ? undefined : uniqueStrings(accounts.map((account) => text(account.open_name))),
    buckets,
    stock_detail_visible: stockVisible,
    warnings: diagnosticWarnings([workbench, schema])
  }, {
    recommendedNextAction: factsReady ? "answer_now" : "repair_workbench_source_then_retry",
    maxAdditionalTools: factsReady ? 0 : 1,
    requiredFactsPresent: factsReady,
    unsupportedFlowsPresent: stockVisible !== true
  });
}

function buildCompanyToPersonRecomputeCard({ caseId, workbench }) {
  const workbenchData = workbenchPayloadData(workbench);
  const workbenchOk = workbench?.ok === true && workbenchData.execution_status === "executed";
  const record = workbenchOk ? firstRecord(workbenchData) : {};
  const totalTxnCount = optionalCountValue(record.total_txn_count);
  const totalAmount = optionalNumberValue(record.total_amount);
  const companySubjectCount = optionalCountValue(record.company_subject_count);
  const personalSubjectCount = optionalCountValue(record.personal_subject_count);
  const summaryMissingTxnCount = optionalCountValue(record.summary_missing_txn_count);
  const summaryMissingAmount = optionalNumberValue(record.summary_missing_amount);
  const rawBuckets = explicitArray(record, "nature_buckets");
  const buckets = rawBuckets?.map((bucket) => pruneEmpty({
    nature_bucket: text(objectOf(bucket).nature_bucket),
    txn_count: optionalCountValue(objectOf(bucket).txn_count),
    amount: optionalNumberValue(objectOf(bucket).amount)
  }) || {});
  const requiredFactsComplete = totalTxnCount > 0
    && totalAmount !== undefined
    && companySubjectCount > 0
    && personalSubjectCount > 0
    && summaryMissingTxnCount !== undefined
    && summaryMissingAmount !== undefined
    && buckets?.some((bucket) => optionalCountValue(bucket.txn_count) > 0 && optionalNumberValue(bucket.amount) !== undefined);
  const factsReady = workbenchOk && requiredFactsComplete;
  return withAnswerCardProtocol({
    card_type: "company_to_person_recompute_card",
    case_id: caseId,
    title: "对公转个人资金重算",
    validation_state: factsReady ? "bounded_workbench_executed" : "required_numeric_fields_missing",
    delivery_state: factsReady ? "controlled_review_ready" : "blocked",
    total_txn_count: totalTxnCount,
    total_amount: totalAmount,
    company_subject_count: companySubjectCount,
    personal_subject_count: personalSubjectCount,
    summary_missing_txn_count: summaryMissingTxnCount,
    summary_missing_amount: summaryMissingAmount,
    nature_buckets: buckets,
    classification_rules: {
      company_name_markers: ["公司", "银行", "法院", "中心", "合作社", "店", "厂", "经营部", "企业", "单位"],
      recognized_compensation_markers: ["薪资", "工资", "劳务", "报销"],
      blank_summary_included_in_total: true
    },
    warnings: diagnosticWarnings([workbench])
  }, {
    recommendedNextAction: factsReady ? "answer_now" : "repair_workbench_source_then_retry",
    maxAdditionalTools: factsReady ? 0 : 1,
    requiredFactsPresent: factsReady,
    unsupportedFlowsPresent: true
  });
}

function compactFlowEdgeForCard(edge) {
  const item = objectOf(edge);
  return pruneEmpty({
    edge_status: text(item.edge_status),
    from_label: text(item.from_label || item.from),
    to_label: text(item.to_label || item.to),
    amount: optionalNumberValue(item.amount),
    txn_count: optionalCountValue(item.txn_count),
    txn_time: text(item.txn_time),
    summary: text(item.summary || item.txn_type || item.business_category_label || item.business_category),
    missing_fields: arrayOf(item.missing_fields).map(text).filter(Boolean)
  }) || {};
}

function completeSupportedFlowEdges(result) {
  const graph = objectOf(result?.flow_graph);
  return arrayOf(graph.edges || graph.edge_preview)
    .map(compactFlowEdgeForCard)
    .filter((edge) => text(edge.edge_status) === "supported"
      && text(edge.from_label)
      && text(edge.to_label)
      && optionalNumberValue(edge.amount) > 0
      && text(edge.txn_time));
}

function buildFlowGraphDeliveryCard({ caseId, holderName, viaName, questionText = "", inlineFlowGraph, sourceToViaSummary, sourceToViaScopedSummary, sourceToViaCoreAccountSummary }) {
  const flowGraph = objectOf(inlineFlowGraph?.flow_graph);
  const edges = arrayOf(flowGraph.edges || flowGraph.edge_preview).map(compactFlowEdgeForCard).filter((edge) => Object.keys(edge).length);
  const supportedEdges = edges.filter((edge) => text(edge.edge_status) === "supported"
    && text(edge.from_label)
    && text(edge.to_label)
    && optionalNumberValue(edge.amount) > 0
    && text(edge.txn_time));
  const boundaryEdges = edges.filter((edge) => !supportedEdges.includes(edge)).map((edge) => ({
    ...edge,
    edge_status: text(edge.edge_status) === "supported" ? "unresolved" : text(edge.edge_status) || "unresolved",
    missing_fields: [
      ...arrayOf(edge.missing_fields),
      !text(edge.from_label) ? "from" : "",
      !text(edge.to_label) ? "to" : "",
      optionalNumberValue(edge.amount) > 0 ? "" : "amount",
      text(edge.txn_time) ? "" : "txn_time"
    ].filter(Boolean)
  }));
  const downstreamEdges = supportedEdges.filter((edge) => text(edge.from_label).includes(viaName) || (!text(edge.from_label) && viaName));
  const productEdges = downstreamEdges.filter((edge) => /理财|基金|证券|申购|赎回|产品/u.test(`${text(edge.to_label)} ${text(edge.summary)}`));
  const requestedAmount = requestedAmountYuanFromQuestion(questionText);
  const coreSummaryMatchesRequest = requestedAmount !== undefined
    && pairSummaryFactComplete(sourceToViaCoreAccountSummary)
    && amountApproximatelyMatches(sourceToViaCoreAccountSummary.amount, requestedAmount);
  const mainSummary = pairSummaryFactComplete(sourceToViaScopedSummary)
    ? sourceToViaScopedSummary
    : coreSummaryMatchesRequest
      ? sourceToViaCoreAccountSummary
      : pairSummaryFactComplete(sourceToViaSummary) ? sourceToViaSummary : undefined;
  return withAnswerCardProtocol({
    card_type: "fund_flow_graph_delivery_card",
    case_id: caseId,
    title: "资金穿透图",
    holder_name: holderName,
    via_holder_name: viaName,
    validation_state: supportedEdges.length ? "bounded_workbench_executed" : "required_edge_fields_missing",
    delivery_state: supportedEdges.length ? "graph_ready" : "blocked",
    supported_edges: supportedEdges,
    boundary_edges: boundaryEdges,
    downstream_edges: downstreamEdges,
    product_or_asset_edges: productEdges,
    source_to_via_summary: mainSummary,
    source_to_via_date_window_summary: pairSummaryFactComplete(sourceToViaScopedSummary) ? sourceToViaScopedSummary : undefined,
    source_to_via_core_account_summary: pairSummaryFactComplete(sourceToViaCoreAccountSummary) ? sourceToViaCoreAccountSummary : undefined,
    required_evidence: ["重点收款账户流水", "银行回单", "开户资料", "产品凭证", "赎回记录", "现金取现后去向", "逐笔余额承接材料"],
    warnings: diagnosticWarnings([inlineFlowGraph])
  }, {
    recommendedNextAction: "answer_now",
    maxAdditionalTools: 0,
    requiredFactsPresent: supportedEdges.length > 0,
    unsupportedFlowsPresent: boundaryEdges.length > 0 || productEdges.length === 0
	  });
}

function userVisibleEntityName(value, fallback = "") {
  const rawName = text(value);
  if (!rawName || rawName === "__unknown__" || /^counterparty:unknown$/iu.test(rawName)) {
    const fallbackText = text(fallback);
    return fallbackText && !/^第\d+项$/u.test(fallbackText) ? fallbackText : "对手字段缺失";
  }
  return rawName;
}

function rankingFactRowComplete(row) {
  const source = objectOf(row);
  const identity = text(
    source.display_name
      || source.holder_name
      || source.account_display
      || source.account_key
      || source.counterparty_key
      || source.counterparty_account
  );
  const txnCount = optionalCountValue(source.txn_count);
  const amount = optionalNumberValue(
    objectOf(source.turnover).yuan
      ?? objectOf(source.outflow).yuan
      ?? objectOf(source.inflow).yuan
      ?? objectOf(source.metric_value).yuan
  );
  return Boolean(identity) && txnCount > 0 && amount > 0;
}

function buildBroadGraphTableCard({ caseId, casegraph, counterpartyRank }) {
  const casegraphReady = text(casegraph?.status) === "ok";
  const graphFacts = casegraphReady ? objectOf(casegraph?.key_facts) : {};
  const topAccounts = compactRankingRows(arrayOf(graphFacts.top_accounts?.rows || graphFacts.top_accounts), 5)
    .filter(rankingFactRowComplete);
  const topHolders = compactRankingRows(arrayOf(graphFacts.top_holders?.rows || graphFacts.top_holders), 5)
    .filter(rankingFactRowComplete);
  const topCounterparties = compactRankingRows(
    arrayOf(
      counterpartyRank?.ok === true
        ? counterpartyRank.data?.rankings
        : graphFacts.top_counterparties?.rows || graphFacts.top_counterparties
    ),
    8
  );
  const counterpartyRows = topCounterparties.length
    ? topCounterparties
    : compactRankingRows(arrayOf(graphFacts.counterparties || graphFacts.top_counterparties), 8);
  const completeCounterpartyRows = counterpartyRows.filter(rankingFactRowComplete);
  const factsReady = completeCounterpartyRows.length > 0;
  return withAnswerCardProtocol({
    card_type: "broad_graph_table_card",
    case_id: caseId,
    title: "当前案件资金流向图谱和重点对手方表格",
    validation_state: factsReady ? "bounded_workbench_executed" : "needs_evidence",
    delivery_state: factsReady ? "graph_table_ready" : "blocked",
    top_accounts: topAccounts,
    top_holders: topHolders,
    top_counterparties: completeCounterpartyRows,
    supported_edges: [],
    boundary_edges: [],
    required_evidence: ["交易级流水", "银行回单", "开户资料", "用途材料", "余额承接"],
    warnings: compactWarningList(casegraph?.warnings, counterpartyRank?.warnings)
  }, {
    recommendedNextAction: factsReady ? "answer_now" : "repair_source_then_retry",
    maxAdditionalTools: factsReady ? 0 : 1,
    requiredFactsPresent: factsReady,
    unsupportedFlowsPresent: true
  });
}

function rankingEntityName(row, fallback = "待核对象") {
  const item = objectOf(row);
  return userVisibleEntityName(
    item.display_name || item.holder_name || item.account_open_name || item.account_display || item.account_key || item.counterparty_key || item.counterparty_account,
    fallback
  );
}

function rankingRowsFromFact(source) {
  if (Array.isArray(source)) return source;
  const data = objectOf(source);
  if (Array.isArray(data.rankings)) return data.rankings;
  const rows = [];
  const buckets = [
    ["outflow_by_amount", "出账"],
    ["inflow_by_amount", "入账"],
    ["by_amount", ""],
    ["top_counterparties", ""],
    ["rows", ""]
  ];
  for (const [key, direction] of buckets) {
    for (const row of arrayOf(data[key])) {
      const next = { ...objectOf(row) };
      if (next.total_amount != null && next.turnover_total == null) next.turnover_total = next.total_amount;
      if (direction && !text(next.ranking_direction)) next.ranking_direction = direction;
      rows.push(next);
    }
    if (rows.length) break;
  }
  return rows;
}

function firstRankingRows(...sources) {
  for (const source of sources) {
    const rows = rankingRowsFromFact(source);
    if (rows.length) return rows;
  }
  return [];
}

function sourceToViaSummaryFromPairAmountReviewCard(card, fallback = {}) {
  const source = objectOf(card);
  const fallbackSummary = objectOf(fallback);
  const focusCluster = objectOf(source.focus_cluster);
  const focusCount = optionalCountValue(focusCluster.effective_count);
  const focusAmount = optionalNumberValue(focusCluster.effective_amount);
  const focusComplete = Boolean(text(focusCluster.date)) && focusCount !== undefined && focusAmount !== undefined;
  return pruneEmpty({
    amount: optionalNumberValue(source.high_confidence_same_fact_effective_amount ?? source.full_period_effective_amount),
    txn_count: optionalCountValue(source.high_confidence_same_fact_count ?? source.full_period_effective_count),
    first_txn_at: text(source.full_period_effective_first_txn_at || source.first_txn_at || fallbackSummary.first_txn_at),
    last_txn_at: text(source.full_period_effective_last_txn_at || source.last_txn_at || fallbackSummary.last_txn_at),
    source_accounts: explicitArray(source, "source_accounts") ?? explicitArray(fallbackSummary, "source_accounts"),
    counterparty_accounts: explicitArray(source, "counterparty_accounts") ?? explicitArray(fallbackSummary, "counterparty_accounts"),
    duplicate_amount: optionalNumberValue(source.confirmed_same_fact_duplicate_amount ?? source.duplicate_or_unsupported_amount),
    duplicate_row_count: optionalCountValue(source.confirmed_same_fact_duplicate_count),
    duplicate_review_status: text(source.duplicate_review_status),
    duplicate_review_explanation: text(source.duplicate_review_explanation),
    largest_date_cluster: focusComplete
      ? {
          date: text(focusCluster.date),
          txn_count: focusCount,
          amount: focusAmount
        }
      : fallbackSummary.largest_date_cluster
  }) || {};
}

function pairSummaryFactComplete(summary) {
  const source = objectOf(summary);
  return optionalNumberValue(source.amount) > 0 && optionalCountValue(source.txn_count) > 0;
}

function mergePairSummaryAccounts(summary, graphSummary) {
  const source = objectOf(summary);
  if (!pairSummaryFactComplete(source)) return undefined;
  const graph = objectOf(graphSummary);
  return pruneEmpty({
    ...source,
    source_accounts: explicitArray(graph, "source_accounts") ?? explicitArray(source, "source_accounts"),
    counterparty_accounts: explicitArray(graph, "counterparty_accounts") ?? explicitArray(source, "counterparty_accounts")
  });
}

function accountStatsFactComplete(stats) {
  const source = objectOf(stats);
  const txnCount = optionalCountValue(source.txn_count);
  const turnover = optionalNumberValue(objectOf(source.turnover).yuan ?? source.turnover_total);
  return txnCount > 0 && turnover > 0;
}

function ownerScopeFactComplete(scope) {
  const source = objectOf(scope);
  return optionalCountValue(source.account_count) > 0
    && Array.isArray(source.account_keys_preview)
    && source.account_keys_preview.some((account) => text(account));
}

function holderDossierFactsComplete(holderAnalysis, holderTopAccounts) {
  if (holderAnalysis?.ok !== true || holderTopAccounts?.ok !== true) return false;
  const holderData = objectOf(holderAnalysis.data);
  const accountAnalysis = objectOf(holderData.account_analysis);
  const scope = compactOwnerScope(holderData.holder_scope || holderData.owner_scope || holderData.subject_scope || {});
  const stats = compactAccountStatsFact(accountAnalysis.account_stats || holderData.account_stats || {});
  const coverage = objectOf(accountAnalysis.coverage);
  const topAccounts = compactRankingRows(holderTopAccounts.data?.rankings, 8);
  return ownerScopeFactComplete(scope)
    && accountStatsFactComplete(stats)
    && coveragePayloadComplete(coverage)
    && topAccounts.some(rankingFactRowComplete);
}

function accountDossierFactsComplete(accountKey, accountAnalysis) {
  if (!text(accountKey) || accountAnalysis?.ok !== true) return false;
  const accountData = objectOf(accountAnalysis.data);
  const detail = objectOf(accountData.account_analysis || accountData);
  const stats = compactAccountStatsFact(detail.account_stats || accountData.account_stats || {});
  const coverage = objectOf(detail.coverage || accountData.coverage);
  return accountStatsFactComplete(stats) && coveragePayloadComplete(coverage);
}

function qaReviewPayloadComplete(qualityResult, duplicateResult) {
  if (qualityResult?.ok !== true || duplicateResult?.ok !== true) return false;
  const quality = objectOf(qualityResult.data);
  const importSummary = objectOf(objectOf(quality.import_lineage).summary);
  const cleaning = objectOf(quality.cleaning_quality);
  const qualityEligibility = objectOf(cleaning.same_fact_eligibility);
  const flags = objectOf(cleaning.flags);
  const duplicateQuality = objectOf(cleaning.duplicate_summary);
  const accountQuality = objectOf(quality.account_identity_quality || quality.account_quality);
  const duplicateData = objectOf(duplicateResult.data);
  const duplicateEligibility = objectOf(duplicateData.same_fact_eligibility);
  const duplicateSummary = objectOf(duplicateData.summary);
  const sameHolder = objectOf(duplicateSummary.same_holder_same_fact);
  const fullCase = objectOf(duplicateSummary.full_case_review_only);
  const policy = objectOf(duplicateData.dedupe_policy);
  const blockingWarning = [...arrayOf(qualityResult.warnings), ...arrayOf(duplicateResult.warnings)]
    .some((warning) => ["blocking", "error"].includes(text(objectOf(warning).severity)));
  return !blockingWarning
    && sameFactEligibilityComplete(qualityEligibility)
    && sameFactEligibilityComplete(duplicateEligibility)
    && text(duplicateData.coverage_status) === "complete"
    && !text(duplicateData.blocker)
    && ["file_log_count", "rows_total", "rows_imported_norm", "rows_dedup"]
      .every((key) => optionalCountValue(importSummary[key]) !== undefined)
    && optionalCountValue(flags.txn_total) !== undefined
    && optionalCountValue(
      duplicateQuality.same_fact_cross_account_groups
        ?? duplicateQuality.same_fact_cross_account_group_count
    ) !== undefined
    && optionalCountValue(duplicateQuality.same_fact_cross_account_extra_rows) !== undefined
    && optionalCountValue(accountQuality.account_count) !== undefined
    && optionalCountValue(
      accountQuality.unregistered_account_count
        ?? accountQuality.unregistered_holder_account_count
        ?? accountQuality.empty_holder_account_count
    ) !== undefined
    && explicitArray(duplicateData, "families") !== undefined
    && optionalCountValue(sameHolder.group_count) !== undefined
    && optionalCountValue(sameHolder.extra_rows) !== undefined
    && optionalCountValue(fullCase.cross_holder_group_count ?? fullCase.group_count) !== undefined
    && policy.full_case_blind_dedupe_allowed === false
    && policy.deduct_from_totals_allowed === false;
}

function coveragePayloadComplete(coverage) {
  const source = objectOf(coverage);
  const txnTotal = optionalCountValue(source.txn_total);
  const txnAnalyzed = optionalCountValue(source.txn_analyzed);
  const accountCount = optionalCountValue(source.account_count ?? source.selected_account_count);
  return txnTotal > 0
    && txnAnalyzed > 0
    && txnAnalyzed === txnTotal
    && accountCount > 0
    && Boolean(text(source.date_min))
    && Boolean(text(source.date_max));
}

function completeRankingRowsFromResult(result, limit) {
  if (result?.ok !== true) return [];
  return compactRankingRows(result.data?.rankings, limit).filter(rankingFactRowComplete);
}

function buildSubjectDossierCard({ caseId, holderName, holderAnalysis, holderTopAccounts }) {
  const factsReady = holderDossierFactsComplete(holderAnalysis, holderTopAccounts);
  const holderData = holderAnalysis?.ok === true ? objectOf(holderAnalysis.data) : {};
  const accountAnalysis = objectOf(holderData.account_analysis);
  const scope = compactOwnerScope(holderData.holder_scope || holderData.owner_scope || holderData.subject_scope || {});
  const stats = compactAccountStatsFact(accountAnalysis.account_stats || holderData.account_stats || {});
  const topAccounts = holderTopAccounts?.ok === true
    ? compactRankingRows(holderTopAccounts.data?.rankings, 8).filter(rankingFactRowComplete)
    : [];
  const counterpartyRows = compactRankingRows(
    firstRankingRows(accountAnalysis.counterparty_rankings),
    8
  ).filter((row) => rankingFactRowComplete(row) && rankingEntityName(row, "") !== holderName);
  const accounts = uniqueStrings(scope.account_keys_preview, 10);
  return withAnswerCardProtocol({
    card_type: "subject_dossier_card",
    case_id: caseId,
    title: "主体账户研判",
    holder_name: holderName,
    validation_state: factsReady ? "ready_from_holder_facts" : "needs_evidence",
    delivery_state: factsReady ? "dossier_ready" : "blocked",
    holder_scope: scope,
    registered_accounts: accounts,
    account_stats: stats,
    top_accounts: topAccounts,
    counterparty_rankings: counterpartyRows,
    investigative_focus: ["大额集中进出", "本人/同名账户往来", "同事实或换卡重复放大", "缺失对手方", "现金断点", "理财/证券/基金资产端线索"],
    required_evidence: ["重点账户完整流水", "开户资料", "银行回单", "收款侧流水", "用途合同/发票/说明", "产品凭证", "余额承接记录"],
    warnings: compactWarningList(holderAnalysis?.warnings, holderTopAccounts?.warnings)
  }, {
    recommendedNextAction: factsReady ? "answer_now" : "repair_source_then_retry",
    maxAdditionalTools: factsReady ? 0 : 1,
    requiredFactsPresent: factsReady,
    unsupportedFlowsPresent: true
  });
}

function buildAccountDossierCard({ caseId, accountKey, accountAnalysis, counterpartyRank }) {
  const factsReady = accountDossierFactsComplete(accountKey, accountAnalysis);
  const accountData = accountAnalysis?.ok === true ? objectOf(accountAnalysis.data) : {};
  const detail = objectOf(accountData.account_analysis || accountData);
  const stats = compactAccountStatsFact(detail.account_stats || accountData.account_stats || {});
  const counterpartyRows = compactRankingRows(
    firstRankingRows(counterpartyRank?.ok === true ? counterpartyRank.data : {}, detail.counterparty_rankings, accountData.counterparty_rankings),
    8
  ).filter(rankingFactRowComplete);
  return withAnswerCardProtocol({
    card_type: "account_dossier_card",
    case_id: caseId,
    title: "账户研判",
    account_key: accountKey,
    validation_state: factsReady ? "ready_from_account_facts" : "needs_evidence",
    delivery_state: factsReady ? "dossier_ready" : "blocked",
    account_stats: stats,
    counterparty_rankings: counterpartyRows,
    investigative_focus: ["大额交易", "集中出入", "现金/产品用途", "同事实/换卡", "对手方缺失"],
    required_evidence: ["账户完整流水", "开户资料", "交易回单", "收款侧流水", "用途合同/发票/说明", "余额承接记录"],
    warnings: compactWarningList(accountAnalysis?.warnings, counterpartyRank?.warnings)
  }, {
    recommendedNextAction: factsReady ? "answer_now" : "repair_source_then_retry",
    maxAdditionalTools: factsReady ? 0 : 1,
    requiredFactsPresent: factsReady,
    unsupportedFlowsPresent: true
  });
}

function diagnosticWarnings(calls = []) {
  return arrayOf(calls).flatMap((call) => arrayOf(call?.warnings)).slice(0, 10);
}

function diagnosticFrontdoorResponse({ caseId, resolved, card, nextActions = [] }) {
  const protocolCard = objectOf(card).answer_card_complete === true
    ? card
    : withAnswerCardProtocol(card);
  const factsReady = protocolCard.required_facts_present === true
    && !["blocked", "needs_evidence"].includes(text(protocolCard.delivery_state));
  const warnings = arrayOf(protocolCard.warnings);
  return {
    tool: "funds_investigate",
    status: factsReady ? "ok" : "partial",
    case_id: caseId,
    answer_card: protocolCard,
    key_facts: {
      diagnostic_card: protocolCard
    },
    warnings,
    evidence_refs: {},
      next_actions: factsReady && nextActions.length ? nextActions : [
        {
          reason: factsReady
            ? "本轮支持事实已收敛；由当前 lead owner 组织研判。若用户继续追一层、调整统计范围、指定新对象或提出新假设，可进入对应核验流程。"
            : "本轮必需事实、覆盖范围或数值字段不完整；只能说明能力边界并重试当前数据源，不得输出零命中或案件事实。"
        }
      ],
      answer_contract: {
      support_material_only: "本轮仅复用结构化支持事实，不得把支持材料当作最终研判。同一问题已完成时只抑制重复入口调用，不阻断新对象、新统计范围或新线索的补充核验。",
      case_name: text(resolved.case?.case_name || resolved.case?.name)
    }
  };
}

export function createFrontdoorRuntime({
  resolveCase,
  safeSkill,
  getCaseGraph,
  buildFundFlowGraph,
  buildDestinationOutflowDiagnosticCard,
  buildViaOneHopDiagnosticCard,
  buildViaContinuationDiagnosticCard,
  buildTopRankingsDiagnosticCard,
  buildClaimReviewDiagnosticCard,
  buildInvestigationLabDiagnosticCard,
  env = {}
}) {
	  async function fundsInvestigate(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const initialRuntimeArgs = objectOf(args);
    const initialIntent = inferFrontDoorIntent(initialRuntimeArgs);
    const initialQuestion = text(initialRuntimeArgs.question || initialRuntimeArgs.report_text);
    if (initialIntent === "full_case" || /(?:正式)?(?:报告|材料|简报|附件|附表|导出|发布)/u.test(initialQuestion)) {
      return reportPublicationBlockedPayload({
        tool: "funds_investigate",
        case_id: text(initialRuntimeArgs.case_id || initialRuntimeArgs.caseId),
        status: "blocked"
      });
    }
    const frontdoorSafeSkill = (skillId, input = {}) =>
      safeSkill(skillId, withInternalRuntimeContext(initialRuntimeArgs, input), { signal });
	    const resolved = await resolveCase(args, { signal });
	    const caseId = resolved.case_id;
	    const questionText = text(args.question);
	    const inferredAccountKeys = extractAccountKeysFromQuestion(questionText);
	    if (!arrayOf(args.account_keys).length && inferredAccountKeys.length) {
	      args = { ...args, account_keys: inferredAccountKeys };
	    }
	    const requestedIntent = inferFrontDoorIntent(args);
	    let holderName = text(args.holder_name);
	    if (requestedIntent === "holder_analysis" && !holderName) {
	      holderName = inferHolderNameForDossier(questionText);
	      if (holderName) args = { ...args, holder_name: holderName };
	    }
	    const explicitViaName = text(args.via_holder_name || args.counterparty_name);
	    const focusKeywords = arrayOf(args.focus_keywords).map(text).filter(Boolean);
	    const graphRouteHolderHint = requestedIntent === "flow_graph"
	      ? holderName || inferSourceHolderFromQuestion(questionText, { viaName: explicitViaName })
	      : "";
	    const graphRouteViaHint = requestedIntent === "flow_graph"
	      ? explicitViaName || inferViaNameFromQuestion(questionText, { holderName: graphRouteHolderHint, focusKeywords })
	      : "";
	    const qaReviewTransferRoute = requestedIntent === "qa_review"
	      && holderName
	      && explicitViaName
	      && isExplicitTransferAmountQuestion(questionText, { holderName, viaName: explicitViaName });
	    const graphTransferRoute = requestedIntent === "flow_graph" && graphRouteHolderHint && graphRouteViaHint;
	    const intent = graphTransferRoute || qaReviewTransferRoute ? "destination" : requestedIntent;
	    const requestedGraph = requestedIntent === "flow_graph";
    if (!text(args.date_start)) {
      const inferredDateStart = inferDateStartFromQuestion(questionText);
      if (inferredDateStart) args = { ...args, date_start: inferredDateStart };
    }
	    if (intent === "destination" && !holderName) {
	      holderName = graphRouteHolderHint || inferSourceHolderFromQuestion(questionText, { viaName: explicitViaName });
	    }
	    const viaName = intent === "destination"
	      ? explicitViaName || graphRouteViaHint || (isTopOutflowClassificationQuestion(questionText) ? "" : inferViaNameFromQuestion(questionText, { holderName, focusKeywords }))
	      : "";
    const wantsViaContinuation = intent === "destination" && viaName && isViaContinuationQuestion(questionText);
    if (intent === "destination" && !holderName) {
      holderName = inferSourceHolderFromQuestion(questionText, { viaName });
    }
    const reportGradeIntent = intent === "full_case";
    const wantsFlowGraph = frontdoorWantsFlowGraph({ intent, requestedGraph, questionText });
    const wantsGraphContinuation = intent === "destination" && viaName && wantsFlowGraph;
    const wantsPairAmountReview = intent === "destination"
      && holderName
      && viaName
      && !wantsViaContinuation
      && !wantsFlowGraph
      && isExplicitTransferAmountQuestion(questionText, { holderName, viaName });
    const wantsPairAmountControl = intent === "destination"
      && holderName
      && viaName
      && (
        wantsPairAmountReview
        || wantsViaContinuation
        || wantsGraphContinuation
        || isExplicitTransferAmountQuestion(questionText, { holderName, viaName })
      );
    const intentAst = buildIntentAst({
      requestedIntent,
      intent,
      holderName,
      viaName,
      accountKeys: args.account_keys,
      dateStart: text(args.date_start),
      dateEnd: text(args.date_end),
      wantsViaContinuation,
      wantsFlowGraph,
      reportGradeIntent
    });
    const planDag = buildPlanDag(intentAst);
    const frontdoorToolBudget = objectOf(intentAst.tool_budget);
    const maxCards = clampInt(args.max_cards, 6, 1, 12);
    if (intent === "name_association_quick_fact") {
      const targetNames = extractLikelyNamesFromQuestion(questionText, focusKeywords);
      const workbench = targetNames.length
        ? await frontdoorSafeSkill("run_case_sql", {
            case_id: caseId,
            purpose: "姓名/主体关联快查：当前案件内按交易字段严格命中与近名排除桶聚合；轻量快查，不扩展为全案深挖。",
            sql: nameAssociationQuickFactSql(targetNames),
            row_limit: 20,
            result_mode: "aggregate",
            allowed_view_policy: "cleaned_and_analysis_only"
          })
        : {
            ok: false,
            data: {},
            warnings: [{
              code: "NAME_QUICK_FACT_TARGET_MISSING",
              severity: "warning",
              message: "未识别到明确姓名，无法执行姓名/主体关联快查。"
            }]
          };
      const card = buildNameAssociationQuickFactCard({
        caseId,
        names: targetNames,
        workbench
      });
      const response = diagnosticFrontdoorResponse({
        caseId,
        resolved,
        card,
        nextActions: [
          { reason: "已完成轻量关联快查；如需确认身份关系，下一步补开户资料、证件号和回单。" }
        ]
      });
      return response;
    }

    if (intent === "financial_product_topic") {
      holderName = inferHolderNameForTopic(questionText, holderName);
      const [workbench, schema] = await Promise.all([
        holderName
          ? frontdoorSafeSkill("run_case_sql", {
              case_id: caseId,
              purpose: "基金/证券/股票专题：按主体户名、同证件号和同名变体账户全集，核算基金、证券三方存管和产品申赎线索。",
              sql: financialProductTopicSql(holderName),
              row_limit: 20,
              result_mode: "aggregate",
              allowed_view_policy: "cleaned_and_analysis_only"
            })
          : Promise.resolve({
              ok: false,
              data: {},
              warnings: [{
                code: "FINANCIAL_TOPIC_HOLDER_MISSING",
                severity: "warning",
                message: "未识别到专题主体，无法核算基金/证券/股票专题。"
              }]
            }),
        frontdoorSafeSkill("inspect_case_schema", {
          case_id: caseId,
          table_limit: 500,
          column_limit: 200,
          include_columns: true
        })
      ]);
      const card = buildFinancialProductTopicCard({
        caseId,
        holderName,
        workbench,
        schema
      });
      const response = diagnosticFrontdoorResponse({
        caseId,
        resolved,
        card,
        nextActions: [
          { reason: "已完成主体账户全集下的金融产品专项资金核算；股票明细不足时，下一步补调证券流水或持仓明细。" }
        ]
      });
      return response;
    }

    if (intent === "company_to_person_recompute") {
      const workbench = await frontdoorSafeSkill("run_case_sql", {
        case_id: caseId,
        purpose: "对公转个人专项重算：先定义对公主体、个人主体、摘要缺失和薪资/工资/劳务/报销可识别口径，再输出总额和分桶金额。",
        sql: companyToPersonRecomputeSql(),
        row_limit: 20,
        result_mode: "aggregate",
        allowed_view_policy: "cleaned_and_analysis_only"
      });
      const card = buildCompanyToPersonRecomputeCard({
        caseId,
        workbench
      });
      const response = diagnosticFrontdoorResponse({
        caseId,
        resolved,
        card,
        nextActions: [
          { reason: "已完成对公转个人专项资金核算；正式材料需结合开户类型、主体身份资料和报告事实结论复核。" }
        ]
      });
      return response;
    }

	    if (intent === "casegraph") {
	      const response = await getCaseGraph({ ...args, case_id: caseId }, { signal });
	      return response;
	    }

	    if (intent === "flow_graph") {
	      const [casegraphResult, counterpartyRank] = await Promise.all([
	        getCaseGraph({ ...args, case_id: caseId, include_rankings: true }, { signal }),
	        frontdoorSafeSkill("rank_counterparties", {
	          case_id: caseId,
	          metric: "turnover",
	          direction_mode: "both",
	          success_filter: "all",
	          cash_filter: "all",
	          counterparty_group_mode: "name",
	          limit: clampInt(args.top_n, 10, 1, 20)
	        })
	      ]);
	      const card = buildBroadGraphTableCard({
	        caseId,
	        casegraph: casegraphResult,
	        counterpartyRank
	      });
	      const response = diagnosticFrontdoorResponse({
	        caseId,
	        resolved,
	        card,
	        nextActions: [
	          { reason: "已按当前案件关系图和重点对手方排行生成图表摘要；如需确定性资金穿透图，请指定来源主体、收款对手、时间窗或交易种子。" }
	        ]
	      });
	      return response;
	    }

	    if (intent === "claim_review") {
      const card = await buildClaimReviewDiagnosticCard(caseId, args, { signal });
	      const response = diagnosticFrontdoorResponse({
	        caseId,
	        resolved,
	        card,
	        nextActions: [
	          {
	            reason: "如果需要形成正式报告级结论，先补做结论引用核验；普通复核答复先基于本次材料纠正金额，并降级无证据链路。"
	          }
	        ]
	      });
      return response;
    }

    if (intent === "old_thread_patterns") {
      const card = await buildInvestigationLabDiagnosticCard(caseId, args, { signal });
      const response = diagnosticFrontdoorResponse({
        caseId,
        resolved,
        card,
        nextActions: [
          {
            reason: "已返回开放式深挖的核验材料、假设队列、证据缺口和事实化风险项；应先输出研判，不要把未证实假设写成事实。"
          }
        ]
      });
      return response;
    }

	    const needsCasegraphContext = intent === "full_case" || args.include_casegraph_context === true;
    const casegraph = needsCasegraphContext
      ? await getCaseGraph({
          ...args,
          case_id: caseId,
          include_rankings: true
        }, { signal })
      : {
          tool: "get_casegraph",
          status: "skipped",
          case_id: caseId,
          key_facts: {},
          warnings: [],
          evidence_refs: {}
        };
	    const childCalls = [];
	    const rankingTarget = inferRankingTarget(args);
	    const rankingMetric = inferRankingMetric(args);
	    const rankingMetrics = intent === "ranking" ? inferRankingMetrics(args) : [rankingMetric];
	    const accountKeys = arrayOf(args.account_keys).map(text).filter(Boolean);
    const counterpartyGroupMode = text(args.counterparty_group_mode || args.counterpartyGroupMode);
	    const rankingPayload = {
	      case_id: caseId,
	      holder_name: holderName,
	      id_no: text(args.id_no),
	      account_keys: accountKeys,
      date_start: text(args.date_start),
      date_end: text(args.date_end),
      match_mode: "exact",
      metric: rankingMetric,
      direction_mode: intent === "ranking" ? "both" : directionModeForMetric(rankingMetric),
      success_filter: "all",
      cash_filter: "all",
      ...(rankingTarget === "counterparties"
        ? { counterparty_group_mode: counterpartyGroupMode === "account" ? "account" : "name" }
        : {}),
      limit: clampInt(args.top_n, intent === "ranking" ? 10 : 8, 1, 50)
    };
	    if (intent === "holder_analysis" && holderName) {
	      childCalls.push(frontdoorSafeSkill("analyze_holder_full", {
        case_id: caseId,
        holder_name: holderName,
        id_no: text(args.id_no),
        match_mode: "exact",
        date_start: text(args.date_start),
        date_end: text(args.date_end),
        counterparty_limit: 10
	      }));
	      childCalls.push(frontdoorSafeSkill("rank_accounts", rankingPayload));
	    } else if (intent === "account_analysis" && accountKeys.length) {
	      childCalls.push(frontdoorSafeSkill("analyze_account_full", {
	        case_id: caseId,
	        account_key: accountKeys[0],
	        account_keys: accountKeys,
	        date_start: text(args.date_start),
	        date_end: text(args.date_end),
	        counterparty_limit: 10
	      }));
	      childCalls.push(frontdoorSafeSkill("rank_counterparties", {
	        ...rankingPayload,
	        holder_name: "",
	        direction_mode: "both",
	        metric: "turnover",
	        limit: 10
	      }));
	    } else if (intent === "ranking") {
      for (const metric of rankingMetrics) {
        childCalls.push(frontdoorSafeSkill(rankingSkillForTarget(rankingTarget), {
          ...rankingPayload,
          metric,
          direction_mode: "both"
        }));
      }
    } else if (intent === "destination" && holderName) {
      const destinationLimit = clampInt(args.top_n, 20, 1, 100);
      const destinationRankFetchLimit = Math.min(destinationLimit + 10, 100);
      if (!viaName) {
        childCalls.push(frontdoorSafeSkill("trace_subject_top_outflows", {
          case_id: caseId,
          holder_name: holderName,
          id_no: text(args.id_no),
          account_keys: arrayOf(args.account_keys).map(text).filter(Boolean),
          date_start: text(args.date_start),
          date_end: text(args.date_end),
          top_n: destinationLimit,
          dedupe_seed_txn_id: true
        }));
      }
      childCalls.push(frontdoorSafeSkill("rank_counterparties", {
        case_id: caseId,
        holder_name: holderName,
        id_no: text(args.id_no),
        account_keys: arrayOf(args.account_keys).map(text).filter(Boolean),
        direction_mode: "out",
        metric: "outflow",
        success_filter: "all",
        cash_filter: "all",
        dedupe_same_holder_same_fact: true,
        counterparty_group_mode: "name",
        limit: destinationRankFetchLimit
      }));
      if (viaName) {
        childCalls.push(frontdoorSafeSkill("rank_counterparties", {
          case_id: caseId,
          holder_name: holderName,
          id_no: text(args.id_no),
          account_keys: arrayOf(args.account_keys).map(text).filter(Boolean),
          direction_mode: "out",
          metric: "outflow",
          success_filter: "all",
          cash_filter: "all",
          dedupe_same_holder_same_fact: true,
          counterparty_group_mode: "account",
          limit: destinationRankFetchLimit
        }));
      }
      if (viaName && (text(args.date_start) || text(args.date_end))) {
        childCalls.push(frontdoorSafeSkill("rank_counterparties", {
          case_id: caseId,
          holder_name: holderName,
          id_no: text(args.id_no),
          account_keys: arrayOf(args.account_keys).map(text).filter(Boolean),
          date_start: text(args.date_start),
          date_end: text(args.date_end),
          direction_mode: "out",
          metric: "outflow",
          success_filter: "all",
          cash_filter: "all",
          dedupe_same_holder_same_fact: true,
          counterparty_group_mode: "name",
          limit: destinationRankFetchLimit
        }));
      }
	    } else if (intent === "qa_review") {
      childCalls.push(frontdoorSafeSkill("audit_case_data_quality", {
        case_id: caseId,
        example_limit: 5
      }));
      childCalls.push(frontdoorSafeSkill("resolve_duplicate_families", {
        case_id: caseId,
        holder_name: holderName,
        id_no: text(args.id_no),
        account_keys: arrayOf(args.account_keys).map(text).filter(Boolean),
        scope_mode: "same_holder_accounts",
        limit: 5
      }));
    } else if (intent === "full_case") {
      childCalls.push(frontdoorSafeSkill("plan_case_analysis", {
        case_id: caseId,
        analysis_goal: text(args.question) || "full_case",
        holder_name: holderName,
        id_no: text(args.id_no),
        account_keys: arrayOf(args.account_keys).map(text).filter(Boolean),
        max_lanes: 4
      }));
    } else {
      childCalls.push(frontdoorSafeSkill("run_investigation_lab", {
        case_id: caseId,
        analysis_goal: text(args.question) || "open_discovery",
        holder_name: holderName,
        id_no: text(args.id_no),
        account_keys: arrayOf(args.account_keys).map(text).filter(Boolean),
        focus_keywords: focusKeywords,
        max_hypotheses: maxCards,
        include_top_outflows: Boolean(holderName || arrayOf(args.account_keys).length),
        include_full_case_rankings: true
      }));
    }
	    if (wantsPairAmountControl) {
	      childCalls.push(frontdoorSafeSkill("resolve_duplicate_families", {
	        case_id: caseId,
	        holder_name: holderName,
	        date_start: text(args.date_start),
	        date_end: text(args.date_end),
	        direction_mode: "out",
	        scope_mode: "same_holder_accounts",
	        limit: 5
	      }));
	    }
	    const initialResults = await Promise.all(childCalls);
	    const results = [...initialResults];
    if (wantsPairAmountControl) {
      results.push(await frontdoorSafeSkill("run_case_sql", {
        case_id: caseId,
        purpose: [
          "当前案件两方往来金额定向核验。",
          "对手方排行只作定位线索，最终金额需核对明细、有效去重和重复风险。",
          "仅使用清洗/分析范围，组合安全事实键，不能把占位交易号单独当作唯一事实键。"
        ].join(" "),
        sql: pairAmountSql({ holderName, viaName }),
        row_limit: 5,
        result_mode: "aggregate",
        allowed_view_policy: "cleaned_and_analysis_only"
      }));
    }
    const sourceCounterpartyRanks = results.filter((item) =>
      item?.ok === true &&
      item.skill_id === "rank_counterparties" &&
      (!holderName || text(item.input?.holder_name) === holderName)
    );
    const sourceNameCounterpartyRanks = sourceCounterpartyRanks.filter((item) => text(item.input?.counterparty_group_mode) !== "account");
    const sourceCounterpartyAccountRank = sourceCounterpartyRanks.find((item) =>
      text(item.input?.counterparty_group_mode) === "account" &&
      !text(item.input?.date_start) &&
      !text(item.input?.date_end)
    );
    const sourceCounterpartyRank = sourceNameCounterpartyRanks.find((item) => !text(item.input?.date_start) && !text(item.input?.date_end))
      || sourceNameCounterpartyRanks[0]
      || sourceCounterpartyRanks[0];
    const sourceScopedCounterpartyRank = sourceNameCounterpartyRanks.find((item) => text(item.input?.date_start) || text(item.input?.date_end));
    const sourceToViaRankSummary = viaName
      ? counterpartySummaryFromRank(sourceCounterpartyRank, viaName, { holderName, scopeLabel: "full_period" })
      : {};
    const sourceToViaCoreAccountSummary = viaName
      ? counterpartyAccountSummaryFromRank(sourceCounterpartyAccountRank, viaName, { holderName, scopeLabel: "core_account" })
      : {};
    const sourceToViaScopedSummary = viaName && sourceScopedCounterpartyRank
      ? counterpartySummaryFromRank(sourceScopedCounterpartyRank, viaName, {
          holderName,
          dateStart: text(sourceScopedCounterpartyRank.input?.date_start),
          dateEnd: text(sourceScopedCounterpartyRank.input?.date_end),
          scopeLabel: "date_window"
        })
      : {};
    let viaCounterpartyRank = (wantsViaContinuation || wantsGraphContinuation) ? results.find((item) =>
      item?.ok === true &&
      item.skill_id === "rank_counterparties" &&
      viaName &&
      text(item.input?.holder_name) === viaName
    ) : null;
    let viaCounterpartyDateStart = text(args.date_start);
    if (intent === "destination" && viaName && (wantsViaContinuation || wantsGraphContinuation) && !viaCounterpartyRank) {
      viaCounterpartyDateStart = viaCounterpartyDateStart
        || text(objectOf(sourceToViaRankSummary.largest_date_cluster).date)
        || firstTxnDateForCounterparty(sourceCounterpartyRank, viaName);
      viaCounterpartyRank = await frontdoorSafeSkill("rank_counterparties", {
        case_id: caseId,
        holder_name: viaName,
        direction_mode: "out",
        metric: "outflow",
        success_filter: "all",
        cash_filter: "all",
        dedupe_same_holder_same_fact: true,
        counterparty_group_mode: "name",
        date_start: viaCounterpartyDateStart,
        date_end: text(args.date_end),
        limit: 8
      });
      results.push(viaCounterpartyRank);
    }
    const inlineFlowGraph = intent === "destination" && holderName && viaName && (wantsViaContinuation || wantsGraphContinuation)
      ? await buildFundFlowGraph({
          ...args,
          case_id: caseId,
          date_start: text(args.date_start) || viaCounterpartyDateStart,
          via_holder_name: viaName,
          top_n: clampInt(args.top_n, 20, 1, 50),
          include_debug: false
        }, { signal })
      : null;
    const holderAnalysis = results.find((item) => item.skill_id === "analyze_holder_full");
    const lab = results.find((item) => item.skill_id === "run_investigation_lab");
    const trace = results.find((item) => item.skill_id === "trace_subject_top_outflows");
    const counterpartyRank = sourceCounterpartyRank || results.find((item) => item.skill_id === "rank_counterparties");
    const frontdoorRankingRowLimit = clampInt(args.top_n, intent === "ranking" ? 10 : 8, 1, 50);
    let pairAmountWorkbench = wantsPairAmountControl
      ? results.find((item) => item.skill_id === "run_case_sql")
      : null;
    if (wantsPairAmountControl && pairAmountWorkbenchNeedsLocalFallback(pairAmountWorkbench)) {
      const fallbackPairAmountWorkbench = await runLocalPairAmountWorkbench({
        caseId,
        holderName,
        viaName,
        env,
        signal
      });
      const workbenchIndex = results.indexOf(pairAmountWorkbench);
      if (workbenchIndex >= 0) {
        results.splice(workbenchIndex, 1, fallbackPairAmountWorkbench);
      } else {
        results.push(fallbackPairAmountWorkbench);
      }
      pairAmountWorkbench = fallbackPairAmountWorkbench;
    }
    const rankingResults = results.filter((item) =>
      item?.ok === true
      && ["rank_accounts", "rank_holders", "rank_counterparties"].includes(item.skill_id)
    );
    const qaQuality = results.find((item) => item.skill_id === "audit_case_data_quality");
    const qaDuplicateFamilies = results.find((item) => item.skill_id === "resolve_duplicate_families");
    const pairSameFactReview = wantsPairAmountControl
      ? results.find((item) =>
          item.skill_id === "resolve_duplicate_families"
          && text(item.input?.scope_mode) === "same_holder_accounts"
          && text(item.input?.holder_name) === holderName
        )
      : null;
    const viaFinancialProbe = results.find((item) =>
      item.skill_id === "hypothesis_probe" &&
      viaName &&
      text(item.input?.holder_name) === viaName &&
      text(item.input?.probe_type) === "report_claim_review"
    );
	    const holderTopAccounts = intent === "holder_analysis"
	      ? results.find((item) => item.skill_id === "rank_accounts")
	      : null;
	    const accountAnalysis = intent === "account_analysis"
	      ? results.find((item) => item.skill_id === "analyze_account_full")
	      : null;
	    const holderAnalysisReady = intent !== "holder_analysis" || holderDossierFactsComplete(holderAnalysis, holderTopAccounts);
	    const holderTopAccountsReady = holderAnalysisReady;
	    const accountAnalysisReady = intent !== "account_analysis" || accountDossierFactsComplete(accountKeys[0], accountAnalysis);
	    const qaAnalysisReady = intent !== "qa_review" || qaReviewPayloadComplete(qaQuality, qaDuplicateFamilies);
	    const completeRankingResults = rankingResults.map((item) => ({
	      item,
	      rows: completeRankingRowsFromResult(item, frontdoorRankingRowLimit)
	    })).filter((entry) => entry.rows.length > 0);
	    const rankingAnalysisReady = intent !== "ranking" || completeRankingResults.length > 0;
	    const requiredChildWarnings = [];
	    if (!holderAnalysisReady || !holderTopAccountsReady) {
      requiredChildWarnings.push({
        code: "HOLDER_ANALYSIS_REQUIRED_FACTS_MISSING",
        severity: "blocking",
	        message: "主体分析必需的确定性子工具未返回完整事实；不得把缺失主体范围、金额或 Top 账户渲染为 0。"
	      });
	    }
	    if (!accountAnalysisReady) {
	      requiredChildWarnings.push({
	        code: "ACCOUNT_ANALYSIS_REQUIRED_FACTS_MISSING",
	        severity: "blocking",
	        message: "账户研判必需的确定性子工具未返回完整事实；不得把缺失账户统计或重点对手方渲染为 0。"
	      });
	    }
    if (!qaAnalysisReady) {
      requiredChildWarnings.push({
        code: "QA_REVIEW_REQUIRED_FACTS_MISSING",
        severity: "blocking",
        message: "数据质量、重复族或去重 policy 缺少完整字段；不得把空审计结果升级为已通过。"
      });
    }
    if (!rankingAnalysisReady) {
      requiredChildWarnings.push({
        code: "RANKING_REQUIRED_FACTS_MISSING",
        severity: "blocking",
        message: "排行结果缺少主体、笔数或金额字段；不得把空排行或零值行升级为事实就绪。"
      });
    }
    const plan = results.find((item) => item.skill_id === "plan_case_analysis");
    const casegraphFacts = objectOf(casegraph.key_facts);
    const declaredGates = explicitArray(casegraphFacts, "gates");
    const blockingGateCount = declaredGates === undefined
      ? undefined
      : declaredGates.filter((item) => text(item.status) !== "passed").length;
    const casegraphContextReady = !needsCasegraphContext || (
      text(casegraph.status) === "ok"
      && declaredGates !== undefined
      && declaredGates.length > 0
      && blockingGateCount === 0
      && coveragePayloadComplete(casegraphFacts.coverage)
    );
    if (!casegraphContextReady) {
      requiredChildWarnings.push({
        code: "CASEGRAPH_CONTEXT_INCOMPLETE",
        severity: "blocking",
        message: "案件图谱 coverage 或 mandatory gates 缺失/未通过；不得把空 gates 或缺失覆盖率升级为完整上下文。"
      });
    }
    const warnings = compactWarningList(casegraph.warnings, ...results.map((item) => item.warnings), inlineFlowGraph?.warnings, requiredChildWarnings);
    const hasBlockingWarning = warnings.some((item) => ["blocking", "error"].includes(text(item.severity)));
    const nextActions = buildFrontdoorNextActions({
      inlineFlowGraph,
      viaName,
      wantsFlowGraph,
      reportGradeIntent,
      caseId
    });
    const evidenceRefs = pruneEmpty({
      query_ids: [
        ...arrayOf(casegraph.evidence_refs?.query_ids),
        ...results.flatMap((item) => arrayOf(item.evidence_refs.query_ids))
      ].map(text).filter(Boolean).slice(0, 18),
      audit_ref: { frontdoor_id: `funds-investigate:${caseId}:${stableHash({ intent, holderName, focusKeywords })}` }
    }) || {};
    const destinationOutflowCard = intent === "destination" && holderName && !viaName
      ? buildDestinationOutflowDiagnosticCard({
          caseId,
          holderName,
          dateStart: text(args.date_start),
          dateEnd: text(args.date_end),
          topN: clampInt(args.top_n, 10, 1, 50),
          traceResult: trace,
          counterpartyRankResult: counterpartyRank,
          calls: [trace, counterpartyRank].filter(Boolean)
        })
      : null;
		    const pairAmountReviewCard = wantsPairAmountControl
		      ? buildPairAmountReviewCard({
		          caseId,
	          holderName,
	          viaName,
          pairAmountWorkbench,
          pairSameFactReview,
          sourceToViaSummary: sourceToViaRankSummary,
          sourceToViaCoreAccountSummary,
		          calls: [sourceCounterpartyRank, sourceCounterpartyAccountRank, pairSameFactReview, pairAmountWorkbench].filter(Boolean)
		        })
		      : null;
    const sourceToViaControlledSummary = pairAmountReviewCard
      ? sourceToViaSummaryFromPairAmountReviewCard(pairAmountReviewCard, sourceToViaRankSummary)
      : sourceToViaRankSummary;
    const viaOneHopCard = intent === "destination" && holderName && viaName && !wantsViaContinuation
      ? buildViaOneHopDiagnosticCard({
          caseId,
          holderName,
          viaName,
          dateStart: text(args.date_start),
          dateEnd: text(args.date_end),
          sourceToViaSummary: sourceToViaControlledSummary,
          sourceToViaScopedSummary,
          sourceToViaCoreAccountSummary,
          calls: [sourceCounterpartyRank, sourceCounterpartyAccountRank, sourceScopedCounterpartyRank].filter(Boolean)
        })
      : null;
    const viaContinuationCard = intent === "destination" && holderName && viaName && wantsViaContinuation
      ? buildViaContinuationDiagnosticCard({
          caseId,
          holderName,
          viaName,
          dateStart: text(args.date_start) || viaCounterpartyDateStart,
          dateEnd: text(args.date_end),
          sourceToViaSummary: sourceToViaControlledSummary,
          sourceToViaScopedSummary,
          sourceToViaCoreAccountSummary,
          viaCounterpartyRank,
          viaFinancialProbe,
          inlineFlowGraph,
          calls: [sourceCounterpartyRank, sourceCounterpartyAccountRank, sourceScopedCounterpartyRank, viaCounterpartyRank, viaFinancialProbe, inlineFlowGraph].filter(Boolean)
        })
      : null;
    const wantsGraphDeliveryCard = Boolean(inlineFlowGraph?.flow_graph)
      && (requestedGraph || /资金流向图|资金穿透图|来源去向图|去向图|流向图|关系图|导出.{0,8}图|画.{0,8}图/u.test(questionText));
    const flowGraphDeliveryCard = wantsGraphDeliveryCard
      ? buildFlowGraphDeliveryCard({
          caseId,
          holderName,
          viaName,
          questionText,
          inlineFlowGraph,
          sourceToViaSummary: sourceToViaControlledSummary,
          sourceToViaScopedSummary,
          sourceToViaCoreAccountSummary
        })
      : null;
		    const subjectDossierCard = intent === "holder_analysis"
		      ? buildSubjectDossierCard({
		          caseId,
		          holderName,
		          holderAnalysis,
		          holderTopAccounts
		        })
		      : null;
		    const accountDossierCard = intent === "account_analysis"
		      ? buildAccountDossierCard({
		          caseId,
		          accountKey: accountKeys[0],
		          accountAnalysis,
		          counterpartyRank
		        })
		      : null;
		    const qaReviewFacts = intent === "qa_review"
		      && qaAnalysisReady
		      ? compactQaReviewFacts(qaQuality.data, qaDuplicateFamilies.data)
		      : undefined;
		    const qaRequiredFactsPresent = intent !== "qa_review" || Boolean(qaAnalysisReady && qaReviewFacts);
		    const holderRequiredFactsPresent = intent !== "holder_analysis" || holderAnalysisReady;
		    const accountRequiredFactsPresent = intent !== "account_analysis" || accountAnalysisReady;
		    const rankingRequiredFactsPresent = intent !== "ranking" || rankingAnalysisReady;
		    const destinationCards = [
		      wantsPairAmountReview ? pairAmountReviewCard : null,
		      flowGraphDeliveryCard,
		      destinationOutflowCard,
		      viaOneHopCard,
		      viaContinuationCard
		    ].filter(Boolean);
		    const destinationRequiredFactsPresent = intent !== "destination"
		      || destinationCards.some((card) => card.required_facts_present === true);
		    const riskDiscoveryRequiredFactsPresent = intent !== "risk_discovery"
		      || (lab?.ok === true && (
		        arrayOf(lab.data?.mandatory_review_cards).length > 0
		        || arrayOf(lab.data?.hypothesis_cards).length > 0
		      ));
		    const fallbackRequiredFactsPresent = !hasBlockingWarning
		      && qaRequiredFactsPresent
		      && holderRequiredFactsPresent
		      && accountRequiredFactsPresent
		      && rankingRequiredFactsPresent
		      && destinationRequiredFactsPresent
		      && riskDiscoveryRequiredFactsPresent
		      && casegraphContextReady;
		    const answerCard = (wantsPairAmountReview ? pairAmountReviewCard : null) || flowGraphDeliveryCard || destinationOutflowCard || viaOneHopCard || viaContinuationCard || subjectDossierCard || accountDossierCard || withAnswerCardProtocol({
      title: "Analytix 资金研判前门",
      summary: "已把模糊自然语言问题路由为轻量核验材料；模型应直接回答本轮事实、统计范围和边界，不要输出内部查询编号，也不要自行合计分页流水。新 continuation/rescope/hypothesis 可使用语义事实工具。",
      intent,
      requested_intent: requestedIntent,
      case_name: text(resolved.case?.case_name || resolved.case?.name),
      holder_name: holderName,
      via_holder_name: viaName,
      intent_ast: intentAst,
      plan_dag: planDag,
      tool_budget: frontdoorToolBudget,
      ...(blockingGateCount === undefined ? {} : { blocking_gate_count: blockingGateCount }),
      casegraph_context_ready: casegraphContextReady,
      answer_ready: fallbackRequiredFactsPresent,
      report_grade_blocked: reportGradeIntent,
      write_blocked: reportGradeIntent,
      mandatory_review_cards_status: reportGradeIntent ? "required_before_formal_report" : undefined,
      report_gate_status: reportGradeIntent
        ? "formal_report_blocked_until_source_audit_data_quality_mandatory_cards_and_validate_report_claims_all_pass"
        : undefined,
      autonomy_boundary: "Codex 自由提出侦查假设；插件只负责事实取数、边界和校验。"
	    }, {
	      recommendedNextAction: reportGradeIntent ? "answer_now_then_validate_report_claims_only_if_formal_report_text_is_required" : "answer_now",
	      maxAdditionalTools: clampInt(frontdoorToolBudget.max_additional_tools, reportGradeIntent ? 1 : 0, 0, 3),
	      requiredFactsPresent: fallbackRequiredFactsPresent,
	      unsupportedFlowsPresent: wantsFlowGraph && !inlineFlowGraph?.flow_graph
	    });
    const responseStatus = hasBlockingWarning || answerCard.required_facts_present !== true ? "partial" : "ok";
    const holderAnalysisData = holderAnalysisReady ? objectOf(holderAnalysis?.data) : {};
    const holderAccountAnalysis = objectOf(holderAnalysisData.account_analysis);
    const holderCoverageSource = objectOf(holderAccountAnalysis.coverage);
    const holderStats = compactAccountStatsFact(holderAccountAnalysis.account_stats || {});
    const accountAnalysisData = accountAnalysisReady ? objectOf(accountAnalysis?.data) : {};
    const accountAnalysisDetail = objectOf(accountAnalysisData.account_analysis || accountAnalysisData);
    const accountCoverageSource = objectOf(accountAnalysisDetail.coverage || accountAnalysisData.coverage);
    const accountStats = compactAccountStatsFact(accountAnalysisDetail.account_stats || accountAnalysisData.account_stats || {});
    const validatedFlowEdges = completeSupportedFlowEdges(inlineFlowGraph);
    const graphFactReady = validatedFlowEdges.length > 0;
    const casegraphReady = text(casegraph.status) === "ok";
    const casegraphCoverage = objectOf(casegraphFacts.coverage);
    const counterpartyFactRows = completeRankingRowsFromResult(counterpartyRank, frontdoorRankingRowLimit);
    const viaCounterpartyFactRows = completeRankingRowsFromResult(viaCounterpartyRank, frontdoorRankingRowLimit);
    const inlineSourceToViaSummary = graphFactReady
      ? objectOf(inlineFlowGraph.key_facts?.source_to_via_summary)
      : {};
    const scopedPairSummary = mergePairSummaryAccounts(sourceToViaScopedSummary, inlineSourceToViaSummary);
    const controlledPairSummary = mergePairSummaryAccounts(sourceToViaControlledSummary, inlineSourceToViaSummary);
    const coreAccountPairSummary = pairSummaryFactComplete(sourceToViaCoreAccountSummary)
      ? sourceToViaCoreAccountSummary
      : undefined;
    const response = {
      tool: "funds_investigate",
      status: responseStatus,
      case_id: caseId,
      answer_card: answerCard,
      key_facts: {
        casegraph: {
          coverage: casegraphReady && coveragePayloadComplete(casegraphCoverage) ? compactCoverageFact(casegraphCoverage) : undefined,
          gates: casegraphReady ? declaredGates : undefined,
          top_accounts: casegraphReady
            ? compactRankingRows(casegraphFacts.top_accounts, 3).filter(rankingFactRowComplete)
            : undefined,
          top_counterparties: casegraphReady
            ? compactRankingRows(casegraphFacts.top_counterparties, 3).filter(rankingFactRowComplete)
            : undefined
        },
	        holder_scope: holderAnalysisReady
	          ? compactOwnerScope(holderAnalysisData.holder_scope || holderAnalysisData.owner_scope || holderAnalysisData.subject_scope || {})
	          : undefined,
	        holder_coverage: holderAnalysisReady && coveragePayloadComplete(holderCoverageSource)
	          ? compactCoverageFact(holderCoverageSource)
	          : undefined,
	        holder_account_stats: holderAnalysisReady && accountStatsFactComplete(holderStats) ? holderStats : undefined,
	        holder_top_accounts: holderAnalysisReady ? completeRankingRowsFromResult(holderTopAccounts, 8) : undefined,
	        account_key: accountKeys[0],
	        account_coverage: accountAnalysisReady && coveragePayloadComplete(accountCoverageSource)
	          ? compactCoverageFact(accountCoverageSource)
	          : undefined,
	        account_stats: accountAnalysisReady && accountStatsFactComplete(accountStats) ? accountStats : undefined,
	        rankings: completeRankingResults.length
          ? completeRankingResults.map(({ item, rows }) => ({
              tool: item.skill_id,
              target: item.skill_id.replace(/^rank_/u, ""),
              metric: text(item.input?.metric) || rankingMetric,
              direction_mode: text(item.input?.direction_mode) || "both",
              success_filter: "all",
              rows
            }))
          : undefined,
	        counterparty_rankings: counterpartyFactRows.length ? counterpartyFactRows : undefined,
        via_counterparty_rankings: viaCounterpartyFactRows.length ? viaCounterpartyFactRows : undefined,
        via_counterparty_date_start: viaCounterpartyDateStart,
        pair_amount_review: pairAmountReviewCard
          ? pruneEmpty({
              validation_state: pairAmountReviewCard.validation_state,
              delivery_state: pairAmountReviewCard.delivery_state,
              first_txn_at: pairAmountReviewCard.first_txn_at,
              last_txn_at: pairAmountReviewCard.last_txn_at,
              statistical_period: pairAmountReviewCard.statistical_period,
              raw_detail_amount: pairAmountReviewCard.raw_detail_amount,
              raw_detail_count: pairAmountReviewCard.raw_detail_count,
              high_confidence_same_fact_effective_amount: pairAmountReviewCard.high_confidence_same_fact_effective_amount,
              high_confidence_same_fact_count: pairAmountReviewCard.high_confidence_same_fact_count,
              full_period_raw_amount: pairAmountReviewCard.full_period_raw_amount,
              full_period_raw_count: pairAmountReviewCard.full_period_raw_count,
              full_period_effective_amount: pairAmountReviewCard.full_period_effective_amount,
              full_period_effective_count: pairAmountReviewCard.full_period_effective_count,
              full_period_first_txn_at: pairAmountReviewCard.full_period_first_txn_at,
              full_period_last_txn_at: pairAmountReviewCard.full_period_last_txn_at,
              full_period_effective_first_txn_at: pairAmountReviewCard.full_period_effective_first_txn_at,
              full_period_effective_last_txn_at: pairAmountReviewCard.full_period_effective_last_txn_at,
              principal_amount_after_fee_exclusion: pairAmountReviewCard.principal_amount_after_fee_exclusion,
              duplicate_or_unsupported_amount: pairAmountReviewCard.duplicate_or_unsupported_amount,
              confirmed_same_fact_duplicate_amount: pairAmountReviewCard.confirmed_same_fact_duplicate_amount,
              confirmed_same_fact_duplicate_count: pairAmountReviewCard.confirmed_same_fact_duplicate_count,
              unresolved_duplicate_or_unsupported_amount: pairAmountReviewCard.unresolved_duplicate_or_unsupported_amount,
              duplicate_review_status: pairAmountReviewCard.duplicate_review_status,
              duplicate_review_explanation: pairAmountReviewCard.duplicate_review_explanation,
              rank_counterparties_use: pairAmountReviewCard.rank_counterparties_use,
              detail_rank_difference_amount: pairAmountReviewCard.detail_rank_difference_amount,
              focus_cluster: pairAmountReviewCard.focus_cluster,
              focus_cluster_role: pairAmountReviewCard.focus_cluster_role,
              focus_cluster_effective_share: pairAmountReviewCard.focus_cluster_effective_share,
              outside_focus_cluster_effective_amount: pairAmountReviewCard.outside_focus_cluster_effective_amount,
              outside_focus_cluster_effective_count: pairAmountReviewCard.outside_focus_cluster_effective_count,
              source_account_count: pairAmountReviewCard.source_account_count,
              source_accounts: hasOwn(pairAmountReviewCard, "source_accounts")
                ? arrayOf(pairAmountReviewCard.source_accounts).slice(0, 20)
                : undefined,
              counterparty_account_count: pairAmountReviewCard.counterparty_account_count,
              counterparty_accounts: hasOwn(pairAmountReviewCard, "counterparty_accounts")
                ? arrayOf(pairAmountReviewCard.counterparty_accounts).slice(0, 20)
                : undefined,
              account_clusters: hasOwn(pairAmountReviewCard, "account_clusters")
                ? arrayOf(pairAmountReviewCard.account_clusters).slice(0, 3)
                : undefined,
              time_clusters: hasOwn(pairAmountReviewCard, "time_clusters")
                ? arrayOf(pairAmountReviewCard.time_clusters).slice(0, 3)
                : undefined,
              top_transactions: hasOwn(pairAmountReviewCard, "top_transactions")
                ? arrayOf(pairAmountReviewCard.top_transactions).slice(0, 20)
                : undefined,
              raw_transaction_candidates: hasOwn(pairAmountReviewCard, "raw_transaction_candidates")
                ? arrayOf(pairAmountReviewCard.raw_transaction_candidates).slice(0, 20)
                : undefined,
              focus_cluster_transactions: hasOwn(pairAmountReviewCard, "focus_cluster_transactions")
                ? arrayOf(pairAmountReviewCard.focus_cluster_transactions).slice(0, 20)
                : undefined,
              expanded_same_name_scope: pairAmountReviewCard.expanded_same_name_scope
            })
          : undefined,
        via_financial_product_clues: viaFinancialProbe?.ok === true
          ? compactFinancialProductLeads(viaFinancialProbe.data, 6)
          : undefined,
        top_outflows: !viaName && trace?.ok === true && Array.isArray(trace.data?.top_outflows)
          ? compactTopOutflowRows(trace.data.top_outflows, frontdoorRankingRowLimit)
          : undefined,
        flow_graph: graphFactReady ? compactFlowGraphForText({ edges: validatedFlowEdges }, 5) : undefined,
        flow_graph_answer: graphFactReady ? flowGraphDeliveryCard : undefined,
        source_to_via_date_window_summary: scopedPairSummary,
        source_to_via_summary: controlledPairSummary,
        source_to_via_core_account_summary: coreAccountPairSummary,
        flow_graph_scope_stats: graphFactReady && inlineFlowGraph?.key_facts
          ? pruneEmpty({
              source_scope_stats: inlineFlowGraph.key_facts.source_scope_stats,
              via_scope_stats: inlineFlowGraph.key_facts.via_scope_stats,
              source_seed_count: optionalCountValue(inlineFlowGraph.key_facts.source_seed_count),
              downstream_seed_count: optionalCountValue(inlineFlowGraph.key_facts.downstream_seed_count),
              source_to_via_date_window_summary: scopedPairSummary,
              source_to_via_summary: controlledPairSummary,
              source_to_via_core_account_summary: coreAccountPairSummary
            })
          : undefined,
        mandatory_review_cards: lab?.ok === true && Array.isArray(lab.data?.mandatory_review_cards)
          ? compactHypothesisCards(lab.data.mandatory_review_cards, maxCards)
          : undefined,
        hypothesis_cards: lab?.ok === true && Array.isArray(lab.data?.hypothesis_cards)
          ? compactHypothesisCards(lab.data.hypothesis_cards, maxCards)
          : undefined,
        qa_review: qaReviewFacts,
        plan_lanes: plan?.ok === true && Array.isArray(plan.data?.lanes)
          ? plan.data.lanes.map((lane) => ({
              lane_id: text(lane.lane_id),
              title: text(lane.title),
              allowed_tools: explicitArray(objectOf(lane), "allowed_tools")?.slice(0, 6)
            })).slice(0, 6)
          : undefined
      },
      warnings,
      evidence_refs: evidenceRefs,
      next_actions: nextActions,
      answer_contract: buildFrontdoorAnswerContract()
    };
    return response;
  }

  return { fundsInvestigate };
}
