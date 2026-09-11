/* Network layout presentation model.
 * Responsibilities: restore network/skeleton presentation state after visibility plans are cleared.
 */
(function () {
  const root = typeof window !== "undefined" ? window : globalThis;

  function defaultAsBool(value, fallback = false) {
    if (value == null) return !!fallback;
    if (typeof value === "boolean") return value;
    if (typeof value === "number") return Number.isFinite(value) && value !== 0;
    if (typeof value === "string") {
      const raw = value.trim().toLowerCase();
      if (!raw) return false;
      if (["0", "false", "no", "off", "null", "undefined", "nan"].includes(raw)) return false;
      if (["1", "true", "yes", "on"].includes(raw)) return true;
      return true;
    }
    return !!value;
  }

  function restoreGraphDataVisibility(nodes = [], edges = []) {
    if (Array.isArray(nodes)) {
      nodes.forEach((node) => {
        if (!node || typeof node !== "object") return;
        node.layoutHiddenBySkeleton = false;
        node.nodeLabelHidden = false;
      });
    }
    if (Array.isArray(edges)) {
      edges.forEach((edge) => {
        if (!edge || typeof edge !== "object") return;
        edge.layoutHiddenBySkeleton = false;
        restoreDenseEdgeBase(edge);
      });
    }
  }

  function restoreDenseEdgeBase(edge) {
    if (!edge || typeof edge !== "object") return false;
    const hasDenseBase =
      edge.__networkDenseBaseStroke != null ||
      edge.__networkDenseBaseLineWidth != null ||
      edge.__networkDenseBaseShowArrow != null ||
      edge.__networkDenseBaseLabel != null;
    if (!hasDenseBase) return false;
    if (edge.__networkDenseBaseStroke != null) edge.stroke = String(edge.__networkDenseBaseStroke || "");
    if (edge.__networkDenseBaseLineWidth != null) edge.lineWidth = Number(edge.__networkDenseBaseLineWidth);
    if (edge.__networkDenseBaseShowArrow != null) edge.showArrow = !!edge.__networkDenseBaseShowArrow;
    if (edge.__networkDenseBaseLabel != null) edge.label = edge.__networkDenseBaseLabel ?? "";
    if (edge.__networkDenseBaseLabelTop != null) edge.labelTop = edge.__networkDenseBaseLabelTop ?? "";
    if (edge.__networkDenseBaseLabelBottom != null) edge.labelBottom = edge.__networkDenseBaseLabelBottom ?? "";
    edge.edgeLabelHidden = false;
    edge.layoutEdgeLabelTier = "";
    edge.__networkDenseStyled = "";
    return true;
  }

  function restoreNetworkNode(graph, item, { graphStyle, asBool }) {
    const model = item?.getModel?.() || {};
    if (model.__networkBaseR == null && model.__networkBaseLineWidth == null) {
      if (model.layoutHiddenBySkeleton || model.nodeLabelHidden) {
        graph.updateItem(item, { layoutHiddenBySkeleton: false, nodeLabelHidden: false });
      }
      return;
    }
    graph.updateItem(item, {
      r: Number(model.__networkBaseR != null ? model.__networkBaseR : model.r),
      lineWidth: Number(model.__networkBaseLineWidth != null ? model.__networkBaseLineWidth : model.lineWidth),
      stroke: String(model.__networkBaseStroke || model.stroke || "rgba(2,6,23,.72)"),
      fill: String(model.__networkBaseFill || model.fill || "rgba(255,255,255,0.05)"),
      nodeShadow:
        model.__networkBaseNodeShadow != null
          ? !!model.__networkBaseNodeShadow
          : asBool(model.nodeShadow, asBool(graphStyle?.nodeShadow, false)),
      layoutHiddenBySkeleton: false,
      nodeLabelHidden: false,
    });
  }

  function restoreNetworkEdge(graph, item) {
    const model = item?.getModel?.() || {};
    const denseRestore = restoreDenseEdgeBase(model);
    if (model.__networkBaseStroke == null && model.__networkBaseLineWidth == null) {
      if (model.layoutHiddenBySkeleton || denseRestore) {
        graph.updateItem(item, {
          stroke: String(model.stroke || "rgba(2,6,23,.68)"),
          lineWidth: Number(model.lineWidth || 1.6),
          showArrow: !!model.showArrow,
          label: model.label ?? "",
          labelTop: model.labelTop ?? "",
          labelBottom: model.labelBottom ?? "",
          edgeLabelHidden: false,
          layoutEdgeLabelTier: "",
          layoutHiddenBySkeleton: false,
        });
      }
      return;
    }
    graph.updateItem(item, {
      stroke: String(model.__networkBaseStroke || model.stroke || "rgba(2,6,23,.68)"),
      lineWidth: Number(model.__networkBaseLineWidth != null ? model.__networkBaseLineWidth : model.lineWidth),
      showArrow: model.__networkBaseShowArrow != null ? !!model.__networkBaseShowArrow : !!model.showArrow,
      label: model.label ?? "",
      labelTop: model.labelTop ?? "",
      labelBottom: model.labelBottom ?? "",
      edgeLabelHidden: false,
      layoutEdgeLabelTier: "",
      layoutHiddenBySkeleton: false,
    });
  }

  function restoreNetworkPresentation(graph, options = {}) {
    if (!graph) return false;
    const nodes = Array.isArray(options.nodes) ? options.nodes : [];
    const edges = Array.isArray(options.edges) ? options.edges : [];
    const graphStyle = options.graphStyle && typeof options.graphStyle === "object" ? options.graphStyle : {};
    const asBool = typeof options.asBool === "function" ? options.asBool : defaultAsBool;
    restoreGraphDataVisibility(nodes, edges);
    graph.setAutoPaint?.(false);
    try {
      (graph.getNodes?.() || []).forEach((item) => restoreNetworkNode(graph, item, { graphStyle, asBool }));
      (graph.getEdges?.() || []).forEach((item) => restoreNetworkEdge(graph, item));
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    return true;
  }

  root.__ANALYTIX_FLOW_LAYOUT_NETWORK_PRESENTATION_MODEL__ = {
    restoreNetworkPresentation,
  };
})();
