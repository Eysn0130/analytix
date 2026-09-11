import { useCallback, useEffect, useRef } from "react";
import { dismissToast, emitToast } from "../../../components/feedback/ToastCenter";
import { formatCompactDurationMs, getDateRangeDurationMs } from "../../../components/feedback/task-duration";
import {
  type CleaningJobDTO,
  getCleaningJob
} from "../../../services/cleaning/api";

interface UseCleaningJobNotificationsOptions {
  activeCaseId: string;
  jobs: CleaningJobDTO[];
}

interface CleaningJobNotifications {
  dismissCleaningToast: (jobId: string) => void;
  notifyCleaningCompletionByJobId: (jobId: string, completedAt?: string) => Promise<void>;
  showCleaningRunningToast: (jobId: string) => void;
}

function getCleaningToastId(jobId: string): string {
  return `cleaning:${String(jobId || "").trim()}`;
}

export function useCleaningJobNotifications({
  activeCaseId,
  jobs
}: UseCleaningJobNotificationsOptions): CleaningJobNotifications {
  const completionNotifiedRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    completionNotifiedRef.current.clear();
  }, [activeCaseId]);

  const showCleaningRunningToast = useCallback((jobId: string): void => {
    if (!String(jobId || "").trim()) {
      return;
    }
    const toastId = getCleaningToastId(jobId);
    emitToast({
      id: toastId,
      tone: "running",
      title: "正在清洗中",
      durationMs: 0,
      dismissible: true,
      dedupeKey: toastId
    });
  }, []);

  const dismissCleaningToast = useCallback((jobId: string): void => {
    dismissToast(getCleaningToastId(jobId));
  }, []);

  const notifyCleaningCompletion = useCallback(
    (job: CleaningJobDTO, completedAt?: string): void => {
      const jobId = String(job.job_id || "").trim();
      if (!jobId || completionNotifiedRef.current.has(jobId)) {
        return;
      }
      completionNotifiedRef.current.add(jobId);

      const duration = formatCompactDurationMs(getDateRangeDurationMs(job.created_at, completedAt || job.updated_at));
      const toastId = getCleaningToastId(jobId);
      emitToast({
        id: toastId,
        tone: "success",
        title: `清洗任务已完成，结果行数待证据验证，耗时：${duration}`,
        durationMs: 4200,
        dismissible: true,
        dedupeKey: toastId
      });
    },
    []
  );

  const notifyCleaningCompletionByJobId = useCallback(
    async (jobId: string, completedAt?: string): Promise<void> => {
      const normalizedJobId = String(jobId || "").trim();
      if (!normalizedJobId || completionNotifiedRef.current.has(normalizedJobId)) {
        return;
      }
      const existingJob = jobs.find((item) => item.job_id === normalizedJobId);
      if (existingJob) {
        notifyCleaningCompletion(existingJob, completedAt);
        return;
      }
      try {
        const current = await getCleaningJob(normalizedJobId, activeCaseId);
        if (current.case_id !== activeCaseId) {
          return;
        }
        notifyCleaningCompletion(current, completedAt);
      } catch {
        // Completion toasts are best-effort; the persisted job state remains the durable source.
      }
    },
    [activeCaseId, jobs, notifyCleaningCompletion]
  );

  return {
    dismissCleaningToast,
    notifyCleaningCompletionByJobId,
    showCleaningRunningToast
  };
}
