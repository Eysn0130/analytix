import type {
  ChartCashFilter,
  ChartDirectionMode,
  ChartFilterToken,
  ChartGranularity,
  ChartMetricMode,
  ChartPanelViewState,
  ChartSuccessFilter,
  StatsWorkspaceTab,
} from "../api/stats-api";

export type { StatsWorkspaceTab } from "../api/stats-api";

export interface ChartDetailSortState {
  col: string;
  dir: "asc" | "desc";
}

export interface ChartAnalysisState {
  activeWorkspaceTab: StatsWorkspaceTab;
  metricMode: ChartMetricMode;
  directionMode: ChartDirectionMode;
  granularity: ChartGranularity;
  successFilter: ChartSuccessFilter;
  cashFilter: ChartCashFilter;
  chartFilters: ChartFilterToken[];
  panelViews: Record<string, ChartPanelViewState>;
  detailSort: ChartDetailSortState;
  detailColumns: string[];
}

export const DEFAULT_CHART_DETAIL_COLUMNS = [
  "txn_time",
  "amount",
  "balance",
  "dc_flag",
  "counterparty_acct",
  "counterparty_name",
  "counterparty_bank",
  "summary",
  "txn_type",
  "is_success",
  "ip_addr",
  "remark",
  "query_feedback_reason",
];

export function createDefaultChartAnalysisState(): ChartAnalysisState {
  return {
    activeWorkspaceTab: "stats",
    metricMode: "amount",
    directionMode: "all",
    granularity: "day",
    successFilter: "all",
    cashFilter: "all",
    chartFilters: [],
    panelViews: {
      trend: { panel_id: "trend", view: "fund-flow" },
      balance: { panel_id: "balance", view: "auto" },
      counterparties: { panel_id: "counterparties", view: "name", metric_basis: "amount" },
      structure: { panel_id: "structure", view: "direction" },
      heatmap: { panel_id: "heatmap", view: "weekday-hour" },
      distribution: { panel_id: "distribution", view: "bank", metric_basis: "amount" },
      flow: { panel_id: "flow", view: "in", metric_basis: "amount" },
      anomalyAmount: { panel_id: "anomalyAmount", view: "bucket", metric_basis: "count" },
      anomalyType: { panel_id: "anomalyType", view: "txn-type", metric_basis: "amount" },
      anomalyEnv: { panel_id: "anomalyEnv", view: "ip", metric_basis: "amount" },
    },
    detailSort: { col: "txn_time", dir: "desc" },
    detailColumns: DEFAULT_CHART_DETAIL_COLUMNS.slice(),
  };
}
