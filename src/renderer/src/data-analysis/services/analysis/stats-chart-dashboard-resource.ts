import type { StatsV2ChartDashboardDTO } from "./stats-api";

const STATS_CHART_DASHBOARD_CACHE_LIMIT = 8;
const STATS_CHART_DASHBOARD_CACHE_TTL_MS = 20_000;

interface StatsChartDashboardCacheEntry {
  dashboard: StatsV2ChartDashboardDTO;
  expiresAt: number;
}

type SharedStatsChartDashboardStore = {
  cache: Map<string, StatsChartDashboardCacheEntry>;
  inflight: Map<string, Promise<StatsV2ChartDashboardDTO>>;
};

type StatsChartDashboardHost = typeof globalThis & {
  __ANALYTIX_SHARED_STATS_CHART_DASHBOARD_STORE__?: SharedStatsChartDashboardStore;
};

const fallbackStore: SharedStatsChartDashboardStore = {
  cache: new Map<string, StatsChartDashboardCacheEntry>(),
  inflight: new Map<string, Promise<StatsV2ChartDashboardDTO>>(),
};

function getSharedStatsChartDashboardStore(): SharedStatsChartDashboardStore {
  const host = globalThis as StatsChartDashboardHost;
  if (host.__ANALYTIX_SHARED_STATS_CHART_DASHBOARD_STORE__) {
    return host.__ANALYTIX_SHARED_STATS_CHART_DASHBOARD_STORE__;
  }
  if (typeof window === "undefined") {
    return fallbackStore;
  }
  const store: SharedStatsChartDashboardStore = {
    cache: new Map<string, StatsChartDashboardCacheEntry>(),
    inflight: new Map<string, Promise<StatsV2ChartDashboardDTO>>(),
  };
  host.__ANALYTIX_SHARED_STATS_CHART_DASHBOARD_STORE__ = store;
  return store;
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

function normalizeChartFilters(
  chartFilters: Array<{
    source_panel_id?: string;
    dimension?: string;
    value?: string;
    payload?: Record<string, unknown>;
  }>
): Array<{ source_panel_id: string; dimension: string; value: string; payload: Record<string, unknown> }> {
  return (Array.isArray(chartFilters) ? chartFilters : [])
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
}

function trimCache(store: SharedStatsChartDashboardStore, now = Date.now()): void {
  Array.from(store.cache.entries()).forEach(([key, entry]) => {
    if (entry.expiresAt <= now) {
      store.cache.delete(key);
    }
  });
  while (store.cache.size > STATS_CHART_DASHBOARD_CACHE_LIMIT) {
    const oldestKey = store.cache.keys().next().value;
    if (!oldestKey) {
      break;
    }
    store.cache.delete(oldestKey);
  }
}

export function buildStatsChartDashboardCacheKey(input: {
  case_id: string;
  selected: string[];
  date_start: string;
  date_end: string;
  metric_mode: string;
  direction_mode: string;
  granularity: string;
  success_filter: string;
  cash_filter: string;
  chart_filters: Array<{
    source_panel_id?: string;
    dimension?: string;
    value?: string;
    payload?: Record<string, unknown>;
  }>;
}): string {
  return stableStringify({
    case_id: String(input.case_id || "").trim(),
    selected: Array.from(new Set((input.selected || []).map((item) => String(item || "").trim()).filter(Boolean))).sort(),
    date_start: String(input.date_start || "").trim(),
    date_end: String(input.date_end || "").trim(),
    metric_mode: String(input.metric_mode || "").trim() || "amount",
    direction_mode: String(input.direction_mode || "").trim() || "all",
    granularity: String(input.granularity || "").trim() || "day",
    success_filter: String(input.success_filter || "").trim() || "all",
    cash_filter: String(input.cash_filter || "").trim() || "all",
    chart_filters: normalizeChartFilters(input.chart_filters),
  });
}

export function getCachedStatsChartDashboard(key: string): StatsV2ChartDashboardDTO | null {
  const store = getSharedStatsChartDashboardStore();
  trimCache(store);
  const normalizedKey = String(key || "");
  const entry = store.cache.get(normalizedKey);
  if (!entry) {
    return null;
  }
  store.cache.delete(normalizedKey);
  store.cache.set(normalizedKey, entry);
  return entry.dashboard;
}

export function loadStatsChartDashboard(
  key: string,
  loader: () => Promise<StatsV2ChartDashboardDTO>
): Promise<StatsV2ChartDashboardDTO> {
  const normalizedKey = String(key || "");
  const cached = getCachedStatsChartDashboard(normalizedKey);
  if (cached) {
    return Promise.resolve(cached);
  }
  const store = getSharedStatsChartDashboardStore();
  const inflight = store.inflight.get(normalizedKey);
  if (inflight) {
    return inflight;
  }
  const request = Promise.resolve()
    .then(loader)
    .then((dashboard) => {
      store.cache.set(normalizedKey, {
        dashboard,
        expiresAt: Date.now() + STATS_CHART_DASHBOARD_CACHE_TTL_MS,
      });
      trimCache(store);
      return dashboard;
    });
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

export function invalidateSharedStatsChartDashboard(caseId = ""): void {
  const store = getSharedStatsChartDashboardStore();
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
