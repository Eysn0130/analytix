(() => {
  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function cloneRow(row) {
    return row && typeof row === "object" ? { ...row } : {};
  }

  function cloneRuntimeGraph(graph) {
    const source = graph && typeof graph === "object" ? graph : {};
    const runtimeRevision = Number(source.runtime_revision ?? source.runtimeRevision);
    const payload = {
      nodes: Array.isArray(source.nodes) ? source.nodes.map((item) => cloneRow(item)) : [],
      edges: Array.isArray(source.edges) ? source.edges.map((item) => cloneRow(item)) : [],
    };
    if (Number.isFinite(runtimeRevision)) {
      payload.runtime_revision = runtimeRevision;
    }
    return payload;
  }

  function getRuntimeNodeIdentity(row, index) {
    const source = row && typeof row === "object" ? row : {};
    const fields = ["id", "node_id", "display_id", "label", "title", "name"];
    for (let i = 0; i < fields.length; i += 1) {
      const value = text(source[fields[i]]);
      if (value) return value;
    }
    return `node:${index}`;
  }

  function getRuntimeEdgeIdentity(row, index) {
    const sourceRow = row && typeof row === "object" ? row : {};
    const directId = text(sourceRow.id || sourceRow.edge_id);
    if (directId) return directId;
    const source = text(sourceRow.source || sourceRow.from_node_id);
    const target = text(sourceRow.target || sourceRow.to_node_id);
    if (source && target) {
      const mode = text(sourceRow.mode);
      const arrow = text(sourceRow.edgeArrow || sourceRow.arrow);
      const label = text(sourceRow.label);
      const amount = sourceRow.amount_total != null ? sourceRow.amount_total : sourceRow.amount;
      const count = sourceRow.tx_count != null ? sourceRow.tx_count : sourceRow.count;
      return `${source}->${target}|m:${mode}|a:${arrow}|l:${label}|amt:${String(amount ?? "")}|cnt:${String(count ?? "")}|idx:${index}`;
    }
    return `edge:${index}`;
  }

  function buildRuntimeGraphPatch(baseGraph, targetGraph) {
    const base = cloneRuntimeGraph(baseGraph);
    const target = cloneRuntimeGraph(targetGraph);

    const baseNodes = new Map();
    base.nodes.forEach((row, index) => {
      baseNodes.set(getRuntimeNodeIdentity(row, index), cloneRow(row));
    });
    const targetNodes = new Map();
    target.nodes.forEach((row, index) => {
      targetNodes.set(getRuntimeNodeIdentity(row, index), cloneRow(row));
    });

    const baseEdges = new Map();
    base.edges.forEach((row, index) => {
      baseEdges.set(getRuntimeEdgeIdentity(row, index), cloneRow(row));
    });
    const targetEdges = new Map();
    target.edges.forEach((row, index) => {
      targetEdges.set(getRuntimeEdgeIdentity(row, index), cloneRow(row));
    });

    const removeNodeIds = [];
    baseNodes.forEach((_row, nodeId) => {
      if (!targetNodes.has(nodeId)) removeNodeIds.push(nodeId);
    });
    const upsertNodes = [];
    targetNodes.forEach((row, nodeId) => {
      const prev = baseNodes.get(nodeId);
      if (!prev || JSON.stringify(prev) !== JSON.stringify(row)) {
        upsertNodes.push(cloneRow(row));
      }
    });

    const removeEdgeIds = [];
    baseEdges.forEach((_row, edgeId) => {
      if (!targetEdges.has(edgeId)) removeEdgeIds.push(edgeId);
    });
    const upsertEdges = [];
    targetEdges.forEach((row, edgeId) => {
      const prev = baseEdges.get(edgeId);
      if (!prev || JSON.stringify(prev) !== JSON.stringify(row)) {
        upsertEdges.push(cloneRow(row));
      }
    });

    const opCount = removeNodeIds.length + upsertNodes.length + removeEdgeIds.length + upsertEdges.length;
    const targetEntityCount = target.nodes.length + target.edges.length;

    return {
      remove_node_ids: removeNodeIds,
      upsert_nodes: upsertNodes,
      remove_edge_ids: removeEdgeIds,
      upsert_edges: upsertEdges,
      summary: {
        base_nodes: base.nodes.length,
        base_edges: base.edges.length,
        target_nodes: target.nodes.length,
        target_edges: target.edges.length,
        op_count: opCount,
        target_entity_count: targetEntityCount,
      },
    };
  }

  function normalizePatchScope(value) {
    const raw = text(value).toLowerCase();
    if (raw === "build" || raw === "view-activate" || raw === "layout-switch" || raw === "local-update") {
      return raw;
    }
    return "local-update";
  }

  function normalizePatchSource(value) {
    return text(value).toLowerCase() === "ws" ? "ws" : "local";
  }

  function buildGraphPatchPayload({
    baseGraph = null,
    targetGraph = null,
    baseRuntimeRevision = null,
    runtimeRevision = null,
    traceId = "",
    baseSnapshotRef = null,
    resultSnapshotRef = null,
    nodes = [],
    edges = [],
    stats = {},
    patchScope = "local-update",
    patchSource = "local",
    viewId = "",
    layoutPreset = "",
    reason = "",
    forceFull = false,
  } = {}) {
    const nextGraph = cloneRuntimeGraph(targetGraph);
    const normalizedBaseRuntimeRevision = Number(baseRuntimeRevision);
    const normalizedRuntimeRevision = Number(runtimeRevision);
    if (Number.isFinite(normalizedRuntimeRevision)) {
      nextGraph.runtime_revision = normalizedRuntimeRevision;
    }
    const patch = buildRuntimeGraphPatch(baseGraph, nextGraph);
    const summary = patch && typeof patch.summary === "object" ? patch.summary : {};
    const targetEntityCount = Math.max(0, Number(summary.target_entity_count) || 0);
    const opCount = Math.max(0, Number(summary.op_count) || 0);
    const useDelta = !forceFull && targetEntityCount > 0 && opCount < targetEntityCount;
    return {
      base_snapshot_ref: baseSnapshotRef && typeof baseSnapshotRef === "object" ? { ...baseSnapshotRef } : null,
      result_snapshot_ref: resultSnapshotRef && typeof resultSnapshotRef === "object" ? { ...resultSnapshotRef } : null,
      base_runtime_revision: Number.isFinite(normalizedBaseRuntimeRevision) ? normalizedBaseRuntimeRevision : null,
      runtime_revision: Number.isFinite(normalizedRuntimeRevision) ? normalizedRuntimeRevision : null,
      trace_id: text(traceId),
      patch_kind: useDelta ? "delta" : "full",
      runtime_graph_patch: useDelta ? patch : null,
      runtime_graph: useDelta ? null : nextGraph,
      nodes: Array.isArray(nodes) ? nodes.map((item) => cloneRow(item)) : [],
      edges: Array.isArray(edges) ? edges.map((item) => cloneRow(item)) : [],
      stats: stats && typeof stats === "object" ? { ...stats } : {},
      patch_scope: normalizePatchScope(patchScope),
      patch_source: normalizePatchSource(patchSource),
      view_id: text(viewId),
      layout_preset: text(layoutPreset),
      reason: text(reason),
    };
  }

  window.__ANALYTIX_FLOW_GRAPH_PATCH_ADAPTER__ = {
    cloneRuntimeGraph,
    buildRuntimeGraphPatch,
    buildGraphPatchPayload,
    normalizePatchScope,
    normalizePatchSource,
  };
})();
