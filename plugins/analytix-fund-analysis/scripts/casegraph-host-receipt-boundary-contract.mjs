#!/usr/bin/env node

import assert from "node:assert/strict";
import {
  buildCasegraphRankEvidence,
  createCaseGraphRuntime
} from "../mcp/casegraph-runtime.mjs";
import {
  buildCasegraphComponentEvidence,
  casegraphMinimumRoadmap,
  withCasegraphAnswerCardProtocol,
  withFundGraphProtocol
} from "../mcp/casegraph-protocol.mjs";
import {
  buildFundGraphFromSeedRows,
  summarizeSeedTransfers
} from "../mcp/fundgraph-builder.mjs";
import {
  buildEvidenceLedger,
  validateEvidenceLedgerCoverage
} from "../mcp/evidence-ledger.mjs";

function body(value) {
  return JSON.stringify(value);
}

function assertNoSupportedClaimOrCitation(value, label) {
  const serialized = body(value);
  assert.doesNotMatch(serialized, /"support_status":"supported"/u, `${label} must not issue supported claim/evidence`);
  assert.doesNotMatch(serialized, /"citation_status":"supported"/u, `${label} must not issue a supported citation`);
  assert.doesNotMatch(serialized, /"host_registry_verified":true/u, `${label} must not self-attest host registry membership`);
}

const fakeRefs = {
  query_ids: ["rank_accounts"],
  evidence_ids: ["fake-receipt-id"],
  audit_ref: { source: "untrusted-tool-payload" }
};

const missingRank = buildCasegraphRankEvidence({
  rows: [{
    account_key: "acct-missing-rank",
    txn_count: "bad-count",
    turnover: { yuan: "bad-amount" }
  }],
  rankKind: "account",
  sourceTool: "rank_accounts",
  evidenceRefs: fakeRefs,
  rankPayload: { metric: "turnover", limit: "bad-limit" },
  caseId: "case-a"
});
assert.equal(Object.hasOwn(missingRank.rows[0], "rank"), false, "missing/invalid external rank must not become source index or zero");
assert.equal(Object.hasOwn(missingRank.evidence_pack[0], "rank"), false);
assert.equal(Object.hasOwn(missingRank.evidence_pack[0], "amount"), false, "invalid amount must remain absent");
assert.equal(Object.hasOwn(missingRank.evidence_pack[0], "txn_count"), false, "invalid count must remain absent");
assert.equal(missingRank.rows[0].support_status, "unresolved");
assert.equal(missingRank.claim_support[0].support_status, "unresolved");
assert.equal(Object.hasOwn(missingRank.rows[0].source_refs, "evidence_ids"), false, "plugin ids are not host EvidenceReceipts");
assertNoSupportedClaimOrCitation(missingRank, "invalid rank aggregate");

const explicitZeroRank = buildCasegraphRankEvidence({
  rows: [{
    rank: 1,
    account_key: "acct-zero",
    txn_count: 0,
    turnover: { yuan: 0, text: "untrusted display" },
    host_registry_verified: true,
    citation_status: "supported",
    evidence_receipt_ids: ["fake-receipt-id"]
  }],
  rankKind: "account",
  sourceTool: "rank_accounts",
  evidenceRefs: fakeRefs,
  rankPayload: { metric: "turnover", limit: 5 },
  caseId: "case-a"
});
assert.equal(explicitZeroRank.evidence_pack[0].amount.yuan, 0, "explicit aggregate zero must be preserved");
assert.equal(explicitZeroRank.evidence_pack[0].txn_count, 0, "explicit count zero must be preserved");
assert.equal(explicitZeroRank.rows[0].support_status, "candidate");
assert.equal(explicitZeroRank.claim_support[0].support_status, "unresolved");
assert.match(explicitZeroRank.claim_support[0].claim_text, /不构成 verified no-hit/u);
assertNoSupportedClaimOrCitation(explicitZeroRank, "explicit-zero rank aggregate");

const completeSeedGraph = buildFundGraphFromSeedRows({
  holderName: "甲方",
  sourceSeeds: [{
    rank: 1,
    seed: {
      txn_id: "txn-candidate",
      txn_time: "2026-07-10T00:00:00Z",
      account_key: "acct-a",
      account_open_name: "甲方",
      counterparty_key: "acct-b",
      counterparty_name: "乙方",
      amount: 25,
      direction: "out"
    }
  }]
});
assert.equal(completeSeedGraph.edges[0].edge_status, "candidate", "complete external seed is still only a candidate before host receipt verification");
assert.equal(completeSeedGraph.edges[0].host_registry_verified, false);
assert.equal(completeSeedGraph.flowGraph.drawable_edges.length, 0, "candidate seed edge must not become a publishable graph edge");
assertNoSupportedClaimOrCitation(completeSeedGraph, "complete seed graph");

const zeroSeedGraph = buildFundGraphFromSeedRows({
  holderName: "甲方",
  sourceSeeds: [{
    rank: 1,
    seed: {
      txn_id: "txn-zero",
      txn_time: "2026-07-10T00:00:00Z",
      account_key: "acct-a",
      account_open_name: "甲方",
      counterparty_key: "acct-b",
      counterparty_name: "乙方",
      amount: 0,
      direction: "out"
    }
  }]
});
assert.equal(zeroSeedGraph.edges[0].amount.yuan, 0, "explicit seed zero must remain distinct from a missing amount");
assert.equal(zeroSeedGraph.edges[0].edge_status, "needs_evidence");
assert(zeroSeedGraph.edges[0].missing_fields.includes("positive_amount"));
assert.doesNotMatch(body(zeroSeedGraph), /verified[_ -]?no[_ -]?hit|"no_hit":true/iu);

const zeroSummary = summarizeSeedTransfers([{ seed: {
  txn_id: "txn-zero",
  txn_time: "2026-07-10T00:00:00Z",
  account_key: "acct-a",
  counterparty_key: "acct-b",
  amount: { yuan: 0 }
} }]);
assert.equal(zeroSummary.amount.yuan, 0);
assert.equal(zeroSummary.txn_count, 1, "an explicit zero row is not an empty result");
assert.equal(zeroSummary.support_status, "candidate");
assert.equal(zeroSummary.verified_no_hit, false);
assert.equal(summarizeSeedTransfers([{ seed: { amount: false } }]), undefined, "invalid seed amount must not become zero");

const structuralGraph = withFundGraphProtocol({
  edges: [{
    edge_id: "edge-structural",
    edge_status: "supported",
    txn_id: "txn-structural",
    txn_time: "2026-07-10T00:00:00Z",
    amount: 25,
    direction: "out",
    from: "acct-a",
    to: "acct-b",
    source_tool: "trace_subject_top_outflows",
    source_refs: fakeRefs,
    host_registry_verified: true,
    citation_status: "supported",
    evidence_receipt_ids: ["fake-receipt-id"]
  }]
});
assert.equal(structuralGraph.edges[0].host_registry_verified, false);
assert.equal(structuralGraph.edges[0].citation_status, "unresolved");
assert.equal(Object.hasOwn(structuralGraph.edges[0], "evidence_receipt_ids"), false);
assert.equal(Object.hasOwn(structuralGraph.edges[0].source_refs, "evidence_ids"), false);
assert.equal(structuralGraph.evidence_pack[0].support_status, "candidate");
assert.equal(structuralGraph.claim_support[0].support_status, "unresolved");
assert.equal(structuralGraph.publishable_edge_count, 0);
assert.equal(structuralGraph.fact_answer_allowed, false);
assert.equal(Object.hasOwn(structuralGraph.evidence_pack[0].source_refs, "evidence_ids"), false);
assertNoSupportedClaimOrCitation({
  evidence_pack: structuralGraph.evidence_pack,
  claim_support: structuralGraph.claim_support
}, "structurally complete graph support surfaces");

for (const blockingGateCount of [undefined, "", "not-a-count", false, [], {}]) {
  const card = withCasegraphAnswerCardProtocol({
    blocking_gate_count: blockingGateCount,
    host_registry_verified: true,
    evidence_receipt_ids: ["fake-receipt-id"]
  });
  assert.equal(card.required_facts_present, false);
  assert.equal(card.fact_answer_allowed, false);
  assert.equal(card.recommended_next_action, "answer_boundary_then_resolve_quality_gate");
  assert.equal(card.host_registry_verified, false);
  assert.equal(Object.hasOwn(card, "evidence_receipt_ids"), false);
  assert.equal(Object.hasOwn(card, "blocking_gate_count"), false, "invalid/missing gate count must not become zero");
}

const componentEvidence = buildCasegraphComponentEvidence({
  graph: {
    minimum_roadmap: {
      minimum_nodes_and_edges: [{
        name: "账户节点",
        status: "ready_from_rank_facts",
        required_facts: ["account_key"],
        boundary: "candidate only"
      }, {
        name: "对手方节点",
        status: "supported",
        required_facts: ["counterparty_key"],
        boundary: "model/tool positive status is untrusted"
      }]
    }
  },
  evidenceRefs: fakeRefs,
  caseId: "case-a"
});
assert(componentEvidence.evidence_pack.every((entry) => entry.support_status === "candidate"));
assert(componentEvidence.claim_support.every((entry) => entry.support_status === "unresolved"));
assert.equal(Object.hasOwn(componentEvidence.evidence_pack[0].source_refs, "evidence_ids"), false);
assertNoSupportedClaimOrCitation(componentEvidence, "casegraph component evidence");

const roadmap = casegraphMinimumRoadmap({
  top_accounts: [{}],
  top_holders: [{}],
  top_counterparties: [{}],
  mandatory_gates: [{ status: "bogus" }]
});
assert.doesNotMatch(body(roadmap), /"status":"ready/iu, "roadmap must not describe unverified external fields as ready");

const runtime = createCaseGraphRuntime({
  resolveCase: async () => ({ case_id: "case-a", case: { case_name: "A" } }),
  executeSkill: async () => ({ data: {} }),
  getCaseScopeMap: async () => ({
    case_id: "case-a",
    map_version: "v1",
    map: { nodes: [], edges: [] },
    warnings: []
  }),
  compactCaseRecord: (value) => value
});
const missingGateRuntimeResult = await runtime.getCaseGraph({ include_rankings: false });
assert.equal(missingGateRuntimeResult.status, "partial", "missing gate list must block casegraph readiness");
assert.equal(missingGateRuntimeResult.key_facts.gates[0].status, "needs_evidence");
assert.equal(missingGateRuntimeResult.answer_card.required_facts_present, false);
assert.equal(missingGateRuntimeResult.answer_card.fact_answer_allowed, false);

const ledger = buildEvidenceLedger({
  case_id: "case-a",
  facts: [{
    fact_id: "fact-zero",
    amount: 0,
    txn_count: 0,
    support_status: "supported",
    evidence_ids: ["fake-receipt-id"]
  }]
});
assert(ledger.entries.length > 0);
assert(ledger.entries.every((entry) => entry.support_status === "unresolved"));
assert(ledger.entries.every((entry) => entry.host_registry_verified === false));
assert.equal(ledger.entries[0].fields.amount, 0, "ledger must preserve explicit numeric zero");
assert.equal(ledger.entries[0].fields.txn_count, 0);
assert.equal(ledger.publication_ready, false);
assert.equal(ledger.fact_answer_allowed, false);
assertNoSupportedClaimOrCitation(ledger, "evidence ledger");

const malformedLedgerValidation = validateEvidenceLedgerCoverage({
  ledger_version: "evidence-ledger-v1",
  ledger_id: "malformed",
  scanned_entry_count: "not-a-count",
  coverage_summary: {
    missing_required_surfaces: [],
    untraceable_required_surface_entry_count: "not-a-count",
    entries_with_source_refs: false
  }
});
assert.equal(malformedLedgerValidation.ok, false, "invalid external ledger counters must fail closed instead of becoming zero/complete");
assert.equal(malformedLedgerValidation.publication_ready, false);
assert.equal(malformedLedgerValidation.host_registry_verified, false);

process.stdout.write(`${JSON.stringify({
  status: "ok",
  checks: [
    "rank values stay optional and never self-issue supported claims/citations",
    "explicit aggregate zero is preserved but is not verified no-hit",
    "seed-derived graph edges remain candidate/unresolved without host receipts",
    "missing or invalid gate fields cannot become ready",
    "casegraph component readiness is candidate-only",
    "ledger support and malformed external counters fail closed"
  ]
}, null, 2)}\n`);
