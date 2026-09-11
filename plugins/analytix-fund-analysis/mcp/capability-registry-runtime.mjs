import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { objectOf, text } from "./runtime-normalizers.mjs";

export const CAPABILITY_REGISTRY_RUNTIME_VERSION = "0.16.16";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const DEFAULT_REGISTRY_PATH = path.join(__dirname, "..", "references", "capability-registry.json");

function textList(value) {
  return Array.isArray(value) ? value.map(text).filter(Boolean) : [];
}

function positiveInteger(value) {
  const next = Number(value);
  return Number.isInteger(next) && next > 0 ? next : 0;
}

export function readCapabilityRegistry(registryPath = DEFAULT_REGISTRY_PATH) {
  const registry = objectOf(JSON.parse(fs.readFileSync(registryPath, "utf8")));
  const contract = objectOf(registry.tool_discovery_contract);
  const priorityTools = textList(contract.priority_tools);
  const defaultVisibleTools = textList(contract.default_visible_tools);
  const frontdoorReportOnlyTools = textList(contract.frontdoor_report_only_tools);
  const fullDiscoveryEnvVars = textList(contract.full_discovery_env_vars);
  const maxDefaultVisibleTools = positiveInteger(contract.max_default_visible_tools);
  const failures = [
    text(registry.schema_version) !== "capability-registry-v1" ? "schema_version" : "",
    !priorityTools.length ? "tool_discovery_contract.priority_tools" : "",
    !defaultVisibleTools.length ? "tool_discovery_contract.default_visible_tools" : "",
    !frontdoorReportOnlyTools.length ? "tool_discovery_contract.frontdoor_report_only_tools" : "",
    !fullDiscoveryEnvVars.length ? "tool_discovery_contract.full_discovery_env_vars" : "",
    !maxDefaultVisibleTools ? "tool_discovery_contract.max_default_visible_tools" : "",
    defaultVisibleTools.length > maxDefaultVisibleTools ? "tool_discovery_contract.default_visible_tools exceeds max_default_visible_tools" : ""
  ].filter(Boolean);
  if (failures.length) {
    throw new Error(`invalid capability registry: ${failures.join(", ")}`);
  }
  return {
    registry,
    toolDiscoveryContract: {
      priorityTools: Object.freeze([...priorityTools]),
      defaultVisibleTools: Object.freeze([...defaultVisibleTools]),
      frontdoorReportOnlyTools: Object.freeze([...frontdoorReportOnlyTools]),
      fullDiscoveryEnvVars: Object.freeze([...fullDiscoveryEnvVars]),
      maxDefaultVisibleTools
    }
  };
}

export function getToolDiscoveryContract(registryPath = DEFAULT_REGISTRY_PATH) {
  return readCapabilityRegistry(registryPath).toolDiscoveryContract;
}

const toolDiscoveryContract = getToolDiscoveryContract();

export const TOOL_LIST_PRIORITY = toolDiscoveryContract.priorityTools;
export const DEFAULT_VISIBLE_TOOL_NAMES = toolDiscoveryContract.defaultVisibleTools;
export const FRONTDOOR_REPORT_ONLY_TOOL_NAMES = toolDiscoveryContract.frontdoorReportOnlyTools;
export const FULL_DISCOVERY_ENV_VARS = toolDiscoveryContract.fullDiscoveryEnvVars;
export const MAX_DEFAULT_VISIBLE_TOOLS = toolDiscoveryContract.maxDefaultVisibleTools;
