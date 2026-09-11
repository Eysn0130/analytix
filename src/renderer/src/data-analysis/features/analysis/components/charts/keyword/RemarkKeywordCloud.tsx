import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";

import type { ChartFilterToken } from "../../../api/stats-api";

import { formatCaseFactCount } from "../../../model/stats-case-fact-number-model";
import {
  KEYWORD_CLOUD_HEIGHT,
  KEYWORD_CLOUD_MIN_HEIGHT,
  KEYWORD_CLOUD_MIN_WIDTH,
  KEYWORD_CLOUD_WIDTH,
  buildKeywordCloudLayout,
  getKeywordCloudLayoutBounds,
} from "../../../model/stats-keyword-cloud-layout-model";
import { prepareRemarkKeywordCloudItems } from "../../../model/stats-keyword-cloud-model";

function sameToken(a: ChartFilterToken, b: ChartFilterToken): boolean {
  return (
    a.source_panel_id === b.source_panel_id &&
    a.dimension === b.dimension &&
    a.value === b.value &&
    JSON.stringify(a.payload || {}) === JSON.stringify(b.payload || {})
  );
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function buildRemarkKeywordToken(panelId: string, label: string): ChartFilterToken {
  return {
    source_panel_id: panelId,
    dimension: "remark_keyword",
    value: label,
    label,
    payload: { field: "remark" },
  };
}

export function RemarkKeywordCloud({
  items,
  panelId,
  activeToken,
  onSelect,
}: {
  items: Array<{ label: string; count: number }>;
  panelId: string;
  activeToken: ChartFilterToken | null;
  onSelect: (token: ChartFilterToken) => void;
}): JSX.Element {
  const canvasRef = useRef<HTMLDivElement | null>(null);
  const [size, setSize] = useState(() => ({ width: KEYWORD_CLOUD_WIDTH, height: KEYWORD_CLOUD_HEIGHT }));
  const visibleItems = useMemo(
    () =>
      prepareRemarkKeywordCloudItems(items)
        .sort((left, right) => right.count - left.count || left.label.localeCompare(right.label, "zh-CN")),
    [items]
  );
  useEffect(() => {
    const element = canvasRef.current;
    if (!element) {
      return;
    }
    const updateSize = (): void => {
      const rect = element.getBoundingClientRect();
      const nextWidth = Math.max(KEYWORD_CLOUD_MIN_WIDTH, Math.round(rect.width || element.clientWidth || KEYWORD_CLOUD_WIDTH));
      const nextHeight = Math.max(KEYWORD_CLOUD_MIN_HEIGHT, Math.round(rect.height || element.clientHeight || KEYWORD_CLOUD_HEIGHT));
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
  const layout = useMemo(() => buildKeywordCloudLayout(visibleItems, size.width, size.height), [size.height, size.width, visibleItems]);
  const layoutBounds = useMemo(() => getKeywordCloudLayoutBounds(layout), [layout]);
  const stageStyle = useMemo(() => {
    const viewportWidth = Math.max(size.width, KEYWORD_CLOUD_MIN_WIDTH);
    const viewportHeight = Math.max(size.height, KEYWORD_CLOUD_MIN_HEIGHT);
    const contentWidth = Math.max(1, layoutBounds.right - layoutBounds.left);
    const contentHeight = Math.max(1, layoutBounds.bottom - layoutBounds.top);
    const maxOffsetX = Math.max(2, viewportWidth - contentWidth - 2);
    const maxOffsetY = Math.max(2, viewportHeight - contentHeight - 2);
    const offsetX = clamp((viewportWidth - contentWidth) / 2, 2, maxOffsetX);
    const offsetY = clamp((viewportHeight - contentHeight) / 2, 2, maxOffsetY);
    return {
      width: `${contentWidth}px`,
      height: `${contentHeight}px`,
      transform: `translate(${Math.round(offsetX)}px, ${Math.round(offsetY)}px)`,
    } as CSSProperties;
  }, [layoutBounds.bottom, layoutBounds.left, layoutBounds.right, layoutBounds.top, size.height, size.width]);

  return (
    <div className="keyword-cloud">
      <div ref={canvasRef} className="keyword-cloud__canvas" aria-label="备注关键词云">
        <div className="keyword-cloud__stage" style={stageStyle}>
          {layout.map((item) => {
            const token = buildRemarkKeywordToken(panelId, item.label);
            const active = activeToken ? sameToken(activeToken, token) : false;
            return (
              <button
                key={`${panelId}-${item.label}`}
                type="button"
                className={`keyword-cloud__word ${active ? "is-active" : ""}`}
                data-tier={item.tier}
                style={
                  {
                    left: `${item.x - layoutBounds.left}px`,
                    top: `${item.y - layoutBounds.top}px`,
                    fontSize: `${item.fontSize}px`,
                    color: item.color,
                    opacity: item.opacity,
                    ["--keyword-hover-color" as string]: item.hoverColor,
                    ["--keyword-hover-scale" as string]: String(item.hoverScale),
                  } as CSSProperties
                }
                aria-label={`${item.label}，命中 ${formatCaseFactCount(item.count)} 次`}
                title={`${item.label} · 命中 ${formatCaseFactCount(item.count)} 次`}
                onClick={() => onSelect(token)}
              >
                {item.label}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
