import type { ChartFilterToken, ChartGranularity } from "../../../api/stats-api";

import {
  buildSvgPath,
  buildTimeBucketToken,
  sameToken,
  shortDateLabel,
  type BalanceTrendPoint,
} from "./chart-render-utils";

interface BalanceTrendChartProps {
  points: BalanceTrendPoint[];
  markers: Array<Record<string, unknown>>;
  granularity: ChartGranularity;
  mode: "balance" | "cumulative-net";
  activeToken: ChartFilterToken | null;
  onSelect: (token: ChartFilterToken) => void;
}

export function BalanceTrendChart({
  points,
  markers,
  granularity,
  mode,
  activeToken,
  onSelect,
}: BalanceTrendChartProps): JSX.Element {
  const width = 360;
  const height = 240;
  const padding = { top: 14, right: 12, bottom: 42, left: 20 };
  const chartWidth = width - padding.left - padding.right;
  const chartHeight = height - padding.top - padding.bottom;
  const values = points;
  const maxValue = Math.max(...values.map((point) => point.value), 0);
  const minValue = Math.min(...values.map((point) => point.value), 0);
  const range = Math.max(1, maxValue - minValue);
  const stepX = values.length > 1 ? chartWidth / (values.length - 1) : chartWidth / 2;
  const labelStep = values.length > 8 ? Math.ceil(values.length / 6) : 1;
  const toY = (value: number): number => padding.top + chartHeight - ((value - minValue) / range) * chartHeight;
  const chartPoints = values.map((point, index) => ({
    ...point,
    x: padding.left + (values.length > 1 ? index * stepX : chartWidth / 2),
    y: toY(point.value),
  }));

  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="chart-svg balance-chart" role="img" aria-label={mode === "balance" ? "余额变化趋势" : "净额累计趋势"}>
      {Array.from({ length: 4 }, (_, index) => {
        const y = padding.top + (chartHeight / 3) * index;
        return <line key={index} x1={padding.left} y1={y} x2={width - padding.right} y2={y} className="chart-grid-line" />;
      })}
      <line x1={padding.left} y1={padding.top + chartHeight} x2={width - padding.right} y2={padding.top + chartHeight} className="chart-axis-line" />
      <path d={buildSvgPath(chartPoints)} className={`chart-line ${mode === "balance" ? "chart-line--balance" : "chart-line--net-alt"}`} />
      {markers.slice(0, 4).map((marker, markerIndex) => {
        const bucket = String(marker.bucket || "");
        const targetPoint = chartPoints.find((point) => point.bucket === bucket);
        if (!targetPoint) {
          return null;
        }
        const token = buildTimeBucketToken("balance", bucket, String(marker.label || bucket), granularity);
        const active = activeToken ? sameToken(activeToken, token) : false;
        return (
          <g key={`marker-${markerIndex}-${bucket}-${String(marker.txn_id || marker.label || "point")}`}>
            <line x1={targetPoint.x} y1={padding.top} x2={targetPoint.x} y2={padding.top + chartHeight} className={`balance-chart__marker-line ${active ? "is-active" : ""}`} />
            <circle cx={targetPoint.x} cy={targetPoint.y} r={active ? 6 : 4.5} className="balance-chart__marker-point" onClick={() => onSelect(token)} />
          </g>
        );
      })}
      {chartPoints.map((point, index) => {
        const token = buildTimeBucketToken("balance", point.bucket, point.label, granularity);
        const active = activeToken ? sameToken(activeToken, token) : false;
        return (
          <g key={point.bucket || index}>
            <circle cx={point.x} cy={point.y} r={active ? 5.5 : 3.5} className={`chart-point ${mode === "balance" ? "chart-point--balance" : "chart-point--net"}`} onClick={() => onSelect(token)} />
            {(index % labelStep === 0 || index === chartPoints.length - 1) && (
              <text x={point.x} y={height - 12} textAnchor="middle" className="chart-axis-label">
                {shortDateLabel(point.label)}
              </text>
            )}
          </g>
        );
      })}
    </svg>
  );
}
