#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const pluginRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryRoot = path.resolve(pluginRoot, "..", "..");

const contracts = [
  "plugins/analytix-fund-analysis/scripts/p0-containment-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/funds-producer-content-v1-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/dataset-snapshot-manifest-v2-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/tool-schema-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/runtime-install-identity-contract.mjs",
];

for (const contract of contracts) {
  const result = spawnSync(process.execPath, [contract], {
    cwd: repositoryRoot,
    env: process.env,
    stdio: "inherit",
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

console.log(`Funds P0 contract suite passed (${contracts.length}/${contracts.length}).`);
