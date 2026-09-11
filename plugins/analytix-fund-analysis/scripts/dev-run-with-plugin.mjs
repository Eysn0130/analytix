#!/usr/bin/env node

import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const PLUGIN_NAME = "analytix-fund-analysis";
const MARKETPLACE_NAME = "analytix-hub";
const DISPLAY_NAME = "Analytix 涉案资金研判";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");
const repoRoot = path.resolve(pluginRoot, "../..");
const runtimeHome = path.resolve(
  process.env.ANALYTIX_AGENT_RUNTIME_HOME ||
    process.env.ANALYTIX_AGENT_RUNTIME_CODEX_HOME ||
    path.join(os.homedir(), ".analytix")
);
const hubMarketplaceRoot = path.join(
  runtimeHome,
  ".cache",
  "analytix-hub-plugins",
  "marketplaces",
  MARKETPLACE_NAME
);
const hubMarketplacePath = path.join(hubMarketplaceRoot, ".agents", "plugins", "marketplace.json");
const overlayRoot = path.join(hubMarketplaceRoot, ".analytix-dev-overlays", PLUGIN_NAME);
const runScript = path.join(repoRoot, "scripts", "dev", "run.sh");
const runtimeConfigPath = path.join(runtimeHome, "config.toml");

function ensureFile(filePath, label) {
  if (!fs.existsSync(filePath) || !fs.statSync(filePath).isFile()) {
    throw new Error(`${label} not found: ${filePath}`);
  }
}

function readJsonFile(filePath) {
  try {
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch (_) {
    return null;
  }
}

function writeJsonFile(filePath, value) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, `${JSON.stringify(value, null, 2)}\n`, "utf8");
}

function pathInsideOrEqual(targetPath, rootPath) {
  const relative = path.relative(path.resolve(rootPath), path.resolve(targetPath));
  return relative === "" || (!!relative && !relative.startsWith("..") && !path.isAbsolute(relative));
}

function removeInsideRuntimeHome(targetPath) {
  if (!pathInsideOrEqual(targetPath, runtimeHome)) {
    throw new Error(`Refusing to remove path outside Analytix runtime home: ${targetPath}`);
  }
  fs.rmSync(targetPath, { recursive: true, force: true });
}

function removePreviousFundInjectionFromHubMarketplace() {
  const marketplace = readJsonFile(hubMarketplacePath);
  if (!marketplace || !Array.isArray(marketplace.plugins)) {
    return false;
  }
  const plugins = marketplace.plugins.filter((plugin) => {
    if (!plugin || plugin.name !== PLUGIN_NAME) {
      return true;
    }
    const sourcePath = String(plugin.source?.path || "").trim();
    return sourcePath !== `./plugins/${PLUGIN_NAME}`;
  });
  if (plugins.length === marketplace.plugins.length) {
    return false;
  }
  writeJsonFile(hubMarketplacePath, {
    ...marketplace,
    name: MARKETPLACE_NAME,
    interface: marketplace.interface || {
      displayName: "Analytix Hub",
    },
    plugins,
  });
  return true;
}

function cleanupPreviousDevState() {
  const removed = [];
  if (removePreviousFundInjectionFromHubMarketplace()) {
    removed.push("hub marketplace entry");
  }
  for (const targetPath of [
    overlayRoot,
    path.join(runtimeHome, ".cache", "analytix-local-marketplaces", "analytix-fund-analysis-test"),
    path.join(runtimeHome, ".cache", "analytix-local-marketplaces", "analytix-fund-analysis-hub-dev"),
  ]) {
    if (fs.existsSync(targetPath)) {
      removeInsideRuntimeHome(targetPath);
      removed.push(targetPath);
    }
  }
  if (removed.length) {
    console.log(`[fund-plugin] cleaned previous dev injection: ${removed.join(", ")}`);
  }
}

function removeFundPluginRuntimeConfigSection() {
  if (!fs.existsSync(runtimeConfigPath)) {
    return;
  }
  const source = fs.readFileSync(runtimeConfigPath, "utf8");
  const section = `[plugins."${PLUGIN_NAME}@${MARKETPLACE_NAME}"]`;
  const sectionIndex = source.indexOf(section);
  if (sectionIndex < 0) {
    return;
  }
  const nextSectionIndex = source.indexOf("\n[", sectionIndex + section.length);
  const before = source.slice(0, sectionIndex).replace(/\n{2,}$/u, "\n\n");
  const after = nextSectionIndex < 0 ? "" : source.slice(nextSectionIndex + 1);
  const nextSource = `${before}${after}`.replace(/\n{3,}/gu, "\n\n").replace(/\s*$/u, "\n");
  fs.writeFileSync(runtimeConfigPath, nextSource, "utf8");
  console.log(`[fund-plugin] removed stale ${PLUGIN_NAME}@${MARKETPLACE_NAME} dev install from runtime config.`);
}

function main() {
  ensureFile(runScript, "Analytix dev launcher");
  cleanupPreviousDevState();
  removeFundPluginRuntimeConfigSection();

  const env = { ...process.env };
  delete env.VITE_ANALYTIX_AUTH_REQUIRED;
  delete env.VITE_ANALYTIX_AUTH_BYPASS_DISABLED;
  delete env.VITE_ANALYTIX_DESKTOP_EDITION_LOCK;
  delete env.VITE_ANALYTIX_DEV_AUTH_BYPASS;
  delete env.ANALYTIX_AGENT_RUNTIME_HUB_PLUGINS_DISABLED;
  delete env.ANALYTIX_AGENT_RUNTIME_ANALYTIX_HUB_MARKETPLACE;
  env.ANALYTIX_FORCE_WEB_BUILD = env.ANALYTIX_FORCE_WEB_BUILD || "1";

  console.log(`[fund-plugin] starting Analytix desktop with published ${DISPLAY_NAME} from Analytix Hub...`);
  const desktop = spawn("bash", [runScript], {
    cwd: repoRoot,
    env,
    stdio: "inherit",
  });
  desktop.once("exit", (code, signal) => {
    if (signal) {
      process.exit(128);
    }
    process.exit(typeof code === "number" ? code : 1);
  });
}

main();
