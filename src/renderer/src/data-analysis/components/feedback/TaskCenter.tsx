import { useCallback, useEffect, useRef, type ReactNode } from "react";
import { dismissToast, emitToast, type ToastTone } from "./ToastCenter";
import { defaultTaskTitle, resolveTaskTitle } from "./task-copy";

export type TaskPhase = "running" | "success";
export type TaskEventAction = "show" | "update" | "success" | "dismiss";

export interface TaskEventDetail {
  id?: string;
  action?: TaskEventAction;
  title?: string;
  detail?: string;
  blocking?: boolean;
  progressValue?: number | null;
  progressLabel?: string;
  durationMs?: number;
  resolve?: () => void;
}

interface TaskMessage {
  id: string;
  phase: TaskPhase;
  title: string;
  detail?: string;
  blocking: boolean;
  progressValue: number | null;
  progressLabel?: string;
  durationMs: number;
  resolve?: () => void;
}

declare global {
  interface WindowEventMap {
    "analytix:task": CustomEvent<TaskEventDetail>;
  }
}

const TASK_EVENT_NAME = "analytix:task";
const TASK_DEFAULT_DURATION_MS = 1320;
const TASK_SUCCESS_TOAST_MIN_DURATION_MS = 3200;

function createTaskId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function normalizeText(value: unknown, fallback = ""): string {
  const text = String(value || "").trim();
  return text || fallback;
}

function normalizeProgressValue(value: number | null | undefined, phase: TaskPhase): number | null {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return phase === "success" ? 100 : null;
  }
  return Math.max(0, Math.min(100, Math.round(value)));
}

function normalizeDuration(value: number | undefined): number {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return TASK_DEFAULT_DURATION_MS;
  }
  return Math.max(240, Math.min(2600, Math.round(value)));
}

function normalizeTaskMessage(detail: TaskEventDetail, fallback?: TaskMessage | null): TaskMessage {
  const phase = detail.action === "success" ? "success" : fallback?.phase || "running";
  const defaultTitle = defaultTaskTitle(phase);
  const title = resolveTaskTitle(detail.title, fallback?.title, phase) || defaultTitle;
  const detailText = normalizeText(detail.detail, fallback?.detail || "");
  const progressLabel = normalizeText(detail.progressLabel, fallback?.progressLabel || "");

  return {
    id: normalizeText(detail.id, fallback?.id || createTaskId()),
    phase,
    title,
    detail: detailText || undefined,
    blocking: detail.blocking ?? fallback?.blocking ?? true,
    progressValue: normalizeProgressValue(detail.progressValue, phase),
    progressLabel: progressLabel || fallback?.progressLabel,
    durationMs: normalizeDuration(detail.durationMs ?? fallback?.durationMs),
    resolve: detail.resolve ?? fallback?.resolve
  };
}

function taskToastId(id: string): string {
  const taskId = normalizeText(id);
  return taskId.startsWith("task:") ? taskId : `task:${taskId}`;
}

function taskToastTone(phase: TaskPhase): ToastTone {
  return phase === "success" ? "success" : "running";
}

function taskToastDetail(task: TaskMessage): string | undefined {
  const parts: string[] = [];
  if (task.detail) {
    parts.push(task.detail);
  }

  const progressText =
    task.phase === "running" && typeof task.progressValue === "number" && Number.isFinite(task.progressValue)
      ? `${task.progressValue}%`
      : "";
  const meta = task.phase === "running" ? task.progressLabel || progressText : "";
  if (meta) {
    parts.push(meta);
  }

  return parts.join(" ").trim() || undefined;
}

function emitTaskToast(task: TaskMessage): string {
  const toastId = taskToastId(task.id);
  const toastTone = taskToastTone(task.phase);
  const toastDurationMs =
    task.phase === "success"
      ? Math.max(task.durationMs, TASK_SUCCESS_TOAST_MIN_DURATION_MS)
      : 0;

  const emittedId = emitToast({
    id: toastId,
    tone: toastTone,
    title: task.title,
    detail: taskToastDetail(task),
    durationMs: toastDurationMs,
    dismissible: true,
    dedupeKey: toastId
  });
  task.resolve?.();
  return emittedId;
}

export function emitTask(detail: TaskEventDetail): string {
  const normalized = normalizeTaskMessage(
    {
      ...detail,
      action: detail.action === "update" ? "update" : "show"
    },
    null
  );
  return emitTaskToast(normalized);
}

export function emitTaskSuccess(detail: TaskEventDetail): Promise<void> {
  const normalized = normalizeTaskMessage(
    {
      ...detail,
      action: "success"
    },
    null
  );
  emitTaskToast(normalized);
  return Promise.resolve();
}

export function dismissTask(id: string): void {
  const taskId = normalizeText(id);
  if (!taskId) {
    return;
  }
  dismissToast(taskToastId(taskId));
}

export function TaskProvider({ children }: { children: ReactNode }): JSX.Element {
  const taskMessagesRef = useRef<Map<string, TaskMessage>>(new Map());

  const routeTaskEvent = useCallback((detail: TaskEventDetail): void => {
    const action = detail.action || "show";
    const taskId = normalizeText(detail.id);

    if (action === "dismiss") {
      if (taskId) {
        dismissTask(taskId);
        taskMessagesRef.current.delete(taskId);
      }
      return;
    }

    const fallback = taskId ? taskMessagesRef.current.get(taskId) || null : null;
    const normalized = normalizeTaskMessage(detail, fallback);
    taskMessagesRef.current.set(normalized.id, normalized);
    emitTaskToast(normalized);

    if (normalized.phase === "success") {
      taskMessagesRef.current.delete(normalized.id);
    }
  }, []);

  useEffect(() => {
    const taskMessages = taskMessagesRef.current;
    const onTask = (event: WindowEventMap[typeof TASK_EVENT_NAME]): void => {
      event.preventDefault();
      routeTaskEvent(event.detail || {});
    };

    window.addEventListener(TASK_EVENT_NAME, onTask);
    return () => {
      window.removeEventListener(TASK_EVENT_NAME, onTask);
      taskMessages.clear();
    };
  }, [routeTaskEvent]);

  return <>{children}</>;
}
