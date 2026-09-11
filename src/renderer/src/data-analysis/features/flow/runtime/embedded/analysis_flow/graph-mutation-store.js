(() => {
  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function cloneRow(row) {
    return row && typeof row === "object" ? { ...row } : {};
  }

  function cloneRuntimeGraph(graph, patchAdapter = null) {
    if (patchAdapter && typeof patchAdapter.cloneRuntimeGraph === "function") {
      return patchAdapter.cloneRuntimeGraph(graph);
    }
    const source = graph && typeof graph === "object" ? graph : {};
    return {
      nodes: Array.isArray(source.nodes) ? source.nodes.map((item) => cloneRow(item)) : [],
      edges: Array.isArray(source.edges) ? source.edges.map((item) => cloneRow(item)) : [],
    };
  }

  function countGraphEntities(graph) {
    const source = graph && typeof graph === "object" ? graph : {};
    return {
      nodes: Array.isArray(source.nodes) ? source.nodes.length : 0,
      edges: Array.isArray(source.edges) ? source.edges.length : 0,
    };
  }

  function resolveRuntimeRevision(value, fallback = 0) {
    const source = value && typeof value === "object" ? value : {};
    const direct = Number(source.runtime_revision ?? source.runtimeRevision);
    if (Number.isFinite(direct)) {
      return direct;
    }
    const next = Number(value);
    return Number.isFinite(next) ? next : fallback;
  }

  function buildEmptyMetrics() {
    return {
      scheduledCount: 0,
      publishedCount: 0,
      skippedCount: 0,
      failedCount: 0,
      lastPatchKind: "",
      lastPatchScope: "",
      lastPatchSource: "",
      lastReason: "",
      lastViewId: "",
      lastLayoutPreset: "",
      lastPatchAt: 0,
      lastOpCount: 0,
      lastTargetEntityCount: 0,
      lastBaseNodes: 0,
      lastBaseEdges: 0,
      lastTargetNodes: 0,
      lastTargetEdges: 0,
      lastBaseRuntimeRevision: 0,
      lastRuntimeRevision: 0,
      lastTraceId: "",
      pending: false,
      pendingScope: "",
    };
  }

  function createPatchTraceId(scope = "local-update") {
    const normalizedScope = text(scope || "local-update") || "local-update";
    const nonce = Math.random().toString(36).slice(2, 8);
    return `flow-patch:${normalizedScope}:${Date.now().toString(36)}:${nonce}`;
  }

  function createGraphMutationStore({
    patchAdapter = null,
    getBackend = null,
    getGraphData = null,
    getActiveViewId = null,
    getLayoutPreset = null,
    onPayload = null,
    onRevisionChange = null,
    patchDelayMs = 80,
  } = {}) {
    const metrics = buildEmptyMetrics();
    let baseGraph = null;
    let baseRuntimeRevision = 0;
    let pendingMeta = null;
    let publishTimer = 0;

    function rememberBase(graph = null, { runtimeRevision = null } = {}) {
      const resolvedRuntimeRevision = resolveRuntimeRevision(
        runtimeRevision,
        resolveRuntimeRevision(graph, baseRuntimeRevision)
      );
      const sourceGraph = graph && typeof graph === "object" ? { ...graph, runtime_revision: resolvedRuntimeRevision } : graph;
      baseGraph = cloneRuntimeGraph(sourceGraph, patchAdapter);
      baseRuntimeRevision = resolvedRuntimeRevision;
      const counts = countGraphEntities(baseGraph);
      metrics.lastBaseNodes = counts.nodes;
      metrics.lastBaseEdges = counts.edges;
      metrics.lastBaseRuntimeRevision = resolvedRuntimeRevision;
      metrics.lastRuntimeRevision = resolvedRuntimeRevision;
      if (typeof onRevisionChange === "function") {
        onRevisionChange(resolvedRuntimeRevision);
      }
      return baseGraph;
    }

    function currentBase() {
      return baseGraph && typeof baseGraph === "object" ? baseGraph : null;
    }

    function currentBaseRevision() {
      return baseRuntimeRevision;
    }

    function notePayload(payload, meta = {}) {
      const patch = payload && typeof payload === "object" ? payload : {};
      const summary =
        patch.runtime_graph_patch && typeof patch.runtime_graph_patch.summary === "object"
          ? patch.runtime_graph_patch.summary
          : {};
      const graphCounts = countGraphEntities(patch.runtime_graph);
      metrics.lastPatchKind = text(patch.patch_kind);
      metrics.lastPatchScope = text(patch.patch_scope || meta.scope);
      metrics.lastPatchSource = text(patch.patch_source || meta.source);
      metrics.lastReason = text(patch.reason || meta.reason);
      metrics.lastViewId = text(patch.view_id || meta.viewId);
      metrics.lastLayoutPreset = text(patch.layout_preset || meta.layoutPreset);
      metrics.lastBaseRuntimeRevision = resolveRuntimeRevision(patch.base_runtime_revision, metrics.lastBaseRuntimeRevision);
      metrics.lastRuntimeRevision = resolveRuntimeRevision(patch.runtime_revision, metrics.lastRuntimeRevision);
      metrics.lastPatchAt = Date.now();
      metrics.lastTraceId = text(patch.trace_id || meta.traceId || metrics.lastTraceId);
      metrics.lastOpCount = Math.max(0, Number(summary.op_count) || 0);
      metrics.lastTargetEntityCount = Math.max(0, Number(summary.target_entity_count) || graphCounts.nodes + graphCounts.edges);
      metrics.lastTargetNodes = Math.max(0, Number(summary.target_nodes) || graphCounts.nodes);
      metrics.lastTargetEdges = Math.max(0, Number(summary.target_edges) || graphCounts.edges);
    }

    function resolvePayload(meta = {}) {
      if (!patchAdapter || typeof patchAdapter.buildGraphPatchPayload !== "function") {
        return null;
      }
      const traceId = text(meta.traceId) || createPatchTraceId(meta.scope || "local-update");
      const nextRuntimeRevision = resolveRuntimeRevision(meta.runtimeRevision, baseRuntimeRevision + 1);
      const targetGraph = cloneRuntimeGraph(typeof getGraphData === "function" ? getGraphData() : null, patchAdapter);
      if (Number.isFinite(nextRuntimeRevision)) {
        targetGraph.runtime_revision = nextRuntimeRevision;
      }
      const counts = countGraphEntities(targetGraph);
      const payload = patchAdapter.buildGraphPatchPayload({
        baseGraph: currentBase(),
        targetGraph,
        baseRuntimeRevision,
        runtimeRevision: nextRuntimeRevision,
        traceId,
        nodes: Array.isArray(targetGraph.nodes) ? targetGraph.nodes : [],
        edges: Array.isArray(targetGraph.edges) ? targetGraph.edges : [],
        stats: {
          node_count: counts.nodes,
          edge_count: counts.edges,
        },
        patchScope: meta.scope || "local-update",
        patchSource: meta.source || "local",
        viewId:
          meta.viewId ||
          (typeof getActiveViewId === "function" ? text(getActiveViewId()) : ""),
        layoutPreset:
          meta.layoutPreset ||
          (typeof getLayoutPreset === "function" ? text(getLayoutPreset()) : ""),
        reason: meta.reason || "",
        forceFull: !!meta.forceFull,
      });
      return { payload, targetGraph, runtimeRevision: nextRuntimeRevision, traceId };
    }

    function publish(meta = {}) {
      const backend = typeof getBackend === "function" ? getBackend() : null;
      const resolved = resolvePayload(meta);
      if (!resolved) {
        rememberBase(typeof getGraphData === "function" ? getGraphData() : null);
        metrics.skippedCount += 1;
        return false;
      }
      const { payload, targetGraph, runtimeRevision, traceId } = resolved;
      meta = { ...meta, traceId };
      notePayload(payload, meta);
      if (!backend || typeof backend.publishFlowGraphPatch !== "function") {
        rememberBase(targetGraph, { runtimeRevision });
        metrics.skippedCount += 1;
        if (typeof onPayload === "function") {
          onPayload(payload, { ok: false, skipped: true, meta });
        }
        return false;
      }
      try {
        backend.publishFlowGraphPatch(JSON.stringify(payload), () => {});
        rememberBase(targetGraph, { runtimeRevision });
        metrics.publishedCount += 1;
        if (typeof onPayload === "function") {
          onPayload(payload, { ok: true, meta });
        }
        return true;
      } catch (error) {
        rememberBase(targetGraph, { runtimeRevision });
        metrics.failedCount += 1;
        if (typeof onPayload === "function") {
          onPayload(payload, { ok: false, error, meta });
        }
        return false;
      }
    }

    function schedule(meta = {}) {
      pendingMeta = pendingMeta && typeof pendingMeta === "object" ? { ...pendingMeta, ...meta } : { ...meta };
      metrics.scheduledCount += 1;
      metrics.pending = true;
      metrics.pendingScope = text(pendingMeta.scope);
      if (publishTimer) {
        return;
      }
      publishTimer = window.setTimeout(() => {
        publishTimer = 0;
        const nextMeta = pendingMeta || {};
        pendingMeta = null;
        metrics.pending = false;
        metrics.pendingScope = "";
        publish(nextMeta);
      }, Math.max(0, Number(patchDelayMs) || 0));
    }

    function flushPending() {
      if (!publishTimer) {
        return false;
      }
      try {
        clearTimeout(publishTimer);
      } catch (e) {}
      publishTimer = 0;
      const nextMeta = pendingMeta || {};
      pendingMeta = null;
      metrics.pending = false;
      metrics.pendingScope = "";
      return publish(nextMeta);
    }

    function getMetricsSnapshot() {
      return {
        ...metrics,
        pending: !!metrics.pending,
        pendingScope: text(metrics.pendingScope),
        base: {
          nodes: metrics.lastBaseNodes,
          edges: metrics.lastBaseEdges,
        },
        target: {
          nodes: metrics.lastTargetNodes,
          edges: metrics.lastTargetEdges,
        },
        revisions: {
          base: metrics.lastBaseRuntimeRevision,
          current: metrics.lastRuntimeRevision,
        },
      };
    }

    return {
      rememberBase,
      currentBase,
      currentBaseRevision,
      publish,
      schedule,
      flushPending,
      getMetricsSnapshot,
    };
  }

  function normalizeGraphPayload(graph, cloneGraphData = null, clone = false) {
    if (clone && typeof cloneGraphData === "function") {
      return cloneGraphData(graph);
    }
    const source = graph && typeof graph === "object" ? graph : {};
    const payload = {
      nodes: Array.isArray(source.nodes) ? source.nodes : [],
      edges: Array.isArray(source.edges) ? source.edges : [],
    };
    const runtimeRevision = Number(source.runtime_revision ?? source.runtimeRevision);
    if (Number.isFinite(runtimeRevision)) {
      payload.runtime_revision = runtimeRevision;
    }
    const graphTier = text(source.graph_tier || source.graphTier);
    if (graphTier) {
      payload.graph_tier = graphTier;
    }
    if (source.render_hints && typeof source.render_hints === "object" && !Array.isArray(source.render_hints)) {
      payload.render_hints = { ...source.render_hints };
    } else if (source.renderHints && typeof source.renderHints === "object" && !Array.isArray(source.renderHints)) {
      payload.render_hints = { ...source.renderHints };
    }
    if (source.projection && typeof source.projection === "object" && !Array.isArray(source.projection)) {
      payload.projection = JSON.parse(JSON.stringify(source.projection));
    }
    if (source.result_snapshot_ref && typeof source.result_snapshot_ref === "object" && !Array.isArray(source.result_snapshot_ref)) {
      payload.result_snapshot_ref = { ...source.result_snapshot_ref };
    } else if (source.resultSnapshotRef && typeof source.resultSnapshotRef === "object" && !Array.isArray(source.resultSnapshotRef)) {
      payload.result_snapshot_ref = { ...source.resultSnapshotRef };
    }
    const hasFlowSummary =
      Object.prototype.hasOwnProperty.call(source, "flow_summary") ||
      Object.prototype.hasOwnProperty.call(source, "flowSummary");
    if (hasFlowSummary) {
      const flowSummary = source.flow_summary ?? source.flowSummary;
      payload.flow_summary =
        flowSummary && typeof flowSummary === "object" && !Array.isArray(flowSummary)
          ? { ...flowSummary }
          : null;
    }
    return payload;
  }

  function createGraphMutationCommandStore({
    getState = null,
    cloneGraphData = null,
    getGraphMutationStore = null,
    onGraphDataReplaced = null,
    cloneGraphProjection = null,
    cloneResultSnapshotRef = null,
    normalizeGraphTier = null,
    normalizeGraphRenderHints = null,
    getGraphMode = null,
    scheduleProjectionPointLayerRedraw = null,
    rebindLayoutWorkerPendingGraphData = null,
    updatePerfMode = null,
  } = {}) {
    const metrics = {
      replaceCount: 0,
      clearCount: 0,
      rememberBaseCount: 0,
      lastReason: "",
      lastAt: 0,
      lastNodes: 0,
      lastEdges: 0,
      lastCloned: false,
    };

    function resolveState() {
      return typeof getState === "function" ? getState() : null;
    }

    function cloneProjection(value) {
      if (typeof cloneGraphProjection === "function") {
        return cloneGraphProjection(value);
      }
      if (!value || typeof value !== "object" || Array.isArray(value)) return null;
      try {
        return JSON.parse(JSON.stringify(value));
      } catch {
        return null;
      }
    }

    function cloneSnapshotRef(value) {
      if (typeof cloneResultSnapshotRef === "function") {
        return cloneResultSnapshotRef(value);
      }
      if (!value || typeof value !== "object" || Array.isArray(value)) return null;
      return { ...value };
    }

    function replaceGraphData(
      nextGraph = null,
      { clone = false, rememberBase = false, reason = "", runtimeRevision = null, preserveGraphMeta = true } = {}
    ) {
      const state = resolveState();
      if (!state || !state.graph || typeof state.graph !== "object") {
        return normalizeGraphPayload(nextGraph, cloneGraphData, clone);
      }
      const previousFlowSummary =
        state.graph.data?.flow_summary && typeof state.graph.data.flow_summary === "object"
          ? { ...state.graph.data.flow_summary }
          : null;
      const payload = normalizeGraphPayload(nextGraph, cloneGraphData, clone);
      state.graph.data = payload;
      const resolvedRuntimeRevision = resolveRuntimeRevision(
        runtimeRevision,
        resolveRuntimeRevision(payload, state.graph.runtimeRevision || 0)
      );
      state.graph.runtimeRevision = resolvedRuntimeRevision;
      if (typeof normalizeGraphTier === "function") {
        state.graphTier = normalizeGraphTier(
          payload?.graph_tier || payload?.graphTier || state.graphTier,
          Array.isArray(payload?.nodes) ? payload.nodes.length : 0
        );
      }
      if (typeof normalizeGraphRenderHints === "function") {
        state.renderHints = normalizeGraphRenderHints(
          payload?.render_hints || payload?.renderHints || state.renderHints,
          Array.isArray(payload?.nodes) ? payload.nodes.length : 0,
          Array.isArray(payload?.edges) ? payload.edges.length : 0,
          typeof getGraphMode === "function" ? text(getGraphMode()) || "relation" : "relation"
        );
      }
      const hasProjection = Object.prototype.hasOwnProperty.call(payload || {}, "projection");
      const hasSnapshotRef =
        Object.prototype.hasOwnProperty.call(payload || {}, "result_snapshot_ref") ||
        Object.prototype.hasOwnProperty.call(payload || {}, "resultSnapshotRef");
      state.graphProjection = hasProjection
        ? cloneProjection(payload?.projection)
        : preserveGraphMeta
        ? cloneProjection(state.graphProjection)
        : null;
      state.resultSnapshotRef = hasSnapshotRef
        ? cloneSnapshotRef(payload?.result_snapshot_ref || payload?.resultSnapshotRef)
        : preserveGraphMeta
        ? cloneSnapshotRef(state.resultSnapshotRef)
        : null;
      if (state.graphProjection) {
        state.graph.data.projection = cloneProjection(state.graphProjection);
      } else if (state.graph?.data && Object.prototype.hasOwnProperty.call(state.graph.data, "projection")) {
        delete state.graph.data.projection;
      }
      if (typeof scheduleProjectionPointLayerRedraw === "function") {
        scheduleProjectionPointLayerRedraw("graph-meta");
      }
      if (state.resultSnapshotRef) {
        state.graph.data.result_snapshot_ref = cloneSnapshotRef(state.resultSnapshotRef);
      } else if (state.graph?.data && Object.prototype.hasOwnProperty.call(state.graph.data, "result_snapshot_ref")) {
        delete state.graph.data.result_snapshot_ref;
      }
      if (
        !Object.prototype.hasOwnProperty.call(payload, "flow_summary") &&
        preserveGraphMeta &&
        previousFlowSummary
      ) {
        state.graph.data.flow_summary = previousFlowSummary;
      }
      if (typeof rebindLayoutWorkerPendingGraphData === "function") {
        rebindLayoutWorkerPendingGraphData(state.graph.data);
      }
      if (typeof updatePerfMode === "function") {
        updatePerfMode(Array.isArray(payload?.nodes) ? payload.nodes.length : 0, Array.isArray(payload?.edges) ? payload.edges.length : 0);
      }
      metrics.replaceCount += 1;
      metrics.lastReason = text(reason);
      metrics.lastAt = Date.now();
      metrics.lastNodes = Array.isArray(payload.nodes) ? payload.nodes.length : 0;
      metrics.lastEdges = Array.isArray(payload.edges) ? payload.edges.length : 0;
      metrics.lastCloned = !!clone;
      if (typeof onGraphDataReplaced === "function") {
        onGraphDataReplaced(payload, { reason: metrics.lastReason });
      }
      if (rememberBase) {
        const mutationStore = typeof getGraphMutationStore === "function" ? getGraphMutationStore() : null;
        if (mutationStore && typeof mutationStore.rememberBase === "function") {
          mutationStore.rememberBase(payload, { runtimeRevision: resolvedRuntimeRevision });
          metrics.rememberBaseCount += 1;
        }
      }
      return payload;
    }

    function clearGraphData(options = {}) {
      metrics.clearCount += 1;
      return replaceGraphData(
        { nodes: [], edges: [], projection: null, result_snapshot_ref: null },
        { ...options, preserveGraphMeta: false }
      );
    }

    function getMetricsSnapshot() {
      return {
        ...metrics,
      };
    }

    return {
      replaceGraphData,
      clearGraphData,
      getMetricsSnapshot,
    };
  }

  window.__ANALYTIX_FLOW_GRAPH_MUTATION_STORE__ = {
    cloneRuntimeGraph,
    createGraphMutationStore,
    createGraphMutationCommandStore,
  };
})();
