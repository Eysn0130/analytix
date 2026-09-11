export type TxnDetailLoadPageKind = "first-page" | "next-page" | "aggregate";

export interface TxnDetailLoadMetricOptions {
  cursorAfter?: unknown;
  cursorBefore?: unknown;
  done?: boolean;
  durationMs?: number;
  operation: string;
  pageKind?: TxnDetailLoadPageKind;
  pageLimit?: number;
  requestBuildMs?: number;
  requestCount?: number;
  reset?: boolean;
  rowsAfter?: number;
  rowsBefore?: number;
  rowsFetched?: number;
  surface: string;
}

export interface TxnDetailLoadMetric {
  txnDetailCursorIn: number;
  txnDetailCursorOut: number;
  txnDetailDone: number;
  txnDetailDurationMs?: number;
  txnDetailOperation: string;
  txnDetailPageKind: TxnDetailLoadPageKind;
  txnDetailPageLimit: number;
  txnDetailRequestBuildMs?: number;
  txnDetailRequestCount: number;
  txnDetailReset: number;
  txnDetailRowsAfter: number;
  txnDetailRowsBefore: number;
  txnDetailRowsFetched: number;
  txnDetailSurface: string;
}

export function readTxnDetailLoadNow(): number {
  const perf = globalThis.performance;
  if (perf && typeof perf.now === "function") {
    return perf.now();
  }
  return Date.now();
}

export function buildTxnDetailLoadMetric({
  cursorAfter,
  cursorBefore,
  done = false,
  durationMs,
  operation,
  pageKind,
  pageLimit = 0,
  requestBuildMs,
  requestCount = 1,
  reset = false,
  rowsAfter,
  rowsBefore = 0,
  rowsFetched = 0,
  surface
}: TxnDetailLoadMetricOptions): TxnDetailLoadMetric {
  const safeRowsBefore = toNonNegativeInteger(rowsBefore);
  const safeRowsFetched = toNonNegativeInteger(rowsFetched);
  const metric: TxnDetailLoadMetric = {
    txnDetailCursorIn: hasCursor(cursorBefore) ? 1 : 0,
    txnDetailCursorOut: hasCursor(cursorAfter) ? 1 : 0,
    txnDetailDone: done ? 1 : 0,
    txnDetailOperation: text(operation) || "detail",
    txnDetailPageKind: pageKind || (reset ? "first-page" : "next-page"),
    txnDetailPageLimit: toNonNegativeInteger(pageLimit),
    txnDetailRequestCount: toNonNegativeInteger(requestCount),
    txnDetailReset: reset ? 1 : 0,
    txnDetailRowsAfter: toNonNegativeInteger(rowsAfter, safeRowsBefore + safeRowsFetched),
    txnDetailRowsBefore: safeRowsBefore,
    txnDetailRowsFetched: safeRowsFetched,
    txnDetailSurface: text(surface) || "unknown",
  };
  if (Number.isFinite(durationMs)) {
    metric.txnDetailDurationMs = roundMetricMs(Number(durationMs));
  }
  if (Number.isFinite(requestBuildMs)) {
    metric.txnDetailRequestBuildMs = roundMetricMs(Number(requestBuildMs));
  }
  return metric;
}

function hasCursor(value: unknown): boolean {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

function toNonNegativeInteger(value: number | undefined, fallback = 0): number {
  const numberValue = Math.floor(Number(value ?? fallback));
  if (Number.isFinite(numberValue) && numberValue >= 0) {
    return numberValue;
  }
  return Math.max(0, Math.floor(Number(fallback) || 0));
}

function roundMetricMs(value: number): number {
  return Math.round(Math.max(0, value) * 1000) / 1000;
}
