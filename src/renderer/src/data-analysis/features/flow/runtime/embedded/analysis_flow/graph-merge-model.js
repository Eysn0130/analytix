/* Graph merge model.
 * Responsibilities: call Rust-backed graph merge projections from the embedded runtime.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function requireFunction(value, message) {
    if (typeof value !== "function") throw new Error(message);
    return value;
  }

  function createSameNameMergeClient(deps = {}) {
    const backendCall = requireFunction(
      deps?.backendCall,
      "same-name-merge-client-backend-call-missing"
    );
    const parseJSONSafe = requireFunction(
      deps?.parseJSONSafe,
      "same-name-merge-client-json-missing"
    );
    const getCaseId =
      typeof deps?.getCaseId === "function"
        ? deps.getCaseId
        : () => String(deps?.caseId || "");

    async function mergeSameNameGraph(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const raw = await backendCall(
        "mergeSameNameGraph",
        JSON.stringify({
          ...request,
          caseId: String(getCaseId() || ""),
          mode: request.mode === "net" ? "net" : "gross",
        })
      );
      const result = parseJSONSafe(raw, {});
      return result && typeof result === "object" ? result : null;
    }

    return { mergeSameNameGraph };
  }

  root.__ANALYTIX_FLOW_GRAPH_MERGE_MODEL__ = {
    createSameNameMergeClient,
  };
})();
