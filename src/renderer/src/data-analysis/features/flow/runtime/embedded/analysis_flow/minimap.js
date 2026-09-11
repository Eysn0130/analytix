(() => {
  const root =
    typeof globalThis !== "undefined"
      ? globalThis
      : typeof self !== "undefined"
        ? self
        : typeof window !== "undefined"
          ? window
          : null;
  if (!root) return;

  function getGraphEngine() {
    return root.AnalytixGraphEngine || null;
  }

  function fallbackClampNumber(value, min, max) {
    const num = Number(value);
    if (!Number.isFinite(num)) return min;
    return Math.max(min, Math.min(max, num));
  }

  function animateGraphPan(graph, dx, dy, duration = 320) {
    if (!graph || (!dx && !dy)) return;
    const start = performance.now();
    let lastX = 0;
    let lastY = 0;
    let lastRefreshAt = -Infinity;
    const refreshIntervalMs = 90;
    const easeInOut = (t) => (t < 0.5 ? 2 * t * t : -1 + (4 - 2 * t) * t);
    const tick = (now) => {
      const t = Math.min(1, (now - start) / duration);
      const eased = easeInOut(t);
      const curX = dx * eased;
      const curY = dy * eased;
      const shouldRefreshLabels = t >= 1 || now - lastRefreshAt >= refreshIntervalMs;
      graph.translate(curX - lastX, curY - lastY, { refreshLabels: shouldRefreshLabels });
      if (shouldRefreshLabels) lastRefreshAt = now;
      lastX = curX;
      lastY = curY;
      if (t < 1) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  }

  function panGraphToPoint(graph, point, duration = 320) {
    if (!graph || !point) return;
    const w = graph.get("width") || 0;
    const h = graph.get("height") || 0;
    const zoom = graph.getZoom ? graph.getZoom() : 1;
    const center =
      typeof graph.getPointByCanvas === "function"
        ? graph.getPointByCanvas(w / 2, h / 2)
        : { x: 0, y: 0 };
    const dx = (center.x - point.x) * zoom;
    const dy = (center.y - point.y) * zoom;
    animateGraphPan(graph, dx, dy, duration);
  }

  function createController(config = {}) {
    const selectMinimapEl =
      typeof config.selectMinimapEl === "function" ? config.selectMinimapEl : () => null;
    const getGraph = typeof config.getGraph === "function" ? config.getGraph : () => null;
    const getNodes = typeof config.getNodes === "function" ? config.getNodes : () => [];
    const getGraphDataBounds =
      typeof config.getGraphDataBounds === "function" ? config.getGraphDataBounds : () => null;
    const clampNumber =
      typeof config.clampNumber === "function" ? config.clampNumber : fallbackClampNumber;

    let minimap = null;
    let boundEl = null;

    const onMinimapClick = (e) => {
      e.preventDefault();
      e.stopPropagation();
      const graph = getGraph();
      if (!graph) return;
      const bounds = minimap?.getBounds?.() || getGraphDataBounds(getNodes());
      if (!bounds) return;
      const mmEl = boundEl;
      if (!mmEl) return;
      const rect = mmEl.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      const x = e.clientX - rect.left;
      const y = e.clientY - rect.top;
      const spanX = Math.max(1, bounds.maxX - bounds.minX);
      const spanY = Math.max(1, bounds.maxY - bounds.minY);
      const scale = Math.min(rect.width / spanX, rect.height / spanY);
      const drawW = spanX * scale;
      const drawH = spanY * scale;
      const padX = (rect.width - drawW) / 2;
      const padY = (rect.height - drawH) / 2;
      const localX = clampNumber(x - padX, 0, drawW);
      const localY = clampNumber(y - padY, 0, drawH);
      const gx = bounds.minX + localX / scale;
      const gy = bounds.minY + localY / scale;
      panGraphToPoint(graph, { x: gx, y: gy }, 320);
    };

    const onMinimapWheel = (e) => {
      e.preventDefault();
      const graph = getGraph();
      if (!graph) return;
      const z = graph.getZoom ? graph.getZoom() : 1;
      const delta = e.deltaY;
      const next = clampNumber(z * (delta < 0 ? 1.08 : 0.92), 0.05, 16);
      if (graph.zoomTo) graph.zoomTo(next, { x: graph.get("width") / 2, y: graph.get("height") / 2 });
      update();
    };

    function bindEvents(mmEl) {
      if (!mmEl) return;
      if (boundEl === mmEl) return;
      unbindEvents();
      boundEl = mmEl;
      mmEl.addEventListener("click", onMinimapClick);
      mmEl.addEventListener("wheel", onMinimapWheel, { passive: false });
    }

    function unbindEvents() {
      if (!boundEl) return;
      boundEl.removeEventListener("click", onMinimapClick);
      boundEl.removeEventListener("wheel", onMinimapWheel);
      boundEl = null;
    }

    function init(graph) {
      const targetGraph = graph || getGraph();
      if (!targetGraph) return;
      const mmEl = selectMinimapEl();
      if (!mmEl) return;
      mmEl.style.display = "";
      const graphEngine = getGraphEngine();
      if (!minimap && graphEngine && typeof graphEngine.Minimap === "function") {
        const mmW = mmEl.clientWidth || 220;
        const mmH = mmEl.clientHeight || 140;
        minimap = new graphEngine.Minimap({
          container: mmEl,
          size: [mmW, mmH],
          type: "delegate",
          contentPaddingRatio: 0.16,
          viewportMarginFactor: 1.18,
          maxNodeSamples: 1800,
          maxEdgeSamples: 2400,
          delegateStyle: {
            fill: "rgba(31,111,235,0.10)",
            stroke: "rgba(31,111,235,0.85)",
            lineWidth: 2,
            radius: 8,
          },
        });
        targetGraph.addPlugin?.(minimap);
      }
      bindEvents(mmEl);
    }

    function update(options = {}) {
      if (!minimap) return;
      let { canvas = true, viewport = true } = options || {};
      if (!canvas && viewport && !minimap?.getBounds?.()) canvas = true;
      try {
        if (canvas) minimap.updateCanvas?.();
        if (viewport) minimap.updateViewport?.();
      } catch (e) {}
    }

    function destroy() {
      unbindEvents();
      if (minimap) {
        try {
          minimap.destroy?.();
        } catch (e) {}
      }
      minimap = null;
    }

    return {
      init,
      update,
      destroy,
    };
  }

  root.AnalytixMinimap = {
    ...(root.AnalytixMinimap || {}),
    createController,
  };
})();
