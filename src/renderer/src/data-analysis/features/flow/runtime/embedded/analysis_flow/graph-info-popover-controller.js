(() => {
  function hideInfoPopover(popover) {
    if (!popover) return null;
    popover.style.display = "none";
    popover.setAttribute("aria-hidden", "true");
    popover.innerHTML = "";
    return null;
  }

  function showInfoPopoverAt(popover, x, y, html, viewport = window) {
    if (!popover) return null;
    popover.innerHTML = String(html || "");
    popover.style.display = "block";
    popover.setAttribute("aria-hidden", "false");
    const rect = popover.getBoundingClientRect();
    const vw = Number(viewport?.innerWidth) || 0;
    const vh = Number(viewport?.innerHeight) || 0;
    const left = Math.max(10, Math.min(Number(x) || 0, vw - rect.width - 10));
    const top = Math.max(10, Math.min(Number(y) || 0, vh - rect.height - 10));
    popover.style.left = `${left}px`;
    popover.style.top = `${top}px`;
    return popover;
  }

  function shouldCloseInfoPopover(activePopover, target, selector) {
    if (!activePopover) return false;
    if (!target || typeof target.closest !== "function") return true;
    return !target.closest(selector);
  }

  function eventTargetLabel(target) {
    return String(target?.id || target?.className || target?.tagName || "").slice(0, 120);
  }

  function routeInlineInfoPopoverWheel(popover, ev, options = {}) {
    if (!popover || !ev) return false;
    const listSelector = options.listSelector || ".edgeTxnList";
    const route = options.route || "edgePopover";
    const handleWheelScroll = options.handleWheelScroll;
    const logWheelRoute = options.logWheelRoute;
    const target = ev.target;
    const list = target?.closest?.(listSelector) || popover.querySelector?.(listSelector);
    if (!list) return false;
    if (typeof logWheelRoute === "function") {
      logWheelRoute("check", {
        route,
        target: eventTargetLabel(target),
      });
    }
    if (typeof handleWheelScroll === "function") {
      handleWheelScroll(list, ev, { fallbackToHorizontal: true });
    }
    return true;
  }

  function routeDocumentInfoPopoverWheel(ev, options = {}) {
    if (!ev) return false;
    const activeEdgeInfoPopover = options.activeEdgeInfoPopover || null;
    const activeNodeInfoPopover = options.activeNodeInfoPopover || null;
    if (
      routeDocumentPopoverWheel(ev, activeEdgeInfoPopover, {
        listSelector: ".edgeTxnList",
        route: "document->edgePopover",
        handleWheelScroll: options.handleWheelScroll,
        logWheelRoute: options.logWheelRoute,
      })
    ) {
      return true;
    }
    return routeDocumentPopoverWheel(ev, activeNodeInfoPopover, {
      listSelector: ".nodeInfoList",
      route: "document->nodePopover",
      handleWheelScroll: options.handleWheelScroll,
      logWheelRoute: options.logWheelRoute,
    });
  }

  function routeDocumentPopoverWheel(ev, popover, options) {
    if (!popover || !popover.contains?.(ev.target)) return false;
    const list = ev.target?.closest?.(options.listSelector) || popover.querySelector?.(options.listSelector);
    if (list) {
      const moved = typeof options.handleWheelScroll === "function"
        ? options.handleWheelScroll(list, ev, { fallbackToHorizontal: true })
        : false;
      if (typeof options.logWheelRoute === "function") {
        options.logWheelRoute("route", {
          route: options.route,
          moved,
          target: eventTargetLabel(ev.target),
        });
      }
    } else if (typeof options.logWheelRoute === "function") {
      options.logWheelRoute("miss", { route: options.route, reason: "no-list" });
    }
    ev.preventDefault();
    ev.stopPropagation();
    return true;
  }

  window.__ANALYTIX_FLOW_INFO_POPOVER_CONTROLLER__ = {
    hideInfoPopover,
    showInfoPopoverAt,
    shouldCloseInfoPopover,
    routeInlineInfoPopoverWheel,
    routeDocumentInfoPopoverWheel,
  };
})();
