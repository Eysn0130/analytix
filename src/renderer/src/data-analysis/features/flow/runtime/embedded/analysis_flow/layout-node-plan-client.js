/* Layout node plan client.
 * Responsibilities: call the Rust-backed layout node plan contract through the embedded backend.
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

  function createLayoutNodePlanClient(deps = {}) {
    const backendCall = requireFunction(
      deps?.backendCall,
      "layout-node-plan-client-backend-call-missing"
    );
    const parseJSONSafe = requireFunction(
      deps?.parseJSONSafe,
      "layout-node-plan-client-json-missing"
    );
    const getCaseId =
      typeof deps?.getCaseId === "function"
        ? deps.getCaseId
        : () => String(deps?.caseId || "");

    async function computeLayoutNodePlan(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const raw = await backendCall(
        "computeLayoutNodePlan",
        JSON.stringify({
          ...request,
          caseId: String(getCaseId() || ""),
          operation: String(request.operation || "").trim(),
        })
      );
      const result = parseJSONSafe(raw, {});
      return result?.ok ? result : null;
    }

    async function projectLayoutCacheKey(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const result = await computeLayoutNodePlan({
        ...request,
        operation: "cache_key",
      });
      return result?.cacheKey && typeof result.cacheKey === "object" ? result.cacheKey : null;
    }

    async function computeNetworkLayoutPlan(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const result = await computeLayoutNodePlan({
        ...request,
        operation: "network_plan",
        mode: "network",
      });
      return result?.networkPlan && typeof result.networkPlan === "object" ? result.networkPlan : null;
    }

    async function computeNetworkCommunityQuality(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const result = await computeLayoutNodePlan({
        ...request,
        operation: "network_community_quality",
        mode: "network",
      });
      return result && typeof result === "object" ? result : null;
    }

    return {
      computeLayoutNodePlan,
      projectLayoutCacheKey,
      computeNetworkLayoutPlan,
      computeNetworkCommunityQuality,
    };
  }

  root.__ANALYTIX_FLOW_LAYOUT_NODE_PLAN_CLIENT__ = {
    createLayoutNodePlanClient,
  };
})();
