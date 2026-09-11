import { useEffect, useMemo, useRef, useState, type CSSProperties, type MouseEvent as ReactMouseEvent, type WheelEvent as ReactWheelEvent } from "react";
import type { ChartFilterToken, ChartGranularity, ChartMetricMode } from "../../../api/stats-api";
import {
  formatCaseFactCount,
  formatCaseFactMoney,
  formatSignedCaseFactMoney,
} from "../../../model/stats-case-fact-number-model";
import {
  buildTrendOverviewPoints,
  buildTrendVisibleYValues,
} from "../../../model/stats-trend-chart-render-model";
import {
  aggregateTrendDisplayPoints,
  clampTrendViewport,
  DAY_MS,
  formatChartDateTime,
  formatDateOnly,
  formatTrendAxisTick,
  getRecommendedTrendFetchGranularity,
  getRecommendedTrendFetchGranularityBase,
  getTrendAxisLabelMinSpacing,
  getTrendAxisUnit,
  getTrendClipCeiling,
  getTrendDisplayGranularityWithHysteresis,
  getTrendMinimumViewportDuration,
  getTrendPointValue,
  normalizeTrendPoints,
  parseDateLike,
  type TrendDisplayGranularity,
  type TrendDisplayPoint,
  type TrendViewport,
} from "../../../model/stats-trend-model";
import { ChartPanel } from "../ChartPanel";

function fmtMoney(value: unknown): string {
  return formatCaseFactMoney(value);
}

function fmtSignedMoney(value: unknown): string {
  return formatSignedCaseFactMoney(value);
}

function fmtCount(value: unknown): string {
  return formatCaseFactCount(value);
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function sameToken(a: ChartFilterToken, b: ChartFilterToken): boolean {
  return a.source_panel_id === b.source_panel_id && a.dimension === b.dimension && a.value === b.value;
}

function buildSvgPath(points: Array<{ x: number; y: number }>): string {
  if (!points.length) {
    return "";
  }
  return points.map((point, index) => `${index === 0 ? "M" : "L"}${point.x},${point.y}`).join(" ");
}

function buildAreaPath(points: Array<{ x: number; y: number }>, baselineY: number): string {
  if (!points.length) {
    return "";
  }
  const first = points[0];
  const last = points[points.length - 1];
  return `${buildSvgPath(points)} L${last.x},${baselineY} L${first.x},${baselineY} Z`;
}

function buildTimeBucketToken(
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

function formatTrendMetric(metricMode: ChartMetricMode, value: number, signed = false): string {
  if (metricMode === "count") {
    const count = Math.abs(value);
    const prefix = signed && value ? (value > 0 ? "+" : "-") : "";
    return `${prefix}${fmtCount(count)} 笔`;
  }
  return signed ? fmtSignedMoney(value) : fmtMoney(value);
}

function buildTrendFilterToken(point: TrendDisplayPoint, rawGranularity: ChartGranularity, displayGranularity: TrendDisplayGranularity): ChartFilterToken {
  return buildTimeBucketToken("trend", point.key, point.label, rawGranularity, {
    range_start: formatChartDateTime(point.startTs),
    range_end: formatChartDateTime(point.endTs),
    display_granularity: displayGranularity,
  });
}

function TrendInteractiveChart({
  points,
  displayGranularity,
  rawGranularity,
  metricMode,
  viewport,
  fullRange,
  seriesVisibility,
  activeToken,
  onViewportChange,
  onSelect,
  onResetView,
}: {
  points: TrendDisplayPoint[];
  displayGranularity: TrendDisplayGranularity;
  rawGranularity: ChartGranularity;
  metricMode: ChartMetricMode;
  viewport: TrendViewport;
  fullRange: TrendViewport;
  seriesVisibility: { in: boolean; out: boolean };
  activeToken: ChartFilterToken | null;
  onViewportChange: (viewport: TrendViewport) => void;
  onSelect: (token: ChartFilterToken) => void;
  onResetView: () => void;
}): JSX.Element {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const onViewportChangeRef = useRef(onViewportChange);
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
  const dragStateRef = useRef<{
    mode: "chart-pan" | "navigator-window" | "navigator-left" | "navigator-right";
    startClientX: number;
    originViewport: TrendViewport;
    trackWidthPx: number;
    moved: boolean;
  } | null>(null);
  const suppressClickRef = useRef(false);
  const width = 960;
  const height = 432;
  const padding = { top: 20, right: 22, bottom: 14, left: 68 };
  const axisLabelOffset = 14;
  const axisLabelHeight = 12;
  const navigatorGap = 18;
  const navigatorHeight = 36;
  const navigatorLabelOffset = 12;
  const navigatorLabelHeight = 10;
  const chartHeight = height - padding.top - padding.bottom - axisLabelOffset - axisLabelHeight - navigatorGap - navigatorHeight - navigatorLabelOffset - navigatorLabelHeight;
  const barBottom = padding.top + chartHeight;
  const axisLabelY = barBottom + axisLabelOffset;
  const navigatorTop = axisLabelY + axisLabelHeight + navigatorGap;
  const navigatorLabelY = navigatorTop + navigatorHeight + navigatorLabelOffset;
  const chartWidth = width - padding.left - padding.right;
  const minViewportDuration = Math.min(getTrendMinimumViewportDuration(rawGranularity), Math.max(1, fullRange.endTs - fullRange.startTs));
  const viewportDuration = Math.max(1, viewport.endTs - viewport.startTs);
  const visibleSeriesCount = Number(seriesVisibility.in) + Number(seriesVisibility.out);
  const chartTokens = useMemo(() => points.map((point) => buildTrendFilterToken(point, rawGranularity, displayGranularity)), [displayGranularity, points, rawGranularity]);
  const activeIndex = useMemo(() => {
    if (!activeToken) {
      return -1;
    }
    return chartTokens.findIndex((token) => sameToken(token, activeToken));
  }, [activeToken, chartTokens]);
  const hoveredPoint = hoveredIndex != null ? points[hoveredIndex] : null;
  const yValues = useMemo(
    () => buildTrendVisibleYValues(points, metricMode, seriesVisibility),
    [metricMode, points, seriesVisibility]
  );
  const clipState = useMemo(() => getTrendClipCeiling(yValues), [yValues]);
  const axisMax = Math.max(1, clipState.ceiling);
  const axisUnit = useMemo(() => getTrendAxisUnit(metricMode, axisMax), [axisMax, metricMode]);
  const tickValues = useMemo(() => Array.from({ length: 5 }, (_, index) => (axisMax / 4) * (4 - index)), [axisMax]);
  const bandWidth = points.length ? chartWidth / points.length : chartWidth;
  const groupWidth = Math.max(2, Math.min(60, bandWidth * 0.74));
  const barGap = visibleSeriesCount > 1 ? Math.min(2, Math.max(0.5, groupWidth * 0.14)) : 0;
  const singleBarWidth = visibleSeriesCount > 1 ? Math.max(0.8, (groupWidth - barGap) / 2) : Math.max(1.2, Math.min(groupWidth, 30));
  const barRadius = Math.min(5, Math.max(0.5, singleBarWidth / 2));
  const bucketHitWidth = Math.max(bandWidth, Math.min(24, bandWidth + 6));
  const maxVisibleLabels = Math.max(3, Math.floor(chartWidth / getTrendAxisLabelMinSpacing(displayGranularity)));
  const labelStep = points.length > maxVisibleLabels ? Math.ceil(points.length / maxVisibleLabels) : 1;
  const overviewPoints = useMemo(
    () =>
      buildTrendOverviewPoints({
        points,
        metricMode,
        chartLeft: padding.left,
        bandWidth,
        navigatorTop,
        navigatorHeight,
      }),
    [bandWidth, metricMode, navigatorHeight, navigatorTop, padding.left, points]
  );

  useEffect(() => {
    onViewportChangeRef.current = onViewportChange;
  }, [onViewportChange]);

  useEffect(() => {
    const handleMouseMove = (event: MouseEvent): void => {
      const dragState = dragStateRef.current;
      if (!dragState) {
        return;
      }
      const deltaX = event.clientX - dragState.startClientX;
      if (Math.abs(deltaX) > 3) {
        dragState.moved = true;
      }
      const fullDuration = Math.max(1, fullRange.endTs - fullRange.startTs);
      if (dragState.mode === "chart-pan") {
        const originDuration = dragState.originViewport.endTs - dragState.originViewport.startTs;
        const shift = (deltaX / Math.max(1, dragState.trackWidthPx)) * originDuration;
        onViewportChangeRef.current(
          clampTrendViewport(
            {
              startTs: dragState.originViewport.startTs - shift,
              endTs: dragState.originViewport.endTs - shift,
            },
            fullRange,
            minViewportDuration
          )
        );
        return;
      }
      if (dragState.mode === "navigator-window") {
        const shift = (deltaX / Math.max(1, dragState.trackWidthPx)) * fullDuration;
        onViewportChangeRef.current(
          clampTrendViewport(
            {
              startTs: dragState.originViewport.startTs + shift,
              endTs: dragState.originViewport.endTs + shift,
            },
            fullRange,
            minViewportDuration
          )
        );
        return;
      }
      if (dragState.mode === "navigator-left") {
        const nextStartTs = clamp(
          dragState.originViewport.startTs + (deltaX / Math.max(1, dragState.trackWidthPx)) * fullDuration,
          fullRange.startTs,
          dragState.originViewport.endTs - minViewportDuration
        );
        onViewportChangeRef.current({
          startTs: nextStartTs,
          endTs: dragState.originViewport.endTs,
        });
        return;
      }
      const nextEndTs = clamp(
        dragState.originViewport.endTs + (deltaX / Math.max(1, dragState.trackWidthPx)) * fullDuration,
        dragState.originViewport.startTs + minViewportDuration,
        fullRange.endTs
      );
      onViewportChangeRef.current({
        startTs: dragState.originViewport.startTs,
        endTs: nextEndTs,
      });
    };

    const handleMouseUp = (): void => {
      if (dragStateRef.current?.moved) {
        suppressClickRef.current = true;
        window.setTimeout(() => {
          suppressClickRef.current = false;
        }, 0);
      }
      dragStateRef.current = null;
    };

    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("mouseup", handleMouseUp);
    return () => {
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("mouseup", handleMouseUp);
    };
  }, [fullRange, minViewportDuration]);

  const beginDrag = (clientX: number, mode: "chart-pan" | "navigator-window" | "navigator-left" | "navigator-right"): void => {
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect) {
      return;
    }
    dragStateRef.current = {
      mode,
      startClientX: clientX,
      originViewport: viewport,
      trackWidthPx: (chartWidth / width) * rect.width,
      moved: false,
    };
  };

  const handleWheel = (event: ReactWheelEvent<HTMLDivElement>): void => {
    if (!points.length) {
      return;
    }
    event.preventDefault();
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect) {
      return;
    }
    const plotLeftPx = (padding.left / width) * rect.width;
    const plotWidthPx = (chartWidth / width) * rect.width;
    const relativeX = clamp(event.clientX - rect.left - plotLeftPx, 0, plotWidthPx);
    const ratio = plotWidthPx ? relativeX / plotWidthPx : 0.5;
    const fullDuration = Math.max(1, fullRange.endTs - fullRange.startTs);
    const targetDuration = clamp(
      viewportDuration *
        (event.deltaY > 0
          ? 1 + clamp(Math.abs(event.deltaY) / 220, 0.18, 0.82) * 0.09
          : 1 / (1 + clamp(Math.abs(event.deltaY) / 220, 0.18, 0.82) * 0.09)),
      minViewportDuration,
      fullDuration
    );
    const anchorTs = viewport.startTs + viewportDuration * ratio;
    const nextViewport = clampTrendViewport(
      {
        startTs: anchorTs - targetDuration * ratio,
        endTs: anchorTs + targetDuration * (1 - ratio),
      },
      fullRange,
      minViewportDuration
    );
    onViewportChange(nextViewport);
  };

  const handleNavigatorJump = (event: ReactMouseEvent<SVGRectElement>): void => {
    if (dragStateRef.current?.moved) {
      return;
    }
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect) {
      return;
    }
    const plotLeftPx = (padding.left / width) * rect.width;
    const plotWidthPx = (chartWidth / width) * rect.width;
    const relativeX = clamp(event.clientX - rect.left - plotLeftPx, 0, plotWidthPx);
    const ratio = plotWidthPx ? relativeX / plotWidthPx : 0.5;
    const fullDuration = Math.max(1, fullRange.endTs - fullRange.startTs);
    const centerTs = fullRange.startTs + fullDuration * ratio;
    onViewportChange(
      clampTrendViewport(
        {
          startTs: centerTs - viewportDuration / 2,
          endTs: centerTs + viewportDuration / 2,
        },
        fullRange,
        minViewportDuration
      )
    );
  };

  const navigatorWindowX = padding.left + ((viewport.startTs - fullRange.startTs) / Math.max(1, fullRange.endTs - fullRange.startTs)) * chartWidth;
  const navigatorWindowWidth = Math.max(24, (viewportDuration / Math.max(1, fullRange.endTs - fullRange.startTs)) * chartWidth);
  const navigatorLeftMaskWidth = Math.max(0, navigatorWindowX - padding.left);
  const navigatorRightMaskX = navigatorWindowX + Math.min(navigatorWindowWidth, chartWidth);
  const navigatorRightMaskWidth = Math.max(0, padding.left + chartWidth - navigatorRightMaskX);
  const hoveredHasClippedValue = hoveredPoint
    ? (seriesVisibility.in && getTrendPointValue(hoveredPoint, metricMode, "in") > axisMax) ||
      (seriesVisibility.out && getTrendPointValue(hoveredPoint, metricMode, "out") > axisMax)
    : false;
  const tooltipY = hoveredIndex != null
    ? padding.top + 12
    : 0;

  return (
    <div ref={containerRef} className="trend-chart-workspace" onWheel={handleWheel}>
      <svg viewBox={`0 0 ${width} ${height}`} className="chart-svg trend-chart" role="img" aria-label="账户进/出账时段分布">
        <text x={padding.left} y={padding.top - 6} className="trend-chart__axis-unit">
          {metricMode === "count" ? "单位：笔" : `单位：${axisUnit.unit}`}
        </text>
        {tickValues.map((value, index) => {
          const ratio = value / axisMax;
          const y = padding.top + chartHeight - ratio * chartHeight;
          return (
            <g key={`tick-${index}`}>
              <line x1={padding.left} y1={y} x2={width - padding.right} y2={y} className={`chart-grid-line ${value === 0 ? "chart-grid-line--baseline" : ""}`} />
              <text x={padding.left - 10} y={y + 4} textAnchor="end" className="chart-axis-label">
                {formatTrendAxisTick(value, axisUnit)}
              </text>
            </g>
          );
        })}
        <line x1={padding.left} y1={barBottom} x2={width - padding.right} y2={barBottom} className="chart-axis-line trend-chart__zero-line" />
        <rect
          x={padding.left}
          y={padding.top}
          width={chartWidth}
          height={chartHeight}
          className="trend-chart__pan-surface"
          onMouseDown={(event) => beginDrag(event.clientX, "chart-pan")}
          onDoubleClick={onResetView}
          onMouseLeave={() => setHoveredIndex(null)}
        />
        {points.map((point, index) => {
          const centerX = padding.left + bandWidth * index + bandWidth / 2;
          const slotX = centerX - bandWidth / 2;
          const token = chartTokens[index];
          const active = activeIndex === index;
          const hovered = hoveredIndex === index;
          const inValue = seriesVisibility.in ? getTrendPointValue(point, metricMode, "in") : 0;
          const outValue = seriesVisibility.out ? getTrendPointValue(point, metricMode, "out") : 0;
          const inHeight = axisMax ? (Math.min(inValue, axisMax) / axisMax) * chartHeight : 0;
          const outHeight = axisMax ? (Math.min(outValue, axisMax) / axisMax) * chartHeight : 0;
          const inX = visibleSeriesCount > 1 ? centerX - barGap / 2 - singleBarWidth : centerX - singleBarWidth / 2;
          const outX = visibleSeriesCount > 1 ? centerX + barGap / 2 : centerX - singleBarWidth / 2;
          const highlightWidth = Math.max(bandWidth - 2, Math.min(24, bandWidth + 4));
          const hitX = centerX - bucketHitWidth / 2;
          return (
            <g key={point.key} data-trend-bucket-index={index}>
              {hovered || active ? (
                <rect
                  x={centerX - highlightWidth / 2}
                  y={padding.top}
                  width={highlightWidth}
                  height={chartHeight + 4}
                  rx={8}
                  className={`trend-chart__bucket-highlight ${active ? "is-active" : ""} ${hovered ? "is-hovered" : ""}`}
                />
              ) : null}
              {seriesVisibility.in ? (
                <g>
                  <rect x={inX} y={barBottom - inHeight} width={singleBarWidth} height={inHeight} rx={barRadius} className="chart-bar chart-bar--in" />
                  {inValue > axisMax ? (
                    <g className="trend-chart__clip-marker">
                      <line x1={inX + 3} y1={padding.top + 7} x2={inX + singleBarWidth - 3} y2={padding.top + 7} />
                      <line x1={inX + 4} y1={padding.top + 11} x2={inX + singleBarWidth - 4} y2={padding.top + 5} />
                    </g>
                  ) : null}
                </g>
              ) : null}
              {seriesVisibility.out ? (
                <g>
                  <rect x={outX} y={barBottom - outHeight} width={singleBarWidth} height={outHeight} rx={barRadius} className="chart-bar chart-bar--out" />
                  {outValue > axisMax ? (
                    <g className="trend-chart__clip-marker">
                      <line x1={outX + 3} y1={padding.top + 7} x2={outX + singleBarWidth - 3} y2={padding.top + 7} />
                      <line x1={outX + 4} y1={padding.top + 11} x2={outX + singleBarWidth - 4} y2={padding.top + 5} />
                    </g>
                  ) : null}
                </g>
              ) : null}
              {hovered ? <line x1={centerX} y1={padding.top} x2={centerX} y2={barBottom} className="trend-chart__guide-line" /> : null}
              <rect
                x={hitX}
                y={padding.top}
                width={bucketHitWidth}
                height={chartHeight + 10}
                className={`trend-chart__hit ${active ? "is-active" : ""}`}
                onMouseEnter={() => setHoveredIndex(index)}
                onMouseMove={() => setHoveredIndex(index)}
                onMouseLeave={() => setHoveredIndex((prev) => (prev === index ? null : prev))}
                onClick={() => {
                  if (suppressClickRef.current) {
                    return;
                  }
                  onSelect(token);
                }}
              />
              {(index % labelStep === 0 || index === points.length - 1) && (
                <text
                  x={index === 0 ? slotX + 6 : index === points.length - 1 ? slotX + Math.max(bandWidth - 6, 18) : centerX}
                  y={axisLabelY}
                  textAnchor={index === 0 ? "start" : index === points.length - 1 ? "end" : "middle"}
                  dominantBaseline="hanging"
                  className="chart-axis-label"
                >
                  {point.axisLabel}
                </text>
              )}
            </g>
          );
        })}
        <g className="trend-chart__navigator">
          <rect x={padding.left} y={navigatorTop} width={chartWidth} height={navigatorHeight} rx={8} className="trend-chart__navigator-bg" />
          {overviewPoints.length > 1 ? <path d={buildAreaPath(overviewPoints, navigatorTop + navigatorHeight)} className="trend-chart__navigator-area" /> : null}
          {navigatorLeftMaskWidth > 0 ? (
            <rect x={padding.left} y={navigatorTop} width={navigatorLeftMaskWidth} height={navigatorHeight} className="trend-chart__navigator-mask" />
          ) : null}
          {navigatorRightMaskWidth > 0 ? (
            <rect x={navigatorRightMaskX} y={navigatorTop} width={navigatorRightMaskWidth} height={navigatorHeight} className="trend-chart__navigator-mask" />
          ) : null}
          <rect
            x={padding.left}
            y={navigatorTop}
            width={chartWidth}
            height={navigatorHeight}
            className="trend-chart__navigator-hit"
            onClick={handleNavigatorJump}
          />
          <rect
            x={navigatorWindowX}
            y={navigatorTop + 1.5}
            width={Math.min(navigatorWindowWidth, chartWidth)}
            height={navigatorHeight - 3}
            rx={8}
            className="trend-chart__navigator-window"
            onMouseDown={(event) => {
              event.stopPropagation();
              beginDrag(event.clientX, "navigator-window");
            }}
          />
          <rect
            x={navigatorWindowX - 7}
            y={navigatorTop + 1.5}
            width={14}
            height={navigatorHeight - 3}
            className="trend-chart__navigator-handle-hit"
            onMouseDown={(event) => {
              event.stopPropagation();
              beginDrag(event.clientX, "navigator-left");
            }}
          />
          <rect
            x={navigatorWindowX + Math.min(navigatorWindowWidth, chartWidth) - 7}
            y={navigatorTop + 1.5}
            width={14}
            height={navigatorHeight - 3}
            className="trend-chart__navigator-handle-hit"
            onMouseDown={(event) => {
              event.stopPropagation();
              beginDrag(event.clientX, "navigator-right");
            }}
          />
          <line x1={navigatorWindowX + 10} y1={navigatorTop + 8} x2={navigatorWindowX + 10} y2={navigatorTop + navigatorHeight - 8} className="trend-chart__navigator-handle" />
          <line
            x1={navigatorWindowX + Math.min(navigatorWindowWidth, chartWidth) - 10}
            y1={navigatorTop + 8}
            x2={navigatorWindowX + Math.min(navigatorWindowWidth, chartWidth) - 10}
            y2={navigatorTop + navigatorHeight - 8}
            className="trend-chart__navigator-handle"
          />
          <text x={padding.left} y={navigatorLabelY} dominantBaseline="hanging" className="trend-chart__navigator-label" textAnchor="start">
            {formatDateOnly(fullRange.startTs)}
          </text>
          <text x={padding.left + chartWidth} y={navigatorLabelY} dominantBaseline="hanging" className="trend-chart__navigator-label" textAnchor="end">
            {formatDateOnly(fullRange.endTs)}
          </text>
        </g>
      </svg>
      {hoveredPoint ? (
        <div
          className="trend-chart__tooltip"
          style={
            {
              left: `${((padding.left + bandWidth * hoveredIndex! + bandWidth / 2) / width) * 100}%`,
              top: `${(tooltipY / height) * 100}%`,
            } as CSSProperties
          }
        >
          <div className="trend-chart__tooltip-title">{hoveredPoint.rangeLabel}</div>
          <div className="trend-chart__tooltip-row">
            <span>进账金额</span>
            <strong>{fmtMoney(hoveredPoint.inAmount)}</strong>
          </div>
          <div className="trend-chart__tooltip-row">
            <span>出账金额</span>
            <strong>{fmtMoney(hoveredPoint.outAmount)}</strong>
          </div>
          <div className="trend-chart__tooltip-row">
            <span>净额</span>
            <strong>{fmtSignedMoney(hoveredPoint.netAmount)}</strong>
          </div>
          <div className="trend-chart__tooltip-row">
            <span>进账笔数</span>
            <strong>{fmtCount(hoveredPoint.inCount)} 笔</strong>
          </div>
          <div className="trend-chart__tooltip-row">
            <span>出账笔数</span>
            <strong>{fmtCount(hoveredPoint.outCount)} 笔</strong>
          </div>
          {hoveredPoint.sourceBucketCount > 1 ? (
            <div className="trend-chart__tooltip-row">
              <span>聚合桶数</span>
              <strong>{fmtCount(hoveredPoint.sourceBucketCount)} 个</strong>
            </div>
          ) : null}
          {hoveredHasClippedValue ? (
            <div className="trend-chart__tooltip-row">
              <span>显示上限</span>
              <strong>{formatTrendMetric(metricMode, axisMax)}</strong>
            </div>
          ) : null}
          <div className="trend-chart__tooltip-row trend-chart__tooltip-row--emphasis">
            <span>{metricMode === "count" ? "总活跃度" : "总金额"}</span>
            <strong>{formatTrendMetric(metricMode, getTrendPointValue(hoveredPoint, metricMode, "total"))}</strong>
          </div>
        </div>
      ) : null}
    </div>
  );
}

export function TrendWorkspace({
  points,
  granularity,
  metricMode,
  activeToken,
  loading,
  empty,
  emptyTitle,
  emptyDescription,
  emptyVariant,
  canResetTrendView,
  onSelect,
  onResetView,
  onGranularityChange,
  dateStart,
  dateEnd,
}: {
  points: Array<Record<string, unknown>>;
  granularity: ChartGranularity;
  metricMode: ChartMetricMode;
  activeToken: ChartFilterToken | null;
  loading: boolean;
  empty: boolean;
  emptyTitle: string;
  emptyDescription: string;
  emptyVariant: "selection" | "filtered";
  canResetTrendView: boolean;
  onSelect: (token: ChartFilterToken) => void;
  onResetView: () => void;
  onGranularityChange: (granularity: ChartGranularity) => void;
  dateStart: string;
  dateEnd: string;
}): JSX.Element {
  const onGranularityChangeRef = useRef(onGranularityChange);
  const normalizedPoints = useMemo(() => normalizeTrendPoints(points, granularity), [granularity, points]);
  const fallbackRange = useMemo<TrendViewport | null>(() => {
    const startTs = parseDateLike(dateStart);
    const endTs = parseDateLike(dateEnd);
    if (startTs == null || endTs == null) {
      return null;
    }
    return { startTs, endTs: endTs + DAY_MS - 1 };
  }, [dateEnd, dateStart]);
  const fullRange = useMemo<TrendViewport | null>(() => {
    if (normalizedPoints.length) {
      return {
        startTs: normalizedPoints[0].startTs,
        endTs: normalizedPoints[normalizedPoints.length - 1].endTs,
      };
    }
    return fallbackRange;
  }, [fallbackRange, normalizedPoints]);
  const [viewport, setViewport] = useState<TrendViewport | null>(fullRange);
  const [seriesVisibility, setSeriesVisibility] = useState({ in: true, out: true });
  const [displayGranularity, setDisplayGranularity] = useState<TrendDisplayGranularity>("day");

  useEffect(() => {
    onGranularityChangeRef.current = onGranularityChange;
  }, [onGranularityChange]);

  useEffect(() => {
    setSeriesVisibility({ in: true, out: true });
  }, [dateEnd, dateStart]);

  useEffect(() => {
    if (!fullRange) {
      setViewport(null);
      return;
    }
    setViewport((previous) => {
      if (!previous) {
        return fullRange;
      }
      const nextViewport = clampTrendViewport(previous, fullRange, getTrendMinimumViewportDuration(granularity));
      if (Math.abs(nextViewport.startTs - previous.startTs) < 1000 && Math.abs(nextViewport.endTs - previous.endTs) < 1000) {
        return previous;
      }
      return nextViewport;
    });
  }, [fullRange, granularity]);

  const resolvedViewport = useMemo(
    () => (viewport && fullRange ? clampTrendViewport(viewport, fullRange, getTrendMinimumViewportDuration(granularity)) : fullRange),
    [fullRange, granularity, viewport]
  );
  const viewportDuration = resolvedViewport ? Math.max(1, resolvedViewport.endTs - resolvedViewport.startTs) : 0;
  useEffect(() => {
    if (!resolvedViewport) {
      setDisplayGranularity("day");
      return;
    }
    setDisplayGranularity((previous) => getTrendDisplayGranularityWithHysteresis(viewportDuration, granularity, previous));
  }, [displayGranularity, granularity, resolvedViewport, viewportDuration]);

  const displayPoints = useMemo(
    () => (resolvedViewport ? aggregateTrendDisplayPoints(normalizedPoints, resolvedViewport, displayGranularity) : []),
    [displayGranularity, normalizedPoints, resolvedViewport]
  );
  const recommendedFetchGranularity = resolvedViewport ? getRecommendedTrendFetchGranularity(viewportDuration, granularity) : granularity;
  const fullRangeRecommendedGranularity = fullRange ? getRecommendedTrendFetchGranularityBase(Math.max(1, fullRange.endTs - fullRange.startTs)) : granularity;
  const isFullViewport =
    resolvedViewport && fullRange
      ? Math.abs(resolvedViewport.startTs - fullRange.startTs) < 1000 && Math.abs(resolvedViewport.endTs - fullRange.endTs) < 1000
      : true;
  const hasSeriesMuted = !seriesVisibility.in || !seriesVisibility.out;
  const canHardReset = !empty && (canResetTrendView || !isFullViewport || hasSeriesMuted);

  useEffect(() => {
    if (!resolvedViewport || !normalizedPoints.length || granularity === recommendedFetchGranularity) {
      return;
    }
    const timer = window.setTimeout(() => {
      onGranularityChangeRef.current(recommendedFetchGranularity);
    }, 420);
    return () => window.clearTimeout(timer);
  }, [granularity, normalizedPoints.length, recommendedFetchGranularity, resolvedViewport]);

  const handleFitView = (): void => {
    if (!fullRange) {
      return;
    }
    setViewport(fullRange);
    if (granularity !== fullRangeRecommendedGranularity) {
      onGranularityChange(fullRangeRecommendedGranularity);
    }
  };

  const handleReset = (): void => {
    if (fullRange) {
      setViewport(fullRange);
    }
    setSeriesVisibility({ in: true, out: true });
    onResetView();
  };

  const toggleSeries = (field: "in" | "out"): void => {
    setSeriesVisibility((previous) => {
      if (!previous[field] && (field === "in" ? previous.out : previous.in)) {
        return { ...previous, [field]: true };
      }
      if (previous[field] && !(field === "in" ? previous.out : previous.in)) {
        return previous;
      }
      return { ...previous, [field]: !previous[field] };
    });
  };

  return (
    <ChartPanel
      panelId="trend"
      title="账户进/出账时段分布"
      loading={loading}
      empty={empty}
      className="chart-panel--span-full chart-panel--primary chart-panel--trend-expanded chart-panel--trend-workzone"
      headerMode="primary"
      actionMode="full"
      emptyTitle={emptyTitle}
      emptyDescription={emptyDescription}
      emptyVariant={emptyVariant}
      headerControls={
        <div className="chart-trend-status">
          <div className="chart-trend-status__legends" aria-label="图例">
            <button
              type="button"
              className={`chart-trend-status__legend ${seriesVisibility.in ? "is-active" : "is-muted"}`}
              onClick={() => toggleSeries("in")}
              aria-pressed={seriesVisibility.in}
            >
              <span className="chart-trend-status__dot chart-trend-status__dot--in" aria-hidden="true" />
              进账
            </button>
            <button
              type="button"
              className={`chart-trend-status__legend ${seriesVisibility.out ? "is-active" : "is-muted"}`}
              onClick={() => toggleSeries("out")}
              aria-pressed={seriesVisibility.out}
            >
              <span className="chart-trend-status__dot chart-trend-status__dot--out" aria-hidden="true" />
              出账
            </button>
          </div>
          <button type="button" className="chart-trend-status__reset" onClick={handleReset} disabled={!canHardReset}>
            重置
          </button>
        </div>
      }
    >
      {resolvedViewport ? (
        <TrendInteractiveChart
          points={displayPoints}
          displayGranularity={displayGranularity}
          rawGranularity={granularity}
          metricMode={metricMode}
          viewport={resolvedViewport}
          fullRange={fullRange || resolvedViewport}
          seriesVisibility={seriesVisibility}
          activeToken={activeToken}
          onViewportChange={setViewport}
          onSelect={onSelect}
          onResetView={handleFitView}
        />
      ) : null}
    </ChartPanel>
  );
}
