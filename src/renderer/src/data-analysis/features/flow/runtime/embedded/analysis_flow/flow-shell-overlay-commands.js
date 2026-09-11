(() => {
  function noop() {}

  function createFlowShellOverlayCommands({ state = null, deps = {} } = {}) {
    const runReactContextMenuAction =
      typeof deps.runReactContextMenuAction === "function" ? deps.runReactContextMenuAction : noop;
    const closeContextMenu = typeof deps.closeContextMenu === "function" ? deps.closeContextMenu : noop;
    const closeDrawer = typeof deps.closeDrawer === "function" ? deps.closeDrawer : noop;
    const runReactDrawerAction =
      typeof deps.runReactDrawerAction === "function" ? deps.runReactDrawerAction : noop;
    const closeNodeInfoPopover =
      typeof deps.closeNodeInfoPopover === "function" ? deps.closeNodeInfoPopover : noop;
    const closeEdgeInfoPopover =
      typeof deps.closeEdgeInfoPopover === "function" ? deps.closeEdgeInfoPopover : noop;
    const closeEdgeLabelDialog =
      typeof deps.closeEdgeLabelDialog === "function" ? deps.closeEdgeLabelDialog : noop;
    const setReactEdgeLabelValue =
      typeof deps.setReactEdgeLabelValue === "function" ? deps.setReactEdgeLabelValue : noop;
    const commitEdgeLabel =
      typeof deps.commitEdgeLabel === "function" ? deps.commitEdgeLabel : noop;
    const openEdgeTxnModal =
      typeof deps.openEdgeTxnModal === "function" ? deps.openEdgeTxnModal : noop;
    const closeTxnModal = typeof deps.closeTxnModal === "function" ? deps.closeTxnModal : noop;
    const toggleTxnModalSort =
      typeof deps.toggleTxnModalSort === "function" ? deps.toggleTxnModalSort : noop;
    const setTxnColumnWidth =
      typeof deps.setTxnColumnWidth === "function" ? deps.setTxnColumnWidth : noop;

    return {
      overlay: {
        runContextMenuAction: (actionId) => runReactContextMenuAction(actionId),
        closeContextMenu: () => closeContextMenu(),
        closeDrawer: () => closeDrawer(),
        runDrawerAction: (actionId) => runReactDrawerAction(actionId),
        closeNodeInfo: () => closeNodeInfoPopover(),
        closeEdgeInfo: () => closeEdgeInfoPopover(),
        closeEdgeLabel: () => closeEdgeLabelDialog(),
        setEdgeLabelValue: (value) => setReactEdgeLabelValue(value),
        saveEdgeLabel: () => commitEdgeLabel(),
        openEdgeTxnDetail: () => {
          const ctx = state?.edgeInfoContext;
          if (!ctx || !ctx.model) return;
          closeEdgeInfoPopover();
          openEdgeTxnModal(ctx.model, ctx.lane, ctx.requestKey);
        },
        closeTxnModal: () => closeTxnModal(),
        toggleTxnSort: (colKey) => toggleTxnModalSort(colKey),
        setTxnColumnWidth: (colKey, width) => setTxnColumnWidth(colKey, width),
      },
    };
  }

  window.__ANALYTIX_FLOW_SHELL_OVERLAY_COMMANDS__ = {
    createFlowShellOverlayCommands,
  };
})();
