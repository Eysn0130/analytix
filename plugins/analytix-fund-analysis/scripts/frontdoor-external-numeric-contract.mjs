#!/usr/bin/env node

import assert from "node:assert/strict";

import { createFrontdoorRuntime } from "../mcp/frontdoor-runtime.mjs";

function failedSkill(name, input) {
  return {
    ok: false,
    skill_id: name,
    input,
    data: {},
    warnings: [{ code: "CONTRACT_SOURCE_MISSING", severity: "warning", message: "source missing" }],
    evidence_refs: {}
  };
}

function successfulSkill(name, input, data) {
  return {
    ok: true,
    skill_id: name,
    input,
    data,
    warnings: [],
    evidence_refs: {}
  };
}

function makeRuntime({ skillHandlers = {}, casegraph, flowGraph } = {}) {
  return createFrontdoorRuntime({
    resolveCase: async () => ({ case_id: "case-a", source: "contract", case: { name: "Contract Case" } }),
    safeSkill: async (name, input) => {
      const handler = skillHandlers[name];
      if (!handler) return failedSkill(name, input);
      const result = typeof handler === "function" ? await handler(input) : handler;
      return {
        skill_id: name,
        input,
        warnings: [],
        evidence_refs: {},
        ...result
      };
    },
    getCaseGraph: async () => casegraph || {
      tool: "get_casegraph",
      status: "skipped",
      key_facts: {},
      warnings: [],
      evidence_refs: {}
    },
    buildFundFlowGraph: async () => flowGraph || {
      tool: "build_fund_flow_graph",
      status: "partial",
      flow_graph: { edges: [] },
      warnings: [],
      key_facts: {}
    },
    buildDestinationOutflowDiagnosticCard: () => ({
      answer_card_complete: true,
      required_facts_present: false,
      delivery_state: "blocked"
    }),
    buildViaOneHopDiagnosticCard: () => ({
      answer_card_complete: true,
      required_facts_present: false,
      delivery_state: "blocked"
    }),
    buildViaContinuationDiagnosticCard: () => ({
      answer_card_complete: true,
      required_facts_present: false,
      delivery_state: "blocked"
    }),
    buildTopRankingsDiagnosticCard: () => ({
      answer_card_complete: true,
      required_facts_present: false,
      delivery_state: "blocked"
    }),
    buildClaimReviewDiagnosticCard: async () => ({
      answer_card_complete: true,
      required_facts_present: false,
      delivery_state: "blocked"
    }),
    buildInvestigationLabDiagnosticCard: async () => ({
      answer_card_complete: true,
      required_facts_present: false,
      delivery_state: "blocked"
    })
  });
}

function workbench(records) {
  return {
    ok: true,
    data: {
      execution_status: "executed",
      records
    }
  };
}

async function runNameQuickFact(record) {
  const runtime = makeRuntime({ skillHandlers: { run_case_sql: workbench([record]) } });
  return runtime.fundsInvestigate({
    intent: "name_association_quick_fact",
    question: "张三有没有案件资金关联",
    focus_keywords: ["张三"]
  });
}

const invalidName = await runNameQuickFact({
  target: "张三",
  strict_txn_count: false,
  strict_amount: { value: {} },
  fuzzy_matches: []
});
assert.equal(invalidName.status, "partial");
assert.equal(invalidName.answer_card.delivery_state, "blocked");
assert.equal(invalidName.answer_card.required_facts_present, false);
assert.equal(Object.hasOwn(invalidName.answer_card.strict_rows[0], "strict_txn_count"), false);
assert.equal(Object.hasOwn(invalidName.answer_card.strict_rows[0], "strict_amount"), false);

const nonFiniteName = await runNameQuickFact({
  target: "张三",
  strict_txn_count: Infinity,
  strict_amount: " ",
  fuzzy_matches: []
});
assert.equal(nonFiniteName.answer_card.delivery_state, "blocked");
assert.equal(Object.hasOwn(nonFiniteName.answer_card.strict_rows[0], "strict_txn_count"), false);

const zeroName = await runNameQuickFact({
  target: "张三",
  strict_txn_count: 0,
  strict_amount: 0,
  coverage_complete: true,
  fuzzy_matches: []
});
assert.equal(zeroName.answer_card.strict_rows[0].strict_txn_count, 0);
assert.equal(zeroName.answer_card.strict_rows[0].strict_amount, 0);
assert.equal(zeroName.answer_card.delivery_state, "blocked");
assert.equal(zeroName.answer_card.required_facts_present, false);
assert.deepEqual(zeroName.answer_card.cannot_confirm, []);

const positiveName = await runNameQuickFact({
  target: "张三",
  strict_txn_count: 2,
  strict_amount: 100,
  fuzzy_matches: []
});
assert.equal(positiveName.status, "ok");
assert.equal(positiveName.answer_card.delivery_state, "quick_fact_ready");
assert.equal(positiveName.answer_card.required_facts_present, true);

async function runFinancialProduct(record) {
  const runtime = makeRuntime({
    skillHandlers: {
      run_case_sql: workbench([record]),
      inspect_case_schema: successfulSkill("inspect_case_schema", {}, {
        schema_status: "available",
        tables: [{ table_name: "analysis_txn_detail_idx" }]
      })
    }
  });
  return runtime.fundsInvestigate({
    intent: "financial_product_topic",
    holder_name: "张三",
    question: "核算张三基金证券情况"
  });
}

const zeroFinancial = await runFinancialProduct({
  subject_account_count: 0,
  subject_accounts: [],
  buckets: []
});
assert.equal(zeroFinancial.answer_card.subject_account_count, 0);
assert.equal(zeroFinancial.answer_card.delivery_state, "blocked");
assert.equal(zeroFinancial.status, "partial");

const completeFinancial = await runFinancialProduct({
  subject_account_count: 1,
  subject_accounts: [{ account_key: "acct-001", open_name: "张三" }],
  buckets: [{ bucket: "基金申赎", direction: "出", txn_count: 1, amount: 100 }]
});
assert.equal(completeFinancial.answer_card.delivery_state, "topic_review_ready");
assert.equal(completeFinancial.answer_card.required_facts_present, true);

async function runCompanyToPerson(record) {
  const runtime = makeRuntime({ skillHandlers: { run_case_sql: workbench([record]) } });
  return runtime.fundsInvestigate({
    intent: "company_to_person_recompute",
    question: "对公转个人专项重算"
  });
}

const zeroCompany = await runCompanyToPerson({
  total_txn_count: 0,
  total_amount: 0,
  company_subject_count: 0,
  personal_subject_count: 0,
  summary_missing_txn_count: 0,
  summary_missing_amount: 0,
  nature_buckets: []
});
assert.equal(zeroCompany.answer_card.total_txn_count, 0);
assert.equal(zeroCompany.answer_card.total_amount, 0);
assert.equal(zeroCompany.answer_card.delivery_state, "blocked");

const completeCompany = await runCompanyToPerson({
  total_txn_count: 2,
  total_amount: 200,
  company_subject_count: 1,
  personal_subject_count: 1,
  summary_missing_txn_count: 0,
  summary_missing_amount: 0,
  nature_buckets: [{ nature_bucket: "其他", txn_count: 2, amount: 200 }]
});
assert.equal(completeCompany.answer_card.delivery_state, "controlled_review_ready");
assert.equal(completeCompany.answer_card.summary_missing_txn_count, 0);

const completePairEligibility = {
  coverage_status: "complete",
  blocker: "",
  same_fact_eligibility: {
    contract: "SameFactEligibilityV1",
    key_version: "analytix.same-fact-dedupe-key/v2",
    coverage_status: "complete",
    requested_row_count: 2,
    fact_eligible_row_count: 2,
    family_eligible_row_count: 2,
    rejected_row_count: 0,
    rejection_reason_histogram: {},
    blocker: ""
  },
  scope: {
    scope_mode: "same_holder_accounts",
    holder_name: "甲方"
  }
};

async function runPairAmount(record, sameFactReview) {
  const runtime = makeRuntime({
    skillHandlers: {
      rank_counterparties: { ok: true, data: { rankings: [] } },
      run_case_sql: workbench([record]),
      ...(sameFactReview ? { resolve_duplicate_families: { ok: true, data: sameFactReview } } : {})
    }
  });
  return runtime.fundsInvestigate({
    intent: "destination",
    holder_name: "甲方",
    via_holder_name: "乙方",
    question: "甲方转给乙方多少钱？"
  });
}

const invalidPair = await runPairAmount({
  raw_detail_count: false,
  raw_detail_amount: {},
  effective_fact_count: "",
  effective_dedup_amount: Infinity
});
assert.equal(invalidPair.status, "partial");
assert.equal(invalidPair.answer_card.delivery_state, "blocked");
assert.equal(invalidPair.answer_card.required_facts_present, false);

const zeroPair = await runPairAmount({
  raw_detail_count: 0,
  raw_detail_amount: 0,
  effective_fact_count: 0,
  effective_dedup_amount: 0,
  confirmed_same_fact_duplicate_amount: 0,
  confirmed_same_fact_duplicate_extra_count: 0,
  source_account_clusters: [],
  counterparty_account_clusters: [],
  date_counterparty_clusters: [],
  duplicate_groups: [],
  top_transactions: [],
  effective_top_transactions: []
});
assert.equal(zeroPair.answer_card.delivery_state, "blocked");
assert.equal(zeroPair.answer_card.required_facts_present, false);

const completePairWithoutEligibility = await runPairAmount({
  raw_detail_count: 2,
  raw_detail_amount: 100,
  effective_fact_count: 2,
  effective_dedup_amount: 100,
  duplicate_or_unsupported_amount: 0,
  confirmed_same_fact_duplicate_amount: 0,
  confirmed_same_fact_duplicate_extra_count: 0,
  first_txn_at: "2026-01-01",
  last_txn_at: "2026-01-02",
  source_account_clusters: [],
  counterparty_account_clusters: [],
  date_counterparty_clusters: [],
  duplicate_groups: [],
  top_transactions: [],
  effective_top_transactions: []
});
assert.equal(completePairWithoutEligibility.status, "partial");
assert.equal(completePairWithoutEligibility.answer_card.delivery_state, "blocked");
assert.equal(completePairWithoutEligibility.answer_card.required_facts_present, false);
assert.equal(Object.hasOwn(completePairWithoutEligibility.answer_card, "confirmed_same_fact_duplicate_amount"), false);
assert.equal(Object.hasOwn(completePairWithoutEligibility.answer_card, "duplicate_review_status"), false);

const completePair = await runPairAmount({
  raw_detail_count: 2,
  raw_detail_amount: 100,
  effective_fact_count: 2,
  effective_dedup_amount: 100,
  duplicate_or_unsupported_amount: 0,
  confirmed_same_fact_duplicate_amount: 0,
  confirmed_same_fact_duplicate_extra_count: 0,
  first_txn_at: "2026-01-01",
  last_txn_at: "2026-01-02",
  source_account_clusters: [],
  counterparty_account_clusters: [],
  date_counterparty_clusters: [],
  duplicate_groups: [],
  top_transactions: [],
  effective_top_transactions: []
}, completePairEligibility);
assert.equal(completePair.status, "ok");
assert.equal(completePair.answer_card.delivery_state, "mini_review_ready");
assert.equal(completePair.answer_card.confirmed_same_fact_duplicate_amount, 0);
assert.equal(completePair.answer_card.duplicate_review_status, "zero_difference_in_checked_scope");
assert.notEqual(completePair.answer_card.duplicate_review_status, "no_duplicate_difference");

async function runTransferGraph(edge) {
  const runtime = makeRuntime({
    skillHandlers: {
      rank_counterparties: { ok: true, data: { rankings: [] } },
      run_case_sql: { ok: false, data: {}, warnings: [] }
    },
    flowGraph: {
      tool: "build_fund_flow_graph",
      status: "ok",
      flow_graph: { edges: [edge] },
      answer_card: {},
      key_facts: {},
      warnings: []
    }
  });
  return runtime.fundsInvestigate({
    intent: "flow_graph",
    holder_name: "甲方",
    via_holder_name: "乙方",
    question: "画甲方转给乙方的资金流向图"
  });
}

for (const amount of [undefined, false, [], {}, { value: false }, Infinity, 0]) {
  const graph = await runTransferGraph({
    edge_status: "supported",
    from_label: "甲方",
    to_label: "乙方",
    amount
  });
  assert.equal(graph.answer_card.delivery_state, "blocked");
  assert.equal(graph.answer_card.required_facts_present, false);
  assert.equal(graph.answer_card.boundary_edges[0].edge_status, "unresolved");
  if (amount === 0) assert.equal(graph.answer_card.boundary_edges[0].amount, 0);
}
const completeGraph = await runTransferGraph({
  edge_status: "supported",
  from_label: "甲方",
  to_label: "乙方",
  amount: 100,
  txn_time: "2026-01-01"
});
assert.equal(completeGraph.answer_card.delivery_state, "graph_ready");
assert.equal(completeGraph.answer_card.supported_edges[0].amount, 100);

async function runBroadGraph(row) {
  const runtime = makeRuntime({
    skillHandlers: {
      rank_counterparties: { ok: true, data: { rankings: [row] } }
    },
    casegraph: {
      tool: "get_casegraph",
      status: "ok",
      key_facts: {},
      warnings: [],
      evidence_refs: {}
    }
  });
  return runtime.fundsInvestigate({ intent: "flow_graph", question: "展示当前案件资金流向图" });
}
const zeroBroadGraph = await runBroadGraph({ display_name: "乙方", txn_count: 0, turnover_total: 0 });
assert.equal(zeroBroadGraph.status, "partial");
assert.equal(zeroBroadGraph.answer_card.delivery_state, "blocked");
const completeBroadGraph = await runBroadGraph({ display_name: "乙方", txn_count: 2, turnover_total: 100 });
assert.equal(completeBroadGraph.status, "ok");
assert.equal(completeBroadGraph.answer_card.delivery_state, "graph_table_ready");

async function runRanking(row) {
  const runtime = makeRuntime({
    skillHandlers: {
      rank_accounts: { ok: true, data: { rankings: [row] } },
      rank_holders: { ok: true, data: { rankings: [row] } },
      rank_counterparties: { ok: true, data: { rankings: [row] } }
    }
  });
  return runtime.fundsInvestigate({ intent: "ranking", question: "账户资金排名" });
}

const zeroRanking = await runRanking({ account_key: "acct-001", txn_count: 0, turnover_total: 0 });
assert.equal(zeroRanking.status, "partial");
assert.equal(zeroRanking.answer_card.required_facts_present, false);
const completeRanking = await runRanking({ account_key: "acct-001", txn_count: 2, turnover_total: 100 });
assert.equal(completeRanking.status, "ok");
assert.equal(completeRanking.answer_card.required_facts_present, true);
assert.equal(Object.hasOwn(completeRanking.answer_card, "blocking_gate_count"), false);
const emptyContextRuntime = makeRuntime({
  skillHandlers: {
    rank_accounts: { ok: true, data: { rankings: [{ account_key: "acct-001", txn_count: 2, turnover_total: 100 }] } },
    rank_holders: { ok: true, data: { rankings: [{ holder_name: "张三", txn_count: 2, turnover_total: 100 }] } },
    rank_counterparties: { ok: true, data: { rankings: [{ display_name: "乙方", txn_count: 2, turnover_total: 100 }] } }
  },
  casegraph: { status: "ok", key_facts: { gates: [] }, warnings: [], evidence_refs: {} }
});
const emptyContextRanking = await emptyContextRuntime.fundsInvestigate({
  intent: "ranking",
  question: "账户资金排名",
  include_casegraph_context: true
});
assert.equal(emptyContextRanking.status, "partial");
assert.equal(emptyContextRanking.answer_card.casegraph_context_ready, false);

const completeQuality = {
  import_lineage: { summary: { file_log_count: 1, rows_total: 2, rows_imported_norm: 2, rows_dedup: 0 } },
  cleaning_quality: {
    flags: { txn_total: 2 },
    duplicate_summary: { same_fact_cross_account_groups: 0, same_fact_cross_account_extra_rows: 0 },
    same_fact_eligibility: {
      contract: "SameFactEligibilityV1",
      key_version: "analytix.same-fact-dedupe-key/v2",
      coverage_status: "complete",
      requested_row_count: 2,
      fact_eligible_row_count: 2,
      family_eligible_row_count: 2,
      rejected_row_count: 0,
      rejection_reason_histogram: {},
      blocker: ""
    }
  },
  account_identity_quality: { account_count: 1, unregistered_account_count: 0 }
};
const completeDuplicate = {
  coverage_status: "complete",
  blocker: "",
  same_fact_eligibility: {
    contract: "SameFactEligibilityV1",
    key_version: "analytix.same-fact-dedupe-key/v2",
    coverage_status: "complete",
    requested_row_count: 2,
    fact_eligible_row_count: 2,
    family_eligible_row_count: 2,
    rejected_row_count: 0,
    rejection_reason_histogram: {},
    blocker: ""
  },
  summary: {
    same_holder_same_fact: { group_count: 0, extra_rows: 0 },
    full_case_review_only: { cross_holder_group_count: 0 }
  },
  families: [],
  dedupe_policy: { full_case_blind_dedupe_allowed: false, deduct_from_totals_allowed: false }
};
async function runQa(quality, duplicate) {
  const runtime = makeRuntime({
    skillHandlers: {
      audit_case_data_quality: { ok: true, data: quality },
      resolve_duplicate_families: { ok: true, data: duplicate }
    }
  });
  return runtime.fundsInvestigate({ intent: "qa_review", question: "复核同事实重复与数据质量" });
}
const emptyQa = await runQa({}, {});
assert.equal(emptyQa.status, "partial");
assert.equal(emptyQa.answer_card.required_facts_present, false);
assert.equal(emptyQa.key_facts.qa_review, undefined);
const completeQa = await runQa(completeQuality, completeDuplicate);
assert.equal(completeQa.status, "ok");
assert.equal(completeQa.answer_card.required_facts_present, true);
const partialQa = await runQa(
  {
    ...completeQuality,
    cleaning_quality: {
      ...completeQuality.cleaning_quality,
      same_fact_eligibility: {
        ...completeQuality.cleaning_quality.same_fact_eligibility,
        coverage_status: "partial",
        family_eligible_row_count: 1,
        rejected_row_count: 1,
        rejection_reason_histogram: { amount_missing_or_invalid: 1 },
        blocker: "same_fact_eligibility_incomplete"
      }
    }
  },
  completeDuplicate
);
assert.equal(partialQa.status, "partial");
assert.equal(partialQa.answer_card.required_facts_present, false);
assert.equal(partialQa.key_facts.qa_review, undefined);
const emptyDuplicateQa = await runQa(completeQuality, {
  ...completeDuplicate,
  coverage_status: "empty_unverified",
  blocker: "same_fact_scope_empty_unverified",
  same_fact_eligibility: {
    ...completeDuplicate.same_fact_eligibility,
    coverage_status: "empty_unverified",
    requested_row_count: 0,
    fact_eligible_row_count: 0,
    family_eligible_row_count: 0,
    blocker: "same_fact_scope_empty_unverified"
  },
  summary: {
    same_holder_same_fact: { group_count: null, extra_rows: null },
    full_case_review_only: { cross_holder_group_count: null }
  }
});
assert.equal(emptyDuplicateQa.status, "partial");
assert.equal(emptyDuplicateQa.answer_card.required_facts_present, false);
assert.equal(emptyDuplicateQa.key_facts.qa_review, undefined);

async function runHolder({ stats, scope, ranking, coverage = { txn_total: 2, txn_analyzed: 2, account_count: 1, date_min: "2026-01-01", date_max: "2026-01-02" } }) {
  const runtime = makeRuntime({
    skillHandlers: {
      analyze_holder_full: {
        ok: true,
        data: {
          holder_scope: scope,
          account_analysis: {
            account_stats: { summary: stats },
            coverage
          }
        }
      },
      rank_accounts: { ok: true, data: { rankings: [ranking] } }
    }
  });
  return runtime.fundsInvestigate({ intent: "holder_analysis", holder_name: "张三", question: "研判张三名下账户" });
}
const zeroHolder = await runHolder({
  stats: { txn_count: 0, turnover_total: 0 },
  scope: { account_count: 1, account_keys_preview: ["acct-001"] },
  ranking: { account_key: "acct-001", txn_count: 0, turnover_total: 0 }
});
assert.equal(zeroHolder.status, "partial");
assert.equal(zeroHolder.answer_card.delivery_state, "blocked");
const completeHolder = await runHolder({
  stats: { txn_count: 2, turnover_total: 100 },
  scope: { account_count: 1, account_keys_preview: ["acct-001"] },
  ranking: { account_key: "acct-001", txn_count: 2, turnover_total: 100 }
});
assert.equal(completeHolder.status, "ok");
assert.equal(completeHolder.answer_card.delivery_state, "dossier_ready");
const partialCoverageHolder = await runHolder({
  stats: { txn_count: 2, turnover_total: 100 },
  scope: { account_count: 1, account_keys_preview: ["acct-001"] },
  ranking: { account_key: "acct-001", txn_count: 2, turnover_total: 100 },
  coverage: { txn_total: 2, txn_analyzed: 1, account_count: 1, date_min: "2026-01-01", date_max: "2026-01-02" }
});
assert.equal(partialCoverageHolder.answer_card.delivery_state, "blocked");

async function runAccount(stats, coverage = { txn_total: 2, txn_analyzed: 2, account_count: 1, date_min: "2026-01-01", date_max: "2026-01-02" }) {
  const runtime = makeRuntime({
    skillHandlers: {
      analyze_account_full: {
        ok: true,
        data: {
          account_analysis: {
            account_stats: { summary: stats },
            coverage
          }
        }
      },
      rank_counterparties: { ok: true, data: { rankings: [] } }
    }
  });
  return runtime.fundsInvestigate({ intent: "account_analysis", account_keys: ["acct-001"], question: "研判账户 acct-001" });
}
const zeroAccount = await runAccount({ txn_count: 0, turnover_total: 0 });
assert.equal(zeroAccount.status, "partial");
assert.equal(zeroAccount.answer_card.delivery_state, "blocked");
const completeAccount = await runAccount({ txn_count: 2, turnover_total: 100 });
assert.equal(completeAccount.status, "ok");
assert.equal(completeAccount.answer_card.delivery_state, "dossier_ready");
const partialCoverageAccount = await runAccount(
  { txn_count: 2, turnover_total: 100 },
  { txn_total: 2, txn_analyzed: 1, account_count: 1, date_min: "2026-01-01", date_max: "2026-01-02" }
);
assert.equal(partialCoverageAccount.answer_card.delivery_state, "blocked");

console.log("frontdoor external numeric contract passed");
