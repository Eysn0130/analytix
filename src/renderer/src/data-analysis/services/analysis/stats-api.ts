import type { HttpClientTiming } from "../http/client";
import {
  LEGACY_STATS_EVIDENCE_BLOCKER,
  queryStatsV2Tree,
} from "./stats-shared";
import type {
  ChartCashFilter,
  ChartDirectionMode,
  ChartFilterToken,
  ChartGranularity,
  ChartMetricMode,
  ChartPanelViewState,
  ChartSuccessFilter,
  StatsKeyType,
  StatsMode,
  StatsRowDTO,
  StatsTreeTab,
  StatsTxnFilter,
  StatsTxnRowDTO,
  StatsTxnSortCol,
  StatsV2ChartDashboardDTO,
  StatsV2ChartDetailRowsDTO,
  StatsV2MetaDTO,
  StatsV2RowsDTO,
  StatsV2TxnRowsDTO,
  StatsWorkspaceTab,
} from "./stats-shared";

export { queryStatsV2Tree };
export type {
  ChartCashFilter,
  ChartDirectionMode,
  ChartFilterToken,
  ChartGranularity,
  ChartMetricMode,
  ChartPanelViewState,
  ChartSuccessFilter,
  StatsKeyType,
  StatsMode,
  StatsRowDTO,
  StatsTreeGroupDTO,
  StatsTreeItemDTO,
  StatsTreeTab,
  StatsTxnFilter,
  StatsTxnRowDTO,
  StatsTxnSortCol,
  StatsV2ChartDashboardDTO,
  StatsV2ChartDetailRowsDTO,
  StatsV2MetaDTO,
  StatsV2RowsDTO,
  StatsV2TxnRowsDTO,
  StatsWorkspaceTab,
} from "./stats-shared";

export interface StatsV2CaseOverviewDTO {
  case_id: string;
  fact_answer_allowed: boolean;
  account_status: "verified" | "partial" | "unavailable";
  account_blocker: string;
  account_count: number | null;
  personal_account_count: number | null;
  corporate_account_count: number | null;
  unknown_account_count: number | null;
  transaction_status: "verified" | "partial" | "unavailable";
  transaction_blocker: string;
  transaction_count: number | null;
  amount_status: "verified" | "partial" | "unavailable";
  amount_blocker: string;
  inflow_amount: number | null;
  outflow_amount: number | null;
  amount_total_rows: number | null;
  amount_present_rows: number | null;
  amount_missing_rows: number | null;
  amount_parse_failed_rows: number | null;
  direction_covered_rows: number | null;
  source_table: string;
  account_source_table: string;
  source_revision: number;
  generated_at: string;
}

export interface StatsV2TxnRowsRequest {
  case_id: string;
  selected: string[];
  date_start: string;
  date_end: string;
  key_type: StatsKeyType;
  key_value: string;
  key_values?: string[];
  filter: StatsTxnFilter;
  sort_col: StatsTxnSortCol;
  sort_dir: "asc" | "desc";
  limit: number;
  cursor?: Record<string, unknown> | null;
  row_format?: "object" | "array";
  fields?: string[];
}

export interface StatsV2TxnRowsTimedResult {
  data: StatsV2TxnRowsDTO;
  timing: HttpClientTiming;
}

export interface StatsV2RowsRequest {
  case_id: string;
  mode: StatsMode;
  selected: string[];
  date_start: string;
  date_end: string;
  search_text?: string;
  row_sort_col?: string;
  row_sort_dir?: "asc" | "desc";
  row_offset?: number;
  row_limit?: number;
  row_format?: "object" | "array";
  fields?: string[];
}

export interface StatsV2RowsTimedResult {
  data: StatsV2RowsDTO;
  timing: HttpClientTiming;
}

export interface StatsV2ChartDashboardRequest {
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
  panel_views: ChartPanelViewState[];
}

export interface StatsV2ChartDetailRowsRequest extends StatsV2ChartDashboardRequest {
  sort_col: string;
  sort_dir: "asc" | "desc";
  page: number;
  limit: number;
  visible_columns: string[];
}

const LOCAL_PROJECTION_TIMING: HttpClientTiming = Object.freeze({
  fetchMs: 0,
  textMs: 0,
  jsonParseMs: 0,
  totalMs: 0,
  responseBytes: 0,
  contentLength: 0,
  jsonBytesHeader: 0,
  status: 0,
  contentEncoding: "",
  requestDurationMs: 0,
  serverTiming: "",
  serverTimingMetrics: Object.freeze({}),
  serverTimingTotalMs: 0,
  resourceTimingAvailable: false,
  resourceDurationMs: 0,
  resourceStartDelayMs: 0,
  resourceQueueMs: 0,
  resourceDnsMs: 0,
  resourceConnectMs: 0,
  resourceSecureConnectionMs: 0,
  resourceRequestMs: 0,
  resourceFetchToResponseMs: 0,
  resourceTtfbMs: 0,
  resourceResponseBodyMs: 0,
  resourceTransferSize: 0,
  resourceEncodedBodySize: 0,
  resourceDecodedBodySize: 0,
  resourceNextHopProtocol: "",
});

const TABLE_WIDTHS_STORAGE_PREFIX = "analytix:stats:table-widths:v1";

function localProjectionTiming(): HttpClientTiming {
  return {
    ...LOCAL_PROJECTION_TIMING,
    serverTimingMetrics: {},
  };
}

function localTableWidthsKey(payload: { tab: string; mode: string }): string {
  const tab = String(payload.tab || "").replace(/[^a-zA-Z0-9_-]/g, "").slice(0, 32) || "stats";
  const mode = String(payload.mode || "").replace(/[^a-zA-Z0-9_-]/g, "").slice(0, 32) || "default";
  return `${TABLE_WIDTHS_STORAGE_PREFIX}:${tab}:${mode}`;
}

function normalizeTableWidths(raw: unknown): number[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw
    .slice(0, 128)
    .map((value) => Number(value))
    .filter((value) => Number.isFinite(value) && value >= 40 && value <= 2048)
    .map((value) => Math.floor(value));
}

export async function queryStatsV2Meta(_caseId?: string): Promise<StatsV2MetaDTO> {
  return {
    contract: "StatsMetaPublicBoundaryV1",
    semantic_status: "source_unavailable",
    fact_answer_allowed: false,
    blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
    case_id: "",
    funds_status: "source_unavailable",
    date_min: "",
    date_max: "",
  };
}

export async function queryStatsV2CaseOverview(_caseId?: string): Promise<StatsV2CaseOverviewDTO> {
  return {
    case_id: "",
    fact_answer_allowed: false,
    account_status: "unavailable",
    account_blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
    account_count: null,
    personal_account_count: null,
    corporate_account_count: null,
    unknown_account_count: null,
    transaction_status: "unavailable",
    transaction_blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
    transaction_count: null,
    amount_status: "unavailable",
    amount_blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
    inflow_amount: null,
    outflow_amount: null,
    amount_total_rows: null,
    amount_present_rows: null,
    amount_missing_rows: null,
    amount_parse_failed_rows: null,
    direction_covered_rows: null,
    source_table: "",
    account_source_table: "",
    source_revision: 0,
    generated_at: "",
  };
}

export async function queryStatsV2TxnRowsWithTiming(
  _payload?: StatsV2TxnRowsRequest,
  _init: RequestInit = {},
): Promise<StatsV2TxnRowsTimedResult> {
  return {
    data: {
      contract: "StatsTxnRowsPublicBoundaryV1",
      semantic_status: "blocked",
      blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
      fact_answer_allowed: false,
      raw_details_exposed: false,
      rows: [],
      done: false,
      next_cursor: null,
    },
    timing: localProjectionTiming(),
  };
}

export async function queryStatsV2RowsWithTiming(
  _payload?: StatsV2RowsRequest,
  _init: RequestInit = {},
): Promise<StatsV2RowsTimedResult> {
  return {
    data: {
      contract: "StatsRowsPublicBoundaryV1",
      semantic_status: "blocked",
      blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
      fact_answer_allowed: false,
      raw_details_exposed: false,
      rows: [],
      total: null,
      row_summary: {},
      status: "controlled_projection_required",
    },
    timing: localProjectionTiming(),
  };
}

export async function queryStatsV2ChartDashboard(
  _payload?: StatsV2ChartDashboardRequest,
): Promise<StatsV2ChartDashboardDTO> {
  return {
    evidence_status: "source_unavailable",
    answer_card_complete: false,
    fact_answer_allowed: false,
    blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
    coverage: {
      total_rows: null,
      amount_present_rows: null,
      amount_missing_rows: null,
      amount_parse_failed_rows: null,
      direction_covered_rows: null,
    },
    selection_mode: "multi-card",
    object_summary: {},
    summary: {},
    trend: {},
    counterparties: {},
    structure: {},
    heatmap: {},
    distribution: {},
    flow: {},
    anomaly: {},
  };
}

export async function queryStatsV2ChartDetailRows(
  _payload?: StatsV2ChartDetailRowsRequest,
): Promise<StatsV2ChartDetailRowsDTO> {
  return {
    evidence_status: "source_unavailable",
    answer_card_complete: false,
    fact_answer_allowed: false,
    blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
    selection_mode: "multi-card",
    rows: [],
    total: null,
  };
}

export async function getStatsV2TableWidths(payload: {
  tab: string;
  mode: string;
}): Promise<{ v?: string; widths?: unknown[] }> {
  if (typeof window === "undefined") {
    return {};
  }
  try {
    const parsed = JSON.parse(window.localStorage.getItem(localTableWidthsKey(payload)) || "null") as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return {};
    }
    const record = parsed as Record<string, unknown>;
    return {
      v: typeof record.v === "string" ? record.v.slice(0, 256) : undefined,
      widths: normalizeTableWidths(record.widths),
    };
  } catch {
    return {};
  }
}

export async function setStatsV2TableWidths(payload: {
  tab: string;
  mode: string;
  version: string;
  widths: unknown[];
}): Promise<{ ok: boolean }> {
  if (typeof window === "undefined") {
    return { ok: false };
  }
  try {
    window.localStorage.setItem(localTableWidthsKey(payload), JSON.stringify({
      v: String(payload.version || "").slice(0, 256),
      widths: normalizeTableWidths(payload.widths),
    }));
    return { ok: true };
  } catch {
    return { ok: false };
  }
}
