/* Network sector placement client.
 * Responsibilities: call the Rust-backed network sector placement contract through the embedded backend.
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

  function createNetworkSectorPlacementClient(deps = {}) {
    const backendCall = requireFunction(
      deps?.backendCall,
      "network-sector-placement-client-backend-call-missing"
    );
    const parseJSONSafe = requireFunction(
      deps?.parseJSONSafe,
      "network-sector-placement-client-json-missing"
    );
    const getCaseId =
      typeof deps?.getCaseId === "function"
        ? deps.getCaseId
        : () => String(deps?.caseId || "");

    async function projectNetworkSectorPlacement(payload = {}) {
      const request = payload && typeof payload === "object" ? payload : {};
      const raw = await backendCall(
        "projectNetworkSectorPlacement",
        JSON.stringify({
          ...request,
          caseId: String(getCaseId() || ""),
        })
      );
      const result = parseJSONSafe(raw, {});
      return result?.ok ? result : null;
    }

    return { projectNetworkSectorPlacement };
  }

  root.__ANALYTIX_FLOW_NETWORK_SECTOR_PLACEMENT_CLIENT__ = {
    createNetworkSectorPlacementClient,
  };
})();
