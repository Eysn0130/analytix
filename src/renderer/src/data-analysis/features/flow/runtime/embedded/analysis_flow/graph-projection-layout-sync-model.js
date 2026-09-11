/* Graph projection layout sync model.
 * Responsibilities: collect lightweight graph model DTOs for the Rust-backed
 * projection layout sync contract.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function jsonSafeNumberValue(value) {
    return typeof value === "number" && !Number.isFinite(value) ? "NaN" : value;
  }

  function collectProjectionLayoutSyncModels(models = []) {
    return (Array.isArray(models) ? models : [])
      .filter((model) => model && typeof model === "object")
      .map((model) => ({
        id: model?.id,
        x: jsonSafeNumberValue(model?.x),
        y: jsonSafeNumberValue(model?.y),
        cluster_node: model?.cluster_node,
        cluster_id: model?.cluster_id,
        projection_cluster_id: model?.projection_cluster_id,
        nodeRenderMode: model?.nodeRenderMode,
        node_render_mode: model?.node_render_mode,
        projection_visible: model?.projection_visible,
        projection_collapsed: model?.projection_collapsed,
      }));
  }

  root.__ANALYTIX_FLOW_GRAPH_PROJECTION_LAYOUT_SYNC_MODEL__ = Object.freeze({
    collectProjectionLayoutSyncModels,
  });
})();
