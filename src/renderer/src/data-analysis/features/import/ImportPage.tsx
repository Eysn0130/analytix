import { CSSProperties, Dispatch, DragEvent as ReactDragEvent, KeyboardEvent as ReactKeyboardEvent, MouseEvent, MutableRefObject, ReactNode, SetStateAction, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import {
  WorkbenchButton,
  WorkbenchMetricCard,
  WorkbenchMetricStrip,
  WorkbenchSegmentedControl,
  WorkbenchTableBody,
  WorkbenchTableShell,
  WorkbenchTableToolbar,
  WorkbenchToolbarButton,
  WorkbenchToolbarSearchField,
} from "../../components/workbench-ui";
import { ContextMenu, ContextMenuItem } from "../../components/overlay/ContextMenu";
import { Dialog } from "../../components/overlay/Dialog";
import { compactToastPath, showToast, toToastErrorDetail } from "../../components/feedback/toast-copy";
import { emitToast } from "../../components/feedback/ToastCenter";
import { formatCompactDurationMs, getDateRangeDurationMs } from "../../components/feedback/task-duration";
import {
  ensureDesktopBackendRuntime,
  hasDesktopBridge,
  pickFiles
} from "../../services/desktop/client";
import {
  controlledArtifactPublicationBlockReason,
  controlledSourceIngestionBlockReason,
} from "../../services/publication-quarantine";
import {
  asExportProgressPayload,
  asJobProgressPayload,
  eventBelongsToCase,
  isExportEvent,
  isImportEvent
} from "../../services/ws/domain-events";
import { useAppStore } from "../../store/app-store";
import { readPersistentString, writePersistentString } from "../../../lib/use-persistent-string-state";
import { cancelExportJob, createRawExportJob, getCaseDetail, getExportJob } from "../cases/api";
import { toErrorMessage } from "../shared/errors";
import {
  projectOrdinaryFieldValue,
  uncontrolledRestrictedPiiExportBlockReason,
} from "../shared/ordinary-pii-projection";
import {
  ImportFileLogDTO,
  ImportFileLogView,
  ImportFileSpec,
  ImportJobDTO,
  createImportJob,
  getImportJob,
  ImportHistoricalDatasetDTO,
  ImportPreviewFileDTO,
  listHistoricalImportDatasets,
  listImportFiles,
  listImportJobs,
  purgeImportFiles,
  previewImportFiles,
  recycleImportFiles,
  restoreImportFiles,
} from "./api";
import { buildWizardFile } from "./adapters/preview-to-wizard";
import {
  runningPercent,
  toLedgerRow,
  toLedgerRowFromJobFile,
} from "./adapters/ledger-row-adapter";
import {
  completeImpactEstimate,
  compareKnownMetrics,
  exactDatasetIndex,
  importCompletionTitle,
  maxKnownMetric,
  projectUnverifiedHistoricalCounts,
  sumCompleteMetrics,
} from "./model/public-count-projection";
import {
  EXPORT_TABLE_LABELS,
  KIND_CN,
  SORT_DIRECTION_OPTIONS,
  SORT_FIELD_OPTIONS,
  STATUS_FILTER_OPTIONS,
  TILE_ORDER,
  WIZARD_STEPS,
  assignExclusiveFieldMappingWithOrigin,
  categoryKindOptions,
  clampNumber,
  clearFieldMappingBySourceWithOrigin,
  defaultKindForCategory,
  fileNameFromPath,
  fileTypeAccent,
  formatExecutionOverviewFileName,
  isTerminalStatus,
  kindLabel,
  mapEventToStatus,
  mappingOriginForMethod,
  normalizeKind,
  resolveFileTypeLabel,
  resolveImportCategory,
  sanitizeFieldMappingOrigins,
  sanitizeManualFieldMapping,
  shouldAutoExpandMappingDetails,
  toInt,
  type ImportDomainCategory,
  type ImportKindFilter,
  type ImportStatusFilter,
  type ImportStudioPhase,
  type ImportWizardArchiveChild,
  type ImportWizardFile,
  type ImportWizardProgressChild,
  type ImportWizardProgressGroup,
  type MappingAssistState,
  type MappingInsight,
  type LedgerRow,
  type MappingTargetFieldPreview,
  type MappingWorkbenchColumn,
  type MappingWorkbenchFilter,
  type SortDirection,
  type SortField,
  type WizardFileState,
  type WizardProgressFilter,
  type WizardStep
} from "./model/import-page-model";
import {
  buildMappingInsight,
  buildMappingWorkbenchColumns,
  defaultMappingWorkbenchFilter,
  mappingBlueprintForKind,
  mappingInsightTone,
  mappingMatchedColumnCount,
  mappingPendingTemplateFields,
  mappingProgressLabel,
  mappingScopeResult,
  mappingWorkbenchFilterMatches,
  preferredActiveMappingFieldKey,
  serializeFieldMapping,
  serializeFieldMappingOrigins,
  serializeKindFieldMapping,
  serializeKindFieldMappingOrigins,
  suggestFieldMappingFromHeaders
} from "./model/field-mapping-model";
import { applySuggestedMappingEntries } from "./model/mapping-assist-model";
import { canContinueImportMappingStep } from "./state/import-mapping-step-state";
import { ImportDropzoneLottieIcon } from "./components/ImportDropzoneLottieIcon";
import "./styles/import-page.css";

type LedgerActionMode = "recycle" | "restore" | "purge";

interface LedgerActionDialogState {
  mode: LedgerActionMode;
  rows: LedgerRow[];
}

interface ExportDialogState {
  open: boolean;
  jobId: string;
  status: "running" | "done" | "failed" | "canceled";
  progress: number;
  stage: string;
  message: string;
  tableKey: string;
  tableLabel: string;
  tableTotal: number;
  tableDone: number;
  tableStatus: string;
  outputPath: string;
  error: string;
}

const IMPORT_PAGE_DESIGN_VIEWPORT_WIDTH = 1600;
const IMPORT_PAGE_DESIGN_CONTENT_WIDTH = 1260;
const IMPORT_PAGE_DESIGN_VIEWPORT_HEIGHT = 920;
const IMPORT_PAGE_MIN_SCALE = 0.72;
const IMPORT_PAGE_MIN_CONTENT_WIDTH = Math.round(IMPORT_PAGE_DESIGN_CONTENT_WIDTH * IMPORT_PAGE_MIN_SCALE);
const IMPORT_PAGE_OVERVIEW_DOCK_GAP = 8;
const IMPORT_PAGE_SELECT_SCROLL_TOLERANCE = 2;
const IMPORT_PAGE_SELECT_DOCK_ENTRY_GUARD_MS = 180;
const EXPORT_TRACK_POLL_OFFLINE_MS = 1200;
const EXPORT_TRACK_POLL_WS_FALLBACK_MS = 8000;
const IMPORT_PAGE_STATE_STORAGE_PREFIX = "analytix:import:page-state:v1";
const IMPORT_LEDGER_VIEW_VALUES = ["active", "recycle"] as const satisfies readonly ImportFileLogView[];
const IMPORT_SORT_FIELD_VALUES = ["created", "status", "kind", "name", "rows", "size"] as const satisfies readonly SortField[];
const IMPORT_SORT_DIRECTION_VALUES = ["desc", "asc"] as const satisfies readonly SortDirection[];
const IMPORT_STATUS_FILTER_VALUES = ["all", "active", "success", "attention"] as const satisfies readonly ImportStatusFilter[];
const IMPORT_KIND_FILTER_VALUES = [
  "all",
  "__tasks__",
  "fc_account",
  "fc_person",
  "fc_coercive_measure",
  "fc_transaction",
  "fc_sub_account",
  "fc_person_address",
  "fc_person_contact",
  "fc_task_success",
  "fc_task_fail"
] as const satisfies readonly ImportKindFilter[];
const IMPORT_CATEGORY_FILTER_VALUES = ["all", "structured", "entity", "support"] as const satisfies readonly ImportDomainCategory[];
const IMPORT_VALIDATION_FILTER_VALUES = ["all", "completed", "pending"] as const satisfies readonly WizardProgressFilter[];

function measureImportViewportScale(pageNode: HTMLElement, scrollHost: HTMLElement): number {
  const viewportWidth = Math.max(window.innerWidth || 0, scrollHost.clientWidth || 0);
  const viewportHeight = Math.max(window.innerHeight || 0, scrollHost.clientHeight || 0);
  const contentWidth = Math.max(pageNode.clientWidth || 0, scrollHost.clientWidth || 0);
  const widthScale = viewportWidth / IMPORT_PAGE_DESIGN_VIEWPORT_WIDTH;
  const contentScale = contentWidth / IMPORT_PAGE_DESIGN_CONTENT_WIDTH;
  const heightScale = viewportHeight / IMPORT_PAGE_DESIGN_VIEWPORT_HEIGHT;
  const nextScale = Math.max(IMPORT_PAGE_MIN_SCALE, Math.min(1, widthScale, contentScale, heightScale));
  return Number(nextScale.toFixed(4));
}

function resolveImportScrollHost(pageNode: HTMLElement | null): HTMLElement | null {
  if (!pageNode) {
    return null;
  }
  const pageHost = pageNode.closest(".data-analysis-surface__page");
  if (pageHost instanceof HTMLElement) {
    return pageHost;
  }
  const surfaceHost = pageNode.closest(".data-analysis-surface");
  if (surfaceHost instanceof HTMLElement) {
    return surfaceHost;
  }
  return pageNode;
}

function importCategoryLabel(category: Exclude<ImportDomainCategory, "all">): string {
  return CATEGORY_META.find((item) => item.key === category)?.label ?? "研判支撑文件";
}

function importCategoryEnglish(category: Exclude<ImportDomainCategory, "all">): string {
  return CATEGORY_META.find((item) => item.key === category)?.english ?? "Support Files";
}

function ImportWizardFileMark({ fileName, fileType }: { fileName: string; fileType: string }): JSX.Element {
  const tone = fileTypeAccent(fileName, fileType);
  const label = resolveFileTypeLabel(fileName, fileType);
  const isZip = tone === "is-zip";

  return (
    <div className={`import-console-upload-card__wizard-mark ${tone}`} aria-hidden="true">
      <span className="import-console-upload-card__wizard-mark-canvas">
        <span className={`import-console-upload-card__wizard-mark-symbol${isZip ? " is-zip" : ""}`}>
          <span className={`import-console-upload-card__wizard-mark-detail${isZip ? " is-zip" : " is-file"}`} />
          <span className="import-console-upload-card__wizard-mark-type">{label}</span>
        </span>
      </span>
    </div>
  );
}

function ImportWorkflowFileSummary(props: {
  fileName: string;
  fileType: string;
  statusTone: "success" | "error" | "warn" | "running" | "muted";
  statusLabel: string;
  metaItems: Array<string | null | undefined>;
  sha256?: string;
  onCopySha256?: () => void;
  actions?: ReactNode;
  message?: string;
  messageTone?: "success" | "error" | "warn" | "running" | "muted";
  contextContent?: ReactNode;
}): JSX.Element {
  const { fileName, fileType, statusTone, statusLabel, metaItems, sha256, onCopySha256, actions, message, messageTone = "muted", contextContent } = props;
  const filteredMetaItems = metaItems.map((item) => String(item || "").trim()).filter(Boolean);
  const hasInfoStrip = filteredMetaItems.length > 0 || Boolean(sha256);
  const hasContext = hasInfoStrip || Boolean(message) || Boolean(contextContent);

  return (
    <div className="import-console-upload-card__summary">
      <ImportWizardFileMark fileName={fileName} fileType={fileType} />
      <div className="import-console-upload-card__body">
        <div className="import-console-upload-card__head">
          <div className="import-console-upload-card__title-main">
            <strong className="import-console-upload-card__file-name">{fileName}</strong>
            <span className={`import-workbench-status-badge tone-${statusTone}`}>{statusLabel}</span>
          </div>
        </div>
        {hasContext ? (
          <div className="import-console-upload-card__context">
            {contextContent ? <div className="import-console-upload-card__context-body">{contextContent}</div> : null}
            {hasInfoStrip ? (
              <div className="import-console-upload-card__info-strip" aria-label="文件信息摘要">
                <div className="import-console-upload-card__info-items">
                  {filteredMetaItems.map((item, index) => (
                    <span key={`${fileName}:summary:${index}`} className="import-console-upload-card__info-item">
                      {item}
                    </span>
                  ))}
                </div>
                {sha256 ? (
                  <button
                    type="button"
                    className={`import-console-upload-card__info-hash tone-${statusTone}`}
                    title={sha256}
                    onClick={onCopySha256}
                  >
                    <span className="import-console-upload-card__info-hash-label">SHA-256值：</span>
                    <strong className="import-console-upload-card__info-hash-value">{shortHash(sha256)}</strong>
                  </button>
                ) : null}
              </div>
            ) : null}
            {message ? <p className={`import-console-upload-card__context-message is-${messageTone}`}>{message}</p> : null}
          </div>
        ) : null}
      </div>
      {actions ? <div className="import-console-upload-card__actions">{actions}</div> : null}
    </div>
  );
}

function ImportArchiveExpandToggleButton(props: {
  expanded: boolean;
  onClick: () => void;
}): JSX.Element {
  const { expanded, onClick } = props;
  return (
    <button
      type="button"
      className="import-console-upload-card__action-toggle"
      aria-expanded={expanded}
      onClick={onClick}
    >
      <span>{expanded ? "收起" : "展开"}</span>
    </button>
  );
}

function ImportWorkflowRemoveButton(props: {
  onClick: () => void;
}): JSX.Element {
  const { onClick } = props;
  return (
    <button type="button" className="import-console-upload-card__remove" onClick={onClick}>
      删除
    </button>
  );
}

function ImportWorkflowCardActions(props: {
  onRemove: () => void;
  toggle?: {
    variant: "archive" | "mapping";
    expanded: boolean;
    onClick: () => void;
    collapsedLabel?: string;
    expandedLabel?: string;
  };
}): JSX.Element {
  const { onRemove, toggle } = props;
  return (
    <>
      {toggle ? (
        toggle.variant === "archive" ? (
          <ImportArchiveExpandToggleButton expanded={toggle.expanded} onClick={toggle.onClick} />
        ) : (
          <button
            type="button"
            className={`import-console-upload-card__action-toggle${toggle.expanded ? " is-active" : ""}`}
            onClick={toggle.onClick}
          >
            {toggle.expanded ? toggle.expandedLabel || "收起" : toggle.collapsedLabel || "展开"}
          </button>
        )
      ) : null}
      <ImportWorkflowRemoveButton onClick={onRemove} />
    </>
  );
}

const ARCHIVE_CHILDREN_SHELL_ENTER_DURATION_MS = 560;
const ARCHIVE_CHILDREN_SHELL_EXIT_DURATION_MS = 360;
const ARCHIVE_CHILDREN_HEAD_ENTER_DURATION_MS = 320;
const ARCHIVE_CHILDREN_HEAD_EXIT_DURATION_MS = 220;
const ARCHIVE_CHILDREN_ITEM_ENTER_DURATION_MS = 440;
const ARCHIVE_CHILDREN_ITEM_EXIT_DURATION_MS = 260;
const ARCHIVE_CHILDREN_ITEM_BASE_DELAY_MS = 56;
const ARCHIVE_CHILDREN_ITEM_ENTER_STAGGER_MS = 44;
const ARCHIVE_CHILDREN_ITEM_EXIT_STAGGER_MS = 34;
const ARCHIVE_CHILDREN_ITEM_ENTER_STAGGER_LIMIT = 6;
const ARCHIVE_CHILDREN_ITEM_EXIT_STAGGER_LIMIT = 5;
const ARCHIVE_CHILD_DETAIL_ENTER_DURATION_MS = 320;
const ARCHIVE_CHILD_DETAIL_EXIT_DURATION_MS = 220;
const ARCHIVE_CHILD_ITEM_HIDE_DURATION_MS = 260;
const VALIDATION_GROUP_ENTER_DURATION_MS = 260;
const VALIDATION_GROUP_EXIT_DURATION_MS = 260;
const ARCHIVE_CHILD_SUMMARY_ANCHOR_Y_PX = (14 * 0.82) + ((42 * 0.9) / 2);
const ARCHIVE_CHILD_NODE_OUTER_SIZE_PX = 8 + (2 * 2);

type ArchiveChildrenMotionState = "collapsed" | "entering" | "entered" | "exiting";

function archiveChildrenEnterTotalMs(count: number): number {
  return Math.max(
    ARCHIVE_CHILDREN_SHELL_ENTER_DURATION_MS,
    ARCHIVE_CHILDREN_ITEM_BASE_DELAY_MS +
      ARCHIVE_CHILDREN_ITEM_ENTER_DURATION_MS +
      Math.min(Math.max(0, count - 1), ARCHIVE_CHILDREN_ITEM_ENTER_STAGGER_LIMIT) * ARCHIVE_CHILDREN_ITEM_ENTER_STAGGER_MS
  );
}

function archiveChildrenExitTotalMs(count: number): number {
  return Math.max(
    ARCHIVE_CHILDREN_SHELL_EXIT_DURATION_MS,
    ARCHIVE_CHILDREN_ITEM_EXIT_DURATION_MS +
      Math.min(Math.max(0, count - 1), ARCHIVE_CHILDREN_ITEM_EXIT_STAGGER_LIMIT) * ARCHIVE_CHILDREN_ITEM_EXIT_STAGGER_MS
  );
}

function usePrefersReducedMotion(): boolean {
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return false;
    }
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches || navigator.webdriver;
  });

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }

    const mediaQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    const updatePreference = (): void => {
      setPrefersReducedMotion(mediaQuery.matches || navigator.webdriver);
    };

    updatePreference();
    mediaQuery.addEventListener?.("change", updatePreference);

    return () => {
      mediaQuery.removeEventListener?.("change", updatePreference);
    };
  }, []);

  return prefersReducedMotion;
}

interface AnimatedExpandPresenceOptions {
  open: boolean;
  enterDurationMs: number;
  exitDurationMs: number;
  prefersReducedMotion: boolean;
}

function useAnimatedExpandPresence(options: AnimatedExpandPresenceOptions): {
  shouldRender: boolean;
  motionState: ArchiveChildrenMotionState;
  contentRef: MutableRefObject<HTMLDivElement | null>;
  shellStyle: CSSProperties;
} {
  const { open, enterDurationMs, exitDurationMs, prefersReducedMotion } = options;
  const [shouldRender, setShouldRender] = useState(open);
  const [motionState, setMotionState] = useState<ArchiveChildrenMotionState>(open ? "entered" : "collapsed");
  const [shellHeight, setShellHeight] = useState<number | "auto">(open ? "auto" : 0);
  const shouldRenderRef = useRef(shouldRender);
  const contentRef = useRef<HTMLDivElement | null>(null);
  const animationFrameIdsRef = useRef<number[]>([]);
  const exitTimerRef = useRef<number | null>(null);
  const hasInitializedMotionRef = useRef(false);

  const clearScheduledMotion = useCallback((): void => {
    animationFrameIdsRef.current.forEach((id) => window.cancelAnimationFrame(id));
    animationFrameIdsRef.current = [];
    if (exitTimerRef.current !== null) {
      window.clearTimeout(exitTimerRef.current);
      exitTimerRef.current = null;
    }
  }, []);

  const measureShellHeight = useCallback((): number => {
    const node = contentRef.current;
    if (!node) {
      return 0;
    }
    return Math.ceil(node.getBoundingClientRect().height);
  }, []);

  useEffect(() => {
    shouldRenderRef.current = shouldRender;
  }, [shouldRender]);

  useLayoutEffect(() => {
    clearScheduledMotion();

    if (!hasInitializedMotionRef.current) {
      hasInitializedMotionRef.current = true;
      shouldRenderRef.current = open;
      setShouldRender(open);
      setMotionState(open ? "entered" : "collapsed");
      setShellHeight(open ? "auto" : 0);
      return clearScheduledMotion;
    }

    if (prefersReducedMotion) {
      shouldRenderRef.current = open;
      setShouldRender(open);
      setMotionState(open ? "entered" : "collapsed");
      setShellHeight(open ? "auto" : 0);
      return clearScheduledMotion;
    }

    if (open) {
      shouldRenderRef.current = true;
      setShouldRender(true);
      setMotionState("entering");
      setShellHeight(0);

      const firstFrame = window.requestAnimationFrame(() => {
        const secondFrame = window.requestAnimationFrame(() => {
          setShellHeight(measureShellHeight());
          exitTimerRef.current = window.setTimeout(() => {
            setMotionState("entered");
            setShellHeight("auto");
            exitTimerRef.current = null;
          }, enterDurationMs);
        });
        animationFrameIdsRef.current.push(secondFrame);
      });

      animationFrameIdsRef.current.push(firstFrame);
      return clearScheduledMotion;
    }

    if (!shouldRenderRef.current) {
      setMotionState("collapsed");
      setShellHeight(0);
      return clearScheduledMotion;
    }

    setMotionState("exiting");
    setShellHeight(measureShellHeight());

    const collapseFrame = window.requestAnimationFrame(() => {
      setShellHeight(0);
    });
    animationFrameIdsRef.current.push(collapseFrame);

    exitTimerRef.current = window.setTimeout(() => {
      shouldRenderRef.current = false;
      setShouldRender(false);
      setMotionState("collapsed");
      setShellHeight(0);
      exitTimerRef.current = null;
    }, exitDurationMs);

    return clearScheduledMotion;
  }, [clearScheduledMotion, enterDurationMs, exitDurationMs, measureShellHeight, open, prefersReducedMotion]);

  return {
    shouldRender,
    motionState,
    contentRef,
    shellStyle: {
      height: prefersReducedMotion ? undefined : shellHeight === "auto" ? "auto" : `${shellHeight}px`,
    },
  };
}

function archiveExpandableItemMotionStyle(index: number, count: number): CSSProperties {
  return {
    "--import-archive-item-order": String(Math.min(index, ARCHIVE_CHILDREN_ITEM_ENTER_STAGGER_LIMIT)),
    "--import-archive-item-reverse-order": String(Math.min(count - index - 1, ARCHIVE_CHILDREN_ITEM_EXIT_STAGGER_LIMIT)),
    "--import-archive-item-anchor-y": `${ARCHIVE_CHILD_SUMMARY_ANCHOR_Y_PX}px`,
    "--import-archive-item-tail-top": `${ARCHIVE_CHILD_SUMMARY_ANCHOR_Y_PX + (ARCHIVE_CHILD_NODE_OUTER_SIZE_PX / 2)}px`,
  } as CSSProperties;
}

interface ArchiveExpandableItem {
  id: string;
  cardTitle?: string;
  summary: ReactNode;
  hidden?: boolean;
  active?: boolean;
  progressId?: string;
  detailOpen?: boolean;
  renderDetail?: () => ReactNode;
}

function ImportArchiveExpandableListItem(props: {
  item: ArchiveExpandableItem;
  index: number;
  totalCount: number;
  prefersReducedMotion: boolean;
}): JSX.Element {
  const { item, index, totalCount, prefersReducedMotion } = props;
  const visibilityMotion = useAnimatedExpandPresence({
    open: !item.hidden,
    enterDurationMs: ARCHIVE_CHILDREN_ITEM_ENTER_DURATION_MS,
    exitDurationMs: ARCHIVE_CHILD_ITEM_HIDE_DURATION_MS,
    prefersReducedMotion,
  });
  const detailMotion = useAnimatedExpandPresence({
    open: !item.hidden && Boolean(item.detailOpen && item.renderDetail),
    enterDurationMs: ARCHIVE_CHILD_DETAIL_ENTER_DURATION_MS,
    exitDurationMs: ARCHIVE_CHILD_DETAIL_EXIT_DURATION_MS,
    prefersReducedMotion,
  });

  if (!visibilityMotion.shouldRender) {
    return <></>;
  }

  const detailStyle = {
    "--import-archive-detail-enter-duration": `${ARCHIVE_CHILD_DETAIL_ENTER_DURATION_MS}ms`,
    "--import-archive-detail-exit-duration": `${ARCHIVE_CHILD_DETAIL_EXIT_DURATION_MS}ms`,
    ...detailMotion.shellStyle,
  } as CSSProperties;

  return (
    <div
      className={`import-console-upload-card__expand-item${detailMotion.shouldRender ? " has-detail" : ""}${item.detailOpen ? " is-detail-open" : ""}`}
      role="listitem"
      data-visibility-state={visibilityMotion.motionState}
      data-progress-active={item.active ? "true" : "false"}
      data-progress-id={item.progressId || item.id}
      style={{
        ...archiveExpandableItemMotionStyle(index, totalCount),
        ...visibilityMotion.shellStyle,
      }}
    >
      <div className="import-console-upload-card__expand-item-card" title={item.cardTitle || undefined}>
        {item.summary}
      </div>
      {item.renderDetail && detailMotion.shouldRender ? (
        <div className="import-console-upload-card__expand-item-detail-shell" data-motion-state={detailMotion.motionState} style={detailStyle}>
          <div className="import-console-upload-card__expand-item-detail" ref={detailMotion.contentRef}>
            {item.renderDetail()}
          </div>
        </div>
      ) : null}
      <span className="import-console-upload-card__expand-item-tail-mask" aria-hidden="true" />
    </div>
  );
}

function ImportArchiveExpandableBranch(props: {
  isExpanded: boolean;
  parentCardClassName: string;
  parentSummary: ReactNode;
  headerTitle: string;
  headerCountLabel: string;
  headerStats: ReactNode;
  listAriaLabel: string;
  items: ArchiveExpandableItem[];
  isActive?: boolean;
  progressId?: string;
}): JSX.Element {
  const { isExpanded, parentCardClassName, parentSummary, headerTitle, headerCountLabel, headerStats, listAriaLabel, items, isActive = false, progressId } = props;
  const prefersReducedMotion = usePrefersReducedMotion();
  const childCount = items.length;
  const shellMotion = useAnimatedExpandPresence({
    open: isExpanded,
    enterDurationMs: archiveChildrenEnterTotalMs(childCount),
    exitDurationMs: archiveChildrenExitTotalMs(childCount),
    prefersReducedMotion,
  });

  const shellStyle = {
    "--import-archive-shell-enter-duration": `${ARCHIVE_CHILDREN_SHELL_ENTER_DURATION_MS}ms`,
    "--import-archive-shell-exit-duration": `${ARCHIVE_CHILDREN_SHELL_EXIT_DURATION_MS}ms`,
    "--import-archive-head-enter-duration": `${ARCHIVE_CHILDREN_HEAD_ENTER_DURATION_MS}ms`,
    "--import-archive-head-exit-duration": `${ARCHIVE_CHILDREN_HEAD_EXIT_DURATION_MS}ms`,
    "--import-archive-item-enter-duration": `${ARCHIVE_CHILDREN_ITEM_ENTER_DURATION_MS}ms`,
    "--import-archive-item-exit-duration": `${ARCHIVE_CHILDREN_ITEM_EXIT_DURATION_MS}ms`,
    "--import-archive-item-base-delay": `${ARCHIVE_CHILDREN_ITEM_BASE_DELAY_MS}ms`,
    "--import-archive-item-enter-stagger": `${ARCHIVE_CHILDREN_ITEM_ENTER_STAGGER_MS}ms`,
    "--import-archive-item-exit-stagger": `${ARCHIVE_CHILDREN_ITEM_EXIT_STAGGER_MS}ms`,
    ...shellMotion.shellStyle,
  } as CSSProperties;

  return (
    <article
      className="import-console-upload-card-branch is-archive-expanded"
      data-progress-group-active={isActive ? "true" : "false"}
      data-progress-id={progressId || undefined}
    >
      <div className={parentCardClassName}>{parentSummary}</div>
      {shellMotion.shouldRender ? (
        <div className="import-console-upload-card__children-shell" data-motion-state={shellMotion.motionState} style={shellStyle}>
          <div className="import-console-upload-card__expand-group" ref={shellMotion.contentRef}>
            <div className="import-console-upload-card__expand-head">
              <div className="import-console-upload-card__expand-head-copy">
                <strong>{headerTitle}</strong>
                <span>{headerCountLabel}</span>
              </div>
              <div className="import-console-upload-card__expand-head-stats" aria-label={`${headerTitle}状态汇总`}>
                {headerStats}
              </div>
            </div>
            <div className="import-console-upload-card__expand-list" role="list" aria-label={listAriaLabel}>
              {items.map((item, index) => (
                <ImportArchiveExpandableListItem
                  key={item.id}
                  item={item}
                  index={index}
                  totalCount={childCount}
                  prefersReducedMotion={prefersReducedMotion}
                />
              ))}
            </div>
          </div>
        </div>
      ) : null}
    </article>
  );
}

function ImportArchiveBranchCard(props: {
  file: ImportWizardFile;
  isExpanded: boolean;
  uploadCardClassName: string;
  uploadCardSummary: ReactNode;
  readyArchiveCount: number;
  runningArchiveCount: number;
  pendingArchiveCount: number;
  failedArchiveCount: number;
  onCopyText: (value: string, label: string) => void | Promise<void>;
}): JSX.Element {
  const {
    file,
    isExpanded,
    uploadCardClassName,
    uploadCardSummary,
    readyArchiveCount,
    runningArchiveCount,
    pendingArchiveCount,
    failedArchiveCount,
    onCopyText,
  } = props;

  const items: ArchiveExpandableItem[] = file.archiveChildren.map((child) => {
    const childHint = wizardArchiveChildHint(child);
    const childStatusTone = fileRowStatusTone(child.status);
    const childKindLabel = wizardArchiveChildKindLabel(child);
    return {
      id: child.id,
      cardTitle: childHint,
      summary: (
        <ImportWorkflowFileSummary
          fileName={child.fileName}
          fileType={child.fileType}
          statusTone={childStatusTone}
          statusLabel={wizardFileStateLabel(child)}
          metaItems={[childKindLabel, child.rowsTotal > 0 ? `${child.rowsTotal.toLocaleString("zh-CN")} 行` : "—"]}
          sha256={child.sha256}
          onCopySha256={child.sha256 ? () => void onCopyText(child.sha256, "SHA-256") : undefined}
        />
      ),
    };
  });

  return (
    <ImportArchiveExpandableBranch
      isExpanded={isExpanded}
      parentCardClassName={uploadCardClassName}
      parentSummary={uploadCardSummary}
      headerTitle="子项清单"
      headerCountLabel={`${file.archiveChildren.length.toLocaleString("zh-CN")} 个子项`}
      headerStats={
        <>
          <span className="tone-success">已检验 {readyArchiveCount.toLocaleString("zh-CN")}</span>
          {runningArchiveCount > 0 ? <span className="tone-running">处理中 {runningArchiveCount.toLocaleString("zh-CN")}</span> : null}
          <span className={pendingArchiveCount > 0 ? "tone-warn" : "tone-muted"}>
            待确认 {pendingArchiveCount.toLocaleString("zh-CN")}
          </span>
          <span className={failedArchiveCount > 0 ? "tone-error" : "tone-muted"}>
            异常 {failedArchiveCount.toLocaleString("zh-CN")}
          </span>
        </>
      }
      listAriaLabel="压缩包子项列表"
      items={items}
    />
  );
}

function ImportWorkflowStageHeader(props: {
  title: string;
  stats?: ReactNode;
  actions?: ReactNode;
  className?: string;
}): JSX.Element {
  const { title, stats, actions, className } = props;

  return (
    <div className={`import-console-dropzone__queue-toolbar import-console-stage-toolbar${className ? ` ${className}` : ""}`}>
      <div className="import-console-stage-toolbar__main">
        <div className="import-console-dropzone__queue-title import-console-stage-toolbar__title">
          <strong>{title}</strong>
        </div>
        {stats ? <div className="import-console-stage-toolbar__stats">{stats}</div> : null}
      </div>
      {actions ? <div className="import-console-dropzone__queue-toolbar-actions import-console-stage-toolbar__actions">{actions}</div> : null}
    </div>
  );
}

function wizardFileKindLabel(file: ImportWizardFile): string {
  if (!file.selectedKind) {
    return "待映射";
  }
  return KIND_CN[file.selectedKind] ?? file.selectedKind;
}

function wizardFileSubtitleItems(file: ImportWizardFile): string[] {
  if (file.archiveChildren.length > 0) {
    return ["压缩包", formatBytes(file.size), `${file.archiveChildren.length.toLocaleString("zh-CN")} 子项`];
  }

  const items = [importCategoryLabel(file.selectedCategory), formatBytes(file.size), wizardFileKindLabel(file)];
  if (file.rowsTotal > 0) {
    items.push(`${file.rowsTotal.toLocaleString("zh-CN")} 行`);
  } else if (file.columnsTotal > 0) {
    items.push(`${file.columnsTotal.toLocaleString("zh-CN")} 列`);
  } else if (file.selectedCategory === "support") {
    items.push("文件登记");
  } else {
    items.push("待映射");
  }
  return items;
}

function wizardFileStateLabel(file: { status: WizardFileState } | WizardFileState): string {
  const status = typeof file === "string" ? file : file.status;
  if (status === "unsupported") {
    return "不支持";
  }
  if (status === "failed") {
    return "异常";
  }
  if (status === "running") {
    return "处理中";
  }
  if (status === "succeeded") {
    return "已完成";
  }
  if (status === "review") {
    return "需确认";
  }
  return "已检验";
}

function fileRowStatusTone(status: WizardFileState): "success" | "error" | "warn" | "running" | "muted" {
  if (status === "succeeded" || status === "ready") {
    return "success";
  }
  if (status === "failed" || status === "unsupported") {
    return "error";
  }
  if (status === "running") {
    return "running";
  }
  if (status === "review") {
    return "warn";
  }
  return "muted";
}

function clampWizardProgress(value: number): number {
  return Math.max(0, Math.min(100, toInt(value)));
}

function validationExecutionTone(status: WizardFileState, started: boolean): "success" | "error" | "warn" | "running" | "muted" {
  if (!started) {
    return "muted";
  }
  if (status === "succeeded") {
    return "success";
  }
  if (status === "failed" || status === "unsupported") {
    return "error";
  }
  if (status === "running") {
    return "running";
  }
  return "muted";
}

function validationExecutionLabel(status: WizardFileState, started: boolean): string {
  if (!started) {
    return "待执行";
  }
  if (status === "succeeded") {
    return "已导入";
  }
  if (status === "failed" || status === "unsupported") {
    return "异常";
  }
  if (status === "running") {
    return "执行中";
  }
  return "待执行";
}

function validationExecutionProgress(status: WizardFileState, progress: number, started: boolean): number {
  if (!started) {
    return 0;
  }
  if (status === "succeeded") {
    return 100;
  }
  return clampWizardProgress(progress);
}

function validationArchiveChildMetaItems(child: ImportWizardProgressChild): string[] {
  return [
    child.kindLabel || child.suggestedKindLabel || wizardArchiveChildKindLabel(child),
    child.rowsTotal > 0 ? `${child.rowsTotal.toLocaleString("zh-CN")} 行` : child.columnsTotal > 0 ? `${child.columnsTotal.toLocaleString("zh-CN")} 列` : "—",
  ];
}

function wizardProgressGroupComplete(file: ImportWizardProgressGroup): boolean {
  return file.status === "succeeded" && file.childProgress.every((child) => child.status === "succeeded");
}

function validationProgressMessageForGroup(file: ImportWizardProgressGroup, started: boolean): string {
  if (!started) {
    return "待执行";
  }

  if (file.status === "failed" || file.status === "unsupported") {
    return "执行异常";
  }

  if (file.childProgress.length > 0) {
    const runningChild = file.childProgress.find((child) => child.isActive || child.status === "running") ?? null;

    if (file.status === "succeeded") {
      return "全部完成";
    }
    if (runningChild) {
      return `执行中 · ${runningChild.fileName}`;
    }
    return "排队中";
  }

  if (file.status === "succeeded") {
    return "已完成";
  }
  if (file.status === "running") {
    return "执行中";
  }
  return "排队中";
}

function validationProgressMessageForChild(child: ImportWizardProgressChild, started: boolean): string {
  if (!started) {
    return "待执行";
  }
  if (child.status === "failed" || child.status === "unsupported") {
    return "执行异常";
  }
  if (child.status === "succeeded") {
    return "已完成";
  }
  if (child.status === "running") {
    return "执行中";
  }
  return "排队中";
}

function previewDetectedByLabel(raw: string): string {
  const token = String(raw || "").trim().toLowerCase();
  if (!token) {
    return "";
  }
  if (token === "archive-entry" || token === "archive") {
    return "压缩包子项";
  }
  if (token === "headers") {
    return "按表头识别";
  }
  if (token === "extension") {
    return "按文件类型识别";
  }
  if (token === "manual") {
    return "按手动指定识别";
  }
  return "";
}

function wizardArchiveChildHint(child: ImportWizardArchiveChild): string {
  return child.issue || previewDetectedByLabel(child.detectedBy) || "等待进入下一步确认";
}

function wizardArchiveChildKindLabel(child: ImportWizardArchiveChild): string {
  if (!child.selectedKind) {
    return child.suggestedKindLabel || "待映射";
  }
  return kindLabel(child.selectedKind);
}

function formatBytes(size: number): string {
  if (!Number.isFinite(size) || size < 0) {
    return "—";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = size;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  if (index === 0) {
    return `${Math.trunc(value)} ${units[index]}`;
  }
  return `${value.toFixed(2)} ${units[index]}`;
}

function shortHash(value: string): string {
  const raw = String(value || "").trim();
  if (!raw) {
    return "—";
  }
  if (raw.length <= 14) {
    return raw;
  }
  return `${raw.slice(0, 6)}…${raw.slice(-6)}`;
}

function formatDateTime(value: string): string {
  const ts = Date.parse(String(value || ""));
  if (!Number.isFinite(ts)) {
    return "—";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  })
    .format(new Date(ts))
    .replace(/\//g, "-");
}

function trimDetailText(value: string, maxLength = 2200): string {
  const text = String(value || "").trim();
  if (!text) {
    return "";
  }
  if (text.length <= maxLength) {
    return text;
  }
  return `${text.slice(0, maxLength)}\n...（已截断）`;
}

function toDomIdFragment(value: string): string {
  const normalized = String(value || "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return normalized || "item";
}

function handleTabListKeyDown(event: ReactKeyboardEvent<HTMLButtonElement>): void {
  const { key, currentTarget } = event;
  if (!["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End"].includes(key)) {
    return;
  }
  const tabList = currentTarget.closest('[role="tablist"]');
  if (!(tabList instanceof HTMLElement)) {
    return;
  }
  const tabs = Array.from(tabList.querySelectorAll<HTMLButtonElement>('[role="tab"]:not(:disabled)'));
  const currentIndex = tabs.indexOf(currentTarget);
  if (currentIndex === -1 || tabs.length === 0) {
    return;
  }
  event.preventDefault();
  let nextIndex = currentIndex;
  if (key === "Home") {
    nextIndex = 0;
  } else if (key === "End") {
    nextIndex = tabs.length - 1;
  } else {
    const direction = key === "ArrowRight" || key === "ArrowDown" ? 1 : -1;
    nextIndex = (currentIndex + direction + tabs.length) % tabs.length;
  }
  const nextTab = tabs[nextIndex];
  nextTab.focus();
  nextTab.click();
}

function isActiveStatusText(statusText: string): boolean {
  return statusText.startsWith("导入中") || statusText === "等待中";
}

function formatKnownCount(value: number | null): string {
  return value === null ? "—" : value.toLocaleString("zh-CN");
}

function formatKnownBytes(value: number | null): string {
  return value === null ? "—" : formatBytes(value);
}

function rowHasAttention(row: LedgerRow): boolean {
  return (
    row.status_text === "失败" ||
    row.status_text === "重复数据" ||
    row.status_text === "已取消" ||
    row.status_text.includes("未验证") ||
    Boolean(String(row.error || row.note || "").trim()) ||
    (row.rows_error !== null && row.rows_error > 0)
  );
}

function rowMatchesStatusFilter(row: LedgerRow, filter: ImportStatusFilter): boolean {
  if (filter === "all") {
    return true;
  }
  if (filter === "active") {
    return isActiveStatusText(row.status_text);
  }
  if (filter === "success") {
    return row.status_text.startsWith("已完成") && !rowHasAttention(row);
  }
  return rowHasAttention(row);
}

function duplicateFlagForRow(args: {
  statusText: string;
  duplicateRows: number | null;
  validRows: number | null;
  source: "ledger" | "historical" | "merged";
}): string {
  if (args.duplicateRows === null || args.validRows === null) {
    return "—";
  }
  if (args.source === "historical" && args.duplicateRows <= 0 && args.validRows <= 0) {
    return "—";
  }
  if (args.statusText === "重复数据") {
    return "重复";
  }
  if (args.duplicateRows > 0 && args.validRows > 0) {
    return "部分重复";
  }
  if (args.duplicateRows > 0) {
    return "重复";
  }
  if (args.statusText === "失败" || args.statusText === "等待中" || args.statusText.startsWith("导入中")) {
    return "—";
  }
  return "否";
}

function duplicateSummaryForRow(args: {
  statusText: string;
  duplicateRows: number | null;
  validRows: number | null;
  source: "ledger" | "historical" | "merged";
}): string {
  if (args.duplicateRows === null || args.validRows === null) {
    return "导入指标未验证";
  }
  if (args.source === "historical" && args.duplicateRows <= 0 && args.validRows <= 0) {
    return "历史数据集记录";
  }
  if (args.statusText === "重复数据") {
    return "文件内容已存在，未新增有效数据";
  }
  if (args.duplicateRows > 0 && args.validRows > 0) {
    return `重复 ${args.duplicateRows.toLocaleString("zh-CN")} 行，新增 ${args.validRows.toLocaleString("zh-CN")} 行`;
  }
  if (args.duplicateRows > 0) {
    return `识别重复 ${args.duplicateRows.toLocaleString("zh-CN")} 行`;
  }
  if (args.validRows > 0) {
    return `新增 ${args.validRows.toLocaleString("zh-CN")} 行`;
  }
  return "待导入结果";
}

interface ImportMetricCardProps {
  label: string;
  value: string;
  hint: string;
  tone?: "neutral" | "accent" | "success" | "warn";
}

interface TypeSummaryRow {
  key: ImportKindFilter;
  title: string;
  taskCount: number;
  importedRows: number | null;
  totalRows: number | null;
  progress: number;
  busy: boolean;
  attentionCount: number;
}

interface HistoricalImportDetailRow {
  key: string;
  fileName: string;
  kindLabel: string;
  rowCount: number | null;
  columnCountLabel: string;
  importedAt: string;
  datasetId: string;
}

interface ImportConsoleRow {
  key: string;
  fileName: string;
  category: Exclude<ImportDomainCategory, "all">;
  categoryLabel: string;
  categoryEnglish: string;
  kindLabel: string;
  statusText: string;
  rowsTotal: number | null;
  validRows: number | null;
  duplicateRows: number | null;
  columnCount: number | null;
  datasetId: string;
  sha256: string;
  path: string;
  importedAt: string;
  detailText: string;
  duplicateFlag: string;
  duplicateSummary: string;
  source: "ledger" | "historical" | "merged";
  ledgerRow: LedgerRow | null;
}

interface ImportCategoryCardModel {
  key: Exclude<ImportDomainCategory, "all">;
  label: string;
  english: string;
  hint: string;
  fileCount: number;
  rowCount: number | null;
}

interface ExecutionTaskTableRow {
  key: string;
  title: string;
  subtitle: string;
  dataType: string;
  submitter: string;
  dateText: string;
  sizeText: string;
  statusText: string;
  tone: "success" | "error" | "warn" | "running" | "muted";
  progressText: string;
  progressValue: number | null;
  detailText: string;
  rowPaths: string[];
  record: ImportConsoleRow | null;
  isLiveTask: boolean;
}

function executionSubmitterLabel(row: ImportConsoleRow): string {
  return row.source === "historical" ? "历史导入" : "当前用户";
}

function executionProgressMeta(row: ImportConsoleRow): { text: string; value: number | null } {
  if (row.statusText.startsWith("已完成") || row.statusText === "已入库" || row.statusText === "重复数据") {
    return { text: "100%", value: 100 };
  }
  if (row.statusText === "失败") {
    return { text: "失败", value: 0 };
  }
  if (isActiveStatusText(row.statusText)) {
    if (row.ledgerRow && row.ledgerRow.rows_total !== null && row.ledgerRow.rows_total > 0) {
      const progress = runningPercent(row.ledgerRow.rows_seen, row.ledgerRow.rows_total);
      return { text: `${progress.toLocaleString("zh-CN")}%`, value: progress };
    }
    return { text: row.statusText, value: 12 };
  }
  return { text: "—", value: null };
}

function isExecutionPendingOrRunning(statusText: string): boolean {
  return isActiveStatusText(statusText) || statusText === "处理中" || statusText === "等待中";
}

function canPreviewExecutionRow(row: ExecutionTaskTableRow): boolean {
  return !row.statusText.includes("失败") && !isExecutionPendingOrRunning(row.statusText);
}

function canDownloadExecutionRow(row: ExecutionTaskTableRow): boolean {
  return row.rowPaths.length === 1 && !row.statusText.includes("失败") && !isExecutionPendingOrRunning(row.statusText);
}

function canRetryExecutionRow(row: ExecutionTaskTableRow): boolean {
  return row.rowPaths.length > 0;
}

function canDeleteExecutionRow(row: ExecutionTaskTableRow): boolean {
  return Boolean(row.record?.ledgerRow) && !isExecutionPendingOrRunning(row.statusText);
}

function ImportTaskActionIcon({
  kind,
}: {
  kind: "view" | "download" | "retry" | "delete";
}): JSX.Element {
  if (kind === "view") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" className="is-view">
        <path
          d="M14 2.75C15.9068 2.75 17.2615 2.75159 18.2892 2.88976C19.2952 3.02503 19.8749 3.27869 20.2981 3.7019C20.7852 4.18904 20.9973 4.56666 21.1147 5.23984C21.2471 5.9986 21.25 7.08092 21.25 9C21.25 9.41422 21.5858 9.75 22 9.75C22.4142 9.75 22.75 9.41422 22.75 9L22.75 8.90369C22.7501 7.1045 22.7501 5.88571 22.5924 4.98199C22.417 3.97665 22.0432 3.32568 21.3588 2.64124C20.6104 1.89288 19.6615 1.56076 18.489 1.40314C17.3498 1.24997 15.8942 1.24998 14.0564 1.25H14C13.5858 1.25 13.25 1.58579 13.25 2C13.25 2.41421 13.5858 2.75 14 2.75Z"
          fill="currentColor"
          opacity="0.5"
        />
        <path
          d="M2.00001 14.25C2.41422 14.25 2.75001 14.5858 2.75001 15C2.75001 16.9191 2.75289 18.0014 2.88529 18.7602C3.00275 19.4333 3.21477 19.811 3.70191 20.2981C4.12512 20.7213 4.70476 20.975 5.71085 21.1102C6.73852 21.2484 8.09318 21.25 10 21.25C10.4142 21.25 10.75 21.5858 10.75 22C10.75 22.4142 10.4142 22.75 10 22.75H9.94359C8.10583 22.75 6.6502 22.75 5.51098 22.5969C4.33856 22.4392 3.38961 22.1071 2.64125 21.3588C1.95681 20.6743 1.58304 20.0233 1.40762 19.018C1.24992 18.1143 1.24995 16.8955 1.25 15.0964L1.25001 15C1.25001 14.5858 1.58579 14.25 2.00001 14.25Z"
          fill="currentColor"
          opacity="0.5"
        />
        <path
          d="M22 14.25C22.4142 14.25 22.75 14.5858 22.75 15L22.75 15.0963C22.7501 16.8955 22.7501 18.1143 22.5924 19.018C22.417 20.0233 22.0432 20.6743 21.3588 21.3588C20.6104 22.1071 19.6615 22.4392 18.489 22.5969C17.3498 22.75 15.8942 22.75 14.0564 22.75H14C13.5858 22.75 13.25 22.4142 13.25 22C13.25 21.5858 13.5858 21.25 14 21.25C15.9068 21.25 17.2615 21.2484 18.2892 21.1102C19.2952 20.975 19.8749 20.7213 20.2981 20.2981C20.7852 19.811 20.9973 19.4333 21.1147 18.7602C21.2471 18.0014 21.25 16.9191 21.25 15C21.25 14.5858 21.5858 14.25 22 14.25Z"
          fill="currentColor"
          opacity="0.5"
        />
        <path
          d="M9.94359 1.25H10C10.4142 1.25 10.75 1.58579 10.75 2C10.75 2.41421 10.4142 2.75 10 2.75C8.09319 2.75 6.73852 2.75159 5.71085 2.88976C4.70476 3.02503 4.12512 3.27869 3.70191 3.7019C3.21477 4.18904 3.00275 4.56666 2.88529 5.23984C2.75289 5.9986 2.75001 7.08092 2.75001 9C2.75001 9.41422 2.41422 9.75 2.00001 9.75C1.58579 9.75 1.25001 9.41422 1.25001 9L1.25 8.90369C1.24995 7.10453 1.24992 5.8857 1.40762 4.98199C1.58304 3.97665 1.95681 3.32568 2.64125 2.64124C3.38961 1.89288 4.33856 1.56076 5.51098 1.40314C6.65019 1.24997 8.10584 1.24998 9.94359 1.25Z"
          fill="currentColor"
          opacity="0.5"
        />
        <path d="M12 10.75C11.3096 10.75 10.75 11.3096 10.75 12C10.75 12.6904 11.3096 13.25 12 13.25C12.6904 13.25 13.25 12.6904 13.25 12C13.25 11.3096 12.6904 10.75 12 10.75Z" fill="currentColor" />
        <path
          fillRule="evenodd"
          clipRule="evenodd"
          d="M5.89243 14.0598C5.29747 13.3697 5 13.0246 5 12C5 10.9754 5.29748 10.6303 5.89242 9.94021C7.08037 8.56222 9.07268 7 12 7C14.9273 7 16.9196 8.56222 18.1076 9.94021C18.7025 10.6303 19 10.9754 19 12C19 13.0246 18.7025 13.3697 18.1076 14.0598C16.9196 15.4378 14.9273 17 12 17C9.07268 17 7.08038 15.4378 5.89243 14.0598ZM9.25 12C9.25 10.4812 10.4812 9.25 12 9.25C13.5188 9.25 14.75 10.4812 14.75 12C14.75 13.5188 13.5188 14.75 12 14.75C10.4812 14.75 9.25 13.5188 9.25 12Z"
          fill="currentColor"
        />
      </svg>
    );
  }
  if (kind === "download") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" className="is-download">
        <path d="M12 7L12 14M12 14L15 11M12 14L9 11" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        <path d="M16 17H12H8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="M2 12C2 7.28595 2 4.92893 3.46447 3.46447C4.92893 2 7.28595 2 12 2C16.714 2 19.0711 2 20.5355 3.46447C22 4.92893 22 7.28595 22 12C22 16.714 22 19.0711 20.5355 20.5355C19.0711 22 16.714 22 12 22C7.28595 22 4.92893 22 3.46447 20.5355C2 19.0711 2 16.714 2 12Z" stroke="currentColor" strokeWidth="1.5" />
      </svg>
    );
  }
  if (kind === "retry") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" className="is-retry">
        <path
          d="M8.25005 8.5C8.25005 8.91421 8.58584 9.25 9.00005 9.25C9.41426 9.25 9.75005 8.91421 9.75005 8.5H8.25005ZM9.00005 8.267H9.75006L9.75004 8.26283L9.00005 8.267ZM9.93892 5.96432L10.4722 6.49171L9.93892 5.96432ZM12.2311 5V4.24999L12.2269 4.25001L12.2311 5ZM16.269 5L16.2732 4.25H16.269V5ZM18.5612 5.96432L18.0279 6.49171V6.49171L18.5612 5.96432ZM19.5 8.267L18.75 8.26283V8.267H19.5ZM19.5 12.233H18.75L18.7501 12.2372L19.5 12.233ZM18.5612 14.5357L18.0279 14.0083L18.5612 14.5357ZM16.269 15.5V16.25L16.2732 16.25L16.269 15.5ZM16 14.75C15.5858 14.75 15.25 15.0858 15.25 15.5C15.25 15.9142 15.5858 16.25 16 16.25V14.75ZM9.00005 9.25C9.41426 9.25 9.75005 8.91421 9.75005 8.5C9.75005 8.08579 9.41426 7.75 9.00005 7.75V9.25ZM8.73105 8.5V7.74999L8.72691 7.75001L8.73105 8.5ZM6.43892 9.46432L6.97218 9.99171L6.43892 9.46432ZM5.50005 11.767H6.25006L6.25004 11.7628L5.50005 11.767ZM5.50005 15.734L6.25005 15.7379V15.734H5.50005ZM8.73105 19L8.72691 19.75H8.73105V19ZM12.769 19V19.75L12.7732 19.75L12.769 19ZM15.0612 18.0357L14.5279 17.5083L15.0612 18.0357ZM16 15.733H15.25L15.2501 15.7372L16 15.733ZM16.75 15.5C16.75 15.0858 16.4143 14.75 16 14.75C15.5858 14.75 15.25 15.0858 15.25 15.5H16.75ZM9.00005 7.75C8.58584 7.75 8.25005 8.08579 8.25005 8.5C8.25005 8.91421 8.58584 9.25 9.00005 9.25V7.75ZM12.7691 8.5L12.7732 7.75H12.7691V8.5ZM15.0612 9.46432L15.5944 8.93694V8.93694L15.0612 9.46432ZM16.0001 11.767L15.2501 11.7628V11.767H16.0001ZM15.2501 15.5C15.2501 15.9142 15.5858 16.25 16.0001 16.25C16.4143 16.25 16.7501 15.9142 16.7501 15.5H15.2501ZM9.75005 8.5V8.267H8.25005V8.5H9.75005ZM9.75004 8.26283C9.74636 7.60005 10.0061 6.96296 10.4722 6.49171L9.40566 5.43694C8.65985 6.19106 8.24417 7.21056 8.25006 8.27117L9.75004 8.26283ZM10.4722 6.49171C10.9382 6.02046 11.5724 5.75365 12.2352 5.74999L12.2269 4.25001C11.1663 4.25587 10.1515 4.68282 9.40566 5.43694L10.4722 6.49171ZM12.2311 5.75H16.269V4.25H12.2311V5.75ZM16.2649 5.74999C16.9277 5.75365 17.5619 6.02046 18.0279 6.49171L19.0944 5.43694C18.3486 4.68282 17.3338 4.25587 16.2732 4.25001L16.2649 5.74999ZM18.0279 6.49171C18.494 6.96296 18.7537 7.60005 18.7501 8.26283L20.25 8.27117C20.2559 7.21056 19.8402 6.19106 19.0944 5.43694L18.0279 6.49171ZM18.75 8.267V12.233H20.25V8.267H18.75ZM18.7501 12.2372C18.7537 12.8999 18.494 13.537 18.0279 14.0083L19.0944 15.0631C19.8402 14.3089 20.2559 13.2894 20.25 12.2288L18.7501 12.2372ZM18.0279 14.0083C17.5619 14.4795 16.9277 14.7463 16.2649 14.75L16.2732 16.25C17.3338 16.2441 18.3486 15.8172 19.0944 15.0631L18.0279 14.0083ZM16.269 14.75H16V16.25H16.269V14.75ZM9.00005 7.75H8.73105V9.25H9.00005V7.75ZM8.72691 7.75001C7.6663 7.75587 6.65146 8.18282 5.90566 8.93694L6.97218 9.99171C7.43824 9.52046 8.07241 9.25365 8.73519 9.24999L8.72691 7.75001ZM5.90566 8.93694C5.15985 9.69106 4.74417 10.7106 4.75006 11.7712L6.25004 11.7628C6.24636 11.1001 6.50612 10.463 6.97218 9.99171L5.90566 8.93694ZM4.75005 11.767V15.734H6.25005V11.767H4.75005ZM4.75006 15.7301C4.73847 17.9382 6.51879 19.7378 8.72691 19.75L8.7352 18.25C7.35533 18.2424 6.2428 17.1178 6.25004 15.7379L4.75006 15.7301ZM8.73105 19.75H12.769V18.25H8.73105V19.75ZM12.7732 19.75C13.8338 19.7441 14.8486 19.3172 15.5944 18.5631L14.5279 17.5083C14.0619 17.9795 13.4277 18.2463 12.7649 18.25L12.7732 19.75ZM15.5944 18.5631C16.3402 17.8089 16.7559 16.7894 16.75 15.7288L15.2501 15.7372C15.2537 16.3999 14.994 17.037 14.5279 17.5083L15.5944 18.5631ZM16.75 15.733V15.5H15.25V15.733H16.75ZM9.00005 9.25H12.7691V7.75H9.00005V9.25ZM12.7649 9.24999C13.4277 9.25365 14.0619 9.52046 14.5279 9.99171L15.5944 8.93694C14.8486 8.18282 13.8338 7.75587 12.7732 7.75001L12.7649 9.24999ZM14.5279 9.99171C14.994 10.463 15.2537 11.1001 15.2501 11.7628L16.75 11.7712C16.7559 10.7106 16.3402 9.69106 15.5944 8.93694L14.5279 9.99171ZM15.2501 11.767V15.5H16.7501V11.767H15.2501Z"
          fill="currentColor"
        />
      </svg>
    );
  }
  return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" className="is-delete">
      <path d="M3 6C3 4.34315 4.34315 3 6 3H8.75C9.37951 3 9.97229 3.29639 10.35 3.8L11.4 5.2C11.7777 5.70361 12.3705 6 13 6H18C19.6569 6 21 7.34315 21 9V18C21 19.6569 19.6569 21 18 21H6C4.34315 21 3 19.6569 3 18V6Z" stroke="currentColor" strokeWidth="2" />
      <path d="M9.5 12L14.5 17M14.5 12L9.5 17" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function ImportTaskStatusIcon({
  kind,
}: {
  kind: "success" | "successAttention" | "duplicate" | "error" | "warn" | "running" | "waiting";
}): JSX.Element {
  if (kind === "success") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path
          d="M16.0303 10.0303C16.3232 9.73744 16.3232 9.26256 16.0303 8.96967C15.7374 8.67678 15.2626 8.67678 14.9697 8.96967L10.5 13.4393L9.03033 11.9697C8.73744 11.6768 8.26256 11.6768 7.96967 11.9697C7.67678 12.2626 7.67678 12.7374 7.96967 13.0303L9.96967 15.0303C10.2626 15.3232 10.7374 15.3232 11.0303 15.0303L16.0303 10.0303Z"
          fill="currentColor"
        />
        <path
          fillRule="evenodd"
          clipRule="evenodd"
          d="M12.0574 1.25H11.9426C9.63424 1.24999 7.82519 1.24998 6.41371 1.43975C4.96897 1.63399 3.82895 2.03933 2.93414 2.93414C2.03933 3.82895 1.63399 4.96897 1.43975 6.41371C1.24998 7.82519 1.24999 9.63422 1.25 11.9426V12.0574C1.24999 14.3658 1.24998 16.1748 1.43975 17.5863C1.63399 19.031 2.03933 20.1711 2.93414 21.0659C3.82895 21.9607 4.96897 22.366 6.41371 22.5603C7.82519 22.75 9.63423 22.75 11.9426 22.75H12.0574C14.3658 22.75 16.1748 22.75 17.5863 22.5603C19.031 22.366 20.1711 21.9607 21.0659 21.0659C21.9607 20.1711 22.366 19.031 22.5603 17.5863C22.75 16.1748 22.75 14.3658 22.75 12.0574V11.9426C22.75 9.63423 22.75 7.82519 22.5603 6.41371C22.366 4.96897 21.9607 3.82895 21.0659 2.93414C20.1711 2.03933 19.031 1.63399 17.5863 1.43975C16.1748 1.24998 14.3658 1.24999 12.0574 1.25ZM3.9948 3.9948C4.56445 3.42514 5.33517 3.09825 6.61358 2.92637C7.91356 2.75159 9.62177 2.75 12 2.75C14.3782 2.75 16.0864 2.75159 17.3864 2.92637C18.6648 3.09825 19.4355 3.42514 20.0052 3.9948C20.5749 4.56445 20.9018 5.33517 21.0736 6.61358C21.2484 7.91356 21.25 9.62177 21.25 12C21.25 14.3782 21.2484 16.0864 21.0736 17.3864C20.9018 18.6648 20.5749 19.4355 20.0052 20.0052C19.4355 20.5749 18.6648 20.9018 17.3864 21.0736C16.0864 21.2484 14.3782 21.25 12 21.25C9.62177 21.25 7.91356 21.2484 6.61358 21.0736C5.33517 20.9018 4.56445 20.5749 3.9948 20.0052C3.42514 19.4355 3.09825 18.6648 2.92637 17.3864C2.75159 16.0864 2.75 14.3782 2.75 12C2.75 9.62177 2.75159 7.91356 2.92637 6.61358C3.09825 5.33517 3.42514 4.56445 3.9948 3.9948Z"
          fill="currentColor"
        />
      </svg>
    );
  }
  if (kind === "successAttention") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path d="M12 8L12 13" stroke="currentColor" strokeWidth="1.56" strokeLinecap="round" />
        <path d="M12 16V15.9888" stroke="currentColor" strokeWidth="1.56" strokeLinecap="round" />
        <path d="M3 12C3 4.5885 4.5885 3 12 3C19.4115 3 21 4.5885 21 12C21 19.4115 19.4115 21 12 21C4.5885 21 3 19.4115 3 12Z" stroke="currentColor" strokeWidth="1.56" />
      </svg>
    );
  }
  if (kind === "duplicate") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path
          opacity="0.4"
          d="M22 11.1V6.9C22 3.4 20.6 2 17.1 2H12.9C9.4 2 8 3.4 8 6.9V8H11.1C14.6 8 16 9.4 16 12.9V16H17.1C20.6 16 22 14.6 22 11.1Z"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M16 17.1V12.9C16 9.4 14.6 8 11.1 8H6.9C3.4 8 2 9.4 2 12.9V17.1C2 20.6 3.4 22 6.9 22H11.1C14.6 22 16 20.6 16 17.1Z"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path d="M6.08008 14.9998L8.03008 16.9498L11.9201 13.0498" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    );
  }
  if (kind === "error") {
    return (
      <svg viewBox="0 0 16 16" fill="none" aria-hidden="true">
        <path d="M5.1 5.1 10.9 10.9M10.9 5.1 5.1 10.9" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
      </svg>
    );
  }
  if (kind === "running") {
    return (
      <svg viewBox="0 0 16 16" fill="none" aria-hidden="true" className="is-spinning">
        <path d="M8 2.3a5.7 5.7 0 1 0 4.9 2.9" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
      </svg>
    );
  }
  if (kind === "warn") {
    return (
      <svg viewBox="0 0 16 16" fill="none" aria-hidden="true">
        <path d="M8 4.1v4.2" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
        <circle cx="8" cy="11.6" r=".9" fill="currentColor" />
      </svg>
    );
  }
  return (
    <svg viewBox="0 0 32 32" fill="none" aria-hidden="true">
      <line x1="16" y1="3" x2="16" y2="8" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="6.8" y1="6.8" x2="10.3" y2="10.3" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="3" y1="16" x2="8" y2="16" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="6.8" y1="25.2" x2="10.3" y2="21.7" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="16" y1="29" x2="16" y2="24" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="25.2" y1="25.2" x2="21.7" y2="21.7" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="29" y1="16" x2="24" y2="16" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <line x1="25.2" y1="6.8" x2="21.7" y2="10.3" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

const CATEGORY_META: Array<{
  key: Exclude<ImportDomainCategory, "all">;
  label: string;
  english: string;
  hint: string;
}> = [
  {
    key: "structured",
    label: "资金数据",
    english: "Funds Data",
    hint: "银行资金交易明细和账户信息"
  },
  {
    key: "entity",
    label: "主体信息",
    english: "Entity Profiles",
    hint: "银行账户登记预留信息"
  },
  {
    key: "support",
    label: "研判文件",
    english: "Research Files",
    hint: "相关文件资料"
  }
];

function renderImportCategoryIcon(category: ImportCategoryCardModel["key"]): JSX.Element {
  if (category === "structured") {
    return (
      <svg viewBox="0 0 20 20" fill="none" aria-hidden="true">
        <rect x="3.5" y="3.5" width="13" height="13" rx="2" stroke="currentColor" strokeWidth="1.4" />
        <path d="M5.25 6.5H14.75" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
        <path d="M5.25 10H14.75" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
        <path d="M5.25 13.5H11.25" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      </svg>
    );
  }
  if (category === "entity") {
    return (
      <svg viewBox="0 0 20 20" fill="none" aria-hidden="true">
        <path d="M10 9.5A2.75 2.75 0 1 0 10 4A2.75 2.75 0 0 0 10 9.5Z" stroke="currentColor" strokeWidth="1.4" />
        <path d="M4.5 15.5C5.1 12.95 6.98 11.7 10 11.7C13.02 11.7 14.9 12.95 15.5 15.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      </svg>
    );
  }
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true">
      <path d="M6 4.25H11.5L14.5 7.25V15.25C14.5 16.08 13.83 16.75 13 16.75H6C5.17 16.75 4.5 16.08 4.5 15.25V5.75C4.5 4.92 5.17 4.25 6 4.25Z" stroke="currentColor" strokeWidth="1.4" />
      <path d="M11.25 4.5V7.5H14.25" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M7.25 10.25H11.75" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      <path d="M7.25 13H10.25" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  );
}

function renderImportLedgerIcon(): JSX.Element {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true">
      <path d="M6.25 8.25A2.25 2.25 0 1 0 6.25 3.75A2.25 2.25 0 0 0 6.25 8.25Z" stroke="currentColor" strokeWidth="1.4" />
      <path d="M13.75 7.25A1.75 1.75 0 1 0 13.75 3.75A1.75 1.75 0 0 0 13.75 7.25Z" stroke="currentColor" strokeWidth="1.4" />
      <path d="M2.75 14.75C3.18 12.58 4.71 11.5 6.75 11.5C8.79 11.5 10.32 12.58 10.75 14.75" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      <path d="M11.9 14.25C12.17 12.82 13.23 12 14.7 12C16.15 12 17.16 12.8 17.45 14.25" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  );
}

function mappingChildSummaryMetaItems(child: ImportWizardArchiveChild, insight: MappingInsight): string[] {
  const total = Math.max(0, insight.headerCount);
  const matched = mappingMatchedColumnCount(insight);
  const pending = Math.max(0, insight.unresolvedCount);
  const kindLabel = wizardArchiveChildKindLabel(child);
  if (total <= 0) {
    return [kindLabel, "待解析表头"];
  }
  return [
    kindLabel,
    `${total.toLocaleString("zh-CN")} 个字段`,
    `已匹配 ${matched.toLocaleString("zh-CN")}/${total.toLocaleString("zh-CN")}`,
    pending > 0 ? `待确认 ${pending.toLocaleString("zh-CN")}` : "已全部确认",
  ];
}

interface MappingWorkbenchCanvasProps {
  scopeId: string;
  kind: string;
  insight: MappingInsight;
  headers: string[];
  sampleRows: string[][];
  activeTargetKey: string;
  onActiveTargetChange: (fieldKey: string) => void;
  onAssignTarget: (fieldKey: string, sourceHeader: string) => void;
  onAssignSource: (sourceHeader: string, fieldKey: string) => void;
}

function mappingColumnOriginLabel(column: MappingWorkbenchColumn): string {
  if (column.origin === "manual") {
    return "手动映射";
  }
  if (column.origin === "ai") {
    return "AI 建议";
  }
  if (column.origin === "auto") {
    return "自动命中";
  }
  if (column.state === "info") {
    return "扩展字段";
  }
  if (column.state === "unresolved") {
    return "待确认";
  }
  return "待绑定";
}

function mappingSourceStateLabel(column: MappingWorkbenchColumn): string {
  if (column.origin === "manual") {
    return "已手动指定";
  }
  if (column.origin === "ai") {
    return "AI 建议";
  }
  if (column.origin === "auto") {
    return column.targetRequired ? "关键字段已识别" : "已自动识别";
  }
  if (column.state === "info") {
    return "扩展保留";
  }
  if (column.state === "suggested") {
    return "建议复核";
  }
  if (column.state === "matched") {
    return "已完成匹配";
  }
  return "待人工确认";
}

function MappingWorkbenchCanvas(props: MappingWorkbenchCanvasProps): JSX.Element {
  const { insight, headers, kind, sampleRows, onAssignSource } = props;
  const [selectedFilter, setSelectedFilter] = useState<MappingWorkbenchFilter | "">("");
  const workbenchScopeId = toDomIdFragment(props.scopeId);
  const workbenchPanelId = `import-mapping-workbench-panel-${workbenchScopeId}`;
  const columns = buildMappingWorkbenchColumns(insight, headers, sampleRows, kind);
  const requiredPendingCount = Math.max(0, insight.requiredTotal - insight.matchedRequired);
  const defaultFilter = defaultMappingWorkbenchFilter(insight);
  const effectiveFilter = selectedFilter || defaultFilter;
  const matchedCount = mappingMatchedColumnCount(insight);
  const totalCount = Math.max(0, insight.headerCount);
  const reviewPendingCount = Math.max(0, insight.unresolvedCount);
  const extendedCount = Math.max(0, totalCount - matchedCount - reviewPendingCount);
  const filterOptions = [
    { value: "pending" as const, label: "待确认" },
    { value: "required" as const, label: "必填" },
    { value: "auto" as const, label: "自动识别" },
    { value: "all" as const, label: "全部" }
  ].map((option) => ({
    ...option,
    count: columns.filter((column) => mappingWorkbenchFilterMatches(column, option.value, requiredPendingCount)).length
  }));
  const filteredColumns = columns.filter((column) => mappingWorkbenchFilterMatches(column, effectiveFilter, requiredPendingCount));
  const displayRows =
    filteredColumns.length > 0
      ? Array.from({ length: 5 }, (_, rowIndex) => filteredColumns.map((column) => column.sampleValues[rowIndex] || ""))
      : [];

  useEffect(() => {
    setSelectedFilter("");
  }, [props.scopeId]);

  const filterCountByValue = new Map(filterOptions.map((option) => [option.value, option.count]));
  const autoSuggestionCount = filterCountByValue.get("auto") ?? 0;
  const summaryLine =
    totalCount > 0
      ? `${totalCount} 列表头 · 已匹配 ${matchedCount} · ${
          reviewPendingCount > 0
            ? `待确认 ${reviewPendingCount}`
            : extendedCount > 0
              ? `扩展字段 ${extendedCount}`
              : "已全部匹配"
        }`
      : "待解析表头";
  const summaryNote =
    reviewPendingCount > 0
      ? autoSuggestionCount > 0
        ? `系统已自动识别 ${autoSuggestionCount} 项，可优先处理待确认字段`
        : "优先处理待确认字段，再切换到全部视图复核"
      : extendedCount > 0
        ? `关键字段已确认，${extendedCount} 个非模板字段会保留为扩展数据`
      : autoSuggestionCount > 0
        ? `字段已确认，可按需查看 ${autoSuggestionCount} 项自动识别`
        : "字段已确认，可切换全部视图复核样本";

  if (columns.length === 0 || insight.targetFields.length === 0) {
    return (
      <div className="import-console-mapping-workbench is-empty">
        <div className="import-console-mapping-workbench__empty">
          当前未返回足够的字段样本，系统会在后续校验阶段继续识别结构。
        </div>
      </div>
    );
  }

  return (
    <div className="import-console-mapping-workbench">
      <div className="import-console-mapping-workbench__focusbar">
        <div className="import-console-mapping-workbench__summary">
          <strong>{summaryLine}</strong>
          <span>{summaryNote}</span>
        </div>
        <div className="import-console-mapping-workbench__filters" role="tablist" aria-label="字段映射聚焦筛选">
          {filterOptions.map((option) => {
            const active = effectiveFilter === option.value;
            const filterTabId = `import-mapping-filter-${workbenchScopeId}-${option.value}`;
            return (
              <button
                key={`${props.scopeId}:${option.value}`}
                id={filterTabId}
                type="button"
                role="tab"
                aria-selected={active}
                aria-controls={workbenchPanelId}
                tabIndex={active ? 0 : -1}
                className={`import-console-mapping-workbench__filter is-${option.value}${active ? " is-active" : ""}`}
                disabled={option.value !== "all" && option.count === 0}
                onClick={() => setSelectedFilter(option.value)}
                onKeyDown={handleTabListKeyDown}
              >
                <span>{option.label}</span>
                <strong>{option.count.toLocaleString("zh-CN")}</strong>
              </button>
            );
          })}
        </div>
      </div>
      <div
        className="import-console-mapping-workbench__table-shell"
        id={workbenchPanelId}
        role="tabpanel"
        aria-labelledby={`import-mapping-filter-${workbenchScopeId}-${effectiveFilter}`}
      >
        {filteredColumns.length > 0 ? (
          <div className="import-console-mapping-workbench__table-scroll">
            <table className="import-console-mapping-workbench__table">
              <thead>
                <tr className="import-console-mapping-workbench__target-row">
                  <th className="import-console-mapping-workbench__stub import-console-mapping-workbench__stub--target">
                    <div className="import-console-mapping-workbench__stub-copy">
                      <strong>映射表头</strong>
                    </div>
                  </th>
                  {filteredColumns.map((column) => (
                    <th
                      key={`${props.scopeId}:${column.header}`}
                      className={`import-console-mapping-workbench__target-cell is-${column.origin} is-${column.state} ${column.targetKey ? "is-bound" : "is-empty"} ${column.targetKey && column.targetKey === props.activeTargetKey ? "is-active-target" : ""}`}
                    >
                      <div className="import-console-mapping-workbench__mapping-head">
                        <div className="import-console-mapping-workbench__mapping-meta">
                          <div className="import-console-mapping-workbench__mapping-tags">
                            <span className={`import-console-mapping-workbench__column-target ${column.targetKey ? `is-${column.origin}` : "is-empty"}`}>
                              {mappingColumnOriginLabel(column)}
                            </span>
                            {column.targetRequired ? (
                              <span className="import-console-mapping-workbench__column-target is-required">必填</span>
                            ) : null}
                          </div>
                        </div>
                        <select
                          className="import-console-select import-console-mapping-workbench__select"
                          value={column.targetKey}
                          aria-label={`设置 ${projectOrdinaryFieldValue("node_id", column.header)} 的映射字段`}
                          onFocus={() => {
                            if (column.targetKey) {
                              props.onActiveTargetChange(column.targetKey);
                            }
                          }}
                          onChange={(event) => {
                            props.onActiveTargetChange(event.target.value);
                            onAssignSource(column.header, event.target.value);
                          }}
                        >
                          <option value="">未绑定</option>
                          {insight.targetFields.map((field) => (
                            <option key={`${props.scopeId}:${column.header}:${field.key}`} value={field.key}>
                              {field.importHeader || field.label}
                              {field.required ? "（必填）" : ""}
                            </option>
                          ))}
                        </select>
                      </div>
                    </th>
                  ))}
                </tr>
                <tr className="import-console-mapping-workbench__source-row">
                  <th className="import-console-mapping-workbench__stub import-console-mapping-workbench__stub--source">
                    <div className="import-console-mapping-workbench__stub-copy">
                      <strong>原表头</strong>
                    </div>
                  </th>
                  {filteredColumns.map((column) => (
                    <th
                      key={`${props.scopeId}:${column.header}:source`}
                      className={`import-console-mapping-workbench__source-cell is-${column.origin} is-${column.state} ${column.targetKey ? "is-bound" : "is-empty"} ${column.targetKey && column.targetKey === props.activeTargetKey ? "is-active-target" : ""}`}
                    >
                      <div className="import-console-mapping-workbench__column-head">
                        <strong title={projectOrdinaryFieldValue("node_id", column.header)}>
                          {projectOrdinaryFieldValue("node_id", column.header)}
                        </strong>
                        <span className={`import-console-mapping-workbench__column-state is-${column.matchBasisTone}`}>{mappingSourceStateLabel(column)}</span>
                        <small className={`import-console-mapping-workbench__column-basis is-${column.matchBasisTone}`}>{column.matchBasis}</small>
                      </div>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {displayRows.map((row, rowIndex) => (
                  <tr key={`${props.scopeId}:sample:${rowIndex}`}>
                    <th className="import-console-mapping-workbench__sample-index">
                      <div className="import-console-mapping-workbench__sample-index-copy">{rowIndex + 1}</div>
                    </th>
                    {row.map((cell, cellIndex) => (
                      <td key={`${props.scopeId}:sample:${rowIndex}:${cellIndex}`}>
                        {projectOrdinaryFieldValue(
                          filteredColumns[cellIndex]?.targetKey || filteredColumns[cellIndex]?.header,
                          cell
                        ).trim() || "—"}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="import-console-mapping-workbench__empty is-filtered">
            当前筛选下暂无可处理列，切回“全部”即可查看完整字段。
          </div>
        )}
      </div>
    </div>
  );
}

function MappingAssistButton(props: {
  disabled: boolean;
  running: boolean;
  onClick: () => void;
}): JSX.Element {
  const { disabled, running, onClick } = props;
  return (
    <button type="button" className="import-console-mapping-card__ai-action" disabled={disabled} onClick={onClick}>
      <svg viewBox="0 0 20 20" fill="none" aria-hidden="true">
        <path
          d="M10 2.5 11.664 6.336 15.5 8l-3.836 1.664L10 13.5 8.336 9.664 4.5 8l3.836-1.664L10 2.5Z"
          fill="currentColor"
        />
        <path d="M15.25 12.25 15.915 13.835 17.5 14.5l-1.585.665L15.25 16.75l-.665-1.585L13 14.5l1.585-.665.665-1.585Z" fill="currentColor" opacity="0.8" />
      </svg>
      <span>{running ? "AI 识别中" : "AI 智能补全"}</span>
    </button>
  );
}

function MappingTemplateToolbar(props: {
  scopeKey: string;
  selectedCategory: Exclude<ImportDomainCategory, "all">;
  selectedKind: string;
  assistState?: MappingAssistState | null;
  action?: JSX.Element | null;
  onCategoryChange: (category: Exclude<ImportDomainCategory, "all">) => void;
  onKindChange: (kind: string) => void;
}): JSX.Element {
  const {
    scopeKey,
    selectedCategory,
    selectedKind,
    assistState,
    action,
    onCategoryChange,
    onKindChange
  } = props;
  const templateScopeId = toDomIdFragment(scopeKey);

  return (
    <>
      <div className="import-console-mapping-card__toolbar">
        <div className="import-console-mapping-card__toolbar-main">
          <span className="import-console-mapping-card__toolbar-label">模板</span>
          <div className="import-console-chip-group import-console-chip-group--mapping" role="tablist" aria-label="选择导入模板">
            {CATEGORY_META.map((category) => {
              const active = selectedCategory === category.key;
              return (
                <button
                  key={`${scopeKey}:${category.key}`}
                  id={`import-template-tab-${templateScopeId}-${category.key}`}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  tabIndex={active ? 0 : -1}
                  className={`import-console-chip ${active ? "is-active" : ""}`}
                  onClick={() => onCategoryChange(category.key)}
                  onKeyDown={handleTabListKeyDown}
                >
                  {category.label}
                </button>
              );
            })}
          </div>
        </div>
        <label className="import-console-mapping-card__toolbar-field">
          <span>类别</span>
          <select className="import-console-select" value={selectedKind} onChange={(event) => onKindChange(event.target.value)}>
            <option value="">请选择类别</option>
            {categoryKindOptions(selectedCategory).map((option) => (
              <option key={`${scopeKey}:${option.value}`} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        {action ? <div className="import-console-mapping-card__toolbar-actions">{action}</div> : null}
      </div>
      {assistState && assistState.status !== "idle" ? (
        <div className={`import-console-mapping-card__assistant-note is-${assistState.status}`}>{assistState.message}</div>
      ) : null}
    </>
  );
}

function MappingPendingTemplateFieldList(props: {
  fields: MappingTargetFieldPreview[];
}): JSX.Element | null {
  const { fields } = props;
  if (fields.length === 0) {
    return null;
  }

  return (
    <div className="import-console-mapping-card__pending-fields">
      <div className="import-console-mapping-card__pending-head">
        <strong>未映射模板字段</strong>
        <span>{fields.some((field) => field.required) ? `共 ${fields.length} 项，请优先确认必填字段。` : `共 ${fields.length} 项待补充。`}</span>
      </div>
      <div className="import-console-mapping-card__pending-list">
        {fields.map((field) => (
          <span key={field.key} className={`import-console-mapping-card__pending-chip ${field.required ? "is-required" : ""}`}>
            <em>{field.required ? "必填" : "建议"}</em>
            <strong>{field.importHeader || field.label}</strong>
          </span>
        ))}
      </div>
    </div>
  );
}

function MappingArchiveChildDetailPanel(props: {
  fileId: string;
  child: ImportWizardArchiveChild;
  childInsight: MappingInsight;
  assistState: MappingAssistState;
  activeTargetKey: string;
  pendingFields: MappingTargetFieldPreview[];
  onApplyAssist: () => void;
  onCategoryChange: (category: Exclude<ImportDomainCategory, "all">) => void;
  onKindChange: (kind: string) => void;
  onActiveTargetChange: (fieldKey: string) => void;
  onAssignTarget: (fieldKey: string, sourceHeader: string) => void;
  onAssignSource: (sourceHeader: string, fieldKey: string) => void;
}): JSX.Element {
  const {
    fileId,
    child,
    childInsight,
    assistState,
    activeTargetKey,
    pendingFields,
    onApplyAssist,
    onCategoryChange,
    onKindChange,
    onActiveTargetChange,
    onAssignTarget,
    onAssignSource,
  } = props;
  const childResult = mappingScopeResult({
    status: child.status,
    selectedCategory: child.domainCategory,
    insight: childInsight,
  });

  return (
    <div className="import-console-upload-card__expand-item-detail-body">
      <div className="import-console-mapping-card__editor-head">
        <div className="import-console-mapping-card__editor-title">
          <strong>{wizardArchiveChildKindLabel(child)}</strong>
        </div>
        <div className="import-console-mapping-card__editor-meta">
          <span className={`import-console-mapping-card__progress-pill is-${childResult.tone === "success" ? "success" : childResult.tone === "warn" ? "warn" : "error"}`}>
            {mappingProgressLabel(childInsight)}
          </span>
        </div>
      </div>
      <MappingTemplateToolbar
        scopeKey={`${fileId}:${child.id}`}
        selectedCategory={child.domainCategory}
        selectedKind={child.selectedKind}
        assistState={assistState}
        action={
          child.domainCategory !== "support" && childInsight.targetFields.length > 0 ? (
            <MappingAssistButton
              disabled={assistState.status === "running"}
              running={assistState.status === "running"}
              onClick={onApplyAssist}
            />
          ) : null
        }
        onCategoryChange={onCategoryChange}
        onKindChange={onKindChange}
      />
      <MappingPendingTemplateFieldList fields={pendingFields} />
      {child.domainCategory !== "support" ? (
        <MappingWorkbenchCanvas
          scopeId={`${fileId}::${child.id}`}
          kind={child.selectedKind || child.suggestedKind}
          insight={childInsight}
          headers={child.headerPreview}
          sampleRows={child.sampleRows}
          activeTargetKey={activeTargetKey}
          onActiveTargetChange={onActiveTargetChange}
          onAssignTarget={onAssignTarget}
          onAssignSource={onAssignSource}
        />
      ) : (
        <div className="import-console-mapping-card__empty">
          当前子项会按研判文件登记，不需要结构化字段映射。
        </div>
      )}
    </div>
  );
}

function ImportArchiveMappingBranchCard(props: {
  file: ImportWizardFile;
  isExpanded: boolean;
  parentCardClassName: string;
  parentSummary: ReactNode;
  archiveItems: Array<{ child: ImportWizardArchiveChild; childInsight: MappingInsight }>;
  activeArchiveChildId: string;
  onToggleChildDetail: (childId: string) => void;
  mappingAssistById: Record<string, MappingAssistState>;
  activeMappingTargetById: Record<string, string>;
  setActiveMappingTargetById: Dispatch<SetStateAction<Record<string, string>>>;
  applyArchiveChildMappingAssist: (fileId: string, childId: string) => Promise<void>;
  updateWizardArchiveChildCategory: (fileId: string, childId: string, category: Exclude<ImportDomainCategory, "all">) => void;
  updateWizardArchiveChildKind: (fileId: string, childId: string, kind: string) => void;
  updateWizardArchiveChildFieldMapping: (fileId: string, childId: string, fieldKey: string, sourceHeader: string) => void;
  updateWizardArchiveChildSourceBinding: (fileId: string, childId: string, sourceHeader: string, fieldKey: string) => void;
}): JSX.Element {
  const {
    file,
    isExpanded,
    parentCardClassName,
    parentSummary,
    archiveItems,
    activeArchiveChildId,
    onToggleChildDetail,
    mappingAssistById,
    activeMappingTargetById,
    setActiveMappingTargetById,
    applyArchiveChildMappingAssist,
    updateWizardArchiveChildCategory,
    updateWizardArchiveChildKind,
    updateWizardArchiveChildFieldMapping,
    updateWizardArchiveChildSourceBinding,
  } = props;

  const stats = archiveItems.reduce(
    (accumulator, { child, childInsight }) => {
      const result = mappingScopeResult({
        status: child.status,
        selectedCategory: child.domainCategory,
        insight: childInsight,
      });
      if (result.tone === "success") {
        accumulator.ready += 1;
      } else if (result.tone === "warn") {
        accumulator.pending += 1;
      } else {
        accumulator.failed += 1;
      }
      return accumulator;
    },
    { ready: 0, pending: 0, failed: 0 }
  );

  const items: ArchiveExpandableItem[] = archiveItems.map(({ child, childInsight }) => {
    const childResult = mappingScopeResult({
      status: child.status,
      selectedCategory: child.domainCategory,
      insight: childInsight,
    });
    const assistScopeKey = `${file.id}::${child.id}`;
    const assistState = mappingAssistById[assistScopeKey] ?? { status: "idle", message: "" };
    const activeTargetKey =
      activeMappingTargetById[assistScopeKey] &&
      childInsight.targetFields.some((field) => field.key === activeMappingTargetById[assistScopeKey])
        ? activeMappingTargetById[assistScopeKey]
        : preferredActiveMappingFieldKey(childInsight.targetFields);

    return {
      id: child.id,
      cardTitle: child.issue || childResult.message,
      detailOpen: activeArchiveChildId === child.id,
      summary: (
        <ImportWorkflowFileSummary
          fileName={child.fileName}
          fileType={child.fileType}
          statusTone={childResult.tone}
          statusLabel={childResult.label}
          metaItems={mappingChildSummaryMetaItems(child, childInsight)}
          actions={
            <button
              type="button"
              className={`import-console-upload-card__action-toggle${activeArchiveChildId === child.id ? " is-active" : ""}`}
              onClick={() => onToggleChildDetail(child.id)}
            >
              <span>{activeArchiveChildId === child.id ? "收起字段映射" : "查看字段映射"}</span>
            </button>
          }
        />
      ),
      renderDetail: () => (
        <MappingArchiveChildDetailPanel
          fileId={file.id}
          child={child}
          childInsight={childInsight}
          assistState={assistState}
          activeTargetKey={activeTargetKey}
          pendingFields={mappingPendingTemplateFields(childInsight)}
          onApplyAssist={() => void applyArchiveChildMappingAssist(file.id, child.id)}
          onCategoryChange={(category) => updateWizardArchiveChildCategory(file.id, child.id, category)}
          onKindChange={(kind) => updateWizardArchiveChildKind(file.id, child.id, kind)}
          onActiveTargetChange={(fieldKey) =>
            setActiveMappingTargetById((previous) => ({ ...previous, [assistScopeKey]: fieldKey }))
          }
          onAssignTarget={(fieldKey, sourceHeader) =>
            updateWizardArchiveChildFieldMapping(file.id, child.id, fieldKey, sourceHeader)
          }
          onAssignSource={(sourceHeader, fieldKey) =>
            updateWizardArchiveChildSourceBinding(file.id, child.id, sourceHeader, fieldKey)
          }
        />
      ),
    };
  });

  return (
    <ImportArchiveExpandableBranch
      isExpanded={isExpanded}
      parentCardClassName={parentCardClassName}
      parentSummary={parentSummary}
      headerTitle="子项映射"
      headerCountLabel={`${archiveItems.length.toLocaleString("zh-CN")} 个子项`}
      headerStats={
        <>
          <span className="tone-success">已确认 {stats.ready.toLocaleString("zh-CN")}</span>
          <span className={stats.pending > 0 ? "tone-warn" : "tone-muted"}>
            待处理 {stats.pending.toLocaleString("zh-CN")}
          </span>
          <span className={stats.failed > 0 ? "tone-error" : "tone-muted"}>
            异常 {stats.failed.toLocaleString("zh-CN")}
          </span>
        </>
      }
      listAriaLabel="压缩包字段映射子项列表"
      items={items}
    />
  );
}

function ImportExecutionStatusAction(props: {
  label: string;
  tone: "success" | "error" | "warn" | "running" | "muted";
  onClick?: () => void;
}): JSX.Element {
  const { label, tone, onClick } = props;
  const className = `import-console-upload-card__action-toggle import-console-upload-card__action-toggle--execution tone-${tone}${onClick ? " is-clickable" : ""}`;

  if (onClick) {
    return (
      <button type="button" className={className} onClick={onClick}>
        <span>{label}</span>
      </button>
    );
  }

  return (
    <span className={className} role="status" aria-live="polite">
      <span>{label}</span>
    </span>
  );
}

function ImportExecutionProgressBlock(props: {
  progress: number;
  tone: "success" | "error" | "warn" | "running" | "muted";
  label: string;
}): JSX.Element {
  const { progress, tone, label } = props;
  const clampedProgress = clampWizardProgress(progress);

  return (
    <div
      className="import-console-upload-card__execution-progress"
      data-execution-tone={tone}
      data-complete={clampedProgress >= 100 ? "true" : "false"}
    >
      <span className="import-console-upload-card__execution-progress-label">{label}</span>
      <div className="import-console-upload-card__progress" role="progressbar" aria-valuenow={clampedProgress} aria-valuemin={0} aria-valuemax={100}>
        <span style={{ width: `${clampedProgress}%` }} />
      </div>
      <strong className="import-console-upload-card__execution-progress-value">{clampedProgress.toLocaleString("zh-CN")}%</strong>
    </div>
  );
}

function ImportValidationProgressGroupShell(props: {
  groupId: string;
  hidden: boolean;
  active: boolean;
  children: ReactNode;
}): JSX.Element {
  const { groupId, hidden, active, children } = props;
  const prefersReducedMotion = usePrefersReducedMotion();
  const motion = useAnimatedExpandPresence({
    open: !hidden,
    enterDurationMs: VALIDATION_GROUP_ENTER_DURATION_MS,
    exitDurationMs: VALIDATION_GROUP_EXIT_DURATION_MS,
    prefersReducedMotion,
  });

  if (!motion.shouldRender) {
    return <></>;
  }

  return (
    <div
      className="import-console-validation-list__item"
      data-visibility-state={motion.motionState}
      data-progress-group-active={active ? "true" : "false"}
      data-progress-id={groupId}
      style={motion.shellStyle}
    >
      <div className="import-console-validation-list__item-shell" ref={motion.contentRef}>
        {children}
      </div>
    </div>
  );
}

function ImportValidationArchiveBranchCard(props: {
  file: ImportWizardProgressGroup;
  isExpanded: boolean;
  started: boolean;
  hideCompletedChildren?: boolean;
  onToggleExpand: () => void;
  onCopyText: (value: string, label: string) => void | Promise<void>;
  onOpenDetail: () => void;
  onOpenChildDetail: (child: ImportWizardProgressChild) => void;
}): JSX.Element {
  const { file, isExpanded, started, hideCompletedChildren = false, onToggleExpand, onCopyText, onOpenDetail, onOpenChildDetail } = props;
  const completedCount = file.childProgress.filter((child) => child.status === "succeeded").length;
  const runningCount = file.childProgress.filter((child) => child.status === "running" || child.isActive).length;
  const failedCount = file.childProgress.filter((child) => child.status === "failed" || child.status === "unsupported").length;
  const pendingCount = Math.max(0, file.childProgress.length - completedCount - runningCount - failedCount);
  const parentTone = validationExecutionTone(file.status, started);
  const parentCardClassName = `import-console-upload-card is-archive-batch is-collapsed is-archive-collapsed import-console-upload-card--execution is-execution-${parentTone}${
    file.isActive ? " is-validation-active" : ""
  }`;

  const parentSummary = (
    <>
      <ImportWorkflowFileSummary
        fileName={file.fileName}
        fileType={file.fileType}
        statusTone={parentTone}
        statusLabel={validationExecutionLabel(file.status, started)}
        metaItems={[]}
        actions={
          <>
            <ImportArchiveExpandToggleButton expanded={isExpanded} onClick={onToggleExpand} />
            <ImportExecutionStatusAction
              label={validationExecutionLabel(file.status, started)}
              tone={parentTone}
              onClick={onOpenDetail}
            />
          </>
        }
        contextContent={
          <ImportExecutionProgressBlock
            progress={validationExecutionProgress(file.status, file.progress, started)}
            tone={parentTone}
            label={validationProgressMessageForGroup(file, started)}
          />
        }
      />
    </>
  );

  const items: ArchiveExpandableItem[] = file.childProgress.map((child) => {
    const childTone = validationExecutionTone(child.status, started);
    return {
      id: child.id,
      cardTitle: validationProgressMessageForChild(child, started),
      hidden: hideCompletedChildren && started && child.status === "succeeded",
      active: child.isActive,
      progressId: `${file.id}::${child.id}`,
      summary: (
        <>
          <ImportWorkflowFileSummary
            fileName={child.fileName}
            fileType={child.fileType}
            statusTone={childTone}
            statusLabel={validationExecutionLabel(child.status, started)}
            metaItems={[]}
            actions={
              <ImportExecutionStatusAction
                label={validationExecutionLabel(child.status, started)}
                tone={childTone}
                onClick={() => onOpenChildDetail(child)}
              />
            }
            contextContent={
              <ImportExecutionProgressBlock
                progress={validationExecutionProgress(child.status, child.progress, started)}
                tone={childTone}
                label={validationProgressMessageForChild(child, started)}
              />
            }
          />
        </>
      ),
    };
  });

  return (
    <ImportArchiveExpandableBranch
      isExpanded={isExpanded}
      parentCardClassName={parentCardClassName}
      parentSummary={parentSummary}
      headerTitle="子项清单"
      headerCountLabel={`${file.childProgress.length.toLocaleString("zh-CN")} 个子项`}
      headerStats={
        <>
          <span className="tone-success">已完成 {completedCount.toLocaleString("zh-CN")}</span>
          <span className={runningCount > 0 ? "tone-running" : "tone-muted"}>
            执行中 {runningCount.toLocaleString("zh-CN")}
          </span>
          <span className={pendingCount > 0 ? "tone-warn" : "tone-muted"}>
            待执行 {pendingCount.toLocaleString("zh-CN")}
          </span>
          <span className={failedCount > 0 ? "tone-error" : "tone-muted"}>
            异常 {failedCount.toLocaleString("zh-CN")}
          </span>
        </>
      }
      listAriaLabel="压缩包执行子项列表"
      items={items}
      isActive={file.isActive}
      progressId={file.id}
    />
  );
}

function ImportValidationSingleFileCard(props: {
  file: ImportWizardProgressGroup;
  started: boolean;
  onCopyText: (value: string, label: string) => void | Promise<void>;
  onOpenDetail: () => void;
}): JSX.Element {
  const { file, started, onCopyText, onOpenDetail } = props;
  const tone = validationExecutionTone(file.status, started);

  return (
    <article
      className={`import-console-upload-card is-single-file is-static import-console-upload-card--execution is-execution-${tone}${
        file.isActive ? " is-validation-active" : ""
      }`}
      data-progress-group-active={file.isActive ? "true" : "false"}
      data-progress-id={file.id}
    >
      <ImportWorkflowFileSummary
        fileName={file.fileName}
        fileType={file.fileType}
        statusTone={tone}
        statusLabel={validationExecutionLabel(file.status, started)}
        metaItems={[]}
        actions={
          <ImportExecutionStatusAction
            label={validationExecutionLabel(file.status, started)}
            tone={tone}
            onClick={onOpenDetail}
          />
        }
        contextContent={
          <ImportExecutionProgressBlock
            progress={validationExecutionProgress(file.status, file.progress, started)}
            tone={tone}
            label={validationProgressMessageForGroup(file, started)}
          />
        }
      />
    </article>
  );
}

function ImportMetricCard({ label, value, hint, tone = "neutral" }: ImportMetricCardProps): JSX.Element {
  return (
    <article className={`import-workbench-kpi import-workbench-kpi--${tone}`}>
      <span className="import-workbench-kpi__label">{label}</span>
      <strong>{value}</strong>
      <small>{hint}</small>
    </article>
  );
}

function statusTone(statusText: string): "success" | "error" | "warn" | "running" | "muted" {
  if (statusText.includes("未验证")) {
    return "warn";
  }
  if (statusText.startsWith("已完成")) {
    return "success";
  }
  if (statusText === "失败") {
    return "error";
  }
  if (statusText.startsWith("导入中")) {
    return "running";
  }
  if (statusText === "重复数据") {
    return "warn";
  }
  return "muted";
}

function statusIconAriaLabel(statusText: string): string {
  if (statusText.includes("未验证")) {
    return "统计未验证";
  }
  if (statusText.startsWith("已完成")) {
    return "成功";
  }
  if (statusText === "失败") {
    return "失败";
  }
  if (statusText.startsWith("导入中")) {
    return "进行中";
  }
  if (statusText === "重复数据") {
    return "提示";
  }
  return "状态";
}

function statusBadgeText(statusText: string): string {
  if (statusText.includes("未验证")) {
    return statusText;
  }
  if (statusText.startsWith("已完成")) {
    return "已完成";
  }
  return statusText;
}

function statusBadgeIconKind(
  statusText: string,
  tone: "success" | "error" | "warn" | "running" | "muted",
): "success" | "successAttention" | "duplicate" | "error" | "warn" | "running" | "waiting" {
  if (statusText.includes("未验证")) {
    return "warn";
  }
  if (statusText.startsWith("已完成") && statusText.includes("有提示")) {
    return "successAttention";
  }
  if (statusText === "重复数据") {
    return "duplicate";
  }
  if (statusText === "等待中") {
    return "waiting";
  }
  if (tone === "muted") {
    return "waiting";
  }
  return tone;
}

function toTs(value: string): number {
  const ts = Date.parse(String(value || ""));
  if (Number.isFinite(ts)) {
    return ts;
  }
  return 0;
}

function getImportPageStateStorageKey(caseId: string, field: string): string {
  return `${IMPORT_PAGE_STATE_STORAGE_PREFIX}:${String(caseId || "").trim()}:${field}`;
}

function readImportPageStateValue<T extends string>(
  caseId: string,
  field: string,
  fallback: T,
  allowed?: readonly T[]
): T {
  const normalizedCaseId = String(caseId || "").trim();
  if (!normalizedCaseId) {
    return fallback;
  }
  return readPersistentString(getImportPageStateStorageKey(normalizedCaseId, field), fallback, allowed);
}

function writeImportPageStateValue(caseId: string, field: string, value: string): void {
  const normalizedCaseId = String(caseId || "").trim();
  if (!normalizedCaseId) {
    return;
  }
  writePersistentString(getImportPageStateStorageKey(normalizedCaseId, field), value);
}

export function ImportPage({ active = true }: { active?: boolean }): JSX.Element {
  const {
    state: {
      session: { activeCaseId },
      runtime: { backendHealth, lastWsEvent, wsStatus }
    },
    actions
  } = useAppStore();

  const desktopAvailable = hasDesktopBridge();
  const [hydratedImportCaseId, setHydratedImportCaseId] = useState("");
  const [caseName, setCaseName] = useState("");
  const [jobs, setJobs] = useState<ImportJobDTO[]>([]);
  const [activeFileLogs, setActiveFileLogs] = useState<ImportFileLogDTO[]>([]);
  const [recycleFileLogs, setRecycleFileLogs] = useState<ImportFileLogDTO[]>([]);
  const [historicalDatasets, setHistoricalDatasets] = useState<ImportHistoricalDatasetDTO[]>([]);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [sortField, setSortField] = useState<SortField>("created");
  const [sortDirection, setSortDirection] = useState<SortDirection>("desc");
  const [statusFilter, setStatusFilter] = useState<ImportStatusFilter>("all");
  const [kindFilter, setKindFilter] = useState<ImportKindFilter>("all");
  const [searchText, setSearchText] = useState("");
  const [hint, setHint] = useState("");
  const [error, setError] = useState("");
  const [detailDialog, setDetailDialog] = useState<{ title: string; text: string } | null>(null);
  const [ledgerActionDialogState, setLedgerActionDialogState] = useState<LedgerActionDialogState | null>(null);
  const [ledgerActionSubmitting, setLedgerActionSubmitting] = useState(false);
  const [selectedLedgerFileIds, setSelectedLedgerFileIds] = useState<string[]>([]);
  const [ledgerView, setLedgerView] = useState<ImportFileLogView>("active");
  const [contextMenuAnchor, setContextMenuAnchor] = useState<{ x: number; y: number } | null>(null);
  const [contextMenuItems, setContextMenuItems] = useState<ContextMenuItem[]>([]);
  const [exportDialog, setExportDialog] = useState<ExportDialogState | null>(null);
  const [selectedRowKey, setSelectedRowKey] = useState("");
  const [selectedHistoricalDatasetId, setSelectedHistoricalDatasetId] = useState("");
  const [categoryFilter, setCategoryFilter] = useState<ImportDomainCategory>("all");
  const [dragActive, setDragActive] = useState(false);
  const [wizardStep, setWizardStep] = useState<WizardStep>("select");
  const [wizardFiles, setWizardFiles] = useState<ImportWizardFile[]>([]);
  const [wizardImportJobId, setWizardImportJobId] = useState("");
  const [previewing, setPreviewing] = useState(false);
  const [previewBatchStats, setPreviewBatchStats] = useState({ totalCount: 0, importedCount: 0 });
  const [expandedArchiveIds, setExpandedArchiveIds] = useState<string[]>([]);
  const [expandedMappingIds, setExpandedMappingIds] = useState<string[]>([]);
  const [mappingAssistById, setMappingAssistById] = useState<Record<string, MappingAssistState>>({});
  const [mappingStageAssistRunning, setMappingStageAssistRunning] = useState(false);
  const [activeMappingTargetById, setActiveMappingTargetById] = useState<Record<string, string>>({});
  const [activeArchiveChildByFileId, setActiveArchiveChildByFileId] = useState<Record<string, string>>({});
  const [validationFilter, setValidationFilter] = useState<WizardProgressFilter>("all");
  const [viewportScale, setViewportScale] = useState(1);

  const eventDedupRef = useRef<Set<string>>(new Set());
  const previousWsStatusRef = useRef(wsStatus);
  const refreshTimerRef = useRef<number | null>(null);
  const exportTrackRef = useRef<{ jobId: string; stopped: boolean; terminalNotified: string }>({
    jobId: "",
    stopped: true,
    terminalNotified: ""
  });
  const importCompletionNotifiedRef = useRef<Set<string>>(new Set());
  const validationStreamRef = useRef<HTMLDivElement | null>(null);
  const activeProgressRef = useRef("");
  const pageRef = useRef<HTMLElement | null>(null);
  const ledgerSelectAllRef = useRef<HTMLInputElement | null>(null);
  const stageSceneRef = useRef<HTMLDivElement | null>(null);
  const stageRef = useRef<HTMLElement | null>(null);
  const selectSnapAnimationFrameRef = useRef<number | null>(null);
  const selectSnapBusyRef = useRef(false);
  const selectDockEntryGuardActiveRef = useRef(false);
  const selectDockEntryGuardPendingRef = useRef(false);
  const selectDockEntryGuardLastWheelAtRef = useRef(0);
  const selectDockLockRef = useRef(false);
  const selectDockUnlockArmedRef = useRef(false);
  const mappingAssistGenerationRef = useRef(0);
  const activeRef = useRef(active);
  const activeCaseIdRef = useRef(activeCaseId);
  activeRef.current = active;
  activeCaseIdRef.current = activeCaseId;

  const closeContextMenu = (): void => {
    setContextMenuAnchor(null);
    setContextMenuItems([]);
  };

  const loadCaseName = useCallback(async (): Promise<void> => {
    if (!activeRef.current) {
      return;
    }
    if (!activeCaseId) {
      setCaseName("");
      return;
    }
    const requestedCaseId = activeCaseId;
    try {
      const detail = await getCaseDetail(requestedCaseId);
      if (!activeRef.current || activeCaseIdRef.current !== requestedCaseId) {
        return;
      }
      setCaseName(detail.case_name || "");
    } catch {
      if (!activeRef.current || activeCaseIdRef.current !== requestedCaseId) {
        return;
      }
      setCaseName("");
    }
  }, [activeCaseId]);

  const loadJobs = useCallback(async (): Promise<void> => {
    if (!activeRef.current) {
      return;
    }
    if (!activeCaseId) {
      setJobs([]);
      return;
    }
    const requestedCaseId = activeCaseId;
    const response = await listImportJobs({
      caseId: requestedCaseId,
      page: 1,
      pageSize: 200,
      status: ""
    });
    if (!activeRef.current || activeCaseIdRef.current !== requestedCaseId) {
      return;
    }
    setJobs(response.items);
  }, [activeCaseId]);

  const loadImportFiles = useCallback(async (): Promise<void> => {
    if (!activeRef.current) {
      return;
    }
    if (!activeCaseId) {
      setActiveFileLogs([]);
      setRecycleFileLogs([]);
      return;
    }
    const requestedCaseId = activeCaseId;
    const activeResponse = await listImportFiles(requestedCaseId, "active");
    if (!activeRef.current || activeCaseIdRef.current !== requestedCaseId) {
      return;
    }
    const recycleResponse = await listImportFiles(requestedCaseId, "recycle");
    if (!activeRef.current || activeCaseIdRef.current !== requestedCaseId) {
      return;
    }
    setActiveFileLogs(activeResponse.items);
    setRecycleFileLogs(recycleResponse.items);
  }, [activeCaseId]);

  const loadHistoricalDatasets = useCallback(async (): Promise<void> => {
    if (!activeRef.current) {
      return;
    }
    if (!activeCaseId) {
      setHistoricalDatasets([]);
      return;
    }
    const requestedCaseId = activeCaseId;
    const response = await listHistoricalImportDatasets(requestedCaseId);
    if (!activeRef.current || activeCaseIdRef.current !== requestedCaseId) {
      return;
    }
    setHistoricalDatasets(response.items);
  }, [activeCaseId]);

  const loadPageData = useCallback(async (): Promise<void> => {
    if (!activeRef.current) {
      return;
    }
    if (!activeCaseId) {
      setJobs([]);
      setActiveFileLogs([]);
      setRecycleFileLogs([]);
      setHistoricalDatasets([]);
      return;
    }

    setLoading(true);
    setError("");
    try {
      await loadJobs();
      await loadImportFiles();
      await loadHistoricalDatasets();
      await loadCaseName();
    } catch (loadError) {
      setError(toErrorMessage(loadError));
    } finally {
      if (activeRef.current) {
        setLoading(false);
      }
    }
  }, [activeCaseId, loadCaseName, loadHistoricalDatasets, loadImportFiles, loadJobs]);

  const scheduleRefresh = useCallback(() => {
    if (!active) {
      return;
    }
    if (refreshTimerRef.current !== null) {
      return;
    }
    refreshTimerRef.current = window.setTimeout(() => {
      refreshTimerRef.current = null;
      if (!activeRef.current) {
        return;
      }
      void loadPageData();
    }, 220);
  }, [active, loadPageData]);

  const notifyImportCompletion = useCallback((job: ImportJobDTO, completedAt?: string): void => {
    const jobId = String(job.job_id || "").trim();
    if (!jobId || job.status !== "succeeded" || importCompletionNotifiedRef.current.has(jobId)) {
      return;
    }
    importCompletionNotifiedRef.current.add(jobId);

    const duration = formatCompactDurationMs(getDateRangeDurationMs(job.created_at, completedAt || job.updated_at));
    emitToast({
      tone: "success",
      title: importCompletionTitle(job.imported_files, duration),
      durationMs: 4200,
      dedupeKey: `import-completed:${jobId}`
    });
  }, []);

  const notifyImportCompletionByJobId = useCallback(
    async (jobId: string, completedAt?: string): Promise<void> => {
      const normalizedJobId = String(jobId || "").trim();
      if (!normalizedJobId || importCompletionNotifiedRef.current.has(normalizedJobId)) {
        return;
      }
      try {
        const current = await getImportJob(activeCaseId, normalizedJobId);
        notifyImportCompletion(current, completedAt);
      } catch {
        // Completion toasts are best-effort; the next refresh will still update the task table.
      }
    },
    [activeCaseId, notifyImportCompletion]
  );

  const notifyExportTerminal = useCallback(
    (jobId: string, status: ExportDialogState["status"], outputPath: string, errorText: string): void => {
      if (exportTrackRef.current.jobId !== jobId) {
        return;
      }
      if (exportTrackRef.current.terminalNotified === status) {
        return;
      }
      exportTrackRef.current.terminalNotified = status;

      if (status === "done") {
        if (outputPath) {
          setHint(`导出完成：${outputPath}`);
          showToast({
            tone: "success",
            title: "导出已完成",
            detail: compactToastPath(outputPath)
          });
        } else {
          setHint("导出完成。");
          showToast({
            tone: "success",
            title: "导出已完成",
            detail: "结果文件已写入导出目录。"
          });
        }
        return;
      }
      if (status === "failed") {
        setError(errorText || "导出失败。");
        showToast({
          tone: "error",
          title: "导出执行失败",
          detail: toToastErrorDetail(errorText || "请稍后重试。")
        });
        return;
      }
      if (status === "canceled") {
        setHint("导出已取消。");
        showToast({
          tone: "info",
          title: "导出已停止",
          detail: "本次导出流程已终止，未继续写入新的结果文件。"
        });
      }
    },
    []
  );

  const syncExportJobSnapshot = useCallback(
    async (jobId: string, options: { notifyTerminal?: boolean; silentNetworkError?: boolean } = {}): Promise<void> => {
      if (!jobId || exportTrackRef.current.stopped) {
        return;
      }
      try {
        const current = await getExportJob(jobId, activeCaseId);
        const status = String(current.status || "");
        const progress = toInt(current.progress);
        const outputPath = String(current.output_path || "");
        const errorText = String(current.error || "");
        const nextStatus: ExportDialogState["status"] =
          status === "succeeded" ? "done" : status === "failed" ? "failed" : status === "canceled" ? "canceled" : "running";

        setExportDialog((prev) => {
          if (!prev || prev.jobId !== jobId) {
            return prev;
          }
          return {
            ...prev,
            status: nextStatus,
            progress,
            outputPath,
            error: errorText,
            stage:
              nextStatus === "done"
                ? "completed"
                : nextStatus === "failed"
                  ? "failed"
                  : nextStatus === "canceled"
                    ? "canceled"
                    : prev.stage || "running",
            message:
              nextStatus === "done"
                ? "导出完成"
                : nextStatus === "failed"
                  ? "导出失败"
                  : nextStatus === "canceled"
                    ? "导出已取消"
                    : prev.message || "导出中...",
            tableStatus:
              nextStatus === "done"
                ? "导出完成"
                : nextStatus === "failed"
                  ? "导出失败"
                  : nextStatus === "canceled"
                    ? "导出已取消"
                    : prev.tableStatus || "导出中..."
          };
        });

        if (options.notifyTerminal && (nextStatus === "done" || nextStatus === "failed" || nextStatus === "canceled")) {
          notifyExportTerminal(jobId, nextStatus, outputPath, errorText);
        }
      } catch (trackError) {
        if (options.silentNetworkError) {
          return;
        }
        const message = toErrorMessage(trackError);
        setError(message);
        showToast({
          tone: "error",
          title: "导出状态同步失败",
          detail: toToastErrorDetail(message)
        });
      }
    },
    [activeCaseId, notifyExportTerminal]
  );

  useLayoutEffect(() => {
    if (!active) {
      return;
    }
    const normalizedCaseId = String(activeCaseId || "").trim();
    // A case switch revokes the previous case projection immediately. Keep no
    // stale rows visible while the new case performs its own reads.
    setCaseName("");
    setJobs([]);
    setActiveFileLogs([]);
    setRecycleFileLogs([]);
    setHistoricalDatasets([]);
    void loadPageData();
    eventDedupRef.current.clear();
    setHydratedImportCaseId(normalizedCaseId);
    setSortField(readImportPageStateValue(normalizedCaseId, "sortField", "created", IMPORT_SORT_FIELD_VALUES));
    setSortDirection(readImportPageStateValue(normalizedCaseId, "sortDirection", "desc", IMPORT_SORT_DIRECTION_VALUES));
    setStatusFilter(readImportPageStateValue(normalizedCaseId, "statusFilter", "all", IMPORT_STATUS_FILTER_VALUES));
    setKindFilter(readImportPageStateValue(normalizedCaseId, "kindFilter", "all", IMPORT_KIND_FILTER_VALUES));
    setSearchText(readImportPageStateValue(normalizedCaseId, "searchText", ""));
    setCategoryFilter(readImportPageStateValue(normalizedCaseId, "categoryFilter", "all", IMPORT_CATEGORY_FILTER_VALUES));
    setValidationFilter(readImportPageStateValue(normalizedCaseId, "validationFilter", "all", IMPORT_VALIDATION_FILTER_VALUES));
    setLedgerView(readImportPageStateValue(normalizedCaseId, "ledgerView", "active", IMPORT_LEDGER_VIEW_VALUES));
    setSelectedRowKey("");
    setSelectedHistoricalDatasetId("");
    setWizardStep("select");
    setWizardFiles([]);
    setWizardImportJobId("");
    setExpandedArchiveIds([]);
    setExpandedMappingIds([]);
    setPreviewing(false);
    setPreviewBatchStats({ totalCount: 0, importedCount: 0 });
    setHint("");
    setError("");
    setSelectedLedgerFileIds([]);
    setLedgerActionDialogState(null);
    activeProgressRef.current = "";
    importCompletionNotifiedRef.current.clear();
  }, [active, activeCaseId, loadPageData]);

  useEffect(() => {
    const normalizedCaseId = String(activeCaseId || "").trim();
    if (!active || !normalizedCaseId || hydratedImportCaseId !== normalizedCaseId) {
      return;
    }
    writeImportPageStateValue(normalizedCaseId, "sortField", sortField);
    writeImportPageStateValue(normalizedCaseId, "sortDirection", sortDirection);
    writeImportPageStateValue(normalizedCaseId, "statusFilter", statusFilter);
    writeImportPageStateValue(normalizedCaseId, "kindFilter", kindFilter);
    writeImportPageStateValue(normalizedCaseId, "searchText", searchText);
    writeImportPageStateValue(normalizedCaseId, "ledgerView", ledgerView);
    writeImportPageStateValue(normalizedCaseId, "categoryFilter", categoryFilter);
    writeImportPageStateValue(normalizedCaseId, "validationFilter", validationFilter);
  }, [
    active,
    activeCaseId,
    categoryFilter,
    hydratedImportCaseId,
    kindFilter,
    ledgerView,
    searchText,
    sortDirection,
    sortField,
    statusFilter,
    validationFilter,
  ]);

  useEffect(() => {
    return () => {
      if (refreshTimerRef.current !== null) {
        window.clearTimeout(refreshTimerRef.current);
        refreshTimerRef.current = null;
      }
      exportTrackRef.current.stopped = true;
      exportTrackRef.current.jobId = "";
      exportTrackRef.current.terminalNotified = "";
    };
  }, []);

  useLayoutEffect(() => {
    const pageNode = pageRef.current;
    const scrollHost = resolveImportScrollHost(pageNode);
    if (!(scrollHost instanceof HTMLElement)) {
      return;
    }
    if (active) {
      scrollHost.classList.add("import-scrollbar-hidden");
    } else {
      scrollHost.classList.remove("import-scrollbar-hidden");
    }
    return () => {
      scrollHost.classList.remove("import-scrollbar-hidden");
    };
  }, [active]);

  useLayoutEffect(() => {
    const pageNode = pageRef.current;
    const scrollHost = resolveImportScrollHost(pageNode);
    if (!(pageNode instanceof HTMLElement) || !(scrollHost instanceof HTMLElement)) {
      return;
    }

    let rafId = 0;

    const syncViewportLayout = (): void => {
      rafId = 0;
      const roundedScale = measureImportViewportScale(pageNode, scrollHost);
      const scrollHostRect = scrollHost.getBoundingClientRect();
      const anchorRect = stageRef.current?.getBoundingClientRect();
      const anchorLeft = anchorRect && anchorRect.width > 0 ? anchorRect.left : scrollHostRect.left;
      const anchorRight = anchorRect && anchorRect.width > 0 ? anchorRect.right : scrollHostRect.right;
      const visibleLeft = clampNumber(anchorLeft, scrollHostRect.left, scrollHostRect.right);
      const visibleRight = clampNumber(anchorRight, visibleLeft, scrollHostRect.right);
      pageNode.style.setProperty("--import-flow-nav-fixed-left", `${Math.max(0, Math.round(visibleLeft))}px`);
      pageNode.style.setProperty("--import-flow-nav-fixed-right", `${Math.max(0, Math.round(window.innerWidth - visibleRight))}px`);
      pageNode.style.setProperty("--import-flow-nav-fixed-bottom", `${Math.max(0, Math.round(window.innerHeight - scrollHostRect.bottom))}px`);
      setViewportScale((previous) => (Math.abs(previous - roundedScale) > 0.001 ? roundedScale : previous));
    };

    const requestSync = (): void => {
      if (rafId !== 0) {
        return;
      }
      rafId = window.requestAnimationFrame(syncViewportLayout);
    };

    const observer = typeof ResizeObserver !== "undefined" ? new ResizeObserver(requestSync) : null;
    observer?.observe(scrollHost);
    observer?.observe(pageNode);
    if (stageRef.current) {
      observer?.observe(stageRef.current);
    }
    syncViewportLayout();
    requestSync();
    window.addEventListener("resize", requestSync);

    return () => {
      if (rafId !== 0) {
        window.cancelAnimationFrame(rafId);
      }
      observer?.disconnect();
      window.removeEventListener("resize", requestSync);
      pageNode.style.removeProperty("--import-flow-nav-fixed-left");
      pageNode.style.removeProperty("--import-flow-nav-fixed-right");
      pageNode.style.removeProperty("--import-flow-nav-fixed-bottom");
    };
  }, [active, previewing, wizardFiles.length, wizardStep]);

  useEffect(() => {
    if (wizardFiles.length === 0 && wizardStep !== "summary") {
      setWizardStep("select");
      setWizardImportJobId("");
    }
  }, [wizardFiles.length, wizardStep]);

  useEffect(() => {
    const previous = previousWsStatusRef.current;
    if (active && activeCaseId && wsStatus === "open" && previous !== "open") {
      void loadPageData();
    }
    previousWsStatusRef.current = wsStatus;
  }, [active, activeCaseId, loadPageData, wsStatus]);

  useEffect(() => {
    if (!active) {
      return;
    }
    if (!lastWsEvent || !activeCaseId) {
      return;
    }

    if (!eventBelongsToCase(lastWsEvent, activeCaseId)) {
      return;
    }

    const payloadObj = (lastWsEvent.payload || {}) as Record<string, unknown>;
    const eventId = String(payloadObj.event_id ?? `${lastWsEvent.sequence}:${lastWsEvent.event}:${lastWsEvent.job_id || ""}`);
    if (eventDedupRef.current.has(eventId)) {
      return;
    }
    eventDedupRef.current.add(eventId);
    if (eventDedupRef.current.size > 1200) {
      eventDedupRef.current.clear();
      eventDedupRef.current.add(eventId);
    }

    if (isImportEvent(lastWsEvent)) {
      const payload = asJobProgressPayload(lastWsEvent.payload);
      const jobId = (lastWsEvent.job_id ?? "").trim();
      if (jobId) {
        setJobs((prev) => {
          const index = prev.findIndex((item) => item.job_id === jobId);
          if (index < 0) {
            return prev;
          }
          const target = prev[index];
          const next = [...prev];
          next[index] = {
            ...target,
            status: mapEventToStatus(lastWsEvent.event, target.status, payloadObj),
            progress: typeof payload.progress === "number" ? payload.progress : target.progress,
            updated_at: lastWsEvent.timestamp
          };
          return next;
        });
      }
      scheduleRefresh();
      if (lastWsEvent.event === "import.job.completed") {
        void notifyImportCompletionByJobId(jobId, lastWsEvent.timestamp);
      }
      return;
    }

    if (isExportEvent(lastWsEvent)) {
      const jobId = String(lastWsEvent.job_id || "").trim();
      if (!jobId) {
        return;
      }
      const payload = asExportProgressPayload(lastWsEvent.payload);
      const counters = (payload.counters || {}) as Record<string, unknown>;
      const tableKey = String(counters.table || "");
      const tableLabel = EXPORT_TABLE_LABELS[tableKey] || tableKey || "";
      const tableTotal = toInt(counters.rows_total);
      const tableDone = toInt(counters.rows_exported);
      const payloadMessage = String(payload.message || "");
      const payloadStage = String(payload.stage || "");
      const payloadCode = String(payload.code || "");
      const payloadOutputPath = String(payload.output_path || payload.path || "");

      setExportDialog((prev) => {
        if (!prev || prev.jobId !== jobId) {
          return prev;
        }

        if (lastWsEvent.event === "export.job.progress") {
          return {
            ...prev,
            status: "running",
            progress: typeof payload.progress === "number" ? payload.progress : prev.progress,
            stage: payloadStage || prev.stage || "running",
            message: payloadMessage || prev.message || "导出中",
            tableKey: tableKey || prev.tableKey,
            tableLabel: tableLabel || prev.tableLabel,
            tableTotal: tableKey ? tableTotal : prev.tableTotal,
            tableDone: tableKey ? tableDone : prev.tableDone,
            tableStatus: payloadMessage || prev.tableStatus || "导出中..."
          };
        }

        if (lastWsEvent.event === "export.job.completed") {
          return {
            ...prev,
            status: "done",
            progress: 100,
            stage: "completed",
            message: "导出完成",
            outputPath: payloadOutputPath || prev.outputPath,
            tableStatus: payloadMessage || prev.tableStatus || "导出完成"
          };
        }

        if (lastWsEvent.event === "export.job.failed") {
          const canceled = payloadCode === "JOB_CANCELED";
          return {
            ...prev,
            status: canceled ? "canceled" : "failed",
            stage: "failed",
            message: canceled ? "导出已取消" : "导出失败",
            error: canceled ? prev.error : payloadMessage || prev.error,
            tableStatus: payloadMessage || prev.tableStatus || "导出失败"
          };
        }

        return prev;
      });

      if (lastWsEvent.event === "export.job.completed") {
        if (payloadOutputPath) {
          notifyExportTerminal(jobId, "done", payloadOutputPath, "");
        } else {
          void syncExportJobSnapshot(jobId, { notifyTerminal: true, silentNetworkError: true });
        }
        return;
      }
      if (lastWsEvent.event === "export.job.failed") {
        if (payloadCode === "JOB_CANCELED") {
          notifyExportTerminal(jobId, "canceled", "", "");
        } else if (payloadMessage) {
          notifyExportTerminal(jobId, "failed", "", payloadMessage);
        } else {
          void syncExportJobSnapshot(jobId, { notifyTerminal: true, silentNetworkError: true });
        }
      }
    }
  }, [active, activeCaseId, lastWsEvent, notifyExportTerminal, notifyImportCompletionByJobId, scheduleRefresh, syncExportJobSnapshot]);

  const createImportTask = useCallback(
    async (payloadFiles: ImportFileSpec[]): Promise<ImportJobDTO | null> => {
      if (!activeCaseId) {
        setError("请先在【案件管理】打开一个案件。");
        showToast({
          tone: "info",
          title: "先打开案件",
          detail: "创建导入任务前，请先在案件页打开一个案件。"
        });
        return null;
      }
      if (payloadFiles.length === 0) {
        setError("请选择至少一个有效文件。");
        showToast({
          tone: "info",
          title: "先选择有效文件",
          detail: "至少选择一个可识别的本地文件后，系统才会创建导入任务。"
        });
        return null;
      }

      setSubmitting(true);
      setError("");
      setHint("");

      try {
        const created = await createImportJob({
          case_id: activeCaseId,
          files: payloadFiles,
          auto_cleaning: false
        });
        setHint(`导入任务已创建：${created.job_id}`);
        setJobs((previous) => (previous.some((item) => item.job_id === created.job_id) ? previous : [created, ...previous]));
        void loadJobs();
        scheduleRefresh();
        return created;
      } catch (submitError) {
        const message = toErrorMessage(submitError);
        setError(message);
        showToast({
          tone: "error",
          title: "导入任务创建失败",
          detail: toToastErrorDetail(message)
        });
        return null;
      } finally {
        setSubmitting(false);
      }
    },
    [activeCaseId, loadJobs, scheduleRefresh]
  );

  const resetWizardWorkspace = useCallback((): void => {
    setWizardStep("select");
    setWizardFiles([]);
    setWizardImportJobId("");
    setExpandedArchiveIds([]);
    setExpandedMappingIds([]);
    setMappingAssistById({});
    setActiveMappingTargetById({});
    setActiveArchiveChildByFileId({});
    setValidationFilter("all");
    setPreviewing(false);
    setPreviewBatchStats({ totalCount: 0, importedCount: 0 });
    setHint("");
    setError("");
    activeProgressRef.current = "";
  }, []);

  const appendWizardFiles = useCallback((previews: ImportPreviewFileDTO[]): void => {
    setWizardFiles((previous) => {
      const next = new Map(previous.map((item) => [item.sourcePath, item]));
      for (const preview of previews) {
        const existing = next.get(preview.source_path);
        const draft = buildWizardFile(preview, existing);
        next.set(draft.sourcePath, draft);
      }
      return Array.from(next.values());
    });
  }, []);

  const previewSelectedFiles = useCallback(
    async (paths: string[]): Promise<void> => {
      const sourceBlockReason = controlledSourceIngestionBlockReason();
      if (sourceBlockReason) {
        setError(sourceBlockReason);
        showToast({ tone: "warning", title: "本地文件采集已阻止", detail: sourceBlockReason });
        return;
      }
      if (!activeCaseId) {
        setError("请先在【案件管理】打开一个案件。");
        showToast({
          tone: "info",
          title: "先打开案件",
          detail: "预检文件前，请先在案件页打开一个案件。"
        });
        return;
      }
      const uniquePaths = Array.from(new Set(paths.map((filePath) => filePath.trim()).filter(Boolean)));
      if (uniquePaths.length === 0) {
        return;
      }
      const existingPaths = new Set(wizardFiles.map((file) => file.sourcePath));
      const pendingCount = uniquePaths.filter((filePath) => !existingPaths.has(filePath)).length;
      setPreviewBatchStats({
        totalCount: wizardFiles.length + pendingCount,
        importedCount: wizardFiles.length
      });
      setPreviewing(true);
      setError("");
      setHint("");
      try {
        if (desktopAvailable) {
          const backendReady = await ensureDesktopBackendRuntime();
          if (!backendReady) {
            throw new Error("数据分析后端服务未就绪，请稍后重试。");
          }
        }
        const payloadFiles: ImportFileSpec[] = uniquePaths.map((filePath) => ({
          file_name: fileNameFromPath(filePath),
          source_path: filePath
        }));
        const response = await previewImportFiles(activeCaseId, payloadFiles);
        appendWizardFiles(response.items);
        setExpandedArchiveIds((previous) => {
          const next = new Set(previous);
          for (const item of response.items) {
            if (Array.isArray(item.archive_children) && item.archive_children.length > 0) {
              next.add(`${item.source_path}:${item.file_name}`);
            }
          }
          return Array.from(next);
        });
        setWizardStep("select");
        setHint("文件预检与 SHA-256 校验已完成，可继续确认字段与类型。");
        showToast({
          tone: "success",
          title: "文件预检已完成",
          detail: `${response.items.length.toLocaleString("zh-CN")} 个文件已完成校验，可继续确认字段与类型。`
        });
      } catch (previewError) {
        const message = toErrorMessage(previewError);
        setError(message);
        showToast({
          tone: "error",
          title: "文件预检失败",
          detail: toToastErrorDetail(message)
        });
      } finally {
        setPreviewing(false);
        setPreviewBatchStats({ totalCount: 0, importedCount: 0 });
      }
    },
    [activeCaseId, appendWizardFiles, desktopAvailable, wizardFiles]
  );

  const onQuickImport = async (): Promise<void> => {
    const sourceBlockReason = controlledSourceIngestionBlockReason();
    if (sourceBlockReason) {
      setError(sourceBlockReason);
      showToast({ tone: "warning", title: "本地文件采集已阻止", detail: sourceBlockReason });
      return;
    }
    if (!activeCaseId) {
      setError("请先在【案件管理】打开一个案件。");
      showToast({
        tone: "info",
        title: "先打开案件",
        detail: "开始导入前，请先在案件页打开一个案件。"
      });
      return;
    }

    const selected = await pickFiles();
    if (selected.length === 0) {
      if (!desktopAvailable) {
        setHint("当前环境暂不支持系统文件选择器，请在桌面版中导入。");
        showToast({
          tone: "info",
          title: "当前环境不支持文件选择器",
          detail: "请在桌面版选择本地文件后，再继续导入。"
        });
      }
      return;
    }

    await previewSelectedFiles(selected);
  };

  const onDropZoneClick = (event: MouseEvent<HTMLElement>): void => {
    if (wizardFiles.length > 0 || !desktopAvailable || previewing) {
      return;
    }

    const target = event.target;
    if (target instanceof HTMLElement && target.closest("button, a, input, select, textarea, summary")) {
      return;
    }

    void onQuickImport();
  };

  const onDropZoneKeyDown = (event: ReactKeyboardEvent<HTMLElement>): void => {
    if (wizardFiles.length > 0 || !desktopAvailable || previewing) {
      return;
    }

    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }

    event.preventDefault();
    void onQuickImport();
  };

  const onDropZoneDragOver = (event: ReactDragEvent<HTMLElement>): void => {
    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
    if (!dragActive) {
      setDragActive(true);
    }
  };

  const onDropZoneDragLeave = (event: ReactDragEvent<HTMLElement>): void => {
    event.preventDefault();
    const relatedTarget = event.relatedTarget;
    if (relatedTarget instanceof Node && event.currentTarget.contains(relatedTarget)) {
      return;
    }
    setDragActive(false);
  };

  const onDropZoneDrop = (event: ReactDragEvent<HTMLElement>): void => {
    event.preventDefault();
    setDragActive(false);
    const sourceBlockReason = controlledSourceIngestionBlockReason();
    setError(sourceBlockReason);
    showToast({ tone: "warning", title: "本地文件采集已阻止", detail: sourceBlockReason });
  };
  const onOpenDbDir = async (): Promise<void> => {
    const reason = controlledSourceIngestionBlockReason();
    setError(reason);
    showToast({ tone: "warning", title: "受控目录访问已阻止", detail: reason });
  };

  const onCopyText = async (value: string, label: string): Promise<void> => {
    const rawContent = String(value ?? "").trim();
    const content = projectOrdinaryFieldValue(label, value).trim();
    const isRestrictedProjection = content !== rawContent;
    if (!content) {
      setHint(`${label} 为空，无法复制。`);
      showToast({
        tone: "info",
        title: "当前没有可复制内容",
        detail: `${label} 为空，请先生成内容后再复制。`
      });
      return;
    }
    try {
      await navigator.clipboard.writeText(content);
      setHint(`${label}${isRestrictedProjection ? " 脱敏副本" : ""}已复制到剪贴板。`);
      showToast({
        tone: "success",
        title: `${label}${isRestrictedProjection ? " 脱敏副本" : ""}已复制`,
        detail: isRestrictedProjection
          ? "普通复制仅写入脱敏副本；完整敏感信息需使用受控 artifact。"
          : "内容已写入剪贴板，可直接粘贴使用。"
      });
    } catch {
      setError(`${label} 复制失败，请检查浏览器剪贴板权限。`);
      showToast({
        tone: "error",
        title: `${label} 复制失败`,
        detail: "请检查当前环境的剪贴板权限后重试。"
      });
    }
  };

  const onOpenRowFolder = async (_row: LedgerRow): Promise<void> => {
    const reason = controlledSourceIngestionBlockReason();
    setError(reason);
    showToast({ tone: "warning", title: "受控来源访问已阻止", detail: reason });
  };

  const canApplyLedgerAction = useCallback((row: LedgerRow | null | undefined, mode: LedgerActionMode): boolean => {
    if (!row?.file_id) {
      return false;
    }
    if (mode === "recycle") {
      return !isActiveStatusText(row.status_text);
    }
    if (mode === "purge") {
      return completeImpactEstimate([row.valid_rows]).status === "complete";
    }
    return true;
  }, []);

  const openLedgerActionDialog = useCallback((mode: LedgerActionMode, rows: LedgerRow[]): void => {
    const normalized = rows.filter((row) => Boolean(row?.file_id));
    if (normalized.length === 0) {
      const actionLabel = mode === "recycle" ? "移入回收站" : mode === "restore" ? "恢复" : "彻底删除";
      setHint(`当前记录不支持${actionLabel}。`);
      showToast({
        tone: "info",
        title: "当前记录不可执行该操作",
        detail: `所选记录暂不支持“${actionLabel}”。`
      });
      return;
    }
    const invalidRows = normalized.filter((row) => !canApplyLedgerAction(row, mode));
    if (invalidRows.length > 0) {
      const message = mode === "recycle"
        ? "导入中或等待中的记录不允许移入回收站。"
        : mode === "purge"
          ? "影响行数未完整核验，禁止彻底删除。"
          : "当前记录不支持该操作。";
      setError(message);
      showToast({
        tone: "warning",
        title: mode === "recycle" ? "当前记录暂不可回收" : mode === "purge" ? "彻底删除已阻止" : "当前记录待复核",
        detail: mode === "recycle"
          ? "导入中或等待中的记录，不允许直接移入回收站。"
          : mode === "purge"
            ? "至少一个文件未提供完整有效行计数；补齐并核验影响范围后才能永久清除。"
            : "请先确认记录状态，再执行当前操作。"
      });
      return;
    }
    setLedgerActionDialogState({
      mode,
      rows: Array.from(
        new Map(normalized.map((row) => [row.file_id, row])).values()
      )
    });
  }, [canApplyLedgerAction]);

  const closeLedgerActionDialog = useCallback((): void => {
    if (ledgerActionSubmitting) {
      return;
    }
    setLedgerActionDialogState(null);
  }, [ledgerActionSubmitting]);

  const onConfirmLedgerAction = useCallback(async (): Promise<void> => {
    if (!ledgerActionDialogState || !activeCaseId) {
      setLedgerActionDialogState(null);
      return;
    }

    const { mode } = ledgerActionDialogState;
    const targets = ledgerActionDialogState.rows.filter((row) => Boolean(row.file_id));
    if (targets.length === 0) {
      setLedgerActionDialogState(null);
      return;
    }
    if (targets.some((row) => !canApplyLedgerAction(row, mode))) {
      const message = mode === "recycle"
        ? "导入中或等待中的记录不允许移入回收站。"
        : mode === "purge"
          ? "影响行数未完整核验，禁止彻底删除。"
          : "当前记录不支持该操作。";
      setError(message);
      showToast({
        tone: "warning",
        title: mode === "recycle" ? "当前记录暂不可回收" : mode === "purge" ? "彻底删除已阻止" : "当前记录待复核",
        detail: mode === "recycle"
          ? "导入中或等待中的记录，不允许直接移入回收站。"
          : mode === "purge"
            ? "至少一个文件未提供完整有效行计数；补齐并核验影响范围后才能永久清除。"
            : "请先确认记录状态，再执行当前操作。"
      });
      return;
    }

    setLedgerActionSubmitting(true);
    try {
      const fileIds = targets.map((row) => row.file_id);
      const result =
        mode === "recycle"
          ? await recycleImportFiles(activeCaseId, fileIds)
          : mode === "restore"
            ? await restoreImportFiles(activeCaseId, fileIds)
            : await purgeImportFiles(activeCaseId, fileIds);
      const affectedIds = new Set(result.file_ids);
      const affectedCount = result.affected_count;
      if (affectedCount === 0) {
        setHint("服务端确认本次操作未改变任何导入记录；未将空结果升级为成功数量。");
        showToast({
          tone: "info",
          title: "没有记录发生变化",
          detail: "已核验返回范围为空；请刷新台账后确认记录的当前状态。"
        });
        setLedgerActionDialogState(null);
        await loadPageData();
        return;
      }
      setSelectedLedgerFileIds((previous) => previous.filter((fileId) => !affectedIds.has(fileId)));
      if (mode === "recycle") {
        setHint(`已移入回收站 ${affectedCount.toLocaleString("zh-CN")} 条导入记录，可在回收站中恢复。`);
        showToast({
          tone: "success",
          title: "记录已移入回收站",
          detail: `已转入 ${affectedCount.toLocaleString("zh-CN")} 条台账记录，并同步写入案件审计。`
        });
      } else if (mode === "restore") {
        setHint(`已恢复 ${affectedCount.toLocaleString("zh-CN")} 条导入记录。`);
        showToast({
          tone: "success",
          title: "导入记录已恢复",
          detail: `已恢复 ${affectedCount.toLocaleString("zh-CN")} 条回收站记录，并同步恢复关联数据。`
        });
      } else {
        setHint(`已彻底删除 ${affectedCount.toLocaleString("zh-CN")} 条回收站记录及其关联数据。`);
        showToast({
          tone: "success",
          title: "回收站记录已清除",
          detail: `已永久清理 ${affectedCount.toLocaleString("zh-CN")} 条记录，并同步写入案件审计。`
        });
      }
      setLedgerActionDialogState(null);
      await loadPageData();
    } catch (actionError) {
      const message = toErrorMessage(actionError);
      setError(message);
      showToast({
        tone: "error",
        title: mode === "recycle" ? "移入回收站失败" : mode === "restore" ? "导入记录恢复失败" : "回收站清除失败",
        detail: toToastErrorDetail(message)
      });
    } finally {
      setLedgerActionSubmitting(false);
    }
  }, [activeCaseId, canApplyLedgerAction, ledgerActionDialogState, loadPageData]);

  const stopExportTracking = (): void => {
    exportTrackRef.current.stopped = true;
    exportTrackRef.current.jobId = "";
    exportTrackRef.current.terminalNotified = "";
  };

  const startExportTracking = (jobId: string): void => {
    exportTrackRef.current = { jobId, stopped: false, terminalNotified: "" };
  };

  const onCloseExportDialog = (): void => {
    stopExportTracking();
    setExportDialog(null);
  };

  const onCancelExport = async (): Promise<void> => {
    if (!exportDialog?.jobId) {
      return;
    }
    try {
      await cancelExportJob(exportDialog.jobId, activeCaseId);
      setExportDialog((prev) =>
        prev
          ? {
              ...prev,
              status: "canceled",
              message: "已请求停止导出任务。",
              tableStatus: "正在停止导出..."
            }
          : prev
      );
      setHint(`已请求取消导出任务：${exportDialog.jobId}`);
      showToast({
        tone: "info",
        title: "已接收取消指令",
        detail: `导出任务 ${exportDialog.jobId} 正在停止，状态更新后会自动同步。`
      });
    } catch (cancelError) {
      const message = toErrorMessage(cancelError);
      setError(message);
      showToast({
        tone: "error",
        title: "导出任务取消失败",
        detail: toToastErrorDetail(message)
      });
    }
  };

  useEffect(() => {
    const jobId = String(exportDialog?.jobId || "").trim();
    if (!jobId || exportTrackRef.current.stopped) {
      return;
    }
    const status = exportDialog?.status;
    if (status === "done" || status === "failed" || status === "canceled") {
      return;
    }

    const pollMs = wsStatus === "open" ? EXPORT_TRACK_POLL_WS_FALLBACK_MS : EXPORT_TRACK_POLL_OFFLINE_MS;
    const silentNetworkError = wsStatus === "open";
    const tick = (): void => {
      void syncExportJobSnapshot(jobId, { notifyTerminal: true, silentNetworkError });
    };
    const initialDelayMs = wsStatus === "open" ? 1800 : 0;

    const initialTimer = window.setTimeout(() => {
      tick();
    }, initialDelayMs);
    const intervalTimer = window.setInterval(() => {
      tick();
    }, pollMs);

    return () => {
      window.clearTimeout(initialTimer);
      window.clearInterval(intervalTimer);
    };
  }, [exportDialog?.jobId, exportDialog?.status, syncExportJobSnapshot, wsStatus]);

  const onExportRaw = async (): Promise<void> => {
    const publicationBlockReason = controlledArtifactPublicationBlockReason();
    if (publicationBlockReason) {
      setError(publicationBlockReason);
      showToast({ tone: "warning", title: "受控导出已阻止", detail: publicationBlockReason });
      return;
    }
    const exportBlockReason = uncontrolledRestrictedPiiExportBlockReason();
    if (exportBlockReason) {
      showToast({
        tone: "warning",
        title: "完整导出已阻止",
        detail: exportBlockReason,
      });
      return;
    }
    if (!activeCaseId) {
      setError("请先在【案件管理】打开一个案件。");
      showToast({
        tone: "info",
        title: "先打开案件",
        detail: "导出导入台账前，请先在案件页打开一个案件。"
      });
      return;
    }

    setError("");
    setHint("");
    stopExportTracking();

    try {
      const job = await createRawExportJob({
        case_id: activeCaseId,
        export_format: "xlsx",
        filters: {},
        output_name: `${caseName || activeCaseId}-原始导出`,
        target_dir: undefined
      });

      setExportDialog({
        open: true,
        jobId: job.job_id,
        status: "running",
        progress: toInt(job.progress),
        stage: "queued",
        message: "准备导出...",
        tableKey: "",
        tableLabel: "等待导出",
        tableTotal: 0,
        tableDone: 0,
        tableStatus: "准备开始...",
        outputPath: "",
        error: ""
      });
      startExportTracking(job.job_id);
      setHint(`已创建导出任务：${job.job_id}`);
      showToast({
        tone: "running",
        title: "导出任务已入列",
        detail: `任务 ${job.job_id} 已进入后台队列，可继续处理当前页面。`
      });
    } catch (exportError) {
      const message = toErrorMessage(exportError);
      setError(message);
      showToast({
        tone: "error",
        title: "导出任务创建失败",
        detail: toToastErrorDetail(message)
      });
    }
  };

  const onOpenExportOutput = async (): Promise<void> => {
    const reason = controlledArtifactPublicationBlockReason();
    setError(reason);
    showToast({ tone: "warning", title: "受控 artifact 访问已阻止", detail: reason });
  };

  const removeWizardFile = useCallback((fileId: string): void => {
    setWizardFiles((previous) => previous.filter((item) => item.id !== fileId));
    setExpandedArchiveIds((previous) => previous.filter((item) => item !== fileId));
    setExpandedMappingIds((previous) => previous.filter((item) => item !== fileId));
    setActiveArchiveChildByFileId((previous) => Object.fromEntries(Object.entries(previous).filter(([key]) => key !== fileId)));
    setMappingAssistById((previous) =>
      Object.fromEntries(Object.entries(previous).filter(([key]) => key !== fileId && !key.startsWith(`${fileId}::`)))
    );
  }, []);

  const toggleArchiveExpand = useCallback((fileId: string): void => {
    setExpandedArchiveIds((previous) =>
      previous.includes(fileId) ? previous.filter((item) => item !== fileId) : [...previous, fileId]
    );
    setActiveArchiveChildByFileId((previous) => {
      if (!previous[fileId]) {
        return previous;
      }
      return {
        ...previous,
        [fileId]: "",
      };
    });
  }, []);

  const toggleMappingExpand = useCallback((fileId: string): void => {
    setExpandedMappingIds((previous) =>
      previous.includes(fileId) ? previous.filter((item) => item !== fileId) : [...previous, fileId]
    );
    setActiveArchiveChildByFileId((previous) => {
      if (!previous[fileId]) {
        return previous;
      }
      return {
        ...previous,
        [fileId]: "",
      };
    });
  }, []);

  const updateWizardCategory = useCallback((fileId: string, category: Exclude<ImportDomainCategory, "all">): void => {
    setWizardFiles((previous) =>
      previous.map((item) => {
        if (item.id !== fileId) {
          return item;
        }
        const nextKind = item.selectedCategory === category && item.selectedKind ? item.selectedKind : defaultKindForCategory(category);
        return {
          ...item,
          selectedCategory: category,
          selectedKind: category === "support" ? "support_file" : nextKind,
          fieldMapping: category === "support" ? {} : item.fieldMapping,
          fieldMappingOrigins: category === "support" ? {} : item.fieldMappingOrigins,
          status: item.status === "unsupported" ? item.status : nextKind ? "ready" : "review"
        };
      })
    );
  }, []);

  const updateWizardKind = useCallback((fileId: string, kind: string): void => {
    setWizardFiles((previous) =>
      previous.map((item) =>
        item.id === fileId
          ? {
              ...item,
              selectedKind: kind,
              fieldMapping: {},
              fieldMappingOrigins: {},
              status: item.status === "unsupported" ? "unsupported" : kind ? "ready" : "review"
            }
          : item
      )
    );
  }, []);

  const updateWizardPassword = useCallback((fileId: string, password: string): void => {
    setWizardFiles((previous) =>
      previous.map((item) =>
        item.id === fileId
          ? {
              ...item,
              password,
              passwordVerified: item.requiresPassword ? false : item.passwordVerified,
              passwordState: item.requiresPassword ? "idle" : item.passwordState,
              passwordValidationMessage: item.requiresPassword ? "" : item.passwordValidationMessage
            }
          : item
      )
    );
  }, []);

  const updateWizardFieldMapping = useCallback((fileId: string, fieldKey: string, sourceHeader: string): void => {
    setWizardFiles((previous) =>
      previous.map((item) =>
        item.id === fileId
          ? (() => {
              const next = assignExclusiveFieldMappingWithOrigin(item.fieldMapping, item.fieldMappingOrigins, fieldKey, sourceHeader, "manual");
              return { ...item, fieldMapping: next.mapping, fieldMappingOrigins: next.origins };
            })()
          : item
      )
    );
  }, []);

  const updateWizardSourceBinding = useCallback((fileId: string, sourceHeader: string, fieldKey: string): void => {
    setWizardFiles((previous) =>
      previous.map((item) =>
        item.id === fileId
          ? (() => {
              const next = fieldKey
                ? assignExclusiveFieldMappingWithOrigin(item.fieldMapping, item.fieldMappingOrigins, fieldKey, sourceHeader, "manual")
                : clearFieldMappingBySourceWithOrigin(item.fieldMapping, item.fieldMappingOrigins, sourceHeader);
              return { ...item, fieldMapping: next.mapping, fieldMappingOrigins: next.origins };
            })()
          : item
      )
    );
  }, []);

  const updateWizardArchiveChildKind = useCallback((fileId: string, childId: string, kind: string): void => {
    setWizardFiles((previous) =>
      previous.map((item) =>
        item.id === fileId
          ? {
              ...item,
              archiveChildren: item.archiveChildren.map((child) =>
                child.id === childId ? { ...child, selectedKind: kind, fieldMapping: {}, fieldMappingOrigins: {} } : child
              )
            }
          : item
      )
    );
  }, []);

  const updateWizardArchiveChildCategory = useCallback(
    (fileId: string, childId: string, category: Exclude<ImportDomainCategory, "all">): void => {
      setWizardFiles((previous) =>
        previous.map((item) =>
          item.id === fileId
            ? {
                ...item,
                archiveChildren: item.archiveChildren.map((child) => {
                  if (child.id !== childId) {
                    return child;
                  }
                  return {
                    ...child,
                    domainCategory: category,
                    selectedKind: defaultKindForCategory(category),
                    fieldMapping: {},
                    fieldMappingOrigins: {},
                    status: child.status === "unsupported" ? child.status : "ready"
                  };
                })
              }
            : item
        )
      );
    },
    []
  );

  const updateWizardArchiveChildFieldMapping = useCallback(
    (fileId: string, childId: string, fieldKey: string, sourceHeader: string): void => {
      setWizardFiles((previous) =>
        previous.map((item) =>
          item.id === fileId
            ? {
                ...item,
                archiveChildren: item.archiveChildren.map((child) =>
                  child.id === childId
                    ? (() => {
                        const next = assignExclusiveFieldMappingWithOrigin(child.fieldMapping, child.fieldMappingOrigins, fieldKey, sourceHeader, "manual");
                        return { ...child, fieldMapping: next.mapping, fieldMappingOrigins: next.origins };
                      })()
                    : child
                )
              }
            : item
        )
      );
    },
    []
  );

  const updateWizardArchiveChildSourceBinding = useCallback((fileId: string, childId: string, sourceHeader: string, fieldKey: string): void => {
    setWizardFiles((previous) =>
      previous.map((item) =>
        item.id === fileId
          ? {
              ...item,
              archiveChildren: item.archiveChildren.map((child) =>
                child.id === childId
                  ? (() => {
                      const next = fieldKey
                        ? assignExclusiveFieldMappingWithOrigin(child.fieldMapping, child.fieldMappingOrigins, fieldKey, sourceHeader, "manual")
                        : clearFieldMappingBySourceWithOrigin(child.fieldMapping, child.fieldMappingOrigins, sourceHeader);
                      return { ...child, fieldMapping: next.mapping, fieldMappingOrigins: next.origins };
                    })()
                  : child
              )
            }
          : item
      )
    );
  }, []);

  const validateWizardPassword = useCallback(
    async (fileId: string): Promise<void> => {
      const target = wizardFiles.find((item) => item.id === fileId);
      if (!activeCaseId || !target) {
        return;
      }
      const password = String(target.password || "").trim();
      if (!password) {
        setWizardFiles((previous) =>
          previous.map((item) =>
            item.id === fileId
              ? {
                  ...item,
                  passwordState: "error",
                  passwordVerified: false,
                  passwordValidationMessage: "请输入密码后再进行验证。"
                }
              : item
          )
        );
        return;
      }

      setWizardFiles((previous) =>
        previous.map((item) =>
          item.id === fileId
            ? {
                ...item,
                passwordState: "validating",
                passwordValidationMessage: "正在验证密码…"
              }
            : item
        )
      );

      try {
        const response = await previewImportFiles(activeCaseId, [
          {
            file_name: target.fileName,
            source_path: target.sourcePath,
            file_kind: target.archiveChildren.length > 0 ? undefined : target.selectedKind || undefined,
            password
          }
        ]);
        const preview = response.items[0];
        if (!preview) {
          throw new Error("未收到密码验证结果。");
        }
        const passwordFailed = String(preview.issue || "").includes("密码");
        setWizardFiles((previous) =>
          previous.map((item) => {
            if (item.id !== fileId) {
              return item;
            }
            const merged = buildWizardFile(preview, item);
            return {
              ...merged,
              password,
              passwordVerified: !passwordFailed,
              passwordState: passwordFailed ? "error" : "verified",
              passwordValidationMessage: passwordFailed ? preview.issue || "密码验证失败，请重新输入正确密码。" : "密码验证通过，可继续下一步。"
            };
          })
        );
        if (passwordFailed) {
          showToast({
            tone: "warning",
            title: "密码校验未通过",
            detail: preview.issue || "请检查密码后重新验证。"
          });
          return;
        }
        showToast({
          tone: "success",
          title: "密码校验已通过",
          detail: target.fileName
        });
      } catch (passwordError) {
        const message = toErrorMessage(passwordError);
        setWizardFiles((previous) =>
          previous.map((item) =>
            item.id === fileId
              ? {
                  ...item,
                  passwordVerified: false,
                  passwordState: "error",
                  passwordValidationMessage: message
                }
              : item
            )
        );
        showToast({
          tone: "error",
          title: "密码校验失败",
          detail: toToastErrorDetail(message)
        });
      }
    },
    [activeCaseId, wizardFiles]
  );

  const mappingInsightsByFileId = useMemo(() => {
    const entries = wizardFiles.map((file) => [
      file.id,
      buildMappingInsight({
        selectedKind: file.selectedKind,
        suggestedKind: file.suggestedKind,
        suggestedKindLabel: file.suggestedKindLabel,
        selectedCategory: file.selectedCategory,
        columnsTotal: file.columnsTotal,
        issue: file.issue,
        headerPreview: file.headerPreview,
        fieldMappings: file.fieldMapping,
        mappingOrigins: file.fieldMappingOrigins,
        mappingStatus: file.mappingStatus,
        mappingMethod: file.mappingMethod,
        mappingMessage: file.mappingMessage,
        mappingRequiredMissing: file.mappingRequiredMissing
      })
    ] as const);
    return new Map(entries);
  }, [wizardFiles]);

  const applyMappingAssist = useCallback(
    async (fileId: string): Promise<void> => {
      const target = wizardFiles.find((item) => item.id === fileId);
      if (!activeCaseId || !target) {
        return;
      }
      const assistGeneration = mappingAssistGenerationRef.current;
      if (target.selectedCategory === "support") {
        setMappingAssistById((previous) => ({
          ...previous,
          [fileId]: { status: "error", message: "研判文件不需要结构化字段映射。" }
        }));
        return;
      }
      if (target.archiveChildren.length > 0) {
        setMappingAssistById((previous) => ({
          ...previous,
          [fileId]: { status: "error", message: "压缩包当前按子项自动识别执行，统一模板暂不支持逐项下发。" }
        }));
        return;
      }
      const effectiveKind = normalizeKind(target.selectedKind || target.suggestedKind || "");
      const blueprint = mappingBlueprintForKind(effectiveKind);
      const headers = Array.from(new Set(target.headerPreview.map((item) => String(item || "").trim()).filter(Boolean)));
      if (!effectiveKind || blueprint.length === 0) {
        setMappingAssistById((previous) => ({
          ...previous,
          [fileId]: { status: "error", message: "当前类型尚未配置字段蓝图，请先手动确认类型。" }
        }));
        return;
      }
      if (headers.length === 0) {
        setMappingAssistById((previous) => ({
          ...previous,
          [fileId]: { status: "error", message: "当前没有可用表头样本，暂时无法请求 AI 补全映射。" }
        }));
        return;
      }

      setMappingAssistById((previous) => ({
        ...previous,
        [fileId]: { status: "running", message: "正在调用本地字段映射建议器…" }
      }));
      try {
        const response = await previewImportFiles(activeCaseId, [
          {
            file_name: target.fileName,
            source_path: target.sourcePath,
            file_kind: effectiveKind,
            password: target.password || undefined
          }
        ]);
        if (mappingAssistGenerationRef.current !== assistGeneration) {
          return;
        }
        const preview = response.items[0];
        const suggestedMappings = sanitizeManualFieldMapping(preview?.field_mapping, headers);
        const suggestedOriginFallback = mappingOriginForMethod(preview?.mapping_method || "");
        const suggestedOrigins = sanitizeFieldMappingOrigins(
          preview?.field_mapping_origins,
          suggestedMappings,
          suggestedOriginFallback
        );
        const nextMapping = { ...target.fieldMapping };
        const nextMappingOrigins = sanitizeFieldMappingOrigins(target.fieldMappingOrigins, nextMapping, "auto");
        const templateMappings = suggestFieldMappingFromHeaders(effectiveKind, headers, {});
        const appliedCount =
          applySuggestedMappingEntries(nextMapping, nextMappingOrigins, templateMappings, () => "auto") +
          applySuggestedMappingEntries(nextMapping, nextMappingOrigins, suggestedMappings, (key) => suggestedOrigins[key] || suggestedOriginFallback);
        if (mappingAssistGenerationRef.current !== assistGeneration) {
          return;
        }
        setWizardFiles((previous) =>
          previous.map((item) => (item.id === fileId ? { ...item, fieldMapping: nextMapping, fieldMappingOrigins: nextMappingOrigins } : item))
        );
        setMappingAssistById((previous) => ({
          ...previous,
          [fileId]: {
            status: "done",
            message:
              appliedCount > 0
                ? `已补全/修正 ${appliedCount.toLocaleString("zh-CN")} 项高置信字段映射，可继续人工微调。`
                : String(preview?.mapping_message || "当前字段映射已与系统建议一致。")
          }
        }));
      } catch (assistError) {
        if (mappingAssistGenerationRef.current !== assistGeneration) {
          return;
        }
        const message = toErrorMessage(assistError);
        setMappingAssistById((previous) => ({
          ...previous,
          [fileId]: { status: "error", message }
        }));
      }
    },
    [activeCaseId, wizardFiles]
  );

  const applyArchiveChildMappingAssist = useCallback(
    async (fileId: string, childId: string): Promise<void> => {
      const parent = wizardFiles.find((item) => item.id === fileId);
      const child = parent?.archiveChildren.find((item) => item.id === childId);
      const assistKey = `${fileId}::${childId}`;
      if (!activeCaseId || !child) {
        return;
      }
      const assistGeneration = mappingAssistGenerationRef.current;
      if (child.domainCategory === "support") {
        setMappingAssistById((previous) => ({
          ...previous,
          [assistKey]: { status: "error", message: "该子项会按研判文件登记，不需要结构化字段映射。" }
        }));
        return;
      }
      const effectiveKind = normalizeKind(child.selectedKind || child.suggestedKind || "");
      const blueprint = mappingBlueprintForKind(effectiveKind);
      const headers = Array.from(new Set(child.headerPreview.map((item) => String(item || "").trim()).filter(Boolean)));
      if (!effectiveKind || blueprint.length === 0) {
        setMappingAssistById((previous) => ({
          ...previous,
          [assistKey]: { status: "error", message: "当前子项尚未确定入库类型，请先手动选择类型。" }
        }));
        return;
      }
      if (headers.length === 0) {
        setMappingAssistById((previous) => ({
          ...previous,
          [assistKey]: { status: "error", message: "当前子项没有可用表头样本，无法请求 AI 补全。" }
        }));
        return;
      }

      setMappingAssistById((previous) => ({
        ...previous,
        [assistKey]: { status: "running", message: "正在调用本地字段映射建议器…" }
      }));
      try {
        const response = parent
          ? await previewImportFiles(activeCaseId, [
              {
                file_name: parent.fileName,
                source_path: parent.sourcePath,
                file_kind: parent.selectedKind || parent.suggestedKind || undefined,
                password: parent.password || undefined
              }
            ])
          : null;
        if (mappingAssistGenerationRef.current !== assistGeneration) {
          return;
        }
        const previewChild = response?.items[0]?.archive_children.find((item) => item.archive_path === child.archivePath);
        const suggestedMappings = sanitizeManualFieldMapping(previewChild?.field_mapping, headers);
        const suggestedOriginFallback = mappingOriginForMethod(previewChild?.mapping_method || "");
        const suggestedOrigins = sanitizeFieldMappingOrigins(
          previewChild?.field_mapping_origins,
          suggestedMappings,
          suggestedOriginFallback
        );
        const nextMapping = { ...child.fieldMapping };
        const nextMappingOrigins = sanitizeFieldMappingOrigins(child.fieldMappingOrigins, nextMapping, "auto");
        const templateMappings = suggestFieldMappingFromHeaders(effectiveKind, headers, {});
        const appliedCount =
          applySuggestedMappingEntries(nextMapping, nextMappingOrigins, templateMappings, () => "auto") +
          applySuggestedMappingEntries(nextMapping, nextMappingOrigins, suggestedMappings, (key) => suggestedOrigins[key] || suggestedOriginFallback);
        if (mappingAssistGenerationRef.current !== assistGeneration) {
          return;
        }
        setWizardFiles((previous) =>
          previous.map((item) =>
            item.id === fileId
              ? {
                  ...item,
                  archiveChildren: item.archiveChildren.map((candidate) =>
                    candidate.id === childId ? { ...candidate, fieldMapping: nextMapping, fieldMappingOrigins: nextMappingOrigins } : candidate
                  )
                }
              : item
          )
        );
        setMappingAssistById((previous) => ({
          ...previous,
          [assistKey]: {
            status: "done",
            message:
              appliedCount > 0
                ? `已补全/修正 ${appliedCount.toLocaleString("zh-CN")} 项高置信字段映射，可继续人工微调。`
                : String(previewChild?.mapping_message || "当前字段映射已与系统建议一致。")
          }
        }));
      } catch (assistError) {
        if (mappingAssistGenerationRef.current !== assistGeneration) {
          return;
        }
        const message = toErrorMessage(assistError);
        setMappingAssistById((previous) => ({
          ...previous,
          [assistKey]: { status: "error", message }
        }));
      }
    },
    [activeCaseId, wizardFiles]
  );

  const wizardPrecheckRows = useMemo(() => {
    return wizardFiles.map((file) => {
      const passwordMissing = file.requiresPassword && !String(file.password || "").trim();
      const passwordInvalid = file.requiresPassword && !file.passwordVerified;
      const invalid = file.status === "unsupported" || passwordMissing || passwordInvalid;
      let message = file.issue || "";
      if (file.status === "unsupported") {
        message = file.issue || "当前格式暂不支持导入。";
      } else if (passwordMissing) {
        message = "该文件已加密，请先输入密码并完成验证。";
      } else if (passwordInvalid) {
        message = file.passwordValidationMessage || "密码尚未验证通过，暂时不能进入下一步。";
      } else if (!message) {
        message = "预检通过，可继续确认字段和入库类型。";
      }
      return {
        ...file,
        invalid,
        message
      };
    });
  }, [wizardFiles]);

  const wizardValidationRows = useMemo(() => {
    return wizardFiles.map((file) => {
      const insight = mappingInsightsByFileId.get(file.id);
      const requiredTotal = insight?.requiredTotal ?? 0;
      const matchedRequired = insight?.matchedRequired ?? 0;
      const requiresMapping = file.selectedCategory !== "support" && !file.selectedKind && file.archiveChildren.length === 0;
      const passwordMissing = file.requiresPassword && !String(file.password || "").trim();
      const passwordInvalid = file.requiresPassword && !file.passwordVerified;
      const missingRequired =
        file.selectedCategory !== "support" &&
        file.archiveChildren.length === 0 &&
        file.headerPreview.length > 0 &&
        requiredTotal > 0 &&
        matchedRequired < requiredTotal;
      const requiredMissingCount = missingRequired ? Math.max(0, requiredTotal - matchedRequired) : 0;
      const archiveBlockingCount = file.archiveChildren.reduce((sum, child) => {
        const childInsight = buildMappingInsight({
          selectedKind: child.selectedKind,
          suggestedKind: child.suggestedKind,
          suggestedKindLabel: child.suggestedKindLabel,
          selectedCategory: child.domainCategory,
          columnsTotal: child.columnsTotal,
          issue: child.issue,
          headerPreview: child.headerPreview,
          fieldMappings: child.fieldMapping,
          mappingOrigins: child.fieldMappingOrigins,
          mappingStatus: child.mappingStatus,
          mappingMethod: child.mappingMethod,
          mappingMessage: child.mappingMessage,
          mappingRequiredMissing: child.mappingRequiredMissing
        });
        const childRequiredTotal = childInsight.requiredTotal;
        const childMatchedRequired = childInsight.matchedRequired;
        const childRequiresKind = child.domainCategory !== "support" && !child.selectedKind;
        const childMissingRequired =
          child.domainCategory !== "support" &&
          child.headerPreview.length > 0 &&
          childRequiredTotal > 0 &&
          childMatchedRequired < childRequiredTotal;
        return sum + (childRequiresKind || childMissingRequired ? 1 : 0);
      }, 0);
      const invalid =
        file.status === "unsupported" ||
        requiresMapping ||
        passwordMissing ||
        passwordInvalid ||
        missingRequired ||
        archiveBlockingCount > 0;
      let message = file.issue || "";
      if (file.status === "unsupported") {
        message = file.issue || "当前格式不支持。";
      } else if (passwordMissing) {
        message = "该文件已加密，请先输入正确密码。";
      } else if (passwordInvalid) {
        message = file.passwordValidationMessage || "密码尚未验证通过。";
      } else if (requiresMapping) {
        message = "请先选择入库类型。";
      } else if (missingRequired) {
        message = `仍有 ${Math.max(0, requiredTotal - matchedRequired).toLocaleString("zh-CN")} 个必填字段未确认。`;
      } else if (archiveBlockingCount > 0) {
        message = `压缩包内仍有 ${archiveBlockingCount.toLocaleString("zh-CN")} 个子项未完成字段确认。`;
      } else if (!message) {
        message = file.selectedCategory === "support" ? "将登记为研判文件，不进入结构化表。" : "字段确认已完成，可执行入库。";
      }
      return {
        ...file,
        invalid,
        passwordMissing,
        requiredMissingCount,
        message
      };
    });
  }, [mappingInsightsByFileId, wizardFiles]);

  const wizardHasBlockingPrecheck = wizardPrecheckRows.some((item) => item.invalid);
  const wizardCanGoMapping = wizardFiles.length > 0 && !wizardHasBlockingPrecheck;
  const wizardCanGoValidation = canContinueImportMappingStep(
    wizardValidationRows.map((item) => ({
      invalid: item.invalid,
      requiredMissingCount: item.requiredMissingCount
    }))
  );
  const mappingStageSummary = useMemo(() => {
    let totalHeaders = 0;
    let matchedHeaders = 0;
    let pendingHeaders = 0;
    let requiredPending = 0;

    const collectInsight = (
      insight: MappingInsight,
      category: Exclude<ImportDomainCategory, "all">
    ): void => {
      if (category === "support") {
        return;
      }
      totalHeaders += Math.max(0, insight.headerCount);
      matchedHeaders += mappingMatchedColumnCount(insight);
      pendingHeaders += Math.max(0, insight.unresolvedCount);
      requiredPending += Math.max(0, insight.requiredTotal - insight.matchedRequired);
    };

    for (const file of wizardFiles) {
      if (file.archiveChildren.length > 0) {
        for (const child of file.archiveChildren) {
          collectInsight(
            buildMappingInsight({
              selectedKind: child.selectedKind,
              suggestedKind: child.suggestedKind,
              suggestedKindLabel: child.suggestedKindLabel,
              selectedCategory: child.domainCategory,
              columnsTotal: child.columnsTotal,
              issue: child.issue,
              headerPreview: child.headerPreview,
              fieldMappings: child.fieldMapping,
              mappingOrigins: child.fieldMappingOrigins,
              mappingStatus: child.mappingStatus,
              mappingMethod: child.mappingMethod,
              mappingMessage: child.mappingMessage,
              mappingRequiredMissing: child.mappingRequiredMissing
            }),
            child.domainCategory
          );
        }
        continue;
      }

      collectInsight(
        mappingInsightsByFileId.get(file.id) ??
          buildMappingInsight({
            selectedKind: file.selectedKind,
            suggestedKind: file.suggestedKind,
            suggestedKindLabel: file.suggestedKindLabel,
            selectedCategory: file.selectedCategory,
            columnsTotal: file.columnsTotal,
            issue: file.issue,
            headerPreview: file.headerPreview,
            fieldMappings: file.fieldMapping,
            mappingOrigins: file.fieldMappingOrigins,
            mappingStatus: file.mappingStatus,
            mappingMethod: file.mappingMethod,
            mappingMessage: file.mappingMessage,
            mappingRequiredMissing: file.mappingRequiredMissing
          }),
        file.selectedCategory
      );
    }

    return {
      totalHeaders,
      matchedHeaders: Math.min(totalHeaders, matchedHeaders),
      pendingHeaders: Math.max(0, pendingHeaders),
      requiredPending: Math.max(0, requiredPending)
    };
  }, [mappingInsightsByFileId, wizardFiles]);
  const mappingValidationStateByFileId = useMemo(
    () => new Map(wizardValidationRows.map((item) => [item.id, item])),
    [wizardValidationRows]
  );
  const mappingStageStatus = useMemo(() => {
    const totalFiles = wizardValidationRows.length;
    const blockingFiles = wizardValidationRows.filter((item) => item.invalid).length;
    const riskFiles = wizardValidationRows.filter((item) => item.status === "failed" || item.status === "unsupported").length;
    const readyFiles = Math.max(0, totalFiles - blockingFiles);
    const keyFieldLabel = mappingStageSummary.requiredPending > 0 ? `缺失 ${mappingStageSummary.requiredPending.toLocaleString("zh-CN")} 项` : "已齐备";
    const tone: "success" | "pending" | "risk" =
      wizardCanGoValidation ? "success" : mappingStageSummary.requiredPending > 0 || riskFiles > 0 ? "risk" : "pending";
    const statusLabel = tone === "success" ? "已完成，可进入执行入库" : tone === "risk" ? "暂不可执行入库" : "需处理后继续";
    const badgeLabel = tone === "success" ? "可继续" : tone === "risk" ? "有异常" : "待处理";
    const overviewText =
      tone === "success"
        ? "本批文件字段确认已完成。"
        : tone === "risk"
          ? "当前步骤存在阻塞问题，暂不可继续。"
          : "当前步骤仍有待处理项。";
    const handoffText =
      tone === "success"
        ? "确认完成，可继续下一步。"
        : tone === "risk"
          ? "当前存在异常，暂不可继续。"
          : "仍有待处理项，请先完成确认。";
    const reasonText = wizardCanGoValidation
      ? "当前步骤已满足执行入库条件。"
      : mappingStageSummary.requiredPending > 0
        ? `当前步骤存在 ${mappingStageSummary.requiredPending.toLocaleString("zh-CN")} 项关键字段缺失，处理完成后方可执行入库。`
      : mappingStageSummary.pendingHeaders > 0
        ? `当前步骤仍有 ${mappingStageSummary.pendingHeaders.toLocaleString("zh-CN")} 项待处理字段，处理完成后方可执行入库。`
        : blockingFiles > 0
            ? `当前步骤仍有 ${blockingFiles.toLocaleString("zh-CN")} 个文件存在阻塞问题，请先处理后继续。`
            : "请先完成字段确认后继续。";

    return {
      tone,
      totalFiles,
      readyFiles,
      blockingFiles,
      pendingHeaders: mappingStageSummary.pendingHeaders,
      matchedHeaders: mappingStageSummary.matchedHeaders,
      totalHeaders: mappingStageSummary.totalHeaders,
      requiredPending: mappingStageSummary.requiredPending,
      keyFieldLabel,
      statusLabel,
      badgeLabel,
      overviewText,
      handoffText,
      reasonText,
      confirmedFilesLabel: totalFiles > 0 ? `${readyFiles.toLocaleString("zh-CN")}/${totalFiles.toLocaleString("zh-CN")}` : "0/0"
    };
  }, [mappingStageSummary, wizardCanGoValidation, wizardValidationRows]);
  const mappingStageAssistTargets = useMemo(() => {
    const targets: Array<{ scopeKey: string; fileId: string; childId?: string }> = [];

    for (const file of wizardFiles) {
      if (file.archiveChildren.length > 0) {
        for (const child of file.archiveChildren) {
          const childHeaders = child.headerPreview.filter((item) => String(item || "").trim());
          const childInsight = buildMappingInsight({
            selectedKind: child.selectedKind,
            suggestedKind: child.suggestedKind,
            suggestedKindLabel: child.suggestedKindLabel,
            selectedCategory: child.domainCategory,
            columnsTotal: child.columnsTotal,
            issue: child.issue,
            headerPreview: child.headerPreview,
            fieldMappings: child.fieldMapping,
            mappingOrigins: child.fieldMappingOrigins,
            mappingStatus: child.mappingStatus,
            mappingMethod: child.mappingMethod,
            mappingMessage: child.mappingMessage,
            mappingRequiredMissing: child.mappingRequiredMissing
          });
          if (child.domainCategory === "support" || childInsight.targetFields.length === 0 || childHeaders.length === 0) {
            continue;
          }
          targets.push({
            scopeKey: `${file.id}::${child.id}`,
            fileId: file.id,
            childId: child.id
          });
        }
        continue;
      }

      const insight =
        mappingInsightsByFileId.get(file.id) ??
        buildMappingInsight({
          selectedKind: file.selectedKind,
          suggestedKind: file.suggestedKind,
          suggestedKindLabel: file.suggestedKindLabel,
          selectedCategory: file.selectedCategory,
          columnsTotal: file.columnsTotal,
          issue: file.issue,
          headerPreview: file.headerPreview,
          fieldMappings: file.fieldMapping,
          mappingOrigins: file.fieldMappingOrigins,
          mappingStatus: file.mappingStatus,
          mappingMethod: file.mappingMethod,
          mappingMessage: file.mappingMessage,
          mappingRequiredMissing: file.mappingRequiredMissing
        });
      const headers = file.headerPreview.filter((item) => String(item || "").trim());
      if (file.selectedCategory === "support" || insight.targetFields.length === 0 || headers.length === 0) {
        continue;
      }
      targets.push({ scopeKey: file.id, fileId: file.id });
    }

    return targets;
  }, [mappingInsightsByFileId, wizardFiles]);
  const mappingStageHasMappings = useMemo(
    () =>
      wizardFiles.some(
        (file) =>
          Object.keys(file.fieldMapping).length > 0 ||
          file.archiveChildren.some((child) => Object.keys(child.fieldMapping).length > 0)
      ),
    [wizardFiles]
  );
  const mappingStageAssistBusy = mappingStageAssistRunning || mappingStageAssistTargets.some((target) => mappingAssistById[target.scopeKey]?.status === "running");
  const resetAllWizardMappings = useCallback((): void => {
    const clearedCount = wizardFiles.reduce((sum, file) => {
      const fileMappingCount = Object.values(file.fieldMapping).filter((value) => String(value || "").trim()).length;
      const archiveMappingCount = file.archiveChildren.reduce(
        (childSum, child) => childSum + Object.values(child.fieldMapping).filter((value) => String(value || "").trim()).length,
        0
      );
      return sum + fileMappingCount + archiveMappingCount;
    }, 0);

    mappingAssistGenerationRef.current += 1;
    setMappingStageAssistRunning(false);
    setMappingAssistById({});
    setWizardFiles((previous) =>
      previous.map((file) => ({
        ...file,
        fieldMapping: {},
        fieldMappingOrigins: {},
        archiveChildren: file.archiveChildren.map((child) => ({ ...child, fieldMapping: {}, fieldMappingOrigins: {} }))
      }))
    );

    if (clearedCount > 0) {
      showToast({
        tone: "success",
        title: "字段匹配已重置",
        detail: `已取消 ${clearedCount.toLocaleString("zh-CN")} 项字段匹配。`
      });
      return;
    }
    if (mappingStageAssistBusy) {
      showToast({
        tone: "success",
        title: "字段匹配已重置",
        detail: "当前正在返回的 AI 补全结果已忽略。"
      });
      return;
    }
    showToast({
      tone: "info",
      title: "当前没有可重置的字段匹配",
      detail: "请先执行自动匹配或手动映射后，再进行重置。"
    });
  }, [mappingStageAssistBusy, wizardFiles]);
  const applyMappingStageAssist = useCallback(async (): Promise<void> => {
    if (mappingStageAssistRunning || mappingStageAssistTargets.length === 0) {
      return;
    }

    const assistGeneration = mappingAssistGenerationRef.current;
    setMappingStageAssistRunning(true);
    try {
      for (const target of mappingStageAssistTargets) {
        if (mappingAssistGenerationRef.current !== assistGeneration) {
          break;
        }
        if (target.childId) {
          await applyArchiveChildMappingAssist(target.fileId, target.childId);
          continue;
        }
        await applyMappingAssist(target.fileId);
      }
    } finally {
      if (mappingAssistGenerationRef.current === assistGeneration) {
        setMappingStageAssistRunning(false);
      }
    }
  }, [applyArchiveChildMappingAssist, applyMappingAssist, mappingStageAssistRunning, mappingStageAssistTargets]);
  const currentWizardJob = useMemo(
    () => jobs.find((item) => item.job_id === wizardImportJobId) ?? null,
    [jobs, wizardImportJobId]
  );
  const wizardValidationStarted = Boolean(currentWizardJob);
  const wizardIsImporting = Boolean(currentWizardJob && (currentWizardJob.status === "running" || currentWizardJob.status === "queued"));
  const wizardCanGoSummary = Boolean(
    (currentWizardJob && (currentWizardJob.status === "succeeded" || currentWizardJob.status === "failed" || currentWizardJob.status === "canceled")) ||
      activeFileLogs.length > 0 ||
      historicalDatasets.length > 0
  );

  const wizardProgressFiles = useMemo(() => {
    if (!currentWizardJob) {
      return wizardValidationRows;
    }
    return wizardValidationRows.map((file) => {
      const matches = (currentWizardJob.files || []).filter((item) => {
        const displayPath = String(item.display_path || "");
        const displayName = String(item.display_name || "");
        const stem = String(file.fileName || "").replace(/\.[^.]+$/, "");
        return (
          displayPath === file.sourcePath ||
          displayPath.startsWith(`${file.sourcePath}::`) ||
          displayName === file.fileName ||
          (stem && displayName.startsWith(stem))
        );
      });
      if (matches.length === 0) {
        const pendingStatus: WizardFileState = file.status === "failed" || file.status === "unsupported" ? file.status : "ready";
        return {
          ...file,
          progress: 0,
          status: pendingStatus,
        };
      }
      const hasFailed = matches.some((item) => item.status === "failed" || item.status === "canceled");
      const hasRunning = matches.some((item) => item.status === "running" || item.status === "queued");
      const progress = matches.length
        ? Math.round(
            matches.reduce((sum, item) => {
              if (item.rows_total !== null && item.rows_total > 0) {
                return sum + runningPercent(item.rows_seen, item.rows_total);
              }
              if (item.status === "succeeded") {
                return sum + 100;
              }
              if (item.status === "failed" || item.status === "canceled") {
                return sum + 100;
              }
              return sum + clampWizardProgress(currentWizardJob.progress);
            }, 0) / matches.length
          )
        : file.progress;
      const issue = matches.map((item) => String(item.error || item.note || "").trim()).filter(Boolean).join("\n");
      const nextStatus: WizardFileState =
        hasFailed ? "failed" : hasRunning ? "running" : "succeeded";
      return {
        ...file,
        status: nextStatus,
        progress,
        issue: issue || file.issue
      };
    });
  }, [currentWizardJob, wizardValidationRows]);

  const wizardProgressGroups = useMemo<ImportWizardProgressGroup[]>(() => {
    const progressById = new Map(wizardProgressFiles.map((file) => [file.id, file]));
    return wizardFiles.map((file) => {
      const progressFile = progressById.get(file.id) ?? file;
      const childProgress: ImportWizardProgressChild[] = file.archiveChildren.map((child) => {
        const matches = (currentWizardJob?.files || []).filter((item) => {
          const displayPath = String(item.display_path || "");
          return (
            displayPath === `${file.sourcePath}::${child.archivePath}` ||
            displayPath === `${file.fileName}::${child.archivePath}` ||
            displayPath.endsWith(`::${child.archivePath}`)
          );
        });
        const hasFailed = matches.some((item) => item.status === "failed" || item.status === "canceled");
        const hasRunning = matches.some((item) => item.status === "running" || item.status === "queued");
        const progress = matches.length
          ? Math.round(
              matches.reduce((sum, item) => {
                if (item.rows_total !== null && item.rows_total > 0) {
                  return sum + runningPercent(item.rows_seen, item.rows_total);
                }
                if (item.status === "succeeded" || item.status === "failed" || item.status === "canceled") {
                  return sum + 100;
                }
                return sum + clampWizardProgress(currentWizardJob?.progress ?? 0);
              }, 0) / matches.length
            )
          : 0;
        const issue = matches.map((item) => String(item.error || item.note || "").trim()).filter(Boolean).join("\n");
        const jobKind = matches.find((item) => String(item.kind || "").trim())?.kind || "";
        const childStatus: WizardFileState =
          hasFailed
            ? "failed"
            : hasRunning
              ? "running"
              : matches.length > 0
                ? "succeeded"
                : child.status === "failed" || child.status === "unsupported"
                  ? child.status
                  : "ready";
        return {
          ...child,
          progress: Math.max(0, Math.min(100, progress)),
          status: childStatus,
          issue: issue || child.issue,
          kindLabel: kindLabel(jobKind || child.selectedKind || child.suggestedKind || ""),
          isActive: hasRunning,
        };
      });
      const isArchive = childProgress.length > 0;
      const archiveHasFailed = childProgress.some((child) => child.status === "failed" || child.status === "unsupported");
      const archiveHasRunning = childProgress.some((child) => child.status === "running" || child.isActive);
      const archiveAllSucceeded = isArchive && childProgress.every((child) => child.status === "succeeded");
      const archiveProgress = isArchive
        ? Math.round(childProgress.reduce((sum, child) => sum + clampWizardProgress(child.progress), 0) / Math.max(childProgress.length, 1))
        : progressFile.progress;
      const archiveIssue = childProgress.map((child) => String(child.issue || "").trim()).filter(Boolean).join("\n");
      const archiveStatus: WizardFileState = archiveHasFailed
        ? "failed"
        : archiveHasRunning
          ? "running"
          : archiveAllSucceeded
            ? "succeeded"
            : progressFile.status === "failed" || progressFile.status === "unsupported"
              ? progressFile.status
              : "ready";
      return {
        ...progressFile,
        progress: isArchive ? archiveProgress : progressFile.progress,
        status: isArchive ? archiveStatus : progressFile.status,
        issue: archiveIssue || progressFile.issue,
        isArchive,
        childProgress,
        isActive: isArchive ? archiveHasRunning : progressFile.status === "running",
        message: String(("message" in progressFile ? progressFile.message : "") || ""),
      };
    });
  }, [currentWizardJob, wizardFiles, wizardProgressFiles]);

  const validationFilterOptions = useMemo(
    () => [
      {
        value: "all" as const,
        label: "全部"
      },
      {
        value: "completed" as const,
        label: "已完成"
      },
      {
        value: "pending" as const,
        label: "进行中 / 待完成"
      }
    ],
    []
  );

  const validationDisplayGroups = useMemo(
    () =>
      wizardProgressGroups.map((file) => {
        const complete = wizardProgressGroupComplete(file);
        const hidden =
          validationFilter === "all"
            ? false
            : validationFilter === "completed"
              ? !complete
              : complete;
        return {
          file,
          complete,
          hidden,
        };
      }),
    [validationFilter, wizardProgressGroups]
  );

  const visibleValidationGroupCount = useMemo(
    () => validationDisplayGroups.filter((item) => !item.hidden).length,
    [validationDisplayGroups]
  );

  const validationAllCompleted = useMemo(
    () => wizardProgressGroups.length > 0 && wizardProgressGroups.every((file) => wizardProgressGroupComplete(file)),
    [wizardProgressGroups]
  );

  const showValidationEmptyState = useMemo(() => {
    if (visibleValidationGroupCount > 0) {
      return false;
    }
    if (wizardStep === "validation" && validationAllCompleted && validationFilter === "pending") {
      return false;
    }
    return true;
  }, [validationAllCompleted, validationFilter, visibleValidationGroupCount, wizardStep]);

  useEffect(() => {
    if (!wizardIsImporting || !validationStreamRef.current) {
      return;
    }
    const target =
      validationStreamRef.current.querySelector<HTMLElement>("[data-progress-active='true']") ||
      validationStreamRef.current.querySelector<HTMLElement>("[data-progress-group-active='true']");
    if (!target) {
      return;
    }
    const activeId = String(target.dataset.progressId || "");
    if (!activeId || activeProgressRef.current === activeId) {
      return;
    }
    activeProgressRef.current = activeId;
    target.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "nearest" });
  }, [validationDisplayGroups, wizardIsImporting]);

  const previousWizardStepRef = useRef<WizardStep>(wizardStep);

  useEffect(() => {
    const previousStep = previousWizardStepRef.current;
    if (wizardStep === "validation" && previousStep !== "validation" && validationAllCompleted && validationFilter === "pending") {
      setValidationFilter("completed");
    }
    previousWizardStepRef.current = wizardStep;
  }, [validationAllCompleted, validationFilter, wizardStep]);

  useEffect(() => {
    if (wizardStep !== "validation" || !validationAllCompleted || validationFilter === "completed") {
      return;
    }
    const timer = window.setTimeout(() => {
      setValidationFilter("completed");
    }, VALIDATION_GROUP_EXIT_DURATION_MS + 60);
    return () => {
      window.clearTimeout(timer);
    };
  }, [validationAllCompleted, validationFilter, wizardStep]);

  const goToWizardStep = useCallback(
    (step: WizardStep): void => {
      if (step === "mapping" && !wizardCanGoMapping) {
        return;
      }
      if (step === "validation" && !wizardCanGoValidation) {
        return;
      }
      if (step === "summary" && !wizardCanGoSummary) {
        return;
      }
      setWizardStep(step);
    },
    [wizardCanGoMapping, wizardCanGoSummary, wizardCanGoValidation]
  );

  const onSubmitWizardImport = useCallback(async (): Promise<void> => {
    if (!wizardCanGoValidation) {
      setError("当前仍有文件未完成映射或格式不支持。");
      showToast({
        tone: "warning",
        title: "仍有文件待复核",
        detail: "请先完成字段映射，或移除当前不支持的文件。"
      });
      return;
    }
    const missingEvidenceReceipt = wizardFiles.some(
      (file) =>
        !file.sha256 ||
        file.size < 0 ||
        file.archiveChildren.some((child) => !child.sha256 || child.size < 0)
    );
    if (missingEvidenceReceipt) {
      setError("文件证据回执不完整，请重新执行文件预检后再导入。");
      showToast({
        tone: "warning",
        title: "证据回执不完整",
        detail: "每个源文件和压缩包成员都必须具有完整 SHA-256 与字节大小。"
      });
      return;
    }
    const created = await createImportTask(
      wizardFiles.map((file) => ({
        file_name: file.fileName,
        source_path: file.sourcePath,
        expected_sha256: file.sha256 || undefined,
        expected_size: file.size,
        file_kind:
          file.archiveChildren.length > 0
            ? undefined
            : file.selectedKind || (file.selectedCategory === "support" ? "support_file" : undefined),
        password: file.password.trim() || undefined,
        field_mapping: serializeFieldMapping(file),
        field_mapping_origins: serializeFieldMappingOrigins(file),
        archive_items:
          file.archiveChildren.length > 0
            ? file.archiveChildren.map((child) => ({
                archive_path: child.archivePath,
                file_kind: child.selectedKind || (child.domainCategory === "support" ? "support_file" : undefined),
                expected_sha256: child.sha256,
                expected_size: child.size,
                field_mapping:
                  child.domainCategory === "support"
                    ? undefined
                    : serializeKindFieldMapping(child.selectedKind || child.suggestedKind || "", child.headerPreview, child.fieldMapping),
                field_mapping_origins:
                  child.domainCategory === "support"
                    ? undefined
                    : serializeKindFieldMappingOrigins(
                        child.selectedKind || child.suggestedKind || "",
                        child.headerPreview,
                        child.fieldMapping,
                        child.fieldMappingOrigins
                      )
              }))
            : undefined
      }))
    );
    if (!created) {
      return;
    }
    setWizardImportJobId(created.job_id);
    setWizardStep("validation");
    setWizardFiles((previous) =>
      previous.map((file) => ({
        ...file,
        status: file.status === "unsupported" ? "unsupported" : "ready",
        progress: 0
      }))
    );
  }, [createImportTask, wizardCanGoValidation, wizardFiles]);

  const allRows = useMemo<LedgerRow[]>(() => {
    const map = new Map<string, LedgerRow>();
    for (const log of activeFileLogs) {
      const row = toLedgerRow(log);
      map.set(row.file_id || row.key, row);
    }

    for (const job of jobs) {
      const jobIsLive = job.status === "running" || job.status === "queued";
      if (!jobIsLive) {
        continue;
      }
      for (const file of job.files || []) {
        const row = toLedgerRowFromJobFile(file, job);
        const key = row.file_id || row.key;
        const existing = map.get(key);
        if (!existing) {
          map.set(key, row);
          continue;
        }
        if (row.status_text.startsWith("导入中") || row.status_text === "等待中") {
          map.set(key, {
            ...existing,
            status_text: row.status_text,
            rows_seen: maxKnownMetric(existing.rows_seen, row.rows_seen),
            rows_total: maxKnownMetric(existing.rows_total, row.rows_total),
            note: existing.note || row.note,
            error: existing.error || row.error
          });
        }
      }
    }

    return Array.from(map.values());
  }, [activeFileLogs, jobs]);

  const recycleRows = useMemo<LedgerRow[]>(() => recycleFileLogs.map((log) => toLedgerRow(log)), [recycleFileLogs]);

  const allConsoleRows = useMemo<ImportConsoleRow[]>(() => {
    const remainingDatasets = [...historicalDatasets];
    const mergedRows: ImportConsoleRow[] = allRows.map((row) => {
      const matchedIndex = exactDatasetIndex(row.file_id, remainingDatasets);

      const dataset = matchedIndex >= 0 ? remainingDatasets.splice(matchedIndex, 1)[0] : null;
      const category = resolveImportCategory(row.kind, row.display_name, row.display_path || row.stored_path);
      const source: ImportConsoleRow["source"] = dataset ? "merged" : "ledger";
      const validRows = row.valid_rows;
      const duplicateRows = row.duplicate_rows;
      const historicalCounts = dataset
        ? projectUnverifiedHistoricalCounts(dataset)
        : null;
      const statusText = dataset && row.status_text === "等待中" && validRows !== null && validRows > 0
        ? "已完成"
        : row.status_text;

      return {
        key: row.key,
        fileName: row.display_name,
        category,
        categoryLabel: importCategoryLabel(category),
        categoryEnglish: importCategoryEnglish(category),
        kindLabel: row.kind_label || "—",
        statusText,
        rowsTotal: row.rows_total,
        validRows,
        duplicateRows,
        // The historical-dataset endpoint does not yet provide a same-epoch,
        // same-snapshot receipt. Do not upgrade its column count into a public
        // case fact while merging it with the stricter import ledger row.
        columnCount: historicalCounts?.columnCount ?? null,
        datasetId: dataset?.dataset_id || row.file_id || "",
        sha256: row.sha256 || "",
        path: row.display_path || row.stored_path || dataset?.stored_path || "",
        importedAt: dataset?.imported_at || row.finished_at || row.created_at || "",
        detailText: trimDetailText(row.error || row.note || ""),
        duplicateFlag: duplicateFlagForRow({ statusText, duplicateRows, validRows, source }),
        duplicateSummary: duplicateSummaryForRow({ statusText, duplicateRows, validRows, source }),
        source,
        ledgerRow: row
      };
    });

    for (const dataset of remainingDatasets) {
      const category = resolveImportCategory(dataset.kind, dataset.filename, dataset.stored_path);
      const historicalCounts = projectUnverifiedHistoricalCounts(dataset);
      const statusText = "历史记录（未核验）";
      mergedRows.push({
        key: `historical:${dataset.dataset_id}`,
        fileName: dataset.filename || "未命名文件",
        category,
        categoryLabel: importCategoryLabel(category),
        categoryEnglish: importCategoryEnglish(category),
        kindLabel: kindLabel(normalizeKind(dataset.kind, dataset.filename) || dataset.kind || ""),
        statusText,
        rowsTotal: historicalCounts.rowsTotal,
        validRows: historicalCounts.validRows,
        duplicateRows: null,
        columnCount: historicalCounts.columnCount,
        datasetId: dataset.dataset_id,
        sha256: "",
        path: dataset.stored_path || "",
        importedAt: dataset.imported_at || "",
        detailText: "历史数据集尚未绑定当前案件 epoch、数据快照与证据回执。",
        duplicateFlag: duplicateFlagForRow({ statusText, duplicateRows: null, validRows: null, source: "historical" }),
        duplicateSummary: duplicateSummaryForRow({ statusText, duplicateRows: null, validRows: null, source: "historical" }),
        source: "historical",
        ledgerRow: null
      });
    }

    return mergedRows.sort((left, right) => {
      const delta = toTs(left.importedAt) - toTs(right.importedAt);
      if (delta !== 0) {
        return -delta;
      }
      return left.fileName.localeCompare(right.fileName);
    });
  }, [allRows, historicalDatasets]);

  const recycleConsoleRows = useMemo<ImportConsoleRow[]>(
    () =>
      recycleRows
        .map((row) => {
          const category = resolveImportCategory(row.kind, row.display_name, row.display_path || row.stored_path);
          const validRows = row.valid_rows;
          const duplicateRows = row.duplicate_rows;
          const statusText = row.status_text;
          const source: ImportConsoleRow["source"] = "ledger";
          return {
            key: row.key,
            fileName: row.display_name,
            category,
            categoryLabel: importCategoryLabel(category),
            categoryEnglish: importCategoryEnglish(category),
            kindLabel: row.kind_label || "—",
            statusText,
            rowsTotal: row.rows_total,
            validRows,
            duplicateRows,
            columnCount: null,
            datasetId: row.file_id || "",
            sha256: row.sha256 || "",
            path: row.display_path || row.stored_path || "",
            importedAt: row.recycled_at || row.finished_at || row.created_at || "",
            detailText: trimDetailText(row.error || row.note || ""),
            duplicateFlag: duplicateFlagForRow({ statusText, duplicateRows, validRows, source }),
            duplicateSummary: duplicateSummaryForRow({ statusText, duplicateRows, validRows, source }),
            source,
            ledgerRow: row
          };
        })
        .sort((left, right) => {
          const delta = toTs(left.importedAt) - toTs(right.importedAt);
          if (delta !== 0) {
            return -delta;
          }
          return left.fileName.localeCompare(right.fileName);
        }),
    [recycleRows]
  );

  const ledgerRows = ledgerView === "recycle" ? recycleRows : allRows;
  const ledgerConsoleRowsBase = ledgerView === "recycle" ? recycleConsoleRows : allConsoleRows;

  const consoleRows = useMemo<ImportConsoleRow[]>(() => {
    const keyword = searchText.trim().toLowerCase();
    return ledgerConsoleRowsBase.filter((row) => {
      if (categoryFilter !== "all" && row.category !== categoryFilter) {
        return false;
      }
      if (!keyword) {
        return true;
      }
      const blob = [
        row.fileName,
        row.categoryLabel,
        row.categoryEnglish,
        row.kindLabel,
        row.statusText,
        row.sha256,
        row.path,
        row.duplicateSummary
      ]
        .join(" ")
        .toLowerCase();
      return blob.includes(keyword);
    });
  }, [categoryFilter, ledgerConsoleRowsBase, searchText]);

  const selectedLedgerFileIdSet = useMemo(() => new Set(selectedLedgerFileIds), [selectedLedgerFileIds]);

  const canSelectLedgerRow = useCallback((row: LedgerRow | null | undefined): boolean => {
    if (!row?.file_id) {
      return false;
    }
    return ledgerView === "recycle" ? true : canApplyLedgerAction(row, "recycle");
  }, [canApplyLedgerAction, ledgerView]);

  const selectableConsoleRows = useMemo(
    () => consoleRows.filter((row) => row.ledgerRow && canSelectLedgerRow(row.ledgerRow)),
    [canSelectLedgerRow, consoleRows]
  );

  const allVisibleSelectableSelected =
    selectableConsoleRows.length > 0 &&
    selectableConsoleRows.every((row) => row.ledgerRow && selectedLedgerFileIdSet.has(row.ledgerRow.file_id));

  const someVisibleSelectableSelected =
    selectableConsoleRows.some((row) => row.ledgerRow && selectedLedgerFileIdSet.has(row.ledgerRow.file_id)) &&
    !allVisibleSelectableSelected;

  const selectedActionRows = useMemo(() => {
    const rowsByFileId = new Map<string, LedgerRow>();
    for (const row of ledgerRows) {
      if (!selectedLedgerFileIdSet.has(row.file_id) || !canSelectLedgerRow(row)) {
        continue;
      }
      rowsByFileId.set(row.file_id, row);
    }
    return Array.from(rowsByFileId.values());
  }, [canSelectLedgerRow, ledgerRows, selectedLedgerFileIdSet]);

  const selectedActionCount = selectedActionRows.length;

  const toggleLedgerRowSelection = useCallback((row: LedgerRow, checked: boolean): void => {
    if (!canSelectLedgerRow(row)) {
      return;
    }
    setSelectedLedgerFileIds((previous) => {
      const next = new Set(previous);
      if (checked) {
        next.add(row.file_id);
      } else {
        next.delete(row.file_id);
      }
      return Array.from(next);
    });
  }, [canSelectLedgerRow]);

  const toggleSelectAllVisibleLedgerRows = useCallback((checked: boolean): void => {
    const visibleIds = selectableConsoleRows
      .map((row) => String(row.ledgerRow?.file_id || "").trim())
      .filter(Boolean);
    if (visibleIds.length === 0) {
      return;
    }
    setSelectedLedgerFileIds((previous) => {
      const next = new Set(previous);
      for (const fileId of visibleIds) {
        if (checked) {
          next.add(fileId);
        } else {
          next.delete(fileId);
        }
      }
      return Array.from(next);
    });
  }, [selectableConsoleRows]);

  useEffect(() => {
    const validIds = new Set(
      ledgerRows
        .filter((row) => canSelectLedgerRow(row))
        .map((row) => String(row.file_id || "").trim())
        .filter(Boolean)
    );
    setSelectedLedgerFileIds((previous) => {
      const next = previous.filter((fileId) => validIds.has(fileId));
      return next.length === previous.length ? previous : next;
    });
  }, [canSelectLedgerRow, ledgerRows]);

  useEffect(() => {
    setSelectedLedgerFileIds([]);
    setLedgerActionDialogState(null);
  }, [ledgerView]);

  useEffect(() => {
    if (ledgerSelectAllRef.current) {
      ledgerSelectAllRef.current.indeterminate = someVisibleSelectableSelected;
    }
  }, [someVisibleSelectableSelected]);

  const categoryCards = useMemo<ImportCategoryCardModel[]>(
    () =>
      CATEGORY_META.map((category) => {
        const rows = ledgerConsoleRowsBase.filter((row) => row.category === category.key);
        return {
          key: category.key,
          label: category.label,
          english: category.english,
          hint: category.hint,
          fileCount: rows.length,
          rowCount: sumCompleteMetrics(
            rows.map((row) => maxKnownMetric(row.validRows, row.rowsTotal))
          )
        };
      }),
    [ledgerConsoleRowsBase]
  );

  const ledgerAttentionCount = useMemo(
    () =>
      ledgerConsoleRowsBase.filter((row) => {
        if (row.ledgerRow) {
          return rowHasAttention(row.ledgerRow);
        }
        return Boolean(row.detailText);
      }).length,
    [ledgerConsoleRowsBase]
  );

  const visibleRows = useMemo(() => {
    const keyword = searchText.trim().toLowerCase();

    const keywordFiltered = keyword
      ? allRows.filter((row) => {
          const blob = [
            row.display_name,
            row.display_path,
            row.stored_path,
            row.sha256,
            row.kind,
            row.kind_label,
            row.file_type,
            row.status_text
          ]
            .join(" ")
            .toLowerCase();
          return blob.includes(keyword);
        })
      : [...allRows];

    const kindFiltered =
      kindFilter === "all" || kindFilter === "__tasks__"
        ? keywordFiltered
        : keywordFiltered.filter((row) => row.kind === kindFilter);

    const filtered = kindFiltered.filter((row) => rowMatchesStatusFilter(row, statusFilter));

    filtered.sort((left, right) => {
      let delta = 0;
      if (sortField === "created") {
        delta = toTs(left.created_at || left.finished_at) - toTs(right.created_at || right.finished_at);
      } else if (sortField === "status") {
        delta = left.status_text.localeCompare(right.status_text);
      } else if (sortField === "kind") {
        delta = left.kind_label.localeCompare(right.kind_label);
      } else if (sortField === "name") {
        delta = left.display_name.localeCompare(right.display_name);
      } else if (sortField === "rows") {
        delta = compareKnownMetrics(left.valid_rows, right.valid_rows, sortDirection);
        if (delta !== 0) {
          return delta;
        }
      } else if (sortField === "size") {
        delta = compareKnownMetrics(left.size, right.size, sortDirection);
        if (delta !== 0) {
          return delta;
        }
      }

      if (delta === 0) {
        delta = toTs(left.created_at || left.finished_at) - toTs(right.created_at || right.finished_at);
      }
      return sortDirection === "asc" ? delta : -delta;
    });

    return filtered;
  }, [allRows, kindFilter, searchText, sortDirection, sortField, statusFilter]);

  useEffect(() => {
    if (!visibleRows.length) {
      setSelectedRowKey("");
      return;
    }
    setSelectedRowKey((previous) => (visibleRows.some((row) => row.key === previous) ? previous : visibleRows[0].key));
  }, [visibleRows]);

  const selectedRow = useMemo(
    () => visibleRows.find((row) => row.key === selectedRowKey) ?? null,
    [selectedRowKey, visibleRows]
  );

  const overview = useMemo(() => {
    const rowsTotal = sumCompleteMetrics(allRows.map((row) => row.rows_total));
    const rowsImported = sumCompleteMetrics(allRows.map((row) => row.valid_rows));
    const successCount = allRows.filter((row) => row.status_text.startsWith("已完成")).length;
    const failCount = allRows.filter((row) => row.status_text === "失败").length;
    const attentionCount = allRows.filter((row) => rowHasAttention(row)).length;
    const runningByRows = allRows.some((row) => isActiveStatusText(row.status_text));
    const runningByJobs = jobs.some((job) => job.status === "running" || job.status === "queued");
    const running = runningByRows || runningByJobs;
    const runningCount = jobs.filter((job) => job.status === "running" || job.status === "queued").length;

    const activeRow = allRows.find((row) => row.status_text.startsWith("导入中"));
    const activeJob = jobs.find((job) => job.status === "running");
    const currentTaskName = activeRow?.display_name || activeJob?.current_file || "—";
    const totalSize = sumCompleteMetrics(allRows.map((row) => row.size));
    const latestFinishedAt = allRows.reduce((latest, row) => {
      const candidate = row.finished_at || row.created_at;
      return toTs(candidate) > toTs(latest) ? candidate : latest;
    }, "");

    const progress =
      rowsTotal !== null && rowsImported !== null && rowsTotal > 0
        ? Math.max(0, Math.min(100, Math.floor((rowsImported * 100) / Math.max(rowsTotal, 1))))
        : Math.max(0, ...jobs.map((item) => toInt(item.progress)));

    return {
      rowsTotal,
      rowsImported,
      successCount,
      failCount,
      attentionCount,
      running,
      runningCount,
      currentTaskName,
      progress,
      totalSize,
      latestFinishedAt
    };
  }, [allRows, jobs]);

  const tileModels = useMemo(() => {
    const valueByKind: Record<string, number> = {};
    const totalByKind: Record<string, number> = {};
    const seenByKind: Record<string, number> = {};
    const activeByKind: Record<string, boolean> = {};
    const unresolvedByKind: Record<string, boolean> = {};

    for (const row of allRows) {
      const kind = row.kind || "";
      if (!kind) {
        continue;
      }
      if (row.valid_rows === null || row.rows_total === null || row.rows_seen === null) {
        unresolvedByKind[kind] = true;
      } else {
        valueByKind[kind] = (valueByKind[kind] ?? 0) + row.valid_rows;
        totalByKind[kind] = (totalByKind[kind] ?? 0) + row.rows_total;
        seenByKind[kind] = (seenByKind[kind] ?? 0) + Math.min(row.rows_seen, row.rows_total || row.rows_seen);
      }
      if (row.status_text.startsWith("导入中") || row.status_text === "等待中") {
        activeByKind[kind] = true;
      }
    }

    const taskTotal = allRows.length;
    const taskDone = allRows.filter(
      (row) =>
        row.status_text.startsWith("已完成") ||
        row.status_text === "失败" ||
        row.status_text === "已取消" ||
        row.status_text === "重复数据"
    ).length;
    const taskBusy = allRows.some((row) => row.status_text.startsWith("导入中") || row.status_text === "等待中");

    return TILE_ORDER.map((key) => {
      if (key === "__tasks__") {
        const percent = taskTotal > 0 ? Math.floor((taskDone * 100) / taskTotal) : 0;
        return {
          key,
          title: KIND_CN[key],
          value: taskTotal,
          progress: taskBusy ? Math.max(percent, 6) : taskTotal > 0 ? 100 : 0,
          busy: taskBusy
        };
      }

      const unresolved = Boolean(unresolvedByKind[key]);
      const total = unresolved ? null : toInt(totalByKind[key]);
      const seen = unresolved ? null : toInt(seenByKind[key]);
      const value = unresolved ? null : toInt(valueByKind[key]);
      const busy = Boolean(activeByKind[key]);
      const progress = total !== null && seen !== null && total > 0
        ? Math.floor((Math.min(seen, total) * 100) / total)
        : value !== null && value > 0
          ? 100
          : 0;
      return {
        key,
        title: KIND_CN[key] ?? key,
        value,
        progress: busy ? Math.max(progress, 8) : progress,
        busy
      };
    });
  }, [allRows]);

  const typeSummaryRows = useMemo<TypeSummaryRow[]>(() => {
    const rowsByKind: Record<string, number> = {};
    const totalByKind: Record<string, number> = {};
    const taskCountByKind: Record<string, number> = {};
    const seenByKind: Record<string, number> = {};
    const attentionByKind: Record<string, number> = {};
    const activeByKind: Record<string, boolean> = {};
    const unresolvedByKind: Record<string, boolean> = {};

    for (const row of allRows) {
      const kind = row.kind || "";
      if (!kind) {
        continue;
      }
      taskCountByKind[kind] = (taskCountByKind[kind] ?? 0) + 1;
      if (row.valid_rows === null || row.rows_total === null || row.rows_seen === null) {
        unresolvedByKind[kind] = true;
      } else {
        rowsByKind[kind] = (rowsByKind[kind] ?? 0) + row.valid_rows;
        totalByKind[kind] = (totalByKind[kind] ?? 0) + row.rows_total;
        seenByKind[kind] = (seenByKind[kind] ?? 0) + Math.min(row.rows_seen, row.rows_total || row.rows_seen);
      }
      if (rowHasAttention(row)) {
        attentionByKind[kind] = (attentionByKind[kind] ?? 0) + 1;
      }
      if (isActiveStatusText(row.status_text)) {
        activeByKind[kind] = true;
      }
    }

    const taskTotal = allRows.length;
    const taskDone = allRows.filter(
      (row) =>
        row.status_text.startsWith("已完成") ||
        row.status_text === "失败" ||
        row.status_text === "已取消" ||
        row.status_text === "重复数据"
    ).length;

    const rows: TypeSummaryRow[] = [
      {
        key: "__tasks__",
        title: KIND_CN.__tasks__,
        taskCount: taskTotal,
        importedRows: overview.rowsImported,
        totalRows: overview.rowsTotal,
        progress: taskTotal > 0 ? Math.floor((taskDone * 100) / taskTotal) : 0,
        busy: overview.running,
        attentionCount: overview.attentionCount
      }
    ];

    for (const key of TILE_ORDER) {
      if (key === "__tasks__") {
        continue;
      }
      rows.push({
        key: key as ImportKindFilter,
        title: KIND_CN[key] ?? key,
        taskCount: toInt(taskCountByKind[key]),
        importedRows: unresolvedByKind[key] ? null : toInt(rowsByKind[key]),
        totalRows: unresolvedByKind[key] ? null : toInt(totalByKind[key]),
        progress:
          !unresolvedByKind[key] && totalByKind[key] && totalByKind[key] > 0
            ? Math.floor((Math.min(toInt(seenByKind[key]), toInt(totalByKind[key])) * 100) / Math.max(toInt(totalByKind[key]), 1))
            : toInt(rowsByKind[key]) > 0
              ? 100
              : 0,
        busy: Boolean(activeByKind[key]),
        attentionCount: toInt(attentionByKind[key])
      });
    }

    return rows;
  }, [allRows, overview.attentionCount, overview.rowsImported, overview.rowsTotal, overview.running]);

  const historicalImportDetailRows = useMemo<HistoricalImportDetailRow[]>(
    () => {
      const keyword = searchText.trim().toLowerCase();
      const filteredDatasets = historicalDatasets.filter((row) => {
        const normalizedKind = normalizeKind(row.kind, row.filename);
        if (kindFilter !== "all" && kindFilter !== "__tasks__" && normalizedKind !== kindFilter) {
          return false;
        }
        if (!keyword) {
          return true;
        }
        const blob = [row.filename, row.kind, normalizedKind, row.dataset_id, row.stored_path].join(" ").toLowerCase();
        return blob.includes(keyword);
      });

      if (filteredDatasets.length > 0) {
        return filteredDatasets.map((row) => {
          const normalizedKind = normalizeKind(row.kind, row.filename);
          const historicalCounts = projectUnverifiedHistoricalCounts(row);
          return {
            key: row.dataset_id,
            fileName: row.filename || "未命名文件",
            kindLabel: kindLabel(normalizedKind || row.kind || ""),
            rowCount: historicalCounts.rowsTotal,
            columnCountLabel: "列数未核验",
            importedAt: formatDateTime(row.imported_at || ""),
            datasetId: row.dataset_id
          };
        });
      }

      return visibleRows.map((row) => ({
        key: row.key,
        fileName: row.display_name,
        kindLabel: row.kind_label || "—",
        rowCount: row.rows_total !== null ? row.rows_total : row.valid_rows,
        columnCountLabel: "—",
        importedAt: formatDateTime(row.created_at || row.finished_at),
        datasetId: row.file_id || row.key
      }));
    },
    [kindFilter, historicalDatasets, searchText, visibleRows]
  );

  useEffect(() => {
    if (!historicalImportDetailRows.length) {
      setSelectedHistoricalDatasetId("");
      return;
    }
    setSelectedHistoricalDatasetId((previous) =>
      historicalImportDetailRows.some((row) => row.datasetId === previous) ? previous : historicalImportDetailRows[0].datasetId
    );
  }, [historicalImportDetailRows]);

  const selectedRowDetailText = trimDetailText(selectedRow?.error || selectedRow?.note || "");
  const sortFieldLabel = SORT_FIELD_OPTIONS.find((item) => item.value === sortField)?.label ?? "创建日期";
  const statusFilterLabel = STATUS_FILTER_OPTIONS.find((item) => item.value === statusFilter)?.label ?? "全部台账";
  const kindFilterLabel =
    kindFilter === "all" || kindFilter === "__tasks__" ? "全部类型" : KIND_CN[kindFilter] ?? kindFilter;
  const categoryFilterLabel =
    categoryFilter === "all" ? "全部导入资产" : importCategoryLabel(categoryFilter as Exclude<ImportDomainCategory, "all">);
  const wizardStepIndex = Math.max(
    0,
    WIZARD_STEPS.findIndex((step) => step.value === wizardStep)
  );
  const studioPhase: ImportStudioPhase =
    wizardStep === "summary"
      ? "completed"
      : wizardIsImporting
        ? "importing"
        : wizardStep === "mapping" || wizardStep === "validation"
          ? "review"
          : wizardFiles.length > 0 || previewing
            ? "staged"
            : "idle";
  const showJourney = wizardFiles.length > 0 || previewing || wizardStep !== "select";
  const dropzoneAnimationEmphasis: "idle" | "drag" = !desktopAvailable || wizardFiles.length > 0
    ? "idle"
    : dragActive
      ? "drag"
      : "idle";
  const batchSummary = {
    totalFiles: wizardFiles.length,
    totalRows: wizardFiles.reduce((sum, file) => sum + Math.max(0, file.rowsTotal), 0),
    structuredFiles: wizardFiles.filter((file) => file.selectedCategory === "structured").length,
    entityFiles: wizardFiles.filter((file) => file.selectedCategory === "entity").length,
    supportFiles: wizardFiles.filter((file) => file.selectedCategory === "support").length,
    reviewCount: wizardValidationRows.filter((item) => item.invalid || item.status === "review").length,
    readyCount: wizardValidationRows.filter((item) => !item.invalid).length
  };
  const currentBatchFileIds = useMemo(
    () => new Set((currentWizardJob?.files || []).map((file) => String(file.file_id || "").trim()).filter(Boolean)),
    [currentWizardJob]
  );
  const batchResultRows = useMemo(() => {
    if (currentBatchFileIds.size === 0) {
      return [];
    }
    return allConsoleRows.filter((row) => {
      const ledgerFileId = String(row.ledgerRow?.file_id || "").trim();
      return currentBatchFileIds.has(String(row.datasetId || "").trim()) || (ledgerFileId ? currentBatchFileIds.has(ledgerFileId) : false);
    });
  }, [allConsoleRows, currentBatchFileIds]);
  const batchResultOverview = useMemo(() => {
    const total = batchResultRows.length;
    const successCount = batchResultRows.filter((row) => row.statusText.startsWith("已完成") || row.statusText === "已入库").length;
    const attentionCount = batchResultRows.filter((row) => row.statusText === "失败" || row.statusText === "重复数据" || Boolean(row.detailText)).length;
    const importedRows = sumCompleteMetrics(batchResultRows.map((row) => row.validRows));
    const totalRows = sumCompleteMetrics(
      batchResultRows.map((row) => maxKnownMetric(row.validRows, row.rowsTotal))
    );
    const typeCount = new Set(batchResultRows.map((row) => row.kindLabel).filter(Boolean)).size;
    const latestImportedAt = batchResultRows.reduce((latest, row) => (toTs(row.importedAt) > toTs(latest) ? row.importedAt : latest), "");
    const successRate = total > 0 ? Math.round((successCount * 100) / total) : 0;
    return {
      total,
      successCount,
      attentionCount,
      importedRows,
      totalRows,
      typeCount,
      latestImportedAt,
      successRate
    };
  }, [batchResultRows]);
  const compactResultRows = batchResultRows.slice(0, 5);
  const hasBatchContext = batchSummary.totalFiles > 0 || Boolean(currentWizardJob);
  const batchArchiveChildCount = wizardFiles.reduce((sum, file) => sum + file.archiveChildren.length, 0);
  const workflowActive = previewing || hasBatchContext || wizardStep !== "select";
  const batchStatusSummary = useMemo(() => {
    if (batchSummary.totalFiles <= 0) {
      return "";
    }
    const parts = [`已接入 ${batchSummary.totalFiles.toLocaleString("zh-CN")} 个文件`];
    if (batchArchiveChildCount > 0) {
      parts.push(`子项数量 ${batchArchiveChildCount.toLocaleString("zh-CN")} 个`);
    }
    return parts.join("，");
  }, [batchArchiveChildCount, batchSummary.totalFiles]);
  const executionTableRows = useMemo<ExecutionTaskTableRow[]>(() => {
    const rows: ExecutionTaskTableRow[] = [];

    if (wizardProgressGroups.length > 0) {
      const totalFiles = wizardProgressGroups.length;
      const totalSize = wizardProgressGroups.reduce((sum, file) => sum + Math.max(0, Number(file.size) || 0), 0);
      const childCount = wizardProgressGroups.reduce((sum, file) => sum + file.childProgress.length, 0);
      const reviewCount = wizardProgressGroups.reduce(
        (sum, file) => sum + (file.status === "review" ? 1 : 0) + file.childProgress.filter((child) => child.status === "review").length,
        0
      );
      const activeCount = wizardProgressGroups.reduce(
        (sum, file) => sum + (file.status === "running" ? 1 : 0) + file.childProgress.filter((child) => child.status === "running").length,
        0
      );
      const failedCount = wizardProgressGroups.reduce(
        (sum, file) => sum + (file.status === "failed" ? 1 : 0) + file.childProgress.filter((child) => child.status === "failed").length,
        0
      );
      const succeededCount = wizardProgressGroups.reduce((sum, file) => {
        if (file.childProgress.length > 0) {
          return sum + file.childProgress.filter((child) => child.status === "succeeded").length;
        }
        return sum + (file.status === "succeeded" ? 1 : 0);
      }, 0);
      const progressPool = wizardProgressGroups.flatMap((file) =>
        file.childProgress.length > 0
          ? file.childProgress.map((child) => Math.max(0, Math.min(100, child.progress)))
          : [Math.max(0, Math.min(100, file.progress))]
      );
      const progressValue = progressPool.length
        ? Math.round(progressPool.reduce((sum, value) => sum + value, 0) / progressPool.length)
        : 0;
      const kindLabels = Array.from(
        new Set(
          wizardProgressGroups
            .map((file) => (file.selectedKind ? wizardFileKindLabel(file) : file.suggestedKindLabel || "待确认类型"))
            .filter(Boolean)
        )
      );

      let statusText = "等待中";
      let tone: ExecutionTaskTableRow["tone"] = "muted";
      if (failedCount > 0) {
        statusText = "失败";
        tone = "error";
      } else if (activeCount > 0 || wizardIsImporting) {
        statusText = "处理中";
        tone = "running";
      } else if (reviewCount > 0) {
        statusText = "待确认";
        tone = "warn";
      } else if (succeededCount > 0 && succeededCount === progressPool.length) {
        statusText = "已完成";
        tone = "success";
      }

      const sameType = kindLabels.length === 1 ? kindLabels[0] : "";
      const sampleNames = wizardProgressGroups.slice(0, 2).map((file) => file.fileName);
      const title =
        totalFiles === 1
          ? wizardProgressGroups[0]?.fileName || "当前导入任务"
          : sameType
            ? `${sameType}批量导入（${totalFiles.toLocaleString("zh-CN")} 个文件）`
            : `批量导入（${totalFiles.toLocaleString("zh-CN")} 个文件）`;
      const subtitle =
        totalFiles === 1
          ? childCount > 0
            ? `归档包 · ${childCount.toLocaleString("zh-CN")} 个子项${reviewCount > 0 ? ` · ${reviewCount.toLocaleString("zh-CN")} 个待确认` : ""}`
            : importCategoryLabel(wizardProgressGroups[0]?.selectedCategory ?? "support")
          : `${sampleNames.join("、")}${totalFiles > sampleNames.length ? ` 等 ${totalFiles.toLocaleString("zh-CN")} 个文件` : ""}`;
      const dateText = formatDateTime(currentWizardJob?.created_at || currentWizardJob?.updated_at || new Date().toISOString());
      const detailText = [
        `任务名称：${title}`,
        `任务说明：${subtitle}`,
        `数据类型：${sameType || `${kindLabels.length.toLocaleString("zh-CN")} 类数据`}`,
        `文件数量：${totalFiles.toLocaleString("zh-CN")} 个`,
        `子项数量：${childCount > 0 ? childCount.toLocaleString("zh-CN") : "—"}`,
        `当前状态：${statusText}`,
        `当前进度：${progressValue.toLocaleString("zh-CN")}%`,
        `总大小：${formatBytes(totalSize)}`,
      ].join("\n");

      rows.push({
        key: "live-batch",
        title,
        subtitle,
        dataType: sameType || (kindLabels.length > 0 ? `${kindLabels.length.toLocaleString("zh-CN")} 类数据` : "待识别"),
        submitter: "当前用户",
        dateText,
        sizeText: formatBytes(totalSize),
        statusText,
        tone,
        progressText: `${progressValue.toLocaleString("zh-CN")}%`,
        progressValue,
        detailText,
        rowPaths: wizardProgressGroups.map((file) => file.sourcePath).filter(Boolean),
        record: null,
        isLiveTask: true,
      });
    }

    rows.push(
      ...allConsoleRows.map((row) => {
        const progress = executionProgressMeta(row);
        const title = row.fileName;
        const subtitle = /\.(zip|rar|7z|tar|gz|tgz|bz2|tbz|xz|txz)$/i.test(row.fileName) ? "归档包任务" : row.categoryLabel;
        const detailText = [
          `文件名：${row.fileName}`,
          `任务说明：${subtitle}`,
          `数据类型：${row.kindLabel || row.categoryLabel}`,
          `提交人：${executionSubmitterLabel(row)}`,
          `日期：${formatDateTime(row.importedAt)}`,
          `大小：${formatKnownBytes(row.ledgerRow?.size ?? null)}`,
          `状态：${row.statusText}`,
          `进度：${progress.text}`,
          `总行数：${formatKnownCount(row.rowsTotal)}`,
          `有效记录：${formatKnownCount(row.validRows)}`,
          `导入路径：${row.path || "—"}`,
          `结果概况：${row.validRows !== null && row.validRows > 0 ? `${row.validRows.toLocaleString("zh-CN")} 条有效记录` : row.duplicateSummary || "—"}`,
          row.detailText ? `\n${row.detailText}` : "",
        ]
          .filter(Boolean)
          .join("\n");

        return {
          key: row.key,
          title,
          subtitle,
          dataType: row.kindLabel || row.categoryLabel,
          submitter: executionSubmitterLabel(row),
          dateText: formatDateTime(row.importedAt),
          sizeText: formatKnownBytes(row.ledgerRow?.size ?? null),
          statusText: row.statusText,
          tone: statusTone(row.statusText),
          progressText: progress.text,
          progressValue: progress.value,
          detailText,
          rowPaths: row.path ? [row.path] : [],
          record: row,
          isLiveTask: false,
        };
      })
    );

    return rows;
  }, [allConsoleRows, currentWizardJob, wizardIsImporting, wizardProgressGroups]);
  const stageCopy = useMemo<Record<
    ImportStudioPhase,
    {
      eyebrow: string;
      title: string;
      description: string;
    }
  >>(() => ({
    idle: {
      eyebrow: "导入工作区",
      title: "选择文件开始导入",
      description: "先完成文件预检、SHA-256 校验与解密，系统会自动识别类型，并为字段确认准备上下文。"
    },
    staged: {
      eyebrow: "文件预检",
      title: previewing ? "正在解析文件结构" : "文件预检已完成，工作台开始展开",
      description: previewing
        ? "系统正在校验 SHA-256、识别文件结构、检测加密状态，并为字段确认准备上下文。"
        : "上传区会继续变形为确认工作台，当前批次的文件、验密状态与识别结果都在这里原位展示。"
    },
    review: {
      eyebrow: wizardStep === "mapping" ? "字段确认" : "执行准备",
      title: wizardStep === "mapping" ? "确认字段匹配" : "准备执行入库",
      description:
        wizardStep === "mapping"
          ? "系统先给出字段与类型建议，人工只需要修正低置信度或关键字段缺失项。"
          : "继续保留同一工作台，字段确认完成后直接进入入库执行与过程反馈。"
    },
    importing: {
      eyebrow: "导入执行",
      title: "导入过程在当前舞台持续推进",
      description: "主舞台不会跳页，文件会逐步进入入库执行，并在同一视图里展示实时进度和问题反馈。"
    },
    completed: {
      eyebrow: "结果回看",
      title: "这一批次已进入结果回看",
      description: "主舞台收束为导入结果概览，底部结果区继续承接完整总表与后续追踪。"
    }
  }), [previewing, wizardStep]);
  const journeySteps = useMemo(() => WIZARD_STEPS.map((step, index) => {
    const enabled =
      step.value === "select" ||
      (step.value === "mapping" && wizardCanGoMapping) ||
      (step.value === "validation" && wizardCanGoValidation) ||
      (step.value === "summary" && wizardCanGoSummary);
    const active = wizardStep === step.value;
    const complete = index < wizardStepIndex;

    let detail = "";
    if (step.value === "select") {
      detail = wizardFiles.length > 0 ? `${wizardFiles.length} 个文件` : "等待预检";
    } else if (step.value === "mapping") {
      detail =
        wizardValidationRows.length > 0
          ? `${wizardValidationRows.filter((item) => !item.invalid).length}/${wizardValidationRows.length} 已确认`
          : "等待确认";
    } else if (step.value === "validation") {
      detail = currentWizardJob ? `进度 ${Math.max(0, toInt(currentWizardJob.progress))}%` : "待开始";
    } else {
      detail = batchResultOverview.total > 0 ? `${batchResultOverview.total} 个结果` : currentWizardJob ? "结果生成中" : "结果待生成";
    }

    return {
      ...step,
      enabled,
      active,
      complete,
      detail
    };
  }), [
    batchResultOverview.total,
    currentWizardJob,
    wizardCanGoMapping,
    wizardCanGoSummary,
    wizardCanGoValidation,
    wizardFiles.length,
    wizardStep,
    wizardStepIndex,
    wizardValidationRows,
  ]);
  const workflowSwitchOptions = useMemo(
    () =>
      journeySteps.map((step, index) => ({
        value: step.value,
        disabled: !step.enabled,
        label: (
          <span className={`import-console-workflow-switch__option-copy${step.complete ? " is-complete" : ""}`}>
            <span className="import-console-workflow-switch__option-index">{step.complete ? "✓" : index + 1}</span>
            <span className="import-console-workflow-switch__option-text">
              <strong>{step.label}</strong>
              <small>{step.detail}</small>
            </span>
          </span>
        )
      })),
    [journeySteps]
  );
  const showResultPreview = false;
  const showResultLedger = wizardCanGoSummary && wizardStep === "summary";
  const isSelectMode = wizardStep === "select";
  const showExecutionOverview = isSelectMode && !workflowActive && executionTableRows.length > 0;
  const shouldUseSelectOverviewDock = false;
  const shouldRenderPinnedImportStage = active && showExecutionOverview && shouldUseSelectOverviewDock;
  const ledgerActionDialogRows = ledgerActionDialogState?.rows ?? [];
  const ledgerActionDialogMode = ledgerActionDialogState?.mode ?? "recycle";
  const ledgerActionDialogCount = ledgerActionDialogRows.length;
  const ledgerActionDialogValidRowEstimate = completeImpactEstimate(
    ledgerActionDialogRows.map((row) => row.valid_rows)
  );
  const ledgerActionDialogBlocksPurge =
    ledgerActionDialogMode === "purge" && ledgerActionDialogValidRowEstimate.status !== "complete";
  const ledgerActionDialogPreviewRows = ledgerActionDialogRows.slice(0, 5);
  const ledgerActionDialogTitle =
    ledgerActionDialogMode === "recycle" ? "确认移入回收站" : ledgerActionDialogMode === "restore" ? "确认恢复" : "确认彻底删除";
  const ledgerActionDialogSubtitle =
    ledgerActionDialogMode === "recycle"
      ? "将从结果总表和业务表移出，并保留在回收站中以便恢复"
      : ledgerActionDialogMode === "restore"
        ? "将从回收站恢复到结果总表，并恢复关联 DuckDB 数据"
        : "将永久清理回收站记录、DuckDB 原始库/清洗库数据及相关索引（不可恢复）";
  const ledgerActionConfirmLabel =
    ledgerActionDialogMode === "recycle" ? "确认移入回收站" : ledgerActionDialogMode === "restore" ? "确认恢复" : "确认彻底删除";
  const ledgerTitle = ledgerView === "recycle" ? "导入回收站" : "结果总表";
  const ledgerTimeColumnLabel = ledgerView === "recycle" ? "回收时间" : "导入时间";
  const ledgerToolbarSearchPlaceholder = ledgerView === "recycle" ? "搜索文件名 / SHA-256 / 回收路径" : "搜索文件名 / SHA-256 / 导入路径";
  const ledgerPrimaryActionLabel = ledgerView === "recycle" ? "恢复" : "移入回收站";
  const showLedgerEmptyMessage = ledgerView !== "recycle" && consoleRows.length === 0;
  const showLedgerPlaceholderRows = ledgerView === "recycle" && consoleRows.length === 0;
  const ledgerPlaceholderRows = showLedgerPlaceholderRows ? Array.from({ length: 4 }, (_, index) => `placeholder-${index}`) : [];

  useLayoutEffect(() => {
    if (wizardStep !== "summary") {
      return;
    }
    const frame = window.requestAnimationFrame(() => {
      const pageNode = pageRef.current;
      const scrollHost = resolveImportScrollHost(pageNode);
      const ledgerWrap = pageNode?.querySelector(".import-console-table-wrap");
      if (scrollHost instanceof HTMLElement) {
        scrollHost.scrollTop = 0;
      }
      if (ledgerWrap instanceof HTMLElement) {
        ledgerWrap.scrollTop = 0;
        ledgerWrap.scrollLeft = 0;
      }
    });
    return () => {
      window.cancelAnimationFrame(frame);
    };
  }, [wizardStep]);

  useLayoutEffect(() => {
    const pageNode = pageRef.current;
    const mainNode = resolveImportScrollHost(pageNode);
    const heroNode = pageNode?.querySelector(".import-console-topbar-spacer");
    const overviewNode = pageNode?.querySelector(".import-console-execution-overview");
    const heroElement = heroNode instanceof HTMLElement ? heroNode : null;
    const overviewElement = overviewNode instanceof HTMLElement ? overviewNode : null;
    const scrollContainer = mainNode instanceof HTMLElement ? mainNode : null;
    const stageScene = stageSceneRef.current;
    const stageElement = stageRef.current;
    if (!active || !pageNode || !shouldRenderPinnedImportStage || !heroElement || !overviewElement || !scrollContainer || !stageScene || !stageElement) {
      return;
    }
    const pageStyles = window.getComputedStyle(pageNode);
    const basePaddingBottom = parseFloat(pageStyles.getPropertyValue("--space-5")) || 24;
    const dockGap = IMPORT_PAGE_OVERVIEW_DOCK_GAP * viewportScale;
    const stageAnchor = 74 * viewportScale;
    let rafId = 0;
    let scrollCorrectionFrame = 0;
    const syncOverviewLayout = (): void => {
      rafId = 0;
      if (scrollCorrectionFrame !== 0) {
        window.cancelAnimationFrame(scrollCorrectionFrame);
        scrollCorrectionFrame = 0;
      }
      const previousScrollTop = scrollContainer.scrollTop;
      const previousMaxScrollTop = Math.max(0, scrollContainer.scrollHeight - scrollContainer.clientHeight);
      const wasAtTop = previousScrollTop <= 2;
      const heroHeight = Math.round(heroElement.getBoundingClientRect().height);
      const dockTop = heroHeight + dockGap;
      const overviewStaticTop = pageNode.offsetTop + overviewElement.offsetTop;
      const previousDockScrollTop = clampNumber(overviewStaticTop - dockTop, 0, previousMaxScrollTop);
      const wasDocked = previousScrollTop >= previousDockScrollTop - IMPORT_PAGE_SELECT_SCROLL_TOLERANCE;
      const previousProgress =
        previousDockScrollTop > IMPORT_PAGE_SELECT_SCROLL_TOLERANCE
          ? clampNumber(previousScrollTop / previousDockScrollTop, 0, 1)
          : previousScrollTop > 0
            ? 1
            : 0;
      const previousDockOverflow = wasDocked ? Math.max(0, previousScrollTop - previousDockScrollTop) : 0;
      const wasIntermediate = !wasAtTop && !wasDocked && previousDockScrollTop > 0;
      const stageRect = stageScene.getBoundingClientRect();
      const stageHeight = Math.ceil(stageElement.getBoundingClientRect().height);
      pageNode.style.setProperty("--import-select-stage-fixed-top", `${stageAnchor}px`);
      pageNode.style.setProperty("--import-select-stage-fixed-left", `${Math.round(stageRect.left)}px`);
      pageNode.style.setProperty("--import-select-stage-fixed-width", `${Math.round(stageRect.width)}px`);
      pageNode.style.setProperty("--import-select-stage-scene-height", `${stageHeight}px`);

      const appliedRunway = parseFloat(window.getComputedStyle(pageNode).getPropertyValue("--import-select-scroll-runway")) || 0;
      const extraRunway = Math.max(0, appliedRunway - basePaddingBottom);
      // The runway itself increases scrollHeight, so remove the previously injected
      // extra space before solving for the next runway value.
      const availableScroll = Math.max(0, scrollContainer.scrollHeight - scrollContainer.clientHeight - extraRunway);
      const totalPaddingBottom = Math.max(
        basePaddingBottom,
        Math.ceil(overviewStaticTop - dockTop - availableScroll + basePaddingBottom)
      );

      pageNode.style.setProperty("--import-select-overview-anchor", `${heroHeight}px`);
      pageNode.style.setProperty("--import-select-scroll-runway", `${totalPaddingBottom}px`);

      if (wasAtTop || wasDocked || wasIntermediate) {
        scrollCorrectionFrame = window.requestAnimationFrame(() => {
          scrollCorrectionFrame = 0;
          const nextMaxScrollTop = Math.max(0, scrollContainer.scrollHeight - scrollContainer.clientHeight);
          const nextDockScrollTop = clampNumber(overviewStaticTop - dockTop, 0, nextMaxScrollTop);
          if (wasAtTop) {
            scrollContainer.scrollTop = 0;
            return;
          }
          if (wasDocked) {
            scrollContainer.scrollTop = clampNumber(nextDockScrollTop + previousDockOverflow, 0, nextMaxScrollTop);
            return;
          }
          scrollContainer.scrollTop = clampNumber(nextDockScrollTop * previousProgress, 0, nextMaxScrollTop);
        });
      }
    };

    const requestSync = (): void => {
      if (rafId !== 0) {
        return;
      }
      rafId = window.requestAnimationFrame(syncOverviewLayout);
    };

    const observer = typeof ResizeObserver !== "undefined" ? new ResizeObserver(requestSync) : null;
    observer?.observe(pageNode);
    observer?.observe(scrollContainer);
    observer?.observe(heroElement);
    observer?.observe(overviewElement);
    observer?.observe(stageScene);
    observer?.observe(stageElement);
    syncOverviewLayout();
    const handleWindowLoad = (): void => {
      requestSync();
    };
    if (document.readyState === "complete") {
      requestSync();
    } else {
      window.addEventListener("load", handleWindowLoad);
    }
    let fontsCanceled = false;
    if ("fonts" in document) {
      void (document as Document & { fonts?: { ready: Promise<unknown> } }).fonts?.ready.then(() => {
        if (!fontsCanceled) {
          requestSync();
        }
      });
    }
    window.addEventListener("resize", requestSync);

    return () => {
      if (rafId !== 0) {
        window.cancelAnimationFrame(rafId);
      }
      if (scrollCorrectionFrame !== 0) {
        window.cancelAnimationFrame(scrollCorrectionFrame);
      }
      fontsCanceled = true;
      window.removeEventListener("load", handleWindowLoad);
      observer?.disconnect();
      window.removeEventListener("resize", requestSync);
    };
  }, [active, executionTableRows.length, shouldRenderPinnedImportStage, viewportScale, wizardFiles.length]);

  useLayoutEffect(() => {
    const pageNode = pageRef.current;
    const scrollHost = resolveImportScrollHost(pageNode);
    const heroNode = pageNode?.querySelector(".import-console-topbar-spacer");
    const overviewNode = pageNode?.querySelector(".import-console-execution-overview");
    const tableWrapNode = pageNode?.querySelector(".import-console-execution-table-wrap");
    const heroElement = heroNode instanceof HTMLElement ? heroNode : null;
    const overviewElement = overviewNode instanceof HTMLElement ? overviewNode : null;
    const tableWrap = tableWrapNode instanceof HTMLElement ? tableWrapNode : null;
    if (
      !active ||
      !isSelectMode ||
      !shouldRenderPinnedImportStage ||
      !(pageNode instanceof HTMLElement) ||
      !(scrollHost instanceof HTMLElement) ||
      !(heroElement instanceof HTMLElement) ||
      !(overviewElement instanceof HTMLElement)
    ) {
      return;
    }

    const stopSceneSnapAnimation = (): void => {
      if (selectSnapAnimationFrameRef.current !== null) {
        window.cancelAnimationFrame(selectSnapAnimationFrameRef.current);
        selectSnapAnimationFrameRef.current = null;
      }
      selectSnapBusyRef.current = false;
    };

    const syncDockEntryGuardState = (): void => {
      pageNode.dataset.importSelectEntryGuard = selectDockEntryGuardPendingRef.current
        ? "pending"
        : selectDockEntryGuardActiveRef.current
          ? "active"
          : "released";
    };

    const releaseDockEntryGuard = (): void => {
      selectDockEntryGuardActiveRef.current = false;
      selectDockEntryGuardPendingRef.current = false;
      selectDockEntryGuardLastWheelAtRef.current = 0;
      syncDockEntryGuardState();
    };

    const scheduleDockEntryGuard = (): void => {
      selectDockEntryGuardPendingRef.current = true;
      selectDockEntryGuardActiveRef.current = false;
      selectDockEntryGuardLastWheelAtRef.current = 0;
      syncDockEntryGuardState();
    };

    const engageDockEntryGuard = (now: number): void => {
      selectDockEntryGuardPendingRef.current = false;
      selectDockEntryGuardActiveRef.current = true;
      selectDockEntryGuardLastWheelAtRef.current = now;
      syncDockEntryGuardState();
    };

    const shouldAbsorbDockEntryWheel = (deltaY: number, now: number): boolean => {
      if (deltaY <= 0) {
        releaseDockEntryGuard();
        return false;
      }
      if (!selectDockEntryGuardActiveRef.current) {
        return false;
      }
      if (now - selectDockEntryGuardLastWheelAtRef.current <= IMPORT_PAGE_SELECT_DOCK_ENTRY_GUARD_MS) {
        selectDockEntryGuardLastWheelAtRef.current = now;
        syncDockEntryGuardState();
        return true;
      }
      releaseDockEntryGuard();
      return false;
    };

    const syncDockLockState = (): void => {
      pageNode.dataset.importSelectLockState = selectDockUnlockArmedRef.current
        ? "armed"
        : selectDockLockRef.current
          ? "locked"
          : "released";
    };

    const releaseDockLock = (): void => {
      selectDockLockRef.current = false;
      selectDockUnlockArmedRef.current = false;
      syncDockLockState();
    };

    const engageDockLock = (): void => {
      selectDockLockRef.current = true;
      selectDockUnlockArmedRef.current = false;
      syncDockLockState();
    };

    const clearDockUnlockArm = (): void => {
      if (!selectDockUnlockArmedRef.current) {
        if (!selectDockLockRef.current) {
          syncDockLockState();
        }
        return;
      }
      selectDockUnlockArmedRef.current = false;
      syncDockLockState();
    };

    const armDockUnlock = (): void => {
      selectDockLockRef.current = true;
      selectDockUnlockArmedRef.current = true;
      syncDockLockState();
    };

    const resolveDockedScrollTop = (): number => {
      const maxScrollTop = Math.max(0, scrollHost.scrollHeight - scrollHost.clientHeight);
      const dockGap = IMPORT_PAGE_OVERVIEW_DOCK_GAP * viewportScale;
      const dockTop = Math.round(heroElement.getBoundingClientRect().height + dockGap);
      const overviewStaticTop = pageNode.offsetTop + overviewElement.offsetTop;
      return clampNumber(overviewStaticTop - dockTop, 0, maxScrollTop);
    };

    const syncSnapProgress = (): {
      maxScrollTop: number;
      dockScrollTop: number;
      isDocked: boolean;
      isAtDockBoundary: boolean;
    } => {
      const maxScrollTop = Math.max(0, scrollHost.scrollHeight - scrollHost.clientHeight);
      const dockScrollTop = resolveDockedScrollTop();
      let currentScrollTop = scrollHost.scrollTop;
      const now = performance.now();
      if (
        selectDockEntryGuardActiveRef.current &&
        now - selectDockEntryGuardLastWheelAtRef.current > IMPORT_PAGE_SELECT_DOCK_ENTRY_GUARD_MS
      ) {
        releaseDockEntryGuard();
      }
      if (
        selectDockEntryGuardActiveRef.current &&
        currentScrollTop > dockScrollTop + IMPORT_PAGE_SELECT_SCROLL_TOLERANCE
      ) {
        scrollHost.scrollTop = dockScrollTop;
        currentScrollTop = dockScrollTop;
        selectDockEntryGuardLastWheelAtRef.current = now;
        syncDockEntryGuardState();
      }
      if (
        selectDockEntryGuardActiveRef.current &&
        tableWrap &&
        tableWrap.scrollTop > IMPORT_PAGE_SELECT_SCROLL_TOLERANCE
      ) {
        tableWrap.scrollTop = 0;
        selectDockEntryGuardLastWheelAtRef.current = now;
        syncDockEntryGuardState();
      }
      const progress =
        dockScrollTop > IMPORT_PAGE_SELECT_SCROLL_TOLERANCE
          ? clampNumber(currentScrollTop / dockScrollTop, 0, 1)
          : currentScrollTop > 0
            ? 1
            : 0;
      const isDocked = currentScrollTop >= dockScrollTop - IMPORT_PAGE_SELECT_SCROLL_TOLERANCE;
      const isAtDockBoundary = Math.abs(currentScrollTop - dockScrollTop) <= IMPORT_PAGE_SELECT_SCROLL_TOLERANCE;
      pageNode.style.setProperty("--import-select-snap-progress", progress.toFixed(4));
      pageNode.dataset.importSelectDocked = isDocked ? "true" : "false";
      if (isDocked) {
        if (selectDockEntryGuardPendingRef.current) {
          engageDockEntryGuard(now);
        } else {
          syncDockEntryGuardState();
        }
        if (!selectDockLockRef.current) {
          engageDockLock();
        } else {
          syncDockLockState();
        }
      } else if (scrollHost.scrollTop <= dockScrollTop - IMPORT_PAGE_SELECT_SCROLL_TOLERANCE) {
        if (selectDockEntryGuardPendingRef.current) {
          syncDockEntryGuardState();
        } else {
          releaseDockEntryGuard();
        }
        releaseDockLock();
      } else if (selectDockEntryGuardPendingRef.current || selectDockEntryGuardActiveRef.current) {
        syncDockEntryGuardState();
      }
      return { maxScrollTop, dockScrollTop, isDocked, isAtDockBoundary };
    };

    const scrollElementBy = (node: HTMLElement | null, deltaY: number, min = 0, max?: number): number => {
      if (!(node instanceof HTMLElement)) {
        return 0;
      }
      const limit = Math.max(0, node.scrollHeight - node.clientHeight);
      if (limit <= 0) {
        return 0;
      }
      const lowerBound = clampNumber(min, 0, limit);
      const upperBound = clampNumber(max ?? limit, lowerBound, limit);
      const start = clampNumber(node.scrollTop, lowerBound, upperBound);
      const next = clampNumber(start + deltaY, lowerBound, upperBound);
      if (Math.abs(next - start) <= 0.5) {
        return 0;
      }
      node.scrollTop = next;
      return next - start;
    };

    const scrollTableBy = (deltaY: number): number => {
      if (!tableWrap || pageNode.dataset.importSelectDocked !== "true") {
        return 0;
      }
      const moved = scrollElementBy(tableWrap, deltaY);
      if (Math.abs(moved) > 0.5) {
        clearDockUnlockArm();
      }
      return moved;
    };

    const scrollPageBeyondDockBy = (deltaY: number, dockScrollTop: number, maxScrollTop: number): number => {
      const moved = scrollElementBy(scrollHost, deltaY, dockScrollTop, maxScrollTop);
      if (Math.abs(moved) > 0.5) {
        clearDockUnlockArm();
      }
      return moved;
    };

    const snapSelectScene = (target: number): void => {
      const maxScrollTop = Math.max(0, scrollHost.scrollHeight - scrollHost.clientHeight);
      const boundedTarget = clampNumber(target, 0, maxScrollTop);
      const start = scrollHost.scrollTop;
      const delta = boundedTarget - start;
      if (Math.abs(delta) < 1) {
        scrollHost.scrollTop = boundedTarget;
        syncSnapProgress();
        selectSnapBusyRef.current = false;
        return;
      }

      stopSceneSnapAnimation();
      selectSnapBusyRef.current = true;
      const duration = 240;
      const warmStart = 0.08;
      const easeOutCubic = (value: number): number => 1 - (1 - value) ** 3;
      const initialProgress = easeOutCubic(warmStart);
      scrollHost.scrollTop = start + delta * initialProgress;
      syncSnapProgress();
      const startTime = performance.now() - duration * warmStart;

      const step = (now: number): void => {
        const elapsed = Math.min(1, (now - startTime) / duration);
        const eased = easeOutCubic(elapsed);
        scrollHost.scrollTop = start + delta * eased;
        syncSnapProgress();
        if (elapsed < 1) {
          selectSnapAnimationFrameRef.current = window.requestAnimationFrame(step);
          return;
        }
        scrollHost.scrollTop = boundedTarget;
        syncSnapProgress();
        selectSnapAnimationFrameRef.current = null;
        selectSnapBusyRef.current = false;
      };

      selectSnapAnimationFrameRef.current = window.requestAnimationFrame(step);
    };

    const handleSelectSceneWheel = (event: WheelEvent): void => {
      if (event.ctrlKey || event.deltaY === 0) {
        return;
      }
      if (!(event.target instanceof Node) || !pageNode.contains(event.target)) {
        return;
      }
      const { maxScrollTop, dockScrollTop, isDocked } = syncSnapProgress();
      if (
        maxScrollTop <= IMPORT_PAGE_SELECT_SCROLL_TOLERANCE ||
        dockScrollTop <= IMPORT_PAGE_SELECT_SCROLL_TOLERANCE ||
        !Number.isFinite(maxScrollTop) ||
        !Number.isFinite(dockScrollTop)
      ) {
        releaseDockEntryGuard();
        releaseDockLock();
        return;
      }

      if (selectSnapBusyRef.current) {
        event.preventDefault();
        return;
      }

      const isAtTop = scrollHost.scrollTop <= IMPORT_PAGE_SELECT_SCROLL_TOLERANCE;
      const now = performance.now();

      if (isDocked && shouldAbsorbDockEntryWheel(event.deltaY, now)) {
        event.preventDefault();
        if (tableWrap) {
          tableWrap.scrollTop = 0;
        }
        if (scrollHost.scrollTop > dockScrollTop + IMPORT_PAGE_SELECT_SCROLL_TOLERANCE) {
          scrollHost.scrollTop = dockScrollTop;
        }
        syncSnapProgress();
        return;
      }

      if (event.deltaY > 0) {
        clearDockUnlockArm();
        if (!isDocked) {
          scheduleDockEntryGuard();
          event.preventDefault();
          snapSelectScene(dockScrollTop);
          return;
        }
        event.preventDefault();
        let remainingDelta = event.deltaY;
        remainingDelta -= scrollTableBy(remainingDelta);
        if (remainingDelta > IMPORT_PAGE_SELECT_SCROLL_TOLERANCE) {
          scrollPageBeyondDockBy(remainingDelta, dockScrollTop, maxScrollTop);
        }
        syncSnapProgress();
        return;
      }

      releaseDockEntryGuard();

      event.preventDefault();

      if (isDocked) {
        let remainingDelta = event.deltaY;
        const movedTable = scrollTableBy(remainingDelta);
        remainingDelta -= movedTable;
        let movedPage = 0;
        if (remainingDelta < -IMPORT_PAGE_SELECT_SCROLL_TOLERANCE) {
          movedPage = scrollPageBeyondDockBy(remainingDelta, dockScrollTop, maxScrollTop);
          remainingDelta -= movedPage;
        }
        const afterScrollState = syncSnapProgress();
        if (
          Math.abs(movedTable) > 0.5 ||
          Math.abs(movedPage) > 0.5 ||
          !afterScrollState.isAtDockBoundary
        ) {
          clearDockUnlockArm();
          return;
        }
      }

      if (isDocked && selectDockLockRef.current) {
        if (!selectDockUnlockArmedRef.current) {
          armDockUnlock();
          return;
        }
        releaseDockLock();
      }

      if (!isAtTop) {
        if (tableWrap) {
          tableWrap.scrollTop = 0;
        }
        snapSelectScene(0);
      }
    };

    syncSnapProgress();
    window.addEventListener("resize", syncSnapProgress);
    window.addEventListener("wheel", handleSelectSceneWheel, { passive: false, capture: true });
    scrollHost.addEventListener("scroll", syncSnapProgress, { passive: true });

    return () => {
      window.removeEventListener("resize", syncSnapProgress);
      window.removeEventListener("wheel", handleSelectSceneWheel, true);
      scrollHost.removeEventListener("scroll", syncSnapProgress);
      stopSceneSnapAnimation();
      releaseDockEntryGuard();
      releaseDockLock();
      pageNode.style.removeProperty("--import-select-snap-progress");
      delete pageNode.dataset.importSelectDocked;
      delete pageNode.dataset.importSelectEntryGuard;
      delete pageNode.dataset.importSelectLockState;
    };
  }, [active, isSelectMode, shouldRenderPinnedImportStage, viewportScale]);

  const buildSortMenuItems = (field: SortField | null): ContextMenuItem[] => {
    if (!field) {
      return [];
    }
    return [
      {
        id: `sort:${field}:asc`,
        label: "按该列升序",
        onSelect: () => {
          setSortField(field);
          setSortDirection("asc");
        }
      },
      {
        id: `sort:${field}:desc`,
        label: "按该列降序",
        onSelect: () => {
          setSortField(field);
          setSortDirection("desc");
        }
      }
    ];
  };

  const openHeaderContextMenu = (event: MouseEvent, field: SortField | null): void => {
    event.preventDefault();
    const items = buildSortMenuItems(field);
    if (items.length === 0) {
      return;
    }
    setContextMenuAnchor({ x: event.clientX, y: event.clientY });
    setContextMenuItems(items);
  };

  const openRowContextMenu = (event: MouseEvent, row: LedgerRow, field: SortField | null): void => {
    event.preventDefault();
    const warnText = (row.error || row.note || "").trim();
    const actions: ContextMenuItem[] = [
      {
        id: `copy-sha:${row.key}`,
        label: "复制 SHA-256",
        disabled: !row.sha256,
        onSelect: () => {
          void onCopyText(row.sha256, "SHA-256");
        }
      },
      {
        id: `copy-path:${row.key}`,
        label: "复制来源路径",
        disabled: !row.display_path,
        onSelect: () => {
          void onCopyText(row.display_path, "来源路径");
        }
      },
      {
        id: `open-folder:${row.key}`,
        label: "打开缓存文件夹",
        disabled: !(row.stored_path || row.display_path),
        onSelect: () => {
          void onOpenRowFolder(row);
        }
      }
    ];

    if (warnText) {
      actions.push({
        id: `view-error:${row.key}`,
        label: row.status_text === "失败" ? "查看失败原因" : "查看导入提示",
        onSelect: () => {
          setDetailDialog({
            title: row.status_text === "失败" ? "导入失败原因" : "导入提示",
            text: warnText.length > 2200 ? `${warnText.slice(0, 2200)}\n...（已截断）` : warnText
          });
        }
      });
    }

    if (ledgerView === "recycle") {
      actions.push(
        {
          id: `restore:${row.key}`,
          label: "恢复到结果总表",
          onSelect: () => {
            openLedgerActionDialog("restore", [row]);
          }
        },
        {
          id: `purge:${row.key}`,
          label: "彻底删除",
          danger: true,
          onSelect: () => {
            openLedgerActionDialog("purge", [row]);
          }
        }
      );
    } else {
      actions.push({
        id: `recycle:${row.key}`,
        label: "移入回收站",
        danger: true,
        disabled: !canApplyLedgerAction(row, "recycle"),
        onSelect: () => {
          openLedgerActionDialog("recycle", [row]);
        }
      });
    }

    const sortItems = buildSortMenuItems(field);
    setContextMenuAnchor({ x: event.clientX, y: event.clientY });
    setContextMenuItems([...actions, ...sortItems]);
  };

  const openRowDetailDialog = (row: LedgerRow): void => {
    const text = trimDetailText(row.error || row.note || "");
    if (!text) {
      return;
    }
    setDetailDialog({
      title: row.status_text === "失败" ? "导入失败原因" : "导入提示",
      text
    });
  };

  const openExecutionRowPreview = useCallback((row: ExecutionTaskTableRow): void => {
    setDetailDialog({
      title: row.title,
      text: row.detailText,
    });
  }, []);

  const openWizardProgressDetail = useCallback((file: ImportWizardProgressGroup): void => {
    const detailLines = [
      `文件名：${file.fileName}`,
      `数据分类：${importCategoryLabel(file.selectedCategory)}`,
      `入库类型：${wizardFileKindLabel(file)}`,
      `状态：${wizardFileStateLabel(file)}`,
      `进度：${Math.max(0, Math.min(100, file.progress)).toLocaleString("zh-CN")}%`,
      `文件大小：${formatBytes(file.size)}`,
      `记录规模：${file.rowsTotal > 0 ? `${file.rowsTotal.toLocaleString("zh-CN")} 行` : file.isArchive ? "归档型批量导入" : "文件登记型导入"}`,
      `来源路径：${file.sourcePath || "—"}`,
      `当前说明：${file.issue || file.message || "系统正在推进当前文件入库。"}`
    ];

    if (file.childProgress.length > 0) {
      detailLines.push("", "子任务明细：");
      file.childProgress.forEach((child, index) => {
        detailLines.push(
          `${index + 1}. ${child.fileName}`,
          `   类型：${child.kindLabel || child.suggestedKindLabel || "待确认"}`,
          `   状态：${wizardFileStateLabel(child)}`,
          `   进度：${Math.max(0, Math.min(100, child.progress)).toLocaleString("zh-CN")}%`,
          `   说明：${child.issue || previewDetectedByLabel(child.detectedBy) || "正在等待执行"}`
        );
      });
    }

    setDetailDialog({
      title: `${file.fileName} · 执行详情`,
      text: detailLines.join("\n")
    });
  }, []);

  const openWizardProgressChildDetail = useCallback((file: ImportWizardProgressGroup, child: ImportWizardProgressChild): void => {
    const detailLines = [
      `父级文件：${file.fileName}`,
      `子项文件：${child.fileName}`,
      `归档路径：${child.archivePath || "—"}`,
      `数据分类：${importCategoryLabel(child.domainCategory)}`,
      `入库类型：${child.kindLabel || child.suggestedKindLabel || wizardArchiveChildKindLabel(child)}`,
      `状态：${validationExecutionLabel(child.status, Boolean(currentWizardJob))}`,
      `进度：${validationExecutionProgress(child.status, child.progress, Boolean(currentWizardJob)).toLocaleString("zh-CN")}%`,
      `文件大小：${formatBytes(child.size)}`,
      `记录规模：${child.rowsTotal > 0 ? `${child.rowsTotal.toLocaleString("zh-CN")} 行` : child.columnsTotal > 0 ? `${child.columnsTotal.toLocaleString("zh-CN")} 列` : "—"}`,
      `SHA-256：${child.sha256 || "—"}`,
      `当前说明：${child.issue || validationProgressMessageForChild(child, Boolean(currentWizardJob))}`,
    ];

    setDetailDialog({
      title: `${child.fileName} · 执行详情`,
      text: detailLines.join("\n"),
    });
  }, [currentWizardJob]);

  const onDownloadExecutionRow = useCallback(
    async (_row: ExecutionTaskTableRow): Promise<void> => {
      const reason = controlledSourceIngestionBlockReason();
      setError(reason);
      showToast({ tone: "warning", title: "受控来源访问已阻止", detail: reason });
    },
    []
  );

  const onRetryExecutionRow = useCallback(
    async (_row: ExecutionTaskTableRow): Promise<void> => {
      const reason = controlledSourceIngestionBlockReason();
      setError(reason);
      showToast({
        tone: "warning",
        title: "受控来源访问已阻止",
        detail: reason,
      });
    },
    []
  );

  const importPageStyle = useMemo(
    () =>
      ({
        "--import-page-scale": viewportScale.toFixed(4),
        "--import-page-min-content-width": `${IMPORT_PAGE_MIN_CONTENT_WIDTH}px`,
        "--import-page-min-scale": IMPORT_PAGE_MIN_SCALE.toFixed(2),
        "--import-select-stage-fixed-top": `calc(74px * ${viewportScale.toFixed(4)})`,
        "--import-select-stage-fixed-left": "0px",
        "--import-select-stage-fixed-width": "100%",
        "--import-select-stage-scene-height": "auto",
      }) as CSSProperties,
    [viewportScale]
  );
  const executionTableLayout = useMemo(() => {
    const layoutScale = Math.max(IMPORT_PAGE_MIN_SCALE, viewportScale);
    const widths = [248, 136, 148, 96, 138, 192, 152].map((width) => Math.round(width * layoutScale));
    return {
      widths,
      minWidth: widths.reduce((sum, width) => sum + width, 0),
    };
  }, [viewportScale]);
  const detailDialogWidth = useMemo(() => Math.round(760 * viewportScale), [viewportScale]);
  const deleteDialogWidth = useMemo(() => Math.round(620 * viewportScale), [viewportScale]);
  const previewQueueStatus = useMemo(() => {
    if (!previewing) {
      return null;
    }
    const totalFiles = previewBatchStats.totalCount > 0 ? previewBatchStats.totalCount : batchSummary.totalFiles;
    const importedFiles = Math.min(totalFiles, Math.max(0, previewBatchStats.importedCount));
    return {
      totalFiles,
      importedFiles
    };
  }, [batchSummary.totalFiles, previewBatchStats, previewing]);
  const renderPreviewQueueStatus = (className?: string) =>
    previewQueueStatus ? (
      <div className={`import-console-dropzone__queue-progress${className ? ` ${className}` : ""}`} role="status" aria-live="polite">
        <span className="import-console-dropzone__queue-progress-icon" aria-hidden="true">
          <svg viewBox="0 0 24 24" focusable="false">
            <circle cx="12" cy="12" r="8.25" fill="none" opacity="0.2" stroke="currentColor" strokeWidth="1.6" />
            <path
              d="M12 3.75a8.25 8.25 0 0 1 7.78 5.52"
              fill="none"
              stroke="currentColor"
              strokeLinecap="round"
              strokeWidth="2.3"
            />
          </svg>
        </span>
        <div className="import-console-dropzone__queue-progress-copy">
          <strong>正在导入数据</strong>
          <span>
            总数 {previewQueueStatus.totalFiles.toLocaleString("zh-CN")} 个文件 · 已接入 {previewQueueStatus.importedFiles.toLocaleString("zh-CN")} 个文件
          </span>
        </div>
      </div>
    ) : null;
  const importedFileCountLabel = wizardFiles.length.toLocaleString("zh-CN");

  return (
    <section
      ref={pageRef}
      className={`page-wrap stack import-workbench-page import-workbench-page--proportional${isSelectMode ? " is-import-select-mode" : ""}${showExecutionOverview ? " has-execution-overview" : ""}`}
      data-import-select-stage-pinned={shouldRenderPinnedImportStage ? "true" : undefined}
      style={importPageStyle}
    >
      <div className="import-console-topbar-spacer" aria-hidden="true" />

      {exportDialog?.open ? (
        <section className="import-console-strip">
          <div className="import-console-strip__copy">
            <span className={`import-workbench-inline-status is-${exportDialog.status}`}>{exportDialog.message || "导出中"}</span>
            <strong>{exportDialog.tableLabel || "等待导出"}</strong>
            <span>
              {toInt(exportDialog.tableDone).toLocaleString("zh-CN")} / {toInt(exportDialog.tableTotal).toLocaleString("zh-CN")} 行
            </span>
            {exportDialog.outputPath ? <span className="import-workbench-code-line">{exportDialog.outputPath}</span> : null}
          </div>
          <div className="import-console-strip__actions">
            {exportDialog.status === "running" ? (
              <WorkbenchButton size="sm" tone="danger" type="button" onClick={() => void onCancelExport()}>
                停止导出
              </WorkbenchButton>
            ) : null}
            {exportDialog.outputPath ? (
              <WorkbenchButton size="sm" tone="ghost" type="button" onClick={() => void onOpenExportOutput()}>
                打开目录
              </WorkbenchButton>
            ) : null}
          </div>
        </section>
      ) : null}

      {!activeCaseId ? (
        <section className="import-console-empty">
          <div className="import-console-empty__eyebrow">Case Required</div>
          <h2>请先打开案件</h2>
          <p>导入页依赖案件上下文来创建任务、读取 historical datasets 表并写入本地数据库。</p>
        </section>
      ) : (
        <>
          <div ref={stageSceneRef} className="import-console-select-scene">
            <section ref={stageRef} className={`import-console-studio is-${studioPhase}`}>
              <div className={`import-console-journey${showJourney ? " is-visible" : ""}`}>
                <WorkbenchSegmentedControl
                  value={wizardStep}
                  options={workflowSwitchOptions}
                  onChange={goToWizardStep}
                  ariaLabel="导入四步工作流"
                  className="import-console-workflow-switch"
                  optionClassName="import-console-workflow-switch__option"
                />
              </div>

              <div className="import-console-studio__layout" style={wizardStep === "summary" ? { display: "none" } : undefined}>
                {wizardStep !== "summary" ? (
                  <section className={`import-console-stage-surface is-${studioPhase} is-step-${wizardStep}`}>
                    <div className="import-console-stage-surface__inner">
                    {wizardStep === "select" ? (
                      <section className={`import-console-stage import-console-stage--select${wizardFiles.length > 0 ? " has-queue" : ""}`}>
                        <section className="import-console-dropzone-shell">
                          <div
                            className={`import-console-dropzone ${dragActive ? "is-active" : ""} ${previewing ? "is-previewing" : ""} ${wizardFiles.length ? "is-filled" : "is-idle"} ${wizardFiles.length === 0 && desktopAvailable && !previewing ? "is-clickable" : ""}`}
                            onDragOver={onDropZoneDragOver}
                            onDragLeave={onDropZoneDragLeave}
                            onDrop={(event) => void onDropZoneDrop(event)}
                            onClick={onDropZoneClick}
                            onKeyDown={onDropZoneKeyDown}
                            role={wizardFiles.length === 0 && desktopAvailable ? "button" : undefined}
                            tabIndex={wizardFiles.length === 0 && desktopAvailable ? 0 : undefined}
                            aria-disabled={wizardFiles.length === 0 && desktopAvailable ? previewing : undefined}
                          >
                            {wizardFiles.length === 0 ? (
                              <div className="import-console-dropzone__copy">
                                <span className="import-console-dropzone__eyebrow">{dragActive ? "释放文件开始导入" : "本地文件导入"}</span>
                                <span className={`import-console-dropzone__icon${dragActive ? " is-active" : ""}`} aria-hidden="true">
                                  <ImportDropzoneLottieIcon disabled={!desktopAvailable} emphasis={dropzoneAnimationEmphasis} />
                                </span>
                                <strong>{previewing ? "正在导入" : dragActive ? "释放文件开始导入" : "选择本地文件或拖拽到此"}</strong>
                                <small>{desktopAvailable ? "系统将自动识别类型，并继续完成映射与校验。" : "当前环境暂不支持本地文件导入，请在桌面版中操作。"}</small>
                                <div className="import-console-dropzone__actions">
                                  {previewing ? (
                                    <>
                                      <div
                                        className="import-console-dropzone__loading-progress"
                                        role="progressbar"
                                        aria-label="正在导入"
                                        aria-valuemin={0}
                                        aria-valuemax={100}
                                      >
                                        <span className="import-console-visually-hidden">正在导入</span>
                                        <span className="import-console-dropzone__loading-progress-bar" aria-hidden="true" />
                                      </div>
                                      <span className="import-console-dropzone__loading-caption">正在读取文件并校验结构…</span>
                                    </>
                                  ) : (
                                    <WorkbenchButton
                                      className="import-console-primary-action"
                                      size="md"
                                      tone="accent"
                                      type="button"
                                      disabled={!desktopAvailable}
                                      onClick={(event) => {
                                        event.stopPropagation();
                                        void onQuickImport();
                                      }}
                                    >
                                      选择文件
                                    </WorkbenchButton>
                                  )}
                                </div>
                                <div
                                  className={`import-console-dropzone__formats${previewing ? " is-placeholder" : ""}`}
                                  aria-label={previewing ? undefined : "支持导入格式"}
                                  aria-hidden={previewing ? true : undefined}
                                >
                                  <span>支持格式</span>
                                  <strong>CSV · XLSX · XLS · ZIP · PDF · DOC · DOCX · TXT</strong>
                                </div>
                              </div>
                            ) : (
                              <div className="import-console-dropzone__queue">
                                <ImportWorkflowStageHeader
                                  title={`已导入文件（${importedFileCountLabel}）`}
                                  actions={
                                    <>
                                      <WorkbenchButton
                                        className="import-console-dropzone__queue-add"
                                        size="sm"
                                        tone="ghost"
                                        type="button"
                                        disabled={previewing}
                                        onClick={() => void onQuickImport()}
                                      >
                                        添加文件
                                      </WorkbenchButton>
                                      <WorkbenchButton
                                        className="import-console-dropzone__queue-clear"
                                        size="sm"
                                        tone="ghost"
                                        type="button"
                                        disabled={previewing}
                                        onClick={resetWizardWorkspace}
                                      >
                                        清空全部
                                      </WorkbenchButton>
                                    </>
                                  }
                                />
                                {previewQueueStatus ? <div className="import-console-dropzone__queue-head">{renderPreviewQueueStatus()}</div> : null}
                                <div className="import-console-dropzone__queue-list">
                                  {wizardFiles.map((file) => {
                                    const hasArchiveChildren = file.archiveChildren.length > 0;
                                    const pendingArchiveCount = file.archiveChildren.filter((item) => item.status === "review").length;
                                    const runningArchiveCount = file.archiveChildren.filter((item) => item.status === "running").length;
                                    const failedArchiveCount = file.archiveChildren.filter((item) => item.status === "failed" || item.status === "unsupported").length;
                                    const readyArchiveCount = Math.max(0, file.archiveChildren.length - pendingArchiveCount - runningArchiveCount - failedArchiveCount);
                                    const isExpanded = hasArchiveChildren && expandedArchiveIds.includes(file.id);
                                    const statusTone = fileRowStatusTone(file.status);
                                    const subtitleItems = wizardFileSubtitleItems(file);
                                    const fileDomId = toDomIdFragment(file.id);
                                    const passwordInputId = `import-password-input-${fileDomId}`;
                                    const passwordHintId = `import-password-hint-${fileDomId}`;
                                    const uploadCardClassName = `import-console-upload-card${
                                      hasArchiveChildren ? " is-archive-batch is-collapsed is-archive-collapsed" : " is-single-file is-static"
                                    }`;
                                    const uploadCardSummary = (
                                      <>
                                        <ImportWorkflowFileSummary
                                          fileName={file.fileName}
                                          fileType={file.fileType}
                                          statusTone={statusTone}
                                          statusLabel={wizardFileStateLabel(file)}
                                          metaItems={subtitleItems}
                                          sha256={file.sha256}
                                          onCopySha256={file.sha256 ? () => void onCopyText(file.sha256, "SHA-256") : undefined}
                                          actions={
                                            <ImportWorkflowCardActions
                                              onRemove={() => removeWizardFile(file.id)}
                                              toggle={
                                                hasArchiveChildren
                                                  ? {
                                                      variant: "archive",
                                                      expanded: isExpanded,
                                                      onClick: () => toggleArchiveExpand(file.id),
                                                    }
                                                  : undefined
                                              }
                                            />
                                          }
                                        />
                                        {file.acceptsPassword ? (
                                          <div className={`import-console-password-inline is-${file.passwordState}`}>
                                            <div className="import-console-password-inline__copy">
                                              <strong>{file.requiresPassword ? "文件已加密，需先验证密码" : "如文件存在密码，可先完成验证"}</strong>
                                              <span id={passwordHintId}>
                                                {file.passwordValidationMessage ||
                                                  (file.requiresPassword
                                                    ? "验证通过后才可进入字段确认。"
                                                    : "无密码可直接继续。")}
                                              </span>
                                            </div>
                                            <div className="import-console-password-inline__controls">
                                              <label className="import-console-password-inline__field" htmlFor={passwordInputId}>
                                                <span className="import-console-visually-hidden">
                                                  {file.requiresPassword ? "文件密码" : "文件密码（可选）"}
                                                </span>
                                                <input
                                                  id={passwordInputId}
                                                  className="import-console-input"
                                                  type="password"
                                                  value={file.password}
                                                  onChange={(event) => updateWizardPassword(file.id, event.target.value)}
                                                  placeholder={file.requiresPassword ? "请输入文件密码" : "无密码可留空"}
                                                  autoComplete="off"
                                                  spellCheck={false}
                                                  aria-describedby={passwordHintId}
                                                  aria-invalid={file.passwordState === "error"}
                                                />
                                              </label>
                                              <button
                                                type="button"
                                                className="import-console-password-inline__action"
                                                disabled={file.passwordState === "validating"}
                                                onClick={() => void validateWizardPassword(file.id)}
                                              >
                                                {file.passwordState === "validating" ? "验证中" : "验证密码"}
                                              </button>
                                            </div>
                                          </div>
                                        ) : null}
                                      </>
                                    );
                                    if (hasArchiveChildren) {
                                      return (
                                        <ImportArchiveBranchCard
                                          key={file.id}
                                          file={file}
                                          isExpanded={isExpanded}
                                          uploadCardClassName={uploadCardClassName}
                                          uploadCardSummary={uploadCardSummary}
                                          readyArchiveCount={readyArchiveCount}
                                          runningArchiveCount={runningArchiveCount}
                                          pendingArchiveCount={pendingArchiveCount}
                                          failedArchiveCount={failedArchiveCount}
                                          onCopyText={onCopyText}
                                        />
                                      );
                                    }
                                    return (
                                      <article key={file.id} className={uploadCardClassName}>
                                        {uploadCardSummary}
                                      </article>
                                    );
                                  })}
                                </div>
                              </div>
                            )}
                          </div>
                        </section>
                      </section>
                    ) : null}

                    {wizardStep === "mapping" ? (
                      <section className="import-console-stage import-console-stage--mapping">
                        <section className="import-console-dropzone-shell import-console-mapping-stage-shell">
                          <div className="import-console-dropzone is-filled import-console-mapping-stage-dropzone">
                            <div className="import-console-dropzone__queue import-console-mapping-stage-queue">
                              <ImportWorkflowStageHeader
                                title="字段确认总览"
                                className={`import-console-mapping-stage-overview is-${mappingStageStatus.tone}`}
                                stats={
                                  <dl className="import-console-mapping-stage-overview__stats" aria-label="字段确认总览指标">
                                    <div className="import-console-mapping-stage-overview__metric">
                                      <dt>文件完成度</dt>
                                      <dd>{mappingStageStatus.confirmedFilesLabel}</dd>
                                    </div>
                                    <div className="import-console-mapping-stage-overview__metric">
                                      <dt>字段完成度</dt>
                                      <dd>
                                        {mappingStageStatus.matchedHeaders.toLocaleString("zh-CN")}/{mappingStageStatus.totalHeaders.toLocaleString("zh-CN")}
                                      </dd>
                                    </div>
                                    <div className="import-console-mapping-stage-overview__metric">
                                      <dt>关键字段</dt>
                                      <dd>{mappingStageStatus.keyFieldLabel}</dd>
                                    </div>
                                  </dl>
                                }
                                actions={
                                  <>
                                    <WorkbenchButton
                                      className="import-console-stage-toolbar__action import-console-stage-toolbar__action--neutral"
                                      size="sm"
                                      tone="ghost"
                                      disabled={!mappingStageHasMappings && !mappingStageAssistBusy}
                                      onClick={resetAllWizardMappings}
                                    >
                                      重制
                                    </WorkbenchButton>
                                    <WorkbenchButton
                                      className="import-console-stage-toolbar__action import-console-stage-toolbar__action--accent"
                                      size="sm"
                                      tone="ghost"
                                      type="button"
                                      loading={mappingStageAssistBusy}
                                      disabled={mappingStageAssistTargets.length === 0 || mappingStageAssistBusy}
                                      onClick={() => void applyMappingStageAssist()}
                                    >
                                      <svg className="import-console-stage-toolbar__action-icon" viewBox="0 0 20 20" fill="none" aria-hidden="true">
                                        <path
                                          d="M10 2.5 11.664 6.336 15.5 8l-3.836 1.664L10 13.5 8.336 9.664 4.5 8l3.836-1.664L10 2.5Z"
                                          fill="currentColor"
                                        />
                                        <path
                                          d="M15.25 12.25 15.915 13.835 17.5 14.5l-1.585.665L15.25 16.75l-.665-1.585L13 14.5l1.585-.665.665-1.585Z"
                                          fill="currentColor"
                                          opacity="0.8"
                                        />
                                      </svg>
                                      <span>{mappingStageAssistBusy ? "AI 识别中" : "AI 智能补全"}</span>
                                    </WorkbenchButton>
                                  </>
                                }
                              />
                              <div className="import-console-mapping-list">
                          {wizardFiles.map((file) => {
                            const insight =
                              mappingInsightsByFileId.get(file.id) ??
                              buildMappingInsight({
                                selectedKind: file.selectedKind,
                                suggestedKind: file.suggestedKind,
                                suggestedKindLabel: file.suggestedKindLabel,
                                selectedCategory: file.selectedCategory,
                                columnsTotal: file.columnsTotal,
                                issue: file.issue,
                                headerPreview: file.headerPreview,
                                fieldMappings: file.fieldMapping,
                                mappingOrigins: file.fieldMappingOrigins,
                                mappingStatus: file.mappingStatus,
                                mappingMethod: file.mappingMethod,
                                mappingMessage: file.mappingMessage,
                                mappingRequiredMissing: file.mappingRequiredMissing
                            });
                            const validationState = mappingValidationStateByFileId.get(file.id) ?? null;
                            const primaryKindLabel = file.selectedKind ? wizardFileKindLabel(file) : file.suggestedKindLabel || "待确认类型";
                            const headerCount = insight.headerCount || Math.max(0, file.columnsTotal);
                            const mappedCount = mappingMatchedColumnCount(insight);
                            const archiveItems = file.archiveChildren.map((child) => {
                              const childInsight = buildMappingInsight({
                                selectedKind: child.selectedKind,
                                suggestedKind: child.suggestedKind,
                                suggestedKindLabel: child.suggestedKindLabel,
                                selectedCategory: child.domainCategory,
                                columnsTotal: child.columnsTotal,
                                issue: child.issue,
                                headerPreview: child.headerPreview,
                                fieldMappings: child.fieldMapping,
                                mappingOrigins: child.fieldMappingOrigins,
                                mappingStatus: child.mappingStatus,
                                mappingMethod: child.mappingMethod,
                                mappingMessage: child.mappingMessage,
                                mappingRequiredMissing: child.mappingRequiredMissing
                              });
                              return { child, childInsight };
                            });
                            const archiveReadyCount = archiveItems.filter(({ childInsight }) => childInsight.riskCount === 0).length;
                            const archiveHeaderCount = archiveItems.reduce((sum, { childInsight }) => sum + Math.max(0, childInsight.headerCount), 0);
                            const archiveMatchedCount = archiveItems.reduce((sum, { childInsight }) => sum + mappingMatchedColumnCount(childInsight), 0);
                            const archivePendingCount = archiveItems.reduce((sum, { childInsight }) => sum + Math.max(0, childInsight.unresolvedCount), 0);
                            const archiveRequiredPendingCount = archiveItems.reduce(
                              (sum, { childInsight }) => sum + Math.max(0, childInsight.requiredTotal - childInsight.matchedRequired),
                              0
                            );
                            const archiveShouldAutoExpandMapping = archiveItems.some(({ child, childInsight }) =>
                              shouldAutoExpandMappingDetails({
                                archiveChildrenCount: 0,
                                selectedCategory: child.domainCategory,
                                selectedKind: child.selectedKind,
                                mappingStatus: child.mappingStatus,
                                mappingMethod: child.mappingMethod,
                                requiredPendingCount: Math.max(0, childInsight.requiredTotal - childInsight.matchedRequired),
                                invalid: false
                              })
                            );
                            const requiredPendingCount = Math.max(0, insight.requiredTotal - insight.matchedRequired);
                            const isArchiveMappingFile = file.archiveChildren.length > 0;
                            const shouldAutoExpandMapping = shouldAutoExpandMappingDetails({
                              archiveChildrenCount: file.archiveChildren.length,
                              selectedCategory: file.selectedCategory,
                              selectedKind: file.selectedKind,
                              mappingStatus: file.mappingStatus,
                              mappingMethod: file.mappingMethod,
                              requiredPendingCount,
                              invalid: Boolean(validationState?.invalid)
                            });
                            const mappingExpanded = isArchiveMappingFile
                              ? expandedArchiveIds.includes(file.id) || archiveShouldAutoExpandMapping
                              : expandedMappingIds.includes(file.id) || shouldAutoExpandMapping;
                            const mappingAssistState = mappingAssistById[file.id] ?? { status: "idle", message: "" };
                            const activeTargetKey =
                              (activeMappingTargetById[file.id] && insight.targetFields.some((field) => field.key === activeMappingTargetById[file.id])
                                ? activeMappingTargetById[file.id]
                                : "") || preferredActiveMappingFieldKey(insight.targetFields);
                            const activeArchiveChildId =
                              activeArchiveChildByFileId[file.id] &&
                              archiveItems.some(({ child }) => child.id === activeArchiveChildByFileId[file.id])
                                ? activeArchiveChildByFileId[file.id]
                                : "";
                            const pendingTemplateFields = mappingPendingTemplateFields(insight);
                            const hasBlockingRisk =
                              file.status === "unsupported" ||
                              file.status === "failed" ||
                              requiredPendingCount > 0 ||
                              archiveRequiredPendingCount > 0;
                            const hasPendingReview =
                              (file.archiveChildren.length > 0 && archivePendingCount > 0) ||
                              (file.selectedCategory !== "support" && insight.unresolvedCount > 0) ||
                              Boolean(validationState?.invalid);
                            const fileResultTone: "success" | "warn" | "error" = hasBlockingRisk ? "error" : hasPendingReview ? "warn" : "success";
                            const fileResultLabel = fileResultTone === "error" ? "异常" : fileResultTone === "warn" ? "待处理" : "已匹配";
                            const fileResultSummary =
                              file.archiveChildren.length > 0
                                ? [
                                    "压缩批次",
                                    `${file.archiveChildren.length.toLocaleString("zh-CN")} 个子项`,
                                    `已匹配 ${archiveMatchedCount.toLocaleString("zh-CN")}/${archiveHeaderCount.toLocaleString("zh-CN")}`,
                                    `待确认 ${archivePendingCount.toLocaleString("zh-CN")}`
                                  ]
                                : file.selectedCategory === "support"
                                  ? [importCategoryLabel(file.selectedCategory), "文件登记型"]
                                  : [
                                      primaryKindLabel,
                                      `${headerCount > 0 ? headerCount.toLocaleString("zh-CN") : "待解析"} 个字段`,
                                      `已匹配 ${mappedCount.toLocaleString("zh-CN")}/${Math.max(0, headerCount).toLocaleString("zh-CN")}`,
                                      `待确认 ${Math.max(0, insight.unresolvedCount).toLocaleString("zh-CN")}`
                                    ];
                            const fileResultMessage =
                              validationState?.message ||
                              (fileResultTone === "success"
                                ? file.selectedCategory === "support"
                                  ? "当前文件将按研判材料登记，不进入结构化映射。"
                                  : "字段确认已完成，可执行入库。"
                                : "当前文件仍需处理后才能进入下一步。");
                            const collapsedCardClassName = `import-console-mapping-card import-console-upload-card ${
                              isArchiveMappingFile ? "is-archive-batch is-archive-collapsed" : "is-single-file"
                            } is-collapsed`;

                            const expandedCardSummary = (
                              <ImportWorkflowFileSummary
                                fileName={file.fileName}
                                fileType={file.fileType}
                                statusTone={fileResultTone}
                                statusLabel={fileResultLabel}
                                metaItems={fileResultSummary}
                                message={isArchiveMappingFile ? undefined : mappingExpanded ? fileResultMessage : undefined}
                                messageTone={fileResultTone}
                                actions={
                                  <ImportWorkflowCardActions
                                    onRemove={() => removeWizardFile(file.id)}
                                    toggle={
                                      isArchiveMappingFile
                                        ? {
                                            variant: "archive",
                                            expanded: mappingExpanded,
                                            onClick: () => toggleArchiveExpand(file.id),
                                          }
                                        : {
                                            variant: "mapping",
                                            expanded: mappingExpanded,
                                            onClick: () => toggleMappingExpand(file.id),
                                            collapsedLabel: "查看字段映射",
                                            expandedLabel: "收起映射详情",
                                          }
                                    }
                                  />
                                }
                              />
                            );

                            if (isArchiveMappingFile) {
                              return (
                                <ImportArchiveMappingBranchCard
                                  key={file.id}
                                  file={file}
                                  isExpanded={mappingExpanded}
                                  parentCardClassName={collapsedCardClassName}
                                  parentSummary={expandedCardSummary}
                                  archiveItems={archiveItems}
                                  activeArchiveChildId={activeArchiveChildId}
                                  onToggleChildDetail={(childId) =>
                                    setActiveArchiveChildByFileId((previous) => ({
                                      ...previous,
                                      [file.id]: previous[file.id] === childId ? "" : childId,
                                    }))
                                  }
                                  mappingAssistById={mappingAssistById}
                                  activeMappingTargetById={activeMappingTargetById}
                                  setActiveMappingTargetById={setActiveMappingTargetById}
                                  applyArchiveChildMappingAssist={applyArchiveChildMappingAssist}
                                  updateWizardArchiveChildCategory={updateWizardArchiveChildCategory}
                                  updateWizardArchiveChildKind={updateWizardArchiveChildKind}
                                  updateWizardArchiveChildFieldMapping={updateWizardArchiveChildFieldMapping}
                                  updateWizardArchiveChildSourceBinding={updateWizardArchiveChildSourceBinding}
                                />
                              );
                            }

                            if (!mappingExpanded) {
                              return (
                                <article key={file.id} className={collapsedCardClassName}>
                                  <ImportWorkflowFileSummary
                                    fileName={file.fileName}
                                    fileType={file.fileType}
                                    statusTone={fileResultTone}
                                    statusLabel={fileResultLabel}
                                    metaItems={fileResultSummary}
                                    actions={
                                      <ImportWorkflowCardActions
                                        onRemove={() => removeWizardFile(file.id)}
                                        toggle={{
                                          variant: "mapping",
                                          expanded: false,
                                          onClick: () => toggleMappingExpand(file.id),
                                          collapsedLabel: "查看字段映射",
                                          expandedLabel: "收起映射详情",
                                        }}
                                      />
                                    }
                                  />
                                </article>
                              );
                            }

                            return (
                              <article
                                key={file.id}
                                className="import-console-mapping-card import-console-upload-card is-single-file is-expanded"
                              >
                                {expandedCardSummary}
                                {mappingExpanded ? (
                                  <div className="import-console-mapping-card__editor">
                                    <div className="import-console-mapping-card__body">
                                      <MappingTemplateToolbar
                                        scopeKey={file.id}
                                        selectedCategory={file.selectedCategory}
                                        selectedKind={file.selectedKind}
                                        assistState={mappingAssistState}
                                        action={
                                          file.selectedCategory !== "support" && insight.targetFields.length > 0 ? (
                                            <MappingAssistButton
                                              disabled={mappingAssistState.status === "running"}
                                              running={mappingAssistState.status === "running"}
                                              onClick={() => void applyMappingAssist(file.id)}
                                            />
                                          ) : null
                                        }
                                        onCategoryChange={(category) => updateWizardCategory(file.id, category)}
                                        onKindChange={(kind) => updateWizardKind(file.id, kind)}
                                      />
                                      <MappingPendingTemplateFieldList fields={pendingTemplateFields} />
                                      {file.selectedCategory !== "support" ? (
                                        <MappingWorkbenchCanvas
                                          scopeId={file.id}
                                          kind={file.selectedKind || file.suggestedKind}
                                          insight={insight}
                                          headers={file.headerPreview}
                                          sampleRows={file.sampleRows}
                                          activeTargetKey={activeTargetKey}
                                          onActiveTargetChange={(fieldKey) =>
                                            setActiveMappingTargetById((previous) => ({ ...previous, [file.id]: fieldKey }))
                                          }
                                          onAssignTarget={(fieldKey, sourceHeader) =>
                                            updateWizardFieldMapping(file.id, fieldKey, sourceHeader)
                                          }
                                          onAssignSource={(sourceHeader, fieldKey) =>
                                            updateWizardSourceBinding(file.id, sourceHeader, fieldKey)
                                          }
                                        />
                                      ) : (
                                        <div className="import-console-mapping-card__empty">
                                          当前文件将按研判文件登记，不需要结构化字段对齐。
                                        </div>
                                      )}
                                    </div>
                                  </div>
                                ) : null}
                              </article>
                            );
                          })}
                              </div>
                            </div>
                          </div>
                        </section>
                      </section>
                    ) : null}

                    {wizardStep === "validation" ? (
                      <section className="import-console-stage import-console-stage--validation">
                        <section className="import-console-dropzone-shell import-console-mapping-stage-shell import-console-validation-stage-shell">
                          <div className="import-console-dropzone is-filled import-console-mapping-stage-dropzone import-console-validation-stage-dropzone">
                            <div className="import-console-dropzone__queue import-console-mapping-stage-queue import-console-validation-stage-queue import-console-validation-panel">
                              <ImportWorkflowStageHeader
                                title={`导入文件（${visibleValidationGroupCount.toLocaleString("zh-CN")}）`}
                                actions={
                                  <div className="import-console-validation-overview__button-group" role="tablist" aria-label="执行入库筛选">
                                    {validationFilterOptions.map((option) => {
                                      const active = validationFilter === option.value;
                                      return (
                                        <WorkbenchButton
                                          key={option.value}
                                          className={`import-console-stage-toolbar__action import-console-validation-overview__filter-button${
                                            active ? " is-active" : ""
                                          }`}
                                          size="sm"
                                          tone="ghost"
                                          type="button"
                                          role="tab"
                                          aria-selected={active}
                                          tabIndex={active ? 0 : -1}
                                          onClick={() => setValidationFilter(option.value)}
                                          onKeyDown={handleTabListKeyDown}
                                        >
                                          {option.label}
                                        </WorkbenchButton>
                                      );
                                    })}
                                  </div>
                                }
                              />
                              <div
                                ref={validationStreamRef}
                                className="import-console-mapping-list import-console-validation-list import-console-validation-list--stream"
                              >
                                {validationDisplayGroups.map(({ file, hidden }) => {
                                  const card = file.childProgress.length > 0 ? (
                                    <ImportValidationArchiveBranchCard
                                      key={file.id}
                                      file={file}
                                      isExpanded={expandedArchiveIds.includes(file.id)}
                                      started={wizardValidationStarted}
                                      hideCompletedChildren={validationFilter === "pending"}
                                      onToggleExpand={() => toggleArchiveExpand(file.id)}
                                      onCopyText={onCopyText}
                                      onOpenDetail={() => openWizardProgressDetail(file)}
                                      onOpenChildDetail={(child) => openWizardProgressChildDetail(file, child)}
                                    />
                                  ) : (
                                    <ImportValidationSingleFileCard
                                      key={file.id}
                                      file={file}
                                      started={wizardValidationStarted}
                                      onCopyText={onCopyText}
                                      onOpenDetail={() => openWizardProgressDetail(file)}
                                    />
                                  );

                                  return (
                                    <ImportValidationProgressGroupShell key={file.id} groupId={file.id} hidden={hidden} active={file.isActive}>
                                      {card}
                                    </ImportValidationProgressGroupShell>
                                  );
                                })}
                                {showValidationEmptyState ? (
                                  <div className="import-console-mapping-card__empty">当前筛选下暂无匹配文件。</div>
                                ) : null}
                              </div>
                            </div>
                          </div>
                        </section>
                      </section>
                    ) : null}

                    {(wizardStep === "select" && wizardFiles.length > 0) || wizardStep === "mapping" || wizardStep === "validation" ? (
                      <div className="import-console-stage__actions import-console-stage__actions--floating import-console-stage__actions--flow-nav">
                        {wizardStep === "select" ? (
                          <>
                            <WorkbenchButton size="sm" type="button" onClick={resetWizardWorkspace}>
                              返回
                            </WorkbenchButton>
                            <div className="import-console-stage__actions-main">
                              <WorkbenchButton
                                size="sm"
                                tone="accent"
                                type="button"
                                disabled={!wizardCanGoMapping || previewing}
                                onClick={() => goToWizardStep("mapping")}
                              >
                                下一步：字段确认
                              </WorkbenchButton>
                            </div>
                          </>
                        ) : null}

                        {wizardStep === "mapping" ? (
                          <>
                            <WorkbenchButton size="sm" type="button" onClick={() => goToWizardStep("select")}>
                              上一步
                            </WorkbenchButton>
                            <div className={`import-console-stage__actions-status is-${mappingStageStatus.tone}`}>
                              <span>{mappingStageStatus.handoffText}</span>
                            </div>
                            <div className="import-console-stage__actions-main">
                              <WorkbenchButton
                                size="sm"
                                tone="accent"
                                type="button"
                                disabled={!wizardCanGoValidation}
                                title={!wizardCanGoValidation ? mappingStageStatus.reasonText : undefined}
                                onClick={() => goToWizardStep("validation")}
                              >
                                下一步：执行入库
                              </WorkbenchButton>
                            </div>
                          </>
                        ) : null}

                        {wizardStep === "validation" ? (
                          <>
                            <WorkbenchButton size="sm" type="button" onClick={() => goToWizardStep("mapping")} disabled={submitting || wizardIsImporting}>
                              返回字段确认
                            </WorkbenchButton>
                            <div className="import-console-stage__actions-main import-console-stage__actions-main--validation">
                              {!currentWizardJob ? (
                                <WorkbenchButton
                                  size="sm"
                                  tone="accent"
                                  type="button"
                                  loading={submitting}
                                  disabled={!wizardCanGoValidation || submitting}
                                  onClick={() => void onSubmitWizardImport()}
                                >
                                  开始执行入库
                                </WorkbenchButton>
                              ) : null}
                              {currentWizardJob && wizardIsImporting ? (
                                <WorkbenchButton size="sm" tone="accent" type="button" loading>
                                  执行入库中
                                </WorkbenchButton>
                              ) : null}
                              {currentWizardJob && !wizardIsImporting ? (
                                <WorkbenchButton
                                  size="sm"
                                  tone="accent"
                                  type="button"
                                  disabled={!wizardCanGoSummary}
                                  onClick={() => goToWizardStep("summary")}
                                >
                                  下一步：结果总表
                                </WorkbenchButton>
                              ) : null}
                            </div>
                          </>
                        ) : null}
                      </div>
                    ) : null}

                    </div>
                  </section>
                ) : null}
              </div>
            </section>
          </div>

          {showExecutionOverview ? (
            <section className="import-console-execution-overview">
              <div className="import-console-execution-overview__head">
                <div>
                  <div className="import-console-section-head__eyebrow">Execution Overview</div>
                  <h2>执行概览</h2>
                </div>
                <div className="import-console-section-head__actions">
                  <WorkbenchButton
                    className="import-console-primary-action"
                    size="md"
                    tone="accent"
                    type="button"
                    disabled={!wizardCanGoSummary}
                    onClick={() => goToWizardStep("summary")}
                  >
                    查看详情
                  </WorkbenchButton>
                </div>
              </div>
              {executionTableRows.length > 0 ? (
                <div className="table-wrap import-console-execution-table-wrap">
                  <table className="data-table import-console-execution-table" style={{ minWidth: `${executionTableLayout.minWidth}px` }}>
                    <colgroup>
                      {executionTableLayout.widths.map((width, index) => (
                        <col key={`execution-col:${index}`} style={{ width: `${width}px` }} />
                      ))}
                    </colgroup>
                    <thead>
                      <tr>
                        <th>文件名</th>
                        <th>数据类型</th>
                        <th>日期</th>
                        <th>大小</th>
                        <th>状态</th>
                        <th>进度</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {executionTableRows.map((row) => {
                        const previewDisabled = !canPreviewExecutionRow(row);
                        const downloadDisabled = !canDownloadExecutionRow(row);
                        const retryDisabled = !canRetryExecutionRow(row);
                        const deleteDisabled = !canDeleteExecutionRow(row);
                        const retryLabel = row.statusText === "失败" ? "重试" : "复制";
                        const displayTitle = formatExecutionOverviewFileName(row.title, row.dataType);
                        return (
                          <tr key={`execution:${row.key}`} className={row.isLiveTask ? "is-live-task" : ""}>
                            <td title={row.title}>
                              <div className="import-console-execution-table__file">
                                <strong className="import-console-execution-table__copyable-text">{displayTitle}</strong>
                                <small>{row.subtitle}</small>
                              </div>
                            </td>
                            <td className="import-console-execution-table__copyable-cell">{row.dataType}</td>
                            <td className="import-console-execution-table__copyable-cell">{row.dateText}</td>
                            <td className="import-console-execution-table__copyable-cell">{row.sizeText}</td>
                            <td>
                              <span className={`import-workbench-status-badge tone-${row.tone}`}>
                                <ImportTaskStatusIcon kind={statusBadgeIconKind(row.statusText, row.tone)} />
                                <span>{statusBadgeText(row.statusText)}</span>
                              </span>
                            </td>
                            <td>
                              <div className="import-console-execution-table__progress">
                                <span>{row.progressText}</span>
                                {row.progressValue !== null ? (
                                  <div
                                    className="import-console-execution-table__progress-track"
                                    role="progressbar"
                                    aria-valuenow={Math.max(0, Math.min(100, row.progressValue))}
                                    aria-valuemin={0}
                                    aria-valuemax={100}
                                  >
                                    <span style={{ width: `${Math.max(0, Math.min(100, row.progressValue))}%` }} />
                                  </div>
                                ) : null}
                              </div>
                            </td>
                            <td>
                              <div className="import-console-execution-table__actions">
                                <button
                                  type="button"
                                  className="import-console-action-icon"
                                  aria-label={`查看 ${row.title}`}
                                  title="查看/预览"
                                  disabled={previewDisabled}
                                  onClick={() => openExecutionRowPreview(row)}
                                >
                                  <ImportTaskActionIcon kind="view" />
                                </button>
                                <button
                                  type="button"
                                  className="import-console-action-icon"
                                  aria-label={`下载 ${row.title}`}
                                  title="下载原始文件"
                                  disabled={downloadDisabled}
                                  onClick={() => void onDownloadExecutionRow(row)}
                                >
                                  <ImportTaskActionIcon kind="download" />
                                </button>
                                <button
                                  type="button"
                                  className="import-console-action-icon"
                                  aria-label={`${retryLabel} ${row.title}`}
                                  title={retryLabel}
                                  disabled={retryDisabled}
                                  onClick={() => void onRetryExecutionRow(row)}
                                >
                                  <ImportTaskActionIcon kind="retry" />
                                </button>
                                <button
                                  type="button"
                                  className="import-console-action-icon is-danger"
                                  aria-label={`移入回收站 ${row.title}`}
                                  title="移入回收站"
                                  disabled={deleteDisabled}
                                  onClick={() => {
                                    if (row.record?.ledgerRow) {
                                      openLedgerActionDialog("recycle", [row.record.ledgerRow]);
                                    }
                                  }}
                                >
                                  <ImportTaskActionIcon kind="delete" />
                                </button>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              ) : null}
            </section>
          ) : null}

          {showResultPreview ? (
            <section className="import-console-results-preview">
              <div className="import-console-results-preview__head">
                <div>
                  <div className="import-console-section-head__eyebrow">Result Preview</div>
                  <h2>导入概括</h2>
                  <p>
                    当前批次成功率 {batchResultOverview.successRate.toLocaleString("zh-CN")}%，
                    {batchResultOverview.importedRows === null
                      ? "有效记录数未验证。"
                      : `已沉淀 ${batchResultOverview.importedRows.toLocaleString("zh-CN")} 条有效记录。`}
                  </p>
                </div>
                <div className="import-console-summary-head__actions">
                  <span className="import-console-note">最近 {compactResultRows.length} 条记录</span>
                  <WorkbenchButton className="import-console-primary-action" size="md" tone="accent" type="button" onClick={() => goToWizardStep("summary")}>
                    查看详情
                  </WorkbenchButton>
                </div>
              </div>
              <div className="import-console-results-preview__list">
                {compactResultRows.map((row) => (
                  <article key={`preview:${row.key}`} className="import-console-results-preview__card">
                    <div>
                      <strong>{row.fileName}</strong>
                      <span>
                        {row.categoryLabel} / {row.kindLabel}
                      </span>
                      <small>{formatDateTime(row.importedAt)}</small>
                    </div>
                    <div className="import-console-results-preview__card-meta">
                      <span>{row.rowsTotal === null ? "行数未验证" : `${row.rowsTotal.toLocaleString("zh-CN")} 行`}</span>
                      <span>{row.validRows !== null && row.validRows > 0 ? `${row.validRows.toLocaleString("zh-CN")} 条有效` : row.duplicateFlag}</span>
                    </div>
                    <div>
                      <span className={`import-workbench-status-badge tone-${statusTone(row.statusText)}`}>{statusBadgeText(row.statusText)}</span>
                      <small>{row.validRows !== null && row.validRows > 0 ? `${row.validRows.toLocaleString("zh-CN")} 条有效记录` : row.duplicateSummary}</small>
                    </div>
                  </article>
                ))}
              </div>
            </section>
          ) : null}

          {showResultLedger ? (
            <div className="import-console-summary-block">
              <section className="import-console-section-head import-console-section-head--ledger import-console-summary-head--aligned">
                <div>
                  <div className="import-console-section-head__eyebrow">Summary</div>
                  <h2>结果总表</h2>
                </div>
              </section>

              <WorkbenchMetricStrip className="import-console-category-grid" columns={3} tabletColumns={2}>
                {categoryCards.map((card) => {
                  const active = categoryFilter === card.key;
                  return (
                    <WorkbenchMetricCard
                      key={card.key}
                      as="button"
                      buttonType="button"
                      active={active}
                      className={`import-console-category-card ${active ? "is-active" : ""}`}
                      icon={renderImportCategoryIcon(card.key)}
                      label={card.label}
                      value={`${card.fileCount.toLocaleString("zh-CN")} 份`}
                      description={card.hint}
                      metaAccent={card.rowCount === null ? "行数未验证" : `${card.rowCount.toLocaleString("zh-CN")} 行数据`}
                      metaTail={active ? "当前筛选" : "点击筛选"}
                      aria-pressed={active}
                      onClick={() => setCategoryFilter((previous) => (previous === card.key ? "all" : card.key))}
                    />
                  );
                })}
              </WorkbenchMetricStrip>

              <WorkbenchTableShell className="import-console-ledger workbench-ui-table-card">
                <WorkbenchTableToolbar className="workbench-ui-table-card__header import-console-ledger__header">
                  <div className="workbench-ui-table-card__copy">
                    <div className="workbench-ui-table-card__title-row">
                      <span className="workbench-ui-table-card__icon" aria-hidden="true">
                        {renderImportLedgerIcon()}
                      </span>
                      <span>{ledgerTitle}:</span>
                      <strong>{consoleRows.length.toLocaleString("zh-CN")} 条记录</strong>
                    </div>
                  </div>

                  <div className="workbench-ui-table-card__tools import-console-ledger__tools">
                    <WorkbenchSegmentedControl
                      value={ledgerView}
                      options={[
                        { value: "active", label: "结果总表" },
                        { value: "recycle", label: "回收站" }
                      ]}
                      onChange={(value) => setLedgerView(value as ImportFileLogView)}
                      ariaLabel="结果总表视图切换"
                      className="import-console-ledger__view-switch"
                    />
                    <WorkbenchToolbarSearchField
                      className="workbench-ui-table-card__search"
                      value={searchText}
                      onChange={setSearchText}
                      placeholder={ledgerToolbarSearchPlaceholder}
                      aria-label={`搜索${ledgerTitle}`}
                      autoComplete="off"
                    />
                    {ledgerView === "recycle" ? (
                      <>
                        <WorkbenchToolbarButton
                          type="button"
                          tone="accent"
                          disabled={selectedActionCount === 0 || ledgerActionSubmitting}
                          badge={selectedActionCount > 0 ? selectedActionCount.toLocaleString("zh-CN") : undefined}
                          onClick={() => openLedgerActionDialog("restore", selectedActionRows)}
                        >
                          恢复
                        </WorkbenchToolbarButton>
                        <WorkbenchToolbarButton
                          type="button"
                          tone="danger"
                          disabled={selectedActionCount === 0 || ledgerActionSubmitting}
                          onClick={() => openLedgerActionDialog("purge", selectedActionRows)}
                        >
                          彻底删除
                        </WorkbenchToolbarButton>
                      </>
                    ) : (
                      <WorkbenchToolbarButton
                        type="button"
                        tone="danger"
                        disabled={selectedActionCount === 0 || ledgerActionSubmitting}
                        badge={selectedActionCount > 0 ? selectedActionCount.toLocaleString("zh-CN") : undefined}
                        onClick={() => openLedgerActionDialog("recycle", selectedActionRows)}
                      >
                        {ledgerPrimaryActionLabel}
                      </WorkbenchToolbarButton>
                    )}
                    {(categoryFilter !== "all" || searchText.trim()) && (
                      <WorkbenchToolbarButton
                        type="button"
                        tone="neutral"
                        onClick={() => {
                          setCategoryFilter("all");
                          setSearchText("");
                        }}
                      >
                        清空视图
                      </WorkbenchToolbarButton>
                    )}
                  </div>
                </WorkbenchTableToolbar>

                <WorkbenchTableBody className="workbench-ui-table-body import-console-ledger__body">
                  {loading ? <div className="import-workbench-table-empty">导入数据加载中...</div> : null}
                  {!loading && showLedgerEmptyMessage ? <div className="import-workbench-table-empty">当前条件下没有可展示的导入记录。</div> : null}

                  {!loading && (consoleRows.length > 0 || showLedgerPlaceholderRows) ? (
                    <div className="table-wrap import-console-table-wrap workbench-ui-ledger-table-wrap">
                      <table className="data-table import-console-table workbench-ui-ledger-table">
                      <colgroup>
                        <col className="import-console-table__col import-console-table__col--selection" />
                        <col className="import-console-table__col import-console-table__col--file" />
                        <col className="import-console-table__col import-console-table__col--category" />
                        <col className="import-console-table__col import-console-table__col--kind" />
                        <col className="import-console-table__col import-console-table__col--status" />
                        <col className="import-console-table__col import-console-table__col--scale" />
                        <col className="import-console-table__col import-console-table__col--duplicate" />
                        <col className="import-console-table__col import-console-table__col--summary" />
                        <col className="import-console-table__col import-console-table__col--sha" />
                        <col className="import-console-table__col import-console-table__col--path" />
                        <col className="import-console-table__col import-console-table__col--time" />
                      </colgroup>
                      <thead>
                        <tr>
                          <th className="import-console-table__selection-cell">
                            <div className="import-console-table__selection-head">
                              <input
                                ref={ledgerSelectAllRef}
                                type="checkbox"
                                className="import-console-table__check"
                                checked={allVisibleSelectableSelected}
                                disabled={selectableConsoleRows.length === 0}
                                aria-label="全选当前结果"
                                onChange={(event) => toggleSelectAllVisibleLedgerRows(event.target.checked)}
                              />
                            </div>
                          </th>
                          <th>文件名</th>
                          <th>分类</th>
                          <th>数据类型</th>
                          <th>导入状态</th>
                          <th>数据规模</th>
                          <th>是否重复</th>
                          <th>重复概况</th>
                          <th>SHA-256</th>
                          <th>导入路径</th>
                          <th>{ledgerTimeColumnLabel}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {consoleRows.map((row) => {
                          const tone = statusTone(row.statusText);
                          const rowLedger = row.ledgerRow;
                          const selectable = Boolean(rowLedger && canSelectLedgerRow(rowLedger));
                          const selected = Boolean(rowLedger && selectedLedgerFileIdSet.has(rowLedger.file_id));
                          const importedAtText = formatDateTime(row.importedAt);
                          const [importDateText, importTimeText = ""] =
                            importedAtText === "—" ? ["—", ""] : importedAtText.split(" ");
                          const totalRowsText = formatKnownCount(row.rowsTotal);
                          const validRowsText = formatKnownCount(row.validRows);
                          const columnCountText = row.columnCount === null
                            ? "—"
                            : row.columnCount.toLocaleString("zh-CN");
                          return (
                            <tr
                              key={row.key}
                              className={row.key === selectedRowKey ? "is-selected" : ""}
                              onClick={() => setSelectedRowKey(row.key)}
                              onContextMenu={
                                row.ledgerRow ? (event) => openRowContextMenu(event, row.ledgerRow as LedgerRow, null) : undefined
                              }
                            >
                              <td className="import-console-table__selection-cell">
                                <input
                                  type="checkbox"
                                  className="import-console-table__check"
                                  checked={selected}
                                  disabled={!selectable}
                                  aria-label={selectable ? `选择 ${row.fileName}` : `${row.fileName} 当前不可操作`}
                                  onClick={(event) => event.stopPropagation()}
                                  onChange={(event) => {
                                    event.stopPropagation();
                                    if (rowLedger) {
                                      toggleLedgerRowSelection(rowLedger, event.target.checked);
                                    }
                                  }}
                                />
                              </td>
                              <td title={row.fileName}>
                                <div className="import-console-table__file">
                                  <strong className="import-console-table__copyable-text">{row.fileName}</strong>
                                  {row.detailText && row.ledgerRow ? (
                                    <button
                                      type="button"
                                      className="import-workbench-inline-link"
                                      onClick={(event) => {
                                        event.stopPropagation();
                                        openRowDetailDialog(row.ledgerRow as LedgerRow);
                                      }}
                                    >
                                      日志
                                    </button>
                                  ) : null}
                                </div>
                              </td>
                              <td className="import-console-table__copyable-cell">
                                <div className="import-console-table__category import-console-table__copyable-block">
                                  <strong>{row.categoryLabel}</strong>
                                  <span>{row.categoryEnglish}</span>
                                </div>
                              </td>
                              <td className="import-console-table__kind-cell">
                                <div className="import-console-table__kind import-console-table__copyable-text">{row.kindLabel}</div>
                              </td>
                              <td className="import-console-table__status-cell">
                                <span className={`import-workbench-status-badge tone-${tone}`}>{statusBadgeText(row.statusText)}</span>
                              </td>
                              <td className="import-console-table__scale-cell">
                                <div className="import-console-table__scale">
                                  <div className="import-console-table__scale-item">
                                    <span className="import-console-table__scale-label">总</span>
                                    <strong className="import-console-table__scale-value">{totalRowsText}</strong>
                                  </div>
                                  <div className="import-console-table__scale-item">
                                    <span className="import-console-table__scale-label">有效</span>
                                    <strong className="import-console-table__scale-value">{validRowsText}</strong>
                                  </div>
                                  <div className="import-console-table__scale-item">
                                    <span className="import-console-table__scale-label">列</span>
                                    <strong className="import-console-table__scale-value">{columnCountText}</strong>
                                  </div>
                                </div>
                              </td>
                              <td>
                                <span className={`import-console-duplicate-badge is-${row.duplicateFlag === "否" ? "clean" : row.duplicateFlag === "—" ? "muted" : "warn"}`}>
                                  {row.duplicateFlag}
                                </span>
                              </td>
                              <td className="import-console-table__duplicate-summary-cell import-console-table__copyable-cell">
                                <div className="import-console-table__duplicate-summary import-console-table__copyable-block">{row.duplicateSummary}</div>
                              </td>
                              <td className="import-console-table__hash-cell">
                                {row.sha256 ? (
                                  <button
                                    type="button"
                                    className="import-workbench-hash-button"
                                    title={row.sha256}
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      void onCopyText(row.sha256, "SHA-256");
                                    }}
                                  >
                                    {shortHash(row.sha256)}
                                  </button>
                                ) : (
                                  "—"
                                )}
                              </td>
                              <td className="import-console-table__path-cell">
                                <div className="import-console-table__path">
                                  <span title={row.path}>{row.path || "—"}</span>
                                  {row.path ? (
                                    <button
                                      type="button"
                                      className="import-workbench-inline-link"
                                      onClick={(event) => {
                                        event.stopPropagation();
                                        void onCopyText(row.path, "导入路径");
                                      }}
                                    >
                                      复制
                                    </button>
                                  ) : null}
                                </div>
                              </td>
                              <td className="import-console-table__time">
                                <div className="import-console-table__time-stack">
                                  <span>{importDateText}</span>
                                  {importTimeText ? <strong>{importTimeText}</strong> : null}
                                </div>
                              </td>
                            </tr>
                          );
                        })}
                        {showLedgerPlaceholderRows
                          ? ledgerPlaceholderRows.map((placeholderKey) => (
                              <tr key={placeholderKey} className="import-console-table__placeholder-row" aria-hidden="true">
                                {Array.from({ length: 11 }).map((_, columnIndex) => (
                                  <td key={`${placeholderKey}-${columnIndex}`}>
                                    <span className="import-console-table__placeholder-spacer"> </span>
                                  </td>
                                ))}
                              </tr>
                            ))
                          : null}
                      </tbody>
                    </table>
                    </div>
                  ) : null}
                </WorkbenchTableBody>
                <div className="import-console-ledger__footer">
                  <span>展示 {consoleRows.length.toLocaleString("zh-CN")} 条{ledgerView === "recycle" ? "回收站" : "导入"}记录</span>
                  <span>总文件 {ledgerConsoleRowsBase.length.toLocaleString("zh-CN")}</span>
                  <span>异常/关注 {ledgerAttentionCount.toLocaleString("zh-CN")}</span>
                </div>
              </WorkbenchTableShell>
            </div>
          ) : null}
        </>
      )}

      <Dialog
        open={detailDialog !== null}
        title={detailDialog?.title || "详情"}
        subtitle="导入任务日志"
        width={detailDialogWidth}
        onClose={() => setDetailDialog(null)}
      >
        <pre className="import-workbench-detail-log">{detailDialog?.text || ""}</pre>
      </Dialog>

      <Dialog
        open={ledgerActionDialogState !== null}
        title={ledgerActionDialogTitle}
        subtitle={ledgerActionDialogSubtitle}
        width={deleteDialogWidth}
        onClose={closeLedgerActionDialog}
        footer={
          <>
            <WorkbenchButton type="button" disabled={ledgerActionSubmitting} onClick={closeLedgerActionDialog}>
              取消
            </WorkbenchButton>
            <WorkbenchButton
              tone={ledgerActionDialogMode === "restore" ? "accent" : "danger"}
              type="button"
              loading={ledgerActionSubmitting}
              disabled={ledgerActionSubmitting || ledgerActionDialogBlocksPurge}
              onClick={() => void onConfirmLedgerAction()}
            >
              {ledgerActionConfirmLabel}
            </WorkbenchButton>
          </>
        }
      >
        <div className="stack import-console-delete-dialog">
          <p>{ledgerActionDialogMode === "restore" ? "恢复条数" : "影响条数"}：{ledgerActionDialogCount.toLocaleString("zh-CN")} 条</p>
          {ledgerActionDialogValidRowEstimate.status === "complete" ? (
            <p>预计影响有效数据：{ledgerActionDialogValidRowEstimate.value.toLocaleString("zh-CN")} 行（当前台账计数完整）</p>
          ) : (
            <p>
              影响行数未知
              {ledgerActionDialogValidRowEstimate.unknownItemCount > 0
                ? `（${ledgerActionDialogValidRowEstimate.unknownItemCount.toLocaleString("zh-CN")} 个文件未提供完整计数）`
                : ""}
            </p>
          )}
          {ledgerActionDialogBlocksPurge ? <p>安全门：影响范围未完整核验，禁止彻底删除。</p> : null}
          {ledgerActionDialogMode === "recycle" ? (
            <>
              <p>处理范围：将从结果总表、原始库、清洗库、清洗日志与文档索引中移出，并转入回收站。</p>
              <p>恢复能力：回收站中的记录可恢复，不会立刻永久清理。</p>
            </>
          ) : ledgerActionDialogMode === "restore" ? (
            <>
              <p>处理范围：将恢复导入台账、DuckDB 原始库/清洗库数据及相关文档索引。</p>
              <p>恢复后会重新回到结果总表，可继续参与检索、统计和后续流程。</p>
            </>
          ) : (
            <>
              <p>删除范围：回收站、DuckDB 原始库、清洗库、清洗日志、文档索引及关联缓存文件。</p>
              <p>该操作不可恢复，请仅在确认不再需要这些记录时执行。</p>
            </>
          )}
          <p>审计记录：本次操作会写入案件审计，便于后续追溯。</p>
          {ledgerActionDialogCount === 1 ? (
            <>
              <p>文件：{ledgerActionDialogRows[0]?.display_name || "-"}</p>
              <p>类型：{ledgerActionDialogRows[0]?.kind_label || "-"}</p>
            </>
          ) : (
            <div className="import-console-delete-dialog__list">
              {ledgerActionDialogPreviewRows.map((row) => (
                <p key={row.file_id}>{row.display_name || row.file_id}</p>
              ))}
              {ledgerActionDialogCount > ledgerActionDialogPreviewRows.length ? (
                <p>…… 另有 {(ledgerActionDialogCount - ledgerActionDialogPreviewRows.length).toLocaleString("zh-CN")} 条记录</p>
              ) : null}
            </div>
          )}
        </div>
      </Dialog>

      <Dialog
        open={Boolean(exportDialog?.open)}
        title="导出中"
        subtitle={exportDialog?.jobId ? `任务 ID: ${exportDialog.jobId}` : ""}
        width={620}
        onClose={onCloseExportDialog}
        footer={
          <>
            {exportDialog?.status === "running" ? (
              <WorkbenchButton tone="danger" type="button" onClick={() => void onCancelExport()}>
                停止导出
              </WorkbenchButton>
            ) : null}
            {exportDialog?.outputPath ? (
              <WorkbenchButton type="button" onClick={() => void onOpenExportOutput()}>
                打开导出目录
              </WorkbenchButton>
            ) : null}
            <WorkbenchButton type="button" onClick={onCloseExportDialog}>
              关闭
            </WorkbenchButton>
          </>
        }
      >
        <div className="stack import-workbench-export-dialog">
          <p>当前数据表：{exportDialog?.tableLabel || "等待导出"}</p>
          <p>
            总数 {toInt(exportDialog?.tableTotal).toLocaleString("zh-CN")} 已完成 {toInt(exportDialog?.tableDone).toLocaleString("zh-CN")}
          </p>
          <p>状态：{exportDialog?.tableStatus || exportDialog?.message || "-"}</p>
          <div
            className="import-workbench-progress-track"
            role="progressbar"
            aria-valuenow={toInt(exportDialog?.progress)}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <span style={{ width: `${Math.max(0, Math.min(100, toInt(exportDialog?.progress)))}%` }} />
          </div>
          <p>总体进度：{toInt(exportDialog?.progress)}%</p>
          <p>总体状态：{exportDialog?.message || "-"}</p>
          {exportDialog?.outputPath ? <p className="import-workbench-code-line">{exportDialog.outputPath}</p> : null}
          {exportDialog?.error ? <p className="hint-error">{exportDialog.error}</p> : null}
        </div>
      </Dialog>

      <ContextMenu open={contextMenuAnchor !== null} anchor={contextMenuAnchor} items={contextMenuItems} onClose={closeContextMenu} />
    </section>
  );
}
