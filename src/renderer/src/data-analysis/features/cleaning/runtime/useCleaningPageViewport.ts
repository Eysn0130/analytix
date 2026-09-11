import { useLayoutEffect, useMemo, useState, type CSSProperties, type RefObject } from "react";
import {
  CLEANING_PAGE_DESIGN_CONTENT_WIDTH,
  CLEANING_PAGE_DESIGN_VIEWPORT_HEIGHT,
  CLEANING_PAGE_DESIGN_VIEWPORT_WIDTH,
  CLEANING_PAGE_MIN_CONTENT_WIDTH,
  CLEANING_PAGE_MIN_SCALE
} from "../model/constants";

interface UseCleaningPageViewportOptions {
  active: boolean;
  pageRef: RefObject<HTMLElement | null>;
}

export function useCleaningPageViewport({
  active,
  pageRef
}: UseCleaningPageViewportOptions): CSSProperties {
  const [viewportScale, setViewportScale] = useState(1);

  useLayoutEffect(() => {
    const pageNode = pageRef.current;
    const scrollHost = pageNode?.closest(".app-main");
    if (!(scrollHost instanceof HTMLElement)) {
      return;
    }
    if (active) {
      scrollHost.classList.add("cleaning-scrollbar-hidden");
    } else {
      scrollHost.classList.remove("cleaning-scrollbar-hidden");
    }
    return () => {
      scrollHost.classList.remove("cleaning-scrollbar-hidden");
    };
  }, [active, pageRef]);

  useLayoutEffect(() => {
    const pageNode = pageRef.current;
    const scrollHost = pageNode?.closest(".app-main");
    if (!(pageNode instanceof HTMLElement) || !(scrollHost instanceof HTMLElement)) {
      return;
    }

    let rafId = 0;

    const syncViewportScale = (): void => {
      rafId = 0;
      const viewportWidth = Math.max(window.innerWidth || 0, scrollHost.clientWidth || 0);
      const viewportHeight = Math.max(window.innerHeight || 0, scrollHost.clientHeight || 0);
      const contentWidth = Math.max(pageNode.clientWidth || 0, scrollHost.clientWidth || 0);
      const widthScale = viewportWidth / CLEANING_PAGE_DESIGN_VIEWPORT_WIDTH;
      const contentScale = contentWidth / CLEANING_PAGE_DESIGN_CONTENT_WIDTH;
      const heightScale = viewportHeight / CLEANING_PAGE_DESIGN_VIEWPORT_HEIGHT;
      const nextScale = Math.max(CLEANING_PAGE_MIN_SCALE, Math.min(1, widthScale, contentScale, heightScale));
      const roundedScale = Number(nextScale.toFixed(4));
      setViewportScale((previous) => (Math.abs(previous - roundedScale) > 0.001 ? roundedScale : previous));
    };

    const requestSync = (): void => {
      if (rafId !== 0) {
        return;
      }
      rafId = window.requestAnimationFrame(syncViewportScale);
    };

    const observer = typeof ResizeObserver !== "undefined" ? new ResizeObserver(requestSync) : null;
    observer?.observe(scrollHost);
    observer?.observe(pageNode);
    syncViewportScale();
    window.addEventListener("resize", requestSync);

    return () => {
      if (rafId !== 0) {
        window.cancelAnimationFrame(rafId);
      }
      observer?.disconnect();
      window.removeEventListener("resize", requestSync);
    };
  }, [pageRef]);

  return useMemo(
    () =>
      ({
        "--cleaning-page-scale": viewportScale.toFixed(4),
        "--cleaning-page-min-content-width": `${CLEANING_PAGE_MIN_CONTENT_WIDTH}px`,
        "--cleaning-page-min-scale": CLEANING_PAGE_MIN_SCALE.toFixed(2)
      }) as CSSProperties,
    [viewportScale]
  );
}
