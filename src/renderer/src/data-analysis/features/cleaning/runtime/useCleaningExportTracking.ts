import { useCallback, useEffect, useRef, type Dispatch, type SetStateAction } from "react";
import { compactToastPath, showToast, toToastErrorDetail } from "../../../components/feedback/toast-copy";
import { getExportJob } from "../../cases/api";
import { toErrorMessage } from "../../shared/errors";
import type { WsConnectionStatus, WsEventEnvelope } from "../../../services/ws/types";
import {
  EXPORT_TRACK_POLL_OFFLINE_MS,
  EXPORT_TRACK_POLL_WS_FALLBACK_MS
} from "../model/constants";
import {
  EXPORT_TERMINAL_RETRY_MESSAGE,
  buildExportTerminalActionFeedbackViewModel,
  buildExportTrackingSyncFailedFeedback,
  type CleaningActionFeedbackEffectViewModel,
  type CleaningActionFeedbackViewModel
} from "../model/action-feedback";
import {
  applyExportJobSnapshotState,
  buildExportJobSnapshotProjection,
  isTerminalExportStateStatus,
  type TerminalExportStateStatus
} from "../model/export";
import {
  applyExportRealtimeState,
  buildExportRealtimeProjection,
  getExportRealtimeTerminalAction,
  isExportRealtimeEventForCase
} from "../model/realtime-events";
import type { ExportState } from "../model/types";

interface UseCleaningExportTrackingOptions {
  activeCaseId: string;
  exportState: ExportState;
  setExportState: Dispatch<SetStateAction<ExportState>>;
  wsStatus: WsConnectionStatus;
  lastWsEvent: WsEventEnvelope | null;
  setError: (message: string) => void;
  setHint: (message: string) => void;
}

interface CleaningExportTracking {
  startExportTracking: (jobId: string) => void;
  stopExportTracking: () => void;
}

function showExportTrackingFeedback(feedback: CleaningActionFeedbackViewModel): void {
  showToast({
    tone: feedback.tone,
    title: feedback.title,
    detail: feedback.detail
  });
}

function applyExportTrackingFeedback(
  effect: CleaningActionFeedbackEffectViewModel,
  setError: (message: string) => void,
  setHint: (message: string) => void
): void {
  if (effect.target === "error") {
    setError(effect.feedback.message || "");
  } else {
    setHint(effect.feedback.hint || "");
  }
  showExportTrackingFeedback(effect.feedback);
}

export function useCleaningExportTracking({
  activeCaseId,
  exportState,
  setExportState,
  wsStatus,
  lastWsEvent,
  setError,
  setHint
}: UseCleaningExportTrackingOptions): CleaningExportTracking {
  const exportTrackRef = useRef<{ jobId: string; stopped: boolean; terminalNotified: string }>({
    jobId: "",
    stopped: true,
    terminalNotified: ""
  });

  const notifyExportTerminal = useCallback(
    (jobId: string, status: TerminalExportStateStatus, outputPath: string, errorText: string): void => {
      if (exportTrackRef.current.jobId !== jobId) {
        return;
      }
      if (exportTrackRef.current.terminalNotified === status) {
        return;
      }
      exportTrackRef.current.terminalNotified = status;
      applyExportTrackingFeedback(
        buildExportTerminalActionFeedbackViewModel({
          compactPath: outputPath ? compactToastPath(outputPath) : "",
          errorDetail: toToastErrorDetail(errorText || EXPORT_TERMINAL_RETRY_MESSAGE),
          errorText,
          outputPath,
          status
        }),
        setError,
        setHint
      );
    },
    [setError, setHint]
  );

  const syncExportJobSnapshot = useCallback(
    async (jobId: string, options: { notifyTerminal?: boolean; silentNetworkError?: boolean } = {}): Promise<void> => {
      if (!jobId || exportTrackRef.current.stopped) {
        return;
      }
      try {
        const current = await getExportJob(jobId, activeCaseId);
        const snapshot = buildExportJobSnapshotProjection(current);
        setExportState((prev) => applyExportJobSnapshotState(prev, jobId, snapshot));

        if (options.notifyTerminal && isTerminalExportStateStatus(snapshot.status)) {
          notifyExportTerminal(jobId, snapshot.status, snapshot.outputPath, snapshot.errorText);
        }
      } catch (trackError) {
        if (options.silentNetworkError) {
          return;
        }
        const message = toErrorMessage(trackError);
        const feedback = buildExportTrackingSyncFailedFeedback(message, toToastErrorDetail(message));
        setError(feedback.message || "");
        showExportTrackingFeedback(feedback);
      }
    },
    [activeCaseId, notifyExportTerminal, setError, setExportState]
  );

  const stopExportTracking = useCallback((): void => {
    exportTrackRef.current.stopped = true;
    exportTrackRef.current.jobId = "";
    exportTrackRef.current.terminalNotified = "";
  }, []);

  const startExportTracking = useCallback((jobId: string): void => {
    exportTrackRef.current = { jobId, stopped: false, terminalNotified: "" };
  }, []);

  useEffect(() => {
    return () => {
      exportTrackRef.current.stopped = true;
      exportTrackRef.current.jobId = "";
      exportTrackRef.current.terminalNotified = "";
    };
  }, []);

  useEffect(() => {
    if (!isExportRealtimeEventForCase(lastWsEvent, activeCaseId)) {
      return;
    }
    const update = buildExportRealtimeProjection(lastWsEvent);
    if (!update.jobId || update.jobId !== exportState.jobId) {
      return;
    }

    setExportState((prev) => applyExportRealtimeState(prev, update));

    const terminalAction = getExportRealtimeTerminalAction(update);
    if (terminalAction?.type === "notify") {
      notifyExportTerminal(update.jobId, terminalAction.status, terminalAction.outputPath, terminalAction.errorText);
      return;
    }
    if (terminalAction?.type === "sync") {
      void syncExportJobSnapshot(update.jobId, { notifyTerminal: true, silentNetworkError: true });
    }
  }, [activeCaseId, exportState.jobId, lastWsEvent, notifyExportTerminal, setExportState, syncExportJobSnapshot]);

  useEffect(() => {
    const jobId = String(exportState.jobId || "").trim();
    if (!jobId || exportTrackRef.current.stopped) {
      return;
    }
    if (exportState.status === "done" || exportState.status === "failed" || exportState.status === "canceled") {
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
  }, [exportState.jobId, exportState.status, syncExportJobSnapshot, wsStatus]);

  return {
    startExportTracking,
    stopExportTracking
  };
}
