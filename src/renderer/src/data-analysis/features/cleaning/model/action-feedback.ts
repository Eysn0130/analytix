import type { TerminalExportStateStatus } from "./export";

export type CleaningActionFeedbackTone = "info" | "success" | "error";
export type CleaningActionFeedbackTarget = "hint" | "error";

export interface CleaningActionFeedbackViewModel {
  detail: string;
  hint?: string;
  message?: string;
  title: string;
  tone: CleaningActionFeedbackTone;
}

export interface CleaningActionFeedbackEffectViewModel {
  feedback: CleaningActionFeedbackViewModel;
  target: CleaningActionFeedbackTarget;
}

export interface ExportTerminalActionFeedbackInput {
  compactPath: string;
  errorDetail: string;
  errorText: string;
  outputPath: string;
  status: TerminalExportStateStatus;
}

export const CLEANING_RUNTIME_HEALTH_UNKNOWN_MESSAGE = "清洗运行时状态暂时无法确认，请稍后重试。";
export const EXPORT_TERMINAL_RETRY_MESSAGE = "请稍后重试。";

export function buildExportDirectoryCanceledFeedback(): CleaningActionFeedbackViewModel {
  return {
    detail: "系统未创建新的导出任务。",
    hint: "已取消选择导出目录。",
    title: "已取消目录选择",
    tone: "info"
  };
}

export function buildExportDirectorySelectedHint(label: string, selectedPath: string): string {
  return `已选择${label}目录：${selectedPath}`;
}

export function buildExportJobCreatedHint(jobId: string): string {
  return `已创建导出任务：${jobId}`;
}

export function buildExportCancelRequestedFeedback(jobId: string): CleaningActionFeedbackViewModel {
  return {
    detail: `导出任务 ${jobId} 正在停止，状态更新后会自动同步。`,
    hint: `已请求取消导出任务：${jobId}`,
    title: "已接收取消指令",
    tone: "info"
  };
}

export function buildExportOutputMissingFeedback(): CleaningActionFeedbackViewModel {
  return {
    detail: "请等待导出完成后，再打开结果目录。",
    hint: "当前还没有可打开的导出目录。",
    title: "导出目录尚未就绪",
    tone: "info"
  };
}

export function buildExportOutputOpenFailedFeedback(
  targetPath: string,
  compactPath: string
): CleaningActionFeedbackViewModel {
  return {
    detail: compactPath,
    message: `打开导出目录失败：${targetPath}`,
    title: "导出目录打开失败",
    tone: "error"
  };
}

export function buildExportOutputOpenedFeedback(targetPath: string, compactPath: string): CleaningActionFeedbackViewModel {
  return {
    detail: compactPath,
    hint: `已打开导出目录：${targetPath}`,
    title: "导出目录已打开",
    tone: "success"
  };
}

export function buildExportTerminalDoneFeedback(
  outputPath: string,
  compactPath: string
): CleaningActionFeedbackViewModel {
  return {
    detail: outputPath ? compactPath : "结果文件已写入导出目录。",
    hint: outputPath ? `导出完成：${outputPath}` : "导出完成",
    title: "导出已完成",
    tone: "success"
  };
}

export function buildExportTerminalFailedFeedback(
  errorText: string,
  detail: string
): CleaningActionFeedbackViewModel {
  return {
    detail,
    message: errorText || "导出失败",
    title: "导出执行失败",
    tone: "error"
  };
}

export function buildExportTerminalCanceledFeedback(): CleaningActionFeedbackViewModel {
  return {
    detail: "本次导出流程已终止，未继续写入新的结果文件。",
    hint: "导出已取消",
    title: "导出已停止",
    tone: "info"
  };
}

export function buildExportTerminalActionFeedbackViewModel({
  compactPath,
  errorDetail,
  errorText,
  outputPath,
  status
}: ExportTerminalActionFeedbackInput): CleaningActionFeedbackEffectViewModel {
  if (status === "failed") {
    return {
      feedback: buildExportTerminalFailedFeedback(errorText, errorDetail),
      target: "error"
    };
  }
  if (status === "done") {
    return {
      feedback: buildExportTerminalDoneFeedback(outputPath, compactPath),
      target: "hint"
    };
  }
  return {
    feedback: buildExportTerminalCanceledFeedback(),
    target: "hint"
  };
}

export function buildCleaningRuntimeHealthUnknownFeedback(detail: string): CleaningActionFeedbackViewModel {
  return {
    detail,
    message: CLEANING_RUNTIME_HEALTH_UNKNOWN_MESSAGE,
    title: "清洗运行时未就绪",
    tone: "error"
  };
}

export function buildCleaningRuntimeUnavailableFeedback(
  message: string,
  detail: string
): CleaningActionFeedbackViewModel {
  return {
    detail,
    message,
    title: "清洗运行时不可用",
    tone: "error"
  };
}

export function buildCleaningJobCreateFailedFeedback(message: string, detail: string): CleaningActionFeedbackViewModel {
  return {
    detail,
    message,
    title: "清洗任务创建失败",
    tone: "error"
  };
}

export function buildExportJobCreateFailedFeedback(message: string, detail: string): CleaningActionFeedbackViewModel {
  return {
    detail,
    message,
    title: "导出任务创建失败",
    tone: "error"
  };
}

export function buildExportCancelFailedFeedback(message: string, detail: string): CleaningActionFeedbackViewModel {
  return {
    detail,
    message,
    title: "导出任务取消失败",
    tone: "error"
  };
}

export function buildExportTrackingSyncFailedFeedback(
  message: string,
  detail: string
): CleaningActionFeedbackViewModel {
  return {
    detail,
    message,
    title: "导出状态同步失败",
    tone: "error"
  };
}
