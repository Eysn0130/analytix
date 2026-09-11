import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type RefObject
} from "react";

const MIN_THUMB_WIDTH = 42;

interface PersistentHorizontalScrollbarProps {
  scrollRef: RefObject<HTMLDivElement | null>;
  className?: string;
  ariaLabel?: string;
}

interface ScrollMetrics {
  scrollLeft: number;
  scrollWidth: number;
  clientWidth: number;
  trackWidth: number;
}

interface DragState {
  startX: number;
  startScrollLeft: number;
  maxScrollLeft: number;
  maxThumbLeft: number;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

export function PersistentHorizontalScrollbar({
  scrollRef,
  className = "",
  ariaLabel = "横向滚动条"
}: PersistentHorizontalScrollbarProps): JSX.Element {
  const trackRef = useRef<HTMLDivElement | null>(null);
  const dragRef = useRef<DragState | null>(null);
  const [metrics, setMetrics] = useState<ScrollMetrics>({
    scrollLeft: 0,
    scrollWidth: 0,
    clientWidth: 0,
    trackWidth: 0
  });

  const syncMetrics = useCallback((): void => {
    const scrollElement = scrollRef.current;
    const trackElement = trackRef.current;
    if (!scrollElement || !trackElement) {
      return;
    }
    const scrollWidth = Math.max(0, Math.ceil(scrollElement.scrollWidth || 0));
    const clientWidth = Math.max(0, Math.ceil(scrollElement.clientWidth || 0));
    const maxScrollLeft = Math.max(0, scrollWidth - clientWidth);
    const scrollLeft = clamp(Math.ceil(scrollElement.scrollLeft || 0), 0, maxScrollLeft);
    const trackWidth = Math.max(0, Math.floor(trackElement.clientWidth || 0));
    setMetrics((current) => {
      if (
        current.scrollLeft === scrollLeft &&
        current.scrollWidth === scrollWidth &&
        current.clientWidth === clientWidth &&
        current.trackWidth === trackWidth
      ) {
        return current;
      }
      return { scrollLeft, scrollWidth, clientWidth, trackWidth };
    });
  }, [scrollRef]);

  const applyScrollLeft = useCallback((nextScrollLeft: number): void => {
    const scrollElement = scrollRef.current;
    if (!scrollElement) {
      return;
    }
    const maxScrollLeft = Math.max(0, scrollElement.scrollWidth - scrollElement.clientWidth);
    scrollElement.scrollLeft = clamp(nextScrollLeft, 0, maxScrollLeft);
    syncMetrics();
  }, [scrollRef, syncMetrics]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return undefined;
    }
    const scrollElement = scrollRef.current;
    if (!scrollElement) {
      return undefined;
    }

    let rafId = 0;
    const scheduleSync = (): void => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      rafId = window.requestAnimationFrame(() => {
        rafId = 0;
        syncMetrics();
      });
    };

    const resizeObserver = typeof ResizeObserver !== "undefined" ? new ResizeObserver(scheduleSync) : null;
    resizeObserver?.observe(scrollElement);
    if (trackRef.current) {
      resizeObserver?.observe(trackRef.current);
    }
    if (scrollElement.firstElementChild instanceof HTMLElement) {
      resizeObserver?.observe(scrollElement.firstElementChild);
    }

    scrollElement.addEventListener("scroll", scheduleSync, { passive: true });
    window.addEventListener("resize", scheduleSync);
    scheduleSync();

    return () => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      resizeObserver?.disconnect();
      scrollElement.removeEventListener("scroll", scheduleSync);
      window.removeEventListener("resize", scheduleSync);
    };
  }, [scrollRef, syncMetrics]);

  const maxScrollLeft = Math.max(0, metrics.scrollWidth - metrics.clientWidth);
  const isScrollable = maxScrollLeft > 0;
  const rawThumbWidth = metrics.scrollWidth > 0 ? Math.round((metrics.clientWidth / metrics.scrollWidth) * metrics.trackWidth) : metrics.trackWidth;
  const thumbWidth = metrics.trackWidth > 0 ? Math.min(metrics.trackWidth, Math.max(MIN_THUMB_WIDTH, rawThumbWidth)) : 0;
  const maxThumbLeft = Math.max(0, metrics.trackWidth - thumbWidth);
  const thumbLeft = isScrollable && maxThumbLeft > 0 ? Math.round((metrics.scrollLeft / maxScrollLeft) * maxThumbLeft) : 0;

  const handleTrackPointerDown = useCallback((event: ReactPointerEvent<HTMLDivElement>): void => {
    if (!isScrollable || event.button !== 0 || !trackRef.current) {
      return;
    }
    const rect = trackRef.current.getBoundingClientRect();
    const nextThumbLeft = clamp(event.clientX - rect.left - thumbWidth / 2, 0, maxThumbLeft);
    const nextScrollLeft = maxThumbLeft > 0 ? (nextThumbLeft / maxThumbLeft) * maxScrollLeft : 0;
    applyScrollLeft(nextScrollLeft);
    event.preventDefault();
  }, [applyScrollLeft, isScrollable, maxScrollLeft, maxThumbLeft, thumbWidth]);

  const handleThumbPointerDown = useCallback((event: ReactPointerEvent<HTMLDivElement>): void => {
    if (!isScrollable || event.button !== 0) {
      return;
    }
    const ownerDocument = event.currentTarget.ownerDocument;
    dragRef.current = {
      startX: event.clientX,
      startScrollLeft: metrics.scrollLeft,
      maxScrollLeft,
      maxThumbLeft
    };

    const handlePointerMove = (moveEvent: PointerEvent): void => {
      const dragState = dragRef.current;
      if (!dragState || dragState.maxThumbLeft <= 0) {
        return;
      }
      const deltaX = moveEvent.clientX - dragState.startX;
      const nextScrollLeft = dragState.startScrollLeft + (deltaX / dragState.maxThumbLeft) * dragState.maxScrollLeft;
      applyScrollLeft(nextScrollLeft);
      moveEvent.preventDefault();
    };

    const handlePointerUp = (): void => {
      dragRef.current = null;
      ownerDocument.removeEventListener("pointermove", handlePointerMove);
      ownerDocument.removeEventListener("pointerup", handlePointerUp);
      ownerDocument.removeEventListener("pointercancel", handlePointerUp);
    };

    ownerDocument.addEventListener("pointermove", handlePointerMove);
    ownerDocument.addEventListener("pointerup", handlePointerUp);
    ownerDocument.addEventListener("pointercancel", handlePointerUp);
    event.preventDefault();
    event.stopPropagation();
  }, [applyScrollLeft, isScrollable, maxScrollLeft, maxThumbLeft, metrics.scrollLeft]);

  const handleKeyDown = useCallback((event: KeyboardEvent<HTMLDivElement>): void => {
    if (!isScrollable) {
      return;
    }
    const step = Math.max(40, Math.floor(metrics.clientWidth * 0.12));
    const pageStep = Math.max(step, Math.floor(metrics.clientWidth * 0.85));
    let nextScrollLeft: number | null = null;
    if (event.key === "ArrowLeft") {
      nextScrollLeft = metrics.scrollLeft - step;
    } else if (event.key === "ArrowRight") {
      nextScrollLeft = metrics.scrollLeft + step;
    } else if (event.key === "PageUp") {
      nextScrollLeft = metrics.scrollLeft - pageStep;
    } else if (event.key === "PageDown") {
      nextScrollLeft = metrics.scrollLeft + pageStep;
    } else if (event.key === "Home") {
      nextScrollLeft = 0;
    } else if (event.key === "End") {
      nextScrollLeft = maxScrollLeft;
    }
    if (nextScrollLeft === null) {
      return;
    }
    applyScrollLeft(nextScrollLeft);
    event.preventDefault();
  }, [applyScrollLeft, isScrollable, maxScrollLeft, metrics.clientWidth, metrics.scrollLeft]);

  return (
    <div
      ref={trackRef}
      className={`persistentHorizontalScrollbar ${isScrollable ? "is-scrollable" : "is-static"} ${className}`.trim()}
      role="scrollbar"
      aria-label={ariaLabel}
      aria-orientation="horizontal"
      aria-valuemin={0}
      aria-valuemax={maxScrollLeft}
      aria-valuenow={metrics.scrollLeft}
      tabIndex={isScrollable ? 0 : -1}
      onKeyDown={handleKeyDown}
      onPointerDown={handleTrackPointerDown}
    >
      <div
        className="persistentHorizontalScrollbar__thumb"
        style={{
          width: `${thumbWidth}px`,
          transform: `translateX(${thumbLeft}px)`
        }}
        onPointerDown={handleThumbPointerDown}
      />
    </div>
  );
}
