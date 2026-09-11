import type { ChartMetricMode } from "../api/stats-api";
import { getTrendPointValue, type TrendDisplayPoint } from "./stats-trend-model";

export interface TrendSeriesVisibility {
  in: boolean;
  out: boolean;
}

export interface TrendOverviewPoint {
  x: number;
  y: number;
}

export interface BuildTrendOverviewPointsInput {
  points: TrendDisplayPoint[];
  metricMode: ChartMetricMode;
  chartLeft: number;
  bandWidth: number;
  navigatorTop: number;
  navigatorHeight: number;
}

export function buildTrendVisibleYValues(
  points: TrendDisplayPoint[],
  metricMode: ChartMetricMode,
  seriesVisibility: TrendSeriesVisibility
): number[] {
  const values: number[] = [];
  for (const point of points) {
    values.push(seriesVisibility.in ? getTrendPointValue(point, metricMode, "in") : 0);
    values.push(seriesVisibility.out ? getTrendPointValue(point, metricMode, "out") : 0);
  }
  return values;
}

export function buildTrendOverviewPoints({
  points,
  metricMode,
  chartLeft,
  bandWidth,
  navigatorTop,
  navigatorHeight,
}: BuildTrendOverviewPointsInput): TrendOverviewPoint[] {
  const totals: number[] = [];
  let maxValue = 1;
  for (const point of points) {
    const value = getTrendPointValue(point, metricMode, "total");
    totals.push(value);
    if (value > maxValue) {
      maxValue = value;
    }
  }

  return points.map((_, index) => {
    const centerX = chartLeft + bandWidth * index + bandWidth / 2;
    const y = navigatorTop + navigatorHeight - (totals[index] / maxValue) * (navigatorHeight - 6);
    return { x: centerX, y };
  });
}
