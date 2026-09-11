(() => {
  function safeCall(fn, ...args) {
    if (typeof fn !== "function") return undefined;
    try {
      return fn(...args);
    } catch (e) {
      return undefined;
    }
  }

  function createCanvasDragController({
    graph = null,
    state = null,
    usingWebglEngine = false,
    isPerfMode = null,
    setInteractionPerf = null,
    log = null,
    syncGraphNodePositions = null,
    applyLayoutEdgeRouting = null,
    syncGraphEdgeRoutingFromData = null,
    updateCurrentViewSnapshot = null,
    updateMinimap = null,
    scheduleGraphPatch = null,
  } = {}) {
    let dragGroupCtx = null;
    let hierarchyRoutingDragRaf = 0;

    function shouldUsePerfMode() {
      return !!safeCall(isPerfMode);
    }

    function applyHierarchyRoutingDuringDrag() {
      const mode = String(state?.layoutPreset || "").trim().toLowerCase();
      if (mode !== "hierarchy") return;
      if (!state?.layoutEdgeRouting?.hierarchy) return;
      const nodes = state?.graph?.data?.nodes;
      const edges = state?.graph?.data?.edges;
      if (!Array.isArray(nodes) || !nodes.length) return;
      if (!Array.isArray(edges) || !edges.length) return;
      safeCall(syncGraphNodePositions, graph);
      safeCall(applyLayoutEdgeRouting, nodes, edges, "hierarchy");
      safeCall(syncGraphEdgeRoutingFromData, graph, edges);
    }

    function scheduleHierarchyRoutingDuringDrag() {
      if (shouldUsePerfMode()) return;
      if (hierarchyRoutingDragRaf) return;
      hierarchyRoutingDragRaf = requestAnimationFrame(() => {
        hierarchyRoutingDragRaf = 0;
        applyHierarchyRoutingDuringDrag();
      });
    }

    function flushHierarchyRoutingDuringDrag() {
      if (hierarchyRoutingDragRaf) {
        try {
          cancelAnimationFrame(hierarchyRoutingDragRaf);
        } catch (e) {}
        hierarchyRoutingDragRaf = 0;
      }
      applyHierarchyRoutingDuringDrag();
    }

    function handleNodeDrag(ev) {
      const node = ev?.item || null;
      const model = node?.getModel?.() || {};
      const id = model.id || "";
      if (dragGroupCtx && id && dragGroupCtx.anchorId === id) {
        const nowX = Number(model.x);
        const nowY = Number(model.y);
        const prevX = Number(dragGroupCtx.lastX);
        const prevY = Number(dragGroupCtx.lastY);
        const dx = nowX - prevX;
        const dy = nowY - prevY;
        if (Number.isFinite(dx) && Number.isFinite(dy) && (Math.abs(dx) > 1e-6 || Math.abs(dy) > 1e-6)) {
          dragGroupCtx.lastX = nowX;
          dragGroupCtx.lastY = nowY;
          dragGroupCtx.memberItems.forEach((item) => {
            const m = item?.getModel?.();
            if (!m || !Number.isFinite(m.x) || !Number.isFinite(m.y)) return;
            m.x += dx;
            m.y += dy;
          });
          graph?.refreshPositions?.();
          if (!usingWebglEngine && !shouldUsePerfMode()) {
            try {
              dragGroupCtx.memberItems.forEach((item) => {
                item.getEdges?.().forEach((edge) => graph?.refreshItem?.(edge));
              });
            } catch (e) {}
          }
        }
      }
      if (!usingWebglEngine) {
        try {
          node?.getEdges?.().forEach((edge) => graph?.refreshItem?.(edge));
        } catch (e) {}
      }
      scheduleHierarchyRoutingDuringDrag();
    }

    function handleNodeDragStart(ev) {
      dragGroupCtx = null;
      const anchor = ev?.item || null;
      const anchorModel = anchor?.getModel?.() || {};
      const anchorId = anchorModel.id || "";
      if (anchorId) {
        let selectedNodes = graph?.findAllByState?.("node", "selected") || [];
        const hasAnchorInState = selectedNodes.some((item) => item?.getModel?.()?.id === anchorId);
        if ((!hasAnchorInState || selectedNodes.length <= 1) && state?.selected instanceof Set && state.selected.has(anchorId)) {
          selectedNodes = Array.from(state.selected)
            .map((id) => graph?.findById?.(id))
            .filter((item) => item?.getType?.() === "node" && item?.getModel?.()?.id);
        }
        const memberItems = selectedNodes.filter((item) => item?.getModel?.()?.id && item.getModel().id !== anchorId);
        if (memberItems.length) {
          dragGroupCtx = {
            anchorId,
            lastX: Number(anchorModel.x) || 0,
            lastY: Number(anchorModel.y) || 0,
            memberItems,
          };
          safeCall(log, "INFO", "drag group start", {
            anchorId,
            members: memberItems.length,
          });
        }
      }
      if (shouldUsePerfMode()) safeCall(setInteractionPerf, true, "node:dragstart");
    }

    function handleNodeDragEnd(ev) {
      if (!usingWebglEngine) {
        const node = ev?.item || null;
        try {
          node?.getEdges?.().forEach((edge) => graph?.refreshItem?.(edge));
        } catch (e) {}
      }
      flushHierarchyRoutingDuringDrag();
      if (dragGroupCtx) {
        safeCall(log, "INFO", "drag group end", {
          anchorId: dragGroupCtx.anchorId || "",
          members: dragGroupCtx.memberItems?.length || 0,
        });
      }
      dragGroupCtx = null;
      const positionsChanged = !!safeCall(syncGraphNodePositions, graph);
      safeCall(updateCurrentViewSnapshot);
      safeCall(updateMinimap);
      if (positionsChanged) {
        safeCall(scheduleGraphPatch, {
          scope: "local-update",
          reason: "node-drag-end",
        });
      }
      if (shouldUsePerfMode()) safeCall(setInteractionPerf, false, "node:dragend");
      return positionsChanged;
    }

    function handleCanvasDragStart() {
      if (shouldUsePerfMode()) safeCall(setInteractionPerf, true, "canvas:dragstart");
    }

    function handleCanvasDragEnd() {
      if (shouldUsePerfMode()) safeCall(setInteractionPerf, false, "canvas:dragend");
    }

    function dispose() {
      dragGroupCtx = null;
      if (hierarchyRoutingDragRaf) {
        try {
          cancelAnimationFrame(hierarchyRoutingDragRaf);
        } catch (e) {}
        hierarchyRoutingDragRaf = 0;
      }
    }

    return {
      handleNodeDrag,
      handleNodeDragStart,
      handleNodeDragEnd,
      handleCanvasDragStart,
      handleCanvasDragEnd,
      dispose,
    };
  }

  window.__ANALYTIX_FLOW_CANVAS_DRAG_CONTROLLER__ = {
    createCanvasDragController,
  };
})();
