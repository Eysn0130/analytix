/* Layout plan cache model.
 * Responsibilities: manage cached Rust-projected layout node plans.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  const DEFAULT_CACHE_MAX = 18;
  const applyModel = root.__ANALYTIX_FLOW_LAYOUT_PLAN_CACHE_APPLY_MODEL__;
  if (!applyModel || typeof applyModel !== "object") {
    throw new Error("layout-plan-cache-apply-model-missing");
  }

  function requireFunction(fn, message) {
    if (typeof fn !== "function") {
      throw new Error(message);
    }
    return fn;
  }

  const normalizeLayoutNodeUpdates = requireFunction(
    applyModel?.normalizeLayoutNodeUpdates,
    "layout-plan-cache-updates-missing"
  );

  function normalizeNodeIdOrder(value) {
    const rows = Array.isArray(value) ? value : [];
    return rows.map((id) => String(id || "").trim()).filter(Boolean);
  }

  function readLayoutPlanCache(cache, cacheKey, options = {}) {
    const key = String(cacheKey || "").trim();
    if (!key) return null;
    const rows = cache && typeof cache === "object" ? cache : {};
    const row = rows[key];
    if (
      !row ||
      typeof row !== "object" ||
      !Array.isArray(row.updates)
    ) {
      return null;
    }
    row.updatedAt = Number(options?.now) || Date.now();
    return row;
  }

  function rememberLayoutPlanCache(cache, options = {}) {
    const key = String(options?.cacheKey || "").trim();
    if (!key) return { remembered: false, prunedKeys: [] };
    const nodeCount = Math.max(0, Number(options?.nodeCount) || 0);
    if (!nodeCount) return { remembered: false, prunedKeys: [] };
    const normalizeLayoutPreset = requireFunction(
      options?.normalizeLayoutPreset,
      "layout-plan-cache-normalize-preset-missing"
    );
    const rows = cache && typeof cache === "object" ? cache : {};
    const updates = normalizeLayoutNodeUpdates(options?.updates);
    if (!Array.isArray(updates) || updates.length !== nodeCount) {
      return { remembered: false, prunedKeys: [] };
    }
    const nodeIds = normalizeNodeIdOrder(options?.nodeIds);
    if (!nodeIds.length || nodeIds.length !== nodeCount) {
      return { remembered: false, prunedKeys: [] };
    }
    for (let i = 0; i < updates.length; i += 1) {
      if (String(updates[i]?.id || "").trim() !== nodeIds[i]) {
        return { remembered: false, prunedKeys: [] };
      }
    }
    rows[key] = {
      key,
      mode: normalizeLayoutPreset(options?.mode),
      nodeCount,
      updates,
      updatedAt: Number(options?.now) || Date.now(),
    };
    const maxEntries = Math.max(1, Number(options?.maxEntries) || DEFAULT_CACHE_MAX);
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

  root.__ANALYTIX_FLOW_LAYOUT_PLAN_CACHE_MODEL__ = {
    readLayoutPlanCache,
    rememberLayoutPlanCache,
  };
})();
