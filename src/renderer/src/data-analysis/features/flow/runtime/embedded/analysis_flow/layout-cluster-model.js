/* Layout cluster model.
 * Responsibilities: derive frontend cluster signatures and slot-size cache rows.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  const DEFAULT_SLOT_CACHE_MAX = 2400;

  function sortStableIds(ids = []) {
    return (ids || [])
      .map((id) => String(id || "").trim())
      .filter(Boolean)
      .sort((a, b) => String(a).localeCompare(String(b), "zh-CN"));
  }

  function clusterSignature(cluster) {
    return sortStableIds(cluster?.nodeIds || []).join("|");
  }

  function normalizeLayoutSlotMode(mode) {
    const m = String(mode || "").trim().toLowerCase();
    if (m === "relation") return "compact";
    return m === "network" ? "network" : "compact";
  }

  function makeClusterSlotCacheKey(mode, signature) {
    const sig = String(signature || "").trim();
    if (!sig) return "";
    return `${normalizeLayoutSlotMode(mode)}|${sig}`;
  }

  function normalizeClusterSlotValue(slot, mode) {
    const kind = normalizeLayoutSlotMode(mode);
    const minW = kind === "network" ? 240 : 210;
    const minH = kind === "network" ? 210 : 190;
    const width = Number(slot?.width);
    const height = Number(slot?.height);
    if (!Number.isFinite(width) || !Number.isFinite(height)) return null;
    if (width <= 0 || height <= 0) return null;
    return {
      width: Math.max(minW, Math.round(width)),
      height: Math.max(minH, Math.round(height)),
    };
  }

  function slotFromClusterBBox(mode, bbox) {
    const b = bbox && typeof bbox === "object" ? bbox : null;
    if (!b) return null;
    const w = Number(b?.width);
    const h = Number(b?.height);
    if (!Number.isFinite(w) || !Number.isFinite(h) || w <= 0 || h <= 0) return null;
    const kind = normalizeLayoutSlotMode(mode);
    if (kind === "network") {
      return normalizeClusterSlotValue({ width: w + 86, height: h + 74 }, "network");
    }
    return normalizeClusterSlotValue({ width: w + 78, height: h + 64 }, "compact");
  }

  function readClusterSlotFromCache(cache, signature, mode) {
    const rows = cache && typeof cache === "object" ? cache : {};
    const key = makeClusterSlotCacheKey(mode, signature);
    if (!key) return null;
    return normalizeClusterSlotValue(rows[key], mode);
  }

  function rememberClusterSlot(cache, options = {}) {
    const rows = cache && typeof cache === "object" ? cache : {};
    const normalized = normalizeClusterSlotValue(options?.slot, options?.mode);
    if (!normalized) return { remembered: false, prunedKeys: [] };
    const key = makeClusterSlotCacheKey(options?.mode, options?.signature);
    if (!key) return { remembered: false, prunedKeys: [] };
    rows[key] = {
      width: normalized.width,
      height: normalized.height,
      updatedAt: Number(options?.now) || Date.now(),
    };
    const maxEntries = Math.max(1, Number(options?.maxEntries) || DEFAULT_SLOT_CACHE_MAX);
    const keys = Object.keys(rows);
    if (keys.length <= maxEntries) return { remembered: true, prunedKeys: [] };
    const stale = keys
      .map((k) => ({ key: k, t: Number(rows?.[k]?.updatedAt) || 0 }))
      .sort((a, b) => a.t - b.t)
      .slice(0, Math.max(1, keys.length - maxEntries));
    stale.forEach((row) => {
      delete rows[row.key];
    });
    return { remembered: true, prunedKeys: stale.map((row) => row.key) };
  }

  function buildClusterSignatureByIdFromLayoutHints(layoutHints) {
    const out = {};
    const semantic =
      layoutHints?.semantic && typeof layoutHints.semantic === "object" ? layoutHints.semantic : null;
    const rows = Array.isArray(semantic?.clusters) ? semantic.clusters : [];
    rows.forEach((cluster) => {
      const clusterId = String(cluster?.clusterId || "").trim();
      if (!clusterId) return;
      out[clusterId] = clusterSignature(cluster);
    });
    return out;
  }

  function updateClusterSlotCacheFromLayoutReport(cache, options = {}) {
    const rows = cache && typeof cache === "object" ? cache : {};
    const layoutReport = options?.layoutReport && typeof options.layoutReport === "object" ? options.layoutReport : null;
    if (!layoutReport) return { updated: 0, prunedKeys: [] };
    const signatureByClusterId = buildClusterSignatureByIdFromLayoutHints(options?.layoutHints || null);
    const maxEntries = Math.max(1, Number(options?.maxEntries) || DEFAULT_SLOT_CACHE_MAX);
    const now = Number(options?.now) || Date.now();
    const pruned = [];
    let updated = 0;
    const applyRows = (mode, clusters) => {
      const list = Array.isArray(clusters) ? clusters : [];
      if (!list.length) return;
      list.forEach((row) => {
        const clusterId = String(row?.clusterId || "").trim();
        if (!clusterId) return;
        const signature = String(signatureByClusterId?.[clusterId] || "").trim();
        if (!signature) return;
        const slot = slotFromClusterBBox(mode, row?.bbox);
        if (!slot) return;
        const result = rememberClusterSlot(rows, {
          signature,
          mode,
          slot,
          maxEntries,
          now,
        });
        if (result.remembered) updated += 1;
        pruned.push(...result.prunedKeys);
      });
    };
    if (layoutReport?.network && typeof layoutReport.network === "object") {
      applyRows("network", layoutReport.network?.clusters);
    }
    if (layoutReport?.compact && typeof layoutReport.compact === "object") {
      applyRows("compact", layoutReport.compact?.clusters);
      applyRows("relation", layoutReport.compact?.clusters);
    }
    return { updated, prunedKeys: pruned };
  }

  root.__ANALYTIX_FLOW_LAYOUT_CLUSTER_MODEL__ = {
    sortStableIds,
    clusterSignature,
    normalizeLayoutSlotMode,
    makeClusterSlotCacheKey,
    normalizeClusterSlotValue,
    slotFromClusterBBox,
    readClusterSlotFromCache,
    rememberClusterSlot,
    buildClusterSignatureByIdFromLayoutHints,
    updateClusterSlotCacheFromLayoutReport,
  };
})();
