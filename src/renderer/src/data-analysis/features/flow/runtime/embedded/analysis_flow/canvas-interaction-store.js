(() => {
  function text(value) {
    return String(value == null ? "" : value).trim();
  }

  function asNumber(value, fallback = 0) {
    const next = Number(value);
    return Number.isFinite(next) ? next : fallback;
  }

  function createEmptyMetrics() {
    return {
      selectionChangeCount: 0,
      highlightChangeCount: 0,
      viewportCaptureCount: 0,
      viewportApplyCount: 0,
      viewportSyncCount: 0,
      redrawCount: 0,
      hitCount: 0,
      lastReason: "",
      lastSelection: {
        type: "",
        id: "",
        selected: false,
        total: 0,
      },
      lastHighlight: {
        type: "",
        id: "",
        active: false,
      },
      lastViewport: {
        zoom: 0,
        centerX: 0,
        centerY: 0,
        hasCenter: false,
      },
      lastHit: {
        kind: "",
        type: "",
        id: "",
        x: 0,
        y: 0,
      },
      lastRedrawDurationMs: 0,
      lastRedrawReason: "",
    };
  }

  function getItemMeta(item) {
    const model = item?.getModel?.() || {};
    return {
      type: text(item?.getType?.()),
      id:
        text(model.id) ||
        text(model.edge_id) ||
        (text(model.source) && text(model.target) ? `${text(model.source)}->${text(model.target)}` : ""),
      model,
    };
  }

  function getSelectionCounts(graph, getSelectedGraphItems) {
    if (!graph || typeof getSelectedGraphItems !== "function") {
      return { nodes: [], edges: [], total: 0 };
    }
    const picked = getSelectedGraphItems(graph) || {};
    const nodes = Array.isArray(picked.nodes) ? picked.nodes : [];
    const edges = Array.isArray(picked.edges) ? picked.edges : [];
    return {
      nodes,
      edges,
      total: nodes.length + edges.length,
    };
  }

  function createCanvasInteractionStore({
    getGraph = null,
    getState = null,
    getSelectedGraphItems = null,
    stopPulseLoop = null,
    syncActiveSelection = null,
    syncEdgeDirectionControl = null,
    updateMinimap = null,
    scheduleLodUpdate = null,
    resolveClientPointFromEvent = null,
    captureGraphViewportImpl = null,
    applyGraphViewportImpl = null,
  } = {}) {
    const metrics = createEmptyMetrics();

    function resolveGraph(targetGraph = null) {
      if (targetGraph) {
        return targetGraph;
      }
      return typeof getGraph === "function" ? getGraph() : null;
    }

    function resolveState() {
      return typeof getState === "function" ? getState() : null;
    }

    function noteReason(reason = "") {
      metrics.lastReason = text(reason);
    }

    function clearSelection(targetGraph = null, reason = "") {
      const graph = resolveGraph(targetGraph);
      if (!graph) return { nodes: 0, edges: 0, total: 0 };
      if (typeof stopPulseLoop === "function") {
        stopPulseLoop();
      }
      const picked = getSelectionCounts(graph, getSelectedGraphItems);
      picked.nodes.forEach((item) => {
        graph.setItemState?.(item, "selected", false);
        graph.setItemState?.(item, "active", false);
      });
      picked.edges.forEach((item) => {
        graph.setItemState?.(item, "selected", false);
        graph.setItemState?.(item, "active", false);
      });
      const state = resolveState();
      if (state && typeof state === "object") {
        state.lastActiveId = "";
      }
      if (typeof syncEdgeDirectionControl === "function") {
        syncEdgeDirectionControl(graph);
      }
      metrics.selectionChangeCount += 1;
      noteReason(reason);
      metrics.lastSelection = {
        type: "",
        id: "",
        selected: false,
        total: 0,
      };
      return picked;
    }

    function toggleItemSelection(item, { targetGraph = null, multi = false, reason = "" } = {}) {
      const graph = resolveGraph(targetGraph);
      if (!graph || !item) return false;
      const meta = getItemMeta(item);
      const wasSelected = !!item.hasState?.("selected");
      if (!multi) {
        clearSelection(graph, `${reason || meta.type || "selection"}:exclusive`);
      }
      const nextSelected = multi ? !wasSelected : true;
      graph.setItemState?.(item, "selected", nextSelected);
      if (typeof syncActiveSelection === "function") {
        syncActiveSelection(graph, nextSelected ? item : null);
      }
      if (typeof syncEdgeDirectionControl === "function") {
        syncEdgeDirectionControl(graph);
      }
      const picked = getSelectionCounts(graph, getSelectedGraphItems);
      metrics.selectionChangeCount += 1;
      noteReason(reason);
      metrics.lastSelection = {
        type: meta.type,
        id: meta.id,
        selected: !!nextSelected,
        total: picked.total,
      };
      return nextSelected;
    }

    function setItemHover(item, active, { targetGraph = null, reason = "" } = {}) {
      const graph = resolveGraph(targetGraph);
      if (!graph || !item) return false;
      const meta = getItemMeta(item);
      graph.setItemState?.(item, "hover", !!active);
      metrics.highlightChangeCount += 1;
      noteReason(reason);
      metrics.lastHighlight = {
        type: meta.type,
        id: meta.id,
        active: !!active,
      };
      return true;
    }

    function captureViewport(targetGraph = null, reason = "") {
      if (typeof captureGraphViewportImpl !== "function") {
        return null;
      }
      const viewport = captureGraphViewportImpl(resolveGraph(targetGraph));
      if (viewport && typeof viewport === "object") {
        const center = viewport.center && typeof viewport.center === "object" ? viewport.center : null;
        metrics.viewportCaptureCount += 1;
        noteReason(reason);
        metrics.lastViewport = {
          zoom: asNumber(viewport.zoom, metrics.lastViewport.zoom),
          centerX: center ? asNumber(center.x, 0) : 0,
          centerY: center ? asNumber(center.y, 0) : 0,
          hasCenter: !!center,
        };
      }
      return viewport;
    }

    function applyViewport(viewport, targetGraph = null, { reason = "" } = {}) {
      if (typeof applyGraphViewportImpl !== "function") {
        return false;
      }
      const ok = !!applyGraphViewportImpl(viewport, resolveGraph(targetGraph));
      if (ok && viewport && typeof viewport === "object") {
        const center = viewport.center && typeof viewport.center === "object" ? viewport.center : null;
        metrics.viewportApplyCount += 1;
        noteReason(reason);
        metrics.lastViewport = {
          zoom: asNumber(viewport.zoom, metrics.lastViewport.zoom),
          centerX: center ? asNumber(center.x, 0) : 0,
          centerY: center ? asNumber(center.y, 0) : 0,
          hasCenter: !!center,
        };
      }
      return ok;
    }

    function syncViewport(targetGraph = null, reason = "") {
      const graph = resolveGraph(targetGraph);
      if (!graph) return;
      if (typeof updateMinimap === "function") {
        updateMinimap();
      }
      if (typeof scheduleLodUpdate === "function") {
        scheduleLodUpdate(graph);
      }
      metrics.viewportSyncCount += 1;
      noteReason(reason);
    }

    function noteHit(kind, item = null, event = null, reason = "") {
      const meta = getItemMeta(item);
      const point =
        typeof resolveClientPointFromEvent === "function" ? resolveClientPointFromEvent(resolveGraph(), event) : null;
      metrics.hitCount += 1;
      noteReason(reason);
      metrics.lastHit = {
        kind: text(kind),
        type: meta.type,
        id: meta.id,
        x: point ? asNumber(point.x, 0) : 0,
        y: point ? asNumber(point.y, 0) : 0,
      };
    }

    function runRedraw(reason = "", fn = null) {
      const startedAt =
        typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
      const result = typeof fn === "function" ? fn() : undefined;
      const endedAt =
        typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
      metrics.redrawCount += 1;
      metrics.lastRedrawReason = text(reason);
      metrics.lastRedrawDurationMs = Math.max(0, Math.round(endedAt - startedAt));
      noteReason(reason);
      return result;
    }

    function getMetricsSnapshot() {
      return {
        ...metrics,
        lastSelection: { ...(metrics.lastSelection || {}) },
        lastHighlight: { ...(metrics.lastHighlight || {}) },
        lastViewport: { ...(metrics.lastViewport || {}) },
        lastHit: { ...(metrics.lastHit || {}) },
      };
    }

    return {
      clearSelection,
      toggleItemSelection,
      setItemHover,
      captureViewport,
      applyViewport,
      syncViewport,
      noteHit,
      runRedraw,
      getMetricsSnapshot,
    };
  }

  window.__ANALYTIX_FLOW_CANVAS_INTERACTION_STORE__ = {
    createCanvasInteractionStore,
  };
})();
