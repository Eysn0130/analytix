import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from "react";
import type { CaseDetailDTO } from "../../cases/api";
import { getCaseDetail } from "../../cases/api";
import {
  type CleaningJobDTO,
  type CleaningStepSummaryDTO,
  listCleaningJobs,
  listCleaningStepSummaries
} from "../../../services/cleaning/api";
import { listImportFiles, type ImportFileLogDTO } from "../../../services/import/api";
import type { WsConnectionStatus } from "../../../services/ws/types";
import { toErrorMessage } from "../../shared/errors";
import { CleaningRequestAuthority } from "./cleaning-request-authority";

export interface CleaningContextSnapshot {
  caseId: string;
  caseDetail: CaseDetailDTO | null;
  importFiles: ImportFileLogDTO[];
  stepSummaries: CleaningStepSummaryDTO[];
}

interface UseCleaningWorkspaceDataOptions {
  active: boolean;
  activeCaseId: string;
  wsStatus: WsConnectionStatus;
  onError: (message: string) => void;
}

interface CleaningWorkspaceDataState {
  caseDetail: CaseDetailDTO | null;
  jobs: CleaningJobDTO[];
  setJobs: Dispatch<SetStateAction<CleaningJobDTO[]>>;
  importFiles: ImportFileLogDTO[];
  stepSummaries: CleaningStepSummaryDTO[];
  jobsLoading: boolean;
  contextLoading: boolean;
  loadJobs: () => Promise<void>;
  loadContext: () => Promise<CleaningContextSnapshot>;
  scheduleJobsRefresh: () => void;
  scheduleContextRefresh: () => void;
}

export function useCleaningWorkspaceData({
  active,
  activeCaseId,
  wsStatus,
  onError
}: UseCleaningWorkspaceDataOptions): CleaningWorkspaceDataState {
  const [caseDetail, setCaseDetail] = useState<CaseDetailDTO | null>(null);
  const [jobs, setJobs] = useState<CleaningJobDTO[]>([]);
  const [importFiles, setImportFiles] = useState<ImportFileLogDTO[]>([]);
  const [stepSummaries, setStepSummaries] = useState<CleaningStepSummaryDTO[]>([]);
  const [jobsLoading, setJobsLoading] = useState(false);
  const [contextLoading, setContextLoading] = useState(false);

  const previousWsStatusRef = useRef(wsStatus);
  const previousActiveRef = useRef(active);
  const activeRef = useRef(active);
  const jobsRefreshTimerRef = useRef<number | null>(null);
  const contextRefreshTimerRef = useRef<number | null>(null);
  const activationRefreshTimerRef = useRef<number | null>(null);
  const requestAuthorityRef = useRef(new CleaningRequestAuthority());
  const snapshotRef = useRef<CleaningContextSnapshot>({
    caseId: "",
    caseDetail: null,
    importFiles: [],
    stepSummaries: []
  });

  activeRef.current = active;
  const caseChanged = requestAuthorityRef.current.bindCase(activeCaseId);
  if (caseChanged) {
    snapshotRef.current = {
      caseId: activeCaseId,
      caseDetail: null,
      importFiles: [],
      stepSummaries: []
    };
  } else {
    snapshotRef.current = {
      caseId: activeCaseId,
      caseDetail,
      importFiles,
      stepSummaries
    };
  }

  const clearRefreshTimers = useCallback((): void => {
    if (jobsRefreshTimerRef.current !== null) {
      window.clearTimeout(jobsRefreshTimerRef.current);
      jobsRefreshTimerRef.current = null;
    }
    if (contextRefreshTimerRef.current !== null) {
      window.clearTimeout(contextRefreshTimerRef.current);
      contextRefreshTimerRef.current = null;
    }
    if (activationRefreshTimerRef.current !== null) {
      window.clearTimeout(activationRefreshTimerRef.current);
      activationRefreshTimerRef.current = null;
    }
  }, []);

  const loadJobs = useCallback(async (): Promise<void> => {
    const authority = requestAuthorityRef.current;
    const token = authority.issue("jobs");
    if (!activeRef.current || token.caseId !== activeCaseId) {
      return;
    }
    if (!activeCaseId) {
      if (authority.accepts(token)) {
        setJobs([]);
      }
      return;
    }

    if (authority.accepts(token)) {
      setJobsLoading(true);
    }
    try {
      const response = await listCleaningJobs({
        caseId: activeCaseId,
        page: 1,
        pageSize: 100
      });
      if (!activeRef.current || !authority.accepts(token)) {
        return;
      }
      if (response.items.some((item) => item.case_id !== token.caseId)) {
        throw new Error("cleaning_case_binding_mismatch");
      }
      onError("");
      setJobs(response.items);
    } catch (loadError) {
      if (activeRef.current && authority.accepts(token)) {
        onError(toErrorMessage(loadError));
      }
    } finally {
      if (activeRef.current && authority.accepts(token)) {
        setJobsLoading(false);
      }
    }
  }, [activeCaseId, onError]);

  const loadContext = useCallback(async (): Promise<CleaningContextSnapshot> => {
    const authority = requestAuthorityRef.current;
    const token = authority.issue("context");
    const emptySnapshot: CleaningContextSnapshot = {
      caseId: activeCaseId,
      caseDetail: null,
      importFiles: [],
      stepSummaries: []
    };
    if (!activeRef.current || token.caseId !== activeCaseId) {
      return emptySnapshot;
    }
    if (!activeCaseId) {
      if (authority.accepts(token)) {
        setCaseDetail(null);
        setImportFiles([]);
        setStepSummaries([]);
      }
      return emptySnapshot;
    }

    if (authority.accepts(token)) {
      setContextLoading(true);
    }
    let nextCaseDetail: CaseDetailDTO | null = null;
    let nextImportFiles: ImportFileLogDTO[] = [];
    let nextStepSummaries: CleaningStepSummaryDTO[] = [];
    try {
      const [caseResult, filesResult, stepsResult] = await Promise.allSettled([
        getCaseDetail(activeCaseId),
        listImportFiles(activeCaseId),
        listCleaningStepSummaries(activeCaseId)
      ]);
      if (!activeRef.current || !authority.accepts(token)) {
        return emptySnapshot;
      }

      nextCaseDetail = caseResult.status === "fulfilled" && caseResult.value.case_id === token.caseId
        ? caseResult.value
        : null;
      setCaseDetail(nextCaseDetail);

      nextImportFiles = filesResult.status === "fulfilled" ? filesResult.value.items : [];
      setImportFiles(nextImportFiles);

      nextStepSummaries = stepsResult.status === "fulfilled" && stepsResult.value.case_id === token.caseId
        ? stepsResult.value.items
        : [];
      setStepSummaries(nextStepSummaries);

      const errors = [caseResult, filesResult, stepsResult]
        .filter((result): result is PromiseRejectedResult => result.status === "rejected")
        .map((result) => toErrorMessage(result.reason))
        .filter(Boolean);
      if (caseResult.status === "fulfilled" && caseResult.value.case_id !== token.caseId) {
        errors.unshift("cleaning_case_binding_mismatch");
      }
      if (stepsResult.status === "fulfilled" && stepsResult.value.case_id !== token.caseId) {
        errors.unshift("cleaning_case_binding_mismatch");
      }
      if (errors.length) {
        onError(errors[0]);
      } else {
        onError("");
      }
    } catch (loadError) {
      if (activeRef.current && authority.accepts(token)) {
        onError(toErrorMessage(loadError));
      }
    } finally {
      if (activeRef.current && authority.accepts(token)) {
        setContextLoading(false);
      }
    }
    return {
      caseId: activeCaseId,
      caseDetail: nextCaseDetail,
      importFiles: nextImportFiles,
      stepSummaries: nextStepSummaries
    };
  }, [activeCaseId, onError]);

  const scheduleJobsRefresh = useCallback(() => {
    if (!active) {
      return;
    }
    if (jobsRefreshTimerRef.current !== null) {
      return;
    }
    jobsRefreshTimerRef.current = window.setTimeout(() => {
      jobsRefreshTimerRef.current = null;
      if (!activeRef.current) {
        return;
      }
      void loadJobs();
    }, 180);
  }, [active, loadJobs]);

  const scheduleContextRefresh = useCallback(() => {
    if (!active) {
      return;
    }
    if (contextRefreshTimerRef.current !== null) {
      return;
    }
    contextRefreshTimerRef.current = window.setTimeout(() => {
      contextRefreshTimerRef.current = null;
      if (!activeRef.current) {
        return;
      }
      void loadContext();
    }, 260);
  }, [active, loadContext]);

  useEffect(() => {
    if (!active) {
      return;
    }
    void loadContext();
  }, [active, loadContext]);

  useEffect(() => {
    if (!active) {
      return;
    }
    void loadJobs();
  }, [active, loadJobs]);

  useEffect(() => clearRefreshTimers, [clearRefreshTimers]);

  useEffect(() => {
    if (!active) {
      clearRefreshTimers();
    }
  }, [active, clearRefreshTimers]);

  useEffect(() => {
    const wasActive = previousActiveRef.current;
    previousActiveRef.current = active;
    if (!active || wasActive) {
      return;
    }

    void Promise.all([loadJobs(), loadContext()]);

    if (activationRefreshTimerRef.current !== null) {
      window.clearTimeout(activationRefreshTimerRef.current);
    }
    // Import completion and context persistence can land a moment after route switching.
    activationRefreshTimerRef.current = window.setTimeout(() => {
      activationRefreshTimerRef.current = null;
      if (!previousActiveRef.current || !activeRef.current) {
        return;
      }
      void Promise.all([loadJobs(), loadContext()]);
    }, 420);
  }, [active, loadContext, loadJobs]);

  useEffect(() => {
    const previous = previousWsStatusRef.current;
    if (active && activeCaseId && wsStatus === "open" && previous !== "open") {
      void loadJobs();
      void loadContext();
    }
    previousWsStatusRef.current = wsStatus;
  }, [active, activeCaseId, loadContext, loadJobs, wsStatus]);

  useEffect(() => {
    clearRefreshTimers();
    setCaseDetail(null);
    setJobs([]);
    setImportFiles([]);
    setStepSummaries([]);
    setJobsLoading(false);
    setContextLoading(false);
  }, [activeCaseId, clearRefreshTimers]);

  return {
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
  };
}
