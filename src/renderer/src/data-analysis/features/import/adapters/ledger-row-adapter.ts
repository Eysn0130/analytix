import type { ImportFileLogDTO, ImportJobDTO } from "../api";
import {
  isTerminalStatus,
  kindLabel,
  normalizeKind,
  type LedgerRow
} from "../model/import-page-model";
import { knownNonnegativeInt } from "../model/public-count-projection";

export function runningPercent(rowsSeen: number | null, rowsTotal: number | null): number {
  if (rowsSeen === null || rowsTotal === null || rowsSeen <= 0 || rowsTotal <= 0) {
    return 0;
  }
  const pct = Math.floor((Math.min(rowsSeen, rowsTotal) * 100) / Math.max(rowsTotal, 1));
  return Math.max(0, Math.min(100, pct));
}

export function resolveStatusText(args: {
  rawStatus: string;
  rowsTotal: number | null;
  rowsSeen: number | null;
  rowsRaw: number | null;
  rowsNorm: number | null;
  rowsDedup: number | null;
  rowsError: number | null;
  errorText: string;
}): string {
  const raw = String(args.rawStatus || "").trim().toLowerCase();
  const rowsTotal = knownNonnegativeInt(args.rowsTotal);
  const rowsSeen = knownNonnegativeInt(args.rowsSeen);
  const rowsRaw = knownNonnegativeInt(args.rowsRaw);
  const rowsNorm = knownNonnegativeInt(args.rowsNorm);
  const rowsDedup = knownNonnegativeInt(args.rowsDedup);
  const rowsError = knownNonnegativeInt(args.rowsError);
  const hasWarn = (rowsError !== null && rowsError > 0) || Boolean(String(args.errorText || "").trim());

  if (raw === "失败" || raw === "failed") {
    return "失败";
  }
  if (raw === "导入中" || raw === "running") {
    const pct = runningPercent(rowsSeen, rowsTotal);
    return pct > 0 ? `导入中 ${pct}%` : "导入中";
  }
  if (raw === "等待中" || raw === "queued") {
    return "等待中";
  }
  if (raw === "已取消" || raw === "canceled" || raw === "cancelled") {
    return "已取消";
  }
  if (raw === "已完成" || raw === "succeeded" || raw === "done") {
    if (
      rowsTotal !== null &&
      rowsTotal > 0 &&
      rowsRaw === 0 &&
      rowsNorm === 0 &&
      rowsDedup === rowsTotal
    ) {
      return "重复数据";
    }
    if ([rowsTotal, rowsSeen, rowsRaw, rowsNorm, rowsDedup, rowsError].some((value) => value === null)) {
      return hasWarn ? "已完成（统计未验证，有提示）" : "已完成（统计未验证）";
    }
    return hasWarn ? "已完成（有提示）" : "已完成";
  }
  return "状态未验证";
}

function normalizeFileStatus(log: ImportFileLogDTO): string {
  const rowsSeen = knownNonnegativeInt(log.rows_imported_raw) ?? knownNonnegativeInt(log.rows_imported);
  const rowsNorm = knownNonnegativeInt(log.rows_imported_norm) ?? knownNonnegativeInt(log.rows_imported);
  const errorText = String(log.error || log.cleaned_error || "");
  return resolveStatusText({
    rawStatus: String(log.status || ""),
    rowsTotal: knownNonnegativeInt(log.rows_total),
    rowsSeen,
    rowsRaw: knownNonnegativeInt(log.rows_imported_raw) ?? knownNonnegativeInt(log.rows_imported),
    rowsNorm,
    rowsDedup: knownNonnegativeInt(log.rows_dedup),
    rowsError: knownNonnegativeInt(log.rows_error),
    errorText
  });
}

export function toLedgerRow(log: ImportFileLogDTO): LedgerRow {
  const rowsTotal = knownNonnegativeInt(log.rows_total);
  const rowsSeen = knownNonnegativeInt(log.rows_imported_raw) ?? knownNonnegativeInt(log.rows_imported);
  const rowsRaw = knownNonnegativeInt(log.rows_imported_raw) ?? knownNonnegativeInt(log.rows_imported);
  const rowsNorm = knownNonnegativeInt(log.rows_imported_norm) ?? knownNonnegativeInt(log.rows_imported);
  const rowsDedup = knownNonnegativeInt(log.rows_dedup);
  const rowsError = knownNonnegativeInt(log.rows_error);
  const rowsSkippedNonData = knownNonnegativeInt(log.rows_skipped_non_data);
  const duplicateRows = rowsDedup;
  const kind = normalizeKind(log.kind, log.filename);

  return {
    key: log.file_id || `${log.filename}:${log.created_at}`,
    file_id: log.file_id || "",
    display_name: log.filename || log.file_id || "未命名文件",
    display_path: log.display_path || "",
    stored_path: log.stored_path || "",
    file_type: log.file_type || "",
    size: knownNonnegativeInt(log.size),
    md5: log.md5 || "",
    sha256: log.sha256 || "",
    kind,
    kind_label: kind ? kindLabel(kind) : "—",
    status_text: normalizeFileStatus(log),
    rows_total: rowsTotal,
    rows_seen: rowsSeen,
    rows_imported_raw: rowsRaw,
    rows_imported_norm: rowsNorm,
    rows_dedup: rowsDedup,
    rows_error: rowsError,
    rows_skipped_non_data: rowsSkippedNonData,
    valid_rows: rowsNorm,
    duplicate_rows: duplicateRows,
    note: "",
    error: String(log.error || log.cleaned_error || ""),
    created_at: String(log.created_at || ""),
    finished_at: String(log.finished_at || ""),
    recycled_at: String(log.recycled_at || ""),
    attempts: null
  };
}

export function toLedgerRowFromJobFile(file: ImportJobDTO["files"][number], job: ImportJobDTO): LedgerRow {
  const rowsTotal = knownNonnegativeInt(file.rows_total);
  const rowsSeen = knownNonnegativeInt(file.rows_seen);
  const rowsRaw = knownNonnegativeInt(file.rows_imported_raw);
  const rowsNorm = knownNonnegativeInt(file.rows_imported_norm);
  const rowsDedup = knownNonnegativeInt(file.rows_dedup);
  const rowsError = knownNonnegativeInt(file.rows_error);
  const rowsSkippedNonData = knownNonnegativeInt(file.rows_skipped_non_data);
  const duplicateRows = rowsDedup;
  const kind = normalizeKind(file.kind, file.display_name);
  const statusText = resolveStatusText({
    rawStatus: String(file.status || ""),
    rowsTotal,
    rowsSeen,
    rowsRaw,
    rowsNorm,
    rowsDedup,
    rowsError,
    errorText: String(file.error || file.note || "")
  });

  return {
    key: file.file_id || `${job.job_id}:${file.display_name}`,
    file_id: file.file_id || "",
    display_name: file.display_name || "未命名文件",
    display_path: file.display_path || "",
    stored_path: "",
    file_type: file.file_type || "",
    size: knownNonnegativeInt(file.size),
    md5: file.md5 || "",
    sha256: file.sha256 || "",
    kind,
    kind_label: kind ? kindLabel(kind) : "—",
    status_text: statusText,
    rows_total: rowsTotal,
    rows_seen: rowsSeen,
    rows_imported_raw: rowsRaw,
    rows_imported_norm: rowsNorm,
    rows_dedup: rowsDedup,
    rows_error: rowsError,
    rows_skipped_non_data: rowsSkippedNonData,
    valid_rows: rowsNorm,
    duplicate_rows: duplicateRows,
    note: String(file.note || ""),
    error: String(file.error || ""),
    created_at: String(job.created_at || ""),
    finished_at: isTerminalStatus(job.status) ? String(job.updated_at || "") : "",
    recycled_at: "",
    attempts: knownNonnegativeInt(file.attempts)
  };
}
