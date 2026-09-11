/* Graph search model.
 * Responsibilities: thin Rust-backed node search client for graph canvas focus.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function requireFunction(value, message) {
    if (typeof value !== "function") throw new Error(message);
    return value;
  }

  function scalarValue(value) {
    if (typeof value === "string") {
      const next = value.trim();
      return next ? next : null;
    }
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "boolean") return value;
    return null;
  }

  function scalarTextForIdentity(value) {
    if (typeof value === "string") return value.trim();
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
    if (typeof value === "boolean") return String(value);
    return "";
  }

  function copySearchScalar(source, target, key) {
    const value = scalarValue(source?.[key]);
    if (value !== null) target[key] = value;
  }

  function normalizeSearchDisplayIds(value) {
    if (Array.isArray(value)) {
      return value.map(scalarValue).filter((item) => item !== null);
    }
    return scalarValue(value);
  }

  function copySearchDisplayIds(source, target, key) {
    const value = normalizeSearchDisplayIds(source?.[key]);
    if (Array.isArray(value)) {
      if (value.length) target[key] = value;
      return;
    }
    if (value) target[key] = value;
  }

  function toSearchNodeRow(item) {
    const model =
      item && typeof item.getModel === "function"
        ? item.getModel() || {}
        : item && typeof item === "object"
          ? item
          : {};
    const row = {};
    copySearchScalar(model, row, "id");
    if (!scalarTextForIdentity(row.id)) return null;
    copySearchScalar(model, row, "title");
    copySearchScalar(model, row, "label");
    copySearchScalar(model, row, "displayId");
    copySearchScalar(model, row, "display_id");
    copySearchScalar(model, row, "displayIdRaw");
    copySearchScalar(model, row, "display_id_raw");
    copySearchDisplayIds(model, row, "displayIds");
    copySearchDisplayIds(model, row, "display_ids");
    return row;
  }

  function normalizeNodeRows(nodes = []) {
    return (Array.isArray(nodes) ? nodes : [])
      .map(toSearchNodeRow)
      .filter(Boolean);
  }

  function createGraphSearchClient(deps = {}) {
    const backendCall = requireFunction(deps?.backendCall, "graph-search-client-backend-call-missing");
    const parseJSONSafe = requireFunction(deps?.parseJSONSafe, "graph-search-client-json-missing");
    const getCaseId =
      typeof deps?.getCaseId === "function"
        ? deps.getCaseId
        : () => text(deps?.caseId || "");

    async function projectGraphSearch(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const raw = await backendCall(
        "projectGraphSearch",
        JSON.stringify({
          caseId: text(getCaseId() || ""),
          nodes: normalizeNodeRows(request.nodes),
          query: text(request.query || request.searchQuery || request.search_query),
          limit: Number.isFinite(Number(request.limit)) ? Math.max(1, Math.min(1000, Math.trunc(Number(request.limit)))) : 1,
        })
      );
      const result = parseJSONSafe(raw, {});
      if (!result || result.ok !== true) {
        throw new Error(text(result?.error) || "graph-search-failed");
      }
      const nodeIds = Array.isArray(result.nodeIds) ? result.nodeIds.map(text).filter(Boolean) : [];
      return {
        query: text(result.query),
        nodeIds,
        firstNodeId: text(result.firstNodeId ?? result.first_node_id ?? nodeIds[0] ?? ""),
        indexSummary:
          result.indexSummary && typeof result.indexSummary === "object" && !Array.isArray(result.indexSummary)
            ? { ...result.indexSummary }
            : result.index_summary && typeof result.index_summary === "object" && !Array.isArray(result.index_summary)
              ? { ...result.index_summary }
              : {},
      };
    }

    return { projectGraphSearch };
  }

  root.__ANALYTIX_FLOW_GRAPH_SEARCH_MODEL__ = {
    createGraphSearchClient,
    normalizeNodeRows,
  };
})();
