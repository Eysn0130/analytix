import {
  fixedFundsBoundaryResourceRead,
  fixedFundsBoundaryResources
} from "./source-unavailable-boundary.mjs";

export const PROGRESSIVE_RESOURCES_VERSION = "0.16.16-p0-source-quarantine";

export const resourceDefinitions = fixedFundsBoundaryResources();

export function resourceListForAgent() {
  return fixedFundsBoundaryResources();
}

export function readResourceForAgent(pluginRoot, uri) {
  void pluginRoot;
  const result = fixedFundsBoundaryResourceRead(uri);
  if (!result) throw new Error(`Unknown P0 boundary resource: ${String(uri || "")}`);
  return result;
}
