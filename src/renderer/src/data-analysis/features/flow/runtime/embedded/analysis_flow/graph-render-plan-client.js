/* Graph render plan client.
 * Responsibilities: call the Rust-backed graph render plan contract through the embedded backend.
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

  function createGraphRenderPlanClient(deps = {}) {
    const backendCall = requireFunction(
      deps?.backendCall,
      "graph-render-plan-client-backend-call-missing"
    );
    const parseJSONSafe = requireFunction(
      deps?.parseJSONSafe,
      "graph-render-plan-client-json-missing"
    );
    const getCaseId =
      typeof deps?.getCaseId === "function"
        ? deps.getCaseId
        : () => String(deps?.caseId || "");

    async function projectGraphRenderPlan(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const raw = await backendCall(
        "projectGraphRenderPlan",
        JSON.stringify({
          ...request,
          caseId: String(getCaseId() || ""),
        })
      );
      const result = parseJSONSafe(raw, {});
      return result?.ok ? result : null;
    }

    return { projectGraphRenderPlan };
  }

  root.__ANALYTIX_FLOW_GRAPH_RENDER_PLAN_CLIENT__ = {
    createGraphRenderPlanClient,
  };
})();
