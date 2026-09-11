#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  inspectProductionMcpEntryClosure,
  PRODUCTION_MCP_ENTRY_CLOSURE_FILES
} from "./production-mcp-entry-closure-contract.mjs";

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const MARKETPLACE_NAME = "analytix-hub";
const PRODUCTION_MCP_ENTRY_CLOSURE_SET = new Set(PRODUCTION_MCP_ENTRY_CLOSURE_FILES);
const require = createRequire(import.meta.url);
const { inspectFundsPluginSourceProjectionsV1 } = require("../../../scripts/after-pack.cjs")._internals;
const CASE_SPECIFIC_PRODUCTION_MARKERS = [
  "合成主体甲",
  "合成主体乙",
  "刑侦扫黑专案",
  "25,831,013",
  "25831013",
  "21,000,000",
  "21000000",
  "42,000,000",
  "42000000",
  "9000000000000000015",
  "9000000000000000018"
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function parseArgs(argv) {
  const options = {
    json: false,
    force: false,
    pluginRoot: PLUGIN_ROOT,
    outRoot: "",
    archivePath: "",
    skipArchive: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--json") options.json = true;
    else if (arg === "--force") options.force = true;
    else if (arg === "--skip-archive") options.skipArchive = true;
    else if (arg === "--plugin-root") {
      options.pluginRoot = path.resolve(text(argv[index + 1]));
      index += 1;
    } else if (arg === "--out-root") {
      options.outRoot = path.resolve(text(argv[index + 1]));
      index += 1;
    } else if (arg === "--archive") {
      options.archivePath = path.resolve(text(argv[index + 1]));
      index += 1;
    } else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/prepare-hub-package.mjs [options]

Prepares a local Analytix Hub package source for release review.
This is not a Hub publish, install, upgrade, or runtime-cache remount path.

Options:
  --plugin-root <path>  Plugin root. Default: current plugin.
  --out-root <path>     Output source root. Default: /tmp/analytix-hub-fund-plugin-<version>-source.
  --archive <path>      Output tar.gz path. Default: /tmp/analytix-fund-analysis-<version>-hub-package.tar.gz.
  --skip-archive        Prepare source tree without tar.gz.
  --force               Replace the output source/archive when they already exist.
  --json                Print JSON.
`);
}

function assertInsideTmp(targetPath) {
  const resolved = path.resolve(targetPath);
  const tmpRoot = path.resolve(os.tmpdir());
  if (resolved !== tmpRoot && !resolved.startsWith(`${tmpRoot}${path.sep}`)) {
    throw new Error(`Refusing to write outside temporary directory: ${resolved}`);
  }
}

function copyPluginSource(pluginRoot, destinationPluginRoot) {
  fs.cpSync(pluginRoot, destinationPluginRoot, {
    recursive: true,
    dereference: false,
    filter(source) {
      const base = path.basename(source);
      const relativePath = path.relative(pluginRoot, source).split(path.sep).join("/");
      if (base === ".DS_Store" || base === "node_modules" || base === ".git") return false;
      if (relativePath === "evidence" || relativePath.startsWith("evidence/")) return false;
      if (relativePath === "output" || relativePath.startsWith("output/")) return false;
      if (relativePath === "scripts" || relativePath.startsWith("scripts/")) return false;
      if (relativePath === "mcp") return true;
      if (relativePath.startsWith("mcp/")) {
        return PRODUCTION_MCP_ENTRY_CLOSURE_SET.has(relativePath);
      }
      return true;
    }
  });
}

function validateProductionMcpPayload(destinationPluginRoot) {
  const mcpRoot = path.join(destinationPluginRoot, "mcp");
  const files = fs.readdirSync(mcpRoot, { withFileTypes: true })
    .map((entry) => {
      if (!entry.isFile() || entry.isSymbolicLink()) {
        throw new Error(`Packaged production MCP entry is not a regular file: mcp/${entry.name}`);
      }
      return `mcp/${entry.name}`;
    })
    .sort();
  const expected = [...PRODUCTION_MCP_ENTRY_CLOSURE_FILES].sort();
  if (JSON.stringify(files) !== JSON.stringify(expected)) {
    throw new Error(`Packaged production MCP inventory mismatch: actual=${files.join(",")} expected=${expected.join(",")}`);
  }
  const closure = inspectProductionMcpEntryClosure(destinationPluginRoot);
  return {
    ok: true,
    contract: "ProductionMcpEntryClosureV1",
    files,
    edge_count: closure.edges.length
  };
}

function writeMarketplace(outRoot, projection) {
  const { packageId, packageVersion, manifest } = projection;
  const marketplace = {
    name: MARKETPLACE_NAME,
    interface: {
      displayName: "Analytix Hub"
    },
    plugins: [
      {
        name: packageId,
        version: packageVersion,
        source: {
          source: "local",
          path: `./plugins/${packageId}`
        },
        policy: {
          installation: "AVAILABLE",
          authentication: "ON_INSTALL"
        },
        category: manifest.interface.category,
        interface: manifest.interface
      }
    ]
  };
  const marketplacePath = path.join(outRoot, ".agents", "plugins", "marketplace.json");
  fs.mkdirSync(path.dirname(marketplacePath), { recursive: true });
  fs.writeFileSync(marketplacePath, `${JSON.stringify(marketplace, null, 2)}\n`, "utf8");
  return marketplacePath;
}

function walkFiles(root) {
  const files = [];
  const stack = [root];
  while (stack.length) {
    const current = stack.pop();
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const absolutePath = path.join(current, entry.name);
      if (entry.isDirectory()) stack.push(absolutePath);
      else if (entry.isFile()) files.push(absolutePath);
    }
  }
  return files.sort();
}

function validateGoldenIsolation(destinationPluginRoot) {
  const files = walkFiles(destinationPluginRoot);
  const relativeFiles = files
    .map((filePath) => path.relative(destinationPluginRoot, filePath).split(path.sep).join("/"))
    .sort();
  const badPathHits = relativeFiles
    .filter((relativePath) => /golden|oracle|eval-fixtures/iu.test(relativePath));
  const artifactPathHits = relativeFiles
    .filter((relativePath) => (
      relativePath === "evidence" ||
      relativePath.startsWith("evidence/") ||
      relativePath.includes("/evidence/") ||
      relativePath === "output" ||
      relativePath.startsWith("output/") ||
      relativePath.includes("/output/") ||
      relativePath === "scripts" ||
      relativePath.startsWith("scripts/")
    ));
  const productionRoots = ["mcp/", "references/", "skills/"];
  const badContentHits = relativeFiles
    .filter((relativePath) => productionRoots.some((root) => relativePath.startsWith(root)))
    .filter((relativePath) => {
      const body = fs.readFileSync(path.join(destinationPluginRoot, relativePath), "utf8");
      return /scripts\/eval-fixtures|ANALYTIX_FUNDS_EVAL_FAST_PATH|oracle_fast_path/iu.test(body);
    });
  const caseSpecificContentHits = relativeFiles
    .filter((relativePath) => /\.(?:json|md|mjs|yaml|yml)$/iu.test(relativePath))
    .flatMap((relativePath) => {
      const body = fs.readFileSync(path.join(destinationPluginRoot, relativePath), "utf8");
      const markers = CASE_SPECIFIC_PRODUCTION_MARKERS.filter((marker) => body.includes(marker));
      return markers.length ? [{ path: relativePath, markers }] : [];
    });
  return {
    ok: badPathHits.length === 0
      && artifactPathHits.length === 0
      && badContentHits.length === 0
      && caseSpecificContentHits.length === 0,
    bad_path_hits: badPathHits,
    artifact_path_hits: artifactPathHits,
    bad_content_hits: badContentHits,
    case_specific_content_hits: caseSpecificContentHits
  };
}

function sha256File(filePath) {
  const hash = crypto.createHash("sha256");
  hash.update(fs.readFileSync(filePath));
  return hash.digest("hex");
}

function createArchive(outRoot, archivePath, packageId, force) {
  assertInsideTmp(archivePath);
  if (fs.existsSync(archivePath)) {
    if (!force) throw new Error(`Archive already exists; pass --force to replace: ${archivePath}`);
    fs.rmSync(archivePath, { force: true });
  }
  const result = spawnSync("tar", ["-C", path.join(outRoot, "plugins"), "-czf", archivePath, packageId], {
    encoding: "utf8"
  });
  if (result.status !== 0) {
    throw new Error(text(result.stderr || result.stdout) || "tar failed");
  }
  return {
    path: archivePath,
    sha256: sha256File(archivePath),
    size_bytes: fs.statSync(archivePath).size
  };
}

function preparePackage(options) {
  const pluginRoot = path.resolve(options.pluginRoot);
  const projection = inspectFundsPluginSourceProjectionsV1(pluginRoot);
  const { packageId, packageVersion, manifest, runtimeIdentity } = projection;
  const sourceMcpClosure = inspectProductionMcpEntryClosure(pluginRoot);
  const outRoot = path.resolve(options.outRoot || path.join(os.tmpdir(), `analytix-hub-fund-plugin-${packageVersion}-source`));
  const archivePath = path.resolve(options.archivePath || path.join(os.tmpdir(), `${packageId}-${packageVersion}-hub-package.tar.gz`));
  assertInsideTmp(outRoot);
  if (fs.existsSync(outRoot)) {
    if (!options.force) throw new Error(`Output root already exists; pass --force to replace: ${outRoot}`);
    fs.rmSync(outRoot, { recursive: true, force: true });
  }
  fs.mkdirSync(path.join(outRoot, "plugins"), { recursive: true });
  const destinationPluginRoot = path.join(outRoot, "plugins", packageId);
  copyPluginSource(pluginRoot, destinationPluginRoot);
  const productionMcpPayload = validateProductionMcpPayload(destinationPluginRoot);
  const marketplacePath = writeMarketplace(outRoot, projection);
  const goldenIsolation = validateGoldenIsolation(destinationPluginRoot);
  if (!goldenIsolation.ok) {
    throw new Error(`Golden/oracle isolation failed: ${JSON.stringify(goldenIsolation)}`);
  }
  const archive = options.skipArchive ? null : createArchive(outRoot, archivePath, packageId, options.force);
  return {
    ok: true,
    plugin: `${packageId}@${packageVersion}`,
    canonical_version: packageVersion,
    manifest_version: text(manifest.version),
    server_version: runtimeIdentity.serverVersion,
    source_root: outRoot,
    plugin_root: destinationPluginRoot,
    marketplace_path: marketplacePath,
    archive,
    source_mcp_entry_closure: {
      contract: "ProductionMcpEntryClosureV1",
      files: sourceMcpClosure.files,
      edge_count: sourceMcpClosure.edges.length
    },
    production_mcp_payload: productionMcpPayload,
    golden_isolation: goldenIsolation,
    boundaries: {
      writes_only_under_tmp: true,
      hub_publish_path: false,
      install_path: false,
      runtime_cache_sync_path: false,
      system_codex_state: false
    }
  };
}

try {
  const options = parseArgs(process.argv.slice(2));
  const result = preparePackage(options);
  if (options.json) {
    console.log(JSON.stringify(result, null, 2));
  } else {
    console.log(`Prepared ${result.plugin}`);
    console.log(`source: ${result.source_root}`);
    if (result.archive) console.log(`archive: ${result.archive.path} sha256=${result.archive.sha256}`);
  }
} catch (error) {
  const message = error instanceof Error ? error.message : String(error);
  if (process.argv.includes("--json")) {
    console.log(JSON.stringify({ ok: false, error: message }, null, 2));
  } else {
    console.error(message);
  }
  process.exit(1);
}
