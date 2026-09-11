import { describe, expect, it } from "vitest";

import {
  normalizeBalanceTrendPoints,
  normalizeHeatmapChartCells,
  normalizeHistogramChartItems,
} from "../components/charts/basic/chart-render-utils";
import { formatChartDetailMoney } from "./chart-detail-model";
import {
  UNKNOWN_CASE_FACT_LABEL,
  formatCaseFactCount,
  formatCaseFactMoney,
  formatCaseFactPercentage,
  readCaseFactBoolean,
  readFiniteCaseFactNumber,
  readNonNegativeCaseFactInteger,
} from "./stats-case-fact-number-model";
import { prepareRemarkKeywordCloudItems } from "./stats-keyword-cloud-model";
import { normalizeStatsRowsSummary } from "./stats-rows-summary-model";

describe("stats chart case-fact numeric boundary", () => {
  it("accepts explicit numeric zero and rejects coercible, missing, boolean, and non-finite values", () => {
    expect(readFiniteCaseFactNumber(0)).toBe(0);
    expect(readNonNegativeCaseFactInteger(0)).toBe(0);

    for (const value of [undefined, null, "", "0", false, true, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(readFiniteCaseFactNumber(value)).toBeNull();
      expect(readNonNegativeCaseFactInteger(value)).toBeNull();
    }

    expect(readCaseFactBoolean(false)).toBe(false);
    expect(readCaseFactBoolean(true)).toBe(true);
    expect(readCaseFactBoolean(0)).toBeNull();
    expect(readCaseFactBoolean("false")).toBeNull();
  });

  it("formats unknown facts with one fixed marker without disguising explicit zero", () => {
    expect(formatCaseFactMoney(null)).toBe(UNKNOWN_CASE_FACT_LABEL);
    expect(formatCaseFactMoney(false)).toBe(UNKNOWN_CASE_FACT_LABEL);
    expect(formatCaseFactMoney(Number.NaN)).toBe(UNKNOWN_CASE_FACT_LABEL);
    expect(formatCaseFactCount("0")).toBe(UNKNOWN_CASE_FACT_LABEL);
    expect(formatCaseFactPercentage(undefined)).toBe(UNKNOWN_CASE_FACT_LABEL);
    expect(formatCaseFactPercentage(101)).toBe(UNKNOWN_CASE_FACT_LABEL);

    expect(formatCaseFactMoney(0)).toBe("¥0.00");
    expect(formatCaseFactCount(0)).toBe("0");
    expect(formatCaseFactPercentage(0)).toBe("0%");
    expect(formatChartDetailMoney(false)).toBe(UNKNOWN_CASE_FACT_LABEL);
  });

  it("drops incomplete heatmap cells instead of summing missing values as zero", () => {
    expect(normalizeHeatmapChartCells([
      { weekday: 0, hour: 0, count: 0 },
      { weekday: 1, hour: 1, count: null, in_count: 2, out_count: 3 },
      { weekday: 2, hour: 2, in_count: 2 },
      { weekday: false, hour: 3, count: 4 },
      { weekday: 7, hour: 3, count: 4 },
      { weekday: 3, hour: 24, count: 4 },
      { weekday: 4, hour: 4, in_count: 2, out_count: 3 },
    ], "count")).toEqual([
      { weekday: 0, hour: 0, value: 0 },
      { weekday: 4, hour: 4, value: 5 },
    ]);

    expect(normalizeHeatmapChartCells([
      { weekday: 0, hour: 0, total_amount: 0 },
      { weekday: 1, hour: 1, value: null, in_amount: 2, out_amount: 3 },
      { weekday: 2, hour: 2, in_amount: 2, out_amount: 3 },
      { weekday: 3, hour: 3, in_amount: false, out_amount: 3 },
    ], "amount")).toEqual([
      { weekday: 0, hour: 0, value: 0 },
      { weekday: 2, hour: 2, value: 5 },
    ]);
  });

  it("drops invalid histogram and balance data while retaining verified zero data", () => {
    expect(normalizeHistogramChartItems([
      { label: "zero", count: 0 },
      { label: "missing" },
      { label: "null", count: null },
      { label: "bool", count: false },
      { label: "fractional", count: 1.5 },
      { label: "non-finite", count: Number.POSITIVE_INFINITY },
    ])).toEqual([{ label: "zero", count: 0 }]);

    expect(normalizeBalanceTrendPoints([
      { bucket: "2026-01-01", label: "zero", balance: 0 },
      { bucket: "2026-01-02", balance: null },
      { bucket: "2026-01-03", balance: false },
      { bucket: "2026-01-04", balance: Number.NaN },
    ], "balance")).toEqual([{ bucket: "2026-01-01", label: "zero", value: 0 }]);
  });

  it("drops invalid keyword counts and preserves an explicit zero count", () => {
    expect(prepareRemarkKeywordCloudItems([
      { label: "missing" },
      { label: "null", count: null },
      { label: "bool", count: false },
      { label: "string", count: "0" },
      { label: "fractional", count: 1.5 },
      { label: "zero", count: 0 },
    ])).toEqual([{ label: "zero", count: 0 }]);
  });

  it("keeps row summary totals unknown unless their native numeric types are valid", () => {
    expect(normalizeStatsRowsSummary({ total_amount: false, total_count: "0" })).toEqual({
      totalAmount: null,
      totalCount: null,
    });
    expect(normalizeStatsRowsSummary({ total_amount: 0, total_count: 0 })).toEqual({
      totalAmount: 0,
      totalCount: 0,
    });
  });
});
