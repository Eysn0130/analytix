#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  ALL_SKILL_NAMES,
  FOCUSED_SKILL_NAMES,
  MARKETPLACE_NAME,
  PLUGIN_CACHE_FILES,
  PLUGIN_NAME,
  REFERENCE_FILES,
  RUNTIME_SKILL_FILES,
  runtimeCacheContractViolations,
} from "./runtime-cache-contract.mjs";
import {
  classifyRuntimeInstallAuthority,
  validateInstalledMarkerIdentity,
  validatePointerDigest,
} from "./runtime-install-identity-contract.mjs";
import {
  inspectProductionMcpEntryClosure,
  PRODUCTION_MCP_ENTRY_CLOSURE_FILES,
} from "./production-mcp-entry-closure-contract.mjs";
import {
  commandBudgetFromCapabilityRegistry,
  formatEvalCoverageSummary,
  validateCapabilityRegistryDerivedMetadataContract,
  validateCommandMetadataBudgetContract,
  validateEvalCoverageContract,
  validateEvalCoverageRegistrySchema,
  withEvalCoverageContract,
} from "./eval-coverage-contract.mjs";
import {
  buildEvidenceLedger,
  validateEvidenceLedgerCoverage,
} from "../mcp/evidence-ledger.mjs";
import { readResourceForAgent } from "../mcp/progressive-resources.mjs";
import {
  FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
  FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI,
  fixedFundsBoundaryPrompts,
  fixedFundsBoundaryResources,
} from "../mcp/source-unavailable-boundary.mjs";
import { withAnswerCardProtocol } from "../mcp/answer-card-protocol.mjs";
import { withClaimReviewProtocol } from "../mcp/claim-verifier-protocol.mjs";
import { validateInvestigationAnswerContract } from "../mcp/investigation-answer-contract.mjs";
import { validateVisualArtifactContract } from "../mcp/visual-artifact-contract.mjs";
import {
  buildCasegraphComponentEvidence as buildCasegraphComponentEvidencePack,
  CASEGRAPH_REQUIRED_NODE_EDGE_NAMES,
  withFundGraphProtocol,
} from "../mcp/casegraph-protocol.mjs";
import { buildCasegraphRankEvidence } from "../mcp/casegraph-runtime.mjs";
import { DEFAULT_VISIBLE_TOOL_NAMES } from "../mcp/capability-registry-runtime.mjs";
import { tools as MCP_TOOL_SCHEMAS } from "../mcp/tool-schemas.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");
const repoRoot = path.resolve(pluginRoot, "..", "..");

const REQUIRED_TASK_TAXONOMY_IDS = [
  "quick_fact",
  "pair_amount",
  "ranking",
  "object_dossier",
  "counterparty",
  "trace_continuation",
  "investigation_lab",
  "full_case_analysis",
  "report",
  "claim_qa",
  "delivery_qc",
  "passive_nonintervention",
];

const REQUIRED_FILES = [
  ".codex-plugin/plugin.json",
  ".mcp.json",
  "agents/openai.yaml",
  "mcp/server.mjs",
  "skills/analytix-fund-analysis/SKILL.md",
  "mcp/agent-context-hygiene.mjs",
  "mcp/agent-output-compiler.mjs",
  "mcp/agent-payload-compiler.mjs",
  "mcp/answer-card-protocol.mjs",
  "mcp/backend-api-client.mjs",
  "mcp/card-renderer.mjs",
  "mcp/capability-registry-runtime.mjs",
  "mcp/case-pipeline-runtime.mjs",
  "mcp/case-scope-map-runtime.mjs",
  "mcp/casegraph-protocol.mjs",
  "mcp/casegraph-runtime.mjs",
  "mcp/cleaned-export-runtime.mjs",
  "mcp/claim-review-diagnostic-runtime.mjs",
  "mcp/claim-verifier-protocol.mjs",
  "mcp/context-compiler.mjs",
  "mcp/diagnostic-fact-helpers.mjs",
  "mcp/destination-diagnostic-runtime.mjs",
  "mcp/duckdb-diagnostic.mjs",
  "mcp/duckdb-workbench-runtime.mjs",
  "mcp/evidence-ledger.mjs",
  "mcp/frontdoor-answer-contract.mjs",
  "mcp/frontdoor-fact-summaries.mjs",
  "mcp/frontdoor-runtime.mjs",
  "mcp/frontdoor-routing.mjs",
  "mcp/fund-flow-graph-runtime.mjs",
  "mcp/fundgraph-builder.mjs",
  "mcp/intent-plan-protocol.mjs",
  "mcp/investigation-lab-diagnostic-runtime.mjs",
  "mcp/investigation-lab-protocol.mjs",
  "mcp/investigation-answer-contract.mjs",
  "mcp/jsonrpc-stdio-runtime.mjs",
  "mcp/artifact-path-policy.mjs",
  "mcp/mcp-request-handler-runtime.mjs",
  "mcp/source-unavailable-boundary.mjs",
  "mcp/mcp-output-policy.mjs",
  "mcp/mcp-tool-result-runtime.mjs",
  "mcp/progressive-prompts.mjs",
  "mcp/progressive-resources.mjs",
  "mcp/runtime-normalizers.mjs",
  "mcp/stable-hash.mjs",
  "mcp/safe-skill-runtime.mjs",
  "mcp/stats-query-runtime.mjs",
  "mcp/tool-call-runtime.mjs",
  "mcp/top-rankings-diagnostic-runtime.mjs",
  "mcp/tool-discovery-policy.mjs",
  "mcp/tool-input-schemas.mjs",
  "mcp/tool-schemas.mjs",
  "mcp/tool-runtime-routing.mjs",
  "mcp/user-facing-language.mjs",
  "mcp/visual-artifact-contract.mjs",
  "skills/analytix-fund-analysis/references/runtime-boundary.md",
  "skills/analytix-fund-analysis/references/analytix-workflow-context.md",
  "skills/analytix-fund-analysis/references/tool-availability.md",
  "skills/analytix-fund-analysis/references/command-router.md",
  "skills/analytix-fund-analysis/references/command-metadata.json",
  "skills/analytix-fund-analysis/references/anti-patterns.md",
  "skills/analytix-fund-analysis/references/capability-registry.json",
  "skills/analytix-fund-analysis/references/capability-registry.schema.json",
  "skills/analytix-fund-analysis/references/casegraph-roadmap.md",
  "skills/analytix-fund-analysis/references/hub-lifecycle.md",
  "skills/analytix-fund-analysis/references/investigation-answer-contract.md",
  "skills/analytix-fund-analysis/references/investigation-answer.schema.json",
  "skills/analytix-fund-analysis/references/mature-plugin-drift-table.md",
  "skills/analytix-fund-analysis/references/plugin-benchmark.md",
  "skills/analytix-fund-analysis/references/top-pluginization-plan.md",
  "skills/analytix-fund-analysis/references/blueprint-implementation-matrix.md",
  "references/runtime-boundary.md",
  "references/analytix-workflow-context.md",
  "references/tool-availability.md",
  "references/command-router.md",
  "references/command-metadata.json",
  "references/anti-patterns.md",
  "references/capability-registry.json",
  "references/capability-registry.schema.json",
  "references/casegraph-roadmap.md",
  "references/hub-lifecycle.md",
  "references/investigation-answer-contract.md",
  "references/investigation-answer.schema.json",
  "references/mature-plugin-drift-table.md",
  "references/plugin-benchmark.md",
  "references/top-pluginization-plan.md",
  "references/blueprint-implementation-matrix.md",
  "scripts/model-ab-eval.mjs",
  "scripts/agent-thread-audit.mjs",
  "scripts/agent-ui-ab-run.mjs",
  "scripts/desktop-multiturn-acceptance.mjs",
  "scripts/eval-coverage-contract.mjs",
  "scripts/functional-eval.mjs",
  "scripts/check-health.mjs",
  "scripts/release-slice-audit.mjs",
  "scripts/prepare-hub-package.mjs",
  "scripts/phase4-diagnostics.mjs",
  "scripts/frontdoor-smoke.mjs",
  "scripts/investigation-answer-contract-smoke.mjs",
  "scripts/investigation-scenario-audit.mjs",
  "scripts/mcp-return-oracle.mjs",
  "scripts/mcp-surface-snapshot.mjs",
  "scripts/pair-amount-contract-smoke.mjs",
  "scripts/p0-contract-suite.mjs",
  "scripts/funds-producer-content-v1-contract.mjs",
  "scripts/dataset-snapshot-manifest-v2-contract.mjs",
  "scripts/tool-schema-contract.mjs",
  "scripts/runtime-install-identity-contract.mjs",
  "scripts/runtime-cache-contract.mjs",
  "scripts/skill-creator-alignment-audit.mjs",
  "scripts/skill-clause-audit.mjs",
  "scripts/sync-runtime-cache.mjs",
  "RELEASE_NOTES.md",
  "README.md",
];

const REQUIRED_AGENT_ROUTING_MARKERS = [
  "entry_status",
  "object_dossier",
  "account-dossier",
  "subject-dossier",
  "full_case",
  "full-case-analysis",
  "report_delivery",
  "report-builder",
  "evidence-request",
  "analysis-critique",
  "claim-review",
  "graph_visualization",
  "graph-visualization",
  "passive_nonintervention",
  'route_to: "none"',
];

const CRITICAL_TOOLS = [
  "funds_investigate",
  "get_casegraph",
  "build_fund_flow_graph",
  "get_current_case",
  "get_case_scope_map",
  "inspect_case_schema",
  "audit_unindexed_sources",
  "audit_case_data_quality",
  "resolve_duplicate_families",
  "get_case_reconciliation",
  "get_scope_coverage",
  "compare_analysis_scopes",
  "rank_accounts",
  "rank_holders",
  "rank_counterparties",
  "resolve_account_scope",
  "resolve_owner_scope",
  "analyze_account_full",
  "analyze_holder_full",
  "hypothesis_probe",
  "plan_case_analysis",
  "run_investigation_lab",
  "trace_subject_top_outflows",
  "trace_fund_next_hop",
  "trace_fund",
  "classify_missing_counterparty_business",
  "validate_continuation_list",
  "get_evidence_pack",
  "run_full_case_analysis",
  "validate_report_claims",
  "export_cleaned_case_data",
  "run_case_sql",
  "create_case_notebook",
];

const DEFAULT_SEMANTIC_DISCOVERY_TOOLS = DEFAULT_VISIBLE_TOOL_NAMES;

const NAVIGATOR_PRIMARY_ALLOWED_COMMANDS = new Set(["/analytix ask"]);

const NAVIGATOR_PRIMARY_ALLOWED_CAPABILITIES = new Set(["frontdoor-qa"]);

const FORBIDDEN_DIFF_PREFIXES = [
  "analytixagent/CodexDesktop/codex/",
  "CodexDesktop/codex/",
  "codex/",
];

const PLUGIN_BOUNDARY_FORBIDDEN_STATUS_PREFIXES = [
  "apps/desktop/",
  "analytixagent/",
  "CodexDesktop/",
  "codex/",
];

const PLUGIN_MANIFEST_ALLOWED_KEYS = new Set([
  "id",
  "name",
  "version",
  "description",
  "skills",
  "apps",
  "mcpServers",
  "interface",
  "author",
  "homepage",
  "repository",
  "license",
  "keywords",
]);

const PLUGIN_INTERFACE_REQUIRED_FIELDS = [
  "displayName",
  "shortDescription",
  "longDescription",
  "developerName",
  "category",
];

const SEMVER_PATTERN =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/u;
const HEX_COLOR_PATTERN = /^#[0-9A-F]{6}$/iu;

const REQUIRED_RESOURCE_URIS = [
  "skill://analytix-fund-analysis/SKILL.md",
  "skill://analytix-fund-analysis/references/command-metadata.json",
  "skill://analytix-fund-analysis/references/command-router.md",
  "skill://analytix-fund-analysis/references/tool-availability.md",
  "skill://analytix-fund-analysis/references/runtime-boundary.md",
  "skill://analytix-fund-analysis/references/hub-lifecycle.md",
  "skill://analytix-fund-analysis/references/anti-patterns.md",
  "skill://analytix-fund-analysis/references/focused-skill-shared.md",
  "skill://analytix-fund-analysis/references/analytix-workflow-context.md",
  "skill://analytix-fund-analysis/references/economic-investigation-analysis.md",
  "skill://analytix-fund-analysis/references/public-security-official-writing.md",
  "skill://analytix-fund-analysis/references/investigation-answer-contract.md",
  "skill://analytix-fund-analysis/references/investigation-answer.schema.json",
  "skill://analytix-fund-analysis/references/capability-registry.json",
  "skill://analytix-fund-analysis/references/capability-registry.schema.json",
];

const DEVELOPMENT_ONLY_RESOURCE_URIS = [
  "skill://analytix-fund-analysis/references/plugin-benchmark.md",
  "skill://analytix-fund-analysis/references/mature-plugin-drift-table.md",
  "skill://analytix-fund-analysis/references/top-pluginization-plan.md",
  "skill://analytix-fund-analysis/references/blueprint-implementation-matrix.md",
  "skill://analytix-fund-analysis/references/casegraph-roadmap.md",
];

const REQUIRED_PROMPT_NAMES = [
  "explore-case-database",
  "diagnose-case-query",
  "build-fund-flow-graph",
  "build-visual-evidence-pack",
  "prepare-investigation-report",
];

const PUBLIC_COPY_FILES = [
  ".codex-plugin/plugin.json",
  "README.md",
  "skills/analytix-fund-analysis/SKILL.md",
  "references/command-router.md",
  "references/tool-availability.md",
  "references/anti-patterns.md",
  "references/command-metadata.json",
  "skills/analytix-fund-analysis/references/command-router.md",
  "skills/analytix-fund-analysis/references/tool-availability.md",
  "skills/analytix-fund-analysis/references/anti-patterns.md",
  "skills/analytix-fund-analysis/references/command-metadata.json",
  "mcp/card-renderer.mjs",
  "mcp/case-scope-map-runtime.mjs",
  "mcp/frontdoor-runtime.mjs",
  "mcp/investigation-lab-diagnostic-runtime.mjs",
  "mcp/investigation-lab-protocol.mjs",
  "mcp/tool-schemas.mjs",
];

const PUBLIC_COPY_FORBIDDEN_PATTERNS = [
  /旧线程/u,
  /像旧线程/u,
  /Old-thread/u,
  /old-thread(?:-style|-quality)?/iu,
];

const RESOURCE_EVAL_ONLY_KEYS = [
  "eval_coverage_contract",
  "eval_tasks",
  "required_task_ids",
  "passive_nonfunds_task_ids",
  "required_dimensions",
];

function text(value) {
  return String(value || "").trim();
}

function skillFrontmatterIssues(body) {
  const source = String(body || "");
  if (!source.startsWith("---\n") && !source.startsWith("---\r\n")) {
    return ["missing YAML frontmatter"];
  }
  const match = source.match(/^---\r?\n([\s\S]*?)\r?\n---/u);
  if (!match) {
    return ["unterminated YAML frontmatter"];
  }
  const issues = [];
  for (const line of match[1].split(/\r?\n/u)) {
    if (!line.trim() || /^\s*#/u.test(line)) continue;
    const pair = line.match(/^([A-Za-z0-9_-]+):\s+(.*)$/u);
    if (!pair) continue;
    const value = pair[2].trim();
    if (!/^["']/u.test(value) && /:\s/u.test(value)) {
      issues.push(`${pair[1]} contains unquoted colon-space`);
    }
  }
  return issues;
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value
    : {};
}

function nonEmptyString(value) {
  return typeof value === "string" && value.trim() ? value.trim() : "";
}

function hasTodoMarker(value) {
  if (typeof value === "string") return value.includes("[TODO:");
  if (Array.isArray(value)) return value.some(hasTodoMarker);
  if (value && typeof value === "object")
    return Object.values(value).some(hasTodoMarker);
  return false;
}

function isHttpsUrl(value) {
  if (!nonEmptyString(value)) return false;
  try {
    return new URL(value).protocol === "https:";
  } catch (_) {
    return false;
  }
}

function isSafeRelativePath(value) {
  const relativePath = nonEmptyString(value);
  if (!relativePath || path.isAbsolute(relativePath)) return false;
  const normalized = path.normalize(relativePath);
  return normalized && normalized !== "." && !normalized.startsWith("..");
}

function pluginRelativePathExists(relativePath) {
  return (
    isSafeRelativePath(relativePath) &&
    fs.existsSync(path.join(pluginRoot, relativePath))
  );
}

function validatePluginManifestIngestionContract(manifest, mcpManifest) {
  const errors = [];
  const source = objectOf(manifest);
  const interfaceConfig = objectOf(source.interface);
  const unknownKeys = Object.keys(source).filter(
    (key) => !PLUGIN_MANIFEST_ALLOWED_KEYS.has(key),
  );
  if (unknownKeys.length)
    errors.push(`unsupported plugin.json fields: ${unknownKeys.join(", ")}`);
  if (hasTodoMarker(source))
    errors.push("plugin.json contains [TODO:] placeholder");
  if (!nonEmptyString(source.name)) errors.push("missing name");
  if (!SEMVER_PATTERN.test(nonEmptyString(source.version)))
    errors.push("version must be strict semver");
  if (!nonEmptyString(source.description)) errors.push("missing description");
  if (source.hooks !== undefined) errors.push("hooks field is not accepted");
  const skillsPath = nonEmptyString(source.skills);
  if (
    !pluginRelativePathExists(skillsPath) ||
    !fs.statSync(path.join(pluginRoot, skillsPath)).isDirectory()
  ) {
    errors.push("skills must point to an existing relative skills directory");
  }
  const mcpServersPath = nonEmptyString(source.mcpServers);
  if (
    !pluginRelativePathExists(mcpServersPath) ||
    path.basename(mcpServersPath) !== ".mcp.json"
  ) {
    errors.push("mcpServers must point to existing .mcp.json");
  }
  if (!objectOf(mcpManifest).mcpServers?.analytix_funds) {
    errors.push(".mcp.json must define analytix_funds server");
  }
  for (const field of PLUGIN_INTERFACE_REQUIRED_FIELDS) {
    if (!nonEmptyString(interfaceConfig[field]))
      errors.push(`interface.${field} is required`);
  }
  const defaultPrompt =
    interfaceConfig.defaultPrompt || interfaceConfig.default_prompt;
  if (
    !Array.isArray(defaultPrompt) ||
    !defaultPrompt.length ||
    defaultPrompt.length > 3 ||
    !defaultPrompt.every(nonEmptyString)
  ) {
    errors.push(
      "interface.defaultPrompt/default_prompt must contain 1-3 non-empty prompts",
    );
  } else if (
    defaultPrompt.some(
      (prompt) =>
        !/(?:当前案件|某主体|某人|某公司|目标主体|付款方|收款方)/u.test(
          prompt,
        ) || /(?:\b\d{8,}\b|20\d{2}-\d{2}-\d{2})/u.test(prompt),
    )
  ) {
    errors.push(
      "interface.defaultPrompt/default_prompt must stay case-neutral and must not contain fixed case identifiers or dates",
    );
  }
  if (
    !Array.isArray(interfaceConfig.capabilities) ||
    !interfaceConfig.capabilities.length ||
    !interfaceConfig.capabilities.every(nonEmptyString)
  ) {
    errors.push("interface.capabilities must be a non-empty string array");
  }
  for (const field of ["websiteURL", "privacyPolicyURL", "termsOfServiceURL"]) {
    if (
      interfaceConfig[field] !== undefined &&
      !isHttpsUrl(interfaceConfig[field])
    ) {
      errors.push(`interface.${field} must be an https URL`);
    }
  }
  if (source.author !== undefined) {
    const author = objectOf(source.author);
    if (!nonEmptyString(author.name)) errors.push("author.name is required");
    if (author.url !== undefined && !isHttpsUrl(author.url))
      errors.push("author.url must be an https URL");
  }
  if (source.homepage !== undefined && !isHttpsUrl(source.homepage))
    errors.push("homepage must be an https URL");
  if (source.repository !== undefined && !isHttpsUrl(source.repository))
    errors.push("repository must be an https URL");
  if (
    interfaceConfig.brandColor !== undefined &&
    !HEX_COLOR_PATTERN.test(nonEmptyString(interfaceConfig.brandColor))
  ) {
    errors.push("interface.brandColor must be #RRGGBB");
  }
  for (const field of ["composerIcon", "logo"]) {
    if (
      interfaceConfig[field] !== undefined &&
      !pluginRelativePathExists(interfaceConfig[field])
    ) {
      errors.push(
        `interface.${field} must point to an existing relative asset`,
      );
    }
  }
  return errors;
}

function sampleEvidenceLedgerPayload({ withSourceRefs = true } = {}) {
  const sourceRefs = withSourceRefs
    ? {
        query_ids: ["sql:sample-supported-edge"],
        evidence_ids: ["fact:sample-supported-edge"],
        audit_ref: {
          table: "analysis_txn_detail_idx",
          sql: "SELECT txn_ts, amount, acct_key, cp_key FROM analysis_txn_detail_idx WHERE ...",
        },
        detail_ref: {
          artifact_id: "artifact:sample-answer-card",
        },
      }
    : {};
  return {
    case_id: "doctor-ledger-sample",
    tool: "funds_investigate",
    facts: [
      {
        fact_id: "fact.amount.1",
        label: "合成主体甲向合成主体乙 2025-08-27 核心集中交易",
        amount_yuan: 21000000,
        txn_count: 5,
        account: "9000000000000000018",
        holder_name: "合成主体甲",
        counterparty_name: "合成主体乙",
        support_query_name: "sql:sample-supported-edge",
        source_hash: "sample-source-hash",
        source_refs: sourceRefs,
      },
    ],
    flow_graph: {
      edges: [
        {
          edge_id: "edge.sample.1",
          edge_status: "supported",
          from: "合成主体甲",
          from_label: "合成主体甲 / 9000000000000000018",
          to: "合成主体乙",
          to_label: "合成主体乙 / 9000000000000000015",
          amount_yuan: 21000000,
          txn_count: 5,
          txn_id: "txn.sample.1",
          txn_time: "2025-08-27",
          source_tool: "funds_investigate",
          source_refs: sourceRefs,
        },
      ],
    },
    verified_claims: [
      {
        claim_id: "claim.sample.1",
        text: "合成主体甲于 2025-08-27 向合成主体乙转出 2100 万元核心集中交易。",
        amount_yuan: 21000000,
        txn_count: 5,
        support_query_name: "sql:sample-supported-edge",
        fact_refs: ["fact.amount.1", "edge.sample.1"],
        source_refs: sourceRefs,
      },
    ],
    missing_source_boundaries: [
      {
        claim_id: "boundary.sample.1",
        text: "未返回 supported transaction edge 的链路不得画 Mermaid。",
        risk_marker: "unsupported_mermaid_or_arrow_flow",
        source_refs: sourceRefs,
      },
    ],
  };
}

function validateEvidenceLedgerDoctorContract() {
  const supportedLedger = buildEvidenceLedger(
    sampleEvidenceLedgerPayload({ withSourceRefs: true }),
  );
  const supported = validateEvidenceLedgerCoverage(supportedLedger);
  const unanchoredLedger = buildEvidenceLedger(
    sampleEvidenceLedgerPayload({ withSourceRefs: false }),
  );
  const unanchored = validateEvidenceLedgerCoverage(unanchoredLedger);
  const failures = [
    ...supported.failures.map(
      (failure) => `supported sample failed: ${failure}`,
    ),
    unanchored.ok
      ? "unanchored sample unexpectedly passed traceability validation"
      : "",
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    supported,
    unanchored,
  };
}

function validateAnswerCardContinuationContract() {
  const card = withAnswerCardProtocol({
    card_type: "top_rankings_card",
    title: "doctor ordinary answer card",
    facts: [
      {
        fact_id: "doctor.fact",
        label: "已返回普通研判事实",
        support_status: "supported",
      },
    ],
  });
  const visibleGuidance = JSON.stringify({
    stop_conditions: card.stop_conditions,
    answer_constraints: card.answer_constraints,
    next_review_actions: card.next_review_actions,
    context_compiler: objectOf(card.context_compiler),
  });
  const failures = [
    card.answer_card_complete !== true
      ? "answer_card_complete is not true"
      : "",
    card.required_facts_present !== false
      ? "model-provided support labels must not satisfy required_facts_present"
      : "",
    card.fact_answer_allowed !== false
      ? "answer_card_complete must not imply fact_answer_allowed"
      : "",
    !/(事实缺口|只能说明缺口|禁止把缺失字段渲染成 0)/u.test(visibleGuidance)
      ? "unsupported answer card must expose only the fixed evidence boundary"
      : "",
    /max_additional_tools/u.test(visibleGuidance)
      ? "ordinary user-visible guidance must not expose max_additional_tools"
      : "",
    /不要继续调用|不得继续|最多追加|最多再调用/u.test(visibleGuidance)
      ? "ordinary answer card still contains hard stop wording"
      : "",
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    card,
  };
}

function validateDataAnalyticsEquivalentBoundaryContract() {
  const sources = [
    "references/top-pluginization-plan.md",
    "references/blueprint-execution-plan.md",
    "skills/analytix-fund-analysis/SKILL.md",
  ];
  const requiredMarkers = [
    {
      id: "preflight envelope",
      patterns: [
        /preflight envelope|case_source_envelope|internal source record|current-case source record|数据来源与口径边界/iu,
        /任务上下文|case identity|current case|当前案件/iu,
      ],
    },
    {
      id: "semantic layer as map",
      patterns: [
        /semantic layer as map|semantic maps help find facts/iu,
        /地图|map/iu,
      ],
    },
    {
      id: "source-of-truth selection",
      patterns: [
        /source-of-truth selection|controlling source of truth|source of truth/iu,
        /控制来源|source_of_truth/iu,
      ],
    },
    {
      id: "source guardrail",
      patterns: [/source guardrail/iu, /必需来源|缺必需来源|required source/iu],
    },
    {
      id: "live/source-backed verification",
      patterns: [
        /live\/source-backed verification|live\/source-backed|source-backed/iu,
        /实际读取|verify data through|MCP/iu,
      ],
    },
    {
      id: "SQL/Python/notebook allowed when useful",
      patterns: [
        /SQL\s*\/?\s*Python\s*\/?\s*notebook allowed|SQL, Python, notebook|SQL\/Python\/notebook|not blanket-forbidden/iu,
        /不得自创禁令|没有禁止|allowed|允许/iu,
      ],
    },
    {
      id: "data quality checks",
      patterns: [
        /data quality checks|data-quality state|data quality/iu,
        /粒度|grain/iu,
        /重复|duplicates/iu,
      ],
    },
    {
      id: "validation state",
      patterns: [
        /validation state|核验状态/iu,
        /已验证|verified|已核验|已有数据支持/iu,
        /needs_review|blocked|unavailable|需补证|当前证据不足|未取得数据|待复核/iu,
      ],
    },
    {
      id: "delivery contract",
      patterns: [
        /delivery contract|delivery discipline|delivery_state|交付要求|交付边界|成果验收要求/iu,
        /真实生成|exist|artifact|交付材料|交付物|附件|图表|报告/iu,
      ],
    },
    {
      id: "render/execute QA",
      patterns: [
        /render\/execute QA|inspected in their final surface|执行状态/iu,
        /打开|渲染|execute/iu,
      ],
    },
    {
      id: "audience language",
      patterns: [
        /audience language/iu,
        /用户|办案用户|user-facing/iu,
        /内部状态|诊断标签|大 JSON|raw rows|source detail rows|来源明细/iu,
      ],
    },
    {
      id: "index/navigator not commander",
      patterns: [
        /index\s*\/?\s*navigator not commander|navigator not commander|index.*commander|root\/index\/funds_investigate/iu,
        /focused skill|navigator|能力发现|意图归类/iu,
      ],
    },
    {
      id: "focused workflow ownership",
      patterns: [
        /focused workflow ownership|focused workflows own|focused skill/iu,
        /workflow|completion gate|完成门|质量门/iu,
      ],
    },
    {
      id: "report completion gate",
      patterns: [
        /report completion gate/iu,
        /报告|report/iu,
        /claim|blocker|复核/iu,
      ],
    },
    {
      id: "scoped state/memory",
      patterns: [
        /scoped state|scoped state\/memory|不保存跨案|no cross-case|跨案事实记忆/iu,
        /Analytix-owned|本案内|selected Analytix case|系统 Codex/iu,
      ],
    },
  ];
  const failures = [];
  for (const relativePath of sources) {
    const body = readTextIfExists(path.join(pluginRoot, relativePath));
    if (!body) {
      failures.push(`${relativePath}: missing`);
      continue;
    }
    const missing = requiredMarkers
      .filter(
        (marker) => !marker.patterns.every((pattern) => pattern.test(body)),
      )
      .map((marker) => marker.id);
    if (missing.length)
      failures.push(`${relativePath}: missing ${missing.join(", ")}`);
  }
  return {
    ok: failures.length === 0,
    failures,
    source_count: sources.length,
    marker_count: requiredMarkers.length,
  };
}

function validateControlledCaseWorkbenchDoctorContract() {
  const schemaByToolName = new Map(
    MCP_TOOL_SCHEMAS.map((tool) => [
      text(tool.name),
      objectOf(tool.inputSchema),
    ]),
  );
  const cleanedExportSchema = objectOf(
    schemaByToolName.get("export_cleaned_case_data"),
  );
  const runSqlSchema = objectOf(schemaByToolName.get("run_case_sql"));
  const notebookSchema = objectOf(schemaByToolName.get("create_case_notebook"));
  const cleanedExportProperties = objectOf(cleanedExportSchema.properties);
  const runSqlProperties = objectOf(runSqlSchema.properties);
  const notebookProperties = objectOf(notebookSchema.properties);
  const runtimeSource = readTextIfExists(
    path.join(pluginRoot, "mcp", "tool-call-runtime.mjs"),
  );
  const cleanedExportRuntimeSource = readTextIfExists(
    path.join(pluginRoot, "mcp", "cleaned-export-runtime.mjs"),
  );
  const mcpConfigSource = readTextIfExists(path.join(pluginRoot, ".mcp.json"));
  const skillSource = readTextIfExists(
    path.join(pluginRoot, "skills", "case-workbench", "SKILL.md"),
  );
  const toolSchemasSource = readTextIfExists(
    path.join(pluginRoot, "mcp", "tool-schemas.mjs"),
  );
  const workbenchRuntimeSource = readTextIfExists(
    path.join(pluginRoot, "mcp", "duckdb-workbench-runtime.mjs"),
  );
  const evidenceLedgerSource = readTextIfExists(
    path.join(pluginRoot, "mcp", "evidence-ledger.mjs"),
  );
  const releaseSliceAudit = readTextIfExists(
    path.join(pluginRoot, "scripts", "release-slice-audit.mjs"),
  );
  const mcpRuntimeContractSource = [
    toolSchemasSource,
    runtimeSource,
    workbenchRuntimeSource,
    evidenceLedgerSource,
    skillSource,
  ].join("\n");
  const runtimeMarkers = [
    "CONTROLLED_CASE_WORKBENCH_TOOLS",
    "case_workbench_evidence_card",
    "runCaseSqlEvidenceCard",
    "source_hash",
    "metric_scope",
    "validation_state",
    "使用专项资金核算前必须说明用途或分析目标",
    "run_case_sql requires executable sql",
    "DDL/DML/extension/export statements are not allowed",
    "raw/source tables are not allowed",
    "external file/network/secret access is not allowed",
    "SQL must be complete",
    'allowed_view_policy = "cleaned_and_analysis_only"',
    "Math.min(Number.isFinite(rowLimit) ? rowLimit : 100, 500)",
    "CONTROLLED_CASE_WORKBENCH_UNAVAILABLE",
    "不得编造自定义查询结果",
  ];
  const skillMarkerGroups = [
    {
      id: "case source record",
      patterns: [
        /case_source_envelope/iu,
        /internal source record/iu,
        /当前案件来源记录/iu,
      ],
    },
    { id: "source_of_truth", patterns: [/source_of_truth/iu] },
    { id: "validation_state", patterns: [/validation_state/iu] },
    { id: "delivery_state", patterns: [/delivery_state/iu] },
    { id: "audit_case_data_quality", patterns: [/audit_case_data_quality/iu] },
    {
      id: "专项资金核算缺口说明",
      patterns: [/专项资金核算缺口说明/iu, /gap_card/iu],
    },
    {
      id: "weak-source substitution",
      patterns: [/weak-source substitution/iu, /弱来源替代/iu],
    },
    { id: "fake delivery", patterns: [/fake delivery/iu, /假交付/iu] },
    {
      id: "row limit at or below 500",
      patterns: [/row limit at or below 500/iu],
    },
  ];
  const failures = [
    !cleanedExportSchema.type ? "export_cleaned_case_data schema missing" : "",
    !arrayOf(cleanedExportSchema.required).includes("purpose")
      ? "export_cleaned_case_data must require purpose"
      : "",
    objectOf(cleanedExportProperties.confirm_export).type !== "boolean"
      ? "export_cleaned_case_data must expose confirm_export boolean"
      : "",
    Object.prototype.hasOwnProperty.call(cleanedExportProperties, "target_dir")
      ? "export_cleaned_case_data must not expose target_dir"
      : "",
    Number(objectOf(cleanedExportProperties.timeout_ms).maximum) !== 110000
      ? "export_cleaned_case_data timeout_ms max must be 110000"
      : "",
    !cleanedExportRuntimeSource.includes(
      "cleaned export output escaped Analytix-owned app data",
    )
      ? "cleaned export runtime missing Analytix-owned path boundary"
      : "",
    !cleanedExportRuntimeSource.includes("inspectCleanedExportDirectory")
      ? "cleaned export runtime missing file inspection"
      : "",
    !cleanedExportRuntimeSource.includes("confirm_export !== true")
      ? "cleaned export runtime missing explicit confirmation gate"
      : "",
    !mcpConfigSource.includes('"export_cleaned_case_data"') ||
    !mcpConfigSource.includes('"approval_mode": "prompt"')
      ? "export_cleaned_case_data must require MCP prompt approval"
      : "",
    !runSqlSchema.type ? "run_case_sql schema missing" : "",
    !notebookSchema.type ? "create_case_notebook schema missing" : "",
    !arrayOf(runSqlSchema.required).includes("purpose")
      ? "run_case_sql must require purpose"
      : "",
    arrayOf(runSqlSchema.required).includes("case_id")
      ? "run_case_sql must not require explicit case_id"
      : "",
    Number(objectOf(runSqlProperties.row_limit).maximum) !== 500
      ? "run_case_sql row_limit max must be 500"
      : "",
    arrayOf(objectOf(runSqlProperties.allowed_view_policy).enum).join(",") !==
    "cleaned_and_analysis_only"
      ? "run_case_sql allowed_view_policy must only allow cleaned_and_analysis_only"
      : "",
    !arrayOf(notebookSchema.required).includes("analysis_goal")
      ? "create_case_notebook must require analysis_goal"
      : "",
    arrayOf(notebookSchema.required).includes("case_id")
      ? "create_case_notebook must not require explicit case_id"
      : "",
    Number(objectOf(notebookProperties.max_rows_per_query).maximum) !== 500
      ? "create_case_notebook max_rows_per_query max must be 500"
      : "",
    arrayOf(objectOf(notebookProperties.allowed_view_policy).enum).join(",") !==
    "cleaned_and_analysis_only"
      ? "create_case_notebook allowed_view_policy must only allow cleaned_and_analysis_only"
      : "",
    ...[
      "sql_name_contract",
      "tables[].sql_name",
      "tables[].columns[].sql_name/sql_identifier",
      "sql_identifier",
      "display_name is UI-only",
    ]
      .filter((marker) => !mcpRuntimeContractSource.includes(marker))
      .map(
        (marker) =>
          `inspect_case_schema MCP/runtime contract missing ${marker}`,
      ),
    ...[
      "case_workbench_evidence_card",
      "source_hash",
      "metric_scope",
      "validation_state",
      "evidence_card",
    ]
      .filter((marker) => !mcpRuntimeContractSource.includes(marker))
      .map(
        (marker) =>
          `run_case_sql MCP/runtime evidence contract missing ${marker}`,
      ),
    ...[
      "plugins/analytix-fund-analysis/mcp/tool-schemas.mjs",
      "plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs",
      "plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs",
      "plugins/analytix-fund-analysis/mcp/evidence-ledger.mjs",
    ]
      .filter((marker) => !releaseSliceAudit.includes(marker))
      .map(
        (marker) => `release slice missing MCP/runtime contract file ${marker}`,
      ),
    ...runtimeMarkers
      .filter((marker) => !runtimeSource.includes(marker))
      .map((marker) => `runtime missing marker ${marker}`),
    ...skillMarkerGroups
      .filter(
        (marker) =>
          !marker.patterns.some((pattern) => pattern.test(skillSource)),
      )
      .map((marker) => `case-workbench skill missing marker ${marker.id}`),
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    runtime_marker_count: runtimeMarkers.length,
    skill_marker_count: skillMarkerGroups.length,
  };
}

function validateClaimVerifierDoctorContract() {
  const riskyReportText = [
    "报告写入草稿：候选账户已确认归属合成主体丙名下。",
    "双空对手方资金闭环已确认，现金/理财资产最终去向已查明。",
    "经核查，确定违法所得 42,000,000 元，最终资金归属已确认。",
    "| 对手方 | 金额 | 笔数 |",
    "| --- | ---: | ---: |",
    "| 张某 | 42000000 元 | 18 |",
    "## 下一步核查建议",
    "```mermaid",
    "flowchart TD",
    "候选账户 --> 现金资产",
    "```",
  ].join("\n");
  const card = withClaimReviewProtocol({
    report_text: riskyReportText,
    facts: [],
    unsupported_claims: [],
    unsupported_flows: [],
  });
  const safeTableCard = withClaimReviewProtocol({
    report_text: [
      "| 对手方 | 金额 | 笔数 |",
      "| --- | ---: | ---: |",
      "| 张某 | 42000000 元 | 18 |",
      "表后研判意见：上述交易集中于张某账户，金额和笔数均较高，对判断资金去向具有证明价值；现有材料仍不能证明最终归属，需调取张某账户后续流水和回单固定。",
    ].join("\n"),
    facts: [
      { fact_id: "fact.safe.table", support_query_name: "safe_table_query" },
    ],
    unsupported_claims: [],
    unsupported_flows: [],
  });
  const serialized = JSON.stringify(card);
  const requiredMarkers = [
    "amount_without_fact_anchor",
    "unsupported_mermaid_or_arrow_flow",
    "candidate_account_as_owned_account",
    "cash_or_asset_destination_without_supported_flow",
    "missing_counterparty_or_cash_break_as_verified",
    "table_without_analysis",
    "claim_without_source_anchor",
    "forbidden_legal_phrasing",
    "claim_text_risk_scan",
  ];
  const visibleGuidance = JSON.stringify({
    stop_conditions: card.stop_conditions,
    answer_constraints: card.answer_constraints,
    next_review_actions: card.next_review_actions,
  });
  const failures = [
    card.write_blocked !== true ? "write_blocked is not true" : "",
    /max_additional_tools/u.test(visibleGuidance)
      ? "claim review visible guidance must not expose max_additional_tools"
      : "",
    !/(继续追一层|改口径|新对象|新假设|新的追查目标)/u.test(visibleGuidance)
      ? "claim review guidance must not block new investigative work"
      : "",
    card.unsupported_flows_present !== true
      ? "unsupported_flows_present is not true"
      : "",
    !arrayOf(card.claim_support_index).length
      ? "claim_support_index missing"
      : "",
    !arrayOf(card.claim_support_index).every(
      (entry) => Object.keys(objectOf(objectOf(entry).source_refs)).length,
    )
      ? "claim_support_index entries must carry source_refs"
      : "",
    objectOf(safeTableCard.claim_text_risk_scan).table_without_analysis === true
      ? "safe table-after-analysis paragraph must not trigger table_without_analysis"
      : "",
    ...requiredMarkers
      .filter((marker) => !serialized.includes(marker))
      .map((marker) => `missing risk marker ${marker}`),
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    card,
  };
}

function validateInvestigationAnswerDoctorContract() {
  const validAnswer = {
    investigation_objective: {
      purpose: "核验甲账户向乙账户转账后是否形成资金承接和分散转出线索",
      scope: "甲账户至乙账户直接交易及乙账户后一跳出账",
      time_range: "2024-01-01 至 2024-03-31",
      source_boundary:
        "当前案件已清洗交易明细、资金流向图谱和专项资金核算 evidence_card",
    },
    conclusion: {
      summary:
        "现有流水支持甲账户向乙账户存在集中转入，乙账户后续存在分散转出特征；资金性质和最终归属暂不能认定。",
      evidence_maturity: "可疑特征",
    },
    verified_facts: [
      {
        fact_id: "fact.direct-transfer",
        fact: "甲账户向乙账户转入 12 笔，合计 123456.78 元。",
        amount_yuan: 123456.78,
        txn_count: 12,
        parties: ["甲账户", "乙账户"],
        support_status: "supported",
        evidence_receipt_ids: ["receipt.fact.direct-transfer"],
        evidence_refs: {
          query_ids: ["run_case_sql.direct-transfer"],
          evidence_ids: ["analysis_txn_detail_idx:txn-001"],
          source_hash: "doctor-source-hash",
        },
      },
    ],
    fund_flow_paths: [
      {
        path_summary: "甲账户 -> 乙账户",
        edges: [
          {
            edge_id: "edge.supported.1",
            from: "甲账户",
            to: "乙账户",
            amount_yuan: 123456.78,
            txn_count: 12,
            support_status: "supported",
            evidence_receipt_ids: ["receipt.edge.supported.1"],
            evidence_refs: {
              evidence_ids: ["analysis_relation_edge:edge.supported.1"],
            },
          },
        ],
      },
    ],
    risk_patterns: [
      {
        pattern: "短期集中转入后分散转出",
        basis: "由直接转账事实和后一跳出账事实支撑。",
        related_fact_ids: ["fact.direct-transfer"],
      },
    ],
    evidence_boundaries: [
      {
        boundary: "现有数据不能直接证明资金性质、账户控制关系或最终归属。",
        next_proof_action: "调取开户资料、交易回单和下游账户流水。",
      },
    ],
    next_proof_actions: [
      {
        action: "调取乙账户下游流水和交易对手身份资料",
        material: "后续流水、开户资料、交易回单",
        proof_value: "核实资金承接关系、实际用途和最终去向。",
      },
    ],
    visible_final_answer:
      "经核验现有交易明细，甲账户向乙账户形成集中转入关系，并存在后续分散转出特征；资金性质和最终归属仍需进一步调取开户资料、交易回单及下游账户流水予以印证。",
  };
  const hostReceiptOptions = {
    verifyEvidenceReceipt({ receiptId, claimType, claim }) {
      if (claimType === "verified_fact") {
        return (
          receiptId === "receipt.fact.direct-transfer" &&
          Number(claim.amount_yuan) === 123456.78 &&
          Number(claim.txn_count) === 12 &&
          arrayOf(claim.parties).join("|") === "甲账户|乙账户"
        );
      }
      if (claimType === "fund_flow_edge") {
        return (
          receiptId === "receipt.edge.supported.1" &&
          Number(claim.amount_yuan) === 123456.78 &&
          Number(claim.txn_count) === 12 &&
          text(claim.from) === "甲账户" &&
          text(claim.to) === "乙账户"
        );
      }
      return false;
    },
  };
  const valid = validateInvestigationAnswerContract(
    validAnswer,
    hostReceiptOptions,
  );

  const missingEvidence = {
    ...validAnswer,
    verified_facts: [
      {
        fact: "甲账户向乙账户转入 12 笔，合计 123456.78 元。",
        support_status: "supported",
      },
    ],
  };
  const missingEvidenceResult = validateInvestigationAnswerContract(
    missingEvidence,
    hostReceiptOptions,
  );

  const candidateFlow = JSON.parse(JSON.stringify(validAnswer));
  candidateFlow.fund_flow_paths[0].edges[0].support_status = "candidate";
  const candidateFlowResult = validateInvestigationAnswerContract(
    candidateFlow,
    hostReceiptOptions,
  );

  const legalUpgrade = {
    ...validAnswer,
    conclusion: {
      summary: "现有流水已经坐实实际控制并锁定违法所得。",
      evidence_maturity: "已核验事实",
    },
  };
  const legalUpgradeResult = validateInvestigationAnswerContract(
    legalUpgrade,
    hostReceiptOptions,
  );
  const claimReviewCard = withClaimReviewProtocol({
    report_text: "报告拟写：甲账户向乙账户转入 12 笔，合计 123456.78 元。",
    facts: [],
    final_investigation_answer: missingEvidence,
  });
  const claimReviewText = JSON.stringify({
    unsupported_claims: claimReviewCard.unsupported_claims,
    missing_source_boundaries: claimReviewCard.missing_source_boundaries,
    next_review_actions: claimReviewCard.next_review_actions,
    contract_review: claimReviewCard.investigation_answer_contract_review,
  });

  const codes = (result) =>
    new Set(arrayOf(result.errors).map((error) => text(error.code)));
  const failures = [
    !valid.ok
      ? `valid investigation answer failed: ${arrayOf(valid.errors)
          .map((error) => error.code)
          .join(", ")}`
      : "",
    !arrayOf(valid.audit_events).some(
      (event) =>
        objectOf(event).event === "investigation_answer_validation:passed",
    )
      ? "valid answer must emit passed audit event"
      : "",
    missingEvidenceResult.ok ? "missing evidence refs unexpectedly passed" : "",
    !codes(missingEvidenceResult).has("FACT_EVIDENCE_RECEIPT_REJECTED")
      ? "missing evidence must return FACT_EVIDENCE_RECEIPT_REJECTED"
      : "",
    !arrayOf(missingEvidenceResult.repair_actions).some((item) =>
      /Evidence Registry|receipt/iu.test(text(item.fact_policy)),
    )
      ? "missing fact repair actions must route back to the host Evidence Registry"
      : "",
    candidateFlowResult.ok ? "candidate flow edge unexpectedly passed" : "",
    !codes(candidateFlowResult).has("FLOW_EDGE_NOT_SUPPORTED")
      ? "candidate flow must return FLOW_EDGE_NOT_SUPPORTED"
      : "",
    legalUpgradeResult.ok ? "unguarded legal upgrade unexpectedly passed" : "",
    !codes(legalUpgradeResult).has("LEGAL_CONCLUSION_UNSUPPORTED")
      ? "legal upgrade must return LEGAL_CONCLUSION_UNSUPPORTED"
      : "",
    claimReviewCard.write_blocked !== true
      ? "claim review must block invalid FinalInvestigationAnswer"
      : "",
    !claimReviewText.includes("FACT_EVIDENCE_RECEIPT_REJECTED")
      ? "claim review must surface FinalInvestigationAnswer validation code"
      : "",
    !claimReviewText.includes("investigation_answer_contract_failed")
      ? "claim review must retain contract risk marker"
      : "",
    !/Evidence Registry|receipt|MCP\/DuckDB|evidence_refs|source_refs/iu.test(
      claimReviewText,
    )
      ? "claim review must route missing structure facts back to source evidence"
      : "",
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    valid,
    missingEvidenceResult,
    candidateFlowResult,
    legalUpgradeResult,
    claimReviewCard,
  };
}

function validateFundGraphEvidencePackDoctorContract() {
  const unsupportedEdges = [
    {
      edge_id: "edge.candidate.1",
      edge_status: "candidate",
      missing_fields: ["counterparty_identity"],
      txn_id: "txn.candidate.1",
      txn_time: "2025-08-27 10:20:00",
      amount: { yuan: 880000, text: "880000.00 元" },
      from: "account:target",
      to: "counterparty:candidate",
    },
    {
      edge_id: "edge.partial.1",
      edge_status: "partial",
      missing_fields: ["source_receipt"],
      txn_id: "txn.partial.1",
      amount: { yuan: 660000, text: "660000.00 元" },
      from: "account:target",
      to: "counterparty:partial",
    },
    {
      edge_id: "edge.missing.1",
      edge_status: "missing",
      missing_fields: ["txn_id", "txn_time"],
      amount: { yuan: 550000, text: "550000.00 元" },
      from: "account:target",
      to: "counterparty:missing",
    },
    {
      edge_id: "edge.needs_review.1",
      edge_status: "needs_review",
      missing_fields: ["review_gate"],
      txn_id: "txn.needs_review.1",
      txn_time: "2025-08-27 10:40:00",
      amount: { yuan: 440000, text: "440000.00 元" },
      from: "account:target",
      to: "counterparty:needs-review",
    },
    {
      edge_id: "edge.needs_evidence.1",
      edge_status: "needs_evidence",
      missing_fields: ["txn_time"],
      amount: { yuan: 330000, text: "330000.00 元" },
      from: "account:target",
      to: "counterparty:cash-break",
    },
  ];
  const graph = withFundGraphProtocol({
    graph_version: "fund-flow-graph-v1",
    nodes: [
      {
        id: "account:source",
        label: "合成主体甲",
        kind: "source_account_or_holder",
      },
      { id: "account:target", label: "合成主体乙", kind: "counterparty" },
    ],
    edges: [
      {
        edge_id: "edge.supported.1",
        edge_status: "supported",
        txn_id: "txn.supported.1",
        txn_time: "2025-08-27 10:00:00",
        amount: { yuan: 21000000, text: "21000000.00 元" },
        direction: "out",
        from: "account:source",
        from_label: "合成主体甲 / 9000000000000000018",
        to: "account:target",
        to_label: "合成主体乙 / 9000000000000000015",
        source_tool: "trace_subject_top_outflows",
        source_refs: {
          query_ids: ["trace_subject_top_outflows"],
          evidence_ids: ["txn.supported.1"],
          audit_ref: { table: "analysis_txn_detail_idx" },
        },
      },
      ...unsupportedEdges,
      {
        edge_id: "edge.empty-status.ignored",
        edge_status: "",
        amount: { yuan: 0, text: "0.00 元" },
        from: "",
        to: "",
        source_tool: "trace_subject_top_outflows",
      },
    ],
    evidence_pack: [
      {
        pack_id: "polluted.evidence_pack.candidate",
        edge_id: "edge.candidate.1",
        support_status: "candidate",
        mermaid_allowed: true,
      },
      {
        pack_id: "polluted.evidence_pack.supported",
        edge_id: "edge.supported.1",
        txn_id: "txn.supported.1",
        support_status: "supported",
        mermaid_allowed: true,
      },
    ],
    claim_support: [
      {
        claim_id: "polluted.claim_support.candidate",
        edge_id: "edge.candidate.1",
        support_status: "supported",
        claim_text: "candidate edge should not be report support",
        fact_refs: ["edge.candidate.1"],
      },
      {
        claim_id: "polluted.claim_support.supported",
        edge_id: "edge.supported.1",
        support_status: "supported",
        fact_refs: ["edge.supported.1"],
      },
    ],
  });
  const evidencePack = arrayOf(graph.evidence_pack);
  const claimSupport = arrayOf(graph.claim_support);
  const evidenceBoundaries = arrayOf(graph.evidence_boundaries);
  const supportedCount = Number(graph.supported_edge_count || 0);
  const unsupportedEdgeIds = unsupportedEdges.map((edge) => edge.edge_id);
  const supportJson = JSON.stringify({ evidencePack, claimSupport });
  const boundaryJson = JSON.stringify(evidenceBoundaries);
  const leakedUnsupportedSupportIds = unsupportedEdgeIds.filter((edgeId) =>
    supportJson.includes(edgeId),
  );
  const missingBoundaryStatuses = [
    "candidate",
    "partial",
    "missing",
    "needs_review",
    "needs_evidence",
  ].filter((status) => !boundaryJson.includes(status));
  const failures = [
    supportedCount !== 1 ? `supported_edge_count ${supportedCount} != 1` : "",
    evidencePack.length !== supportedCount
      ? "evidence_pack must contain exactly supported edges"
      : "",
    claimSupport.length !== supportedCount
      ? "claim_support must contain exactly supported edges"
      : "",
    evidenceBoundaries.length < unsupportedEdges.length
      ? "unsupported/candidate edges must become evidence_boundaries"
      : "",
    leakedUnsupportedSupportIds.length
      ? `unsupported edge leaked into support surface: ${leakedUnsupportedSupportIds.join(", ")}`
      : "",
    missingBoundaryStatuses.length
      ? `missing boundary statuses: ${missingBoundaryStatuses.join(", ")}`
      : "",
    !objectOf(evidencePack[0]).mermaid_allowed
      ? "supported evidence_pack must mark mermaid_allowed=true"
      : "",
    !Object.keys(objectOf(objectOf(evidencePack[0]).source_refs)).length
      ? "evidence_pack source_refs missing"
      : "",
    !Object.keys(objectOf(objectOf(claimSupport[0]).source_refs)).length
      ? "claim_support source_refs missing"
      : "",
    !arrayOf(objectOf(claimSupport[0]).fact_refs).includes("edge.supported.1")
      ? "claim_support fact_refs missing supported edge id"
      : "",
    !arrayOf(graph.next_review_actions).length
      ? "evidence boundary next_review_actions missing"
      : "",
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    graph,
  };
}

function validateVisualArtifactDoctorContract() {
  const codes = (result) =>
    new Set(arrayOf(result.errors).map((error) => text(error.code)));
  const validTable = validateVisualArtifactContract({
    artifact_type: "evidence_table",
    title: "重点账户大额支出证据表",
    scope: {
      unit: "元",
      time_window: "2025-08-01 至 2025-08-31",
      metric: "转出金额",
      direction: "支出",
      evidence_status: "已由交易明细支持",
    },
    rows: [
      {
        account_no: "9000000000000000018",
        counterparty_account: "9000000000000000015",
        amount: "21000000.00",
        txn_time: "2025-08-27 10:00:00",
        txn_id: "txn.supported.1",
        evidence_refs: ["txn.supported.1"],
      },
    ],
    table_after_analysis:
      "该笔支出金额较大，可作为重点资金去向线索，资金性质仍需结合合同、票据及对手方身份材料继续印证。",
  });
  const invalidTable = validateVisualArtifactContract({
    artifact_type: "evidence_table",
    title: "SQL debug candidate table",
    rows: [{ account_no: "9000000000000000018", amount: "21000000.00" }],
  });
  const validGraph = validateVisualArtifactContract({
    artifact_type: "fund_flow_graph",
    title: "合成主体甲账户资金流向图",
    edges: [
      {
        edge_id: "edge.supported.1",
        edge_status: "supported",
        from_label: "合成主体甲 / 9000000000000000018",
        to_label: "合成主体乙 / 9000000000000000015",
        amount: "21000000.00 元",
        txn_time: "2025-08-27 10:00:00",
        txn_id: "txn.supported.1",
        source_refs: {
          query_ids: ["trace_subject_top_outflows"],
          evidence_ids: ["txn.supported.1"],
        },
      },
    ],
  });
  const invalidGraph = validateVisualArtifactContract({
    artifact_type: "fund_flow_graph",
    title: "候选资金流向图",
    edges: [
      {
        edge_id: "edge.needs_review.1",
        edge_status: "needs_review",
        from_label: "合成主体甲 / 9000000000000000018",
        to_label: "待核收款账户",
        amount: "440000.00 元",
        txn_time: "2025-08-27 10:40:00",
        txn_id: "txn.needs_review.1",
      },
    ],
  });
  const validImage = validateVisualArtifactContract({
    artifact_type: "png",
    title: "重点账户收支结构图",
    inspection_status: "passed",
    nonblank: true,
    source_refs: { query_ids: ["account_income_expense_summary"] },
  });
  const invalidImage = validateVisualArtifactContract({
    artifact_type: "png",
    title: "MCP debug chart",
    inspection_status: "pending",
  });
  const invalidTableCodes = codes(invalidTable);
  const invalidGraphCodes = codes(invalidGraph);
  const invalidImageCodes = codes(invalidImage);
  const failures = [
    !validTable.ok
      ? `valid evidence table failed: ${arrayOf(validTable.errors)
          .map((error) => text(error.code))
          .join(", ")}`
      : "",
    invalidTable.ok ? "invalid evidence table unexpectedly passed" : "",
    !invalidTableCodes.has("VISUAL_SCOPE_FIELD_MISSING")
      ? "invalid evidence table must require visible scope fields"
      : "",
    !invalidTableCodes.has("VISUAL_ROW_SOURCE_ANCHOR_MISSING")
      ? "invalid evidence table must require row source anchors"
      : "",
    !invalidTableCodes.has("TABLE_AFTER_ANALYSIS_MISSING")
      ? "invalid evidence table must require table-after analysis"
      : "",
    !invalidTableCodes.has("VISIBLE_LABEL_INTERNAL_TERM")
      ? "invalid evidence table must block internal visible terms"
      : "",
    !validGraph.ok
      ? `valid fund-flow graph failed: ${arrayOf(validGraph.errors)
          .map((error) => text(error.code))
          .join(", ")}`
      : "",
    invalidGraph.ok ? "invalid fund-flow graph unexpectedly passed" : "",
    !invalidGraphCodes.has("GRAPH_EDGE_NOT_SUPPORTED")
      ? "invalid fund-flow graph must block non-supported edges"
      : "",
    !validImage.ok
      ? `valid image artifact failed: ${arrayOf(validImage.errors)
          .map((error) => text(error.code))
          .join(", ")}`
      : "",
    invalidImage.ok ? "invalid image artifact unexpectedly passed" : "",
    !invalidImageCodes.has("ARTIFACT_FILE_INSPECTION_NOT_PASSED")
      ? "invalid image must require passed inspection"
      : "",
    !invalidImageCodes.has("IMAGE_NONBLANK_CHECK_MISSING")
      ? "invalid image must require nonblank check"
      : "",
    !invalidImageCodes.has("VISIBLE_LABEL_INTERNAL_TERM")
      ? "invalid image must block internal visible terms"
      : "",
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    validTable,
    invalidTable,
    validGraph,
    invalidGraph,
    validImage,
    invalidImage,
  };
}

function validateCasegraphRankEvidencePackDoctorContract() {
  const rankPayload = {
    case_id: "case-doctor",
    holder_name: "合成主体甲",
    id_no: "provided",
    account_keys: ["9000000000000000018"],
    metric: "turnover",
    success_filter: "all",
    cash_filter: "all",
    limit: 5,
  };
  const rankSources = [
    buildCasegraphRankEvidence({
      rows: [
        {
          rank: 1,
          account_key: "9000000000000000018",
          account_display: "账户 9000000000000000018",
          holder_name: "合成主体甲",
          txn_count: 12,
          turnover: { yuan: 42000000, text: "42000000.00 元" },
        },
      ],
      rankKind: "account",
      sourceTool: "rank_accounts",
      evidenceRefs: {
        query_ids: ["rank_accounts"],
        evidence_ids: ["rank_accounts.result.1"],
        audit_ref: { table: "analysis_rank_accounts" },
      },
      rankPayload,
      caseId: "case-doctor",
    }),
    buildCasegraphRankEvidence({
      rows: [
        {
          rank: 1,
          holder_name: "合成主体乙",
          account_count: 2,
          txn_count: 8,
          turnover: { yuan: 21000000, text: "21000000.00 元" },
        },
      ],
      rankKind: "holder",
      sourceTool: "rank_holders",
      evidenceRefs: {
        query_ids: ["rank_holders"],
        evidence_ids: ["rank_holders.result.1"],
        audit_ref: { table: "analysis_rank_holders" },
      },
      rankPayload,
      caseId: "case-doctor",
    }),
    buildCasegraphRankEvidence({
      rows: [
        {
          rank: 1,
          counterparty_key: "9000000000000000015",
          counterparty_account: "9000000000000000015",
          display_name: "合成主体乙",
          txn_count: 6,
          turnover: { yuan: 10500000, text: "10500000.00 元" },
        },
      ],
      rankKind: "counterparty",
      sourceTool: "rank_counterparties",
      evidenceRefs: {
        query_ids: ["rank_counterparties"],
        evidence_ids: ["rank_counterparties.result.1"],
        audit_ref: { table: "analysis_rank_counterparties" },
      },
      rankPayload,
      caseId: "case-doctor",
    }),
  ];
  const rows = rankSources.flatMap((source) => arrayOf(source.rows));
  const evidencePack = rankSources.flatMap((source) =>
    arrayOf(source.evidence_pack),
  );
  const claimSupport = rankSources.flatMap((source) =>
    arrayOf(source.claim_support),
  );
  const ledger = buildEvidenceLedger({
    tool: "get_casegraph",
    case_id: "case-doctor",
    evidence_refs: {
      query_ids: [
        "get_casegraph",
        "rank_accounts",
        "rank_holders",
        "rank_counterparties",
      ],
      audit_ref: { doctor: "casegraph_rank_evidence_pack" },
    },
    key_facts: {
      top_accounts: rankSources[0].rows,
      top_holders: rankSources[1].rows,
      top_counterparties: rankSources[2].rows,
      casegraph_evidence_pack: evidencePack,
      claim_support: claimSupport,
    },
    casegraph: {
      source_refs: {
        query_ids: ["get_casegraph"],
        audit_ref: { doctor: "casegraph_rank_evidence_pack" },
      },
      top_accounts: rankSources[0].rows,
      top_holders: rankSources[1].rows,
      top_counterparties: rankSources[2].rows,
      evidence_pack: evidencePack,
      claim_support: claimSupport,
    },
  });
  const ledgerSummary = objectOf(ledger.coverage_summary);
  const failures = [
    rows.length !== 3
      ? `expected 3 annotated rank rows, got ${rows.length}`
      : "",
    evidencePack.length !== 3
      ? `expected 3 rank evidence packs, got ${evidencePack.length}`
      : "",
    claimSupport.length !== 3
      ? `expected 3 rank claim supports, got ${claimSupport.length}`
      : "",
    !rows.every((row) => text(objectOf(row).fact_id))
      ? "rank rows must carry fact_id"
      : "",
    !rows.every(
      (row) => Object.keys(objectOf(objectOf(row).source_refs)).length,
    )
      ? "rank rows must carry source_refs"
      : "",
    !evidencePack.every(
      (pack) => Object.keys(objectOf(objectOf(pack).source_refs)).length,
    )
      ? "rank evidence_pack entries must carry source_refs"
      : "",
    !claimSupport.every(
      (claim) => Object.keys(objectOf(objectOf(claim).source_refs)).length,
    )
      ? "rank claim_support entries must carry source_refs"
      : "",
    !claimSupport.every((claim) => arrayOf(objectOf(claim).fact_refs).length)
      ? "rank claim_support entries must carry fact_refs"
      : "",
    JSON.stringify(
      claimSupport.map((claim) => objectOf(claim).claim_text),
    ).includes("名下账户确认")
      ? "rank claim support must not upgrade candidate/aggregate facts into ownership confirmation"
      : "",
    Number(ledgerSummary.untraceable_required_surface_entry_count || 0) !== 0
      ? `rank ledger has ${ledgerSummary.untraceable_required_surface_entry_count} untraceable required-surface entries`
      : "",
    Number(ledgerSummary.entries_with_source_refs || 0) <= 0
      ? "rank ledger has no entries with source_refs"
      : "",
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    evidencePack,
    claimSupport,
    ledgerSummary,
  };
}

function validateCasegraphComponentEvidencePackDoctorContract() {
  const componentStatuses = [
    "ready_from_rank_facts",
    "ready_from_rank_facts",
    "ready_from_holder_facts",
    "ready_from_rank_facts",
    "needs_supported_trace_fact",
    "needs_duplicate_family_review",
    "ready_from_gates",
    "roadmap_ready_probe_required",
    "roadmap_ready_source_gap_required",
    "roadmap_ready_evidence_pack_required",
    "needs_validate_report_claims_for_report_tasks",
  ];
  const components = CASEGRAPH_REQUIRED_NODE_EDGE_NAMES.map((name, index) => ({
    name,
    status: componentStatuses[index] || "needs_review",
    required_facts: [`${name}.deterministic_fact`, `${name}.source_ref`],
    boundary: `${name} doctor boundary；非 supported 组件只能写为线索、缺口或 next_review_action。`,
  }));
  const evidenceRefs = {
    query_ids: ["get_casegraph", "get_case_scope_map"],
    evidence_ids: ["casegraph.scope.doctor"],
    audit_ref: { doctor: "casegraph_component_evidence_pack" },
  };
  const componentEvidence = buildCasegraphComponentEvidencePack({
    graph: {
      minimum_roadmap: {
        minimum_nodes_and_edges: components,
      },
    },
    scopeMap: {
      mandatory_gates: [
        { gate: "source_audit", status: "passed" },
        { gate: "report_gate", status: "needs_review" },
      ],
    },
    evidenceRefs,
    caseId: "case-doctor",
  });
  const evidencePack = arrayOf(componentEvidence.evidence_pack);
  const claimSupport = arrayOf(componentEvidence.claim_support);
  const evidenceBoundaries = arrayOf(componentEvidence.evidence_boundaries);
  const ledger = buildEvidenceLedger({
    tool: "get_casegraph",
    case_id: "case-doctor",
    evidence_refs: evidenceRefs,
    key_facts: {
      casegraph_evidence_pack: evidencePack,
      claim_support: claimSupport,
      evidence_boundaries: evidenceBoundaries,
    },
    casegraph: {
      source_refs: evidenceRefs,
      evidence_pack: evidencePack,
      claim_support: claimSupport,
      evidence_boundaries: evidenceBoundaries,
      component_status_counts: componentEvidence.component_status_counts,
    },
  });
  const ledgerSummary = objectOf(ledger.coverage_summary);
  const packComponents = evidencePack.map((pack) =>
    text(objectOf(pack).component),
  );
  const unsupportedClaimCount = claimSupport.filter(
    (claim) => text(objectOf(claim).support_status) !== "supported",
  ).length;
  const failures = [
    evidencePack.length !== CASEGRAPH_REQUIRED_NODE_EDGE_NAMES.length
      ? `expected ${CASEGRAPH_REQUIRED_NODE_EDGE_NAMES.length} component evidence packs, got ${evidencePack.length}`
      : "",
    claimSupport.length !== CASEGRAPH_REQUIRED_NODE_EDGE_NAMES.length
      ? `expected ${CASEGRAPH_REQUIRED_NODE_EDGE_NAMES.length} component claim supports, got ${claimSupport.length}`
      : "",
    evidenceBoundaries.length <= 0
      ? "component evidence boundaries missing"
      : "",
    CASEGRAPH_REQUIRED_NODE_EDGE_NAMES.find(
      (name) => !packComponents.includes(name),
    )
      ? `missing component ${CASEGRAPH_REQUIRED_NODE_EDGE_NAMES.find((name) => !packComponents.includes(name))}`
      : "",
    !evidencePack.every((pack) => text(objectOf(pack).fact_id))
      ? "component evidence_pack entries must carry fact_id"
      : "",
    !evidencePack.every(
      (pack) => Object.keys(objectOf(objectOf(pack).source_refs)).length,
    )
      ? "component evidence_pack entries must carry source_refs"
      : "",
    !claimSupport.every(
      (claim) => Object.keys(objectOf(objectOf(claim).source_refs)).length,
    )
      ? "component claim_support entries must carry source_refs"
      : "",
    !claimSupport.every((claim) => arrayOf(objectOf(claim).fact_refs).length)
      ? "component claim_support entries must carry fact_refs"
      : "",
    unsupportedClaimCount <= 0
      ? "lead/needs_evidence components must remain downgraded in claim_support"
      : "",
    !evidenceBoundaries.every(
      (boundary) =>
        Object.keys(objectOf(objectOf(boundary).source_refs)).length,
    )
      ? "component evidence_boundaries must carry source_refs"
      : "",
    !evidenceBoundaries.every((boundary) =>
      text(objectOf(boundary).next_review_action),
    )
      ? "component evidence_boundaries must carry next_review_action"
      : "",
    Number(ledgerSummary.untraceable_required_surface_entry_count || 0) !== 0
      ? `component ledger has ${ledgerSummary.untraceable_required_surface_entry_count} untraceable required-surface entries`
      : "",
    Number(ledgerSummary.entries_with_source_refs || 0) <= 0
      ? "component ledger has no entries with source_refs"
      : "",
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    evidencePack,
    claimSupport,
    evidenceBoundaries,
    ledgerSummary,
  };
}

function quotedStrings(source) {
  return [...String(source || "").matchAll(/"([^"]+)"/gu)].map(
    (match) => match[1],
  );
}

function parseArgs(argv) {
  const options = { json: false, skipGit: false, skipRuntime: false };
  for (const arg of argv) {
    if (arg === "--json") options.json = true;
    else if (arg === "--skip-git") options.skipGit = true;
    else if (arg === "--skip-runtime") options.skipRuntime = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/doctor.mjs [options]

Options:
  --json          Print machine-readable JSON.
  --skip-git      Do not inspect git diff boundaries.
  --skip-runtime  Do not inspect installed Analytix runtime cache.
`);
}

class DoctorReport {
  constructor() {
    this.results = [];
  }

  add(name, status, detail = "") {
    this.results.push({ name, status, detail: text(detail) });
  }

  pass(name, detail = "") {
    this.add(name, "pass", detail);
  }

  warn(name, detail = "") {
    this.add(name, "warn", detail);
  }

  fail(name, detail = "") {
    this.add(name, "fail", detail);
  }

  get failed() {
    return this.results.some((item) => item.status === "fail");
  }

  print(json = false) {
    if (json) {
      console.log(
        JSON.stringify({ ok: !this.failed, results: this.results }, null, 2),
      );
      return;
    }
    for (const result of this.results) {
      const marker =
        result.status === "pass"
          ? "PASS"
          : result.status === "warn"
            ? "WARN"
            : "FAIL";
      console.log(
        `[${marker}] ${result.name}${result.detail ? ` - ${result.detail}` : ""}`,
      );
    }
  }
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function extractServerTools(serverSource) {
  const tools = new Set();
  const regex = /name:\s*"([a-zA-Z0-9_]+)"/gu;
  let match = regex.exec(serverSource);
  while (match) {
    tools.add(match[1]);
    match = regex.exec(serverSource);
  }
  return tools;
}

function commandToolNames(command) {
  return Array.isArray(command.primaryTools)
    ? command.primaryTools.map(text).filter(Boolean)
    : [];
}

function extractRouterCommands(routerSource) {
  const commands = [];
  const regex = /\| `(\/analytix[^`]*)` \|/gu;
  let match = regex.exec(routerSource);
  while (match) {
    commands.push(text(match[1]));
    match = regex.exec(routerSource);
  }
  return commands;
}

function runGitDiffNameOnly() {
  const result = spawnSync("git", ["diff", "--name-only"], {
    cwd: repoRoot,
    encoding: "utf8",
  });
  if (result.status !== 0) {
    throw new Error(text(result.stderr || result.stdout) || "git diff failed");
  }
  return result.stdout.split(/\r?\n/u).map(text).filter(Boolean);
}

function runGitStatusPaths() {
  const result = spawnSync(
    "git",
    ["status", "--short", "--untracked-files=all"],
    {
      cwd: repoRoot,
      encoding: "utf8",
    },
  );
  if (result.status !== 0) {
    throw new Error(
      text(result.stderr || result.stdout) || "git status failed",
    );
  }
  return result.stdout
    .split(/\r?\n/u)
    .map((line) => text(line).slice(3).trim())
    .filter(Boolean)
    .flatMap((filePath) =>
      filePath.includes(" -> ")
        ? filePath.split(" -> ").map(text).filter(Boolean)
        : [filePath],
    );
}

function readTextIfExists(filePath) {
  return fs.existsSync(filePath) ? fs.readFileSync(filePath, "utf8") : "";
}

function sourceConstantVersion(filePath, constantName) {
  const source = readTextIfExists(filePath);
  const escapedName = constantName.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
  return (
    source.match(new RegExp(`${escapedName}\\s*=\\s*"([^"]+)"`, "u"))?.[1] || ""
  );
}

function pluginVersionAlignmentGaps(expectedVersion) {
  if (!expectedVersion) return ["plugin manifest version is missing"];
  const contracts = [
    [path.join(pluginRoot, "mcp", "server.mjs"), "SERVER_VERSION"],
    [
      path.join(pluginRoot, "mcp", "tool-call-runtime.mjs"),
      "TOOL_CALL_RUNTIME_VERSION",
    ],
    [
      path.join(pluginRoot, "mcp", "mcp-request-handler-runtime.mjs"),
      "MCP_REQUEST_HANDLER_RUNTIME_VERSION",
    ],
    [
      path.join(pluginRoot, "mcp", "mcp-tool-result-runtime.mjs"),
      "MCP_TOOL_RESULT_RUNTIME_VERSION",
    ],
    [
      path.join(pluginRoot, "mcp", "duckdb-workbench-runtime.mjs"),
      "DUCKDB_WORKBENCH_RUNTIME_VERSION",
    ],
    [
      path.join(pluginRoot, "scripts", "eval-coverage-contract.mjs"),
      "EVAL_COVERAGE_CONTRACT_VERSION",
    ],
  ];
  const gaps = contracts.flatMap(([filePath, constantName]) => {
    const observed = sourceConstantVersion(filePath, constantName);
    return observed === expectedVersion
      ? []
      : [
          `${path.relative(pluginRoot, filePath)} ${constantName}=${observed || "missing"}`,
        ];
  });
  const cacheVersion = sourceConstantVersion(
    path.join(pluginRoot, "scripts", "runtime-cache-contract.mjs"),
    "RUNTIME_CACHE_CONTRACT_VERSION",
  );
  if (cacheVersion !== `${expectedVersion}-runtime-cache-contract`) {
    gaps.push(
      `scripts/runtime-cache-contract.mjs RUNTIME_CACHE_CONTRACT_VERSION=${cacheVersion || "missing"}`,
    );
  }
  for (const relativePath of [
    "references/capability-registry.json",
    "skills/analytix-fund-analysis/references/capability-registry.json",
  ]) {
    const observed = text(
      readJsonIfExists(path.join(pluginRoot, relativePath))?.plugin?.version,
    );
    if (observed !== expectedVersion)
      gaps.push(`${relativePath} plugin.version=${observed || "missing"}`);
  }
  const probeVersion = sourceConstantVersion(
    path.join(pluginRoot, "scripts", "source-live-probe-contract.mjs"),
    "SOURCE_LIVE_PROBE_CONTRACT_VERSION",
  );
  if (probeVersion !== expectedVersion) {
    gaps.push(
      `scripts/source-live-probe-contract.mjs SOURCE_LIVE_PROBE_CONTRACT_VERSION=${probeVersion || "missing"}`,
    );
  }
  const managerSource = readTextIfExists(
    path.join(
      repoRoot,
      "packages",
      "runtime-go",
      "internal",
      "mcp",
      "manager.go",
    ),
  );
  const hostVersion =
    managerSource.match(/fundsEvidenceKernelVersion\s*=\s*"([^"]+)"/u)?.[1] ||
    "";
  if (hostVersion !== expectedVersion)
    gaps.push(`Go host fundsEvidenceKernelVersion=${hostVersion || "missing"}`);
  if (
    !managerSource.includes('spec.ReadOnlyToolNames["count_case_rows"] = true')
  )
    gaps.push("Go host exact count_case_rows read-only allowlist is missing");
  return gaps;
}

function readJsonIfExists(filePath) {
  try {
    return JSON.parse(readTextIfExists(filePath));
  } catch (_) {
    return null;
  }
}

function findPatternViolations({ relativePaths, patterns }) {
  return relativePaths.flatMap((relativePath) => {
    const filePath = path.join(pluginRoot, relativePath);
    const source = readTextIfExists(filePath);
    if (!source) return [];
    return source
      .split(/\r?\n/u)
      .flatMap((line, index) =>
        patterns.some((pattern) => pattern.test(line))
          ? [`${relativePath}:${index + 1}`]
          : [],
      );
  });
}

function sameFileText(leftPath, rightPath) {
  if (!fs.existsSync(leftPath) || !fs.existsSync(rightPath)) return false;
  return (
    fs.readFileSync(leftPath, "utf8") === fs.readFileSync(rightPath, "utf8")
  );
}

function defaultAnalytixRuntimeHome() {
  return path.join(os.homedir(), ".analytix");
}

function legacyInstalledPluginRoots(runtimeHome, expectedVersion) {
  const roots = [];
  const marketplacePluginRoot = path.join(
    runtimeHome,
    ".cache",
    `${MARKETPLACE_NAME}-plugins`,
    "marketplaces",
    MARKETPLACE_NAME,
    "plugins",
    PLUGIN_NAME,
  );
  if (fs.existsSync(marketplacePluginRoot)) {
    for (const entry of fs.readdirSync(marketplacePluginRoot, {
      withFileTypes: true,
    })) {
      if (!entry.isDirectory()) continue;
      if (
        entry.name === expectedVersion ||
        entry.name.startsWith(`${expectedVersion}-`)
      ) {
        roots.push(path.join(marketplacePluginRoot, entry.name));
      }
    }
  }
  return [...new Set(roots.map((item) => path.resolve(item)))];
}

function canonicalHostPluginRoot(runtimeHome, expectedVersion) {
  return path.join(
    runtimeHome,
    "plugins",
    "cache",
    MARKETPLACE_NAME,
    PLUGIN_NAME,
    expectedVersion,
  );
}

function canonicalDirectoryPresent(filePath) {
  try {
    const stat = fs.lstatSync(filePath);
    return stat.isDirectory() && !stat.isSymbolicLink() && fs.realpathSync(filePath) === filePath;
  } catch {
    return false;
  }
}

function runtimeSkillFilesFor(skillName) {
  return skillName === PLUGIN_NAME
    ? RUNTIME_SKILL_FILES
    : ["SKILL.md", "agents/openai.yaml"];
}

function workspaceRuntimeCacheSha256() {
  const hash = crypto.createHash("sha256");
  for (const relativePath of PLUGIN_CACHE_FILES) {
    const filePath = path.join(pluginRoot, relativePath);
    hash.update(`${relativePath}\0`);
    if (fs.existsSync(filePath)) hash.update(fs.readFileSync(filePath));
    hash.update("\0");
  }
  return hash.digest("hex");
}

function checkLocalRuntimeInstallCache(report, expectedVersion) {
  const runtimeHome =
    text(process.env.ANALYTIX_AGENT_RUNTIME_HOME) ||
    defaultAnalytixRuntimeHome();
  report.pass(
    "local runtime cache scope",
    "runtime cache compares production MCP/skill files inside Analytix-owned runtime only; release gate scripts and RELEASE_NOTES.md are local release evidence; sync-runtime-cache is not Hub publish/install path",
  );
  if (!fs.existsSync(runtimeHome)) {
    report.warn(
      "local runtime install cache",
      `runtime not found: ${runtimeHome}`,
    );
    return;
  }
  const installedPluginRoot = path.join(
    runtimeHome,
    "plugins",
    "cache",
    MARKETPLACE_NAME,
    PLUGIN_NAME,
  );
  const installedVersions = fs.existsSync(installedPluginRoot)
    ? fs
        .readdirSync(installedPluginRoot, { withFileTypes: true })
        .filter((entry) => entry.isDirectory())
        .map((entry) => entry.name)
        .sort()
    : [];
  const canonicalHostRoot = canonicalHostPluginRoot(runtimeHome, expectedVersion);
  const canonicalHostPresent = fs.existsSync(canonicalHostRoot);
  const legacyProjectionRoots = legacyInstalledPluginRoots(runtimeHome, expectedVersion);
  const generatedMarketplacePath = path.join(
    runtimeHome,
    ".cache",
    "analytix-hub-plugins",
    "marketplaces",
    "analytix-hub",
    ".agents",
    "plugins",
    "marketplace.json",
  );
  const generatedMarketplace = readJsonIfExists(generatedMarketplacePath);
  const generatedPlugin = Array.isArray(generatedMarketplace?.plugins)
    ? generatedMarketplace.plugins.find(
        (plugin) => text(plugin?.name) === PLUGIN_NAME,
      )
    : null;
  const generatedSourcePath = text(generatedPlugin?.source?.path);
  const generatedSourceSha256 = text(generatedPlugin?.source?.sha256);
  const generatedPluginVersion = text(generatedPlugin?.version);
  const generatedMarketplaceMissing = generatedMarketplace && !generatedPlugin;
  const generatedMarketplaceStale =
    generatedSourcePath &&
    !generatedSourcePath.includes(
      `/analytix-fund-analysis/${expectedVersion}`,
    ) &&
    !generatedSourcePath.includes(
      `/analytix-fund-analysis/${expectedVersion}-`,
    );
  const generatedMarketplaceVersionStale =
    generatedPluginVersion && generatedPluginVersion !== expectedVersion;
  const installPolicyPath = path.join(
    runtimeHome,
    ".cache",
    "analytix-hub-plugins",
    "install-policy.json",
  );
  const installPolicy = readJsonIfExists(installPolicyPath);
  const installPolicyPlugin = Array.isArray(installPolicy?.requiredPlugins)
    ? installPolicy.requiredPlugins.find(
        (plugin) => text(plugin?.pluginName) === PLUGIN_NAME,
      )
    : null;
  const installPolicyVersion = text(installPolicyPlugin?.version);
  const installPolicySha256 = text(installPolicyPlugin?.packageSha256);
  const installPolicyStale =
    installPolicyVersion && installPolicyVersion !== expectedVersion;
  const workspaceSha256 = workspaceRuntimeCacheSha256();
  const canonicalMarker = canonicalHostPresent
    ? readJsonIfExists(path.join(canonicalHostRoot, ".analytix-hub-installed-plugin.json"))
    : null;
  const legacyMarkerIdentities = legacyProjectionRoots.map((root) => ({
    root: path.basename(root),
    ...validateInstalledMarkerIdentity({
      marker: readJsonIfExists(path.join(root, ".analytix-hub-installed-plugin.json")),
      workspaceContentSha256: workspaceSha256,
    }),
  }));
  const pointerDigestFailures = [
    ...validatePointerDigest(generatedSourceSha256, "marketplace source"),
    ...validatePointerDigest(installPolicySha256, "install-policy package"),
  ];
  const legacyProjectionLabels = [
    generatedMarketplaceMissing ? "generated marketplace missing analytix-fund-analysis" : "",
    generatedMarketplaceStale ? `generated marketplace source=${generatedSourcePath}` : "",
    generatedMarketplaceVersionStale ? `generated marketplace version=${generatedPluginVersion}` : "",
    installPolicyStale ? `install-policy version=${installPolicyVersion}` : "",
    ...pointerDigestFailures,
    ...legacyMarkerIdentities.flatMap((marker) =>
      marker.failures.map((failure) => `${marker.root}:${failure}`),
    ),
    ...legacyProjectionRoots.map((root) => `legacy projection=${path.basename(root)}`),
  ].filter(Boolean);
  const installAuthority = classifyRuntimeInstallAuthority({
    expectedVersion,
    workspaceContentSha256: workspaceSha256,
    canonicalHostPresent,
    canonicalHostCanonical: canonicalDirectoryPresent(canonicalHostRoot),
    canonicalVisibleVersions: installedVersions,
    canonicalMarker,
    legacyProjectionLabels,
  });
  const expectedInstalledManifestMissing = canonicalHostPresent &&
    !fs.existsSync(path.join(canonicalHostRoot, ".codex-plugin", "plugin.json"));
  const skillPath = path.join(
    canonicalHostRoot,
    "skills",
    "analytix-fund-analysis",
    "SKILL.md",
  );
  const skillSource = readTextIfExists(skillPath);
  const staleSkillMarkers = ALL_SKILL_NAMES.flatMap((skillName) => {
    const skillMarker = readJsonIfExists(
      path.join(runtimeHome, "skills", skillName, ".analytix-hub-skill.json"),
    );
    const skillMarkerVersion = text(skillMarker?.version);
    if (!skillMarkerVersion || skillMarkerVersion === expectedVersion)
      return [];
    return [`${skillName}:${skillMarkerVersion}`];
  });
  const missingSemanticToolbox =
    skillSource &&
    (!skillSource.includes("semantic fact toolbox") ||
      !skillSource.includes("funds_investigate") ||
      !skillSource.includes("navigator"));
  const runtimeContentDrift = installAuthority.ok
    ? [canonicalHostRoot].flatMap((installedVersionRoot) =>
        PLUGIN_CACHE_FILES.filter(
          (relativePath) =>
            !sameFileText(
              path.join(pluginRoot, relativePath),
              path.join(installedVersionRoot, relativePath),
            ),
        ).map(
          (relativePath) =>
            `${path.basename(installedVersionRoot)}:${relativePath}`,
        ),
      )
    : [];
  const runtimeSkillDrift = installAuthority.ok
    ? ALL_SKILL_NAMES.flatMap((skillName) =>
        runtimeSkillFilesFor(skillName)
          .filter(
            (relativePath) =>
              !sameFileText(
                path.join(pluginRoot, "skills", skillName, relativePath),
                path.join(canonicalHostRoot, "skills", skillName, relativePath),
              ),
          )
          .map((relativePath) => `${skillName}:${relativePath}`),
      )
    : [];
  if (
    !installAuthority.ok ||
    expectedInstalledManifestMissing ||
    missingSemanticToolbox
  ) {
    const details = [
      installAuthority.failures.length
        ? `canonical host materialization invalid: ${installAuthority.failures.join(", ")}`
        : "",
      expectedInstalledManifestMissing
        ? `installed ${expectedVersion} missing .codex-plugin/plugin.json`
        : "",
      missingSemanticToolbox
        ? "top-level required skill missing semantic-toolbox/navigator guidance"
        : "",
    ]
      .filter(Boolean)
      .join("; ");
    report.fail("local runtime install cache", details);
    return;
  }
  const legacyWarnings = [
    ...installAuthority.warnings,
    ...staleSkillMarkers.map((marker) => `top-level skill=${marker}`),
  ];
  if (legacyWarnings.length) {
    report.warn(
      "legacy runtime projection",
      `${legacyWarnings.join("; ")}; ignored because only the current-run Go host binding may authorize discovery or execution`,
    );
  }
  if (runtimeContentDrift.length || runtimeSkillDrift.length) {
    const details = [
      runtimeContentDrift.length
        ? `cache differs from workspace: ${runtimeContentDrift.slice(0, 8).join(", ")}${runtimeContentDrift.length > 8 ? ", ..." : ""}`
        : "",
      runtimeSkillDrift.length
        ? `runtime required skill differs from workspace: ${runtimeSkillDrift.slice(0, 8).join(", ")}${runtimeSkillDrift.length > 8 ? ", ..." : ""}`
        : "",
    ]
      .filter(Boolean)
      .join("; ");
    report.warn(
      "local runtime install cache drift",
      `${details}; the packaged Go host binding remains authoritative and must be revalidated on every run`,
    );
  } else {
    report.pass(
      "local runtime install cache drift",
      "workspace files match installed runtime cache",
    );
  }
  report.pass(
    "local runtime install cache",
    `canonical-host=${expectedVersion} packageAuthority=${installAuthority.packageAuthorityFileSha256} sourceTree=${installAuthority.sourceTreeSha256} fact-tools=disabled; current-run authority is verified by the Go/Electron host, not by doctor`,
  );
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const report = new DoctorReport();

  for (const relativePath of REQUIRED_FILES) {
    const filePath = path.join(pluginRoot, relativePath);
    if (fs.existsSync(filePath)) {
      report.pass(`file ${relativePath}`);
    } else {
      report.fail(`file ${relativePath}`, "missing");
    }
  }

  const pluginManifestPath = path.join(
    pluginRoot,
    ".codex-plugin",
    "plugin.json",
  );
  const mcpManifestPath = path.join(pluginRoot, ".mcp.json");
  const serverPath = path.join(pluginRoot, "mcp", "server.mjs");
  const requestHandlerPath = path.join(
    pluginRoot,
    "mcp",
    "mcp-request-handler-runtime.mjs",
  );
  const toolSchemasPath = path.join(pluginRoot, "mcp", "tool-schemas.mjs");
  const skillPath = path.join(
    pluginRoot,
    "skills",
    "analytix-fund-analysis",
    "SKILL.md",
  );
  const metadataPath = path.join(
    pluginRoot,
    "references",
    "command-metadata.json",
  );
  const capabilityRegistryPath = path.join(
    pluginRoot,
    "references",
    "capability-registry.json",
  );
  const capabilityRegistrySchemaPath = path.join(
    pluginRoot,
    "references",
    "capability-registry.schema.json",
  );
  const evalCoverageContractPath = path.join(
    pluginRoot,
    "scripts",
    "eval-fixtures",
    "eval-coverage-contract.json",
  );
  const routerPath = path.join(pluginRoot, "references", "command-router.md");
  const releaseNotesPath = path.join(pluginRoot, "RELEASE_NOTES.md");
  const embeddedReferenceDir = path.join(
    pluginRoot,
    "skills",
    "analytix-fund-analysis",
    "references",
  );
  const rootReferenceDir = path.join(pluginRoot, "references");
  let pluginManifest = {};
  let mcpManifest = {};
  let serverSource = "";
  let requestHandlerSource = "";
  let toolSchemasSource = "";
  let skillSource = "";
  let routerSource = "";
  let releaseNotesSource = "";
  let commandMetadata = {};
  let capabilityRegistry = {};
  let capabilityRegistrySchema = {};

  try {
    pluginManifest = readJson(pluginManifestPath);
    mcpManifest = readJson(mcpManifestPath);
    commandMetadata = readJson(metadataPath);
    capabilityRegistry = readJson(capabilityRegistryPath);
    capabilityRegistrySchema = readJson(capabilityRegistrySchemaPath);
    serverSource = fs.readFileSync(serverPath, "utf8");
    requestHandlerSource = fs.existsSync(requestHandlerPath)
      ? fs.readFileSync(requestHandlerPath, "utf8")
      : "";
    toolSchemasSource = fs.existsSync(toolSchemasPath)
      ? fs.readFileSync(toolSchemasPath, "utf8")
      : "";
    skillSource = fs.readFileSync(skillPath, "utf8");
    routerSource = fs.readFileSync(routerPath, "utf8");
    releaseNotesSource = fs.readFileSync(releaseNotesPath, "utf8");
    report.pass(
      "json parse",
      "plugin manifest, MCP manifest, command metadata, capability registry",
    );
  } catch (error) {
    report.fail(
      "json parse",
      error instanceof Error ? error.message : String(error),
    );
  }
  const fundsMcpConfig = objectOf(objectOf(mcpManifest.mcpServers).analytix_funds);
  const p0SourceQuarantined = fundsMcpConfig.disabled === true;

  const manifestIngestionErrors = validatePluginManifestIngestionContract(
    pluginManifest,
    mcpManifest,
  );
  if (manifestIngestionErrors.length) {
    report.fail(
      "plugin manifest ingestion contract",
      manifestIngestionErrors.join("; "),
    );
  } else {
    report.pass(
      "plugin manifest ingestion contract",
      "plugin.json matches accepted ingestion shape with skills, MCP server, interface metadata, and assets",
    );
  }

  const focusedSkillFailures = [];
  const focusedSkillWarnings = [];
  const topAgent = readTextIfExists(
    path.join(pluginRoot, "agents", "openai.yaml"),
  );
  if (!topAgent) {
    focusedSkillFailures.push("top-level agents/openai.yaml missing");
  } else {
    const missingTopAgentMarkers = REQUIRED_AGENT_ROUTING_MARKERS.filter(
      (marker) => !topAgent.includes(marker),
    );
    if (missingTopAgentMarkers.length) {
      focusedSkillFailures.push(
        `top-level agents/openai.yaml missing routing markers: ${missingTopAgentMarkers.join(", ")}`,
      );
    }
  }
  for (const skillName of FOCUSED_SKILL_NAMES) {
    const skillFile = path.join(pluginRoot, "skills", skillName, "SKILL.md");
    const agentFile = path.join(
      pluginRoot,
      "skills",
      skillName,
      "agents",
      "openai.yaml",
    );
    const body = readTextIfExists(skillFile);
    const agent = readTextIfExists(agentFile);
    if (!body) {
      focusedSkillFailures.push(`${skillName}: missing SKILL.md`);
      continue;
    }
    const frontmatterIssues = skillFrontmatterIssues(body);
    if (frontmatterIssues.length)
      focusedSkillFailures.push(
        `${skillName}: ${frontmatterIssues.join(", ")}`,
      );
    if (!agent)
      focusedSkillFailures.push(`${skillName}: missing agents/openai.yaml`);
    if (!body.includes(`name: ${skillName}`))
      focusedSkillFailures.push(`${skillName}: frontmatter name mismatch`);
    if (!/Use (when|for)/u.test(body) && !/description: Use/u.test(body))
      focusedSkillWarnings.push(`${skillName}: weak Use when wording`);
    if (!/Not for/u.test(body) && !/不适用/u.test(body))
      focusedSkillWarnings.push(`${skillName}: missing non-goal wording`);
    if (
      !body.includes(
        "../analytix-fund-analysis/references/focused-skill-shared.md",
      )
    )
      focusedSkillFailures.push(
        `${skillName}: missing shared focused-skill reference`,
      );
    if (
      /top-pluginization-plan|blueprint-execution-plan|golden|oracle/u.test(
        body,
      )
    )
      focusedSkillFailures.push(
        `${skillName}: should not copy blueprints or eval fixtures`,
      );
    const lineCount = body.split(/\r?\n/u).length;
    if (lineCount > 90)
      focusedSkillWarnings.push(`${skillName}: ${lineCount} lines`);
  }
  const registryFocusedSkills = new Set(
    Array.isArray(capabilityRegistry.focused_skills)
      ? capabilityRegistry.focused_skills
          .map((item) => text(item?.id))
          .filter(Boolean)
      : [],
  );
  const missingRegistrySkills = FOCUSED_SKILL_NAMES.filter(
    (skillName) => !registryFocusedSkills.has(skillName),
  );
  if (missingRegistrySkills.length) {
    focusedSkillFailures.push(
      `registry missing focused skills: ${missingRegistrySkills.join(", ")}`,
    );
  }
  const manifestText = JSON.stringify(pluginManifest.interface || {});
  const manifestSurfaceMarkers = {
    "account-dossier": [
      "account-dossier",
      "account dossier",
      "某卡",
      "某账户",
      "账户画像",
      "账户与主体画像",
    ],
    "subject-dossier": [
      "subject-dossier",
      "subject dossier",
      "某人",
      "某公司",
      "主体画像",
    ],
    "full-case-analysis": [
      "full-case-analysis",
      "full case analysis",
      "全案分析",
    ],
    "report-builder": ["report-builder", "report builder", "报告生成"],
    "evidence-request": [
      "evidence-request",
      "evidence request",
      "补调",
      "取证",
    ],
    "analysis-critique": [
      "analysis-critique",
      "critique",
      "missing",
      "next focused skill",
      "研判完整性复核",
    ],
    "claim-review": ["claim-review", "claim review", "claim 复核", "报告复核"],
  };
  for (const [marker, aliases] of Object.entries(manifestSurfaceMarkers)) {
    if (!aliases.some((alias) => manifestText.includes(alias))) {
      focusedSkillWarnings.push(
        `manifest interface does not visibly mention ${marker}`,
      );
    }
  }
  if (focusedSkillFailures.length) {
    report.fail(
      "focused skill product surface",
      focusedSkillFailures.join("; "),
    );
  } else {
    report.pass(
      "focused skill product surface",
      `${FOCUSED_SKILL_NAMES.length} focused skills with short entries, agents/openai.yaml, shared reference, and registry mapping`,
    );
  }
  if (focusedSkillWarnings.length) {
    report.warn(
      "focused skill product surface polish",
      focusedSkillWarnings.join("; "),
    );
  }

  const syntax = spawnSync(process.execPath, ["--check", serverPath], {
    cwd: pluginRoot,
    encoding: "utf8",
  });
  if (syntax.status === 0) {
    report.pass("mcp server syntax", "node --check passed");
  } else {
    report.fail(
      "mcp server syntax",
      text(syntax.stderr || syntax.stdout).slice(0, 500),
    );
  }
  const releaseScriptChecks = [
    "scripts/agent-thread-audit.mjs",
    "scripts/check-health.mjs",
    "scripts/desktop-multiturn-acceptance.mjs",
    "scripts/release-slice-audit.mjs",
    "scripts/mcp-surface-snapshot.mjs",
    "scripts/prepare-hub-package.mjs",
    "scripts/phase4-diagnostics.mjs",
    "scripts/functional-eval.mjs",
    "scripts/blueprint-capability-audit.mjs",
    "scripts/investigation-scenario-audit.mjs",
    "scripts/mcp-return-oracle.mjs",
    "scripts/skill-creator-alignment-audit.mjs",
    "scripts/skill-clause-audit.mjs",
    "scripts/pair-amount-contract-smoke.mjs",
    "scripts/p0-contract-suite.mjs",
    "scripts/funds-producer-content-v1-contract.mjs",
    "scripts/dataset-snapshot-manifest-v2-contract.mjs",
    "scripts/tool-schema-contract.mjs",
    "scripts/runtime-install-identity-contract.mjs",
  ].map((relativePath) => {
    const result = spawnSync(
      process.execPath,
      ["--check", path.join(pluginRoot, relativePath)],
      {
        cwd: pluginRoot,
        encoding: "utf8",
      },
    );
    return {
      relativePath,
      ok: result.status === 0,
      detail: text(result.stderr || result.stdout).slice(0, 500),
    };
  });
  const failedReleaseScriptChecks = releaseScriptChecks.filter(
    (item) => !item.ok,
  );
  if (failedReleaseScriptChecks.length) {
    report.fail(
      "release gate script syntax",
      failedReleaseScriptChecks
        .map((item) => `${item.relativePath}: ${item.detail}`)
        .join("; "),
    );
  } else {
    report.pass(
      "release gate script syntax",
      `${releaseScriptChecks.length} scripts passed node --check`,
    );
  }

  const serverTools = extractServerTools(
    `${serverSource}\n${toolSchemasSource}`,
  );
  const missingCriticalTools = CRITICAL_TOOLS.filter(
    (tool) => !serverTools.has(tool),
  );
  if (missingCriticalTools.length) {
    report.fail(
      "critical tool surface",
      `missing ${missingCriticalTools.join(", ")}`,
    );
  } else {
    report.pass(
      "critical tool surface",
      p0SourceQuarantined
        ? `${CRITICAL_TOOLS.length} dormant critical schemas remain available for offline audit; production exposed=0`
        : `${CRITICAL_TOOLS.length} critical tools present`,
    );
  }

  const toolDiscoverySourcePath = path.join(
    pluginRoot,
    "mcp",
    "tool-discovery-policy.mjs",
  );
  const toolDiscoverySource = fs.existsSync(toolDiscoverySourcePath)
    ? fs.readFileSync(toolDiscoverySourcePath, "utf8")
    : "";
  const capabilityRegistryRuntimePath = path.join(
    pluginRoot,
    "mcp",
    "capability-registry-runtime.mjs",
  );
  const capabilityRegistryRuntimeSource = fs.existsSync(
    capabilityRegistryRuntimePath,
  )
    ? fs.readFileSync(capabilityRegistryRuntimePath, "utf8")
    : "";
  const toolDiscoveryContract = objectOf(
    capabilityRegistry.tool_discovery_contract,
  );
  const registryPriorityTools = arrayOf(toolDiscoveryContract.priority_tools)
    .map(text)
    .filter(Boolean);
  const registryDefaultTools = arrayOf(
    toolDiscoveryContract.default_visible_tools,
  )
    .map(text)
    .filter(Boolean);
  const registryReportOnlyTools = arrayOf(
    toolDiscoveryContract.frontdoor_report_only_tools,
  )
    .map(text)
    .filter(Boolean);
  const registryFullEnvVars = arrayOf(
    toolDiscoveryContract.full_discovery_env_vars,
  )
    .map(text)
    .filter(Boolean);
  const maxDefaultVisibleTools = Number(
    toolDiscoveryContract.max_default_visible_tools || 0,
  );
  const missingSemanticPriorityTools = DEFAULT_SEMANTIC_DISCOVERY_TOOLS.filter(
    (tool) => !registryPriorityTools.includes(tool),
  );
  const hostCaptureAdvertisedToolsMatch = requestHandlerSource.match(
    /const advertisedTools = \[(?<body>[\s\S]*?)\];\s*const advertisedByName =/u,
  );
  const hostCaptureAdvertisedToolsBody =
    hostCaptureAdvertisedToolsMatch?.groups?.body || "";
  const hostCaptureAdvertisedToolNames = [
    ...hostCaptureAdvertisedToolsBody.matchAll(
      /^\s*name:\s*([^,\n]+),?\s*$/gmu,
    ),
  ].map((match) => match[1]);
  const hostCaptureTaskSupportForbiddenCount = (
    hostCaptureAdvertisedToolsBody.match(
      /execution:\s*\{\s*taskSupport:\s*"forbidden"\s*\}/gu,
    ) || []
  ).length;
  const hostCaptureSchemaBindings = [
    "inputSchema: FUNDS_COUNT_TOOL_INPUT_SCHEMA_V2",
    "outputSchema: FUNDS_COUNT_TOOL_OUTCOME_SCHEMA_V2",
    "inputSchema: FUNDS_ACCOUNT_FLOW_TOOL_INPUT_SCHEMA_V1",
    "outputSchema: FUNDS_ACCOUNT_FLOW_HOST_CAPTURE_OUTPUT_SCHEMA_V1",
  ];
  const fixedHostCaptureProtocol =
    JSON.stringify(hostCaptureAdvertisedToolNames) ===
      JSON.stringify(["\"count_case_rows\"", "FUNDS_ACCOUNT_FLOW_TOOL_NAME_V1"]) &&
    requestHandlerSource.includes(
      'const FUNDS_ACCOUNT_FLOW_TOOL_NAME_V1 = "analyze_account_flows";',
    ) &&
    hostCaptureTaskSupportForbiddenCount === 2 &&
    hostCaptureSchemaBindings.every((binding) =>
      hostCaptureAdvertisedToolsBody.includes(binding),
    );
  const executionAllowlistBound = p0SourceQuarantined
    ? fixedHostCaptureProtocol &&
      requestHandlerSource.includes("new Map(advertisedTools.map") &&
      requestHandlerSource.includes("return { tools: advertisedTools };") &&
      requestHandlerSource.includes("const tool = advertisedByName.get(name);") &&
      requestHandlerSource.includes("Unknown or unadvertised tool")
    : requestHandlerSource.includes("orderedToolsForAgent(tools, env)");
  if (
    !registryPriorityTools.length ||
    missingSemanticPriorityTools.length ||
    !toolDiscoverySource.includes('from "./capability-registry-runtime.mjs"') ||
    !toolDiscoverySource.includes("TOOL_LIST_PRIORITY") ||
    !capabilityRegistryRuntimeSource.includes("readCapabilityRegistry") ||
    !capabilityRegistryRuntimeSource.includes("capability-registry.json") ||
    /TOOL_LIST_PRIORITY\s*=\s*\[/u.test(toolDiscoverySource) ||
    !executionAllowlistBound
  ) {
    report.fail(
      "tool list priority",
      [
        !registryPriorityTools.length
          ? "missing capability registry tool_discovery_contract.priority_tools"
          : "",
        missingSemanticPriorityTools.length
          ? `missing semantic priority tools: ${missingSemanticPriorityTools.join(", ")}`
          : "",
        !toolDiscoverySource.includes(
          'from "./capability-registry-runtime.mjs"',
        )
          ? "tool discovery policy does not import capability registry runtime"
          : "",
        !toolDiscoverySource.includes("TOOL_LIST_PRIORITY")
          ? "tool discovery policy does not consume TOOL_LIST_PRIORITY"
          : "",
        !capabilityRegistryRuntimeSource.includes("readCapabilityRegistry")
          ? "missing capability registry runtime reader"
          : "",
        !capabilityRegistryRuntimeSource.includes("capability-registry.json")
          ? "capability registry runtime does not read registry JSON"
          : "",
        /TOOL_LIST_PRIORITY\s*=\s*\[/u.test(toolDiscoverySource)
          ? "tool discovery policy still owns hardcoded priority list"
          : "",
        !executionAllowlistBound
          ? p0SourceQuarantined
            ? "disabled MCP must expose exactly the two fixed host-capture schemas (both taskSupport=forbidden) through the shared tools/list/tools/call allowlist"
            : "tools/list must derive its allowlist from orderedToolsForAgent(tools, env)"
          : "",
      ]
        .filter(Boolean)
        .join("; "),
    );
  } else {
    report.pass(
      "tool list priority",
      p0SourceQuarantined
        ? "disabled MCP keeps exactly two fixed host-capture schemas with taskSupport=forbidden on the shared tools/list/tools/call allowlist; dormant priority schemas remain registry-backed"
        : `${registryPriorityTools.length} priority tools are registry-backed`,
    );
  }

  if (
    !registryDefaultTools.length ||
    !registryReportOnlyTools.length ||
    !registryFullEnvVars.length ||
    !maxDefaultVisibleTools ||
    registryDefaultTools.length > maxDefaultVisibleTools ||
    DEFAULT_SEMANTIC_DISCOVERY_TOOLS.some(
      (tool) => !registryDefaultTools.includes(tool),
    ) ||
    !toolDiscoverySource.includes("DEFAULT_VISIBLE_TOOL_NAMES") ||
    !toolDiscoverySource.includes("FRONTDOOR_REPORT_ONLY_TOOL_NAMES") ||
    !toolDiscoverySource.includes("FULL_DISCOVERY_ENV_VARS") ||
    !toolDiscoverySource.includes(
      "AGENT_DISCOVERY_TOOL_NAMES = new Set(DEFAULT_VISIBLE_TOOL_NAMES)",
    ) ||
    !toolDiscoverySource.includes("frontdoor_report_only") ||
    !toolDiscoverySource.includes(
      "frontdoorReportOnlyToolNames.has(tool.name)",
    ) ||
    toolDiscoverySource.includes(".slice(0, MAX_DEFAULT_VISIBLE_TOOLS)") ||
    /AGENT_DISCOVERY_TOOL_NAMES\s*=\s*new Set\(\[/u.test(toolDiscoverySource)
  ) {
    report.fail(
      "default tool discovery",
      [
        !registryDefaultTools.length
          ? "missing capability registry tool_discovery_contract.default_visible_tools"
          : "",
        !registryReportOnlyTools.length
          ? "missing capability registry tool_discovery_contract.frontdoor_report_only_tools"
          : "",
        !registryFullEnvVars.length
          ? "missing capability registry tool_discovery_contract.full_discovery_env_vars"
          : "",
        !maxDefaultVisibleTools
          ? "missing capability registry tool_discovery_contract.max_default_visible_tools"
          : "",
        registryDefaultTools.length > maxDefaultVisibleTools
          ? `too many default-visible tools: ${registryDefaultTools.length} > ${maxDefaultVisibleTools}`
          : "",
        DEFAULT_SEMANTIC_DISCOVERY_TOOLS.some(
          (tool) => !registryDefaultTools.includes(tool),
        )
          ? `missing semantic default tools: ${DEFAULT_SEMANTIC_DISCOVERY_TOOLS.filter((tool) => !registryDefaultTools.includes(tool)).join(", ")}`
          : "",
        !toolDiscoverySource.includes("DEFAULT_VISIBLE_TOOL_NAMES")
          ? "tool discovery policy does not consume default-visible registry list"
          : "",
        !toolDiscoverySource.includes("FRONTDOOR_REPORT_ONLY_TOOL_NAMES")
          ? "tool discovery policy does not consume report-only registry list"
          : "",
        !toolDiscoverySource.includes("FULL_DISCOVERY_ENV_VARS")
          ? "tool discovery policy does not consume full-discovery env vars"
          : "",
        !toolDiscoverySource.includes(
          "AGENT_DISCOVERY_TOOL_NAMES = new Set(DEFAULT_VISIBLE_TOOL_NAMES)",
        )
          ? "AGENT_DISCOVERY_TOOL_NAMES is not registry-backed"
          : "",
        !toolDiscoverySource.includes("frontdoor_report_only")
          ? "missing frontdoor report-only profile"
          : "",
        !toolDiscoverySource.includes(
          "frontdoorReportOnlyToolNames.has(tool.name)",
        )
          ? "report-only profile is not registry-backed"
          : "",
        toolDiscoverySource.includes(".slice(0, MAX_DEFAULT_VISIBLE_TOOLS)")
          ? "default discovery still truncates semantic toolbox"
          : "",
        /AGENT_DISCOVERY_TOOL_NAMES\s*=\s*new Set\(\[/u.test(
          toolDiscoverySource,
        )
          ? "tool discovery policy still owns hardcoded default discovery set"
          : "",
      ]
        .filter(Boolean)
        .join("; "),
    );
  } else {
    report.pass(
      "default tool discovery",
      p0SourceQuarantined
        ? `production advertises zero tools; ${registryDefaultTools.length} registry-backed schemas remain dormant and cannot be restored by discovery flags`
        : `${registryDefaultTools.length} semantic default-visible tools are registry-backed without slice truncation; full mode available`,
    );
  }

  const schemaByToolName = new Map(
    MCP_TOOL_SCHEMAS.map((tool) => [text(tool.name), tool.inputSchema || {}]),
  );
  const caseIdRequiredDefaultTools = DEFAULT_SEMANTIC_DISCOVERY_TOOLS.filter(
    (toolName) =>
      arrayOf(schemaByToolName.get(toolName)?.required)
        .map(text)
        .includes("case_id"),
  );
  const missingDefaultToolSchemas = DEFAULT_SEMANTIC_DISCOVERY_TOOLS.filter(
    (toolName) => !schemaByToolName.has(toolName),
  );
  if (caseIdRequiredDefaultTools.length || missingDefaultToolSchemas.length) {
    report.fail(
      "host-bound optional case schemas",
      [
        caseIdRequiredDefaultTools.length
          ? `default tools require explicit case_id instead of accepting the frozen host context: ${caseIdRequiredDefaultTools.join(", ")}`
          : "",
        missingDefaultToolSchemas.length
          ? `default tools missing schemas: ${missingDefaultToolSchemas.join(", ")}`
          : "",
      ]
        .filter(Boolean)
        .join("; "),
    );
  } else {
    report.pass(
      "host-bound optional case schemas",
      p0SourceQuarantined
        ? `${DEFAULT_SEMANTIC_DISCOVERY_TOOLS.length} dormant semantic schemas remain host-bound; none is advertised while source authority is unavailable`
        : `${DEFAULT_SEMANTIC_DISCOVERY_TOOLS.length} default semantic tools may omit case_id only because runtime authority supplies the frozen case context; backend active-case fallback is not accepted`,
    );
  }

  const expectedNonReadOnlyTools = new Set([
    "export_cleaned_case_data",
    "create_case_notebook",
    "run_full_case_analysis",
  ]);
  const toolAnnotationFailures = [];
  for (const tool of MCP_TOOL_SCHEMAS) {
    const name = text(tool.name);
    if (!tool.title || text(tool.annotations?.title) !== text(tool.title)) {
      toolAnnotationFailures.push(`${name}: missing visible title/annotation`);
    }
    if (
      tool.inputSchema?.type !== "object" ||
      tool.inputSchema?.additionalProperties !== false
    ) {
      toolAnnotationFailures.push(
        `${name}: root inputSchema must be closed object`,
      );
    }
    if (tool.annotations?.destructiveHint !== false) {
      toolAnnotationFailures.push(`${name}: destructiveHint must be false`);
    }
    if (tool.annotations?.openWorldHint !== false) {
      toolAnnotationFailures.push(`${name}: openWorldHint must be false`);
    }
    if (expectedNonReadOnlyTools.has(name)) {
      if (tool.annotations?.readOnlyHint !== false) {
        toolAnnotationFailures.push(
          `${name}: artifact-writing tool must set readOnlyHint=false`,
        );
      }
    } else if (tool.annotations?.readOnlyHint !== true) {
      toolAnnotationFailures.push(
        `${name}: fact/diagnostic tool must set readOnlyHint=true`,
      );
    }
  }
  if (toolAnnotationFailures.length) {
    report.fail("mcp tool annotations", toolAnnotationFailures.join("; "));
  } else {
    report.pass(
      "mcp tool annotations",
      `${MCP_TOOL_SCHEMAS.length} tools have closed schemas, public titles, and accurate read/write hints`,
    );
  }

  const resourceSourcePath = path.join(
    pluginRoot,
    "mcp",
    "progressive-resources.mjs",
  );
  const resourceSource = fs.existsSync(resourceSourcePath)
    ? fs.readFileSync(resourceSourcePath, "utf8")
    : "";
  const resourceContractSource = `${serverSource}\n${requestHandlerSource}\n${resourceSource}`;
  const missingResourceHandlers = [
    "resources/list",
    "resources/read",
    "resources/templates/list",
    "resourceDefinitions",
    "resources: {}",
  ].filter((needle) => !resourceContractSource.includes(needle));
  const missingResourceUris = REQUIRED_RESOURCE_URIS.filter(
    (uri) => !resourceContractSource.includes(uri),
  );
  const evalResourceUris = [
    "skill://analytix-fund-analysis/references/golden-eval-rubric.md",
  ].filter((uri) => resourceSource.includes(uri));
  const developmentResourceUris = DEVELOPMENT_ONLY_RESOURCE_URIS.filter((uri) =>
    resourceSource.includes(uri),
  );
  const p0Resources = fixedFundsBoundaryResources();
  const p0ResourceContractValid =
    p0Resources.length === 1 &&
    text(p0Resources[0]?.uri) === FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI &&
    requestHandlerSource.includes("fixedFundsBoundaryResources()") &&
    requestHandlerSource.includes("fixedFundsBoundaryResourceRead(") &&
    requestHandlerSource.includes("Unknown P0 boundary resource");
  if (p0SourceQuarantined && !p0ResourceContractValid) {
    report.fail(
      "mcp resources",
      "P0 production surface must expose exactly the fixed source-unavailable resource and reject every other URI",
    );
  } else if (p0SourceQuarantined) {
    report.pass(
      "mcp resources",
      "production exposes exactly one fixed source-unavailable resource; dormant skill resources are not readable",
    );
  } else if (
    missingResourceHandlers.length ||
    missingResourceUris.length ||
    evalResourceUris.length ||
    developmentResourceUris.length
  ) {
    report.fail(
      "mcp resources",
      [
        missingResourceHandlers.length
          ? `missing handlers: ${missingResourceHandlers.join(", ")}`
          : "",
        missingResourceUris.length
          ? `missing uris: ${missingResourceUris.join(", ")}`
          : "",
        evalResourceUris.length
          ? `eval-only resources exposed: ${evalResourceUris.join(", ")}`
          : "",
        developmentResourceUris.length
          ? `development-only resources exposed: ${developmentResourceUris.join(", ")}`
          : "",
      ]
        .filter(Boolean)
        .join("; "),
    );
  } else {
    report.pass(
      "mcp resources",
      `${REQUIRED_RESOURCE_URIS.length} progressive-disclosure resources exposed`,
    );
  }

  const promptSourcePath = path.join(
    pluginRoot,
    "mcp",
    "progressive-prompts.mjs",
  );
  const promptSource = fs.existsSync(promptSourcePath)
    ? fs.readFileSync(promptSourcePath, "utf8")
    : "";
  const promptContractSource = `${serverSource}\n${requestHandlerSource}\n${promptSource}`;
  const missingPromptHandlers = [
    "prompts/list",
    "prompts/get",
    "promptListForAgent",
    "getPromptForAgent",
    "prompts: {}",
  ].filter((needle) => !promptContractSource.includes(needle));
  const missingPromptNames = REQUIRED_PROMPT_NAMES.filter(
    (name) => !promptContractSource.includes(name),
  );
  const forbiddenPromptMarkers = [
    "golden-eval-rubric",
    "eval-fixtures",
    "golden-answer-set",
    "model-ab-eval",
  ].filter((marker) => promptSource.includes(marker));
  const p0Prompts = fixedFundsBoundaryPrompts();
  const p0PromptContractValid =
    p0Prompts.length === 1 &&
    text(p0Prompts[0]?.name) === FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME &&
    arrayOf(p0Prompts[0]?.arguments).length === 0 &&
    requestHandlerSource.includes("fixedFundsBoundaryPrompts()") &&
    requestHandlerSource.includes("fixedFundsBoundaryPrompt(") &&
    requestHandlerSource.includes("P0 boundary prompt does not accept arguments");
  if (p0SourceQuarantined && !p0PromptContractValid) {
    report.fail(
      "mcp prompts",
      "P0 production surface must expose exactly one argument-free source-unavailable prompt",
    );
  } else if (p0SourceQuarantined) {
    report.pass(
      "mcp prompts",
      "production exposes exactly one argument-free fixed boundary prompt; dormant workflow prompts are not exposed",
    );
  } else if (
    missingPromptHandlers.length ||
    missingPromptNames.length ||
    forbiddenPromptMarkers.length
  ) {
    report.fail(
      "mcp prompts",
      [
        missingPromptHandlers.length
          ? `missing handlers: ${missingPromptHandlers.join(", ")}`
          : "",
        missingPromptNames.length
          ? `missing prompts: ${missingPromptNames.join(", ")}`
          : "",
        forbiddenPromptMarkers.length
          ? `eval-only markers exposed: ${forbiddenPromptMarkers.join(", ")}`
          : "",
      ]
        .filter(Boolean)
        .join("; "),
    );
  } else {
    report.pass(
      "mcp prompts",
      `${REQUIRED_PROMPT_NAMES.length} workflow prompts exposed for Agent database/visual/report work`,
    );
  }

  let capabilityRegistryResourceText = "";
  let capabilityRegistryResourceRejected = false;
  try {
    const registryResource = readResourceForAgent(
      pluginRoot,
      "skill://analytix-fund-analysis/references/capability-registry.json",
    );
    capabilityRegistryResourceText = text(
      registryResource?.contents?.[0]?.text,
    );
  } catch (error) {
    capabilityRegistryResourceRejected = true;
    if (!p0SourceQuarantined) {
      report.fail(
        "mcp capability registry resource filter",
        error instanceof Error ? error.message : String(error),
      );
    }
  }
  const exposedEvalRegistryKeys = RESOURCE_EVAL_ONLY_KEYS.filter((key) =>
    capabilityRegistryResourceText.includes(`"${key}"`),
  );
  if (p0SourceQuarantined && capabilityRegistryResourceRejected) {
    report.pass(
      "mcp capability registry resource filter",
      "dormant capability registry is not readable from the production MCP surface",
    );
  } else if (p0SourceQuarantined) {
    report.fail(
      "mcp capability registry resource filter",
      "dormant capability registry unexpectedly remained readable during P0 quarantine",
    );
  } else if (exposedEvalRegistryKeys.length) {
    report.fail(
      "mcp capability registry resource filter",
      `production resource exposes eval-only keys: ${exposedEvalRegistryKeys.join(", ")}`,
    );
  } else if (capabilityRegistryResourceText) {
    report.pass(
      "mcp capability registry resource filter",
      "capability registry resource omits eval-only task coverage fields",
    );
  }

  const publicCopyViolations = findPatternViolations({
    relativePaths: PUBLIC_COPY_FILES,
    patterns: PUBLIC_COPY_FORBIDDEN_PATTERNS,
  });
  if (publicCopyViolations.length) {
    report.fail(
      "public product wording",
      `process wording found in ${publicCopyViolations.join(", ")}`,
    );
  } else {
    report.pass(
      "public product wording",
      "manifest, skill, references, and user-visible MCP text avoid process-only wording",
    );
  }

  const commandList = Array.isArray(commandMetadata.commands)
    ? commandMetadata.commands
    : [];
  if (commandList.length >= 10) {
    report.pass("command metadata size", `${commandList.length} commands`);
  } else {
    report.fail("command metadata size", "expected at least 10 commands");
  }

  const metadataTools = new Set(commandList.flatMap(commandToolNames));
  const missingMetadataTools = [...metadataTools].filter(
    (tool) => !serverTools.has(tool),
  );
  if (missingMetadataTools.length) {
    report.fail(
      "command metadata tools",
      `missing from dormant schema inventory: ${missingMetadataTools.join(", ")}`,
    );
  } else {
    report.pass(
      "command metadata tools",
      p0SourceQuarantined
        ? `${metadataTools.size} command references resolve to dormant schemas; production exposed=0`
        : `${metadataTools.size} referenced tools exposed`,
    );
  }

  const routerCommands = extractRouterCommands(routerSource);
  const metadataCommands = commandList
    .map((item) => text(item.command))
    .filter(Boolean);
  const metadataCommandSet = new Set(metadataCommands);
  const routerCommandSet = new Set(routerCommands);
  const missingMetadataCommands = routerCommands.filter(
    (command) => !metadataCommandSet.has(command),
  );
  const extraMetadataCommands = metadataCommands.filter(
    (command) => !routerCommandSet.has(command),
  );
  if (missingMetadataCommands.length || extraMetadataCommands.length) {
    const details = [
      missingMetadataCommands.length
        ? `missing metadata: ${missingMetadataCommands.join(", ")}`
        : "",
      extraMetadataCommands.length
        ? `extra metadata: ${extraMetadataCommands.join(", ")}`
        : "",
    ]
      .filter(Boolean)
      .join("; ");
    report.fail("command metadata coverage", details);
  } else {
    report.pass(
      "command metadata coverage",
      `${metadataCommands.length} commands match command-router`,
    );
  }

  const malformedCommands = commandList.flatMap((command) => {
    const name = text(command.command) || "<missing command>";
    const missing = [];
    if (!text(command.lane)) missing.push("lane");
    if (!Array.isArray(command.primaryTools) || !command.primaryTools.length)
      missing.push("primaryTools");
    if (!Array.isArray(command.requiredGates) || !command.requiredGates.length)
      missing.push("requiredGates");
    if (!text(command.answerContract)) missing.push("answerContract");
    return missing.length ? [`${name} missing ${missing.join("/")}`] : [];
  });
  if (malformedCommands.length) {
    report.fail("command metadata contracts", malformedCommands.join("; "));
  } else {
    report.pass(
      "command metadata contracts",
      "every command has lane, primaryTools, requiredGates, and answerContract",
    );
  }

  const taskTaxonomy = Array.isArray(capabilityRegistry.task_taxonomy)
    ? capabilityRegistry.task_taxonomy.map((item) =>
        item && typeof item === "object" ? item : {},
      )
    : [];
  const taskTaxonomyIds = new Set(
    taskTaxonomy.map((item) => text(item.id)).filter(Boolean),
  );
  const focusedSkillIds = new Set(FOCUSED_SKILL_NAMES);
  const commandFocusedSkillFailures = commandList.flatMap((command) => {
    const name = text(command.command) || "<missing command>";
    const primary = text(command.focusedSkill);
    const secondary = Array.isArray(command.secondaryFocusedSkills)
      ? command.secondaryFocusedSkills.map(text).filter(Boolean)
      : [];
    const failures = [];
    if (!primary) failures.push(`${name} missing focusedSkill`);
    else if (!focusedSkillIds.has(primary))
      failures.push(`${name} unknown focusedSkill ${primary}`);
    if (!text(command.taskType)) failures.push(`${name} missing taskType`);
    else if (!taskTaxonomyIds.has(text(command.taskType)))
      failures.push(`${name} unknown taskType ${text(command.taskType)}`);
    for (const skillName of secondary) {
      if (!focusedSkillIds.has(skillName))
        failures.push(`${name} unknown secondaryFocusedSkill ${skillName}`);
    }
    return failures;
  });
  const commandCoveredFocusedSkills = new Set(
    commandList
      .flatMap((command) => [
        text(command.focusedSkill),
        ...(Array.isArray(command.secondaryFocusedSkills)
          ? command.secondaryFocusedSkills.map(text)
          : []),
      ])
      .filter(Boolean),
  );
  const missingCommandFocusedSkills = FOCUSED_SKILL_NAMES.filter(
    (skillName) => !commandCoveredFocusedSkills.has(skillName),
  );
  if (
    commandFocusedSkillFailures.length ||
    missingCommandFocusedSkills.length
  ) {
    report.fail(
      "command metadata focused skill routing",
      [
        commandFocusedSkillFailures.join("; "),
        missingCommandFocusedSkills.length
          ? `focused skills not covered by commands: ${missingCommandFocusedSkills.join(", ")}`
          : "",
      ]
        .filter(Boolean)
        .join("; "),
    );
  } else {
    report.pass(
      "command metadata focused skill routing",
      `${commandList.length} commands map to ${commandCoveredFocusedSkills.size} focused skills with taskType`,
    );
  }

  const metadataCommandSetForTaxonomy = new Set(
    commandList.map((command) => text(command.command)).filter(Boolean),
  );
  const taskTaxonomyFailures = taskTaxonomy.flatMap((taskType) => {
    const taskTypeId = text(taskType.id) || "<missing-task-type>";
    const skillFailures = Array.isArray(taskType.focused_skills)
      ? taskType.focused_skills
          .map(text)
          .filter(Boolean)
          .filter((skillName) => !focusedSkillIds.has(skillName))
      : [];
    const missingCommands = Array.isArray(taskType.commands)
      ? taskType.commands
          .map(text)
          .filter(Boolean)
          .filter((command) => !metadataCommandSetForTaxonomy.has(command))
      : [];
    return [
      !text(taskType.label) ? `${taskTypeId} missing label` : "",
      !Array.isArray(taskType.use_when) || !taskType.use_when.length
        ? `${taskTypeId} missing use_when`
        : "",
      !text(taskType.completion_standard)
        ? `${taskTypeId} missing completion_standard`
        : "",
      skillFailures.length
        ? `${taskTypeId} unknown focused_skills ${skillFailures.join(", ")}`
        : "",
      missingCommands.length
        ? `${taskTypeId} commands missing from metadata ${missingCommands.join(", ")}`
        : "",
    ].filter(Boolean);
  });
  const missingRequiredTaskTypes = REQUIRED_TASK_TAXONOMY_IDS.filter(
    (taskTypeId) => !taskTaxonomyIds.has(taskTypeId),
  );
  const metadataCoveredTaskTypes = new Set(
    commandList.map((command) => text(command.taskType)).filter(Boolean),
  );
  const missingMetadataTaskTypes = [...taskTaxonomyIds]
    .filter((taskTypeId) => taskTypeId !== "passive_nonintervention")
    .filter((taskTypeId) => !metadataCoveredTaskTypes.has(taskTypeId));
  if (
    taskTaxonomyFailures.length ||
    missingRequiredTaskTypes.length ||
    missingMetadataTaskTypes.length
  ) {
    report.fail(
      "capability registry task taxonomy",
      [
        taskTaxonomyFailures.join("; "),
        missingRequiredTaskTypes.length
          ? `required task types missing: ${missingRequiredTaskTypes.join(", ")}`
          : "",
        missingMetadataTaskTypes.length
          ? `task types not used by command metadata: ${missingMetadataTaskTypes.join(", ")}`
          : "",
      ]
        .filter(Boolean)
        .join("; "),
    );
  } else {
    report.pass(
      "capability registry task taxonomy",
      `${taskTaxonomy.length} task types cover command metadata and passive nonintervention`,
    );
  }

  const analytix = commandList.find((item) => item.command === "/analytix");
  const audit = commandList.find((item) => item.command === "/analytix audit");
  const lab = commandList.find((item) => item.command === "/analytix lab");
  const plan = commandList.find((item) => item.command === "/analytix plan");
  const routeFailures = [];
  if (
    !commandToolNames(analytix).some((name) =>
      ["get_current_case", "get_case_scope_map"].includes(name),
    )
  )
    routeFailures.push("/analytix missing entry/status semantic tools");
  if (
    !commandToolNames(audit).some((name) =>
      [
        "inspect_case_schema",
        "audit_unindexed_sources",
        "audit_case_data_quality",
        "resolve_duplicate_families",
      ].includes(name),
    )
  )
    routeFailures.push("/analytix audit missing schema/quality semantic tools");
  if (!commandToolNames(lab).includes("hypothesis_probe"))
    routeFailures.push("/analytix lab missing hypothesis semantic tool");
  if (!commandToolNames(plan).includes("plan_case_analysis"))
    routeFailures.push("/analytix plan missing commander plan");
  if (routeFailures.length) {
    report.fail("route gates", routeFailures.join("; "));
  } else {
    report.pass(
      "route gates",
      "entry/status, source/schema audit, lab hypothesis, and commander plan are routable",
    );
  }

  const registryCapabilities = arrayOf(capabilityRegistry.capabilities);
  const evalCoverageSchemaValidation = validateEvalCoverageRegistrySchema({
    capabilityRegistrySchema,
  });
  const navigatorPrimaryCommandFailures = commandList
    .filter((command) =>
      arrayOf(command.primaryTools).map(text).includes("funds_investigate"),
    )
    .filter(
      (command) =>
        !NAVIGATOR_PRIMARY_ALLOWED_COMMANDS.has(text(command.command)),
    )
    .map(
      (command) =>
        `${text(command.command) || "<missing command>"} (${text(command.taskType) || "missing taskType"})`,
    );
  const navigatorPrimaryCapabilityFailures = registryCapabilities
    .filter((capability) =>
      arrayOf(capability.primary_tools).map(text).includes("funds_investigate"),
    )
    .filter(
      (capability) =>
        !NAVIGATOR_PRIMARY_ALLOWED_CAPABILITIES.has(text(capability.id)),
    )
    .map((capability) => text(capability.id) || "<missing capability>");
  if (
    navigatorPrimaryCommandFailures.length ||
    navigatorPrimaryCapabilityFailures.length
  ) {
    report.fail(
      "funds_investigate navigator routing",
      [
        navigatorPrimaryCommandFailures.length
          ? `command primaryTools still route through navigator: ${navigatorPrimaryCommandFailures.join(", ")}`
          : "",
        navigatorPrimaryCapabilityFailures.length
          ? `capability primary_tools still route through navigator: ${navigatorPrimaryCapabilityFailures.join(", ")}`
          : "",
      ]
        .filter(Boolean)
        .join("; "),
    );
  } else {
    report.pass(
      "funds_investigate navigator routing",
      "funds_investigate is only a fuzzy navigator/support route; explicit Pair Amount, ranking, dossier, trace, lab, report, and data-quality tasks use focused owners or targeted semantic tools",
    );
  }
  const registryCommandOwners = new Map();
  const duplicateRegistryCommands = [];
  for (const capability of registryCapabilities) {
    for (const command of arrayOf(capability.commands)
      .map(text)
      .filter(Boolean)) {
      if (registryCommandOwners.has(command)) {
        duplicateRegistryCommands.push(command);
      } else {
        registryCommandOwners.set(command, text(capability.id));
      }
    }
  }
  const metadataCommandNames = commandList
    .map((item) => text(item.command))
    .filter(Boolean);
  const missingRegistryCommands = metadataCommandNames.filter(
    (command) => !registryCommandOwners.has(command),
  );
  const extraRegistryCommands = [...registryCommandOwners.keys()].filter(
    (command) => !metadataCommandSet.has(command),
  );
  const registryToolNames = new Set(
    registryCapabilities.flatMap((capability) =>
      [
        ...arrayOf(capability.primary_tools),
        ...arrayOf(capability.conditional_tools),
      ]
        .map(text)
        .filter(Boolean),
    ),
  );
  const missingRegistryTools = [...registryToolNames].filter(
    (tool) => !serverTools.has(tool),
  );
  const commandToolCoverageFailures = commandList.flatMap((command) => {
    const commandName = text(command.command);
    const capability = registryCapabilities.find((item) =>
      arrayOf(item.commands).map(text).includes(commandName),
    );
    if (!commandName || !capability) return [];
    const capabilityTools = new Set(
      [
        ...arrayOf(capability.primary_tools),
        ...arrayOf(capability.conditional_tools),
      ]
        .map(text)
        .filter(Boolean),
    );
    const commandTools = [
      ...arrayOf(command.primaryTools),
      ...arrayOf(command.conditionalTools),
    ]
      .map(text)
      .filter(Boolean);
    const missing = commandTools.filter((tool) => !capabilityTools.has(tool));
    return missing.length ? [`${commandName}: ${missing.join(", ")}`] : [];
  });
  const commandBudgetFailures = commandList.flatMap((command) => {
    const commandName = text(command.command);
    const budget = objectOf(command.toolBudget);
    const recommended = Number(budget.recommendedToolCalls);
    const max = Number(budget.maxToolCalls);
    const maxWithAudit = Number(budget.maxToolCallsWithAuditBoundaries);
    const derivedBudget = commandBudgetFromCapabilityRegistry(
      commandName,
      capabilityRegistry,
    );
    const ordinaryMax = Number(derivedBudget?.ordinary_max_tool_calls || 1);
    const auditMax = Number(
      derivedBudget?.max_command_audit_tool_calls ||
        derivedBudget?.report_max_tool_calls ||
        2,
    );
    if (!Object.keys(budget).length) return [];
    if (
      recommended > ordinaryMax ||
      max > ordinaryMax ||
      maxWithAudit > auditMax
    ) {
      return [
        `${commandName}: recommended=${budget.recommendedToolCalls} max=${budget.maxToolCalls} audit=${budget.maxToolCallsWithAuditBoundaries}`,
      ];
    }
    return [];
  });
  const commandBudgetContract = validateCommandMetadataBudgetContract({
    capabilityRegistry,
    commandMetadata,
  });
  const budgetFailures = registryCapabilities.flatMap((capability) => {
    const capabilityId = text(capability.id);
    const budget = objectOf(capability.tool_budget);
    const ordinaryMax = Number(budget.ordinary_max_tool_calls);
    const reportMax = Number(budget.report_max_tool_calls);
    const commandAuditMax = Number(budget.max_command_audit_tool_calls || 2);
    const maxOrdinary = capabilityId === "full-case-analysis" ? 3 : 2;
    if (
      ordinaryMax < 1 ||
      ordinaryMax > maxOrdinary ||
      reportMax > commandAuditMax ||
      reportMax < Math.min(ordinaryMax, 2)
    ) {
      return [
        `${capabilityId} ordinary=${budget.ordinary_max_tool_calls} report=${budget.report_max_tool_calls}`,
      ];
    }
    return [];
  });
  const evidenceFailures = registryCapabilities.flatMap((capability) => {
    const contract = objectOf(capability.evidence_contract);
    const missing = [];
    if (contract.ledger_required !== true) missing.push("ledger_required");
    if (contract.supported_edge_required_for_mermaid !== true)
      missing.push("supported_edge_required_for_mermaid");
    if (contract.user_visible_opaque_refs_allowed !== false)
      missing.push("user_visible_opaque_refs_allowed=false");
    return missing.length
      ? [`${text(capability.id)} missing ${missing.join("/")}`]
      : [];
  });
  const goldenPath = path.join(
    pluginRoot,
    "scripts",
    "eval-fixtures",
    "golden-answer-set.json",
  );
  const golden = readJsonIfExists(goldenPath);
  const evalCoverageFixture = readJsonIfExists(evalCoverageContractPath);
  const capabilityRegistryWithEval = withEvalCoverageContract(
    capabilityRegistry,
    evalCoverageFixture || {},
  );
  const goldenTaskIds = new Set(
    arrayOf(golden?.tasks)
      .map((task) => text(task.id))
      .filter(Boolean),
  );
  const registryEvalTaskIds = new Set(
    arrayOf(capabilityRegistryWithEval.capabilities).flatMap((capability) =>
      arrayOf(capability.eval_tasks).map(text).filter(Boolean),
    ),
  );
  const derivedRegistryMetadata =
    validateCapabilityRegistryDerivedMetadataContract({
      capabilityRegistry: capabilityRegistryWithEval,
      commandMetadata,
      goldenAnswerSet: golden || {},
    });
  const missingEvalTasks = [...registryEvalTaskIds].filter(
    (taskId) => !goldenTaskIds.has(taskId),
  );
  const uncoveredGoldenTasks = [...goldenTaskIds].filter(
    (taskId) => !registryEvalTaskIds.has(taskId),
  );
  const registryFailures = [
    text(capabilityRegistry.schema_version) !== "capability-registry-v1"
      ? "schema_version must be capability-registry-v1"
      : "",
    text(capabilityRegistry.plugin?.name) !== text(pluginManifest.name)
      ? "plugin name mismatch"
      : "",
    text(capabilityRegistry.plugin?.version) !== text(pluginManifest.version)
      ? "plugin version mismatch"
      : "",
    !evalCoverageSchemaValidation.ok
      ? evalCoverageSchemaValidation.failures.join("; ")
      : "",
    !registryCapabilities.length ? "no capabilities" : "",
    duplicateRegistryCommands.length
      ? `duplicate commands: ${duplicateRegistryCommands.join(", ")}`
      : "",
    missingRegistryCommands.length
      ? `missing commands: ${missingRegistryCommands.join(", ")}`
      : "",
    extraRegistryCommands.length
      ? `extra commands: ${extraRegistryCommands.join(", ")}`
      : "",
    missingRegistryTools.length
      ? `tools not exposed by MCP server: ${missingRegistryTools.join(", ")}`
      : "",
    commandToolCoverageFailures.length
      ? `command tools missing from owning capability: ${commandToolCoverageFailures.join("; ")}`
      : "",
    commandBudgetFailures.length
      ? `command tool budgets exceed registry budget: ${commandBudgetFailures.join("; ")}`
      : "",
    !commandBudgetContract.ok
      ? `command budget contract: ${commandBudgetContract.failures.join("; ")}`
      : "",
    budgetFailures.length
      ? `bad tool budgets: ${budgetFailures.join("; ")}`
      : "",
    evidenceFailures.length
      ? `bad evidence contracts: ${evidenceFailures.join("; ")}`
      : "",
    missingEvalTasks.length
      ? `eval tasks not in golden set: ${missingEvalTasks.join(", ")}`
      : "",
    uncoveredGoldenTasks.length
      ? `golden tasks not covered by registry: ${uncoveredGoldenTasks.join(", ")}`
      : "",
  ].filter(Boolean);
  if (registryFailures.length) {
    report.fail("capability registry drift", registryFailures.join("; "));
  } else {
    report.pass(
      "capability registry drift",
      `${registryCapabilities.length} capabilities cover ${metadataCommandNames.length} commands, ${registryToolNames.size} tools, and ${goldenTaskIds.size} golden task ids`,
    );
  }
  if (derivedRegistryMetadata.ok) {
    const summary = derivedRegistryMetadata.summary;
    report.pass(
      "capability registry derived metadata",
      `${summary.command_count} commands and ${summary.tool_count} tools derive from production registry; ${summary.eval_task_count} eval tasks, ${summary.passive_nonfunds_task_count} passive tasks, and ${summary.report_task_count} report tasks derive from eval-only contract`,
    );
  } else {
    report.fail(
      "capability registry derived metadata",
      derivedRegistryMetadata.failures.join("; "),
    );
  }
  const evalCoverage = validateEvalCoverageContract({
    capabilityRegistry: capabilityRegistryWithEval,
    goldenAnswerSet: golden || {},
  });
  if (evalCoverage.ok) {
    report.pass(
      "golden eval coverage ratchet",
      formatEvalCoverageSummary(evalCoverage),
    );
  } else {
    report.fail(
      "golden eval coverage ratchet",
      evalCoverage.failures.join("; "),
    );
  }

  const ledgerContract = validateEvidenceLedgerDoctorContract();
  if (ledgerContract.ok) {
    report.pass(
      "evidence ledger traceability contract",
      `required surfaces covered; unanchored sample rejected with ${ledgerContract.unanchored.failures.length} validation failures`,
    );
  } else {
    report.fail(
      "evidence ledger traceability contract",
      ledgerContract.failures.join("; "),
    );
  }

  const answerCardContinuationContract =
    validateAnswerCardContinuationContract();
  if (answerCardContinuationContract.ok) {
    report.pass(
      "answer card continuation contract",
      "answer_card_complete is separate from fact_answer_allowed and unsupported cards remain boundary-only",
    );
  } else {
    report.fail(
      "answer card continuation contract",
      answerCardContinuationContract.failures.join("; "),
    );
  }

  const dataAnalyticsBoundaryContract =
    validateDataAnalyticsEquivalentBoundaryContract();
  if (dataAnalyticsBoundaryContract.ok) {
    report.pass(
      "Data Analytics equivalent boundary contract",
      `${dataAnalyticsBoundaryContract.marker_count} soft-boundary principles are present across ${dataAnalyticsBoundaryContract.source_count} blueprint/skill surfaces`,
    );
  } else {
    report.fail(
      "Data Analytics equivalent boundary contract",
      dataAnalyticsBoundaryContract.failures.join("; "),
    );
  }

  const controlledCaseWorkbenchContract =
    validateControlledCaseWorkbenchDoctorContract();
  if (controlledCaseWorkbenchContract.ok) {
    report.pass(
      "controlled Case Workbench contract",
      "SQL/notebook tools require purpose plus frozen host case context or explicit case_id, reject backend active-case fallback, enforce cleaned/analysis scope, clamp rows, block raw/write/external paths, and return capability gaps instead of fabricated facts",
    );
  } else {
    report.fail(
      "controlled Case Workbench contract",
      controlledCaseWorkbenchContract.failures.join("; "),
    );
  }

  const claimVerifierContract = validateClaimVerifierDoctorContract();
  if (claimVerifierContract.ok) {
    report.pass(
      "claim verifier text risk contract",
      "synthetic report text blocks unsupported amounts, Mermaid flows, ownership upgrades, cash/asset destinations, missing counterparty breaks, legal overreach, report tables without analysis, and unanchored claims",
    );
  } else {
    report.fail(
      "claim verifier text risk contract",
      claimVerifierContract.failures.join("; "),
    );
  }

  const investigationAnswerContract =
    validateInvestigationAnswerDoctorContract();
  if (investigationAnswerContract.ok) {
    report.pass(
      "investigation answer schema contract",
      "FinalInvestigationAnswer requires objective, conclusion, verified evidence, supported flow paths, boundaries, proof actions, and deterministic repair guidance",
    );
  } else {
    report.fail(
      "investigation answer schema contract",
      investigationAnswerContract.failures.join("; "),
    );
  }

  const fundGraphEvidenceContract =
    validateFundGraphEvidencePackDoctorContract();
  if (fundGraphEvidenceContract.ok) {
    report.pass(
      "fundgraph evidence pack contract",
      "supported transaction edges produce evidence_pack and claim_support with source refs; incomplete edges stay in evidence_boundaries",
    );
  } else {
    report.fail(
      "fundgraph evidence pack contract",
      fundGraphEvidenceContract.failures.join("; "),
    );
  }

  const visualArtifactContract = validateVisualArtifactDoctorContract();
  if (visualArtifactContract.ok) {
    report.pass(
      "visual artifact delivery contract",
      "tables, charts, fund-flow graphs, and image/file handoffs require scope, source anchors, supported edges, clean visible labels, and file inspection",
    );
  } else {
    report.fail(
      "visual artifact delivery contract",
      visualArtifactContract.failures.join("; "),
    );
  }

  const casegraphRankEvidenceContract =
    validateCasegraphRankEvidencePackDoctorContract();
  if (casegraphRankEvidenceContract.ok) {
    report.pass(
      "casegraph rank evidence pack contract",
      "account, holder, and counterparty rank facts produce evidence_pack and claim_support with source refs",
    );
  } else {
    report.fail(
      "casegraph rank evidence pack contract",
      casegraphRankEvidenceContract.failures.join("; "),
    );
  }

  const casegraphComponentEvidenceContract =
    validateCasegraphComponentEvidencePackDoctorContract();
  if (casegraphComponentEvidenceContract.ok) {
    report.pass(
      "casegraph component evidence pack contract",
      "roadmap components produce evidence_pack, claim_support, evidence_boundaries, and source refs without upgrading leads into facts",
    );
  } else {
    report.fail(
      "casegraph component evidence pack contract",
      casegraphComponentEvidenceContract.failures.join("; "),
    );
  }

  const lineCount = skillSource.split(/\r?\n/u).length;
  if (lineCount <= 220) {
    report.pass("root skill size", `${lineCount} lines`);
  } else {
    report.warn(
      "root skill size",
      `${lineCount} lines; consider moving more detail to references`,
    );
  }

  const commandTableRows = skillSource
    .split(/\r?\n/u)
    .filter((line) => line.trim().startsWith("| `/analytix")).length;
  if (commandTableRows === 0) {
    report.pass(
      "progressive disclosure",
      "root skill delegates command table to references",
    );
  } else {
    report.warn(
      "progressive disclosure",
      `${commandTableRows} command rows remain in root skill`,
    );
  }

  const staleReferenceLinks = skillSource.includes("../../references/");
  if (staleReferenceLinks) {
    report.fail(
      "self-contained skill references",
      "SKILL.md must use ./references so required skill copies remain valid",
    );
  } else if (skillSource.includes("./references/command-router.md")) {
    report.pass(
      "self-contained skill references",
      "SKILL.md uses embedded reference paths",
    );
  } else {
    report.warn(
      "self-contained skill references",
      "expected ./references links were not found",
    );
  }

  const referenceDrift = REFERENCE_FILES.filter((relativePath) => {
    const rootText = readTextIfExists(
      path.join(rootReferenceDir, relativePath),
    );
    const embeddedText = readTextIfExists(
      path.join(embeddedReferenceDir, relativePath),
    );
    return !rootText || !embeddedText || rootText !== embeddedText;
  });
  if (referenceDrift.length) {
    report.fail("embedded reference sync", referenceDrift.join(", "));
  } else {
    report.pass(
      "embedded reference sync",
      `${REFERENCE_FILES.length} files match root references`,
    );
  }

  const runtimeCacheEvalOnlyViolations = runtimeCacheContractViolations();
  const productionMcpCacheOmissions = PRODUCTION_MCP_ENTRY_CLOSURE_FILES
    .filter((relativePath) => !PLUGIN_CACHE_FILES.includes(relativePath));
  const dormantMcpCacheEntries = PLUGIN_CACHE_FILES
    .filter((relativePath) => relativePath.startsWith("mcp/"))
    .filter((relativePath) => !PRODUCTION_MCP_ENTRY_CLOSURE_FILES.includes(relativePath));
  let productionMcpClosureError = "";
  try {
    inspectProductionMcpEntryClosure(pluginRoot);
  } catch (error) {
    productionMcpClosureError = error instanceof Error ? error.message : String(error);
  }
  if (runtimeCacheEvalOnlyViolations.length) {
    report.fail(
      "runtime cache production file list",
      runtimeCacheEvalOnlyViolations
        .map(
          (item) =>
            `${item.scope}:${item.relativePath} matches ${item.forbidden_pattern}`,
        )
        .join("; "),
    );
  } else {
    report.pass(
      "runtime cache production file list",
      "runtime sync contract excludes eval fixtures, golden answer sets, eval rubrics, and oracle fast paths",
    );
  }
  if (
    productionMcpCacheOmissions.length ||
    dormantMcpCacheEntries.length ||
    productionMcpClosureError
  ) {
    report.fail(
      "runtime cache mcp entry closure",
      [
        productionMcpCacheOmissions.length
          ? `missing production modules: ${productionMcpCacheOmissions.join(", ")}`
          : "",
        dormantMcpCacheEntries.length
          ? `dormant modules included: ${dormantMcpCacheEntries.join(", ")}`
          : "",
        productionMcpClosureError,
      ].filter(Boolean).join("; "),
    );
  } else {
    report.pass(
      "runtime cache mcp entry closure",
      `runtime cache includes exactly the ${PRODUCTION_MCP_ENTRY_CLOSURE_FILES.length} structurally verified production MCP modules and excludes dormant modules`,
    );
  }

  const mcpConfig = mcpManifest?.mcpServers?.analytix_funds || {};
  const mcpEnv = objectOf(mcpConfig.env);
  if (
    mcpConfig.disabled === true &&
    text(mcpConfig.cwd) === "." &&
    Array.isArray(mcpConfig.env_vars) &&
    mcpEnv.ANALYTIX_API_BASE_URL === undefined
  ) {
    report.pass(
      "mcp packaging",
      "funds MCP is P0-disabled before process creation; dormant cwd/env remain package-audited only",
    );
  } else {
    report.fail(
      "mcp packaging",
      "analytix_funds must be disabled until Go native authority exists, with cwd='.', env_vars allowlist, and no hardcoded backend URL",
    );
  }

  if (text(pluginManifest.name) && text(pluginManifest.version)) {
    report.pass(
      "plugin manifest identity",
      `${pluginManifest.name}@${pluginManifest.version}`,
    );
    const versionGaps = pluginVersionAlignmentGaps(
      text(pluginManifest.version),
    );
    if (versionGaps.length) {
      report.fail("plugin runtime version alignment", versionGaps.join("; "));
    } else {
      report.pass(
        "plugin runtime version alignment",
        `all runtime, registry, probe, cache, and Go host allowlist identities match ${text(pluginManifest.version)}`,
      );
    }
    if (releaseNotesSource.includes(`## ${text(pluginManifest.version)}`)) {
      report.pass(
        "release notes current version",
        `RELEASE_NOTES.md contains ${text(pluginManifest.version)}`,
      );
    } else {
      report.fail(
        "release notes current version",
        `RELEASE_NOTES.md must include ## ${text(pluginManifest.version)}`,
      );
    }
    if (options.skipRuntime) {
      report.pass(
        "local runtime install cache",
        "skipped by --skip-runtime; Hub install/remount verification must run after publish",
      );
    } else {
      checkLocalRuntimeInstallCache(report, text(pluginManifest.version));
    }
  } else {
    report.fail("plugin manifest identity", "name or version missing");
  }

  if (!options.skipGit) {
    try {
      const changed = runGitDiffNameOnly();
      const forbidden = changed.filter((filePath) =>
        FORBIDDEN_DIFF_PREFIXES.some((prefix) => filePath.startsWith(prefix)),
      );
      if (forbidden.length) {
        report.fail("Codex isolation diff", forbidden.join(", "));
      } else {
        report.pass(
          "Codex isolation diff",
          "no changed files under analytixagent/CodexDesktop/codex",
        );
      }
    } catch (error) {
      report.warn(
        "Codex isolation diff",
        error instanceof Error ? error.message : String(error),
      );
    }
    try {
      const changed = runGitStatusPaths();
      const forbidden = changed.filter((filePath) =>
        PLUGIN_BOUNDARY_FORBIDDEN_STATUS_PREFIXES.some((prefix) =>
          filePath.startsWith(prefix),
        ),
      );
      if (forbidden.length) {
        report.fail("plugin boundary diff", forbidden.join(", "));
      } else {
        report.pass(
          "plugin boundary diff",
          "no changed or untracked files under desktop runtime, analytixagent, or Codex runtime paths",
        );
      }
    } catch (error) {
      report.warn(
        "plugin boundary diff",
        error instanceof Error ? error.message : String(error),
      );
    }
  }

  report.print(options.json);
  process.exit(report.failed ? 1 : 0);
}

main();
