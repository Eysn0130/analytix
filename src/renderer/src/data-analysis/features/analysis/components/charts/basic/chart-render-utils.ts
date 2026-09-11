import type { ChartFilterToken, ChartGranularity } from "../../../api/stats-api";
import {
  readFiniteCaseFactNumber,
  readNonNegativeCaseFactInteger,
} from "../../../model/stats-case-fact-number-model";

export interface NumericChartItem {
  label: string;
  value: number;
  rawValue?: string;
  detail?: string;
  secondaryValue?: string;
  amount?: number;
  count?: number;
  net?: number;
  payload?: Record<string, unknown>;
}

export interface HeatmapChartCell {
  weekday: number;
  hour: number;
  value: number;
}

export interface HistogramChartItem {
  label: string;
  count: number;
}

export interface BalanceTrendPoint {
  bucket: string;
  label: string;
  value: number;
}

export const WEEKDAY_LABELS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];

function hasOwn(item: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(item, key);
}

function readFirstPresentNumber(
  item: Record<string, unknown>,
  keys: string[],
  reader: (value: unknown) => number | null
): { present: boolean; value: number | null } {
  for (const key of keys) {
    if (hasOwn(item, key)) {
      return { present: true, value: reader(item[key]) };
    }
  }
  return { present: false, value: null };
}

function readCompletePair(
  item: Record<string, unknown>,
  leftKey: string,
  rightKey: string,
  reader: (value: unknown) => number | null
): number | null {
  if (!hasOwn(item, leftKey) || !hasOwn(item, rightKey)) {
    return null;
  }
  const left = reader(item[leftKey]);
  const right = reader(item[rightKey]);
  if (left == null || right == null) {
    return null;
  }
  const total = left + right;
  return Number.isFinite(total) ? total : null;
}

function readHeatmapMetric(cell: Record<string, unknown>, metricBasis: "amount" | "count"): number | null {
  if (metricBasis === "count") {
    const direct = readFirstPresentNumber(cell, ["count", "total_count"], readNonNegativeCaseFactInteger);
    if (direct.present) {
      return direct.value;
    }
    const total = readCompletePair(cell, "in_count", "out_count", readNonNegativeCaseFactInteger);
    return total != null && Number.isSafeInteger(total) ? total : null;
  }
  const direct = readFirstPresentNumber(cell, ["value", "total_amount"], readFiniteCaseFactNumber);
  return direct.present
    ? direct.value
    : readCompletePair(cell, "in_amount", "out_amount", readFiniteCaseFactNumber);
}

export function normalizeHeatmapChartCells(
  cells: Array<Record<string, unknown>>,
  metricBasis: "amount" | "count"
): HeatmapChartCell[] {
  return (Array.isArray(cells) ? cells : []).flatMap((cell): HeatmapChartCell[] => {
    const weekday = readNonNegativeCaseFactInteger(cell.weekday);
    const hour = readNonNegativeCaseFactInteger(cell.hour);
    const value = readHeatmapMetric(cell, metricBasis);
    if (weekday == null || weekday > 6 || hour == null || hour > 23 || value == null) {
      return [];
    }
    return [{ weekday, hour, value }];
  });
}

export function normalizeHistogramChartItems(items: Array<Record<string, unknown>>): HistogramChartItem[] {
  return (Array.isArray(items) ? items : []).flatMap((item): HistogramChartItem[] => {
    const label = typeof item.label === "string" ? item.label.trim() : "";
    const count = readNonNegativeCaseFactInteger(item.count);
    return label && count != null ? [{ label, count }] : [];
  });
}

export function normalizeBalanceTrendPoints(
  points: Array<Record<string, unknown>>,
  mode: "balance" | "cumulative-net"
): BalanceTrendPoint[] {
  return (Array.isArray(points) ? points : []).flatMap((point): BalanceTrendPoint[] => {
    const bucket = typeof point.bucket === "string" && point.bucket.trim()
      ? point.bucket.trim()
      : typeof point.label === "string"
        ? point.label.trim()
        : "";
    const label = typeof point.label === "string" && point.label.trim() ? point.label.trim() : bucket;
    const value = readFiniteCaseFactNumber(mode === "balance" ? point.balance : point.value);
    return bucket && value != null ? [{ bucket, label, value }] : [];
  });
}

export function sameToken(a: ChartFilterToken, b: ChartFilterToken): boolean {
  return (
    a.source_panel_id === b.source_panel_id &&
    a.dimension === b.dimension &&
    a.value === b.value &&
    JSON.stringify(a.payload || {}) === JSON.stringify(b.payload || {})
  );
}

export function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

export function shortDateLabel(value: string): string {
  const text = String(value || "").trim();
  if (!text) {
    return "";
  }
  if (/^\d{4}-\d{2}-\d{2}$/.test(text)) {
    return text.slice(5);
  }
  if (/^\d{2}:/.test(text)) {
    return text;
  }
  if (/^\d{4}-\d{2}$/.test(text)) {
    return text;
  }
  return text.replace(/^(\d{4}-)/, "");
}

export function buildSvgPath(points: Array<{ x: number; y: number }>): string {
  if (!points.length) {
    return "";
  }
  return points.map((point, index) => `${index === 0 ? "M" : "L"}${point.x},${point.y}`).join(" ");
}

export function buildTimeBucketToken(
  panelId: string,
  bucket: string,
  label: string,
  granularity: ChartGranularity,
  extraPayload: Record<string, unknown> = {}
): ChartFilterToken {
  return {
    source_panel_id: panelId,
    dimension: "time_bucket",
    value: bucket,
    label,
    payload: { granularity, bucket, ...extraPayload },
  };
}
