import { describe, expect, it } from "vitest";

import { normalizeStatsRowsResult } from "./stats-rows-query-model";
import { normalizeStatsRowsSummary } from "./stats-rows-summary-model";
import { normalizeStatsTxnRowsResult } from "./stats-txn-query-model";

const hostileFacts = {
  account_no: "6222020000000000",
  amount: 123456.78,
  mac_addr: "AA:BB:CC:DD:EE:FF",
};

describe("direct stats evidence boundary", () => {
  it("rejects self-reported rows authority and preserves unknown totals", () => {
    const result = normalizeStatsRowsResult({
      contract: "StatsRowsPublicBoundaryV1",
      semantic_status: "verified",
      fact_answer_allowed: true,
      raw_details_exposed: true,
      rows: [hostileFacts],
      total: 1,
      row_summary: { total_amount: hostileFacts.amount, total_count: 1 },
    });

    expect(result).toMatchObject({
      factAnswerAllowed: false,
      rows: [],
      rowSummary: {},
      total: null,
    });
    expect(JSON.stringify(result)).not.toContain(hostileFacts.account_no);
    expect(JSON.stringify(result)).not.toContain(hostileFacts.mac_addr);
  });

  it("rejects self-reported transaction authority and cannot create a no-hit", () => {
    const result = normalizeStatsTxnRowsResult({
      contract: "StatsTxnRowsPublicBoundaryV1",
      semantic_status: "verified",
      fact_answer_allowed: true,
      raw_details_exposed: true,
      rows: [hostileFacts],
      done: true,
      next_cursor: { offset: 1 },
    });

    expect(result).toEqual({
      factAnswerAllowed: false,
      semanticStatus: "blocked",
      blocker: "host_evidence_receipt_required",
      rows: [],
      done: false,
      nextCursor: null,
    });
  });

  it("keeps a missing summary distinct from numeric zero", () => {
    expect(normalizeStatsRowsSummary({})).toEqual({ totalAmount: null, totalCount: null });
    expect(normalizeStatsRowsSummary({ total_amount: 0, total_count: 0 })).toEqual({ totalAmount: 0, totalCount: 0 });
  });
});
