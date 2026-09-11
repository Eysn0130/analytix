import type {
  CleaningStepDetailDTO,
  CleaningStepSummaryDTO,
  JobStatus
} from "../../../services/cleaning/api";
import type { ImportFileLogDTO } from "../../../services/import/api";
import type { CaseDetailDTO } from "../../cases/api";
import type { CleaningAvailability, CleaningFileStage } from "./types";
import {
  canonicalNonnegativeCount,
  reconcileEquivalentCounts,
  sumCompleteCounts
} from "./known-metrics";

export function mapEventToStatus(event: string, previous: JobStatus): JobStatus {
  if (previous === "succeeded" || previous === "failed" || previous === "canceled") {
    return previous;
  }
  if (event.endsWith("queued")) {
    return "queued";
  }
  if (event.endsWith("progress") || event.endsWith("log")) {
    return "running";
  }
  if (event.endsWith("completed")) {
    return "succeeded";
  }
  if (event.endsWith("failed")) {
    return "failed";
  }
  return previous;
}

export function mapEventToStatusByPayload(event: string, previous: JobStatus, payload: Record<string, unknown>): JobStatus {
  if (event.endsWith("failed") && String(payload.code ?? "").trim() === "JOB_CANCELED") {
    if (previous === "succeeded" || previous === "failed") {
      return previous;
    }
    return "canceled";
  }
  if (event.endsWith("failed")) {
    return "failed";
  }
  return mapEventToStatus(event, previous);
}

export function getJobTone(status: string): "good" | "warn" | "danger" | "neutral" {
  if (status === "succeeded") {
    return "good";
  }
  if (status === "failed" || status === "canceled") {
    return "danger";
  }
  if (status === "running" || status === "queued") {
    return "warn";
  }
  return "neutral";
}

export function getStatusLabel(status: string): string {
  if (status === "queued") {
    return "等待中";
  }
  if (status === "running") {
    return "执行中";
  }
  if (status === "succeeded" || status === "done") {
    return "已完成";
  }
  if (status === "failed") {
    return "失败";
  }
  if (status === "canceled") {
    return "已取消";
  }
  if (status === "pending") {
    return "待处理";
  }
  if (status === "idle") {
    return "未启动";
  }
  return status || "--";
}

export function getCleaningFileStage(file: ImportFileLogDTO): CleaningFileStage {
  const status = typeof file.cleaned_status === "string" ? file.cleaned_status.trim() : "";
  if (status === "running") {
    return "running";
  }
  if (status === "failed" || status === "error") {
    return "failed";
  }
  if (status === "done") {
    return "done";
  }
  return "pending";
}

export function parseDecoratedCell(value: string): { text: string; tone: "default" | "good" | "danger" } {
  if (value.startsWith("__GREEN__")) {
    return { text: value.replace("__GREEN__", ""), tone: "good" };
  }
  if (value.startsWith("__RED__")) {
    return { text: value.replace("__RED__", ""), tone: "danger" };
  }
  return { text: value, tone: "default" };
}

export function getKindClass(kind: string): string {
  if (kind === "补全") {
    return "is-fill";
  }
  if (kind === "修正") {
    return "is-fix";
  }
  if (kind === "标记") {
    return "is-mark";
  }
  if (kind === "停用") {
    return "is-paused";
  }
  return "is-process";
}

export function getImportedRowCount(file: ImportFileLogDTO): number | null {
  return canonicalNonnegativeCount(file.rows_imported_norm);
}

export function isCleaningScopeKind(kind: string): boolean {
  return kind === "fc_transaction" || kind === "fc_account";
}

export function buildCleaningAvailability(importFiles: ImportFileLogDTO[], caseDetail: CaseDetailDTO | null): CleaningAvailability {
  const scopeFiles: ImportFileLogDTO[] = [];
  const transactionImportCounts: Array<number | null> = [];
  importFiles.forEach((item) => {
    const kind = typeof item.kind === "string" ? item.kind : "";
    const importedRowCount = getImportedRowCount(item);
    if (kind === "fc_transaction") {
      transactionImportCounts.push(importedRowCount);
    }
    if (isCleaningScopeKind(kind) && importedRowCount !== null && importedRowCount > 0) {
      scopeFiles.push(item);
    }
  });
  const transactionImportRows = sumCompleteCounts(transactionImportCounts);
  const transactionStatsRows = canonicalNonnegativeCount(caseDetail?.stats.tx);
  const scopeRows = reconcileEquivalentCounts(transactionStatsRows, transactionImportRows);
  const hasTransactionData = scopeRows === null ? null : scopeRows > 0;
  return {
    hasTransactionData,
    scopeFiles,
    scopeRows
  };
}

export function normalizeCleaningStepDetailPage(detail: CleaningStepDetailDTO, items: CleaningStepDetailDTO["items"]): CleaningStepDetailDTO {
  void items;
  return {
    ...detail,
    fact_answer_allowed: false,
    raw_details_exposed: false,
    items: [],
    page: null
  };
}
