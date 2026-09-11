import type { ChartFilterToken } from "../../../api/stats-api";

import { sameToken, type HistogramChartItem } from "./chart-render-utils";

interface HistogramChartProps {
  items: HistogramChartItem[];
  activeToken: ChartFilterToken | null;
  onSelect: (token: ChartFilterToken) => void;
}

export function HistogramChart({
  items,
  activeToken,
  onSelect,
}: HistogramChartProps): JSX.Element {
  const width = 360;
  const height = 240;
  const padding = { top: 20, right: 12, bottom: 42, left: 24 };
  const chartWidth = width - padding.left - padding.right;
  const chartHeight = height - padding.top - padding.bottom;
  const maxCount = Math.max(1, ...items.map((item) => item.count));
  const barWidth = items.length ? chartWidth / items.length - 10 : 24;
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="chart-svg histogram-chart" role="img" aria-label="大额交易分布图">
      <line x1={padding.left} y1={padding.top + chartHeight} x2={width - padding.right} y2={padding.top + chartHeight} className="chart-axis-line" />
      {items.map((item, index) => {
        const x = padding.left + index * (barWidth + 10);
        const h = (item.count / maxCount) * chartHeight;
        const token: ChartFilterToken = {
          source_panel_id: "anomalyAmount",
          dimension: "amount_bucket",
          value: item.label,
          label: item.label,
          payload: {},
        };
        const active = activeToken ? sameToken(activeToken, token) : false;
        return (
          <g key={item.label || index}>
            <rect x={x} y={padding.top + chartHeight - h} width={Math.max(18, barWidth)} height={h} rx={8} className={`histogram-chart__bar ${active ? "is-active" : ""}`} onClick={() => onSelect(token)} />
            <text x={x + Math.max(18, barWidth) / 2} y={height - 12} textAnchor="middle" className="chart-axis-label">
              {item.label}
            </text>
          </g>
        );
      })}
    </svg>
  );
}
