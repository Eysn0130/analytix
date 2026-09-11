#!/usr/bin/env node

import assert from "node:assert/strict";

import {
  compactAccountStatsFact,
  compactFlowGraphForText,
  compactMoneyFact,
  compactOwnerScope,
  compactQaReviewFacts,
  compactToolPayloadForAgent,
  sameFactEligibilityComplete
} from "../mcp/agent-payload-compiler.mjs";

function hasOwn(value, key) {
  return Object.prototype.hasOwnProperty.call(value || {}, key);
}

function compileTool(tool, data) {
  return compactToolPayloadForAgent({
    tool,
    response: { status: "ok", data }
  });
}

for (const value of [undefined, null, "", " ", false, true, [], [0], {}, "NaN", "0x10", Number.NaN, Number.POSITIVE_INFINITY]) {
  assert.equal(compactMoneyFact(value), undefined, `invalid money ${String(value)} must remain absent`);
}
assert.equal(compactMoneyFact(0)?.yuan, 0, "explicit numeric zero must survive money projection");
assert.equal(compactMoneyFact("0")?.yuan, 0, "strict numeric string zero must survive money projection");

const partialAccount = compactAccountStatsFact({ summary: { inflow_total: 10 } });
assert.equal(partialAccount.inflow?.yuan, 10);
assert.equal(hasOwn(partialAccount, "turnover"), false, "one-sided account flow must not synthesize turnover");
const completeAccount = compactAccountStatsFact({ summary: { inflow_total: 10, outflow_total: 5 } });
assert.equal(completeAccount.turnover?.yuan, 15, "complete inflow and outflow may derive turnover");
const explicitZeroAccount = compactAccountStatsFact({ summary: { turnover_total: 0 } });
assert.equal(explicitZeroAccount.turnover?.yuan, 0, "explicit account turnover zero must survive");

const missingOwnerScope = compactOwnerScope({});
for (const key of ["id_no_provided", "account_keys_preview", "account_keys_truncated"]) {
  assert.equal(hasOwn(missingOwnerScope, key), false, `missing owner scope ${key} must remain unknown`);
}
const explicitEmptyOwnerScope = compactOwnerScope({
  id_no_provided: false,
  account_keys: [],
  account_keys_truncated: false
});
assert.equal(explicitEmptyOwnerScope.id_no_provided, false);
assert.deepEqual(explicitEmptyOwnerScope.account_keys_preview, []);
assert.equal(explicitEmptyOwnerScope.account_keys_truncated, false);
const invalidOwnerScope = compactOwnerScope({
  id_no_provided: "false",
  account_keys: "not-an-array",
  account_keys_truncated: "false"
});
for (const key of ["id_no_provided", "account_keys_preview", "account_keys_truncated"]) {
  assert.equal(hasOwn(invalidOwnerScope, key), false, `invalid owner scope ${key} must remain unknown`);
}
const conflictingOwnerScope = compactOwnerScope({
  id_no_provided: false,
  subject: { id_no_provided: true },
  account_keys_truncated: false,
  direct_account_keys_truncated: true
});
assert.equal(hasOwn(conflictingOwnerScope, "id_no_provided"), false);
assert.equal(hasOwn(conflictingOwnerScope, "account_keys_truncated"), false);

function compiledTopAccount(row) {
  return compileTool("analyze_holder_full", {
    holder_scope: { holder_name: "测试主体" },
    account_analysis: { account_stats: { accounts: [row] } }
  }).key_facts.holder_top_accounts[0];
}
assert.equal(hasOwn(compiledTopAccount({ account_key: "a", inflow_total: 10 }), "turnover"), false);
assert.equal(compiledTopAccount({ account_key: "b", inflow_total: 10, outflow_total: 5 }).turnover?.yuan, 15);
assert.equal(compiledTopAccount({ account_key: "c", turnover_total: 0 }).turnover?.yuan, 0);

const missingWorkbench = compileTool("run_case_sql", {
  execution_status: "executed"
}).key_facts.workbench;
assert.equal(hasOwn(missingWorkbench, "row_count"), false, "missing workbench row_count must remain absent");
assert.equal(hasOwn(missingWorkbench, "returned_row_count"), false);
assert.equal(hasOwn(missingWorkbench, "source_row_dump_exposed"), false, "missing raw-row status must remain unknown");
assert.match(missingWorkbench.evidence_card_summary, /状态未知/u);
assert.match(missingWorkbench.validation_summary_text, /状态未知/u);
assert.match(missingWorkbench.validation_status, /返回行数=未知/u);
assert.doesNotMatch(missingWorkbench.validation_status, /返回行数=0/u);

const unverifiedWorkbenchIds = compileTool("run_case_sql", {
  execution_status: "executed",
  query_id: "model-or-tool-supplied-query-id",
  evidence_id: "model-or-tool-supplied-evidence-id",
  artifact_id: "model-or-tool-supplied-artifact-id",
  report_path: "/tmp/unpublished-report.docx",
  validation_state: { artifact_written: true }
}).key_facts.workbench;
for (const key of ["query_created", "evidence_created", "artifact_created"]) {
  assert.equal(hasOwn(unverifiedWorkbenchIds, key), false, `${key} must not be minted from a non-empty id`);
}
assert.equal(
  hasOwn(unverifiedWorkbenchIds, "artifact_inspection_status"),
  false,
  "artifact_written must not be upgraded to inspection passed"
);

const zeroWorkbench = compileTool("run_case_sql", {
  execution_status: "executed",
  records: [],
  truncated: false
}).key_facts.workbench;
assert.equal(zeroWorkbench.row_count, 0, "an explicitly present empty row structure may derive zero rows");
assert.equal(zeroWorkbench.returned_row_count, 0);
assert.equal(zeroWorkbench.source_row_dump_exposed, undefined);
assert.match(zeroWorkbench.validation_status, /返回行数=0/u);

const explicitPrivateWorkbench = compileTool("run_case_sql", {
  execution_status: "executed",
  records: [],
  raw_rows_exposed: false
}).key_facts.workbench;
assert.equal(explicitPrivateWorkbench.source_row_dump_exposed, false);
assert.match(explicitPrivateWorkbench.evidence_card_summary, /明确未向模型展开来源明细/u);

const invalidWorkbench = compileTool("run_case_sql", {
  execution_status: "executed",
  row_count: false
}).key_facts.workbench;
assert.equal(hasOwn(invalidWorkbench, "row_count"), false, "invalid integer types must not become zero");

const unknownDiagnosticPlan = compileTool("diagnose_case_sql", {
  execution_status: "diagnosed",
  plan: {
    full_scan_possible: "false",
    estimated_row_upper_bound: "0"
  }
}).key_facts.workbench.plan_summary;
assert.equal(hasOwn(unknownDiagnosticPlan, "full_table_scan_possible"), false);
assert.equal(hasOwn(unknownDiagnosticPlan, "estimated_row_upper_bound"), false);

const explicitDiagnosticPlan = compileTool("diagnose_case_sql", {
  execution_status: "diagnosed",
  plan: {
    full_scan_possible: false,
    estimated_row_upper_bound: 0
  }
}).key_facts.workbench.plan_summary;
assert.equal(explicitDiagnosticPlan.full_table_scan_possible, false);
assert.equal(explicitDiagnosticPlan.estimated_row_upper_bound, 0);

const partialQa = compactQaReviewFacts({
  account_identity_quality: { account_count: 2 }
}, {});
assert.deepEqual(partialQa.field_gaps, { account_dim_account_count: 2 });
for (const key of ["import_lineage", "cleaning_flags", "time_quality", "same_fact_risk", "samples"]) {
  assert.equal(hasOwn(partialQa, key), false, `partial QA must not synthesize ${key}`);
}
assert.equal(partialQa.policy.status, "unknown", "missing dedupe policy must remain unknown");
assert.equal(hasOwn(partialQa.policy, "full_case_blind_dedupe_allowed"), false);
assert.equal(hasOwn(partialQa.policy, "deduct_from_totals_allowed"), false);

const explicitZeroQa = compactQaReviewFacts({
  cleaning_quality: { flags: { txn_total: 0 } },
  account_identity_quality: { unregistered_turnover_total: 0 }
}, {
  dedupe_policy: {
    full_case_blind_dedupe_allowed: false,
    deduct_from_totals_allowed: false
  }
});
assert.equal(explicitZeroQa.cleaning_flags.txn_total, 0);
assert.equal(explicitZeroQa.field_gaps.account_dim_unregistered_turnover, 0);
assert.equal(explicitZeroQa.policy.full_case_blind_dedupe_allowed, false);
assert.equal(explicitZeroQa.policy.deduct_from_totals_allowed, false);

function sameFactEligibility(overrides = {}) {
  return {
    contract: "SameFactEligibilityV1",
    key_version: "analytix.same-fact-dedupe-key/v2",
    coverage_status: "complete",
    blocker: null,
    requested_row_count: 2,
    fact_eligible_row_count: 2,
    family_eligible_row_count: 2,
    rejected_row_count: 0,
    rejection_reason_histogram: {},
    ...overrides
  };
}

assert.equal(sameFactEligibilityComplete(sameFactEligibility()), true);
for (const invalid of [
  sameFactEligibility({ coverage_status: "partial", blocker: "same_fact_eligibility_incomplete" }),
  sameFactEligibility({ requested_row_count: 0, fact_eligible_row_count: 0, family_eligible_row_count: 0 }),
  sameFactEligibility({ rejected_row_count: 1, rejection_reason_histogram: { missing_amount: 1 } }),
  sameFactEligibility({ key_version: "analytix.same-fact-dedupe-key/v1" })
]) {
  assert.equal(sameFactEligibilityComplete(invalid), false, "incomplete eligibility must fail closed");
}

const partialSameFactQa = compactQaReviewFacts({
  cleaning_quality: {
    same_fact_eligibility: sameFactEligibility({
      coverage_status: "partial",
      blocker: "same_fact_eligibility_incomplete",
      fact_eligible_row_count: 1,
      family_eligible_row_count: 1,
      rejected_row_count: 1,
      rejection_reason_histogram: { missing_amount: 1 }
    }),
    duplicate_summary: {
      same_fact_cross_account_groups: 9,
      same_fact_cross_account_extra_rows: 99,
      same_holder_same_fact_candidate_duplicate_amount: 123456
    },
    duplicate_examples: {
      same_fact_cross_account: [{ amount: 123456, row_count: 99 }]
    }
  }
}, {
  same_fact_eligibility: sameFactEligibility({
    coverage_status: "partial",
    blocker: "same_fact_eligibility_incomplete",
    fact_eligible_row_count: 1,
    family_eligible_row_count: 1,
    rejected_row_count: 1,
    rejection_reason_histogram: { missing_amount: 1 }
  }),
  summary: {
    same_holder_same_fact: { group_count: 9, extra_rows: 99, candidate_duplicate_amount: 123456 }
  },
  families: [{ amount: 123456, row_count: 99 }]
});
assert.equal(hasOwn(partialSameFactQa, "same_fact_risk"), false, "partial eligibility must suppress same-fact facts");
assert.equal(hasOwn(partialSameFactQa, "samples"), false, "partial eligibility must suppress same-fact samples");

const completeSameFactQa = compactQaReviewFacts({
  cleaning_quality: {
    same_fact_eligibility: sameFactEligibility(),
    duplicate_summary: {
      same_fact_cross_account_groups: 0,
      same_fact_cross_account_extra_rows: 0
    }
  }
}, {
  same_fact_eligibility: sameFactEligibility(),
  summary: {
    same_holder_same_fact: { group_count: 0, extra_rows: 0, candidate_duplicate_amount: 0 }
  }
});
assert.equal(completeSameFactQa.same_fact_risk.cross_account_same_fact_groups, 0);
assert.equal(completeSameFactQa.same_fact_risk.same_holder_group_count, 0);
assert.equal(completeSameFactQa.same_fact_risk.same_holder_candidate_duplicate_amount, 0);

const missingGraph = compactFlowGraphForText({ graph_version: "1" });
for (const key of ["node_count", "edge_count", "supported_edge_count", "incomplete_edge_count", "edge_status_counts"]) {
  assert.equal(hasOwn(missingGraph, key), false, `missing graph arrays must not derive ${key}`);
}
const nodesOnlyGraph = compactFlowGraphForText({ nodes: [] });
assert.equal(nodesOnlyGraph.node_count, 0);
for (const key of ["edge_count", "supported_edge_count", "incomplete_edge_count"]) {
  assert.equal(hasOwn(nodesOnlyGraph, key), false, `nodes-only graph must not derive ${key}`);
}
const explicitEmptyGraph = compactFlowGraphForText({ nodes: [], edges: [] });
assert.equal(explicitEmptyGraph.node_count, 0);
assert.equal(explicitEmptyGraph.edge_count, 0);
assert.equal(explicitEmptyGraph.supported_edge_count, 0);
assert.equal(explicitEmptyGraph.incomplete_edge_count, 0);

const missingCaseGraphSummary = compactToolPayloadForAgent({
  tool: "funds_investigate",
  status: "ok",
  answer_card: { title: "scope" },
  casegraph: { graph_version: "1" }
}).key_facts.casegraph_summary;
for (const key of ["node_count", "gate_count", "boundary_count"]) {
  assert.equal(hasOwn(missingCaseGraphSummary, key), false, `casegraph summary must not derive ${key} from a missing array`);
}

const missingRankingCounts = compileTool("rank_accounts", {
  rankings: [],
  data_exhausted: true
});
const missingRankingContext = missingRankingCounts.key_facts.ranking_context;
const missingRankingContract = missingRankingCounts.key_facts.rankings[0];
assert.equal(hasOwn(missingRankingContext, "returned_count"), false);
for (const key of ["returned_count", "visible_count", "filtered_count", "data_exhausted", "compact_text_row_count", "final_answer_expected_min_rows"]) {
  assert.equal(hasOwn(missingRankingContract, key), false, `missing ranking count must not synthesize ${key}`);
}
assert.doesNotMatch(JSON.stringify(missingRankingCounts), /no[_ -]?hit|verified[_ -]?no[_ -]?hit/iu);

const explicitEmptyRanking = compileTool("rank_accounts", {
  rankings: [],
  returned_count: 0,
  data_exhausted: true
});
assert.equal(explicitEmptyRanking.key_facts.ranking_context.returned_count, 0);
assert.equal(explicitEmptyRanking.key_facts.rankings[0].returned_count, 0);
assert.equal(explicitEmptyRanking.key_facts.rankings[0].data_exhausted, true);

process.stdout.write(`${JSON.stringify({
  status: "ok",
  checks: [
    "strict optional money and integer facts",
    "account turnover requires explicit turnover or complete inflow and outflow",
    "owner scope booleans and arrays preserve unknown, explicit false, and alias conflicts",
    "workbench missing rows remain distinct from explicit empty rows",
    "partial QA projects only present fields and unknown policy",
    "same-fact facts require complete non-empty SameFactEligibilityV1 coverage",
    "graph counts derive only from present arrays",
    "ranking count gaps cannot become exhausted or no-hit"
  ]
}, null, 2)}\n`);
