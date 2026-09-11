import { useEffect, useMemo, useRef, useState } from "react";
import type { FundsDeterministicCleaningResult } from "../../../../../shared/analytix-api";
import { CleaningDiffPreview } from "../../../components/chat/AcceptedSlotDisplay";
import { useAppStore } from "../../store/app-store";
import { CleaningEmptyState } from "./components/CleaningEmptyState";
import { CleaningExportStrip } from "./components/CleaningExportStrip";
import { CleaningHero } from "./components/CleaningHero";
import { CleaningProcessFlow } from "./components/CleaningProcessFlow";
import { CleaningStepDetailDialog } from "./components/CleaningStepDetailDialog";
import { CleaningStepInsightGrid } from "./components/CleaningStepInsightGrid";
import { CleaningSummaryGrid } from "./components/CleaningSummaryGrid";
import { buildCleaningFileStats } from "./model/file-stats";
import { buildIdleExportState } from "./model/export";
import { buildCleaningHeroActionsViewModel } from "./model/hero-actions";
import { buildCleaningStepDetailDialogViewModel } from "./model/step-detail-render-model";
import { buildCleaningAvailability } from "./model/status";
import {
  buildCleanedExportReadinessViewModel,
  buildCleaningProcessNodes,
  buildCleaningSummaryCards,
  buildCleaningStepDetailFooterViewModel,
  buildCleaningStepImpactViewModel,
  buildCleaningStepViewModels,
  getCleaningBoardStatus,
  getCleaningBoardStatusLabel
} from "./model/view-model";
import type {
  ExportState
} from "./model/types";
import { getCleaningRuntimeUnavailableMessage } from "./runtime/health";
import { useCleaningActions } from "./runtime/useCleaningActions";
import { useCleaningExportTracking } from "./runtime/useCleaningExportTracking";
import { useCleaningJobNotifications } from "./runtime/useCleaningJobNotifications";
import { useCleaningPageViewport } from "./runtime/useCleaningPageViewport";
import { useCleaningRealtimeEvents } from "./runtime/useCleaningRealtimeEvents";
import { useCleaningRuntimeHealth } from "./runtime/useCleaningRuntimeHealth";
import { useCleaningStepDetail } from "./runtime/useCleaningStepDetail";
import { useCleaningWorkspaceData } from "./runtime/useCleaningWorkspaceData";
import "./styles/cleaning-page.css";

export function CleaningPage({ active = true }: { active?: boolean }): JSX.Element {
  const {
    state: {
      session: { activeCaseId },
      runtime: { lastWsEvent, wsStatus }
    }
  } = useAppStore();

  const [error, setError] = useState("");
  const {
    caseDetail,
    jobs,
    setJobs,
    importFiles,
    stepSummaries,
    jobsLoading,
    contextLoading,
    loadJobs,
    loadContext,
    scheduleJobsRefresh,
    scheduleContextRefresh
  } = useCleaningWorkspaceData({
    active,
    activeCaseId,
    wsStatus,
    onError: setError
  });
  const [hint, setHint] = useState("");
  const [exportState, setExportState] = useState<ExportState>(buildIdleExportState);
  const [deterministicCleaning, setDeterministicCleaning] = useState<FundsDeterministicCleaningResult | null>(null);
  const [deterministicCleaningWorking, setDeterministicCleaningWorking] = useState(false);
  const deterministicInvocationRef = useRef(0);
  const activeCaseIdRef = useRef(activeCaseId);
  activeCaseIdRef.current = activeCaseId;

  const pageRef = useRef<HTMLElement | null>(null);
  const {
    cleaningRuntimeHealth,
    cleaningRuntimeHealthError,
    setCleaningRuntimeHealthError,
    syncCleaningRuntimeHealth
  } = useCleaningRuntimeHealth(active);

  const {
    dismissCleaningToast,
    notifyCleaningCompletionByJobId,
    showCleaningRunningToast
  } = useCleaningJobNotifications({
    activeCaseId,
    jobs
  });

  const orderedJobs = useMemo(() => {
    return [...jobs].sort((left, right) => {
      return new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime();
    });
  }, [jobs]);

  const currentJob = useMemo(
    () => orderedJobs.find((item) => item.status === "running" || item.status === "queued") ?? null,
    [orderedJobs]
  );
  const latestSucceededJob = useMemo(
    () => orderedJobs.find((item) => item.status === "succeeded") ?? null,
    [orderedJobs]
  );
  const latestJob = orderedJobs[0] ?? null;
  const caseName = caseDetail?.case_name || activeCaseId;

  const fileStats = useMemo(() => {
    return buildCleaningFileStats(importFiles);
  }, [importFiles]);

  const cleaningAvailability = useMemo(() => {
    return buildCleaningAvailability(importFiles, caseDetail);
  }, [caseDetail, importFiles]);
  const cleaningRuntimeUnavailableMessage = useMemo(() => {
    return getCleaningRuntimeUnavailableMessage(cleaningRuntimeHealth);
  }, [cleaningRuntimeHealth]);
  const cleaningRuntimeUnavailable = Boolean(cleaningRuntimeUnavailableMessage);
  const cleaningRuntimeBanner = cleaningRuntimeUnavailableMessage || (cleaningRuntimeHealthError ? "清洗运行时状态暂时无法确认，请稍后重试。" : "");
  const cleanedExportViewModel = useMemo(
    () => buildCleanedExportReadinessViewModel(cleaningAvailability, currentJob, latestSucceededJob),
    [cleaningAvailability, currentJob, latestSucceededJob]
  );
  const cleanedExportReady = cleanedExportViewModel.ready;
  const cleanedExportButtonTitle = cleanedExportViewModel.buttonTitle;
  const cleanedExportHint = cleanedExportViewModel.hint;
  const cleanedOutputReady = cleanedExportReady || Boolean(latestSucceededJob);

  const enrichedSteps = useMemo(() => buildCleaningStepViewModels(stepSummaries), [stepSummaries]);

  const {
    detailStep,
    detailData,
    detailLoading,
    detailError,
    detailColumns,
    detailRows,
    detailTopSpacerHeight,
    detailBottomSpacerHeight,
    detailTableWrapRef,
    openStepDetail,
    closeStepDetail,
    onDetailTableScroll,
    onDetailTableWheel
  } = useCleaningStepDetail({ activeCaseId });
  const detailFooter = useMemo(
    () => buildCleaningStepDetailFooterViewModel(detailData, detailError),
    [detailData, detailError]
  );
  const detailDialog = useMemo(
    () =>
      buildCleaningStepDetailDialogViewModel({
        detailData,
        detailError,
        footerLabel: detailFooter?.label || "",
        step: detailStep
      }),
    [detailData, detailError, detailFooter?.label, detailStep]
  );

  const { accountStepAffected, stepInsightMeta, txnStepAffected } = useMemo(
    () => buildCleaningStepImpactViewModel(enrichedSteps),
    [enrichedSteps]
  );
  const summaryCards = useMemo(
    () =>
      buildCleaningSummaryCards({
        accountStepAffected,
        caseDetail,
        cleanedOutputReady,
        cleaningAvailability,
        currentJob,
        fileStats,
        latestSucceededJob,
        txnStepAffected
      }),
    [accountStepAffected, caseDetail, cleanedOutputReady, cleaningAvailability, currentJob, fileStats, latestSucceededJob, txnStepAffected]
  );

  const boardStatus = useMemo(
    () => getCleaningBoardStatus(currentJob, latestJob, latestSucceededJob, cleanedOutputReady),
    [cleanedOutputReady, currentJob, latestJob, latestSucceededJob]
  );
  const boardStatusLabel = useMemo(
    () =>
      getCleaningBoardStatusLabel({
        currentJob,
        latestJob,
        latestSucceededJob,
        cleanedOutputReady,
        contextLoading,
        jobsLoading
      }),
    [cleanedOutputReady, contextLoading, currentJob, jobsLoading, latestJob, latestSucceededJob]
  );

  const processNodes = useMemo(
    () =>
      buildCleaningProcessNodes({
        fileStats,
        caseDetail,
        caseName,
        cleanedOutputReady,
        currentJob,
        latestSucceededJob,
        cleaningAvailability,
        orderedJobCount: orderedJobs.length,
        wsStatus
      }),
    [
      caseDetail,
      caseName,
      cleanedOutputReady,
      cleaningAvailability,
      currentJob,
      fileStats,
      latestSucceededJob,
      orderedJobs.length,
      wsStatus
    ]
  );

  useCleaningRealtimeEvents({
    active,
    activeCaseId,
    dismissCleaningToast,
    lastWsEvent,
    notifyCleaningCompletionByJobId,
    scheduleContextRefresh,
    scheduleJobsRefresh,
    setJobs
  });

  const { startExportTracking, stopExportTracking } = useCleaningExportTracking({
    activeCaseId,
    exportState,
    setExportState,
    wsStatus,
    lastWsEvent,
    setError,
    setHint
  });

  const {
    exportBusy,
    runActionWorking,
    recleanActionWorking,
    exportRawWorking,
    exportCleanedWorking,
    onExportCleaned,
    onExportRaw,
    onCancelExport,
    onOpenExportOutput
  } = useCleaningActions({
    activeCaseId,
    caseName,
    caseDetail,
    importFiles,
    contextLoading,
    currentJob,
    cleanedExportReady,
    cleaningAvailability,
    cleaningRuntimeHealth,
    exportState,
    setExportState,
    setError,
    setHint,
    setCleaningRuntimeHealthError,
    loadJobs,
    loadContext,
    syncCleaningRuntimeHealth,
    showCleaningRunningToast,
    startExportTracking,
    stopExportTracking
  });

  useEffect(() => {
    deterministicInvocationRef.current += 1;
    setDeterministicCleaning(null);
    setDeterministicCleaningWorking(false);
  }, [active, activeCaseId]);

  const onRunDeterministicCleaning = async (): Promise<void> => {
    if (deterministicCleaningWorking) return;
    const invocation = deterministicInvocationRef.current + 1;
    deterministicInvocationRef.current = invocation;
    const caseAtInvocation = activeCaseId;
    setDeterministicCleaningWorking(true);
    setDeterministicCleaning(null);
    setError("");
    setHint("");
    try {
      const result = await window.analytix.runtime.runDeterministicFundsCleaning();
      if (deterministicInvocationRef.current !== invocation || activeCaseIdRef.current !== caseAtInvocation) return;
      setDeterministicCleaning(result);
      if (result.ok) {
        setHint(`确定性清洗已提交：${result.changedRowCount}/${result.rowCount} 行发生变化。`);
      } else {
        setError(result.message);
      }
    } catch {
      if (deterministicInvocationRef.current === invocation && activeCaseIdRef.current === caseAtInvocation) {
        setError("确定性清洗结果未知，请先核对当前数据快照再重试。");
      }
    } finally {
      if (deterministicInvocationRef.current === invocation) {
        setDeterministicCleaningWorking(false);
      }
    }
  };

  const cleaningPageStyle = useCleaningPageViewport({ active, pageRef });
  const heroActions = useMemo(
    () =>
      buildCleaningHeroActionsViewModel({
        cleanedExportButtonTitle,
        cleanedExportReady,
        cleaningAvailability,
        cleaningRuntimeUnavailable,
        cleaningRuntimeUnavailableMessage,
        currentJobActive: Boolean(currentJob),
        exportBusy,
        exportCleanedWorking,
        exportRawWorking,
        recleanActionWorking: recleanActionWorking || deterministicCleaningWorking,
        runActionWorking: runActionWorking || deterministicCleaningWorking
      }),
    [
      cleanedExportButtonTitle,
      cleanedExportReady,
      cleaningAvailability,
      cleaningRuntimeUnavailable,
      cleaningRuntimeUnavailableMessage,
      currentJob,
      exportBusy,
      exportCleanedWorking,
      exportRawWorking,
      recleanActionWorking,
      runActionWorking,
      deterministicCleaningWorking
    ]
  );

  if (!activeCaseId) {
    return (
      <section
        ref={pageRef}
        className="page-wrap stack cleaning-page cleaning-page--proportional cleaning-page--empty"
        style={cleaningPageStyle}
      >
        <div className="cleaning-shell">
          <div className="cleaning-topbar-spacer" aria-hidden="true" />
          <CleaningEmptyState />
        </div>
      </section>
    );
  }

  return (
    <section
      ref={pageRef}
      className="page-wrap stack cleaning-page cleaning-page--proportional"
      style={cleaningPageStyle}
    >
      <div className="cleaning-shell">
        <div className="cleaning-topbar-spacer" aria-hidden="true" />
        <div className="cleaning-dashboard">
          <CleaningHero
            actions={heroActions}
            boardStatus={boardStatus}
            boardStatusLabel={boardStatusLabel}
            cleanedExportHint={cleanedExportHint}
            cleanedExportReady={cleanedExportReady}
            onExportCleaned={onExportCleaned}
            onExportRaw={onExportRaw}
            onRunCleaning={onRunDeterministicCleaning}
          />

          <CleaningExportStrip
            exportState={exportState}
            onCancelExport={() => void onCancelExport()}
            onOpenExportOutput={() => void onOpenExportOutput()}
          />

          {hint ? <div className="cleaning-banner is-success">{hint}</div> : null}
          {cleaningRuntimeBanner ? <div className="cleaning-banner is-error">{cleaningRuntimeBanner}</div> : null}
          {error ? <div className="cleaning-banner is-error">{error}</div> : null}

          {deterministicCleaning?.ok ? (
            <section aria-label="确定性清洗差异预览">
              <CleaningDiffPreview selector={deterministicCleaning.selector} />
            </section>
          ) : null}

          <CleaningSummaryGrid cards={summaryCards} />

          <CleaningProcessFlow
            active={active}
            status={boardStatus}
            nodes={processNodes}
          />

          <CleaningStepInsightGrid
            metaLabel={stepInsightMeta}
            selectedStep={detailStep}
            steps={enrichedSteps}
            onOpenStepDetail={openStepDetail}
          />
        </div>
      </div>

      <CleaningStepDetailDialog
        dialog={detailDialog}
        columns={detailColumns}
        rows={detailRows}
        loading={detailLoading}
        onClose={closeStepDetail}
        tableWrapRef={detailTableWrapRef}
        onTableScroll={onDetailTableScroll}
        onTableWheel={onDetailTableWheel}
        topSpacerHeight={detailTopSpacerHeight}
        bottomSpacerHeight={detailBottomSpacerHeight}
      />
    </section>
  );
}
