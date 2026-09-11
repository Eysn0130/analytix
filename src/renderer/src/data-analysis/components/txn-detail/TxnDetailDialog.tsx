import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent, ReactNode, Ref, UIEventHandler, WheelEventHandler } from "react";
import { PersistentHorizontalScrollbar } from "../scrollbar/PersistentHorizontalScrollbar";
import "../workbench-ui/workbench-ui-kit.css";
import "./txn-detail-dialog.css";

const MIN_COLUMN_WIDTH = 64;
const MAX_COLUMN_WIDTH = 480;

export interface TxnDetailDialogColumn {
  key: string;
  title: string;
  width: number;
  sortable?: boolean;
  sorted?: boolean;
  sortDir?: "asc" | "desc";
  resizable?: boolean;
}

export interface TxnDetailDialogCell {
  text: string;
  mono?: boolean;
  tone?: string;
  align?: "left" | "right" | "center" | string;
  kind?: "text" | "money" | "direction";
  title?: string;
}

export interface TxnDetailDialogRow {
  key: string;
  className?: string;
  cells: Record<string, TxnDetailDialogCell | undefined>;
}

interface TxnDetailDialogProps {
  open: boolean;
  title: string;
  ariaLabel?: string;
  meta?: string;
  loading?: boolean;
  emptyText?: string;
  columns: TxnDetailDialogColumn[];
  rows: TxnDetailDialogRow[];
  onClose: () => void;
  onToggleSort?: (columnKey: string) => void;
  onResizeColumn?: (columnKey: string, width: number) => void;
  tableWrapRef?: Ref<HTMLDivElement>;
  onTableScroll?: UIEventHandler<HTMLDivElement>;
  onTableWheel?: WheelEventHandler<HTMLDivElement>;
  extraToolbarActions?: ReactNode;
  footerContent?: ReactNode;
  wrapClassName?: string;
  tableWrapClassName?: string;
  topSpacerHeight?: number;
  bottomSpacerHeight?: number;
  skeletonRowCount?: number;
}

function clampColumnWidth(value: number, fallback: number): number {
  if (!Number.isFinite(value)) {
    return fallback;
  }
  return Math.max(MIN_COLUMN_WIDTH, Math.min(MAX_COLUMN_WIDTH, Math.floor(value)));
}

function buildWidthMap(columns: TxnDetailDialogColumn[]): Record<string, number> {
  return columns.reduce<Record<string, number>>((accumulator, column) => {
    accumulator[column.key] = clampColumnWidth(column.width, 120);
    return accumulator;
  }, {});
}

function stopEventPropagation(event: { stopPropagation: () => void }): void {
  event.stopPropagation();
}

function assignRef<T>(ref: Ref<T> | undefined, value: T | null): void {
  if (!ref) {
    return;
  }
  if (typeof ref === "function") {
    ref(value);
    return;
  }
  (ref as { current: T | null }).current = value;
}

function getTxnCellToneClass(tone: string | undefined): string {
  if (tone === "in") {
    return "txnToneIn";
  }
  if (tone === "out") {
    return "txnToneOut";
  }
  if (tone === "good") {
    return "txnToneGood";
  }
  if (tone === "danger") {
    return "txnToneDanger";
  }
  return "";
}

function buildTxnCellClassName(cell: TxnDetailDialogCell | undefined, isMoney: boolean): string | undefined {
  const className = [
    isMoney ? "txnAmt" : "",
    cell?.mono ? "txnMono" : "",
    getTxnCellToneClass(cell?.tone)
  ]
    .filter(Boolean)
    .join(" ");
  return className || undefined;
}

const TxnDetailDialogBodyRow = memo(function TxnDetailDialogBodyRow({
  row,
  columns,
}: {
  row: TxnDetailDialogRow;
  columns: TxnDetailDialogColumn[];
}): JSX.Element {
  return (
    <tr className={row.className}>
      {columns.map((column) => {
        const cell = row.cells[column.key];
        const text = cell ? cell.text : "-";
        const isDirection = cell?.kind === "direction";
        const isMoney = cell?.kind === "money" || cell?.align === "right";
        if (isDirection) {
          return (
            <td key={`${row.key}-${column.key}`}>
              <span className={`txnDir ${cell?.tone || ""}`.trim()}>{text}</span>
            </td>
          );
        }
        return (
          <td
            key={`${row.key}-${column.key}`}
            className={buildTxnCellClassName(cell, isMoney)}
            title={cell?.title ?? text}
          >
            {text}
          </td>
        );
      })}
    </tr>
  );
});

export function TxnDetailDialog({
  open,
  title,
  ariaLabel,
  meta,
  loading = false,
  emptyText = "暂无交易明细",
  columns,
  rows,
  onClose,
  onToggleSort,
  onResizeColumn,
  tableWrapRef,
  onTableScroll,
  onTableWheel,
  extraToolbarActions,
  footerContent,
  wrapClassName = "",
  tableWrapClassName = "",
  topSpacerHeight = 0,
  bottomSpacerHeight = 0,
  skeletonRowCount = 10
}: TxnDetailDialogProps): JSX.Element {
  const localTableWrapRef = useRef<HTMLDivElement | null>(null);
  const setTableWrapRef = useCallback((node: HTMLDivElement | null): void => {
    localTableWrapRef.current = node;
    assignRef(tableWrapRef, node);
  }, [tableWrapRef]);
  const [resizedColumnWidths, setResizedColumnWidths] = useState<Record<string, number>>(() => buildWidthMap(columns));
  const tracksColumnWidths = Boolean(onResizeColumn);

  useEffect(() => {
    if (!tracksColumnWidths) {
      return;
    }
    const nextWidths = buildWidthMap(columns);
    setResizedColumnWidths((current) => {
      const currentKeys = Object.keys(current);
      const nextKeys = Object.keys(nextWidths);
      if (
        currentKeys.length === nextKeys.length &&
        nextKeys.every((key) => Number(current[key] || 0) === Number(nextWidths[key] || 0))
      ) {
        return current;
      }
      return nextWidths;
    });
  }, [columns, open, tracksColumnWidths]);

  const columnWidths = useMemo(
    () => (tracksColumnWidths ? resizedColumnWidths : buildWidthMap(columns)),
    [columns, resizedColumnWidths, tracksColumnWidths]
  );

  const totalTableWidth = useMemo(
    () => columns.reduce((sum, column) => sum + Number(columnWidths[column.key] ?? column.width ?? 0), 0),
    [columnWidths, columns]
  );
  const tableBodyColSpan = Math.max(1, columns.length);

  const loadingSkeleton = loading && rows.length === 0;

  const beginResize = (column: TxnDetailDialogColumn, event: ReactMouseEvent<HTMLDivElement>): void => {
    if (!onResizeColumn || !column.resizable) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    const startWidth = Number(columnWidths[column.key] ?? column.width) || column.width;
    const startX = event.clientX;
    let pendingWidth = startWidth;

    const handleMove = (moveEvent: MouseEvent): void => {
      pendingWidth = clampColumnWidth(startWidth + moveEvent.clientX - startX, startWidth);
      setResizedColumnWidths((current) => ({
        ...current,
        [column.key]: pendingWidth
      }));
    };

    const handleUp = (): void => {
      document.removeEventListener("mousemove", handleMove);
      document.removeEventListener("mouseup", handleUp);
      onResizeColumn(column.key, pendingWidth);
    };

    document.addEventListener("mousemove", handleMove);
    document.addEventListener("mouseup", handleUp);
  };

  return (
    <div className={`modalWrap txnDetailDialogWrap ${open ? "open" : ""} ${wrapClassName}`.trim()} aria-hidden={!open}>
      <div className="modalBackdrop txnDetailDialogBackdrop txnModalBackdrop" aria-hidden="true" onClick={onClose} />
      <div
        className="modal txnDetailDialogModal txnModal"
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel || title || "交易明细"}
        onMouseDown={stopEventPropagation}
        onClick={stopEventPropagation}
        onWheel={stopEventPropagation}
      >
        <section className="workbench-ui-table-shell txnModalShell">
          <div className="workbench-ui-table-toolbar tableToolbar txnModalToolbar">
            <div className="txnModalToolbarMain">
              <div className="txnModalToolbarTitle modalTitle">{title}</div>
              {meta ? <div className="txnModalToolbarMeta">{meta}</div> : null}
            </div>
            <div className="workbench-ui-table-actions txnModalToolbarActions">
              {extraToolbarActions}
              <button
                className="workbench-ui-btn workbench-ui-btn--ghost workbench-ui-btn--md ghost chipBtn navStyle ghost"
                type="button"
                onClick={onClose}
              >
                <span className="workbench-ui-btn__content">关闭</span>
              </button>
            </div>
          </div>

          <div className="modalBody txnModalBody">
            <div className="txnTableViewport">
            <div
              className={`txnTableWrap ${tableWrapClassName}`.trim()}
              ref={setTableWrapRef}
              onScroll={onTableScroll}
              onWheel={(event) => {
                event.stopPropagation();
                onTableWheel?.(event);
              }}
            >
              <div className="txnInner">
                <table className="txnTable" style={{ width: `${Math.max(totalTableWidth, 0)}px` }}>
                  <colgroup>
                    {columns.map((column) => (
                      <col key={column.key} data-col={column.key} style={{ width: `${columnWidths[column.key] ?? column.width}px` }} />
                    ))}
                  </colgroup>
                  <thead>
                    <tr>
                      {columns.map((column) => {
                        const activeSort = !!column.sortable && !!column.sorted;
                        return (
                          <th
                            key={column.key}
                            data-col={column.key}
                            className={`${column.sortable ? "sortable" : ""} ${activeSort ? "sorted" : ""}`.trim()}
                            style={{ width: `${columnWidths[column.key] ?? column.width}px` }}
                            title={column.title}
                            onClick={() => {
                              if (!column.sortable) {
                                return;
                              }
                              onToggleSort?.(column.key);
                            }}
                          >
                            <span className="txnThLabel">
                              <span className="txnThText">{column.title}</span>
                              {column.sortable ? (
                                <span className={`txnSortIcon ${activeSort ? "active" : ""}`.trim()} aria-hidden="true">
                                  {activeSort ? (column.sortDir === "desc" ? "↓" : "↑") : "↕"}
                                </span>
                              ) : null}
                            </span>
                            {column.resizable && onResizeColumn ? (
                              <div
                                className="resizer"
                                data-col={column.key}
                                onMouseDown={(event) => beginResize(column, event)}
                                onClick={stopEventPropagation}
                              />
                            ) : null}
                          </th>
                        );
                      })}
                    </tr>
                  </thead>
                  <tbody>
                    {loadingSkeleton
                      ? Array.from({ length: skeletonRowCount }).map((_, rowIndex) => (
                          <tr key={`sk-${rowIndex}`} className="txnSk">
                            {columns.map((column) => (
                              <td key={`${column.key}-${rowIndex}`}>
                                <div className="txnSkCell" />
                              </td>
                            ))}
                          </tr>
                        ))
                      : null}
                    {!loadingSkeleton && topSpacerHeight > 0 ? (
                      <tr className="txnSpacer">
                        <td colSpan={tableBodyColSpan} style={{ height: `${topSpacerHeight}px` }} />
                      </tr>
                    ) : null}
                    {!loadingSkeleton
                      ? rows.map((row) => (
                          <TxnDetailDialogBodyRow
                            key={row.key}
                            row={row}
                            columns={columns}
                          />
                        ))
                      : null}
                    {!loadingSkeleton && bottomSpacerHeight > 0 ? (
                      <tr className="txnSpacer">
                        <td colSpan={tableBodyColSpan} style={{ height: `${bottomSpacerHeight}px` }} />
                      </tr>
                    ) : null}
                    {!loadingSkeleton && !rows.length ? (
                      <tr>
                        <td colSpan={tableBodyColSpan} className="txnEmpty">
                          {emptyText}
                        </td>
                      </tr>
                    ) : null}
                  </tbody>
                </table>
              </div>
            </div>
              <PersistentHorizontalScrollbar
                className="txnPersistentHorizontalScrollbar"
                scrollRef={localTableWrapRef}
                ariaLabel="交易详情横向滚动条"
              />
            </div>
          </div>

          {footerContent ? <div className="workbench-ui-table-footer tableFooter txnModalFooter">{footerContent}</div> : null}
        </section>
      </div>
    </div>
  );
}
