import type { StatsTxnRowDTO, StatsV2ChartDetailRowsDTO } from "./stats-api";

const STATS_CHART_DETAIL_CACHE_LIMIT = 8;
const STATS_CHART_DETAIL_CACHE_TTL_MS = 20_000;

export interface StatsChartDetailCacheEntry {
  rows: StatsTxnRowDTO[];
  total: number | null;
  allRowsLoaded: boolean;
  expiresAt: number;
}

type SharedStatsChartDetailStore = {
  cache: Map<string, StatsChartDetailCacheEntry>;
  inflight: Map<string, Promise<StatsV2ChartDetailRowsDTO>>;
};

declare global {
  interface Window {
    __ANALYTIX_SHARED_STATS_CHART_DETAIL_STORE__?: SharedStatsChartDetailStore;
  }
}

const fallbackStore: SharedStatsChartDetailStore = {
  cache: new Map<string, StatsChartDetailCacheEntry>(),
  inflight: new Map<string, Promise<StatsV2ChartDetailRowsDTO>>(),
};

function getSharedStatsChartDetailStore(): SharedStatsChartDetailStore {
  if (typeof window === "undefined") {
    return fallbackStore;
  }
  if (window.__ANALYTIX_SHARED_STATS_CHART_DETAIL_STORE__) {
    return window.__ANALYTIX_SHARED_STATS_CHART_DETAIL_STORE__;
  }
  window.__ANALYTIX_SHARED_STATS_CHART_DETAIL_STORE__ = {
    cache: new Map<string, StatsChartDetailCacheEntry>(),
    inflight: new Map<string, Promise<StatsV2ChartDetailRowsDTO>>(),
  };
  return window.__ANALYTIX_SHARED_STATS_CHART_DETAIL_STORE__;
}

function stableStringify(value: unknown): string {
  return JSON.stringify(value, (_key, rawValue) => {
    if (Array.isArray(rawValue)) {
      return rawValue;
    }
    if (rawValue && typeof rawValue === "object") {
      return Object.keys(rawValue as Record<string, unknown>)
        .sort()
        .reduce<Record<string, unknown>>((acc, key) => {
          acc[key] = (rawValue as Record<string, unknown>)[key];
          return acc;
        }, {});
    }
    return rawValue;
  });
}

function trimCache(store: SharedStatsChartDetailStore, now = Date.now()): void {
  Array.from(store.cache.entries()).forEach(([key, entry]) => {
    if (entry.expiresAt <= now) {
      store.cache.delete(key);
    }
  });
  while (store.cache.size > STATS_CHART_DETAIL_CACHE_LIMIT) {
    const oldestKey = store.cache.keys().next().value;
    if (!oldestKey) {
      break;
    }
    store.cache.delete(oldestKey);
  }
}

function toFiniteNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function toKnownNonNegativeInteger(value: unknown): number | null {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

function parseDateLike(value: unknown): number | null {
  const text = String(value || "").trim();
  if (!text) {
    return null;
  }
  const normalized = text.includes("T") ? text : text.includes(" ") ? text.replace(" ", "T") : `${text}T00:00:00`;
  const timestamp = Date.parse(normalized);
  return Number.isFinite(timestamp) ? timestamp : null;
}

function compareText(left: unknown, right: unknown): number {
  return String(left || "").localeCompare(String(right || ""), "zh-CN");
}

export function buildStatsChartDetailCacheKey(input: {
  case_id: string;
  selected: string[];
  date_start: string;
  date_end: string;
  direction_mode: string;
  success_filter: string;
  cash_filter: string;
  chart_filters: Array<{
    source_panel_id?: string;
    dimension?: string;
    value?: string;
    payload?: Record<string, unknown>;
  }>;
  page: number;
  limit: number;
}): string {
  const filters = (Array.isArray(input.chart_filters) ? input.chart_filters : [])
    .map((item) => ({
      source_panel_id: String(item.source_panel_id || "").trim(),
      dimension: String(item.dimension || "").trim(),
      value: String(item.value || "").trim(),
      payload: item.payload && typeof item.payload === "object" && !Array.isArray(item.payload) ? item.payload : {},
    }))
    .sort((left, right) =>
      `${left.dimension}:${left.value}:${left.source_panel_id}:${stableStringify(left.payload)}`.localeCompare(
        `${right.dimension}:${right.value}:${right.source_panel_id}:${stableStringify(right.payload)}`,
        "en"
      )
    );
  return stableStringify({
    case_id: String(input.case_id || "").trim(),
    selected: Array.from(new Set((input.selected || []).map((item) => String(item || "").trim()).filter(Boolean))).sort(),
    date_start: String(input.date_start || "").trim(),
    date_end: String(input.date_end || "").trim(),
    direction_mode: String(input.direction_mode || "").trim() || "all",
    success_filter: String(input.success_filter || "").trim() || "all",
    cash_filter: String(input.cash_filter || "").trim() || "all",
    chart_filters: filters,
    page: Math.max(1, Number(input.page || 1)),
    limit: Math.max(1, Number(input.limit || 200)),
  });
}

export function getCachedStatsChartDetail(key: string): StatsChartDetailCacheEntry | null {
  const store = getSharedStatsChartDetailStore();
  trimCache(store);
  const entry = store.cache.get(String(key || ""));
  if (!entry) {
    return null;
  }
  store.cache.delete(String(key || ""));
  store.cache.set(String(key || ""), entry);
  return entry;
}

export function setCachedStatsChartDetail(
  key: string,
  entry: { rows: StatsTxnRowDTO[]; total: number | null; allRowsLoaded: boolean }
): void {
  const store = getSharedStatsChartDetailStore();
  store.cache.set(String(key || ""), {
    rows: Array.isArray(entry.rows) ? entry.rows : [],
    total: toKnownNonNegativeInteger(entry.total),
    allRowsLoaded: entry.allRowsLoaded === true,
    expiresAt: Date.now() + STATS_CHART_DETAIL_CACHE_TTL_MS,
  });
  trimCache(store);
}

export function loadStatsChartDetailRows(
  key: string,
  loader: () => Promise<StatsV2ChartDetailRowsDTO>
): Promise<StatsV2ChartDetailRowsDTO> {
  const normalizedKey = String(key || "");
  const store = getSharedStatsChartDetailStore();
  const inflight = store.inflight.get(normalizedKey);
  if (inflight) {
    return inflight;
  }
  const request = Promise.resolve().then(loader);
  store.inflight.set(normalizedKey, request);
  request.then(
    () => {
      if (store.inflight.get(normalizedKey) === request) {
        store.inflight.delete(normalizedKey);
      }
    },
    () => {
      if (store.inflight.get(normalizedKey) === request) {
        store.inflight.delete(normalizedKey);
      }
    }
  );
  return request;
}

export function invalidateSharedStatsChartDetail(caseId = ""): void {
  const store = getSharedStatsChartDetailStore();
  const normalizedCaseId = String(caseId || "").trim();
  Array.from(store.cache.keys()).forEach((key) => {
    if (!normalizedCaseId || key.includes(`"case_id":"${normalizedCaseId}"`)) {
      store.cache.delete(key);
    }
  });
  Array.from(store.inflight.keys()).forEach((key) => {
    if (!normalizedCaseId || key.includes(`"case_id":"${normalizedCaseId}"`)) {
      store.inflight.delete(key);
    }
  });
}

export function sortCachedStatsChartDetailRows(
  rows: StatsTxnRowDTO[],
  sortCol: string,
  sortDir: "asc" | "desc"
): StatsTxnRowDTO[] {
  const reverse = sortDir === "desc";
  const items = [...(Array.isArray(rows) ? rows : [])];
  items.sort((left, right) => {
    let result = 0;
    if (sortCol === "amount") {
      result = compareKnownNumber(left.amount, right.amount, reverse);
    } else if (sortCol === "balance") {
      result = compareKnownNumber(left.balance, right.balance, reverse);
    } else if (sortCol === "counterparty_name") {
      result = compareText(left.counterparty_name, right.counterparty_name);
    } else {
      result = compareKnownNumber(parseDateLike(left.txn_time), parseDateLike(right.txn_time), reverse);
    }
    if (result === 0) {
      result = compareText(left.txn_id || left.id, right.txn_id || right.id);
      return reverse ? -result : result;
    }
    if (sortCol === "amount" || sortCol === "balance" || sortCol === "txn_time") {
      return result;
    }
    return reverse ? -result : result;
  });
  return items;
}

function compareKnownNumber(left: unknown, right: unknown, reverse: boolean): number {
  const leftNumber = toFiniteNumber(left);
  const rightNumber = toFiniteNumber(right);
  if (leftNumber === null || rightNumber === null) {
    if (leftNumber === rightNumber) {
      return 0;
    }
    return leftNumber === null ? 1 : -1;
  }
  const result = leftNumber - rightNumber;
  return reverse ? -result : result;
}
