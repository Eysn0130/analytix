import type {
  ChartCashFilter,
  ChartDirectionMode,
  ChartFilterToken,
  ChartGranularity,
  ChartMetricMode,
  ChartPanelViewState,
  ChartSuccessFilter,
  StatsV2ChartDashboardRequest,
  StatsV2ChartDetailRowsRequest,
} from "../api/stats-api";
import type { ChartAnalysisState } from "../model/chart-analysis-types";

export type StatsChartDashboardRequest = StatsV2ChartDashboardRequest;
export type StatsChartDetailRowsRequest = StatsV2ChartDetailRowsRequest;

interface BuildStatsChartDashboardRequestInput {
  activeCaseId: string;
  selectedAccounts: string[];
  dateStart: string;
  dateEnd: string;
  metricMode: ChartMetricMode;
  directionMode: ChartDirectionMode;
  granularity: ChartGranularity;
  successFilter: ChartSuccessFilter;
  cashFilter: ChartCashFilter;
  chartFilters: ChartFilterToken[];
}

interface BuildStatsChartDetailBaseRequestInput {
  activeCaseId: string;
  selectedAccounts: string[];
  dateStart: string;
  dateEnd: string;
  directionMode: ChartDirectionMode;
  successFilter: ChartSuccessFilter;
  cashFilter: ChartCashFilter;
  chartFilters: ChartFilterToken[];
  previewLimit: number;
}

export function getPanelView(state: ChartAnalysisState, panelId: string, fallbackView: string, metricBasis = ""): ChartPanelViewState {
  return state.panelViews[panelId] || { panel_id: panelId, view: fallbackView, metric_basis: metricBasis };
}

export function sameChartFilterToken(left: ChartFilterToken, right: ChartFilterToken): boolean {
  return (
    left.source_panel_id === right.source_panel_id &&
    left.dimension === right.dimension &&
    left.value === right.value &&
    JSON.stringify(left.payload || {}) === JSON.stringify(right.payload || {})
  );
}

export function buildActiveTokenByPanel(chartFilters: ChartFilterToken[]): Record<string, ChartFilterToken> {
  const next: Record<string, ChartFilterToken> = {};
  (Array.isArray(chartFilters) ? chartFilters : []).forEach((token) => {
    if (token.source_panel_id) {
      next[token.source_panel_id] = token;
    }
  });
  return next;
}

export function updateChartPanelView(
  state: ChartAnalysisState,
  panelId: string,
  patch: Partial<ChartPanelViewState>
): ChartAnalysisState {
  const current = getPanelView(state, panelId, patch.view || "default", patch.metric_basis || "");
  const nextView = {
    ...current,
    ...patch,
    panel_id: panelId,
  };
  if (
    current.view === nextView.view &&
    current.metric_basis === nextView.metric_basis &&
    current.panel_id === nextView.panel_id
  ) {
    return state;
  }
  return {
    ...state,
    panelViews: {
      ...state.panelViews,
      [panelId]: nextView,
    },
  };
}

export function toggleChartFilter(state: ChartAnalysisState, token: ChartFilterToken): ChartAnalysisState {
  const current = state.chartFilters.find((item) => item.source_panel_id === token.source_panel_id);
  if (current && sameChartFilterToken(current, token)) {
    return {
      ...state,
      chartFilters: state.chartFilters.filter((item) => item.source_panel_id !== token.source_panel_id),
    };
  }
  return {
    ...state,
    chartFilters: [...state.chartFilters.filter((item) => item.source_panel_id !== token.source_panel_id), token],
  };
}

export function clearChartFilters(state: ChartAnalysisState): ChartAnalysisState {
  return state.chartFilters.length ? { ...state, chartFilters: [] } : state;
}

export function setChartTrendGranularity(state: ChartAnalysisState, nextGranularity: ChartGranularity): ChartAnalysisState {
  return state.granularity === nextGranularity ? state : { ...state, granularity: nextGranularity };
}

export function resetChartTrendView(
  state: ChartAnalysisState,
  defaults: Pick<ChartAnalysisState, "metricMode" | "granularity">
): ChartAnalysisState {
  return {
    ...state,
    metricMode: defaults.metricMode,
    granularity: defaults.granularity,
    chartFilters: [],
  };
}

export function canResetChartTrendView({
  hasSelection,
  hasActiveFilters,
  state,
  defaults,
}: {
  hasSelection: boolean;
  hasActiveFilters: boolean;
  state: Pick<ChartAnalysisState, "metricMode" | "granularity">;
  defaults: Pick<ChartAnalysisState, "metricMode" | "granularity">;
}): boolean {
  return hasSelection && (hasActiveFilters || state.metricMode !== defaults.metricMode || state.granularity !== defaults.granularity);
}

export function buildStatsChartDashboardRequest(input: BuildStatsChartDashboardRequestInput): StatsChartDashboardRequest | null {
  if (!input.activeCaseId || !input.selectedAccounts.length) {
    return null;
  }
  return {
    case_id: input.activeCaseId,
    selected: input.selectedAccounts,
    date_start: input.dateStart,
    date_end: input.dateEnd,
    metric_mode: input.metricMode,
    direction_mode: input.directionMode,
    granularity: input.granularity,
    success_filter: input.successFilter,
    cash_filter: input.cashFilter,
    chart_filters: input.chartFilters,
    panel_views: [],
  };
}

export function buildStatsChartDetailBaseRequest(input: BuildStatsChartDetailBaseRequestInput): StatsChartDetailRowsRequest | null {
  if (!input.activeCaseId || !input.selectedAccounts.length) {
    return null;
  }
  return {
    case_id: input.activeCaseId,
    selected: input.selectedAccounts,
    date_start: input.dateStart,
    date_end: input.dateEnd,
    metric_mode: "amount",
    direction_mode: input.directionMode,
    granularity: "day",
    success_filter: input.successFilter,
    cash_filter: input.cashFilter,
    chart_filters: input.chartFilters,
    panel_views: [],
    page: 1,
    limit: input.previewLimit,
    visible_columns: [],
    sort_col: "txn_time",
    sort_dir: "desc",
  };
}

export function getStatsChartDashboardCacheKeyInput(request: StatsChartDashboardRequest | null): {
  case_id: string;
  selected: string[];
  date_start: string;
  date_end: string;
  metric_mode: ChartMetricMode;
  direction_mode: ChartDirectionMode;
  granularity: ChartGranularity;
  success_filter: ChartSuccessFilter;
  cash_filter: ChartCashFilter;
  chart_filters: ChartFilterToken[];
} | null {
  if (!request) {
    return null;
  }
  return {
    case_id: request.case_id,
    selected: request.selected,
    date_start: request.date_start,
    date_end: request.date_end,
    metric_mode: request.metric_mode,
    direction_mode: request.direction_mode,
    granularity: request.granularity,
    success_filter: request.success_filter,
    cash_filter: request.cash_filter,
    chart_filters: request.chart_filters,
  };
}

export function getStatsChartDetailCacheKeyInput(request: StatsChartDetailRowsRequest | null): {
  case_id: string;
  selected: string[];
  date_start: string;
  date_end: string;
  direction_mode: ChartDirectionMode;
  success_filter: ChartSuccessFilter;
  cash_filter: ChartCashFilter;
  chart_filters: ChartFilterToken[];
  page: number;
  limit: number;
} | null {
  if (!request) {
    return null;
  }
  return {
    case_id: request.case_id,
    selected: request.selected,
    date_start: request.date_start,
    date_end: request.date_end,
    direction_mode: request.direction_mode,
    success_filter: request.success_filter,
    cash_filter: request.cash_filter,
    chart_filters: request.chart_filters,
    page: request.page,
    limit: request.limit,
  };
}
