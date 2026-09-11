import {
  CSSProperties,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState
} from "react";
import { createPortal } from "react-dom";
import { showToast } from "../../components/feedback/toast-copy";
import { PageTopBar } from "../../components/page-topbar/PageTopBar";
import { PersistentHorizontalScrollbar } from "../../components/scrollbar/PersistentHorizontalScrollbar";
import { TxnDetailDialog } from "../../components/txn-detail/TxnDetailDialog";
import { buildTxnDetailLoadMetric } from "../../components/txn-detail/txn-detail-load-metrics-model";
import {
  buildTxnDetailScrollLoadMetrics,
  shouldLoadMoreTxnDetailRows
} from "../../components/txn-detail/txn-detail-window-model";
import { useTxnDetailViewport } from "../../components/txn-detail/useTxnDetailViewport";
import {
  WorkbenchButton,
  WorkbenchModeCard,
  WorkbenchSearchField,
  WorkbenchSegmentedControl,
  WorkbenchTableActions,
  WorkbenchTableFooter,
  WorkbenchTableShell,
  WorkbenchTableToolbar
} from "../../components/workbench-ui";
import { DateRangeTextInput } from "./components/panels/DateRangeTextInput";
import { AnalysisTreePanel } from "./components/tree/AnalysisTreePanel";
import {
  clearPersistedStatsFlowContexts,
  writePersistedStatsFlowContexts,
} from "./resources/flow-context";
import {
  querySharedStatsMeta,
} from "./resources/stats-meta-resource";
import {
  invalidateSharedStatsRows,
} from "./resources/stats-rows-resource";
import {
  AnalysisFlowBuildJobReq,
  cancelAnalysisFlowBuildJob,
  createAnalysisFlowBuildJob,
  getSharedStatsFlowWarmService
} from "./resources/flow-stats-link";
import { eventBelongsToCase } from "../../services/ws/domain-events";
import { useAppStore } from "../../store/app-store";
import { toErrorMessage } from "../shared/errors";
import {
  projectOrdinaryClipboardValue,
  projectOrdinaryFieldValue,
} from "../shared/ordinary-pii-projection";
import {
  StatsRowDTO,
  StatsTxnRowDTO,
  StatsMode,
  StatsTreeTab,
  StatsTxnSortCol,
  getStatsV2TableWidths,
  queryStatsV2RowsWithTiming,
  queryStatsV2TxnRowsWithTiming,
  setStatsV2TableWidths
} from "./api/stats-api";
import { ChartAnalysisDashboard } from "./components/charts/ChartAnalysisDashboard";
import { stageFlowTransferPayload } from "../flow/graph/adapters/flow-page-transfer";
import { ChartAnalysisState, createDefaultChartAnalysisState } from "./model/chart-analysis-types";
import { shouldRunStatsFlowSideWork } from "./state/stats-flow-side-work-model";
import {
  STATS_ROWS_SELECTION_SETTLE_MS,
  getStatsRowsRefreshDelayMs,
} from "./state/stats-rows-refresh-state";
import {
  resolveStatsFlowExpectedTotalAmount,
} from "./model/stats-flow-transfer-model";
import {
  resolveStatsToast,
  type StatsToastKind,
} from "./state/stats-toast-model";
import {
  filterPreparedStatsTreeGroups,
  invalidateSharedStatsTree,
  querySharedStatsTree,
  type StatsPreparedTreeGroup,
  type StatsPreparedTreeItem
} from "./resources/stats-tree-resource";
import {
  addExpandedStatsTreeGroup,
  canCollapseStatsTreeGroup,
  countSelectedStatsTreeGroups,
  selectDefaultStatsTreeAccounts,
  selectStatsTreeGroupAccounts,
  selectVisibleStatsTreeAccounts,
  toggleExpandedStatsTreeGroup,
  toggleStatsTreeAccountSelection,
} from "./model/stats-left-tree-model";
import { buildStatsVisibleRowSelection } from "./model/stats-row-selection-model";
import {
  normalizeStatsRowsResult,
} from "./model/stats-rows-query-model";
import { normalizeStatsRowsSummary, type StatsRowsSummary } from "./model/stats-rows-summary-model";
import {
  UNKNOWN_CASE_FACT_LABEL,
  formatCaseFactCount,
  formatCaseFactDecimal,
  readCaseFactBoolean,
  readFiniteCaseFactNumber,
  readNonNegativeCaseFactInteger,
} from "./model/stats-case-fact-number-model";
import {
  buildStatsTxnFooterSummary,
} from "./model/stats-txn-summary-model";
import {
  normalizeStatsTxnRowsResult,
} from "./model/stats-txn-query-model";
import {
  finishStatsPerformanceStage,
  publishStatsPerformanceEntry,
  readStatsPerformanceNow,
  startStatsPerformanceStage,
  type StatsPerformanceMark,
} from "./model/stats-performance-model";
import {
  buildStatsTxnModalColumns,
  buildStatsTxnModalRows,
} from "./model/stats-txn-table-render-model";
import statsBaseCssRaw from "./styles/stats-base.css?raw";
import "./styles/stats-react-host.css";
import "./styles/stats-modern-overrides.css";

const STATS_PAGE_STATE_VERSION = 1;
const STATS_PAGE_STATE_STORAGE_PREFIX = "analytix:stats:page-state";
const ROW_HEIGHT = 32;
const MIN_COL_WIDTH = 60;
const MAX_COL_WIDTH = 520;
const GRID_HEADER_HEIGHT = 34;
const GRID_OVERSCAN_ROWS = 160;
const TXN_ROW_HEIGHT = 32;
const TXN_HEADER_HEIGHT = 32;
const TXN_OVERSCAN_ROWS = 128;
const TXN_LOAD_MORE_THRESHOLD_ROWS = 128;
const STATS_POST_CLEANING_REFRESH_MS = 260;
const STATS_INITIAL_TREE_RETRY_MS = 420;
const STATS_INVALIDATION_EVENT_NAMES = new Set([
  "import.job.completed",
  "cleaning.job.completed"
]);
const EMPTY_STATS_TXN_ROWS: StatsTxnRowDTO[] = [];
type SortDir = 1 | -1;
type TxnFilter = "all" | "in" | "out";

interface StatsTxnRenderModelTiming {
  renderModelMs: number;
  windowStart: number;
  windowRowCount: number;
  rowOverscan: number;
  columnCount: number;
  totalColumnCount: number;
  cellCount: number;
}

type TreeItem = StatsPreparedTreeItem;
type TreeGroup = StatsPreparedTreeGroup;

interface ColumnDef {
  id: string;
  title: string;
  w: number;
  type?: "money" | "count" | "net";
  sortable?: boolean;
}

interface StatsPageStateSnapshot {
  version: number;
  leftCollapsed: boolean;
  treeTab: StatsTreeTab;
  mode: StatsMode;
  activeWorkspaceTab: ChartAnalysisState["activeWorkspaceTab"];
  tableSearch: string;
  expandedGroupIds: string[];
  selectedAccounts: string[];
  dateStart: string;
  dateEnd: string;
  selectedRowIds: string[];
  activeRowId: string;
  txnFilter: TxnFilter;
  tableSort: {
    colId: string;
    dir: SortDir;
  };
  chartAnalysis: ChartAnalysisState;
}

type BuildFlowPayloadFromRows = (
  view: "relation" | "net",
  options?: { notify?: boolean }
) => Record<string, unknown> | null;

const COL_SELECT: ColumnDef = { id: "__sel", title: "", w: 48, sortable: false };

const COLS_IN_ACCOUNT: ColumnDef[] = [
  { id: "counterparty_account", title: "对方账户", w: 180 },
  { id: "counterparty_name", title: "户名", w: 140 },
  { id: "relation", title: "社会关系", w: 110 },
  { id: "location", title: "归属地", w: 120 },
  { id: "bank", title: "归属行", w: 150 },
  { id: "doc", title: "已调单", w: 90 },
  { id: "total_amount", title: "总金额", w: 120, type: "money" },
  { id: "total_count", title: "总次数", w: 100, type: "count" },
  { id: "net_in", title: "净流入", w: 120, type: "net" },
  { id: "in_amount", title: "流入金额", w: 130, type: "money" },
  { id: "in_count", title: "流入次数", w: 110, type: "count" },
  { id: "out_amount", title: "流出金额", w: 130, type: "money" },
  { id: "out_count", title: "流出次数", w: 110, type: "count" },
  { id: "first_time", title: "最早交易时间", w: 160 },
  { id: "last_time", title: "最晚交易时间", w: 160 }
];

const COLS_OUT_ACCOUNT: ColumnDef[] = [
  { id: "counterparty_account", title: "对方账户", w: 180 },
  { id: "counterparty_name", title: "户名", w: 140 },
  { id: "relation", title: "社会关系", w: 110 },
  { id: "location", title: "归属地", w: 120 },
  { id: "bank", title: "归属行", w: 150 },
  { id: "doc", title: "已调单", w: 90 },
  { id: "total_amount", title: "总金额", w: 120, type: "money" },
  { id: "total_count", title: "总次数", w: 100, type: "count" },
  { id: "net_out", title: "净流出", w: 120, type: "net" },
  { id: "out_amount", title: "流出金额", w: 130, type: "money" },
  { id: "out_count", title: "流出次数", w: 110, type: "count" },
  { id: "in_amount", title: "流入金额", w: 130, type: "money" },
  { id: "in_count", title: "流入次数", w: 110, type: "count" },
  { id: "first_time", title: "最早交易时间", w: 160 },
  { id: "last_time", title: "最晚交易时间", w: 160 }
];

const COLS_IN_NAME: ColumnDef[] = [
  { id: "counterparty_name", title: "户名", w: 160 },
  { id: "relation", title: "社会关系", w: 110 },
  { id: "location", title: "归属地", w: 120 },
  { id: "bank", title: "归属行", w: 150 },
  { id: "doc", title: "已调单", w: 90 },
  { id: "total_amount", title: "总金额", w: 120, type: "money" },
  { id: "total_count", title: "总次数", w: 100, type: "count" },
  { id: "net_in", title: "净流入", w: 120, type: "net" },
  { id: "in_amount", title: "流入金额", w: 130, type: "money" },
  { id: "in_count", title: "流入次数", w: 110, type: "count" },
  { id: "out_amount", title: "流出金额", w: 130, type: "money" },
  { id: "out_count", title: "流出次数", w: 110, type: "count" },
  { id: "first_time", title: "最早交易时间", w: 160 },
  { id: "last_time", title: "最晚交易时间", w: 160 }
];

const COLS_OUT_NAME: ColumnDef[] = [
  { id: "counterparty_name", title: "户名", w: 160 },
  { id: "relation", title: "社会关系", w: 110 },
  { id: "location", title: "归属地", w: 120 },
  { id: "bank", title: "归属行", w: 150 },
  { id: "doc", title: "已调单", w: 90 },
  { id: "total_amount", title: "总金额", w: 120, type: "money" },
  { id: "total_count", title: "总次数", w: 100, type: "count" },
  { id: "net_out", title: "净流出", w: 120, type: "net" },
  { id: "out_amount", title: "流出金额", w: 130, type: "money" },
  { id: "out_count", title: "流出次数", w: 110, type: "count" },
  { id: "in_amount", title: "流入金额", w: 130, type: "money" },
  { id: "in_count", title: "流入次数", w: 110, type: "count" },
  { id: "first_time", title: "最早交易时间", w: 160 },
  { id: "last_time", title: "最晚交易时间", w: 160 }
];

const COLS_BY_MODE: Record<StatsMode, ColumnDef[]> = {
  inAccount: COLS_IN_ACCOUNT,
  outAccount: COLS_OUT_ACCOUNT,
  inName: COLS_IN_NAME,
  outName: COLS_OUT_NAME
};

interface ModeOption {
  key: StatsMode;
  title: string;
  dimension: string;
  direction: string;
  flow: "in" | "out";
  subject: "account" | "name";
}

const MODE_OPTIONS: ModeOption[] = [
  {
    key: "inAccount",
    title: "按流入账户",
    dimension: "账户维度",
    direction: "流入侧",
    flow: "in",
    subject: "account"
  },
  {
    key: "outAccount",
    title: "按流出账户",
    dimension: "账户维度",
    direction: "流出侧",
    flow: "out",
    subject: "account"
  },
  {
    key: "inName",
    title: "按流入户名",
    dimension: "户名维度",
    direction: "流入侧",
    flow: "in",
    subject: "name"
  },
  {
    key: "outName",
    title: "按流出户名",
    dimension: "户名维度",
    direction: "流出侧",
    flow: "out",
    subject: "name"
  }
];

type QuickRangeOptionValue = "7" | "180" | "365" | "all";
type DatePopoverAnchor = "start" | "end";

const QUICK_RANGE_OPTIONS: Array<{ value: QuickRangeOptionValue; label: string }> = [
  { value: "7", label: "近一周" },
  { value: "180", label: "近半年" },
  { value: "365", label: "近一年" },
  { value: "all", label: "全部" }
];

const TXN_FILTER_OPTIONS: Array<{ value: TxnFilter; label: string }> = [
  { value: "all", label: "全部" },
  { value: "in", label: "流入" },
  { value: "out", label: "流出" }
];

const DATE_CALENDAR_WEEKDAY_LABELS = ["日", "一", "二", "三", "四", "五", "六"] as const;

interface DateCalendarCellData {
  adjacent: boolean;
  anchorActive: boolean;
  current: boolean;
  disabled: boolean;
  inRange: boolean;
  isRangeEnd: boolean;
  isRangeStart: boolean;
  today: boolean;
  value: string;
}

function toDate10(input: string): string {
  const text = String(input || "").trim();
  if (!text) {
    return "";
  }
  return text.slice(0, 10);
}

function createLocalDate(year: number, monthIndex: number, day: number): Date {
  const next = new Date(year, monthIndex, day);
  next.setHours(12, 0, 0, 0);
  return next;
}

function parseDate10(value: string): Date | null {
  const text = toDate10(value);
  const match = text.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) {
    return null;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (!Number.isFinite(year) || !Number.isFinite(month) || !Number.isFinite(day)) {
    return null;
  }
  const next = createLocalDate(year, month - 1, day);
  if (next.getFullYear() !== year || next.getMonth() !== month - 1 || next.getDate() !== day) {
    return null;
  }
  return next;
}

function formatDate10(value: Date): string {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function startOfMonth(value: Date): Date {
  return createLocalDate(value.getFullYear(), value.getMonth(), 1);
}

function endOfMonth(value: Date): Date {
  return createLocalDate(value.getFullYear(), value.getMonth() + 1, 0);
}

function addMonths(value: Date, delta: number): Date {
  return startOfMonth(createLocalDate(value.getFullYear(), value.getMonth() + delta, 1));
}

function formatCalendarMonthLabel(value: Date): string {
  return new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "long" }).format(value);
}

function buildDateCalendarCells(
  visibleMonth: Date,
  rangeStart: string,
  rangeEnd: string,
  min: string,
  max: string,
  anchor: DatePopoverAnchor
): DateCalendarCellData[] {
  const monthStart = startOfMonth(visibleMonth);
  const gridStart = createLocalDate(monthStart.getFullYear(), monthStart.getMonth(), 1 - monthStart.getDay());
  const todayValue = formatDate10(new Date());
  const anchorValue = anchor === "start" ? rangeStart : rangeEnd;
  const cells: DateCalendarCellData[] = [];

  for (let index = 0; index < 42; index += 1) {
    const next = createLocalDate(gridStart.getFullYear(), gridStart.getMonth(), gridStart.getDate() + index);
    const value = formatDate10(next);
    cells.push({
      adjacent: next.getMonth() !== monthStart.getMonth(),
      anchorActive: Boolean(anchorValue) && value === anchorValue,
      current: next.getMonth() === monthStart.getMonth(),
      disabled: Boolean((min && value < min) || (max && value > max)),
      inRange: Boolean(rangeStart && rangeEnd && value >= rangeStart && value <= rangeEnd),
      isRangeEnd: Boolean(rangeEnd) && value === rangeEnd,
      isRangeStart: Boolean(rangeStart) && value === rangeStart,
      today: value === todayValue,
      value
    });
  }

  return cells;
}

function getTxnTargetLabel(row: StatsRowDTO | null): string {
  if (!row) {
    return "未选择对象";
  }
  const typeLabel = row.keyType === "name" ? "户名" : "账户";
  const value = row.keyLabel || row.keyValue || "";
  return `${typeLabel} · ${row.keyType === "name" ? value : projectOrdinaryFieldValue("account_no", value)}`;
}

function clampWidth(value: number, fallback: number): number {
  if (!Number.isFinite(value)) {
    return fallback;
  }
  return Math.max(MIN_COL_WIDTH, Math.min(MAX_COL_WIDTH, Math.floor(value)));
}

function roundStatsPerfMs(value: number): number {
  return Math.round(Math.max(0, Number(value) || 0) * 1000) / 1000;
}

function buildStatsHttpTimingMeta(timing: {
  fetchMs?: number;
  textMs?: number;
  jsonParseMs?: number;
  totalMs?: number;
  responseBytes?: number;
  contentLength?: number;
  jsonBytesHeader?: number;
  status?: number;
  contentEncoding?: string;
  requestDurationMs?: number;
  serverTiming?: string;
  serverTimingTotalMs?: number;
  resourceTimingAvailable?: boolean;
  resourceDurationMs?: number;
  resourceStartDelayMs?: number;
  resourceQueueMs?: number;
  resourceDnsMs?: number;
  resourceConnectMs?: number;
  resourceSecureConnectionMs?: number;
  resourceRequestMs?: number;
  resourceFetchToResponseMs?: number;
  resourceTtfbMs?: number;
  resourceResponseBodyMs?: number;
  resourceTransferSize?: number;
  resourceEncodedBodySize?: number;
  resourceDecodedBodySize?: number;
  resourceNextHopProtocol?: string;
} | null | undefined): Record<string, unknown> {
  if (!timing) {
    return {};
  }
  const httpFetchMs = roundStatsPerfMs(Number(timing.fetchMs || 0));
  const httpTextMs = roundStatsPerfMs(Number(timing.textMs || 0));
  const httpJsonParseMs = roundStatsPerfMs(Number(timing.jsonParseMs || 0));
  const httpClientTotalMs = roundStatsPerfMs(Number(timing.totalMs || 0));
  const serverTimingTotalMs = roundStatsPerfMs(Number(timing.serverTimingTotalMs || 0));
  const requestDurationMs = roundStatsPerfMs(Number(timing.requestDurationMs || 0));
  const resourceTimingAvailable = Boolean(timing.resourceTimingAvailable);
  const meta: Record<string, unknown> = {
    httpFetchMs,
    httpTextMs,
    httpJsonParseMs,
    httpClientTotalMs,
    httpBodyReadParseMs: roundStatsPerfMs(httpTextMs + httpJsonParseMs),
    responseBytes: Math.max(0, Math.floor(Number(timing.responseBytes || 0))),
    contentLength: Math.max(0, Math.floor(Number(timing.contentLength || 0))),
    jsonBytesHeader: Math.max(0, Math.floor(Number(timing.jsonBytesHeader || 0))),
    httpStatus: Math.max(0, Math.floor(Number(timing.status || 0))),
    contentEncoding: String(timing.contentEncoding || "identity"),
    serverTimingLength: String(timing.serverTiming || "").length,
    serverTimingTotalMs,
    httpFetchMinusServerTimingMs: roundStatsPerfMs(Math.max(0, httpFetchMs - serverTimingTotalMs)),
    httpClientMinusServerTimingMs: roundStatsPerfMs(Math.max(0, httpClientTotalMs - serverTimingTotalMs)),
  };
  if (requestDurationMs > 0) {
    meta.requestDurationMs = requestDurationMs;
    meta.requestMinusServerTimingMs = roundStatsPerfMs(Math.max(0, requestDurationMs - serverTimingTotalMs));
    meta.httpFetchMinusRequestMs = roundStatsPerfMs(Math.max(0, httpFetchMs - requestDurationMs));
    meta.httpClientMinusRequestMs = roundStatsPerfMs(Math.max(0, httpClientTotalMs - requestDurationMs));
  }
  if (resourceTimingAvailable) {
    const resourceQueueMs = roundStatsPerfMs(Number(timing.resourceQueueMs || 0));
    const resourceRequestMs = roundStatsPerfMs(Number(timing.resourceRequestMs || timing.resourceTtfbMs || 0));
    const resourceTtfbMs = roundStatsPerfMs(Number(timing.resourceTtfbMs || 0));
    const resourceResponseBodyMs = roundStatsPerfMs(Number(timing.resourceResponseBodyMs || 0));
    meta.resourceTimingAvailable = 1;
    meta.resourceDurationMs = roundStatsPerfMs(Number(timing.resourceDurationMs || 0));
    meta.resourceStartDelayMs = roundStatsPerfMs(Number(timing.resourceStartDelayMs || 0));
    meta.resourceQueueMs = resourceQueueMs;
    meta.resourceDnsMs = roundStatsPerfMs(Number(timing.resourceDnsMs || 0));
    meta.resourceConnectMs = roundStatsPerfMs(Number(timing.resourceConnectMs || 0));
    meta.resourceSecureConnectionMs = roundStatsPerfMs(Number(timing.resourceSecureConnectionMs || 0));
    meta.resourceRequestMs = resourceRequestMs;
    meta.resourceFetchToResponseMs = roundStatsPerfMs(Number(timing.resourceFetchToResponseMs || 0));
    meta.resourceTtfbMs = resourceTtfbMs;
    meta.resourceResponseBodyMs = resourceResponseBodyMs;
    meta.resourceTransferSize = Math.max(0, Math.floor(Number(timing.resourceTransferSize || 0)));
    meta.resourceEncodedBodySize = Math.max(0, Math.floor(Number(timing.resourceEncodedBodySize || 0)));
    meta.resourceDecodedBodySize = Math.max(0, Math.floor(Number(timing.resourceDecodedBodySize || 0)));
    meta.resourceNextHopProtocol = String(timing.resourceNextHopProtocol || "");
    meta.resourceTtfbMinusServerTimingMs = roundStatsPerfMs(Math.max(0, resourceTtfbMs - serverTimingTotalMs));
    if (requestDurationMs > 0) {
      meta.resourceTtfbMinusRequestMs = roundStatsPerfMs(Math.max(0, resourceTtfbMs - requestDurationMs));
      meta.resourceRequestMinusRequestMs = roundStatsPerfMs(Math.max(0, resourceRequestMs - requestDurationMs));
    }
    meta.resourceBodyMinusHttpTextMs = roundStatsPerfMs(Math.max(0, resourceResponseBodyMs - httpTextMs));
  }
  return meta;
}

function getColumnGrow(column: ColumnDef): number {
  if (column.id === "__sel") {
    return 0;
  }
  if (column.id === "doc") {
    return 0.45;
  }
  if (column.type === "count") {
    return 0.55;
  }
  if (column.type === "money" || column.type === "net") {
    return 0.8;
  }
  if (column.id === "first_time" || column.id === "last_time") {
    return 1;
  }
  return 1.35;
}

function formatGridTrack(column: ColumnDef, width: number): string {
  const base = `${width}px`;
  const grow = getColumnGrow(column);
  return grow > 0 ? `minmax(${base}, ${grow}fr)` : base;
}

function renderModeFlowIcon(flow: ModeOption["flow"]): JSX.Element {
  if (flow === "in") {
    return (
      <svg viewBox="0 0 24 24" fill="none">
        <path d="M5 12H19" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" />
        <path d="M11 6L5 12L11 18" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    );
  }

  return (
    <svg viewBox="0 0 24 24" fill="none">
      <path d="M5 12H19" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" />
      <path d="M13 6L19 12L13 18" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function renderModeSubjectIcon(subject: ModeOption["subject"]): JSX.Element {
  if (subject === "account") {
    return (
      <svg viewBox="0 0 24 24" fill="none">
        <rect x="4" y="6.5" width="16" height="11" rx="2.75" stroke="currentColor" strokeWidth="1.8" />
        <path d="M4.75 10.25H19.25" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
        <path d="M8 14.25H11" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
      </svg>
    );
  }

  return (
    <svg viewBox="0 0 24 24" fill="none">
      <circle cx="12" cy="8.25" r="3.1" stroke="currentColor" strokeWidth="1.8" />
      <path d="M6.5 18.5C7.7 15.9 9.66 14.6 12 14.6C14.34 14.6 16.3 15.9 17.5 18.5" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  );
}

function fmtMoney(value: unknown): string {
  return formatCaseFactDecimal(value);
}

function uniqueText(values: string[]): string[] {
  return Array.from(new Set(values.map((item) => String(item || "").trim()).filter(Boolean)));
}

function sameTextArray(left: string[], right: string[]): boolean {
  if (left === right) {
    return true;
  }
  if (left.length !== right.length) {
    return false;
  }
  for (let index = 0; index < left.length; index += 1) {
    if (left[index] !== right[index]) {
      return false;
    }
  }
  return true;
}

function sameTreeItems(left: TreeItem[], right: TreeItem[]): boolean {
  if (left === right) {
    return true;
  }
  if (left.length !== right.length) {
    return false;
  }
  for (let index = 0; index < left.length; index += 1) {
    const leftItem = left[index];
    const rightItem = right[index];
    if (leftItem.id !== rightItem.id || leftItem.title !== rightItem.title || leftItem.sub !== rightItem.sub) {
      return false;
    }
  }
  return true;
}

function sameTreeGroups(left: TreeGroup[], right: TreeGroup[]): boolean {
  if (left === right) {
    return true;
  }
  if (left.length !== right.length) {
    return false;
  }
  for (let index = 0; index < left.length; index += 1) {
    const leftGroup = left[index];
    const rightGroup = right[index];
    if (
      leftGroup.id !== rightGroup.id ||
      leftGroup.title !== rightGroup.title ||
      leftGroup.meta !== rightGroup.meta ||
      leftGroup.extra !== rightGroup.extra ||
      !sameTreeItems(leftGroup.items, rightGroup.items)
    ) {
      return false;
    }
  }
  return true;
}

function stableHashText(value: string): string {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0).toString(36);
}

function getStableTreeGroupId(
  treeTab: StatsTreeTab,
  group: Pick<TreeGroup, "title" | "meta" | "extra" | "items">,
  backendGroupId: string
): string {
  const normalizedBackendGroupId = String(backendGroupId || "").trim();
  if (normalizedBackendGroupId) {
    return normalizedBackendGroupId;
  }
  const itemKeys = uniqueText(
    (Array.isArray(group.items) ? group.items : []).map((item) => String(item.id || "").trim()).filter(Boolean)
  ).sort();
  const rawKey = [
    String(treeTab || "byName").trim() || "byName",
    String(group.title || "").trim(),
    String(group.meta || "").trim(),
    itemKeys.join(",")
  ].join("\u001f");
  if (!rawKey.split("\u001f").join("").trim()) {
    return normalizedBackendGroupId;
  }
  return `${String(treeTab || "byName").trim() || "byName"}-${stableHashText(rawKey)}`;
}

function normalizeKeyValue(value: unknown): string {
  return String(value ?? "").trim();
}

function displayBlank(value: unknown): string {
  return String(value ?? "");
}

const PLACEHOLDER_TOKEN_PREFIX = "__cp_placeholder__::";
const PLACEHOLDER_KIND_LABELS: Record<string, string> = {
  db_null: "NULL",
  empty: "空串",
  slash_n: "\\N",
  dash: "-",
  emdash: "—",
  fw_dash: "－",
  literal_null: "null",
  literal_none: "none",
  literal_nan: "nan"
};

function normalizePlaceholderKind(value: unknown): string {
  const text = normalizeKeyValue(value);
  return Object.prototype.hasOwnProperty.call(PLACEHOLDER_KIND_LABELS, text) ? text : "";
}

function placeholderKindFromToken(value: unknown): string {
  const text = normalizeKeyValue(value);
  if (text.startsWith(PLACEHOLDER_TOKEN_PREFIX)) {
    const remainder = text.slice(PLACEHOLDER_TOKEN_PREFIX.length).split("::name::", 1)[0];
    return normalizePlaceholderKind(remainder);
  }
  return "";
}

function classifyPlaceholderDocKeyValue(value: unknown): string {
  const tokenKind = placeholderKindFromToken(value);
  if (tokenKind) {
    return tokenKind;
  }
  const text = normalizeKeyValue(value);
  if (!text) {
    return "empty";
  }
  if (text === "\\N") {
    return "slash_n";
  }
  if (text === "-") {
    return "dash";
  }
  if (text === "—") {
    return "emdash";
  }
  if (text === "－") {
    return "fw_dash";
  }
  const lower = text.toLowerCase();
  if (lower === "null") {
    return "literal_null";
  }
  if (lower === "none") {
    return "literal_none";
  }
  if (lower === "nan") {
    return "literal_nan";
  }
  if (text.includes("__unknown_cp__name::__empty__")) {
    return "db_null";
  }
  return "";
}

function isPlaceholderDocKeyValue(value: unknown, keyType: "account" | "name" = "account"): boolean {
  if (keyType === "name") {
    const text = normalizeKeyValue(value);
    if (!text) {
      return true;
    }
    const lower = text.toLowerCase();
    return text.includes("__unknown_cp__name::__empty__") || lower === "unknown" || lower.startsWith("unknown_");
  }
  return Boolean(classifyPlaceholderDocKeyValue(value));
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function parseDay(text: string): Date | null {
  const trimmed = String(text || "").trim();
  const match = trimmed.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) {
    return null;
  }
  const y = Number(match[1]);
  const m = Number(match[2]);
  const d = Number(match[3]);
  if (!Number.isFinite(y) || !Number.isFinite(m) || !Number.isFinite(d)) {
    return null;
  }
  const dt = new Date(y, m - 1, d);
  if (dt.getFullYear() !== y || dt.getMonth() !== m - 1 || dt.getDate() !== d) {
    return null;
  }
  return dt;
}

function formatDay(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

function getActiveQuickRangeValue(start: string, end: string, min: string, max: string): "" | "7" | "180" | "365" | "all" {
  if (!start || !end) {
    return "";
  }
  if (min && max && start === min && end === max) {
    return "all";
  }
  const startDate = parseDay(start);
  const endDate = parseDay(end);
  if (!startDate || !endDate) {
    return "";
  }
  const diffDays = Math.round((endDate.getTime() - startDate.getTime()) / 86400000) + 1;
  if (diffDays === 7) {
    return "7";
  }
  if (diffDays === 180) {
    return "180";
  }
  if (diffDays === 365) {
    return "365";
  }
  return "";
}

function resolveQuickRange(days: number | "all", currentEnd: string, min: string, max: string): { start: string; end: string } {
  if (days === "all") {
    return { start: min || "", end: max || "" };
  }
  const end = parseDay(currentEnd || max) || new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - (days - 1));
  let nextStart = formatDay(start);
  if (min) {
    const minDate = parseDay(min);
    if (minDate && start < minDate) {
      nextStart = min;
    }
  }
  return { start: nextStart, end: formatDay(end) };
}

function getColumnsForMode(mode: StatsMode): ColumnDef[] {
  return [COL_SELECT, ...(COLS_BY_MODE[mode] || COLS_IN_ACCOUNT)];
}

function getDefaultSort(mode: StatsMode): { colId: string; dir: SortDir } {
  return String(mode || "").startsWith("out")
    ? { colId: "net_out", dir: -1 }
    : { colId: "net_in", dir: -1 };
}

function getColsVersion(columns: ColumnDef[]): string {
  return columns.map((column) => column.id).join("|");
}

function getDocTagClass(doc: string): string {
  if (doc === "已调单") {
    return "tag ok";
  }
  if (doc === "待调单") {
    return "tag warn";
  }
  return "tag";
}

async function writeTextToClipboard(text: string): Promise<void> {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  if (typeof document === "undefined" || !document.body) {
    throw new Error("Clipboard API unavailable");
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.top = "-9999px";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  try {
    if (!document.execCommand("copy")) {
      throw new Error("Copy command rejected");
    }
  } finally {
    document.body.removeChild(textarea);
  }
}

const STATS_REACT_SHELL_SELECTOR = ".stats-react-shell";

function splitCssLeadingTrivia(value: string): { leading: string; selector: string } {
  const match = value.match(/^(\s*(?:\/\*[\s\S]*?\*\/\s*)*)/);
  const leading = match?.[0] ?? "";
  return {
    leading,
    selector: value.slice(leading.length)
  };
}

function scopeStatsSelector(selector: string): string {
  const normalized = selector
    .trim()
    .replace(/:root/g, STATS_REACT_SHELL_SELECTOR)
    .replace(/\bhtml\b/g, STATS_REACT_SHELL_SELECTOR)
    .replace(/\bbody\b/g, STATS_REACT_SHELL_SELECTOR);
  if (!normalized || normalized.startsWith("@")) {
    return selector;
  }
  if (normalized.includes(STATS_REACT_SHELL_SELECTOR)) {
    return normalized;
  }
  return `${STATS_REACT_SHELL_SELECTOR} ${normalized}`;
}

function scopeStatsSelectorList(selectorList: string): string {
  const { leading, selector } = splitCssLeadingTrivia(selectorList);
  return `${leading}${selector.split(",").map(scopeStatsSelector).join(",")}`;
}

function findCssBlockEnd(css: string, openIndex: number): number {
  let depth = 1;
  for (let index = openIndex + 1; index < css.length; index += 1) {
    const char = css[index];
    if (char === "{") {
      depth += 1;
    } else if (char === "}") {
      depth -= 1;
      if (depth === 0) {
        return index;
      }
    }
  }
  return -1;
}

function buildStatsReactShellCss(raw: string): string {
  let output = "";
  let cursor = 0;
  while (cursor < raw.length) {
    const openIndex = raw.indexOf("{", cursor);
    if (openIndex === -1) {
      output += raw.slice(cursor);
      break;
    }
    const selector = raw.slice(cursor, openIndex);
    const closeIndex = findCssBlockEnd(raw, openIndex);
    if (closeIndex === -1) {
      output += raw.slice(cursor);
      break;
    }
    const body = raw.slice(openIndex + 1, closeIndex);
    const trimmedSelector = selector.trim();
    if (/^@(media|supports|container)\b/u.test(trimmedSelector)) {
      output += `${selector}{${buildStatsReactShellCss(body)}}`;
    } else if (trimmedSelector.startsWith("@")) {
      output += `${selector}{${body}}`;
    } else {
      output += `${scopeStatsSelectorList(selector)}{${body}}`;
    }
    cursor = closeIndex + 1;
  }
  return output;
}

const STATS_REACT_CSS = buildStatsReactShellCss(statsBaseCssRaw);

function getStatsPageStateStorageKey(caseId: string): string {
  return `${STATS_PAGE_STATE_STORAGE_PREFIX}:${String(caseId || "").trim()}`;
}

function readStatsPageState(caseId: string): StatsPageStateSnapshot | null {
  const normalizedCaseId = String(caseId || "").trim();
  if (!normalizedCaseId) {
    return null;
  }
  try {
    localStorage.removeItem(getStatsPageStateStorageKey(normalizedCaseId));
    Object.keys(localStorage).forEach((key) => {
      if (key.startsWith(`${STATS_PAGE_STATE_STORAGE_PREFIX}:`)) {
        localStorage.removeItem(key);
      }
    });
  } catch {
    // The P0 contract remains no-write even when legacy cleanup is unavailable.
  }
  return null;
}

function writeStatsPageState(caseId: string, snapshot: StatsPageStateSnapshot): void {
  void snapshot;
  clearStatsPageState(caseId);
}

function clearStatsPageState(caseId: string): void {
  const normalizedCaseId = String(caseId || "").trim();
  if (!normalizedCaseId) {
    return;
  }
  try {
    localStorage.removeItem(getStatsPageStateStorageKey(normalizedCaseId));
  } catch {
    // ignore localStorage failures
  }
}

export function AnalysisPage({ active = true }: { active?: boolean }): JSX.Element {
  const {
    state: {
      session: { activeCaseId },
      runtime: { lastWsEvent },
      resolvedTheme
    }
  } = useAppStore();

  const [hydratedStatsCaseId, setHydratedStatsCaseId] = useState("");
  const [leftCollapsed, setLeftCollapsed] = useState(false);
  const [treeTab, setTreeTab] = useState<StatsTreeTab>("byName");
  const [mode, setMode] = useState<StatsMode>("inAccount");
  const [chartAnalysisState, setChartAnalysisState] = useState<ChartAnalysisState>(() => createDefaultChartAnalysisState());
  const [leftSearch, setLeftSearch] = useState("");
  const [leftSearchExpanded, setLeftSearchExpanded] = useState(false);
  const [tableSearch, setTableSearch] = useState("");
  const openVisualAnalysis = useCallback(() => {
    window.dispatchEvent(new CustomEvent("data-analysis:navigate", { detail: { itemId: "visual" } }));
  }, []);

  const [groups, setGroups] = useState<TreeGroup[]>([]);
  const [expandedGroupIds, setExpandedGroupIds] = useState<string[]>([]);
  const [selectedAccounts, setSelectedAccounts] = useState<string[]>([]);

  const [fundsStatus, setFundsStatus] = useState("unknown");
  const [dateMin, setDateMin] = useState("");
  const [dateMax, setDateMax] = useState("");
  const [dateStart, setDateStart] = useState("");
  const [dateEnd, setDateEnd] = useState("");
  const [datePopoverAnchor, setDatePopoverAnchor] = useState<DatePopoverAnchor | null>(null);
  const [datePopoverMonth, setDatePopoverMonth] = useState<Date>(() => startOfMonth(createLocalDate(new Date().getFullYear(), new Date().getMonth(), 1)));
  const [datePopoverPosition, setDatePopoverPosition] = useState<CSSProperties>({ top: -9999, left: -9999 });
  const [metaInitialized, setMetaInitialized] = useState(false);

  const [rows, setRows] = useState<StatsRowDTO[]>([]);
  const [rowsSummary, setRowsSummary] = useState<StatsRowsSummary>({ totalAmount: null, totalCount: null });
  const [rowsStatus, setRowsStatus] = useState("");
  const [tableSort, setTableSort] = useState<{ colId: string; dir: SortDir }>(() => getDefaultSort("inAccount"));

  const [selectedRowIds, setSelectedRowIds] = useState<string[]>([]);
  const [activeRowId, setActiveRowId] = useState("");
  const [drawerRowId, setDrawerRowId] = useState("");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerClosing, setDrawerClosing] = useState(false);
  const [drawerPos, setDrawerPos] = useState({ left: 12, top: 12 });

  const [tableWidths, setTableWidths] = useState<number[]>([]);
  const [loadingMeta, setLoadingMeta] = useState(false);
  const [loadingTree, setLoadingTree] = useState(false);
  const [treeUnavailable, setTreeUnavailable] = useState(false);
  const [loadingRows, setLoadingRows] = useState(false);
  const [loadingTxn, setLoadingTxn] = useState(false);

  const [txnModalOpen, setTxnModalOpen] = useState(false);
  const [txnRows, setTxnRows] = useState<StatsTxnRowDTO[]>([]);
  const [txnDone, setTxnDone] = useState(false);
  const [txnFactAnswerAllowed, setTxnFactAnswerAllowed] = useState(false);
  const [txnCursor, setTxnCursor] = useState<Record<string, unknown> | null>(null);
  const [txnFilter, setTxnFilter] = useState<TxnFilter>("all");
  const [txnSortCol, setTxnSortCol] = useState<StatsTxnSortCol>("txn_time");
  const [txnSortDir, setTxnSortDir] = useState<"asc" | "desc">("desc");

  const tableWidthsRef = useRef<number[]>([]);
  const groupsRef = useRef<TreeGroup[]>([]);
  const selectedAccountsRef = useRef<string[]>([]);
  const metaInitializedRef = useRef(false);
  const rowsRefreshTimerRef = useRef<number | null>(null);
  const rowsRefreshSettledAtRef = useRef(0);
  const statsSyncTimerRef = useRef<number | null>(null);
  const treeLoadSeqRef = useRef(0);
  const treeLoadPromiseRef = useRef<{ key: string; promise: Promise<void> } | null>(null);
  const autoSelectInitialStatsGroupRef = useRef(false);
  const rowsQueryRef = useRef<{ token: number }>({ token: 0 });
  const txnQueryRef = useRef<{ token: number; reset: boolean }>({
    token: 0,
    reset: true
  });
  const rowsApplyPerformanceRef = useRef<{ mark: StatsPerformanceMark; meta: Record<string, unknown> } | null>(null);
  const txnApplyPerformanceRef = useRef<{ mark: StatsPerformanceMark; meta: Record<string, unknown> } | null>(null);
  const txnOpenPerformanceRef = useRef<{ mark: StatsPerformanceMark; meta: Record<string, unknown> } | null>(null);
  const txnRenderModelTimingRef = useRef<StatsTxnRenderModelTiming | null>(null);
  const datePopoverRef = useRef<HTMLDivElement | null>(null);
  const dateStartFieldRef = useRef<HTMLDivElement | null>(null);
  const dateEndFieldRef = useRef<HTMLDivElement | null>(null);
  const dateStartTriggerRef = useRef<HTMLButtonElement | null>(null);
  const dateEndTriggerRef = useRef<HTMLButtonElement | null>(null);
  const dateStartInputRef = useRef<HTMLInputElement | null>(null);
  const dateEndInputRef = useRef<HTMLInputElement | null>(null);
  const dateTailPointerOpenedAtRef = useRef(0);
  const leftToolbarActionsRef = useRef<HTMLDivElement | null>(null);
  const leftPanelRef = useRef<HTMLElement | null>(null);
  const leftSearchInputRef = useRef<HTMLInputElement | null>(null);
  const gridScrollRef = useRef<HTMLDivElement | null>(null);
  const gridHeaderRef = useRef<HTMLDivElement | null>(null);
  const drawerRef = useRef<HTMLElement | null>(null);
  const drawerRafRef = useRef<number | null>(null);
  const flowPrefetchTimerRef = useRef<number | null>(null);
  const flowPrefetchIssuedKeyRef = useRef("");
  const flowPrefetchSeqRef = useRef(0);
  const flowPrefetchJobRef = useRef<{ jobId: string; key: string }>({ jobId: "", key: "" });
  const buildFlowPayloadFromRowsRef = useRef<BuildFlowPayloadFromRows>(() => null);
  const shellRef = useRef<HTMLElement | null>(null);
  const skipNextModeDefaultSortRef = useRef(false);
  const scheduledStatsSyncEventIdsRef = useRef<Set<string>>(new Set());
  const [gridScrollTop, setGridScrollTop] = useState(0);
  const [gridViewportRows, setGridViewportRows] = useState(18);
  const [gridViewportWidth, setGridViewportWidth] = useState(0);
  const [gridBodyHeight, setGridBodyHeight] = useState(ROW_HEIGHT * 18);
  const [gridHeaderHeight, setGridHeaderHeight] = useState(GRID_HEADER_HEIGHT);
  const [gridScrollbarGutterWidth, setGridScrollbarGutterWidth] = useState(0);
  const [gridScrollOffsetTop, setGridScrollOffsetTop] = useState(0);
  const gridScrollTopRef = useRef(0);
  const txnCursorRef = useRef<Record<string, unknown> | null>(null);
  const loadingTxnRef = useRef(false);
  const txnDoneRef = useRef(false);
  const txnRowsLengthRef = useRef(0);
  const statsPageStateReady = Boolean(activeCaseId) && hydratedStatsCaseId === String(activeCaseId || "").trim();
  const activeTreeLoadKey = `${String(activeCaseId || "").trim()}:${treeTab}`;
  useEffect(() => {
    groupsRef.current = groups;
  }, [groups]);

  useEffect(() => {
    selectedAccountsRef.current = selectedAccounts;
  }, [selectedAccounts]);

  useEffect(() => {
    metaInitializedRef.current = metaInitialized;
  }, [metaInitialized]);

  useEffect(() => {
    txnCursorRef.current = txnCursor;
  }, [txnCursor]);

  useEffect(() => {
    txnRowsLengthRef.current = txnRows.length;
  }, [txnRows.length]);

  useLayoutEffect(() => {
    const pending = rowsApplyPerformanceRef.current;
    if (!pending) {
      return;
    }
    rowsApplyPerformanceRef.current = null;
    publishStatsPerformanceEntry(finishStatsPerformanceStage(pending.mark, {
      ...pending.meta,
      committedRowCount: rows.length,
      status: rowsStatus,
    }));
  }, [rows.length, rowsStatus]);

  useLayoutEffect(() => {
    if (!txnModalOpen) {
      return;
    }
    const pending = txnOpenPerformanceRef.current;
    if (!pending) {
      return;
    }
    txnOpenPerformanceRef.current = null;
    publishStatsPerformanceEntry(finishStatsPerformanceStage(pending.mark, {
      ...pending.meta,
      modalOpen: true,
    }));
  }, [txnModalOpen]);

  useLayoutEffect(() => {
    const pending = txnApplyPerformanceRef.current;
    if (!pending) {
      return;
    }
    txnApplyPerformanceRef.current = null;
    const renderModelTiming = txnRenderModelTimingRef.current;
    const entry = finishStatsPerformanceStage(pending.mark, {
      ...pending.meta,
      ...(renderModelTiming || {}),
      committedRowCount: txnRows.length,
      done: txnDone,
    });
    if (renderModelTiming) {
      entry.meta.commitMinusRenderModelMs = roundStatsPerfMs(entry.durationMs - renderModelTiming.renderModelMs);
    }
    publishStatsPerformanceEntry(entry);
  }, [txnDone, txnRows.length]);

  useEffect(() => {
    loadingTxnRef.current = loadingTxn;
  }, [loadingTxn]);

  useEffect(() => {
    txnDoneRef.current = txnDone;
  }, [txnDone]);

  useEffect(() => {
    if (!leftSearchExpanded || leftCollapsed) {
      return;
    }
    const rafId = window.requestAnimationFrame(() => {
      const input = leftSearchInputRef.current;
      if (!input) {
        return;
      }
      input.focus();
      const caret = input.value.length;
      input.setSelectionRange(caret, caret);
    });
    return () => {
      window.cancelAnimationFrame(rafId);
    };
  }, [leftCollapsed, leftSearchExpanded]);

  useEffect(() => {
    if (!leftSearchExpanded) {
      return;
    }
    const onPointerDown = (event: PointerEvent): void => {
      const toolbar = leftToolbarActionsRef.current;
      const leftPanel = leftPanelRef.current;
      const target = event.target;
      if (
        !(target instanceof Node) ||
        (toolbar && toolbar.contains(target)) ||
        (leftPanel && leftPanel.contains(target))
      ) {
        return;
      }
      setLeftSearch("");
      setLeftSearchExpanded(false);
    };
    document.addEventListener("pointerdown", onPointerDown, true);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown, true);
    };
  }, [leftSearchExpanded]);

  useLayoutEffect(() => {
    if (!datePopoverAnchor) {
      return;
    }
    const updatePosition = (): void => {
      const field = datePopoverAnchor === "start" ? dateStartFieldRef.current : dateEndFieldRef.current;
      if (!field) {
        return;
      }
      const rect = field.getBoundingClientRect();
      const popover = datePopoverRef.current;
      const popoverWidth = popover?.offsetWidth ?? 258;
      const popoverHeight = popover?.offsetHeight ?? 292;
      const viewportPadding = 12;
      const gap = 8;
      const horizontalOffset = datePopoverAnchor === "start" ? 18 : 56;
      let left = rect.left + horizontalOffset;
      left = Math.max(viewportPadding, Math.min(left, window.innerWidth - popoverWidth - viewportPadding));
      let top = rect.bottom + gap;
      if (top + popoverHeight + viewportPadding > window.innerHeight) {
        top = Math.max(viewportPadding, rect.top - popoverHeight - gap);
      }
      setDatePopoverPosition({ left, top });
    };
    updatePosition();
    window.addEventListener("resize", updatePosition);
    window.addEventListener("scroll", updatePosition, true);
    return () => {
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
    };
  }, [datePopoverAnchor, datePopoverMonth]);

  useEffect(() => {
    if (!datePopoverAnchor) {
      return;
    }
    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target;
      if (!(target instanceof Node)) {
        return;
      }
      const startTrigger = dateStartTriggerRef.current;
      const endTrigger = dateEndTriggerRef.current;
      const popover = datePopoverRef.current;
      if (
        (startTrigger && startTrigger.contains(target)) ||
        (endTrigger && endTrigger.contains(target)) ||
        (popover && popover.contains(target))
      ) {
        return;
      }
      setDatePopoverAnchor(null);
    };
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key !== "Escape") {
        return;
      }
      event.preventDefault();
      setDatePopoverAnchor(null);
      const trigger = datePopoverAnchor === "start" ? dateStartTriggerRef.current : dateEndTriggerRef.current;
      trigger?.focus();
    };
    document.addEventListener("pointerdown", onPointerDown, true);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown, true);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [datePopoverAnchor]);

  const columns = useMemo(() => getColumnsForMode(mode), [mode]);
  const selectedAccountsSet = useMemo(() => new Set(selectedAccounts), [selectedAccounts]);
  const selectedRowSet = useMemo(() => new Set(selectedRowIds), [selectedRowIds]);
  const activeWorkspaceTab = chartAnalysisState.activeWorkspaceTab;
  const datePopoverBounds = useMemo(
    () =>
      datePopoverAnchor === "start"
        ? { min: dateMin, max: dateEnd || dateMax }
        : { min: dateStart || dateMin, max: dateMax },
    [dateEnd, dateMax, dateMin, datePopoverAnchor, dateStart]
  );
  const datePopoverCells = useMemo(
    () =>
      datePopoverAnchor
        ? buildDateCalendarCells(datePopoverMonth, dateStart, dateEnd, datePopoverBounds.min, datePopoverBounds.max, datePopoverAnchor)
        : [],
    [dateEnd, datePopoverAnchor, datePopoverBounds.max, datePopoverBounds.min, datePopoverMonth, dateStart]
  );
  const datePopoverMonthLabel = useMemo(() => formatCalendarMonthLabel(datePopoverMonth), [datePopoverMonth]);
  const datePopoverPrevDisabled = useMemo(() => {
    if (!datePopoverBounds.min) {
      return false;
    }
    return formatDate10(endOfMonth(addMonths(datePopoverMonth, -1))) < datePopoverBounds.min;
  }, [datePopoverBounds.min, datePopoverMonth]);
  const datePopoverNextDisabled = useMemo(() => {
    if (!datePopoverBounds.max) {
      return false;
    }
    return formatDate10(startOfMonth(addMonths(datePopoverMonth, 1))) > datePopoverBounds.max;
  }, [datePopoverBounds.max, datePopoverMonth]);
  const collapsedDimensionValue = treeTab === "byCard" ? "卡号" : "用户";
  const collapsedDimensionTitle = `当前对象维度：${collapsedDimensionValue}`;
  const collapsedSelectedCardCountText = selectedAccounts.length > 99 ? "99+" : String(selectedAccounts.length);
  const expandedGroupSet = useMemo(() => new Set(expandedGroupIds), [expandedGroupIds]);

  const activeRow = useMemo(() => rows.find((item) => item.id === activeRowId) || null, [rows, activeRowId]);
  const drawerRow = useMemo(() => rows.find((item) => item.id === drawerRowId) || null, [rows, drawerRowId]);

  const visibleGroups = useMemo(() => filterPreparedStatsTreeGroups(groups, leftSearch), [groups, leftSearch]);
  const showTreeBlockingState = loadingTree && groups.length === 0;
  const chartEmptyStateMode =
    !String(activeCaseId || "").trim()
      ? "no-case"
      : showTreeBlockingState
        ? "tree-loading"
        : treeUnavailable
          ? "tree-unavailable"
          : !visibleGroups.length
            ? String(leftSearch || "").trim()
              ? "search-empty"
              : "no-objects"
            : "await-selection";
  const collapsedSelectedObjectCount = useMemo(
    () => countSelectedStatsTreeGroups(groups, selectedAccountsSet),
    [groups, selectedAccountsSet]
  );
  const collapsedSelectedObjectCountText = collapsedSelectedObjectCount > 99 ? "99+" : String(collapsedSelectedObjectCount);
  const visibleRows = rows;

  const visibleRowSelection = useMemo(
    () => buildStatsVisibleRowSelection(visibleRows, selectedRowSet),
    [selectedRowSet, visibleRows]
  );
  const visibleSelectedRows = visibleRowSelection.rows;
  const allVisibleRowsSelected = visibleRowSelection.allSelected;
  const gridWindowStart = useMemo(
    () => Math.max(0, Math.floor(gridScrollTop / ROW_HEIGHT) - GRID_OVERSCAN_ROWS),
    [gridScrollTop]
  );
  const gridWindowEnd = useMemo(
    () => Math.min(visibleRows.length, gridWindowStart + gridViewportRows + GRID_OVERSCAN_ROWS * 2),
    [gridViewportRows, gridWindowStart, visibleRows.length]
  );
  const gridWindowRows = useMemo(() => visibleRows.slice(gridWindowStart, gridWindowEnd), [gridWindowEnd, gridWindowStart, visibleRows]);
  const showEmptyGrid = rowsStatus === "no-selection";
  const emptyGridRowCount = useMemo(() => Math.max(gridViewportRows, 1), [gridViewportRows]);
  const emptyGridRows = useMemo(() => Array.from({ length: emptyGridRowCount }, (_, index) => index), [emptyGridRowCount]);
  const gridCanvasRowCount = visibleRows.length || (showEmptyGrid ? emptyGridRowCount : 0);
  const gridCanvasHeight = visibleRows.length ? gridCanvasRowCount * ROW_HEIGHT : showEmptyGrid ? gridBodyHeight : 0;
  const txnViewportResetKey = `${activeRow?.id || ""}:${txnFilter}:${txnSortCol}:${txnSortDir}:${dateStart}:${dateEnd}`;
  const txnViewport = useTxnDetailViewport({
    open: txnModalOpen,
    rowCount: txnRows.length,
    rowHeight: TXN_ROW_HEIGHT,
    headerHeight: TXN_HEADER_HEIGHT,
    overscanRows: TXN_OVERSCAN_ROWS,
    initialViewportRows: 16,
    resetKey: txnViewportResetKey
  });
  const txnWindowStart = txnViewport.windowStart;
  const txnWindowEnd = txnViewport.windowEnd;
  const txnWindowRows = useMemo(() => txnRows.slice(txnWindowStart, txnWindowEnd), [txnRows, txnWindowEnd, txnWindowStart]);
  const txnTopSpacerHeight = txnViewport.topSpacerHeight;
  const txnBottomSpacerHeight = txnViewport.bottomSpacerHeight;
  const txnViewportResetScrollPosition = txnViewport.resetScrollPosition;
  const txnTableWrapRef = txnViewport.tableWrapRef;

  const defaultWidths = useMemo(() => columns.map((column) => column.w), [columns]);

  const effectiveWidths = useMemo(() => {
    const raw = tableWidths.length === columns.length ? tableWidths : defaultWidths;
    return raw.map((width, idx) => clampWidth(Number(width), defaultWidths[idx]));
  }, [columns.length, defaultWidths, tableWidths]);

  useEffect(() => {
    tableWidthsRef.current = effectiveWidths;
  }, [effectiveWidths]);

  const gridCols = useMemo(
    () => effectiveWidths.map((width, index) => formatGridTrack(columns[index], width)).join(" "),
    [columns, effectiveWidths]
  );
  const gridMinWidth = useMemo(() => effectiveWidths.reduce((sum, width) => sum + width, 0), [effectiveWidths]);
  const gridContentWidth = useMemo(() => Math.max(gridMinWidth, gridViewportWidth), [gridMinWidth, gridViewportWidth]);

  const gridStyle = useMemo(
    () =>
      ({
        "--gridCols": gridCols,
        "--gridMinWidth": `${gridMinWidth}px`,
        "--gridContentWidth": `${gridContentWidth}px`,
        "--gridHeaderHeight": `${gridHeaderHeight}px`,
        "--gridScrollbarGutterWidth": `${gridScrollbarGutterWidth}px`,
        "--gridScrollOffsetTop": `${gridScrollOffsetTop}px`
      }) as CSSProperties,
    [gridCols, gridContentWidth, gridHeaderHeight, gridMinWidth, gridScrollbarGutterWidth, gridScrollOffsetTop]
  );

  const totalAmount = rowsSummary.totalAmount;
  const totalCount = rowsSummary.totalCount;
  const rowsFactUnavailable = rowsStatus === "blocked" || rowsStatus === "source_unavailable";
  const activeQuickRange = useMemo(() => getActiveQuickRangeValue(dateStart, dateEnd, dateMin, dateMax), [dateEnd, dateMax, dateMin, dateStart]);

  const pushToast = useCallback(
    (message: string, kind: StatsToastKind = "info") => {
      showToast(resolveStatsToast(message, kind));
    },
    []
  );

  const copyTreeItemCardNumber = useCallback(
    async (cardNumber: string) => {
      const value = projectOrdinaryClipboardValue("card_no", cardNumber);
      if (!value) {
        pushToast("卡号为空，无法复制", "warn");
        return;
      }
      try {
        await writeTextToClipboard(value);
        pushToast("已复制卡号脱敏副本", "ok");
      } catch {
        pushToast("复制卡号脱敏副本失败，请检查剪贴板权限", "danger");
      }
    },
    [pushToast]
  );

  const openLeftSearch = useCallback(() => {
    setLeftSearchExpanded(true);
  }, []);

  const collapseLeftSearch = useCallback(() => {
    setLeftSearch("");
    setLeftSearchExpanded(false);
  }, []);

  const updateDrawerLayout = useCallback((): void => {
    const scroll = gridScrollRef.current;
    const drawer = drawerRef.current;
    if (!scroll || !drawer) {
      return;
    }
    const rect = scroll.getBoundingClientRect();
    const pad = 12;
    const headerH = gridHeaderRef.current?.offsetHeight || 0;
    const viewportW = scroll.clientWidth || rect.width || 0;
    const viewportH = scroll.clientHeight || rect.height || 0;
    const drawerW = drawer.offsetWidth || 360;
    const left = Math.round((rect.left || 0) + Math.max(pad, viewportW - drawerW - pad));
    const top = Math.round((rect.top || 0) + headerH + pad);
    setDrawerPos((prev) => (prev.left === left && prev.top === top ? prev : { left, top }));
    const height = Math.max(220, viewportH - headerH - pad * 2);
    drawer.style.height = `${height}px`;
  }, []);

  const scheduleDrawerLayout = useCallback((): void => {
    if (drawerRafRef.current !== null) {
      return;
    }
    drawerRafRef.current = window.requestAnimationFrame(() => {
      drawerRafRef.current = null;
      updateDrawerLayout();
    });
  }, [updateDrawerLayout]);

  const scheduleGridScrollTopUpdate = useCallback((scrollTop: number): void => {
    const nextScrollTop = Math.max(0, scrollTop);
    if (gridScrollTopRef.current === nextScrollTop) {
      return;
    }
    gridScrollTopRef.current = nextScrollTop;
    setGridScrollTop((prev) => (prev === nextScrollTop ? prev : nextScrollTop));
  }, []);

  const openDrawer = useCallback(
    (row: StatsRowDTO): void => {
      setActiveRowId(row.id);
      setDrawerRowId(row.id);
      setDrawerClosing(false);
      setDrawerOpen(true);
      scheduleDrawerLayout();
    },
    [scheduleDrawerLayout]
  );

  const closeDrawer = useCallback((): void => {
    if (!drawerOpen) {
      setDrawerClosing(false);
      setDrawerRowId("");
      return;
    }
    setDrawerOpen(false);
    setDrawerClosing(true);
  }, [drawerOpen]);

  const loadMeta = useCallback(async (options?: { force?: boolean }) => {
    if (!activeCaseId) {
      return;
    }
    setLoadingMeta(true);
    try {
      const meta = await querySharedStatsMeta(activeCaseId, options);
      const nextMin = toDate10(meta.date_min);
      const nextMax = toDate10(meta.date_max);
      setFundsStatus(meta.funds_status || "unknown");
      setDateMin(nextMin);
      setDateMax(nextMax);
      if (!metaInitializedRef.current) {
        metaInitializedRef.current = true;
        setMetaInitialized(true);
        setDateStart(nextMin);
        setDateEnd(nextMax);
      }
    } catch (error) {
      pushToast(toErrorMessage(error), "danger");
    } finally {
      setLoadingMeta(false);
    }
  }, [activeCaseId, pushToast]);

  const loadTree = useCallback(async (options?: { force?: boolean }) => {
    if (!activeCaseId) {
      return;
    }
    const force = Boolean(options?.force);
    const loadKey = `${String(activeCaseId || "").trim()}:${treeTab}`;
    const inFlight = treeLoadPromiseRef.current;
    if (!force && inFlight && inFlight.key === loadKey) {
      return inFlight.promise;
    }
    if (force) {
      treeLoadPromiseRef.current = null;
      invalidateSharedStatsTree(activeCaseId);
    }
    const requestSeq = treeLoadSeqRef.current + 1;
    treeLoadSeqRef.current = requestSeq;
    setTreeUnavailable(false);
    setLoadingTree(true);
    const pendingPromise = (async () => {
      try {
        const nextGroups = await querySharedStatsTree({
          case_id: activeCaseId,
          tab: treeTab,
          force,
        });
        if (treeLoadSeqRef.current !== requestSeq) {
          return;
        }
        setTreeUnavailable(false);
        setGroups((prev) => (sameTreeGroups(prev, nextGroups) ? prev : nextGroups));
        const allowed = new Set<string>(nextGroups.flatMap((group) => group.itemKeys));
        const currentSelectedAccounts = selectedAccountsRef.current.filter((item) => allowed.has(item));
        const defaultSelection = !currentSelectedAccounts.length && autoSelectInitialStatsGroupRef.current
          ? selectDefaultStatsTreeAccounts(nextGroups)
          : { expandedGroupIds: [], selectedAccounts: [] };
        const nextSelectedAccounts = currentSelectedAccounts.length
          ? currentSelectedAccounts
          : defaultSelection.selectedAccounts;
        if (nextSelectedAccounts.length > 0) {
          autoSelectInitialStatsGroupRef.current = false;
        }
        const allowedGroupIds = new Set(nextGroups.map((group) => group.id).filter(Boolean));
        setExpandedGroupIds((prev) => {
          const nextExpandedGroupIds = uniqueText([
            ...prev.filter((id) => allowedGroupIds.has(id)),
            ...defaultSelection.expandedGroupIds,
          ]);
          return sameTextArray(prev, nextExpandedGroupIds) ? prev : nextExpandedGroupIds;
        });
        setSelectedAccounts((prev) => {
          return sameTextArray(prev, nextSelectedAccounts) ? prev : nextSelectedAccounts;
        });
      } catch (error) {
        if (treeLoadSeqRef.current !== requestSeq) {
          return;
        }
        if (toErrorMessage(error) === "host_evidence_receipt_required") {
          pushToast("统计事实未发布：缺少当前案件的宿主证据回执", "warn");
        } else {
          pushToast(toErrorMessage(error), "danger");
        }
        setTreeUnavailable(true);
        setGroups([]);
      } finally {
        if (treeLoadSeqRef.current === requestSeq) {
          setLoadingTree(false);
        }
      }
    })();
    treeLoadPromiseRef.current = { key: loadKey, promise: pendingPromise };
    return pendingPromise.finally(() => {
      if (treeLoadPromiseRef.current?.promise === pendingPromise) {
        treeLoadPromiseRef.current = null;
      }
    });
  }, [activeCaseId, treeTab, pushToast]);

  useEffect(() => {
    const normalizedCaseId = String(activeCaseId || "").trim();
    if (!normalizedCaseId) {
      treeLoadSeqRef.current += 1;
      treeLoadPromiseRef.current = null;
      if (hydratedStatsCaseId) {
        setHydratedStatsCaseId("");
      }
      return;
    }
    if (hydratedStatsCaseId === normalizedCaseId) {
      return;
    }
    skipNextModeDefaultSortRef.current = false;
    treeLoadSeqRef.current += 1;
    treeLoadPromiseRef.current = null;
    rowsQueryRef.current = { token: rowsQueryRef.current.token + 1 };
    txnQueryRef.current = { token: txnQueryRef.current.token + 1, reset: true };
    scheduledStatsSyncEventIdsRef.current.clear();
    const snapshot = readStatsPageState(normalizedCaseId);
    autoSelectInitialStatsGroupRef.current = !snapshot || snapshot.selectedAccounts.length > 0;
    const nextChartAnalysis = snapshot?.chartAnalysis || createDefaultChartAnalysisState();
    setLeftCollapsed(Boolean(snapshot?.leftCollapsed));
    setTreeTab(snapshot?.treeTab || "byName");
    setMode(snapshot?.mode || "inAccount");
    setChartAnalysisState(nextChartAnalysis);
    setLeftSearch("");
    setLeftSearchExpanded(false);
    setTableSearch(snapshot?.tableSearch || "");
    setGroups([]);
    setExpandedGroupIds(snapshot?.expandedGroupIds || []);
    setSelectedAccounts(snapshot?.selectedAccounts || []);
    setFundsStatus("unknown");
    setDateMin("");
    setDateMax("");
    setDateStart(snapshot?.dateStart || "");
    setDateEnd(snapshot?.dateEnd || "");
    const nextMetaInitialized = Boolean(snapshot?.dateStart || snapshot?.dateEnd);
    metaInitializedRef.current = nextMetaInitialized;
    setMetaInitialized(nextMetaInitialized);
    setRows([]);
    setRowsSummary({ totalAmount: null, totalCount: null });
    setRowsStatus("");
    setTableSort(snapshot?.tableSort || getDefaultSort(snapshot?.mode || "inAccount"));
    setSelectedRowIds(snapshot?.selectedRowIds || []);
    setActiveRowId(snapshot?.activeRowId || "");
    setDrawerRowId("");
    setDrawerOpen(false);
    setDrawerClosing(false);
    setDrawerPos({ left: 12, top: 12 });
    setLoadingRows(false);
    setLoadingTxn(false);
    setTxnModalOpen(false);
    setTxnRows([]);
    setTxnDone(false);
    setTxnFactAnswerAllowed(false);
    setTxnCursor(null);
    setTxnFilter(snapshot?.txnFilter || "all");
    setTxnSortCol("txn_time");
    setTxnSortDir("desc");
    gridScrollTopRef.current = 0;
    loadingTxnRef.current = false;
    txnDoneRef.current = false;
    txnCursorRef.current = null;
    setGridScrollTop(0);
    if (gridScrollRef.current) {
      gridScrollRef.current.scrollTop = 0;
    }
    txnViewportResetScrollPosition();
    setHydratedStatsCaseId(normalizedCaseId);
  }, [activeCaseId, hydratedStatsCaseId, txnViewportResetScrollPosition]);

  useEffect(() => {
    if (!activeCaseId || !statsPageStateReady) {
      return;
    }
    writeStatsPageState(activeCaseId, {
      version: STATS_PAGE_STATE_VERSION,
      leftCollapsed,
      treeTab,
      mode,
      activeWorkspaceTab,
      tableSearch,
      expandedGroupIds,
      selectedAccounts,
      dateStart,
      dateEnd,
      selectedRowIds,
      activeRowId,
      txnFilter,
      tableSort,
      chartAnalysis: chartAnalysisState,
    });
  }, [
    activeCaseId,
    activeRowId,
    activeWorkspaceTab,
    chartAnalysisState,
    dateEnd,
    dateStart,
    expandedGroupIds,
    leftCollapsed,
    mode,
    selectedAccounts,
    selectedRowIds,
    statsPageStateReady,
    tableSearch,
    tableSort,
    treeTab,
    txnFilter,
  ]);

  useEffect(() => {
    if (!shouldRunStatsFlowSideWork({ activeCaseId, activeWorkspaceTab, loadingRows, statsPageStateReady })) {
      return;
    }
    const timer = window.setTimeout(() => {
      const relationPayload = buildFlowPayloadFromRowsRef.current("relation", { notify: false });
      const flowPayload = buildFlowPayloadFromRowsRef.current("net", { notify: false });
      const persistedRelationPayload = relationPayload ? buildFlowJobPayload(relationPayload) : null;
      const persistedFlowPayload = flowPayload ? buildFlowJobPayload(flowPayload) : null;
      if (!persistedRelationPayload && !persistedFlowPayload) {
        clearPersistedStatsFlowContexts(activeCaseId);
        return;
      }
      writePersistedStatsFlowContexts(activeCaseId, {
        updatedAt: new Date().toISOString(),
        treeTab,
        mode,
        relationPayload: persistedRelationPayload,
        flowPayload: persistedFlowPayload,
      });
    }, 80);
    return () => window.clearTimeout(timer);
  }, [
    activeCaseId,
    activeRow,
    activeWorkspaceTab,
    dateEnd,
    dateStart,
    mode,
    loadingRows,
    rows,
    selectedAccounts,
    selectedRowIds,
    statsPageStateReady,
    treeTab,
    visibleSelectedRows,
  ]);

  const applyRowsQueryResult = useCallback((raw: unknown, token: number) => {
    if (rowsQueryRef.current.token !== token) {
      return;
    }
    const applyMark = startStatsPerformanceStage("rows.apply", {
      token,
      selectedCount: selectedAccountsRef.current.length,
    });
    const payload = normalizeStatsRowsResult(raw);
    const nextRows = payload.rows;
    rowsApplyPerformanceRef.current = {
      mark: applyMark,
      meta: {
        token,
        rowCount: nextRows.length,
        total: payload.total,
      },
    };
    setRows(nextRows);
    setRowsSummary(normalizeStatsRowsSummary(payload.rowSummary));
    setRowsStatus(payload.factAnswerAllowed ? payload.status : payload.semanticStatus);
    const ids = new Set(nextRows.map((row) => row.id));
    setSelectedRowIds((prev) => prev.filter((id) => ids.has(id)));
    setActiveRowId((prev) => (prev && ids.has(prev) ? prev : nextRows[0]?.id || ""));
    setLoadingRows(false);
  }, []);

  const applyTxnQueryResult = useCallback((raw: unknown, token: number, reset: boolean) => {
    if (txnQueryRef.current.token !== token) {
      return;
    }
    const applyMark = startStatsPerformanceStage("txn.apply", {
      token,
      reset,
      loadedRowCount: txnRowsLengthRef.current,
    });
    const payload = normalizeStatsTxnRowsResult(raw);
    const nextRows = payload.rows;
    txnApplyPerformanceRef.current = {
      mark: applyMark,
      meta: {
        token,
        reset,
        appendedRowCount: nextRows.length,
        previousRowCount: reset ? 0 : txnRowsLengthRef.current,
      },
    };
    setTxnFactAnswerAllowed(payload.factAnswerAllowed);
    setTxnRows((prev) => payload.factAnswerAllowed ? (reset ? nextRows : [...prev, ...nextRows]) : []);
    const done = payload.done;
    setTxnDone(done);
    txnDoneRef.current = done;
    const nextCursor = payload.nextCursor;
    setTxnCursor(nextCursor);
    txnCursorRef.current = nextCursor;
    setLoadingTxn(false);
    loadingTxnRef.current = false;
  }, []);

  const markRowsRefreshPending = useCallback((): void => {
    if (!activeCaseId || !statsPageStateReady) {
      return;
    }
    rowsRefreshSettledAtRef.current = Date.now() + STATS_ROWS_SELECTION_SETTLE_MS;
    rowsQueryRef.current = { token: rowsQueryRef.current.token + 1 };
    setRowsStatus("loading");
    setLoadingRows(true);
  }, [activeCaseId, statsPageStateReady]);

  const refreshRows = useCallback(async () => {
    if (!activeCaseId) {
      return;
    }
    if (!selectedAccounts.length) {
      rowsQueryRef.current = { token: rowsQueryRef.current.token + 1 };
      setRows([]);
      setRowsSummary({ totalAmount: null, totalCount: null });
      setRowsStatus("no-selection");
      setSelectedRowIds([]);
      setActiveRowId("");
      setDrawerOpen(false);
      setDrawerClosing(false);
      setDrawerRowId("");
      setLoadingRows(false);
      return;
    }

    const token = rowsQueryRef.current.token + 1;
    rowsQueryRef.current = { token };
    const queryMark = startStatsPerformanceStage("rows.query", {
      token,
      mode,
      selectedCount: selectedAccounts.length,
      rowSortCol: tableSort.colId,
      rowSortDir: tableSort.dir === 1 ? "asc" : "desc",
      searchTextLength: String(tableSearch || "").trim().length,
    });
    setLoadingRows(true);

    try {
      const timedResult = await queryStatsV2RowsWithTiming();
      const result = timedResult.data;
      if (rowsQueryRef.current.token !== token) {
        return;
      }
      const queryEntry = finishStatsPerformanceStage(queryMark, {
        requestBuildMs: 0,
        source: "local-boundary",
        ...buildStatsHttpTimingMeta(timedResult.timing),
        rowCount: Array.isArray(result.rows) ? result.rows.length : 0,
      });
      queryEntry.meta.clientUnaccountedMs = roundStatsPerfMs(queryEntry.durationMs);
      publishStatsPerformanceEntry(queryEntry);
      applyRowsQueryResult(result, token);
    } catch (error) {
      if (rowsQueryRef.current.token !== token) {
        return;
      }
      publishStatsPerformanceEntry(finishStatsPerformanceStage(queryMark, {
        requestBuildMs: 0,
        source: "local-boundary",
        error: true,
      }));
      setRows([]);
      setRowsSummary({ totalAmount: null, totalCount: null });
      setRowsStatus("source_unavailable");
      setSelectedRowIds([]);
      setActiveRowId("");
      setDrawerOpen(false);
      setDrawerClosing(false);
      setDrawerRowId("");
      setLoadingRows(false);
      pushToast(toErrorMessage(error), "danger");
    }
  }, [
    activeCaseId,
    applyRowsQueryResult,
    mode,
    pushToast,
    selectedAccounts,
    tableSearch,
    tableSort
  ]);

  const scheduleStatsSync = useCallback(
    (delay = STATS_POST_CLEANING_REFRESH_MS) => {
      if (statsSyncTimerRef.current !== null) {
        window.clearTimeout(statsSyncTimerRef.current);
      }
      statsSyncTimerRef.current = window.setTimeout(() => {
        statsSyncTimerRef.current = null;
        invalidateSharedStatsRows(activeCaseId);
        void loadMeta({ force: true });
        void loadTree({ force: true }).finally(() => {
          if (selectedAccountsRef.current.length) {
            void refreshRows();
          }
        });
      }, Math.max(0, delay));
    },
    [activeCaseId, loadMeta, loadTree, refreshRows]
  );

  const loadTxnRows = useCallback(
    async (reset: boolean) => {
      if (!activeCaseId || !activeRow) {
        return;
      }
      const token = txnQueryRef.current.token + 1;
      const rowsBeforeLoad = reset ? 0 : txnRowsLengthRef.current;
      txnQueryRef.current = { token, reset };
      const queryMark = startStatsPerformanceStage("txn.query", {
        token,
        reset,
        loadedRowCount: rowsBeforeLoad,
        filter: txnFilter,
        sortCol: txnSortCol,
        sortDir: txnSortDir,
      });
      if (reset) {
        setTxnRows([]);
        setTxnDone(false);
        setTxnFactAnswerAllowed(false);
        txnDoneRef.current = false;
        setTxnCursor(null);
        txnCursorRef.current = null;
        txnViewportResetScrollPosition();
      }
      loadingTxnRef.current = true;
      setLoadingTxn(true);

      try {
        const timedResult = await queryStatsV2TxnRowsWithTiming();
        const result = timedResult.data;
        if (txnQueryRef.current.token !== token) {
          return;
        }
        const queryEntry = finishStatsPerformanceStage(queryMark, {
          requestBuildMs: 0,
          source: "local-boundary",
          ...buildStatsHttpTimingMeta(timedResult.timing),
          rowCount: Array.isArray(result.rows) ? result.rows.length : 0,
          done: Boolean(result.done),
          ...buildTxnDetailLoadMetric({
            surface: "analysis",
            operation: "stats-modal",
            reset,
            pageLimit: 0,
            requestBuildMs: 0,
            rowsBefore: rowsBeforeLoad,
            rowsFetched: Array.isArray(result.rows) ? result.rows.length : 0,
            rowsAfter: reset
              ? (Array.isArray(result.rows) ? result.rows.length : 0)
              : rowsBeforeLoad + (Array.isArray(result.rows) ? result.rows.length : 0),
            cursorBefore: null,
            cursorAfter: result.next_cursor,
            done: Boolean(result.done)
          }),
        });
        queryEntry.meta.txnDetailDurationMs = queryEntry.durationMs;
        queryEntry.meta.clientUnaccountedMs = roundStatsPerfMs(queryEntry.durationMs);
        publishStatsPerformanceEntry(queryEntry);
        applyTxnQueryResult(result, token, reset);
      } catch (error) {
        if (txnQueryRef.current.token !== token) {
          return;
        }
        publishStatsPerformanceEntry(finishStatsPerformanceStage(queryMark, {
          requestBuildMs: 0,
          source: "local-boundary",
          error: true,
        }));
        setLoadingTxn(false);
        loadingTxnRef.current = false;
        setTxnRows([]);
        setTxnDone(false);
        setTxnFactAnswerAllowed(false);
        setTxnCursor(null);
        txnDoneRef.current = false;
        txnCursorRef.current = null;
        pushToast(toErrorMessage(error), "danger");
      }
    },
    [
      activeCaseId,
      activeRow,
      applyTxnQueryResult,
      pushToast,
      txnViewportResetScrollPosition,
      txnFilter,
      txnSortCol,
      txnSortDir
    ]
  );

  const maybeLoadMoreTxnRows = useCallback(
    (target?: HTMLDivElement | null) => {
      if (!txnFactAnswerAllowed) {
        return;
      }
      const el = target || txnTableWrapRef.current;
      if (!el) {
        return;
      }
      const loadMetrics = buildTxnDetailScrollLoadMetrics({
        scrollHeight: el.scrollHeight,
        scrollTop: el.scrollTop,
        clientHeight: el.clientHeight,
        rowHeight: TXN_ROW_HEIGHT,
        thresholdRows: TXN_LOAD_MORE_THRESHOLD_ROWS
      });
      if (!shouldLoadMoreTxnDetailRows({
        open: txnModalOpen,
        rowCount: txnRows.length,
        loading: loadingTxnRef.current,
        done: txnDoneRef.current,
        remainingPx: loadMetrics.remainingPx,
        thresholdPx: loadMetrics.thresholdPx,
      })) {
        return;
      }
      const triggerMark = startStatsPerformanceStage("txn.scroll-load-trigger", {
        rowCount: txnRows.length,
        remainingPx: Math.round(loadMetrics.remainingPx),
        thresholdPx: loadMetrics.thresholdPx,
        cursorPresent: Boolean(txnCursorRef.current),
      });
      publishStatsPerformanceEntry(finishStatsPerformanceStage(triggerMark));
      void loadTxnRows(false);
    },
    [loadTxnRows, txnFactAnswerAllowed, txnModalOpen, txnRows.length, txnTableWrapRef]
  );

  const loadSavedWidths = useCallback(async () => {
    const version = getColsVersion(columns);
    setTableWidths(defaultWidths);
    try {
      const out = await getStatsV2TableWidths({
        tab: treeTab,
        mode
      });
      const widthList = Array.isArray(out.widths) ? out.widths : [];
      if (String(out.v || "") !== version || widthList.length !== columns.length) {
        return;
      }
      setTableWidths(widthList.map((item, idx) => clampWidth(Number(item), defaultWidths[idx])));
    } catch {
      // ignore width load failure
    }
  }, [columns, defaultWidths, mode, treeTab]);

  const persistWidths = useCallback(
    async (nextWidths: number[]) => {
      const payload = {
        tab: treeTab,
        mode,
        version: getColsVersion(columns),
        widths: nextWidths.slice()
      };
      try {
        await setStatsV2TableWidths(payload);
      } catch {
        // ignore width save failure
      }
    },
    [columns, mode, treeTab]
  );

  useEffect(() => {
    if (skipNextModeDefaultSortRef.current) {
      skipNextModeDefaultSortRef.current = false;
      return;
    }
    setTableSort(getDefaultSort(mode));
  }, [mode]);

  useEffect(() => {
    if (!activeCaseId) {
      rowsQueryRef.current = { token: rowsQueryRef.current.token + 1 };
      txnQueryRef.current = { token: txnQueryRef.current.token + 1, reset: true };
      scheduledStatsSyncEventIdsRef.current.clear();
      setGroups([]);
      setTreeUnavailable(false);
      setSelectedAccounts([]);
      setRows([]);
      setRowsSummary({ totalAmount: null, totalCount: null });
      setRowsStatus("");
      setSelectedRowIds([]);
      setActiveRowId("");
      setDrawerRowId("");
      setDrawerOpen(false);
      setDrawerClosing(false);
      setDrawerPos({ left: 12, top: 12 });
      setTxnRows([]);
      setTxnCursor(null);
      setTxnDone(false);
      setTxnFactAnswerAllowed(false);
      setDateStart("");
      setDateEnd("");
      setDateMin("");
      setDateMax("");
      metaInitializedRef.current = false;
      setMetaInitialized(false);
      setFundsStatus("unknown");
      setChartAnalysisState(createDefaultChartAnalysisState());
      setHydratedStatsCaseId("");
      return;
    }
    if (!statsPageStateReady) {
      return;
    }
    void loadMeta();
  }, [activeCaseId, loadMeta, statsPageStateReady]);

  useEffect(() => {
    if (!drawerRowId) {
      return;
    }
    const exists = rows.some((item) => item.id === drawerRowId);
    if (exists) {
      return;
    }
    setDrawerOpen(false);
    setDrawerClosing(false);
    setDrawerRowId("");
  }, [drawerRowId, rows]);

  useEffect(() => {
    if (!drawerOpen && !drawerClosing) {
      return;
    }
    scheduleDrawerLayout();
    const onResize = (): void => {
      scheduleDrawerLayout();
    };
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("resize", onResize);
    };
  }, [drawerClosing, drawerOpen, scheduleDrawerLayout]);

  useEffect(() => {
    if (!activeCaseId || !statsPageStateReady) {
      return;
    }
    void loadTree();
    const retryTimer = window.setTimeout(() => {
      if (groupsRef.current.length === 0 && treeLoadPromiseRef.current?.key !== activeTreeLoadKey) {
        void loadTree();
      }
    }, STATS_INITIAL_TREE_RETRY_MS);
    return () => {
      window.clearTimeout(retryTimer);
    };
  }, [activeCaseId, activeTreeLoadKey, loadTree, statsPageStateReady]);

  useEffect(() => {
    void loadSavedWidths();
  }, [loadSavedWidths]);

  useLayoutEffect(() => {
    if (!active || activeWorkspaceTab !== "stats") {
      return;
    }

    let rafId = 0;
    const syncGridViewport = (): void => {
      const scrollElement = gridScrollRef.current;
      if (!scrollElement) {
        return;
      }
      const headerHeight = Math.max(1, Math.ceil(gridHeaderRef.current?.getBoundingClientRect().height || GRID_HEADER_HEIGHT));
      const bodyHeight = Math.max(ROW_HEIGHT, Math.floor(scrollElement.clientHeight - headerHeight));
      const viewportWidth = Math.max(0, Math.ceil(scrollElement.getBoundingClientRect().width));
      const scrollbarGutterWidth = Math.max(0, scrollElement.offsetWidth - scrollElement.clientWidth);
      const scrollOffsetTop = Math.max(0, Math.floor(scrollElement.offsetTop));
      const nextViewportRows = Math.max(1, Math.ceil(bodyHeight / ROW_HEIGHT));
      setGridViewportRows((prev) => (prev === nextViewportRows ? prev : nextViewportRows));
      setGridBodyHeight((prev) => (prev === bodyHeight ? prev : bodyHeight));
      setGridHeaderHeight((prev) => (prev === headerHeight ? prev : headerHeight));
      setGridScrollbarGutterWidth((prev) => (prev === scrollbarGutterWidth ? prev : scrollbarGutterWidth));
      setGridScrollOffsetTop((prev) => (prev === scrollOffsetTop ? prev : scrollOffsetTop));
      setGridViewportWidth((prev) => (prev === viewportWidth ? prev : viewportWidth));
      if (drawerOpen || drawerClosing) {
        scheduleDrawerLayout();
      }
    };
    const scheduleGridViewportSync = (): void => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      rafId = window.requestAnimationFrame(() => {
        rafId = 0;
        syncGridViewport();
      });
    };

    const resizeObserver = typeof ResizeObserver !== "undefined" ? new ResizeObserver(scheduleGridViewportSync) : null;
    const scrollElement = gridScrollRef.current;
    const headerElement = gridHeaderRef.current;
    if (scrollElement) {
      resizeObserver?.observe(scrollElement);
    }
    if (headerElement) {
      resizeObserver?.observe(headerElement);
    }

    scheduleGridViewportSync();
    window.addEventListener("resize", scheduleGridViewportSync);
    return () => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      resizeObserver?.disconnect();
      window.removeEventListener("resize", scheduleGridViewportSync);
    };
  }, [active, activeWorkspaceTab, drawerClosing, drawerOpen, gridCanvasRowCount, scheduleDrawerLayout, showEmptyGrid]);

  useEffect(() => {
    return () => {
      treeLoadSeqRef.current += 1;
      treeLoadPromiseRef.current = null;
      if (statsSyncTimerRef.current !== null) {
        window.clearTimeout(statsSyncTimerRef.current);
        statsSyncTimerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    return () => {
      if (drawerRafRef.current !== null) {
        window.cancelAnimationFrame(drawerRafRef.current);
        drawerRafRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    if (!activeCaseId || !statsPageStateReady) {
      return;
    }
    if (rowsRefreshTimerRef.current !== null) {
      window.clearTimeout(rowsRefreshTimerRef.current);
      rowsRefreshTimerRef.current = null;
    }
    const delayMs = getStatsRowsRefreshDelayMs(Date.now(), rowsRefreshSettledAtRef.current);
    rowsRefreshTimerRef.current = window.setTimeout(() => {
      rowsRefreshTimerRef.current = null;
      rowsRefreshSettledAtRef.current = 0;
      void refreshRows();
    }, delayMs);
    return () => {
      if (rowsRefreshTimerRef.current !== null) {
        window.clearTimeout(rowsRefreshTimerRef.current);
        rowsRefreshTimerRef.current = null;
      }
    };
  }, [activeCaseId, refreshRows, mode, dateStart, dateEnd, selectedAccounts, statsPageStateReady, tableSearch, tableSort]);

  useEffect(() => {
    if (!txnModalOpen || !activeRow) {
      return;
    }
    setTxnRows([]);
    setTxnDone(false);
    setTxnFactAnswerAllowed(false);
    setTxnCursor(null);
    void loadTxnRows(true);
  }, [activeRow, loadTxnRows, txnFilter, txnSortCol, txnSortDir, txnModalOpen, dateStart, dateEnd, selectedAccounts]);

  useEffect(() => {
    if (!activeCaseId || !statsPageStateReady || !lastWsEvent) {
      return;
    }
    if (!STATS_INVALIDATION_EVENT_NAMES.has(lastWsEvent.event) || !eventBelongsToCase(lastWsEvent, activeCaseId)) {
      return;
    }
    const payload = asRecord(lastWsEvent.payload);
    const eventId = String(payload.event_id || `${lastWsEvent.sequence}:${lastWsEvent.event}:${lastWsEvent.job_id || activeCaseId}`);
    if (scheduledStatsSyncEventIdsRef.current.has(eventId)) {
      return;
    }
    scheduledStatsSyncEventIdsRef.current.add(eventId);
    if (scheduledStatsSyncEventIdsRef.current.size > 64) {
      scheduledStatsSyncEventIdsRef.current.clear();
      scheduledStatsSyncEventIdsRef.current.add(eventId);
    }
    scheduleStatsSync();
  }, [activeCaseId, lastWsEvent, scheduleStatsSync, statsPageStateReady]);

  useEffect(() => {
    function onEsc(event: KeyboardEvent): void {
      if (event.key !== "Escape") {
        return;
      }
      if (txnModalOpen) {
        setTxnModalOpen(false);
        return;
      }
      if (drawerOpen || drawerClosing) {
        closeDrawer();
      }
    }
    window.addEventListener("keydown", onEsc);
    return () => {
      window.removeEventListener("keydown", onEsc);
    };
  }, [closeDrawer, drawerClosing, drawerOpen, txnModalOpen]);

  const toggleGroupExpand = (groupId: string): void => {
    const normalizedGroupId = String(groupId || "").trim();
    if (!normalizedGroupId) {
      return;
    }
    setExpandedGroupIds((prev) => {
      const isExpanded = prev.includes(normalizedGroupId);
      if (isExpanded) {
        const group = groupsRef.current.find((item) => item.id === normalizedGroupId);
        if (!canCollapseStatsTreeGroup(group, new Set(selectedAccountsRef.current))) {
          pushToast("请先取消该父级下的子级勾选，再收起父级", "warn");
          return prev;
        }
      }
      return toggleExpandedStatsTreeGroup(prev, normalizedGroupId);
    });
  };

  const toggleAccount = (groupId: string, accountKey: string): void => {
    const key = String(accountKey || "").trim();
    if (!key) {
      return;
    }
    autoSelectInitialStatsGroupRef.current = false;
    setExpandedGroupIds((prev) => addExpandedStatsTreeGroup(prev, groupId));
    markRowsRefreshPending();
    setSelectedAccounts((prev) => toggleStatsTreeAccountSelection(prev, key));
  };

  const setGroupSelected = (group: TreeGroup, checked: boolean): void => {
    if (!group.itemKeys.length) {
      return;
    }
    autoSelectInitialStatsGroupRef.current = false;
    if (checked) {
      setExpandedGroupIds((prev) => addExpandedStatsTreeGroup(prev, group.id));
    }
    markRowsRefreshPending();
    setSelectedAccounts((prev) => selectStatsTreeGroupAccounts(prev, group, checked));
  };

  const onSelectAllAccounts = (): void => {
    autoSelectInitialStatsGroupRef.current = false;
    const nextSelection = selectVisibleStatsTreeAccounts(visibleGroups);
    setExpandedGroupIds((prev) => uniqueText([...prev, ...nextSelection.expandedGroupIds]));
    markRowsRefreshPending();
    setSelectedAccounts(nextSelection.selectedAccounts);
  };

  const clearSelectedAccounts = (): void => {
    autoSelectInitialStatsGroupRef.current = false;
    markRowsRefreshPending();
    setSelectedAccounts([]);
  };

  const onHeaderDateInputCommit = useCallback(
    (anchor: "start" | "end") =>
      (nextValue: string): void => {
        let next = toDate10(String(nextValue || ""));
        if (!next) {
          if (anchor === "start") {
            setDateStart("");
          } else {
            setDateEnd("");
          }
          return;
        }
        if (dateMin && next < dateMin) {
          next = dateMin;
        }
        if (dateMax && next > dateMax) {
          next = dateMax;
        }
        if (anchor === "start") {
          setDateStart(next);
          if (dateEnd && next > dateEnd) {
            setDateEnd(next);
          }
          return;
        }
        setDateEnd(next);
        if (dateStart && next < dateStart) {
          setDateStart(next);
        }
      },
    [dateEnd, dateMax, dateMin, dateStart]
  );

  const openDatePopover = useCallback(
    (anchor: DatePopoverAnchor): void => {
      setDatePopoverAnchor((prev) => {
        if (prev === anchor) {
          return null;
        }
        const fallbackValue = anchor === "start" ? dateStart || dateMin || dateEnd || dateMax : dateEnd || dateMax || dateStart || dateMin;
        const monthSeed = parseDate10(fallbackValue) || createLocalDate(new Date().getFullYear(), new Date().getMonth(), 1);
        setDatePopoverMonth(startOfMonth(monthSeed));
        return anchor;
      });
    },
    [dateEnd, dateMax, dateMin, dateStart]
  );

  const focusHeaderDateInput = useCallback((anchor: DatePopoverAnchor): void => {
    const input = anchor === "start" ? dateStartInputRef.current : dateEndInputRef.current;
    input?.focus({ preventScroll: true });
  }, []);

  const onHeaderDateTailPointerDown = useCallback(
    (anchor: DatePopoverAnchor, event: ReactPointerEvent<HTMLButtonElement>): void => {
      if (event.button !== 0) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      dateTailPointerOpenedAtRef.current = Date.now();
      event.currentTarget.focus({ preventScroll: true });
      openDatePopover(anchor);
    },
    [openDatePopover]
  );

  const resolveHeaderDateTailAnchor = useCallback((event: ReactPointerEvent<HTMLElement>): DatePopoverAnchor | null => {
    const isInTailZone = (anchor: DatePopoverAnchor): boolean => {
      const field = anchor === "start" ? dateStartFieldRef.current : dateEndFieldRef.current;
      const trigger = anchor === "start" ? dateStartTriggerRef.current : dateEndTriggerRef.current;
      if (!field) {
        return false;
      }
      const fieldRect = field.getBoundingClientRect();
      if (
        event.clientY < fieldRect.top ||
        event.clientY > fieldRect.bottom ||
        event.clientX < fieldRect.left ||
        event.clientX > fieldRect.right + (anchor === "start" ? 8 : 0)
      ) {
        return false;
      }
      const triggerRect = trigger?.getBoundingClientRect();
      const triggerWidth = triggerRect?.width && Number.isFinite(triggerRect.width) ? triggerRect.width : 24;
      const tailWidth = Math.max(28, Math.ceil(triggerWidth) + 8);
      return event.clientX >= fieldRect.right - tailWidth;
    };

    if (isInTailZone("start")) {
      return "start";
    }
    if (isInTailZone("end")) {
      return "end";
    }
    return null;
  }, []);

  const onHeaderDateFieldPointerDown = useCallback(
    (anchor: DatePopoverAnchor, event: ReactPointerEvent<HTMLElement>): void => {
      if (event.button !== 0 || resolveHeaderDateTailAnchor(event) === anchor) {
        return;
      }
      event.stopPropagation();
      if (event.target === (anchor === "start" ? dateStartInputRef.current : dateEndInputRef.current)) {
        return;
      }
      event.preventDefault();
      focusHeaderDateInput(anchor);
    },
    [focusHeaderDateInput, resolveHeaderDateTailAnchor]
  );

  const openHeaderDateTailFromPointer = useCallback(
    (anchor: DatePopoverAnchor): void => {
      const trigger = anchor === "start" ? dateStartTriggerRef.current : dateEndTriggerRef.current;
      dateTailPointerOpenedAtRef.current = Date.now();
      trigger?.focus({ preventScroll: true });
      openDatePopover(anchor);
    },
    [openDatePopover]
  );

  const onHeaderDateGroupPointerDownCapture = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>): void => {
      if (event.button !== 0) {
        return;
      }
      const anchor = resolveHeaderDateTailAnchor(event);
      if (!anchor) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      openHeaderDateTailFromPointer(anchor);
    },
    [openHeaderDateTailFromPointer, resolveHeaderDateTailAnchor]
  );

  const onHeaderDateTailClick = useCallback(
    (anchor: DatePopoverAnchor, event: ReactMouseEvent<HTMLButtonElement>): void => {
      event.preventDefault();
      event.stopPropagation();
      if (event.detail > 0 && Date.now() - dateTailPointerOpenedAtRef.current < 500) {
        return;
      }
      openDatePopover(anchor);
    },
    [openDatePopover]
  );

  const selectDateFromPopover = useCallback(
    (anchor: DatePopoverAnchor, value: string): void => {
      onHeaderDateInputCommit(anchor)(value);
      setDatePopoverAnchor(null);
      window.requestAnimationFrame(() => {
        const trigger = anchor === "start" ? dateStartTriggerRef.current : dateEndTriggerRef.current;
        trigger?.focus();
      });
    },
    [onHeaderDateInputCommit]
  );

  const onQuickRange = (days: number | "all"): void => {
    const nextRange = resolveQuickRange(days, dateEnd || dateMax, dateMin, dateMax);
    setDateStart(nextRange.start);
    setDateEnd(nextRange.end);
  };

  const onResetFilters = (): void => {
    setLeftSearch("");
    setLeftSearchExpanded(false);
    setTableSearch("");
    markRowsRefreshPending();
    setSelectedAccounts([]);
    setSelectedRowIds([]);
    setActiveRowId("");
    setDrawerOpen(false);
    setDrawerClosing(false);
    setDrawerRowId("");
    setTxnFilter("all");
    setDateStart(dateMin || "");
    setDateEnd(dateMax || "");
    setChartAnalysisState((prev) => ({
      ...createDefaultChartAnalysisState(),
      activeWorkspaceTab: prev.activeWorkspaceTab
    }));
  };

  const onToggleRowSelect = (rowId: string, checked: boolean): void => {
    if (loadingRows) {
      return;
    }
    setSelectedRowIds((prev) => {
      const set = new Set(prev);
      if (checked) {
        set.add(rowId);
      } else {
        set.delete(rowId);
      }
      return Array.from(set);
    });
  };

  const onToggleSelectAllRows = (checked: boolean): void => {
    if (loadingRows) {
      return;
    }
    const keys = visibleRows.map((row) => row.id);
    if (checked) {
      setSelectedRowIds(uniqueText(keys));
      return;
    }
    setSelectedRowIds([]);
  };

  const openTxnModal = (row: StatsRowDTO, filter: TxnFilter): void => {
    if (loadingRows) {
      return;
    }
    txnOpenPerformanceRef.current = {
      mark: startStatsPerformanceStage("txn.open", {
        rowId: row.id,
        filter,
      }),
      meta: {
        totalCount: Number(row.total_count || 0),
      },
    };
    setActiveRowId(row.id);
    setTxnFilter(filter);
    setTxnSortCol("txn_time");
    setTxnSortDir("desc");
    setTxnRows([]);
    setTxnCursor(null);
    setTxnDone(false);
    setTxnFactAnswerAllowed(false);
    setTxnModalOpen(true);
  };

  const renderTxnCountCell = (row: StatsRowDTO, filter: TxnFilter, raw: unknown, columnId: string): JSX.Element => {
    const count = readNonNegativeCaseFactInteger(raw);
    if (count == null) {
      return (
        <div className="cell num" key={columnId}>
          {UNKNOWN_CASE_FACT_LABEL}
        </div>
      );
    }
    if (count <= 0) {
      return (
        <div className="cell num" key={columnId}>
          {count}
        </div>
      );
    }
    return (
      <div
        className="cell txnLink num"
        key={columnId}
        onClick={(event) => {
          event.stopPropagation();
          openTxnModal(row, filter);
        }}
      >
        {count}
      </div>
    );
  };

  const onSortColumn = (colId: string): void => {
    if (colId === "__sel" || colId === "doc") {
      return;
    }
    setTableSort((prev) => {
      if (prev.colId === colId) {
        return { colId, dir: (prev.dir === 1 ? -1 : 1) as SortDir };
      }
      return { colId, dir: -1 };
    });
  };

  const onStartResize = (index: number, event: ReactMouseEvent): void => {
    event.preventDefault();
    event.stopPropagation();
    const startX = event.clientX;
    const initial = tableWidthsRef.current[index] ?? defaultWidths[index] ?? 120;

    function onMove(moveEvent: MouseEvent): void {
      const delta = moveEvent.clientX - startX;
      setTableWidths((prev) => {
        const base = prev.length === columns.length ? prev.slice() : defaultWidths.slice();
        base[index] = clampWidth(initial + delta, defaultWidths[index] ?? 120);
        return base;
      });
    }

    function onUp(): void {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
      void persistWidths(tableWidthsRef.current);
    }

    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
  };

  const buildFlowPrefetchKey = (payload: Record<string, unknown>): string => {
    const focusIds = uniqueText(Array.isArray(payload.focusIds) ? payload.focusIds.map((item) => String(item || "")) : []);
    const focusNames = uniqueText(Array.isArray(payload.focusNames) ? payload.focusNames.map((item) => String(item || "")) : []);
    const focusPlaceholderKinds = uniqueText(
      Array.isArray(payload.focusPlaceholderKinds) ? payload.focusPlaceholderKinds.map((item) => String(item || "")) : []
    );
    const leftSeeds = uniqueText(Array.isArray(payload.leftSeeds) ? payload.leftSeeds.map((item) => String(item || "")) : []);
    const seeds = uniqueText(Array.isArray(payload.seeds) ? payload.seeds.map((item) => String(item || "")) : []);
    return JSON.stringify({
      caseId: String(payload.caseId || ""),
      view: String(payload.view || ""),
      layout: String(payload.layout || ""),
      tab: String(payload.tab || ""),
      mode: String(payload.mode || ""),
      dateStart: String(payload.dateStart || ""),
      dateEnd: String(payload.dateEnd || ""),
      focusId: String(payload.focusId || ""),
      focusName: String(payload.focusName || ""),
      focusLabel: String(payload.focusLabel || ""),
      focusKeyType: String(payload.focusKeyType || ""),
      focusOnly: readCaseFactBoolean(payload.focusOnly),
      focusUnknownName: readCaseFactBoolean(payload.focusUnknownName),
      includeMissingCounterparty: readCaseFactBoolean(payload.includeMissingCounterparty),
      focusCounterpartyStrict: readCaseFactBoolean(payload.focusCounterpartyStrict),
      expectedTotalAmount: readFiniteCaseFactNumber(payload.expectedTotalAmount),
      expectedRowCount: readNonNegativeCaseFactInteger(payload.expectedRowCount),
      seeds,
      leftSeeds,
      focusIds,
      focusNames,
      focusPlaceholderKinds
    });
  };

  const shouldSkipFlowPrefetch = (payload: Record<string, unknown>): boolean => {
    const source = String(payload.source || "").trim().toLowerCase();
    const focusOnly = readCaseFactBoolean(payload.focusOnly);
    const expectedRowCount = readNonNegativeCaseFactInteger(payload.expectedRowCount);
    if (focusOnly == null || expectedRowCount == null) {
      return false;
    }
    return source === "stats" && focusOnly && expectedRowCount > 0 && expectedRowCount <= 10000;
  };

  function buildFlowJobPayload(payload: Record<string, unknown>): AnalysisFlowBuildJobReq | null {
    const caseId = String(payload.caseId || "").trim();
    const seeds = uniqueText(Array.isArray(payload.seeds) ? payload.seeds.map((item) => String(item || "")) : []);
    const focusOnly = readCaseFactBoolean(payload.focusOnly);
    const focusUnknownName = readCaseFactBoolean(payload.focusUnknownName);
    const includeMissingCounterparty = readCaseFactBoolean(payload.includeMissingCounterparty);
    const focusCounterpartyStrict = readCaseFactBoolean(payload.focusCounterpartyStrict);
    const expectedRowCount = readNonNegativeCaseFactInteger(payload.expectedRowCount);
    if (
      !caseId ||
      !seeds.length ||
      focusOnly == null ||
      focusUnknownName == null ||
      includeMissingCounterparty == null ||
      focusCounterpartyStrict == null ||
      expectedRowCount == null
    ) {
      return null;
    }
    const requestIdBase = String(payload.requestId || "").trim();
    return {
      case_id: caseId,
      seeds,
      depth: 1,
      direction: "both",
      min_amount: 0,
      source: String(payload.source || ""),
      request_id: requestIdBase ? `${requestIdBase}:prefetch` : `stats-prefetch-${Date.now().toString(36)}`,
      view: String(payload.view || ""),
      layout: String(payload.layout || ""),
      date_start: String(payload.dateStart || ""),
      date_end: String(payload.dateEnd || ""),
      focus_id: String(payload.focusId || ""),
      focus_name: String(payload.focusName || ""),
      focus_key_type: String(payload.focusKeyType || ""),
      focus_label: String(payload.focusLabel || ""),
      focus_only: focusOnly,
      focus_unknown_name: focusUnknownName,
      include_missing_counterparty: includeMissingCounterparty,
      focus_counterparty_strict: focusCounterpartyStrict,
      left_seeds: uniqueText(Array.isArray(payload.leftSeeds) ? payload.leftSeeds.map((item) => String(item || "")) : []),
      expected_total_amount: readFiniteCaseFactNumber(payload.expectedTotalAmount),
      expected_row_count: expectedRowCount,
      focus_ids: uniqueText(Array.isArray(payload.focusIds) ? payload.focusIds.map((item) => String(item || "")) : []),
      focus_names: uniqueText(Array.isArray(payload.focusNames) ? payload.focusNames.map((item) => String(item || "")) : []),
      focus_placeholder_kinds: uniqueText(
        Array.isArray(payload.focusPlaceholderKinds) ? payload.focusPlaceholderKinds.map((item) => String(item || "")) : []
      )
    };
  }

  const buildFlowPayloadFromRows = (
    view: "relation" | "net",
    options: { notify?: boolean } = {}
  ): Record<string, unknown> | null => {
    const notify = options.notify !== false;
    if (loadingRows) {
      if (notify) pushToast("统计行正在刷新，请稍候", "warn");
      return null;
    }
    if (!activeCaseId) {
      if (notify) pushToast("请先选择案件", "warn");
      return null;
    }
    const targetRows = visibleSelectedRows.length
      ? visibleSelectedRows
      : activeRow
      ? [activeRow]
      : [];
    if (!targetRows.length) {
      if (notify) pushToast("未选择统计行", "warn");
      return null;
    }
    const leftSeeds = uniqueText(selectedAccounts);
    const fallbackSeeds = targetRows
      .map((row) => {
        const rowKeyType = row.keyType === "name" || row.keyType === "account" ? row.keyType : "account";
        const key = normalizeKeyValue(row.keyValue);
        return rowKeyType === "account" && isPlaceholderDocKeyValue(key, rowKeyType) ? "" : key;
      })
      .filter(Boolean);
    const seeds = uniqueText(leftSeeds.length ? leftSeeds : fallbackSeeds);
    if (!seeds.length) {
      if (notify) pushToast("缺少可用对象，无法生成图谱", "warn");
      return null;
    }

    const focusIds: string[] = [];
    const focusNames: string[] = [];
    const focusPlaceholderKinds: string[] = [];
    let hasUnknownName = false;
    let hasUnknownAccount = false;
    targetRows.forEach((row) => {
      const rowKeyType: "account" | "name" =
        row.keyType === "name" || row.keyType === "account"
          ? row.keyType
          : mode.includes("Name")
          ? "name"
          : "account";
      const key = normalizeKeyValue(
        row.keyValue ||
          (rowKeyType === "name" ? row.counterparty_name || "" : row.counterparty_account || "")
      );
      const accountCell = String(row.counterparty_account || "").trim();
      const placeholderKind =
        rowKeyType === "account"
          ? normalizePlaceholderKind(row.placeholderKind) ||
            classifyPlaceholderDocKeyValue(key || row.counterparty_account || row.keyLabel || "")
          : "";
      if (!key || isPlaceholderDocKeyValue(key, rowKeyType)) {
        if (rowKeyType === "name") {
          hasUnknownName = true;
        } else {
          hasUnknownAccount = true;
          if (placeholderKind) {
            focusPlaceholderKinds.push(placeholderKind);
          }
        }
        return;
      }
      if (rowKeyType === "account" && !accountCell) {
        hasUnknownAccount = true;
      }
      if (rowKeyType === "name") {
        focusNames.push(key);
      } else {
        focusIds.push(key);
      }
    });

    const keyType: "account" | "name" = focusNames.length || hasUnknownName ? "name" : "account";
    const focusId = keyType === "account" ? focusIds[0] || "" : "";
    const focusName = keyType === "name" ? focusNames[0] || "" : "";
    if (!focusIds.length && !focusNames.length && !hasUnknownName && !hasUnknownAccount) {
      if (notify) pushToast("缺少可用对象，无法生成图谱", "warn");
      return null;
    }
    const requestId = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`;
    const expectedTotalAmount = resolveStatsFlowExpectedTotalAmount(targetRows, view);
    const focusLabel =
      keyType === "name"
        ? focusNames.length === 1
          ? focusName
          : hasUnknownName && !focusNames.length
          ? "未知户名"
          : ""
        : focusId;

    return {
      caseId: activeCaseId,
      view,
      layout: view === "net" ? "flow" : "compact",
      seeds,
      focusId,
      focusName,
      focusLabel,
      focusKeyType: keyType,
      focusIds: keyType === "account" ? uniqueText(focusIds) : [],
      focusNames: keyType === "name" ? uniqueText(focusNames) : [],
      focusPlaceholderKinds: keyType === "account" ? uniqueText(focusPlaceholderKinds) : [],
      focusUnknownName: keyType === "name" ? hasUnknownName : false,
      includeMissingCounterparty: keyType === "account" ? hasUnknownAccount : false,
      focusCounterpartyStrict: keyType === "account",
      focusOnly: true,
      dateStart,
      dateEnd,
      tab: treeTab,
      mode,
      source: "stats",
      requestId,
      leftSeeds,
      expectedTotalAmount,
      expectedRowCount: targetRows.length
    };
  };
  buildFlowPayloadFromRowsRef.current = buildFlowPayloadFromRows;

  const openFlow = (view: "relation" | "net"): void => {
    const payload = buildFlowPayloadFromRows(view);
    if (!payload) {
      return;
    }
    stageFlowTransferPayload(payload);
    openVisualAnalysis();
  };

  useEffect(() => {
    if (flowPrefetchTimerRef.current != null) {
      window.clearTimeout(flowPrefetchTimerRef.current);
      flowPrefetchTimerRef.current = null;
    }

    if (!shouldRunStatsFlowSideWork({ activeCaseId, activeWorkspaceTab, loadingRows, loadingTxn, txnModalOpen })) {
      flowPrefetchIssuedKeyRef.current = "";
      const activePrefetch = flowPrefetchJobRef.current;
      flowPrefetchJobRef.current = { jobId: "", key: "" };
      if (activePrefetch.jobId) {
        void cancelAnalysisFlowBuildJob(activePrefetch.jobId).catch(() => {});
      }
      return;
    }

    const payload = buildFlowPayloadFromRowsRef.current("relation", { notify: false });
    if (!payload) {
      flowPrefetchIssuedKeyRef.current = "";
      const activePrefetch = flowPrefetchJobRef.current;
      flowPrefetchJobRef.current = { jobId: "", key: "" };
      if (activePrefetch.jobId) {
        void cancelAnalysisFlowBuildJob(activePrefetch.jobId).catch(() => {});
      }
      return;
    }

    if (
      String(payload.source || "") !== "stats" ||
      !payload.focusOnly ||
      !payload.focusCounterpartyStrict ||
      String(payload.focusKeyType || "") !== "account"
    ) {
      flowPrefetchIssuedKeyRef.current = "";
      const activePrefetch = flowPrefetchJobRef.current;
      flowPrefetchJobRef.current = { jobId: "", key: "" };
      if (activePrefetch.jobId) {
        void cancelAnalysisFlowBuildJob(activePrefetch.jobId).catch(() => {});
      }
      return;
    }

    const prefetchKey = buildFlowPrefetchKey(payload);
    if (!prefetchKey || flowPrefetchIssuedKeyRef.current === prefetchKey) {
      return;
    }

    if (shouldSkipFlowPrefetch(payload)) {
      flowPrefetchTimerRef.current = window.setTimeout(() => {
        const requestPayload = buildFlowJobPayload(payload);
        if (!requestPayload) {
          return;
        }
        const seq = flowPrefetchSeqRef.current + 1;
        flowPrefetchSeqRef.current = seq;
        flowPrefetchIssuedKeyRef.current = prefetchKey;
        const activePrefetch = flowPrefetchJobRef.current;
        if (activePrefetch.jobId && activePrefetch.key && activePrefetch.key !== prefetchKey) {
          void cancelAnalysisFlowBuildJob(activePrefetch.jobId).catch(() => {});
        }
        flowPrefetchJobRef.current = { jobId: "", key: prefetchKey };
        const flowService = getSharedStatsFlowWarmService();
        flowService.setCaseId(requestPayload.case_id);
        void flowService.warmFlowGraph(requestPayload).catch(() => {
          if (flowPrefetchIssuedKeyRef.current === prefetchKey && flowPrefetchSeqRef.current === seq) {
            flowPrefetchIssuedKeyRef.current = "";
          }
        });
      }, 120);
      return () => {
        if (flowPrefetchTimerRef.current != null) {
          window.clearTimeout(flowPrefetchTimerRef.current);
          flowPrefetchTimerRef.current = null;
        }
      };
    }

    flowPrefetchTimerRef.current = window.setTimeout(() => {
      const jobPayload = buildFlowJobPayload(payload);
      if (!jobPayload) {
        return;
      }
      const seq = flowPrefetchSeqRef.current + 1;
      flowPrefetchSeqRef.current = seq;
      flowPrefetchIssuedKeyRef.current = prefetchKey;
      const activePrefetch = flowPrefetchJobRef.current;
      if (activePrefetch.jobId && activePrefetch.key && activePrefetch.key !== prefetchKey) {
        void cancelAnalysisFlowBuildJob(activePrefetch.jobId).catch(() => {});
        flowPrefetchJobRef.current = { jobId: "", key: "" };
      }
      void createAnalysisFlowBuildJob(jobPayload)
        .then((job) => {
          if (flowPrefetchSeqRef.current !== seq) {
            const staleJobId = String(job.job_id || "").trim();
            if (staleJobId) {
              void cancelAnalysisFlowBuildJob(staleJobId).catch(() => {});
            }
            return;
          }
          flowPrefetchJobRef.current = {
            jobId: String(job.job_id || "").trim(),
            key: prefetchKey
          };
          if (job.status === "failed" || job.status === "canceled") {
            if (flowPrefetchIssuedKeyRef.current === prefetchKey) {
              flowPrefetchIssuedKeyRef.current = "";
            }
          }
        })
        .catch(() => {
          if (flowPrefetchIssuedKeyRef.current === prefetchKey) {
            flowPrefetchIssuedKeyRef.current = "";
          }
        });
    }, 240);

    return () => {
      if (flowPrefetchTimerRef.current != null) {
        window.clearTimeout(flowPrefetchTimerRef.current);
        flowPrefetchTimerRef.current = null;
      }
    };
  }, [activeCaseId, activeRow, activeWorkspaceTab, dateEnd, dateStart, loadingRows, loadingTxn, mode, selectedAccounts, treeTab, txnModalOpen, visibleSelectedRows]);

  useEffect(() => {
    return () => {
      if (flowPrefetchTimerRef.current != null) {
        window.clearTimeout(flowPrefetchTimerRef.current);
        flowPrefetchTimerRef.current = null;
      }
      const activePrefetch = flowPrefetchJobRef.current;
      flowPrefetchJobRef.current = { jobId: "", key: "" };
      if (activePrefetch.jobId) {
        void cancelAnalysisFlowBuildJob(activePrefetch.jobId).catch(() => {});
      }
    };
  }, []);

  const statsHeaderActions = (
    <div className="stats-page-topbar__actions">
      <WorkbenchButton className="chipBtn navStyle ghost" tone="ghost" type="button" onClick={onResetFilters}>
        重置
      </WorkbenchButton>
    </div>
  );

  const statsHeaderFilters = (
    <div className="stats-page-topbar__content">
      <div className="filters">
        <span className="stats-page-topbar__railLabel" aria-hidden="true">
          时间
        </span>
        <div className="dateRangeWrap">
          <div
            className="dateRangeGroup dateRangeTrigger"
            role="group"
            aria-label="时间范围"
            onPointerDownCapture={onHeaderDateGroupPointerDownCapture}
          >
            <div
              className="dateRangeField dateRangeTriggerField"
              ref={dateStartFieldRef}
              onPointerDown={(event) => onHeaderDateFieldPointerDown("start", event)}
            >
              <label
                className="dateRangeFieldText"
                htmlFor="stats-date-start-input"
                onPointerDown={(event) => onHeaderDateFieldPointerDown("start", event)}
              >
                <span className="dateRangeHint">起</span>
                <DateRangeTextInput
                  ariaLabel="最早时间"
                  className="dateRangeValueInput"
                  id="stats-date-start-input"
                  inputRef={dateStartInputRef}
                  name="stats-date-start"
                  value={dateStart}
                  min={dateMin || undefined}
                  max={dateEnd || dateMax || undefined}
                  onCommit={onHeaderDateInputCommit("start")}
                />
              </label>
              <button
                className={`dateRangeFieldTail ${datePopoverAnchor === "start" ? "is-open" : ""}`}
                type="button"
                ref={dateStartTriggerRef}
                aria-controls={datePopoverAnchor === "start" ? "stats-date-picker-popover" : undefined}
                aria-expanded={datePopoverAnchor === "start"}
                aria-haspopup="dialog"
                aria-label="打开最早时间选择器"
                onPointerDown={(event) => onHeaderDateTailPointerDown("start", event)}
                onClick={(event) => onHeaderDateTailClick("start", event)}
              >
                <span className="dateRangeFieldChevron">
                  <svg viewBox="0 0 24 24" fill="none">
                    <path d="m8 10 4 4 4-4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                </span>
              </button>
            </div>
            <div className="dateRangeDivider" aria-hidden="true" />
            <div
              className="dateRangeField dateRangeTriggerField"
              ref={dateEndFieldRef}
              onPointerDown={(event) => onHeaderDateFieldPointerDown("end", event)}
            >
              <label
                className="dateRangeFieldText"
                htmlFor="stats-date-end-input"
                onPointerDown={(event) => onHeaderDateFieldPointerDown("end", event)}
              >
                <span className="dateRangeHint">止</span>
                <DateRangeTextInput
                  ariaLabel="最晚时间"
                  className="dateRangeValueInput"
                  id="stats-date-end-input"
                  inputRef={dateEndInputRef}
                  name="stats-date-end"
                  value={dateEnd}
                  min={dateStart || dateMin || undefined}
                  max={dateMax || undefined}
                  onCommit={onHeaderDateInputCommit("end")}
                />
              </label>
              <button
                className={`dateRangeFieldTail ${datePopoverAnchor === "end" ? "is-open" : ""}`}
                type="button"
                ref={dateEndTriggerRef}
                aria-controls={datePopoverAnchor === "end" ? "stats-date-picker-popover" : undefined}
                aria-expanded={datePopoverAnchor === "end"}
                aria-haspopup="dialog"
                aria-label="打开最晚时间选择器"
                onPointerDown={(event) => onHeaderDateTailPointerDown("end", event)}
                onClick={(event) => onHeaderDateTailClick("end", event)}
              >
                <span className="dateRangeFieldChevron">
                  <svg viewBox="0 0 24 24" fill="none">
                    <path d="m8 10 4 4 4-4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                </span>
              </button>
            </div>
          </div>
        </div>
        <div
          className={`filtersSwitchWrap ${
            activeQuickRange === "7"
              ? "is-7"
              : activeQuickRange === "180"
                ? "is-180"
                : activeQuickRange === "365"
                  ? "is-365"
                  : activeQuickRange === "all"
                    ? "is-all"
                    : "is-none"
          }`}
          aria-label="快捷时间范围"
        >
          <WorkbenchSegmentedControl
            className={`quickSwitch ${
              activeQuickRange === "7"
                ? "is-7"
                : activeQuickRange === "180"
                  ? "is-180"
                  : activeQuickRange === "365"
                    ? "is-365"
                    : activeQuickRange === "all"
                      ? "is-all"
                      : "is-none"
            }`}
            optionClassName="quickSwitchChip"
            indicatorClassName="quickSwitchIndicator"
            ariaLabel="快捷时间范围"
            value={activeQuickRange || null}
            options={QUICK_RANGE_OPTIONS}
            onChange={(nextValue) => {
              if (nextValue === "all") {
                onQuickRange("all");
                return;
              }
              onQuickRange(Number(nextValue));
            }}
          />
        </div>
        {statsHeaderActions}
      </div>
    </div>
  );

  const statsHeaderTools = (
    <div className="stats-page-topbar__cluster">
      <div className="stats-page-topbar__rail" role="group" aria-label="统计筛选与操作">
        {statsHeaderFilters}
      </div>
    </div>
  );

  const txnTargetLabel = getTxnTargetLabel(activeRow);
  const txnFooterRows = activeRow ? EMPTY_STATS_TXN_ROWS : txnRows;
  const txnFooterSummary = useMemo(
    () => buildStatsTxnFooterSummary(activeRow, txnFooterRows),
    [activeRow, txnFooterRows]
  );
  const txnModalColumns = useMemo(
    () => buildStatsTxnModalColumns(txnSortCol, txnSortDir),
    [txnSortCol, txnSortDir]
  );
  const txnModalRows = useMemo(() => {
    const startedAt = readStatsPerformanceNow();
    const nextRows = buildStatsTxnModalRows(txnWindowRows, txnWindowStart);
    txnRenderModelTimingRef.current = {
      renderModelMs: roundStatsPerfMs(readStatsPerformanceNow() - startedAt),
      windowStart: txnWindowStart,
      windowRowCount: txnWindowRows.length,
      rowOverscan: TXN_OVERSCAN_ROWS,
      columnCount: txnModalColumns.length,
      totalColumnCount: txnModalColumns.length,
      cellCount: txnWindowRows.length * txnModalColumns.length,
    };
    return nextRows;
  }, [txnModalColumns.length, txnWindowRows, txnWindowStart]);

  return (
    <section className="stats-react-shell ui-classic-refined-shell" data-theme={resolvedTheme} ref={shellRef}>
      <style>{STATS_REACT_CSS}</style>
      <PageTopBar
        className={`stats-header stats-header--${activeWorkspaceTab}`}
        hideThemeToggle
        tabs={[
          {
            key: "stats",
            label: "统计分析",
            active: activeWorkspaceTab === "stats",
            onClick: () =>
              setChartAnalysisState((prev) =>
                prev.activeWorkspaceTab === "stats" ? prev : { ...prev, activeWorkspaceTab: "stats" }
              )
          },
          {
            key: "charts",
            label: "图表分析",
            active: activeWorkspaceTab === "charts",
            onClick: () =>
              setChartAnalysisState((prev) =>
                prev.activeWorkspaceTab === "charts" ? prev : { ...prev, activeWorkspaceTab: "charts" }
              )
          }
        ]}
        tabsAriaLabel="统计页面"
        toolsContent={statsHeaderTools}
      />
      <div className={`app ui-classic-refined ${leftCollapsed ? "left-collapsed" : ""}`} style={gridStyle}>
        <AnalysisTreePanel
          panelRef={leftPanelRef}
          toolbarActionsRef={leftToolbarActionsRef}
          searchInputRef={leftSearchInputRef}
          leftCollapsed={leftCollapsed}
          treeTab={treeTab}
          leftSearch={leftSearch}
          leftSearchExpanded={leftSearchExpanded}
          visibleGroups={visibleGroups}
          selectedAccounts={selectedAccounts}
          selectedAccountsSet={selectedAccountsSet}
          expandedGroupSet={expandedGroupSet}
          showTreeBlockingState={showTreeBlockingState}
          treeUnavailable={treeUnavailable}
          collapsedDimensionTitle={collapsedDimensionTitle}
          collapsedDimensionValue={collapsedDimensionValue}
          collapsedSelectedObjectCount={collapsedSelectedObjectCount}
          collapsedSelectedObjectCountText={collapsedSelectedObjectCountText}
          collapsedSelectedCardCountText={collapsedSelectedCardCountText}
          onTreeTabChange={setTreeTab}
          onLeftSearchChange={setLeftSearch}
          onOpenLeftSearch={openLeftSearch}
          onCollapseLeftSearch={collapseLeftSearch}
          onSelectAllAccounts={onSelectAllAccounts}
          onClearSelectedAccounts={clearSelectedAccounts}
          onGroupExpandToggle={toggleGroupExpand}
          onGroupSelectedChange={setGroupSelected}
          onAccountToggle={toggleAccount}
          onEmptySelectionContextMenu={() => pushToast("请先选择对象后执行批量操作", "warn")}
          onCopyTreeItemCardNumber={copyTreeItemCardNumber}
        />

        <button
          className="gapToggle"
          title={leftCollapsed ? "展开" : "折叠"}
          type="button"
          aria-label={leftCollapsed ? "展开左侧" : "折叠左侧"}
          aria-expanded={!leftCollapsed}
          onClick={() => setLeftCollapsed((prev) => !prev)}
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none">
            <path d="M15 6l-6 6 6 6" stroke="rgba(2,6,23,.6)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>

        <section className={`right ${activeWorkspaceTab === "charts" ? "right--charts" : ""}`}>
          {activeWorkspaceTab === "charts" ? (
            <ChartAnalysisDashboard
              activeCaseId={String(activeCaseId || "")}
              selectedAccounts={selectedAccounts}
              selectedObjectCount={collapsedSelectedObjectCount}
              dateStart={dateStart}
              dateEnd={dateEnd}
              emptyStateMode={chartEmptyStateMode}
              leftSearch={leftSearch}
              chartState={chartAnalysisState}
              setChartState={setChartAnalysisState}
            />
          ) : (
            <>
              <section className="modePanel">
                <div className="modePanelHead">
                  <div>
                    <div className="modePanelTitle">聚合</div>
                    <div className="panelEyebrow">方式</div>
                  </div>
                </div>
                <div className="modeRow" role="tablist" aria-label="聚合方式">
                  {MODE_OPTIONS.map(({ key: modeKey, title, dimension, direction, flow, subject }) => {
                    return (
                      <WorkbenchModeCard
                        className={`modeCard ${mode === modeKey ? "active" : ""}`}
                        active={mode === modeKey}
                        data-mode={modeKey}
                        type="button"
                        key={modeKey}
                        role="tab"
                        aria-selected={mode === modeKey}
                        onClick={() => setMode(modeKey)}
                      >
                        <div className="mLead" aria-hidden="true">
                          <div className="mIconStack">
                            <span className="mIconBadge mIconBadgePrimary">{renderModeFlowIcon(flow)}</span>
                            <span className="mIconBadge mIconBadgeSecondary">{renderModeSubjectIcon(subject)}</span>
                          </div>
                        </div>
                        <div className="mText">
                          <div className="mTitle">{title}</div>
                          <div className="mMetaLine">
                            <span>{dimension}</span>
                            <span className="mMetaSep" aria-hidden="true">
                              ·
                            </span>
                            <span>{direction}</span>
                          </div>
                        </div>
                      </WorkbenchModeCard>
                    );
                  })}
                </div>
              </section>

              <WorkbenchTableShell className="card tableCard cardSolid">
                <WorkbenchTableToolbar className="tableToolbar">
                  <div className="tRight">
                    <WorkbenchTableActions className="rowActions">
                      <WorkbenchButton className="chipBtn navStyle" type="button" disabled={loadingRows || !visibleRows.length} onClick={() => openFlow("relation")}>
                        批量关联图谱
                      </WorkbenchButton>
                      <WorkbenchButton className="chipBtn navStyle" type="button" disabled={loadingRows || !visibleRows.length} onClick={() => openFlow("net")}>
                        批量净值流向
                      </WorkbenchButton>
                    </WorkbenchTableActions>
                    <WorkbenchSearchField
                      className="tableSearchField tableToolbarSearchField"
                      inputClassName="toolbarSearchInput tableToolbarSearchInput"
                      clearClassName="toolbarSearchClear"
                      wrapperRole="search"
                      value={tableSearch}
                      onChange={setTableSearch}
                      id="tableSearch"
                      placeholder="搜索：对方账号/开户名/归属地"
                      autoComplete="off"
                    />
                  </div>
                </WorkbenchTableToolbar>

                <div className="gridScrollFrame">
                  <div
                    className="gridScroll"
                    ref={gridScrollRef}
                    onScroll={(event) => {
                      scheduleGridScrollTopUpdate(event.currentTarget.scrollTop);
                      if (drawerOpen || drawerClosing) {
                        scheduleDrawerLayout();
                      }
                    }}
                    onWheel={(event) => {
                      const horizontalDelta = event.deltaX || (event.shiftKey ? event.deltaY : 0);
                      if (!horizontalDelta) {
                        return;
                      }
                      const element = event.currentTarget;
                      const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
                      if (maxScrollLeft <= 0) {
                        return;
                      }
                      const currentScrollLeft = element.scrollLeft;
                      const nextScrollLeft = Math.min(maxScrollLeft, Math.max(0, currentScrollLeft + horizontalDelta));
                      if (nextScrollLeft === currentScrollLeft) {
                        return;
                      }
                      element.scrollLeft = nextScrollLeft;
                      if (event.shiftKey || Math.abs(horizontalDelta) >= Math.abs(event.deltaY)) {
                        event.preventDefault();
                      }
                    }}
                  >
                    <div className="gridInner" style={gridStyle}>
                      <div className="gridHeader" ref={gridHeaderRef} style={{ gridTemplateColumns: gridCols }}>
                        {columns.map((column, index) => {
                          const sorted = tableSort.colId === column.id;
                          const sortText = sorted ? (tableSort.dir === 1 ? "↑" : "↓") : "↕";
                          if (column.id === "__sel") {
                            return (
                              <div className="hCell sel" key={column.id}>
                                <button
                                  type="button"
                                  className={`cb ${allVisibleRowsSelected ? "checked" : ""}`}
                                  disabled={loadingRows}
                                  title={allVisibleRowsSelected ? "取消选择全部可见账户" : "选择全部可见账户"}
                                  aria-label={allVisibleRowsSelected ? "取消选择全部可见账户" : "选择全部可见账户"}
                                  aria-pressed={allVisibleRowsSelected}
                                  onClick={() => onToggleSelectAllRows(!allVisibleRowsSelected)}
                                >
                                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none">
                                    <path d="M20 6L9 17l-5-5" stroke="rgba(31,111,235,.95)" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
                                  </svg>
                                </button>
                                <div className="resizer" onMouseDown={(event) => onStartResize(index, event)} />
                              </div>
                            );
                          }
                          return (
                            <div className={`hCell ${sorted ? "sorted" : ""}`} key={column.id} onClick={() => onSortColumn(column.id)}>
                              <span>{column.title}</span>
                              <span className="hSort">{sortText}</span>
                              <div className="resizer" onMouseDown={(event) => onStartResize(index, event)} />
                            </div>
                          );
                        })}
                      </div>
                      <div className="gridCanvas" style={{ height: `${gridCanvasHeight}px` }}>
                        {visibleRows.length
                          ? gridWindowRows.map((row, windowIndex) => {
                            const rowIndex = gridWindowStart + windowIndex;
                            const rowSelected = selectedRowSet.has(row.id);
                            const rowClass = [
                              "row",
                              rowIndex % 2 === 0 ? "even" : "",
                              rowSelected ? "selected" : ""
                            ]
                              .filter(Boolean)
                              .join(" ");
                            return (
                              <div
                                className={rowClass}
                                key={row.id}
                                style={{
                                  top: `${rowIndex * ROW_HEIGHT}px`,
                                  height: `${ROW_HEIGHT}px`,
                                  gridTemplateColumns: gridCols
                                }}
                                onClick={() => {
                                  if (!loadingRows) {
                                    openDrawer(row);
                                  }
                                }}
                                onDoubleClick={() => openTxnModal(row, "all")}
                              >
                                <div className="cell sel">
                                  <button
                                    type="button"
                                    className={`cb ${rowSelected ? "checked" : ""}`}
                                    disabled={loadingRows}
                                    title={rowSelected ? "取消选择该账户" : "选择该账户"}
                                    aria-label={`${rowSelected ? "取消选择" : "选择"}账户 ${row.counterparty_name || projectOrdinaryFieldValue("counterparty_account", row.counterparty_account) || "未命名账户"}`}
                                    aria-pressed={rowSelected}
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      onToggleRowSelect(row.id, !rowSelected);
                                    }}
                                  >
                                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none">
                                      <path d="M20 6L9 17l-5-5" stroke="rgba(31,111,235,.95)" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
                                    </svg>
                                  </button>
                                </div>
                                {columns.slice(1).map((column) => {
                                  const data = row as unknown as Record<string, unknown>;
                                  const raw = data[column.id];
                                  if (column.id === "doc") {
                                    return (
                                      <div className="cell" key={column.id}>
                                        <span className={getDocTagClass(String(raw || "未调单"))}>{String(raw || "未调单")}</span>
                                      </div>
                                    );
                                  }
                                  if (column.id === "total_count") {
                                    return renderTxnCountCell(row, "all", raw, column.id);
                                  }
                                  if (column.id === "in_count") {
                                    return renderTxnCountCell(row, "in", raw, column.id);
                                  }
                                  if (column.id === "out_count") {
                                    return renderTxnCountCell(row, "out", raw, column.id);
                                  }
                                  if (column.type === "money" || column.type === "net") {
                                    return (
                                      <div className="cell num" key={column.id}>
                                        {fmtMoney(raw)}
                                      </div>
                                    );
                                  }
                                  if (column.type === "count") {
                                    return (
                                      <div className="cell num" key={column.id}>
                                        {formatCaseFactCount(raw)}
                                      </div>
                                    );
                                  }
                                  return (
                                    <div className="cell" key={column.id}>
                                      {displayBlank(projectOrdinaryFieldValue(column.id, raw))}
                                    </div>
                                  );
                                })}
                              </div>
                            );
                          })
                        : showEmptyGrid
                          ? emptyGridRows.map((rowIndex) => (
                              <div
                                aria-hidden="true"
                                className={`row placeholder ${rowIndex % 2 === 0 ? "even" : ""}`.trim()}
                                key={`placeholder-${rowIndex}`}
                                style={{
                                  top: `${rowIndex * ROW_HEIGHT}px`,
                                  height: `${ROW_HEIGHT}px`,
                                  gridTemplateColumns: gridCols
                                }}
                              >
                                {columns.map((column) => (
                                  <div className={`cell ${column.id === "__sel" ? "sel" : ""}`.trim()} key={column.id} />
                                ))}
                              </div>
                            ))
                          : null}
                    </div>

                    <div
                      className={`drawerWrap ${drawerOpen ? "active" : ""}`}
                      style={{ "--dx": `${drawerPos.left}px`, "--dy": `${drawerPos.top}px` } as CSSProperties}
                    >
                      <aside
                        ref={drawerRef}
                        className={`drawer ${drawerOpen ? "open" : ""} ${drawerClosing ? "closing" : ""}`.trim()}
                        aria-hidden={!drawerOpen}
                        onTransitionEnd={(event) => {
                          if (event.target !== event.currentTarget || event.propertyName !== "transform") {
                            return;
                          }
                          if (!drawerOpen && drawerClosing) {
                            setDrawerClosing(false);
                            setDrawerRowId("");
                          }
                        }}
                      >
                        <div className="dHead">
                          <div className="dTitle">详情 - {drawerRow?.counterparty_name || projectOrdinaryFieldValue("counterparty_account", drawerRow?.counterparty_account) || ""}</div>
                          <button className="dClose" type="button" onClick={closeDrawer}>
                            关闭
                          </button>
                        </div>
                        <div className="dBody">
                          {drawerRow ? (
                            <>
                              <div className="kv">
                                <div className="k">对方账号</div>
                                <div className="v">{projectOrdinaryFieldValue("counterparty_account", drawerRow.counterparty_account)}</div>
                                <div className="k">开户名</div>
                                <div className="v">{drawerRow.counterparty_name || ""}</div>
                                <div className="k">归属地</div>
                                <div className="v">{drawerRow.location || ""}</div>
                                <div className="k">归属行</div>
                                <div className="v">{drawerRow.bank || ""}</div>
                                <div className="k">社会关系</div>
                                <div className="v">{drawerRow.relation || ""}</div>
                                <div className="k">已调单</div>
                                <div className="v">{drawerRow.doc || ""}</div>
                                <div className="k">总金额</div>
                                <div className="v">{fmtMoney(drawerRow.total_amount)}</div>
                                <div className="k">流入金额</div>
                                <div className="v">{fmtMoney(drawerRow.in_amount)}</div>
                                <div className="k">流出金额</div>
                                <div className="v">{fmtMoney(drawerRow.out_amount)}</div>
                                <div className="k">总次数</div>
                                <div className="v">{formatCaseFactCount(drawerRow.total_count)}</div>
                                <div className="k">流入/流出次</div>
                                <div className="v">
                                  {formatCaseFactCount(drawerRow.in_count)} / {formatCaseFactCount(drawerRow.out_count)}
                                </div>
                                <div className="k">最早/最晚</div>
                                <div className="v">{[drawerRow.first_time || "", drawerRow.last_time || ""].filter(Boolean).join(" ~ ")}</div>
                              </div>
                              <div className="miniTitle">快捷操作</div>
                              <div className="timeline">
                                <div className="event" onClick={() => openTxnModal(drawerRow, "all")}>
                                  <span className="time">查看明细</span>
                                  <span>打开交易明细弹窗</span>
                                  <span className="amt">共 {formatCaseFactCount(drawerRow.total_count)} 笔</span>
                                </div>
                                <div className="event" onClick={() => openFlow("relation")}>
                                  <span className="time">图谱</span>
                                  <span>推送到关联图谱</span>
                                  <span className="amt">关联</span>
                                </div>
                              </div>
                            </>
                          ) : (
                            <div className="timelineEmpty">请选择一行查看详情</div>
                          )}
                        </div>
                      </aside>
                    </div>
                  </div>
                </div>
                  <PersistentHorizontalScrollbar
                    className="gridPersistentHorizontalScrollbar"
                    scrollRef={gridScrollRef}
                    ariaLabel="统计表横向滚动条"
                  />
                </div>
                <div className="gridHeaderGutterFill" aria-hidden="true" />

                <WorkbenchTableFooter className="tableFooter">
                  {rowsFactUnavailable ? (
                    <div>统计事实未发布：缺少当前案件的宿主证据回执</div>
                  ) : (
                    <>
                      <div>
                        共 <span className="footMono workbench-ui-table-footer__mono">{rowsStatus === "no-selection" ? "--" : visibleRows.length}</span> 行
                      </div>
                      <div>
                        总金额 <span className="footMono workbench-ui-table-footer__mono">{fmtMoney(totalAmount)}</span>｜总次数{" "}
                        <span className="footMono workbench-ui-table-footer__mono">{formatCaseFactCount(totalCount)}</span>
                      </div>
                    </>
                  )}
                </WorkbenchTableFooter>
              </WorkbenchTableShell>
            </>
          )}
        </section>
      </div>

      {datePopoverAnchor && typeof document !== "undefined"
        ? createPortal(
            <div
              id="stats-date-picker-popover"
              ref={datePopoverRef}
              className="statsDateEditorPopover"
              data-theme={resolvedTheme}
              style={datePopoverPosition}
              role="dialog"
              aria-label={datePopoverAnchor === "start" ? "最早时间选择器" : "最晚时间选择器"}
            >
              <div className="dateCalendarPanel">
                <div className="dateCalendarHead">
                  <button
                    className="dateCalendarNavBtn"
                    type="button"
                    aria-label="上一个月"
                    disabled={datePopoverPrevDisabled}
                    onClick={() => setDatePopoverMonth((prev) => addMonths(prev, -1))}
                  >
                    <svg viewBox="0 0 24 24" fill="none">
                      <path d="M14 7l-5 5 5 5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  </button>
                  <div className="dateCalendarMonthLabel">{datePopoverMonthLabel}</div>
                  <button
                    className="dateCalendarNavBtn"
                    type="button"
                    aria-label="下一个月"
                    disabled={datePopoverNextDisabled}
                    onClick={() => setDatePopoverMonth((prev) => addMonths(prev, 1))}
                  >
                    <svg viewBox="0 0 24 24" fill="none">
                      <path d="M10 7l5 5-5 5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  </button>
                </div>
                <div className="dateCalendarWeekdays" aria-hidden="true">
                  {DATE_CALENDAR_WEEKDAY_LABELS.map((label) => (
                    <span key={label}>{label}</span>
                  ))}
                </div>
                <div className="dateCalendarGrid">
                  {datePopoverCells.map((cell) => (
                    <button
                      key={cell.value}
                      className={[
                        "dateCalendarCell",
                        cell.current ? "current" : "adjacent",
                        cell.disabled ? "disabled" : "",
                        cell.today ? "today" : "",
                        cell.inRange ? "in-range" : "",
                        cell.isRangeStart ? "range-start" : "",
                        cell.isRangeEnd ? "range-end" : "",
                        cell.anchorActive ? "anchor-active" : ""
                      ]
                        .filter(Boolean)
                        .join(" ")}
                      type="button"
                      disabled={cell.disabled}
                      aria-pressed={cell.anchorActive}
                      onClick={() => selectDateFromPopover(datePopoverAnchor, cell.value)}
                    >
                      {cell.value.slice(-2).replace(/^0/, "")}
                    </button>
                  ))}
                </div>
              </div>
            </div>,
            document.body
          )
        : null}

      <TxnDetailDialog
        open={txnModalOpen}
        ariaLabel="交易明细"
        title="交易明细"
        meta={txnTargetLabel}
        columns={txnModalColumns}
        rows={txnModalRows}
        loading={loadingTxn}
        emptyText={txnFactAnswerAllowed ? "暂无明细" : "交易明细未发布：缺少当前案件的宿主证据回执"}
        onClose={() => setTxnModalOpen(false)}
        onToggleSort={(columnKey) => {
          const sortKey = columnKey as StatsTxnSortCol;
          if (txnSortCol === sortKey) {
            setTxnSortDir((prev) => (prev === "asc" ? "desc" : "asc"));
            return;
          }
          setTxnSortCol(sortKey);
          setTxnSortDir(sortKey === "amount" ? "desc" : "asc");
        }}
        tableWrapRef={txnTableWrapRef}
        tableWrapClassName={loadingTxn ? "is-sorting" : ""}
        onTableScroll={(event) => {
          txnViewport.onTableScroll(event);
          maybeLoadMoreTxnRows(event.currentTarget);
        }}
        onTableWheel={txnViewport.onTableWheel}
        wrapClassName="txnModalWrap txnModalThemeStats stats-txn-modal"
        topSpacerHeight={txnTopSpacerHeight}
        bottomSpacerHeight={txnBottomSpacerHeight}
        extraToolbarActions={
          <div className="txnModalFilterSwitchWrap" aria-label="交易方向筛选">
            <WorkbenchSegmentedControl
              className="quickSwitch"
              optionClassName="quickSwitchChip"
              indicatorClassName="quickSwitchIndicator"
              ariaLabel="交易方向筛选"
              value={txnFilter}
              options={TXN_FILTER_OPTIONS}
              onChange={(value) => setTxnFilter(value as TxnFilter)}
            />
          </div>
        }
        footerContent={txnFactAnswerAllowed ? (
          <>
              <div className="txnModalFooterSummary">
                <span className="txnModalFooterSummaryLabel">对象统计</span>
                <span>
                  总次数 <span className="footMono workbench-ui-table-footer__mono">{txnFooterSummary.totalCount}</span>
                </span>
                <span>
                  流入 <span className="footMono workbench-ui-table-footer__mono">{txnFooterSummary.inCount}</span> 笔 /{" "}
                  <span className="footMono workbench-ui-table-footer__mono">{fmtMoney(txnFooterSummary.inAmount)}</span>
                </span>
                <span>
                  流出 <span className="footMono workbench-ui-table-footer__mono">{txnFooterSummary.outCount}</span> 笔 /{" "}
                  <span className="footMono workbench-ui-table-footer__mono">{fmtMoney(txnFooterSummary.outAmount)}</span>
                </span>
                <span>
                  时间范围 <span className="footMono workbench-ui-table-footer__mono">{txnFooterSummary.firstTime || "-"}</span> ~{" "}
                  <span className="footMono workbench-ui-table-footer__mono">{txnFooterSummary.lastTime || "-"}</span>
                </span>
              </div>
          </>
        ) : null}
      />

    </section>
  );
}
