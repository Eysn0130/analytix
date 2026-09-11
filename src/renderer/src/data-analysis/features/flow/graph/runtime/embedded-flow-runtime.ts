import { emitConfirm, type ConfirmEventDetail } from "../../../../components/feedback/ConfirmCenter";
import { emitPrompt, type PromptEventDetail, type PromptValues } from "../../../../components/feedback/PromptCenter";
import { dismissTask, emitTask, emitTaskSuccess, type TaskEventDetail } from "../../../../components/feedback/TaskCenter";
import { createEmbeddedFlowBridgeService, EmbeddedFlowBridgeService } from "../api/flow-bridge-service";
import {
  createFlowSummaryBoundary,
  projectFlowSummaryForChrome,
  type FlowSummaryProjection,
} from "../model/flow-summary-boundary";
import { createEmbeddedFlowRuntimeStore, type EmbeddedFlowRuntimeStore } from "./embedded-flow-runtime-store";
import { projectFlowPerfSnapshot } from "./flow-perf-public-projection";
import {
  EMBEDDED_FLOW_RUNTIME_APP_PATH,
  EMBEDDED_FLOW_RUNTIME_LOADER_PATH,
  resolveEmbeddedFlowRuntimeScriptUrls,
} from "./embedded-flow-runtime-assets";
import flowIndexHtml from "../../runtime/embedded/analysis_flow/index.html?raw";
import statsBaseCss from "../../runtime/embedded/analysis_stats/app.css?raw";
import flowRuntimeCss from "../../runtime/embedded/analysis_flow/app.css?raw";
import workbenchUiKitCss from "../../../../components/workbench-ui/workbench-ui-kit.css?raw";
import txnDetailDialogCss from "../../../../components/txn-detail/txn-detail-dialog.css?raw";
import { projectOrdinaryFieldValue } from "../../../shared/ordinary-pii-projection";
import {
  isDataAnalysisRuntimeEndpointAuthorityCurrent,
  resolveDataAnalysisRuntimeEndpointAuthority,
  type DataAnalysisRuntimeEndpointAuthority,
} from "../../../../services/runtime-base";

const FLOW_RUNTIME_BODY_CLASS = "flow-runtime-body";
const FLOW_SHELL_STATE_SCHEMA_VERSION = 1;
const warnedShellStateSchemaVersions = new Set<number>();
const FLOW_RUNTIME_HOST_BASE_CSS = `
:host {
  display: block;
  width: 100%;
  height: 100%;
  min-height: 100%;
}

.${FLOW_RUNTIME_BODY_CLASS} {
  width: 100%;
  height: 100%;
  min-height: 100%;
  margin: 0;
  overflow: hidden;
}
`;

const FLOW_RUNTIME_REACT_SHELL_CSS = `
.${FLOW_RUNTIME_BODY_CLASS} {
  position: relative;
}

.analytix-react-shell-host {
  position: absolute;
  inset: 0;
  z-index: 24;
  pointer-events: none;
}

.analytix-react-shell-host > * {
  pointer-events: auto;
}

.analytix-react-shell-page-overlay {
  position: absolute;
  inset: 0;
  z-index: 28;
  pointer-events: none;
}

.analytix-react-shell-page-overlay > * {
  pointer-events: auto;
}

.analytix-react-shell-page-overlay > .analytix-react-shell-gap-mount {
  pointer-events: none;
}

.analytix-react-overlay-panel {
  position: fixed;
  z-index: 1009;
  min-width: 220px;
  max-width: 420px;
  padding: 10px;
  border-radius: 12px;
  border: 1px solid rgba(15, 23, 42, 0.12);
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 16px 36px rgba(2, 6, 23, 0.18);
}

.analytix-react-context-menu {
  display: grid;
  gap: 4px;
  width: max-content;
  min-width: 0;
  max-width: min(184px, calc(100vw - 20px));
  padding: 6px;
  border-radius: 10px;
}

.analytix-react-toolbar-menu {
  display: grid;
  gap: 4px;
  width: max-content;
  min-width: 0;
  max-width: min(160px, calc(100vw - 20px));
  padding: 6px;
  border-radius: 10px;
}

.analytix-react-context-item {
  width: 100%;
  border: 1px solid transparent;
  border-radius: 8px;
  padding: 6px 10px;
  font-size: 11px;
  line-height: 1.35;
  text-align: left;
  color: rgba(2, 6, 23, 0.88);
  background: rgba(255, 255, 255, 0.92);
  cursor: pointer;
  white-space: nowrap;
}

.analytix-react-context-item:hover {
  border-color: rgba(31, 111, 235, 0.38);
  background: rgba(31, 111, 235, 0.12);
}

.analytix-react-context-item:disabled {
  color: rgba(2, 6, 23, 0.4);
  cursor: not-allowed;
  background: rgba(248, 250, 252, 0.9);
}

.analytix-react-context-sep {
  height: 1px;
  margin: 6px 2px;
  background: rgba(15, 23, 42, 0.08);
}

.analytix-react-export-hint {
  margin-top: 8px;
  font-size: 11px;
  line-height: 1.4;
  color: rgba(2, 6, 23, 0.55);
}

:root[data-theme="dark"] .analytix-react-overlay-panel {
  border-color: color-mix(in oklab, var(--color-border, #313b50) 76%, transparent);
  background: linear-gradient(
    180deg,
    color-mix(in oklab, var(--color-bg-elevated, #181f2b) 92%, #101827),
    color-mix(in oklab, var(--color-bg-subtle, #232c3b) 78%, #101827)
  );
  color: var(--color-text, #ecf2ff);
  box-shadow: 0 18px 42px rgba(3, 8, 15, 0.44);
}

:root[data-theme="dark"] .analytix-react-context-item {
  border-color: color-mix(in oklab, var(--color-border, #313b50) 70%, transparent);
  background: color-mix(in oklab, var(--color-bg-elevated, #181f2b) 88%, var(--color-bg-subtle, #232c3b));
  color: var(--color-text, #ecf2ff);
}

:root[data-theme="dark"] .analytix-react-context-item:hover {
  border-color: color-mix(in oklab, var(--color-accent, #5f74ff) 38%, var(--color-border, #313b50));
  background: color-mix(in oklab, var(--color-accent, #5f74ff) 14%, var(--color-bg-subtle, #232c3b));
}

:root[data-theme="dark"] .analytix-react-context-item:disabled {
  background: color-mix(in oklab, var(--color-bg-subtle, #232c3b) 68%, transparent);
  color: color-mix(in oklab, var(--color-text-muted, #9baaca) 72%, transparent);
}

:root[data-theme="dark"] .analytix-react-context-sep {
  background: color-mix(in oklab, var(--color-border, #313b50) 78%, transparent);
}

:root[data-theme="dark"] .analytix-react-export-hint {
  color: var(--color-text-muted, #9baaca);
}

.analytix-react-shell-active #leftCard,
.analytix-react-shell-active .styleGroup,
.analytix-react-shell-active .layoutGroup,
.analytix-react-shell-active .analyzeGroup,
.analytix-react-shell-active .opsGroup,
.analytix-react-shell-active #viewsBar {
  position: relative;
}

.analytix-react-shell-active #leftCard > :not(.analytix-react-shell-host),
.analytix-react-shell-active .styleGroup > :not(.analytix-react-shell-host),
.analytix-react-shell-active .layoutGroup > :not(.analytix-react-shell-host),
.analytix-react-shell-active .analyzeGroup > :not(.analytix-react-shell-host),
.analytix-react-shell-active .opsGroup > :not(.analytix-react-shell-host),
.analytix-react-shell-active #viewsBar > :not(.analytix-react-shell-host) {
  visibility: hidden !important;
  pointer-events: none !important;
}

.analytix-react-shell-active #graphWrap > [data-shell-mount="empty-view"] {
  inset: 16px auto auto 18px;
  width: auto;
  height: auto;
}

.analytix-react-shell-active #graphSearchFloat > :not(.analytix-react-shell-host),
.analytix-react-shell-active #graphStatsFloat > :not(.analytix-react-shell-host) {
  display: none !important;
}

.analytix-react-shell-active #gapToggle {
  visibility: hidden !important;
  pointer-events: none !important;
}

.analytix-react-shell-active #graphSearchFloat > .analytix-react-shell-host,
.analytix-react-shell-active #graphStatsFloat > .analytix-react-shell-host {
  position: relative;
  inset: auto;
  display: block;
  z-index: 2;
}

.analytix-react-shell-active #graphSearchFloat > .analytix-react-shell-host {
  width: 100%;
  height: 100%;
}

.analytix-react-shell-active #graphStatsFloat > .analytix-react-shell-host {
  width: auto;
  height: auto;
}

.app.left-collapsed #leftCard > .analytix-react-shell-host {
  display: block !important;
}

.analytix-react-graph-search,
.analytix-react-graph-stats {
  display: flex;
  min-width: 0;
  min-height: 0;
}

.analytix-react-graph-search {
  width: 100%;
  height: 100%;
  align-items: center;
  gap: 0;
}

.analytix-react-graph-search-input {
  flex: 1 1 auto;
  min-width: 0;
}

.analytix-react-graph-search-clear {
  opacity: 0;
  pointer-events: none;
  transition: opacity 160ms ease, transform 160ms ease;
  transform: scale(0.92);
}

.analytix-react-graph-search.has-value .analytix-react-graph-search-clear {
  opacity: 1;
  pointer-events: auto;
  transform: scale(1);
}

.analytix-react-graph-stats {
  width: auto;
  height: auto;
  align-items: center;
  flex-wrap: nowrap;
  gap: 0;
}
`;

interface EmbeddedFlowRequest extends Record<string, unknown> {}

export interface EmbeddedFlowAnchorRect {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
}

interface EmbeddedFlowShellView {
  id: string;
  title: string;
  saved: boolean;
  active: boolean;
  counts: {
    nodes: number;
    edges: number;
  };
}

interface EmbeddedFlowShellTreeItem {
  id: string;
  title: string;
  sub: string;
}

interface EmbeddedFlowShellTreeGroup {
  id: string;
  title: string;
  meta: string;
  extra: string;
  items: EmbeddedFlowShellTreeItem[];
}

export interface EmbeddedFlowShellContextMenuItem {
  id: string;
  label: string;
  disabled: boolean;
  separator: boolean;
}

export interface EmbeddedFlowShellStylePopoverOption {
  value: string;
  label: string;
  dash?: number[];
}

export interface EmbeddedFlowShellStylePopoverTab {
  id: string;
  label: string;
}

export interface EmbeddedFlowShellDetailRow {
  label: string;
  value: string;
  mono: boolean;
  tone: string;
}

export interface EmbeddedFlowShellDetailAction {
  id: string;
  label: string;
  variant: string;
}

export interface EmbeddedFlowShellPoint {
  x: number;
  y: number;
}

export interface EmbeddedFlowShellNodeInfoPreview {
  size: number;
  fill: string;
  stroke: string;
  lineWidth: number;
}

export interface EmbeddedFlowShellEdgeInfoRow {
  time: string;
  amountText: string;
  amountTone: string;
}

export interface EmbeddedFlowShellEdgeTxnOverlayState {
  title: string;
  amountText: string;
  amountTone: string;
  countText: string;
  countClickable: boolean;
  loading: boolean;
  error: string;
  rows: EmbeddedFlowShellEdgeInfoRow[];
}

export interface EmbeddedFlowShellTxnColumn {
  key: string;
  title: string;
  width: number;
  sortable: boolean;
  sorted: boolean;
  sortDir: string;
}

export interface EmbeddedFlowShellTxnCell {
  text: string;
  mono: boolean;
  align: string;
  tone: string;
}

export interface EmbeddedFlowShellTxnRow {
  key: string;
  cells: Record<string, EmbeddedFlowShellTxnCell>;
}

interface EmbeddedFlowShellOverlayState {
  filter: {
    open: boolean;
    anchorRect: EmbeddedFlowAnchorRect | null;
  };
  stylePopover: {
    open: boolean;
    anchorRect: EmbeddedFlowAnchorRect | null;
    sourceKey: string;
    kind: "" | "list" | "color" | "lineStyle" | "arrow" | "shape" | "iconLibrary";
    currentValue: string;
    options: EmbeddedFlowShellStylePopoverOption[];
    commonColors: string[];
    recentColors: string[];
    iconTabs: EmbeddedFlowShellStylePopoverTab[];
    activeTab: string;
    iconQuery: string;
    icons: string[];
  };
  menu: {
    open: boolean;
    anchorRect: EmbeddedFlowAnchorRect | null;
    sourceKey: string;
    items: EmbeddedFlowShellContextMenuItem[];
  };
  exportPanel: {
    open: boolean;
    anchorRect: EmbeddedFlowAnchorRect | null;
    scale: number;
  };
  contextMenu: {
    open: boolean;
    x: number;
    y: number;
    items: EmbeddedFlowShellContextMenuItem[];
  };
  drawer: {
    open: boolean;
    title: string;
    kind: string;
    rows: EmbeddedFlowShellDetailRow[];
    actions: EmbeddedFlowShellDetailAction[];
  };
  nodeInfo: {
    open: boolean;
    point: EmbeddedFlowShellPoint | null;
    preview: EmbeddedFlowShellNodeInfoPreview | null;
    category: string;
    userName: string;
    accountLabel: string;
    accountValue: string;
    accountValueMono: boolean;
    accounts: string[];
    moreCount: number;
  };
  edgeInfo: EmbeddedFlowShellEdgeTxnOverlayState & {
    open: boolean;
    point: EmbeddedFlowShellPoint | null;
  };
  edgeLabel: {
    open: boolean;
    editable: boolean;
    title: string;
    value: string;
    placeholder: string;
    hint: string;
    confirmLabel: string;
    cancelLabel: string;
    readonly: EmbeddedFlowShellEdgeTxnOverlayState & {
      open: boolean;
    };
  };
  txnModal: {
    open: boolean;
    title: string;
    hint: string;
    loading: boolean;
    error: string;
    requestKey: string;
    columns: EmbeddedFlowShellTxnColumn[];
    rows: EmbeddedFlowShellTxnRow[];
    emptyText: string;
    sortCol: string;
    sortDir: string;
  };
}

export interface EmbeddedFlowShellState {
  schemaVersion: number;
  caseId: string;
  caseName: string;
  leftCollapsed: boolean;
  tab: "byName" | "byCard";
  search: string;
  treeSemanticStatus: "uninitialized" | "source_unavailable" | "blocked";
  graphSearch: {
    query: string;
    focusToken: number;
  };
  projection: {
    mode: string;
    canExpand: boolean;
    selectedNodeCount: number;
  };
  treeData: EmbeddedFlowShellTreeGroup[];
  expandedIds: string[];
  selectedIds: string[];
  selectedCount: number;
  filters: {
    dir: "all" | "in" | "out";
    hop: number;
    minAmount: number;
    maxEdges: number;
  };
  layout: {
    preset: string;
    selected: boolean;
    direction: Record<string, string>;
    edgeRouting: Record<string, boolean>;
  };
  analysis: {
    encodeWidth: boolean;
    encodeColor: boolean;
    nodeScale: boolean;
    filterActive: boolean;
    filterMin: number | null;
    filterMax: number | null;
    collapseChildren: boolean;
  };
  graphStats: FlowSummaryProjection;
  ops: {
    graphLoading: boolean;
    edgeDetail: boolean;
    canUndo: boolean;
    canRedo: boolean;
  };
  style: {
    fontFamilyLabel: string;
    fontSizeLabel: string;
    fontBold: boolean;
    fontItalic: boolean;
    fontUnderline: boolean;
    fontShadow: boolean;
    textColor: string;
    outlineColor: string;
    nodeFill: string;
    lineStyleLabel: string;
    lineWidthLabel: string;
    lineArrowLabel: string;
    nodeShapeLabel: string;
    iconSizeLabel: string;
    createNodeMode: boolean;
    canEditEdgeDirection: boolean;
  };
  views: EmbeddedFlowShellView[];
  activeViewId: string;
  overlay: EmbeddedFlowShellOverlayState;
}

export interface EmbeddedFlowShellBridge {
  getState: () => EmbeddedFlowShellState;
  getPerfSnapshot: () => Record<string, unknown>;
  subscribe: (
    listener: (state: EmbeddedFlowShellState, meta?: { reason?: string }) => void
  ) => (() => void) | void;
  commands: {
    triggerControl: (controlId: string, eventType?: string) => boolean;
    style: {
      openFontFamily: (anchorRect: EmbeddedFlowAnchorRect) => void;
      openFontSize: (anchorRect: EmbeddedFlowAnchorRect) => void;
      closePopover: () => void;
      applyPopoverOption: (value: string | number) => void;
      applyPopoverColor: (color: string, close?: boolean) => void;
      setIconLibraryTab: (tabId: string) => void;
      setIconLibraryQuery: (query: string) => void;
      applyIconSymbol: (iconId: string) => void;
      increaseFontSize: () => void;
      decreaseFontSize: () => void;
      toggleBold: () => void;
      toggleItalic: () => void;
      toggleUnderline: () => void;
      toggleShadow: () => void;
      openTextColor: (anchorRect: EmbeddedFlowAnchorRect) => void;
      openOutlineColor: (anchorRect: EmbeddedFlowAnchorRect) => void;
      openLineStyle: (anchorRect: EmbeddedFlowAnchorRect) => void;
      openLineWidth: (anchorRect: EmbeddedFlowAnchorRect) => void;
      openLineArrow: (anchorRect: EmbeddedFlowAnchorRect) => void;
      openNodeShape: (anchorRect: EmbeddedFlowAnchorRect) => void;
      openNodeFill: (anchorRect: EmbeddedFlowAnchorRect) => void;
      openIconSize: (anchorRect: EmbeddedFlowAnchorRect) => void;
      toggleCreateNodeMode: () => void;
      openIconLibrary: (anchorRect: EmbeddedFlowAnchorRect) => void;
      createLinkFromSelection: () => void;
    };
    layout: {
      setPreset: (preset: "compact" | "network" | "hierarchy" | "flow") => void;
      openEdgeRoutingMenu: (mode: "hierarchy" | "flow", anchorRect: EmbeddedFlowAnchorRect) => void;
    };
    analysis: {
      openFilter: (anchorRect: EmbeddedFlowAnchorRect) => void;
      closeFilter: () => void;
      setFilterMin: (value: number | string) => void;
      setFilterMax: (value: number | string) => void;
      clearFilter: () => void;
      toggleCollapseChildren: () => void;
      assignGroupTag: (index: 1 | 2 | 3) => void;
      openGroupOps: (anchorRect: EmbeddedFlowAnchorRect) => void;
      toggleEncodeWidth: () => void;
      toggleEncodeColor: () => void;
      toggleNodeScale: () => void;
    };
    ops: {
      mergeNodes: () => void;
      mergeNodesNet: () => void;
      openMergeMenu: (anchorRect: EmbeddedFlowAnchorRect) => void;
      fitGraph: () => void;
      openExport: (anchorRect: EmbeddedFlowAnchorRect) => void;
      closeExport: () => void;
      setExportScale: (scale: number) => void;
      exportFormat: (format: "png" | "jpg" | "pdf") => void;
      toggleEdgeDetail: () => void;
      redo: () => void;
      undo: () => void;
    };
    graph: {
      setSearch: (value: string) => void;
      clearSearch: () => void;
      runSearch: () => void;
      expandViewport: () => void;
      expandSelectionPath: () => void;
    };
    canvas: {
      clearSelection: () => void;
      selectAll: () => void;
      selectAdjacent: (anchorId: string, options?: { append?: boolean; includeCurrentSelection?: boolean }) => void;
      fitToSelection: () => void;
      captureViewport: () => Record<string, unknown> | null;
      restoreViewport: (viewport: Record<string, unknown>) => boolean;
      focusItem: (itemId: string) => boolean;
      zoomIn: () => void;
      zoomOut: () => void;
      resetViewport: () => void;
    };
    setLeftCollapsed: (collapsed: boolean) => void;
    toggleLeftCollapsed: () => void;
    setTab: (tab: "byName" | "byCard") => Promise<void> | void;
    setSearch: (value: string) => void;
    toggleGroup: (groupId: string) => void;
    toggleGroupSelect: (groupId: string) => void;
    toggleItemSelect: (itemId: string) => void;
    selectAll: () => void;
    clearSelection: () => void;
    setDir: (dir: "all" | "in" | "out") => void;
    setHop: (value: number | string) => void;
    setMinAmount: (value: number | string) => void;
    setMaxEdges: (value: number | string) => void;
    buildGraph: (options?: Record<string, unknown>) => Promise<void> | void;
    clearGraph: () => void;
    addView: () => void;
    activateView: (viewId: string) => void;
    saveCurrentView: () => Promise<void> | void;
    closeView: (viewId: string) => Promise<void> | void;
    renameView: (viewId: string) => Promise<void> | void;
    reorderViews: (fromId: string, targetId: string, place?: "before" | "after") => Promise<void> | void;
    openViewContextMenu: (viewId: string, x: number, y: number) => void;
    fitGraph: (options?: Record<string, unknown>) => void;
    refit: () => void;
    overlay: {
      runContextMenuAction: (actionId: string) => Promise<void> | void;
      closeContextMenu: () => void;
      closeDrawer: () => void;
      runDrawerAction: (actionId: string) => Promise<void> | void;
      closeNodeInfo: () => void;
      closeEdgeInfo: () => void;
      closeEdgeLabel: () => void;
      setEdgeLabelValue: (value: string) => void;
      saveEdgeLabel: () => void;
      openEdgeTxnDetail: () => void;
      closeTxnModal: () => void;
      toggleTxnSort: (colKey: string) => void;
      setTxnColumnWidth: (colKey: string, width: number) => void;
    };
  };
}

export interface EmbeddedFlowShellMounts {
  left: HTMLElement | null;
  gap: HTMLElement | null;
  style: HTMLElement | null;
  layout: HTMLElement | null;
  analyze: HTMLElement | null;
  ops: HTMLElement | null;
  views: HTMLElement | null;
  emptyView: HTMLElement | null;
  graphSearch: HTMLElement | null;
  graphStats: HTMLElement | null;
  overlay: HTMLElement | null;
}

type EmbeddedFlowWindow = Window &
  typeof globalThis & {
    __ANALYTIX_VIZ_SHELL__?: EmbeddedFlowShellBridge;
    __ANALYTIX_FLOW_RUNTIME_BACKEND__?: unknown;
    __ANALYTIX_FLOW_RUNTIME_STORE__?: EmbeddedFlowRuntimeStore;
    __ANALYTIX_FLOW_BRIDGE_MAPPER__?: Record<string, unknown>;
    __ANALYTIX_FLOW_RUNTIME_BOOT__?: Record<string, unknown>;
    __ANALYTIX_FLOW_RUNTIME_BOOT_PROMISE__?: Promise<void>;
    __ANALYTIX_FLOW_RUNTIME_BOOT_ERROR__?: string;
    __ANALYTIX_TASK__?: {
      show: (detail: TaskEventDetail) => string;
      success: (detail: TaskEventDetail) => Promise<void>;
      dismiss: (id: string) => void;
    };
    __ANALYTIX_CONFIRM__?: (detail: ConfirmEventDetail) => Promise<boolean>;
    __ANALYTIX_PROMPT__?: (detail: PromptEventDetail) => Promise<PromptValues | null>;
    __analytixOnLoadFinished?: () => void;
    __analytixVizFromStats?: (payload: EmbeddedFlowRequest) => void;
    __analytixVizRefit?: () => void;
  };

interface ParsedEmbeddedFlowPage {
  bodyHtml: string;
  scriptUrls: string[];
  runtimeLoaderUrl: string;
  appRuntimeScriptUrl: string;
  stylesText: string;
  workerBaseUrl: string;
}

export interface FlowCanvasAdapter {
  attach: (container: HTMLElement) => Promise<void>;
  detach: (container: HTMLElement) => void;
  getShellBridge: () => EmbeddedFlowShellBridge | null;
  getShellMounts: () => EmbeddedFlowShellMounts;
  openRequest: (payload: EmbeddedFlowRequest) => void;
  refit: () => void;
  setCaseId: (caseId: string) => void;
}

export type EmbeddedFlowRuntime = FlowCanvasAdapter;

let runtimePromise: Promise<FlowCanvasAdapter> | null = null;
let runtimeAuthorityKey = "";

function resolveEmbeddedRuntimeUrl(relativePath: string): string {
  const normalized = String(relativePath || "").replace(/^\/+/, "");
  return new URL(`flow-runtime/${normalized}`, window.location.href).toString();
}

function trimTrailingSlash(value: string): string {
  return value.endsWith("/") ? value.slice(0, -1) : value;
}

const EMBEDDED_THEME_CUSTOM_PROPERTIES = [
  "--font-sans",
  "--font-mono",
  "--motion-fast",
  "--motion-medium",
  "--color-bg",
  "--color-bg-elevated",
  "--color-bg-subtle",
  "--color-text",
  "--color-text-muted",
  "--color-border",
  "--color-accent",
  "--color-accent-strong",
  "--color-success",
  "--color-warning",
  "--color-danger",
  "--shadow-card",
  "--shadow-elevated",
  "--gradient-atmosphere",
  "--program-shell-bg",
  "--program-shell-border",
  "--radius-sm",
  "--radius-md",
  "--radius-lg",
  "--radius-xl",
  "--space-1",
  "--space-2",
  "--space-3",
  "--space-4",
  "--space-5",
  "--space-6"
];

function syncEmbeddedThemeToIframe(targetDocument: Document, sourceDocument: Document = document): void {
  const sourceRoot = sourceDocument.documentElement;
  const targetRoot = targetDocument.documentElement;
  const sourceWindow = sourceDocument.defaultView || window;
  const sourceStyles = sourceWindow.getComputedStyle(sourceRoot);

  EMBEDDED_THEME_CUSTOM_PROPERTIES.forEach((propertyName) => {
    const value = sourceStyles.getPropertyValue(propertyName).trim();
    if (value) {
      targetRoot.style.setProperty(propertyName, value);
    } else {
      targetRoot.style.removeProperty(propertyName);
    }
  });

  const themeName = String(sourceRoot.dataset.theme || "").trim();
  if (themeName) {
    targetRoot.dataset.theme = themeName;
  } else {
    delete targetRoot.dataset.theme;
  }

  const colorScheme = String(sourceStyles.getPropertyValue("color-scheme") || sourceRoot.style.colorScheme || "").trim();
  if (colorScheme) {
    targetRoot.style.colorScheme = colorScheme;
  } else {
    targetRoot.style.removeProperty("color-scheme");
  }
}

function bindFlowThemeSync(targetDocument: Document, sourceDocument: Document = document): () => void {
  const sourceRoot = sourceDocument.documentElement;
  syncEmbeddedThemeToIframe(targetDocument, sourceDocument);

  const observer = new MutationObserver(() => {
    syncEmbeddedThemeToIframe(targetDocument, sourceDocument);
  });
  observer.observe(sourceRoot, {
    attributes: true,
    attributeFilter: ["data-theme", "style", "class"]
  });

  const handleMediaChange = (): void => {
    syncEmbeddedThemeToIframe(targetDocument, sourceDocument);
  };

  const media =
    typeof window.matchMedia === "function" ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  if (media) {
    if (typeof media.addEventListener === "function") {
      media.addEventListener("change", handleMediaChange);
    } else if (typeof media.addListener === "function") {
      media.addListener(handleMediaChange);
    }
  }

  return () => {
    observer.disconnect();
    if (media) {
      if (typeof media.removeEventListener === "function") {
        media.removeEventListener("change", handleMediaChange);
      } else if (typeof media.removeListener === "function") {
        media.removeListener(handleMediaChange);
      }
    }
  };
}

async function loadEmbeddedFlowPage(): Promise<ParsedEmbeddedFlowPage> {
  const parsed = new DOMParser().parseFromString(flowIndexHtml, "text/html");

  parsed.querySelectorAll("script").forEach((node) => node.remove());

  const appRuntimeScriptUrl = resolveEmbeddedRuntimeUrl(EMBEDDED_FLOW_RUNTIME_APP_PATH);
  const runtimeLoaderUrl = resolveEmbeddedRuntimeUrl(EMBEDDED_FLOW_RUNTIME_LOADER_PATH);
  const workerBaseUrl = new URL("./", appRuntimeScriptUrl).toString();

  return {
    bodyHtml: parsed.body.innerHTML,
    scriptUrls: resolveEmbeddedFlowRuntimeScriptUrls(resolveEmbeddedRuntimeUrl),
    runtimeLoaderUrl,
    appRuntimeScriptUrl,
    stylesText: `${statsBaseCss}\n\n${flowRuntimeCss}\n\n${workbenchUiKitCss}\n\n${txnDetailDialogCss}`,
    workerBaseUrl
  };
}

function appendShellMount(container: Element | null, name: string): HTMLElement | null {
  if (!container || container.nodeType !== 1 || typeof (container as Element).appendChild !== "function") {
    return null;
  }
  const host = container as Element;
  const doc = host.ownerDocument || document;
  const mount = doc.createElement("div");
  mount.className = "analytix-react-shell-host";
  mount.dataset.shellMount = name;
  host.appendChild(mount);
  return mount;
}

function createShellMounts(bodyElement: HTMLElement): EmbeddedFlowShellMounts {
  bodyElement.classList.add("analytix-react-shell-active");
  const app = bodyElement.querySelector(".app");
  const doc = bodyElement.ownerDocument || document;
  const overlay = doc.createElement("div");
  overlay.className = "analytix-react-shell-page-overlay";
  app?.appendChild(overlay);

  const gap = doc.createElement("div");
  gap.className = "analytix-react-shell-host analytix-react-shell-gap-mount";
  gap.dataset.shellMount = "gap";
  overlay.appendChild(gap);

  return {
    left: appendShellMount(bodyElement.querySelector("#leftCard"), "left"),
    gap,
    style: appendShellMount(bodyElement.querySelector(".styleGroup"), "style"),
    layout: appendShellMount(bodyElement.querySelector(".layoutGroup"), "layout"),
    analyze: appendShellMount(bodyElement.querySelector(".analyzeGroup"), "analyze"),
    ops: appendShellMount(bodyElement.querySelector(".opsGroup"), "ops"),
    views: appendShellMount(bodyElement.querySelector("#viewsBar"), "views"),
    emptyView: appendShellMount(bodyElement.querySelector("#graphWrap"), "empty-view"),
    graphSearch: appendShellMount(bodyElement.querySelector("#graphSearchFloat"), "graph-search"),
    graphStats: appendShellMount(bodyElement.querySelector("#graphStatsFloat"), "graph-stats"),
    overlay
  };
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function toTextValue(value: unknown, fallback = ""): string {
  const next = value == null ? fallback : String(value);
  return next.trim();
}

function toBooleanValue(value: unknown, fallback = false): boolean {
  if (typeof value === "boolean") {
    return value;
  }
  if (typeof value === "string") {
    const next = value.trim().toLowerCase();
    if (next === "true" || next === "1") return true;
    if (next === "false" || next === "0" || next === "") return false;
  }
  if (typeof value === "number") {
    return Number.isFinite(value) ? value !== 0 : fallback;
  }
  if (value == null) {
    return fallback;
  }
  return Boolean(value);
}

function toFiniteNumberValue(value: unknown, fallback: number): number {
  const next = Number(value);
  return Number.isFinite(next) ? next : fallback;
}

function clampNumberValue(value: unknown, fallback: number, minValue: number, maxValue: number): number {
  const next = toFiniteNumberValue(value, fallback);
  return Math.min(maxValue, Math.max(minValue, next));
}

function resolveShellStateSchemaVersion(source: Record<string, unknown>, fallback: number): number {
  const rawVersion =
    source.schemaVersion ??
    source.schema_version ??
    source.shellStateSchemaVersion ??
    source.shell_state_schema_version;
  const sourceVersion = Math.max(1, Math.round(toFiniteNumberValue(rawVersion, fallback)));
  if (sourceVersion !== FLOW_SHELL_STATE_SCHEMA_VERSION && !warnedShellStateSchemaVersions.has(sourceVersion)) {
    warnedShellStateSchemaVersions.add(sourceVersion);
    console.warn(
      `[flow-runtime] shell state schema ${sourceVersion} detected, expected ${FLOW_SHELL_STATE_SCHEMA_VERSION}; applying schema sanitizer.`
    );
  }
  return FLOW_SHELL_STATE_SCHEMA_VERSION;
}

function toStringArray(value: unknown): string[] {
  const source = Array.isArray(value) ? value : typeof value === "string" ? [value] : [];
  const result: string[] = [];
  const seen = new Set<string>();
  source.forEach((item) => {
    const next = toTextValue(item);
    if (!next || seen.has(next)) {
      return;
    }
    seen.add(next);
    result.push(next);
  });
  return result;
}

function sanitizeAnchorRect(value: unknown): EmbeddedFlowAnchorRect | null {
  const source = asRecord(value);
  const left = toFiniteNumberValue(source.left, NaN);
  const top = toFiniteNumberValue(source.top, NaN);
  const right = toFiniteNumberValue(source.right, NaN);
  const bottom = toFiniteNumberValue(source.bottom, NaN);
  const width = toFiniteNumberValue(source.width, NaN);
  const height = toFiniteNumberValue(source.height, NaN);
  if (![left, top, right, bottom, width, height].every(Number.isFinite)) {
    return null;
  }
  return { left, top, right, bottom, width, height };
}

function sanitizePoint(value: unknown): EmbeddedFlowShellPoint | null {
  const source = asRecord(value);
  const x = toFiniteNumberValue(source.x, NaN);
  const y = toFiniteNumberValue(source.y, NaN);
  if (!Number.isFinite(x) || !Number.isFinite(y)) {
    return null;
  }
  return { x, y };
}

function sanitizeContextMenuItems(value: unknown): EmbeddedFlowShellContextMenuItem[] {
  return (Array.isArray(value) ? value : []).map((item) => {
    const source = asRecord(item);
    return {
      id: toTextValue(source.id),
      label: projectOrdinaryFieldValue("", source.label),
      disabled: toBooleanValue(source.disabled, false),
      separator: toBooleanValue(source.separator, false)
    };
  });
}

function sanitizeStylePopoverOptions(value: unknown): EmbeddedFlowShellStylePopoverOption[] {
  return (Array.isArray(value) ? value : []).map((item) => {
    const source = asRecord(item);
    const dash = Array.isArray(source.dash)
      ? source.dash
          .map((part) => toFiniteNumberValue(part, NaN))
          .filter((part) => Number.isFinite(part))
      : undefined;
    return {
      value: toTextValue(source.value),
      label: projectOrdinaryFieldValue("", source.label || source.value),
      ...(dash && dash.length ? { dash } : {})
    };
  });
}

function sanitizeStylePopoverTabs(value: unknown): EmbeddedFlowShellStylePopoverTab[] {
  return (Array.isArray(value) ? value : []).map((item) => {
    const source = asRecord(item);
    return {
      id: toTextValue(source.id),
      label: projectOrdinaryFieldValue("", source.label || source.id)
    };
  });
}

function sanitizeDetailRows(value: unknown): EmbeddedFlowShellDetailRow[] {
  return (Array.isArray(value) ? value : []).map((item) => {
    const source = asRecord(item);
    const label = projectOrdinaryFieldValue("", source.label);
    return {
      label,
      value: projectOrdinaryFieldValue(label, source.value),
      mono: toBooleanValue(source.mono, false),
      tone: toTextValue(source.tone)
    };
  });
}

function sanitizeDetailActions(value: unknown): EmbeddedFlowShellDetailAction[] {
  return (Array.isArray(value) ? value : []).map((item) => {
    const source = asRecord(item);
    return {
      id: toTextValue(source.id),
      label: projectOrdinaryFieldValue("", source.label),
      variant: toTextValue(source.variant || "default") || "default"
    };
  });
}

function sanitizeEdgeTxnRows(value: unknown): EmbeddedFlowShellEdgeInfoRow[] {
  return (Array.isArray(value) ? value : []).map((item) => {
    const source = asRecord(item);
    return {
      time: toTextValue(source.time),
      amountText: toTextValue(source.amountText),
      amountTone: toTextValue(source.amountTone)
    };
  });
}

function sanitizeTxnColumns(value: unknown): EmbeddedFlowShellTxnColumn[] {
  return (Array.isArray(value) ? value : []).map((item) => {
    const source = asRecord(item);
    const sortDir = toTextValue(source.sortDir).toLowerCase() === "desc" ? "desc" : "asc";
    return {
      key: toTextValue(source.key),
      title: toTextValue(source.title || source.key),
      width: clampNumberValue(source.width, 140, 72, 640),
      sortable: toBooleanValue(source.sortable, false),
      sorted: toBooleanValue(source.sorted, false),
      sortDir
    };
  });
}

function sanitizeTxnRows(value: unknown): EmbeddedFlowShellTxnRow[] {
  return (Array.isArray(value) ? value : []).map((item, index) => {
    const source = asRecord(item);
    const rawCells = asRecord(source.cells);
    const cells = Object.entries(rawCells).reduce<Record<string, EmbeddedFlowShellTxnCell>>((accumulator, [key, rawCell]) => {
      const cell = asRecord(rawCell);
      const alignRaw = toTextValue(cell.align).toLowerCase();
      accumulator[key] = {
        text: projectOrdinaryFieldValue(key, cell.text),
        mono: toBooleanValue(cell.mono, false),
        align: alignRaw === "center" || alignRaw === "right" ? alignRaw : "left",
        tone: toTextValue(cell.tone)
      };
      return accumulator;
    }, {});
    return {
      key: `flow-txn-row-${index}`,
      cells
    };
  });
}

function sanitizeTreePresentationToken(value: unknown, kind: "group" | "account"): string {
  const token = toTextValue(value);
  return new RegExp(`^flow-tree-${kind}-\\d+-\\d+$`).test(token) ? token : "";
}

function sanitizeTreeData(value: unknown): EmbeddedFlowShellState["treeData"] {
  return (Array.isArray(value) ? value : []).map((group) => {
    const source = asRecord(group);
    const meta = toTextValue(source.meta);
    return {
      id: sanitizeTreePresentationToken(source.id, "group"),
      title: projectOrdinaryFieldValue(meta === "未登记户名" ? "account_no" : "", source.title),
      meta: projectOrdinaryFieldValue("", meta),
      extra: projectOrdinaryFieldValue("", source.extra),
      items: (Array.isArray(source.items) ? source.items : []).map((item) => {
        const next = asRecord(item);
        return {
          id: sanitizeTreePresentationToken(next.id, "account"),
          title: projectOrdinaryFieldValue("account_no", next.title),
          sub: projectOrdinaryFieldValue("", next.sub)
        };
      }).filter((item) => item.id)
    };
  }).filter((group) => group.id);
}

function sanitizeViews(value: unknown): EmbeddedFlowShellView[] {
  return (Array.isArray(value) ? value : []).map((item) => {
    const source = asRecord(item);
    const counts = asRecord(source.counts);
    return {
      id: toTextValue(source.id),
      title: projectOrdinaryFieldValue("node_id", source.title || "图"),
      saved: toBooleanValue(source.saved, false),
      active: toBooleanValue(source.active, false),
      counts: {
        nodes: Math.max(0, Math.round(toFiniteNumberValue(counts.nodes, 0))),
        edges: Math.max(0, Math.round(toFiniteNumberValue(counts.edges, 0)))
      }
    };
  });
}

function sanitizeNodeInfoPreview(value: unknown): EmbeddedFlowShellNodeInfoPreview | null {
  const source = asRecord(value);
  const size = toFiniteNumberValue(source.size, NaN);
  if (!Number.isFinite(size)) {
    return null;
  }
  return {
    size: clampNumberValue(size, 32, 12, 120),
    fill: toTextValue(source.fill || "rgba(255,255,255,0.05)") || "rgba(255,255,255,0.05)",
    stroke: toTextValue(source.stroke || "#1f6feb") || "#1f6feb",
    lineWidth: clampNumberValue(source.lineWidth, 2, 0, 24)
  };
}

function sanitizeDirectionMap(value: unknown): Record<string, string> {
  return Object.entries(asRecord(value)).reduce<Record<string, string>>((accumulator, [key, rawValue]) => {
    const nextKey = toTextValue(key);
    if (!nextKey) {
      return accumulator;
    }
    accumulator[nextKey] = toTextValue(rawValue);
    return accumulator;
  }, {});
}

function sanitizeBooleanMap(value: unknown): Record<string, boolean> {
  return Object.entries(asRecord(value)).reduce<Record<string, boolean>>((accumulator, [key, rawValue]) => {
    const nextKey = toTextValue(key);
    if (!nextKey) {
      return accumulator;
    }
    accumulator[nextKey] = toBooleanValue(rawValue, false);
    return accumulator;
  }, {});
}

function sanitizeEdgeTxnOverlayState(
  value: unknown,
  fallbackTitle: string
): EmbeddedFlowShellEdgeTxnOverlayState {
  const source = asRecord(value);
  return {
    title: projectOrdinaryFieldValue("node_id", source.title || fallbackTitle) || fallbackTitle,
    amountText: toTextValue(source.amountText),
    amountTone: toTextValue(source.amountTone),
    countText: toTextValue(source.countText || "0") || "0",
    countClickable: toBooleanValue(source.countClickable, false),
    loading: toBooleanValue(source.loading, false),
    error: projectOrdinaryFieldValue("", source.error),
    rows: sanitizeEdgeTxnRows(source.rows)
  };
}

function createDefaultOverlayState(): EmbeddedFlowShellOverlayState {
  return {
    filter: {
      open: false,
      anchorRect: null
    },
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
      icons: []
    },
    menu: {
      open: false,
      anchorRect: null,
      sourceKey: "",
      items: []
    },
    exportPanel: {
      open: false,
      anchorRect: null,
      scale: 1
    },
    contextMenu: {
      open: false,
      x: 0,
      y: 0,
      items: []
    },
    drawer: {
      open: false,
      title: "",
      kind: "",
      rows: [],
      actions: []
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
      moreCount: 0
    },
    edgeInfo: {
      open: false,
      point: null,
      title: "资金交易",
      amountText: "",
      amountTone: "",
      countText: "0",
      countClickable: false,
      loading: false,
      error: "",
      rows: []
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
        rows: []
      }
    },
    txnModal: {
      open: false,
      title: "交易明细",
      hint: "",
      loading: false,
      error: "",
      requestKey: "",
      columns: [],
      rows: [],
      emptyText: "暂无交易明细",
      sortCol: "txn_time",
      sortDir: "asc"
    }
  };
}

function sanitizeOverlayState(value: unknown): EmbeddedFlowShellOverlayState {
  const source = asRecord(value);
  const defaults = createDefaultOverlayState();
  const styleKind = toTextValue(asRecord(source.stylePopover).kind);
  const nextStyleKind =
    styleKind === "list" ||
    styleKind === "color" ||
    styleKind === "lineStyle" ||
    styleKind === "arrow" ||
    styleKind === "shape" ||
    styleKind === "iconLibrary"
      ? styleKind
      : "";
  const edgeInfoBase = sanitizeEdgeTxnOverlayState(source.edgeInfo, defaults.edgeInfo.title);
  const readonlyBase = sanitizeEdgeTxnOverlayState(asRecord(source.edgeLabel).readonly, defaults.edgeLabel.readonly.title);
  const txnModalSource = asRecord(source.txnModal);
  const txnSortDir = toTextValue(txnModalSource.sortDir).toLowerCase() === "desc" ? "desc" : "asc";
  return {
    filter: {
      open: toBooleanValue(asRecord(source.filter).open, defaults.filter.open),
      anchorRect: sanitizeAnchorRect(asRecord(source.filter).anchorRect)
    },
    stylePopover: {
      open: toBooleanValue(asRecord(source.stylePopover).open, defaults.stylePopover.open),
      anchorRect: sanitizeAnchorRect(asRecord(source.stylePopover).anchorRect),
      sourceKey: toTextValue(asRecord(source.stylePopover).sourceKey),
      kind: nextStyleKind,
      currentValue: toTextValue(asRecord(source.stylePopover).currentValue),
      options: sanitizeStylePopoverOptions(asRecord(source.stylePopover).options),
      commonColors: toStringArray(asRecord(source.stylePopover).commonColors),
      recentColors: toStringArray(asRecord(source.stylePopover).recentColors),
      iconTabs: sanitizeStylePopoverTabs(asRecord(source.stylePopover).iconTabs),
      activeTab: toTextValue(asRecord(source.stylePopover).activeTab),
      iconQuery: toTextValue(asRecord(source.stylePopover).iconQuery),
      icons: toStringArray(asRecord(source.stylePopover).icons)
    },
    menu: {
      open: toBooleanValue(asRecord(source.menu).open, defaults.menu.open),
      anchorRect: sanitizeAnchorRect(asRecord(source.menu).anchorRect),
      sourceKey: toTextValue(asRecord(source.menu).sourceKey),
      items: sanitizeContextMenuItems(asRecord(source.menu).items)
    },
    exportPanel: {
      open: toBooleanValue(asRecord(source.exportPanel).open, defaults.exportPanel.open),
      anchorRect: sanitizeAnchorRect(asRecord(source.exportPanel).anchorRect),
      scale: clampNumberValue(asRecord(source.exportPanel).scale, defaults.exportPanel.scale, 0.1, 8)
    },
    contextMenu: {
      open: toBooleanValue(asRecord(source.contextMenu).open, defaults.contextMenu.open),
      x: toFiniteNumberValue(asRecord(source.contextMenu).x, defaults.contextMenu.x),
      y: toFiniteNumberValue(asRecord(source.contextMenu).y, defaults.contextMenu.y),
      items: sanitizeContextMenuItems(asRecord(source.contextMenu).items)
    },
    drawer: {
      open: toBooleanValue(asRecord(source.drawer).open, defaults.drawer.open),
      title: projectOrdinaryFieldValue("node_id", asRecord(source.drawer).title),
      kind: toTextValue(asRecord(source.drawer).kind),
      rows: sanitizeDetailRows(asRecord(source.drawer).rows),
      actions: sanitizeDetailActions(asRecord(source.drawer).actions)
    },
    nodeInfo: {
      open: toBooleanValue(asRecord(source.nodeInfo).open, defaults.nodeInfo.open),
      point: sanitizePoint(asRecord(source.nodeInfo).point),
      preview: sanitizeNodeInfoPreview(asRecord(source.nodeInfo).preview),
      category: projectOrdinaryFieldValue("", asRecord(source.nodeInfo).category),
      userName: projectOrdinaryFieldValue("", asRecord(source.nodeInfo).userName),
      accountLabel: toTextValue(asRecord(source.nodeInfo).accountLabel),
      accountValue: projectOrdinaryFieldValue("account_no", asRecord(source.nodeInfo).accountValue),
      accountValueMono: toBooleanValue(asRecord(source.nodeInfo).accountValueMono, defaults.nodeInfo.accountValueMono),
      accounts: toStringArray(asRecord(source.nodeInfo).accounts).map((account) =>
        projectOrdinaryFieldValue("account_no", account)
      ),
      moreCount: Math.max(0, Math.round(toFiniteNumberValue(asRecord(source.nodeInfo).moreCount, defaults.nodeInfo.moreCount)))
    },
    edgeInfo: {
      open: toBooleanValue(asRecord(source.edgeInfo).open, defaults.edgeInfo.open),
      point: sanitizePoint(asRecord(source.edgeInfo).point),
      ...edgeInfoBase
    },
    edgeLabel: {
      open: toBooleanValue(asRecord(source.edgeLabel).open, defaults.edgeLabel.open),
      editable: toBooleanValue(asRecord(source.edgeLabel).editable, defaults.edgeLabel.editable),
      title:
        projectOrdinaryFieldValue("", asRecord(source.edgeLabel).title || defaults.edgeLabel.title) ||
        defaults.edgeLabel.title,
      value: projectOrdinaryFieldValue("node_id", asRecord(source.edgeLabel).value),
      placeholder:
        toTextValue(asRecord(source.edgeLabel).placeholder || defaults.edgeLabel.placeholder) || defaults.edgeLabel.placeholder,
      hint: toTextValue(asRecord(source.edgeLabel).hint || defaults.edgeLabel.hint) || defaults.edgeLabel.hint,
      confirmLabel:
        toTextValue(asRecord(source.edgeLabel).confirmLabel || defaults.edgeLabel.confirmLabel) ||
        defaults.edgeLabel.confirmLabel,
      cancelLabel:
        toTextValue(asRecord(source.edgeLabel).cancelLabel || defaults.edgeLabel.cancelLabel) || defaults.edgeLabel.cancelLabel,
      readonly: {
        open: toBooleanValue(asRecord(asRecord(source.edgeLabel).readonly).open, defaults.edgeLabel.readonly.open),
        ...readonlyBase
      }
    },
    txnModal: {
      open: toBooleanValue(txnModalSource.open, defaults.txnModal.open),
      title: projectOrdinaryFieldValue("node_id", txnModalSource.title || defaults.txnModal.title) || defaults.txnModal.title,
      hint: projectOrdinaryFieldValue("", txnModalSource.hint),
      loading: toBooleanValue(txnModalSource.loading, defaults.txnModal.loading),
      error: projectOrdinaryFieldValue("", txnModalSource.error),
      requestKey: toTextValue(txnModalSource.requestKey),
      columns: sanitizeTxnColumns(txnModalSource.columns),
      rows: sanitizeTxnRows(txnModalSource.rows),
      emptyText: toTextValue(txnModalSource.emptyText || defaults.txnModal.emptyText) || defaults.txnModal.emptyText,
      sortCol: toTextValue(txnModalSource.sortCol || defaults.txnModal.sortCol) || defaults.txnModal.sortCol,
      sortDir: txnSortDir
    }
  };
}

function createDefaultShellState(): EmbeddedFlowShellState {
  return {
    schemaVersion: FLOW_SHELL_STATE_SCHEMA_VERSION,
    caseId: "",
    caseName: "",
    leftCollapsed: false,
    tab: "byName",
    search: "",
    treeSemanticStatus: "uninitialized",
    graphSearch: {
      query: "",
      focusToken: 0
    },
    projection: {
      mode: "full",
      canExpand: false,
      selectedNodeCount: 0
    },
    treeData: [],
    expandedIds: [],
    selectedIds: [],
    selectedCount: 0,
    filters: {
      dir: "all",
      hop: 1,
      minAmount: 0,
      maxEdges: 800
    },
    layout: {
      preset: "compact",
      selected: false,
      direction: {},
      edgeRouting: {}
    },
    analysis: {
      encodeWidth: false,
      encodeColor: false,
      nodeScale: false,
      filterActive: false,
      filterMin: null,
      filterMax: null,
      collapseChildren: false
    },
    graphStats: createFlowSummaryBoundary("unloaded", "关联图布局"),
    ops: {
      graphLoading: false,
      edgeDetail: false,
      canUndo: false,
      canRedo: false
    },
    style: {
      fontFamilyLabel: "字体",
      fontSizeLabel: "12",
      fontBold: false,
      fontItalic: false,
      fontUnderline: false,
      fontShadow: false,
      textColor: "#0f172a",
      outlineColor: "#1f6feb",
      nodeFill: "rgba(255,255,255,0.05)",
      lineStyleLabel: "线型",
      lineWidthLabel: "线宽 2",
      lineArrowLabel: "方向",
      nodeShapeLabel: "形状",
      iconSizeLabel: "大小 18",
      createNodeMode: false,
      canEditEdgeDirection: false
    },
    views: [],
    activeViewId: "",
    overlay: createDefaultOverlayState()
  };
}

function sanitizeShellState(value: unknown): EmbeddedFlowShellState {
  const source = asRecord(value);
  const defaults = createDefaultShellState();
  const graphSearch = asRecord(source.graphSearch);
  const filters = asRecord(source.filters);
  const layout = asRecord(source.layout);
  const analysis = asRecord(source.analysis);
  const graphStats = asRecord(source.graphStats);
  const ops = asRecord(source.ops);
  const style = asRecord(source.style);
  const selectedIds = toStringArray(source.selectedIds)
    .map((id) => sanitizeTreePresentationToken(id, "account"))
    .filter(Boolean);
  const tab = toTextValue(source.tab) === "byCard" ? "byCard" : "byName";
  const dir = toTextValue(filters.dir).toLowerCase();
  const layoutPreset = toTextValue(layout.preset);
  const filterMin = toFiniteNumberValue(analysis.filterMin, NaN);
  const filterMax = toFiniteNumberValue(analysis.filterMax, NaN);
  return {
    schemaVersion: resolveShellStateSchemaVersion(source, defaults.schemaVersion),
    caseId: projectOrdinaryFieldValue("", source.caseId),
    caseName: projectOrdinaryFieldValue("", source.caseName),
    leftCollapsed: toBooleanValue(source.leftCollapsed, defaults.leftCollapsed),
    tab,
    search: projectOrdinaryFieldValue("", source.search),
    graphSearch: {
      query: projectOrdinaryFieldValue("node_id", graphSearch.query),
      focusToken: Math.max(0, Math.round(toFiniteNumberValue(graphSearch.focusToken, defaults.graphSearch.focusToken)))
    },
    projection: {
      mode: toTextValue(asRecord(source.projection).mode) || defaults.projection.mode,
      canExpand: toBooleanValue(asRecord(source.projection).canExpand, defaults.projection.canExpand),
      selectedNodeCount: Math.max(
        0,
        Math.round(toFiniteNumberValue(asRecord(source.projection).selectedNodeCount, defaults.projection.selectedNodeCount))
      )
    },
    treeSemanticStatus:
      source.treeSemanticStatus === "source_unavailable"
        ? "source_unavailable"
        : source.treeSemanticStatus === "blocked"
          ? "blocked"
          : "uninitialized",
    treeData: sanitizeTreeData(source.treeData),
    expandedIds: toStringArray(source.expandedIds)
      .map((id) => sanitizeTreePresentationToken(id, "group"))
      .filter(Boolean),
    selectedIds,
    selectedCount: Math.max(
      selectedIds.length,
      Math.round(toFiniteNumberValue(source.selectedCount, selectedIds.length))
    ),
    filters: {
      dir: dir === "in" || dir === "out" ? dir : "all",
      hop: Math.round(clampNumberValue(filters.hop, defaults.filters.hop, 1, 8)),
      minAmount: Math.max(0, toFiniteNumberValue(filters.minAmount, defaults.filters.minAmount)),
      maxEdges: Math.max(50, Math.round(toFiniteNumberValue(filters.maxEdges, defaults.filters.maxEdges)))
    },
    layout: {
      preset: layoutPreset || defaults.layout.preset,
      selected: toBooleanValue(layout.selected, defaults.layout.selected),
      direction: sanitizeDirectionMap(layout.direction),
      edgeRouting: sanitizeBooleanMap(layout.edgeRouting)
    },
    analysis: {
      encodeWidth: toBooleanValue(analysis.encodeWidth, defaults.analysis.encodeWidth),
      encodeColor: toBooleanValue(analysis.encodeColor, defaults.analysis.encodeColor),
      nodeScale: toBooleanValue(analysis.nodeScale, defaults.analysis.nodeScale),
      filterActive: toBooleanValue(analysis.filterActive, defaults.analysis.filterActive),
      filterMin: Number.isFinite(filterMin) ? filterMin : null,
      filterMax: Number.isFinite(filterMax) ? filterMax : null,
      collapseChildren: toBooleanValue(analysis.collapseChildren, defaults.analysis.collapseChildren)
    },
    graphStats: projectFlowSummaryForChrome(graphStats),
    ops: {
      graphLoading: toBooleanValue(ops.graphLoading, defaults.ops.graphLoading),
      edgeDetail: toBooleanValue(ops.edgeDetail, defaults.ops.edgeDetail),
      canUndo: toBooleanValue(ops.canUndo, defaults.ops.canUndo),
      canRedo: toBooleanValue(ops.canRedo, defaults.ops.canRedo)
    },
    style: {
      fontFamilyLabel: toTextValue(style.fontFamilyLabel || defaults.style.fontFamilyLabel) || defaults.style.fontFamilyLabel,
      fontSizeLabel: toTextValue(style.fontSizeLabel || defaults.style.fontSizeLabel) || defaults.style.fontSizeLabel,
      fontBold: toBooleanValue(style.fontBold, defaults.style.fontBold),
      fontItalic: toBooleanValue(style.fontItalic, defaults.style.fontItalic),
      fontUnderline: toBooleanValue(style.fontUnderline, defaults.style.fontUnderline),
      fontShadow: toBooleanValue(style.fontShadow, defaults.style.fontShadow),
      textColor: toTextValue(style.textColor || defaults.style.textColor) || defaults.style.textColor,
      outlineColor: toTextValue(style.outlineColor || defaults.style.outlineColor) || defaults.style.outlineColor,
      nodeFill: toTextValue(style.nodeFill || defaults.style.nodeFill) || defaults.style.nodeFill,
      lineStyleLabel: toTextValue(style.lineStyleLabel || defaults.style.lineStyleLabel) || defaults.style.lineStyleLabel,
      lineWidthLabel: toTextValue(style.lineWidthLabel || defaults.style.lineWidthLabel) || defaults.style.lineWidthLabel,
      lineArrowLabel: toTextValue(style.lineArrowLabel || defaults.style.lineArrowLabel) || defaults.style.lineArrowLabel,
      nodeShapeLabel: toTextValue(style.nodeShapeLabel || defaults.style.nodeShapeLabel) || defaults.style.nodeShapeLabel,
      iconSizeLabel: toTextValue(style.iconSizeLabel || defaults.style.iconSizeLabel) || defaults.style.iconSizeLabel,
      createNodeMode: toBooleanValue(style.createNodeMode, defaults.style.createNodeMode),
      canEditEdgeDirection: toBooleanValue(style.canEditEdgeDirection, defaults.style.canEditEdgeDirection)
    },
    views: sanitizeViews(source.views),
    activeViewId: toTextValue(source.activeViewId),
    overlay: sanitizeOverlayState(source.overlay)
  };
}

export const sanitizeEmbeddedFlowShellState = sanitizeShellState;

function wrapEmbeddedFlowShellBridge(
  bridge: EmbeddedFlowShellBridge | null | undefined,
  options: { getHostPerfSnapshot?: () => Record<string, unknown> } = {}
): EmbeddedFlowShellBridge | null {
  if (!bridge || typeof bridge !== "object") {
    return null;
  }
  return {
    commands: bridge.commands,
    getState: (): EmbeddedFlowShellState => {
      try {
        return sanitizeShellState(bridge.getState());
      } catch {
        return createDefaultShellState();
      }
    },
    getPerfSnapshot: (): Record<string, unknown> => {
      try {
        const snapshot = bridge.getPerfSnapshot();
        const base = snapshot && typeof snapshot === "object" ? snapshot : {};
        const hostPerf = options.getHostPerfSnapshot?.();
        return projectFlowPerfSnapshot({
          ...base,
          ...(hostPerf && typeof hostPerf === "object" ? hostPerf : {}),
        });
      } catch {
        const hostPerf = options.getHostPerfSnapshot?.();
        return projectFlowPerfSnapshot(hostPerf);
      }
    },
    subscribe(listener) {
      if (typeof bridge.subscribe !== "function") {
        return undefined;
      }
      const nextListener = (state: EmbeddedFlowShellState, meta?: { reason?: string }) => {
        listener(sanitizeShellState(state), meta && typeof meta === "object" ? { reason: toTextValue(meta.reason) } : undefined);
      };
      const unsubscribe = bridge.subscribe(nextListener);
      if (typeof unsubscribe === "function") {
        return () => {
          unsubscribe();
        };
      }
      return unsubscribe;
    }
  };
}

async function createEmbeddedFlowCanvasAdapter(
  authority: DataAnalysisRuntimeEndpointAuthority,
): Promise<FlowCanvasAdapter> {
  if (!isDataAnalysisRuntimeEndpointAuthorityCurrent(authority)) {
    throw new Error("data_analysis_backend_authority_invalidated");
  }
  const page = await loadEmbeddedFlowPage();
  if (!isDataAnalysisRuntimeEndpointAuthorityCurrent(authority)) {
    throw new Error("data_analysis_backend_authority_invalidated");
  }
  const host = document.createElement("div");
  host.className = "flow-runtime-host";
  host.style.display = "block";
  host.style.width = "100%";
  host.style.height = "100%";
  host.style.minHeight = "100%";
  const runtimeApiBase = authority.apiBaseUrl;
  let iframe: HTMLIFrameElement | null = null;
  let runtimeStore: EmbeddedFlowRuntimeStore | null = null;
  let bridgeService: EmbeddedFlowBridgeService | null = null;
  let shellBridge: EmbeddedFlowShellBridge | null = null;
  let canvasBridge: ReturnType<EmbeddedFlowRuntimeStore["getBrowserBridge"]> | null = null;
  let bootPromise: Promise<void> | null = null;
  let themeSyncCleanup: (() => void) | null = null;
  let currentCaseId = "";
  let authorityInvalidated = false;
  const runtimeHostMetrics = {
    attachCount: 0,
    attachSuccessCount: 0,
    attachFailureCount: 0,
    selfHealCount: 0,
    selfHealSuccessCount: 0,
    lastDurationMs: 0,
    lastStage: "idle",
    lastErrorCode: "",
    lastAttemptAt: 0,
    lastReadyAt: 0,
    hasCaseContext: false,
    assetLoadCount: 0,
    assetErrorCount: 0,
    lastAssetStage: "",
    lastAssetErrorCode: "",
    shellReadyCount: 0,
    openRequestCount: 0,
    lastOpenRequestAt: 0,
    refitCount: 0,
    lastRefitAt: 0,
    lastSelfHealReason: "",
    lastSelfHealAt: 0,
  };
  let shellMounts: EmbeddedFlowShellMounts = {
    left: null,
    gap: null,
    style: null,
    layout: null,
    analyze: null,
    ops: null,
    views: null,
    emptyView: null,
    graphSearch: null,
    graphStats: null,
    overlay: null,
  };
  const installRuntimeGlobals = (target: EmbeddedFlowWindow | null): void => {
    if (!target) return;
    target.__ANALYTIX_FLOW_RUNTIME_BACKEND__ = runtimeStore?.getBackend?.() || null;
    target.__ANALYTIX_FLOW_RUNTIME_STORE__ = runtimeStore || undefined;
    target.__ANALYTIX_TASK__ = {
      show: (detail: TaskEventDetail) => emitTask(detail),
      success: (detail: TaskEventDetail) => emitTaskSuccess(detail),
      dismiss: (id: string) => dismissTask(id)
    };
    target.__ANALYTIX_CONFIRM__ = (detail: ConfirmEventDetail) => emitConfirm(detail);
    target.__ANALYTIX_PROMPT__ = (detail: PromptEventDetail) => emitPrompt(detail);
  };

  const nowMs = (): number =>
    typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();

  const getHostPerfSnapshot = (): Record<string, unknown> => ({
    runtimeHost: { ...runtimeHostMetrics }
  });

  const reportRuntimeHostIssue = (level: "INFO" | "WARN" | "ERROR", _message: string, data: Record<string, unknown> = {}): void => {
    const payload = {
      level,
      topic: "runtimeHost",
      hasData: Object.keys(data).length > 0,
      source: "flow-runtime-host"
    };
    try {
      runtimeStore?.getBackend?.()?.logFlowDebug?.(JSON.stringify(payload), () => {});
    } catch {
      // noop
    }
    const method = level === "ERROR" ? console.error : level === "WARN" ? console.warn : console.info;
    method(`[FlowRuntimeHost][${level}] runtimeHost`);
  };

  const wait = (ms: number): Promise<void> =>
    new Promise((resolve) => {
      window.setTimeout(resolve, Math.max(0, Math.round(ms)));
    });

  const html = `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width,initial-scale=1" />
    <base href="${page.workerBaseUrl}" />
    <style>${FLOW_RUNTIME_HOST_BASE_CSS}\n${FLOW_RUNTIME_REACT_SHELL_CSS}\n${page.stylesText}</style>
  </head>
  <body class="${FLOW_RUNTIME_BODY_CLASS}">
    ${page.bodyHtml}
  </body>
</html>`;

  const resetRuntimeBindings = (): void => {
    themeSyncCleanup?.();
    themeSyncCleanup = null;
    shellBridge = null;
    canvasBridge = null;
    runtimeStore = null;
    shellMounts = {
      left: null,
      gap: null,
      style: null,
      layout: null,
      analyze: null,
      ops: null,
      views: null,
      emptyView: null,
      graphSearch: null,
      graphStats: null,
      overlay: null,
    };
  };

  const hasCurrentAuthority = (): boolean =>
    !authorityInvalidated && isDataAnalysisRuntimeEndpointAuthorityCurrent(authority);

  const revokeAuthority = (): void => {
    if (authorityInvalidated) {
      return;
    }
    authorityInvalidated = true;
    if (iframe && host.contains(iframe)) {
      iframe.remove();
    }
    iframe = null;
    host.remove();
    resetRuntimeBindings();
    bridgeService?.dispose();
    bridgeService = null;
  };

  const requireCurrentAuthority = (): void => {
    if (!hasCurrentAuthority()) {
      revokeAuthority();
      throw new Error("data_analysis_backend_authority_invalidated");
    }
  };

  const loadRuntimeModule = async (
    liveIframeDocument: Document,
    liveIframeWindow: EmbeddedFlowWindow,
    scriptUrl: string
  ): Promise<void> => {
    requireCurrentAuthority();
    runtimeHostMetrics.lastAssetStage = "runtime-module";
    runtimeHostMetrics.lastStage = "boot:asset-load";
    await new Promise<void>((resolve, reject) => {
      const script = liveIframeDocument.createElement("script");
      let settled = false;
      let timer: number | null = null;
      const abortSignal = authority.lease?.signal;
      const handleAbort = (): void => {
        finish(new Error("data_analysis_backend_authority_invalidated"));
      };
      const finish = (error?: Error): void => {
        if (settled) {
          return;
        }
        settled = true;
        abortSignal?.removeEventListener("abort", handleAbort);
        if (timer !== null) {
          liveIframeWindow.clearTimeout(timer);
        }
        if (error) {
          script.remove();
          reject(error);
          return;
        }
        resolve();
      };
      timer = liveIframeWindow.setTimeout(() => {
        finish(new Error(`runtime asset load timed out: ${scriptUrl}`));
      }, 12_000);
      abortSignal?.addEventListener("abort", handleAbort, { once: true });
      if (abortSignal?.aborted) {
        handleAbort();
        return;
      }
      script.src = scriptUrl;
      script.type = "module";
      script.async = false;
      script.onload = () => {
        Promise.resolve(liveIframeWindow.__ANALYTIX_FLOW_RUNTIME_BOOT_PROMISE__)
          .then(() => finish())
          .catch((error) =>
            finish(error instanceof Error ? error : new Error(String(error || "embedded runtime module boot failed")))
          );
      };
      script.onerror = () => finish(new Error(`runtime asset load failed: ${scriptUrl}`));
      if (!hasCurrentAuthority()) {
        finish(new Error("data_analysis_backend_authority_invalidated"));
        return;
      }
      liveIframeDocument.body.appendChild(script);
    });
    requireCurrentAuthority();
  };

  const ensureBooted = async (container: HTMLElement): Promise<void> => {
    requireCurrentAuthority();
    if (shellBridge && iframe && host.parentElement === container) {
      return;
    }
    if (bootPromise) {
      await bootPromise;
      return;
    }
    bootPromise = (async () => {
      requireCurrentAuthority();
      runtimeHostMetrics.lastStage = "boot:start";
      if (host.parentElement !== container) {
        requireCurrentAuthority();
        container.replaceChildren(host);
      }

      requireCurrentAuthority();
      host.replaceChildren();
      iframe = document.createElement("iframe");
      iframe.className = "flow-runtime-frame";
      iframe.style.display = "block";
      iframe.style.width = "100%";
      iframe.style.height = "100%";
      iframe.style.minHeight = "100%";
      iframe.style.border = "0";
      iframe.setAttribute("aria-hidden", "false");
      requireCurrentAuthority();
      host.appendChild(iframe);
      runtimeHostMetrics.lastStage = "boot:iframe-ready";

      const iframeWindow = iframe.contentWindow as EmbeddedFlowWindow | null;
      const iframeDocument = iframe.contentDocument;
      if (!iframeWindow || !iframeDocument) {
        throw new Error("runtime iframe unavailable");
      }

      requireCurrentAuthority();
      iframeDocument.open();
      iframeDocument.write(html);
      iframeDocument.close();
      runtimeHostMetrics.lastStage = "boot:document-ready";

      const liveIframeWindow = iframe.contentWindow as EmbeddedFlowWindow | null;
      const liveIframeDocument = iframe.contentDocument;
      if (!liveIframeWindow || !liveIframeDocument) {
        throw new Error("runtime iframe unavailable after document write");
      }
      themeSyncCleanup?.();
      requireCurrentAuthority();
      themeSyncCleanup = bindFlowThemeSync(liveIframeDocument, document);

      requireCurrentAuthority();
      const service = createEmbeddedFlowBridgeService({
        apiBaseUrl: runtimeApiBase
      });
      bridgeService = service;
      requireCurrentAuthority();
      runtimeStore = createEmbeddedFlowRuntimeStore({
        service,
        targetWindow: liveIframeWindow,
      });
      installRuntimeGlobals(liveIframeWindow);
      liveIframeWindow.__ANALYTIX_FLOW_RUNTIME_BOOT__ = {
        scriptUrls: page.scriptUrls,
        onEvent: (event: Record<string, unknown>) => {
          const kind = String(event?.event || "").trim();
          const scriptUrl = String(event?.scriptUrl || "").trim();
          const error = String(event?.error || "").trim();
          if (scriptUrl) runtimeHostMetrics.lastAssetStage = "embedded-script";
          if (kind === "asset:start") {
            runtimeHostMetrics.lastStage = "boot:asset-load";
            return;
          }
          if (kind === "asset:loaded") {
            runtimeHostMetrics.assetLoadCount += 1;
            runtimeHostMetrics.lastAssetErrorCode = "";
            return;
          }
          if (kind === "asset:error") {
            runtimeHostMetrics.assetErrorCount += 1;
            runtimeHostMetrics.lastAssetErrorCode = "asset_load_failed";
            reportRuntimeHostIssue("ERROR", "embedded runtime asset load failed", {
              scriptUrl,
              error,
            });
            return;
          }
          if (kind === "boot:ready") {
            runtimeHostMetrics.lastStage = "boot:scripts-ready";
          }
        },
      };
      runtimeHostMetrics.lastStage = "boot:store-ready";

      requireCurrentAuthority();
      await loadRuntimeModule(liveIframeDocument, liveIframeWindow, page.runtimeLoaderUrl);
      requireCurrentAuthority();

      if (currentCaseId) {
        runtimeStore.getBrowserBridge().__setCaseId?.(currentCaseId);
      }

      const bodyElement = liveIframeDocument.body as HTMLElement;
      requireCurrentAuthority();
      shellMounts = createShellMounts(bodyElement);
      shellBridge = wrapEmbeddedFlowShellBridge(liveIframeWindow.__ANALYTIX_VIZ_SHELL__ ?? null, {
        getHostPerfSnapshot
      });
      if (!shellBridge) {
        throw new Error("runtime shell bridge missing after boot");
      }
      runtimeHostMetrics.shellReadyCount += 1;
      runtimeHostMetrics.lastStage = "boot:shell-ready";
      canvasBridge = runtimeStore.getBrowserBridge();

      // FlowPage can hand off stats -> flow requests before the runtime iframe
      // finishes booting, so replay any queued open/refit work once the shell is ready.
      const pendingOpenRequest = runtimeStore.takePendingOpenRequest();
      const pendingRefit = runtimeStore.consumePendingRefit();
      if (pendingOpenRequest) {
        if (typeof liveIframeWindow.__analytixVizFromStats === "function") {
          liveIframeWindow.__analytixVizFromStats(pendingOpenRequest);
        } else {
          canvasBridge?.__openRequest?.(pendingOpenRequest);
        }
      }
      if (pendingRefit) {
        if (typeof liveIframeWindow.__analytixVizRefit === "function") {
          liveIframeWindow.__analytixVizRefit();
        } else {
          canvasBridge?.__refit?.();
        }
      }
      requireCurrentAuthority();
    })().catch((error) => {
      runtimeHostMetrics.lastErrorCode = "embedded_runtime_boot_failed";
      runtimeHostMetrics.lastStage = "boot:failed";
      resetRuntimeBindings();
      if (iframe && host.contains(iframe)) {
        iframe.remove();
      }
      iframe = null;
      throw error;
    }).finally(() => {
      bootPromise = null;
    });
    await bootPromise;
  };

  const adapter: FlowCanvasAdapter = {
    async attach(container: HTMLElement): Promise<void> {
      requireCurrentAuthority();
      const startedAt = nowMs();
      runtimeHostMetrics.attachCount += 1;
      runtimeHostMetrics.lastAttemptAt = Date.now();
      runtimeHostMetrics.hasCaseContext = Boolean(currentCaseId);
      runtimeHostMetrics.lastStage = "attach:start";
      runtimeHostMetrics.lastErrorCode = "";
      try {
        await ensureBooted(container);
        runtimeHostMetrics.attachSuccessCount += 1;
        runtimeHostMetrics.lastReadyAt = Date.now();
        runtimeHostMetrics.lastDurationMs = Math.max(0, Math.round(nowMs() - startedAt));
        runtimeHostMetrics.lastStage = "attach:ready";
      } catch (error) {
        if (!hasCurrentAuthority()) {
          revokeAuthority();
          throw error;
        }
        const initialMessage = error instanceof Error ? error.message : "embedded runtime attach failed";
        runtimeHostMetrics.attachFailureCount += 1;
        runtimeHostMetrics.lastDurationMs = Math.max(0, Math.round(nowMs() - startedAt));
        runtimeHostMetrics.lastErrorCode = "embedded_runtime_attach_failed";
        runtimeHostMetrics.lastStage = "attach:failed";
        reportRuntimeHostIssue("WARN", "embedded runtime attach failed, retrying once", {
          error: initialMessage
        });
        runtimeHostMetrics.selfHealCount += 1;
        runtimeHostMetrics.lastSelfHealReason = "attach-retry";
        runtimeHostMetrics.lastSelfHealAt = Date.now();
        try {
          await wait(80);
          requireCurrentAuthority();
          await ensureBooted(container);
          runtimeHostMetrics.attachSuccessCount += 1;
          runtimeHostMetrics.selfHealSuccessCount += 1;
          runtimeHostMetrics.lastReadyAt = Date.now();
          runtimeHostMetrics.lastDurationMs = Math.max(0, Math.round(nowMs() - startedAt));
          runtimeHostMetrics.lastErrorCode = "";
          runtimeHostMetrics.lastStage = "attach:recovered";
          reportRuntimeHostIssue("INFO", "embedded runtime attach recovered after retry", {
            durationMs: runtimeHostMetrics.lastDurationMs
          });
          return;
        } catch (retryError) {
          const retryMessage = retryError instanceof Error ? retryError.message : "embedded runtime attach retry failed";
          runtimeHostMetrics.attachFailureCount += 1;
          runtimeHostMetrics.lastDurationMs = Math.max(0, Math.round(nowMs() - startedAt));
          runtimeHostMetrics.lastErrorCode = "embedded_runtime_attach_retry_failed";
          runtimeHostMetrics.lastStage = "attach:retry-failed";
          reportRuntimeHostIssue("ERROR", "embedded runtime attach retry failed", {
            error: retryMessage
          });
          throw retryError;
        }
      }
    },
    detach(container: HTMLElement): void {
      if (host.parentElement === container) {
        host.remove();
      }
      if (iframe && host.contains(iframe)) {
        iframe.remove();
      }
      iframe = null;
      resetRuntimeBindings();
    },
    getShellBridge(): EmbeddedFlowShellBridge | null {
      return shellBridge;
    },
    getShellMounts(): EmbeddedFlowShellMounts {
      return shellMounts;
    },
    openRequest(payload: EmbeddedFlowRequest): void {
      if (!hasCurrentAuthority()) {
        revokeAuthority();
        return;
      }
      runtimeHostMetrics.openRequestCount += 1;
      runtimeHostMetrics.lastOpenRequestAt = Date.now();
      runtimeHostMetrics.lastStage = "runtime:open-request";
      const iframeWindow = iframe?.contentWindow as EmbeddedFlowWindow | null;
      if (iframeWindow && typeof iframeWindow.__analytixVizFromStats === "function") {
        iframeWindow.__analytixVizFromStats(payload);
        return;
      }
      canvasBridge?.__openRequest?.(payload);
    },
    refit(): void {
      if (!hasCurrentAuthority()) {
        revokeAuthority();
        return;
      }
      runtimeHostMetrics.refitCount += 1;
      runtimeHostMetrics.lastRefitAt = Date.now();
      runtimeHostMetrics.lastStage = "runtime:refit";
      const iframeWindow = iframe?.contentWindow as EmbeddedFlowWindow | null;
      if (iframeWindow && typeof iframeWindow.__analytixVizRefit === "function") {
        iframeWindow.__analytixVizRefit();
        return;
      }
      canvasBridge?.__refit?.();
    },
    setCaseId(caseId: string): void {
      if (!hasCurrentAuthority()) {
        revokeAuthority();
        return;
      }
      currentCaseId = String(caseId || "").trim();
      runtimeHostMetrics.hasCaseContext = Boolean(currentCaseId);
      runtimeHostMetrics.lastStage = "runtime:set-case";
      canvasBridge?.__setCaseId?.(currentCaseId);
    }
  };

  if (authority.lease) {
    authority.lease.signal.addEventListener("abort", revokeAuthority, { once: true });
    if (authority.lease.signal.aborted) {
      revokeAuthority();
    }
  }
  return adapter;
}

export function ensureFlowCanvasAdapter(apiBaseUrl: string): Promise<FlowCanvasAdapter> {
  let authority;
  try {
    authority = resolveDataAnalysisRuntimeEndpointAuthority({
      requestedApiBaseUrl: trimTrailingSlash(String(apiBaseUrl || "").trim()),
    });
  } catch (error) {
    return Promise.reject(error);
  }
  const nextAuthorityKey = authority.authorityId;
  if (runtimePromise && runtimeAuthorityKey !== nextAuthorityKey) {
    runtimePromise = null;
  }
  if (!runtimePromise) {
    runtimeAuthorityKey = nextAuthorityKey;
    const request = createEmbeddedFlowCanvasAdapter(authority).catch((error) => {
      if (runtimePromise === request) {
        runtimePromise = null;
        runtimeAuthorityKey = "";
      }
      throw error;
    });
    runtimePromise = request;
    if (authority.lease) {
      const clearRevokedAdapter = (): void => {
        if (runtimePromise === request) {
          runtimePromise = null;
          runtimeAuthorityKey = "";
        }
      };
      authority.lease.signal.addEventListener("abort", clearRevokedAdapter, { once: true });
      if (authority.lease.signal.aborted) {
        clearRevokedAdapter();
      }
    }
  }
  return runtimePromise;
}

export function ensureEmbeddedFlowRuntime(apiBaseUrl: string): Promise<EmbeddedFlowRuntime> {
  return ensureFlowCanvasAdapter(apiBaseUrl);
}
