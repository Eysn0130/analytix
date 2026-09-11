import type { ExportKind, ExportState, ExportStateStatus } from "./types";
import { canonicalProgress } from "./known-metrics";

export type ActiveExportKind = Exclude<ExportKind, "">;
export type TerminalExportStateStatus = Extract<ExportStateStatus, "done" | "failed" | "canceled">;

export interface CleaningExportTaskCopy {
  runningTitle: string;
  runningDetail: string;
  successTitle: string;
  successDetail: string;
}

export interface ExportJobSnapshotInput {
  error?: string | null;
  output_path?: string | null;
  progress?: number | null;
  status?: string | null;
}

export interface ExportJobSnapshotProjection {
  errorText: string;
  outputPath: string;
  progress: number | null;
  status: ExportStateStatus;
}

export function getCleaningExportTaskCopy(kind: "cleaned" | "raw"): CleaningExportTaskCopy {
  if (kind === "cleaned") {
    return {
      runningTitle: "正在创建已清洗数据导出任务",
      runningDetail: "系统正在提交已清洗数据导出请求，并锁定界面以避免重复创建。",
      successTitle: "已清洗数据导出任务已创建",
      successDetail: "导出流程已启动，可继续在进度面板中跟踪状态。"
    };
  }
  return {
    runningTitle: "正在创建未清洗数据导出任务",
    runningDetail: "系统正在提交未清洗数据导出请求，并锁定界面以避免重复创建。",
    successTitle: "未清洗数据导出任务已创建",
    successDetail: "导出流程已启动，可继续在进度面板中跟踪状态。"
  };
}

export function buildIdleExportState(): ExportState {
  return {
    jobId: "",
    kind: "",
    status: "idle",
    progress: null,
    outputPath: "",
    error: "",
    message: "尚未发起导出任务"
  };
}

export function getExportRunningStateMessage(kind: ActiveExportKind): string {
  if (kind === "cleaned") {
    return "准备导出已清洗数据";
  }
  return "准备导出未清洗数据";
}

export function buildRunningExportState({
  jobId,
  kind,
  progress
}: {
  jobId: string;
  kind: ActiveExportKind;
  progress: unknown;
}): ExportState {
  return {
    jobId,
    kind,
    status: "running",
    progress: canonicalProgress(progress),
    outputPath: "",
    error: "",
    message: getExportRunningStateMessage(kind)
  };
}

export function buildCancelingExportState(previous: ExportState): ExportState {
  return {
    ...previous,
    status: "canceled",
    message: "正在停止导出"
  };
}

export function mapExportJobStatus(status: string): ExportStateStatus {
  if (status === "succeeded") {
    return "done";
  }
  if (status === "failed") {
    return "failed";
  }
  if (status === "canceled") {
    return "canceled";
  }
  return "running";
}

export function isTerminalExportStateStatus(status: ExportStateStatus): status is TerminalExportStateStatus {
  return status === "done" || status === "failed" || status === "canceled";
}

export function getExportStateMessage(nextStatus: ExportStateStatus, previousMessage: string): string {
  if (nextStatus === "done") {
    return "已完成导出";
  }
  if (nextStatus === "failed") {
    return "导出失败";
  }
  if (nextStatus === "canceled") {
    return "导出已取消";
  }
  return previousMessage || "导出进行中";
}

export function buildExportJobSnapshotProjection(snapshot: ExportJobSnapshotInput): ExportJobSnapshotProjection {
  const status = mapExportJobStatus(String(snapshot.status || ""));
  return {
    errorText: String(snapshot.error || ""),
    outputPath: String(snapshot.output_path || ""),
    progress: canonicalProgress(snapshot.progress),
    status
  };
}

export function applyExportJobSnapshotState(
  previous: ExportState,
  jobId: string,
  projection: ExportJobSnapshotProjection
): ExportState {
  if (previous.jobId !== jobId) {
    return previous;
  }
  return {
    ...previous,
    status: projection.status,
    progress: projection.progress ?? previous.progress,
    outputPath: projection.outputPath,
    error: projection.errorText,
    message: getExportStateMessage(projection.status, previous.message)
  };
}
