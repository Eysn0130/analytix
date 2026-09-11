import { queryStatsV2Meta, type StatsV2MetaDTO } from "./stats-api";

const STATS_META_CACHE_LIMIT = 12;
const STATS_META_CACHE_TTL_MS = 5_000;

type SharedStatsMetaStore = {
  cache: Map<string, { expiresAt: number; value: StatsV2MetaDTO }>;
  inflight: Map<string, Promise<StatsV2MetaDTO>>;
};

type StatsMetaHost = typeof globalThis & {
  __ANALYTIX_SHARED_STATS_META_STORE__?: SharedStatsMetaStore;
};

const fallbackStore: SharedStatsMetaStore = {
  cache: new Map<string, { expiresAt: number; value: StatsV2MetaDTO }>(),
  inflight: new Map<string, Promise<StatsV2MetaDTO>>(),
};

function getSharedStatsMetaStore(): SharedStatsMetaStore {
  const host = globalThis as StatsMetaHost;
  if (host.__ANALYTIX_SHARED_STATS_META_STORE__) {
    return host.__ANALYTIX_SHARED_STATS_META_STORE__;
  }
  if (typeof window === "undefined") {
    return fallbackStore;
  }
  const store: SharedStatsMetaStore = {
    cache: new Map<string, { expiresAt: number; value: StatsV2MetaDTO }>(),
    inflight: new Map<string, Promise<StatsV2MetaDTO>>(),
  };
  host.__ANALYTIX_SHARED_STATS_META_STORE__ = store;
  return store;
}

function trimCache(store: SharedStatsMetaStore, now = Date.now()): void {
  Array.from(store.cache.entries()).forEach(([key, entry]) => {
    if (entry.expiresAt <= now) {
      store.cache.delete(key);
    }
  });
  while (store.cache.size > STATS_META_CACHE_LIMIT) {
    const oldestKey = store.cache.keys().next().value;
    if (!oldestKey) {
      break;
    }
    store.cache.delete(oldestKey);
  }
}

export function invalidateSharedStatsMeta(caseId = ""): void {
  const store = getSharedStatsMetaStore();
  const normalizedCaseId = String(caseId || "").trim();
  Array.from(store.cache.keys()).forEach((key) => {
    if (!normalizedCaseId || key === normalizedCaseId) {
      store.cache.delete(key);
    }
  });
  Array.from(store.inflight.keys()).forEach((key) => {
    if (!normalizedCaseId || key === normalizedCaseId) {
      store.inflight.delete(key);
    }
  });
}

export function querySharedStatsMeta(caseId: string, options?: { force?: boolean }): Promise<StatsV2MetaDTO> {
  const normalizedCaseId = String(caseId || "").trim();
  if (!normalizedCaseId) {
    return Promise.resolve({
      contract: "StatsMetaPublicBoundaryV1",
      semantic_status: "blocked",
      fact_answer_allowed: false,
      blocker: "case_unavailable",
      case_id: "",
      funds_status: "no-case",
      date_min: "",
      date_max: "",
    });
  }

  const store = getSharedStatsMetaStore();
  const force = Boolean(options?.force);
  const now = Date.now();
  trimCache(store, now);
  if (force) {
    store.cache.delete(normalizedCaseId);
  } else {
    const cached = store.cache.get(normalizedCaseId);
    if (cached && cached.expiresAt > now) {
      return Promise.resolve(cached.value);
    }
    const inflight = store.inflight.get(normalizedCaseId);
    if (inflight) {
      return inflight;
    }
  }

  const request = queryStatsV2Meta()
    .then((meta) => {
      if (store.inflight.get(normalizedCaseId) === request) {
        store.cache.set(normalizedCaseId, {
          expiresAt: Date.now() + STATS_META_CACHE_TTL_MS,
          value: meta,
        });
        trimCache(store);
      }
      return meta;
    })
    .finally(() => {
      if (store.inflight.get(normalizedCaseId) === request) {
        store.inflight.delete(normalizedCaseId);
      }
    });
  store.inflight.set(normalizedCaseId, request);
  return request;
}
