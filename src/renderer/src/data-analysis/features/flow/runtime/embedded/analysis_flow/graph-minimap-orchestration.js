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

  function createFlowMinimapStore(config = {}) {
    const state = config.state || {};
    const selectMinimapEl =
      typeof config.selectMinimapEl === "function" ? config.selectMinimapEl : () => null;
    const getGraph = typeof config.getGraph === "function" ? config.getGraph : () => state.graph?.instance || null;
    const getNodes = typeof config.getNodes === "function" ? config.getNodes : () => state.graph?.data?.nodes || [];
    const getGraphDataBounds =
      typeof config.getGraphDataBounds === "function" ? config.getGraphDataBounds : () => null;
    const clampNumber = typeof config.clampNumber === "function" ? config.clampNumber : (value) => Number(value) || 0;
    const applyGraphModes = typeof config.applyGraphModes === "function" ? config.applyGraphModes : () => {};
    const log = typeof config.log === "function" ? config.log : () => {};
    const nodeThreshold = Math.max(1, Number(config.nodeThreshold) || 300);
    const edgeThreshold = Math.max(1, Number(config.edgeThreshold) || 800);
    const minimapFactory =
      root.AnalytixMinimap && typeof root.AnalytixMinimap.createController === "function"
        ? root.AnalytixMinimap.createController
        : null;

    const minimapController = minimapFactory
      ? minimapFactory({
          selectMinimapEl,
          getGraph,
          getNodes,
          getGraphDataBounds,
          clampNumber,
        })
      : null;

    function init(graph) {
      minimapController?.init?.(graph || getGraph());
    }

    function destroy() {
      minimapController?.destroy?.();
    }

    function update(options = {}) {
      minimapController?.update?.(options);
    }

    function updatePerfMode(nodesLen, edgesLen) {
      const next = Number(nodesLen) >= nodeThreshold || Number(edgesLen) >= edgeThreshold;
      if (state.perfMode === next) return next;
      state.perfMode = next;
      try {
        log("INFO", "perf mode changed", {
          perf: next,
          nodes: Number(nodesLen) || 0,
          edges: Number(edgesLen) || 0,
          thresholds: {
            nodes: nodeThreshold,
            edges: edgeThreshold,
          },
        });
      } catch (e) {}
      const mmEl = selectMinimapEl();
      if (mmEl) mmEl.style.display = "";
      const graph = getGraph();
      if (graph) {
        applyGraphModes(graph, next);
        init(graph);
      }
      update();
      return next;
    }

    return {
      init,
      destroy,
      update,
      updatePerfMode,
    };
  }

  root.__ANALYTIX_GRAPH_MINIMAP_ORCHESTRATION__ = {
    createFlowMinimapStore,
  };
})();
