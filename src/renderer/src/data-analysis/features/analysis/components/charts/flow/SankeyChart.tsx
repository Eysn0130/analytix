import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";

import type { ChartFilterToken } from "../../../api/stats-api";
import { projectDetectedOrdinaryRestrictedPii } from "../../../../shared/ordinary-pii-projection";
import {
  buildSankeyChartLayout,
  SANKEY_CHART_HEIGHT,
  SANKEY_CHART_MIN_HEIGHT,
  SANKEY_CHART_MIN_WIDTH,
  SANKEY_CHART_WIDTH,
  truncateSankeyLabel,
  type SankeyChartItem,
} from "../../../model/stats-sankey-render-model";
import {
  formatCaseFactCount,
  formatCaseFactMoney,
} from "../../../model/stats-case-fact-number-model";
import { sameToken } from "../basic/chart-render-utils";

interface FlowHoverState {
  item: SankeyChartItem;
  x: number;
  y: number;
  align: "left" | "right";
}

interface SankeyChartProps {
  centerLabel: string;
  inboundItems: Array<Record<string, unknown>>;
  outboundItems: Array<Record<string, unknown>>;
  direction: "in" | "out";
  activeToken: ChartFilterToken | null;
  onSelect: (token: ChartFilterToken) => void;
}

function fmtMoney(value: unknown): string {
  return formatCaseFactMoney(value);
}

function fmtCount(value: unknown): string {
  return formatCaseFactCount(value);
}

function formatRangeLabel(start: string, end: string): string {
  const startText = String(start || "").trim();
  const endText = String(end || "").trim();
  if (!startText && !endText) {
    return "未限定";
  }
  if (startText && endText) {
    return `${startText} 至 ${endText}`;
  }
  return startText || endText;
}

function formatFlowTimeRange(start: string, end: string): string {
  const startText = String(start || "").trim();
  const endText = String(end || "").trim();
  if (!startText && !endText) {
    return "未限定";
  }
  if (startText && endText && startText === endText) {
    return startText;
  }
  return formatRangeLabel(startText, endText);
}

function flowRibbonFill(direction: "in" | "out", phase: "lead" | "merge", alpha: number, active: boolean, hovered: boolean): string {
  const tone = direction === "in" ? (phase === "lead" ? [101, 214, 186] : [84, 204, 177]) : phase === "lead" ? [236, 161, 122] : [230, 149, 108];
  const [r, g, b] = hovered ? tone.map((value, index) => (index === 1 ? Math.min(255, value + 10) : Math.max(0, value - 4))) : tone;
  const boostedAlpha = active ? Math.min(alpha + 0.12, 0.96) : hovered ? Math.min(alpha + 0.08, 0.9) : alpha;
  return `rgba(${r}, ${g}, ${b}, ${boostedAlpha.toFixed(3)})`;
}

export function SankeyChart({
  centerLabel: _centerLabel,
  inboundItems,
  outboundItems,
  direction,
  activeToken,
  onSelect,
}: SankeyChartProps): JSX.Element {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [size, setSize] = useState({ width: SANKEY_CHART_WIDTH, height: SANKEY_CHART_HEIGHT });
  const isInbound = direction === "in";
  const [hovered, setHovered] = useState<FlowHoverState | null>(null);
  useEffect(() => {
    const element = containerRef.current;
    if (!element) {
      return;
    }
    const updateSize = (): void => {
      const rect = element.getBoundingClientRect();
      const nextWidth = Math.max(SANKEY_CHART_MIN_WIDTH, Math.round(rect.width || element.clientWidth || SANKEY_CHART_WIDTH));
      const nextHeight = Math.max(SANKEY_CHART_MIN_HEIGHT, Math.round(rect.height || element.clientHeight || SANKEY_CHART_HEIGHT));
      setSize((prev) => (Math.abs(prev.width - nextWidth) <= 1 && Math.abs(prev.height - nextHeight) <= 1 ? prev : { width: nextWidth, height: nextHeight }));
    };

    updateSize();
    if (typeof ResizeObserver === "undefined") {
      if (typeof window !== "undefined") {
        window.addEventListener("resize", updateSize);
        return () => window.removeEventListener("resize", updateSize);
      }
      return;
    }
    const observer = new ResizeObserver(() => updateSize());
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const width = size.width;
  const height = size.height;
  const rawItems = isInbound ? inboundItems : outboundItems;
  const layout = useMemo(
    () =>
      buildSankeyChartLayout({
        rawItems,
        direction,
        width,
        height,
      }),
    [direction, height, rawItems, width]
  );
  const tooltipAlign = "right";
  const buildHoverState = (item: SankeyChartItem, x: number, y: number): FlowHoverState => ({
    item,
    x,
    y,
    align: tooltipAlign,
  });
  const hoveredLabel = hovered?.item.label || "";
  const rows = layout.rows.map((row) => {
    const active = activeToken ? sameToken(activeToken, row.token) : false;
    const hoveredRow = hoveredLabel === row.item.label;
    const dimmed = Boolean(hoveredLabel) && !hoveredRow;
    const fillAlpha = dimmed ? Math.max(row.rankAlpha * 0.28, 0.1) : row.rankAlpha;
    return {
      ...row,
      displayLabel: projectDetectedOrdinaryRestrictedPii(row.item.label),
      active,
      hoveredRow,
      dimmed,
      flowFill: flowRibbonFill(direction, "lead", fillAlpha, active, hoveredRow),
    };
  });

  return (
    <div ref={containerRef} className={`sankey-layout sankey-layout--single ${hoveredLabel ? "is-hovering" : ""}`}>
      <svg viewBox={`0 0 ${width} ${height}`} className="chart-svg sankey-chart sankey-chart--single" role="img" aria-label="资金流向图">
        {rows.map((row) => (
          <g
            key={`${direction}-${row.item.label}`}
            className={`sankey-chart__row ${row.active ? "is-active" : ""} ${row.hoveredRow ? "is-hovered" : ""} ${row.dimmed ? "is-dimmed" : ""}`}
            onClick={() => onSelect(row.token)}
            onMouseEnter={() => setHovered(buildHoverState(row.item, row.hoverX, row.centerY))}
            onMouseLeave={() => setHovered((current) => (current?.item.label === row.item.label ? null : current))}
            onFocus={() => setHovered(buildHoverState(row.item, row.hoverX, row.centerY))}
            onBlur={() => setHovered((current) => (current?.item.label === row.item.label ? null : current))}
            tabIndex={0}
          >
            <title>{row.displayLabel}</title>
            <path
              d={row.ribbonPath}
              className={`sankey-chart__ribbon sankey-chart__ribbon--${direction} ${row.active ? "is-active" : ""} ${row.hoveredRow ? "is-hovered" : ""} ${row.dimmed ? "is-dimmed" : ""}`}
              style={{ fill: row.flowFill } as CSSProperties}
            />
            <rect
              x={layout.leftNodeX}
              y={row.nodeY}
              width={layout.nodeWidth}
              height={layout.nodeHeight}
              rx={0}
              className={`sankey-chart__node-card sankey-chart__node-card--counterparty sankey-chart__node-card--${direction} ${row.active ? "is-active" : ""}`}
            />
            <text
              x={layout.labelTextX}
              y={row.centerY + 3.5}
              className="sankey-chart__label sankey-chart__label--counterparty"
              textAnchor="end"
            >
              {truncateSankeyLabel(row.displayLabel, layout.labelCharLimit)}
            </text>
            <text
              x={layout.amountTextX}
              y={row.centerY - Math.max(row.flowWidth * 0.16, 1.4)}
              className="sankey-chart__label sankey-chart__label--metric"
              textAnchor="start"
            >
              {fmtMoney(row.item.amount)}
            </text>
          </g>
        ))}
        <rect
          x={layout.targetX}
          y={layout.stackTop}
          width={layout.nodeWidth}
          height={layout.stackHeight}
          rx={0}
          className={`sankey-chart__node-card sankey-chart__node-card--current sankey-chart__node-card--${direction} ${hoveredLabel ? "is-active" : ""}`}
        />
      </svg>
      {hovered ? (
        <div
          className={`sankey-chart__tooltip ${hovered.align === "right" ? "is-align-right" : ""}`}
          style={{
            left: `${(hovered.x / width) * 100}%`,
            top: `${(hovered.y / height) * 100}%`,
          }}
        >
          <div className="sankey-chart__tooltip-title">{projectDetectedOrdinaryRestrictedPii(hovered.item.label)}</div>
          <div className="sankey-chart__tooltip-row">
            <span>金额</span>
            <strong>{fmtMoney(hovered.item.amount)}</strong>
          </div>
          <div className="sankey-chart__tooltip-row">
            <span>次数</span>
            <strong>{fmtCount(hovered.item.count)} 笔</strong>
          </div>
          <div className="sankey-chart__tooltip-row sankey-chart__tooltip-row--range">
            <span>时间范围</span>
            <strong>{formatFlowTimeRange(hovered.item.firstTime, hovered.item.lastTime)}</strong>
          </div>
        </div>
      ) : null}
    </div>
  );
}
