#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { pairAmountSql } from "../mcp/frontdoor-runtime.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function countValue(value, label) {
  const raw = String(value ?? "").trim();
  assert(/^\d+$/u.test(raw), `${label} must be a canonical non-negative integer`);
  return BigInt(raw);
}

function moneyCents(value, label) {
  const raw = String(value ?? "").trim();
  assert(/^-?\d+(?:\.\d+)?$/u.test(raw), `${label} must be a canonical decimal`);
  const negative = raw.startsWith("-");
  const unsigned = negative ? raw.slice(1) : raw;
  const [whole, fraction = ""] = unsigned.split(".");
  let cents = (BigInt(whole) * 100n) + BigInt(fraction.slice(0, 2).padEnd(2, "0"));
  if (fraction.length > 2 && fraction[2] >= "5") cents += 1n;
  return negative ? -cents : cents;
}

const configuredCaseDb = process.env.ANALYTIX_PAIR_AMOUNT_DEDUP_CASE_DB
  || process.env.ANALYTIX_FUNDS_CASE_DB
  || process.env.ANALYTIX_CASE_DB;
assert(configuredCaseDb, "set ANALYTIX_PAIR_AMOUNT_DEDUP_CASE_DB, ANALYTIX_FUNDS_CASE_DB, or ANALYTIX_CASE_DB");
const caseDb = path.resolve(configuredCaseDb);

assert(fs.existsSync(caseDb), `case database not found: ${caseDb}`);

const python = String.raw`
import duckdb
import json
import sys

payload = json.load(sys.stdin)
con = duckdb.connect(payload["case_db"], read_only=True)
row = con.execute(payload["sql"]).fetchone()
cols = [item[0] for item in con.description]
print(json.dumps(None if row is None else dict(zip(cols, row)), ensure_ascii=False, default=str))
`;

function queryFirstRecord(sql) {
  const result = spawnSync("python3", ["-c", python], {
    input: JSON.stringify({ case_db: caseDb, sql }),
    encoding: "utf8",
    maxBuffer: 1024 * 1024 * 20
  });
  if (result.status !== 0) {
    process.stderr.write(result.stderr || result.stdout);
    process.exit(result.status || 1);
  }
  const record = JSON.parse(result.stdout);
  assert(record && typeof record === "object" && !Array.isArray(record), "query must return one record");
  return record;
}

const candidateSql = `WITH base AS (
  SELECT
    account_open_name AS holder_name,
    coalesce(cp_name, cp_name_pick, stats_name_key, counterparty_name, '') AS via_name,
    coalesce(nullif(trim(cast(card_no AS varchar)), ''), nullif(trim(cast(acct_no AS varchar)), ''), nullif(trim(cast(acct_key AS varchar)), ''), '') AS account_key_norm,
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
      CASE
        WHEN txn_id IS NOT NULL
          AND length(trim(cast(txn_id AS varchar))) > 0
          AND lower(trim(cast(txn_id AS varchar))) NOT IN ('查无信息','无','未知','unknown','__unknown__','null','none','na','n/a','-','--')
        THEN trim(cast(txn_id AS varchar))
        ELSE ''
      END,
      coalesce(summary, ''),
      coalesce(remark, ''),
      coalesce(txn_type, ''),
      coalesce(cast(is_success AS varchar), '')
    ) AS same_holder_cross_account_fact_key
  FROM analysis_txn_detail_idx
  WHERE account_open_name IS NOT NULL
    AND account_open_name <> ''
    AND dc_val = '出'
    AND amount > 0
    AND coalesce(cp_name, cp_name_pick, stats_name_key, counterparty_name, '') <> ''
), duplicate_groups AS (
  SELECT
    holder_name,
    via_name,
    same_holder_cross_account_fact_key,
    count(*) AS raw_count,
    count(DISTINCT account_key_norm) AS account_count
  FROM base
  WHERE account_key_norm <> ''
  GROUP BY holder_name, via_name, same_holder_cross_account_fact_key
  HAVING count(*) > 1 AND count(DISTINCT account_key_norm) > 1
), candidates AS (
  SELECT
    holder_name,
    via_name,
    count(*) AS duplicate_group_count,
    sum(raw_count - 1) AS duplicate_extra_count
  FROM duplicate_groups
  GROUP BY holder_name, via_name
)
SELECT holder_name, via_name
FROM candidates
ORDER BY duplicate_group_count DESC, duplicate_extra_count DESC, holder_name, via_name
LIMIT 1`;

const configuredHolderName = String(process.env.ANALYTIX_PAIR_AMOUNT_DEDUP_HOLDER || "").trim();
const configuredViaName = String(process.env.ANALYTIX_PAIR_AMOUNT_DEDUP_VIA || "").trim();
assert(Boolean(configuredHolderName) === Boolean(configuredViaName), "holder and via overrides must be provided together");
const candidate = configuredHolderName
  ? { holder_name: configuredHolderName, via_name: configuredViaName }
  : queryFirstRecord(candidateSql);
const holderName = String(candidate.holder_name || "").trim();
const viaName = String(candidate.via_name || "").trim();
assert(holderName && viaName, "frozen snapshot has no cross-account repeated-fact candidate");

const record = queryFirstRecord(pairAmountSql({ holderName, viaName }));
const rawDetailCount = countValue(record.raw_detail_count, "raw_detail_count");
const effectiveFactCount = countValue(record.effective_fact_count, "effective_fact_count");
const rawDetailAmount = moneyCents(record.raw_detail_amount, "raw_detail_amount");
const effectiveDedupAmount = moneyCents(record.effective_dedup_amount, "effective_dedup_amount");
const duplicateOrUnsupportedAmount = moneyCents(record.duplicate_or_unsupported_amount, "duplicate_or_unsupported_amount");
const confirmedDuplicateAmount = moneyCents(record.confirmed_same_fact_duplicate_amount, "confirmed_same_fact_duplicate_amount");
const confirmedDuplicateExtraCount = countValue(record.confirmed_same_fact_duplicate_extra_count, "confirmed_same_fact_duplicate_extra_count");

assert(rawDetailCount > effectiveFactCount, "a selected duplicate candidate must reduce the effective fact count");
assert(rawDetailAmount > effectiveDedupAmount, "a selected duplicate candidate must reduce the effective amount");
assert(rawDetailAmount - effectiveDedupAmount === duplicateOrUnsupportedAmount, "raw/effective amounts must conserve exactly to cents");
assert(confirmedDuplicateAmount > 0n, "selected candidate must contain a confirmed cross-account duplicate amount");
assert(confirmedDuplicateAmount <= duplicateOrUnsupportedAmount, "confirmed duplicate amount cannot exceed the total raw/effective difference");
assert(confirmedDuplicateExtraCount > 0n, "selected candidate must contain confirmed duplicate extra rows");

const duplicateGroups = Array.isArray(record.duplicate_groups) ? record.duplicate_groups : [];
const confirmedGroups = duplicateGroups.filter((group) => group?.dedupe_basis === "confirmed_same_holder_cross_account_same_fact");
assert(confirmedGroups.length > 0, "selected candidate must expose confirmed repeated-record groups");
let groupDuplicateAmount = 0n;
let groupDuplicateExtraCount = 0n;
for (const [index, group] of confirmedGroups.entries()) {
  const accountCount = countValue(group.account_count, `duplicate_groups[${index}].account_count`);
  const rawCount = countValue(group.raw_count, `duplicate_groups[${index}].raw_count`);
  const duplicateAmount = moneyCents(group.duplicate_amount, `duplicate_groups[${index}].duplicate_amount`);
  assert(accountCount > 1n, "each confirmed duplicate group must span multiple payer accounts");
  assert(rawCount > 1n, "each confirmed duplicate group must contain repeated rows");
  assert(duplicateAmount > 0n, "each confirmed duplicate group must remove a positive duplicate amount");
  groupDuplicateAmount += duplicateAmount;
  groupDuplicateExtraCount += rawCount - 1n;
}
assert(groupDuplicateAmount === confirmedDuplicateAmount, "confirmed group amounts must equal the aggregate confirmed duplicate amount");
assert(groupDuplicateExtraCount === confirmedDuplicateExtraCount, "confirmed group extra rows must equal the aggregate confirmed duplicate count");

console.log(JSON.stringify({
  ok: true,
  candidate_source: configuredHolderName ? "explicit_environment" : "deterministic_frozen_snapshot",
  exact_money_conservation: true,
  effective_count_reduced: true,
  effective_amount_reduced: true,
  confirmed_cross_account_groups_present: true,
  pii_emitted: false
}, null, 2));
