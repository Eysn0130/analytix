import { queryStatsV2CaseOverview, type StatsV2CaseOverviewDTO } from "./stats-api";
import { subscribeStatsCacheInvalidation } from "./stats-cache-events";
import { ensureDesktopBackendRuntime } from "../desktop/client";
import { normalizeWorkspaceRoot } from "../../../lib/workspace-path";

const CASE_OVERVIEW_CACHE_LIMIT = 12;
const CASE_OVERVIEW_FRESH_TTL_MS = 30_000;
const CASE_OVERVIEW_RETAIN_TTL_MS = 10 * 60_000;

export interface SharedActiveCaseContext {
  caseId: string;
  workspaceRoot: string;
  updatedAt: number;
}

type ActiveCaseContextListener = (context: SharedActiveCaseContext | null) => void;
type CaseOverviewListener = (overview: StatsV2CaseOverviewDTO) => void;

type CaseOverviewCacheEntry = {
  expiresAt: number;
  staleAt: number;
  value: StatsV2CaseOverviewDTO;
};

type SharedCaseOverviewStore = {
  cache: Map<string, CaseOverviewCacheEntry>;
  inflight: Map<string, Promise<StatsV2CaseOverviewDTO>>;
  activeCaseContext: SharedActiveCaseContext | null;
  activeCaseListeners: Set<ActiveCaseContextListener>;
  overviewListeners: Map<string, Set<CaseOverviewListener>>;
  statsInvalidationUnsubscribe: (() => void) | null;
};

type CaseOverviewHost = typeof globalThis & {
  __ANALYTIX_SHARED_CASE_OVERVIEW_STORE__?: SharedCaseOverviewStore;
};

const fallbackStore: SharedCaseOverviewStore = {
  cache: new Map<string, CaseOverviewCacheEntry>(),
  inflight: new Map<string, Promise<StatsV2CaseOverviewDTO>>(),
  activeCaseContext: null,
  activeCaseListeners: new Set<ActiveCaseContextListener>(),
  overviewListeners: new Map<string, Set<CaseOverviewListener>>(),
  statsInvalidationUnsubscribe: null,
};

function emptyCaseOverview(caseId = ""): StatsV2CaseOverviewDTO {
  return {
    case_id: String(caseId || "").trim(),
    fact_answer_allowed: false,
    account_status: "unavailable",
    account_blocker: "case_unavailable",
    account_count: null,
    personal_account_count: null,
    corporate_account_count: null,
    unknown_account_count: null,
    transaction_status: "unavailable",
    transaction_blocker: "case_unavailable",
    transaction_count: null,
    amount_status: "unavailable",
    amount_blocker: "case_unavailable",
    inflow_amount: null,
    outflow_amount: null,
    amount_total_rows: null,
    amount_present_rows: null,
    amount_missing_rows: null,
    amount_parse_failed_rows: null,
    direction_covered_rows: null,
    source_table: "",
    account_source_table: "",
    source_revision: 1,
    generated_at: "",
  };
}

function getSharedCaseOverviewStore(): SharedCaseOverviewStore {
  const host = globalThis as CaseOverviewHost;
  if (host.__ANALYTIX_SHARED_CASE_OVERVIEW_STORE__) {
    ensureStatsInvalidationSubscription(host.__ANALYTIX_SHARED_CASE_OVERVIEW_STORE__);
    return host.__ANALYTIX_SHARED_CASE_OVERVIEW_STORE__;
  }
  if (typeof window === "undefined") {
    return fallbackStore;
  }
  const store: SharedCaseOverviewStore = {
    cache: new Map<string, CaseOverviewCacheEntry>(),
    inflight: new Map<string, Promise<StatsV2CaseOverviewDTO>>(),
    activeCaseContext: null,
    activeCaseListeners: new Set<ActiveCaseContextListener>(),
    overviewListeners: new Map<string, Set<CaseOverviewListener>>(),
    statsInvalidationUnsubscribe: null,
  };
  host.__ANALYTIX_SHARED_CASE_OVERVIEW_STORE__ = store;
  ensureStatsInvalidationSubscription(store);
  return store;
}

function ensureStatsInvalidationSubscription(store: SharedCaseOverviewStore): void {
  if (store.statsInvalidationUnsubscribe || typeof window === "undefined") {
    return;
  }
  store.statsInvalidationUnsubscribe = subscribeStatsCacheInvalidation((event) => {
    invalidateSharedCaseOverview(event.caseId);
    if (store.activeCaseContext?.caseId === event.caseId) {
      void preloadSharedCaseOverview(event.caseId, { force: true, keepStale: true }).catch(() => {});
    }
  });
}

function trimCache(store: SharedCaseOverviewStore, now = Date.now()): void {
  Array.from(store.cache.entries()).forEach(([key, entry]) => {
    if (entry.expiresAt <= now) {
      store.cache.delete(key);
    }
  });
  while (store.cache.size > CASE_OVERVIEW_CACHE_LIMIT) {
    const oldestKey = store.cache.keys().next().value;
    if (!oldestKey) {
      break;
    }
    store.cache.delete(oldestKey);
  }
}

function isFresh(entry: CaseOverviewCacheEntry, now = Date.now()): boolean {
  return entry.staleAt > now;
}

function emitActiveCaseContext(store: SharedCaseOverviewStore): void {
  const context = store.activeCaseContext;
  store.activeCaseListeners.forEach((listener) => {
    try {
      listener(context);
    } catch {
      // Context notifications are best-effort and must not block navigation.
    }
  });
}

function emitCaseOverview(store: SharedCaseOverviewStore, caseId: string, overview: StatsV2CaseOverviewDTO): void {
  const listeners = store.overviewListeners.get(caseId);
  if (!listeners) {
    return;
  }
  listeners.forEach((listener) => {
    try {
      listener(overview);
    } catch {
      // Overview listeners are UI hints; never block cache refresh.
    }
  });
}

function writeCaseOverviewCache(
  store: SharedCaseOverviewStore,
  caseId: string,
  overview: StatsV2CaseOverviewDTO
): void {
  store.cache.set(caseId, {
    expiresAt: Date.now() + CASE_OVERVIEW_RETAIN_TTL_MS,
    staleAt: Date.now() + CASE_OVERVIEW_FRESH_TTL_MS,
    value: overview,
  });
  trimCache(store);
  emitCaseOverview(store, caseId, overview);
}

export function setSharedActiveCaseContext(
  context: { caseId?: string | null; workspaceRoot?: string | null } | null
): SharedActiveCaseContext | null {
  const store = getSharedCaseOverviewStore();
  const caseId = String(context?.caseId || "").trim();
  const workspaceRoot = String(context?.workspaceRoot || "").trim();
  const previous = store.activeCaseContext;
  if (!caseId) {
    if (previous === null) {
      return null;
    }
    store.activeCaseContext = null;
    emitActiveCaseContext(store);
    return null;
  }
  if (previous?.caseId === caseId && previous.workspaceRoot === workspaceRoot) {
    return previous;
  }
  const next: SharedActiveCaseContext = {
    caseId,
    workspaceRoot,
    updatedAt: Date.now(),
  };
  store.activeCaseContext = next;
  emitActiveCaseContext(store);
  return next;
}

export function getSharedActiveCaseContext(): SharedActiveCaseContext | null {
  return getSharedCaseOverviewStore().activeCaseContext;
}

export function subscribeSharedActiveCaseContext(
  listener: ActiveCaseContextListener,
  options?: { emitCurrent?: boolean }
): () => void {
  const store = getSharedCaseOverviewStore();
  store.activeCaseListeners.add(listener);
  if (options?.emitCurrent) {
    listener(store.activeCaseContext);
  }
  return () => {
    store.activeCaseListeners.delete(listener);
  };
}

export function subscribeSharedCaseOverview(
  caseId: string,
  listener: CaseOverviewListener,
  options?: { emitCurrent?: boolean }
): () => void {
  const normalizedCaseId = String(caseId || "").trim();
  if (!normalizedCaseId) {
    return () => {};
  }
  const store = getSharedCaseOverviewStore();
  const listeners = store.overviewListeners.get(normalizedCaseId) ?? new Set<CaseOverviewListener>();
  listeners.add(listener);
  store.overviewListeners.set(normalizedCaseId, listeners);
  if (options?.emitCurrent) {
    const cached = readCachedSharedCaseOverview(normalizedCaseId);
    if (cached) {
      listener(cached);
    }
  }
  return () => {
    const current = store.overviewListeners.get(normalizedCaseId);
    if (!current) {
      return;
    }
    current.delete(listener);
    if (current.size === 0) {
      store.overviewListeners.delete(normalizedCaseId);
    }
  };
}

export function invalidateSharedCaseOverview(caseId = ""): void {
  const store = getSharedCaseOverviewStore();
  const normalizedCaseId = String(caseId || "").trim();
  const now = Date.now();
  Array.from(store.cache.entries()).forEach(([key, entry]) => {
    if (!normalizedCaseId || key === normalizedCaseId) {
      store.cache.set(key, { ...entry, staleAt: now - 1 });
    }
  });
  Array.from(store.inflight.keys()).forEach((key) => {
    if (!normalizedCaseId || key === normalizedCaseId) {
      store.inflight.delete(key);
    }
  });
}

export function readCachedSharedCaseOverview(caseId: string): StatsV2CaseOverviewDTO | null {
  const normalizedCaseId = String(caseId || "").trim();
  if (!normalizedCaseId) {
    return null;
  }
  const store = getSharedCaseOverviewStore();
  const now = Date.now();
  trimCache(store, now);
  const cached = store.cache.get(normalizedCaseId);
  return cached && cached.expiresAt > now ? cached.value : null;
}

function requestSharedCaseOverview(caseId: string, store: SharedCaseOverviewStore): Promise<StatsV2CaseOverviewDTO> {
  const existing = store.inflight.get(caseId);
  if (existing) {
    return existing;
  }
  const request = queryStatsV2CaseOverview()
    .then((overview) => {
      if (store.inflight.get(caseId) === request) {
        writeCaseOverviewCache(store, caseId, overview);
      }
      return overview;
    })
    .finally(() => {
      if (store.inflight.get(caseId) === request) {
        store.inflight.delete(caseId);
      }
    });
  store.inflight.set(caseId, request);
  return request;
}

export function querySharedCaseOverview(
  caseId: string,
  options?: { force?: boolean; keepStale?: boolean }
): Promise<StatsV2CaseOverviewDTO> {
  const normalizedCaseId = String(caseId || "").trim();
  if (!normalizedCaseId) {
    return Promise.resolve(emptyCaseOverview());
  }

  const store = getSharedCaseOverviewStore();
  const force = Boolean(options?.force);
  const now = Date.now();
  trimCache(store, now);
  const cached = store.cache.get(normalizedCaseId);
  if (force) {
    if (!options?.keepStale) {
      store.cache.delete(normalizedCaseId);
    }
    return requestSharedCaseOverview(normalizedCaseId, store);
  }

  if (cached && cached.expiresAt > now) {
    if (!isFresh(cached, now)) {
      void requestSharedCaseOverview(normalizedCaseId, store).catch(() => {});
    }
    return Promise.resolve(cached.value);
  }

  const inflight = store.inflight.get(normalizedCaseId);
  if (inflight) {
    return inflight;
  }

  return requestSharedCaseOverview(normalizedCaseId, store);
}

export function preloadSharedCaseOverview(
  caseId: string,
  options?: { force?: boolean; keepStale?: boolean }
): Promise<StatsV2CaseOverviewDTO> {
  return querySharedCaseOverview(caseId, options);
}

export async function preloadCaseOverviewForWorkspace(
  workspaceRoot: string
): Promise<StatsV2CaseOverviewDTO | null> {
  const normalizedWorkspaceRoot = normalizeWorkspaceRoot(workspaceRoot);
  if (!normalizedWorkspaceRoot || typeof window === "undefined") {
    return null;
  }
  const bridge = window.analytix?.dataAnalysis;
  if (
    typeof bridge?.ensureBackend !== "function" ||
    typeof bridge.ensureWorkspaceCase !== "function"
  ) {
    return null;
  }
  const ready = await ensureDesktopBackendRuntime();
  if (!ready) {
    return null;
  }
  const result = await bridge.ensureWorkspaceCase({ workspaceRoot: normalizedWorkspaceRoot });
  if (!result.ok) {
    throw new Error(result.message || "ensure workspace case failed");
  }
  setSharedActiveCaseContext({ caseId: result.caseId, workspaceRoot: normalizedWorkspaceRoot });
  return preloadSharedCaseOverview(result.caseId);
}
