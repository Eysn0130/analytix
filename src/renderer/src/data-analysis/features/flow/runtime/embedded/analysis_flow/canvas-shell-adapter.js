(() => {
  const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__ || null;
  if (
    !piiProjection ||
    typeof piiProjection.projectDetected !== "function" ||
    typeof piiProjection.projectField !== "function"
  ) {
    throw new Error("ordinary PII projection missing for canvas shell");
  }

  function noop() {}

  function asString(value, fallback = "") {
    const next = String(value == null ? "" : value).trim();
    return next || fallback;
  }

  function asNumber(value, fallback = 0) {
    const next = Number(value);
    return Number.isFinite(next) ? next : fallback;
  }

  function cloneList(value) {
    return Array.isArray(value) ? value.slice() : [];
  }

  function pickFunction(source, key) {
    return source && typeof source[key] === "function" ? source[key] : noop;
  }

  function layoutPresetLabel(preset) {
    const mode = String(preset || "").trim().toLowerCase();
    if (mode === "network") return "网络图布局";
    if (mode === "compact" || mode === "relation") return "关联图布局";
    if (mode === "hierarchy") return "层级图布局";
    if (mode === "flow") return "流向图布局";
    return mode || "布局";
  }

  const FLOW_SUMMARY_BOUNDARY_CODES = Object.freeze({
    unloaded: "no_result_loaded",
    blocked: "publication_blocked",
    unknown: "host_verification_missing",
    partial: "partial_coverage",
  });

  function buildFlowSummaryBoundary(status, layoutLabel = "布局") {
    const normalizedStatus = Object.prototype.hasOwnProperty.call(FLOW_SUMMARY_BOUNDARY_CODES, status)
      ? status
      : "unknown";
    return {
      contract: "FlowGraphSummaryV1",
      status: normalizedStatus,
      factAnswerAllowed: false,
      layoutLabel: asString(layoutLabel, "布局"),
      nodes: null,
      edges: null,
      amount: null,
      boundaryCode: FLOW_SUMMARY_BOUNDARY_CODES[normalizedStatus],
    };
  }

  function projectInternalFlowSummary(value, layoutLabel) {
    const source = value && typeof value === "object" && !Array.isArray(value) ? value : null;
    if (!source || source.contract !== "FlowGraphSummaryV1") return null;
    const status = String(source.status || "").trim();
    if (!Object.prototype.hasOwnProperty.call(FLOW_SUMMARY_BOUNDARY_CODES, status)) return null;
    if (
      source.factAnswerAllowed !== false ||
      source.nodes !== null ||
      source.edges !== null ||
      source.amount !== null ||
      source.boundaryCode !== FLOW_SUMMARY_BOUNDARY_CODES[status]
    ) {
      return null;
    }
    return buildFlowSummaryBoundary(status, source.layoutLabel || layoutLabel);
  }

  function parseAmountWithCurrency(text) {
    const raw = String(text || "");
    if (!raw || !/[￥元]/.test(raw)) {
      return 0;
    }
    const numeric = raw.replace(/[^\d.+-]/g, "");
    const amount = Number(numeric);
    return Number.isFinite(amount) ? amount : 0;
  }

  function edgeAmountTotal(model) {
    const forward = Math.abs(Number(model?.forward_amount) || 0);
    const reverse = Math.abs(Number(model?.reverse_amount) || 0);
    if (forward || reverse) return forward + reverse;
    const outAmt = Math.abs(Number(model?.out_amount) || 0);
    const inAmt = Math.abs(Number(model?.in_amount) || 0);
    if (outAmt || inAmt) return outAmt + inAmt;
    const amount = Math.abs(Number(model?.amount) || 0);
    if (amount) return amount;
    const top = parseAmountWithCurrency(model?.labelTop);
    const bot = parseAmountWithCurrency(model?.labelBottom);
    if (top || bot) return top + bot;
    const labelAmt = parseAmountWithCurrency(model?.label);
    if (labelAmt) return labelAmt;
    return 0;
  }

  function computeGraphTotalAmount(edges) {
    let total = 0;
    (edges || []).forEach((edge) => {
      total += edgeAmountTotal(edge);
    });
    return total;
  }

  function buildGraphStatsSummary({ graphData = null, layoutPreset = "" } = {}) {
    const layoutLabel = layoutPresetLabel(layoutPreset);
    const carriedBoundary = projectInternalFlowSummary(graphData?.flow_summary, layoutLabel);
    if (carriedBoundary) return carriedBoundary;
    if (!graphData) return buildFlowSummaryBoundary("unloaded", layoutLabel);
    const projection =
      graphData?.projection && typeof graphData.projection === "object" && !Array.isArray(graphData.projection)
        ? graphData.projection
        : null;
    if (String(projection?.mode || "").trim().toLowerCase() === "skeleton") {
      return buildFlowSummaryBoundary("partial", layoutLabel);
    }
    return buildFlowSummaryBoundary("unknown", layoutLabel);
  }

  function buildProjectionSummary({ graphData = null, selectedGraphNodeCount = 0 } = {}) {
    const projection =
      graphData?.projection && typeof graphData.projection === "object" && !Array.isArray(graphData.projection)
        ? graphData.projection
        : {};
    const mode = asString(projection.mode, "full").toLowerCase() || "full";
    const snapshotRef =
      graphData?.result_snapshot_ref && typeof graphData.result_snapshot_ref === "object"
        ? graphData.result_snapshot_ref
        : projection?.source_result_snapshot_ref && typeof projection.source_result_snapshot_ref === "object"
        ? projection.source_result_snapshot_ref
        : null;
    const snapshotId = asString(snapshotRef?.snapshot_id || snapshotRef?.graph_hash);
    return {
      mode,
      canExpand: mode === "skeleton" && !!snapshotId,
      selectedNodeCount: Math.max(0, asNumber(selectedGraphNodeCount, 0)),
    };
  }

  function normalizeCanvasViews({ views = [], activeViewId = "" } = {}) {
    return (Array.isArray(views) ? views : []).map((view) => ({
      id: String(view?.id || ""),
      title: piiProjection.projectField("node_id", view?.title || ""),
      saved: !!view?.saved,
      active: String(view?.id || "") === String(activeViewId || ""),
      counts: {
        nodes: Math.max(0, Number(view?.counts?.nodes) || 0),
        edges: Math.max(0, Number(view?.counts?.edges) || 0)
      }
    }));
  }

  function buildCanvasChromeState({
    graphData = null,
    layoutPreset = "",
    graphSearchQuery = "",
    graphSearchFocusToken = 0,
    selectedGraphNodeCount = 0,
    views = [],
    activeViewId = ""
  } = {}) {
    return {
      graphSearch: {
        query: piiProjection.projectField("node_id", graphSearchQuery || ""),
        focusToken: Math.max(0, Number(graphSearchFocusToken) || 0)
      },
      projection: buildProjectionSummary({ graphData, selectedGraphNodeCount }),
      graphStats: buildGraphStatsSummary({ graphData, layoutPreset }),
      views: normalizeCanvasViews({ views, activeViewId })
    };
  }

  function buildCanvasShellState({
    caseId = "",
    caseName = "",
    leftCollapsed = false,
    tab = "byName",
    search = "",
    graphSearch = null,
    projection = null,
    treeData = [],
    expandedIds = [],
    selectedIds = [],
    selectedCount = 0,
    filters = null,
    layout = null,
    analysis = null,
    graphStats = null,
    ops = null,
    style = null,
    views = [],
    activeViewId = "",
    overlay = null,
  } = {}) {
    const nextGraphSearch =
      graphSearch && typeof graphSearch === "object"
        ? {
            query: piiProjection.projectField("node_id", graphSearch.query),
            focusToken: Math.max(0, asNumber(graphSearch.focusToken, 0)),
          }
        : {
            query: "",
            focusToken: 0,
          };
    const nextGraphStats =
      projectInternalFlowSummary(graphStats, graphStats?.layoutLabel || "布局") ||
      buildFlowSummaryBoundary("unknown", graphStats?.layoutLabel || "布局");
    return {
      caseId: asString(caseId),
      caseName: piiProjection.projectDetected(caseName),
      leftCollapsed: !!leftCollapsed,
      tab: asString(tab) === "byCard" ? "byCard" : "byName",
      search: piiProjection.projectField("node_id", search),
      graphSearch: nextGraphSearch,
      projection:
        projection && typeof projection === "object"
          ? {
              mode: asString(projection.mode, "full").toLowerCase() || "full",
              canExpand: !!projection.canExpand,
              selectedNodeCount: Math.max(0, asNumber(projection.selectedNodeCount, 0)),
            }
          : {
              mode: "full",
              canExpand: false,
              selectedNodeCount: 0,
            },
      treeData: cloneList(treeData),
      expandedIds: cloneList(expandedIds),
      selectedIds: cloneList(selectedIds),
      selectedCount: Math.max(0, asNumber(selectedCount, 0)),
      filters: filters && typeof filters === "object" ? { ...filters } : {},
      layout: layout && typeof layout === "object" ? { ...layout } : {},
      analysis: analysis && typeof analysis === "object" ? { ...analysis } : {},
      graphStats: nextGraphStats,
      ops: ops && typeof ops === "object" ? { ...ops } : {},
      style: style && typeof style === "object" ? { ...style } : {},
      views: normalizeCanvasViews({ views, activeViewId }),
      activeViewId: asString(activeViewId),
      overlay: overlay && typeof overlay === "object" ? { ...overlay } : {},
    };
  }

  function buildCanvasCommandRegistry(definitions = {}) {
    const style = definitions.style || {};
    const layout = definitions.layout || {};
    const analysis = definitions.analysis || {};
    const ops = definitions.ops || {};
    const graph = definitions.graph || {};
    const canvas = definitions.canvas || {};
    const overlay = definitions.overlay || {};

    return {
      triggerControl: pickFunction(definitions, "triggerControl"),
      style: {
        openFontFamily: pickFunction(style, "openFontFamily"),
        openFontSize: pickFunction(style, "openFontSize"),
        closePopover: pickFunction(style, "closePopover"),
        applyPopoverOption: pickFunction(style, "applyPopoverOption"),
        applyPopoverColor: pickFunction(style, "applyPopoverColor"),
        setIconLibraryTab: pickFunction(style, "setIconLibraryTab"),
        setIconLibraryQuery: pickFunction(style, "setIconLibraryQuery"),
        applyIconSymbol: pickFunction(style, "applyIconSymbol"),
        increaseFontSize: pickFunction(style, "increaseFontSize"),
        decreaseFontSize: pickFunction(style, "decreaseFontSize"),
        toggleBold: pickFunction(style, "toggleBold"),
        toggleItalic: pickFunction(style, "toggleItalic"),
        toggleUnderline: pickFunction(style, "toggleUnderline"),
        toggleShadow: pickFunction(style, "toggleShadow"),
        openTextColor: pickFunction(style, "openTextColor"),
        openOutlineColor: pickFunction(style, "openOutlineColor"),
        openLineStyle: pickFunction(style, "openLineStyle"),
        openLineWidth: pickFunction(style, "openLineWidth"),
        openLineArrow: pickFunction(style, "openLineArrow"),
        openNodeShape: pickFunction(style, "openNodeShape"),
        openNodeFill: pickFunction(style, "openNodeFill"),
        openIconSize: pickFunction(style, "openIconSize"),
        toggleCreateNodeMode: pickFunction(style, "toggleCreateNodeMode"),
        openIconLibrary: pickFunction(style, "openIconLibrary"),
        createLinkFromSelection: pickFunction(style, "createLinkFromSelection"),
      },
      layout: {
        setPreset: pickFunction(layout, "setPreset"),
        openEdgeRoutingMenu: pickFunction(layout, "openEdgeRoutingMenu"),
      },
      analysis: {
        openFilter: pickFunction(analysis, "openFilter"),
        closeFilter: pickFunction(analysis, "closeFilter"),
        setFilterMin: pickFunction(analysis, "setFilterMin"),
        setFilterMax: pickFunction(analysis, "setFilterMax"),
        clearFilter: pickFunction(analysis, "clearFilter"),
        toggleCollapseChildren: pickFunction(analysis, "toggleCollapseChildren"),
        assignGroupTag: pickFunction(analysis, "assignGroupTag"),
        openGroupOps: pickFunction(analysis, "openGroupOps"),
        toggleEncodeWidth: pickFunction(analysis, "toggleEncodeWidth"),
        toggleEncodeColor: pickFunction(analysis, "toggleEncodeColor"),
        toggleNodeScale: pickFunction(analysis, "toggleNodeScale"),
      },
      ops: {
        mergeNodes: pickFunction(ops, "mergeNodes"),
        mergeNodesNet: pickFunction(ops, "mergeNodesNet"),
        openMergeMenu: pickFunction(ops, "openMergeMenu"),
        fitGraph: pickFunction(ops, "fitGraph"),
        openExport: pickFunction(ops, "openExport"),
        closeExport: pickFunction(ops, "closeExport"),
        setExportScale: pickFunction(ops, "setExportScale"),
        exportFormat: pickFunction(ops, "exportFormat"),
        toggleEdgeDetail: pickFunction(ops, "toggleEdgeDetail"),
        redo: pickFunction(ops, "redo"),
        undo: pickFunction(ops, "undo"),
      },
      graph: {
        setSearch: pickFunction(graph, "setSearch"),
        clearSearch: pickFunction(graph, "clearSearch"),
        runSearch: pickFunction(graph, "runSearch"),
        expandViewport: pickFunction(graph, "expandViewport"),
        expandSelectionPath: pickFunction(graph, "expandSelectionPath"),
      },
      canvas: {
        clearSelection: pickFunction(canvas, "clearSelection"),
        selectAll: pickFunction(canvas, "selectAll"),
        selectAdjacent: pickFunction(canvas, "selectAdjacent"),
        fitToSelection: pickFunction(canvas, "fitToSelection"),
        captureViewport: pickFunction(canvas, "captureViewport"),
        restoreViewport: pickFunction(canvas, "restoreViewport"),
        focusItem: pickFunction(canvas, "focusItem"),
        zoomIn: pickFunction(canvas, "zoomIn"),
        zoomOut: pickFunction(canvas, "zoomOut"),
        resetViewport: pickFunction(canvas, "resetViewport"),
      },
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
      overlay: {
        runContextMenuAction: pickFunction(overlay, "runContextMenuAction"),
        closeContextMenu: pickFunction(overlay, "closeContextMenu"),
        closeDrawer: pickFunction(overlay, "closeDrawer"),
        runDrawerAction: pickFunction(overlay, "runDrawerAction"),
        closeNodeInfo: pickFunction(overlay, "closeNodeInfo"),
        closeEdgeInfo: pickFunction(overlay, "closeEdgeInfo"),
        closeEdgeLabel: pickFunction(overlay, "closeEdgeLabel"),
        setEdgeLabelValue: pickFunction(overlay, "setEdgeLabelValue"),
        saveEdgeLabel: pickFunction(overlay, "saveEdgeLabel"),
        openEdgeTxnDetail: pickFunction(overlay, "openEdgeTxnDetail"),
        closeTxnModal: pickFunction(overlay, "closeTxnModal"),
        toggleTxnSort: pickFunction(overlay, "toggleTxnSort"),
        setTxnColumnWidth: pickFunction(overlay, "setTxnColumnWidth"),
      },
    };
  }

  window.__ANALYTIX_FLOW_CANVAS_ADAPTER__ = {
    layoutPresetLabel,
    edgeAmountTotal,
    computeGraphTotalAmount,
    buildGraphStatsSummary,
    buildProjectionSummary,
    normalizeCanvasViews,
    buildCanvasChromeState,
    buildCanvasShellState,
    buildCanvasCommandRegistry
  };
})();
