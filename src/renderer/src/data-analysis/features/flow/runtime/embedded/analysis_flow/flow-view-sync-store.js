(() => {
  const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__ || null;
  if (!piiProjection || typeof piiProjection.projectField !== "function") {
    throw new Error("ordinary PII projection missing for flow view sync");
  }

  function createFlowViewSyncStore({ deps = {} } = {}) {
    const getActiveView = typeof deps.getActiveView === "function" ? deps.getActiveView : () => null;
    const snapshotGraphData = typeof deps.snapshotGraphData === "function" ? deps.snapshotGraphData : () => ({ nodes: [], edges: [] });
    const captureGraphViewport =
      typeof deps.captureGraphViewport === "function" ? deps.captureGraphViewport : () => null;
    const nowIso = typeof deps.nowIso === "function" ? deps.nowIso : () => new Date().toISOString();
    const syncAnalysisSourceDataFromCurrent =
      typeof deps.syncAnalysisSourceDataFromCurrent === "function" ? deps.syncAnalysisSourceDataFromCurrent : () => {};
    const viewCounts = typeof deps.viewCounts === "function" ? deps.viewCounts : () => ({ nodes: 0, edges: 0 });
    const query = typeof deps.query === "function" ? deps.query : () => null;

    function updateCurrentViewSnapshot() {
      const view = getActiveView();
      if (!view) return;
      const snapshot = snapshotGraphData();
      view.graph = snapshot;
      view.counts = { nodes: snapshot.nodes.length, edges: snapshot.edges.length };
      const viewport = captureGraphViewport();
      if (viewport) view.viewport = viewport;
      view.updatedAt = nowIso();
      syncAnalysisSourceDataFromCurrent();
    }

    function updateViewCard(view = getActiveView()) {
      const titleEl = query("#viewTitle");
      const countsEl = query("#viewCounts");
      if (!titleEl || !countsEl) return;
      if (!view) {
        titleEl.textContent = "视图";
        countsEl.textContent = "节点 0 · 边 0";
        return;
      }
      const counts = viewCounts(view);
      titleEl.textContent = piiProjection.projectField("node_id", view.title || "视图");
      countsEl.textContent = `节点 ${counts.nodes} · 边 ${counts.edges}`;
    }

    return {
      updateCurrentViewSnapshot,
      updateViewCard,
    };
  }

  window.__ANALYTIX_FLOW_VIEW_SYNC_STORE__ = {
    createFlowViewSyncStore,
  };
})();
