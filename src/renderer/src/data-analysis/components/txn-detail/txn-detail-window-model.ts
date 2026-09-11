export interface TxnDetailViewportRowsOptions {
  clientHeight: number;
  rowHeight: number;
  headerHeight?: number;
  fallbackRows?: number;
}

export interface TxnDetailRowWindowOptions {
  rowCount: number;
  scrollTop: number;
  viewportRows: number;
  rowHeight: number;
  overscanRows: number;
}

export interface TxnDetailRowWindow {
  startIndex: number;
  endIndex: number;
  topSpacerHeight: number;
  bottomSpacerHeight: number;
}

export interface TxnDetailScrollLoadMetricsOptions {
  scrollHeight: number;
  scrollTop: number;
  clientHeight: number;
  rowHeight: number;
  thresholdRows?: number;
}

export interface TxnDetailScrollLoadMetrics {
  remainingPx: number;
  thresholdPx: number;
}

export interface TxnDetailLoadMoreOptions {
  open: boolean;
  rowCount: number;
  loading: boolean;
  done: boolean;
  remainingPx: number;
  thresholdPx: number;
}

function toSafeNumber(value: number, fallback = 0): number {
  return Number.isFinite(value) ? value : fallback;
}

export function estimateTxnDetailViewportRows({
  clientHeight,
  rowHeight,
  headerHeight = 0,
  fallbackRows = 1
}: TxnDetailViewportRowsOptions): number {
  const safeRowHeight = Math.max(1, Math.floor(toSafeNumber(rowHeight, 1)));
  const safeFallbackRows = Math.max(1, Math.floor(toSafeNumber(fallbackRows, 1)));
  const safeClientHeight = toSafeNumber(clientHeight, -1);
  if (safeClientHeight <= 0) {
    return safeFallbackRows;
  }
  const bodyHeight = Math.max(safeRowHeight, safeClientHeight - toSafeNumber(headerHeight));
  return Math.max(1, Math.ceil(bodyHeight / safeRowHeight));
}

export function buildTxnDetailRowWindow({
  rowCount,
  scrollTop,
  viewportRows,
  rowHeight,
  overscanRows
}: TxnDetailRowWindowOptions): TxnDetailRowWindow {
  const safeRowCount = Math.max(0, Math.floor(toSafeNumber(rowCount)));
  const safeRowHeight = Math.max(1, Math.floor(toSafeNumber(rowHeight, 1)));
  const safeViewportRows = Math.max(1, Math.floor(toSafeNumber(viewportRows, 1)));
  const safeOverscanRows = Math.max(0, Math.floor(toSafeNumber(overscanRows)));
  const safeScrollTop = Math.max(0, toSafeNumber(scrollTop));
  const windowSize = safeViewportRows + safeOverscanRows * 2;
  const maxStartIndex = Math.max(0, safeRowCount - windowSize);
  const rawStartIndex = Math.max(0, Math.floor(safeScrollTop / safeRowHeight) - safeOverscanRows);
  const startIndex = Math.min(rawStartIndex, maxStartIndex);
  const endIndex = Math.min(safeRowCount, startIndex + windowSize);

  return {
    startIndex,
    endIndex,
    topSpacerHeight: Math.max(0, startIndex * safeRowHeight),
    bottomSpacerHeight: Math.max(0, (safeRowCount - endIndex) * safeRowHeight)
  };
}

export function buildTxnDetailScrollLoadMetrics({
  scrollHeight,
  scrollTop,
  clientHeight,
  rowHeight,
  thresholdRows = 4
}: TxnDetailScrollLoadMetricsOptions): TxnDetailScrollLoadMetrics {
  const safeScrollHeight = Math.max(0, toSafeNumber(scrollHeight));
  const safeScrollTop = Math.max(0, toSafeNumber(scrollTop));
  const safeClientHeight = Math.max(0, toSafeNumber(clientHeight));
  const safeRowHeight = Math.max(1, Math.floor(toSafeNumber(rowHeight, 1)));
  const safeThresholdRows = Math.max(0, Math.floor(toSafeNumber(thresholdRows)));
  return {
    remainingPx: Math.max(0, safeScrollHeight - safeScrollTop - safeClientHeight),
    thresholdPx: safeRowHeight * safeThresholdRows
  };
}

export function shouldLoadMoreTxnDetailRows({
  open,
  rowCount,
  loading,
  done,
  remainingPx,
  thresholdPx
}: TxnDetailLoadMoreOptions): boolean {
  if (!open || rowCount <= 0 || loading || done) {
    return false;
  }
  return Number(remainingPx) <= Math.max(0, Number(thresholdPx) || 0);
}
