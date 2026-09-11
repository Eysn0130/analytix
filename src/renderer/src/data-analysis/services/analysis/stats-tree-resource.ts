import {
  getStatsTreeGroupSecondaryText,
  getStatsTreeItemPrimaryText,
  getStatsTreeItemSecondaryText,
  isStatsTreeAccountLikeGroup,
  normalizeStatsTreeGroups,
} from "./stats-tree-display";
import { queryStatsV2Tree, type StatsTreeGroupDTO, type StatsTreeItemDTO, type StatsTreeTab } from "./stats-shared";

const STATS_TREE_CACHE_LIMIT = 12;
const STATS_TREE_CACHE_TTL_MS = 15_000;
const STATS_TREE_PERSIST_PREFIX = "analytix:shared-stats-tree:v4:";

export interface StatsPreparedTreeItem extends StatsTreeItemDTO {
  primaryText: string;
  secondaryText: string;
  searchText: string;
}

export interface StatsPreparedTreeGroup extends StatsTreeGroupDTO {
  secondaryText: string;
  identityText: string;
  searchText: string;
  isAccountLikeGroup: boolean;
  itemKeys: string[];
  items: StatsPreparedTreeItem[];
}

type SharedStatsTreeStore = {
  cache: Map<string, { expiresAt: number; value: StatsPreparedTreeGroup[] }>;
  inflight: Map<string, Promise<StatsPreparedTreeGroup[]>>;
};

declare global {
  interface Window {
    __ANALYTIX_SHARED_STATS_TREE_STORE__?: SharedStatsTreeStore;
  }
}

const fallbackStore: SharedStatsTreeStore = {
  cache: new Map<string, { expiresAt: number; value: StatsPreparedTreeGroup[] }>(),
  inflight: new Map<string, Promise<StatsPreparedTreeGroup[]>>(),
};

function getSharedStatsTreeStore(): SharedStatsTreeStore {
  if (typeof window === "undefined") {
    return fallbackStore;
  }
  if (window.__ANALYTIX_SHARED_STATS_TREE_STORE__) {
    return window.__ANALYTIX_SHARED_STATS_TREE_STORE__;
  }
  window.__ANALYTIX_SHARED_STATS_TREE_STORE__ = {
    cache: new Map<string, { expiresAt: number; value: StatsPreparedTreeGroup[] }>(),
    inflight: new Map<string, Promise<StatsPreparedTreeGroup[]>>(),
  };
  return window.__ANALYTIX_SHARED_STATS_TREE_STORE__;
}

function trimCache(store: SharedStatsTreeStore, now = Date.now()): void {
  Array.from(store.cache.entries()).forEach(([key, entry]) => {
    if (entry.expiresAt <= now) {
      store.cache.delete(key);
    }
  });
  while (store.cache.size > STATS_TREE_CACHE_LIMIT) {
    const oldestKey = store.cache.keys().next().value;
    if (!oldestKey) {
      break;
    }
    store.cache.delete(oldestKey);
  }
}

function uniqueItemKeys(items: Array<Pick<StatsTreeItemDTO, "id">>): string[] {
  return Array.from(new Set(items.map((item) => String(item.id || "").trim()).filter(Boolean)));
}

function normalizeSearchText(parts: Array<string | undefined>): string {
  return parts
    .map((part) => String(part || "").trim().toLowerCase())
    .filter(Boolean)
    .join(" ");
}

function getCacheKey(caseId: string, tab: StatsTreeTab): string {
  return `${String(caseId || "").trim()}:${String(tab || "byName").trim() || "byName"}`;
}

function getPersistStorageKey(caseId: string, tab: StatsTreeTab): string {
  return `${STATS_TREE_PERSIST_PREFIX}${getCacheKey(caseId, tab)}`;
}

function readPersistedGroups(caseId: string, tab: StatsTreeTab): StatsPreparedTreeGroup[] | null {
  if (typeof window === "undefined") {
    return null;
  }
  window.localStorage.removeItem(getPersistStorageKey(caseId, tab));
  try {
    Object.keys(window.localStorage).forEach((key) => {
      if (key.startsWith("analytix:shared-stats-tree:")) {
        window.localStorage.removeItem(key);
      }
    });
  } catch {
    // Ignore storage cleanup failures; the network-backed cache remains authoritative.
  }
  return null;
}

function writePersistedGroups(caseId: string, tab: StatsTreeTab, value: StatsPreparedTreeGroup[]): void {
  void caseId;
  void tab;
  void value;
}

function prepareStatsTreeGroups(tab: StatsTreeTab, groups: StatsTreeGroupDTO[]): StatsPreparedTreeGroup[] {
  return normalizeStatsTreeGroups(tab, Array.isArray(groups) ? groups : []).map((group) => {
    const items = (Array.isArray(group.items) ? group.items : []).map((item) => {
      const primaryText = getStatsTreeItemPrimaryText(item, tab);
      const secondaryText = getStatsTreeItemSecondaryText(item);
      return {
        ...item,
        primaryText,
        secondaryText,
        searchText: normalizeSearchText([item.id, item.title, item.sub, primaryText, secondaryText]),
      };
    });
    const secondaryText = getStatsTreeGroupSecondaryText(tab, group);
    return {
      ...group,
      secondaryText,
      identityText: secondaryText,
      searchText: normalizeSearchText([group.title, group.meta, group.extra, secondaryText]),
      isAccountLikeGroup: isStatsTreeAccountLikeGroup(tab, group),
      itemKeys: uniqueItemKeys(items),
      items,
    };
  });
}

export function invalidateSharedStatsTree(caseId = ""): void {
  const store = getSharedStatsTreeStore();
  const normalizedCaseId = String(caseId || "").trim();
  Array.from(store.cache.keys()).forEach((key) => {
    if (!normalizedCaseId || key.startsWith(`${normalizedCaseId}:`)) {
      store.cache.delete(key);
    }
  });
  Array.from(store.inflight.keys()).forEach((key) => {
    if (!normalizedCaseId || key.startsWith(`${normalizedCaseId}:`)) {
      store.inflight.delete(key);
    }
  });
  if (typeof window !== "undefined") {
    Object.keys(window.localStorage).forEach((key) => {
      if (!key.startsWith(STATS_TREE_PERSIST_PREFIX)) {
        return;
      }
      if (!normalizedCaseId || key.startsWith(`${STATS_TREE_PERSIST_PREFIX}${normalizedCaseId}:`)) {
        window.localStorage.removeItem(key);
      }
    });
  }
}

export function peekSharedStatsTree(payload: {
  case_id: string;
  tab: StatsTreeTab;
}): StatsPreparedTreeGroup[] {
  const caseId = String(payload.case_id || "").trim();
  const tab = payload.tab;
  if (!caseId) {
    return [];
  }
  const store = getSharedStatsTreeStore();
  const cacheKey = getCacheKey(caseId, tab);
  const now = Date.now();
  trimCache(store, now);
  const cached = store.cache.get(cacheKey);
  if (cached && cached.expiresAt > now) {
    return cached.value;
  }
  const persisted = readPersistedGroups(caseId, tab);
  if (persisted?.length) {
    store.cache.set(cacheKey, {
      expiresAt: now + STATS_TREE_CACHE_TTL_MS,
      value: persisted,
    });
    trimCache(store, now);
    return persisted;
  }
  return [];
}

export async function querySharedStatsTree(payload: {
  case_id: string;
  tab: StatsTreeTab;
  force?: boolean;
}): Promise<StatsPreparedTreeGroup[]> {
  const caseId = String(payload.case_id || "").trim();
  const tab = payload.tab;
  const force = Boolean(payload.force);
  if (!caseId) {
    return [];
  }
  const store = getSharedStatsTreeStore();
  const cacheKey = getCacheKey(caseId, tab);
  const now = Date.now();
  trimCache(store, now);
  if (force) {
    store.cache.delete(cacheKey);
  } else {
    const cached = store.cache.get(cacheKey);
    if (cached && cached.expiresAt > now) {
      return cached.value;
    }
    const persisted = readPersistedGroups(caseId, tab);
    if (persisted && persisted.length > 0) {
      store.cache.set(cacheKey, {
        expiresAt: now + STATS_TREE_CACHE_TTL_MS,
        value: persisted,
      });
      trimCache(store, now);
      return persisted;
    }
    const inflight = store.inflight.get(cacheKey);
    if (inflight) {
      return inflight;
    }
  }
  const request = queryStatsV2Tree()
    .then((_result): StatsPreparedTreeGroup[] => {
      throw new Error("host_evidence_receipt_required");
    })
    .then((groups) => {
      if (store.inflight.get(cacheKey) !== request) {
        return groups;
      }
      store.cache.set(cacheKey, {
        expiresAt: Date.now() + STATS_TREE_CACHE_TTL_MS,
        value: groups,
      });
      writePersistedGroups(caseId, tab, groups);
      trimCache(store);
      return groups;
    })
    .finally(() => {
      if (store.inflight.get(cacheKey) === request) {
        store.inflight.delete(cacheKey);
      }
    });
  store.inflight.set(cacheKey, request);
  return request;
}

export function filterPreparedStatsTreeGroups(
  groups: StatsPreparedTreeGroup[],
  keyword: string
): StatsPreparedTreeGroup[] {
  const normalizedKeyword = String(keyword || "").trim().toLowerCase();
  if (!normalizedKeyword) {
    return groups;
  }
  return groups
    .map((group) => {
      if (group.searchText.includes(normalizedKeyword)) {
        return group;
      }
      const items = group.items.filter((item) => item.searchText.includes(normalizedKeyword));
      if (!items.length) {
        return null;
      }
      return {
        ...group,
        items,
        itemKeys: uniqueItemKeys(items),
      };
    })
    .filter((group): group is StatsPreparedTreeGroup => Boolean(group));
}

export function collectPreparedStatsTreeAccountKeys(groups: StatsPreparedTreeGroup[]): string[] {
  return Array.from(new Set(groups.flatMap((group) => group.itemKeys)));
}
