/* Analysis graph data model.
 * Responsibilities: preserve node coordinates and call the Rust-backed analysis graph projection.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function requireFunction(value, message) {
    if (typeof value !== "function") throw new Error(message);
    return value;
  }

  function textId(value) {
    return String(value || "").trim();
  }

  function isMapLike(value) {
    return (
      value &&
      typeof value === "object" &&
      typeof value.get === "function" &&
      typeof value.set === "function" &&
      typeof value.size === "number"
    );
  }

  function collectFiniteNodePositions(nodes = [], out = null) {
    const map = isMapLike(out) ? out : new Map();
    (Array.isArray(nodes) ? nodes : []).forEach((node) => {
      const id = textId(node?.id);
      if (!id) return;
      const x = Number(node?.x);
      const y = Number(node?.y);
      if (!Number.isFinite(x) || !Number.isFinite(y)) return;
      map.set(id, { x, y });
    });
    return map;
  }

  function applyNodePositions(nodes = [], positions = null, { overwrite = false } = {}) {
    if (!Array.isArray(nodes) || !isMapLike(positions) || !positions.size) return;
    nodes.forEach((node) => {
      const id = textId(node?.id);
      if (!id) return;
      const pos = positions.get(id);
      if (!pos) return;
      const hasX = Number.isFinite(Number(node?.x));
      const hasY = Number.isFinite(Number(node?.y));
      if (overwrite || !hasX) node.x = pos.x;
      if (overwrite || !hasY) node.y = pos.y;
    });
  }

  function createAnalysisGraphDataClient(deps = {}) {
    const backendCall = requireFunction(
      deps?.backendCall,
      "analysis-graph-data-client-backend-call-missing"
    );
    const parseJSONSafe = requireFunction(
      deps?.parseJSONSafe,
      "analysis-graph-data-client-json-missing"
    );
    const getCaseId =
      typeof deps?.getCaseId === "function"
        ? deps.getCaseId
        : () => String(deps?.caseId || "");

    async function projectAnalysisGraphData(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const raw = await backendCall(
        "projectAnalysisGraphData",
        JSON.stringify({
          ...request,
          caseId: String(getCaseId() || ""),
        })
      );
      const result = parseJSONSafe(raw, {});
      return result && typeof result === "object" ? result : null;
    }

    return { projectAnalysisGraphData };
  }

  root.__ANALYTIX_FLOW_ANALYSIS_GRAPH_DATA_MODEL__ = {
    collectFiniteNodePositions,
    applyNodePositions,
    createAnalysisGraphDataClient,
  };
})();
