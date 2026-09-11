/* Layout role projection client.
 * Responsibilities: call the Rust-backed role graph projection contract through the embedded backend.
 */
(() => {
  const root =
    typeof globalThis !== "undefined" ? globalThis : typeof window !== "undefined" ? window : null;
  if (!root) return;

  function requireFunction(fn, message) {
    if (typeof fn !== "function") {
      throw new Error(message);
    }
    return fn;
  }

  function createLayoutRoleProjectionClient(deps = {}) {
    const backendCall = requireFunction(
      deps?.backendCall,
      "layout-role-projection-client-backend-call-missing"
    );
    const parseJSONSafe = requireFunction(
      deps?.parseJSONSafe,
      "layout-role-projection-client-json-missing"
    );
    const getCaseId =
      typeof deps?.getCaseId === "function"
        ? deps.getCaseId
        : () => String(deps?.caseId || "");

    async function projectLayoutRoleGraph(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const raw = await backendCall(
        "projectLayoutRoleGraph",
        JSON.stringify({
          ...request,
          caseId: String(getCaseId() || ""),
        })
      );
      const result = parseJSONSafe(raw, {});
      return result?.ok ? result : null;
    }

    return { projectLayoutRoleGraph };
  }

  root.__ANALYTIX_FLOW_LAYOUT_ROLE_PROJECTION_CLIENT__ = {
    createLayoutRoleProjectionClient,
  };
})();
