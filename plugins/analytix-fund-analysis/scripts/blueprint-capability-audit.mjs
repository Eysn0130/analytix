#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { tools as MCP_TOOL_SCHEMAS } from "../mcp/tool-schemas.mjs";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");
const DEFAULT_CASE_DB = process.env.ANALYTIX_FUNDS_ORACLE_CASE_DB
  || process.env.ANALYTIX_FUNDS_CASE_DB
  || process.env.ANALYTIX_CASE_DB
  || "";

const SHARED_CONTRACT_REQUIRED_FILES = [
  "manifest.json",
  "input.schema.json",
  "output.schema.json",
  "policy.json",
  "examples.json"
];

const WORKBENCH_TOOLS = new Set([
  "inspect_case_schema",
  "profile_case_schema",
  "count_case_rows",
  "explain_case_sql",
  "diagnose_case_sql",
  "preview_case_rows",
  "inspect_workbench_history",
  "case_sql_recipes",
  "run_case_sql",
  "create_case_notebook",
  "export_cleaned_case_data"
]);

const EXPECTED_NON_READ_ONLY_TOOLS = new Set([
  "export_cleaned_case_data",
  "create_case_notebook",
  "run_full_case_analysis"
]);

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function unique(values) {
  return [...new Set(values.map(text).filter(Boolean))];
}

function readText(relativePath) {
  return fs.readFileSync(path.join(REPO_ROOT, relativePath), "utf8");
}

function readExistingTexts(relativePaths) {
  return relativePaths
    .filter((relativePath) => exists(relativePath))
    .map((relativePath) => readText(relativePath))
    .join("\n");
}

function readJson(relativePath) {
  return JSON.parse(readText(relativePath));
}

function exists(relativePath) {
  return fs.existsSync(path.join(REPO_ROOT, relativePath));
}

function timestamp() {
  return new Date().toISOString().replace(/[:.]/gu, "-");
}

function defaultOutputDir() {
  return path.join(REPO_ROOT, "output", "analytix-fund-analysis", "functional-closure", timestamp());
}

function parseArgs(argv) {
  const options = {
    json: false,
    outputDir: "",
    caseDb: DEFAULT_CASE_DB,
    noDuckdb: false,
    failOnStructuralGaps: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--json") options.json = true;
    else if (arg === "--output-dir") options.outputDir = next();
    else if (arg.startsWith("--output-dir=")) options.outputDir = arg.slice("--output-dir=".length);
    else if (arg === "--case-db") options.caseDb = next();
    else if (arg.startsWith("--case-db=")) options.caseDb = arg.slice("--case-db=".length);
    else if (arg === "--no-duckdb") options.noDuckdb = true;
    else if (arg === "--fail-on-structural-gaps") options.failOnStructuralGaps = true;
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
  console.log(`Usage: node plugins/analytix-fund-analysis/scripts/blueprint-capability-audit.mjs [options]

Deterministically audits blueprint/capability closure. It does not call an LLM,
does not score answer quality, and does not launch Analytix.

Options:
  --json                         Print JSON to stdout.
  --output-dir <dir>             Write JSON/Markdown to a specific directory.
  --case-db <path>               Optional real DuckDB case path for read-only schema readiness.
                                  Defaults to ANALYTIX_FUNDS_ORACLE_CASE_DB,
                                  ANALYTIX_FUNDS_CASE_DB, or ANALYTIX_CASE_DB
                                  when set; otherwise DuckDB readiness is skipped.
  --no-duckdb                    Skip local DuckDB readiness audit.
  --fail-on-structural-gaps      Exit non-zero when structural P0 gaps are found.
`);
}

function markersPresent(body, markers) {
  const haystack = text(body);
  return markers.map((marker) => ({
    marker,
    present: haystack.includes(marker)
  }));
}

function normalizeForSearch(value) {
  return text(value)
    .toLowerCase()
    .replace(/[_-]+/gu, " ")
    .replace(/\s+/gu, " ");
}

function sourceIncludesMarker(source, marker) {
  return text(source).includes(marker)
    || normalizeForSearch(source).includes(normalizeForSearch(marker));
}

function statusFromFindings(findings, { releaseBlocker = false } = {}) {
  const missing = findings.filter((item) => item.severity === "missing" || item.severity === "p0");
  if (missing.length) return "missing";
  if (releaseBlocker || findings.some((item) => item.severity === "partial")) return "partial";
  return "done";
}

function parseFunctionalTasks(functionalSource) {
  return unique([...functionalSource.matchAll(/\bid:\s*"([^"]+)"/gu)].map((match) => match[1]));
}

function parseGoldenTasks(goldenAnswerSet) {
  return new Set(arrayOf(objectOf(goldenAnswerSet).tasks).map((task) => text(task.id)).filter(Boolean));
}

function parseMatrixRows(matrixSource) {
  const rows = [];
  for (const line of matrixSource.split(/\r?\n/u)) {
    if (!line.startsWith("| ")) continue;
    if (line.includes("---") || line.includes("Blueprint item")) continue;
    const cells = line.split("|").slice(1, -1).map((cell) => cell.trim());
    if (cells.length < 7) continue;
    rows.push({
      item: cells[0],
      target_capability: cells[1],
      implemented_files: cells[2],
      status: cells[3],
      test: cells[4],
      evidence_path: cells[5],
      release_blocker: cells[6]
    });
  }
  return rows;
}

function contractStatus(toolName) {
  const contractDir = path.join(REPO_ROOT, "shared", "skill-contracts", toolName);
  if (!fs.existsSync(contractDir)) {
    return {
      exists: false,
      missing_files: [],
      status: "mcp_schema_runtime"
    };
  }
  const missingFiles = SHARED_CONTRACT_REQUIRED_FILES.filter((file) => !fs.existsSync(path.join(contractDir, file)));
  return {
    exists: true,
    missing_files: missingFiles,
    status: missingFiles.length ? "missing" : "done"
  };
}

function auditFocusedSkills(registry) {
  return arrayOf(registry.focused_skills).map((skill) => {
    const id = text(skill.id);
    const skillPath = `plugins/analytix-fund-analysis/skills/${id}/SKILL.md`;
    const agentPath = `plugins/analytix-fund-analysis/skills/${id}/agents/openai.yaml`;
    const findings = [];
    if (!exists(skillPath)) findings.push({ severity: "missing", message: "missing SKILL.md" });
    if (!exists(agentPath)) findings.push({ severity: "missing", message: "missing agents/openai.yaml" });
    const body = exists(skillPath) ? readText(skillPath) : "";
    const requiredSections = ["## Use when", "## Not for", "## Workflow", "## Completion Gate"];
    const sectionCoverage = markersPresent(body, requiredSections);
    const missingSections = sectionCoverage.filter((item) => !item.present).map((item) => item.marker);
    if (missingSections.length) findings.push({ severity: "partial", message: `missing or non-standard sections: ${missingSections.join(", ")}` });
    if (!arrayOf(skill.use_when).length) findings.push({ severity: "missing", message: "registry use_when is empty" });
    if (!text(skill.completion_standard)) findings.push({ severity: "missing", message: "registry completion_standard is empty" });
    return {
      id,
      status: statusFromFindings(findings),
      registry_use_when_count: arrayOf(skill.use_when).length,
      command_count: arrayOf(skill.commands).length,
      skill_path: skillPath,
      agent_path: agentPath,
      section_coverage: sectionCoverage,
      completion_standard: text(skill.completion_standard),
      findings
    };
  });
}

function auditCapabilities({ registry, commandMetadata, goldenTaskIds, toolNames, doctorSource, functionalSource }) {
  const commandOwners = new Map(arrayOf(commandMetadata.commands).map((command) => [text(command.command), objectOf(command)]));
  const evalFixture = readJson("plugins/analytix-fund-analysis/scripts/eval-fixtures/eval-coverage-contract.json");
  const capabilityEvalTasks = objectOf(evalFixture.capability_eval_tasks);
  const doctorAndFunctionalSource = `${doctorSource}\n${functionalSource}`;
  return arrayOf(registry.capabilities).map((capability) => {
    const id = text(capability.id);
    const findings = [];
    const primaryTools = arrayOf(capability.primary_tools).map(text).filter(Boolean);
    const conditionalTools = arrayOf(capability.conditional_tools).map(text).filter(Boolean);
    const allTools = unique([...primaryTools, ...conditionalTools]);
    const missingTools = allTools.filter((tool) => !toolNames.has(tool));
    const unknownCommands = arrayOf(capability.commands).map(text).filter(Boolean).filter((command) => !commandOwners.has(command));
    const commandOwnerMismatches = arrayOf(capability.commands).map(text).filter(Boolean).flatMap((command) => {
      const owner = commandOwners.get(command);
      if (!owner) return [];
      const commandTools = arrayOf(owner.primaryTools).map(text).filter(Boolean);
      const missingFromCapability = commandTools.filter((tool) => !allTools.includes(tool));
      return missingFromCapability.length ? [`${command} command tools not in capability: ${missingFromCapability.join(", ")}`] : [];
    });
    const evidenceContract = objectOf(capability.evidence_contract);
    const answerContract = objectOf(capability.answer_card_contract);
    const toolBudget = objectOf(capability.tool_budget);
    const evalTasks = arrayOf(capability.eval_tasks).length
      ? arrayOf(capability.eval_tasks).map(text).filter(Boolean)
      : arrayOf(capabilityEvalTasks[id]).map(text).filter(Boolean);
    const missingEvalTasks = evalTasks.filter((taskId) => !goldenTaskIds.has(taskId));
    const doctorChecks = arrayOf(capability.doctor_checks).map(text).filter(Boolean);
    const missingDoctorMarkers = doctorChecks.filter((marker) => !sourceIncludesMarker(doctorAndFunctionalSource, marker));
    if (missingTools.length) findings.push({ severity: "missing", message: `unknown MCP tools: ${missingTools.join(", ")}` });
    if (unknownCommands.length) findings.push({ severity: "missing", message: `commands missing command metadata: ${unknownCommands.join(", ")}` });
    if (commandOwnerMismatches.length) findings.push({ severity: "partial", message: commandOwnerMismatches.join("; ") });
    if (!primaryTools.length) findings.push({ severity: "missing", message: "primary_tools is empty" });
    if (!arrayOf(capability.required_gates).length) findings.push({ severity: "missing", message: "required_gates is empty" });
    if (!arrayOf(capability.stop_conditions).length) findings.push({ severity: "missing", message: "stop_conditions is empty" });
    if (!Object.keys(answerContract).length) findings.push({ severity: "missing", message: "answer_card_contract missing" });
    if (evidenceContract.user_visible_opaque_refs_allowed !== false) findings.push({ severity: "missing", message: "opaque refs must be hidden from user-visible output" });
    if (!evidenceContract.ledger_required) findings.push({ severity: "partial", message: "evidence ledger is not required" });
    if (!Number(toolBudget.ordinary_max_tool_calls)) findings.push({ severity: "partial", message: "ordinary tool budget missing" });
    if (!evalTasks.length) findings.push({ severity: "partial", message: "no deterministic eval coverage task mapped" });
    if (missingEvalTasks.length && evalTasks.length) findings.push({ severity: "partial", message: `mapped eval tasks not in golden coverage fixture: ${missingEvalTasks.join(", ")}` });
    if (missingDoctorMarkers.length && doctorChecks.length) findings.push({ severity: "advisory", message: `doctor check labels are not literal source markers: ${missingDoctorMarkers.join(", ")}` });
    return {
      id,
      lane: text(capability.lane),
      intent: text(capability.intent),
      status: statusFromFindings(findings),
      command_count: arrayOf(capability.commands).length,
      primary_tools: primaryTools,
      conditional_tools: conditionalTools,
      required_gate_count: arrayOf(capability.required_gates).length,
      stop_condition_count: arrayOf(capability.stop_conditions).length,
      eval_tasks: evalTasks,
      missing_eval_tasks: missingEvalTasks,
      doctor_checks: doctorChecks,
      doctor_marker_advisories: missingDoctorMarkers,
      evidence_contract: {
        ledger_required: Boolean(evidenceContract.ledger_required),
        supported_edge_required_for_mermaid: Boolean(evidenceContract.supported_edge_required_for_mermaid),
        user_visible_opaque_refs_allowed: evidenceContract.user_visible_opaque_refs_allowed
      },
      findings
    };
  });
}

function auditTools({ registry, toolNames, toolSchemas, toolRuntimeSource, backendSource, releaseSliceSource }) {
  const registryTools = unique(arrayOf(registry.capabilities).flatMap((capability) => [
    ...arrayOf(capability.primary_tools),
    ...arrayOf(capability.conditional_tools)
  ]));
  const schemaRows = toolSchemas.map((tool) => {
    const name = text(tool.name);
    const findings = [];
    const annotations = objectOf(tool.annotations);
    const inputSchema = objectOf(tool.inputSchema);
    if (!name) findings.push({ severity: "missing", message: "tool name missing" });
    if (EXPECTED_NON_READ_ONLY_TOOLS.has(name)) {
      if (annotations.readOnlyHint !== false) {
        findings.push({ severity: "missing", message: "artifact-writing tool must set readOnlyHint=false" });
      }
    } else if (annotations.readOnlyHint !== true) {
      findings.push({ severity: "partial", message: "fact/diagnostic tool must set readOnlyHint=true" });
    }
    if (annotations.destructiveHint !== false) findings.push({ severity: "missing", message: "destructiveHint must be false" });
    if (annotations.openWorldHint !== false) findings.push({ severity: "missing", message: "openWorldHint must be false" });
    if (inputSchema.additionalProperties !== false) findings.push({ severity: "partial", message: "inputSchema.additionalProperties should be false" });
    const contract = contractStatus(name);
    const isWorkbench = WORKBENCH_TOOLS.has(name);
    if (contract.exists && contract.missing_files.length) findings.push({ severity: "missing", message: `shared contract missing files: ${contract.missing_files.join(", ")}` });
    if (isWorkbench && !toolRuntimeSource.includes(name)) findings.push({ severity: "missing", message: "workbench tool missing runtime marker" });
    if (name === "run_case_sql") {
      const markers = [
        "case_workbench_evidence_card",
        "source_hash",
        "metric_scope",
        "validation_state",
        "evidence_card"
      ];
      const missingMarkers = markers.filter((marker) => !backendSource.includes(marker) || !toolRuntimeSource.includes(marker));
      if (missingMarkers.length) findings.push({ severity: "missing", message: `run_case_sql evidence handoff marker missing: ${missingMarkers.join(", ")}` });
      const requiredReleaseFiles = [
        "plugins/analytix-fund-analysis/mcp/tool-schemas.mjs",
        "plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs",
        "plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs",
        "plugins/analytix-fund-analysis/mcp/evidence-ledger.mjs"
      ];
      const missingReleaseFiles = requiredReleaseFiles.filter((relativePath) => !releaseSliceSource.includes(relativePath));
      if (missingReleaseFiles.length) {
        findings.push({ severity: "missing", message: `run_case_sql MCP/runtime contract missing from release slice: ${missingReleaseFiles.join(", ")}` });
      }
    }
    return {
      name,
      in_capability_registry: registryTools.includes(name),
      status: statusFromFindings(findings),
      read_only_hint: annotations.readOnlyHint,
      destructive_hint: annotations.destructiveHint,
      has_shared_contract: contract.exists,
      shared_contract_status: contract.status,
      contract_model: contract.exists ? "shared_skill_contracts" : "mcp_schema_runtime",
      findings
    };
  });
  const registryOnly = registryTools.filter((tool) => !toolNames.has(tool));
  return {
    rows: schemaRows,
    registry_tool_count: registryTools.length,
    schema_tool_count: toolSchemas.length,
    registry_only_missing_schema: registryOnly
  };
}

function auditSharedContracts() {
  const root = path.join(REPO_ROOT, "shared", "skill-contracts");
  if (!fs.existsSync(root)) {
    const toolRuntimeSource = readExistingTexts([
      "plugins/analytix-fund-analysis/mcp/tool-schemas.mjs",
      "plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs",
      "plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs",
      "plugins/analytix-fund-analysis/mcp/evidence-ledger.mjs",
      "plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md"
    ]);
    return MCP_TOOL_SCHEMAS
      .filter((tool) => WORKBENCH_TOOLS.has(text(tool.name)))
      .map((tool) => {
        const name = text(tool.name);
        const schema = objectOf(tool.inputSchema);
        const annotations = objectOf(tool.annotations);
        const findings = [];
        if (schema.type !== "object") findings.push({ severity: "missing", message: "input schema must be an object" });
        if (!Object.keys(objectOf(schema.properties)).length) findings.push({ severity: "missing", message: "input schema properties missing" });
        if (schema.additionalProperties !== false) findings.push({ severity: "partial", message: "input schema additionalProperties should be false" });
        if (annotations.destructiveHint !== false) findings.push({ severity: "missing", message: "destructiveHint must be false" });
        if (annotations.openWorldHint !== false) findings.push({ severity: "missing", message: "openWorldHint must be false" });
        if (name === "inspect_case_schema") {
          const requiredMarkers = ["sql_name_contract", "tables[].sql_name", "tables[].columns[].sql_name/sql_identifier", "sql_identifier", "display_name is UI-only"];
          const missingMarkers = requiredMarkers.filter((marker) => !toolRuntimeSource.includes(marker));
          if (missingMarkers.length) findings.push({ severity: "missing", message: `inspect_case_schema runtime contract missing: ${missingMarkers.join(", ")}` });
        }
        if (name === "run_case_sql") {
          const requiredMarkers = [
            "case_workbench_evidence_card",
            "source_hash",
            "metric_scope",
            "validation_state",
            "evidence_card",
            "allowed_view_policy = \"cleaned_and_analysis_only\""
          ];
          const missingMarkers = requiredMarkers.filter((marker) => !toolRuntimeSource.includes(marker));
          if (missingMarkers.length) findings.push({ severity: "missing", message: `run_case_sql runtime contract missing: ${missingMarkers.join(", ")}` });
        }
        return {
          skill_id: name,
          status: statusFromFindings(findings),
          contract_model: "mcp_schema_runtime",
          missing_files: [],
          read_only: annotations.readOnlyHint !== false,
          side_effect_class: annotations.readOnlyHint === false ? "artifact_write_or_notebook" : "read_current_case",
          findings
        };
      });
  }
  const dirs = fs.readdirSync(root, { withFileTypes: true }).filter((entry) => entry.isDirectory()).map((entry) => entry.name).sort();
  return dirs.map((dir) => {
    const missingFiles = SHARED_CONTRACT_REQUIRED_FILES.filter((file) => !fs.existsSync(path.join(root, dir, file)));
    const policy = fs.existsSync(path.join(root, dir, "policy.json")) ? JSON.parse(fs.readFileSync(path.join(root, dir, "policy.json"), "utf8")) : {};
    const outputSchema = fs.existsSync(path.join(root, dir, "output.schema.json")) ? JSON.parse(fs.readFileSync(path.join(root, dir, "output.schema.json"), "utf8")) : {};
    const findings = [];
    if (missingFiles.length) findings.push({ severity: "missing", message: `missing files: ${missingFiles.join(", ")}` });
    const writeClassesAllowedByContract = new Set([
      "analysis_artifact_write",
      "projection_write",
      "write_case_metadata",
      "write_current_case_export",
      "write_full_case_report_baseline"
    ]);
    if (objectOf(policy).read_only !== true && !writeClassesAllowedByContract.has(text(objectOf(policy).side_effect_class))) {
      findings.push({ severity: "partial", message: "policy does not declare read_only true" });
    }
    if (!objectOf(outputSchema).properties) findings.push({ severity: "missing", message: "output schema has no properties" });
    return {
      skill_id: dir,
      status: statusFromFindings(findings),
      missing_files: missingFiles,
      read_only: objectOf(policy).read_only,
      side_effect_class: text(objectOf(policy).side_effect_class),
      findings
    };
  });
}

function auditBlueprintMatrix(matrixSource) {
  const rows = parseMatrixRows(matrixSource);
  const releaseBlockerRows = rows.filter((row) => /yes|blocker/iu.test(row.release_blocker));
  const partialRows = rows.filter((row) => text(row.status).toLowerCase() === "partial");
  const doneRows = rows.filter((row) => text(row.status).toLowerCase() === "done");
  return {
    rows,
    summary: {
      row_count: rows.length,
      done_count: doneRows.length,
      partial_count: partialRows.length,
      release_blocker_count: releaseBlockerRows.length
    },
    remaining_blockers: releaseBlockerRows.map((row) => ({
      item: row.item,
      status: row.status,
      release_blocker: row.release_blocker,
      test: row.test
    }))
  };
}

function auditMaturePluginParity({ rootSkill, indexSkill, caseWorkbench, deliveryQc, matrixSource, toolSchemasSource, functionalSource, investigationAnswerSource, investigationAnswerValidatorSource }) {
  const combined = [
    rootSkill,
    indexSkill,
    caseWorkbench,
    deliveryQc,
    matrixSource,
    toolSchemasSource,
    functionalSource,
    investigationAnswerSource,
    investigationAnswerValidatorSource
  ].join("\n");
  const rows = [
    {
      source: "Data Analytics",
      principle: "source + validation + delivery; semantic maps locate facts but live/source-backed evidence controls claims",
      markers: [
        "source-of-truth",
        "validation_state",
        "delivery",
        "focused workflows",
        "case_source_envelope",
        "report",
        "artifact"
      ],
      analytix_enhancement: "adds current-case-only, cleaned/analysis DuckDB boundary, evidence cards, claim review, and public-security economic-investigation wording"
    },
    {
      source: "Investment Banking",
      principle: "router admits and hands off; lead owner owns client-ready deliverable; internal support stays hidden",
      markers: [
        "Navigator not commander",
        "focused owner",
        "support layer",
        "final_answer_owned_by_focused_skill",
        "hero deliverable"
      ],
      analytix_enhancement: "turns lead-owner delivery into pair amount, dossier, tracing, report, visual evidence, claim review, and delivery QC owners"
    },
    {
      source: "Public Equity Investing",
      principle: "professional judgment separates evidence, uncertainty, action, and missing proof",
      markers: [
        "professional judgment",
        "暂不能认定",
        "unsupported",
        "claim-review",
        "evidence maturity",
        "下一步"
      ],
      analytix_enhancement: "uses evidence maturity and proof-action language instead of investment thesis wording"
    },
    {
      source: "crystaldba/postgres-mcp",
      principle: "schema/object inspection, restricted safe SQL, read-only execution, explain plans, slow-path diagnostics, health checks, and deterministic index analysis",
      markers: [
        "inspect_case_schema",
        "run_case_sql",
        "explain_case_sql",
        "diagnose_case_sql",
        "count_case_rows",
        "preview_case_rows",
        "read-only",
        "fc_*_norm",
        "analysis_*"
      ],
      analytix_enhancement: "adapts CrystalDBA-style database-site tools to DuckDB/current-case scope while adding evidence_card/source_hash/metric_scope/validation_state, graph/materialized-index health, and no raw/source tables"
    },
    {
      source: "567-labs/instructor",
      principle: "schema-first response models, deterministic validation failures, repair/reask guidance, hooks, failed-attempt inspection, and provider-agnostic structured output",
      markers: [
        "FinalInvestigationAnswer",
        "verified_facts",
        "evidence_refs",
        "fund_flow_paths",
        "evidence_boundaries",
        "next_proof_actions",
        "repair_actions",
        "audit_events"
      ],
      analytix_enhancement: "turns final answers and report sections into an internal validated structure while forcing missing facts back to MCP/DuckDB or evidence boundaries instead of retry-based hallucination"
    }
  ];
  return rows.map((row) => {
    const markerResults = markersPresent(combined, row.markers);
    const missingMarkers = markerResults.filter((item) => !item.present).map((item) => item.marker);
    const findings = missingMarkers.length ? [{ severity: "partial", message: `missing markers: ${missingMarkers.join(", ")}` }] : [];
    return {
      ...row,
      status: statusFromFindings(findings),
      marker_results: markerResults,
      findings
    };
  });
}

function auditDuckdbReadiness(caseDb, skip) {
  if (skip) return { status: "skipped", reason: "--no-duckdb set" };
  if (!caseDb) return { status: "skipped", reason: "case db path omitted" };
  if (!fs.existsSync(caseDb)) return { status: "missing", reason: `case db does not exist: ${caseDb}` };
  const python = path.join(REPO_ROOT, ".venv", "bin", "python");
  const pythonBin = fs.existsSync(python) ? python : "python3";
  const script = String.raw`
import duckdb, json, sys
path = sys.argv[1]
key_tables = ["analysis_txn_detail_idx", "analysis_txn_daily_agg", "analysis_account_dim", "analysis_entity_node", "analysis_relation_edge", "analysis_evidence_ref"]
con = duckdb.connect(path, read_only=True)
try:
    tables = [row[0] for row in con.execute("""
        SELECT table_name
        FROM information_schema.tables
        WHERE table_schema='main'
          AND (table_name LIKE 'fc\\_%\\_norm' ESCAPE '\\' OR table_name LIKE 'analysis\\_%' ESCAPE '\\')
        ORDER BY table_name
    """).fetchall()]
    key = {}
    for table in key_tables:
        if table in tables:
            key[table] = con.execute(f'SELECT COUNT(*) FROM "{table}"').fetchone()[0]
        else:
            key[table] = None
    graph_health = {}
    if "analysis_relation_edge" in tables and "analysis_entity_node" in tables:
        row = con.execute("""
            SELECT
              COUNT(DISTINCT edge.edge_id) AS edge_count,
              COUNT(DISTINCT CASE
                WHEN NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.src_entity_id)
                  OR NOT EXISTS (SELECT 1 FROM analysis_entity_node node WHERE node.entity_id = edge.dst_entity_id)
                THEN edge.edge_id END) AS missing_endpoint_edge_count
            FROM analysis_relation_edge edge
        """).fetchone()
        graph_health = {
            "edge_count": row[0],
            "missing_endpoint_edge_count": row[1],
            "missing_endpoint_ratio": (row[1] / row[0]) if row[0] else 0,
        }
    print(json.dumps({"allowed_table_count": len(tables), "key_table_rows": key, "graph_health": graph_health}, ensure_ascii=False))
finally:
    con.close()
`;
  const result = spawnSync(pythonBin, ["-c", script, caseDb], {
    cwd: REPO_ROOT,
    encoding: "utf8",
    maxBuffer: 1024 * 1024
  });
  if (result.status !== 0) {
    return {
      status: "partial",
      reason: text(result.stderr || result.stdout) || "duckdb readiness check failed"
    };
  }
  const payload = JSON.parse(result.stdout || "{}");
  const keyRows = objectOf(payload.key_table_rows);
  const graphHealth = objectOf(payload.graph_health);
  const missingKeyTables = Object.entries(keyRows).filter(([, count]) => count == null).map(([table]) => table);
  const emptyKeyTables = Object.entries(keyRows).filter(([, count]) => Number(count) === 0).map(([table]) => table);
  const graphEndpointGaps = Number(graphHealth.missing_endpoint_edge_count || 0);
  return {
    status: missingKeyTables.length ? "partial" : "done",
    case_db: caseDb,
    allowed_table_count: Number(payload.allowed_table_count || 0),
    key_table_rows: keyRows,
    graph_health: graphHealth,
    findings: [
      ...missingKeyTables.map((table) => ({ severity: "partial", message: `missing key table ${table}` })),
      ...emptyKeyTables.map((table) => ({ severity: "partial", message: `key table ${table} is empty` })),
      ...(graphEndpointGaps
        ? [{ severity: "p1", message: `graph endpoint join gaps: ${graphEndpointGaps}/${Number(graphHealth.edge_count || 0)} analysis_relation_edge rows do not fully join analysis_entity_node` }]
        : [])
    ]
  };
}

function countStatuses(rows) {
  return rows.reduce((acc, row) => {
    const status = text(row.status) || "unknown";
    acc[status] = (acc[status] || 0) + 1;
    return acc;
  }, {});
}

function structuralGapCount(report) {
  const focused = report.focused_skills.filter((row) => row.status === "missing").length;
  const capabilities = report.capabilities.filter((row) => row.status === "missing").length;
  const tools = report.tools.rows.filter((row) => row.status === "missing").length + report.tools.registry_only_missing_schema.length;
  const contracts = report.shared_contracts.filter((row) => row.status === "missing").length;
  const parity = report.mature_plugin_parity.filter((row) => row.status === "missing").length;
  return focused + capabilities + tools + contracts + parity;
}

function renderMarkdown(report) {
  const lines = [];
  lines.push(`# Analytix 能力闭环审计`);
  lines.push("");
  lines.push(`生成时间：${report.generated_at}`);
  lines.push("");
  lines.push("本审计只做蓝本、合同、MCP、技能、脚本和 DuckDB 结构性闭环核验；不启动 Analytix，不调用大模型，不对模型答案评分。");
  lines.push("");
  lines.push("## 总览");
  lines.push("");
  lines.push(`- Focused skills：${JSON.stringify(report.summary.focused_skill_status_counts)}`);
  lines.push(`- Capabilities：${JSON.stringify(report.summary.capability_status_counts)}`);
  lines.push(`- MCP tools：${JSON.stringify(report.summary.tool_status_counts)}`);
  lines.push(`- MCP/runtime contracts：${JSON.stringify(report.summary.shared_contract_status_counts)}`);
  lines.push(`- 蓝本矩阵：${report.blueprint_matrix.summary.done_count} done / ${report.blueprint_matrix.summary.partial_count} partial / ${report.blueprint_matrix.summary.release_blocker_count} release blockers`);
  lines.push(`- 结构性 P0 缺口：${report.summary.structural_gap_count}`);
  lines.push(`- 发布判断：${report.summary.release_readiness}`);
  lines.push("");
  lines.push("## 能力闭环");
  lines.push("");
  lines.push("| Capability | Lane | Status | Primary Tools | Findings |");
  lines.push("| --- | --- | --- | --- | --- |");
  for (const row of report.capabilities) {
    lines.push(`| ${row.id} | ${row.lane} | ${row.status} | ${row.primary_tools.join(", ")} | ${row.findings.map((item) => item.message).join("; ") || "closed at static contract level"} |`);
  }
  lines.push("");
  lines.push("## 成熟插件对照");
  lines.push("");
  lines.push("| Source | Status | Mature Principle | Analytix Enhancement | Findings |");
  lines.push("| --- | --- | --- | --- | --- |");
  for (const row of report.mature_plugin_parity) {
    lines.push(`| ${row.source} | ${row.status} | ${row.principle} | ${row.analytix_enhancement} | ${row.findings.map((item) => item.message).join("; ") || "parity markers present"} |`);
  }
  lines.push("");
  lines.push("## 未闭环发布项");
  lines.push("");
  for (const blocker of report.blueprint_matrix.remaining_blockers) {
    lines.push(`- ${blocker.item}：${blocker.status}；${blocker.release_blocker}；验证要求：${blocker.test}`);
  }
  lines.push("");
  lines.push("## DuckDB 数据面");
  lines.push("");
  lines.push(`- 状态：${report.duckdb_readiness.status}`);
  if (report.duckdb_readiness.allowed_table_count != null) {
    lines.push(`- 允许清洗/analysis 表数量：${report.duckdb_readiness.allowed_table_count}`);
    lines.push(`- 关键表行数：${JSON.stringify(report.duckdb_readiness.key_table_rows)}`);
    if (Object.keys(objectOf(report.duckdb_readiness.graph_health)).length) {
      lines.push(`- 图谱健康：${JSON.stringify(report.duckdb_readiness.graph_health)}`);
    }
    for (const finding of arrayOf(report.duckdb_readiness.findings)) {
      lines.push(`- 数据面风险（${finding.severity}）：${finding.message}`);
    }
  } else if (report.duckdb_readiness.reason) {
    lines.push(`- 原因：${report.duckdb_readiness.reason}`);
  }
  lines.push("");
  lines.push("## 下一步建议");
  lines.push("");
  for (const item of report.recommendations) lines.push(`- ${item}`);
  lines.push("");
  return `${lines.join("\n")}\n`;
}

function buildReport(options) {
  const registry = readJson("plugins/analytix-fund-analysis/references/capability-registry.json");
  const embeddedRegistry = readJson("plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/capability-registry.json");
  const commandMetadata = readJson("plugins/analytix-fund-analysis/references/command-metadata.json");
  const goldenAnswerSet = readJson("plugins/analytix-fund-analysis/scripts/eval-fixtures/golden-answer-set.json");
  const matrixSource = readText("plugins/analytix-fund-analysis/references/blueprint-implementation-matrix.md");
  const functionalSource = readText("plugins/analytix-fund-analysis/scripts/functional-eval.mjs");
  const doctorSource = readText("plugins/analytix-fund-analysis/scripts/doctor.mjs");
  const releaseSliceSource = readText("plugins/analytix-fund-analysis/scripts/release-slice-audit.mjs");
  const toolRuntimeSource = readText("plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs")
    + "\n" + readText("plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs");
  const backendSource = readExistingTexts([
    "plugins/analytix-fund-analysis/mcp/tool-call-runtime.mjs",
    "plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs",
    "plugins/analytix-fund-analysis/mcp/case-project-context.mjs",
    "plugins/analytix-fund-analysis/mcp/cleaned-export-runtime.mjs",
    "plugins/analytix-fund-analysis/mcp/safe-skill-runtime.mjs",
    "plugins/analytix-fund-analysis/mcp/mcp-request-handler-runtime.mjs"
  ]);
  const rootSkill = readText("plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md");
  const indexSkill = readText("plugins/analytix-fund-analysis/skills/index/SKILL.md");
  const caseWorkbench = readText("plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md");
  const deliveryQc = readText("plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md");
  const toolSchemasSource = readText("plugins/analytix-fund-analysis/mcp/tool-schemas.mjs");
  const investigationAnswerSource = readText("plugins/analytix-fund-analysis/references/investigation-answer-contract.md");
  const investigationAnswerValidatorSource = readText("plugins/analytix-fund-analysis/mcp/investigation-answer-contract.mjs");
  const functionalTasks = parseFunctionalTasks(functionalSource);
  const goldenTaskIds = parseGoldenTasks(goldenAnswerSet);
  const toolNames = new Set(MCP_TOOL_SCHEMAS.map((tool) => text(tool.name)).filter(Boolean));
  const focusedSkills = auditFocusedSkills(registry);
  const capabilities = auditCapabilities({
    registry,
    commandMetadata,
    goldenTaskIds,
    toolNames,
    doctorSource,
    functionalSource
  });
  const tools = auditTools({
    registry,
    toolNames,
    toolSchemas: MCP_TOOL_SCHEMAS,
    toolRuntimeSource,
    backendSource,
    releaseSliceSource
  });
  const sharedContracts = auditSharedContracts();
  const blueprintMatrix = auditBlueprintMatrix(matrixSource);
  const maturePluginParity = auditMaturePluginParity({
    rootSkill,
    indexSkill,
    caseWorkbench,
    deliveryQc,
    matrixSource,
    toolSchemasSource,
    functionalSource,
    investigationAnswerSource,
    investigationAnswerValidatorSource
  });
  const duckdbReadiness = auditDuckdbReadiness(options.caseDb, options.noDuckdb);
  const registryCopiesMatch = JSON.stringify(registry) === JSON.stringify(embeddedRegistry);
  const report = {
    generated_at: new Date().toISOString(),
    plugin: registry.plugin,
    methodology: {
      no_llm_scoring: true,
      no_analytix_launch: true,
      checked_sources: [
        "capability-registry.json",
        "focused SKILL.md files",
        "MCP tool schemas/runtime",
        "MCP schema/runtime contracts",
        "doctor/functional-eval/release-slice scripts",
        "blueprint implementation matrix",
        "optional read-only DuckDB schema/count readiness"
      ]
    },
    registry_copies_match: registryCopiesMatch,
    functional_task_count: functionalTasks.length,
    focused_skills: focusedSkills,
    capabilities,
    tools,
    shared_contracts: sharedContracts,
    blueprint_matrix: blueprintMatrix,
    mature_plugin_parity: maturePluginParity,
    duckdb_readiness: duckdbReadiness
  };
  report.summary = {
    focused_skill_status_counts: countStatuses(focusedSkills),
    capability_status_counts: countStatuses(capabilities),
    tool_status_counts: countStatuses(tools.rows),
    shared_contract_status_counts: countStatuses(sharedContracts),
    mature_parity_status_counts: countStatuses(maturePluginParity),
    registry_copies_match: registryCopiesMatch,
    structural_gap_count: structuralGapCount(report),
    release_readiness: blueprintMatrix.summary.release_blocker_count === 0 && structuralGapCount(report) === 0
      ? "static-parity-ready"
      : "not-publishable"
  };
  report.recommendations = [
    "继续保持不使用大模型评分作为通过条件；把 L3 真前门 canary 和 L5 经侦审查作为发布前人工/运行证据。",
    "把 partial 能力按蓝本拆成三类处理：active-case 同步 replay、前门 owner 成稿 replay、报告/图表 artifact render/read QA。",
    "对 frontdoor-runtime 中仍直接调用 run_case_sql 的兼容路径做下一轮收敛：保留为 Workbench support，最终成稿必须回 focused owner。",
    "将 Workbench evidence_card 模式扩展到 explain/diagnose/profile/preview/history，形成统一 database-site diagnostic evidence card。",
    "用真实案件库只读 schema/count canary 覆盖清洗/analysis 表，不读取 raw/source 表，不把真实案件固定结论写入 production reference。"
  ];
  return report;
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const outputDir = options.outputDir ? path.resolve(options.outputDir) : defaultOutputDir();
  const report = buildReport(options);
  fs.mkdirSync(outputDir, { recursive: true });
  const jsonPath = path.join(outputDir, "blueprint-capability-audit.json");
  const mdPath = path.join(outputDir, "blueprint-capability-audit.md");
  fs.writeFileSync(jsonPath, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(mdPath, renderMarkdown(report));
  const output = {
    status: report.summary.structural_gap_count ? "structural_gaps" : "ok",
    generated_at: report.generated_at,
    output_dir: outputDir,
    json_path: jsonPath,
    markdown_path: mdPath,
    summary: report.summary,
    duckdb_readiness: report.duckdb_readiness,
    blueprint_remaining_blockers: report.blueprint_matrix.remaining_blockers.length
  };
  if (options.json) console.log(JSON.stringify(output, null, 2));
  else {
    console.log(`blueprint capability audit: ${output.status}`);
    console.log(`json: ${jsonPath}`);
    console.log(`markdown: ${mdPath}`);
    console.log(`summary: ${JSON.stringify(report.summary)}`);
  }
  if (options.failOnStructuralGaps && report.summary.structural_gap_count) process.exit(1);
}

main();
