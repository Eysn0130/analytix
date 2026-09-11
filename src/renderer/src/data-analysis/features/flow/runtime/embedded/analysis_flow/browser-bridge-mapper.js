(function initAnalytixFlowBridgeMapper(global) {
  "use strict";

  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function asObject(value) {
    return value && typeof value === "object" && !Array.isArray(value) ? value : {};
  }

  function uniqueStrings(values) {
    var source = Array.isArray(values) ? values : typeof values === "string" ? [values] : [];
    var result = [];
    var seen = new Set();
    source.forEach(function eachValue(item) {
      var next = text(item);
      if (!next || seen.has(next)) return;
      seen.add(next);
      result.push(next);
    });
    return result;
  }

  function toFiniteNumber(value, fallback) {
    var next = Number(value);
    return Number.isFinite(next) ? next : fallback;
  }

  function clampInt(value, fallback, minValue, maxValue) {
    var next = Math.round(toFiniteNumber(value, fallback));
    return Math.max(minValue, Math.min(maxValue, next));
  }

  function cloneJson(value, fallback) {
    try {
      return JSON.parse(JSON.stringify(value));
    } catch (_error) {
      return fallback;
    }
  }

  function cloneGraphEntities(values) {
    return (Array.isArray(values) ? values : []).map(function mapItem(item) {
      return item && typeof item === "object" ? cloneJson(item, {}) : {};
    });
  }

  function sanitizeSnapshotRef(value) {
    var source = asObject(value);
    var snapshotId = text(source.snapshot_id || source.snapshotId);
    var graphHash = text(source.graph_hash || source.graphHash);
    if (!snapshotId && !graphHash) return null;
    return {
      snapshot_id: snapshotId,
      graph_hash: graphHash,
      node_count: Math.max(0, clampInt(source.node_count != null ? source.node_count : source.nodeCount, 0, 0, Number.MAX_SAFE_INTEGER)),
      edge_count: Math.max(0, clampInt(source.edge_count != null ? source.edge_count : source.edgeCount, 0, 0, Number.MAX_SAFE_INTEGER)),
      stored_at: text(source.stored_at || source.storedAt),
    };
  }

  function sanitizeFlowRuntimeGraphPayload(value) {
    var source = asObject(value);
    return {
      nodes: cloneGraphEntities(source.nodes),
      edges: cloneGraphEntities(source.edges),
    };
  }

  function buildBlockedFlowSummary() {
    return {
      contract: "FlowGraphSummaryV1",
      status: "blocked",
      factAnswerAllowed: false,
      layoutLabel: "布局",
      nodes: null,
      edges: null,
      amount: null,
      boundaryCode: "publication_blocked",
    };
  }

  function isBlockedPublicFlowResult(value) {
    var source = asObject(value);
    return (
      source.contract === "FlowPublicResultBoundaryV1" &&
      source.publication_status === "blocked" &&
      source.fact_answer_allowed === false &&
      source.content_access === "controlled_artifact_required"
    );
  }

  function projectSavedFlowViewRecord(value, fallbackCaseId) {
    var source = asObject(value);
    return {
      id: text(source.id || source.view_id || source.viewId),
      title: "Saved flow view",
      mode: "relation",
      focusId: "",
      focusName: "",
      focusIds: [],
      focusNames: [],
      focusPlaceholderKinds: [],
      focusOnly: false,
      focusCounterpartyStrict: false,
      filters: {
        dir: "all",
        hop: 1,
        minAmount: 0,
        maxEdges: 800,
      },
      style: null,
      viewport: null,
      graph: { nodes: [], edges: [] },
      counts: { nodes: 0, edges: 0 },
      saved: true,
      caseId: text(source.caseId || source.case_id || fallbackCaseId),
      createdAt: "",
      updatedAt: "",
    };
  }

  function resolveRuntimeGraph(flowData) {
    var payload = asObject(flowData);
    var runtimeGraph = payload.runtime_graph || payload.runtimeGraph;
    if (runtimeGraph && typeof runtimeGraph === "object") {
      return sanitizeFlowRuntimeGraphPayload(runtimeGraph);
    }
    return null;
  }

  function getRawRuntimeGraph(flowData) {
    var payload = asObject(flowData);
    var runtimeGraph = payload.runtime_graph || payload.runtimeGraph;
    return runtimeGraph && typeof runtimeGraph === "object" ? runtimeGraph : null;
  }

  function shouldUseNetworkRuntimeGraphFastPath(requestPayload, flowData, runtimeGraph, graphTier) {
    var request = asObject(requestPayload);
    var layout = text(request.layout || request.layoutPreset).toLowerCase();
    if (layout !== "network") return false;
    var graph = asObject(runtimeGraph);
    var nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    var edges = Array.isArray(graph.edges) ? graph.edges : [];
    var tier = text(graphTier || (flowData && (flowData.graph_tier || flowData.graphTier))).toLowerCase();
    return tier === "large" || tier === "xlarge" || nodes.length > 1200 || edges.length > 3500;
  }

  function buildFlowRuntimeGraphResponse(requestPayload, flowData) {
    if (isBlockedPublicFlowResult(flowData)) {
      return {
        ok: true,
        nodes: [],
        edges: [],
        stats: asObject(flowData && flowData.stats),
        flow_summary: buildBlockedFlowSummary(),
        result_snapshot_ref: null,
      };
    }
    var graphTier = text(flowData && (flowData.graph_tier || flowData.graphTier));
    var renderHints = cloneJson(asObject(flowData && (flowData.render_hints || flowData.renderHints)), {});
    var projection = cloneJson(asObject(flowData && flowData.projection), {});
    var rawRuntimeGraph = getRawRuntimeGraph(flowData);
    if (shouldUseNetworkRuntimeGraphFastPath(requestPayload, flowData, rawRuntimeGraph, graphTier)) {
      return {
        ok: true,
        nodes: Array.isArray(rawRuntimeGraph.nodes) ? rawRuntimeGraph.nodes : [],
        edges: Array.isArray(rawRuntimeGraph.edges) ? rawRuntimeGraph.edges : [],
        stats: asObject(flowData && flowData.stats),
        graph_tier: graphTier,
        render_hints: renderHints,
        projection: projection,
        result_snapshot_ref: sanitizeSnapshotRef(flowData && (flowData.result_snapshot_ref || flowData.resultSnapshotRef)),
        runtime_graph_passthrough: true,
      };
    }
    var runtimeGraph = rawRuntimeGraph ? sanitizeFlowRuntimeGraphPayload(rawRuntimeGraph) : null;
    if (runtimeGraph) {
      return {
        ok: true,
        nodes: runtimeGraph.nodes,
        edges: runtimeGraph.edges,
        stats: asObject(flowData && flowData.stats),
        graph_tier: graphTier,
        render_hints: renderHints,
        projection: projection,
        result_snapshot_ref: sanitizeSnapshotRef(flowData && (flowData.result_snapshot_ref || flowData.resultSnapshotRef)),
      };
    }

    var selectedSeeds = new Set(uniqueStrings(requestPayload && requestPayload.seeds));
    var nodeStats = new Map();
    var rawEdges = Array.isArray(flowData && flowData.edges) ? flowData.edges : [];

    rawEdges.forEach(function eachEdge(edge) {
      var sourceId = text(edge && edge.from_node_id);
      var targetId = text(edge && edge.to_node_id);
      var amount = toFiniteNumber(edge && edge.amount_total, 0);
      var count = clampInt(edge && edge.tx_count, 0, 0, Number.MAX_SAFE_INTEGER);

      [sourceId, targetId].forEach(function eachNodeId(nodeId) {
        if (!nodeId) return;
        var current = nodeStats.get(nodeId) || { total_amount: 0, total_count: 0 };
        current.total_amount += amount;
        current.total_count += count;
        nodeStats.set(nodeId, current);
      });
    });

    var nodes = (Array.isArray(flowData && flowData.nodes) ? flowData.nodes : []).map(function mapNode(node) {
      var nodeId = text(node && node.node_id);
      var totals = nodeStats.get(nodeId) || { total_amount: 0, total_count: 0 };
      var nodeType = text(node && node.node_type).toLowerCase();
      return {
        id: nodeId,
        title: text(node && node.label) || nodeId,
        name: nodeType === "holder" ? text(node && node.label) : "",
        display_id: nodeType === "account" ? nodeId : "",
        ntype: selectedSeeds.has(nodeId) ? "seed" : nodeType === "unknown" ? "unknown" : "node",
        total_amount: totals.total_amount,
        total_count: totals.total_count,
      };
    });

    var edges = rawEdges.map(function mapEdge(edge) {
      var amount = toFiniteNumber(edge && edge.amount_total, 0);
      var count = clampInt(edge && edge.tx_count, 0, 0, Number.MAX_SAFE_INTEGER);
      return {
        id: text(edge && edge.edge_id) || text(edge && edge.from_node_id) + "=>" + text(edge && edge.to_node_id),
        source: text(edge && edge.from_node_id),
        target: text(edge && edge.to_node_id),
        amount: amount,
        count: count,
        label: amount > 0 ? "￥" + amount.toLocaleString("zh-CN", { maximumFractionDigits: 2 }) : String(count || ""),
        first_time: "",
        last_time: "",
      };
    });

    return {
      ok: true,
      nodes: nodes,
      edges: edges,
      stats: asObject(flowData && flowData.stats),
      graph_tier: graphTier,
      render_hints: renderHints,
      projection: projection,
      result_snapshot_ref: sanitizeSnapshotRef(flowData && (flowData.result_snapshot_ref || flowData.resultSnapshotRef)),
    };
  }

  function mapBackendViewToRuntimeView(item) {
    return projectSavedFlowViewRecord(item, text(item && (item.case_id || item.caseId)));
  }

  global.__ANALYTIX_FLOW_BRIDGE_MAPPER__ = {
    buildFlowRuntimeGraphResponse: buildFlowRuntimeGraphResponse,
    mapBackendViewToRuntimeView: mapBackendViewToRuntimeView,
    sanitizeFlowRuntimeGraphPayload: sanitizeFlowRuntimeGraphPayload,
    sanitizeSnapshotRef: sanitizeSnapshotRef,
  };
})(window);
