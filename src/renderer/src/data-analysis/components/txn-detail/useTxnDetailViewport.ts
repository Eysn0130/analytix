import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
  type UIEventHandler,
  type WheelEventHandler
} from "react";
import { buildTxnDetailRowWindow, estimateTxnDetailViewportRows } from "./txn-detail-window-model";

interface UseTxnDetailViewportOptions {
  open: boolean;
  rowCount: number;
  rowHeight: number;
  headerHeight: number;
  overscanRows: number;
  initialViewportRows?: number;
  resetKey?: string | number | null;
}

interface TxnDetailViewportState {
  tableWrapRef: RefObject<HTMLDivElement | null>;
  scrollTop: number;
  scrollLeft: number;
  viewportRows: number;
  viewportWidth: number;
  windowStart: number;
  windowEnd: number;
  topSpacerHeight: number;
  bottomSpacerHeight: number;
  onTableScroll: UIEventHandler<HTMLDivElement>;
  onTableWheel: WheelEventHandler<HTMLDivElement>;
  resetScrollPosition: () => void;
}

export function useTxnDetailViewport({
  open,
  rowCount,
  rowHeight,
  headerHeight,
  overscanRows,
  initialViewportRows = 12,
  resetKey = null
}: UseTxnDetailViewportOptions): TxnDetailViewportState {
  const tableWrapRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [scrollLeft, setScrollLeft] = useState(0);
  const [viewportRows, setViewportRows] = useState(initialViewportRows);
  const [viewportWidth, setViewportWidth] = useState(1);

  const applyScrollPosition = useCallback((next: { scrollTop: number; scrollLeft: number }): void => {
    const safeNext = {
      scrollTop: Math.max(0, next.scrollTop || 0),
      scrollLeft: Math.max(0, next.scrollLeft || 0)
    };
    setScrollTop((current) => (current === safeNext.scrollTop ? current : safeNext.scrollTop));
    setScrollLeft((current) => (current === safeNext.scrollLeft ? current : safeNext.scrollLeft));
  }, []);

  const syncViewport = useCallback((): void => {
    const element = tableWrapRef.current;
    if (!element) {
      return;
    }
    const nextViewportRows = estimateTxnDetailViewportRows({
      clientHeight: element.clientHeight,
      rowHeight,
      headerHeight,
      fallbackRows: initialViewportRows
    });
    const nextViewportWidth = Math.max(1, Math.ceil(element.clientWidth || 0));
    const nextScrollLeft = Math.max(0, element.scrollLeft || 0);
    setViewportRows((current) => (current === nextViewportRows ? current : nextViewportRows));
    setViewportWidth((current) => (current === nextViewportWidth ? current : nextViewportWidth));
    setScrollLeft((current) => (current === nextScrollLeft ? current : nextScrollLeft));
  }, [headerHeight, initialViewportRows, rowHeight]);

  const resetScrollPosition = useCallback((): void => {
    applyScrollPosition({ scrollTop: 0, scrollLeft: 0 });
    const element = tableWrapRef.current;
    if (element) {
      element.scrollTop = 0;
      element.scrollLeft = 0;
    }
  }, [applyScrollPosition]);

  useEffect(() => {
    if (!open) {
      return;
    }
    let rafId = 0;
    const scheduleSync = (): void => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      rafId = window.requestAnimationFrame(() => {
        rafId = 0;
        syncViewport();
      });
    };
    const resizeObserver = typeof ResizeObserver !== "undefined" ? new ResizeObserver(scheduleSync) : null;
    if (tableWrapRef.current) {
      resizeObserver?.observe(tableWrapRef.current);
    }
    scheduleSync();
    window.addEventListener("resize", scheduleSync);
    return () => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      resizeObserver?.disconnect();
      window.removeEventListener("resize", scheduleSync);
    };
  }, [open, syncViewport]);

  useEffect(() => {
    if (open) {
      resetScrollPosition();
    }
  }, [open, resetKey, resetScrollPosition]);

  const rowWindow = useMemo(
    () =>
      buildTxnDetailRowWindow({
        rowCount,
        scrollTop,
        viewportRows,
        rowHeight,
        overscanRows
      }),
    [overscanRows, rowCount, rowHeight, scrollTop, viewportRows]
  );

  const onTableScroll: UIEventHandler<HTMLDivElement> = useCallback((event) => {
    const element = event.currentTarget;
    applyScrollPosition({
      scrollTop: element.scrollTop,
      scrollLeft: element.scrollLeft
    });
  }, [applyScrollPosition]);

  const onTableWheel: WheelEventHandler<HTMLDivElement> = useCallback((event) => {
    const horizontalDelta = event.deltaX || (event.shiftKey ? event.deltaY : 0);
    if (!horizontalDelta) {
      return;
    }
    const element = event.currentTarget;
    const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
    if (maxScrollLeft <= 0) {
      return;
    }
    const currentScrollLeft = element.scrollLeft;
    const nextScrollLeft = Math.min(maxScrollLeft, Math.max(0, currentScrollLeft + horizontalDelta));
    if (nextScrollLeft === currentScrollLeft) {
      return;
    }
    element.scrollLeft = nextScrollLeft;
    applyScrollPosition({
      scrollTop: element.scrollTop,
      scrollLeft: element.scrollLeft
    });
    if (event.shiftKey || Math.abs(horizontalDelta) >= Math.abs(event.deltaY)) {
      event.preventDefault();
    }
  }, [applyScrollPosition]);

  return {
    tableWrapRef,
    scrollTop,
    scrollLeft,
    viewportRows,
    viewportWidth,
    windowStart: rowWindow.startIndex,
    windowEnd: rowWindow.endIndex,
    topSpacerHeight: rowWindow.topSpacerHeight,
    bottomSpacerHeight: rowWindow.bottomSpacerHeight,
    onTableScroll,
    onTableWheel,
    resetScrollPosition
  };
}
