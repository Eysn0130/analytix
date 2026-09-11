import type { CleaningJobDTO } from "../../../services/cleaning/api";
import {
  asCleaningLogPayload,
  asExportProgressPayload,
  asJobProgressPayload,
  eventBelongsToCase,
  isCleaningEvent,
  isExportEvent,
} from "../../../services/ws/domain-events";
import type { WsEventEnvelope } from "../../../services/ws/types";
import { mapEventToStatusByPayload } from "./status";
import type { CleaningLiveEvent, ExportState, ExportStateStatus } from "./types";
import { canonicalNonnegativeCount, canonicalProgress } from "./known-metrics";

export interface CleaningRealtimeProjection {
  eventId: string;
  eventName: string;
  jobId: string;
  liveEvent: CleaningLiveEvent;
  payload: Record<string, unknown>;
  shouldRefreshJobs: boolean;
  shouldRefreshContext: boolean;
}

export interface ExportRealtimeProjection {
  eventName: string;
  jobId: string;
  payloadMessage: string;
  payloadCode: string;
  payloadOutputPath: string;
  progress: number | null;
}

export type ExportRealtimeTerminalAction =
  | {
      type: "notify";
      status: Extract<ExportStateStatus, "done" | "failed" | "canceled">;
      outputPath: string;
      errorText: string;
    }
  | { type: "sync" };

const CLEANING_EVENT_MESSAGES: Readonly<Record<string, string>> = Object.freeze({
  "cleaning.job.queued": "cleaning job queued",
  "cleaning.job.progress": "cleaning job progress",
  "cleaning.job.log": "cleaning operational log",
  "cleaning.job.completed": "cleaning completed",
  "cleaning.job.failed": "cleaning failed",
});

export function isCleaningRealtimeEventForCase(
  event: WsEventEnvelope | null | undefined,
  activeCaseId: string
): event is WsEventEnvelope {
  return Boolean(event && activeCaseId && isCleaningEvent(event) && eventBelongsToCase(event, activeCaseId));
}

export function isExportRealtimeEventForCase(
  event: WsEventEnvelope | null | undefined,
  activeCaseId: string
): event is WsEventEnvelope {
  return Boolean(event && activeCaseId && isExportEvent(event) && eventBelongsToCase(event, activeCaseId));
}

export function rememberCleaningRealtimeEvent(seenEventIds: Set<string>, eventId: string, maxSize = 1500): boolean {
  if (seenEventIds.has(eventId)) {
    return false;
  }
  seenEventIds.add(eventId);
  if (seenEventIds.size > maxSize) {
    seenEventIds.clear();
    seenEventIds.add(eventId);
  }
  return true;
}

export function buildCleaningRealtimeProjection(event: WsEventEnvelope): CleaningRealtimeProjection {
  const progressPayload = asJobProgressPayload(event.payload);
  const logPayload = asCleaningLogPayload(event.payload);
  const payloadObj: Record<string, unknown> = {
    ...progressPayload,
    ...logPayload,
  };
  const jobId = (event.job_id ?? "").trim();
  const eventId = String(payloadObj.event_id ?? `${event.sequence}:${event.event}:${jobId}`);
  const rawStep = canonicalNonnegativeCount(logPayload.step);
  const step = rawStep !== null && rawStep >= 1 && rawStep <= 10 ? rawStep : null;
  const progress = canonicalProgress(progressPayload.progress);
  const canceled = event.event === "cleaning.job.failed" && progressPayload.code === "JOB_CANCELED";
  return {
    eventId,
    eventName: event.event,
    jobId,
    liveEvent: {
      sequence: event.sequence,
      event: event.event,
      job_id: jobId,
      level: logPayload.level ?? (event.type === "log" ? "info" : event.type),
      message: canceled
        ? "cleaning canceled"
        : CLEANING_EVENT_MESSAGES[event.event] ?? "cleaning status update",
      step,
      progress,
      timestamp: event.timestamp,
    },
    payload: payloadObj,
    shouldRefreshJobs: event.event.endsWith("completed") || event.event.endsWith("failed") || event.event.endsWith("queued"),
    shouldRefreshContext: event.event.endsWith("completed") || event.event.endsWith("failed"),
  };
}

export function applyCleaningRealtimeJobProjection(
  previousJobs: CleaningJobDTO[],
  update: CleaningRealtimeProjection
): CleaningJobDTO[] {
  const index = previousJobs.findIndex((item) => item.job_id === update.jobId);
  if (index < 0) {
    return previousJobs;
  }

  const target = previousJobs[index];
  const next = [...previousJobs];
  next[index] = {
    ...target,
    status: mapEventToStatusByPayload(update.eventName, target.status, update.payload),
    progress: update.liveEvent.progress ?? target.progress,
    cleaned_rows: null,
    updated_at: update.liveEvent.timestamp,
  };
  return next;
}

export function buildExportRealtimeProjection(event: WsEventEnvelope): ExportRealtimeProjection {
  const payload = asExportProgressPayload(event.payload);
  return {
    eventName: event.event,
    jobId: (event.job_id ?? "").trim(),
    payloadMessage: "",
    payloadCode: payload.code === "JOB_CANCELED" ? payload.code : "",
    payloadOutputPath: "",
    progress: canonicalProgress(payload.progress),
  };
}

export function applyExportRealtimeState(previous: ExportState, update: ExportRealtimeProjection): ExportState {
  if (previous.jobId !== update.jobId) {
    return previous;
  }
  if (update.eventName === "export.job.progress") {
    return {
      ...previous,
      status: "running",
      progress: update.progress ?? previous.progress,
      message: update.payloadMessage || previous.message || "导出进行中",
    };
  }
  if (update.eventName === "export.job.completed") {
    return {
      ...previous,
      status: "done",
      progress: 100,
      outputPath: update.payloadOutputPath || previous.outputPath,
      message: update.payloadMessage || "已完成导出",
    };
  }
  if (update.eventName === "export.job.failed") {
    const canceled = update.payloadCode === "JOB_CANCELED";
    return {
      ...previous,
      status: canceled ? "canceled" : "failed",
      message: canceled ? "导出已取消" : update.payloadMessage || "导出失败",
      error: canceled ? previous.error : update.payloadMessage || previous.error,
    };
  }
  return previous;
}

export function getExportRealtimeTerminalAction(update: ExportRealtimeProjection): ExportRealtimeTerminalAction | null {
  if (update.eventName === "export.job.completed") {
    if (update.payloadOutputPath) {
      return {
        type: "notify",
        status: "done",
        outputPath: update.payloadOutputPath,
        errorText: "",
      };
    }
    return { type: "sync" };
  }

  if (update.eventName === "export.job.failed") {
    if (update.payloadCode === "JOB_CANCELED") {
      return {
        type: "notify",
        status: "canceled",
        outputPath: "",
        errorText: "",
      };
    }
    if (update.payloadMessage) {
      return {
        type: "notify",
        status: "failed",
        outputPath: "",
        errorText: update.payloadMessage,
      };
    }
    return { type: "sync" };
  }

  return null;
}
