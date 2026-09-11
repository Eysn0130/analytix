(() => {
  const CLIPBOARD_TYPE = "application/x-analytix-flow-graph-selection";
  const CLIPBOARD_VERSION = 1;

  const TRANSIENT_EDGE_KEYS = new Set([
    "id",
    "__baseLabel",
    "__baseLabelTop",
    "__baseLabelBottom",
    "__hitLane",
    "__layoutBackflow",
    "__selectedLane",
    "edgeLabelAnchorMode",
    "edgeLabelMiddleRatio",
    "edgeLabelOffsetY",
    "edgeLabelPreferDegree",
    "edgeLabelSideCapRatio",
    "edgeLabelSideOffset",
    "edgeOffset",
    "fill",
    "fontBold",
    "fontFamily",
    "fontItalic",
    "fontShadow",
    "fontSize",
    "fontUnderline",
    "gap",
    "holePad",
    "layoutBackflow",
    "layoutBridge",
    "layoutCorridor",
    "layoutCorridorLevel",
    "layoutHiddenBySkeleton",
    "lineWidth",
    "orthBias",
    "orthSharedCoord",
    "outPad",
    "showArrow",
    "stroke",
    "subTextColor",
    "textColor",
    "type",
  ]);

  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function cloneValue(value) {
    if (value == null || typeof value !== "object") return value;
    try {
      return JSON.parse(JSON.stringify(value));
    } catch (e) {
      return Array.isArray(value) ? value.slice() : { ...value };
    }
  }

  function cloneRow(row) {
    return row && typeof row === "object" ? cloneValue(row) : {};
  }

  function normalizeIdList(value) {
    return (Array.isArray(value) ? value : [])
      .map((item) => text(item))
      .filter(Boolean);
  }

  function getNodeId(row) {
    return text(row?.id || row?.node_id || row?.display_id || row?.displayId);
  }

  function getEdgeId(row) {
    return text(row?.id || row?.edge_id);
  }

  function getEdgeSource(row) {
    return text(row?.source || row?.from_node_id);
  }

  function getEdgeTarget(row) {
    return text(row?.target || row?.to_node_id);
  }

  function edgeSelectionKey(row, index = 0) {
    const id = getEdgeId(row);
    if (id) return id;
    const source = getEdgeSource(row);
    const target = getEdgeTarget(row);
    return source && target ? `${source}->${target}|${index}` : "";
  }

  function stableSanitize(value, { edge = false } = {}) {
    if (Array.isArray(value)) {
      return value.map((item) => stableSanitize(item, { edge }));
    }
    if (!value || typeof value !== "object") {
      return value;
    }
    const out = {};
    Object.keys(value)
      .sort()
      .forEach((key) => {
        if (edge && TRANSIENT_EDGE_KEYS.has(key)) return;
        if (key.startsWith("__")) return;
        const next = stableSanitize(value[key], { edge });
        if (next !== undefined) out[key] = next;
      });
    return out;
  }

  function stableStringify(value) {
    return JSON.stringify(stableSanitize(value, { edge: true }));
  }

  function edgeSemanticKey(edge) {
    const source = getEdgeSource(edge);
    const target = getEdgeTarget(edge);
    if (!source || !target) return "";
    return `${source}->${target}|${stableStringify(edge || {})}`;
  }

  function buildGraphClipboardPayload({
    graphData = null,
    selectedNodeIds = [],
    selectedEdgeIds = [],
    sourceViewId = "",
    createdAt = "",
  } = {}) {
    const data = graphData && typeof graphData === "object" ? graphData : {};
    const graphNodes = Array.isArray(data.nodes) ? data.nodes : [];
    const graphEdges = Array.isArray(data.edges) ? data.edges : [];
    const selectedNodes = new Set(normalizeIdList(selectedNodeIds));
    const selectedEdges = new Set(normalizeIdList(selectedEdgeIds));
    if (!selectedNodes.size && !selectedEdges.size) return null;

    const nodeRowsById = new Map();
    graphNodes.forEach((node) => {
      const id = getNodeId(node);
      if (id && !nodeRowsById.has(id)) nodeRowsById.set(id, node);
    });

    const edgeRows = [];
    const edgeKeys = new Set();
    graphEdges.forEach((edge, index) => {
      const source = getEdgeSource(edge);
      const target = getEdgeTarget(edge);
      const explicit =
        selectedEdges.has(edgeSelectionKey(edge, index)) ||
        selectedEdges.has(getEdgeId(edge)) ||
        (source && target && selectedEdges.has(`${source}->${target}`));
      const betweenSelected = source && target && selectedNodes.has(source) && selectedNodes.has(target);
      if (!explicit && !betweenSelected) return;
      const key = getEdgeId(edge) || `${source}->${target}|${index}`;
      if (edgeKeys.has(key)) return;
      edgeKeys.add(key);
      edgeRows.push(edge);
      if (source) selectedNodes.add(source);
      if (target) selectedNodes.add(target);
    });

    const nodes = Array.from(selectedNodes)
      .map((id) => nodeRowsById.get(id))
      .filter(Boolean)
      .map((node) => cloneRow(node));
    const availableNodeIds = new Set(nodes.map((node) => getNodeId(node)).filter(Boolean));
    const edges = edgeRows
      .filter((edge) => availableNodeIds.has(getEdgeSource(edge)) && availableNodeIds.has(getEdgeTarget(edge)))
      .map((edge) => cloneRow(edge));

    if (!nodes.length && !edges.length) return null;
    return {
      type: CLIPBOARD_TYPE,
      version: CLIPBOARD_VERSION,
      sourceViewId: text(sourceViewId),
      createdAt: createdAt || new Date().toISOString(),
      nodes,
      edges,
    };
  }

  function parseGraphClipboardPayload(value) {
    const source =
      typeof value === "string"
        ? (() => {
            try {
              return JSON.parse(value);
            } catch (e) {
              return null;
            }
          })()
        : value;
    if (!source || typeof source !== "object") return null;
    if (text(source.type) !== CLIPBOARD_TYPE) return null;
    const nodes = Array.isArray(source.nodes) ? source.nodes.map((node) => cloneRow(node)) : [];
    const edges = Array.isArray(source.edges) ? source.edges.map((edge) => cloneRow(edge)) : [];
    if (!nodes.length && !edges.length) return null;
    return {
      type: CLIPBOARD_TYPE,
      version: Number(source.version) || CLIPBOARD_VERSION,
      sourceViewId: text(source.sourceViewId || source.source_view_id),
      createdAt: text(source.createdAt || source.created_at),
      nodes,
      edges,
    };
  }

  function defaultCreateEdgeId(edge, index, usedIds) {
    const source = getEdgeSource(edge).replace(/[^\w-]+/g, "_") || "source";
    const target = getEdgeTarget(edge).replace(/[^\w-]+/g, "_") || "target";
    let suffix = Math.max(1, Number(index) + 1 || 1);
    let id = `edge_paste_${source}_${target}_${suffix}`;
    while (usedIds?.has?.(id)) {
      suffix += 1;
      id = `edge_paste_${source}_${target}_${suffix}`;
    }
    return id;
  }

  function mergeGraphClipboardPayload(targetGraph = null, payload = null, options = {}) {
    const parsed = parseGraphClipboardPayload(payload);
    const target = targetGraph && typeof targetGraph === "object" ? targetGraph : {};
    const nextNodes = Array.isArray(target.nodes) ? target.nodes.map((node) => cloneRow(node)) : [];
    const nextEdges = Array.isArray(target.edges) ? target.edges.map((edge) => cloneRow(edge)) : [];
    if (!parsed) {
      return {
        graph: { nodes: nextNodes, edges: nextEdges },
        addedNodes: [],
        reusedNodes: [],
        addedEdges: [],
        skippedEdges: [],
      };
    }

    const nodeIds = new Set(nextNodes.map((node) => getNodeId(node)).filter(Boolean));
    const addedNodes = [];
    const reusedNodes = [];
    parsed.nodes.forEach((node) => {
      const id = getNodeId(node);
      if (!id) return;
      if (nodeIds.has(id)) {
        reusedNodes.push(id);
        return;
      }
      const nextNode = cloneRow(node);
      nextNodes.push(nextNode);
      nodeIds.add(id);
      addedNodes.push(id);
    });

    const edgeIds = new Set(nextEdges.map((edge) => getEdgeId(edge)).filter(Boolean));
    const edgeSemanticKeys = new Set(nextEdges.map((edge) => edgeSemanticKey(edge)).filter(Boolean));
    const addedEdges = [];
    const skippedEdges = [];
    const createEdgeId = typeof options.createEdgeId === "function" ? options.createEdgeId : defaultCreateEdgeId;
    parsed.edges.forEach((edge, index) => {
      const source = getEdgeSource(edge);
      const targetId = getEdgeTarget(edge);
      if (!source || !targetId || !nodeIds.has(source) || !nodeIds.has(targetId)) {
        skippedEdges.push(getEdgeId(edge) || `${source}->${targetId}`);
        return;
      }
      const semanticKey = edgeSemanticKey(edge);
      if (semanticKey && edgeSemanticKeys.has(semanticKey)) {
        skippedEdges.push(getEdgeId(edge) || semanticKey);
        return;
      }
      const nextEdge = cloneRow(edge);
      let edgeId = getEdgeId(nextEdge);
      if (!edgeId || edgeIds.has(edgeId)) {
        edgeId = text(createEdgeId(nextEdge, index, edgeIds));
        if (edgeId) nextEdge.id = edgeId;
      }
      if (!edgeId) {
        skippedEdges.push(semanticKey || `${source}->${targetId}`);
        return;
      }
      nextEdges.push(nextEdge);
      edgeIds.add(edgeId);
      if (semanticKey) edgeSemanticKeys.add(semanticKey);
      addedEdges.push(edgeId);
    });

    return {
      graph: { nodes: nextNodes, edges: nextEdges },
      addedNodes,
      reusedNodes,
      addedEdges,
      skippedEdges,
    };
  }

  window.__ANALYTIX_FLOW_GRAPH_CLIPBOARD_MODEL__ = {
    CLIPBOARD_TYPE,
    buildGraphClipboardPayload,
    parseGraphClipboardPayload,
    mergeGraphClipboardPayload,
    edgeSemanticKey,
  };
})();
