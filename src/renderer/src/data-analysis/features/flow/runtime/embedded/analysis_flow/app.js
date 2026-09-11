(() => {
  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));
  const on = (el, evt, handler) => {
    if (el) el.addEventListener(evt, handler);
  };
  const isEditableContext = (target) => {
    const resolveEl = (node) => {
      if (!node) return null;
      if (node.nodeType === 1) return node;
      return node.parentElement;
    };
    const isEditableEl = (el) => {
      if (!el || el.nodeType !== 1) return false;
      const tag = el.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return true;
      if (el.isContentEditable) return true;
      if (!el.closest) return false;
      return !!el.closest(
        "input, textarea, [contenteditable='true'], [contenteditable=''], [contenteditable='plaintext-only']"
      );
    };
    const el = resolveEl(target);
    const active = document.activeElement;
    return isEditableEl(el) || isEditableEl(active);
  };

  const LOG_PREFIX = "[Viz][JS]";
  const LOG_LEVEL_RANK = { DEBUG: 10, INFO: 20, WARN: 30, ERROR: 40 };
  const DEFAULT_LOG_SWITCH = {
    enabled: true,
    minLevel: "INFO",
    console: true,
    backend: true,
    core: true,
    dataFeed: true,
    graphTrace: true,
    graphSample: false,
    graphRender: true,
    layout: true,
    interaction: false,
    perf: false,
    debug: false,
    layoutVerbose: false,
    webglWarn: false,
    handoffProbe: false,
    canvasMetrics: false,
  };
  const LOG_STORAGE_KEY = "analysisFlow.log.switch";
  const FLOW_UI_BUILD = "flow-ui-20260222-06";
  function createAnalysisState(overrides = {}) {
    return {
      encodeWidth: false,
      encodeColor: false,
      nodeScale: false,
      filterMin: null,
      filterMax: null,
      collapseChildren: false,
      ...overrides,
    };
  }
  try {
    window.__ANALYTIX_FLOW_UI_BUILD = FLOW_UI_BUILD;
  } catch (e) {}
  let backendRef = null;
  let activeDrillPopover = null;
  let activeNodeInfoPopover = null;
  let activeEdgeInfoPopover = null;
  let logSwitch = { ...DEFAULT_LOG_SWITCH };
  const viewPersistenceDebug = {
    lastBackendCall: null,
    lastContextAction: null,
    lastSaveView: null,
  };
  const ordinaryPiiProjection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__ || null;
  const shellOverlayAdapter = window.__ANALYTIX_FLOW_OVERLAY_ADAPTER__ || null;
  const canvasShellAdapter = window.__ANALYTIX_FLOW_CANVAS_ADAPTER__ || null;
  const canvasCommandAdapter = window.__ANALYTIX_FLOW_CANVAS_COMMAND_ADAPTER__ || null;
  const canvasDragControllerAdapter = window.__ANALYTIX_FLOW_CANVAS_DRAG_CONTROLLER__ || null;
  const canvasDrillControllerAdapter = window.__ANALYTIX_FLOW_CANVAS_DRILL_CONTROLLER__ || null;
  const canvasHitViewportControllerAdapter = window.__ANALYTIX_FLOW_CANVAS_HIT_VIEWPORT_CONTROLLER__ || null;
  const canvasInteractionStoreAdapter = window.__ANALYTIX_FLOW_CANVAS_INTERACTION_STORE__ || null;
  const canvasRedrawControllerAdapter = window.__ANALYTIX_FLOW_CANVAS_REDRAW_CONTROLLER__ || null;
  const flowDebugDiagnosticsAdapter = window.__ANALYTIX_FLOW_DEBUG_DIAGNOSTICS__ || null;
  const flowDebugSurfaceAdapter = window.__ANALYTIX_FLOW_DEBUG_SURFACE__ || null;
  const flowObservabilityAdapter = window.__ANALYTIX_FLOW_OBSERVABILITY_ADAPTER__ || null;
  const graphMutationStoreAdapter = window.__ANALYTIX_FLOW_GRAPH_MUTATION_STORE__ || null;
  const graphPatchAdapter = window.__ANALYTIX_FLOW_GRAPH_PATCH_ADAPTER__ || null;
  const graphClipboardModel = window.__ANALYTIX_FLOW_GRAPH_CLIPBOARD_MODEL__ || null;
  const graphEdgeOffsetModel = window.__ANALYTIX_FLOW_GRAPH_EDGE_OFFSET_MODEL__ || null;
  const flowRuntimeStateAdapter = window.__ANALYTIX_FLOW_RUNTIME_STATE__ || null;
  const flowLayoutCommandStoreAdapter = window.__ANALYTIX_FLOW_LAYOUT_COMMAND_STORE__ || null;
  const flowShellStateStoreAdapter = window.__ANALYTIX_FLOW_SHELL_STATE_STORE__ || null;
  const flowSessionStoreAdapter = window.__ANALYTIX_FLOW_SESSION_STORE__ || null;
  const flowShellSyncStoreAdapter = window.__ANALYTIX_FLOW_SHELL_SYNC_STORE__ || null;
  const flowGraphCommandStoreAdapter = window.__ANALYTIX_FLOW_GRAPH_COMMAND_STORE__ || null;
  const flowSelectionStoreAdapter = window.__ANALYTIX_FLOW_SELECTION_STORE__ || null;
  const flowViewSyncStoreAdapter = window.__ANALYTIX_FLOW_VIEW_SYNC_STORE__ || null;
  const flowShellFacadeAdapter = window.__ANALYTIX_FLOW_SHELL_FACADE__ || null;
  const flowShellToolbarCommandsAdapter = window.__ANALYTIX_FLOW_SHELL_TOOLBAR_COMMANDS__ || null;
  const flowShellAnalysisCommandsAdapter = window.__ANALYTIX_FLOW_SHELL_ANALYSIS_COMMANDS__ || null;
  const flowShellCanvasCommandsAdapter = window.__ANALYTIX_FLOW_SHELL_CANVAS_COMMANDS__ || null;
  const flowShellOverlayCommandsAdapter = window.__ANALYTIX_FLOW_SHELL_OVERLAY_COMMANDS__ || null;
  const flowLayoutRuntimeAdapter = window.__ANALYTIX_FLOW_LAYOUT_RUNTIME_ADAPTER__ || null;
  const flowLayoutNodePlanClientFactory =
    window.__ANALYTIX_FLOW_LAYOUT_NODE_PLAN_CLIENT__ || null;
  const flowLayoutRoleProjectionClientFactory =
    window.__ANALYTIX_FLOW_LAYOUT_ROLE_PROJECTION_CLIENT__ || null;
  const flowGraphRenderPlanClientFactory =
    window.__ANALYTIX_FLOW_GRAPH_RENDER_PLAN_CLIENT__ || null;
  const flowNetworkSectorPlacementCacheFactory =
    window.__ANALYTIX_FLOW_NETWORK_SECTOR_PLACEMENT_CACHE__ || null;
  const flowNetworkSectorPlacementClientFactory =
    window.__ANALYTIX_FLOW_NETWORK_SECTOR_PLACEMENT_CLIENT__ || null;
  const flowLayoutWorkerResultModel = window.__ANALYTIX_FLOW_LAYOUT_WORKER_RESULT_MODEL__ || null;
  const flowLayoutGraphApplyModel = window.__ANALYTIX_FLOW_LAYOUT_GRAPH_APPLY_MODEL__ || null;
  const flowLayoutGraphApplyControllerFactory =
    window.__ANALYTIX_FLOW_LAYOUT_GRAPH_APPLY_CONTROLLER__ || null;
  const flowLayoutWorkerLifecycleControllerFactory =
    window.__ANALYTIX_FLOW_LAYOUT_WORKER_LIFECYCLE_CONTROLLER__ || null;
  const flowLayoutWorkerSchedulerControllerFactory =
    window.__ANALYTIX_FLOW_LAYOUT_WORKER_SCHEDULER_CONTROLLER__ || null;
  const flowLayoutPrewarmControllerFactory =
    window.__ANALYTIX_FLOW_LAYOUT_PREWARM_CONTROLLER__ || null;
  const flowLayoutClusterModel = window.__ANALYTIX_FLOW_LAYOUT_CLUSTER_MODEL__ || null;
  const flowLayoutPlanCacheApplyModel =
    window.__ANALYTIX_FLOW_LAYOUT_PLAN_CACHE_APPLY_MODEL__ || null;
  const flowLayoutPlanCacheModel = window.__ANALYTIX_FLOW_LAYOUT_PLAN_CACHE_MODEL__ || null;
  const flowLayoutPlanCacheControllerFactory =
    window.__ANALYTIX_FLOW_LAYOUT_PLAN_CACHE_CONTROLLER__ || null;
  const flowLayoutNetworkModel = window.__ANALYTIX_FLOW_LAYOUT_NETWORK_MODEL__ || null;
  const flowLayoutNetworkPresentationModel = window.__ANALYTIX_FLOW_LAYOUT_NETWORK_PRESENTATION_MODEL__ || null;
  const txnDetailRenderModel = window.__ANALYTIX_FLOW_TXN_DETAIL_RENDER_MODEL__ || null;
  const txnDetailRowModel = window.__ANALYTIX_FLOW_TXN_DETAIL_ROW_MODEL__ || null;
  const txnModalController = window.__ANALYTIX_FLOW_TXN_MODAL_CONTROLLER__ || null;
  const edgeDetailRenderModel = window.__ANALYTIX_FLOW_EDGE_DETAIL_RENDER_MODEL__ || null;
  const nodeDetailRenderModel = window.__ANALYTIX_FLOW_NODE_DETAIL_RENDER_MODEL__ || null;
  const infoPopoverController = window.__ANALYTIX_FLOW_INFO_POPOVER_CONTROLLER__ || null;
  const drawerController = window.__ANALYTIX_FLOW_DRAWER_CONTROLLER__ || null;
  const detailCommandController = window.__ANALYTIX_FLOW_DETAIL_COMMAND_CONTROLLER__ || null;
  const edgeTxnQueryModel = window.__ANALYTIX_FLOW_EDGE_TXN_QUERY_MODEL__ || null;
  const graphMinimapOrchestrationAdapter = window.__ANALYTIX_GRAPH_MINIMAP_ORCHESTRATION__ || null;
  const runtimeStore = window.__ANALYTIX_FLOW_RUNTIME_STORE__ || null;
  if (!flowRuntimeStateAdapter) {
    throw new Error("flow runtime state adapter missing");
  }
  if (!graphEdgeOffsetModel) {
    throw new Error("graph edge offset model missing");
  }
  if (
    !ordinaryPiiProjection ||
    typeof ordinaryPiiProjection.projectField !== "function" ||
    typeof ordinaryPiiProjection.projectDetected !== "function"
  ) {
    throw new Error("ordinary PII projection missing for flow app");
  }
  if (!flowSessionStoreAdapter) {
    throw new Error("flow session store adapter missing");
  }
  if (!graphClipboardModel) {
    throw new Error("flow graph clipboard model missing");
  }
  if (!flowShellStateStoreAdapter) {
    throw new Error("flow shell state store adapter missing");
  }
  if (!flowLayoutCommandStoreAdapter) {
    throw new Error("flow layout command store adapter missing");
  }
  if (!flowShellSyncStoreAdapter) {
    throw new Error("flow shell sync store adapter missing");
  }
  if (!flowGraphCommandStoreAdapter) {
    throw new Error("flow graph command store adapter missing");
  }
  if (!flowSelectionStoreAdapter) {
    throw new Error("flow selection store adapter missing");
  }
  if (!flowViewSyncStoreAdapter) {
    throw new Error("flow view sync store adapter missing");
  }
  if (!flowGraphRenderPlanClientFactory) {
    throw new Error("flow graph render plan client missing");
  }
  if (!flowNetworkSectorPlacementCacheFactory) {
    throw new Error("flow network sector placement cache missing");
  }
  if (!flowNetworkSectorPlacementClientFactory) {
    throw new Error("flow network sector placement client missing");
  }
  if (!flowShellFacadeAdapter) {
    throw new Error("flow shell facade adapter missing");
  }
  if (!flowShellToolbarCommandsAdapter) {
    throw new Error("flow shell toolbar commands adapter missing");
  }
  if (!flowShellAnalysisCommandsAdapter) {
    throw new Error("flow shell analysis commands adapter missing");
  }
  if (!flowShellCanvasCommandsAdapter) {
    throw new Error("flow shell canvas commands adapter missing");
  }
  if (!flowShellOverlayCommandsAdapter) {
    throw new Error("flow shell overlay commands adapter missing");
  }
  if (!flowLayoutRuntimeAdapter) {
    throw new Error("flow layout runtime adapter missing");
  }
  if (!flowLayoutNodePlanClientFactory) {
    throw new Error("flow layout node plan client missing");
  }
  if (!flowLayoutRoleProjectionClientFactory) {
    throw new Error("flow layout role projection client missing");
  }
  if (!flowLayoutWorkerResultModel) {
    throw new Error("flow layout worker result model missing");
  }
  if (!flowLayoutGraphApplyModel) {
    throw new Error("flow layout graph apply model missing");
  }
  if (!flowLayoutGraphApplyControllerFactory) {
    throw new Error("flow layout graph apply controller missing");
  }
  if (!flowLayoutWorkerLifecycleControllerFactory) {
    throw new Error("flow layout worker lifecycle controller missing");
  }
  if (!flowLayoutWorkerSchedulerControllerFactory) {
    throw new Error("flow layout worker scheduler controller missing");
  }
  if (!flowLayoutPrewarmControllerFactory) {
    throw new Error("flow layout prewarm controller missing");
  }
  if (!flowLayoutClusterModel) {
    throw new Error("flow layout cluster model missing");
  }
  if (!flowLayoutPlanCacheApplyModel) {
    throw new Error("flow layout plan cache apply model missing");
  }
  if (!flowLayoutPlanCacheModel) {
    throw new Error("flow layout plan cache model missing");
  }
  if (!flowLayoutPlanCacheControllerFactory) {
    throw new Error("flow layout plan cache controller missing");
  }
  if (!flowLayoutNetworkModel) {
    throw new Error("flow layout network model missing");
  }
  if (!flowLayoutNetworkPresentationModel) {
    throw new Error("flow layout network presentation model missing");
  }
  if (!txnDetailRenderModel) {
    throw new Error("flow txn detail render model missing");
  }
  if (!txnDetailRowModel) {
    throw new Error("flow txn detail row model missing");
  }
  if (!txnModalController) {
    throw new Error("flow txn modal controller missing");
  }
  if (!edgeDetailRenderModel) {
    throw new Error("flow edge detail render model missing");
  }
  if (!nodeDetailRenderModel) {
    throw new Error("flow node detail render model missing");
  }
  if (!infoPopoverController) {
    throw new Error("flow info popover controller missing");
  }
  if (!drawerController) {
    throw new Error("flow drawer controller missing");
  }
  if (!detailCommandController) {
    throw new Error("flow detail command controller missing");
  }
  if (!graphMinimapOrchestrationAdapter) {
    throw new Error("graph minimap orchestration adapter missing");
  }
  const {
    createLayoutNodePlanClient,
  } = flowLayoutNodePlanClientFactory;
  if (typeof createLayoutNodePlanClient !== "function") {
    throw new Error("flow layout node plan client missing createLayoutNodePlanClient");
  }
  const {
    createLayoutRoleProjectionClient,
  } = flowLayoutRoleProjectionClientFactory;
  if (typeof createLayoutRoleProjectionClient !== "function") {
    throw new Error("flow layout role projection client missing createLayoutRoleProjectionClient");
  }
  const {
    applyLayoutWorkerNodePlan,
  } = flowLayoutWorkerResultModel;
  const {
    buildLayoutNodeTargets,
    resolveLayoutApplyPlan,
  } = flowLayoutGraphApplyModel;
  const {
    createLayoutGraphApplyController,
  } = flowLayoutGraphApplyControllerFactory;
  if (typeof createLayoutGraphApplyController !== "function") {
    throw new Error("flow layout graph apply controller missing createLayoutGraphApplyController");
  }
  const {
    createLayoutWorkerLifecycleController,
  } = flowLayoutWorkerLifecycleControllerFactory;
  if (typeof createLayoutWorkerLifecycleController !== "function") {
    throw new Error("flow layout worker lifecycle controller missing createLayoutWorkerLifecycleController");
  }
  const {
    createLayoutWorkerSchedulerController,
  } = flowLayoutWorkerSchedulerControllerFactory;
  if (typeof createLayoutWorkerSchedulerController !== "function") {
    throw new Error("flow layout worker scheduler controller missing createLayoutWorkerSchedulerController");
  }
  const {
    createLayoutPrewarmController,
  } = flowLayoutPrewarmControllerFactory;
  if (typeof createLayoutPrewarmController !== "function") {
    throw new Error("flow layout prewarm controller missing createLayoutPrewarmController");
  }
  const {
    sortStableIds,
    updateClusterSlotCacheFromLayoutReport: updateClusterSlotModelCacheFromLayoutReport,
  } = flowLayoutClusterModel;
  const {
    createLayoutPlanCacheController,
  } = flowLayoutPlanCacheControllerFactory;
  if (typeof createLayoutPlanCacheController !== "function") {
    throw new Error("flow layout plan cache controller missing createLayoutPlanCacheController");
  }
  const {
    summarizeLayout,
  } = flowLayoutNetworkModel;
  const { restoreNetworkPresentation: restoreNetworkPresentationModel } = flowLayoutNetworkPresentationModel;
  const {
    TXN_COL_MIN_W,
    TXN_COL_MAX_W,
    TXN_DETAIL_COLS,
    formatTxnMoney: formatTxnMoneyModel,
    isTxnSortableCol: isTxnSortableColModel,
    isTxnMoneyCol: isTxnMoneyColModel,
    isTxnMonoCol: isTxnMonoColModel,
    getTxnSortLabel: getTxnSortLabelModel,
  } = txnDetailRenderModel;
  const {
    initializeTxnModalState,
    resetTxnModalState,
    toggleTxnModalSortState,
  } = txnModalController;
  const {
    buildTxnModalRows,
  } = txnDetailRowModel;
  const {
    buildEdgeTxnReadonlyHtml: buildEdgeTxnReadonlyHtmlModel,
    buildEdgeDetailHtml: buildEdgeDetailHtmlModel,
  } = edgeDetailRenderModel;
  const {
    buildNodePreviewHtml: buildNodePreviewHtmlModel,
    buildNodeInfoHtml: buildNodeInfoHtmlModel,
    buildNodeDetailHtml: buildNodeDetailHtmlModel,
    buildNodeDetailDrawer: buildNodeDetailDrawerModel,
  } = nodeDetailRenderModel;
  const {
    hideInfoPopover,
    showInfoPopoverAt,
    shouldCloseInfoPopover,
    routeInlineInfoPopoverWheel,
    routeDocumentInfoPopoverWheel,
  } = infoPopoverController;
  const {
    closeDrawerDom,
    openDrawerDom,
    closeReactDrawerState,
    openReactDrawerState,
    runReactDrawerAction: runReactDrawerActionModel,
  } = drawerController;
  const {
    openNodeDetailDrawer: openNodeDetailDrawerModel,
    openEdgeDetailDrawer: openEdgeDetailDrawerModel,
  } = detailCommandController;
  [
    ["summarizeLayout", summarizeLayout],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`flow layout network model missing ${name}`);
  });
  if (typeof restoreNetworkPresentationModel !== "function") {
    throw new Error("flow layout network presentation model missing restoreNetworkPresentation");
  }
  [
    ["formatTxnMoney", formatTxnMoneyModel],
    ["isTxnSortableCol", isTxnSortableColModel],
    ["isTxnMoneyCol", isTxnMoneyColModel],
    ["isTxnMonoCol", isTxnMonoColModel],
    ["getTxnSortLabel", getTxnSortLabelModel],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`flow txn detail render model missing ${name}`);
  });
  if (!Array.isArray(TXN_DETAIL_COLS) || !TXN_DETAIL_COLS.length) {
    throw new Error("flow txn detail render model missing TXN_DETAIL_COLS");
  }
  if (typeof buildTxnModalRows !== "function") {
    throw new Error("flow txn detail row model missing buildTxnModalRows");
  }
  [
    ["initializeTxnModalState", initializeTxnModalState],
    ["resetTxnModalState", resetTxnModalState],
    ["toggleTxnModalSortState", toggleTxnModalSortState],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`flow txn modal controller missing ${name}`);
  });
  [
    ["buildEdgeTxnReadonlyHtml", buildEdgeTxnReadonlyHtmlModel],
    ["buildEdgeDetailHtml", buildEdgeDetailHtmlModel],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`flow edge detail render model missing ${name}`);
  });
  [
    ["buildNodePreviewHtml", buildNodePreviewHtmlModel],
    ["buildNodeInfoHtml", buildNodeInfoHtmlModel],
    ["buildNodeDetailHtml", buildNodeDetailHtmlModel],
    ["buildNodeDetailDrawer", buildNodeDetailDrawerModel],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`flow node detail render model missing ${name}`);
  });
  [
    ["hideInfoPopover", hideInfoPopover],
    ["showInfoPopoverAt", showInfoPopoverAt],
    ["shouldCloseInfoPopover", shouldCloseInfoPopover],
    ["routeInlineInfoPopoverWheel", routeInlineInfoPopoverWheel],
    ["routeDocumentInfoPopoverWheel", routeDocumentInfoPopoverWheel],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`flow info popover controller missing ${name}`);
  });
  [
    ["closeDrawerDom", closeDrawerDom],
    ["openDrawerDom", openDrawerDom],
    ["closeReactDrawerState", closeReactDrawerState],
    ["openReactDrawerState", openReactDrawerState],
    ["runReactDrawerAction", runReactDrawerActionModel],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`flow drawer controller missing ${name}`);
  });
  [
    ["openNodeDetailDrawer", openNodeDetailDrawerModel],
    ["openEdgeDetailDrawer", openEdgeDetailDrawerModel],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`flow detail command controller missing ${name}`);
  });
  const graphDataModel = window.AnalytixGraphDataModel;
  if (!graphDataModel) throw new Error("graph-data-model-missing");
  const {
    applyEdgeOffsetUpdates,
    summarizeEdgeOffsets,
  } = graphEdgeOffsetModel;
  [
    ["applyEdgeOffsetUpdates", applyEdgeOffsetUpdates],
    ["summarizeEdgeOffsets", summarizeEdgeOffsets],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`graph edge offset model missing ${name}`);
  });
  const graphSearchModel = window.__ANALYTIX_FLOW_GRAPH_SEARCH_MODEL__;
  if (!graphSearchModel) throw new Error("graph-search-model-missing");
  const analysisGraphDataModel = window.__ANALYTIX_FLOW_ANALYSIS_GRAPH_DATA_MODEL__;
  if (!analysisGraphDataModel) throw new Error("analysis-graph-data-model-missing");
  const graphMergeModel = window.__ANALYTIX_FLOW_GRAPH_MERGE_MODEL__;
  if (!graphMergeModel) throw new Error("graph-merge-model-missing");
  const graphTransactionModel = window.__ANALYTIX_FLOW_GRAPH_TRANSACTION_MODEL__;
  if (!graphTransactionModel) throw new Error("graph-transaction-model-missing");
  if (!edgeTxnQueryModel) throw new Error("graph-edge-txn-query-model-missing");
  const {
    cloneGraphData,
    cloneFlowSummaryBoundary,
    cloneResultSnapshotRef,
    cloneGraphProjection,
    normalizeSnapshotGraphPayload,
    buildGraphSnapshotData,
    isUnknownAccountLabel,
    stripDisplayIdPrefix,
    normalizeDisplayIds,
    formatDisplayIdLabel,
    collectNodeDisplayIds,
    normalizeNodeDisplay,
    normalizeRawGraphPayload,
    buildGraphNodeDisplayModel,
    buildGraphEdgeDisplayModel,
    buildGraphTxnNodeId,
    buildGraphTxnNodeModel,
    buildGraphTxnEdgeModel,
    applyGraphDisplayFilters,
  } = graphDataModel;
  const { createGraphSearchClient } = graphSearchModel;
  if (typeof createGraphSearchClient !== "function") {
    throw new Error("graph search model missing createGraphSearchClient");
  }
  const {
    collectFiniteNodePositions,
    applyNodePositions,
    createAnalysisGraphDataClient,
  } = analysisGraphDataModel;
  if (typeof createAnalysisGraphDataClient !== "function") {
    throw new Error("analysis graph data model missing createAnalysisGraphDataClient");
  }
  const { createSameNameMergeClient } = graphMergeModel;
  if (typeof createSameNameMergeClient !== "function") {
    throw new Error("graph merge model missing createSameNameMergeClient");
  }
  const {
    normalizeTxnTimeText,
    parseTxnTimeValue,
    normalizeAccountTxnRows,
    normalizeTxnModalRows,
    sortTxnModalRows,
    computeDrillOutflows,
  } = graphTransactionModel;
  const {
    normalizeEdgeTxnKeyList,
    buildEdgeTxnCounterpartyQueries,
    resolveEdgeTxnRowOwnerKey,
    refineEdgeTxnBackendRows,
    readTxnDetailLoadNow,
    buildTxnDetailLoadMetric,
    normalizeEdgeTxnCursorPageResult,
    resolveEdgeTxnReadonlyPreviewRows,
    buildEdgeTxnDirectionalRuleSets,
  } = edgeTxnQueryModel;
  [
    ["normalizeEdgeTxnKeyList", normalizeEdgeTxnKeyList],
    ["buildEdgeTxnCounterpartyQueries", buildEdgeTxnCounterpartyQueries],
    ["resolveEdgeTxnRowOwnerKey", resolveEdgeTxnRowOwnerKey],
    ["refineEdgeTxnBackendRows", refineEdgeTxnBackendRows],
    ["readTxnDetailLoadNow", readTxnDetailLoadNow],
    ["buildTxnDetailLoadMetric", buildTxnDetailLoadMetric],
    ["normalizeEdgeTxnCursorPageResult", normalizeEdgeTxnCursorPageResult],
    ["resolveEdgeTxnReadonlyPreviewRows", resolveEdgeTxnReadonlyPreviewRows],
    ["buildEdgeTxnDirectionalRuleSets", buildEdgeTxnDirectionalRuleSets],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`graph edge txn query model missing ${name}`);
  });
  const graphProjectionLayoutModel = window.__ANALYTIX_FLOW_GRAPH_PROJECTION_LAYOUT_MODEL__;
  if (!graphProjectionLayoutModel) throw new Error("graph-projection-layout-model-missing");
  const {
    isClusterProjectionNode,
    isSkeletonProjectionMode: isSkeletonProjectionModeModel,
    isCollapsedProjectionNode,
    buildProjectionAutoMaterializeSignature,
    createProjectionLayoutSeedClient,
  } = graphProjectionLayoutModel;
  [
    ["isClusterProjectionNode", isClusterProjectionNode],
    ["isSkeletonProjectionMode", isSkeletonProjectionModeModel],
    ["isCollapsedProjectionNode", isCollapsedProjectionNode],
    ["buildProjectionAutoMaterializeSignature", buildProjectionAutoMaterializeSignature],
    ["createProjectionLayoutSeedClient", createProjectionLayoutSeedClient],
  ].forEach(([name, fn]) => {
    if (typeof fn !== "function") throw new Error(`graph projection layout model missing ${name}`);
  });
  const graphProjectionLayoutSyncModel = window.__ANALYTIX_FLOW_GRAPH_PROJECTION_LAYOUT_SYNC_MODEL__;
  if (!graphProjectionLayoutSyncModel) throw new Error("graph-projection-layout-sync-model-missing");
  const { collectProjectionLayoutSyncModels } = graphProjectionLayoutSyncModel;
  if (typeof collectProjectionLayoutSyncModels !== "function") {
    throw new Error("graph projection layout sync model missing collectProjectionLayoutSyncModels");
  }

  function getGraphEngine() {
    return window.AnalytixGraphEngine || null;
  }

  function getRuntimeBackend() {
    if (runtimeStore && typeof runtimeStore.getBackend === "function") {
      const backend = runtimeStore.getBackend();
      if (backend && typeof backend === "object") return backend;
    }
    const backend = window.__ANALYTIX_FLOW_RUNTIME_BACKEND__ || null;
    return backend && typeof backend === "object" ? backend : null;
  }

  async function fetchEdgeLaneTxnRows(model, lane = "", options = {}) {
    const loadStartedAt = readTxnDetailLoadNow();
    const signedEnabled = isSignedEdgeDetailEnabled(model);
    const signed = edgeSignedMeta(model, lane);
    const direction = resolveEdgeDirectionByLane(model, signed.selectedLane);
    const fromKeys = edgeNodeAccountKeysById(direction.from);
    const toKeys = edgeNodeAccountKeysById(direction.to);
    const fromPlaceholder = edgeNodePlaceholderMetaById(direction.from);
    const toPlaceholder = edgeNodePlaceholderMetaById(direction.to);
    if (!fromKeys.length || !toKeys.length) return [];
    const rangeStart = String(model?.first_time || "").slice(0, 10);
    const rangeEnd = String(model?.last_time || "").slice(0, 10);
    const toName = edgeNodeTitleById(direction.to);
    const fromName = edgeNodeTitleById(direction.from);
    let backendPageCount = 0;
    let backendRowsCount = 0;

    const queryModelOptions = {
      normalizeKey,
      normalizePlaceholderKind,
      placeholderTokenPrefix: PLACEHOLDER_TOKEN_PREFIX,
    };
    const fetchRowsByRule = async (rule) => {
      const ownerKeys = normalizeEdgeTxnKeyList(rule.ownerKeys, queryModelOptions);
      if (!ownerKeys.length) return [];
      const queries = buildEdgeTxnCounterpartyQueries(
        {
          cpKeys: rule.cpKeys,
          cpName: rule.cpName,
          cpPlaceholder: rule.cpPlaceholder,
        },
        queryModelOptions
      );
      if (!queries.length) return [];
      const groups = await Promise.all(
        queries.map(async (query) => {
          const rows = [];
          const seenCursors = new Set();
          let cursor = null;
          while (true) {
            const page = await fetchStatsTxnRowsPage({
              selected: ownerKeys,
              keyType: query.keyType,
              keyValue: query.keyValue,
              keyValues: Array.isArray(query.keyValues) ? query.keyValues : [],
              dateStart: rangeStart,
              dateEnd: rangeEnd,
              filter: rule.dir,
              sortDir: "asc",
              limit: EDGE_TXN_DETAIL_PAGE_LIMIT,
              cursor,
            });
            backendPageCount += 1;
            backendRowsCount += page.rows.length;
            rows.push(
              ...refineEdgeTxnBackendRows(page.rows, query, queryModelOptions).map((row) => ({
                ...row,
                __ownerKey: resolveEdgeTxnRowOwnerKey(row, ownerKeys, queryModelOptions),
                __flow: rule.flow,
              }))
            );
            if (page.done || !page.nextCursor) break;
            const cursorKey = JSON.stringify(page.nextCursor);
            if (seenCursors.has(cursorKey)) break;
            seenCursors.add(cursorKey);
            cursor = page.nextCursor;
          }
          return rows;
        })
      );
      return groups.flat();
    };
    const pickDirectionalRows = async (
      flowFromKeys,
      flowToKeys,
      flowFromName,
      flowToName,
      flowTag,
      flowFromPlaceholder,
      flowToPlaceholder
    ) => {
      const ruleSets = buildEdgeTxnDirectionalRuleSets(
        {
          flowFromKeys,
          flowToKeys,
          flowFromName,
          flowToName,
          flowTag,
          flowFromPlaceholder,
          flowToPlaceholder,
        },
        queryModelOptions
      );
      const strictGroups = await Promise.all(ruleSets.strict.map((rule) => fetchRowsByRule(rule)));
      const strict = strictGroups.flat();
      if (strict.length) return { rows: strict, usedFallback: false };
      // Preserve the previous tolerance for datasets with opposite dc_flag semantics on one side.
      const fallbackGroups = await Promise.all(ruleSets.fallback.map((rule) => fetchRowsByRule(rule)));
      const fallback = fallbackGroups.flat();
      if (fallback.length) return { rows: fallback, usedFallback: true };
      const allDirectionGroups = await Promise.all(
        ruleSets.strict.map((rule) => fetchRowsByRule({ ...rule, dir: "all" }))
      );
      const allDirection = allDirectionGroups.flat();
      return { rows: allDirection, usedFallback: allDirection.length > 0 };
    };

    const mode = String(model?.mode || "").toLowerCase();
    const strictForwardPick = await pickDirectionalRows(
      fromKeys,
      toKeys,
      fromName,
      toName,
      "forward",
      fromPlaceholder,
      toPlaceholder
    );
    const strictReversePick =
      signedEnabled && mode !== "double"
        ? await pickDirectionalRows(toKeys, fromKeys, toName, fromName, "reverse", toPlaceholder, fromPlaceholder)
        : { rows: [], usedFallback: false };

    let filtered = [];
    let strictRows = [];
    let usedFallback = false;
    if (signedEnabled && mode !== "double") {
      // Net-merged single edge: show both directions (+/-), not just one direction.
      strictRows = strictForwardPick.rows.concat(strictReversePick.rows);
      filtered = strictRows;
      usedFallback = !!(strictForwardPick.usedFallback || strictReversePick.usedFallback);
    } else {
      strictRows = strictForwardPick.rows;
      filtered = strictRows;
      usedFallback = strictForwardPick.usedFallback;
    }
    const filteredBeforeDedupCount = filtered.length;
    filtered.sort((a, b) => {
      const ta = parseTxnTimeValue(a?.txn_time || "") || 0;
      const tb = parseTxnTimeValue(b?.txn_time || "") || 0;
      return ta - tb;
    });
    const result = filtered.map((row) => {
      const amt = Math.abs(Number(row?.amount) || 0);
      const rowDir = resolveEdgeTxnRowDirection(row, signed);
      const signedAmount = rowDir === "out" ? -amt : amt;
      return {
        ...row,
        __signedAmount: signedAmount,
        __signClass: rowDir,
        __dir: rowDir,
      };
    });
    const forwardRowsCount = result.filter((row) => String(row?.__flow || "") === "forward").length;
    const reverseRowsCount = result.filter((row) => String(row?.__flow || "") === "reverse").length;
    const forwardAmountSum = result
      .filter((row) => String(row?.__flow || "") === "forward")
      .reduce((sum, row) => sum + Math.abs(Number(row?.amount) || 0), 0);
    const reverseAmountSum = result
      .filter((row) => String(row?.__flow || "") === "reverse")
      .reduce((sum, row) => sum + Math.abs(Number(row?.amount) || 0), 0);
    try {
      const loadMetric = buildTxnDetailLoadMetric({
        surface: "flow",
        operation: "edge-detail",
        pageKind: "aggregate",
        requestCount: backendPageCount,
        pageLimit: EDGE_TXN_DETAIL_PAGE_LIMIT,
        rowsBefore: 0,
        rowsFetched: backendRowsCount,
        rowsAfter: result.length,
        done: true,
        durationMs: readTxnDetailLoadNow() - loadStartedAt,
      });
      const payload = {
        id: String(model?.id || "").trim(),
        mode: String(model?.mode || "").toLowerCase(),
        lane: signed.selectedLane || "",
        requestKey: String(options?.requestKey || ""),
        direction: {
          ...direction,
          fromKeys,
          toKeys,
          fromPlaceholder,
          toPlaceholder,
        },
        signedEnabled,
        dateRange: { start: rangeStart || "", end: rangeEnd || "" },
        counts: {
          backendPages: backendPageCount,
          backendRows: backendRowsCount,
          strictRows: strictRows.length,
          filteredBeforeDedup: filteredBeforeDedupCount,
          finalRows: result.length,
          usedFallback,
          forwardRows: forwardRowsCount,
          reverseRows: reverseRowsCount,
        },
        loadMetric,
        sums: {
          forwardAmount: forwardAmountSum,
          reverseAmount: reverseAmountSum,
          netAmount: forwardAmountSum - reverseAmountSum,
          laneAmount: edgeAmountByLane(model, signed.selectedLane || lane || ""),
          totalAmount: edgeAmountTotal(model),
        },
      };
      if (state.debugTrace) {
        payload.sample = result.slice(0, 3).map((row) => ({
          txn_id: String(row?.txn_id || row?.__txid || "").trim(),
          txn_time: normalizeTxnTimeText(row?.txn_time || ""),
          amount: Number(row?.amount) || 0,
          signedAmount: Number(row?.__signedAmount) || 0,
          owner: normalizeKey(row?.__ownerKey || ""),
          cp_acct: normalizeKey(row?.counterparty_acct || ""),
          cp_name: String(row?.counterparty_name || "").trim(),
          dc: String(row?.dc_flag || row?.dcFlag || "").trim(),
          flow: String(row?.__flow || "").trim(),
        }));
      }
      log("INFO", "edge txn rows fetched", payload);
      log("INFO", "edge txn detail perf", loadMetric);
    } catch (e) {}
    return result;
  }

  function buildEdgeTxnReadonlyHtml(model, lane = "", rows = [], opts = {}) {
    return buildEdgeTxnReadonlyHtmlModel(model, lane, rows, opts, {
      escapeHtml,
      fmtMoney,
      normalizeTxnTimeText,
      isSignedEdgeDetailEnabled,
      edgeSignedMeta,
      formatSignedMoney,
      resolveEdgeTxnReadonlyPreviewRows: resolveEdgeTxnReadonlyScrollableRows,
    });
  }

  function formatTxnMoney(value) {
    return formatTxnMoneyModel(value, { fmtMoney });
  }

  function getTxnDetailWidthsKey() {
    return "analytix.flow.txnDetail.cols";
  }

  function getTxnDetailColsVersion() {
    return TXN_DETAIL_COLS.map((c) => `${c.key}:${c.w}`).join("|");
  }

  function clampTxnColWidth(n) {
    const num = Number(n);
    if (!Number.isFinite(num)) return TXN_COL_MIN_W;
    return Math.max(TXN_COL_MIN_W, Math.min(TXN_COL_MAX_W, Math.round(num)));
  }

  function ensureTxnDetailWidths() {
    if (Array.isArray(state.txnDetailWidths) && state.txnDetailWidths.length === TXN_DETAIL_COLS.length) {
      return state.txnDetailWidths;
    }
    let widths = null;
    try {
      const raw = localStorage.getItem(getTxnDetailWidthsKey());
      if (raw) {
        const parsed = JSON.parse(raw);
        if (parsed && parsed.v === getTxnDetailColsVersion() && Array.isArray(parsed.widths)) {
          widths = parsed.widths.map((w, i) => clampTxnColWidth(w ?? TXN_DETAIL_COLS[i]?.w ?? 120));
        }
      }
    } catch (e) {}
    if (!Array.isArray(widths) || widths.length !== TXN_DETAIL_COLS.length) {
      widths = TXN_DETAIL_COLS.map((c) => clampTxnColWidth(c.w));
    }
    state.txnDetailWidths = widths;
    return widths;
  }

  function saveTxnDetailWidths() {
    try {
      const payload = {
        v: getTxnDetailColsVersion(),
        widths: ensureTxnDetailWidths().slice(0, TXN_DETAIL_COLS.length),
      };
      localStorage.setItem(getTxnDetailWidthsKey(), JSON.stringify(payload));
    } catch (e) {}
  }

  function isTxnSortableCol(colKey) {
    return isTxnSortableColModel(colKey);
  }

  function isTxnMoneyCol(colKey) {
    return isTxnMoneyColModel(colKey);
  }

  function isTxnMonoCol(colKey) {
    return isTxnMonoColModel(colKey);
  }

  function isTxnModalOpen() {
    return !!state.txnModal?.open;
  }

  function getTxnSortLabel(sortCol, sortDir) {
    return getTxnSortLabelModel(sortCol, sortDir);
  }

  function buildTxnModalTitle(model, lane = "") {
    const dir = resolveEdgeDirectionByLane(model, lane);
    const fromLabel = edgeNodeTitleById(dir.from) || String(dir.from || "").trim() || "-";
    const toLabel = edgeNodeTitleById(dir.to) || String(dir.to || "").trim() || "-";
    return `交易详情 - ${fromLabel} → ${toLabel}`;
  }

  let wheelRouteLogAt = 0;
  let wheelRouteLogKey = "";
  function logWheelRoute(kind, payload = {}) {
    if (!(state.debugTrace || window.__ANALYTIX_DEBUG_WHEEL_ROUTE)) return;
    const now = Date.now();
    const key = `${kind}|${payload.route || ""}|${payload.target || ""}|${payload.moved ? "1" : "0"}`;
    if (wheelRouteLogKey === key && now - wheelRouteLogAt < 40) return;
    wheelRouteLogKey = key;
    wheelRouteLogAt = now;
    log("INFO", "wheel route", { kind, ...payload });
  }

  function normalizeWheelDelta(ev) {
    let dx = Number(ev?.deltaX) || 0;
    let dy = Number(ev?.deltaY) || 0;
    const mode = Number(ev?.deltaMode) || 0;
    if (mode === 1) {
      dx *= 16;
      dy *= 16;
    } else if (mode === 2) {
      dx *= window.innerWidth || 1;
      dy *= window.innerHeight || 1;
    }
    return { dx, dy };
  }

  function handleWheelScroll(container, ev, { fallbackToHorizontal = true } = {}) {
    if (!container) return false;
    const { dx, dy } = normalizeWheelDelta(ev);
    if (!dx && !dy) return false;

    const maxTop = Math.max(0, (container.scrollHeight || 0) - (container.clientHeight || 0));
    const maxLeft = Math.max(0, (container.scrollWidth || 0) - (container.clientWidth || 0));
    const hasY = maxTop > 0;
    const hasX = maxLeft > 0;
    if (!hasY && !hasX) {
      logWheelRoute("miss", {
        route: "container-scroll",
        reason: "no-scroll-range",
        maxTop,
        maxLeft,
        deltaX: dx,
        deltaY: dy,
      });
      return false;
    }

    const prevTop = container.scrollTop || 0;
    const prevLeft = container.scrollLeft || 0;
    let nextTop = prevTop;
    let nextLeft = prevLeft;

    if (Math.abs(dx) > 0.01 && hasX) nextLeft += dx;
    if (Math.abs(dy) > 0.01) {
      if (ev.shiftKey && hasX) {
        nextLeft += dy;
      } else if (hasY) {
        nextTop += dy;
      } else if (fallbackToHorizontal && hasX) {
        nextLeft += dy;
      }
    }

    nextTop = Math.max(0, Math.min(maxTop, nextTop));
    nextLeft = Math.max(0, Math.min(maxLeft, nextLeft));

    if (nextTop === prevTop && nextLeft === prevLeft) {
      logWheelRoute("miss", {
        route: "container-scroll",
        reason: "clamped-no-move",
        scrollTop: prevTop,
        scrollLeft: prevLeft,
        maxTop,
        maxLeft,
        deltaX: dx,
        deltaY: dy,
      });
      return false;
    }
    container.scrollTop = nextTop;
    container.scrollLeft = nextLeft;
    ev.preventDefault();
    ev.stopPropagation();
    logWheelRoute("route", {
      route: "container-scroll",
      moved: true,
      scrollTop: nextTop,
      scrollLeft: nextLeft,
      maxTop,
      maxLeft,
      deltaX: dx,
      deltaY: dy,
    });
    return true;
  }

  function openEdgeTxnModal(model, lane = "", requestKey = "") {
    if (!model) return;
    const selectedLane = resolveEdgeLaneForDialog(model, lane);
    const key = String(requestKey || buildEdgeInfoRequestKey(model, selectedLane));
    const m = state.txnModal;
    const title = buildTxnModalTitle(model, selectedLane);
    const reqId = initializeTxnModalState(m, {
      title,
      model,
      lane: selectedLane,
      requestKey: key,
      expectedTotal: model?.count,
    });
    closeContextMenu();
    scheduleShellStateSync("react-txn-modal-open");
    Promise.resolve()
      .then(() => fetchEdgeLaneTxnRowsWithDedup(model, selectedLane, key))
      .then((rows) => {
        if (!m.open || m.reqId !== reqId) return;
        m.rows = normalizeTxnModalRows(rows || []);
        m.loading = false;
        m.error = "";
        m.expectedTotal = Math.max(m.expectedTotal, m.rows.length);
        scheduleShellStateSync("react-txn-modal-loaded");
      })
      .catch(() => {
        if (!m.open || m.reqId !== reqId) return;
        m.loading = false;
        m.error = "明细加载失败";
        scheduleShellStateSync("react-txn-modal-error");
      });
  }

  function closeTxnModal() {
    const m = state.txnModal;
    resetTxnModalState(m);
    scheduleShellStateSync("react-txn-modal-close");
  }


  function closeNodeInfoPopover() {
    if (isReactShellSubscribed()) {
      closeReactNodeInfo();
      activeNodeInfoPopover = null;
      return;
    }
    const pop = $("#nodeInfoPopover");
    if (!pop) return;
    activeNodeInfoPopover = hideInfoPopover(pop);
  }

  function closeEdgeInfoPopover() {
    if (isReactShellSubscribed()) {
      if (state.edgeInfoDebounceTimer) {
        clearTimeout(state.edgeInfoDebounceTimer);
        state.edgeInfoDebounceTimer = 0;
        state.edgeInfoDebounceKey = "";
      }
      activeEdgeInfoPopover = null;
      state.edgeInfoActiveKey = "";
      state.edgeInfoContext = null;
      state.edgeInfoPopoverSeq += 1;
      closeReactEdgeInfo();
      return;
    }
    const pop = $("#edgeInfoPopover");
    if (!pop) return;
    if (state.edgeInfoDebounceTimer) {
      clearTimeout(state.edgeInfoDebounceTimer);
      state.edgeInfoDebounceTimer = 0;
      state.edgeInfoDebounceKey = "";
    }
    activeEdgeInfoPopover = hideInfoPopover(pop);
    state.edgeInfoActiveKey = "";
    state.edgeInfoContext = null;
    state.edgeInfoPopoverSeq += 1;
  }

  function buildEdgeInfoRequestKey(model, lane = "") {
    const m = model || {};
    const selectedLane = resolveEdgeLaneForDialog(m, lane);
    return [
      String(state.caseId || ""),
      String(state.requestId || ""),
      String(m.id || ""),
      String(m.mode || "").toLowerCase(),
      selectedLane || "",
      String(m.first_time || ""),
      String(m.last_time || ""),
      isSignedEdgeDetailEnabled(m) ? "1" : "0",
    ].join("|");
  }

  function shouldLogEdgeInfoEvent(kind, key, windowMs = EDGE_INFO_LOG_WINDOW_MS) {
    if (!kind || !key) return true;
    const now = Date.now();
    const sig = `${kind}|${key}`;
    const lastAt = Number(state.edgeInfoLogDedup.get(sig) || 0);
    if (lastAt && now - lastAt < windowMs) return false;
    state.edgeInfoLogDedup.set(sig, now);
    if (state.edgeInfoLogDedup.size > 180) {
      const dropKey = state.edgeInfoLogDedup.keys().next().value;
      if (dropKey) state.edgeInfoLogDedup.delete(dropKey);
    }
    return true;
  }

  function pruneEdgeInfoTxnCache() {
    const cache = state.edgeInfoTxnCache;
    if (!(cache instanceof Map)) return;
    const now = Date.now();
    cache.forEach((entry, key) => {
      if (!entry || now - Number(entry.ts || 0) > EDGE_INFO_POPOVER_CACHE_TTL_MS) cache.delete(key);
    });
    while (cache.size > 140) {
      const first = cache.keys().next().value;
      if (!first) break;
      cache.delete(first);
    }
  }

  function clearEdgeInfoTxnCache({ clearLane = false } = {}) {
    if (state.edgeInfoTxnCache instanceof Map) state.edgeInfoTxnCache.clear();
    if (state.edgeInfoTxnInflight instanceof Map) state.edgeInfoTxnInflight.clear();
    if (state.edgeInfoLogDedup instanceof Map) state.edgeInfoLogDedup.clear();
    if (clearLane) state.edgeLaneById = Object.create(null);
  }

  async function fetchEdgeLaneTxnRowsWithDedup(model, lane = "", requestKey = "") {
    const key = String(requestKey || buildEdgeInfoRequestKey(model, lane));
    pruneEdgeInfoTxnCache();
    const cache = state.edgeInfoTxnCache;
    const inflight = state.edgeInfoTxnInflight;
    if (cache instanceof Map && cache.has(key)) {
      const entry = cache.get(key) || {};
      const ts = Number(entry.ts || 0);
      if (Date.now() - ts <= EDGE_INFO_POPOVER_CACHE_TTL_MS && Array.isArray(entry.rows)) {
        if (state.debugTrace) log("INFO", "edge txn rows cache hit", { key, rows: entry.rows.length });
        return entry.rows;
      }
      cache.delete(key);
    }
    if (inflight instanceof Map && inflight.has(key)) {
      if (state.debugTrace) log("INFO", "edge txn rows inflight reuse", { key });
      return inflight.get(key);
    }
    const task = fetchEdgeLaneTxnRows(model, lane, { requestKey })
      .then((rows) => {
        if (cache instanceof Map) cache.set(key, { ts: Date.now(), rows: Array.isArray(rows) ? rows : [] });
        return rows;
      })
      .finally(() => {
        if (inflight instanceof Map) inflight.delete(key);
      });
    if (inflight instanceof Map) inflight.set(key, task);
    return task;
  }

  function openEdgeInfoPopover(x, y, model, lane = "") {
    if (!model) return;
    closeNodeInfoPopover();
    const selectedLane = resolveEdgeLaneForDialog(model, lane);
    const requestKey = buildEdgeInfoRequestKey(model, selectedLane);
    const signedEnabled = isSignedEdgeDetailEnabled(model);
    const signed = edgeSignedMeta(model, selectedLane);
    if (selectedLane) rememberEdgeLane(model, selectedLane);
    const seq = ++state.edgeInfoPopoverSeq;
    state.edgeInfoActiveKey = requestKey;
    state.edgeInfoOpenAt = Date.now();
    state.edgeInfoContext = { model, lane: selectedLane, requestKey };
    if (isReactShellSubscribed()) {
      activeEdgeInfoPopover = null;
      openReactEdgeInfo(
        { x, y },
        buildReactEdgeInfoState(model, selectedLane, [], {
          loading: true,
          enableCountLink: true,
          requestKey,
        }),
        "react-edge-info-open"
      );
    } else {
      const pop = $("#edgeInfoPopover");
      if (!pop) return;
      activeEdgeInfoPopover = showInfoPopoverAt(
        pop,
        x,
        y,
        buildEdgeTxnReadonlyHtml(model, selectedLane, [], { loading: true, enableCountLink: true }),
        window
      );
    }
    try {
      if (shouldLogEdgeInfoEvent("open", requestKey)) {
        log("INFO", "edge info popover open", {
          id: model?.id || "",
          mode: model?.mode || "",
          lane: selectedLane || "",
          amount: signed.laneAmount,
          signedAmount: signedEnabled ? signed.signedAmount : null,
          sign: signedEnabled ? signed.signLabel : "",
          signedEnabled,
        });
      }
    } catch (e) {}
    if (state.edgeInfoDebounceTimer) {
      clearTimeout(state.edgeInfoDebounceTimer);
      state.edgeInfoDebounceTimer = 0;
      state.edgeInfoDebounceKey = "";
    }
    state.edgeInfoDebounceKey = requestKey;
    state.edgeInfoDebounceTimer = window.setTimeout(() => {
      state.edgeInfoDebounceTimer = 0;
      state.edgeInfoDebounceKey = "";
      Promise.resolve()
        .then(async () => {
          const rows = await fetchEdgeLaneTxnRowsWithDedup(model, selectedLane, requestKey);
          if (seq !== state.edgeInfoPopoverSeq) return;
          if (isReactShellSubscribed()) {
            if (!state.reactOverlay?.edgeInfo?.open) return;
            if (String(state.reactOverlay?.edgeInfo?.requestKey || "") !== requestKey) return;
            openReactEdgeInfo(
              { x, y },
              buildReactEdgeInfoState(model, selectedLane, rows, {
                enableCountLink: true,
                requestKey,
              }),
              "react-edge-info-loaded"
            );
          } else {
            const pop = $("#edgeInfoPopover");
            if (activeEdgeInfoPopover !== pop) return;
            pop.innerHTML = buildEdgeTxnReadonlyHtml(model, selectedLane, rows, { enableCountLink: true });
          }
          try {
            if (shouldLogEdgeInfoEvent("loaded", requestKey)) {
              const payload = {
                id: model?.id || "",
                mode: model?.mode || "",
                lane: selectedLane || "",
                amount: signed.laneAmount,
                signedAmount: signedEnabled ? signed.signedAmount : null,
                sign: signedEnabled ? signed.signLabel : "",
                signedEnabled,
                rows: rows.length,
              };
              if (state.debugTrace) {
                payload.sample = rows.slice(0, 3).map((row) => ({
                  txn_id: String(row?.txn_id || row?.__txid || "").trim(),
                  txn_time: normalizeTxnTimeText(row?.txn_time || ""),
                  amount: Number(row?.amount) || 0,
                  signedAmount: Number(row?.__signedAmount) || 0,
                  owner: normalizeKey(row?.__ownerKey || ""),
                  cp_acct: normalizeKey(row?.counterparty_acct || ""),
                  cp_name: String(row?.counterparty_name || "").trim(),
                  dc: String(row?.dc_flag || row?.dcFlag || "").trim(),
                }));
              }
              log("INFO", "edge info popover loaded", payload);
            }
          } catch (e) {}
        })
        .catch((err) => {
          if (seq !== state.edgeInfoPopoverSeq) return;
          if (isReactShellSubscribed()) {
            if (!state.reactOverlay?.edgeInfo?.open) return;
            if (String(state.reactOverlay?.edgeInfo?.requestKey || "") !== requestKey) return;
            openReactEdgeInfo(
              { x, y },
              buildReactEdgeInfoState(model, selectedLane, [], {
                error: "明细加载失败",
                enableCountLink: true,
                requestKey,
              }),
              "react-edge-info-error"
            );
          } else {
            const pop = $("#edgeInfoPopover");
            if (activeEdgeInfoPopover !== pop) return;
            pop.innerHTML = buildEdgeTxnReadonlyHtml(model, selectedLane, [], {
              error: "明细加载失败",
              enableCountLink: true,
            });
          }
          try {
            log("WARN", "edge info popover failed", {
              id: model?.id || "",
              error: String(err?.message || err),
            });
          } catch (e) {}
        });
    }, EDGE_INFO_POPOVER_DEBOUNCE_MS);
  }

  function getNodeCategoryName(model) {
    const raw = String(model?.categoryName || model?.category || model?.ntype || "").trim();
    if (!raw) return "节点";
    if (raw === "seed") return "种子账户";
    if (raw === "node") return "账户";
    return raw;
  }

  function getNodeUserName(model) {
    const name = String(model?.name || "").trim();
    if (name) return name;
    const title = String(model?.title || model?.label || "").trim();
    if (title && !isLikelyAccountId(title)) return title;
    return "未知";
  }

  function getNodeAccount(model) {
    const list = collectNodeDisplayIds(model);
    if (list.length) return list[0];
    const raw = String(
      model?.displayIdRaw || model?.display_id_raw || model?.displayId || model?.display_id || ""
    ).trim();
    const cleaned = stripDisplayIdPrefix(raw);
    if (cleaned && !cleaned.includes("账户合集") && !isUnknownAccountLabel(cleaned)) return cleaned;
    const id = String(model?.id || "").trim();
    return id || "未知";
  }

  function getNodeAccountList(model) {
    const list = collectNodeDisplayIds(model);
    return Array.isArray(list) ? list.filter(Boolean) : [];
  }

  function buildNodePreviewHtml({ size = 32, fill = "rgba(255,255,255,0.9)", stroke = "#1f6feb", lineWidth = 2 } = {}) {
    return buildNodePreviewHtmlModel({ size, fill, stroke, lineWidth }, { escapeHtml });
  }

  function openNodeInfoPopover(x, y, model) {
    if (!model) return;
    closeEdgeInfoPopover();
    if (isReactShellSubscribed()) {
      activeNodeInfoPopover = null;
      openReactNodeInfo({ x, y }, buildReactNodeInfoState(model));
      return;
    }
    const pop = $("#nodeInfoPopover");
    if (!pop) return;
    const style = state.graphStyle || {};
    const stroke = String(model.stroke || style.nodeColor || "#1f6feb");
    const fill = String(model.fill || style.nodeFill || "rgba(255,255,255,0.05)");
    const line = Number(model.lineWidth || style.nodeWidth || 2);
    const radius = Number(model.r || style.nodeSize || 18);
    const size = Math.round(Math.max(26, Math.min(52, radius * 2 || 32)));
    const accountList = getNodeAccountList(model);
    const isGroup = accountList.length > 1;
    const category = isGroup ? "账户合集" : getNodeCategoryName(model);
    const userName = isGroup ? "账户合集" : getNodeUserName(model);
    const account = getNodeAccount(model);
    const accountLabel = isGroup ? `卡号（${accountList.length}）` : "卡号";
    activeNodeInfoPopover = showInfoPopoverAt(pop, x, y, buildNodeInfoHtmlModel(
      {
        preview: {
            size,
            fill,
            stroke,
            lineWidth: Number.isFinite(line) ? line : 2,
        },
        category,
        userName,
        accountLabel,
        account,
        accountList,
        listMax: 18,
      },
      { escapeHtml }
    ), window);
  }

  // ===== Left tree =====
  const icons = {
    check:
      '<svg width="14" height="14" viewBox="0 0 24 24" fill="none">' +
      '<path d="M20 6L9 17l-5-5" stroke="rgba(31,111,235,.95)" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>' +
      "</svg>",
    chevron:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none">' +
      '<path d="M9 6l6 6-6 6" stroke="rgba(2,6,23,.55)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>' +
      "</svg>",
  };

  function getTreeExpandedKey(caseId, tab) {
    return `${caseId || "none"}|${tab || "byName"}`;
  }

  function restoreTreeExpanded(groups) {
    const ids = new Set((groups || []).map((g) => g.id));
    const key = getTreeExpandedKey(state.caseId, state.tab);
    const saved = state.treeExpandedMap.get(key);
    if (Array.isArray(saved) && saved.length) {
      state.expanded = new Set(saved.filter((id) => ids.has(id)));
    } else {
      state.expanded = new Set();
    }
  }

  function persistTreeExpanded() {
    if (!state.caseId) return;
    const key = getTreeExpandedKey(state.caseId, state.tab);
    state.treeExpandedMap.set(key, Array.from(state.expanded));
  }

  async function loadTreeData(tab, { resetSelection = true } = {}) {
    let groups = null;
    let treeUnavailable = false;
    let treeSemanticStatus = "uninitialized";
    if (state.backend && typeof state.backend.getStatsTree === "function" && state.caseId) {
      const raw = await backendCall("getStatsTree", JSON.stringify({ tab, caseId: state.caseId || "" }));
      const res = parseJSONSafe(raw, null);
      treeUnavailable = true;
      treeSemanticStatus = res?.semanticStatus === "source_unavailable" ? "source_unavailable" : "blocked";
    }
    if (!groups) groups = [];

    state.treeData = groups;
    state.treeUnavailable = treeUnavailable;
    state.treeSemanticStatus = treeSemanticStatus;
    restoreTreeExpanded(groups);
    if (resetSelection) state.selected = new Set();
    renderTree();
  }

  function updateSelectionSummary() {
    const n = state.selected.size;
    const el = $("#selCount");
    if (el) el.textContent = `已选 ${n}`;
    scheduleShellStateSync("selection-summary");
  }

  function renderTree() {
    const root = $("#tree");
    if (!root) return;
    if (state.treeUnavailable) {
      root.innerHTML = '<div class="treeEmpty">数据源不可用，无法确认对象范围</div>';
      updateSelectionSummary();
      scheduleShellStateSync("tree-render-unavailable");
      return;
    }
    const presentationStore = ensureShellStateStore();
    root.innerHTML = presentationStore.buildLegacyTreeOuterHtml({
      search: state.search || "",
      expandedIds: Array.from(state.expanded || []),
      selectedIds: Array.from(state.selected || []),
      icons,
    });
    updateSelectionSummary();
    scheduleShellStateSync("tree-render");
  }

  function toggleGroup(gid) {
    const rawGroupId = ensureShellStateStore().resolvePresentationId(gid);
    if (!rawGroupId) return;
    if (state.expanded.has(rawGroupId)) state.expanded.delete(rawGroupId);
    else state.expanded.add(rawGroupId);
    persistTreeExpanded();
    renderTree();
  }

  function toggleGroupSelect(gid) {
    const rawGroupId = ensureShellStateStore().resolvePresentationId(gid);
    if (!rawGroupId) return;
    const g = state.treeData.find((x) => x.id === rawGroupId);
    if (!g) return;
    const ids = (g.items || []).map((x) => x.id);
    const all = ids.every((id) => state.selected.has(id));
    if (all) ids.forEach((id) => state.selected.delete(id));
    else ids.forEach((id) => state.selected.add(id));
    renderTree();
  }

  function toggleItemSelect(iid) {
    const rawItemId = ensureShellStateStore().resolvePresentationId(iid);
    if (!rawItemId) return;
    if (state.selected.has(rawItemId)) state.selected.delete(rawItemId);
    else state.selected.add(rawItemId);
    renderTree();
  }

  async function setTab(tab, options = {}) {
    const resetSelection = options.resetSelection !== false;
    state.tab = tab;
    $("#tabByName").classList.toggle("active", tab === "byName");
    $("#tabByCard").classList.toggle("active", tab === "byCard");
    $("#tabByName").setAttribute("aria-selected", tab === "byName" ? "true" : "false");
    $("#tabByCard").setAttribute("aria-selected", tab === "byCard" ? "true" : "false");

    await loadTreeData(tab, { resetSelection });
  }

  function setSearchQuery(value) {
    state.search = String(value || "");
    const input = $("#leftSearch");
    if (input && input.value !== state.search) input.value = state.search;
    renderTree();
    scheduleShellStateSync("search-query");
  }

  function setDirectionFilter(dir) {
    const next = dir === "in" || dir === "out" ? dir : "all";
    state.dir = next;
    $$(".segMini").forEach((btn) => {
      btn.classList.toggle("active", (btn.dataset.dir || "all") === next);
    });
    scheduleShellStateSync("direction-filter");
  }

  function setHopFilter(value) {
    state.hop = Math.max(1, Math.min(3, Number(value) || 1));
    const input = $("#hop");
    if (input) input.value = String(state.hop);
    const text = $("#hopText");
    if (text) text.textContent = String(state.hop);
    scheduleShellStateSync("hop-filter");
  }

  function setMinAmountFilter(value) {
    const next = Number(value);
    state.minAmount = Number.isFinite(next) ? Math.max(0, next) : 0;
    const input = $("#minAmount");
    if (input) input.value = String(state.minAmount);
    scheduleShellStateSync("min-amount");
  }

  function setMaxEdgesFilter(value) {
    const next = Number(value);
    state.maxEdges = Number.isFinite(next) ? Math.max(50, Math.min(5000, Math.round(next))) : 800;
    const input = $("#maxEdges");
    if (input) input.value = String(state.maxEdges);
    scheduleShellStateSync("max-edges");
  }

  function triggerShellControl(controlId, eventType = "click") {
    const el = $("#" + String(controlId || ""));
    if (!el) return false;
    if (eventType === "contextmenu") {
      const rect = el.getBoundingClientRect();
      const event = new MouseEvent("contextmenu", {
        bubbles: true,
        cancelable: true,
        clientX: rect.left + rect.width / 2,
        clientY: rect.top + rect.height / 2,
      });
      return el.dispatchEvent(event);
    }
    const event = new MouseEvent(String(eventType || "click"), {
      bubbles: true,
      cancelable: true,
      clientX: 0,
      clientY: 0,
    });
    return el.dispatchEvent(event);
  }

  // ===== Drawer =====
  function closeDrawer() {
    if (isReactShellSubscribed()) {
      closeReactDrawer();
      return;
    }
    const drawer = $("#drawer");
    const wrap = $("#drawerWrap");
    closeDrawerDom({ drawer, wrap });
  }

  function openDrawer(title, html) {
    openDrawerDom(
      {
        wrap: $("#drawerWrap"),
        drawer: $("#drawer"),
        titleEl: $("#drawerTitle"),
        bodyEl: $("#drawerBody"),
      },
      title,
      html
    );
  }

  function openNodeDetailDrawer(model, graph) {
    openNodeDetailDrawerModel(model, graph, {
      isReactShellSubscribed,
      openDrawer,
      buildNodeDetailHtml,
      buildReactNodeDetailDrawer,
      openReactDrawer,
      copyText,
      toast,
    });
  }

  function openEdgeDetailDrawer(model) {
    openEdgeDetailDrawerModel(model, {
      isReactShellSubscribed,
      openDrawer,
      buildEdgeDetailHtml,
      buildReactEdgeDetailDrawer,
      openReactDrawer,
    });
  }

  function ensureGraph() {
    if (state.graph.inited && state.graph.instance) return state.graph.instance;
    const container = $("#graphContainer");
    if (!container) return null;

    const perfMode = isPerfMode();
    const modes = buildGraphModes(perfMode);
    const rendererPref = getGraphRendererPreference();
    let graph = null;
    try {
      const graphEngine = getGraphEngine();
      if (!graphEngine || typeof graphEngine.Graph !== "function") {
        throw new Error("Analytix graph engine unavailable");
      }
      graph = new graphEngine.Graph({
        container,
        width: container.clientWidth || 800,
        height: container.clientHeight || 600,
        minZoom: 0.05,
        maxZoom: 16,
        modes,
        defaultNode: { type: "hollow-entity" },
        defaultEdge: { type: "center-link" },
        renderer: rendererPref.renderer,
        scaleWithView: true,
        highDpi: true,
        highDpiMax: 4,
        textAtlasScale: 4,
        gpuTextShadow: false,
        webglWarn: !!logSwitch.webglWarn,
      });
    } catch (e) {
      try {
        log("ERROR", "webgl renderer init failed", {
          message: String(e?.message || e),
          webgl: rendererPref.caps?.webgl,
          webgl2: rendererPref.caps?.webgl2,
        });
      } catch (e2) {}
      try {
        toast("WebGL 初始化失败，无法创建图谱", "danger");
      } catch (e2) {}
      return null;
    }
    try {
      log("INFO", "renderer selected", {
        renderer: graph?.get?.("renderer") || rendererPref.renderer,
        preferWebGL: rendererPref.preferWebGL,
        webgl: rendererPref.caps?.webgl,
        webgl2: rendererPref.caps?.webgl2,
      });
    } catch (e) {}
    logGpuProbe(graph);
    logGraphDebug(graph, "renderer debug");
    initLayoutWorker();

    // Bottom-right minimap (glass overlay) for orientation on large graphs.
    try {
      initMinimap(graph);
    } catch (e) {
      // Minimap is non-critical; keep graph usable if plugin init fails.
    }

    const usingWebglEngine = String(getGraphEngine()?.version || "").startsWith("AnalytixWebGL");
    ensureCanvasDragController(graph, usingWebglEngine);
    graph.on("node:drag", (ev) => {
      state.canvasDragController?.handleNodeDrag?.(ev);
    });
    graph.on("node:dragstart", (ev) => {
      state.canvasDragController?.handleNodeDragStart?.(ev);
    });
    graph.on("node:dragend", (ev) => {
      state.canvasDragController?.handleNodeDragEnd?.(ev);
    });
    graph.on("canvas:dragstart", () => {
      state.canvasDragController?.handleCanvasDragStart?.();
    });
    graph.on("canvas:dragend", () => {
      state.canvasDragController?.handleCanvasDragEnd?.();
    });
    graph.on("canvas:click", (ev) => {
      closeDrawer();
      hideNodeHoverTip();
      noteCanvasHit("canvas-click", null, ev, "canvas:click");
      if (state.createNodeMode) {
        try {
          const e = extractDomEvent(ev);
          if (e && typeof graph.getPointByClient === "function") {
            const point = graph.getPointByClient(e.clientX, e.clientY);
            createNodeAt(point);
          } else {
            const x = ev?.canvasX ?? ev?.x ?? 0;
            const y = ev?.canvasY ?? ev?.y ?? 0;
            createNodeAt({ x, y });
          }
          setCreateNodeMode(false);
          setGhostFloatVisible(false);
        } catch (e) {}
        return;
      }
      try {
        clearCanvasSelection(graph, "canvas:click");
      } catch (e) {}
    });

    graph.on("node:click", (ev) => {
      closeDrawer();
      const item = ev.item;
      noteCanvasHit("node-click", item, ev, "node:click");
      const model = item?.getModel?.() || {};
      const isMulti = isMultiSelectEvent(ev);
      if (state.createNodeMode && model.ghost) {
        const e = extractDomEvent(ev);
        if (e && typeof graph.getPointByClient === "function") {
          const point = graph.getPointByClient(e.clientX, e.clientY);
          createNodeAt(point);
        } else {
          const x = ev?.canvasX ?? model.x ?? 0;
          const y = ev?.canvasY ?? model.y ?? 0;
          createNodeAt({ x, y });
        }
        setCreateNodeMode(false);
        setGhostFloatVisible(false);
        return;
      }
      if (!item) return;
      const wasSelected = item.hasState?.("selected");
      const nextSelected = toggleCanvasItemSelection(item, {
        targetGraph: graph,
        multi: isMulti,
        reason: "node:click",
      });
      if (isGhostProbeWindow()) {
        if (isDeleteProbeVerbose()) {
          logGhostProbe(
            "node-click-near-delete",
            {
              nodeId: String(model?.id || "").trim(),
              isMulti,
              wasSelected: !!wasSelected,
              nextSelected: !!nextSelected,
            },
            {
              dedupKey: `node-click-near-delete|${state.lastDeleteProbeId || ""}|${String(model?.id || "").trim()}|${nextSelected ? 1 : 0}`,
              dedupMs: 120,
              onlyInDeleteWindow: true,
            }
          );
        }
        scheduleGraphConsistencyCheck("node-click-near-delete");
      }
    });
    graph.on("node:contextmenu", (ev) => {
      const item = ev.item;
      if (!item) return;
      const model = item.getModel?.() || {};
      const items = buildNodeContextMenuItems(model);
      if (!items.length) {
        closeContextMenu();
        return;
      }
      const e = extractDomEvent(ev);
      if (!e) return;
      e.preventDefault();
      e.stopPropagation?.();
      try {
        const graphPoint =
          typeof graph.getPointByClient === "function"
            ? graph.getPointByClient(e.clientX, e.clientY)
            : { x: model.x, y: model.y };
        state.lastContextNode = {
          id: model.id,
          clientX: e.clientX,
          clientY: e.clientY,
          graphX: Number.isFinite(graphPoint?.x) ? graphPoint.x : null,
          graphY: Number.isFinite(graphPoint?.y) ? graphPoint.y : null,
          modelX: Number.isFinite(model.x) ? model.x : null,
          modelY: Number.isFinite(model.y) ? model.y : null,
          ts: Date.now(),
        };
        log("INFO", "node contextmenu", {
          id: model.id || "",
          title: model.title || "",
          client: { x: e.clientX, y: e.clientY },
          graph: {
            x: Number.isFinite(graphPoint?.x) ? Math.round(graphPoint.x * 100) / 100 : null,
            y: Number.isFinite(graphPoint?.y) ? Math.round(graphPoint.y * 100) / 100 : null,
          },
          model: {
            x: Number.isFinite(model.x) ? Math.round(model.x * 100) / 100 : null,
            y: Number.isFinite(model.y) ? Math.round(model.y * 100) / 100 : null,
          },
        });
      } catch (e) {}
      openContextMenu(e.clientX, e.clientY, items);
    });
    graph.on("node:dblclick", (ev) => {
      const item = ev.item;
      if (!item) return;
      const model = item.getModel?.() || {};
      const projectionMode = String(state.graphProjection?.mode || "").trim().toLowerCase();
      if (
        projectionMode === "skeleton" &&
        (isClusterProjectionNode(model) || String(model?.projection_cluster_id || model?.cluster_id || "").trim())
      ) {
        closeContextMenu();
        closeDrillPopover();
        closeDrawer();
        void expandProjectionNode(model, { reason: "node:dblclick" });
        return;
      }
      const e = extractDomEvent(ev);
      if (!e) return;
      e.preventDefault();
      e.stopPropagation?.();
      closeContextMenu();
      closeDrillPopover();
      closeDrawer();
      openNodeInfoPopover(e.clientX, e.clientY, model);
    });
    graph.on("edge:click", (ev) => {
      closeDrawer();
      closeEdgeInfoPopover();
      const isMulti = isMultiSelectEvent(ev);
      const item = ev.item;
      noteCanvasHit("edge-click", item, ev, "edge:click");
      if (!item) return;
      const wasSelected = item.hasState?.("selected");
      const nextSelected = toggleCanvasItemSelection(item, {
        targetGraph: graph,
        multi: isMulti,
        reason: "edge:click",
      });
      const edgeModel = item.getModel?.() || {};
      if (isGhostProbeWindow()) {
        const edgeId =
          String(edgeModel?.id || "").trim() ||
          `${String(edgeModel?.source || "").trim()}->${String(edgeModel?.target || "").trim()}`;
        if (isDeleteProbeVerbose()) {
          logGhostProbe(
            "edge-click-near-delete",
            {
              edgeId,
              isMulti,
              wasSelected: !!wasSelected,
              nextSelected: !!nextSelected,
            },
            {
              dedupKey: `edge-click-near-delete|${state.lastDeleteProbeId || ""}|${edgeId}|${nextSelected ? 1 : 0}`,
              dedupMs: 120,
              onlyInDeleteWindow: true,
            }
          );
        }
        scheduleGraphConsistencyCheck("edge-click-near-delete");
      }
      if (!nextSelected) {
        closeEdgeInfoPopover();
        return;
      }
      const model = edgeModel;
      let lane = "";
      if (String(model.mode || "").toLowerCase() === "double") {
        lane = resolveEdgeLane(model, "") || inferEdgeLaneByPoint(graph, model, ev);
        if (lane) {
          model.__selectedLane = lane;
          model.__hitLane = lane;
          rememberEdgeLane(model, lane);
        }
      }
      try {
        const laneLog = lane || resolveEdgeLane(model, "");
        const topAmt = parseAmountWithCurrency(model.labelTop) || 0;
        const botAmt = parseAmountWithCurrency(model.labelBottom) || 0;
        const clickLogKey = [
          String(model.id || ""),
          String(model.mode || ""),
          laneLog || "",
          String(edgeAmountByLane(model, laneLog)),
        ].join("|");
        if (shouldLogEdgeInfoEvent("click-lane", clickLogKey, 220)) {
          log("INFO", "edge click detail lane", {
            id: model.id || "",
            mode: model.mode || "",
            lane: laneLog,
            lineAmount: edgeAmountByLane(model, laneLog),
            topLabelAmount: topAmt,
            bottomLabelAmount: botAmt,
          });
        }
      } catch (e) {}
      if (!model.userCreated) {
        const point = resolveClientPointFromEvent(graph, ev);
        openEdgeInfoPopover(point.x, point.y, model, lane);
        return;
      }
      openEdgeDetailDrawer(model);
    });
    graph.on("edge:dblclick", (ev) => {
      const item = ev.item;
      noteCanvasHit("edge-dblclick", item, ev, "edge:dblclick");
      if (!item) return;
      closeEdgeInfoPopover();
      const model = item.getModel?.() || {};
      let lane = "";
      if (String(model.mode || "").toLowerCase() === "double") {
        lane = resolveEdgeLane(model, "") || inferEdgeLaneByPoint(graph, model, ev);
        if (lane) {
          model.__selectedLane = lane;
          model.__hitLane = lane;
          rememberEdgeLane(model, lane);
        }
      }
      if (!model.userCreated) {
        const point = resolveClientPointFromEvent(graph, ev);
        openEdgeInfoPopover(point.x, point.y, model, lane);
        return;
      }
      openEdgeLabelDialog(item);
    });

    graph.on("node:mouseenter", (ev) => {
      if (isPerfMode()) {
        if (ev.item) {
          revealNetworkDenseIncidentEdgesForHover(graph, ev.item, "node:mouseenter", { allowPerfMode: true });
        }
        return;
      }
      if (ev.item) {
        noteCanvasHit("node-hover", ev.item, ev, "node:mouseenter");
        setCanvasItemHover(ev.item, true, { targetGraph: graph, reason: "node:mouseenter" });
        revealNetworkDenseIncidentEdgesForHover(graph, ev.item, "node:mouseenter");
        showNodeHoverTip(ev.item.getModel?.() || {}, ev);
      }
    });
    graph.on("node:mouseleave", (ev) => {
      if (ev.item) {
        setCanvasItemHover(ev.item, false, { targetGraph: graph, reason: "node:mouseleave" });
      }
      restoreNetworkDenseHoverEdges(graph, "node:mouseleave");
      hideNodeHoverTip();
    });
    graph.on("edge:mouseenter", (ev) => {
      if (isPerfMode()) return;
      if (ev.item) {
        noteCanvasHit("edge-hover", ev.item, ev, "edge:mouseenter");
        setCanvasItemHover(ev.item, true, { targetGraph: graph, reason: "edge:mouseenter" });
      }
    });
    graph.on("edge:mouseleave", (ev) => {
      if (ev.item) {
        setCanvasItemHover(ev.item, false, { targetGraph: graph, reason: "edge:mouseleave" });
      }
    });
    graph.on("canvas:mouseleave", () => {
      restoreNetworkDenseHoverEdges(graph, "canvas:mouseleave");
      hideNodeHoverTip();
    });
    graph.on("mousemove", (ev) => {
      if (!state.createNodeMode) return;
      const e = extractDomEvent(ev);
      if (e) {
        updateGhostFromClient(e.clientX, e.clientY);
        return;
      }
      const x = ev?.canvasX ?? ev?.x ?? 0;
      const y = ev?.canvasY ?? ev?.y ?? 0;
      updateGhostPosition({ x, y });
    });

    graph.on("viewportchange", () => {
      syncCanvasViewport(graph, "viewportchange");
    });
    graph.on("afterrender", () => {
      if (isDeleteProbeVerbose()) {
        logGhostProbe(
          "afterrender-near-delete",
          { reason: "afterrender" },
          {
            dedupKey: `afterrender-near-delete|${state.graphMutationSeq || 0}`,
            dedupMs: 120,
            onlyInDeleteWindow: true,
          }
        );
      }
      scheduleGraphConsistencyCheck("afterrender");
      scheduleProjectionPointLayerRedraw("afterrender");
      scheduleProjectionAutoMaterialize("afterrender");
    });

    if (!state.graphResizeHandler) {
      state.graphResizeHandler = () => {
        try {
          const g = state.graph?.instance;
          const el = $("#graphContainer");
          if (!g || !el) return;
          const w = el.clientWidth || 800;
          const h = el.clientHeight || 600;
          g.changeSize(w, h);
          scheduleProjectionPointLayerRedraw("resize");
          scheduleProjectionAutoMaterialize("resize");
        } catch (e) {}
      };
      window.addEventListener("resize", state.graphResizeHandler);
    }

    state.graph.inited = true;
    state.graph.instance = graph;
    return graph;
  }

  function buildNodeDetailHtml(model, graph) {
    const id = model.id || "";
    const degree = (() => {
      try {
        const item = graph.findById(id);
        if (!item) return "";
        return String((item.getEdges() || []).length);
      } catch (e) {
        return "";
      }
    })();
    const html = buildNodeDetailHtmlModel(model, { degree }, { escapeHtml, fmtMoney });

    setTimeout(() => {
      $("#btnCopyNode")?.addEventListener("click", () => {
        copyText(id, "account_no");
        toast("已复制脱敏副本", "ok");
      });
      $("#btnFocusNode")?.addEventListener("click", () => {
        try {
          const item = graph.findById(id);
          if (item) graph.focusItem(item, true, { easing: "easeCubic", duration: 240 });
        } catch (e) {}
      });
    }, 0);

    return html;
  }

  function buildEdgeDetailHtml(model) {
    return buildEdgeDetailHtmlModel(model, {
      escapeHtml,
      fmtMoney,
      fmtShortRange,
      resolveEdgeLane,
      edgeAmountTotal,
      edgeAmountByLane,
    });
  }

  function renderReadonlyEdgeDialog(model, lane = "", rows = [], opts = {}) {
    if (isReactShellSubscribed() && state.reactOverlay?.edgeLabel?.open) {
      state.reactOverlay.edgeLabel.readonly = buildReactEdgeLabelReadonlyState(model, lane, rows, opts);
      state.reactOverlay.edgeLabel.editable = false;
      state.reactOverlay.edgeLabel.hint = "";
      state.reactOverlay.edgeLabel.cancelLabel = "关闭";
      scheduleShellStateSync("react-edge-label-readonly");
      return;
    }
    const panel = $("#edgeLabelReadonly");
    if (!panel) return;
    panel.innerHTML = buildEdgeTxnReadonlyHtml(model, lane, rows, opts);
    panel.hidden = false;
  }

  function setEdgeLabelDialogMode(editable) {
    if (isReactShellSubscribed() && state.reactOverlay?.edgeLabel?.open) {
      state.reactOverlay.edgeLabel.editable = !!editable;
      state.reactOverlay.edgeLabel.hint = editable ? "双击线条可再次编辑" : "";
      state.reactOverlay.edgeLabel.cancelLabel = editable ? "取消" : "关闭";
      if (editable) {
        state.reactOverlay.edgeLabel.readonly.open = false;
        state.reactOverlay.edgeLabel.readonly.rows = [];
        state.reactOverlay.edgeLabel.readonly.error = "";
        state.reactOverlay.edgeLabel.readonly.loading = false;
      }
      scheduleShellStateSync("react-edge-label-mode");
      return;
    }
    const input = $("#edgeLabelInput");
    const hint = $("#edgeLabelHint");
    const saveBtn = $("#edgeLabelSave");
    const cancelBtn = $("#edgeLabelCancel");
    const panel = $("#edgeLabelReadonly");
    const card = $("#edgeLabelCard");
    if (input) {
      input.style.display = editable ? "block" : "none";
      input.disabled = !editable;
      input.readOnly = !editable;
    }
    if (hint) {
      hint.style.display = editable ? "block" : "none";
      hint.textContent = editable ? "双击线条可再次编辑" : "";
    }
    if (panel) {
      panel.hidden = editable;
      if (editable) panel.innerHTML = "";
    }
    if (saveBtn) {
      saveBtn.style.display = editable ? "" : "none";
      saveBtn.disabled = !editable;
      saveBtn.style.opacity = "1";
    }
    if (cancelBtn) cancelBtn.textContent = editable ? "取消" : "关闭";
    if (card) card.classList.toggle("edgeTxnMode", !editable);
  }

  function openEdgeLabelDialog(edge) {
    if (!edge) return;
    const model = edge.getModel?.() || {};
    const presentationLabel = ordinaryPiiProjection.projectField(model.userCreated ? "node_id" : "", model.label || "");
    if (!model.userCreated) return;
    const editable = true;
    ++state.edgeLabelDialogSeq;
    state.edgeLabelTarget = edge;
    if (isReactShellSubscribed()) {
      openReactEdgeLabel(
        {
          editable,
          title: "编辑连线文本",
          value: presentationLabel,
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
        "react-edge-label-open"
      );
      return;
    }
    const mask = $("#edgeLabelModal");
    const input = $("#edgeLabelInput");
    const title = $("#edgeLabelTitle");
    if (!mask || !input) return;
    setEdgeLabelDialogMode(editable);
    input.value = presentationLabel;
    if (title) title.textContent = "编辑连线文本";
    mask.classList.add("show");
    mask.setAttribute("aria-hidden", "false");
    setTimeout(() => input.focus(), 0);
  }

  function closeEdgeLabelDialog() {
    if (isReactShellSubscribed()) {
      state.edgeLabelDialogSeq += 1;
      closeReactEdgeLabel();
      return;
    }
    const mask = $("#edgeLabelModal");
    if (!mask) return;
    mask.classList.remove("show");
    mask.setAttribute("aria-hidden", "true");
    state.edgeLabelTarget = null;
    state.edgeLabelDialogSeq += 1;
    const panel = $("#edgeLabelReadonly");
    if (panel) {
      panel.hidden = true;
      panel.innerHTML = "";
    }
  }

  function commitEdgeLabel() {
    const edge = state.edgeLabelTarget;
    if (!edge) return closeEdgeLabelDialog();
    const model = edge.getModel?.() || {};
    if (!model.userCreated) return closeEdgeLabelDialog();
    const input = $("#edgeLabelInput");
    const submittedLabel = (
      isReactShellSubscribed() ? state.reactOverlay?.edgeLabel?.value || "" : input?.value || ""
    ).trim();
    const existingLabel = String(model.label || "").trim();
    const existingPresentation = ordinaryPiiProjection.projectField(model.userCreated ? "node_id" : "", existingLabel);
    const label =
      submittedLabel === existingPresentation
        ? existingLabel
        : ordinaryPiiProjection.projectField(model.userCreated ? "node_id" : "", submittedLabel);
    const graph = ensureGraph();
    try {
      if (graph && edge) graph.updateItem(edge, { label });
      updateGraphDataById([edge], { label }, "edges");
      updateCurrentViewSnapshot();
    } catch (e) {}
    closeEdgeLabelDialog();
  }

  function openViewCloseModal(view) {
    if (!view) return Promise.resolve(false);
    const name = ordinaryPiiProjection.projectField("node_id", view.title || "图") || "图";
    const saved = !!view.saved;
    if (typeof window.__ANALYTIX_CONFIRM__ === "function") {
      return Promise.resolve(
        window.__ANALYTIX_CONFIRM__({
          tone: "danger",
          title: "确定关闭当前视图？",
          subtitle: saved ? `${name} · 已存储视图` : `${name} · 未存储视图`,
          description: saved
            ? "关闭后将同步移除该视图，无法恢复。"
            : "关闭后将从视图列表移除，未存储内容会丢失。",
          confirmLabel: "关闭视图",
          cancelLabel: "保留视图",
          width: 520,
        })
      );
    }
    const mask = $("#viewCloseModal");
    if (!mask) {
      const ok = confirm(
        saved
          ? `确定关闭“${name}”吗？\n\n关闭后将同步移除该视图，无法恢复。`
          : `确定关闭“${name}”吗？\n\n关闭后将从视图列表移除，未存储内容会丢失。`
      );
      return Promise.resolve(ok);
    }
    if (state.viewCloseResolver) closeViewCloseModal(false);
    const subtitle = $("#viewCloseSubtitle");
    const nameEl = $("#viewCloseName");
    const statusEl = $("#viewCloseStatus");
    const msgEl = $("#viewCloseMessage");
    const hintEl = $("#viewCloseHint");

    if (subtitle) subtitle.textContent = saved ? "关闭画布视图" : "关闭未存储视图";
    if (nameEl) nameEl.textContent = name;
    if (statusEl) {
      statusEl.textContent = saved ? "已存储" : "未存储";
      statusEl.classList.toggle("saved", saved);
    }
    if (msgEl) {
      msgEl.textContent = saved
        ? "关闭后将同步移除该视图，无法恢复。"
        : "关闭后将从视图列表移除，未存储内容会丢失。";
    }
    if (hintEl) {
      hintEl.textContent = saved ? "如需保留，请先导出或复制数据。" : "建议先存储视图，避免误删。";
    }

    mask.classList.add("show");
    mask.setAttribute("aria-hidden", "false");
    setTimeout(() => $("#viewCloseCancel")?.focus(), 0);

    return new Promise((resolve) => {
      state.viewCloseResolver = resolve;
    });
  }

  function closeViewCloseModal(confirmed) {
    const mask = $("#viewCloseModal");
    if (mask) {
      mask.classList.remove("show");
      mask.setAttribute("aria-hidden", "true");
    }
    const resolver = state.viewCloseResolver;
    state.viewCloseResolver = null;
    if (typeof resolver === "function") resolver(!!confirmed);
  }


  function hasActiveAnalysisEncodings() {
    return !!(state.analysis?.encodeWidth || state.analysis?.encodeColor || state.analysis?.nodeScale);
  }

  function applyStyleToGraph(style, { syncView = true, applyAnalysis = true } = {}) {
    const graph = ensureGraph();
    if (!graph) return;
    const next = { ...state.graphStyle, ...style };
    state.graphStyle = next;
    ensureRibbonStyle();
    state.ribbonStyle = { ...state.ribbonStyle, ...next };

    try {
      const edgeUpdate = buildEdgeUpdate(next);
      if (state.edgeDirectionLocked) {
        delete edgeUpdate.edgeArrow;
        delete edgeUpdate.mode;
      }
      if (state.analysis?.encodeWidth && edgeUpdate.lineWidth != null) {
        edgeUpdate.__baseLineWidth = edgeUpdate.lineWidth;
      }
      if (state.analysis?.encodeColor && edgeUpdate.stroke != null) {
        edgeUpdate.__baseStroke = edgeUpdate.stroke;
      }
      const nodeUpdate = buildNodeUpdate(next);
      if (nodeUpdate.r != null) {
        nodeUpdate.__baseR = nodeUpdate.r;
      }
      if (nodeUpdate.lineWidth != null) {
        nodeUpdate.__baseLineWidth = nodeUpdate.lineWidth;
      }
      const shouldFreeze = !state.introAnimating;
      const positions = shouldFreeze ? captureNodePositions(graph) : null;
      graph.setAutoPaint(false);
      try {
        graph.getEdges().forEach((edge) => {
          graph.updateItem(edge, edgeUpdate);
        });
        graph.getNodes().forEach((node) => {
          graph.updateItem(node, nodeUpdate);
        });
        if (positions) restoreNodePositions(graph, positions);
      } finally {
        graph.setAutoPaint(true);
        graph.paint();
      }
      if (Array.isArray(state.graph.data.edges)) {
        state.graph.data.edges.forEach((edge) => Object.assign(edge, edgeUpdate));
      }
      if (Array.isArray(state.graph.data.nodes)) {
        state.graph.data.nodes.forEach((node) => Object.assign(node, nodeUpdate));
      }
    } catch (e) {}

    if (syncView) {
      const view = getActiveView();
      if (view) {
        view.style = { ...next };
        view.updatedAt = nowIso();
        renderViewsBar();
      }
    }
    syncRibbonUI(state.ribbonStyle);
    if (applyAnalysis || hasActiveAnalysisEncodings()) {
      applyAnalysisEncodings();
    } else {
      syncAnalysisButtons();
    }
    if (state.lod.shadowsReduced) applyShadowLOD(true);
    if (state.layoutPreset === "network") {
      const nodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
      const edges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
      applyDenseNetworkRuntimeLod(graph, {
        mode: state.networkLayout?.lastPlanMode || resolveNetworkTopologyModeForGraph(nodes, edges),
        nodeCount: nodes.length,
        edgeCount: edges.length,
      });
    }
  }

  function syncGraphThemePalette(reason = "", { force = false } = {}) {
    const themeName = resolveGraphThemeName();
    if (!force && state.graphThemeName === themeName) return;
    if (!force && !isGraphThemeManagedStyle(state.graphStyle)) {
      state.graphThemeName = themeName;
      return;
    }

    const palette = createGraphThemeStyle(themeName);
    state.graphThemeName = themeName;
    state.graphStyle = { ...state.graphStyle, ...palette };
    ensureRibbonStyle();
    state.ribbonStyle = { ...state.ribbonStyle, ...palette };

    const graph = state.graph?.instance || null;
    if (graph) {
      const updateGraphItems = (items, kind) => {
        const rows = Array.isArray(items) ? items : [];
        rows.forEach((item) => {
          const model = item?.getModel?.() || {};
          const patch = buildGraphThemeItemPatch(model, kind, palette);
          if (!Object.keys(patch).length) return;
          try {
            graph.updateItem(item, patch);
          } catch (e) {}
        });
      };
      const canSetAutoPaint = typeof graph.setAutoPaint === "function";
      const prevAutoPaint = canSetAutoPaint ? graph.autoPaint !== false : true;
      if (canSetAutoPaint) graph.setAutoPaint(false);
      try {
        updateGraphItems(graph.getNodes?.() || [], "node");
        updateGraphItems(graph.getEdges?.() || [], "edge");
      } finally {
        if (canSetAutoPaint) graph.setAutoPaint(prevAutoPaint);
        graph.paint?.();
      }
    }

    const updateDataRows = (rows, kind) => {
      (Array.isArray(rows) ? rows : []).forEach((row) => {
        const patch = buildGraphThemeItemPatch(row, kind, palette);
        if (Object.keys(patch).length) Object.assign(row, patch);
      });
    };
    updateDataRows(state.graph?.data?.nodes, "node");
    updateDataRows(state.graph?.data?.edges, "edge");
    syncRibbonUI(state.ribbonStyle);
    scheduleShellStateSync(`graph-theme-palette:${reason || themeName}`);
  }

  function bindGraphThemePaletteSync() {
    if (state.graphThemeObserver || typeof MutationObserver !== "function") return;
    const observer = new MutationObserver(() => syncGraphThemePalette("root-theme"));
    try {
      observer.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ["data-theme", "class", "style"],
      });
      state.graphThemeObserver = observer;
    } catch (e) {}
  }

  function buildIconDataUrl(symbolId, color = "#0f172a") {
    if (!symbolId) return "";
    const key = `${symbolId}|${color}`;
    if (iconDataUrlCache.has(key)) return iconDataUrlCache.get(key);
    const symbol = document.getElementById(symbolId);
    if (!symbol) return "";
    const viewBox = symbol.getAttribute("viewBox") || "0 0 24 24";
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}" width="24" height="24" fill="none" color="${color}">${symbol.innerHTML}</svg>`;
    const dataUrl = `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
    iconDataUrlCache.set(key, dataUrl);
    return dataUrl;
  }

  function getPopoverFocusableElements(root) {
    return $$(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      root
    ).filter((el) => {
      if (!el || el.getAttribute("aria-hidden") === "true") return false;
      return el.offsetParent !== null || el.getClientRects().length > 0;
    });
  }

  function describePopoverAnchor(anchor, contentEl) {
    const explicitLabel = String(contentEl?.dataset?.popoverLabel || anchor?.dataset?.popoverLabel || "").trim();
    if (explicitLabel) return explicitLabel;
    const ariaLabel = String(anchor?.getAttribute?.("aria-label") || "").trim();
    if (ariaLabel) return `${ariaLabel}选项`;
    const text = String(anchor?.textContent || "").replace(/\s+/g, " ").trim();
    return text ? `${text}选项` : "工具栏选项";
  }

  function focusRibbonPopoverTarget(popover) {
    const preferred =
      popover.querySelector("[data-popover-initial-focus='true']") ||
      getPopoverFocusableElements(popover)[0] ||
      popover;
    if (preferred && typeof preferred.focus === "function") {
      preferred.focus({ preventScroll: true });
    }
  }

  function moveRibbonPopoverFocus(popover, step) {
    const focusables = getPopoverFocusableElements(popover);
    if (!focusables.length) return;
    const active = document.activeElement;
    const currentIndex = Math.max(0, focusables.indexOf(active));
    const nextIndex = (currentIndex + step + focusables.length) % focusables.length;
    focusables[nextIndex]?.focus({ preventScroll: true });
  }

  function closeRibbonPopover(options = {}) {
    const { restoreFocus = true } = options;
    const pop = $("#ribbonPopover");
    const anchor = activePopoverAnchor;
    if (!pop) return;
    pop.style.display = "none";
    pop.setAttribute("aria-hidden", "true");
    pop.removeAttribute("aria-label");
    pop.innerHTML = "";
    if (anchor) {
      anchor.classList.remove("popover-open");
      anchor.setAttribute("aria-expanded", "false");
    }
    activePopoverAnchor = null;
    if (restoreFocus && anchor && typeof anchor.focus === "function") {
      window.setTimeout(() => anchor.focus({ preventScroll: true }), 0);
    }
    syncAnalysisButtons();
  }

  function openRibbonPopover(anchor, contentEl) {
    const pop = $("#ribbonPopover");
    if (!pop || !anchor || !contentEl) return;
    closeRibbonPopover({ restoreFocus: false });
    pop.appendChild(contentEl);
    pop.style.display = "block";
    pop.setAttribute("aria-hidden", "false");
    pop.setAttribute("role", "dialog");
    pop.setAttribute("aria-modal", "false");
    pop.setAttribute("aria-label", describePopoverAnchor(anchor, contentEl));
    const rect = anchor.getBoundingClientRect();
    const popRect = pop.getBoundingClientRect();
    let left = rect.left;
    let top = rect.bottom + 6;
    if (left + popRect.width > window.innerWidth - 8) {
      left = window.innerWidth - popRect.width - 8;
    }
    if (left < 8) left = 8;
    if (top + popRect.height > window.innerHeight - 8) {
      top = rect.top - popRect.height - 6;
    }
    pop.style.left = `${Math.round(left)}px`;
    pop.style.top = `${Math.round(top)}px`;
    activePopoverAnchor = anchor;
    anchor.classList.add("popover-open");
    anchor.setAttribute("aria-expanded", "true");
    window.requestAnimationFrame(() => {
      if (activePopoverAnchor === anchor) {
        focusRibbonPopoverTarget(pop);
      }
    });
    syncAnalysisButtons();
  }

  function toggleRibbonPopover(anchor, builder) {
    const pop = $("#ribbonPopover");
    if (!pop || !anchor) return;
    if (activePopoverAnchor === anchor && pop.style.display === "block") {
      closeRibbonPopover({ restoreFocus: false });
      return;
    }
    const content = builder();
    openRibbonPopover(anchor, content);
  }

  function pushRecentColor(color) {
    if (!color) return;
    state.recentColors = [color, ...state.recentColors.filter((c) => c !== color)].slice(0, 8);
  }

  function getColorValue(target) {
    ensureRibbonStyle();
    if (target === "textColor") return state.ribbonStyle.textColor;
    if (target === "outlineColor") return state.ribbonStyle.edgeColor ?? state.ribbonStyle.nodeColor;
    if (target === "edgeColor") return state.ribbonStyle.edgeColor;
    if (target === "nodeFill") return state.ribbonStyle.nodeFill;
    if (target === "nodeColor") return state.ribbonStyle.nodeColor;
    return "#1f2937";
  }

  function normalizePopoverValue(value) {
    return String(value ?? "").trim().toLowerCase();
  }

  function describeColorTarget(target) {
    if (target === "textColor") return "文字颜色";
    if (target === "outlineColor") return "轮廓颜色";
    if (target === "edgeColor") return "线条颜色";
    if (target === "nodeFill") return "节点填充颜色";
    if (target === "nodeColor") return "节点颜色";
    return "颜色";
  }

  function buildColorChoiceLabel(color, target, options = {}) {
    const { recent = false } = options;
    const prefix = recent ? "最近使用" : "选择";
    return `${prefix}${describeColorTarget(target)} ${String(color || "").trim().toUpperCase()}`;
  }

  function buildIconChoiceLabel(categoryLabel, index) {
    return `${String(categoryLabel || "节点样式").trim()}图标 第${index + 1}款`;
  }

  function setPopoverOptionSelectionState(control, isSelected, options = {}) {
    const mode = String(options.mode || "toggle");
    if (!control) return;
    control.classList.toggle("active", !!isSelected);
    control.removeAttribute("role");
    control.removeAttribute("aria-pressed");
    control.removeAttribute("aria-selected");
    control.removeAttribute("aria-checked");
    control.removeAttribute("tabindex");
    if (mode === "radio") {
      control.setAttribute("role", "radio");
      control.setAttribute("aria-checked", isSelected ? "true" : "false");
      control.tabIndex = isSelected ? 0 : -1;
      return;
    }
    if (mode === "tab") {
      control.setAttribute("role", "tab");
      control.setAttribute("aria-selected", isSelected ? "true" : "false");
      control.tabIndex = isSelected ? 0 : -1;
      return;
    }
    control.setAttribute("aria-pressed", isSelected ? "true" : "false");
  }

  function applyColorTarget(target, color) {
    if (!color) return;
    pushRecentColor(color);
    if (target === "textColor") updateRibbonState({ textColor: color });
    if (target === "outlineColor") updateRibbonState({ edgeColor: color, nodeColor: color });
    if (target === "edgeColor") updateRibbonState({ edgeColor: color });
    if (target === "nodeFill") updateRibbonState({ nodeFill: color });
    if (target === "nodeColor") updateRibbonState({ nodeColor: color });
  }

  function buildColorPopover(target) {
    const wrap = document.createElement("div");
    const colorTargetLabel = describeColorTarget(target);
    const currentColor = normalizePopoverValue(getColorValue(target));
    wrap.dataset.popoverLabel = `${colorTargetLabel}选项`;
    const title = document.createElement("div");
    title.className = "popoverTitle";
    title.textContent = "常用色";
    const grid = document.createElement("div");
    grid.className = "colorGrid";
    grid.setAttribute("role", "radiogroup");
    grid.setAttribute("aria-label", `${colorTargetLabel} 常用色`);
    let initialFocusAssigned = false;
    COMMON_COLORS.forEach((color) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "colorCell";
      btn.style.setProperty("--swatch", color);
      const isSelected = normalizePopoverValue(color) === currentColor;
      setPopoverOptionSelectionState(btn, isSelected, { mode: "radio" });
      btn.setAttribute("aria-label", buildColorChoiceLabel(color, target));
      btn.title = `${colorTargetLabel} ${String(color || "").trim().toUpperCase()}`;
      if (!initialFocusAssigned && isSelected) {
        btn.dataset.popoverInitialFocus = "true";
        initialFocusAssigned = true;
      }
      btn.addEventListener("click", () => {
        applyColorTarget(target, color);
        closeRibbonPopover();
      });
      grid.appendChild(btn);
    });

    const recentTitle = document.createElement("div");
    recentTitle.className = "popoverTitle";
    recentTitle.textContent = "最近";
    const recentGrid = document.createElement("div");
    recentGrid.className = "colorGrid";
    recentGrid.setAttribute("role", "radiogroup");
    recentGrid.setAttribute("aria-label", `${colorTargetLabel} 最近颜色`);
    const recent = state.recentColors || [];
    for (let i = 0; i < 8; i += 1) {
      const color = recent[i];
      if (!color) {
        const empty = document.createElement("div");
        empty.className = "colorCell empty";
        recentGrid.appendChild(empty);
        continue;
      }
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "colorCell";
      btn.style.setProperty("--swatch", color);
      const isSelected = normalizePopoverValue(color) === currentColor;
      setPopoverOptionSelectionState(btn, isSelected, { mode: "radio" });
      btn.setAttribute("aria-label", buildColorChoiceLabel(color, target, { recent: true }));
      btn.title = `${colorTargetLabel} ${String(color || "").trim().toUpperCase()}`;
      if (!initialFocusAssigned && isSelected) {
        btn.dataset.popoverInitialFocus = "true";
        initialFocusAssigned = true;
      }
      btn.addEventListener("click", () => {
        applyColorTarget(target, color);
        closeRibbonPopover();
      });
      recentGrid.appendChild(btn);
    }

    const customRow = document.createElement("div");
    customRow.className = "colorCustomRow";
    const input = document.createElement("input");
    input.type = "color";
    input.value = getColorValue(target) || "#1f2937";
    input.setAttribute("aria-label", `${colorTargetLabel} 自定义颜色`);
    if (!initialFocusAssigned) {
      input.dataset.popoverInitialFocus = "true";
    }
    input.addEventListener("input", (e) => {
      applyColorTarget(target, e.target.value);
    });
    const label = document.createElement("div");
    label.textContent = "自定义";
    label.style.fontSize = "12px";
    label.style.color = "var(--flow-toolbar-text-muted)";
    customRow.appendChild(input);
    customRow.appendChild(label);

    wrap.appendChild(title);
    wrap.appendChild(grid);
    wrap.appendChild(recentTitle);
    wrap.appendChild(recentGrid);
    wrap.appendChild(customRow);
    return wrap;
  }

  function buildListPopover(options, currentValue, onSelect) {
    const wrap = document.createElement("div");
    wrap.className = "popoverList";
    wrap.setAttribute("role", "radiogroup");
    options.forEach((opt) => {
      const value = typeof opt === "object" ? opt.value : opt;
      const label = typeof opt === "object" ? opt.label : String(opt);
      const item = document.createElement("button");
      item.type = "button";
      item.className = "popoverItem";
      const isSelected = String(value) === String(currentValue);
      setPopoverOptionSelectionState(item, isSelected, { mode: "radio" });
      if (isSelected) {
        item.dataset.popoverInitialFocus = "true";
      }
      item.textContent = label;
      item.addEventListener("click", () => {
        onSelect(value);
        closeRibbonPopover();
      });
      wrap.appendChild(item);
    });
    return wrap;
  }

  function buildLineStylePopover(options, currentValue, onSelect) {
    const wrap = document.createElement("div");
    wrap.className = "popoverList";
    wrap.setAttribute("role", "radiogroup");
    options.forEach((opt) => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "popoverItem withIcon";
      const isSelected = String(opt.value) === String(currentValue);
      setPopoverOptionSelectionState(item, isSelected, { mode: "radio" });
      if (isSelected) {
        item.dataset.popoverInitialFocus = "true";
      }

      const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
      svg.setAttribute("viewBox", "0 0 56 20");
      svg.setAttribute("class", "popoverIcon popoverIcon--line");
      const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
      line.setAttribute("x1", "4");
      line.setAttribute("y1", "10");
      line.setAttribute("x2", "52");
      line.setAttribute("y2", "10");
      line.setAttribute("stroke", "currentColor");
      line.setAttribute("stroke-width", "2.25");
      line.setAttribute("stroke-linecap", "round");
      if (Array.isArray(opt.dash) && opt.dash.length) {
        line.setAttribute("stroke-dasharray", opt.dash.join(" "));
      }
      svg.appendChild(line);

      const label = document.createElement("span");
      label.className = "popoverLabel";
      label.textContent = opt.label || String(opt.value);

      item.appendChild(svg);
      item.appendChild(label);
      item.addEventListener("click", () => {
        onSelect(opt.value);
        closeRibbonPopover();
      });
      wrap.appendChild(item);
    });
    return wrap;
  }

  function buildShapePopover(options, currentValue, onSelect) {
    const wrap = document.createElement("div");
    wrap.className = "popoverList";
    wrap.setAttribute("role", "radiogroup");
    options.forEach((opt) => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "popoverItem withIcon";
      const isSelected = String(opt.value) === String(currentValue);
      setPopoverOptionSelectionState(item, isSelected, { mode: "radio" });
      if (isSelected) {
        item.dataset.popoverInitialFocus = "true";
      }

      const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
      svg.setAttribute("viewBox", "0 0 24 24");
      svg.setAttribute("class", "popoverIcon popoverIcon--shape");
      svg.setAttribute("fill", "none");
      svg.setAttribute("stroke", "currentColor");
      svg.setAttribute("stroke-width", "2.25");
      svg.setAttribute("stroke-linecap", "round");
      svg.setAttribute("stroke-linejoin", "round");

      const addPath = (d) => {
        const p = document.createElementNS("http://www.w3.org/2000/svg", "path");
        p.setAttribute("d", d);
        svg.appendChild(p);
      };
      const regularPolygon = (sides, r, cx = 24, cy = 6, rot = -Math.PI / 2) => {
        const pts = [];
        for (let i = 0; i < sides; i += 1) {
          const ang = rot + (Math.PI * 2 * i) / sides;
          pts.push([cx + Math.cos(ang) * r, cy + Math.sin(ang) * r]);
        }
        return pts;
      };
      const pathFromPts = (pts) => {
        if (!pts.length) return "";
        const [x0, y0] = pts[0];
        let d = `M${x0.toFixed(2)} ${y0.toFixed(2)}`;
        for (let i = 1; i < pts.length; i += 1) {
          const [x, y] = pts[i];
          d += ` L${x.toFixed(2)} ${y.toFixed(2)}`;
        }
        return d + " Z";
      };
      const starPath = (cx = 12, cy = 12, outer = 7.8, inner = 3.6, rot = -Math.PI / 2) => {
        const pts = [];
        for (let i = 0; i < 10; i += 1) {
          const ang = rot + (Math.PI * i) / 5;
          const r = i % 2 === 0 ? outer : inner;
          pts.push([cx + Math.cos(ang) * r, cy + Math.sin(ang) * r]);
        }
        return pathFromPts(pts);
      };

      if (opt.value === "circle") {
        const c = document.createElementNS("http://www.w3.org/2000/svg", "circle");
        c.setAttribute("cx", "12");
        c.setAttribute("cy", "12");
        c.setAttribute("r", "7");
        svg.appendChild(c);
      } else if (opt.value === "rect") {
        const r = document.createElementNS("http://www.w3.org/2000/svg", "rect");
        r.setAttribute("x", "5");
        r.setAttribute("y", "5.5");
        r.setAttribute("width", "14");
        r.setAttribute("height", "13");
        r.setAttribute("rx", "1.75");
        svg.appendChild(r);
      } else if (opt.value === "roundrect") {
        const r = document.createElementNS("http://www.w3.org/2000/svg", "rect");
        r.setAttribute("x", "4.5");
        r.setAttribute("y", "5");
        r.setAttribute("width", "15");
        r.setAttribute("height", "14");
        r.setAttribute("rx", "4.75");
        svg.appendChild(r);
      } else if (opt.value === "diamond") {
        addPath("M12 4 L19.5 12 L12 20 L4.5 12 Z");
      } else if (opt.value === "pill") {
        const r = document.createElementNS("http://www.w3.org/2000/svg", "rect");
        r.setAttribute("x", "3.5");
        r.setAttribute("y", "7");
        r.setAttribute("width", "17");
        r.setAttribute("height", "10");
        r.setAttribute("rx", "5");
        svg.appendChild(r);
      } else if (opt.value === "triangle") {
        addPath(pathFromPts(regularPolygon(3, 8, 12, 12)));
      } else if (opt.value === "pentagon") {
        addPath(pathFromPts(regularPolygon(5, 7.6, 12, 12)));
      } else if (opt.value === "hexagon") {
        addPath(pathFromPts(regularPolygon(6, 7.6, 12, 12)));
      } else if (opt.value === "octagon") {
        addPath(pathFromPts(regularPolygon(8, 7.4, 12, 12)));
      } else if (opt.value === "star") {
        addPath(starPath());
      }

      const label = document.createElement("span");
      label.className = "popoverLabel";
      label.textContent = opt.label || String(opt.value);

      item.appendChild(svg);
      item.appendChild(label);
      item.addEventListener("click", () => {
        onSelect(opt.value);
        closeRibbonPopover();
      });
      wrap.appendChild(item);
    });
    return wrap;
  }

  function buildArrowPopover(options, currentValue, onSelect) {
    const wrap = document.createElement("div");
    wrap.className = "popoverList";
    wrap.setAttribute("role", "radiogroup");
    options.forEach((opt) => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "popoverItem withIcon";
      const isSelected = String(opt.value) === String(currentValue);
      setPopoverOptionSelectionState(item, isSelected, { mode: "radio" });
      if (isSelected) {
        item.dataset.popoverInitialFocus = "true";
      }

      const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
      svg.setAttribute("viewBox", "0 0 56 20");
      svg.setAttribute("class", "popoverIcon popoverIcon--arrow");
      const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
      line.setAttribute("x1", "7");
      line.setAttribute("y1", "10");
      line.setAttribute("x2", "49");
      line.setAttribute("y2", "10");
      line.setAttribute("stroke", "currentColor");
      line.setAttribute("stroke-width", "2.25");
      line.setAttribute("stroke-linecap", "round");
      svg.appendChild(line);

      const addArrow = (x, dir) => {
        const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
        if (dir === "left") {
          path.setAttribute("d", `M ${x} 10 L ${x + 8} 5 L ${x + 8} 15 Z`);
        } else {
          path.setAttribute("d", `M ${x} 10 L ${x - 8} 5 L ${x - 8} 15 Z`);
        }
        path.setAttribute("fill", "currentColor");
        svg.appendChild(path);
      };

      if (opt.value === "end" || opt.value === "both") addArrow(49, "right");
      if (opt.value === "start" || opt.value === "both") addArrow(7, "left");

      const label = document.createElement("span");
      label.className = "popoverLabel";
      label.textContent = opt.label || String(opt.value);

      item.appendChild(svg);
      item.appendChild(label);
      item.addEventListener("click", () => {
        onSelect(opt.value);
        closeRibbonPopover();
      });
      wrap.appendChild(item);
    });
    return wrap;
  }

  function buildIconLibraryPopover() {
    const wrap = document.createElement("div");
    wrap.className = "iconPopover";
    wrap.dataset.popoverLabel = "节点样式库";
    const currentIconSymbol = String(state.ribbonStyle?.iconSymbol || "").trim();

    const tabs = document.createElement("div");
    tabs.className = "iconTabs";
    tabs.setAttribute("role", "tablist");
    tabs.setAttribute("aria-label", "节点样式分类");
    ICON_LIBRARY.forEach((cat) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "iconTab";
      btn.textContent = cat.label;
      setPopoverOptionSelectionState(btn, state.iconLibrary.tab === cat.id, { mode: "tab" });
      btn.addEventListener("click", () => {
        state.iconLibrary.tab = cat.id;
        state.iconLibrary.query = "";
        renderIcons();
        tabs.querySelectorAll(".iconTab").forEach((t) => setPopoverOptionSelectionState(t, t === btn, { mode: "tab" }));
      });
      tabs.appendChild(btn);
    });

    const searchWrap = document.createElement("div");
    searchWrap.className = "iconSearch";
    const input = document.createElement("input");
    input.type = "search";
    input.placeholder = "搜索图标";
    input.value = state.iconLibrary.query || "";
    input.autocomplete = "off";
    input.setAttribute("aria-label", "搜索节点样式图标");
    input.dataset.popoverInitialFocus = "true";
    input.addEventListener("input", (e) => {
      state.iconLibrary.query = e.target.value;
      renderIcons();
    });
    searchWrap.appendChild(input);

    const grid = document.createElement("div");
    grid.className = "iconGrid";

    function renderIcons() {
      const tab = state.iconLibrary.tab || "company";
      const query = String(state.iconLibrary.query || "").trim().toLowerCase();
      const cat = ICON_LIBRARY.find((c) => c.id === tab) || ICON_LIBRARY[0];
      const icons = (cat?.icons || []).filter((id) => !query || id.toLowerCase().includes(query));
      grid.setAttribute("role", "radiogroup");
      grid.setAttribute("aria-label", `${cat?.label || "节点样式"}图标列表`);
      grid.innerHTML = "";
      icons.forEach((id, index) => {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "iconBtn";
        const isSelected = currentIconSymbol === id;
        const iconLabel = buildIconChoiceLabel(cat?.label || "节点样式", index);
        setPopoverOptionSelectionState(btn, isSelected, { mode: "radio" });
        btn.setAttribute("aria-label", iconLabel);
        btn.title = iconLabel;
        btn.innerHTML = `<svg class="ico"><use href="#${id}"></use></svg>`;
        btn.addEventListener("click", () => {
          updateRibbonState({ iconSymbol: id });
          closeRibbonPopover();
        });
        grid.appendChild(btn);
      });
    }

    renderIcons();
    wrap.appendChild(tabs);
    wrap.appendChild(searchWrap);
    wrap.appendChild(grid);
    return wrap;
  }

  function applyReactStylePopoverOption(value) {
    const sourceKey = String(state.reactOverlay?.stylePopover?.sourceKey || "");
    if (!sourceKey) return;
    if (sourceKey === "style-font-family") {
      updateRibbonState({ fontFamily: value });
    } else if (sourceKey === "style-font-size") {
      updateRibbonState({ fontSize: Number(value) });
    } else if (sourceKey === "style-line-style") {
      updateRibbonState({ edgeDash: value });
    } else if (sourceKey === "style-line-width") {
      updateRibbonState({ edgeWidth: Number(value) });
    } else if (sourceKey === "style-line-arrow") {
      const graph = ensureGraph();
      if (!canEditEdgeDirection(graph)) {
        toast("统计分析展示模式下禁止修改方向", "warn");
        return;
      }
      updateRibbonState({ edgeArrow: value });
    } else if (sourceKey === "style-node-shape") {
      updateRibbonState({ nodeShape: value });
    } else if (sourceKey === "style-icon-size") {
      updateRibbonState({ nodeSize: Number(value) });
    } else {
      return;
    }
    closeReactStylePopover({ sync: false });
    scheduleShellStateSync("react-style-popover-select");
  }

  function applyReactStylePopoverColor(color, closeAfter = false) {
    const sourceKey = String(state.reactOverlay?.stylePopover?.sourceKey || "");
    if (!sourceKey) return;
    if (sourceKey === "style-text-color") {
      applyColorTarget("textColor", color);
    } else if (sourceKey === "style-outline-color") {
      applyColorTarget("outlineColor", color);
    } else if (sourceKey === "style-node-fill") {
      applyColorTarget("nodeFill", color);
    } else {
      return;
    }
    if (closeAfter) {
      closeReactStylePopover({ sync: false });
      scheduleShellStateSync("react-style-popover-color-close");
      return;
    }
    scheduleShellStateSync("react-style-popover-color");
  }

  function setReactStyleIconLibraryTab(tabId) {
    state.iconLibrary.tab = String(tabId || "company") || "company";
    state.iconLibrary.query = "";
    scheduleShellStateSync("react-style-icon-tab");
  }

  function setReactStyleIconLibraryQuery(query) {
    state.iconLibrary.query = String(query || "");
    scheduleShellStateSync("react-style-icon-query");
  }

  function applyReactStyleIconSymbol(iconId) {
    updateRibbonState({ iconSymbol: iconId });
    closeReactStylePopover({ sync: false });
    scheduleShellStateSync("react-style-icon-select");
  }

  function buildExportPopover() {
    const wrap = document.createElement("div");
    wrap.dataset.popoverLabel = "导出选项";
    const title = document.createElement("div");
    title.className = "popoverTitle";
    title.textContent = "导出格式";
    const grid = document.createElement("div");
    grid.className = "exportGrid";
    grid.setAttribute("role", "radiogroup");
    grid.setAttribute("aria-label", "导出格式");
    ["png", "jpg", "pdf"].forEach((fmt) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "popoverItem";
      const isDefaultFormat = fmt === "png";
      setPopoverOptionSelectionState(btn, isDefaultFormat, { mode: "radio" });
      btn.setAttribute("aria-label", `导出为 ${fmt.toUpperCase()}`);
      if (isDefaultFormat) {
        btn.dataset.popoverInitialFocus = "true";
      }
      btn.textContent = fmt.toUpperCase();
      btn.addEventListener("click", () => {
        exportCurrentView(fmt, state.exportScale || 1);
        closeRibbonPopover();
      });
      grid.appendChild(btn);
    });

    const scaleRow = document.createElement("div");
    scaleRow.className = "exportScale";
    scaleRow.setAttribute("role", "radiogroup");
    scaleRow.setAttribute("aria-label", "导出倍率");
    [1, 2, 4].forEach((scale) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = `${scale}x`;
      btn.setAttribute("aria-label", `导出倍率 ${scale} 倍`);
      setPopoverOptionSelectionState(btn, (state.exportScale || 1) === scale, { mode: "radio" });
      btn.addEventListener("click", () => {
        state.exportScale = scale;
        scaleRow.querySelectorAll("button").forEach((b) => setPopoverOptionSelectionState(b, b === btn, { mode: "radio" }));
      });
      scaleRow.appendChild(btn);
    });

    wrap.appendChild(title);
    wrap.appendChild(grid);
    wrap.appendChild(scaleRow);
    return wrap;
  }

  function isReactShellSubscribed() {
    return shellBridgeListeners.size > 0;
  }

  function closeReactFilterOverlay({ sync = true } = {}) {
    if (!state.reactOverlay?.filter?.open && !state.reactOverlay?.filter?.anchorRect) return;
    state.reactOverlay.filter.open = false;
    state.reactOverlay.filter.anchorRect = null;
    if (sync) scheduleShellStateSync("react-filter-close");
  }

  function closeReactStylePopover({ sync = true } = {}) {
    if (!state.reactOverlay?.stylePopover?.open && !state.reactOverlay?.stylePopover?.anchorRect) return;
    state.reactOverlay.stylePopover.open = false;
    state.reactOverlay.stylePopover.anchorRect = null;
    state.reactOverlay.stylePopover.sourceKey = "";
    if (sync) scheduleShellStateSync("react-style-popover-close");
  }

  const REACT_OVERLAY_CLOSE_GUARD_MS = 220;

  function toggleReactStylePopover(sourceKey, anchorRect) {
    const rect = normalizeAnchorRect(anchorRect);
    const key = String(sourceKey || "").trim();
    if (!rect || !key) {
      closeReactStylePopover();
      return;
    }
    const nextOpen = !(state.reactOverlay?.stylePopover?.open && state.reactOverlay?.stylePopover?.sourceKey === key);
    closeRibbonPopover();
    closeReactFilterOverlay({ sync: false });
    closeReactToolbarMenu({ sync: false });
    closeReactExportOverlay({ sync: false });
    closeContextMenu({ sync: false });
    closeReactStylePopover({ sync: false });
    if (!nextOpen) {
      scheduleShellStateSync("react-style-popover-close");
      return;
    }
    state.reactOverlay.suppressCloseUntil = Date.now() + REACT_OVERLAY_CLOSE_GUARD_MS;
    state.reactOverlay.stylePopover.open = true;
    state.reactOverlay.stylePopover.anchorRect = rect;
    state.reactOverlay.stylePopover.sourceKey = key;
    scheduleShellStateSync("react-style-popover-open");
  }

  function closeReactToolbarMenu({ sync = true } = {}) {
    reactContextMenuActions.clear();
    const hasItems = Array.isArray(state.reactOverlay?.menu?.items) && state.reactOverlay.menu.items.length > 0;
    if (!state.reactOverlay?.menu?.open && !state.reactOverlay?.menu?.anchorRect && !hasItems) return;
    state.reactOverlay.menu.open = false;
    state.reactOverlay.menu.anchorRect = null;
    state.reactOverlay.menu.sourceKey = "";
    state.reactOverlay.menu.items = [];
    if (sync) scheduleShellStateSync("react-toolbar-menu-close");
  }

  function toggleReactToolbarMenu(sourceKey, anchorRect, items) {
    const rect = normalizeAnchorRect(anchorRect);
    const key = String(sourceKey || "").trim();
    const rows = Array.isArray(items) ? items : [];
    if (!rect || !key || !rows.length) {
      closeReactToolbarMenu();
      return;
    }
    const nextOpen = !(state.reactOverlay?.menu?.open && state.reactOverlay?.menu?.sourceKey === key);
    closeRibbonPopover();
    closeReactFilterOverlay({ sync: false });
    closeReactStylePopover({ sync: false });
    closeReactExportOverlay({ sync: false });
    closeContextMenu({ sync: false });
    closeReactToolbarMenu({ sync: false });
    if (!nextOpen) {
      scheduleShellStateSync("react-toolbar-menu-close");
      return;
    }
    state.reactOverlay.suppressCloseUntil = Date.now() + REACT_OVERLAY_CLOSE_GUARD_MS;
    state.reactOverlay.menu.open = true;
    state.reactOverlay.menu.anchorRect = rect;
    state.reactOverlay.menu.sourceKey = key;
    state.reactOverlay.menu.items = buildReactContextMenuItems(rows);
    scheduleShellStateSync("react-toolbar-menu-open");
  }

  function toggleReactFilterOverlay(anchorRect) {
    const rect = normalizeAnchorRect(anchorRect);
    if (!rect) return;
    const nextOpen = !state.reactOverlay?.filter?.open;
    closeRibbonPopover();
    closeReactStylePopover({ sync: false });
    closeReactExportOverlay({ sync: false });
    closeContextMenu({ sync: false });
    closeReactToolbarMenu({ sync: false });
    if (!nextOpen) {
      closeReactFilterOverlay({ sync: true });
      return;
    }
    state.reactOverlay.suppressCloseUntil = Date.now() + REACT_OVERLAY_CLOSE_GUARD_MS;
    state.reactOverlay.filter.open = true;
    state.reactOverlay.filter.anchorRect = rect;
    scheduleShellStateSync("react-filter-open");
  }

  function closeReactExportOverlay({ sync = true } = {}) {
    if (!state.reactOverlay?.exportPanel?.open && !state.reactOverlay?.exportPanel?.anchorRect) return;
    state.reactOverlay.exportPanel.open = false;
    state.reactOverlay.exportPanel.anchorRect = null;
    if (sync) scheduleShellStateSync("react-export-close");
  }

  function toggleReactExportOverlay(anchorRect) {
    const rect = normalizeAnchorRect(anchorRect);
    if (!rect) return;
    const nextOpen = !state.reactOverlay?.exportPanel?.open;
    closeRibbonPopover();
    closeReactFilterOverlay({ sync: false });
    closeReactStylePopover({ sync: false });
    closeContextMenu({ sync: false });
    closeReactToolbarMenu({ sync: false });
    if (!nextOpen) {
      closeReactExportOverlay({ sync: true });
      return;
    }
    state.reactOverlay.suppressCloseUntil = Date.now() + REACT_OVERLAY_CLOSE_GUARD_MS;
    state.reactOverlay.exportPanel.open = true;
    state.reactOverlay.exportPanel.anchorRect = rect;
    scheduleShellStateSync("react-export-open");
  }

  function runReactContextMenuAction(actionId) {
    const id = String(actionId || "").trim();
    if (!id) return;
    const menuItem = (state.reactOverlay?.contextMenu?.items || []).find((item) => String(item?.id || "") === id) || null;
    const action = reactContextMenuActions.get(id);
    viewPersistenceDebug.lastContextAction = {
      actionId: id,
      label: ordinaryPiiProjection.projectDetected(menuItem?.label || ""),
      hasAction: typeof action === "function",
      startedAt: nowIso(),
      status: "started",
    };
    closeContextMenu({ sync: false });
    closeReactToolbarMenu({ sync: false });
    scheduleShellStateSync("react-overlay-action");
    if (typeof action !== "function") return;
    try {
      const result = action();
      if (result && typeof result.then === "function") {
        result
          .then(() => {
            viewPersistenceDebug.lastContextAction = {
              ...(viewPersistenceDebug.lastContextAction || {}),
              finishedAt: nowIso(),
              status: "resolved",
            };
            scheduleShellStateSync("react-overlay-action-complete");
          })
          .catch(() => {
            viewPersistenceDebug.lastContextAction = {
              ...(viewPersistenceDebug.lastContextAction || {}),
              finishedAt: nowIso(),
              status: "rejected",
            };
            scheduleShellStateSync("react-overlay-action-error");
          });
      } else {
        viewPersistenceDebug.lastContextAction = {
          ...(viewPersistenceDebug.lastContextAction || {}),
          finishedAt: nowIso(),
          status: "completed",
        };
        scheduleShellStateSync("react-overlay-action-complete");
      }
    } catch (e) {}
  }

  function buildReactDetailRow(label, value, options = {}) {
    if (shellOverlayAdapter && typeof shellOverlayAdapter.buildDetailRow === "function") {
      return shellOverlayAdapter.buildDetailRow(label, value, options);
    }
    return {
      label: String(label || ""),
      value: String(value == null ? "" : value),
      mono: !!options.mono,
      tone: String(options.tone || ""),
    };
  }

  function closeReactDrawer({ sync = true } = {}) {
    closeReactDrawerState(state.reactOverlay, reactDrawerActions, scheduleShellStateSync, { sync });
  }

  function openReactDrawer(payload, actionEntries = [], reason = "react-drawer-open") {
    openReactDrawerState(state.reactOverlay, reactDrawerActions, scheduleShellStateSync, payload, actionEntries, reason);
  }

  function runReactDrawerAction(actionId) {
    runReactDrawerActionModel(reactDrawerActions, actionId);
  }

  function buildReactNodeDetailDrawer(model, graph) {
    const node = model || {};
    const degree = (() => {
      try {
        const item = graph?.findById?.(String(node.id || "").trim());
        if (!item) return "";
        return String((item.getEdges?.() || []).length || "");
      } catch (e) {
        return "";
      }
    })();
    if (shellOverlayAdapter && typeof shellOverlayAdapter.buildNodeDetailDrawer === "function") {
      return shellOverlayAdapter.buildNodeDetailDrawer(
        node,
        {
          degree,
          typeLabel: String(node.ntype || node.type || "").trim(),
        },
        {
          formatMoney: fmtMoney,
        }
      );
    }
    const id = String(node.id || "").trim();
    return buildNodeDetailDrawerModel(node, { degree, id }, { buildDetailRow: buildReactDetailRow });
  }

  function buildReactEdgeDetailDrawer(model) {
    if (shellOverlayAdapter && typeof shellOverlayAdapter.buildEdgeDetailDrawer === "function") {
      return shellOverlayAdapter.buildEdgeDetailDrawer(model, {
        resolveEdgeLane,
        edgeAmountTotal,
        edgeAmountByLane,
        formatRange: fmtShortRange,
        formatMoney: fmtMoney,
      });
    }
    const edge = model || {};
    return {
      kind: "edgeDetail",
      title: String(edge.label || edge.id || "连线详情"),
      rows: [buildReactDetailRow("ID", String(edge.id || "").trim(), { mono: true })],
      actions: [],
      meta: null,
    };
  }

  function buildReactNodeInfoState(model) {
    if (shellOverlayAdapter && typeof shellOverlayAdapter.buildNodeInfoPayload === "function") {
      return shellOverlayAdapter.buildNodeInfoPayload(model, state.graphStyle || {}, {
        getNodeAccountList,
        getNodeCategoryName,
        getNodeUserName,
        getNodeAccount,
      });
    }
    return {
      preview: null,
      category: "",
      userName: "",
      accountLabel: "卡号",
      accountValue: "",
      accountValueMono: true,
      accounts: [],
      moreCount: 0,
    };
  }

  function openReactNodeInfo(point, payload, reason = "react-node-info-open") {
    const nextPoint = point && typeof point === "object" ? { x: Number(point.x) || 0, y: Number(point.y) || 0 } : null;
    const next = payload && typeof payload === "object" ? payload : {};
    state.reactOverlay.nodeInfo.open = true;
    state.reactOverlay.nodeInfo.point = nextPoint;
    state.reactOverlay.nodeInfo.preview = next.preview ? { ...next.preview } : null;
    state.reactOverlay.nodeInfo.category = String(next.category || "");
    state.reactOverlay.nodeInfo.userName = String(next.userName || "");
    state.reactOverlay.nodeInfo.accountLabel = String(next.accountLabel || "");
    state.reactOverlay.nodeInfo.accountValue = String(next.accountValue || "");
    state.reactOverlay.nodeInfo.accountValueMono = !!next.accountValueMono;
    state.reactOverlay.nodeInfo.accounts = Array.isArray(next.accounts) ? next.accounts.slice() : [];
    state.reactOverlay.nodeInfo.moreCount = Math.max(0, Number(next.moreCount) || 0);
    scheduleShellStateSync(reason);
  }

  function closeReactNodeInfo({ sync = true } = {}) {
    if (!state.reactOverlay?.nodeInfo?.open && !state.reactOverlay?.nodeInfo?.point) return;
    state.reactOverlay.nodeInfo.open = false;
    state.reactOverlay.nodeInfo.point = null;
    state.reactOverlay.nodeInfo.preview = null;
    state.reactOverlay.nodeInfo.category = "";
    state.reactOverlay.nodeInfo.userName = "";
    state.reactOverlay.nodeInfo.accountLabel = "";
    state.reactOverlay.nodeInfo.accountValue = "";
    state.reactOverlay.nodeInfo.accountValueMono = false;
    state.reactOverlay.nodeInfo.accounts = [];
    state.reactOverlay.nodeInfo.moreCount = 0;
    if (sync) scheduleShellStateSync("react-node-info-close");
  }

  function resolveEdgeTxnReadonlyScrollableRows(rows) {
    const source = Array.isArray(rows) ? rows : [];
    return resolveEdgeTxnReadonlyPreviewRows(source, source.length);
  }

  function buildReactEdgeInfoState(model, lane = "", rows = [], opts = {}) {
    if (shellOverlayAdapter && typeof shellOverlayAdapter.buildEdgeTxnOverlayPayload === "function") {
      return shellOverlayAdapter.buildEdgeTxnOverlayPayload(model, lane, rows, opts, {
        isSignedEdgeDetailEnabled,
        edgeSignedMeta,
        formatSignedMoney,
        formatMoney: fmtMoney,
        normalizeTxnTimeText,
        buildRequestKey: buildEdgeInfoRequestKey,
        resolveEdgeTxnReadonlyPreviewRows: resolveEdgeTxnReadonlyScrollableRows,
      });
    }
    return {
      title: "资金交易",
      amountText: "",
      amountTone: "",
      countText: "0",
      countClickable: false,
      loading: false,
      error: "",
      rows: [],
      requestKey: "",
    };
  }

  function openReactEdgeInfo(point, payload, reason = "react-edge-info-open") {
    const nextPoint = point && typeof point === "object" ? { x: Number(point.x) || 0, y: Number(point.y) || 0 } : null;
    const next = payload && typeof payload === "object" ? payload : {};
    state.reactOverlay.edgeInfo.open = true;
    state.reactOverlay.edgeInfo.point = nextPoint;
    state.reactOverlay.edgeInfo.requestKey = String(next.requestKey || "");
    state.reactOverlay.edgeInfo.title = String(next.title || "资金交易");
    state.reactOverlay.edgeInfo.amountText = String(next.amountText || "");
    state.reactOverlay.edgeInfo.amountTone = String(next.amountTone || "");
    state.reactOverlay.edgeInfo.countText = String(next.countText || "0");
    state.reactOverlay.edgeInfo.countClickable = !!next.countClickable;
    state.reactOverlay.edgeInfo.loading = !!next.loading;
    state.reactOverlay.edgeInfo.error = String(next.error || "");
    state.reactOverlay.edgeInfo.rows = Array.isArray(next.rows) ? next.rows.map((row) => ({ ...row })) : [];
    scheduleShellStateSync(reason);
  }

  function closeReactEdgeInfo({ sync = true } = {}) {
    if (!state.reactOverlay?.edgeInfo?.open && !state.reactOverlay?.edgeInfo?.point) return;
    state.reactOverlay.edgeInfo.open = false;
    state.reactOverlay.edgeInfo.point = null;
    state.reactOverlay.edgeInfo.requestKey = "";
    state.reactOverlay.edgeInfo.title = "资金交易";
    state.reactOverlay.edgeInfo.amountText = "";
    state.reactOverlay.edgeInfo.amountTone = "";
    state.reactOverlay.edgeInfo.countText = "0";
    state.reactOverlay.edgeInfo.countClickable = false;
    state.reactOverlay.edgeInfo.loading = false;
    state.reactOverlay.edgeInfo.error = "";
    state.reactOverlay.edgeInfo.rows = [];
    if (sync) scheduleShellStateSync("react-edge-info-close");
  }

  function buildReactEdgeLabelReadonlyState(model, lane = "", rows = [], opts = {}) {
    const payload = buildReactEdgeInfoState(model, lane, rows, opts);
    return {
      open: true,
      title: String(payload.title || "资金交易"),
      amountText: String(payload.amountText || ""),
      amountTone: String(payload.amountTone || ""),
      countText: String(payload.countText || "0"),
      countClickable: !!payload.countClickable,
      loading: !!payload.loading,
      error: String(payload.error || ""),
      rows: Array.isArray(payload.rows) ? payload.rows.map((row) => ({ ...row })) : [],
    };
  }

  function openReactEdgeLabel(payload, reason = "react-edge-label-open") {
    const next = payload && typeof payload === "object" ? payload : {};
    state.reactOverlay.edgeLabel.open = true;
    state.reactOverlay.edgeLabel.editable = next.editable !== false;
    state.reactOverlay.edgeLabel.title = String(next.title || "编辑连线文本");
    state.reactOverlay.edgeLabel.value = String(next.value || "");
    state.reactOverlay.edgeLabel.placeholder = String(next.placeholder || "输入文本内容…");
    state.reactOverlay.edgeLabel.hint = String(next.hint || "");
    state.reactOverlay.edgeLabel.confirmLabel = String(next.confirmLabel || "确定");
    state.reactOverlay.edgeLabel.cancelLabel = String(
      next.cancelLabel || (next.editable === false ? "关闭" : "取消")
    );
    const readonly = next.readonly && typeof next.readonly === "object" ? next.readonly : {};
    state.reactOverlay.edgeLabel.readonly.open = !!readonly.open;
    state.reactOverlay.edgeLabel.readonly.title = String(readonly.title || "资金交易");
    state.reactOverlay.edgeLabel.readonly.amountText = String(readonly.amountText || "");
    state.reactOverlay.edgeLabel.readonly.amountTone = String(readonly.amountTone || "");
    state.reactOverlay.edgeLabel.readonly.countText = String(readonly.countText || "0");
    state.reactOverlay.edgeLabel.readonly.countClickable = !!readonly.countClickable;
    state.reactOverlay.edgeLabel.readonly.loading = !!readonly.loading;
    state.reactOverlay.edgeLabel.readonly.error = String(readonly.error || "");
    state.reactOverlay.edgeLabel.readonly.rows = Array.isArray(readonly.rows)
      ? readonly.rows.map((row) => ({ ...row }))
      : [];
    scheduleShellStateSync(reason);
  }

  function closeReactEdgeLabel({ sync = true, clearTarget = true } = {}) {
    const edgeLabel = state.reactOverlay?.edgeLabel || {};
    const hasReadonlyRows = Array.isArray(edgeLabel.readonly?.rows) && edgeLabel.readonly.rows.length > 0;
    if (!edgeLabel.open && !edgeLabel.value && !hasReadonlyRows) {
      if (clearTarget) state.edgeLabelTarget = null;
      return;
    }
    state.reactOverlay.edgeLabel.open = false;
    state.reactOverlay.edgeLabel.editable = true;
    state.reactOverlay.edgeLabel.title = "编辑连线文本";
    state.reactOverlay.edgeLabel.value = "";
    state.reactOverlay.edgeLabel.placeholder = "输入文本内容…";
    state.reactOverlay.edgeLabel.hint = "双击线条可再次编辑";
    state.reactOverlay.edgeLabel.confirmLabel = "确定";
    state.reactOverlay.edgeLabel.cancelLabel = "取消";
    state.reactOverlay.edgeLabel.readonly.open = false;
    state.reactOverlay.edgeLabel.readonly.title = "资金交易";
    state.reactOverlay.edgeLabel.readonly.amountText = "";
    state.reactOverlay.edgeLabel.readonly.amountTone = "";
    state.reactOverlay.edgeLabel.readonly.countText = "0";
    state.reactOverlay.edgeLabel.readonly.countClickable = false;
    state.reactOverlay.edgeLabel.readonly.loading = false;
    state.reactOverlay.edgeLabel.readonly.error = "";
    state.reactOverlay.edgeLabel.readonly.rows = [];
    if (clearTarget) state.edgeLabelTarget = null;
    if (sync) scheduleShellStateSync("react-edge-label-close");
  }

  function setReactEdgeLabelValue(value, { sync = true } = {}) {
    if (!state.reactOverlay?.edgeLabel) return;
    state.reactOverlay.edgeLabel.value = String(value == null ? "" : value);
    if (sync) scheduleShellStateSync("react-edge-label-value");
  }

  function toggleTxnModalSort(colKey) {
    const modal = state.txnModal;
    if (!toggleTxnModalSortState(modal, colKey, isTxnSortableCol)) return;
    scheduleShellStateSync("react-txn-modal-sort");
  }

  function setTxnColumnWidth(colKey, width) {
    const key = String(colKey || "");
    const index = TXN_DETAIL_COLS.findIndex((column) => column.key === key);
    if (index < 0) return;
    const widths = ensureTxnDetailWidths();
    widths[index] = clampTxnColWidth(width);
    saveTxnDetailWidths();
    scheduleShellStateSync("react-txn-modal-width");
  }

  function buildReactContextMenuItems(items) {
    const source = Array.isArray(items) ? items : [];
    reactContextMenuActions.clear();
    return source.map((item, index) => {
      if (item?.type === "sep") {
        return {
          id: `sep-${index}`,
          label: "",
          disabled: false,
          separator: true,
        };
      }
      const id = `ctx-${++reactContextMenuSeq}`;
      if (typeof item?.action === "function") {
        reactContextMenuActions.set(id, item.action);
      }
      return {
        id,
        label: String(item?.label || ""),
        disabled: !!item?.disabled,
        separator: false,
      };
    });
  }

  function openContextMenuDom(x, y, items) {
    const menu = $("#contextMenu");
    if (!menu) return;
    menu.innerHTML = "";
    items.forEach((item) => {
      if (item.type === "sep") {
        const sep = document.createElement("div");
        sep.className = "ctxSep";
        menu.appendChild(sep);
        return;
      }
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "ctxItem";
      btn.textContent = item.label;
      if (item.disabled) btn.classList.add("disabled");
      if (!item.disabled) {
        btn.addEventListener("click", (ev) => {
          ev.preventDefault();
          ev.stopPropagation();
          closeContextMenu();
          item.action && item.action(ev);
        });
      }
      menu.appendChild(btn);
    });

    menu.style.display = "block";
    menu.setAttribute("aria-hidden", "false");
    const rect = menu.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const left = Math.max(10, Math.min(x, vw - rect.width - 10));
    const top = Math.max(10, Math.min(y, vh - rect.height - 10));
    menu.style.left = `${left}px`;
    menu.style.top = `${top}px`;
  }

  function getProjectionPointLayer(projection = state.graphProjection) {
    const source = projection && typeof projection === "object" && !Array.isArray(projection) ? projection : null;
    const pointLayer =
      source?.point_layer && typeof source.point_layer === "object" && !Array.isArray(source.point_layer)
        ? source.point_layer
        : source?.pointLayer && typeof source.pointLayer === "object" && !Array.isArray(source.pointLayer)
        ? source.pointLayer
        : null;
    if (!pointLayer) return null;
    const buckets = Array.isArray(pointLayer.buckets)
      ? pointLayer.buckets
          .map((raw) => (raw && typeof raw === "object" && !Array.isArray(raw) ? raw : null))
          .filter(Boolean)
          .map((bucket) => ({
            clusterId: String(bucket.cluster_id || bucket.clusterId || "").trim(),
            anchorId: String(bucket.anchor_id || bucket.anchorId || "").trim(),
            pointCount: Math.max(0, Number(bucket.point_count || bucket.pointCount || bucket.member_count || bucket.memberCount) || 0),
            totalAmount: Number(bucket.total_amount || bucket.totalAmount) || 0,
            totalCount: Math.max(0, Number(bucket.total_count || bucket.totalCount) || 0),
          }))
          .filter((bucket) => bucket.clusterId && bucket.pointCount > 0)
      : [];
    if (!buckets.length) return null;
    return {
      mode: String(pointLayer.mode || "clustered").trim().toLowerCase() || "clustered",
      totalPointCount: Math.max(
        0,
        Number(pointLayer.total_point_count || pointLayer.totalPointCount) || buckets.reduce((sum, bucket) => sum + bucket.pointCount, 0)
      ),
      bucketCount: buckets.length,
      buckets,
    };
  }

  function ensureProjectionPointLayerCanvas() {
    const graphContainer = $("#graphContainer");
    if (!graphContainer) return null;
    let canvas = state.pointLayerOverlay?.canvas || null;
    if (!canvas || !canvas.isConnected) {
      canvas = document.createElement("canvas");
      canvas.className = "graphPointLayer";
      canvas.setAttribute("aria-hidden", "true");
      if (graphContainer.firstChild) graphContainer.insertBefore(canvas, graphContainer.firstChild);
      else graphContainer.appendChild(canvas);
      state.pointLayerOverlay.canvas = canvas;
      state.pointLayerOverlay.ctx = null;
      state.pointLayerOverlay.width = 0;
      state.pointLayerOverlay.height = 0;
      state.pointLayerOverlay.dpr = 1;
    }
    if (!state.pointLayerOverlay.ctx) {
      state.pointLayerOverlay.ctx = canvas.getContext("2d", { alpha: true });
    }
    return canvas;
  }

  function resizeProjectionPointLayerCanvas() {
    const canvas = ensureProjectionPointLayerCanvas();
    const ctx = state.pointLayerOverlay?.ctx || canvas?.getContext?.("2d", { alpha: true }) || null;
    if (!canvas || !ctx) return null;
    const graphContainer = $("#graphContainer");
    const width = Math.max(0, graphContainer?.clientWidth || 0);
    const height = Math.max(0, graphContainer?.clientHeight || 0);
    if (!width || !height) return null;
    const dpr = clampNumber(window.devicePixelRatio || 1, 1, 2);
    if (
      width !== state.pointLayerOverlay.width ||
      height !== state.pointLayerOverlay.height ||
      Math.abs(dpr - (state.pointLayerOverlay.dpr || 1)) > 0.01
    ) {
      canvas.width = Math.max(1, Math.round(width * dpr));
      canvas.height = Math.max(1, Math.round(height * dpr));
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      state.pointLayerOverlay.width = width;
      state.pointLayerOverlay.height = height;
      state.pointLayerOverlay.dpr = dpr;
    }
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    return { canvas, ctx, width, height, dpr };
  }

  function clearProjectionPointLayer() {
    if (state.pointLayerOverlay?.raf) {
      cancelAnimationFrame(state.pointLayerOverlay.raf);
      state.pointLayerOverlay.raf = 0;
    }
    const canvas = state.pointLayerOverlay?.canvas || null;
    const ctx = state.pointLayerOverlay?.ctx || null;
    if (canvas && ctx) {
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      ctx.clearRect(0, 0, canvas.width || 0, canvas.height || 0);
      canvas.style.display = "none";
    }
  }

  function hashProjectionPointSeed(value) {
    const text = String(value || "");
    let hash = 2166136261 >>> 0;
    for (let i = 0; i < text.length; i += 1) {
      hash ^= text.charCodeAt(i);
      hash = Math.imul(hash, 16777619);
    }
    return hash >>> 0;
  }

  function drawProjectionPointLayer(graph = state.graph?.instance || null, reason = "") {
    const pointLayer = getProjectionPointLayer();
    if (!graph || !pointLayer || !pointLayer.buckets.length) {
      clearProjectionPointLayer();
      return;
    }
    const resized = resizeProjectionPointLayerCanvas();
    if (!resized) return;
    const { canvas, ctx, width, height } = resized;
    canvas.style.display = "block";
    ctx.clearRect(0, 0, width, height);
    const zoom = clampNumber(Number(graph.getZoom?.()) || 1, 0.05, 16);
    const pointSize = zoom < 0.16 ? 1 : zoom < 0.42 ? 1.15 : zoom < 0.85 ? 1.35 : 1.55;
    const alpha = zoom < 0.16 ? 0.22 : zoom < 0.42 ? 0.28 : zoom < 0.85 ? 0.34 : 0.42;
    const goldenAngle = Math.PI * (3 - Math.sqrt(5));
    ctx.fillStyle = `rgba(34, 197, 94, ${alpha})`;
    for (const bucket of pointLayer.buckets) {
      const item = graph.findById?.(bucket.clusterId) || (bucket.anchorId ? graph.findById?.(bucket.anchorId) : null);
      const model = item?.getModel?.() || null;
      const worldX = Number(model?.x);
      const worldY = Number(model?.y);
      if (!Number.isFinite(worldX) || !Number.isFinite(worldY)) continue;
      const center = graph.getCanvasByPoint?.(worldX, worldY) || { x: worldX, y: worldY };
      const centerX = Number(center?.x);
      const centerY = Number(center?.y);
      if (!Number.isFinite(centerX) || !Number.isFinite(centerY)) continue;
      const pointCount = Math.max(0, Math.round(bucket.pointCount || 0));
      if (!pointCount) continue;
      const radius = clampNumber(Math.sqrt(pointCount) * (zoom < 0.3 ? 1.4 : 1.9), 10, 120);
      if (centerX < -radius || centerX > width + radius || centerY < -radius || centerY > height + radius) continue;
      const phase = (hashProjectionPointSeed(bucket.clusterId || bucket.anchorId) % 6283) / 1000;
      for (let index = 0; index < pointCount; index += 1) {
        const ratio = Math.sqrt((index + 0.5) / pointCount);
        const angle = phase + index * goldenAngle;
        const px = centerX + Math.cos(angle) * radius * ratio;
        const py = centerY + Math.sin(angle) * radius * ratio;
        ctx.fillRect(Math.round(px), Math.round(py), pointSize, pointSize);
      }
    }
    try {
      if (state.debugPerf && pointLayer.totalPointCount > 0) {
        log("INFO", "projection point layer draw", {
          reason: String(reason || ""),
          points: pointLayer.totalPointCount,
          buckets: pointLayer.bucketCount,
          zoom,
        });
      }
    } catch (e) {}
  }

  function scheduleProjectionPointLayerRedraw(reason = "") {
    if (state.pointLayerOverlay?.raf) cancelAnimationFrame(state.pointLayerOverlay.raf);
    state.pointLayerOverlay.raf = requestAnimationFrame(() => {
      state.pointLayerOverlay.raf = 0;
      drawProjectionPointLayer(state.graph?.instance || null, reason);
    });
  }

  function snapshotGraphData(options = {}) {
    const preferStateData = !!options?.preferStateData;
    if (preferStateData || state.edgeBatching) {
      return cloneGraphData(state.graph.data);
    }
    const graph = state.graph.instance;
    if (graph && typeof graph.save === "function") {
      try {
        const saved = graph.save();
        if (saved && Array.isArray(saved.nodes)) {
          const snapshot = buildGraphSnapshotData(saved, {
            graphTier: state.graphTier || "",
            renderHints: state.renderHints,
            projection: state.graphProjection,
            resultSnapshotRef: state.resultSnapshotRef,
            runtimeRevision: state.graph.runtimeRevision,
          });
          const expectEdges = state.graph?.data?.edges?.length || 0;
          if (expectEdges && snapshot.edges.length < expectEdges) {
            return cloneGraphData(state.graph.data);
          }
          return snapshot;
        }
      } catch (e) {}
    }
    return cloneGraphData(state.graph.data);
  }

  const NODE_STYLE_KEYS = [
    "stroke",
    "fill",
    "lineWidth",
    "r",
    "textColor",
    "subTextColor",
    "icon",
    "iconSymbol",
    "iconSize",
    "nodeShape",
    "fontFamily",
    "fontSize",
    "fontBold",
    "fontItalic",
    "fontUnderline",
    "fontShadow",
    "nodeShadow",
  ];

  const EDGE_STYLE_KEYS = [
    "stroke",
    "lineWidth",
    "edgeDash",
    "edgeArrow",
    "mode",
    "textColor",
    "fontFamily",
    "fontSize",
    "fontBold",
    "fontItalic",
    "fontUnderline",
    "fontShadow",
    "gap",
    "holePad",
    "outPad",
    "showArrow",
  ];

  function pickStyle(obj, keys) {
    const out = {};
    keys.forEach((k) => {
      if (obj[k] != null) out[k] = obj[k];
    });
    return out;
  }

  function buildInitialStyleSnapshot(snapshot) {
    const graph = snapshot && typeof snapshot === "object" ? snapshot : { nodes: [], edges: [] };
    return {
      nodes: (graph.nodes || []).map((node) => ({ id: node?.id, ...pickStyle(node, NODE_STYLE_KEYS) })),
      edges: (graph.edges || []).map((edge) => ({ id: edge?.id, ...pickStyle(edge, EDGE_STYLE_KEYS) })),
      graphStyle: { ...state.graphStyle },
      ribbonStyle: { ...state.ribbonStyle },
    };
  }

  function buildUndoSnapshot() {
    ensureRibbonStyle();
    return {
      graph: snapshotGraphData(),
      graphStyle: { ...state.graphStyle },
      ribbonStyle: { ...state.ribbonStyle },
      analysis: {
        encodeWidth: !!state.analysis?.encodeWidth,
        encodeColor: !!state.analysis?.encodeColor,
        nodeScale: !!state.analysis?.nodeScale,
      },
    };
  }

  function schedulePostPaintTask(run, { timeout = 160 } = {}) {
    if (typeof run !== "function") return;
    const invoke = () => {
      try {
        run();
      } catch (e) {}
    };
    const scheduleIdle = () => {
      if (typeof requestIdleCallback === "function") {
        requestIdleCallback(() => invoke(), { timeout });
      } else {
        setTimeout(() => invoke(), 16);
      }
    };
    if (typeof requestAnimationFrame === "function") {
      requestAnimationFrame(() => scheduleIdle());
    } else {
      scheduleIdle();
    }
  }

  function resolveEdgeBatchSize(total = 0, ctx = state) {
    const count = Math.max(0, Number(total) || 0);
    const hints = getActiveGraphRenderHints(ctx, ctx?.graph?.data?.nodes?.length || 0, count);
    const hinted = Number(hints.edge_batch_size);
    if (Number.isFinite(hinted) && hinted > 0) return Math.max(48, Math.round(hinted));
    if (count >= 12000) return 220;
    if (count >= 6000) return 180;
    if (count >= 3000) return 140;
    if (count >= EDGE_BATCH_THRESHOLD) return 110;
    return EDGE_BATCH_SIZE;
  }

  function resolveEdgeRefreshBatchSize(total = 0, ctx = state) {
    const count = Math.max(0, Number(total) || 0);
    const hints = getActiveGraphRenderHints(ctx, ctx?.graph?.data?.nodes?.length || 0, count);
    const hinted = Number(hints.edge_refresh_batch_size);
    if (Number.isFinite(hinted) && hinted > 0) return Math.max(96, Math.round(hinted));
    return GRAPH_ITEM_REFRESH_BATCH_SIZE;
  }

  function scheduleGraphItemRefreshBatches(
    graph,
    items,
    { batchSize = GRAPH_ITEM_REFRESH_BATCH_SIZE, timeout = GRAPH_ITEM_REFRESH_BATCH_TIMEOUT_MS, isCancelled = null, onComplete = null } = {}
  ) {
    const rows = Array.isArray(items) ? items.filter(Boolean) : [];
    if (!graph || !rows.length) {
      if (typeof onComplete === "function") onComplete({ cancelled: false, duration: 0, batches: 0 });
      return false;
    }
    const size = Math.max(24, Number(batchSize) || GRAPH_ITEM_REFRESH_BATCH_SIZE);
    const waitMs = Math.max(48, Number(timeout) || GRAPH_ITEM_REFRESH_BATCH_TIMEOUT_MS);
    let index = 0;
    let batches = 0;
    const startedAt = performance.now();
    const applyBatch = () => {
      if (typeof isCancelled === "function" && isCancelled()) {
        if (typeof onComplete === "function") {
          onComplete({ cancelled: true, duration: Math.round((performance.now() - startedAt) * 10) / 10, batches });
        }
        return;
      }
      const upper = Math.min(rows.length, index + size);
      const canSetAutoPaint = typeof graph.setAutoPaint === "function";
      const prevAutoPaint = canSetAutoPaint ? graph.autoPaint !== false : true;
      if (canSetAutoPaint) graph.setAutoPaint(false);
      try {
        for (; index < upper; index += 1) {
          graph.refreshItem?.(rows[index]);
        }
      } catch (e) {}
      if (canSetAutoPaint) graph.setAutoPaint(prevAutoPaint);
      graph.paint?.();
      batches += 1;
      if (index < rows.length) {
        schedulePostPaintTask(applyBatch, { timeout: waitMs });
        return;
      }
      if (typeof onComplete === "function") {
        onComplete({ cancelled: false, duration: Math.round((performance.now() - startedAt) * 10) / 10, batches });
      }
    };
    schedulePostPaintTask(applyBatch, { timeout: waitMs });
    return true;
  }

  function syncUndoButtons() {
    const btnUndo = $("#btnUndo");
    if (btnUndo) {
      const ok = (state.undoStack || []).length > 0;
      btnUndo.disabled = !ok;
      btnUndo.setAttribute("aria-disabled", ok ? "false" : "true");
    }
    const btnRedo = $("#btnRedo");
    if (btnRedo) {
      const ok = !!state.initialStyleSnapshot;
      btnRedo.disabled = !ok;
      btnRedo.setAttribute("aria-disabled", ok ? "false" : "true");
    }
    scheduleShellStateSync("undo-buttons");
  }

  function recordUndoSnapshot() {
    if (state.undoLocked) return;
    const snapshot = buildUndoSnapshot();
    if (!snapshot.graph.nodes.length && !snapshot.graph.edges.length) return;
    state.undoStack = [...(state.undoStack || []), snapshot].slice(-30);
    syncUndoButtons();
  }

  function applySnapshotState(snapshot) {
    const graph = ensureGraph();
    if (!graph || !snapshot) return;
    state.undoLocked = true;
    try {
      const nextGraph = replaceRuntimeGraphData(snapshot.graph, { clone: true, reason: "apply-snapshot-state" });
      graph.changeData(nextGraph);
      state.graphStyle = { ...snapshot.graphStyle };
      ensureRibbonStyle();
      state.ribbonStyle = { ...snapshot.ribbonStyle };
      state.analysis = createAnalysisState({ ...state.analysis, ...(snapshot.analysis || {}) });
      syncRibbonUI(state.ribbonStyle);
      syncAnalysisButtons();
      syncEdgeDetailButton();
      updateGraphStats();
      syncAnalysisSourceDataFromCurrent();
      scheduleGraphPatch({
        scope: "local-update",
        reason: "apply-snapshot-state",
      });
    } finally {
      state.undoLocked = false;
      syncUndoButtons();
    }
  }

  function resetToInitialStyles() {
    const graph = ensureGraph();
    const initial = state.initialStyleSnapshot;
    if (!graph || !initial) return;
    recordUndoSnapshot();
    const nodeStyleMap = new Map((initial.nodes || []).map((n) => [n.id, pickStyle(n, NODE_STYLE_KEYS)]));
    const edgeStyleMap = new Map((initial.edges || []).map((e) => [e.id, pickStyle(e, EDGE_STYLE_KEYS)]));
    graph.setAutoPaint(false);
    try {
      graph.getNodes().forEach((node) => {
        const model = node.getModel?.() || {};
        const style = nodeStyleMap.get(model.id);
        if (!style) return;
        graph.updateItem(node, { ...style });
      });
      graph.getEdges().forEach((edge) => {
        const model = edge.getModel?.() || {};
        const style = edgeStyleMap.get(model.id);
        if (!style) return;
        graph.updateItem(edge, { ...style });
      });
    } finally {
      graph.setAutoPaint(true);
      graph.paint();
    }
    if (Array.isArray(state.graph.data.nodes)) {
      state.graph.data.nodes.forEach((n) => {
        const style = nodeStyleMap.get(n.id);
        if (style) Object.assign(n, style);
      });
    }
    if (Array.isArray(state.graph.data.edges)) {
      state.graph.data.edges.forEach((e) => {
        const style = edgeStyleMap.get(e.id);
        if (style) Object.assign(e, style);
      });
    }
    state.graphStyle = { ...initial.graphStyle };
    ensureRibbonStyle();
    state.ribbonStyle = { ...initial.ribbonStyle };
    state.analysis = createAnalysisState({
      filterMin: state.analysis?.filterMin,
      filterMax: state.analysis?.filterMax,
      collapseChildren: !!state.analysis?.collapseChildren,
    });
    syncRibbonUI(state.ribbonStyle);
    syncAnalysisButtons();
    updateGraphStats();
  }

  function getGraphCenterPoint(graph) {
    return ensureCanvasHitViewportController()?.getGraphCenterPoint?.(graph) || null;
  }

  function captureGraphViewport(targetGraph = null) {
    return ensureCanvasHitViewportController()?.captureViewport?.(targetGraph) || null;
  }

  function applyGraphViewport(viewport, targetGraph = null) {
    return !!ensureCanvasHitViewportController()?.applyViewport?.(viewport, targetGraph);
  }

  function updateCurrentViewSnapshot() {
    ensureViewSyncStore().updateCurrentViewSnapshot();
    const view = getActiveView();
    if (!view) return;
    const selectedIds = Array.from(state.selected || []);
    const focusIds = parseUniqueList([state.focusId, ...(state.focusIds || []), ...selectedIds], { allowString: true });
    view.mode = state.graphMode;
    view.focusId = state.focusId || focusIds[0] || "";
    view.focusName = state.focusName || "";
    view.focusLabel = state.focusLabel || "";
    view.focusIds = focusIds;
    view.focusNames = parseUniqueList([state.focusName, ...(state.focusNames || [])], { allowString: true });
    view.focusPlaceholderKinds = parseUniqueList(state.focusPlaceholderKinds || [], { allowString: true });
    view.focusKeyType = state.focusKeyType || "";
    view.focusOnly = !!state.focusOnly;
    view.focusSelfOnly = !!state.focusSelfOnly;
    view.focusCounterpartyStrict = !!state.focusCounterpartyStrict;
    view.filters = {
      dir: state.dir,
      hop: state.hop,
      minAmount: state.minAmount,
      maxEdges: state.maxEdges,
    };
    view.style = { ...state.graphStyle };
  }

  function updateViewCard(view = getActiveView()) {
    ensureViewSyncStore().updateViewCard(view);
  }

  function syncToolbarOverflowState() {
    const toolbar = $("#graphToolbar");
    if (!toolbar) return;
    const overflowDelta = toolbar.scrollWidth - toolbar.clientWidth;
    const hasOverflow = overflowDelta > 8;
    const atStart = toolbar.scrollLeft <= 4;
    const atEnd = toolbar.scrollLeft + toolbar.clientWidth >= toolbar.scrollWidth - 4;
    toolbar.dataset.toolbarOverflowing = hasOverflow ? "true" : "false";
    toolbar.dataset.toolbarScrollStart = atStart ? "true" : "false";
    toolbar.dataset.toolbarScrollEnd = atEnd ? "true" : "false";
  }

  function scheduleToolbarOverflowSync() {
    window.requestAnimationFrame(syncToolbarOverflowState);
  }


  function renderViewsBar() {
    const wrap = $("#viewsScroll");
    if (!wrap) return;

    if (isReactShellSubscribed()) {
      wrap.innerHTML = "";
      const bar = $("#viewsBar");
      if (bar) bar.classList.remove("compact");
      scheduleShellStateSync("views-bar");
      return;
    }

    wrap.innerHTML = "";

    if (!state.views.length) {
      const empty = document.createElement("div");
      empty.className = "viewEmpty";
      empty.textContent = "暂无视图";
      wrap.appendChild(empty);
    }

    state.views.forEach((view, idx) => {
      const tab = document.createElement("div");
      tab.className = "viewTab" + (view.id === state.activeViewId ? " active" : "");
      tab.dataset.vid = view.id;
      tab.setAttribute("role", "tab");
      tab.setAttribute("aria-selected", view.id === state.activeViewId ? "true" : "false");
      tab.setAttribute("tabindex", "0");
      tab.setAttribute("draggable", "true");

      const bridge = document.createElement("div");
      bridge.className = "tabBridge";
      bridge.setAttribute("aria-hidden", "true");

      const closeBtn = document.createElement("button");
      closeBtn.type = "button";
      closeBtn.className = "viewClose";
      closeBtn.setAttribute("aria-label", "关闭标签页");
      closeBtn.innerHTML =
        '<svg width="16" height="16" viewBox="0 0 24 24" fill="none">' +
        '<path d="M6 6L18 18" stroke="rgba(2,6,23,.65)" stroke-width="2" stroke-linecap="round" />' +
        '<path d="M18 6L6 18" stroke="rgba(2,6,23,.65)" stroke-width="2" stroke-linecap="round" />' +
        "</svg>";
      closeBtn.addEventListener("mousedown", (ev) => ev.stopPropagation());
      closeBtn.addEventListener("mousedown", (ev) => ev.preventDefault());
      closeBtn.addEventListener("click", (ev) => {
        ev.stopPropagation();
        deleteView(view.id);
      });

      const title = document.createElement("div");
      title.className = "viewTitle";
      const rawTitle = String(view.title || "").trim();
      const m =
        rawTitle.match(/^图(\d+)$/) ||
        rawTitle.match(/^视图(\d+)$/) ||
        rawTitle.match(/^起始页(?:\s*(\d+))?$/);
      const matchedNumber = m ? m[1] || (rawTitle === "起始页" ? "1" : "") : "";
      const displayNumber = /^\d{1,6}$/.test(matchedNumber) ? String(Number(matchedNumber) || "") : matchedNumber;
      const displayTitle = ordinaryPiiProjection.projectField(
        "node_id",
        m ? `图${displayNumber}` : rawTitle
      );
      title.textContent = displayTitle || "图";

      tab.appendChild(bridge);
      tab.appendChild(closeBtn);
      tab.appendChild(title);
      tab.addEventListener("click", () => setActiveView(view.id));
      tab.addEventListener("keydown", (ev) => {
        if (ev.key === "Enter" || ev.key === " ") {
          ev.preventDefault();
          setActiveView(view.id);
        }
      });
      tab.addEventListener("dragstart", (ev) => {
        try {
          ev.dataTransfer.effectAllowed = "move";
          ev.dataTransfer.setData("text/plain", view.id);
        } catch (e) {}
        tab.classList.add("dragging");
        state._dragViewId = view.id;
      });
      tab.addEventListener("dragend", () => {
        $$(".viewTab").forEach((el) => el.classList.remove("dragging", "dragOverLeft", "dragOverRight"));
        state._dragViewId = "";
      });
      tab.addEventListener("dragover", (ev) => {
        ev.preventDefault();
        const from = state._dragViewId || "";
        if (!from || from === view.id) return;
        $$(".viewTab").forEach((el) => {
          if (el !== tab) el.classList.remove("dragOverLeft", "dragOverRight");
        });
        const rect = tab.getBoundingClientRect();
        const left = ev.clientX < rect.left + rect.width / 2;
        tab.classList.toggle("dragOverLeft", left);
        tab.classList.toggle("dragOverRight", !left);
      });
      tab.addEventListener("dragleave", () => {
        tab.classList.remove("dragOverLeft", "dragOverRight");
      });
      tab.addEventListener("drop", async (ev) => {
        ev.preventDefault();
        const from = (() => {
          try {
            return ev.dataTransfer.getData("text/plain") || state._dragViewId || "";
          } catch (e) {
            return state._dragViewId || "";
          }
        })();
        if (!from || from === view.id) return;
        const rect = tab.getBoundingClientRect();
        const left = ev.clientX < rect.left + rect.width / 2;
        reorderViews(from, view.id, left ? "before" : "after");
        tab.classList.remove("dragOverLeft", "dragOverRight");
        await persistViewsOrder();
      });
      tab.addEventListener("contextmenu", (ev) => {
        ev.preventDefault();
        openViewContextMenu(view.id, ev.clientX, ev.clientY);
      });
      wrap.appendChild(tab);

      if (idx < state.views.length - 1) {
        const sep = document.createElement("div");
        sep.className = "tabSep";
        sep.textContent = "|";
        sep.setAttribute("aria-hidden", "true");
        wrap.appendChild(sep);
      }
    });

    if (!wrap.dataset.dragBound) {
      wrap.dataset.dragBound = "1";
      wrap.addEventListener("dragover", (ev) => {
        const from = state._dragViewId || "";
        if (!from) return;
        ev.preventDefault();
        const tabs = $$(".viewTab", wrap);
        if (!tabs.length) return;
        let target = null;
        let place = "after";
        for (const t of tabs) {
          const vid = t.dataset.vid || "";
          if (!vid || vid === from) continue;
          const rect = t.getBoundingClientRect();
          const mid = rect.left + rect.width / 2;
          if (ev.clientX < mid) {
            target = t;
            place = "before";
            break;
          }
          target = t;
          place = "after";
        }
        tabs.forEach((t) => t.classList.remove("dragOverLeft", "dragOverRight"));
        if (target) {
          target.classList.add(place === "before" ? "dragOverLeft" : "dragOverRight");
        }
      });
      wrap.addEventListener("drop", async (ev) => {
        const from = state._dragViewId || "";
        if (!from) return;
        ev.preventDefault();
        const tabs = $$(".viewTab", wrap);
        tabs.forEach((t) => t.classList.remove("dragOverLeft", "dragOverRight"));
        if (!tabs.length) return;
        let targetId = "";
        let place = "after";
        for (const t of tabs) {
          const vid = t.dataset.vid || "";
          if (!vid || vid === from) continue;
          const rect = t.getBoundingClientRect();
          const mid = rect.left + rect.width / 2;
          if (ev.clientX < mid) {
            targetId = vid;
            place = "before";
            break;
          }
          targetId = vid;
          place = "after";
        }
        if (!targetId) return;
        reorderViews(from, targetId, place);
        await persistViewsOrder();
      });
    }

    const bar = $("#viewsBar");
    if (bar) bar.classList.toggle("compact", state.views.length > 9);
    scheduleShellStateSync("views-bar");
  }

  function reorderViews(fromId, targetId, place = "before") {
    const fromIdx = state.views.findIndex((v) => v.id === fromId);
    const targetIdx = state.views.findIndex((v) => v.id === targetId);
    if (fromIdx < 0 || targetIdx < 0) return;
    const [moved] = state.views.splice(fromIdx, 1);
    let insertAt = targetIdx;
    if (fromIdx < targetIdx) insertAt -= 1;
    if (place === "after") insertAt += 1;
    insertAt = Math.max(0, Math.min(state.views.length, insertAt));
    state.views.splice(insertAt, 0, moved);
    renderViewsBar();
  }

  async function persistViewsOrder() {
    if (!state.caseId || !state.backend || typeof state.backend.reorderFlowViews !== "function") return;
    const order = state.views.map((v) => v.id);
    await backendCall("reorderFlowViews", JSON.stringify({ caseId: state.caseId, order }));
  }

  async function applyViewGraph(view, { patchMeta = null } = {}) {
    const data = view?.graph || { nodes: [], edges: [] };
    resetAnalysisFilterTools({ clearRange: true, clearSource: true, closePopover: true });
    const graph = ensureGraph();
    if (!graph) return;
    const nodeCount = Array.isArray(data?.nodes) ? data.nodes.length : 0;
    const edgeCount = Array.isArray(data?.edges) ? data.edges.length : 0;
    const graphTier = normalizeGraphTier(data?.graph_tier || data?.graphTier || "", nodeCount);
    const renderHints = normalizeGraphRenderHints(
      data?.render_hints || data?.renderHints || null,
      nodeCount,
      edgeCount,
      view?.mode || state.graphMode || "relation"
    );
    const nextGraph = cloneGraphData({
      ...(data || {}),
      graph_tier: graphTier,
      render_hints: renderHints,
    });
    let renderSummary = null;
    try {
      renderSummary = await applyGraphTierRenderPlanWithRust(
        nextGraph.nodes || [],
        nextGraph.edges || [],
        { ...state, graphTier, renderHints },
        { preserveEdgeLabels: false, name: "apply-view-graph" }
      );
    } catch (e) {
      log("WARN", "rust view graph render plan failed", { message: String(e?.message || e), stack: e?.stack || "" });
      toast("图谱展示计划失败", "danger");
      return;
    }
    const viewId = String(view?.id || "").trim();
    if (viewId && state.activeViewId && state.activeViewId !== viewId) return;
    const activeGraphTier = renderSummary?.tier || graphTier;
    const activeRenderHints = renderSummary?.renderHints || renderHints;
    nextGraph.graph_tier = activeGraphTier;
    nextGraph.render_hints = activeRenderHints;
    replaceRuntimeGraphData(nextGraph, { reason: "apply-view-graph" });
    state.graphTier = activeGraphTier;
    state.renderHints = activeRenderHints;
    const useEdgeBatch = shouldBatchEdges(nextGraph.edges?.length || 0, {
      ...state,
      graphTier: activeGraphTier,
      renderHints: activeRenderHints,
      graph: { ...state.graph, data: nextGraph },
    });
    cancelEdgeBatching();
    try {
      graph.clear();
    } catch (e) {}
    try {
      if (useEdgeBatch) {
        graph.data({ nodes: nextGraph.nodes || [], edges: [] });
        graph.render();
        if (graph.refreshPositions) graph.refreshPositions();
        if (graph.paint) graph.paint();
        syncGraphItemPositionsFromData(graph, nextGraph.nodes || [], "view-batch-render", { force: true, log });
        updateMinimap();
        renderEdgesInBatches(graph, nextGraph.edges || [], {
          onComplete: () => {
            if (state.edgeDetail) applyEdgeDetailLabels(true);
            updateMinimap();
            fitGraph({ force: true });
          },
        });
      } else {
        graph.data(nextGraph);
        graph.render();
        const graphEdges = graph.getEdges?.() || [];
        if (graphEdges.length >= EDGE_BATCH_FINAL_REFRESH_THRESHOLD) {
          scheduleGraphItemRefreshBatches(graph, graphEdges, {
            batchSize: Math.max(resolveEdgeRefreshBatchSize(graphEdges.length), resolveEdgeBatchSize(graphEdges.length)),
            timeout: GRAPH_ITEM_REFRESH_BATCH_TIMEOUT_MS,
          });
        } else {
          graphEdges.forEach((e) => graph.refreshItem(e));
        }
        updateMinimap();
      }
    } catch (e) {}
    updateGraphStats();
    if (view?.style) applyStyleToGraph(view.style, { syncView: false });
    const restored = applyGraphViewport(view?.viewport);
    finalizeView({ fit: !restored });
    syncAnalysisSourceDataFromCurrent({ force: true });
    commitShellStateSync("apply-view-graph");
    if (patchMeta) {
      publishGraphPatch({
        scope: patchMeta.scope || "view-activate",
        reason: patchMeta.reason || "apply-view-graph",
        viewId: patchMeta.viewId || view?.id || "",
        layoutPreset: patchMeta.layoutPreset || state.layoutPreset || "",
        forceFull: !!patchMeta.forceFull,
      });
    } else {
      rememberGraphPatchBase(nextGraph);
    }
  }

  async function ensureViewGraphLoaded(view) {
    if (!view) return view;
    const currentGraph = normalizeSnapshotGraphPayload(view.graph);
    view.graph = currentGraph;
    return view;
  }

  function setActiveView(viewId, { applyGraph = true, skipCapture = false } = {}) {
    if (!viewId) return;
    if (viewId === state.activeViewId) {
      if (applyGraph) {
        const activeView = getActiveView();
        void ensureViewGraphLoaded(activeView).then((resolvedView) => {
          if (resolvedView && state.activeViewId === viewId) {
            void applyViewGraph(resolvedView, {
              patchMeta: {
                scope: "view-activate",
                reason: "reapply-active-view",
                viewId,
              },
            });
          }
        });
      }
      return;
    }
    if (!skipCapture) updateCurrentViewSnapshot();
    state.activeViewId = viewId;
    const view = getActiveView();
    if (view) {
      state.graphMode = view.mode || "relation";
      state.focusId = view.focusId || "";
      state.focusName = view.focusName || "";
      state.focusIds = parseUniqueList([state.focusId, ...(view.focusIds || [])]);
      state.focusNames = parseUniqueList([state.focusName, ...(view.focusNames || [])]);
      if (!state.focusId && state.focusIds.length) state.focusId = state.focusIds[0];
      if (!state.focusName && state.focusNames.length) state.focusName = state.focusNames[0];
      state.focusOnly = !!view.focusOnly;
      state.focusKeyType = "";
      state.focusCounterpartyStrict = false;
      state.source = "";
      state.requestId = "";
      state.focusSelfOnly = false;
      state.focusSelfOnlyOnce = false;
      state.focusLabel = "";
      if (view.filters) {
        state.dir = view.filters.dir || state.dir;
        state.hop = view.filters.hop || state.hop;
        state.minAmount = view.filters.minAmount ?? state.minAmount;
        state.maxEdges = view.filters.maxEdges ?? state.maxEdges;
      }
      if (view.style) {
        state.graphStyle = { ...state.graphStyle, ...view.style };
        setToolbarStyle(state.graphStyle);
      }
    }
    renderViewsBar();
    updateViewCard(view);
    if (applyGraph && view) {
      void ensureViewGraphLoaded(view).then((resolvedView) => {
        if (resolvedView && state.activeViewId === viewId) {
          void applyViewGraph(resolvedView, {
            patchMeta: {
              scope: "view-activate",
              reason: "set-active-view",
              viewId,
            },
          });
        }
      });
    }
  }

  function addView(view, { activate = true, applyGraph = false } = {}) {
    if (!view) return;
    state.views.push(view);
    syncViewCounter();
    renderViewsBar();
    if (activate) {
      state.activeViewId = view.id;
      updateViewCard(view);
      if (applyGraph) {
        void applyViewGraph(view, {
          patchMeta: {
            scope: "view-activate",
            reason: "add-view-activate",
            viewId: view.id,
          },
        });
      }
    }
  }

  function normalizeView(raw) {
    if (!raw || typeof raw !== "object") return null;
    const graph = raw.graph || {};
    const nodes = Array.isArray(graph.nodes) ? graph.nodes : [];
    const edges = Array.isArray(graph.edges) ? graph.edges : [];
    const viewport = raw.viewport || raw.viewPort || raw.view_port || null;
    return {
      id: String(raw.id || createViewId()),
      title: String(raw.title || "视图"),
      mode: raw.mode || raw.view || "relation",
      focusId: raw.focusId || raw.focus?.id || "",
      focusName: raw.focusName || raw.focus?.name || "",
      focusIds: parseUniqueList(raw.focusIds || raw.focus_ids || [], { allowString: true }),
      focusNames: parseUniqueList(raw.focusNames || raw.focus_names || [], { allowString: true }),
      focusOnly: !!(raw.focusOnly ?? raw.focus?.only),
      focusCounterpartyStrict: !!(raw.focusCounterpartyStrict ?? raw.focus?.counterpartyStrict),
      filters: raw.filters || {},
      style: raw.style || raw.graphStyle || null,
      viewport,
      graph: { nodes, edges },
      counts: raw.counts || { nodes: nodes.length, edges: edges.length },
      saved: true,
      createdAt: raw.created_at || raw.createdAt || "",
      updatedAt: raw.updated_at || raw.updatedAt || "",
    };
  }

  async function loadViewsFromBackend() {
    state.views = [];
    state.activeViewId = "";
    renderViewsBar();
    updateViewCard(null);
    if (!state.caseId || !state.backend || typeof state.backend.listFlowViews !== "function") return;
    const raw = await backendCall("listFlowViews", JSON.stringify({ caseId: state.caseId }));
    const res = parseJSONSafe(raw, {});
    const views = Array.isArray(res?.views) ? res.views.map(normalizeView).filter(Boolean) : [];
    state.views = views;
    syncViewCounter();
    renderViewsBar();
    if (views.length) {
      setActiveView(views[0].id, { applyGraph: true, skipCapture: true });
    } else {
      updateViewCard(null);
    }
    state.viewsLoaded = true;
  }

  function buildViewFromSnapshot(snapshot, { title = "" } = {}) {
    const style = { ...state.graphStyle };
    const t = title || nextViewTitle();
    const viewport = captureGraphViewport();
    return {
      id: createViewId(),
      title: t,
      mode: state.graphMode,
      focusId: state.focusId || "",
      focusName: state.focusName || "",
      focusIds: state.focusIds ? state.focusIds.slice() : [],
      focusNames: state.focusNames ? state.focusNames.slice() : [],
      focusPlaceholderKinds: state.focusPlaceholderKinds ? state.focusPlaceholderKinds.slice() : [],
      focusOnly: !!state.focusOnly,
      focusCounterpartyStrict: !!state.focusCounterpartyStrict,
      filters: {
        dir: state.dir,
        hop: state.hop,
        minAmount: state.minAmount,
        maxEdges: state.maxEdges,
      },
      style,
      viewport,
      graph: snapshot,
      counts: { nodes: snapshot.nodes.length, edges: snapshot.edges.length },
      saved: false,
      createdAt: nowIso(),
      updatedAt: nowIso(),
    };
  }

  async function saveCurrentView() {
    const view = getActiveView();
    viewPersistenceDebug.lastSaveView = {
      caseId: String(state.caseId || ""),
      phase: "start",
      startedAt: nowIso(),
      viewId: String(view?.id || ""),
    };
    if (!view) {
      viewPersistenceDebug.lastSaveView.phase = "no-view";
      toast("暂无视图可保存", "warn");
      return;
    }
    updateCurrentViewSnapshot();
    if (!state.backend || typeof state.backend.saveFlowView !== "function") {
      viewPersistenceDebug.lastSaveView.phase = "backend-missing";
      toast("后端未就绪", "warn");
      return;
    }
    const payload = { caseId: state.caseId || "", view };
    const raw = await backendCall("saveFlowView", JSON.stringify(payload));
    const res = parseJSONSafe(raw, {});
    viewPersistenceDebug.lastSaveView = {
      ...(viewPersistenceDebug.lastSaveView || {}),
      finishedAt: nowIso(),
      responseLength: String(raw == null ? "" : raw).length,
      resultId: String(res?.id || ""),
      resultOk: !!res?.ok,
    };
    if (res?.ok) {
      if (res.id) {
        view.id = res.id;
        state.activeViewId = view.id;
      }
      view.saved = true;
      viewPersistenceDebug.lastSaveView.phase = "saved";
      renderViewsBar();
      updateViewCard(view);
      scheduleShellStateSync("view-save");
      toast("视图已存储", "ok");
    } else if (res?.cancelled) {
      viewPersistenceDebug.lastSaveView.phase = "cancelled";
      toast("已取消保存", "warn");
    } else {
      viewPersistenceDebug.lastSaveView.phase = "failed";
      const safeError = ordinaryPiiProjection.projectDetected(res?.error || "存储失败");
      viewPersistenceDebug.lastSaveView.error = safeError;
      toast(safeError, "danger");
    }
  }

  async function deleteView(viewId) {
    const view = state.views.find((v) => v.id === viewId);
    if (!view) return;
    const ok = await openViewCloseModal(view);
    if (!ok) return;

    if (view.saved && state.backend && typeof state.backend.deleteFlowView === "function") {
      await backendCall("deleteFlowView", JSON.stringify({ caseId: state.caseId || "", viewId: view.id }));
    }
    state.views = state.views.filter((v) => v.id !== viewId);
    if (state.activeViewId === viewId) {
      if (!state.views.length) {
        state.activeViewId = "";
        clearGraph();
        renderViewsBar();
        updateViewCard(null);
      } else {
        state.activeViewId = state.views[0].id;
        setActiveView(state.activeViewId, { applyGraph: true, skipCapture: true });
      }
    } else {
      renderViewsBar();
    }
  }

  async function renameView(viewId) {
    const view = state.views.find((v) => v.id === viewId);
    if (!view) return;
    let next = "";
    const existingTitle = String(view.title || "图").trim() || "图";
    const presentationTitle = ordinaryPiiProjection.projectField("node_id", existingTitle) || "图";
    if (typeof window.__ANALYTIX_PROMPT__ === "function") {
      const values = await window.__ANALYTIX_PROMPT__({
        title: "重命名视图",
        subtitle: presentationTitle,
        description: "请输入新的视图名称；取消后不会保存本次修改。",
        confirmLabel: "保存名称",
        cancelLabel: "取消",
        width: 520,
        fields: [
          {
            key: "title",
            label: "视图名称",
            placeholder: "请输入视图名称",
            defaultValue: presentationTitle,
            description: "建议使用便于区分的业务名称。",
            selectOnFocus: true,
          },
        ],
      });
      if (!values) return;
      next = String(values.title || "");
    } else {
      next = prompt("重命名", presentationTitle) || "";
    }
    const submittedTitle = next.trim();
    const nextTitle =
      submittedTitle === presentationTitle
        ? existingTitle
        : ordinaryPiiProjection.projectField("node_id", submittedTitle);
    if (!nextTitle) return;
    view.title = nextTitle;
    view.updatedAt = nowIso();
    renderViewsBar();
    updateViewCard(view);
    if (view.saved && state.backend && typeof state.backend.renameFlowView === "function") {
      await backendCall("renameFlowView", JSON.stringify({ caseId: state.caseId || "", viewId: view.id, title: view.title }));
    }
  }

  function buildViewContextMenuItems(viewId) {
    const id = String(viewId || "").trim();
    if (!id) return [];
    return [
      {
        label: "存储视图",
        action: async () => {
          setActiveView(id, { applyGraph: true });
          await saveCurrentView();
        },
      },
      {
        label: "重命名",
        action: () => {
          setActiveView(id, { applyGraph: true });
          renameView(id);
        },
      },
      { type: "sep" },
      {
        label: "导出 JPG",
        action: () => {
          setActiveView(id, { applyGraph: true });
          exportCurrentView("jpg");
        },
      },
      {
        label: "导出 PDF",
        action: () => {
          setActiveView(id, { applyGraph: true });
          exportCurrentView("pdf");
        },
      },
      {
        label: "导出 PNG",
        action: () => {
          setActiveView(id, { applyGraph: true });
          exportCurrentView("png");
        },
      },
      { type: "sep" },
      { label: "删除", action: () => deleteView(id) },
    ];
  }

  function openViewContextMenu(viewId, x, y) {
    openContextMenu(x, y, buildViewContextMenuItems(viewId));
  }

  async function exportCurrentView(format, scale = 1) {
    const view = getActiveView();
    if (!view) {
      toast("暂无视图可导出", "warn");
      return;
    }
    const graph = ensureGraph();
    if (!graph) {
      toast("暂无图谱", "warn");
      return;
    }
    if (!state.backend || typeof state.backend.exportFlowGraph !== "function") {
      toast("后端未就绪", "warn");
      return;
    }
    let dataUrl = "";
    try {
      const ratio = clampNumber(scale, 1, 4);
      dataUrl = graph.toDataURL("image/png", { backgroundColor: "#ffffff", ratio });
    } catch (e) {
      toast("导出失败", "danger");
      return;
    }
    const payload = {
      caseId: state.caseId || "",
      title: buildExportTitle(view),
      subject: buildExportSubject(view),
      dataUrl,
      format: format || "pdf",
    };
    const raw = await backendCall("exportFlowGraph", JSON.stringify(payload));
    const res = parseJSONSafe(raw, {});
    if (res?.ok) toast("导出完成", "ok");
    else if (res?.cancelled) toast("已取消导出", "warn");
    else toast(res?.error || "导出失败", "danger");
  }

  // ===== Context menu =====
  function closeContextMenu({ sync = true } = {}) {
    const menu = $("#contextMenu");
    if (menu) {
      menu.style.display = "none";
      menu.setAttribute("aria-hidden", "true");
      menu.innerHTML = "";
    }
    reactContextMenuActions.clear();
    if (state.reactOverlay?.contextMenu?.open || (state.reactOverlay?.contextMenu?.items || []).length) {
      state.reactOverlay.contextMenu.open = false;
      state.reactOverlay.contextMenu.x = 0;
      state.reactOverlay.contextMenu.y = 0;
      state.reactOverlay.contextMenu.items = [];
      if (sync) scheduleShellStateSync("react-context-close");
    }
  }

  function openContextMenu(x, y, items) {
    const rows = Array.isArray(items) ? items : [];
    closeRibbonPopover();
    closeReactFilterOverlay({ sync: false });
    closeReactStylePopover({ sync: false });
    closeReactExportOverlay({ sync: false });
    closeReactToolbarMenu({ sync: false });
    if (!isReactShellSubscribed()) {
      openContextMenuDom(x, y, rows);
      return;
    }
    state.reactOverlay.contextMenu.open = true;
    state.reactOverlay.contextMenu.x = Number(x) || 0;
    state.reactOverlay.contextMenu.y = Number(y) || 0;
    state.reactOverlay.contextMenu.items = buildReactContextMenuItems(rows);
    scheduleShellStateSync("react-context-open");
  }

  function closeDrillPopover() {
    const pop = $("#drillPopover");
    if (!pop) return;
    pop.style.display = "none";
    pop.setAttribute("aria-hidden", "true");
    pop.innerHTML = "";
    activeDrillPopover = null;
  }

  function openDrillPopover(x, y, model, ctxPos) {
    const pop = $("#drillPopover");
    if (!pop) return;
    const cfg = getDrillConfig();
    pop.innerHTML = `
      <div class="drillTitle">穿透分析</div>
      <div class="drillSection">
        <label class="drillOption">
          <input type="radio" name="drillPolicy" value="auto">
          <span class="drillOptionMain">自动（推荐）</span>
          <span class="drillOptionSub">余额优先，缺失余额用时间窗</span>
        </label>
        <label class="drillOption">
          <input type="radio" name="drillPolicy" value="balance">
          <span class="drillOptionMain">余额回落</span>
          <span class="drillOptionSub">余额回落即停止追踪</span>
        </label>
        <label class="drillOption">
          <input type="radio" name="drillPolicy" value="time">
          <span class="drillOptionMain">时间窗</span>
          <span class="drillOptionSub">仅看时间窗内的转出</span>
        </label>
      </div>
      <div class="drillRow">
        <div class="drillLabel">时间窗(天)</div>
        <input class="drillInput" type="number" min="0" max="3650" step="1" value="${cfg.windowDays}">
        <div class="drillHint">0 表示不限制</div>
      </div>
      <div class="drillActions">
        <button class="drillBtn ghost" type="button">取消</button>
        <button class="drillBtn primary" type="button">开始穿透</button>
      </div>
    `;
    const policyInputs = pop.querySelectorAll("input[name='drillPolicy']");
    policyInputs.forEach((input) => {
      if (input.value === cfg.policy) input.checked = true;
      input.addEventListener("change", () => {
        state.drillConfig = { ...state.drillConfig, policy: input.value };
      });
    });
    const windowInput = pop.querySelector(".drillInput");
    if (windowInput) {
      windowInput.addEventListener("change", () => {
        const raw = Number(windowInput.value);
        let next = Number.isFinite(raw) ? raw : cfg.windowDays;
        if (next < 0) next = 0;
        if (next > 3650) next = 3650;
        windowInput.value = String(next);
        state.drillConfig = { ...state.drillConfig, windowDays: next };
      });
    }
    const [cancelBtn, okBtn] = pop.querySelectorAll(".drillBtn");
    if (cancelBtn) cancelBtn.addEventListener("click", () => closeDrillPopover());
    if (okBtn) {
      okBtn.addEventListener("click", () => {
        closeDrillPopover();
        drillTxnNode(model, ctxPos);
      });
    }
    pop.style.display = "block";
    pop.setAttribute("aria-hidden", "false");
    const rect = pop.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const left = Math.max(10, Math.min(x, vw - rect.width - 10));
    const top = Math.max(10, Math.min(y, vh - rect.height - 10));
    pop.style.left = `${left}px`;
    pop.style.top = `${top}px`;
    activeDrillPopover = pop;
  }




  function normalizeLogLevel(level, fallback = "INFO") {
    const lv = String(level || fallback || "INFO").trim().toUpperCase();
    if (lv === "DEBUG" || lv === "INFO" || lv === "WARN" || lv === "ERROR") return lv;
    return String(fallback || "INFO").trim().toUpperCase();
  }

  function normalizeLogSwitch(raw) {
    const src = raw && typeof raw === "object" ? raw : {};
    return {
      enabled: asBool(src.enabled, DEFAULT_LOG_SWITCH.enabled),
      minLevel: normalizeLogLevel(src.minLevel, DEFAULT_LOG_SWITCH.minLevel),
      console: asBool(src.console, DEFAULT_LOG_SWITCH.console),
      backend: asBool(src.backend, DEFAULT_LOG_SWITCH.backend),
      core: asBool(src.core, DEFAULT_LOG_SWITCH.core),
      dataFeed: asBool(src.dataFeed, DEFAULT_LOG_SWITCH.dataFeed),
      graphTrace: asBool(src.graphTrace, DEFAULT_LOG_SWITCH.graphTrace),
      graphSample: asBool(src.graphSample, DEFAULT_LOG_SWITCH.graphSample),
      graphRender: asBool(src.graphRender, DEFAULT_LOG_SWITCH.graphRender),
      layout: asBool(src.layout, DEFAULT_LOG_SWITCH.layout),
      interaction: asBool(src.interaction, DEFAULT_LOG_SWITCH.interaction),
      perf: asBool(src.perf, DEFAULT_LOG_SWITCH.perf),
      debug: asBool(src.debug, DEFAULT_LOG_SWITCH.debug),
      layoutVerbose: asBool(src.layoutVerbose, DEFAULT_LOG_SWITCH.layoutVerbose),
      webglWarn: asBool(src.webglWarn, DEFAULT_LOG_SWITCH.webglWarn),
      handoffProbe: asBool(src.handoffProbe, DEFAULT_LOG_SWITCH.handoffProbe),
      canvasMetrics: asBool(src.canvasMetrics, DEFAULT_LOG_SWITCH.canvasMetrics),
    };
  }

  function readStoredLogSwitch() {
    try {
      const fromWindow = window.__ANALYTIX_VIZ_LOG;
      if (fromWindow && typeof fromWindow === "object") {
        return normalizeLogSwitch(fromWindow);
      }
    } catch (e) {}
    try {
      const raw = localStorage.getItem(LOG_STORAGE_KEY);
      if (!raw) return null;
      const data = JSON.parse(raw);
      return normalizeLogSwitch(data);
    } catch (e) {}
    return null;
  }

  function resolveLogTopic(message, data) {
    const msg = String(message || "").trim().toLowerCase();
    if (!msg) return "debug";
    if (msg === "graph trace") {
      const phase = String(data?.phase || "").trim().toLowerCase();
      if (phase === "sample") return "graphSample";
      if (phase.includes("render")) return "graphRender";
      if (phase.includes("layout")) return "layout";
      if (phase === "response") return "dataFeed";
      return "graphTrace";
    }
    if (
      msg.startsWith("initbackend") ||
      msg.startsWith("synccase") ||
      msg.startsWith("casechanged") ||
      msg.startsWith("caseid") ||
      msg.includes("log switch")
    ) {
      return "core";
    }
    if (
      msg.includes("stats->viz probe payload") ||
      msg.includes("viz->backend probe getflowgraph request") ||
      msg.includes("viz probe render payload")
    ) {
      return "handoffProbe";
    }
    if (msg.includes("viz canvas snapshot")) return "canvasMetrics";
    if (
      msg.includes("stats->viz") ||
      msg.includes("openflowgraphfromrow payload") ||
      msg.includes("payload received") ||
      msg.includes("direct graph request")
    ) {
      return "dataFeed";
    }
    if (msg.includes("ghost probe")) return "layout";
    if (msg.includes("layout")) return "layout";
    if (msg.includes("render") || msg.includes("renderer") || msg.includes("gpu probe")) {
      return "graphRender";
    }
    if (
      msg.includes("contextmenu") ||
      msg.includes("edge click") ||
      msg.includes("node click") ||
      msg.includes("popover")
    ) {
      return "interaction";
    }
    if (msg.includes("perf") || msg.includes("wheelroute")) return "perf";
    if (msg.includes("graph") || msg.includes("drill")) return "graphTrace";
    return "debug";
  }

  function shouldEmitLog(level, message, data) {
    const cfg = logSwitch || DEFAULT_LOG_SWITCH;
    if (!cfg.enabled) return false;
    const lv = normalizeLogLevel(level, "INFO");
    const rank = LOG_LEVEL_RANK[lv] || LOG_LEVEL_RANK.INFO;
    const minRank = LOG_LEVEL_RANK[normalizeLogLevel(cfg.minLevel, "INFO")] || LOG_LEVEL_RANK.INFO;
    if (rank < minRank) return false;
    if (rank >= LOG_LEVEL_RANK.WARN) return true;
    const topic = resolveLogTopic(message, data);
    return !!cfg[topic];
  }

  function shouldLogHandoffProbe() {
    return !!(logSwitch?.handoffProbe || state.debugTrace || logSwitch?.debug);
  }

  function shouldLogCanvasMetrics() {
    return !!logSwitch?.canvasMetrics;
  }

  function applyLogSwitch(raw, { persist = false } = {}) {
    logSwitch = normalizeLogSwitch({ ...logSwitch, ...(raw || {}) });
    try {
      window.__ANALYTIX_VIZ_LOG = { ...logSwitch };
    } catch (e) {}
    if (persist) {
      try {
        localStorage.setItem(LOG_STORAGE_KEY, JSON.stringify(logSwitch));
      } catch (e) {}
    }
    return { ...logSwitch };
  }

  const initialLogSwitch = readStoredLogSwitch();
  if (initialLogSwitch) {
    logSwitch = initialLogSwitch;
  }

  function log(level, message, data) {
    const safeLevel = normalizeLogLevel(level, "INFO");
    if (!shouldEmitLog(safeLevel, message, data)) return;
    const safeTopic = resolveLogTopic(message, data);
    const safeEnvelope = {
      level: safeLevel,
      topic: safeTopic,
      hasData: data !== undefined && data !== null,
    };
    if (logSwitch.console) {
      console.log(`${LOG_PREFIX}[${safeLevel}] ${safeTopic}`);
    }
    if (!logSwitch.backend) return;
    try {
      if (backendRef && typeof backendRef.logFlowDebug === "function") {
        backendRef.logFlowDebug(JSON.stringify(safeEnvelope));
      }
    } catch (e) {}
  }

  function layoutPresetLabel(preset) {
    if (canvasShellAdapter && typeof canvasShellAdapter.layoutPresetLabel === "function") {
      return canvasShellAdapter.layoutPresetLabel(preset);
    }
    const mode = String(preset || "").trim().toLowerCase();
    if (mode === "network") return "网络图布局";
    if (mode === "compact" || mode === "relation") return "关联图布局";
    if (mode === "hierarchy") return "层级图布局";
    if (mode === "flow") return "流向图布局";
    return mode || "布局";
  }

  function buildGraphStatsSummary(graphData, layoutPreset) {
    if (canvasShellAdapter && typeof canvasShellAdapter.buildGraphStatsSummary === "function") {
      return canvasShellAdapter.buildGraphStatsSummary({ graphData, layoutPreset });
    }
    return {
      contract: "FlowGraphSummaryV1",
      status: graphData ? "unknown" : "unloaded",
      factAnswerAllowed: false,
      layoutLabel: layoutPresetLabel(layoutPreset),
      nodes: null,
      edges: null,
      amount: null,
      boundaryCode: graphData ? "host_verification_missing" : "no_result_loaded",
    };
  }

  function logLayoutSwitchBanner(nextPreset, prevPreset) {
    const nextLabel = layoutPresetLabel(nextPreset);
    const prevLabel = layoutPresetLabel(prevPreset);
    const hasTransition = String(nextPreset || "") && String(prevPreset || "") && String(nextPreset) !== String(prevPreset);
    const transition = hasTransition ? `（${prevLabel} -> ${nextLabel}）` : "";
    log("INFO", `[LAYOUT_SWITCH] - - - - 切换 - - - ${nextLabel} - - - - ${transition}`.trim());
  }

  function parseJSONSafe(raw, fallback = {}) {
    try {
      if (!raw) return fallback;
      if (typeof raw === "object") return raw;
      return JSON.parse(raw);
    } catch (e) {
      return fallback;
    }
  }

  function asBool(value, fallback = false) {
    if (value == null) return !!fallback;
    if (typeof value === "boolean") return value;
    if (typeof value === "number") return Number.isFinite(value) && value !== 0;
    if (typeof value === "string") {
      const s = value.trim().toLowerCase();
      if (!s) return false;
      if (["0", "false", "no", "off", "null", "undefined", "nan"].includes(s)) return false;
      if (["1", "true", "yes", "on"].includes(s)) return true;
      return true;
    }
    return !!value;
  }

  function escapeHtml(value) {
    return String(value ?? "")
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#39;");
  }

  function compactToastText(value, maxLength = 88) {
    const text = String(value ?? "").replace(/\s+/g, " ").trim();
    if (!text) return "";
    if (text.length <= maxLength) return text;
    return `${text.slice(0, Math.max(0, maxLength - 1)).trimEnd()}…`;
  }

  function resolveFlowToastPayload(message, kind = "info") {
    const text = compactToastText(message);
    if (!text) {
      return { tone: "info", title: "系统提示", detail: "" };
    }

    if (kind === "danger") {
      const title = text.includes("图谱")
        ? "图谱执行失败"
        : text.includes("导出")
          ? "图谱导出失败"
          : "图谱操作失败";
      return { tone: "error", title, detail: text };
    }

    if (kind === "warn") {
      if (text === "后端未就绪") {
        return {
          tone: "info",
          title: "后端服务尚未就绪",
          detail: "请等待服务启动完成后，再继续当前操作。"
        };
      }
      if (text === "未选择案件") {
        return {
          tone: "info",
          title: "先选择案件",
          detail: "进入图谱前，请先在案件页打开一个案件。"
        };
      }
      if (text.includes("请先在统计分析选择账户后再展示图谱")) {
        return {
          tone: "info",
          title: "先选择统计对象",
          detail: "请先在统计分析中选择账户后，再生成图谱。"
        };
      }
      if (text.includes("统计分析展示模式下禁止修改方向")) {
        return {
          tone: "warning",
          title: "当前模式不支持修改方向",
          detail: "请退出统计分析展示模式后，再调整图谱方向。"
        };
      }
      if (text === "暂无视图可保存") {
        return {
          tone: "info",
          title: "当前没有可保存视图",
          detail: "请先生成或调整图谱视图后，再执行保存。"
        };
      }
      if (text === "暂无视图可导出") {
        return {
          tone: "info",
          title: "当前没有可导出视图",
          detail: "请先生成图谱视图后，再执行导出。"
        };
      }
      if (text === "暂无图谱" || text === "当前无图谱可操作") {
        return {
          tone: "info",
          title: "当前没有可用图谱",
          detail: "请先生成图谱结果后，再执行当前操作。"
        };
      }
      if (text === "未识别到可收缩的子节点") {
        return {
          tone: "info",
          title: "当前没有可收缩子节点",
          detail: "请先展开图谱后，再执行收缩。"
        };
      }
      if (text === "暂无可合并节点") {
        return {
          tone: "info",
          title: "当前没有可合并节点",
          detail: "请先选择可合并节点后，再执行操作。"
        };
      }
      if (text === "请先选择节点") {
        return {
          tone: "info",
          title: "先选择节点",
          detail: "请先选择目标节点后，再执行当前操作。"
        };
      }
      if (text === "未找到符合的节点" || text === "未找到节点") {
        return {
          tone: "info",
          title: "未找到匹配节点",
          detail: "请调整检索条件后，再执行当前操作。"
        };
      }
      if (text === "请至少选择两个节点" || text === "请先选择至少两个节点再展开路径") {
        return {
          tone: "info",
          title: "至少选择两个节点",
          detail: "请补足节点选择后，再执行当前操作。"
        };
      }
      if (text === "当前视口不可用") {
        return {
          tone: "info",
          title: "当前视口不可用",
          detail: "请等待图谱渲染完成后，再执行当前操作。"
        };
      }
      if (text === "当前视口没有可展开的折叠节点") {
        return {
          tone: "info",
          title: "当前视口没有可展开节点",
          detail: "请切换视口范围或调整筛选条件后，再执行操作。"
        };
      }
      if (text === "已取消导出" || text === "已取消保存") {
        return {
          tone: "info",
          title: text === "已取消导出" ? "已取消导出" : "已取消保存",
          detail: "本次操作未继续执行。"
        };
      }
      return {
        tone: "warning",
        title: "图谱操作待复核",
        detail: text
      };
    }

    if (kind === "ok") {
      if (text === "已复制") {
        return {
          tone: "success",
          title: "内容已复制",
          detail: "当前内容已写入剪贴板。"
        };
      }
      if (text === "视图已存储") {
        return {
          tone: "success",
          title: "图谱视图已保存",
          detail: "当前视图已写入工作区。"
        };
      }
      if (text === "导出完成") {
        return {
          tone: "success",
          title: "图谱已导出",
          detail: "导出结果已写入目标目录。"
        };
      }
      if (text === "合并成功") {
        return {
          tone: "success",
          title: "节点已合并",
          detail: "当前节点关系已完成合并。"
        };
      }
      if (text === "已展开匹配簇") {
        return {
          tone: "success",
          title: "匹配簇已展开",
          detail: "相关节点簇已展开到当前视口。"
        };
      }
      if (text.startsWith("已创建 ") && text.includes(" 条连接")) {
        return {
          tone: "success",
          title: "连接已创建",
          detail: text
        };
      }
      if (text.startsWith("图谱已生成")) {
        return {
          tone: "success",
          title: "图谱已生成",
          detail: text.replace(/^图谱已生成\s*[·-]?\s*/, "") || "图谱结果已准备完成。"
        };
      }
      if (text.includes("已开启") || text.includes("已关闭")) {
        return {
          tone: "success",
          title: "折线显示已更新",
          detail: text
        };
      }
      return { tone: "success", title: compactToastText(text, 24), detail: "" };
    }

    return {
      tone: "info",
      title: compactToastText(text, 24) || "系统提示",
      detail: text.length > 24 ? text : ""
    };
  }

  function toast(message, kind = "info") {
    const text = String(message ?? "").trim();
    if (!text) return;
    const payload = resolveFlowToastPayload(text, kind);

    try {
      const event = new CustomEvent("analytix:toast", {
        detail: {
          tone: payload.tone,
          title: payload.title,
          detail: payload.detail || undefined
        },
        cancelable: true,
      });
      window.dispatchEvent(event);
      if (event.defaultPrevented) return;
    } catch (e) {}

    const wrap = $("#toastWrap");
    if (!wrap) return;

    const node = document.createElement("div");
    node.className = "toast";

    const dot = document.createElement("div");
    dot.className = "tDot";
    if (kind === "ok") dot.classList.add("ok");
    if (kind === "warn") dot.classList.add("warn");
    if (kind === "danger") dot.classList.add("danger");

    const msg = document.createElement("div");
    msg.className = "tMsg";
    msg.textContent = payload.detail ? `${payload.title} · ${payload.detail}` : payload.title;

    node.appendChild(dot);
    node.appendChild(msg);
    wrap.appendChild(node);

    setTimeout(() => {
      node.style.opacity = "0";
      node.style.transform = "translateY(6px)";
    }, 2200);
    setTimeout(() => {
      try {
        node.remove();
      } catch (e) {}
    }, 2600);
  }

  function copyText(text, fieldName = "") {
    const value = ordinaryPiiProjection.projectField(fieldName, text);
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(value).catch(() => {});
      return;
    }
    try {
      const ta = document.createElement("textarea");
      ta.value = value;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.focus();
      ta.select();
      document.execCommand("copy");
      ta.remove();
    } catch (e) {}
  }

  async function writeGraphClipboardText() {
    // A graph payload can contain raw account identifiers throughout nested
    // node and edge metadata. Until the host provides a typed, auditable
    // controlled-copy authority, ordinary clipboard export must fail closed.
    state.graphClipboardText = "";
    return false;
  }

  async function readGraphClipboardText() {
    if (navigator.clipboard && navigator.clipboard.readText) {
      try {
        const value = await navigator.clipboard.readText();
        if (graphClipboardModel.parseGraphClipboardPayload(value)) {
          state.graphClipboardText = value;
          return value;
        }
      } catch (e) {}
    }
    return state.graphClipboardText || "";
  }

  const PLACEHOLDER_TOKEN_PREFIX = "__cp_placeholder__::";
  const PLACEHOLDER_KIND_LABELS = {
    db_null: "NULL",
    empty: "空串",
    slash_n: "\\N",
    dash: "-",
    emdash: "—",
    fw_dash: "－",
    literal_null: "null",
    literal_none: "none",
    literal_nan: "nan",
  };
  function normalizePlaceholderKind(value) {
    const text = String(value || "").trim();
    return Object.prototype.hasOwnProperty.call(PLACEHOLDER_KIND_LABELS, text) ? text : "";
  }

  function placeholderKindFromLabel(value) {
    const text = String(value || "").trim();
    if (!text) return "";
    for (const [kind, label] of Object.entries(PLACEHOLDER_KIND_LABELS)) {
      if (label === text) return kind;
    }
    return "";
  }

  function placeholderKindFromToken(value) {
    const text = String(value || "").trim();
    if (!text.startsWith(PLACEHOLDER_TOKEN_PREFIX)) return "";
    const remainder = text.slice(PLACEHOLDER_TOKEN_PREFIX.length).split("::name::", 1)[0];
    return normalizePlaceholderKind(remainder);
  }

  function placeholderNameFromToken(value) {
    const text = String(value || "").trim();
    if (!text.startsWith(PLACEHOLDER_TOKEN_PREFIX)) return "";
    const marker = "::name::";
    const idx = text.indexOf(marker);
    if (idx < 0) return "";
    return text.slice(idx + marker.length).trim();
  }

  function isPlaceholderNodeId(value) {
    return !!placeholderKindFromToken(value);
  }

  function isNoisySeedCandidateId(value) {
    const text = String(value || "").trim();
    if (!text) return true;
    if (placeholderKindFromToken(text) || placeholderKindFromLabel(text)) return true;
    const lower = text.toLowerCase();
    return lower === "unknown" || lower === "未知" || lower === "null" || lower === "none" || lower === "nan";
  }

  function isLikelyAccountId(value) {
    const text = String(value || "").trim();
    if (!text) return false;
    return /^\d{8,}$/.test(text);
  }

  let nodeHoverTip = null;
  let networkDenseHoverEdgeReveal = { graph: null, rows: [] };
  function ensureNodeHoverTip() {
    if (nodeHoverTip) return nodeHoverTip;
    const el = document.createElement("div");
    el.id = "nodeHoverTip";
    el.className = "nodeHoverTip";
    el.setAttribute("aria-hidden", "true");
    el.innerHTML = '<div class="nodeHoverTipTitle"></div><div class="nodeHoverTipList"></div>';
    document.body.appendChild(el);
    nodeHoverTip = el;
    return el;
  }

  function hideNodeHoverTip() {
    if (!nodeHoverTip) return;
    nodeHoverTip.style.display = "none";
    nodeHoverTip.setAttribute("aria-hidden", "true");
  }

  function showNodeHoverTip(model, ev) {
    if (!model) return;
    const list = normalizeDisplayIds(model.displayIds || model.display_ids || model.displayIdRaw || model.display_id || model.displayId);
    if (list.length < 2) {
      hideNodeHoverTip();
      return;
    }
    const presentation = shellOverlayAdapter.buildNodeHoverPresentation(model, list);
    const tip = ensureNodeHoverTip();
    const title = presentation.title;
    const titleEl = tip.querySelector(".nodeHoverTipTitle");
    const listEl = tip.querySelector(".nodeHoverTipList");
    if (titleEl) titleEl.textContent = `${title} · 账户合集（${list.length}）`;
    if (listEl) {
      listEl.innerHTML = presentation.accounts
        .map((id) => `<div class="nodeHoverTipItem">${escapeHtml(id)}</div>`)
        .join("");
    }

    const e = extractDomEvent(ev);
    let x = (e?.clientX ?? 0) + 12;
    let y = (e?.clientY ?? 0) + 12;
    tip.style.display = "block";
    tip.setAttribute("aria-hidden", "false");
    const rect = tip.getBoundingClientRect();
    const maxX = Math.max(12, window.innerWidth - rect.width - 12);
    const maxY = Math.max(12, window.innerHeight - rect.height - 12);
    x = Math.min(x, maxX);
    y = Math.min(y, maxY);
    tip.style.left = `${Math.max(12, x)}px`;
    tip.style.top = `${Math.max(12, y)}px`;
  }

  function backendCall(method, payload) {
    return new Promise((resolve) => {
      if (!state.backend || typeof state.backend[method] !== "function") {
        viewPersistenceDebug.lastBackendCall = {
          method: String(method || ""),
          status: "missing",
          finishedAt: nowIso(),
        };
        resolve(null);
        return;
      }
      viewPersistenceDebug.lastBackendCall = {
        method: String(method || ""),
        payloadLength: String(payload == null ? "" : payload).length,
        startedAt: nowIso(),
        status: "pending",
      };
      let settled = false;
      const finish = (raw) => {
        if (settled) return;
        settled = true;
        viewPersistenceDebug.lastBackendCall = {
          ...(viewPersistenceDebug.lastBackendCall || {}),
          responseLength: String(raw == null ? "" : raw).length,
          finishedAt: nowIso(),
          status: raw == null ? "empty" : "resolved",
        };
        resolve(raw == null ? null : raw);
      };
      try {
        const fn = state.backend[method];
        const expectsCallback = Number(fn.length || 0) >= 2;
        viewPersistenceDebug.lastBackendCall = {
          ...(viewPersistenceDebug.lastBackendCall || {}),
          expectsCallback,
        };
        const result = fn(payload, finish);
        if (result && typeof result.then === "function") {
          result
            .then((raw) => {
              if (raw != null || !expectsCallback) finish(raw);
            })
            .catch(() => finish(null));
        } else if (result != null) {
          finish(result);
        }
      } catch (e) {
        viewPersistenceDebug.lastBackendCall = {
          ...(viewPersistenceDebug.lastBackendCall || {}),
          error: ordinaryPiiProjection.projectDetected(e?.message || e || ""),
        };
        finish(null);
      }
    });
  }

  const {
    computeLayoutNodePlan,
    projectLayoutCacheKey,
    computeNetworkLayoutPlan,
    computeNetworkCommunityQuality,
  } = createLayoutNodePlanClient({
    backendCall,
    parseJSONSafe,
    getCaseId: () => state.caseId || "",
  });
  const { projectAnalysisGraphData } = createAnalysisGraphDataClient({
    backendCall,
    parseJSONSafe,
    getCaseId: () => state.caseId || "",
  });
  const { projectGraphSearch } = createGraphSearchClient({
    backendCall,
    parseJSONSafe,
    getCaseId: () => state.caseId || "",
  });
  const { projectProjectionLayoutSeed } = createProjectionLayoutSeedClient({
    backendCall,
    parseJSONSafe,
    getCaseId: () => state.caseId || "",
  });
  const { mergeSameNameGraph } = createSameNameMergeClient({
    backendCall,
    parseJSONSafe,
    getCaseId: () => state.caseId || "",
  });
  const { projectGraphRenderPlan } =
    flowGraphRenderPlanClientFactory.createGraphRenderPlanClient({
      backendCall,
      parseJSONSafe,
      getCaseId: () => state.caseId || "",
    });
  const { projectNetworkSectorPlacement } =
    flowNetworkSectorPlacementClientFactory.createNetworkSectorPlacementClient({
      backendCall,
      parseJSONSafe,
      getCaseId: () => state.caseId || "",
    });
  const networkSectorPlacementCache =
    flowNetworkSectorPlacementCacheFactory.createNetworkSectorPlacementCache({
      maxEntries: 32,
      nowFn: () => Date.now(),
    });

  async function projectNetworkSectorPlacementWithRust(payload = {}) {
    const result = await projectNetworkSectorPlacement(payload);
    const sectorPlacement =
      result?.sectorPlacement && typeof result.sectorPlacement === "object"
        ? result.sectorPlacement
        : null;
    if (!sectorPlacement) {
      throw new Error("network-sector-placement-rust-result-missing");
    }
    return sectorPlacement;
  }

  function prefetchNetworkSectorPlacementWithRust(payload = {}, cacheKey = "") {
    if (typeof networkSectorPlacementCache.recordPrefetchEvent === "function") {
      networkSectorPlacementCache.recordPrefetchEvent("requested");
    }
    let request = {};
    try {
      request = payload && typeof payload === "object" ? JSON.parse(JSON.stringify(payload)) : {};
    } catch (error) {
      if (typeof networkSectorPlacementCache.recordPrefetchEvent === "function") {
        networkSectorPlacementCache.recordPrefetchEvent("invalid");
      }
      if (state.debugTrace || state.debugLayout) {
        try {
          log("WARN", "network sector placement prefetch payload clone failed", {
            message: String(error?.message || error),
          });
        } catch (e) {}
      }
      return false;
    }
    const key =
      String(cacheKey || "").trim() ||
      (typeof networkSectorPlacementCache.makeKey === "function"
        ? networkSectorPlacementCache.makeKey(request)
        : "");
    if (!key) {
      if (typeof networkSectorPlacementCache.recordPrefetchEvent === "function") {
        networkSectorPlacementCache.recordPrefetchEvent("invalid");
      }
      return false;
    }
    if (networkSectorPlacementCache.read(key, { source: "prefetch" })) {
      if (typeof networkSectorPlacementCache.recordPrefetchEvent === "function") {
        networkSectorPlacementCache.recordPrefetchEvent("cached");
      }
      return true;
    }
    if (!networkSectorPlacementCache.markPending(key)) {
      if (typeof networkSectorPlacementCache.recordPrefetchEvent === "function") {
        networkSectorPlacementCache.recordPrefetchEvent("pending");
      }
      return true;
    }
    if (typeof networkSectorPlacementCache.recordPrefetchEvent === "function") {
      networkSectorPlacementCache.recordPrefetchEvent("queued");
    }
    void projectNetworkSectorPlacementWithRust(request)
      .then((sectorPlacement) => {
        networkSectorPlacementCache.remember(key, sectorPlacement);
        if (typeof networkSectorPlacementCache.recordPrefetchEvent === "function") {
          networkSectorPlacementCache.recordPrefetchEvent("stored");
        }
      })
      .catch((error) => {
        if (typeof networkSectorPlacementCache.recordPrefetchEvent === "function") {
          networkSectorPlacementCache.recordPrefetchEvent("failed");
        }
        if (state.debugTrace || state.debugLayout) {
          try {
            log("WARN", "network sector placement prefetch failed", {
              message: String(error?.message || error),
            });
          } catch (e) {}
        }
      })
      .finally(() => {
        networkSectorPlacementCache.clearPending(key);
      });
    return true;
  }

  function installNetworkSectorPlacementCacheBridge() {
    const engine = getLayoutEngine();
    if (!engine || typeof engine !== "object") return false;
    engine.networkSectorPlacementCache = {
      requireHit: false,
      makeKey(payload = {}) {
        return networkSectorPlacementCache.makeKey(payload);
      },
      read(cacheKey = "") {
        return networkSectorPlacementCache.read(cacheKey, { source: "layout" });
      },
      prefetch(payload = {}, cacheKey = "") {
        return prefetchNetworkSectorPlacementWithRust(payload, cacheKey);
      },
    };
    return true;
  }
  installNetworkSectorPlacementCacheBridge();

  async function applyProjectedEdgeOffsets(edges = [], { reason = "" } = {}) {
    const links = Array.isArray(edges) ? edges : [];
    if (!links.length) return { applied: 0, summary: summarizeEdgeOffsets(links) };
    const result = await projectGraphRenderPlan({
      renderPlan: {
        name: reason ? `edge-offset-${reason}` : "edge-offset",
        nodes: [],
        edges: links,
        ctx: {},
        hints: {},
      },
    });
    const renderResult = Array.isArray(result?.renderResults) ? result.renderResults[0] : null;
    if (!renderResult || typeof renderResult !== "object") {
      throw new Error("graph-edge-offset-rust-result-missing");
    }
    const applied = applyEdgeOffsetUpdates(links, renderResult.edgeOffsetUpdates || []);
    const summary = summarizeEdgeOffsets(links);
    try {
      if (state.debugLayout && summary.multiPairs > 0) {
        log("INFO", "edge offsets computed", { ...summary, reason });
      }
    } catch (e) {}
    return { applied, summary };
  }

  const { projectLayoutRoleGraph } =
    createLayoutRoleProjectionClient({
      backendCall,
      parseJSONSafe,
      getCaseId: () => state.caseId || "",
    });

  function fmtMoney(n) {
    const num = Number(n) || 0;
    return num.toLocaleString("zh-CN", { maximumFractionDigits: 2, minimumFractionDigits: 0 });
  }

  function fmtShortDateStr(value) {
    const s = String(value || "").trim();
    if (!s) return "";
    const m = s.match(/(\d{4})-(\d{2})-(\d{2})/);
    if (!m) return "";
    return `${m[1].slice(2)}-${m[2]}-${m[3]}`;
  }

  function fmtShortRange(start, end) {
    const a = fmtShortDateStr(start);
    const b = fmtShortDateStr(end);
    if (!a && !b) return "";
    if (a && (!b || a === b)) return a;
    return `${a}~${b}`;
  }

  const FONT_LIST = [
    {
      label: "思源黑体",
      value:
        '"Source Han Sans SC","Noto Sans SC","PingFang SC","Microsoft YaHei",ui-sans-serif,system-ui',
    },
    {
      label: "苹方",
      value: '"PingFang SC","Noto Sans SC","Microsoft YaHei",ui-sans-serif,system-ui',
    },
    {
      label: "微软雅黑",
      value: '"Microsoft YaHei","PingFang SC","Noto Sans SC",ui-sans-serif,system-ui',
    },
    {
      label: "等线",
      value: '"DengXian","Microsoft YaHei",ui-sans-serif,system-ui',
    },
    {
      label: "DIN",
      value: '"DIN Alternate","DIN Condensed","DIN","Helvetica Neue",ui-sans-serif,system-ui',
    },
    {
      label: "Roboto",
      value: '"Roboto","Helvetica Neue",ui-sans-serif,system-ui',
    },
  ];

  const FONT_SIZE_LIST = [8, 9, 10, 11, 12, 13, 14, 16, 18, 20, 22, 24, 28, 32, 36];
  const LINE_STYLE_OPTIONS = [
    { label: "实线", value: "solid", dash: [] },
    { label: "虚线", value: "dash", dash: [6, 4] },
    { label: "点线", value: "dot", dash: [2, 3] },
    { label: "点划线", value: "dashdot", dash: [6, 3, 2, 3] },
    { label: "长虚线", value: "longdash", dash: [10, 4] },
  ];
  const LINE_WIDTH_OPTIONS = [1, 1.5, 2, 2.5, 3, 4];
  const ARROW_OPTIONS = [
    { label: "无", value: "none" },
    { label: "从左到右", value: "end" },
    { label: "从右到左", value: "start" },
    { label: "双向", value: "both" },
  ];
  const NODE_SHAPE_OPTIONS = [
    { label: "圆形", value: "circle" },
    { label: "矩形", value: "rect" },
    { label: "圆角矩形", value: "roundrect" },
    { label: "菱形", value: "diamond" },
    { label: "胶囊", value: "pill" },
    { label: "三角形", value: "triangle" },
    { label: "五边形", value: "pentagon" },
    { label: "六边形", value: "hexagon" },
    { label: "八边形", value: "octagon" },
    { label: "五角星", value: "star" },
  ];
  const NODE_SIZE_OPTIONS = [12, 14, 16, 18, 20, 22, 24, 28, 32];
  const COMMON_COLORS = [
    "#0f172a",
    "#1f2937",
    "#334155",
    "#475569",
    "#64748b",
    "#94a3b8",
    "#1f6feb",
    "#0ea5e9",
    "#10b981",
    "#22c55e",
    "#f59e0b",
    "#ef4444",
    "#a855f7",
    "#ec4899",
  ];
  const GRAPH_THEME_STYLE_DEFAULTS = Object.freeze({
    light: Object.freeze({
      edgeColor: "#1f2937",
      nodeColor: "#1f6feb",
      textColor: "#0f172a",
      nodeFill: "rgba(255,255,255,0.05)",
    }),
    dark: Object.freeze({
      edgeColor: "rgba(148,163,184,.72)",
      nodeColor: "#8ea0ff",
      textColor: "rgba(236,242,255,.94)",
      nodeFill: "rgba(148,163,184,.14)",
    }),
  });
  const GRAPH_THEME_STYLE_KEYS = ["edgeColor", "nodeColor", "textColor", "nodeFill"];
  const GRAPH_THEME_MANAGED_VALUE_ALIASES = Object.freeze({
    edgeColor: Object.freeze(["rgba(2,6,23,.68)", "rgba(148,163,184,.72)"]),
    nodeColor: Object.freeze(["rgba(2,6,23,.72)", "rgba(148,163,184,.78)"]),
    textColor: Object.freeze([
      "rgba(2,6,23,.86)",
      "rgba(2,6,23,.82)",
      "rgba(2,6,23,.72)",
      "rgba(236,242,255,.94)",
      "rgba(236,242,255,.9)",
      "rgba(203,213,225,.78)",
    ]),
    nodeFill: Object.freeze(["rgba(255,255,255,0.05)", "rgba(148,163,184,.14)"]),
  });

  function normalizeGraphThemeToken(value) {
    return String(value || "")
      .trim()
      .replace(/\s+/g, "")
      .toLowerCase();
  }

  function resolveGraphThemeName() {
    try {
      const attr = String(document.documentElement?.dataset?.theme || document.documentElement?.getAttribute?.("data-theme") || "")
        .trim()
        .toLowerCase();
      if (attr.includes("dark")) return "dark";
    } catch (e) {}
    return "light";
  }

  function createGraphThemeStyle(themeName = resolveGraphThemeName()) {
    const palette = GRAPH_THEME_STYLE_DEFAULTS[themeName] || GRAPH_THEME_STYLE_DEFAULTS.light;
    return { ...palette };
  }

  function createDefaultGraphStyle() {
    return {
      ...createGraphThemeStyle(),
      edgeWidth: 2,
      nodeWidth: 2,
      nodeSize: 24,
      nodeShape: "circle",
      fontFamily: FONT_LIST[0]?.value || "PingFang SC",
      fontSize: 13,
      fontBold: false,
      fontItalic: false,
      fontUnderline: false,
      fontShadow: false,
      nodeShadow: false,
      edgeDash: "solid",
      edgeArrow: "end",
      iconSize: 16,
      iconSymbol: "",
    };
  }

  function isGraphThemeManagedValue(value, key) {
    const token = normalizeGraphThemeToken(value);
    if (!token) return true;
    const values = [
      GRAPH_THEME_STYLE_DEFAULTS.light[key],
      GRAPH_THEME_STYLE_DEFAULTS.dark[key],
      ...(GRAPH_THEME_MANAGED_VALUE_ALIASES[key] || []),
    ].map(normalizeGraphThemeToken);
    return values.includes(token);
  }

  function isGraphThemeManagedStyle(style) {
    const source = style && typeof style === "object" ? style : {};
    return GRAPH_THEME_STYLE_KEYS.every((key) => isGraphThemeManagedValue(source[key], key));
  }

  function buildGraphThemeItemPatch(model, kind, palette) {
    const source = model && typeof model === "object" ? model : {};
    const patch = {};
    const assignIfManaged = (field, key, value) => {
      if (isGraphThemeManagedValue(source[field], key)) patch[field] = value;
    };
    if (kind === "node") {
      assignIfManaged("stroke", "nodeColor", palette.nodeColor);
      assignIfManaged("fill", "nodeFill", palette.nodeFill);
      assignIfManaged("textColor", "textColor", palette.textColor);
      assignIfManaged("subTextColor", "textColor", palette.textColor);
    } else if (kind === "edge") {
      assignIfManaged("stroke", "edgeColor", palette.edgeColor);
      assignIfManaged("textColor", "textColor", palette.textColor);
    }
    return patch;
  }
  const PERF_NODE_THRESHOLD = 300;
  const PERF_EDGE_THRESHOLD = 800;
  const PERF_OPTIMIZE_ZOOM = 0.6;
  // Keep labels visible for analysis; only hide when zoom is extremely small.
  const LOD_LABEL_HIDE_ZOOM = 0.18;
  const LOD_LABEL_SHOW_ZOOM = 0.24;
  const LOD_SHADOW_HIDE_ZOOM = 0.55;
  const LOD_SHADOW_SHOW_ZOOM = 0.72;
  // Batch edges only for very large graphs; avoid layout collapse on mid-size graphs.
  const EDGE_BATCH_THRESHOLD = 1500;
  const EDGE_BATCH_SIZE = 80;
  const EDGE_BATCH_FINAL_REFRESH_THRESHOLD = 480;
  const GRAPH_ITEM_REFRESH_BATCH_SIZE = 240;
  const GRAPH_ITEM_REFRESH_BATCH_TIMEOUT_MS = 96;
  const STATS_FAST_FIRST_PAINT_MAX_NODES = 10000;
  const STATS_FAST_FIRST_PAINT_MAX_EDGES = 32000;
  const graphRenderModel = window.AnalytixGraphRenderModel;
  if (!graphRenderModel) throw new Error("graph-render-model-missing");
  const {
    resolveGraphTierByNodeCount,
    normalizeGraphTier,
    buildDefaultGraphRenderHints,
    normalizeGraphRenderHints,
    resolveNodeBaseRadius,
    resolveNodeBaseLineWidth,
    applyGraphRenderPlanUpdates,
    syncGraphRenderItems,
    applyGraphNodeTargets,
    buildGraphPositionUpdates,
    summarizeGraphItemLayout,
    isLayoutCollapsed,
    syncGraphItemPositionsFromData,
  } = graphRenderModel;
  const LAYOUT_SWITCH_ANIMATE_MAX_NODES = 1200;
  const LAYOUT_SWITCH_ANIMATE_MAX_EDGES = 3600;
  const DELETE_VISUAL_GUARD_DELAYS = [60, 180, 420, 900, 1500];
  const LAYOUT_WORKER_TIMEOUT_MS = 2800;
  const LAYOUT_ALGO_VERSION = flowLayoutRuntimeAdapter.getLayoutAlgoVersion();
  const LAYOUT_WORKER_SCRIPT = flowLayoutRuntimeAdapter.createLayoutWorkerScript(LAYOUT_ALGO_VERSION);
  const GRAPH_LAYOUT_CONFIG = Object.freeze({
    MAX_PROMOTION_HOPS: 4,
    CORE_NEIGHBORHOOD_HOPS: 2,
    MAX_SHORTEST_PATHS: 24,
    MAX_PROMOTION_CORE_PAIRS: 320,
    SUPPRESS_NOISY_PATH_PROMOTION: true,
    ENABLE_SEED_PROTECTION: true,
    SEED_DEMOTION_BYPASS_WEIGHT: null,
    R_ADJ_MIN: 74,
    R_ADJ_MAX: 192,
    R_LEAF_MIN: 220,
    COMPACT_CORE_SPACING: 108,
    COMPACT_ADJ_SPACING: 86,
    COMPACT_RING_GAP: 56,
    COMPACT_CLUSTER_MARGIN: 110,
    NETWORK_CORE_SPACING: 152,
    NETWORK_ADJ_SPACING: 118,
    NETWORK_SECTOR_MIN_ANGLE: Math.PI / 5.5,
    NETWORK_SECTOR_MAX_ANGLE: Math.PI * 1.55,
    NETWORK_SECTOR_LAYER_GAP: 68,
    NETWORK_CLUSTER_MARGIN: 136,
    ANIM_DURATION: 520,
    ANIM_EASING: "easeInOutCubic",
    LABEL_LAG_MS: 40,
    ANGULAR_BUCKETS: 36,
  });
  const layoutGraphApplyController = createLayoutGraphApplyController({
    buildLayoutNodeTargets,
    resolveLayoutApplyPlan,
    animateToLayout,
    applyGraphNodeTargets,
    buildGraphPositionUpdates,
    centerGraphOnLayout,
    finalizeView,
    updateMinimap,
    scheduleProjectionLayoutIndexSync,
    publishGraphPatch,
    graphLayoutConfig: GRAPH_LAYOUT_CONFIG,
  });
  const LAYOUT_DEBUG_ROW_LIMIT = 160;
  const LAYOUT_LOG_ROLE_SAMPLE_LIMIT = 10;
  const LAYOUT_LOG_CLUSTER_SAMPLE_LIMIT = 6;
  const LAYOUT_LOG_PATH_PAIR_SAMPLE_LIMIT = 8;
  const LAYOUT_LOG_PROMOTED_NODE_SAMPLE_LIMIT = 8;
  const EDGE_INFO_POPOVER_DEBOUNCE_MS = 120;
  const EDGE_INFO_POPOVER_CACHE_TTL_MS = 6000;
  const EDGE_INFO_LOG_WINDOW_MS = 450;
  const EDGE_TXN_DETAIL_PAGE_LIMIT = 500;

  const ICON_LIBRARY = [
    {
      id: "company",
      label: "公司",
      icons: ["ico-company-1", "ico-company-2", "ico-company-3", "ico-company-4", "ico-company-5", "ico-company-6"],
    },
    {
      id: "person",
      label: "人员",
      icons: ["ico-person-1", "ico-person-2", "ico-person-3", "ico-person-4", "ico-person-5", "ico-person-6"],
    },
    {
      id: "media",
      label: "账号介质",
      icons: ["ico-media-1", "ico-media-2", "ico-media-3", "ico-media-4", "ico-media-5", "ico-media-6"],
    },
    {
      id: "network",
      label: "网络设备",
      icons: ["ico-network-1", "ico-network-2", "ico-network-3", "ico-network-4", "ico-network-5", "ico-network-6"],
    },
    {
      id: "address",
      label: "地址",
      icons: ["ico-address-1", "ico-address-2", "ico-address-3", "ico-address-4", "ico-address-5", "ico-address-6"],
    },
    {
      id: "risk",
      label: "风险标识",
      icons: ["ico-risk-1", "ico-risk-2", "ico-risk-3", "ico-risk-4", "ico-risk-5", "ico-risk-6"],
    },
  ];

  const iconDataUrlCache = new Map();
  const reactContextMenuActions = new Map();
  let reactContextMenuSeq = 0;
  let activePopoverAnchor = null;

  const state = {
    inited: false,
    backend: null,
    backendInit: false,

    caseId: "",
    caseName: "",

    leftCollapsed: false,

    tab: "byName",
    search: "",
    treeData: [],
    treeSemanticStatus: "uninitialized",
    expanded: new Set(),
    selected: new Set(),
    treeExpandedMap: new Map(), // session only

    dir: "all",
    hop: 1,
    minAmount: 0,
    maxEdges: 800,

    graph: {
      inited: false,
      instance: null,
      data: { nodes: [], edges: [] },
      runtimeRevision: 0,
      layout: { type: "force" },
    },
    graphTier: "small",
    renderHints: null,
    graphMode: "relation",
    layoutPreset: "compact",
    layoutDirection: { hierarchy: "down", flow: "right" },
    layoutEdgeRouting: { hierarchy: false, flow: false },
    layoutSelected: false,
    layoutTopologyDirty: false,
    layoutWorker: null,
    layoutWorkerSeq: 0,
    layoutWorkerPending: null,
    layoutWorkerStage: "",
    layoutWorkerDisabledReason: "",
    lastAppliedLayoutCacheKey: "",
    graphBuildSeq: 0,
    layoutPrewarmTasks: {
      network: null,
      compact: null,
    },
    focusId: "",
    focusName: "",
    focusLabel: "",
    focusIds: [],
    focusNames: [],
    focusPlaceholderKinds: [],
    focusKeyType: "",
    focusUnknownName: false,
    focusCounterpartyStrict: false,
    source: "",
    requestId: "",
    expectedTotalAmount: null,
    expectedRowCount: 0,
    focusOnly: false,
    focusSelfOnly: false,
    focusSelfOnlyOnce: false,
    includeMissingCounterparty: false,
    lastRequest: null,

    views: [],
    activeViewId: "",
    viewCounter: 0,
    viewsLoaded: false,

    lastActiveId: "",
    edgeDirectionLocked: false,
    createNodeMode: false,
    ghostNodeId: "",
    introAnimating: false,
    introAnimSeq: 0,
    introSuppressUntil: 0,
    pulseAnimating: false,
    pulseRaf: 0,
    pulseStart: 0,
    modifiers: { ctrl: false, meta: false },
    lastPointer: null,
    nodeCounter: 0,
    perfMode: false,
    edgeLabelTarget: null,
    edgeLabelDialogSeq: 0,
    edgeInfoPopoverSeq: 0,
    edgeInfoActiveKey: "",
    edgeInfoOpenAt: 0,
    edgeInfoDebounceTimer: 0,
    edgeInfoDebounceKey: "",
    edgeInfoContext: null,
    edgeInfoTxnInflight: new Map(),
    edgeInfoTxnCache: new Map(),
    edgeInfoLogDedup: new Map(),
    graphTraceWarnDedup: new Map(),
    graphTraceSummaryDedup: new Map(),
    ghostProbeDedup: new Map(),
    lastGraphContainerSize: { w: 0, h: 0 },
    lastMutationReason: "",
    lastMutationAt: 0,
    graphMutationStore: null,
    graphMutationCommands: null,
    graphClipboardText: "",
    canvasDragController: null,
    canvasDrillController: null,
    canvasHitViewportController: null,
    canvasInteractionStore: null,
    canvasRedrawController: null,
    flowDebugDiagnostics: null,
    deleteProbeSeq: 0,
    lastDeleteProbeId: "",
    lastDeleteProbeAt: 0,
    deleteVisualGuardSeq: 0,
    deleteVisualGuardTimers: [],
    edgeLaneById: Object.create(null),
    signedEdgeDetail: false,
    viewCloseResolver: null,
    undoStack: [],
    initialStyleSnapshot: null,
    undoLocked: false,
    gpuLogged: false,
    edgeBatchSeq: 0,
    edgeBatching: false,
    graphMutationSeq: 0,
    directRenderSeq: 0,
    graphLoading: false,
    graphSearchQuery: "",
    graphSearchFocusToken: 0,
    pendingFit: false,
    interactionPerf: false,
    interactionPerfRestore: null,
    interactionPerfTimer: 0,
    lod: {
      edgeLabelsHidden: false,
      shadowsReduced: false,
    },
    networkLayout: {
      viewMode: "full", // auto | skeleton | full
      activeMode: "full",
      autoNodeThreshold: 180,
      autoEdgeThreshold: 260,
      crossingThreshold: 320,
      longEdgeRatioThreshold: 0.1,
      leafTopKPerCommunity: 6,
      labelZoomBridge: 1.0,
      labelZoomCluster: 1.2,
      labelZoomLeaf: 1.35,
      expandedCommunities: new Set(),
      lastCompactCoreIds: [],
      lastDecision: null,
      planSeq: 0,
      lastPlanQuality: null,
      lastPlanSampleQuality: null,
      lastPlanCommunityQuality: [],
      lastPlanLod: null,
      lastPlanSupergraph: null,
      lastPlanMode: "",
      lastPlanGraphSignature: "",
      lastPlanAttempt: null,
      lastPlanDrop: null,
      topologyPlanCache: {},
      topologyPlanCacheMaxEntries: 4,
      lastViewportQuality: null,
      communityQualitySeq: 0,
      communityQualityTimer: 0,
      lastCommunityQualityAttempt: null,
      lastCommunityQualityDrop: null,
      lastCommunityQualityStaleDrop: null,
      viewportRefineTimer: 0,
      viewportRefineSignature: "",
      viewportRefineLastAt: 0,
      hiddenNodeCount: 0,
      hiddenEdgeCount: 0,
      perf: null,
    },
    layoutSemanticState: {
      clusterAnchorBySignature: {},
      clusterCenterBySignature: {},
      clusterSlotBySignature: {},
      layoutPlanCache: {},
      layoutCacheKeyProjection: null,
      layoutPlanCacheStats: {},
      seedCoreHistory: {},
      roleByNodeId: {},
      clusterByNodeId: {},
      layoutCaseByClusterId: {},
      lastReports: null,
    },
    lodRaf: 0,
    graphConsistencyRaf: 0,
    graphConsistencyMuteUntil: 0,
    graphResizeHandler: null,
    analysis: createAnalysisState(),
    analysisSourceData: null,
    analysisCollapsedChildMap: {},
    analysisCollapsedCoreIds: [],
    edgeDetail: false,

    graphStyle: createDefaultGraphStyle(),
    graphThemeName: resolveGraphThemeName(),
    graphThemeObserver: null,
    iconLibrary: { tab: "company", query: "" },
    ribbonStyle: null,
    recentColors: [],
    exportScale: 1,
    reactOverlay: flowRuntimeStateAdapter.createReactOverlayState(),
    directRenderSeq: 0,
    debugDrill: false,
    debugLayout: false,
    debugPerf: false,
    debugTrace: false,
    lastBuildGraphPerf: null,
    resultSnapshotRef: null,
    graphProjection: null,
    graphExpandSeq: 0,
    pointLayerOverlay: flowRuntimeStateAdapter.createPointLayerOverlayState(),
    projectionAutoMaterialize: flowRuntimeStateAdapter.createProjectionAutoMaterializeState(),
    projectionLayoutSync: flowRuntimeStateAdapter.createProjectionLayoutSyncState(),
    txnDetailWidths: null,
    txnModal: flowRuntimeStateAdapter.createTxnModalState(),
    drillConfig: flowRuntimeStateAdapter.createDrillConfigState(),
    perfMetrics:
      flowObservabilityAdapter && typeof flowObservabilityAdapter.createFlowPerfMetrics === "function"
        ? flowObservabilityAdapter.createFlowPerfMetrics()
        : null,
  };

  try {
    if (typeof window.__ANALYTIX_DEBUG_PERF === "boolean") {
      state.debugPerf = window.__ANALYTIX_DEBUG_PERF;
    }
  } catch (e) {}
  try {
    if (typeof window.__ANALYTIX_DEBUG_TRACE === "boolean") {
      state.debugTrace = window.__ANALYTIX_DEBUG_TRACE;
    }
  } catch (e) {}

  function networkPerfNow() {
    try {
      if (performance && typeof performance.now === "function") return performance.now();
    } catch (_error) {}
    return Date.now();
  }

  function roundNetworkPerfMs(value) {
    const number = Number(value);
    return Number.isFinite(number) ? Math.max(0, Math.round(number * 100) / 100) : 0;
  }

  function ensureNetworkRuntimePerf(reason = "") {
    if (!state.networkLayout || typeof state.networkLayout !== "object") return null;
    const existing =
      state.networkLayout.perf && typeof state.networkLayout.perf === "object" ? state.networkLayout.perf : null;
    if (existing) return existing;
    const now = networkPerfNow();
    state.networkLayout.perf = {
      seq: 0,
      reason: String(reason || ""),
      startedAt: now,
      startedAtWall: Date.now(),
      events: [],
      durations: {},
      budgets: {
        syncStageMs: 50,
        coarseSeedMs: 50,
        skeletonFirstPaintMs: 200,
        viewportRefineMs: 1200,
      },
      maxSynchronousStageMs: 0,
      lastEvent: null,
    };
    return state.networkLayout.perf;
  }

  function resetNetworkRuntimePerf(reason = "", detail = {}) {
    if (!state.networkLayout || typeof state.networkLayout !== "object") return null;
    const previous = state.networkLayout.perf && typeof state.networkLayout.perf === "object" ? state.networkLayout.perf : {};
    const now = networkPerfNow();
    const perf = {
      seq: Math.max(0, Number(previous.seq) || 0) + 1,
      reason: String(reason || ""),
      startedAt: now,
      startedAtWall: Date.now(),
      events: [],
      durations: {},
      budgets: {
        syncStageMs: 50,
        coarseSeedMs: 50,
        skeletonFirstPaintMs: 200,
        viewportRefineMs: 1200,
      },
      maxSynchronousStageMs: 0,
      lastEvent: null,
      ...(detail && typeof detail === "object" ? { startDetail: { ...detail } } : {}),
    };
    state.networkLayout.perf = perf;
    return recordNetworkRuntimePerfEvent("network-switch-requested", detail);
  }

  function recordNetworkRuntimePerfEvent(stage = "", detail = {}) {
    const name = String(stage || "").trim();
    if (!name) return null;
    const perf = ensureNetworkRuntimePerf(name);
    if (!perf) return null;
    const now = networkPerfNow();
    const offsetMs = roundNetworkPerfMs(now - Number(perf.startedAt || now));
    const event = {
      stage: name,
      at: Date.now(),
      offsetMs,
      ...(detail && typeof detail === "object" ? { ...detail } : {}),
    };
    const durationMs = Number(event.durationMs);
    if (Number.isFinite(durationMs)) {
      const roundedDuration = roundNetworkPerfMs(durationMs);
      event.durationMs = roundedDuration;
      perf.durations[name] = roundedDuration;
      if (event.synchronous !== false) {
        perf.maxSynchronousStageMs = Math.max(Number(perf.maxSynchronousStageMs) || 0, roundedDuration);
      }
    }
    if (name === "preset-immediate" || name === "preset-applied") {
      perf.presetSwitchMs = offsetMs;
    } else if (name === "coarse-seed-applied") {
      perf.coarseSeedMs = Number(event.durationMs) || 0;
    } else if (name === "skeleton-rendered") {
      perf.skeletonFirstPaintMs = offsetMs;
      perf.skeletonRenderMs = Number(event.durationMs) || 0;
    } else if (name === "topology-plan-response") {
      perf.finalPlanResponseMs = offsetMs;
    } else if (name === "topology-plan-applied") {
      perf.finalPlanApplyMs = Number(event.durationMs) || 0;
      perf.finalPlanReadyMs = offsetMs;
    } else if (name === "viewport-refined") {
      perf.viewportRefineApplyMs = Number(event.durationMs) || 0;
      perf.viewportRefineReadyMs = offsetMs;
    }
    const events = Array.isArray(perf.events) ? perf.events : [];
    events.push(event);
    while (events.length > 48) events.shift();
    perf.events = events;
    perf.lastEvent = event;
    return event;
  }

  function yieldToMainThread() {
    if (typeof MessageChannel === "function") {
      return new Promise((resolve) => {
        const channel = new MessageChannel();
        channel.port1.onmessage = () => {
          channel.port1.close();
          channel.port2.close();
          resolve();
        };
        channel.port2.postMessage(0);
      });
    }
    return new Promise((resolve) => setTimeout(resolve, 0));
  }

  const layoutWorkerLifecycleController = createLayoutWorkerLifecycleController({
    state,
    performanceApi: performance,
    clearTimeoutFn: clearTimeout,
    recordLayoutWorkerMetrics,
    logGraphTrace,
    log,
    computeGraphTotalAmount,
    resetLayoutWorkerInstance,
    applyPendingLayoutFallback,
    updateLayoutButtons,
    scheduleReinit: () => setTimeout(() => initLayoutWorker(), 0),
  });
  const layoutWorkerSchedulerController = createLayoutWorkerSchedulerController({
    state,
    runtimeAdapter: flowLayoutRuntimeAdapter,
    lifecycleController: layoutWorkerLifecycleController,
    recordLayoutWorkerMetrics,
    resetLayoutWorkerInstance,
    createLayoutTraceId,
    logLayoutCoreMissing,
    graphLayoutConfig: GRAPH_LAYOUT_CONFIG,
    timeoutBaseMs: LAYOUT_WORKER_TIMEOUT_MS,
    setTimeoutFn: setTimeout,
    clearTimeoutFn: clearTimeout,
    performanceApi: performance,
  });

  const shellBridgeListeners = new Set();
  let shellSyncStore = null;
  let layoutCommandStore = null;
  let shellStateStore = null;
  let sessionStore = null;
  let graphCommandStore = null;
  let selectionStore = null;
  let viewSyncStore = null;
  let minimapStore = null;

  function ensureShellSyncStore() {
    if (!shellSyncStore) {
      shellSyncStore = flowShellSyncStoreAdapter.createFlowShellSyncStore({
        runtimeStore,
        listeners: shellBridgeListeners,
        buildShellState: () => buildShellState(),
        targetWindow: window,
      });
    }
    return shellSyncStore;
  }

  function ensureShellStateStore() {
    if (!shellStateStore) {
      shellStateStore = flowShellStateStoreAdapter.createFlowShellStateStore({
        state,
        deps: {
          canvasShellAdapter,
          getSelectionState: (graph) => getSelectionState(graph),
          buildShellViews: () => buildShellViews(),
          buildShellStyleState: () => buildShellStyleState(),
          buildShellOverlayState: () => buildShellOverlayState(),
          buildProjectionShellState: () => buildProjectionShellState(),
          buildGraphStatsSummary,
          layoutPresetLabel,
          hasAnalysisAmountFilter,
          clampNumber,
        },
      });
    }
    return shellStateStore;
  }

  function ensureLayoutCommandStore() {
    if (!layoutCommandStore) {
      layoutCommandStore = flowLayoutCommandStoreAdapter.createFlowLayoutCommandStore({
        state,
        deps: {
          queryAll: $$,
          scheduleShellStateSync: (reason) => scheduleShellStateSync(reason),
          terminateLayoutPrewarmTask,
          restoreNetworkPresentation,
          reapplyGraphTierVisualState,
          clearGraphSelection: (graph) => clearGraphSelection(graph),
          syncGraphNodePositions,
          captureNodePositions,
          getGraphDataBounds,
          resolveFocusIdFromNodes,
          applyLayout,
          updateMinimap: (options) => updateMinimap(options),
          shouldAnimateLayoutSwitch,
          animateToLayout,
          finalizeView,
          scheduleProjectionLayoutIndexSync,
          publishGraphPatch,
          createLayoutTraceId,
          applyGraphNodeTargets,
          buildGraphPositionUpdates,
          ensureGraphDataVisible,
          resolveEdgeRefreshBatchSize,
          resolveEdgeBatchSize,
          scheduleGraphItemRefreshBatches,
          graphItemRefreshBatchTimeoutMs: GRAPH_ITEM_REFRESH_BATCH_TIMEOUT_MS,
          edgeBatchFinalRefreshThreshold: EDGE_BATCH_FINAL_REFRESH_THRESHOLD,
          graphLayoutConfig: GRAPH_LAYOUT_CONFIG,
        },
      });
    }
    return layoutCommandStore;
  }

  function ensureSessionStore() {
    if (!sessionStore) {
      sessionStore = flowSessionStoreAdapter.createFlowSessionStore({
        state,
        deps: {
          log,
          setCaseBadge,
          loadTreeData,
          syncFundsStatus,
          clearGraph,
          loadViewsFromBackend,
        },
      });
    }
    return sessionStore;
  }

  function ensureGraphCommandStore() {
    if (!graphCommandStore) {
      graphCommandStore = flowGraphCommandStoreAdapter.createFlowGraphCommandStore({ state });
    }
    return graphCommandStore;
  }

  function ensureSelectionStore() {
    if (!selectionStore) {
      selectionStore = flowSelectionStoreAdapter.createFlowSelectionStore({
        state,
        deps: {
          getSelectedGraphItems,
          clearCanvasSelection,
          startPulseLoop,
          stopPulseLoop,
          scheduleShellStateSync: (reason) => scheduleShellStateSync(reason),
        },
      });
    }
    return selectionStore;
  }

  function ensureViewSyncStore() {
    if (!viewSyncStore) {
      viewSyncStore = flowViewSyncStoreAdapter.createFlowViewSyncStore({
        deps: {
          getActiveView,
          snapshotGraphData,
          captureGraphViewport: (targetGraph) => captureGraphViewport(targetGraph),
          nowIso,
          syncAnalysisSourceDataFromCurrent,
          viewCounts,
          query: $,
        },
      });
    }
    return viewSyncStore;
  }

  function ensureMinimapStore() {
    if (!minimapStore) {
      minimapStore = graphMinimapOrchestrationAdapter.createFlowMinimapStore({
        state,
        selectMinimapEl: () => $("#minimap"),
        getGraph: () => state.graph.instance,
        getNodes: () => state.graph.data.nodes,
        getGraphDataBounds,
        clampNumber,
        applyGraphModes,
        log,
        nodeThreshold: PERF_NODE_THRESHOLD,
        edgeThreshold: PERF_EDGE_THRESHOLD,
      });
    }
    return minimapStore;
  }
  const reactDrawerActions = new Map();

  function recordTextAtlasMetrics(snapshot = null) {
    if (!state.perfMetrics || !flowObservabilityAdapter || typeof flowObservabilityAdapter.recordTextAtlas !== "function") {
      return snapshot;
    }
    return flowObservabilityAdapter.recordTextAtlas(state.perfMetrics, snapshot);
  }

  function recordLayoutWorkerMetrics(event, details = {}) {
    if (!state.perfMetrics || !flowObservabilityAdapter || typeof flowObservabilityAdapter.recordLayoutWorker !== "function") {
      return;
    }
    flowObservabilityAdapter.recordLayoutWorker(state.perfMetrics, event, details);
  }

  function createLayoutTraceId(preset = "") {
    const mode = String(preset || state.layoutPreset || "compact").trim() || "compact";
    const nonce = Math.random().toString(36).slice(2, 8);
    return `flow-layout:${mode}:${Date.now().toString(36)}:${nonce}`;
  }

  const flowDebugSurface =
    flowDebugSurfaceAdapter && typeof flowDebugSurfaceAdapter.createFlowDebugSurface === "function"
      ? flowDebugSurfaceAdapter.createFlowDebugSurface({
          state,
          ensureGraph,
          getGraphEngine,
          ensureLayoutSemanticState,
          getLayoutEngine,
          recordTextAtlasMetrics,
          flowObservabilityAdapter,
          getNetworkSectorPlacementCacheSnapshot: () => networkSectorPlacementCache.snapshot(),
        })
      : null;

  state.graphMutationStore =
    graphMutationStoreAdapter && typeof graphMutationStoreAdapter.createGraphMutationStore === "function"
      ? graphMutationStoreAdapter.createGraphMutationStore({
          patchAdapter: graphPatchAdapter,
          getBackend: () => state.backend,
          getGraphData: () => state.graph?.data || null,
          getActiveViewId: () => state.activeViewId || "",
          getLayoutPreset: () => state.layoutPreset || "",
          onRevisionChange: (runtimeRevision) => {
            if (Number.isFinite(runtimeRevision)) {
              state.graph.runtimeRevision = runtimeRevision;
            }
          },
        })
      : null;
  state.graphMutationCommands =
    graphMutationStoreAdapter && typeof graphMutationStoreAdapter.createGraphMutationCommandStore === "function"
      ? graphMutationStoreAdapter.createGraphMutationCommandStore({
          getState: () => state,
          cloneGraphData,
          getGraphMutationStore: () => state.graphMutationStore,
          cloneGraphProjection,
          cloneResultSnapshotRef,
          normalizeGraphTier,
          normalizeGraphRenderHints,
          getGraphMode: () => state.graphMode || "relation",
          scheduleProjectionPointLayerRedraw: (reason) => scheduleProjectionPointLayerRedraw(reason),
          rebindLayoutWorkerPendingGraphData,
          updatePerfMode,
          onGraphDataReplaced: (nextGraph, meta = {}) => {
            state.graph.data = nextGraph;
            noteNetworkGraphDataReplacement(nextGraph, meta);
          },
        })
      : null;
  state.canvasInteractionStore =
    canvasInteractionStoreAdapter && typeof canvasInteractionStoreAdapter.createCanvasInteractionStore === "function"
      ? canvasInteractionStoreAdapter.createCanvasInteractionStore({
          getGraph: () => state.graph?.instance || null,
          getState: () => state,
          getSelectedGraphItems,
          stopPulseLoop,
          syncActiveSelection,
          syncEdgeDirectionControl,
          updateMinimap: () => updateMinimap(),
          scheduleLodUpdate: (graph) => scheduleLodUpdate(graph),
          resolveClientPointFromEvent,
          captureGraphViewportImpl: (targetGraph) => captureGraphViewport(targetGraph),
          applyGraphViewportImpl: (viewport, targetGraph) => applyGraphViewport(viewport, targetGraph),
        })
      : null;

  function disposeCanvasDragController() {
    if (state.canvasDragController && typeof state.canvasDragController.dispose === "function") {
      try {
        state.canvasDragController.dispose();
      } catch (e) {}
    }
    state.canvasDragController = null;
  }

  function ensureCanvasDragController(graph, usingWebglEngine = false) {
    if (!graph) {
      disposeCanvasDragController();
      return null;
    }
    if (!canvasDragControllerAdapter || typeof canvasDragControllerAdapter.createCanvasDragController !== "function") {
      disposeCanvasDragController();
      return null;
    }
    disposeCanvasDragController();
    state.canvasDragController = canvasDragControllerAdapter.createCanvasDragController({
      graph,
      state,
      usingWebglEngine,
      isPerfMode,
      setInteractionPerf,
      log,
      syncGraphNodePositions,
      applyLayoutEdgeRouting,
      syncGraphEdgeRoutingFromData,
      updateCurrentViewSnapshot,
      updateMinimap,
      scheduleGraphPatch,
    });
    return state.canvasDragController;
  }

  function ensureCanvasRedrawController() {
    if (state.canvasRedrawController) return state.canvasRedrawController;
    if (!canvasRedrawControllerAdapter || typeof canvasRedrawControllerAdapter.createCanvasRedrawController !== "function") {
      return null;
    }
    state.canvasRedrawController = canvasRedrawControllerAdapter.createCanvasRedrawController({
      state,
      ensureGraph,
      isDeleteReason,
      isDeleteProbeVerbose,
      shouldLogConsistencyProbeNormal,
      shouldAlwaysRecreateAfterDelete,
      shouldRunDeleteVisualGuard,
      graphEdgeIdentityKey,
      collectGhostProbeCounts,
      isGraphRuntimeStable,
      cancelGraphTransientTasks,
      logGhostProbe,
      log,
      captureGraphViewport,
      applyGraphViewport,
      ensureGraphDataVisible,
      cloneGraphData,
      runCanvasRedraw,
      updateMinimap,
      applyStyleToGraph,
      applyAnalysisEncodings,
      applyEdgeDetailLabels,
      syncEdgeDetailButton,
      destroyMinimap,
      beforeGraphRecreate: () => {
        disposeCanvasDragController();
      },
    });
    return state.canvasRedrawController;
  }

  function ensureCanvasHitViewportController() {
    if (state.canvasHitViewportController) return state.canvasHitViewportController;
    if (
      !canvasHitViewportControllerAdapter ||
      typeof canvasHitViewportControllerAdapter.createCanvasHitViewportController !== "function"
    ) {
      return null;
    }
    state.canvasHitViewportController = canvasHitViewportControllerAdapter.createCanvasHitViewportController({
      ensureGraph,
      clampNumber,
      updateMinimap,
      scheduleLodUpdate,
      getCanvasInteractionStore: () => state.canvasInteractionStore,
      extractDomEventImpl: extractDomEvent,
    });
    return state.canvasHitViewportController;
  }

  function ensureCanvasDrillController() {
    if (state.canvasDrillController) return state.canvasDrillController;
    if (!canvasDrillControllerAdapter || typeof canvasDrillControllerAdapter.createCanvasDrillController !== "function") {
      return null;
    }
    state.canvasDrillController = canvasDrillControllerAdapter.createCanvasDrillController({
      state,
      txnDrillState,
      ensureGraph,
      captureNodePositions,
      layoutScale,
      applyEdgeOffsets: applyProjectedEdgeOffsets,
      replaceRuntimeGraphData,
      applyStyleToGraph,
      applyAnalysisEncodings,
      applyEdgeDetailLabels,
      syncEdgeDetailButton,
      animateToLayout,
      syncGraphNodePositions,
      updateGraphStats,
      updateCurrentViewSnapshot,
      focusGraphItemById,
      scheduleGraphPatch,
      parseTxnTimeValue,
      fetchAccountTxnRows,
      normalizeAccountTxnRows,
      computeDrillOutflows: (rows, baseTxn) => computeDrillOutflows(rows, baseTxn, getDrillConfig()),
      buildTxnNodeId: buildGraphTxnNodeId,
      buildTxnNodeModel: (payload) =>
        buildGraphTxnNodeModel(payload, {
          style: state.graphStyle || {},
          theme: state.graphThemeName || resolveGraphThemeName(),
          defaultFontFamily: FONT_LIST[0]?.value || "PingFang SC",
        }),
      buildTxnEdgeModel: (payload) =>
        buildGraphTxnEdgeModel(payload, {
          style: state.graphStyle || {},
          theme: state.graphThemeName || resolveGraphThemeName(),
          defaultFontFamily: FONT_LIST[0]?.value || "PingFang SC",
        }),
      updateGraphDataById,
      getGraphNodeInfo,
      getGraphExtremes,
      log,
      toast,
      getDrillConfig,
    });
    return state.canvasDrillController;
  }

  function ensureFlowDebugDiagnostics() {
    if (state.flowDebugDiagnostics) return state.flowDebugDiagnostics;
    if (!flowDebugDiagnosticsAdapter || typeof flowDebugDiagnosticsAdapter.createFlowDebugDiagnostics !== "function") {
      return null;
    }
    state.flowDebugDiagnostics = flowDebugDiagnosticsAdapter.createFlowDebugDiagnostics({
      state,
      log,
      getGraphEngine,
      detectWebGLSupport,
      shouldLogCanvasMetrics,
      summarizeGraphData,
      toFiniteNumber,
      computeGraphTotalAmount,
    });
    return state.flowDebugDiagnostics;
  }

  function rememberGraphPatchBase(graph = state.graph?.data || null) {
    if (state.graphMutationStore && typeof state.graphMutationStore.rememberBase === "function") {
      return state.graphMutationStore.rememberBase(graph, { runtimeRevision: state.graph.runtimeRevision || 0 });
    }
    return graph;
  }

  function replaceRuntimeGraphData(nextGraph, options = {}) {
    if (state.graphMutationCommands && typeof state.graphMutationCommands.replaceGraphData === "function") {
      return state.graphMutationCommands.replaceGraphData(nextGraph, options);
    }
    const payload = options?.clone ? cloneGraphData(nextGraph) : nextGraph || { nodes: [], edges: [] };
    state.graph.data = payload;
    return state.graph.data;
  }

  function clearRuntimeGraphData(options = {}) {
    if (state.graphMutationCommands && typeof state.graphMutationCommands.clearGraphData === "function") {
      return state.graphMutationCommands.clearGraphData(options);
    }
    return replaceRuntimeGraphData({ nodes: [], edges: [] }, options);
  }

  function clearCanvasSelection(targetGraph = null, reason = "") {
    if (state.canvasInteractionStore && typeof state.canvasInteractionStore.clearSelection === "function") {
      const result = state.canvasInteractionStore.clearSelection(targetGraph, reason);
      scheduleShellStateSync("graph-selection-clear");
      return result;
    }
    return { nodes: 0, edges: 0, total: 0 };
  }

  function toggleCanvasItemSelection(item, options = {}) {
    if (state.canvasInteractionStore && typeof state.canvasInteractionStore.toggleItemSelection === "function") {
      return state.canvasInteractionStore.toggleItemSelection(item, options || {});
    }
    return false;
  }

  function setCanvasItemHover(item, active, options = {}) {
    if (state.canvasInteractionStore && typeof state.canvasInteractionStore.setItemHover === "function") {
      return state.canvasInteractionStore.setItemHover(item, active, options || {});
    }
    return false;
  }

  function captureCanvasViewport(targetGraph = null, reason = "") {
    if (state.canvasInteractionStore && typeof state.canvasInteractionStore.captureViewport === "function") {
      return state.canvasInteractionStore.captureViewport(targetGraph, reason);
    }
    return captureGraphViewport(targetGraph);
  }

  function applyCanvasViewport(viewport, targetGraph = null, options = {}) {
    if (state.canvasInteractionStore && typeof state.canvasInteractionStore.applyViewport === "function") {
      return state.canvasInteractionStore.applyViewport(viewport, targetGraph, options || {});
    }
    return applyGraphViewport(viewport, targetGraph);
  }

  function syncCanvasViewport(targetGraph = null, reason = "") {
    if (state.canvasInteractionStore && typeof state.canvasInteractionStore.syncViewport === "function") {
      state.canvasInteractionStore.syncViewport(targetGraph, reason);
      scheduleProjectionPointLayerRedraw(`viewport:${reason || "interaction-store"}`);
      scheduleProjectionAutoMaterialize(`viewport:${reason || "interaction-store"}`);
      scheduleNetworkViewportRefinement(`viewport:${reason || "interaction-store"}`);
      return;
    }
    const graph = targetGraph || state.graph?.instance || null;
    updateMinimap();
    if (graph) {
      scheduleLodUpdate(graph);
    }
    scheduleProjectionPointLayerRedraw(`viewport:${reason || "graph"}`);
    scheduleProjectionAutoMaterialize(`viewport:${reason || "graph"}`);
    scheduleNetworkViewportRefinement(`viewport:${reason || "graph"}`);
  }

  function noteCanvasHit(kind, item = null, event = null, reason = "") {
    ensureCanvasHitViewportController()?.noteHit?.(kind, item, event, reason);
  }

  function runCanvasRedraw(reason = "", fn = null) {
    if (state.canvasInteractionStore && typeof state.canvasInteractionStore.runRedraw === "function") {
      return state.canvasInteractionStore.runRedraw(reason, fn);
    }
    return typeof fn === "function" ? fn() : undefined;
  }

  function publishGraphPatch(meta = {}) {
    return ensureGraphCommandStore().publish(meta);
  }

  function scheduleGraphPatch(meta = {}) {
    ensureGraphCommandStore().schedule(meta);
  }

  function buildShellStyleState() {
    return flowRuntimeStateAdapter.buildShellStyleState(state, {
      fontList: FONT_LIST,
      lineStyleOptions: LINE_STYLE_OPTIONS,
      arrowOptions: ARROW_OPTIONS,
      nodeShapeOptions: NODE_SHAPE_OPTIONS,
      findOptionLabel,
      canEditEdgeDirection,
      asBool,
    });
  }

  function buildShellViews() {
    return flowRuntimeStateAdapter.buildShellViews(state, {
      canvasShellAdapter,
      viewCounts,
    });
  }

  function buildShellOverlayState() {
    return flowRuntimeStateAdapter.buildShellOverlayState(state, {
      shellOverlayAdapter,
      overlayOptions: {
        commonColors: COMMON_COLORS,
        fontList: FONT_LIST,
        fontSizeList: FONT_SIZE_LIST,
        lineStyleOptions: LINE_STYLE_OPTIONS,
        lineWidthOptions: LINE_WIDTH_OPTIONS,
        arrowOptions: ARROW_OPTIONS,
        nodeShapeOptions: NODE_SHAPE_OPTIONS,
        nodeSizeOptions: NODE_SIZE_OPTIONS,
        iconLibrary: ICON_LIBRARY,
        getColorValue,
        txnColumns: TXN_DETAIL_COLS,
        buildTxnModalRows,
        ensureTxnDetailWidths,
        sortTxnModalRows,
        isTxnSortableCol,
        isTxnMoneyCol,
        isTxnMonoCol,
        getTxnSortLabel,
        formatTxnMoney,
      },
    });
  }

  function buildShellState() {
    return ensureShellStateStore().build();
  }

  function flushShellStateSync(reason = "") {
    return ensureShellSyncStore().flush(reason);
  }

  function scheduleShellStateSync(reason = "") {
    ensureShellSyncStore().schedule(reason);
  }

  function commitShellStateSync(reason = "") {
    try {
      flushShellStateSync(reason || "graph-commit");
    } catch (e) {}
  }

  function finalizeShellCommandRegistry(definitions) {
    if (canvasCommandAdapter && typeof canvasCommandAdapter.finalizeCanvasCommandRegistry === "function") {
      return canvasCommandAdapter.finalizeCanvasCommandRegistry(definitions || {}, canvasShellAdapter);
    }
    if (canvasShellAdapter && typeof canvasShellAdapter.buildCanvasCommandRegistry === "function") {
      return canvasShellAdapter.buildCanvasCommandRegistry(definitions || {});
    }
    return definitions || {};
  }

  const txnDrillState = {
    loading: false,
    cache: new Map(),
  };

  function isPerfMode() {
    if (state.perfMode) return true;
    const nodes = state.graph?.data?.nodes?.length || 0;
    const edges = state.graph?.data?.edges?.length || 0;
    return nodes >= PERF_NODE_THRESHOLD || edges >= PERF_EDGE_THRESHOLD;
  }

  function detectWebGLSupport() {
    try {
      const canvas = document.createElement("canvas");
      const webgl2 = canvas.getContext("webgl2");
      const webgl =
        webgl2 || canvas.getContext("webgl") || canvas.getContext("experimental-webgl");
      return { webgl: !!webgl, webgl2: !!webgl2 };
    } catch (e) {
      return { webgl: false, webgl2: false };
    }
  }

  function getGraphRendererPreference() {
    const caps = detectWebGLSupport();
    return { preferWebGL: true, renderer: "webgl", caps };
  }

  function logGpuProbe(graph) {
    const diagnostics = ensureFlowDebugDiagnostics();
    diagnostics?.logGpuProbe?.(graph);
  }

  function logGraphDebug(graph, message, extra = {}) {
    const diagnostics = ensureFlowDebugDiagnostics();
    diagnostics?.logGraphDebug?.(graph, message, extra || {});
  }

  function toFiniteNumber(value, digits = null) {
    const num = Number(value);
    if (!Number.isFinite(num)) return null;
    if (digits == null) return num;
    return Number(num.toFixed(Math.max(0, Number(digits) || 0)));
  }

  function truncateTextMiddle(value, max = 96) {
    const text = String(value ?? "");
    const limit = Math.max(16, Number(max) || 96);
    if (text.length <= limit) return text;
    const keep = Math.max(6, Math.floor((limit - 3) / 2));
    return `${text.slice(0, keep)}...${text.slice(-keep)}`;
  }

  function sanitizeTraceCounts(counts) {
    if (!counts || typeof counts !== "object") return null;
    const nodes = Math.max(0, Number(counts.nodes) || 0);
    const edges = Math.max(0, Number(counts.edges) || 0);
    return { nodes: Math.round(nodes), edges: Math.round(edges) };
  }

  function summarizeLayoutBoundsForTrace(layout) {
    if (!layout || typeof layout !== "object") return null;
    const minX = toFiniteNumber(layout.minX);
    const maxX = toFiniteNumber(layout.maxX);
    const minY = toFiniteNumber(layout.minY);
    const maxY = toFiniteNumber(layout.maxY);
    const spanX = minX != null && maxX != null ? Number((maxX - minX).toFixed(2)) : null;
    const spanY = minY != null && maxY != null ? Number((maxY - minY).toFixed(2)) : null;
    return {
      count: Math.max(0, Number(layout.count) || 0),
      finite: Math.max(0, Number(layout.finite) || 0),
      spanX,
      spanY,
    };
  }

  function summarizeSpacingForTrace(spacing) {
    if (!spacing || typeof spacing !== "object") return null;
    return {
      edgeLength: {
        count: Math.max(0, Number(spacing?.edgeLength?.count) || 0),
        min: toFiniteNumber(spacing?.edgeLength?.min, 2),
        max: toFiniteNumber(spacing?.edgeLength?.max, 2),
        avg: toFiniteNumber(spacing?.edgeLength?.avg, 2),
        selfLoops: Math.max(0, Number(spacing?.edgeLength?.selfLoops) || 0),
        missingNode: Math.max(0, Number(spacing?.edgeLength?.missingNode) || 0),
      },
      nodeGap: {
        sampled: Math.max(0, Number(spacing?.nodeGap?.sampled) || 0),
        closePairs: Math.max(0, Number(spacing?.nodeGap?.closePairs) || 0),
        min: toFiniteNumber(spacing?.nodeGap?.min, 2),
      },
    };
  }

  function summarizeOutliersForTrace(outliers) {
    if (!outliers || typeof outliers !== "object") return null;
    const top = Array.isArray(outliers?.top) && outliers.top.length ? outliers.top[0] : null;
    return {
      count: Math.max(0, Number(outliers?.count) || 0),
      max: toFiniteNumber(outliers?.max, 2),
      avg: toFiniteNumber(outliers?.avg, 2),
      longThreshold: toFiniteNumber(outliers?.longThreshold, 2),
      longCount: Math.max(0, Number(outliers?.longCount) || 0),
      longEdgeRatio: toFiniteNumber(outliers?.longEdgeRatio, 3),
      edgeCrossings: Math.max(0, Number(outliers?.edgeCrossings) || 0),
      crossingSampledEdges: Math.max(0, Number(outliers?.crossingSampledEdges) || 0),
      top: top
        ? {
            id: truncateTextMiddle(top?.id || "", 76),
            len: toFiniteNumber(top?.len, 2),
            sourceBand: String(top?.sourceBand || ""),
            targetBand: String(top?.targetBand || ""),
          }
        : null,
    };
  }

  function summarizeAlgoVersionForTrace(version) {
    const raw = String(version || "").trim();
    if (!raw) return "";
    const parts = raw.split("-").filter(Boolean);
    if (parts.length <= 6) return truncateTextMiddle(raw, 100);
    const tail = String(parts[parts.length - 1] || "").trim();
    if (!tail) return `${parts.slice(0, 2).join("-")} +${parts.length - 2} flags`;
    return `${parts.slice(0, 2).join("-")} +${parts.length - 2} flags [${truncateTextMiddle(tail, 20)}]`;
  }

  function resolveGraphTraceLifecycle(phase, payload = {}) {
    const p = String(phase || "").trim().toLowerCase();
    let stage = "event";
    if (!p) stage = "event";
    else if (p.includes("cancel")) stage = "cancelled";
    else if (p.includes("skip")) stage = "skipped";
    else if (p.endsWith("-start")) stage = "start";
    else if (p.includes("preview")) stage = "preview";
    else if (p.endsWith("-done") || p === "layout") stage = "done";
    else if (p.includes("-apply")) stage = "apply";
    else if (p === "response") stage = "response";
    else if (p.includes("render")) stage = "render";
    else if (p === "sample") stage = "sample";
    const preview = !!payload?.preview;
    const deferred = !!payload?.deferred;
    let final = stage === "apply" || !!payload?.final;
    if (stage === "start" || stage === "preview" || stage === "cancelled" || stage === "skipped") final = false;
    if (preview || deferred) final = false;
    return { stage, final, preview, deferred };
  }

  function summarizeGraphTracePayload(phase, payload = {}, lifecycle = null) {
    const out = {};
    const p = String(phase || "").trim().toLowerCase();
    const counts = sanitizeTraceCounts(payload?.counts);
    if (counts) out.counts = counts;
    const amount = toFiniteNumber(payload?.amount, 2);
    if (amount != null) out.amount = amount;
    if (payload?.view != null) out.view = String(payload.view);
    if (payload?.focusId) out.focusId = String(payload.focusId);
    if (payload?.reason) out.reason = String(payload.reason);
    if (payload?.sourceRequestId) out.sourceRequestId = String(payload.sourceRequestId);
    if (payload?.targetRequestId) out.targetRequestId = String(payload.targetRequestId);
    if (payload?.workerSeq != null) out.workerSeq = Math.max(0, Number(payload.workerSeq) || 0);
    if (payload?.nextSeq != null) out.nextSeq = Math.max(0, Number(payload.nextSeq) || 0);
    if (payload?.timeoutMs != null) out.timeoutMs = Math.max(0, Number(payload.timeoutMs) || 0);
    if (payload?.ms != null) out.ms = Math.max(0, Number(payload.ms) || 0);
    if (payload?.applied != null) out.applied = Math.max(0, Number(payload.applied) || 0);
    if ("animate" in payload) out.animate = !!payload?.animate;
    if ("fit" in payload) out.fit = !!payload?.fit;
    if (payload?.algoVersion) out.algoVersion = summarizeAlgoVersionForTrace(payload.algoVersion);

    if (p.includes("layout")) {
      if (payload?.before) out.before = summarizeLayoutBoundsForTrace(payload.before);
      if (payload?.layout) out.layout = summarizeLayoutBoundsForTrace(payload.layout);
      if (payload?.spacing) out.spacing = summarizeSpacingForTrace(payload.spacing);
      if (payload?.edgeOutliers) out.edgeOutliers = summarizeOutliersForTrace(payload.edgeOutliers);
    }
    if (lifecycle?.preview) out.preview = true;
    if (lifecycle?.deferred) out.deferred = true;
    return out;
  }

  function shouldGraphTraceDedup(map, key, windowMs = 1800) {
    if (!(map instanceof Map)) return true;
    const now = Date.now();
    const previous = Number(map.get(key) || 0);
    if (previous && now - previous < windowMs) return false;
    map.set(key, now);
    if (map.size > 320) {
      for (const [k, ts] of map.entries()) {
        if (now - Number(ts || 0) > windowMs * 10) map.delete(k);
        if (map.size <= 240) break;
      }
    }
    return true;
  }

  function isGhostProbeWindow(windowMs = 2600) {
    const at = Number(state.lastDeleteProbeAt || 0);
    if (!at) return false;
    return Date.now() - at <= Math.max(240, Number(windowMs) || 0);
  }

  function collectGhostProbeCounts(graph = null) {
    const g = graph || state.graph?.instance || null;
    const data = state.graph?.data || { nodes: [], edges: [] };
    const expectedNodes = Array.isArray(data.nodes) ? data.nodes.length : 0;
    const expectedEdges = Array.isArray(data.edges) ? data.edges.length : 0;
    const runtimeNodes = g?.getNodes?.().length || 0;
    const runtimeEdges = g?.getEdges?.().length || 0;
    return {
      expected: { nodes: expectedNodes, edges: expectedEdges },
      runtime: { nodes: runtimeNodes, edges: runtimeEdges },
      delta: { nodes: runtimeNodes - expectedNodes, edges: runtimeEdges - expectedEdges },
    };
  }

  function collectGhostProbeEngineInfo(graph = null) {
    const g = graph || state.graph?.instance || null;
    if (!g || typeof g.getDebugInfo !== "function") return null;
    try {
      const info = g.getDebugInfo() || {};
      return {
        renderer: String(info?.renderer || ""),
        counts: {
          nodes: Number(info?.counts?.nodes || 0),
          edges: Number(info?.counts?.edges || 0),
          edgeSegments: Number(info?.counts?.edgeSegments || 0),
          arrows: Number(info?.counts?.arrows || 0),
          edgeVisible: Number(info?.counts?.edgeVisible || 0),
        },
        cull: {
          enabled: !!info?.cull?.enabled,
          active: !!info?.cull?.active,
          viewDirty: !!info?.cull?.viewDirty,
          threshold: Number(info?.cull?.threshold || 0),
        },
        worker: {
          supported: !!info?.worker?.supported,
          busy: !!info?.worker?.busy,
          threshold: Number(info?.worker?.threshold || 0),
        },
        repair: {
          count: Number(info?.repair?.count || 0),
          reason: String(info?.repair?.reason || ""),
          at: Number(info?.repair?.at || 0),
        },
      };
    } catch (e) {
      return null;
    }
  }

  function isGraphRuntimeStable(graph = null) {
    const g = graph || state.graph?.instance || null;
    if (!g) return true;
    const counts = collectGhostProbeCounts(g);
    if ((counts?.delta?.nodes || 0) !== 0) return false;
    if ((counts?.delta?.edges || 0) !== 0) return false;
    const info = collectGhostProbeEngineInfo(g);
    if (!info) return true;
    if ((info?.counts?.nodes || 0) !== (counts?.expected?.nodes || 0)) return false;
    if ((info?.counts?.edges || 0) !== (counts?.expected?.edges || 0)) return false;
    if ((counts?.expected?.edges || 0) <= 0 && (info?.counts?.edgeSegments || 0) > 0) return false;
    return true;
  }

  function logGhostProbe(
    phase,
    payload = {},
    { level = "DEBUG", dedupKey = "", dedupMs = 260, onlyInDeleteWindow = false } = {}
  ) {
    void phase;
    void payload;
    void level;
    void dedupKey;
    void dedupMs;
    void onlyInDeleteWindow;
  }

  function collectGraphTraceIssues(phase, payload = {}, lifecycle = null) {
    void phase;
    void payload;
    void lifecycle;
    return [];
  }

  function emitGraphTraceIssues(phase, payload = {}, lifecycle = null) {
    const issues = collectGraphTraceIssues(phase, payload, lifecycle);
    if (!issues.length) return;
    const stage = String(lifecycle?.stage || "event");
    issues.forEach((item) => {
      // Unknown-counterparty warnings in preview/event phases are repetitive and low-action.
      if (item.code === "unknown-counterparty-node" && stage !== "done" && stage !== "apply") return;
      const dedupKey = `${state.requestId || ""}|${item.code}`;
      const dedupWindow = item.code === "unknown-counterparty-node" ? 120000 : 6000;
      if (!shouldGraphTraceDedup(state.graphTraceWarnDedup, dedupKey, dedupWindow)) return;
      log("WARN", "graph trace consistency", {
        phase,
        requestId: state.requestId || "",
        source: state.source || "",
        caseId: state.caseId || "",
        stage,
        code: item.code,
        issue: item.issue,
        detail: item.detail || null,
      });
    });
  }

  function maybeLogLayoutSummary(phase, summarized = {}, lifecycle = null) {
    void phase;
    void summarized;
    void lifecycle;
  }

  function logGraphTrace(phase, payload = {}, level = "INFO") {
    const safeLevel = normalizeLogLevel(level, "INFO");
    const normalizedPhase = String(phase || "").trim().toLowerCase();
    if (safeLevel !== "DEBUG") {
      const mutedInfoPhases = new Set([
        "response",
        "sample",
        "empty",
        "layout",
        "layout-preview",
        "layout-worker-start",
        "layout-worker-done",
        "layout-worker-apply",
        "layout-apply-start",
        "layout-apply-done",
        "layout-apply-skip",
        "render-start",
      ]);
      if (mutedInfoPhases.has(normalizedPhase)) return;
    }
    const sourcePayload = payload && typeof payload === "object" ? payload : { data: payload };
    const lifecycle = resolveGraphTraceLifecycle(phase, sourcePayload);
    const summaryPayload = summarizeGraphTracePayload(phase, sourcePayload, lifecycle);
    const base = {
      phase,
      requestId: state.requestId || "",
      source: state.source || "",
      caseId: state.caseId || "",
      stage: lifecycle.stage,
      final: !!lifecycle.final,
    };
    if (safeLevel === "DEBUG") {
      log("DEBUG", "graph trace", {
        ...base,
        detail: true,
        ...sourcePayload,
      });
      emitGraphTraceIssues(phase, sourcePayload, lifecycle);
      return;
    }
    log(safeLevel, "graph trace", {
      ...base,
      ...summaryPayload,
    });
    maybeLogLayoutSummary(phase, summaryPayload, lifecycle);
    emitGraphTraceIssues(phase, sourcePayload, lifecycle);
    if (shouldEmitLog("DEBUG", "graph trace", { phase })) {
      log("DEBUG", "graph trace", {
        ...base,
        detail: true,
        ...sourcePayload,
      });
    }
  }

  function logGraphRenderStatus(graph, dataCounts, phase) {
    if (!graph || typeof graph.getDebugInfo !== "function") return;
    try {
      const info = graph.getDebugInfo();
      const engineNodes = Number(info?.counts?.nodes || 0);
      const engineEdges = Number(info?.counts?.edges || 0);
      const dataNodes = Number(dataCounts?.nodes || 0);
      const dataEdges = Number(dataCounts?.edges || 0);
      const missingNodes = dataNodes > 0 && engineNodes === 0;
      const missingEdges = dataEdges > 0 && engineEdges === 0;
      if (missingNodes || missingEdges) {
        log("WARN", "graph render empty", {
          phase,
          requestId: state.requestId || "",
          dataCounts: { nodes: dataNodes, edges: dataEdges },
          engineCounts: info?.counts || {},
          buffers: info?.buffers || {},
          cull: info?.cull || {},
          renderer: info?.renderer || "",
          pixelRatio: info?.pixelRatio || 1,
          scale: info?.scale || 1,
          translate: info?.translate || { x: 0, y: 0 },
          programs: info?.programs || {},
          shaders: info?.shaders || {},
          sample: dataCounts?.sample || null,
        });
        return;
      }
      if (state.debugTrace) {
        log("INFO", "graph render status", {
          phase,
          requestId: state.requestId || "",
          dataCounts: { nodes: dataNodes, edges: dataEdges },
          engineCounts: info?.counts || {},
          buffers: info?.buffers || {},
          cull: info?.cull || {},
          renderer: info?.renderer || "",
          pixelRatio: info?.pixelRatio || 1,
          scale: info?.scale || 1,
          translate: info?.translate || { x: 0, y: 0 },
          programs: info?.programs || {},
          shaders: info?.shaders || {},
          sample: dataCounts?.sample || null,
        });
      }
    } catch (e) {}
  }

  function applyEdgeLabelLOD(show, { edges } = {}) {
    const graph = ensureGraph();
    if (!graph) return;
    const targets = edges || graph.getEdges?.() || [];
    if (!targets.length) return;
    const isEdgeFocused = (edge) => {
      if (!edge) return false;
      if (edge.hasState?.("selected") || edge.hasState?.("active") || edge.hasState?.("hover")) return true;
      const source = edge.getSource?.();
      const target = edge.getTarget?.();
      const nodeHot = (node) =>
        !!(node && (node.hasState?.("selected") || node.hasState?.("active") || node.hasState?.("hover")));
      return nodeHot(source) || nodeHot(target);
    };
    graph.setAutoPaint(false);
    try {
      targets.forEach((edge) => {
        const model = edge.getModel?.() || {};
        if (model.__baseLabel == null) {
          model.__baseLabel = model.label ?? "";
          model.__baseLabelTop = model.labelTop ?? "";
          model.__baseLabelBottom = model.labelBottom ?? "";
        }
        if (show || isEdgeFocused(edge)) {
          graph.updateItem(edge, {
            label: model.__baseLabel ?? "",
            labelTop: model.__baseLabelTop ?? "",
            labelBottom: model.__baseLabelBottom ?? "",
            detailLabel: false,
          });
        } else {
          graph.updateItem(edge, { label: "", labelTop: "", labelBottom: "", detailLabel: false });
        }
      });
    } finally {
      graph.setAutoPaint(true);
      graph.paint();
    }
    if (show && state.edgeDetail) {
      applyEdgeDetailLabels(true);
    }
  }

  function applyShadowLOD(reduce, { nodes, edges } = {}) {
    const graph = ensureGraph();
    if (!graph) return;
    const nextFontShadow = reduce ? false : asBool(state.graphStyle.fontShadow, false);
    const nextNodeShadow = reduce ? false : asBool(state.graphStyle.nodeShadow, false);
    const nodeItems = nodes || graph.getNodes?.() || [];
    const edgeItems = edges || graph.getEdges?.() || [];
    if (!nodeItems.length && !edgeItems.length) return;
    graph.setAutoPaint(false);
    try {
      nodeItems.forEach((node) => {
        graph.updateItem(node, { fontShadow: nextFontShadow, nodeShadow: nextNodeShadow });
      });
      edgeItems.forEach((edge) => {
        graph.updateItem(edge, { fontShadow: nextFontShadow });
      });
    } finally {
      graph.setAutoPaint(true);
      graph.paint();
    }
  }

  function setInteractionPerf(on, reason = "") {
    const graph = ensureGraph();
    if (!graph) return;
    if (state.interactionPerf === !!on) return;
    state.interactionPerf = !!on;
    if (state.interactionPerfTimer) {
      clearTimeout(state.interactionPerfTimer);
      state.interactionPerfTimer = 0;
    }
    if (on) {
      state.interactionPerfRestore = {
        edgeDetail: !!state.edgeDetail,
        lod: { ...state.lod },
      };
      try {
        applyEdgeLabelLOD(false);
        applyShadowLOD(true);
        if (state.edgeDetail) {
          state.edgeDetail = false;
          applyEdgeDetailLabels(false);
          syncEdgeDetailButton();
        }
        if (state.debugPerf) log("INFO", "interaction perf on", { reason });
      } catch (e) {}
      state.interactionPerfTimer = setTimeout(() => setInteractionPerf(false, "auto-timeout"), 2000);
      return;
    }
    const restore = state.interactionPerfRestore || {};
    state.interactionPerfRestore = null;
    if (restore.edgeDetail) {
      state.edgeDetail = true;
      applyEdgeDetailLabels(true);
      syncEdgeDetailButton();
    }
    updateLodByZoom(graph, { force: true });
    try {
      if (state.debugPerf) log("INFO", "interaction perf off", { reason });
    } catch (e) {}
  }

  function updateLodByZoom(graph, { force = false } = {}) {
    if (!graph) return;
    const zoom = graph.getZoom ? graph.getZoom() : 1;
    // User requirement: show all labels; disable edge-label zoom LOD hiding.
    const hideLabels = false;
    const reduceShadows = state.lod.shadowsReduced
      ? zoom < LOD_SHADOW_SHOW_ZOOM
      : zoom < LOD_SHADOW_HIDE_ZOOM;

    if (force || hideLabels !== state.lod.edgeLabelsHidden) {
      state.lod.edgeLabelsHidden = hideLabels;
      applyEdgeLabelLOD(!hideLabels);
      if (state.debugPerf) {
        try {
          log("INFO", "lod edge labels", {
            hidden: hideLabels,
            zoom,
            reason: "disabled_by_user",
          });
        } catch (e) {}
      }
    }
    if (force || reduceShadows !== state.lod.shadowsReduced) {
      state.lod.shadowsReduced = reduceShadows;
      applyShadowLOD(reduceShadows);
      if (state.debugPerf) {
        try {
          log("INFO", "lod shadows", { reduced: reduceShadows, zoom });
        } catch (e) {}
      }
    }
  }

  function scheduleLodUpdate(graph) {
    if (state.lodRaf) return;
    state.lodRaf = requestAnimationFrame(() => {
      state.lodRaf = 0;
      updateLodByZoom(graph);
    });
  }

  function buildGraphModes(perfMode) {
    const zoomMode = perfMode
      ? { type: "zoom-canvas", enableOptimize: true, optimizeZoom: PERF_OPTIMIZE_ZOOM }
      : "zoom-canvas";
    const dragCanvasMode = perfMode ? { type: "drag-canvas", enableOptimize: true } : "drag-canvas";
    const dragNodeMode = perfMode
      ? {
          type: "drag-node",
          enableDelegate: true,
          delegateStyle: {
            fill: "rgba(31,111,235,0.12)",
            stroke: "rgba(31,111,235,0.65)",
            lineWidth: 1.5,
          },
        }
      : "drag-node";
    return { default: [dragCanvasMode, zoomMode, dragNodeMode] };
  }

  function applyGraphModes(graph, perfMode) {
    if (!graph || typeof graph.setMode !== "function") return;
    try {
      graph.set("modes", buildGraphModes(perfMode));
      graph.setMode("default");
      if (state.debugPerf) log("INFO", "graph modes updated", { perf: !!perfMode, dragDelegate: !!perfMode });
    } catch (e) {}
  }

  function shouldBatchEdges(edgesLen, ctx = state) {
    const total = Math.max(0, Number(edgesLen) || 0);
    if (!total) return false;
    const hints = getActiveGraphRenderHints(ctx, ctx?.graph?.data?.nodes?.length || 0, total);
    const edgeMode = String(hints.edge_mode || "").trim().toLowerCase();
    const threshold = Math.max(0, Number(hints.edge_batch_threshold) || EDGE_BATCH_THRESHOLD);
    if (edgeMode === "full") return false;
    if (edgeMode === "skeleton") return true;
    return total >= threshold;
  }

  function cancelEdgeBatching() {
    state.edgeBatchSeq += 1;
    state.edgeBatching = false;
  }

  function resetLayoutWorkerInstance(reason = "", { reinit = true } = {}) {
    const worker = state.layoutWorker;
    if (worker) {
      try {
        worker.terminate();
      } catch (e) {}
    }
    state.layoutWorker = null;
    state.layoutWorkerStage = "";
    if (reinit) {
      try {
        initLayoutWorker();
      } catch (e) {
        try {
          log("WARN", "layout worker reinit failed", {
            reason: String(reason || ""),
            message: String(e?.message || e || ""),
          });
        } catch (e2) {}
      }
    }
    return true;
  }

  function cancelPendingLayoutWorker(reason = "", { resetWorker = true } = {}) {
    const pending = state.layoutWorkerPending;
    if (!pending) return false;
    if (pending.timer) {
      try {
        clearTimeout(pending.timer);
      } catch (e) {}
    }
    state.layoutWorkerPending = null;
    try {
      logGraphTrace("layout-worker-cancelled", {
        view: pending.preset || "compact",
        focusId: pending.focusId || "",
        counts: { nodes: pending.nodes?.length || 0, edges: pending.edges?.length || 0 },
        amount: computeGraphTotalAmount(pending.edges || []),
        reason: reason || "graph-mutation",
        workerSeq: pending.seq || 0,
        sourceRequestId: pending.token || "",
        targetRequestId: state.requestId || "",
      });
    } catch (e) {}
    if (resetWorker && state.layoutWorker) {
      resetLayoutWorkerInstance(reason || "layout-worker-cancel");
    } else {
      state.layoutWorkerStage = "idle";
    }
    return true;
  }

  function cancelGraphTransientTasks(reason = "") {
    const prevMutationSeq = state.graphMutationSeq || 0;
    state.graphMutationSeq = prevMutationSeq + 1;
    const mutationReason = reason || "graph-mutation";
    state.lastMutationReason = mutationReason;
    state.lastMutationAt = Date.now();
    if (mutationReason.includes("delete-selected")) {
      state.deleteProbeSeq = (state.deleteProbeSeq || 0) + 1;
      state.lastDeleteProbeId = `del-${state.deleteProbeSeq}`;
      state.lastDeleteProbeAt = state.lastMutationAt;
    }
    if (Array.isArray(state.deleteVisualGuardTimers) && state.deleteVisualGuardTimers.length) {
      state.deleteVisualGuardTimers.forEach((timer) => {
        try {
          clearTimeout(timer);
        } catch (e) {}
      });
      state.deleteVisualGuardTimers = [];
    }
    state.deleteVisualGuardSeq = (state.deleteVisualGuardSeq || 0) + 1;
    cancelEdgeBatching();
    cancelPendingLayoutWorker(mutationReason);
    terminateLayoutPrewarmTask(mutationReason);
    layoutPlanCacheController.clearCaches();
    networkSectorPlacementCache.clear();
    if (state.networkLayout?.viewportRefineTimer) {
      try {
        clearTimeout(state.networkLayout.viewportRefineTimer);
      } catch (e) {}
      state.networkLayout.viewportRefineTimer = 0;
    }
    if (state.networkLayout?.communityQualityTimer) {
      try {
        clearTimeout(state.networkLayout.communityQualityTimer);
      } catch (e) {}
      state.networkLayout.communityQualityTimer = 0;
    }
    if (state.networkLayout) {
      state.networkLayout.communityQualitySeq = Number(state.networkLayout.communityQualitySeq || 0) + 1;
    }
    state.directRenderSeq = (state.directRenderSeq || 0) + 1;
    if (state.interactionPerfTimer) {
      try {
        clearTimeout(state.interactionPerfTimer);
      } catch (e) {}
      state.interactionPerfTimer = 0;
    }
    if (state.projectionAutoMaterialize?.timer) {
      try {
        clearTimeout(state.projectionAutoMaterialize.timer);
      } catch (e) {}
      state.projectionAutoMaterialize.timer = 0;
    }
    if (state.projectionAutoMaterialize) {
      state.projectionAutoMaterialize.inflight = false;
      state.projectionAutoMaterialize.inflightSignature = "";
      if (mutationReason === "clear-graph" || mutationReason === "graph-mutation") {
        state.projectionAutoMaterialize.lastSignature = "";
        state.projectionAutoMaterialize.lastAt = 0;
      }
    }
    if (state.projectionLayoutSync?.timer) {
      try {
        clearTimeout(state.projectionLayoutSync.timer);
      } catch (e) {}
      state.projectionLayoutSync.timer = 0;
    }
    if (state.projectionLayoutSync) {
      state.projectionLayoutSync.inflight = false;
      state.projectionLayoutSync.inflightSignature = "";
      if (mutationReason === "clear-graph" || mutationReason === "graph-mutation") {
        state.projectionLayoutSync.lastSignature = "";
        state.projectionLayoutSync.lastSnapshotId = "";
        state.projectionLayoutSync.lastAt = 0;
      }
    }
    if (state.interactionPerf) {
      try {
        setInteractionPerf(false, reason || "graph-mutation");
      } catch (e) {
        state.interactionPerf = false;
        state.interactionPerfRestore = null;
      }
    } else {
      state.interactionPerfRestore = null;
    }
    state.introAnimSeq += 1;
    state.introAnimating = false;
    state.introSuppressUntil = 0;
    if (!isDeleteReason(mutationReason) || isDeleteProbeVerbose()) {
      logGhostProbe(
        "mutation-cancelled",
        {
          triggerReason: mutationReason,
          prevMutationSeq,
          nextMutationSeq: state.graphMutationSeq || 0,
        },
        {
          dedupKey: `mutation-cancelled|${state.graphMutationSeq || 0}|${mutationReason}`,
          dedupMs: 120,
        }
      );
    }
  }

  function renderEdgesInBatches(graph, edges, { onComplete } = {}) {
    if (!graph) return;
    const total = edges?.length || 0;
    if (!total) {
      if (typeof onComplete === "function") onComplete();
      return;
    }
    state.edgeBatchSeq += 1;
    const seq = state.edgeBatchSeq;
    state.edgeBatching = true;
    const batchSize = resolveEdgeBatchSize(total);
    let cursor = 0;
    const t0 = performance.now();
    logGhostProbe("edge-batch-start", { seq, edges: total, batchSize }, { dedupKey: `edge-batch-start|${seq}`, dedupMs: 120 });
    if (state.debugPerf) {
      try {
        log("INFO", "edge batch render start", { edges: total, batchSize });
      } catch (e) {}
    }
    const tick = () => {
      if (seq !== state.edgeBatchSeq) {
        state.edgeBatching = false;
        logGhostProbe(
          "edge-batch-cancelled",
          { seq, cursor, total, currentSeq: state.edgeBatchSeq || 0 },
          { dedupKey: `edge-batch-cancelled|${seq}|${cursor}`, dedupMs: 180 }
        );
        return;
      }
      const slice = edges.slice(cursor, cursor + batchSize);
      if (!slice.length) {
        state.edgeBatching = false;
        const ms = Math.round(performance.now() - t0);
        logGhostProbe("edge-batch-done", { seq, edges: total, ms }, { dedupKey: `edge-batch-done|${seq}`, dedupMs: 120 });
        if (state.debugPerf) {
          try {
            log("INFO", "edge batch render done", { edges: total, ms });
          } catch (e) {}
        }
        try {
          const graphEdges = graph.getEdges?.() || [];
          if (graphEdges.length >= EDGE_BATCH_FINAL_REFRESH_THRESHOLD) {
            scheduleGraphItemRefreshBatches(graph, graphEdges, {
              batchSize: Math.max(resolveEdgeRefreshBatchSize(graphEdges.length), batchSize * 2),
              timeout: GRAPH_ITEM_REFRESH_BATCH_TIMEOUT_MS,
              onComplete: () => {
                try {
                  if (graph.refreshPositions) graph.refreshPositions();
                } catch (e) {}
                graph.paint?.();
              },
            });
          } else {
            graphEdges.forEach((edge) => graph.refreshItem?.(edge));
            if (graph.refreshPositions) graph.refreshPositions();
          }
          syncGraphItemPositionsFromData(
            graph,
            state.graph.data?.nodes || [],
            "edge-batch-final",
            { force: true, log }
          );
        } catch (e) {}
        try {
          const layoutNow = summarizeGraphItemLayout(graph);
          if (isLayoutCollapsed(layoutNow)) {
            log("WARN", "edge batch done but layout collapsed, rerender full", {
              layout: layoutNow,
              nodes: state.graph.data?.nodes?.length || 0,
              edges: state.graph.data?.edges?.length || 0,
            });
            graph.changeData(state.graph.data);
            graph.render();
            graph.getEdges?.().forEach((edge) => graph.refreshItem?.(edge));
            if (graph.refreshPositions) graph.refreshPositions();
            syncGraphItemPositionsFromData(
              graph,
              state.graph.data?.nodes || [],
              "edge-batch-rerender",
              { force: true, log }
            );
          }
        } catch (e) {}
        if (state.pendingFit) {
          state.pendingFit = false;
          scheduleFit();
        }
        if (typeof onComplete === "function") onComplete();
        return;
      }
      graph.setAutoPaint(false);
      const added = [];
      let interrupted = false;
      try {
        for (const edge of slice) {
          if (seq !== state.edgeBatchSeq) {
            interrupted = true;
            break;
          }
          const item = graph.addItem("edge", edge);
          if (item) {
            added.push(item);
          }
        }
        // Avoid refreshPositions per batch to reduce layout jitter on large graphs.
      } catch (e) {}
      graph.setAutoPaint(true);
      graph.paint();
      if (seq !== state.edgeBatchSeq || interrupted) {
        state.edgeBatching = false;
        logGhostProbe(
          "edge-batch-interrupted",
          { seq, currentSeq: state.edgeBatchSeq || 0, cursor, total, interrupted: !!interrupted },
          { level: "WARN", dedupKey: `edge-batch-interrupted|${seq}|${cursor}`, dedupMs: 160 }
        );
        forceRedrawGraphFromState("edge-batch-interrupted", { restoreViewport: true });
        return;
      }
      if (state.lod.edgeLabelsHidden) applyEdgeLabelLOD(false, { edges: added });
      if (state.lod.shadowsReduced) applyShadowLOD(true, { edges: added });
      cursor += slice.length;
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  }

  function initMinimap(graph) {
    ensureMinimapStore().init(graph);
  }

  function destroyMinimap() {
    ensureMinimapStore().destroy();
  }

  function updatePerfMode(nodesLen, edgesLen) {
    ensureMinimapStore().updatePerfMode(nodesLen, edgesLen);
  }

  function updateMinimap(options = {}) {
    ensureMinimapStore().update(options);
  }

  function getActiveGraphRenderHints(ctx = state, nodeCount = 0, edgeCount = 0) {
    const nodeValue = Number(nodeCount);
    const edgeValue = Number(edgeCount);
    const totalNodes = Math.max(
      0,
      Number.isFinite(nodeValue) ? nodeValue : ctx?.graph?.data?.nodes?.length || 0
    );
    const totalEdges = Math.max(
      0,
      Number.isFinite(edgeValue) ? edgeValue : ctx?.graph?.data?.edges?.length || 0
    );
    const tier = normalizeGraphTier(ctx?.graphTier || ctx?.renderHints?.tier, totalNodes);
    return normalizeGraphRenderHints(
      ctx?.renderHints && typeof ctx.renderHints === "object" ? { ...ctx.renderHints, tier } : { tier },
      totalNodes,
      totalEdges,
      ctx?.graphMode || ctx?.view || "relation"
    );
  }

  function buildGraphRenderPlanContext(ctx = state, graphTier = "", renderHints = null) {
    const selected =
      ctx?.selected instanceof Set
        ? Array.from(ctx.selected)
        : Array.isArray(ctx?.selected)
        ? ctx.selected
        : [];
    const projection = ctx?.graphProjection && typeof ctx.graphProjection === "object"
      ? { mode: String(ctx.graphProjection.mode || "").trim() }
      : null;
    const graphMode = String(
      ctx?.graphMode ||
        ctx?.view ||
        ctx?.viewMode ||
        renderHints?.view_mode ||
        renderHints?.viewMode ||
        ""
    ).trim();
    return {
      graphTier: graphTier || ctx?.graphTier || "",
      graphMode,
      view: graphMode,
      renderHints: renderHints && typeof renderHints === "object" ? { ...renderHints } : null,
      graphProjection: projection,
      focusId: String(ctx?.focusId || ""),
      focusName: String(ctx?.focusName || ""),
      focusLabel: String(ctx?.focusLabel || ""),
      focusIds: parseUniqueList(ctx?.focusIds || [], { allowString: true }),
      leftSeeds: parseUniqueList(ctx?.leftSeeds || [], { allowString: true }),
      selected: parseUniqueList(selected, { allowString: true }),
    };
  }

  function applyGraphStagedLabelPlanUpdates(nodes = [], edges = [], plan = null) {
    if (!plan || typeof plan !== "object") {
      return { nodeUpdates: 0, edgeUpdates: 0 };
    }
    const rows = Array.isArray(nodes) ? nodes : [];
    const links = Array.isArray(edges) ? edges : [];
    const nodeIndex = new Map();
    const edgeIndex = new Map();
    rows.forEach((node, index) => {
      const id = String(node?.id || "").trim();
      if (id) nodeIndex.set(id, index);
    });
    links.forEach((edge, index) => {
      const id = String(edge?.id || "").trim();
      if (id) edgeIndex.set(id, index);
    });
    const resolveTarget = (items, indexMap, update) => {
      if (!Array.isArray(items) || !update || typeof update !== "object") return null;
      let targetIndex = Number.isInteger(update.index) ? update.index : -1;
      if (targetIndex < 0 || targetIndex >= items.length) {
        const id = String(update.id || "").trim();
        targetIndex = id ? Number(indexMap.get(id)) : -1;
      }
      return targetIndex >= 0 && targetIndex < items.length ? items[targetIndex] : null;
    };
    let nodeUpdates = 0;
    let edgeUpdates = 0;
    (Array.isArray(plan.nodeLabelUpdates) ? plan.nodeLabelUpdates : []).forEach((update) => {
      const target = resolveTarget(rows, nodeIndex, update);
      if (!target || typeof target !== "object") return;
      ["nodeLabelHidden", "displayId", "display_id"].forEach((key) => {
        if (Object.prototype.hasOwnProperty.call(update, key)) target[key] = update[key];
      });
      nodeUpdates += 1;
    });
    (Array.isArray(plan.edgeLabelUpdates) ? plan.edgeLabelUpdates : []).forEach((update) => {
      const target = resolveTarget(links, edgeIndex, update);
      if (!target || typeof target !== "object") return;
      ["label", "labelTop", "labelBottom", "detailLabel"].forEach((key) => {
        if (Object.prototype.hasOwnProperty.call(update, key)) target[key] = update[key];
      });
      edgeUpdates += 1;
    });
    return { nodeUpdates, edgeUpdates };
  }

  async function applyGraphTierRenderPlanWithRust(
    nodes = [],
    edges = [],
    ctx = state,
    { preserveEdgeLabels = false, name = "", includeStagedLabels = false } = {}
  ) {
    const rows = Array.isArray(nodes) ? nodes : [];
    const links = Array.isArray(edges) ? edges : [];
    const rawHints =
      ctx?.renderHints && typeof ctx.renderHints === "object" && !Array.isArray(ctx.renderHints)
        ? { ...ctx.renderHints }
        : {};
    const graphTier = normalizeGraphTier(
      ctx?.graphTier || rawHints.tier || rawHints.graph_tier || rawHints.graphTier,
      rows.length
    );
    const hintInput = {
      ...rawHints,
      tier: graphTier,
      view_mode:
        String(rawHints.view_mode || rawHints.viewMode || ctx?.graphMode || ctx?.view || "relation")
          .trim()
          .toLowerCase() || "relation",
    };
    const renderCtx = buildGraphRenderPlanContext(ctx, graphTier, hintInput);
    const result = await projectGraphRenderPlan({
      renderPlan: {
        name: String(name || "").trim(),
        nodes: rows,
        edges: links,
        ctx: renderCtx,
        hints: hintInput,
        graphStyle: state.graphStyle,
        preserveEdgeLabels,
        stagedFocusIds: collectStagedLabelFocusIds(ctx),
        ...(includeStagedLabels ? { stagedLabel: { enabled: true } } : {}),
      },
    });
    const renderResult = Array.isArray(result?.renderResults) ? result.renderResults[0] : null;
    const renderPlan = renderResult?.renderPlan;
    if (!renderPlan || typeof renderPlan !== "object") {
      throw new Error("graph-render-plan-rust-result-missing");
    }
    applyGraphRenderPlanUpdates(rows, links, renderPlan);
    applyEdgeOffsetUpdates(links, renderResult?.edgeOffsetUpdates || []);
    const stagedLabelPlan = renderResult?.stagedLabelPlan || null;
    if (stagedLabelPlan) {
      applyGraphStagedLabelPlanUpdates(rows, links, stagedLabelPlan);
    }
    const normalizedRenderHints =
      renderResult?.normalizedRenderHints && typeof renderResult.normalizedRenderHints === "object"
        ? renderResult.normalizedRenderHints
        : null;
    if (!normalizedRenderHints) {
      throw new Error("graph-render-hints-rust-result-missing");
    }
    const summary = renderResult?.summary || renderPlan.summary || {};
    return {
      tier: normalizedRenderHints.tier || summary.tier || graphTier,
      renderHints: normalizedRenderHints,
      entityCount: Math.max(0, Number(summary.entityCount) || 0),
      dotCount: Math.max(0, Number(summary.dotCount) || 0),
      focusCount: Math.max(0, Number(summary.focusCount) || 0),
      renderPlan,
      stagedLabelPlan,
    };
  }

  async function reapplyGraphTierVisualState(graph = ensureGraph(), { preserveEdgeLabels = true } = {}) {
    const nodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
    const edges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    const graphDataRef = state.graph?.data || null;
    const mutationSeq = Number(state.graphMutationSeq || 0);
    let summary = null;
    try {
      summary = await applyGraphTierRenderPlanWithRust(
        nodes,
        edges,
        state,
        { preserveEdgeLabels, name: "reapply-graph-tier-visual-state" }
      );
    } catch (e) {
      log("WARN", "rust graph visual state render plan failed", { message: String(e?.message || e), stack: e?.stack || "" });
      toast("图谱展示计划失败", "danger");
      return null;
    }
    if ((Number(state.graphMutationSeq || 0) !== mutationSeq) || state.graph?.data !== graphDataRef) {
      return null;
    }
    state.graphTier = summary.tier;
    state.renderHints = summary.renderHints;
    if (!graph) return summary;
    graph.setAutoPaint(false);
    try {
      syncGraphRenderItems(graph, nodes, edges, summary.renderPlan);
    } finally {
      graph.setAutoPaint(true);
      graph.paint?.();
    }
    return summary;
  }

  // ===== Left collapse =====
  function applyLeftCollapsed(collapsed) {
    state.leftCollapsed = !!collapsed;
    const app = $(".app");
    if (app) app.classList.toggle("left-collapsed", state.leftCollapsed);
    const gap = $("#gapToggle");
    if (gap) {
      gap.setAttribute("aria-expanded", state.leftCollapsed ? "false" : "true");
      gap.setAttribute("title", state.leftCollapsed ? "展开" : "折叠");
      gap.setAttribute("aria-label", state.leftCollapsed ? "展开左侧" : "折叠左侧");
    }
    scheduleShellStateSync("left-collapsed");
  }

  // ===== Case badge =====
  function setCaseBadge(name, caseId) {
    const safeName = (name || "").trim();
    const safeId = (caseId || "").trim();
    const label = safeName ? `案件：${safeName}` : safeId ? `案件：${safeId}` : "未选择案件";
    const text = $("#caseBadgeText");
    if (text) text.textContent = label;
    const badge = $("#caseBadge");
    if (badge) badge.setAttribute("title", label);
    if (text) text.setAttribute("title", label);
    state.caseName = safeName;
    scheduleShellStateSync("case-badge");
  }

  function fetchCaseName() {
    return ensureSessionStore().fetchCaseName();
  }

  async function updateCaseBadge() {
    return ensureSessionStore().updateCaseBadge();
  }

  // ===== Backend =====
  function initBackend() {
    if (state.backend || state.backendInit) return true;
    const backend = getRuntimeBackend();
    if (!backend) return false;

    state.backendInit = true;
    log("INFO", "initBackend start");
    state.backend = backend;
    backendRef = state.backend || null;
    if (!state.backend) {
      log("WARN", "initBackend missing backend");
      return false;
    }
    try {
      if (typeof state.backend.getFlowLogConfig === "function") {
        state.backend.getFlowLogConfig((raw) => {
          const res = parseJSONSafe(raw, {});
          if (!res || !res.ok || !res.logConfig) return;
          applyLogSwitch(res.logConfig, { persist: false });
          if (state.graph?.inited && state.graph?.instance) {
            state.graph.instance.set?.("webglWarn", !!logSwitch.webglWarn);
          }
        });
      }
    } catch (e) {}

    try {
      state.backend.caseChanged?.connect?.((cid) => {
        const nextCaseId = String(cid || "").trim();
        if (nextCaseId === state.caseId) {
          if (state.debugTrace) {
            log("DEBUG", `caseChanged duplicate -> ${nextCaseId || "<empty>"}`);
          }
        } else {
          if (state.debugTrace) log("DEBUG", `caseChanged -> ${nextCaseId || "<empty>"}`);
        }
        setCaseId(nextCaseId);
      });
    } catch (e) {}

    syncCaseFromBackend("init");
    syncFundsStatus();
    if (state.lastRequest) {
      const req = state.lastRequest;
      state.lastRequest = null;
      setTimeout(() => applyGraphRequest(req), 0);
    }
    return true;
  }

  function ensureBackendReady(retries = 12) {
    if (state.backend) return;
    const ok = initBackend();
    if (ok || retries <= 0) return;
    setTimeout(() => ensureBackendReady(retries - 1), 120);
  }

  function syncCaseFromBackend(reason = "") {
    ensureSessionStore().syncCaseFromBackend(reason);
  }

  async function setCaseId(caseId) {
    await ensureSessionStore().setCaseId(caseId);
  }

  function parseUniqueList(raw, { allowString = false } = {}) {
    if (raw == null) return [];
    if (typeof raw === "string") {
      if (!allowString) return [];
      raw = [raw];
    }
    if (!Array.isArray(raw)) return [];
    const out = [];
    const seen = new Set();
    for (const item of raw) {
      const val = String(item || "").trim();
      if (!val || seen.has(val)) continue;
      seen.add(val);
      out.push(val);
    }
    return out;
  }

  function resolveGraphModeLayout(payload) {
    const raw = String(
      payload.view ||
        payload.mode ||
        payload.graphMode ||
        payload.scene ||
        payload.layout ||
        payload.type ||
        ""
    )
      .trim()
      .toLowerCase();
    const hint = `${raw} ${payload.viewLabel || ""} ${payload.title || ""} ${payload.label || ""}`.toLowerCase();
    const isNet = /net|flow|流向|净值/.test(hint);
    const isRelation = /relation|关联/.test(hint);
    if (isNet) return { graphMode: "net", layoutPreset: "flow" };
    if (isRelation) return { graphMode: "relation", layoutPreset: "compact" };
    if (raw === "net") return { graphMode: "net", layoutPreset: "flow" };
    if (raw === "flow") return { graphMode: "net", layoutPreset: "flow" };
    if (raw === "relation") return { graphMode: "relation", layoutPreset: "compact" };
    if (raw === "compact") return { graphMode: "relation", layoutPreset: "compact" };
    if (raw === "network") return { graphMode: "relation", layoutPreset: "network" };
    if (raw === "hierarchy") return { graphMode: "net", layoutPreset: "hierarchy" };
    return { graphMode: state.graphMode || "relation", layoutPreset: state.layoutPreset || "compact" };
  }

  async function applyGraphRequest(payload) {
    let data = typeof payload === "string" ? parseJSONSafe(payload, {}) : payload || {};
    if (typeof data === "string") data = parseJSONSafe(data, {});
    setEdgeDirectionLocked(true);
    const directGraph = isDirectGraphPayload(data);
    const snapshotRef = getRequestSnapshotRef(data);
    if (!state.backend) {
      state.lastRequest = data;
      ensureBackendReady();
      if (!directGraph) return;
    }
    const caseId = String(data.caseId || data.case_id || "").trim();
    if (caseId && caseId !== state.caseId) {
      await setCaseId(caseId);
    }

    let seeds = parseUniqueList(data.seeds || data.selectedSeeds || data.selected_seeds || []);
    if (!seeds.length) {
      const fallback = String(data.focusId || data.focus_id || "").trim();
      if (fallback) seeds = [fallback];
    }
    if (seeds.length) {
      state.selected = new Set(seeds);
      updateSelectionSummary();
    }
    const requestedTab = String(data.tab || "").trim();
    if ((requestedTab === "byName" || requestedTab === "byCard") && requestedTab !== state.tab) {
      await setTab(requestedTab, { resetSelection: false });
    }

    const resolved = resolveGraphModeLayout(data);
    const normalizedSource = normalizeFlowSource(data.source || "");
    const fromStats = normalizedSource === "stats";
    state.graphMode = resolved.graphMode;
    state.layoutPreset = resolved.layoutPreset;
    state.signedEdgeDetail = false;
    if (directGraph) {
      state.graphMode = resolved.graphMode || state.graphMode || "relation";
      state.layoutPreset = String(data.layoutPreset || data.layout || resolved.layoutPreset || "flow");
    }
    setLayoutPreset(state.layoutPreset, { animate: false, markSelected: fromStats || directGraph });
    const focusIds = parseUniqueList(data.focusIds || data.focus_ids || [], { allowString: true });
    const focusNames = parseUniqueList(data.focusNames || data.focus_names || [], { allowString: true });
    const focusPlaceholderKinds = parseUniqueList(
      data.focusPlaceholderKinds || data.focus_placeholder_kinds || [],
      { allowString: true }
    ).map((kind) => normalizePlaceholderKind(kind)).filter(Boolean);
    state.focusId = String(data.focusId || data.focus_id || "").trim();
    state.focusName = String(data.focusName || data.focus_name || "").trim();
    state.focusIds = parseUniqueList([state.focusId, ...focusIds]);
    state.focusNames = parseUniqueList([state.focusName, ...focusNames]);
    state.focusPlaceholderKinds = parseUniqueList(focusPlaceholderKinds, { allowString: true });
    if (!state.focusId && state.focusIds.length) state.focusId = state.focusIds[0];
    if (!state.focusName && state.focusNames.length) state.focusName = state.focusNames[0];
    state.focusOnly = !!(data.focusOnly || data.focus_only);
    state.focusLabel = String(data.focusLabel || data.focus_label || "").trim();
    state.focusKeyType = String(data.focusKeyType || data.focus_key_type || data.keyType || "").trim().toLowerCase();
    state.focusUnknownName = !!(
      data.focusUnknownName || data.focus_unknown_name || data.focusUnknown
    );
    state.focusCounterpartyStrict = !!(
      data.focusCounterpartyStrict ||
      data.focus_counterparty_strict
    );
    state.includeMissingCounterparty = !!(
      data.includeMissingCounterparty ||
        data.include_missing_counterparty ||
        data.includeUnknownAccount ||
        data.include_unknown_account
    );
    state.source = normalizedSource;
    state.requestId = String(data.requestId || data.request_id || "").trim();
    {
      const expectedAmountRaw = Number(data.expectedTotalAmount ?? data.expected_total_amount);
      state.expectedTotalAmount = Number.isFinite(expectedAmountRaw) ? toFiniteNumber(expectedAmountRaw, 2) : null;
      const expectedRowRaw = Number(data.expectedRowCount ?? data.expected_row_count);
      state.expectedRowCount = Number.isFinite(expectedRowRaw) && expectedRowRaw > 0 ? Math.round(expectedRowRaw) : 0;
    }
    const focusSelfOnly = !!(data.focusSelfOnly ?? data.focus_self_only ?? data.onlySelf);
    state.focusSelfOnly = focusSelfOnly;
    state.focusSelfOnlyOnce = focusSelfOnly;
    state.lastRequest = { ...data, source: normalizedSource };
    if (fromStats && shouldLogHandoffProbe()) {
      log("INFO", "stats->viz probe payload", {
        requestId: state.requestId || "",
        view: state.graphMode || "",
        layoutPreset: state.layoutPreset || "",
        dateStart: String(data.dateStart || data.date_start || "").trim(),
        dateEnd: String(data.dateEnd || data.date_end || "").trim(),
        seeds: {
          count: seeds.length,
          items: seeds.slice(0, 16),
          truncated: seeds.length > 16,
        },
        focus: {
          keyType: state.focusKeyType || "",
          id: state.focusId || "",
          name: state.focusName || "",
          label: state.focusLabel || "",
          ids: Array.isArray(state.focusIds) ? state.focusIds.slice(0, 16) : [],
          idsCount: Array.isArray(state.focusIds) ? state.focusIds.length : 0,
          names: Array.isArray(state.focusNames) ? state.focusNames.slice(0, 16) : [],
          namesCount: Array.isArray(state.focusNames) ? state.focusNames.length : 0,
          placeholderKinds: Array.isArray(state.focusPlaceholderKinds) ? state.focusPlaceholderKinds.slice() : [],
          unknownName: !!state.focusUnknownName,
          counterpartyStrict: !!state.focusCounterpartyStrict,
          includeMissingCounterparty: !!state.includeMissingCounterparty,
          only: !!state.focusOnly,
        },
        expected: {
          totalAmount: state.expectedTotalAmount,
          rowCount: state.expectedRowCount,
        },
      });
    }
    void fromStats;

    if (snapshotRef) {
      const inlineGraph = getDirectGraphPayload(data);
      const inlineNodeCount = Array.isArray(inlineGraph.nodes) ? inlineGraph.nodes.length : 0;
      const inlineEdgeCount = Array.isArray(inlineGraph.edges) ? inlineGraph.edges.length : 0;
      const snapshotNodeCount = Number(snapshotRef?.node_count || 0);
      const snapshotEdgeCount = Number(snapshotRef?.edge_count || 0);
      const shouldPreferSnapshot =
        !directGraph ||
        snapshotNodeCount > inlineNodeCount ||
        snapshotEdgeCount > inlineEdgeCount;
      if (shouldPreferSnapshot) {
        const appliedFromSnapshot = await applySnapshotGraphPayload(data, snapshotRef);
        if (appliedFromSnapshot) {
          const summary = buildGraphStatsSummary(state.graph?.data || null, state.layoutPreset || resolved.layoutPreset || "compact");
          if ((summary.nodes > 0 || summary.edges > 0) || !directGraph || (!inlineNodeCount && !inlineEdgeCount)) {
            return;
          }
          try {
            log("WARN", "snapshot render produced empty graph, fallback to direct payload", {
              snapshotId: String(snapshotRef?.snapshot_id || snapshotRef?.graph_hash || "").trim(),
              summary,
              inline: { nodes: inlineNodeCount, edges: inlineEdgeCount },
              requestId: state.requestId || "",
            });
          } catch (e) {}
        }
      }
    }

    if (directGraph) {
      await applyDirectGraphPayload(data);
      return;
    }
    await buildGraph({ createView: true });
  }

  function normalizeKey(value) {
    const raw = String(value || "").trim();
    if (!raw) return "";
    let out = raw.replace(/\s+/g, "");
    const dash = out.indexOf("-");
    const under = out.indexOf("_");
    if (dash > 0 || under > 0) {
      const idx = dash > 0 && under > 0 ? Math.min(dash, under) : dash > 0 ? dash : under;
      out = out.slice(0, idx);
    }
    return out;
  }

  function normalizeFlowSource(value) {
    const source = String(value || "").trim().toLowerCase();
    if (source === "stats-react") return "stats";
    return source;
  }

  function isStatsSourceValue(value) {
    return normalizeFlowSource(value) === "stats";
  }

  function normalizeRequestedProjectionRenderMode(value) {
    const mode = String(value || "").trim().toLowerCase();
    if (mode === "full" || mode === "skeleton" || mode === "auto") return mode;
    return "auto";
  }

  function resolveRequestedProjectionRenderMode(ctx = state) {
    const lastGraph = ctx?.lastRequest?.graph;
    const lastDrill = ctx?.lastRequest?.drill;
    return normalizeRequestedProjectionRenderMode(
      lastGraph?.render_mode ||
        lastGraph?.renderMode ||
        lastDrill?.render_mode ||
        lastDrill?.renderMode ||
        "auto"
    );
  }

  function isSkeletonProjectionMode(projection = state.graphProjection) {
    return isSkeletonProjectionModeModel(projection);
  }

  function canExpandCurrentProjection() {
    const snapshotRef = cloneResultSnapshotRef(state.resultSnapshotRef);
    const snapshotId = String(snapshotRef?.snapshot_id || snapshotRef?.graph_hash || "").trim();
    return !!state.caseId && !!snapshotId && isSkeletonProjectionMode();
  }

  function canSyncCurrentProjectionLayout() {
    return (
      canExpandCurrentProjection() &&
      !!state.backend &&
      typeof state.backend.syncFlowResultSnapshotProjectionLayout === "function"
    );
  }

  function hasProjectionLayoutSyncForSnapshot(snapshotId) {
    const targetSnapshotId = String(snapshotId || "").trim();
    return (
      !!targetSnapshotId &&
      targetSnapshotId === String(state.projectionLayoutSync?.lastSnapshotId || "").trim() &&
      Number(state.projectionLayoutSync?.lastAt || 0) > 0
    );
  }

  function collectProjectionLayoutSyncNodes(targetGraph = ensureGraph()) {
    const graph = targetGraph || ensureGraph();
    const items = Array.isArray(graph?.getNodes?.()) ? graph.getNodes() : [];
    return collectProjectionLayoutSyncModels(items.map((item) => item?.getModel?.() || {}));
  }

  function scheduleProjectionLayoutIndexSync(reason = "", options = {}) {
    if (state.projectionLayoutSync?.timer) {
      try {
        clearTimeout(state.projectionLayoutSync.timer);
      } catch (e) {}
      state.projectionLayoutSync.timer = 0;
    }
    if (!canSyncCurrentProjectionLayout()) return false;
    const delay = Math.max(0, Number(options?.delayMs ?? 160) || 0);
    const run = async () => {
      state.projectionLayoutSync.timer = 0;
      if (!canSyncCurrentProjectionLayout()) return;
      if ((state.introAnimating || state.graphLoading || state.edgeBatching) && !options?.force) {
        scheduleProjectionLayoutIndexSync(reason || "projection-layout-sync-retry", { ...options, delayMs: 180 });
        return;
      }
      const graph = ensureGraph();
      const snapshotRef = cloneResultSnapshotRef(state.resultSnapshotRef);
      const snapshotId = String(snapshotRef?.snapshot_id || snapshotRef?.graph_hash || "").trim();
      if (!graph || !snapshotId) return;
      const nodes = collectProjectionLayoutSyncNodes(graph);
      const viewport = captureGraphViewport(graph) || {};
      const graphSize = {
        width: Number(graph.get?.("width")) || Number(viewport?.size?.width) || 0,
        height: Number(graph.get?.("height")) || Number(viewport?.size?.height) || 0,
      };
      if (!nodes.length && !viewport) return;
      if (
        state.projectionLayoutSync?.inflight &&
        state.projectionLayoutSync?.inflightSignature === snapshotId
      ) {
        scheduleProjectionLayoutIndexSync(reason || "projection-layout-sync-inflight", { ...options, delayMs: 180 });
        return;
      }
      state.projectionLayoutSync.inflight = true;
      state.projectionLayoutSync.inflightSignature = snapshotId;
      try {
        const raw = await backendCall(
          "syncFlowResultSnapshotProjectionLayout",
          JSON.stringify({
            caseId: state.caseId || "",
            snapshotId,
            viewport,
            graphSize,
            nodes,
          })
        );
        const res = parseJSONSafe(raw, {});
        if (res?.ok) {
          const signature = String(res?.layout_sync_signature || res?.layout_index_summary?.signature || "").trim();
          state.projectionLayoutSync.lastSignature = signature;
          state.projectionLayoutSync.lastSnapshotId = snapshotId;
          state.projectionLayoutSync.lastAt = Date.now();
          if (state.graphProjection && res.layout_index_summary && typeof res.layout_index_summary === "object") {
            state.graphProjection = {
              ...(state.graphProjection || {}),
              layout_index_summary: { ...(res.layout_index_summary || {}) },
            };
          }
          if (state.debugPerf || state.debugTrace) {
            log("INFO", "projection layout index synced", {
              reason: String(reason || ""),
              snapshotId,
              nodeCount: nodes.length,
              updatedNodeCount: Number(res?.updated_node_count) || 0,
            });
          }
        }
      } finally {
        state.projectionLayoutSync.inflight = false;
        state.projectionLayoutSync.inflightSignature = "";
      }
    };
    if (!delay) {
      void run();
      return true;
    }
    state.projectionLayoutSync.timer = setTimeout(() => {
      void run();
    }, delay);
    return true;
  }

  async function collectVisibleProjectionExpandTargets(targetGraph = ensureGraph(), options = {}) {
    const graph = targetGraph || ensureGraph();
    const container = $("#graphContainer");
    const width = Math.max(0, container?.clientWidth || graph?.get?.("width") || 0);
    const height = Math.max(0, container?.clientHeight || graph?.get?.("height") || 0);
    if (!graph || !width || !height) {
      return { clusterIds: [], tileIds: [], nodeIds: [] };
    }
    const entries = [];
    const items = Array.isArray(graph.getNodes?.()) ? graph.getNodes() : [];
    items.forEach((item) => {
      const model = item?.getModel?.() || {};
      if (!isCollapsedProjectionNode(model)) return;
      const worldX = Number(model?.x);
      const worldY = Number(model?.y);
      if (!Number.isFinite(worldX) || !Number.isFinite(worldY)) return;
      const canvasPoint = graph.getCanvasByPoint?.(worldX, worldY) || { x: worldX, y: worldY };
      const x = Number(canvasPoint?.x);
      const y = Number(canvasPoint?.y);
      if (!Number.isFinite(x) || !Number.isFinite(y)) return;
      const id = String(model?.id || "").trim();
      if (!id) return;
      entries.push({
        ...model,
        id,
        canvasX: x,
        canvasY: y,
      });
    });
    const result = await projectGraphRenderPlan({
      renderPlans: [],
      viewportExpandTargets: {
        ...options,
        width,
        height,
        entries,
        projection: state.graphProjection,
      },
    });
    const targets = result?.viewportExpandTargets;
    if (!targets || typeof targets !== "object") {
      throw new Error("projection-viewport-targets-rust-result-missing");
    }
    return {
      clusterIds: parseUniqueList(targets.clusterIds, { allowString: true }),
      tileIds: parseUniqueList(targets.tileIds, { allowString: true }),
      nodeIds: parseUniqueList(targets.nodeIds, { allowString: true }),
    };
  }

  function isProjectionPointLayerActive(projection = state.graphProjection, hints = state.renderHints) {
    const hintMode = String(hints?.point_layer_mode || "").trim().toLowerCase();
    if (hintMode === "clustered") return true;
    return !!getProjectionPointLayer(projection);
  }

  function shouldAutoMaterializeProjectionViewport(hints = state.renderHints, projection = state.graphProjection) {
    if (!canExpandCurrentProjection()) return false;
    if (!isSkeletonProjectionMode(projection)) return false;
    if (!isProjectionPointLayerActive(projection, hints)) return false;
    const normalizedHints = normalizeGraphRenderHints(
      hints || { tier: state.graphTier || "xlarge", point_layer_mode: "clustered" },
      state.graph?.data?.nodes?.length || 0,
      state.graph?.data?.edges?.length || 0,
      state.graphMode || "relation"
    );
    if (normalizedHints.projection_auto_expand === false) return false;
    return normalizeGraphTier(normalizedHints.tier || state.graphTier, state.graph?.data?.nodes?.length || 0) === "xlarge";
  }

  function scheduleProjectionAutoMaterialize(reason = "", options = {}) {
    if (state.projectionAutoMaterialize?.timer) {
      try {
        clearTimeout(state.projectionAutoMaterialize.timer);
      } catch (e) {}
      state.projectionAutoMaterialize.timer = 0;
    }
    if (!shouldAutoMaterializeProjectionViewport()) return false;
    if (state.graphLoading || state.edgeBatching || state.introAnimating) return false;
    const hints = normalizeGraphRenderHints(
      state.renderHints || { tier: state.graphTier || "xlarge", point_layer_mode: "clustered" },
      state.graph?.data?.nodes?.length || 0,
      state.graph?.data?.edges?.length || 0,
      state.graphMode || "relation"
    );
    const delay = Math.max(120, Number(options?.delayMs || hints.projection_auto_expand_delay_ms) || 280);
    state.projectionAutoMaterialize.timer = setTimeout(async () => {
      state.projectionAutoMaterialize.timer = 0;
      if (!shouldAutoMaterializeProjectionViewport()) return;
      if (state.graphLoading || state.edgeBatching || state.introAnimating) return;
      const graph = ensureGraph();
      const snapshotRef = cloneResultSnapshotRef(state.resultSnapshotRef);
      const snapshotId = String(snapshotRef?.snapshot_id || snapshotRef?.graph_hash || "").trim();
      if (!graph || !snapshotId) return;
      const viewport = captureGraphViewport(graph);
      if (!viewport) return;
      const zoom = Number.isFinite(Number(viewport?.zoom)) ? Number(viewport.zoom) : Number(graph.getZoom?.()) || 1;
      const minZoom = clampNumber(Number(hints.projection_auto_expand_min_zoom) || 0.18, 0.05, 4);
      if (zoom < minZoom) return;
      const hasLayoutIndex = hasProjectionLayoutSyncForSnapshot(snapshotId);
      let fallbackTargets = { clusterIds: [], tileIds: [], nodeIds: [] };
      if (!hasLayoutIndex) {
        try {
          fallbackTargets = await collectVisibleProjectionExpandTargets(graph, {
            padding: 108,
            maxClusters: Math.max(1, Number(hints.projection_auto_expand_max_clusters) || 24),
            maxNodes: Math.max(1, Number(hints.projection_auto_expand_max_nodes) || 240),
            preferTiles: false,
          });
        } catch (error) {
          log("WARN", "projection viewport targets failed", {
            reason: reason || "projection-auto-viewport",
            message: String(error?.message || error || ""),
          });
          return;
        }
      }
      if (
        !hasLayoutIndex &&
        !fallbackTargets.clusterIds.length &&
        !fallbackTargets.tileIds.length &&
        !fallbackTargets.nodeIds.length
      ) {
        return;
      }
      const signature = buildProjectionAutoMaterializeSignature(snapshotId, viewport);
      const cooldownMs = Math.max(300, Number(hints.projection_auto_expand_cooldown_ms) || 1200);
      if (state.projectionAutoMaterialize?.inflight && state.projectionAutoMaterialize?.inflightSignature === signature) {
        return;
      }
      if (
        signature &&
        signature === String(state.projectionAutoMaterialize?.lastSignature || "") &&
        Date.now() - Number(state.projectionAutoMaterialize?.lastAt || 0) < cooldownMs
      ) {
        return;
      }
      state.projectionAutoMaterialize.inflight = true;
      state.projectionAutoMaterialize.inflightSignature = signature;
      try {
        const ok = await requestProjectionExpand(
          {
            clusterIds: fallbackTargets.clusterIds,
            tileIds: fallbackTargets.tileIds,
            nodeIds: fallbackTargets.nodeIds,
            viewport,
            expandIntent: "auto-viewport",
            includeNeighbors: true,
            neighborDepth: 1,
            hintMaterializeLimit: options?.materializeLimit ?? hints.projection_viewport_materialize_limit,
          },
          { reason: reason || "projection-auto-viewport", preserveViewport: true }
        );
        if (ok) {
          state.projectionAutoMaterialize.lastSignature = signature;
          state.projectionAutoMaterialize.lastAt = Date.now();
        }
      } finally {
        state.projectionAutoMaterialize.inflight = false;
        state.projectionAutoMaterialize.inflightSignature = "";
      }
    }, delay);
    return true;
  }

  function resolveProjectionMaterializeLimit(extra = 2000, { min = 4000, max = 20000 } = {}) {
    const base = Math.max(0, Number(state.graph?.data?.nodes?.length || 0));
    return Math.max(min, Math.min(max, base + Math.max(0, Number(extra) || 0)));
  }

  function normalizeProjectionExpandIntent(intent = "") {
    const token = String(intent || "")
      .trim()
      .toLowerCase()
      .replace(/_/g, "-");
    if (token === "auto" || token === "auto-viewport" || token === "projection-auto-viewport") return "auto-viewport";
    if (token === "viewport" || token === "manual-viewport" || token === "viewport-expand") return "viewport";
    if (token === "path" || token === "selection-path" || token === "selection-path-expand") return "path";
    if (token === "search" || token === "graph-search" || token === "graph-search-hidden") return "search";
    if (token === "cluster" || token === "tile" || token === "cluster-tile") return "cluster";
    return "node";
  }

  function resolveProjectionExpandBudget(intent = "node", options = {}) {
    const currentNodes = Math.max(0, Number(state.graph?.data?.nodes?.length || 0));
    const currentEdges = Math.max(0, Number(state.graph?.data?.edges?.length || 0));
    const hints = normalizeGraphRenderHints(
      state.renderHints || { tier: state.graphTier || "large" },
      currentNodes,
      currentEdges,
      state.graphMode || "relation"
    );
    const tier = normalizeGraphTier(hints.tier || state.graphTier, currentNodes);
    const isHugeCanvas = tier === "xlarge" || currentNodes >= 10000;
    const normalizedIntent = normalizeProjectionExpandIntent(intent);
    const profiles = isHugeCanvas
      ? {
          node: { extra: 1200, min: 1200, max: 3600 },
          cluster: { extra: 1800, min: 1800, max: 5200 },
          viewport: { extra: 2600, min: 3000, max: 9000 },
          "auto-viewport": { extra: 1400, min: 1200, max: 5000 },
          path: { extra: 2200, min: 3200, max: 9000 },
          search: { extra: 1200, min: 1600, max: 4200 },
        }
      : {
          node: { extra: 1800, min: 2200, max: 8000 },
          cluster: { extra: 2400, min: 3000, max: 12000 },
          viewport: { extra: 3200, min: 5000, max: 16000 },
          "auto-viewport": { extra: 1800, min: 2200, max: 7500 },
          path: { extra: 2800, min: 5000, max: 14000 },
          search: { extra: 1600, min: 2200, max: 7000 },
        };
    const profile = profiles[normalizedIntent] || profiles.node;
    const rawRequested = options?.materializeLimit ?? options?.materialize_limit;
    const requestedLimit = rawRequested !== undefined && rawRequested !== null && rawRequested !== "" ? Number(rawRequested) : NaN;
    const rawHint = options?.hintMaterializeLimit ?? options?.hint_materialize_limit ?? options?.hintLimit;
    const hintLimit = rawHint !== undefined && rawHint !== null && rawHint !== "" ? Number(rawHint) : NaN;
    const baseLimit = resolveProjectionMaterializeLimit(profile.extra, { min: profile.min, max: profile.max });
    const requestedOrHintedLimit = Number.isFinite(requestedLimit)
      ? requestedLimit
      : Math.max(baseLimit, Number.isFinite(hintLimit) ? hintLimit : 0);
    const materializeLimit = clampNumber(requestedOrHintedLimit, 1, profile.max);
    return {
      intent: normalizedIntent,
      materializeLimit,
      max: profile.max,
      baseLimit,
      requestedLimit: Number.isFinite(requestedLimit) ? requestedLimit : null,
      hintLimit: Number.isFinite(hintLimit) ? hintLimit : null,
      staged:
        materializeLimit < requestedOrHintedLimit ||
        (Number.isFinite(hintLimit) && materializeLimit < hintLimit) ||
        (Number.isFinite(requestedLimit) && materializeLimit < requestedLimit),
    };
  }

  function buildProjectionShellState() {
    return flowRuntimeStateAdapter.buildProjectionShellState(state, {
      canvasShellAdapter,
      getSelectionState: (graph) => getSelectionState(graph),
      canExpandCurrentProjection,
    });
  }

  function resolveFocusId(graph, focusId, focusName) {
    if (!graph || !focusId) return "";
    const direct = graph.findById?.(focusId);
    if (direct) return focusId;

    const norm = normalizeKey(focusId);
    if (!norm) return "";
    const nodes = graph.getNodes?.() || [];
    for (const n of nodes) {
      const id = n?.getID?.() || n?.getModel?.()?.id || "";
      if (!id) continue;
      if (normalizeKey(id) === norm) return id;
    }
    if (focusName) {
      for (const n of nodes) {
        const model = n?.getModel?.() || {};
        const title = String(model.title || model.label || "").trim();
        if (title && title.includes(focusName)) return model.id || "";
      }
    }
    return "";
  }

  function focusGraphNode(options = {}) {
    const shouldSelect = !!options?.select;
    if (!shouldSelect) return;
    if (!state.focusId && !state.focusName) return;
    try {
      const graph = ensureGraph();
      if (!graph) return;
      const targetId = resolveFocusId(graph, state.focusId, state.focusName);
      if (!targetId) return;
      const item = graph.findById?.(targetId);
      if (item) {
        clearGraphSelection(graph);
        graph.setItemState(item, "selected", true);
        syncActiveSelection(graph, item);
      }
    } catch (e) {}
  }

  function focusGraphItemById(id, { animate = true, pulse = true } = {}) {
    if (!id) return false;
    const graph = ensureGraph();
    if (!graph) return false;
    const item = graph.findById?.(id);
    if (!item) return false;
    try {
      clearGraphSelection(graph);
      graph.setItemState(item, "selected", true);
      graph.setItemState(item, "active", true);
      syncActiveSelection(graph, item);
    } catch (e) {}
    try {
      if (typeof graph.focusItem === "function") {
        graph.focusItem(item, !!animate, { easing: "easeCubic", duration: 260 });
      }
    } catch (e) {}
    if (pulse) startPulseLoop();
    return true;
  }

  function updateCleanPill(status) {
    const pill = $("#cleanPill");
    if (!pill) return;
    if (status === "not-clean") {
      pill.style.display = "";
      pill.textContent = "未清洗";
    } else {
      pill.style.display = "none";
    }
  }

  function syncFundsStatus() {
    if (!state.backend || typeof state.backend.getFundsStatus !== "function") return;
    try {
      state.backend.getFundsStatus((status) => updateCleanPill(status));
    } catch (e) {}
  }

  // ===== Views (multi tabs) =====
  function nowIso() {
    return new Date().toISOString();
  }

  function createViewId() {
    return `view_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`;
  }

  function syncViewCounter() {
    let max = 0;
    for (const v of state.views) {
      const title = String(v?.title || "");
      const m =
        title.match(/^图(\d+)$/) ||
        title.match(/^视图(\d+)$/) ||
        title.match(/^起始页(?:\s*(\d+))?$/);
      if (m) {
        const n = Number(m[1] || (title === "起始页" ? 1 : 0)) || 0;
        max = Math.max(max, n);
      }
    }
    state.viewCounter = Math.max(state.viewCounter, max);
  }

  function nextViewTitle() {
    state.viewCounter += 1;
    return `图${state.viewCounter}`;
  }

  function getActiveView() {
    return state.views.find((v) => v.id === state.activeViewId) || null;
  }

  function viewCounts(view) {
    if (!view) return { nodes: 0, edges: 0 };
    const n = view.counts?.nodes ?? view.graph?.nodes?.length ?? 0;
    const e = view.counts?.edges ?? view.graph?.edges?.length ?? 0;
    return { nodes: n, edges: e };
  }

  function buildEmptyView(title = "") {
    const t = title || nextViewTitle();
    return {
      id: createViewId(),
      title: t,
      mode: state.graphMode,
      focusId: "",
      focusName: "",
      focusLabel: "",
      focusIds: [],
      focusNames: [],
      focusPlaceholderKinds: [],
      focusKeyType: "",
      focusCounterpartyStrict: false,
      source: "",
      focusOnly: false,
      focusSelfOnly: false,
      filters: {
        dir: state.dir,
        hop: state.hop,
        minAmount: state.minAmount,
        maxEdges: state.maxEdges,
      },
      style: { ...state.graphStyle },
      graph: { nodes: [], edges: [] },
      counts: { nodes: 0, edges: 0 },
      saved: false,
      createdAt: nowIso(),
      updatedAt: nowIso(),
    };
  }

  function buildExportSubject(view) {
    const name = String(view?.focusName || state.focusName || "").trim();
    if (name) return ordinaryPiiProjection.projectDetected(name);
    return ordinaryPiiProjection.projectField("account_no", view?.focusId || state.focusId || "");
  }

  function buildExportTitle(view) {
    const caseName = ordinaryPiiProjection.projectDetected(state.caseName || state.caseId || "");
    const subject = buildExportSubject(view);
    const title = ordinaryPiiProjection.projectField("node_id", view?.title || "");
    const parts = [];
    if (caseName) parts.push(caseName);
    if (subject) parts.push(subject);
    else if (title && title !== caseName) parts.push(title);
    const combined = parts.filter(Boolean).join(" ").trim();
    return combined || title || caseName || "图谱";
  }

  function clampNumber(value, min, max) {
    const n = Number(value);
    if (!Number.isFinite(n)) return min;
    return Math.min(max, Math.max(min, n));
  }

  function summarizeGraphData(nodes = [], edges = [], limit = 10) {
    const max = Math.max(1, Number(limit) || 10);
    const pickNodes = nodes.slice(0, max).map((n) => ({
      id: n.id,
      title: n.title,
      name: n.name,
      display_id: n.displayId || n.display_id || "",
      ntype: n.ntype,
      total_amount: n.total_amount ?? null,
      total_count: n.total_count ?? null,
    }));
    const pickEdges = edges.slice(0, max).map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      amount: e.amount ?? null,
      count: e.count ?? null,
      in_amount: e.in_amount ?? null,
      out_amount: e.out_amount ?? null,
      forward_amount: e.forward_amount ?? null,
      reverse_amount: e.reverse_amount ?? null,
      mode: e.mode,
      label: e.label,
    }));
    return {
      counts: { nodes: nodes.length, edges: edges.length },
      sample: {
        nodes: pickNodes,
        edges: pickEdges,
        truncated: { nodes: nodes.length > max, edges: edges.length > max },
      },
    };
  }

  function emitCanvasSnapshotLog(reason, graph = null) {
    const diagnostics = ensureFlowDebugDiagnostics();
    diagnostics?.emitCanvasSnapshotLog?.(reason, graph);
  }

  function parseAmountFromLabel(text) {
    if (!text) return 0;
    const raw = String(text);
    const matches = raw.match(/-?\d[\d,]*\.?\d*/g);
    if (!matches || !matches.length) return 0;
    let max = 0;
    matches.forEach((m) => {
      const val = Number(String(m).replace(/,/g, ""));
      if (Number.isFinite(val)) max = Math.max(max, Math.abs(val));
    });
    return max;
  }

  function mixRgb(base, tint, ratio = 0.5) {
    const r = Math.round(base.r * (1 - ratio) + tint.r * ratio);
    const g = Math.round(base.g * (1 - ratio) + tint.g * ratio);
    const b = Math.round(base.b * (1 - ratio) + tint.b * ratio);
    return { r, g, b };
  }

  function rgbToCss(rgb) {
    if (!rgb) return "";
    return `rgb(${rgb.r}, ${rgb.g}, ${rgb.b})`;
  }

  function normalizeGroupTags(raw) {
    if (!Array.isArray(raw)) return [];
    const seen = new Set();
    const tags = [];
    raw.forEach((val) => {
      const n = Number(val);
      if (!Number.isFinite(n)) return;
      if (n < 1 || n > 7) return;
      if (seen.has(n)) return;
      seen.add(n);
      tags.push(n);
    });
    tags.sort((a, b) => a - b);
    return tags;
  }

  function createNodeId() {
    return `node_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 6)}`;
  }

  function hexToRgb(hex) {
    if (!hex) return null;
    const raw = String(hex).trim();
    if (!raw.startsWith("#")) return null;
    let h = raw.slice(1);
    if (h.length === 3) h = h.split("").map((c) => c + c).join("");
    if (h.length !== 6) return null;
    const num = Number.parseInt(h, 16);
    if (!Number.isFinite(num)) return null;
    return { r: (num >> 16) & 255, g: (num >> 8) & 255, b: num & 255 };
  }

  function mixColor(base, tint, ratio = 0.5) {
    const b = hexToRgb(base);
    const t = hexToRgb(tint);
    if (!b || !t) return tint || base;
    const r = Math.round(b.r * (1 - ratio) + t.r * ratio);
    const g = Math.round(b.g * (1 - ratio) + t.g * ratio);
    const b2 = Math.round(b.b * (1 - ratio) + t.b * ratio);
    return `rgb(${r}, ${g}, ${b2})`;
  }

  function resolveLineDash(value) {
    const match = LINE_STYLE_OPTIONS.find((opt) => opt.value === value);
    return match ? match.dash : [];
  }

  function findOptionLabel(options, value, fallback = "") {
    const match = options.find((opt) => opt.value === value);
    return match ? match.label : fallback;
  }

  function deriveEdgeAmounts(edge, mainId) {
    const inAmt = Number(edge?.in_amount) || 0;
    const outAmt = Number(edge?.out_amount) || 0;
    const forward = Number(edge?.forward_amount) || 0;
    const reverse = Number(edge?.reverse_amount) || 0;
    const hasInOut = inAmt !== 0 || outAmt !== 0;
    const hasForward = forward !== 0 || reverse !== 0;
    const s = edge?.source;
    const t = edge?.target;
    const touchesMain = !!mainId && (s === mainId || t === mainId);
    let amtST = 0;
    let amtTS = 0;
    let inToMain = 0;
    let outFromMain = 0;
    if (touchesMain && hasInOut) {
      outFromMain = outAmt;
      inToMain = inAmt;
      if (s === mainId) {
        amtST = outAmt;
        amtTS = inAmt;
      } else {
        amtST = inAmt;
        amtTS = outAmt;
      }
    } else if (hasForward) {
      amtST = forward;
      amtTS = reverse;
      if (touchesMain) {
        if (s === mainId) {
          outFromMain = forward;
          inToMain = reverse;
        } else {
          outFromMain = reverse;
          inToMain = forward;
        }
      }
    } else {
      amtST = outAmt;
      amtTS = inAmt;
      if (touchesMain) {
        outFromMain = outAmt;
        inToMain = inAmt;
      }
    }
    return { amtST, amtTS, inToMain, outFromMain, touchesMain };
  }

  function ensureRibbonStyle() {
    if (!state.ribbonStyle) state.ribbonStyle = { ...state.graphStyle };
  }

  function setEdgeDirectionLocked(locked) {
    state.edgeDirectionLocked = !!locked;
    syncEdgeDirectionControl(state.graph.instance);
  }

  function normalizeOutlinePartial(partial) {
    if (!partial) return partial;
    const next = { ...partial };
    if (next.outlineColor != null) {
      next.edgeColor = next.outlineColor;
      next.nodeColor = next.outlineColor;
      delete next.outlineColor;
    }
    if (next.edgeColor != null && next.nodeColor == null) next.nodeColor = next.edgeColor;
    if (next.nodeColor != null && next.edgeColor == null) next.edgeColor = next.nodeColor;
    return next;
  }

  function syncRibbonUI(style) {
    if (!style) return;
    const fontLabel = FONT_LIST.find((f) => f.value === style.fontFamily)?.label || "字体";
    const fontFamilyLabel = $("#fontFamilyLabel");
    if (fontFamilyLabel) fontFamilyLabel.textContent = fontLabel;
    const fontSizeLabel = $("#fontSizeLabel");
    if (fontSizeLabel) fontSizeLabel.textContent = String(style.fontSize ?? 12);

    const setToggle = (id, on) => {
      const btn = $("#" + id);
      if (!btn) return;
      btn.classList.toggle("active", !!on);
      btn.setAttribute("aria-pressed", on ? "true" : "false");
    };

    setToggle("btnBold", asBool(style.fontBold, false));
    setToggle("btnItalic", asBool(style.fontItalic, false));
    setToggle("btnUnderline", asBool(style.fontUnderline, false));
    setToggle("btnShadow", asBool(style.fontShadow, false));

    const setSwatch = (id, color) => {
      const el = $("#" + id);
      if (!el) return;
      el.style.background = color || "#1f2937";
    };

    const outlineColor = style.nodeColor ?? style.edgeColor ?? "#1f6feb";
    setSwatch("textColorSwatch", style.textColor);
    setSwatch("outlineColorSwatch", outlineColor);
    setSwatch("lineColorSwatch", outlineColor);
    setSwatch("nodeFillSwatch", style.nodeFill || "rgba(255,255,255,0.05)");

    const lineStyleLabel = $("#lineStyleLabel");
    if (lineStyleLabel) {
      lineStyleLabel.textContent = "线型";
      lineStyleLabel.setAttribute("title", findOptionLabel(LINE_STYLE_OPTIONS, style.edgeDash, "线型"));
    }

    const lineWidthLabel = $("#lineWidthLabel");
    if (lineWidthLabel) {
      lineWidthLabel.textContent = "线宽";
      lineWidthLabel.setAttribute("title", `线宽 ${style.edgeWidth ?? 2}`);
    }

    const lineArrowLabel = $("#lineArrowLabel");
    if (lineArrowLabel) {
      lineArrowLabel.textContent = "方向";
      lineArrowLabel.setAttribute("title", findOptionLabel(ARROW_OPTIONS, style.edgeArrow, "方向"));
    }

    const nodeShapeLabel = $("#nodeShapeLabel");
    if (nodeShapeLabel) {
      nodeShapeLabel.textContent = "形状";
      nodeShapeLabel.setAttribute("title", findOptionLabel(NODE_SHAPE_OPTIONS, style.nodeShape, "形状"));
    }

    const iconSizeLabel = $("#iconSizeLabel");
    if (iconSizeLabel) {
      iconSizeLabel.textContent = "大小";
      iconSizeLabel.setAttribute("title", `大小 ${style.nodeSize ?? 18}`);
    }

    syncGhostFloatStyle(style);
  }

  function setToolbarStyle(style) {
    if (!style) return;
    const next = { ...state.graphStyle, ...style };
    state.graphStyle = next;
    ensureRibbonStyle();
    state.ribbonStyle = { ...state.ribbonStyle, ...next };
    syncRibbonUI(state.ribbonStyle);
    scheduleShellStateSync("toolbar-style");
  }

  function buildNodeUpdate(style) {
    const out = {};
    if (style.nodeColor != null) out.stroke = style.nodeColor;
    if (style.nodeFill != null) out.fill = style.nodeFill;
    if (style.nodeWidth != null) out.lineWidth = style.nodeWidth;
    if (style.nodeSize != null) out.r = style.nodeSize;
    if (style.textColor != null) {
      out.textColor = style.textColor;
      out.subTextColor = style.textColor;
    }
    if (style.fontFamily != null) out.fontFamily = style.fontFamily;
    if (style.fontSize != null) out.fontSize = style.fontSize;
    if (style.fontBold != null) out.fontBold = style.fontBold;
    if (style.fontItalic != null) out.fontItalic = style.fontItalic;
    if (style.fontUnderline != null) out.fontUnderline = style.fontUnderline;
    if (style.fontShadow != null) out.fontShadow = style.fontShadow;
    if (style.nodeShadow != null) out.nodeShadow = style.nodeShadow;
    if (style.iconSymbol != null) out.iconSymbol = style.iconSymbol;
    if (style.iconSize != null) out.iconSize = style.iconSize;
    if (style.nodeShape != null) out.nodeShape = style.nodeShape;
    return out;
  }

  function buildEdgeUpdate(style) {
    const out = {};
    if (style.edgeColor != null) out.stroke = style.edgeColor;
    if (style.edgeWidth != null) out.lineWidth = style.edgeWidth;
    if (style.edgeDash != null) out.edgeDash = style.edgeDash;
    if (style.edgeArrow != null) out.edgeArrow = style.edgeArrow;
    if (style.edgeMode != null) out.mode = style.edgeMode;
    if (style.textColor != null) out.textColor = style.textColor;
    if (style.fontFamily != null) out.fontFamily = style.fontFamily;
    if (style.fontSize != null) out.fontSize = style.fontSize;
    if (style.fontBold != null) out.fontBold = style.fontBold;
    if (style.fontItalic != null) out.fontItalic = style.fontItalic;
    if (style.fontUnderline != null) out.fontUnderline = style.fontUnderline;
    if (style.fontShadow != null) out.fontShadow = style.fontShadow;
    return out;
  }

  function captureNodePositions(graph) {
    const map = new Map();
    if (!graph || typeof graph.getNodes !== "function") return map;
    graph.getNodes().forEach((node) => {
      const model = node.getModel?.();
      const id = model?.id;
      if (!id) return;
      const x = model?.x;
      const y = model?.y;
      if (Number.isFinite(x) && Number.isFinite(y)) map.set(id, { x, y });
    });
    return map;
  }

  function restoreNodePositions(graph, positions) {
    if (!graph || !positions || !positions.size || typeof graph.getNodes !== "function") return;
    graph.getNodes().forEach((node) => {
      const model = node.getModel?.();
      const id = model?.id;
      if (!id) return;
      const pos = positions.get(id);
      if (!pos) return;
      if (Number.isFinite(pos.x)) model.x = pos.x;
      if (Number.isFinite(pos.y)) model.y = pos.y;
    });
    graph.refreshPositions?.();
  }

  function getSelectedGraphItems(graph) {
    if (!graph || typeof graph.findAllByState !== "function") return { nodes: [], edges: [] };
    const nodes = graph.findAllByState("node", "selected") || [];
    const edges = graph.findAllByState("edge", "selected") || [];
    return { nodes, edges };
  }

  function canEditEdgeDirection(graph) {
    if (!state.edgeDirectionLocked) return true;
    const { edges } = getSelectedGraphItems(graph);
    if (!edges.length) return true;
    return edges.every((edge) => edge.getModel?.()?.userCreated);
  }

  function syncEdgeDirectionControl(graph) {
    const btn = $("#btnLineArrow");
    if (!btn) return;
    const canEdit = canEditEdgeDirection(graph);
    btn.disabled = !canEdit;
    btn.setAttribute("aria-disabled", canEdit ? "false" : "true");
    if (canEdit) {
      btn.setAttribute("title", "设置线条方向");
    } else {
      btn.setAttribute("title", "方向已锁定");
    }
  }

  function selectedGraphNodeIds(nodes = []) {
    return (Array.isArray(nodes) ? nodes : [])
      .map((node) => String(node?.getID?.() || node?.getModel?.()?.id || "").trim())
      .filter(Boolean);
  }

  function selectedGraphEdgeIds(edges = []) {
    return (Array.isArray(edges) ? edges : [])
      .map((edge) => {
        const model = edge?.getModel?.() || {};
        return (
          String(edge?.getID?.() || model.id || model.edge_id || "").trim() ||
          (model.source && model.target ? `${String(model.source).trim()}->${String(model.target).trim()}` : "")
        );
      })
      .filter(Boolean);
  }

  function clipboardNodeId(node) {
    return String(node?.id || node?.node_id || node?.display_id || node?.displayId || "").trim();
  }

  function createPastedEdgeId(edge, index, usedIds) {
    const source = String(edge?.source || edge?.from_node_id || "source").replace(/[^\w-]+/g, "_") || "source";
    const target = String(edge?.target || edge?.to_node_id || "target").replace(/[^\w-]+/g, "_") || "target";
    const stamp = Date.now().toString(36);
    let suffix = Math.max(1, Number(index) + 1 || 1);
    let id = `edge_paste_${source}_${target}_${stamp}_${suffix}`;
    while (usedIds?.has?.(id)) {
      suffix += 1;
      id = `edge_paste_${source}_${target}_${stamp}_${suffix}`;
    }
    return id;
  }

  function selectPastedGraphItems(graph, nodeIds = [], edgeIds = []) {
    if (!graph) return;
    clearGraphSelection(graph);
    const nodeSet = new Set((Array.isArray(nodeIds) ? nodeIds : []).map((id) => String(id || "").trim()).filter(Boolean));
    const edgeSet = new Set((Array.isArray(edgeIds) ? edgeIds : []).map((id) => String(id || "").trim()).filter(Boolean));
    let active = null;
    nodeSet.forEach((id) => {
      const item = graph.findById?.(id);
      if (!item) return;
      graph.setItemState?.(item, "selected", true);
      active = item;
    });
    edgeSet.forEach((id) => {
      const item = graph.findById?.(id);
      if (!item) return;
      graph.setItemState?.(item, "selected", true);
    });
    syncActiveSelection(graph, active);
    syncEdgeDirectionControl(graph);
  }

  async function copyGraphSelectionToClipboard() {
    const graph = ensureGraph();
    if (!graph) return false;
    const { nodes, edges } = getSelectedGraphItems(graph);
    if (!nodes.length && !edges.length) return false;
    const payload = graphClipboardModel.buildGraphClipboardPayload({
      graphData: state.graph.data,
      selectedNodeIds: selectedGraphNodeIds(nodes),
      selectedEdgeIds: selectedGraphEdgeIds(edges),
      sourceViewId: state.activeViewId,
    });
    if (!payload) return false;
    const copied = await writeGraphClipboardText(JSON.stringify(payload));
    if (!copied) {
      toast("完整图谱复制已阻止：当前没有可验证的受控敏感信息复制授权", "warn");
      return false;
    }
    toast(`已复制 ${payload.nodes.length} 个节点、${payload.edges.length} 条线`, "ok");
    return true;
  }

  async function pasteGraphClipboardFromClipboard() {
    const graph = ensureGraph();
    if (!graph) return false;
    const text = await readGraphClipboardText();
    const payload = graphClipboardModel.parseGraphClipboardPayload(text);
    if (!payload) return false;
    const merge = graphClipboardModel.mergeGraphClipboardPayload(state.graph.data, payload, {
      createEdgeId: createPastedEdgeId,
    });
    const addedNodeCount = merge.addedNodes.length;
    const addedEdgeCount = merge.addedEdges.length;
    if (!addedNodeCount && !addedEdgeCount) {
      toast("粘贴内容已存在", "warn");
      return false;
    }
    recordUndoSnapshot();
    cancelGraphTransientTasks("paste-graph-selection");
    const viewport = captureGraphViewport(graph);
    const nextGraph = replaceRuntimeGraphData(merge.graph, { reason: "paste-graph-selection-state" });
    try {
      graph.changeData(nextGraph);
    } catch (e) {
      try {
        graph.data(nextGraph);
        graph.render();
      } catch (e2) {}
    }
    if (viewport) applyGraphViewport(viewport, graph);
    state.layoutTopologyDirty = true;
    terminateLayoutPrewarmTask("topology-dirty");
    layoutPlanCacheController.clearCaches();
    networkSectorPlacementCache.clear();
    applyAnalysisEncodings();
    if (state.edgeDetail) applyEdgeDetailLabels(true);
    updateGraphStats();
    updateMinimap();
    updateCurrentViewSnapshot();
    updateLayoutButtons();
    syncAnalysisSourceDataFromCurrent();
    scheduleGraphPatch({
      scope: "local-update",
      reason: "paste-graph-selection",
    });
    selectPastedGraphItems(
      graph,
      payload.nodes.map((node) => clipboardNodeId(node)).filter(Boolean),
      merge.addedEdges
    );
    toast(`已粘贴 ${addedNodeCount} 个新节点、${addedEdgeCount} 条新线`, "ok");
    return true;
  }

  function updateGraphDataRowsById(graphData, listKey, ids, updateForRow) {
    if (!graphData || !Array.isArray(graphData?.[listKey]) || !ids?.size || typeof updateForRow !== "function") {
      return 0;
    }
    let changed = 0;
    graphData[listKey].forEach((row) => {
      const id = row?.id;
      if (!ids.has(id)) return;
      const update = updateForRow(row);
      if (!update || typeof update !== "object" || Array.isArray(update)) return;
      Object.assign(row, update);
      changed += 1;
    });
    return changed;
  }

  function updateGraphDataById(items, update, listKey) {
    if (!update || !items || !items.length) return;
    const ids = new Set(items.map((item) => item.getModel?.()?.id ?? item.getID?.() ?? ""));
    if (!ids.size) return;
    const applyUpdate = () => update;
    const changed =
      updateGraphDataRowsById(state.graph.data, listKey, ids, applyUpdate) +
      updateGraphDataRowsById(state.analysisSourceData, listKey, ids, applyUpdate);
    if (changed > 0) {
      scheduleGraphPatch({
        scope: "local-update",
        reason: `graph-data-${String(listKey || "items")}-update`,
      });
    }
  }

  function syncGraphNodePositions(graph) {
    if (!graph || typeof graph.getNodes !== "function") return;
    const nodes = graph.getNodes() || [];
    if (!nodes.length) return;
    const target = state.graph.data?.nodes;
    if (!Array.isArray(target) || !target.length) return;
    const map = new Map(target.map((n) => [n.id, n]));
    let changed = false;
    nodes.forEach((node) => {
      const model = node.getModel?.() || {};
      const id = model.id;
      if (!id) return;
      const t = map.get(id);
      if (!t) return;
      if (Number.isFinite(model.x) && Number(model.x) !== Number(t.x)) {
        t.x = model.x;
        changed = true;
      }
      if (Number.isFinite(model.y) && Number(model.y) !== Number(t.y)) {
        t.y = model.y;
        changed = true;
      }
    });
    return changed;
  }

  function applyStyleToSelection(partial) {
    const graph = ensureGraph();
    if (!graph) return false;
    const { nodes, edges } = getSelectedGraphItems(graph);
    if (!nodes.length && !edges.length) return false;
    recordUndoSnapshot();
    const nodeUpdate = buildNodeUpdate(partial);
    const edgeUpdate = buildEdgeUpdate(partial);
    const shouldFreeze = !state.introAnimating;
    const positions = shouldFreeze ? captureNodePositions(graph) : null;
    graph.setAutoPaint(false);
    try {
      if (nodes.length && Object.keys(nodeUpdate).length) {
        nodes.forEach((node) => graph.updateItem(node, nodeUpdate));
        updateGraphDataById(nodes, nodeUpdate, "nodes");
      }
      if (edges.length && Object.keys(edgeUpdate).length) {
        edges.forEach((edge) => graph.updateItem(edge, edgeUpdate));
        updateGraphDataById(edges, edgeUpdate, "edges");
      }
    } finally {
      if (positions) restoreNodePositions(graph, positions);
      graph.setAutoPaint(true);
      graph.paint();
    }
    return true;
  }

  function updateNodeGroupTags(nodes, updater) {
    const graph = ensureGraph();
    if (!graph || !nodes || !nodes.length) return false;
    const nextById = new Map();
    graph.setAutoPaint(false);
    try {
      nodes.forEach((node) => {
        const model = node.getModel?.() || {};
        const next = normalizeGroupTags(updater(model) || []);
        graph.updateItem(node, { groupTags: next });
        const id = model.id;
        if (id) nextById.set(id, next);
      });
    } finally {
      graph.setAutoPaint(true);
      graph.paint();
    }
    if (nextById.size) {
      const ids = new Set(nextById.keys());
      const applyGroupTags = (row) => {
        const tags = nextById.get(row?.id);
        return tags ? { groupTags: tags.slice() } : null;
      };
      const changed =
        updateGraphDataRowsById(state.graph.data, "nodes", ids, applyGroupTags) +
        updateGraphDataRowsById(state.analysisSourceData, "nodes", ids, applyGroupTags);
      if (changed > 0) {
        scheduleGraphPatch({
          scope: "local-update",
          reason: "graph-data-node-group-tags-update",
        });
      }
    }
    return true;
  }

  function edgeMetric(model) {
    const { forward, reverse } = edgeDirectionalAmounts(model);
    const total = Math.abs(forward) + Math.abs(reverse);
    if (total > 0) return total;
    const labelAmt = parseAmountFromLabel(model.label);
    if (labelAmt) return labelAmt;
    const count = Math.abs(Number(model.count) || 0);
    return count || 0;
  }

  function isSelfLoopEdge(model) {
    if (!model) return false;
    if (model.type === "self-loop") return true;
    const s = model.source;
    const t = model.target;
    return s != null && t != null && String(s) === String(t);
  }

  function edgeDirectionalAmounts(model) {
    const forward = Math.abs(Number(model.forward_amount) || 0);
    const reverse = Math.abs(Number(model.reverse_amount) || 0);
    if (forward || reverse) return { forward, reverse };
    const top = parseAmountFromLabel(model.labelTop);
    const bot = parseAmountFromLabel(model.labelBottom);
    if (top || bot) return { forward: top, reverse: bot };
    const arrow = model.edgeArrow || model.arrow || (model.showArrow ? "end" : "none");
    const flip = arrow === "start";
    const amount = Math.abs(Number(model.amount) || 0);
    if (amount) return flip ? { forward: 0, reverse: amount } : { forward: amount, reverse: 0 };
    const outAmt = Math.abs(Number(model.out_amount) || 0);
    const inAmt = Math.abs(Number(model.in_amount) || 0);
    if (outAmt || inAmt) return flip ? { forward: inAmt, reverse: outAmt } : { forward: outAmt, reverse: inAmt };
    const labelAmt = parseAmountFromLabel(model.label);
    if (labelAmt) return flip ? { forward: 0, reverse: labelAmt } : { forward: labelAmt, reverse: 0 };
    const count = Math.abs(Number(model.count) || 0);
    return flip ? { forward: 0, reverse: count } : { forward: count, reverse: 0 };
  }

  function resolveEdgeLane(model, fallback = "") {
    const raw = String(model?.__selectedLane || model?.__hitLane || fallback || "").trim().toLowerCase();
    if (raw === "bottom") return "bottom";
    if (raw === "top") return "top";
    return "";
  }

  function edgeLaneAmounts(model) {
    const { forward, reverse } = edgeDirectionalAmounts(model || {});
    return {
      top: Math.abs(Number(forward) || 0),
      bottom: Math.abs(Number(reverse) || 0),
    };
  }

  function edgeAmountByLane(model, lane = "") {
    const m = model || {};
    const isDouble = String(m.mode || "").toLowerCase() === "double";
    const normalizedLane = resolveEdgeLane(m, lane);
    // For double-edges, prefer lane labels (what user sees on each line) to avoid
    // backend directional fields that may carry aggregated totals.
    const topLabelAmt = parseAmountWithCurrency(m.labelTop);
    const bottomLabelAmt = parseAmountWithCurrency(m.labelBottom);
    const { top: topDirAmt, bottom: bottomDirAmt } = edgeLaneAmounts(m);
    const top = topLabelAmt > 0 ? topLabelAmt : topDirAmt;
    const bottom = bottomLabelAmt > 0 ? bottomLabelAmt : bottomDirAmt;
    if (!isDouble) {
      const single = top + bottom;
      return single > 0 ? single : edgeAmountTotal(m);
    }
    if (normalizedLane === "bottom") return bottom;
    if (normalizedLane === "top") return top;
    return top || bottom || 0;
  }

  function resolveSingleEdgeLane(model) {
    const arrow = String(model?.edgeArrow || model?.arrow || "").trim().toLowerCase();
    if (arrow === "start") return "bottom";
    if (arrow === "end" || arrow === "both") return "top";
    return "";
  }

  function formatSignedMoney(value) {
    const n = Number(value) || 0;
    const sign = n < 0 ? "-" : "+";
    return `${sign}￥${fmtMoney(Math.abs(n))}`;
  }

  function isSignedEdgeDetailEnabled(model) {
    const mode = String(model?.mode || "").toLowerCase();
    if (mode === "double") return false;
    return true;
  }

  function resolveEdgeTxnRowDirection(row, edgeSigned = null) {
    const direct = String(row?.__dir || row?.__signClass || "").trim().toLowerCase();
    if (direct === "out" || direct === "in") return direct;
    const dc = String(row?.dc_flag || row?.dcFlag || "").trim();
    if (dc.includes("出")) return "out";
    if (dc.includes("进") || dc.includes("入")) return "in";
    const signedAmount = Number(row?.__signedAmount);
    if (Number.isFinite(signedAmount) && signedAmount < 0) return "out";
    if (Number.isFinite(signedAmount) && signedAmount > 0) return "in";
    const flow = String(row?.__flow || "").trim();
    if (flow === "reverse") return "out";
    if (flow === "forward") return "in";
    return Number(edgeSigned?.sign) < 0 ? "out" : "in";
  }

  function edgeSignedMeta(model, lane = "") {
    const m = model || {};
    const mode = String(m.mode || "").toLowerCase();
    const isDouble = mode === "double";
    const { top, bottom } = edgeLaneAmounts(m);
    let dominantLane = "";
    if (top > 0 || bottom > 0) dominantLane = top >= bottom ? "top" : "bottom";

    let selectedLane = resolveEdgeLaneForDialog(m, lane);
    if (!selectedLane) {
      if (isDouble) selectedLane = dominantLane || "top";
      else selectedLane = resolveSingleEdgeLane(m) || dominantLane || "top";
    }

    const laneAmount = edgeAmountByLane(m, selectedLane);
    let sign = 1;
    if (dominantLane) sign = selectedLane === dominantLane ? 1 : -1;
    const signedAmount = sign >= 0 ? Math.abs(laneAmount) : -Math.abs(laneAmount);
    const signClass = sign >= 0 ? "in" : "out";
    const signLabel = sign >= 0 ? "进" : "出";
    return {
      selectedLane,
      dominantLane,
      sign,
      signClass,
      signLabel,
      laneAmount: Math.abs(laneAmount),
      signedAmount,
    };
  }

  function inferEdgeLaneByPoint(graph, model, ev) {
    if (!graph || !model) return "";
    if (String(model.mode || "").toLowerCase() !== "double") return "";
    const sxNode = graph.findById?.(model.source)?.getModel?.();
    const txNode = graph.findById?.(model.target)?.getModel?.();
    if (!sxNode || !txNode) return "";
    const sx = Number(sxNode.x);
    const sy = Number(sxNode.y);
    const tx = Number(txNode.x);
    const ty = Number(txNode.y);
    const px = Number(ev?.x ?? ev?.canvasX);
    const py = Number(ev?.y ?? ev?.canvasY);
    if (![sx, sy, tx, ty, px, py].every(Number.isFinite)) return "";
    const dx = tx - sx;
    const dy = ty - sy;
    const len = Math.hypot(dx, dy);
    if (len < 1e-6) return "";
    const ux = dx / len;
    const uy = dy / len;
    const perpX = -uy;
    const perpY = ux;
    const mx = (sx + tx) / 2;
    const my = (sy + ty) / 2;
    const sign = (px - mx) * perpX + (py - my) * perpY;
    return sign >= 0 ? "top" : "bottom";
  }

  function rememberEdgeLane(model, lane) {
    const edgeId = String(model?.id || "").trim();
    const normalized = resolveEdgeLane(model, lane);
    if (!edgeId || !normalized) return;
    state.edgeLaneById[edgeId] = normalized;
  }

  function resolveEdgeLaneForDialog(model, fallback = "") {
    const direct = resolveEdgeLane(model, fallback);
    if (direct) return direct;
    const edgeId = String(model?.id || "").trim();
    if (edgeId) {
      const cached = resolveEdgeLane(model, state.edgeLaneById[edgeId] || "");
      if (cached) return cached;
    }
    const top = parseAmountWithCurrency(model?.labelTop);
    const bottom = parseAmountWithCurrency(model?.labelBottom);
    if (top > 0 && bottom <= 0) return "top";
    if (bottom > 0 && top <= 0) return "bottom";
    return "";
  }

  function resolveEdgeDirectionByLane(model, lane = "") {
    const source = String(model?.source || "").trim();
    const target = String(model?.target || "").trim();
    const mode = String(model?.mode || "").toLowerCase();
    const selectedLane = resolveEdgeLaneForDialog(model, lane);
    if (!source || !target) return { from: source, to: target, lane: selectedLane };
    if (mode === "double") {
      if (selectedLane === "bottom") return { from: target, to: source, lane: "bottom" };
      return { from: source, to: target, lane: "top" };
    }
    const arrow = String(model?.edgeArrow || model?.arrow || "").toLowerCase();
    if (arrow === "start") return { from: target, to: source, lane: selectedLane };
    return { from: source, to: target, lane: selectedLane };
  }

  function edgeNodeTitleById(id) {
    const node = edgeNodeModelById(id);
    return String(node?.title || node?.name || "").trim();
  }

  function edgeNodeModelById(id) {
    const nid = String(id || "").trim();
    if (!nid) return null;
    const nodes = state.graph.data?.nodes || [];
    return nodes.find((n) => String(n?.id || "").trim() === nid) || null;
  }

  function edgeNodeAccountKeysById(id) {
    const out = [];
    const seen = new Set();
    const push = (value) => {
      const key = normalizeKey(value);
      if (!key || seen.has(key)) return;
      seen.add(key);
      out.push(key);
    };
    const node = edgeNodeModelById(id);
    const displayIds = collectNodeDisplayIds(node || {});
    displayIds.forEach((val) => push(val));
    // Keep node id as fallback for non-account keyed nodes.
    push(id);
    return out;
  }

  function edgeNodePlaceholderMetaById(id) {
    const node = edgeNodeModelById(id);
    const nodeId = String(node?.id || id || "").trim();
    const labelCandidates = [
      String(node?.title || "").trim(),
      String(node?.name || "").trim(),
      String(node?.displayId || node?.display_id || "").trim(),
      ...collectNodeDisplayIds(node || {}),
    ].filter(Boolean);
    const kind =
      placeholderKindFromToken(nodeId) || labelCandidates.map((value) => placeholderKindFromLabel(value)).find(Boolean) || "";
    if (!kind) return { kind: "", label: "", name: "" };
    const label = String(PLACEHOLDER_KIND_LABELS[kind] || "").trim();
    const tokenName = placeholderNameFromToken(nodeId);
    const fallbackName = labelCandidates.find((value) => value && value !== label) || "";
    const name = isUnknownAccountLabel(tokenName || fallbackName, { exact: true }) ? "" : tokenName || fallbackName;
    return { kind, label, name };
  }

  function computeNodeFlowTotals(edges) {
    const totals = new Map();
    const degree = new Map();
    edges.forEach((edge) => {
      const model = edge.getModel?.() || {};
      const s = model.source;
      const t = model.target;
      if (!s || !t) return;
      const { forward, reverse } = edgeDirectionalAmounts(model);
      const sum = totals.get(s) || { in: 0, out: 0 };
      const tum = totals.get(t) || { in: 0, out: 0 };
      sum.out += forward;
      sum.in += reverse;
      tum.in += forward;
      tum.out += reverse;
      totals.set(s, sum);
      totals.set(t, tum);
      degree.set(s, (degree.get(s) || 0) + 1);
      degree.set(t, (degree.get(t) || 0) + 1);
    });
    return { totals, degree };
  }

  function applyEdgeWidthEncoding(on) {
    const graph = ensureGraph();
    if (!graph) return;
    const edges = graph.getEdges?.() || [];
    if (!edges.length) return;
    const entriesAll = edges.map((edge) => {
      const model = edge.getModel?.() || {};
      return { edge, model, value: edgeMetric(model) };
    });
    const entries = entriesAll.filter((e) => !isSelfLoopEdge(e.model));
    const values = entries.map((e) => e.value).filter((v) => Number.isFinite(v));
    const min = values.length ? Math.min(...values) : 0;
    const max = values.length ? Math.max(...values) : 0;
    graph.setAutoPaint(false);
    try {
      entriesAll.forEach(({ edge, model, value }) => {
        const base = model.__baseLineWidth ?? model.lineWidth ?? state.graphStyle.edgeWidth ?? 2;
        const nextBase = model.__baseLineWidth == null ? base : model.__baseLineWidth;
        let lineWidth = base;
        if (on && !isSelfLoopEdge(model)) {
          const rawT = max > min ? (value - min) / (max - min) : 0.5;
          const t = Math.pow(clampNumber(rawT, 0, 1), 0.6);
          const minW = clampNumber(base * 0.6, 1, 10);
          const maxW = clampNumber(base * 4.5, minW + 1, 20);
          lineWidth = minW + (maxW - minW) * t;
        } else {
          lineWidth = nextBase ?? base;
        }
        const update = { lineWidth };
        if (model.__baseLineWidth == null) update.__baseLineWidth = nextBase;
        graph.updateItem(edge, update);
        graph.refreshItem?.(edge);
      });
    } finally {
      graph.setAutoPaint(true);
      graph.paint();
    }
    const data = state.graph.data?.edges;
    if (Array.isArray(data)) {
      const map = new Map(data.map((e) => [e.id, e]));
      entriesAll.forEach(({ model, value }) => {
        const id = model.id;
        if (!id || !map.has(id)) return;
        const base = model.__baseLineWidth ?? model.lineWidth ?? state.graphStyle.edgeWidth ?? 2;
        const nextBase = model.__baseLineWidth == null ? base : model.__baseLineWidth;
        let lineWidth = base;
        if (on && !isSelfLoopEdge(model)) {
          const rawT = max > min ? (value - min) / (max - min) : 0.5;
          const t = Math.pow(clampNumber(rawT, 0, 1), 0.6);
          const minW = clampNumber(base * 0.6, 1, 10);
          const maxW = clampNumber(base * 4.5, minW + 1, 20);
          lineWidth = minW + (maxW - minW) * t;
        } else {
          lineWidth = nextBase ?? base;
        }
        const target = map.get(id);
        target.lineWidth = lineWidth;
        if (target.__baseLineWidth == null) target.__baseLineWidth = nextBase;
      });
    }
  }

  function applyEdgeColorEncoding(on) {
    const graph = ensureGraph();
    if (!graph) return;
    const edges = graph.getEdges?.() || [];
    if (!edges.length) return;
    const entriesAll = edges.map((edge) => {
      const model = edge.getModel?.() || {};
      return { edge, model, value: edgeMetric(model) };
    });
    const entries = entriesAll.filter((e) => !isSelfLoopEdge(e.model));
    const values = entries.map((e) => e.value).filter((v) => Number.isFinite(v));
    const min = values.length ? Math.min(...values) : 0;
    const max = values.length ? Math.max(...values) : 0;
    graph.setAutoPaint(false);
    try {
      entriesAll.forEach(({ edge, model, value }) => {
        const baseStroke = model.__baseStroke ?? model.stroke ?? state.graphStyle.edgeColor ?? "#1f2937";
        const startRgb = { r: 232, g: 234, b: 238 };
        const endRgb = { r: 160, g: 0, b: 0 };
        const rawT = max > min ? (value - min) / (max - min) : value > 0 ? 1 : 0;
        const t = Math.pow(clampNumber(rawT, 0, 1), 0.85);
        const stroke = on && !isSelfLoopEdge(model) ? rgbToCss(mixRgb(startRgb, endRgb, t)) : baseStroke;
        const update = { stroke };
        if (model.__baseStroke == null) update.__baseStroke = baseStroke;
        graph.updateItem(edge, update);
        graph.refreshItem?.(edge);
      });
    } finally {
      graph.setAutoPaint(true);
      graph.paint();
    }
    const data = state.graph.data?.edges;
    if (Array.isArray(data)) {
      const map = new Map(data.map((e) => [e.id, e]));
      entriesAll.forEach(({ model, value }) => {
        const id = model.id;
        if (!id || !map.has(id)) return;
        const baseStroke = model.__baseStroke ?? model.stroke ?? state.graphStyle.edgeColor ?? "#1f2937";
        const startRgb = { r: 232, g: 234, b: 238 };
        const endRgb = { r: 160, g: 0, b: 0 };
        const rawT = max > min ? (value - min) / (max - min) : value > 0 ? 1 : 0;
        const t = Math.pow(clampNumber(rawT, 0, 1), 0.85);
        const stroke = on && !isSelfLoopEdge(model) ? rgbToCss(mixRgb(startRgb, endRgb, t)) : baseStroke;
        const target = map.get(id);
        target.stroke = stroke;
        if (target.__baseStroke == null) target.__baseStroke = baseStroke;
      });
    }
  }

  function applyNodeScaleEncoding(on) {
    const graph = ensureGraph();
    if (!graph) return;
    const nodes = graph.getNodes?.() || [];
    if (!nodes.length) return;
    const edges = graph.getEdges?.() || [];
    const { totals, degree } = computeNodeFlowTotals(edges);
    const entries = nodes.map((node) => {
      const model = node.getModel?.() || {};
      const flow = totals.get(model.id) || { in: 0, out: 0 };
      const total = flow.in + flow.out;
      const deg = degree.get(model.id) || 0;
      return { node, model, value: total, deg };
    });
    let values = entries.map((e) => e.value).filter((v) => Number.isFinite(v));
    let min = values.length ? Math.min(...values) : 0;
    let max = values.length ? Math.max(...values) : 0;
    if (Math.abs(max - min) < 1e-6) {
      entries.forEach((e) => {
        e.value = e.deg || 0;
      });
      values = entries.map((e) => e.value).filter((v) => Number.isFinite(v));
      min = values.length ? Math.min(...values) : 0;
      max = values.length ? Math.max(...values) : 0;
    }
    graph.setAutoPaint(false);
    try {
      entries.forEach(({ node, model, value }) => {
        const base = resolveNodeBaseRadius(model, state.graphStyle);
        const nextBase = base;
        const baseLineWidth = resolveNodeBaseLineWidth(model, state.graphStyle);
        let r = base;
        let lineWidth = baseLineWidth;
        if (on) {
          const rawT = max > min ? (value - min) / (max - min) : 0.5;
          const t = Math.pow(clampNumber(rawT, 0, 1), 0.65);
          const minR = clampNumber(base * 0.7, 10, 32);
          const maxR = clampNumber(base * 2.4, minR + 2, 64);
          r = minR + (maxR - minR) * t;
          const widthScale = Math.sqrt(Math.max(r / Math.max(nextBase, 1), 0.01));
          lineWidth = clampNumber(
            baseLineWidth * (0.82 + widthScale * 0.36),
            Math.max(0.8, baseLineWidth * 0.75),
            Math.max(baseLineWidth + 1.2, baseLineWidth * 1.6)
          );
        } else {
          r = nextBase ?? base;
          lineWidth = baseLineWidth;
        }
        const update = {
          r,
          lineWidth,
          __analysisScaledR: on ? r : null,
          __analysisScaledLineWidth: on ? lineWidth : null
        };
        if (model.__baseR == null) update.__baseR = nextBase;
        if (model.__baseLineWidth == null) update.__baseLineWidth = baseLineWidth;
        graph.updateItem(node, update);
      });
      edges.forEach((edge) => graph.refreshItem?.(edge));
    } finally {
      graph.setAutoPaint(true);
      graph.paint();
    }
    const data = state.graph.data?.nodes;
    if (Array.isArray(data)) {
      const map = new Map(data.map((n) => [n.id, n]));
      entries.forEach(({ model, value }) => {
        const id = model.id;
        if (!id || !map.has(id)) return;
        const base = resolveNodeBaseRadius(model, state.graphStyle);
        const nextBase = base;
        const baseLineWidth = resolveNodeBaseLineWidth(model, state.graphStyle);
        let r = base;
        let lineWidth = baseLineWidth;
        if (on) {
          const rawT = max > min ? (value - min) / (max - min) : 0.5;
          const t = Math.pow(clampNumber(rawT, 0, 1), 0.65);
          const minR = clampNumber(base * 0.7, 10, 32);
          const maxR = clampNumber(base * 2.4, minR + 2, 64);
          r = minR + (maxR - minR) * t;
          const widthScale = Math.sqrt(Math.max(r / Math.max(nextBase, 1), 0.01));
          lineWidth = clampNumber(
            baseLineWidth * (0.82 + widthScale * 0.36),
            Math.max(0.8, baseLineWidth * 0.75),
            Math.max(baseLineWidth + 1.2, baseLineWidth * 1.6)
          );
        } else {
          r = nextBase ?? base;
          lineWidth = baseLineWidth;
        }
        const target = map.get(id);
        target.r = r;
        target.lineWidth = lineWidth;
        if (target.__baseR == null) target.__baseR = nextBase;
        if (target.__baseLineWidth == null) target.__baseLineWidth = baseLineWidth;
        if (on) {
          target.__analysisScaledR = r;
          target.__analysisScaledLineWidth = lineWidth;
        } else {
          delete target.__analysisScaledR;
          delete target.__analysisScaledLineWidth;
        }
      });
    }
  }

  function buildEdgeDetailText(model) {
    const count = Number(model?.count) || 0;
    const parts = [];
    if (count > 0) parts.push(`${count}次`);
    const range = fmtShortRange(model?.first_time, model?.last_time);
    if (range) parts.push(range);
    return parts.join(" · ");
  }

  function applyEdgeDetailLabels(on) {
    const graph = ensureGraph();
    if (!graph) return;
    if (on && state.lod.edgeLabelsHidden) return;
    const edges = graph.getEdges?.() || [];
    if (!edges.length) return;
    graph.setAutoPaint(false);
    try {
      edges.forEach((edge) => {
        const model = edge.getModel?.() || {};
        if (model.__baseLabel == null) {
          model.__baseLabel = model.label ?? "";
          model.__baseLabelTop = model.labelTop ?? "";
          model.__baseLabelBottom = model.labelBottom ?? "";
        }
        const baseLabel = model.__baseLabel ?? model.label ?? "";
        const baseTop = model.__baseLabelTop ?? model.labelTop ?? "";
        const baseBot = model.__baseLabelBottom ?? model.labelBottom ?? "";
        if (!on) {
          graph.updateItem(edge, { label: baseLabel, labelTop: baseTop, labelBottom: baseBot, detailLabel: false });
          return;
        }
        const detail = buildEdgeDetailText(model);
        let nextLabel = baseLabel;
        let detailFlag = false;
        if (detail) {
          if (model.mode === "double") {
            nextLabel = detail;
            detailFlag = true;
          } else {
            nextLabel = baseLabel ? `${baseLabel} · ${detail}` : detail;
          }
        }
        graph.updateItem(edge, { label: nextLabel, detailLabel: detailFlag });
      });
    } finally {
      graph.setAutoPaint(true);
      graph.paint();
    }
    const data = state.graph.data?.edges;
    if (Array.isArray(data)) {
      const map = new Map(data.map((e) => [e.id, e]));
      edges.forEach((edge) => {
        const model = edge.getModel?.() || {};
        const id = model.id;
        if (!id || !map.has(id)) return;
        const target = map.get(id);
        if (target.__baseLabel == null) target.__baseLabel = model.__baseLabel ?? model.label ?? "";
        if (target.__baseLabelTop == null) target.__baseLabelTop = model.__baseLabelTop ?? model.labelTop ?? "";
        if (target.__baseLabelBottom == null) target.__baseLabelBottom = model.__baseLabelBottom ?? model.labelBottom ?? "";
        target.label = model.label ?? target.label;
        target.labelTop = model.labelTop ?? target.labelTop;
        target.labelBottom = model.labelBottom ?? target.labelBottom;
        target.detailLabel = !!model.detailLabel;
      });
    }
  }

  function syncAnalysisButtons() {
    const { encodeWidth, encodeColor, nodeScale } = state.analysis || {};
    const hasAmountFilter = Number.isFinite(state.analysis?.filterMin) || Number.isFinite(state.analysis?.filterMax);
    const collapseChildren = !!state.analysis?.collapseChildren;
    const filterBtn = $("#btnAnalysisFilter");
    if (filterBtn) {
      const popOpen = activePopoverAnchor === filterBtn;
      filterBtn.classList.toggle("active", hasAmountFilter);
      filterBtn.classList.toggle("popover-open", popOpen);
      filterBtn.setAttribute("aria-expanded", popOpen ? "true" : "false");
    }
    $("#btnAnalysisCollapse")?.classList.toggle("active", collapseChildren);
    $("#btnEncodeWidth")?.classList.toggle("active", !!encodeWidth);
    $("#btnEncodeColor")?.classList.toggle("active", !!encodeColor);
    $("#btnNodeScale")?.classList.toggle("active", !!nodeScale);
    scheduleShellStateSync("analysis-buttons");
  }

  function syncEdgeDetailButton() {
    $("#btnEdgeDetail")?.classList.toggle("active", !!state.edgeDetail);
    scheduleShellStateSync("edge-detail");
  }

  const TOOLBAR_BUTTON_TITLES = {
    btnFontFamily: "选择字体",
    btnFontSize: "选择字号",
    btnFontInc: "增大字号",
    btnFontDec: "减小字号",
    btnBold: "切换加粗",
    btnItalic: "切换斜体",
    btnUnderline: "切换下划线",
    btnShadow: "切换文字阴影",
    btnTextColor: "设置文字颜色",
    btnOutlineColor: "设置轮廓颜色",
    btnLineStyle: "设置线型",
    btnLineWidth: "设置线宽",
    btnLineArrow: "设置线条方向",
    btnNodeShape: "设置节点形状",
    btnNodeFill: "设置节点填充",
    btnIconSize: "设置节点大小",
    btnNodeNew: "新建节点",
    btnIconLibrary: "选择节点样式",
    btnNodeLink: "连接选中节点",
    btnLayoutCompact: "切换关联图",
    btnLayoutNetwork: "切换网络图",
    btnLayoutHierarchy: "切换层级图",
    btnLayoutFlow: "切换流向图",
    btnAnalysisFilter: "按金额筛选",
    btnAnalysisCollapse: "收缩弱关联",
    btnGroup1: "加入组合 1",
    btnGroup2: "加入组合 2",
    btnGroup3: "加入组合 3",
    btnGroupOps: "组合运算",
    btnEncodeWidth: "金额映射粗细",
    btnEncodeColor: "金额映射颜色",
    btnNodeScale: "金额映射节点",
    btnMergeNodes: "合并同名节点",
    btnFit: "适配当前视图",
    btnExport: "导出图谱",
    btnEdgeDetail: "显示连线详情",
    btnRedo: "恢复初始样式",
    btnUndo: "撤销上一步",
    btnAddView: "新建视图",
  };

  function syncToolbarButtonTitles() {
    Object.entries(TOOLBAR_BUTTON_TITLES).forEach(([id, title]) => {
      $("#" + id)?.setAttribute("title", title);
    });
  }

  function applyAnalysisEncodings() {
    if (!state.analysis) return;
    applyEdgeWidthEncoding(!!state.analysis.encodeWidth);
    applyEdgeColorEncoding(!!state.analysis.encodeColor);
    applyNodeScaleEncoding(!!state.analysis.nodeScale);
    void reapplyGraphTierVisualState(state.graph?.instance || null, { preserveEdgeLabels: true });
    syncAnalysisButtons();
  }

  function hasAnalysisAmountFilter() {
    return Number.isFinite(state.analysis?.filterMin) || Number.isFinite(state.analysis?.filterMax);
  }

  function parseAnalysisAmountInput(value) {
    const raw = String(value ?? "").trim();
    if (!raw) return null;
    const cleaned = raw.replace(/[,\s￥元]/g, "");
    if (!cleaned || cleaned === "-" || cleaned === "." || cleaned === "-.") return null;
    const num = Number(cleaned);
    if (!Number.isFinite(num)) return null;
    return Math.max(0, num);
  }

  function captureCurrentNodePositions() {
    const map = collectFiniteNodePositions(state.graph?.data?.nodes || []);
    const graph = state.graph?.instance || null;
    if (!graph) return map;
    const runtime = captureNodePositions(graph);
    runtime.forEach((pos, id) => {
      if (!pos || !Number.isFinite(pos.x) || !Number.isFinite(pos.y)) return;
      map.set(id, { x: pos.x, y: pos.y });
    });
    return map;
  }

  function snapshotAnalysisSourceData(options = {}) {
    const preferStateData = !!options?.preferStateData;
    const snapshot = cloneGraphData(state.graph.data);
    // Persist the runtime coordinates so filter/collapse can restore exact placement later.
    if (!preferStateData) {
      applyNodePositions(snapshot.nodes, captureCurrentNodePositions(), { overwrite: true });
    }
    return snapshot;
  }

  function ensureAnalysisSourceData() {
    if (!state.analysisSourceData) {
      state.analysisSourceData = snapshotAnalysisSourceData();
    }
    return cloneGraphData(state.analysisSourceData);
  }

  function syncAnalysisSourceDataFromCurrent({ force = false, preferStateData = false } = {}) {
    const hasFilter = hasAnalysisAmountFilter();
    const collapsed = !!state.analysis?.collapseChildren;
    if (!force && (hasFilter || collapsed)) return;
    state.analysisSourceData = snapshotAnalysisSourceData({ preferStateData });
  }

  function resetAnalysisFilterTools({ clearRange = true, clearSource = true, closePopover = false } = {}) {
    if (!state.analysis) state.analysis = createAnalysisState();
    if (clearRange) {
      state.analysis.filterMin = null;
      state.analysis.filterMax = null;
    }
    state.analysis.collapseChildren = false;
    state.analysisCollapsedChildMap = {};
    state.analysisCollapsedCoreIds = [];
    if (clearSource) state.analysisSourceData = null;
    if (closePopover && activePopoverAnchor?.id === "btnAnalysisFilter") closeRibbonPopover();
    syncAnalysisButtons();
  }

  function requireAnalysisGraphDataResult(result) {
    const errorText = String(result?.error || "").trim();
    if (
      !result ||
      typeof result !== "object" ||
      !Array.isArray(result.nodes) ||
      !Array.isArray(result.edges) ||
      !Array.isArray(result.coreIds) ||
      !result.childMap ||
      typeof result.childMap !== "object" ||
      Array.isArray(result.childMap)
    ) {
      throw new Error(errorText || "分析图谱计算结果无效");
    }
    return {
      nodes: result.nodes,
      edges: result.edges,
      coreIds: result.coreIds,
      childMap: result.childMap,
    };
  }

  function reportAnalysisGraphDataError(error, fallback = "分析图谱计算失败") {
    const message = String(error?.message || error || "").trim() || fallback;
    try {
      log("WARN", "analysis graph data projection failed", { message });
    } catch (_error) {}
    toast(message, "danger");
  }

  async function buildAnalysisGraphData({ collapseChildren = !!state.analysis?.collapseChildren } = {}) {
    const source = ensureAnalysisSourceData();
    const result = await projectAnalysisGraphData({
      source,
      collapseChildren,
      filterMin: state.analysis?.filterMin,
      filterMax: state.analysis?.filterMax,
    });
    return requireAnalysisGraphDataResult(result);
  }

  function applyAnalysisViewData(nextData, { preserveViewport = true } = {}) {
    const graph = ensureGraph();
    if (!graph) return false;
    const viewport = preserveViewport ? captureGraphViewport() : null;
    const currentPositions = captureCurrentNodePositions();
    const nodes = Array.isArray(nextData?.nodes) ? nextData.nodes.map((n) => ({ ...n })) : [];
    const edges = Array.isArray(nextData?.edges) ? nextData.edges.map((e) => ({ ...e })) : [];
    // Keep already visible nodes pinned, only add/remove visibility for analysis operations.
    applyNodePositions(nodes, currentPositions, { overwrite: true });
    const nextGraph = replaceRuntimeGraphData({ nodes, edges }, { reason: "apply-analysis-view-data" });
    try {
      graph.changeData(nextGraph);
    } catch (e) {
      try {
        graph.data(nextGraph);
        graph.render();
      } catch (e2) {}
    }
    if (viewport) applyGraphViewport(viewport, graph);
    applyAnalysisEncodings();
    if (state.edgeDetail) applyEdgeDetailLabels(true);
    syncEdgeDetailButton();
    updateGraphStats();
    updateMinimap();
    return true;
  }

  async function applyAnalysisFiltersToCanvas({ preserveViewport = true, manageLoading = true } = {}) {
    if (manageLoading) setGraphLoading(true, "正在更新分析图谱…");
    try {
      const collapsed = !!state.analysis?.collapseChildren;
      const hasFilter = hasAnalysisAmountFilter();
      if (!collapsed && !hasFilter) {
        if (state.analysisSourceData) {
          const restored = cloneGraphData(state.analysisSourceData);
          state.analysisSourceData = null;
          state.analysisCollapsedChildMap = {};
          state.analysisCollapsedCoreIds = [];
          applyAnalysisViewData(restored, { preserveViewport });
          scheduleGraphPatch({
            scope: "local-update",
            reason: "analysis-filters-reset",
          });
        }
        syncAnalysisButtons();
        return;
      }
      let derived;
      try {
        derived = await buildAnalysisGraphData({ collapseChildren: collapsed });
      } catch (error) {
        reportAnalysisGraphDataError(error);
        syncAnalysisButtons();
        return;
      }
      state.analysisCollapsedChildMap = derived.childMap || {};
      state.analysisCollapsedCoreIds = derived.coreIds || [];
      applyAnalysisViewData({ nodes: derived.nodes, edges: derived.edges }, { preserveViewport });
      scheduleGraphPatch({
        scope: "local-update",
        reason: collapsed ? "analysis-collapse-children" : "analysis-amount-filter",
      });
      syncAnalysisButtons();
    } finally {
      if (manageLoading) setGraphLoading(false);
    }
  }

  async function collapseAnalysisChildren({ animate = true } = {}) {
    const graph = ensureGraph();
    if (!graph || !state.graph.data?.nodes?.length) {
      toast("当前无图谱可操作", "warn");
      return;
    }
    setGraphLoading(true, "正在收缩分析图谱…");
    let derived;
    try {
      derived = await buildAnalysisGraphData({ collapseChildren: false });
    } catch (error) {
      reportAnalysisGraphDataError(error);
      setGraphLoading(false);
      return;
    }
    const childMap = derived.childMap || {};
    const childIds = Object.keys(childMap);
    if (!childIds.length) {
      toast("未识别到可收缩的子节点", "warn");
      setGraphLoading(false);
      return;
    }
    state.analysisCollapsedChildMap = childMap;
    state.analysisCollapsedCoreIds = derived.coreIds || [];
    const applyCollapsed = () => {
      state.analysis.collapseChildren = true;
      void applyAnalysisFiltersToCanvas({ preserveViewport: true, manageLoading: false }).finally(() => {
        setGraphLoading(false);
      });
    };
    if (!animate) {
      applyCollapsed();
      return;
    }
    applyAnalysisViewData({ nodes: derived.nodes, edges: derived.edges }, { preserveViewport: true });
    const positions = captureNodePositions(graph);
    const targets = new Map();
    childIds.forEach((childId) => {
      const coreId = childMap[childId];
      const corePos = positions.get(coreId);
      if (!corePos) return;
      targets.set(childId, { x: corePos.x, y: corePos.y });
    });
    if (!targets.size) {
      applyCollapsed();
      return;
    }
    state.analysis.collapseChildren = true;
    syncAnalysisButtons();
    animateToLayout(null, targets, {
      graph,
      duration: 420,
      finalize: false,
      fit: false,
      onComplete: () => {
        applyCollapsed();
      },
    });
  }

  async function expandAnalysisChildren({ animate = true } = {}) {
    const graph = ensureGraph();
    if (!graph) return;
    let derived;
    try {
      derived = await buildAnalysisGraphData({ collapseChildren: false });
    } catch (error) {
      reportAnalysisGraphDataError(error);
      return;
    }
    const childMap =
      state.analysisCollapsedChildMap && Object.keys(state.analysisCollapsedChildMap).length
        ? state.analysisCollapsedChildMap
        : derived.childMap || {};
    state.analysis.collapseChildren = false;
    syncAnalysisButtons();
    if (!animate) {
      applyAnalysisViewData({ nodes: derived.nodes, edges: derived.edges }, { preserveViewport: true });
      if (!hasAnalysisAmountFilter()) state.analysisSourceData = null;
      state.analysisCollapsedChildMap = {};
      state.analysisCollapsedCoreIds = [];
      scheduleGraphPatch({
        scope: "local-update",
        reason: "analysis-expand-children",
      });
      return;
    }
    const corePositions = captureNodePositions(graph);
    const targets = new Map();
    const startNodes = derived.nodes.map((node) => {
      const id = String(node?.id || "").trim();
      const tx = Number(node?.x);
      const ty = Number(node?.y);
      if (id && Number.isFinite(tx) && Number.isFinite(ty)) {
        targets.set(id, { x: tx, y: ty });
      }
      const currentPos = id ? corePositions.get(id) : null;
      if (currentPos) return { ...node, x: currentPos.x, y: currentPos.y };
      const coreId = childMap[id];
      const corePos = coreId ? corePositions.get(coreId) : null;
      if (!corePos) return { ...node };
      return { ...node, x: corePos.x, y: corePos.y };
    });
    const startEdges = derived.edges.map((edge) => ({ ...edge }));
    applyAnalysisViewData({ nodes: startNodes, edges: startEdges }, { preserveViewport: true });
    if (targets.size) {
      animateToLayout(null, targets, {
        graph,
        duration: 420,
        finalize: false,
        fit: false,
        onComplete: () => {
          syncGraphNodePositions(graph);
          updateCurrentViewSnapshot();
          scheduleGraphPatch({
            scope: "local-update",
            reason: "analysis-expand-children",
          });
        },
      });
    }
    if (!hasAnalysisAmountFilter()) state.analysisSourceData = null;
    state.analysisCollapsedChildMap = {};
    state.analysisCollapsedCoreIds = [];
    syncAnalysisButtons();
  }

  function toggleAnalysisCollapse() {
    if (state.analysis?.collapseChildren) {
      void expandAnalysisChildren({ animate: true });
      return;
    }
    void collapseAnalysisChildren({ animate: true });
  }

  function applyAnalysisAmountFilterState(minValue, maxValue) {
    state.analysis.filterMin = parseAnalysisAmountInput(minValue);
    state.analysis.filterMax = parseAnalysisAmountInput(maxValue);
    void applyAnalysisFiltersToCanvas({ preserveViewport: true });
    syncAnalysisButtons();
  }

  function clearAnalysisAmountFilterState() {
    state.analysis.filterMin = null;
    state.analysis.filterMax = null;
    void applyAnalysisFiltersToCanvas({ preserveViewport: true });
    syncAnalysisButtons();
  }

  function buildAmountFilterPopover() {
    const panel = document.createElement("div");
    panel.className = "amountFilterPanel";
    panel.dataset.popoverLabel = "金额筛选";

    const title = document.createElement("div");
    title.className = "amountFilterTitle";
    title.textContent = "金额设定";

    const grid = document.createElement("div");
    grid.className = "amountFilterGrid";

    const minWrap = document.createElement("div");
    minWrap.className = "amountFilterField";
    const minLabel = document.createElement("label");
    minLabel.id = "amountFilterMinLabel";
    minLabel.htmlFor = "amountFilterMin";
    minLabel.textContent = "最小金额";
    const minInput = document.createElement("input");
    minInput.id = "amountFilterMin";
    minInput.className = "amountFilterInput";
    minInput.type = "text";
    minInput.inputMode = "decimal";
    minInput.placeholder = "如 10000";
    minInput.value = Number.isFinite(state.analysis?.filterMin) ? String(state.analysis.filterMin) : "";
    minInput.setAttribute("aria-labelledby", "amountFilterMinLabel");
    minInput.dataset.popoverInitialFocus = "true";
    minWrap.appendChild(minLabel);
    minWrap.appendChild(minInput);

    const maxWrap = document.createElement("div");
    maxWrap.className = "amountFilterField";
    const maxLabel = document.createElement("label");
    maxLabel.id = "amountFilterMaxLabel";
    maxLabel.htmlFor = "amountFilterMax";
    maxLabel.textContent = "最大金额";
    const maxInput = document.createElement("input");
    maxInput.id = "amountFilterMax";
    maxInput.className = "amountFilterInput";
    maxInput.type = "text";
    maxInput.inputMode = "decimal";
    maxInput.placeholder = "如 100000";
    maxInput.value = Number.isFinite(state.analysis?.filterMax) ? String(state.analysis.filterMax) : "";
    maxInput.setAttribute("aria-labelledby", "amountFilterMaxLabel");
    maxWrap.appendChild(maxLabel);
    maxWrap.appendChild(maxInput);

    grid.appendChild(minWrap);
    grid.appendChild(maxWrap);

    const hint = document.createElement("div");
    hint.className = "amountFilterHint";
    hint.textContent = "只显示范围内金额。";

    const actions = document.createElement("div");
    actions.className = "amountFilterActions";
    const clearBtn = document.createElement("button");
    clearBtn.className = "amountFilterClear";
    clearBtn.type = "button";
    clearBtn.textContent = "清空范围";
    clearBtn.title = "清空金额范围";
    actions.appendChild(clearBtn);

    const applyFromInputs = () => {
      applyAnalysisAmountFilterState(minInput.value, maxInput.value);
    };
    minInput.addEventListener("input", applyFromInputs);
    maxInput.addEventListener("input", applyFromInputs);
    clearBtn.addEventListener("click", () => {
      minInput.value = "";
      maxInput.value = "";
      clearAnalysisAmountFilterState();
      minInput.focus();
    });

    panel.appendChild(title);
    panel.appendChild(grid);
    panel.appendChild(hint);
    panel.appendChild(actions);
    return panel;
  }

  function applyMergedGraphData(nodes, edges, { animate = true } = {}) {
    const graph = ensureGraph();
    if (!graph) return;
    closeEdgeInfoPopover();
    clearEdgeInfoTxnCache({ clearLane: true });
    const targets = new Map();
    nodes.forEach((n) => {
      let x = Number(n.x);
      let y = Number(n.y);
      if (!Number.isFinite(x)) x = 0;
      if (!Number.isFinite(y)) y = 0;
      n.x = x;
      n.y = y;
      targets.set(n.id, { x, y });
    });

    replaceRuntimeGraphData({ nodes, edges }, { reason: "apply-merged-graph-data" });
    if (!animate || nodes.length < 2) {
      graph.changeData(state.graph.data);
      updateMinimap();
      syncAnalysisSourceDataFromCurrent();
      scheduleGraphPatch({
        scope: "local-update",
        reason: "apply-merged-graph-data",
      });
      return;
    }

    const summary = summarizeLayout(nodes);
    const center = {
      x: summary.minX != null && summary.maxX != null ? (summary.minX + summary.maxX) / 2 : 0,
      y: summary.minY != null && summary.maxY != null ? (summary.minY + summary.maxY) / 2 : 0,
    };
    const jitterScale = 1.2;
    const collapsed = nodes.map((n, idx) => {
      const jitter = ((idx % 9) - 4) * jitterScale;
      return { ...n, x: center.x + jitter, y: center.y - jitter };
    });
    graph.changeData({ nodes: collapsed, edges });
    animateToLayout(null, targets, { graph, duration: 520 });
    syncAnalysisSourceDataFromCurrent();
    scheduleGraphPatch({
      scope: "local-update",
      reason: "apply-merged-graph-data",
    });
  }

  async function relayoutMergedGraph(nodes, edges, prevNodes = []) {
    const prevBounds = getGraphDataBounds(prevNodes || []);
    const prevCenter = prevBounds
      ? { x: (prevBounds.minX + prevBounds.maxX) / 2, y: (prevBounds.minY + prevBounds.maxY) / 2 }
      : null;
    const focusId = resolveFocusIdFromNodes(nodes, state.focusId, state.focusName);
    const layoutSemanticProjection = await prepareLayoutSemanticProjection(
      nodes,
      edges,
      state.layoutPreset || "compact",
      { reason: "merge-relayout" }
    );
    applyLayout(nodes, edges, state.layoutPreset || "compact", focusId, {
      semanticProjection: layoutSemanticProjection,
    });
    if (!prevCenter) return;
    const nextBounds = getGraphDataBounds(nodes);
    if (!nextBounds) return;
    const nextCenter = { x: (nextBounds.minX + nextBounds.maxX) / 2, y: (nextBounds.minY + nextBounds.maxY) / 2 };
    const dx = prevCenter.x - nextCenter.x;
    const dy = prevCenter.y - nextCenter.y;
    if (!Number.isFinite(dx) || !Number.isFinite(dy)) return;
    nodes.forEach((n) => {
      if (!Number.isFinite(n.x) || !Number.isFinite(n.y)) return;
      n.x += dx;
      n.y += dy;
    });
  }

  function summarizeDisplayIssues(nodes = []) {
    const issues = [];
    nodes.forEach((n) => {
      const name = String(n?.name || n?.title || "").trim();
      if (!name) return;
      const ids = collectNodeDisplayIds(n);
      const displayId = String(n?.displayId || n?.display_id || "").trim();
      if (ids.length > 1 && !displayId.includes("账户合集")) {
        issues.push({ name, id: n?.id || "", displayId, idsCount: ids.length });
      }
    });
    return issues.slice(0, 8);
  }

  function summarizeNameGroup(nodes = [], nameKey) {
    const list = nodes.filter(
      (n) => String(n?.name || n?.title || "").trim() === nameKey
    );
    return list.map((n) => ({
      id: n?.id || "",
      displayId: n?.displayId || n?.display_id || "",
      displayIdsCount: collectNodeDisplayIds(n).length,
      displayIdRaw: n?.displayIdRaw || n?.display_id_raw || "",
    }));
  }

  function requireSameNameMergeResult(result) {
    const errorText = String(result?.error || "").trim();
    if (
      !result ||
      typeof result !== "object" ||
      !Array.isArray(result.nodes) ||
      !Array.isArray(result.edges)
    ) {
      throw new Error(errorText || "同名节点合并结果无效");
    }
    return {
      mode: result.mode === "net" ? "net" : "gross",
      hasMergeTarget: !!result.hasMergeTarget,
      nodes: result.nodes,
      edges: result.edges,
    };
  }

  function reportSameNameMergeError(error, fallback = "同名节点合并失败") {
    const message = String(error?.message || error || "").trim() || fallback;
    try {
      log("WARN", "same-name graph merge failed", { message });
    } catch (_error) {}
    toast(message, "danger");
  }

  async function projectSameNameMergeData(data, mode = "gross") {
    const result = await mergeSameNameGraph({
      nodes: Array.isArray(data?.nodes) ? data.nodes : [],
      edges: Array.isArray(data?.edges) ? data.edges : [],
      mode: mode === "net" ? "net" : "gross",
    });
    return requireSameNameMergeResult(result);
  }

  async function mergeSameNameNodes() {
    const graph = ensureGraph();
    if (!graph) return;
    syncGraphNodePositions(graph);
    const data = state.graph.data;
    if (!data?.nodes?.length) {
      toast("暂无可合并节点", "warn");
      return;
    }
    try {
      const issues = summarizeDisplayIssues(data.nodes);
      log("INFO", "mergeSameNameNodes start", {
        nodes: data.nodes.length,
        edges: data.edges?.length || 0,
        issues,
        jyp: summarizeNameGroup(data.nodes, "蒋云泉"),
      });
    } catch (e) {}

    let mergeResult;
    try {
      mergeResult = await projectSameNameMergeData(data, "gross");
    } catch (error) {
      reportSameNameMergeError(error);
      return;
    }
    if (!mergeResult.hasMergeTarget) {
      toast("暂无可合并节点", "warn");
      return;
    }
    recordUndoSnapshot();
    const mergedNodes = mergeResult.nodes.map((node) => ({ ...node }));
    const mergedEdges = mergeResult.edges.map((edge) => ({ ...edge }));
    mergedNodes.forEach((n) => normalizeNodeDisplay(n, { allowGroup: !!n.mergedDisplay }));
    try {
      await relayoutMergedGraph(mergedNodes, mergedEdges, data.nodes || []);
    } catch (e) {
      try {
        log("WARN", "mergeSameNameNodes relayout failed", {
          message: String(e?.message || e),
        });
      } catch (_error) {}
      toast("合并布局失败", "danger");
      return;
    }
    applyMergedGraphData(mergedNodes, mergedEdges, { animate: true });
    state.signedEdgeDetail = false;
    applyAnalysisEncodings();
    if (state.edgeDetail) applyEdgeDetailLabels(true);
    updateGraphStats();
    updateCurrentViewSnapshot();
    syncEdgeDetailButton();
    try {
      const issues = summarizeDisplayIssues(mergedNodes);
      log("INFO", "mergeSameNameNodes done", {
        nodes: mergedNodes.length,
        edges: mergedEdges.length,
        issues,
        jyp: summarizeNameGroup(mergedNodes, "蒋云泉"),
      });
    } catch (e) {}
    toast("合并成功", "ok");
  }

  async function mergeSameNameNodesNet() {
    const graph = ensureGraph();
    if (!graph) return;
    syncGraphNodePositions(graph);
    let data = snapshotGraphData();
    if (!data?.nodes?.length) {
      const liveNodes = graph.getNodes?.() || [];
      const liveEdges = graph.getEdges?.() || [];
      if (liveNodes.length) {
        data = {
          nodes: liveNodes.map((n) => ({ ...(n.getModel?.() || {}) })),
          edges: liveEdges.map((e) => ({ ...(e.getModel?.() || {}) })),
        };
      }
    }
    if (!data?.nodes?.length) {
      toast("暂无可合并节点", "warn");
      return;
    }
    try {
      const issues = summarizeDisplayIssues(data.nodes);
      log("INFO", "mergeSameNameNodesNet start", {
        nodes: data.nodes.length,
        edges: data.edges?.length || 0,
        issues,
        jyp: summarizeNameGroup(data.nodes, "蒋云泉"),
      });
    } catch (e) {}

    let mergeResult;
    try {
      mergeResult = await projectSameNameMergeData(data, "net");
    } catch (error) {
      reportSameNameMergeError(error);
      return;
    }
    recordUndoSnapshot();
    const mergedNodes = mergeResult.nodes.map((node) => ({ ...node }));
    const mergedEdges = mergeResult.edges.map((edge) => ({ ...edge }));
    mergedNodes.forEach((n) => normalizeNodeDisplay(n, { allowGroup: !!n.mergedDisplay }));
    try {
      await relayoutMergedGraph(mergedNodes, mergedEdges, data.nodes || []);
    } catch (e) {
      try {
        log("WARN", "mergeSameNameNodesNet relayout failed", {
          message: String(e?.message || e),
        });
      } catch (_error) {}
      toast("合并布局失败", "danger");
      return;
    }
    applyMergedGraphData(mergedNodes, mergedEdges, { animate: true });
    state.signedEdgeDetail = true;
    applyAnalysisEncodings();
    if (state.edgeDetail) applyEdgeDetailLabels(true);
    updateGraphStats();
    updateCurrentViewSnapshot();
    syncEdgeDetailButton();
    try {
      const issues = summarizeDisplayIssues(mergedNodes);
      log("INFO", "mergeSameNameNodesNet done", {
        nodes: mergedNodes.length,
        edges: mergedEdges.length,
        issues,
        jyp: summarizeNameGroup(mergedNodes, "蒋云泉"),
      });
    } catch (e) {}
    toast("合并成功", "ok");
  }

  function assignGroupTag(tagId) {
    const graph = ensureGraph();
    if (!graph) return;
    const { nodes } = getSelectedGraphItems(graph);
    if (!nodes.length) {
      toast("请先选择节点", "warn");
      return;
    }
    recordUndoSnapshot();
    updateNodeGroupTags(nodes, (model) => {
      const tags = normalizeGroupTags(model.groupTags);
      if (!tags.includes(tagId)) tags.push(tagId);
      return tags;
    });
  }

  function applyGroupOperation(mode) {
    const graph = ensureGraph();
    if (!graph) return;
    const nodes = graph.getNodes?.() || [];
    const matched = nodes.filter((node) => {
      const tags = normalizeGroupTags(node.getModel?.()?.groupTags);
      if (!tags.length) return false;
      if (mode === "intersection") return tags.length >= 2;
      if (mode === "difference") return tags.length === 1;
      return false;
    });
    clearGraphSelection(graph);
    if (!matched.length) {
      toast("未找到符合的节点", "warn");
      return;
    }
    matched.forEach((node) => graph.setItemState(node, "selected", true));
    syncActiveSelection(graph, matched[matched.length - 1] || null);
    syncEdgeDirectionControl(graph);
  }

  function updateRibbonState(partial, { apply = true } = {}) {
    ensureRibbonStyle();
    const nextPartial = normalizeOutlinePartial(partial);
    state.ribbonStyle = { ...state.ribbonStyle, ...nextPartial };
    syncRibbonUI(state.ribbonStyle);
    scheduleShellStateSync("ribbon-style");
    if (apply) applyRibbonStyle(nextPartial);
  }

  function applyRibbonStyle(partial) {
    if (!partial) return;
    const graph = ensureGraph();
    const next = { ...partial };
    if (next.edgeArrow != null) {
      if (!canEditEdgeDirection(graph)) {
        delete next.edgeArrow;
      } else {
        next.edgeMode = next.edgeArrow === "both" ? "double" : "single";
      }
    }
    if (Object.keys(next).length) applyStyleToSelection(next);
  }

  function getSelectionState(graph) {
    return ensureSelectionStore().getSelectionState(graph);
  }

  function extractDomEvent(ev) {
    return (
      ev?.originalEvent ||
      ev?.event ||
      ev?.sourceEvent ||
      ev?.domEvent ||
      ev?.gEvent?.originalEvent ||
      ev
    );
  }

  function resolveClientPointFromEvent(graph, ev) {
    const e = extractDomEvent(ev);
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

  function isMultiSelectEvent(ev) {
    const e = extractDomEvent(ev);
    if (!e) return state.modifiers.ctrl || state.modifiers.meta;
    const meta = !!(e.metaKey || e.getModifierState?.("Meta") || state.modifiers.meta);
    const ctrl = !!(e.ctrlKey || e.getModifierState?.("Control") || state.modifiers.ctrl);
    return meta || ctrl;
  }

  function clearGraphSelection(graph) {
    ensureSelectionStore().clearGraphSelection(graph);
  }

  function selectAllGraphItems() {
    const graph = ensureGraph();
    if (!graph) return;
    clearCanvasSelection(graph, "select-all-reset");
    const nodes = graph.getNodes?.() || [];
    const edges = graph.getEdges?.() || [];
    nodes.forEach((n) => graph.setItemState(n, "selected", true));
    edges.forEach((e) => graph.setItemState(e, "selected", true));
    syncActiveSelection(graph, nodes[nodes.length - 1] || null);
    syncEdgeDirectionControl(graph);
  }

  function selectAdjacentGraphNodes(anchorId, { append = true, includeCurrentSelection = true } = {}) {
    const graph = ensureGraph();
    const targetId = String(anchorId || "").trim();
    if (!graph || !targetId) return { base: 0, neighbor: 0, selected: 0 };
    const item = graph.findById?.(targetId);
    if (!item) return { base: 0, neighbor: 0, selected: 0 };
    const data = state.graph?.data || { nodes: [], edges: [] };
    const nodeMap = new Map((data.nodes || []).map((node) => [String(node?.id || ""), node]));
    const edgeRows = Array.isArray(data.edges) ? data.edges : [];
    const neighbors = new Set();
    edgeRows.forEach((edge) => {
      const s = String(edge?.source || "").trim();
      const t = String(edge?.target || "").trim();
      if (!s || !t) return;
      if (s === targetId && t !== targetId) neighbors.add(t);
      if (t === targetId && s !== targetId) neighbors.add(s);
    });
    const selected = new Set();
    if (append && includeCurrentSelection) {
      const current = graph.findAllByState?.("node", "selected") || [];
      current.forEach((node) => {
        const id = String(node?.getModel?.()?.id || "").trim();
        if (id) selected.add(id);
      });
    }
    selected.add(targetId);
    neighbors.forEach((id) => selected.add(id));
    if (!append) clearGraphSelection(graph);
    const nodeItems = graph.getNodes?.() || [];
    nodeItems.forEach((nodeItem) => {
      const id = String(nodeItem?.getModel?.()?.id || "").trim();
      const role = String(nodeMap.get(id)?.role || "").toLowerCase();
      const shouldSelect = selected.has(id);
      if (shouldSelect) {
        graph.setItemState(nodeItem, "selected", true);
      } else if (!append) {
        graph.setItemState(nodeItem, "selected", false);
      }
      if (shouldSelect && (role === "core" || role === "adjacent")) {
        graph.setItemState(nodeItem, "active", true);
      }
    });
    syncActiveSelection(graph, graph.findById?.(targetId) || null);
    syncEdgeDirectionControl(graph);
    return { base: 1, neighbor: neighbors.size, selected: selected.size };
  }

  function applyColorAlpha(color, alpha = 1) {
    const a = clampNumber(Number(alpha), 0, 1);
    const raw = String(color || "").trim();
    if (!raw) return `rgba(2,6,23,${a.toFixed(3)})`;
    const rgba = raw.match(/^rgba?\(([^)]+)\)$/i);
    if (rgba) {
      const parts = rgba[1]
        .split(",")
        .map((part) => part.trim())
        .filter(Boolean);
      const r = Number(parts[0]);
      const g = Number(parts[1]);
      const b = Number(parts[2]);
      if (Number.isFinite(r) && Number.isFinite(g) && Number.isFinite(b)) {
        return `rgba(${Math.round(r)}, ${Math.round(g)}, ${Math.round(b)}, ${a.toFixed(3)})`;
      }
    }
    const hex = raw.match(/^#([0-9a-f]{3}|[0-9a-f]{6})$/i);
    if (hex) {
      let value = hex[1];
      if (value.length === 3) value = value.split("").map((ch) => ch + ch).join("");
      const r = Number.parseInt(value.slice(0, 2), 16);
      const g = Number.parseInt(value.slice(2, 4), 16);
      const b = Number.parseInt(value.slice(4, 6), 16);
      if (Number.isFinite(r) && Number.isFinite(g) && Number.isFinite(b)) {
        return `rgba(${r}, ${g}, ${b}, ${a.toFixed(3)})`;
      }
    }
    return a >= 0.999 ? raw : `rgba(2,6,23,${a.toFixed(3)})`;
  }

  function deleteSelectedGraphItems() {
    const graph = ensureGraph();
    if (!graph) return false;
    const { nodes, edges } = getSelectedGraphItems(graph);
    if (!nodes.length && !edges.length) return false;
    recordUndoSnapshot();
    cancelGraphTransientTasks("delete-selected");

    const nodeIds = new Set(
      nodes
        .map((n) => n.getID?.() || n.getModel?.()?.id || "")
        .map((id) => String(id || "").trim())
        .filter(Boolean)
    );
    const edgeIds = new Set(
      edges
        .map((e) => e.getID?.() || e.getModel?.()?.id || "")
        .map((id) => String(id || "").trim())
        .filter(Boolean)
    );
    const edgeKeys = new Set(
      edges
        .map((e) => graphEdgeIdentityKey(e?.getModel?.() || {}))
        .map((key) => String(key || "").trim())
        .filter(Boolean)
    );
    const selectedNodeSample = Array.from(nodeIds).slice(0, 8);
    const selectedEdgeSample = Array.from(edgeIds).slice(0, 8);
    const deleteProbeId = state.lastDeleteProbeId || "";
    if (isDeleteProbeVerbose()) {
      logGhostProbe(
        "delete-selected-start",
        {
          deleteProbeId,
          selected: { nodes: nodeIds.size, edges: edgeIds.size },
          selectedNodeSample,
          selectedEdgeSample,
        },
        { dedupKey: `delete-selected-start|${deleteProbeId}|${nodeIds.size}|${edgeIds.size}`, dedupMs: 160 }
      );
    }
    const runDeleteMutation = () => {
      const data = state.graph.data || { nodes: [], edges: [] };
      const prevNodeCount = Array.isArray(data.nodes) ? data.nodes.length : 0;
      const prevEdgeCount = Array.isArray(data.edges) ? data.edges.length : 0;
      const nextNodes = Array.isArray(data.nodes)
        ? data.nodes.filter((n) => !nodeIds.has(String(n?.id || "").trim()))
        : [];
      const nextEdges = Array.isArray(data.edges)
        ? data.edges.filter(
            (e) =>
              !edgeIds.has(String(e?.id || "").trim()) &&
              !edgeKeys.has(graphEdgeIdentityKey(e)) &&
              !nodeIds.has(String(e?.source || "").trim()) &&
              !nodeIds.has(String(e?.target || "").trim())
          )
        : [];

      replaceRuntimeGraphData({ nodes: nextNodes, edges: nextEdges }, { reason: "delete-selected-state" });
      if (isDeleteProbeVerbose()) {
        logGhostProbe("delete-selected-state-updated", {
          deleteProbeId,
          removed: { nodes: Math.max(0, prevNodeCount - nextNodes.length), edges: Math.max(0, prevEdgeCount - nextEdges.length) },
          next: { nodes: nextNodes.length, edges: nextEdges.length },
        });
      }
      let redrawOk = false;
      try {
        redrawOk = forceRedrawGraphFromState("delete-selected", {
          restoreViewport: true,
          hideDuringRedraw: false,
        });
      } catch (e) {}
      if (!redrawOk) {
        try {
          graph.data(cloneGraphData(state.graph.data));
          graph.render();
          graph.getEdges?.().forEach((edge) => graph.refreshItem?.(edge));
          graph.paint?.();
          ensureGraphDataVisible(graph, nextNodes, nextEdges, "delete-selected");
          updateMinimap();
        } catch (e) {}
      }
      if (!redrawOk || isDeleteProbeVerbose()) {
        logGhostProbe("delete-selected-redraw", { deleteProbeId, redrawOk });
      }
      const runtimeStableAfterRedraw = isGraphRuntimeStable(state.graph.instance || graph);
      const alwaysRecreate = shouldAlwaysRecreateAfterDelete();
      const recreateNeeded = alwaysRecreate || !redrawOk || !runtimeStableAfterRedraw;
      let recreateTriggered = false;
      let recreateOk = false;
      if (recreateNeeded) {
        recreateTriggered = true;
        try {
          recreateOk = recreateGraphInstanceFromState("delete-selected", {
            restoreViewport: true,
            hideDuringRebuild: false,
          });
        } catch (e) {}
      }
      if ((recreateTriggered && !recreateOk) || isDeleteProbeVerbose()) {
        logGhostProbe(
          "delete-selected-recreate",
          {
            deleteProbeId,
            alwaysRecreate,
            recreateNeeded,
            recreateTriggered,
            recreateOk,
            runtimeStableAfterRedraw,
          },
          recreateTriggered && !recreateOk ? { level: "WARN" } : undefined
        );
      }
      scheduleGraphConsistencyProbe("delete-selected");
      const visualGuardEnabled = shouldRunDeleteVisualGuard();
      if (visualGuardEnabled) {
        scheduleDeleteVisualGuard("delete-selected", DELETE_VISUAL_GUARD_DELAYS);
      } else {
        if (isDeleteProbeVerbose()) {
          logGhostProbe(
            "delete-selected-visual-guard-skip",
            { deleteProbeId, skip: "debug-disabled" },
            {
              dedupKey: `delete-selected-visual-guard-skip|${deleteProbeId}`,
              dedupMs: 300,
              onlyInDeleteWindow: true,
            }
          );
        }
      }
      if (isDeleteProbeVerbose()) {
        logGhostProbe("delete-selected-reconcile-scheduled", {
          deleteProbeId,
          mode: visualGuardEnabled
            ? alwaysRecreate
              ? "consistency-probe+visual-guard(debug)+always-recreate"
              : "consistency-probe+visual-guard(debug)+recreate-on-mismatch"
            : alwaysRecreate
              ? "consistency-probe+always-recreate"
              : "consistency-probe+recreate-on-mismatch",
        });
      }
      state.layoutTopologyDirty = true;
      terminateLayoutPrewarmTask("topology-dirty");
      layoutPlanCacheController.clearCaches();
      networkSectorPlacementCache.clear();
      clearGraphSelection(state.graph.instance || graph);
      closeDrawer();
      if (state.edgeDetail) applyEdgeDetailLabels(true);
      applyAnalysisEncodings();
      updateGraphStats();
      updateCurrentViewSnapshot();
      updateLayoutButtons();
      scheduleGraphPatch({
        scope: "local-update",
        reason: "delete-selected-graph-items",
      });
    };

    runDeleteMutation();
    return true;
  }

  function syncActiveSelection(graph, candidate) {
    ensureSelectionStore().syncActiveSelection(graph, candidate);
  }

  function setCreateNodeMode(on) {
    const graph = ensureGraph();
    if (!graph) return;
    state.createNodeMode = !!on;
    const btn = $("#btnNodeNew");
    if (btn) btn.classList.toggle("active", state.createNodeMode);
    scheduleShellStateSync("create-node-mode");
    if (!state.createNodeMode) {
      setGhostFloatVisible(false);
      removeGhostNode();
      clearGraphSelection(graph);
      return;
    }
    if (!state.ghostNodeId) state.ghostNodeId = `ghost_${Date.now().toString(36)}`;
    clearGraphSelection(graph);
    removeGhostNode();
    setGhostFloatVisible(true);
    syncGhostFloatStyle();
    if (state.lastPointer) {
      updateGhostFromClient(state.lastPointer.x, state.lastPointer.y);
    } else {
      const rect = graph.get("container")?.getBoundingClientRect?.();
      if (rect) updateGhostFromClient(rect.left + rect.width / 2, rect.top + rect.height / 2);
    }
  }

  function removeGhostNode() {
    const graph = ensureGraph();
    if (!graph || !state.ghostNodeId) return;
    const ghost = graph.findById?.(state.ghostNodeId);
    if (ghost) {
      try {
        graph.removeItem(ghost);
      } catch (e) {}
    }
  }

  function updateGhostPosition(point) {
    const graph = ensureGraph();
    if (!graph || !state.ghostNodeId) return;
    const ghost = graph.findById?.(state.ghostNodeId);
    if (!ghost) return;
    const model = ghost.getModel?.() || {};
    model.x = point.x;
    model.y = point.y;
    graph.refreshItem?.(ghost);
  }

  function syncGhostFloatStyle(style) {
    const node = $("#ghostFloatNode");
    if (!node) return;
    const s = style || state.ribbonStyle || state.graphStyle || {};
    const stroke = s.nodeColor || "#1f6feb";
    const fill = s.nodeFill || "rgba(31,111,235,0.12)";
    const lineWidth = clampNumber(s.nodeWidth ?? 2, 1, 6);
    const r = clampNumber(s.nodeSize ?? 18, 10, 40);
    const size = r * 2;
    node.style.width = `${size}px`;
    node.style.height = `${size}px`;
    node.style.borderColor = stroke;
    node.style.borderWidth = `${lineWidth}px`;
    node.style.background = fill;
  }

  function setGhostFloatVisible(on) {
    const wrap = $("#ghostFloat");
    if (!wrap) return;
    wrap.classList.toggle("visible", !!on);
    wrap.style.display = on ? "block" : "none";
  }

  function updateGhostFloatPosition(clientX, clientY) {
    const wrap = $("#ghostFloat");
    if (!wrap) return;
    wrap.style.transform = `translate3d(${clientX}px, ${clientY}px, 0)`;
  }

  function updateGhostFromClient(clientX, clientY) {
    const graph = ensureGraph();
    if (!graph) return;
    const container = graph.get("container");
    if (!container || typeof graph.getPointByClient !== "function") return;
    const rect = container.getBoundingClientRect();
    if (!rect || !rect.width || !rect.height) return;
    if (state.createNodeMode) {
      updateGhostFloatPosition(clientX, clientY);
    }
  }

  function createNodeAt(point) {
    const graph = ensureGraph();
    if (!graph) return;
    const style = state.ribbonStyle || state.graphStyle;
    const nodeIndex = (state.nodeCounter = (state.nodeCounter || 0) + 1);
    const nodeTitle = `新建${nodeIndex}`;
    const node = {
      id: createNodeId(),
      x: point.x,
      y: point.y,
      type: "hollow-entity",
      r: style.nodeSize || 18,
      stroke: style.nodeColor || "#1f6feb",
      fill: style.nodeFill || "rgba(255,255,255,0.05)",
      lineWidth: style.nodeWidth || 2,
      textColor: style.textColor || "rgba(2,6,23,.86)",
      subTextColor: style.textColor || "rgba(2,6,23,.72)",
      fontFamily: style.fontFamily || FONT_LIST[0]?.value || "PingFang SC",
      fontSize: style.fontSize || 13,
      fontBold: asBool(style.fontBold, false),
      fontItalic: asBool(style.fontItalic, false),
      fontUnderline: asBool(style.fontUnderline, false),
      fontShadow: asBool(style.fontShadow, false),
      nodeShadow: asBool(style.nodeShadow, false),
      iconSymbol: style.iconSymbol || "",
      iconSize: style.iconSize || 16,
      nodeShape: style.nodeShape || "circle",
      title: nodeTitle,
      displayIdRaw: nodeTitle,
      name: "",
      userCreated: true,
    };
    try {
      graph.addItem("node", node);
      state.graph.data.nodes.push({ ...node });
      updateGraphStats();
      updateCurrentViewSnapshot();
      scheduleGraphPatch({
        scope: "local-update",
        reason: "create-user-node",
      });
      clearGraphSelection(graph);
      const item = graph.findById?.(node.id);
      if (item) {
        graph.setItemState(item, "selected", true);
        syncActiveSelection(graph, item);
      }
    } catch (e) {}
  }

  function addUserCreatedEdge(graph, sourceId, targetId) {
    const source = String(sourceId || "").trim();
    const target = String(targetId || "").trim();
    if (!graph || !source || !target || source === target) return null;
    const exists = state.graph.data.edges.some(
      (edge) => (edge.source === source && edge.target === target) || (edge.source === target && edge.target === source)
    );
    if (exists) return null;
    const style = state.ribbonStyle || state.graphStyle;
    const edge = {
      id: `edge_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 6)}`,
      source,
      target,
      type: "center-link",
      mode: "single",
      label: "",
      stroke: style.edgeColor || "rgba(2,6,23,.68)",
      lineWidth: style.edgeWidth || 2,
      edgeDash: style.edgeDash || "solid",
      edgeArrow: "none",
      gap: 20,
      holePad: 2,
      outPad: 0.8,
      showArrow: false,
      textColor: style.textColor || "rgba(2,6,23,.82)",
      fontFamily: style.fontFamily || FONT_LIST[0]?.value || "PingFang SC",
      fontSize: style.fontSize || 13,
      fontBold: asBool(style.fontBold, false),
      fontItalic: asBool(style.fontItalic, false),
      fontUnderline: asBool(style.fontUnderline, false),
      fontShadow: asBool(style.fontShadow, false),
      userCreated: true,
    };
    try {
      const item = graph.addItem("edge", edge);
      if (item && graph.updateItem) graph.updateItem(item, edge);
      state.graph.data.edges.push({ ...edge });
      return { edge, item };
    } catch (e) {
      return null;
    }
  }

  function createLinksFromSelection() {
    const graph = ensureGraph();
    if (!graph) return;
    const { nodes } = getSelectedGraphItems(graph);
    if (!nodes.length || nodes.length < 2) {
      toast("请至少选择两个节点", "warn");
      return;
    }
    const created = [];
    for (let i = 0; i < nodes.length - 1; i += 1) {
      const s = nodes[i].getModel?.()?.id;
      const t = nodes[i + 1].getModel?.()?.id;
      const createdEntry = addUserCreatedEdge(graph, s, t);
      if (createdEntry?.edge) created.push(createdEntry.edge);
    }
    if (created.length) {
      try {
        graph.refreshPositions?.();
        created.forEach((edge) => {
          const item = graph.findById?.(edge.id);
          if (item) graph.refreshItem?.(item);
        });
        graph.getEdges?.().forEach((edge) => graph.refreshItem?.(edge));
        graph.paint();
      } catch (e) {}
      updateGraphStats();
      updateCurrentViewSnapshot();
      scheduleGraphPatch({
        scope: "local-update",
        reason: "create-user-edge",
      });
      clearGraphSelection(graph);
      created.forEach((edge) => {
        const item = graph.findById?.(edge.id);
        if (item) graph.setItemState(item, "selected", true);
      });
      syncEdgeDirectionControl(graph);
      toast(`已创建 ${created.length} 条连接`, "ok");
    }
  }

  function updateModifierState(e) {
    if (!e) return;
    const key = e.key;
    const isMetaKey = key === "Meta" || key === "OS";
    const isCtrlKey = key === "Control";
    state.modifiers.meta = !!(e.metaKey || (isMetaKey && e.type === "keydown"));
    state.modifiers.ctrl = !!(e.ctrlKey || (isCtrlKey && e.type === "keydown"));
  }

  function startPulseLoop() {
    if (state.pulseAnimating) return;
    const graph = ensureGraph();
    if (!graph) return;
    state.pulseAnimating = true;
    state.pulseStart = performance.now();
    const period = 1600;
    const pulseAmp = 1.6;
    const tick = (now) => {
      if (!state.pulseAnimating) return;
      const t = ((now - state.pulseStart) % period) / period;
      const s = Math.sin(t * Math.PI * 2);
      let hasActive = false;
      graph.setAutoPaint(false);
      graph.getNodes().forEach((node) => {
        if (!node.hasState?.("active") || !node.hasState?.("selected")) return;
        const model = node.getModel() || {};
        const group = node.getContainer();
        const halo = group?.find?.((e) => e.get("name") === "halo");
        if (!halo) return;
        hasActive = true;
        const baseR = (model.r ?? 18) + 3;
        halo.attr({
          r: baseR + pulseAmp * s,
          opacity: 0.6 + 0.15 * s,
          shadowBlur: 9 + 2 * ((s + 1) / 2),
        });
      });
      graph.setAutoPaint(true);
      if (hasActive) graph.paint();
      state.pulseRaf = requestAnimationFrame(tick);
    };
    state.pulseRaf = requestAnimationFrame(tick);
  }

  function stopPulseLoop() {
    state.pulseAnimating = false;
    if (state.pulseRaf) {
      cancelAnimationFrame(state.pulseRaf);
      state.pulseRaf = 0;
    }
  }

  function ensureGraph() {
    if (state.graph.inited && state.graph.instance) return state.graph.instance;
    const container = $("#graphContainer");
    if (!container) return null;

    const perfMode = isPerfMode();
    const modes = buildGraphModes(perfMode);
    const rendererPref = getGraphRendererPreference();
    let graph = null;
    try {
      const graphEngine = getGraphEngine();
      if (!graphEngine || typeof graphEngine.Graph !== "function") {
        throw new Error("Analytix graph engine unavailable");
      }
      graph = new graphEngine.Graph({
        container,
        width: container.clientWidth || 800,
        height: container.clientHeight || 600,
        minZoom: 0.05,
        maxZoom: 16,
        modes,
        defaultNode: { type: "hollow-entity" },
        defaultEdge: { type: "center-link" },
        renderer: rendererPref.renderer,
        scaleWithView: true,
        highDpi: true,
        highDpiMax: 4,
        textAtlasScale: 4,
        gpuTextShadow: false,
        webglWarn: !!logSwitch.webglWarn,
      });
    } catch (e) {
      try {
        log("ERROR", "webgl renderer init failed", {
          message: String(e?.message || e),
          webgl: rendererPref.caps?.webgl,
          webgl2: rendererPref.caps?.webgl2,
        });
      } catch (e2) {}
      try {
        toast("WebGL 初始化失败，无法创建图谱", "danger");
      } catch (e2) {}
      return null;
    }
    try {
      log("INFO", "renderer selected", {
        renderer: graph?.get?.("renderer") || rendererPref.renderer,
        preferWebGL: rendererPref.preferWebGL,
        webgl: rendererPref.caps?.webgl,
        webgl2: rendererPref.caps?.webgl2,
      });
    } catch (e) {}
    logGpuProbe(graph);
    logGraphDebug(graph, "renderer debug");
    initLayoutWorker();

    try {
      initMinimap(graph);
    } catch (e) {}

    const usingWebglEngine = String(getGraphEngine()?.version || "").startsWith("AnalytixWebGL");
    ensureCanvasDragController(graph, usingWebglEngine);
    graph.on("node:drag", (ev) => {
      state.canvasDragController?.handleNodeDrag?.(ev);
    });
    graph.on("node:dragstart", (ev) => {
      state.canvasDragController?.handleNodeDragStart?.(ev);
    });
    graph.on("node:dragend", (ev) => {
      state.canvasDragController?.handleNodeDragEnd?.(ev);
    });
    graph.on("canvas:dragstart", () => {
      state.canvasDragController?.handleCanvasDragStart?.();
    });
    graph.on("canvas:dragend", () => {
      state.canvasDragController?.handleCanvasDragEnd?.();
    });
    graph.on("canvas:click", (ev) => {
      closeDrawer();
      hideNodeHoverTip();
      noteCanvasHit("canvas-click", null, ev, "canvas:click");
      if (state.createNodeMode) {
        try {
          const e = extractDomEvent(ev);
          if (e && typeof graph.getPointByClient === "function") {
            const point = graph.getPointByClient(e.clientX, e.clientY);
            createNodeAt(point);
          } else {
            const x = ev?.canvasX ?? ev?.x ?? 0;
            const y = ev?.canvasY ?? ev?.y ?? 0;
            createNodeAt({ x, y });
          }
          setCreateNodeMode(false);
          setGhostFloatVisible(false);
        } catch (e) {}
        return;
      }
      try {
        clearCanvasSelection(graph, "canvas:click");
      } catch (e) {}
    });

    graph.on("node:click", (ev) => {
      closeDrawer();
      const item = ev.item;
      noteCanvasHit("node-click", item, ev, "node:click");
      const model = item?.getModel?.() || {};
      const isMulti = isMultiSelectEvent(ev);
      if (state.createNodeMode && model.ghost) {
        const e = extractDomEvent(ev);
        if (e && typeof graph.getPointByClient === "function") {
          const point = graph.getPointByClient(e.clientX, e.clientY);
          createNodeAt(point);
        } else {
          const x = ev?.canvasX ?? model.x ?? 0;
          const y = ev?.canvasY ?? model.y ?? 0;
          createNodeAt({ x, y });
        }
        setCreateNodeMode(false);
        setGhostFloatVisible(false);
        return;
      }
      if (!item) return;
      const wasSelected = item.hasState?.("selected");
      const nextSelected = toggleCanvasItemSelection(item, {
        targetGraph: graph,
        multi: isMulti,
        reason: "node:click",
      });
      if (isGhostProbeWindow()) {
        if (isDeleteProbeVerbose()) {
          logGhostProbe(
            "node-click-near-delete",
            {
              nodeId: String(model?.id || "").trim(),
              isMulti,
              wasSelected: !!wasSelected,
              nextSelected: !!nextSelected,
            },
            {
              dedupKey: `node-click-near-delete|${state.lastDeleteProbeId || ""}|${String(model?.id || "").trim()}|${nextSelected ? 1 : 0}`,
              dedupMs: 120,
              onlyInDeleteWindow: true,
            }
          );
        }
        scheduleGraphConsistencyCheck("node-click-near-delete");
      }
    });
    graph.on("node:contextmenu", (ev) => {
      const item = ev.item;
      if (!item) return;
      const model = item.getModel?.() || {};
      const items = buildNodeContextMenuItems(model);
      if (!items.length) {
        closeContextMenu();
        return;
      }
      const e = extractDomEvent(ev);
      if (!e) return;
      e.preventDefault();
      e.stopPropagation?.();
      try {
        const graphPoint =
          typeof graph.getPointByClient === "function"
            ? graph.getPointByClient(e.clientX, e.clientY)
            : { x: model.x, y: model.y };
        state.lastContextNode = {
          id: model.id,
          clientX: e.clientX,
          clientY: e.clientY,
          graphX: Number.isFinite(graphPoint?.x) ? graphPoint.x : null,
          graphY: Number.isFinite(graphPoint?.y) ? graphPoint.y : null,
          modelX: Number.isFinite(model.x) ? model.x : null,
          modelY: Number.isFinite(model.y) ? model.y : null,
          ts: Date.now(),
        };
        log("INFO", "node contextmenu", {
          id: model.id || "",
          title: model.title || "",
          client: { x: e.clientX, y: e.clientY },
          graph: {
            x: Number.isFinite(graphPoint?.x) ? Math.round(graphPoint.x * 100) / 100 : null,
            y: Number.isFinite(graphPoint?.y) ? Math.round(graphPoint.y * 100) / 100 : null,
          },
          model: {
            x: Number.isFinite(model.x) ? Math.round(model.x * 100) / 100 : null,
            y: Number.isFinite(model.y) ? Math.round(model.y * 100) / 100 : null,
          },
        });
      } catch (e) {}
      openContextMenu(e.clientX, e.clientY, items);
    });
    graph.on("node:dblclick", (ev) => {
      const item = ev.item;
      if (!item) return;
      const model = item.getModel?.() || {};
      const projectionMode = String(state.graphProjection?.mode || "").trim().toLowerCase();
      if (
        projectionMode === "skeleton" &&
        (isClusterProjectionNode(model) || String(model?.projection_cluster_id || model?.cluster_id || "").trim())
      ) {
        closeContextMenu();
        closeDrillPopover();
        closeDrawer();
        void expandProjectionNode(model, { reason: "node:dblclick" });
        return;
      }
      const e = extractDomEvent(ev);
      if (!e) return;
      e.preventDefault();
      e.stopPropagation?.();
      closeContextMenu();
      closeDrillPopover();
      closeDrawer();
      openNodeInfoPopover(e.clientX, e.clientY, model);
    });
    graph.on("edge:click", (ev) => {
      closeDrawer();
      closeEdgeInfoPopover();
      const isMulti = isMultiSelectEvent(ev);
      const item = ev.item;
      noteCanvasHit("edge-click", item, ev, "edge:click");
      if (!item) return;
      const wasSelected = item.hasState?.("selected");
      const nextSelected = toggleCanvasItemSelection(item, {
        targetGraph: graph,
        multi: isMulti,
        reason: "edge:click",
      });
      const edgeModel = item.getModel?.() || {};
      if (isGhostProbeWindow()) {
        const edgeId =
          String(edgeModel?.id || "").trim() ||
          `${String(edgeModel?.source || "").trim()}->${String(edgeModel?.target || "").trim()}`;
        if (isDeleteProbeVerbose()) {
          logGhostProbe(
            "edge-click-near-delete",
            {
              edgeId,
              isMulti,
              wasSelected: !!wasSelected,
              nextSelected: !!nextSelected,
            },
            {
              dedupKey: `edge-click-near-delete|${state.lastDeleteProbeId || ""}|${edgeId}|${nextSelected ? 1 : 0}`,
              dedupMs: 120,
              onlyInDeleteWindow: true,
            }
          );
        }
        scheduleGraphConsistencyCheck("edge-click-near-delete");
      }
      if (!nextSelected) {
        closeEdgeInfoPopover();
        return;
      }
      const model = edgeModel;
      let lane = "";
      if (String(model.mode || "").toLowerCase() === "double") {
        lane = resolveEdgeLane(model, "") || inferEdgeLaneByPoint(graph, model, ev);
        if (lane) {
          model.__selectedLane = lane;
          model.__hitLane = lane;
          rememberEdgeLane(model, lane);
        }
      }
      try {
        const laneLog = lane || resolveEdgeLane(model, "");
        const topAmt = parseAmountWithCurrency(model.labelTop) || 0;
        const botAmt = parseAmountWithCurrency(model.labelBottom) || 0;
        const clickLogKey = [
          String(model.id || ""),
          String(model.mode || ""),
          laneLog || "",
          String(edgeAmountByLane(model, laneLog)),
        ].join("|");
        if (shouldLogEdgeInfoEvent("click-lane", clickLogKey, 220)) {
          log("INFO", "edge click detail lane", {
            id: model.id || "",
            mode: model.mode || "",
            lane: laneLog,
            lineAmount: edgeAmountByLane(model, laneLog),
            topLabelAmount: topAmt,
            bottomLabelAmount: botAmt,
          });
        }
      } catch (e) {}
      if (!model.userCreated) {
        const point = resolveClientPointFromEvent(graph, ev);
        openEdgeInfoPopover(point.x, point.y, model, lane);
        return;
      }
      openEdgeDetailDrawer(model);
    });
    graph.on("edge:dblclick", (ev) => {
      const item = ev.item;
      noteCanvasHit("edge-dblclick", item, ev, "edge:dblclick");
      if (!item) return;
      closeEdgeInfoPopover();
      const model = item.getModel?.() || {};
      let lane = "";
      if (String(model.mode || "").toLowerCase() === "double") {
        lane = resolveEdgeLane(model, "") || inferEdgeLaneByPoint(graph, model, ev);
        if (lane) {
          model.__selectedLane = lane;
          model.__hitLane = lane;
          rememberEdgeLane(model, lane);
        }
      }
      if (!model.userCreated) {
        const point = resolveClientPointFromEvent(graph, ev);
        openEdgeInfoPopover(point.x, point.y, model, lane);
        return;
      }
      openEdgeLabelDialog(item);
    });

    graph.on("node:mouseenter", (ev) => {
      if (isPerfMode()) {
        if (ev.item) {
          revealNetworkDenseIncidentEdgesForHover(graph, ev.item, "node:mouseenter", { allowPerfMode: true });
        }
        return;
      }
      if (ev.item) {
        noteCanvasHit("node-hover", ev.item, ev, "node:mouseenter");
        setCanvasItemHover(ev.item, true, { targetGraph: graph, reason: "node:mouseenter" });
        revealNetworkDenseIncidentEdgesForHover(graph, ev.item, "node:mouseenter");
        showNodeHoverTip(ev.item.getModel?.() || {}, ev);
      }
    });
    graph.on("node:mouseleave", (ev) => {
      if (ev.item) {
        setCanvasItemHover(ev.item, false, { targetGraph: graph, reason: "node:mouseleave" });
      }
      restoreNetworkDenseHoverEdges(graph, "node:mouseleave");
      hideNodeHoverTip();
    });
    graph.on("edge:mouseenter", (ev) => {
      if (isPerfMode()) return;
      if (ev.item) {
        noteCanvasHit("edge-hover", ev.item, ev, "edge:mouseenter");
        setCanvasItemHover(ev.item, true, { targetGraph: graph, reason: "edge:mouseenter" });
      }
    });
    graph.on("edge:mouseleave", (ev) => {
      if (ev.item) {
        setCanvasItemHover(ev.item, false, { targetGraph: graph, reason: "edge:mouseleave" });
      }
    });
    graph.on("canvas:mouseleave", () => {
      restoreNetworkDenseHoverEdges(graph, "canvas:mouseleave");
      hideNodeHoverTip();
    });
    graph.on("mousemove", (ev) => {
      if (!state.createNodeMode) return;
      const e = extractDomEvent(ev);
      if (e) {
        updateGhostFromClient(e.clientX, e.clientY);
        return;
      }
      const x = ev?.canvasX ?? ev?.x ?? 0;
      const y = ev?.canvasY ?? ev?.y ?? 0;
      updateGhostPosition({ x, y });
    });

    graph.on("viewportchange", () => {
      syncCanvasViewport(graph, "viewportchange");
    });
    graph.on("afterrender", () => {
      if (isDeleteProbeVerbose()) {
        logGhostProbe(
          "afterrender-near-delete",
          { reason: "afterrender" },
          {
            dedupKey: `afterrender-near-delete|${state.graphMutationSeq || 0}`,
            dedupMs: 120,
            onlyInDeleteWindow: true,
          }
        );
      }
      scheduleGraphConsistencyCheck("afterrender");
      scheduleProjectionPointLayerRedraw("afterrender");
      scheduleProjectionAutoMaterialize("afterrender");
    });

    if (!state.graphResizeHandler) {
      state.graphResizeHandler = () => {
        try {
          const g = state.graph?.instance;
          const el = $("#graphContainer");
          if (!g || !el) return;
          const w = el.clientWidth || 800;
          const h = el.clientHeight || 600;
          g.changeSize(w, h);
          scheduleProjectionPointLayerRedraw("resize");
          scheduleProjectionAutoMaterialize("resize");
        } catch (e) {}
      };
      window.addEventListener("resize", state.graphResizeHandler);
    }

    state.graph.inited = true;
    state.graph.instance = graph;
    return graph;
  }

  function updateGraphStats() {
    const summary = buildGraphStatsSummary(state.graph.data, state.layoutPreset || "compact");
    const n = summary.nodes;
    const e = summary.edges;
    const view = getActiveView();
    if (view) view.counts = summary.status === "verified" ? { nodes: n, edges: e } : { nodes: null, edges: null };
    updateGraphStatsFloat(summary);
    updateViewCard(view);
    renderViewsBar();
    scheduleShellStateSync("graph-stats");
  }

  function parseAmountWithCurrency(text) {
    const raw = String(text || "");
    if (!raw) return 0;
    if (!/[￥元]/.test(raw)) return 0;
    return parseAmountFromLabel(raw);
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
    if (canvasShellAdapter && typeof canvasShellAdapter.computeGraphTotalAmount === "function") {
      return canvasShellAdapter.computeGraphTotalAmount(edges);
    }
    let total = 0;
    (edges || []).forEach((edge) => {
      total += edgeAmountTotal(edge);
    });
    return total;
  }

  function getGraphSearchInput() {
    return $("#graphSearch");
  }

  function syncGraphSearchQuery(value, { syncShell = true } = {}) {
    const next = String(value == null ? "" : value);
    state.graphSearchQuery = next;
    const input = getGraphSearchInput();
    if (input && input.value !== next) {
      input.value = next;
    }
    if (syncShell) {
      scheduleShellStateSync("graph-search-query");
    }
  }

  function requestReactGraphSearchFocus() {
    state.graphSearchFocusToken = (Number(state.graphSearchFocusToken) || 0) + 1;
    scheduleShellStateSync("graph-search-focus");
  }

  function updateGraphStatsFloat(summary = null) {
    const wrap = $("#graphStatsFloat");
    if (!wrap) return;
    const status = String(summary?.status || "unknown").trim();
    const verified = status === "verified";
    const layoutLabel = String(summary?.layoutLabel || "").trim();
    const layoutEl = $("#graphStatsLayout");
    const nodesEl = $("#graphStatsNodes");
    const edgesEl = $("#graphStatsEdges");
    const amountEl = $("#graphStatsAmount");
    wrap.dataset.flowSummaryStatus = verified ? "verified" : status || "unknown";
    if (layoutEl) {
      layoutEl.textContent = verified
        ? layoutLabel || layoutPresetLabel(state.layoutPreset || "compact")
        : status === "blocked"
          ? "图谱事实发布已阻断"
          : status === "partial"
            ? "仅有部分数据覆盖"
            : status === "unloaded"
              ? "尚未加载图谱结果"
              : "图谱统计尚未通过宿主证据验证";
    }
    if (nodesEl) nodesEl.textContent = verified ? String(summary.nodes) : "—";
    if (edgesEl) edgesEl.textContent = verified ? String(summary.edges) : "—";
    if (amountEl) amountEl.textContent = verified ? `￥${fmtMoney(summary.amount)}` : "—";
  }

  function setGraphLoading(on, text = "") {
    state.graphLoading = !!on;
    const overlay = $("#graphLoading");
    if (!overlay) return;
    if (!on) {
      overlay.style.display = "none";
      overlay.setAttribute("aria-hidden", "true");
      const hasGraphData = (state.graph?.data?.nodes?.length || 0) > 0;
      if (state.pendingFit || hasGraphData) {
        state.pendingFit = true;
        setTimeout(() => {
          if (!state.pendingFit) return;
          scheduleFit(10);
        }, 0);
      }
      scheduleShellStateSync("graph-loading");
      return;
    }
    const t = $("#graphLoadingText");
    if (t) t.textContent = text || "正在生成图谱…";
    overlay.style.display = "flex";
    overlay.setAttribute("aria-hidden", "false");
    scheduleShellStateSync("graph-loading");
  }

  function normalizeGraphPayload(res) {
    const rawPayload = normalizeRawGraphPayload(res);
    const seedSet = new Set(Array.from(state.selected));
    const style = state.graphStyle || {};

    let nodes = (rawPayload.nodeEntries || [])
      .map((entry) =>
        buildGraphNodeDisplayModel(entry, {
          style,
          theme: state.graphThemeName || resolveGraphThemeName(),
          isSeed: seedSet.has(entry?.id),
          defaultFontFamily: FONT_LIST[0]?.value || "PingFang SC",
        })
      )
      .filter(Boolean);

    let edges = (rawPayload.edgeEntries || [])
      .map((entry) =>
        buildGraphEdgeDisplayModel(entry, {
          style,
          theme: state.graphThemeName || resolveGraphThemeName(),
          defaultFontFamily: FONT_LIST[0]?.value || "PingFang SC",
        })
      )
      .filter(Boolean);

    const filtered = applyGraphDisplayFilters(
      { nodes, edges },
      {
        graphMode: state.graphMode,
        focusKeyType: state.focusKeyType,
        focusId: state.focusId,
        focusName: state.focusName,
        focusLabel: state.focusLabel,
        focusNames: state.focusNames,
        focusUnknownName: state.focusUnknownName,
        includeMissingCounterparty: state.includeMissingCounterparty,
        focusPlaceholderKinds: state.focusPlaceholderKinds,
        selectedIds: state.selected instanceof Set ? Array.from(state.selected) : [],
        focusOnly: state.focusOnly,
        isStatsSource: isStatsSourceValue(state.source),
        isStatsBatchFocus: isStatsBatchFocusRequest(state),
      }
    );

    return { nodes: filtered.nodes, edges: filtered.edges };
  }

  function shouldChunkNetworkBuildNormalize(res) {
    const layoutPreset = String(state.layoutPreset || "").trim().toLowerCase();
    if (layoutPreset !== "network") return false;
    const nodeCount = Array.isArray(res?.nodes) ? res.nodes.length : 0;
    const edgeCount = Array.isArray(res?.edges) ? res.edges.length : 0;
    return nodeCount > 1200 || edgeCount > 3500;
  }

  function createChunkedNormalizeDetail(res) {
    return {
      chunked: true,
      nodes: Array.isArray(res?.nodes) ? res.nodes.length : 0,
      edges: Array.isArray(res?.edges) ? res.edges.length : 0,
      phases: [],
      maxChunkMs: 0,
      yieldCount: 0,
    };
  }

  async function maybeYieldChunkedNormalize(detail, phase, chunkStartedAt, processed, total) {
    const durationMs = Math.max(0, Math.round((performance.now() - Number(chunkStartedAt || performance.now())) * 10) / 10);
    detail.maxChunkMs = Math.max(Number(detail.maxChunkMs) || 0, durationMs);
    detail.phases.push({
      phase,
      processed,
      total,
      durationMs,
    });
    detail.yieldCount += 1;
    await yieldToMainThread();
    return performance.now();
  }

  async function normalizeGraphPayloadChunked(res) {
    const detail = createChunkedNormalizeDetail(res);
    const nodesRaw = Array.isArray(res?.nodes) ? res.nodes : [];
    const edgesRaw = Array.isArray(res?.edges) ? res.edges : [];
    const nodeMap = new Map();
    const nodeRemap = new Map();
    let chunkStart = performance.now();
    for (let index = 0; index < nodesRaw.length; index += 1) {
      const node = nodesRaw[index];
      const id = String(node?.id || "").trim();
      if (id) {
        const title = String(node?.title || node?.label || "").trim();
        const name = String(node?.name || "").trim();
        const displayIdRaw = String(node?.display_id || node?.displayId || node?.id || "").trim();
        const displayIds = normalizeDisplayIds(node?.display_ids || node?.displayIds || displayIdRaw);
        const keyBase = `${name || title || ""}||${displayIdRaw || ""}`.trim();
        const key = keyBase && keyBase !== "||" ? keyBase : id;
        const existing = nodeMap.get(key);
        if (!existing) {
          nodeMap.set(key, { raw: node, id, title, name, displayIdRaw, displayIds });
        } else if (existing.id !== id) {
          nodeRemap.set(id, existing.id);
          const existingAmount = Number(existing.raw?.total_amount) || 0;
          const existingCount = Number(existing.raw?.total_count) || 0;
          const nextAmount = Number(node?.total_amount) || 0;
          const nextCount = Number(node?.total_count) || 0;
          existing.displayIds = normalizeDisplayIds([...(existing.displayIds || []), ...(displayIds || [])]);
          existing.raw = {
            ...existing.raw,
            total_amount: existingAmount + nextAmount,
            total_count: existingCount + nextCount,
            ntype: existing.raw?.ntype === "seed" || node?.ntype === "seed" ? "seed" : existing.raw?.ntype,
          };
        }
      }
      if ((index + 1) % 800 === 0 && index + 1 < nodesRaw.length) {
        chunkStart = await maybeYieldChunkedNormalize(detail, "raw-nodes", chunkStart, index + 1, nodesRaw.length);
      }
    }
    detail.phases.push({
      phase: "raw-nodes",
      processed: nodesRaw.length,
      total: nodesRaw.length,
      durationMs: Math.max(0, Math.round((performance.now() - chunkStart) * 10) / 10),
    });
    const nodeValues = Array.from(nodeMap.values());
    const nodeEntries = [];
    chunkStart = performance.now();
    for (let index = 0; index < nodeValues.length; index += 1) {
      const { raw, id, title, name, displayIdRaw, displayIds } = nodeValues[index];
      const displayList = Array.isArray(displayIds) && displayIds.length ? displayIds : normalizeDisplayIds(displayIdRaw);
      const mergedDisplay = !!raw?.mergedDisplay;
      nodeEntries.push({
        raw,
        id,
        title,
        name,
        displayIdRaw,
        displayIds: displayList,
        displayId: formatDisplayIdLabel(displayList, displayIdRaw, { allowGroup: mergedDisplay }),
        mergedDisplay,
      });
      if ((index + 1) % 900 === 0 && index + 1 < nodeValues.length) {
        chunkStart = await maybeYieldChunkedNormalize(detail, "node-entries", chunkStart, index + 1, nodeValues.length);
      }
    }
    detail.phases.push({
      phase: "node-entries",
      processed: nodeValues.length,
      total: nodeValues.length,
      durationMs: Math.max(0, Math.round((performance.now() - chunkStart) * 10) / 10),
    });
    const edgeEntries = [];
    chunkStart = performance.now();
    for (let index = 0; index < edgesRaw.length; index += 1) {
      const edge = edgesRaw[index];
      const sourceRaw = String(edge?.source || "").trim();
      const targetRaw = String(edge?.target || "").trim();
      const source = nodeRemap.get(sourceRaw) || sourceRaw;
      const target = nodeRemap.get(targetRaw) || targetRaw;
      if (source && target) {
        edgeEntries.push({
          raw: edge,
          sourceRaw,
          targetRaw,
          source,
          target,
          isSelfLoop: source === target,
        });
      }
      if ((index + 1) % 1800 === 0 && index + 1 < edgesRaw.length) {
        chunkStart = await maybeYieldChunkedNormalize(detail, "raw-edges", chunkStart, index + 1, edgesRaw.length);
      }
    }
    detail.phases.push({
      phase: "raw-edges",
      processed: edgesRaw.length,
      total: edgesRaw.length,
      durationMs: Math.max(0, Math.round((performance.now() - chunkStart) * 10) / 10),
    });
    const seedSet = new Set(Array.from(state.selected));
    const style = state.graphStyle || {};
    const theme = state.graphThemeName || resolveGraphThemeName();
    const defaultFontFamily = FONT_LIST[0]?.value || "PingFang SC";
    const nodes = [];
    chunkStart = performance.now();
    for (let index = 0; index < nodeEntries.length; index += 1) {
      const entry = nodeEntries[index];
      const node = buildGraphNodeDisplayModel(entry, {
        style,
        theme,
        isSeed: seedSet.has(entry?.id),
        defaultFontFamily,
      });
      if (node) nodes.push(node);
      if ((index + 1) % 900 === 0 && index + 1 < nodeEntries.length) {
        chunkStart = await maybeYieldChunkedNormalize(detail, "node-display", chunkStart, index + 1, nodeEntries.length);
      }
    }
    detail.phases.push({
      phase: "node-display",
      processed: nodeEntries.length,
      total: nodeEntries.length,
      durationMs: Math.max(0, Math.round((performance.now() - chunkStart) * 10) / 10),
    });
    const edges = [];
    chunkStart = performance.now();
    for (let index = 0; index < edgeEntries.length; index += 1) {
      const edge = buildGraphEdgeDisplayModel(edgeEntries[index], {
        style,
        theme,
        defaultFontFamily,
      });
      if (edge) edges.push(edge);
      if ((index + 1) % 1800 === 0 && index + 1 < edgeEntries.length) {
        chunkStart = await maybeYieldChunkedNormalize(detail, "edge-display", chunkStart, index + 1, edgeEntries.length);
      }
    }
    detail.phases.push({
      phase: "edge-display",
      processed: edgeEntries.length,
      total: edgeEntries.length,
      durationMs: Math.max(0, Math.round((performance.now() - chunkStart) * 10) / 10),
    });
    const filterStart = performance.now();
    const filtered = applyGraphDisplayFilters(
      { nodes, edges },
      {
        graphMode: state.graphMode,
        focusKeyType: state.focusKeyType,
        focusId: state.focusId,
        focusName: state.focusName,
        focusLabel: state.focusLabel,
        focusNames: state.focusNames,
        focusUnknownName: state.focusUnknownName,
        includeMissingCounterparty: state.includeMissingCounterparty,
        focusPlaceholderKinds: state.focusPlaceholderKinds,
        selectedIds: state.selected instanceof Set ? Array.from(state.selected) : [],
        focusOnly: state.focusOnly,
        isStatsSource: isStatsSourceValue(state.source),
        isStatsBatchFocus: isStatsBatchFocusRequest(state),
      }
    );
    const filterDuration = Math.max(0, Math.round((performance.now() - filterStart) * 10) / 10);
    detail.maxChunkMs = Math.max(Number(detail.maxChunkMs) || 0, filterDuration);
    detail.phases.push({
      phase: "display-filter",
      processed: nodes.length + edges.length,
      total: nodes.length + edges.length,
      durationMs: filterDuration,
    });
    detail.nodeEntries = nodeEntries.length;
    detail.edgeEntries = edgeEntries.length;
    detail.nodeRemapCount = nodeRemap.size;
    return {
      nodes: filtered.nodes,
      edges: filtered.edges,
      normalizeDetail: detail,
    };
  }

  async function normalizeGraphPayloadForBuild(res) {
    if (!shouldChunkNetworkBuildNormalize(res)) {
      return { ...normalizeGraphPayload(res), normalizeDetail: { chunked: false } };
    }
    return normalizeGraphPayloadChunked(res);
  }

  function initLayoutWorker() {
    if (state.layoutWorker || typeof Worker === "undefined" || state.layoutWorkerDisabledReason) return;
    try {
      const worker = flowLayoutRuntimeAdapter.createLayoutWorker({ script: LAYOUT_WORKER_SCRIPT });
      if (!worker) return;
      worker.onmessage = async (ev) => {
        const data = ev.data || {};
        const pending = state.layoutWorkerPending;
        if (!pending || pending.seq !== data.seq) return;
        if (String(data?.type || "") === "layout-progress") {
          layoutWorkerLifecycleController.recordProgress(pending, data);
          return;
        }
        layoutWorkerLifecycleController.clearPendingTimer(pending);
        layoutWorkerLifecycleController.recordComplete(pending, data);
        if (pending.token && pending.token !== state.requestId) {
          layoutWorkerLifecycleController.cancelStaleRequest(pending, data);
          return;
        }
        const workerFallbackToSync = (reason = "", { disableWorker = false } = {}) => {
          layoutWorkerLifecycleController.fallbackToSync(pending, data, reason, { disableWorker });
        };
        if (String(data?.error || "").trim().toLowerCase() === "layout-core-missing") {
          workerFallbackToSync("layout-core-missing", { disableWorker: true });
          return;
        }
        const nodes = pending.nodes;
        const workerNodes = Array.isArray(data?.nodes) ? data.nodes : [];
        const nodePlanResult = await applyLayoutWorkerNodePlan(nodes, workerNodes, {
          metaKeys: flowLayoutRuntimeAdapter.getLayoutMetaKeys(),
          computeLayoutNodePlan,
          beforeApply: () =>
            state.layoutWorkerPending === pending &&
            (!pending.token || pending.token === state.requestId),
        });
        if (state.layoutWorkerPending !== pending || (pending.token && pending.token !== state.requestId)) {
          layoutWorkerLifecycleController.recordStaleAfterNodePlan(pending, data);
          return;
        }
        if (!nodePlanResult.applied) {
          layoutWorkerLifecycleController.failNodePlan(pending, data, nodePlanResult);
          return;
        }
        const workerPreset = String(pending.preset || "compact").trim().toLowerCase();
        if (
          (workerPreset === "compact" || workerPreset === "relation" || workerPreset === "network") &&
          nodePlanResult.gridFallbackLikely === true
        ) {
          workerFallbackToSync("layout-worker-grid-fallback", { disableWorker: true });
          return;
        }
        if (data?.layoutReport && typeof data.layoutReport === "object") {
          const layoutReport = consumeNetworkSectorPlacementPayloadReport(data.layoutReport, {
            source: "layout-worker",
            key: pending?.layoutCacheKey || "",
          });
          updateClusterSlotCacheFromLayoutReport(layoutReport, pending?.layoutHints || null);
          const layoutReportSummary =
            summarizeLayoutReportPayload(layoutReport) || layoutReport;
          const layoutReportDigest =
            digestLayoutReportPayload(layoutReportSummary) || layoutReportSummary;
          try {
            log("INFO", "layout worker report", layoutReportDigest);
          } catch (e) {}
          const semanticState = ensureLayoutSemanticState();
          semanticState.lastReports = {
            ...(semanticState.lastReports || {}),
            worker: layoutReportSummary,
          };
        }
        if (pending?.layoutCacheKey) {
          layoutPlanCacheController.rememberCache(pending.layoutCacheKey, pending.preset || "compact", nodes || []);
          state.lastAppliedLayoutCacheKey = String(pending.layoutCacheKey || "");
        }
        if (pending.deferApply?.preserveCenter) {
          recenterLayoutByBounds(nodes, pending.deferApply.preserveCenter);
        }
        applyLayoutEdgeRouting(nodes, pending.edges || [], pending.preset || "compact");
        const graph = ensureGraph();
        if (graph) {
          syncGraphEdgeRoutingFromData(graph, pending.edges || []);
          if ((graph.getNodes?.().length || 0) === 0) {
            ensureGraphDataVisible(graph, pending.nodes || [], pending.edges || [], "layout-worker");
          }
          layoutGraphApplyController.applyPreparedLayout({
            graph,
            nodes,
            deferApply: pending.deferApply || {},
            syncReason: "layout-worker-apply",
            patchMeta: pending.patchMeta || null,
            allowAnimation: true,
          });
        }
        const donePreset = pending.preset || "compact";
        if (donePreset === "compact") {
          state.networkLayout.lastCompactCoreIds = [];
        }
        if (donePreset === "network") {
          updateMinimap();
          scheduleCompactLayoutPrewarm({ reason: "layout-worker-done" });
        }
        if (donePreset === "compact" || donePreset === "relation") {
          scheduleNetworkLayoutPrewarm({ reason: "layout-worker-done" });
        }
        updateLayoutButtons();
        state.layoutWorkerPending = null;
        state.layoutWorkerStage = "idle";
      };
      worker.onerror = (ev) => {
        layoutWorkerLifecycleController.handleRuntimeError(ev, state.layoutWorkerPending);
      };
      state.layoutWorker = worker;
      state.layoutWorkerStage = "idle";
    } catch (e) {
      state.layoutWorker = null;
      state.layoutWorkerStage = "";
      try {
        log("WARN", "layout worker init failed", { message: String(e?.message || e) });
      } catch (e2) {}
    }
  }

  function shouldUseLayoutWorker() {
    return !state.layoutWorkerDisabledReason && !!state.layoutWorker;
  }

  function layoutPlaceholder(nodes) {
    if (!nodes || !nodes.length) return nodes;
    const count = nodes.length;
    const scale = layoutScale(count);
    const cols = Math.ceil(Math.sqrt(count));
    const spacing = 120 * scale;
    const rows = Math.ceil(count / cols);
    const halfW = ((cols - 1) * spacing) / 2;
    const halfH = ((rows - 1) * spacing) / 2;
    nodes.forEach((n, i) => {
      const cx = i % cols;
      const cy = Math.floor(i / cols);
      n.x = cx * spacing - halfW;
      n.y = cy * spacing - halfH;
    });
    return nodes;
  }

  function applyPendingLayoutFallback(pending, reason = "") {
    if (!pending) return false;
    try {
      applyLayoutSync(
        pending.nodes || [],
        pending.edges || [],
        pending.preset || "compact",
        pending.focusId || "",
        pending.layoutHints || {}
      );
      applyLayoutEdgeRouting(pending.nodes || [], pending.edges || [], pending.preset || "compact");
      if (pending?.layoutCacheKey) {
        layoutPlanCacheController.rememberCache(
          pending.layoutCacheKey,
          pending.preset || "compact",
          pending.nodes || []
        );
        state.lastAppliedLayoutCacheKey = String(pending.layoutCacheKey || "");
      }
      const fallbackPreset = pending.preset || "compact";
      if (fallbackPreset === "compact") {
        state.networkLayout.lastCompactCoreIds = [];
      }
      const graph = ensureGraph();
      if (graph) {
        syncGraphEdgeRoutingFromData(graph, pending.edges || []);
        layoutGraphApplyController.applyPreparedLayout({
          graph,
          nodes: pending.nodes || [],
          deferApply: pending.deferApply || {},
          syncReason: "layout-worker-fallback",
          allowAnimation: false,
        });
      }
      if (fallbackPreset === "network") {
        updateMinimap();
        scheduleCompactLayoutPrewarm({ reason: `${reason || "layout-worker"}-fallback` });
      }
      if (fallbackPreset === "compact" || fallbackPreset === "relation") {
        scheduleNetworkLayoutPrewarm({ reason: `${reason || "layout-worker"}-fallback` });
      }
      updateLayoutButtons();
      state.layoutWorkerStage = "idle";
      if (pending.patchMeta) {
        publishGraphPatch(pending.patchMeta);
      }
      return true;
    } catch (e) {
      return false;
    }
  }

  function applyNetworkPlanNodeUpdates(nodes, nodeUpdates, { allowPartial = false } = {}) {
    const rows = Array.isArray(nodes) ? nodes : [];
    const updates = Array.isArray(nodeUpdates) ? nodeUpdates : [];
    if (!rows.length || !updates.length) return false;
    if (!allowPartial && rows.length !== updates.length) return false;
    const rowById = new Map();
    rows.forEach((node, index) => {
      const id = String(node?.id || "").trim();
      if (id && !rowById.has(id)) rowById.set(id, { node, index });
    });
    const normalizedUpdates = [];
    for (const update of updates) {
      const id = String(update?.id || "").trim();
      if (!id) continue;
      const index = Number(update?.index);
      const indexedNode = Number.isFinite(index) && index >= 0 ? nodes[index] : null;
      const row =
        indexedNode && String(indexedNode?.id || "").trim() === id
          ? { node: indexedNode, index }
          : rowById.get(id);
      if (!row) continue;
      const x = Number(update?.x);
      const y = Number(update?.y);
      if (!Number.isFinite(x) || !Number.isFinite(y)) continue;
      normalizedUpdates.push({ node: row.node, update, x, y });
    }
    if (!normalizedUpdates.length) return false;
    if (!allowPartial && normalizedUpdates.length < Math.floor(updates.length * 0.9)) return false;
    normalizedUpdates.forEach(({ node, update, x, y }) => {
      node.x = x;
      node.y = y;
    });
    return true;
  }

  function networkEdgeKey(source, target) {
    const s = networkPlanEndpoint(source);
    const t = networkPlanEndpoint(target);
    return s && t ? `${s}\u0000${t}` : "";
  }

  function applyNetworkPlanEdgeUpdates(edges, edgeUpdates) {
    // Network topology plans are position-only; edges keep their runtime data and style.
    return false;
    const edgeRows = Array.isArray(edges) ? edges : [];
    const updateRows = Array.isArray(edgeUpdates) ? edgeUpdates : [];
    if (!edgeRows.length || !updateRows.length) return false;
    const updatesByKey = new Map();
    updateRows.forEach((update) => {
      const key = networkEdgeKey(update?.source, update?.target);
      if (key) updatesByKey.set(key, update);
    });
    let applied = 0;
    edgeRows.forEach((edge) => {
      const source = typeof edge?.source === "object" ? edge.source?.id : edge?.source;
      const target = typeof edge?.target === "object" ? edge.target?.id : edge?.target;
      const update = updatesByKey.get(networkEdgeKey(source, target));
      if (!update) return;
      [
        "layoutEdgeTier",
        "layoutCommunityPair",
        "layoutEdgeBundleId",
        "layoutEdgeBundleSize",
        "layoutEdgeBundleWeight",
        "layoutEdgeBundleRank",
        "layoutEdgeBundled",
        "layoutEdgeBundleRepresentative",
        "layoutEdgeBundleRouted",
        "edgeOffset",
        "edgeRoute",
        "orthAxis",
        "orthBias",
        "edgeLabelAnchorMode",
      ].forEach((key) => {
        const value = update?.[key];
        if (value == null || value === "") delete edge[key];
        else edge[key] = value;
      });
      edge.layoutHiddenBySkeleton = String(update?.layoutEdgeTier || "") === "hidden";
      applied += 1;
    });
    return applied > 0;
  }

  function buildNetworkPlanNodeUpdateIdSet(nodeUpdates = []) {
    return new Set(
      (Array.isArray(nodeUpdates) ? nodeUpdates : [])
        .map((update) => String(update?.id || "").trim())
        .filter(Boolean)
    );
  }

  function collectNetworkPlanUpdatedNodes(nodes = [], nodeUpdates = []) {
    const rowById = new Map(
      (Array.isArray(nodes) ? nodes : [])
        .map((node) => [String(node?.id || "").trim(), node])
        .filter(([id]) => !!id)
    );
    return (Array.isArray(nodeUpdates) ? nodeUpdates : [])
      .map((update) => rowById.get(String(update?.id || "").trim()))
      .filter(Boolean);
  }

  function collectNetworkPlanVisibleNodes(nodes = [], nodeUpdates = []) {
    const updatedNodes = collectNetworkPlanUpdatedNodes(nodes, nodeUpdates);
    const visibleNodes = updatedNodes.filter(
      (node) => String(node?.layoutVisibilityTier || "") !== "hidden" && !node?.layoutHiddenBySkeleton
    );
    return visibleNodes.length ? visibleNodes : updatedNodes;
  }

  function restoreNetworkFullGraphVisibilityState(nodes = [], edges = [], mode = "") {
    if (isLargeNetworkTopologyMode(mode)) return { nodes: 0, edges: 0 };
    let restoredNodes = 0;
    let restoredEdges = 0;
    (Array.isArray(nodes) ? nodes : []).forEach((node) => {
      if (!node || typeof node !== "object") return;
      const hidden = !!node.layoutHiddenBySkeleton || String(node.layoutVisibilityTier || "") === "hidden";
      if (!hidden) return;
      node.layoutVisibilityTier = "secondary";
      node.layoutHiddenBySkeleton = false;
      restoredNodes += 1;
    });
    (Array.isArray(edges) ? edges : []).forEach((edge) => {
      if (!edge || typeof edge !== "object") return;
      const hidden = !!edge.layoutHiddenBySkeleton || String(edge.layoutEdgeTier || "") === "hidden";
      if (!hidden) return;
      edge.layoutEdgeTier = "secondary";
      edge.layoutHiddenBySkeleton = false;
      restoredEdges += 1;
    });
    return { nodes: restoredNodes, edges: restoredEdges };
  }

  function collectNetworkRuntimeVisibleModels(graph) {
    const items = Array.isArray(graph?.getNodes?.()) ? graph.getNodes() : [];
    return items
      .map((item) => item?.getModel?.() || null)
      .filter((model) => {
        if (!model) return false;
        if (model.layoutHiddenBySkeleton || String(model.layoutVisibilityTier || "") === "hidden") return false;
        return Number.isFinite(Number(model.x)) && Number.isFinite(Number(model.y));
      });
  }

  function estimateNetworkRuntimeLabelHalfWidth(model = {}) {
    const text = String(model.name || model.title || model.label || model.displayId || model.display_id || model.id || "");
    const units = Array.from(text).reduce((sum, ch) => {
      const code = ch.codePointAt(0) || 0;
      if (code > 255) return sum + 1.02;
      if (/[A-Z]/.test(ch)) return sum + 0.72;
      return sum + 0.62;
    }, 0);
    const fontSize = Math.max(8, Math.min(36, Number(model.fontSize || state.graphStyle?.fontSize || 13) || 13));
    return Math.max(22, units * fontSize * 0.5);
  }

  function networkRuntimeLabelBox(model = {}, padding = 0) {
    const x = Number(model.x);
    const y = Number(model.y);
    if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
    const radius = Math.max(1, Number(model.r || model.radius) || 18);
    const halfWidth = Math.max(0, Number(model.labelHalfWidth) || estimateNetworkRuntimeLabelHalfWidth(model));
    const height = Math.max(10, Number(model.labelBottom || model.labelHeight) || 18);
    const pad = Math.max(0, Number(padding) || 0);
    return {
      minX: x - halfWidth - pad,
      minY: y + radius + 6 - pad,
      maxX: x + halfWidth + pad,
      maxY: y + radius + 6 + height + pad,
    };
  }

  function networkRuntimeBoxesOverlap(left, right) {
    if (!left || !right) return false;
    return left.minX <= right.maxX && left.maxX >= right.minX && left.minY <= right.maxY && left.maxY >= right.minY;
  }

  function networkRuntimeLabelPriority(model = {}) {
    const role = String(model.role || model.layoutBand || "").trim().toLowerCase();
    const roleRank = role === "core" ? 4 : role === "bridge" ? 3 : role === "adjacent" ? 2 : 1;
    const visibilityRank = String(model.layoutVisibilityTier || "") === "primary" ? 2 : 1;
    return (
      visibilityRank * 10000 +
      roleRank * 1000 +
      (Number(model.layoutBridgeScore) || 0) * 260 +
      Math.log1p(Math.max(0, Number(model.weight || model.total_amount || model.totalAmount || model.amount) || 0)) * 80
    );
  }

  function suppressNetworkRuntimeOverlappingLabels(graph, { maxLabels = 8, padding = 12 } = {}) {
    const items = Array.isArray(graph?.getNodes?.()) ? graph.getNodes() : [];
    if (!items.length) return 0;
    const dataNodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
    const dataNodeById = new Map(dataNodes.map((node) => [String(node?.id || "").trim(), node]));
    const rows = items
      .map((item) => ({ item, model: item?.getModel?.() || {} }))
      .filter(({ model }) => {
        if (model.layoutHiddenBySkeleton || String(model.layoutVisibilityTier || "") === "hidden") return false;
        if (model.nodeLabelHidden || String(model.layoutLabelTier || "") === "hidden") return false;
        return Number.isFinite(Number(model.x)) && Number.isFinite(Number(model.y));
      })
      .sort((left, right) => {
        const score = networkRuntimeLabelPriority(right.model) - networkRuntimeLabelPriority(left.model);
        if (score) return score;
        return String(left.model.id || "").localeCompare(String(right.model.id || ""), "zh-CN");
      });
    const accepted = [];
    let suppressed = 0;
    rows.forEach(({ item, model }) => {
      const box = networkRuntimeLabelBox(model, padding);
      const collides = accepted.some((acceptedBox) => networkRuntimeBoxesOverlap(box, acceptedBox));
      if (accepted.length >= maxLabels || collides) {
        const id = String(model.id || "").trim();
        const dataNode = dataNodeById.get(id);
        if (dataNode) {
          dataNode.layoutLabelTier = "hidden";
          dataNode.nodeLabelHidden = true;
        }
        graph.updateItem?.(item, {
          layoutLabelTier: "hidden",
          nodeLabelHidden: true,
        });
        suppressed += 1;
      } else if (box) {
        accepted.push(box);
      }
    });
    return suppressed;
  }

  function networkRuntimeEdgeEndpointId(value) {
    if (value && typeof value === "object") return String(value.id || value.key || "").trim();
    return String(value || "").trim();
  }

  function networkRuntimeNodeRadius(model = {}) {
    return Math.max(2, Number(model.collisionRadius || model.r || model.radius) || 18);
  }

  function networkRuntimeSegmentsIntersect(left, right) {
    if (!left || !right) return false;
    const orient = (a, b, c) => {
      const value = (b.x - a.x) * (c.y - a.y) - (b.y - a.y) * (c.x - a.x);
      if (Math.abs(value) < 1e-6) return 0;
      return value > 0 ? 1 : -1;
    };
    const o1 = orient(left.source, left.target, right.source);
    const o2 = orient(left.source, left.target, right.target);
    const o3 = orient(right.source, right.target, left.source);
    const o4 = orient(right.source, right.target, left.target);
    return o1 !== o2 && o3 !== o4;
  }

  function collectNetworkRuntimeConnectivityStats(nodeRows = [], edgeRows = []) {
    const ids = (Array.isArray(nodeRows) ? nodeRows : [])
      .map(({ model }) => String(model?.id || "").trim())
      .filter(Boolean);
    const nodeIds = new Set(ids);
    const degree = new Map(ids.map((id) => [id, 0]));
    const adjacency = new Map(ids.map((id) => [id, []]));
    (Array.isArray(edgeRows) ? edgeRows : []).forEach((row) => {
      const source = String(row?.sourceId || "").trim();
      const target = String(row?.targetId || "").trim();
      if (!source || !target || source === target || !nodeIds.has(source) || !nodeIds.has(target)) return;
      degree.set(source, (degree.get(source) || 0) + 1);
      degree.set(target, (degree.get(target) || 0) + 1);
      adjacency.get(source)?.push(target);
      adjacency.get(target)?.push(source);
    });
    let isolatedNodeCount = 0;
    degree.forEach((value) => {
      if (value <= 0) isolatedNodeCount += 1;
    });
    const visited = new Set();
    let componentCount = 0;
    let largestComponentNodeCount = 0;
    ids.forEach((id) => {
      if (visited.has(id)) return;
      componentCount += 1;
      const queue = [id];
      visited.add(id);
      let size = 0;
      while (queue.length) {
        const next = queue.shift();
        size += 1;
        (adjacency.get(next) || []).forEach((neighbor) => {
          if (visited.has(neighbor)) return;
          visited.add(neighbor);
          queue.push(neighbor);
        });
      }
      largestComponentNodeCount = Math.max(largestComponentNodeCount, size);
    });
    return {
      connectedNodeCount: Math.max(0, ids.length - isolatedNodeCount),
      isolatedNodeCount,
      isolatedNodeRatio: ids.length ? Math.round((isolatedNodeCount / ids.length) * 10000) / 10000 : 0,
      componentCount,
      largestComponentNodeCount,
    };
  }

  function collectNetworkRuntimeQualityReport(graph, { mode = "", scope = "viewport", bounds = null, reason = "" } = {}) {
    const startedAt = performance.now();
    const nodeItems = Array.isArray(graph?.getNodes?.()) ? graph.getNodes() : [];
    const edgeItems = Array.isArray(graph?.getEdges?.()) ? graph.getEdges() : [];
    const maxNodes = scope === "viewport" ? 360 : 520;
    const maxEdges = scope === "viewport" ? 520 : 900;
    const scopedNodeRows = nodeItems
      .map((item) => ({ item, model: item?.getModel?.() || {} }))
      .filter(({ item, model }) => {
        const id = String(model?.id || "").trim();
        if (!id) return false;
        if (model.layoutHiddenBySkeleton || String(model.layoutVisibilityTier || "") === "hidden") return false;
        const x = Number(model.x);
        const y = Number(model.y);
        if (!Number.isFinite(x) || !Number.isFinite(y)) return false;
        return !bounds || isNetworkNodeInBounds(model, bounds) || isNetworkViewportPriorityNode(item);
      })
      .sort((left, right) => networkRuntimeLabelPriority(right.model) - networkRuntimeLabelPriority(left.model));
    const sampledNodeRows = scopedNodeRows.slice(0, maxNodes);
    const nodeById = new Map(sampledNodeRows.map(({ model }) => [String(model.id || "").trim(), model]));
    const visibleTierStats = {};
    sampledNodeRows.forEach(({ model }) => {
      const nodeTier = String(model.layoutVisibilityTier || "visible") || "visible";
      const labelTier =
        model.nodeLabelHidden || String(model.layoutLabelTier || "") === "hidden"
          ? "hidden"
          : String(model.layoutLabelTier || "visible") || "visible";
      visibleTierStats[`node:${nodeTier}`] = (visibleTierStats[`node:${nodeTier}`] || 0) + 1;
      visibleTierStats[`label:${labelTier}`] = (visibleTierStats[`label:${labelTier}`] || 0) + 1;
    });
    let nodeOverlapCount = 0;
    const overlapRows = sampledNodeRows.slice(0, 240);
    for (let i = 0; i < overlapRows.length; i += 1) {
      const left = overlapRows[i].model;
      const leftRadius = networkRuntimeNodeRadius(left);
      for (let j = i + 1; j < overlapRows.length; j += 1) {
        const right = overlapRows[j].model;
        const dx = Number(left.x) - Number(right.x);
        const dy = Number(left.y) - Number(right.y);
        const minDistance = leftRadius + networkRuntimeNodeRadius(right) + 4;
        if (dx * dx + dy * dy < minDistance * minDistance) nodeOverlapCount += 1;
      }
    }
    let labelOverlapEstimate = 0;
    const labelBoxes = sampledNodeRows
      .filter(({ model }) => !model.nodeLabelHidden && String(model.layoutLabelTier || "") !== "hidden")
      .slice(0, 120)
      .map(({ model }) => networkRuntimeLabelBox(model, 6))
      .filter(Boolean);
    for (let i = 0; i < labelBoxes.length; i += 1) {
      for (let j = i + 1; j < labelBoxes.length; j += 1) {
        if (networkRuntimeBoxesOverlap(labelBoxes[i], labelBoxes[j])) labelOverlapEstimate += 1;
      }
    }
    const edgeRows = edgeItems
      .map((item, index) => {
        const model = item?.getModel?.() || {};
        if (model.layoutHiddenBySkeleton || String(model.layoutEdgeTier || "") === "hidden") return null;
        const sourceId = networkRuntimeEdgeEndpointId(model.source);
        const targetId = networkRuntimeEdgeEndpointId(model.target);
        const source = nodeById.get(sourceId);
        const target = nodeById.get(targetId);
        if (!source || !target || sourceId === targetId) return null;
        return { model, index, sourceId, targetId, source, target };
      })
      .filter(Boolean)
      .sort((left, right) => {
        const leftRank = String(left.model.layoutEdgeTier || "") === "primary" ? 2 : 1;
        const rightRank = String(right.model.layoutEdgeTier || "") === "primary" ? 2 : 1;
        return rightRank - leftRank || left.index - right.index;
      })
      .slice(0, maxEdges);
    const connectivityStats = collectNetworkRuntimeConnectivityStats(sampledNodeRows, edgeRows);
    edgeRows.forEach(({ model }) => {
      const edgeTier = String(model.layoutEdgeTier || "visible") || "visible";
      visibleTierStats[`edge:${edgeTier}`] = (visibleTierStats[`edge:${edgeTier}`] || 0) + 1;
    });
    const edgeLengths = edgeRows
      .map(({ source, target }) => Math.hypot(Number(source.x) - Number(target.x), Number(source.y) - Number(target.y)))
      .filter((value) => Number.isFinite(value));
    const edgeLengthMean = edgeLengths.length
      ? edgeLengths.reduce((sum, value) => sum + value, 0) / edgeLengths.length
      : 0;
    const edgeLengthStdDev = edgeLengths.length
      ? Math.sqrt(edgeLengths.reduce((sum, value) => sum + (value - edgeLengthMean) * (value - edgeLengthMean), 0) / edgeLengths.length)
      : 0;
    const crossingEdges = edgeRows.slice(0, 180).map((row) => ({
      sourceId: row.sourceId,
      targetId: row.targetId,
      source: { x: Number(row.source.x), y: Number(row.source.y) },
      target: { x: Number(row.target.x), y: Number(row.target.y) },
    }));
    let edgeCrossingSample = 0;
    for (let i = 0; i < crossingEdges.length; i += 1) {
      for (let j = i + 1; j < crossingEdges.length; j += 1) {
        const left = crossingEdges[i];
        const right = crossingEdges[j];
        if (
          left.sourceId === right.sourceId ||
          left.sourceId === right.targetId ||
          left.targetId === right.sourceId ||
          left.targetId === right.targetId
        ) {
          continue;
        }
        if (networkRuntimeSegmentsIntersect(left, right)) edgeCrossingSample += 1;
      }
    }
    const communityBoxes = new Map();
    sampledNodeRows.forEach(({ model }) => {
      const community = String(model.layoutCommunity || model.clusterId || model.layoutClusterId || "").trim();
      if (!community) return;
      const radius = networkRuntimeNodeRadius(model);
      const box = communityBoxes.get(community) || {
        minX: Infinity,
        minY: Infinity,
        maxX: -Infinity,
        maxY: -Infinity,
        count: 0,
      };
      box.minX = Math.min(box.minX, Number(model.x) - radius);
      box.minY = Math.min(box.minY, Number(model.y) - radius);
      box.maxX = Math.max(box.maxX, Number(model.x) + radius);
      box.maxY = Math.max(box.maxY, Number(model.y) + radius);
      box.count += 1;
      communityBoxes.set(community, box);
    });
    const clusterBoxes = Array.from(communityBoxes.values())
      .filter((box) => box.count >= 3 && Number.isFinite(box.minX) && Number.isFinite(box.maxX))
      .slice(0, 120);
    let clusterBBoxOverlapCount = 0;
    for (let i = 0; i < clusterBoxes.length; i += 1) {
      for (let j = i + 1; j < clusterBoxes.length; j += 1) {
        if (networkRuntimeBoxesOverlap(clusterBoxes[i], clusterBoxes[j])) clusterBBoxOverlapCount += 1;
      }
    }
    return {
      durationMs: Math.round((performance.now() - startedAt) * 100) / 100,
      nodeOverlapCount,
      labelOverlapEstimate,
      edgeCrossingSample,
      edgeLengthStdDev: Math.round(edgeLengthStdDev * 100) / 100,
      clusterBBoxOverlapCount,
      mode: String(mode || ""),
      qualityScope: String(scope || "viewport"),
      qualityNodeCount: sampledNodeRows.length,
      qualityLabelCount: labelBoxes.length,
      qualityEdgeCount: edgeRows.length,
      connectedNodeCount: connectivityStats.connectedNodeCount,
      isolatedNodeCount: connectivityStats.isolatedNodeCount,
      isolatedNodeRatio: connectivityStats.isolatedNodeRatio,
      componentCount: connectivityStats.componentCount,
      largestComponentNodeCount: connectivityStats.largestComponentNodeCount,
      visibleTierStats,
      sampled: scopedNodeRows.length > sampledNodeRows.length || edgeItems.length > edgeRows.length,
      source: "network-runtime-refinement",
      reason: String(reason || ""),
    };
  }

  function buildNetworkPlanEdgeUpdateFilters(edgeUpdates = []) {
    const ids = new Set();
    const pairs = new Set();
    (Array.isArray(edgeUpdates) ? edgeUpdates : []).forEach((update) => {
      const id = String(update?.id || "").trim();
      if (id) ids.add(id);
      const pair = networkEdgeKey(update?.source, update?.target);
      if (pair) pairs.add(pair);
    });
    return { ids, pairs };
  }

  function applyNetworkPlanPartialPositionsToGraph(graph, nodeUpdates = []) {
    if (!graph || typeof graph.findById !== "function") return false;
    const updates = Array.isArray(nodeUpdates) ? nodeUpdates : [];
    if (!updates.length) return false;
    const canSetAutoPaint = typeof graph.setAutoPaint === "function";
    if (canSetAutoPaint) graph.setAutoPaint(false);
    let applied = 0;
    const changedItems = [];
    try {
      updates.forEach((update) => {
        const id = String(update?.id || "").trim();
        if (!id) return;
        const x = Number(update?.x);
        const y = Number(update?.y);
        if (!Number.isFinite(x) || !Number.isFinite(y)) return;
        const item = graph.findById(id);
        const model = item?.getModel?.();
        if (!model) return;
        model.x = x;
        model.y = y;
        changedItems.push(item);
        applied += 1;
      });
      if (typeof graph.refreshItem === "function") {
        changedItems.forEach((item) => {
          try {
            graph.refreshItem(item);
          } catch (_error) {}
        });
      }
    } finally {
      if (canSetAutoPaint) graph.setAutoPaint(true);
      graph.paint?.();
    }
    if (applied > 0) {
      updateMinimap();
      scheduleProjectionLayoutIndexSync("network-topology-plan-partial-apply", { delayMs: 80 });
    }
    return applied > 0;
  }

  function networkRuntimeEdgeMatchesPlan(model = {}, edgeFilterIds = null, edgeFilterPairs = null) {
    const id = String(model?.id || "").trim();
    if (id && edgeFilterIds?.has(id)) return true;
    return !!edgeFilterPairs?.has(networkEdgeKey(model?.source, model?.target));
  }

  function ensureNetworkPlanRuntimeSkeletonItems(graph, nodes = [], edges = [], nodeUpdates = [], edgeUpdates = [], options = {}) {
    // Network layout must not materialize a skeleton subset; the graph keeps all current runtime items.
    return { nodes: 0, edges: 0 };
    if (!graph || typeof graph.addItem !== "function") return { nodes: 0, edges: 0 };
    const mode = String(options?.mode || state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (mode !== "xlarge") return { nodes: 0, edges: 0 };
    const updateRows = Array.isArray(nodeUpdates) ? nodeUpdates : [];
    if (!updateRows.length) return { nodes: 0, edges: 0 };
    const nodeById = new Map(
      (Array.isArray(nodes) ? nodes : [])
        .map((node) => [String(node?.id || "").trim(), node])
        .filter(([id]) => !!id)
    );
    const updateById = new Map(
      updateRows
        .map((update) => [String(update?.id || "").trim(), update])
        .filter(([id]) => !!id)
    );
    const edgeByPair = new Map(
      (Array.isArray(edges) ? edges : [])
        .map((edge) => {
          const source = networkPlanEndpoint(edge?.source);
          const target = networkPlanEndpoint(edge?.target);
          return [networkEdgeKey(source, target), edge];
        })
        .filter(([key]) => !!key)
    );
    const runtimeNodeIds = new Set(
      (graph.getNodes?.() || [])
        .map((item) => String(item?.getModel?.()?.id || "").trim())
        .filter(Boolean)
    );
    const runtimeEdgeKeys = new Set(
      (graph.getEdges?.() || [])
        .map((item) => {
          const model = item?.getModel?.() || {};
          return networkEdgeKey(model.source, model.target);
        })
        .filter(Boolean)
    );
    let addedNodes = 0;
    let addedEdges = 0;
    let skippedEdges = 0;
    const materializeNode = (update) => {
      const id = String(update?.id || "").trim();
      if (!id) return false;
      if (graph.findById?.(id)) {
        runtimeNodeIds.add(id);
        return true;
      }
      const dataNode = nodeById.get(id) || {};
      const renderNode = toNetworkSkeletonRenderNode({
        ...dataNode,
        id,
        x: Number(update?.x),
        y: Number(update?.y),
        layoutCommunity: update?.layoutCommunity || dataNode.layoutCommunity || "",
        layoutSuperNodeId: update?.layoutSuperNodeId || dataNode.layoutSuperNodeId || "",
        layoutCommunityRole: update?.layoutCommunityRole || dataNode.layoutCommunityRole || "",
        layoutCommunityNodeCount: update?.layoutCommunityNodeCount ?? dataNode.layoutCommunityNodeCount,
        layoutCommunityVisibleNodeCount:
          update?.layoutCommunityVisibleNodeCount ?? dataNode.layoutCommunityVisibleNodeCount,
        layoutVisibilityTier: update?.layoutVisibilityTier || dataNode.layoutVisibilityTier || "secondary",
        layoutLabelTier: update?.layoutLabelTier || dataNode.layoutLabelTier || "hidden",
        layoutBridgeScore: update?.layoutBridgeScore ?? dataNode.layoutBridgeScore,
        layoutHiddenBySkeleton: String(update?.layoutVisibilityTier || dataNode.layoutVisibilityTier || "") === "hidden",
        nodeLabelHidden: String(update?.layoutLabelTier || dataNode.layoutLabelTier || "") === "hidden",
        r: Number(dataNode.r || dataNode.radius) || 18,
        lineWidth: Number(dataNode.lineWidth) || 2,
      });
      const item = graph.addItem?.("node", renderNode);
      if (!item) return false;
      runtimeNodeIds.add(id);
      addedNodes += 1;
      return true;
    };
    graph.setAutoPaint?.(false);
    try {
      updateRows.forEach((update) => {
        const id = String(update?.id || "").trim();
        if (!id) return;
        if (runtimeNodeIds.has(id) && graph.findById?.(id)) return;
        materializeNode(update);
      });
      (Array.isArray(edgeUpdates) ? edgeUpdates : []).forEach((update, index) => {
        const source = networkPlanEndpoint(update?.source);
        const target = networkPlanEndpoint(update?.target);
        const key = networkEdgeKey(source, target);
        if (!source || !target || !key || runtimeEdgeKeys.has(key)) return;
        if (!graph.findById?.(source)) materializeNode(updateById.get(source));
        if (!graph.findById?.(target)) materializeNode(updateById.get(target));
        if (!graph.findById?.(source) || !graph.findById?.(target)) {
          skippedEdges += 1;
          return;
        }
        const dataEdge = edgeByPair.get(key) || {};
        const renderEdge = toNetworkSkeletonRenderEdge({
          ...dataEdge,
          id: dataEdge.id || update?.id || `network-plan-edge:${index}:${source}->${target}`,
          source,
          target,
          layoutEdgeTier: update?.layoutEdgeTier || dataEdge.layoutEdgeTier || "secondary",
          layoutCommunityPair: update?.layoutCommunityPair || dataEdge.layoutCommunityPair || "",
          layoutEdgeBundleId: update?.layoutEdgeBundleId || dataEdge.layoutEdgeBundleId || "",
          layoutEdgeBundleSize: update?.layoutEdgeBundleSize ?? dataEdge.layoutEdgeBundleSize,
          layoutEdgeBundleWeight: update?.layoutEdgeBundleWeight ?? dataEdge.layoutEdgeBundleWeight,
          layoutEdgeBundleRank: update?.layoutEdgeBundleRank ?? dataEdge.layoutEdgeBundleRank,
          layoutEdgeBundled: update?.layoutEdgeBundled ?? dataEdge.layoutEdgeBundled,
          layoutEdgeBundleRepresentative:
            update?.layoutEdgeBundleRepresentative ?? dataEdge.layoutEdgeBundleRepresentative,
          layoutEdgeBundleRouted: update?.layoutEdgeBundleRouted ?? dataEdge.layoutEdgeBundleRouted,
          edgeOffset: update?.edgeOffset ?? dataEdge.edgeOffset,
          edgeRoute: update?.edgeRoute || dataEdge.edgeRoute || "",
          orthAxis: update?.orthAxis || dataEdge.orthAxis || "",
          orthBias: update?.orthBias ?? dataEdge.orthBias,
          edgeLabelAnchorMode: update?.edgeLabelAnchorMode || dataEdge.edgeLabelAnchorMode || "",
          layoutHiddenBySkeleton: String(update?.layoutEdgeTier || dataEdge.layoutEdgeTier || "") === "hidden",
        });
        const item = graph.addItem?.("edge", renderEdge);
        if (!item) {
          skippedEdges += 1;
          return;
        }
        runtimeEdgeKeys.add(key);
        addedEdges += 1;
      });
      if (addedNodes || addedEdges) graph.refreshPositions?.();
    } finally {
      graph.setAutoPaint?.(true);
      if (addedNodes || addedEdges) graph.paint?.();
    }
    if (addedNodes || addedEdges || skippedEdges) {
      recordNetworkRuntimePerfEvent("runtime-skeleton-materialized", {
        reason: String(options?.reason || ""),
        mode,
        nodes: addedNodes,
        edges: addedEdges,
        skippedEdges,
        planNodes: updateRows.length,
        planEdges: Array.isArray(edgeUpdates) ? edgeUpdates.length : 0,
        runtimeNodes: Array.isArray(graph.getNodes?.()) ? graph.getNodes().length : 0,
        runtimeEdges: Array.isArray(graph.getEdges?.()) ? graph.getEdges().length : 0,
      });
    }
    return { nodes: addedNodes, edges: addedEdges };
  }

  function pruneNetworkRuntimeToPlanSkeleton(graph, options = {}) {
    // Network layout is coordinate-only; never prune runtime nodes or edges from a layout plan.
    return false;
    if (!graph || typeof graph.removeItem !== "function") return false;
    const normalizedMode = String(options?.mode || "").trim().toLowerCase();
    if (normalizedMode !== "xlarge" || options?.allowPrune !== true) return false;
    const nodeFilterIds = options?.nodeFilterIds instanceof Set ? options.nodeFilterIds : new Set();
    const edgeFilterIds = options?.edgeFilterIds instanceof Set ? options.edgeFilterIds : new Set();
    const edgeFilterPairs = options?.edgeFilterPairs instanceof Set ? options.edgeFilterPairs : new Set();
    if (!nodeFilterIds.size) return false;
    const nodeItems = Array.isArray(graph.getNodes?.()) ? graph.getNodes() : [];
    const edgeItems = Array.isArray(graph.getEdges?.()) ? graph.getEdges() : [];
    if (!nodeItems.length) return false;
    const plannedRuntimeNodeCount = nodeItems.filter((item) => {
      const id = String(item?.getModel?.()?.id || "").trim();
      return id && nodeFilterIds.has(id);
    }).length;
    const selectedEdgeItems = [];
    const fallbackEdgeItems = [];
    edgeItems.forEach((item) => {
      const model = item?.getModel?.() || {};
      const source = networkPlanEndpoint(model.source);
      const target = networkPlanEndpoint(model.target);
      if (!source || !target) return;
      const bothPlanned = nodeFilterIds.has(source) && nodeFilterIds.has(target);
      if (bothPlanned) fallbackEdgeItems.push(item);
      if (bothPlanned && networkRuntimeEdgeMatchesPlan(model, edgeFilterIds, edgeFilterPairs)) {
        selectedEdgeItems.push(item);
      }
    });
    const selectedEdgeFloor =
      normalizedMode === "large" ? Math.min(80, Math.ceil(fallbackEdgeItems.length * 0.65)) : 48;
    const selectedEdgeRatio = normalizedMode === "large" ? 0.65 : 0.5;
    const keptEdgeItems =
      selectedEdgeItems.length >= selectedEdgeFloor ||
      selectedEdgeItems.length >= fallbackEdgeItems.length * selectedEdgeRatio
        ? selectedEdgeItems
        : fallbackEdgeItems;
    const keepEdgeItems = new Set(keptEdgeItems);
    const keepNodeIds = new Set();
    keptEdgeItems.forEach((item) => {
      const model = item?.getModel?.() || {};
      const source = networkPlanEndpoint(model.source);
      const target = networkPlanEndpoint(model.target);
      if (source) keepNodeIds.add(source);
      if (target) keepNodeIds.add(target);
    });
    let backfilledEdges = 0;
    const requestedMinVisible = Math.max(0, Number(options?.minVisibleNodes) || 0);
    const baseMinVisible = Math.min(nodeItems.length, Math.max(96, Math.min(260, requestedMinVisible || 96)));
    const edgeAnchoredMinVisible = keepNodeIds.size
      ? normalizedMode === "xlarge"
        ? Math.min(baseMinVisible, Math.max(32, keepNodeIds.size))
        : Math.min(
            baseMinVisible,
            Math.max(32, keepNodeIds.size + Math.min(12, Math.ceil(keptEdgeItems.length * 0.18)))
          )
      : baseMinVisible;
    const minVisible =
      keptEdgeItems.length > 0 && (normalizedMode === "xlarge" || keptEdgeItems.length < 80)
        ? edgeAnchoredMinVisible
        : baseMinVisible;
    if (plannedRuntimeNodeCount < minVisible) return false;
    if (
      normalizedMode === "xlarge" &&
      keepNodeIds.size < minVisible &&
      keptEdgeItems.length > 0 &&
      typeof graph.addItem === "function"
    ) {
      const dataEdges = Array.isArray(options?.dataEdges)
        ? options.dataEdges
        : Array.isArray(state.graph?.data?.edges)
          ? state.graph.data.edges
          : [];
      const nodeRankById = new Map(
        nodeItems
          .map((item) => {
            const model = item?.getModel?.() || {};
            const id = String(model?.id || "").trim();
            return [id, rankNetworkCoarseNode(model)];
          })
          .filter(([id]) => !!id)
      );
      const existingEdgeKeys = new Set();
      edgeItems.forEach((item) => {
        const model = item?.getModel?.() || {};
        const id = String(model?.id || "").trim();
        const source = networkPlanEndpoint(model.source);
        const target = networkPlanEndpoint(model.target);
        const pair = networkEdgeKey(source, target);
        if (id) existingEdgeKeys.add(`id:${id}`);
        if (pair) existingEdgeKeys.add(`pair:${pair}`);
      });
      dataEdges
        .map((edge, index) => {
          const source = networkPlanEndpoint(edge?.source);
          const target = networkPlanEndpoint(edge?.target);
          if (!source || !target || source === target) return null;
          const sourceKept = keepNodeIds.has(source);
          const targetKept = keepNodeIds.has(target);
          if (sourceKept === targetKept) return null;
          const nextId = sourceKept ? target : source;
          if (!nodeFilterIds.has(nextId) || !graph.findById?.(nextId)) return null;
          const id = String(edge?.id || "").trim();
          const pair = networkEdgeKey(source, target);
          if ((id && existingEdgeKeys.has(`id:${id}`)) || (pair && existingEdgeKeys.has(`pair:${pair}`))) return null;
          const weight = Math.max(
            0,
            Number(edge?.weight) ||
              Number(edge?.amount) ||
              Number(edge?.total_amount) ||
              Number(edge?.totalAmount) ||
              Number(edge?.count) ||
              0
          );
          return {
            edge,
            index,
            id,
            pair,
            source,
            target,
            nextId,
            score: Math.log1p(weight) * 1400 + (nodeRankById.get(nextId) || 0) * 0.01,
          };
        })
        .filter(Boolean)
        .sort((left, right) => right.score - left.score || left.index - right.index)
        .some((row) => {
          if (keepNodeIds.size >= minVisible) return true;
          const renderEdge = toNetworkSkeletonRenderEdge({
            ...row.edge,
            id: row.id || row.edge?.id,
            source: row.source,
            target: row.target,
            layoutEdgeTier: row.edge?.layoutEdgeTier || "secondary",
            layoutHiddenBySkeleton: false,
          });
          const item = graph.addItem?.("edge", renderEdge);
          if (!item) return false;
          keepEdgeItems.add(item);
          keepNodeIds.add(row.source);
          keepNodeIds.add(row.target);
          if (row.id) existingEdgeKeys.add(`id:${row.id}`);
          if (row.pair) existingEdgeKeys.add(`pair:${row.pair}`);
          return false;
        });
    }
    if (normalizedMode !== "xlarge" && keepNodeIds.size < minVisible) {
      nodeItems
        .map((item, index) => {
          const model = item?.getModel?.() || {};
          return {
            item,
            index,
            id: String(model?.id || "").trim(),
            score: rankNetworkCoarseNode(model),
            primary: String(model?.layoutVisibilityTier || "") === "primary",
          };
        })
        .filter((row) => row.id && nodeFilterIds.has(row.id) && !keepNodeIds.has(row.id))
        .sort((left, right) => {
          if (left.primary !== right.primary) return left.primary ? -1 : 1;
          return right.score - left.score || left.id.localeCompare(right.id, "zh-CN") || left.index - right.index;
        })
        .some((row) => {
          if (keepNodeIds.size >= minVisible) return true;
          keepNodeIds.add(row.id);
          return false;
        });
    }
    if (keepNodeIds.size < Math.min(normalizedMode === "xlarge" ? 16 : 24, nodeItems.length)) return false;
    if (
      keptEdgeItems.length > 0 &&
      keptEdgeItems.length < 120 &&
      typeof graph.addItem === "function" &&
      keepNodeIds.size >= Math.min(normalizedMode === "xlarge" ? 32 : 48, nodeItems.length)
    ) {
      const dataEdges = Array.isArray(options?.dataEdges)
        ? options.dataEdges
        : Array.isArray(state.graph?.data?.edges)
          ? state.graph.data.edges
          : [];
      const nodeRankById = new Map(
        nodeItems
          .map((item) => {
            const model = item?.getModel?.() || {};
            const id = String(model?.id || "").trim();
            return [id, rankNetworkCoarseNode(model)];
          })
          .filter(([id]) => !!id)
      );
      const existingEdgeKeys = new Set();
      const endpointCounts = new Map();
      edgeItems.forEach((item) => {
        const model = item?.getModel?.() || {};
        const id = String(model?.id || "").trim();
        const source = networkPlanEndpoint(model.source);
        const target = networkPlanEndpoint(model.target);
        const pair = networkEdgeKey(source, target);
        if (id) existingEdgeKeys.add(`id:${id}`);
        if (pair) existingEdgeKeys.add(`pair:${pair}`);
      });
      keptEdgeItems.forEach((item) => {
        const model = item?.getModel?.() || {};
        const source = networkPlanEndpoint(model.source);
        const target = networkPlanEndpoint(model.target);
        if (source) endpointCounts.set(source, (endpointCounts.get(source) || 0) + 1);
        if (target) endpointCounts.set(target, (endpointCounts.get(target) || 0) + 1);
      });
      const targetEdgeCount = Math.min(
        resolveNetworkRuntimeEdgeBudget("xlarge"),
        Math.max(keptEdgeItems.length, Math.ceil(Math.min(keepNodeIds.size, minVisible) * 0.95))
      );
      const endpointLimit = Math.max(8, Math.min(24, Math.ceil(targetEdgeCount / 8)));
      const candidateRows = dataEdges
        .map((edge, index) => {
          const source = networkPlanEndpoint(edge?.source);
          const target = networkPlanEndpoint(edge?.target);
          if (!source || !target || source === target || !keepNodeIds.has(source) || !keepNodeIds.has(target)) return null;
          const id = String(edge?.id || "").trim();
          const pair = networkEdgeKey(source, target);
          if ((id && existingEdgeKeys.has(`id:${id}`)) || (pair && existingEdgeKeys.has(`pair:${pair}`))) return null;
          const weight = Math.max(
            0,
            Number(edge?.weight) ||
              Number(edge?.amount) ||
              Number(edge?.total_amount) ||
              Number(edge?.totalAmount) ||
              Number(edge?.count) ||
              0
          );
          const sourceDegree = endpointCounts.get(source) || 0;
          const targetDegree = endpointCounts.get(target) || 0;
          return {
            edge,
            index,
            id,
            pair,
            source,
            target,
            sourceDegree,
            targetDegree,
            score:
              (sourceDegree === 0 ? 900000 : 0) +
              (targetDegree === 0 ? 900000 : 0) +
              Math.log1p(weight) * 1200 +
              (nodeRankById.get(source) || 0) * 0.001 +
              (nodeRankById.get(target) || 0) * 0.001,
          };
        })
        .filter(Boolean)
        .sort((left, right) => right.score - left.score || right.index - left.index);
      candidateRows.some((row) => {
        if (keepEdgeItems.size >= targetEdgeCount) return true;
        if ((endpointCounts.get(row.source) || 0) >= endpointLimit && (endpointCounts.get(row.target) || 0) >= endpointLimit) {
          return false;
        }
        const renderEdge = toNetworkSkeletonRenderEdge({
          ...row.edge,
          id: row.id || row.edge?.id,
          source: row.source,
          target: row.target,
          layoutEdgeTier: row.edge?.layoutEdgeTier || "secondary",
          layoutHiddenBySkeleton: false,
        });
        const item = graph.addItem?.("edge", renderEdge);
        if (!item) return false;
        keepEdgeItems.add(item);
        if (row.id) existingEdgeKeys.add(`id:${row.id}`);
        if (row.pair) existingEdgeKeys.add(`pair:${row.pair}`);
        endpointCounts.set(row.source, (endpointCounts.get(row.source) || 0) + 1);
        endpointCounts.set(row.target, (endpointCounts.get(row.target) || 0) + 1);
        backfilledEdges += 1;
        return false;
      });
    }
    let lodPrunedNodes = 0;
    let lodPrunedEdges = 0;
    if (normalizedMode === "large" && keepNodeIds.size > 40 && keepEdgeItems.size > 0) {
      const degreeById = new Map();
      keepEdgeItems.forEach((item) => {
        const model = item?.getModel?.() || {};
        const source = networkPlanEndpoint(model.source);
        const target = networkPlanEndpoint(model.target);
        if (source) degreeById.set(source, (degreeById.get(source) || 0) + 1);
        if (target) degreeById.set(target, (degreeById.get(target) || 0) + 1);
      });
      const protectedIds = new Set(
        parseUniqueList(
          [
            state.focusId,
            ...(state.focusIds || []),
            ...(state.selected || []),
            ...parseUniqueList(state.graphProjection?.path_node_ids || state.graphProjection?.pathNodeIds || [], {
              allowString: true,
            }),
          ],
          { allowString: true }
        )
      );
      const removableRows = nodeItems
        .map((item, index) => {
          const model = item?.getModel?.() || {};
          const id = String(model?.id || "").trim();
          return {
            item,
            index,
            id,
            degree: degreeById.get(id) || 0,
            score: rankNetworkCoarseNode(model),
            tier: String(model.layoutVisibilityTier || ""),
            role: String(model.layoutCommunityRole || ""),
          };
        })
        .filter(
          (row) =>
            row.id &&
            keepNodeIds.has(row.id) &&
            !protectedIds.has(row.id) &&
            row.degree <= 4 &&
            row.role !== "community-anchor" &&
            row.role !== "bridge"
        )
        .sort((left, right) => {
          const leftTierPenalty = left.tier === "primary" ? 100000 : left.tier === "secondary" ? 1000 : 0;
          const rightTierPenalty = right.tier === "primary" ? 100000 : right.tier === "secondary" ? 1000 : 0;
          return (
            left.degree - right.degree ||
            leftTierPenalty - rightTierPenalty ||
            left.score - right.score ||
            right.index - left.index
          );
        });
      removableRows.some((row) => {
        if (keepNodeIds.size <= 40) return true;
        keepNodeIds.delete(row.id);
        lodPrunedNodes += 1;
        return false;
      });
      if (lodPrunedNodes) {
        Array.from(keepEdgeItems).forEach((item) => {
          const model = item?.getModel?.() || {};
          const source = networkPlanEndpoint(model.source);
          const target = networkPlanEndpoint(model.target);
          if (source && target && keepNodeIds.has(source) && keepNodeIds.has(target)) return;
          keepEdgeItems.delete(item);
          lodPrunedEdges += 1;
        });
      }
    }
    let removedNodes = 0;
    let removedEdges = 0;
    graph.setAutoPaint?.(false);
    try {
      edgeItems.forEach((item) => {
        const model = item?.getModel?.() || {};
        const source = networkPlanEndpoint(model.source);
        const target = networkPlanEndpoint(model.target);
        if (keepEdgeItems.has(item) && keepNodeIds.has(source) && keepNodeIds.has(target)) return;
        graph.removeItem?.(item);
        removedEdges += 1;
      });
      nodeItems.forEach((item) => {
        const id = String(item?.getModel?.()?.id || "").trim();
        if (id && keepNodeIds.has(id)) return;
        graph.removeItem?.(item);
        removedNodes += 1;
      });
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    if (removedNodes || removedEdges) {
      recordNetworkRuntimePerfEvent("skeleton-pruned", {
        reason: String(options?.reason || ""),
        mode: normalizedMode,
        nodes: keepNodeIds.size,
        edges: keepEdgeItems.size,
        minVisible,
        baseMinVisible,
        edgeAnchoredMinVisible,
        backfilledEdges,
        lodPrunedNodes,
        lodPrunedEdges,
        removedNodes,
        removedEdges,
      });
      updateMinimap();
      return true;
    }
    return false;
  }

  function networkPlanEndpointSample(edge) {
    const source = typeof edge?.source === "object" ? edge.source?.id : edge?.source;
    const target = typeof edge?.target === "object" ? edge.target?.id : edge?.target;
    return `${String(source || "").trim()}>${String(target || "").trim()}`;
  }

  function buildNetworkPlanGraphSignature(nodes = [], edges = []) {
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const edgeRows = Array.isArray(edges) ? edges : [];
    const sampleIndexes = (length) => {
      if (length <= 0) return [];
      return Array.from(new Set([0, Math.floor(length / 2), length - 1])).filter(
        (index) => index >= 0 && index < length
      );
    };
    const nodeSample = sampleIndexes(nodeRows.length)
      .map((index) => String(nodeRows[index]?.id || "").trim())
      .join("|");
    const edgeSample = sampleIndexes(edgeRows.length)
      .map((index) => networkPlanEndpointSample(edgeRows[index]))
      .join("|");
    return `${nodeRows.length}:${edgeRows.length}:${nodeSample}:${edgeSample}`;
  }

  function noteNetworkPlanDrop(stage = "", detail = {}) {
    if (!state.networkLayout || typeof state.networkLayout !== "object") return;
    state.networkLayout.lastPlanDrop = {
      stage: String(stage || ""),
      at: Date.now(),
      ...(detail && typeof detail === "object" ? detail : {}),
    };
  }

  function noteNetworkGraphDataReplacement(nextGraph = null, meta = {}) {
    const nodes = Array.isArray(nextGraph?.nodes) ? nextGraph.nodes : [];
    const edges = Array.isArray(nextGraph?.edges) ? nextGraph.edges : [];
    const signature = buildNetworkPlanGraphSignature(nodes, edges);
    const networkLayout = state.networkLayout;
    if (!networkLayout || typeof networkLayout !== "object") return;
    const lastSignature = String(networkLayout.lastPlanGraphSignature || "");
    if (!lastSignature || signature === lastSignature) return;
    const reason = String(meta?.reason || "");
    if (state.layoutPreset === "network" && reason.includes("projection-expand")) {
      networkLayout.lastPlanGraphSignature = signature;
      networkLayout.lastPlanDrop = null;
      return;
    }
    networkLayout.lastPlanMode = "";
    networkLayout.lastPlanQuality = null;
    networkLayout.lastPlanSampleQuality = null;
    networkLayout.lastPlanCommunityQuality = [];
    networkLayout.lastPlanLod = null;
    networkLayout.lastPlanSupergraph = null;
    networkLayout.topologyPlanCache = {};
    networkLayout.hiddenNodeCount = 0;
    networkLayout.hiddenEdgeCount = 0;
    networkLayout.lastViewportRefine = null;
    networkLayout.lastViewportQuality = null;
    networkLayout.lastCommunityQualityAttempt = null;
    networkLayout.lastCommunityQualityDrop = null;
    networkLayout.lastCommunityQualityStaleDrop = null;
    networkLayout.communityQualitySeq = Number(networkLayout.communityQualitySeq || 0) + 1;
    networkLayout.viewportRefineSignature = "";
    networkLayout.viewportRefineLastAt = 0;
    networkLayout.lastPlanDrop = {
      stage: "graph-replaced",
      reason: String(meta?.reason || ""),
      graphSignature: signature,
      previousGraphSignature: lastSignature,
      at: Date.now(),
    };
  }

  function cloneNetworkPlanLod(lod = null) {
    if (!lod || typeof lod !== "object") return null;
    try {
      return JSON.parse(JSON.stringify(lod));
    } catch (_error) {
      return {
        mode: String(lod.mode || ""),
        budgets: lod.budgets && typeof lod.budgets === "object" ? { ...lod.budgets } : {},
      };
    }
  }

  function cloneNetworkPlanSupergraph(supergraph = null) {
    if (!supergraph || typeof supergraph !== "object") return null;
    try {
      return JSON.parse(JSON.stringify(supergraph));
    } catch (_error) {
      return {
        mode: String(supergraph.mode || ""),
        nodeCount: Number(supergraph.nodeCount || 0) || 0,
        edgeCount: Number(supergraph.edgeCount || 0) || 0,
      };
    }
  }

  function resolveNetworkRuntimeEdgeBudget(mode = "") {
    const normalizedMode = String(mode || state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    const lod = state.networkLayout?.lastPlanLod && typeof state.networkLayout.lastPlanLod === "object" ? state.networkLayout.lastPlanLod : {};
    const budgets = lod.budgets && typeof lod.budgets === "object" ? lod.budgets : {};
    const raw = Number(budgets.runtimeEdgeBudget ?? lod.runtimeEdgeBudget);
    const fallback = normalizedMode === "xlarge" ? 640 : normalizedMode === "large" ? 900 : 0;
    const next = Number.isFinite(raw) && raw > 0 ? raw : fallback;
    return Math.max(160, Math.min(1200, next || 640));
  }

  function updateNetworkPlanVisibilityStats(networkPlan = {}, nodes = [], edges = []) {
    const quality = networkPlan?.quality && typeof networkPlan.quality === "object" ? networkPlan.quality : {};
    const visibleTierStats =
      quality.visibleTierStats && typeof quality.visibleTierStats === "object" ? quality.visibleTierStats : {};
    const lod = networkPlan?.lod && typeof networkPlan.lod === "object" ? networkPlan.lod : {};
    const clonedLod = cloneNetworkPlanLod(lod);
    if (clonedLod) state.networkLayout.lastPlanLod = clonedLod;
    const clonedSupergraph = cloneNetworkPlanSupergraph(networkPlan?.supergraph);
    if (clonedSupergraph) state.networkLayout.lastPlanSupergraph = clonedSupergraph;
    const nodeTiers = lod.nodeTiers && typeof lod.nodeTiers === "object" ? lod.nodeTiers : {};
    const edgeTiers = lod.edgeTiers && typeof lod.edgeTiers === "object" ? lod.edgeTiers : {};
    const firstFinite = (...values) => {
      for (const value of values) {
        const numeric = Number(value);
        if (Number.isFinite(numeric)) return numeric;
      }
      return 0;
    };
    const hiddenNodeCount = firstFinite(
      nodeTiers.hidden,
      visibleTierStats["node:hidden"],
      Array.isArray(nodes)
        ? nodes.filter((node) => String(node?.layoutVisibilityTier || "") === "hidden").length
        : 0
    );
    const hiddenEdgeCount = firstFinite(
      edgeTiers.hidden,
      visibleTierStats["edge:hidden"],
      Array.isArray(edges) ? edges.filter((edge) => !!edge?.layoutHiddenBySkeleton).length : 0
    );
    state.networkLayout.hiddenNodeCount = Math.max(0, Number(hiddenNodeCount) || 0);
    state.networkLayout.hiddenEdgeCount = Math.max(0, Number(hiddenEdgeCount) || 0);
  }

  function applyNetworkPlanPresentationToGraph(graph, nodes, edges, options = {}) {
    // Network layout plans do not own label, node, edge, or style presentation.
    return false;
    if (!graph) return false;
    const normalizedMode = String(options?.mode || state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    const preserveLabelFields = !isLargeNetworkTopologyMode(normalizedMode);
    const nodeFilterIds = options?.nodeFilterIds instanceof Set ? options.nodeFilterIds : null;
    const edgeFilterIds = options?.edgeFilterIds instanceof Set ? options.edgeFilterIds : null;
    const edgeFilterPairs = options?.edgeFilterPairs instanceof Set ? options.edgeFilterPairs : null;
    const nodeById = new Map((Array.isArray(nodes) ? nodes : []).map((node) => [String(node?.id || ""), node]));
    const edgeById = new Map((Array.isArray(edges) ? edges : []).map((edge) => [String(edge?.id || ""), edge]));
    const edgeByPair = new Map(
      (Array.isArray(edges) ? edges : []).map((edge) => {
        const source = typeof edge?.source === "object" ? edge.source?.id : edge?.source;
        const target = typeof edge?.target === "object" ? edge.target?.id : edge?.target;
        return [networkEdgeKey(source, target), edge];
      })
    );
    const nodeItems = nodeFilterIds
      ? Array.from(nodeFilterIds)
          .map((id) => graph.findById?.(id))
          .filter(Boolean)
      : graph.getNodes?.() || [];
    const edgeItems =
      edgeFilterIds || edgeFilterPairs
        ? (graph.getEdges?.() || []).filter((item) => {
            const model = item?.getModel?.() || {};
            const id = String(model?.id || "").trim();
            if (id && edgeFilterIds?.has(id)) return true;
            return !!edgeFilterPairs?.has(networkEdgeKey(model?.source, model?.target));
          })
        : graph.getEdges?.() || [];
    graph.setAutoPaint?.(false);
    try {
      nodeItems.forEach((item) => {
        const model = item?.getModel?.() || {};
        const node = nodeById.get(String(model?.id || ""));
        if (!node) return;
        const hidden = String(node.layoutVisibilityTier || "") === "hidden";
        const labelHidden = String(node.layoutLabelTier || "") === "hidden";
        if (!hidden && !labelHidden && !model.layoutHiddenBySkeleton && !model.nodeLabelHidden) return;
        if (model.__networkBaseR == null) model.__networkBaseR = Number(model.r != null ? model.r : node.r);
        if (model.__networkBaseLineWidth == null) {
          model.__networkBaseLineWidth = Number(model.lineWidth != null ? model.lineWidth : node.lineWidth);
        }
        if (model.__networkBaseStroke == null) model.__networkBaseStroke = String(model.stroke || node.stroke || "");
        if (model.__networkBaseFill == null) model.__networkBaseFill = String(model.fill || node.fill || "");
        if (model.__networkBaseNodeShadow == null) model.__networkBaseNodeShadow = !!model.nodeShadow;
        if (model.__networkBaseName == null) model.__networkBaseName = model.name ?? node.name ?? "";
        if (model.__networkBaseTitle == null) model.__networkBaseTitle = model.title ?? node.title ?? "";
        if (model.__networkBaseLabel == null) model.__networkBaseLabel = model.label ?? node.label ?? "";
        if (model.__networkBaseDisplayId == null) model.__networkBaseDisplayId = model.displayId ?? node.displayId ?? "";
        if (model.__networkBaseDisplayIdSnake == null) {
          model.__networkBaseDisplayIdSnake = model.display_id ?? node.display_id ?? "";
        }
        if (model.__networkBaseIcon == null) model.__networkBaseIcon = model.icon ?? node.icon ?? "";
        if (model.__networkBaseIconSymbol == null) model.__networkBaseIconSymbol = model.iconSymbol ?? node.iconSymbol ?? "";
        if (model.__networkBaseIconSize == null) model.__networkBaseIconSize = Number(model.iconSize ?? node.iconSize ?? 0) || 0;
        if (model.__networkBaseFontShadow == null) model.__networkBaseFontShadow = !!(model.fontShadow ?? node.fontShadow);
        const baseR = Number.isFinite(Number(model.__networkBaseR)) ? Number(model.__networkBaseR) : 18;
        const baseLineWidth = Number.isFinite(Number(model.__networkBaseLineWidth))
          ? Number(model.__networkBaseLineWidth)
          : 2;
        const communityRole = String(node.layoutCommunityRole || model.layoutCommunityRole || "").trim();
        const communityNodeCount = Math.max(0, Number(node.layoutCommunityNodeCount || model.layoutCommunityNodeCount) || 0);
        const roleRadiusBoost =
          !hidden && normalizedMode === "xlarge" && communityRole === "community-anchor"
            ? Math.min(10, Math.log1p(communityNodeCount || 1) * 1.15)
            : !hidden && normalizedMode === "xlarge" && communityRole === "bridge"
              ? 3.2
              : 0;
        const roleLineBoost =
          !hidden && normalizedMode === "xlarge" && communityRole === "community-anchor"
            ? 0.55
            : !hidden && normalizedMode === "xlarge" && communityRole === "bridge"
              ? 0.35
              : 0;
        const visibleRadius =
          normalizedMode === "xlarge" ? baseR + roleRadiusBoost + 1.2 : baseR + roleRadiusBoost;
        const visibleLineWidth =
          normalizedMode === "xlarge" ? Math.max(baseLineWidth + roleLineBoost, 2.55) : baseLineWidth + roleLineBoost;
        graph.updateItem?.(item, {
          r: hidden ? Math.max(1.4, Math.min(3.6, baseR * 0.18)) : visibleRadius,
          lineWidth: hidden ? Math.max(0.2, Math.min(0.45, baseLineWidth * 0.22)) : visibleLineWidth,
          stroke: hidden ? "rgba(100,116,139,0.055)" : String(model.__networkBaseStroke || model.stroke || ""),
          fill: hidden ? "rgba(148,163,184,0.018)" : String(model.__networkBaseFill || model.fill || ""),
          nodeShadow: hidden ? false : !!model.__networkBaseNodeShadow,
          layoutHiddenBySkeleton: hidden,
          nodeLabelHidden: labelHidden,
          layoutCommunity: node.layoutCommunity || "",
          layoutSuperNodeId: node.layoutSuperNodeId || "",
          layoutCommunityRole: communityRole,
          layoutCommunityNodeCount: communityNodeCount,
          layoutCommunityVisibleNodeCount: Math.max(
            0,
            Number(node.layoutCommunityVisibleNodeCount || model.layoutCommunityVisibleNodeCount) || 0
          ),
          layoutVisibilityTier: node.layoutVisibilityTier || "",
          layoutLabelTier: node.layoutLabelTier || "",
          layoutBridgeScore: Number(node.layoutBridgeScore) || 0,
          name: labelHidden && !preserveLabelFields ? "" : node.name ?? model.__networkBaseName,
          title: labelHidden && !preserveLabelFields ? "" : node.title ?? model.__networkBaseTitle,
          label: labelHidden && !preserveLabelFields ? "" : node.label ?? model.__networkBaseLabel,
          displayId: labelHidden && !preserveLabelFields ? "" : node.displayId ?? model.__networkBaseDisplayId,
          display_id: labelHidden && !preserveLabelFields ? "" : node.display_id ?? model.__networkBaseDisplayIdSnake,
          icon: labelHidden && !preserveLabelFields ? "" : node.icon ?? model.__networkBaseIcon,
          iconSymbol: labelHidden && !preserveLabelFields ? "" : node.iconSymbol ?? model.__networkBaseIconSymbol,
          iconSize: labelHidden && !preserveLabelFields ? 0 : Number(node.iconSize ?? model.__networkBaseIconSize) || 0,
          fontShadow: labelHidden && !preserveLabelFields ? false : !!model.__networkBaseFontShadow,
        });
      });
      edgeItems.forEach((item) => {
        const model = item?.getModel?.() || {};
        const edge =
          edgeById.get(String(model?.id || "")) ||
          edgeByPair.get(networkEdgeKey(model?.source, model?.target));
        if (!edge) return;
        const hidden = !!edge.layoutHiddenBySkeleton;
        if (!hidden && !model.layoutHiddenBySkeleton && !model.layoutEdgeTier) return;
        if (model.__networkBaseStroke == null) model.__networkBaseStroke = String(model.stroke || edge.stroke || "");
        if (model.__networkBaseLineWidth == null) {
          model.__networkBaseLineWidth = Number(model.lineWidth != null ? model.lineWidth : edge.lineWidth);
        }
        if (model.__networkBaseShowArrow == null) model.__networkBaseShowArrow = !!model.showArrow;
        const baseLineWidth = Number.isFinite(Number(model.__networkBaseLineWidth))
          ? Number(model.__networkBaseLineWidth)
          : 1.6;
        const bundleSize = Math.max(0, Number(edge.layoutEdgeBundleSize || model.layoutEdgeBundleSize) || 0);
        const bundleWeight = Math.max(0, Number(edge.layoutEdgeBundleWeight || model.layoutEdgeBundleWeight) || 0);
        const bundleBoost =
          bundleSize > 1 ? Math.min(3.2, Math.log1p(bundleSize) * 0.55 + Math.log1p(bundleWeight) * 0.035) : 0;
        const edgeTier = String(edge.layoutEdgeTier || model.layoutEdgeTier || "");
        const largeTopologyMode = isLargeNetworkTopologyMode(normalizedMode);
        const visibleMinLineWidth = largeTopologyMode ? (edgeTier === "primary" ? 0.9 : 0.72) : edgeTier === "primary" ? 1.08 : 0.82;
        const rawVisibleLineWidth =
          bundleBoost > 0
            ? Math.max(visibleMinLineWidth, baseLineWidth, Math.min(5.5, baseLineWidth + bundleBoost))
            : Math.max(visibleMinLineWidth, baseLineWidth);
        const visibleLineWidth = largeTopologyMode
          ? Math.min(normalizedMode === "large" ? 1.32 : 1.12, rawVisibleLineWidth)
          : rawVisibleLineWidth;
        const visibleStroke = largeTopologyMode
          ? edgeTier === "primary"
            ? "rgba(15,23,42,0.58)"
            : "rgba(30,41,59,0.42)"
          : String(model.__networkBaseStroke || edge.stroke || model.stroke || state.graphStyle?.edgeColor || "").trim() ||
            "rgba(30,41,59,0.34)";
        graph.updateItem?.(item, {
          stroke: hidden ? "rgba(100,116,139,0.035)" : visibleStroke,
          lineWidth: hidden ? Math.max(0.25, Math.min(0.7, baseLineWidth * 0.35)) : visibleLineWidth,
          showArrow: hidden ? false : largeTopologyMode ? false : !!model.__networkBaseShowArrow,
          layoutHiddenBySkeleton: hidden,
          layoutEdgeTier: edge.layoutEdgeTier || "",
          layoutCommunityPair: edge.layoutCommunityPair || "",
          layoutEdgeBundleId: edge.layoutEdgeBundleId || "",
          layoutEdgeBundleSize: bundleSize || 0,
          layoutEdgeBundleWeight: bundleWeight || 0,
          layoutEdgeBundleRank: Number(edge.layoutEdgeBundleRank || 0) || 0,
          layoutEdgeBundled: !!edge.layoutEdgeBundled,
          layoutEdgeBundleRepresentative: !!edge.layoutEdgeBundleRepresentative,
          layoutEdgeBundleRouted: !!edge.layoutEdgeBundleRouted,
          edgeOffset: Number(edge.edgeOffset || 0) || 0,
          edgeRoute: edge.edgeRoute || "",
          orthAxis: edge.orthAxis || "",
          orthBias: Number(edge.orthBias || 0) || 0,
          edgeLabelAnchorMode: edge.edgeLabelAnchorMode || "",
        });
      });
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    return true;
  }

  function isLargeNetworkTopologyMode(mode) {
    const value = String(mode || "").trim().toLowerCase();
    return value === "large" || value === "xlarge";
  }

  function resolveNetworkTopologyModeBySize(nodeCount = 0, edgeCount = 0) {
    const nodes = Math.max(0, Number(nodeCount) || 0);
    const edges = Math.max(0, Number(edgeCount) || 0);
    if (nodes > 5000 || edges > 15000) return "xlarge";
    if (nodes > 1200 || edges > 3500) return "large";
    if (nodes > 150 || edges > 400) return "medium";
    return "small";
  }

  function resolveNetworkTopologyModeForGraph(nodes = [], edges = [], ctx = {}) {
    const nodeCount = Array.isArray(nodes) ? nodes.length : Math.max(0, Number(nodes) || 0);
    const edgeCount = Array.isArray(edges) ? edges.length : Math.max(0, Number(edges) || 0);
    const preferCurrentGraph =
      ctx?.preferCurrentGraph !== false &&
      ctx?.useFullGraphCounts !== true &&
      ctx?.useFullSnapshotCounts !== true &&
      ctx?.useFullSnapshotForTopology !== true;
    if (preferCurrentGraph) return resolveNetworkTopologyModeBySize(nodeCount, edgeCount);
    const projection =
      ctx?.projection && typeof ctx.projection === "object"
        ? ctx.projection
        : ctx?.graphProjection && typeof ctx.graphProjection === "object"
        ? ctx.graphProjection
        : state.graphProjection && typeof state.graphProjection === "object"
        ? state.graphProjection
        : {};
    const projectedNodeCount = Math.max(
      0,
      Number(projection.full_node_count ?? projection.fullNodeCount ?? projection.visible_leaf_node_count ?? projection.visibleLeafNodeCount) || 0
    );
    const projectedEdgeCount = Math.max(
      0,
      Number(projection.full_edge_count ?? projection.fullEdgeCount) || 0
    );
    const snapshotRef =
      ctx?.resultSnapshotRef && typeof ctx.resultSnapshotRef === "object"
        ? ctx.resultSnapshotRef
        : state.resultSnapshotRef && typeof state.resultSnapshotRef === "object"
        ? state.resultSnapshotRef
        : state.graph?.data?.result_snapshot_ref && typeof state.graph.data.result_snapshot_ref === "object"
        ? state.graph.data.result_snapshot_ref
        : state.graph?.data?.resultSnapshotRef && typeof state.graph.data.resultSnapshotRef === "object"
        ? state.graph.data.resultSnapshotRef
        : {};
    const snapshotNodeCount = Math.max(
      0,
      Number(snapshotRef.node_count ?? snapshotRef.nodeCount ?? snapshotRef.full_node_count ?? snapshotRef.fullNodeCount) || 0
    );
    const snapshotEdgeCount = Math.max(
      0,
      Number(snapshotRef.edge_count ?? snapshotRef.edgeCount ?? snapshotRef.full_edge_count ?? snapshotRef.fullEdgeCount) || 0
    );
    const resolvedNodeCount = Math.max(nodeCount, projectedNodeCount, snapshotNodeCount);
    const resolvedEdgeCount = Math.max(edgeCount, projectedEdgeCount, snapshotEdgeCount);
    return resolveNetworkTopologyModeBySize(resolvedNodeCount, resolvedEdgeCount);
  }

  function rankNetworkCoarseNode(node) {
    const role = String(node?.role || node?.layoutBand || "").trim().toLowerCase();
    const roleRank = role === "core" ? 4 : role === "bridge" ? 3 : role === "adjacent" ? 2 : 1;
    const bridge = Number(node?.layoutBridgeScore) || 0;
    const amount = Math.log1p(
      Math.max(
        0,
        Number(node?.total_amount ?? node?.totalAmount ?? node?.amount ?? node?.weight ?? node?.total_count) || 0
      )
    );
    return roleRank * 100000 + bridge * 10000 + amount;
  }

  function networkCoarseStartedAt() {
    return typeof performance !== "undefined" && typeof performance.now === "function"
      ? performance.now()
      : Date.now();
  }

  function networkCoarseElapsedMs(startedAt) {
    const now =
      typeof performance !== "undefined" && typeof performance.now === "function"
        ? performance.now()
        : Date.now();
    return Math.max(0, Math.round((now - Number(startedAt || now)) * 100) / 100);
  }

  function compareNetworkCoarseNodeRows(left, right) {
    return right.score - left.score || left.id.localeCompare(right.id, "zh-CN") || left.index - right.index;
  }

  function compareNetworkCoarseBridgeRows(left, right) {
    return right.score - left.score || left.index - right.index;
  }

  function resolveNetworkSkeletonEdgeBudget(mode, nodeCount = 0, edgeCount = 0) {
    const normalizedMode = String(mode || "").trim().toLowerCase();
    const nodes = Math.max(0, Number(nodeCount) || 0);
    const edges = Math.max(0, Number(edgeCount) || 0);
    if (!edges) return 0;
    if (normalizedMode === "large") {
      return Math.min(edges, Math.max(240, Math.min(720, Math.ceil(nodes * 1.8))));
    }
    if (normalizedMode === "xlarge") {
      return Math.min(edges, Math.max(160, Math.min(256, Math.ceil(nodes * 0.75))));
    }
    return edges;
  }

  function pushNetworkCoarseBoundedRow(rows, row, limit, compareRows) {
    const maxRows = Math.max(0, Number(limit) || 0);
    if (!Array.isArray(rows) || !row || maxRows <= 0 || typeof compareRows !== "function") return false;
    if (rows.length >= maxRows && compareRows(row, rows[rows.length - 1]) >= 0) return false;
    let insertAt = rows.length;
    while (insertAt > 0 && compareRows(row, rows[insertAt - 1]) < 0) insertAt -= 1;
    rows.splice(insertAt, 0, row);
    if (rows.length > maxRows) rows.pop();
    return true;
  }

  function createNetworkCoarseCommunityKeyResolver(nodeRows = []) {
    let firstClusterId = "";
    let multipleClusterIds = false;
    (Array.isArray(nodeRows) ? nodeRows : []).some((node) => {
      const clusterId = String(node?.layoutClusterId || node?.clusterId || node?.layoutCommunity || "").trim();
      if (!clusterId) return false;
      if (!firstClusterId) {
        firstClusterId = clusterId;
        return false;
      }
      if (clusterId !== firstClusterId) {
        multipleClusterIds = true;
        return true;
      }
      return false;
    });
    return (node, index) => {
      const clusterId = String(node?.layoutClusterId || node?.clusterId || node?.layoutCommunity || "").trim();
      const hint = String(node?.clusterHint || "").trim();
      if (!multipleClusterIds && hint) return `hint:${hint}`;
      return clusterId || hint || `coarse:${Math.floor(index / 80)}`;
    };
  }

  function applyNetworkCoarseVisiblePlanState(nodes = [], edges = [], { reason = "", graphSignature = "" } = {}) {
    const startedAt = networkCoarseStartedAt();
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const edgeRows = Array.isArray(edges) ? edges : [];
    if (!nodeRows.length) return false;
    const mode = "xlarge";
    const primaryLimit = 16;
    const visibleLimit = Math.min(200, nodeRows.length);
    const baseVisibleLimit = Math.min(160, visibleLimit);
    const bridgeCandidateLimit = Math.max(visibleLimit * 4, 320);
    const communityKeyForNode = createNetworkCoarseCommunityKeyResolver(nodeRows);
    const ranked = [];
    const nodeEntryById = new Map();
    nodeRows.forEach((node, index) => {
      const id = String(node?.id || "").trim();
      node.layoutVisibilityTier = "hidden";
      node.layoutLabelTier = "hidden";
      node.layoutHiddenBySkeleton = true;
      node.nodeLabelHidden = true;
      if (!id) return;
      const row = {
        node,
        index,
        id,
        score: rankNetworkCoarseNode(node),
        communityKey: communityKeyForNode(node, index),
      };
      nodeEntryById.set(id, row);
      pushNetworkCoarseBoundedRow(ranked, row, visibleLimit, compareNetworkCoarseNodeRows);
    });
    const baseRows = ranked.slice(0, baseVisibleLimit);
    const visibleIds = new Set(baseRows.map((row) => row.id));
    const primaryIds = new Set(ranked.slice(0, Math.min(primaryLimit, baseVisibleLimit)).map((row) => row.id));
    const visibleByCommunity = new Map();
    baseRows.forEach((row) => {
      visibleByCommunity.set(row.communityKey, (visibleByCommunity.get(row.communityKey) || 0) + 1);
    });
    const edgeWeight = (edge) =>
      Math.max(
        0,
        Number(edge?.weight) ||
          Number(edge?.amount) ||
          Number(edge?.total_amount) ||
          Number(edge?.totalAmount) ||
          Number(edge?.count) ||
          0
      );
    const bridgeRows = [];
    edgeRows.forEach((edge, index) => {
      const source = networkPlanEndpoint(edge?.source);
      const target = networkPlanEndpoint(edge?.target);
      const sourceEntry = nodeEntryById.get(source);
      const targetEntry = nodeEntryById.get(target);
      if (!sourceEntry || !targetEntry || source === target) return;
      if (sourceEntry.communityKey === targetEntry.communityKey) return;
      const bridgeScore =
        (Number(sourceEntry.node?.layoutBridgeScore) || 0) + (Number(targetEntry.node?.layoutBridgeScore) || 0);
      pushNetworkCoarseBoundedRow(
        bridgeRows,
        {
          source,
          target,
          sourceCommunity: sourceEntry.communityKey,
          targetCommunity: targetEntry.communityKey,
          score: edgeWeight(edge) * (1 + bridgeScore * 0.85),
          index,
        },
        bridgeCandidateLimit,
        compareNetworkCoarseBridgeRows
      );
    });
    bridgeRows.forEach((row) => {
      if (visibleIds.size >= visibleLimit) return;
      [row.source, row.target].forEach((id) => {
        if (visibleIds.size >= visibleLimit || visibleIds.has(id)) return;
        const entry = nodeEntryById.get(id);
        if (!entry) return;
        if ((visibleByCommunity.get(entry.communityKey) || 0) >= 4) return;
        visibleIds.add(id);
        visibleByCommunity.set(entry.communityKey, (visibleByCommunity.get(entry.communityKey) || 0) + 1);
      });
    });
    const visibleRows = Array.from(visibleIds)
      .map((id) => nodeEntryById.get(id))
      .filter(Boolean)
      .sort(compareNetworkCoarseNodeRows);
    const communityRows = new Map();
    visibleRows.forEach((row) => {
      const community = communityRows.get(row.communityKey) || { key: row.communityKey, count: 0, score: 0 };
      community.count += 1;
      community.score += row.score;
      communityRows.set(row.communityKey, community);
    });
    const communities = Array.from(communityRows.values()).sort(
      (left, right) => right.score - left.score || right.count - left.count || left.key.localeCompare(right.key, "zh-CN")
    );
    const spacing = 560;
    const centers = new Map();
    communities.forEach((community, index) => {
      const jx = (hashProjectionPointSeed(`network-coarse-center-x|${community.key}`) / 0xffffffff - 0.5) * spacing * 0.16;
      const jy = (hashProjectionPointSeed(`network-coarse-center-y|${community.key}`) / 0xffffffff - 0.5) * spacing * 0.16;
      const angle = index * 2.399963229728653;
      const radius = 90 + Math.sqrt(index) * 96;
      centers.set(community.key, {
        x: Math.cos(angle) * radius + jx,
        y: Math.sin(angle) * radius * 0.72 + jy,
      });
    });
    const ordinalByCommunity = new Map();
    let primaryCount = 0;
    let secondaryCount = 0;
    visibleRows.forEach((row) => {
      const primary = primaryIds.has(row.id);
      row.node.layoutVisibilityTier = primary ? "primary" : "secondary";
      row.node.layoutLabelTier = primary ? "primary" : "hidden";
      row.node.layoutHiddenBySkeleton = false;
      row.node.nodeLabelHidden = !primary;
      if (primary) primaryCount += 1;
      else secondaryCount += 1;
      const ordinal = ordinalByCommunity.get(row.communityKey) || 0;
      ordinalByCommunity.set(row.communityKey, ordinal + 1);
      const center = centers.get(row.communityKey) || { x: 0, y: 0 };
      const angle =
        (hashProjectionPointSeed(`network-coarse-local|${row.communityKey}|${row.id || row.index}`) / 0xffffffff) *
        Math.PI *
        2;
      const localRadius = (primary ? 28 : 54) + Math.sqrt(ordinal) * (primary ? 22 : 28);
      row.node.x = center.x + Math.cos(angle) * localRadius;
      row.node.y = center.y + Math.sin(angle) * localRadius;
    });
    const visibleCount = visibleRows.length;
    const hiddenCount = Math.max(0, nodeRows.length - visibleCount);
    const visibleTierStats = {
      "node:primary": primaryCount,
      "node:secondary": secondaryCount,
      "node:hidden": hiddenCount,
      "label:primary": primaryCount,
      "label:hidden": Math.max(0, nodeRows.length - primaryCount),
    };
    const quality = {
      durationMs: networkCoarseElapsedMs(startedAt),
      nodeOverlapCount: 0,
      labelOverlapEstimate: 0,
      edgeCrossingSample: 0,
      edgeLengthStdDev: 0,
      clusterBBoxOverlapCount: 0,
      mode,
      qualityScope: "visible-skeleton",
      qualityNodeCount: visibleCount,
      qualityLabelCount: primaryCount,
      qualityEdgeCount: 0,
      visibleTierStats,
      source: "network-coarse-seed",
      coarseVisibleOnly: true,
      bridgeCandidateSampled: edgeRows.length > bridgeRows.length,
    };
    state.networkLayout.lastPlanMode = mode;
    state.networkLayout.lastPlanQuality = quality;
    state.networkLayout.lastPlanGraphSignature = String(graphSignature || buildNetworkPlanGraphSignature(nodeRows, edgeRows));
    state.networkLayout.hiddenNodeCount = hiddenCount;
    state.networkLayout.hiddenEdgeCount = 0;
    state.networkLayout.lastPlanDrop = null;
    if (state.networkLayout.lastPlanAttempt && typeof state.networkLayout.lastPlanAttempt === "object") {
      state.networkLayout.lastPlanAttempt = {
        ...state.networkLayout.lastPlanAttempt,
        coarseAppliedAt: Date.now(),
        coarseMode: mode,
      };
    }
    recordNetworkRuntimePerfEvent("coarse-seed-applied", {
      mode,
      reason: String(reason || ""),
      durationMs: quality.durationMs,
      nodes: visibleCount,
      totalNodes: nodeRows.length,
      edges: edgeRows.length,
      source: "network-coarse-visible",
    });
    const semanticState = ensureLayoutSemanticState();
    semanticState.lastReports = {
      ...(semanticState.lastReports || {}),
      networkPlan: {
        version: "network-coarse-seed-v1",
        mode,
        quality,
        report: {
          source: "network-coarse-seed",
          reason: String(reason || ""),
          coarseVisibleOnly: true,
        },
      },
    };
    renderNetworkCoarseSkeletonToGraph(nodeRows, edgeRows, mode, reason || "network-coarse-visible");
    scheduleNetworkViewportRefinement("network-coarse-seed", { delayMs: 180, force: true });
    return true;
  }

  function applyNetworkCoarsePlanState(nodes = [], edges = [], { reason = "", graphSignature = "" } = {}) {
    // Network layout is position-only; coarse skeleton display is intentionally disabled.
    return false;
    const startedAt = networkCoarseStartedAt();
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const edgeRows = Array.isArray(edges) ? edges : [];
    const mode = resolveNetworkTopologyModeForGraph(nodeRows, edgeRows);
    if (!isLargeNetworkTopologyMode(mode) || !nodeRows.length) return false;
    if (mode === "xlarge") {
      return applyNetworkCoarseVisiblePlanState(nodeRows, edgeRows, { reason, graphSignature });
    }
    const primaryLimit = 16;
    const visibleLimit = mode === "xlarge" ? Math.min(200, nodeRows.length) : Math.min(360, nodeRows.length);
    const baseVisibleLimit = mode === "xlarge" ? Math.min(160, visibleLimit) : Math.min(300, visibleLimit);
    const ranked = nodeRows
      .map((node, index) => ({ node, index, score: rankNetworkCoarseNode(node), id: String(node?.id || "") }))
      .sort((left, right) => right.score - left.score || left.id.localeCompare(right.id, "zh-CN"));
    const clusterIds = new Set(
      nodeRows
        .map((node) => String(node?.layoutClusterId || node?.clusterId || node?.layoutCommunity || "").trim())
        .filter(Boolean)
    );
    const communityKeyForNode = (node, index) => {
      const clusterId = String(node?.layoutClusterId || node?.clusterId || node?.layoutCommunity || "").trim();
      const hint = String(node?.clusterHint || "").trim();
      if (clusterIds.size <= 1 && hint) return `hint:${hint}`;
      return clusterId || hint || `coarse:${Math.floor(index / 80)}`;
    };
    const nodeEntryById = new Map(
      nodeRows
        .map((node, index) => [String(node?.id || "").trim(), { node, index }])
        .filter(([id]) => !!id)
    );
    const visibleIds = new Set(ranked.slice(0, baseVisibleLimit).map((row) => row.id));
    const primaryIds = new Set(ranked.slice(0, Math.min(primaryLimit, baseVisibleLimit)).map((row) => row.id));
    const visibleByCommunity = new Map();
    const countVisibleCommunity = (id) => {
      const entry = nodeEntryById.get(String(id || "").trim());
      if (!entry) return "";
      const community = communityKeyForNode(entry.node, entry.index);
      visibleByCommunity.set(community, (visibleByCommunity.get(community) || 0) + 1);
      return community;
    };
    visibleIds.forEach((id) => countVisibleCommunity(id));
    const edgeWeight = (edge) =>
      Math.max(
        0,
        Number(edge?.weight) ||
          Number(edge?.amount) ||
          Number(edge?.total_amount) ||
          Number(edge?.totalAmount) ||
          Number(edge?.count) ||
          0
      );
    edgeRows
      .map((edge, index) => {
        const source = networkPlanEndpoint(edge?.source);
        const target = networkPlanEndpoint(edge?.target);
        const sourceEntry = nodeEntryById.get(source);
        const targetEntry = nodeEntryById.get(target);
        if (!sourceEntry || !targetEntry || source === target) return null;
        const sourceCommunity = communityKeyForNode(sourceEntry.node, sourceEntry.index);
        const targetCommunity = communityKeyForNode(targetEntry.node, targetEntry.index);
        if (!sourceCommunity || sourceCommunity === targetCommunity) return null;
        const bridgeScore =
          (Number(sourceEntry.node?.layoutBridgeScore) || 0) + (Number(targetEntry.node?.layoutBridgeScore) || 0);
        return {
          source,
          target,
          sourceCommunity,
          targetCommunity,
          score: edgeWeight(edge) * (1 + bridgeScore * 0.85),
          index,
        };
      })
      .filter(Boolean)
      .sort((left, right) => right.score - left.score || left.index - right.index)
      .forEach((row) => {
        if (visibleIds.size >= visibleLimit) return;
        [row.source, row.target].forEach((id) => {
          if (visibleIds.size >= visibleLimit || visibleIds.has(id)) return;
          const entry = nodeEntryById.get(id);
          if (!entry) return;
          const community = communityKeyForNode(entry.node, entry.index);
          const cap = mode === "xlarge" ? 4 : 8;
          if ((visibleByCommunity.get(community) || 0) >= cap) return;
          visibleIds.add(id);
          visibleByCommunity.set(community, (visibleByCommunity.get(community) || 0) + 1);
        });
      });
    const communityRows = new Map();
    nodeRows.forEach((node, index) => {
      const key = communityKeyForNode(node, index);
      const row = communityRows.get(key) || { key, count: 0, score: 0 };
      row.count += 1;
      row.score += rankNetworkCoarseNode(node);
      communityRows.set(key, row);
    });
    const communities = Array.from(communityRows.values()).sort(
      (left, right) => right.score - left.score || right.count - left.count || left.key.localeCompare(right.key, "zh-CN")
    );
    const cols = Math.max(1, Math.ceil(Math.sqrt(Math.max(1, communities.length))));
    const rows = Math.max(1, Math.ceil(Math.max(1, communities.length) / cols));
    const spacing = mode === "xlarge" ? 560 : 660;
    const centers = new Map();
    communities.forEach((community, index) => {
      const jx = (hashProjectionPointSeed(`network-coarse-center-x|${community.key}`) / 0xffffffff - 0.5) * spacing * 0.16;
      const jy = (hashProjectionPointSeed(`network-coarse-center-y|${community.key}`) / 0xffffffff - 0.5) * spacing * 0.16;
      if (mode === "xlarge") {
        const angle = index * 2.399963229728653;
        const radius = 90 + Math.sqrt(index) * 96;
        centers.set(community.key, {
          x: Math.cos(angle) * radius + jx,
          y: Math.sin(angle) * radius * 0.72 + jy,
        });
      } else {
        const col = index % cols;
        const row = Math.floor(index / cols);
        centers.set(community.key, {
          x: (col - (cols - 1) / 2) * spacing + jx,
          y: (row - (rows - 1) / 2) * spacing + jy,
        });
      }
    });
    const ordinalByCommunity = new Map();
    const visibleTierStats = {};
    nodeRows.forEach((node, index) => {
      const id = String(node?.id || "");
      const visible = visibleIds.has(id);
      const primary = primaryIds.has(id);
      node.layoutVisibilityTier = visible ? (primary ? "primary" : "secondary") : "hidden";
      node.layoutLabelTier = primary ? "primary" : "hidden";
      node.layoutHiddenBySkeleton = !visible;
      node.nodeLabelHidden = !primary;
      const communityKey = communityKeyForNode(node, index);
      const ordinal = ordinalByCommunity.get(communityKey) || 0;
      ordinalByCommunity.set(communityKey, ordinal + 1);
      const center = centers.get(communityKey) || { x: 0, y: 0 };
      const angle =
        (hashProjectionPointSeed(`network-coarse-local|${communityKey}|${id || index}`) / 0xffffffff) * Math.PI * 2;
      const localRadius = visible
        ? (primary ? 28 : 54) + Math.sqrt(ordinal) * (primary ? 22 : 28)
        : 96 + Math.sqrt(ordinal) * 22;
      node.x = center.x + Math.cos(angle) * localRadius;
      node.y = center.y + Math.sin(angle) * localRadius;
      const tier = String(node.layoutVisibilityTier || "hidden");
      const label = String(node.layoutLabelTier || "hidden");
      visibleTierStats[`node:${tier}`] = (visibleTierStats[`node:${tier}`] || 0) + 1;
      visibleTierStats[`label:${label}`] = (visibleTierStats[`label:${label}`] || 0) + 1;
    });
    const quality = {
      durationMs: networkCoarseElapsedMs(startedAt),
      nodeOverlapCount: 0,
      labelOverlapEstimate: 0,
      edgeCrossingSample: 0,
      edgeLengthStdDev: 0,
      clusterBBoxOverlapCount: 0,
      mode,
      qualityScope: "visible-skeleton",
      qualityNodeCount: visibleIds.size,
      qualityLabelCount: Math.min(primaryLimit, visibleIds.size),
      qualityEdgeCount: 0,
      visibleTierStats,
      source: "network-coarse-seed",
    };
    state.networkLayout.lastPlanMode = mode;
    state.networkLayout.lastPlanQuality = quality;
    state.networkLayout.lastPlanGraphSignature = String(graphSignature || buildNetworkPlanGraphSignature(nodeRows, edgeRows));
    state.networkLayout.hiddenNodeCount = Math.max(0, nodeRows.length - visibleIds.size);
    state.networkLayout.hiddenEdgeCount = 0;
    state.networkLayout.lastPlanDrop = null;
    if (state.networkLayout.lastPlanAttempt && typeof state.networkLayout.lastPlanAttempt === "object") {
      state.networkLayout.lastPlanAttempt = {
        ...state.networkLayout.lastPlanAttempt,
        coarseAppliedAt: Date.now(),
        coarseMode: mode,
      };
    }
    recordNetworkRuntimePerfEvent("coarse-seed-applied", {
      mode,
      reason: String(reason || ""),
      durationMs: quality.durationMs,
      nodes: visibleIds.size,
      totalNodes: nodeRows.length,
      edges: edgeRows.length,
      source: "network-coarse-seed",
    });
    const semanticState = ensureLayoutSemanticState();
    semanticState.lastReports = {
      ...(semanticState.lastReports || {}),
      networkPlan: {
        version: "network-coarse-seed-v1",
        mode,
        quality,
        report: { source: "network-coarse-seed", reason: String(reason || "") },
      },
    };
    scheduleNetworkViewportRefinement("network-coarse-seed", { delayMs: 180, force: true });
    return true;
  }

  function shouldSuppressDenseNetworkEdgeLabels(mode, nodeCount = 0, edgeCount = 0) {
    return false;
  }

  function selectDenseNetworkStrongEdgeIndexes(rows = [], { mode = "", nodeCount = 0, edgeCount = 0 } = {}) {
    const denseRows = Array.isArray(rows) ? rows : [];
    if (!denseRows.length) return new Set();
    const normalizedMode = String(mode || "").trim().toLowerCase();
    const nodes = Math.max(1, Number(nodeCount) || 0);
    const edges = Math.max(1, Number(edgeCount) || denseRows.length);
    const strongLimit = Math.min(
      denseRows.length,
      normalizedMode === "small"
        ? Math.max(28, Math.min(64, Math.ceil(nodes * 1.15), Math.ceil(edges * 0.12)))
        : Math.max(72, Math.min(140, Math.ceil(nodes * 0.78), Math.ceil(edges * 0.08)))
    );
    const strongIndexes = new Set();
    const strongDegree = new Map();
    const takeStrong = (row) => {
      if (!row || !Number.isFinite(Number(row.index)) || strongIndexes.has(row.index) || strongIndexes.size >= strongLimit) {
        return false;
      }
      strongIndexes.add(row.index);
      if (row.source) strongDegree.set(row.source, (strongDegree.get(row.source) || 0) + 1);
      if (row.target) strongDegree.set(row.target, (strongDegree.get(row.target) || 0) + 1);
      return true;
    };
    denseRows.forEach((row) => {
      if (strongIndexes.size >= strongLimit || !row.source || !row.target || row.source === row.target) return;
      if ((strongDegree.get(row.source) || 0) === 0 || (strongDegree.get(row.target) || 0) === 0) {
        takeStrong(row);
      }
    });
    denseRows.forEach((row) => {
      if (strongIndexes.size >= strongLimit) return;
      takeStrong(row);
    });
    return strongIndexes;
  }

  function applyDenseNetworkRenderLod(nodes = [], edges = [], { mode = "" } = {}) {
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const edgeRows = Array.isArray(edges) ? edges : [];
    if (!edgeRows.length || !shouldSuppressDenseNetworkEdgeLabels(mode, nodeRows.length, edgeRows.length)) {
      return { edgeLabelsSuppressed: 0 };
    }
    const denseRows = edgeRows
      .map((edge, index) => {
        const source = networkPlanEndpoint(edge?.source);
        const target = networkPlanEndpoint(edge?.target);
        const weight = Math.max(
          0,
          Number(edge?.weight) ||
            Number(edge?.amount) ||
            Number(edge?.total_amount) ||
            Number(edge?.totalAmount) ||
            Number(edge?.count) ||
            0
        );
        return { index, source, target, weight };
      })
      .sort((left, right) => right.weight - left.weight || left.index - right.index);
    const strongIndexes = selectDenseNetworkStrongEdgeIndexes(denseRows, {
      mode,
      nodeCount: nodeRows.length,
      edgeCount: edgeRows.length,
    });
    let edgeLabelsSuppressed = 0;
    edgeRows.forEach((edge, index) => {
      if (!edge || typeof edge !== "object") return;
      if (edge.__networkDenseBaseStroke == null) edge.__networkDenseBaseStroke = String(edge.stroke || "");
      if (edge.__networkDenseBaseLineWidth == null) edge.__networkDenseBaseLineWidth = Number(edge.lineWidth || 1.4);
      if (edge.__networkDenseBaseShowArrow == null) edge.__networkDenseBaseShowArrow = !!edge.showArrow;
      if (edge.__networkDenseBaseLabel == null) {
        edge.__networkDenseBaseLabel = edge.label ?? "";
        edge.__networkDenseBaseLabelTop = edge.labelTop ?? "";
        edge.__networkDenseBaseLabelBottom = edge.labelBottom ?? "";
      }
      const baseLineWidth = Number.isFinite(Number(edge.__networkDenseBaseLineWidth))
        ? Number(edge.__networkDenseBaseLineWidth)
        : 1.4;
      const strong = strongIndexes.has(index);
      const targetStroke = strong ? "rgba(30,41,59,0.34)" : "rgba(71,85,105,0.16)";
      const targetLineWidth = strong ? Math.max(0.9, Math.min(1.18, baseLineWidth * 0.62)) : 0.34;
      const targetStyle = strong ? "strong" : "weak";
      if (
        edge.edgeLabelHidden === true &&
        String(edge.layoutEdgeLabelTier || "") === "hidden" &&
        edge.__networkDenseStyled === targetStyle &&
        String(edge.stroke || "") === targetStroke &&
        Math.abs(Number(edge.lineWidth || 0) - targetLineWidth) < 0.01 &&
        edge.showArrow === false &&
        !edge.label &&
        !edge.labelTop &&
        !edge.labelBottom
      ) {
        return;
      }
      Object.assign(edge, {
        label: "",
        labelTop: "",
        labelBottom: "",
        label_top: "",
        label_bottom: "",
        detailLabel: false,
        edgeLabelHidden: true,
        layoutEdgeLabelTier: "hidden",
        stroke: targetStroke,
        lineWidth: targetLineWidth,
        showArrow: false,
        __networkDenseStyled: targetStyle,
      });
      edgeLabelsSuppressed += 1;
    });
    return { edgeLabelsSuppressed };
  }

  function expandDenseNetworkNodePositions(nodes = [], { mode = "", edgeCount = 0 } = {}) {
    const rows = Array.isArray(nodes) ? nodes : [];
    if (!rows.length || !shouldSuppressDenseNetworkEdgeLabels(mode, rows.length, edgeCount)) return 1;
    const finiteRows = rows.filter((node) => Number.isFinite(Number(node?.x)) && Number.isFinite(Number(node?.y)));
    if (finiteRows.length < 2) return 1;
    const density = Math.max(0, Number(edgeCount) || 0) / Math.max(1, rows.length);
    const factor = Math.max(1.18, Math.min(2.45, 1 + Math.max(0, density - 2.4) * 0.18));
    const center = finiteRows.reduce(
      (acc, node) => {
        acc.x += Number(node.x) || 0;
        acc.y += Number(node.y) || 0;
        return acc;
      },
      { x: 0, y: 0 }
    );
    center.x /= finiteRows.length;
    center.y /= finiteRows.length;
    finiteRows.forEach((node) => {
      node.x = center.x + (Number(node.x) - center.x) * factor;
      node.y = center.y + (Number(node.y) - center.y) * factor;
    });
    return Math.round(factor * 100) / 100;
  }

  function suppressDenseNetworkRuntimeNodeLabels(graph, { mode = "", nodeCount = 0, edgeCount = 0 } = {}) {
    if (!graph || !shouldSuppressDenseNetworkEdgeLabels(mode, nodeCount, edgeCount)) return 0;
    const items = Array.isArray(graph.getNodes?.()) ? graph.getNodes() : [];
    if (!items.length) return 0;
    const maxLabels = String(mode || "").trim().toLowerCase() === "small" ? 14 : 28;
    const rows = items
      .map((item) => ({ item, model: item?.getModel?.() || {} }))
      .filter(({ model }) => {
        if (model.layoutHiddenBySkeleton || String(model.layoutVisibilityTier || "") === "hidden") return false;
        return Number.isFinite(Number(model.x)) && Number.isFinite(Number(model.y));
      })
      .sort((left, right) => {
        const score = networkRuntimeLabelPriority(right.model) - networkRuntimeLabelPriority(left.model);
        if (score) return score;
        return String(left.model.id || "").localeCompare(String(right.model.id || ""), "zh-CN");
      });
    const accepted = [];
    let suppressed = 0;
    graph.setAutoPaint?.(false);
    try {
      rows.forEach(({ item, model }) => {
        const box = networkRuntimeLabelBox(model, 12);
        const collides = accepted.some((acceptedBox) => networkRuntimeBoxesOverlap(box, acceptedBox));
        if (accepted.length >= maxLabels || collides) {
          graph.updateItem?.(item, {
            layoutLabelTier: "hidden",
            nodeLabelHidden: true,
          });
          graph.refreshItem?.(item);
          suppressed += 1;
        } else if (box) {
          accepted.push(box);
          graph.updateItem?.(item, {
            layoutLabelTier: String(model.layoutLabelTier || "primary"),
            nodeLabelHidden: false,
          });
          graph.refreshItem?.(item);
        }
      });
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    return suppressed;
  }

  function suppressDenseNetworkRuntimeEdgeLabels(graph, { mode = "", nodeCount = 0, edgeCount = 0 } = {}) {
    if (!graph || !shouldSuppressDenseNetworkEdgeLabels(mode, nodeCount, edgeCount)) return 0;
    const edgeItems = graph.getEdges?.() || [];
    if (!edgeItems.length) return 0;
    const dataEdges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    const dataEdgeById = new Map(
      dataEdges
        .map((edge) => [String(edge?.id || "").trim(), edge])
        .filter(([id]) => id)
    );
    const dataEdgeByPair = new Map(
      dataEdges
        .map((edge) => [networkEdgeKey(edge?.source, edge?.target), edge])
        .filter(([key]) => key && key !== "->")
    );
    const denseRows = edgeItems
      .map((item, index) => {
        const model = item?.getModel?.() || {};
        const source = networkPlanEndpoint(model?.source);
        const target = networkPlanEndpoint(model?.target);
        const weight = Math.max(
          0,
          Number(model.weight) ||
            Number(model.amount) ||
            Number(model.total_amount) ||
            Number(model.totalAmount) ||
            Number(model.count) ||
            0
        );
        return { item, model, index, source, target, weight };
      })
      .sort((left, right) => right.weight - left.weight || left.index - right.index);
    const normalizedMode = String(mode || "").trim().toLowerCase();
    const nodes = Math.max(1, Number(nodeCount) || 0);
    const strongIndexes = selectDenseNetworkStrongEdgeIndexes(denseRows, {
      mode: normalizedMode,
      nodeCount: nodes,
      edgeCount: edgeItems.length,
    });
    let suppressed = 0;
    graph.setAutoPaint?.(false);
    try {
      edgeItems.forEach((item, index) => {
        const model = item?.getModel?.() || {};
        if (model.__networkDenseBaseStroke == null) model.__networkDenseBaseStroke = String(model.stroke || "");
        if (model.__networkDenseBaseLineWidth == null) model.__networkDenseBaseLineWidth = Number(model.lineWidth || 1.4);
        if (model.__networkDenseBaseShowArrow == null) model.__networkDenseBaseShowArrow = !!model.showArrow;
        if (model.__networkDenseBaseLabel == null) {
          model.__networkDenseBaseLabel = model.label ?? "";
          model.__networkDenseBaseLabelTop = model.labelTop ?? "";
          model.__networkDenseBaseLabelBottom = model.labelBottom ?? "";
        }
        const strong = strongIndexes.has(index);
        const baseLineWidth = Number.isFinite(Number(model.__networkDenseBaseLineWidth))
          ? Number(model.__networkDenseBaseLineWidth)
          : 1.4;
        const targetStroke = strong ? "rgba(30,41,59,0.34)" : "rgba(71,85,105,0.16)";
        const targetLineWidth = strong ? Math.max(0.9, Math.min(1.18, baseLineWidth * 0.62)) : 0.34;
        const targetStyle = strong ? "strong" : "weak";
        const currentLineWidth = Number(model.lineWidth);
        if (
          model.edgeLabelHidden === true &&
          String(model.layoutEdgeLabelTier || "") === "hidden" &&
          model.__networkDenseStyled === targetStyle &&
          String(model.stroke || "") === targetStroke &&
          Number.isFinite(currentLineWidth) &&
          Math.abs(currentLineWidth - targetLineWidth) < 0.01 &&
          model.showArrow === false &&
          !model.label &&
          !model.labelTop &&
          !model.labelBottom
        ) {
          return;
        }
        const update = {
          label: "",
          labelTop: "",
          labelBottom: "",
          detailLabel: false,
          edgeLabelHidden: true,
          layoutEdgeLabelTier: "hidden",
          stroke: targetStroke,
          lineWidth: targetLineWidth,
          showArrow: false,
          __networkDenseStyled: targetStyle,
        };
        graph.updateItem?.(item, update);
        const dataEdge =
          dataEdgeById.get(String(model?.id || "").trim()) ||
          dataEdgeByPair.get(networkEdgeKey(model?.source, model?.target));
        if (dataEdge) {
          if (dataEdge.__networkDenseBaseStroke == null) dataEdge.__networkDenseBaseStroke = String(dataEdge.stroke || "");
          if (dataEdge.__networkDenseBaseLineWidth == null) dataEdge.__networkDenseBaseLineWidth = Number(dataEdge.lineWidth || 1.4);
          if (dataEdge.__networkDenseBaseShowArrow == null) dataEdge.__networkDenseBaseShowArrow = !!dataEdge.showArrow;
          if (dataEdge.__networkDenseBaseLabel == null) {
            dataEdge.__networkDenseBaseLabel = dataEdge.label ?? "";
            dataEdge.__networkDenseBaseLabelTop = dataEdge.labelTop ?? "";
            dataEdge.__networkDenseBaseLabelBottom = dataEdge.labelBottom ?? "";
          }
          Object.assign(dataEdge, update);
        }
        graph.refreshItem?.(item);
        suppressed += 1;
      });
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    return suppressed;
  }

  function applyDenseNetworkRuntimeLod(graph, { mode = "", nodeCount = 0, edgeCount = 0 } = {}) {
    if (networkDenseHoverEdgeReveal?.rows?.length) {
      restoreNetworkDenseHoverEdges(graph, "dense-lod-refresh");
    }
    if (!graph || !shouldSuppressDenseNetworkEdgeLabels(mode, nodeCount, edgeCount)) {
      return { edgeLabelsSuppressed: 0, nodeLabelsSuppressed: 0 };
    }
    const edgeLabelsSuppressed = suppressDenseNetworkRuntimeEdgeLabels(graph, { mode, nodeCount, edgeCount });
    const nodeLabelsSuppressed = suppressDenseNetworkRuntimeNodeLabels(graph, { mode, nodeCount, edgeCount });
    return { edgeLabelsSuppressed, nodeLabelsSuppressed };
  }

  function networkDenseHoverEdgeWeight(model) {
    return Math.max(
      0,
      Number(model?.weight) ||
        Number(model?.amount) ||
        Number(model?.total_amount) ||
        Number(model?.totalAmount) ||
        Number(model?.count) ||
        0
    );
  }

  function captureNetworkDenseHoverEdgeVisual(model = {}) {
    const keys = [
      "label",
      "labelTop",
      "labelBottom",
      "detailLabel",
      "edgeLabelHidden",
      "layoutEdgeLabelTier",
      "layoutHiddenBySkeleton",
      "layoutEdgeTier",
      "stroke",
      "lineWidth",
      "showArrow",
      "__networkDenseStyled",
      "__networkDenseHoverReveal",
    ];
    const values = {};
    const absentKeys = [];
    keys.forEach((key) => {
      if (Object.prototype.hasOwnProperty.call(model, key)) values[key] = model[key];
      else absentKeys.push(key);
    });
    return { values, absentKeys };
  }

  function restoreNetworkDenseHoverEdgeVisual(target, snapshot) {
    if (!target || !snapshot) return {};
    (Array.isArray(snapshot.absentKeys) ? snapshot.absentKeys : []).forEach((key) => {
      try {
        delete target[key];
      } catch (_) {
        // Some graph internals may expose sealed model fields.
      }
    });
    const values = snapshot.values && typeof snapshot.values === "object" ? snapshot.values : {};
    Object.assign(target, values);
    return {
      ...values,
      __networkDenseHoverReveal: values.__networkDenseHoverReveal === true,
    };
  }

  function restoreNetworkDenseHoverEdges(graph, reason = "") {
    const activeRows = Array.isArray(networkDenseHoverEdgeReveal?.rows)
      ? networkDenseHoverEdgeReveal.rows
      : [];
    if (!activeRows.length) return 0;
    const targetGraph = graph || networkDenseHoverEdgeReveal.graph;
    let restored = 0;
    targetGraph?.setAutoPaint?.(false);
    try {
      activeRows.forEach((row) => {
        try {
          if (row?.dataEdge && row.dataVisual) {
            restoreNetworkDenseHoverEdgeVisual(row.dataEdge, row.dataVisual);
          }
          if (!targetGraph || !row?.item?.getModel) return;
          const model = row.item.getModel?.() || {};
          const update = restoreNetworkDenseHoverEdgeVisual(model, row.visual);
          targetGraph.updateItem?.(row.item, update);
          targetGraph.refreshItem?.(row.item);
          restored += 1;
        } catch (_) {
          // The graph can be rebuilt while a hover reveal is active.
        }
      });
    } finally {
      targetGraph?.setAutoPaint?.(true);
      targetGraph?.paint?.();
      networkDenseHoverEdgeReveal = { graph: null, rows: [] };
    }
    if (state.networkLayout) {
      state.networkLayout.lastDenseHoverRevealRestore = {
        reason: String(reason || ""),
        edges: restored,
        at: Date.now(),
      };
    }
    return restored;
  }

  function revealNetworkDenseIncidentEdgesForHover(graph, nodeItem, reason = "", options = {}) {
    if (networkDenseHoverEdgeReveal?.rows?.length) restoreNetworkDenseHoverEdges(graph, "replace-hover");
    if (!graph || state.layoutPreset !== "network" || (isPerfMode() && options?.allowPerfMode !== true)) return 0;
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    const nodeCount = graph.getNodes?.()?.length || state.graph?.data?.nodes?.length || 0;
    const edgeCount = graph.getEdges?.()?.length || state.graph?.data?.edges?.length || 0;
    if (!shouldSuppressDenseNetworkEdgeLabels(mode, nodeCount, edgeCount)) return 0;
    const nodeId = networkPlanEndpoint(nodeItem?.getModel?.()?.id);
    if (!nodeId) return 0;
    const dataEdges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    const dataEdgeById = new Map(
      dataEdges
        .map((edge) => [String(edge?.id || "").trim(), edge])
        .filter(([id]) => id)
    );
    const dataEdgeByPair = new Map(
      dataEdges
        .map((edge) => [networkEdgeKey(edge?.source, edge?.target), edge])
        .filter(([key]) => key && key !== "->")
    );
    const rows = (graph.getEdges?.() || [])
      .map((item, index) => {
        const model = item?.getModel?.() || {};
        const source = networkPlanEndpoint(model?.source);
        const target = networkPlanEndpoint(model?.target);
        const incident = source === nodeId || target === nodeId;
        const denseStyled = String(model.__networkDenseStyled || "");
        const weakLine = Number(model.lineWidth) > 0 && Number(model.lineWidth) <= 0.45;
        const hiddenBySkeleton = !!model.layoutHiddenBySkeleton || String(model.layoutEdgeTier || "") === "hidden";
        const needsReveal = denseStyled === "weak" || weakLine || hiddenBySkeleton;
        return {
          item,
          model,
          index,
          source,
          target,
          incident,
          needsReveal,
          weight: networkDenseHoverEdgeWeight(model),
        };
      })
      .filter((row) => row.incident && row.needsReveal && row.source && row.target && row.source !== row.target)
      .sort((left, right) => right.weight - left.weight || left.index - right.index);
    const limit = mode === "xlarge" ? 56 : 36;
    const selected = rows.slice(0, limit);
    if (!selected.length) {
      if (state.networkLayout) {
        state.networkLayout.lastDenseHoverReveal = {
          reason: String(reason || ""),
          nodeId,
          edges: 0,
          candidates: rows.length,
          mode,
          at: Date.now(),
        };
      }
      return 0;
    }
    const revealRows = [];
    graph.setAutoPaint?.(false);
    try {
      selected.forEach((row) => {
        const dataEdge =
          dataEdgeById.get(String(row.model?.id || "").trim()) ||
          dataEdgeByPair.get(networkEdgeKey(row.model?.source, row.model?.target));
        const baseLineWidth = Number.isFinite(Number(row.model.__networkDenseBaseLineWidth))
          ? Number(row.model.__networkDenseBaseLineWidth)
          : Number(row.model.lineWidth) || 1.2;
        revealRows.push({
          item: row.item,
          visual: captureNetworkDenseHoverEdgeVisual(row.model),
          dataEdge,
          dataVisual: dataEdge ? captureNetworkDenseHoverEdgeVisual(dataEdge) : null,
        });
        const update = {
          label: "",
          labelTop: "",
          labelBottom: "",
          detailLabel: false,
          edgeLabelHidden: true,
          layoutEdgeLabelTier: "hidden",
          layoutHiddenBySkeleton: false,
          layoutEdgeTier: String(row.model.layoutEdgeTier || "secondary") === "hidden" ? "secondary" : row.model.layoutEdgeTier || "secondary",
          stroke: "rgba(15,23,42,0.42)",
          lineWidth: Math.max(0.88, Math.min(1.35, baseLineWidth * 0.78)),
          showArrow: false,
          __networkDenseStyled: "hover",
          __networkDenseHoverReveal: true,
        };
        graph.updateItem?.(row.item, update);
        graph.refreshItem?.(row.item);
      });
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    networkDenseHoverEdgeReveal = { graph, rows: revealRows };
    if (state.networkLayout) {
      state.networkLayout.lastDenseHoverReveal = {
        reason: String(reason || ""),
        nodeId,
        edges: selected.length,
        candidates: rows.length,
        mode,
        at: Date.now(),
      };
    }
    return selected.length;
  }

  function selectNetworkSkeletonRenderEdges(nodes = [], edges = [], limit = 900, options = {}) {
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const edgeRows = Array.isArray(edges) ? edges : [];
    if (!nodeRows.length || !edgeRows.length) return [];
    let visibleIds = new Set(
      nodeRows
        .filter((node) => String(node?.layoutVisibilityTier || "") !== "hidden")
        .map((node) => String(node?.id || "").trim())
        .filter(Boolean)
    );
    const maxVisibleIds = Math.min(360, nodeRows.length);
    if (!visibleIds.size || visibleIds.size > maxVisibleIds) {
      visibleIds = new Set(
        nodeRows
          .map((node) => ({ node, score: rankNetworkCoarseNode(node), id: String(node?.id || "").trim() }))
          .filter((row) => row.id)
          .sort((left, right) => right.score - left.score || left.id.localeCompare(right.id, "zh-CN"))
          .slice(0, maxVisibleIds)
          .map((row) => row.id)
      );
    }
    const communityById = new Map(
      nodeRows
        .map((node) => [
          String(node?.id || "").trim(),
          String(node?.layoutCommunity || node?.clusterId || node?.clusterHint || "").trim(),
        ])
        .filter(([id]) => id)
    );
    const positionById = new Map(
      nodeRows
        .map((node) => [
          String(node?.id || "").trim(),
          { x: Number(node?.x) || 0, y: Number(node?.y) || 0 },
        ])
        .filter(([id]) => id)
    );
    const maxEdges = Math.max(0, Number(limit) || 0);
    const endpointLimit = Math.max(
      1,
      Number(options?.endpointLimit) || (maxEdges <= 140 ? 6 : 14)
    );
    const communityPairLimit = Math.max(
      2,
      Number(options?.communityPairLimit) || Math.ceil(endpointLimit * 2)
    );
    const sameCommunityPairLimit = Math.max(communityPairLimit, maxEdges);
    const crossCommunityLimit = Math.max(
      2,
      Number(options?.crossCommunityLimit) || (maxEdges <= 140 ? 18 : 56)
    );
    const edgeWeight = (edge) =>
      Math.max(
        0,
        Number(edge?.weight) ||
          Number(edge?.amount) ||
          Number(edge?.total_amount) ||
          Number(edge?.totalAmount) ||
          Number(edge?.count) ||
          0
      );
    const edgeCommunityPair = (source, target) => {
      const sourceCommunity = communityById.get(source) || source;
      const targetCommunity = communityById.get(target) || target;
      return sourceCommunity <= targetCommunity
        ? `${sourceCommunity}|${targetCommunity}`
        : `${targetCommunity}|${sourceCommunity}`;
    };
    const isCrossCommunityEdge = (source, target) => {
      const sourceCommunity = communityById.get(source) || source;
      const targetCommunity = communityById.get(target) || target;
      return sourceCommunity !== targetCommunity;
    };
    const edgeLength = (source, target) => {
      const sourcePosition = positionById.get(source);
      const targetPosition = positionById.get(target);
      if (!sourcePosition || !targetPosition) return 0;
      return Math.hypot(sourcePosition.x - targetPosition.x, sourcePosition.y - targetPosition.y);
    };
    const includeIncidentEdges = options?.includeIncidentEdges === true;
    const edgeBundleRouteMeta = (row) => {
      const bundleSize = Math.max(1, Number(row?.bundle?.count) || 1);
      if (!row?.crossCommunity || bundleSize <= 1) return {};
      const pair = String(row.communityPair || "");
      const sourcePosition = positionById.get(row.source) || { x: 0, y: 0 };
      const targetPosition = positionById.get(row.target) || { x: 0, y: 0 };
      const dx = targetPosition.x - sourcePosition.x;
      const dy = targetPosition.y - sourcePosition.y;
      const sign = hashProjectionPointSeed(`network-superedge-route|${pair}`) / 0xffffffff >= 0.5 ? 1 : -1;
      const bundleWeight = Math.max(0, Number(row?.bundle?.weight) || row.weight || 0);
      const magnitude = Math.max(22, Math.min(92, 18 + Math.log1p(bundleSize) * 11 + Math.log1p(bundleWeight) * 0.9));
      return {
        layoutEdgeBundleRouted: true,
        edgeOffset: Math.round(sign * magnitude * 100) / 100,
        edgeRoute: "orthogonal",
        orthAxis: Math.abs(dx) >= Math.abs(dy) ? "horizontal" : "vertical",
        orthBias:
          Math.round((0.34 + (hashProjectionPointSeed(`network-superedge-bias|${pair}`) / 0xffffffff) * 0.32) * 10000) /
          10000,
        edgeLabelAnchorMode: "center-snap",
      };
    };
    const buildCandidates = (requireBothVisible) =>
      edgeRows.filter((edge) => {
        const source = networkPlanEndpoint(edge?.source);
        const target = networkPlanEndpoint(edge?.target);
        return requireBothVisible
          ? visibleIds.has(source) && visibleIds.has(target)
          : visibleIds.has(source) || visibleIds.has(target);
      });
    const endpointCounts = new Map();
    const communityPairCounts = new Map();
    let crossCommunityCount = 0;
    const selected = [];
    const deferred = [];
    const keepProjectedSkeletonEdges = includeIncidentEdges && edgeRows.length <= maxEdges;
    const canTake = (row, relaxed = false) => {
      const endpointBudget = keepProjectedSkeletonEdges
        ? Number.POSITIVE_INFINITY
        : relaxed
          ? endpointLimit * 2
          : endpointLimit;
      const samePairBudget = keepProjectedSkeletonEdges
        ? Number.POSITIVE_INFINITY
        : relaxed
          ? sameCommunityPairLimit * 2
          : sameCommunityPairLimit;
      const crossBudget = keepProjectedSkeletonEdges
        ? Number.POSITIVE_INFINITY
        : relaxed
          ? crossCommunityLimit * 2
          : crossCommunityLimit;
      const crossPairBudget =
        keepProjectedSkeletonEdges ? Number.POSITIVE_INFINITY : maxEdges <= 140 ? (relaxed ? 2 : 1) : relaxed ? 4 : 2;
      return (
        (endpointCounts.get(row.source) || 0) < endpointBudget &&
        (endpointCounts.get(row.target) || 0) < endpointBudget &&
        (communityPairCounts.get(row.communityPair) || 0) < (row.crossCommunity ? crossPairBudget : samePairBudget) &&
        (!row.crossCommunity || crossCommunityCount < crossBudget)
      );
    };
    const take = (row) => {
      selected.push(row);
      endpointCounts.set(row.source, (endpointCounts.get(row.source) || 0) + 1);
      endpointCounts.set(row.target, (endpointCounts.get(row.target) || 0) + 1);
      communityPairCounts.set(row.communityPair, (communityPairCounts.get(row.communityPair) || 0) + 1);
      if (row.crossCommunity) crossCommunityCount += 1;
    };
    const candidateRows = buildCandidates(!includeIncidentEdges)
      .map((edge, index) => {
        const source = networkPlanEndpoint(edge?.source);
        const target = networkPlanEndpoint(edge?.target);
        const crossCommunity = isCrossCommunityEdge(source, target);
        const length = edgeLength(source, target);
        const weight = edgeWeight(edge);
        const visibleEndpointCount = (visibleIds.has(source) ? 1 : 0) + (visibleIds.has(target) ? 1 : 0);
        return {
          edge,
          index,
          weight,
          source,
          target,
          visibleEndpointCount,
          crossCommunity,
          communityPair: edgeCommunityPair(source, target),
          score:
            (weight * (crossCommunity ? 0.42 : 1.25) * (visibleEndpointCount >= 2 ? 1.18 : 0.78)) /
            (1 + length / 1200),
        };
      })
      .filter((row) => row.source && row.target && row.source !== row.target && row.visibleEndpointCount > 0);
    const bundleByPair = new Map();
    candidateRows.forEach((row) => {
      const bundle = bundleByPair.get(row.communityPair) || { count: 0, weight: 0, rank: 0 };
      bundle.count += 1;
      bundle.weight += row.weight;
      bundleByPair.set(row.communityPair, bundle);
    });
    Array.from(bundleByPair.entries())
      .sort((left, right) => right[1].weight - left[1].weight || right[1].count - left[1].count || left[0].localeCompare(right[0], "zh-CN"))
      .forEach(([pair], index) => {
        const bundle = bundleByPair.get(pair);
        if (bundle) bundle.rank = index + 1;
      });
    candidateRows
      .map((row) => {
        const bundle = bundleByPair.get(row.communityPair) || { count: 1, weight: row.weight, rank: 0 };
        const bundleScore = Math.log1p(Math.max(0, bundle.weight)) + Math.sqrt(Math.max(1, bundle.count)) * 0.3;
        return {
          ...row,
          bundle,
          score:
            ((row.weight * (row.crossCommunity ? 0.75 : 1.0) + bundleScore * (row.crossCommunity ? 2.4 : 0.2)) /
              (1 + edgeLength(row.source, row.target) / 1200)),
        };
      })
      .sort((left, right) => right.score - left.score || right.weight - left.weight || left.index - right.index)
      .forEach((next) => {
        const source = next.source;
        const target = next.target;
        if (!source || !target || source === target) return;
        if (selected.length < maxEdges && canTake(next)) {
          take(next);
        } else {
          deferred.push(next);
        }
      });
    const minFill = Math.min(maxEdges, Math.ceil(maxEdges * (includeIncidentEdges ? 0.72 : 0.55)));
    deferred.some((row) => {
      if (selected.length >= minFill) return true;
      if (canTake(row, true)) take(row);
      return false;
    });
    if (includeIncidentEdges && selected.length < minFill) {
      const selectedIndexes = new Set(selected.map((row) => row.index));
      deferred.some((row) => {
        if (selected.length >= minFill) return true;
        if (selectedIndexes.has(row.index)) return false;
        take(row);
        selectedIndexes.add(row.index);
        return false;
      });
    }
    return selected
      .slice(0, maxEdges)
      .map((row) =>
        toNetworkSkeletonRenderEdge(row.edge, {
          layoutCommunityPair: row.communityPair,
          layoutEdgeBundleId: row.communityPair,
          layoutEdgeBundleSize: row.bundle?.count || 1,
          layoutEdgeBundleWeight: Math.round((Number(row.bundle?.weight) || row.weight || 0) * 100) / 100,
          layoutEdgeBundleRank: row.bundle?.rank || 0,
          layoutEdgeBundled: (row.bundle?.count || 1) > 1,
          layoutEdgeBundleRepresentative: true,
          layoutEdgeTier: row.crossCommunity || (row.bundle?.count || 1) > 1 ? "primary" : "secondary",
          ...edgeBundleRouteMeta(row),
        })
      );
  }

  function toNetworkSkeletonRenderEdge(edge = {}, bundleMeta = null) {
    if (!edge || typeof edge !== "object") return edge;
    return {
      ...edge,
      ...(bundleMeta && typeof bundleMeta === "object" ? bundleMeta : {}),
      label: "",
      labelTop: "",
      labelBottom: "",
      label_top: "",
      label_bottom: "",
      edgeLabelHidden: true,
      layoutEdgeLabelTier: "hidden",
    };
  }

  function revealNetworkRuntimeSkeletonEdges(graph, { mode = "", reason = "" } = {}) {
    if (state.layoutPreset !== "network") return 0;
    const normalizedMode = String(mode || state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (!isLargeNetworkTopologyMode(normalizedMode) || !graph || typeof graph.updateItem !== "function") return 0;
    const edgeItems = Array.isArray(graph.getEdges?.()) ? graph.getEdges() : [];
    if (!edgeItems.length || edgeItems.length > resolveNetworkRuntimeEdgeBudget(normalizedMode)) return 0;
    let changed = 0;
    const canSetAutoPaint = typeof graph.setAutoPaint === "function";
    if (canSetAutoPaint) graph.setAutoPaint(false);
    try {
      edgeItems.forEach((item) => {
        const model = item?.getModel?.() || {};
        const tier = String(model.layoutEdgeTier || "secondary").trim();
        const hidden = !!model.layoutHiddenBySkeleton || tier === "hidden" || Number(model.lineWidth || 0) <= 0.05;
        const lineWidthCap = normalizedMode === "large" ? 1.32 : 1.12;
        const needsDeclutter =
          !hidden && (model.showArrow !== false || Number(model.lineWidth || 0) > lineWidthCap + 0.01);
        if (!hidden && Number(model.lineWidth || 0) >= 0.72 && !needsDeclutter) return;
        if (model.__networkBaseShowArrow == null) model.__networkBaseShowArrow = !!model.showArrow;
        const visibleTier = tier && tier !== "hidden" ? tier : "secondary";
        const rawLineWidth = Math.max(Number(model.__networkBaseLineWidth) || 0, visibleTier === "primary" ? 0.9 : 0.72);
        const lineWidth = Math.min(lineWidthCap, rawLineWidth);
        const stroke = visibleTier === "primary" ? "rgba(15,23,42,0.58)" : "rgba(30,41,59,0.42)";
        graph.updateItem(item, {
          stroke,
          lineWidth,
          showArrow: false,
          layoutHiddenBySkeleton: false,
          layoutEdgeTier: visibleTier,
        });
        changed += 1;
      });
    } finally {
      if (canSetAutoPaint) graph.setAutoPaint(true);
      if (changed) graph.paint?.();
    }
    if (changed) {
      recordNetworkRuntimePerfEvent("runtime-skeleton-edges-revealed", {
        reason: String(reason || ""),
        mode: normalizedMode,
        edges: changed,
        runtimeEdges: edgeItems.length,
      });
    }
    return changed;
  }

  function toNetworkSkeletonRenderNode(node = {}) {
    if (!node || typeof node !== "object") return node;
    if (String(node?.layoutLabelTier || "") !== "hidden" && !node?.nodeLabelHidden) return node;
    return {
      ...node,
      name: "",
      title: "",
      label: "",
      displayId: "",
      display_id: "",
      icon: "",
      iconSymbol: "",
      iconSize: 0,
      fontShadow: false,
      nodeLabelHidden: true,
    };
  }

  function capNetworkSkeletonRenderLabels(nodes = [], mode = "") {
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const normalizedMode = String(mode || "").trim().toLowerCase();
    const labelLimit =
      normalizedMode === "large" ? 12 : normalizedMode === "xlarge" ? 16 : nodeRows.length;
    if (!nodeRows.length || nodeRows.length <= labelLimit) return nodeRows;
    const keepIds = new Set(
      nodeRows
        .map((node, index) => {
          const id = String(node?.id || "").trim();
          const labelVisible = !node?.nodeLabelHidden && String(node?.layoutLabelTier || "") !== "hidden";
          const visibilityTier = String(node?.layoutVisibilityTier || "").trim().toLowerCase();
          return {
            id,
            index,
            score:
              (labelVisible ? 1_000_000 : 0) +
              (visibilityTier === "primary" ? 500_000 : visibilityTier === "secondary" ? 120_000 : 0) +
              rankNetworkCoarseNode(node),
          };
        })
        .filter((row) => row.id)
        .sort((left, right) => right.score - left.score || left.index - right.index || left.id.localeCompare(right.id, "zh-CN"))
        .slice(0, labelLimit)
        .map((row) => row.id)
    );
    return nodeRows.map((node) => {
      const id = String(node?.id || "").trim();
      if (id && keepIds.has(id)) return node;
      return toNetworkSkeletonRenderNode({
        ...node,
        layoutLabelTier: "hidden",
        nodeLabelHidden: true,
      });
    });
  }

  function collectNetworkRenderEdgeEndpointIds(edges = []) {
    const ids = new Set();
    (Array.isArray(edges) ? edges : []).forEach((edge) => {
      const source = networkPlanEndpoint(edge?.source);
      const target = networkPlanEndpoint(edge?.target);
      if (source) ids.add(source);
      if (target) ids.add(target);
    });
    return ids;
  }

  function filterNetworkSkeletonRenderEdgesToNodes(edges = [], nodeIds = null) {
    const ids = nodeIds instanceof Set ? nodeIds : new Set();
    if (!ids.size) return [];
    return (Array.isArray(edges) ? edges : []).filter((edge) => {
      const source = networkPlanEndpoint(edge?.source);
      const target = networkPlanEndpoint(edge?.target);
      return source && target && ids.has(source) && ids.has(target);
    });
  }

  function selectNetworkSkeletonRenderNodes(nodes = [], edges = [], mode = "") {
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const normalizedMode = String(mode || "").trim().toLowerCase();
    if (!isLargeNetworkTopologyMode(normalizedMode) || !nodeRows.length) return nodeRows;
    const endpointIds = collectNetworkRenderEdgeEndpointIds(edges);
    if (!endpointIds.size) {
      const fallbackLimit = normalizedMode === "xlarge" ? 96 : 160;
      return nodeRows
        .map((node, index) => ({ node, index, score: rankNetworkCoarseNode(node), id: String(node?.id || "").trim() }))
        .filter((row) => row.id)
        .sort((left, right) => right.score - left.score || left.id.localeCompare(right.id, "zh-CN") || left.index - right.index)
        .slice(0, Math.min(fallbackLimit, nodeRows.length))
        .map((row) => row.node);
    }
    const endpointDegree = new Map();
    (Array.isArray(edges) ? edges : []).forEach((edge) => {
      const source = networkPlanEndpoint(edge?.source);
      const target = networkPlanEndpoint(edge?.target);
      if (source) endpointDegree.set(source, (endpointDegree.get(source) || 0) + 1);
      if (target) endpointDegree.set(target, (endpointDegree.get(target) || 0) + 1);
    });
    const maxVisible = Math.min(nodeRows.length, normalizedMode === "xlarge" ? 260 : 240);
    return nodeRows
      .map((node, index) => ({ node, index, score: rankNetworkCoarseNode(node), id: String(node?.id || "").trim() }))
      .filter((row) => row.id && endpointIds.has(row.id))
      .sort((left, right) => {
        const degreeDelta = (endpointDegree.get(right.id) || 0) - (endpointDegree.get(left.id) || 0);
        if (degreeDelta) return degreeDelta;
        return right.score - left.score || left.id.localeCompare(right.id, "zh-CN") || left.index - right.index;
      })
      .slice(0, maxVisible)
      .map((row) => row.node);
  }

  function collectNetworkRuntimeNodeModels(graph) {
    return (Array.isArray(graph?.getNodes?.()) ? graph.getNodes() : [])
      .map((item) => item?.getModel?.() || null)
      .filter(Boolean);
  }

  function invalidateNetworkRuntimeEdgeGeometry(graph, reason = "") {
    if (state.layoutPreset !== "network" || !graph || typeof graph.updateItem !== "function") return 0;
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (!isLargeNetworkTopologyMode(mode)) return 0;
    const edgeItems = Array.isArray(graph.getEdges?.()) ? graph.getEdges() : [];
    if (!edgeItems.length) return 0;
    let touched = 0;
    edgeItems.forEach((item) => {
      const model = item?.getModel?.() || {};
      graph.updateItem?.(item, {
        edgeRoute: String(model.edgeRoute || model.route || ""),
      });
      touched += 1;
    });
    if (touched) {
      recordNetworkRuntimePerfEvent("runtime-edge-geometry-invalidated", {
        reason: String(reason || ""),
        mode,
        edges: touched,
      });
    }
    return touched;
  }

  function redrawNetworkRuntimeSkeletonGraph(graph, reason = "") {
    if (state.layoutPreset !== "network" || !graph || typeof graph.data !== "function" || typeof graph.render !== "function") {
      return false;
    }
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (!isLargeNetworkTopologyMode(mode)) return false;
    const nodeItems = Array.isArray(graph.getNodes?.()) ? graph.getNodes() : [];
    const edgeItems = Array.isArray(graph.getEdges?.()) ? graph.getEdges() : [];
    if (!nodeItems.length) return false;
    const nodes = nodeItems
      .map((item) => {
        const model = item?.getModel?.() || {};
        const id = String(model?.id || "").trim();
        if (!id) return null;
        return { ...model, id };
      })
      .filter(Boolean);
    const nodeIds = new Set(nodes.map((node) => String(node.id || "")).filter(Boolean));
    const edges = edgeItems
      .map((item, index) => {
        const model = item?.getModel?.() || {};
        const source = networkPlanEndpoint(model.source);
        const target = networkPlanEndpoint(model.target);
        if (!source || !target || !nodeIds.has(source) || !nodeIds.has(target)) return null;
        return {
          ...model,
          id: String(model.id || `network-runtime-edge:${index}:${source}->${target}`),
          source,
          target,
        };
      })
      .filter(Boolean);
    const viewport = captureGraphViewport(graph);
    const startedAt = networkPerfNow();
    try {
      graph.clear?.();
      graph.data({ nodes, edges });
      graph.render();
      graph.refreshPositions?.();
      graph.paint?.();
      if (viewport) applyGraphViewport(viewport, graph);
      recordNetworkRuntimePerfEvent("runtime-skeleton-redrawn", {
        reason: String(reason || ""),
        mode,
        nodes: nodes.length,
        edges: edges.length,
        durationMs: roundNetworkPerfMs(networkPerfNow() - startedAt),
      });
      return true;
    } catch (error) {
      try {
        log("WARN", "network runtime skeleton redraw failed", {
          reason: String(reason || ""),
          message: String(error?.message || error),
        });
      } catch (_error) {}
      return false;
    }
  }

  function pruneNetworkRuntimeDisconnectedSkeleton(graph, options = {}) {
    if (state.layoutPreset !== "network") return false;
    const mode = String(options?.mode || state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (mode !== "xlarge" || !graph || typeof graph.removeItem !== "function") return false;
    const nodeItems = Array.isArray(graph.getNodes?.()) ? graph.getNodes() : [];
    const edgeItems = Array.isArray(graph.getEdges?.()) ? graph.getEdges() : [];
    if (nodeItems.length < 24 || !edgeItems.length) return false;
    const itemById = new Map();
    nodeItems.forEach((item) => {
      const id = String(item?.getModel?.()?.id || "").trim();
      if (id) itemById.set(id, item);
    });
    const adjacency = new Map(Array.from(itemById.keys()).map((id) => [id, new Set()]));
    const edgeRows = [];
    edgeItems.forEach((item) => {
      const model = item?.getModel?.() || {};
      const source = networkPlanEndpoint(model.source);
      const target = networkPlanEndpoint(model.target);
      if (!source || !target || source === target || !itemById.has(source) || !itemById.has(target)) return;
      adjacency.get(source)?.add(target);
      adjacency.get(target)?.add(source);
      edgeRows.push({ item, source, target });
    });
    const zeroDegreeIds = Array.from(adjacency.entries())
      .filter(([, neighbors]) => !neighbors.size)
      .map(([id]) => id);
    const visited = new Set();
    const componentById = new Map();
    const components = [];
    Array.from(adjacency.keys())
      .sort((left, right) => left.localeCompare(right, "zh-CN"))
      .forEach((id) => {
        if (visited.has(id) || !adjacency.get(id)?.size) return;
        const queue = [id];
        const ids = [];
        visited.add(id);
        while (queue.length) {
          const next = queue.shift();
          ids.push(next);
          (adjacency.get(next) || []).forEach((neighbor) => {
            if (visited.has(neighbor)) return;
            visited.add(neighbor);
            queue.push(neighbor);
          });
        }
        const key = `component:${components.length}`;
        ids.forEach((nodeId) => componentById.set(nodeId, key));
        components.push({ key, ids, edgeCount: 0 });
      });
    edgeRows.forEach((row) => {
      const key = componentById.get(row.source);
      if (!key || key !== componentById.get(row.target)) return;
      const component = components.find((entry) => entry.key === key);
      if (component) component.edgeCount += 1;
    });
    const allowedComponentKeys = new Set(
      components
        .sort((left, right) => right.ids.length - left.ids.length || right.edgeCount - left.edgeCount || left.key.localeCompare(right.key))
        .slice(0, 4)
        .map((entry) => entry.key)
    );
    const keepNodeIds = new Set();
    components.forEach((component) => {
      if (!allowedComponentKeys.has(component.key)) return;
      component.ids.forEach((id) => keepNodeIds.add(id));
    });
    const removeNodeIds = new Set(zeroDegreeIds);
    itemById.forEach((_item, id) => {
      if (keepNodeIds.has(id)) return;
      removeNodeIds.add(id);
    });
    if (!removeNodeIds.size) return false;
    let removedNodes = 0;
    let removedEdges = 0;
    graph.setAutoPaint?.(false);
    try {
      edgeRows.forEach((row) => {
        if (!removeNodeIds.has(row.source) && !removeNodeIds.has(row.target)) return;
        graph.removeItem?.(row.item);
        removedEdges += 1;
      });
      removeNodeIds.forEach((id) => {
        const item = itemById.get(id);
        if (!item) return;
        graph.removeItem?.(item);
        removedNodes += 1;
      });
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    if (removedNodes || removedEdges) {
      recordNetworkRuntimePerfEvent("runtime-skeleton-disconnected-pruned", {
        reason: String(options?.reason || ""),
        mode,
        removedNodes,
        removedEdges,
        zeroDegreeNodes: zeroDegreeIds.length,
        components: components.length,
        keptComponents: Math.min(4, components.length),
      });
      updateMinimap();
      return true;
    }
    return false;
  }

  function relaxNetworkRuntimeSkeletonGraph(graph, options = {}) {
    if (state.layoutPreset !== "network") return false;
    const mode = String(options?.mode || state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (!isLargeNetworkTopologyMode(mode)) return false;
    pruneNetworkRuntimeDisconnectedSkeleton(graph, {
      mode,
      reason: `${options?.reason || "network-runtime-relax"}:pre-relax`,
    });
    const nodeItems = Array.isArray(graph?.getNodes?.()) ? graph.getNodes() : [];
    const edgeItems = Array.isArray(graph?.getEdges?.()) ? graph.getEdges() : [];
    if (nodeItems.length < 24 || nodeItems.length > 260 || !edgeItems.length) return false;
    const startedAt = networkPerfNow();
    const rows = [];
    const rowById = new Map();
    nodeItems.forEach((item, index) => {
      const model = item?.getModel?.() || {};
      const id = String(model?.id || "").trim();
      if (!id) return;
      const angle = Math.PI * 2 * (hashProjectionPointSeed(`network-runtime-relax|${id}`) / 0xffffffff);
      const x = Number.isFinite(Number(model.x)) ? Number(model.x) : Math.cos(angle) * 120;
      const y = Number.isFinite(Number(model.y)) ? Number(model.y) : Math.sin(angle) * 120;
      const radius = Math.max(12, Math.min(42, Number(model.collisionRadius || model.r || model.radius) || 18));
      const row = {
        item,
        model,
        id,
        index,
        x,
        y,
        seedX: x,
        seedY: y,
        vx: 0,
        vy: 0,
        radius,
        degree: 0,
        neighbors: [],
        community: String(model.layoutCommunity || model.clusterId || model.layoutClusterId || "").trim(),
      };
      rows.push(row);
      rowById.set(id, row);
    });
    if (rows.length < 24 || rows.length > 260) return false;
    const edgeRows = [];
    edgeItems.forEach((item, index) => {
      const model = item?.getModel?.() || {};
      const source = networkPlanEndpoint(model.source);
      const target = networkPlanEndpoint(model.target);
      const sourceRow = rowById.get(source);
      const targetRow = rowById.get(target);
      if (!sourceRow || !targetRow || sourceRow === targetRow) return;
      sourceRow.degree += 1;
      targetRow.degree += 1;
      sourceRow.neighbors.push(targetRow.index);
      targetRow.neighbors.push(sourceRow.index);
      const rawWeight =
        Number(model.weight) ||
        Number(model.amount) ||
        Number(model.total_amount) ||
        Number(model.totalAmount) ||
        Number(model.count) ||
        1;
      edgeRows.push({
        source: sourceRow.index,
        target: targetRow.index,
        weight: Math.max(0.75, Math.min(4.5, Math.log1p(Math.max(1, rawWeight)))),
        crossCommunity:
          !!sourceRow.community && !!targetRow.community && sourceRow.community !== targetRow.community,
        index,
      });
    });
    if (!edgeRows.length) return false;
    const layoutDensity = edgeRows.length / Math.max(1, rows.length);
    const communityAnchors = new Map();
    rows.forEach((row) => {
      const community = row.community || `node:${row.id}`;
      const anchor = communityAnchors.get(community) || { x: 0, y: 0, count: 0 };
      anchor.x += row.seedX;
      anchor.y += row.seedY;
      anchor.count += 1;
      communityAnchors.set(community, anchor);
    });
    communityAnchors.forEach((anchor) => {
      anchor.x /= Math.max(1, anchor.count);
      anchor.y /= Math.max(1, anchor.count);
    });
    const before = summarizeLayout(rows);
    const rowAt = new Map(rows.map((row, index) => [row.index, index]));
	    const spreadHubLeaves = () => {
	      if (mode !== "xlarge" || rows.length <= 180) return 0;
      const groups = new Map();
      rows.forEach((row, denseIndex) => {
        if (row.degree < 1 || row.degree > 3 || !row.neighbors.length) return;
        const hubDenseIndex = row.neighbors
          .map((neighborIndex) => rowAt.get(neighborIndex))
          .filter((index) => index != null)
          .sort((left, right) => rows[right].degree - rows[left].degree || rows[left].id.localeCompare(rows[right].id, "zh-CN"))[0];
        if (hubDenseIndex == null) return;
        const hub = rows[hubDenseIndex];
        if (!hub || hub.degree < 8 || hub.degree < row.degree * 4) return;
        if (row.community && hub.community && row.community !== hub.community) return;
        const list = groups.get(hubDenseIndex) || [];
        list.push(denseIndex);
        groups.set(hubDenseIndex, list);
      });
      let adjusted = 0;
      groups.forEach((leafIndexes, hubDenseIndex) => {
        if (leafIndexes.length < 6) return;
        const hub = rows[hubDenseIndex];
        const baseAngle = Math.PI * 2 * (hashProjectionPointSeed(`network-runtime-leaf-ring|${hub.id}`) / 0xffffffff);
        const capacity = Math.max(10, Math.min(30, Math.ceil(Math.sqrt(leafIndexes.length) * 5)));
        const ordered = leafIndexes
          .map((leafIndex) => {
            const leaf = rows[leafIndex];
            const angle = Math.atan2(leaf.y - hub.y, leaf.x - hub.x);
            return { leafIndex, angle, id: leaf.id };
          })
          .sort((left, right) => left.angle - right.angle || left.id.localeCompare(right.id, "zh-CN"));
        ordered.forEach((entry, index) => {
          const leaf = rows[entry.leafIndex];
          const ring = Math.floor(index / capacity);
          const ringStart = ring * capacity;
          const ringCount = Math.max(1, Math.min(capacity, ordered.length - ringStart));
          const slot = index - ringStart;
          const angle = baseAngle + (slot / ringCount) * Math.PI * 2 + ring * 0.23;
          const radius =
            hub.radius +
            leaf.radius +
            Math.max(92, Math.min(220, 102 + Math.sqrt(leafIndexes.length) * 5 + ring * 34 + Math.min(30, hub.degree * 0.55)));
          const targetX = hub.x + Math.cos(angle) * radius;
          const targetY = hub.y + Math.sin(angle) * radius;
          leaf.x = leaf.x * 0.42 + targetX * 0.58;
          leaf.y = leaf.y * 0.42 + targetY * 0.58;
          adjusted += 1;
        });
      });
      return adjusted;
    };
    const pullPeripheralNodesToNeighbors = (passes = 2) => {
      let adjusted = 0;
      for (let pass = 0; pass < passes; pass += 1) {
        rows.forEach((row) => {
          if (row.degree < 1 || row.degree > 3 || !row.neighbors.length) return;
          const neighborRows = row.neighbors
            .map((neighborIndex) => rows[rowAt.get(neighborIndex)])
            .filter(Boolean);
          if (!neighborRows.length) return;
          const weightSum = neighborRows.reduce((sum, neighbor) => sum + Math.max(1, neighbor.degree || 1), 0);
          const anchor = neighborRows.reduce(
            (acc, neighbor) => {
              const weight = Math.max(1, neighbor.degree || 1);
              acc.x += neighbor.x * weight;
              acc.y += neighbor.y * weight;
              acc.radius += neighbor.radius * weight;
              acc.degree += Math.max(1, neighbor.degree || 1) * weight;
              return acc;
            },
            { x: 0, y: 0, radius: 0, degree: 0 }
          );
          anchor.x /= Math.max(1, weightSum);
          anchor.y /= Math.max(1, weightSum);
          anchor.radius /= Math.max(1, weightSum);
          anchor.degree /= Math.max(1, weightSum);
          const dx = row.x - anchor.x;
          const dy = row.y - anchor.y;
          const dist = Math.max(0.01, Math.hypot(dx, dy));
          const target =
            row.radius +
            anchor.radius +
            (row.degree <= 1 ? 58 : row.degree <= 2 ? 72 : 86) +
            Math.min(34, Math.sqrt(Math.max(1, anchor.degree)) * 5);
          if (dist <= target) return;
          const pull = Math.min(260, (dist - target) * (row.degree <= 1 ? 0.68 : 0.54));
          row.x -= (dx / dist) * pull;
          row.y -= (dy / dist) * pull;
          adjusted += 1;
        });
      }
      return adjusted;
    };
    const resolveCollisions = (passes = 1) => {
      let adjusted = 0;
      for (let pass = 0; pass < passes; pass += 1) {
        for (let i = 0; i < rows.length; i += 1) {
          for (let j = i + 1; j < rows.length; j += 1) {
            let dx = rows[j].x - rows[i].x;
            let dy = rows[j].y - rows[i].y;
            if (Math.abs(dx) + Math.abs(dy) < 0.001) {
              const angle =
                Math.PI * 2 * (hashProjectionPointSeed(`network-runtime-final-overlap|${rows[i].id}|${rows[j].id}`) / 0xffffffff);
              dx = Math.cos(angle) * 0.2;
              dy = Math.sin(angle) * 0.2;
            }
            const dist = Math.max(0.01, Math.hypot(dx, dy));
            const minDist = rows[i].radius + rows[j].radius + 34;
            if (dist >= minDist) continue;
            const push = (minDist - dist) * 0.52;
            const ux = dx / dist;
            const uy = dy / dist;
            rows[i].x -= ux * push;
            rows[i].y -= uy * push;
            rows[j].x += ux * push;
            rows[j].y += uy * push;
            adjusted += 1;
          }
        }
      }
      return adjusted;
    };
    const contractLongEdges = (passes = 4) => {
      let adjusted = 0;
      for (let pass = 0; pass < passes; pass += 1) {
        edgeRows.forEach((edge) => {
          const sourceIndex = rowAt.get(edge.source);
          const targetIndex = rowAt.get(edge.target);
          if (sourceIndex == null || targetIndex == null) return;
          const source = rows[sourceIndex];
          const target = rows[targetIndex];
          const dx = target.x - source.x;
          const dy = target.y - source.y;
          const dist = Math.max(0.01, Math.hypot(dx, dy));
          const maxLen = edge.crossCommunity
            ? layoutDensity >= 2.4
              ? 340
              : layoutDensity >= 1.6
                ? 300
                : 250
            : layoutDensity >= 2.4
              ? 190
              : layoutDensity >= 1.6
                ? 172
                : 138;
          if (dist <= maxLen) return;
          const ux = dx / dist;
          const uy = dy / dist;
          const excess = Math.min(420, (dist - maxLen) * 0.48);
          const sourceDegree = Math.max(1, source.degree || 1);
          const targetDegree = Math.max(1, target.degree || 1);
          const sourceMove = targetDegree / (sourceDegree + targetDegree);
          const targetMove = sourceDegree / (sourceDegree + targetDegree);
          source.x += ux * excess * sourceMove;
          source.y += uy * excess * sourceMove;
          target.x -= ux * excess * targetMove;
          target.y -= uy * excess * targetMove;
          adjusted += 1;
        });
      }
      return adjusted;
    };
    const requestedIterations = Number(options?.iterations);
    const iterations = Number.isFinite(requestedIterations) && requestedIterations > 0
      ? Math.max(2, Math.min(64, requestedIterations))
      : Math.max(18, Math.min(64, rows.length <= 120 ? 56 : layoutDensity >= 2.4 ? 44 : 40));
    const edgePullScale = layoutDensity >= 2.4 ? 0.010 : layoutDensity >= 1.6 ? 0.0125 : 0.016;
    const repulsionBase = 6200 * (layoutDensity >= 2.4 ? 2.8 : layoutDensity >= 1.6 ? 1.75 : 1.08);
    const collisionPadding = layoutDensity >= 2.4 ? 46 : layoutDensity >= 1.6 ? 36 : 26;
    for (let iter = 0; iter < iterations; iter += 1) {
      const cooling = Math.max(0.2, 1 - (iter / Math.max(1, iterations)) * 0.72);
      const maxStep = 28 * cooling;
      const forces = rows.map(() => ({ x: 0, y: 0 }));
      edgeRows.forEach((edge) => {
        const sourceIndex = rowAt.get(edge.source);
        const targetIndex = rowAt.get(edge.target);
        if (sourceIndex == null || targetIndex == null) return;
        const source = rows[sourceIndex];
        const target = rows[targetIndex];
        const dx = target.x - source.x;
        const dy = target.y - source.y;
        const dist = Math.max(0.01, Math.hypot(dx, dy));
        const idealBase = edge.crossCommunity
          ? layoutDensity >= 2.4
            ? 500
            : layoutDensity >= 1.6
              ? 420
              : 360
          : layoutDensity >= 2.4
            ? 210
            : layoutDensity >= 1.6
              ? 188
              : 168;
        const idealMax = edge.crossCommunity ? 720 : layoutDensity >= 2.4 ? 320 : 240;
        const idealMin = edge.crossCommunity ? 220 : 108;
        const ideal =
          Math.max(idealMin, Math.min(idealMax, idealBase / Math.sqrt(edge.weight))) +
          (source.radius + target.radius) * (edge.crossCommunity ? 0.78 : 0.42);
        const pullScale = edge.crossCommunity ? edgePullScale * 0.72 : edgePullScale;
        const pull = Math.max(-7, Math.min(6, (dist - ideal) * pullScale * edge.weight * cooling));
        const ux = dx / dist;
        const uy = dy / dist;
        forces[sourceIndex].x += ux * pull;
        forces[sourceIndex].y += uy * pull;
        forces[targetIndex].x -= ux * pull;
        forces[targetIndex].y -= uy * pull;
      });
      for (let i = 0; i < rows.length; i += 1) {
        for (let j = i + 1; j < rows.length; j += 1) {
          let dx = rows[j].x - rows[i].x;
          let dy = rows[j].y - rows[i].y;
          if (Math.abs(dx) + Math.abs(dy) < 0.001) {
            const angle =
              Math.PI * 2 * (hashProjectionPointSeed(`network-runtime-overlap|${rows[i].id}|${rows[j].id}`) / 0xffffffff);
            dx = Math.cos(angle) * 0.2;
            dy = Math.sin(angle) * 0.2;
          }
          const distSq = Math.max(16, dx * dx + dy * dy);
          const dist = Math.sqrt(distSq);
          const minDist = rows[i].radius + rows[j].radius + collisionPadding;
          const sameCommunity = rows[i].community && rows[i].community === rows[j].community;
          let push = (repulsionBase * (sameCommunity ? 0.72 : 1.48)) / distSq;
          if (dist < minDist) push += (minDist - dist) * (layoutDensity >= 2.4 ? 0.32 : 0.2);
          const ux = dx / dist;
          const uy = dy / dist;
          forces[i].x -= ux * push;
          forces[i].y -= uy * push;
          forces[j].x += ux * push;
          forces[j].y += uy * push;
        }
      }
      rows.forEach((row, index) => {
        const anchor = row.degree >= 8 ? 0.0024 : 0.0032;
        forces[index].x += (row.seedX - row.x) * anchor * cooling;
        forces[index].y += (row.seedY - row.y) * anchor * cooling;
        const communityAnchor = communityAnchors.get(row.community || `node:${row.id}`);
        if (communityAnchor && communityAnchor.count > 1) {
          forces[index].x += (communityAnchor.x - row.x) * 0.00032 * cooling;
          forces[index].y += (communityAnchor.y - row.y) * 0.00032 * cooling;
        }
        forces[index].x -= row.x * 0.000035;
        forces[index].y -= row.y * 0.000035;
      });
      rows.forEach((row, index) => {
        row.vx = Math.max(-maxStep, Math.min(maxStep, row.vx * 0.58 + forces[index].x));
        row.vy = Math.max(-maxStep, Math.min(maxStep, row.vy * 0.58 + forces[index].y));
        row.x += row.vx;
        row.y += row.vy;
      });
    }
    const hubLeafAdjustments = spreadHubLeaves();
    const longEdgeAdjustments = contractLongEdges(layoutDensity >= 1.6 ? 16 : 10);
    const afterRelax = summarizeLayout(rows);
    if (afterRelax.finite && afterRelax.minX != null && afterRelax.maxX != null && afterRelax.minY != null && afterRelax.maxY != null) {
      const cx = (afterRelax.minX + afterRelax.maxX) / 2;
      const cy = (afterRelax.minY + afterRelax.maxY) / 2;
      const width = Math.max(1, afterRelax.maxX - afterRelax.minX);
      const height = Math.max(1, afterRelax.maxY - afterRelax.minY);
      const targetWidth =
        rows.length <= 80
          ? mode === "large"
            ? Math.max(680, Math.min(880, Math.sqrt(rows.length) * 118))
            : Math.max(520, Math.min(640, Math.sqrt(rows.length) * 78))
          : Math.max(960, Math.min(2000, Math.sqrt(rows.length) * 150));
      const targetHeight = targetWidth * (rows.length <= 80 ? 0.68 : 0.62);
      const scaleX = Math.max(rows.length <= 80 ? 0.36 : 0.72, Math.min(1.6, targetWidth / width));
      const scaleY = Math.max(rows.length <= 80 ? 0.36 : 0.62, Math.min(1.6, targetHeight / height));
      if (Math.abs(scaleX - 1) > 0.04 || Math.abs(scaleY - 1) > 0.04) {
        rows.forEach((row) => {
          row.x = cx + (row.x - cx) * scaleX;
          row.y = cy + (row.y - cy) * scaleY;
        });
      }
      rows.forEach((row) => {
        const radial = row.degree <= 1 ? 1.03 : row.degree <= 3 ? 1.025 : row.degree <= 6 ? 1.012 : 1;
        if (radial <= 1) return;
        row.x = cx + (row.x - cx) * radial;
        row.y = cy + (row.y - cy) * radial;
      });
      const normalized = summarizeLayout(rows);
      if (normalized?.finite && normalized.minX != null && normalized.maxX != null && normalized.minY != null && normalized.maxY != null) {
        const ncx = (normalized.minX + normalized.maxX) / 2;
        const ncy = (normalized.minY + normalized.maxY) / 2;
        const nwidth = Math.max(1, normalized.maxX - normalized.minX);
        const nheight = Math.max(1, normalized.maxY - normalized.minY);
        const aspect = nwidth / nheight;
        if (aspect > 2.05 || aspect < 0.62) {
          const targetAspect = 1.55;
          const scaleX = aspect > targetAspect ? Math.max(0.52, Math.min(1, targetAspect / aspect)) : Math.min(1.32, targetAspect / aspect);
          const scaleY = aspect > targetAspect ? Math.min(1.28, Math.sqrt(aspect / targetAspect)) : Math.max(0.62, Math.min(1, aspect / targetAspect));
          rows.forEach((row) => {
            row.x = ncx + (row.x - ncx) * scaleX;
            row.y = ncy + (row.y - ncy) * scaleY;
          });
        }
        const distances = rows
          .map((row) => Math.hypot(row.x - ncx, row.y - ncy))
          .filter((value) => Number.isFinite(value))
          .sort((left, right) => left - right);
        const q90 = distances[Math.max(0, Math.min(distances.length - 1, Math.floor(distances.length * 0.9)))] || 0;
        const radiusLimit =
          rows.length <= 80
            ? mode === "large"
              ? Math.max(360, Math.min(760, q90 * 1.05 || 520))
              : Math.max(260, Math.min(480, q90 * 0.72 || 360))
            : Math.max(520, Math.min(1450, q90 * 1.18 || 900));
        rows.forEach((row) => {
          const dx = row.x - ncx;
          const dy = row.y - ncy;
          const dist = Math.hypot(dx, dy);
          if (!Number.isFinite(dist) || dist <= radiusLimit || dist <= 1) return;
          const scale = radiusLimit / dist;
          row.x = ncx + dx * scale;
          row.y = ncy + dy * scale;
        });
      }
    }
    const finalLongEdgeAdjustments = contractLongEdges(layoutDensity >= 1.6 ? 10 : 6);
    const finalCollisionAdjustments = resolveCollisions(rows.length <= 140 ? 3 : 2);
    const peripheralPullAdjustments = pullPeripheralNodesToNeighbors(rows.length <= 120 ? 3 : 2);
    const postPeripheralLongEdgeAdjustments = peripheralPullAdjustments ? contractLongEdges(layoutDensity >= 1.6 ? 8 : 4) : 0;
    const postPeripheralCollisionAdjustments = peripheralPullAdjustments ? resolveCollisions(rows.length <= 140 ? 2 : 1) : 0;
	    const compactRuntimeShell = () => {
	      if (mode === "large" || mode === "xlarge" || rows.length > 140) return 0;
      const summary = summarizeLayout(rows);
      if (!summary?.finite || summary.minX == null || summary.maxX == null || summary.minY == null || summary.maxY == null) {
        return 0;
      }
      const cx = (summary.minX + summary.maxX) / 2;
      const cy = (summary.minY + summary.maxY) / 2;
      const spanX = Math.max(1, summary.maxX - summary.minX);
      const spanY = Math.max(1, summary.maxY - summary.minY);
	      const scaleX = spanX > 620 ? 0.58 : spanX > 520 ? 0.64 : 0.74;
	      const scaleY = spanY > 420 ? 0.34 : spanY > 320 ? 0.4 : 0.52;
      rows.forEach((row) => {
        row.x = cx + (row.x - cx) * scaleX;
        row.y = cy + (row.y - cy) * scaleY;
      });
      return rows.length;
    };
    const compactShellAdjustments = compactRuntimeShell();
    const postCompactCollisionAdjustments = compactShellAdjustments ? resolveCollisions(1) : 0;
    const postCompactLongEdgeAdjustments = compactShellAdjustments ? contractLongEdges(layoutDensity >= 1.6 ? 8 : 4) : 0;
    const postCompactFinalCollisionAdjustments =
      compactShellAdjustments && postCompactLongEdgeAdjustments ? resolveCollisions(1) : 0;
	    const finalVisualPack = () => {
	      if (mode === "large" || mode === "xlarge" || rows.length > 140) return 0;
      const summary = summarizeLayout(rows);
      if (!summary?.finite || summary.minX == null || summary.maxX == null || summary.minY == null || summary.maxY == null) {
        return 0;
      }
      const cx = (summary.minX + summary.maxX) / 2;
      const cy = (summary.minY + summary.maxY) / 2;
      rows.forEach((row) => {
	        row.x = cx + (row.x - cx) * 0.88;
	        row.y = cy + (row.y - cy) * (rows.length > 80 ? 0.5 : 0.58);
      });
      return rows.length;
    };
    const finalVisualPackAdjustments = finalVisualPack();
    const openShortRuntimeEdges = (passes = 2) => {
      let adjusted = 0;
      for (let pass = 0; pass < passes; pass += 1) {
        edgeRows.forEach((edge) => {
          const sourceIndex = rowAt.get(edge.source);
          const targetIndex = rowAt.get(edge.target);
          if (sourceIndex == null || targetIndex == null) return;
          const source = rows[sourceIndex];
          const target = rows[targetIndex];
          let dx = target.x - source.x;
          let dy = target.y - source.y;
          if (Math.abs(dx) + Math.abs(dy) < 0.001) {
            const angle =
              Math.PI * 2 * (hashProjectionPointSeed(`network-runtime-short-edge|${source.id}|${target.id}`) / 0xffffffff);
            dx = Math.cos(angle) * 0.2;
            dy = Math.sin(angle) * 0.2;
          }
          const dist = Math.max(0.01, Math.hypot(dx, dy));
          const minDist = source.radius + target.radius + (edge.crossCommunity ? 66 : 52);
          if (dist >= minDist) return;
          const push = Math.min(42, (minDist - dist) * 0.46);
          const ux = dx / dist;
          const uy = dy / dist;
          const sourceDegree = Math.max(1, source.degree || 1);
          const targetDegree = Math.max(1, target.degree || 1);
          const sourceMove = targetDegree / (sourceDegree + targetDegree);
          const targetMove = sourceDegree / (sourceDegree + targetDegree);
          source.x -= ux * push * sourceMove;
          source.y -= uy * push * sourceMove;
          target.x += ux * push * targetMove;
          target.y += uy * push * targetMove;
          adjusted += 1;
        });
      }
      return adjusted;
    };
    const finalShortEdgeAdjustments =
      rows.length <= 140 ? openShortRuntimeEdges(mode === "large" ? 3 : finalVisualPackAdjustments ? 2 : 0) : 0;
    const dataNodeById = new Map(
      (Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [])
        .map((node) => [String(node?.id || "").trim(), node])
        .filter(([id]) => !!id)
    );
    graph.setAutoPaint?.(false);
    try {
      rows.forEach((row) => {
        row.model.x = row.x;
        row.model.y = row.y;
        const dataNode = dataNodeById.get(row.id);
        if (dataNode) {
          dataNode.x = row.x;
          dataNode.y = row.y;
        }
        graph.refreshItem?.(row.item);
      });
      invalidateNetworkRuntimeEdgeGeometry(graph, `${options?.reason || "network-runtime-relax"}:node-position-update`);
      graph.refreshPositions?.();
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    const after = summarizeLayout(rows);
    recordNetworkRuntimePerfEvent("runtime-skeleton-relaxed", {
      reason: String(options?.reason || ""),
      mode,
      nodes: rows.length,
      edges: edgeRows.length,
      hubLeafAdjustments,
      longEdgeAdjustments,
      finalLongEdgeAdjustments,
      finalCollisionAdjustments,
      peripheralPullAdjustments,
      postPeripheralLongEdgeAdjustments,
      postPeripheralCollisionAdjustments,
      compactShellAdjustments,
      postCompactCollisionAdjustments,
      postCompactLongEdgeAdjustments,
      postCompactFinalCollisionAdjustments,
      finalVisualPackAdjustments,
      finalShortEdgeAdjustments,
      durationMs: roundNetworkPerfMs(networkPerfNow() - startedAt),
      before: before?.finite
        ? { minX: before.minX, maxX: before.maxX, minY: before.minY, maxY: before.maxY }
        : null,
      after: after?.finite ? { minX: after.minX, maxX: after.maxX, minY: after.minY, maxY: after.maxY } : null,
    });
    return true;
  }

  function scheduleNetworkRuntimeSkeletonPolish(graph, options = {}) {
    return false;
    const mode = String(options?.mode || state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (state.layoutPreset !== "network" || !isLargeNetworkTopologyMode(mode)) return false;
    const reason = String(options?.reason || "network-runtime-polish");
    const iterations = Math.max(2, Math.min(8, Number(options?.iterations) || 2));
    schedulePostPaintTask(() => {
      if (state.layoutPreset !== "network") return;
      const runtimeGraph = graph === state.graph?.instance ? graph : ensureGraph();
      if (!runtimeGraph) return;
      const startedAt = networkPerfNow();
      relaxNetworkRuntimeSkeletonGraph(runtimeGraph, {
        mode,
        reason,
        iterations,
      });
      const spread = mode === "xlarge"
        ? spreadNetworkRuntimeCollapsedCommunities(runtimeGraph, `${reason}:post-polish-spread`)
        : false;
      redrawNetworkRuntimeSkeletonGraph(runtimeGraph, `${reason}:post-polish-redraw`);
      suppressNetworkRuntimeOverlappingLabels(runtimeGraph, {
        maxLabels: mode === "xlarge" ? 8 : 12,
        padding: mode === "xlarge" ? 14 : 12,
      });
      centerNetworkRuntimeSkeletonAfterExpand(runtimeGraph, reason);
      const bounds = networkViewportBounds(runtimeGraph, mode === "xlarge" ? 280 : 220);
      const viewportQuality = collectNetworkRuntimeQualityReport(runtimeGraph, {
        mode,
        scope: "viewport",
        bounds,
        reason: `${reason}:post-polish`,
      });
      state.networkLayout.lastViewportQuality = viewportQuality;
      state.networkLayout.lastViewportRefine = {
        ...(state.networkLayout.lastViewportRefine || {}),
        reason: `${reason}:post-polish`,
        mode,
        quality: viewportQuality,
        at: Date.now(),
      };
      const semanticState = ensureLayoutSemanticState();
      semanticState.lastReports = {
        ...(semanticState.lastReports || {}),
        networkViewport: {
          mode,
          quality: viewportQuality,
          report: {
            reason: `${reason}:post-polish`,
            spread,
          },
        },
      };
      updateMinimap();
      recordNetworkRuntimePerfEvent("runtime-skeleton-polished", {
        reason,
        mode,
        spread,
        durationMs: networkPerfNow() - startedAt,
      });
    }, { timeout: 48 });
    return true;
  }

  function capNetworkRuntimeSkeletonGraph(graph, options = {}) {
    return false;
    if (state.layoutPreset !== "network") return false;
    const mode = String(options?.mode || state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (mode !== "xlarge") return false;
    const nodeItems = Array.isArray(graph?.getNodes?.()) ? graph.getNodes() : [];
    const edgeItems = Array.isArray(graph?.getEdges?.()) ? graph.getEdges() : [];
    const maxNodes = Math.max(64, Math.min(360, Number(options?.maxNodes) || 260));
    const maxEdges = Math.max(160, Math.min(1200, Number(options?.maxEdges) || resolveNetworkRuntimeEdgeBudget(mode)));
    if (nodeItems.length <= maxNodes && edgeItems.length <= maxEdges) return false;
    const priorityIds = new Set(
      parseUniqueList(
        [
          ...(Array.isArray(options?.priorityNodeIds) ? options.priorityNodeIds : []),
          ...(parseUniqueList(state.graphProjection?.path_node_ids || state.graphProjection?.pathNodeIds || [], {
            allowString: true,
          })),
          ...Array.from(state.selected || []),
          state.focusId || "",
        ],
        { allowString: true }
      )
    );
    const degree = new Map();
    edgeItems.forEach((item) => {
      const model = item?.getModel?.() || {};
      const source = String(model.source || "").trim();
      const target = String(model.target || "").trim();
      if (source) degree.set(source, (degree.get(source) || 0) + 1);
      if (target) degree.set(target, (degree.get(target) || 0) + 1);
    });
    const rankedNodes = nodeItems
      .map((item, index) => {
        const model = item?.getModel?.() || {};
        const id = String(model?.id || "").trim();
        const syntheticCluster = id.startsWith("__cluster__::") ? 1 : 0;
        const visibleTier = String(model.layoutVisibilityTier || "");
        const labelTier = String(model.layoutLabelTier || "");
        const priority = priorityIds.has(id) ? 100000 : 0;
        const tierScore = visibleTier === "primary" ? 9000 : visibleTier === "secondary" ? 4500 : 0;
        const labelScore = labelTier === "primary" ? 1200 : 0;
        return {
          item,
          model,
          id,
          index,
          score:
            priority +
            tierScore +
            labelScore +
            (degree.get(id) || 0) * 180 +
            rankNetworkCoarseNode(model) -
            syntheticCluster * 240,
        };
      })
      .filter((row) => row.id)
      .sort((left, right) => right.score - left.score || left.id.localeCompare(right.id, "zh-CN") || left.index - right.index);
    let keepNodeIds = new Set(rankedNodes.slice(0, maxNodes).map((row) => row.id));
    priorityIds.forEach((id) => {
      if (id && rankedNodes.some((row) => row.id === id)) keepNodeIds.add(id);
    });
    const rankedEdges = edgeItems
      .map((item, index) => {
        const model = item?.getModel?.() || {};
        const source = String(model.source || "").trim();
        const target = String(model.target || "").trim();
        const weight =
          Number(model.weight) ||
          Number(model.amount) ||
          Number(model.total_amount) ||
          Number(model.totalAmount) ||
          Number(model.count) ||
          1;
        return {
          item,
          model,
          index,
          source,
          target,
          score: Math.log1p(Math.max(1, weight)) + (degree.get(source) || 0) * 0.08 + (degree.get(target) || 0) * 0.08,
        };
      })
      .filter((row) => row.source && row.target && keepNodeIds.has(row.source) && keepNodeIds.has(row.target))
      .sort((left, right) => right.score - left.score || left.index - right.index);
    let selectedEdgeRows = [];
    const deferredEdgeRows = [];
    const endpointEdgeCounts = new Map();
    const edgeEndpointLimit = Math.max(28, Math.min(76, Math.ceil(maxEdges / Math.sqrt(Math.max(1, maxNodes))) + 4));
    const canTakeEdge = (row, relaxed = false) => {
      const limit = relaxed ? edgeEndpointLimit * 2 : edgeEndpointLimit;
      return (endpointEdgeCounts.get(row.source) || 0) < limit && (endpointEdgeCounts.get(row.target) || 0) < limit;
    };
    const takeEdge = (row) => {
      selectedEdgeRows.push(row);
      endpointEdgeCounts.set(row.source, (endpointEdgeCounts.get(row.source) || 0) + 1);
      endpointEdgeCounts.set(row.target, (endpointEdgeCounts.get(row.target) || 0) + 1);
    };
    rankedEdges.forEach((row) => {
      if (selectedEdgeRows.length >= maxEdges) return;
      if (canTakeEdge(row)) takeEdge(row);
      else deferredEdgeRows.push(row);
    });
    const minSelectedEdges = Math.min(maxEdges, Math.ceil(maxEdges * 0.72), rankedEdges.length);
    deferredEdgeRows.some((row) => {
      if (selectedEdgeRows.length >= minSelectedEdges) return true;
      if (canTakeEdge(row, true)) takeEdge(row);
      return false;
    });
    deferredEdgeRows.some((row) => {
      if (selectedEdgeRows.length >= minSelectedEdges) return true;
      if (selectedEdgeRows.includes(row)) return false;
      takeEdge(row);
      return false;
    });
    const incidentNodeIds = new Set();
    selectedEdgeRows.forEach((row) => {
      if (row.source) incidentNodeIds.add(row.source);
      if (row.target) incidentNodeIds.add(row.target);
    });
    if (incidentNodeIds.size) {
      const priorityKeepIds = new Set(
        Array.from(priorityIds).filter((id) => id && incidentNodeIds.has(id) && rankedNodes.some((row) => row.id === id))
      );
      let connectedNodeIds = new Set([...incidentNodeIds, ...priorityKeepIds]);
      if (mode === "xlarge" && selectedEdgeRows.length) {
        const adjacency = new Map();
        selectedEdgeRows.forEach((row) => {
          if (!connectedNodeIds.has(row.source) || !connectedNodeIds.has(row.target)) return;
          if (!adjacency.has(row.source)) adjacency.set(row.source, new Set());
          if (!adjacency.has(row.target)) adjacency.set(row.target, new Set());
          adjacency.get(row.source).add(row.target);
          adjacency.get(row.target).add(row.source);
        });
        const visited = new Set();
        const componentById = new Map();
        const componentRows = [];
        Array.from(adjacency.keys())
          .sort((left, right) => left.localeCompare(right, "zh-CN"))
          .forEach((id) => {
            if (visited.has(id)) return;
            const queue = [id];
            const nodes = [];
            visited.add(id);
            while (queue.length) {
              const next = queue.shift();
              nodes.push(next);
              (adjacency.get(next) || []).forEach((neighbor) => {
                if (visited.has(neighbor)) return;
                visited.add(neighbor);
                queue.push(neighbor);
              });
            }
            const key = `component:${componentRows.length}`;
            nodes.forEach((nodeId) => componentById.set(nodeId, key));
            componentRows.push({
              key,
              nodes,
              priority: nodes.some((nodeId) => priorityKeepIds.has(nodeId)),
              edgeCount: 0,
            });
          });
        selectedEdgeRows.forEach((row) => {
          const key = componentById.get(row.source);
          if (!key || key !== componentById.get(row.target)) return;
          const component = componentRows.find((entry) => entry.key === key);
          if (component) component.edgeCount += 1;
        });
        if (componentRows.length > 4) {
          const allowedComponents = new Set(
            componentRows
              .sort((left, right) => {
                if (left.priority !== right.priority) return left.priority ? -1 : 1;
                return right.nodes.length - left.nodes.length || right.edgeCount - left.edgeCount || left.key.localeCompare(right.key);
              })
              .slice(0, 4)
              .map((entry) => entry.key)
          );
          const limitedNodeIds = new Set();
          componentRows.forEach((entry) => {
            if (!allowedComponents.has(entry.key)) return;
            entry.nodes.forEach((nodeId) => limitedNodeIds.add(nodeId));
          });
          const limitedEdgeRows = selectedEdgeRows.filter(
            (row) => limitedNodeIds.has(row.source) && limitedNodeIds.has(row.target)
          );
          if (limitedNodeIds.size >= 16 && limitedEdgeRows.length >= Math.min(16, selectedEdgeRows.length)) {
            connectedNodeIds = limitedNodeIds;
            selectedEdgeRows = limitedEdgeRows;
          }
        }
      }
      if (connectedNodeIds.size > maxNodes) {
        connectedNodeIds = new Set(
          rankedNodes
            .filter((row) => connectedNodeIds.has(row.id))
            .slice(0, maxNodes)
            .map((row) => row.id)
        );
        priorityKeepIds.forEach((id) => {
          if (id) connectedNodeIds.add(id);
        });
      }
      keepNodeIds = connectedNodeIds;
    }
    const keepEdgeItems = new Set(
      selectedEdgeRows
        .filter((row) => keepNodeIds.has(row.source) && keepNodeIds.has(row.target))
        .map((row) => row.item)
    );
    let removedNodes = 0;
    let removedEdges = 0;
    graph.setAutoPaint?.(false);
    try {
      edgeItems.forEach((item) => {
        const model = item?.getModel?.() || {};
        const source = String(model.source || "").trim();
        const target = String(model.target || "").trim();
        if (keepEdgeItems.has(item) && keepNodeIds.has(source) && keepNodeIds.has(target)) return;
        graph.removeItem?.(item);
        removedEdges += 1;
      });
      nodeItems.forEach((item) => {
        const id = String(item?.getModel?.()?.id || "").trim();
        if (id && keepNodeIds.has(id)) return;
        graph.removeItem?.(item);
        removedNodes += 1;
      });
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    const nextProjection = cloneGraphProjection(options?.projection || state.graphProjection);
    state.graphProjection = nextProjection;
    replaceRuntimeGraphData(
      snapshotGraphDataFromRuntime(graph, {
        graphTier: state.graphTier || "",
        renderHints: state.renderHints || null,
        projection: cloneGraphProjection(nextProjection),
        resultSnapshotRef: cloneResultSnapshotRef(state.resultSnapshotRef),
        runtimeRevision: state.graph?.runtimeRevision,
      }),
      { reason: `network-runtime-cap:${options?.reason || "xlarge"}`, preserveGraphMeta: true }
    );
    recordNetworkRuntimePerfEvent("runtime-skeleton-capped", {
      reason: String(options?.reason || ""),
      mode,
      nodes: graph.getNodes?.().length || 0,
      edges: graph.getEdges?.().length || 0,
      edgeEndpointLimit,
      removedNodes,
      removedEdges,
      sourceNodes: nodeItems.length,
      sourceEdges: edgeItems.length,
    });
    updateMinimap();
    updateGraphStats();
    return true;
  }

  function renderNetworkCoarseSkeletonToGraph(nodes = [], edges = [], mode = "", reason = "") {
    return false;
    if (state.layoutPreset !== "network") return false;
    const normalizedMode = String(mode || "").trim().toLowerCase();
    if (normalizedMode !== "xlarge") return false;
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const edgeRows = Array.isArray(edges) ? edges : [];
    if (!nodeRows.length) return false;
    const graph = ensureGraph();
    if (!graph || typeof graph.data !== "function" || typeof graph.render !== "function") return false;
    const renderEdges = selectNetworkSkeletonRenderEdges(
      nodeRows,
      edgeRows,
      resolveNetworkSkeletonEdgeBudget(normalizedMode, nodeRows.length, edgeRows.length),
      { includeIncidentEdges: true }
    );
    const renderNodes = selectNetworkSkeletonRenderNodes(nodeRows, renderEdges, normalizedMode).map((node) =>
      toNetworkSkeletonRenderNode(node)
    );
    if (!renderNodes.length) return false;
    const renderNodeIds = new Set(renderNodes.map((node) => String(node?.id || "").trim()).filter(Boolean));
    const boundedRenderEdges = filterNetworkSkeletonRenderEdgesToNodes(renderEdges, renderNodeIds);
    const canSetAutoPaint = typeof graph.setAutoPaint === "function";
    if (canSetAutoPaint) graph.setAutoPaint(false);
    try {
      graph.data({ nodes: renderNodes, edges: boundedRenderEdges });
    } finally {
      if (canSetAutoPaint) graph.setAutoPaint(true);
    }
    state.networkLayout.activeMode = "skeleton";
    graph.render();
    applyNetworkPlanPresentationToGraph(graph, renderNodes, boundedRenderEdges, { mode: normalizedMode });
    revealNetworkRuntimeSkeletonEdges(graph, { mode: normalizedMode, reason: `${reason || "network-coarse"}:coarse-skeleton` });
    relaxNetworkRuntimeSkeletonGraph(graph, { mode: normalizedMode, reason: `${reason || "network-coarse"}:coarse-skeleton` });
    fitGraph({ force: true });
    centerGraphOnLayout(
      graph,
      collectNetworkRuntimeNodeModels(graph),
      Math.max(0.28, Math.min(0.9, Number(graph.getZoom?.()) || 0.55))
    );
    updateMinimap();
    recordNetworkRuntimePerfEvent("coarse-skeleton-rendered", {
      reason: String(reason || ""),
      mode: normalizedMode,
      nodes: renderNodes.length,
      totalNodes: nodeRows.length,
      edges: renderEdges.length,
      renderedEdges: boundedRenderEdges.length,
      totalEdges: edgeRows.length,
    });
    return true;
  }

  function summarizeNetworkSkeletonTextCandidates(nodes = [], edges = []) {
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const edgeRows = Array.isArray(edges) ? edges : [];
    let nodeTextRows = 0;
    let hiddenNodeLabels = 0;
    let iconRows = 0;
    nodeRows.forEach((node) => {
      const hidden = !!node?.nodeLabelHidden || String(node?.layoutLabelTier || "") === "hidden";
      if (hidden) {
        hiddenNodeLabels += 1;
      } else {
        const name = String(node?.name || "").trim();
        const title = String(node?.title || node?.label || node?.id || "").trim();
        const displayId = String(node?.displayId || node?.display_id || "").trim();
        if (name || title) nodeTextRows += 1;
        if (displayId) nodeTextRows += 1;
      }
      if (String(node?.iconSymbol || "").trim() || node?.icon) iconRows += 1;
    });
    let edgeTextRows = 0;
    edgeRows.forEach((edge) => {
      if (edge?.label) edgeTextRows += 1;
      if (edge?.labelTop) edgeTextRows += 1;
      if (edge?.labelBottom) edgeTextRows += 1;
    });
    return {
      nodes: nodeRows.length,
      edges: edgeRows.length,
      hiddenNodeLabels,
      nodeTextRows,
      edgeTextRows,
      iconRows,
      totalTextRows: nodeTextRows + edgeTextRows + iconRows,
    };
  }

  function networkViewportBounds(graph, paddingPx = 220) {
    const viewport = captureGraphViewport(graph);
    if (!viewport) return null;
    const zoom = Math.max(0.05, Number(viewport?.zoom) || Number(graph?.getZoom?.()) || 1);
    const size = viewport?.size && typeof viewport.size === "object" ? viewport.size : {};
    const width = Number(size.width) || Number(graph?.get?.("width")) || 0;
    const height = Number(size.height) || Number(graph?.get?.("height")) || 0;
    if (typeof graph?.getPointByCanvas === "function" && width > 0 && height > 0) {
      try {
        const pad = Math.max(0, Number(paddingPx) || 0);
        const corners = [
          graph.getPointByCanvas(-pad, -pad),
          graph.getPointByCanvas(width + pad, -pad),
          graph.getPointByCanvas(width + pad, height + pad),
          graph.getPointByCanvas(-pad, height + pad),
        ].filter((point) => point && Number.isFinite(Number(point.x)) && Number.isFinite(Number(point.y)));
        if (corners.length) {
          return {
            minX: Math.min(...corners.map((point) => Number(point.x))),
            maxX: Math.max(...corners.map((point) => Number(point.x))),
            minY: Math.min(...corners.map((point) => Number(point.y))),
            maxY: Math.max(...corners.map((point) => Number(point.y))),
          };
        }
      } catch (_error) {}
    }
    const center = viewport?.center && typeof viewport.center === "object" ? viewport.center : {};
    const cx = Number(center.x);
    const cy = Number(center.y);
    if (!Number.isFinite(cx) || !Number.isFinite(cy) || width <= 0 || height <= 0) return null;
    const padWorld = Math.max(0, Number(paddingPx) || 0) / zoom;
    const halfW = width / zoom / 2 + padWorld;
    const halfH = height / zoom / 2 + padWorld;
    return { minX: cx - halfW, maxX: cx + halfW, minY: cy - halfH, maxY: cy + halfH };
  }

  function networkViewportSignature(bounds, mode) {
    if (!bounds) return "";
    return [
      String(mode || ""),
      Math.round(Number(bounds.minX) / 80),
      Math.round(Number(bounds.minY) / 80),
      Math.round(Number(bounds.maxX) / 80),
      Math.round(Number(bounds.maxY) / 80),
      Number(state.graphMutationSeq || 0),
      Number(state.networkLayout?.planSeq || 0),
    ].join(":");
  }

  function isNetworkNodeInBounds(model, bounds) {
    if (!model || !bounds) return false;
    const x = Number(model.x);
    const y = Number(model.y);
    return (
      Number.isFinite(x) &&
      Number.isFinite(y) &&
      x >= bounds.minX &&
      x <= bounds.maxX &&
      y >= bounds.minY &&
      y <= bounds.maxY
    );
  }

  function isNetworkViewportPriorityNode(item, searchText = "") {
    const model = item?.getModel?.() || {};
    const id = String(model?.id || "").trim();
    if (!id) return false;
    if (item?.hasState?.("selected") || item?.hasState?.("active") || item?.hasState?.("hover")) return true;
    const focusIds = parseUniqueList(
      [
        state.focusId,
        ...(state.focusIds || []),
        ...(state.selected || []),
        ...parseUniqueList(state.graphProjection?.path_node_ids || state.graphProjection?.pathNodeIds || [], {
          allowString: true,
        }),
      ],
      {
        allowString: true,
      }
    );
    if (focusIds.includes(id)) return true;
    const query = normalizeKey(searchText || state.graphSearchQuery || state.search || "");
    if (!query) return false;
    return [model.id, model.name, model.title, model.label, model.displayId, model.display_id]
      .map((value) => normalizeKey(value))
      .some((value) => value && value.includes(query));
  }

  function isNetworkViewportPriorityDataNode(node, searchText = "") {
    const id = String(node?.id || "").trim();
    if (!id) return false;
    const focusIds = parseUniqueList(
      [
        state.focusId,
        ...(state.focusIds || []),
        ...(state.selected || []),
        ...parseUniqueList(state.graphProjection?.path_node_ids || state.graphProjection?.pathNodeIds || [], {
          allowString: true,
        }),
      ],
      {
        allowString: true,
      }
    );
    if (focusIds.includes(id)) return true;
    const query = normalizeKey(searchText || state.graphSearchQuery || state.search || "");
    if (!query) return false;
    return [node.id, node.name, node.title, node.label, node.displayId, node.display_id]
      .map((value) => normalizeKey(value))
      .some((value) => value && value.includes(query));
  }

  function revealNetworkNodeForViewport(graph, item, dataNode, { showLabel = false } = {}) {
    const model = item?.getModel?.() || {};
    const baseR = Number.isFinite(Number(model.__networkBaseR)) ? Number(model.__networkBaseR) : Number(model.r) || 18;
    const baseLineWidth = Number.isFinite(Number(model.__networkBaseLineWidth))
      ? Number(model.__networkBaseLineWidth)
      : Number(model.lineWidth) || 2;
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    const visibleRadius = mode === "xlarge" ? baseR + 1.2 : baseR;
    const visibleLineWidth = mode === "xlarge" ? Math.max(baseLineWidth, 2.55) : baseLineWidth;
    if (model.__networkBaseName == null) model.__networkBaseName = model.name ?? dataNode?.name ?? "";
    if (model.__networkBaseTitle == null) model.__networkBaseTitle = model.title ?? dataNode?.title ?? "";
    if (model.__networkBaseLabel == null) model.__networkBaseLabel = model.label ?? dataNode?.label ?? "";
    if (model.__networkBaseDisplayId == null) model.__networkBaseDisplayId = model.displayId ?? dataNode?.displayId ?? "";
    if (model.__networkBaseDisplayIdSnake == null) {
      model.__networkBaseDisplayIdSnake = model.display_id ?? dataNode?.display_id ?? "";
    }
    if (model.__networkBaseIcon == null) model.__networkBaseIcon = model.icon ?? dataNode?.icon ?? "";
    if (model.__networkBaseIconSymbol == null) model.__networkBaseIconSymbol = model.iconSymbol ?? dataNode?.iconSymbol ?? "";
    if (model.__networkBaseIconSize == null) model.__networkBaseIconSize = Number(model.iconSize ?? dataNode?.iconSize ?? 0) || 0;
    if (model.__networkBaseFontShadow == null) model.__networkBaseFontShadow = !!(model.fontShadow ?? dataNode?.fontShadow);
    if (dataNode && typeof dataNode === "object") {
      if (String(dataNode.layoutVisibilityTier || "") === "hidden") dataNode.layoutVisibilityTier = "secondary";
      if (showLabel && String(dataNode.layoutLabelTier || "") === "hidden") dataNode.layoutLabelTier = "secondary";
      dataNode.layoutHiddenBySkeleton = false;
      if (showLabel) dataNode.nodeLabelHidden = false;
    }
    graph.updateItem?.(item, {
      r: visibleRadius,
      lineWidth: visibleLineWidth,
      stroke: String(model.__networkBaseStroke || model.stroke || ""),
      fill: String(model.__networkBaseFill || model.fill || ""),
      nodeShadow: !!model.__networkBaseNodeShadow,
      layoutHiddenBySkeleton: false,
      nodeLabelHidden: showLabel ? false : !!model.nodeLabelHidden,
      layoutVisibilityTier: String(dataNode?.layoutVisibilityTier || model.layoutVisibilityTier || "secondary"),
      layoutLabelTier: String(dataNode?.layoutLabelTier || model.layoutLabelTier || ""),
      name: showLabel ? dataNode?.name ?? model.__networkBaseName : "",
      title: showLabel ? dataNode?.title ?? model.__networkBaseTitle : "",
      label: showLabel ? dataNode?.label ?? model.__networkBaseLabel : "",
      displayId: showLabel ? dataNode?.displayId ?? model.__networkBaseDisplayId : "",
      display_id: showLabel ? dataNode?.display_id ?? model.__networkBaseDisplayIdSnake : "",
      icon: showLabel ? dataNode?.icon ?? model.__networkBaseIcon : "",
      iconSymbol: showLabel ? dataNode?.iconSymbol ?? model.__networkBaseIconSymbol : "",
      iconSize: showLabel ? Number(dataNode?.iconSize ?? model.__networkBaseIconSize) || 0 : 0,
      fontShadow: showLabel ? !!model.__networkBaseFontShadow : false,
    });
  }

  function revealNetworkEdgeForViewport(graph, item, dataEdge) {
    const model = item?.getModel?.() || {};
    const baseLineWidth = Number.isFinite(Number(model.__networkBaseLineWidth))
      ? Number(model.__networkBaseLineWidth)
      : Number(model.lineWidth) || 1.6;
    const rawTier = String(dataEdge?.layoutEdgeTier || model.layoutEdgeTier || "secondary");
    const tier = rawTier === "hidden" ? "secondary" : rawTier || "secondary";
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    const largeTopologyMode = isLargeNetworkTopologyMode(mode);
    const rawVisibleLineWidth = Math.max(
      baseLineWidth,
      largeTopologyMode ? (tier === "primary" ? 0.9 : 0.72) : tier === "primary" ? 1.08 : 0.82
    );
    const visibleLineWidth = largeTopologyMode ? Math.min(mode === "large" ? 1.32 : 1.12, rawVisibleLineWidth) : rawVisibleLineWidth;
    const visibleStroke = largeTopologyMode
      ? tier === "primary"
        ? "rgba(15,23,42,0.58)"
        : "rgba(30,41,59,0.42)"
      : String(model.__networkBaseStroke || dataEdge?.stroke || model.stroke || state.graphStyle?.edgeColor || "").trim()
        || "rgba(30,41,59,0.34)";
    if (dataEdge && typeof dataEdge === "object") {
      if (String(dataEdge.layoutEdgeTier || "") === "hidden") dataEdge.layoutEdgeTier = "secondary";
      dataEdge.layoutHiddenBySkeleton = false;
    }
    graph.updateItem?.(item, {
      stroke: visibleStroke,
      lineWidth: visibleLineWidth,
      showArrow: largeTopologyMode ? false : !!model.__networkBaseShowArrow,
      layoutHiddenBySkeleton: false,
      layoutEdgeTier: tier,
      layoutEdgeBundleRouted: !!(dataEdge?.layoutEdgeBundleRouted || model.layoutEdgeBundleRouted),
      edgeOffset: Number(dataEdge?.edgeOffset || model.edgeOffset || 0) || 0,
      edgeRoute: String(dataEdge?.edgeRoute || model.edgeRoute || ""),
      orthAxis: String(dataEdge?.orthAxis || model.orthAxis || ""),
      orthBias: Number(dataEdge?.orthBias || model.orthBias || 0) || 0,
      edgeLabelAnchorMode: String(dataEdge?.edgeLabelAnchorMode || model.edgeLabelAnchorMode || ""),
    });
  }

  function applyNetworkViewportRefinement({ reason = "", force = false } = {}) {
    return false;
    const refineStartedAt = networkPerfNow();
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (state.layoutPreset !== "network" || !isLargeNetworkTopologyMode(mode)) return false;
    const graph = ensureGraph();
    if (!graph) return false;
    const bounds = networkViewportBounds(graph, mode === "xlarge" ? 280 : 220);
    if (!bounds) return false;
    const signature = networkViewportSignature(bounds, mode);
    const cooldownMs = 260;
    if (
      !force &&
      signature &&
      signature === String(state.networkLayout?.viewportRefineSignature || "") &&
      Date.now() - Number(state.networkLayout?.viewportRefineLastAt || 0) < cooldownMs
    ) {
      return false;
    }
    const nodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
    const edges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    const nodeById = new Map(nodes.map((node) => [String(node?.id || ""), node]));
    const edgeById = new Map(edges.map((edge) => [String(edge?.id || ""), edge]));
    const edgeByPair = new Map(
      edges.map((edge) => {
        const source = typeof edge?.source === "object" ? edge.source?.id : edge?.source;
        const target = typeof edge?.target === "object" ? edge.target?.id : edge?.target;
        return [networkEdgeKey(source, target), edge];
      })
    );
    const nodeLimit = mode === "xlarge" ? 48 : 320;
    const labelLimit = mode === "xlarge" ? 6 : 12;
    const materializeNodeLimit = mode === "xlarge" ? 32 : 0;
    const candidates = [];
    const runtimeNodeIds = new Set(
      (graph.getNodes?.() || [])
        .map((item) => String(item?.getModel?.()?.id || "").trim())
        .filter(Boolean)
    );
    (graph.getNodes?.() || []).forEach((item) => {
      const model = item?.getModel?.() || {};
      const id = String(model?.id || "").trim();
      if (!id) return;
      const hidden = !!model.layoutHiddenBySkeleton || String(model.layoutVisibilityTier || "") === "hidden";
      const labelHidden = !!model.nodeLabelHidden || String(model.layoutLabelTier || "") === "hidden";
      if (!hidden && !labelHidden) return;
      const priority = isNetworkViewportPriorityNode(item);
      if (!priority && !isNetworkNodeInBounds(model, bounds)) return;
      const bridgeScore = Number(model.layoutBridgeScore) || 0;
      candidates.push({ item, id, priority, bridgeScore, hidden, labelHidden });
    });
    const materializeCandidates = [];
    if (mode === "xlarge" && materializeNodeLimit > 0) {
      nodes.forEach((node) => {
        const id = String(node?.id || "").trim();
        if (!id || runtimeNodeIds.has(id)) return;
        const hidden = String(node?.layoutVisibilityTier || "") === "hidden" || !!node?.layoutHiddenBySkeleton;
        if (!hidden) return;
        const priority = isNetworkViewportPriorityDataNode(node);
        if (!priority && !isNetworkNodeInBounds(node, bounds)) return;
        materializeCandidates.push({
          node,
          id,
          priority,
          bridgeScore: Number(node?.layoutBridgeScore) || 0,
        });
      });
      materializeCandidates.sort((left, right) => {
        if (left.priority !== right.priority) return left.priority ? -1 : 1;
        return right.bridgeScore - left.bridgeScore || left.id.localeCompare(right.id);
      });
    }
    candidates.sort((left, right) => {
      if (left.priority !== right.priority) return left.priority ? -1 : 1;
      return right.bridgeScore - left.bridgeScore || left.id.localeCompare(right.id);
    });
    const revealIds = new Set();
    const materializedIds = new Set();
    let labelCount = 0;
    let materializedEdges = 0;
    graph.setAutoPaint?.(false);
    try {
      candidates.slice(0, nodeLimit).forEach((entry) => {
        const showLabel = entry.priority || (entry.labelHidden && labelCount < labelLimit);
        if (showLabel) labelCount += 1;
        revealIds.add(entry.id);
        revealNetworkNodeForViewport(graph, entry.item, nodeById.get(entry.id), { showLabel });
      });
      materializeCandidates.slice(0, Math.max(0, materializeNodeLimit - revealIds.size)).forEach((entry) => {
        const showLabel = entry.priority || labelCount < labelLimit;
        if (showLabel) labelCount += 1;
        if (String(entry.node.layoutVisibilityTier || "") === "hidden") entry.node.layoutVisibilityTier = "secondary";
        if (showLabel && String(entry.node.layoutLabelTier || "") === "hidden") {
          entry.node.layoutLabelTier = "secondary";
        }
        entry.node.layoutHiddenBySkeleton = false;
        entry.node.nodeLabelHidden = !showLabel;
        const renderNode = showLabel ? entry.node : toNetworkSkeletonRenderNode(entry.node);
        const item = graph.addItem?.("node", renderNode);
        if (!item) return;
        runtimeNodeIds.add(entry.id);
        revealIds.add(entry.id);
        materializedIds.add(entry.id);
        revealNetworkNodeForViewport(graph, item, entry.node, { showLabel });
      });
      let edgeCount = 0;
      const edgeLimit = mode === "xlarge" ? 64 : 460;
      (graph.getEdges?.() || []).forEach((item) => {
        if (edgeCount >= edgeLimit) return;
        const model = item?.getModel?.() || {};
        if (!model.layoutHiddenBySkeleton && String(model.layoutEdgeTier || "") !== "hidden") return;
        const source = String(model.source || "");
        const target = String(model.target || "");
        if (!revealIds.has(source) && !revealIds.has(target)) return;
        const dataEdge =
          edgeById.get(String(model.id || "")) || edgeByPair.get(networkEdgeKey(model.source, model.target));
        revealNetworkEdgeForViewport(graph, item, dataEdge);
        edgeCount += 1;
      });
      if (materializedIds.size) {
        const materializedEdgeEndpointCounts = new Map();
        let materializedCrossCommunityEdges = 0;
        const canMaterializeEdge = (row) => {
          if (mode !== "xlarge") return true;
          return (
            (materializedEdgeEndpointCounts.get(row.source) || 0) < 4 &&
            (materializedEdgeEndpointCounts.get(row.target) || 0) < 4 &&
            (!row.crossCommunity || materializedCrossCommunityEdges < 18)
          );
        };
        const rememberMaterializedEdge = (row) => {
          if (mode !== "xlarge") return;
          materializedEdgeEndpointCounts.set(row.source, (materializedEdgeEndpointCounts.get(row.source) || 0) + 1);
          materializedEdgeEndpointCounts.set(row.target, (materializedEdgeEndpointCounts.get(row.target) || 0) + 1);
          if (row.crossCommunity) materializedCrossCommunityEdges += 1;
        };
        const rankedEdges = edges
          .filter((edge) => {
            const id = String(edge?.id || "").trim();
            if (id && graph.findById?.(id)) return false;
            const source = networkPlanEndpoint(edge?.source);
            const target = networkPlanEndpoint(edge?.target);
            if (!source || !target) return false;
            if (!runtimeNodeIds.has(source) || !runtimeNodeIds.has(target)) return false;
            return materializedIds.has(source) || materializedIds.has(target);
          })
          .map((edge, index) => ({
            edge,
            index,
            source: networkPlanEndpoint(edge?.source),
            target: networkPlanEndpoint(edge?.target),
            weight:
              Number(edge?.weight) ||
              Number(edge?.amount) ||
              Number(edge?.total_amount) ||
              Number(edge?.totalAmount) ||
              Number(edge?.count) ||
              0,
          }))
          .map((row) => {
            const sourceNode = nodeById.get(row.source) || {};
            const targetNode = nodeById.get(row.target) || {};
            const sourceCommunity = String(sourceNode?.layoutCommunity || sourceNode?.clusterId || row.source || "");
            const targetCommunity = String(targetNode?.layoutCommunity || targetNode?.clusterId || row.target || "");
            const length = Math.hypot(
              (Number(sourceNode?.x) || 0) - (Number(targetNode?.x) || 0),
              (Number(sourceNode?.y) || 0) - (Number(targetNode?.y) || 0)
            );
            const crossCommunity = sourceCommunity !== targetCommunity;
            return {
              ...row,
              crossCommunity,
              score: (row.weight * (crossCommunity ? 0.38 : 1.2)) / (1 + length / 1200),
            };
          })
          .sort((left, right) => right.score - left.score || right.weight - left.weight || left.index - right.index);
        const remainingEdgeBudget = Math.max(0, edgeLimit - edgeCount);
        rankedEdges.some((row) => {
          if (materializedEdges >= remainingEdgeBudget) return true;
          const { edge, source, target } = row;
          if (!source || !target || !runtimeNodeIds.has(source) || !runtimeNodeIds.has(target)) return false;
          if (!canMaterializeEdge(row)) return false;
          const renderEdge =
            mode === "xlarge" ? toNetworkSkeletonRenderEdge({ ...edge, source, target }) : { ...edge, source, target };
          const item = graph.addItem?.("edge", renderEdge);
          if (!item) return;
          revealNetworkEdgeForViewport(graph, item, edge);
          rememberMaterializedEdge(row);
          materializedEdges += 1;
          edgeCount += 1;
          return false;
        });
      }
      state.networkLayout.viewportRefineSignature = signature;
      state.networkLayout.viewportRefineLastAt = Date.now();
      const labelCollisionSuppressed = suppressNetworkRuntimeOverlappingLabels(graph, {
        maxLabels: mode === "xlarge" ? 8 : 12,
        padding: mode === "xlarge" ? 14 : 12,
      });
      const viewportQuality = collectNetworkRuntimeQualityReport(graph, {
        mode,
        scope: "viewport",
        bounds,
        reason,
      });
      const refineDurationMs = roundNetworkPerfMs(networkPerfNow() - refineStartedAt);
      state.networkLayout.lastViewportQuality = viewportQuality;
      state.networkLayout.lastViewportRefine = {
        reason: String(reason || ""),
        mode,
        nodes: revealIds.size,
        materializedNodes: materializedIds.size,
        materializedEdges,
        labels: labelCount,
        labelCollisionSuppressed,
        durationMs: refineDurationMs,
        quality: viewportQuality,
        at: state.networkLayout.viewportRefineLastAt,
      };
      recordNetworkRuntimePerfEvent("viewport-refined", {
        reason: String(reason || ""),
        mode,
        durationMs: refineDurationMs,
        nodes: revealIds.size,
        materializedNodes: materializedIds.size,
        materializedEdges,
        labels: labelCount,
        labelCollisionSuppressed,
      });
      const semanticState = ensureLayoutSemanticState();
      semanticState.lastReports = {
        ...(semanticState.lastReports || {}),
        networkViewport: {
          mode,
          quality: viewportQuality,
          report: {
            reason: String(reason || ""),
            materializedNodes: materializedIds.size,
            materializedEdges,
            labelCollisionSuppressed,
          },
        },
      };
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
      if (mode === "xlarge" && force && (materializedIds.size || revealIds.size)) {
        const visibleRuntimeNodes = collectNetworkRuntimeVisibleModels(graph);
        if (visibleRuntimeNodes.length) {
          centerGraphOnLayout(
            graph,
            visibleRuntimeNodes,
            Math.max(0.24, Math.min(0.9, Number(graph.getZoom?.()) || 0.55))
          );
        }
      }
      if (materializedIds.size || materializedEdges) updateMinimap();
    }
    const runtimeEdgeBudget = resolveNetworkRuntimeEdgeBudget(mode);
    let viewportRuntimeLayoutRefined = false;
    if (
      mode === "xlarge" &&
      (materializedIds.size || materializedEdges) &&
      ((graph.getNodes?.().length || 0) > 260 || (graph.getEdges?.().length || 0) > runtimeEdgeBudget)
    ) {
      const capped = capNetworkRuntimeSkeletonGraph(graph, {
        mode,
        reason: `viewport-refine:${reason || "network"}`,
        maxNodes: 260,
        maxEdges: runtimeEdgeBudget,
      });
      if (capped) {
        relaxNetworkRuntimeSkeletonGraph(graph, {
          mode,
          reason: `viewport-refine:${reason || "network"}`,
          iterations: 32,
        });
        viewportRuntimeLayoutRefined = true;
        if (force) {
          const visibleRuntimeNodes = collectNetworkRuntimeVisibleModels(graph);
          if (visibleRuntimeNodes.length) {
              centerGraphOnLayout(
                graph,
                visibleRuntimeNodes,
                Math.max(0.24, Math.min(0.9, Number(graph.getZoom?.()) || 0.55))
              );
          }
        }
      }
    }
    if (
      !viewportRuntimeLayoutRefined &&
      mode === "xlarge" &&
      (materializedIds.size || materializedEdges || revealIds.size >= 24)
    ) {
      viewportRuntimeLayoutRefined = relaxNetworkRuntimeSkeletonGraph(graph, {
        mode,
        reason: `viewport-refine:${reason || "network"}:post-materialize`,
        iterations: 28,
      });
      if (viewportRuntimeLayoutRefined) {
        suppressNetworkRuntimeOverlappingLabels(graph, {
          maxLabels: 8,
          padding: 14,
        });
        if (force) {
          const visibleRuntimeNodes = collectNetworkRuntimeVisibleModels(graph);
          if (visibleRuntimeNodes.length) {
            centerGraphOnLayout(
              graph,
              visibleRuntimeNodes,
              Math.max(0.24, Math.min(0.9, Number(graph.getZoom?.()) || 0.55))
            );
          }
        }
      }
    }
    if (
      mode === "xlarge" &&
      viewportRuntimeLayoutRefined &&
      Number(state.networkLayout?.suppressTopologyPlanUntil || 0) > Date.now()
    ) {
      const stabilized = spreadNetworkRuntimeCollapsedCommunities(
        graph,
        `viewport-refine:${reason || "network"}:runtime-stabilize`
      );
      const centered = centerNetworkRuntimeSkeletonAfterExpand(
        graph,
        `viewport-refine:${reason || "network"}:runtime-stabilize`
      );
      if (stabilized || centered) {
        suppressDenseNetworkRuntimeEdgeLabels(graph, {
          mode,
          nodeCount: graph.getNodes?.().length || 0,
          edgeCount: graph.getEdges?.().length || 0,
        });
        suppressNetworkRuntimeOverlappingLabels(graph, {
          maxLabels: 8,
          padding: 14,
        });
        updateMinimap();
      }
    }
    return revealIds.size > 0 || materializedIds.size > 0 || materializedEdges > 0;
  }

  function scheduleNetworkViewportRefinement(reason = "", options = {}) {
    return false;
    if (state.networkLayout?.viewportRefineTimer) {
      const pending = state.networkLayout.viewportRefineTimerMeta || {};
      if (pending.force === true && options?.force !== true) {
        return false;
      }
      clearTimeout(state.networkLayout.viewportRefineTimer);
      state.networkLayout.viewportRefineTimer = 0;
      state.networkLayout.viewportRefineTimerMeta = null;
    }
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (state.layoutPreset !== "network" || !isLargeNetworkTopologyMode(mode)) return false;
    const delayMs = Math.max(0, Number(options?.delayMs ?? 160) || 0);
    state.networkLayout.viewportRefineTimerMeta = {
      reason: String(reason || ""),
      force: options?.force === true,
    };
    state.networkLayout.viewportRefineTimer = setTimeout(() => {
      state.networkLayout.viewportRefineTimer = 0;
      state.networkLayout.viewportRefineTimerMeta = null;
      applyNetworkViewportRefinement({ reason, force: !!options?.force });
    }, delayMs);
    return true;
  }

  function noteNetworkCommunityQualityDrop(stage = "", detail = {}) {
    if (!state.networkLayout || typeof state.networkLayout !== "object") return;
    state.networkLayout.lastCommunityQualityDrop = {
      stage: String(stage || ""),
      at: Date.now(),
      ...(detail && typeof detail === "object" ? detail : {}),
    };
    if (String(stage || "").includes("stale")) {
      state.networkLayout.lastCommunityQualityStaleDrop = {
        ...state.networkLayout.lastCommunityQualityDrop,
      };
    }
  }

  function applyNetworkCommunityQualityResult(result = {}, context = {}) {
    const rows = Array.isArray(result?.communityQuality)
      ? result.communityQuality.filter((row) => row && typeof row === "object")
      : [];
    if (!rows.length) {
      noteNetworkCommunityQualityDrop("empty-community-quality", {
        reason: String(context?.reason || ""),
        seq: Number(context?.seq || 0),
      });
      return false;
    }
    state.networkLayout.lastPlanCommunityQuality = rows;
    state.networkLayout.lastCommunityQualityDrop = null;
    state.networkLayout.lastCommunityQualityAttempt = {
      ...(state.networkLayout.lastCommunityQualityAttempt || {}),
      appliedAt: Date.now(),
      appliedMode: String(result?.mode || context?.mode || ""),
      communityQualityCount: rows.length,
      report: result?.report && typeof result.report === "object" ? { ...result.report } : {},
    };
    const semanticState = ensureLayoutSemanticState();
    semanticState.lastReports = {
      ...(semanticState.lastReports || {}),
      networkCommunityQuality: {
        mode: String(result?.mode || context?.mode || ""),
        quality: result?.quality && typeof result.quality === "object" ? result.quality : null,
        communityQuality: rows,
        report: result?.report || {},
      },
      networkPlan: {
        ...(semanticState.lastReports?.networkPlan || {}),
        communityQuality: rows,
      },
    };
    return true;
  }

  async function refreshNetworkCommunityQuality({ reason = "" } = {}) {
    if (typeof computeNetworkCommunityQuality !== "function") return false;
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (state.layoutPreset !== "network" || !isLargeNetworkTopologyMode(mode)) return false;
    const nodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
    const edges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    if (!nodes.length) return false;
    const seq = Number(state.networkLayout?.communityQualitySeq || 0) + 1;
    state.networkLayout.communityQualitySeq = seq;
    const graphMutationSeq = Number(state.graphMutationSeq || 0);
    const planSeq = Number(state.networkLayout?.planSeq || 0);
    const graphSignature = buildNetworkPlanGraphSignature(nodes, edges);
    const focusId = resolveFocusIdFromNodes(nodes, state.focusId, state.focusName);
    state.networkLayout.lastCommunityQualityAttempt = {
      reason: String(reason || ""),
      seq,
      mode,
      graphMutationSeq,
      planSeq,
      graphSignature,
      counts: { nodes: nodes.length, edges: edges.length },
      startedAt: Date.now(),
    };
    state.networkLayout.lastCommunityQualityDrop = null;
    const useSnapshotCandidate =
      isLargeNetworkTopologyMode(mode) || nodes.length > 5000 || edges.length > 15000;
    const snapshotRef = useSnapshotCandidate
      ? cloneResultSnapshotRef(state.resultSnapshotRef || state.graph?.data?.result_snapshot_ref || state.graph?.data?.resultSnapshotRef)
      : null;
    const useSnapshotGraph = !!(snapshotRef && (snapshotRef.snapshot_id || snapshotRef.graph_hash));
    let result = null;
    try {
      result = await computeNetworkCommunityQuality({
        nodes: useSnapshotGraph ? [] : projectNetworkPlanNodes(nodes),
        edges: useSnapshotGraph ? [] : projectNetworkPlanEdges(edges),
        resultSnapshotRef: useSnapshotGraph ? snapshotRef : {},
        focusId,
        graphMutationSeq,
        layoutDirection: state.layoutDirection || {},
        seedContext: {
          selectedIds: Array.from(state.selected || []),
          leftSeeds: Array.isArray(state.lastRequest?.leftSeeds) ? state.lastRequest.leftSeeds : [],
          focusKeyType: state.focusKeyType || "",
          tab: state.tab || "",
          source: state.source || "",
          focusUnknownName: !!state.focusUnknownName,
          focusCounterpartyStrict: !!state.focusCounterpartyStrict,
        },
        semantic: projectNetworkPlanSemanticProjection(null),
      });
    } catch (error) {
      noteNetworkCommunityQualityDrop("compute-failed", {
        reason: String(reason || ""),
        seq,
        message: String(error?.message || error),
      });
      return false;
    }
    const currentNodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
    const currentEdges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    const currentGraphSignature = buildNetworkPlanGraphSignature(currentNodes, currentEdges);
    const currentMode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (
      state.layoutPreset !== "network" ||
      currentMode !== mode ||
      currentGraphSignature !== graphSignature ||
      Number(state.networkLayout?.communityQualitySeq || 0) !== seq
    ) {
      noteNetworkCommunityQualityDrop("stale-after-compute", {
        reason: String(reason || ""),
        seq,
        graphMutationSeq,
        currentGraphMutationSeq: Number(state.graphMutationSeq || 0),
        currentPlanSeq: Number(state.networkLayout?.planSeq || 0),
        currentSeq: Number(state.networkLayout?.communityQualitySeq || 0),
        graphSignature,
        currentGraphSignature,
        mode,
        currentMode,
      });
      return false;
    }
    return applyNetworkCommunityQualityResult(result, { reason, seq, mode });
  }

  function scheduleNetworkCommunityQualityRefresh(reason = "", options = {}) {
    if (state.networkLayout?.communityQualityTimer) {
      clearTimeout(state.networkLayout.communityQualityTimer);
      state.networkLayout.communityQualityTimer = 0;
    }
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (state.layoutPreset !== "network" || !isLargeNetworkTopologyMode(mode)) return false;
    if (hasProjectionExpandedNetworkIntent()) {
      suppressNetworkTopologyPlanForProjectionExpand(`community-quality:${reason || "refresh"}`);
    }
    const delayMs = Math.max(0, Number(options?.delayMs ?? 180) || 0);
    state.networkLayout.communityQualitySeq = Number(state.networkLayout.communityQualitySeq || 0) + 1;
    state.networkLayout.communityQualityTimer = setTimeout(() => {
      state.networkLayout.communityQualityTimer = 0;
      void refreshNetworkCommunityQuality({ reason });
    }, delayMs);
    return true;
  }

  function networkPlanEndpoint(value) {
    if (value && typeof value === "object") return String(value.id || value.key || "").trim();
    return String(value || "").trim();
  }

  function projectNetworkPlanNodes(nodes = []) {
    return (Array.isArray(nodes) ? nodes : [])
      .map((node) => {
        const id = String(node?.id || "").trim();
        if (!id) return null;
        const out = { id };
        [
          "x",
          "y",
          "prevX",
          "prevY",
          "r",
          "radius",
          "labelHalfWidth",
          "labelBottom",
          "collisionRadius",
          "weight",
          "amount",
          "total_amount",
          "totalAmount",
          "total_count",
        ].forEach((key) => {
          const value = Number(node?.[key]);
          if (Number.isFinite(value)) out[key] = value;
        });
        [
          "role",
          "layoutBand",
          "clusterId",
          "layoutClusterId",
          "layoutCommunity",
          "layoutVisibilityTier",
          "layoutLabelTier",
          "title",
          "label",
          "name",
        ].forEach((key) => {
          const value = String(node?.[key] || "").trim();
          if (value) out[key] = value;
        });
        if (node?.nodeLabelHidden === true) out.nodeLabelHidden = true;
        return out;
      })
      .filter(Boolean);
  }

  function projectNetworkPlanEdges(edges = []) {
    return (Array.isArray(edges) ? edges : [])
      .map((edge) => {
        const source = networkPlanEndpoint(edge?.source);
        const target = networkPlanEndpoint(edge?.target);
        if (!source || !target || source === target) return null;
        const out = { source, target };
        ["amount", "weight", "value", "count", "total_amount", "totalAmount"].forEach((key) => {
          const value = Number(edge?.[key]);
          if (Number.isFinite(value)) out[key] = value;
        });
        return out;
      })
      .filter(Boolean);
  }

  function projectNetworkPlanSemanticProjection(semanticProjection = null) {
    const source = semanticProjection && typeof semanticProjection === "object" ? semanticProjection : {};
    const semantic = source.semantic && typeof source.semantic === "object" ? source.semantic : {};
    const reports = semantic.reports && typeof semantic.reports === "object" ? semantic.reports : {};
    const clusters = reports.clusters && typeof reports.clusters === "object" ? reports.clusters : {};
    const projection = state.graphProjection && typeof state.graphProjection === "object" ? state.graphProjection : {};
    const networkExpandedCommunityIds =
      state.networkLayout?.expandedCommunities instanceof Set
        ? Array.from(state.networkLayout.expandedCommunities)
        : Array.isArray(state.networkLayout?.expandedCommunities)
          ? state.networkLayout.expandedCommunities
          : [];
    const expandedCommunityIds = parseUniqueList(
      [
        ...networkExpandedCommunityIds,
        ...parseUniqueList(projection.expanded_cluster_ids || projection.expandedClusterIds || [], { allowString: true }),
        ...parseUniqueList(projection.expanded_community_ids || projection.expandedCommunityIds || [], { allowString: true }),
        ...parseUniqueList(projection.cluster_ids || projection.clusterIds || [], { allowString: true }),
      ],
      { allowString: true }
    ).slice(0, 64);
    const projectionIntentNodeIds = parseUniqueList(
      [
        ...parseUniqueList(projection.search_match_node_ids || projection.searchMatchNodeIds || [], { allowString: true }),
        ...parseUniqueList(projection.path_node_ids || projection.pathNodeIds || [], { allowString: true }),
        ...parseUniqueList(projection.expanded_node_ids || projection.expandedNodeIds || [], { allowString: true }),
      ],
      { allowString: true }
    ).slice(0, 256);
    const selectionIntentNodeIds = parseUniqueList([...Array.from(state.selected || []), state.focusId || ""], {
      allowString: true,
    }).slice(0, 256);
    const primaryIntentNodeIds = projectionIntentNodeIds.length ? projectionIntentNodeIds : selectionIntentNodeIds;
    const intentNodeIds = parseUniqueList([...projectionIntentNodeIds, ...selectionIntentNodeIds], {
      allowString: true,
    }).slice(0, 256);
    const projectionSearchQuery = String(projection.search_query || projection.searchQuery || "").trim().slice(0, 120);
    const hasProjectionIntent = !!(
      projectionIntentNodeIds.length ||
      expandedCommunityIds.length ||
      projectionSearchQuery
    );
    return {
      preferredCoreIds: Array.isArray(source.preferredCoreIds) ? source.preferredCoreIds.slice(0, 256) : [],
      corePlacement: String(source.corePlacement || ""),
      corePin: String(source.corePin || ""),
      coreSource: String(source.coreSource || ""),
      semantic: {
        mode: String(semantic.mode || ""),
        clusterCount: Number(clusters.totalClusters) || 0,
        communityQualityCommunityIds: expandedCommunityIds,
        communityQualityPrimaryNodeIds: primaryIntentNodeIds,
        communityQualityNodeIds: intentNodeIds,
        communityQualityIntent: {
          hasProjection: hasProjectionIntent,
          expandedCommunityCount: expandedCommunityIds.length,
          primaryNodeIntentCount: primaryIntentNodeIds.length,
          nodeIntentCount: intentNodeIds.length,
          searchQuery: projectionSearchQuery,
        },
      },
    };
  }

  function hasProjectionExpandedNetworkIntent(semanticProjection = null) {
    const semanticRoot =
      semanticProjection && typeof semanticProjection === "object" ? semanticProjection.semantic || semanticProjection : {};
    const semantic = semanticRoot && typeof semanticRoot === "object" ? semanticRoot : {};
    const intent =
      semantic.communityQualityIntent && typeof semantic.communityQualityIntent === "object"
        ? semantic.communityQualityIntent
        : {};
    if (
      intent.hasProjection === true &&
      (Number(intent.nodeIntentCount || 0) > 0 ||
        Number(intent.expandedCommunityCount || 0) > 0 ||
        String(intent.searchQuery || "").trim())
    ) {
      return true;
    }
    const projection = state.graphProjection && typeof state.graphProjection === "object" ? state.graphProjection : {};
    if (!projection || !Object.keys(projection).length) return false;
    const expandedNodeIds = parseUniqueList(
      [
        ...parseUniqueList(projection.expanded_node_ids || projection.expandedNodeIds || [], { allowString: true }),
        ...parseUniqueList(projection.path_node_ids || projection.pathNodeIds || [], { allowString: true }),
        ...parseUniqueList(projection.search_match_node_ids || projection.searchMatchNodeIds || [], { allowString: true }),
      ],
      { allowString: true }
    );
    const expandedCommunityIds = parseUniqueList(
      [
        ...parseUniqueList(projection.expanded_cluster_ids || projection.expandedClusterIds || [], { allowString: true }),
        ...parseUniqueList(projection.expanded_community_ids || projection.expandedCommunityIds || [], { allowString: true }),
        ...parseUniqueList(projection.cluster_ids || projection.clusterIds || [], { allowString: true }),
      ],
      { allowString: true }
    );
    const searchQuery = String(projection.search_query || projection.searchQuery || "").trim();
    return !!(expandedNodeIds.length || expandedCommunityIds.length || searchQuery);
  }

  function shouldDropTopologyPlanForProjectionExpandedNetwork(reason = "", topologyMode = "", semanticProjection = null) {
    if (!isLargeNetworkTopologyMode(topologyMode)) return false;
    if (!hasProjectionExpandedNetworkIntent(semanticProjection)) return false;
    const reasonText = String(reason || "");
    noteNetworkPlanDrop("projection-expanded-state", {
      reason: reasonText,
      mode: String(topologyMode || ""),
      graphMutationSeq: Number(state.graphMutationSeq || 0),
      planSeq: Number(state.networkLayout?.planSeq || 0),
    });
    recordNetworkRuntimePerfEvent("topology-plan-suppressed", {
      reason: reasonText,
      suppressReason: "projection-expanded-state",
      nodes: Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes.length : 0,
      edges: Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges.length : 0,
    });
    return true;
  }

  function buildNetworkLightweightSemanticProjection(nodes = [], { reason = "", mode = "" } = {}) {
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const semanticStartedAt = networkPerfNow();
    const candidateRows = [];
    const communityIds = new Set();
    nodeRows.forEach((node, index) => {
      const id = String(node?.id || "").trim();
      const role = String(node?.role || node?.layoutBand || "").trim().toLowerCase();
      const visibilityTier = String(node?.layoutVisibilityTier || "").trim().toLowerCase();
      const communityId =
        String(node?.layoutClusterId || node?.clusterId || node?.layoutCommunity || node?.clusterHint || "").trim() ||
        `coarse:${Math.floor(index / 80)}`;
      communityIds.add(communityId);
      if (!id) return;
      if (role !== "core" && role !== "bridge" && visibilityTier !== "primary") return;
      pushNetworkCoarseBoundedRow(
        candidateRows,
        {
          id,
          index,
          score:
            (role === "core" ? 1_000_000 : role === "bridge" ? 500_000 : 100_000) +
            rankNetworkCoarseNode(node),
        },
        256,
        compareNetworkCoarseNodeRows
      );
    });
    const preferredCoreIds = parseUniqueList(
      [
        ...collectLayoutSeedCoreCandidates(nodeRows),
        ...candidateRows.map((row) => row.id),
      ],
      { allowString: true }
    ).slice(0, 256);
    const durationMs = networkPerfNow() - semanticStartedAt;
    recordNetworkRuntimePerfEvent("semantic-projection-lightweight", {
      reason: String(reason || ""),
      mode: String(mode || ""),
      durationMs,
      nodes: nodeRows.length,
      preferredCoreCount: preferredCoreIds.length,
      clusterCount: communityIds.size,
      source: "network-lightweight",
    });
    return {
      preferredCoreIds,
      corePlacement: "soft",
      corePin: "",
      coreSource: "network-lightweight",
      semantic: {
        mode: "network",
        reports: {
          clusters: {
            totalClusters: communityIds.size,
          },
        },
      },
    };
  }

  function incrementLayoutPlanCacheStat(key) {
    const name = String(key || "").trim();
    if (!name) return 0;
    const semanticState = ensureLayoutSemanticState();
    const stats = semanticState.layoutPlanCacheStats || {};
    semanticState.layoutPlanCacheStats = stats;
    stats[name] = Math.max(0, Number(stats[name] || 0)) + 1;
    return stats[name];
  }

  function buildNetworkTopologyPlanSeedContext() {
    return {
      selectedIds: Array.from(state.selected || []),
      leftSeeds: Array.isArray(state.lastRequest?.leftSeeds) ? state.lastRequest.leftSeeds : [],
      focusKeyType: state.focusKeyType || "",
      tab: state.tab || "",
      source: state.source || "",
      focusUnknownName: !!state.focusUnknownName,
      focusCounterpartyStrict: !!state.focusCounterpartyStrict,
    };
  }

  function buildNetworkTopologyPlanCacheKey({
    focusId = "",
    graphMutationSeq = 0,
    graphSignature = "",
    semanticProjection = null,
  } = {}) {
    const snapshotRef = cloneResultSnapshotRef(
      state.resultSnapshotRef || state.graph?.data?.result_snapshot_ref || state.graph?.data?.resultSnapshotRef
    );
    const identity = {
      version: "network-topology-plan-v1",
      focusId: String(focusId || "").trim(),
      graphMutationSeq: Number(graphMutationSeq || 0),
      graphSignature: String(graphSignature || ""),
      snapshotId: String(snapshotRef?.snapshot_id || snapshotRef?.snapshotId || ""),
      graphHash: String(snapshotRef?.graph_hash || snapshotRef?.graphHash || ""),
      layoutDirection:
        state.layoutDirection && typeof state.layoutDirection === "object" ? state.layoutDirection : {},
      seedContext: buildNetworkTopologyPlanSeedContext(),
      semantic: projectNetworkPlanSemanticProjection(semanticProjection),
    };
    try {
      return `network-topology-plan:${JSON.stringify(identity)}`;
    } catch (_error) {
      return "";
    }
  }

  function cloneNetworkTopologyPlan(plan) {
    if (!plan || typeof plan !== "object") return null;
    try {
      return JSON.parse(JSON.stringify(plan));
    } catch (_error) {
      return null;
    }
  }

  function readNetworkTopologyPlanCache(cacheKey, graphSignature) {
    const key = String(cacheKey || "").trim();
    if (!key) return null;
    const networkLayout = state.networkLayout || {};
    const cache =
      networkLayout.topologyPlanCache && typeof networkLayout.topologyPlanCache === "object"
        ? networkLayout.topologyPlanCache
        : {};
    const entry = cache[key];
    const plan = entry && typeof entry === "object" ? entry.plan : null;
    if (!plan || typeof plan !== "object") {
      incrementLayoutPlanCacheStat("networkPlanCacheMiss");
      return null;
    }
    if (String(entry.graphSignature || "") !== String(graphSignature || "")) {
      incrementLayoutPlanCacheStat("networkPlanCacheMiss");
      return null;
    }
    const cloned = cloneNetworkTopologyPlan(plan);
    if (!cloned) {
      incrementLayoutPlanCacheStat("networkPlanCacheMiss");
      return null;
    }
    entry.updatedAt = Date.now();
    incrementLayoutPlanCacheStat("networkPlanCacheHit");
    incrementLayoutPlanCacheStat("planCacheHit");
    return cloned;
  }

  function readLatestNetworkTopologyPlanCacheByGraphSignature(graphSignature = "") {
    const signature = String(graphSignature || "").trim();
    if (!signature) return null;
    const networkLayout = state.networkLayout || {};
    const cache =
      networkLayout.topologyPlanCache && typeof networkLayout.topologyPlanCache === "object"
        ? networkLayout.topologyPlanCache
        : {};
    const rows = Object.values(cache)
      .filter((entry) => {
        const row = entry && typeof entry === "object" ? entry : null;
        return row?.plan && typeof row.plan === "object" && String(row.graphSignature || "") === signature;
      })
      .sort((left, right) => (Number(right.updatedAt) || 0) - (Number(left.updatedAt) || 0));
    const entry = rows[0] || null;
    const plan = entry?.plan && typeof entry.plan === "object" ? cloneNetworkTopologyPlan(entry.plan) : null;
    if (!plan) return null;
    entry.updatedAt = Date.now();
    incrementLayoutPlanCacheStat("networkPlanCacheHit");
    incrementLayoutPlanCacheStat("planCacheHit");
    return {
      key: String(entry.key || ""),
      plan,
    };
  }

  function rememberNetworkTopologyPlanCache(cacheKey, plan, graphSignature) {
    const key = String(cacheKey || "").trim();
    const planMode = String(plan?.mode || "").trim().toLowerCase();
    if (!key || planMode !== "xlarge") return false;
    const cloned = cloneNetworkTopologyPlan(plan);
    if (!cloned) return false;
    const networkLayout = state.networkLayout || {};
    if (!networkLayout.topologyPlanCache || typeof networkLayout.topologyPlanCache !== "object") {
      networkLayout.topologyPlanCache = {};
    }
    const cache = networkLayout.topologyPlanCache;
    cache[key] = {
      key,
      graphSignature: String(graphSignature || ""),
      plan: cloned,
      updatedAt: Date.now(),
    };
    const maxEntries = Math.max(1, Number(networkLayout.topologyPlanCacheMaxEntries) || 4);
    const keys = Object.keys(cache);
    if (keys.length > maxEntries) {
      keys
        .map((rowKey) => ({ key: rowKey, t: Number(cache[rowKey]?.updatedAt) || 0 }))
        .sort((a, b) => a.t - b.t)
        .slice(0, Math.max(1, keys.length - maxEntries))
        .forEach((row) => delete cache[row.key]);
    }
    incrementLayoutPlanCacheStat("networkPlanCacheRemembered");
    return true;
  }

  function shouldSuppressNetworkTopologyPlan(reason = "") {
    const suppressUntil = Number(state.networkLayout?.suppressTopologyPlanUntil || 0);
    if (suppressUntil && Date.now() < suppressUntil) {
      noteNetworkPlanDrop("projection-expand-suppressed", {
        reason: String(reason || ""),
        suppressReason: String(state.networkLayout?.suppressTopologyPlanReason || ""),
        suppressUntil,
      });
      recordNetworkRuntimePerfEvent("topology-plan-suppressed", {
        reason: String(reason || ""),
        suppressReason: String(state.networkLayout?.suppressTopologyPlanReason || ""),
        suppressUntil,
      });
      return true;
    }
    return false;
  }

  function suppressNetworkTopologyPlanForProjectionExpand(reason = "", durationMs = 10000) {
    if (!state.networkLayout || typeof state.networkLayout !== "object") return false;
    const suppressUntil = Date.now() + Math.max(500, Number(durationMs) || 10000);
    state.networkLayout.suppressTopologyPlanUntil = suppressUntil;
    state.networkLayout.suppressTopologyPlanReason = reason || "projection-expand";
    state.networkLayout.planSeq = Number(state.networkLayout.planSeq || 0) + 1;
    noteNetworkPlanDrop("projection-expand-cancelled-pending", {
      reason: String(reason || ""),
      suppressUntil,
      planSeq: Number(state.networkLayout.planSeq || 0),
    });
    recordNetworkRuntimePerfEvent("topology-plan-pending-cancelled", {
      reason: String(reason || ""),
      suppressUntil,
      planSeq: Number(state.networkLayout.planSeq || 0),
    });
    setTimeout(() => {
      if (Date.now() >= Number(state.networkLayout?.suppressTopologyPlanUntil || 0)) {
        state.networkLayout.suppressTopologyPlanUntil = 0;
        state.networkLayout.suppressTopologyPlanReason = "";
      }
    }, Math.max(500, Number(durationMs) || 10000) + 200);
    return true;
  }

  async function scheduleNetworkTopologyPlan({ reason = "", semanticProjection = null, commandSeq = 0 } = {}) {
    if (typeof computeNetworkLayoutPlan !== "function") return false;
    const nodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
    const edges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    if (state.layoutPreset !== "network" || !nodes.length) return false;
    if (shouldSuppressNetworkTopologyPlan(reason)) return false;
    const seq = Number(state.networkLayout?.planSeq || 0) + 1;
    state.networkLayout.planSeq = seq;
    const focusId = resolveFocusIdFromNodes(nodes, state.focusId, state.focusName);
    const graphMutationSeq = Number(state.graphMutationSeq || 0);
    const graphSignature = buildNetworkPlanGraphSignature(nodes, edges);
    const topologyMode = resolveNetworkTopologyModeForGraph(nodes, edges);
    if (shouldDropTopologyPlanForProjectionExpandedNetwork(reason, topologyMode, semanticProjection)) return false;
    const useSnapshotGraph = false;
    state.networkLayout.lastPlanAttempt = {
      reason: String(reason || ""),
      seq,
      mode: topologyMode,
      graphMutationSeq,
      graphSignature,
      counts: { nodes: nodes.length, edges: edges.length },
      commandSeq: Number(commandSeq || 0),
      startedAt: Date.now(),
    };
    recordNetworkRuntimePerfEvent("topology-plan-requested", {
      reason: String(reason || ""),
      seq,
      commandSeq: Number(commandSeq || 0),
      nodes: nodes.length,
      edges: edges.length,
      graphSignature,
    });
    state.networkLayout.lastPlanDrop = null;
    let cacheKey = "";
    const skipCacheKey = false;
    const topologyPlanCacheKey = skipCacheKey
      ? buildNetworkTopologyPlanCacheKey({
          focusId,
          graphMutationSeq,
          graphSignature,
          semanticProjection,
        })
      : "";
    if (skipCacheKey) {
      const cachedPlan = readNetworkTopologyPlanCache(topologyPlanCacheKey, graphSignature);
      if (cachedPlan) {
        return applyNetworkTopologyPlan(cachedPlan, {
          reason: reason || "network-topology-plan-cache-hit",
          cacheKey: topologyPlanCacheKey,
        });
      }
    }
    if (!skipCacheKey) {
      try {
        cacheKey =
          layoutPlanCacheController.makeCacheKey("network", focusId, nodes, edges) ||
          (await layoutPlanCacheController.prepareCacheKey("network", focusId, nodes, edges));
      } catch (error) {
        cacheKey = "";
      }
    }
    if (
      state.layoutPreset !== "network" ||
      (commandSeq && Number(state.layoutPresetCommandSeq || 0) !== Number(commandSeq)) ||
      Number(state.graphMutationSeq || 0) !== graphMutationSeq ||
      Number(state.networkLayout?.planSeq || 0) !== seq
    ) {
      noteNetworkPlanDrop("stale-before-compute", {
        reason: String(reason || ""),
        seq,
        graphMutationSeq,
        currentGraphMutationSeq: Number(state.graphMutationSeq || 0),
        currentPlanSeq: Number(state.networkLayout?.planSeq || 0),
      });
      return false;
    }
    if (skipCacheKey && topologyMode === "xlarge") {
      await yieldToMainThread();
    }
    if (
      state.layoutPreset !== "network" ||
      (commandSeq && Number(state.layoutPresetCommandSeq || 0) !== Number(commandSeq)) ||
      Number(state.graphMutationSeq || 0) !== graphMutationSeq ||
      Number(state.networkLayout?.planSeq || 0) !== seq
    ) {
      noteNetworkPlanDrop("stale-before-deferred-compute", {
        reason: String(reason || ""),
        seq,
        graphMutationSeq,
        currentGraphMutationSeq: Number(state.graphMutationSeq || 0),
        currentPlanSeq: Number(state.networkLayout?.planSeq || 0),
      });
      return false;
    }
    if (shouldSuppressNetworkTopologyPlan(reason)) return false;
    if (shouldDropTopologyPlanForProjectionExpandedNetwork(reason, topologyMode, semanticProjection)) return false;
    let plan = null;
    const computeStartedAt = networkPerfNow();
    try {
      plan = await computeNetworkLayoutPlan({
        nodes: projectNetworkPlanNodes(nodes),
        edges: projectNetworkPlanEdges(edges),
        resultSnapshotRef: {},
        focusId,
        graphMutationSeq,
        layoutDirection: state.layoutDirection || {},
        seedContext: {
          selectedIds: Array.from(state.selected || []),
          leftSeeds: Array.isArray(state.lastRequest?.leftSeeds) ? state.lastRequest.leftSeeds : [],
          focusKeyType: state.focusKeyType || "",
          tab: state.tab || "",
          source: state.source || "",
          focusUnknownName: !!state.focusUnknownName,
          focusCounterpartyStrict: !!state.focusCounterpartyStrict,
        },
        semantic: projectNetworkPlanSemanticProjection(semanticProjection),
      });
      recordNetworkRuntimePerfEvent("topology-plan-response", {
        reason: String(reason || ""),
        seq,
        mode: String(plan?.mode || ""),
        durationMs: networkPerfNow() - computeStartedAt,
        synchronous: false,
        sourceSnapshotRef: useSnapshotGraph,
      });
    } catch (error) {
      try {
        log("WARN", "network layout plan failed", {
          reason: String(reason || ""),
          message: String(error?.message || error),
        });
      } catch (_error) {}
      noteNetworkPlanDrop("compute-failed", {
        reason: String(reason || ""),
        seq,
        message: String(error?.message || error),
      });
      return false;
    }
    if (!plan || typeof plan !== "object") {
      noteNetworkPlanDrop("empty-result", {
        reason: String(reason || ""),
        seq,
        graphSignature,
      });
      return false;
    }
    if (
      state.layoutPreset !== "network" ||
      (commandSeq && Number(state.layoutPresetCommandSeq || 0) !== Number(commandSeq)) ||
      Number(state.graphMutationSeq || 0) !== graphMutationSeq ||
      Number(state.networkLayout?.planSeq || 0) !== seq
    ) {
      noteNetworkPlanDrop("stale-after-compute", {
        reason: String(reason || ""),
        seq,
        graphMutationSeq,
        currentGraphMutationSeq: Number(state.graphMutationSeq || 0),
        currentPlanSeq: Number(state.networkLayout?.planSeq || 0),
        mode: String(plan?.mode || ""),
      });
      return false;
    }
    return applyNetworkTopologyPlan(plan, { reason, cacheKey: cacheKey || topologyPlanCacheKey });
  }

  function applyNetworkTopologyPlan(plan, { reason = "", cacheKey = "" } = {}) {
    const applyStartedAt = networkPerfNow();
    const networkPlan = plan && typeof plan === "object" ? plan : null;
    const nodeUpdates = Array.isArray(networkPlan?.nodeUpdates) ? networkPlan.nodeUpdates : [];
    const nodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
    const edges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    if (state.layoutPreset !== "network" || !nodeUpdates.length || !nodes.length) return false;
    const graph = ensureGraph();
    if (!graph) return false;
    cancelPendingLayoutWorker("network-topology-plan-apply");
    const planMode = String(networkPlan?.mode || "").trim().toLowerCase();
    const beforeNodePositions = captureNodePositions(graph);
    if (!applyNetworkPlanNodeUpdates(nodes, nodeUpdates, { allowPartial: false })) {
      noteNetworkPlanDrop("apply-node-updates-failed", {
        reason: String(reason || ""),
        mode: planMode,
        nodeUpdates: nodeUpdates.length,
        nodes: nodes.length,
      });
      return false;
    }
    state.networkLayout.lastPlanMode = planMode;
    state.networkLayout.lastPlanGraphSignature = buildNetworkPlanGraphSignature(nodes, edges);
    if (networkPlan?.quality && typeof networkPlan.quality === "object") {
      state.networkLayout.lastPlanQuality = networkPlan.quality;
      state.networkLayout.lastPlanSampleQuality =
        networkPlan?.sampleQuality && typeof networkPlan.sampleQuality === "object" ? networkPlan.sampleQuality : null;
      const planCommunityQuality = normalizeNetworkPlanCommunityQuality(networkPlan);
      state.networkLayout.lastPlanCommunityQuality = planCommunityQuality;
      const semanticState = ensureLayoutSemanticState();
      semanticState.lastReports = {
        ...(semanticState.lastReports || {}),
        networkPlan: {
          version: String(networkPlan.version || ""),
          mode: String(networkPlan.mode || ""),
          quality: networkPlan.quality,
          sampleQuality:
            networkPlan?.sampleQuality && typeof networkPlan.sampleQuality === "object" ? networkPlan.sampleQuality : null,
          communityQuality: planCommunityQuality,
          supergraph:
            networkPlan?.supergraph && typeof networkPlan.supergraph === "object" ? networkPlan.supergraph : null,
          report: networkPlan.report || {},
        },
      };
    }
    state.networkLayout.hiddenNodeCount = 0;
    state.networkLayout.hiddenEdgeCount = 0;
    state.networkLayout.activeMode = "full";
    layoutGraphApplyController.applyPreparedLayout({
      graph,
      nodes,
      deferApply: {
        animate: true,
        duration:
          String(networkPlan?.mode || "") === "small"
            ? 220
            : String(networkPlan?.mode || "") === "medium"
              ? 320
              : 360,
        easing: getGraphLayoutConfig().ANIM_EASING,
        fit: true,
        deferFinalize: false,
        startPositions: beforeNodePositions,
      },
      syncReason: "network-topology-plan-apply",
      patchMeta: {
        scope: "layout-switch",
        reason: reason || "network-topology-plan",
        viewId: state.activeViewId || "",
        layoutPreset: "network",
        traceId: createLayoutTraceId("network-plan"),
      },
      allowAnimation: true,
    });
    if (cacheKey) {
      layoutPlanCacheController.rememberCache(cacheKey, "network", nodes);
      state.lastAppliedLayoutCacheKey = String(cacheKey || "");
    }
    if (state.networkLayout?.lastPlanAttempt) {
      state.networkLayout.lastPlanAttempt = {
        ...state.networkLayout.lastPlanAttempt,
        appliedAt: Date.now(),
        appliedMode: planMode,
      };
    }
    try {
      log("INFO", "network layout plan applied", {
        reason: String(reason || ""),
        mode: String(networkPlan?.mode || ""),
        quality: networkPlan?.quality || null,
      });
    } catch (_error) {}
    recordNetworkRuntimePerfEvent("topology-plan-applied", {
      reason: String(reason || ""),
      mode: planMode,
      durationMs: networkPerfNow() - applyStartedAt,
      nodeUpdates: nodeUpdates.length,
      edgeUpdates: Array.isArray(networkPlan?.edgeUpdates) ? networkPlan.edgeUpdates.length : 0,
      cacheHit: String(reason || "").includes("cache-hit"),
    });
    return true;
  }

  function normalizeNetworkPlanCommunityQuality(networkPlan = {}) {
    if (Array.isArray(networkPlan?.communityQuality) && networkPlan.communityQuality.length) {
      return networkPlan.communityQuality.filter((row) => row && typeof row === "object");
    }
    const mode = String(networkPlan?.mode || "").trim().toLowerCase();
    if (!isLargeNetworkTopologyMode(mode) || !Array.isArray(networkPlan?.communities)) return [];
    const durationMs = Number(networkPlan?.quality?.durationMs);
    return networkPlan.communities
      .filter((community) => community && typeof community === "object")
      .slice(0, 12)
      .map((community) => {
        const nodeCount = Math.max(0, Number(community.nodeCount || 0) || 0);
        return {
          durationMs: Number.isFinite(durationMs) ? durationMs : 0,
          nodeOverlapCount: 0,
          labelOverlapEstimate: 0,
          edgeCrossingSample: 0,
          edgeLengthStdDev: 0,
          clusterBBoxOverlapCount: 0,
          mode,
          communityCount: 1,
          qualityScope: "community",
          qualityNodeCount: nodeCount,
          qualityLabelCount: 0,
          qualityEdgeCount: 0,
          sampled: true,
          source: "network-plan-community-fallback",
          communityId: String(community.id || ""),
          communityNodeTotal: nodeCount,
          communityEdgeTotal: 0,
          visibleNodeCount: nodeCount,
          visibleTierStats: {},
        };
      });
  }

  function scheduleLayoutPrewarm(mode, { reason = "", nodes: inputNodes = null, edges: inputEdges = null } = {}) {
    const nodes = Array.isArray(inputNodes) ? inputNodes : Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
    const edges = Array.isArray(inputEdges) ? inputEdges : Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    if (nodes.length > 5000 || edges.length > 15000) return false;
    const task = layoutPrewarmController.scheduleLayoutPrewarm(mode, {
      reason,
      nodes: inputNodes,
      edges: inputEdges,
    });
    if (task && typeof task.catch === "function") {
      task.catch((error) => {
        try {
          log("WARN", "layout prewarm failed", { mode, reason, message: String(error?.message || error) });
        } catch (e) {}
      });
    }
    return task;
  }

  function scheduleNetworkLayoutPrewarm({ reason = "", nodes: inputNodes = null, edges: inputEdges = null } = {}) {
    return scheduleLayoutPrewarm("network", { reason, nodes: inputNodes, edges: inputEdges });
  }

  function scheduleCompactLayoutPrewarm({ reason = "", nodes: inputNodes = null, edges: inputEdges = null } = {}) {
    return scheduleLayoutPrewarm("compact", { reason, nodes: inputNodes, edges: inputEdges });
  }

  function scheduleLayoutWorker(nodes, edges, preset, focusId, layoutHints = {}, layoutCacheKey = "") {
    return layoutWorkerSchedulerController.scheduleLayoutWorker(
      nodes,
      edges,
      preset,
      focusId,
      layoutHints,
      layoutCacheKey
    );
  }

  function rebindLayoutWorkerPendingGraphData(nextData) {
    const pending = state.layoutWorkerPending;
    if (!pending) return false;
    if (!nextData || !Array.isArray(nextData.nodes) || !Array.isArray(nextData.edges)) return false;
    if (pending.token && pending.token !== (state.requestId || "")) return false;
    const nextNodes = nextData.nodes;
    const nextEdges = nextData.edges;
    const prevNodes = Array.isArray(pending.nodes) ? pending.nodes : [];
    const prevEdges = Array.isArray(pending.edges) ? pending.edges : [];
    if (prevNodes.length !== nextNodes.length || prevEdges.length !== nextEdges.length) return false;
    const sameIdentity = (left, right, keyOf) => {
      if (left.length !== right.length) return false;
      const counter = new Map();
      for (const item of left) {
        const key = keyOf(item);
        if (!key) return false;
        counter.set(key, (counter.get(key) || 0) + 1);
      }
      for (const item of right) {
        const key = keyOf(item);
        if (!key) return false;
        const cur = counter.get(key) || 0;
        if (cur <= 0) return false;
        if (cur === 1) counter.delete(key);
        else counter.set(key, cur - 1);
      }
      return counter.size === 0;
    };
    const nodeKey = (node) => {
      const id = String(node?.id || "").trim();
      return id ? `n:${id}` : "";
    };
    const edgeKey = (edge) => {
      const id = String(edge?.id || "").trim();
      if (id) return `e:${id}`;
      const source = String(edge?.source || "");
      const target = String(edge?.target || "");
      if (!source || !target) return "";
      const mode = String(edge?.mode || "");
      const arrow = String(edge?.edgeArrow || edge?.arrow || "");
      const showArrow = edge?.showArrow ? "1" : "0";
      return `e:${source}->${target}|m:${mode}|a:${arrow}|s:${showArrow}`;
    };
    if (!sameIdentity(prevNodes, nextNodes, nodeKey)) return false;
    if (!sameIdentity(prevEdges, nextEdges, edgeKey)) return false;
    pending.nodes = nextNodes;
    pending.edges = nextEdges;
    return true;
  }

  function resolveFocusIdFromNodes(nodes, focusId, focusName) {
    if (!nodes || !nodes.length) return "";
    const direct = nodes.find((n) => n.id === focusId);
    if (direct) return direct.id;
    const norm = normalizeKey(focusId);
    if (norm) {
      const match = nodes.find((n) => normalizeKey(n.id) === norm);
      if (match) return match.id;
    }
    if (focusName) {
      const match = nodes.find((n) => String(n.title || "").includes(focusName));
      if (match) return match.id;
    }
    return "";
  }

  // Layout algorithms are sourced from graph_engine.js core + mode modules.
  let layoutCoreMissingLogged = false;

  function getLayoutEngine() {
    return flowLayoutRuntimeAdapter.getLayoutEngine();
  }

  function logLayoutCoreMissing(method) {
    if (layoutCoreMissingLogged) return;
    layoutCoreMissingLogged = true;
    try {
      log("ERROR", "layout core engine missing", {
        method,
        expected: "window.AnalytixLayoutEngine",
      });
    } catch (e) {}
  }

  function callLayoutCore(method, args) {
    return flowLayoutRuntimeAdapter.callLayoutCore(method, args, {
      onMissing: logLayoutCoreMissing,
    });
  }

  function layoutScale(...args) {
    const value = Number(callLayoutCore("layoutScale", args));
    return Number.isFinite(value) && value > 0 ? value : 1;
  }

  function normalizeLayoutPreset(...args) {
    return callLayoutCore("normalizeLayoutPreset", args);
  }

  function normalizeLayoutDirection(...args) {
    return callLayoutCore("normalizeLayoutDirection", args);
  }

  function normalizeCorePlacement(...args) {
    return callLayoutCore("normalizeCorePlacement", args);
  }

  function normalizeCorePin(...args) {
    return callLayoutCore("normalizeCorePin", args);
  }

  function normalizeCoreSource(...args) {
    return callLayoutCore("normalizeCoreSource", args);
  }

  function clearLayoutMeta(...args) {
    return callLayoutCore("clearLayoutMeta", args);
  }

  function recenterLayout(...args) {
    return callLayoutCore("recenterLayout", args);
  }

  function recenterLayoutByBounds(...args) {
    return callLayoutCore("recenterLayoutByBounds", args);
  }

  function analyzeDirectionalStructure(...args) {
    return callLayoutCore("analyzeDirectionalStructure", args);
  }

  function applyLayoutEdgeRouting(nodes, edges, mode) {
    if (!Array.isArray(edges) || !edges.length) return;
    const clearDirectionalEdgeMeta = () => {
      edges.forEach((edge) => {
        edge.layoutBackflow = false;
        edge.__layoutBackflow = false;
        delete edge.edgeRoute;
        delete edge.orthAxis;
        delete edge.orthBias;
        delete edge.orthSharedCoord;
        delete edge.edgeLabelAnchorMode;
        delete edge.edgeLabelOffsetY;
        delete edge.edgeLabelSideOffset;
        delete edge.edgeLabelMiddleRatio;
        delete edge.edgeLabelSideCapRatio;
        delete edge.edgeLabelPreferDegree;
      });
    };
    const clearNetworkEdgeMeta = () => {
      edges.forEach((edge) => {
        delete edge.layoutBridge;
        delete edge.layoutCommunity;
        delete edge.layoutCorridor;
        delete edge.layoutCorridorLevel;
        delete edge.layoutEdgeTier;
        delete edge.layoutCommunityPair;
        delete edge.layoutEdgeBundleId;
        delete edge.layoutEdgeBundleSize;
        delete edge.layoutEdgeBundleWeight;
        delete edge.layoutEdgeBundleRank;
        delete edge.layoutEdgeBundled;
        delete edge.layoutEdgeBundleRepresentative;
        if (edge.layoutEdgeBundleRouted) {
          delete edge.edgeOffset;
        }
        delete edge.layoutEdgeBundleRouted;
        delete edge.layoutHiddenBySkeleton;
      });
    };

    const directional = mode === "hierarchy" || mode === "flow";
    clearDirectionalEdgeMeta();
    if (mode === "network") {
      clearNetworkEdgeMeta();
      return;
    }
    clearNetworkEdgeMeta();
    if (!directional) {
      return;
    }
    const analysis = analyzeDirectionalStructure(nodes || [], edges || []);
    const orthEnabled = !!state.layoutEdgeRouting?.[mode];
    if (mode === "hierarchy") {
      callLayoutCore("applyHierarchyEdgeRouting", [nodes || [], edges || [], analysis, { orthEnabled }]);
      return;
    }
    if (mode === "flow") {
      callLayoutCore("applyFlowEdgeRouting", [nodes || [], edges || [], analysis, { orthEnabled }]);
      return;
    }
  }

  function normalizeNetworkViewMode(value) {
    const v = String(value || "").trim().toLowerCase();
    if (v === "full") return "full";
    if (v === "skeleton") return "skeleton";
    return "auto";
  }

  function resolveNetworkLayoutConfig() {
    const cfg = state.networkLayout || {};
    const viewMode = normalizeNetworkViewMode(cfg.viewMode);
    return {
      viewMode,
      activeMode: normalizeNetworkViewMode(cfg.activeMode || "full"),
      autoNodeThreshold: Math.max(24, Number(cfg.autoNodeThreshold) || 180),
      autoEdgeThreshold: Math.max(24, Number(cfg.autoEdgeThreshold) || 260),
      crossingThreshold: Math.max(20, Number(cfg.crossingThreshold) || 320),
      longEdgeRatioThreshold: clampNumber(Number(cfg.longEdgeRatioThreshold), 0.02, 0.9),
      leafTopKPerCommunity: Math.max(1, Math.min(32, Number(cfg.leafTopKPerCommunity) || 6)),
      labelZoomBridge: clampNumber(Number(cfg.labelZoomBridge), 0.3, 8),
      labelZoomCluster: clampNumber(Number(cfg.labelZoomCluster), 0.3, 8),
      labelZoomLeaf: clampNumber(Number(cfg.labelZoomLeaf), 0.3, 8),
      expandedCommunities:
        cfg.expandedCommunities instanceof Set
          ? cfg.expandedCommunities
          : new Set(Array.isArray(cfg.expandedCommunities) ? cfg.expandedCommunities : []),
      lastCompactCoreIds: Array.isArray(cfg.lastCompactCoreIds) ? cfg.lastCompactCoreIds.slice() : [],
    };
  }

  function getGraphLayoutConfig() {
    const fromWindow =
      window && window.__ANALYTIX_GRAPH_LAYOUT_CONFIG && typeof window.__ANALYTIX_GRAPH_LAYOUT_CONFIG === "object"
        ? window.__ANALYTIX_GRAPH_LAYOUT_CONFIG
        : null;
    return { ...GRAPH_LAYOUT_CONFIG, ...(fromWindow || {}) };
  }

  function ensureLayoutSemanticState() {
    if (!state.layoutSemanticState || typeof state.layoutSemanticState !== "object") {
      state.layoutSemanticState = {
        clusterAnchorBySignature: {},
        clusterCenterBySignature: {},
        clusterSlotBySignature: {},
        layoutPlanCache: {},
        layoutCacheKeyProjection: null,
        layoutPlanCacheStats: {},
        seedCoreHistory: {},
        roleByNodeId: {},
        clusterByNodeId: {},
        layoutCaseByClusterId: {},
        lastReports: null,
      };
    }
    if (!state.layoutSemanticState.clusterSlotBySignature || typeof state.layoutSemanticState.clusterSlotBySignature !== "object") {
      state.layoutSemanticState.clusterSlotBySignature = {};
    }
    if (!state.layoutSemanticState.layoutPlanCache || typeof state.layoutSemanticState.layoutPlanCache !== "object") {
      state.layoutSemanticState.layoutPlanCache = {};
    }
    if (!state.layoutSemanticState.layoutCacheKeyProjection || typeof state.layoutSemanticState.layoutCacheKeyProjection !== "object") {
      state.layoutSemanticState.layoutCacheKeyProjection = null;
    }
    if (!state.layoutSemanticState.layoutPlanCacheStats || typeof state.layoutSemanticState.layoutPlanCacheStats !== "object") {
      state.layoutSemanticState.layoutPlanCacheStats = {};
    }
    return state.layoutSemanticState;
  }

  function nodeIdPriorityForPair(node, role = "", seed = false) {
    const normalized = String(role || "").toLowerCase();
    if (normalized === "core") return 0;
    if (normalized === "adjacent") return 1;
    if (normalized === "leaf") return 2;
    if (seed) return 3;
    return 4;
  }

  function rolePlainObjectFromMap(mapLike) {
    const out = {};
    if (!(mapLike instanceof Map)) return out;
    mapLike.forEach((value, key) => {
      out[String(key)] = value;
    });
    return out;
  }

  function isCardKeywordText(value) {
    const text = String(value || "").trim().toLowerCase();
    if (!text) return false;
    return /card|card_no|bankcard|交易卡号|银行卡|卡号/.test(text);
  }

  function buildTreeItemIndex(treeData) {
    const out = new Map();
    (Array.isArray(treeData) ? treeData : []).forEach((group) => {
      const groupId = String(group?.id || "").trim();
      const groupTitle = String(group?.title || "").trim();
      const groupMeta = String(group?.meta || "").trim();
      (Array.isArray(group?.items) ? group.items : []).forEach((item) => {
        const id = String(item?.id || "").trim();
        if (!id || out.has(id)) return;
        out.set(id, {
          ...item,
          __groupId: groupId,
          __groupTitle: groupTitle,
          __groupMeta: groupMeta,
        });
      });
    });
    return out;
  }

  function isCardLikeTreeItem(item) {
    if (!item || typeof item !== "object") return false;
    if (isCardKeywordText(item?.keyType || item?.key_type || item?.type || item?.kind)) return true;
    if (isCardKeywordText(item?.title || item?.sub || item?.label)) return true;
    if (isCardKeywordText(item?.__groupTitle || item?.__groupMeta || item?.__groupId)) return true;
    return false;
  }

  function filterNoisySeedIds(values) {
    const source = Array.isArray(values) ? values : [];
    const sorted = sortStableIds(source);
    if (!sorted.length) return [];
    const clean = sorted.filter((id) => !isNoisySeedCandidateId(id));
    return clean.length ? clean : sorted;
  }

  function isStatsBatchFocusRequest(ctx = state) {
    const source = String(ctx?.source || "").trim().toLowerCase();
    if (!isStatsSourceValue(source) || !ctx?.focusOnly) return false;
    const focusIds = parseUniqueList(ctx?.focusIds || [], { allowString: true });
    const focusNames = parseUniqueList(ctx?.focusNames || [], { allowString: true });
    const focusPlaceholderKinds = parseUniqueList(ctx?.focusPlaceholderKinds || [], { allowString: true });
    const expectedRowCount = Number(ctx?.expectedRowCount) || 0;
    return (
      expectedRowCount > 1 ||
      !!ctx?.focusUnknownName ||
      !!ctx?.includeMissingCounterparty ||
      focusPlaceholderKinds.length > 0 ||
      focusIds.length > 1 ||
      focusNames.length > 1
    );
  }

  function isStatsFastFocusAccountContext(ctx = state) {
    const source = normalizeFlowSource(ctx?.source || "");
    if (!isStatsSourceValue(source) || !ctx?.focusOnly || !ctx?.focusCounterpartyStrict) return false;
    if (Math.max(1, Math.min(3, Number(ctx?.hop) || 1)) !== 1) return false;
    const focusKeyType = String(ctx?.focusKeyType || "").trim().toLowerCase();
    const focusIds = parseUniqueList(ctx?.focusIds || [], { allowString: true });
    return focusKeyType === "account" || (!focusKeyType && focusIds.length > 0);
  }

  function shouldUseStatsFastFirstPaint(ctx = state, nodes = [], edges = [], responsePayload = null) {
    if (!isStatsFastFocusAccountContext(ctx)) return false;
    const nodeCount = Array.isArray(nodes) ? nodes.length : 0;
    const edgeCount = Array.isArray(edges) ? edges.length : 0;
    if (!nodeCount || !edgeCount) return false;
    const hints = normalizeGraphRenderHints(
      responsePayload?.render_hints,
      nodeCount,
      edgeCount,
      ctx?.graphMode || responsePayload?.render_hints?.view_mode || "relation"
    );
    if (hints.prefer_fast_first_paint === false) return false;
    if (nodeCount > STATS_FAST_FIRST_PAINT_MAX_NODES || edgeCount > STATS_FAST_FIRST_PAINT_MAX_EDGES) return false;
    const buildMode = String(responsePayload?.stats?.context_applied?.build_mode || "").trim().toLowerCase();
    if (buildMode && buildMode !== "stats_focus_account_fast") return false;
    return true;
  }

  function shouldAnimateLayoutSwitch(nodeCount = 0, edgeCount = 0, preset = "", ctx = state) {
    const nodes = Math.max(0, Number(nodeCount) || 0);
    const edges = Math.max(0, Number(edgeCount) || 0);
    const mode = normalizeLayoutPreset(preset);
    const hints = getActiveGraphRenderHints(ctx, nodes, edges);
    const maxNodes = Math.max(0, Number(hints.layout_switch_animate_max_nodes) || 0);
    const maxEdges = Math.max(0, Number(hints.layout_switch_animate_max_edges) || 0);
    if (nodes <= 1) return false;
    if (nodes <= 20) return true;
    if (!maxNodes || !maxEdges) return false;
    if (mode === "network") {
      return nodes <= Math.min(maxNodes, 900) && edges <= Math.min(maxEdges, 2600);
    }
    return nodes <= maxNodes && edges <= maxEdges;
  }

  function collectStagedLabelFocusIds(ctx = state) {
    const statsBatchFocus = isStatsBatchFocusRequest(ctx);
    const selected =
      ctx.selected instanceof Set
        ? Array.from(ctx.selected)
        : Array.isArray(ctx.selected)
        ? ctx.selected
        : [];
    const source =
      ctx && typeof ctx === "object"
        ? []
            .concat(ctx.focusId == null ? [] : [ctx.focusId])
            .concat(statsBatchFocus ? [] : Array.isArray(ctx.focusIds) ? ctx.focusIds : [])
            .concat(Array.isArray(ctx.leftSeeds) ? ctx.leftSeeds : [])
            .concat(statsBatchFocus ? selected.slice(0, 24) : selected)
        : [];
    return filterNoisySeedIds(source);
  }

  function collectGraphTextRenderSummary(graph = null) {
    const g = graph || state.graph?.instance || null;
    if (!g || typeof g.getDebugInfo !== "function") return null;
    try {
      const text = g.getDebugInfo()?.text || {};
      return {
        active: !!text?.active,
        backend: String(text?.backend || ""),
        labels: Number(text?.labels || 0),
        sets: Number(text?.sets || 0),
        atlases: Number(text?.atlases || 0),
        glyphs: Number(text?.glyphs || 0),
        textureBytes: Number(text?.textureBytes || 0),
      };
    } catch (e) {
      return null;
    }
  }

  function playLightGraphIntro(
    containerEl,
    { duration = 220, translateY = 10, scale = 0.988, isCancelled = null, onComplete = null } = {}
  ) {
    if (!containerEl) {
      if (typeof onComplete === "function") onComplete({ cancelled: false, duration: 0 });
      return;
    }
    const durationMs = Math.max(160, Number(duration) || 220);
    const prevTransition = containerEl.style.transition || "";
    const prevOpacity = containerEl.style.opacity || "";
    const prevTransform = containerEl.style.transform || "";
    const prevWillChange = containerEl.style.willChange || "";
    let settled = false;
    let timer = 0;
    const finish = (cancelled = false) => {
      if (settled) return;
      settled = true;
      if (timer) {
        try {
          clearTimeout(timer);
        } catch (e) {}
      }
      containerEl.style.transition = prevTransition;
      containerEl.style.opacity = prevOpacity || "1";
      containerEl.style.transform = prevTransform || "";
      containerEl.style.willChange = prevWillChange;
      if (typeof onComplete === "function") {
        try {
          onComplete({ cancelled, duration: durationMs });
        } catch (e) {}
      }
    };
    containerEl.style.willChange = "opacity, transform";
    containerEl.style.transition = "none";
    containerEl.style.opacity = "0.01";
    containerEl.style.transform = `translate3d(0, ${Math.max(4, Number(translateY) || 0)}px, 0) scale(${Math.max(
      0.96,
      Math.min(1, Number(scale) || 0.988)
    )})`;
    void containerEl.offsetWidth;
    requestAnimationFrame(() => {
      if (typeof isCancelled === "function" && isCancelled()) {
        finish(true);
        return;
      }
      const easing = "cubic-bezier(0.22, 1, 0.36, 1)";
      containerEl.style.transition = `opacity ${durationMs}ms ${easing}, transform ${durationMs}ms ${easing}`;
      containerEl.style.opacity = prevOpacity || "1";
      containerEl.style.transform = prevTransform || "";
    });
    timer = window.setTimeout(() => {
      finish(typeof isCancelled === "function" && isCancelled());
    }, durationMs + 48);
  }

  function scheduleStagedNodeLabelReveal(plan, { graph = null, isCancelled = null, onComplete = null } = {}) {
    const g = graph || state.graph?.instance || null;
    if (!plan || !Array.isArray(plan.deferred) || !plan.deferred.length || !g) {
      if (typeof onComplete === "function") onComplete({ cancelled: false, duration: 0, batches: 0 });
      return;
    }
    const dataNodeMap = new Map(
      (Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : []).map((node) => [
        String(node?.id || "").trim(),
        node,
      ])
    );
    const dataEdgeMap = new Map(
      (Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : []).map((edge) => [
        String(edge?.id || "").trim(),
        edge,
      ])
    );
    const batchSize = Math.max(24, Number(plan?.budget?.revealBatchSize) || 120);
    const batchGapMs = Math.max(12, Number(plan?.budget?.batchGapMs) || 18);
    const revealDelay = Math.max(0, Number(plan?.budget?.revealDelay) || 0);
    let index = 0;
    let batches = 0;
    let startedAt = 0;
    const applyBatch = () => {
      if (typeof isCancelled === "function" && isCancelled()) {
        if (typeof onComplete === "function") onComplete({ cancelled: true, duration: 0, batches });
        return;
      }
      if (!startedAt) startedAt = performance.now();
      const upper = Math.min(plan.deferred.length, index + batchSize);
      batches += 1;
      g.setAutoPaint(false);
      try {
        for (; index < upper; index += 1) {
          const item = plan.deferred[index] || {};
          if (item.kind === "edge") {
            const patch = {
              label: item.label,
              labelTop: item.labelTop,
              labelBottom: item.labelBottom,
              detailLabel: !!item.detailLabel,
              __baseLabel: item.label,
              __baseLabelTop: item.labelTop,
              __baseLabelBottom: item.labelBottom,
            };
            const runtimeItem = g.findById?.(item.id);
            if (runtimeItem) g.updateItem(runtimeItem, patch);
            const dataEdge = dataEdgeMap.get(String(item.id || "").trim());
            if (dataEdge) Object.assign(dataEdge, patch);
            continue;
          }
          const patch = {
            nodeLabelHidden: !!item.nodeLabelHidden,
            displayId: item.displayId,
            display_id: item.display_id,
          };
          const runtimeItem = g.findById?.(item.id);
          if (runtimeItem) g.updateItem(runtimeItem, patch);
          const dataNode = dataNodeMap.get(String(item.id || "").trim());
          if (dataNode) Object.assign(dataNode, patch);
        }
      } finally {
        g.setAutoPaint(true);
      }
      try {
        g.paint?.();
      } catch (e) {}
      if (index < plan.deferred.length) {
        window.setTimeout(() => {
          schedulePostPaintTask(applyBatch, { timeout: batchGapMs + 80 });
        }, batchGapMs);
        return;
      }
      if (state.layoutPreset === "network") {
        const nodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
        const edges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
        applyDenseNetworkRuntimeLod(g, {
          mode: state.networkLayout?.lastPlanMode || resolveNetworkTopologyModeForGraph(nodes, edges),
          nodeCount: nodes.length,
          edgeCount: edges.length,
        });
      }
      const duration = Math.round((performance.now() - startedAt) * 10) / 10;
      if (typeof onComplete === "function") onComplete({ cancelled: false, duration, batches });
    };
    const kickoff = () => {
      if (typeof isCancelled === "function" && isCancelled()) {
        if (typeof onComplete === "function") onComplete({ cancelled: true, duration: 0, batches });
        return;
      }
      schedulePostPaintTask(applyBatch, { timeout: Math.max(96, revealDelay || 96) });
    };
    if (revealDelay > 0) {
      window.setTimeout(kickoff, revealDelay);
    } else {
      kickoff();
    }
  }

  function getSeedCoreCandidates(statSelection) {
    const ctx =
      statSelection instanceof Set || Array.isArray(statSelection)
        ? { selected: statSelection }
        : statSelection && typeof statSelection === "object"
        ? statSelection
        : {};
    const sourceType = normalizeFlowSource(ctx.source || "");
    const focusOnly = !!ctx.focusOnly;
    const focusKeyType = String(ctx.focusKeyType || ctx.keyType || "").trim().toLowerCase();
    const focusIds = filterNoisySeedIds(
      []
        .concat(ctx.focusId == null ? [] : [ctx.focusId])
        .concat(Array.isArray(ctx.focusIds) ? ctx.focusIds : [])
    );
    const focusNames = parseUniqueList(
      []
        .concat(ctx.focusName == null ? [] : [ctx.focusName])
        .concat(Array.isArray(ctx.focusNames) ? ctx.focusNames : []),
      { allowString: true }
    );
    const focusPlaceholderKinds = parseUniqueList(ctx?.focusPlaceholderKinds || [], { allowString: true });
    const statsBatchFocus = isStatsBatchFocusRequest({
      source: sourceType,
      focusOnly,
      focusIds,
      focusNames,
      focusPlaceholderKinds,
      focusUnknownName: !!ctx.focusUnknownName,
      includeMissingCounterparty: !!ctx.includeMissingCounterparty,
      expectedRowCount: Number(ctx.expectedRowCount) || 0,
    });
    if (
      isStatsSourceValue(sourceType) &&
      focusOnly &&
      (focusKeyType === "account" || !focusKeyType) &&
      !statsBatchFocus
    ) {
      const statsFocusCoreLimit = 6;
      const focusSeeds = focusIds.slice(0, statsFocusCoreLimit);
      if (focusSeeds.length) return focusSeeds;
    }
    const fromSelection =
      ctx.selected instanceof Set
        ? Array.from(ctx.selected)
        : Array.isArray(ctx.selected)
        ? ctx.selected
        : [];
    const fromLeftSeeds = Array.isArray(ctx.leftSeeds) ? ctx.leftSeeds : [];
    const source = fromLeftSeeds.length ? fromLeftSeeds : fromSelection;
    const seedIds = filterNoisySeedIds(source);
    if (!seedIds.length) return [];
    const tab = String(ctx.tab || "").trim().toLowerCase();
    const strictCardSource = isCardKeywordText(focusKeyType) || tab === "bycard";
    if (strictCardSource) return seedIds;
    const treeIndex = buildTreeItemIndex(ctx.treeData);
    if (!treeIndex.size) return seedIds;
    const cardLike = seedIds.filter((id) => isCardLikeTreeItem(treeIndex.get(id)));
    return cardLike.length ? cardLike : seedIds;
  }

  const LAYOUT_SLOT_CACHE_MAX = 2400;
  const LAYOUT_PLAN_CACHE_MAX = 18;
  const layoutPlanCacheController = createLayoutPlanCacheController({
    state,
    ensureSemanticState: ensureLayoutSemanticState,
    planCacheModel: flowLayoutPlanCacheModel,
    applyModel: flowLayoutPlanCacheApplyModel,
    runtimeAdapter: flowLayoutRuntimeAdapter,
    computeLayoutNodePlan,
    projectLayoutCacheKey,
    normalizeLayoutPreset,
    algoVersion: LAYOUT_ALGO_VERSION,
    maxEntries: LAYOUT_PLAN_CACHE_MAX,
  });
  const NETWORK_PREWARM_MIN_NODES = 72;
  const COMPACT_PREWARM_MIN_NODES = 72;
  const NETWORK_PREWARM_TIMEOUT_MS = 12000;
  const layoutPrewarmController = createLayoutPrewarmController({
    state,
    runtimeAdapter: flowLayoutRuntimeAdapter,
    layoutPlanCacheController,
    resolveFocusIdFromNodes,
    collectLayoutHints,
    prepareLayoutSemanticProjection,
    ensureLayoutSemanticState,
    updateClusterSlotCacheFromLayoutReport,
    prefetchNetworkSectorPlacement: prefetchNetworkSectorPlacementWithRust,
    logLayoutCoreMissing,
    shouldLayoutVerboseLog,
    log,
    networkMinNodes: NETWORK_PREWARM_MIN_NODES,
    compactMinNodes: COMPACT_PREWARM_MIN_NODES,
    timeoutMs: NETWORK_PREWARM_TIMEOUT_MS,
    workerScript: LAYOUT_WORKER_SCRIPT,
    workerSupported: typeof Worker !== "undefined",
    requestIdleCallbackFn:
      typeof requestIdleCallback === "function" ? requestIdleCallback : null,
    setTimeoutFn: setTimeout,
    clearTimeoutFn: clearTimeout,
    performanceApi: performance,
  });

  function updateClusterSlotCacheFromLayoutReport(layoutReport, layoutHints = null) {
    const semanticState = ensureLayoutSemanticState();
    const cache = semanticState.clusterSlotBySignature || {};
    updateClusterSlotModelCacheFromLayoutReport(cache, {
      layoutReport,
      layoutHints,
      maxEntries: LAYOUT_SLOT_CACHE_MAX,
    });
    semanticState.clusterSlotBySignature = cache;
  }

  function prefetchNetworkSectorPlacementPayloadRows(rows = [], options = {}) {
    const source = String(options?.source || "").trim();
    const key = String(options?.key || "").slice(0, 96);
    let requested = 0;
    let accepted = 0;
    (Array.isArray(rows) ? rows : []).forEach((row) => {
      const payload =
        row?.payload && typeof row.payload === "object"
          ? row.payload
          : row && typeof row === "object"
            ? row
            : null;
      if (!payload) return;
      requested += 1;
      if (prefetchNetworkSectorPlacementWithRust(payload)) accepted += 1;
    });
    if (requested > 0 && (state.debugTrace || state.debugLayout)) {
      try {
        log("INFO", "network sector placement prewarm queued", {
          source,
          key,
          requested,
          accepted,
        });
      } catch (e) {}
    }
    return { requested, accepted };
  }

  function consumeNetworkSectorPlacementPayloadReport(layoutReport, options = {}) {
    if (!layoutReport || typeof layoutReport !== "object") return null;
    const report = { ...layoutReport };
    const payloadRows = Array.isArray(report.networkSectorPlacementPayloads)
      ? report.networkSectorPlacementPayloads
      : [];
    if (payloadRows.length) {
      prefetchNetworkSectorPlacementPayloadRows(payloadRows, options);
      delete report.networkSectorPlacementPayloads;
    }
    return report;
  }

  function terminateLayoutPrewarmTask(reason = "", mode = "") {
    return layoutPrewarmController.terminateLayoutPrewarmTask(reason, mode);
  }

  function animateToLayout(prevPositions, nextPositions, options = {}) {
    const graph = options?.graph || ensureGraph();
    if (!graph || !(nextPositions instanceof Map) || !nextPositions.size) return false;
    const startMap =
      prevPositions instanceof Map
        ? prevPositions
        : options?.startPositions instanceof Map
        ? options.startPositions
        : null;
    const lag =
      options?.labelLagMs == null ? GRAPH_LAYOUT_CONFIG.LABEL_LAG_MS : Number(options.labelLagMs);
    animateNodesToTargets(graph, nextPositions, {
      duration: Math.max(220, Number(options?.duration) || GRAPH_LAYOUT_CONFIG.ANIM_DURATION),
      startPositions: startMap,
      fit: !!options?.fit,
      finalize: options?.finalize !== false,
      onComplete: typeof options?.onComplete === "function" ? options.onComplete : undefined,
      onProgress: typeof options?.onProgress === "function" ? options.onProgress : undefined,
      pinMap: options?.pinMap instanceof Map ? options.pinMap : undefined,
      easing: options?.easing || GRAPH_LAYOUT_CONFIG.ANIM_EASING,
      labelLagMs: Number.isFinite(lag) ? Math.max(0, lag) : GRAPH_LAYOUT_CONFIG.LABEL_LAG_MS,
    });
    return true;
  }

  function shouldLayoutVerboseLog() {
    return !!(logSwitch?.layoutVerbose || logSwitch?.debug || state?.debugTrace);
  }

  function summarizeCounterRows(counter, limit = 8, { labelKey = "key", valueKey = "count" } = {}) {
    const rows = Object.entries(counter || {}).map(([key, rawValue]) => ({
      [labelKey]: String(key || ""),
      [valueKey]: Math.max(0, Number(rawValue) || 0),
    }));
    rows.sort(
      (a, b) =>
        (b[valueKey] || 0) - (a[valueKey] || 0) ||
        String(a[labelKey] || "").localeCompare(String(b[labelKey] || ""), "zh-CN")
    );
    const max = Math.max(1, Number(limit) || 8);
    return rows.slice(0, max);
  }

  function isSuspiciousLayoutNodeId(id) {
    const text = String(id || "").trim();
    if (!text) return true;
    if (isPlaceholderNodeId(text)) return true;
    if (text === "\\N") return true;
    if (text.includes("__unknown_cp__name::__empty__")) return true;
    if (/^unknown/i.test(text)) return true;
    return false;
  }

  function sampleRoleNodes(rows, role, limit = LAYOUT_LOG_ROLE_SAMPLE_LIMIT) {
    if (!Array.isArray(rows) || !rows.length) return [];
    const max = Math.max(1, Number(limit) || LAYOUT_LOG_ROLE_SAMPLE_LIMIT);
    const out = [];
    for (let i = 0; i < rows.length; i += 1) {
      const row = rows[i] || {};
      const rowRole = String(row?.role || "").toLowerCase();
      if (rowRole !== String(role || "").toLowerCase()) continue;
      out.push(truncateTextMiddle(String(row?.id || ""), 46));
      if (out.length >= max) break;
    }
    return out;
  }

  function summarizeRoleReportRows(rows, clusterRows = []) {
    const roleRows = Array.isArray(rows) ? rows : [];
    const roleCounts = { core: 0, adjacent: 0, leaf: 0, other: 0 };
    const whyCounter = {};
    const clusterCounter = {};
    const suspiciousSample = [];
    let suspiciousCount = 0;
    let layoutAnchorCount = 0;
    roleRows.forEach((row) => {
      const role = String(row?.role || "").toLowerCase();
      if (role === "core" || role === "adjacent" || role === "leaf") roleCounts[role] += 1;
      else roleCounts.other += 1;
      const why = String(row?.why || "").trim();
      if (why) whyCounter[why] = (Number(whyCounter[why]) || 0) + 1;
      const clusterId = String(row?.clusterId || "").trim();
      if (clusterId) clusterCounter[clusterId] = (Number(clusterCounter[clusterId]) || 0) + 1;
      if (row?.layoutAnchorForCluster) layoutAnchorCount += 1;
      if (!isSuspiciousLayoutNodeId(row?.id)) return;
      suspiciousCount += 1;
      if (suspiciousSample.length >= LAYOUT_LOG_ROLE_SAMPLE_LIMIT) return;
      suspiciousSample.push({
        id: truncateTextMiddle(String(row?.id || ""), 52),
        role: role || "",
        why: why || "",
      });
    });
    const rowsByNodeCount = (Array.isArray(clusterRows) ? clusterRows : [])
      .slice()
      .sort(
        (a, b) =>
          (Number(b?.nodeCount) || 0) - (Number(a?.nodeCount) || 0) ||
          String(a?.clusterId || "").localeCompare(String(b?.clusterId || ""), "zh-CN")
      )
      .slice(0, LAYOUT_LOG_CLUSTER_SAMPLE_LIMIT)
      .map((row) => ({
        clusterId: String(row?.clusterId || ""),
        nodeCount: Math.max(0, Number(row?.nodeCount) || 0),
        coreCount: Math.max(0, Number(row?.coreCount) || 0),
        adjacentCount: Math.max(0, Number(row?.adjacentCount) || 0),
        leafCount: Math.max(0, Number(row?.leafCount) || 0),
        layoutCase: String(row?.layoutCase || ""),
      }));
    const total = roleRows.length;
    return {
      total,
      roleCounts,
      leafRatio: total > 0 ? Number((roleCounts.leaf / total).toFixed(3)) : 0,
      layoutAnchorCount,
      whyTop: summarizeCounterRows(whyCounter, 8, { labelKey: "why", valueKey: "count" }),
      clusterCount: Object.keys(clusterCounter).length,
      topClusters: rowsByNodeCount,
      suspiciousNodes: {
        count: suspiciousCount,
        sample: suspiciousSample,
      },
      samples: {
        core: sampleRoleNodes(roleRows, "core"),
        adjacent: sampleRoleNodes(roleRows, "adjacent"),
        leaf: sampleRoleNodes(roleRows, "leaf"),
      },
    };
  }

  function summarizePathPromotionReport(report) {
    const src = report && typeof report === "object" ? report : {};
    const corePairs = Array.isArray(src?.corePairs) ? src.corePairs : [];
    const rejected = Array.isArray(src?.rejected) ? src.rejected : [];
    const hopCounter = {};
    const basisCounter = {};
    const promotedCounter = {};
    const droppedNoisyCounter = {};
    const rejectedCounter = {};
    let candidateSum = 0;
    let maxCandidate = 0;
    let droppedNoisyTotal = 0;
    corePairs.forEach((row) => {
      const hops = Math.max(0, Number(row?.shortestHops) || 0);
      const candidatePathCount = Math.max(0, Number(row?.candidatePathCount) || 0);
      const basis = String(row?.selectedBasis || "").trim();
      hopCounter[hops] = (Number(hopCounter[hops]) || 0) + 1;
      if (basis) basisCounter[basis] = (Number(basisCounter[basis]) || 0) + 1;
      candidateSum += candidatePathCount;
      if (candidatePathCount > maxCandidate) maxCandidate = candidatePathCount;
      (Array.isArray(row?.promotedNodes) ? row.promotedNodes : []).forEach((idRaw) => {
        const id = String(idRaw || "").trim();
        if (!id) return;
        promotedCounter[id] = (Number(promotedCounter[id]) || 0) + 1;
      });
      (Array.isArray(row?.droppedNoisyNodes) ? row.droppedNoisyNodes : []).forEach((idRaw) => {
        const id = String(idRaw || "").trim();
        if (!id) return;
        droppedNoisyCounter[id] = (Number(droppedNoisyCounter[id]) || 0) + 1;
        droppedNoisyTotal += 1;
      });
    });
    rejected.forEach((row) => {
      const reason = String(row?.reason || "unknown").trim() || "unknown";
      rejectedCounter[reason] = (Number(rejectedCounter[reason]) || 0) + 1;
    });
    const highCandidatePairs = corePairs
      .slice()
      .sort(
        (a, b) =>
          (Number(b?.candidatePathCount) || 0) - (Number(a?.candidatePathCount) || 0) ||
          (Number(b?.shortestHops) || 0) - (Number(a?.shortestHops) || 0)
      )
      .slice(0, LAYOUT_LOG_PATH_PAIR_SAMPLE_LIMIT)
      .map((row) => ({
        pair: (Array.isArray(row?.pair) ? row.pair : [])
          .slice(0, 2)
          .map((id) => truncateTextMiddle(String(id || ""), 38)),
        shortestHops: Math.max(0, Number(row?.shortestHops) || 0),
        candidatePathCount: Math.max(0, Number(row?.candidatePathCount) || 0),
        selectedBasis: String(row?.selectedBasis || ""),
        promotedCount: Array.isArray(row?.promotedNodes) ? row.promotedNodes.length : 0,
        promotedSample: (Array.isArray(row?.promotedNodes) ? row.promotedNodes : [])
          .slice(0, 3)
          .map((id) => truncateTextMiddle(String(id || ""), 38)),
        droppedNoisyCount: Array.isArray(row?.droppedNoisyNodes) ? row.droppedNoisyNodes.length : 0,
      }));
    const promotedTop = summarizeCounterRows(promotedCounter, LAYOUT_LOG_PROMOTED_NODE_SAMPLE_LIMIT, {
      labelKey: "id",
      valueKey: "hits",
    }).map((row) => ({
      id: truncateTextMiddle(String(row?.id || ""), 40),
      hits: Math.max(0, Number(row?.hits) || 0),
    }));
    return {
      maxHops: Math.max(0, Number(src?.maxHops) || 0),
      pairCount: corePairs.length,
      rejectedCount: rejected.length,
      truncated: !!src?.truncated,
      shortestHops: summarizeCounterRows(hopCounter, 8, { labelKey: "hops", valueKey: "count" }),
      selectedBasis: summarizeCounterRows(basisCounter, 8, { labelKey: "basis", valueKey: "count" }),
      candidatePathStats: {
        max: maxCandidate,
        avg: corePairs.length ? Number((candidateSum / corePairs.length).toFixed(2)) : 0,
      },
      promotedNodes: {
        distinctCount: Object.keys(promotedCounter).length,
        top: promotedTop,
      },
      droppedNoisyNodes: {
        total: droppedNoisyTotal,
        top: summarizeCounterRows(droppedNoisyCounter, LAYOUT_LOG_PROMOTED_NODE_SAMPLE_LIMIT, {
          labelKey: "id",
          valueKey: "hits",
        }).map((row) => ({
          id: truncateTextMiddle(String(row?.id || ""), 40),
          hits: Math.max(0, Number(row?.hits) || 0),
        })),
      },
      highCandidatePairs,
      rejectedReasons: summarizeCounterRows(rejectedCounter, 8, { labelKey: "reason", valueKey: "count" }),
    };
  }

  function summarizeClusterRowsForLog(clusterRows) {
    const rows = Array.isArray(clusterRows) ? clusterRows : [];
    const totals = {
      nodes: 0,
      edges: 0,
      core: 0,
      adjacent: 0,
      leaf: 0,
      layoutAnchor: 0,
    };
    rows.forEach((row) => {
      totals.nodes += Math.max(0, Number(row?.nodeCount) || 0);
      totals.edges += Math.max(0, Number(row?.edgeCount) || 0);
      totals.core += Math.max(0, Number(row?.coreCount) || 0);
      totals.adjacent += Math.max(0, Number(row?.adjacentCount) || 0);
      totals.leaf += Math.max(0, Number(row?.leafCount) || 0);
      totals.layoutAnchor += Math.max(0, Number(row?.layoutAnchorCount) || 0);
    });
    const topClusters = rows
      .slice()
      .sort(
        (a, b) =>
          (Number(b?.nodeCount) || 0) - (Number(a?.nodeCount) || 0) ||
          String(a?.clusterId || "").localeCompare(String(b?.clusterId || ""), "zh-CN")
      )
      .slice(0, LAYOUT_LOG_CLUSTER_SAMPLE_LIMIT)
      .map((row) => ({
        clusterId: String(row?.clusterId || ""),
        nodeCount: Math.max(0, Number(row?.nodeCount) || 0),
        edgeCount: Math.max(0, Number(row?.edgeCount) || 0),
        coreCount: Math.max(0, Number(row?.coreCount) || 0),
        adjacentCount: Math.max(0, Number(row?.adjacentCount) || 0),
        leafCount: Math.max(0, Number(row?.leafCount) || 0),
        layoutCase: String(row?.layoutCase || ""),
        hasCore: !!row?.hasCore,
      }));
    return {
      totalClusters: rows.length,
      totals,
      leafRatio: totals.nodes > 0 ? Number((totals.leaf / totals.nodes).toFixed(3)) : 0,
      topClusters,
    };
  }

  function summarizeBboxUnion(rows) {
    const source = Array.isArray(rows) ? rows : [];
    let minX = Infinity;
    let maxX = -Infinity;
    let minY = Infinity;
    let maxY = -Infinity;
    let count = 0;
    source.forEach((row) => {
      const box = row?.bbox || {};
      const x1 = Number(box?.minX);
      const x2 = Number(box?.maxX);
      const y1 = Number(box?.minY);
      const y2 = Number(box?.maxY);
      if (!Number.isFinite(x1) || !Number.isFinite(x2) || !Number.isFinite(y1) || !Number.isFinite(y2)) return;
      minX = Math.min(minX, x1);
      maxX = Math.max(maxX, x2);
      minY = Math.min(minY, y1);
      maxY = Math.max(maxY, y2);
      count += 1;
    });
    if (!count) return null;
    return {
      minX: Number(minX.toFixed(2)),
      maxX: Number(maxX.toFixed(2)),
      minY: Number(minY.toFixed(2)),
      maxY: Number(maxY.toFixed(2)),
      width: Number((maxX - minX).toFixed(2)),
      height: Number((maxY - minY).toFixed(2)),
    };
  }

  function summarizeCompactLayoutReport(report) {
    const src = report && typeof report === "object" ? report : {};
    const clusters = Array.isArray(src?.clusters) ? src.clusters : [];
    let totalAvoidanceHits = 0;
    let totalRingCount = 0;
    let totalRingNodes = 0;
    let totalCenters = 0;
    const topClusters = clusters
      .map((cluster) => {
        const rings = Array.isArray(cluster?.rings) ? cluster.rings : [];
        const centers = Array.isArray(cluster?.centers) ? cluster.centers : [];
        const ringNodes = rings.reduce((sum, ring) => sum + Math.max(0, Number(ring?.count) || 0), 0);
        const avoidanceHits = Math.max(0, Number(cluster?.avoidanceHits) || 0);
        totalAvoidanceHits += avoidanceHits;
        totalRingCount += Math.max(0, Number(cluster?.ringCount) || 0);
        totalRingNodes += ringNodes;
        totalCenters += centers.length;
        return {
          clusterId: String(cluster?.clusterId || ""),
          layoutCase: String(cluster?.layoutCase || ""),
          centerCount: centers.length,
          ringCount: Math.max(0, Number(cluster?.ringCount) || 0),
          ringNodes,
          avoidanceHits,
          bbox: cluster?.bbox || null,
        };
      })
      .sort(
        (a, b) =>
          b.avoidanceHits - a.avoidanceHits ||
          b.ringNodes - a.ringNodes ||
          String(a.clusterId || "").localeCompare(String(b.clusterId || ""), "zh-CN")
      )
      .slice(0, LAYOUT_LOG_CLUSTER_SAMPLE_LIMIT);
    return {
      mode: "compact",
      clusterCount: Math.max(0, Number(src?.clusterCount) || clusters.length),
      bbox: summarizeBboxUnion(clusters),
      totals: {
        centers: totalCenters,
        ringCount: totalRingCount,
        ringNodes: totalRingNodes,
        avoidanceHits: totalAvoidanceHits,
      },
      topClusters,
    };
  }

  function summarizeNetworkLayoutReport(report) {
    const src = report && typeof report === "object" ? report : {};
    const clusters = Array.isArray(src?.clusters) ? src.clusters : [];
    let totalCenters = 0;
    let totalAvoidanceHits = 0;
    let boundaryPenaltyCenters = 0;
    let totalEnvelopeAdjustments = 0;
    let totalEnvelopeRadiusPulls = 0;
    let totalEnvelopeAnglePulls = 0;
    let totalUniformAdjustments = 0;
    let totalUniformPlanned = 0;
    let totalUniformRejected = 0;
    let totalUniformSparseLayerRebalances = 0;
    let totalUniformFallbackRescues = 0;
    const centerDiagnostics = [];
    const topClusters = clusters
      .map((cluster) => {
        const centers = Array.isArray(cluster?.centers) ? cluster.centers : [];
        const avoidanceHits = centers.reduce((sum, row) => sum + Math.max(0, Number(row?.avoidanceHits) || 0), 0);
        const maxTopScore = centers.reduce(
          (max, row) => Math.max(max, Number.isFinite(Number(row?.topScore)) ? Number(row.topScore) : 0),
          0
        );
        const penaltyCenters = centers.filter((row) => !!row?.boundaryPenaltyTriggered).length;
        const envelopeAdjustments = centers.reduce(
          (sum, row) => sum + Math.max(0, Number(row?.envelopeAdjustments) || 0),
          0
        );
        const envelopeRadiusPulls = centers.reduce(
          (sum, row) => sum + Math.max(0, Number(row?.envelopeRadiusPulls) || 0),
          0
        );
        const envelopeAnglePulls = centers.reduce(
          (sum, row) => sum + Math.max(0, Number(row?.envelopeAnglePulls) || 0),
          0
        );
        const uniformAdjustments = centers.reduce(
          (sum, row) => sum + Math.max(0, Number(row?.uniformAdjustments) || 0),
          0
        );
        const uniformPlanned = centers.reduce(
          (sum, row) => sum + Math.max(0, Number(row?.uniformPlanned) || 0),
          0
        );
        const uniformRejected = centers.reduce(
          (sum, row) => sum + Math.max(0, Number(row?.uniformRejected) || 0),
          0
        );
        const uniformSparseLayerRebalances = centers.reduce(
          (sum, row) => sum + Math.max(0, Number(row?.uniformSparseLayerRebalances) || 0),
          0
        );
        const uniformFallbackRescues = centers.reduce(
          (sum, row) => sum + Math.max(0, Number(row?.uniformFallbackRescues) || 0),
          0
        );
        totalCenters += centers.length;
        totalAvoidanceHits += avoidanceHits;
        boundaryPenaltyCenters += penaltyCenters;
        totalEnvelopeAdjustments += envelopeAdjustments;
        totalEnvelopeRadiusPulls += envelopeRadiusPulls;
        totalEnvelopeAnglePulls += envelopeAnglePulls;
        totalUniformAdjustments += uniformAdjustments;
        totalUniformPlanned += uniformPlanned;
        totalUniformRejected += uniformRejected;
        totalUniformSparseLayerRebalances += uniformSparseLayerRebalances;
        totalUniformFallbackRescues += uniformFallbackRescues;
        centers.forEach((row) => {
          if (!row || typeof row !== "object") return;
          centerDiagnostics.push({
            centerId: String(row?.centerId || ""),
            centerType: String(row?.centerType || ""),
            leafCount: Math.max(0, Number(row?.leafCount) || 0),
            sectorCount: Math.max(0, Number(row?.sectorCount) || 0),
            maxLeafRadius: Number.isFinite(Number(row?.maxLeafRadius))
              ? Number(Number(row.maxLeafRadius).toFixed(2))
              : null,
            avoidanceHits: Math.max(0, Number(row?.avoidanceHits) || 0),
            uniformAdjustments: Math.max(0, Number(row?.uniformAdjustments) || 0),
            uniformPlanned: Math.max(0, Number(row?.uniformPlanned) || 0),
            uniformRejected: Math.max(0, Number(row?.uniformRejected) || 0),
            uniformSparseLayerRebalances: Math.max(
              0,
              Number(row?.uniformSparseLayerRebalances) || 0
            ),
            uniformFallbackRescues: Math.max(0, Number(row?.uniformFallbackRescues) || 0),
            envelopeAdjustments: Math.max(0, Number(row?.envelopeAdjustments) || 0),
          });
        });
        return {
          clusterId: String(cluster?.clusterId || ""),
          layoutCase: String(cluster?.layoutCase || ""),
          centerCount: centers.length,
          avoidanceHits,
          boundaryPenaltyCenters: penaltyCenters,
          envelopeAdjustments,
          envelopeRadiusPulls,
          envelopeAnglePulls,
          uniformAdjustments,
          uniformPlanned,
          uniformRejected,
          uniformSparseLayerRebalances,
          uniformFallbackRescues,
          maxTopScore: Number(maxTopScore.toFixed(3)),
          bbox: cluster?.bbox || null,
        };
      })
      .sort(
        (a, b) =>
          b.avoidanceHits - a.avoidanceHits ||
          b.centerCount - a.centerCount ||
          String(a.clusterId || "").localeCompare(String(b.clusterId || ""), "zh-CN")
      )
      .slice(0, LAYOUT_LOG_CLUSTER_SAMPLE_LIMIT);
    return {
      mode: "network",
      clusterCount: Math.max(0, Number(src?.clusterCount) || clusters.length),
      bbox: summarizeBboxUnion(clusters),
      totals: {
        centers: totalCenters,
        avoidanceHits: totalAvoidanceHits,
        boundaryPenaltyCenters,
        envelopeAdjustments: totalEnvelopeAdjustments,
        envelopeRadiusPulls: totalEnvelopeRadiusPulls,
        envelopeAnglePulls: totalEnvelopeAnglePulls,
        uniformAdjustments: totalUniformAdjustments,
        uniformPlanned: totalUniformPlanned,
        uniformRejected: totalUniformRejected,
        uniformSparseLayerRebalances: totalUniformSparseLayerRebalances,
        uniformFallbackRescues: totalUniformFallbackRescues,
      },
      topCenters: centerDiagnostics
        .sort(
          (a, b) =>
            b.leafCount - a.leafCount ||
            b.uniformAdjustments - a.uniformAdjustments ||
            String(a.centerId || "").localeCompare(String(b.centerId || ""), "zh-CN")
        )
        .slice(0, 8),
      topClusters,
    };
  }

  function samplePathPromotionReportForVerbose(report, limit = LAYOUT_DEBUG_ROW_LIMIT) {
    const src = report && typeof report === "object" ? report : {};
    const max = Math.max(1, Number(limit) || LAYOUT_DEBUG_ROW_LIMIT);
    const corePairs = Array.isArray(src?.corePairs) ? src.corePairs : [];
    const rejected = Array.isArray(src?.rejected) ? src.rejected : [];
    return {
      maxHops: Math.max(0, Number(src?.maxHops) || 0),
      truncated: !!src?.truncated,
      corePairs: corePairs.slice(0, max).map((row) => ({
        pair: (Array.isArray(row?.pair) ? row.pair : []).slice(0, 2),
        shortestHops: Math.max(0, Number(row?.shortestHops) || 0),
        candidatePathCount: Math.max(0, Number(row?.candidatePathCount) || 0),
        neighborhoodFilteredPathCount: Math.max(0, Number(row?.neighborhoodFilteredPathCount) || 0),
        selectedBasis: String(row?.selectedBasis || ""),
        promotedNodes: (Array.isArray(row?.promotedNodes) ? row.promotedNodes : []).slice(0, 8),
        promotedNodesTruncated: (Array.isArray(row?.promotedNodes) ? row.promotedNodes.length : 0) > 8,
        droppedNoisyNodes: (Array.isArray(row?.droppedNoisyNodes) ? row.droppedNoisyNodes : []).slice(0, 8),
        droppedNoisyNodesTruncated:
          (Array.isArray(row?.droppedNoisyNodes) ? row.droppedNoisyNodes.length : 0) > 8,
      })),
      corePairsTruncated: corePairs.length > max,
      rejected: rejected.slice(0, Math.max(8, Math.floor(max / 2))),
      rejectedTruncated: rejected.length > Math.max(8, Math.floor(max / 2)),
    };
  }

  function summarizeLayoutReportPayload(report) {
    if (!report || typeof report !== "object") return null;
    const out = {};
    const roleReport = report?.roleReport;
    if (roleReport && typeof roleReport === "object") {
      if ("roleCounts" in roleReport && !Array.isArray(roleReport?.nodes)) {
        out.roleReport = roleReport;
      } else {
        out.roleReport = summarizeRoleReportRows(
          Array.isArray(roleReport?.nodes) ? roleReport.nodes : [],
          Array.isArray(report?.clusterReport?.clusters) ? report.clusterReport.clusters : []
        );
      }
    }
    const pathReport = report?.pathPromotionReport;
    if (pathReport && typeof pathReport === "object") {
      if ("pairCount" in pathReport && !Array.isArray(pathReport?.corePairs)) {
        out.pathPromotionReport = pathReport;
      } else {
        out.pathPromotionReport = summarizePathPromotionReport(pathReport);
      }
    }
    const clusterReport = report?.clusterReport;
    if (clusterReport && typeof clusterReport === "object") {
      if ("totals" in clusterReport && !Array.isArray(clusterReport?.clusters)) {
        out.clusterReport = clusterReport;
      } else {
        out.clusterReport = summarizeClusterRowsForLog(
          Array.isArray(clusterReport?.clusters) ? clusterReport.clusters : []
        );
      }
    }
    if (report?.compact && typeof report.compact === "object") {
      out.compact = summarizeCompactLayoutReport(report.compact);
    }
    if (report?.network && typeof report.network === "object") {
      out.network = summarizeNetworkLayoutReport(report.network);
    }
    return out;
  }

  function digestLayoutReportPayload(report) {
    if (!report || typeof report !== "object") return null;
    const role = report?.roleReport && typeof report.roleReport === "object" ? report.roleReport : {};
    const path = report?.pathPromotionReport && typeof report.pathPromotionReport === "object" ? report.pathPromotionReport : {};
    const cluster =
      report?.clusterReport && typeof report.clusterReport === "object" ? report.clusterReport : {};
    const compact = report?.compact && typeof report.compact === "object" ? report.compact : null;
    const network = report?.network && typeof report.network === "object" ? report.network : null;
    const digest = {};
    if (Object.keys(role).length) {
      digest.roleReport = {
        total: Math.max(0, Number(role?.total) || 0),
        roleCounts: role?.roleCounts || {},
        leafRatio: Number(role?.leafRatio) || 0,
        clusterCount: Math.max(0, Number(role?.clusterCount) || 0),
        suspiciousNodes: Math.max(0, Number(role?.suspiciousNodes?.count) || 0),
        hiddenLeafLabels: Math.max(0, Number(role?.labelPolicy?.hiddenLeafLabels) || 0),
        hiddenAdjacentLabels: Math.max(0, Number(role?.labelPolicy?.hiddenAdjacentLabels) || 0),
      };
    }
    if (Object.keys(path).length) {
      digest.pathPromotionReport = {
        pairCount: Math.max(0, Number(path?.pairCount) || 0),
        rejectedCount: Math.max(0, Number(path?.rejectedCount) || 0),
        promotedDistinct: Math.max(0, Number(path?.promotedNodes?.distinctCount) || 0),
        droppedNoisyTotal: Math.max(0, Number(path?.droppedNoisyNodes?.total) || 0),
        candidatePathStats: {
          max: Math.max(0, Number(path?.candidatePathStats?.max) || 0),
          avg: Number(path?.candidatePathStats?.avg) || 0,
        },
      };
    }
    if (Object.keys(cluster).length) {
      digest.clusterReport = {
        totalClusters: Math.max(0, Number(cluster?.totalClusters) || 0),
        totals: cluster?.totals || {},
        leafRatio: Number(cluster?.leafRatio) || 0,
      };
    }
    if (compact) {
      digest.compact = {
        clusterCount: Math.max(0, Number(compact?.clusterCount) || 0),
        bbox: compact?.bbox || null,
        totals: compact?.totals || {},
      };
    }
    if (network) {
      digest.network = {
        clusterCount: Math.max(0, Number(network?.clusterCount) || 0),
        bbox: network?.bbox || null,
        totals: network?.totals || {},
        topCenters: Array.isArray(network?.topCenters) ? network.topCenters.slice(0, 4) : [],
      };
    }
    return digest;
  }

  function cloneLayoutSemanticValue(value) {
    if (!value || typeof value !== "object") return null;
    try {
      return JSON.parse(JSON.stringify(value));
    } catch (e) {
      return null;
    }
  }

  function normalizeLayoutSemanticProjection(value) {
    const source =
      value?.projection && typeof value.projection === "object" ? value.projection : value;
    if (!source || typeof source !== "object") return null;
    const roleGraph = cloneLayoutSemanticValue(source.roleGraph);
    const demotion = cloneLayoutSemanticValue(source.demotion);
    const promotion = cloneLayoutSemanticValue(source.promotion);
    const roleResult = cloneLayoutSemanticValue(source.roleResult);
    const clusterResult = cloneLayoutSemanticValue(source.clusterResult);
    const clusterLayoutPlan = cloneLayoutSemanticValue(source.clusterLayoutPlan);
    const preferredCoreIds = cloneLayoutSemanticValue(source.preferredCoreIds);
    const labelPolicy = cloneLayoutSemanticValue(source.labelPolicy);
    if (
      !roleGraph ||
      !demotion ||
      !promotion ||
      !roleResult ||
      !clusterResult ||
      !Array.isArray(roleGraph.nodeIds) ||
      !Array.isArray(clusterResult.clusters) ||
      !roleResult.rolesById ||
      !roleResult.nodeMetaById ||
      !clusterResult.nodeClusterById ||
      !labelPolicy ||
      typeof labelPolicy !== "object"
    ) {
      return null;
    }
    if (clusterResult.nodeMetaById && typeof clusterResult.nodeMetaById === "object") {
      roleResult.nodeMetaById = cloneLayoutSemanticValue(clusterResult.nodeMetaById) || roleResult.nodeMetaById;
    }
    return {
      roleGraph,
      demotion,
      promotion,
      roleResult,
      clusterResult,
      clusterLayoutPlan,
      preferredCoreIds: Array.isArray(preferredCoreIds) ? preferredCoreIds : [],
      labelPolicy,
    };
  }

  function collectLayoutSeedCoreCandidates(nodeRows) {
    const rows = Array.isArray(nodeRows) ? nodeRows : [];
    const nodeIdSet = new Set(
      rows
        .map((node) => String(node?.id || "").trim())
        .filter(Boolean)
    );
    let seedCoreCandidates = getSeedCoreCandidates({
      selected: state.selected,
      leftSeeds: state.lastRequest?.leftSeeds,
      source: state.source,
      focusOnly: state.focusOnly,
      focusId: state.focusId,
      focusIds: state.focusIds,
      focusName: state.focusName,
      focusNames: state.focusNames,
      focusPlaceholderKinds: state.focusPlaceholderKinds,
      focusUnknownName: state.focusUnknownName,
      includeMissingCounterparty: state.includeMissingCounterparty,
      focusKeyType: state.focusKeyType,
      keyType: state.focusKeyType,
      expectedRowCount: Number(state.expectedRowCount) || 0,
      tab: state.tab,
      treeData: state.treeData,
    }).filter((id) => nodeIdSet.has(id));
    if (!seedCoreCandidates.length && !isStatsSourceValue(state.source)) {
      seedCoreCandidates = sortStableIds(
        rows
          .filter((node) => String(node?.ntype || "").toLowerCase() === "seed")
          .map((node) => String(node?.id || "").trim())
          .filter(Boolean)
      );
    }
    return seedCoreCandidates;
  }

  function resolveClusterLayoutMargin(mode, cfg = {}) {
    const layoutMode = String(mode || "").trim().toLowerCase();
    return layoutMode === "network"
      ? Math.max(56, Number(cfg.NETWORK_CLUSTER_MARGIN) || 136)
      : Math.max(42, Number(cfg.COMPACT_CLUSTER_MARGIN) || 110);
  }

  async function prepareLayoutSemanticProjection(nodes, edges, mode, options = {}) {
    const layoutMode = normalizeLayoutPreset(mode);
    if (layoutMode !== "compact" && layoutMode !== "network" && layoutMode !== "relation") {
      return null;
    }
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const edgeRows = Array.isArray(edges) ? edges : [];
    const cfg = getGraphLayoutConfig();
    const semanticState = ensureLayoutSemanticState();
    const clusterLayoutMode = layoutMode === "network" ? "network" : "compact";
    const startedAt = performance.now();
    const response = await projectLayoutRoleGraph({
      nodes: nodeRows,
      edges: edgeRows,
      semanticPipeline: {
        seedCoreCandidates: collectLayoutSeedCoreCandidates(nodeRows),
        focusId: state.focusId,
        options: {
          ...cfg,
          seedHistory: semanticState.seedCoreHistory || {},
        },
        clusterLayout: {
          mode: clusterLayoutMode,
          config: cfg,
          margin: resolveClusterLayoutMargin(layoutMode, cfg),
          prevAnchorBySignature: semanticState.clusterAnchorBySignature || {},
          prevCenterBySignature: semanticState.clusterCenterBySignature || {},
          slotBySignature: semanticState.clusterSlotBySignature || {},
        },
      },
    });
    const semanticProjection = normalizeLayoutSemanticProjection(response?.projection || response);
    if (!semanticProjection) {
      throw new Error("layout-semantic-pipeline-unavailable");
    }
    if (
      options?.commitSemanticState !== false &&
      semanticProjection.demotion?.nextSeedCoreHistory &&
      typeof semanticProjection.demotion.nextSeedCoreHistory === "object"
    ) {
      semanticState.seedCoreHistory = semanticProjection.demotion.nextSeedCoreHistory;
    }
    if (state.debugLayout || state.debugTrace) {
      try {
        log("INFO", "layout semantic pipeline projected", {
          reason: String(options?.reason || ""),
          mode: layoutMode,
          nodes: nodeRows.length,
          edges: edgeRows.length,
          durationMs: Math.round((performance.now() - startedAt) * 10) / 10,
        });
      } catch (e) {}
    }
    return semanticProjection;
  }

  function collectLayoutHints(nodes, edges, mode, options = {}) {
    const layoutOpts = options && typeof options === "object" ? options : {};
    const logSource = String(layoutOpts.logSource || "active").trim().toLowerCase() || "active";
    const logReason = String(layoutOpts.logReason || "").trim();
    const suppressSemanticLog = !!layoutOpts.suppressSemanticLog;
    const layoutMode = String(mode || "").trim().toLowerCase();
    if (layoutMode !== "compact" && layoutMode !== "network" && layoutMode !== "relation") {
      return {};
    }
    const nodeRows = Array.isArray(nodes) ? nodes : [];
    const edgeRows = Array.isArray(edges) ? edges : [];
    const cfg = getGraphLayoutConfig();
    const semanticState = ensureLayoutSemanticState();
    const projectedSemantic = normalizeLayoutSemanticProjection(layoutOpts.semanticProjection);
    if (!projectedSemantic) {
      throw new Error("layout-semantic-projection-required");
    }
    const roleGraph = projectedSemantic.roleGraph;
    const promotionResult = projectedSemantic.promotion;
    const roleResult = projectedSemantic.roleResult;
    const clusterResult = projectedSemantic.clusterResult;
    const clusterLayoutPlan = projectedSemantic.clusterLayoutPlan;
    if (
      !clusterLayoutPlan ||
      typeof clusterLayoutPlan !== "object" ||
      !clusterLayoutPlan.clusterPlacementById ||
      !clusterLayoutPlan.nextCenterBySignature ||
      !clusterLayoutPlan.nextAnchorBySignature
    ) {
      throw new Error("layout-cluster-plan-required");
    }
    if (projectedSemantic?.demotion?.nextSeedCoreHistory) {
      semanticState.seedCoreHistory = projectedSemantic.demotion.nextSeedCoreHistory || {};
    }
    semanticState.clusterAnchorBySignature = clusterLayoutPlan.nextAnchorBySignature || {};
    semanticState.clusterCenterBySignature = clusterLayoutPlan.nextCenterBySignature || {};
    const clusterById = new Map();
    const clusterAnchorSetById = new Map();
    clusterResult.clusters.forEach((cluster) => {
      const clusterId = String(cluster?.clusterId || "").trim();
      if (!clusterId) return;
      clusterById.set(clusterId, cluster);
      const anchorSet = new Set(
        (cluster?.layoutAnchorIds || [])
          .map((id) => String(id || "").trim())
          .filter(Boolean)
      );
      clusterAnchorSetById.set(clusterId, anchorSet);
    });
    const globalPlan = clusterLayoutPlan;
    const nodeById = new Map(nodeRows.map((node) => [String(node?.id || "").trim(), node]));
    const roleByNodeId = {};
    const clusterByNodeId = {};
    roleGraph.nodeIds.forEach((id) => {
      const node = nodeById.get(id);
      const role = String(roleResult.rolesById?.[id] || "leaf");
      const clusterId = String(clusterResult.nodeClusterById?.[id] || "");
      const meta = roleResult.nodeMetaById?.[id] || {};
      roleByNodeId[id] = role;
      clusterByNodeId[id] = clusterId;
      if (!node) return;
      node.role = role;
      node.clusterId = clusterId;
      node.seedCoreCandidate = !!meta.seedCoreCandidate;
      node.isSeedSelected = !!meta.isSeedSelected;
      node.demoteReason = meta.demoteReason || null;
      node.isPathPromotedAdjacent = !!meta.isPathPromotedAdjacent;
      node.isBridgeAdjacent = !!meta.isBridgeAdjacent;
      node.isHubAdjacent = !!meta.isHubAdjacent;
      node.layoutAnchorForCluster = !!meta.layoutAnchorForCluster;
      node.weight = Number(meta.weight) || 0;
      node.prevX = Number.isFinite(node?.x) ? Number(node.x) : null;
      node.prevY = Number.isFinite(node?.y) ? Number(node.y) : null;
      node.layoutBand = role === "core" ? "core" : role === "adjacent" ? "adjacent" : "leaf";
      node.layoutCase = String(clusterById.get(clusterId)?.layoutCase || "");
      node.layoutClusterId = clusterId || "";
      node.layoutRoleWhy = Array.isArray(meta?.why) ? meta.why.join("|") : "";
      if (clusterAnchorSetById.get(clusterId)?.has(id)) {
        node.layoutAnchorForCluster = true;
      }
    });
    // Dense-label suppression is disabled here; keep render-plan label state intact.
    nodeRows.forEach((node) => {
      if (!node || typeof node !== "object") return;
      node.__labelForceShow = false;
    });
    const labelPolicy = projectedSemantic.labelPolicy;
    edgeRows.forEach((edge) => {
      const modeText = String(edge?.mode || "").toLowerCase();
      const arrow = String(edge?.edgeArrow || edge?.arrow || "").toLowerCase();
      edge.lineType = modeText === "double" || arrow === "both" ? "double" : "single";
      edge.isVisible = edge?.isVisible !== false;
    });
    const verboseLayoutLog = shouldLayoutVerboseLog();
    const shouldBuildSemanticReports =
      !suppressSemanticLog &&
      (verboseLayoutLog ||
        shouldEmitLog("DEBUG", "layout semantic role report") ||
        state.debugTrace ||
        state.debugLayout);
    let roleDebugRows = [];
    let clusterRows = [];
    if (shouldBuildSemanticReports) {
      roleDebugRows = roleGraph.nodeIds.map((id) => {
        const meta = roleResult.nodeMetaById?.[id] || {};
        const role = String(roleResult.rolesById?.[id] || "");
        const primaryWhy =
          (meta?.why || []).find((row) =>
            [
              "seed_core_kept",
              "demote_deg1",
              "demote_deg2",
              "demote_coreOnly",
              "path_promoted_adjacent",
              "bridge_adjacent",
              "hub_adjacent",
              "leaf_single_entry",
            ].includes(row)
          ) ||
          (role === "core" ? "seed_core_kept" : role === "adjacent" ? "bridge_adjacent" : "leaf_single_entry");
        return {
          id,
          role,
          clusterId: String(clusterResult.nodeClusterById?.[id] || ""),
          why: primaryWhy,
          layoutAnchorForCluster: !!meta.layoutAnchorForCluster,
        };
      });
      clusterRows = clusterResult.clusters.map((cluster) => ({
        clusterId: cluster.clusterId,
        nodeCount: cluster.nodeIds.length,
        edgeCount: cluster.edgeCount,
        coreCount: cluster.coreIds.length,
        adjacentCount: cluster.adjacentIds.length,
        leafCount: cluster.leafIds.length,
        layoutAnchorCount: cluster.layoutAnchorIds.length,
        layoutCase: cluster.layoutCase,
        hasCore: cluster.hasCore,
      }));
    }
    const canvasSize = getGraphContainerSize();
    const halfW = Math.max(920, Number(canvasSize?.w) * 0.9 || 920);
    const halfH = Math.max(720, Number(canvasSize?.h) * 0.9 || 720);
    const semanticReports = shouldBuildSemanticReports
      ? {
          roleDebug: roleDebugRows,
          pathPromotion: promotionResult.promotionReport,
          labelPolicy,
          clusters: {
            totalClusters: clusterResult.clusters.length,
            rows: clusterRows,
          },
        }
      : {
          labelPolicy,
          clusters: {
            totalClusters: clusterResult.clusters.length,
            rows: [],
          },
        };
    const semanticPayload = {
      mode: layoutMode === "relation" ? "compact" : layoutMode,
      config: {
        MAX_PROMOTION_HOPS: cfg.MAX_PROMOTION_HOPS,
        CORE_NEIGHBORHOOD_HOPS: cfg.CORE_NEIGHBORHOOD_HOPS,
        SUPPRESS_NOISY_PATH_PROMOTION: cfg.SUPPRESS_NOISY_PATH_PROMOTION !== false,
        ENABLE_SEED_PROTECTION: !!cfg.ENABLE_SEED_PROTECTION,
        SEED_DEMOTION_BYPASS_WEIGHT:
          Number.isFinite(Number(cfg.SEED_DEMOTION_BYPASS_WEIGHT))
            ? Number(cfg.SEED_DEMOTION_BYPASS_WEIGHT)
            : null,
        R_ADJ_MIN: cfg.R_ADJ_MIN,
        R_ADJ_MAX: cfg.R_ADJ_MAX,
        R_LEAF_MIN: cfg.R_LEAF_MIN,
        COMPACT_CORE_SPACING: cfg.COMPACT_CORE_SPACING,
        COMPACT_ADJ_SPACING: cfg.COMPACT_ADJ_SPACING,
        COMPACT_RING_GAP: cfg.COMPACT_RING_GAP,
        COMPACT_CLUSTER_MARGIN: cfg.COMPACT_CLUSTER_MARGIN,
        NETWORK_CORE_SPACING: cfg.NETWORK_CORE_SPACING,
        NETWORK_ADJ_SPACING: cfg.NETWORK_ADJ_SPACING,
        NETWORK_SECTOR_MIN_ANGLE: cfg.NETWORK_SECTOR_MIN_ANGLE,
        NETWORK_SECTOR_MAX_ANGLE: cfg.NETWORK_SECTOR_MAX_ANGLE,
        NETWORK_SECTOR_LAYER_GAP: cfg.NETWORK_SECTOR_LAYER_GAP,
        NETWORK_CLUSTER_MARGIN: cfg.NETWORK_CLUSTER_MARGIN,
        ANIM_DURATION: cfg.ANIM_DURATION,
        ANIM_EASING: cfg.ANIM_EASING,
        LABEL_LAG_MS: cfg.LABEL_LAG_MS,
        ANGULAR_BUCKETS: cfg.ANGULAR_BUCKETS,
      },
      nodeMetaById: roleResult.nodeMetaById,
      rolesById: roleByNodeId,
      leafEntryById: roleResult.leafEntryById,
      clusters: clusterResult.clusters.map((cluster) => ({
        clusterId: cluster.clusterId,
        nodeIds: sortStableIds(cluster.nodeIds),
        coreIds: sortStableIds(cluster.coreIds),
        adjacentIds: sortStableIds(cluster.adjacentIds),
        leafIds: sortStableIds(cluster.leafIds),
        layoutAnchorIds: sortStableIds(cluster.layoutAnchorIds),
        layoutCase: cluster.layoutCase,
        hasCore: !!cluster.hasCore,
        edgeCount: Number(cluster.edgeCount) || 0,
        bbox: cluster.bbox || null,
        localAdjacency: cluster.localAdjacency || {},
      })),
      clusterPlacementById: globalPlan.clusterPlacementById,
      networkNodeUpdatesByClusterId: globalPlan.networkNodeUpdatesByClusterId || {},
      networkCenterReportsByClusterId: globalPlan.networkCenterReportsByClusterId || {},
      networkLeafZonesByClusterId: globalPlan.networkLeafZonesByClusterId || {},
      reports: semanticReports,
      roleGraph: {
        components: roleGraph.components.map((comp, index) => ({
          componentId: index,
          nodeIds: comp,
        })),
      },
      canvasBounds: {
        minX: -halfW,
        minY: -halfH,
        maxX: halfW,
        maxY: halfH,
      },
    };
    semanticState.roleByNodeId = roleByNodeId;
    semanticState.clusterByNodeId = clusterByNodeId;
    semanticState.layoutCaseByClusterId = rolePlainObjectFromMap(
      new Map(clusterResult.clusters.map((cluster) => [cluster.clusterId, cluster.layoutCase]))
    );
    semanticState.lastReports = semanticPayload.reports;
    let layoutLogPayload = null;
    if (shouldBuildSemanticReports) {
      const withLogContext = (payload) => {
        if (logSource === "active" && !logReason) return payload;
        return {
          ...(payload || {}),
          logSource,
          ...(logReason ? { logReason } : {}),
        };
      };
      const roleSummary = summarizeRoleReportRows(roleDebugRows, clusterRows);
      const pathSummary = summarizePathPromotionReport(promotionResult.promotionReport);
      const clusterSummary = summarizeClusterRowsForLog(clusterRows);
      const sampledRoleRows =
        roleDebugRows.length > LAYOUT_DEBUG_ROW_LIMIT
          ? roleDebugRows.slice(0, LAYOUT_DEBUG_ROW_LIMIT)
          : roleDebugRows;
      layoutLogPayload = {
        roleReport: {
          ...roleSummary,
          labelPolicy,
        },
        pathPromotionReport: pathSummary,
        clusterReport: clusterSummary,
      };
      if (verboseLayoutLog) {
        layoutLogPayload.detail = {
          roleReport: {
            total: roleDebugRows.length,
            truncated: roleDebugRows.length > sampledRoleRows.length,
            nodes: sampledRoleRows,
          },
          pathPromotionReport: samplePathPromotionReportForVerbose(
            promotionResult.promotionReport,
            LAYOUT_DEBUG_ROW_LIMIT
          ),
          clusterReport: {
            totalClusters: clusterResult.clusters.length,
            clusters: clusterRows.slice(0, Math.max(12, LAYOUT_DEBUG_ROW_LIMIT)),
            truncated: clusterRows.length > Math.max(12, LAYOUT_DEBUG_ROW_LIMIT),
          },
        };
      }
      try {
        const semanticLogLevel = verboseLayoutLog ? "INFO" : "DEBUG";
        log(
          semanticLogLevel,
          "layout semantic role report",
          withLogContext(
            verboseLayoutLog
              ? {
                  ...roleSummary,
                  labelPolicy,
                  sampledNodes: sampledRoleRows,
                  sampledNodesTruncated: roleDebugRows.length > sampledRoleRows.length,
                }
              : { ...roleSummary, labelPolicy }
          )
        );
        log(
          semanticLogLevel,
          "layout semantic path report",
          withLogContext(
            verboseLayoutLog
              ? {
                  ...pathSummary,
                  detail: samplePathPromotionReportForVerbose(
                    promotionResult.promotionReport,
                    LAYOUT_DEBUG_ROW_LIMIT
                  ),
                }
              : pathSummary
          )
        );
        log(
          semanticLogLevel,
          "layout semantic cluster report",
          withLogContext(
            verboseLayoutLog
              ? {
                  ...clusterSummary,
                  detail: {
                    clusters: clusterRows.slice(0, Math.max(12, LAYOUT_DEBUG_ROW_LIMIT)),
                    truncated: clusterRows.length > Math.max(12, LAYOUT_DEBUG_ROW_LIMIT),
                  },
                }
              : clusterSummary
          )
        );
      } catch (e) {}
    }
    const out = {
      preferredCoreIds: Array.isArray(projectedSemantic.preferredCoreIds)
        ? projectedSemantic.preferredCoreIds
        : [],
      corePlacement: "semantic-corridor",
      corePin: "none",
      coreSource: "hint-first",
      semantic: semanticPayload,
    };
    if (layoutLogPayload && typeof layoutLogPayload === "object") {
      out.__layoutReport = layoutLogPayload;
    }
    return out;
  }

  function applyLayoutSync(nodes, edges, mode, focusId, layoutHints = {}) {
    const engine = getLayoutEngine();
    if (!engine || typeof engine.applyLayout !== "function") {
      logLayoutCoreMissing("applyLayout");
      clearLayoutMeta(nodes);
      layoutPlaceholder(nodes);
      return nodes;
    }
    const normalizedHints = {
      ...layoutHints,
      preferredCoreIds: Array.isArray(layoutHints?.preferredCoreIds) ? layoutHints.preferredCoreIds : [],
      corePlacement: normalizeCorePlacement(layoutHints?.corePlacement),
      corePin: normalizeCorePin(layoutHints?.corePin),
      coreSource: normalizeCoreSource(layoutHints?.coreSource),
    };
    return engine.applyLayout(
      nodes,
      edges,
      mode,
      focusId,
      state.layoutDirection || {},
      normalizedHints
    );
  }

  function shouldForceSyncLayoutFirstPaint(nodes, edges) {
    const sourceType = normalizeFlowSource(state.source || "");
    if (sourceType !== "stats") return false;
    if (!state.graphLoading) return false;
    const nodeCount = Array.isArray(nodes) ? nodes.length : 0;
    const edgeCount = Array.isArray(edges) ? edges.length : 0;
    if (!nodeCount || !edgeCount) return false;
    if (nodeCount > 1600 || edgeCount > 4800) return false;
    return true;
  }

  function applyLayout(nodes, edges, preset, focusId, options = {}) {
    const layoutOptions = options && typeof options === "object" ? options : {};
    const mode = normalizeLayoutPreset(preset);
    const normalizedMode = mode;
    const cacheMode = normalizedMode;
    const hasSemanticProjection = !!normalizeLayoutSemanticProjection(layoutOptions.semanticProjection);
    const disablePlanCache = !!layoutOptions.disablePlanCache;
    const enablePlanCache =
      (cacheMode === "network" || cacheMode === "compact") &&
      !disablePlanCache &&
      !state.layoutTopologyDirty &&
      !hasSemanticProjection;
    const layoutCacheKey = enablePlanCache
      ? layoutPlanCacheController.makeCacheKey(cacheMode, focusId, nodes || [], edges || [])
      : "";
    if (enablePlanCache && !layoutCacheKey) {
      void layoutPlanCacheController.prepareCacheKey(cacheMode, focusId, nodes || [], edges || []);
    }
    if (!layoutCacheKey) {
      state.lastAppliedLayoutCacheKey = "";
    }
    if (enablePlanCache && layoutCacheKey) {
      const cachedPlan = layoutPlanCacheController.readCache(layoutCacheKey);
      if (cachedPlan) {
        const applied = layoutPlanCacheController.applyCacheEntry(cachedPlan, nodes);
        if (applied) {
          cancelPendingLayoutWorker("layout-cache-hit");
          applyLayoutEdgeRouting(nodes, edges, mode);
          state.lastAppliedLayoutCacheKey = String(layoutCacheKey || "");
          return nodes;
        }
      }
    }
    let layoutHints = {};
    try {
      layoutHints = collectLayoutHints(nodes, edges, mode, {
        logSource: "active",
        semanticProjection: layoutOptions.semanticProjection,
      });
    } catch (e) {
      log("WARN", "layout semantic projection missing, using placeholder layout", {
        message: String(e?.message || e),
        preset: mode,
        nodes: nodes?.length || 0,
        edges: edges?.length || 0,
      });
      clearLayoutMeta(nodes);
      layoutPlaceholder(nodes);
      applyLayoutEdgeRouting(nodes, edges, mode);
      return nodes;
    }
    const forceSyncFirstPaint = shouldForceSyncLayoutFirstPaint(nodes, edges);
    if (shouldUseLayoutWorker(nodes, edges) && !forceSyncFirstPaint) {
      applyLayoutEdgeRouting(nodes, edges, mode);
      const summary = summarizeLayout(nodes || []);
      if (isLayoutCollapsed(summary)) {
        layoutPlaceholder(nodes);
      }
      const scheduled = scheduleLayoutWorker(
        nodes,
        edges,
        mode,
        focusId,
        mode === "network"
          ? { ...layoutHints, collectNetworkSectorPlacementPayloads: true }
          : layoutHints,
        enablePlanCache ? layoutCacheKey : ""
      );
      if (scheduled) return nodes;
    }
    if (forceSyncFirstPaint && shouldUseLayoutWorker(nodes, edges) && state.debugTrace) {
      try {
        log("INFO", "layout first paint forced sync", {
          source: state.source || "",
          nodes: nodes?.length || 0,
          edges: edges?.length || 0,
          mode: normalizedMode || "",
        });
      } catch (e) {}
    }
    const laidOut = applyLayoutSync(nodes, edges, mode, focusId, layoutHints);
    if (enablePlanCache && layoutCacheKey) {
      layoutPlanCacheController.rememberCache(layoutCacheKey, mode, laidOut || nodes || []);
      state.lastAppliedLayoutCacheKey = String(layoutCacheKey || "");
    }
    applyLayoutEdgeRouting(nodes, edges, mode);
    if (layoutHints?.__layoutReport && typeof layoutHints.__layoutReport === "object") {
      const layoutReport = consumeNetworkSectorPlacementPayloadReport(layoutHints.__layoutReport, {
        source: "layout-sync",
        key: layoutCacheKey || "",
      });
      updateClusterSlotCacheFromLayoutReport(layoutReport, layoutHints);
      const layoutReportSummary =
        summarizeLayoutReportPayload(layoutReport) || layoutReport;
      const layoutReportDigest =
        digestLayoutReportPayload(layoutReportSummary) || layoutReportSummary;
      try {
        log("INFO", "layout sync report", layoutReportDigest);
      } catch (e) {}
      const semanticState = ensureLayoutSemanticState();
      semanticState.lastReports = {
        ...(semanticState.lastReports || {}),
        sync: layoutReportSummary,
      };
    }
    if (normalizedMode === "compact" || normalizedMode === "relation") {
      scheduleNetworkLayoutPrewarm({ reason: "layout-sync-complete" });
    } else if (normalizedMode === "network") {
      scheduleCompactLayoutPrewarm({ reason: "layout-sync-complete" });
    }
    return laidOut;
  }

  // ===== Txn Drill =====
  function isDirectGraphPayload(data) {
    const graph = data?.graph || data?.graphData || data?.graph_data;
    return !!(graph && Array.isArray(graph.nodes) && Array.isArray(graph.edges));
  }

  function getDirectGraphPayload(data) {
    return data?.graph || data?.graphData || data?.graph_data || { nodes: [], edges: [] };
  }

  function getRequestSnapshotRef(data) {
    return cloneResultSnapshotRef(
      data?.result_snapshot_ref ||
        data?.resultSnapshotRef ||
        data?.source_result_snapshot_ref ||
        data?.sourceResultSnapshotRef
    );
  }

  async function applySnapshotGraphPayload(data, snapshotRef) {
    const targetSnapshotRef = cloneResultSnapshotRef(snapshotRef || getRequestSnapshotRef(data));
    const snapshotId = String(targetSnapshotRef?.snapshot_id || targetSnapshotRef?.graph_hash || "").trim();
    if (
      !snapshotId ||
      !state.caseId ||
      !state.backend ||
      typeof state.backend.getFlowResultSnapshot !== "function"
    ) {
      return false;
    }
    const raw = await backendCall(
      "getFlowResultSnapshot",
      JSON.stringify({
        caseId: state.caseId,
        snapshotId,
      })
    );
    const res = parseJSONSafe(raw, {});
    if (!res?.ok) {
      toast(res?.error || "图谱快照加载失败", "danger");
      return false;
    }
    const runtimeGraph = normalizeSnapshotGraphPayload(
      res.runtime_graph || {
        nodes: Array.isArray(res.nodes) ? res.nodes : [],
        edges: Array.isArray(res.edges) ? res.edges : [],
      }
    );
    const resolvedSnapshotRef = cloneResultSnapshotRef(
      res.result_snapshot_ref || res.snapshot_ref || targetSnapshotRef
    );
    const requestedLayout = String(data?.layoutPreset || data?.layout || "flow").trim() || "flow";
    state.graphMode = /relation|关联/i.test(String(data?.view || data?.mode || "")) ? "relation" : "net";
    state.layoutPreset = requestedLayout;
    setLayoutPreset(requestedLayout, { animate: false, markSelected: true });
    const layoutSemanticProjection = await prepareLayoutSemanticProjection(
      runtimeGraph.nodes,
      runtimeGraph.edges,
      requestedLayout,
      { reason: "snapshot-graph-payload" }
    );
    await renderDirectGraph(runtimeGraph.nodes, runtimeGraph.edges, {
      title: data?.title || "",
      toastLabel: "图谱快照已加载",
      createView: true,
      responsePayload: {
        result_snapshot_ref: resolvedSnapshotRef,
        projection: res.projection || data?.projection || null,
        render_hints: res.render_hints || data?.render_hints || data?.renderHints || null,
        graph_tier: data?.graph_tier || data?.graphTier || "",
      },
      layoutSemanticProjection,
    });
    return true;
  }

  function ensureLayoutAfterRender(graph, nodes, edges, reason = "", options = {}) {
    const layoutOptions = options && typeof options === "object" ? options : {};
    if (!graph || !Array.isArray(nodes) || nodes.length < 2) return false;
    const summary = summarizeLayout(nodes);
    const spanX = summary.maxX != null && summary.minX != null ? summary.maxX - summary.minX : 0;
    const spanY = summary.maxY != null && summary.minY != null ? summary.maxY - summary.minY : 0;
    const collapsed =
      !summary.finite ||
      summary.finite < nodes.length ||
      (spanX < 1 && spanY < 1 && nodes.length > 1);
    if (!collapsed) return false;
    const focus = resolveFocusIdFromNodes(nodes, state.focusId, state.focusName);
    applyLayout(nodes, edges, state.layoutPreset || "flow", focus, {
      semanticProjection: layoutOptions.semanticProjection,
    });
    try {
      graph.data({ nodes, edges });
      graph.render();
      graph.getEdges().forEach((e) => graph.refreshItem(e));
    } catch (e) {}
    try {
      log("WARN", "layout collapsed after render, rerendered", {
        reason,
        layoutPreset: state.layoutPreset || "",
        focusId: focus || "",
        layout: summarizeLayout(nodes || []),
        edgeOffsets: summarizeEdgeOffsets(edges),
      });
    } catch (e) {}
    return true;
  }

  function getGraphSizeInfo(graph) {
    const containerEl = graph?.get?.("container");
    const rect = (el) => {
      if (!el || !el.getBoundingClientRect) return null;
      const r = el.getBoundingClientRect();
      let display = "";
      let visibility = "";
      let opacity = "";
      try {
        const style = window.getComputedStyle(el);
        display = style.display;
        visibility = style.visibility;
        opacity = style.opacity;
      } catch (e) {}
      return {
        w: Math.round(r.width),
        h: Math.round(r.height),
        clientW: el.clientWidth || 0,
        clientH: el.clientHeight || 0,
        display,
        visibility,
        opacity,
        visible: !!el.offsetParent,
      };
    };
    return {
      window: { w: window.innerWidth, h: window.innerHeight },
      graph: {
        w: graph?.get?.("width") ?? null,
        h: graph?.get?.("height") ?? null,
        zoom: graph?.getZoom?.() ?? null,
      },
      app: rect($(".app")),
      right: rect($(".right")),
      card: rect($("#graphCard")),
      wrap: rect($("#graphWrap")),
      container: rect(containerEl),
    };
  }

  function centerGraphOnLayout(graph, nodes, zoom) {
    if (!graph || !Array.isArray(nodes) || !nodes.length) return false;
    const summary = summarizeLayout(nodes || []);
    if (!summary.finite || summary.minX == null || summary.maxX == null || summary.minY == null || summary.maxY == null) {
      return false;
    }
    const center = {
      x: (summary.minX + summary.maxX) / 2,
      y: (summary.minY + summary.maxY) / 2,
    };
    const z = Number.isFinite(zoom) ? zoom : graph.getZoom?.();
    return applyGraphViewport({ zoom: Number.isFinite(z) ? z : undefined, center });
  }

  function centerNetworkRuntimeSkeletonAfterExpand(graph, reason = "") {
    if (state.layoutPreset !== "network") return false;
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (!isLargeNetworkTopologyMode(mode)) return false;
    const nodes = collectNetworkRuntimeNodeModels(graph);
    if (nodes.length < (mode === "xlarge" ? 40 : 24) || nodes.length > 360) return false;
    const summary = summarizeLayout(nodes);
    const width = Math.max(1, Number(graph?.get?.("width")) || 0);
    const height = Math.max(1, Number(graph?.get?.("height")) || 0);
    const spanX = summary?.finite && summary.minX != null && summary.maxX != null ? Math.max(1, summary.maxX - summary.minX) : 0;
    const spanY = summary?.finite && summary.minY != null && summary.maxY != null ? Math.max(1, summary.maxY - summary.minY) : 0;
    const fitZoom =
      spanX > 0 && spanY > 0
        ? Math.min((width * 0.78) / spanX, (height * 0.76) / spanY)
        : Number(graph?.getZoom?.());
    const maxZoom = mode === "large" ? 0.92 : 1.02;
    const zoom = Math.max(0.24, Math.min(maxZoom, Number.isFinite(fitZoom) ? fitZoom : 0.56));
    const applied = centerGraphOnLayout(graph, nodes, zoom);
    if (applied) {
      recordNetworkRuntimePerfEvent("projection-expand-runtime-centered", {
        reason: String(reason || ""),
        mode,
        nodes: nodes.length,
        spanX,
        spanY,
        zoom,
      });
    }
    return applied;
  }

  function spreadNetworkRuntimeCollapsedCommunities(graph, reason = "") {
    if (state.layoutPreset !== "network") return false;
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (mode !== "xlarge") return false;
    const items = Array.isArray(graph?.getNodes?.()) ? graph.getNodes() : [];
    const minSpreadNodes = 56;
    const maxSpreadNodes = 72;
    if (items.length < minSpreadNodes || items.length > maxSpreadNodes) return false;
    const rows = [];
    items.forEach((item, index) => {
      const model = item?.getModel?.() || {};
      const id = String(model?.id || "").trim();
      const x = Number(model.x);
      const y = Number(model.y);
      if (!id || !Number.isFinite(x) || !Number.isFinite(y)) return;
      rows.push({
        item,
        model,
        id,
        index,
        x,
        y,
        community: String(model.layoutCommunity || model.layoutSuperNodeId || model.clusterId || model.layoutClusterId || "").trim(),
        score: rankNetworkCoarseNode(model),
      });
    });
    if (rows.length < minSpreadNodes) return false;
    const summary = summarizeLayout(rows);
    if (!summary?.finite || summary.minX == null || summary.maxX == null || summary.minY == null || summary.maxY == null) return false;
    const center = {
      x: (summary.minX + summary.maxX) / 2,
      y: (summary.minY + summary.maxY) / 2,
    };
    const groups = new Map();
    rows.forEach((row) => {
      const community = row.community || `node:${row.id}`;
      const group = groups.get(community) || { id: community, rows: [], x: 0, y: 0, score: 0 };
      group.rows.push(row);
      group.x += row.x;
      group.y += row.y;
      group.score += row.score;
      groups.set(community, group);
    });
    const groupRows = Array.from(groups.values())
      .map((group) => {
        group.x /= Math.max(1, group.rows.length);
        group.y /= Math.max(1, group.rows.length);
        return group;
      })
      .sort((left, right) => right.rows.length - left.rows.length || right.score - left.score || left.id.localeCompare(right.id, "zh-CN"));
    if (groupRows.length <= 1) return false;
    const outerRadius = Math.max(520, Math.min(1800, Math.sqrt(rows.length) * 96));
    const golden = Math.PI * (3 - Math.sqrt(5));
    const horizontalSpread = 1.42;
    const dataNodeById = new Map(
      (Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [])
        .map((node) => [String(node?.id || "").trim(), node])
        .filter(([id]) => !!id)
    );
    let moved = 0;
    const startedAt = networkPerfNow();
    graph.setAutoPaint?.(false);
    try {
      groupRows.forEach((group, groupIndex) => {
        const groupCount = groupRows.length;
        const groupAngle =
          groupCount > 1
            ? groupIndex * golden + Math.PI * 2 * (hashProjectionPointSeed(`network-runtime-group|${group.id}`) / 0xffffffff)
            : Math.PI * 2 * (hashProjectionPointSeed(`network-runtime-single-group|${group.id}`) / 0xffffffff);
        const groupRadius = groupCount > 1 ? outerRadius * Math.sqrt((groupIndex + 0.5) / groupCount) : 0;
        const anchor = {
          x: center.x + Math.cos(groupAngle) * groupRadius * horizontalSpread,
          y: center.y + Math.sin(groupAngle) * groupRadius * 0.96,
        };
        const ordered = group.rows
          .slice()
          .sort((left, right) => right.score - left.score || left.id.localeCompare(right.id, "zh-CN") || left.index - right.index);
        const innerBase = Math.max(150, Math.min(680, Math.sqrt(ordered.length) * 72));
        ordered.forEach((row, rowIndex) => {
          const localAngle =
            rowIndex * golden + Math.PI * 2 * (hashProjectionPointSeed(`network-runtime-spread|${group.id}|${row.id}`) / 0xffffffff);
          const localRadius = ordered.length > 1 ? Math.sqrt(rowIndex + 0.35) * (innerBase / Math.sqrt(ordered.length)) : 0;
          const targetX = anchor.x + Math.cos(localAngle) * localRadius * horizontalSpread;
          const targetY = anchor.y + Math.sin(localAngle) * localRadius * 0.92;
          const nextX = row.x * 0.1 + targetX * 0.9;
          const nextY = row.y * 0.1 + targetY * 0.9;
          row.model.x = nextX;
          row.model.y = nextY;
          const dataNode = dataNodeById.get(row.id);
          if (dataNode) {
            dataNode.x = nextX;
            dataNode.y = nextY;
          }
          moved += 1;
        });
      });
      invalidateNetworkRuntimeEdgeGeometry(graph, `${reason || "network-runtime-spread"}:node-position-update`);
      graph.refreshPositions?.();
    } finally {
      graph.setAutoPaint?.(true);
      graph.paint?.();
    }
    recordNetworkRuntimePerfEvent("projection-expand-runtime-spread", {
      reason: String(reason || ""),
      mode,
      nodes: rows.length,
      communities: groupRows.length,
      moved,
      durationMs: roundNetworkPerfMs(networkPerfNow() - startedAt),
    });
    return moved > 0;
  }

  function stabilizeNetworkRuntimeForSuppressedTopologyPlan(reason = "") {
    return false;
    if (state.layoutPreset !== "network") return false;
    const suppressUntil = Number(state.networkLayout?.suppressTopologyPlanUntil || 0);
    if (!suppressUntil || Date.now() >= suppressUntil) return false;
    const mode = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    if (mode !== "xlarge") return false;
    const graph = ensureGraph();
    if (!graph) return false;
    const spread = spreadNetworkRuntimeCollapsedCommunities(graph, `${reason || "topology-suppressed"}:runtime-stabilize`);
    const centered = centerNetworkRuntimeSkeletonAfterExpand(graph, `${reason || "topology-suppressed"}:runtime-stabilize`);
    if (spread || centered) {
      suppressDenseNetworkRuntimeEdgeLabels(graph, {
        mode,
        nodeCount: graph.getNodes?.().length || 0,
        edgeCount: graph.getEdges?.().length || 0,
      });
      suppressNetworkRuntimeOverlappingLabels(graph, {
        maxLabels: 8,
        padding: 14,
      });
      updateMinimap();
    }
    return spread || centered;
  }

  function graphNodeMapById(nodes = []) {
    const out = new Map();
    (Array.isArray(nodes) ? nodes : []).forEach((node) => {
      const id = String(node?.id || "").trim();
      if (!id) return;
      out.set(id, node);
    });
    return out;
  }

  function graphEdgeMapById(edges = []) {
    const out = new Map();
    (Array.isArray(edges) ? edges : []).forEach((edge) => {
      const id = String(edge?.id || "").trim();
      if (!id) return;
      out.set(id, edge);
    });
    return out;
  }

  function collectProjectionLayoutSeedBaseNodes(graph, baseNodes = []) {
    const out = [];
    const seen = new Set();
    const remember = (node) => {
      const id = String(node?.id || "").trim();
      const x = Number(node?.x);
      const y = Number(node?.y);
      if (!id || !Number.isFinite(x) || !Number.isFinite(y) || seen.has(id)) return;
      seen.add(id);
      out.push({ ...(node || {}), x, y });
    };
    (graph?.getNodes?.() || []).forEach((item) => remember(item?.getModel?.() || null));
    (Array.isArray(baseNodes) ? baseNodes : []).forEach(remember);
    return out;
  }

  function snapshotGraphDataFromRuntime(graph, metadata = {}) {
    if (!graph) return buildGraphSnapshotData({ nodes: [], edges: [] }, metadata);
    if (typeof graph.save === "function") {
      try {
        const saved = graph.save();
        if (saved && Array.isArray(saved.nodes)) return buildGraphSnapshotData(saved, metadata);
      } catch (e) {}
    }
    return buildGraphSnapshotData(
      {
        nodes: (graph.getNodes?.() || []).map((item) => ({ ...(item?.getModel?.() || {}) })),
        edges: (graph.getEdges?.() || []).map((item) => ({ ...(item?.getModel?.() || {}) })),
      },
      metadata
    );
  }

  function buildBoundedProjectionRuntimePatch(graph, nextNodes = [], nextEdges = [], options = {}) {
    const nodeLimit = Math.max(0, Math.floor(Number(options?.maxUpsertNodes) || 0));
    if (!graph || nodeLimit <= 0) return null;
    const edgeLimit = Math.max(0, Math.floor(Number(options?.maxUpsertEdges) || 0));
    const currentNodeIds = new Set(
      (graph.getNodes?.() || [])
        .map((item) => String(item?.getModel?.()?.id || "").trim())
        .filter(Boolean)
    );
    const nodeById = graphNodeMapById(nextNodes);
    const priorityIds = parseUniqueList(options?.priorityNodeIds || [], { allowString: true });
    const selectedIds = [];
    const selectedSet = new Set();
    const remember = (nodeId) => {
      const id = String(nodeId || "").trim();
      if (!id || currentNodeIds.has(id) || selectedSet.has(id) || !nodeById.has(id) || selectedIds.length >= nodeLimit) {
        return false;
      }
      selectedSet.add(id);
      selectedIds.push(id);
      return true;
    };
    priorityIds.forEach(remember);
    const edgeRows = (Array.isArray(nextEdges) ? nextEdges : [])
      .map((edge, index) => {
        const source = networkPlanEndpoint(edge?.source);
        const target = networkPlanEndpoint(edge?.target);
        return {
          edge,
          index,
          id: String(edge?.id || "").trim(),
          source,
          target,
          weight:
            Math.abs(Number(edge?.weight) || 0) ||
            Math.abs(Number(edge?.amount) || Number(edge?.total_amount) || Number(edge?.totalAmount) || 0) ||
            Math.abs(Number(edge?.count) || 0),
        };
      })
      .filter((row) => row.source && row.target);
    edgeRows
      .filter((row) => currentNodeIds.has(row.source) || currentNodeIds.has(row.target) || selectedSet.has(row.source) || selectedSet.has(row.target))
      .sort((left, right) => right.weight - left.weight || left.index - right.index)
      .some((row) => {
        if (selectedIds.length >= nodeLimit) return true;
        if (currentNodeIds.has(row.source) || selectedSet.has(row.source)) remember(row.target);
        if (selectedIds.length >= nodeLimit) return true;
        if (currentNodeIds.has(row.target) || selectedSet.has(row.target)) remember(row.source);
        return false;
      });
    if (!selectedIds.length) {
      for (const node of Array.isArray(nextNodes) ? nextNodes : []) {
        remember(node?.id);
        if (selectedIds.length >= nodeLimit) break;
      }
    }
    if (!selectedIds.length) return null;
    const allowedNodeIds = new Set([...currentNodeIds, ...selectedSet]);
    const upsertEdges = [];
    for (const row of edgeRows) {
      if (edgeLimit > 0 && upsertEdges.length >= edgeLimit) break;
      if (!allowedNodeIds.has(row.source) || !allowedNodeIds.has(row.target)) continue;
      if (!selectedSet.has(row.source) && !selectedSet.has(row.target)) continue;
      if (row.id && graph.findById?.(row.id)) continue;
      upsertEdges.push(row.edge);
    }
    return {
      remove_node_ids: [],
      upsert_nodes: selectedIds.map((nodeId) => nodeById.get(nodeId)).filter(Boolean),
      remove_edge_ids: [],
      upsert_edges: upsertEdges,
      summary: {
        bounded_client_patch: true,
        selected_nodes: selectedIds.length,
        selected_edges: upsertEdges.length,
        source_nodes: Array.isArray(nextNodes) ? nextNodes.length : 0,
        source_edges: Array.isArray(nextEdges) ? nextEdges.length : 0,
      },
    };
  }

  function applyIncrementalProjectionPatch(
    nextNodes = [],
    nextEdges = [],
    patchPayload = {},
    {
      reason = "",
      preserveViewport = true,
      graphTier = "",
      renderHints = null,
      projection = null,
      resultSnapshotRef = null,
      maxUpsertNodes = 0,
      maxUpsertEdges = 0,
      priorityNodeIds = [],
    } = {}
  ) {
    const graph = ensureGraph();
    if (!graph) return false;
    const patchKind = String(patchPayload?.patch_kind || "").trim().toLowerCase();
    const upsertNodeLimit = Math.max(0, Math.floor(Number(maxUpsertNodes) || 0));
    const upsertEdgeLimit = Math.max(0, Math.floor(Number(maxUpsertEdges) || 0));
    let runtimePatch =
      patchPayload?.runtime_graph_patch && typeof patchPayload.runtime_graph_patch === "object"
        ? patchPayload.runtime_graph_patch
        : null;
    if ((patchKind !== "delta" || !runtimePatch) && upsertNodeLimit > 0) {
      runtimePatch = buildBoundedProjectionRuntimePatch(graph, nextNodes, nextEdges, {
        maxUpsertNodes: upsertNodeLimit,
        maxUpsertEdges: upsertEdgeLimit,
        priorityNodeIds,
      });
    }
    if (!runtimePatch || (patchKind !== "delta" && upsertNodeLimit <= 0)) return false;
    const removeNodeIds = new Set(
      (Array.isArray(runtimePatch.remove_node_ids) ? runtimePatch.remove_node_ids : [])
        .map((id) => String(id || "").trim())
        .filter(Boolean)
    );
    const removeEdgeIds = new Set(
      (Array.isArray(runtimePatch.remove_edge_ids) ? runtimePatch.remove_edge_ids : [])
        .map((id) => String(id || "").trim())
        .filter(Boolean)
    );
    const nextNodeMap = graphNodeMapById(nextNodes);
    const rawUpsertNodeRows = Array.isArray(runtimePatch.upsert_nodes) ? runtimePatch.upsert_nodes : [];
    const priorityNodeSet = new Set(parseUniqueList(priorityNodeIds, { allowString: true }));
    const rawUpsertNodeIds = new Set(
      rawUpsertNodeRows
        .map((row) => String(row?.id || "").trim())
        .filter(Boolean)
    );
    const priorityBackfillNodeRows = [];
    priorityNodeSet.forEach((id) => {
      if (!id || rawUpsertNodeIds.has(id) || graph.findById?.(id) || !nextNodeMap.has(id)) return;
      priorityBackfillNodeRows.push(nextNodeMap.get(id));
      rawUpsertNodeIds.add(id);
    });
    let upsertNodeRows = priorityBackfillNodeRows.length
      ? [...priorityBackfillNodeRows, ...rawUpsertNodeRows]
      : rawUpsertNodeRows;
    if (upsertNodeLimit > 0 && upsertNodeRows.length > upsertNodeLimit) {
      const priorityRows = [];
      const regularRows = [];
      upsertNodeRows.forEach((row) => {
        const id = String(row?.id || "").trim();
        if (id && priorityNodeSet.has(id)) priorityRows.push(row);
        else regularRows.push(row);
      });
      upsertNodeRows = [...priorityRows, ...regularRows].slice(0, upsertNodeLimit);
    }
	    let upsertNodeIds = new Set(
	      upsertNodeRows
	        .map((row) => String(row?.id || "").trim())
	        .filter(Boolean)
	    );
	    const currentNodeIds = new Set(
	      (graph.getNodes?.() || [])
	        .map((item) => String(item?.getModel?.()?.id || "").trim())
	        .filter(Boolean)
	    );
	    const buildRuntimeNodeIds = () => {
	      const ids = new Set(currentNodeIds);
	      removeNodeIds.forEach((nodeId) => ids.delete(nodeId));
	      upsertNodeIds.forEach((nodeId) => ids.add(nodeId));
	      return ids;
	    };
	    let runtimeNodeIds = buildRuntimeNodeIds();
    const rawUpsertEdgeRows = Array.isArray(runtimePatch.upsert_edges) ? runtimePatch.upsert_edges : [];
    const rawUpsertEdgeIds = new Set(
      rawUpsertEdgeRows
        .map((row) => String(row?.id || "").trim())
        .filter(Boolean)
    );
    const priorityBackfillNodeIds = new Set(
      priorityBackfillNodeRows
        .map((row) => String(row?.id || "").trim())
        .filter(Boolean)
    );
    const upsertEdgeRowsBase = rawUpsertEdgeRows.filter((row) => {
      const source = networkPlanEndpoint(row?.source);
      const target = networkPlanEndpoint(row?.target);
      return source && target && runtimeNodeIds.has(source) && runtimeNodeIds.has(target);
    });
    const priorityBackfillEdgeRows = priorityBackfillNodeIds.size
      ? (Array.isArray(nextEdges) ? nextEdges : []).filter((row) => {
          const edgeId = String(row?.id || "").trim();
          if (edgeId && (rawUpsertEdgeIds.has(edgeId) || graph.findById?.(edgeId))) return false;
          const source = networkPlanEndpoint(row?.source);
          const target = networkPlanEndpoint(row?.target);
          return (
            source &&
            target &&
            runtimeNodeIds.has(source) &&
            runtimeNodeIds.has(target) &&
            (priorityBackfillNodeIds.has(source) || priorityBackfillNodeIds.has(target))
          );
        })
      : [];
	    let upsertEdgeRows = priorityBackfillEdgeRows.length
	      ? [...priorityBackfillEdgeRows, ...upsertEdgeRowsBase]
	      : upsertEdgeRowsBase;
	    if (upsertEdgeLimit > 0 && upsertEdgeRows.length > upsertEdgeLimit) {
	      upsertEdgeRows = upsertEdgeRows.slice(0, upsertEdgeLimit);
	    }
	    const requireNetworkPatchConnectivity =
	      state.layoutPreset === "network" &&
	      upsertNodeLimit > 0 &&
	      isLargeNetworkTopologyMode(state.networkLayout?.lastPlanMode || graphTier);
	    let skippedDisconnectedNetworkNodes = 0;
	    if (requireNetworkPatchConnectivity && upsertNodeRows.length) {
	      const incidentUpsertNodeIds = new Set();
	      upsertEdgeRows.forEach((row) => {
	        const source = networkPlanEndpoint(row?.source);
	        const target = networkPlanEndpoint(row?.target);
	        if (source && upsertNodeIds.has(source)) incidentUpsertNodeIds.add(source);
	        if (target && upsertNodeIds.has(target)) incidentUpsertNodeIds.add(target);
	      });
	      const filteredUpsertNodeRows = upsertNodeRows.filter((row) => {
	        const id = String(row?.id || "").trim();
	        return id && (priorityNodeSet.has(id) || incidentUpsertNodeIds.has(id));
	      });
	      skippedDisconnectedNetworkNodes = upsertNodeRows.length - filteredUpsertNodeRows.length;
	      if (skippedDisconnectedNetworkNodes > 0) {
	        upsertNodeRows = filteredUpsertNodeRows;
	        upsertNodeIds = new Set(
	          upsertNodeRows
	            .map((row) => String(row?.id || "").trim())
	            .filter(Boolean)
	        );
	        runtimeNodeIds = buildRuntimeNodeIds();
	        upsertEdgeRows = upsertEdgeRows.filter((row) => {
	          const source = networkPlanEndpoint(row?.source);
	          const target = networkPlanEndpoint(row?.target);
	          return source && target && runtimeNodeIds.has(source) && runtimeNodeIds.has(target);
	        });
	      }
	    }
	    const upsertEdgeIds = new Set(
	      upsertEdgeRows
	        .map((row) => String(row?.id || "").trim())
	        .filter(Boolean)
	    );
    if (!removeNodeIds.size && !removeEdgeIds.size && !upsertNodeIds.size && !upsertEdgeIds.size) {
      return false;
    }
    const nextEdgeMap = graphEdgeMapById(nextEdges);
    const viewport = preserveViewport ? captureGraphViewport(graph) : null;
    const shouldStoreRuntimeSnapshot = state.layoutPreset === "network" && upsertNodeLimit > 0;
    if (!shouldStoreRuntimeSnapshot) {
      replaceRuntimeGraphData(
        {
          nodes: nextNodes,
          edges: nextEdges,
          graph_tier: graphTier,
          render_hints: renderHints,
          projection,
          result_snapshot_ref: resultSnapshotRef,
        },
        { reason: `projection-expand:${reason || "projection-expand"}-state`, preserveGraphMeta: true }
      );
    }
    const changedEdgeItems = [];
    graph.setAutoPaint?.(false);
    try {
      removeEdgeIds.forEach((edgeId) => {
        const item = graph.findById?.(edgeId);
        if (item) graph.removeItem?.(item);
      });
      removeNodeIds.forEach((nodeId) => {
        const item = graph.findById?.(nodeId);
        if (item) graph.removeItem?.(item);
      });
      upsertNodeRows.forEach((row) => {
        const nodeId = String(row?.id || "").trim();
        const model = nextNodeMap.get(nodeId);
        if (!model) return;
        const item = graph.findById?.(nodeId);
        if (item) {
          graph.updateItem?.(item, model);
        } else {
          graph.addItem?.("node", model);
        }
      });
      upsertEdgeRows.forEach((row) => {
        const edgeId = String(row?.id || "").trim();
        const model = nextEdgeMap.get(edgeId);
        if (!model) return;
        const item = graph.findById?.(edgeId);
        if (item) {
          graph.updateItem?.(item, model);
          changedEdgeItems.push(item);
        } else {
          const created = graph.addItem?.("edge", model);
          const runtimeItem = created || graph.findById?.(edgeId);
          if (runtimeItem) {
            graph.updateItem?.(runtimeItem, model);
            changedEdgeItems.push(runtimeItem);
          }
        }
      });
    } catch (e) {
      try {
        graph.setAutoPaint?.(true);
        graph.paint?.();
      } catch (_error) {}
      return false;
    }
    graph.setAutoPaint?.(true);
    graph.paint?.();
    syncGraphItemPositionsFromData(graph, nextNodes, reason || "projection-expand", { force: true, log });
    if (viewport) {
      applyGraphViewport(viewport, graph);
    }
    if (shouldStoreRuntimeSnapshot) {
      replaceRuntimeGraphData(
        snapshotGraphDataFromRuntime(graph, {
          graphTier,
          renderHints,
          projection,
          resultSnapshotRef,
          runtimeRevision: state.graph?.runtimeRevision,
        }),
        { reason: `projection-expand:${reason || "projection-expand"}-runtime-state`, preserveGraphMeta: true }
      );
    }
	    if (changedEdgeItems.length) {
	      scheduleGraphItemRefreshBatches(graph, changedEdgeItems, {
	        batchSize: Math.min(180, Math.max(48, changedEdgeItems.length)),
	        timeout: 120,
	      });
	    }
	    if (skippedDisconnectedNetworkNodes > 0) {
	      recordNetworkRuntimePerfEvent("projection-expand-disconnected-nodes-skipped", {
	        reason: String(reason || ""),
	        mode: String(state.networkLayout?.lastPlanMode || graphTier || ""),
	        nodes: skippedDisconnectedNetworkNodes,
	      });
	    }
    updateMinimap();
    updateGraphStats();
    updateLayoutButtons();
    syncEdgeDetailButton();
    const view = getActiveView();
    if (view) {
      view.graph = cloneGraphData(state.graph.data);
      view.counts = {
        nodes: Array.isArray(state.graph.data?.nodes) ? state.graph.data.nodes.length : 0,
        edges: Array.isArray(state.graph.data?.edges) ? state.graph.data.edges.length : 0,
      };
      const nextViewport = captureGraphViewport(graph);
      if (nextViewport) view.viewport = nextViewport;
      view.updatedAt = nowIso();
      updateViewCard(view);
      renderViewsBar();
    }
    commitShellStateSync("projection-patch");
    return true;
  }

  async function renderDirectGraph(
    nodes,
    edges,
    {
      title = "",
      toastLabel = "",
      createView = true,
      responsePayload = null,
      preserveViewport = false,
      fitView = true,
      preserveGraphMeta = false,
      suppressNetworkTopologyPlan = false,
      layoutSemanticProjection = null,
      renderPlanApplied = false,
      stagedLabelPlan: precomputedStagedLabelPlan = null,
      graphDataReplaceReason = "render-direct-graph",
    } = {}
  ) {
    resetAnalysisFilterTools({ clearRange: true, clearSource: true, closePopover: true });
    const graph = ensureGraph();
    if (!graph) {
      toast("图谱组件加载失败", "danger");
      return;
    }
    cancelEdgeBatching();

    state.directRenderSeq = (state.directRenderSeq || 0) + 1;
    const renderSeq = state.directRenderSeq;
    const mutationSeq = state.graphMutationSeq || 0;
    const existingViewport = preserveViewport ? captureGraphViewport(graph) : null;
    const sizeReady = await waitForGraphContainerSize(320, 200, 14, 80);
    if (state.directRenderSeq !== renderSeq) return;
    if ((state.graphMutationSeq || 0) !== mutationSeq) return;
    const containerEl = graph.get("container");
    const cw = containerEl?.clientWidth || sizeReady.w || 0;
    const ch = containerEl?.clientHeight || sizeReady.h || 0;
    if (cw > 0 && ch > 0 && graph.changeSize) {
      graph.changeSize(cw, ch);
    }

    const layoutSummaryBefore = summarizeLayout(nodes || []);
    let needsLayout = false;
    try {
      needsLayout =
        (nodes || []).length > 1 &&
        (!layoutSummaryBefore.finite ||
          layoutSummaryBefore.minX == null ||
          layoutSummaryBefore.maxX == null ||
          layoutSummaryBefore.minY == null ||
          layoutSummaryBefore.maxY == null ||
          (layoutSummaryBefore.maxX - layoutSummaryBefore.minX < 1 &&
            layoutSummaryBefore.maxY - layoutSummaryBefore.minY < 1));
      if (needsLayout) {
        const focus = resolveFocusIdFromNodes(nodes, state.focusId, state.focusName);
        applyLayout(nodes, edges, state.layoutPreset || "flow", focus, {
          semanticProjection: layoutSemanticProjection,
        });
      }
    } catch (e) {}
    const graphTier = normalizeGraphTier(responsePayload?.graph_tier || responsePayload?.graphTier || "", nodes.length);
    const renderHints = normalizeGraphRenderHints(
      responsePayload?.render_hints || responsePayload?.renderHints || null,
      nodes.length,
      edges.length,
      state.graphMode || "relation"
    );
    let renderSummary = null;
    if (!renderPlanApplied) {
      try {
        renderSummary = await applyGraphTierRenderPlanWithRust(
          nodes,
          edges,
          { ...state, graphTier, renderHints },
          { preserveEdgeLabels: false, name: "render-direct-graph", includeStagedLabels: true }
        );
      } catch (e) {
        if (state.directRenderSeq !== renderSeq) return;
        if ((state.graphMutationSeq || 0) !== mutationSeq) return;
        log("WARN", "rust direct graph render plan failed", { message: String(e?.message || e), stack: e?.stack || "" });
        toast("图谱展示计划失败", "danger");
        return;
      }
      if (state.directRenderSeq !== renderSeq) return;
      if ((state.graphMutationSeq || 0) !== mutationSeq) return;
    }
    const activeGraphTier = renderSummary?.tier || graphTier;
    const activeRenderHints = renderSummary?.renderHints || renderHints;
    const runtimeGraphDataReason = String(graphDataReplaceReason || "render-direct-graph");
    state.graphTier = activeGraphTier;
    state.renderHints = activeRenderHints;
    const useEdgeBatch = shouldBatchEdges(edges.length, {
      ...state,
      graphTier: activeGraphTier,
      renderHints: activeRenderHints,
      graph: { ...state.graph, data: { nodes, edges } },
    });
    const stagedLabelPlan = renderSummary?.stagedLabelPlan || precomputedStagedLabelPlan || null;
    replaceRuntimeGraphData(
      {
        nodes,
        edges,
        graph_tier: activeGraphTier,
        render_hints: activeRenderHints,
        projection: cloneGraphProjection(responsePayload?.projection),
        result_snapshot_ref: cloneResultSnapshotRef(responsePayload?.result_snapshot_ref || responsePayload?.resultSnapshotRef),
        flow_summary: cloneFlowSummaryBoundary(responsePayload?.flow_summary || responsePayload?.flowSummary),
      },
      { reason: `${runtimeGraphDataReason}-input`, preserveGraphMeta }
    );
    try {
      log("INFO", "direct graph render start", {
        nodes: nodes?.length || 0,
        edges: edges?.length || 0,
        perfMode: isPerfMode(),
        edgeBatch: useEdgeBatch,
        renderer: graph.get?.("renderer") || "",
        title: title || "",
        createView,
        layoutPreset: state.layoutPreset || "flow",
        needsLayout,
        layoutBefore: layoutSummaryBefore,
        edgeOffsets: summarizeEdgeOffsets(edges),
        sizeReady,
      });
    } catch (e) {}

    const isRenderCancelled = () =>
      state.directRenderSeq !== renderSeq || (state.graphMutationSeq || 0) !== mutationSeq;
    const smallGraphIntro = !useEdgeBatch && nodes.length > 1 && nodes.length <= 20;
    const suppressStagedLabelsForNetworkSkeleton = false;
    const useLightIntro = !!stagedLabelPlan && !suppressStagedLabelsForNetworkSkeleton && nodes.length > 1;
    const introAnimate = !useLightIntro && !useEdgeBatch && nodes.length > 1 && nodes.length <= 600;
    const animateDuration = smallGraphIntro ? 860 : 640;
    const introEasing = smallGraphIntro ? "easeOutCubic" : "easeInOutCubic";
    const lightIntroDuration = Number(stagedLabelPlan?.budget?.introDuration) || 220;
    const startStagedLabelReveal = () => {
      if (!stagedLabelPlan || suppressStagedLabelsForNetworkSkeleton) return;
      scheduleStagedNodeLabelReveal(stagedLabelPlan, {
        graph,
        isCancelled: isRenderCancelled,
        onComplete: ({ cancelled = false } = {}) => {
          if (cancelled || isRenderCancelled()) return;
          const revealedSnapshot = snapshotGraphData();
          replaceRuntimeGraphData(revealedSnapshot, {
            reason: `${runtimeGraphDataReason}-staged-labels`,
            preserveGraphMeta,
          });
          syncAnalysisSourceDataFromCurrent({ force: true });
          updateCurrentViewSnapshot();
        },
      });
    };
    const prevOpacity = containerEl?.style.opacity || "";
    try {
      graph.clear();
    } catch (e) {}
    let relayout = false;
    let centered = false;
    try {
      if (useEdgeBatch) {
        graph.data({ nodes, edges: [] });
        graph.render();
        if (graph.paint) graph.paint();
        ensureGraphDataVisible(graph, nodes, [], "direct-batch");
        if (graph.refreshPositions) graph.refreshPositions();
        if (graph.paint) graph.paint();
        syncGraphItemPositionsFromData(graph, nodes, "direct-batch-render", { force: true, log });
        updateMinimap();
        renderEdgesInBatches(graph, edges, {
          onComplete: () => {
            if (state.edgeDetail) applyEdgeDetailLabels(true);
            updateMinimap();
            if (fitView) fitGraph({ force: true });
            emitCanvasSnapshotLog("direct-graph-edge-batch-done", graph);
          },
        });
      } else {
        graph.data({ nodes, edges });
        graph.render();
        if (graph.paint) graph.paint();
        ensureGraphDataVisible(graph, nodes, edges, "direct-render");
        graph.getEdges().forEach((e) => graph.refreshItem(e));
        emitCanvasSnapshotLog("direct-graph-rendered", graph);
      }
      if (existingViewport) {
        applyGraphViewport(existingViewport, graph);
      }
      relayout =
        !useEdgeBatch &&
        ensureLayoutAfterRender(graph, nodes, edges, "direct", {
          semanticProjection: layoutSemanticProjection,
        });
      centered = preserveViewport ? false : centerGraphOnLayout(graph, nodes);
      updateMinimap();
      if (useLightIntro) {
        state.introAnimating = true;
        state.introSuppressUntil = Date.now() + lightIntroDuration + 220;
        playLightGraphIntro(containerEl, {
          duration: lightIntroDuration,
          isCancelled: isRenderCancelled,
          onComplete: ({ cancelled = false } = {}) => {
            if (cancelled || isRenderCancelled()) return;
            state.introAnimating = false;
            state.introSuppressUntil = 0;
            finalizeView({ fit: fitView });
            startStagedLabelReveal();
          },
        });
      } else if (introAnimate) {
        state.introAnimating = true;
        state.introSuppressUntil = Date.now() + animateDuration + (smallGraphIntro ? 260 : 200);
        const summary = summarizeLayout(nodes);
        const collapseCenter = {
          x: summary.minX != null && summary.maxX != null ? (summary.minX + summary.maxX) / 2 : 0,
          y: summary.minY != null && summary.maxY != null ? (summary.minY + summary.maxY) / 2 : 0,
        };
        const layoutTargets = new Map(nodes.map((n) => [n.id, { x: n.x, y: n.y }]));
        const jitterScale = smallGraphIntro ? 0.84 : 1.2;
        if (containerEl) containerEl.style.opacity = "0";
        graph.setAutoPaint(false);
        graph.getNodes().forEach((node, idx) => {
          const model = node.getModel() || {};
          const target = layoutTargets.get(model.id);
          if (!target) return;
          const jitter = ((idx % 9) - 4) * jitterScale;
          model.x = collapseCenter.x + jitter;
          model.y = collapseCenter.y - jitter;
        });
        graph.refreshPositions();
        graph.setAutoPaint(true);
        graph.paint();
        setTimeout(() => {
          if (isRenderCancelled()) return;
          if (containerEl) containerEl.style.opacity = prevOpacity || "1";
          animateNodesToTargets(graph, layoutTargets, {
            duration: animateDuration,
            easing: introEasing,
            finalize: false,
            fit: false,
            onComplete: () => {
              if (isRenderCancelled()) return;
              syncGraphNodePositions(graph);
              const relayoutAfter = ensureLayoutAfterRender(graph, nodes, edges, "direct-anim-final", {
                semanticProjection: layoutSemanticProjection,
              });
              if (!preserveViewport && (relayoutAfter || smallGraphIntro)) centerGraphOnLayout(graph, nodes);
              updateCurrentViewSnapshot();
              finalizeView({ fit: fitView && smallGraphIntro });
              startStagedLabelReveal();
              log("INFO", "direct graph animate done", {
                nodes: nodes.length,
                edges: edges.length,
                duration: animateDuration,
                easing: introEasing,
                smallGraph: smallGraphIntro,
                centered,
                relayoutAfter,
              });
            },
          });
        }, 40);
      } else {
        state.introAnimating = false;
        state.introSuppressUntil = 0;
        if (containerEl) containerEl.style.opacity = prevOpacity || "1";
        startStagedLabelReveal();
      }
      try {
        log("INFO", "direct graph animate", {
          nodes: nodes.length,
          edges: edges.length,
          useLightIntro,
          introAnimate,
          duration: animateDuration,
          easing: introEasing,
          smallGraph: smallGraphIntro,
          relayout,
          centered,
        });
      } catch (e) {}
    } catch (e) {
      try {
        log("WARN", "direct graph render failed", {
          message: String(e?.message || e),
          stack: e?.stack || "",
        });
      } catch (e2) {}
      try {
        graph.changeData({ nodes, edges });
      } catch (e2) {}
    }

    applyStyleToGraph(state.graphStyle, { syncView: false });
    if (state.edgeDetail) applyEdgeDetailLabels(true);
    syncEdgeDetailButton();

    const snapshot = introAnimate || useLightIntro ? cloneGraphData(state.graph.data) : snapshotGraphData();
    state.initialStyleSnapshot = buildInitialStyleSnapshot(snapshot);
    state.undoStack = [];
    syncUndoButtons();
    replaceRuntimeGraphData(snapshot, { reason: `${runtimeGraphDataReason}-snapshot`, preserveGraphMeta });
    syncAnalysisSourceDataFromCurrent({ force: true });
    publishGraphPatch({
      scope: "build",
      source: "local",
      reason: "render-direct-graph",
      forceFull: true,
    });
    if (createView || !getActiveView()) {
      const view = buildViewFromSnapshot(snapshot, { title: title || nextViewTitle() });
      addView(view, { activate: true, applyGraph: false });
    } else {
      updateCurrentViewSnapshot();
    }
    updateGraphStats();
    updateLayoutButtons();
    if (state.layoutPreset === "network" && !suppressNetworkTopologyPlan) {
      void scheduleNetworkTopologyPlan({
        reason: "render-direct-graph",
        semanticProjection: layoutSemanticProjection,
        commandSeq: Number(state.layoutPresetCommandSeq || 0),
      });
    }
    scheduleProjectionLayoutIndexSync("render-direct-graph", { delayMs: 120 });
    if (!introAnimate && !useLightIntro) finalizeView({ fit: fitView });
    if (toastLabel) toast(toastLabel, "ok");
    let relayoutTriggered = false;
    let forcedRelayout = false;
    let layoutAfterSummary = summarizeLayout(nodes || []);
    if (!introAnimate && !useLightIntro) {
      relayoutTriggered = ensureLayoutAfterRender(graph, nodes, edges, "direct-final", {
        semanticProjection: layoutSemanticProjection,
      });
      try {
        const spanX =
          layoutAfterSummary.maxX != null && layoutAfterSummary.minX != null
            ? layoutAfterSummary.maxX - layoutAfterSummary.minX
            : 0;
        const spanY =
          layoutAfterSummary.maxY != null && layoutAfterSummary.minY != null
            ? layoutAfterSummary.maxY - layoutAfterSummary.minY
            : 0;
        const collapsed =
          (nodes || []).length > 1 &&
          (!layoutAfterSummary.finite ||
            layoutAfterSummary.finite < (nodes || []).length ||
            (spanX < 1 && spanY < 1));
        if (collapsed) {
          log("WARN", "layout collapsed in snapshot, force relayout", {
            layoutPreset: state.layoutPreset || "",
            layout: layoutAfterSummary,
            edgeOffsets: summarizeEdgeOffsets(state.graph.data?.edges || edges || []),
          });
          applyLayout(nodes, edges, state.layoutPreset || "flow", resolveFocusIdFromNodes(nodes, state.focusId, state.focusName), {
            semanticProjection: layoutSemanticProjection,
          });
          graph.data({ nodes, edges });
          graph.render();
          graph.getEdges().forEach((e) => graph.refreshItem(e));
          if (!preserveViewport) centerGraphOnLayout(graph, nodes);
          forcedRelayout = true;
          updateCurrentViewSnapshot();
          layoutAfterSummary = summarizeLayout(state.graph.data?.nodes || []);
        }
      } catch (e) {}
      if (relayoutTriggered) {
        applyStyleToGraph(state.graphStyle, { syncView: false });
        if (state.edgeDetail) applyEdgeDetailLabels(true);
        syncEdgeDetailButton();
        updateGraphStats();
        updateCurrentViewSnapshot();
        scheduleProjectionLayoutIndexSync("render-direct-graph-relayout", { delayMs: 80 });
        finalizeView({ fit: fitView });
      }
    }
    commitShellStateSync("render-direct-graph");
    try {
      const view = getActiveView();
      log("INFO", "direct graph render done", {
        nodes: nodes?.length || 0,
        edges: edges?.length || 0,
        graphNodes: graph.getNodes?.().length || 0,
        graphEdges: graph.getEdges?.().length || 0,
        layoutAfter: layoutAfterSummary,
        edgeOffsets: summarizeEdgeOffsets(edges),
        relayoutTriggered,
        forcedRelayout,
        view: view ? { id: view.id, title: view.title, counts: viewCounts(view) } : null,
        graphMode: state.graphMode,
        layoutPreset: state.layoutPreset || "",
        layoutSelected: !!state.layoutSelected,
        edgeDetail: !!state.edgeDetail,
      });
    } catch (e) {}

    if (!sizeReady.ready) {
      setTimeout(async () => {
        if (state.directRenderSeq !== renderSeq) return;
        if ((state.graphMutationSeq || 0) !== mutationSeq) return;
        const ready2 = await waitForGraphContainerSize(320, 200, 10, 80);
        if (state.directRenderSeq !== renderSeq) return;
        if ((state.graphMutationSeq || 0) !== mutationSeq) return;
        if (!ready2.ready) {
          log("WARN", "direct graph rerender skipped (container not ready)", {
            nodes: nodes?.length || 0,
            edges: edges?.length || 0,
            sizeReady: ready2,
          });
          return;
        }
        const g = ensureGraph();
        if (!g) return;
        if (g.changeSize) g.changeSize(ready2.w, ready2.h);
        const focus = resolveFocusIdFromNodes(nodes, state.focusId, state.focusName);
        applyLayout(nodes, edges, state.layoutPreset || "flow", focus, {
          semanticProjection: layoutSemanticProjection,
        });
        replaceRuntimeGraphData({ nodes, edges }, { reason: `${runtimeGraphDataReason}-rerender`, preserveGraphMeta });
        g.data({ nodes, edges });
        g.render();
        g.getEdges().forEach((e) => g.refreshItem(e));
        applyStyleToGraph(state.graphStyle, { syncView: false });
        if (state.edgeDetail) applyEdgeDetailLabels(true);
        syncEdgeDetailButton();
        updateGraphStats();
        updateCurrentViewSnapshot();
        scheduleProjectionLayoutIndexSync("render-direct-graph-rerender", { delayMs: 80 });
        finalizeView({ fit: fitView });
        commitShellStateSync("render-direct-graph-rerender");
        scheduleGraphPatch({
          scope: "local-update",
          reason: "render-direct-graph-rerender",
        });
        log("INFO", "direct graph rerender after size ready", {
          nodes: nodes?.length || 0,
          edges: edges?.length || 0,
          layout: summarizeLayout(nodes || []),
          edgeOffsets: summarizeEdgeOffsets(edges),
          sizeReady: ready2,
          ui: getGraphSizeInfo(g),
        });
      }, 120);
    }
  }

  async function applyDirectGraphPayload(data) {
    const graphPayload = getDirectGraphPayload(data);
    const normalized = normalizeGraphPayload(graphPayload);
    const drill = data?.drill || data?.txnDrill || null;
    if (drill) {
      if (!drill.dateStart && data?.dateStart) drill.dateStart = data.dateStart;
      if (!drill.dateEnd && data?.dateEnd) drill.dateEnd = data.dateEnd;
    }
    if (drill && drill.targetId) {
      const target = normalized.nodes.find((n) => n.id === drill.targetId);
      if (target) {
        target.drillable = !!drill.available;
        target.drill = drill;
      }
    }
    txnDrillState.cache.clear();
    txnDrillState.loading = false;

    const resolved = resolveGraphModeLayout(data || {});
    const layoutPreset = data?.layoutPreset || data?.layout || resolved.layoutPreset || "flow";
    state.graphMode = resolved.graphMode || "relation";
    state.layoutPreset = layoutPreset;
    setLayoutPreset(layoutPreset, { animate: false, markSelected: true });

    const focusId = resolveFocusIdFromNodes(normalized.nodes, state.focusId, state.focusName);
    const layoutSemanticProjection = await prepareLayoutSemanticProjection(
      normalized.nodes,
      normalized.edges,
      layoutPreset,
      { reason: "direct-graph-payload" }
    );
    applyLayout(normalized.nodes, normalized.edges, layoutPreset, focusId, {
      semanticProjection: layoutSemanticProjection,
    });
    try {
      const baseTxn = drill?.baseTxn || {};
      log("INFO", "direct graph payload prepared", {
        requestId: state.requestId || "",
        layoutPreset,
        graphMode: state.graphMode,
        layoutSelected: !!state.layoutSelected,
        focusId: focusId || "",
        drill: drill
          ? {
              targetId: drill.targetId || "",
              accountKey: drill.accountKey || "",
              available: !!drill.available,
              dateStart: drill.dateStart || "",
              dateEnd: drill.dateEnd || "",
              baseTxn: {
                amount: baseTxn.amount ?? null,
                time: baseTxn.time || "",
                txn_id: baseTxn.txn_id || "",
                from: baseTxn?.from?.account || "",
                to: baseTxn?.to?.account || "",
              },
            }
          : null,
        layout: summarizeLayout(normalized.nodes || []),
        edgeOffsets: summarizeEdgeOffsets(normalized.edges),
        summary: summarizeGraphData(normalized.nodes || [], normalized.edges || [], 4),
      });
    } catch (e) {}
    await renderDirectGraph(normalized.nodes, normalized.edges, {
      title: data?.title || "",
      toastLabel: "图谱结果已加载",
      createView: true,
      responsePayload: {
        flow_summary: cloneFlowSummaryBoundary(data?.flow_summary || data?.flowSummary),
        result_snapshot_ref: cloneResultSnapshotRef(data?.result_snapshot_ref || data?.resultSnapshotRef),
        projection: data?.projection || null,
        render_hints: data?.render_hints || data?.renderHints || null,
        graph_tier: data?.graph_tier || data?.graphTier || "",
      },
      layoutSemanticProjection,
    });
  }

  function focusProjectionExpandedNode(nodeId, { openDrawer = false } = {}) {
    const targetId = String(nodeId || "").trim();
    if (!targetId) return false;
    const graph = ensureGraph();
    if (!graph) return false;
    const item = graph.findById?.(targetId);
    if (!item) return false;
    try {
      clearGraphSelection(graph);
      graph.setItemState(item, "selected", true);
      graph.setItemState(item, "active", true);
      syncActiveSelection(graph, item);
      graph.focusItem?.(item, true, { easing: "easeCubic", duration: 240 });
      if (openDrawer) {
        openNodeDetailDrawer(item.getModel?.() || {}, graph);
      }
    } catch (e) {}
    return true;
  }

  async function requestProjectionExpand(
    expandPayload = {},
    { reason = "", preserveViewport = true, focusNodeId = "", openNodeDetail = false } = {}
  ) {
    const snapshotRef = cloneResultSnapshotRef(state.resultSnapshotRef);
    const snapshotId = String(snapshotRef?.snapshot_id || snapshotRef?.graph_hash || "").trim();
    if (!state.caseId || !snapshotId || !isSkeletonProjectionMode()) {
      return false;
    }
    const networkModeBeforeExpand = String(state.networkLayout?.lastPlanMode || "").trim().toLowerCase();
    const shouldRestoreNetworkModeForExpand =
      state.layoutPreset === "network" && isLargeNetworkTopologyMode(networkModeBeforeExpand);
    const clusterIds = parseUniqueList(expandPayload?.clusterIds || expandPayload?.cluster_ids || [], { allowString: true });
    const tileIds = parseUniqueList(expandPayload?.tileIds || expandPayload?.tile_ids || [], { allowString: true });
    const nodeIds = parseUniqueList(expandPayload?.nodeIds || expandPayload?.node_ids || [], { allowString: true });
    const pathNodeIds = parseUniqueList(expandPayload?.pathNodeIds || expandPayload?.path_node_ids || [], { allowString: true });
    const searchQuery = String(expandPayload?.searchQuery || expandPayload?.search_query || expandPayload?.query || "").trim();
    const searchLimit = Number.isFinite(Number(expandPayload?.searchLimit ?? expandPayload?.search_limit))
      ? clampNumber(Number(expandPayload?.searchLimit ?? expandPayload?.search_limit), 1, 256)
      : 24;
    const viewport =
      expandPayload?.viewport && typeof expandPayload.viewport === "object" && !Array.isArray(expandPayload.viewport)
        ? { ...(expandPayload.viewport || {}) }
        : null;
    const reset = !!expandPayload?.reset;
    if (!clusterIds.length && !tileIds.length && !nodeIds.length && !pathNodeIds.length && !viewport && !reset && !searchQuery) {
      return false;
    }
    const expandIntent = normalizeProjectionExpandIntent(
      expandPayload?.expandIntent ||
        expandPayload?.expand_intent ||
        (viewport
          ? "viewport"
          : searchQuery
          ? "search"
          : pathNodeIds.length
          ? "path"
          : clusterIds.length || tileIds.length
          ? "cluster"
          : "node")
    );
    const expandBudget = resolveProjectionExpandBudget(expandIntent, {
      materializeLimit: expandPayload?.materializeLimit ?? expandPayload?.materialize_limit,
      hintMaterializeLimit:
        expandPayload?.hintMaterializeLimit ?? expandPayload?.hint_materialize_limit ?? expandPayload?.hintLimit,
    });
    const expandSeq = (Number(state.graphExpandSeq) || 0) + 1;
    state.graphExpandSeq = expandSeq;
    const requestPayload = {
      caseId: state.caseId || "",
      snapshotId,
      searchQuery,
      searchLimit,
      clusterIds,
      tileIds,
      nodeIds,
      pathNodeIds,
      viewport,
      includeNeighbors: expandPayload?.includeNeighbors !== false,
      neighborDepth: clampNumber(Number(expandPayload?.neighborDepth), 0, 3),
      materializeLimit: expandBudget.materializeLimit,
      reset,
      graph: {
        ...(state.lastRequest?.graph && typeof state.lastRequest.graph === "object" ? state.lastRequest.graph : {}),
        render_mode: "skeleton",
      },
      drill: state.lastRequest?.drill && typeof state.lastRequest.drill === "object" ? state.lastRequest.drill : {},
    };
    if (state.layoutPreset === "network") {
      suppressNetworkTopologyPlanForProjectionExpand(reason || "projection-expand");
      if (state.networkLayout?.expandedCommunities instanceof Set) {
        clusterIds.forEach((id) => {
          if (id) state.networkLayout.expandedCommunities.add(id);
        });
      }
    }
    setGraphLoading(true, "正在展开图谱…");
    try {
      const raw = await backendCall("expandFlowResultSnapshot", JSON.stringify(requestPayload));
      if (expandSeq !== (Number(state.graphExpandSeq) || 0)) return false;
      const res = parseJSONSafe(raw, {});
      if (!res || !res.ok) {
        toast(res?.error || "图谱展开失败", "danger");
        return false;
      }
      const normalized = normalizeGraphPayload(res);
      const responseProjectionSource = res?.projection && typeof res.projection === "object" ? res.projection : {};
      const responseProjection = {
        ...responseProjectionSource,
        expanded_cluster_ids: parseUniqueList(
          [
            ...clusterIds,
            ...parseUniqueList(responseProjectionSource.expanded_cluster_ids || responseProjectionSource.expandedClusterIds || [], {
              allowString: true,
            }),
          ],
          { allowString: true }
        ),
        cluster_ids: parseUniqueList(
          [
            ...clusterIds,
            ...parseUniqueList(responseProjectionSource.cluster_ids || responseProjectionSource.clusterIds || [], {
              allowString: true,
            }),
          ],
          { allowString: true }
        ),
        expanded_node_ids: parseUniqueList(
          [
            ...parseUniqueList(responseProjectionSource.expanded_node_ids || responseProjectionSource.expandedNodeIds || [], {
              allowString: true,
            }),
            ...nodeIds,
            ...pathNodeIds,
          ],
          { allowString: true }
        ),
        path_node_ids: parseUniqueList(
          [
            ...parseUniqueList(responseProjectionSource.path_node_ids || responseProjectionSource.pathNodeIds || [], {
              allowString: true,
            }),
            ...pathNodeIds,
          ],
          { allowString: true }
        ),
        search_query: searchQuery || String(responseProjectionSource.search_query || responseProjectionSource.searchQuery || "").trim(),
      };
      let seededLayout = null;
      try {
        const seedBaseNodes =
          shouldRestoreNetworkModeForExpand && isLargeNetworkTopologyMode(networkModeBeforeExpand)
            ? collectProjectionLayoutSeedBaseNodes(ensureGraph(), state.graph?.data?.nodes || [])
            : state.graph?.data?.nodes || [];
        seededLayout = await projectProjectionLayoutSeed(normalized.nodes, seedBaseNodes);
      } catch (error) {
        log("WARN", "rust projection layout seed failed", { message: String(error?.message || error), stack: error?.stack || "" });
        toast("图谱布局种子失败", "danger");
        return false;
      }
      const graphTier = normalizeGraphTier(res?.graph_tier || res?.graphTier || "", normalized.nodes.length);
      const renderHints = normalizeGraphRenderHints(
        res?.render_hints || res?.renderHints || null,
        normalized.nodes.length,
        normalized.edges.length,
        state.graphMode || "relation"
      );
      let renderSummary = null;
      try {
        renderSummary = await applyGraphTierRenderPlanWithRust(
          normalized.nodes,
          normalized.edges,
          { ...state, graphTier, renderHints, graphProjection: cloneGraphProjection(responseProjection) },
          { preserveEdgeLabels: false, name: "projection-expand" }
        );
      } catch (e) {
        log("WARN", "rust projection render plan failed", { message: String(e?.message || e), stack: e?.stack || "" });
        toast("图谱展示计划失败", "danger");
        return false;
      }
      const projectionSearchMatchIds = Array.isArray(responseProjection.search_match_node_ids)
        ? responseProjection.search_match_node_ids
        : Array.isArray(responseProjection.searchMatchNodeIds)
        ? responseProjection.searchMatchNodeIds
        : [];
      const activeGraphTier = renderSummary?.tier || graphTier;
      const activeRenderHints = renderSummary?.renderHints || renderHints;
      const shouldBoundNetworkExpandPatch = false;
      let appliedIncrementally = applyIncrementalProjectionPatch(normalized.nodes, normalized.edges, res, {
        reason: reason || "projection-expand",
        preserveViewport,
        graphTier: activeGraphTier,
        renderHints: activeRenderHints,
        projection: cloneGraphProjection(responseProjection),
        resultSnapshotRef: cloneResultSnapshotRef(res?.result_snapshot_ref || res?.resultSnapshotRef),
        maxUpsertNodes: shouldBoundNetworkExpandPatch ? Math.max(48, expandBudget.materializeLimit) : 0,
        maxUpsertEdges: shouldBoundNetworkExpandPatch ? Math.max(256, expandBudget.materializeLimit * 12) : 0,
        priorityNodeIds: parseUniqueList(
          [searchQuery, ...nodeIds, ...pathNodeIds, ...projectionSearchMatchIds],
          { allowString: true }
        ),
      });
      if (!appliedIncrementally && shouldBoundNetworkExpandPatch) {
        const graph = ensureGraph();
        if (graph) {
          replaceRuntimeGraphData(
            snapshotGraphDataFromRuntime(graph, {
              graphTier: activeGraphTier,
              renderHints: activeRenderHints,
              projection: cloneGraphProjection(responseProjection),
              resultSnapshotRef: cloneResultSnapshotRef(res?.result_snapshot_ref || res?.resultSnapshotRef),
              runtimeRevision: state.graph?.runtimeRevision,
            }),
            { reason: `projection-expand:${reason || "projection-expand"}-bounded-noop`, preserveGraphMeta: true }
          );
          recordNetworkRuntimePerfEvent("projection-expand-bounded-noop", {
            reason: String(reason || ""),
            mode: networkModeBeforeExpand || activeGraphTier,
            sourceNodes: normalized.nodes.length,
            sourceEdges: normalized.edges.length,
            runtimeNodes: graph.getNodes?.().length || 0,
            runtimeEdges: graph.getEdges?.().length || 0,
          });
          appliedIncrementally = true;
        }
      }
      if (!appliedIncrementally) {
        const layoutSemanticProjection = await prepareLayoutSemanticProjection(
          normalized.nodes,
          normalized.edges,
          state.layoutPreset || "flow",
          { reason: "projection-expand-render" }
        );
        let fallbackRenderSummary = null;
        try {
          fallbackRenderSummary = await applyGraphTierRenderPlanWithRust(
            normalized.nodes,
            normalized.edges,
            { ...state, graphTier: activeGraphTier, renderHints: activeRenderHints, graphProjection: cloneGraphProjection(responseProjection) },
            { preserveEdgeLabels: false, name: "projection-expand-render", includeStagedLabels: true }
          );
        } catch (e) {
          log("WARN", "rust projection full render plan failed", { message: String(e?.message || e), stack: e?.stack || "" });
          toast("图谱展示计划失败", "danger");
          return false;
        }
        await renderDirectGraph(normalized.nodes, normalized.edges, {
          title: getActiveView()?.title || "",
          toastLabel: "",
          createView: false,
          responsePayload: { ...res, projection: responseProjection, graph_tier: activeGraphTier, render_hints: activeRenderHints },
          preserveViewport: preserveViewport,
          fitView: false,
          preserveGraphMeta: true,
          suppressNetworkTopologyPlan: true,
          layoutSemanticProjection,
          renderPlanApplied: true,
          stagedLabelPlan: fallbackRenderSummary?.stagedLabelPlan || null,
          graphDataReplaceReason: "projection-expand-render-direct-graph",
        });
      }
      state.graphProjection = cloneGraphProjection(responseProjection);
      state.resultSnapshotRef = cloneResultSnapshotRef(res?.result_snapshot_ref || res?.resultSnapshotRef || state.resultSnapshotRef);
      const resolvedFocusNodeId = focusNodeId || String(projectionSearchMatchIds[0] || "").trim();
      if (shouldBoundNetworkExpandPatch && state.layoutPreset === "network") {
        capNetworkRuntimeSkeletonGraph(ensureGraph(), {
          mode: state.networkLayout?.lastPlanMode || networkModeBeforeExpand,
          reason: `projection-expand:${expandIntent}`,
          projection: cloneGraphProjection(responseProjection),
          priorityNodeIds: [resolvedFocusNodeId, ...nodeIds, ...pathNodeIds, ...projectionSearchMatchIds],
          maxNodes: 260,
          maxEdges: resolveNetworkRuntimeEdgeBudget(state.networkLayout?.lastPlanMode || networkModeBeforeExpand),
        });
      }
      if (resolvedFocusNodeId) {
        focusProjectionExpandedNode(resolvedFocusNodeId, { openDrawer: !!openNodeDetail });
      }
      scheduleProjectionLayoutIndexSync(reason || "projection-expand", { delayMs: 120 });
      scheduleShellStateSync("projection-expand");
      if (shouldRestoreNetworkModeForExpand && state.networkLayout && typeof state.networkLayout === "object") {
        state.networkLayout.lastPlanMode = networkModeBeforeExpand;
      }
      if (false && state.layoutPreset === "network" && isLargeNetworkTopologyMode(state.networkLayout?.lastPlanMode)) {
        const graph = ensureGraph();
        spreadNetworkRuntimeCollapsedCommunities(graph, `projection-expand:${expandIntent}`);
        relaxNetworkRuntimeSkeletonGraph(graph, {
          mode: state.networkLayout?.lastPlanMode,
          reason: `projection-expand:${expandIntent}`,
          iterations: 4,
        });
        centerNetworkRuntimeSkeletonAfterExpand(graph, `projection-expand:${expandIntent}`);
        suppressDenseNetworkRuntimeEdgeLabels(graph, {
          mode: state.networkLayout?.lastPlanMode,
          nodeCount: graph.getNodes?.().length || 0,
          edgeCount: graph.getEdges?.().length || 0,
        });
        scheduleNetworkViewportRefinement(`projection-expand:${expandIntent}`, { delayMs: 80, force: true });
        scheduleNetworkCommunityQualityRefresh(`projection-expand:${expandIntent}`, { delayMs: 140 });
      }
      try {
        log("INFO", "projection expand applied", {
          reason: reason || "",
          requestId: state.requestId || "",
          snapshotId,
          searchQuery,
          clusterIds,
          tileIds,
          nodeIds,
          pathNodeIds,
          viewport: viewport ? { zoom: Number(viewport.zoom) || null, center: viewport.center || null } : null,
          expandIntent,
          materializeLimit: requestPayload.materializeLimit,
          materializeBudget: expandBudget,
          nodes: normalized.nodes.length,
          edges: normalized.edges.length,
          projectionMode: String(res?.projection?.mode || "").trim().toLowerCase() || "skeleton",
          seededLayout,
          incremental: !!appliedIncrementally,
          patchKind: String(res?.patch_kind || "").trim().toLowerCase() || "full",
        });
      } catch (e) {}
      return true;
    } finally {
      if (expandSeq === (Number(state.graphExpandSeq) || 0)) {
        setGraphLoading(false);
      }
    }
  }

  async function expandProjectionNode(model, { reason = "" } = {}) {
    if (!model || typeof model !== "object") return false;
    const nodeId = String(model?.id || "").trim();
    const clusterId = isClusterProjectionNode(model)
      ? nodeId
      : String(model?.projection_cluster_id || model?.cluster_id || "").trim();
    if (!clusterId && !nodeId) {
      return false;
    }
    return requestProjectionExpand(
      {
        clusterIds: clusterId ? [clusterId] : [],
        nodeIds: !clusterId && nodeId ? [nodeId] : [],
        expandIntent: clusterId ? "cluster" : "node",
        includeNeighbors: !clusterId,
        neighborDepth: !clusterId ? 1 : 0,
      },
      {
        reason: reason || "node-expand",
        preserveViewport: true,
        focusNodeId: !clusterId && nodeId ? nodeId : "",
        openNodeDetail: !clusterId,
      }
    );
  }

  async function expandProjectionViewport({ reason = "" } = {}) {
    if (!canExpandCurrentProjection()) return false;
    const graph = ensureGraph();
    const viewport = captureGraphViewport(graph);
    if (!viewport) {
      toast("当前视口不可用", "warn");
      return false;
    }
    const snapshotRef = cloneResultSnapshotRef(state.resultSnapshotRef);
    const snapshotId = String(snapshotRef?.snapshot_id || snapshotRef?.graph_hash || "").trim();
    const hasLayoutIndex = hasProjectionLayoutSyncForSnapshot(snapshotId);
    let fallbackTargets = { clusterIds: [], tileIds: [], nodeIds: [] };
    if (!hasLayoutIndex) {
      try {
        fallbackTargets = await collectVisibleProjectionExpandTargets(graph, {
          padding: 96,
          maxClusters: 48,
          maxNodes: 480,
          preferTiles: false,
        });
      } catch (error) {
        log("WARN", "projection viewport targets failed", {
          reason: reason || "viewport-expand",
          message: String(error?.message || error || ""),
        });
        toast("当前视口展开目标计算失败", "warn");
        return false;
      }
    }
    if (
      !hasLayoutIndex &&
      !fallbackTargets.clusterIds.length &&
      !fallbackTargets.tileIds.length &&
      !fallbackTargets.nodeIds.length
    ) {
      toast("当前视口没有可展开的折叠节点", "warn");
      return false;
    }
    const hints = normalizeGraphRenderHints(
      state.renderHints || { tier: state.graphTier || "large" },
      state.graph?.data?.nodes?.length || 0,
      state.graph?.data?.edges?.length || 0,
      state.graphMode || "relation"
    );
    return requestProjectionExpand(
      {
        clusterIds: fallbackTargets.clusterIds,
        tileIds: fallbackTargets.tileIds,
        nodeIds: fallbackTargets.nodeIds,
        viewport,
        expandIntent: "viewport",
        includeNeighbors: true,
        neighborDepth: 1,
        hintMaterializeLimit: hints.projection_viewport_materialize_limit,
      },
      { reason: reason || "viewport-expand", preserveViewport: true }
    );
  }

  async function expandProjectionSelectionPath({ reason = "" } = {}) {
    if (!canExpandCurrentProjection()) return false;
    const graph = ensureGraph();
    const selectedNodes = (getSelectedGraphItems(graph).nodes || [])
      .map((item) => item?.getModel?.() || {})
      .filter((model) => !isClusterProjectionNode(model))
      .map((model) => String(model?.id || "").trim())
      .filter(Boolean);
    const pathNodeIds = parseUniqueList(selectedNodes, { allowString: true });
    if (pathNodeIds.length < 2) {
      toast("请先选择至少两个节点再展开路径", "warn");
      return false;
    }
    return requestProjectionExpand(
      {
        pathNodeIds,
        expandIntent: "path",
        includeNeighbors: true,
        neighborDepth: 1,
      },
      { reason: reason || "selection-path-expand", preserveViewport: true, focusNodeId: pathNodeIds[pathNodeIds.length - 1] }
    );
  }

  async function fetchAccountTxnRowsPage(accountKey, opts = {}) {
    if (!state.backend || typeof state.backend.getAccountTxnRows !== "function") {
      return { rows: [], done: true, nextCursor: null };
    }
    const limit = Math.max(0, Math.floor(Number(opts.limit) || 0));
    const payload = {
      caseId: state.caseId || "",
      accountKey: accountKey || "",
      dateStart: opts.dateStart || "",
      dateEnd: opts.dateEnd || "",
      startTime: opts.startTime || "",
      endTime: opts.endTime || "",
      sortDir: opts.sortDir || "asc",
      limit,
      cursor: opts.cursor || null,
    };
    const raw = await backendCall("getAccountTxnRows", JSON.stringify(payload));
    const res = parseJSONSafe(raw, {});
    const page = normalizeEdgeTxnCursorPageResult(res, limit);
    if (!page.ok) throw new Error(page.blocker || "account transaction evidence unavailable");
    return page;
  }

  async function fetchStatsTxnRowsPage(opts = {}) {
    if (!state.backend || typeof state.backend.getStatsTxnRows !== "function") {
      return { rows: [], done: true, nextCursor: null };
    }
    const limit = Math.max(0, Math.floor(Number(opts.limit) || 0));
    const payload = {
      caseId: state.caseId || "",
      selected: Array.isArray(opts.selected) ? opts.selected : [],
      keyType: opts.keyType === "name" ? "name" : "account",
      keyValue: opts.keyValue || "",
      keyValues: Array.isArray(opts.keyValues) ? opts.keyValues : [],
      dateStart: opts.dateStart || "",
      dateEnd: opts.dateEnd || "",
      filter: opts.filter === "in" || opts.filter === "out" ? opts.filter : "all",
      sortCol: opts.sortCol === "amount" ? "amount" : "txn_time",
      sortDir: opts.sortDir || "asc",
      limit,
      cursor: opts.cursor || null,
    };
    const raw = await backendCall("getStatsTxnRows", JSON.stringify(payload));
    const res = parseJSONSafe(raw, {});
    const page = normalizeEdgeTxnCursorPageResult(res, limit);
    if (!page.ok) throw new Error(page.blocker || "transaction evidence unavailable");
    return page;
  }

  async function fetchAccountTxnRows(accountKey, opts = {}) {
    const page = await fetchAccountTxnRowsPage(accountKey, opts);
    return page.rows;
  }

  function getDrillConfig() {
    const cfg = state.drillConfig || {};
    const policyRaw = String(cfg.policy || "auto").toLowerCase();
    const policy = ["auto", "balance", "time"].includes(policyRaw) ? policyRaw : "auto";
    let windowDays = Number(cfg.windowDays);
    if (!Number.isFinite(windowDays) || windowDays < 0) windowDays = 7;
    if (windowDays > 3650) windowDays = 3650;
    return { policy, windowDays };
  }

  function getGraphNodeInfo(graph, id) {
    return ensureCanvasHitViewportController()?.getGraphNodeInfo?.(graph, id) || null;
  }

  function getGraphExtremes(graph) {
    return ensureCanvasHitViewportController()?.getGraphExtremes?.(graph) || null;
  }

  function getGraphEdgeClientPoint(graph, model) {
    return ensureCanvasHitViewportController()?.getGraphEdgeClientPoint?.(graph, model) || null;
  }


  function mergeDrillGraph(extraNodes, extraEdges, options = {}) {
    return ensureCanvasDrillController()?.mergeDrillGraph?.(extraNodes, extraEdges, options);
  }

  async function drillTxnNode(model, ctxPos = null) {
    return ensureCanvasDrillController()?.drillTxnNode?.(model, ctxPos);
  }

  function buildNodeContextMenuItems(model) {
    if (!model) return [];
    const items = [];
    if (canExpandCurrentProjection()) {
      const selectedPathCandidateCount = (getSelectedGraphItems(ensureGraph()).nodes || [])
        .map((item) => item?.getModel?.() || {})
        .filter((row) => !isClusterProjectionNode(row))
        .length;
      if (isCollapsedProjectionNode(model) || String(model?.projection_cluster_id || model?.cluster_id || "").trim()) {
        items.push({
          label: isClusterProjectionNode(model) ? "展开簇节点" : "展开当前节点",
          action: () => {
            void expandProjectionNode(model, { reason: "node:contextmenu" });
          },
        });
      }
      items.push({
        label: "展开当前视口",
        action: () => {
          void expandProjectionViewport({ reason: "node:contextmenu-viewport" });
        },
      });
      items.push({
        label: "展开选中路径",
        disabled: selectedPathCandidateCount < 2,
        action: () => {
          void expandProjectionSelectionPath({ reason: "node:contextmenu-path" });
        },
      });
    }
    if (model.drillable && model.drill) {
      const disabled = !!model.drillExpanded || txnDrillState.loading;
      const label = model.drillExpanded ? "已穿透" : "穿透交易";
      items.push(
        {
          label,
          disabled,
          action: () => {
            const ctx = state.lastContextNode && state.lastContextNode.id === model.id ? state.lastContextNode : null;
            const anchorX = Number.isFinite(ctx?.clientX) ? ctx.clientX : window.innerWidth / 2;
            const anchorY = Number.isFinite(ctx?.clientY) ? ctx.clientY : window.innerHeight / 2;
            openDrillPopover(anchorX, anchorY, model, ctx);
          },
        },
      );
    }
    return items;
  }

  function updateLayoutButtons() {
    ensureLayoutCommandStore().updateLayoutButtons();
  }

  function syncGraphEdgeRoutingFromData(graph, edges) {
    ensureLayoutCommandStore().syncGraphEdgeRoutingFromData(graph, edges);
  }

  function setLayoutPreset(
    preset,
    { animate = true, markSelected = false, forceRecompute = false, semanticProjection = null } = {}
  ) {
    const normalizedPreset = String(preset || "compact").trim().toLowerCase();
    restoreNetworkDenseHoverEdges(null, "layout-preset-change");
    ensureLayoutCommandStore().setLayoutPreset(preset, {
      animate,
      markSelected,
      forceRecompute,
      semanticProjection,
      ensureGraph,
    });
    if (normalizedPreset === "network") {
      recordNetworkRuntimePerfEvent("preset-applied", {
        reason: "set-layout-preset",
        commandSeq: Number(state.layoutPresetCommandSeq || 0),
        forceRecompute: !!forceRecompute,
      });
      void scheduleNetworkTopologyPlan({
        reason: "set-layout-preset",
        semanticProjection,
        commandSeq: Number(state.layoutPresetCommandSeq || 0),
      });
    }
  }

  function applyNetworkNodeLabelLod(graph, cfg, { zoom = null, force = false } = {}) {
    // Network label LOD algorithm is intentionally cleared from app.js.
    void graph;
    void cfg;
    void zoom;
    void force;
  }

  function applyNetworkVisibilityPresentation(graph, nodes, edges, plan, cfg, zoom = null) {
    // Network visibility presentation algorithm is intentionally cleared from app.js.
    void nodes;
    void edges;
    void plan;
    void cfg;
    void zoom;
    restoreNetworkPresentation(graph);
  }

  function restoreNetworkPresentation(graph) {
    return restoreNetworkPresentationModel(graph, {
      nodes: state.graph?.data?.nodes,
      edges: state.graph?.data?.edges,
      graphStyle: state.graphStyle,
      asBool,
    });
  }

  function applyNetworkPostLayoutPresentation({
    nodes,
    edges,
    focusId,
    readability,
    source = "layout",
    preview = false,
  } = {}) {
    // Network post-layout algorithm is intentionally cleared from app.js.
    void nodes;
    void edges;
    void focusId;
    void readability;
    void source;
    void preview;
    return null;
  }

  function ensureGraphDataVisible(graph, nodes, edges, reason = "") {
    if (!graph || !Array.isArray(nodes) || !nodes.length) return false;
    const countNow = () => ({
      nodes: graph.getNodes?.().length || 0,
      edges: graph.getEdges?.().length || 0,
    });
    let counts = countNow();
    if (counts.nodes > 0) return true;
    for (let i = 0; i < 2; i += 1) {
      try {
        graph.data({ nodes, edges });
        graph.render();
        if (graph.paint) graph.paint();
      } catch (e) {}
      counts = countNow();
      if (counts.nodes > 0) {
        log("WARN", "graph items rehydrated", {
          reason: reason || "render",
          counts,
          expect: { nodes: nodes.length, edges: edges?.length || 0 },
          renderer: graph.get?.("renderer") || "",
        });
        return true;
      }
    }
    log("WARN", "graph items empty after rehydrate", {
      reason: reason || "render",
      counts,
      expect: { nodes: nodes.length, edges: edges?.length || 0 },
      renderer: graph.get?.("renderer") || "",
    });
    return false;
  }

  function graphEdgeIdentityKey(edge) {
    const id = String(edge?.id || "").trim();
    if (id) return `id:${id}`;
    const source = String(edge?.source || "").trim();
    const target = String(edge?.target || "").trim();
    if (!source || !target) return "";
    const mode = String(edge?.mode || "").trim();
    const arrow = String(edge?.edgeArrow || edge?.arrow || "").trim();
    const label = String(edge?.label || "").trim();
    return `k:${source}->${target}|m:${mode}|a:${arrow}|l:${label}`;
  }

  function runGraphConsistencyCheck(reason = "") {
    const controller = ensureCanvasRedrawController();
    if (controller && typeof controller.runGraphConsistencyCheck === "function") {
      return controller.runGraphConsistencyCheck(reason);
    }
    return false;
  }

  function scheduleGraphConsistencyCheck(reason = "") {
    const controller = ensureCanvasRedrawController();
    if (controller && typeof controller.scheduleGraphConsistencyCheck === "function") {
      controller.scheduleGraphConsistencyCheck(reason);
    }
  }

  function forceRedrawGraphFromState(reason = "", { restoreViewport = true, hideDuringRedraw = false } = {}) {
    const controller = ensureCanvasRedrawController();
    if (controller && typeof controller.forceRedrawGraphFromState === "function") {
      return controller.forceRedrawGraphFromState(reason, { restoreViewport, hideDuringRedraw });
    }
    return false;
  }

  function recreateGraphInstanceFromState(reason = "", { restoreViewport = true, hideDuringRebuild = false } = {}) {
    const controller = ensureCanvasRedrawController();
    if (controller && typeof controller.recreateGraphInstanceFromState === "function") {
      return controller.recreateGraphInstanceFromState(reason, { restoreViewport, hideDuringRebuild });
    }
    return false;
  }

  function shouldRunDeleteVisualGuard() {
    try {
      const forced = window.__ANALYTIX_DELETE_VISUAL_GUARD;
      if (typeof forced === "boolean") return forced;
    } catch (e) {}
    return !!(state.debugTrace || state.debugPerf || logSwitch.debug);
  }

  function shouldAlwaysRecreateAfterDelete() {
    try {
      const forced = window.__ANALYTIX_DELETE_ALWAYS_RECREATE;
      if (typeof forced === "boolean") return forced;
    } catch (e) {}
    return false;
  }

  function isDeleteReason(reason = "") {
    const text = String(reason || "").trim().toLowerCase();
    return text.startsWith("delete-selected");
  }

  function isDeleteProbeVerbose() {
    try {
      const forced = window.__ANALYTIX_DELETE_PROBE_VERBOSE;
      if (typeof forced === "boolean") return forced;
    } catch (e) {}
    return false;
  }

  function shouldLogConsistencyProbeNormal() {
    return isDeleteProbeVerbose();
  }

  function scheduleDeleteVisualGuard(reason = "", delays = DELETE_VISUAL_GUARD_DELAYS) {
    const controller = ensureCanvasRedrawController();
    if (controller && typeof controller.scheduleDeleteVisualGuard === "function") {
      controller.scheduleDeleteVisualGuard(reason, delays);
    }
  }

  function runGraphVisualSweep(reason = "", frames = 2) {
    const controller = ensureCanvasRedrawController();
    if (controller && typeof controller.runGraphVisualSweep === "function") {
      return controller.runGraphVisualSweep(reason, frames);
    }
    return false;
  }

  function scheduleGraphMutationReconcile(reason = "", delays = [80, 260, 720, 1100, 1600]) {
    const controller = ensureCanvasRedrawController();
    if (controller && typeof controller.scheduleGraphMutationReconcile === "function") {
      controller.scheduleGraphMutationReconcile(reason, delays);
    }
  }

  function scheduleGraphConsistencyProbe(reason = "", delays = [120, 360, 900, 1500]) {
    const seq = state.graphMutationSeq || 0;
    const list = Array.isArray(delays) ? delays : [180, 900, 1500];
    if (!isDeleteReason(reason) || isDeleteProbeVerbose()) {
      logGhostProbe("consistency-probe-schedule", { reason: reason || "mutation", seq, delays: list.slice() });
    }
    list.forEach((msRaw) => {
      const ms = Math.max(0, Number(msRaw) || 0);
      setTimeout(() => {
        if (seq !== (state.graphMutationSeq || 0)) {
          logGhostProbe(
            "consistency-probe-skip",
            { reason: reason || "mutation", seq, currentSeq: state.graphMutationSeq || 0, ms, skip: "seq-mismatch" },
            { dedupKey: `consistency-probe-skip|${seq}|${ms}|seq`, dedupMs: 180, onlyInDeleteWindow: true }
          );
          return;
        }
        if (state.graphLoading || state.edgeBatching) {
          logGhostProbe(
            "consistency-probe-skip",
            {
              reason: reason || "mutation",
              seq,
              currentSeq: state.graphMutationSeq || 0,
              ms,
              skip: state.graphLoading ? "graphLoading" : "edgeBatching",
            },
            { dedupKey: `consistency-probe-skip|${seq}|${ms}|busy`, dedupMs: 180, onlyInDeleteWindow: true }
          );
          return;
        }
        const mismatchHandled = runGraphConsistencyCheck(`${reason || "mutation"}@${ms}ms`);
        let visualSweep = false;
        if (!mismatchHandled && String(reason || "").startsWith("delete-selected")) {
          visualSweep = runGraphVisualSweep(`${reason || "mutation"}@${ms}ms`, 2);
        }
        if (mismatchHandled || shouldLogConsistencyProbeNormal(`${reason || "mutation"}@${ms}ms`)) {
          logGhostProbe("consistency-probe-run", {
            reason: reason || "mutation",
            seq,
            ms,
            mismatchHandled: !!mismatchHandled,
            visualSweep: !!visualSweep,
          });
        }
      }, ms);
    });
  }

  async function buildGraph(options = {}) {
    const opts = options || {};
    const createView = !!opts.createView;
    const viewTitle = String(opts.title || "").trim();
    const buildSeq = (Number(state.graphBuildSeq) || 0) + 1;
    state.graphBuildSeq = buildSeq;
    state.lastBuildGraphPerf = null;
    const isStaleBuild = () => buildSeq !== (Number(state.graphBuildSeq) || 0);
    if (!state.caseId) {
      toast("未选择案件", "warn");
      return;
    }
    if (!state.selected.size) {
      toast("请先在统计分析选择账户后再展示图谱", "warn");
      return;
    }
    if (!state.backend || typeof state.backend.getFlowGraph !== "function") {
      toast("后端未就绪", "warn");
      return;
    }
    resetAnalysisFilterTools({ clearRange: true, clearSource: true, closePopover: true });

    const seedList = Array.from(state.selected);
    if (!seedList.length && state.focusId) seedList.push(state.focusId);
    const payload = {
      caseId: state.caseId || "",
      seeds: seedList,
      leftSeeds: Array.isArray(state.lastRequest?.leftSeeds)
        ? state.lastRequest.leftSeeds
        : seedList,
      tab: state.tab,
      dir: state.dir,
      hop: state.hop,
      minAmount: Number(state.minAmount) || 0,
      maxEdges: Number(state.maxEdges) || 800,
      view: state.graphMode,
      layout: state.layoutPreset || "compact",
      focusOnly: state.focusOnly,
      focusSelfOnly: state.focusSelfOnly,
      focusId: state.focusId || "",
      focusName: state.focusName || "",
      focusLabel: state.focusLabel || "",
      focusKeyType: state.focusKeyType || "",
      focusUnknownName: !!state.focusUnknownName,
      focusCounterpartyStrict: !!state.focusCounterpartyStrict,
      includeMissingCounterparty: !!state.includeMissingCounterparty,
      focusIds: state.focusIds || [],
      focusNames: state.focusNames || [],
      focusPlaceholderKinds: state.focusPlaceholderKinds || [],
      source: state.source || "",
      requestId: state.requestId || "",
      expectedTotalAmount:
        state.expectedTotalAmount != null ? toFiniteNumber(state.expectedTotalAmount, 2) : null,
      expectedRowCount: Number(state.expectedRowCount) || 0,
      dateStart: String(state.lastRequest?.dateStart || state.lastRequest?.date_start || ""),
      dateEnd: String(state.lastRequest?.dateEnd || state.lastRequest?.date_end || ""),
      graph: {
        ...(state.lastRequest?.graph && typeof state.lastRequest.graph === "object" ? state.lastRequest.graph : {}),
        render_mode: resolveRequestedProjectionRenderMode(state),
      },
    };
    if (isStatsSourceValue(state.source) && shouldLogHandoffProbe()) {
      log("INFO", "viz->backend probe getFlowGraph request", {
        requestId: payload.requestId || "",
        caseId: payload.caseId || "",
        view: payload.view || "",
        dateStart: payload.dateStart || "",
        dateEnd: payload.dateEnd || "",
        seeds: {
          count: Array.isArray(payload.seeds) ? payload.seeds.length : 0,
          items: Array.isArray(payload.seeds) ? payload.seeds.slice(0, 16) : [],
          truncated: Array.isArray(payload.seeds) ? payload.seeds.length > 16 : false,
        },
        leftSeeds: {
          count: Array.isArray(payload.leftSeeds) ? payload.leftSeeds.length : 0,
          items: Array.isArray(payload.leftSeeds) ? payload.leftSeeds.slice(0, 16) : [],
          truncated: Array.isArray(payload.leftSeeds) ? payload.leftSeeds.length > 16 : false,
        },
        expected: {
          totalAmount:
            payload.expectedTotalAmount != null ? toFiniteNumber(payload.expectedTotalAmount, 2) : null,
          rowCount: Number(payload.expectedRowCount) || 0,
        },
        focus: {
          keyType: payload.focusKeyType || "",
          counterpartyStrict: !!payload.focusCounterpartyStrict,
          id: payload.focusId || "",
          name: payload.focusName || "",
          label: payload.focusLabel || "",
          idsCount: Array.isArray(payload.focusIds) ? payload.focusIds.length : 0,
          namesCount: Array.isArray(payload.focusNames) ? payload.focusNames.length : 0,
          placeholderKinds: Array.isArray(payload.focusPlaceholderKinds) ? payload.focusPlaceholderKinds.slice() : [],
          only: !!payload.focusOnly,
          selfOnly: !!payload.focusSelfOnly,
        },
      });
    }

    setGraphLoading(true, "正在生成图谱…");
    const t0 = performance.now();
    const buildPerf = {
      fetch: 0,
      parse: 0,
      normalize: 0,
      semanticProjection: 0,
      layoutApply: 0,
      layout: 0,
      sizeWait: 0,
      render: 0,
      style: 0,
      snapshot: 0,
      renderPlan: 0,
      stagedLabels: null,
      textInitial: null,
    };
    const recordBuildPerfPhase = (stage = "", startedAt = 0, detail = {}) => {
      const name = String(stage || "").trim();
      if (!name) return null;
      const durationMs = Math.max(0, Math.round((performance.now() - Number(startedAt || performance.now())) * 10) / 10);
      if (!Array.isArray(buildPerf.timeline)) buildPerf.timeline = [];
      const event = {
        stage: name,
        offsetMs: Math.max(0, Math.round((performance.now() - t0) * 10) / 10),
        durationMs,
        ...(detail && typeof detail === "object" ? { ...detail } : {}),
      };
      buildPerf.timeline.push(event);
      while (buildPerf.timeline.length > 48) buildPerf.timeline.shift();
      buildPerf.maxPhaseMs = Math.max(Number(buildPerf.maxPhaseMs) || 0, durationMs);
      return event;
    };
    const yieldBuildPerfPhase = async (stage = "", detail = {}) => {
      const yieldStart = performance.now();
      await yieldToMainThread();
      return recordBuildPerfPhase(stage, yieldStart, detail);
    };
    const raw = await backendCall("getFlowGraph", JSON.stringify(payload));
    const rawResponseIsObject = !!(raw && typeof raw === "object");
    buildPerf.responseTransport = rawResponseIsObject ? "object" : "json";
    buildPerf.responsePassthrough = !!(rawResponseIsObject && raw.runtime_graph_passthrough);
    buildPerf.fetch = Math.round((performance.now() - t0) * 10) / 10;
    const parseStart = performance.now();
    const res = parseJSONSafe(raw, {});
    buildPerf.parse = Math.round((performance.now() - parseStart) * 10) / 10;
    recordBuildPerfPhase("parse", parseStart);
    if (isStaleBuild()) {
      try {
        logGraphTrace("build-cancelled", { stage: "response", reason: "superseded", buildSeq });
      } catch (e) {}
      return;
    }
    if (!res || !res.ok) {
      if (isStaleBuild()) return;
      setGraphLoading(false);
      toast(res?.error || "图谱生成失败", "danger");
      return;
    }
    const responseFlowSummary = cloneFlowSummaryBoundary(res?.flow_summary || res?.flowSummary);
    if (responseFlowSummary?.status === "blocked") {
      try {
        state.graph?.instance?.clear?.();
      } catch (_error) {}
      replaceRuntimeGraphData(
        {
          nodes: [],
          edges: [],
          flow_summary: responseFlowSummary,
        },
        { reason: "build-graph-publication-blocked", preserveGraphMeta: false }
      );
      setGraphLoading(false);
      updateGraphStats();
      commitShellStateSync("build-graph-publication-blocked");
      toast("图谱事实发布已阻断", "warn");
      return;
    }
    try {
      logGraphTrace("response", {
        counts: { nodes: res?.nodes?.length || 0, edges: res?.edges?.length || 0 },
        truncated: res?.truncated || null,
        tab: state.tab,
        mode: state.dir,
        hop: state.hop,
        maxEdges: state.maxEdges,
        seeds: seedList.length,
        focus: {
          id: state.focusId || "",
          name: state.focusName || "",
          label: state.focusLabel || "",
          keyType: state.focusKeyType || "",
        },
        focusOnly: !!state.focusOnly,
        focusSelfOnly: !!state.focusSelfOnly,
        includeMissingCounterparty: !!state.includeMissingCounterparty,
        focusIds: Array.isArray(state.focusIds) ? state.focusIds.length : 0,
        focusNames: Array.isArray(state.focusNames) ? state.focusNames.length : 0,
        focusPlaceholderKinds: Array.isArray(state.focusPlaceholderKinds) ? state.focusPlaceholderKinds.length : 0,
      });
    } catch (e) {}

    let nodes = [];
    let edges = [];
    try {
      const normalizeStart = performance.now();
      const normalized = await normalizeGraphPayloadForBuild(res);
      nodes = normalized.nodes || [];
      edges = normalized.edges || [];
      buildPerf.normalize = Math.round((performance.now() - normalizeStart) * 10) / 10;
      if (normalized.normalizeDetail) buildPerf.normalizeDetail = normalized.normalizeDetail;
      recordBuildPerfPhase("normalize", normalizeStart, {
        nodes: nodes.length,
        edges: edges.length,
        chunked: !!normalized.normalizeDetail?.chunked,
        maxChunkMs: Number(normalized.normalizeDetail?.maxChunkMs || 0),
        yieldCount: Number(normalized.normalizeDetail?.yieldCount || 0),
      });
    } catch (e) {
      if (isStaleBuild()) return;
      setGraphLoading(false);
      log("WARN", "normalize graph failed", { message: String(e?.message || e), stack: e?.stack || "" });
      toast("图谱生成失败", "danger");
      return;
    }
    if (isStaleBuild()) return;
    if (isStatsSourceValue(state.source) && shouldLogHandoffProbe()) {
      const renderedAmount = toFiniteNumber(computeGraphTotalAmount(edges), 2);
      const expectedAmount = state.expectedTotalAmount != null ? toFiniteNumber(state.expectedTotalAmount, 2) : null;
      const amountDelta =
        expectedAmount != null ? toFiniteNumber(renderedAmount - expectedAmount, 2) : null;
      log("INFO", "viz probe render payload", {
        requestId: state.requestId || "",
        counts: { nodes: nodes.length, edges: edges.length },
        amount: renderedAmount,
        expected: {
          totalAmount: expectedAmount,
          rowCount: Number(state.expectedRowCount) || 0,
          amountDelta,
        },
      });
    }
    if (state.debugTrace && isStatsSourceValue(state.lastRequest?.source)) {
      try {
        const summary = summarizeGraphData(nodes, edges, 8);
        log("INFO", "graph data summary", {
          requestId: state.requestId || "",
          ...summary,
        });
      } catch (e) {}
    }
    if (!nodes.length || !edges.length) {
      logGraphTrace(
        "empty",
        { stage: "normalize", counts: { nodes: nodes.length, edges: edges.length } },
        "WARN"
      );
    }
    let sampleSummary = null;
    try {
      sampleSummary = summarizeGraphData(nodes, edges, 4);
      logGraphTrace("sample", {
        counts: sampleSummary.counts,
        sample: sampleSummary.sample,
      });
    } catch (e) {}
    if (state.focusSelfOnlyOnce) {
      state.focusSelfOnly = false;
      state.focusSelfOnlyOnce = false;
      state.focusLabel = "";
    }
    const focusId = resolveFocusIdFromNodes(nodes, state.focusId, state.focusName);
    const graphTier = normalizeGraphTier(res?.graph_tier || res?.graphTier || "", nodes.length);
    const renderHints = normalizeGraphRenderHints(
      res?.render_hints || res?.renderHints || null,
      nodes.length,
      edges.length,
      state.graphMode || "relation"
    );
    const responseProjection = cloneGraphProjection(res?.projection);
    const initialLayoutPreset = state.layoutPreset || "compact";
    const networkBuildMode =
      initialLayoutPreset === "network"
        ? resolveNetworkTopologyModeForGraph(nodes, edges, {
            graphTier,
            renderHints,
            projection: responseProjection,
          })
        : "";
    const deferNetworkBuildSemantic = false;
    const useNetworkSkeletonRenderPlanFastPath = false;
    const renderPlanStart = performance.now();
    let renderSummary = null;
    if (useNetworkSkeletonRenderPlanFastPath) {
      renderSummary = {
        tier: graphTier,
        renderHints,
        entityCount: Math.min(nodes.length, graphTier === "xlarge" ? 200 : 320),
        dotCount: Math.max(0, nodes.length - Math.min(nodes.length, graphTier === "xlarge" ? 200 : 320)),
        focusCount: 0,
        renderPlan: {
          summary: {
            tier: graphTier,
            nodeCount: nodes.length,
            edgeCount: edges.length,
            source: "network-skeleton-render-plan-fast-path",
          },
          normalizedRenderHints: renderHints,
          nodeRenderModes: [],
          nodeRenderUpdates: [],
          edgeRenderUpdates: [],
        },
        stagedLabelPlan: null,
      };
      buildPerf.renderPlanFastPath = true;
    } else {
      try {
        renderSummary = await applyGraphTierRenderPlanWithRust(
          nodes,
          edges,
          { ...state, graphTier, renderHints, leftSeeds: payload.leftSeeds },
          { preserveEdgeLabels: false, name: "build-graph", includeStagedLabels: true }
        );
      } catch (e) {
        if (isStaleBuild()) return;
        setGraphLoading(false);
        log("WARN", "rust graph render plan failed", { message: String(e?.message || e), stack: e?.stack || "" });
        toast("图谱展示计划失败", "danger");
        return;
      }
    }
    if (isStaleBuild()) return;
    buildPerf.renderPlan = Math.round((performance.now() - renderPlanStart) * 10) / 10;
    recordBuildPerfPhase(useNetworkSkeletonRenderPlanFastPath ? "render-plan-fast-path" : "render-plan", renderPlanStart, {
      tier: graphTier,
      mode: networkBuildMode || "",
    });
    const activeGraphTier = renderSummary?.tier || graphTier;
    const activeRenderHints = renderSummary?.renderHints || renderHints;
    state.graphTier = activeGraphTier;
    state.renderHints = activeRenderHints;
    const statsFastFirstPaint = shouldUseStatsFastFirstPaint({ ...state, graphTier: activeGraphTier, renderHints: activeRenderHints }, nodes, edges, res);
    const layoutStart = performance.now();
    let layoutSemanticProjection = null;
    if (deferNetworkBuildSemantic) {
      applyNetworkCoarsePlanState(nodes, edges, {
        reason: "build-graph-deferred-semantic-seed",
        graphSignature: buildNetworkPlanGraphSignature(nodes, edges),
      });
      buildPerf.deferredSemanticProjection = true;
      buildPerf.networkSemanticStrategy = "lightweight";
      recordBuildPerfPhase("semantic-projection-deferred", layoutStart, {
        preset: initialLayoutPreset,
        mode: networkBuildMode,
      });
      recordBuildPerfPhase("layout-apply-deferred", layoutStart, {
        preset: initialLayoutPreset,
        mode: networkBuildMode,
      });
    } else {
      const semanticProjectionStart = performance.now();
      layoutSemanticProjection = await prepareLayoutSemanticProjection(
        nodes,
        edges,
        initialLayoutPreset,
        { reason: "build-graph" }
      );
      buildPerf.semanticProjection = Math.round((performance.now() - semanticProjectionStart) * 10) / 10;
      recordBuildPerfPhase("semantic-projection", semanticProjectionStart, { preset: initialLayoutPreset });
      if (isStaleBuild()) return;
      const layoutApplyStart = performance.now();
      applyLayout(nodes, edges, initialLayoutPreset, focusId, {
        semanticProjection: layoutSemanticProjection,
      });
      buildPerf.layoutApply = Math.round((performance.now() - layoutApplyStart) * 10) / 10;
      recordBuildPerfPhase("layout-apply", layoutApplyStart, { preset: initialLayoutPreset });
    }
    buildPerf.layout = Math.round((performance.now() - layoutStart) * 10) / 10;
    const layoutSummary = summarizeLayout(nodes);
    if (isLayoutCollapsed(layoutSummary)) {
      logGraphTrace(
        "layout-collapsed",
        { view: state.layoutPreset || "compact", focusId, layout: layoutSummary },
        "WARN"
      );
    }
    const useEdgeBatch = shouldBatchEdges(edges.length, {
      ...state,
      graphTier: activeGraphTier,
      renderHints: activeRenderHints,
      graph: { ...state.graph, data: { nodes, edges } },
    });
    const stagedLabelPlan = renderSummary?.stagedLabelPlan || null;
    if (stagedLabelPlan) {
      buildPerf.stagedLabels = {
        t0Count: stagedLabelPlan.t0Count,
        deferredCount: stagedLabelPlan.deferredCount,
        deferredNodeCount: stagedLabelPlan.deferredNodeCount,
        deferredEdgeCount: stagedLabelPlan.deferredEdgeCount,
        hiddenNodeCount: stagedLabelPlan.hiddenNodeCount,
        tierHiddenNodeCount: stagedLabelPlan.tierHiddenNodeCount,
        suppressedSubtitleCount: stagedLabelPlan.suppressedSubtitleCount,
        hiddenEdgeLabelCount: stagedLabelPlan.hiddenEdgeLabelCount,
        initialLabelRows: stagedLabelPlan.initialLabelRows,
        initialNodeLabelRows: stagedLabelPlan.initialNodeLabelRows,
        initialEdgeLabelRows: stagedLabelPlan.initialEdgeLabelRows,
        budget: { ...stagedLabelPlan.budget },
      };
    }
    const initialReplaceStart = performance.now();
    replaceRuntimeGraphData(
      {
        nodes,
        edges,
        graph_tier: activeGraphTier,
        render_hints: activeRenderHints,
        projection: responseProjection,
        result_snapshot_ref: cloneResultSnapshotRef(res?.result_snapshot_ref || res?.resultSnapshotRef),
        flow_summary: cloneFlowSummaryBoundary(res?.flow_summary || res?.flowSummary),
      },
      { reason: "build-graph-initial" }
    );
    buildPerf.replaceInitial = Math.round((performance.now() - initialReplaceStart) * 10) / 10;
    recordBuildPerfPhase("replace-initial", initialReplaceStart, { nodes: nodes.length, edges: edges.length });
    if (state.layoutPreset === "network" && !deferNetworkBuildSemantic) {
      void scheduleNetworkTopologyPlan({
        reason: "build-graph-initial",
        semanticProjection: layoutSemanticProjection,
        commandSeq: Number(state.layoutPresetCommandSeq || 0),
      });
    }
    scheduleNetworkLayoutPrewarm({ reason: "build-graph-initial", nodes, edges });
    scheduleCompactLayoutPrewarm({ reason: "build-graph-initial", nodes, edges });

    const graph = ensureGraph();
    if (!graph) {
      if (isStaleBuild()) return;
      setGraphLoading(false);
      toast("图谱组件加载失败", "danger");
      return;
    }
    cancelEdgeBatching();

    try {
      const graphClearStart = performance.now();
      graph.clear();
      buildPerf.graphClear = Math.round((performance.now() - graphClearStart) * 10) / 10;
      recordBuildPerfPhase("graph-clear", graphClearStart);
    } catch (e) {}
    if (state.layoutPreset === "network" && shouldSuppressDenseNetworkEdgeLabels(networkBuildMode, nodes.length, edges.length)) {
      await yieldBuildPerfPhase("yield-before-dense-render", { mode: networkBuildMode });
      if (isStaleBuild()) return;
    }
    try {
      const renderMutationSeq = Number(state.graphMutationSeq || 0);
      const isRenderCancelled = () =>
        isStaleBuild() || renderMutationSeq !== (Number(state.graphMutationSeq) || 0);
      const deferEdgeRefresh =
        statsFastFirstPaint && !useEdgeBatch && !state.edgeDetail && !state.signedEdgeDetail;
      const suppressStagedLabelsForNetworkSkeleton = false;
      const useLightIntro = !!stagedLabelPlan && !suppressStagedLabelsForNetworkSkeleton && nodes.length > 1;
      const introAnimate =
        !useLightIntro && !statsFastFirstPaint && !useEdgeBatch && nodes.length > 1 && nodes.length <= 600;
      const smallGraphIntro = !useEdgeBatch && nodes.length > 1 && nodes.length <= 20;
      const introDuration = smallGraphIntro ? 880 : 520;
      const introEasing = smallGraphIntro ? "easeOutCubic" : "easeInOutCubic";
      const useLightSnapshot = statsFastFirstPaint && !useEdgeBatch && !introAnimate;
      const lightIntroDuration = Number(stagedLabelPlan?.budget?.introDuration) || 220;
      const startStagedLabelReveal = () => {
        if (!stagedLabelPlan || suppressStagedLabelsForNetworkSkeleton) return;
        scheduleStagedNodeLabelReveal(stagedLabelPlan, {
          graph,
          isCancelled: isRenderCancelled,
          onComplete: ({ cancelled = false, duration = 0, batches = 0 } = {}) => {
            if (cancelled || isRenderCancelled()) return;
            const syncStart = performance.now();
            const revealedSnapshot = snapshotGraphData();
            replaceRuntimeGraphData(revealedSnapshot, { reason: "build-graph-staged-labels" });
            syncAnalysisSourceDataFromCurrent({ force: true });
            rememberGraphPatchBase(revealedSnapshot);
            updateCurrentViewSnapshot();
            const syncMs = Math.round((performance.now() - syncStart) * 10) / 10;
            if (state.debugPerf || isStatsSourceValue(state.source)) {
              log("INFO", "build graph staged labels done", {
                requestId: state.requestId || "",
                counts: { nodes: nodes.length, edges: edges.length },
                staged: buildPerf.stagedLabels,
                duration,
                batches,
                syncMs,
                text: collectGraphTextRenderSummary(graph),
              });
            }
          },
        });
      };
      state.introAnimating = introAnimate || useLightIntro;
      const sizeWaitStart = performance.now();
      const ready = await resolveGraphRenderSize({
        minW: 320,
        minH: 200,
        retries: 14,
        delay: 80,
        preferFast: statsFastFirstPaint,
      });
      buildPerf.sizeWait = Math.round((performance.now() - sizeWaitStart) * 10) / 10;
      if (isStaleBuild()) return;
      const containerEl = graph.get("container");
      const cw = containerEl?.clientWidth || ready.w || 0;
      const ch = containerEl?.clientHeight || ready.h || 0;
      if (cw > 0 && ch > 0 && graph.changeSize) {
        graph.changeSize(cw, ch);
        state.lastGraphContainerSize = { w: cw, h: ch };
      }
      const sizeInfo = state.debugTrace
        ? (() => {
            const wrapEl = $("#graphWrap");
            const cardEl = $("#graphCard");
            const rightEl = $(".right");
            const appEl = $(".app");
            const rect = (el) => {
              if (!el) return null;
              const r = el.getBoundingClientRect();
              return { w: Math.round(r.width), h: Math.round(r.height) };
            };
            return {
              window: { w: window.innerWidth, h: window.innerHeight },
              app: rect(appEl),
              right: rect(rightEl),
              card: rect(cardEl),
              wrap: rect(wrapEl),
              container: rect(containerEl),
            };
          })()
        : null;
      if (state.debugTrace) {
        log("INFO", "render graph start", {
          nodes: nodes.length,
          edges: edges.length,
          perfMode: isPerfMode(),
          edgeBatch: useEdgeBatch,
          deferEdgeRefresh,
          fastFirstPaint: statsFastFirstPaint,
          lightSnapshot: useLightSnapshot,
          renderer: graph.get?.("renderer") || "",
          graphW: graph.get("width"),
          graphH: graph.get("height"),
          containerW: cw,
          containerH: ch,
          sizeSource: ready.source || "",
          sizes: sizeInfo,
          sizeReady: ready.ready,
        });
      }
      const renderStart = performance.now();
      const layoutTargets = new Map(nodes.map((n) => [n.id, { x: n.x, y: n.y }]));
      const prevOpacity = containerEl?.style.opacity || "";
      const networkInitialMode =
        state.layoutPreset === "network"
          ? resolveNetworkTopologyModeForGraph(nodes, edges, {
              graphTier: activeGraphTier,
              renderHints: activeRenderHints,
              projection: responseProjection,
            })
          : "";
      const networkSkeletonInitialRender = false;
      state.networkLayout.activeMode = "full";
      if (networkSkeletonInitialRender) {
        applyNetworkCoarsePlanState(nodes, edges, {
          reason: "network-skeleton-first-paint",
          graphSignature: buildNetworkPlanGraphSignature(nodes, edges),
        });
      }
      let firstPaintEdges = networkSkeletonInitialRender
        ? selectNetworkSkeletonRenderEdges(
            nodes,
            edges,
            resolveNetworkSkeletonEdgeBudget(networkInitialMode, nodes.length, edges.length),
            { includeIncidentEdges: isLargeNetworkTopologyMode(networkInitialMode) }
          )
        : edges;
      const firstPaintNodeCandidates =
        networkSkeletonInitialRender
          ? selectNetworkSkeletonRenderNodes(nodes, firstPaintEdges, networkInitialMode).map((node) =>
              toNetworkSkeletonRenderNode(node)
            )
          : nodes;
      const firstPaintNodes = networkSkeletonInitialRender
        ? capNetworkSkeletonRenderLabels(firstPaintNodeCandidates, networkInitialMode)
        : firstPaintNodeCandidates;
      if (networkSkeletonInitialRender) {
        const firstPaintNodeIds = new Set(firstPaintNodes.map((node) => String(node?.id || "").trim()).filter(Boolean));
        firstPaintEdges = filterNetworkSkeletonRenderEdgesToNodes(firstPaintEdges, firstPaintNodeIds);
      }
      const denseRenderLod = !networkSkeletonInitialRender
        ? applyDenseNetworkRenderLod(firstPaintNodes, firstPaintEdges, { mode: networkInitialMode })
        : { edgeLabelsSuppressed: 0 };
      if (networkSkeletonInitialRender) {
        buildPerf.firstPaintText = summarizeNetworkSkeletonTextCandidates(firstPaintNodes, firstPaintEdges);
      }
      if (useEdgeBatch) {
        const renderDataStart = performance.now();
        if (networkSkeletonInitialRender && typeof graph.setAutoPaint === "function") {
          graph.setAutoPaint(false);
          try {
            graph.data({ nodes: firstPaintNodes, edges: firstPaintEdges });
          } finally {
            graph.setAutoPaint(true);
          }
          graph.render();
        } else {
          graph.data({ nodes: firstPaintNodes, edges: networkSkeletonInitialRender ? firstPaintEdges : [] });
          graph.render();
        }
        if (!networkSkeletonInitialRender) {
          if (graph.refreshPositions) graph.refreshPositions();
          if (graph.paint) graph.paint();
        }
        ensureGraphDataVisible(graph, firstPaintNodes, networkSkeletonInitialRender ? firstPaintEdges : [], "build-batch");
        if (!networkSkeletonInitialRender) {
          syncGraphItemPositionsFromData(graph, firstPaintNodes, "batch-render", { force: true, log });
        }
        buildPerf.renderData = Math.round((performance.now() - renderDataStart) * 10) / 10;
        recordBuildPerfPhase("render-data", renderDataStart, {
          nodes: firstPaintNodes.length,
          edges: networkSkeletonInitialRender ? firstPaintEdges.length : 0,
          edgeBatch: true,
        });
        if (networkSkeletonInitialRender) {
          applyNetworkPlanPresentationToGraph(graph, firstPaintNodes, firstPaintEdges, { mode: networkInitialMode });
	          relaxNetworkRuntimeSkeletonGraph(graph, {
	            mode: networkInitialMode,
	            reason: "network-skeleton-first-paint",
	            iterations: networkInitialMode === "xlarge" ? 28 : 16,
	          });
          fitGraph({ force: true });
          centerGraphOnLayout(
            graph,
            networkInitialMode === "xlarge" ? collectNetworkRuntimeNodeModels(graph) : firstPaintNodes,
            Math.max(0.28, Math.min(0.9, Number(graph.getZoom?.()) || 0.55))
          );
        }
        updateMinimap();
        if (containerEl) containerEl.style.opacity = prevOpacity || "1";
        finalizeView();
        if (networkSkeletonInitialRender) {
          fitGraph({ force: true });
          updateMinimap();
          logGraphRenderStatus(
            graph,
            {
              nodes: firstPaintNodes.length,
              totalNodes: nodes.length,
              edges: firstPaintEdges.length,
              totalEdges: edges.length,
              mode: networkInitialMode,
              sample: sampleSummary?.sample || null,
            },
            "network-skeleton-rendered"
          );
          emitCanvasSnapshotLog("build-graph-network-skeleton", graph);
          recordNetworkRuntimePerfEvent("skeleton-rendered", {
            mode: networkInitialMode,
            reason: "build-graph-network-skeleton",
            durationMs: performance.now() - renderStart,
            nodes: firstPaintNodes.length,
            totalNodes: nodes.length,
            edges: firstPaintEdges.length,
            totalEdges: edges.length,
          });
        } else {
          renderEdgesInBatches(graph, edges, {
            onComplete: () => {
              if (state.edgeDetail) applyEdgeDetailLabels(true);
              updateMinimap();
              fitGraph({ force: true });
              logGraphRenderStatus(
                graph,
                { nodes: nodes.length, edges: edges.length, sample: sampleSummary?.sample || null },
                "edge-batch-done"
              );
              emitCanvasSnapshotLog("build-graph-edge-batch-done", graph);
            },
          });
        }
      } else {
        const renderDataStart = performance.now();
        if (networkSkeletonInitialRender && typeof graph.setAutoPaint === "function") {
          graph.setAutoPaint(false);
          try {
            graph.data({ nodes: firstPaintNodes, edges: firstPaintEdges });
          } finally {
            graph.setAutoPaint(true);
          }
          graph.render();
        } else {
          graph.data({ nodes: firstPaintNodes, edges: firstPaintEdges });
          graph.render();
        }
        if (!networkSkeletonInitialRender && graph.paint) graph.paint();
        ensureGraphDataVisible(graph, firstPaintNodes, firstPaintEdges, "build-direct");
        if (networkSkeletonInitialRender) {
          applyNetworkPlanPresentationToGraph(graph, firstPaintNodes, firstPaintEdges, { mode: networkInitialMode });
	          relaxNetworkRuntimeSkeletonGraph(graph, {
	            mode: networkInitialMode,
	            reason: "network-skeleton-first-paint",
	            iterations: networkInitialMode === "xlarge" ? 28 : 10,
	          });
          fitGraph({ force: true });
          centerGraphOnLayout(
            graph,
            networkInitialMode === "xlarge" ? collectNetworkRuntimeNodeModels(graph) : firstPaintNodes,
            Math.max(0.28, Math.min(0.9, Number(graph.getZoom?.()) || 0.55))
          );
          buildPerf.renderData = Math.round((performance.now() - renderDataStart) * 10) / 10;
          recordBuildPerfPhase("render-data", renderDataStart, {
            nodes: firstPaintNodes.length,
            edges: firstPaintEdges.length,
            edgeBatch: false,
          });
          recordNetworkRuntimePerfEvent("skeleton-rendered", {
            mode: networkInitialMode,
            reason: "build-graph-network-skeleton",
            durationMs: performance.now() - renderStart,
            nodes: firstPaintNodes.length,
            totalNodes: nodes.length,
            edges: firstPaintEdges.length,
            totalEdges: edges.length,
          });
        }
        updateMinimap();
        if (deferEdgeRefresh) {
          scheduleGraphItemRefreshBatches(graph, graph.getEdges?.() || [], {
            batchSize: Math.max(resolveEdgeRefreshBatchSize(edges.length), resolveEdgeBatchSize(edges.length)),
            timeout: 180,
            isCancelled: isStaleBuild,
            onComplete: ({ cancelled = false } = {}) => {
              if (cancelled || isStaleBuild()) return;
              updateMinimap();
              emitCanvasSnapshotLog("build-graph-edge-refresh-idle", graph);
            },
          });
        } else {
          graph.getEdges().forEach((e) => graph.refreshItem(e));
        }
        logGraphRenderStatus(
          graph,
          { nodes: nodes.length, edges: edges.length, sample: sampleSummary?.sample || null },
          "rendered"
        );
        emitCanvasSnapshotLog("build-graph-rendered", graph);
      }
      if (networkSkeletonInitialRender && networkInitialMode === "large") {
        const finalPlanSemanticProjection =
          layoutSemanticProjection ||
          buildNetworkLightweightSemanticProjection(nodes, {
            reason: "network-skeleton-first-paint-final",
            mode: networkInitialMode,
          });
        const finalPlanCommandSeq = Number(state.layoutPresetCommandSeq || 0);
        setTimeout(() => {
          if (isStaleBuild() || state.layoutPreset !== "network") return;
          if (state.networkLayout?.lastPlanSampleQuality) return;
          void scheduleNetworkTopologyPlan({
            reason: "network-skeleton-first-paint-final",
            semanticProjection: finalPlanSemanticProjection,
            commandSeq: finalPlanCommandSeq,
          });
        }, 80);
      }
      const denseEdgeLabelsSuppressed =
        Number(denseRenderLod?.edgeLabelsSuppressed || 0) +
        suppressDenseNetworkRuntimeEdgeLabels(graph, {
          mode: networkInitialMode,
          nodeCount: nodes.length,
          edgeCount: edges.length,
        });
      if (denseEdgeLabelsSuppressed > 0) {
        buildPerf.denseEdgeLabels = {
          suppressed: denseEdgeLabelsSuppressed,
          mode: networkInitialMode,
          density: Math.round((edges.length / Math.max(1, nodes.length)) * 100) / 100,
        };
      }
      const denseNodeLabelsSuppressed = suppressDenseNetworkRuntimeNodeLabels(graph, {
        mode: networkInitialMode,
        nodeCount: nodes.length,
        edgeCount: edges.length,
      });
      if (denseNodeLabelsSuppressed > 0) {
        buildPerf.denseNodeLabels = {
          suppressed: denseNodeLabelsSuppressed,
          mode: networkInitialMode,
        };
      }
      buildPerf.textInitial = collectGraphTextRenderSummary(graph);
      if (stagedLabelPlan && (state.debugPerf || isStatsSourceValue(state.source))) {
        log("INFO", "build graph staged labels first paint", {
          requestId: state.requestId || "",
          counts: { nodes: nodes.length, edges: edges.length },
          staged: buildPerf.stagedLabels,
          text: buildPerf.textInitial,
          statsFastFirstPaint: !!statsFastFirstPaint,
          useLightIntro: !!useLightIntro,
        });
      }
      if (useLightIntro) {
        state.introAnimating = true;
        state.introSuppressUntil = Date.now() + lightIntroDuration + 220;
        playLightGraphIntro(containerEl, {
          duration: lightIntroDuration,
          isCancelled: isRenderCancelled,
          onComplete: ({ cancelled = false } = {}) => {
            if (cancelled || isRenderCancelled()) return;
            state.introAnimating = false;
            state.introSuppressUntil = 0;
            finalizeView({ fit: true });
            startStagedLabelReveal();
          },
        });
      } else if (introAnimate) {
        state.introAnimating = true;
        state.introSuppressUntil = Date.now() + (smallGraphIntro ? introDuration + 280 : 3200);
        fitGraph({ force: true });
        const summary = summarizeLayout(nodes);
        const collapseCenter = {
          x: summary.minX != null && summary.maxX != null ? (summary.minX + summary.maxX) / 2 : 0,
          y: summary.minY != null && summary.maxY != null ? (summary.minY + summary.maxY) / 2 : 0,
        };
        const jitterScale = smallGraphIntro ? 0.84 : 1.2;
        if (containerEl) containerEl.style.opacity = "0";
        graph.setAutoPaint(false);
        graph.getNodes().forEach((node, idx) => {
          const model = node.getModel() || {};
          const target = layoutTargets.get(model.id);
          if (!target) return;
          const jitter = ((idx % 9) - 4) * jitterScale;
          model.x = collapseCenter.x + jitter;
          model.y = collapseCenter.y - jitter;
        });
        graph.refreshPositions();
        graph.setAutoPaint(true);
        graph.paint();
        requestAnimationFrame(() => {
          if (isRenderCancelled()) return;
          if (containerEl) containerEl.style.opacity = prevOpacity || "1";
          animateNodesToTargets(graph, layoutTargets, {
            duration: introDuration,
            easing: introEasing,
            fit: smallGraphIntro,
            onComplete: () => {
              if (isRenderCancelled()) return;
              syncGraphNodePositions(graph);
              if (smallGraphIntro) centerGraphOnLayout(graph, nodes);
              updateCurrentViewSnapshot();
              startStagedLabelReveal();
            },
          });
        });
      } else {
        state.introAnimating = false;
        state.introSuppressUntil = 0;
        if (containerEl) containerEl.style.opacity = prevOpacity || "1";
        if (!useEdgeBatch) finalizeView();
        startStagedLabelReveal();
      }
      buildPerf.render = Math.round((performance.now() - renderStart) * 10) / 10;
      setTimeout(() => {
        try {
          const containerEl = graph.get("container");
          const cw = containerEl?.clientWidth || 0;
          const ch = containerEl?.clientHeight || 0;
          if (cw > 0 && ch > 0 && graph.changeSize) {
            graph.changeSize(cw, ch);
            state.lastGraphContainerSize = { w: cw, h: ch };
          }
          if (state.debugTrace) {
            log("INFO", "render graph post-size", { containerW: cw, containerH: ch });
          }
        } catch (e) {}
      }, 120);
    } catch (e) {
      state.introAnimating = false;
      log("WARN", "render graph failed", {
        message: String(e?.message || e),
        stack: e?.stack || "",
      });
      try {
        graph.changeData({ nodes, edges });
        finalizeView();
        log("INFO", "render graph fallback changeData ok");
      } catch (e2) {
        log("WARN", "render graph fallback failed", {
          message: String(e2?.message || e2),
          stack: e2?.stack || "",
        });
      }
    }

    if (isStaleBuild()) return;
    if (deferNetworkBuildSemantic) {
      await yieldBuildPerfPhase("yield-after-render", { mode: networkBuildMode });
      if (isStaleBuild()) return;
    }
    const styleStart = performance.now();
    applyStyleToGraph(state.graphStyle, { syncView: false, applyAnalysis: false });
    if (state.edgeDetail) applyEdgeDetailLabels(true);
    const densePostStyleMode =
      state.networkLayout?.lastPlanMode ||
      resolveNetworkTopologyModeForGraph(nodes, edges, {
        graphTier: activeGraphTier,
        renderHints: activeRenderHints,
        projection: responseProjection,
      });
    const densePostStyleLod = applyDenseNetworkRuntimeLod(graph, {
      mode: densePostStyleMode,
      nodeCount: nodes.length,
      edgeCount: edges.length,
    });
    if (densePostStyleLod.edgeLabelsSuppressed > 0) {
      buildPerf.denseEdgeLabels = {
        suppressed: densePostStyleLod.edgeLabelsSuppressed,
        mode: densePostStyleMode,
        density: Math.round((edges.length / Math.max(1, nodes.length)) * 100) / 100,
      };
    }
    if (densePostStyleLod.nodeLabelsSuppressed > 0) {
      buildPerf.denseNodeLabels = {
        suppressed: densePostStyleLod.nodeLabelsSuppressed,
        mode: densePostStyleMode,
      };
    }
    syncEdgeDetailButton();
    buildPerf.style = Math.round((performance.now() - styleStart) * 10) / 10;
    recordBuildPerfPhase("style", styleStart);

    if (isStaleBuild()) return;
    const snapshotStart = performance.now();
    const snapshot = snapshotGraphData({ preferStateData: statsFastFirstPaint });
    buildPerf.snapshot = Math.round((performance.now() - snapshotStart) * 10) / 10;
    recordBuildPerfPhase("snapshot", snapshotStart, { preferStateData: !!statsFastFirstPaint });
    state.initialStyleSnapshot = buildInitialStyleSnapshot(snapshot);
    state.undoStack = [];
    syncUndoButtons();
    const viewCommitStart = performance.now();
    replaceRuntimeGraphData(snapshot, { reason: "build-graph-snapshot" });
    syncAnalysisSourceDataFromCurrent({ force: true, preferStateData: statsFastFirstPaint });
    rememberGraphPatchBase(snapshot);
    if (createView || !getActiveView()) {
      const view = buildViewFromSnapshot(snapshot, { title: viewTitle });
      addView(view, { activate: true, applyGraph: false });
    } else {
      updateCurrentViewSnapshot();
    }
    updateGraphStats();
    buildPerf.viewCommit = Math.round((performance.now() - viewCommitStart) * 10) / 10;
    recordBuildPerfPhase("view-commit", viewCommitStart, { createView: !!createView });
    if (deferNetworkBuildSemantic) {
      await yieldBuildPerfPhase("yield-before-network-plan", { mode: networkBuildMode });
      if (isStaleBuild()) return;
    }

    const ms = Math.round(performance.now() - t0);
    state.lastBuildGraphPerf = {
      requestId: state.requestId || "",
      recordedAt: Date.now(),
      statsFastFirstPaint: !!statsFastFirstPaint,
      counts: { nodes: nodes.length, edges: edges.length },
      total: ms,
      perf: {
        ...buildPerf,
        stagedLabels: buildPerf.stagedLabels
          ? {
              ...buildPerf.stagedLabels,
              budget: buildPerf.stagedLabels.budget ? { ...buildPerf.stagedLabels.budget } : null,
            }
          : null,
        textInitial: buildPerf.textInitial ? { ...buildPerf.textInitial } : null,
      },
    };
    if (isStaleBuild()) return;
    setGraphLoading(false);
    updateLayoutButtons();
    if (state.layoutPreset === "network" && deferNetworkBuildSemantic) {
      const deferredBuildSeq = buildSeq;
      const deferredGraphMutationSeq = Number(state.graphMutationSeq || 0);
      const deferredCommandSeq = Number(state.layoutPresetCommandSeq || 0);
      if (
        deferredBuildSeq === Number(state.graphBuildSeq || 0) &&
        deferredGraphMutationSeq === Number(state.graphMutationSeq || 0)
      ) {
        const lightweightSemanticProjection = buildNetworkLightweightSemanticProjection(nodes, {
          reason: "build-graph-lightweight-semantic",
          mode: networkBuildMode,
        });
        void scheduleNetworkTopologyPlan({
          reason: "build-graph-lightweight-semantic",
          semanticProjection: lightweightSemanticProjection,
          commandSeq: deferredCommandSeq,
        });
      }
    } else if (state.layoutPreset === "network") {
      void scheduleNetworkTopologyPlan({
        reason: "build-graph-snapshot",
        semanticProjection: layoutSemanticProjection,
        commandSeq: Number(state.layoutPresetCommandSeq || 0),
      });
    }
    scheduleProjectionLayoutIndexSync("build-graph", { delayMs: 120 });
    if (state.debugPerf || ms >= 400) {
      log("INFO", "build graph perf", {
        total: ms,
        statsFastFirstPaint: !!statsFastFirstPaint,
        counts: { nodes: nodes.length, edges: edges.length },
        perf: buildPerf,
      });
    }
    toast(`图谱已生成 · ${nodes.length} 节点 / ${edges.length} 边 · ${ms}ms`, "ok");
  }

  function clearGraph() {
    cancelGraphTransientTasks("clear-graph");
    resetAnalysisFilterTools({ clearRange: true, clearSource: true, closePopover: true });
    clearRuntimeGraphData({ reason: "clear-graph-state" });
    clearProjectionPointLayer();
    state.graphTier = "small";
    state.renderHints = buildDefaultGraphRenderHints("small", 0, 0, state.graphMode || "relation");
    state.networkLayout.activeMode = "full";
    state.networkLayout.hiddenNodeCount = 0;
    state.networkLayout.hiddenEdgeCount = 0;
    state.networkLayout.lastDecision = null;
    state.networkLayout.lastPlanSampleQuality = null;
    state.networkLayout.lastPlanCommunityQuality = [];
    state.networkLayout.lastViewportQuality = null;
    state.networkLayout.lastViewportRefine = null;
    state.networkLayout.lastCommunityQualityAttempt = null;
    state.networkLayout.lastCommunityQualityDrop = null;
    state.networkLayout.lastCommunityQualityStaleDrop = null;
    state.networkLayout.communityQualitySeq = Number(state.networkLayout.communityQualitySeq || 0) + 1;
    if (state.networkLayout.expandedCommunities instanceof Set) {
      state.networkLayout.expandedCommunities.clear();
    }
    state.initialStyleSnapshot = null;
    state.undoStack = [];
    syncUndoButtons();
    const view = getActiveView();
    if (view) {
      view.graph = { nodes: [], edges: [] };
      view.counts = { nodes: 0, edges: 0 };
    }
    updateGraphStats();
    try {
      const graph = ensureGraph();
      if (graph) graph.clear();
    } catch (e) {}
    syncAnalysisSourceDataFromCurrent({ force: true });
    publishGraphPatch({
      scope: "local-update",
      reason: "clear-graph",
      forceFull: true,
    });
    updateLayoutButtons();
    closeDrawer();
  }

  function fitGraph(options = {}) {
    const { force = false } = options || {};
    if (!force && (state.graphLoading || state.edgeBatching)) return;
    if (!force && (state.introAnimating || Date.now() < state.introSuppressUntil)) return;
    try {
      const graph = ensureGraph();
      if (!graph) return;
      try {
        graph.fitView(60);
      } catch (e) {
        log("WARN", "fitView failed", e);
      }
      const z = graph.getZoom ? graph.getZoom() : 1;
      const fitMode = String(state.networkLayout?.lastPlanMode || state.graphTier || "").trim().toLowerCase();
      const maxZoom =
        state.layoutPreset === "network" && isLargeNetworkTopologyMode(fitMode)
          ? fitMode === "large"
            ? 0.96
            : 0.9
          : 1.2;
      const clamped = Math.min(Math.max(z, 0.05), maxZoom);
      if (Math.abs(clamped - z) > 0.001 && graph.zoomTo) {
        graph.zoomTo(clamped);
      }
      if (graph.fitCenter) graph.fitCenter();
      if (state.debugPerf) log("INFO", "fitGraph", { zoom: clamped });
      updateLodByZoom(graph, { force: true });
      if (graph.paint) graph.paint();
    } catch (e) {}
  }

  function scheduleFit(retries = 8) {
    if (state.introAnimating || Date.now() < state.introSuppressUntil) {
      state.pendingFit = true;
      if (retries > 0) setTimeout(() => scheduleFit(retries - 1), 80);
      return;
    }
    if (state.graphLoading || state.edgeBatching) {
      state.pendingFit = true;
      if (retries > 0) setTimeout(() => scheduleFit(retries - 1), 120);
      return;
    }
    const container = $("#graphContainer");
    if (!container) return;
    const w = container.clientWidth || 0;
    const h = container.clientHeight || 0;
    if (w > 10 && h > 10) {
      try {
        const graph = ensureGraph();
        if (graph) graph.changeSize(w, h);
      } catch (e) {}
      fitGraph();
      state.pendingFit = false;
      return;
    }
    state.pendingFit = true;
    if (retries <= 0) return;
    setTimeout(() => scheduleFit(retries - 1), 80);
  }

  function getGraphContainerSize() {
    const container = $("#graphContainer");
    if (!container) return { w: 0, h: 0 };
    const w = container.clientWidth || 0;
    const h = container.clientHeight || 0;
    if (w > 10 && h > 10) {
      state.lastGraphContainerSize = { w, h };
    }
    return { w, h };
  }

  function pickGraphRenderFallbackSize(minW = 320, minH = 200) {
    const candidates = [];
    const pushRect = (source, el, { padW = 0, padH = 0 } = {}) => {
      if (!el || typeof el.getBoundingClientRect !== "function") return;
      const rect = el.getBoundingClientRect();
      const w = Math.max(0, Math.round(rect.width - padW));
      const h = Math.max(0, Math.round(rect.height - padH));
      if (w > 0 && h > 0) {
        candidates.push({ source, w, h });
      }
    };
    pushRect("wrap", $("#graphWrap"));
    pushRect("card", $("#graphCard"), { padW: 12, padH: 12 });
    pushRect("right", $(".right"), { padW: 24, padH: 24 });
    if ((state.lastGraphContainerSize?.w || 0) > 0 && (state.lastGraphContainerSize?.h || 0) > 0) {
      candidates.push({
        source: "cached",
        w: Math.round(state.lastGraphContainerSize.w),
        h: Math.round(state.lastGraphContainerSize.h),
      });
    }
    if (window.innerWidth > 0 && window.innerHeight > 0) {
      candidates.push({
        source: "window",
        w: Math.max(minW, Math.round(window.innerWidth - 420)),
        h: Math.max(minH, Math.round(window.innerHeight - 220)),
      });
    }
    if (!candidates.length) return null;
    const ready = candidates.find((item) => item.w >= minW && item.h >= minH);
    if (ready) return ready;
    return candidates.sort((a, b) => b.w * b.h - a.w * a.h)[0] || null;
  }

  function getGraphDataBounds(nodes) {
    const summary = summarizeLayout(nodes || []);
    if (!summary.finite || summary.minX == null || summary.maxX == null) return null;
    return summary;
  }

  function waitForGraphContainerSize(minW = 320, minH = 200, retries = 12, delay = 80) {
    return new Promise((resolve) => {
      const tick = (left) => {
        const { w, h } = getGraphContainerSize();
        if (w >= minW && h >= minH) {
          resolve({ w, h, ready: true });
          return;
        }
        if (left <= 0) {
          resolve({ w, h, ready: false });
          return;
        }
        setTimeout(() => tick(left - 1), delay);
      };
      tick(retries);
    });
  }

  async function resolveGraphRenderSize({
    minW = 320,
    minH = 200,
    retries = 12,
    delay = 80,
    preferFast = false,
  } = {}) {
    const immediate = getGraphContainerSize();
    if (immediate.w >= minW && immediate.h >= minH) {
      return { w: immediate.w, h: immediate.h, ready: true, source: "container" };
    }
    const fallback = pickGraphRenderFallbackSize(minW, minH);
    if (preferFast && fallback && fallback.w > 0 && fallback.h > 0) {
      return { w: fallback.w, h: fallback.h, ready: false, source: fallback.source || "fallback" };
    }
    const waited = await waitForGraphContainerSize(minW, minH, retries, delay);
    if (waited.w >= minW && waited.h >= minH) {
      return { w: waited.w, h: waited.h, ready: true, source: "container" };
    }
    if (fallback && fallback.w > 0 && fallback.h > 0) {
      return { w: fallback.w, h: fallback.h, ready: false, source: fallback.source || "fallback" };
    }
    return { w: waited.w || 0, h: waited.h || 0, ready: false, source: "unresolved" };
  }

  function finalizeView({ fit = true } = {}) {
    if (fit) scheduleFit();
    focusGraphNode();
  }

  function easeInOutCubic(t) {
    return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
  }

  function easeOutCubic(t) {
    return 1 - Math.pow(1 - t, 3);
  }

  function easeInCubic(t) {
    return t * t * t;
  }

  function resolveLayoutEasing(easingName) {
    const raw = String(easingName || GRAPH_LAYOUT_CONFIG.ANIM_EASING || "").trim().toLowerCase();
    if (raw === "linear") return (t) => t;
    if (raw === "easeoutcubic" || raw === "ease-out-cubic") return easeOutCubic;
    if (raw === "easeincubic" || raw === "ease-in-cubic") return easeInCubic;
    return easeInOutCubic;
  }

  function animateNodesToTargets(
    graph,
    targets,
    {
      duration = GRAPH_LAYOUT_CONFIG.ANIM_DURATION,
      finalize = true,
      fit = false,
      onComplete,
      onProgress,
      pinMap,
      startPositions = null,
      easing = null,
      labelLagMs = GRAPH_LAYOUT_CONFIG.LABEL_LAG_MS,
    } = {}
  ) {
    if (!graph) return;
    const animSeq = ++state.introAnimSeq;
    const easingFn = resolveLayoutEasing(easing);
    const nodes = graph.getNodes ? graph.getNodes() : [];
    const startMap = new Map();
    nodes.forEach((node) => {
      const model = node.getModel() || {};
      const id = model.id;
      if (!id) return;
      if (startPositions instanceof Map) {
        const start = startPositions.get(id);
        if (start && Number.isFinite(start.x) && Number.isFinite(start.y)) {
          startMap.set(id, { x: Number(start.x), y: Number(start.y) });
          return;
        }
      }
      startMap.set(id, { x: model.x || 0, y: model.y || 0 });
    });
    const t0 = performance.now();
    state.introAnimating = true;
    const durationMs = Math.max(80, Number(duration) || GRAPH_LAYOUT_CONFIG.ANIM_DURATION);
    const tick = (now) => {
      if (animSeq !== state.introAnimSeq) return;
      const p = Math.min(1, (now - t0) / durationMs);
      const k = easingFn(p);
      graph.setAutoPaint(false);
      nodes.forEach((node) => {
        const model = node.getModel() || {};
        const id = model.id;
        const start = startMap.get(id);
        const target = targets.get(id);
        if (start && target) {
          model.x = start.x + (target.x - start.x) * k;
          model.y = start.y + (target.y - start.y) * k;
          return;
        }
        if (pinMap && pinMap.has(id)) {
          const pinned = pinMap.get(id);
          if (pinned && Number.isFinite(pinned.x)) model.x = pinned.x;
          if (pinned && Number.isFinite(pinned.y)) model.y = pinned.y;
        }
      });
      if (typeof onProgress === "function") {
        try {
          onProgress({ p, k, graph, nodes });
        } catch (e) {}
      }
      graph.refreshPositions();
      graph.setAutoPaint(true);
      graph.paint();
      if (p < 1) {
        requestAnimationFrame(tick);
      } else {
        if (animSeq !== state.introAnimSeq) return;
        state.introAnimating = false;
        state.introSuppressUntil = 0;
        const lag = Math.max(0, Number(labelLagMs) || 0);
        if (lag > 0) {
          setTimeout(() => {
            if (animSeq !== state.introAnimSeq) return;
            try {
              graph.refreshPositions?.();
              graph.paint?.();
            } catch (e) {}
          }, lag);
        }
        updateMinimap();
        if (typeof onComplete === "function") {
          try {
            onComplete();
          } catch (e) {}
        }
        if (finalize) finalizeView({ fit });
      }
    };
    requestAnimationFrame(tick);
  }

  function relayoutGraph() {
    try {
      setLayoutPreset(state.layoutPreset || "compact", { animate: true });
    } catch (e) {}
  }

  function toggleLayoutEdgeRouting(mode) {
    if (mode !== "hierarchy" && mode !== "flow") return;
    const next = !state.layoutEdgeRouting?.[mode];
    state.layoutEdgeRouting = { ...(state.layoutEdgeRouting || {}), [mode]: next };
    if (state.graph.data.nodes.length) {
      try {
        setLayoutPreset(state.layoutPreset || mode, { animate: false, markSelected: true });
      } catch (e) {}
    }
    const text = mode === "hierarchy" ? "层级图" : "流向图";
    toast(`${text}${next ? "已开启" : "已关闭"}折线`, "ok");
  }

  function buildLayoutEdgeRoutingMenuItems(mode) {
    if (mode !== "hierarchy" && mode !== "flow") return [];
    const enabled = !!state.layoutEdgeRouting?.[mode];
    return [
      {
        label: enabled ? "取消折线" : "添加折线",
        action: () => toggleLayoutEdgeRouting(mode),
      },
    ];
  }

  function buildGroupOpsMenuItems() {
    return [
      {
        label: "交集",
        action: () => applyGroupOperation("intersection"),
      },
      {
        label: "差集",
        action: () => applyGroupOperation("difference"),
      },
    ];
  }

  function buildMergeMenuItems() {
    return [{ label: "净值合并", action: () => void mergeSameNameNodesNet() }];
  }

  function openLayoutEdgeRoutingMenu(mode, anchorEl) {
    if (!anchorEl) return;
    if (mode !== "hierarchy" && mode !== "flow") return;
    const rect = anchorEl.getBoundingClientRect();
    openContextMenu(rect.left, rect.bottom + 6, buildLayoutEdgeRoutingMenuItems(mode));
  }

  function normalizeAnchorRect(anchorRect) {
    const rect = anchorRect && typeof anchorRect === "object" ? anchorRect : null;
    if (!rect) return null;
    const left = Number(rect.left);
    const top = Number(rect.top);
    const right = Number(rect.right);
    const bottom = Number(rect.bottom);
    const width = Number(rect.width);
    const height = Number(rect.height);
    if (![left, top, right, bottom, width, height].every((value) => Number.isFinite(value))) {
      return null;
    }
    return { left, top, right, bottom, width, height };
  }

  function createContextMenuAnchorRectFromEvent(event) {
    const clientX = Number(event?.clientX);
    const clientY = Number(event?.clientY);
    if (Number.isFinite(clientX) && Number.isFinite(clientY) && (clientX !== 0 || clientY !== 0)) {
      return {
        left: clientX,
        top: clientY,
        right: clientX,
        bottom: clientY,
        width: 0,
        height: 0,
      };
    }
    const currentTargetRect =
      event?.currentTarget && typeof event.currentTarget.getBoundingClientRect === "function"
        ? normalizeAnchorRect(event.currentTarget.getBoundingClientRect())
        : null;
    if (currentTargetRect) return currentTargetRect;
    return event?.target && typeof event.target.getBoundingClientRect === "function"
      ? normalizeAnchorRect(event.target.getBoundingClientRect())
      : null;
  }

  function createVirtualAnchor(anchorRect, id = "") {
    const rect = normalizeAnchorRect(anchorRect);
    if (!rect) return null;
    return {
      id: String(id || ""),
      classList: {
        add() {},
        remove() {},
      },
      setAttribute() {},
      contains() {
        return false;
      },
      getBoundingClientRect() {
        return {
          ...rect,
          x: rect.left,
          y: rect.top,
          toJSON() {
            return { ...rect, x: rect.left, y: rect.top };
          },
        };
      },
    };
  }

  function eventComposedPathContainsClass(event, className) {
    if (!event || typeof event.composedPath !== "function") return false;
    const path = event.composedPath();
    if (!Array.isArray(path)) return false;
    return path.some((node) => node?.classList?.contains?.(className));
  }

  function isReactOverlayEvent(event) {
    return eventComposedPathContainsClass(event, "analytix-react-overlay-panel");
  }

  function openRibbonPopoverFromRect(anchorRect, anchorId, builder) {
    const anchor = createVirtualAnchor(anchorRect, anchorId);
    if (!anchor) return;
    toggleRibbonPopover(anchor, builder);
  }

  function runLayoutPresetCommand(preset) {
    const nextPreset = String(preset || "compact").trim().toLowerCase();
    if (!nextPreset) return;
    const needsRecompute = !!state.layoutTopologyDirty || nextPreset === "compact";
    const isSame = state.layoutPreset === nextPreset && state.layoutSelected && !needsRecompute;
    const canToggleDirection = nextPreset === "hierarchy" || nextPreset === "flow";
    const before = summarizeLayout(state.graph.data.nodes || []);
    const action = needsRecompute
      ? "recompute-after-mutation"
      : isSame
      ? canToggleDirection
        ? "toggle-direction"
        : "reapply"
      : "apply";
    if (nextPreset === "hierarchy") {
      if (isSame) {
        state.layoutDirection.hierarchy = state.layoutDirection.hierarchy === "down" ? "up" : "down";
      } else {
        state.layoutDirection.hierarchy = "down";
      }
    }
    if (nextPreset === "flow") {
      if (isSame) {
        state.layoutDirection.flow = state.layoutDirection.flow === "right" ? "left" : "right";
      } else {
        state.layoutDirection.flow = "right";
      }
    }
    logLayoutSwitchBanner(nextPreset, state.layoutPreset);
    logGraphTrace("layout-button", {
      view: nextPreset,
      action,
      direction: { ...state.layoutDirection },
      counts: {
        nodes: state.graph.data?.nodes?.length || 0,
        edges: state.graph.data?.edges?.length || 0,
      },
      before,
    });
    const needsSemanticProjection =
      nextPreset === "compact" || nextPreset === "network" || nextPreset === "relation";
    const nodes = Array.isArray(state.graph?.data?.nodes) ? state.graph.data.nodes : [];
    const edges = Array.isArray(state.graph?.data?.edges) ? state.graph.data.edges : [];
    const commandSeq = Number(state.layoutPresetCommandSeq || 0) + 1;
    state.layoutPresetCommandSeq = commandSeq;
    if (nextPreset === "network") {
      resetNetworkRuntimePerf("layout-button", {
        commandSeq,
        nodes: nodes.length,
        edges: edges.length,
        beforePreset: before,
      });
    }
    if (!needsSemanticProjection || !nodes.length || !edges.length) {
      setLayoutPreset(nextPreset, { animate: true, markSelected: true, forceRecompute: true });
      return;
    }
    if (nextPreset === "network" && !state.layoutTopologyDirty) {
      const focusId = resolveFocusIdFromNodes(nodes, state.focusId, state.focusName);
      const graphSignature = buildNetworkPlanGraphSignature(nodes, edges);
      const networkTopologyMode = resolveNetworkTopologyModeForGraph(nodes, edges);
      if (isLargeNetworkTopologyMode(networkTopologyMode)) {
        const cachedTopologyPlan = readLatestNetworkTopologyPlanCacheByGraphSignature(graphSignature);
        if (cachedTopologyPlan?.plan) {
          state.layoutPreset = "network";
          state.layoutSelected = true;
          updateLayoutButtons();
          recordNetworkRuntimePerfEvent("preset-immediate", {
            reason: "layout-button-topology-cache",
            commandSeq,
            nodes: nodes.length,
            edges: edges.length,
          });
          if (
            applyNetworkTopologyPlan(cachedTopologyPlan.plan, {
              reason: "network-topology-plan-cache-hit",
              cacheKey: cachedTopologyPlan.key,
            })
          ) {
            return;
          }
        }
      }
      if (!isLargeNetworkTopologyMode(networkTopologyMode)) {
        const layoutCacheKey = layoutPlanCacheController.makeCacheKey("network", focusId, nodes, edges);
        if (layoutCacheKey && layoutPlanCacheController.readCache(layoutCacheKey)) {
          setLayoutPreset(nextPreset, { animate: true, markSelected: true, forceRecompute: false });
          return;
        }
        void layoutPlanCacheController.prepareCacheKey("network", focusId, nodes, edges);
      }
      state.layoutPreset = "network";
      state.layoutSelected = true;
      updateLayoutButtons();
      recordNetworkRuntimePerfEvent("preset-immediate", {
        reason: "layout-button-pre-semantic",
        commandSeq,
        nodes: nodes.length,
        edges: edges.length,
      });
      if (isLargeNetworkTopologyMode(networkTopologyMode)) {
        const lightweightSemanticProjection = buildNetworkLightweightSemanticProjection(nodes, {
          reason: "layout-button-lightweight-semantic",
          mode: networkTopologyMode,
        });
        setTimeout(() => {
          if (
            Number(state.layoutPresetCommandSeq || 0) !== commandSeq ||
            state.layoutPreset !== "network" ||
            state.graph?.data?.nodes !== nodes ||
            state.graph?.data?.edges !== edges
          ) {
            recordNetworkRuntimePerfEvent("preset-large-apply-skipped", {
              reason: "layout-button-stale-large",
              commandSeq,
              nodes: nodes.length,
              edges: edges.length,
            });
            return;
          }
          state.layoutTopologyDirty = false;
          state.networkLayout.activeMode = "full";
          state.networkLayout.hiddenNodeCount = 0;
          state.networkLayout.hiddenEdgeCount = 0;
          recordNetworkRuntimePerfEvent("preset-applied", {
            reason: "layout-button-large-fast-path",
            commandSeq,
            forceRecompute: true,
          });
          if (nodes.length < 180 || nodes.length > 220) {
            stabilizeNetworkRuntimeForSuppressedTopologyPlan("layout-button-large-fast-path");
          }
          void scheduleNetworkTopologyPlan({
            reason: "layout-button-lightweight-semantic",
            semanticProjection: lightweightSemanticProjection,
            commandSeq,
          });
        }, 0);
        return;
      }
      if (nodes.length <= 20 && edges.length <= 80) {
        const nodeRef = nodes;
        const edgeRef = edges;
        setTimeout(() => {
          if (
            Number(state.layoutPresetCommandSeq || 0) !== commandSeq ||
            state.layoutPreset !== "network" ||
            state.graph?.data?.nodes !== nodeRef ||
            state.graph?.data?.edges !== edgeRef
          ) {
            recordNetworkRuntimePerfEvent("preset-small-preview-skipped", {
              reason: "layout-button-stale-preview",
              commandSeq,
              nodes: nodeRef.length,
              edges: edgeRef.length,
            });
            return;
          }
          void (async () => {
            let semanticProjection = null;
            try {
              semanticProjection = await prepareLayoutSemanticProjection(nodeRef, edgeRef, nextPreset, {
                reason: "layout-button-small-preview",
                commitSemanticState: false,
              });
            } catch (error) {
              try {
                log("WARN", "layout button semantic projection failed", {
                  preset: nextPreset,
                  message: String(error?.message || error),
                });
              } catch (_error) {}
            }
            if (
              Number(state.layoutPresetCommandSeq || 0) !== commandSeq ||
              state.layoutPreset !== "network" ||
              state.graph?.data?.nodes !== nodeRef ||
              state.graph?.data?.edges !== edgeRef
            ) {
              recordNetworkRuntimePerfEvent("preset-small-preview-skipped", {
                reason: "layout-button-stale-after-semantic",
                commandSeq,
                nodes: nodeRef.length,
                edges: edgeRef.length,
              });
              return;
            }
            setLayoutPreset(nextPreset, {
              animate: true,
              markSelected: true,
              forceRecompute: true,
              semanticProjection,
            });
          })();
        }, 180);
        return;
      }
    }
    void (async () => {
      let semanticProjection = null;
      try {
        semanticProjection = await prepareLayoutSemanticProjection(nodes, edges, nextPreset, {
          reason: "layout-button",
          commitSemanticState: false,
        });
      } catch (error) {
        try {
          log("WARN", "layout button semantic projection failed", {
            preset: nextPreset,
            message: String(error?.message || error),
          });
        } catch (_error) {}
      }
      if (Number(state.layoutPresetCommandSeq || 0) !== commandSeq) return;
      setLayoutPreset(nextPreset, {
        animate: true,
        markSelected: true,
        forceRecompute: true,
        semanticProjection,
      });
    })();
  }

  function reportGraphSearchError(error) {
    const message = String(error?.message || error || "").trim() || "图谱搜索失败";
    try {
      log("WARN", "graph search failed", { message });
    } catch (_error) {}
    toast(message, "danger");
  }

  async function searchAndFocusGraph() {
    const q = String(state.graphSearchQuery || getGraphSearchInput()?.value || "").trim();
    if (!q) return;
    const graph = ensureGraph();
    if (!graph) return;
    const resolveSearchTarget = async () => {
      const items = Array.isArray(graph.getNodes?.()) ? graph.getNodes() : [];
      const result = await projectGraphSearch({
        nodes: items,
        query: q,
        limit: 1,
      });
      const targetId = String(result?.firstNodeId || "").trim();
      if (!targetId) return null;
      const direct = graph.findById?.(targetId);
      if (direct) return direct;
      return items.find((item) => String(item?.getModel?.()?.id || "").trim() === targetId) || null;
    };
    let target = null;
    try {
      target = await resolveSearchTarget();
    } catch (error) {
      reportGraphSearchError(error);
      return;
    }
    if (!target && canExpandCurrentProjection()) {
      const expanded = await requestProjectionExpand(
        {
          searchQuery: q,
          searchLimit: 48,
        },
        {
          reason: "graph-search-hidden",
          preserveViewport: true,
        }
      );
      if (expanded) {
        try {
          target = await resolveSearchTarget();
        } catch (error) {
          reportGraphSearchError(error);
          return;
        }
      }
    }
    if (!target) {
      toast("未找到节点", "warn");
      return;
    }
    const model = target.getModel() || {};
    if (canExpandCurrentProjection() && isCollapsedProjectionNode(model)) {
      const expanded = await expandProjectionNode(model, { reason: "graph-search" });
      if (expanded) {
        const focused = focusProjectionExpandedNode(String(model?.id || "").trim(), { openDrawer: true });
        if (!focused && isClusterProjectionNode(model)) {
          toast("已展开匹配簇", "ok");
        }
        return;
      }
    }
    try {
      graph.focusItem(target, true, { easing: "easeCubic", duration: 240 });
      clearGraphSelection(graph);
      graph.setItemState(target, "selected", true);
      syncActiveSelection(graph, target);
      openNodeDetailDrawer(model, graph);
    } catch (e) {}
  }

  // ===== Events =====
  function bindEvents() {
    on($("#tabByName"), "click", () => setTab("byName"));
    on($("#tabByCard"), "click", () => setTab("byCard"));

    on($("#gapToggle"), "click", () => applyLeftCollapsed(!state.leftCollapsed));

    on($("#leftSearch"), "input", (e) => {
      setSearchQuery(e.target.value);
    });
    on($("#leftSearchClear"), "click", () => {
      setSearchQuery("");
      $("#leftSearch").focus();
    });

    on($("#tree"), "click", (e) => {
      const head = e.target.closest(".gHead");
      const item = e.target.closest(".item");
      if (head) {
        const gid = head.dataset.gid;
        const onCb = e.target.closest(".cb");
        const onArrow = e.target.closest(".gArrow");
        if (onCb) return toggleGroupSelect(gid);
        if (onArrow) return toggleGroup(gid);
        return toggleGroup(gid);
      }
      if (item) return toggleItemSelect(item.dataset.iid);
    });

    on($("#btnSelectAll"), "click", () => {
      state.treeData.forEach((g) => (g.items || []).forEach((it) => state.selected.add(it.id)));
      updateSelectionSummary();
      renderTree();
    });
    on($("#btnClearAll"), "click", () => {
      state.selected = new Set();
      updateSelectionSummary();
      renderTree();
    });

    $$(".segMini").forEach((btn) => {
      btn.addEventListener("click", () => {
        setDirectionFilter(btn.dataset.dir || "all");
      });
    });

    on($("#hop"), "input", (e) => {
      setHopFilter(e.target.value);
    });
    on($("#minAmount"), "blur", () => {
      setMinAmountFilter(($("#minAmount").value || "").trim());
    });
    on($("#maxEdges"), "blur", () => {
      setMaxEdgesFilter(($("#maxEdges").value || "").trim());
    });

    on($("#btnBuild"), "click", () => {
      setEdgeDirectionLocked(false);
      buildGraph();
    });
    on($("#btnClearGraph"), "click", () => clearGraph());
    on($("#btnMergeNodes"), "click", () => void mergeSameNameNodes());
    on($("#btnMergeNodes"), "contextmenu", (e) => {
      e.preventDefault();
      const rect = createContextMenuAnchorRectFromEvent(e);
      if (!rect) return;
      if (isReactShellSubscribed()) {
        toggleReactToolbarMenu("ops-merge", rect, buildMergeMenuItems());
        return;
      }
      openContextMenu(rect.left, rect.bottom + 6, buildMergeMenuItems());
    });
    on($("#btnFit"), "click", () => fitGraph());
    on($("#btnRelayout"), "click", () => relayoutGraph());
    on($("#btnUndo"), "click", () => {
      const stack = state.undoStack || [];
      const snapshot = stack.pop();
      state.undoStack = stack;
      if (!snapshot) return;
      applySnapshotState(snapshot);
    });
    on($("#btnEdgeDetail"), "click", () => {
      state.edgeDetail = !state.edgeDetail;
      applyEdgeDetailLabels(state.edgeDetail);
      syncEdgeDetailButton();
      updateCurrentViewSnapshot();
    });
    on($("#btnRedo"), "click", () => {
      resetToInitialStyles();
    });

    on($("#btnExport"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactExportOverlay(e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () => buildExportPopover());
    });

    const adjustFontSize = (delta) => {
      ensureRibbonStyle();
      const current = clampNumber(state.ribbonStyle.fontSize ?? 13, 8, 36);
      updateRibbonState({ fontSize: clampNumber(current + delta, 8, 36) });
    };

    on($("#btnFontFamily"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-font-family", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () =>
        buildListPopover(FONT_LIST, state.ribbonStyle?.fontFamily, (val) => updateRibbonState({ fontFamily: val }))
      );
    });
    on($("#btnFontSize"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-font-size", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () =>
        buildListPopover(FONT_SIZE_LIST, state.ribbonStyle?.fontSize, (val) => updateRibbonState({ fontSize: val }))
      );
    });
    on($("#btnFontInc"), "click", () => adjustFontSize(1));
    on($("#btnFontDec"), "click", () => adjustFontSize(-1));

    [
      ["btnBold", "fontBold"],
      ["btnItalic", "fontItalic"],
      ["btnUnderline", "fontUnderline"],
      ["btnShadow", "fontShadow"],
    ].forEach(([id, key]) => {
      on($("#" + id), "click", () => {
        ensureRibbonStyle();
        updateRibbonState({ [key]: !state.ribbonStyle[key] });
      });
    });

    on($("#btnTextColor"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-text-color", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () => buildColorPopover("textColor"));
    });
    on($("#btnOutlineColor"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-outline-color", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () => buildColorPopover("outlineColor"));
    });
    on($("#btnLineColor"), "click", (e) => {
      toggleRibbonPopover(e.currentTarget, () => buildColorPopover("outlineColor"));
    });
    on($("#btnNodeFill"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-node-fill", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () => buildColorPopover("nodeFill"));
    });

    on($("#btnLineStyle"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-line-style", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () =>
        buildLineStylePopover(LINE_STYLE_OPTIONS, state.ribbonStyle?.edgeDash, (val) => updateRibbonState({ edgeDash: val }))
      );
    });
    on($("#btnLineWidth"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-line-width", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () =>
        buildListPopover(LINE_WIDTH_OPTIONS, state.ribbonStyle?.edgeWidth, (val) => updateRibbonState({ edgeWidth: Number(val) }))
      );
    });
    on($("#btnLineArrow"), "click", (e) => {
      const graph = ensureGraph();
      if (!canEditEdgeDirection(graph)) {
        toast("统计分析展示模式下禁止修改方向", "warn");
        return;
      }
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-line-arrow", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () =>
        buildArrowPopover(ARROW_OPTIONS, state.ribbonStyle?.edgeArrow, (val) => updateRibbonState({ edgeArrow: val }))
      );
    });

    on($("#btnNodeShape"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-node-shape", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () =>
        buildShapePopover(NODE_SHAPE_OPTIONS, state.ribbonStyle?.nodeShape, (val) => updateRibbonState({ nodeShape: val }))
      );
    });
    on($("#btnIconSize"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-icon-size", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () =>
        buildListPopover(NODE_SIZE_OPTIONS, state.ribbonStyle?.nodeSize, (val) => updateRibbonState({ nodeSize: Number(val) }))
      );
    });
    on($("#btnIconLibrary"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactStylePopover("style-icon-library", e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () => buildIconLibraryPopover());
    });
    on($("#btnNodeNew"), "click", () => {
      setCreateNodeMode(!state.createNodeMode);
    });
    on($("#btnNodeLink"), "click", () => {
      createLinksFromSelection();
    });

    $$(".layoutBtn").forEach((btn) => {
      btn.addEventListener("click", () => {
        runLayoutPresetCommand(btn.dataset.layout || "compact");
      });
    });

    const bindLayoutContext = (id, mode) => {
      const btn = $("#" + id);
      if (!btn) return;
      btn.addEventListener("contextmenu", (e) => {
        e.preventDefault();
        const rect = createContextMenuAnchorRectFromEvent(e);
        if (!rect) return;
        if (isReactShellSubscribed()) {
          toggleReactToolbarMenu(`layout-edge-routing-${mode}`, rect, buildLayoutEdgeRoutingMenuItems(mode));
          return;
        }
        openContextMenu(rect.left, rect.bottom + 6, buildLayoutEdgeRoutingMenuItems(mode));
      });
    };
    bindLayoutContext("btnLayoutHierarchy", "hierarchy");
    bindLayoutContext("btnLayoutFlow", "flow");

    on($("#btnAnalysisFilter"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactFilterOverlay(e.currentTarget.getBoundingClientRect());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () => buildAmountFilterPopover());
    });

    on($("#btnAnalysisCollapse"), "click", () => {
      toggleAnalysisCollapse();
    });

    [1, 2, 3].forEach((idx) => {
      on($("#btnGroup" + idx), "click", () => assignGroupTag(idx));
    });

    on($("#btnGroupOps"), "click", (e) => {
      if (isReactShellSubscribed()) {
        toggleReactToolbarMenu("analysis-group-ops", e.currentTarget.getBoundingClientRect(), buildGroupOpsMenuItems());
        return;
      }
      toggleRibbonPopover(e.currentTarget, () =>
        buildListPopover(
          [
            { label: "交集", value: "intersection" },
            { label: "差集", value: "difference" },
          ],
          "",
          (val) => applyGroupOperation(val)
        )
      );
    });

    on($("#btnEncodeWidth"), "click", () => {
      recordUndoSnapshot();
      state.analysis.encodeWidth = !state.analysis.encodeWidth;
      applyEdgeWidthEncoding(state.analysis.encodeWidth);
      syncAnalysisButtons();
    });
    on($("#btnEncodeColor"), "click", () => {
      recordUndoSnapshot();
      state.analysis.encodeColor = !state.analysis.encodeColor;
      applyEdgeColorEncoding(state.analysis.encodeColor);
      syncAnalysisButtons();
    });
    on($("#btnNodeScale"), "click", () => {
      recordUndoSnapshot();
      state.analysis.nodeScale = !state.analysis.nodeScale;
      applyNodeScaleEncoding(state.analysis.nodeScale);
      syncAnalysisButtons();
    });

    on($("#btnAddView"), "click", () => {
      const view = buildEmptyView(nextViewTitle());
      addView(view, { activate: true, applyGraph: true });
    });

    on($("#btnViewSave"), "click", () => saveCurrentView());
    on($("#btnViewDelete"), "click", () => clearGraph());
    on($("#btnViewExport"), "click", () => exportCurrentView("pdf"));
    on($("#btnViewExport"), "contextmenu", (e) => {
      e.preventDefault();
      const rect = e.currentTarget.getBoundingClientRect();
      openContextMenu(rect.left, rect.bottom + 6, [
        { label: "导出 PDF", action: () => exportCurrentView("pdf") },
        { label: "导出 JPG", action: () => exportCurrentView("jpg") },
        { label: "导出 PNG", action: () => exportCurrentView("png") },
      ]);
    });

    on($("#graphSearch"), "keydown", (e) => {
      if (e.key === "Enter") searchAndFocusGraph();
    });
    on($("#graphSearch"), "input", (e) => {
      syncGraphSearchQuery(e.target.value);
    });
    on($("#graphSearchClear"), "click", () => {
      syncGraphSearchQuery("");
      if (!isReactShellSubscribed()) {
        getGraphSearchInput()?.focus();
      }
    });

    on($("#drawerClose"), "click", () => closeDrawer());
    on($("#edgeLabelSave"), "click", () => commitEdgeLabel());
    on($("#edgeLabelCancel"), "click", () => closeEdgeLabelDialog());
    const edgeLabelMask = $("#edgeLabelModal");
    if (edgeLabelMask) {
      edgeLabelMask.addEventListener("click", (e) => {
        if (e.target === edgeLabelMask) closeEdgeLabelDialog();
      });
    }
    const edgeLabelInput = $("#edgeLabelInput");
    if (edgeLabelInput) {
      edgeLabelInput.addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          commitEdgeLabel();
        } else if (e.key === "Escape") {
          e.preventDefault();
          closeEdgeLabelDialog();
        }
      });
    }

    const edgeInfoPopover = $("#edgeInfoPopover");
    if (edgeInfoPopover) {
      edgeInfoPopover.addEventListener("click", (e) => {
        const trigger = e.target.closest("[data-action='open-edge-txn-detail']");
        if (!trigger) return;
        e.preventDefault();
        e.stopPropagation();
        const ctx = state.edgeInfoContext;
        if (!ctx || !ctx.model) return;
        closeEdgeInfoPopover();
        openEdgeTxnModal(ctx.model, ctx.lane, ctx.requestKey);
      });
      edgeInfoPopover.addEventListener(
        "wheel",
        (e) => {
          routeInlineInfoPopoverWheel(edgeInfoPopover, e, {
            listSelector: ".edgeTxnList",
            route: "edgePopover",
            handleWheelScroll,
            logWheelRoute,
          });
        },
        { passive: false, capture: true }
      );
    }

    const nodeInfoPopover = $("#nodeInfoPopover");
    if (nodeInfoPopover) {
      nodeInfoPopover.addEventListener(
        "wheel",
        (e) => {
          routeInlineInfoPopoverWheel(nodeInfoPopover, e, {
            listSelector: ".nodeInfoList",
            route: "nodePopover",
            handleWheelScroll,
            logWheelRoute,
          });
        },
        { passive: false, capture: true }
      );
    }

    on($("#viewCloseCancel"), "click", () => closeViewCloseModal(false));
    on($("#viewCloseConfirm"), "click", () => closeViewCloseModal(true));
    const viewCloseMask = $("#viewCloseModal");
    if (viewCloseMask) {
      viewCloseMask.addEventListener("click", (e) => {
        if (e.target === viewCloseMask) closeViewCloseModal(false);
      });
    }

    const graphWrap = $("#graphWrap");
    if (graphWrap) {
      graphWrap.addEventListener("contextmenu", (e) => {
        if (!state.createNodeMode) return;
        e.preventDefault();
        setCreateNodeMode(false);
        setGhostFloatVisible(false);
      });
    }

    document.addEventListener("click", (e) => {
      const insideReactOverlay = isReactOverlayEvent(e);
      const canCloseReactOverlay = Date.now() >= Number(state.reactOverlay?.suppressCloseUntil || 0);
      if (!insideReactOverlay && e.target.closest("#contextMenu") == null && canCloseReactOverlay) closeContextMenu();
      if (!insideReactOverlay && canCloseReactOverlay) {
        closeReactFilterOverlay();
        closeReactStylePopover();
        closeReactToolbarMenu();
        closeReactExportOverlay();
      }
      if (
        activePopoverAnchor &&
        !e.target.closest("#ribbonPopover") &&
        !activePopoverAnchor.contains(e.target)
      ) {
        closeRibbonPopover({ restoreFocus: false });
      }
      if (activeDrillPopover && !e.target.closest("#drillPopover")) closeDrillPopover();
      if (shouldCloseInfoPopover(activeNodeInfoPopover, e.target, "#nodeInfoPopover")) closeNodeInfoPopover();
      if (shouldCloseInfoPopover(activeEdgeInfoPopover, e.target, "#edgeInfoPopover")) closeEdgeInfoPopover();
      if (!insideReactOverlay && canCloseReactOverlay && state.reactOverlay?.nodeInfo?.open) closeNodeInfoPopover();
      if (!insideReactOverlay && canCloseReactOverlay && state.reactOverlay?.edgeInfo?.open) closeEdgeInfoPopover();
    });

    document.addEventListener(
      "wheel",
      (e) => {
        if (isTxnModalOpen()) {
          if (isReactShellSubscribed()) {
            const target = e.target;
            if (
              target instanceof Element &&
              target.closest(".analytix-react-txn-modal, .txnDetailDialogWrap, .txnModalWrap")
            ) {
              return;
            }
          }
          // Always consume wheel while modal is open to avoid graph zoom stealing input.
          e.preventDefault();
          e.stopPropagation();
          return;
        }

        if (routeDocumentInfoPopoverWheel(e, {
          activeEdgeInfoPopover,
          activeNodeInfoPopover,
          handleWheelScroll,
          logWheelRoute,
        })) {
          return;
        }
      },
      { passive: false, capture: true }
    );

    document.addEventListener("keydown", (e) => {
      updateModifierState(e);
      if (state.viewCloseResolver) {
        if (e.key === "Escape") {
          e.preventDefault();
          closeViewCloseModal(false);
          return;
        }
        if (e.key === "Enter") {
          e.preventDefault();
          closeViewCloseModal(true);
          return;
        }
        return;
      }
      if (e.key === "Escape" && isTxnModalOpen()) {
        e.preventDefault();
        closeTxnModal();
        return;
      }
      if (e.key === "Escape") {
        closeRibbonPopover();
        closeContextMenu();
        closeReactFilterOverlay({ sync: false });
        closeReactStylePopover({ sync: false });
        closeReactToolbarMenu({ sync: false });
        closeReactExportOverlay({ sync: false });
        closeDrillPopover();
        closeNodeInfoPopover();
        closeEdgeInfoPopover();
        closeEdgeLabelDialog();
        clearGraphSelection(state.graph.instance);
        closeDrawer();
        if (state.createNodeMode) setCreateNodeMode(false);
        setGhostFloatVisible(false);
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "a") {
        if (isEditableContext(e.target)) return;
        e.preventDefault();
        selectAllGraphItems();
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "c") {
        if (isEditableContext(e.target)) return;
        const selection = getSelectionState(state.graph.instance);
        if (selection.total > 0) {
          e.preventDefault();
          void copyGraphSelectionToClipboard();
          return;
        }
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "v") {
        if (isEditableContext(e.target)) return;
        e.preventDefault();
        void pasteGraphClipboardFromClipboard();
        return;
      }
      if (e.key === "Delete" || e.key === "Backspace") {
        if (isEditableContext(e.target)) return;
        if (deleteSelectedGraphItems()) {
          e.preventDefault();
          return;
        }
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "f") {
        e.preventDefault();
        if (isReactShellSubscribed()) {
          requestReactGraphSearchFocus();
        } else {
          getGraphSearchInput()?.focus();
        }
      }
    });

    document.addEventListener("keyup", (e) => {
      updateModifierState(e);
    });

    document.addEventListener("mousedown", (e) => {
      updateModifierState(e);
    });

    const ribbonPopover = $("#ribbonPopover");
    if (ribbonPopover) {
      ribbonPopover.tabIndex = -1;
      ribbonPopover.addEventListener("keydown", (e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          closeRibbonPopover();
          return;
        }
        if (isEditableContext(e.target)) {
          return;
        }
        if (e.key === "ArrowDown" || e.key === "ArrowRight") {
          e.preventDefault();
          moveRibbonPopoverFocus(ribbonPopover, 1);
        } else if (e.key === "ArrowUp" || e.key === "ArrowLeft") {
          e.preventDefault();
          moveRibbonPopoverFocus(ribbonPopover, -1);
        } else if (e.key === "Home") {
          const first = getPopoverFocusableElements(ribbonPopover)[0];
          if (first) {
            e.preventDefault();
            first.focus({ preventScroll: true });
          }
        } else if (e.key === "End") {
          const focusables = getPopoverFocusableElements(ribbonPopover);
          const last = focusables[focusables.length - 1];
          if (last) {
            e.preventDefault();
            last.focus({ preventScroll: true });
          }
        }
      });
    }

    const graphToolbar = $("#graphToolbar");
    if (graphToolbar && !graphToolbar.dataset.overflowBound) {
      graphToolbar.dataset.overflowBound = "1";
      graphToolbar.addEventListener("scroll", syncToolbarOverflowState, { passive: true });
      window.addEventListener("resize", scheduleToolbarOverflowSync);
      scheduleToolbarOverflowSync();
      window.setTimeout(syncToolbarOverflowState, 120);
    }

    document.addEventListener("mousemove", (e) => {
      state.lastPointer = { x: e.clientX, y: e.clientY };
      if (!state.createNodeMode) return;
      updateGhostFromClient(e.clientX, e.clientY);
    });

    window.addEventListener("blur", () => {
      state.modifiers.ctrl = false;
      state.modifiers.meta = false;
    });
  }

  function init() {
    if (state.inited) return;
    state.inited = true;
    bindEvents();
    syncToolbarButtonTitles();
    setTab("byName");
    setToolbarStyle(state.graphStyle);
    setEdgeDirectionLocked(false);
    setLayoutPreset(state.layoutPreset, { animate: false });
    ensureGraph();
    bindGraphThemePaletteSync();
    syncGraphThemePalette("init", { force: true });
    syncAnalysisButtons();
    syncEdgeDetailButton();
    syncUndoButtons();
    initBackend();
    ensureBackendReady();
    scheduleToolbarOverflowSync();
    setTimeout(() => syncCaseFromBackend("init-delay"), 200);
  }

  function buildShellToolbarCommands() {
    return flowShellToolbarCommandsAdapter.createFlowShellToolbarCommands({
      state,
      deps: {
        isReactShellSubscribed,
        triggerShellControl,
        toggleReactStylePopover,
        openRibbonPopoverFromRect,
        buildListPopover,
        buildColorPopover,
        buildLineStylePopover,
        buildArrowPopover,
        buildShapePopover,
        buildIconLibraryPopover,
        closeReactStylePopover,
        applyReactStylePopoverOption,
        applyReactStylePopoverColor,
        setReactStyleIconLibraryTab,
        setReactStyleIconLibraryQuery,
        applyReactStyleIconSymbol,
        ensureRibbonStyle,
        clampNumber,
        updateRibbonState,
        ensureGraph,
        canEditEdgeDirection,
        toast,
        setCreateNodeMode,
        createLinksFromSelection,
        runLayoutPresetCommand,
        toggleReactToolbarMenu,
        buildLayoutEdgeRoutingMenuItems,
        createVirtualAnchor,
        openLayoutEdgeRoutingMenu,
        mergeSameNameNodes,
        mergeSameNameNodesNet,
        normalizeAnchorRect,
        buildMergeMenuItems,
        openContextMenu,
        fitGraph,
        toggleReactExportOverlay,
        closeReactExportOverlay,
        scheduleShellStateSync,
        exportCurrentView,
        applyEdgeDetailLabels,
        syncEdgeDetailButton,
        updateCurrentViewSnapshot,
        resetToInitialStyles,
        applySnapshotState,
        syncGraphSearchQuery,
        searchAndFocusGraph,
        expandProjectionViewport,
        expandProjectionSelectionPath,
        FONT_LIST,
        FONT_SIZE_LIST,
        LINE_STYLE_OPTIONS,
        LINE_WIDTH_OPTIONS,
        ARROW_OPTIONS,
        NODE_SHAPE_OPTIONS,
        NODE_SIZE_OPTIONS,
      },
    });
  }

  function buildShellAnalysisCommands() {
    return flowShellAnalysisCommandsAdapter.createFlowShellAnalysisCommands({
      state,
      deps: {
        toggleReactFilterOverlay,
        closeReactFilterOverlay,
        applyAnalysisAmountFilterState,
        clearAnalysisAmountFilterState,
        toggleAnalysisCollapse,
        assignGroupTag,
        isReactShellSubscribed,
        toggleReactToolbarMenu,
        buildGroupOpsMenuItems,
        openRibbonPopoverFromRect,
        buildListPopover,
        applyGroupOperation,
        recordUndoSnapshot,
        applyEdgeWidthEncoding,
        applyEdgeColorEncoding,
        applyNodeScaleEncoding,
        syncAnalysisButtons,
      },
    });
  }

  function buildShellCanvasCommands() {
    return flowShellCanvasCommandsAdapter.createFlowShellCanvasCommands({
      deps: {
        ensureGraph,
        clearCanvasSelection,
        selectAllGraphItems,
        selectAdjacentGraphNodes,
        fitGraph,
        captureCanvasViewport,
        applyCanvasViewport,
        focusGraphItemById,
        clampNumber,
      },
    });
  }

  function buildShellOverlayCommands() {
    return flowShellOverlayCommandsAdapter.createFlowShellOverlayCommands({
      state,
      deps: {
        runReactContextMenuAction,
        closeContextMenu,
        closeDrawer,
        runReactDrawerAction,
        closeNodeInfoPopover,
        closeEdgeInfoPopover,
        closeEdgeLabelDialog,
        setReactEdgeLabelValue,
        commitEdgeLabel,
        openEdgeTxnModal,
        closeTxnModal,
        toggleTxnModalSort,
        setTxnColumnWidth,
      },
    });
  }

  function buildShellCommandDefinitions() {
    return {
      ...buildShellToolbarCommands(),
      ...buildShellAnalysisCommands(),
      ...buildShellCanvasCommands(),
      ...buildShellOverlayCommands(),
      setLeftCollapsed: (collapsed) => applyLeftCollapsed(collapsed),
      toggleLeftCollapsed: () => applyLeftCollapsed(!state.leftCollapsed),
      setTab: (tab) => setTab(tab),
      setSearch: (value) => setSearchQuery(value),
      toggleGroup: (gid) => toggleGroup(gid),
      toggleGroupSelect: (gid) => toggleGroupSelect(gid),
      toggleItemSelect: (iid) => toggleItemSelect(iid),
      selectAll: () => {
        state.treeData.forEach((group) => (group.items || []).forEach((item) => state.selected.add(item.id)));
        updateSelectionSummary();
        renderTree();
      },
      clearSelection: () => {
        state.selected = new Set();
        updateSelectionSummary();
        renderTree();
      },
      setDir: (dir) => setDirectionFilter(dir),
      setHop: (value) => setHopFilter(value),
      setMinAmount: (value) => setMinAmountFilter(value),
      setMaxEdges: (value) => setMaxEdgesFilter(value),
      buildGraph: (options = {}) => buildGraph(options || {}),
      clearGraph: () => clearGraph(),
      addView: () => {
        const view = buildEmptyView(nextViewTitle());
        addView(view, { activate: true, applyGraph: true });
      },
      activateView: (viewId) => setActiveView(viewId),
      saveCurrentView: () => saveCurrentView(),
      closeView: (viewId) => deleteView(viewId),
      renameView: (viewId) => renameView(viewId),
      reorderViews: async (fromId, targetId, place = "before") => {
        reorderViews(fromId, targetId, place);
        await persistViewsOrder();
        scheduleShellStateSync("reorder-views");
      },
      openViewContextMenu: (viewId, x, y) => {
        openViewContextMenu(viewId, x, y);
      },
      fitGraph: (options = {}) => fitGraph(options || {}),
      refit: () => finalizeView(),
    };
  }

  window.__ANALYTIX_VIZ_SHELL__ = flowShellFacadeAdapter.createFlowShellFacade({
    runtimeStore,
    listeners: shellBridgeListeners,
    buildShellState: () => buildShellState(),
    getPerfSnapshot: () => window.__ANALYTIX_VIZ_DEBUG__?.getPerfSnapshot?.() || {},
    finalizeShellCommandRegistry,
    commandDefinitions: buildShellCommandDefinitions(),
  });

  window.__ANALYTIX_VIZ_DEBUG__ = {
    getLayoutSnapshot() {
      return flowDebugSurface?.getLayoutSnapshot?.() || {};
    },
    openNodeDrawer(nodeId) {
      const graph = ensureGraph();
      if (!graph || !nodeId) return false;
      const item = graph.findById?.(nodeId);
      if (!item) return false;
      openNodeDetailDrawer(item.getModel?.() || {}, graph);
      return true;
    },
    openNodeInfo(nodeId) {
      const graph = ensureGraph();
      if (!graph || !nodeId) return false;
      const item = graph.findById?.(nodeId);
      if (!item) return false;
      const model = item.getModel?.() || {};
      const point = getGraphNodeInfo(graph, nodeId)?.client || getGraphEdgeClientPoint(graph, model) || {
        x: window.innerWidth / 2,
        y: window.innerHeight / 2,
      };
      openNodeInfoPopover(point.x, point.y, model);
      return true;
    },
    openEdgeInfo(edgeId, lane = "") {
      const graph = ensureGraph();
      if (!graph || !edgeId) return false;
      const item = graph.findById?.(edgeId);
      if (!item) return false;
      const model = item.getModel?.() || {};
      const point = getGraphEdgeClientPoint(graph, model) || {
        x: window.innerWidth / 2,
        y: window.innerHeight / 2,
      };
      openEdgeInfoPopover(point.x, point.y, model, lane);
      return true;
    },
    openTxnModal(edgeId, lane = "") {
      const graph = ensureGraph();
      if (!graph || !edgeId) return false;
      const item = graph.findById?.(edgeId);
      if (!item) return false;
      openEdgeTxnModal(item.getModel?.() || {}, lane);
      return true;
    },
    createUserEdge(sourceId, targetId) {
      const graph = ensureGraph();
      if (!graph) return "";
      const createdEntry = addUserCreatedEdge(graph, sourceId, targetId);
      const createdEdge = createdEntry?.edge || null;
      if (!createdEdge) return "";
      try {
        graph.refreshPositions?.();
        const item = graph.findById?.(createdEdge.id);
        if (item) {
          clearGraphSelection(graph);
          graph.refreshItem?.(item);
          graph.setItemState(item, "selected", true);
          syncActiveSelection(graph, item);
        }
        graph.getEdges?.().forEach((edge) => graph.refreshItem?.(edge));
        graph.paint();
      } catch (e) {}
      updateGraphStats();
      updateCurrentViewSnapshot();
      scheduleGraphPatch({
        scope: "local-update",
        reason: "debug-create-user-edge",
      });
      syncEdgeDirectionControl(graph);
      return createdEdge.id;
    },
    createUserNode(x = 96, y = 96) {
      const graph = ensureGraph();
      if (!graph) return "";
      const beforeIds = new Set((state.graph?.data?.nodes || []).map((node) => String(node?.id || "")).filter(Boolean));
      createNodeAt({ x: Number(x) || 96, y: Number(y) || 96 });
      const selected = graph.findAllByState?.("node", "selected") || [];
      const item = selected.find((node) => node.getModel?.()?.userCreated) || selected[0] || null;
      const selectedId = String(item?.getModel?.()?.id || "");
      if (selectedId) return selectedId;
      const created = (state.graph?.data?.nodes || []).find((node) => {
        const id = String(node?.id || "");
        return id && !beforeIds.has(id) && node?.userCreated;
      });
      return String(created?.id || "");
    },
    selectNode(nodeId, append = false) {
      const graph = ensureGraph();
      const item = graph?.findById?.(nodeId);
      if (!graph || !item || !state.canvasInteractionStore) return false;
      return !!state.canvasInteractionStore.toggleItemSelection?.(item, {
        targetGraph: graph,
        multi: !!append,
        reason: "debug-select-node",
      });
    },
    deleteSelection() {
      return deleteSelectedGraphItems();
    },
    moveNode(nodeId, dx = 32, dy = 18) {
      const graph = ensureGraph();
      if (!graph || !nodeId) return false;
      const item = graph.findById?.(nodeId);
      if (!item) return false;
      const usingWebglEngine = String(getGraphEngine()?.version || "").startsWith("AnalytixWebGL");
      const dragController = ensureCanvasDragController(graph, usingWebglEngine);
      if (!dragController) return false;
      dragController.handleNodeDragStart?.({ item });
      const model = item.getModel?.() || {};
      const nextX = Number(model.x || 0) + Number(dx || 0);
      const nextY = Number(model.y || 0) + Number(dy || 0);
      try {
        graph.updateItem?.(item, {
          x: nextX,
          y: nextY,
        });
      } catch (e) {
        model.x = nextX;
        model.y = nextY;
        graph.refreshPositions?.();
      }
      dragController.handleNodeDrag?.({ item });
      return !!dragController.handleNodeDragEnd?.({ item });
    },
    drillNode(nodeId) {
      const graph = ensureGraph();
      if (!graph || !nodeId) return false;
      const item = graph.findById?.(nodeId);
      if (!item) return false;
      void drillTxnNode(item.getModel?.() || {});
      return true;
    },
    expandProjectionForDebug(payload = {}, options = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const opts = options && typeof options === "object" ? options : {};
      if (opts.forceSkeleton) {
        state.graphProjection = {
          ...(state.graphProjection && typeof state.graphProjection === "object" ? state.graphProjection : {}),
          mode: "skeleton",
          ...(opts.projection && typeof opts.projection === "object" ? opts.projection : {}),
        };
      }
      if (opts.snapshotRef && typeof opts.snapshotRef === "object") {
        state.resultSnapshotRef = cloneResultSnapshotRef(opts.snapshotRef);
      }
      return requestProjectionExpand(request, {
        reason: String(opts.reason || "debug-projection-expand"),
        preserveViewport: opts.preserveViewport !== false,
        focusNodeId: String(opts.focusNodeId || ""),
        openNodeDetail: !!opts.openNodeDetail,
      });
    },
    refreshNetworkCommunityQualityForDebug(payload = {}, options = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const opts = options && typeof options === "object" ? options : {};
      state.graphProjection = {
        ...(state.graphProjection && typeof state.graphProjection === "object" ? state.graphProjection : {}),
        mode: "skeleton",
        search_match_node_ids: parseUniqueList(request.searchMatchNodeIds || request.search_match_node_ids || [], {
          allowString: true,
        }),
        expanded_node_ids: parseUniqueList(request.expandedNodeIds || request.expanded_node_ids || request.nodeIds || [], {
          allowString: true,
        }),
        expanded_cluster_ids: parseUniqueList(request.expandedClusterIds || request.expanded_cluster_ids || request.clusterIds || [], {
          allowString: true,
        }),
        path_node_ids: parseUniqueList(request.pathNodeIds || request.path_node_ids || [], {
          allowString: true,
        }),
        search_query: String(request.searchQuery || request.search_query || request.query || "").trim(),
      };
      if (opts.snapshotRef && typeof opts.snapshotRef === "object") {
        state.resultSnapshotRef = cloneResultSnapshotRef(opts.snapshotRef);
      }
      return scheduleNetworkCommunityQualityRefresh(String(opts.reason || "debug-community-quality-refresh"), {
        delayMs: Math.max(0, Number(opts.delayMs) || 0),
      });
    },
    openEdgeLabel(edgeId) {
      const graph = ensureGraph();
      if (!graph || !edgeId) return false;
      const item = graph.findById?.(edgeId);
      if (!item) return false;
      openEdgeLabelDialog(item);
      return true;
    },
    getEdgeLabel(edgeId) {
      const graph = ensureGraph();
      if (!graph || !edgeId) return "";
      const item = graph.findById?.(edgeId);
      return String(item?.getModel?.()?.label || "");
    },
    getRendererSnapshot() {
      return flowDebugSurface?.getRendererSnapshot?.() || {};
    },
    getGraphSelectionSnapshot() {
      return flowDebugSurface?.getGraphSelectionSnapshot?.() || {};
    },
    getGraphItemStyleSnapshot(ids) {
      return flowDebugSurface?.getGraphItemStyleSnapshot?.(ids) || {};
    },
    getGraphEdgePresentationSnapshot() {
      return flowDebugSurface?.getGraphEdgePresentationSnapshot?.() || {};
    },
    triggerNetworkDenseHoverRevealForDebug() {
      return flowDebugSurface?.triggerNetworkDenseHoverRevealForDebug?.() || {};
    },
    restoreNetworkDenseHoverRevealForDebug(nodeId = "") {
      return flowDebugSurface?.restoreNetworkDenseHoverRevealForDebug?.(nodeId) || {};
    },
    getGraphViewportSnapshot() {
      return flowDebugSurface?.getGraphViewportSnapshot?.() || {};
    },
    getGraphRuntimeTopologySnapshot() {
      return flowDebugSurface?.getGraphRuntimeTopologySnapshot?.() || {};
    },
    getViewPersistenceSnapshot() {
      const active = getActiveView();
      return {
        activeView: active
          ? {
              id: String(active.id || ""),
              title: String(active.title || ""),
              saved: !!active.saved,
              counts: viewCounts(active),
            }
          : null,
        lastBackendCall: viewPersistenceDebug.lastBackendCall ? { ...viewPersistenceDebug.lastBackendCall } : null,
        lastContextAction: viewPersistenceDebug.lastContextAction ? { ...viewPersistenceDebug.lastContextAction } : null,
        lastSaveView: viewPersistenceDebug.lastSaveView ? { ...viewPersistenceDebug.lastSaveView } : null,
      };
    },
    getTextAtlasSnapshot() {
      return flowDebugSurface?.getTextAtlasSnapshot?.() || {};
    },
    getLastBuildGraphPerf() {
      const snapshot = state.lastBuildGraphPerf;
      if (!snapshot || typeof snapshot !== "object") {
        return {};
      }
      return {
        ...snapshot,
        counts: snapshot.counts ? { ...snapshot.counts } : { nodes: 0, edges: 0 },
        perf:
          snapshot.perf && typeof snapshot.perf === "object"
            ? {
                ...snapshot.perf,
                stagedLabels:
                  snapshot.perf.stagedLabels && typeof snapshot.perf.stagedLabels === "object"
                    ? {
                        ...snapshot.perf.stagedLabels,
                        budget:
                          snapshot.perf.stagedLabels.budget && typeof snapshot.perf.stagedLabels.budget === "object"
                            ? { ...snapshot.perf.stagedLabels.budget }
                            : null,
                      }
                    : null,
                textInitial:
                  snapshot.perf.textInitial && typeof snapshot.perf.textInitial === "object"
                    ? { ...snapshot.perf.textInitial }
                    : null,
              }
            : {},
      };
    },
    getPerfSnapshot() {
      return flowDebugSurface?.getPerfSnapshot?.() || {};
    },
    projectNetworkSectorPlacement(payload = {}) {
      return projectNetworkSectorPlacementWithRust(payload);
    },
    exportPerfSnapshot() {
      return flowDebugSurface?.exportPerfSnapshot?.() || "";
    },
  };

  window.__analytixSetVizLogConfig = (cfg, opts = {}) => {
    const persist = !!opts?.persist;
    const applied = applyLogSwitch(cfg, { persist });
    try {
      if (state.graph?.inited && state.graph?.instance) {
        state.graph.instance.set?.("webglWarn", !!applied.webglWarn);
      }
    } catch (e) {}
    return applied;
  };
  window.__analytixGetVizLogConfig = () => ({ ...logSwitch });

  function scheduleInit() {
    setTimeout(init, 0);
  }

  window.__analytixOnLoadFinished = init;
  window.__analytixVizFromStats = (payload) => {
    applyGraphRequest(payload);
  };
  window.__analytixVizRefit = () => {
    finalizeView();
  };
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", scheduleInit, { once: true });
  } else {
    scheduleInit();
  }
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") {
      syncCaseFromBackend("visible");
      if (state.graph.data.nodes.length) finalizeView();
    }
  });
})();
