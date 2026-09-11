(() => {
  function createFlowSelectionStore({ state = null, deps = {} } = {}) {
    const getSelectedGraphItems =
      typeof deps.getSelectedGraphItems === "function" ? deps.getSelectedGraphItems : () => ({ nodes: [], edges: [] });
    const clearCanvasSelection = typeof deps.clearCanvasSelection === "function" ? deps.clearCanvasSelection : () => {};
    const startPulseLoop = typeof deps.startPulseLoop === "function" ? deps.startPulseLoop : () => {};
    const stopPulseLoop = typeof deps.stopPulseLoop === "function" ? deps.stopPulseLoop : () => {};
    const scheduleShellStateSync =
      typeof deps.scheduleShellStateSync === "function" ? deps.scheduleShellStateSync : () => {};

    function getSelectionState(graph) {
      const { nodes, edges } = getSelectedGraphItems(graph);
      return { nodes, edges, total: nodes.length + edges.length };
    }

    function clearGraphSelection(graph) {
      if (!graph) return;
      clearCanvasSelection(graph, "clear-graph-selection");
    }

    function syncActiveSelection(graph, candidate) {
      if (!graph) return;
      const { nodes } = getSelectionState(graph);
      const enablePulse = nodes.length > 0 && nodes.length <= 100;
      const activeNodes = graph.findAllByState?.("node", "active") || [];
      activeNodes.forEach((node) => {
        if (!node.hasState?.("selected")) graph.setItemState(node, "active", false);
      });
      if (!enablePulse) {
        nodes.forEach((node) => graph.setItemState(node, "active", false));
        if (state) state.lastActiveId = "";
        stopPulseLoop();
        scheduleShellStateSync("graph-selection");
        return;
      }
      nodes.forEach((node) => graph.setItemState(node, "active", true));
      let next = null;
      if (candidate && candidate.hasState?.("selected") && candidate.getType?.() === "node") next = candidate;
      if (!next && nodes.length) next = nodes[nodes.length - 1];
      if (state) {
        state.lastActiveId = next ? next.getID?.() || next.getModel?.()?.id || "" : "";
      }
      startPulseLoop();
      scheduleShellStateSync("graph-selection");
    }

    return {
      getSelectionState,
      clearGraphSelection,
      syncActiveSelection,
    };
  }

  window.__ANALYTIX_FLOW_SELECTION_STORE__ = {
    createFlowSelectionStore,
  };
})();
