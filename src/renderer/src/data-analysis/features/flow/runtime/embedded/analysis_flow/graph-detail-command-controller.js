(() => {
  function openNodeDetailDrawer(model, graph, deps = {}) {
    if (!callBool(deps.isReactShellSubscribed)) {
      if (typeof deps.openDrawer !== "function" || typeof deps.buildNodeDetailHtml !== "function") return false;
      deps.openDrawer(model?.title || model?.label || model?.id || "节点", deps.buildNodeDetailHtml(model, graph));
      return true;
    }
    if (typeof deps.buildReactNodeDetailDrawer !== "function" || typeof deps.openReactDrawer !== "function") return false;
    const payload = deps.buildReactNodeDetailDrawer(model, graph);
    deps.openReactDrawer(payload, buildNodeDetailActionEntries(payload, model, graph, deps));
    return true;
  }

  function openEdgeDetailDrawer(model, deps = {}) {
    if (!callBool(deps.isReactShellSubscribed)) {
      if (typeof deps.openDrawer !== "function" || typeof deps.buildEdgeDetailHtml !== "function") return false;
      deps.openDrawer(model?.label || model?.id || "连线详情", deps.buildEdgeDetailHtml(model));
      return true;
    }
    if (typeof deps.buildReactEdgeDetailDrawer !== "function" || typeof deps.openReactDrawer !== "function") return false;
    deps.openReactDrawer(deps.buildReactEdgeDetailDrawer(model), [], "react-edge-drawer-open");
    return true;
  }

  function buildNodeDetailActionEntries(payload, model, graph, deps = {}) {
    return [
      {
        id: "copy-node-id",
        action: () => {
          const nodeId = String(payload?.meta?.nodeId || model?.id || "").trim();
          if (!nodeId) return;
          if (typeof deps.copyText === "function") deps.copyText(nodeId);
          if (typeof deps.toast === "function") deps.toast("已复制", "ok");
        },
      },
      {
        id: "focus-node",
        action: () => {
          const nodeId = String(payload?.meta?.nodeId || model?.id || "").trim();
          if (!nodeId) return;
          try {
            const item = graph?.findById?.(nodeId);
            if (item) graph.focusItem(item, true, { easing: "easeCubic", duration: 240 });
          } catch (e) {}
        },
      },
    ];
  }

  function callBool(fn) {
    return typeof fn === "function" ? !!fn() : false;
  }

  window.__ANALYTIX_FLOW_DETAIL_COMMAND_CONTROLLER__ = {
    openNodeDetailDrawer,
    openEdgeDetailDrawer,
    buildNodeDetailActionEntries,
  };
})();
