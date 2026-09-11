import { afterEach, describe, expect, it } from "vitest";

import {
  getCachedStatsChartDetail,
  invalidateSharedStatsChartDetail,
  setCachedStatsChartDetail,
  sortCachedStatsChartDetailRows,
} from "./stats-chart-detail-resource";
import { loadAllStatsChartDetailRows } from "../../features/analysis/resources/stats-chart-detail-resource";
import type {
  StatsTxnRowDTO,
  StatsV2ChartDetailRowsDTO,
  StatsV2ChartDetailRowsRequest,
} from "./stats-api";

function row(id: string, amount: number | "", balance: number | "" = ""): StatsTxnRowDTO {
  return {
    id,
    txn_id: id,
    txn_time: "2026-07-20 10:00:00",
    amount,
    balance,
    counterparty_name: "",
  } as StatsTxnRowDTO;
}

function payload(limit = 2): StatsV2ChartDetailRowsRequest {
  return {
    case_id: "case-a",
    selected: [],
    date_start: "",
    date_end: "",
    metric_mode: "amount",
    direction_mode: "all",
    granularity: "day",
    success_filter: "all",
    cash_filter: "all",
    chart_filters: [],
    panel_views: [],
    sort_col: "txn_time",
    sort_dir: "asc",
    page: 1,
    limit,
    visible_columns: [],
  };
}

function detail(rows: StatsTxnRowDTO[], total: number | null): StatsV2ChartDetailRowsDTO {
  return {
    evidence_status: "verified",
    answer_card_complete: true,
    fact_answer_allowed: true,
    blocker: "",
    selection_mode: "multi-card",
    rows,
    total,
  };
}

describe("stats chart detail evidence boundary", () => {
  afterEach(() => invalidateSharedStatsChartDetail());

  it("keeps an unknown cache total unresolved while preserving observed zero", () => {
    setCachedStatsChartDetail("unknown", {
      rows: [],
      total: undefined as unknown as number,
      allRowsLoaded: false,
    });
    setCachedStatsChartDetail("zero", { rows: [], total: 0, allRowsLoaded: true });

    expect(getCachedStatsChartDetail("unknown")?.total).toBeNull();
    expect(getCachedStatsChartDetail("zero")?.total).toBe(0);
  });

  it("sorts missing numeric facts after known values in both directions", () => {
    const rows = [row("missing", ""), row("zero", 0), row("five", 5)];

    expect(sortCachedStatsChartDetailRows(rows, "amount", "asc").map((item) => item.id)).toEqual([
      "zero",
      "five",
      "missing",
    ]);
    expect(sortCachedStatsChartDetailRows(rows, "amount", "desc").map((item) => item.id)).toEqual([
      "five",
      "zero",
      "missing",
    ]);
  });

  it("rejects unknown totals instead of converting them to row length or zero", async () => {
    await expect(loadAllStatsChartDetailRows(payload(), async () => detail([], null))).rejects.toThrow(
      "联动明细总数缺少可验证的完整性信息"
    );
  });

  it("rejects partial and changing pagination totals", async () => {
    await expect(
      loadAllStatsChartDetailRows(payload(), async () => detail([row("one", 1)], 2))
    ).rejects.toThrow("仅拉取到 1/2 行");

    await expect(
      loadAllStatsChartDetailRows(payload(1), async ({ page }) =>
        page === 1 ? detail([row("one", 1)], 2) : detail([row("two", 2)], 3)
      )
    ).rejects.toThrow("总数在分页期间发生变化");
  });

  it("returns only an exactly complete verified page sequence", async () => {
    const result = await loadAllStatsChartDetailRows(payload(2), async ({ page }) =>
      page === 1 ? detail([row("one", 1), row("two", 2)], 3) : detail([row("three", 3)], 3)
    );

    expect(result.total).toBe(3);
    expect(result.rows.map((item) => item.id)).toEqual(["one", "two", "three"]);
  });
});
