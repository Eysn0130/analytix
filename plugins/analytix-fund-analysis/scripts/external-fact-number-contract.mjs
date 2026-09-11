#!/usr/bin/env node

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import {
  clampInt,
  intOrUndefined,
  intValue,
  numberOrUndefined,
  sumNumberRows
} from "../mcp/runtime-normalizers.mjs";
import { renderDiagnosticCardForAgent } from "../mcp/card-renderer.mjs";
import { analyzeReportClaimText } from "../mcp/claim-verifier-protocol.mjs";
import {
  counterpartyAccountSummaryFromRank,
  counterpartySummaryFromRank,
  duplicateCandidateRiskText,
  moneyText
} from "../mcp/frontdoor-fact-summaries.mjs";
import { createDestinationDiagnosticRuntime } from "../mcp/destination-diagnostic-runtime.mjs";

const invalidNumbers = [
  ["undefined", undefined],
  ["null", null],
  ["empty", ""],
  ["blank", " \t\n"],
  ["false", false],
  ["true", true],
  ["array", []],
  ["array-zero", [0]],
  ["object", {}],
  ["boxed-number", new Number(0)],
  ["nan", Number.NaN],
  ["positive-infinity", Number.POSITIVE_INFINITY],
  ["negative-infinity", Number.NEGATIVE_INFINITY],
  ["nan-string", "NaN"],
  ["infinity-string", "Infinity"],
  ["hex-string", "0x10"],
  ["comma-string", "1,000"]
];

for (const [label, value] of invalidNumbers) {
  assert.equal(numberOrUndefined(value), undefined, `${label} must not become an optional number`);
  assert.equal(intOrUndefined(value), undefined, `${label} must not become an optional integer`);
  assert.equal(intValue(value), undefined, `${label} must not become an integer through the internal helper`);
}

for (const [value, expected] of [
  [0, 0],
  [-0, -0],
  [1.25, 1.25],
  [-2.5, -2.5],
  ["0", 0],
  [" 0 ", 0],
  ["1.25", 1.25],
  ["-2.5", -2.5],
  ["1e3", 1000]
]) {
  assert.equal(numberOrUndefined(value), expected, `${String(value)} must remain an explicit number`);
}
assert.equal(intOrUndefined(0), 0);
assert.equal(intOrUndefined("0"), 0);
assert.equal(intOrUndefined(2), 2);
assert.equal(intOrUndefined("-2"), -2);
assert.equal(intOrUndefined(1.5), undefined, "fractional number must not be truncated into an optional integer");
assert.equal(intOrUndefined("1.5"), undefined, "fractional string must not be truncated into an optional integer");

// Internal limit/counter defaults remain deliberately separate from optional external facts.
assert.equal(intValue(undefined, 7), 7);
assert.equal(clampInt(undefined, 10, 1, 200), 10);
assert.equal(clampInt(999, 10, 1, 200), 200);

assert.equal(sumNumberRows([], "amount"), undefined, "an empty aggregation is not numeric zero");
assert.equal(sumNumberRows([{ amount: 0 }, { amount: "0" }], "amount"), 0, "explicit zero rows remain zero");
assert.equal(sumNumberRows([{ amount: 1.25 }, { amount: "2.75" }], "amount"), 4);
for (const invalid of [undefined, null, "", " ", false, true, [], [1], {}, "0x10", "1,000"]) {
  assert.equal(
    sumNumberRows([{ amount: 10 }, { amount: invalid }], "amount"),
    undefined,
    `aggregation containing ${JSON.stringify(invalid)} must stay unresolved`,
  );
}

function rankResult(row, { dedupe = false, dedupeReadiness } = {}) {
  return {
    input: { dedupe_same_holder_same_fact: dedupe },
    data: {
      coverage_status: "complete",
      filters: { dedupe_same_holder_same_fact: dedupe },
      same_fact_dedupe: dedupeReadiness,
      rankings: [{ display_name: "乙方", ...row }]
    }
  };
}

function assertNoFactUpgrade(value, label) {
  const body = JSON.stringify(value);
  assert.equal(body.includes('"amount":0'), false, `${label} must not synthesize amount zero`);
  assert.equal(body.includes('"txn_count":0'), false, `${label} must not synthesize count zero`);
  assert.equal(/"(?:support_status|fact_status)":"supported"|verified[_ -]?no[_ -]?hit|"no_hit"/iu.test(body), false, `${label} must not publish supported/no-hit state`);
}

for (const [label, value] of invalidNumbers) {
  const invalidCount = rankResult({ txn_count: value, outflow: { yuan: 100 } });
  const invalidAmount = rankResult({ txn_count: 2, outflow: { yuan: value } });
  for (const [kind, result] of [
    ["counterparty-count", counterpartySummaryFromRank(invalidCount, "乙方")],
    ["counterparty-amount", counterpartySummaryFromRank(invalidAmount, "乙方")],
    ["account-count", counterpartyAccountSummaryFromRank(invalidCount, "乙方")],
    ["account-amount", counterpartyAccountSummaryFromRank(invalidAmount, "乙方")]
  ]) {
    assert.deepEqual(result, {}, `${kind}/${label} must remain an absent fact, not an unresolved object that downstream can mark supported`);
    assertNoFactUpgrade(result, `${kind}/${label}`);
  }
}

const explicitZero = counterpartySummaryFromRank(rankResult({
  txn_count: 0,
  outflow: { yuan: 0 }
}), "乙方");
assert.equal(explicitZero.txn_count, 0);
assert.equal(explicitZero.amount, 0);
const strictStringZero = counterpartyAccountSummaryFromRank(rankResult({
  txn_count: "0",
  outflow: { yuan: "0" },
  counterparty_account: "acct-zero"
}), "乙方");
assert.equal(strictStringZero.txn_count, 0);
assert.equal(strictStringZero.amount, 0);
assert.deepEqual(strictStringZero.counterparty_accounts, ["acct-zero"]);

const optionalMalice = counterpartySummaryFromRank(rankResult({
  txn_count: 2,
  outflow: { yuan: 100 },
  source_account_count: false,
  duplicate_row_count: [],
  duplicate_amount: {},
  largest_date_cluster: {
    date: "2026-07-10",
    txn_count: true,
    amount: [0]
  }
}), "乙方");
assert.equal(optionalMalice.amount, 100);
assert.equal(optionalMalice.txn_count, 2);
for (const key of ["source_account_count", "duplicate_row_count", "duplicate_amount"]) {
  assert.equal(Object.prototype.hasOwnProperty.call(optionalMalice, key), false, `${key} must not be synthesized from a malicious type`);
}
assert.equal(Object.prototype.hasOwnProperty.call(optionalMalice.largest_date_cluster, "txn_count"), false);
assert.equal(Object.prototype.hasOwnProperty.call(optionalMalice.largest_date_cluster, "amount"), false);
assertNoFactUpgrade(optionalMalice, "optional malicious fields");

const invalidTruncation = counterpartySummaryFromRank(rankResult({
  txn_count: 2,
  outflow: { yuan: 100 },
  source_accounts: ["acct-a"],
  source_accounts_truncated: "false"
}), "乙方");
assert.equal(
  Object.prototype.hasOwnProperty.call(invalidTruncation, "source_accounts_truncated"),
  false,
  "string false must not become a true truncation fact"
);
const explicitFalseTruncation = counterpartySummaryFromRank(rankResult({
  txn_count: 2,
  outflow: { yuan: 100 },
  source_accounts: ["acct-a"],
  source_accounts_truncated: false
}), "乙方");
assert.equal(explicitFalseTruncation.source_accounts_truncated, false);

const completeDedupeReadiness = {
  contract: "SameFactDedupeCoverageV1",
  key_version: "analytix.same-fact-dedupe-key/v2",
  requested: true,
  scope_authorized: true,
  applied: true,
  coverage_status: "complete",
  blocker: null
};
assert.equal(
  counterpartySummaryFromRank(
    rankResult({ txn_count: 2, outflow: { yuan: 100 } }, {
      dedupe: true,
      dedupeReadiness: completeDedupeReadiness
    }),
    "乙方"
  ).amount,
  100
);
for (const [label, rank] of [
  ["partial-ranking", { data: { coverage_status: "partial", rankings: [{ display_name: "乙方", txn_count: 2, outflow: { yuan: 100 } }] } }],
  ["missing-dedupe-readiness", rankResult({ txn_count: 2, outflow: { yuan: 100 } }, { dedupe: true })],
  ["unapplied-dedupe", rankResult({ txn_count: 2, outflow: { yuan: 100 } }, {
    dedupe: true,
    dedupeReadiness: { ...completeDedupeReadiness, applied: false, coverage_status: "partial", blocker: "same_fact_dedupe_key_coverage_incomplete" }
  })],
  ["dedupe-signal-mismatch", {
    input: { dedupe_same_holder_same_fact: true },
    data: {
      coverage_status: "complete",
      filters: { dedupe_same_holder_same_fact: false },
      same_fact_dedupe: completeDedupeReadiness,
      rankings: [{ display_name: "乙方", txn_count: 2, outflow: { yuan: 100 } }]
    }
  }]
]) {
  const summary = counterpartySummaryFromRank(rank, "乙方");
  assert.deepEqual(summary, {}, `${label} must not emit a counterparty fact summary`);
  assertNoFactUpgrade(summary, label);
}

assert.equal(moneyText(0), "0.00 元");
assert.equal(moneyText("0"), "0.00 元");
assert.equal(moneyText({ yuan: 0, text: "9,999,999.00 元" }), "0.00 元", "numeric value must be authoritative over untrusted display text");
for (const value of [undefined, null, "", " ", false, true, [], [0], {}, { text: "0.00 元" }, "not-a-number"]) {
  assert.equal(moneyText(value), "", "invalid money input must not render an amount");
}

for (const value of [undefined, null, "", " ", false, true, [], [0], {}, Number.NaN, Number.POSITIVE_INFINITY]) {
  const riskText = duplicateCandidateRiskText({
    same_holder_group_count: value,
    same_holder_extra_rows: value,
    same_holder_candidate_duplicate_amount: value
  });
  assert.match(riskText, /未返回|不得补成 0/u);
  assert.doesNotMatch(riskText, /显式返回同事实候选计数为 0/u);
}
assert.match(duplicateCandidateRiskText({
  same_holder_group_count: 0,
  same_holder_extra_rows: "0",
  same_holder_candidate_duplicate_amount: 0
}), /显式返回同事实候选计数为 0/u);

const phase4DiagnosticsSource = readFileSync(
  new URL("./phase4-diagnostics.mjs", import.meta.url),
  "utf8"
);
assert.match(
  phase4DiagnosticsSource,
  /duplicateCandidateRiskText as runtimeDuplicateCandidateRiskText/u,
  "phase4 diagnostics must reuse the tested restrained duplicate-risk renderer"
);
assert.match(
  phase4DiagnosticsSource,
  /import \{ intOrUndefined \} from "\.\.\/mcp\/runtime-normalizers\.mjs";/u,
  "phase4 diagnostics must reuse the strict integer normalizer"
);
assert.doesNotMatch(
  phase4DiagnosticsSource,
  /numberValue\([^\n]+\)\s*\|\|\s*0/gu,
  "phase4 diagnostics must not turn a missing external amount into zero"
);
assert.doesNotMatch(
  phase4DiagnosticsSource,
  /Number\(value \?\? 0\)/u,
  "phase4 diagnostics must not turn a missing external count into zero"
);
assert.doesNotMatch(
  phase4DiagnosticsSource,
  /\$\{amountValue \|\| ["']未知["']\}/u,
  "phase4 diagnostics must preserve an observed zero count"
);

const destinationRuntime = createDestinationDiagnosticRuntime({ moneyText });
const missingViaCard = destinationRuntime.buildViaOneHopDiagnosticCard({
  caseId: "case-missing",
  holderName: "甲方",
  viaName: "乙方",
  sourceToViaSummary: {},
  sourceToViaScopedSummary: {},
  sourceToViaCoreAccountSummary: {}
});
const missingDuplicateGuard = missingViaCard.facts.find((fact) => fact.fact_id === "via_one_hop.duplicate_guard");
assert.equal(missingViaCard.required_facts_present, false);
assert.equal(
  missingDuplicateGuard.support_status,
  "unsupported",
  "a missing duplicate guard must keep the evidence-ledger default instead of being upgraded to a fact state",
);
assert.equal(Object.hasOwn(missingDuplicateGuard, "amount"), false, "empty duplicate guard must not synthesize amount zero");

const partialViaCard = destinationRuntime.buildViaOneHopDiagnosticCard({
  caseId: "case-partial",
  holderName: "甲方",
  viaName: "乙方",
  sourceToViaSummary: {
    amount: 100,
    first_txn_at: "2026-01-01",
    last_txn_at: "2026-01-02",
    largest_date_cluster: { date: "2026-01-01", amount: 100 }
  },
  sourceToViaScopedSummary: {},
  sourceToViaCoreAccountSummary: {}
});
const partialFullPeriod = partialViaCard.facts.find((fact) => fact.fact_id === "via_one_hop.full_period");
assert.equal(partialViaCard.required_facts_present, false);
assert.equal(
  partialFullPeriod.support_status,
  "unsupported",
  "a partial diagnostic must remain fact-ineligible until the host evidence registry validates it",
);
assert.equal(Object.hasOwn(partialFullPeriod, "amount"), false);
assert.equal(Object.hasOwn(partialFullPeriod, "count"), false);
assert.equal(Object.hasOwn(partialFullPeriod, "answer_text"), false);

for (const invalid of [true, false, [], {}, "0x10", "1,000"]) {
  for (const invalidField of ["amount", "txn_count"]) {
    const summary = {
      amount: 100,
      txn_count: 2,
      first_txn_at: "2026-01-01",
      last_txn_at: "2026-01-02",
    };
    summary[invalidField] = invalid;
    const maliciousCard = destinationRuntime.buildViaOneHopDiagnosticCard({
      caseId: "case-malicious-number",
      holderName: "甲方",
      viaName: "乙方",
      sourceToViaSummary: summary,
      sourceToViaCoreAccountSummary: {
        ...summary,
        counterparty_accounts: ["6222020000000000000"],
      },
    });
    assert.equal(
      maliciousCard.required_facts_present,
      false,
      `${invalidField}=${JSON.stringify(invalid)} must not make the card answer-ready`,
    );
    for (const factId of ["via_one_hop.full_period", "via_one_hop.core_account"]) {
      const fact = maliciousCard.facts.find((item) => item.fact_id === factId);
      assert.notEqual(fact.support_status, "supported", `${factId}/${invalidField} must not be supported`);
      assert.equal(
        Object.hasOwn(fact, invalidField === "amount" ? "amount" : "count"),
        false,
        `${factId}/${invalidField} must remain absent`,
      );
    }
    assertNoFactUpgrade(maliciousCard, `destination/${invalidField}/${JSON.stringify(invalid)}`);
  }
}

const explicitZeroViaCard = destinationRuntime.buildViaOneHopDiagnosticCard({
  caseId: "case-zero",
  holderName: "甲方",
  viaName: "乙方",
  sourceToViaSummary: { amount: 0, txn_count: 0, first_txn_at: "2026-01-01", last_txn_at: "2026-01-01" },
  sourceToViaCoreAccountSummary: {
    amount: 0,
    txn_count: 0,
    first_txn_at: "2026-01-01",
    last_txn_at: "2026-01-01",
    counterparty_accounts: ["6222020000000000000"]
  }
});
assert.equal(explicitZeroViaCard.required_facts_present, false, "explicit zero is not a host-verified no-hit receipt");
assert.equal(explicitZeroViaCard.facts.some((fact) => fact.support_status === "supported"), false);

for (const invalid of [true, false, [], [1], {}, "0x10", "1,000"]) {
  const rendered = renderDiagnosticCardForAgent({
    card_type: "source_to_counterparty_amount_card",
    holder_name: "甲方",
    via_holder_name: "乙方",
    source_to_via_summary: { amount: invalid, txn_count: invalid },
  });
  assert.match(rendered, /未返回|缺失|不得写成 0/u);
  assert.doesNotMatch(rendered, /\b1\s*笔|1\.00\s*元/u, `renderer must reject ${JSON.stringify(invalid)} numeric coercion`);
}

for (const invalid of [true, false, [], [1], {}, "0x10", "1,000", -1, 1.5]) {
  const risk = analyzeReportClaimText("资金已进入理财资产。", {
    supportedEdgeCount: invalid,
    flow_graph: { edges: [] },
  });
  assert.equal(
    Array.isArray(risk.unsupported_flows) && risk.unsupported_flows.length > 0,
    true,
    `invalid supported-edge count ${JSON.stringify(invalid)} must not suppress the no-flow boundary`,
  );
}

const partialContinuation = destinationRuntime.buildViaContinuationDiagnosticCard({
  caseId: "case-continuation-partial",
  holderName: "甲方",
  viaName: "乙方",
  sourceToViaSummary: { amount: 100 },
  viaCounterpartyRank: { data: { rankings: [{ display_name: "丙方", outflow: 50, txn_count: 1 }] } }
});
assert.equal(partialContinuation.required_facts_present, false, "partial one-hop and downstream summaries cannot authorize a continuation answer");

process.stdout.write(`${JSON.stringify({
  status: "ok",
  checks: [
    "optional number/int reject missing and malicious types",
    "internal limit defaults remain separate",
    "frontdoor summaries cannot synthesize zero or supported/no-hit",
    "destination one-hop and continuation partial values stay unsupported or unresolved",
    "destination cards reject boolean, container, hex, and comma numeric coercion",
    "explicit numeric and strict string zero remain distinct",
    "money display ignores untrusted object text",
    "aggregate, renderer, and claim review reject JavaScript numeric coercion"
  ]
}, null, 2)}\n`);
