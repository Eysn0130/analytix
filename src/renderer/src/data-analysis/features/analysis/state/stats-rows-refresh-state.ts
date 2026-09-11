export const STATS_ROWS_REFRESH_DELAY_MS = 80;
export const STATS_ROWS_SELECTION_SETTLE_MS = 220;

export function getStatsRowsRefreshDelayMs(nowMs: number, settledAtMs: number): number {
  const settleDelayMs = settledAtMs > nowMs ? Math.ceil(settledAtMs - nowMs) : 0;
  return Math.max(STATS_ROWS_REFRESH_DELAY_MS, settleDelayMs);
}
