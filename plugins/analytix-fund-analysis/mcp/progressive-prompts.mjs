import {
  fixedFundsBoundaryPrompt,
  fixedFundsBoundaryPrompts
} from "./source-unavailable-boundary.mjs";

export const PROGRESSIVE_PROMPTS_VERSION = "0.16.16-p0-source-quarantine";

export function promptListForAgent() {
  return fixedFundsBoundaryPrompts();
}

export function getPromptForAgent(name, args = {}) {
  if (!args || typeof args !== "object" || Array.isArray(args) || Object.keys(args).length) {
    throw new Error("P0 boundary prompt does not accept arguments");
  }
  const result = fixedFundsBoundaryPrompt(name);
  if (!result) throw new Error(`Unknown P0 boundary prompt: ${String(name || "")}`);
  return result;
}
