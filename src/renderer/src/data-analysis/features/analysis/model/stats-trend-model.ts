import type { ChartGranularity, ChartMetricMode } from "../api/stats-api";

export const HOUR_MS = 60 * 60 * 1000;
export const DAY_MS = 24 * HOUR_MS;
export const WEEK_MS = 7 * DAY_MS;

export type TrendDisplayGranularity = "year" | "month" | "week" | "day" | "hour";

export interface TrendViewport {
  startTs: number;
  endTs: number;
}

export interface TrendResolvedPoint {
  bucket: string;
  label: string;
  startTs: number;
  endTs: number;
  inAmount: number;
  outAmount: number;
  netAmount: number;
  totalAmount: number;
  inCount: number;
  outCount: number;
  netCount: number;
  totalCount: number;
}

export interface TrendDisplayPoint extends TrendResolvedPoint {
  key: string;
  axisLabel: string;
  rangeLabel: string;
  sourceBucketCount: number;
}

export interface TrendAxisUnit {
  unit: string;
  divisor: number;
  precision: number;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function toNum(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

export function parseDateLike(value: string): number | null {
  const text = String(value || "").trim();
  if (!text) {
    return null;
  }
  const normalized = text.includes("T") ? text : text.includes(" ") ? text.replace(" ", "T") : `${text}T00:00:00`;
  const timestamp = Date.parse(normalized);
  return Number.isFinite(timestamp) ? timestamp : null;
}

function pad2(value: number): string {
  return String(value).padStart(2, "0");
}

export function formatChartDateTime(timestamp: number, withSeconds = true): string {
  const date = new Date(timestamp);
  const base = `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
  return withSeconds ? `${base}:${pad2(date.getSeconds())}` : base;
}

export function formatDateOnly(timestamp: number): string {
  const date = new Date(timestamp);
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`;
}

function formatDateTimeMinute(timestamp: number): string {
  const date = new Date(timestamp);
  return `${formatDateOnly(timestamp)} ${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
}

function formatMonthOnly(timestamp: number): string {
  const date = new Date(timestamp);
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}`;
}

function isSameDay(leftTs: number, rightTs: number): boolean {
  return formatDateOnly(leftTs) === formatDateOnly(rightTs);
}

function isSameMonth(leftTs: number, rightTs: number): boolean {
  return formatMonthOnly(leftTs) === formatMonthOnly(rightTs);
}

function isSameYear(leftTs: number, rightTs: number): boolean {
  return new Date(leftTs).getFullYear() === new Date(rightTs).getFullYear();
}

function getWeekStartTs(timestamp: number): number {
  const date = new Date(timestamp);
  const weekday = date.getDay();
  const delta = weekday === 0 ? -6 : 1 - weekday;
  date.setDate(date.getDate() + delta);
  date.setHours(0, 0, 0, 0);
  return date.getTime();
}

function formatTrendBucketTitle(timestamp: number, granularity: TrendDisplayGranularity): string {
  const date = new Date(timestamp);
  if (granularity === "year") {
    return String(date.getFullYear());
  }
  if (granularity === "month") {
    return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}`;
  }
  if (granularity === "week") {
    return `${formatDateOnly(getWeekStartTs(timestamp))} 周`;
  }
  if (granularity === "hour") {
    return `${formatDateOnly(timestamp)} ${pad2(date.getHours())}:00`;
  }
  return formatDateOnly(timestamp);
}

function formatTrendRangeLabel(startTs: number, endTs: number, granularity: TrendDisplayGranularity): string {
  if (granularity === "hour") {
    return `${formatDateTimeMinute(startTs)} - ${formatDateTimeMinute(endTs)}`;
  }
  if (granularity === "day") {
    return isSameDay(startTs, endTs) ? formatDateOnly(startTs) : `${formatDateOnly(startTs)} - ${formatDateOnly(endTs)}`;
  }
  if (granularity === "month") {
    return isSameMonth(startTs, endTs) ? formatMonthOnly(startTs) : `${formatMonthOnly(startTs)} - ${formatMonthOnly(endTs)}`;
  }
  if (granularity === "week") {
    const start = getWeekStartTs(startTs);
    const end = start + WEEK_MS - 1;
    return `${formatDateOnly(start)} - ${formatDateOnly(Math.min(end, endTs))}`;
  }
  return isSameYear(startTs, endTs) ? String(new Date(startTs).getFullYear()) : `${new Date(startTs).getFullYear()}-${new Date(endTs).getFullYear()}`;
}

function formatTrendAxisLabel(timestamp: number, granularity: TrendDisplayGranularity, viewportStartTs: number, viewportEndTs: number): string {
  const date = new Date(timestamp);
  if (granularity === "year") {
    return String(date.getFullYear());
  }
  if (granularity === "month") {
    return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}`;
  }
  if (granularity === "week") {
    return formatDateOnly(getWeekStartTs(timestamp));
  }
  if (granularity === "day") {
    return formatDateOnly(timestamp);
  }
  if (isSameDay(viewportStartTs, viewportEndTs)) {
    return `${formatDateOnly(timestamp)} ${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
  }
  return `${formatDateOnly(timestamp)} ${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
}

export function getTrendAxisLabelMinSpacing(granularity: TrendDisplayGranularity): number {
  if (granularity === "year") {
    return 72;
  }
  if (granularity === "month") {
    return 94;
  }
  if (granularity === "week") {
    return 112;
  }
  if (granularity === "day") {
    return 118;
  }
  return 148;
}

export function getTrendBucketRange(bucket: string, granularity: ChartGranularity): TrendViewport | null {
  const value = String(bucket || "").trim();
  if (!value) {
    return null;
  }
  if (granularity === "month") {
    const match = value.match(/^(\d{4})-(\d{2})$/);
    if (!match) {
      return null;
    }
    const year = Number(match[1]);
    const month = Number(match[2]) - 1;
    const startTs = new Date(year, month, 1, 0, 0, 0, 0).getTime();
    const endTs = new Date(year, month + 1, 1, 0, 0, 0, 0).getTime() - 1;
    return { startTs, endTs };
  }
  if (granularity === "week") {
    const startTs = parseDateLike(value);
    if (startTs == null) {
      return null;
    }
    return { startTs, endTs: startTs + WEEK_MS - 1 };
  }
  if (granularity === "hour") {
    const normalized = /^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}$/.test(value) ? `${value}:00` : value;
    const startTs = parseDateLike(normalized);
    if (startTs == null) {
      return null;
    }
    return { startTs, endTs: startTs + HOUR_MS - 1 };
  }
  const startTs = parseDateLike(value);
  if (startTs == null) {
    return null;
  }
  return { startTs, endTs: startTs + DAY_MS - 1 };
}

export function normalizeTrendPoints(points: Array<Record<string, unknown>>, granularity: ChartGranularity): TrendResolvedPoint[] {
  const resolved: TrendResolvedPoint[] = [];
  for (const item of points) {
    const bucket = String(item.bucket || item.label || "").trim();
    const label = String(item.label || bucket);
    const range = getTrendBucketRange(bucket, granularity);
    if (!bucket || !range) {
      continue;
    }
    const inAmount = toNum(item.in_amount);
    const outAmount = toNum(item.out_amount);
    const netAmount = toNum(item.net_amount);
    const totalAmount = toNum(item.total_amount);
    const inCount = toNum(item.in_count);
    const outCount = toNum(item.out_count);
    const netCount = toNum(item.net_count);
    const totalCount = toNum(item.total_count);
    if (
      inAmount == null ||
      outAmount == null ||
      netAmount == null ||
      totalAmount == null ||
      inCount == null ||
      outCount == null ||
      netCount == null ||
      totalCount == null
    ) {
      continue;
    }
    resolved.push({
      bucket,
      label,
      startTs: range.startTs,
      endTs: range.endTs,
      inAmount,
      outAmount,
      netAmount,
      totalAmount,
      inCount,
      outCount,
      netCount,
      totalCount,
    });
  }
  resolved.sort((left, right) => left.startTs - right.startTs);
  return resolved;
}

function getTrendDisplayGranularity(durationMs: number): TrendDisplayGranularity {
  if (durationMs > DAY_MS * 365 * 10) {
    return "year";
  }
  if (durationMs > DAY_MS * 365 * 3) {
    return "month";
  }
  if (durationMs > DAY_MS * 120) {
    return "week";
  }
  if (durationMs > HOUR_MS * 72) {
    return "day";
  }
  return "hour";
}

function getTrendRawResolutionRank(granularity: ChartGranularity): number {
  if (granularity === "hour") {
    return 4;
  }
  if (granularity === "day") {
    return 3;
  }
  if (granularity === "week") {
    return 2;
  }
  return 1;
}

function getTrendDisplayRank(granularity: TrendDisplayGranularity): number {
  if (granularity === "year") {
    return 0;
  }
  if (granularity === "month") {
    return 1;
  }
  if (granularity === "week") {
    return 2;
  }
  if (granularity === "day") {
    return 3;
  }
  return 4;
}

function resolveTrendDisplayGranularity(desired: TrendDisplayGranularity, rawGranularity: ChartGranularity): TrendDisplayGranularity {
  const rank = Math.min(getTrendDisplayRank(desired), getTrendRawResolutionRank(rawGranularity));
  if (rank <= 0) {
    return "year";
  }
  if (rank === 1) {
    return "month";
  }
  if (rank === 2) {
    return "week";
  }
  if (rank === 3) {
    return "day";
  }
  return "hour";
}

export function getTrendDisplayGranularityWithHysteresis(
  durationMs: number,
  rawGranularity: ChartGranularity,
  previousGranularity?: TrendDisplayGranularity
): TrendDisplayGranularity {
  const current = resolveTrendDisplayGranularity(previousGranularity || getTrendDisplayGranularity(durationMs), rawGranularity);
  if (current === "year") {
    return resolveTrendDisplayGranularity(durationMs < DAY_MS * 365 * 9 ? "month" : "year", rawGranularity);
  }
  if (current === "month") {
    if (durationMs > DAY_MS * 365 * 12) {
      return resolveTrendDisplayGranularity("year", rawGranularity);
    }
    if (durationMs < DAY_MS * 365 * 2.5) {
      return resolveTrendDisplayGranularity("week", rawGranularity);
    }
    return resolveTrendDisplayGranularity("month", rawGranularity);
  }
  if (current === "week") {
    if (durationMs > DAY_MS * 365 * 3.5) {
      return resolveTrendDisplayGranularity("month", rawGranularity);
    }
    if (durationMs < DAY_MS * 90) {
      return resolveTrendDisplayGranularity("day", rawGranularity);
    }
    return resolveTrendDisplayGranularity("week", rawGranularity);
  }
  if (current === "day") {
    if (durationMs > DAY_MS * 150) {
      return resolveTrendDisplayGranularity("week", rawGranularity);
    }
    if (durationMs < HOUR_MS * 60) {
      return resolveTrendDisplayGranularity("hour", rawGranularity);
    }
    return resolveTrendDisplayGranularity("day", rawGranularity);
  }
  return resolveTrendDisplayGranularity(durationMs > HOUR_MS * 96 ? "day" : "hour", rawGranularity);
}

function normalizeTrendFetchGranularity(granularity: ChartGranularity): "month" | "week" | "day" | "hour" {
  if (granularity === "month") {
    return "month";
  }
  if (granularity === "week") {
    return "week";
  }
  if (granularity === "hour") {
    return "hour";
  }
  return "day";
}

export function getRecommendedTrendFetchGranularity(durationMs: number, currentGranularity?: ChartGranularity): ChartGranularity {
  const current = normalizeTrendFetchGranularity(currentGranularity || getRecommendedTrendFetchGranularityBase(durationMs));
  if (current === "month") {
    if (durationMs < DAY_MS * 365 * 2.5) {
      return "week";
    }
    return "month";
  }
  if (current === "week") {
    if (durationMs > DAY_MS * 365 * 3.5) {
      return "month";
    }
    if (durationMs < DAY_MS * 90) {
      return "day";
    }
    return "week";
  }
  if (current === "hour") {
    return durationMs > HOUR_MS * 96 ? "day" : "hour";
  }
  if (durationMs > DAY_MS * 365 * 3.5) {
    return "month";
  }
  if (durationMs > DAY_MS * 150) {
    return "week";
  }
  if (durationMs < HOUR_MS * 60) {
    return "hour";
  }
  return "day";
}

export function getRecommendedTrendFetchGranularityBase(durationMs: number): ChartGranularity {
  if (durationMs > DAY_MS * 365 * 3.5) {
    return "month";
  }
  if (durationMs > DAY_MS * 150) {
    return "week";
  }
  if (durationMs > HOUR_MS * 60) {
    return "day";
  }
  return "hour";
}

export function getTrendMinimumViewportDuration(granularity: ChartGranularity): number {
  if (granularity === "hour") {
    return HOUR_MS * 6;
  }
  if (granularity === "month") {
    return DAY_MS * 21;
  }
  if (granularity === "week") {
    return DAY_MS * 3;
  }
  return HOUR_MS * 12;
}

export function clampTrendViewport(viewport: TrendViewport, fullRange: TrendViewport, minDuration: number): TrendViewport {
  const fullDuration = Math.max(1, fullRange.endTs - fullRange.startTs);
  const nextDuration = clamp(viewport.endTs - viewport.startTs, Math.min(minDuration, fullDuration), fullDuration);
  let startTs = viewport.startTs;
  let endTs = startTs + nextDuration;
  if (startTs < fullRange.startTs) {
    startTs = fullRange.startTs;
    endTs = startTs + nextDuration;
  }
  if (endTs > fullRange.endTs) {
    endTs = fullRange.endTs;
    startTs = endTs - nextDuration;
  }
  return {
    startTs: clamp(startTs, fullRange.startTs, fullRange.endTs),
    endTs: clamp(endTs, fullRange.startTs, fullRange.endTs),
  };
}

function buildTrendAggregationKey(timestamp: number, granularity: TrendDisplayGranularity): string {
  const date = new Date(timestamp);
  if (granularity === "year") {
    return String(date.getFullYear());
  }
  if (granularity === "month") {
    return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}`;
  }
  if (granularity === "week") {
    return formatDateOnly(getWeekStartTs(timestamp));
  }
  if (granularity === "hour") {
    return `${formatDateOnly(timestamp)} ${pad2(date.getHours())}:00`;
  }
  return formatDateOnly(timestamp);
}

export function aggregateTrendDisplayPoints(points: TrendResolvedPoint[], viewport: TrendViewport, granularity: TrendDisplayGranularity): TrendDisplayPoint[] {
  const grouped = new Map<string, TrendDisplayPoint>();
  for (const point of points) {
    if (point.endTs < viewport.startTs || point.startTs > viewport.endTs) {
      continue;
    }
    const key = buildTrendAggregationKey(point.startTs, granularity);
    const existing = grouped.get(key);
    if (existing) {
      existing.startTs = Math.min(existing.startTs, point.startTs);
      existing.endTs = Math.max(existing.endTs, point.endTs);
      existing.inAmount += point.inAmount;
      existing.outAmount += point.outAmount;
      existing.netAmount += point.netAmount;
      existing.totalAmount += point.totalAmount;
      existing.inCount += point.inCount;
      existing.outCount += point.outCount;
      existing.netCount += point.netCount;
      existing.totalCount += point.totalCount;
      existing.sourceBucketCount += 1;
      continue;
    }
    grouped.set(key, {
      ...point,
      key,
      label: formatTrendBucketTitle(point.startTs, granularity),
      axisLabel: "",
      rangeLabel: "",
      sourceBucketCount: 1,
    });
  }

  return Array.from(grouped.values())
    .sort((left, right) => left.startTs - right.startTs)
    .map((point) => ({
      ...point,
      axisLabel: formatTrendAxisLabel(point.startTs, granularity, viewport.startTs, viewport.endTs),
      rangeLabel: formatTrendRangeLabel(point.startTs, point.endTs, granularity),
      totalAmount: point.inAmount + point.outAmount,
      totalCount: point.inCount + point.outCount,
      netAmount: point.inAmount - point.outAmount,
      netCount: point.inCount - point.outCount,
    }));
}

export function getTrendPointValue(point: TrendResolvedPoint | TrendDisplayPoint, metricMode: ChartMetricMode, field: "in" | "out" | "net" | "total"): number {
  const useCount = metricMode === "count";
  if (field === "in") {
    return useCount ? point.inCount : point.inAmount;
  }
  if (field === "out") {
    return useCount ? point.outCount : point.outAmount;
  }
  if (field === "net") {
    return useCount ? point.netCount : point.netAmount;
  }
  return useCount ? point.totalCount : point.totalAmount;
}

export function getTrendAxisUnit(metricMode: ChartMetricMode, maxValue: number): TrendAxisUnit {
  if (metricMode === "count") {
    return { unit: "笔", divisor: 1, precision: 0 };
  }
  if (maxValue >= 100000000) {
    return { unit: "亿元", divisor: 100000000, precision: 2 };
  }
  if (maxValue >= 10000) {
    return { unit: "万元", divisor: 10000, precision: 1 };
  }
  return { unit: "元", divisor: 1, precision: 0 };
}

export function formatTrendAxisTick(value: number, axisUnit: TrendAxisUnit): string {
  const scaled = value / axisUnit.divisor;
  return scaled.toLocaleString("zh-CN", {
    minimumFractionDigits: axisUnit.precision,
    maximumFractionDigits: axisUnit.precision,
  });
}

function getQuantile(values: number[], ratio: number): number {
  if (!values.length) {
    return 0;
  }
  const index = (values.length - 1) * ratio;
  const lower = Math.floor(index);
  const upper = Math.ceil(index);
  if (lower === upper) {
    return values[lower];
  }
  const weight = index - lower;
  return values[lower] * (1 - weight) + values[upper] * weight;
}

export function getTrendClipCeiling(values: number[]): { ceiling: number; clipped: boolean } {
  const normalized = values.filter((value) => value > 0).sort((left, right) => left - right);
  const maxValue = normalized[normalized.length - 1] ?? 1;
  if (normalized.length < 4) {
    return { ceiling: maxValue, clipped: false };
  }
  const p75 = getQuantile(normalized, 0.75);
  const p90 = getQuantile(normalized, 0.9);
  const secondLargest = normalized[normalized.length - 2] ?? maxValue;
  const candidate = Math.max(p75 * 1.45, p90 * 1.18, secondLargest * 1.06);
  if (maxValue > candidate * 1.28) {
    return { ceiling: candidate, clipped: true };
  }
  return { ceiling: maxValue, clipped: false };
}
