import type { StatsV2RowsDTO, StatsV2RowsTimedResult } from "./stats-api";

interface StatsRowsCacheEntry {
  rows: StatsV2RowsDTO;
  expiresAt: number;
}

type SharedStatsRowsStore = {
  cache: Map<string, StatsRowsCacheEntry>;
  inflight: Map<string, Promise<StatsRowsLoadResult>>;
};

type StatsRowsHost = typeof globalThis & {
  __ANALYTIX_SHARED_STATS_ROWS_STORE__?: SharedStatsRowsStore;
};

const fallbackStore: SharedStatsRowsStore = {
  cache: new Map<string, StatsRowsCacheEntry>(),
  inflight: new Map<string, Promise<StatsRowsLoadResult>>(),
};

export interface StatsRowsLoadResult {
  rows: StatsV2RowsDTO;
  timing: StatsV2RowsTimedResult["timing"] | null;
  source: "cache" | "network-or-inflight";
}

function getSharedStatsRowsStore(): SharedStatsRowsStore {
  const host = globalThis as StatsRowsHost;
  if (host.__ANALYTIX_SHARED_STATS_ROWS_STORE__) {
    return host.__ANALYTIX_SHARED_STATS_ROWS_STORE__;
  }
  if (typeof window === "undefined") {
    return fallbackStore;
  }
  const store: SharedStatsRowsStore = {
    cache: new Map<string, StatsRowsCacheEntry>(),
    inflight: new Map<string, Promise<StatsRowsLoadResult>>(),
  };
  host.__ANALYTIX_SHARED_STATS_ROWS_STORE__ = store;
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

export function buildStatsRowsCacheKey(input: {
  case_id: string;
  mode: string;
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
}): string {
  return stableStringify({
    case_id: String(input.case_id || "").trim(),
    mode: String(input.mode || "").trim(),
    selected: Array.from(new Set((input.selected || []).map((item) => String(item || "").trim()).filter(Boolean))).sort(),
    date_start: String(input.date_start || "").trim(),
    date_end: String(input.date_end || "").trim(),
    search_text: String(input.search_text || "").trim(),
    row_sort_col: String(input.row_sort_col || "").trim(),
    row_sort_dir: input.row_sort_dir === "asc" ? "asc" : "desc",
    row_offset: Math.max(0, Number(input.row_offset || 0)),
    row_limit: Math.max(0, Number(input.row_limit || 0)),
    row_format: input.row_format === "array" ? "array" : "object",
    fields: (input.fields || []).map((item) => String(item || "").trim()).filter(Boolean),
  });
}

export function getCachedStatsRows(key: string): StatsV2RowsDTO | null {
  const store = getSharedStatsRowsStore();
  const normalizedKey = String(key || "");
  store.cache.delete(normalizedKey);
  return null;
}

export function loadStatsRows(
  key: string,
  loader: () => Promise<StatsV2RowsTimedResult>
): Promise<StatsRowsLoadResult> {
  const normalizedKey = String(key || "");
  const cached = getCachedStatsRows(normalizedKey);
  if (cached) {
    return Promise.resolve({
      rows: cached,
      timing: null,
      source: "cache",
    });
  }
  const store = getSharedStatsRowsStore();
  const inflight = store.inflight.get(normalizedKey);
  if (inflight) {
    return inflight;
  }
  const request = Promise.resolve()
    .then(loader)
    .then((result) => {
      return {
        rows: result.data,
        timing: result.timing,
        source: "network-or-inflight" as const,
      };
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

export function invalidateSharedStatsRows(caseId = ""): void {
  const store = getSharedStatsRowsStore();
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
