(() => {
  function safeCall(fn, ...args) {
    if (typeof fn !== "function") return undefined;
    try {
      return fn(...args);
    } catch (e) {
      return undefined;
    }
  }

  function createCanvasHitViewportController({
    ensureGraph = null,
    clampNumber = null,
    updateMinimap = null,
    scheduleLodUpdate = null,
    getCanvasInteractionStore = null,
    extractDomEventImpl = null,
  } = {}) {
    function getGraphCenterPoint(graph) {
      if (!graph) return null;
      const w = graph.get ? graph.get("width") : 0;
      const h = graph.get ? graph.get("height") : 0;
      if (typeof graph.getPointByCanvas === "function") {
        try {
          return graph.getPointByCanvas(w / 2, h / 2);
        } catch (e) {}
      }
      if (typeof graph.getPointByClient === "function") {
        try {
          const container = graph.get?.("container");
          const rect = container?.getBoundingClientRect?.();
          if (rect) {
            return graph.getPointByClient(rect.left + w / 2, rect.top + h / 2);
          }
        } catch (e) {}
      }
      return null;
    }

    function resolveGraph(targetGraph = null) {
      return targetGraph || safeCall(ensureGraph) || null;
    }

    function captureViewport(targetGraph = null) {
      const graph = resolveGraph(targetGraph);
      if (!graph) return null;
      const zoom = typeof graph.getZoom === "function" ? graph.getZoom() : 1;
      if (!Number.isFinite(zoom)) return null;
      const width = Number(graph.get ? graph.get("width") : 0) || 0;
      const height = Number(graph.get ? graph.get("height") : 0) || 0;
      const center = getGraphCenterPoint(graph);
      if (center && Number.isFinite(center.x) && Number.isFinite(center.y)) {
        return {
          zoom,
          center: { x: center.x, y: center.y },
          size: width > 0 && height > 0 ? { width, height } : undefined,
        };
      }
      return { zoom, size: width > 0 && height > 0 ? { width, height } : undefined };
    }

    function applyViewport(viewport, targetGraph = null) {
      if (!viewport) return false;
      const graph = resolveGraph(targetGraph);
      if (!graph) return false;
      const zoom = Number(viewport.zoom);
      const hasZoom = Number.isFinite(zoom);
      const center = viewport.center;
      const hasCenter = center && Number.isFinite(center.x) && Number.isFinite(center.y);
      if (!hasZoom && !hasCenter) return false;
      if (hasZoom && typeof graph.zoomTo === "function") {
        const clamped =
          typeof clampNumber === "function" ? clampNumber(zoom, 0.05, 16) : Math.min(16, Math.max(0.05, zoom));
        const w = graph.get ? graph.get("width") : 0;
        const h = graph.get ? graph.get("height") : 0;
        try {
          graph.zoomTo(clamped, { x: (w || 0) / 2, y: (h || 0) / 2 });
        } catch (e) {}
      }
      if (hasCenter) {
        const zoomNow = typeof graph.getZoom === "function" ? graph.getZoom() : 1;
        const cur = getGraphCenterPoint(graph);
        if (cur && Number.isFinite(cur.x) && Number.isFinite(cur.y)) {
          const dx = (cur.x - center.x) * zoomNow;
          const dy = (cur.y - center.y) * zoomNow;
          if (Number.isFinite(dx) && Number.isFinite(dy) && typeof graph.translate === "function") {
            graph.translate(dx, dy, { refreshLabels: true });
          }
        }
      }
      return true;
    }

    function syncViewport(targetGraph = null) {
      const graph = resolveGraph(targetGraph);
      safeCall(updateMinimap);
      if (graph) {
        safeCall(scheduleLodUpdate, graph);
      }
    }

    function resolveClientPointFromEvent(graph, ev) {
      const e = safeCall(extractDomEventImpl, ev) || ev || null;
      if (e && Number.isFinite(e.clientX) && Number.isFinite(e.clientY)) {
        return { x: e.clientX, y: e.clientY };
      }
      const canvas = graph?.get?.("canvas");
      const el = canvas?.get?.("el");
      const rect = el?.getBoundingClientRect?.() || null;
      const cx = Number(ev?.canvasX ?? ev?.x);
      const cy = Number(ev?.canvasY ?? ev?.y);
      if (rect && Number.isFinite(cx) && Number.isFinite(cy)) {
        return { x: rect.left + cx, y: rect.top + cy };
      }
      return { x: window.innerWidth / 2, y: window.innerHeight / 2 };
    }

    function noteHit(kind, item = null, event = null, reason = "") {
      const store = safeCall(getCanvasInteractionStore);
      if (store && typeof store.noteHit === "function") {
        store.noteHit(kind, item, event, reason);
      }
    }

    function getGraphNodeInfo(graph, id) {
      if (!graph || !id) return null;
      const item = graph.findById?.(id);
      if (!item) return null;
      const model = item.getModel?.() || {};
      let client = null;
      try {
        if (typeof graph.getClientByPoint === "function" && Number.isFinite(model.x) && Number.isFinite(model.y)) {
          const pt = graph.getClientByPoint(model.x, model.y);
          if (pt && Number.isFinite(pt.x) && Number.isFinite(pt.y)) {
            client = { x: Math.round(pt.x * 100) / 100, y: Math.round(pt.y * 100) / 100 };
          }
        }
      } catch (e) {}
      return {
        id: model.id || id,
        title: model.title || model.label || "",
        x: Number.isFinite(model.x) ? Math.round(model.x * 100) / 100 : null,
        y: Number.isFinite(model.y) ? Math.round(Number(model.y) * 100) / 100 : null,
        client,
      };
    }

    function getGraphExtremes(graph) {
      if (!graph || typeof graph.getNodes !== "function") return null;
      const nodes = graph.getNodes() || [];
      let left = null;
      let right = null;
      nodes.forEach((node) => {
        const model = node.getModel?.() || {};
        const x = Number(model.x);
        if (!Number.isFinite(x)) return;
        let client = null;
        try {
          if (typeof graph.getClientByPoint === "function" && Number.isFinite(model.y)) {
            const pt = graph.getClientByPoint(model.x, model.y);
            if (pt && Number.isFinite(pt.x) && Number.isFinite(pt.y)) {
              client = { x: Math.round(pt.x * 100) / 100, y: Math.round(pt.y * 100) / 100 };
            }
          }
        } catch (e) {}
        const info = {
          id: model.id || "",
          title: model.title || model.label || "",
          x: Math.round(x * 100) / 100,
          y: Number.isFinite(model.y) ? Math.round(Number(model.y) * 100) / 100 : null,
          client,
        };
        if (!left || x < left.x) left = info;
        if (!right || x > right.x) right = info;
      });
      return { left, right };
    }

    function getGraphEdgeClientPoint(graph, model) {
      if (!graph || !model || typeof graph.getClientByPoint !== "function") return null;
      try {
        const source = graph.findById?.(model.source)?.getModel?.();
        const target = graph.findById?.(model.target)?.getModel?.();
        const sx = Number(source?.x);
        const sy = Number(source?.y);
        const tx = Number(target?.x);
        const ty = Number(target?.y);
        if (![sx, sy, tx, ty].every(Number.isFinite)) return null;
        const point = graph.getClientByPoint((sx + tx) / 2, (sy + ty) / 2);
        if (!point || !Number.isFinite(point.x) || !Number.isFinite(point.y)) return null;
        return {
          x: Math.round(point.x * 100) / 100,
          y: Math.round(point.y * 100) / 100,
        };
      } catch (e) {
        return null;
      }
    }

    return {
      getGraphCenterPoint,
      captureViewport,
      applyViewport,
      syncViewport,
      resolveClientPointFromEvent,
      noteHit,
      getGraphNodeInfo,
      getGraphExtremes,
      getGraphEdgeClientPoint,
    };
  }

  window.__ANALYTIX_FLOW_CANVAS_HIT_VIEWPORT_CONTROLLER__ = {
    createCanvasHitViewportController,
  };
})();
