import { ReactNode, UIEvent, useEffect, useMemo, useState } from "react";

export interface VirtualizedColumn<RowT> {
  id: string;
  header: string;
  width?: number;
  align?: "left" | "right" | "center";
  renderCell: (row: RowT, rowIndex: number) => ReactNode;
}

interface VirtualizedTableProps<RowT> {
  rows: RowT[];
  columns: Array<VirtualizedColumn<RowT>>;
  rowHeight?: number;
  height?: number;
  overscan?: number;
  rowKey: (row: RowT, rowIndex: number) => string;
  emptyText?: string;
  className?: string;
  onWindowStats?: (stats: { totalRows: number; renderedRows: number; start: number; end: number }) => void;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

export function VirtualizedTable<RowT>({
  rows,
  columns,
  rowHeight = 36,
  height = 420,
  overscan = 8,
  rowKey,
  emptyText = "暂无数据",
  className = "",
  onWindowStats
}: VirtualizedTableProps<RowT>): JSX.Element {
  const [scrollTop, setScrollTop] = useState(0);

  const totalRows = rows.length;
  const totalHeight = totalRows * rowHeight;
  const viewportRows = Math.ceil(height / rowHeight);

  const startIndex = clamp(Math.floor(scrollTop / rowHeight) - overscan, 0, Math.max(0, totalRows - 1));
  const endIndex = clamp(startIndex + viewportRows + overscan * 2, 0, totalRows);

  const visible = useMemo(() => rows.slice(startIndex, endIndex), [rows, startIndex, endIndex]);

  useEffect(() => {
    if (!onWindowStats) {
      return;
    }
    onWindowStats({
      totalRows,
      renderedRows: visible.length,
      start: startIndex,
      end: endIndex
    });
  }, [onWindowStats, totalRows, visible.length, startIndex, endIndex]);

  const templateColumns = columns.map((col) => (col.width ? `${col.width}px` : "minmax(120px, 1fr)")).join(" ");

  const onScroll = (event: UIEvent<HTMLDivElement>) => {
    setScrollTop(event.currentTarget.scrollTop);
  };

  return (
    <div className={`vt-root ${className}`.trim()}>
      <div className="vt-head" style={{ gridTemplateColumns: templateColumns }}>
        {columns.map((col) => (
          <div key={col.id} className={`vt-cell vt-head-cell align-${col.align || "left"}`}>
            {col.header}
          </div>
        ))}
      </div>

      {rows.length === 0 ? <div className="vt-empty">{emptyText}</div> : null}

      {rows.length > 0 ? (
        <div className="vt-body" style={{ height }} onScroll={onScroll}>
          <div className="vt-spacer" style={{ height: totalHeight }}>
            <div className="vt-window" style={{ transform: `translateY(${startIndex * rowHeight}px)` }}>
              {visible.map((row, i) => {
                const rowIndex = startIndex + i;
                return (
                  <div
                    key={rowKey(row, rowIndex)}
                    className="vt-row"
                    style={{ height: rowHeight, gridTemplateColumns: templateColumns }}
                  >
                    {columns.map((col) => (
                      <div key={col.id} className={`vt-cell align-${col.align || "left"}`}>
                        {col.renderCell(row, rowIndex)}
                      </div>
                    ))}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
