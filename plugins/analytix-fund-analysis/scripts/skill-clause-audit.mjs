#!/usr/bin/env node

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");

function codexHome() {
  return path.resolve(String(process.env.CODEX_HOME || "").trim() || path.join(os.homedir(), ".codex"));
}

function newestCachedSkillPath(sourceName, pluginName, skillPath) {
  const pluginCacheRoot = path.join(codexHome(), "plugins", "cache", sourceName, pluginName);
  if (fs.existsSync(pluginCacheRoot)) {
    const versions = fs.readdirSync(pluginCacheRoot, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .sort()
      .reverse();
    for (const version of versions) {
      const candidate = path.join(pluginCacheRoot, version, skillPath);
      if (fs.existsSync(candidate)) return candidate;
    }
  }
  return path.join(pluginCacheRoot, "<missing>", skillPath);
}

const MATURE_SKILL_PATHS = [
  newestCachedSkillPath("openai-curated-remote", "data-analytics", "skills/index/SKILL.md"),
  newestCachedSkillPath("openai-curated-remote", "data-analytics", "skills/product-business-analysis/SKILL.md"),
  newestCachedSkillPath("openai-curated-remote", "data-analytics", "skills/metric-diagnostics/SKILL.md"),
  newestCachedSkillPath("openai-curated-remote", "data-analytics", "skills/build-report/SKILL.md"),
  newestCachedSkillPath("openai-curated-remote", "data-analytics", "skills/visualize-data/SKILL.md"),
  newestCachedSkillPath("openai-curated-remote", "investment-banking", "skills/investment-banking/SKILL.md"),
  newestCachedSkillPath("openai-curated-remote", "public-equity-investing", "skills/public-equity-investing/SKILL.md")
];

const REFERENCE_PATHS = [
  "plugins/analytix-fund-analysis/references/focused-skill-shared.md",
  "plugins/analytix-fund-analysis/references/database-site-diagnostics.md",
  "plugins/analytix-fund-analysis/references/economic-investigation-analysis.md",
  "plugins/analytix-fund-analysis/references/economic-investigation-language.md",
  "plugins/analytix-fund-analysis/references/public-security-official-writing.md",
  "plugins/analytix-fund-analysis/references/investigation-answer-contract.md",
  "plugins/analytix-fund-analysis/references/blueprint-execution-plan.md",
  "plugins/analytix-fund-analysis/references/blueprint-implementation-matrix.md",
  "plugins/analytix-fund-analysis/references/top-pluginization-plan.md"
];

const CLAUSE_CATEGORIES = [
  ["invocation_gate", /\b(?:Use when|Not for|Invocation Gate|do not activate|如果|用户要求|明确|普通案件任务)\b|不适用|不得.*激活/iu],
  ["router_owner", /\b(?:router|route|focused owner|lead owner|Navigator|commander|handoff|selected owner|final answer owned|最终成稿|路由|交给|负责最终|不得替代)\b/iu],
  ["source_guardrail", /\b(?:source[- ]of[- ]truth|source guardrail|live source|semantic layer|current-case|当前案件|事实来源|权威来源|弱来源|语义事实|cleaned|analysis_|fc_)\b/iu],
  ["tool_rule", /\b(?:MCP|tool|SQL|DuckDB|Workbench|run_case_sql|inspect_case_schema|profile_case_schema|diagnose_case_sql|preview_case_rows|export_cleaned_case_data|notebook|只读|限行)\b/iu],
  ["evidence_rule", /\b(?:evidence|source_hash|metric_scope|validation_state|source_refs|依据|流水|账户|金额|笔数|期间|链路|核验|证据|证明|暂不能认定|补证)\b/iu],
  ["analysis_method", /\b(?:hypoth|driver|diagnostic|decompose|framework|comparison|baseline|decision|recommendation|异常|研判|特征|意义|穿透|去向|画像|假设|专题)\b/iu],
  ["output_language", /\b(?:Output Contract|Answer Rules|Audience|Language|user-facing|visible|final|口吻|经侦|公安|公文|用户可见|不要写|不得写|结论先行)\b/iu],
  ["completion_gate", /\b(?:Completion Gate|Quality Bar|complete|incomplete|fails if|完成|失败|交付|报告|附件|render|inspect)\b/iu]
];

const HARD_REQUIRED_ANALYTIX_MARKERS = [
  {
    id: "agent_freedom_rule",
    file: "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md",
    markers: ["Agent Freedom Rule", "must not replace Codex thinking", "constrains fact acquisition and evidence integrity"]
  },
  {
    id: "workbench_readonly_duckdb_boundary",
    file: "plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md",
    markers: ["read-only", "cleaned `fc_*_norm`", "approved", "`analysis_*`", "row limit", "evidence_card"]
  },
  {
    id: "database_mcp_ladder",
    file: "plugins/analytix-fund-analysis/references/database-site-diagnostics.md",
    markers: ["inspect_case_schema", "profile_case_schema", "count_case_rows", "explain_case_sql", "diagnose_case_sql", "preview_case_rows", "run_case_sql"]
  },
  {
    id: "semantic_first_not_sql_default",
    file: "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md",
    markers: ["current-case semantic tools first", "case-workbench only for explicit SQL/notebook/custom analysis gaps"]
  },
  {
    id: "public_security_voice",
    file: "plugins/analytix-fund-analysis/references/public-security-official-writing.md",
    markers: ["证据成熟度", "法律敏感降级", "表后研判", "下一步调取"]
  },
  {
    id: "instructor_style_structured_delivery",
    file: "plugins/analytix-fund-analysis/references/investigation-answer-contract.md",
    markers: ["FinalInvestigationAnswer", "verified_facts", "evidence_boundaries", "next_proof_actions", "MCP/DuckDB"]
  },
  {
    id: "investigation_objective_gate",
    file: "plugins/analytix-fund-analysis/references/focused-skill-shared.md",
    markers: ["Investigation Objective Gate", "Ask a concise clarification only when the missing purpose would change", "proceed without asking", "next investigative direction"]
  }
];

const MATURE_PARITY_PATTERNS = [
  {
    id: "router_is_not_substantive_worker",
    mature_basis: ["Investment Banking router: selected lead skill owns substantive work", "Public Equity router: router owns admission and lead-skill selection only"],
    analytix_files: [
      "plugins/analytix-fund-analysis/skills/index/SKILL.md",
      "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md"
    ],
    markers: ["Navigator not commander", "focused owner", "must not replace focused skills"]
  },
  {
    id: "semantic_map_must_be_verified",
    mature_basis: ["Data Analytics: semantic layer is source-selection input, not substitute for live reads"],
    analytix_files: [
      "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md",
      "plugins/analytix-fund-analysis/references/focused-skill-shared.md"
    ],
    markers: ["Semantic maps find facts but do not prove them", "source-of-truth selection", "deterministic current-case facts"]
  },
  {
    id: "analysis_thinking_is_guided_not_blocked",
    mature_basis: ["Data Analytics focused workflows ask the model to frame hypotheses, compare scopes, validate drivers, and state uncertainty"],
    analytix_files: [
      "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md",
      "plugins/analytix-fund-analysis/skills/investigation-lab/SKILL.md",
      "plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md"
    ],
    markers: ["Codex should still infer", "choose hypotheses", "deliberate exploration", "hypothesis"]
  },
  {
    id: "delivery_surface_has_completion_gate",
    mature_basis: ["Data Analytics build-report: report run is incomplete until selected report surface exists or blocker is recorded"],
    analytix_files: [
      "plugins/analytix-fund-analysis/skills/report-builder/SKILL.md",
      "plugins/analytix-fund-analysis/skills/visual-evidence/SKILL.md",
      "plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md"
    ],
    markers: ["Completion Gate", "artifact", "blocked", "inspect"]
  },
  {
    id: "database_mcp_crystaldba_style_guardrails",
    mature_basis: ["crystaldba/postgres-mcp: restricted safe SQL, schema/object inspection, query diagnostics, explain plans, health and slow-path tooling"],
    analytix_files: [
      "plugins/analytix-fund-analysis/skills/case-workbench/SKILL.md",
      "plugins/analytix-fund-analysis/references/database-site-diagnostics.md",
      "plugins/analytix-fund-analysis/mcp/duckdb-workbench-runtime.mjs"
    ],
    markers: ["read-only", "inspect_case_schema", "diagnose_case_sql", "profile_case_schema", "case_sql_recipes"]
  },
  {
    id: "instructor_style_schema_validation",
    mature_basis: ["567-labs/instructor: schema-first response models, validation failure repair, hooks, failed attempts, and provider-agnostic structured outputs"],
    analytix_files: [
      "plugins/analytix-fund-analysis/references/investigation-answer-contract.md",
      "plugins/analytix-fund-analysis/mcp/investigation-answer-contract.mjs",
      "plugins/analytix-fund-analysis/skills/delivery-qc/SKILL.md"
    ],
    markers: ["FinalInvestigationAnswer", "validation", "MCP/DuckDB", "evidence_refs", "repair_actions"]
  },
  {
    id: "objective_gate_clarifies_only_when_material",
    mature_basis: [
      "Data Analytics: clarify when missing input materially changes the analytical frame; otherwise assume and proceed",
      "Investment Banking and Public Equity Investing: router selects a lead owner without doing specialist work or over-prompting"
    ],
    analytix_files: [
      "plugins/analytix-fund-analysis/skills/analytix-fund-analysis/SKILL.md",
      "plugins/analytix-fund-analysis/skills/index/SKILL.md",
      "plugins/analytix-fund-analysis/references/focused-skill-shared.md"
    ],
    markers: ["Investigation Objective Gate", "Ask at most three", "proceed without asking", "focused owner"]
  }
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function normalizedSearchText(value) {
  return text(value)
    .replace(/\s+/gu, " ")
    .replace(/[“”]/gu, "\"")
    .replace(/[`*_]/gu, "")
    .toLowerCase();
}

function timestamp() {
  return new Date().toISOString().replace(/[:.]/gu, "-");
}

function defaultOutputDir() {
  return path.join(REPO_ROOT, "output", "analytix-fund-analysis", "skill-clause-audit", timestamp());
}

function parseArgs(argv) {
  const options = {
    json: false,
    outputDir: "",
    failOnHardGaps: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--json") options.json = true;
    else if (arg === "--output-dir") options.outputDir = next();
    else if (arg.startsWith("--output-dir=")) options.outputDir = arg.slice("--output-dir=".length);
    else if (arg === "--fail-on-hard-gaps") options.failOnHardGaps = true;
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
  console.log(`Usage: node plugins/analytix-fund-analysis/scripts/skill-clause-audit.mjs [options]

Audits every Analytix fund-analysis SKILL.md clause against mature-plugin
patterns. It is a text-contract audit only; pair it with mcp-return-oracle for
real DuckDB return validation.

Options:
  --output-dir <dir>       Write JSON/Markdown evidence to this directory.
  --fail-on-hard-gaps      Exit non-zero for missing sections or parity blockers.
  --json                   Print JSON to stdout.
`);
}

function readText(relativeOrAbsolutePath) {
  const resolved = path.isAbsolute(relativeOrAbsolutePath)
    ? relativeOrAbsolutePath
    : path.join(REPO_ROOT, relativeOrAbsolutePath);
  return fs.readFileSync(resolved, "utf8");
}

function exists(relativeOrAbsolutePath) {
  const resolved = path.isAbsolute(relativeOrAbsolutePath)
    ? relativeOrAbsolutePath
    : path.join(REPO_ROOT, relativeOrAbsolutePath);
  return fs.existsSync(resolved);
}

function skillFiles() {
  const skillsRoot = path.join(PLUGIN_ROOT, "skills");
  return fs.readdirSync(skillsRoot, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => `plugins/analytix-fund-analysis/skills/${entry.name}/SKILL.md`)
    .filter(exists)
    .sort();
}

function classifyClause(body) {
  const categories = CLAUSE_CATEGORIES
    .filter(([, pattern]) => pattern.test(body))
    .map(([category]) => category);
  return categories.length ? categories : ["context"];
}

function extractClauses(file, body) {
  const clauses = [];
  let section = "";
  const lines = body.split(/\r?\n/u);
  for (let index = 0; index < lines.length; index += 1) {
    const raw = lines[index];
    const line = raw.trim();
    if (!line || line === "---" || line.startsWith("```")) continue;
    if (/^#{1,6}\s+/u.test(line)) {
      section = line.replace(/^#{1,6}\s+/u, "");
      clauses.push({
        file,
        line: index + 1,
        section,
        text: line,
        categories: ["section_header"]
      });
      continue;
    }
    if (/^\|?\s*:?-{3,}/u.test(line)) continue;
    if (/^(name|description):\s*/u.test(line)) continue;
    const normalized = line.replace(/^\s*(?:[-*]|\d+\.)\s*/u, "");
    clauses.push({
      file,
      line: index + 1,
      section,
      text: normalized,
      categories: classifyClause(normalized)
    });
  }
  return clauses;
}

function missingMarkers(body, markers) {
  const normalizedBody = normalizedSearchText(body);
  return markers.filter((marker) => !normalizedBody.includes(normalizedSearchText(marker)));
}

function auditRequiredSections(file, body) {
  const skillName = path.basename(path.dirname(file));
  const required = skillName === "index"
    ? ["## Use when", "## Not for", "## Workflow", "## Completion Gate"]
    : ["## Use when", "## Not for", "## Workflow", "## Output Contract", "## Completion Gate"];
  const missing = missingMarkers(body, required);
  return missing.map((marker) => ({
    severity: "p0",
    file,
    message: `${skillName} missing mature-skill section: ${marker}`
  }));
}

function auditOverconstraint(file, body) {
  const findings = [];
  const patterns = [
    {
      pattern: /必须逐字出现|按固定顺序写|不得改写成/iu,
      severity: "warning",
      message: "visible-answer wording appears over-constrained; prefer evidence obligations and allow professional equivalent headings unless a formal material requires exact text"
    },
    {
      pattern: /不得.*探索|禁止.*假设|只允许.*结论/iu,
      severity: "p1",
      message: "clause may restrict investigative exploration instead of only constraining fact acquisition"
    }
  ];
  for (const item of patterns) {
    if (item.pattern.test(body)) findings.push({ severity: item.severity, file, message: item.message });
  }
  return findings;
}

function auditSkillFiles(files) {
  const rows = [];
  const findings = [];
  for (const file of files) {
    const body = readText(file);
    const clauses = extractClauses(file, body);
    const categoryCounts = {};
    for (const clause of clauses) {
      for (const category of clause.categories) {
        categoryCounts[category] = Number(categoryCounts[category] || 0) + 1;
      }
    }
    findings.push(...auditRequiredSections(file, body));
    findings.push(...auditOverconstraint(file, body));
    rows.push({
      file,
      clause_count: clauses.length,
      category_counts: categoryCounts,
      clauses
    });
  }
  return { rows, findings };
}

function auditHardMarkers() {
  const findings = [];
  const rows = [];
  for (const check of HARD_REQUIRED_ANALYTIX_MARKERS) {
    if (!exists(check.file)) {
      findings.push({ severity: "p0", file: check.file, message: `${check.id} file missing` });
      rows.push({ id: check.id, status: "missing", file: check.file });
      continue;
    }
    const body = readText(check.file);
    const missing = missingMarkers(body, check.markers);
    if (missing.length) {
      findings.push({
        severity: "p0",
        file: check.file,
        message: `${check.id} missing marker(s): ${missing.join(", ")}`
      });
    }
    rows.push({
      id: check.id,
      status: missing.length ? "missing" : "passed",
      file: check.file,
      missing
    });
  }
  return { rows, findings };
}

function auditMatureBaselines() {
  const rows = [];
  const findings = [];
  for (const file of MATURE_SKILL_PATHS) {
    if (!exists(file)) {
      findings.push({
        severity: "p1",
        file,
        message: "mature plugin baseline SKILL.md is missing; parity audit cannot claim current Data Analytics / Banking / Equity comparison for this baseline"
      });
      rows.push({ file, status: "missing", clause_count: 0, category_counts: {} });
      continue;
    }
    const body = readText(file);
    const clauses = extractClauses(file, body);
    const categoryCounts = {};
    for (const clause of clauses) {
      for (const category of clause.categories) {
        categoryCounts[category] = Number(categoryCounts[category] || 0) + 1;
      }
    }
    rows.push({ file, status: "read", clause_count: clauses.length, category_counts: categoryCounts });
  }
  return { rows, findings };
}

function auditMatureParity() {
  const findings = [];
  const rows = [];
  for (const pattern of MATURE_PARITY_PATTERNS) {
    const combined = pattern.analytix_files
      .filter(exists)
      .map(readText)
      .join("\n");
    const missingFiles = pattern.analytix_files.filter((file) => !exists(file));
    const missingMarkers = missingMarkersInCombined(combined, pattern.markers);
    const status = missingFiles.length || missingMarkers.length ? "missing" : "passed";
    if (status !== "passed") {
      findings.push({
        severity: "p0",
        file: pattern.analytix_files.join(", "),
        message: `${pattern.id} not mature-plugin-parity complete; missing files=${missingFiles.join(", ") || "none"} markers=${missingMarkers.join(", ") || "none"}`
      });
    }
    rows.push({
      id: pattern.id,
      status,
      mature_basis: pattern.mature_basis,
      analytix_files: pattern.analytix_files,
      missing_files: missingFiles,
      missing_markers: missingMarkers
    });
  }
  return { rows, findings };
}

function missingMarkersInCombined(body, markers) {
  return missingMarkers(body, markers);
}

function auditReferenceClauses() {
  const rows = [];
  const findings = [];
  for (const file of REFERENCE_PATHS) {
    if (!exists(file)) {
      findings.push({ severity: "p1", file, message: "important reference missing" });
      rows.push({ file, status: "missing", clause_count: 0 });
      continue;
    }
    const body = readText(file);
    const clauses = extractClauses(file, body);
    rows.push({
      file,
      status: "read",
      clause_count: clauses.length,
      categories: [...new Set(clauses.flatMap((clause) => clause.categories))]
    });
  }
  return { rows, findings };
}

function summarizeFindings(findings) {
  const counts = {};
  for (const finding of findings) {
    counts[finding.severity] = Number(counts[finding.severity] || 0) + 1;
  }
  return counts;
}

function writeEvidence(outputDir, payload) {
  const resolved = path.resolve(REPO_ROOT, outputDir || defaultOutputDir());
  fs.mkdirSync(resolved, { recursive: true });
  const jsonPath = path.join(resolved, "skill-clause-audit.json");
  const mdPath = path.join(resolved, "skill-clause-audit.md");
  fs.writeFileSync(jsonPath, `${JSON.stringify(payload, null, 2)}\n`);
  fs.writeFileSync(mdPath, renderMarkdown(payload));
  return { output_dir: resolved, json_path: jsonPath, markdown_path: mdPath };
}

function renderMarkdown(payload) {
  const lines = [
    "# Analytix Skill Clause Audit",
    "",
    `- Status: ${payload.status}`,
    `- Generated: ${payload.generated_at}`,
    `- Analytix skill files: ${payload.skill_file_count}`,
    `- Analytix clauses: ${payload.clause_count}`,
    `- Findings: ${JSON.stringify(payload.finding_counts)}`,
    "",
    "## Mature Parity",
    "",
    "| Pattern | Status | Missing |",
    "| --- | --- | --- |"
  ];
  for (const row of payload.mature_parity || []) {
    lines.push(`| ${row.id} | ${row.status} | ${[...(row.missing_files || []), ...(row.missing_markers || [])].join(", ").replace(/\|/gu, "\\|")} |`);
  }
  lines.push("", "## Findings");
  if (!payload.findings.length) {
    lines.push("- No hard gaps or warnings.");
  } else {
    for (const finding of payload.findings) {
      lines.push(`- ${finding.severity}: ${finding.file || ""} ${finding.message}`.trim());
    }
  }
  lines.push("", "## Skill Clause Coverage", "", "| Skill | Clauses | Key Categories |", "| --- | ---: | --- |");
  for (const row of payload.skills || []) {
    lines.push(`| ${row.file} | ${row.clause_count} | ${Object.keys(row.category_counts || {}).join(", ")} |`);
  }
  lines.push("");
  return `${lines.join("\n")}\n`;
}

function runAudit() {
  const files = skillFiles();
  const skillAudit = auditSkillFiles(files);
  const hardMarkers = auditHardMarkers();
  const matureBaselines = auditMatureBaselines();
  const matureParity = auditMatureParity();
  const referenceAudit = auditReferenceClauses();
  const findings = [
    ...skillAudit.findings,
    ...hardMarkers.findings,
    ...matureBaselines.findings,
    ...matureParity.findings,
    ...referenceAudit.findings
  ];
  const hardFindings = findings.filter((finding) => ["p0", "p1"].includes(finding.severity));
  const clauseCount = skillAudit.rows.reduce((sum, row) => sum + row.clause_count, 0);
  return {
    status: hardFindings.length ? "needs_attention" : "ok",
    generated_at: new Date().toISOString(),
    audit: "analytix-skill-clause-audit",
    basis: {
      mature_plugin_paths: MATURE_SKILL_PATHS,
      reference_paths: REFERENCE_PATHS,
      external_mcp_sources_considered: [
      "https://github.com/crystaldba/postgres-mcp",
      "https://github.com/567-labs/instructor",
      "https://github.com/motherduckdb/mcp-server-motherduck",
        "https://github.com/boettiger-lab/mcp-server-duckdb"
      ]
    },
    skill_file_count: files.length,
    clause_count: clauseCount,
    finding_counts: summarizeFindings(findings),
    hard_finding_count: hardFindings.length,
    skills: skillAudit.rows.map(({ clauses, ...row }) => row),
    clause_inventory: skillAudit.rows.flatMap((row) => row.clauses),
    required_markers: hardMarkers.rows,
    mature_baselines: matureBaselines.rows,
    mature_parity: matureParity.rows,
    references: referenceAudit.rows,
    findings
  };
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const payload = runAudit();
  const evidence = writeEvidence(options.outputDir, payload);
  payload.output = {
    output_dir: path.relative(REPO_ROOT, evidence.output_dir),
    json_path: path.relative(REPO_ROOT, evidence.json_path),
    markdown_path: path.relative(REPO_ROOT, evidence.markdown_path)
  };
  if (options.json) {
    const { clause_inventory: _clauses, ...compactPayload } = payload;
    compactPayload.clause_inventory = {
      omitted_from_stdout: true,
      reason: "Full per-clause inventory is written to the JSON evidence file.",
      count: payload.clause_inventory.length
    };
    console.log(JSON.stringify(compactPayload, null, 2));
  } else {
    console.log(`Skill clause audit: ${payload.status}`);
    console.log(`  skills: ${payload.skill_file_count}`);
    console.log(`  clauses: ${payload.clause_count}`);
    console.log(`  hard findings: ${payload.hard_finding_count}`);
    console.log(`  output: ${payload.output.json_path}`);
  }
  if (options.failOnHardGaps && payload.hard_finding_count) process.exitCode = 1;
}

main();
