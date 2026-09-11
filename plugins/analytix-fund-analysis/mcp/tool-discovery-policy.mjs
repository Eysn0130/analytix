import {
  DEFAULT_VISIBLE_TOOL_NAMES,
  FRONTDOOR_REPORT_ONLY_TOOL_NAMES,
  FULL_DISCOVERY_ENV_VARS,
  TOOL_LIST_PRIORITY
} from "./capability-registry-runtime.mjs";

export const TOOL_DISCOVERY_POLICY_VERSION = "0.16.16";

export { TOOL_LIST_PRIORITY };

export const AGENT_DISCOVERY_TOOL_NAMES = new Set(DEFAULT_VISIBLE_TOOL_NAMES);
const frontdoorReportOnlyToolNames = new Set(FRONTDOOR_REPORT_ONLY_TOOL_NAMES);

const toolListPriority = new Map(TOOL_LIST_PRIORITY.map((name, index) => [name, index]));

function envText(env, key) {
  return String(env?.[key] || "").trim();
}

export function exposeFullToolDiscovery(env = process.env) {
  return FULL_DISCOVERY_ENV_VARS.some((key) => /^(1|true|yes|full|all)$/iu.test(envText(env, key)));
}

export function orderedToolsForAgent(tools, env = process.env) {
  const ordered = (Array.isArray(tools) ? tools : [])
    .map((tool, index) => ({ tool, index }))
    .sort((left, right) => {
      const leftPriority = toolListPriority.has(left.tool.name)
        ? toolListPriority.get(left.tool.name)
        : TOOL_LIST_PRIORITY.length + left.index;
      const rightPriority = toolListPriority.has(right.tool.name)
        ? toolListPriority.get(right.tool.name)
        : TOOL_LIST_PRIORITY.length + right.index;
      return leftPriority - rightPriority;
    })
    .map((item) => item.tool);

  if (exposeFullToolDiscovery(env)) {
    return ordered;
  }
  if (envText(env, "ANALYTIX_FUNDS_EVAL_TOOL_PROFILE") === "frontdoor_report_only") {
    return ordered.filter((tool) => frontdoorReportOnlyToolNames.has(tool.name));
  }
  return ordered.filter((tool) => AGENT_DISCOVERY_TOOL_NAMES.has(tool.name));
}
