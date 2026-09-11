(() => {
  function closeDrawerDom({ drawer, wrap } = {}) {
    if (!drawer) return;
    if (!drawer.classList.contains("open")) {
      drawer.setAttribute("aria-hidden", "true");
      drawer.classList.remove("closing");
      if (wrap) wrap.classList.remove("active");
      return;
    }
    if (wrap) wrap.classList.remove("active");
    drawer.classList.remove("open");
    drawer.classList.add("closing");
    drawer.setAttribute("aria-hidden", "true");
    const onEnd = (e) => {
      if (e.propertyName !== "transform") return;
      drawer.classList.remove("closing");
      drawer.removeEventListener("transitionend", onEnd);
    };
    drawer.addEventListener("transitionend", onEnd);
  }

  function openDrawerDom({ wrap, drawer, titleEl, bodyEl } = {}, title, html) {
    if (!wrap || !drawer) return false;
    if (titleEl) titleEl.textContent = title || "详情";
    if (bodyEl) bodyEl.innerHTML = html || "";
    wrap.classList.add("active");
    drawer.classList.add("open");
    drawer.classList.remove("closing");
    drawer.setAttribute("aria-hidden", "false");
    return true;
  }

  function closeReactDrawerState(reactOverlay, actions, scheduleShellStateSync, { sync = true } = {}) {
    if (actions && typeof actions.clear === "function") actions.clear();
    const drawer = reactOverlay?.drawer || {};
    const hasRows = Array.isArray(drawer.rows) && drawer.rows.length > 0;
    if (!drawer.open && !drawer.title && !hasRows) return false;
    drawer.open = false;
    drawer.kind = "";
    drawer.title = "";
    drawer.rows = [];
    drawer.actions = [];
    drawer.meta = null;
    if (sync && typeof scheduleShellStateSync === "function") scheduleShellStateSync("react-drawer-close");
    return true;
  }

  function openReactDrawerState(
    reactOverlay,
    actions,
    scheduleShellStateSync,
    payload,
    actionEntries = [],
    reason = "react-drawer-open"
  ) {
    const next = payload && typeof payload === "object" ? payload : {};
    if (actions && typeof actions.clear === "function") actions.clear();
    (Array.isArray(actionEntries) ? actionEntries : []).forEach((entry) => {
      const id = String(entry?.id || "").trim();
      if (!id || typeof entry?.action !== "function") return;
      actions.set(id, entry.action);
    });
    const drawer = reactOverlay?.drawer;
    if (!drawer) return false;
    drawer.open = true;
    drawer.kind = String(next.kind || "");
    drawer.title = String(next.title || "详情");
    drawer.rows = Array.isArray(next.rows) ? next.rows.map((row) => ({ ...row })) : [];
    drawer.actions = Array.isArray(next.actions) ? next.actions.map((item) => ({ ...item })) : [];
    drawer.meta = next.meta && typeof next.meta === "object" ? { ...next.meta } : null;
    if (typeof scheduleShellStateSync === "function") scheduleShellStateSync(reason);
    return true;
  }

  function runReactDrawerAction(actions, actionId) {
    const id = String(actionId || "").trim();
    if (!id || !actions || typeof actions.get !== "function") return false;
    const action = actions.get(id);
    if (typeof action !== "function") return false;
    try {
      action();
      return true;
    } catch (e) {
      return false;
    }
  }

  window.__ANALYTIX_FLOW_DRAWER_CONTROLLER__ = {
    closeDrawerDom,
    openDrawerDom,
    closeReactDrawerState,
    openReactDrawerState,
    runReactDrawerAction,
  };
})();
