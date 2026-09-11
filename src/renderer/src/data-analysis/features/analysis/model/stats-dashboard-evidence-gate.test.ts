import { describe, expect, it } from "vitest";

import { formatChartDetailMoney } from "./chart-detail-model";
import {
  buildDashboardHeroCopy,
  buildSummaryTiles,
  getEmptyStateCopy,
  getDashboardEvidenceBoundaryCopy,
} from "./stats-dashboard-view-model";

describe("stats dashboard evidence gate", () => {
  it("does not turn missing summary values into zero facts", () => {
    const tiles = buildSummaryTiles({}, true);
    expect(tiles.map((tile) => tile.value)).toEqual(["--", "--", "--", "--", "--"]);
  });

  it("keeps numeric zero distinct from missing after fact authority", () => {
    const tiles = buildSummaryTiles({
      total_in_amount: 0,
      total_out_amount: 0,
      net_in_amount: 0,
      active_counterparty_count: 0,
      txn_total_count: 0,
    }, true);
    expect(tiles.map((tile) => tile.value)).toEqual(["¥0.00", "¥0.00", "¥0.00", "0", "0"]);
  });

  it("uses fixed boundary copy for unavailable and partial sources", () => {
    expect(getDashboardEvidenceBoundaryCopy("source_unavailable", "materialization_unavailable")?.title)
      .toBe("案件数据源不可用");
    expect(getDashboardEvidenceBoundaryCopy("partial", "amount_or_direction_coverage_partial")?.title)
      .toBe("数据覆盖不完整");
    expect(getDashboardEvidenceBoundaryCopy("blocked", "host_evidence_receipt_required")?.description)
      .toContain("宿主 EvidenceReceipt");
  });

  it("does not format a missing detail amount as zero", () => {
    expect(formatChartDetailMoney(null)).toBe("--");
    expect(formatChartDetailMoney("")).toBe("--");
    expect(formatChartDetailMoney("not-a-number")).toBe("--");
    expect(formatChartDetailMoney(0)).toBe("¥0.00");
  });

  it("does not describe an unavailable tree as an empty case", () => {
    const hero = buildDashboardHeroCopy({
      activeFilterCount: 0,
      emptyStateMode: "tree-unavailable",
      hasActiveFilters: false,
      hasSelection: false,
      leftSearch: "",
      resolvedSelectedObjectCount: 0,
      selectedAccounts: [],
    });
    expect(hero.title).toBe("分析对象来源不可用");
    expect(hero.subtitle).toContain("有效证据回执");
    expect(getEmptyStateCopy(false, false, "primary", "tree-unavailable").description)
      .toContain("不能把来源不可用解释为没有对象");
  });
});
