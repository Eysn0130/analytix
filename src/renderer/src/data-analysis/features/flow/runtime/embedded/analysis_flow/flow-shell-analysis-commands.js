(() => {
  function noop() {}

  function createFlowShellAnalysisCommands({ state = null, deps = {} } = {}) {
    const toggleReactFilterOverlay =
      typeof deps.toggleReactFilterOverlay === "function" ? deps.toggleReactFilterOverlay : noop;
    const closeReactFilterOverlay =
      typeof deps.closeReactFilterOverlay === "function" ? deps.closeReactFilterOverlay : noop;
    const applyAnalysisAmountFilterState =
      typeof deps.applyAnalysisAmountFilterState === "function" ? deps.applyAnalysisAmountFilterState : noop;
    const clearAnalysisAmountFilterState =
      typeof deps.clearAnalysisAmountFilterState === "function" ? deps.clearAnalysisAmountFilterState : noop;
    const toggleAnalysisCollapse =
      typeof deps.toggleAnalysisCollapse === "function" ? deps.toggleAnalysisCollapse : noop;
    const assignGroupTag = typeof deps.assignGroupTag === "function" ? deps.assignGroupTag : noop;
    const isReactShellSubscribed =
      typeof deps.isReactShellSubscribed === "function" ? deps.isReactShellSubscribed : () => false;
    const toggleReactToolbarMenu =
      typeof deps.toggleReactToolbarMenu === "function" ? deps.toggleReactToolbarMenu : noop;
    const buildGroupOpsMenuItems =
      typeof deps.buildGroupOpsMenuItems === "function" ? deps.buildGroupOpsMenuItems : () => [];
    const openRibbonPopoverFromRect =
      typeof deps.openRibbonPopoverFromRect === "function" ? deps.openRibbonPopoverFromRect : noop;
    const buildListPopover = typeof deps.buildListPopover === "function" ? deps.buildListPopover : () => "";
    const applyGroupOperation =
      typeof deps.applyGroupOperation === "function" ? deps.applyGroupOperation : noop;
    const recordUndoSnapshot =
      typeof deps.recordUndoSnapshot === "function" ? deps.recordUndoSnapshot : noop;
    const applyEdgeWidthEncoding =
      typeof deps.applyEdgeWidthEncoding === "function" ? deps.applyEdgeWidthEncoding : noop;
    const applyEdgeColorEncoding =
      typeof deps.applyEdgeColorEncoding === "function" ? deps.applyEdgeColorEncoding : noop;
    const applyNodeScaleEncoding =
      typeof deps.applyNodeScaleEncoding === "function" ? deps.applyNodeScaleEncoding : noop;
    const syncAnalysisButtons =
      typeof deps.syncAnalysisButtons === "function" ? deps.syncAnalysisButtons : noop;

    return {
      analysis: {
        openFilter: (anchorRect) => toggleReactFilterOverlay(anchorRect),
        closeFilter: () => closeReactFilterOverlay(),
        setFilterMin: (value) => applyAnalysisAmountFilterState(value, state?.analysis?.filterMax),
        setFilterMax: (value) => applyAnalysisAmountFilterState(state?.analysis?.filterMin, value),
        clearFilter: () => clearAnalysisAmountFilterState(),
        toggleCollapseChildren: () => {
          toggleAnalysisCollapse();
        },
        assignGroupTag: (index) => {
          assignGroupTag(index);
        },
        openGroupOps: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactToolbarMenu("analysis-group-ops", anchorRect, buildGroupOpsMenuItems());
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnGroupOps", () =>
            buildListPopover(
              [
                { label: "交集", value: "intersection" },
                { label: "差集", value: "difference" },
              ],
              "",
              (val) => applyGroupOperation(val)
            )
          );
        },
        toggleEncodeWidth: () => {
          recordUndoSnapshot();
          state.analysis.encodeWidth = !state.analysis.encodeWidth;
          applyEdgeWidthEncoding(state.analysis.encodeWidth);
          syncAnalysisButtons();
        },
        toggleEncodeColor: () => {
          recordUndoSnapshot();
          state.analysis.encodeColor = !state.analysis.encodeColor;
          applyEdgeColorEncoding(state.analysis.encodeColor);
          syncAnalysisButtons();
        },
        toggleNodeScale: () => {
          recordUndoSnapshot();
          state.analysis.nodeScale = !state.analysis.nodeScale;
          applyNodeScaleEncoding(state.analysis.nodeScale);
          syncAnalysisButtons();
        },
      },
    };
  }

  window.__ANALYTIX_FLOW_SHELL_ANALYSIS_COMMANDS__ = {
    createFlowShellAnalysisCommands,
  };
})();
