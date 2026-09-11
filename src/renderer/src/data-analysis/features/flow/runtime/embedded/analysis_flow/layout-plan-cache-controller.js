/* Layout plan cache controller.
 * Responsibilities: orchestrate cache key/read/write/apply around the Rust-backed node plan contract.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function requireFunction(fn, message) {
    if (typeof fn !== "function") {
      throw new Error(message);
    }
    return fn;
  }

  function createLayoutPlanCacheController(deps = {}) {
    const state = deps?.state && typeof deps.state === "object" ? deps.state : null;
    if (!state) throw new Error("layout-plan-cache-controller-state-missing");
    const planCacheModel =
      deps?.planCacheModel && typeof deps.planCacheModel === "object" ? deps.planCacheModel : null;
    if (!planCacheModel) throw new Error("layout-plan-cache-controller-model-missing");
    const applyModel =
      deps?.applyModel && typeof deps.applyModel === "object" ? deps.applyModel : null;
    if (!applyModel) throw new Error("layout-plan-cache-controller-apply-model-missing");
    const runtimeAdapter =
      deps?.runtimeAdapter && typeof deps.runtimeAdapter === "object" ? deps.runtimeAdapter : null;
    if (!runtimeAdapter) throw new Error("layout-plan-cache-controller-runtime-missing");
    const ensureSemanticState = requireFunction(
      deps?.ensureSemanticState,
      "layout-plan-cache-controller-semantic-state-missing"
    );
    const computeLayoutNodePlan = requireFunction(
      deps?.computeLayoutNodePlan,
      "layout-plan-cache-controller-node-plan-missing"
    );
    const normalizeLayoutPreset = requireFunction(
      deps?.normalizeLayoutPreset,
      "layout-plan-cache-controller-normalize-preset-missing"
    );
    const getLayoutMetaKeys = requireFunction(
      runtimeAdapter?.getLayoutMetaKeys,
      "layout-plan-cache-controller-meta-keys-missing"
    );
    const readLayoutPlanCache = requireFunction(
      planCacheModel?.readLayoutPlanCache,
      "layout-plan-cache-controller-read-missing"
    );
    const rememberLayoutPlanCache = requireFunction(
      planCacheModel?.rememberLayoutPlanCache,
      "layout-plan-cache-controller-remember-missing"
    );
    const applyLayoutPlanCacheEntry = requireFunction(
      applyModel?.applyLayoutPlanCacheEntry,
      "layout-plan-cache-controller-apply-missing"
    );
    const projectLayoutCacheKey = requireFunction(
      deps?.projectLayoutCacheKey,
      "layout-plan-cache-controller-rust-key-missing"
    );
    const algoVersion = String(deps?.algoVersion || "");
    const maxEntries = Math.max(1, Number(deps?.maxEntries) || 18);

    function ensureCacheStats() {
      const semanticState = ensureSemanticState();
      if (!semanticState.layoutPlanCacheStats || typeof semanticState.layoutPlanCacheStats !== "object") {
        semanticState.layoutPlanCacheStats = {};
      }
      return semanticState.layoutPlanCacheStats;
    }

    function incrementStat(key) {
      const stats = ensureCacheStats();
      stats[key] = Math.max(0, Number(stats[key] || 0)) + 1;
      return stats[key];
    }

    function buildSeedContext() {
      return {
        selectedIds: Array.from(state.selected || []),
        leftSeeds: Array.isArray(state.lastRequest?.leftSeeds) ? state.lastRequest.leftSeeds : [],
        focusKeyType: state.focusKeyType || "",
        tab: state.tab || "",
        source: state.source || "",
        focusUnknownName: !!state.focusUnknownName,
        focusCounterpartyStrict: !!state.focusCounterpartyStrict,
      };
    }

    function buildCacheProjectionInput(mode, focusId, nodes, edges) {
      const nodeRows = Array.isArray(nodes) ? nodes : [];
      const edgeRows = Array.isArray(edges) ? edges : [];
      const layoutDirection =
        state.layoutDirection && typeof state.layoutDirection === "object"
          ? state.layoutDirection
          : {};
      const seedContext = buildSeedContext();
      return {
        nodes: nodeRows,
        edges: edgeRows,
        algoVersion,
        mode: normalizeLayoutPreset(mode),
        focusId: String(focusId || "").trim(),
        graphMutationSeq: Number(state.graphMutationSeq || 0),
        layoutDirection,
        seedContext,
      };
    }

    function buildCacheProjectionIdentity(input) {
      return JSON.stringify({
        algoVersion: input.algoVersion,
        mode: input.mode,
        focusId: input.focusId,
        graphMutationSeq: input.graphMutationSeq,
        layoutDirection: input.layoutDirection,
        seedContext: input.seedContext,
      });
    }

    function isProjectedCacheKeyCurrent(projected, input, identity) {
      return !!(
        projected &&
        typeof projected === "object" &&
        projected.nodesRef === input.nodes &&
        projected.edgesRef === input.edges &&
        Number(projected.nodesLen || 0) === input.nodes.length &&
        Number(projected.edgesLen || 0) === input.edges.length &&
        projected.identity === identity &&
        typeof projected.key === "string" &&
        projected.key
      );
    }

    async function prepareCacheKey(mode, focusId, nodes, edges) {
      const semanticState = ensureSemanticState();
      const input = buildCacheProjectionInput(mode, focusId, nodes, edges);
      const identity = buildCacheProjectionIdentity(input);
      const current = semanticState.layoutCacheKeyProjection;
      if (isProjectedCacheKeyCurrent(current, input, identity)) {
        incrementStat("rustKeyProjectionReused");
        return current.key;
      }
      try {
        const cacheKey = await projectLayoutCacheKey(input);
        const key = String(cacheKey?.key || "").trim();
        if (!key) {
          incrementStat("rustKeyProjectionFailed");
          return "";
        }
        semanticState.layoutCacheKeyProjection = {
          key,
          cacheKey,
          identity,
          nodesRef: input.nodes,
          edgesRef: input.edges,
          nodesLen: input.nodes.length,
          edgesLen: input.edges.length,
        };
        incrementStat("rustKeyProjectionPrepared");
        return key;
      } catch (error) {
        incrementStat("rustKeyProjectionFailed");
        return "";
      }
    }

    function makeCacheKey(mode, focusId, nodes, edges) {
      const semanticState = ensureSemanticState();
      const input = buildCacheProjectionInput(mode, focusId, nodes, edges);
      const identity = buildCacheProjectionIdentity(input);
      const projected = semanticState.layoutCacheKeyProjection;
      if (isProjectedCacheKeyCurrent(projected, input, identity)) {
        incrementStat("rustKeySyncHit");
        return projected.key;
      }
      incrementStat("rustKeySyncMiss");
      return "";
    }

    function readCache(cacheKey) {
      const semanticState = ensureSemanticState();
      const entry = readLayoutPlanCache(semanticState.layoutPlanCache || {}, cacheKey);
      if (String(cacheKey || "").trim()) {
        incrementStat(entry ? "planCacheHit" : "planCacheMiss");
      }
      return entry;
    }

    async function rememberCache(cacheKey, mode, nodes) {
      const key = String(cacheKey || "").trim();
      const nodeRows = Array.isArray(nodes) ? nodes : [];
      if (!key || !nodeRows.length) return false;
      const semanticState = ensureSemanticState();
      const cache = semanticState.layoutPlanCache || {};
      semanticState.layoutPlanCache = cache;
      try {
        const result = await computeLayoutNodePlan({
          operation: "project",
          nodes: nodeRows,
          metaKeys: getLayoutMetaKeys(),
        });
        const updates = Array.isArray(result?.updates) ? result.updates : null;
        if (!updates || semanticState.layoutPlanCache !== cache) return false;
        const remembered = rememberLayoutPlanCache(cache, {
          cacheKey: key,
          mode,
          nodeCount: nodeRows.length,
          nodeIds: nodeRows.map((node) => node?.id),
          updates,
          normalizeLayoutPreset,
          maxEntries,
        });
        if (remembered?.remembered) {
          incrementStat("planCacheRemembered");
        }
        return !!remembered?.remembered;
      } catch (e) {
        return false;
      }
    }

    function applyCacheEntry(cacheEntry, nodes) {
      return applyLayoutPlanCacheEntry(cacheEntry, nodes, {
        metaKeys: getLayoutMetaKeys(),
      });
    }

    function clearCaches() {
      const semanticState = ensureSemanticState();
      semanticState.layoutPlanCache = {};
      semanticState.layoutCacheKeyProjection = null;
      semanticState.layoutPlanCacheStats = {};
    }

    return {
      prepareCacheKey,
      makeCacheKey,
      readCache,
      rememberCache,
      applyCacheEntry,
      clearCaches,
    };
  }

  root.__ANALYTIX_FLOW_LAYOUT_PLAN_CACHE_CONTROLLER__ = {
    createLayoutPlanCacheController,
  };
})();
