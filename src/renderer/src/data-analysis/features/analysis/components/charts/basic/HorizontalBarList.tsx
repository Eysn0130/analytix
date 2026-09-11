import type { CSSProperties } from "react";

import type { ChartFilterToken } from "../../../api/stats-api";

import { sameToken, type NumericChartItem } from "./chart-render-utils";

interface HorizontalBarListProps {
  items: NumericChartItem[];
  panelId: string;
  dimension: string;
  activeToken: ChartFilterToken | null;
  onSelect: (token: ChartFilterToken) => void;
  variant?: "default" | "compact";
  showRank?: boolean;
  labelColumn?: string;
  subtitleFormatter?: (item: NumericChartItem) => string[];
}

export function HorizontalBarList({
  items,
  panelId,
  dimension,
  activeToken,
  onSelect,
  variant = "default",
  showRank = false,
  labelColumn = "对象",
  subtitleFormatter,
}: HorizontalBarListProps): JSX.Element {
  const listStyle = {
    ["--rank-bars-columns" as string]: [showRank ? "42px" : null, "minmax(0, 1fr)"].filter(Boolean).join(" "),
  } as CSSProperties;

  return (
    <div className={`rank-bars ${variant === "compact" ? "rank-bars--compact" : ""}`} style={listStyle}>
      <div className="rank-bars__columns" aria-hidden="true">
        {showRank ? <span className="rank-bars__columns-cell rank-bars__columns-cell--rank">序</span> : null}
        <span className="rank-bars__columns-cell">{labelColumn}</span>
      </div>
      <div className="rank-bars__rows">
        {items.map((item, index) => {
          const token: ChartFilterToken = {
            source_panel_id: panelId,
            dimension,
            value: item.rawValue || item.label,
            label: item.label,
            payload: item.payload || {},
          };
          const active = activeToken ? sameToken(activeToken, token) : false;
          const subtitleParts = subtitleFormatter ? subtitleFormatter(item).filter(Boolean) : [item.detail, item.secondaryValue].filter(Boolean);
          const labelClassName = dimension === "ip_addr" || dimension === "txn_id" ? "rank-bars__label rank-bars__label--code" : "rank-bars__label";
          return (
            <button
              key={`${panelId}-${item.label}-${item.rawValue || ""}`}
              type="button"
              className={`rank-bars__item ${active ? "is-active" : ""}`}
              onClick={() => onSelect(token)}
              title={[item.label, ...subtitleParts].filter(Boolean).join(" | ")}
            >
              {showRank ? <span className="rank-bars__rank">{String(index + 1).padStart(2, "0")}</span> : null}
              <span className="rank-bars__main">
                <span className={labelClassName}>{item.label}</span>
                {subtitleParts.length ? (
                  <span className="rank-bars__subline">
                    {subtitleParts.map((part, partIndex) => (
                      <span key={`${item.label}-${partIndex}-${part}`} className="rank-bars__subline-part">
                        {part}
                      </span>
                    ))}
                  </span>
                ) : null}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
