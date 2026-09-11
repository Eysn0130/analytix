#!/usr/bin/env node

import assert from "node:assert/strict";

import { createAgentOutputCompiler } from "../mcp/agent-output-compiler.mjs";
import { compactToolPayloadForAgent } from "../mcp/agent-payload-compiler.mjs";
import {
  PUBLICATION_RECEIPT_REQUIRED_CODE,
  PUBLICATION_RECEIPT_REQUIRED_TEXT,
} from "../mcp/report-publication-guard.mjs";

const compiler = createAgentOutputCompiler({});

function fullCasePayload(status, data) {
  return {
    tool: "run_full_case_analysis",
    response: {
      data: {
        skill_id: "run_full_case_analysis",
        status,
        data,
      },
    },
  };
}

function numericZeroPaths(value, path = "$") {
  if (typeof value === "number") return value === 0 ? [path] : [];
  if (Array.isArray(value)) {
    return value.flatMap((item, index) =>
      numericZeroPaths(item, `${path}[${index}]`),
    );
  }
  if (!value || typeof value !== "object") return [];
  return Object.entries(value).flatMap(([key, item]) =>
    numericZeroPaths(item, `${path}.${key}`),
  );
}

function assertNoRenderedWholeCaseZero(value, label) {
  const rendered = typeof value === "string" ? value : JSON.stringify(value);
  for (const pattern of [
    /(?:^|[^\d])0\.00\s*元/u,
    /(?:^|[^\d])0\s*笔/u,
    /(?:^|[^\d])0\s*个账户/u,
    /全案范围[^\n]*(?:0\s*笔|0\s*个账户)/u,
    /资金总量[^\n]*0(?:\.0+)?\s*元/u,
  ]) {
    assert.doesNotMatch(
      rendered,
      pattern,
      `${label} rendered a whole-case zero fact`,
    );
  }
}

const emptyPayload = fullCasePayload("ok", {
  semantic_status: "ok",
  coverage_complete: true,
  partial_coverage: false,
  dataset_snapshot_id: "snapshot-empty-v1",
  fact_answer_allowed: true,
  verified_no_hit: true,
  safeToAnswer: true,
  host_verified_no_hit_receipt_present: true,
  full_case_package: {
    coverage: {
      transaction_detail: {
        txn_count: 0,
        account_count: 0,
        counterparty_name_count: 0,
        inflow_amount: 0,
        outflow_amount: 0,
        turnover_amount: 0,
        first_txn_at: "2026-01-01",
        last_txn_at: "2026-01-31",
      },
    },
    holder_rankings: [],
    destination_outflows: [],
    source_inflows: [],
    top_holder_outflows: [],
    evidence_gaps: ["尚缺宿主 VerifiedNoHit Receipt。"],
  },
});

const compactEmpty = compactToolPayloadForAgent(emptyPayload);
assert.equal(compactEmpty.semantic_status, "ok");
assert.equal(compactEmpty.source_coverage_complete, true);
assert.equal(compactEmpty.source_dataset_snapshot_id, "snapshot-empty-v1");
assert.deepEqual(compactEmpty.evidence_gaps, [
  "尚缺宿主 VerifiedNoHit Receipt。",
]);

const emptyStructured = compiler.compactStructuredContent(emptyPayload, {
  write_report: false,
});
assert.equal(emptyStructured.status, "blocked");
assert.equal(emptyStructured.semantic_status, "blocked");
assert.equal(emptyStructured.error_code, PUBLICATION_RECEIPT_REQUIRED_CODE);
assert.equal(emptyStructured.reason, PUBLICATION_RECEIPT_REQUIRED_TEXT);
assert.equal(emptyStructured.isError, true);
assert.equal(emptyStructured.safeToAnswer, false);
assert.equal(emptyStructured.write_blocked, true);
assert.equal(
  JSON.stringify(emptyStructured).includes("snapshot-empty-v1"),
  false,
);
assert.deepEqual(
  numericZeroPaths(emptyStructured),
  [],
  "EmptyResultIsNotZero: quarantined payload must not expose zero-valued case facts",
);
const emptyText = compiler.compactToolText(emptyPayload, {
  write_report: false,
});
assert.equal(emptyText, PUBLICATION_RECEIPT_REQUIRED_TEXT);
assertNoRenderedWholeCaseZero(emptyText, "EmptyResultIsNotZero/text");
assertNoRenderedWholeCaseZero(
  emptyStructured,
  "EmptyResultIsNotZero/structured",
);

const partialPayload = fullCasePayload("partial", {
  semantic_status: "ok",
  coverage_status: "partial",
  coverage_complete: false,
  partial_coverage: true,
  dataset_snapshot_id: "snapshot-partial-v2",
  fact_answer_allowed: true,
  safeToAnswer: true,
  full_case_package: {
    coverage: {
      transaction_detail: {
        txn_count: 2,
        account_count: 1,
        counterparty_name_count: 0,
        inflow_amount: 0,
        outflow_amount: 0,
        turnover_amount: 0,
        first_txn_at: "2026-02-01",
        last_txn_at: "2026-02-29",
      },
    },
    holder_rankings: [
      {
        rank: 1,
        name: "仅覆盖主体甲",
        txn_count: 2,
        account_count: 0,
        amount: 0,
      },
    ],
    destination_outflows: [],
    source_inflows: [],
    top_holder_outflows: [],
    evidence_gaps: ["仅覆盖一家主体和一个月。", "其他账户与时间范围未检查。"],
  },
});

const compactPartial = compactToolPayloadForAgent(partialPayload);
assert.equal(compactPartial.semantic_status, "partial");
assert.equal(compactPartial.coverage_status, "partial");
assert.equal(compactPartial.source_coverage_complete, false);
assert.equal(compactPartial.partial_coverage, true);
assert.equal(compactPartial.source_dataset_snapshot_id, "snapshot-partial-v2");
assert.deepEqual(compactPartial.evidence_gaps, [
  "仅覆盖一家主体和一个月。",
  "其他账户与时间范围未检查。",
]);

const partialStructured = compiler.compactStructuredContent(partialPayload, {
  write_report: false,
});
assert.equal(partialStructured.status, "blocked");
assert.equal(partialStructured.semantic_status, "blocked");
assert.equal(partialStructured.error_code, PUBLICATION_RECEIPT_REQUIRED_CODE);
assert.equal(partialStructured.reason, PUBLICATION_RECEIPT_REQUIRED_TEXT);
assert.equal(partialStructured.isError, true);
assert.equal(partialStructured.safeToAnswer, false);
assert.equal(JSON.stringify(partialStructured).includes("仅覆盖主体甲"), false);
assert.equal(
  JSON.stringify(partialStructured).includes("snapshot-partial-v2"),
  false,
);
assert.deepEqual(
  numericZeroPaths(partialStructured),
  [],
  "PartialCoverageCannotBecomeWholeCaseConclusion: quarantined payload must not expose zero-valued facts",
);
const partialText = compiler.compactToolText(partialPayload, {
  write_report: false,
});
assert.equal(partialText, PUBLICATION_RECEIPT_REQUIRED_TEXT);
assertNoRenderedWholeCaseZero(
  partialText,
  "PartialCoverageCannotBecomeWholeCaseConclusion/text",
);
assertNoRenderedWholeCaseZero(
  partialStructured,
  "PartialCoverageCannotBecomeWholeCaseConclusion/structured",
);

process.stdout.write(
  `${JSON.stringify(
    {
      status: "ok",
      checks: [
        "EmptyResultIsNotZero",
        "PartialCoverageCannotBecomeWholeCaseConclusion",
        "full-case material is quarantined until the host issues a PublicationReceipt",
        "caller-reported no-hit/readiness never authorizes a case fact",
        "exact-count behavior remains covered by count-case-rows-contract-smoke",
      ],
    },
    null,
    2,
  )}\n`,
);
