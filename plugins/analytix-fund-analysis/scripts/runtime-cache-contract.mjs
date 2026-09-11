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
