import type { CSSProperties } from "react";

import type { ChartFilterToken } from "../../../api/stats-api";

import { clamp, sameToken, WEEKDAY_LABELS, type HeatmapChartCell } from "./chart-render-utils";

interface HeatmapSvgProps {
  cells: HeatmapChartCell[];
  activeToken: ChartFilterToken | null;
  onSelect: (token: ChartFilterToken) => void;
}

export function HeatmapSvg({
  cells,
  activeToken,
  onSelect,
}: HeatmapSvgProps): JSX.Element {
  const width = 360;
  const height = 240;
  const padding = { left: 34, right: 12, top: 32, bottom: 12 };
  const chartWidth = width - padding.left - padding.right;
  const chartHeight = height - padding.top - padding.bottom;
  const cellWidth = chartWidth / 24;
  const cellHeight = chartHeight / 7;
  const maxValue = Math.max(1, ...cells.map((cell) => Math.abs(cell.value)));
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="chart-svg heatmap-chart" role="img" aria-label="时间热力图">
      {Array.from({ length: 24 }, (_, hour) => (
        <text key={`h-${hour}`} x={padding.left + hour * cellWidth + cellWidth / 2} y={18} textAnchor="middle" className="chart-axis-label">
          {hour}
        </text>
      ))}
      {WEEKDAY_LABELS.map((label, index) => (
        <text key={label} x={padding.left - 10} y={padding.top + index * cellHeight + cellHeight / 2 + 3} textAnchor="end" className="chart-axis-label">
          {label}
        </text>
      ))}
      {cells.map((cell) => {
        const { weekday, hour } = cell;
        const intensity = clamp(Math.abs(cell.value) / maxValue, 0.08, 1);
        const token: ChartFilterToken = {
          source_panel_id: "heatmap",
          dimension: "heatmap_cell",
          value: `${weekday}-${hour}`,
          label: `${WEEKDAY_LABELS[weekday] || weekday} ${String(hour).padStart(2, "0")}:00`,
          payload: { weekday, hour },
        };
        const active = activeToken ? sameToken(activeToken, token) : false;
        return (
          <rect
            key={`${weekday}-${hour}`}
            x={padding.left + hour * cellWidth}
            y={padding.top + weekday * cellHeight}
            width={Math.max(cellWidth - 2.4, 8)}
            height={Math.max(cellHeight - 3.2, 16)}
            rx={4}
            className={`heatmap-chart__cell ${active ? "is-active" : ""}`}
            style={{ ["--heatmap-alpha" as string]: String(intensity) } as CSSProperties}
            onClick={() => onSelect(token)}
          />
        );
      })}
    </svg>
  );
}
