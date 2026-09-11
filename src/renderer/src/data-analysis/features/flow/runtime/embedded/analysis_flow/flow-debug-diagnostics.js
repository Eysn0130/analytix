(() => {
  function safeCall(fn, ...args) {
    if (typeof fn !== "function") return undefined;
    try {
      return fn(...args);
    } catch (e) {
      return undefined;
    }
  }

  function createFlowDebugDiagnostics({
    state = null,
    log = null,
    getGraphEngine = null,
    detectWebGLSupport = null,
    shouldLogCanvasMetrics = null,
    summarizeGraphData = null,
    toFiniteNumber = null,
    computeGraphTotalAmount = null,
  } = {}) {
    let lastCanvasSnapshotSignature = "";

    function logGpuProbe(graph) {
      if (state?.gpuLogged) return;
      if (state) state.gpuLogged = true;
      const support = safeCall(detectWebGLSupport) || { webgl: false, webgl2: false };
      const renderer =
        (graph && typeof graph.get === "function" && graph.get("renderer")) ||
        (graph && typeof graph.getRenderer === "function" && graph.getRenderer()) ||
        "webgl";
      safeCall(log, "INFO", "gpu probe", {
        renderer,
        webgl: !!support.webgl,
        webgl2: !!support.webgl2,
        webgpu: !!navigator.gpu,
        engineVersion: safeCall(getGraphEngine)?.version || "",
      });
    }

    function logGraphDebug(graph, message, extra = {}) {
      if (!graph || typeof graph.getDebugInfo !== "function") return;
      try {
        const info = graph.getDebugInfo();
        const dataNodes = Number(extra?.nodes || 0);
        const dataEdges = Number(extra?.edges || 0);
        const engineNodes = Number(info?.counts?.nodes || 0);
        const engineEdges = Number(info?.counts?.edges || 0);
        const mismatch = (dataNodes > 0 && engineNodes === 0) || (dataEdges > 0 && engineEdges === 0);
        if (!state?.debugTrace && !mismatch) return;
        safeCall(log, "INFO", message, { ...extra, ...info });
      } catch (e) {}
    }

    function emitCanvasSnapshotLog(reason, graph = null) {
      if (!safeCall(shouldLogCanvasMetrics)) return;
      const dataNodes = Array.isArray(state?.graph?.data?.nodes) ? state.graph.data.nodes : [];
      const dataEdges = Array.isArray(state?.graph?.data?.edges) ? state.graph.data.edges : [];
      const runtimeGraph = graph || state?.graph?.instance || null;
      const runtimeNodes = runtimeGraph && typeof runtimeGraph.getNodes === "function" ? runtimeGraph.getNodes().length : 0;
      const runtimeEdges = runtimeGraph && typeof runtimeGraph.getEdges === "function" ? runtimeGraph.getEdges().length : 0;
      const amount = safeCall(toFiniteNumber, safeCall(computeGraphTotalAmount, dataEdges), 2);
      const summary = safeCall(summarizeGraphData, dataNodes, dataEdges, 6) || null;
      const signature = [
        state?.requestId || "",
        String(reason || ""),
        dataNodes.length,
        dataEdges.length,
        runtimeNodes,
        runtimeEdges,
        amount,
      ].join("|");
      if (signature === lastCanvasSnapshotSignature) return;
      lastCanvasSnapshotSignature = signature;
      safeCall(log, "INFO", "viz canvas snapshot", {
        requestId: state?.requestId || "",
        reason: String(reason || ""),
        counts: {
          dataNodes: dataNodes.length,
          dataEdges: dataEdges.length,
          runtimeNodes,
          runtimeEdges,
        },
        amount,
        renderer:
          (runtimeGraph && typeof runtimeGraph.getRenderer === "function" && runtimeGraph.getRenderer()) ||
          (runtimeGraph && typeof runtimeGraph.get === "function" && runtimeGraph.get("renderer")) ||
          "",
        summary,
      });
    }

    return {
      logGpuProbe,
      logGraphDebug,
      emitCanvasSnapshotLog,
    };
  }

  window.__ANALYTIX_FLOW_DEBUG_DIAGNOSTICS__ = {
    createFlowDebugDiagnostics,
  };
})();
