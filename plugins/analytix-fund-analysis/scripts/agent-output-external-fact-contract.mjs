#!/usr/bin/env node

import assert from "node:assert/strict";
import { createAgentOutputCompiler } from "../mcp/agent-output-compiler.mjs";

const compiler = createAgentOutputCompiler({});
const invalidValues = [
  ["undefined", undefined],
  ["null", null],
  ["empty", ""],
  ["blank", " \t"],
  ["false", false],
  ["true", true],
  ["array", []],
  ["array-zero", [0]],
  ["object", {}],
  ["nan", Number.NaN],
  ["infinity", Number.POSITIVE_INFINITY]
];

function pairPayload(amount, count, extra = {}) {
  return {
    status: "ok",
    answer_card: {
      card_type: "pair_amount_review_card",
      required_facts_present: true,
      answer_card_complete: true,
      full_period_effective_amount: amount,
      full_period_effective_count: count,
      ...extra
    }
  };
}

function assertNoSyntheticZero(text, label) {
  assert.doesNotMatch(text, /(?:^|[^\d])0\.00\s*元/u, `${label} synthesized zero amount`);
  assert.doesNotMatch(text, /(?:^|[^\d])0\s*笔/u, `${label} synthesized zero count`);
  assert.doesNotMatch(text, /(?:^|[^\d])0(?:\.0+)?%/u, `${label} synthesized zero percentage`);
  assert.doesNotMatch(text, /事实用途:[^\n]*事实已足够/u, `${label} upgraded malformed values to facts-enough`);
}

for (const [label, value] of invalidValues) {
  const payload = pairPayload(value, value, {
    first_txn_at: "2026-07-01",
    last_txn_at: "2026-07-10",
    raw_detail_amount: value,
    raw_detail_count: value,
    focus_cluster: {
      effective_amount: value,
      effective_count: value,
      share_of_effective_amount: value
    },
    outside_focus_cluster_effective_amount: 999,
    outside_focus_cluster_effective_count: 9
  });
  const structured = compiler.compactStructuredContent(payload);
  const pack = structured.support_facts?.pair_amount_fact_pack || {};
  assert.equal(Object.prototype.hasOwnProperty.call(pack, "effective_amount"), false, `${label} amount must be absent`);
  assert.equal(Object.prototype.hasOwnProperty.call(pack, "effective_count"), false, `${label} count must be absent`);
  assert.equal(Object.prototype.hasOwnProperty.call(structured.support_facts || {}, "other_verified_transfer_amount"), false, `${label} must not accept an ungrounded Pair difference`);
  assert.equal(structured.validation_state.required_facts_present, false, `${label} must fail required facts`);
  assert.equal(structured.validation_state.answer_card_complete, false, `${label} must fail card completeness`);
  assertNoSyntheticZero(compiler.agentReadableToolText(payload), `pair/${label}`);

  const diagnosticPayload = {
    status: "ok",
    answer_card: {
      card_type: "source_to_counterparty_amount_card",
      facts: [{
        fact_id: `fact-${label}`,
        label: "恶意数值事实",
        answer_text: "恶意数值已经获得支持",
        amount: value,
        count: value,
        support_status: "supported"
      }]
    }
  };
  const diagnosticStructured = compiler.compactStructuredContent(diagnosticPayload);
  const diagnosticFact = diagnosticStructured.support_facts?.facts?.[0] || {};
  assert.equal(diagnosticFact.support_status, "needs_evidence", `${label} diagnostic fact must be downgraded`);
  assert.equal(Object.prototype.hasOwnProperty.call(diagnosticFact, "amount"), false);
  assert.equal(Object.prototype.hasOwnProperty.call(diagnosticFact, "count"), false);
  assert.equal(Object.prototype.hasOwnProperty.call(diagnosticFact, "statement"), false);
  assert.equal(compiler.agentReadableToolText(diagnosticPayload).includes("恶意数值已经获得支持"), false);
}

const zeroPayload = pairPayload(0, "0", {
  first_txn_at: "2026-07-01",
  last_txn_at: "2026-07-10"
});
const zeroStructured = compiler.compactStructuredContent(zeroPayload);
assert.equal(zeroStructured.support_facts.pair_amount_fact_pack.effective_amount, 0);
assert.equal(zeroStructured.support_facts.pair_amount_fact_pack.effective_count, 0);
assert.equal(zeroStructured.validation_state.required_facts_present, false);
assert.equal(zeroStructured.validation_state.answer_card_complete, false);
assert.equal(zeroStructured.validation_state.verified_no_hit_allowed, false);
const zeroText = compiler.agentReadableToolText(zeroPayload);
assert.match(zeroText, /显式零值（未验证未命中）/u);
assert.match(zeroText, /0\.00 元/u);
assert.doesNotMatch(zeroText, /事实用途:[^\n]*事实已足够/u);

const incompleteDifference = compiler.compactStructuredContent(pairPayload(100, 10, {
  first_txn_at: "2026-07-01",
  last_txn_at: "2026-07-10",
  focus_cluster: { effective_amount: false, effective_count: 3 },
  outside_focus_cluster_effective_amount: 70,
  outside_focus_cluster_effective_count: 7
}));
assert.equal(Object.prototype.hasOwnProperty.call(incompleteDifference.support_facts, "other_verified_transfer_amount"), false);
assert.equal(Object.prototype.hasOwnProperty.call(incompleteDifference.support_facts.pair_amount_fact_pack, "other_verified_transfer_amount"), false);

const completeDifference = compiler.compactStructuredContent(pairPayload(100, 10, {
  first_txn_at: "2026-07-01",
  last_txn_at: "2026-07-10",
  focus_cluster: { effective_amount: 30, effective_count: 3 }
}));
assert.equal(completeDifference.support_facts.pair_amount_fact_pack.other_verified_transfer_amount, 70);
assert.equal(completeDifference.support_facts.pair_amount_fact_pack.other_verified_transfer_count, 7);
assert.equal(completeDifference.support_facts.pair_amount_structure_review.other_verified_transfers.effective_amount, 70);
assert.equal(completeDifference.support_facts.pair_amount_structure_review.other_verified_transfers.effective_count, 7);

const invalidKeyedDifference = compiler.compactStructuredContent(pairPayload(100, 10, {
  first_txn_at: "2026-07-01",
  last_txn_at: "2026-07-10",
  focus_cluster: { effective_amount: 30, effective_count: 3 },
  outside_focus_cluster_effective_amount: false,
  outside_focus_cluster_effective_count: []
}));
assert.equal(Object.prototype.hasOwnProperty.call(invalidKeyedDifference.support_facts.pair_amount_fact_pack, "other_verified_transfer_amount"), false);
assert.equal(Object.prototype.hasOwnProperty.call(invalidKeyedDifference.support_facts.pair_amount_fact_pack, "other_verified_transfer_count"), false);

const partialCueBucket = compiler.compactStructuredContent(pairPayload(100, 2, {
  first_txn_at: "2026-07-01",
  last_txn_at: "2026-07-10",
  top_transactions: [
    { txn_time: "2026-07-01", amount: 100, summary: "借款还款" },
    { txn_time: "2026-07-02", amount: null, summary: "借款还款" }
  ]
})).support_facts.pair_amount_fact_pack.row_cue_buckets[0];
assert.equal(partialCueBucket.coverage_status, "partial_missing_amount");
assert.equal(partialCueBucket.amount_valid_count, 1);
assert.equal(partialCueBucket.amount_missing_count, 1);
assert.equal(Object.prototype.hasOwnProperty.call(partialCueBucket, "amount"), false, "partial cue bucket must not expose a partial sum as the bucket amount");

const completeCueBucket = compiler.compactStructuredContent(pairPayload(5, 2, {
  first_txn_at: "2026-07-01",
  last_txn_at: "2026-07-10",
  top_transactions: [
    { txn_time: "2026-07-01", amount: 0, summary: "借款还款" },
    { txn_time: "2026-07-02", amount: 5, summary: "借款还款" }
  ]
})).support_facts.pair_amount_fact_pack.row_cue_buckets[0];
assert.equal(completeCueBucket.coverage_status, "complete");
assert.equal(completeCueBucket.amount_valid_count, 2);
assert.equal(completeCueBucket.amount_missing_count, 0);
assert.equal(completeCueBucket.amount, 5, "complete cue bucket must preserve valid zero while summing all rows");

const overflowCueBucket = compiler.compactStructuredContent(pairPayload(1, 2, {
  first_txn_at: "2026-07-01",
  last_txn_at: "2026-07-10",
  top_transactions: [
    { txn_time: "2026-07-01", amount: 1e308, summary: "借款还款" },
    { txn_time: "2026-07-02", amount: 1e308, summary: "借款还款" }
  ]
})).support_facts.pair_amount_fact_pack.row_cue_buckets[0];
assert.equal(overflowCueBucket.coverage_status, "invalid_non_finite_total");
assert.equal(overflowCueBucket.amount_valid_count, 2);
assert.equal(overflowCueBucket.amount_missing_count, 0);
assert.equal(Object.prototype.hasOwnProperty.call(overflowCueBucket, "amount"), false, "overflowed cue bucket must not expose a non-finite or synthetic amount");

const malformedFullCase = {
  status: "ok",
  key_facts: {
    full_case_package: {
      coverage: { txn_count: false, row_count: [], turnover_amount: {} },
      scope_stats: { account_count: " ", counterparty_count: true },
      field_quality: { counterparty: false, ip_addr: [], mac_addr: {}, cash_flag: " " },
      top_accounts: [{ txn_count: false, turnover: [] }]
    }
  }
};
const malformedFullText = compiler.agentReadableToolText(malformedFullCase);
assert.match(malformedFullText, /未返回可验证的案件事实字段/u);
assertNoSyntheticZero(malformedFullText, "full-case keyed invalid");
assert.doesNotMatch(malformedFullText, /事实覆盖: 仅限/u);

const malformedGraph = {
  status: "ok",
  key_facts: {
    fund_flow_fact_pack: {
      source_holder: "甲方",
      via_holder: "乙方",
      source_edges: [{
        edge_id: "edge-malformed",
        edge_status: "supported",
        from_label: "伪边付款方",
        to_label: "伪边收款方",
        txn_time: "2026-07-10",
        amount: false,
        txn_count: []
      }],
      current_answer_sufficiency: "事实已足够"
    }
  }
};
const malformedGraphText = compiler.agentReadableToolText(malformedGraph);
assert.equal(malformedGraphText.includes("伪边付款方"), false, "supported graph row without amount must not render as a fact row");
assert.equal(malformedGraphText.includes("事实已足够"), false, "graph narrative readiness must not be trusted");
assert.match(malformedGraphText, /没有金额字段完整/u);
const malformedGraphStructured = compiler.compactStructuredContent(malformedGraph);
const malformedEdge = malformedGraphStructured.compact_evidence.fund_flow_fact_pack.source_edges[0];
assert.equal(malformedEdge.edge_status, "needs_evidence");
assert.equal(Object.prototype.hasOwnProperty.call(malformedEdge, "amount"), false);
assert.equal(Object.prototype.hasOwnProperty.call(malformedEdge, "txn_count"), false);

process.stdout.write(`${JSON.stringify({
  status: "ok",
  checks: [
    "malformed external numerics never become zero facts",
    "diagnostic supported facts downgrade on invalid numeric fields",
    "explicit zero remains visible but cannot become verified no-hit",
    "Pair differences require complete valid operands",
    "cue bucket amounts require complete finite row-level aggregation",
    "keyed-but-invalid full-case payload is not fact-ready",
    "supported graph rows require a valid amount"
  ]
}, null, 2)}\n`);
