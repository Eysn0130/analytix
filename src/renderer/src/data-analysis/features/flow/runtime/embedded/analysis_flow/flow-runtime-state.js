(() => {
  const piiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__ || null;
  if (!piiProjection || typeof piiProjection.projectDetected !== "function") {
    throw new Error("ordinary PII projection missing for flow runtime state");
  }

  function asText(value, fallback = "") {
    const next = String(value == null ? "" : value).trim();
    return next || fallback;
  }

  function asCount(value, fallback = 0) {
    const next = Number(value);
    return Number.isFinite(next) ? Math.max(0, Math.round(next)) : Math.max(0, Number(fallback) || 0);
  }

  function invokeOrFallback(fn, fallback) {
    if (typeof fn !== "function") return fallback;
    try {
      return fn();
    } catch {
      return fallback;
    }
  }

  function createReactOverlayState() {
    return {
      suppressCloseUntil: 0,
      filter: {
        open: false,
        anchorRect: null,
      },
      stylePopover: {
        open: false,
        anchorRect: null,
        sourceKey: "",
      },
      menu: {
        open: false,
        anchorRect: null,
        sourceKey: "",
        items: [],
      },
      exportPanel: {
        open: false,
        anchorRect: null,
      },
      contextMenu: {
        open: false,
        x: 0,
        y: 0,
        items: [],
      },
      drawer: {
        open: false,
        kind: "",
        title: "",
        rows: [],
        actions: [],
        meta: null,
      },
      nodeInfo: {
        open: false,
        point: null,
        preview: null,
        category: "",
        userName: "",
        accountLabel: "",
        accountValue: "",
        accountValueMono: false,
        accounts: [],
        moreCount: 0,
      },
      edgeInfo: {
        open: false,
        point: null,
        requestKey: "",
        title: "资金交易",
        amountText: "",
        amountTone: "",
        countText: "0",
        countClickable: false,
        loading: false,
        error: "",
        rows: [],
      },
      edgeLabel: {
        open: false,
        editable: true,
        title: "编辑连线文本",
        value: "",
        placeholder: "输入文本内容…",
        hint: "双击线条可再次编辑",
        confirmLabel: "确定",
        cancelLabel: "取消",
        readonly: {
          open: false,
          title: "资金交易",
          amountText: "",
          amountTone: "",
          countText: "0",
          countClickable: false,
          loading: false,
          error: "",
          rows: [],
        },
      },
    };
  }

  function createPointLayerOverlayState() {
    return {
      canvas: null,
      ctx: null,
      raf: 0,
      width: 0,
      height: 0,
      dpr: 1,
    };
  }

  function createProjectionAutoMaterializeState() {
    return {
      timer: 0,
      inflight: false,
      inflightSignature: "",
      lastSignature: "",
      lastAt: 0,
    };
  }

  function createProjectionLayoutSyncState() {
    return {
      timer: 0,
      inflight: false,
      inflightSignature: "",
      lastSignature: "",
      lastSnapshotId: "",
      lastAt: 0,
    };
  }

  function createTxnModalState() {
    return {
      open: false,
      title: "交易明细",
      model: null,
      lane: "",
      requestKey: "",
      rows: [],
      loading: false,
      error: "",
      sortCol: "txn_time",
      sortDir: "asc",
      expectedTotal: 0,
      reqId: 0,
    };
  }

  function createDrillConfigState() {
    return {
      policy: "auto",
      windowDays: 7,
    };
  }

  function buildProjectionShellState(state, deps = {}) {
    const canvasShellAdapter = deps.canvasShellAdapter || null;
    const graphData = state?.graph?.data || null;
    const graph = state?.graph?.instance || null;
    const selectionState = invokeOrFallback(
      () => (typeof deps.getSelectionState === "function" ? deps.getSelectionState(graph) : { nodes: [] }),
      { nodes: [] }
    );
    const selectedNodeCount = Array.isArray(selectionState?.nodes) ? selectionState.nodes.length : 0;
    if (canvasShellAdapter && typeof canvasShellAdapter.buildProjectionSummary === "function") {
      return canvasShellAdapter.buildProjectionSummary({
        graphData,
        selectedGraphNodeCount: selectedNodeCount,
      });
    }
    const projection = graphData?.projection && typeof graphData.projection === "object" ? graphData.projection : state?.graphProjection || {};
    return {
      mode: asText(projection.mode, "full").toLowerCase() || "full",
      canExpand:
        typeof deps.canExpandCurrentProjection === "function" ? !!invokeOrFallback(() => deps.canExpandCurrentProjection(), false) : false,
      selectedNodeCount: asCount(selectedNodeCount, 0),
    };
  }

  function buildDefaultShellOverlayState() {
    return {
      filter: { open: false, anchorRect: null },
      stylePopover: {
        open: false,
        anchorRect: null,
        sourceKey: "",
        kind: "",
        currentValue: "",
        options: [],
        commonColors: [],
        recentColors: [],
        iconTabs: [],
        activeTab: "",
        iconQuery: "",
        icons: [],
      },
      menu: { open: false, anchorRect: null, sourceKey: "", items: [] },
      exportPanel: { open: false, anchorRect: null, scale: 1 },
      contextMenu: { open: false, x: 0, y: 0, items: [] },
      drawer: { open: false, title: "", kind: "", rows: [], actions: [], meta: null },
      nodeInfo: {
        open: false,
        point: null,
        preview: null,
        category: "",
        userName: "",
        accountLabel: "",
        accountValue: "",
        accountValueMono: false,
        accounts: [],
        moreCount: 0,
      },
      edgeInfo: {
        open: false,
        point: null,
        title: "",
        amountText: "",
        amountTone: "",
        countText: "0",
        countClickable: false,
        loading: false,
        error: "",
        rows: [],
      },
      edgeLabel: {
        open: false,
        editable: true,
        title: "编辑连线文本",
        value: "",
        placeholder: "输入文本内容…",
        hint: "双击线条可再次编辑",
        confirmLabel: "确定",
        cancelLabel: "取消",
        readonly: {
          open: false,
          title: "资金交易",
          amountText: "",
          amountTone: "",
          countText: "0",
          countClickable: false,
          loading: false,
          error: "",
          rows: [],
        },
      },
      txnModal: {
        open: false,
        title: "交易明细",
        hint: "",
        loading: false,
        error: "",
        columns: [],
        rows: [],
        emptyText: "暂无交易明细",
        sortCol: "txn_time",
        sortDir: "asc",
      },
    };
  }

  function buildShellOverlayState(state, deps = {}) {
    const adapter = deps.shellOverlayAdapter || null;
    if (adapter && typeof adapter.buildOverlayState === "function") {
      return adapter.buildOverlayState(state, deps.overlayOptions || {});
    }
    return buildDefaultShellOverlayState();
  }

  function buildShellStyleState(state, deps = {}) {
    const style = state?.ribbonStyle || state?.graphStyle || {};
    const fontList = Array.isArray(deps.fontList) ? deps.fontList : [];
    const lineStyleOptions = Array.isArray(deps.lineStyleOptions) ? deps.lineStyleOptions : [];
    const arrowOptions = Array.isArray(deps.arrowOptions) ? deps.arrowOptions : [];
    const nodeShapeOptions = Array.isArray(deps.nodeShapeOptions) ? deps.nodeShapeOptions : [];
    const findOptionLabel = typeof deps.findOptionLabel === "function" ? deps.findOptionLabel : null;
    const canEditEdgeDirection = typeof deps.canEditEdgeDirection === "function" ? deps.canEditEdgeDirection : null;
    const asBool = typeof deps.asBool === "function" ? deps.asBool : (value, fallback = false) => (typeof value === "boolean" ? value : !!fallback);
    return {
      fontFamilyLabel: fontList.find((row) => row && row.value === style.fontFamily)?.label || "字体",
      fontSizeLabel: String(style.fontSize ?? 12),
      fontBold: asBool(style.fontBold, false),
      fontItalic: asBool(style.fontItalic, false),
      fontUnderline: asBool(style.fontUnderline, false),
      fontShadow: asBool(style.fontShadow, false),
      textColor: style.textColor || "#0f172a",
      outlineColor: style.nodeColor ?? style.edgeColor ?? "#1f6feb",
      nodeFill: style.nodeFill || "rgba(255,255,255,0.05)",
      lineStyleLabel: findOptionLabel ? findOptionLabel(lineStyleOptions, style.edgeDash, "线型") : "线型",
      lineWidthLabel: `线宽 ${style.edgeWidth ?? 2}`,
      lineArrowLabel: findOptionLabel ? findOptionLabel(arrowOptions, style.edgeArrow, "方向") : "方向",
      nodeShapeLabel: findOptionLabel ? findOptionLabel(nodeShapeOptions, style.nodeShape, "形状") : "形状",
      iconSizeLabel: `大小 ${style.nodeSize ?? 18}`,
      createNodeMode: !!state?.createNodeMode,
      canEditEdgeDirection: canEditEdgeDirection ? !!invokeOrFallback(() => canEditEdgeDirection(state?.graph?.instance), false) : false,
    };
  }

  function buildShellViews(state, deps = {}) {
    const views = Array.isArray(state?.views) ? state.views : [];
    const viewCounts = typeof deps.viewCounts === "function" ? deps.viewCounts : () => ({ nodes: 0, edges: 0 });
    const normalizeCanvasViews =
      deps.canvasShellAdapter && typeof deps.canvasShellAdapter.normalizeCanvasViews === "function"
        ? deps.canvasShellAdapter.normalizeCanvasViews.bind(deps.canvasShellAdapter)
        : null;
    const items = views.map((view) => {
      const counts = invokeOrFallback(() => viewCounts(view), { nodes: 0, edges: 0 }) || { nodes: 0, edges: 0 };
      return {
        id: asText(view?.id),
        title: piiProjection.projectDetected(view?.title),
        saved: !!view?.saved,
        active: view?.id === state?.activeViewId,
        counts: {
          nodes: asCount(counts?.nodes, 0),
          edges: asCount(counts?.edges, 0),
        },
      };
    });
    if (normalizeCanvasViews) {
      return invokeOrFallback(
        () =>
          normalizeCanvasViews({
            views: items,
            activeViewId: asText(state?.activeViewId),
          }),
        items
      );
    }
    return items;
  }

  window.__ANALYTIX_FLOW_RUNTIME_STATE__ = {
    createReactOverlayState,
    createPointLayerOverlayState,
    createProjectionAutoMaterializeState,
    createProjectionLayoutSyncState,
    createTxnModalState,
    createDrillConfigState,
    buildProjectionShellState,
    buildShellOverlayState,
    buildShellStyleState,
    buildShellViews,
  };
})();
