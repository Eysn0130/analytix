import { MouseEvent as ReactMouseEvent, WheelEvent as ReactWheelEvent, useLayoutEffect, useRef, useState } from "react";
import type { CSSProperties, ReactNode, ReactPortal } from "react";
import { createPortal } from "react-dom";
import type { FlowAnchorRect, FlowShellState } from "../runtime/flow-runtime";

export function stopEventPropagation(event: Event | ReactMouseEvent): void {
  event.stopPropagation();
}

type FloatingPanelAnchorAlign = "start" | "center";

function buildAnchoredPanelPosition(
  anchorRect: FlowAnchorRect,
  panelRect: DOMRect,
  align: FloatingPanelAnchorAlign
): { left: number; top: number } {
  let left =
    align === "center"
      ? anchorRect.left + anchorRect.width / 2 - panelRect.width / 2
      : anchorRect.left;
  let top = anchorRect.bottom + 6;
  if (left + panelRect.width > window.innerWidth - 8) {
    left = window.innerWidth - panelRect.width - 8;
  }
  if (left < 8) {
    left = 8;
  }
  if (top + panelRect.height > window.innerHeight - 8) {
    top = anchorRect.top - panelRect.height - 6;
  }
  if (top < 8) {
    top = 8;
  }
  return { left: Math.round(left), top: Math.round(top) };
}

function buildContextMenuPosition(x: number, y: number, panelRect: DOMRect): { left: number; top: number } {
  const left = Math.max(10, Math.min(x, window.innerWidth - panelRect.width - 10));
  const top = Math.max(10, Math.min(y, window.innerHeight - panelRect.height - 10));
  return { left: Math.round(left), top: Math.round(top) };
}

interface FloatingPanelProps {
  mount: HTMLElement | null;
  open: boolean;
  className: string;
  anchorRect?: FlowAnchorRect | null;
  anchorAlign?: FloatingPanelAnchorAlign;
  point?: { x: number; y: number } | null;
  panelStyle?: CSSProperties;
  ariaHidden?: boolean;
  role?: string;
  children: ReactNode;
}

export function FloatingPanel({
  mount,
  open,
  className,
  anchorRect = null,
  anchorAlign = "start",
  point = null,
  panelStyle,
  ariaHidden,
  role,
  children
}: FloatingPanelProps): ReactPortal | null {
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [position, setPosition] = useState<{ left: number; top: number; visible: boolean }>({
    left: -9999,
    top: -9999,
    visible: false
  });

  useLayoutEffect(() => {
    if (!open) {
      setPosition({ left: -9999, top: -9999, visible: false });
      return;
    }
    const panel = panelRef.current;
    if (!panel) {
      return;
    }
    const panelRect = panel.getBoundingClientRect();
    if (anchorRect) {
      const next = buildAnchoredPanelPosition(anchorRect, panelRect, anchorAlign);
      setPosition({ ...next, visible: true });
      return;
    }
    if (point) {
      const next = buildContextMenuPosition(point.x, point.y, panelRect);
      setPosition({ ...next, visible: true });
      return;
    }
    setPosition({ left: -9999, top: -9999, visible: false });
  }, [
    anchorRect,
    anchorRect?.bottom,
    anchorRect?.height,
    anchorRect?.left,
    anchorRect?.right,
    anchorRect?.top,
    anchorRect?.width,
    anchorAlign,
    open,
    point,
    point?.x,
    point?.y
  ]);

  if (!mount || !open) {
    return null;
  }

  return createPortal(
    <div
      ref={panelRef}
      className={className}
      role={role}
      aria-hidden={ariaHidden}
      style={{
        ...panelStyle,
        left: `${position.left}px`,
        top: `${position.top}px`,
        visibility: position.visible ? "visible" : "hidden"
      }}
      onMouseDown={stopEventPropagation}
      onClick={stopEventPropagation}
      onWheel={stopEventPropagation}
      onContextMenu={(event) => {
        event.preventDefault();
        event.stopPropagation();
      }}
    >
      {children}
    </div>,
    mount
  );
}

type EdgeTxnOverlayData =
  | FlowShellState["overlay"]["edgeInfo"]
  | FlowShellState["overlay"]["edgeLabel"]["readonly"];

function normalizeWheelDelta(event: ReactWheelEvent): { dx: number; dy: number } {
  let dx = Number(event.deltaX) || 0;
  let dy = Number(event.deltaY) || 0;
  if (event.deltaMode === 1) {
    dx *= 16;
    dy *= 16;
  } else if (event.deltaMode === 2) {
    dx *= window.innerWidth || 1;
    dy *= window.innerHeight || 1;
  }
  return { dx, dy };
}

function routeEdgeTxnWheel(event: ReactWheelEvent<HTMLDivElement>): void {
  const list = event.currentTarget.querySelector<HTMLElement>(".edgeTxnList");
  if (!list) {
    event.stopPropagation();
    return;
  }

  const { dx, dy } = normalizeWheelDelta(event);
  const maxTop = Math.max(0, list.scrollHeight - list.clientHeight);
  const maxLeft = Math.max(0, list.scrollWidth - list.clientWidth);
  const hasY = maxTop > 0;
  const hasX = maxLeft > 0;
  if ((!dx && !dy) || (!hasY && !hasX)) {
    event.stopPropagation();
    return;
  }

  const prevTop = list.scrollTop;
  const prevLeft = list.scrollLeft;
  let nextTop = prevTop;
  let nextLeft = prevLeft;

  if (Math.abs(dx) > 0.01 && hasX) {
    nextLeft += dx;
  }
  if (Math.abs(dy) > 0.01) {
    if (event.shiftKey && hasX) {
      nextLeft += dy;
    } else if (hasY) {
      nextTop += dy;
    } else if (hasX) {
      nextLeft += dy;
    }
  }

  nextTop = Math.max(0, Math.min(maxTop, nextTop));
  nextLeft = Math.max(0, Math.min(maxLeft, nextLeft));
  if (nextTop !== prevTop || nextLeft !== prevLeft) {
    list.scrollTop = nextTop;
    list.scrollLeft = nextLeft;
  }
  event.preventDefault();
  event.stopPropagation();
}

export function EdgeTxnReadonlyPanel({
  data,
  onOpenDetail
}: {
  data: EdgeTxnOverlayData;
  onOpenDetail?: (() => void) | null;
}): JSX.Element {
  const amountClass = data.amountTone ? ` edgeTxnMetricValue--${data.amountTone}` : "";

  return (
    <div className="edgeTxnReadonly" onWheel={routeEdgeTxnWheel}>
      <div className="edgeTxnHeader">
        <div className="edgeTxnCategory">{data.title || "资金交易"}</div>
        <div className="edgeTxnSummary">
          <div className="edgeTxnMetric">
            <div className="edgeTxnMetricLabel">金额合计</div>
            <div className={`edgeTxnMetricValue${amountClass}`}>{data.amountText}</div>
          </div>
          <div className="edgeTxnMetric">
            <div className="edgeTxnMetricLabel">次数</div>
            {data.countClickable && onOpenDetail ? (
              <button className="edgeTxnMetricValue edgeTxnCountLink" type="button" onClick={onOpenDetail}>
                {data.countText}
              </button>
            ) : (
              <div className="edgeTxnMetricValue">{data.countText}</div>
            )}
          </div>
        </div>
      </div>
      <div className="edgeTxnSectionTitle">交易情况</div>
      <div className="edgeTxnList edgeTxnList--cap5">
        {data.loading ? <div className="edgeTxnEmpty">正在加载交易明细…</div> : null}
        {!data.loading && data.error ? <div className="edgeTxnEmpty">{data.error}</div> : null}
        {!data.loading && !data.error && !data.rows.length ? <div className="edgeTxnEmpty">暂无可展示的交易明细</div> : null}
        {!data.loading && !data.error
          ? data.rows.map((row, index) => (
              <div key={`${row.time}-${row.amountText}-${index}`} className="edgeTxnRow">
                <div className="edgeTxnTime">{row.time || "--"}</div>
                <div className={`edgeTxnAmount${row.amountTone ? ` edgeTxnAmount--${row.amountTone}` : ""}`}>
                  {row.amountText}
                </div>
              </div>
            ))
          : null}
      </div>
    </div>
  );
}
