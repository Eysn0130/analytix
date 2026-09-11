import type { SortDirection } from "./import-page-model";

export function knownNonnegativeInt(value: unknown): number | null {
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    Object.is(value, -0)
  ) {
    return null;
  }
  return value;
}

export function maxKnownMetric(left: number | null, right: number | null): number | null {
  if (left === null) {
    return right;
  }
  if (right === null) {
    return left;
  }
  return Math.max(left, right);
}

export function sumCompleteMetrics(values: Array<number | null>): number | null {
  if (values.length === 0 || values.some((value) => value === null)) {
    return null;
  }
  return values.reduce<number>((sum, value) => sum + (value as number), 0);
}

export type CompleteImpactEstimate =
  | { status: "complete"; value: number; unknownItemCount: 0 }
  | { status: "unknown"; value: null; unknownItemCount: number };

export interface UnverifiedHistoricalCountProjection {
  status: "unverified";
  rowsTotal: null;
  validRows: null;
  columnCount: null;
}

/**
 * The legacy historical-dataset endpoint has no context epoch, dataset
 * snapshot, or host EvidenceReceipt. Even syntactically valid numbers remain
 * unavailable to the ordinary UI until that authority contract exists.
 */
export function projectUnverifiedHistoricalCounts(
  _dataset: { rows: unknown; cols: unknown },
): UnverifiedHistoricalCountProjection {
  return {
    status: "unverified",
    rowsTotal: null,
    validRows: null,
    columnCount: null,
  };
}

/**
 * Produces an all-or-nothing impact estimate for destructive or reversible
 * ledger actions. A partial sum is deliberately not exposed because it can be
 * mistaken for the complete impact of the operation.
 */
export function completeImpactEstimate(values: readonly unknown[]): CompleteImpactEstimate {
  const counts = values.map(knownNonnegativeInt);
  const unknownItemCount = counts.filter((value) => value === null).length;
  if (values.length === 0 || unknownItemCount > 0) {
    return { status: "unknown", value: null, unknownItemCount };
  }
  let total = 0;
  for (const value of counts) {
    const next = total + (value as number);
    if (!Number.isSafeInteger(next)) {
      return { status: "unknown", value: null, unknownItemCount: 0 };
    }
    total = next;
  }
  return {
    status: "complete",
    value: total,
    unknownItemCount: 0,
  };
}

export function compareKnownMetrics(
  left: number | null,
  right: number | null,
  direction: SortDirection,
): number {
  if (left === null && right === null) {
    return 0;
  }
  if (left === null) {
    return 1;
  }
  if (right === null) {
    return -1;
  }
  return direction === "asc" ? left - right : right - left;
}

export function exactDatasetIndex(
  fileId: string,
  datasets: ReadonlyArray<{ dataset_id: string }>,
): number {
  const normalized = String(fileId || "").trim();
  if (!normalized) {
    return -1;
  }
  return datasets.findIndex((dataset) => dataset.dataset_id === normalized);
}

export function importCompletionTitle(importedFiles: unknown, duration: string): string {
  const count = knownNonnegativeInt(importedFiles);
  return count === null
    ? `导入任务已完成，文件数未验证，耗时：${duration}`
    : `已成功导入 ${count.toLocaleString("zh-CN")} 个文件，耗时：${duration}`;
}
