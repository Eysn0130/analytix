import { describe, expect, it } from "vitest";
import {
  completeImpactEstimate,
  compareKnownMetrics,
  exactDatasetIndex,
  importCompletionTitle,
  maxKnownMetric,
  projectUnverifiedHistoricalCounts,
  sumCompleteMetrics,
} from "./public-count-projection";

describe("import public count projection", () => {
  it("keeps explicit zero distinct from unresolved values", () => {
    expect(maxKnownMetric(0, null)).toBe(0);
    expect(maxKnownMetric(null, null)).toBeNull();
    expect(sumCompleteMetrics([0, 0])).toBe(0);
    expect(sumCompleteMetrics([0, null])).toBeNull();
    expect(sumCompleteMetrics([])).toBeNull();
  });

  it("sorts unresolved metrics last in both directions", () => {
    expect(compareKnownMetrics(null, 7, "asc")).toBeGreaterThan(0);
    expect(compareKnownMetrics(null, 7, "desc")).toBeGreaterThan(0);
    expect(compareKnownMetrics(3, 7, "asc")).toBeLessThan(0);
    expect(compareKnownMetrics(3, 7, "desc")).toBeGreaterThan(0);
  });

  it("never binds a same-name or same-path historical dataset without exact identity", () => {
    const datasets = [
      { dataset_id: "old-file" },
      { dataset_id: "file-alpha" },
    ];

    expect(exactDatasetIndex("file-alpha", datasets)).toBe(1);
    expect(exactDatasetIndex("same-name-only", datasets)).toBe(-1);
    expect(exactDatasetIndex("", datasets)).toBe(-1);
  });

  it("keeps a verified zero completion count and never invents an unresolved count", () => {
    expect(importCompletionTitle(0, "1 秒")).toBe("已成功导入 0 个文件，耗时：1 秒");
    expect(importCompletionTitle(null, "1 秒")).toBe("导入任务已完成，文件数未验证，耗时：1 秒");
    expect(importCompletionTitle("7", "1 秒")).toBe("导入任务已完成，文件数未验证，耗时：1 秒");
    expect(importCompletionTitle(true, "1 秒")).toBe("导入任务已完成，文件数未验证，耗时：1 秒");
  });

  it("never turns an unknown destructive-action impact into zero", () => {
    expect(completeImpactEstimate([null])).toEqual({
      status: "unknown",
      value: null,
      unknownItemCount: 1,
    });
    expect(completeImpactEstimate([12, null, 3])).toEqual({
      status: "unknown",
      value: null,
      unknownItemCount: 1,
    });
    expect(completeImpactEstimate([])).toEqual({
      status: "unknown",
      value: null,
      unknownItemCount: 0,
    });
    expect(completeImpactEstimate([Number.MAX_SAFE_INTEGER, 1])).toEqual({
      status: "unknown",
      value: null,
      unknownItemCount: 0,
    });
  });

  it("keeps an explicit complete zero scoped as a complete ledger estimate", () => {
    expect(completeImpactEstimate([0, 0])).toEqual({
      status: "complete",
      value: 0,
      unknownItemCount: 0,
    });
    expect(completeImpactEstimate([4, 6])).toEqual({
      status: "complete",
      value: 10,
      unknownItemCount: 0,
    });
  });

  it("does not publish historical row or column counts without a receipt-bound snapshot", () => {
    expect(projectUnverifiedHistoricalCounts({ rows: null, cols: null })).toEqual({
      status: "unverified",
      rowsTotal: null,
      validRows: null,
      columnCount: null,
    });
    expect(projectUnverifiedHistoricalCounts({ rows: 0, cols: 0 })).toEqual({
      status: "unverified",
      rowsTotal: null,
      validRows: null,
      columnCount: null,
    });
    expect(projectUnverifiedHistoricalCounts({ rows: 500, cols: 18 })).toEqual({
      status: "unverified",
      rowsTotal: null,
      validRows: null,
      columnCount: null,
    });
  });
});
