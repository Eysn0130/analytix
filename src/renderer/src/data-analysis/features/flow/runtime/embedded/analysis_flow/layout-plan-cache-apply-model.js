/* Layout plan cache apply model.
 * Responsibilities: validate and apply cached layout node plans onto live graph nodes.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;
  const updateModel = root.__ANALYTIX_FLOW_LAYOUT_NODE_UPDATE_MODEL__;

  function requireFunction(fn, message) {
    if (typeof fn !== "function") {
      throw new Error(message);
    }
    return fn;
  }

  const normalizeLayoutNodeUpdates = requireFunction(
    updateModel?.normalizeLayoutNodeUpdates,
    "layout-node-update-model-missing"
  );
  const applyLayoutNodeUpdates = requireFunction(
    updateModel?.applyLayoutNodeUpdates,
    "layout-node-update-model-missing"
  );

  function applyLayoutPlanCacheEntry(cacheEntry, nodes, options = {}) {
    const entry = cacheEntry && typeof cacheEntry === "object" ? cacheEntry : null;
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    if (!entry || !nodeRows.length) return false;
    const updates = Array.isArray(entry.updates) ? entry.updates : null;
    if (!updates) return false;
    if ((Number(entry?.nodeCount) || 0) !== nodeRows.length) return false;
    return applyLayoutNodeUpdates(nodeRows, updates, options?.metaKeys);
  }

  root.__ANALYTIX_FLOW_LAYOUT_PLAN_CACHE_APPLY_MODEL__ = {
    normalizeLayoutNodeUpdates,
    applyLayoutNodeUpdates,
    applyLayoutPlanCacheEntry,
  };
})();
