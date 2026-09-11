#!/usr/bin/env node

import crypto from "node:crypto";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  ALL_SKILL_NAMES,
  MARKETPLACE_NAME,
  PLUGIN_CACHE_FILES,
  PLUGIN_NAME,
  RUNTIME_CACHE_COMMIT_ORDER,
  RUNTIME_CACHE_TRANSACTION_VERSION,
  RUNTIME_SKILL_FILES,
} from "./runtime-cache-contract.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");
const HASH_RE = /^[0-9a-f]{64}$/u;
const MAX_SOURCE_FILE_BYTES = 64 * 1024 * 1024;

const USER_FACING_SKILL_CATALOG = {
  "analytix-fund-analysis": ["Analytix 涉案资金研判", "请按当前案件事实直接给出经侦研判结论、核验意见和补证建议。"],
  index: ["案件资金研判总入口", "请把当前案件资金问题路由到对应经侦核验流程。"],
  "quick-fact": ["资金事实快查", "请直接核验当前案件事实，按严格命中、模糊命中、排除对象、不能确认输出。"],
  "pair-amount-investigation": ["特定双方资金往来核验", "请核验特定双方之间资金往来金额，说明原明细、去重后金额、重复风险、异常特征、核验意见和下游追查建议。"],
  "fund-tracing": ["资金来源去向追踪", "请追踪当前案件资金来源、去向、未调取端点和资金断点。"],
  "case-context": ["案件数据范围核验", "请说明当前案件范围、数据覆盖情况和核验意见。"],
  "data-quality": ["数据质量与口径复核", "请复核导入清洗、重复交易、金额统计范围和核验意见。"],
  "case-workbench": ["专项资金核算", "请按当前案件只读统计范围完成自定义资金核算，并说明范围、总额、核验意见和补证建议。"],
  "account-dossier": ["账户资金画像", "请对当前案件指定银行卡或账户形成资金画像，并说明核验意见和补证建议。"],
  "subject-dossier": ["主体资金画像", "请对当前案件指定个人或单位主体形成资金画像，并说明核验意见和补证建议。"],
  "counterparty-analysis": ["重点对手方研判", "请研判当前案件重点对手方及关联资金通道，并给出补证优先级。"],
  "investigation-lab": ["异常资金线索研判", "请形成异常资金线索、核验方向、事实成熟度和补证建议。"],
  "full-case-analysis": ["全案资金研判", "请形成全案资金流入流出、主体、对手方、异常特征、核验意见和补证建议。"],
  "visual-evidence": ["证据图表附件", "请生成当前案件证据表、图表或附件，并使用经侦业务标题和图例。"],
  "report-builder": ["经侦研判报告", "请按基本情况、资金流入流出、重点对手方、异常特征、资金去向、核验意见、补证建议形成报告材料。"],
  "claim-review": ["报告事实结论复核", "请复核报告事实判断、金额统计范围、资金路径和图表措辞。"],
  "delivery-qc": ["研判材料交付复核", "请复核输出是否具备经侦研判结论、流水依据、异常判断、案件意义、核验意见和补证建议。"],
  "analysis-critique": ["研判完整性复核", "请复核资金研判是否遗漏方向、统计范围错误、依据不足或过度下结论。"],
  "graph-visualization": ["资金流向图谱", "请生成当前案件关系图、资金流向图或资金穿透图，并标清已有流水支持、需补证、线索和未调取端点。"],
};

function text(value) {
  return String(value == null ? "" : value).trim();
}

function parseArgs(argv) {
  const options = {
    apply: false,
    confirmLocalRemount: false,
    json: false,
    runtimeHome: "",
    selfTestRuntimeConfig: false,
    selfTestLocalRemount: false,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--apply") options.apply = true;
    else if (arg === "--confirm-local-remount") options.confirmLocalRemount = true;
    else if (arg === "--json") options.json = true;
    else if (arg === "--runtime-home") options.runtimeHome = next();
    else if (arg === "--self-test-runtime-config") options.selfTestRuntimeConfig = true;
    else if (arg === "--self-test-local-remount") options.selfTestLocalRemount = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/sync-runtime-cache.mjs [options]

Safely remount workspace plugin bytes into an existing Analytix-owned runtime.

Options:
  --apply                 Execute the journaled remount. Without this flag, this is a read-only dry run.
  --confirm-local-remount Required with --apply; confirms the existing runtime and funds MCP are stopped for this local same-version remount.
  --runtime-home <path>   Analytix runtime home. Default: ANALYTIX_AGENT_RUNTIME_HOME or ~/.analytix.
  --self-test-runtime-config Run the runtime config path-remount self-test in a temporary runtime home.
  --self-test-local-remount Run transaction, crash recovery, concurrency, source-snapshot, and idempotence tests in temporary homes.
  --json                  Print machine-readable JSON.

This tool is not a Hub publish, install, upgrade, customer update, or online hot-swap path.
`);
}

function defaultRuntimeHome() {
  return path.join(os.homedir(), ".analytix");
}

function resolveRuntimeHomeInput(options) {
  return path.resolve(
    text(options.runtimeHome)
      || text(process.env.ANALYTIX_AGENT_RUNTIME_HOME)
      || text(process.env.ANALYTIX_AGENT_RUNTIME_CODEX_HOME)
      || defaultRuntimeHome(),
  );
}

function pathInsideOrEqual(targetPath, rootPath) {
  const relative = path.relative(path.resolve(rootPath), path.resolve(targetPath));
  return relative === "" || (!!relative && !relative.startsWith("..") && !path.isAbsolute(relative));
}

function assertSafeRelative(relativePath) {
  if (
    !relativePath
    || path.isAbsolute(relativePath)
    || relativePath.includes("\\")
    || relativePath.split("/").some((part) => !part || part === "." || part === "..")
  ) {
    throw new Error(`unsafe relative runtime-cache path: ${relativePath}`);
  }
}

function assertAnalytixRuntimeHome(runtimeHome) {
  const normalized = path.resolve(runtimeHome);
  const runtimeName = path.basename(normalized);
  if (!fs.existsSync(normalized)) {
    throw new Error(`Installed runtime cache is missing for ${PLUGIN_NAME} under ${normalized}`);
  }
  if (!fs.lstatSync(normalized).isDirectory()) {
    throw new Error(`Analytix runtime home is not a directory: ${normalized}`);
  }
  if (!['.analytix', 'analytix-agent-runtime', 'analytix-agent-runtime-bridge'].includes(runtimeName) || !/analytix/iu.test(normalized)) {
    throw new Error(`Refusing to sync non-Analytix runtime home: ${runtimeHome}`);
  }
  const systemCodexHome = path.join(os.homedir(), ".codex");
  if (pathInsideOrEqual(normalized, systemCodexHome)) {
    throw new Error(`Refusing to sync inside system Codex home: ${runtimeHome}`);
  }
  const canonical = fs.realpathSync.native(normalized);
  if (fs.existsSync(systemCodexHome)) {
    const canonicalCodexHome = fs.realpathSync.native(systemCodexHome);
    if (pathInsideOrEqual(canonical, canonicalCodexHome)) {
      throw new Error(`Refusing to sync inside system Codex home: ${runtimeHome}`);
    }
  }
  return canonical;
}

function assertNoSymlinkBelow(rootPath, targetPath, expectedKind = "any") {
  const root = fs.realpathSync.native(rootPath);
  const target = path.resolve(targetPath);
  if (!pathInsideOrEqual(target, root)) {
    throw new Error(`path escapes canonical root: ${target}`);
  }
  const relative = path.relative(root, target);
  let cursor = root;
  const parts = relative ? relative.split(path.sep) : [];
  for (const part of parts) {
    cursor = path.join(cursor, part);
    let stat;
    try {
      stat = fs.lstatSync(cursor);
    } catch (error) {
      if (error && typeof error === "object" && "code" in error && error.code === "ENOENT") break;
      throw error;
    }
    if (stat.isSymbolicLink()) {
      throw new Error(`symlink is forbidden in runtime-cache path: ${cursor}`);
    }
  }
  let stat;
  try {
    stat = fs.lstatSync(target);
  } catch (error) {
    if (error && typeof error === "object" && "code" in error && error.code === "ENOENT") return;
    throw error;
  }
  if (stat.isSymbolicLink()) throw new Error(`symlink is forbidden in runtime-cache path: ${target}`);
  if (expectedKind === "file" && !stat.isFile()) throw new Error(`expected regular file: ${target}`);
  if (expectedKind === "directory" && !stat.isDirectory()) throw new Error(`expected directory: ${target}`);
}

function openNoFollow(filePath, flags, mode) {
  return fs.openSync(filePath, flags | (fs.constants.O_NOFOLLOW || 0), mode);
}

function readRegularFileOnce(rootPath, relativePath) {
  assertSafeRelative(relativePath);
  const filePath = path.join(rootPath, relativePath);
  assertNoSymlinkBelow(rootPath, filePath, "file");
  const descriptor = openNoFollow(filePath, fs.constants.O_RDONLY);
  try {
    const before = fs.fstatSync(descriptor);
    if (!before.isFile() || before.size > MAX_SOURCE_FILE_BYTES) {
      throw new Error(`source is not a bounded regular file: ${filePath}`);
    }
    const bytes = fs.readFileSync(descriptor);
    const after = fs.fstatSync(descriptor);
    if (
      before.dev !== after.dev
      || before.ino !== after.ino
      || before.size !== after.size
      || before.mtimeMs !== after.mtimeMs
      || bytes.length !== after.size
    ) {
      throw new Error(`source changed while snapshotting: ${filePath}`);
    }
    return bytes;
  } finally {
    fs.closeSync(descriptor);
  }
}

function sha256(bytes) {
  return crypto.createHash("sha256").update(bytes).digest("hex");
}

function canonicalJson(value) {
  return Buffer.from(`${JSON.stringify(value, null, 2)}\n`, "utf8");
}

function parseJsonBytes(bytes, label) {
  try {
    return JSON.parse(bytes.toString("utf8"));
  } catch (error) {
    throw new Error(`${label} is not valid JSON: ${error instanceof Error ? error.message : String(error)}`);
  }
}

function captureSourceSnapshot(sourceRoot = pluginRoot) {
  if (!fs.existsSync(sourceRoot) || !fs.lstatSync(sourceRoot).isDirectory()) {
    throw new Error(`plugin source root is missing: ${sourceRoot}`);
  }
  const canonicalRoot = fs.realpathSync.native(sourceRoot);
  const files = new Map();
  const missing = [];
  for (const relativePath of PLUGIN_CACHE_FILES) {
    try {
      files.set(relativePath, readRegularFileOnce(canonicalRoot, relativePath));
    } catch (error) {
      if (error && typeof error === "object" && "code" in error && error.code === "ENOENT") {
        missing.push(relativePath);
      } else {
        throw error;
      }
    }
  }
  if (missing.length) {
    throw new Error(`Missing source files; runtime state was not touched: ${missing.join(", ")}`);
  }
  const hash = crypto.createHash("sha256");
  for (const relativePath of PLUGIN_CACHE_FILES) {
    hash.update(relativePath);
    hash.update("\0");
    hash.update(files.get(relativePath));
    hash.update("\0");
  }
  const manifest = parseJsonBytes(files.get(".codex-plugin/plugin.json"), "plugin manifest");
  const version = text(manifest.version);
  if (!version) throw new Error("plugin version missing");
  return {
    root: canonicalRoot,
    files,
    manifest,
    version,
    workspaceSha256: hash.digest("hex"),
  };
}

function runtimeHubPluginsRoot(runtimeHome) {
  return path.join(runtimeHome, ".cache", `${MARKETPLACE_NAME}-plugins`);
}

function runtimeMarketplaceRoot(runtimeHome) {
  return path.join(runtimeHubPluginsRoot(runtimeHome), "marketplaces", MARKETPLACE_NAME);
}

function generatedMarketplacePath(runtimeHome) {
  return path.join(runtimeMarketplaceRoot(runtimeHome), ".agents", "plugins", "marketplace.json");
}

function runtimeConfigPath(runtimeHome) {
  return path.join(runtimeHome, "data", "config.json");
}

function installPolicyPath(runtimeHome) {
  return path.join(runtimeHubPluginsRoot(runtimeHome), "install-policy.json");
}

function runtimeSkillCatalogPath(runtimeHome) {
  return path.join(runtimeHubPluginsRoot(runtimeHome), "skills", "catalog.json");
}

function runtimeSkillMarkerPath(runtimeHome, skillName) {
  return path.join(runtimeHome, "skills", skillName, ".analytix-hub-skill.json");
}

function directPluginCacheRoot(runtimeHome) {
  return path.join(runtimeHome, "plugins", "cache", MARKETPLACE_NAME, PLUGIN_NAME);
}

function marketplacePluginCacheRoot(runtimeHome) {
  return path.join(runtimeMarketplaceRoot(runtimeHome), "plugins", PLUGIN_NAME);
}

function transactionControlRoot(runtimeHome) {
  return path.join(runtimeHubPluginsRoot(runtimeHome), "remount-transactions", PLUGIN_NAME);
}

function quarantineRoot(runtimeHome) {
  return path.join(runtimeHome, ".tmp", "runtime-cache-remount-quarantine", PLUGIN_NAME);
}

function marketplaceCacheName(version, sourceHash) {
  if (!HASH_RE.test(sourceHash)) throw new Error("marketplace generation requires a complete SHA-256");
  return `${version}-local-${sourceHash}`;
}

function runtimeSkillFilesFor(skillName) {
  return skillName === PLUGIN_NAME ? RUNTIME_SKILL_FILES : ["SKILL.md", "agents/openai.yaml"];
}

function unquoteYamlScalar(value) {
  const raw = text(value);
  if ((raw.startsWith('"') && raw.endsWith('"')) || (raw.startsWith("'") && raw.endsWith("'"))) {
    return raw.slice(1, -1).trim();
  }
  return raw;
}

function skillFrontmatterValue(source, key) {
  const match = source.match(new RegExp(`^${key}:\\s*([^\\n#]+?)\\s*$`, "mu"));
  return unquoteYamlScalar(match?.[1] || "");
}

function sourceSkillMetadata(snapshot, skillName) {
  const relativePath = `skills/${skillName}/SKILL.md`;
  const bytes = snapshot.files.get(relativePath);
  if (!bytes) return null;
  const source = bytes.toString("utf8");
  const [displayName, defaultPrompt] = USER_FACING_SKILL_CATALOG[skillName] || [];
  return {
    name: skillFrontmatterValue(source, "name") || skillName,
    description: skillFrontmatterValue(source, "description"),
    displayName,
    defaultPrompt,
  };
}

function directPluginMarker(plan) {
  const relativeSourcePath = `./plugins/${PLUGIN_NAME}/${path.basename(plan.marketplace_installed_root)}`;
  return {
    managedBy: MARKETPLACE_NAME,
    marketplaceName: MARKETPLACE_NAME,
    pluginName: PLUGIN_NAME,
    version: plan.version,
    packageSha256: plan.workspace_sha256,
    sourcePath: plan.marketplace_installed_root,
    source: { source: "local", path: relativeSourcePath },
    transactionVersion: RUNTIME_CACHE_TRANSACTION_VERSION,
    commitOrder: RUNTIME_CACHE_COMMIT_ORDER,
  };
}

function skillMarker(plan, skillName) {
  return {
    managedBy: MARKETPLACE_NAME,
    skillName,
    pluginName: PLUGIN_NAME,
    version: plan.version,
    skillPath: `skills/${skillName}/SKILL.md`,
    sourceKind: "plugin",
    packageSha256: plan.workspace_sha256,
    transactionVersion: RUNTIME_CACHE_TRANSACTION_VERSION,
  };
}

function desiredPluginFiles(snapshot, plan) {
  const files = new Map();
  for (const relativePath of PLUGIN_CACHE_FILES) files.set(relativePath, snapshot.files.get(relativePath));
  files.set(".analytix-hub-installed-plugin.json", canonicalJson(directPluginMarker(plan)));
  return files;
}

function desiredSkillFiles(snapshot, plan, skillName) {
  const files = new Map();
  for (const relativePath of runtimeSkillFilesFor(skillName)) {
    const sourcePath = `skills/${skillName}/${relativePath}`;
    const bytes = snapshot.files.get(sourcePath);
    if (!bytes) throw new Error(`Missing source files; runtime state was not touched: ${sourcePath}`);
    files.set(relativePath, bytes);
  }
  files.set(".analytix-hub-skill.json", canonicalJson(skillMarker(plan, skillName)));
  return files;
}

function readMetadataFile(runtimeHome, filePath, required) {
  assertNoSymlinkBelow(runtimeHome, filePath, "file");
  if (!fs.existsSync(filePath)) {
    if (required) throw new Error(`required runtime metadata is missing: ${filePath}`);
    return null;
  }
  return readRegularFileOnce(runtimeHome, path.relative(runtimeHome, filePath).split(path.sep).join("/"));
}

function patchGeneratedMarketplaceValue(value, plan) {
  const marketplace = structuredClone(value);
  const sourcePath = `./plugins/${PLUGIN_NAME}/${path.basename(plan.marketplace_installed_root)}`;
  const plugins = Array.isArray(marketplace.plugins) ? marketplace.plugins : [];
  let found = false;
  for (const entry of plugins) {
    if (text(entry.name) !== PLUGIN_NAME) continue;
    found = true;
    entry.version = plan.version;
    const source = entry.source && typeof entry.source === "object" ? entry.source : {};
    if (source.source !== "local" || source.path !== sourcePath || source.sha256 !== plan.workspace_sha256) {
      entry.source = { ...source, source: "local", path: sourcePath, sha256: plan.workspace_sha256 };
    }
    entry.category = text(plan.manifest?.interface?.category) || entry.category || "Productivity";
    entry.transactionVersion = RUNTIME_CACHE_TRANSACTION_VERSION;
  }
  if (!found) {
    marketplace.plugins = [...plugins, {
      name: PLUGIN_NAME,
      version: plan.version,
      description: text(plan.manifest?.description),
      interface: plan.manifest?.interface || {},
      category: text(plan.manifest?.interface?.category) || "Productivity",
      source: { source: "local", path: sourcePath, sha256: plan.workspace_sha256 },
      transactionVersion: RUNTIME_CACHE_TRANSACTION_VERSION,
    }];
  }
  return marketplace;
}

function patchInstallPolicyValue(value, plan, snapshot) {
  const policy = structuredClone(value);
  for (const entry of Array.isArray(policy.requiredPlugins) ? policy.requiredPlugins : []) {
    if (text(entry.pluginName) !== PLUGIN_NAME) continue;
    entry.version = plan.version;
    entry.packageSha256 = plan.workspace_sha256;
    entry.displayName = text(plan.manifest?.interface?.displayName) || entry.displayName || "Analytix 涉案资金研判";
    entry.transactionVersion = RUNTIME_CACHE_TRANSACTION_VERSION;
  }
  for (const entry of Array.isArray(policy.requiredSkills) ? policy.requiredSkills : []) {
    if (text(entry.pluginName) !== PLUGIN_NAME) continue;
    const skillName = text(entry.skillName || entry?.skill?.name);
    const metadata = sourceSkillMetadata(snapshot, skillName);
    entry.version = plan.version;
    if (metadata?.displayName) entry.displayName = metadata.displayName;
    if (metadata?.description) entry.shortDescription = metadata.description;
  }
  return policy;
}

function valueLooksLikeInstalledFundAnalysisPath(value) {
  const body = text(value).replace(/\\/gu, "/");
  return body.includes(`/plugins/cache/${MARKETPLACE_NAME}/${PLUGIN_NAME}/`)
    || body.includes(`/.cache/${MARKETPLACE_NAME}-plugins/marketplaces/${MARKETPLACE_NAME}/plugins/${PLUGIN_NAME}/`);
}

function patchRuntimeConfigValue(value, directRoot) {
  const config = structuredClone(value);
  const capabilities = config.capabilities && typeof config.capabilities === "object" ? config.capabilities : null;
  const mcp = capabilities?.mcp && typeof capabilities.mcp === "object" ? capabilities.mcp : null;
  const servers = mcp?.servers && typeof mcp.servers === "object" ? mcp.servers : null;
  const server = servers?.analytix_funds && typeof servers.analytix_funds === "object" ? servers.analytix_funds : null;
  if (!server) return config;
  const serverPath = path.join(directRoot, "mcp", "server.mjs");
  server.args = Array.isArray(server.args) ? [serverPath, ...server.args.slice(1)] : [serverPath];
  server.cwd = directRoot;
  const skills = capabilities?.skills && typeof capabilities.skills === "object" ? capabilities.skills : null;
  if (Array.isArray(skills?.roots)) {
    const skillRoot = path.join(directRoot, "skills");
    skills.roots = skills.roots.map((root) => valueLooksLikeInstalledFundAnalysisPath(root) ? skillRoot : root);
  }
  return config;
}

function patchSkillCatalogValue(value, plan, snapshot) {
  const catalog = structuredClone(value);
  const knownSkills = new Set(ALL_SKILL_NAMES);
  for (const entry of Array.isArray(catalog.skills) ? catalog.skills : []) {
    if (text(entry?.pluginName) !== PLUGIN_NAME) continue;
    const skillName = text(entry.skillName || entry?.skill?.name);
    if (!knownSkills.has(skillName)) continue;
    const metadata = sourceSkillMetadata(snapshot, skillName);
    if (!metadata) continue;
    entry.version = plan.version;
    if (typeof entry.iconUrl === "string") entry.iconUrl = entry.iconUrl.replace(/([?&]v=)[^&]+/u, `$1${plan.version}`);
    if (metadata.description) entry.shortDescription = metadata.description;
    if (metadata.displayName) entry.displayName = metadata.displayName;
    entry.interface = {
      ...(entry.interface && typeof entry.interface === "object" ? entry.interface : {}),
      ...(metadata.displayName ? { displayName: metadata.displayName } : {}),
      ...(metadata.description ? { shortDescription: metadata.description } : {}),
      ...(metadata.defaultPrompt ? { defaultPrompt: metadata.defaultPrompt } : {}),
    };
    entry.skill = {
      ...(entry.skill && typeof entry.skill === "object" ? entry.skill : {}),
      name: metadata.name,
      ...(metadata.description ? { description: metadata.description } : {}),
    };
  }
  return catalog;
}

function desiredDirectories(fileMap) {
  const directories = new Set();
  for (const relativePath of fileMap.keys()) {
    let current = path.posix.dirname(relativePath);
    while (current && current !== ".") {
      directories.add(current);
      current = path.posix.dirname(current);
    }
  }
  return directories;
}

function treeHash(fileMap, directories = desiredDirectories(fileMap)) {
  const hash = crypto.createHash("sha256");
  for (const directory of [...directories].sort()) {
    hash.update(`d\0${directory}\0`);
  }
  for (const [relativePath, bytes] of [...fileMap.entries()].sort(([left], [right]) => left.localeCompare(right))) {
    hash.update(`f\0${relativePath}\0`);
    hash.update(bytes);
    hash.update("\0");
  }
  return hash.digest("hex");
}

function readDirectoryState(rootPath) {
  if (!fs.existsSync(rootPath)) return null;
  const stat = fs.lstatSync(rootPath);
  if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error(`runtime cache root must be a regular directory: ${rootPath}`);
  const files = new Map();
  const directories = new Set();
  const visit = (currentPath, prefix) => {
    const entries = fs.readdirSync(currentPath, { withFileTypes: true }).sort((left, right) => left.name.localeCompare(right.name));
    for (const entry of entries) {
      const absolutePath = path.join(currentPath, entry.name);
      const relativePath = prefix ? `${prefix}/${entry.name}` : entry.name;
      const entryStat = fs.lstatSync(absolutePath);
      if (entryStat.isSymbolicLink()) throw new Error(`symlink is forbidden in runtime-cache tree: ${absolutePath}`);
      if (entryStat.isDirectory()) {
        directories.add(relativePath);
        visit(absolutePath, relativePath);
      } else if (entryStat.isFile()) {
        const descriptor = openNoFollow(absolutePath, fs.constants.O_RDONLY);
        try {
          files.set(relativePath, fs.readFileSync(descriptor));
        } finally {
          fs.closeSync(descriptor);
        }
      } else {
        throw new Error(`non-regular runtime-cache entry is forbidden: ${absolutePath}`);
      }
    }
  };
  visit(rootPath, "");
  return { files, directories, hash: treeHash(files, directories) };
}

function exactDirectoryMatches(rootPath, desiredFiles) {
  const state = readDirectoryState(rootPath);
  if (!state) return false;
  const desiredDirs = desiredDirectories(desiredFiles);
  if (state.files.size !== desiredFiles.size || state.directories.size !== desiredDirs.size) return false;
  for (const directory of desiredDirs) if (!state.directories.has(directory)) return false;
  for (const [relativePath, bytes] of desiredFiles) {
    const actual = state.files.get(relativePath);
    if (!actual || !actual.equals(bytes)) return false;
  }
  return true;
}

function pathHash(filePath) {
  if (!fs.existsSync(filePath)) return "missing";
  const stat = fs.lstatSync(filePath);
  if (!stat.isFile() || stat.isSymbolicLink()) throw new Error(`metadata target must be a regular file: ${filePath}`);
  const descriptor = openNoFollow(filePath, fs.constants.O_RDONLY);
  try {
    return sha256(fs.readFileSync(descriptor));
  } finally {
    fs.closeSync(descriptor);
  }
}

function listInstalledDirectories(parentPath) {
  if (!fs.existsSync(parentPath)) return [];
  const output = [];
  for (const entry of fs.readdirSync(parentPath, { withFileTypes: true })) {
    const target = path.join(parentPath, entry.name);
    const stat = fs.lstatSync(target);
    if (stat.isSymbolicLink()) throw new Error(`symlink is forbidden in installed cache: ${target}`);
    if (entry.isDirectory() && !entry.name.startsWith(".")) output.push(target);
  }
  return output.sort();
}

function preflightLayout(options, snapshot) {
  const runtimeHome = assertAnalytixRuntimeHome(resolveRuntimeHomeInput(options));
  const directParent = directPluginCacheRoot(runtimeHome);
  const marketplaceParent = marketplacePluginCacheRoot(runtimeHome);
  assertNoSymlinkBelow(runtimeHome, directParent);
  assertNoSymlinkBelow(runtimeHome, marketplaceParent);
  const previousRoots = [...listInstalledDirectories(directParent), ...listInstalledDirectories(marketplaceParent)];
  if (!previousRoots.length) {
    throw new Error(`Installed runtime cache is missing for ${PLUGIN_NAME} under ${runtimeHome}`);
  }
  const directRoot = path.join(directParent, snapshot.version);
  if (!fs.existsSync(directRoot)) {
    throw new Error(`Existing same-version direct runtime cache is required for local remount of ${PLUGIN_NAME}@${snapshot.version}: ${directRoot}`);
  }
  assertNoSymlinkBelow(runtimeHome, directRoot, "directory");
  const skillRoots = new Map();
  for (const skillName of ALL_SKILL_NAMES) {
    const skillRoot = path.join(runtimeHome, "skills", skillName);
    const mountedSkillPath = path.join(skillRoot, "SKILL.md");
    if (!fs.existsSync(mountedSkillPath)) {
      throw new Error(`Runtime required skill mount is missing for ${skillName}: ${mountedSkillPath}`);
    }
    assertNoSymlinkBelow(runtimeHome, skillRoot, "directory");
    assertNoSymlinkBelow(runtimeHome, mountedSkillPath, "file");
    skillRoots.set(skillName, skillRoot);
  }
  const pointerPath = generatedMarketplacePath(runtimeHome);
  if (!fs.existsSync(pointerPath)) throw new Error(`required runtime metadata is missing: ${pointerPath}`);
  assertNoSymlinkBelow(runtimeHome, pointerPath, "file");
  for (const optionalPath of [installPolicyPath(runtimeHome), runtimeConfigPath(runtimeHome), runtimeSkillCatalogPath(runtimeHome)]) {
    if (fs.existsSync(optionalPath)) assertNoSymlinkBelow(runtimeHome, optionalPath, "file");
  }
  return {
    runtimeHome,
    directParent,
    directRoot,
    marketplaceParent,
    marketplaceRoot: path.join(marketplaceParent, marketplaceCacheName(snapshot.version, snapshot.workspaceSha256)),
    previousRoots,
    skillRoots,
    pointerPath,
  };
}

function assertRuntimeProcessesStopped(runtimeHome, snapshot) {
  let processList;
  try {
    if (process.platform === "win32") {
      const raw = execFileSync("powershell.exe", [
        "-NoLogo",
        "-NoProfile",
        "-NonInteractive",
        "-Command",
        "Get-CimInstance Win32_Process | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress",
      ], {
        encoding: "utf8",
        maxBuffer: 8 * 1024 * 1024,
        windowsHide: true,
      });
      const rows = JSON.parse(raw);
      processList = (Array.isArray(rows) ? rows : [rows])
        .map((entry) => `${Number(entry?.ProcessId) || 0} ${text(entry?.CommandLine)}`)
        .join("\n");
    } else {
      processList = execFileSync("ps", ["-axo", "pid=,command="], {
        encoding: "utf8",
        maxBuffer: 8 * 1024 * 1024,
      });
    }
  } catch (error) {
    throw new Error(`cannot prove the Analytix runtime is stopped: ${error instanceof Error ? error.message : String(error)}`);
  }
  const directRoot = path.join(directPluginCacheRoot(runtimeHome), snapshot.version);
  const normalizedRuntimeHome = runtimeHome.replace(/\\/gu, "/").toLowerCase();
  const normalizedDirectRoot = directRoot.replace(/\\/gu, "/").toLowerCase();
  const conflicts = processList.split(/\r?\n/u).filter((line) => {
    const normalized = line.replace(/\\/gu, "/").toLowerCase();
    const pidMatch = line.match(/^\s*(\d+)/u);
    if (Number(pidMatch?.[1]) === process.pid) return false;
    return normalized.includes(normalizedDirectRoot)
      || (normalized.includes("runtime-server") && normalized.includes(normalizedRuntimeHome))
      || (normalized.includes(normalizedRuntimeHome) && normalized.includes("analytix-fund-analysis/mcp/server.mjs"));
  });
  if (conflicts.length) {
    throw new Error(`Analytix runtime or funds MCP is still running; stop it before --confirm-local-remount (process_count=${conflicts.length})`);
  }
}

function metadataDescriptor(runtimeHome, key, target, required, patcher) {
  const oldBytes = readMetadataFile(runtimeHome, target, required);
  if (!oldBytes) return null;
  const oldValue = parseJsonBytes(oldBytes, key);
  const newBytes = canonicalJson(patcher(oldValue));
  return {
    key,
    target,
    oldHash: sha256(oldBytes),
    newHash: sha256(newBytes),
    newBytes,
    changed: !oldBytes.equals(newBytes),
  };
}

function buildPlan(options, snapshot, layout) {
  const plan = {
    plugin: `${PLUGIN_NAME}@${snapshot.version}`,
    version: snapshot.version,
    manifest: snapshot.manifest,
    workspace_sha256: snapshot.workspaceSha256,
    mode: options.apply ? "apply" : "dry_run",
    runtime_home: layout.runtimeHome,
    installed_root: layout.marketplaceRoot,
    marketplace_installed_root: layout.marketplaceRoot,
    direct_installed_root: layout.directRoot,
    previous_installed_roots: layout.previousRoots,
    installed_roots: [layout.marketplaceRoot, layout.directRoot],
    allowed_write_roots: [
      layout.marketplaceParent,
      layout.directParent,
      ...layout.skillRoots.values(),
      runtimeHubPluginsRoot(layout.runtimeHome),
      path.join(layout.runtimeHome, "data"),
      quarantineRoot(layout.runtimeHome),
    ],
    write_guard: {
      requires_existing_install: true,
      requires_confirm_local_remount_for_apply: true,
      confirm_local_remount: Boolean(options.confirmLocalRemount),
      requires_runtime_and_mcp_stopped: true,
      runtime_offline_confirmation: Boolean(options.confirmLocalRemount),
      hub_publish_path: false,
      install_path: false,
      upgrade_path: false,
      global_atomicity: "not_available_across_independent_directories",
      commit_marker: RUNTIME_CACHE_COMMIT_ORDER,
      runtime_home_scope: "Analytix-owned runtime only",
    },
  };
  const pluginFiles = desiredPluginFiles(snapshot, plan);
  const directoryPlans = [
    { key: "marketplace_generation", kind: "generation", target: layout.marketplaceRoot, files: pluginFiles },
    { key: "direct_plugin", kind: "swap", target: layout.directRoot, files: pluginFiles },
    ...ALL_SKILL_NAMES.map((skillName) => ({
      key: `skill:${skillName}`,
      kind: "swap",
      target: layout.skillRoots.get(skillName),
      files: desiredSkillFiles(snapshot, plan, skillName),
      skillName,
    })),
  ];
  for (const entry of directoryPlans) {
    entry.newHash = treeHash(entry.files);
    const oldState = readDirectoryState(entry.target);
    entry.oldHash = oldState?.hash || "missing";
    entry.changed = !exactDirectoryMatches(entry.target, entry.files);
    if (entry.kind === "generation" && oldState && entry.changed) {
      throw new Error(`content-addressed marketplace generation is corrupt or collided: ${entry.target}`);
    }
  }
  const metadata = [
    metadataDescriptor(layout.runtimeHome, "runtime_install_policy", installPolicyPath(layout.runtimeHome), false, (value) => patchInstallPolicyValue(value, plan, snapshot)),
    metadataDescriptor(layout.runtimeHome, "runtime_config", runtimeConfigPath(layout.runtimeHome), false, (value) => patchRuntimeConfigValue(value, layout.directRoot)),
    metadataDescriptor(layout.runtimeHome, "runtime_skill_catalog", runtimeSkillCatalogPath(layout.runtimeHome), false, (value) => patchSkillCatalogValue(value, plan, snapshot)),
    metadataDescriptor(layout.runtimeHome, "runtime_generated_marketplace", layout.pointerPath, true, (value) => patchGeneratedMarketplaceValue(value, plan)),
  ].filter(Boolean);
  const pointer = metadata.find((entry) => entry.key === "runtime_generated_marketplace");
  if (!pointer) throw new Error("generated marketplace pointer is required as the last commit marker");
  const staleDirect = listInstalledDirectories(layout.directParent).filter((entry) => entry !== layout.directRoot);
  const staleMarketplace = listInstalledDirectories(layout.marketplaceParent).filter((entry) => entry !== layout.marketplaceRoot);
  plan.directoryPlans = directoryPlans;
  plan.metadataPlans = metadata;
  plan.pointerPlan = pointer;
  plan.staleDirect = staleDirect;
  plan.staleMarketplace = staleMarketplace;
  plan.items = directoryPlans.flatMap((directoryPlan) => [...directoryPlan.files.keys()]
    .filter((relativePath) => relativePath !== ".analytix-hub-installed-plugin.json" && relativePath !== ".analytix-hub-skill.json")
    .map((relativePath) => ({
      scope: directoryPlan.skillName ? "runtime_skill" : "plugin_cache",
      skill: directoryPlan.skillName,
      action: directoryPlan.changed ? "copy" : "up_to_date",
      src: path.join(snapshot.root, directoryPlan.skillName ? `skills/${directoryPlan.skillName}/${relativePath}` : relativePath),
      dest: path.join(directoryPlan.target, relativePath),
    })));
  const metadataAction = (key, missingAction) => {
    const entry = metadata.find((candidate) => candidate.key === key);
    if (!entry) return { scope: key, action: missingAction, dest: key };
    return { scope: key, action: entry.changed ? (options.apply ? "patch" : "would_patch") : "up_to_date", dest: entry.target };
  };
  plan.catalog_patch = metadataAction("runtime_skill_catalog", "missing_runtime_catalog");
  plan.install_policy_patch = metadataAction("runtime_install_policy", "missing_install_policy");
  plan.runtime_config_patch = metadataAction("runtime_config", "missing_runtime_config");
  plan.generated_marketplace_patch = metadataAction("runtime_generated_marketplace", "missing_generated_marketplace");
  plan.skill_marker_patches = ALL_SKILL_NAMES.map((skillName) => {
    const directoryPlan = directoryPlans.find((entry) => entry.key === `skill:${skillName}`);
    return { scope: "runtime_skill_marker", skill: skillName, action: directoryPlan.changed ? (options.apply ? "patch" : "would_patch") : "up_to_date", dest: runtimeSkillMarkerPath(layout.runtimeHome, skillName) };
  });
  plan.installed_plugin_marker_patches = directoryPlans.filter((entry) => !entry.skillName).map((entry) => ({
    scope: "runtime_installed_plugin_marker",
    action: entry.changed ? (options.apply ? "patch" : "would_patch") : "up_to_date",
    dest: path.join(entry.target, ".analytix-hub-installed-plugin.json"),
  }));
  plan.stale_cache_patch = { scope: "runtime_stale_plugin_cache", action: staleDirect.length ? (options.apply ? "quarantine" : "would_quarantine") : "up_to_date", dest: layout.directParent, moved: staleDirect };
  plan.stale_marketplace_cache_patch = { scope: "runtime_stale_marketplace_plugin_cache", action: staleMarketplace.length ? (options.apply ? "quarantine" : "would_quarantine") : "up_to_date", dest: layout.marketplaceParent, moved: staleMarketplace };
  plan.noop = directoryPlans.every((entry) => !entry.changed)
    && metadata.every((entry) => !entry.changed)
    && !staleDirect.length
    && !staleMarketplace.length;
  return plan;
}

function fsyncDirectory(directoryPath) {
  let descriptor;
  try {
    descriptor = fs.openSync(directoryPath, fs.constants.O_RDONLY);
    fs.fsyncSync(descriptor);
  } catch (error) {
    if (!error || typeof error !== "object" || !["EINVAL", "ENOTSUP", "EBADF", "EPERM"].includes(error.code)) throw error;
  } finally {
    if (descriptor !== undefined) fs.closeSync(descriptor);
  }
}

function ensureDirectory(directoryPath, mode = 0o700) {
  fs.mkdirSync(directoryPath, { recursive: true, mode });
  const stat = fs.lstatSync(directoryPath);
  if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error(`unsafe transaction directory: ${directoryPath}`);
}

function writeFileExclusive(filePath, bytes, mode = 0o600, durable = true) {
  const descriptor = openNoFollow(filePath, fs.constants.O_CREAT | fs.constants.O_EXCL | fs.constants.O_WRONLY, mode);
  try {
    fs.writeFileSync(descriptor, bytes);
    if (durable) fs.fsyncSync(descriptor);
  } finally {
    fs.closeSync(descriptor);
  }
}

function atomicWriteFile(filePath, bytes, mode = 0o600) {
  ensureDirectory(path.dirname(filePath));
  const temporaryPath = path.join(path.dirname(filePath), `.${path.basename(filePath)}.${process.pid}.${crypto.randomBytes(8).toString("hex")}.tmp`);
  writeFileExclusive(temporaryPath, bytes, mode);
  fs.renameSync(temporaryPath, filePath);
  fsyncDirectory(path.dirname(filePath));
}

function writeExactDirectory(rootPath, files, durable = true) {
  if (fs.existsSync(rootPath)) throw new Error(`transaction staging path already exists: ${rootPath}`);
  fs.mkdirSync(rootPath, { mode: 0o700 });
  const directories = [...desiredDirectories(files)].sort((left, right) => left.split("/").length - right.split("/").length || left.localeCompare(right));
  for (const relativePath of directories) fs.mkdirSync(path.join(rootPath, relativePath), { mode: 0o700 });
  for (const [relativePath, bytes] of [...files.entries()].sort(([left], [right]) => left.localeCompare(right))) {
    writeFileExclusive(path.join(rootPath, relativePath), bytes, 0o600, durable);
  }
  if (durable) {
    for (const relativePath of [...directories].sort((left, right) => right.split("/").length - left.split("/").length || right.localeCompare(left))) {
      fsyncDirectory(path.join(rootPath, relativePath));
    }
    fsyncDirectory(rootPath);
    fsyncDirectory(path.dirname(rootPath));
  }
  const state = readDirectoryState(rootPath);
  const expectedHash = treeHash(files);
  if (!state || state.hash !== expectedHash) throw new Error(`staged runtime-cache tree failed readback: ${rootPath}`);
}

function transactionPaths(runtimeHome) {
  const controlRoot = transactionControlRoot(runtimeHome);
  return {
    controlRoot,
    lockDirectory: path.join(controlRoot, ".lock"),
    journalPath: path.join(controlRoot, "journal.json"),
  };
}

function isPidAlive(pid) {
  if (!Number.isSafeInteger(pid) || pid <= 0) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return Boolean(error && typeof error === "object" && error.code === "EPERM");
  }
}

function acquireExclusiveLock(runtimeHome) {
  const paths = transactionPaths(runtimeHome);
  assertNoSymlinkBelow(runtimeHome, paths.controlRoot);
  ensureDirectory(paths.controlRoot);
  const owner = {
    version: RUNTIME_CACHE_TRANSACTION_VERSION,
    ownerId: crypto.randomBytes(16).toString("hex"),
    pid: process.pid,
    hostname: os.hostname(),
    createdAt: new Date().toISOString(),
  };
  const create = () => {
    fs.mkdirSync(paths.lockDirectory, { mode: 0o700 });
    atomicWriteFile(path.join(paths.lockDirectory, "owner.json"), canonicalJson(owner));
    fsyncDirectory(paths.controlRoot);
  };
  try {
    create();
  } catch (error) {
    if (!error || typeof error !== "object" || error.code !== "EEXIST") throw error;
    const lockStat = fs.lstatSync(paths.lockDirectory);
    if (!lockStat.isDirectory() || lockStat.isSymbolicLink()) {
      throw new Error("runtime-cache remount lock is not a regular directory");
    }
    const ownerPath = path.join(paths.lockDirectory, "owner.json");
    let current;
    let currentOwnerBytes = Buffer.alloc(0);
    try {
      const ownerStat = fs.lstatSync(ownerPath);
      if (!ownerStat.isFile() || ownerStat.isSymbolicLink()) throw new Error("lock owner is not a regular file");
      currentOwnerBytes = fs.readFileSync(ownerPath);
      current = parseJsonBytes(currentOwnerBytes, "runtime-cache lock owner");
    } catch {
      if (Date.now() - lockStat.mtimeMs < 30_000) {
        throw new Error("runtime-cache remount lock is initializing; retry after the bounded owner-write grace period");
      }
      current = {
        ownerId: `ownerless-${sha256(Buffer.from(`${lockStat.dev}:${lockStat.ino}:${lockStat.mtimeMs}`, "utf8"))}`,
        pid: -1,
        hostname: os.hostname(),
      };
      currentOwnerBytes = Buffer.from(current.ownerId, "utf8");
    }
    if (current.hostname !== os.hostname() || isPidAlive(Number(current.pid))) {
      const boundedOwner = /^[0-9a-f]{32}$/u.test(text(current.ownerId)) ? text(current.ownerId) : "invalid";
      throw new Error(`runtime-cache remount lock is busy (owner=${boundedOwner})`);
    }
    const staleOwnerId = /^[0-9a-f]{32}$/u.test(text(current.ownerId))
      ? text(current.ownerId)
      : `invalid-${sha256(currentOwnerBytes).slice(0, 32)}`;
    const stalePath = path.join(paths.controlRoot, `.stale-lock-${staleOwnerId}`);
    if (fs.existsSync(stalePath)) throw new Error(`stale lock quarantine already exists: ${stalePath}`);
    fs.renameSync(paths.lockDirectory, stalePath);
    fsyncDirectory(paths.controlRoot);
    create();
  }
  return { ...paths, owner };
}

function releaseExclusiveLock(lock) {
  const ownerPath = path.join(lock.lockDirectory, "owner.json");
  const current = parseJsonBytes(fs.readFileSync(ownerPath), "runtime-cache lock owner");
  if (current.ownerId !== lock.owner.ownerId) throw new Error("runtime-cache lock ownership changed before release");
  fs.rmSync(lock.lockDirectory, { recursive: true, force: false });
  fsyncDirectory(lock.controlRoot);
}

function safeEphemeralPath(runtimeHome, targetPath, transactionId) {
  if (!pathInsideOrEqual(targetPath, runtimeHome)) throw new Error(`transaction path escapes runtime home: ${targetPath}`);
  if (!targetPath.includes(transactionId)) throw new Error(`transaction cleanup path lacks transaction id: ${targetPath}`);
}

function removeEphemeral(runtimeHome, targetPath, transactionId) {
  if (!fs.existsSync(targetPath)) return;
  safeEphemeralPath(runtimeHome, targetPath, transactionId);
  const stat = fs.lstatSync(targetPath);
  if (stat.isSymbolicLink()) throw new Error(`refusing to remove symlink transaction path: ${targetPath}`);
  fs.rmSync(targetPath, { recursive: stat.isDirectory(), force: false });
  fsyncDirectory(path.dirname(targetPath));
}

function transactionRecord(plan) {
  const transactionId = `${Date.now().toString(36)}-${crypto.randomBytes(12).toString("hex")}`;
  const directoryRecords = plan.directoryPlans.map((entry) => ({
    key: entry.key,
    kind: entry.kind,
    target: entry.target,
    stage: path.join(path.dirname(entry.target), `.analytix-remount-stage-${transactionId}-${entry.key.replace(/[^a-z0-9]+/giu, "-")}`),
    backup: entry.kind === "swap" ? path.join(path.dirname(entry.target), `.analytix-remount-backup-${transactionId}-${entry.key.replace(/[^a-z0-9]+/giu, "-")}`) : "",
    oldHash: entry.oldHash,
    newHash: entry.newHash,
    changed: entry.changed,
    preexisting: fs.existsSync(entry.target),
  }));
  const metadataRecords = plan.metadataPlans.map((entry) => ({
    key: entry.key,
    target: entry.target,
    stage: path.join(path.dirname(entry.target), `.${path.basename(entry.target)}.analytix-remount-stage-${transactionId}`),
    backup: path.join(path.dirname(entry.target), `.${path.basename(entry.target)}.analytix-remount-backup-${transactionId}`),
    oldHash: entry.oldHash,
    newHash: entry.newHash,
    changed: entry.changed,
  }));
  const quarantineBase = path.join(quarantineRoot(plan.runtime_home), transactionId);
  const stale = [
    ...plan.staleDirect.map((source) => ({ kind: "direct", source })),
    ...plan.staleMarketplace.map((source) => ({ kind: "marketplace", source })),
  ].map((entry) => {
    const state = readDirectoryState(entry.source);
    if (!state) throw new Error(`stale cache disappeared during plan: ${entry.source}`);
    return {
      ...entry,
      hash: state.hash,
      destination: path.join(quarantineBase, `${entry.kind}-${path.basename(entry.source)}-${state.hash}`),
    };
  });
  return {
    version: RUNTIME_CACHE_TRANSACTION_VERSION,
    transactionId,
    state: "prepared",
    lastMutation: "journal_prepared",
    runtimeHome: plan.runtime_home,
    plugin: plan.plugin,
    pluginVersion: plan.version,
    workspaceSha256: plan.workspace_sha256,
    commitOrder: RUNTIME_CACHE_COMMIT_ORDER,
    marketplacePointerKey: "runtime_generated_marketplace",
    directories: directoryRecords,
    metadata: metadataRecords,
    stale,
    quarantineBase,
  };
}

function writeJournal(journalPath, journal) {
  atomicWriteFile(journalPath, canonicalJson(journal));
}

class SimulatedCrash extends Error {
  constructor(point) {
    super(`simulated crash at ${point}`);
    this.name = "SimulatedCrash";
  }
}

function checkpoint(context, point) {
  context.journal.lastMutation = point;
  writeJournal(context.lock.journalPath, context.journal);
  if (context.options.faultPoint === point) throw new SimulatedCrash(point);
}

function assertExactKeys(value, allowedKeys, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error(`${label} must be an object`);
  const allowed = new Set(allowedKeys);
  const unknown = Object.keys(value).filter((key) => !allowed.has(key));
  if (unknown.length) throw new Error(`${label} has unknown fields: ${unknown.join(",")}`);
}

function validateJournal(runtimeHome, journal) {
  assertExactKeys(journal, [
    "version", "transactionId", "state", "lastMutation", "runtimeHome", "plugin", "pluginVersion",
    "workspaceSha256", "commitOrder", "marketplacePointerKey", "directories", "metadata", "stale", "quarantineBase",
  ], "runtime-cache remount journal");
  if (
    journal?.version !== RUNTIME_CACHE_TRANSACTION_VERSION
    || !/^[a-z0-9]+-[0-9a-f]{24}$/u.test(text(journal.transactionId))
    || journal.runtimeHome !== runtimeHome
    || journal.commitOrder !== RUNTIME_CACHE_COMMIT_ORDER
    || !["prepared", "committed"].includes(journal.state)
    || journal.plugin !== `${PLUGIN_NAME}@${journal.pluginVersion}`
    || !HASH_RE.test(text(journal.workspaceSha256))
    || !Array.isArray(journal.directories)
    || !Array.isArray(journal.metadata)
    || !Array.isArray(journal.stale)
  ) {
    throw new Error("runtime-cache remount journal is invalid; refusing recovery");
  }
  for (const entry of journal.directories) {
    assertExactKeys(entry, ["key", "kind", "target", "stage", "backup", "oldHash", "newHash", "changed", "preexisting"], "journal directory record");
    if (!['generation', 'swap'].includes(entry.kind) || !HASH_RE.test(entry.newHash) || !['missing'].includes(entry.oldHash) && !HASH_RE.test(entry.oldHash)) {
      throw new Error("journal directory hash or kind is invalid");
    }
  }
  for (const entry of journal.metadata) {
    assertExactKeys(entry, ["key", "target", "stage", "backup", "oldHash", "newHash", "changed"], "journal metadata record");
    if (!HASH_RE.test(entry.oldHash) || !HASH_RE.test(entry.newHash)) throw new Error("journal metadata hash is invalid");
  }
  for (const entry of [...journal.directories, ...journal.metadata]) {
    for (const key of ["target", "stage", "backup"]) {
      if (!entry[key]) continue;
      if (!pathInsideOrEqual(entry[key], runtimeHome)) throw new Error(`journal ${key} escapes runtime home`);
      if ((key === "stage" || key === "backup") && !entry[key].includes(journal.transactionId)) {
        throw new Error(`journal ${key} lacks transaction binding`);
      }
    }
  }
  if (!pathInsideOrEqual(journal.quarantineBase, runtimeHome) || !journal.quarantineBase.includes(journal.transactionId)) {
    throw new Error("journal quarantine path is outside the transaction authority");
  }
  for (const entry of journal.stale) {
    assertExactKeys(entry, ["kind", "source", "hash", "destination"], "journal stale-cache record");
    if (
      !pathInsideOrEqual(entry.source, runtimeHome)
      || !pathInsideOrEqual(entry.destination, runtimeHome)
      || !entry.destination.includes(journal.transactionId)
      || !HASH_RE.test(text(entry.hash))
    ) {
      throw new Error("journal stale-cache quarantine record is invalid");
    }
  }
  return journal;
}

function classifyMetadata(entry) {
  const targetHash = pathHash(entry.target);
  const backupHash = pathHash(entry.backup);
  if (targetHash === entry.newHash) return "new";
  if (targetHash === entry.oldHash && backupHash === "missing") return "old";
  if (targetHash === "missing" && backupHash === entry.oldHash) return "old_interrupted";
  throw new Error(`metadata CAS state is indeterminate for ${entry.key}`);
}

function directoryHashOrMissing(targetPath) {
  const state = readDirectoryState(targetPath);
  return state?.hash || "missing";
}

function classifyDirectory(entry) {
  const targetHash = directoryHashOrMissing(entry.target);
  const backupHash = entry.backup ? directoryHashOrMissing(entry.backup) : "missing";
  if (targetHash === entry.newHash) return "new";
  if (targetHash === entry.oldHash && backupHash === "missing") return "old";
  if (targetHash === "missing" && backupHash === entry.oldHash) return "old_interrupted";
  if (entry.kind === "generation" && entry.oldHash === "missing" && targetHash === "missing") return "old";
  throw new Error(`directory CAS state is indeterminate for ${entry.key}`);
}

function moveDirectoryToQuarantine(runtimeHome, source, destination) {
  if (!fs.existsSync(source)) return;
  assertNoSymlinkBelow(runtimeHome, source, "directory");
  assertNoSymlinkBelow(runtimeHome, path.dirname(destination));
  ensureDirectory(path.dirname(destination));
  if (fs.existsSync(destination)) throw new Error(`quarantine destination already exists: ${destination}`);
  fs.renameSync(source, destination);
  fsyncDirectory(path.dirname(source));
  fsyncDirectory(path.dirname(destination));
}

function rollbackMetadata(runtimeHome, entry, transactionId) {
  if (!entry.changed) return;
  const state = classifyMetadata(entry);
  if (state === "old") {
    removeEphemeral(runtimeHome, entry.stage, transactionId);
    return;
  }
  if (state === "old_interrupted") {
    fs.renameSync(entry.backup, entry.target);
    fsyncDirectory(path.dirname(entry.target));
    removeEphemeral(runtimeHome, entry.stage, transactionId);
    return;
  }
  const discard = `${entry.stage}.rollback-new`;
  if (fs.existsSync(discard)) throw new Error(`rollback discard already exists: ${discard}`);
  fs.renameSync(entry.target, discard);
  fs.renameSync(entry.backup, entry.target);
  fsyncDirectory(path.dirname(entry.target));
  removeEphemeral(runtimeHome, discard, transactionId);
  removeEphemeral(runtimeHome, entry.stage, transactionId);
}

function rollbackDirectory(runtimeHome, entry, journal) {
  if (!entry.changed) return;
  const state = classifyDirectory(entry);
  if (entry.kind === "generation") {
    if (state === "new" && !entry.preexisting) {
      moveDirectoryToQuarantine(runtimeHome, entry.target, path.join(journal.quarantineBase, `uncommitted-generation-${entry.newHash}`));
    }
    removeEphemeral(runtimeHome, entry.stage, journal.transactionId);
    return;
  }
  if (state === "old") {
    removeEphemeral(runtimeHome, entry.stage, journal.transactionId);
    return;
  }
  if (state === "old_interrupted") {
    fs.renameSync(entry.backup, entry.target);
    fsyncDirectory(path.dirname(entry.target));
    removeEphemeral(runtimeHome, entry.stage, journal.transactionId);
    return;
  }
  const destination = path.join(journal.quarantineBase, `rolled-back-${entry.key.replace(/[^a-z0-9]+/giu, "-")}-${entry.newHash}`);
  moveDirectoryToQuarantine(runtimeHome, entry.target, destination);
  fs.renameSync(entry.backup, entry.target);
  fsyncDirectory(path.dirname(entry.target));
  removeEphemeral(runtimeHome, entry.stage, journal.transactionId);
}

function performStaleQuarantine(runtimeHome, entry) {
  const sourceHash = directoryHashOrMissing(entry.source);
  const destinationHash = directoryHashOrMissing(entry.destination);
  if (sourceHash === "missing" && destinationHash === entry.hash) return;
  if (sourceHash === entry.hash && destinationHash === "missing") {
    moveDirectoryToQuarantine(runtimeHome, entry.source, entry.destination);
    return;
  }
  throw new Error(`stale-cache quarantine CAS mismatch: ${entry.source}`);
}

function cleanupCommitted(runtimeHome, journal) {
  for (const entry of journal.stale) performStaleQuarantine(runtimeHome, entry);
  for (const entry of journal.directories) {
    removeEphemeral(runtimeHome, entry.stage, journal.transactionId);
    if (entry.backup && fs.existsSync(entry.backup)) {
      const backupHash = directoryHashOrMissing(entry.backup);
      const destination = path.join(journal.quarantineBase, `pre-remount-${entry.key.replace(/[^a-z0-9]+/giu, "-")}-${backupHash}`);
      moveDirectoryToQuarantine(runtimeHome, entry.backup, destination);
    }
  }
  for (const entry of journal.metadata) {
    removeEphemeral(runtimeHome, entry.stage, journal.transactionId);
    removeEphemeral(runtimeHome, entry.backup, journal.transactionId);
  }
}

function recoverExistingTransaction(lock) {
  if (!fs.existsSync(lock.journalPath)) return { recovered: false, disposition: "none" };
  const journalStat = fs.lstatSync(lock.journalPath);
  if (!journalStat.isFile() || journalStat.isSymbolicLink()) {
    throw new Error("runtime-cache remount journal is not a regular file");
  }
  if (!text(lock.owner.runtimeHome)) throw new Error("runtime-cache recovery lock lacks its canonical runtime home");
  const journal = validateJournal(lock.owner.runtimeHome, parseJsonBytes(fs.readFileSync(lock.journalPath), "runtime-cache remount journal"));
  const runtimeHome = journal.runtimeHome;
  const pointer = journal.metadata.find((entry) => entry.key === journal.marketplacePointerKey);
  if (!pointer) throw new Error("runtime-cache journal has no marketplace pointer commit record");
  const pointerState = classifyMetadata(pointer);
  const pointerChanged = pointer.oldHash !== pointer.newHash;
  const transactionCommitted = pointerChanged
    ? pointerState === "new"
    : journal.state === "committed";
  if (transactionCommitted) {
    for (const entry of journal.directories) {
      if (directoryHashOrMissing(entry.target) !== entry.newHash) throw new Error(`committed transaction directory mismatch: ${entry.key}`);
    }
    for (const entry of journal.metadata) {
      if (pathHash(entry.target) !== entry.newHash) throw new Error(`committed transaction metadata mismatch: ${entry.key}`);
    }
    cleanupCommitted(runtimeHome, journal);
    fs.rmSync(lock.journalPath);
    fsyncDirectory(path.dirname(lock.journalPath));
    return { recovered: true, disposition: "committed_forward" };
  }
  const rollbackErrors = [];
  for (const entry of [...journal.metadata].reverse()) {
    try {
      rollbackMetadata(runtimeHome, entry, journal.transactionId);
    } catch (error) {
      rollbackErrors.push(`${entry.key}: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
  for (const entry of [...journal.directories].reverse()) {
    try {
      rollbackDirectory(runtimeHome, entry, journal);
    } catch (error) {
      rollbackErrors.push(`${entry.key}: ${error instanceof Error ? error.message : String(error)}`);
    }
  }
  if (rollbackErrors.length) {
    throw new Error(`precommit rollback is incomplete; journal retained: ${rollbackErrors.join("; ")}`);
  }
  fs.rmSync(lock.journalPath);
  fsyncDirectory(path.dirname(lock.journalPath));
  return { recovered: true, disposition: "rolled_back" };
}

function stageTransaction(context, plan) {
  const directoryByKey = new Map(plan.directoryPlans.map((entry) => [entry.key, entry]));
  for (const record of context.journal.directories) {
    if (!record.changed) continue;
    const desired = directoryByKey.get(record.key);
    writeExactDirectory(record.stage, desired.files, !context.options.skipDataFsyncForTest);
    checkpoint(context, `after_stage:${record.key}`);
  }
  const metadataByKey = new Map(plan.metadataPlans.map((entry) => [entry.key, entry]));
  for (const record of context.journal.metadata) {
    if (!record.changed) continue;
    writeFileExclusive(record.stage, metadataByKey.get(record.key).newBytes, 0o600, !context.options.skipDataFsyncForTest);
    if (!context.options.skipDataFsyncForTest) fsyncDirectory(path.dirname(record.stage));
    checkpoint(context, `after_metadata_stage:${record.key}`);
  }
}

function publishDirectory(context, record) {
  if (!record.changed) return;
  if (record.kind === "generation") {
    if (directoryHashOrMissing(record.target) !== "missing") throw new Error(`generation appeared after preflight: ${record.target}`);
    fs.renameSync(record.stage, record.target);
    fsyncDirectory(path.dirname(record.target));
    checkpoint(context, "after_generation_published");
    return;
  }
  if (directoryHashOrMissing(record.target) !== record.oldHash || fs.existsSync(record.backup)) {
    throw new Error(`directory old-hash CAS failed: ${record.key}`);
  }
  fs.renameSync(record.target, record.backup);
  fsyncDirectory(path.dirname(record.target));
  if (directoryHashOrMissing(record.backup) !== record.oldHash) {
    throw new Error(`directory changed during old-hash CAS: ${record.key}`);
  }
  checkpoint(context, record.key === "direct_plugin" ? "after_direct_backup_moved" : `after_skill_backup_moved:${record.key.slice(6)}`);
  fs.renameSync(record.stage, record.target);
  fsyncDirectory(path.dirname(record.target));
  checkpoint(context, record.key === "direct_plugin" ? "after_direct_published" : `after_skill_published:${record.key.slice(6)}`);
}

function publishMetadata(context, record) {
  if (!record.changed) return;
  if (pathHash(record.target) !== record.oldHash || fs.existsSync(record.backup)) {
    throw new Error(`metadata old-hash CAS failed: ${record.key}`);
  }
  fs.renameSync(record.target, record.backup);
  fsyncDirectory(path.dirname(record.target));
  if (pathHash(record.backup) !== record.oldHash) {
    throw new Error(`metadata changed during old-hash CAS: ${record.key}`);
  }
  checkpoint(context, `after_metadata_backup_moved:${record.key}`);
  fs.renameSync(record.stage, record.target);
  fsyncDirectory(path.dirname(record.target));
  checkpoint(context, record.key === "runtime_generated_marketplace" ? "after_marketplace_pointer_published" : `after_metadata_published:${record.key}`);
}

function executeTransaction(options, plan, lock, recovery) {
  if (plan.noop) return { ...plan, recovery, transaction: { action: "up_to_date", commit_order: RUNTIME_CACHE_COMMIT_ORDER } };
  const journal = transactionRecord(plan);
  const context = { options, plan, lock, journal };
  writeJournal(lock.journalPath, journal);
  if (options.faultPoint === "after_journal_prepared") throw new SimulatedCrash("after_journal_prepared");
  try {
    stageTransaction(context, plan);
    for (const record of journal.directories) publishDirectory(context, record);
    for (const record of journal.metadata.filter((entry) => entry.key !== journal.marketplacePointerKey)) publishMetadata(context, record);
    for (const record of journal.directories) {
      if (directoryHashOrMissing(record.target) !== record.newHash) throw new Error(`precommit directory readback failed: ${record.key}`);
    }
    for (const record of journal.metadata.filter((entry) => entry.key !== journal.marketplacePointerKey)) {
      if (pathHash(record.target) !== record.newHash) throw new Error(`precommit metadata readback failed: ${record.key}`);
    }
    const pointer = journal.metadata.find((entry) => entry.key === journal.marketplacePointerKey);
    publishMetadata(context, pointer);
    journal.state = "committed";
    checkpoint(context, "after_commit_journal");
    let quarantined = 0;
    for (const entry of journal.stale) {
      performStaleQuarantine(journal.runtimeHome, entry);
      quarantined += 1;
      if (quarantined === 1) checkpoint(context, "after_first_quarantine");
    }
    cleanupCommitted(journal.runtimeHome, journal);
    fs.rmSync(lock.journalPath);
    fsyncDirectory(path.dirname(lock.journalPath));
    return {
      ...plan,
      recovery,
      transaction: {
        action: "committed",
        version: RUNTIME_CACHE_TRANSACTION_VERSION,
        transaction_id: journal.transactionId,
        commit_order: RUNTIME_CACHE_COMMIT_ORDER,
        source_snapshot_sha256: journal.workspaceSha256,
        quarantined_count: journal.stale.length,
      },
    };
  } catch (error) {
    if (error instanceof SimulatedCrash && options.simulateCrash) throw error;
    try {
      recoverExistingTransaction(lock);
    } catch (recoveryError) {
      throw new Error(`${error instanceof Error ? error.message : String(error)}; recovery failed closed: ${recoveryError instanceof Error ? recoveryError.message : String(recoveryError)}`);
    }
    throw error;
  }
}

function assertApplyConfirmation(options) {
  if (options.apply && !options.confirmLocalRemount) {
    throw new Error("Refusing --apply without --confirm-local-remount; local remount requires the existing Analytix runtime and funds MCP to be stopped");
  }
}

function executeSync(options) {
  assertApplyConfirmation(options);
  const sourceSnapshot = captureSourceSnapshot(options.sourceRoot || pluginRoot);
  if (typeof options.afterSourceSnapshot === "function") options.afterSourceSnapshot(sourceSnapshot);
  if (!options.apply) {
    const layout = preflightLayout(options, sourceSnapshot);
    return buildPlan(options, sourceSnapshot, layout);
  }
  const runtimeHome = assertAnalytixRuntimeHome(resolveRuntimeHomeInput(options));
  assertRuntimeProcessesStopped(runtimeHome, sourceSnapshot);
  const controlPaths = transactionPaths(runtimeHome);
  assertNoSymlinkBelow(runtimeHome, controlPaths.controlRoot);
  const recoveryPending = fs.existsSync(controlPaths.journalPath);
  if (recoveryPending) {
    assertNoSymlinkBelow(runtimeHome, controlPaths.journalPath, "file");
  } else {
    preflightLayout(options, sourceSnapshot);
  }
  const lock = acquireExclusiveLock(runtimeHome);
  lock.owner.runtimeHome = runtimeHome;
  try {
    const recovery = recoverExistingTransaction(lock);
    const layout = preflightLayout(options, sourceSnapshot);
    const plan = buildPlan(options, sourceSnapshot, layout);
    if (typeof options.afterPlan === "function") options.afterPlan(plan);
    return executeTransaction({ ...options, sourceSnapshot }, plan, lock, recovery);
  } finally {
    releaseExclusiveLock(lock);
  }
}

function summarizeActions(items) {
  const list = Array.isArray(items) ? items : [];
  if (!list.length) return "not_checked";
  const counts = new Map();
  for (const item of list) counts.set(item.action, (counts.get(item.action) || 0) + 1);
  return [...counts.entries()].map(([action, count]) => `${action}:${count}`).join(", ");
}

function printableResult(result) {
  const copyCount = result.items.filter((item) => item.action === "copy").length;
  const missingCount = result.items.filter((item) => item.action === "missing_source").length;
  const { directoryPlans: _directoryPlans, metadataPlans: _metadataPlans, pointerPlan: _pointerPlan, ...publicResult } = result;
  return {
    ...publicResult,
    summary: {
      copy_count: copyCount,
      missing_source_count: missingCount,
      up_to_date_count: result.items.filter((item) => item.action === "up_to_date").length,
      runtime_skill_catalog: result.catalog_patch?.action || "not_checked",
      runtime_install_policy: result.install_policy_patch?.action || "not_checked",
      runtime_config: result.runtime_config_patch?.action || "not_checked",
      runtime_skill_marker: summarizeActions(result.skill_marker_patches),
      runtime_installed_plugin_marker: summarizeActions(result.installed_plugin_marker_patches),
      runtime_generated_marketplace: result.generated_marketplace_patch?.action || "not_checked",
      runtime_stale_plugin_cache: result.stale_cache_patch?.action || "not_checked",
      runtime_stale_marketplace_plugin_cache: result.stale_marketplace_cache_patch?.action || "not_checked",
    },
  };
}

function printResult(result, json) {
  const output = printableResult(result);
  if (json) {
    console.log(JSON.stringify(output, null, 2));
    return;
  }
  console.log(`[runtime-cache] ${output.plugin} ${output.mode}`);
  console.log(`[runtime-cache] runtime_home=${output.runtime_home}`);
  console.log(`[runtime-cache] copy=${output.summary.copy_count} up_to_date=${output.summary.up_to_date_count} missing_source=${output.summary.missing_source_count}`);
  console.log(`[runtime-cache] transaction=${output.transaction?.action || "dry_run"} commit=${RUNTIME_CACHE_COMMIT_ORDER}`);
  if (output.mode === "dry_run" && output.summary.copy_count) {
    console.log("[runtime-cache] dry run only; stop the runtime/MCP and rerun with --apply --confirm-local-remount for an existing same-version local remount.");
  }
}

function createSourceFixture(rootPath, version, omittedPath = "") {
  for (const relativePath of PLUGIN_CACHE_FILES) {
    if (relativePath === omittedPath) continue;
    const target = path.join(rootPath, relativePath);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    let bytes = Buffer.from(`fixture:${relativePath}\n`, "utf8");
    if (relativePath === ".codex-plugin/plugin.json") {
      bytes = canonicalJson({
        name: PLUGIN_NAME,
        version,
        description: "transaction fixture",
        interface: { displayName: "Analytix 涉案资金研判", category: "Productivity" },
      });
    } else if (/^skills\/[^/]+\/SKILL\.md$/u.test(relativePath)) {
      const skillName = relativePath.split("/")[1];
      bytes = Buffer.from(`---\nname: ${skillName}\ndescription: ${skillName} fixture\n---\nfixture\n`, "utf8");
    }
    fs.writeFileSync(target, bytes);
  }
}

function createRuntimeFixture(parentPath, version) {
  const runtimeHome = path.join(parentPath, ".analytix");
  const directRoot = path.join(directPluginCacheRoot(runtimeHome), version);
  fs.mkdirSync(directRoot, { recursive: true });
  fs.writeFileSync(path.join(directRoot, "old.txt"), "old direct generation\n", "utf8");
  for (const skillName of ALL_SKILL_NAMES) {
    const skillRoot = path.join(runtimeHome, "skills", skillName);
    fs.mkdirSync(skillRoot, { recursive: true });
    fs.writeFileSync(path.join(skillRoot, "SKILL.md"), "old skill generation\n", "utf8");
  }
  const oldMarketplaceRoot = path.join(marketplacePluginCacheRoot(runtimeHome), `${version}-local-${"1".repeat(64)}`);
  fs.mkdirSync(oldMarketplaceRoot, { recursive: true });
  fs.writeFileSync(path.join(oldMarketplaceRoot, "old.txt"), "old marketplace generation\n", "utf8");
  const pointer = {
    plugins: [{
      name: PLUGIN_NAME,
      version,
      source: { source: "local", path: `./plugins/${PLUGIN_NAME}/${path.basename(oldMarketplaceRoot)}`, sha256: "1".repeat(64) },
    }],
  };
  fs.mkdirSync(path.dirname(generatedMarketplacePath(runtimeHome)), { recursive: true });
  fs.writeFileSync(generatedMarketplacePath(runtimeHome), canonicalJson(pointer));
  fs.writeFileSync(installPolicyPath(runtimeHome), canonicalJson({
    requiredPlugins: [{ pluginName: PLUGIN_NAME, version, packageSha256: "1".repeat(64) }],
    requiredSkills: ALL_SKILL_NAMES.map((skillName) => ({ pluginName: PLUGIN_NAME, skillName, version })),
  }));
  fs.mkdirSync(path.dirname(runtimeSkillCatalogPath(runtimeHome)), { recursive: true });
  fs.writeFileSync(runtimeSkillCatalogPath(runtimeHome), canonicalJson({
    skills: ALL_SKILL_NAMES.map((skillName) => ({ pluginName: PLUGIN_NAME, skillName, version, skill: { name: skillName }, interface: {} })),
  }));
  fs.mkdirSync(path.dirname(runtimeConfigPath(runtimeHome)), { recursive: true });
  fs.writeFileSync(runtimeConfigPath(runtimeHome), canonicalJson({
    capabilities: {
      mcp: { servers: { analytix_funds: { command: "node", args: [path.join(directRoot, "mcp", "server.mjs")], cwd: directRoot } } },
      skills: { roots: [path.join(directRoot, "skills")] },
    },
  }));
  return { runtimeHome, directRoot, oldMarketplaceRoot };
}

function runtimeStateHash(runtimeHome) {
  return readDirectoryState(runtimeHome)?.hash || "missing";
}

function verifyCommittedFixture(result) {
  if (!HASH_RE.test(result.workspace_sha256)) throw new Error("fixture workspace hash is not a complete SHA-256");
  for (const installedRoot of result.installed_roots) {
    const marker = parseJsonBytes(fs.readFileSync(path.join(installedRoot, ".analytix-hub-installed-plugin.json")), "installed marker");
    if (marker.packageSha256 !== result.workspace_sha256) throw new Error("installed marker hash diverged");
  }
  const pointer = parseJsonBytes(fs.readFileSync(generatedMarketplacePath(result.runtime_home)), "marketplace pointer");
  const plugin = pointer.plugins.find((entry) => text(entry.name) === PLUGIN_NAME);
  if (plugin?.source?.sha256 !== result.workspace_sha256 || !text(plugin?.source?.path).endsWith(result.workspace_sha256)) {
    throw new Error("marketplace pointer does not bind the complete source hash");
  }
}

function expectFailure(fn, marker) {
  let message = "";
  try {
    fn();
  } catch (error) {
    message = error instanceof Error ? error.message : String(error);
  }
  if (!message.includes(marker)) throw new Error(`expected failure containing ${marker}; got ${message || "success"}`);
  return message;
}

function selfTestLocalRemount(json) {
  const tempParent = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-runtime-remount-"));
  try {
    const sourceRoot = path.join(tempParent, "source");
    const version = "0.16.16";
    createSourceFixture(sourceRoot, version);

    const missingVersionFixture = createRuntimeFixture(path.join(tempParent, "missing-version"), "0.0.0-old");
    const missingVersionError = expectFailure(() => executeSync({
      apply: false,
      confirmLocalRemount: false,
      runtimeHome: missingVersionFixture.runtimeHome,
      sourceRoot,
    }), "Existing same-version direct runtime cache is required");

    const fixture = createRuntimeFixture(path.join(tempParent, "current"), version);
    const applied = executeSync({
      apply: true,
      confirmLocalRemount: true,
      runtimeHome: fixture.runtimeHome,
      sourceRoot,
    });
    verifyCommittedFixture(applied);
    const quarantineSafe = !fs.existsSync(fixture.oldMarketplaceRoot)
      && fs.existsSync(applied.marketplace_installed_root)
      && fs.existsSync(applied.direct_installed_root)
      && fs.existsSync(quarantineRoot(applied.runtime_home));
    if (!quarantineSafe) throw new Error("stale generation quarantine displaced a committed target or lost the stale generation");
    const dry = executeSync({ apply: false, confirmLocalRemount: false, runtimeHome: fixture.runtimeHome, sourceRoot });
    const secondDryRunCopyCount = dry.items.filter((item) => item.action === "copy").length;
    if (secondDryRunCopyCount !== 0 || !dry.noop) throw new Error("local remount is not idempotent");

    const missingSourceRoot = path.join(tempParent, "missing-source");
    createSourceFixture(missingSourceRoot, version, "mcp/server.mjs");
    const missingSourceFixture = createRuntimeFixture(path.join(tempParent, "missing-source-runtime"), version);
    const missingBefore = runtimeStateHash(missingSourceFixture.runtimeHome);
    expectFailure(() => executeSync({
      apply: true,
      confirmLocalRemount: true,
      runtimeHome: missingSourceFixture.runtimeHome,
      sourceRoot: missingSourceRoot,
    }), "Missing source files; runtime state was not touched");
    const missingAfter = runtimeStateHash(missingSourceFixture.runtimeHome);
    if (missingBefore !== missingAfter) throw new Error("missing source mutated runtime state");

    const symlinkSourceRoot = path.join(tempParent, "symlink-source");
    createSourceFixture(symlinkSourceRoot, version);
    const symlinkTarget = path.join(symlinkSourceRoot, "mcp", "server.mjs");
    fs.rmSync(symlinkTarget);
    fs.symlinkSync(path.join(symlinkSourceRoot, "mcp", "abort-runtime.mjs"), symlinkTarget);
    const symlinkFixture = createRuntimeFixture(path.join(tempParent, "symlink-runtime"), version);
    const symlinkBefore = runtimeStateHash(symlinkFixture.runtimeHome);
    expectFailure(() => executeSync({
      apply: true,
      confirmLocalRemount: true,
      runtimeHome: symlinkFixture.runtimeHome,
      sourceRoot: symlinkSourceRoot,
    }), "symlink is forbidden");
    if (runtimeStateHash(symlinkFixture.runtimeHome) !== symlinkBefore) throw new Error("symlink preflight mutated runtime state");

    const runtimeSymlinkFixture = createRuntimeFixture(path.join(tempParent, "runtime-symlink"), version);
    const runtimeSymlinkHome = assertAnalytixRuntimeHome(runtimeSymlinkFixture.runtimeHome);
    const replacedSkillRoot = path.join(runtimeSymlinkHome, "skills", ALL_SKILL_NAMES[0]);
    const externalSkillRoot = path.join(tempParent, "external-skill-root");
    fs.mkdirSync(externalSkillRoot, { recursive: true });
    fs.writeFileSync(path.join(externalSkillRoot, "SKILL.md"), "external\n", "utf8");
    fs.rmSync(replacedSkillRoot, { recursive: true });
    fs.symlinkSync(externalSkillRoot, replacedSkillRoot, "dir");
    expectFailure(() => executeSync({
      apply: true,
      confirmLocalRemount: true,
      runtimeHome: runtimeSymlinkFixture.runtimeHome,
      sourceRoot,
    }), "symlink is forbidden");
    if (fs.existsSync(transactionControlRoot(runtimeSymlinkHome))) {
      throw new Error("runtime symlink preflight wrote a lock or journal before rejection");
    }

    const mutationSourceRoot = path.join(tempParent, "mutation-source");
    createSourceFixture(mutationSourceRoot, version);
    const mutationFixture = createRuntimeFixture(path.join(tempParent, "mutation-runtime"), version);
    const mutationRelativePath = "mcp/server.mjs";
    const originalSourceBytes = fs.readFileSync(path.join(mutationSourceRoot, mutationRelativePath));
    const mutationApplied = executeSync({
      apply: true,
      confirmLocalRemount: true,
      runtimeHome: mutationFixture.runtimeHome,
      sourceRoot: mutationSourceRoot,
      afterSourceSnapshot: () => fs.writeFileSync(path.join(mutationSourceRoot, mutationRelativePath), "mutated after snapshot\n", "utf8"),
    });
    if (!fs.readFileSync(path.join(mutationApplied.direct_installed_root, mutationRelativePath)).equals(originalSourceBytes)) {
      throw new Error("transaction reread mutable source bytes after snapshot");
    }

    const concurrentFixture = createRuntimeFixture(path.join(tempParent, "concurrent"), version);
    const concurrentRuntimeHome = assertAnalytixRuntimeHome(concurrentFixture.runtimeHome);
    const heldLock = acquireExclusiveLock(concurrentRuntimeHome);
    const concurrentError = expectFailure(() => executeSync({
      apply: true,
      confirmLocalRemount: true,
      runtimeHome: concurrentFixture.runtimeHome,
      sourceRoot,
    }), "runtime-cache remount lock is busy");
    releaseExclusiveLock(heldLock);

    const casFixture = createRuntimeFixture(path.join(tempParent, "old-hash-cas"), version);
    const casRuntimeHome = assertAnalytixRuntimeHome(casFixture.runtimeHome);
    const casDirectBefore = directoryHashOrMissing(path.join(directPluginCacheRoot(casRuntimeHome), version));
    const casPointerBefore = pathHash(generatedMarketplacePath(casRuntimeHome));
    const casError = expectFailure(() => executeSync({
      apply: true,
      confirmLocalRemount: true,
      runtimeHome: casFixture.runtimeHome,
      sourceRoot,
      afterPlan: (plan) => {
        fs.writeFileSync(
          installPolicyPath(plan.runtime_home),
          canonicalJson({ externallyMutatedAfterPlan: true }),
        );
      },
    }), "metadata old-hash CAS failed");
    if (
      directoryHashOrMissing(path.join(directPluginCacheRoot(casRuntimeHome), version)) !== casDirectBefore
      || pathHash(generatedMarketplacePath(casRuntimeHome)) !== casPointerBefore
      || !fs.existsSync(transactionPaths(casRuntimeHome).journalPath)
    ) {
      throw new Error("old-hash CAS failure did not restore installed directories and retain its recovery journal");
    }

    const faultPoints = [
      "after_journal_prepared",
      "after_generation_published",
      "after_direct_backup_moved",
      "after_direct_published",
      `after_skill_backup_moved:${ALL_SKILL_NAMES[0]}`,
      `after_metadata_published:runtime_install_policy`,
      "after_marketplace_pointer_published",
      "after_first_quarantine",
    ];
    const recoveryDispositions = [];
    const crashPointerStates = [];
    for (let index = 0; index < faultPoints.length; index += 1) {
      const faultFixture = createRuntimeFixture(path.join(tempParent, `fault-${index}`), version);
      const faultPoint = faultPoints[index];
      expectFailure(() => executeSync({
        apply: true,
        confirmLocalRemount: true,
        runtimeHome: faultFixture.runtimeHome,
        sourceRoot,
        faultPoint,
        simulateCrash: true,
        skipDataFsyncForTest: true,
      }), `simulated crash at ${faultPoint}`);
      const crashedMarketplace = parseJsonBytes(
        fs.readFileSync(generatedMarketplacePath(assertAnalytixRuntimeHome(faultFixture.runtimeHome))),
        "crashed marketplace pointer",
      );
      const crashedPointerHash = text(crashedMarketplace.plugins.find((entry) => text(entry.name) === PLUGIN_NAME)?.source?.sha256);
      const shouldBeCommitted = ["after_marketplace_pointer_published", "after_first_quarantine"].includes(faultPoint);
      if ((crashedPointerHash !== "1".repeat(64)) !== shouldBeCommitted) {
        throw new Error(`marketplace pointer exposed the wrong transaction state after ${faultPoint}`);
      }
      crashPointerStates.push(shouldBeCommitted ? "committed" : "old_generation");
      const recovered = executeSync({
        apply: true,
        confirmLocalRemount: true,
        runtimeHome: faultFixture.runtimeHome,
        sourceRoot,
        skipDataFsyncForTest: true,
      });
      verifyCommittedFixture(recovered);
      recoveryDispositions.push(recovered.recovery?.disposition || "none");
    }

    const markerHashes = applied.installed_roots.map((root) => text(parseJsonBytes(
      fs.readFileSync(path.join(root, ".analytix-hub-installed-plugin.json")),
      "installed marker",
    ).packageSha256));
    const report = {
      ok: true,
      same_version_guard_error: missingVersionError,
      workspace_sha256: applied.workspace_sha256,
      installed_root_count: applied.installed_roots.length,
      marker_hashes: markerHashes,
      generation_name_uses_full_sha256: path.basename(applied.marketplace_installed_root).endsWith(applied.workspace_sha256),
      missing_source_zero_write: missingBefore === missingAfter,
      canonical_no_symlink_preflight: runtimeStateHash(symlinkFixture.runtimeHome) === symlinkBefore,
      runtime_symlink_preflight_zero_write: true,
      source_snapshot_is_single_read: true,
      concurrent_apply_error: concurrentError,
      old_hash_cas_error: casError,
      old_hash_cas_fail_closed: true,
      quarantine_safe: quarantineSafe,
      fault_points_tested: faultPoints,
      crash_pointer_states: crashPointerStates,
      uncommitted_generation_not_installed: crashPointerStates.slice(0, -2).every((state) => state === "old_generation"),
      recovery_dispositions: recoveryDispositions,
      second_dry_run_copy_count: secondDryRunCopyCount,
      commit_order: RUNTIME_CACHE_COMMIT_ORDER,
      offline_boundary: "--confirm-local-remount requires runtime and MCP stopped because independent directory swaps are not globally atomic",
    };
    if (json) console.log(JSON.stringify(report, null, 2));
    else console.log("[runtime-cache] journaled local remount self-test passed");
  } finally {
    fs.rmSync(tempParent, { recursive: true, force: true });
  }
}

function selfTestRuntimeConfigPatch(json) {
  const tempParent = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-runtime-config-"));
  try {
    const runtimeHome = path.join(tempParent, ".analytix");
    const oldRoot = path.join(runtimeHome, "plugins", "cache", MARKETPLACE_NAME, PLUGIN_NAME, "0.16.8");
    const directRoot = path.join(runtimeHome, "plugins", "cache", MARKETPLACE_NAME, PLUGIN_NAME, "0.16.9");
    const configPath = runtimeConfigPath(runtimeHome);
    fs.mkdirSync(path.dirname(configPath), { recursive: true });
    const original = {
      capabilities: {
        mcp: { servers: { analytix_funds: { enabled: true, transport: "stdio", command: "node", args: [path.join(oldRoot, "mcp", "server.mjs")], cwd: oldRoot } } },
        skills: { roots: [path.join(oldRoot, "skills"), path.join(runtimeHome, "skills", "other")] },
      },
    };
    fs.writeFileSync(configPath, canonicalJson(original));
    const next = patchRuntimeConfigValue(original, directRoot);
    const nextBytes = canonicalJson(next);
    const dryAction = nextBytes.equals(canonicalJson(original)) ? "up_to_date" : "would_patch";
    atomicWriteFile(configPath, nextBytes);
    const patched = parseJsonBytes(fs.readFileSync(configPath), "runtime config");
    const server = patched.capabilities.mcp.servers.analytix_funds;
    const roots = patched.capabilities.skills.roots;
    const report = {
      ok: dryAction === "would_patch"
        && server.args[0] === path.join(directRoot, "mcp", "server.mjs")
        && server.cwd === directRoot
        && roots[0] === path.join(directRoot, "skills")
        && roots[1] === path.join(runtimeHome, "skills", "other"),
      dry_action: dryAction,
      apply_action: "patch",
      server_arg: server.args[0],
      server_cwd: server.cwd,
      skill_root: roots[0],
    };
    if (json) console.log(JSON.stringify(report, null, 2));
    else console.log(report.ok ? "[runtime-cache] runtime config remount self-test passed" : "[runtime-cache] runtime config remount self-test failed");
    if (!report.ok) process.exitCode = 1;
  } finally {
    fs.rmSync(tempParent, { recursive: true, force: true });
  }
}

function main() {
  let options = { json: false };
  try {
    options = parseArgs(process.argv.slice(2));
    if (options.selfTestRuntimeConfig) return selfTestRuntimeConfigPatch(options.json);
    if (options.selfTestLocalRemount) return selfTestLocalRemount(options.json);
    const result = executeSync(options);
    printResult(result, options.json);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (options.json) console.log(JSON.stringify({ ok: false, error: message }, null, 2));
    else console.error(`[runtime-cache] ${message}`);
    process.exitCode = 1;
  }
}

main();
