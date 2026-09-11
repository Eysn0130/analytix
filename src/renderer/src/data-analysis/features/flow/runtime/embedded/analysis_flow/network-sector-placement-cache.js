/* Network sector placement cache.
 * Responsibilities: cache plain Rust-shaped sectorPlacement DTOs by canonical payload key.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  const CACHE_VERSION = "network-sector-placement-v2";
  const DEFAULT_MAX_ENTRIES = 24;

  function nowMs(nowFn) {
    const value = typeof nowFn === "function" ? Number(nowFn()) : Date.now();
    return Number.isFinite(value) ? value : Date.now();
  }

  function normalizeKeyValue(value) {
    if (Array.isArray(value)) return value.map((item) => normalizeKeyValue(item));
    if (!value || typeof value !== "object") {
      if (typeof value === "number") return Number.isFinite(value) ? value : null;
      if (typeof value === "string" || typeof value === "boolean" || value == null) return value;
      return null;
    }
    const out = {};
    Object.keys(value)
      .sort()
      .forEach((key) => {
        const item = value[key];
        if (typeof item === "undefined" || typeof item === "function") return;
        out[key] = normalizeKeyValue(item);
      });
    return out;
  }

  function cloneJson(value) {
    if (!value || typeof value !== "object") return null;
    try {
      return JSON.parse(JSON.stringify(value));
    } catch (error) {
      return null;
    }
  }

  function makeNetworkSectorPlacementCacheKey(payload = {}) {
    const source = payload && typeof payload === "object" ? payload : {};
    const normalized = normalizeKeyValue(source);
    try {
      return `${CACHE_VERSION}:${JSON.stringify(normalized)}`;
    } catch (error) {
      return "";
    }
  }

  function createStats() {
    return {
      readHits: 0,
      readMisses: 0,
      layoutReadHits: 0,
      layoutReadMisses: 0,
      prefetchReadHits: 0,
      prefetchReadMisses: 0,
      remembered: 0,
      pruned: 0,
      pendingAccepted: 0,
      pendingDeduped: 0,
      pendingCleared: 0,
      prefetchRequested: 0,
      prefetchQueued: 0,
      prefetchCached: 0,
      prefetchPending: 0,
      prefetchStored: 0,
      prefetchFailed: 0,
      prefetchInvalid: 0,
    };
  }

  function normalizeReadSource(options = {}) {
    const source = String(options?.source || "").trim().toLowerCase();
    return source === "layout" || source === "prefetch" ? source : "";
  }

  function rate(numerator, denominator) {
    const n = Math.max(0, Number(numerator) || 0);
    const d = Math.max(0, Number(denominator) || 0);
    return d > 0 ? Number((n / d).toFixed(4)) : 0;
  }

  function createNetworkSectorPlacementCache(options = {}) {
    const rows = new Map();
    const pending = new Set();
    const stats = createStats();
    const maxEntries = Math.max(1, Number(options?.maxEntries) || DEFAULT_MAX_ENTRIES);
    const nowFn = typeof options?.nowFn === "function" ? options.nowFn : null;

    function recordRead(source, hit) {
      if (hit) stats.readHits += 1;
      else stats.readMisses += 1;
      if (source === "layout") {
        if (hit) stats.layoutReadHits += 1;
        else stats.layoutReadMisses += 1;
      } else if (source === "prefetch") {
        if (hit) stats.prefetchReadHits += 1;
        else stats.prefetchReadMisses += 1;
      }
    }

    function recordPrefetchEvent(event, count = 1) {
      const amount = Math.max(1, Number(count) || 1);
      const name = String(event || "").trim();
      const keyByEvent = {
        requested: "prefetchRequested",
        queued: "prefetchQueued",
        cached: "prefetchCached",
        pending: "prefetchPending",
        stored: "prefetchStored",
        failed: "prefetchFailed",
        invalid: "prefetchInvalid",
      };
      const key = keyByEvent[name] || "";
      if (!key) return false;
      stats[key] += amount;
      return true;
    }

    function prune() {
      if (rows.size <= maxEntries) return [];
      const stale = Array.from(rows.values())
        .sort((a, b) => (Number(a.updatedAt) || 0) - (Number(b.updatedAt) || 0))
        .slice(0, Math.max(1, rows.size - maxEntries));
      stale.forEach((entry) => rows.delete(entry.key));
      return stale.map((entry) => entry.key);
    }

    function read(cacheKey, options = {}) {
      const key = String(cacheKey || "").trim();
      const source = normalizeReadSource(options);
      if (!key) return null;
      const entry = rows.get(key);
      if (!entry || !entry.sectorPlacement || typeof entry.sectorPlacement !== "object") {
        recordRead(source, false);
        return null;
      }
      entry.updatedAt = nowMs(nowFn);
      const cloned = cloneJson(entry.sectorPlacement);
      recordRead(source, !!cloned);
      return cloned;
    }

    function remember(cacheKey, sectorPlacement) {
      const key = String(cacheKey || "").trim();
      const placement = cloneJson(sectorPlacement);
      if (!key || !placement) return { remembered: false, prunedKeys: [] };
      rows.set(key, {
        key,
        sectorPlacement: placement,
        updatedAt: nowMs(nowFn),
      });
      stats.remembered += 1;
      const prunedKeys = prune();
      stats.pruned += prunedKeys.length;
      return { remembered: true, prunedKeys };
    }

    function markPending(cacheKey) {
      const key = String(cacheKey || "").trim();
      if (!key) return false;
      if (pending.has(key)) {
        stats.pendingDeduped += 1;
        return false;
      }
      pending.add(key);
      stats.pendingAccepted += 1;
      return true;
    }

    function clearPending(cacheKey) {
      const key = String(cacheKey || "").trim();
      if (!key) return false;
      const cleared = pending.delete(key);
      if (cleared) stats.pendingCleared += 1;
      return cleared;
    }

    function isPending(cacheKey) {
      const key = String(cacheKey || "").trim();
      return !!key && pending.has(key);
    }

    function clear() {
      rows.clear();
      pending.clear();
      Object.assign(stats, createStats());
    }

    function snapshot() {
      return {
        size: rows.size,
        pending: pending.size,
        keys: Array.from(rows.keys()),
        stats: {
          ...stats,
          hitRate: rate(stats.readHits, stats.readHits + stats.readMisses),
          layoutHitRate: rate(stats.layoutReadHits, stats.layoutReadHits + stats.layoutReadMisses),
        },
      };
    }

    return {
      makeKey: makeNetworkSectorPlacementCacheKey,
      read,
      remember,
      markPending,
      clearPending,
      isPending,
      recordPrefetchEvent,
      clear,
      snapshot,
    };
  }

  root.__ANALYTIX_FLOW_NETWORK_SECTOR_PLACEMENT_CACHE__ = {
    CACHE_VERSION,
    makeNetworkSectorPlacementCacheKey,
    createNetworkSectorPlacementCache,
  };
})();
