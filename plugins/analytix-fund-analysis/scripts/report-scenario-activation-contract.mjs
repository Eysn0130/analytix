#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { scoreAnswers, TASKS } from "./model-ab-eval.mjs";

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");
const ORDINARY_FULL_CASE_TASK_IDS = [
  "full_case_analysis_tree",
  "replay_full_case_simple_prompt_runs_tree"
];
const FORCED_TYPOLOGY_PATTERN = /(?:团伙|共同控制|对公向个人|现金同存同取|正常成本|利益输送|资产消费|投资理财|支付通道|商户平台|虚拟资产|OTC|境外|跨境|票税合同|工程(?:项目)?|项目款|串通投标|围标|行贿|受贿)/iu;
const INACTIVE_SCENARIO_PATTERN = /(?:工程(?:项目)?|项目款|利益输送|串通投标|围标|行贿|受贿)/iu;

function text(value) {
  return value == null ? "" : String(value);
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function readRepo(relativePath) {
  return fs.readFileSync(path.resolve(REPO_ROOT, relativePath), "utf8");
}

function assertMarkers(label, body, markers) {
  const missing = markers.filter((marker) => !body.includes(marker));
  assert(missing.length === 0, `${label} missing marker(s): ${missing.join(", ")}`);
}

function taskById(taskId) {
  const task = TASKS.find((item) => text(item.id) === taskId);
  assert(task, `missing model A/B task: ${taskId}`);
  return task;
}

function scoringBlock(source, taskId) {
  const marker = `if (record.task_id === "${taskId}"`;
  const start = source.indexOf(marker);
  assert(start >= 0, `missing scoring block: ${taskId}`);
  const next = source.indexOf("\n  if (record.task_id ===", start + marker.length);
  return source.slice(start, next >= 0 ? next : source.length);
}

function scoreNonProjectAnswer(answer) {
  const result = scoreAnswers([
    {
      task_id: "replay_economic_crime_typology_leads",
      case_id: "case_non_project_contract",
      mode: "full_plugin",
      answer,
      tool_calls: [
        {
          server: "analytix_funds",
          tool: "get_case_scope_map",
          status: "ok"
        }
      ]
    }
  ]);
  assert(result.invalid_run !== true, `non-project scoring fixture invalid: ${text(result.abort_reason)}`);
  assert(Array.isArray(result.results) && result.results.length === 1, "non-project scoring fixture produced no result");
  return result.results[0];
}

function main() {
  const rootSchema = readRepo("plugins/analytix-fund-analysis/references/report-schema.md");
  const embeddedSchema = readRepo("plugins/analytix-fund-analysis/skills/analytix-fund-analysis/references/report-schema.md");
  const reportSkill = readRepo("plugins/analytix-fund-analysis/skills/report-builder/SKILL.md");
  const reportAgent = readRepo("plugins/analytix-fund-analysis/skills/report-builder/agents/openai.yaml");
  const modelEvalSource = readRepo("plugins/analytix-fund-analysis/scripts/model-ab-eval.mjs");

  assert(rootSchema === embeddedSchema, "root and embedded report-schema copies differ");
  assertMarkers("report schema scenario gate", rootSchema, [
    "## Scenario Activation Gate",
    "host-authoritative",
    "same thread, turn, case binding, context epoch, and dataset snapshot",
    "source capability that supports the exact claim type and fields",
    "Engineering/project identity",
    "Project-fund flow",
    "Benefit-transfer lead",
    "Collusive-bidding or bid-rigging lead",
    "Bribery/corruption lead",
    "omit its heading, narrative, table, chart, and placeholder",
    "ordinary full-case report is therefore not a typology checklist"
  ]);
  assertMarkers("report-builder scenario gate", reportSkill, [
    "Scenario Activation Gate",
    "同 thread、turn",
    "source capability 支持该具体 claim",
    "未激活时整段省略",
    "普通全案不是 typology checklist"
  ]);
  assertMarkers("report-builder agent scenario gate", reportAgent, [
    "当前报告链处于 P0 隔离期",
    "不要调用或模拟 run_full_case_analysis",
    "不生成报告草稿",
    "补证动作"
  ]);

  for (const taskId of ORDINARY_FULL_CASE_TASK_IDS) {
    const task = taskById(taskId);
    const promptedOrRewarded = [task.prompt, ...(task.expectedMarkers || [])].map(text).join("\n");
    assert(
      !FORCED_TYPOLOGY_PATTERN.test(promptedOrRewarded),
      `${taskId} still prompts or rewards fixed typology coverage`
    );
    assert(
      !FORCED_TYPOLOGY_PATTERN.test(scoringBlock(modelEvalSource, taskId)),
      `${taskId} scoring block still rewards fixed typology markers`
    );
  }

  const reportGateTask = taskById("full_report_gate");
  assert(
    ![reportGateTask.prompt, ...(reportGateTask.expectedMarkers || [])].map(text).join("\n").includes("必查专题"),
    "full_report_gate still treats every typology as a mandatory report section"
  );

  const negativeTask = taskById("replay_economic_crime_typology_leads");
  assert(negativeTask.scenarioFixture === "non_project_transactions_only", "non-project scenario fixture is missing");
  assert(
    Array.isArray(negativeTask.forbiddenOutputPatterns) && negativeTask.forbiddenOutputPatterns.length > 0,
    "non-project scenario fixture lacks deterministic inactive-section rejection"
  );

  const safe = scoreNonProjectAnswer(
    "已查范围：仅核验当前案件交易流水及其时间、账户和金额字段。事实边界：现有材料只支持交易记录本身，不能认定超出该来源能力的专题事实。数据缺口包括其他权威业务材料和人工复核记录。下一步补证：按用户明确的调查方向调取相应权威材料后逐项复核。"
  );
  const unsafe = scoreNonProjectAnswer(
    "已查范围：当前案件交易流水。\n\n## 项目款与利益输送\n现有转账形态可作为该专题故事的占位内容。"
  );
  const scenarioFailure = "non-project case rendered an inactive report scenario";
  assert(!safe.auto_fail.includes(scenarioFailure), "safe non-project answer was rejected by the scenario gate");
  assert(unsafe.auto_fail.includes(scenarioFailure), "inactive non-project scenario section was not rejected");
  assert(INACTIVE_SCENARIO_PATTERN.test("项目款与利益输送"), "inactive scenario test pattern is ineffective");

  const payload = {
    status: "ok",
    schema_copies_match: true,
    ordinary_full_case_task_count: ORDINARY_FULL_CASE_TASK_IDS.length,
    ordinary_full_case_forced_typology_markers: 0,
    non_project_safe_answer_accepted_by_scenario_gate: true,
    non_project_inactive_scenario_rejected: true
  };
  if (process.argv.includes("--json")) {
    console.log(JSON.stringify(payload, null, 2));
  } else {
    console.log("Report scenario activation contract: ok");
  }
}

try {
  main();
} catch (error) {
  const payload = {
    status: "failed",
    error: error instanceof Error ? error.message : String(error)
  };
  if (process.argv.includes("--json")) {
    console.log(JSON.stringify(payload, null, 2));
  } else {
    console.error(payload.error);
  }
  process.exitCode = 1;
}
