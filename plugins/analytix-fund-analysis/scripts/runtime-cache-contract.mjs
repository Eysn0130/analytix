export const PLUGIN_NAME = "analytix-fund-analysis";
export const MARKETPLACE_NAME = "analytix-hub";
export const RUNTIME_CACHE_CONTRACT_VERSION = "0.16.16-runtime-cache-contract";
export const RUNTIME_CACHE_TRANSACTION_VERSION = "RuntimeCacheRemountTransactionV1";
export const RUNTIME_CACHE_COMMIT_ORDER = "marketplace_pointer_last";

export const EVAL_ONLY_RUNTIME_CACHE_FORBIDDEN_PATTERNS = [
  "scripts/eval-fixtures",
  "golden-answer-set.json",
  "golden-eval-rubric.md",
  "eval-coverage-contract.json",
  "oracle_fast_path",
  "ANALYTIX_FUNDS_EVAL_FAST_PATH",
];

export const REFERENCE_FILES = [
  "anti-patterns.md",
  "analytix-workflow-context.md",
  "blueprint-implementation-matrix.md",
  "blueprint-execution-plan.md",
  "capability-registry.json",
  "capability-registry.schema.json",
  "casegraph-roadmap.md",
  "command-metadata.json",
  "command-router.md",
  "database-site-diagnostics.md",
  "domain-playbook.md",
  "economic-investigation-analysis.md",
  "economic-investigation-language.md",
  "focused-skill-shared.md",
  "fund-path-and-cash-bridge.md",
  "hub-lifecycle.md",
  "investigation-answer-contract.md",
  "investigation-answer.schema.json",
  "mature-plugin-drift-table.md",
  "plugin-benchmark.md",
  "public-security-official-writing.md",
  "report-schema.md",
  "runtime-boundary.md",
  "top-pluginization-plan.md",
  "tool-availability.md",
];

export const FOCUSED_SKILL_NAMES = [
  "index",
  "case-context",
  "data-quality",
  "quick-fact",
  "pair-amount-investigation",
  "account-dossier",
  "subject-dossier",
  "counterparty-analysis",
  "fund-tracing",
  "investigation-lab",
  "full-case-analysis",
  "report-builder",
  "evidence-request",
  "analysis-critique",
  "claim-review",
  "delivery-qc",
  "graph-visualization",
  "visual-evidence",
  "case-workbench",
];

export const ALL_SKILL_NAMES = [PLUGIN_NAME, ...FOCUSED_SKILL_NAMES];

export const PLUGIN_CACHE_FILES = [
  ".codex-plugin/plugin.json",
  ".mcp.json",
  "agents/openai.yaml",
  ...PRODUCTION_MCP_ENTRY_CLOSURE_FILES,
  ...REFERENCE_FILES.map((relativePath) => `references/${relativePath}`),
  "skills/analytix-fund-analysis/SKILL.md",
  "skills/analytix-fund-analysis/agents/openai.yaml",
  ...REFERENCE_FILES.map(
    (relativePath) =>
      `skills/analytix-fund-analysis/references/${relativePath}`,
  ),
  ...FOCUSED_SKILL_NAMES.flatMap((skillName) => [
    `skills/${skillName}/SKILL.md`,
    `skills/${skillName}/agents/openai.yaml`,
  ]),
];

export const RUNTIME_SKILL_FILES = [
  "SKILL.md",
  ...REFERENCE_FILES.map((relativePath) => `references/${relativePath}`),
];

export function runtimeCacheContractViolations() {
  const entries = [
    ...REFERENCE_FILES.map((relativePath) => ({
      scope: "reference",
      relativePath,
    })),
    ...PLUGIN_CACHE_FILES.map((relativePath) => ({
      scope: "plugin_cache",
      relativePath,
    })),
    ...RUNTIME_SKILL_FILES.map((relativePath) => ({
      scope: "runtime_skill",
      relativePath,
    })),
  ];
  return entries.flatMap((entry) => {
    const hits = EVAL_ONLY_RUNTIME_CACHE_FORBIDDEN_PATTERNS.filter((pattern) =>
      entry.relativePath.includes(pattern),
    );
    return hits.map((pattern) => ({ ...entry, forbidden_pattern: pattern }));
  });
}
import { PRODUCTION_MCP_ENTRY_CLOSURE_FILES } from "./production-mcp-entry-closure-contract.mjs";

// Root references are maintained source; embedded copies keep skills portable.
function referenceDirectory(pluginRoot, relative, allowMissing = false) {
  const root = fs.realpathSync(pluginRoot);
  const directory = path.join(root, relative);
  if (!fs.lstatSync(directory).isDirectory() || fs.realpathSync(directory) !== directory) {
    throw new Error(`Reference directory must be a regular source directory: ${relative}`);
  }
  const names = fs.readdirSync(directory).sort();
  const expected = [...REFERENCE_FILES].sort();
  if (names.some(name => !expected.includes(name)) ||
      (!allowMissing && JSON.stringify(names) !== JSON.stringify(expected))) {
    throw new Error(`Reference inventory mismatch: ${relative}`);
  }
  for (const name of names) {
    const stat = fs.lstatSync(path.join(directory, name));
    if (!stat.isFile() || stat.isSymbolicLink()) {
      throw new Error(`Reference must be a regular file: ${relative}/${name}`);
    }
  }
  return directory;
}

export function inspectReferenceCopies(pluginRoot) {
  const source = referenceDirectory(pluginRoot, "references");
  const destination = referenceDirectory(pluginRoot, `skills/${PLUGIN_NAME}/references`);
  let bytes = 0;
  for (const name of REFERENCE_FILES) {
    const value = fs.readFileSync(path.join(source, name));
    if (!value.equals(fs.readFileSync(path.join(destination, name)))) {
      throw new Error(`Embedded reference drift: ${name}`);
    }
    bytes += value.length;
  }
  return { files: REFERENCE_FILES.length, bytes };
}

export function synchronizeReferenceCopies(pluginRoot) {
  const source = referenceDirectory(pluginRoot, "references");
  const destination = referenceDirectory(pluginRoot, `skills/${PLUGIN_NAME}/references`, true);
  for (const name of REFERENCE_FILES) {
    fs.writeFileSync(path.join(destination, name), fs.readFileSync(path.join(source, name)));
  }
  return inspectReferenceCopies(pluginRoot);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  if (args.length > 1 || (args.length === 1 && args[0] !== "--sync")) {
    throw new Error("Usage: runtime-cache-contract.mjs [--sync]");
  }
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const result = args[0] === "--sync" ? synchronizeReferenceCopies(root) : inspectReferenceCopies(root);
  console.log(JSON.stringify({ status: "pass", ...result }));
}
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
