(() => {
  function noop() {}

  function createFlowShellToolbarCommands({ state = null, deps = {} } = {}) {
    const isReactShellSubscribed =
      typeof deps.isReactShellSubscribed === "function" ? deps.isReactShellSubscribed : () => false;
    const triggerShellControl =
      typeof deps.triggerShellControl === "function" ? deps.triggerShellControl : noop;
    const toggleReactStylePopover =
      typeof deps.toggleReactStylePopover === "function" ? deps.toggleReactStylePopover : noop;
    const openRibbonPopoverFromRect =
      typeof deps.openRibbonPopoverFromRect === "function" ? deps.openRibbonPopoverFromRect : noop;
    const buildListPopover = typeof deps.buildListPopover === "function" ? deps.buildListPopover : () => "";
    const buildColorPopover = typeof deps.buildColorPopover === "function" ? deps.buildColorPopover : () => "";
    const buildLineStylePopover =
      typeof deps.buildLineStylePopover === "function" ? deps.buildLineStylePopover : () => "";
    const buildArrowPopover = typeof deps.buildArrowPopover === "function" ? deps.buildArrowPopover : () => "";
    const buildShapePopover = typeof deps.buildShapePopover === "function" ? deps.buildShapePopover : () => "";
    const buildIconLibraryPopover =
      typeof deps.buildIconLibraryPopover === "function" ? deps.buildIconLibraryPopover : () => "";
    const closeReactStylePopover =
      typeof deps.closeReactStylePopover === "function" ? deps.closeReactStylePopover : noop;
    const applyReactStylePopoverOption =
      typeof deps.applyReactStylePopoverOption === "function" ? deps.applyReactStylePopoverOption : noop;
    const applyReactStylePopoverColor =
      typeof deps.applyReactStylePopoverColor === "function" ? deps.applyReactStylePopoverColor : noop;
    const setReactStyleIconLibraryTab =
      typeof deps.setReactStyleIconLibraryTab === "function" ? deps.setReactStyleIconLibraryTab : noop;
    const setReactStyleIconLibraryQuery =
      typeof deps.setReactStyleIconLibraryQuery === "function" ? deps.setReactStyleIconLibraryQuery : noop;
    const applyReactStyleIconSymbol =
      typeof deps.applyReactStyleIconSymbol === "function" ? deps.applyReactStyleIconSymbol : noop;
    const ensureRibbonStyle = typeof deps.ensureRibbonStyle === "function" ? deps.ensureRibbonStyle : noop;
    const clampNumber = typeof deps.clampNumber === "function" ? deps.clampNumber : (value) => Number(value) || 0;
    const updateRibbonState = typeof deps.updateRibbonState === "function" ? deps.updateRibbonState : noop;
    const ensureGraph = typeof deps.ensureGraph === "function" ? deps.ensureGraph : () => null;
    const canEditEdgeDirection =
      typeof deps.canEditEdgeDirection === "function" ? deps.canEditEdgeDirection : () => true;
    const toast = typeof deps.toast === "function" ? deps.toast : noop;
    const setCreateNodeMode = typeof deps.setCreateNodeMode === "function" ? deps.setCreateNodeMode : noop;
    const createLinksFromSelection =
      typeof deps.createLinksFromSelection === "function" ? deps.createLinksFromSelection : noop;
    const runLayoutPresetCommand =
      typeof deps.runLayoutPresetCommand === "function" ? deps.runLayoutPresetCommand : noop;
    const toggleReactToolbarMenu =
      typeof deps.toggleReactToolbarMenu === "function" ? deps.toggleReactToolbarMenu : noop;
    const buildLayoutEdgeRoutingMenuItems =
      typeof deps.buildLayoutEdgeRoutingMenuItems === "function" ? deps.buildLayoutEdgeRoutingMenuItems : () => [];
    const createVirtualAnchor =
      typeof deps.createVirtualAnchor === "function" ? deps.createVirtualAnchor : () => null;
    const openLayoutEdgeRoutingMenu =
      typeof deps.openLayoutEdgeRoutingMenu === "function" ? deps.openLayoutEdgeRoutingMenu : noop;
    const mergeSameNameNodes =
      typeof deps.mergeSameNameNodes === "function" ? deps.mergeSameNameNodes : noop;
    const mergeSameNameNodesNet =
      typeof deps.mergeSameNameNodesNet === "function" ? deps.mergeSameNameNodesNet : noop;
    const normalizeAnchorRect =
      typeof deps.normalizeAnchorRect === "function" ? deps.normalizeAnchorRect : () => null;
    const buildMergeMenuItems =
      typeof deps.buildMergeMenuItems === "function" ? deps.buildMergeMenuItems : () => [];
    const openContextMenu = typeof deps.openContextMenu === "function" ? deps.openContextMenu : noop;
    const fitGraph = typeof deps.fitGraph === "function" ? deps.fitGraph : noop;
    const toggleReactExportOverlay =
      typeof deps.toggleReactExportOverlay === "function" ? deps.toggleReactExportOverlay : noop;
    const closeReactExportOverlay =
      typeof deps.closeReactExportOverlay === "function" ? deps.closeReactExportOverlay : noop;
    const scheduleShellStateSync =
      typeof deps.scheduleShellStateSync === "function" ? deps.scheduleShellStateSync : noop;
    const exportCurrentView =
      typeof deps.exportCurrentView === "function" ? deps.exportCurrentView : noop;
    const applyEdgeDetailLabels =
      typeof deps.applyEdgeDetailLabels === "function" ? deps.applyEdgeDetailLabels : noop;
    const syncEdgeDetailButton =
      typeof deps.syncEdgeDetailButton === "function" ? deps.syncEdgeDetailButton : noop;
    const updateCurrentViewSnapshot =
      typeof deps.updateCurrentViewSnapshot === "function" ? deps.updateCurrentViewSnapshot : noop;
    const resetToInitialStyles =
      typeof deps.resetToInitialStyles === "function" ? deps.resetToInitialStyles : noop;
    const applySnapshotState =
      typeof deps.applySnapshotState === "function" ? deps.applySnapshotState : noop;
    const syncGraphSearchQuery =
      typeof deps.syncGraphSearchQuery === "function" ? deps.syncGraphSearchQuery : noop;
    const searchAndFocusGraph =
      typeof deps.searchAndFocusGraph === "function" ? deps.searchAndFocusGraph : async () => {};
    const expandProjectionViewport =
      typeof deps.expandProjectionViewport === "function" ? deps.expandProjectionViewport : async () => {};
    const expandProjectionSelectionPath =
      typeof deps.expandProjectionSelectionPath === "function" ? deps.expandProjectionSelectionPath : async () => {};

    const FONT_LIST = Array.isArray(deps.FONT_LIST) ? deps.FONT_LIST : [];
    const FONT_SIZE_LIST = Array.isArray(deps.FONT_SIZE_LIST) ? deps.FONT_SIZE_LIST : [];
    const LINE_STYLE_OPTIONS = Array.isArray(deps.LINE_STYLE_OPTIONS) ? deps.LINE_STYLE_OPTIONS : [];
    const LINE_WIDTH_OPTIONS = Array.isArray(deps.LINE_WIDTH_OPTIONS) ? deps.LINE_WIDTH_OPTIONS : [];
    const ARROW_OPTIONS = Array.isArray(deps.ARROW_OPTIONS) ? deps.ARROW_OPTIONS : [];
    const NODE_SHAPE_OPTIONS = Array.isArray(deps.NODE_SHAPE_OPTIONS) ? deps.NODE_SHAPE_OPTIONS : [];
    const NODE_SIZE_OPTIONS = Array.isArray(deps.NODE_SIZE_OPTIONS) ? deps.NODE_SIZE_OPTIONS : [];

    return {
      triggerControl: (controlId, eventType = "click") => triggerShellControl(controlId, eventType),
      style: {
        openFontFamily: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-font-family", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnFontFamily", () =>
            buildListPopover(FONT_LIST, state?.ribbonStyle?.fontFamily, (val) => updateRibbonState({ fontFamily: val }))
          );
        },
        openFontSize: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-font-size", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnFontSize", () =>
            buildListPopover(FONT_SIZE_LIST, state?.ribbonStyle?.fontSize, (val) => updateRibbonState({ fontSize: val }))
          );
        },
        closePopover: () => closeReactStylePopover(),
        applyPopoverOption: (value) => applyReactStylePopoverOption(value),
        applyPopoverColor: (color, closeAfter = false) => applyReactStylePopoverColor(color, closeAfter),
        setIconLibraryTab: (tabId) => setReactStyleIconLibraryTab(tabId),
        setIconLibraryQuery: (query) => setReactStyleIconLibraryQuery(query),
        applyIconSymbol: (iconId) => applyReactStyleIconSymbol(iconId),
        increaseFontSize: () => {
          ensureRibbonStyle();
          const current = clampNumber(state?.ribbonStyle?.fontSize ?? 13, 8, 36);
          updateRibbonState({ fontSize: clampNumber(current + 1, 8, 36) });
        },
        decreaseFontSize: () => {
          ensureRibbonStyle();
          const current = clampNumber(state?.ribbonStyle?.fontSize ?? 13, 8, 36);
          updateRibbonState({ fontSize: clampNumber(current - 1, 8, 36) });
        },
        toggleBold: () => {
          ensureRibbonStyle();
          updateRibbonState({ fontBold: !state?.ribbonStyle?.fontBold });
        },
        toggleItalic: () => {
          ensureRibbonStyle();
          updateRibbonState({ fontItalic: !state?.ribbonStyle?.fontItalic });
        },
        toggleUnderline: () => {
          ensureRibbonStyle();
          updateRibbonState({ fontUnderline: !state?.ribbonStyle?.fontUnderline });
        },
        toggleShadow: () => {
          ensureRibbonStyle();
          updateRibbonState({ fontShadow: !state?.ribbonStyle?.fontShadow });
        },
        openTextColor: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-text-color", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnTextColor", () => buildColorPopover("textColor"));
        },
        openOutlineColor: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-outline-color", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnOutlineColor", () => buildColorPopover("outlineColor"));
        },
        openLineStyle: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-line-style", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnLineStyle", () =>
            buildLineStylePopover(LINE_STYLE_OPTIONS, state?.ribbonStyle?.edgeDash, (val) => updateRibbonState({ edgeDash: val }))
          );
        },
        openLineWidth: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-line-width", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnLineWidth", () =>
            buildListPopover(LINE_WIDTH_OPTIONS, state?.ribbonStyle?.edgeWidth, (val) => updateRibbonState({ edgeWidth: Number(val) }))
          );
        },
        openLineArrow: (anchorRect) => {
          const graph = ensureGraph();
          if (!canEditEdgeDirection(graph)) {
            toast("统计分析展示模式下禁止修改方向", "warn");
            return;
          }
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-line-arrow", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnLineArrow", () =>
            buildArrowPopover(ARROW_OPTIONS, state?.ribbonStyle?.edgeArrow, (val) => updateRibbonState({ edgeArrow: val }))
          );
        },
        openNodeShape: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-node-shape", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnNodeShape", () =>
            buildShapePopover(NODE_SHAPE_OPTIONS, state?.ribbonStyle?.nodeShape, (val) => updateRibbonState({ nodeShape: val }))
          );
        },
        openNodeFill: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-node-fill", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnNodeFill", () => buildColorPopover("nodeFill"));
        },
        openIconSize: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-icon-size", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnIconSize", () =>
            buildListPopover(NODE_SIZE_OPTIONS, state?.ribbonStyle?.nodeSize, (val) => updateRibbonState({ nodeSize: Number(val) }))
          );
        },
        toggleCreateNodeMode: () => {
          setCreateNodeMode(!state?.createNodeMode);
        },
        openIconLibrary: (anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactStylePopover("style-icon-library", anchorRect);
            return;
          }
          openRibbonPopoverFromRect(anchorRect, "btnIconLibrary", () => buildIconLibraryPopover());
        },
        createLinkFromSelection: () => {
          createLinksFromSelection();
        },
      },
      layout: {
        setPreset: (preset) => runLayoutPresetCommand(preset),
        openEdgeRoutingMenu: (mode, anchorRect) => {
          if (isReactShellSubscribed()) {
            toggleReactToolbarMenu(`layout-edge-routing-${mode}`, anchorRect, buildLayoutEdgeRoutingMenuItems(mode));
            return;
          }
          const anchor = createVirtualAnchor(anchorRect, `layout-${mode}`);
          if (!anchor) return;
          openLayoutEdgeRoutingMenu(mode, anchor);
        },
      },
      ops: {
        mergeNodes: () => mergeSameNameNodes(),
        mergeNodesNet: () => mergeSameNameNodesNet(),
        openMergeMenu: (anchorRect) => {
          const rect = normalizeAnchorRect(anchorRect);
          if (!rect) return;
          if (isReactShellSubscribed()) {
            toggleReactToolbarMenu("ops-merge", rect, buildMergeMenuItems());
            return;
          }
          openContextMenu(rect.left, rect.bottom + 6, buildMergeMenuItems());
        },
        fitGraph: () => fitGraph({ force: true }),
        openExport: (anchorRect) => toggleReactExportOverlay(anchorRect),
        closeExport: () => closeReactExportOverlay(),
        setExportScale: (scale) => {
          state.exportScale = clampNumber(scale, 1, 4);
          scheduleShellStateSync("react-export-scale");
        },
        exportFormat: (format) => {
          closeReactExportOverlay({ sync: false });
          exportCurrentView(format, state.exportScale || 1);
          scheduleShellStateSync("react-export-format");
        },
        toggleEdgeDetail: () => {
          state.edgeDetail = !state.edgeDetail;
          applyEdgeDetailLabels(state.edgeDetail);
          syncEdgeDetailButton();
          updateCurrentViewSnapshot();
        },
        redo: () => resetToInitialStyles(),
        undo: () => {
          const stack = state.undoStack || [];
          const snapshot = stack.pop();
          state.undoStack = stack;
          if (!snapshot) return;
          applySnapshotState(snapshot);
        },
      },
      graph: {
        setSearch: (value) => syncGraphSearchQuery(value),
        clearSearch: () => syncGraphSearchQuery(""),
        runSearch: () => void searchAndFocusGraph(),
        expandViewport: () => void expandProjectionViewport({ reason: "shell-graph-expand-viewport" }),
        expandSelectionPath: () => void expandProjectionSelectionPath({ reason: "shell-graph-expand-selection-path" }),
      },
    };
  }

  window.__ANALYTIX_FLOW_SHELL_TOOLBAR_COMMANDS__ = {
    createFlowShellToolbarCommands,
  };
})();
