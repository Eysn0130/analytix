import { describe, expect, it } from "vitest";

import type { StatsRowDTO, StatsTxnRowDTO } from "../api/stats-api";
import {
  buildDistributionItems,
  buildIpItems,
  buildRemarkKeywordItems,
  buildSummaryKeywordItems,
  buildTxnTypeItems,
  getMetricItemValue,
} from "./stats-dashboard-view-model";
import {
  resolveStatsFlowExpectedRowAmount,
  resolveStatsFlowExpectedTotalAmount,
} from "./stats-flow-transfer-model";
import { normalizeSankeyChartItems } from "./stats-sankey-render-model";
import {
  getStatsTxnDirectionTone,
  summarizeStatsRowForTxnFooter,
  summarizeStatsTxnRows,
} from "./stats-txn-summary-model";
import { normalizeTrendPoints } from "./stats-trend-model";

function statsRow(totalAmount: unknown): StatsRowDTO {
  return { total_amount: totalAmount } as StatsRowDTO;
}

function txnRow(amount: unknown, direction: unknown, txnTime = "2026-01-01 10:00:00"): StatsTxnRowDTO {
  return {
    amount,
    dc_flag: direction,
    txn_time: txnTime,
  } as StatsTxnRowDTO;
}

function trendPoint(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    bucket: "2026-01-01",
    label: "2026-01-01",
    in_amount: 0,
    out_amount: 0,
    net_amount: 0,
    total_amount: 0,
    in_count: 0,
    out_count: 0,
    net_count: 0,
    total_count: 0,
    ...overrides,
  };
}

describe("stats missing-value models", () => {
  it("keeps a selected flow total unknown when any row amount is missing, null, or invalid", () => {
    expect(resolveStatsFlowExpectedRowAmount(statsRow(undefined), "relation")).toBeNull();
    expect(resolveStatsFlowExpectedRowAmount(statsRow(null), "relation")).toBeNull();
    expect(resolveStatsFlowExpectedRowAmount(statsRow("bad"), "relation")).toBeNull();
    expect(resolveStatsFlowExpectedTotalAmount([], "relation")).toBeNull();
    expect(resolveStatsFlowExpectedTotalAmount([statsRow(10), statsRow(null)], "relation")).toBeNull();
    expect(resolveStatsFlowExpectedTotalAmount([statsRow(0), statsRow(2)], "relation")).toBe(2);
  });

  it("omits incomplete dashboard metrics while preserving explicit numeric zero", () => {
    expect(getMetricItemValue({}, "amount")).toBeNull();
    expect(getMetricItemValue({ total_amount: null, amount: 7 }, "amount")).toBeNull();
    expect(getMetricItemValue({ total_amount: "bad" }, "amount")).toBeNull();
    expect(getMetricItemValue({ total_amount: 0 }, "amount")).toBe(0);

    expect(buildDistributionItems([
      { label: "missing" },
      { label: "null", total_amount: null },
      { label: "invalid", total_amount: "bad" },
      { label: "zero", total_amount: 0, total_count: 0 },
    ])).toEqual([{ label: "zero", rawValue: "zero", value: 0, amount: 0, count: 0 }]);

    expect(buildTxnTypeItems([
      { label: "invalid", amount: null, count: 1 },
      { label: "zero", amount: 0, count: 0 },
    ], "amount")).toEqual([{ label: "zero", rawValue: "zero", value: 0, amount: 0, count: 0 }]);

    expect(buildIpItems([
      { label: "invalid", amount: Number.NaN, count: 1 },
      { label: "zero", amount: 0, count: 0 },
    ])).toEqual([expect.objectContaining({ rawValue: "zero", value: 0, amount: 0, count: 0 })]);

    expect(buildSummaryKeywordItems([
      { label: "missing" },
      { label: "invalid", count: 1.5 },
      { label: "zero", count: 0 },
    ])).toEqual([{ label: "zero", rawValue: "zero", value: 0, count: 0 }]);

    expect(buildRemarkKeywordItems([
      { label: "null", count: null },
      { label: "invalid", count: "bad" },
      { label: "zero", count: 0 },
    ])).toEqual([{ label: "zero", count: 0 }]);
  });

  it("omits trend rows with any unresolved metric and retains an all-zero verified row", () => {
    expect(normalizeTrendPoints([
      trendPoint({ in_amount: undefined }),
      trendPoint({ out_amount: null }),
      trendPoint({ total_count: "bad" }),
      trendPoint(),
    ], "day")).toEqual([expect.objectContaining({
      inAmount: 0,
      outAmount: 0,
      netAmount: 0,
      totalAmount: 0,
      inCount: 0,
      outCount: 0,
      netCount: 0,
      totalCount: 0,
    })]);
  });

  it("omits incomplete Sankey rows and preserves an explicit zero amount and count", () => {
    expect(normalizeSankeyChartItems([
      { label: "missing" },
      { label: "null", amount: null, count: 1 },
      { label: "invalid", amount: "bad", count: 1 },
      { label: "fractional-count", amount: 1, count: 1.5 },
      { label: "zero", amount: 0, count: 0 },
    ])).toEqual([{
      label: "zero",
      amount: 0,
      count: 0,
      firstTime: "",
      lastTime: "",
    }]);
  });

  it("keeps unknown transaction directions unresolved instead of counting them as inflow", () => {
    expect(getStatsTxnDirectionTone(undefined)).toBe("unknown");
    expect(getStatsTxnDirectionTone("unknown")).toBe("unknown");
    expect(getStatsTxnDirectionTone("进")).toBe("in");
    expect(getStatsTxnDirectionTone("出")).toBe("out");

    expect(summarizeStatsTxnRows([])).toMatchObject({
      totalCount: null,
      inCount: null,
      outCount: null,
      inAmount: null,
      outAmount: null,
    });
    expect(summarizeStatsTxnRows([txnRow(null, "进")])).toMatchObject({
      totalCount: 1,
      inCount: 1,
      outCount: null,
      inAmount: null,
      outAmount: null,
    });
    expect(summarizeStatsTxnRows([txnRow("bad", "进")])).toMatchObject({
      inCount: 1,
      inAmount: null,
    });
    expect(summarizeStatsTxnRows([txnRow(8, "unknown")])).toMatchObject({
      totalCount: 1,
      inCount: null,
      outCount: null,
      inAmount: null,
      outAmount: null,
    });
    expect(summarizeStatsTxnRows([txnRow(0, "进")])).toMatchObject({
      totalCount: 1,
      inCount: 1,
      inAmount: 0,
    });
  });

  it("keeps missing active-row aggregates nullable and preserves explicit zeros", () => {
    expect(summarizeStatsRowForTxnFooter({
      total_count: null,
      in_count: undefined,
      out_count: "bad",
      in_amount: null,
      out_amount: Number.NaN,
    } as unknown as StatsRowDTO)).toMatchObject({
      totalCount: null,
      inCount: null,
      outCount: null,
      inAmount: null,
      outAmount: null,
    });
    expect(summarizeStatsRowForTxnFooter({
      total_count: 0,
      in_count: 0,
      out_count: 0,
      in_amount: 0,
      out_amount: 0,
    } as StatsRowDTO)).toMatchObject({
      totalCount: 0,
      inCount: 0,
      outCount: 0,
      inAmount: 0,
      outAmount: 0,
    });
  });
});
