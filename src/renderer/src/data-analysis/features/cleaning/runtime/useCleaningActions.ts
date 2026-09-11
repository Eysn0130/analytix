import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { task } from "../../../components/feedback/task";
import { showToast, toToastErrorDetail } from "../../../components/feedback/toast-copy";
import { controlledArtifactPublicationBlockReason } from "../../../services/publication-quarantine";
import type { BackendHealth } from "../../../services/http/system";
import {
  type CleaningJobDTO,
  createCleaningJob,
} from "../../../services/cleaning/api";
import type { ImportFileLogDTO } from "../../../services/import/api";
import { cancelExportJob, createCleanedExportJob, createRawExportJob } from "../../cases/api";
import type { CaseDetailDTO } from "../../cases/api";
import { toErrorMessage } from "../../shared/errors";
import { uncontrolledRestrictedPiiExportBlockReason } from "../../shared/ordinary-pii-projection";
import { toButtonActionErrorMessage } from "../api/errors";
import {
  buildCleanedExportNotReadyActionBlock,
  buildCleaningActiveExportActionBlock,
  buildExportBusyActionBlock,
  buildNoCleanableDataActionBlock,
  buildOpenCaseRequiredActionBlock,
  buildRunningCleaningJobActionBlock,
  buildUnverifiedCleanableDataActionBlock,
  type CleaningActionBlockViewModel
} from "../model/action-blocks";
import {
  CLEANING_RUNTIME_HEALTH_UNKNOWN_MESSAGE,
  buildCleaningJobCreateFailedFeedback,
  buildCleaningRuntimeHealthUnknownFeedback,
  buildCleaningRuntimeUnavailableFeedback,
  buildExportCancelFailedFeedback,
  buildExportCancelRequestedFeedback,
  buildExportJobCreateFailedFeedback,
  buildExportJobCreatedHint,
  type CleaningActionFeedbackViewModel
} from "../model/action-feedback";
import { buildCancelingExportState, buildRunningExportState, getCleaningExportTaskCopy } from "../model/export";
import { buildCleaningAvailability } from "../model/status";
import type { CleaningAvailability, CleaningRunMode, ExportState } from "../model/types";
import { getCleaningRuntimeUnavailableMessage } from "./health";
import type { CleaningContextSnapshot } from "./useCleaningWorkspaceData";

export type CleaningActionName = "" | "run" | "reclean" | "export-cleaned" | "export-raw";

interface UseCleaningActionsOptions {
  activeCaseId: string;
  caseName: string;
  caseDetail: CaseDetailDTO | null;
  importFiles: ImportFileLogDTO[];
  contextLoading: boolean;
  currentJob: CleaningJobDTO | null;
  cleanedExportReady: boolean;
  cleaningAvailability: CleaningAvailability;
  cleaningRuntimeHealth: BackendHealth | null;
  exportState: ExportState;
  setExportState: Dispatch<SetStateAction<ExportState>>;
  setError: (message: string) => void;
  setHint: (message: string) => void;
  setCleaningRuntimeHealthError: (message: string) => void;
  loadJobs: () => Promise<void>;
  loadContext: () => Promise<CleaningContextSnapshot>;
  syncCleaningRuntimeHealth: () => Promise<BackendHealth>;
  showCleaningRunningToast: (jobId: string) => void;
  startExportTracking: (jobId: string) => void;
  stopExportTracking: () => void;
}

interface CleaningActions {
  exportBusy: boolean;
  runActionWorking: boolean;
  recleanActionWorking: boolean;
  exportRawWorking: boolean;
  exportCleanedWorking: boolean;
  onRunCleaning: (forceRebuild?: boolean) => Promise<void>;
  onExportCleaned: () => Promise<void>;
  onExportRaw: () => Promise<void>;
  onCancelExport: () => Promise<void>;
  onOpenExportOutput: () => Promise<void>;
}

function showActionBlock(block: CleaningActionBlockViewModel, setError: (message: string) => void): void {
  setError(block.message);
  showToast({
    tone: block.tone,
    title: block.title,
    detail: block.detail,
  });
}

function showActionFeedback(feedback: CleaningActionFeedbackViewModel): void {
  showToast({
    tone: feedback.tone,
    title: feedback.title,
    detail: feedback.detail,
  });
}

function showErrorFeedback(feedback: CleaningActionFeedbackViewModel, setError: (message: string) => void): void {
  setError(feedback.message || "");
  showActionFeedback(feedback);
}

export function useCleaningActions({
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
  stopExportTracking,
}: UseCleaningActionsOptions): CleaningActions {
  const [runningAction, setRunningAction] = useState<CleaningActionName>("");
  const [activeRunMode, setActiveRunMode] = useState<CleaningRunMode>("");
  const manualActionGateRef = useRef(false);
  const activeCaseIdRef = useRef(activeCaseId);
  activeCaseIdRef.current = activeCaseId;

  const exportBusy = runningAction === "export-raw" || runningAction === "export-cleaned" || exportState.status === "running";
  const runActionWorking = runningAction === "run" || (activeRunMode === "run" && Boolean(currentJob));
  const recleanActionWorking = runningAction === "reclean" || (activeRunMode === "reclean" && Boolean(currentJob));
  const exportRawWorking = runningAction === "export-raw" || (exportState.kind === "raw" && exportState.status === "running");
  const exportCleanedWorking =
    runningAction === "export-cleaned" || (exportState.kind === "cleaned" && exportState.status === "running");

  useEffect(() => {
    if (!currentJob) {
      setActiveRunMode("");
    }
  }, [currentJob]);

  const onRunCleaning = useCallback(
    async (forceRebuild = false): Promise<void> => {
      if (manualActionGateRef.current) {
        return;
      }
      if (!activeCaseId) {
        showActionBlock(buildOpenCaseRequiredActionBlock("cleaning"), setError);
        return;
      }
      if (currentJob) {
        showActionBlock(buildRunningCleaningJobActionBlock(currentJob.job_id), setError);
        return;
      }
      const frozenCaseId = activeCaseId;
      manualActionGateRef.current = true;
      setRunningAction(forceRebuild ? "reclean" : "run");
      setError("");
      setHint("");
      try {
        let runtimeHealth = cleaningRuntimeHealth;
        try {
          runtimeHealth = await syncCleaningRuntimeHealth();
          if (activeCaseIdRef.current !== frozenCaseId) {
            return;
          }
        } catch (healthError) {
          const detail = toErrorMessage(healthError);
          setCleaningRuntimeHealthError(detail);
          const feedback = buildCleaningRuntimeHealthUnknownFeedback(
            toToastErrorDetail(detail, CLEANING_RUNTIME_HEALTH_UNKNOWN_MESSAGE)
          );
          showErrorFeedback(feedback, setError);
          return;
        }
        const runtimeUnavailableMessage = getCleaningRuntimeUnavailableMessage(runtimeHealth);
        if (runtimeUnavailableMessage) {
          showErrorFeedback(
            buildCleaningRuntimeUnavailableFeedback(runtimeUnavailableMessage, toToastErrorDetail(runtimeUnavailableMessage)),
            setError
          );
          return;
        }
        const contextSnapshot =
          contextLoading || (cleaningAvailability.hasTransactionData !== true && importFiles.length === 0)
            ? await loadContext()
            : {
                caseId: frozenCaseId,
                caseDetail,
                importFiles,
                stepSummaries: []
              };
        if (activeCaseIdRef.current !== frozenCaseId || contextSnapshot.caseId !== frozenCaseId) {
          return;
        }
        const nextAvailability = buildCleaningAvailability(contextSnapshot.importFiles, contextSnapshot.caseDetail);
        if (nextAvailability.hasTransactionData === null) {
          showActionBlock(buildUnverifiedCleanableDataActionBlock(), setError);
          return;
        }
        if (nextAvailability.hasTransactionData === false) {
          showActionBlock(buildNoCleanableDataActionBlock(), setError);
          return;
        }
        const created = await createCleaningJob({
          case_id: frozenCaseId,
          steps: [],
          force_rebuild: forceRebuild,
        });
        if (activeCaseIdRef.current !== frozenCaseId || created.case_id !== frozenCaseId) {
          return;
        }
        setActiveRunMode(forceRebuild ? "reclean" : "run");
        showCleaningRunningToast(created.job_id);
        await loadJobs();
      } catch (runError) {
        const message = toButtonActionErrorMessage(runError);
        showErrorFeedback(buildCleaningJobCreateFailedFeedback(message, toToastErrorDetail(message)), setError);
      } finally {
        setRunningAction("");
        manualActionGateRef.current = false;
      }
    },
    [
      activeCaseId,
      caseDetail,
      cleaningAvailability,
      cleaningRuntimeHealth,
      contextLoading,
      currentJob,
      importFiles,
      loadContext,
      loadJobs,
      setCleaningRuntimeHealthError,
      setError,
      setHint,
      showCleaningRunningToast,
      syncCleaningRuntimeHealth,
    ]
  );

  const onExportCleaned = useCallback(async (): Promise<void> => {
    const publicationBlockReason = controlledArtifactPublicationBlockReason();
    if (publicationBlockReason) {
      setError(publicationBlockReason);
      showToast({ tone: "warning", title: "受控导出已阻止", detail: publicationBlockReason });
      return;
    }
    if (manualActionGateRef.current) {
      return;
    }
    const exportBlockReason = uncontrolledRestrictedPiiExportBlockReason();
    if (exportBlockReason) {
      setError(exportBlockReason);
      showToast({ tone: "warning", title: "完整导出已阻止", detail: exportBlockReason });
      return;
    }
    if (!activeCaseId) {
      showActionBlock(buildOpenCaseRequiredActionBlock("export"), setError);
      return;
    }
    if (exportBusy) {
      showActionBlock(buildExportBusyActionBlock(), setError);
      return;
    }
    if (currentJob) {
      showActionBlock(buildCleaningActiveExportActionBlock(), setError);
      return;
    }
    if (!cleanedExportReady) {
      showActionBlock(buildCleanedExportNotReadyActionBlock(), setError);
      return;
    }
    manualActionGateRef.current = true;
    setRunningAction("export-cleaned");
    setError("");
    setHint("");

    try {
      const taskCopy = getCleaningExportTaskCopy("cleaned");
      stopExportTracking();
      await task.run(
        {
          title: taskCopy.runningTitle,
          detail: taskCopy.runningDetail,
          successTitle: taskCopy.successTitle,
          successDetail: taskCopy.successDetail,
        },
        async () => {
          const job = await createCleanedExportJob({
            case_id: activeCaseId,
            export_format: "xlsx",
            filters: {},
            output_name: `${caseName || activeCaseId}-已清洗导出`,
            target_dir: undefined,
          });
          setExportState(buildRunningExportState({
            jobId: job.job_id,
            kind: "cleaned",
            progress: job.progress,
          }));
          startExportTracking(job.job_id);
          setHint(buildExportJobCreatedHint(job.job_id));
          return job;
        }
      );
    } catch (exportError) {
      const message = toButtonActionErrorMessage(exportError);
      showErrorFeedback(buildExportJobCreateFailedFeedback(message, toToastErrorDetail(message)), setError);
    } finally {
      setRunningAction("");
      manualActionGateRef.current = false;
    }
  }, [
    activeCaseId,
    caseName,
    cleanedExportReady,
    currentJob,
    exportBusy,
    setError,
    setExportState,
    setHint,
    startExportTracking,
    stopExportTracking,
  ]);

  const onExportRaw = useCallback(async (): Promise<void> => {
    const publicationBlockReason = controlledArtifactPublicationBlockReason();
    if (publicationBlockReason) {
      setError(publicationBlockReason);
      showToast({ tone: "warning", title: "受控导出已阻止", detail: publicationBlockReason });
      return;
    }
    if (manualActionGateRef.current) {
      return;
    }
    const exportBlockReason = uncontrolledRestrictedPiiExportBlockReason();
    if (exportBlockReason) {
      setError(exportBlockReason);
      showToast({ tone: "warning", title: "完整导出已阻止", detail: exportBlockReason });
      return;
    }
    if (!activeCaseId) {
      showActionBlock(buildOpenCaseRequiredActionBlock("export"), setError);
      return;
    }
    if (exportBusy) {
      showActionBlock(buildExportBusyActionBlock(), setError);
      return;
    }
    manualActionGateRef.current = true;
    setRunningAction("export-raw");
    setError("");
    setHint("");

    try {
      const taskCopy = getCleaningExportTaskCopy("raw");
      stopExportTracking();
      await task.run(
        {
          title: taskCopy.runningTitle,
          detail: taskCopy.runningDetail,
          successTitle: taskCopy.successTitle,
          successDetail: taskCopy.successDetail,
        },
        async () => {
          const job = await createRawExportJob({
            case_id: activeCaseId,
            export_format: "xlsx",
            filters: {},
            output_name: `${caseName || activeCaseId}-未清洗导出`,
            target_dir: undefined,
          });
          setExportState(buildRunningExportState({
            jobId: job.job_id,
            kind: "raw",
            progress: job.progress,
          }));
          startExportTracking(job.job_id);
          setHint(buildExportJobCreatedHint(job.job_id));
          return job;
        }
      );
    } catch (exportError) {
      const message = toButtonActionErrorMessage(exportError);
      showErrorFeedback(buildExportJobCreateFailedFeedback(message, toToastErrorDetail(message)), setError);
    } finally {
      setRunningAction("");
      manualActionGateRef.current = false;
    }
  }, [
    activeCaseId,
    caseName,
    exportBusy,
    setError,
    setExportState,
    setHint,
    startExportTracking,
    stopExportTracking,
  ]);

  const onCancelExport = useCallback(async (): Promise<void> => {
    if (!exportState.jobId) {
      return;
    }
    try {
      await cancelExportJob(exportState.jobId, activeCaseId);
      const feedback = buildExportCancelRequestedFeedback(exportState.jobId);
      setHint(feedback.hint || "");
      showActionFeedback(feedback);
      setExportState((prev) => buildCancelingExportState(prev));
    } catch (cancelError) {
      const message = toErrorMessage(cancelError);
      showErrorFeedback(buildExportCancelFailedFeedback(message, toToastErrorDetail(message)), setError);
    }
  }, [activeCaseId, exportState.jobId, setError, setExportState, setHint]);

  const onOpenExportOutput = useCallback(async (): Promise<void> => {
    const reason = controlledArtifactPublicationBlockReason();
    setError(reason);
    showToast({ tone: "warning", title: "受控 artifact 访问已阻止", detail: reason });
  }, [setError]);

  return {
    exportBusy,
    runActionWorking,
    recleanActionWorking,
    exportRawWorking,
    exportCleanedWorking,
    onRunCleaning,
    onExportCleaned,
    onExportRaw,
    onCancelExport,
    onOpenExportOutput,
  };
}
