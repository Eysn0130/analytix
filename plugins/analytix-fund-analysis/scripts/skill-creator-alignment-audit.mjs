#!/usr/bin/env node

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");
const repoRoot = path.resolve(pluginRoot, "..", "..");

const REQUIRED_SECTIONS = [
  "## Use when",
  "## Not for",
  "## Workflow",
  "## Output Contract",
  "## Completion Gate"
];

const FORBIDDEN_SKILL_FACT_PATTERNS = [
  /江苏航案件分析/u,
  /河南颍淮建工/u,
  /analysis_txn_detail_idx\s*=\s*2645472/u,
  /2,645,472/u,
  /frontdoor-p0-oracle\/2026/u,
  /golden answer/u,
  /golden-answer/u
];

function codexPluginCachePath(...segments) {
  return path.join(os.homedir(), ".codex", "plugins", "cache", ...segments);
}

const MATURE_SOURCE_EXPECTATIONS = [
  {
    id: "data_analytics_index",
    kind: "local_mature_plugin",
    path: codexPluginCachePath("openai-curated-remote", "data-analytics", "0.1.49-2470779139f2", "skills", "index", "SKILL.md"),
    borrowed_rules: [
      "router chooses the narrowest focused workflow",
      "source access guardrail before claims",
      "live source reads before data-backed conclusions",
      "user-facing language hides implementation terms"
    ]
  },
  {
    id: "data_analytics_validate_report_visual_notebook",
    kind: "local_mature_plugin",
    paths: [
      codexPluginCachePath("openai-curated-remote", "data-analytics", "0.1.49-2470779139f2", "skills", "validate-data", "SKILL.md"),
      codexPluginCachePath("openai-curated-remote", "data-analytics", "0.1.49-2470779139f2", "skills", "build-report", "SKILL.md"),
      codexPluginCachePath("openai-curated-remote", "data-analytics", "0.1.49-2470779139f2", "skills", "visualize-data", "SKILL.md"),
      codexPluginCachePath("openai-curated-remote", "data-analytics", "0.1.49-2470779139f2", "skills", "jupyter-notebooks", "SKILL.md")
    ],
    borrowed_rules: [
      "validate claims, calculations, visuals, caveats, and conclusions",
      "artifact/report/chart/notebook work is incomplete until rendered or execution gaps are recorded",
      "notebooks are reproducible artifacts, not scratchpad dumps"
    ]
  },
  {
    id: "investment_banking_router_qc",
    kind: "local_mature_plugin",
    paths: [
      codexPluginCachePath("openai-curated-remote", "investment-banking", "0.1.27", "skills", "investment-banking", "SKILL.md"),
      codexPluginCachePath("openai-curated-remote", "investment-banking", "0.1.27", "skills", "memo-builder", "SKILL.md"),
      codexPluginCachePath("openai-curated-remote", "investment-banking", "0.1.27", "skills", "ib-deck-qc", "SKILL.md")
    ],
    borrowed_rules: [
      "router owns admission and lead-skill selection only",
      "lead workflow owns deliverable intake and final response",
      "support artifacts and handoffs are not hero deliverables",
      "QC ties material numbers, sources, charts, and conclusions before circulation"
    ]
  },
  {
    id: "public_equity_router_pm_judgment",
    kind: "local_mature_plugin",
    paths: [
      codexPluginCachePath("openai-curated-remote", "public-equity-investing", "0.1.29", "skills", "public-equity-investing", "SKILL.md"),
      codexPluginCachePath("openai-curated-remote", "public-equity-investing", "0.1.29", "skills", "memo-builder", "SKILL.md"),
      codexPluginCachePath("openai-curated-remote", "public-equity-investing", "0.1.29", "skills", "deck-report-qc", "SKILL.md")
    ],
    borrowed_rules: [
      "owner skill owns investment judgment and user-facing synthesis",
      "substantial memos need source posture, decision hinge, disconfirmers, and action discipline",
      "QC separates confirmed mismatch, externally verified error, and needs review"
    ]
  },
  {
    id: "openai_codex_skills_manual",
    kind: "official_docs",
    url: "https://developers.openai.com/codex/skills",
    borrowed_rules: [
      "skills use progressive disclosure",
      "description is the implicit-trigger surface",
      "keep each skill focused on one job",
      "use plugins as the installable distribution unit"
    ]
  },
  {
    id: "anthropic_agent_skills",
    kind: "official_docs",
    url: "https://docs.anthropic.com/en/docs/agents-and-tools/agent-skills/overview",
    borrowed_rules: [
      "skills package instructions, scripts, and resources",
      "progressive disclosure keeps context efficient",
      "scripts carry deterministic repeated work",
      "skills should be forward-tested on realistic tasks"
    ]
  }
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function timestamp() {
  return new Date().toISOString().replace(/[:.]/gu, "-");
}

function readJson(relativePath) {
  return JSON.parse(fs.readFileSync(path.join(pluginRoot, relativePath), "utf8"));
}

function parseArgs(argv) {
  const options = {
    json: false,
    failOnGaps: false,
    outputRoot: path.join(repoRoot, "output", "analytix-fund-analysis", "skill-creator-alignment")
  };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--json") options.json = true;
    else if (arg === "--fail-on-gaps") options.failOnGaps = true;
    else if (arg === "--output-root") options.outputRoot = argv[++i];
    else if (arg.startsWith("--output-root=")) options.outputRoot = arg.slice("--output-root=".length);
    else if (arg === "--help" || arg === "-h") {
      console.log(`Usage: node scripts/skill-creator-alignment-audit.mjs [--json] [--fail-on-gaps] [--output-root <dir>]`);
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  return options;
}

function parseFrontmatter(markdown) {
  const match = markdown.match(/^---\n([\s\S]*?)\n---\n?/u);
  if (!match) return { fields: {}, body: markdown, raw: "" };
  const fields = {};
  for (const line of match[1].split(/\r?\n/u)) {
    const field = line.match(/^([A-Za-z0-9_-]+):\s*(.*)$/u);
    if (!field) continue;
    fields[field[1]] = field[2].replace(/^"|"$/gu, "").trim();
  }
  return { fields, body: markdown.slice(match[0].length), raw: match[1] };
}

function skillFiles() {
  const skillsRoot = path.join(pluginRoot, "skills");
  return fs.readdirSync(skillsRoot)
    .sort()
    .map((name) => ({ name, file: path.join(skillsRoot, name, "SKILL.md") }))
    .filter((item) => fs.existsSync(item.file));
}

function relativeToPlugin(filePath) {
  return path.relative(pluginRoot, filePath).split(path.sep).join("/");
}

function markdownLinks(markdown) {
  const links = [];
  const regex = /\[[^\]]+\]\(([^)]+)\)/gu;
  for (const match of markdown.matchAll(regex)) {
    const target = match[1].trim();
    if (!target || target.startsWith("#") || /^[a-z][a-z0-9+.-]*:/iu.test(target)) continue;
    links.push(target.split("#")[0]);
  }
  return links;
}

function linkExists(fromFile, link) {
  const normalized = link.replace(/^<|>$/gu, "");
  const target = path.resolve(path.dirname(fromFile), normalized);
  return fs.existsSync(target);
}

function auditSkill(item) {
  const content = fs.readFileSync(item.file, "utf8");
  const lineCount = content.split(/\r?\n/u).length;
  const { fields, raw } = parseFrontmatter(content);
  const failures = [];
  const warnings = [];
  const references = markdownLinks(content);
  const brokenLinks = references.filter((link) => !linkExists(item.file, link));
  const missingSections = REQUIRED_SECTIONS.filter((section) => !content.includes(section));
  const forbiddenMatches = FORBIDDEN_SKILL_FACT_PATTERNS
    .filter((pattern) => pattern.test(content))
    .map((pattern) => pattern.source);

  if (fields.name !== item.name) failures.push(`frontmatter name ${fields.name || "<missing>"} != folder ${item.name}`);
  if (!text(fields.description)) failures.push("frontmatter description missing");
  if (raw.split(/\r?\n/u).some((line) => line && !/^name:|^description:/u.test(line))) {
    failures.push("frontmatter must only contain name and description");
  }
  if (text(fields.description).length > 240) warnings.push(`description is long (${text(fields.description).length} chars)`);
  if (lineCount > 500) failures.push(`SKILL.md exceeds 500 lines (${lineCount})`);
  else if (lineCount > 120) warnings.push(`SKILL.md polish warning: ${lineCount} lines`);
  if (missingSections.length > 0) failures.push(`missing required sections: ${missingSections.join(", ")}`);
  if (brokenLinks.length > 0) failures.push(`broken markdown links: ${brokenLinks.join(", ")}`);
  if (forbiddenMatches.length > 0) failures.push(`production skill contains forbidden eval/case facts: ${forbiddenMatches.join(", ")}`);

  return {
    skill: item.name,
    path: relativeToPlugin(item.file),
    line_count: lineCount,
    description_chars: text(fields.description).length,
    reference_links: references.length,
    broken_links: brokenLinks,
    missing_sections: missingSections,
    warnings,
    failures,
    status: failures.length > 0 ? "failed" : "ok"
  };
}

function auditMatureSources() {
  return MATURE_SOURCE_EXPECTATIONS.map((source) => {
    const paths = [source.path, ...(source.paths || [])].filter(Boolean);
    const missing = paths.filter((item) => !fs.existsSync(item));
    return {
      ...source,
      paths,
      path_status: paths.length === 0 ? "external" : (missing.length === 0 ? "present" : "missing"),
      missing_paths: missing
    };
  });
}

function auditProductionResourceBoundary() {
  const runtimeContract = fs.readFileSync(path.join(pluginRoot, "scripts", "runtime-cache-contract.mjs"), "utf8");
  const forbidden = [
    "scripts/eval-fixtures",
    "golden-answer-set.json",
    "golden-eval-rubric.md",
    "ANALYTIX_FUNDS_EVAL_FAST_PATH"
  ];
  return {
    runtime_cache_contract_path: "scripts/runtime-cache-contract.mjs",
    forbidden_patterns_declared: forbidden.filter((pattern) => runtimeContract.includes(pattern)),
    missing_forbidden_patterns: forbidden.filter((pattern) => !runtimeContract.includes(pattern))
  };
}

function buildAudit() {
  const manifest = readJson(".codex-plugin/plugin.json");
  const skillResults = skillFiles().map(auditSkill);
  const matureSources = auditMatureSources();
  const productionBoundary = auditProductionResourceBoundary();
  const failures = [
    ...skillResults.flatMap((item) => item.failures.map((failure) => `${item.skill}: ${failure}`)),
    ...matureSources.flatMap((item) => item.missing_paths.map((missing) => `${item.id}: missing ${missing}`)),
    ...productionBoundary.missing_forbidden_patterns.map((pattern) => `runtime cache contract missing forbidden pattern ${pattern}`)
  ];
  const warnings = skillResults.flatMap((item) => item.warnings.map((warning) => `${item.skill}: ${warning}`));
  return {
    id: "skill_creator_alignment",
    generated_at: new Date().toISOString(),
    plugin: {
      name: manifest.name,
      version: manifest.version
    },
    sources: {
      openai_codex_manual: "https://developers.openai.com/codex/skills",
      openai_plugins_manual: "https://developers.openai.com/codex/plugins/build",
      anthropic_agent_skills: "https://docs.anthropic.com/en/docs/agents-and-tools/agent-skills/overview",
      local_mature_plugins: matureSources
    },
    criteria: {
      max_skill_lines: 500,
      polish_warning_lines: 120,
      required_sections: REQUIRED_SECTIONS,
      frontmatter_fields: ["name", "description"],
      production_skill_forbidden_fact_patterns: FORBIDDEN_SKILL_FACT_PATTERNS.map((pattern) => pattern.source)
    },
    skill_results: skillResults,
    production_resource_boundary: productionBoundary,
    summary: {
      skill_count: skillResults.length,
      failed_skill_count: skillResults.filter((item) => item.status === "failed").length,
      warning_count: warnings.length,
      mature_source_count: matureSources.length,
      missing_mature_source_count: matureSources.filter((item) => item.path_status === "missing").length,
      failure_count: failures.length
    },
    warnings,
    failures,
    status: failures.length === 0 ? "ok" : "failed"
  };
}

function writeAudit(audit, outputRoot) {
  const outDir = path.join(outputRoot, timestamp());
  fs.mkdirSync(outDir, { recursive: true });
  const jsonPath = path.join(outDir, "skill-creator-alignment.json");
  const mdPath = path.join(outDir, "skill-creator-alignment.md");
  fs.writeFileSync(jsonPath, `${JSON.stringify(audit, null, 2)}\n`);
  fs.writeFileSync(mdPath, [
    "# Skill Creator Alignment Audit",
    "",
    `Status: ${audit.status}`,
    `Plugin: ${audit.plugin.name}@${audit.plugin.version}`,
    `Skills: ${audit.summary.skill_count}`,
    `Failures: ${audit.summary.failure_count}`,
    `Warnings: ${audit.summary.warning_count}`,
    "",
    "## Mature Sources",
    "",
    ...audit.sources.local_mature_plugins.map((source) => `- ${source.id}: ${source.path_status}`),
    "",
    "## Skill Results",
    "",
    ...audit.skill_results.map((item) => `- ${item.skill}: ${item.status}; lines=${item.line_count}; warnings=${item.warnings.length}; failures=${item.failures.length}`),
    "",
    audit.failures.length ? "## Failures" : "## Failures\n\nNone.",
    ...(audit.failures.length ? ["", ...audit.failures.map((failure) => `- ${failure}`)] : []),
    ""
  ].join("\n"));
  return { outDir, jsonPath, mdPath };
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  const audit = buildAudit();
  const output = writeAudit(audit, options.outputRoot || path.join(os.tmpdir(), "analytix-skill-audit"));
  const result = { ...audit, output };
  if (options.json) console.log(JSON.stringify(result, null, 2));
  else {
    console.log(`status=${audit.status}`);
    console.log(`output=${output.outDir}`);
    for (const failure of audit.failures) console.log(`FAIL ${failure}`);
    for (const warning of audit.warnings) console.log(`WARN ${warning}`);
  }
  if (options.failOnGaps && audit.status !== "ok") process.exit(1);
}

main();
