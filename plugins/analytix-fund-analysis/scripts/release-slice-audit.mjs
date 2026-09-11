#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import strictJson from "../../../scripts/lib/strict-json.cjs";
import {
  validateFundsMcpSurfaceSnapshotV2,
} from "./mcp-surface-snapshot.mjs";
import {
  inspectProductionMcpEntryClosure,
  PRODUCTION_MCP_ENTRY_CLOSURE_FILES,
} from "./production-mcp-entry-closure-contract.mjs";

const { parseStrictJsonObject } = strictJson;

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");
const RELEASE_SLICE_SCRIPT =
  "plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs";
const DEFAULT_MCP_SURFACE_SNAPSHOT_PATH =
  "output/analytix-fund-analysis/evidence/mcp-surface-snapshot.json";
const FUNCTIONAL_CLOSURE_OUTPUT_ROOT =
  "output/analytix-fund-analysis/functional-closure";
const GENERATED_EVIDENCE_OUTPUT_PREFIXES = [
  "output/analytix-fund-analysis/",
  "packages/runtime-go/output/analytix-fund-analysis/",
];
const GOAL_WIDE_UNRELATED_DIRTY_GROUPS = new Set([
  "workspace_hygiene",
  "runtime_closure_matrix_doc",
  "runtime_contract_conformance",
  "runtime_go_regression_tests",
  "runtime_go_build_artifact",
  "runtime_go_product_regression_matrix",
  "runtime_model_execution_ref",
  "runtime_sse_ipc_recovery",
  "runtime_closure_matrix_audit",
  "runtime_lint_cleanup",
  "runtime_tool_result_image",
  "runtime_skill_plugin_mentions",
  "runtime_direct_answer_profile",
  "runtime_speed_cache_gate",
  "runtime_event_contracts",
  "runtime_info_bridge",
  "renderer_runtime_projection",
  "runtime_plugin_skill_boundary",
  "packaged_runtime_boundary",
  "provider_capability_probe",
  "provider_model_picker",
  "renderer_subagent_lifecycle",
  "other_plugin_manifest",
]);
const FUNCTIONAL_CLOSURE_REQUIRED_FAMILIES = [
  "pair_amount_reconciliation",
  "case_source_envelope",
  "focused_owner_architecture",
  "support_layer_hidden",
  "analytix_workflow_context",
  "economic_investigation_delivery",
  "mcp_decentralization",
  "case_workbench_control",
  "output_leak_prevention",
  "release_package_isolation",
  "frontdoor_semantic_tools",
  "frontdoor_weak_source_boundaries",
];
const EVAL_FIXTURE_RELEASE_FILES = [
  "plugins/analytix-fund-analysis/scripts/eval-fixtures/golden-answer-set.json",
  "plugins/analytix-fund-analysis/scripts/eval-fixtures/golden-eval-rubric.md",
  "plugins/analytix-fund-analysis/scripts/eval-fixtures/eval-coverage-contract.json",
];
const FOCUSED_SKILL_NAMES = [
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
const FOCUSED_SKILL_RELEASE_FILES = [
  "plugins/analytix-fund-analysis/agents/openai.yaml",
  "plugins/analytix-fund-analysis/references/economic-investigation-analysis.md",
  "plugins/analytix-fund-analysis/references/economic-investigation-language.md",
  "plugins/analytix-fund-analysis/references/focused-skill-shared.md",
  "plugins/analytix-fund-analysis/references/analytix-workflow-context.md",
  "plugins/analytix-fund-analysis/references/public-security-official-writing.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/agents/openai.yaml",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/economic-investigation-analysis.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/economic-investigation-language.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/focused-skill-shared.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/analytix-workflow-context.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/public-security-official-writing.md",
  ...FOCUSED_SKILL_NAMES.flatMap((skillName) => [
    `plugins/analytix-fund-analysis/skills/${skillName}/SKILL.md`,
    `plugins/analytix-fund-analysis/skills/${skillName}/agents/openai.yaml`,
  ]),
];
export const RELEASE_VERSION_FILES = [
  "package.json",
  "package-lock.json",
  "plugins/analytix-fund-analysis/.codex-plugin/plugin.json",
  ...PRODUCTION_MCP_ENTRY_CLOSURE_FILES.map(
    (relativePath) => `plugins/analytix-fund-analysis/${relativePath}`,
  ),
  "plugins/analytix-fund-analysis/README.md",
  "plugins/analytix-fund-analysis/RELEASE_NOTES.md",
  "plugins/analytix-fund-analysis/references/capability-registry.json",
  "plugins/analytix-fund-analysis/references/capability-registry.schema.json",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/capability-registry.json",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/capability-registry.schema.json",
  "plugins/analytix-fund-analysis/scripts/eval-coverage-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure-manifest.cjs",
  "plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure.json",
  "plugins/analytix-fund-analysis/scripts/runtime-cache-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/source-live-probe-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/source-live-probe-go-wire-contract.mjs",
  "packages/runtime-go/internal/mcp/manager.go",
];
const ISOLATION_NODE_CHECK_FILES = [
  "electron-builder.config.cjs",
  "scripts/after-pack.cjs",
  "plugins/analytix-fund-analysis/mcp/server.mjs",
  "plugins/analytix-fund-analysis/mcp/abort-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/artifact-path-policy.mjs",
  "plugins/analytix-fund-analysis/mcp/agent-context-hygiene.mjs",
  "plugins/analytix-fund-analysis/mcp/agent-output-compiler.mjs",
  "plugins/analytix-fund-analysis/mcp/agent-payload-compiler.mjs",
  "plugins/analytix-fund-analysis/mcp/answer-card-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/backend-api-client.mjs",
  "plugins/analytix-fund-analysis/mcp/card-renderer.mjs",
  "plugins/analytix-fund-analysis/mcp/casegraph-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/casegraph-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/case-scope-map-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/cleaned-export-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/claim-review-diagnostic-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/claim-verifier-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/context-compiler.mjs",
  "plugins/analytix-fund-analysis/mcp/destination-diagnostic-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/duckdb-diagnostic.mjs",
  "plugins/analytix-fund-analysis/mcp/duckdb-sql-policy.mjs",
  "plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/evidence-ledger.mjs",
  "plugins/analytix-fund-analysis/mcp/fund-flow-graph-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/fundgraph-builder.mjs",
  "plugins/analytix-fund-analysis/mcp/frontdoor-answer-contract.mjs",
  "plugins/analytix-fund-analysis/mcp/frontdoor-fact-summaries.mjs",
  "plugins/analytix-fund-analysis/mcp/frontdoor-routing.mjs",
  "plugins/analytix-fund-analysis/mcp/frontdoor-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/intent-plan-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/investigation-lab-diagnostic-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/investigation-lab-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/investigation-answer-contract.mjs",
  "plugins/analytix-fund-analysis/mcp/progressive-prompts.mjs",
  "plugins/analytix-fund-analysis/mcp/progressive-resources.mjs",
  "plugins/analytix-fund-analysis/mcp/report-publication-guard.mjs",
  "plugins/analytix-fund-analysis/mcp/runtime-normalizers.mjs",
  "plugins/analytix-fund-analysis/mcp/stable-hash.mjs",
  "plugins/analytix-fund-analysis/mcp/tool-schemas.mjs",
  "plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/top-rankings-diagnostic-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/user-facing-language.mjs",
  "plugins/analytix-fund-analysis/scripts/agent-thread-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/agent-ui-ab-run.mjs",
  "plugins/analytix-fund-analysis/scripts/blueprint-capability-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/check-health.mjs",
  "plugins/analytix-fund-analysis/scripts/desktop-multiturn-acceptance.mjs",
  "plugins/analytix-fund-analysis/scripts/diagnostic-card-diff.mjs",
  "plugins/analytix-fund-analysis/scripts/doctor.mjs",
  "plugins/analytix-fund-analysis/scripts/eval-coverage-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/functional-eval.mjs",
  "plugins/analytix-fund-analysis/scripts/frontdoor-p0-oracle.mjs",
  "plugins/analytix-fund-analysis/scripts/funds-producer-content-v1-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/dataset-snapshot-manifest-v2-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/tool-schema-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/p0-contract-suite.mjs",
  "plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/count-case-rows-contract-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/investigation-answer-contract-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/investigation-scenario-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/mcp-return-oracle.mjs",
  "plugins/analytix-fund-analysis/scripts/mcp-surface-snapshot.mjs",
  "plugins/analytix-fund-analysis/scripts/model-ab-eval.mjs",
  "plugins/analytix-fund-analysis/scripts/pair-amount-contract-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/pair-amount-real-case-dedup-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/phase4-diagnostics.mjs",
  "plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure-manifest.cjs",
  "plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure.json",
  "plugins/analytix-fund-analysis/scripts/runtime-cache-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/skill-creator-alignment-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/skill-clause-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/sync-runtime-cache.mjs",
  "plugins/analytix-fund-analysis/scripts/user-visible-language-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/prepare-hub-package.mjs",
  RELEASE_SLICE_SCRIPT,
];

export const RELEASE_SLICE_FILES = [
  ...RELEASE_VERSION_FILES,
  ...EVAL_FIXTURE_RELEASE_FILES,
  "electron-builder.config.cjs",
  "scripts/after-pack.cjs",
  "packages/runtime-go/internal/mcp/manager_test.go",
  "packages/runtime-go/internal/mcp/manager_test_double.go",
  "packages/runtime-go/internal/mcp/source_probe_plugin_contract_test.go",
  "packages/runtime-go/internal/server/mcp_tool_schema_test.go",
  "src/main/packaging-config.test.ts",
  "src/main/analytix-process.ts",
  "src/main/analytix-process.test.ts",
  "plugins/analytix-fund-analysis/.mcp.json",
  "plugins/analytix-fund-analysis/mcp/abort-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/artifact-path-policy.mjs",
  "plugins/analytix-fund-analysis/mcp/agent-context-hygiene.mjs",
  "plugins/analytix-fund-analysis/mcp/agent-output-compiler.mjs",
  "plugins/analytix-fund-analysis/mcp/agent-payload-compiler.mjs",
  "plugins/analytix-fund-analysis/mcp/answer-card-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/capability-registry-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/card-renderer.mjs",
  "plugins/analytix-fund-analysis/mcp/case-pipeline-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/case-project-context.mjs",
  "plugins/analytix-fund-analysis/mcp/casegraph-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/casegraph-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/case-scope-map-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/cleaned-export-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/backend-api-client.mjs",
  "plugins/analytix-fund-analysis/mcp/claim-review-diagnostic-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/claim-verifier-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/context-compiler.mjs",
  "plugins/analytix-fund-analysis/mcp/destination-diagnostic-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/diagnostic-fact-helpers.mjs",
  "plugins/analytix-fund-analysis/mcp/duckdb-diagnostic.mjs",
  "plugins/analytix-fund-analysis/mcp/duckdb-sql-policy.mjs",
  "plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/evidence-ledger.mjs",
  "plugins/analytix-fund-analysis/mcp/fund-flow-graph-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/fundgraph-builder.mjs",
  "plugins/analytix-fund-analysis/mcp/frontdoor-answer-contract.mjs",
  "plugins/analytix-fund-analysis/mcp/frontdoor-fact-summaries.mjs",
  "plugins/analytix-fund-analysis/mcp/frontdoor-routing.mjs",
  "plugins/analytix-fund-analysis/mcp/frontdoor-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/intent-plan-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/investigation-lab-diagnostic-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/investigation-lab-protocol.mjs",
  "plugins/analytix-fund-analysis/mcp/jsonrpc-stdio-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/mcp-output-policy.mjs",
  "plugins/analytix-fund-analysis/mcp/mcp-request-handler-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/mcp-tool-result-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/progressive-prompts.mjs",
  "plugins/analytix-fund-analysis/mcp/progressive-resources.mjs",
  "plugins/analytix-fund-analysis/mcp/report-publication-guard.mjs",
  "plugins/analytix-fund-analysis/mcp/runtime-normalizers.mjs",
  "plugins/analytix-fund-analysis/mcp/safe-skill-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/stats-query-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/stable-hash.mjs",
  "plugins/analytix-fund-analysis/mcp/investigation-answer-contract.mjs",
  "plugins/analytix-fund-analysis/mcp/tool-discovery-policy.mjs",
  "plugins/analytix-fund-analysis/mcp/tool-input-schemas.mjs",
  "plugins/analytix-fund-analysis/mcp/tool-runtime-routing.mjs",
  "plugins/analytix-fund-analysis/mcp/tool-schemas.mjs",
  "plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/top-rankings-diagnostic-runtime.mjs",
  "plugins/analytix-fund-analysis/mcp/user-facing-language.mjs",
  "plugins/analytix-fund-analysis/mcp/visual-artifact-contract.mjs",
  "plugins/analytix-fund-analysis/references/anti-patterns.md",
  "plugins/analytix-fund-analysis/references/blueprint-execution-plan.md",
  "plugins/analytix-fund-analysis/references/blueprint-implementation-matrix.md",
  "plugins/analytix-fund-analysis/references/command-metadata.json",
  "plugins/analytix-fund-analysis/references/command-router.md",
  "plugins/analytix-fund-analysis/references/database-site-diagnostics.md",
  "plugins/analytix-fund-analysis/references/hub-lifecycle.md",
  "plugins/analytix-fund-analysis/references/investigation-answer-contract.md",
  "plugins/analytix-fund-analysis/references/investigation-answer.schema.json",
  "plugins/analytix-fund-analysis/references/mature-plugin-drift-table.md",
  "plugins/analytix-fund-analysis/references/plugin-benchmark.md",
  "plugins/analytix-fund-analysis/references/public-security-official-writing.md",
  "plugins/analytix-fund-analysis/references/runtime-boundary.md",
  "plugins/analytix-fund-analysis/references/report-schema.md",
  "plugins/analytix-fund-analysis/references/top-pluginization-plan.md",
  "plugins/analytix-fund-analysis/references/tool-availability.md",
  ...FOCUSED_SKILL_RELEASE_FILES,
  "plugins/analytix-fund-analysis/scripts/agent-ui-ab-run.mjs",
  "plugins/analytix-fund-analysis/scripts/agent-thread-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/blueprint-capability-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/check-health.mjs",
  "plugins/analytix-fund-analysis/scripts/desktop-multiturn-acceptance.mjs",
  "plugins/analytix-fund-analysis/scripts/diagnostic-card-diff.mjs",
  "plugins/analytix-fund-analysis/scripts/doctor.mjs",
  "plugins/analytix-fund-analysis/scripts/duckdb-diagnostic-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/duckdb-external-result-safety-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/duckdb-sql-policy-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/duckdb-trace-external-number-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/functional-eval.mjs",
  "plugins/analytix-fund-analysis/scripts/frontdoor-p0-oracle.mjs",
  "plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/count-case-rows-contract-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/investigation-answer-contract-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/investigation-scenario-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/golden-qa.mjs",
  "plugins/analytix-fund-analysis/scripts/mcp-return-oracle.mjs",
  "plugins/analytix-fund-analysis/scripts/mcp-surface-snapshot.mjs",
  "plugins/analytix-fund-analysis/scripts/model-ab-eval.mjs",
  "plugins/analytix-fund-analysis/scripts/pair-amount-contract-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/pair-amount-real-case-dedup-smoke.mjs",
  "plugins/analytix-fund-analysis/scripts/phase4-diagnostics.mjs",
  "plugins/analytix-fund-analysis/scripts/p0-containment-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/funds-producer-content-v1-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/dataset-snapshot-manifest-v2-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/tool-schema-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/p0-contract-suite.mjs",
  "plugins/analytix-fund-analysis/scripts/report-publication-guard-contract.mjs",
  "plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/prepare-hub-package.mjs",
  "plugins/analytix-fund-analysis/scripts/skill-creator-alignment-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/skill-clause-audit.mjs",
  "plugins/analytix-fund-analysis/scripts/sync-runtime-cache.mjs",
  "plugins/analytix-fund-analysis/scripts/user-visible-language-smoke.mjs",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/anti-patterns.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/blueprint-execution-plan.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/blueprint-implementation-matrix.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/command-metadata.json",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/command-router.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/database-site-diagnostics.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/hub-lifecycle.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/investigation-answer-contract.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/investigation-answer.schema.json",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/mature-plugin-drift-table.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/plugin-benchmark.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/public-security-official-writing.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/runtime-boundary.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/report-schema.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/top-pluginization-plan.md",
  "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/tool-availability.md",
];

function parseArgs(argv) {
  const options = {
    json: false,
    summaryJson: false,
    failOnMissing: false,
    failOnUnrelated: false,
    failOnPublishBlockers: false,
    candidateVersion: "",
    functionalClosureEvidencePath: "",
    mcpSurfaceEvidencePath: "",
    stagingPlan: false,
    readinessPlan: false,
    verifyIsolated: false,
    selfTestGeneratedEvidenceIgnore: false,
    selfTestMcpSurfaceEvidence: false,
    selfTestNativeRuntimeEvidence: false,
    selfTestUnrelatedDirtyGroups: false,
    selfTestReleaseTagStatus: false,
    selfTestSummaryJson: false,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const requiredValue = (flag) => {
      const value = String(argv[index + 1] || "").trim();
      if (!value || value.startsWith("--")) {
        throw new Error(`${flag} requires a value`);
      }
      index += 1;
      return value;
    };
    if (arg === "--json") options.json = true;
    else if (arg === "--summary-json") options.summaryJson = true;
    else if (arg === "--fail-on-missing") options.failOnMissing = true;
    else if (arg === "--fail-on-unrelated") options.failOnUnrelated = true;
    else if (arg === "--fail-on-publish-blockers")
      options.failOnPublishBlockers = true;
    else if (arg === "--candidate-version") {
      options.candidateVersion = requiredValue(arg);
    } else if (arg.startsWith("--candidate-version=")) {
      options.candidateVersion = String(
        arg.slice("--candidate-version=".length),
      ).trim();
      if (!options.candidateVersion)
        throw new Error("--candidate-version requires a value");
    } else if (arg === "--functional-closure-evidence") {
      options.functionalClosureEvidencePath = requiredValue(arg);
    } else if (arg.startsWith("--functional-closure-evidence=")) {
      options.functionalClosureEvidencePath = String(
        arg.slice("--functional-closure-evidence=".length),
      ).trim();
      if (!options.functionalClosureEvidencePath)
        throw new Error("--functional-closure-evidence requires a value");
    } else if (arg === "--mcp-surface-evidence") {
      options.mcpSurfaceEvidencePath = requiredValue(arg);
    } else if (arg.startsWith("--mcp-surface-evidence=")) {
      options.mcpSurfaceEvidencePath = String(
        arg.slice("--mcp-surface-evidence=".length),
      ).trim();
      if (!options.mcpSurfaceEvidencePath)
        throw new Error("--mcp-surface-evidence requires a value");
    } else if (arg === "--staging-plan") options.stagingPlan = true;
    else if (arg === "--readiness-plan") options.readinessPlan = true;
    else if (arg === "--verify-isolated") options.verifyIsolated = true;
    else if (arg === "--self-test-generated-evidence-ignore")
      options.selfTestGeneratedEvidenceIgnore = true;
    else if (arg === "--self-test-mcp-surface-evidence")
      options.selfTestMcpSurfaceEvidence = true;
    else if (arg === "--self-test-native-runtime-evidence")
      options.selfTestNativeRuntimeEvidence = true;
    else if (arg === "--self-test-unrelated-dirty-groups")
      options.selfTestUnrelatedDirtyGroups = true;
    else if (arg === "--self-test-release-tag-status")
      options.selfTestReleaseTagStatus = true;
    else if (arg === "--self-test-summary-json")
      options.selfTestSummaryJson = true;
    else if (arg === "--strict") {
      options.failOnMissing = true;
      options.failOnUnrelated = true;
    } else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }
  if (options.failOnPublishBlockers || options.mcpSurfaceEvidencePath) {
    options.readinessPlan = true;
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/release-slice-audit.mjs [options]

Options:
  --json                Print machine-readable JSON.
  --summary-json        Print bounded machine-readable release blocker summary.
  --fail-on-missing     Exit non-zero when a release-slice file is missing.
  --fail-on-unrelated   Exit non-zero when dirty paths fall outside the release slice.
  --fail-on-publish-blockers
                        Build full readiness and exit non-zero when any publish blocker remains.
  --candidate-version   Candidate next plugin version for read-only release planning.
  --functional-closure-evidence
                        Read-only release functional closure evidence under ${FUNCTIONAL_CLOSURE_OUTPUT_ROOT}.
  --mcp-surface-evidence
                        Read-only source-bound MCP V2 diagnostic. Implies --readiness-plan.
                        Default: output/analytix-fund-analysis/evidence/mcp-surface-snapshot.json.
  --staging-plan        Print a read-only plan for staging only release-slice paths.
  --readiness-plan      Print a read-only release commit/tag readiness plan.
  --verify-isolated     Verify the release slice in a temporary detached git worktree.
  --self-test-generated-evidence-ignore
                        Verify local generated-evidence dirty-path classification.
  --self-test-mcp-surface-evidence
                        Verify strict V2 ingestion and stale/forged evidence rejection.
  --self-test-native-runtime-evidence
                        Verify packaged native runtime evidence candidate-path detection.
  --self-test-unrelated-dirty-groups
                        Verify unrelated dirty path grouping used by staging diagnostics.
  --self-test-release-tag-status
                        Verify release tag missing/stale/at-head diagnostics.
  --self-test-summary-json
                        Verify bounded summary JSON shape.
  --strict              Enable missing-file and unrelated-dirty fail-on checks.
`);
}

function runCommand(
  command,
  args,
  { cwd = REPO_ROOT, allowStatus = [0] } = {},
) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
  });
  if (!allowStatus.includes(result.status)) {
    const detail = String(result.stderr || result.stdout || "").trim();
    throw new Error(
      `${command} ${args.join(" ")} failed${detail ? `: ${detail}` : ""}`,
    );
  }
  return result;
}

function runGit(args, options = {}) {
  return String(runCommand("git", args, options).stdout || "").trim();
}

function parsePorcelainLine(line) {
  const status = line.slice(0, 2).padEnd(2, " ");
  const pathStart = line[2] === " " ? 3 : 2;
  const rawPath = line.slice(pathStart);
  const pathValue = rawPath.includes(" -> ")
    ? rawPath.split(" -> ").at(-1)
    : rawPath;
  return {
    status,
    path: pathValue,
  };
}

function gitStatusEntries() {
  const output = runGit(["status", "--porcelain=v1", "--untracked-files=all"]);
  if (!output) return [];
  return output
    .split("\n")
    .map((line) => line.trimEnd())
    .filter(Boolean)
    .map(parsePorcelainLine);
}

function isEvidenceCleanupPath(entry) {
  return (
    text(entry?.path).startsWith("plugins/analytix-fund-analysis/evidence/") &&
    text(entry?.status).includes("D")
  );
}

function isGeneratedEvidenceOutputPath(entry) {
  const status = text(entry?.status);
  const entryPath = text(entry?.path);
  return (
    !status.includes("D") &&
    GENERATED_EVIDENCE_OUTPUT_PREFIXES.some((prefix) =>
      entryPath.startsWith(prefix),
    )
  );
}

function selfTestGeneratedEvidenceIgnore() {
  const cases = [
    {
      name: "repo output evidence is ignored",
      entry: {
        status: "??",
        path: "output/analytix-fund-analysis/frontdoor-p0-oracle/run/result.json",
      },
      expected: true,
    },
    {
      name: "runtime-go cwd output evidence is ignored",
      entry: {
        status: "??",
        path: "packages/runtime-go/output/analytix-fund-analysis/frontdoor-canary-runs/run/threads/t/thread.json",
      },
      expected: true,
    },
    {
      name: "tracked output evidence modification is ignored",
      entry: {
        status: " M",
        path: "output/analytix-fund-analysis/frontdoor-p0-oracle/run/result.json",
      },
      expected: true,
    },
    {
      name: "tracked output evidence deletion is not ignored",
      entry: {
        status: " D",
        path: "output/analytix-fund-analysis/frontdoor-p0-oracle/run/result.json",
      },
      expected: false,
    },
    {
      name: "unrelated runtime-go output is not ignored",
      entry: {
        status: "??",
        path: "packages/runtime-go/output/other/result.json",
      },
      expected: false,
    },
  ];
  const results = cases.map((testCase) => {
    const actual = isGeneratedEvidenceOutputPath(testCase.entry);
    return {
      ...testCase,
      actual,
      ok: actual === testCase.expected,
    };
  });
  return {
    ok: results.every((result) => result.ok),
    prefixes: GENERATED_EVIDENCE_OUTPUT_PREFIXES,
    results,
  };
}

function unrelatedDirtyGroupForPath(relativePath) {
  const value = text(relativePath);
  if (value === ".gitignore") {
    return "workspace_hygiene";
  }
  if (
    value === "docs/analytix/upstreams/runtime-closure-p0-p1-matrix.md" ||
    value === "docs/analytix/upstreams/go-runtime-conformance.md"
  ) {
    return "runtime_closure_matrix_doc";
  }
  if (
    value === "packages/runtime-go/internal/server/http_contract.go" ||
    value.startsWith("packages/runtime/src/conformance/")
  ) {
    return "runtime_contract_conformance";
  }
  if (
    value === "packages/runtime-go/internal/server/runtime_shutdown_test.go" ||
    value === "packages/runtime-go/internal/server/durable_store_test.go" ||
    value ===
      "packages/runtime-go/internal/server/model_execution_ref_test.go" ||
    value === "packages/runtime-go/runtime_server_test.go"
  ) {
    return "runtime_go_regression_tests";
  }
  if (value === "packages/runtime-go/runtime-server") {
    return "runtime_go_build_artifact";
  }
  if (value === "packages/runtime-go/internal/server/vision_bridge_test.go") {
    return "runtime_info_bridge";
  }
  if (
    value ===
    "packages/runtime-go/internal/server/product_regression_matrix_test.go"
  ) {
    return "runtime_go_product_regression_matrix";
  }
  if (
    value === "packages/runtime/src/contracts/model-execution-ref.ts" ||
    value ===
      "packages/runtime/src/delegation-test-support/child-agent-executor.ts" ||
    value ===
      "packages/runtime/src/delegation-test-support/delegation-runtime.ts" ||
    value === "packages/runtime/src/loop-test-support/agent-loop.ts" ||
    value === "packages/runtime/tests/child-agent-executor.test.ts" ||
    value === "packages/runtime/tests/contracts.test.ts" ||
    value === "packages/runtime/tests/delegation-runtime.test.ts" ||
    value === "packages/runtime/tests/thread-summary-route.test.ts"
  ) {
    return "runtime_model_execution_ref";
  }
  if (
    value === "src/main/runtime-sse-ipc.ts" ||
    value === "src/main/runtime-sse-ipc.test.ts"
  ) {
    return "runtime_sse_ipc_recovery";
  }
  if (
    value === "scripts/runtime-closure-matrix-audit.mjs" ||
    value === "scripts/runtime-go-product-regression.mjs"
  ) {
    return "runtime_closure_matrix_audit";
  }
  if (
    value ===
      "packages/runtime/src/adapters/computer-use/open-computer-use-backend.ts" ||
    value === "packages/runtime/src/environment/command-probe.ts" ||
    value === "src/renderer/src/lib/composer-mentions.ts"
  ) {
    return "runtime_lint_cleanup";
  }
  if (value.startsWith("packages/runtime/src/loop/tool-result-image")) {
    return "runtime_tool_result_image";
  }
  if (
    value === "packages/runtime/src/skills/skill-runtime.ts" ||
    value === "packages/runtime/tests/skill-runtime.test.ts"
  ) {
    return "runtime_skill_plugin_mentions";
  }
  if (
    value === "packages/runtime/src/loop/agent-loop.ts" ||
    value === "packages/runtime/tests/loop.test.ts"
  ) {
    return "runtime_direct_answer_profile";
  }
  if (
    value === "packages/runtime/src/cache/prefix-cache-diagnostics.ts" ||
    value === "packages/runtime/tests/cache.test.ts" ||
    value === "scripts/runtime-go-speed-cache-gate.mjs"
  ) {
    return "runtime_speed_cache_gate";
  }
  if (value === "packages/runtime/src/contracts/events.ts") {
    return "runtime_event_contracts";
  }
  if (
    value === "src/main/analytix-process.ts" ||
    value === "src/main/services/skill-service.ts" ||
    value === "src/main/services/skill-service.test.ts"
  ) {
    return "runtime_plugin_skill_boundary";
  }
  if (
    value === "scripts/after-pack.cjs" ||
    value === "src/main/packaging-config.test.ts"
  ) {
    return "packaged_runtime_boundary";
  }
  if (value.startsWith("src/main/provider-connection")) {
    return "provider_capability_probe";
  }
  if (value === "src/main/upstream-models.ts") {
    return "provider_model_picker";
  }
  if (value === "src/shared/app-settings-provider.ts") {
    return "provider_capability_probe";
  }
  if (
    value === "src/shared/app-settings-normalize.ts" ||
    value === "src/shared/app-settings-provider.test.ts" ||
    value === "src/shared/app-settings.test.ts" ||
    value ===
      "packages/runtime/src/adapters/model/multi-provider-model-client.ts" ||
    value ===
      "packages/runtime/src/adapters/model/multi-provider-model-client.test.ts" ||
    value === "src/renderer/src/agent/analytix-runtime.ts" ||
    value === "src/renderer/src/store/chat-store-app-actions.ts" ||
    value === "src/renderer/src/store/chat-store-app-actions.test.ts" ||
    value === "packages/runtime-go/internal/provider/provider.go" ||
    value === "packages/runtime-go/internal/provider/provider_test.go"
  ) {
    return "provider_capability_probe";
  }
  if (value.startsWith("src/renderer/src/components/summary/")) {
    return "renderer_subagent_lifecycle";
  }
  if (
    value === "src/renderer/src/agent/analytix-contract.ts" ||
    value === "src/renderer/src/agent/analytix-mapper.test.ts" ||
    value === "src/renderer/src/agent/analytix-runtime.test.ts"
  ) {
    return "renderer_runtime_projection";
  }
  if (
    value === "src/renderer/src/components/chat/FloatingComposer.tsx" ||
    value === "src/renderer/src/components/chat/FloatingComposer.test.ts"
  ) {
    return "provider_model_picker";
  }
  if (value.startsWith("plugins/analytix-computer-use/")) {
    return "other_plugin_manifest";
  }
  return "other";
}

function selfTestUnrelatedDirtyGroups() {
  const cases = [
    {
      name: "workspace hygiene ignore policy",
      path: ".gitignore",
      expected: "workspace_hygiene",
    },
    {
      name: "runtime closure matrix doc",
      path: "docs/analytix/upstreams/runtime-closure-p0-p1-matrix.md",
      expected: "runtime_closure_matrix_doc",
    },
    {
      name: "runtime Go conformance doc",
      path: "docs/analytix/upstreams/go-runtime-conformance.md",
      expected: "runtime_closure_matrix_doc",
    },
    {
      name: "runtime conformance fixture",
      path: "packages/runtime/src/conformance/fixtures/go-g1-shadow-contract.json",
      expected: "runtime_contract_conformance",
    },
    {
      name: "runtime server regression",
      path: "packages/runtime-go/runtime_server_test.go",
      expected: "runtime_go_regression_tests",
    },
    {
      name: "runtime durable store regression",
      path: "packages/runtime-go/internal/server/durable_store_test.go",
      expected: "runtime_go_regression_tests",
    },
    {
      name: "runtime model execution ref Go regression",
      path: "packages/runtime-go/internal/server/model_execution_ref_test.go",
      expected: "runtime_go_regression_tests",
    },
    {
      name: "runtime Go build artifact",
      path: "packages/runtime-go/runtime-server",
      expected: "runtime_go_build_artifact",
    },
    {
      name: "runtime info vision bridge regression",
      path: "packages/runtime-go/internal/server/vision_bridge_test.go",
      expected: "runtime_info_bridge",
    },
    {
      name: "runtime product regression matrix",
      path: "packages/runtime-go/internal/server/product_regression_matrix_test.go",
      expected: "runtime_go_product_regression_matrix",
    },
    {
      name: "runtime model execution ref contract",
      path: "packages/runtime/src/contracts/model-execution-ref.ts",
      expected: "runtime_model_execution_ref",
    },
    {
      name: "runtime model execution ref delegation support",
      path: "packages/runtime/src/delegation-test-support/delegation-runtime.ts",
      expected: "runtime_model_execution_ref",
    },
    {
      name: "runtime model execution ref contract regression",
      path: "packages/runtime/tests/contracts.test.ts",
      expected: "runtime_model_execution_ref",
    },
    {
      name: "runtime SSE IPC recovery",
      path: "src/main/runtime-sse-ipc.ts",
      expected: "runtime_sse_ipc_recovery",
    },
    {
      name: "runtime SSE IPC recovery regression",
      path: "src/main/runtime-sse-ipc.test.ts",
      expected: "runtime_sse_ipc_recovery",
    },
    {
      name: "runtime closure matrix audit",
      path: "scripts/runtime-closure-matrix-audit.mjs",
      expected: "runtime_closure_matrix_audit",
    },
    {
      name: "runtime closure matrix audit product regression hook",
      path: "scripts/runtime-go-product-regression.mjs",
      expected: "runtime_closure_matrix_audit",
    },
    {
      name: "runtime lint cleanup",
      path: "packages/runtime/src/environment/command-probe.ts",
      expected: "runtime_lint_cleanup",
    },
    {
      name: "runtime tool result image",
      path: "packages/runtime/src/loop/tool-result-image.ts",
      expected: "runtime_tool_result_image",
    },
    {
      name: "runtime skill plugin mention runtime",
      path: "packages/runtime/src/skills/skill-runtime.ts",
      expected: "runtime_skill_plugin_mentions",
    },
    {
      name: "runtime skill plugin mention regression",
      path: "packages/runtime/tests/skill-runtime.test.ts",
      expected: "runtime_skill_plugin_mentions",
    },
    {
      name: "runtime direct answer profile",
      path: "packages/runtime/src/loop/agent-loop.ts",
      expected: "runtime_direct_answer_profile",
    },
    {
      name: "runtime direct answer profile regression",
      path: "packages/runtime/tests/loop.test.ts",
      expected: "runtime_direct_answer_profile",
    },
    {
      name: "runtime speed cache diagnostics",
      path: "packages/runtime/src/cache/prefix-cache-diagnostics.ts",
      expected: "runtime_speed_cache_gate",
    },
    {
      name: "runtime speed cache regression",
      path: "packages/runtime/tests/cache.test.ts",
      expected: "runtime_speed_cache_gate",
    },
    {
      name: "runtime speed cache gate script",
      path: "scripts/runtime-go-speed-cache-gate.mjs",
      expected: "runtime_speed_cache_gate",
    },
    {
      name: "runtime event contract",
      path: "packages/runtime/src/contracts/events.ts",
      expected: "runtime_event_contracts",
    },
    {
      name: "main runtime plugin boundary",
      path: "src/main/analytix-process.ts",
      expected: "runtime_plugin_skill_boundary",
    },
    {
      name: "main skill service boundary",
      path: "src/main/services/skill-service.ts",
      expected: "runtime_plugin_skill_boundary",
    },
    {
      name: "main skill service boundary regression",
      path: "src/main/services/skill-service.test.ts",
      expected: "runtime_plugin_skill_boundary",
    },
    {
      name: "packaged after-pack boundary",
      path: "scripts/after-pack.cjs",
      expected: "packaged_runtime_boundary",
    },
    {
      name: "packaging config regression",
      path: "src/main/packaging-config.test.ts",
      expected: "packaged_runtime_boundary",
    },
    {
      name: "provider capability probe",
      path: "src/main/provider-connection.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "provider model picker",
      path: "src/main/upstream-models.ts",
      expected: "provider_model_picker",
    },
    {
      name: "Go provider contract",
      path: "packages/runtime-go/internal/provider/provider.go",
      expected: "provider_capability_probe",
    },
    {
      name: "Go provider contract regression",
      path: "packages/runtime-go/internal/provider/provider_test.go",
      expected: "provider_capability_probe",
    },
    {
      name: "provider request selection",
      path: "src/shared/app-settings-provider.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "provider request normalization",
      path: "src/shared/app-settings-normalize.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "multi-provider model client",
      path: "packages/runtime/src/adapters/model/multi-provider-model-client.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "multi-provider model client regression",
      path: "packages/runtime/src/adapters/model/multi-provider-model-client.test.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "provider request selection settings regression",
      path: "src/shared/app-settings-provider.test.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "provider request selection settings integration regression",
      path: "src/shared/app-settings.test.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "provider runtime request selection",
      path: "src/renderer/src/agent/analytix-runtime.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "provider chat model action selection",
      path: "src/renderer/src/store/chat-store-app-actions.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "provider chat model action regression",
      path: "src/renderer/src/store/chat-store-app-actions.test.ts",
      expected: "provider_capability_probe",
    },
    {
      name: "renderer subagent lifecycle",
      path: "src/renderer/src/components/summary/SubagentSummaryRows.tsx",
      expected: "renderer_subagent_lifecycle",
    },
    {
      name: "renderer runtime contract projection",
      path: "src/renderer/src/agent/analytix-contract.ts",
      expected: "renderer_runtime_projection",
    },
    {
      name: "renderer mapper projection regression",
      path: "src/renderer/src/agent/analytix-mapper.test.ts",
      expected: "renderer_runtime_projection",
    },
    {
      name: "renderer runtime projection regression",
      path: "src/renderer/src/agent/analytix-runtime.test.ts",
      expected: "renderer_runtime_projection",
    },
    {
      name: "composer model picker",
      path: "src/renderer/src/components/chat/FloatingComposer.tsx",
      expected: "provider_model_picker",
    },
    {
      name: "composer model picker regression",
      path: "src/renderer/src/components/chat/FloatingComposer.test.ts",
      expected: "provider_model_picker",
    },
    {
      name: "other plugin manifest",
      path: "plugins/analytix-computer-use/.codex-plugin/plugin.json",
      expected: "other_plugin_manifest",
    },
    {
      name: "unknown unrelated path",
      path: "some/other/file.txt",
      expected: "other",
    },
  ];
  const results = cases.map((testCase) => {
    const actual = unrelatedDirtyGroupForPath(testCase.path);
    return {
      ...testCase,
      actual,
      ok: actual === testCase.expected,
    };
  });
  const grouped = groupDirtyEntries(
    cases.map((testCase) => ({ status: " M", path: testCase.path })),
  );
  const split = splitGoalWideUnrelatedDirtyGroups(grouped);
  const splitOK =
    split.goalWideCount === 52 &&
    split.unknownCount === 1 &&
    split.unknownGroups.length === 1 &&
    split.unknownGroups[0].group === "other";
  return {
    ok: results.every((result) => result.ok) && splitOK,
    results,
    split,
    splitOK,
  };
}

function groupDirtyEntries(entries) {
  const groups = new Map();
  for (const entry of entries) {
    const group = unrelatedDirtyGroupForPath(entry.path);
    const current = groups.get(group) || {
      group,
      count: 0,
      paths: [],
    };
    current.count += 1;
    current.paths.push(entry.path);
    groups.set(group, current);
  }
  return [...groups.values()].sort((left, right) =>
    left.group.localeCompare(right.group),
  );
}

function dirtyGroupPathCount(groups) {
  return groups.reduce((total, group) => total + Number(group.count || 0), 0);
}

function splitGoalWideUnrelatedDirtyGroups(groups) {
  const goalWideGroups = [];
  const unknownGroups = [];
  for (const group of groups) {
    if (GOAL_WIDE_UNRELATED_DIRTY_GROUPS.has(group.group))
      goalWideGroups.push(group);
    else unknownGroups.push(group);
  }
  return {
    goalWideGroups,
    unknownGroups,
    goalWideCount: dirtyGroupPathCount(goalWideGroups),
    unknownCount: dirtyGroupPathCount(unknownGroups),
  };
}

function gitTagsAtHead() {
  const output = runGit(["tag", "--points-at", "HEAD"]);
  return output
    ? output
        .split("\n")
        .map((tag) => tag.trim())
        .filter(Boolean)
    : [];
}

function readJson(relativePath) {
  return JSON.parse(
    fs.readFileSync(path.join(REPO_ROOT, relativePath), "utf8"),
  );
}

function readText(relativePath) {
  return fs.readFileSync(path.join(REPO_ROOT, relativePath), "utf8");
}

function serverVersion() {
  const source = readText("plugins/analytix-fund-analysis/mcp/server.mjs");
  const match = source.match(/SERVER_VERSION\s*=\s*"([^"]+)"/u);
  return match ? match[1] : "";
}

function sourceConstantVersion(relativePath, constantName) {
  const source = readText(relativePath);
  const escapedName = constantName.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
  const match = source.match(
    new RegExp(`${escapedName}\\s*=\\s*"([^"]+)"`, "u"),
  );
  return match ? match[1] : "";
}

function pluginRuntimeVersionGaps(expectedVersion) {
  if (!expectedVersion) return ["plugin manifest version is missing"];
  const contracts = [
    ["mcp/server.mjs", "SERVER_VERSION"],
    [
      "mcp/mcp-request-handler-runtime.mjs",
      "MCP_REQUEST_HANDLER_RUNTIME_VERSION",
    ],
    ["scripts/eval-coverage-contract.mjs", "EVAL_COVERAGE_CONTRACT_VERSION"],
  ];
  const gaps = contracts.flatMap(([suffix, constantName]) => {
    const relativePath = `plugins/analytix-fund-analysis/${suffix}`;
    const observed = sourceConstantVersion(relativePath, constantName);
    return observed === expectedVersion
      ? []
      : [
          `${relativePath} ${constantName}=${observed || "missing"} expected ${expectedVersion}`,
        ];
  });
  try {
    inspectProductionMcpEntryClosure(PLUGIN_ROOT);
  } catch (error) {
    gaps.push(
      `production MCP entry closure mismatch: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  const cacheVersion = sourceConstantVersion(
    "plugins/analytix-fund-analysis/scripts/runtime-cache-contract.mjs",
    "RUNTIME_CACHE_CONTRACT_VERSION",
  );
  if (cacheVersion !== `${expectedVersion}-runtime-cache-contract`) {
    gaps.push(
      `plugins/analytix-fund-analysis/scripts/runtime-cache-contract.mjs RUNTIME_CACHE_CONTRACT_VERSION=${cacheVersion || "missing"} expected ${expectedVersion}-runtime-cache-contract`,
    );
  }
  const probeVersion = sourceConstantVersion(
    "plugins/analytix-fund-analysis/scripts/source-live-probe-contract.mjs",
    "SOURCE_LIVE_PROBE_CONTRACT_VERSION",
  );
  if (probeVersion !== expectedVersion) {
    gaps.push(
      `plugins/analytix-fund-analysis/scripts/source-live-probe-contract.mjs SOURCE_LIVE_PROBE_CONTRACT_VERSION=${probeVersion || "missing"} expected ${expectedVersion}`,
    );
  }
  const goWireProbeVersion = sourceConstantVersion(
    "plugins/analytix-fund-analysis/scripts/source-live-probe-go-wire-contract.mjs",
    "SOURCE_LIVE_PROBE_GO_WIRE_CONTRACT_VERSION",
  );
  if (goWireProbeVersion !== expectedVersion) {
    gaps.push(
      `plugins/analytix-fund-analysis/scripts/source-live-probe-go-wire-contract.mjs SOURCE_LIVE_PROBE_GO_WIRE_CONTRACT_VERSION=${goWireProbeVersion || "missing"} expected ${expectedVersion}`,
    );
  }
  const managerSource = readText("packages/runtime-go/internal/mcp/manager.go");
  const hostFundsVersion =
    managerSource.match(/fundsEvidenceKernelVersion\s*=\s*"([^"]+)"/u)?.[1] ||
    "";
  if (hostFundsVersion !== expectedVersion) {
    gaps.push(
      `packages/runtime-go/internal/mcp/manager.go fundsEvidenceKernelVersion=${hostFundsVersion || "missing"} expected ${expectedVersion}`,
    );
  }
  if (
    !managerSource.includes('spec.ReadOnlyToolNames["count_case_rows"] = true')
  ) {
    gaps.push(
      "packages/runtime-go/internal/mcp/manager.go missing exact count_case_rows host read-only allowlist",
    );
  }
  return gaps;
}

function bumpPatchVersion(version) {
  const match = String(version || "").match(/^(\d+)\.(\d+)\.(\d+)(.*)$/u);
  if (!match) return "";
  return `${match[1]}.${match[2]}.${Number(match[3]) + 1}${match[4] || ""}`;
}

function nextAvailablePatchVersion(
  version,
  tagPrefix = "analytix-fund-analysis-v",
  maxAttempts = 100,
) {
  let nextVersion = bumpPatchVersion(version);
  for (let attempt = 0; attempt < maxAttempts && nextVersion; attempt += 1) {
    if (!tagExists(`${tagPrefix}${nextVersion}`)) return nextVersion;
    nextVersion = bumpPatchVersion(nextVersion);
  }
  return nextVersion;
}

function validSemverPatchVersion(version) {
  return /^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/u.test(String(version || ""));
}

const NATIVE_ANALYSIS_COMPUTE_REQUIRED_COMMANDS = [
  "stats-query-worker",
  "query-stats-tree",
  "query-stats-date-range",
  "query-stats-txn-rows",
];

function tagExists(tag) {
  if (!tag) return false;
  const result = runCommand(
    "git",
    ["rev-parse", "--verify", "--quiet", `refs/tags/${tag}`],
    {
      allowStatus: [0, 1],
    },
  );
  return result.status === 0;
}

function releaseTagStatus(tag, exists, atHead) {
  const name = text(tag);
  if (!name) {
    return {
      tag: "",
      exists: false,
      at_head: false,
      state: "missing_name",
      blocker: "current release tag is missing",
    };
  }
  if (!exists) {
    return {
      tag: name,
      exists: false,
      at_head: false,
      state: "missing",
      blocker: `${name} does not exist`,
    };
  }
  if (!atHead) {
    return {
      tag: name,
      exists: true,
      at_head: false,
      state: "stale",
      blocker: `${name} does not point at HEAD`,
    };
  }
  return {
    tag: name,
    exists: true,
    at_head: true,
    state: "at_head",
    blocker: "",
  };
}

function selfTestReleaseTagStatus() {
  const cases = [
    {
      name: "missing tag name",
      args: ["", false, false],
      expectedState: "missing_name",
      expectedBlocker: "current release tag is missing",
    },
    {
      name: "missing tag",
      args: ["analytix-fund-analysis-v0.16.9", false, false],
      expectedState: "missing",
      expectedBlocker: "analytix-fund-analysis-v0.16.9 does not exist",
    },
    {
      name: "stale tag",
      args: ["analytix-fund-analysis-v0.16.9", true, false],
      expectedState: "stale",
      expectedBlocker: "analytix-fund-analysis-v0.16.9 does not point at HEAD",
    },
    {
      name: "tag at head",
      args: ["analytix-fund-analysis-v0.16.9", true, true],
      expectedState: "at_head",
      expectedBlocker: "",
    },
  ];
  const results = cases.map((testCase) => {
    const actual = releaseTagStatus(...testCase.args);
    return {
      ...testCase,
      actual,
      ok:
        actual.state === testCase.expectedState &&
        actual.blocker === testCase.expectedBlocker,
    };
  });
  return {
    ok: results.every((result) => result.ok),
    results,
  };
}

function buildReleaseSliceSummary(audit) {
  const identity = objectOf(audit.current_release_identity);
  const boundary = objectOf(audit.unrelated_dirty_release_boundary);
  const isolation = objectOf(audit.isolation_check);
  const stagingPlan = objectOf(audit.staging_plan);
  const readinessPlan = objectOf(audit.readiness_plan);
  const functionalEvidence = objectOf(audit.functional_closure_evidence);
  const publishBlockers = arrayOf(audit.publish_blockers)
    .map(text)
    .filter(Boolean);
  const summary = {
    schemaVersion: 1,
    id: "analytix-fund-analysis-release-slice-summary",
    status: publishBlockers.length ? "blocked" : "ready",
    head: text(audit.head),
    current_release_identity: {
      manifest_version: text(identity.manifest_version),
      server_version: text(identity.server_version),
      current_release_tag: text(identity.current_release_tag),
      current_release_tag_exists: identity.current_release_tag_exists === true,
      current_release_tag_at_head:
        identity.current_release_tag_at_head === true,
      current_release_tag_state: text(identity.current_release_tag_state),
      publish_identity_ready: identity.publish_identity_ready === true,
    },
    dirty: {
      total: Number(audit.dirty_total || 0),
      generated_evidence: Number(audit.generated_evidence_dirty_count || 0),
      release_slice: Number(audit.related_dirty_count || 0),
      unrelated: Number(audit.unrelated_dirty_count || 0),
      goal_wide_unrelated: Number(audit.goal_wide_unrelated_dirty_count || 0),
      unknown_unrelated: Number(audit.unknown_unrelated_dirty_count || 0),
    },
    release_boundary: {
      all_unrelated_paths_classified:
        boundary.all_unrelated_paths_classified === true,
      goal_wide_groups_are_release_blocking:
        boundary.goal_wide_groups_are_release_blocking === true,
      release_slice_commit_must_exclude_goal_wide_paths:
        boundary.release_slice_commit_must_exclude_goal_wide_paths === true,
    },
    publish_blocker_count: publishBlockers.length,
    publish_blockers: publishBlockers,
    missing_release_slice_files: arrayOf(audit.missing_release_slice_files)
      .map(text)
      .filter(Boolean),
  };
  if (Object.keys(isolation).length) {
    summary.isolation_check = {
      ok: isolation.ok === true,
      isolated_dirty_total: Number(isolation.isolated_dirty_total || 0),
      isolated_related_dirty_count: Number(
        isolation.isolated_related_dirty_count || 0,
      ),
      isolated_unrelated_dirty_count: Number(
        isolation.isolated_unrelated_dirty_count || 0,
      ),
      node_check_count: arrayOf(isolation.node_check_files).length,
      doctor_skip_git_runtime_ok: isolation.doctor_skip_git_runtime_ok === true,
      doctor_result_count: Number(isolation.doctor_result_count || 0),
      cleanup: {
        worktree_removed: objectOf(isolation.cleanup).worktree_removed === true,
        temp_root_removed:
          objectOf(isolation.cleanup).temp_root_removed === true,
      },
    };
  }
  if (Object.keys(stagingPlan).length) {
    const goalWideKeepOutCount = Number(
      stagingPlan.goal_wide_unrelated_paths_to_keep_out_count ??
        stagingPlan.leave_unstaged_goal_wide_count ??
        0,
    );
    const unknownKeepOutCount = Number(
      stagingPlan.unknown_unrelated_paths_to_keep_out_count ??
        stagingPlan.leave_unstaged_unknown_count ??
        0,
    );
    summary.staging_plan = {
      ready_for_authorized_stage:
        stagingPlan.ready_for_authorized_stage === true,
      stage_path_count: Number(stagingPlan.stage_path_count || 0),
      leave_unstaged_count: Number(stagingPlan.leave_unstaged_count || 0),
      goal_wide_unrelated_paths_to_keep_out_count: Number.isFinite(
        goalWideKeepOutCount,
      )
        ? goalWideKeepOutCount
        : 0,
      unknown_unrelated_paths_to_keep_out_count: Number.isFinite(
        unknownKeepOutCount,
      )
        ? unknownKeepOutCount
        : 0,
      mutates_index: stagingPlan.mutates_index === true,
      mutates_files: stagingPlan.mutates_files === true,
    };
  }
  if (Object.keys(readinessPlan).length) {
    summary.readiness_plan = {
      can_publish_current_tree: readinessPlan.can_publish_current_tree === true,
      candidate_version_ready: readinessPlan.candidate_version_ready === true,
      current_version: text(readinessPlan.current_version),
      candidate_version: text(readinessPlan.candidate_version),
      blocker_count: arrayOf(readinessPlan.blockers).length,
      candidate_blocker_count: arrayOf(readinessPlan.candidate_blockers).length,
      authorization_required_before: arrayOf(
        readinessPlan.authorization_required_before,
      )
        .map(text)
        .filter(Boolean),
    };
  }
  if (Object.keys(functionalEvidence).length) {
    summary.functional_closure_evidence = {
      ready: functionalEvidence.ready === true,
      local_non_backend_ready:
        functionalEvidence.local_non_backend_ready === true,
      task_count: Number(functionalEvidence.task_count || 0),
      blocker_count: arrayOf(functionalEvidence.blockers).length,
    };
  }
  return summary;
}

function selfTestSummaryJson() {
  const summary = buildReleaseSliceSummary({
    head: "abc123",
    current_release_identity: {
      manifest_version: "0.16.9",
      server_version: "0.16.9",
      current_release_tag: "analytix-fund-analysis-v0.16.9",
      current_release_tag_exists: false,
      current_release_tag_at_head: false,
      current_release_tag_state: "missing",
      publish_identity_ready: false,
    },
    dirty_total: 53,
    generated_evidence_dirty_count: 748,
    related_dirty_count: 36,
    unrelated_dirty_count: 17,
    goal_wide_unrelated_dirty_count: 17,
    unknown_unrelated_dirty_count: 0,
    unrelated_dirty_release_boundary: {
      all_unrelated_paths_classified: true,
      goal_wide_groups_are_release_blocking: true,
      release_slice_commit_must_exclude_goal_wide_paths: true,
    },
    staging_plan: {
      ready_for_authorized_stage: true,
      stage_path_count: 36,
      stage_paths: [{ path: "large-stage-list-must-not-leak" }],
      leave_unstaged_count: 17,
      leave_unstaged_paths: [{ path: "large-leave-list-must-not-leak" }],
      leave_unstaged_goal_wide_count: 17,
      leave_unstaged_unknown_count: 0,
      mutates_index: false,
      mutates_files: false,
    },
    readiness_plan: {
      can_publish_current_tree: false,
      candidate_version_ready: true,
      current_version: "0.16.9",
      candidate_version: "0.16.9",
      blockers: ["missing tag", "dirty tree"],
      candidate_blockers: [],
      authorization_required_before: [
        "version file edits",
        "git add",
        "git commit",
        "git tag",
        "git push",
        "Hub publish",
        "marketplace update",
      ],
    },
    publish_blockers: ["missing tag", "dirty tree"],
    missing_release_slice_files: [],
    related_dirty: [{ path: "large-path-list-must-not-leak" }],
    unrelated_dirty: [{ path: "large-unrelated-list-must-not-leak" }],
    isolation_check: {
      ok: true,
      isolated_dirty_total: 36,
      isolated_related_dirty_count: 36,
      isolated_unrelated_dirty_count: 0,
      node_check_files: [{ ok: true }, { ok: true }],
      doctor_skip_git_runtime_ok: true,
      doctor_result_count: 151,
      cleanup: {
        worktree_removed: true,
        temp_root_removed: true,
      },
    },
  });
  const serialized = JSON.stringify(summary);
  const ok =
    summary.status === "blocked" &&
    summary.publish_blocker_count === 2 &&
    summary.publish_blockers.length === summary.publish_blocker_count &&
    summary.dirty.total === 53 &&
    summary.dirty.release_slice === 36 &&
    summary.dirty.goal_wide_unrelated === 17 &&
    summary.staging_plan?.stage_path_count === 36 &&
    summary.staging_plan?.leave_unstaged_count === 17 &&
    summary.staging_plan?.goal_wide_unrelated_paths_to_keep_out_count === 17 &&
    summary.staging_plan?.unknown_unrelated_paths_to_keep_out_count === 0 &&
    summary.staging_plan?.mutates_index === false &&
    summary.readiness_plan?.can_publish_current_tree === false &&
    summary.readiness_plan?.candidate_version_ready === true &&
    summary.readiness_plan?.blocker_count === 2 &&
    summary.readiness_plan?.candidate_blocker_count === 0 &&
    summary.readiness_plan?.authorization_required_before.length === 7 &&
    summary.readiness_plan?.authorization_required_before.includes(
      "git push",
    ) &&
    summary.readiness_plan?.authorization_required_before.includes(
      "Hub publish",
    ) &&
    summary.readiness_plan?.authorization_required_before.includes(
      "marketplace update",
    ) &&
    summary.isolation_check?.node_check_count === 2 &&
    summary.isolation_check?.doctor_result_count === 151 &&
    !serialized.includes("large-path-list-must-not-leak") &&
    !serialized.includes("large-unrelated-list-must-not-leak") &&
    !serialized.includes("large-stage-list-must-not-leak") &&
    !serialized.includes("large-leave-list-must-not-leak");
  return { ok, summary };
}

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value
    : {};
}

function numberValue(value) {
  const next = Number(value ?? 0);
  return Number.isFinite(next) ? next : 0;
}

function insideDirectory(parent, child) {
  const relative = path.relative(parent, child);
  return (
    Boolean(relative) &&
    !relative.startsWith("..") &&
    !path.isAbsolute(relative)
  );
}

function resolveQualityEvidencePath(input) {
  const requested = text(input);
  if (!requested) return "";
  return path.resolve(
    path.isAbsolute(requested) ? requested : path.join(REPO_ROOT, requested),
  );
}

function packagedSmokePlatformDir(
  platform = process.platform,
  arch = process.arch,
) {
  if (platform === "darwin") return arch === "arm64" ? "mac-arm64" : "mac-x64";
  if (platform === "win32") return "win";
  return "linux";
}

function nativeAnalysisComputeBinaryPaths({
  platform = process.platform,
  arch = process.arch,
  root = REPO_ROOT,
} = {}) {
  const binaryName =
    platform === "win32"
      ? "analytix-analysis-compute.exe"
      : "analytix-analysis-compute";
  const smokeDir = packagedSmokePlatformDir(platform, arch);
  return [
    ...new Set([
      path.join(root, "runtime", binaryName),
      path.join(
        root,
        "dist",
        "codex-packaged-smoke",
        smokeDir,
        "analytix.app",
        "Contents",
        "Resources",
        "runtime",
        binaryName,
      ),
      path.join(
        root,
        "dist",
        "codex-packaged-smoke",
        "mac-arm64",
        "analytix.app",
        "Contents",
        "Resources",
        "runtime",
        binaryName,
      ),
      path.join(
        root,
        "dist",
        "codex-packaged-smoke",
        "mac-x64",
        "analytix.app",
        "Contents",
        "Resources",
        "runtime",
        binaryName,
      ),
      path.join(
        root,
        "dist",
        "codex-packaged-smoke",
        "win",
        "resources",
        "runtime",
        binaryName,
      ),
      path.join(
        root,
        "dist",
        "codex-packaged-smoke",
        "linux",
        "resources",
        "runtime",
        binaryName,
      ),
    ]),
  ];
}

function buildNativeAnalysisComputeEvidence({
  platform = process.platform,
  arch = process.arch,
  root = REPO_ROOT,
} = {}) {
  const candidatePaths = nativeAnalysisComputeBinaryPaths({
    platform,
    arch,
    root,
  });
  const binaryPath =
    candidatePaths.find((candidate) => fs.existsSync(candidate)) ||
    candidatePaths[0];
  const relativeBinaryPath = path
    .relative(root, binaryPath)
    .split(path.sep)
    .join("/");
  const relativeCandidatePaths = candidatePaths.map((candidate) =>
    path.relative(root, candidate).split(path.sep).join("/"),
  );
  const binaryExists = fs.existsSync(binaryPath);
  const blockers = [];
  let binarySizeBytes = 0;
  let helpStatus = null;
  let helpOutput = "";
  let commandSurfaceMissing = [...NATIVE_ANALYSIS_COMPUTE_REQUIRED_COMMANDS];
  if (!binaryExists) {
    blockers.push(
      `packaged runtime binary is missing (checked: ${relativeCandidatePaths.join(", ")})`,
    );
  } else {
    const stat = fs.statSync(binaryPath);
    binarySizeBytes = stat.size;
    if (binarySizeBytes <= 0) blockers.push(`${relativeBinaryPath} is empty`);
    const help = runCommand(binaryPath, ["--help"], { allowStatus: [0, 1] });
    helpStatus = help.status;
    helpOutput = `${help.stdout || ""}\n${help.stderr || ""}`;
    commandSurfaceMissing = NATIVE_ANALYSIS_COMPUTE_REQUIRED_COMMANDS.filter(
      (command) => !helpOutput.includes(command),
    );
    if (helpStatus !== 0)
      blockers.push(
        `${relativeBinaryPath} --help exited with status ${helpStatus}`,
      );
    if (commandSurfaceMissing.length) {
      blockers.push(
        `missing required command(s): ${commandSurfaceMissing.join(", ")}`,
      );
    }
  }
  return {
    read_only: true,
    mutates_files: false,
    mutates_index: false,
    binary_path: relativeBinaryPath,
    candidate_binary_paths: relativeCandidatePaths,
    binary_exists: binaryExists,
    binary_size_bytes: binarySizeBytes,
    verified_by: `${relativeBinaryPath} --help`,
    help_status: helpStatus,
    required_command_surface: NATIVE_ANALYSIS_COMPUTE_REQUIRED_COMMANDS,
    command_surface_missing: commandSurfaceMissing,
    command_surface_ok:
      binaryExists && commandSurfaceMissing.length === 0 && helpStatus === 0,
    packaged_runtime_evidence_ready: blockers.length === 0,
    blockers,
  };
}

function selfTestNativeRuntimeEvidence() {
  const tempRoot = fs.mkdtempSync(
    path.join(os.tmpdir(), "analytix-native-runtime-evidence-"),
  );
  try {
    const binaryPath = path.join(
      tempRoot,
      "dist",
      "codex-packaged-smoke",
      "mac-arm64",
      "analytix.app",
      "Contents",
      "Resources",
      "runtime",
      "analytix-analysis-compute",
    );
    fs.mkdirSync(path.dirname(binaryPath), { recursive: true });
    fs.writeFileSync(
      binaryPath,
      [
        "#!/usr/bin/env sh",
        "echo 'stats-query-worker query-stats-tree query-stats-date-range query-stats-txn-rows'",
      ].join("\n"),
      "utf8",
    );
    fs.chmodSync(binaryPath, 0o755);
    const evidence = buildNativeAnalysisComputeEvidence({
      platform: "darwin",
      arch: "arm64",
      root: tempRoot,
    });
    return {
      ok:
        evidence.packaged_runtime_evidence_ready === true &&
        evidence.binary_path ===
          "dist/codex-packaged-smoke/mac-arm64/analytix.app/Contents/Resources/runtime/analytix-analysis-compute" &&
        evidence.command_surface_ok === true &&
        evidence.command_surface_missing.length === 0,
      evidence,
    };
  } finally {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

function normalizeEvidencePath(relativeOrAbsolutePath, fallbackPath) {
  const requested = text(relativeOrAbsolutePath) || fallbackPath;
  const absolutePath = path.resolve(REPO_ROOT, requested);
  return {
    absolutePath,
    relativePath: path
      .relative(REPO_ROOT, absolutePath)
      .split(path.sep)
      .join("/"),
  };
}

function buildMcpSurfaceSnapshotEvidence(manifestVersion, evidencePath = "") {
  const evidence = normalizeEvidencePath(
    evidencePath,
    DEFAULT_MCP_SURFACE_SNAPSHOT_PATH,
  );
  const { absolutePath } = evidence;
  const { relativePath } = evidence;
  const contractBlockers = [];
  let snapshot = {};
  if (!fs.existsSync(absolutePath)) {
    contractBlockers.push(`${relativePath} is missing`);
  } else {
    try {
      const stat = fs.lstatSync(absolutePath);
      if (stat.isSymbolicLink() || !stat.isFile()) {
        throw new Error("MCP surface evidence must be a regular non-symlink file");
      }
      snapshot = parseStrictJsonObject(fs.readFileSync(absolutePath), {
        maxBytes: 1 << 20,
        maxDepth: 20,
        maxTokens: 16_384,
        maxStringBytes: 128 << 10,
        maxNumberBytes: 128,
      });
    } catch (error) {
      contractBlockers.push(
        `${relativePath} is not valid strict evidence JSON: ${error instanceof Error ? error.message : String(error)}`,
      );
    }
  }
  const plugin = objectOf(snapshot.plugin);
  const mcp = objectOf(snapshot.mcp);
  let validation = {
    contract_ready: false,
    publication_ready: false,
    release_blockers: [
      "production_mcp_disabled",
      "production_mcp_p0_quarantine",
      "dataset_snapshot_authority_unavailable",
    ],
    checks: {},
    blockers: [],
  };
  if (!contractBlockers.length) {
    try {
      validation = validateFundsMcpSurfaceSnapshotV2(snapshot);
      contractBlockers.push(...validation.blockers);
    } catch (error) {
      contractBlockers.push(
        `MCP surface evidence validation failed: ${error instanceof Error ? error.message : String(error)}`,
      );
    }
  }
  if (text(plugin.version) !== manifestVersion) {
    contractBlockers.push(
      `snapshot plugin version ${text(plugin.version) || "missing"} != current ${manifestVersion}`,
    );
  }
  const contractReady =
    validation.contract_ready === true && contractBlockers.length === 0;
  const publicationBlockers = uniqueTextList(validation.release_blockers);
  const publicationReady = false;
  return {
    read_only: true,
    mutates_files: false,
    mutates_index: false,
    diagnostic_only: true,
    path: relativePath,
    schema_version: text(snapshot.schema_version),
    legacy_snapshot: text(snapshot.schema_version) !== "FundsMcpSurfaceSnapshotV2",
    generated_at: text(snapshot.generated_at),
    plugin_version: text(plugin.version),
    surface_mode: contractReady
      ? "p0_source_unavailable_quarantine"
      : "invalid_or_stale_snapshot",
    source_binding_matches_current:
      objectOf(validation.checks).source_binding_current === true,
    tool_count: arrayOf(mcp.tools).length,
    resource_count: arrayOf(mcp.resources).length,
    resource_template_count: arrayOf(mcp.resource_templates).length,
    prompt_count: arrayOf(mcp.prompts).length,
    contract_ready: contractReady,
    containment_ready: contractReady,
    fact_execution_ready: false,
    publication_ready: publicationReady,
    ready: publicationReady,
    contract_blockers: uniqueTextList(contractBlockers),
    publication_blockers: publicationBlockers,
    blockers: uniqueTextList([
      ...contractBlockers,
      ...publicationBlockers,
    ]),
  };
}

function sha256Text(value) {
  return crypto.createHash("sha256").update(value).digest("hex");
}

function selfTestMcpSurfaceEvidence() {
  const tempRoot = fs.mkdtempSync(
    path.join(os.tmpdir(), "analytix-mcp-surface-evidence-"),
  );
  const manifestVersion = text(
    readJson("plugins/analytix-fund-analysis/.codex-plugin/plugin.json")
      .version,
  );
  const snapshotPath = path.join(tempRoot, "valid-v2.json");
  const writeJsonFile = (name, value) => {
    const filePath = path.join(tempRoot, `${name}.json`);
    fs.writeFileSync(filePath, `${JSON.stringify(value, null, 2)}\n`);
    return filePath;
  };
  const refreshTranscript = (value) => {
    value.transcript_sha256 = sha256Text(JSON.stringify(value.mcp));
    return value;
  };
  try {
    const capture = spawnSync(
      process.execPath,
      [
        path.join(SCRIPT_DIR, "mcp-surface-snapshot.mjs"),
        "--output",
        snapshotPath,
        "--fail-on-violation",
        "--json",
      ],
      { cwd: REPO_ROOT, encoding: "utf8" },
    );
    if (capture.status !== 0) {
      return {
        ok: false,
        capture_status: capture.status,
        capture_error: text(capture.stderr || capture.stdout).slice(0, 500),
        results: [],
      };
    }
    const baseline = JSON.parse(fs.readFileSync(snapshotPath, "utf8"));
    const cases = [];
    const addCase = (name, filePath, expectedContractReady = false) => {
      const evidence = buildMcpSurfaceSnapshotEvidence(
        manifestVersion,
        filePath,
      );
      cases.push({
        name,
        ok:
          evidence.contract_ready === expectedContractReady &&
          evidence.publication_ready === false,
        contract_ready: evidence.contract_ready,
        publication_ready: evidence.publication_ready,
        blockers: evidence.contract_blockers,
      });
    };
    addCase("valid_v2_is_diagnostic_only", snapshotPath, true);

    addCase(
      "legacy_same_version_rejected",
      writeJsonFile("legacy", {
        generated_at: baseline.generated_at,
        plugin: baseline.plugin,
        mcp: { tool_count: 30, tools: ["count_case_rows"] },
        ok: true,
      }),
    );

    const sourceHashMismatch = structuredClone(baseline);
    sourceHashMismatch.source_binding.aggregate_sha256 = "0".repeat(64);
    addCase(
      "source_hash_mismatch_rejected",
      writeJsonFile("source-hash-mismatch", sourceHashMismatch),
    );

    const nonemptyTools = structuredClone(baseline);
    nonemptyTools.mcp.tools.push("run_full_case_analysis");
    nonemptyTools.mcp.tools.sort();
    nonemptyTools.mcp.tool_count += 1;
    nonemptyTools.mcp.raw_observations.tools_list.tools.push({
      name: "run_full_case_analysis",
    });
    addCase(
      "nonempty_tools_rejected",
      writeJsonFile("nonempty-tools", refreshTranscript(nonemptyTools)),
    );

    const extraResource = structuredClone(baseline);
    const staleResource = {
      uri: "analytix://funds/stale-resource",
      name: "stale",
      description: "stale",
      mimeType: "text/plain",
    };
    extraResource.mcp.resources.push(staleResource);
    extraResource.mcp.resource_count += 1;
    extraResource.mcp.raw_observations.resources_list.resources.push(
      staleResource,
    );
    addCase(
      "extra_resource_rejected",
      writeJsonFile("extra-resource", refreshTranscript(extraResource)),
    );

    const promptArguments = structuredClone(baseline);
    const forbiddenArgument = {
      name: "case_id",
      description: "must not exist",
      required: false,
    };
    promptArguments.mcp.prompts[0].arguments.push(forbiddenArgument);
    promptArguments.mcp.raw_observations.prompts_list.prompts[0].arguments.push(
      forbiddenArgument,
    );
    addCase(
      "prompt_arguments_rejected",
      writeJsonFile("prompt-arguments", refreshTranscript(promptArguments)),
    );

    const boundaryDrift = structuredClone(baseline);
    const driftedText = `${boundaryDrift.mcp.raw_observations.resource_read.contents[0].text}x`;
    boundaryDrift.mcp.raw_observations.resource_read.contents[0].text =
      driftedText;
    boundaryDrift.mcp.resource_read.text_sha256 = sha256Text(driftedText);
    addCase(
      "boundary_drift_rejected",
      writeJsonFile("boundary-drift", refreshTranscript(boundaryDrift)),
    );

    const publicationForgery = structuredClone(baseline);
    publicationForgery.publication_ready = true;
    addCase(
      "publication_ready_forgery_rejected",
      writeJsonFile("publication-forgery", publicationForgery),
    );

    const nestedUnknown = structuredClone(baseline);
    nestedUnknown.mcp.releaseEligible = true;
    addCase(
      "nested_unknown_property_rejected",
      writeJsonFile("nested-unknown", refreshTranscript(nestedUnknown)),
    );

    const duplicateKeyPath = path.join(tempRoot, "duplicate-key.json");
    fs.writeFileSync(
      duplicateKeyPath,
      '{"schema_version":"FundsMcpSurfaceSnapshotV2","schema_version":"FundsMcpSurfaceSnapshotV2"}',
    );
    addCase("duplicate_json_key_rejected", duplicateKeyPath);

    const invalidUtf8Path = path.join(tempRoot, "invalid-utf8.json");
    fs.writeFileSync(invalidUtf8Path, Buffer.from([0xff, 0xfe, 0xfd]));
    addCase("invalid_utf8_rejected", invalidUtf8Path);

    const oversizedPath = path.join(tempRoot, "oversized.json");
    fs.writeFileSync(oversizedPath, Buffer.alloc((1 << 20) + 1, 0x20));
    addCase("oversized_evidence_rejected", oversizedPath);

    const symlinkPath = path.join(tempRoot, "snapshot-symlink.json");
    fs.symlinkSync(snapshotPath, symlinkPath);
    addCase("symlink_evidence_rejected", symlinkPath);

    return {
      ok: cases.every((testCase) => testCase.ok),
      capture_status: capture.status,
      result_count: cases.length,
      results: cases,
    };
  } finally {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

function modeBucket(scoring, mode) {
  return objectOf(objectOf(scoring).by_mode?.[mode]);
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function uniqueTextList(value) {
  return [...new Set(arrayOf(value).map(text).filter(Boolean))];
}

function buildFunctionalClosureEvidenceCheck(audit, evidencePath) {
  const outputRoot = path.join(REPO_ROOT, FUNCTIONAL_CLOSURE_OUTPUT_ROOT);
  const resolvedPath = resolveQualityEvidencePath(evidencePath);
  const manifest = readJson(
    "plugins/analytix-fund-analysis/.codex-plugin/plugin.json",
  );
  const hardBlockers = [];
  const releaseOnlyBlockers = [];
  let payload = {};
  if (!resolvedPath) {
    hardBlockers.push("functional closure evidence path is missing");
  } else if (!insideDirectory(outputRoot, resolvedPath)) {
    hardBlockers.push(
      `functional closure evidence must be under ${FUNCTIONAL_CLOSURE_OUTPUT_ROOT}`,
    );
  } else if (!fs.existsSync(resolvedPath)) {
    hardBlockers.push(
      `functional closure evidence file does not exist: ${path.relative(REPO_ROOT, resolvedPath)}`,
    );
  } else {
    payload = JSON.parse(fs.readFileSync(resolvedPath, "utf8"));
  }
  const identity = objectOf(payload.release_identity);
  const results = arrayOf(payload.results).map(objectOf);
  const passedFamilies = new Set(
    results
      .filter((result) => text(result.status) === "passed")
      .map((result) => text(result.family))
      .filter(Boolean),
  );
  const failedResults = results.filter(
    (result) => text(result.status) === "failed",
  );
  const skippedResults = results.filter(
    (result) => text(result.status) === "skipped",
  );
  const backendSkippedResults = skippedResults.filter((result) =>
    /requires live backend|--skip-backend/iu.test(text(result.reason)),
  );
  const nonBackendSkippedResults = skippedResults.filter(
    (result) => !backendSkippedResults.includes(result),
  );
  const missingFamilies = FUNCTIONAL_CLOSURE_REQUIRED_FAMILIES.filter(
    (family) => !passedFamilies.has(family),
  );
  const backendSkippedFamilies = new Set(
    backendSkippedResults.map((result) => text(result.family)).filter(Boolean),
  );
  const missingLiveBackendFamilies = missingFamilies.filter((family) =>
    backendSkippedFamilies.has(family),
  );
  const missingHardFamilies = missingFamilies.filter(
    (family) => !backendSkippedFamilies.has(family),
  );
  if (payload.status !== "ok")
    hardBlockers.push(
      `functional closure status is not ok: ${text(payload.status || "missing")}`,
    );
  if (!results.length)
    hardBlockers.push("functional closure evidence has no task results");
  if (failedResults.length)
    hardBlockers.push(
      `functional closure has failed task(s): ${failedResults
        .map((result) => text(result.id))
        .filter(Boolean)
        .join(", ")}`,
    );
  if (nonBackendSkippedResults.length)
    hardBlockers.push(
      `functional closure has skipped non-backend task(s): ${nonBackendSkippedResults
        .map((result) => text(result.id))
        .filter(Boolean)
        .join(", ")}`,
    );
  if (backendSkippedResults.length)
    releaseOnlyBlockers.push(
      `functional closure still needs live backend task(s): ${backendSkippedResults
        .map((result) => text(result.id))
        .filter(Boolean)
        .join(", ")}`,
    );
  if (missingHardFamilies.length)
    hardBlockers.push(
      `functional closure missing required families: ${missingHardFamilies.join(", ")}`,
    );
  if (missingLiveBackendFamilies.length)
    releaseOnlyBlockers.push(
      `functional closure missing live-backend families: ${missingLiveBackendFamilies.join(", ")}`,
    );
  if (!identity.manifest_version)
    hardBlockers.push(
      "functional closure missing release_identity.manifest_version",
    );
  if (
    identity.manifest_version &&
    identity.manifest_version !== String(manifest.version || "")
  ) {
    hardBlockers.push(
      `functional closure manifest version ${identity.manifest_version} != current ${manifest.version || ""}`,
    );
  }
  if (!identity.server_version)
    hardBlockers.push(
      "functional closure missing release_identity.server_version",
    );
  if (identity.server_version && identity.server_version !== serverVersion()) {
    hardBlockers.push(
      `functional closure server version ${identity.server_version} != current ${serverVersion()}`,
    );
  }
  if (!identity.head)
    hardBlockers.push("functional closure missing release_identity.head");
  if (
    identity.head &&
    audit.head &&
    identity.head !== audit.head &&
    !identity.head.startsWith(audit.head)
  ) {
    hardBlockers.push(
      `functional closure HEAD ${identity.head} != current ${audit.head}`,
    );
  }
  const blockers = [...hardBlockers, ...releaseOnlyBlockers];
  return {
    read_only: true,
    mutates_files: false,
    mutates_index: false,
    path: resolvedPath ? path.relative(REPO_ROOT, resolvedPath) : "",
    required_families: FUNCTIONAL_CLOSURE_REQUIRED_FAMILIES,
    passed_families: [...passedFamilies],
    release_identity: identity,
    task_count: results.length,
    failed_count: failedResults.length,
    skipped_count: skippedResults.length,
    backend_skipped_count: backendSkippedResults.length,
    hard_blockers: hardBlockers,
    release_only_blockers: releaseOnlyBlockers,
    local_non_backend_ready: hardBlockers.length === 0,
    ready: blockers.length === 0,
    blockers,
  };
}

function buildCurrentReleaseIdentity(tagsAtHead, statusEntries = []) {
  const manifest = readJson(
    "plugins/analytix-fund-analysis/.codex-plugin/plugin.json",
  );
  const manifestVersion = String(manifest.version || "");
  const mcpServerVersion = serverVersion();
  const currentTag = manifestVersion
    ? `analytix-fund-analysis-v${manifestVersion}`
    : "";
  const releaseNotes = readText(
    "plugins/analytix-fund-analysis/RELEASE_NOTES.md",
  );
  const releaseNotesCurrent = manifestVersion
    ? releaseNotes.includes(`## ${manifestVersion}`)
    : false;
  const currentTagAtHead = currentTag ? tagsAtHead.includes(currentTag) : false;
  const currentTagExists = currentTag ? tagExists(currentTag) : false;
  const currentTagStatus = releaseTagStatus(
    currentTag,
    currentTagExists,
    currentTagAtHead,
  );
  const workingTreeClean = statusEntries.length === 0;
  const runtimeVersionGaps = pluginRuntimeVersionGaps(manifestVersion);
  const blockers = [
    manifestVersion !== mcpServerVersion
      ? `manifest/server version mismatch: ${manifestVersion} vs ${mcpServerVersion}`
      : "",
    ...runtimeVersionGaps.map(
      (gap) => `plugin runtime version mismatch: ${gap}`,
    ),
    !releaseNotesCurrent
      ? `RELEASE_NOTES.md missing ## ${manifestVersion}`
      : "",
    currentTagStatus.blocker,
    !workingTreeClean
      ? `release identity does not include ${statusEntries.length} dirty worktree path(s)`
      : "",
  ].filter(Boolean);
  return {
    manifest_version: manifestVersion,
    server_version: mcpServerVersion,
    current_release_tag: currentTag,
    current_release_tag_exists: currentTagExists,
    current_release_tag_at_head: currentTagAtHead,
    current_release_tag_state: currentTagStatus.state,
    release_notes_current: releaseNotesCurrent,
    runtime_version_gaps: runtimeVersionGaps,
    working_tree_clean: workingTreeClean,
    publish_identity_ready: blockers.length === 0,
    blockers,
  };
}

function buildAudit() {
  const releaseSliceSet = new Set(RELEASE_SLICE_FILES);
  const allStatusEntries = gitStatusEntries();
  const generatedEvidenceDirty = allStatusEntries.filter(
    isGeneratedEvidenceOutputPath,
  );
  const statusEntries = allStatusEntries.filter(
    (entry) => !isGeneratedEvidenceOutputPath(entry),
  );
  const relatedDirty = statusEntries.filter(
    (entry) => releaseSliceSet.has(entry.path) || isEvidenceCleanupPath(entry),
  );
  const unrelatedDirty = statusEntries.filter(
    (entry) =>
      !releaseSliceSet.has(entry.path) && !isEvidenceCleanupPath(entry),
  );
  const unrelatedDirtyGroups = groupDirtyEntries(unrelatedDirty);
  const goalWideUnrelatedDirty =
    splitGoalWideUnrelatedDirtyGroups(unrelatedDirtyGroups);
  const intentionalReleaseSliceDeletions = new Set(
    relatedDirty
      .filter((entry) => entry.status.trim() === "D")
      .map((entry) => entry.path),
  );
  const missingReleaseSliceFiles = RELEASE_SLICE_FILES.filter(
    (relativePath) =>
      !fs.existsSync(path.join(REPO_ROOT, relativePath)) &&
      !intentionalReleaseSliceDeletions.has(relativePath),
  );
  const cleanReleaseSliceFiles = RELEASE_SLICE_FILES.filter(
    (relativePath) =>
      fs.existsSync(path.join(REPO_ROOT, relativePath)) &&
      !relatedDirty.some((entry) => entry.path === relativePath),
  );
  const tagsAtHead = gitTagsAtHead();
  const currentReleaseIdentity = buildCurrentReleaseIdentity(
    tagsAtHead,
    statusEntries,
  );
  return {
    audit: "analytix-fund-analysis-release-slice",
    contract_version: 1,
    repo_root: REPO_ROOT,
    head: runGit(["rev-parse", "--short", "HEAD"]),
    tags_at_head: tagsAtHead,
    current_release_identity: currentReleaseIdentity,
    release_slice_files: RELEASE_SLICE_FILES,
    release_slice_count: RELEASE_SLICE_FILES.length,
    dirty_total: statusEntries.length,
    generated_evidence_dirty_count: generatedEvidenceDirty.length,
    generated_evidence_dirty: generatedEvidenceDirty,
    related_dirty_count: relatedDirty.length,
    unrelated_dirty_count: unrelatedDirty.length,
    related_dirty: relatedDirty,
    intentional_release_slice_deletions: [
      ...intentionalReleaseSliceDeletions,
    ].sort(),
    unrelated_dirty: unrelatedDirty,
    unrelated_dirty_groups: unrelatedDirtyGroups,
    goal_wide_unrelated_dirty_count: goalWideUnrelatedDirty.goalWideCount,
    goal_wide_unrelated_dirty_groups: goalWideUnrelatedDirty.goalWideGroups,
    unknown_unrelated_dirty_count: goalWideUnrelatedDirty.unknownCount,
    unknown_unrelated_dirty_groups: goalWideUnrelatedDirty.unknownGroups,
    unrelated_dirty_release_boundary: {
      all_unrelated_paths_classified:
        unrelatedDirty.length === goalWideUnrelatedDirty.goalWideCount,
      goal_wide_groups_are_release_blocking: true,
      release_slice_commit_must_exclude_goal_wide_paths: true,
    },
    clean_release_slice_files: cleanReleaseSliceFiles,
    eval_fixture_release_files: EVAL_FIXTURE_RELEASE_FILES,
    missing_release_slice_files: missingReleaseSliceFiles,
    publish_blockers: [
      ...currentReleaseIdentity.blockers,
      relatedDirty.length
        ? `release-slice changes are uncommitted: ${relatedDirty.length}`
        : "",
      unrelatedDirty.length
        ? `unrelated dirty paths: ${unrelatedDirty.length}`
        : "",
      missingReleaseSliceFiles.length
        ? `missing release-slice files: ${missingReleaseSliceFiles.length}`
        : "",
    ].filter(Boolean),
    boundaries: {
      read_only: true,
      workspace_read_only: true,
      source: "git status --porcelain=v1 --untracked-files=all",
      scope: "workspace release evidence only",
      generated_output_under_output_analytix_fund_analysis_is_local_evidence: true,
      eval_fixtures_are_eval_only: true,
      temporary_worktree_allowed: true,
      runtime_cache_sync_path: false,
      hub_publish_path: false,
    },
  };
}

function createReleaseSlicePatch(audit) {
  const untracked = new Set(
    audit.related_dirty
      .filter((entry) => entry.status.trim() === "??")
      .map((entry) => entry.path),
  );
  const trackedDirty = audit.related_dirty
    .map((entry) => entry.path)
    .filter((relativePath) => !untracked.has(relativePath));
  let patch = "";
  if (trackedDirty.length) {
    patch += String(
      runCommand("git", ["diff", "--binary", "--", ...trackedDirty]).stdout ||
        "",
    );
  }
  for (const relativePath of untracked) {
    const result = runCommand(
      "git",
      ["diff", "--no-index", "--binary", "--", "/dev/null", relativePath],
      {
        allowStatus: [0, 1],
      },
    );
    patch += String(result.stdout || "");
  }
  return patch;
}

function runNodeCheck(cwd, relativePath) {
  runCommand(process.execPath, ["--check", relativePath], { cwd });
  return { relativePath, ok: true };
}

function verifyReleaseSliceIsolation(audit) {
  const tempRoot = fs.mkdtempSync(
    path.join(os.tmpdir(), "analytix-release-slice-"),
  );
  const worktreePath = path.join(tempRoot, "worktree");
  const patchPath = path.join(tempRoot, "release-slice.patch");
  const cleanup = {
    worktree_removed: false,
    temp_root_removed: false,
  };
  const startedAt = new Date().toISOString();
  try {
    const patch = createReleaseSlicePatch(audit);
    fs.writeFileSync(patchPath, patch);
    runCommand("git", ["worktree", "add", "--detach", worktreePath, "HEAD"]);
    if (patch.trim()) {
      runCommand("git", ["apply", "--check", patchPath], { cwd: worktreePath });
      runCommand("git", ["apply", patchPath], { cwd: worktreePath });
    }
    const sourceNodeModules = path.join(REPO_ROOT, "node_modules");
    const sourceNodeModulesStat = fs.lstatSync(sourceNodeModules);
    if (!sourceNodeModulesStat.isDirectory() || sourceNodeModulesStat.isSymbolicLink()) {
      throw new Error("release isolation requires a regular installed node_modules dependency root");
    }
    fs.symlinkSync(
      sourceNodeModules,
      path.join(worktreePath, "node_modules"),
      process.platform === "win32" ? "junction" : "dir",
    );
    const isolatedAudit = JSON.parse(
      String(
        runCommand(
          process.execPath,
          [RELEASE_SLICE_SCRIPT, "--json", "--fail-on-unrelated"],
          {
            cwd: worktreePath,
          },
        ).stdout || "{}",
      ),
    );
    const nodeChecks = ISOLATION_NODE_CHECK_FILES.map((relativePath) =>
      runNodeCheck(worktreePath, relativePath),
    );
    const productionMcpEntryClosure = JSON.parse(
      String(
        runCommand(
          process.execPath,
          ["plugins/analytix-fund-analysis/scripts/production-mcp-entry-closure-contract.mjs"],
          { cwd: worktreePath },
        ).stdout || "{}",
      ),
    );
    const hubPackage = JSON.parse(
      String(
        runCommand(
          process.execPath,
          [
            "plugins/analytix-fund-analysis/scripts/prepare-hub-package.mjs",
            "--json",
            "--skip-archive",
            "--force",
            "--out-root",
            path.join(tempRoot, "hub-package"),
          ],
          { cwd: worktreePath },
        ).stdout || "{}",
      ),
    );
    const doctor = JSON.parse(
      String(
        runCommand(
          process.execPath,
          [
            "plugins/analytix-fund-analysis/scripts/doctor.mjs",
            "--json",
            "--skip-git",
            "--skip-runtime",
          ],
          {
            cwd: worktreePath,
          },
        ).stdout || "{}",
      ),
    );
    return {
      ok: true,
      started_at: startedAt,
      completed_at: new Date().toISOString(),
      source_head: audit.head,
      tags_at_head: audit.tags_at_head,
      patch_bytes: Buffer.byteLength(patch),
      current_unrelated_dirty_count: audit.unrelated_dirty_count,
      isolated_dirty_total: isolatedAudit.dirty_total,
      isolated_related_dirty_count: isolatedAudit.related_dirty_count,
      isolated_unrelated_dirty_count: isolatedAudit.unrelated_dirty_count,
      isolated_missing_release_slice_files:
        isolatedAudit.missing_release_slice_files,
      node_check_files: nodeChecks,
      production_mcp_entry_closure_ok:
        productionMcpEntryClosure.status === "ok",
      production_mcp_entry_closure_files:
        productionMcpEntryClosure.files || [],
      hub_package_production_mcp_payload_ok:
        hubPackage.production_mcp_payload?.ok === true,
      hub_package_production_mcp_payload_files:
        hubPackage.production_mcp_payload?.files || [],
      doctor_skip_git_ok: doctor.ok === true,
      doctor_skip_git_runtime_ok: doctor.ok === true,
      doctor_result_count: Array.isArray(doctor.results)
        ? doctor.results.length
        : 0,
      boundaries: {
        workspace_read_only: true,
        temporary_detached_git_worktree: true,
        runtime_cache_sync_path: false,
        hub_publish_path: false,
      },
      cleanup,
    };
  } finally {
    const removeWorktree = spawnSync(
      "git",
      ["worktree", "remove", "--force", worktreePath],
      {
        cwd: REPO_ROOT,
        encoding: "utf8",
      },
    );
    cleanup.worktree_removed =
      removeWorktree.status === 0 || !fs.existsSync(worktreePath);
    fs.rmSync(tempRoot, { recursive: true, force: true });
    cleanup.temp_root_removed = !fs.existsSync(tempRoot);
  }
}

function buildStagingPlan(audit) {
  const stagePaths = audit.related_dirty.map((entry) => entry.path);
  const leaveUnstagedPaths = audit.unrelated_dirty.map((entry) => entry.path);
  const isolation = audit.isolation_check || null;
  const isolationReady =
    !isolation ||
    (isolation.ok &&
      isolation.isolated_unrelated_dirty_count === 0 &&
      !audit.missing_release_slice_files.length);
  return {
    read_only: true,
    mutates_index: false,
    release_slice_only: true,
    ready_for_authorized_stage: Boolean(stagePaths.length && isolationReady),
    stage_path_count: stagePaths.length,
    stage_paths: stagePaths,
    leave_unstaged_count: leaveUnstagedPaths.length,
    leave_unstaged_paths: leaveUnstagedPaths,
    leave_unstaged_goal_wide_count: audit.goal_wide_unrelated_dirty_count,
    goal_wide_unrelated_paths_to_keep_out_count:
      audit.goal_wide_unrelated_dirty_count,
    leave_unstaged_goal_wide_groups: audit.goal_wide_unrelated_dirty_groups,
    leave_unstaged_unknown_count: audit.unknown_unrelated_dirty_count,
    unknown_unrelated_paths_to_keep_out_count:
      audit.unknown_unrelated_dirty_count,
    leave_unstaged_unknown_groups: audit.unknown_unrelated_dirty_groups,
    pathspec_stdin: stagePaths.length ? `${stagePaths.join("\n")}\n` : "",
    suggested_commands_after_authorization: [
      {
        purpose: "stage only release-slice paths",
        command: ["git", "add", "--pathspec-from-file=-"],
        stdin: "pathspec_stdin",
      },
      {
        purpose:
          "verify staged paths contain no unrelated flow/network changes",
        command: ["git", "diff", "--cached", "--name-only"],
      },
      {
        purpose: "rerun release identity gates after commit/tag authorization",
        command: [
          "node",
          "plugins/analytix-fund-analysis/scripts/phase4-diagnostics.mjs",
          "--release-guard",
          "--json",
        ],
      },
    ],
    authorization_required_before: [
      "git add",
      "git commit",
      "git tag",
      "git push",
      "Hub publish",
      "marketplace update",
    ],
    authorization_model: {
      current_thread_explicit_authorization_required: true,
      goal_15_6_full_authorization_satisfies: true,
      autonomous_release_allowed_after_authorization_and_gates: true,
      no_auth_bypass: true,
    },
    boundaries: {
      workspace_read_only: true,
      release_slice_only: true,
      release_guard_is_read_only: true,
      release_actions_are_gated: true,
    },
  };
}

function buildReadinessPlan(audit, options) {
  const manifest = readJson(
    "plugins/analytix-fund-analysis/.codex-plugin/plugin.json",
  );
  const registry = readJson(
    "plugins/analytix-fund-analysis/references/capability-registry.json",
  );
  const manifestVersion = String(manifest.version || "");
  const mcpServerVersion = serverVersion();
  const currentTag = manifestVersion
    ? `analytix-fund-analysis-v${manifestVersion}`
    : "";
  const currentTagAtHead = currentTag
    ? audit.tags_at_head.includes(currentTag)
    : false;
  const currentTagExists = currentTag ? tagExists(currentTag) : false;
  const currentTagStatus = releaseTagStatus(
    currentTag,
    currentTagExists,
    currentTagAtHead,
  );
  const currentTagStale = Boolean(currentTagExists && !currentTagAtHead);
  const releaseNotes = readText(
    "plugins/analytix-fund-analysis/RELEASE_NOTES.md",
  );
  const releaseNotesCurrent = manifestVersion
    ? releaseNotes.includes(`## ${manifestVersion}`)
    : false;
  const dirtyAfterCurrentTag = Boolean(
    audit.related_dirty_count || audit.unrelated_dirty_count,
  );
  const shouldSuggestPatchVersion = Boolean(
    manifestVersion &&
    ((currentTagAtHead && dirtyAfterCurrentTag) || currentTagStale),
  );
  const suggestedNextVersion = shouldSuggestPatchVersion
    ? nextAvailablePatchVersion(manifestVersion)
    : manifestVersion;
  const suggestedNextTag = suggestedNextVersion
    ? `analytix-fund-analysis-v${suggestedNextVersion}`
    : "";
  const candidateVersion = String(
    options.candidateVersion || suggestedNextVersion || "",
  ).trim();
  const candidateTag = candidateVersion
    ? `analytix-fund-analysis-v${candidateVersion}`
    : "";
  const candidateTagExists = tagExists(candidateTag);
  const versionFilesForNextRelease = RELEASE_VERSION_FILES;
  const versionReplacementsForCandidate = [
    {
      file: "plugins/analytix-fund-analysis/.codex-plugin/plugin.json",
      from: `"version": "${manifestVersion}"`,
      to: `"version": "${candidateVersion}"`,
    },
    {
      file: "plugins/analytix-fund-analysis/mcp/server.mjs",
      from: `SERVER_VERSION = "${mcpServerVersion}"`,
      to: `SERVER_VERSION = "${candidateVersion}"`,
    },
    {
      file: "plugins/analytix-fund-analysis/mcp/mcp-request-handler-runtime.mjs",
      from: `MCP_REQUEST_HANDLER_RUNTIME_VERSION = "${manifestVersion}"`,
      to: `MCP_REQUEST_HANDLER_RUNTIME_VERSION = "${candidateVersion}"`,
    },
    {
      file: "plugins/analytix-fund-analysis/README.md",
      from: `${manifestVersion} 候选发布的前提`,
      to: `${candidateVersion} 候选发布的前提`,
    },
    {
      file: "plugins/analytix-fund-analysis/RELEASE_NOTES.md",
      from: "# Analytix Fund Analysis Release Notes",
      to: `prepend ## ${candidateVersion} after the title and preserve every historical section including ## ${manifestVersion}`,
    },
    {
      file: "plugins/analytix-fund-analysis/references/capability-registry.json",
      from: `"version": "${manifestVersion}"`,
      to: `"version": "${candidateVersion}"`,
    },
    {
      file: "plugins/analytix-fund-analysis/references/capability-registry.schema.json",
      from: "evalCoverageContract",
      to: "evalCoverageContract",
    },
    {
      file: "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/capability-registry.json",
      from: `"version": "${manifestVersion}"`,
      to: `"version": "${candidateVersion}"`,
    },
    {
      file: "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/capability-registry.schema.json",
      from: "evalCoverageContract",
      to: "evalCoverageContract",
    },
    {
      file: "plugins/analytix-fund-analysis/scripts/eval-coverage-contract.mjs",
      from: `EVAL_COVERAGE_CONTRACT_VERSION = "${manifestVersion}"`,
      to: `EVAL_COVERAGE_CONTRACT_VERSION = "${candidateVersion}"`,
    },
    {
      file: "plugins/analytix-fund-analysis/scripts/runtime-cache-contract.mjs",
      from: `RUNTIME_CACHE_CONTRACT_VERSION = "${manifestVersion}-runtime-cache-contract"`,
      to: `RUNTIME_CACHE_CONTRACT_VERSION = "${candidateVersion}-runtime-cache-contract"`,
    },
    {
      file: "plugins/analytix-fund-analysis/scripts/source-live-probe-contract.mjs",
      from: `SOURCE_LIVE_PROBE_CONTRACT_VERSION = "${manifestVersion}"`,
      to: `SOURCE_LIVE_PROBE_CONTRACT_VERSION = "${candidateVersion}"`,
    },
    {
      file: "plugins/analytix-fund-analysis/scripts/source-live-probe-go-wire-contract.mjs",
      from: `SOURCE_LIVE_PROBE_GO_WIRE_CONTRACT_VERSION = "${manifestVersion}"`,
      to: `SOURCE_LIVE_PROBE_GO_WIRE_CONTRACT_VERSION = "${candidateVersion}"`,
    },
    {
      file: "packages/runtime-go/internal/mcp/manager.go",
      from: `fundsEvidenceKernelVersion = "${manifestVersion}"`,
      to: `fundsEvidenceKernelVersion = "${candidateVersion}"`,
    },
  ];
  const candidateVersionFileGaps = candidateVersion
    ? versionReplacementsForCandidate
        .filter((item) => !readText(item.file).includes(item.from))
        .map((item) => `${item.file} missing ${item.from}`)
    : [];
  const runtimeVersionGaps = pluginRuntimeVersionGaps(manifestVersion);
  const blockers = [
    manifestVersion !== mcpServerVersion
      ? `manifest/server version mismatch: ${manifestVersion} vs ${mcpServerVersion}`
      : "",
    ...runtimeVersionGaps.map(
      (gap) => `plugin runtime version mismatch: ${gap}`,
    ),
    !releaseNotesCurrent
      ? `RELEASE_NOTES.md missing ## ${manifestVersion}`
      : "",
    currentTagStatus.blocker,
    audit.unrelated_dirty_count
      ? `unrelated dirty paths must stay out of release commit: ${audit.unrelated_dirty_count}`
      : "",
    audit.related_dirty_count ? "release-slice changes are uncommitted" : "",
  ].filter(Boolean);
  const candidateBlockers = [
    !candidateVersion ? "candidate version is missing" : "",
    candidateVersion && !validSemverPatchVersion(candidateVersion)
      ? `candidate version is not semver-like: ${candidateVersion}`
      : "",
    shouldSuggestPatchVersion && candidateVersion === manifestVersion
      ? `candidate version must differ from current publish identity ${manifestVersion}`
      : "",
    candidateTagExists ? `${candidateTag} already exists` : "",
    ...candidateVersionFileGaps,
  ].filter(Boolean);
  const evalCoverageFixture = readJson(
    "plugins/analytix-fund-analysis/scripts/eval-fixtures/eval-coverage-contract.json",
  );
  const evalCoverageContract = objectOf(
    evalCoverageFixture.eval_coverage_contract,
  );
  const requiredCaseIds = Array.isArray(evalCoverageContract.required_case_ids)
    ? evalCoverageContract.required_case_ids.map(text).filter(Boolean)
    : [];
  const primaryHealthCaseId = requiredCaseIds.includes("3c72e755b1f2")
    ? "3c72e755b1f2"
    : text(requiredCaseIds[0]);
  const nativeAnalysisComputeEvidence = buildNativeAnalysisComputeEvidence();
  const mcpSurfaceSnapshotEvidence = buildMcpSurfaceSnapshotEvidence(
    manifestVersion,
    options.mcpSurfaceEvidencePath,
  );
  const nativeAnalysisComputeGate = {
    required_for_case_bound_health: true,
    required_command_surface: NATIVE_ANALYSIS_COMPUTE_REQUIRED_COMMANDS,
    failure_signal: "Rust analysis compute stats-query-worker unavailable",
    local_env_override: "ANALYTIX_ANALYSIS_COMPUTE_BIN",
    local_env_override_is_verification_only: true,
    local_temp_binary_is_release_evidence: false,
    packaged_runtime_evidence_required: true,
    packaged_runtime_evidence_ready:
      nativeAnalysisComputeEvidence.packaged_runtime_evidence_ready,
    packaged_runtime_evidence: nativeAnalysisComputeEvidence,
    packaging_evidence: [
      "scripts/build/build_runtime_binaries.js --target analysis-compute",
      "scripts/build/check_standard_windows_runtime_assets.js verifies analytix-analysis-compute.exe and stats-query-worker",
      "macOS/Windows packaged runtime must include analytix-analysis-compute with the required command surface",
    ],
  };
  const caseBoundHealthGate = {
    generic_check_health_is_smoke_only: true,
    mcp_surface_snapshot_required_before_publish: true,
    mcp_surface_snapshot_evidence: mcpSurfaceSnapshotEvidence,
    case_bound_health_required_before_publish: true,
    native_analysis_compute_gate: nativeAnalysisComputeGate,
    required_case_ids: requiredCaseIds,
    primary_case_id: primaryHealthCaseId,
    suggested_commands: [
      [
        "node",
        "plugins/analytix-fund-analysis/scripts/mcp-surface-snapshot.mjs",
        "--json",
        "--fail-on-violation",
      ].join(" "),
      [
        "node",
        "plugins/analytix-fund-analysis/scripts/check-health.mjs",
        "--json",
      ].join(" "),
      primaryHealthCaseId
        ? [
            "node",
            "plugins/analytix-fund-analysis/scripts/check-health.mjs",
            "--case-id",
            primaryHealthCaseId,
            "--deep",
            "--timeout-ms",
            "180000",
            "--json",
          ].join(" ")
        : "",
      primaryHealthCaseId
        ? [
            "node",
            "plugins/analytix-fund-analysis/scripts/phase4-diagnostics.mjs",
            "--run",
            "B0",
            "--backend-url",
            "http://127.0.0.1:18731",
            "--json",
          ].join(" ")
        : "",
    ].filter(Boolean),
    publish_boundary:
      "a no-active-case generic health warning is not sufficient release evidence; at least one registry-required case must pass live case-bound health",
  };
  const releaseBlockers = [
    ...blockers,
    mcpSurfaceSnapshotEvidence.contract_ready
      ? ""
      : `MCP surface snapshot evidence missing, stale, or invalid: ${mcpSurfaceSnapshotEvidence.contract_blockers.join("; ")}`,
    mcpSurfaceSnapshotEvidence.publication_ready
      ? ""
      : `MCP publication readiness blocked: ${mcpSurfaceSnapshotEvidence.publication_blockers.join("; ")}`,
    nativeAnalysisComputeEvidence.packaged_runtime_evidence_ready
      ? ""
      : `packaged runtime evidence missing: ${nativeAnalysisComputeEvidence.blockers.join("; ")}`,
  ].filter(Boolean);
  return {
    read_only: true,
    mutates_files: false,
    mutates_index: false,
    current_version: manifestVersion,
    server_version: mcpServerVersion,
    current_tag: currentTag,
    current_tag_exists: currentTagExists,
    current_tag_at_head: currentTagAtHead,
    current_tag_state: currentTagStatus.state,
    current_tag_stale: currentTagStale,
    release_notes_current: releaseNotesCurrent,
    dirty_after_current_tag: dirtyAfterCurrentTag,
    should_suggest_patch_version: shouldSuggestPatchVersion,
    can_publish_current_tree: releaseBlockers.length === 0,
    suggested_next_version: suggestedNextVersion,
    suggested_next_tag: suggestedNextTag,
    candidate_version: candidateVersion,
    candidate_tag: candidateTag,
    candidate_tag_exists: candidateTagExists,
    candidate_version_ready: candidateBlockers.length === 0,
    candidate_blockers: candidateBlockers,
    candidate_version_file_gaps: candidateVersionFileGaps,
    version_replacements_for_candidate: versionReplacementsForCandidate,
    version_files_for_next_release: versionFilesForNextRelease,
    case_bound_health_gate: caseBoundHealthGate,
    release_slice_stage_path_count: audit.related_dirty_count,
    unrelated_paths_to_keep_out_count: audit.unrelated_dirty_count,
    goal_wide_unrelated_paths_to_keep_out_count:
      audit.goal_wide_unrelated_dirty_count,
    unknown_unrelated_paths_to_keep_out_count:
      audit.unknown_unrelated_dirty_count,
    blockers: releaseBlockers,
    required_sequence_after_authorization: [
      "update version files if creating a new release version",
      "rerun release-slice-audit --verify-isolated --staging-plan --readiness-plan",
      "stage only release-slice and approved version files",
      "verify git diff --cached --name-only contains no unrelated flow/network paths",
      "run node --check, Python compile, doctor, MCP surface snapshot under output/analytix-fund-analysis/evidence, generic check-health, packaged-native case-bound check-health, B0, and release guard",
      "run functional-eval plus real frontdoor functional closure for pair amount, object dossier, full-case analysis, continue-one-hop, workbench, visual/report delivery, claim review, and passive nonfunds tasks; attach --functional-closure-evidence and --mcp-surface-evidence",
      "prove packaged analytix-analysis-compute stats-query-worker is available; a local /tmp ANALYTIX_ANALYSIS_COMPUTE_BIN override is verification-only, not release evidence",
      "create release commit only after the gates pass",
      `create ${candidateTag || suggestedNextTag || "matching release tag"} only after the release commit`,
      "publish through Analytix Hub only after release guard has 0 fail",
    ],
    authorization_required_before: [
      "version file edits",
      "git add",
      "git commit",
      "git tag",
      "git push",
      "Hub publish",
      "marketplace update",
    ],
    authorization_model: {
      current_thread_explicit_authorization_required: true,
      goal_15_6_full_authorization_satisfies: true,
      autonomous_release_allowed_after_authorization_and_gates: true,
      no_auth_bypass: true,
    },
    boundaries: {
      release_guard_is_read_only: true,
      release_actions_are_gated: true,
      version_identity_must_match: true,
      hub_publish_requires_authenticated_admin_console: true,
    },
  };
}

function printHuman(audit) {
  console.log(`Release slice audit: ${audit.audit}`);
  console.log(
    `HEAD: ${audit.head}${audit.tags_at_head.length ? ` (${audit.tags_at_head.join(", ")})` : ""}`,
  );
  console.log(`Dirty paths: ${audit.dirty_total}`);
  if (audit.generated_evidence_dirty_count) {
    console.log(
      `Generated evidence paths ignored: ${audit.generated_evidence_dirty_count}`,
    );
  }
  console.log(
    `Related dirty: ${audit.related_dirty_count}/${audit.release_slice_count}`,
  );
  console.log(`Unrelated dirty: ${audit.unrelated_dirty_count}`);
  if (audit.publish_blockers.length) {
    console.log(`Publish blockers: ${audit.publish_blockers.join("; ")}`);
  } else {
    console.log("Publish blockers: none");
  }
  if (audit.related_dirty.length) {
    console.log("\nRelated dirty paths:");
    for (const entry of audit.related_dirty)
      console.log(`  ${entry.status} ${entry.path}`);
  }
  if (audit.unrelated_dirty.length) {
    console.log("\nUnrelated dirty paths:");
    for (const entry of audit.unrelated_dirty)
      console.log(`  ${entry.status} ${entry.path}`);
    if (audit.unrelated_dirty_groups?.length) {
      console.log("\nUnrelated dirty groups:");
      for (const group of audit.unrelated_dirty_groups) {
        console.log(`  ${group.group}: ${group.count}`);
      }
    }
    if (audit.goal_wide_unrelated_dirty_groups?.length) {
      console.log(
        `\nGoal-wide runtime closure dirty groups: ${audit.goal_wide_unrelated_dirty_count}`,
      );
      for (const group of audit.goal_wide_unrelated_dirty_groups) {
        console.log(`  ${group.group}: ${group.count}`);
      }
    }
    if (audit.unknown_unrelated_dirty_groups?.length) {
      console.log(
        `\nUnknown unrelated dirty groups: ${audit.unknown_unrelated_dirty_count}`,
      );
      for (const group of audit.unknown_unrelated_dirty_groups) {
        console.log(`  ${group.group}: ${group.count}`);
      }
    }
  }
  if (audit.missing_release_slice_files.length) {
    console.log("\nMissing release-slice files:");
    for (const relativePath of audit.missing_release_slice_files)
      console.log(`  ${relativePath}`);
  }
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.selfTestGeneratedEvidenceIgnore) {
    const result = selfTestGeneratedEvidenceIgnore();
    if (options.json) {
      console.log(JSON.stringify(result, null, 2));
    } else {
      console.log(
        `Generated evidence ignore self-test: ${result.ok ? "pass" : "fail"}`,
      );
      for (const testCase of result.results) {
        console.log(`  ${testCase.ok ? "ok" : "fail"} ${testCase.name}`);
      }
    }
    if (!result.ok) process.exitCode = 1;
    return;
  }
  if (options.selfTestMcpSurfaceEvidence) {
    const result = selfTestMcpSurfaceEvidence();
    if (options.json) {
      console.log(JSON.stringify(result, null, 2));
    } else {
      console.log(
        `MCP surface evidence self-test: ${result.ok ? "pass" : "fail"}`,
      );
      for (const testCase of result.results) {
        console.log(`  ${testCase.ok ? "ok" : "fail"} ${testCase.name}`);
      }
    }
    if (!result.ok) process.exitCode = 1;
    return;
  }
  if (options.selfTestNativeRuntimeEvidence) {
    const result = selfTestNativeRuntimeEvidence();
    if (options.json) {
      console.log(JSON.stringify(result, null, 2));
    } else {
      console.log(
        `Native runtime evidence self-test: ${result.ok ? "pass" : "fail"}`,
      );
    }
    if (!result.ok) process.exitCode = 1;
    return;
  }
  if (options.selfTestUnrelatedDirtyGroups) {
    const result = selfTestUnrelatedDirtyGroups();
    if (options.json) {
      console.log(JSON.stringify(result, null, 2));
    } else {
      console.log(
        `Unrelated dirty groups self-test: ${result.ok ? "pass" : "fail"}`,
      );
      for (const testCase of result.results) {
        console.log(`  ${testCase.ok ? "ok" : "fail"} ${testCase.name}`);
      }
    }
    if (!result.ok) process.exitCode = 1;
    return;
  }
  if (options.selfTestReleaseTagStatus) {
    const result = selfTestReleaseTagStatus();
    if (options.json) {
      console.log(JSON.stringify(result, null, 2));
    } else {
      console.log(
        `Release tag status self-test: ${result.ok ? "pass" : "fail"}`,
      );
      for (const testCase of result.results) {
        console.log(`  ${testCase.ok ? "ok" : "fail"} ${testCase.name}`);
      }
    }
    if (!result.ok) process.exitCode = 1;
    return;
  }
  if (options.selfTestSummaryJson) {
    const result = selfTestSummaryJson();
    if (options.json || options.summaryJson) {
      console.log(JSON.stringify(result, null, 2));
    } else {
      console.log(
        `Release slice summary JSON self-test: ${result.ok ? "pass" : "fail"}`,
      );
    }
    if (!result.ok) process.exitCode = 1;
    return;
  }
  const audit = buildAudit();
  if (
    options.failOnPublishBlockers &&
    !options.functionalClosureEvidencePath
  ) {
    audit.publish_blockers.push(
      "functional closure evidence is required by --fail-on-publish-blockers",
    );
  }
  if (options.verifyIsolated) {
    audit.isolation_check = verifyReleaseSliceIsolation(audit);
    if (!audit.isolation_check.ok) {
      audit.publish_blockers.push("release slice isolation check failed");
    }
  }
  if (options.stagingPlan) {
    audit.staging_plan = buildStagingPlan(audit);
  }
  if (options.readinessPlan) {
    audit.readiness_plan = buildReadinessPlan(audit, options);
    for (const blocker of arrayOf(audit.readiness_plan.blockers)
      .map(text)
      .filter(Boolean)) {
      if (
        blocker === "release-slice changes are uncommitted" &&
        audit.publish_blockers.some((existing) =>
          existing.startsWith("release-slice changes are uncommitted"),
        )
      ) {
        continue;
      }
      if (!audit.publish_blockers.includes(blocker))
        audit.publish_blockers.push(blocker);
    }
  }
  if (options.functionalClosureEvidencePath) {
    audit.functional_closure_evidence = buildFunctionalClosureEvidenceCheck(
      audit,
      options.functionalClosureEvidencePath,
    );
    if (!audit.functional_closure_evidence.ready) {
      audit.publish_blockers.push(
        audit.functional_closure_evidence.local_non_backend_ready
          ? "functional closure still needs release-only live backend evidence"
          : "functional closure evidence check failed",
      );
    }
  }
  if (options.summaryJson) {
    console.log(JSON.stringify(buildReleaseSliceSummary(audit), null, 2));
  } else if (options.json) {
    console.log(JSON.stringify(audit, null, 2));
  } else {
    printHuman(audit);
    if (audit.staging_plan) {
      console.log(
        `\nStaging plan: ${audit.staging_plan.ready_for_authorized_stage ? "ready after authorization" : "not ready"}`,
      );
      console.log(`  stage paths: ${audit.staging_plan.stage_path_count}`);
      console.log(
        `  leave unstaged paths: ${audit.staging_plan.leave_unstaged_count}`,
      );
      console.log("  no git index changes were made");
    }
    if (audit.readiness_plan) {
      console.log(
        `\nReadiness plan: ${audit.readiness_plan.can_publish_current_tree ? "publishable" : "not publishable"}`,
      );
      console.log(`  current version: ${audit.readiness_plan.current_version}`);
      console.log(
        `  candidate version: ${audit.readiness_plan.candidate_version}`,
      );
      console.log(`  blockers: ${audit.readiness_plan.blockers.length}`);
      console.log("  no files or index entries were changed");
    }
    if (audit.functional_closure_evidence) {
      console.log(
        `\nFunctional closure evidence: ${audit.functional_closure_evidence.ready ? "pass" : "fail"}`,
      );
      console.log(`  file: ${audit.functional_closure_evidence.path}`);
      console.log(`  tasks: ${audit.functional_closure_evidence.task_count}`);
      console.log(
        `  passed families: ${audit.functional_closure_evidence.passed_families.join(", ") || "none"}`,
      );
      console.log(
        `  blockers: ${audit.functional_closure_evidence.blockers.length}`,
      );
    }
    if (audit.isolation_check) {
      console.log(
        `\nIsolation check: ${audit.isolation_check.ok ? "pass" : "fail"}`,
      );
      console.log(
        `  isolated related dirty: ${audit.isolation_check.isolated_related_dirty_count}`,
      );
      console.log(
        `  isolated unrelated dirty: ${audit.isolation_check.isolated_unrelated_dirty_count}`,
      );
      console.log(
        `  node --check files: ${audit.isolation_check.node_check_files.length}`,
      );
      console.log(
        `  doctor --skip-git --skip-runtime: ${audit.isolation_check.doctor_skip_git_runtime_ok ? "pass" : "fail"}`,
      );
      console.log(
        `  temporary worktree removed: ${audit.isolation_check.cleanup.worktree_removed ? "yes" : "no"}`,
      );
    }
  }
  if (
    (options.failOnUnrelated && audit.unrelated_dirty.length) ||
    (options.failOnMissing && audit.missing_release_slice_files.length) ||
    (options.failOnPublishBlockers && audit.publish_blockers.length)
  ) {
    process.exitCode = 1;
  }
}

try {
  main();
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
