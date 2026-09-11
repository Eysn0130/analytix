import { dismissTask, emitTask, emitTaskSuccess, type TaskEventDetail } from "./TaskCenter";
import { resolveTaskTitle } from "./task-copy";

export interface TaskStartOptions {
  title: string;
  detail?: string;
  blocking?: boolean;
  progressValue?: number | null;
  progressLabel?: string;
  minVisibleMs?: number;
  successTitle?: string;
  successDetail?: string;
  successDurationMs?: number;
}

export type TaskUpdateOptions = Pick<TaskStartOptions, "title" | "detail" | "blocking" | "progressValue" | "progressLabel">;

export interface TaskSuccessOptions {
  title?: string;
  detail?: string;
  durationMs?: number;
  progressValue?: number | null;
  progressLabel?: string;
}

export interface TaskController {
  id: string;
  update: (options: TaskUpdateOptions) => void;
  success: (options?: TaskSuccessOptions) => Promise<void>;
  dismiss: () => void;
  fail: () => void;
}

type StandaloneSuccessOptions = Pick<TaskStartOptions, "title" | "detail" | "blocking" | "progressValue" | "progressLabel" | "successDurationMs">;

const DEFAULT_MIN_VISIBLE_MS = 420;
const DEFAULT_SUCCESS_DURATION_MS = 1320;

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, Math.max(0, Math.round(ms)));
  });
}

function buildTaskDetail(options: TaskUpdateOptions & Pick<TaskEventDetail, "id" | "action">): TaskEventDetail {
  return {
    id: options.id,
    action: options.action,
    title: options.title,
    detail: options.detail,
    blocking: options.blocking,
    progressValue: options.progressValue,
    progressLabel: options.progressLabel
  };
}

function createTaskController(options: TaskStartOptions): TaskController {
  const id = emitTask(
    buildTaskDetail({
      id: undefined,
      action: "show",
      title: options.title,
      detail: options.detail,
      blocking: options.blocking ?? true,
      progressValue: options.progressValue,
      progressLabel: options.progressLabel
    })
  );
  const startedAt = Date.now();
  const minVisibleMs =
    typeof options.minVisibleMs === "number" && Number.isFinite(options.minVisibleMs)
      ? Math.max(0, Math.round(options.minVisibleMs))
      : DEFAULT_MIN_VISIBLE_MS;
  const successDurationMs =
    typeof options.successDurationMs === "number" && Number.isFinite(options.successDurationMs)
      ? options.successDurationMs
      : DEFAULT_SUCCESS_DURATION_MS;

  return {
    id,
    update(nextOptions: TaskUpdateOptions): void {
      emitTask(
        buildTaskDetail({
          id,
          action: "update",
          ...nextOptions
        })
      );
    },
    async success(nextOptions?: TaskSuccessOptions): Promise<void> {
      const elapsed = Date.now() - startedAt;
      if (elapsed < minVisibleMs) {
        await wait(minVisibleMs - elapsed);
      }
      await emitTaskSuccess({
        id,
        title: resolveTaskTitle(nextOptions?.title, options.successTitle, "success"),
        detail: nextOptions?.detail ?? options.successDetail,
        blocking: options.blocking ?? true,
        progressValue: nextOptions?.progressValue ?? 100,
        progressLabel: nextOptions?.progressLabel ?? options.progressLabel,
        durationMs: nextOptions?.durationMs ?? successDurationMs
      });
    },
    dismiss(): void {
      dismissTask(id);
    },
    fail(): void {
      dismissTask(id);
    }
  };
}

async function runTask<ResultT>(
  options: TaskStartOptions,
  executor: (controller: TaskController) => Promise<ResultT>
): Promise<ResultT> {
  const controller = createTaskController(options);
  try {
    const result = await executor(controller);
    await controller.success();
    return result;
  } catch (error) {
    controller.dismiss();
    throw error;
  }
}

async function showSuccess(options: StandaloneSuccessOptions): Promise<void> {
  await emitTaskSuccess({
    title: resolveTaskTitle(options.title, undefined, "success"),
    detail: options.detail,
    blocking: options.blocking ?? true,
    progressValue: options.progressValue ?? 100,
    progressLabel: options.progressLabel,
    durationMs: options.successDurationMs ?? DEFAULT_SUCCESS_DURATION_MS
  });
}

export const task = Object.freeze({
  start: createTaskController,
  run: runTask,
  success: showSuccess
});

export type Task = typeof task;
