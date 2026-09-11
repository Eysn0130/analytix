(() => {
  function noop() {}

  function cloneSection(value) {
    return value && typeof value === "object" ? { ...value } : {};
  }

  function pickFunction(source, key) {
    return source && typeof source[key] === "function" ? source[key] : noop;
  }

  function finalizeCanvasCommandRegistry(definitions = {}, shellAdapter = null) {
    const adapter =
      shellAdapter && typeof shellAdapter.buildCanvasCommandRegistry === "function"
        ? shellAdapter
        : window.__ANALYTIX_FLOW_CANVAS_ADAPTER__ || null;
    if (adapter && typeof adapter.buildCanvasCommandRegistry === "function") {
      return adapter.buildCanvasCommandRegistry(definitions || {});
    }
    return {
      ...cloneSection(definitions),
      style: cloneSection(definitions.style),
      layout: cloneSection(definitions.layout),
      analysis: cloneSection(definitions.analysis),
      ops: cloneSection(definitions.ops),
      graph: cloneSection(definitions.graph),
      canvas: {
        clearSelection: pickFunction(definitions.canvas, "clearSelection"),
        selectAll: pickFunction(definitions.canvas, "selectAll"),
        selectAdjacent: pickFunction(definitions.canvas, "selectAdjacent"),
        fitToSelection: pickFunction(definitions.canvas, "fitToSelection"),
        captureViewport: pickFunction(definitions.canvas, "captureViewport"),
        restoreViewport: pickFunction(definitions.canvas, "restoreViewport"),
        focusItem: pickFunction(definitions.canvas, "focusItem"),
        zoomIn: pickFunction(definitions.canvas, "zoomIn"),
        zoomOut: pickFunction(definitions.canvas, "zoomOut"),
        resetViewport: pickFunction(definitions.canvas, "resetViewport"),
      },
      overlay: cloneSection(definitions.overlay),
      triggerControl: pickFunction(definitions, "triggerControl"),
      setLeftCollapsed: pickFunction(definitions, "setLeftCollapsed"),
      toggleLeftCollapsed: pickFunction(definitions, "toggleLeftCollapsed"),
      setTab: pickFunction(definitions, "setTab"),
      setSearch: pickFunction(definitions, "setSearch"),
      toggleGroup: pickFunction(definitions, "toggleGroup"),
      toggleGroupSelect: pickFunction(definitions, "toggleGroupSelect"),
      toggleItemSelect: pickFunction(definitions, "toggleItemSelect"),
      selectAll: pickFunction(definitions, "selectAll"),
      clearSelection: pickFunction(definitions, "clearSelection"),
      setDir: pickFunction(definitions, "setDir"),
      setHop: pickFunction(definitions, "setHop"),
      setMinAmount: pickFunction(definitions, "setMinAmount"),
      setMaxEdges: pickFunction(definitions, "setMaxEdges"),
      buildGraph: pickFunction(definitions, "buildGraph"),
      clearGraph: pickFunction(definitions, "clearGraph"),
      addView: pickFunction(definitions, "addView"),
      activateView: pickFunction(definitions, "activateView"),
      saveCurrentView: pickFunction(definitions, "saveCurrentView"),
      closeView: pickFunction(definitions, "closeView"),
      renameView: pickFunction(definitions, "renameView"),
      reorderViews: pickFunction(definitions, "reorderViews"),
      openViewContextMenu: pickFunction(definitions, "openViewContextMenu"),
      fitGraph: pickFunction(definitions, "fitGraph"),
      refit: pickFunction(definitions, "refit"),
    };
  }

  window.__ANALYTIX_FLOW_CANVAS_COMMAND_ADAPTER__ = {
    finalizeCanvasCommandRegistry,
  };
})();
