import { useEffect, useRef, type Dispatch, type SetStateAction } from "react";
import type { CleaningJobDTO } from "../../../services/cleaning/api";
import type { WsEventEnvelope } from "../../../services/ws/types";
import {
  applyCleaningRealtimeJobProjection,
  buildCleaningRealtimeProjection,
  isCleaningRealtimeEventForCase,
  rememberCleaningRealtimeEvent
} from "../model/realtime-events";

interface UseCleaningRealtimeEventsOptions {
  active: boolean;
  activeCaseId: string;
  dismissCleaningToast: (jobId: string) => void;
  lastWsEvent: WsEventEnvelope | null;
  notifyCleaningCompletionByJobId: (jobId: string, completedAt?: string) => Promise<void>;
  scheduleContextRefresh: () => void;
  scheduleJobsRefresh: () => void;
  setJobs: Dispatch<SetStateAction<CleaningJobDTO[]>>;
}

export function useCleaningRealtimeEvents({
  active,
  activeCaseId,
  dismissCleaningToast,
  lastWsEvent,
  notifyCleaningCompletionByJobId,
  scheduleContextRefresh,
  scheduleJobsRefresh,
  setJobs
}: UseCleaningRealtimeEventsOptions): void {
  const eventDedupRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    eventDedupRef.current.clear();
  }, [activeCaseId]);

  useEffect(() => {
    if (!active) {
      return;
    }
    if (!isCleaningRealtimeEventForCase(lastWsEvent, activeCaseId)) {
      return;
    }

    const update = buildCleaningRealtimeProjection(lastWsEvent);
    if (!rememberCleaningRealtimeEvent(eventDedupRef.current, update.eventId)) {
      return;
    }

    if (!update.jobId) {
      scheduleJobsRefresh();
      return;
    }

    setJobs((prev) => {
      const next = applyCleaningRealtimeJobProjection(prev, update);
      if (next === prev) {
        scheduleJobsRefresh();
      }
      return next;
    });

    if (update.shouldRefreshJobs) {
      scheduleJobsRefresh();
    }

    if (update.shouldRefreshContext) {
      scheduleContextRefresh();
    }
    if (update.eventName === "cleaning.job.completed") {
      void notifyCleaningCompletionByJobId(update.jobId, update.liveEvent.timestamp);
      return;
    }
    if (update.eventName === "cleaning.job.failed") {
      dismissCleaningToast(update.jobId);
    }
  }, [
    active,
    activeCaseId,
    dismissCleaningToast,
    lastWsEvent,
    notifyCleaningCompletionByJobId,
    scheduleContextRefresh,
    scheduleJobsRefresh,
    setJobs
  ]);
}
