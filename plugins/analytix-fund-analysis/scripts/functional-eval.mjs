#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { tools as MCP_TOOL_SCHEMAS } from "../mcp/tool-schemas.mjs";
import {
  hostFactRuntimeContext,
  writeCaseProjectBinding
} from "./host-runtime-context-fixture.mjs";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");
const DEFAULT_BACKEND_URL = "http://127.0.0.1:18731";
const DEFAULT_OUTPUT_ROOT = path.join(os.tmpdir(), "analytix-fund-analysis", "functional-closure");

const TASKS = [
  {
    id: "pair_amount_contract",
    family: "pair_amount_reconciliation",
    description: "Verify ranking/frontdoor support cannot lure Codex into final amount-review conclusions and pair-amount-investigation owns original/effective/dedup review.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/pair-amount-contract-smoke.mjs"]],
    requires_backend: false
  },
  {
    id: "lead_owner_contract",
    family: "focused_owner_architecture",
    description: "Verify index/root metadata route substantive lanes to focused owners and keep funds_investigate as navigator/support.",
    runner: "static",
    check: checkLeadOwnerContract,
    requires_backend: false
  },
  {
    id: "support_layer_hidden_contract",
    family: "support_layer_hidden",
    description: "Verify production support runtimes do not generate answer drafts and ordinary agent-readable output is concise fact text with internal support state hidden.",
    runner: "static",
    check: checkSupportLayerHiddenContract,
    requires_backend: false
  },
  {
    id: "quick_fact_no_pair_amount_owner",
    family: "pair_amount_reconciliation",
    description: "Verify quick-fact no longer owns Pair Amount or instructs funds_investigate first-call amount drafts.",
    runner: "static",
    check: checkQuickFactNoPairAmountOwner,
    requires_backend: false
  },
  {
    id: "workflow_context_contract",
    family: "analytix_workflow_context",
    description: "Verify Analytix workflow context covers current case, cleaned exports, analysis indexes, reports, visuals, attachments, and continuation.",
    runner: "static",
    check: checkWorkflowContextContract,
    requires_backend: false
  },
  {
    id: "delivery_qc_contract",
    family: "economic_investigation_delivery",
    description: "Verify delivery QC blocks fact-only, template-like, engineering-word-leaking case answers.",
    runner: "static",
    check: checkDeliveryQcContract,
    requires_backend: false
  },
  {
    id: "non_template_output_contract",
    family: "economic_investigation_delivery",
    description: "Verify substantive outputs require investigative judgment rather than templates, tool manuals, or audit checklists.",
    runner: "static",
    check: checkNonTemplateOutputContract,
    requires_backend: false
  },
  {
    id: "cleaned_export_contract",
    family: "analytix_workflow_context",
    description: "Verify cleaned-detail export preserves Analytix cleaned fields and uses review sheets/inspection before delivery.",
    runner: "static",
    check: checkCleanedExportContract,
    requires_backend: false
  },
  {
    id: "report_continuation_contract",
    family: "analytix_workflow_context",
    description: "Verify report-builder supports continuation, local section patches, full-report amount recomputation, and artifact handoff.",
    runner: "static",
    check: checkReportContinuationContract,
    requires_backend: false
  },
  {
    id: "visual_artifact_contract",
    family: "analytix_workflow_context",
    description: "Verify visual-evidence requires real table/chart/Mermaid/PNG/JPG/workbook artifacts or explicit delivery blockers.",
    runner: "static",
    check: checkVisualArtifactContract,
    requires_backend: false
  },
  {
    id: "investigative_spine_contract",
    family: "economic_investigation_delivery",
    description: "Verify substantive answers must cover what was checked, conclusion, evidence, abnormality, case significance, limits, and next materials.",
    runner: "static",
    check: checkInvestigativeSpineContract,
    requires_backend: false
  },
  {
    id: "professional_judgment_contract",
    family: "economic_investigation_delivery",
    description: "Verify critique/QC blocks fact-only answers that lack tables, abnormality, case significance, limits, and next proof actions.",
    runner: "static",
    check: checkProfessionalJudgmentContract,
    requires_backend: false
  },
  {
    id: "model_eval_standard_contract",
    family: "economic_investigation_delivery",
    description: "Verify the legacy comparison harness no longer rewards support-card headings, Workbench/MCP internals, or report-gate engineering markers.",
    runner: "static",
    check: checkModelEvalStandardContract,
    requires_backend: false
  },
  {
    id: "pair_amount_mini_investigation_smoke",
    family: "pair_amount_reconciliation",
    description: "Verify Pair Amount delivery requires mini investigation material and support cards cannot be treated as complete answers.",
    runner: "static",
    check: checkPairAmountMiniInvestigationSmoke,
    requires_backend: false
  },
  {
    id: "hero_delivery_contract",
    family: "economic_investigation_delivery",
    description: "Verify graph, visual, report, appendix, and evidence-request paths require real hero deliverables or explicit blockers.",
    runner: "static",
    check: checkHeroDeliveryContract,
    requires_backend: false
  },
  {
    id: "desktop_multiturn_investigation_contract",
    family: "frontdoor_p1_acceptance",
    description: "Verify desktop real-case smoke hooks cover Pair Amount, continuation, graph/report/artifact workflows without embedding real facts.",
    runner: "static",
    check: checkDesktopMultiturnInvestigationContract,
    requires_backend: false
  },
  {
    id: "case_memo_plan_contract",
    family: "economic_investigation_delivery",
    description: "Verify report material planning reflects case-investigation memo structure and proof actions.",
    runner: "static",
    check: checkCaseMemoPlanContract,
    requires_backend: false
  },
  {
    id: "report_scenario_activation_contract",
    family: "economic_investigation_delivery",
    description: "Verify report scenario sections require same-case source capabilities and host receipts, ordinary full-case evals do not force typologies, and a non-project case rejects inactive sections.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/report-scenario-activation-contract.mjs", "--json"]],
    requires_backend: false
  },
  {
    id: "evidence_maturity_language_contract",
    family: "economic_investigation_delivery",
    description: "Verify user-visible evidence maturity vocabulary is present and engineering states stay hidden.",
    runner: "static",
    check: checkEvidenceMaturityLanguageContract,
    requires_backend: false
  },
  {
    id: "public_security_material_contract",
    family: "economic_investigation_delivery",
    description: "Verify report-grade tables, visuals, full-case answers, evidence requests, and claim review use public-security material language and table-after-analysis gates.",
    runner: "static",
    check: checkPublicSecurityMaterialContract,
    requires_backend: false
  },
  {
    id: "investigation_answer_schema_contract",
    family: "economic_investigation_delivery",
    description: "Verify Instructor-style schema-first final-answer validation blocks unanchored facts, candidate flow edges, legal upgrades, and engineering leakage without LLM scoring.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/investigation-answer-contract-smoke.mjs"]],
    requires_backend: false
  },
  {
    id: "focused_final_answer_adapter_contract",
    family: "economic_investigation_delivery",
    description: "Verify focused skills share an internal FinalInvestigationAnswer spine before rendering public-security economic-investigation prose.",
    runner: "static",
    check: checkFocusedFinalAnswerAdapterContract,
    requires_backend: false
  },
  {
    id: "user_visible_language_contract",
    family: "output_leak_prevention",
    description: "Verify ordinary visible text from compiler/card renderer is translated into economic-investigation language.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/user-visible-language-smoke.mjs"]],
    requires_backend: false
  },
  {
    id: "case_source_envelope_contract",
    family: "case_source_envelope",
    description: "Verify focused skills define the Data Analytics-style source, validation, delivery envelope.",
    runner: "static",
    check: checkCaseSourceEnvelopeContract,
    requires_backend: false
  },
  {
    id: "investigation_objective_gate_contract",
    family: "economic_investigation_delivery",
    description: "Verify Analytix clarifies only when purpose changes scope/delivery and otherwise proceeds with source-backed investigative direction.",
    runner: "static",
    check: checkInvestigationObjectiveGateContract,
    requires_backend: false
  },
  {
    id: "blueprint_14_2_matrix_contract",
    family: "benchmark_parity",
    description: "Verify the 14.2 benchmark matrix has concrete parity/gap, evidence, and release-blocker judgments.",
    runner: "static",
    check: checkBlueprint14_2MatrixContract,
    requires_backend: false
  },
  {
    id: "blueprint_capability_audit",
    family: "benchmark_parity",
    description: "Run deterministic blueprint/capability closure audit without launching Analytix or scoring LLM answers.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/blueprint-capability-audit.mjs", "--json", "--no-duckdb", "--fail-on-structural-gaps"]],
    output_directory: "blueprint-capability-audit",
    requires_backend: false
  },
  {
    id: "skill_clause_audit",
    family: "benchmark_parity",
    description: "Run clause-by-clause skill audit against mature-plugin source, owner, evidence, language, and completion-gate patterns.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/skill-clause-audit.mjs", "--json", "--fail-on-hard-gaps"]],
    output_directory: "skill-clause-audit",
    requires_backend: false
  },
  {
    id: "mcp_return_oracle_duckdb",
    family: "case_workbench_control",
    description: "Validate Workbench MCP returns against a read-only DuckDB oracle when a real case DB is available; no LLM answer scoring.",
    runner: "command",
    command: ["node", [
      "plugins/analytix-fund-analysis/scripts/mcp-return-oracle.mjs",
      "--summary-json",
      "--no-write",
      "--require-case-db",
      "--fail-on-gaps"
    ]],
    requires_backend: false,
    env_only: true,
    env_required: ["ANALYTIX_CASE_PROJECT_ROOT"],
    process_timeout_multiplier: 2
  },
  {
    id: "investigation_scenario_coverage_duckdb",
    family: "case_workbench_control",
    description: "Validate real cleaned/analysis DuckDB tables structurally support public-security fund-investigation scenarios without LLM scoring.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/investigation-scenario-audit.mjs", "--json", "--require-case-db", "--fail-on-structural-gaps"]],
    requires_backend: false,
    env_only: true,
    env_required: ["ANALYTIX_CASE_PROJECT_ROOT"],
    process_timeout_multiplier: 2
  },
  {
    id: "mcp_decentralization_contract",
    family: "mcp_decentralization",
    description: "Verify funds_investigate is a navigator and tool discovery is registry-backed instead of MCP-commanded.",
    runner: "static",
    check: checkMcpDecentralizationContract,
    requires_backend: false
  },
  {
    id: "case_workbench_control_contract",
    family: "case_workbench_control",
    description: "Verify controlled 专项资金核算 schemas and runtime block raw, write, external, unbounded, and fake-delivery paths.",
    runner: "static",
    check: checkCaseWorkbenchControlContract,
    requires_backend: false
  },
  {
    id: "schema_sql_name_contract",
    family: "case_workbench_control",
    description: "Verify inspect_case_schema exposes SQL-safe names and Workbench gaps do not fall back to funds_investigate answer drafting.",
    runner: "static",
    check: checkSchemaSqlNameContract,
    requires_backend: false
  },
  {
    id: "output_leak_contract",
    family: "output_leak_prevention",
    description: "Verify user-visible output surfaces strip internal protocol, report gate, doctor, opaque ref, and raw-row labels.",
    runner: "static",
    check: checkOutputLeakContract,
    requires_backend: false
  },
  {
    id: "user_delivery_contract",
    family: "economic_investigation_delivery",
    description: "Verify ordinary answers use answer-first economic-investigation delivery instead of audit headings or mechanical instruction contracts.",
    runner: "static",
    check: checkUserDeliveryContract,
    requires_backend: false
  },
  {
    id: "hub_package_isolation_contract",
    family: "release_package_isolation",
    description: "Prepare a temporary Hub package source and verify scripts/evidence/output/golden/oracle material is excluded.",
    runner: "package",
    requires_backend: false
  },
  {
    id: "frontdoor_real_pair_amount_env",
    family: "frontdoor_semantic_tools",
    description: "Run an env-driven real-case Pair Amount frontdoor smoke without storing real case answers in production fixtures.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs", "--tasks", "real_pair_amount_review", "--forbid-user-language-leakage"]],
    requires_backend: true,
    env_only: true,
    env_required: [
      "ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_CASE_ID",
      "ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_QUESTION",
      "ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_MARKERS"
    ]
  },
  {
    id: "frontdoor_real_case_source_blocker_env",
    family: "frontdoor_weak_source_boundaries",
    description: "Run an env-driven explicit invalid-case source blocker smoke without retrying or guessing case ids.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs", "--tasks", "real_case_source_blocker", "--forbid-user-language-leakage"]],
    requires_backend: true,
    env_only: true,
    env_required: [
      "ANALYTIX_FUNDS_CASE_SOURCE_BLOCKER_CASE_ID",
      "ANALYTIX_FUNDS_CASE_SOURCE_BLOCKER_MARKERS"
    ]
  },
  {
    id: "frontdoor_real_p1_env",
    family: "frontdoor_p1_acceptance",
    description: "Run one env-driven real-case P1 desktop-acceptance frontdoor smoke without storing real names or amounts in production fixtures.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs", "--tasks", "real_p1_frontdoor", "--forbid-user-language-leakage"]],
    requires_backend: true,
    env_only: true,
    env_required: [
      "ANALYTIX_FUNDS_REAL_P1_CASE_ID",
      "ANALYTIX_FUNDS_REAL_P1_QUESTION",
      "ANALYTIX_FUNDS_REAL_P1_MARKERS"
    ]
  },
  {
    id: "frontdoor_smoke_b1",
    family: "frontdoor_semantic_tools",
    description: "Run deterministic direct-MCP P0 containment smoke: without the Go host EvidenceReceipt registry, B1 fact paths must return only a fixed boundary.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs", "--profile", "b1", "--forbid-user-language-leakage"]],
    requires_backend: false,
    process_timeout_multiplier: 6
  },
  {
    id: "frontdoor_smoke_b2_weak",
    family: "frontdoor_weak_source_boundaries",
    description: "Run deterministic direct-MCP P0 containment smoke for weak-source and report paths; no fact or report may bypass the host registries.",
    runner: "command",
    command: ["node", ["plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs", "--profile", "b2-weak", "--forbid-user-language-leakage"]],
    requires_backend: false,
    process_timeout_multiplier: 8
  }
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function parseCsv(value) {
  return text(value).split(",").map((item) => item.trim()).filter(Boolean);
}

function readJson(relativePath) {
  return JSON.parse(fs.readFileSync(path.join(REPO_ROOT, relativePath), "utf8"));
}

function readText(relativePath) {
  return fs.readFileSync(path.join(REPO_ROOT, relativePath), "utf8");
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function schemaFor(toolName) {
  const tool = MCP_TOOL_SCHEMAS.find((item) => item.name === toolName);
  assert(tool, `missing MCP tool schema: ${toolName}`);
  return objectOf(tool.inputSchema);
}

function assertMarkers(label, body, markers) {
  const missing = markers.filter((marker) => !String(body || "").includes(marker));
  assert(missing.length === 0, `${label} missing marker(s): ${missing.join(", ")}`);
  return markers.length;
}

function parseJsonPayload(value) {
  try {
    return objectOf(JSON.parse(text(value)));
  } catch {
    return {};
  }
}

function gitHead() {
  const result = spawnSync("git", ["rev-parse", "HEAD"], {
    cwd: REPO_ROOT,
    encoding: "utf8"
  });
  return result.status === 0 ? text(result.stdout) : "";
}

function serverVersion() {
  const body = fs.readFileSync(path.join(REPO_ROOT, "plugins/analytix-fund-analysis/mcp/server.mjs"), "utf8");
  return text(body.match(/SERVER_VERSION\s*=\s*"([^"]+)"/u)?.[1]);
}

function releaseIdentity() {
  const manifest = readJson("plugins/analytix-fund-analysis/.codex-plugin/plugin.json");
  return {
    manifest_version: text(manifest.version),
    server_version: serverVersion(),
    head: gitHead()
  };
}

function timestamp() {
  return new Date().toISOString().replace(/[:.]/gu, "-");
}

function parseArgs(argv) {
  const options = {
    json: false,
    listTasks: false,
    backendUrl: DEFAULT_BACKEND_URL,
    taskIds: [],
    output: "",
    skipBackend: false,
    timeoutMs: 180_000
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--json") options.json = true;
    else if (arg === "--list-tasks") options.listTasks = true;
    else if (arg === "--tasks") options.taskIds = parseCsv(next());
    else if (arg.startsWith("--tasks=")) options.taskIds = parseCsv(arg.slice("--tasks=".length));
    else if (arg === "--backend-url") options.backendUrl = next() || options.backendUrl;
    else if (arg.startsWith("--backend-url=")) options.backendUrl = text(arg.slice("--backend-url=".length)) || options.backendUrl;
    else if (arg === "--output") options.output = next();
    else if (arg.startsWith("--output=")) options.output = text(arg.slice("--output=".length));
    else if (arg === "--skip-backend") options.skipBackend = true;
    else if (arg === "--timeout-ms") options.timeoutMs = Number(next()) || options.timeoutMs;
    else if (arg.startsWith("--timeout-ms=")) options.timeoutMs = Number(arg.slice("--timeout-ms=".length)) || options.timeoutMs;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  options.backendUrl = text(options.backendUrl).replace(/\/+$/u, "") || DEFAULT_BACKEND_URL;
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/functional-eval.mjs [options]

Runs source + validation + delivery functional checks. This runner is not an
A/B scorer and does not compare full_plugin against plugin_disabled.

Options:
  --list-tasks          Print the functional task registry.
  --tasks <ids>         Comma-separated task ids. Default: all tasks.
  --backend-url <url>   Analytix backend base URL. Default: ${DEFAULT_BACKEND_URL}
  --skip-backend        Skip tasks that require a live backend.
  --timeout-ms <n>      Per-task timeout. Default: 180000
  --output <file>       Output JSON path. Default: ${DEFAULT_OUTPUT_ROOT}/<timestamp>/functional-eval.json
  --json                Print machine-readable JSON.
`);
}

function selectedTasks(options) {
  if (!options.taskIds.length) return TASKS.filter((task) => !task.env_only);
  const known = new Set(TASKS.map((task) => task.id));
  const unknown = options.taskIds.filter((taskId) => !known.has(taskId));
  if (unknown.length) throw new Error(`unknown functional task id(s): ${unknown.join(", ")}`);
  return TASKS.filter((task) => options.taskIds.includes(task.id));
}

function defaultOutputPath() {
  return path.join(
    DEFAULT_OUTPUT_ROOT,
    timestamp(),
    "functional-eval.json"
  );
}

function checkCaseSourceEnvelopeContract() {
  const caseWorkbench = readText("plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md");
  const quickFact = readText("plugins/analytix-fund-analysis/skills/quick-fact/SKILL.md");
  const pairAmount = readText("plugins/analytix-fund-analysis/skills/pair-amount-investigation/SKILL.md");
  const shared = readText("plugins/analytix-fund-analysis/references/focused-skill-shared.md");
  const indexSkill = readText("plugins/analytix-fund-analysis/skills/index/SKILL.md");
  const caseSourceRecordMarkers = assertMarkers("case-workbench source record fields", caseWorkbench, [
    "case_identity",
    "source_of_truth",
    "source_scope",
    "data_quality_state",
    "metric_scope",
    "validation_state",
    "delivery_state",
    "gap_card"
  ]);
  if (!/case_source_envelope|internal source record|当前案件来源记录/iu.test(caseWorkbench)) {
    throw new Error("case-workbench source record missing internal envelope equivalent marker");
  }
  return {
    envelope_marker_count: caseSourceRecordMarkers + 1,
    quick_fact_boundary_marker_count: assertMarkers("quick-fact Pair Amount non-owner boundary", quickFact, [
      "does not own Pair Amount",
      "pair-amount-investigation",
      "Pair Amount turns are not complete in quick-fact"
    ]),
    pair_marker_count: assertMarkers("pair-amount owner amount review", pairAmount, [
      "raw detail amount/count",
      "effective or high-confidence dedup",
      "账户集合",
      "time concentrations",
      "duplicate/evidence-insufficient amount",
      "next verification path"
    ]),
    shared_boundary_marker_count: assertMarkers("focused-skill-shared Pair Amount boundary", shared, [
      "pair-amount-investigation",
      "原明细/高置信去重统计",
      "同事实/换卡风险",
      "核验意见",
      "本次依据",
      "支持程度"
    ]),
    index_route_marker_count: assertMarkers("index Pair Amount route", indexSkill, [
      "pair-amount-investigation",
      "data-quality",
      "case-workbench"
    ])
  };
}

function checkInvestigationObjectiveGateContract() {
  const rootSkill = readText("plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md");
  const indexSkill = readText("plugins/analytix-fund-analysis/skills/index/SKILL.md");
  const shared = readText("plugins/analytix-fund-analysis/references/focused-skill-shared.md");
  const embeddedShared = readText("plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/focused-skill-shared.md");
  assert(shared === embeddedShared, "embedded focused-skill-shared reference must match root reference");
  assert(!/每次.*(?:先|必须).*问/u.test(shared), "objective gate must not become mandatory pre-questioning");
  assert(!/问卷|表单/u.test(shared), "objective gate must not frame the workflow as a questionnaire");
  return {
    shared_objective_gate_markers: assertMarkers("shared investigation objective gate", shared, [
      "Investigation Objective Gate",
      "Ask a concise clarification only when the missing purpose would change",
      "Ask at most three concrete questions",
      "proceed without asking",
      "Never use clarification as a way to avoid source-backed work",
      "next investigative direction",
      "evidence maturity"
    ]),
    root_objective_gate_markers: assertMarkers("root investigation objective gate", rootSkill, [
      "Investigation Objective Gate",
      "trying to prove a relationship",
      "recompute an amount",
      "source/destination",
      "Ask only",
      "when the missing purpose would materially change object scope",
      "proceed and state the"
    ]),
    index_objective_gate_markers: assertMarkers("index investigation objective gate", indexSkill, [
      "Investigation Objective Gate",
      "ask at most three concrete case-material questions",
      "is inferable, route directly",
      "focused owner"
    ])
  };
}

function checkFocusedFinalAnswerAdapterContract() {
  const shared = readText("plugins/analytix-fund-analysis/references/focused-skill-shared.md");
  const embeddedShared = readText("plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/focused-skill-shared.md");
  const answerContract = readText("plugins/analytix-fund-analysis/references/investigation-answer-contract.md");
  const report = readText("plugins/analytix-fund-analysis/skills/report-builder/SKILL.md");
  const claimReview = readText("plugins/analytix-fund-analysis/skills/claim-review/SKILL.md");
  const deliveryQc = readText("plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md");
  assert(shared === embeddedShared, "embedded focused-skill-shared reference must match root reference");
  const ownerSkills = [
    "pair-amount-investigation",
    "account-dossier",
    "subject-dossier",
    "fund-tracing",
    "full-case-analysis",
    "visual-evidence",
    "evidence-request",
    "report-builder",
    "claim-review",
    "delivery-qc"
  ];
  for (const skill of ownerSkills) {
    const body = readText(`plugins/analytix-fund-analysis/skills/${skill}/SKILL.md`);
    assert(body.includes("focused-skill-shared"), `${skill} must read focused-skill-shared so the structured answer adapter applies`);
  }
  return {
    shared_adapter_marker_count: assertMarkers("focused structured answer adapter", shared, [
      "Internal Structured Answer Contract",
      "FinalInvestigationAnswer",
      "not visible JSON",
      "investigation_objective",
      "conclusion",
      "verified_facts",
      "MCP/DuckDB",
      "fund_flow_paths",
      "risk_patterns",
      "evidence_boundaries",
      "next_proof_actions",
      "final_investigation_answer",
      "Missing structure may be repaired",
      "missing facts, amounts, transaction edges, ownership/control claims",
      "conclusions must return to MCP/DuckDB",
      "public-security economic-investigation prose"
    ]),
    contract_reference_marker_count: assertMarkers("investigation answer contract source boundary", answerContract, [
      "567-labs/instructor",
      "schema-first validation",
      "model scorer",
      "FinalInvestigationAnswer",
      "MCP/DuckDB",
      "evidence_boundaries",
      "next_proof_actions",
      "audit_events",
      "validation failure",
      "It must not be asked to invent",
      "missing fact"
    ]),
    validation_path_marker_count: assertMarkers("report/claim/delivery structured validation path", [
      report,
      claimReview,
      deliveryQc
    ].join("\n"), [
      "FinalInvestigationAnswer",
      "final_investigation_answer",
      "validate_report_claims",
      "delivery-qc",
      "users see only公安经侦自然语言"
    ]),
    owner_skill_count: ownerSkills.length
  };
}

function checkLeadOwnerContract() {
  const rootSkill = readText("plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md");
  const indexSkill = readText("plugins/analytix-fund-analysis/skills/index/SKILL.md");
  const rootAgent = readText("plugins/analytix-fund-analysis/agents/openai.yaml");
  const metadata = readJson("plugins/analytix-fund-analysis/references/command-metadata.json");
  const registry = readJson("plugins/analytix-fund-analysis/references/capability-registry.json");
  const routing = objectOf(metadata.focusedSkillRouting);
  assert(arrayOf(routing["pair-amount-investigation"]).length > 0, "metadata must route Pair Amount to pair-amount-investigation");
  const focusedIds = arrayOf(registry.focused_skills).map((item) => text(objectOf(item).id));
  assert(focusedIds.includes("pair-amount-investigation"), "registry missing pair-amount-investigation");
  assert(focusedIds.includes("delivery-qc"), "registry missing delivery-qc");
  assert(!rootSkill.includes("首个内部动作应调用当前案件自然语言事实核验入口"), "root skill must not mandate first-call funds_investigate");
  assert(!rootSkill.includes("普通桌面问法应优先使用本工具"), "root skill must not prioritize funds_investigate for ordinary questions");
  return {
    root_marker_count: assertMarkers("root owner-first markers", rootSkill, [
      "focused owner",
      "funds_investigate",
      "不得替代",
      "pair-amount-investigation",
      "delivery-qc"
    ]),
    index_marker_count: assertMarkers("index owner route markers", indexSkill, [
      "focused owner",
      "support only",
      "pair-amount-investigation",
      "delivery-qc"
    ]),
    agent_marker_count: assertMarkers("agent owner route markers", rootAgent, [
      "desktop_owner_first",
      "pair_amount_investigation",
      "delivery_qc"
    ])
  };
}

function checkSupportLayerHiddenContract() {
  const compiler = readText("plugins/analytix-fund-analysis/mcp/agent-output-compiler.mjs");
  const contextCompiler = readText("plugins/analytix-fund-analysis/mcp/context-compiler.mjs");
  const cardRenderer = readText("plugins/analytix-fund-analysis/mcp/card-renderer.mjs");
  const frontdoor = readText("plugins/analytix-fund-analysis/mcp/frontdoor-runtime.mjs");
  const destination = readText("plugins/analytix-fund-analysis/mcp/destination-diagnostic-runtime.mjs");
  const topRankings = readText("plugins/analytix-fund-analysis/mcp/top-rankings-diagnostic-runtime.mjs");
  for (const [label, body] of Object.entries({ frontdoor, destination, topRankings })) {
    assert(!body.includes("answer_draft"), `${label} must not generate answer_draft in production runtime`);
  }
  assert(compiler.includes("buildSupportEvidenceEnvelope"), "agent output compiler must build a structured support envelope");
  assert(compiler.includes("boundedSupportEnvelope"), "agent output compiler must bound structured support before rendering concise fact text");
  assert(compiler.includes("renderSupportEnvelopeForAgent"), "agent output compiler must render readable non-JSON support text by default");
  assert(compiler.includes("support_only: true"), "agent output compiler must mark ordinary output support-only");
  assert(compiler.includes("final_answer_owned_by_focused_skill: true"), "agent output compiler must leave final answer to focused owner");
  assert(!compiler.includes("renderPairAmountReviewCardForAgent"), "agent output compiler must not contain Pair Amount prose renderer");
  assert(!compiler.includes("renderSupportCardForAgent"), "agent output compiler must not contain generic support prose renderer");
  assert(!compiler.includes("renderClaimReviewCardForAgent"), "agent output compiler must not contain claim-review prose renderer");
  assert(!contextCompiler.includes("card.answer_draft"), "context compiler must not promote answer_draft into must_write_facts");
  assert(cardRenderer.includes("renderDiagnosticCardForAudit"), "card renderer prose must be explicitly audit-only");
  assert(cardRenderer.includes("renderDiagnosticCardForAgent"), "card renderer must expose ordinary agent support renderer");
  assert(!/return\s+JSON\.stringify\(\{\s*support_only/u.test(cardRenderer), "ordinary card renderer must not return JSON support envelopes to the model");
  return {
    compiler_marker_count: assertMarkers("support-layer envelope markers", compiler, [
      "source_map",
      "validation_state",
      "support_facts",
      "compact_evidence",
      "known_risks",
      "query_guidance",
      "suggested_next_directions",
      "action_type",
      "evidence_basis",
      "artifact_provenance"
    ]),
    production_answer_draft_generators: 0
  };
}

function checkQuickFactNoPairAmountOwner() {
  const quickFact = readText("plugins/analytix-fund-analysis/skills/quick-fact/SKILL.md");
  assert(!quickFact.includes("compact investigative mini-review"), "quick-fact must not define Pair Amount mini-review");
  assert(!quickFact.includes("First call `funds_investigate` as the amount-review mini-review navigator"), "quick-fact must not first-call funds_investigate for amount review");
  return {
    marker_count: assertMarkers("quick-fact non-owner markers", quickFact, [
      "does not own Pair Amount",
      "pair-amount-investigation",
      "must not write the final amount conclusion",
      "Pair Amount turns are not complete in quick-fact"
    ])
  };
}

function checkWorkflowContextContract() {
  const workflow = readText("plugins/analytix-fund-analysis/references/analytix-workflow-context.md");
  const caseContext = readText("plugins/analytix-fund-analysis/skills/case-context/SKILL.md");
  const workbench = readText("plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md");
  return {
    workflow_marker_count: assertMarkers("workflow context reference", workflow, [
      "Current Case And Scope",
      "Cleaned Tables And Analysis Indexes",
      "Analysis, Graphs, Images, And Attachments",
      "Reports And Multi-Turn Continuation",
      "清洗明细导出",
      "空对手复核",
      "Mermaid/PNG/JPG",
      "Project-fund",
      "Delivery Spine"
    ]),
    case_context_marker_count: assertMarkers("case-context workflow markers", caseContext, [
      "cleaned export",
      "report/attachment directories",
      "graph/image"
    ]),
    workbench_marker_count: assertMarkers("case-workbench workflow markers", workbench, [
      "During P0 containment",
      "hidden and rejected before execution",
      "create no file"
    ])
  };
}

function checkDeliveryQcContract() {
  const deliveryQc = readText("plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md");
  const agent = readText("plugins/analytix-fund-analysis/skills/delivery-qc/agents/openai.yaml");
  return {
    delivery_qc_marker_count: assertMarkers("delivery QC skill", deliveryQc, [
      "查什么",
      "结论是什么",
      "依据哪些流水/账户/链路",
      "异常在哪里",
      "案件意义是什么",
      "还不能认定什么",
      "下一步调什么材料",
      "核验口径",
      "全期间同名收款人",
      "重点收款账户核验",
      "implementation vocabulary"
    ]),
    agent_marker_count: assertMarkers("delivery QC agent", agent, [
      "研判材料交付复核",
      "补证动作",
      "不新增案件事实"
    ])
  };
}

function checkNonTemplateOutputContract() {
  const shared = readText("plugins/analytix-fund-analysis/references/focused-skill-shared.md");
  const deliveryQc = readText("plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md");
  return {
    shared_marker_count: assertMarkers("non-template shared contract", shared, [
      "Answers that only restate a support card, audit checklist, or number fail",
      "Tool manual instead of investigation result",
      "delivery gate"
    ]),
    qc_marker_count: assertMarkers("non-template QC blockers", deliveryQc, [
      "tool card",
      "audit checklist",
      "gives only figures",
      "lacks a table"
    ])
  };
}

async function checkCleanedExportContract() {
  const workflow = readText("plugins/analytix-fund-analysis/references/analytix-workflow-context.md");
  const workbench = readText("plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md");
  const runtimeSource = readText("plugins/analytix-fund-analysis/mcp/cleaned-export-runtime.mjs");
  const exportSchema = schemaFor("export_cleaned_case_data");
  const exportProperties = objectOf(exportSchema.properties);
  assert(arrayOf(exportSchema.required).includes("purpose"), "cleaned export must require purpose");
  assert(objectOf(exportProperties.confirm_export).type === "boolean", "cleaned export must expose explicit confirmation");
  assert(!Object.prototype.hasOwnProperty.call(exportProperties, "target_dir"), "cleaned export must not expose arbitrary target_dir");
  assertMarkers("cleaned export runtime", runtimeSource, [
    "confirm_export",
    "cleaned export output escaped Analytix-owned app data",
    "inspectCleanedExportDirectory",
    "workbook_signature_present",
    "workbook_structure_present",
    "delivery_status: \"delivered\"",
    "delivery_status: \"blocked\""
  ]);

  const { createCleanedExportRuntime } = await import("../mcp/cleaned-export-runtime.mjs");
  const { compactToolPayloadForAgent } = await import("../mcp/agent-payload-compiler.mjs");
  const tempHome = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-cleaned-export-contract-"));
  const appRoot = path.join(tempHome, "data-analysis");
  const deliveredDir = path.join(appRoot, "cases", "case-contract", "exports", "delivered");
  const escapedDir = path.join(tempHome, "outside", "escaped");
  fs.mkdirSync(deliveredDir, { recursive: true });
  fs.mkdirSync(escapedDir, { recursive: true });
  const writeMinimalXlsxArchive = (filePath) => {
    const names = ["[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/worksheets/sheet1.xml"];
    const localParts = [];
    const directoryParts = [];
    let localOffset = 0;
    for (const name of names) {
      const nameBytes = Buffer.from(name, "utf8");
      const local = Buffer.alloc(30);
      local.writeUInt32LE(0x04034b50, 0);
      local.writeUInt16LE(20, 4);
      local.writeUInt16LE(nameBytes.length, 26);
      localParts.push(local, nameBytes);
      const directory = Buffer.alloc(46);
      directory.writeUInt32LE(0x02014b50, 0);
      directory.writeUInt16LE(20, 4);
      directory.writeUInt16LE(20, 6);
      directory.writeUInt16LE(nameBytes.length, 28);
      directory.writeUInt32LE(localOffset, 42);
      directoryParts.push(directory, nameBytes);
      localOffset += local.length + nameBytes.length;
    }
    const directoryBytes = Buffer.concat(directoryParts);
    const end = Buffer.alloc(22);
    end.writeUInt32LE(0x06054b50, 0);
    end.writeUInt16LE(names.length, 8);
    end.writeUInt16LE(names.length, 10);
    end.writeUInt32LE(directoryBytes.length, 12);
    end.writeUInt32LE(localOffset, 16);
    fs.writeFileSync(filePath, Buffer.concat([...localParts, directoryBytes, end]));
  };
  writeMinimalXlsxArchive(path.join(deliveredDir, "交易明细信息.xlsx"));
  writeMinimalXlsxArchive(path.join(escapedDir, "交易明细信息.xlsx"));
  try {
    const baseOptions = {
      resolveCase: async () => ({ case_id: "case-contract" }),
      httpPostData: async () => ({ job_id: "job-contract", case_id: "case-contract", status: "queued", progress: 0 }),
      sleepFn: async () => {},
      env: { HOME: tempHome, ANALYTIX_DATA_ANALYSIS_DIR: appRoot }
    };
    const deliveredRuntime = createCleanedExportRuntime({
      ...baseOptions,
      httpJson: async () => ({
        data: {
          job_id: "job-contract",
          case_id: "case-contract",
          status: "succeeded",
          progress: 100,
          output_path: deliveredDir
        }
      })
    });
    const noPurpose = await deliveredRuntime.exportCleanedCaseData({ confirm_export: true });
    assert(noPurpose.status === "blocked", "cleaned export without purpose must be blocked");
    const noConfirm = await deliveredRuntime.exportCleanedCaseData({ purpose: "交付清洗明细" });
    assert(noConfirm.status === "blocked", "cleaned export without explicit confirmation must be blocked");
    const invalidTables = await deliveredRuntime.exportCleanedCaseData({
      purpose: "验证非法表名阻断",
      confirm_export: true,
      tables: ["not_a_cleaned_table"]
    });
    assert(invalidTables.status === "blocked", "cleaned export with invalid tables must be blocked");
    assert(invalidTables.error_code === "CLEANED_EXPORT_TABLES_INVALID", "invalid cleaned export tables must not fall back silently");
    const delivered = await deliveredRuntime.exportCleanedCaseData({
      purpose: "交付清洗明细",
      confirm_export: true
    });
    assert(delivered.status === "ok", "cleaned export must deliver only after file inspection");
    assert(delivered.key_facts?.cleaned_export?.inspection_status === "passed", "cleaned export inspection must pass");
    assert(delivered.key_facts?.cleaned_export?.file_count === 1, "cleaned export must report inspected file count");
    const compactDelivery = compactToolPayloadForAgent(delivered);
    const compactDeliveryText = JSON.stringify(compactDelivery);
    assert(compactDeliveryText.includes("\"delivery_status\":\"delivered\""), "agent support envelope must retain cleaned export delivery status");
    assert(compactDeliveryText.includes("\"inspection_status\":\"passed\""), "agent support envelope must retain cleaned export inspection status");
    assert(compactDeliveryText.includes("交易明细信息.xlsx"), "agent support envelope must retain delivered file name");
    assert(!compactDeliveryText.includes(deliveredDir), "agent support envelope must not expose the full cleaned export path");
    assert(!compactDeliveryText.includes("output_path"), "agent support envelope must not expose cleaned export output_path");
    const pending = await deliveredRuntime.exportCleanedCaseData({
      purpose: "创建清洗明细并返回继续检查编号",
      confirm_export: true,
      wait_for_completion: false
    });
    const compactPendingText = JSON.stringify(compactToolPayloadForAgent(pending));
    assert(compactPendingText.includes("\"job_id\":\"job-contract\""), "pending agent support envelope must retain cleaned export continuation job_id");
    assert(!compactPendingText.includes("output_path"), "pending agent support envelope must not expose cleaned export output_path");

    const escapedRuntime = createCleanedExportRuntime({
      ...baseOptions,
      httpJson: async () => ({
        data: {
          job_id: "job-escaped",
          case_id: "case-contract",
          status: "succeeded",
          progress: 100,
          output_path: escapedDir
        }
      })
    });
    const escaped = await escapedRuntime.exportCleanedCaseData({
      purpose: "验证目录边界",
      confirm_export: true
    });
    assert(escaped.status === "blocked", "cleaned export outside Analytix-owned app data must be blocked");
    assert(escaped.error_code === "CLEANED_EXPORT_INSPECTION_FAILED", "escaped export must fail inspection");
  } finally {
    fs.rmSync(tempHome, { recursive: true, force: true });
  }

  return {
    workflow_marker_count: assertMarkers("cleaned export workflow", workflow, [
      "Cleaned export requests",
      "field names",
      "Chinese headers",
      "field order",
      "empty-counterparty",
      "export_cleaned_case_data"
    ]),
    workbench_marker_count: assertMarkers("cleaned export workbench", workbench, [
      "export_cleaned_case_data",
      "hidden and rejected before execution",
      "capability boundary",
      "create no file"
    ]),
    schema_has_confirmation_and_no_target_dir: true,
    delivered_file_inspection_verified: true,
    analytix_owned_path_boundary_verified: true,
    agent_support_envelope_hides_local_path: true,
    pending_support_envelope_retains_continuation_job_id: true
  };
}

function checkReportContinuationContract() {
  const workflow = readText("plugins/analytix-fund-analysis/references/analytix-workflow-context.md");
  const report = readText("plugins/analytix-fund-analysis/skills/report-builder/SKILL.md");
  return {
    workflow_marker_count: assertMarkers("report workflow context", workflow, [
      "Report work may continue an existing report",
      "Full-report amount checks",
      "preserve the existing structure"
    ]),
    report_marker_count: assertMarkers("report continuation skill", report, [
      "continue",
      "full-report amount verification",
      "preserve the existing report structure",
      "recompute the displayed figures",
      "verifying the modified section"
    ])
  };
}

async function checkVisualArtifactContract() {
  const workflow = readText("plugins/analytix-fund-analysis/references/analytix-workflow-context.md");
  const visual = readText("plugins/analytix-fund-analysis/skills/visual-evidence/SKILL.md");
  const { validateVisualArtifactContract } = await import("../mcp/visual-artifact-contract.mjs");
  const codes = (result) => new Set(arrayOf(result.errors).map((error) => text(error.code)));
  const validTable = validateVisualArtifactContract({
    artifact_type: "evidence_table",
    title: "重点账户大额支出证据表",
    scope: {
      unit: "元",
      time_window: "2025-08-01 至 2025-08-31",
      metric: "转出金额",
      direction: "支出",
      evidence_status: "已由交易明细支持"
    },
    rows: [
      {
        account_no: "9000000000000000018",
        counterparty_account: "9000000000000000015",
        amount: "21000000.00",
        txn_time: "2025-08-27 10:00:00",
        txn_id: "txn.supported.1",
        evidence_refs: ["txn.supported.1"]
      }
    ],
    table_after_analysis: "该笔支出金额较大，可作为重点资金去向线索，资金性质仍需结合合同、票据及对手方身份材料继续印证。"
  });
  const invalidTable = validateVisualArtifactContract({
    artifact_type: "evidence_table",
    title: "SQL debug candidate table",
    rows: [{ account_no: "9000000000000000018", amount: "21000000.00" }]
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
        source_refs: { query_ids: ["trace_subject_top_outflows"], evidence_ids: ["txn.supported.1"] }
      }
    ]
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
        txn_id: "txn.needs_review.1"
      }
    ]
  });
  const validImage = validateVisualArtifactContract({
    artifact_type: "png",
    title: "重点账户收支结构图",
    inspection_status: "passed",
    nonblank: true,
    source_refs: { query_ids: ["account_income_expense_summary"] }
  });
  const invalidImage = validateVisualArtifactContract({
    artifact_type: "png",
    title: "MCP debug chart",
    inspection_status: "pending"
  });
  const invalidTableCodes = codes(invalidTable);
  const invalidGraphCodes = codes(invalidGraph);
  const invalidImageCodes = codes(invalidImage);
  const failures = [
    !validTable.ok ? `valid evidence table failed: ${arrayOf(validTable.errors).map((error) => text(error.code)).join(", ")}` : "",
    invalidTable.ok ? "invalid evidence table unexpectedly passed" : "",
    !invalidTableCodes.has("VISUAL_SCOPE_FIELD_MISSING") ? "invalid evidence table must require visible scope fields" : "",
    !invalidTableCodes.has("VISUAL_ROW_SOURCE_ANCHOR_MISSING") ? "invalid evidence table must require row source anchors" : "",
    !invalidTableCodes.has("TABLE_AFTER_ANALYSIS_MISSING") ? "invalid evidence table must require table-after analysis" : "",
    !invalidTableCodes.has("VISIBLE_LABEL_INTERNAL_TERM") ? "invalid evidence table must block internal visible terms" : "",
    !validGraph.ok ? `valid fund-flow graph failed: ${arrayOf(validGraph.errors).map((error) => text(error.code)).join(", ")}` : "",
    invalidGraph.ok ? "invalid fund-flow graph unexpectedly passed" : "",
    !invalidGraphCodes.has("GRAPH_EDGE_NOT_SUPPORTED") ? "invalid fund-flow graph must block non-supported edges" : "",
    !validImage.ok ? `valid image artifact failed: ${arrayOf(validImage.errors).map((error) => text(error.code)).join(", ")}` : "",
    invalidImage.ok ? "invalid image artifact unexpectedly passed" : "",
    !invalidImageCodes.has("ARTIFACT_FILE_INSPECTION_NOT_PASSED") ? "invalid image must require passed inspection" : "",
    !invalidImageCodes.has("IMAGE_NONBLANK_CHECK_MISSING") ? "invalid image must require nonblank check" : "",
    !invalidImageCodes.has("VISIBLE_LABEL_INTERNAL_TERM") ? "invalid image must block internal visible terms" : ""
  ].filter(Boolean);
  if (failures.length) {
    throw new Error(failures.join("; "));
  }
  return {
    workflow_marker_count: assertMarkers("visual artifact workflow", workflow, [
      "Mermaid",
      "PNG",
      "JPG",
      "available Analytix/Codex delivery surface",
      "standalone PDF",
      "OCR pipeline",
      "asset-registration connector"
    ]),
    visual_marker_count: assertMarkers("visual artifact skill", visual, [
      "Mermaid, PNG, JPG",
      "available Analytix/Codex delivery surface",
      "inspect the delivered surface",
      "images are nonblank",
      "read/render inspection",
      "no standalone OCR/export/render system",
      "chat summary cannot claim file delivery"
    ]),
    validator_contract_checked: true,
    valid_table_status: validTable.status,
    invalid_table_error_codes: [...invalidTableCodes].sort(),
    valid_graph_status: validGraph.status,
    invalid_graph_error_codes: [...invalidGraphCodes].sort(),
    valid_image_status: validImage.status,
    invalid_image_error_codes: [...invalidImageCodes].sort()
  };
}

function checkInvestigativeSpineContract() {
  const workflow = readText("plugins/analytix-fund-analysis/references/analytix-workflow-context.md");
  const pair = readText("plugins/analytix-fund-analysis/skills/pair-amount-investigation/SKILL.md");
  const deliveryQc = readText("plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md");
  return {
    workflow_spine_marker_count: assertMarkers("workflow delivery spine", workflow, [
      "What was checked",
      "What conclusion is supported",
      "What is abnormal",
      "Why it matters to the case",
      "What still cannot be determined",
      "What material should be obtained next"
    ]),
    pair_spine_marker_count: assertMarkers("pair amount investigative spine", pair, [
      "结论先行",
      "转账结构研判",
      "not a request for a reusable amount-audit template",
      "异常特征",
      "案件意义",
      "暂不能认定",
      "下一步取证"
    ]),
    delivery_spine_marker_count: assertMarkers("delivery QC spine", deliveryQc, [
      "查什么",
      "案件意义是什么",
      "下一步调什么材料"
    ])
  };
}

function checkProfessionalJudgmentContract() {
  const deliveryQc = readText("plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md");
  const critique = readText("plugins/analytix-fund-analysis/skills/analysis-critique/SKILL.md");
  const shared = readText("plugins/analytix-fund-analysis/references/focused-skill-shared.md");
  return {
    delivery_qc_judgment_markers: assertMarkers("delivery QC professional judgment", deliveryQc, [
      "gives only figures",
      "lacks a table",
      "omits abnormal features or case significance",
      "下一步调什么材料"
    ]),
    critique_judgment_markers: assertMarkers("analysis critique professional judgment", critique, [
      "no table",
      "no abnormal-feature scan",
      "no case significance",
      "no next investigative action",
      "support card"
    ]),
    shared_judgment_markers: assertMarkers("shared professional judgment anti-patterns", shared, [
      "investigative judgment",
      "abnormal",
      "case significance",
      "next proof"
    ])
  };
}

async function checkModelEvalStandardContract() {
  const modelEval = await import("./model-ab-eval.mjs");
  const modelEvalSource = readText("plugins/analytix-fund-analysis/scripts/model-ab-eval.mjs");
  const tasks = Array.isArray(modelEval.TASKS) ? modelEval.TASKS : [];
  const modes = Array.isArray(modelEval.MODES) ? modelEval.MODES : [];
  const forbiddenSuccessMarkers = [
    "金额核验小研判",
    "金额口径",
    "口径差异说明",
    "口径提示",
    "全期间同名收款人",
    "全期间同名收款人口径",
    "同名收款人合计",
    "重点收款账号统计",
    "重点收款账户核验",
    "三个观察角度",
    "核心窗口口径",
    "核心账户/核心集中交易",
    "当前案件可见",
    "Case Workbench",
    "Workbench",
    "MCP",
    "claim",
    "write_blocked",
    "mandatory_review_cards",
    "source_audit",
    "unsupported",
    "evidence status",
    "feature family",
    "supporting facts",
    "downgrade reason",
    "next proof",
    "validate_continuation_list",
    "q_xxx",
    "audit_ref",
    "artifact_id"
  ];
  const forbiddenPromptMarkers = [
    "金额核验小研判",
    "金额口径",
    "口径差异说明",
    "口径提示",
    "全期间同名收款人",
    "全期间同名收款人口径",
    "同名收款人合计",
    "重点收款账号统计",
    "重点收款账户核验",
    "三个观察角度",
    "核心窗口口径",
    "核心账户/核心集中交易",
    "Case Workbench",
    "Workbench",
    "MCP",
    "claim QA",
    "write_blocked",
    "mandatory_review_cards",
    "source_audit",
    "unsupported flow",
    "feature family",
    "supporting facts",
    "downgrade reason",
    "next proof",
    "q_xxx",
    "audit_ref",
    "artifact_id"
  ];
  const badExpectedMarkers = [];
  for (const task of tasks) {
    for (const marker of task.expectedMarkers || []) {
      const hit = forbiddenSuccessMarkers.find((forbidden) => String(marker).includes(forbidden));
      if (hit) badExpectedMarkers.push(`${task.id}: ${marker}`);
    }
  }
  assert(badExpectedMarkers.length === 0, `model-ab-eval rewards old/internal success markers: ${badExpectedMarkers.join("; ")}`);

  const badPromptMarkers = [];
  for (const task of tasks) {
    const prompt = String(task.prompt || "");
    const title = String(task.title || "");
    for (const forbidden of forbiddenPromptMarkers) {
      if (prompt.includes(forbidden) || title.includes(forbidden)) {
        badPromptMarkers.push(`${task.id}: ${forbidden}`);
      }
    }
  }
  for (const mode of modes) {
    const instruction = `${mode.label || ""} ${mode.instruction || ""}`;
    for (const forbidden of ["Case Workbench", "Workbench", "MCP", "claim review"]) {
      if (instruction.includes(forbidden)) badPromptMarkers.push(`mode:${mode.id}: ${forbidden}`);
    }
  }
  assert(badPromptMarkers.length === 0, `model-ab-eval still prompts with old/internal wording: ${badPromptMarkers.join("; ")}`);

  const forbiddenPositiveScoringMarkers = [
    "Case Workbench",
    "Workbench",
    "MCP",
    "claim",
    "write_blocked",
    "report gate",
    "mandatory_review_cards",
    "source_audit",
    "unsupported",
    "supported edge",
    "supported transaction edge",
    "evidence status",
    "feature family",
    "supporting facts",
    "downgrade reason",
    "next proof",
    "fact owner",
    "owner skill",
    "validate_continuation_list",
    "Mermaid",
    "同名收款人合计",
    "三个观察角度",
    "核心窗口口径",
    "核心账户/核心集中交易",
    "金额核验小研判",
    "金额口径",
    "口径差异说明",
    "口径提示",
    "全期间同名收款人",
    "重点收款账号统计",
    "重点收款账户核验"
  ];
  const positiveRewardSnippets = [];
  const scoringArrayRegex = /Math\.min\([^)]*countHits\(answer,\s*\[([\s\S]*?)\]\)/gu;
  for (const match of modelEvalSource.matchAll(scoringArrayRegex)) {
    const snippet = match[1];
    const hit = forbiddenPositiveScoringMarkers.find((marker) => snippet.includes(marker));
    if (hit) {
      positiveRewardSnippets.push(snippet.replace(/\s+/gu, " ").slice(0, 180));
    }
  }
  assert(positiveRewardSnippets.length === 0, `model-ab-eval positive scoring still rewards old/internal wording: ${positiveRewardSnippets.join("; ")}`);

  return {
    checked_tasks: tasks.length,
    checked_modes: modes.length,
    forbidden_success_marker_count: forbiddenSuccessMarkers.length,
    forbidden_prompt_marker_count: forbiddenPromptMarkers.length,
    forbidden_positive_scoring_marker_count: forbiddenPositiveScoringMarkers.length,
    professional_eval_standard: true
  };
}

function checkPairAmountMiniInvestigationSmoke() {
  const pair = readText("plugins/analytix-fund-analysis/skills/pair-amount-investigation/SKILL.md");
  const compiler = readText("plugins/analytix-fund-analysis/mcp/agent-output-compiler.mjs");
  const cardRenderer = readText("plugins/analytix-fund-analysis/mcp/card-renderer.mjs");
  const frontdoor = readText("plugins/analytix-fund-analysis/mcp/frontdoor-runtime.mjs");
  const commandMetadata = readText("plugins/analytix-fund-analysis/references/command-metadata.json");
  const forbiddenSupportTitles = [
    "同名收款人聚合支持",
    "重点收款账户支持",
    "金额核验小研判",
    "口径差异说明",
    "全期间同名收款人统计",
    "重点收款账号统计",
    "重点收款账户核验"
  ];
  for (const [label, body] of Object.entries({ compiler, cardRenderer, commandMetadata })) {
    for (const marker of forbiddenSupportTitles) {
      assert(!body.includes(marker), `${label} still rewards old Pair Amount support-card wording: ${marker}`);
    }
  }
  assert(!frontdoor.includes("answer_draft"), "Pair Amount frontdoor support must not generate answer_draft");
  assert(frontdoor.includes("dedup_key_safety"), "Pair Amount support must expose dedup-key safety structure");
  assert(frontdoor.includes("duplicate_or_unsupported_amount"), "Pair Amount support must expose duplicate/evidence-insufficient amount");
  assert(frontdoor.includes("top_transactions"), "Pair Amount support must expose Top transaction candidates for the lead owner table");
  assert(frontdoor.includes("full_period_pair_amount_with_focus_cluster"), "Pair Amount support must keep full pair detail as the unrestricted controlling scope when a focused concentration exists");
  assert(frontdoor.includes("PAIR_AMOUNT_DETAIL_AGGREGATE_DISAGREEMENT"), "Pair Amount support must flag detail-vs-rank disagreements without shrinking to a focused concentration");
  assert(frontdoor.includes("PAIR_AMOUNT_FOCUS_CLUSTER_NOT_TOTAL"), "Pair Amount support must mark focused concentrations as abnormal clues, not unrestricted totals");
  assert(frontdoor.includes("rank_cross_check_amount"), "Pair Amount runtime may retain rank cross-check internally for audit");
  assert(compiler.includes('"top_transactions"'), "support envelope must retain Pair Amount Top transaction candidates");
  assert(compiler.includes('"effective_stat_basis"'), "support envelope must retain Pair Amount effective-stat basis");
  assert(compiler.includes('"first_txn_at"') && compiler.includes('"last_txn_at"'), "support envelope must retain Pair Amount statistical period scalars");
  assert(compiler.includes('"full_period_effective_amount"'), "support envelope must retain full pair amount scalar");
  assert(compiler.includes("publicPairAmountSupportKey"), "support envelope must translate internal focus-cluster fields before exposing model facts");
  assert(compiler.includes('"major_concentrated_transfer_role"'), "support envelope must retain focused concentration role with business-readable wording");
  assert(compiler.includes('"receiver_account_groups"') && compiler.includes('"time_concentrations"'), "support envelope must expose account/time groupings with economic-investigation wording");
  return {
    pair_owner_markers: assertMarkers("pair amount mini investigation owner", pair, [
      "结论先行",
      "重点交易表",
      "账户集合",
      "time concentrations",
      "转账结构研判",
      "集中交易以外往来",
	      "异常特征",
	      "案件意义",
	      "暂不能认定",
	      "本次依据",
	      "统计范围",
	      "核验意见",
	      "Do not use `口径`, `核验口径`",
	      "actual statistical period",
	      "下一步取证"
    ]),
    support_hidden_markers: assertMarkers("pair amount support hidden markers", compiler + "\n" + cardRenderer + "\n" + commandMetadata, [
      "support_only",
      "final_answer_owned_by_focused_skill",
      "source_to_counterparty_amount_card is support evidence only",
      "pair_amount_structure_review",
      "pair_amount_fact_pack",
      "renderSupportEnvelopeForAgent",
      "large-transaction table",
      "difference explanation",
      "差异说明",
      "当前不能认定",
      "relationship/purpose leads"
	    ])
  };
}

function checkHeroDeliveryContract() {
  const graph = readText("plugins/analytix-fund-analysis/skills/graph-visualization/SKILL.md");
  const visual = readText("plugins/analytix-fund-analysis/skills/visual-evidence/SKILL.md");
  const report = readText("plugins/analytix-fund-analysis/skills/report-builder/SKILL.md");
  const evidenceRequest = readText("plugins/analytix-fund-analysis/skills/evidence-request/SKILL.md");
  return {
    graph_hero_markers: assertMarkers("graph hero deliverable", graph, [
      "资金流向图",
      "compact table",
      "funds break",
      "待补证事项"
    ]),
    visual_hero_markers: assertMarkers("visual hero deliverable", visual, [
      "table, chart",
      "Mermaid, PNG, JPG",
      "delivered file or explicit blocker",
      "inspect the delivered surface"
    ]),
    report_hero_markers: assertMarkers("report hero deliverable", report, [
      "selected report mode",
      "materialized draft or explicit delivery gap",
      "preserve the existing report structure",
      "proof"
    ]),
    evidence_request_markers: assertMarkers("evidence request hero deliverable", evidenceRequest, [
      "补调",
      "证据缺口",
      "调证",
      "证明"
    ])
  };
}

function checkDesktopMultiturnInvestigationContract() {
  const frontdoorSmoke = readText("plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs");
  const desktopAcceptance = readText("plugins/analytix-fund-analysis/scripts/desktop-multiturn-acceptance.mjs");
  const workflow = readText("plugins/analytix-fund-analysis/references/analytix-workflow-context.md");
  const report = readText("plugins/analytix-fund-analysis/skills/report-builder/SKILL.md");
  const scenarioBlockStart = frontdoorSmoke.indexOf("const DESKTOP_MULTITURN_ACCEPTANCE_SCENARIOS");
  const scenarioBlockEnd = frontdoorSmoke.indexOf("function desktopScenarioEnvName");
  const scenarioBlock = scenarioBlockStart >= 0 && scenarioBlockEnd > scenarioBlockStart
    ? frontdoorSmoke.slice(scenarioBlockStart, scenarioBlockEnd)
    : "";
  assert(scenarioBlock, "desktop multi-turn scenario block is missing");
  assert(!/(合成主体甲|合成主体乙|42,000,000|42000000)/u.test(scenarioBlock), "desktop multi-turn scenarios must not hardcode real case facts");
  assert(!/(合成主体甲|合成主体乙|42,000,000|42000000)/u.test(desktopAcceptance), "desktop Electron acceptance runner must not hardcode real case facts");
  return {
    real_frontdoor_hooks: assertMarkers("desktop real-case smoke hooks", frontdoorSmoke, [
      "real_pair_amount_review",
      "real_p1_frontdoor",
      "ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_MARKERS",
      "ANALYTIX_FUNDS_REAL_P1_FORBIDDEN_MARKERS",
      "zhangjinzhi_continuation",
      "outflow_continuation"
    ]),
    desktop_acceptance_suite_markers: assertMarkers("desktop 9-scenario acceptance suite", frontdoorSmoke, [
      "DESKTOP_MULTITURN_ACCEPTANCE_SCENARIOS",
      "ANALYTIX_FUNDS_DESKTOP_MULTITURN_ENABLED",
      "ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID",
      "ANALYTIX_FUNDS_DESKTOP_MULTITURN_REQUIRE_MARKERS",
      "desktop_pair_amount_material",
      "desktop_flow_graph_material",
      "desktop_subject_account_dossier",
      "desktop_downstream_continuation",
      "desktop_cleaned_export_evidence",
      "desktop_blank_counterparty_review",
      "desktop_report_continuation",
      "desktop_full_case_analysis",
      "desktop_project_related_asset_probe",
      "ANALYTIX_FUNDS_DESKTOP_PAIR_AMOUNT_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_FLOW_GRAPH_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_SUBJECT_DOSSIER_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_DOWNSTREAM_TRACE_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_CLEANED_EXPORT_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_BLANK_COUNTERPARTY_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_REPORT_CONTINUATION_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_FULL_CASE_ANALYSIS_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_PROJECT_RELATED_ASSET_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_PROJECT_RELATED_ASSET_MARKERS",
      "经梳理",
      "期间",
      "笔数",
      "金额",
      "大额交易",
      "异常",
      "资金意义",
      "补证",
      "暂不能认定"
    ]),
    real_electron_acceptance_runner_markers: assertMarkers("desktop Electron multi-turn acceptance runner", desktopAcceptance, [
      "agentThreadStart",
      "sendMessageFromView",
      "vscode://codex/turn/start",
      "thread/read",
      "ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID",
      "ANALYTIX_FUNDS_DESKTOP_PAIR_AMOUNT_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_FLOW_GRAPH_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_SUBJECT_DOSSIER_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_DOWNSTREAM_TRACE_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_CLEANED_EXPORT_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_BLANK_COUNTERPARTY_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_REPORT_CONTINUATION_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_FULL_CASE_ANALYSIS_QUESTION",
      "ANALYTIX_FUNDS_DESKTOP_PROJECT_RELATED_ASSET_QUESTION",
      "userVisibleLeakageLabels",
      "FORBIDDEN_VISIBLE_PATTERNS",
      "requiredKinds",
      "desktop_pair_amount_material",
      "desktop_flow_graph_material",
      "desktop_subject_account_dossier",
      "desktop_downstream_continuation",
      "desktop_cleaned_export_evidence",
      "desktop_blank_counterparty_review",
      "desktop_report_continuation",
      "desktop_full_case_analysis",
      "desktop_project_related_asset_probe"
    ]),
    continuation_context_markers: assertMarkers("workflow continuation context", workflow, [
      "multi-turn continuation",
      "Report continuation",
      "Attachment directory",
      "Graph and image delivery"
    ]),
    report_continuation_markers: assertMarkers("report continuation desktop path", report, [
      "continue an existing report",
      "full-report amount verification",
      "verifying the modified section"
    ])
  };
}

function checkCaseMemoPlanContract() {
  const report = readText("plugins/analytix-fund-analysis/skills/report-builder/SKILL.md");
  const reportSchema = readText("plugins/analytix-fund-analysis/references/report-schema.md");
  return {
    report_builder_marker_count: assertMarkers("case memo plan report builder", report, [
      "selected report mode",
      "full-case briefing",
      "one-person/one-company dossier",
      "project/corruption/bid-topic report",
      "single-lead verification memo",
      "proof"
    ]),
    report_schema_marker_count: assertMarkers("case memo plan schema", reportSchema, [
      "基本情况",
      "资金流入流出",
      "重点对手方",
      "异常特征",
      "核验意见",
      "补证建议"
    ])
  };
}

function checkEvidenceMaturityLanguageContract() {
  const workflow = readText("plugins/analytix-fund-analysis/references/analytix-workflow-context.md");
  const deliveryQc = readText("plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md");
  const userFacingLanguage = readText("plugins/analytix-fund-analysis/mcp/user-facing-language.mjs");
  return {
    workflow_marker_count: assertMarkers("workflow evidence maturity", workflow, [
      "已有流水支持",
      "高可信支持",
      "线索",
      "需复核",
      "暂不能认定",
      "可形成材料候选"
    ]),
    delivery_qc_marker_count: assertMarkers("delivery QC evidence maturity", deliveryQc, [
      "已有流水支持",
      "高可信支持",
      "线索",
      "需复核",
      "暂不能认定",
      "可形成材料候选"
    ]),
    language_guard_marker_count: assertMarkers("user facing language guard", userFacingLanguage, [
      "USER_VISIBLE_FORBIDDEN_PATTERNS",
      "case_id",
      "workflow",
      "support layer"
    ])
  };
}

function checkPublicSecurityMaterialContract() {
  const publicWriting = readText("plugins/analytix-fund-analysis/references/public-security-official-writing.md");
  const embeddedPublicWriting = readText("plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/public-security-official-writing.md");
  const report = readText("plugins/analytix-fund-analysis/skills/report-builder/SKILL.md");
  const claimReview = readText("plugins/analytix-fund-analysis/skills/claim-review/SKILL.md");
  const deliveryQc = readText("plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md");
  const evidenceRequest = readText("plugins/analytix-fund-analysis/skills/evidence-request/SKILL.md");
  const visual = readText("plugins/analytix-fund-analysis/skills/visual-evidence/SKILL.md");
  const fullCase = readText("plugins/analytix-fund-analysis/skills/full-case-analysis/SKILL.md");
  const claimVerifier = readText("plugins/analytix-fund-analysis/mcp/claim-verifier-protocol.mjs");
  const toolSchemas = readText("plugins/analytix-fund-analysis/mcp/tool-schemas.mjs");
  const reportSchema = readText("plugins/analytix-fund-analysis/references/report-schema.md");
  const embeddedReportSchema = readText("plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/report-schema.md");
  assert(publicWriting === embeddedPublicWriting, "public-security-official-writing root and embedded copies must match");
  assert(reportSchema === embeddedReportSchema, "report-schema root and embedded copies must match");
  return {
    public_writing_markers: assertMarkers("public-security official writing reference", publicWriting, [
      "## 材料类型",
      "## 篇章顺序",
      "## 证据成熟度",
      "## 法律敏感降级",
      "## 表后研判",
      "异常",
      "证明价值",
      "现有材料还不能证明",
      "下一步调取"
    ]),
    owner_skill_markers: assertMarkers("public-security material owner skills", [
      report,
      claimReview,
      deliveryQc,
      evidenceRequest,
      visual,
      fullCase
    ].join("\n"), [
      "public-security-official-writing",
      "表后研判",
      "证明价值",
      "法律敏感",
      "异常特征",
      "暂不能认定",
      "下一步"
    ]),
    claim_verifier_markers: assertMarkers("claim verifier table-after-analysis gate", claimVerifier, [
      "hasMarkdownTableWithoutAnalysis",
      "table_without_analysis",
      "表后未接研判意见",
      "不能用表格或附件目录替代",
      "异常特征、证明价值、仍不能认定事项和下一步调取材料"
    ]),
    structured_report_gate_markers: assertMarkers("structured report/claim gate", [
      report,
      claimReview,
      reportSchema,
      claimVerifier,
      toolSchemas
    ].join("\n"), [
      "FinalInvestigationAnswer",
      "final_investigation_answer",
      "validate_report_claims",
      "investigation_answer_contract_failed",
      "audit_events",
      "missing source refs",
      "unsupported path edges",
      "legal upgrades",
      "proof actions"
    ])
  };
}

function checkBlueprint14_2MatrixContract() {
  const files = [
    "plugins/analytix-fund-analysis/references/blueprint-execution-plan.md",
    "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/blueprint-execution-plan.md",
    "plugins/analytix-fund-analysis/references/top-pluginization-plan.md",
    "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/top-pluginization-plan.md"
  ];
  const requiredMarkers = [
    "mature principle",
    "Analytix 落点",
    "parity/gap",
    "经侦增强",
    "证据文件",
    "release blocker",
    "Data Analytics：多 skill / source guardrail / 核验状态 / 交付要求",
    "Impeccable",
    "CodeGraph",
    "Superpowers",
    "oh-my-codex",
    "mcp/user-facing-language.mjs",
    "mcp/frontdoor-runtime.mjs",
    "scripts/functional-eval.mjs",
    "scripts/frontdoor-smoke.mjs",
    "runtime-boundary.md",
    "用户只见经侦结论",
    "review pass",
    "P1 env smoke outputs"
  ];
  const results = {};
  for (const file of files) {
    const body = readText(file);
    results[file] = {
      marker_count: assertMarkers(file, body, requiredMarkers),
      conditional_blocker_count: (body.match(/Conditional/gu) || []).length,
      no_blocker_count: (body.match(/\| No/gu) || []).length
    };
    assert(results[file].conditional_blocker_count >= 3, `${file} must identify conditional release blockers`);
    assert(results[file].no_blocker_count >= 2, `${file} must identify non-blocking mechanisms`);
  }
  return results;
}

function checkMcpDecentralizationContract() {
  const schemas = new Map(MCP_TOOL_SCHEMAS.map((tool) => [tool.name, tool]));
  const fundsDescription = text(objectOf(schemas.get("funds_investigate")).description);
  const rankHoldersDescription = text(objectOf(schemas.get("rank_holders")).description);
  const rankCounterpartiesDescription = text(objectOf(schemas.get("rank_counterparties")).description);
  const flowGraphDescription = text(objectOf(schemas.get("build_fund_flow_graph")).description);
  assert(fundsDescription.includes("当前案件资金问题的自然语言事实核验入口"), "funds_investigate must describe itself as the current-case natural-language fact verifier");
  assert(fundsDescription.includes("姓名/主体是否和案件资金有关联"), "funds_investigate must expose the name-association quick fact route");
  assert(fundsDescription.includes("严格命中/模糊命中/排除对象/不能确认"), "funds_investigate must describe the four name-association buckets");
  assert(fundsDescription.includes("A 转给 B 多少钱"), "funds_investigate must name the pair-amount route");
  assert(fundsDescription.includes("不得为了当前案件事实先搜索工作区文件"), "funds_investigate must prevent file exploration for current-case facts");
  assert(fundsDescription.includes("Shell"), "funds_investigate must explicitly prevent shell exploration for current-case facts");
  assert(fundsDescription.includes("明确 Top/排行"), "ordinary explicit ranking tasks must still choose targeted tools directly");
  assert(rankHoldersDescription.includes("不要把本工具作为") && rankHoldersDescription.includes("关联快查入口"), "rank_holders must not claim name-association quick facts");
  assert(rankHoldersDescription.includes("资金研判入口") && rankHoldersDescription.includes("资金事实快查路线"), "rank_holders must point name-association questions to funds_investigate");
  assert(rankCounterpartiesDescription.includes("不要把本工具作为") && rankCounterpartiesDescription.includes("关联快查入口"), "rank_counterparties must not claim name-association quick facts");
  assert(rankCounterpartiesDescription.includes("资金研判入口") && rankCounterpartiesDescription.includes("资金事实快查路线"), "rank_counterparties must point name-association questions to funds_investigate");
  assert(!flowGraphDescription.includes("Mermaid"), "build_fund_flow_graph description must not ask the model to expose Mermaid");
  assert(!flowGraphDescription.includes("needs_review"), "build_fund_flow_graph description must not expose internal status labels");

  const toolDiscovery = readText("plugins/analytix-fund-analysis/mcp/tool-discovery-policy.mjs");
  const registryRuntime = readText("plugins/analytix-fund-analysis/mcp/capability-registry-runtime.mjs");
  const registry = readJson("plugins/analytix-fund-analysis/references/capability-registry.json");
  const contract = objectOf(registry.tool_discovery_contract);
  const defaultTools = arrayOf(contract.default_visible_tools).map(text).filter(Boolean);
  const requiredSourceBackedTools = [
    "get_current_case",
    "investigate_pair_amount",
    "get_case_scope_map",
    "get_casegraph",
    "rank_counterparties",
    "analyze_account_full",
    "analyze_holder_full",
    "trace_subject_top_outflows",
    "hypothesis_probe",
    "audit_case_data_quality",
    "validate_report_claims",
    "run_case_sql"
  ];
  const missingDefault = requiredSourceBackedTools.filter((tool) => !defaultTools.includes(tool));
  assert(missingDefault.length === 0, `default source-backed toolbox missing: ${missingDefault.join(", ")}`);
  const p0HiddenTools = [
    "create_case_notebook",
    "export_cleaned_case_data",
    "run_full_case_analysis"
  ];
  const unexpectedlyVisible = p0HiddenTools.filter((tool) => defaultTools.includes(tool));
  assert(unexpectedlyVisible.length === 0, `P0 write/full-case tools must stay hidden: ${unexpectedlyVisible.join(", ")}`);
  assert(toolDiscovery.includes("from \"./capability-registry-runtime.mjs\""), "tool discovery must read registry runtime");
  assert(toolDiscovery.includes("orderedToolsForAgent"), "tools/list must use orderedToolsForAgent");
  assert(!/TOOL_LIST_PRIORITY\s*=\s*\[/u.test(toolDiscovery), "tool discovery must not own a hardcoded priority list");
  assert(!toolDiscovery.includes(".slice(0, MAX_DEFAULT_VISIBLE_TOOLS)"), "tool discovery must not truncate default toolbox by slice");
  assert(registryRuntime.includes("readCapabilityRegistry"), "capability registry runtime reader missing");
  return {
    funds_investigate_role: "frontdoor_with_name_association_and_pair_amount_routes",
    default_source_backed_tool_count: defaultTools.length,
    required_source_backed_tool_count: requiredSourceBackedTools.length,
    p0_hidden_tool_count: p0HiddenTools.length,
    registry_backed_discovery: true
  };
}

async function checkCaseWorkbenchControlContract() {
  const runSqlSchema = schemaFor("run_case_sql");
  const notebookSchema = schemaFor("create_case_notebook");
  const releaseSliceAudit = readText("plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs");
  const toolSchemasSource = readText("plugins/analytix-fund-analysis/mcp/tool-schemas.mjs");
  const runtimeSource = readText("plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs");
  const workbenchRuntime = readText("plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs");
  const evidenceLedger = readText("plugins/analytix-fund-analysis/mcp/evidence-ledger.mjs");
  const caseWorkbench = readText("plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md");
  const mcpRuntimeContract = [
    toolSchemasSource,
    runtimeSource,
    workbenchRuntime,
    evidenceLedger,
    caseWorkbench
  ].join("\n");
  const runSqlProperties = objectOf(runSqlSchema.properties);
  const notebookProperties = objectOf(notebookSchema.properties);
  assert(arrayOf(runSqlSchema.required).includes("purpose"), "run_case_sql must require purpose");
  assert(!arrayOf(runSqlSchema.required).includes("case_id"), "run_case_sql may omit case_id only when the host injects a frozen case context");
  assert(Number(objectOf(runSqlProperties.row_limit).maximum) === 500, "run_case_sql row_limit maximum must be 500");
  assert(arrayOf(objectOf(runSqlProperties.allowed_view_policy).enum).join(",") === "cleaned_and_analysis_only", "run_case_sql allowed view policy must be cleaned_and_analysis_only only");
  assert(arrayOf(notebookSchema.required).includes("analysis_goal"), "create_case_notebook must require analysis_goal");
  assert(!arrayOf(notebookSchema.required).includes("case_id"), "create_case_notebook may omit case_id only when the host injects a frozen case context");
  assert(!/\/api\/v1\/cases\/active|backend_active_case/u.test(mcpRuntimeContract), "case workbench must not resolve facts through a backend active-case fallback");
  assert(Number(objectOf(notebookProperties.max_rows_per_query).maximum) === 500, "notebook max_rows_per_query maximum must be 500");
  assert(arrayOf(objectOf(notebookProperties.allowed_view_policy).enum).join(",") === "cleaned_and_analysis_only", "notebook allowed view policy must be cleaned_and_analysis_only only");
  assertMarkers("inspect_case_schema MCP/runtime SQL-safe contract", mcpRuntimeContract, [
    "sql_name_contract",
    "tables[].sql_name",
    "tables[].columns[].sql_name/sql_identifier",
    "sql_identifier",
    "display_name is UI-only",
    "display_name 只用于展示"
  ]);
  assertMarkers("run_case_sql MCP/runtime evidence card contract", mcpRuntimeContract, [
    "case_workbench_evidence_card",
    "source_hash",
    "metric_scope",
    "validation_state",
    "evidence_card"
  ]);
  assertMarkers("release slice MCP/runtime contract", releaseSliceAudit, [
    "plugins/analytix-fund-analysis/mcp/tool-schemas.mjs",
    "plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs",
    "plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs",
    "plugins/analytix-fund-analysis/mcp/evidence-ledger.mjs"
  ]);
  assertMarkers("controlled workbench runtime", runtimeSource, [
    "CONTROLLED_CASE_WORKBENCH_TOOLS",
    "case_workbench_evidence_card",
    "runCaseSqlEvidenceCard",
    "source_hash",
    "metric_scope",
    "validation_state",
    "DDL/DML/extension/export statements are not allowed",
    "raw/source tables are not allowed",
    "external file/network/secret access is not allowed",
    "SQL must be complete",
    "allowed_view_policy = \"cleaned_and_analysis_only\"",
    "Math.min(Number.isFinite(rowLimit) ? rowLimit : 100, 500)",
    "CONTROLLED_CASE_WORKBENCH_UNAVAILABLE"
  ]);

  const { createToolCallRuntime } = await import("../mcp/tool-call-runtime.mjs");
  const {
    createMcpRequestHandlerRuntime,
    validateToolInput,
  } = await import("../mcp/mcp-request-handler-runtime.mjs");
  const authorityRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-functional-host-context-"));
  process.on("exit", () => fs.rmSync(authorityRoot, { recursive: true, force: true }));
  const authorityProjectRoot = path.join(authorityRoot, "case-project");
  fs.mkdirSync(authorityProjectRoot, { recursive: true });
  const authorityBinding = writeCaseProjectBinding(authorityProjectRoot, "case_functional_eval");
  const authorityFor = (name, args) => hostFactRuntimeContext({
    ...authorityBinding,
    caseId: "case_functional_eval",
    toolName: name,
    args,
    serverVersion: "functional-eval"
  });
  let observedHandlerArgs = null;
  const requestHandler = createMcpRequestHandlerRuntime({
    serverName: "analytix_funds",
    serverVersion: "functional-eval",
    pluginRoot: PLUGIN_ROOT,
    env: { ANALYTIX_CASE_PROJECT_ROOT: authorityProjectRoot },
    callTool: async (_name, args) => {
      observedHandlerArgs = args;
      return { status: "blocked" };
    },
    toolResult: () => ({
      structuredContent: {
        transportStatus: "success",
        semanticStatus: "blocked",
        safeToAnswer: false,
        isError: true,
        blocker: "functional_eval_boundary",
        partialCoverage: {},
        data: {},
        candidateEvidenceReceipts: []
      }
    })
  });
  await requestHandler.handleRequest({
    method: "initialize",
    params: { protocolVersion: "2025-11-25", capabilities: {}, clientInfo: { name: "functional-eval", version: "1.0.0" } }
  });
  await requestHandler.handleNotification({ method: "notifications/initialized", params: {} });
  const runSqlTool = MCP_TOOL_SCHEMAS.find((tool) => tool.name === "run_case_sql");
  assert(runSqlTool, "run_case_sql schema must exist for input validation");
  let unknownFieldRejected = false;
  try {
    validateToolInput(runSqlTool, {
      purpose: "Reject unknown input fields",
      sql: "SELECT count(*) AS txn_count FROM analysis_txn_detail_idx",
      duckdb_path: "/tmp/cross-case.duckdb"
    });
  } catch (error) {
    unknownFieldRejected = Number(error?.code) === -32602 && String(error?.message || error).includes("not allowed");
  }
  assert(unknownFieldRejected, "run_case_sql schema validation must reject unknown input fields such as duckdb_path");

  let unadvertisedRejected = false;
  try {
    await requestHandler.handleRequest({
      method: "tools/call",
      params: {
        name: "run_case_sql",
        arguments: {
          purpose: "Reject unknown input fields",
          sql: "SELECT count(*) AS txn_count FROM analysis_txn_detail_idx",
          duckdb_path: "/tmp/cross-case.duckdb"
        }
      }
    });
  } catch (error) {
    unadvertisedRejected = Number(error?.code) === -32602
      && String(error?.message || error).includes("Unknown or unadvertised tool");
  }
  assert(
    unadvertisedRejected && observedHandlerArgs === null,
    "P0 MCP request handler must reject run_case_sql at the shared advertisement/execution allowlist before dispatch"
  );
  const sqlArguments = {
    purpose: "Allow host-injected current-workspace context",
    sql: "SELECT count(*) AS txn_count FROM analysis_txn_detail_idx"
  };
  const expectP0Unadvertised = async (name, args) => {
    try {
      await requestHandler.handleRequest({
        method: "tools/call",
        params: {
          name,
          arguments: args,
          _meta: { analytixRuntimeContext: authorityFor(name, args) }
        }
      });
      return false;
    } catch (error) {
      return Number(error?.code) === -32602
        && String(error?.message || error).includes("Unknown or unadvertised tool");
    }
  };
  assert(
    await expectP0Unadvertised("run_case_sql", sqlArguments),
    "P0 MCP request handler must not execute a valid hidden run_case_sql call"
  );
  const pairArguments = {
    question: "A 转给 B 多少钱？",
    holder_name: "A",
    counterparty_name: "B"
  };
  assert(
    await expectP0Unadvertised("investigate_pair_amount", pairArguments)
      && observedHandlerArgs === null,
    "P0 MCP request handler must not execute a valid hidden pair-amount call",
  );
  const runtime = createToolCallRuntime({
    executeSkill: async (_name, args) => ({ observed_args: args, rows: [{ supported_amount: 1 }] })
  });
  const noPurpose = await runtime.callTool("run_case_sql", { sql: "SELECT 1 AS n" });
  assert(noPurpose.status === "blocked", "run_case_sql without purpose must be blocked");
  const rawRead = await runtime.callTool("run_case_sql", {
    purpose: "Verify raw table block",
    sql: "SELECT count(*) AS n FROM fc_transaction_raw"
  });
  assert(rawRead.status === "blocked", "run_case_sql raw table access must be blocked");
  const ddl = await runtime.callTool("run_case_sql", {
    purpose: "Verify write block",
    sql: "DROP TABLE analysis_txn_detail_idx"
  });
  assert(ddl.status === "blocked", "run_case_sql DDL/DML must be blocked");
  const ok = await runtime.callTool("run_case_sql", {
    purpose: "Targeted aggregate over cleaned analysis scope after preflight",
    sql: "SELECT count(*) AS txn_count FROM analysis_txn_detail_idx",
    row_limit: 5000
  });
  assert(ok.status === "ok", "valid cleaned aggregate may reach the internal read-only executor");
  assert(ok.response?.observed_args?.row_limit === 500, "row_limit must be clamped to 500");
  assert(ok.response?.observed_args?.allowed_view_policy === "cleaned_and_analysis_only", "allowed_view_policy must be enforced");
  assert(ok.evidence_card?.card_type === "case_workbench_evidence_card", "run_case_sql ok path must expose evidence_card");
  assert(!ok.response?.data?.source_hash, "run_case_sql must not synthesize source_hash when the backend omitted it");
  assert(ok.evidence_card?.support_status === "unsupported", "run_case_sql without a host receipt must remain unsupported");
  assert(ok.evidence_card?.fact_answer_allowed === false, "run_case_sql transport success must not enable fact answers");
  assert(arrayOf(ok.evidence_card?.missing_fields).includes("host_evidence_receipt"), "run_case_sql must expose the missing host receipt boundary");
  assert(ok.response?.data?.metric_scope?.allowed_view_policy === "cleaned_and_analysis_only", "run_case_sql ok path must propagate metric_scope");
  assert(ok.response?.data?.validation_state?.current_case_only === true, "run_case_sql ok path must propagate validation_state");
  const rankTopN = await runtime.callTool("rank_holders", {
    metric: "turnover",
    top_n: 15
  });
  assert(rankTopN.response?.observed_args?.limit === 15, "rank_holders top_n must preserve user requested Top-N");
  const rankRequestedLimit = await runtime.callTool("rank_counterparties", {
    metric: "outflow",
    requested_limit: 20
  });
  assert(rankRequestedLimit.response?.observed_args?.limit === 20, "rank_counterparties requested_limit must preserve user requested Top-N");
  const pairRuntime = createToolCallRuntime({
    fundsInvestigate: async (args) => ({ observed_args: args })
  });
  const pairAliasNormalized = await pairRuntime.callTool("investigate_pair_amount", {
    question: "江苏农垦土地资源开发有限公司转给江苏省农垦集团有限公司多少钱？",
    holder_name: "江苏农垦土地资源开发有限公司",
    counterparty_name: "江苏省农垦集团有限公司"
  });
  assert(pairAliasNormalized.observed_args?.payer_name === "江苏农垦土地资源开发有限公司", "investigate_pair_amount must normalize holder_name into payer_name");
  assert(pairAliasNormalized.observed_args?.receiver_name === "江苏省农垦集团有限公司", "investigate_pair_amount must normalize counterparty_name into receiver_name");
  const gapRuntime = createToolCallRuntime({
    executeSkill: async () => {
      throw new Error("backend capability not implemented");
    },
    env: {
      ...process.env,
      ANALYTIX_FUNDS_DISABLE_LOCAL_DUCKDB_FALLBACK: "1"
    }
  });
  const gap = await gapRuntime.callTool("run_case_sql", {
    purpose: "Verify capability gap",
    sql: "SELECT count(*) AS txn_count FROM analysis_txn_detail_idx"
  });
  assert(gap.status === "capability_gap", "backend failure must return capability_gap instead of fabricated facts");
  assert(arrayOf(gap.recommended_ladder).includes("run_case_sql"), "capability gap must include deterministic Workbench ladder");
  assert(gap.safe_to_answer_current_task === false, "capability gap must block final fact answering");
  return {
    schema_policy: "purpose, frozen host case context or explicit case_id, cleaned/analysis only, row_limit <= 500",
    runtime_blocks: ["missing purpose", "raw/source table", "DDL/DML", "external/file/network", "placeholder SQL"],
    runtime_boundary_path_verified: true,
    runtime_gap_path_verified: true,
    mcp_unknown_input_rejection_verified: true,
    pair_amount_declared_alias_contract_verified: true,
    top_n_alias_preservation_verified: true,
    workbench_ladder_contract_verified: true,
    schema_sql_name_contract: true,
    run_case_sql_evidence_card_contract: true
  };
}

async function checkSchemaSqlNameContract() {
  const caseWorkbench = readText("plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md");
  const toolRuntime = readText("plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs");
  const workbenchRuntime = readText("plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs");
  const toolSchemas = new Map(MCP_TOOL_SCHEMAS.map((tool) => [tool.name, tool]));
  const inspectDescription = text(objectOf(toolSchemas.get("inspect_case_schema")).description);
  const runSqlDescription = text(objectOf(toolSchemas.get("run_case_sql")).description);

  assertMarkers("inspect_case_schema SQL-safe runtime contract", workbenchRuntime, [
    "sql_name_contract",
    "sql_name: table.table_name",
    "sql_identifier: quoteIdentifier(table.table_name)",
    "display_name: table.table_name",
    "sql_name: columnName",
    "sql_identifier: quoteIdentifier(columnName)",
    "display_name: columnName",
    "display_name is UI-only"
  ]);
  assertMarkers("schema SQL-safe policies", `${inspectDescription}\n${runSqlDescription}\n${caseWorkbench}`, [
    "table.sql_name",
    "column.sql_name",
    "sql_identifier",
    "display_name 只用于展示",
    "Chinese `display_name` labels are UI-only",
    "Chinese display_name labels are UI-only and are not executable SQL"
  ]);
  assertMarkers("schema SQL-safe MCP descriptions", `${inspectDescription}\n${runSqlDescription}`, [
    "table.sql_name",
    "column.sql_name",
    "sql_identifier",
    "display_name",
    "not executable SQL",
    "不能写进可执行 SQL"
  ]);
  assertMarkers("workbench capability gap runtime", toolRuntime, [
    "CONTROLLED_CASE_WORKBENCH_UNAVAILABLE",
    "专项资金核算能力暂不可用",
    "不得编造自定义查询结果",
    "产品化为新的后端语义核验能力"
  ]);

  const { createToolCallRuntime } = await import("../mcp/tool-call-runtime.mjs");
  const gapRuntime = createToolCallRuntime({
    executeSkill: async () => {
      throw new Error("backend capability not implemented");
    },
    env: {
      ...process.env,
      ANALYTIX_FUNDS_DISABLE_LOCAL_DUCKDB_FALLBACK: "1"
    }
  });
  const gap = await gapRuntime.callTool("run_case_sql", {
    purpose: "Verify schema SQL-safe capability gap",
    sql: "SELECT count(*) AS txn_count FROM analysis_txn_detail_idx"
  });
  const gapText = JSON.stringify(gap);
  assert(gap.status === "capability_gap", "run_case_sql backend failures must return capability_gap");
  assert(gapText.includes("CONTROLLED_CASE_WORKBENCH_UNAVAILABLE"), "capability gap must expose the real Workbench unavailable reason");
  assert(!gapText.includes("funds_investigate"), "capability gap must not route back to funds_investigate drafting");

  return {
    inspect_case_schema_sql_safe_names: true,
    display_name_ui_only: true,
    run_case_sql_policy_uses_sql_name_or_identifier: true,
    capability_gap_does_not_fallback_to_funds_investigate: true
  };
}

function checkOutputLeakContract() {
  const renderer = readText("plugins/analytix-fund-analysis/mcp/card-renderer.mjs");
  const answerProtocol = readText("plugins/analytix-fund-analysis/mcp/answer-card-protocol.mjs");
  const frontdoorSmoke = readText("plugins/analytix-fund-analysis/scripts/frontdoor-smoke.mjs");
  const payloadCompiler = readText("plugins/analytix-fund-analysis/mcp/agent-payload-compiler.mjs");
  const userFacingLanguage = readText("plugins/analytix-fund-analysis/mcp/user-facing-language.mjs");
  const userVisibleSmoke = readText("plugins/analytix-fund-analysis/scripts/user-visible-language-smoke.mjs");
  return {
    renderer_filter_marker_count: assertMarkers("card renderer leak filter", renderer, [
      "answer_card_complete",
      "write_blocked",
      "diagnostic label",
      "doctor",
      "model-visible text receives only the compact Answer Card",
      "是否已留存须以宿主证据登记为准",
      "OPAQUE_REF_PATTERN"
    ]),
    answer_protocol_marker_count: assertMarkers("answer card protocol budget fields", answerProtocol, [
      "answer_card_complete",
      "max_additional_tools",
      "只抑制同一问题"
    ]),
    frontdoor_smoke_marker_count: assertMarkers("frontdoor smoke leak flags", frontdoorSmoke, [
      "--forbid-doc-leakage",
      "--forbid-report-gate-leakage",
      "--forbid-user-language-leakage",
      "DOC_LEAKAGE_MARKERS",
      "REPORT_GATE_LEAKAGE_MARKERS",
      "userVisibleLeakageLabels",
      "OPAQUE_REF_PATTERN"
    ]),
    user_language_marker_count: assertMarkers("user-facing language guard", userFacingLanguage, [
      "USER_VISIBLE_FORBIDDEN_PATTERNS",
      "translateUserVisibleText",
      "assertNoUserVisibleLeakage",
      "Controlled",
      "资金流向图",
      "已有数据支持",
      "线索",
      "当前证据不足"
    ]),
    user_language_smoke_marker_count: assertMarkers("user-visible language smoke", userVisibleSmoke, [
      "renderDiagnosticCardForAgent",
      "claim_review_card",
      "pair_amount_review_card",
      "flow graph card",
      "case source blocker",
      "vague-scope leak sample",
      "assertClean"
    ]),
    workbench_payload_marker_count: assertMarkers("workbench payload compaction", payloadCompiler, [
      "raw_rows_exposed",
      "result_preview",
      "source_scope",
      "validation_status",
      "answer_delivery_notes",
      "evidence_boundary"
    ])
  };
}

function runPackageIsolationTask(options) {
  const manifest = readJson("plugins/analytix-fund-analysis/.codex-plugin/plugin.json");
  const version = text(manifest.version) || "unknown";
  const outRoot = path.join(os.tmpdir(), `analytix-functional-package-${version}-${timestamp()}`);
  const result = spawnSync("node", [
    "plugins/analytix-fund-analysis/scripts/prepare-hub-package.mjs",
    "--skip-archive",
    "--force",
    "--out-root",
    outRoot,
    "--json"
  ], {
    cwd: REPO_ROOT,
    encoding: "utf8",
    timeout: options.timeoutMs
  });
  const stdout = text(result.stdout);
  const stderr = text(result.stderr);
  if (result.status !== 0) {
    throw new Error(`prepare-hub-package failed: ${stderr || stdout}`);
  }
  const payload = JSON.parse(stdout);
  assert(payload.golden_isolation?.ok === true, "Hub package golden/oracle isolation must pass");
  assert(
    arrayOf(payload.golden_isolation?.case_specific_content_hits).length === 0,
    "Hub package must not contain fixed real-case facts"
  );
  assert(payload.boundaries?.writes_only_under_tmp === true, "Hub package must write only under tmp");
  assert(payload.boundaries?.hub_publish_path === false, "prepare package must not be a Hub publish path");
  return {
    source_root: payload.source_root,
    plugin: payload.plugin,
    golden_isolation: payload.golden_isolation,
    boundaries: payload.boundaries
  };
}

function checkUserDeliveryContract() {
  const compiler = readText("plugins/analytix-fund-analysis/mcp/agent-output-compiler.mjs");
  const payloadCompiler = readText("plugins/analytix-fund-analysis/mcp/agent-payload-compiler.mjs");
  const destinationRuntime = readText("plugins/analytix-fund-analysis/mcp/destination-diagnostic-runtime.mjs");
  const flowRuntime = readText("plugins/analytix-fund-analysis/mcp/fund-flow-graph-runtime.mjs");
  const fundgraphBuilder = readText("plugins/analytix-fund-analysis/mcp/fundgraph-builder.mjs");
  const casegraphProtocol = readText("plugins/analytix-fund-analysis/mcp/casegraph-protocol.mjs");
  const investigationLabProtocol = readText("plugins/analytix-fund-analysis/mcp/investigation-lab-protocol.mjs");
  const intentPlanProtocol = readText("plugins/analytix-fund-analysis/mcp/intent-plan-protocol.mjs");
  const investigationLabRuntime = readText("plugins/analytix-fund-analysis/mcp/investigation-lab-diagnostic-runtime.mjs");
  const claimReviewRuntime = readText("plugins/analytix-fund-analysis/mcp/claim-review-diagnostic-runtime.mjs");
  const graphSkill = readText("plugins/analytix-fund-analysis/skills/graph-visualization/SKILL.md");
  const subjectSkill = readText("plugins/analytix-fund-analysis/skills/subject-dossier/SKILL.md");
  const fundTracingSkill = readText("plugins/analytix-fund-analysis/skills/fund-tracing/SKILL.md");
  const claimReviewSkill = readText("plugins/analytix-fund-analysis/skills/claim-review/SKILL.md");
  const reportBuilderSkill = readText("plugins/analytix-fund-analysis/skills/report-builder/SKILL.md");
  const userFacingLanguage = readText("plugins/analytix-fund-analysis/mcp/user-facing-language.mjs");
  const userVisibleSmoke = readText("plugins/analytix-fund-analysis/scripts/user-visible-language-smoke.mjs");
  const productionTexts = {
    compiler,
    payloadCompiler,
    destinationRuntime,
    flowRuntime,
    fundgraphBuilder,
    casegraphProtocol,
    investigationLabProtocol,
    intentPlanProtocol,
    investigationLabRuntime,
    claimReviewRuntime,
    graphSkill,
    subjectSkill,
    fundTracingSkill,
    claimReviewSkill,
    reportBuilderSkill
  };
  const forbidden = [
    "按当前案件、未限定时间和账户口径核验",
    "全期间同名对手户名聚合口径",
    "全期间同名对手户名聚合",
    "同名对手全期间聚合",
    "核心收款账号确定口径",
    "核心收款账号口径",
    "按核心收款账号核验",
    "核心收款账号",
    "口径与来源边界",
    "已有数据支持的资金边",
    "已有数据支持的交易级资金边",
    "已有数据支持的资金边（资金流向图依据表）",
    "资金流向图（仅使用上表已有数据支持的资金边）",
    "Subject Dossier",
    "归属边界（direct / candidate）",
    "候选线索账户 0 个",
    "必须保留",
    "不要合并或改名",
    "作答骨架",
    "第一行保留"
  ];
  for (const [label, body] of Object.entries(productionTexts)) {
    for (const marker of forbidden) {
      assert(!body.includes(marker), `${label} still exposes delivery anti-pattern: ${marker}`);
    }
  }
  return {
    compiler_support_markers: assertMarkers("economic-investigation compiler support envelope", compiler, [
      "support_only: true",
      "final_answer_owned_by_focused_skill: true",
      "source_map",
      "support_facts",
      "compact_evidence",
      "known_risks",
      "query_guidance",
      "suggested_next_directions"
    ]),
    flow_runtime_markers: assertMarkers("fund-flow graph runtime delivery", flowRuntime, [
      "资金穿透图",
      "资金来源",
      "主要资金链路",
      "下游去向",
      "资金断点",
      "待补证事项"
    ]),
    graph_skill_markers: assertMarkers("graph visualization skill delivery", graphSkill, [
      "结论",
      "资金来源",
      "主要资金链路",
      "下游去向",
      "资金断点",
      "待补证事项"
    ]),
    subject_skill_markers: assertMarkers("subject dossier delivery", subjectSkill, [
      "账户基本情况",
      "登记账户清单",
      "重点账户表",
      "资金流入",
      "资金流出",
      "重点对手方",
      "异常特征",
      "可疑用途或去向",
      "核查建议"
    ]),
    negative_guard_markers: assertMarkers("mechanical delivery negative guard", userFacingLanguage + "\n" + userVisibleSmoke, [
      "机械开头",
      "内部金额统计标题",
      "内部图谱标题",
      "direct/candidate",
      "指令型标题合同",
      "mechanical delivery heading sample"
    ])
  };
}

async function runTask(task, options) {
  if (task.requires_backend && options.skipBackend) {
    return {
      id: task.id,
      family: task.family,
      status: "skipped",
      reason: "requires live backend and --skip-backend was set"
    };
  }
  const missingEnv = arrayOf(task.env_required).map(text).filter((name) => !text(process.env[name]));
  if (missingEnv.length) {
    return {
      id: task.id,
      family: task.family,
      status: "failed",
      description: task.description,
      reason: `missing required environment variable(s): ${missingEnv.join(", ")}`
    };
  }
  if (task.runner === "static" || task.runner === "package") {
    const startedAt = Date.now();
    try {
      const details = task.runner === "package" ? runPackageIsolationTask(options) : await task.check(options);
      return {
        id: task.id,
        family: task.family,
        status: "passed",
        description: task.description,
        elapsed_ms: Date.now() - startedAt,
        details
      };
    } catch (error) {
      return {
        id: task.id,
        family: task.family,
        status: "failed",
        description: task.description,
        elapsed_ms: Date.now() - startedAt,
        error: error instanceof Error ? error.message : String(error)
      };
    }
  }
  const [command, baseArgs] = task.command;
  const args = [...baseArgs];
  if (options.output && task.output_directory) {
    const resolvedOutput = path.resolve(REPO_ROOT, options.output);
    const outputBase = path.basename(resolvedOutput, path.extname(resolvedOutput));
    args.push(
      "--output-dir",
      path.join(path.dirname(resolvedOutput), `${outputBase}.artifacts`, task.output_directory)
    );
  }
  if (baseArgs.some((arg) => String(arg).includes("frontdoor-smoke.mjs"))) {
    args.push(
      "--backend-url",
      options.backendUrl,
      "--timeout-ms",
      String(options.timeoutMs),
      "--forbid-doc-leakage",
      "--forbid-report-gate-leakage",
      "--json"
    );
    if (options.output) {
      args.push(
        "--output",
        path.join(path.dirname(path.resolve(REPO_ROOT, options.output)), `${task.id}.frontdoor-smoke.json`)
      );
    }
  }
  const startedAt = Date.now();
  const result = spawnSync(command, args, {
    cwd: REPO_ROOT,
    encoding: "utf8",
    timeout: (options.timeoutMs * Math.max(1, Number(task.process_timeout_multiplier || 1))) + 5000
  });
  const elapsedMs = Date.now() - startedAt;
  const stdout = text(result.stdout);
  const stderr = text(result.stderr);
  const commandPayload = parseJsonPayload(stdout);
  const commandStatus = text(commandPayload.status);
  let status = result.status === 0 ? "passed" : "failed";
  if (result.status === 0 && commandStatus === "skipped") status = "skipped";
  if (result.status === 0 && commandStatus === "failed") status = "failed";
  return {
    id: task.id,
    family: task.family,
    status,
    description: task.description,
    reason: status === "skipped" ? text(commandPayload.reason || "command reported skipped") : undefined,
    exit_code: result.status,
    signal: result.signal || "",
    elapsed_ms: elapsedMs,
    command: [command, ...args].join(" "),
    stdout_tail: stdout.slice(-3000),
    stderr_tail: stderr.slice(-3000)
  };
}

function writeOutput(outputPath, payload) {
  const resolved = path.resolve(REPO_ROOT, outputPath || defaultOutputPath());
  fs.mkdirSync(path.dirname(resolved), { recursive: true });
  fs.writeFileSync(resolved, `${JSON.stringify(payload, null, 2)}\n`);
  return resolved;
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.listTasks) {
    const payload = {
      status: "ok",
      task_count: TASKS.length,
      tasks: TASKS.map(({ command, check, ...task }) => task)
    };
    console.log(JSON.stringify(payload, null, 2));
    return;
  }
  if (!options.output) options.output = defaultOutputPath();
  const tasks = selectedTasks(options);
  const results = [];
  for (const task of tasks) {
    results.push(await runTask(task, options));
  }
  const failures = results.filter((result) => result.status === "failed");
  const payload = {
    status: failures.length ? "failed" : "ok",
    generated_at: new Date().toISOString(),
    note: "Functional eval checks source + validation + delivery behavior; it is not an A/B scorer.",
    release_identity: releaseIdentity(),
    backend_url: options.backendUrl,
    task_count: tasks.length,
    passed_count: results.filter((result) => result.status === "passed").length,
    skipped_count: results.filter((result) => result.status === "skipped").length,
    failed_count: failures.length,
    results
  };
  const outputPath = writeOutput(options.output, payload);
  payload.output_path = path.relative(REPO_ROOT, outputPath);
  if (options.json) {
    console.log(JSON.stringify(payload, null, 2));
  } else {
    console.log(`Functional eval: ${payload.status}`);
    console.log(`  passed: ${payload.passed_count}`);
    console.log(`  skipped: ${payload.skipped_count}`);
    console.log(`  failed: ${payload.failed_count}`);
    console.log(`  output: ${payload.output_path}`);
  }
  if (failures.length) process.exitCode = 1;
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exitCode = 1;
});
