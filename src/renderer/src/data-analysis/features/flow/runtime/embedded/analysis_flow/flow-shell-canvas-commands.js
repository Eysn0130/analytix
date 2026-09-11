(() => {
  function noop() {}

  function createFlowShellCanvasCommands({ deps = {} } = {}) {
    const ensureGraph = typeof deps.ensureGraph === "function" ? deps.ensureGraph : () => null;
    const clearCanvasSelection =
      typeof deps.clearCanvasSelection === "function" ? deps.clearCanvasSelection : noop;
    const selectAllGraphItems =
      typeof deps.selectAllGraphItems === "function" ? deps.selectAllGraphItems : noop;
    const selectAdjacentGraphNodes =
      typeof deps.selectAdjacentGraphNodes === "function" ? deps.selectAdjacentGraphNodes : noop;
    const fitGraph = typeof deps.fitGraph === "function" ? deps.fitGraph : noop;
    const captureCanvasViewport =
      typeof deps.captureCanvasViewport === "function" ? deps.captureCanvasViewport : () => null;
    const applyCanvasViewport =
      typeof deps.applyCanvasViewport === "function" ? deps.applyCanvasViewport : () => false;
    const focusGraphItemById =
      typeof deps.focusGraphItemById === "function" ? deps.focusGraphItemById : noop;
    const clampNumber = typeof deps.clampNumber === "function" ? deps.clampNumber : (value) => Number(value) || 0;

    return {
      canvas: {
        clearSelection: () => {
          clearCanvasSelection(ensureGraph(), "react-canvas-clear-selection");
        },
        selectAll: () => {
          selectAllGraphItems();
        },
        selectAdjacent: (anchorId, options = {}) => {
          selectAdjacentGraphNodes(anchorId, options || {});
        },
        fitToSelection: () => {
          fitGraph({ force: true });
        },
        captureViewport: () => captureCanvasViewport(ensureGraph(), "react-canvas-capture-viewport"),
        restoreViewport: (viewport) =>
          !!applyCanvasViewport(viewport, ensureGraph(), { reason: "react-canvas-restore-viewport" }),
        focusItem: (itemId) => focusGraphItemById(itemId, { animate: true, pulse: true }),
        zoomIn: () => {
          const graph = ensureGraph();
          if (!graph || typeof graph.getZoom !== "function" || typeof graph.zoomTo !== "function") return;
          const current = graph.getZoom() || 1;
          graph.zoomTo(clampNumber(current * 1.1, 0.05, 16));
        },
        zoomOut: () => {
          const graph = ensureGraph();
          if (!graph || typeof graph.getZoom !== "function" || typeof graph.zoomTo !== "function") return;
          const current = graph.getZoom() || 1;
          graph.zoomTo(clampNumber(current / 1.1, 0.05, 16));
        },
        resetViewport: () => {
          fitGraph({ force: true });
        },
      },
    };
  }

  window.__ANALYTIX_FLOW_SHELL_CANVAS_COMMANDS__ = {
    createFlowShellCanvasCommands,
  };
})();
