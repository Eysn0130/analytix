export function formatCompactDurationMs(value: number | null | undefined): string {
  const totalSeconds = Math.max(0, Math.round(Number(value || 0) / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return `${hours}h${minutes}m${seconds}s`;
  }
  if (minutes > 0) {
    return `${minutes}m${seconds}s`;
  }
  return `${seconds}s`;
}

export function getDateRangeDurationMs(startedAt: string | null | undefined, endedAt: string | null | undefined): number {
  const started = Date.parse(String(startedAt || ""));
  const ended = Date.parse(String(endedAt || ""));
  if (!Number.isFinite(started) || !Number.isFinite(ended)) {
    return 0;
  }
  return Math.max(0, ended - started);
}
