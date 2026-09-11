import { afterEach, describe, expect, it, vi } from "vitest";
import analysisPageSource from "../../features/analysis/AnalysisPage.tsx?raw";
import flowBridgeSource from "../../features/flow/graph/api/flow-bridge-service.ts?raw";
import statsApiSource from "./stats-api.ts?raw";
import statsSharedSource from "./stats-shared.ts?raw";
import {
  queryStatsV2CaseOverview,
  queryStatsV2ChartDashboard,
  queryStatsV2ChartDetailRows,
  queryStatsV2Meta,
  queryStatsV2RowsWithTiming,
  queryStatsV2TxnRowsWithTiming,
  type StatsV2ChartDashboardRequest,
  type StatsV2ChartDetailRowsRequest,
  type StatsV2RowsRequest,
  type StatsV2TxnRowsRequest,
} from "./stats-api";
import { queryStatsV2Tree } from "./stats-shared";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("legacy stats renderer quarantine", () => {
  it("projects every fact surface locally without reflecting untrusted case or PII fields", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const secret = "Bearer stats-secret";
    const card = "6217000012345678901";
    const payload: StatsV2TxnRowsRequest = {
      case_id: "case-secret",
      selected: [card],
      date_start: secret,
      date_end: secret,
      key_type: "account",
      key_value: card,
      filter: "all",
      sort_col: "txn_time",
      sort_dir: "desc",
      limit: 200,
    };
    const rowsPayload: StatsV2RowsRequest = {
      case_id: payload.case_id,
      selected: payload.selected,
      date_start: payload.date_start,
      date_end: payload.date_end,
      mode: "inAccount",
    };
    const dashboardPayload: StatsV2ChartDashboardRequest = {
      case_id: payload.case_id,
      selected: payload.selected,
      date_start: payload.date_start,
      date_end: payload.date_end,
      metric_mode: "amount",
      direction_mode: "all",
      granularity: "day",
      success_filter: "all",
      cash_filter: "all",
      chart_filters: [{ source_panel_id: "panel", dimension: secret, value: card, label: secret }],
      panel_views: [],
    };
    const detailPayload: StatsV2ChartDetailRowsRequest = {
      ...dashboardPayload,
      sort_col: payload.sort_col,
      sort_dir: payload.sort_dir,
      page: 1,
      limit: payload.limit,
      visible_columns: [secret],
    };

    const [meta, overview, tree, rows, txnRows, dashboard, detail] = await Promise.all([
      queryStatsV2Meta("case-secret"),
      queryStatsV2CaseOverview("case-secret"),
      queryStatsV2Tree({ case_id: "case-secret", tab: "byCard" }),
      queryStatsV2RowsWithTiming(rowsPayload),
      queryStatsV2TxnRowsWithTiming(payload),
      queryStatsV2ChartDashboard(dashboardPayload),
      queryStatsV2ChartDetailRows(detailPayload),
    ]);

    expect(meta).toMatchObject({ case_id: "", funds_status: "source_unavailable" });
    expect(overview).toMatchObject({ case_id: "", fact_answer_allowed: false });
    expect(tree).toMatchObject({ fact_answer_allowed: false, groups: [] });
    expect(rows.data).toMatchObject({ fact_answer_allowed: false, raw_details_exposed: false, rows: [] });
    expect(txnRows.data).toMatchObject({ fact_answer_allowed: false, raw_details_exposed: false, rows: [] });
    expect(dashboard).toMatchObject({ evidence_status: "source_unavailable", fact_answer_allowed: false });
    expect(detail).toMatchObject({ evidence_status: "source_unavailable", fact_answer_allowed: false, rows: [] });
    const serializedProjection = JSON.stringify([meta, overview, tree, rows, txnRows, dashboard, detail]);
    expect(serializedProjection).not.toContain("case-secret");
    expect(serializedProjection).not.toContain(card);
    expect(serializedProjection).not.toContain(secret);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("contains no production renderer transport or job/export contract for the legacy stats API", () => {
    for (const source of [statsApiSource, statsSharedSource, flowBridgeSource]) {
      expect(source).not.toContain("/api/v1/analysis/stats/v2");
    }
    for (const source of [statsApiSource, analysisPageSource]) {
      expect(source).not.toContain("StatsV2QueryJobDTO");
      expect(source).not.toContain("result_ref");
      expect(source).not.toContain("source_result_ref");
      expect(source).not.toContain("createStatsV2QueryJob");
      expect(source).not.toContain("createStatsV2ExportJob");
      expect(source).not.toContain("getStatsV2ExportJob");
      expect(source).not.toContain("queryStatsV2AccountDeleteInfo");
      expect(source).not.toContain("updateStatsV2AccountInfo");
      expect(source).not.toContain("deleteStatsV2Accounts");
      expect(source).not.toContain("setStatsV2DocPending");
      expect(source).not.toContain("导出 Excel");
    }
    expect(statsApiSource).not.toContain("httpClient.");
    expect(analysisPageSource).not.toContain("prompt.form");
    expect(analysisPageSource).not.toContain("confirm.danger");
  });
});
