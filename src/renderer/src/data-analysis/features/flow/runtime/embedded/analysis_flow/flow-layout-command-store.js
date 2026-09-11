(() => {
  function createFlowLayoutCommandStore({ state = null, deps = {} } = {}) {
    const queryAll = typeof deps.queryAll === "function" ? deps.queryAll : () => [];
    const scheduleShellStateSync =
      typeof deps.scheduleShellStateSync === "function" ? deps.scheduleShellStateSync : () => {};
    const terminateLayoutPrewarmTask =
      typeof deps.terminateLayoutPrewarmTask === "function" ? deps.terminateLayoutPrewarmTask : () => {};
    const restoreNetworkPresentation =
      typeof deps.restoreNetworkPresentation === "function" ? deps.restoreNetworkPresentation : () => {};
    const reapplyGraphTierVisualState =
      typeof deps.reapplyGraphTierVisualState === "function" ? deps.reapplyGraphTierVisualState : () => {};
    const clearGraphSelection = typeof deps.clearGraphSelection === "function" ? deps.clearGraphSelection : () => {};
    const syncGraphNodePositions =
      typeof deps.syncGraphNodePositions === "function" ? deps.syncGraphNodePositions : () => {};
    const captureNodePositions = typeof deps.captureNodePositions === "function" ? deps.captureNodePositions : () => null;
    const getGraphDataBounds =
      typeof deps.getGraphDataBounds === "function" ? deps.getGraphDataBounds : () => null;
    const resolveFocusIdFromNodes =
      typeof deps.resolveFocusIdFromNodes === "function" ? deps.resolveFocusIdFromNodes : () => "";
    const applyLayout = typeof deps.applyLayout === "function" ? deps.applyLayout : () => {};
    const updateMinimap = typeof deps.updateMinimap === "function" ? deps.updateMinimap : () => {};
    const shouldAnimateLayoutSwitch =
      typeof deps.shouldAnimateLayoutSwitch === "function" ? deps.shouldAnimateLayoutSwitch : () => false;
    const animateToLayout = typeof deps.animateToLayout === "function" ? deps.animateToLayout : () => false;
    const finalizeView = typeof deps.finalizeView === "function" ? deps.finalizeView : () => {};
    const scheduleProjectionLayoutIndexSync =
      typeof deps.scheduleProjectionLayoutIndexSync === "function" ? deps.scheduleProjectionLayoutIndexSync : () => {};
    const publishGraphPatch = typeof deps.publishGraphPatch === "function" ? deps.publishGraphPatch : () => {};
    const createLayoutTraceId =
      typeof deps.createLayoutTraceId === "function" ? deps.createLayoutTraceId : () => "";
    const applyGraphNodeTargets =
      typeof deps.applyGraphNodeTargets === "function" ? deps.applyGraphNodeTargets : () => false;
    const buildGraphPositionUpdates =
      typeof deps.buildGraphPositionUpdates === "function" ? deps.buildGraphPositionUpdates : () => [];
    const ensureGraphDataVisible =
      typeof deps.ensureGraphDataVisible === "function" ? deps.ensureGraphDataVisible : () => {};
    const resolveEdgeRefreshBatchSize =
      typeof deps.resolveEdgeRefreshBatchSize === "function" ? deps.resolveEdgeRefreshBatchSize : () => 0;
    const resolveEdgeBatchSize =
      typeof deps.resolveEdgeBatchSize === "function" ? deps.resolveEdgeBatchSize : () => 0;
    const scheduleGraphItemRefreshBatches =
      typeof deps.scheduleGraphItemRefreshBatches === "function" ? deps.scheduleGraphItemRefreshBatches : () => {};
    const graphItemRefreshBatchTimeoutMs = Math.max(0, Number(deps.graphItemRefreshBatchTimeoutMs) || 0);
    const edgeBatchFinalRefreshThreshold = Math.max(1, Number(deps.edgeBatchFinalRefreshThreshold) || 1);
    const graphLayoutConfig = deps.graphLayoutConfig || {};

    function updateLayoutButtons() {
      const mode = state?.layoutPreset || "compact";
      const hasNodes = state?.graph?.data?.nodes?.length > 0;
      const show = hasNodes && !!state?.layoutSelected;
      queryAll(".layoutBtn").forEach((btn) => {
        btn.classList.toggle("active", show && btn.dataset.layout === mode);
      });
      scheduleShellStateSync("layout-buttons");
    }

    function syncGraphEdgeRoutingFromData(graph, edges) {
      if (!graph || !Array.isArray(edges)) return;
      const edgeMap = new Map(edges.map((edge) => [edge.id, edge]));
      const changed = [];
      (graph.getEdges?.() || []).forEach((item) => {
        const model = item?.getModel?.();
        const src = edgeMap.get(model?.id);
        if (!model || !src) return;
        const nextBackflow = !!src.layoutBackflow;
        const nextBackflowTag = !!src.__layoutBackflow;
        const nextRoute = src.edgeRoute ? String(src.edgeRoute) : "";
        const nextAxis = src.orthAxis ? String(src.orthAxis) : "";
        const nextBiasRaw = Number(src.orthBias);
        const hasNextBias = Number.isFinite(nextBiasRaw);
        const nextSharedRaw = Number(src.orthSharedCoord);
        const hasNextShared = Number.isFinite(nextSharedRaw);
        const prevBackflow = !!model.layoutBackflow;
        const prevBackflowTag = !!model.__layoutBackflow;
        const prevRoute = model.edgeRoute ? String(model.edgeRoute) : "";
        const prevAxis = model.orthAxis ? String(model.orthAxis) : "";
        const prevBiasRaw = Number(model.orthBias);
        const hasPrevBias = Number.isFinite(prevBiasRaw);
        const prevSharedRaw = Number(model.orthSharedCoord);
        const hasPrevShared = Number.isFinite(prevSharedRaw);
        const biasChanged =
          hasNextBias !== hasPrevBias || (hasNextBias && hasPrevBias && Math.abs(nextBiasRaw - prevBiasRaw) > 1e-6);
        const sharedChanged =
          hasNextShared !== hasPrevShared ||
          (hasNextShared && hasPrevShared && Math.abs(nextSharedRaw - prevSharedRaw) > 1e-6);
        const changedNow =
          prevBackflow !== nextBackflow ||
          prevBackflowTag !== nextBackflowTag ||
          prevRoute !== nextRoute ||
          prevAxis !== nextAxis ||
          biasChanged ||
          sharedChanged;
        model.layoutBackflow = nextBackflow;
        model.__layoutBackflow = nextBackflowTag;
        if (nextRoute) model.edgeRoute = nextRoute;
        else delete model.edgeRoute;
        if (nextAxis) model.orthAxis = nextAxis;
        else delete model.orthAxis;
        if (hasNextBias) model.orthBias = nextBiasRaw;
        else delete model.orthBias;
        if (hasNextShared) model.orthSharedCoord = nextSharedRaw;
        else delete model.orthSharedCoord;
        if (changedNow) changed.push(item);
      });
      if (!changed.length) return;
      if (changed.length >= edgeBatchFinalRefreshThreshold) {
        scheduleGraphItemRefreshBatches(graph, changed, {
          batchSize: Math.max(resolveEdgeRefreshBatchSize(changed.length), resolveEdgeBatchSize(changed.length)),
          timeout: graphItemRefreshBatchTimeoutMs,
          onComplete: () => {
            try {
              if (typeof graph.refreshPositions === "function") graph.refreshPositions();
            } catch (e) {}
            graph.paint?.();
          },
        });
        return;
      }
      const canSetAutoPaint = typeof graph.setAutoPaint === "function";
      const prevAutoPaint = canSetAutoPaint ? graph.autoPaint !== false : true;
      if (canSetAutoPaint) graph.setAutoPaint(false);
      try {
        if (typeof graph.refreshItem === "function") {
          changed.forEach((item) => {
            try {
              graph.refreshItem(item);
            } catch (e) {}
          });
        }
        if (typeof graph.refreshPositions === "function") graph.refreshPositions();
      } catch (e) {}
      if (canSetAutoPaint) graph.setAutoPaint(prevAutoPaint);
      if (typeof graph.paint === "function") graph.paint();
    }

    function setLayoutPreset(
      preset,
      { animate = true, markSelected = false, ensureGraph = null, forceRecompute = false, semanticProjection = null } = {}
    ) {
      const mode = preset || "compact";
      if (String(mode || "").trim().toLowerCase() === "network") {
        terminateLayoutPrewarmTask("network-selected");
      }
      state.layoutPreset = mode;
      if (markSelected) state.layoutSelected = true;
      updateLayoutButtons();
      if (!state?.graph?.data?.nodes?.length) return;
      const graph = typeof ensureGraph === "function" ? ensureGraph() : null;
      if (!graph) return;
      restoreNetworkPresentation(graph);
      state.networkLayout.activeMode = "full";
      state.networkLayout.hiddenNodeCount = 0;
      state.networkLayout.hiddenEdgeCount = 0;
      if (mode !== "network" && state.networkLayout.expandedCommunities instanceof Set) {
        state.networkLayout.expandedCommunities.clear();
      }
      reapplyGraphTierVisualState(graph, { preserveEdgeLabels: true });
      clearGraphSelection(graph);
      syncGraphNodePositions(graph);
      const beforeNodePositions = captureNodePositions(graph);
      const prevBounds = getGraphDataBounds(state.graph.data.nodes);
      const prevCenter = prevBounds
        ? { x: (prevBounds.minX + prevBounds.maxX) / 2, y: (prevBounds.minY + prevBounds.maxY) / 2 }
        : null;
      const focusId = resolveFocusIdFromNodes(state.graph.data.nodes, state.focusId, state.focusName);
      applyLayout(state.graph.data.nodes, state.graph.data.edges, mode, focusId, {
        disablePlanCache: !!forceRecompute,
        semanticProjection,
      });
      state.layoutTopologyDirty = false;
      const pending = state.layoutWorkerPending;
      const deferredByWorker =
        !!pending &&
        pending.nodes === state.graph.data.nodes &&
        pending.edges === state.graph.data.edges &&
        pending.preset === mode;
      const deferNodeCount = state.graph.data?.nodes?.length || 0;
      const deferSmallGraph = deferNodeCount > 1 && deferNodeCount <= 20;
      if (deferredByWorker && pending) {
        pending.deferApply = {
          animate: !!animate,
          duration: deferSmallGraph ? 760 : 560,
          easing: deferSmallGraph ? "easeOutCubic" : graphLayoutConfig.ANIM_EASING,
          fit: pending.deferApply?.fit != null ? !!pending.deferApply.fit : !!state.graphLoading,
          startPositions: beforeNodePositions,
          preserveCenter:
            prevCenter && Number.isFinite(prevCenter.x) && Number.isFinite(prevCenter.y)
              ? { x: prevCenter.x, y: prevCenter.y }
              : null,
        };
        pending.patchMeta = {
          scope: "layout-switch",
          reason: "set-layout-preset",
          viewId: state.activeViewId || "",
          layoutPreset: mode,
          traceId: pending.traceId || createLayoutTraceId(mode),
        };
      } else if (prevCenter) {
        const nextBounds = getGraphDataBounds(state.graph.data.nodes);
        if (nextBounds) {
          const nextCenter = { x: (nextBounds.minX + nextBounds.maxX) / 2, y: (nextBounds.minY + nextBounds.maxY) / 2 };
          const dx = prevCenter.x - nextCenter.x;
          const dy = prevCenter.y - nextCenter.y;
          if (Number.isFinite(dx) && Number.isFinite(dy)) {
            state.graph.data.nodes.forEach((node) => {
              if (!Number.isFinite(node.x) || !Number.isFinite(node.y)) return;
              node.x += dx;
              node.y += dy;
            });
          }
        }
      }
      if (mode === "compact") {
        state.networkLayout.lastCompactCoreIds = [];
      }
      if (deferredByWorker) {
        updateMinimap();
        return;
      }
      syncGraphEdgeRoutingFromData(graph, state.graph.data.edges);
      const nodeCount = state.graph.data?.nodes?.length || 0;
      const edgeCount = state.graph.data?.edges?.length || 0;
      const targetRows = Array.isArray(state.graph.data.nodes) ? state.graph.data.nodes : [];
      const targetUpdates = buildGraphPositionUpdates(targetRows);
      const targets = new Map(targetRows.map((node) => [node.id, { x: node.x, y: node.y }]));
      const animateSwitch = !!animate && shouldAnimateLayoutSwitch(nodeCount, edgeCount, mode, state);
      if (animateSwitch) {
        const nodes = graph.getNodes ? graph.getNodes() : [];
        if (!nodes.length) {
          graph.data(state.graph.data);
          graph.render();
          if (graph.paint) graph.paint();
          ensureGraphDataVisible(graph, state.graph.data.nodes, state.graph.data.edges, "layout-apply");
          updateMinimap();
          finalizeView();
          return;
        }
        animateToLayout(beforeNodePositions, targets, {
          graph,
          duration: 520,
          finalize: false,
          fit: false,
          onComplete: () => {
            finalizeView();
            scheduleProjectionLayoutIndexSync("set-layout-preset-animate", { delayMs: 80 });
            publishGraphPatch({
              scope: "layout-switch",
              reason: "set-layout-preset",
              viewId: state.activeViewId || "",
              layoutPreset: mode,
              traceId: createLayoutTraceId(mode),
            });
          },
        });
        return;
      }
      const applied = applyGraphNodeTargets(graph, targetRows, targetUpdates, { paint: true });
      if (!applied) {
        graph.data(state.graph.data);
        graph.render();
        if (graph.paint) graph.paint();
        ensureGraphDataVisible(graph, state.graph.data.nodes, state.graph.data.edges, "layout-apply");
      }
      updateMinimap();
      finalizeView();
      scheduleProjectionLayoutIndexSync("set-layout-preset", { delayMs: 80 });
      publishGraphPatch({
        scope: "layout-switch",
        reason: "set-layout-preset",
        viewId: state.activeViewId || "",
        layoutPreset: mode,
        traceId: createLayoutTraceId(mode),
      });
    }

    return {
      updateLayoutButtons,
      syncGraphEdgeRoutingFromData,
      setLayoutPreset,
    };
  }

  window.__ANALYTIX_FLOW_LAYOUT_COMMAND_STORE__ = {
    createFlowLayoutCommandStore,
  };
})();
