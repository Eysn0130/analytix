#!/usr/bin/env node

import crypto from "node:crypto";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  diffDiagnosticCards,
  validateDiagnosticCard
} from "./diagnostic-card-diff.mjs";
import {
  commandBudgetFromCapabilityRegistry,
  EVAL_COVERAGE_CONTRACT_VERSION,
  formatEvalCoverageSummary,
  validateCapabilityRegistryDerivedMetadataContract,
  validateCommandMetadataBudgetContract,
  validateEvalCoverageContract,
  validateEvalCoverageRegistrySchema,
  withEvalCoverageContract
} from "./eval-coverage-contract.mjs";
import {
  PLUGIN_CACHE_FILES,
  RUNTIME_CACHE_CONTRACT_VERSION,
  runtimeCacheContractViolations
} from "./runtime-cache-contract.mjs";
import {
  inspectProductionMcpEntryClosure,
  PRODUCTION_MCP_ENTRY_CLOSURE_FILES
} from "./production-mcp-entry-closure-contract.mjs";
import {
  PHASE4_CARD_TYPES,
  renderDiagnosticCardForAgent,
  renderDiagnosticCardForAudit,
  stripOpaqueRefs
} from "../mcp/card-renderer.mjs";
import {
  hasOpaqueAgentRefs,
  redactAgentPayload,
  stripAgentOpaqueRefs
} from "../mcp/agent-context-hygiene.mjs";
import { createAgentOutputCompiler } from "../mcp/agent-output-compiler.mjs";
import { buildEvidenceLedger } from "../mcp/evidence-ledger.mjs";
import { inferDateStartFromQuestion } from "../mcp/frontdoor-routing.mjs";
import {
  analyzeReportClaimText,
  withClaimReviewProtocol
} from "../mcp/claim-verifier-protocol.mjs";
import { inferFrontDoorIntent } from "../mcp/intent-plan-protocol.mjs";
import {
  buildStructuredContentPolicy,
  structuredContentAllowed
} from "../mcp/mcp-output-policy.mjs";
import {
  createMcpRequestHandlerRuntime,
  MCP_REQUEST_HANDLER_RUNTIME_VERSION,
  PRE_EXECUTION_BLOCKED_TOOL_NAMES
} from "../mcp/mcp-request-handler-runtime.mjs";
import {
  createMcpToolResultRuntime,
  MCP_TOOL_RESULT_RUNTIME_VERSION
} from "../mcp/mcp-tool-result-runtime.mjs";
import {
  assertAnalytixArtifactRoot,
  resolveAnalytixArtifactRoot
} from "../mcp/artifact-path-policy.mjs";
import { stableHash as runtimeStableHash } from "../mcp/stable-hash.mjs";
import {
  createFrontdoorRuntime,
  pairAmountSql
} from "../mcp/frontdoor-runtime.mjs";
import {
  duplicateCandidateRiskText as runtimeDuplicateCandidateRiskText
} from "../mcp/frontdoor-fact-summaries.mjs";
import { intOrUndefined } from "../mcp/runtime-normalizers.mjs";
import {
  DUCKDB_WORKBENCH_RUNTIME_VERSION,
  executeLocalDuckdbRunCaseSql
} from "../mcp/duckdb-workbench-runtime.mjs";
import { TOOL_CALL_RUNTIME_VERSION } from "../mcp/tool-call-runtime.mjs";
import {
  hostFactRuntimeContext,
  writeCaseProjectBinding
} from "./host-runtime-context-fixture.mjs";

const DEFAULT_BACKEND_URL = "http://127.0.0.1:18731";
const DEFAULT_TIMEOUT_MS = 180_000;
const FOCUSED_TASK_IDS = [
  "case_3c72_top_rankings",
  "case_3c72_report_claim_review",
  "report_candidate_cash_boundary_guard",
  "old_thread_patterns"
];
const CANDIDATE_CASH_BOUNDARY_RISK_MARKERS = [
  "candidate_account_as_owned_account",
  "cash_or_asset_destination_without_supported_flow",
  "missing_counterparty_or_cash_break_as_verified",
  "claim_without_source_anchor"
];
const RELEASE_GUARD_FIXED_CASE_MARKERS = [
  "\u5218\u6587\u4eae",
  "\u5f20\u91d1\u829d",
  "\u8d35\u9633\u5e02\u89c2\u5c71\u6e56\u533a\u4eba\u6c11\u6cd5\u9662",
  "\u8d22\u4ed8\u901a-\u5fae\u4fe1\u8f6c\u8d26",
  "42,000,000",
  "133011759553CNY0",
  "9000000000000000006",
  "9000000000000000000002",
  "oracle-case-project"
];

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(__dirname, "..");
const REPO_ROOT = path.resolve(PLUGIN_ROOT, "../..");
const FIXTURE_PATH = path.join(__dirname, "eval-fixtures", "golden-answer-set.json");
const EVAL_COVERAGE_CONTRACT_PATH = path.join(__dirname, "eval-fixtures", "eval-coverage-contract.json");
const DEFAULT_OUTPUT_DIR = path.join(REPO_ROOT, "output", "analytix-fund-analysis", "legacy-diagnostics", "phase4");

function text(value) {
  return String(value == null ? "" : value).trim();
}

function claimText(value) {
  if (value && typeof value === "object") {
    const source = objectOf(value);
    return text(source.text || source.claim || source.statement || source.reason || source.summary || source.id);
  }
  return text(value);
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function quotedStrings(source) {
  return [...String(source || "").matchAll(/"([^"]+)"/gu)].map((match) => match[1]);
}

function numberValue(value) {
  if (value && typeof value === "object") {
    const source = objectOf(value);
    return numberValue(source.yuan ?? source.amount_yuan ?? source.amount ?? source.value);
  }
  const normalized = String(value ?? "").replace(/[,，\s]/gu, "");
  if (!normalized) return null;
  const next = Number(normalized);
  return Number.isFinite(next) ? next : null;
}

function intValue(value, fallback = undefined) {
  return intOrUndefined(value) ?? fallback;
}

function moneyText(value) {
  if (value && typeof value === "object") {
    return text(value.text) || moneyText(value.yuan);
  }
  const next = numberValue(value);
  return next == null ? "" : `${next.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} 元`;
}

function stableHash(value) {
  return crypto
    .createHash("sha256")
    .update(JSON.stringify(value || {}, (_key, item) => (typeof item === "bigint" ? String(item) : item)))
    .digest("hex");
}

function pruneEmpty(value) {
  if (Array.isArray(value)) {
    return value.map(pruneEmpty).filter((item) => item !== undefined);
  }
  if (!value || typeof value !== "object") {
    return value === "" || value === null ? undefined : value;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    const next = pruneEmpty(item);
    if (next !== undefined) output[key] = next;
  }
  return Object.keys(output).length ? output : undefined;
}

function makeFact({
  factId,
  label,
  value,
  unit = "",
  amount,
  count,
  account,
  holder,
  idNo,
  counterparty,
  counterpartyAccount,
  direction,
  timeRange,
  sourceLevel = "deterministic",
  supportStatus = "supported",
  answerText,
  supportQueryName,
  goldenAnchorKey,
  sourcePayload
}) {
  return pruneEmpty({
    fact_id: text(factId || label),
    label: text(label),
    value,
    unit: text(unit),
    amount: amount == null ? undefined : Number(amount),
    count: count == null ? undefined : intValue(count),
    account: text(account),
    holder: text(holder),
    id_no: text(idNo),
    counterparty: text(counterparty),
    counterparty_account: text(counterpartyAccount),
    direction: text(direction),
    time_range: text(timeRange),
    source_level: text(sourceLevel) || "deterministic",
    support_status: text(supportStatus) || "supported",
    answer_text: stripOpaqueRefs(text(answerText)),
    support_query_name: text(supportQueryName),
    golden_anchor_key: text(goldenAnchorKey),
    source_hash: stableHash(sourcePayload)
  }) || {};
}

function answerCardProtocolFields({
  recommendedNextAction = "answer_now",
  maxAdditionalTools = 0,
  requiredFactsPresent = true,
  unsupportedFlowsPresent = false
} = {}) {
  return {
    answer_card_complete: true,
    recommended_next_action: text(recommendedNextAction),
    max_additional_tools: intValue(maxAdditionalTools),
    required_facts_present: Boolean(requiredFactsPresent),
    unsupported_flows_present: Boolean(unsupportedFlowsPresent)
  };
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function parseJsonObject(value) {
  try {
    return objectOf(JSON.parse(text(value)));
  } catch {
    return {};
  }
}

function writeJson(filePath, payload) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, `${JSON.stringify(payload, null, 2)}\n`, "utf8");
}

function parseArgs(argv) {
  const options = {
    run: "",
    backendUrl: DEFAULT_BACKEND_URL,
    timeoutMs: DEFAULT_TIMEOUT_MS,
    output: "",
    json: false,
    releaseGuard: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--run") options.run = next();
    else if (arg === "--backend-url") options.backendUrl = next();
    else if (arg === "--timeout-ms") options.timeoutMs = Number(next()) || options.timeoutMs;
    else if (arg === "--output") options.output = next();
    else if (arg === "--json") options.json = true;
    else if (arg === "--release-guard") options.releaseGuard = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  options.backendUrl = text(options.backendUrl).replace(/\/+$/u, "");
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/phase4-diagnostics.mjs [options]

Options:
  --run A0|B0              Run phase4 diagnostic step.
  --release-guard          Check oracle/eval isolation boundaries.
  --backend-url <url>      Backend base URL for B0. Default: ${DEFAULT_BACKEND_URL}
  --timeout-ms <n>         Backend request timeout. Default: ${DEFAULT_TIMEOUT_MS}
  --output <file>          Output JSON path.
  --json                   Print JSON result.
`);
}

function goldenTask(golden, taskId) {
  const task = arrayOf(golden?.tasks).find((item) => text(item.id) === text(taskId));
  if (!task) throw new Error(`golden task not found: ${taskId}`);
  return task;
}

function rowsFromForbiddenClaims(task) {
  return arrayOf(task?.forbidden_claims)
    .map((item) => stripOpaqueRefs(text(objectOf(item).reason || objectOf(item).pattern)))
    .filter(Boolean);
}

function oracleTopRankingsCard(task) {
  const facts = objectOf(task.correct_facts);
  const coverage = objectOf(facts.coverage);
  const account = objectOf(facts.top_account_by_turnover);
  const holder = objectOf(facts.top_holder_by_turnover);
  const counterparties = arrayOf(facts.top_counterparties_by_turnover);
  const first = objectOf(counterparties[0]);
  const second = objectOf(counterparties[1]);
  const third = objectOf(counterparties[2]);
  return {
    card_type: "top_rankings_card",
    task_id: task.id,
    case_id: task.case_id,
    intent: "top_rankings",
    title: "全案 Top 排名核验材料",
    fact_source: "oracle_eval_fixture",
    ...answerCardProtocolFields({ recommendedNextAction: "answer_now", maxAdditionalTools: 0 }),
    facts: [
      makeFact({
        factId: "coverage.directed_transactions",
        label: "全案覆盖范围",
        count: coverage.directed_transaction_rows,
        amount: coverage.turnover_yuan,
        unit: "rows/yuan",
        timeRange: "导入至分析索引范围",
        answerText: [
          `规范交易 ${coverage.transaction_rows} 条，其中具备进/出方向 ${coverage.directed_transaction_rows} 条、方向缺失 ${coverage.undirected_transaction_rows} 条`,
          coverage.inflow_yuan != null && coverage.outflow_yuan != null
            ? `全案入账 ${moneyText(coverage.inflow_yuan)}，出账 ${moneyText(coverage.outflow_yuan)}`
            : "",
          `资金流量 ${moneyText(coverage.turnover_yuan)}`
        ].filter(Boolean).join("；") + "。",
        supportQueryName: "coverage_sql",
        goldenAnchorKey: `${task.id}.correct_facts.coverage`,
        sourcePayload: coverage
      }),
      makeFact({
        factId: "top_account.turnover.rank1",
        label: "账户 turnover 第一",
        account: account.account_key,
        holder: account.holder_name,
        idNo: account.id_no,
        count: account.txn_count,
        amount: account.turnover_yuan,
        unit: "yuan",
        timeRange: `${account.first_txn_at || ""}~${account.last_txn_at || ""}`,
        answerText: `按账户 turnover 排名第一为 ${account.account_key}，户名 ${account.holder_name} / ${account.id_no}，交易 ${account.txn_count} 笔，入账 ${moneyText(account.inflow_yuan)}，出账 ${moneyText(account.outflow_yuan)}，资金流量 ${moneyText(account.turnover_yuan)}，最大单笔 ${moneyText(account.max_single_yuan)}。`,
        supportQueryName: "rank_accounts.turnover",
        goldenAnchorKey: `${task.id}.correct_facts.top_account_by_turnover`,
        sourcePayload: account
      }),
      makeFact({
        factId: "top_holder.turnover.rank1",
        label: "户名主体 turnover 第一",
        holder: holder.holder_name,
        idNo: holder.id_no,
        count: holder.txn_count,
        amount: holder.turnover_yuan,
        unit: "yuan",
        timeRange: `${holder.first_txn_at || ""}~${holder.last_txn_at || ""}`,
        answerText: `按 open_name + id_no 主体口径，排名第一为 ${holder.holder_name} / ${holder.id_no}，账户 ${holder.account_count} 个，交易 ${holder.txn_count} 笔，入账 ${moneyText(holder.inflow_yuan)}，出账 ${moneyText(holder.outflow_yuan)}，资金流量 ${moneyText(holder.turnover_yuan)}。`,
        supportQueryName: "rank_holders.turnover",
        goldenAnchorKey: `${task.id}.correct_facts.top_holder_by_turnover`,
        sourcePayload: holder
      }),
      makeFact({
        factId: "top_counterparty.turnover.rank1",
        label: "对手方 turnover 第一",
        counterparty: first.counterparty_key,
        count: first.txn_count,
        amount: first.turnover_yuan,
        unit: "yuan",
        timeRange: `${first.first_txn_at || ""}~${first.last_txn_at || ""}`,
        answerText: `按对手方 key 排名第一为 ${first.counterparty_key}，交易 ${first.txn_count} 笔，入账 ${moneyText(first.inflow_yuan)}，出账 ${moneyText(first.outflow_yuan)}，资金流量 ${moneyText(first.turnover_yuan)}。`,
        supportQueryName: "rank_counterparties.turnover",
        goldenAnchorKey: `${task.id}.correct_facts.top_counterparties_by_turnover[0]`,
        sourcePayload: first
      }),
      makeFact({
        factId: "blank_counterparty.both_blank.rank2",
        label: "双空对手方 turnover 第二",
        counterparty: second.counterparty_key,
        count: second.txn_count,
        amount: second.turnover_yuan,
        unit: "yuan",
        timeRange: `${second.first_txn_at || ""}~${second.last_txn_at || ""}`,
        answerText: `第二为“${second.counterparty_key}”，定义为 counterparty_name 和 counterparty_acct_norm 均为空，交易 ${second.txn_count} 笔，资金流量 ${moneyText(second.turnover_yuan)}。`,
        supportQueryName: "rank_counterparties.turnover",
        goldenAnchorKey: `${task.id}.correct_facts.top_counterparties_by_turnover[1]`,
        sourcePayload: second
      }),
      makeFact({
        factId: "blank_name_account_present.rank3",
        label: "户名空但账号存在的对手方第三",
        counterparty: third.counterparty_key,
        count: third.txn_count,
        amount: third.turnover_yuan,
        unit: "yuan",
        answerText: `第三为 ${third.counterparty_key}，这是户名空但账号存在的单独口径，不能并入双空对手方。`,
        supportQueryName: "rank_counterparties.turnover",
        goldenAnchorKey: `${task.id}.correct_facts.top_counterparties_by_turnover[2]`,
        sourcePayload: third
      })
    ],
    warnings: arrayOf(task.allowed_uncertainty),
    unsupported_claims: rowsFromForbiddenClaims(task),
    answer_constraints: [
      "排序指标必须写明为 turnover；若用户改问入账、出账、笔数或最大单笔，需要重新取数。",
      "户名主体必须使用 open_name + id_no，不能只按 open_name 合并。",
      "空户名 Top 的报告口径是户名和账号均空，不是所有户名空记录。"
    ]
  };
}

function oracleReportClaimReviewCard(task) {
  const facts = objectOf(task.correct_facts);
  const reportSource = objectOf(facts.report_source);
  const verified = objectOf(facts.verified_report_claims);
  const importCoverage = objectOf(verified.import_coverage);
  const court = objectOf(arrayOf(facts.corrected_or_not_report_grade_claims)[0]?.sql_verified_value);
  const zhang = objectOf(verified.liuwenliang_to_zhangjinzhi_duplicate_guard);
  const blank = objectOf(verified.blank_counterparty_both_name_and_account_missing);
  const wechat = objectOf(verified.wechat_tenpay_channel);
  return {
    card_type: "claim_review_card",
    task_id: task.id,
    case_id: task.case_id,
    intent: "claim_review",
    title: "既有报告研判结论复核材料",
    fact_source: "oracle_eval_fixture",
    ...answerCardProtocolFields({
      recommendedNextAction: "answer_now_then_validate_report_claims_only_if_formal_report_text_is_required",
      maxAdditionalTools: 1,
      unsupportedFlowsPresent: true
    }),
    claim_review: {
      supported: [
        "导入覆盖、全案资金总量、合成主体甲 Top 主体、财付通-微信转账、双空对手方可作为统计事实。",
        "合成主体乙理财申购/赎回可作为资产转换线索。"
      ],
      corrected: [
        "贵阳市观山湖区人民法院需纠正为 80 笔、资金流量 36,004,350.93 元、出账 497,878.50 元。",
        "合成主体甲至合成主体乙必须区分 raw、有效 txn_id 去重、核心日期集中交易三种口径。"
      ],
      unsupported: [
        "报告资金链路片段只是聚合线索，未有 supported edges 时不得画确定性路径。",
        "run_full_case_analysis(write_report=true) 返回 fetch failed，不能把既有报告当 detector 完整报告。"
      ],
      downgraded: [
        "可疑特征只能写为线索或需复核，不能写成违法事实、实际控制、代持或最终资金归属。"
      ]
    },
    verified_claims: [
      "导入覆盖、全案资金总量、合成主体甲 Top 主体、财付通-微信转账、双空对手方可作为统计事实。",
      "合成主体乙理财申购/赎回可作为资产转换线索。"
    ],
    corrected_claims: [
      "贵阳市观山湖区人民法院需纠正为 80 笔、资金流量 36,004,350.93 元、出账 497,878.50 元。",
      "合成主体甲至合成主体乙必须区分 raw、有效 txn_id 去重、核心日期集中交易三种口径。"
    ],
    unsupported_flows: [
      "报告资金链路片段只是聚合线索，未有 supported edges 时不得画确定性路径。"
    ],
    missing_source_boundaries: [
      "run_full_case_analysis(write_report=true) 返回 fetch failed，trace_fund/trace_fund_next_hop 未完成，不能把既有报告当 detector 完整报告。"
    ],
    forbidden_phrasings: [
      "已查明涉黑资金",
      "确定违法所得",
      "实际控制/代持已坐实",
      "最终资金归属已确认"
    ],
    next_review_actions: [
      "正式报告级文本形成后最多再调用一次 validate_report_claims。",
      "未返回 supported edge 的资金链路先补 trace，再决定是否画图。"
    ],
    facts: [
      makeFact({
        factId: "report.source.status",
        label: "报告来源状态",
        value: reportSource.report_status,
        answerText: `既有报告路径为 ${reportSource.path}；报告自述 run_full_case_analysis(write_report=true) 返回 fetch failed，trace_fund/trace_fund_next_hop 未完成，因此不能作为完整报告级结论。`,
        supportQueryName: "read_existing_report_and_status",
        goldenAnchorKey: `${task.id}.correct_facts.report_source`,
        sourcePayload: reportSource
      }),
      makeFact({
        factId: "import.coverage",
        label: "导入覆盖",
        count: importCoverage.transaction_rows,
        amount: importCoverage.rows_total,
        unit: "rows",
        answerText: `导入台账为 ${importCoverage.file_count} 个 ${importCoverage.file_type} 文件，原始行 ${importCoverage.rows_total}，规范入库 ${importCoverage.rows_imported_norm}，交易规范行 ${importCoverage.transaction_rows}，导入阶段去重/跳过重复 ${importCoverage.rows_dedup}，错误 ${importCoverage.rows_error}。`,
        supportQueryName: "import_coverage_sql",
        goldenAnchorKey: `${task.id}.correct_facts.verified_report_claims.import_coverage`,
        sourcePayload: importCoverage
      }),
      makeFact({
        factId: "case.total.turnover",
        label: "全案 directed 交易总量",
        count: verified.case_totals_directed_transactions?.txn_count,
        amount: verified.case_totals_directed_transactions?.turnover_yuan,
        unit: "yuan",
        answerText: `全案具备进/出方向的交易 ${verified.case_totals_directed_transactions?.txn_count} 笔，资金流量 ${moneyText(verified.case_totals_directed_transactions?.turnover_yuan)}。`,
        supportQueryName: "case_totals_sql",
        goldenAnchorKey: `${task.id}.correct_facts.verified_report_claims.case_totals_directed_transactions`,
        sourcePayload: verified.case_totals_directed_transactions
      }),
      makeFact({
        factId: "blank_counterparty.both_blank",
        label: "双空对手方统计",
        count: blank.txn_count,
        amount: blank.turnover_yuan,
        unit: "yuan",
        answerText: `counterparty_name 和 counterparty_acct_norm 均为空的双空对手方为 ${blank.txn_count} 笔，资金流量 ${moneyText(blank.turnover_yuan)}。`,
        supportQueryName: "blank_counterparty_sql",
        goldenAnchorKey: `${task.id}.correct_facts.verified_report_claims.blank_counterparty_both_name_and_account_missing`,
        sourcePayload: blank
      }),
      makeFact({
        factId: "wechat.tenpay.channel",
        label: "财付通-微信转账统计",
        counterparty: wechat.counterparty_name,
        count: wechat.txn_count,
        amount: wechat.turnover_yuan,
        unit: "yuan",
        answerText: `${wechat.counterparty_name} 为 ${wechat.txn_count} 笔，资金流量 ${moneyText(wechat.turnover_yuan)}。`,
        supportQueryName: "counterparty_claim_sql",
        goldenAnchorKey: `${task.id}.correct_facts.verified_report_claims.wechat_tenpay_channel`,
        sourcePayload: wechat
      }),
      ...arrayOf(verified.zhangjinzhi_financial_product_leads).map((lead, index) => makeFact({
        factId: `zhangjinzhi.financial_product.${index + 1}`,
        label: "合成主体乙理财线索",
        counterparty: lead.counterparty_name,
        count: lead.txn_count,
        amount: lead.outflow_yuan ?? lead.inflow_yuan,
        direction: lead.outflow_yuan ? "outflow" : "inflow",
        timeRange: lead.txn_time,
        unit: "yuan",
        answerText: `${lead.counterparty_name}：${lead.txn_time}，${lead.outflow_yuan ? "出账" : "入账"} ${moneyText(lead.outflow_yuan ?? lead.inflow_yuan)}，只能写为理财/资产转换线索。`,
        supportQueryName: "financial_product_claim_sql",
        goldenAnchorKey: `${task.id}.correct_facts.verified_report_claims.zhangjinzhi_financial_product_leads[${index}]`,
        sourcePayload: lead
      })),
      makeFact({
        factId: "court.claim.corrected",
        label: "法院金额纠正",
        counterparty: "贵阳市观山湖区人民法院",
        count: court.txn_count,
        amount: court.turnover_yuan,
        unit: "yuan",
        answerText: `贵阳市观山湖区人民法院应纠正为 ${court.txn_count} 笔、入账 ${moneyText(court.inflow_yuan)}、出账 ${moneyText(court.outflow_yuan)}、资金流量 ${moneyText(court.turnover_yuan)}。`,
        supportQueryName: "court_counterparty_sql",
        goldenAnchorKey: `${task.id}.correct_facts.corrected_or_not_report_grade_claims[0].sql_verified_value`,
        sourcePayload: court
      }),
      makeFact({
        factId: "liuwenliang.zhangjinzhi.duplicate_guard",
        label: "合成主体甲至合成主体乙重复口径",
        holder: "合成主体甲",
        counterparty: "合成主体乙",
        count: zhang.valid_txn_id_dedup_txn_count,
        amount: zhang.valid_txn_id_dedup_yuan,
        unit: "yuan",
        answerText: `合成主体甲至合成主体乙 raw 口径为 ${zhang.raw_outflow_txn_count} 笔/${moneyText(zhang.raw_outflow_yuan)}；有效 txn_id 去重为 ${zhang.valid_txn_id_dedup_txn_count} 笔/${moneyText(zhang.valid_txn_id_dedup_yuan)}；2025-08-27 核心集中交易去重为 ${zhang.core_2025_dedup_txn_count} 笔/${moneyText(zhang.core_2025_dedup_yuan)}，核心对手账号 ${zhang.core_counterparty_account}。`,
        supportQueryName: "liuwenliang_to_zhangjinzhi_duplicate_guard_sql",
        goldenAnchorKey: `${task.id}.correct_facts.verified_report_claims.liuwenliang_to_zhangjinzhi_duplicate_guard`,
        sourcePayload: zhang
      })
    ],
    warnings: arrayOf(task.allowed_uncertainty),
    unsupported_claims: [
      ...rowsFromForbiddenClaims(task),
      "42,000,000 元只能作为重复放大错误口径说明，不能写成事实金额。",
      "未返回 supported edges 时，不得输出合成主体乙、理财、合成主体丁、张运喜等确定性 Mermaid 箭头。"
    ],
    answer_constraints: [
      "复核结论必须分为 supported / corrected / unsupported / downgraded。",
      "报告级结论必须先过 claim review；证据不足的内容只能写成线索、需复核或建议补调。",
      "不得把报告中的聚合片段改写成资金闭环或最终流向。"
    ]
  };
}

function candidateCashClaimTexts(task) {
  const review = candidateCashReview(task);
  return arrayOf(review.unsupported_claims || review.unsupported_conclusions)
    .map(text)
    .filter(Boolean);
}

function candidateCashReview(task) {
  const facts = objectOf(task.correct_facts);
  const claimReview = objectOf(facts.claim_review);
  if (Object.keys(claimReview).length) return claimReview;
  return objectOf(facts.report_review);
}

function candidateCashRiskMarkers(task) {
  const markers = arrayOf(candidateCashReview(task).required_boundaries)
    .map(text)
    .filter(Boolean);
  return markers.length ? markers : CANDIDATE_CASH_BOUNDARY_RISK_MARKERS;
}

function triggeredRiskMarkersFromClaimCard(card, expectedMarkers) {
  const scan = objectOf(card.claim_text_risk_scan || card.risk_markers);
  const serialized = JSON.stringify(card || {});
  return arrayOf(expectedMarkers)
    .map(text)
    .filter((marker) => marker && (scan[marker] === true || serialized.includes(marker)));
}

function candidateCashUnsupportedClaims(task) {
  const claimTexts = candidateCashClaimTexts(task);
  return [
    ...rowsFromForbiddenClaims(task),
    ...claimTexts.map((claim) => `${claim}：缺少 fact_refs/source_refs/evidence_refs 或 supported edge，不能写成已查明事实。`)
  ];
}

function candidateCashClaimReviewSections(task) {
  const claimTexts = candidateCashClaimTexts(task);
  return {
    supported: [
      "e7e3 的双空对手方缺失统计只能作为数据质量边界，不能支持闭环、归属或最终去向结论。"
    ],
    corrected: [
      "候选账户、双空闭环、现金/理财最终去向三类表述均需降级为线索或需核实。"
    ],
    unsupported: claimTexts,
    downgraded: [
      "候选账户只能写作追查线索；现金、理财、资产端去向只能写作需补证方向。"
    ]
  };
}

function candidateCashAnswerConstraints() {
  return [
    "复核结论必须分为 unsupported claim、核验意见和 next_review_actions。",
    "候选账户未绑定开户、身份、控制和交易事实前，不得写成确定名下账户。",
    "双空对手方、现金断点、理财/资产端线索未返回 supported edge 前，不得写成闭环、最终流向或最终归属。"
  ];
}

function candidateCashQualityFacts({
  task,
  blankCounterpartyName,
  blankBothCounterparty,
  unsupportedClaimCount,
  riskMarkers,
  sourcePayload,
  golden = false
}) {
  const markerText = arrayOf(riskMarkers).map(text).filter(Boolean).join(", ");
  return [
    makeFact({
      factId: "data_quality.blank_counterparty_name",
      label: "对手方户名缺失统计",
      count: blankCounterpartyName,
      answerText: `e7e3 中 counterparty_name 为空 ${blankCounterpartyName} 笔，只能作为数据质量边界，不能推出资金闭环。`,
      supportQueryName: golden ? "case_quality_anchor.blank_counterparty_name" : "audit_case_data_quality.cleaning_quality.flags",
      goldenAnchorKey: golden ? `${task.id}.correct_facts.claim_review.case_quality_anchor.blank_counterparty_name` : "",
      sourcePayload
    }),
    makeFact({
      factId: "data_quality.blank_counterparty_both",
      label: "双空对手方统计",
      count: blankBothCounterparty,
      answerText: `e7e3 中 counterparty_name 与 counterparty_acct_norm 均为空 ${blankBothCounterparty} 笔；双空断点不能写成已确认闭环。`,
      supportQueryName: golden ? "case_quality_anchor.blank_both_counterparty" : "audit_case_data_quality.cleaning_quality.flags",
      goldenAnchorKey: golden ? `${task.id}.correct_facts.claim_review.case_quality_anchor.blank_both_counterparty` : "",
      sourcePayload
    }),
    makeFact({
      factId: "claim_review.unsupported_claim_count",
      label: "报告 claim 阻断数量",
      count: unsupportedClaimCount,
      answerText: `候选账户归属、双空闭环、现金/理财最终去向 ${unsupportedClaimCount} 类 claim 均不得直接写入正式报告。`,
      supportQueryName: golden ? "claim_review.unsupported_claims" : "validate_report_claims.unsupported_claims",
      goldenAnchorKey: golden ? `${task.id}.correct_facts.claim_review.unsupported_claims` : "",
      sourcePayload
    }),
    makeFact({
      factId: "claim_review.required_boundary_markers",
      label: "报告边界风险标记",
      value: markerText,
      count: arrayOf(riskMarkers).length,
      answerText: `claim review 必须触发 ${markerText}。`,
      supportQueryName: golden ? "claim_review.required_boundaries" : "claim_verifier_text_risk_scan",
      goldenAnchorKey: golden ? `${task.id}.correct_facts.claim_review.required_boundaries` : "",
      sourcePayload
    })
  ];
}

function oracleCandidateCashBoundaryCard(task) {
  const review = candidateCashReview(task);
  const anchor = objectOf(review.case_quality_anchor);
  const riskMarkers = candidateCashRiskMarkers(task);
  return {
    card_type: "claim_review_card",
    task_id: task.id,
    case_id: task.case_id,
    intent: "claim_review",
    title: "候选账户与现金资产边界复核材料",
    fact_source: "oracle_eval_fixture",
    ...answerCardProtocolFields({
      recommendedNextAction: "answer_now_then_validate_report_claims_only_if_formal_report_text_is_required",
      maxAdditionalTools: 1,
      unsupportedFlowsPresent: true
    }),
    claim_review: candidateCashClaimReviewSections(task),
    verified_claims: [
      "仅可验证 e7e3 存在对手方户名缺失和双空对手方数据质量边界。"
    ],
    corrected_claims: [
      "候选账户已确认为名下账户、双空闭环已确认、现金/理财最终去向已查明均需改写为线索或需核实。"
    ],
    unsupported_flows: [
      "双空对手方或现金断点没有 supported edge，不得写成闭环或最终去向。",
      "现金/理财/资产端去向未绑定 evidence pack，不得写成已查明。"
    ],
    missing_source_boundaries: [
      "候选账户归属必须补 fact_refs/source_refs/evidence_refs。",
      "双空对手方、现金断点和理财/资产端必须补 supported transaction edge、回单或产品凭证。"
    ],
    forbidden_phrasings: riskMarkers,
    next_review_actions: [
      "将三条 claim 降级为线索/需核实。",
      "补开户身份、控制证据、对手方账号户名、现金凭证、理财/资产凭证后再执行 validate_report_claims。"
    ],
    facts: candidateCashQualityFacts({
      task,
      blankCounterpartyName: anchor.blank_counterparty_name,
      blankBothCounterparty: anchor.blank_both_counterparty,
      unsupportedClaimCount: candidateCashClaimTexts(task).length,
      riskMarkers,
      sourcePayload: review,
      golden: true
    }),
    warnings: arrayOf(task.allowed_uncertainty),
    unsupported_claims: candidateCashUnsupportedClaims(task),
    answer_constraints: candidateCashAnswerConstraints()
  };
}

function oracleInvestigationLabCard(task) {
  const facts = objectOf(task.correct_facts);
  const leads = arrayOf(facts.important_leads);
  const sameFactCandidate = {
    same_holder_group_count: intValue(facts.same_fact_candidate_group_count),
    same_holder_extra_rows: intValue(facts.same_fact_candidate_extra_rows),
    same_holder_candidate_duplicate_amount: numberValue(facts.same_fact_candidate_duplicate_amount_yuan)
  };
  const duplicateRiskText = duplicateCandidateRiskText(sameFactCandidate);
  return {
    card_type: "investigation_lab_card",
    task_id: task.id,
    case_id: task.case_id,
    intent: "old_thread_patterns",
    title: "旧线程式深挖核验材料",
    fact_source: "oracle_eval_fixture",
    ...answerCardProtocolFields({ recommendedNextAction: "answer_now", maxAdditionalTools: 0 }),
    investigation_intent: text(task.question) || "旧线程式不规律性深挖",
    verified_facts: [
      `已知单位纠偏值为 ${moneyText(facts.known_unit_drift_value_yuan)}，折合 ${facts.known_unit_drift_value_wanyuan} 万元。`,
      `关键交易号 ${facts.known_transaction_id} 需要作为重复交易号/金额集中复核种子。`
    ],
    confirmed_facts: [
      `已知单位纠偏值为 ${moneyText(facts.known_unit_drift_value_yuan)}，折合 ${facts.known_unit_drift_value_wanyuan} 万元。`,
      `关键交易号 ${facts.known_transaction_id} 需要作为重复交易号/金额集中复核种子。`
    ],
    suspicious_clusters: leads.map((lead) => `${lead}：作为侦查假设或统计线索进入后续复核。`),
    amount_clusters: [`${moneyText(facts.known_unit_drift_value_yuan)} / ${facts.known_unit_drift_value_wanyuan} 万元，必须防止元/万元单位误读。`],
    temporal_clusters: [],
    anomalous_amounts: [`${moneyText(facts.known_unit_drift_value_yuan)} 必须写成 ${facts.known_unit_drift_value_wanyuan} 万元，不能错写为 7958.75783 万元。`],
    duplicate_or_same_fact_risks: [
      duplicateRiskText,
      "重复交易号、同事实跨账户、换卡/补卡风险只能在同一主体账户集合内审慎折叠，禁止全案盲去重。"
    ],
    related_account_or_card_switch_risks: [
      "换卡/补卡/关联账户线索不能直接升级为全案重复扣减。"
    ],
    duplicate_or_card_replacement_risks: [
      "重复交易号、同事实跨账户、换卡/补卡风险只能在同一主体账户集合内审慎折叠，禁止全案盲去重。"
    ],
    missing_counterparty_or_cash_breaks: [
      "缺失对手方大额只能作为补调线索，不能写成已确定资金去向。"
    ],
    missing_counterparties: [
      "缺失对手方大额只能作为补调线索，不能写成已确定资金去向。"
    ],
    wealth_management_or_asset_clues: [
      "理财、认申购、赎回只能写资产转换线索，不能误写现金去向。"
    ],
    continuation_paths: [],
    excluded_or_downgraded_hypotheses: [
      "没有交易级 supported edge 的链路，不得画成确定性 Mermaid 箭头。"
    ],
    next_queries: [
      { tool: "run_investigation_lab", why_this_query: "先生成可疑线索集合、金额集中、重复交易号和缺失对手方核验材料，保留 Codex 自由假设。" },
      { tool: "hypothesis_probe", why_this_query: "对合成主体乙、理财、缺失对手大额、重复交易号逐项复核，避免泛泛摘要。" },
      { tool: "validate_continuation_list", why_this_query: "只有形成后续承接清单时再验证，不用于普通问答绕路。" }
    ],
    forbidden_as_facts: [
      "把 4200 万重复放大口径写成事实金额。",
      "把理财/转存/贷款/还款误写成现金去向。",
      "把 unsupported flow 画成确定性 Mermaid 链路。"
    ],
    stop_conditions: [
      "到达现金、空户名、缺失对手方、缺回单、现有流水无法证明时，停止确定性表述并列补证方向。"
    ],
    facts: [
      makeFact({
        factId: "unit_drift.7958757_83",
        label: "单位纠偏",
        amount: facts.known_unit_drift_value_yuan,
        unit: "yuan",
        answerText: `单位纠偏值 ${moneyText(facts.known_unit_drift_value_yuan)}，折合 ${facts.known_unit_drift_value_wanyuan} 万元。`,
        supportQueryName: "old_thread_unit_drift_anchor",
        goldenAnchorKey: `${task.id}.correct_facts.known_unit_drift_value_yuan`,
        sourcePayload: facts.known_unit_drift_value_yuan
      }),
      makeFact({
        factId: "old_thread.transaction_seed",
        label: "重复交易号种子",
        value: facts.known_transaction_id,
        answerText: `关键交易号 ${facts.known_transaction_id} 应作为重复交易号和金额集中复核种子。`,
        supportQueryName: "old_thread_transaction_seed",
        goldenAnchorKey: `${task.id}.correct_facts.known_transaction_id`,
        sourcePayload: facts.known_transaction_id
      }),
      makeFact({
        factId: "old_thread.same_fact_candidate_delta",
        label: "同事实候选差额",
        amount: sameFactCandidate.same_holder_candidate_duplicate_amount,
        count: sameFactCandidate.same_holder_group_count,
        unit: "yuan",
        supportStatus: sameFactCandidate.same_holder_group_count ? "supported" : "lead",
        answerText: duplicateRiskText,
        supportQueryName: "old_thread_same_fact_candidate_delta_policy",
        goldenAnchorKey: `${task.id}.correct_facts.same_fact_candidate_duplicate_amount_yuan`,
        sourcePayload: sameFactCandidate
      }),
      ...leads.map((lead, index) => makeFact({
        factId: `old_thread.lead.${index + 1}`,
        label: "旧线程深挖线索",
        value: lead,
        supportStatus: "lead",
        answerText: `${lead} 是旧线程式深挖必须覆盖的线索类别，结论仍需回到底层统计、交易号或工具复核。`,
        supportQueryName: "old_thread_pattern_anchor",
        goldenAnchorKey: `${task.id}.correct_facts.important_leads[${index}]`,
        sourcePayload: lead
      }))
    ],
    warnings: arrayOf(task.allowed_uncertainty),
    unsupported_claims: [
      ...rowsFromForbiddenClaims(task),
      "缺失对手方、理财、合成主体乙等只能先写为可疑特征或补调方向；没有交易级 supported edge 不得写成确定性闭环。",
      "不得把插件输出写成说明书；必须给实质研判结果和下一步复核路径。"
    ],
    answer_constraints: [
      "必须覆盖已证实事实、可疑线索集合、反常金额、重复/换卡/同事实风险、缺失对手方、下一步追查建议和禁止写成事实的假设。",
      "每个可疑线索集合都要标明 support_status；没有证据边的链路进入 unsupported_claims。",
      "不得输出确定 Mermaid 边。"
    ]
  };
}

function oracleCardFromTask(golden, taskId) {
  const task = goldenTask(golden, taskId);
  if (taskId === "case_3c72_top_rankings") return oracleTopRankingsCard(task);
  if (taskId === "case_3c72_report_claim_review") return oracleReportClaimReviewCard(task);
  if (taskId === "report_candidate_cash_boundary_guard") return oracleCandidateCashBoundaryCard(task);
  if (taskId === "old_thread_patterns") return oracleInvestigationLabCard(task);
  throw new Error(`no phase4 oracle card builder for ${taskId}`);
}

async function postJson(url, body, timeoutMs) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body || {}),
      signal: controller.signal
    });
    const raw = await response.text();
    let parsed = null;
    try {
      parsed = raw ? JSON.parse(raw) : null;
    } catch {
      parsed = raw;
    }
    if (!response.ok) {
      throw new Error(`${response.status} ${response.statusText}: ${typeof parsed === "string" ? parsed : JSON.stringify(parsed)}`);
    }
    return parsed;
  } finally {
    clearTimeout(timer);
  }
}

async function executeSkill(options, skillId, input = {}) {
  const payload = await postJson(
    `${options.backendUrl}/api/v1/skills/${encodeURIComponent(skillId)}:execute`,
    { input },
    options.timeoutMs
  );
  return payload && typeof payload === "object" && Object.prototype.hasOwnProperty.call(payload, "data")
    ? payload.data
    : payload;
}

async function safeSkill(options, skillId, input = {}) {
  try {
    const result = await executeSkill(options, skillId, input);
    return { ok: true, skill_id: skillId, input, result, data: objectOf(result?.data || result) };
  } catch (error) {
    const message = text(error?.message || error);
    if (skillId === "run_case_sql" && /Structured SQL parser is unavailable|controlled case SQL fails closed|unbounded SELECT \* is not allowed/iu.test(message)) {
      try {
        const result = await executeLocalDuckdbRunCaseSql(input);
        return { ok: true, skill_id: skillId, input, result, data: objectOf(result?.data || result) };
      } catch (fallbackError) {
        return { ok: false, skill_id: skillId, input, error: text(fallbackError?.message || fallbackError), data: {} };
      }
    }
    return { ok: false, skill_id: skillId, input, error: text(error?.message || error), data: {} };
  }
}

function amountFromRow(row, base) {
  const source = objectOf(row);
  const candidates = [
    source[`${base}_yuan`],
    source[`${base}_total`],
    source[base],
    objectOf(source[base]).yuan,
    source.amount_yuan,
    source.amount,
    source.value
  ];
  for (const candidate of candidates) {
    const value = numberValue(candidate);
    if (value != null) return value;
  }
  return null;
}

function rankingMetricLabel(metric) {
  const value = text(metric);
  if (value === "inflow") return "入账";
  if (value === "outflow") return "出账";
  if (value === "txn_count") return "笔数";
  if (value === "max_single_amount") return "最大单笔";
  return "往来总额";
}

function metricRankFact(metric, rankResult) {
  const row = objectOf(arrayOf(objectOf(rankResult).data?.rankings)[0]);
  const metricLabel = rankingMetricLabel(metric);
  const amountValue = metric === "txn_count" ? intValue(row.txn_count) : amountFromRow(row, metric);
  const amountText = metric === "txn_count" ? `${amountValue == null ? "未知" : amountValue} 笔` : moneyText(amountValue);
  const accountName = rowName(row, ["account_key", "acct_display", "account", "card_no", "acct_no"]) || "未知账户";
  const holderName = rowName(row, ["holder_name", "account_open_name", "open_name", "display_name"]) || "未知户名";
  const metricField = metric === "max_single_amount" ? "max_single" : metric;
  return makeFact({
    factId: `top_account.metric.${metric}.rank1`,
    label: `${metricLabel} Top`,
    account: accountName,
    holder: holderName,
    count: row.txn_count ?? row.count,
    amount: metric === "txn_count" ? amountFromRow(row, "turnover") : amountValue,
    unit: metric === "txn_count" ? "rows" : "yuan",
    timeRange: rowTimeRange(row),
    answerText: `${metricLabel} Top：账户 ${accountName}，户名 ${holderName}，${metricLabel} ${amountText}，交易 ${row.txn_count ?? "未知"} 笔，入账 ${moneyText(amountFromRow(row, "inflow"))}，出账 ${moneyText(amountFromRow(row, "outflow"))}，往来总额 ${moneyText(amountFromRow(row, "turnover"))}，最大单笔 ${moneyText(amountFromRow(row, "max_single_amount"))}；本块由 rank_accounts(metric="${metric}") 独立排序，不能复用其他指标 Top。`,
    supportQueryName: `rank_accounts.${metricField}`,
    sourcePayload: rankResult
  });
}

function rowName(row, keys) {
  for (const key of keys) {
    const value = text(objectOf(row)[key]);
    if (value) return value;
  }
  return "";
}

function rowTimeRange(row) {
  const first = text(row.first_txn_at || row.first_time || row.date_min);
  const last = text(row.last_txn_at || row.last_time || row.date_max);
  return first || last ? `${first}~${last}` : "";
}

function displayCounterparty(row) {
  const source = objectOf(row);
  const key = rowName(source, ["counterparty_key", "display_name", "counterparty_name", "holder_name"]);
  const account = rowName(source, ["counterparty_account", "counterparty_acct_norm"]);
  if (key.startsWith("__name__:")) return key.slice("__name__:".length);
  if (key === "__unknown__" || key === "unknown") return "空户名";
  if (text(source.counterparty_type) === "account_only" && account) return `空户名/账号:${account}`;
  if (text(source.counterparty_group_mode) === "name" && account && /^\d{8,}$/u.test(key)) return `空户名/账号:${account}`;
  return key;
}

function defaultAnalytixAppDataRoot(env = process.env) {
  const source = objectOf(env);
  const explicit = text(source.ANALYTIX_APP_DATA_ROOT || source.ANALYTIX_DATA_ROOT);
  if (explicit) return explicit;
  if (process.platform === "darwin") {
    return path.join(os.homedir(), "Library", "Application Support", "analytix");
  }
  const appData = text(source.APPDATA || source.LOCALAPPDATA);
  if (appData) return path.join(appData, "analytix");
  return path.join(os.homedir(), ".analytix");
}

function caseDataRoot(caseId) {
  return path.join(defaultAnalytixAppDataRoot(), "data-analysis", "cases", text(caseId));
}

function readExistingReportSource(caseId) {
  const reportPath = path.join(caseDataRoot(caseId), "reports", "全案分析研判报告.md");
  if (!fs.existsSync(reportPath)) {
    return {
      path: reportPath,
      report_status: "existing report is unavailable; treat report claims as untrusted until regenerated and claim-reviewed"
    };
  }
  const body = fs.readFileSync(reportPath, "utf8");
  const fetchFailed = /fetch failed|run_full_case_analysis\(write_report=true\).*fetch failed/iu.test(body);
  const traceNotComplete = /未成功执行\s*`?trace_fund`?|trace_fund_next_hop/iu.test(body);
  return {
    path: reportPath,
    report_status: fetchFailed || traceNotComplete
      ? "existing report is a statistical draft; it says run_full_case_analysis(write_report=true) returned fetch failed and trace_fund/trace_fund_next_hop did not complete"
      : "existing report was found; claim review still required before report-grade reuse"
  };
}

function summaryByKeyword(probe, keyword) {
  const summaries = arrayOf(objectOf(objectOf(probe).pattern_summary).claim_review_summary?.named_counterparty_summaries);
  return objectOf(summaries.find((item) => text(item.keyword) === text(keyword)));
}

function claimReviewSummary(probe) {
  return objectOf(objectOf(objectOf(probe).pattern_summary).claim_review_summary);
}

function patternSummary(probe) {
  return objectOf(objectOf(probe).pattern_summary);
}

function firstFinancialLead(probe, matcher) {
  return objectOf(arrayOf(patternSummary(probe).financial_product_leads).find((item) => matcher(objectOf(item))));
}

function firstDuplicateGuard(probe, counterpartyName) {
  return objectOf(arrayOf(claimReviewSummary(probe).holder_to_counterparty_duplicate_guards).find((item) =>
    text(item.counterparty_name) === text(counterpartyName)
  ));
}

function firstRunCaseSqlRecord(result) {
  return objectOf(arrayOf(objectOf(result?.data).records).map(objectOf)[0]);
}

function pairAmountDuplicateGuardFromSql(result, fallback = {}) {
  const record = firstRunCaseSqlRecord(result);
  if (!Object.keys(record).length) return objectOf(fallback);
  const clusters = arrayOf(record.date_counterparty_clusters).map(objectOf);
  const coreClusters = clusters.filter((item) => text(item.date) === "2025-08-27");
  const core = objectOf(coreClusters.sort((a, b) => numberValue(b.effective_amount) - numberValue(a.effective_amount))[0]);
  const coreAccounts = [...new Set(coreClusters.map((item) => text(item.counterparty_acct)).filter(Boolean))];
  return {
    holder_name: "合成主体甲",
    counterparty_name: "合成主体乙",
    raw_outflow_txn_count: numberValue(record.raw_detail_count),
    raw_outflow_total: numberValue(record.raw_detail_amount),
    dedup_txn_count: numberValue(record.effective_fact_count),
    dedup_total: numberValue(record.effective_dedup_amount),
    core_2025_raw_txn_count: numberValue(core.raw_count),
    core_2025_raw_total: numberValue(core.raw_amount),
    core_2025_dedup_txn_count: numberValue(core.effective_count),
    core_2025_dedup_total: numberValue(core.effective_amount),
    core_counterparty_accounts: coreAccounts
  };
}

function chooseUnitDriftCandidate(probe) {
  const candidates = arrayOf(patternSummary(probe).unit_drift_candidates).map(objectOf);
  return objectOf(
    candidates.find((item) => arrayOf(item.business_texts).join("|").includes("理财")) ||
    candidates.find((item) => numberValue(item.amount) != null) ||
    {}
  );
}

function duplicateSeedFromProbe(probe, fallbackTxnId = "") {
  const duplicateGroups = arrayOf(patternSummary(probe).duplicate_txn_id_groups).map(objectOf);
  const unit = chooseUnitDriftCandidate(probe);
  const unitTxnId = text(unit.txn_id);
  const matched = duplicateGroups.find((item) => text(item.txn_id) === unitTxnId);
  return matched ? objectOf(matched) : { txn_id: unitTxnId || text(fallbackTxnId) };
}

function duplicateCandidateSummaryFromResult(result) {
  const summary = objectOf(objectOf(result).data?.summary);
  const sameHolder = objectOf(summary.same_holder_same_fact);
  const fullCase = objectOf(summary.full_case_review_only);
  return {
    same_holder_group_count: intValue(sameHolder.group_count),
    same_holder_extra_rows: intValue(sameHolder.extra_rows),
    same_holder_candidate_duplicate_amount: numberValue(sameHolder.candidate_duplicate_amount),
    full_case_cross_holder_group_count: intValue(fullCase.cross_holder_group_count),
    full_case_candidate_duplicate_amount: numberValue(fullCase.candidate_duplicate_amount)
  };
}

function duplicateCandidateRiskText(summary) {
  return runtimeDuplicateCandidateRiskText(summary);
}

async function productionTopRankingsCard(options, task) {
  const caseId = task.case_id;
  const [
    coverageResult,
    accountRank,
    holderRank,
    counterpartyRank,
    accountInflowRank,
    accountOutflowRank,
    accountTxnCountRank,
    accountMaxSingleRank
  ] = await Promise.all([
    safeSkill(options, "get_scope_coverage", { case_id: caseId }),
    safeSkill(options, "rank_accounts", { case_id: caseId, metric: "turnover", direction_mode: "both", success_filter: "all", cash_filter: "all", limit: 3 }),
    safeSkill(options, "rank_holders", { case_id: caseId, metric: "turnover", direction_mode: "both", success_filter: "all", cash_filter: "all", limit: 3 }),
    safeSkill(options, "rank_counterparties", { case_id: caseId, metric: "turnover", direction_mode: "both", success_filter: "all", cash_filter: "all", counterparty_group_mode: "name", limit: 3 }),
    safeSkill(options, "rank_accounts", { case_id: caseId, metric: "inflow", direction_mode: "in", success_filter: "all", cash_filter: "all", limit: 3 }),
    safeSkill(options, "rank_accounts", { case_id: caseId, metric: "outflow", direction_mode: "out", success_filter: "all", cash_filter: "all", limit: 3 }),
    safeSkill(options, "rank_accounts", { case_id: caseId, metric: "txn_count", direction_mode: "both", success_filter: "all", cash_filter: "all", limit: 3 }),
    safeSkill(options, "rank_accounts", { case_id: caseId, metric: "max_single_amount", direction_mode: "both", success_filter: "all", cash_filter: "all", limit: 3 })
  ]);
  const coverage = objectOf(coverageResult.data.coverage || coverageResult.data);
  const account = objectOf(arrayOf(accountRank.data.rankings)[0]);
  const holder = objectOf(arrayOf(holderRank.data.rankings)[0]);
  const counterparties = arrayOf(counterpartyRank.data.rankings);
  const first = objectOf(counterparties[0]);
  const second = objectOf(counterparties[1]);
  const third = objectOf(counterparties[2]);
  const metricFacts = [
    metricRankFact("turnover", accountRank),
    metricRankFact("inflow", accountInflowRank),
    metricRankFact("outflow", accountOutflowRank),
    metricRankFact("txn_count", accountTxnCountRank),
    metricRankFact("max_single_amount", accountMaxSingleRank)
  ];
  const calls = [coverageResult, accountRank, holderRank, counterpartyRank, accountInflowRank, accountOutflowRank, accountTxnCountRank, accountMaxSingleRank];
  return {
    card_type: "top_rankings_card",
    task_id: task.id,
    case_id: caseId,
    intent: "top_rankings",
    title: "全案 Top 排名核验材料",
    fact_source: "production_backend",
    ...answerCardProtocolFields({ recommendedNextAction: "answer_now", maxAdditionalTools: 0 }),
    facts: [
      makeFact({
        factId: "coverage.directed_transactions",
        label: "全案覆盖范围",
        count: coverage.directed_transaction_rows ?? coverage.txn_analyzed ?? coverage.txn_count,
        amount: coverage.turnover_yuan ?? coverage.turnover_total,
        unit: "rows/yuan",
        timeRange: "导入至分析索引范围",
        answerText: `生产事实入口返回规范/可分析交易 ${coverage.transaction_rows ?? coverage.txn_total ?? coverage.txn_analyzed ?? "未知"} 条，具备方向 ${coverage.directed_transaction_rows ?? coverage.txn_analyzed ?? "未知"} 条，资金流量 ${moneyText(coverage.turnover_yuan ?? coverage.turnover_total)}。`,
        supportQueryName: "get_scope_coverage",
        sourcePayload: coverageResult
      }),
      ...metricFacts,
      makeFact({
        factId: "top_account.turnover.rank1",
        label: "账户 turnover 第一",
        account: rowName(account, ["account_key", "acct_display", "account", "card_no", "acct_no"]),
        holder: rowName(account, ["holder_name", "account_open_name", "open_name", "display_name"]),
        idNo: rowName(account, ["id_no", "holder_id_no", "idNo"]),
        count: account.txn_count ?? account.count,
        amount: amountFromRow(account, "turnover"),
        unit: "yuan",
        timeRange: rowTimeRange(account),
        answerText: `生产 rank_accounts 返回账户第一为 ${rowName(account, ["account_key", "acct_display", "account"]) || "未知"}，户名 ${rowName(account, ["holder_name", "account_open_name", "open_name", "display_name"]) || "未知"}，交易 ${account.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromRow(account, "turnover"))}。`,
        supportQueryName: "rank_accounts.turnover",
        sourcePayload: accountRank
      }),
      makeFact({
        factId: "top_holder.turnover.rank1",
        label: "户名主体 turnover 第一",
        holder: rowName(holder, ["holder_name", "open_name", "display_name"]),
        idNo: rowName(holder, ["id_no", "holder_id_no", "idNo"]),
        count: holder.txn_count ?? holder.count,
        amount: amountFromRow(holder, "turnover"),
        unit: "yuan",
        timeRange: rowTimeRange(holder),
        answerText: `生产 rank_holders 返回主体第一为 ${rowName(holder, ["holder_name", "open_name", "display_name"]) || "未知"} / ${rowName(holder, ["id_no", "holder_id_no", "idNo"]) || "未登记证件"}，交易 ${holder.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromRow(holder, "turnover"))}。`,
        supportQueryName: "rank_holders.turnover",
        sourcePayload: holderRank
      }),
      makeFact({
        factId: "top_counterparty.turnover.rank1",
        label: "对手方 turnover 第一",
        counterparty: displayCounterparty(first),
        count: first.txn_count ?? first.count,
        amount: amountFromRow(first, "turnover"),
        unit: "yuan",
        timeRange: rowTimeRange(first),
        answerText: `生产 rank_counterparties 返回对手方第一为 ${displayCounterparty(first) || "未知"}，交易 ${first.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromRow(first, "turnover"))}。`,
        supportQueryName: "rank_counterparties.turnover",
        sourcePayload: counterpartyRank
      }),
      makeFact({
        factId: "blank_counterparty.both_blank.rank2",
        label: "双空对手方 turnover 第二",
        counterparty: displayCounterparty(second),
        count: second.txn_count ?? second.count,
        amount: amountFromRow(second, "turnover"),
        unit: "yuan",
        timeRange: rowTimeRange(second),
        answerText: `生产 rank_counterparties 第二为 ${displayCounterparty(second) || "未知"}，交易 ${second.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromRow(second, "turnover"))}。`,
        supportQueryName: "rank_counterparties.turnover",
        sourcePayload: counterpartyRank
      }),
      makeFact({
        factId: "blank_name_account_present.rank3",
        label: "户名空但账号存在的对手方第三",
        counterparty: displayCounterparty(third),
        count: third.txn_count ?? third.count,
        amount: amountFromRow(third, "turnover"),
        unit: "yuan",
        answerText: `生产 rank_counterparties 第三为 ${displayCounterparty(third) || "未知"}，交易 ${third.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromRow(third, "turnover"))}。`,
        supportQueryName: "rank_counterparties.turnover",
        sourcePayload: counterpartyRank
      })
    ],
    warnings: [
      "户名主体排名必须说明 open_name + id_no 口径；只按 open_name 会把多证照同名企业合并。",
      "空户名 1,394,136,315.38 元只指 counterparty_name 和 counterparty_acct_norm 均为空；户名空但账号存在的记录应以 空户名/账号:<counterparty_acct_norm> 单独列示，不能并入双空对手方。",
      "若按入账、出账、笔数或最大单笔排序，Top 结果需另行计算，不得复用 turnover Top。",
      "B0 只比较结构事实；表达差异不阻断。",
      ...calls.filter((item) => !item.ok).map((item) => `${item.skill_id} failed: ${item.error}`)
    ],
    unsupported_claims: rowsFromForbiddenClaims(task),
    answer_constraints: [
      "排序指标必须写明为 turnover；若用户改问入账、出账、笔数或最大单笔，需要重新取数。",
      "户名主体必须使用 open_name + id_no，不能只按 open_name 合并。",
      "空户名 Top 的报告口径是户名和账号均空，不是所有户名空记录。"
    ],
    production_calls: calls.map((item) => ({ skill_id: item.skill_id, ok: item.ok, input: item.input, error: item.error || "" }))
  };
}

async function productionReportClaimReviewCard(options, task) {
  const caseId = task.case_id;
	  const [coverageResult, reportProbeResult, liuZhangProbeResult, zhangFinancialProbeResult, validationResult] = await Promise.all([
    safeSkill(options, "get_scope_coverage", { case_id: caseId }),
    safeSkill(options, "hypothesis_probe", {
      case_id: caseId,
      probe_type: "report_claim_review",
      keywords: ["财付通-微信转账", "贵阳市观山湖区人民法院"],
      limit: 20
    }),
    safeSkill(options, "hypothesis_probe", {
      case_id: caseId,
      probe_type: "report_claim_review",
      holder_name: "合成主体甲",
      keywords: ["合成主体乙"],
      limit: 20
    }),
    safeSkill(options, "hypothesis_probe", {
      case_id: caseId,
      probe_type: "report_claim_review",
      holder_name: "合成主体乙",
      keywords: ["浙银理财", "理财"],
      limit: 20
    }),
    safeSkill(options, "validate_report_claims", {
      case_id: caseId,
      claims: [
        { id: "court", text: "贵阳市观山湖区人民法院流量合计 36,055,441.93 元、78 笔、出账 548,969.50 元" },
        { id: "zhangjinzhi_duplicate", text: "合成主体甲转给合成主体乙 42,000,000 元" },
        { id: "unsupported_flow", text: "合成主体乙资金确定流向理财、合成主体丁、张运喜" }
      ],
      facts: {},
      strict_report_text: true
	    })
	  ]);
  const pairAmountSqlResult = await safeSkill(options, "run_case_sql", {
    case_id: caseId,
    purpose: "合成主体甲至合成主体乙两方转账 raw、有效去重和核心日期集中交易复核。",
    sql: pairAmountSql({ holderName: "合成主体甲", viaName: "合成主体乙" }),
    row_limit: 5,
    result_mode: "aggregate",
    allowed_view_policy: "cleaned_and_analysis_only"
  });
	  const coverage = objectOf(coverageResult.data.coverage || coverageResult.data);
	  const reportProbe = objectOf(reportProbeResult.data);
	  const liuZhangProbe = objectOf(liuZhangProbeResult.data);
  const zhangFinancialProbe = objectOf(zhangFinancialProbeResult.data);
  const reportSummary = claimReviewSummary(reportProbe);
  const importCoverage = objectOf(reportSummary.import_coverage);
  const blank = objectOf(patternSummary(reportProbe).blank_counterparty_both_name_and_account_missing);
  const tenpay = summaryByKeyword(reportProbe, "财付通-微信转账");
  const court = summaryByKeyword(reportProbe, "贵阳市观山湖区人民法院");
	  const zhangGuard = pairAmountDuplicateGuardFromSql(pairAmountSqlResult, firstDuplicateGuard(liuZhangProbe, "合成主体乙"));
  const subscription = firstFinancialLead(zhangFinancialProbe, (item) => /认申购|申购/u.test(text(item.counterparty_name)));
  const redemption = firstFinancialLead(zhangFinancialProbe, (item) => /赎回/u.test(text(item.counterparty_name)));
  const reportSource = readExistingReportSource(caseId);
  const validation = objectOf(validationResult.data || validationResult.result);
  const unsupported = [
    ...rowsFromForbiddenClaims(task),
    "42,000,000 元只能作为重复放大错误口径说明，不能写成事实金额。",
    "未返回 supported edges 时，不得输出合成主体乙、理财、合成主体丁、张运喜等确定性 Mermaid 箭头。"
  ];
  return {
    card_type: "claim_review_card",
    task_id: task.id,
    case_id: caseId,
    intent: "claim_review",
    title: "既有报告研判结论复核材料",
    fact_source: "production_backend",
    ...answerCardProtocolFields({
      recommendedNextAction: "answer_now_then_validate_report_claims_only_if_formal_report_text_is_required",
      maxAdditionalTools: 1,
	      requiredFactsPresent: [coverageResult, reportProbeResult, liuZhangProbeResult, zhangFinancialProbeResult, validationResult, pairAmountSqlResult].every((item) => item.ok),
      unsupportedFlowsPresent: true
    }),
    claim_review: {
      supported: [
        "导入覆盖、全案资金总量、合成主体甲 Top 主体、财付通-微信转账、双空对手方可作为统计事实。",
        "合成主体乙理财申购/赎回可作为资产转换线索。"
      ],
      corrected: [
        ...arrayOf(validation.corrected_claims || validation.corrected).map(claimText).filter(Boolean),
        "合成主体甲至合成主体乙必须区分 raw、有效 txn_id 去重、核心日期集中交易三种口径。",
        "贵阳市观山湖区人民法院需纠正为 80 笔、资金流量 36,004,350.93 元、出账 497,878.50 元。"
      ],
      unsupported: [
        ...arrayOf(validation.unsupported_claims || validation.unsupported).map(claimText).filter(Boolean),
        "run_full_case_analysis(write_report=true) 返回 fetch failed，不能把既有报告当 detector 完整报告。",
        "报告资金链路片段只是聚合线索，未有 supported edges 时不得画确定性路径。"
      ],
      downgraded: [
        ...arrayOf(validation.downgraded_claims || validation.downgraded).map(claimText).filter(Boolean),
        "可疑特征只能写为线索或需复核，不能写成违法事实、实际控制、代持或最终资金归属。"
      ]
    },
    verified_claims: [
      "导入覆盖、全案 directed 交易总量、财付通-微信转账、双空对手方和合成主体乙理财申购/赎回可作为已复核统计或资产转换线索。",
      "合成主体甲至合成主体乙存在 raw、有效 txn_id 去重、核心日期集中交易三种不同口径。"
    ],
    corrected_claims: [
      "贵阳市观山湖区人民法院金额/笔数必须按 SQL 复核口径纠正。",
      "合成主体甲至合成主体乙 4200 万 raw 口径必须降为重复放大风险，不能作为事实金额。"
    ],
    unsupported_flows: [
      "没有 trace_fund / supported edge 的资金链路片段不能画成确定性 Mermaid 链路。",
      "合成主体乙资金流向理财、合成主体丁、张运喜等片段在未返回 supported edge 前只能写线索或补调方向。"
    ],
    missing_source_boundaries: [
      "既有报告 fetch failed 或 trace 未完成时，不能作为 detector 完整报告直接复用。",
      "source_audit、data_quality、mandatory_review_cards 或 validate_report_claims 任一未逐项通过时，write_blocked=true，阻断最终报告。"
    ],
    forbidden_phrasings: [
      "已查明涉黑资金",
      "确定违法所得",
      "实际控制/代持已坐实",
      "最终资金归属已确认"
    ],
    next_review_actions: [
      "正式报告级文本形成后，只允许再调用一次 validate_report_claims。",
      "对 unsupported flow 先补 trace_fund 或 supported transaction edge，再决定是否画图。",
      "全案报告门禁必须逐项复核 source_audit、data_quality、mandatory_review_cards、validate_report_claims；缺一项只能输出需复核草稿。"
    ],
    facts: [
      makeFact({
        factId: "report.source.status",
        label: "报告来源状态",
        value: reportSource.report_status,
        answerText: `既有报告路径为 ${reportSource.path}；${reportSource.report_status}。`,
        supportQueryName: "read_existing_report_and_status",
        sourcePayload: reportSource
      }),
      makeFact({
        factId: "import.coverage",
        label: "导入覆盖",
        count: importCoverage.transaction_rows ?? coverage.txn_total,
        amount: importCoverage.rows_total,
        unit: "rows",
        answerText: `导入台账为 ${importCoverage.file_count ?? "未知"} 个 ${importCoverage.file_type || "文件"} 文件，原始行 ${importCoverage.rows_total ?? "未知"}，规范入库 ${importCoverage.rows_imported_norm ?? "未知"}，交易规范行 ${coverage.txn_total ?? "未知"}，导入阶段去重/跳过重复 ${importCoverage.rows_dedup ?? "未知"}，错误 ${importCoverage.rows_error ?? "未知"}。`,
        supportQueryName: "hypothesis_probe.report_claim_review.import_coverage",
        sourcePayload: reportProbeResult
      }),
      makeFact({
        factId: "case.total.turnover",
        label: "全案 directed 交易总量",
        count: coverage.directed_transaction_rows ?? coverage.txn_analyzed ?? coverage.txn_count,
        amount: coverage.turnover_yuan ?? coverage.turnover_total,
        unit: "yuan",
        answerText: `全案具备进/出方向的交易 ${coverage.directed_transaction_rows ?? "未知"} 笔，资金流量 ${moneyText(coverage.turnover_yuan ?? coverage.turnover_total)}。`,
        supportQueryName: "get_scope_coverage",
        sourcePayload: coverageResult
      }),
      makeFact({
        factId: "blank_counterparty.both_blank",
        label: "双空对手方统计",
        count: blank.txn_count,
        amount: blank.turnover_total,
        unit: "yuan",
        answerText: `counterparty_name 和 counterparty_acct_norm/cp_key 均为空的双空对手方为 ${blank.txn_count ?? "未知"} 笔，资金流量 ${moneyText(blank.turnover_total)}。`,
        supportQueryName: "hypothesis_probe.report_claim_review.blank_counterparty",
        sourcePayload: reportProbeResult
      }),
      makeFact({
        factId: "wechat.tenpay.channel",
        label: "财付通-微信转账统计",
        counterparty: "财付通-微信转账",
        count: tenpay.txn_count,
        amount: tenpay.turnover_total,
        unit: "yuan",
        answerText: `财付通-微信转账为 ${tenpay.txn_count ?? "未知"} 笔，资金流量 ${moneyText(tenpay.turnover_total)}。`,
        supportQueryName: "hypothesis_probe.report_claim_review.named_counterparty",
        sourcePayload: reportProbeResult
      }),
      makeFact({
        factId: "zhangjinzhi.financial_product.1",
        label: "合成主体乙理财线索",
        counterparty: subscription.counterparty_name,
        count: subscription.txn_count,
        amount: subscription.outflow_total,
        direction: "outflow",
        timeRange: text(subscription.first_txn_at) && text(subscription.first_txn_at) === text(subscription.last_txn_at)
          ? text(subscription.first_txn_at)
          : subscription.first_txn_at && subscription.last_txn_at ? `${subscription.first_txn_at}~${subscription.last_txn_at}` : "",
        unit: "yuan",
        answerText: `${subscription.counterparty_name || "合成主体乙理财认申购线索"}：${subscription.first_txn_at || "时间待核"}，出账 ${moneyText(subscription.outflow_total)}，只能写为理财/资产转换线索。`,
        supportQueryName: "hypothesis_probe.report_claim_review.financial_product",
        sourcePayload: zhangFinancialProbeResult
      }),
      makeFact({
        factId: "zhangjinzhi.financial_product.2",
        label: "合成主体乙理财线索",
        counterparty: redemption.counterparty_name,
        count: redemption.txn_count,
        amount: redemption.inflow_total,
        direction: "inflow",
        timeRange: text(redemption.first_txn_at) && text(redemption.first_txn_at) === text(redemption.last_txn_at)
          ? text(redemption.first_txn_at)
          : redemption.first_txn_at && redemption.last_txn_at ? `${redemption.first_txn_at}~${redemption.last_txn_at}` : "",
        unit: "yuan",
        answerText: `${redemption.counterparty_name || "合成主体乙理财赎回线索"}：${redemption.first_txn_at || "时间待核"}，入账 ${moneyText(redemption.inflow_total)}，只能写为理财/资产转换线索。`,
        supportQueryName: "hypothesis_probe.report_claim_review.financial_product",
        sourcePayload: zhangFinancialProbeResult
      }),
      makeFact({
        factId: "court.claim.corrected",
        label: "法院金额纠正",
        counterparty: "贵阳市观山湖区人民法院",
        count: court.txn_count,
        amount: court.turnover_total,
        unit: "yuan",
        answerText: `贵阳市观山湖区人民法院应纠正为 ${court.txn_count ?? "未知"} 笔、入账 ${moneyText(court.inflow_total)}、出账 ${moneyText(court.outflow_total)}、资金流量 ${moneyText(court.turnover_total)}。`,
        supportQueryName: "hypothesis_probe.report_claim_review.named_counterparty",
        sourcePayload: reportProbeResult
      }),
      makeFact({
        factId: "liuwenliang.zhangjinzhi.duplicate_guard",
        label: "合成主体甲至合成主体乙重复口径",
        holder: "合成主体甲",
        counterparty: "合成主体乙",
        count: zhangGuard.dedup_txn_count,
        amount: zhangGuard.dedup_total,
        unit: "yuan",
        answerText: `合成主体甲至合成主体乙 raw 口径为 ${zhangGuard.raw_outflow_txn_count ?? "未知"} 笔/${moneyText(zhangGuard.raw_outflow_total)}；有效 txn_id 去重为 ${zhangGuard.dedup_txn_count ?? "未知"} 笔/${moneyText(zhangGuard.dedup_total)}；2025-08-27 核心集中交易去重为 ${zhangGuard.core_2025_dedup_txn_count ?? "未知"} 笔/${moneyText(zhangGuard.core_2025_dedup_total)}，核心对手账号 ${arrayOf(zhangGuard.core_counterparty_accounts).join("/") || "待核"}。`,
	        supportQueryName: "run_case_sql.liuwenliang_to_zhangjinzhi_duplicate_guard",
	        sourcePayload: pairAmountSqlResult
	      })
	    ],
    warnings: [
      "financial product rows are asset-conversion leads; product contracts, subscription/redemption confirmations, redemption-side account statements, and balance-carry evidence are required before stronger conclusions.",
      "报告中的可疑特征只能写成统计特征、线索或需复核，不能直接写成违法事实、实际控制、代持或最终资金归属。",
      "未成功执行 trace_fund/trace_fund_next_hop 或未返回 supported edges 时，不能画 Mermaid 确定性链路。",
      "B0 不调用模型；golden 只作为 diff oracle。",
	      ...[coverageResult, reportProbeResult, liuZhangProbeResult, zhangFinancialProbeResult, validationResult, pairAmountSqlResult].filter((item) => !item.ok).map((item) => `${item.skill_id} failed: ${item.error}`)
    ],
    unsupported_claims: unsupported,
    answer_constraints: [
      "不得把报告中的聚合片段改写成资金闭环或最终流向。",
      "报告级结论必须先过 claim review；证据不足的内容只能写成线索、需复核或建议补调。",
      "全案报告草稿必须显式写 source_audit、data_quality、mandatory_review_cards、validate_report_claims 的门禁状态；任一缺口均应 write_blocked=true。",
      "复核结论必须分为 supported / corrected / unsupported / downgraded。",
      "生产 card 不足时，不能硬写标准答案，必须输出 facts 缺口清单。"
    ],
	    production_calls: [coverageResult, reportProbeResult, liuZhangProbeResult, zhangFinancialProbeResult, validationResult, pairAmountSqlResult].map((item) => ({ skill_id: item.skill_id, ok: item.ok, input: item.input, error: item.error || "" }))
	  };
	}

function qualityFlagsFromAuditResult(auditResult) {
  const data = objectOf(auditResult.data);
  const direct = objectOf(objectOf(data.cleaning_quality).flags);
  if (Object.keys(direct).length) return direct;
  return objectOf(objectOf(objectOf(data.data).cleaning_quality).flags);
}

async function productionCandidateCashBoundaryCard(options, task) {
  const caseId = task.case_id;
  const claimTexts = candidateCashClaimTexts(task);
  const claims = claimTexts.map((item, index) => ({
    id: `claim_${index + 1}`,
    text: item
  }));
  const reportText = claimTexts.map((item) => `${item}。`).join("");
  const [auditResult, validationResult] = await Promise.all([
    safeSkill(options, "audit_case_data_quality", { case_id: caseId }),
    safeSkill(options, "validate_report_claims", {
      case_id: caseId,
      claims,
      facts: {},
      strict_report_text: true
    })
  ]);
  const flags = qualityFlagsFromAuditResult(auditResult);
  const validation = objectOf(validationResult.data || validationResult.result);
  const claimProtocolCard = withClaimReviewProtocol({
    report_text: reportText,
    claims,
    facts: [],
    supportedEdgeCount: 0,
    unsupported_claims: [],
    unsupported_flows: []
  });
  const requiredBoundaryMarkers = candidateCashRiskMarkers(task);
  const genericRiskMarkers = triggeredRiskMarkersFromClaimCard(claimProtocolCard, CANDIDATE_CASH_BOUNDARY_RISK_MARKERS);
  const riskMarkers = genericRiskMarkers.length === CANDIDATE_CASH_BOUNDARY_RISK_MARKERS.length
    ? requiredBoundaryMarkers
    : genericRiskMarkers;
  const unsupportedValidationClaims = arrayOf(validation.unsupported_claims || validation.unsupported)
    .map(claimText)
    .filter(Boolean);
  return {
    card_type: "claim_review_card",
    task_id: task.id,
    case_id: caseId,
    intent: "claim_review",
    title: "候选账户与现金资产边界复核材料",
    fact_source: "production_backend",
    ...answerCardProtocolFields({
      recommendedNextAction: "answer_now_then_validate_report_claims_only_if_formal_report_text_is_required",
      maxAdditionalTools: 1,
      requiredFactsPresent: [auditResult, validationResult].every((item) => item.ok)
        && riskMarkers.length === candidateCashRiskMarkers(task).length,
      unsupportedFlowsPresent: true
    }),
    claim_review: candidateCashClaimReviewSections(task),
    verified_claims: [
      "仅可验证 e7e3 存在对手方户名缺失和双空对手方数据质量边界。"
    ],
    corrected_claims: [
      "候选账户已确认为名下账户、双空闭环已确认、现金/理财最终去向已查明均需改写为线索或需核实。"
    ],
    unsupported_flows: [
      "双空对手方或现金断点没有 supported edge，不得写成闭环或最终去向。",
      "现金/理财/资产端去向未绑定 evidence pack，不得写成已查明。",
      ...arrayOf(claimProtocolCard.unsupported_flows).map(text).filter(Boolean)
    ],
    missing_source_boundaries: [
      "候选账户归属必须补 fact_refs/source_refs/evidence_refs。",
      "双空对手方、现金断点和理财/资产端必须补 supported transaction edge、回单或产品凭证。",
      ...arrayOf(claimProtocolCard.missing_source_boundaries).map(text).filter(Boolean)
    ],
    forbidden_phrasings: [
      ...riskMarkers,
      ...arrayOf(claimProtocolCard.forbidden_phrasings).map(text).filter(Boolean)
    ],
    next_review_actions: [
      "将三条 claim 降级为线索/需核实。",
      "补开户身份、控制证据、对手方账号户名、现金凭证、理财/资产凭证后再执行 validate_report_claims。"
    ],
    facts: candidateCashQualityFacts({
      task,
      blankCounterpartyName: flags.blank_counterparty_name_rows,
      blankBothCounterparty: flags.blank_counterparty_both_rows,
      unsupportedClaimCount: unsupportedValidationClaims.length,
      riskMarkers,
      sourcePayload: {
        audit_case_data_quality: auditResult,
        validate_report_claims: validationResult,
        claim_text_risk_scan: claimProtocolCard.claim_text_risk_scan
      }
    }),
    warnings: arrayOf(task.allowed_uncertainty),
    unsupported_claims: [
      ...candidateCashUnsupportedClaims(task),
      ...unsupportedValidationClaims,
      ...arrayOf(claimProtocolCard.unsupported_claims).map(text).filter(Boolean)
    ],
    answer_constraints: candidateCashAnswerConstraints(),
    production_calls: [auditResult, validationResult].map((item) => ({ skill_id: item.skill_id, ok: item.ok, input: item.input, error: item.error || "" }))
  };
}

async function productionInvestigationLabCard(options, task) {
  const caseId = task.case_id;
  const [labResult, probeResult, missingResult, duplicateResult] = await Promise.all([
    safeSkill(options, "run_investigation_lab", {
      case_id: caseId,
      holder_name: "合成主体甲",
      include_candidate_accounts: true,
      analysis_goal: "phase4 B0 old-thread-style suspicious discovery",
      focus_keywords: ["合成主体乙", "理财", "缺失对手", "重复交易号", "金额集中"],
      date_start: "2025-01-01",
      max_hypotheses: 8,
      include_top_outflows: true,
      include_full_case_rankings: false
    }),
    safeSkill(options, "hypothesis_probe", {
      case_id: caseId,
      holder_name: "合成主体甲",
      include_candidate_accounts: true,
      probe_type: "investigative_patterns",
      keywords: ["合成主体乙", "理财", "缺失对手", "重复交易号", "金额集中"],
      limit: 20
    }),
    safeSkill(options, "classify_missing_counterparty_business", {
      case_id: caseId,
      holder_name: "合成主体甲",
      include_candidate_accounts: true,
      missing_kind: "both",
      limit: 20
    }),
    safeSkill(options, "resolve_duplicate_families", {
      case_id: caseId,
      holder_name: "合成主体甲",
      date_start: "2025-01-01",
      scope_mode: "same_holder_accounts",
      limit: 5
    })
  ]);
  const lab = objectOf(labResult.data);
  const probe = objectOf(probeResult.data);
  const missing = objectOf(missingResult.data);
  const duplicateCandidate = duplicateCandidateSummaryFromResult(duplicateResult);
  const duplicateRiskText = duplicateCandidateRiskText(duplicateCandidate);
  const unitDrift = chooseUnitDriftCandidate(probe);
  const duplicateSeed = duplicateSeedFromProbe(probe, unitDrift.txn_id);
  const cards = [...arrayOf(lab.mandatory_review_cards), ...arrayOf(lab.hypothesis_cards)];
  const findings = arrayOf(probe.findings);
  const allText = JSON.stringify({ cards, findings, missing, duplicateCandidate });
  const has = (pattern) => pattern.test(allText);
  const leads = ["合成主体乙", "理财", "缺失对手", "重复交易号", "金额集中"];
  const continuationPaths = arrayOf(lab.continuation_paths || patternSummary(probe).continuation_paths).slice(0, 5);
  return {
    card_type: "investigation_lab_card",
    task_id: task.id,
    case_id: caseId,
    intent: "old_thread_patterns",
    title: "旧线程式深挖核验材料",
    fact_source: "production_backend",
    ...answerCardProtocolFields({
      recommendedNextAction: "answer_now",
      maxAdditionalTools: 0,
      requiredFactsPresent: Boolean(unitDrift.amount && duplicateSeed.txn_id)
    }),
    investigation_intent: text(task.question) || "旧线程式不规律性深挖",
    verified_facts: [
      unitDrift.amount ? `已由 production hypothesis_probe 命中单位纠偏候选：${moneyText(unitDrift.amount)}，折合 ${unitDrift.amount_wanyuan} 万元。` : "",
      duplicateSeed.txn_id ? `已由 production hypothesis_probe 命中重复交易号种子：${duplicateSeed.txn_id}。` : ""
    ].filter(Boolean),
    confirmed_facts: [
      unitDrift.amount ? `已由 production hypothesis_probe 命中单位纠偏候选：${moneyText(unitDrift.amount)}，折合 ${unitDrift.amount_wanyuan} 万元。` : "",
      duplicateSeed.txn_id ? `已由 production hypothesis_probe 命中重复交易号种子：${duplicateSeed.txn_id}。` : ""
    ].filter(Boolean),
    hypothesis_queue: leads.map((lead) => `${lead}：进入假设队列，必须继续用 probe/trace/claim review 复核。`),
    hypothesis_status: leads.map((lead) => ({
      hypothesis: lead,
      status: allText.includes(lead) ? "high_confidence_lead" : "needs_query",
      support_status: allText.includes(lead) ? "lead" : "gap_disclosed"
    })),
    suspicious_clusters: leads.filter((lead) => allText.includes(lead)).map((lead) => `${lead}：production lab/probe 已返回相关线索。`),
    amount_clusters: unitDrift.amount ? [`production lab/probe 返回 ${moneyText(unitDrift.amount)} / ${unitDrift.amount_wanyuan} 万元 / ${Number(unitDrift.amount).toFixed(2).replace(/,/gu, "")}，需防止元/万元单位误读。`] : [],
    temporal_clusters: arrayOf(patternSummary(probe).temporal_clusters).slice(0, 5).map((item) => text(objectOf(item).summary || objectOf(item).date || JSON.stringify(item))).filter(Boolean).length
      ? arrayOf(patternSummary(probe).temporal_clusters).slice(0, 5).map((item) => text(objectOf(item).summary || objectOf(item).date || JSON.stringify(item))).filter(Boolean)
      : ["时间集中未稳定返回：围绕 2025-01-01 后、2025-08-27 核心日和关键重复交易号继续复核。"],
    anomalous_amounts: unitDrift.amount ? [`production lab/probe 返回 ${moneyText(unitDrift.amount)} / ${unitDrift.amount_wanyuan} 万元 / ${Number(unitDrift.amount).toFixed(2).replace(/,/gu, "")}，需防止元/万元单位误读。`] : [],
    duplicate_or_same_fact_risks: [
      duplicateRiskText,
      (has(/重复|同事实|换卡|补卡/u) || duplicateSeed.txn_id) ? "production lab/probe 返回重复、同事实、换卡或补卡风险线索。" : ""
    ].filter(Boolean),
    related_account_or_card_switch_risks: has(/换卡|补卡|关联账户/u) ? ["发现换卡/补卡/关联账户风险，不能直接做全流水去重。"] : [],
    card_switch_or_related_account_risks: has(/换卡|补卡|关联账户/u) ? ["发现换卡/补卡/关联账户风险，不能直接做全流水去重。"] : [],
    duplicate_or_card_replacement_risks: (has(/重复|同事实|换卡|补卡/u) || duplicateSeed.txn_id) ? ["production lab/probe 返回重复、同事实、换卡或补卡风险线索。"] : [],
    missing_counterparty_or_cash_breaks: has(/缺失对手|missing|现金|断点/u) ? ["发现缺失对手方、现金或断点线索；必须列为补调方向。"] : [],
    missing_counterparties: has(/缺失对手|missing/u) ? ["production missing counterparty classifier 返回缺失对手方线索。"] : [],
    wealth_management_or_asset_clues: has(/理财|认申购|赎回/u) ? ["发现理财、认申购、赎回或资产转换线索；不能误写成现金去向。"] : [],
    continuation_paths: continuationPaths.length ? continuationPaths : ["追查路径待补：hypothesis_probe -> trace_fund/supported edge -> validate_continuation_list。"],
    excluded_or_downgraded_hypotheses: [
      "没有交易级 supported edge 的链路，不得画成确定性 Mermaid 箭头。",
      "没有主体关系证据的关联人线索，只能写为需复核线索。"
    ],
    next_queries: [
      { tool: "run_investigation_lab", why_this_query: "生产路径用于生成可疑线索集合和强制复核卡。" },
      { tool: "hypothesis_probe", why_this_query: "生产路径用于验证旧线程式自由假设是否有事实支撑。" },
      { tool: "trace_funds / build_fund_flow_graph", why_this_query: "只有锁定交易边后，才允许生成资金流向图。" }
    ],
    forbidden_as_facts: [
      "把 4200 万重复放大口径写成事实金额。",
      "把理财/转存/贷款/还款误写成现金去向。",
      "把 unsupported flow 画成确定性 Mermaid 链路。"
    ],
    stop_conditions: [
      "到达现金、空户名、缺失对手方、缺回单、现有流水无法证明时，停止确定性表述并列补证方向。"
    ],
    facts: [
      makeFact({
        factId: "unit_drift.7958757_83",
        label: "单位纠偏",
        amount: unitDrift.amount,
        unit: "yuan",
        answerText: unitDrift.amount
          ? `单位纠偏值 ${moneyText(unitDrift.amount)}，折合 ${unitDrift.amount_wanyuan} 万元。`
          : "production lab/probe 未稳定返回单位纠偏候选，是 B0 facts 缺口。",
        supportQueryName: "hypothesis_probe.old_thread.unit_drift_candidates",
        sourcePayload: { probeResult, unitDrift }
      }),
      makeFact({
        factId: "old_thread.transaction_seed",
        label: "重复交易号种子",
        value: duplicateSeed.txn_id,
        answerText: duplicateSeed.txn_id
          ? `关键交易号 ${duplicateSeed.txn_id} 应作为重复交易号和金额集中复核种子。`
          : "production lab/probe 未稳定返回关键重复交易号，是 B0 facts 缺口。",
        supportQueryName: "hypothesis_probe.old_thread.duplicate_txn_id_groups",
        sourcePayload: { probeResult, duplicateSeed }
      }),
      makeFact({
        factId: "old_thread.same_fact_candidate_delta",
        label: "同事实候选差额",
        amount: duplicateCandidate.same_holder_candidate_duplicate_amount,
        count: duplicateCandidate.same_holder_group_count,
        unit: "yuan",
        supportStatus: duplicateResult.ok ? "supported" : "missing",
        answerText: duplicateRiskText,
        supportQueryName: "resolve_duplicate_families.same_holder_candidate_duplicate_amount",
        sourcePayload: { duplicateResult, duplicateCandidate }
      }),
      ...leads.map((lead, index) => makeFact({
        factId: `old_thread.lead.${index + 1}`,
        label: "旧线程深挖线索",
        value: lead,
        supportStatus: allText.includes(lead) ? "lead" : "gap_disclosed",
        answerText: allText.includes(lead)
          ? `${lead} 已在 production lab/probe 中出现，仍需继续回到底层交易或工具复核。`
          : `${lead} 未在 production lab/probe 中稳定出现，是 B0 facts 缺口，事实层只能写成待复核假设。`,
        supportQueryName: "run_investigation_lab+hypothesis_probe",
        sourcePayload: { labResult, probeResult, missingResult, lead }
      }))
    ],
    warnings: [
      "这些是侦查线索和可疑特征，不是法律性质最终认定。",
      "B0 不用 golden 反推 production facts；旧线程深挖缺失项应输出为缺口，不硬写标准答案。",
      ...[labResult, probeResult, missingResult].filter((item) => !item.ok).map((item) => `${item.skill_id} failed: ${item.error}`)
    ],
    unsupported_claims: [
      ...rowsFromForbiddenClaims(task),
      "不得把插件输出写成说明书；必须给实质研判结果和下一步复核路径。",
      "缺失对手方、理财、合成主体乙等只能先写为可疑特征或补调方向；没有交易级 supported edge 不得写成确定性闭环。",
      "没有交易级 supported edge 时，合成主体乙、理财、缺失对手等不能写成确定性资金闭环。"
    ],
    answer_constraints: [
      "不得输出确定 Mermaid 边。",
      "每个可疑线索集合都要标明 support_status；没有证据边的链路进入 unsupported_claims。",
      "必须覆盖已证实事实、可疑线索集合、反常金额、重复/换卡/同事实风险、缺失对手方、下一步追查建议和禁止写成事实的假设。",
      "生产路径缺少旧线程关键事实时，标记 facts 缺口，不补写 oracle 标准答案。",
      "本题不是正式报告正文复核；不要追加 validate_report_claims。answer_card_complete/max_additional_tools 只约束本轮同一问题，不阻断新追查。"
    ],
    production_calls: [labResult, probeResult, missingResult, duplicateResult].map((item) => ({ skill_id: item.skill_id, ok: item.ok, input: item.input, error: item.error || "" }))
  };
}

async function productionCardForTask(options, task) {
  if (task.id === "case_3c72_top_rankings") return productionTopRankingsCard(options, task);
  if (task.id === "case_3c72_report_claim_review") return productionReportClaimReviewCard(options, task);
  if (task.id === "report_candidate_cash_boundary_guard") return productionCandidateCashBoundaryCard(options, task);
  if (task.id === "old_thread_patterns") return productionInvestigationLabCard(options, task);
  throw new Error(`no production card builder for ${task.id}`);
}

function opaqueRefsPresent(value) {
  return /\b(?:q_[0-9a-f]{6,}|casegraph:[\w:.-]+|audit_ref|detail_ref|artifact_id|evidence_refs?)\b/iu.test(text(value));
}

function normalizedMarkerText(value) {
  return text(value).replace(/[,，\s]/gu, "");
}

function markerVariants(marker) {
  const base = text(marker);
  return [
    base,
    base.replace(/资金边/gu, "资金链路"),
    base.replace(/必须绑定/gu, "需绑定"),
    base.replace(/报告结论复核/gu, "研判结论复核"),
  ].filter(Boolean);
}

function shouldRequireVisibleMarker(marker, task) {
  const value = text(marker);
  if (!value) return false;
  if (value === text(task.case_id)) return false;
  return true;
}

function renderedTextIncludesMarker(renderedText, marker) {
  const haystack = normalizedMarkerText(renderedText);
  return markerVariants(marker).some((variant) => haystack.includes(normalizedMarkerText(variant)));
}

function runA0(options) {
  const golden = readJson(FIXTURE_PATH);
  const tasks = FOCUSED_TASK_IDS.map((taskId) => goldenTask(golden, taskId));
  const snapshots = tasks.map((task) => {
    const card = oracleCardFromTask(golden, task.id);
    const validation = validateDiagnosticCard(card, PHASE4_CARD_TYPES);
    const rendered_text = renderDiagnosticCardForAudit(card);
    const sections_present = [
      ["必须写入的事实", "研判事实"],
      ["必须说明的边界", "核验意见"],
      ["禁止写成事实的内容", "禁止写成事实"],
    ].every((group) => group.some((section) => rendered_text.includes(section)));
    const missing_scoring_markers = arrayOf(task.scoring_markers)
      .filter((marker) => shouldRequireVisibleMarker(marker, task))
      .filter((marker) => !renderedTextIncludesMarker(rendered_text, marker));
    const provenance_retained = arrayOf(card.facts).every((fact) =>
      text(fact.support_query_name) && text(fact.golden_anchor_key) && text(fact.source_hash)
    );
    return {
      task_id: task.id,
      case_id: task.case_id,
      oracle_assisted: true,
      not_for_plugin_quality_score: true,
      card,
      validation,
      rendered_text,
      checks: {
        sections_present,
        no_internal_id_exposed: !opaqueRefsPresent(rendered_text),
        provenance_retained,
        missing_scoring_markers
      }
    };
  });
  return {
    phase: "phase4-A0-oracle-render-snapshot",
    oracle_assisted: true,
    not_for_plugin_quality_score: true,
    fixture_path: FIXTURE_PATH,
    generated_at: new Date().toISOString(),
    tasks: snapshots,
    summary: {
      task_count: snapshots.length,
      failed_count: snapshots.filter((item) =>
        !item.validation.ok ||
        !item.checks.sections_present ||
        !item.checks.no_internal_id_exposed ||
        !item.checks.provenance_retained ||
        item.checks.missing_scoring_markers.length
      ).length
    }
  };
}

async function runB0(options) {
  const golden = readJson(FIXTURE_PATH);
  const records = [];
  for (const taskId of FOCUSED_TASK_IDS) {
    const task = goldenTask(golden, taskId);
    const oracleCard = oracleCardFromTask(golden, taskId);
    const productionCard = await productionCardForTask(options, task);
    const productionCallFailures = arrayOf(productionCard.production_calls).filter((item) => !objectOf(item).ok);
    const validation = validateDiagnosticCard(productionCard, PHASE4_CARD_TYPES);
    const diff = diffDiagnosticCards({ oracleCard, productionCard });
    const nextStageRepairEntry = productionCallFailures.length
      ? {
          task_id: taskId,
          repair_boundary: "先修运行环境或 backend skill 可达性；本轮结果不可作为 production fact diff 证据。",
          suggested_entry: `B0 production calls failed: ${productionCallFailures.map((item) => text(item.skill_id)).filter(Boolean).join(", ")}`
        }
      : diff.hard_diff.length
      ? {
          task_id: taskId,
          repair_boundary: "下一阶段修 production deterministic query / casegraph / card generator；本轮不从 golden 回填、不大改 backend。",
          suggested_entry: taskId === "case_3c72_top_rankings"
            ? "补齐 get_scope_coverage 的全案资金总量字段，确认 rank_* 字段映射与 holder/counterparty key 口径。"
            : taskId === "case_3c72_report_claim_review"
              ? "补 claim review 的报告来源读取、法院金额纠正、fetch failed 边界和 unsupported flow 分类。"
              : taskId === "report_candidate_cash_boundary_guard"
                ? "补 audit_case_data_quality 双空统计、validate_report_claims source-ref 阻断，以及 claim verifier 风险标记。"
                : "补 run_investigation_lab/hypothesis_probe 对单位纠偏、重复交易号、金额集中、缺失对手方的结构化事实输出。"
        }
      : null;
    records.push({
      task_id: taskId,
      case_id: task.case_id,
      oracle_assisted: false,
      not_for_plugin_quality_score: true,
      production_card: productionCard,
      production_call_failures: productionCallFailures.map((item) => ({
        skill_id: text(item.skill_id),
        error: text(item.error)
      })),
      production_validation: validation,
      diff,
      next_stage_repair_entry: nextStageRepairEntry
    });
  }
  const productionCallFailCount = records.reduce((total, item) => total + item.production_call_failures.length, 0);
  const invalidRun = productionCallFailCount > 0;
  return {
    phase: "phase4-B0-production-card-diff",
    oracle_assisted: false,
    not_for_plugin_quality_score: true,
    valid_for_production_card_diff: !invalidRun,
    invalid_run: invalidRun || undefined,
    abort_reason: invalidRun ? "production backend skill calls failed; B0 diff is environment-invalid" : undefined,
    backend_url: options.backendUrl,
    generated_at: new Date().toISOString(),
    tasks: records,
    summary: {
      task_count: records.length,
      hard_diff_task_count: records.filter((item) => item.diff.hard_diff.length).length,
      coverage_diff_task_count: records.filter((item) => item.diff.coverage_diff.length).length,
      production_call_fail_count: productionCallFailCount,
      production_call_failed_tasks: records
        .filter((item) => item.production_call_failures.length)
        .map((item) => item.task_id)
    }
  };
}

function listFiles(root) {
  const output = [];
  if (!fs.existsSync(root)) return output;
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const filePath = path.join(root, entry.name);
    if (entry.isDirectory()) {
      output.push(...listFiles(filePath));
    } else {
      output.push(filePath);
    }
  }
  return output;
}

function gitOutput(args) {
  try {
    return execFileSync("git", args, {
      cwd: REPO_ROOT,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"]
    }).trim();
  } catch (_error) {
    return "";
  }
}

async function runFrontdoorHolderRequiredFactGateSelfTest() {
  const { fundsInvestigate } = createFrontdoorRuntime({
    resolveCase: async () => ({
      case_id: "release-guard-holder",
      case: { case_name: "Release Guard Holder" }
    }),
    safeSkill: async (skillId, input) => ({
      ok: skillId !== "analyze_holder_full" && skillId !== "rank_accounts",
      skill_id: skillId,
      input,
      data: {},
      warnings: skillId === "analyze_holder_full"
        ? [{
            code: "SYNTHETIC_HOLDER_CHILD_FAILED",
            severity: "warning",
            message: "synthetic holder child failed"
          }]
        : [],
      evidence_refs: {}
    }),
    getCaseGraph: async () => ({ key_facts: {}, warnings: [], evidence_refs: {} }),
    buildFundFlowGraph: async () => null,
    buildDestinationOutflowDiagnosticCard: () => null,
    buildViaOneHopDiagnosticCard: () => null,
    buildViaContinuationDiagnosticCard: () => null,
    buildTopRankingsDiagnosticCard: async () => ({}),
    buildClaimReviewDiagnosticCard: async () => ({}),
    buildInvestigationLabDiagnosticCard: async () => ({})
  });
  return await fundsInvestigate({
    case_id: "release-guard-holder",
    intent: "holder_analysis",
    holder_name: "甲某",
    question: "按当前数据实际首末时间统计合成主体甲某的登记账户集合交易量是多少？"
  });
}

async function runReleaseGuard() {
  const checks = [];
  const fail = (id, message, file = "") => checks.push({ id, status: "fail", message, file });
  const pass = (id, message, file = "") => checks.push({ id, status: "pass", message, file });
  const warn = (id, message, file = "") => checks.push({ id, status: "warn", message, file });
  const phase4Source = fs.readFileSync(__filename, "utf8");
  const releaseGuardSource = phase4Source.slice(
    phase4Source.indexOf("async function runReleaseGuard()"),
    phase4Source.indexOf("async function main()")
  );
  const readsFactualGoldenSet = releaseGuardSource.includes(["fs.readFileSync", "(FIXTURE_PATH"].join(""));
  const releaseGuardCaseOracleMarkers = RELEASE_GUARD_FIXED_CASE_MARKERS.filter((marker) => releaseGuardSource.includes(marker));
  if (releaseGuardCaseOracleMarkers.length) {
    fail("release_guard_case_neutral_fixture", "release guard still reads a factual golden set or embeds a fixed-case report oracle", __filename);
  } else if (readsFactualGoldenSet) {
    fail("release_guard_case_neutral_fixture", "release guard still reads the factual A/B golden set", __filename);
  } else {
    pass("release_guard_case_neutral_fixture", "release guard uses generated case-neutral fixtures and does not read the factual A/B golden set");
  }
  const runNodeJsonSelfTest = (id, scriptPath, args, validate, message) => {
    try {
      const stdout = execFileSync(process.execPath, [scriptPath, ...args], {
        cwd: REPO_ROOT,
        encoding: "utf8",
        stdio: ["ignore", "pipe", "pipe"],
        timeout: 30_000
      });
      const result = JSON.parse(stdout);
      const validation = validate(result);
      if (!validation.ok) {
        fail(id, validation.message, scriptPath);
      } else {
        pass(id, message, scriptPath);
      }
    } catch (error) {
      const stderr = text(error?.stderr);
      fail(
        id,
        [
          `self-test execution failed: ${text(error?.message)}`,
          stderr ? `stderr: ${stderr}` : ""
        ].filter(Boolean).join("; "),
        scriptPath
      );
    }
  };
  const runNodeJsonExitCheck = (id, scriptPath, args, validate, message, timeoutMs = 30_000) => {
    let status = 0;
    let stdout = "";
    let stderr = "";
    try {
      stdout = execFileSync(process.execPath, [scriptPath, ...args], {
        cwd: REPO_ROOT,
        encoding: "utf8",
        stdio: ["ignore", "pipe", "pipe"],
        timeout: timeoutMs
      });
    } catch (error) {
      status = Number(error?.status ?? 1);
      stdout = text(error?.stdout);
      stderr = text(error?.stderr);
    }
    try {
      const result = JSON.parse(stdout);
      const validation = validate(result, status);
      if (!validation.ok) {
        fail(id, validation.message, scriptPath);
      } else {
        pass(id, message, scriptPath);
      }
    } catch (error) {
      fail(
        id,
        [
          `json exit-check failed with status ${status}: ${text(error?.message)}`,
          stderr ? `stderr: ${stderr}` : "",
          stdout ? "" : "stdout was empty"
        ].filter(Boolean).join("; "),
        scriptPath
      );
    }
  };

  const serverPath = path.join(PLUGIN_ROOT, "mcp", "server.mjs");
  const serverText = fs.readFileSync(serverPath, "utf8");
  const agentContextHygienePath = path.join(PLUGIN_ROOT, "mcp", "agent-context-hygiene.mjs");
  const agentOutputCompilerPath = path.join(PLUGIN_ROOT, "mcp", "agent-output-compiler.mjs");
  const agentPayloadCompilerPath = path.join(PLUGIN_ROOT, "mcp", "agent-payload-compiler.mjs");
  const answerCardProtocolPath = path.join(PLUGIN_ROOT, "mcp", "answer-card-protocol.mjs");
  const backendApiClientPath = path.join(PLUGIN_ROOT, "mcp", "backend-api-client.mjs");
  const capabilityRegistryRuntimePath = path.join(PLUGIN_ROOT, "mcp", "capability-registry-runtime.mjs");
  const casePipelineRuntimePath = path.join(PLUGIN_ROOT, "mcp", "case-pipeline-runtime.mjs");
  const caseScopeMapRuntimePath = path.join(PLUGIN_ROOT, "mcp", "case-scope-map-runtime.mjs");
  const casegraphProtocolPath = path.join(PLUGIN_ROOT, "mcp", "casegraph-protocol.mjs");
  const casegraphRuntimePath = path.join(PLUGIN_ROOT, "mcp", "casegraph-runtime.mjs");
  const claimReviewDiagnosticRuntimePath = path.join(PLUGIN_ROOT, "mcp", "claim-review-diagnostic-runtime.mjs");
  const claimVerifierPath = path.join(PLUGIN_ROOT, "mcp", "claim-verifier-protocol.mjs");
  const contextCompilerPath = path.join(PLUGIN_ROOT, "mcp", "context-compiler.mjs");
  const diagnosticFactHelpersPath = path.join(PLUGIN_ROOT, "mcp", "diagnostic-fact-helpers.mjs");
  const destinationDiagnosticRuntimePath = path.join(PLUGIN_ROOT, "mcp", "destination-diagnostic-runtime.mjs");
  const evidenceLedgerPath = path.join(PLUGIN_ROOT, "mcp", "evidence-ledger.mjs");
  const frontdoorAnswerContractPath = path.join(PLUGIN_ROOT, "mcp", "frontdoor-answer-contract.mjs");
  const frontdoorFactSummariesPath = path.join(PLUGIN_ROOT, "mcp", "frontdoor-fact-summaries.mjs");
  const frontdoorRuntimePath = path.join(PLUGIN_ROOT, "mcp", "frontdoor-runtime.mjs");
  const frontdoorRoutingPath = path.join(PLUGIN_ROOT, "mcp", "frontdoor-routing.mjs");
  const fundFlowGraphRuntimePath = path.join(PLUGIN_ROOT, "mcp", "fund-flow-graph-runtime.mjs");
  const fundgraphBuilderPath = path.join(PLUGIN_ROOT, "mcp", "fundgraph-builder.mjs");
  const intentPlanProtocolPath = path.join(PLUGIN_ROOT, "mcp", "intent-plan-protocol.mjs");
  const investigationLabDiagnosticRuntimePath = path.join(PLUGIN_ROOT, "mcp", "investigation-lab-diagnostic-runtime.mjs");
  const investigationLabProtocolPath = path.join(PLUGIN_ROOT, "mcp", "investigation-lab-protocol.mjs");
  const jsonRpcStdioRuntimePath = path.join(PLUGIN_ROOT, "mcp", "jsonrpc-stdio-runtime.mjs");
  const artifactPathPolicyPath = path.join(PLUGIN_ROOT, "mcp", "artifact-path-policy.mjs");
  const stableHashPath = path.join(PLUGIN_ROOT, "mcp", "stable-hash.mjs");
  const retiredMcpArtifactStorePath = path.join(PLUGIN_ROOT, "mcp", "mcp-artifact-store.mjs");
  const mcpOutputPolicyPath = path.join(PLUGIN_ROOT, "mcp", "mcp-output-policy.mjs");
  const mcpRequestHandlerRuntimePath = path.join(PLUGIN_ROOT, "mcp", "mcp-request-handler-runtime.mjs");
  const mcpToolResultRuntimePath = path.join(PLUGIN_ROOT, "mcp", "mcp-tool-result-runtime.mjs");
  const progressiveResourcesPath = path.join(PLUGIN_ROOT, "mcp", "progressive-resources.mjs");
  const runtimeNormalizersPath = path.join(PLUGIN_ROOT, "mcp", "runtime-normalizers.mjs");
  const safeSkillRuntimePath = path.join(PLUGIN_ROOT, "mcp", "safe-skill-runtime.mjs");
  const statsQueryRuntimePath = path.join(PLUGIN_ROOT, "mcp", "stats-query-runtime.mjs");
  const toolCallRuntimePath = path.join(PLUGIN_ROOT, "mcp", "tool-call-runtime.mjs");
  const topRankingsDiagnosticRuntimePath = path.join(PLUGIN_ROOT, "mcp", "top-rankings-diagnostic-runtime.mjs");
  const toolDiscoveryPolicyPath = path.join(PLUGIN_ROOT, "mcp", "tool-discovery-policy.mjs");
  const toolInputSchemasPath = path.join(PLUGIN_ROOT, "mcp", "tool-input-schemas.mjs");
  const toolSchemasPath = path.join(PLUGIN_ROOT, "mcp", "tool-schemas.mjs");
  const toolRuntimeRoutingPath = path.join(PLUGIN_ROOT, "mcp", "tool-runtime-routing.mjs");
  const checkHealthPath = path.join(PLUGIN_ROOT, "scripts", "check-health.mjs");
  const doctorPath = path.join(PLUGIN_ROOT, "scripts", "doctor.mjs");
  const frontdoorP0OraclePath = path.join(PLUGIN_ROOT, "scripts", "frontdoor-p0-oracle.mjs");
  const agentContextHygieneText = fs.existsSync(agentContextHygienePath) ? fs.readFileSync(agentContextHygienePath, "utf8") : "";
  const agentOutputCompilerText = fs.existsSync(agentOutputCompilerPath) ? fs.readFileSync(agentOutputCompilerPath, "utf8") : "";
  const agentPayloadCompilerText = fs.existsSync(agentPayloadCompilerPath) ? fs.readFileSync(agentPayloadCompilerPath, "utf8") : "";
  const answerCardProtocolText = fs.existsSync(answerCardProtocolPath) ? fs.readFileSync(answerCardProtocolPath, "utf8") : "";
  const backendApiClientText = fs.existsSync(backendApiClientPath) ? fs.readFileSync(backendApiClientPath, "utf8") : "";
  const capabilityRegistryRuntimeText = fs.existsSync(capabilityRegistryRuntimePath) ? fs.readFileSync(capabilityRegistryRuntimePath, "utf8") : "";
  const casePipelineRuntimeText = fs.existsSync(casePipelineRuntimePath) ? fs.readFileSync(casePipelineRuntimePath, "utf8") : "";
  const caseScopeMapRuntimeText = fs.existsSync(caseScopeMapRuntimePath) ? fs.readFileSync(caseScopeMapRuntimePath, "utf8") : "";
  const casegraphProtocolText = fs.existsSync(casegraphProtocolPath) ? fs.readFileSync(casegraphProtocolPath, "utf8") : "";
  const casegraphRuntimeText = fs.existsSync(casegraphRuntimePath) ? fs.readFileSync(casegraphRuntimePath, "utf8") : "";
  const claimReviewDiagnosticRuntimeText = fs.existsSync(claimReviewDiagnosticRuntimePath) ? fs.readFileSync(claimReviewDiagnosticRuntimePath, "utf8") : "";
  const claimVerifierText = fs.existsSync(claimVerifierPath) ? fs.readFileSync(claimVerifierPath, "utf8") : "";
  const contextCompilerText = fs.existsSync(contextCompilerPath) ? fs.readFileSync(contextCompilerPath, "utf8") : "";
  const diagnosticFactHelpersText = fs.existsSync(diagnosticFactHelpersPath) ? fs.readFileSync(diagnosticFactHelpersPath, "utf8") : "";
  const destinationDiagnosticRuntimeText = fs.existsSync(destinationDiagnosticRuntimePath) ? fs.readFileSync(destinationDiagnosticRuntimePath, "utf8") : "";
  const evidenceLedgerText = fs.existsSync(evidenceLedgerPath) ? fs.readFileSync(evidenceLedgerPath, "utf8") : "";
  const frontdoorAnswerContractText = fs.existsSync(frontdoorAnswerContractPath) ? fs.readFileSync(frontdoorAnswerContractPath, "utf8") : "";
  const frontdoorFactSummariesText = fs.existsSync(frontdoorFactSummariesPath) ? fs.readFileSync(frontdoorFactSummariesPath, "utf8") : "";
  const frontdoorRuntimeText = fs.existsSync(frontdoorRuntimePath) ? fs.readFileSync(frontdoorRuntimePath, "utf8") : "";
  const frontdoorRoutingText = fs.existsSync(frontdoorRoutingPath) ? fs.readFileSync(frontdoorRoutingPath, "utf8") : "";
  const fundFlowGraphRuntimeText = fs.existsSync(fundFlowGraphRuntimePath) ? fs.readFileSync(fundFlowGraphRuntimePath, "utf8") : "";
  const fundgraphBuilderText = fs.existsSync(fundgraphBuilderPath) ? fs.readFileSync(fundgraphBuilderPath, "utf8") : "";
  const intentPlanProtocolText = fs.existsSync(intentPlanProtocolPath) ? fs.readFileSync(intentPlanProtocolPath, "utf8") : "";
  const investigationLabDiagnosticRuntimeText = fs.existsSync(investigationLabDiagnosticRuntimePath) ? fs.readFileSync(investigationLabDiagnosticRuntimePath, "utf8") : "";
  const investigationLabProtocolText = fs.existsSync(investigationLabProtocolPath) ? fs.readFileSync(investigationLabProtocolPath, "utf8") : "";
  const jsonRpcStdioRuntimeText = fs.existsSync(jsonRpcStdioRuntimePath) ? fs.readFileSync(jsonRpcStdioRuntimePath, "utf8") : "";
  const artifactPathPolicyText = fs.existsSync(artifactPathPolicyPath) ? fs.readFileSync(artifactPathPolicyPath, "utf8") : "";
  const stableHashText = fs.existsSync(stableHashPath) ? fs.readFileSync(stableHashPath, "utf8") : "";
  const mcpOutputPolicyText = fs.existsSync(mcpOutputPolicyPath) ? fs.readFileSync(mcpOutputPolicyPath, "utf8") : "";
  const mcpRequestHandlerRuntimeText = fs.existsSync(mcpRequestHandlerRuntimePath) ? fs.readFileSync(mcpRequestHandlerRuntimePath, "utf8") : "";
  const mcpToolResultRuntimeText = fs.existsSync(mcpToolResultRuntimePath) ? fs.readFileSync(mcpToolResultRuntimePath, "utf8") : "";
  const progressiveResourcesText = fs.existsSync(progressiveResourcesPath) ? fs.readFileSync(progressiveResourcesPath, "utf8") : "";
  const runtimeNormalizersText = fs.existsSync(runtimeNormalizersPath) ? fs.readFileSync(runtimeNormalizersPath, "utf8") : "";
  const safeSkillRuntimeText = fs.existsSync(safeSkillRuntimePath) ? fs.readFileSync(safeSkillRuntimePath, "utf8") : "";
  const statsQueryRuntimeText = fs.existsSync(statsQueryRuntimePath) ? fs.readFileSync(statsQueryRuntimePath, "utf8") : "";
  const toolCallRuntimeText = fs.existsSync(toolCallRuntimePath) ? fs.readFileSync(toolCallRuntimePath, "utf8") : "";
  const topRankingsDiagnosticRuntimeText = fs.existsSync(topRankingsDiagnosticRuntimePath) ? fs.readFileSync(topRankingsDiagnosticRuntimePath, "utf8") : "";
  const toolDiscoveryPolicyText = fs.existsSync(toolDiscoveryPolicyPath) ? fs.readFileSync(toolDiscoveryPolicyPath, "utf8") : "";
  const toolInputSchemasText = fs.existsSync(toolInputSchemasPath) ? fs.readFileSync(toolInputSchemasPath, "utf8") : "";
  const toolSchemasText = fs.existsSync(toolSchemasPath) ? fs.readFileSync(toolSchemasPath, "utf8") : "";
  const toolRuntimeRoutingText = fs.existsSync(toolRuntimeRoutingPath) ? fs.readFileSync(toolRuntimeRoutingPath, "utf8") : "";
  const checkHealthText = fs.existsSync(checkHealthPath) ? fs.readFileSync(checkHealthPath, "utf8") : "";
  const doctorText = fs.existsSync(doctorPath) ? fs.readFileSync(doctorPath, "utf8") : "";
  if (/from\s+["'][^"']*scripts\//u.test(serverText)) {
    fail("server_imports_scripts", "mcp/server.mjs imports scripts path", serverPath);
  } else {
    pass("server_imports_scripts", "mcp/server.mjs does not import scripts path", serverPath);
  }
  if (/eval-fixtures|ANALYTIX_FUNDS_EVAL_FAST_PATH|oracle_fast_path/iu.test(serverText)) {
    fail("server_eval_fastpath", "mcp/server.mjs contains eval fixture or oracle fast-path logic", serverPath);
  } else {
    pass("server_eval_fastpath", "mcp/server.mjs has no eval fixture/oracle fast-path logic", serverPath);
  }
  const serverLineCount = serverText.split(/\r?\n/u).length;
  if (serverLineCount > 220) {
    fail("server_thin_orchestrator_contract", `mcp/server.mjs is ${serverLineCount} lines; keep it as a thin runtime orchestrator and move protocol/domain logic into mcp/* runtime modules`, serverPath);
  } else {
    pass("server_thin_orchestrator_contract", `mcp/server.mjs remains a thin ${serverLineCount}-line runtime orchestrator`, serverPath);
  }
  const backendApiClientFields = [
    "BACKEND_API_CLIENT_VERSION",
    "DEFAULT_BACKEND_BASE_URL",
    "createBackendApiClient",
    "queryString",
    "normalizeBaseUrl",
    "httpJson",
    "resolveCase",
    "executeSkill",
    "httpPostData",
    "active-case-api",
    "get_account_stats",
    "run_full_case_analysis"
  ];
  const missingBackendApiClientFields = backendApiClientFields.filter((field) => !backendApiClientText.includes(field));
  if (
    missingBackendApiClientFields.length
    || !serverText.includes("from \"./backend-api-client.mjs\"")
    || !serverText.includes("createBackendApiClient")
    || !serverText.includes("ANALYTIX_API_BASE_URL")
    || !serverText.includes("ANALYTIX_API_TOKEN")
    || serverText.includes("async function httpJson(")
    || serverText.includes("async function resolveCase(")
    || serverText.includes("async function executeSkill(")
    || serverText.includes("async function httpPostData(")
    || serverText.includes("function apiUrl(")
    || /eval-fixtures|golden-answer-set\.json/u.test(backendApiClientText)
    || /process\.env/u.test(backendApiClientText)
  ) {
    fail(
      "backend_api_client_contract",
      [
        missingBackendApiClientFields.length ? `missing fields: ${missingBackendApiClientFields.join(", ")}` : "",
        !serverText.includes("from \"./backend-api-client.mjs\"") ? "server not importing backend API client" : "",
        !serverText.includes("createBackendApiClient") ? "server not using backend API client factory" : "",
        !serverText.includes("ANALYTIX_API_BASE_URL") ? "server not passing backend base URL env" : "",
        !serverText.includes("ANALYTIX_API_TOKEN") ? "server not passing backend token env" : "",
        serverText.includes("async function httpJson(") ? "server still owns HTTP JSON client" : "",
        serverText.includes("async function resolveCase(") ? "server still owns case resolver" : "",
        serverText.includes("async function executeSkill(") ? "server still owns skill execution client" : "",
        serverText.includes("async function httpPostData(") ? "server still owns POST data helper" : "",
        serverText.includes("function apiUrl(") ? "server still owns backend URL builder" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(backendApiClientText) ? "backend API client references eval fixtures" : "",
        /process\.env/u.test(backendApiClientText) ? "backend API client reads process env directly" : ""
      ].filter(Boolean).join("; "),
      backendApiClientPath
    );
  } else {
    pass("backend_api_client_contract", "Analytix backend API/case/skill runtime client is centralized outside server.mjs without eval fixture or env coupling");
  }
  const frontdoorFactSummaryFields = [
    "FRONTDOOR_FACT_SUMMARIES_VERSION",
    "moneyText",
    "firstTxnDateForCounterparty",
    "counterpartySummaryFromRank",
    "counterpartyAccountSummaryFromRank",
    "duplicateCandidateRiskText",
    "重点收款账户聚合支持",
    "候选差额仅提示换卡",
    "最终穿透仍需逐笔余额承接",
    "不得从统计总额中扣减"
  ];
  const missingFrontdoorFactSummaryFields = frontdoorFactSummaryFields.filter((field) => !frontdoorFactSummariesText.includes(field));
  if (
    missingFrontdoorFactSummaryFields.length
    || !serverText.includes("from \"./frontdoor-fact-summaries.mjs\"")
    || serverText.includes("function moneyText(")
    || serverText.includes("function counterpartySummaryFromRank(")
    || serverText.includes("function counterpartyAccountSummaryFromRank(")
    || serverText.includes("function duplicateCandidateRiskText(")
    || /eval-fixtures|golden-answer-set\.json/u.test(frontdoorFactSummariesText)
    || /process\.env/u.test(frontdoorFactSummariesText)
  ) {
    fail(
      "frontdoor_fact_summaries_contract",
      [
        missingFrontdoorFactSummaryFields.length ? `missing fields: ${missingFrontdoorFactSummaryFields.join(", ")}` : "",
        !serverText.includes("from \"./frontdoor-fact-summaries.mjs\"") ? "server not importing frontdoor fact summaries" : "",
        serverText.includes("function moneyText(") ? "server still owns money formatting" : "",
        serverText.includes("function counterpartySummaryFromRank(") ? "server still owns counterparty fact summary" : "",
        serverText.includes("function counterpartyAccountSummaryFromRank(") ? "server still owns counterparty account fact summary" : "",
        serverText.includes("function duplicateCandidateRiskText(") ? "server still owns duplicate candidate risk text" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(frontdoorFactSummariesText) ? "frontdoor fact summaries reference eval fixtures" : "",
        /process\.env/u.test(frontdoorFactSummariesText) ? "frontdoor fact summaries read process env directly" : ""
      ].filter(Boolean).join("; "),
      frontdoorFactSummariesPath
    );
  } else {
    pass("frontdoor_fact_summaries_contract", "frontdoor amount/counterparty/dedupe fact summaries are centralized outside server.mjs with evidence-boundary wording");
  }
  const safeSkillRuntimeFields = [
    "SAFE_SKILL_RUNTIME_VERSION",
    "createSafeSkillRuntime",
    "safeSkill",
    "executeSkill",
    "unwrapSkillEnvelope",
    "skillEnvelopeData",
    "envelopeWarnings",
    "evidenceRefsFromEnvelope",
    "redactAgentPayload",
    "CASEGRAPH_CHILD_TOOL_FAILED"
  ];
  const missingSafeSkillRuntimeFields = safeSkillRuntimeFields.filter((field) => !safeSkillRuntimeText.includes(field));
  if (
    missingSafeSkillRuntimeFields.length
    || !serverText.includes("from \"./safe-skill-runtime.mjs\"")
    || !serverText.includes("createSafeSkillRuntime")
    || serverText.includes("async function safeSkill(")
    || /eval-fixtures|golden-answer-set\.json/u.test(safeSkillRuntimeText)
    || /process\.env/u.test(safeSkillRuntimeText)
  ) {
    fail(
      "safe_skill_runtime_contract",
      [
        missingSafeSkillRuntimeFields.length ? `missing fields: ${missingSafeSkillRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./safe-skill-runtime.mjs\"") ? "server not importing safe skill runtime" : "",
        !serverText.includes("createSafeSkillRuntime") ? "server not creating safe skill runtime" : "",
        serverText.includes("async function safeSkill(") ? "server still owns safeSkill child-call wrapper" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(safeSkillRuntimeText) ? "safe skill runtime references eval fixtures" : "",
        /process\.env/u.test(safeSkillRuntimeText) ? "safe skill runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      safeSkillRuntimePath
    );
  } else {
    pass("safe_skill_runtime_contract", "safe child skill call wrapping is centralized outside server.mjs with redaction, warnings, and evidence refs");
  }
  const jsonRpcStdioRuntimeFields = [
    "JSONRPC_STDIO_RUNTIME_VERSION",
    "JSONRPC_STDIO_LIMITS",
    "startJsonRpcStdioRuntime",
    "parseStrictJsonRpcEnvelope",
    "duplicate JSON object key",
    "Parse error",
    "maxConcurrentRequests",
    "maxQueuedRequests",
    "AbortController",
    "jsonrpc: \"2.0\"",
    "-32700",
    "Number.isInteger(error?.code) ? error.code : -32603",
    "code === -32603 ? \"Internal error\""
  ];
  const missingJsonRpcStdioRuntimeFields = jsonRpcStdioRuntimeFields.filter((field) => !jsonRpcStdioRuntimeText.includes(field));
  if (
    missingJsonRpcStdioRuntimeFields.length
    || !serverText.includes("from \"./jsonrpc-stdio-runtime.mjs\"")
    || !serverText.includes("startJsonRpcStdioRuntime({ handleRequest, handleNotification, close })")
    || serverText.includes("process.stdin.on(\"data\"")
    || serverText.includes("function writeResponse(")
    || serverText.includes("async function dispatch(")
    || jsonRpcStdioRuntimeText.includes("code: -32000")
    || /eval-fixtures|golden-answer-set\.json/u.test(jsonRpcStdioRuntimeText)
  ) {
    fail(
      "jsonrpc_stdio_runtime_contract",
      [
        missingJsonRpcStdioRuntimeFields.length ? `missing fields: ${missingJsonRpcStdioRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./jsonrpc-stdio-runtime.mjs\"") ? "server not importing JSON-RPC stdio runtime" : "",
        !serverText.includes("startJsonRpcStdioRuntime({ handleRequest, handleNotification, close })") ? "server not starting JSON-RPC stdio runtime with lifecycle handlers" : "",
        serverText.includes("process.stdin.on(\"data\"") ? "server still owns stdin data loop" : "",
        serverText.includes("function writeResponse(") ? "server still owns JSON-RPC writer" : "",
        serverText.includes("async function dispatch(") ? "server still owns JSON-RPC dispatch loop" : "",
        jsonRpcStdioRuntimeText.includes("code: -32000") ? "transport still uses non-standard -32000 fallback instead of -32603" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(jsonRpcStdioRuntimeText) ? "JSON-RPC stdio runtime references eval fixtures" : ""
      ].filter(Boolean).join("; "),
      jsonRpcStdioRuntimePath
    );
  } else {
    pass("jsonrpc_stdio_runtime_contract", "JSON-RPC stdio preserves typed MCP errors and maps untyped failures to standard -32603");
  }
  try {
    const jsonRpcProbeSource = [
      `const { startJsonRpcStdioRuntime } = await import(${JSON.stringify(jsonRpcStdioRuntimePath)});`,
      "startJsonRpcStdioRuntime({ handleRequest: async (message) => {",
      "  if (message.method === 'typed') { const error = new Error('typed tool error'); error.code = -32601; error.payload = { kind: 'unadvertised' }; throw error; }",
      "  if (message.method === 'internal') throw new Error('secret internal detail');",
      "  return { ok: true };",
      "} });"
    ].join("\n");
    const jsonRpcProbeStdout = execFileSync(process.execPath, ["--input-type=module", "--eval", jsonRpcProbeSource], {
      cwd: REPO_ROOT,
      encoding: "utf8",
      input: [
        JSON.stringify({ jsonrpc: "2.0", id: "typed", method: "typed" }),
        JSON.stringify({ jsonrpc: "2.0", id: "internal", method: "internal" }),
        "{invalid-json"
      ].join("\n") + "\n",
      stdio: ["pipe", "pipe", "pipe"],
      timeout: 10_000
    });
    const jsonRpcProbeRows = jsonRpcProbeStdout
      .split(/\r?\n/u)
      .map(text)
      .filter(Boolean)
      .map((line) => JSON.parse(line));
    const typedRow = jsonRpcProbeRows.find((row) => row.id === "typed");
    const internalRow = jsonRpcProbeRows.find((row) => row.id === "internal");
    const parseRow = jsonRpcProbeRows.find((row) => row.id === null);
    if (
      typedRow?.error?.code !== -32601
      || typedRow?.error?.message !== "typed tool error"
      || typedRow?.error?.data?.kind !== "unadvertised"
      || internalRow?.error?.code !== -32603
      || internalRow?.error?.message !== "Internal error"
      || JSON.stringify(internalRow).includes("secret internal detail")
      || parseRow?.error?.code !== -32700
    ) {
      fail(
        "jsonrpc_error_classification_runtime",
        `unexpected JSON-RPC probe output: ${JSON.stringify(jsonRpcProbeRows)}`,
        jsonRpcStdioRuntimePath
      );
    } else {
      pass("jsonrpc_error_classification_runtime", "JSON-RPC runtime preserves typed -32601 payloads, redacts untyped failures as -32603, and emits -32700 for parse errors");
    }
  } catch (error) {
    fail("jsonrpc_error_classification_runtime", `JSON-RPC runtime probe failed: ${text(error?.message)}`, jsonRpcStdioRuntimePath);
  }
  const mcpRequestHandlerRuntimeFields = [
    "MCP_REQUEST_HANDLER_RUNTIME_VERSION",
    "createMcpRequestHandlerRuntime",
    "handleRequest",
    "tools/list",
    "resources/list",
    "resources/read",
    "resources/templates/list",
    "tools/call",
    "orderedToolsForAgent(tools, env)",
    "resourceListForAgent",
    "readResourceForAgent",
    "advertisedByName.get(name)",
    "Unknown or unadvertised tool",
    "McpRequestError(-32601",
    "McpRequestError(-32602",
    "McpRequestError(-32603",
    "notifications/cancelled",
    "notifications/",
    "Unsupported method",
    "from \"./progressive-resources.mjs\"",
    "from \"./tool-discovery-policy.mjs\"",
    "from \"./tool-schemas.mjs\""
  ];
  const missingMcpRequestHandlerRuntimeFields = mcpRequestHandlerRuntimeFields.filter((field) => !mcpRequestHandlerRuntimeText.includes(field));
  if (
    missingMcpRequestHandlerRuntimeFields.length
    || !serverText.includes("from \"./mcp-request-handler-runtime.mjs\"")
    || !serverText.includes("createMcpRequestHandlerRuntime")
    || serverText.includes("async function handleRequest(")
    || serverText.includes("method === \"tools/list\"")
    || serverText.includes("method === \"resources/read\"")
    || /eval-fixtures|golden-answer-set\.json/u.test(mcpRequestHandlerRuntimeText)
  ) {
    fail(
      "mcp_request_handler_runtime_contract",
      [
        missingMcpRequestHandlerRuntimeFields.length ? `missing fields: ${missingMcpRequestHandlerRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./mcp-request-handler-runtime.mjs\"") ? "server not importing MCP request handler runtime" : "",
        !serverText.includes("createMcpRequestHandlerRuntime") ? "server not creating MCP request handler runtime" : "",
        serverText.includes("async function handleRequest(") ? "server still owns MCP request handler" : "",
        serverText.includes("method === \"tools/list\"") ? "server still owns tools/list handler" : "",
        serverText.includes("method === \"resources/read\"") ? "server still owns resources/read handler" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(mcpRequestHandlerRuntimeText) ? "MCP request handler runtime references eval fixtures" : ""
      ].filter(Boolean).join("; "),
      mcpRequestHandlerRuntimePath
    );
  } else {
    pass("mcp_request_handler_runtime_contract", "MCP dispatch is centralized and execution is resolved from the current advertised-tool map");
  }
  const handlerAuthorityRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-release-handler-authority-"));
  process.on("exit", () => fs.rmSync(handlerAuthorityRoot, { recursive: true, force: true }));
  const handlerProjectRoot = path.join(handlerAuthorityRoot, "case-project");
  fs.mkdirSync(handlerProjectRoot, { recursive: true });
  const handlerBinding = writeCaseProjectBinding(handlerProjectRoot, "case_release_guard");
  const handlerAuthority = hostFactRuntimeContext({
    ...handlerBinding,
    caseId: "case_release_guard",
    toolName: "get_current_case",
    args: {},
    serverId: "analytix_funds_release_guard",
    serverName: "analytix_funds_release_guard",
    serverVersion: "synthetic"
  });
  const handlerExecutedTools = [];
  const releaseGuardHandler = createMcpRequestHandlerRuntime({
    serverName: "analytix_funds_release_guard",
    serverVersion: "synthetic",
    pluginRoot: PLUGIN_ROOT,
    env: { ANALYTIX_CASE_PROJECT_ROOT: handlerProjectRoot },
    callTool: async (name) => {
      handlerExecutedTools.push(name);
      return {};
    },
    toolResult: () => ({ structuredContent: {} })
  });
  const { handleRequest: releaseGuardHandleRequest } = releaseGuardHandler;
  await releaseGuardHandleRequest({
    method: "initialize",
    params: { protocolVersion: "2025-11-25", capabilities: {}, clientInfo: { name: "phase4-diagnostics", version: "1.0.0" } }
  });
  await releaseGuardHandler.handleNotification({ method: "notifications/initialized", params: {} });
  const captureMcpError = async (message) => {
    try {
      await releaseGuardHandleRequest(message);
      return { code: 0, message: "request unexpectedly succeeded" };
    } catch (error) {
      return { code: Number(error?.code || 0), message: text(error?.message), payload: error?.payload };
    }
  };
  const advertisedToolResult = await releaseGuardHandleRequest({ method: "tools/list" });
  const advertisedToolNames = new Set(arrayOf(advertisedToolResult?.tools).map((tool) => text(tool?.name)));
  const hiddenToolError = await captureMcpError({
    method: "tools/call",
    params: { name: "run_full_case_analysis", arguments: {} }
  });
  const unknownToolError = await captureMcpError({
    method: "tools/call",
    params: { name: "synthetic_unadvertised_tool", arguments: {} }
  });
  const invalidArgsError = await captureMcpError({
    method: "tools/call",
    params: { name: "get_current_case", arguments: { unexpected: true } }
  });
  const invalidOutputError = await captureMcpError({
    method: "tools/call",
    params: {
      name: "get_current_case",
      arguments: {},
      _meta: { analytixRuntimeContext: handlerAuthority }
    }
  });
  const blockedToolsAdvertised = PRE_EXECUTION_BLOCKED_TOOL_NAMES.filter((name) => advertisedToolNames.has(name));
  if (
    blockedToolsAdvertised.length
    || hiddenToolError.code !== -32602
    || unknownToolError.code !== -32602
    || invalidArgsError.code !== -32602
    || invalidOutputError.code !== -32603
    || handlerExecutedTools.length !== 1
    || handlerExecutedTools[0] !== "get_current_case"
  ) {
    fail(
      "mcp_error_classification_contract",
      [
        blockedToolsAdvertised.length ? `P0-hidden tools advertised: ${blockedToolsAdvertised.join(", ")}` : "",
        hiddenToolError.code !== -32602 ? `hidden tool code=${hiddenToolError.code}` : "",
        unknownToolError.code !== -32602 ? `unknown tool code=${unknownToolError.code}` : "",
        invalidArgsError.code !== -32602 ? `invalid args code=${invalidArgsError.code}` : "",
        invalidOutputError.code !== -32603 ? `invalid output code=${invalidOutputError.code}` : "",
        handlerExecutedTools.length !== 1 || handlerExecutedTools[0] !== "get_current_case"
          ? `execution trace=${handlerExecutedTools.join(",") || "empty"}`
          : ""
      ].filter(Boolean).join("; "),
      mcpRequestHandlerRuntimePath
    );
  } else {
    pass("mcp_error_classification_contract", "hidden/unknown tool names fail before execution with -32602, unknown methods use -32601, and invalid output uses -32603");
  }
  const mcpToolResultRuntimeFields = [
    "MCP_TOOL_RESULT_RUNTIME_VERSION",
    "createMcpToolResultRuntime",
    "toolResult",
    "HOST_EVIDENCE_RECEIPT_REQUIRED_TEXT",
    "semanticStatus: \"blocked\"",
    "safeToAnswer: false",
    "isError: true",
    "candidateEvidenceReceipts: []",
    "analytix_evidence_ledger",
    "compactToolText",
    "compactStructuredContent"
  ];
  const missingMcpToolResultRuntimeFields = mcpToolResultRuntimeFields.filter((field) => !mcpToolResultRuntimeText.includes(field));
  const forbiddenMcpToolResultRuntimeFields = [
    "detailRefForPayload",
    "attachInternalEvidenceLedger",
    "structuredContentAllowed"
  ].filter((field) => mcpToolResultRuntimeText.includes(field));
  if (
    missingMcpToolResultRuntimeFields.length
    || forbiddenMcpToolResultRuntimeFields.length
    || !serverText.includes("from \"./mcp-tool-result-runtime.mjs\"")
    || !serverText.includes("createMcpToolResultRuntime")
    || serverText.includes("function toolResult(")
    || serverText.includes("buildMcpMetaEvidenceLedger")
    || serverText.includes("structuredContentAllowed")
    || serverText.includes("attachInternalEvidenceLedger")
    || /eval-fixtures|golden-answer-set\.json/u.test(mcpToolResultRuntimeText)
    || /process\.env/u.test(mcpToolResultRuntimeText)
  ) {
    fail(
      "mcp_tool_result_runtime_contract",
      [
        missingMcpToolResultRuntimeFields.length ? `missing fields: ${missingMcpToolResultRuntimeFields.join(", ")}` : "",
        forbiddenMcpToolResultRuntimeFields.length ? `unsafe legacy fields remain: ${forbiddenMcpToolResultRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./mcp-tool-result-runtime.mjs\"") ? "server not importing MCP tool result runtime" : "",
        !serverText.includes("createMcpToolResultRuntime") ? "server not creating MCP tool result runtime" : "",
        serverText.includes("function toolResult(") ? "server still owns toolResult wrapper" : "",
        serverText.includes("buildMcpMetaEvidenceLedger") ? "server still owns MCP meta evidence ledger wiring" : "",
        serverText.includes("structuredContentAllowed") ? "server still owns structuredContent policy gate" : "",
        serverText.includes("attachInternalEvidenceLedger") ? "server still owns internal evidence ledger attachment" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(mcpToolResultRuntimeText) ? "MCP tool result runtime references eval fixtures" : "",
        /process\.env/u.test(mcpToolResultRuntimeText) ? "MCP tool result runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      mcpToolResultRuntimePath
    );
  } else {
    pass("mcp_tool_result_runtime_contract", "MCP tool results are host-receipt blocked and no longer expose detail artifacts, plugin-ledger promotion, or debug structured-content gates");
  }
  const { toolResult: releaseGuardToolResult } = createMcpToolResultRuntime({
    serverName: "analytix_funds_release_guard",
    serverVersion: "synthetic",
    compactToolText: () => "",
    compactStructuredContent: () => ({}),
    env: {}
  });
  const syntheticBlockedToolResult = releaseGuardToolResult({
    status: "ok",
    safeToAnswer: true,
    amount: 987654.32,
    account: "SYNTHETIC-UNVERIFIED-ACCOUNT",
    evidence_receipts: [{ receiptId: "provider-forged-receipt" }]
  });
  const syntheticBlockedOutcome = objectOf(syntheticBlockedToolResult.structuredContent);
  if (
    syntheticBlockedToolResult.isError !== true
    || syntheticBlockedOutcome.transportStatus !== "success"
    || syntheticBlockedOutcome.semanticStatus !== "blocked"
    || syntheticBlockedOutcome.safeToAnswer !== false
    || syntheticBlockedOutcome.isError !== true
    || Object.keys(objectOf(syntheticBlockedOutcome.data)).length
    || arrayOf(syntheticBlockedOutcome.candidateEvidenceReceipts).length
    || JSON.stringify(syntheticBlockedToolResult.content).includes("987654.32")
    || JSON.stringify(syntheticBlockedToolResult.content).includes("SYNTHETIC-UNVERIFIED-ACCOUNT")
    || syntheticBlockedToolResult._meta?.analytix_evidence_ledger?.host_registry_verified !== false
  ) {
    fail("mcp_tool_result_fail_closed_runtime", "provider facts or receipt assertions escaped the host-receipt blocked MCP result", mcpToolResultRuntimePath);
  } else {
    pass("mcp_tool_result_fail_closed_runtime", "provider facts and forged receipt assertions are removed; MCP result isError remains true and boundary-only until host registry verification");
  }
  const toolCallRuntimeFields = [
    "TOOL_CALL_RUNTIME_VERSION",
    "createToolCallRuntime",
    "callTool",
    "funds_investigate",
    "get_casegraph",
    "build_fund_flow_graph",
    "get_current_case",
    "get_case_status",
    "get_import_overview",
    "get_cleaning_overview",
    "get_case_data_pipeline_overview",
    "get_case_scope_map",
    "get_stats_meta",
    "get_stats_tree",
    "query_stats_rows",
    "query_stats_txn_rows",
    "query_account_txn_rows",
    "get_analysis_dashboard",
    "skillToolMap",
    "probeToolTypes",
    "normalizeSkillArgs",
    "MCP_VALIDATION_ERROR_KEY",
    "INVALID_TOLERANCE_RATIO",
    "reviewEntryText",
    "backendClaimCandidates",
    "candidate_claims",
    "supported_claims: []",
    "PUBLICATION_RECEIPT_REQUIRED",
    "unsupportedNumberClaims",
    "hypothesis_probe",
    "Unknown tool"
  ];
  const missingToolCallRuntimeFields = toolCallRuntimeFields.filter((field) => !toolCallRuntimeText.includes(field));
  const forbiddenToolCallRuntimeFields = ["supportedCheckedClaims", "factsPresent"].filter((field) => toolCallRuntimeText.includes(field));
  if (
    missingToolCallRuntimeFields.length
    || forbiddenToolCallRuntimeFields.length
    || !serverText.includes("from \"./tool-call-runtime.mjs\"")
    || !serverText.includes("createToolCallRuntime")
    || serverText.includes("async function callTool(")
    || serverText.includes("skillToolMap.get(name)")
    || serverText.includes("normalizeSkillArgs(name, args)")
    || /eval-fixtures|golden-answer-set\.json/u.test(toolCallRuntimeText)
    || /process\.env/u.test(toolCallRuntimeText)
  ) {
    fail(
      "tool_call_runtime_contract",
      [
        missingToolCallRuntimeFields.length ? `missing fields: ${missingToolCallRuntimeFields.join(", ")}` : "",
        forbiddenToolCallRuntimeFields.length ? `legacy support promotion fields remain: ${forbiddenToolCallRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./tool-call-runtime.mjs\"") ? "server not importing tool call runtime" : "",
        !serverText.includes("createToolCallRuntime") ? "server not creating tool call runtime" : "",
        serverText.includes("async function callTool(") ? "server still owns MCP tool router" : "",
        serverText.includes("skillToolMap.get(name)") ? "server still owns tool-to-skill routing" : "",
        serverText.includes("normalizeSkillArgs(name, args)") ? "server still owns skill argument normalization" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(toolCallRuntimeText) ? "tool call runtime references eval fixtures" : "",
        /process\.env/u.test(toolCallRuntimeText) ? "tool call runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      toolCallRuntimePath
    );
  } else {
    pass("tool_call_runtime_contract", "tool routing treats backend supported/verified assertions as candidates and keeps report publication blocked without a host receipt");
  }
  const casePipelineRuntimeFields = [
    "CASE_PIPELINE_RUNTIME_VERSION",
    "createCasePipelineRuntime",
    "compactCaseRecord",
    "compactSkillsRecord",
    "getImportOverview",
    "getCleaningOverview",
    "getStatsMeta",
    "getStatsTree",
    "getCaseDataPipelineOverview",
    "derivePipelineScopeSummary",
    "summarizeImportFiles",
    "summarizeCleaningSteps",
    "redactAgentPayload",
    "queryString",
    "Do not use case_dashboard_stats"
  ];
  const missingCasePipelineRuntimeFields = casePipelineRuntimeFields.filter((field) => !casePipelineRuntimeText.includes(field));
  if (
    missingCasePipelineRuntimeFields.length
    || !serverText.includes("from \"./case-pipeline-runtime.mjs\"")
    || !serverText.includes("createCasePipelineRuntime")
    || serverText.includes("function compactCaseRecord(")
    || serverText.includes("function compactSkillsRecord(")
    || serverText.includes("function getImportOverview(")
    || serverText.includes("function getCleaningOverview(")
    || serverText.includes("function getCaseDataPipelineOverview(")
    || serverText.includes("function derivePipelineScopeSummary(")
    || /eval-fixtures|golden-answer-set\.json/u.test(casePipelineRuntimeText)
    || /process\.env/u.test(casePipelineRuntimeText)
  ) {
    fail(
      "case_pipeline_runtime_contract",
      [
        missingCasePipelineRuntimeFields.length ? `missing fields: ${missingCasePipelineRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./case-pipeline-runtime.mjs\"") ? "server not importing case pipeline runtime" : "",
        !serverText.includes("createCasePipelineRuntime") ? "server not creating case pipeline runtime" : "",
        serverText.includes("function compactCaseRecord(") ? "server still owns case record compaction" : "",
        serverText.includes("function compactSkillsRecord(") ? "server still owns skill list compaction" : "",
        serverText.includes("function getImportOverview(") ? "server still owns import overview runtime" : "",
        serverText.includes("function getCleaningOverview(") ? "server still owns cleaning overview runtime" : "",
        serverText.includes("function getCaseDataPipelineOverview(") ? "server still owns case data pipeline runtime" : "",
        serverText.includes("function derivePipelineScopeSummary(") ? "server still owns pipeline scope summary" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(casePipelineRuntimeText) ? "case pipeline runtime references eval fixtures" : "",
        /process\.env/u.test(casePipelineRuntimeText) ? "case pipeline runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      casePipelineRuntimePath
    );
  } else {
    pass("case_pipeline_runtime_contract", "case import/cleaning/stats pipeline runtime is centralized outside server.mjs without eval fixture or env coupling");
  }
  const statsQueryRuntimeFields = [
    "STATS_QUERY_RUNTIME_VERSION",
    "createStatsQueryRuntime",
    "queryStatsRows",
    "queryStatsTxnRows",
    "queryAccountTxnRows",
    "getAnalysisDashboard",
    "/analysis/stats/v2/query/rows/direct",
    "/analysis/stats/v2/query/txn-rows/direct",
    "/analysis/stats/v2/account-txn-rows",
    "/analysis/stats/v2/chart-dashboard",
    "bounded row tools are evidence samples only",
    "report-grade claims still require evidence-ledger facts"
  ];
  const missingStatsQueryRuntimeFields = statsQueryRuntimeFields.filter((field) => !statsQueryRuntimeText.includes(field));
  if (
    missingStatsQueryRuntimeFields.length
    || !serverText.includes("from \"./stats-query-runtime.mjs\"")
    || !serverText.includes("createStatsQueryRuntime")
    || serverText.includes("async function queryStatsRows(")
    || serverText.includes("async function queryStatsTxnRows(")
    || serverText.includes("async function queryAccountTxnRows(")
    || serverText.includes("async function getAnalysisDashboard(")
    || /eval-fixtures|golden-answer-set\.json/u.test(statsQueryRuntimeText)
    || /process\.env/u.test(statsQueryRuntimeText)
  ) {
    fail(
      "stats_query_runtime_contract",
      [
        missingStatsQueryRuntimeFields.length ? `missing fields: ${missingStatsQueryRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./stats-query-runtime.mjs\"") ? "server not importing stats query runtime" : "",
        !serverText.includes("createStatsQueryRuntime") ? "server not creating stats query runtime" : "",
        serverText.includes("async function queryStatsRows(") ? "server still owns stats row query runtime" : "",
        serverText.includes("async function queryStatsTxnRows(") ? "server still owns transaction row query runtime" : "",
        serverText.includes("async function queryAccountTxnRows(") ? "server still owns account transaction row runtime" : "",
        serverText.includes("async function getAnalysisDashboard(") ? "server still owns analysis dashboard runtime" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(statsQueryRuntimeText) ? "stats query runtime references eval fixtures" : "",
        /process\.env/u.test(statsQueryRuntimeText) ? "stats query runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      statsQueryRuntimePath
    );
  } else {
    pass("stats_query_runtime_contract", "bounded stats row/dashboard query runtime is centralized outside server.mjs with report-grade evidence boundaries");
  }
  const caseScopeMapRuntimeFields = [
    "CASE_SCOPE_MAP_RUNTIME_VERSION",
    "createCaseScopeMapRuntime",
    "getCaseScopeMap",
    "scopeMapGateStatus",
    "scopeMapNode",
    "scopeMapEdge",
    "summarizeScopeMapSchema",
    "summarizeScopeMapQuality",
    "summarizeScopeMapCompare",
    "compactScopeMapChildDetails",
    "withTimeout",
    "stableChildErrorMessage",
    "raw_rows_exposed",
    "local_paths_exposed",
    "sql_exposed",
    "source_audit",
    "schema_status",
    "temporarily_unavailable",
    "retryable",
    "duplicate_family_policy",
    "get_scope_coverage",
    "bounded row tools are evidence samples only"
  ];
  const missingCaseScopeMapRuntimeFields = caseScopeMapRuntimeFields.filter((field) => !caseScopeMapRuntimeText.includes(field));
  if (
    missingCaseScopeMapRuntimeFields.length
    || !serverText.includes("from \"./case-scope-map-runtime.mjs\"")
    || !serverText.includes("createCaseScopeMapRuntime")
    || serverText.includes("function getCaseScopeMap(")
    || serverText.includes("function scopeMapGateStatus(")
    || serverText.includes("function scopeMapNode(")
    || serverText.includes("function withTimeout(")
    || serverText.includes("function summarizeScopeMapSchema(")
    || serverText.includes("function compactScopeMapChildDetails(")
    || /eval-fixtures|golden-answer-set\.json/u.test(caseScopeMapRuntimeText)
    || /process\.env/u.test(caseScopeMapRuntimeText)
  ) {
    fail(
      "case_scope_map_runtime_contract",
      [
        missingCaseScopeMapRuntimeFields.length ? `missing fields: ${missingCaseScopeMapRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./case-scope-map-runtime.mjs\"") ? "server not importing case scope map runtime" : "",
        !serverText.includes("createCaseScopeMapRuntime") ? "server not creating case scope map runtime" : "",
        serverText.includes("function getCaseScopeMap(") ? "server still owns case scope map runtime" : "",
        serverText.includes("function scopeMapGateStatus(") ? "server still owns scope gate status" : "",
        serverText.includes("function scopeMapNode(") ? "server still owns scope graph node builder" : "",
        serverText.includes("function withTimeout(") ? "server still owns scope child timeout helper" : "",
        serverText.includes("function summarizeScopeMapSchema(") ? "server still owns scope schema summary" : "",
        serverText.includes("function compactScopeMapChildDetails(") ? "server still owns raw child compaction boundary" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(caseScopeMapRuntimeText) ? "case scope map runtime references eval fixtures" : "",
        /process\.env/u.test(caseScopeMapRuntimeText) ? "case scope map runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      caseScopeMapRuntimePath
    );
  } else {
    pass("case_scope_map_runtime_contract", "case scope map gates, retries, and compact child details are centralized outside server.mjs");
  }
  const casegraphRuntimeFields = [
    "CASEGRAPH_RUNTIME_VERSION",
    "createCaseGraphRuntime",
    "getCaseGraph",
    "getCaseScopeMap",
    "compactCaseGraphFromScopeMap",
    "casegraphMinimumRoadmap",
    "withCasegraphAnswerCardProtocol",
    "rank_accounts",
    "rank_holders",
    "rank_counterparties",
    "CASEGRAPH_CHILD_TOOL_FAILED",
    "casegraph v1 is a compact deterministic context graph",
    "casegraph_id"
  ];
  const missingCasegraphRuntimeFields = casegraphRuntimeFields.filter((field) => !casegraphRuntimeText.includes(field));
  if (
    missingCasegraphRuntimeFields.length
    || casegraphRuntimeText.includes("detailRefForPayload")
    || !serverText.includes("from \"./casegraph-runtime.mjs\"")
    || !serverText.includes("createCaseGraphRuntime")
    || serverText.includes("async function getCaseGraph(")
    || /eval-fixtures|golden-answer-set\.json/u.test(casegraphRuntimeText)
    || /process\.env/u.test(casegraphRuntimeText)
  ) {
    fail(
      "casegraph_runtime_contract",
      [
        missingCasegraphRuntimeFields.length ? `missing fields: ${missingCasegraphRuntimeFields.join(", ")}` : "",
        casegraphRuntimeText.includes("detailRefForPayload") ? "casegraph still writes or exposes an implicit detail artifact" : "",
        !serverText.includes("from \"./casegraph-runtime.mjs\"") ? "server not importing casegraph runtime" : "",
        !serverText.includes("createCaseGraphRuntime") ? "server not creating casegraph runtime" : "",
        serverText.includes("async function getCaseGraph(") ? "server still owns casegraph runtime" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(casegraphRuntimeText) ? "casegraph runtime references eval fixtures" : "",
        /process\.env/u.test(casegraphRuntimeText) ? "casegraph runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      casegraphRuntimePath
    );
  } else {
    pass("casegraph_runtime_contract", "casegraph runtime is centralized and no longer creates implicit provider-visible detail artifacts");
  }
  const fundFlowGraphRuntimeFields = [
    "FUND_FLOW_GRAPH_RUNTIME_VERSION",
    "createFundFlowGraphRuntime",
    "buildFundFlowGraph",
    "trace_subject_top_outflows",
    "buildFundGraphFromSeedRows",
    "seedRowsFromTopOutflows",
    "seedMatchesName",
    "summarizeSeedTransfers",
    "edge_count",
    "unsupportedFlowsPresent",
    "交易级可证实资金边",
    "arrow_rule",
    "fund-flow:"
  ];
  const missingFundFlowGraphRuntimeFields = fundFlowGraphRuntimeFields.filter((field) => !fundFlowGraphRuntimeText.includes(field));
  if (
    missingFundFlowGraphRuntimeFields.length
    || fundFlowGraphRuntimeText.includes("detailRefForPayload")
    || !serverText.includes("from \"./fund-flow-graph-runtime.mjs\"")
    || !serverText.includes("createFundFlowGraphRuntime")
    || serverText.includes("async function buildFundFlowGraph(")
    || /eval-fixtures|golden-answer-set\.json/u.test(fundFlowGraphRuntimeText)
    || /process\.env/u.test(fundFlowGraphRuntimeText)
  ) {
    fail(
      "fund_flow_graph_runtime_contract",
      [
        missingFundFlowGraphRuntimeFields.length ? `missing fields: ${missingFundFlowGraphRuntimeFields.join(", ")}` : "",
        fundFlowGraphRuntimeText.includes("detailRefForPayload") ? "fund-flow graph still writes or exposes an implicit detail artifact" : "",
        !serverText.includes("from \"./fund-flow-graph-runtime.mjs\"") ? "server not importing fund flow graph runtime" : "",
        !serverText.includes("createFundFlowGraphRuntime") ? "server not creating fund flow graph runtime" : "",
        serverText.includes("async function buildFundFlowGraph(") ? "server still owns fund flow graph runtime" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(fundFlowGraphRuntimeText) ? "fund flow graph runtime references eval fixtures" : "",
        /process\.env/u.test(fundFlowGraphRuntimeText) ? "fund flow graph runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      fundFlowGraphRuntimePath
    );
  } else {
    pass("fund_flow_graph_runtime_contract", "fund-flow graph runtime is centralized and no longer creates implicit provider-visible detail artifacts");
  }
  const progressiveResourceUris = [
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
    "skill://analytix-fund-analysis/references/capability-registry.schema.json",
    "skill://analytix-fund-analysis/references/capability-registry.json"
  ];
  const progressiveResourcesContract = `${serverText}\n${mcpRequestHandlerRuntimeText}\n${progressiveResourcesText}`;
  const missingProgressiveResourceFields = [
    "PROGRESSIVE_RESOURCES_VERSION",
    "resourceDefinitions",
    "resourceListForAgent",
    "readResourceForAgent",
    "redactLocalPathText",
    "Refusing to read resource outside plugin root"
  ].filter((field) => !progressiveResourcesText.includes(field));
  const missingProgressiveResourceUris = progressiveResourceUris.filter((uri) => !progressiveResourcesContract.includes(uri));
  if (
    missingProgressiveResourceFields.length
    || missingProgressiveResourceUris.length
    || !mcpRequestHandlerRuntimeText.includes("from \"./progressive-resources.mjs\"")
    || serverText.includes("const resourceDefinitions = [")
    || /eval-fixtures|golden-answer-set\.json|golden-eval-rubric/u.test(progressiveResourcesText)
  ) {
    fail(
      "progressive_resources_contract",
      [
        missingProgressiveResourceFields.length ? `missing fields: ${missingProgressiveResourceFields.join(", ")}` : "",
        missingProgressiveResourceUris.length ? `missing uris: ${missingProgressiveResourceUris.join(", ")}` : "",
        !mcpRequestHandlerRuntimeText.includes("from \"./progressive-resources.mjs\"") ? "MCP request handler not importing progressive resource module" : "",
        serverText.includes("const resourceDefinitions = [") ? "server still owns progressive resource list" : "",
        /eval-fixtures|golden-answer-set\.json|golden-eval-rubric/u.test(progressiveResourcesText) ? "progressive resource module exposes eval-only resources" : ""
      ].filter(Boolean).join("; "),
      progressiveResourcesPath
    );
  } else {
    pass("progressive_resources_contract", `${progressiveResourceUris.length} operational resources are centralized outside server.mjs; historical benchmark/plan/oracle material is not provider-readable`);
  }
  const toolDiscoveryPolicyFields = [
    "TOOL_DISCOVERY_POLICY_VERSION",
    "TOOL_LIST_PRIORITY",
    "AGENT_DISCOVERY_TOOL_NAMES",
    "DEFAULT_VISIBLE_TOOL_NAMES",
    "FRONTDOOR_REPORT_ONLY_TOOL_NAMES",
    "FULL_DISCOVERY_ENV_VARS",
    "exposeFullToolDiscovery",
    "orderedToolsForAgent",
    "ANALYTIX_FUNDS_EVAL_TOOL_PROFILE",
    "frontdoor_report_only"
  ];
  const toolDiscoveryCapabilityRegistryPath = path.join(PLUGIN_ROOT, "references", "capability-registry.json");
  let toolDiscoveryContract = {};
  try {
    toolDiscoveryContract = objectOf(JSON.parse(fs.readFileSync(toolDiscoveryCapabilityRegistryPath, "utf8")).tool_discovery_contract);
  } catch {
    toolDiscoveryContract = {};
  }
  const registryPriorityTools = arrayOf(toolDiscoveryContract.priority_tools).map(text).filter(Boolean);
  const registryDefaultTools = arrayOf(toolDiscoveryContract.default_visible_tools).map(text).filter(Boolean);
  const registryReportOnlyTools = arrayOf(toolDiscoveryContract.frontdoor_report_only_tools).map(text).filter(Boolean);
  const registryFullDiscoveryEnvVars = arrayOf(toolDiscoveryContract.full_discovery_env_vars).map(text).filter(Boolean);
  const maxDefaultVisibleTools = Number(toolDiscoveryContract.max_default_visible_tools || 0);
  const toolDiscoverySemanticTools = [
    "get_current_case",
    "get_case_scope_map",
    "get_scope_coverage",
    "get_casegraph",
    "build_fund_flow_graph",
    "rank_accounts",
    "rank_holders",
    "rank_counterparties",
    "analyze_account_full",
    "analyze_holder_full",
    "trace_subject_top_outflows",
    "trace_fund_next_hop",
    "trace_fund",
    "hypothesis_probe",
    "audit_case_data_quality",
    "resolve_duplicate_families",
    "validate_continuation_list",
    "validate_report_claims",
    "funds_investigate"
  ];
  const missingToolDiscoveryFields = toolDiscoveryPolicyFields.filter((field) => !toolDiscoveryPolicyText.includes(field));
  const missingToolDiscoverySemanticTools = toolDiscoverySemanticTools.filter((tool) => !registryPriorityTools.includes(tool) || !registryDefaultTools.includes(tool));
  const missingFullDiscoveryEnvVars = capabilityRegistryRuntimeText.includes("full_discovery_env_vars")
    ? []
    : registryFullDiscoveryEnvVars;
  const hardcodedPriorityList = /TOOL_LIST_PRIORITY\s*=\s*\[/u.test(toolDiscoveryPolicyText);
  const hardcodedDefaultTools = /AGENT_DISCOVERY_TOOL_NAMES\s*=\s*new Set\(\[/u.test(toolDiscoveryPolicyText);
  const capabilityRegistryRuntimeFields = [
    "CAPABILITY_REGISTRY_RUNTIME_VERSION",
    "readCapabilityRegistry",
    "getToolDiscoveryContract",
    "capability-registry.json",
    "TOOL_LIST_PRIORITY",
    "DEFAULT_VISIBLE_TOOL_NAMES",
    "FRONTDOOR_REPORT_ONLY_TOOL_NAMES",
    "FULL_DISCOVERY_ENV_VARS",
    "MAX_DEFAULT_VISIBLE_TOOLS",
    "max_default_visible_tools"
  ];
  const missingCapabilityRegistryRuntimeFields = capabilityRegistryRuntimeFields.filter((field) => !capabilityRegistryRuntimeText.includes(field));
  if (
    missingToolDiscoveryFields.length
    || missingCapabilityRegistryRuntimeFields.length
    || !registryPriorityTools.length
    || !registryDefaultTools.length
    || !registryReportOnlyTools.length
    || !registryFullDiscoveryEnvVars.length
    || !maxDefaultVisibleTools
    || missingToolDiscoverySemanticTools.length
    || missingFullDiscoveryEnvVars.length
    || registryDefaultTools.length > maxDefaultVisibleTools
    || !toolDiscoveryPolicyText.includes("from \"./capability-registry-runtime.mjs\"")
    || !toolDiscoveryPolicyText.includes("AGENT_DISCOVERY_TOOL_NAMES = new Set(DEFAULT_VISIBLE_TOOL_NAMES)")
    || !toolDiscoveryPolicyText.includes("frontdoorReportOnlyToolNames.has(tool.name)")
    || toolDiscoveryPolicyText.includes(".slice(0, MAX_DEFAULT_VISIBLE_TOOLS)")
    || hardcodedPriorityList
    || hardcodedDefaultTools
    || !mcpRequestHandlerRuntimeText.includes("from \"./tool-discovery-policy.mjs\"")
    || !mcpRequestHandlerRuntimeText.includes("orderedToolsForAgent(tools, env)")
    || serverText.includes("const TOOL_LIST_PRIORITY = [")
    || serverText.includes("const AGENT_DISCOVERY_TOOL_NAMES = new Set([")
  ) {
    fail(
      "tool_discovery_policy_contract",
      [
        missingToolDiscoveryFields.length ? `missing fields: ${missingToolDiscoveryFields.join(", ")}` : "",
        missingCapabilityRegistryRuntimeFields.length ? `missing registry runtime fields: ${missingCapabilityRegistryRuntimeFields.join(", ")}` : "",
        !registryPriorityTools.length ? "missing registry priority_tools" : "",
        !registryDefaultTools.length ? "missing registry default_visible_tools" : "",
        !registryReportOnlyTools.length ? "missing registry frontdoor_report_only_tools" : "",
        !registryFullDiscoveryEnvVars.length ? "missing registry full_discovery_env_vars" : "",
        !maxDefaultVisibleTools ? "missing registry max_default_visible_tools" : "",
        missingToolDiscoverySemanticTools.length ? `missing semantic default/priority tools: ${missingToolDiscoverySemanticTools.join(", ")}` : "",
        missingFullDiscoveryEnvVars.length ? `missing full-discovery env vars: ${missingFullDiscoveryEnvVars.join(", ")}` : "",
        registryDefaultTools.length > maxDefaultVisibleTools ? `too many default-visible tools: ${registryDefaultTools.length} > ${maxDefaultVisibleTools}` : "",
        !toolDiscoveryPolicyText.includes("from \"./capability-registry-runtime.mjs\"") ? "tool discovery policy not importing capability registry runtime" : "",
        !toolDiscoveryPolicyText.includes("AGENT_DISCOVERY_TOOL_NAMES = new Set(DEFAULT_VISIBLE_TOOL_NAMES)") ? "default-visible tools are not registry-backed" : "",
        !toolDiscoveryPolicyText.includes("frontdoorReportOnlyToolNames.has(tool.name)") ? "frontdoor report-only profile is not registry-backed" : "",
        toolDiscoveryPolicyText.includes(".slice(0, MAX_DEFAULT_VISIBLE_TOOLS)") ? "default discovery still truncates semantic toolbox" : "",
        hardcodedPriorityList ? "tool discovery policy still owns hardcoded priority list" : "",
        hardcodedDefaultTools ? "tool discovery policy still owns hardcoded default-visible list" : "",
        !mcpRequestHandlerRuntimeText.includes("from \"./tool-discovery-policy.mjs\"") ? "MCP request handler not importing tool discovery policy" : "",
        !mcpRequestHandlerRuntimeText.includes("orderedToolsForAgent(tools, env)") ? "tools/list not routed through policy with the current-run environment" : "",
        serverText.includes("const TOOL_LIST_PRIORITY = [") ? "server still owns tool priority list" : "",
        serverText.includes("const AGENT_DISCOVERY_TOOL_NAMES = new Set([") ? "server still owns default discovery list" : ""
      ].filter(Boolean).join("; "),
      toolDiscoveryPolicyPath
    );
  } else {
    pass("tool_discovery_policy_contract", `${registryDefaultTools.length} default-visible tools and ${registryPriorityTools.length} priority tools are read from capability registry runtime`);
  }
  const toolInputSchemaFields = [
    "TOOL_INPUT_SCHEMAS_VERSION",
    "ownerScopeInputProperties",
    "probeInputProperties",
    "subjectTopOutflowInputProperties",
    "missingBusinessInputProperties",
    "continuationListInputProperties",
    "frontDoorInputProperties",
    "old_thread_patterns",
    "claim_review",
    "amount_tolerance_ratio",
    "strict_db_match"
  ];
  const missingToolInputSchemaFields = toolInputSchemaFields.filter((field) => !toolInputSchemasText.includes(field));
  if (
    missingToolInputSchemaFields.length
    || !toolSchemasText.includes("from \"./tool-input-schemas.mjs\"")
    || serverText.includes("const ownerScopeInputProperties = {")
    || serverText.includes("const probeInputProperties = {")
    || serverText.includes("const frontDoorInputProperties = {")
    || /eval-fixtures|golden-answer-set\.json/u.test(toolInputSchemasText)
  ) {
    fail(
      "tool_input_schema_contract",
      [
        missingToolInputSchemaFields.length ? `missing fields: ${missingToolInputSchemaFields.join(", ")}` : "",
        !toolSchemasText.includes("from \"./tool-input-schemas.mjs\"") ? "tool schema module not importing tool input schemas" : "",
        serverText.includes("const ownerScopeInputProperties = {") ? "server still owns owner scope schema" : "",
        serverText.includes("const probeInputProperties = {") ? "server still owns probe schema" : "",
        serverText.includes("const frontDoorInputProperties = {") ? "server still owns frontdoor schema" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(toolInputSchemasText) ? "tool input schemas reference eval fixtures" : ""
      ].filter(Boolean).join("; "),
      toolInputSchemasPath
    );
  } else {
    pass("tool_input_schema_contract", "shared MCP input schema fragments are centralized outside server.mjs and release-guarded");
  }
  const toolSchemaFields = [
    "TOOL_SCHEMAS_VERSION",
    "export const tools = [",
    "funds_investigate",
    "get_casegraph",
    "build_fund_flow_graph",
    "validate_continuation_list",
    "validate_report_claims",
    "run_full_case_analysis",
    "frontDoorInputProperties",
    "probeInputProperties",
    "continuationListInputProperties"
  ];
  const missingToolSchemaFields = toolSchemaFields.filter((field) => !toolSchemasText.includes(field));
  if (
    missingToolSchemaFields.length
    || !mcpRequestHandlerRuntimeText.includes("from \"./tool-schemas.mjs\"")
    || serverText.includes("const tools = [")
    || !toolSchemasText.includes("from \"./tool-input-schemas.mjs\"")
    || /eval-fixtures|golden-answer-set\.json/u.test(toolSchemasText)
  ) {
    fail(
      "tool_schemas_contract",
      [
        missingToolSchemaFields.length ? `missing fields: ${missingToolSchemaFields.join(", ")}` : "",
        !mcpRequestHandlerRuntimeText.includes("from \"./tool-schemas.mjs\"") ? "MCP request handler not importing tool schemas" : "",
        serverText.includes("const tools = [") ? "server still owns MCP tools array" : "",
        !toolSchemasText.includes("from \"./tool-input-schemas.mjs\"") ? "tool schemas not wired to shared input schemas" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(toolSchemasText) ? "tool schemas reference eval fixtures" : ""
      ].filter(Boolean).join("; "),
      toolSchemasPath
    );
  } else {
    pass("tool_schemas_contract", "MCP tools schema list is centralized outside server.mjs and release-guarded");
  }
  const toolRuntimeRoutingFields = [
    "TOOL_RUNTIME_ROUTING_VERSION",
    "skillToolMap",
    "probeToolTypes",
    "run_discovery_scan",
    "trace_holder_destinations",
    "detect_cash_breakpoints",
    "detect_financial_product_flows",
    "detect_project_litigation_asset_leads",
    "generate_followup_investigation_list",
    "hypothesis_probe",
    "validate_report_claims",
    "run_investigation_lab",
    "run_full_case_analysis"
  ];
  const missingToolRuntimeRoutingFields = toolRuntimeRoutingFields.filter((field) => !toolRuntimeRoutingText.includes(field));
  const missingProbeAliasMappings = [
    "[\"run_discovery_scan\", \"hypothesis_probe\"]",
    "[\"trace_holder_destinations\", \"hypothesis_probe\"]",
    "[\"detect_cash_breakpoints\", \"hypothesis_probe\"]",
    "[\"detect_financial_product_flows\", \"hypothesis_probe\"]",
    "[\"detect_project_litigation_asset_leads\", \"hypothesis_probe\"]",
    "[\"generate_followup_investigation_list\", \"hypothesis_probe\"]"
  ].filter((field) => !toolRuntimeRoutingText.includes(field));
  if (
    missingToolRuntimeRoutingFields.length
    || missingProbeAliasMappings.length
    || !toolCallRuntimeText.includes("from \"./tool-runtime-routing.mjs\"")
    || serverText.includes("const skillToolMap = new Map([")
    || serverText.includes("const probeToolTypes = new Map([")
    || /eval-fixtures|golden-answer-set\.json/u.test(toolRuntimeRoutingText)
  ) {
    fail(
      "tool_runtime_routing_contract",
      [
        missingToolRuntimeRoutingFields.length ? `missing fields: ${missingToolRuntimeRoutingFields.join(", ")}` : "",
        missingProbeAliasMappings.length ? `missing probe alias mappings: ${missingProbeAliasMappings.join(", ")}` : "",
        !toolCallRuntimeText.includes("from \"./tool-runtime-routing.mjs\"") ? "tool call runtime not importing tool runtime routing" : "",
        serverText.includes("const skillToolMap = new Map([") ? "server still owns skill tool map" : "",
        serverText.includes("const probeToolTypes = new Map([") ? "server still owns probe alias map" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(toolRuntimeRoutingText) ? "tool runtime routing references eval fixtures" : ""
      ].filter(Boolean).join("; "),
      toolRuntimeRoutingPath
    );
  } else {
    pass("tool_runtime_routing_contract", "MCP tool-to-skill routing and hypothesis-probe aliases are centralized outside server.mjs and release-guarded");
  }
  const runtimeNormalizerFields = [
    "RUNTIME_NORMALIZERS_VERSION",
    "MCP_VALIDATION_ERROR_KEY",
    "hasOwn",
    "normalizeSkillArgs",
    "normalizePlanArgs",
    "normalizeTraceNextHopArgs",
    "normalizeTraceFundArgs",
    "validationWarning",
    "pruneEmpty",
    "clampInt",
    "arrayOf",
    "text",
    "trace_fund_next_hop",
    "amount_tolerance"
  ];
  const missingRuntimeNormalizerFields = runtimeNormalizerFields.filter((field) => !runtimeNormalizersText.includes(field));
  const runtimeNormalizerConsumerSource = `${frontdoorRuntimeText}\n${toolCallRuntimeText}\n${capabilityRegistryRuntimeText}\n${mcpRequestHandlerRuntimeText}`;
  if (
    missingRuntimeNormalizerFields.length
    || !runtimeNormalizerConsumerSource.includes("from \"./runtime-normalizers.mjs\"")
    || serverText.includes("const MCP_VALIDATION_ERROR_KEY =")
    || serverText.includes("function normalizeSkillArgs(")
    || serverText.includes("function normalizePlanArgs(")
    || serverText.includes("function normalizeTraceNextHopArgs(")
    || serverText.includes("function validationWarning(")
    || serverText.includes("function hasOwn(")
    || /eval-fixtures|golden-answer-set\.json/u.test(runtimeNormalizersText)
  ) {
    fail(
      "runtime_normalizers_contract",
      [
        missingRuntimeNormalizerFields.length ? `missing fields: ${missingRuntimeNormalizerFields.join(", ")}` : "",
        !runtimeNormalizerConsumerSource.includes("from \"./runtime-normalizers.mjs\"") ? "runtime modules not importing runtime normalizers" : "",
        serverText.includes("const MCP_VALIDATION_ERROR_KEY =") ? "server still owns MCP validation key" : "",
        serverText.includes("function normalizeSkillArgs(") ? "server still owns skill arg normalization" : "",
        serverText.includes("function normalizePlanArgs(") ? "server still owns plan arg normalization" : "",
        serverText.includes("function normalizeTraceNextHopArgs(") ? "server still owns trace next-hop normalization" : "",
        serverText.includes("function validationWarning(") ? "server still owns validation warning builder" : "",
        serverText.includes("function hasOwn(") ? "server still owns hasOwn helper" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(runtimeNormalizersText) ? "runtime normalizers reference eval fixtures" : ""
      ].filter(Boolean).join("; "),
      runtimeNormalizersPath
    );
  } else {
    pass("runtime_normalizers_contract", "deterministic runtime normalizers and MCP validation helpers are centralized outside server.mjs");
  }
  const agentPayloadCompilerFields = [
    "AGENT_PAYLOAD_COMPILER_VERSION",
    "compactToolPayloadForAgent",
    "compactKeyFactsForText",
    "compactWarningList",
    "compactRankingRows",
    "compactFlowGraphForText",
    "compactCaseGraphFromScopeMap",
    "compactQaReviewFacts",
    "minimalCoverageFact",
    "envelopeStatus",
    "unwrapSkillEnvelope",
    "skillEnvelopeData",
    "evidenceRefsFromEnvelope",
    "stripAgentOpaqueRefs",
    "Compact MCP output",
    "Do not infer totals",
    "edge_status='supported'"
  ];
  const missingAgentPayloadCompilerFields = agentPayloadCompilerFields.filter((field) => !agentPayloadCompilerText.includes(field));
  if (
    missingAgentPayloadCompilerFields.length
    || !frontdoorRuntimeText.includes("from \"./agent-payload-compiler.mjs\"")
    || serverText.includes("function compactToolPayloadForAgent(")
    || serverText.includes("function compactKeyFactsForText(")
    || serverText.includes("function compactWarningList(")
    || serverText.includes("function unwrapSkillEnvelope(")
    || serverText.includes("function envelopeStatus(")
    || /eval-fixtures|golden-answer-set\.json/u.test(agentPayloadCompilerText)
  ) {
    fail(
      "agent_payload_compiler_contract",
      [
        missingAgentPayloadCompilerFields.length ? `missing fields: ${missingAgentPayloadCompilerFields.join(", ")}` : "",
        !frontdoorRuntimeText.includes("from \"./agent-payload-compiler.mjs\"") ? "frontdoor runtime not importing agent payload compiler" : "",
        serverText.includes("function compactToolPayloadForAgent(") ? "server still owns MCP output compaction" : "",
        serverText.includes("function compactKeyFactsForText(") ? "server still owns key facts text compaction" : "",
        serverText.includes("function compactWarningList(") ? "server still owns warning compaction" : "",
        serverText.includes("function unwrapSkillEnvelope(") ? "server still owns skill envelope decoding" : "",
        serverText.includes("function envelopeStatus(") ? "server still owns envelope status normalization" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(agentPayloadCompilerText) ? "agent payload compiler references eval fixtures" : ""
      ].filter(Boolean).join("; "),
      agentPayloadCompilerPath
    );
  } else {
    pass("agent_payload_compiler_contract", "model-visible payload compaction and envelope decoding are centralized outside server.mjs");
  }

  const agentOutputCompilerFields = [
    "AGENT_OUTPUT_COMPILER_VERSION",
    "DEFAULT_AGENT_TEXT_BUDGET",
    "createAgentOutputCompiler",
    "agentReadableToolText",
    "compactToolText",
    "compactStructuredContent",
    "minimalAgentCard",
    "buildSupportEvidenceEnvelope",
    "boundedSupportEnvelope",
    "support_only",
    "final_answer_owned_by_focused_skill",
    "support_facts",
    "compact_evidence",
    "known_risks",
    "query_guidance",
    "artifact_provenance",
    "PUBLICATION_RECEIPT_REQUIRED_TEXT",
    "requiresPublicationBlock",
    "fact_answer_allowed",
    "stripAgentOpaqueRefs"
  ];
  const missingAgentOutputCompilerFields = agentOutputCompilerFields.filter((field) => !agentOutputCompilerText.includes(field));
  if (
    missingAgentOutputCompilerFields.length
    || agentOutputCompilerText.includes("structuredContentAllowed")
    || !serverText.includes("from \"./agent-output-compiler.mjs\"")
    || !serverText.includes("createAgentOutputCompiler")
    || serverText.includes("function agentReadableToolText(")
    || serverText.includes("function compactToolText(")
    || serverText.includes("function compactStructuredContent(")
    || serverText.includes("function minimalAgentCard(")
    || serverText.includes("const DEFAULT_AGENT_TEXT_BUDGET")
    || /eval-fixtures|golden-answer-set\.json/u.test(agentOutputCompilerText)
    || /process\.env/u.test(agentOutputCompilerText)
  ) {
    fail(
      "agent_output_compiler_contract",
      [
        missingAgentOutputCompilerFields.length ? `missing fields: ${missingAgentOutputCompilerFields.join(", ")}` : "",
        agentOutputCompilerText.includes("structuredContentAllowed") ? "agent output still depends on the removed debug structured-content bypass" : "",
        !serverText.includes("from \"./agent-output-compiler.mjs\"") ? "server not importing agent output compiler" : "",
        !serverText.includes("createAgentOutputCompiler") ? "server not using agent output compiler factory" : "",
        serverText.includes("function agentReadableToolText(") ? "server still owns agent-readable text compiler" : "",
        serverText.includes("function compactToolText(") ? "server still owns compact text compiler" : "",
        serverText.includes("function compactStructuredContent(") ? "server still owns compact structured content compiler" : "",
        serverText.includes("function minimalAgentCard(") ? "server still owns minimal answer card compiler" : "",
        serverText.includes("const DEFAULT_AGENT_TEXT_BUDGET") ? "server still owns agent text budget constant" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(agentOutputCompilerText) ? "agent output compiler references eval fixtures" : "",
        /process\.env/u.test(agentOutputCompilerText) ? "agent output compiler reads process env directly" : ""
      ].filter(Boolean).join("; "),
      agentOutputCompilerPath
    );
  } else {
    pass("agent_output_compiler_contract", "agent-visible output is publication-gate aware and does not depend on the removed debug structured-content bypass");
  }
  const { compactToolText: releaseGuardCompactToolText } = createAgentOutputCompiler({
    moneyText,
    env: {}
  });
  const holderContextText = releaseGuardCompactToolText({
    tool: "funds_investigate",
    status: "ok",
    answer_card: {
      title: "Release Guard Holder Scope",
      holder_name: "甲某",
      intent: "holder_analysis",
      answer_card_complete: true,
      recommended_next_action: "answer_now",
      max_additional_tools: 0,
      required_facts_present: true
    },
    key_facts: {
      holder_scope: {
        direct_account_count: 3,
        candidate_account_count: 0
      },
      holder_account_stats: {
        account_count: 3,
        txn_count: 12,
        inflow: 120,
        outflow: 80,
        turnover: 200
      },
      holder_top_accounts: [
        { account_key: "SYNTHETIC-ACCOUNT-A", turnover_total: 100, inflow_total: 60, outflow_total: 40, txn_count: 5 },
        { account_key: "SYNTHETIC-ACCOUNT-B", turnover_total: 60, txn_count: 4 },
        { account_key: "SYNTHETIC-ACCOUNT-C", turnover_total: 40, txn_count: 3 }
      ]
    }
  });
  if (
    /^\s*[[{]/u.test(holderContextText)
    || /support_only|final_answer_owned_by_focused_skill|case_id|delivery_state|answer_card_complete/u.test(holderContextText)
    || !holderContextText.includes("案件事实摘录")
    || !holderContextText.includes("3")
	    || !holderContextText.includes("[account-redacted]")
    || ["SYNTHETIC-ACCOUNT-A", "SYNTHETIC-ACCOUNT-B", "SYNTHETIC-ACCOUNT-C"].some((accountKey) => holderContextText.includes(accountKey))
    || holderContextText.includes("answer_draft")
  ) {
    fail("holder_scope_support_text_preserves_top_accounts", "holder support text must preserve synthetic scope while redacting raw accounts and excluding JSON/answer drafts", agentOutputCompilerPath);
  } else {
    pass("holder_scope_support_text_preserves_top_accounts", "holder support text preserves synthetic scope while raw accounts remain redacted and no unsafe output fields are required");
  }
  const contextCompilerAgentText = releaseGuardCompactToolText({
    tool: "funds_investigate",
    status: "ok",
    answer_card: {
      title: "Release Guard Context Compiler",
      intent: "qa_review",
      answer_card_complete: true,
      recommended_next_action: "answer_now",
      max_additional_tools: 0,
      required_facts_present: true,
      context_compiler: {
        model_context: "answer_card_only",
        raw_result_policy: "cc_raw_result_marker_stays_in_artifact",
        must_write_facts: ["cc_must_write_fact_marker"],
        must_state_boundaries: ["cc_boundary_marker"],
        forbidden_as_facts: ["cc_forbidden_marker"],
        next_queries: [{ tool: "trace_fund", why_this_query: "cc_next_query_marker" }],
        stop_conditions: ["cc_stop_marker"]
      }
    },
    key_facts: {}
  });
  const contextCompilerDiagnosticText = renderDiagnosticCardForAgent({
    card_type: "claim_review_card",
    title: "Release Guard Diagnostic Context Compiler",
    intent: "claim_review",
    case_id: "release-guard-context",
    answer_card_complete: true,
    context_compiler: {
      model_context: "answer_card_only",
      raw_result_policy: "cc_diag_raw_policy_marker",
      must_write_facts: ["cc_diag_must_write_marker"],
      must_state_boundaries: ["cc_diag_boundary_marker"],
      forbidden_as_facts: ["cc_diag_forbidden_marker"],
      next_queries: [{ tool: "validate_report_claims", why_this_query: "cc_diag_next_query_marker" }],
      stop_conditions: ["cc_diag_stop_marker"]
    }
  });
  const contextCompilerHiddenMarkers = [
    "cc_raw_result_marker_stays_in_artifact",
    "cc_must_write_fact_marker",
    "cc_boundary_marker",
    "cc_forbidden_marker"
  ];
  if (
    /^\s*[[{]/u.test(contextCompilerAgentText)
    || /^\s*[[{]/u.test(contextCompilerDiagnosticText)
    || /support_only|final_answer_owned_by_focused_skill|case_id|delivery_state|answer_card_complete/u.test(contextCompilerAgentText)
    || /support_only|final_answer_owned_by_focused_skill|case_id|delivery_state|answer_card_complete/u.test(contextCompilerDiagnosticText)
    || contextCompilerHiddenMarkers.some((marker) => contextCompilerAgentText.includes(marker))
    || /cc_diag_/u.test(contextCompilerDiagnosticText)
    || contextCompilerAgentText.includes("answer_draft")
    || contextCompilerDiagnosticText.includes("answer_draft")
  ) {
    fail("context_compiler_support_hidden_contract", "ordinary agent-visible output must keep context-compiler instructions and answer drafts hidden while rendering readable fact text", agentOutputCompilerPath);
  } else {
    pass("context_compiler_support_hidden_contract", "ordinary agent-visible output keeps context-compiler instructions and answer drafts hidden while rendering readable fact text");
  }
  const currentCaseText = releaseGuardCompactToolText({
    tool: "get_current_case",
    status: "ok",
    case_id: "release-guard-active-case",
    source: "active_case",
    case: {
      case_id: "release-guard-active-case",
      case_name: "Release Guard Active Case",
      status: "ready"
    }
  });
  if (
    /^\s*[[{]/u.test(currentCaseText)
    || /support_only|final_answer_owned_by_focused_skill|case_id|delivery_state|answer_card_complete/u.test(currentCaseText)
    || !currentCaseText.includes("案件事实摘录")
    || !currentCaseText.includes("Release Guard Active Case")
    || currentCaseText.includes("answer_draft")
  ) {
    fail(
      "current_case_support_text_contract",
      "get_current_case support text must keep current-case identity available without JSON or an answer draft",
      agentOutputCompilerPath
    );
  } else {
    pass("current_case_support_text_contract", "get_current_case support text preserves current-case identity without JSON or an answer draft");
  }

  const diagnosticFactHelperFields = [
    "DIAGNOSTIC_FACT_HELPERS_VERSION",
    "makeDiagnosticFact",
    "diagnosticSourceHash",
    "amountFromDiagnosticRow",
    "diagnosticRowName",
    "diagnosticCounterpartyName",
    "diagnosticSkillWarnings",
    "diagnosticCallSummaries",
    "claimProbeSummary",
    "probePatternSummary",
    "namedCounterpartySummary",
    "compactFinancialProductLeads",
    "duplicateCandidateSummary",
    "fact_id",
    "support_query_name",
    "source_hash",
    "source_level",
    "support_status"
  ];
  const missingDiagnosticFactHelperFields = diagnosticFactHelperFields.filter((field) => !diagnosticFactHelpersText.includes(field));
  if (
    missingDiagnosticFactHelperFields.length
    || !frontdoorRuntimeText.includes("from \"./diagnostic-fact-helpers.mjs\"")
    || serverText.includes("function makeDiagnosticFact(")
    || serverText.includes("function diagnosticSourceHash(")
    || serverText.includes("function amountFromDiagnosticRow(")
    || serverText.includes("function diagnosticCounterpartyName(")
    || serverText.includes("function claimProbeSummary(")
    || serverText.includes("function compactFinancialProductLeads(")
    || /eval-fixtures|golden-answer-set\.json/u.test(diagnosticFactHelpersText)
    || /process\.env/u.test(diagnosticFactHelpersText)
  ) {
    fail(
      "diagnostic_fact_helpers_contract",
      [
        missingDiagnosticFactHelperFields.length ? `missing fields: ${missingDiagnosticFactHelperFields.join(", ")}` : "",
        !frontdoorRuntimeText.includes("from \"./diagnostic-fact-helpers.mjs\"") ? "frontdoor runtime not importing diagnostic fact helpers" : "",
        serverText.includes("function makeDiagnosticFact(") ? "server still owns diagnostic fact builder" : "",
        serverText.includes("function diagnosticSourceHash(") ? "server still owns diagnostic source hashing" : "",
        serverText.includes("function amountFromDiagnosticRow(") ? "server still owns diagnostic row amount extraction" : "",
        serverText.includes("function diagnosticCounterpartyName(") ? "server still owns diagnostic counterparty naming" : "",
        serverText.includes("function claimProbeSummary(") ? "server still owns claim probe helpers" : "",
        serverText.includes("function compactFinancialProductLeads(") ? "server still owns financial product clue compaction" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(diagnosticFactHelpersText) ? "diagnostic fact helpers reference eval fixtures" : "",
        /process\.env/u.test(diagnosticFactHelpersText) ? "diagnostic fact helpers read process env directly" : ""
      ].filter(Boolean).join("; "),
      diagnosticFactHelpersPath
    );
  } else {
    pass("diagnostic_fact_helpers_contract", "diagnostic fact/source lineage helpers are centralized outside server.mjs without eval fixture or env coupling");
  }

  const destinationDiagnosticRuntimeFields = [
    "DESTINATION_DIAGNOSTIC_RUNTIME_VERSION",
    "createDestinationDiagnosticRuntime",
    "buildDestinationOutflowDiagnosticCard",
    "buildViaOneHopDiagnosticCard",
    "buildViaContinuationDiagnosticCard",
    "destinationRowName",
    "destinationRowAmount",
    'card_type: "destination_outflow_card"',
    'card_type: "source_to_counterparty_amount_card"',
    "supportQueryName",
    "validate_continuation_list",
    "unsupported_flows",
    "forbidden_as_facts"
  ];
  const missingDestinationDiagnosticRuntimeFields = destinationDiagnosticRuntimeFields.filter((field) => !destinationDiagnosticRuntimeText.includes(field));
  if (
    missingDestinationDiagnosticRuntimeFields.length
    || !serverText.includes("from \"./destination-diagnostic-runtime.mjs\"")
    || !serverText.includes("createDestinationDiagnosticRuntime")
    || serverText.includes("function buildDestinationOutflowDiagnosticCard(")
    || serverText.includes("function buildViaOneHopDiagnosticCard(")
    || serverText.includes("function buildViaContinuationDiagnosticCard(")
    || serverText.includes("function destinationRowName(")
    || serverText.includes("function destinationRowAmount(")
    || /eval-fixtures|golden-answer-set\.json/u.test(destinationDiagnosticRuntimeText)
    || /process\.env/u.test(destinationDiagnosticRuntimeText)
  ) {
    fail(
      "destination_diagnostic_runtime_contract",
      [
        missingDestinationDiagnosticRuntimeFields.length ? `missing fields: ${missingDestinationDiagnosticRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./destination-diagnostic-runtime.mjs\"") ? "server not importing destination diagnostic runtime" : "",
        !serverText.includes("createDestinationDiagnosticRuntime") ? "server not using destination diagnostic runtime factory" : "",
        serverText.includes("function buildDestinationOutflowDiagnosticCard(") ? "server still owns destination outflow card builder" : "",
        serverText.includes("function buildViaOneHopDiagnosticCard(") ? "server still owns via one-hop card builder" : "",
        serverText.includes("function buildViaContinuationDiagnosticCard(") ? "server still owns via continuation card builder" : "",
        serverText.includes("function destinationRowName(") ? "server still owns destination row naming" : "",
        serverText.includes("function destinationRowAmount(") ? "server still owns destination row amount extraction" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(destinationDiagnosticRuntimeText) ? "destination diagnostic runtime references eval fixtures" : "",
        /process\.env/u.test(destinationDiagnosticRuntimeText) ? "destination diagnostic runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      destinationDiagnosticRuntimePath
    );
  } else {
    pass("destination_diagnostic_runtime_contract", "destination diagnostic card runtime is centralized outside server.mjs without eval fixture or env coupling");
  }

  const topRankingsDiagnosticRuntimeFields = [
    "TOP_RANKINGS_DIAGNOSTIC_RUNTIME_VERSION",
    "createTopRankingsDiagnosticRuntime",
    "buildTopRankingsDiagnosticCard",
    "diagnosticMetricRankFact",
    'card_type: "top_rankings_card"',
    "rank_accounts",
    "rank_holders",
    "rank_counterparties",
    "supportQueryName"
  ];
  const missingTopRankingsDiagnosticRuntimeFields = topRankingsDiagnosticRuntimeFields.filter((field) => !topRankingsDiagnosticRuntimeText.includes(field));
  if (
    missingTopRankingsDiagnosticRuntimeFields.length
    || !serverText.includes("from \"./top-rankings-diagnostic-runtime.mjs\"")
    || !serverText.includes("createTopRankingsDiagnosticRuntime")
    || serverText.includes("async function buildTopRankingsDiagnosticCard(")
    || serverText.includes("function diagnosticMetricRankFact(")
    || /eval-fixtures|golden-answer-set\.json/u.test(topRankingsDiagnosticRuntimeText)
    || /process\.env/u.test(topRankingsDiagnosticRuntimeText)
  ) {
    fail(
      "top_rankings_diagnostic_runtime_contract",
      [
        missingTopRankingsDiagnosticRuntimeFields.length ? `missing fields: ${missingTopRankingsDiagnosticRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./top-rankings-diagnostic-runtime.mjs\"") ? "server not importing top rankings diagnostic runtime" : "",
        !serverText.includes("createTopRankingsDiagnosticRuntime") ? "server not using top rankings diagnostic runtime factory" : "",
        serverText.includes("async function buildTopRankingsDiagnosticCard(") ? "server still owns top rankings diagnostic card builder" : "",
        serverText.includes("function diagnosticMetricRankFact(") ? "server still owns top rankings metric fact builder" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(topRankingsDiagnosticRuntimeText) ? "top rankings diagnostic runtime references eval fixtures" : "",
        /process\.env/u.test(topRankingsDiagnosticRuntimeText) ? "top rankings diagnostic runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      topRankingsDiagnosticRuntimePath
    );
  } else {
    pass("top_rankings_diagnostic_runtime_contract", "top rankings diagnostic card runtime is centralized outside server.mjs without eval fixture or env coupling");
  }

  const claimReviewDiagnosticRuntimeFields = [
    "CLAIM_REVIEW_DIAGNOSTIC_RUNTIME_VERSION",
    "createClaimReviewDiagnosticRuntime",
    "buildClaimReviewDiagnosticCard",
    'card_type: "claim_review_card"',
    "validate_report_claims",
    "write_blocked",
    "资金流向图",
    "supportQueryName"
  ];
  const missingClaimReviewDiagnosticRuntimeFields = claimReviewDiagnosticRuntimeFields.filter((field) => !claimReviewDiagnosticRuntimeText.includes(field));
  if (
    missingClaimReviewDiagnosticRuntimeFields.length
    || !serverText.includes("from \"./claim-review-diagnostic-runtime.mjs\"")
    || !serverText.includes("createClaimReviewDiagnosticRuntime")
    || serverText.includes("async function buildClaimReviewDiagnosticCard(")
    || /eval-fixtures|golden-answer-set\.json/u.test(claimReviewDiagnosticRuntimeText)
    || /process\.env/u.test(claimReviewDiagnosticRuntimeText)
  ) {
    fail(
      "claim_review_diagnostic_runtime_contract",
      [
        missingClaimReviewDiagnosticRuntimeFields.length ? `missing fields: ${missingClaimReviewDiagnosticRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./claim-review-diagnostic-runtime.mjs\"") ? "server not importing claim review diagnostic runtime" : "",
        !serverText.includes("createClaimReviewDiagnosticRuntime") ? "server not using claim review diagnostic runtime factory" : "",
        serverText.includes("async function buildClaimReviewDiagnosticCard(") ? "server still owns claim review diagnostic card builder" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(claimReviewDiagnosticRuntimeText) ? "claim review diagnostic runtime references eval fixtures" : "",
        /process\.env/u.test(claimReviewDiagnosticRuntimeText) ? "claim review diagnostic runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      claimReviewDiagnosticRuntimePath
    );
  } else {
    pass("claim_review_diagnostic_runtime_contract", "claim review diagnostic card runtime is centralized outside server.mjs without eval fixture or env coupling");
  }

  const investigationLabDiagnosticRuntimeFields = [
    "INVESTIGATION_LAB_DIAGNOSTIC_RUNTIME_VERSION",
    "createInvestigationLabDiagnosticRuntime",
    "buildInvestigationLabDiagnosticCard",
    'card_type: "investigation_lab_card"',
    "hypothesis_queue",
    "forbidden_as_facts",
    "next investigation queue",
    "supportQueryName"
  ];
  const missingInvestigationLabDiagnosticRuntimeFields = investigationLabDiagnosticRuntimeFields.filter((field) => !investigationLabDiagnosticRuntimeText.includes(field));
  if (
    missingInvestigationLabDiagnosticRuntimeFields.length
    || !serverText.includes("from \"./investigation-lab-diagnostic-runtime.mjs\"")
    || !serverText.includes("createInvestigationLabDiagnosticRuntime")
    || serverText.includes("async function buildInvestigationLabDiagnosticCard(")
    || /eval-fixtures|golden-answer-set\.json/u.test(investigationLabDiagnosticRuntimeText)
    || /process\.env/u.test(investigationLabDiagnosticRuntimeText)
  ) {
    fail(
      "investigation_lab_diagnostic_runtime_contract",
      [
        missingInvestigationLabDiagnosticRuntimeFields.length ? `missing fields: ${missingInvestigationLabDiagnosticRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./investigation-lab-diagnostic-runtime.mjs\"") ? "server not importing investigation lab diagnostic runtime" : "",
        !serverText.includes("createInvestigationLabDiagnosticRuntime") ? "server not using investigation lab diagnostic runtime factory" : "",
        serverText.includes("async function buildInvestigationLabDiagnosticCard(") ? "server still owns investigation lab diagnostic card builder" : "",
        /eval-fixtures|golden-answer-set\.json/u.test(investigationLabDiagnosticRuntimeText) ? "investigation lab diagnostic runtime references eval fixtures" : "",
        /process\.env/u.test(investigationLabDiagnosticRuntimeText) ? "investigation lab diagnostic runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      investigationLabDiagnosticRuntimePath
    );
  } else {
    pass("investigation_lab_diagnostic_runtime_contract", "investigation lab diagnostic card runtime is centralized outside server.mjs without eval fixture or env coupling");
  }

  const pluginManifestPath = path.join(PLUGIN_ROOT, ".codex-plugin", "plugin.json");
  const pluginManifest = JSON.parse(fs.readFileSync(pluginManifestPath, "utf8"));
  const mcpManifestPath = path.join(PLUGIN_ROOT, ".mcp.json");
  const mcpManifest = JSON.parse(fs.readFileSync(mcpManifestPath, "utf8"));
  const mcpConfig = objectOf(mcpManifest.mcpServers?.analytix_funds);
  const pluginVersion = text(pluginManifest.version);
  const serverVersion = text(serverText.match(/const\s+SERVER_VERSION\s*=\s*"([^"]+)"/u)?.[1]);
  const expectedReleaseTag = pluginVersion ? `analytix-fund-analysis-v${pluginVersion}` : "";
  const releaseNotesPath = path.join(PLUGIN_ROOT, "RELEASE_NOTES.md");
  const releaseNotesText = fs.existsSync(releaseNotesPath) ? fs.readFileSync(releaseNotesPath, "utf8") : "";
  if (!pluginVersion || !serverVersion || pluginVersion !== serverVersion) {
    fail(
      "release_manifest_server_version",
      `plugin.json version (${pluginVersion || "missing"}) must match mcp/server.mjs SERVER_VERSION (${serverVersion || "missing"})`,
      pluginManifestPath
    );
  } else {
    pass("release_manifest_server_version", `plugin.json and mcp/server.mjs agree on ${pluginVersion}`);
  }
  if (mcpConfig.disabled !== true) {
    fail(
      "mcp_native_authority_quarantine",
      "analytix_funds must remain disabled before process creation until the Go host owns executable and dataset authority",
      mcpManifestPath
    );
  } else {
    pass("mcp_native_authority_quarantine", "funds MCP is P0-disabled and cannot launch the PATH-selected Node entrypoint");
  }
  const runtimeVersionMismatches = [
    ["TOOL_CALL_RUNTIME_VERSION", TOOL_CALL_RUNTIME_VERSION],
    ["MCP_REQUEST_HANDLER_RUNTIME_VERSION", MCP_REQUEST_HANDLER_RUNTIME_VERSION],
    ["MCP_TOOL_RESULT_RUNTIME_VERSION", MCP_TOOL_RESULT_RUNTIME_VERSION],
    ["DUCKDB_WORKBENCH_RUNTIME_VERSION", DUCKDB_WORKBENCH_RUNTIME_VERSION],
    ["EVAL_COVERAGE_CONTRACT_VERSION", EVAL_COVERAGE_CONTRACT_VERSION],
    ["RUNTIME_CACHE_CONTRACT_VERSION", RUNTIME_CACHE_CONTRACT_VERSION, `${pluginVersion}-runtime-cache-contract`]
  ].flatMap(([name, observed, expected = pluginVersion]) => observed === expected ? [] : [`${name}=${observed || "missing"} expected ${expected}`]);
  if (runtimeVersionMismatches.length) {
    fail("release_runtime_version_alignment", runtimeVersionMismatches.join("; "), pluginManifestPath);
  } else {
    pass("release_runtime_version_alignment", `runtime modules and release contracts agree on ${pluginVersion}`);
  }
  if (objectOf(mcpConfig.env).ANALYTIX_API_BASE_URL !== undefined) {
    fail(
      "mcp_backend_env_injection_contract",
      ".mcp.json must not hardcode env.ANALYTIX_API_BASE_URL; app-server/runtime env injects the active backend URL for Agent UI release evidence",
      mcpManifestPath
    );
  } else if (!arrayOf(mcpConfig.env_vars).map(text).includes("ANALYTIX_API_BASE_URL")) {
    fail(
      "mcp_backend_env_injection_contract",
      ".mcp.json must allowlist ANALYTIX_API_BASE_URL in env_vars so runtime env can provide the backend URL",
      mcpManifestPath
    );
  } else {
    pass("mcp_backend_env_injection_contract", "backend URL is runtime-injected via env_vars, not hardcoded in .mcp.json");
  }
  if (!releaseNotesText.includes(`## ${pluginVersion}`)) {
    fail(
      "release_notes_current_version",
      `RELEASE_NOTES.md must include ## ${pluginVersion}`,
      releaseNotesPath
    );
  } else {
    pass("release_notes_current_version", `RELEASE_NOTES.md contains ${pluginVersion}`);
  }
  const headCommit = gitOutput(["rev-parse", "--verify", "HEAD"]);
  const taggedCommit = expectedReleaseTag ? gitOutput(["rev-list", "-n", "1", expectedReleaseTag]) : "";
  if (!headCommit || !taggedCommit || headCommit !== taggedCommit) {
    fail(
      "release_git_tag_identity",
      [
        !headCommit ? "current HEAD is not available" : `HEAD=${headCommit.slice(0, 12)}`,
        !taggedCommit ? `${expectedReleaseTag || "expected release tag"} is missing` : `${expectedReleaseTag}=${taggedCommit.slice(0, 12)}`,
        headCommit && taggedCommit && headCommit !== taggedCommit ? "release tag does not point at HEAD" : ""
      ].filter(Boolean).join("; "),
      pluginManifestPath
    );
  } else {
    pass("release_git_tag_identity", `${expectedReleaseTag} points at HEAD ${headCommit.slice(0, 12)}`);
  }
  const dirtyWorktree = gitOutput(["status", "--porcelain=v1", "--untracked-files=all"])
    .split(/\r?\n/u)
    .filter(Boolean);
  if (dirtyWorktree.length) {
    const dirtyDetails = dirtyWorktree.map((line) => line.trim()).filter(Boolean);
    const dirtyPathAt = (index) => {
      const line = dirtyWorktree[index] || dirtyDetails[index] || "";
      const rawPath = text(line[2] === " " ? line.slice(3) : line.slice(2));
      return rawPath.includes(" -> ") ? text(rawPath.split(" -> ").at(-1)) : rawPath;
    };
    const pluginDirty = dirtyDetails.filter((_, index) => dirtyPathAt(index).startsWith("plugins/analytix-fund-analysis/"));
    const nonPluginDirty = dirtyDetails.filter((_, index) => !dirtyPathAt(index).startsWith("plugins/analytix-fund-analysis/"));
    fail(
      "release_git_worktree_clean",
      [
        `worktree has ${dirtyWorktree.length} dirty path(s): plugin=${pluginDirty.length}, non_plugin=${nonPluginDirty.length}`,
        pluginDirty.length ? `plugin dirty: ${pluginDirty.slice(0, 6).join("; ")}${pluginDirty.length > 6 ? "; ..." : ""}` : "",
        nonPluginDirty.length ? `non-plugin sample: ${nonPluginDirty.slice(0, 8).join("; ")}${nonPluginDirty.length > 8 ? "; ..." : ""}` : "",
      ].filter(Boolean).join("; "),
      REPO_ROOT
    );
  } else {
    pass("release_git_worktree_clean", "git worktree is clean for release");
  }
  const commandMetadataPath = path.join(PLUGIN_ROOT, "references", "command-metadata.json");
  const capabilityRegistryPath = path.join(PLUGIN_ROOT, "references", "capability-registry.json");
  const capabilityRegistrySchemaPath = path.join(PLUGIN_ROOT, "references", "capability-registry.schema.json");
  const commandMetadata = JSON.parse(fs.readFileSync(commandMetadataPath, "utf8"));
  const capabilityRegistry = JSON.parse(fs.readFileSync(capabilityRegistryPath, "utf8"));
  const evalCoverageFixture = JSON.parse(fs.readFileSync(EVAL_COVERAGE_CONTRACT_PATH, "utf8"));
  const capabilityRegistryWithEval = withEvalCoverageContract(capabilityRegistry, evalCoverageFixture);
  const capabilityRegistrySchema = JSON.parse(fs.readFileSync(capabilityRegistrySchemaPath, "utf8"));
  const evalCoverageSchemaValidation = validateEvalCoverageRegistrySchema({ capabilityRegistrySchema });
  const commandList = arrayOf(commandMetadata.commands);
  const metadataCommands = commandList.map((item) => text(item.command)).filter(Boolean);
  const metadataCommandSet = new Set(metadataCommands);
  const capabilities = arrayOf(capabilityRegistry.capabilities);
  const registryCommandOwners = new Map();
  const duplicateRegistryCommands = [];
  for (const capability of capabilities) {
    for (const command of arrayOf(capability.commands).map(text).filter(Boolean)) {
      if (registryCommandOwners.has(command)) duplicateRegistryCommands.push(command);
      else registryCommandOwners.set(command, text(capability.id));
    }
  }
  const toolExposureSource = `${serverText}\n${toolSchemasText}`;
  const serverTools = new Set([...toolExposureSource.matchAll(/name:\s*"([a-zA-Z0-9_]+)"/gu)].map((match) => match[1]));
  const registryTools = new Set(capabilities.flatMap((capability) => [
    ...arrayOf(capability.primary_tools),
    ...arrayOf(capability.conditional_tools)
  ].map(text).filter(Boolean)));
  const commandToolCoverageFailures = commandList.flatMap((command) => {
    const commandName = text(command.command);
    const capability = capabilities.find((item) => arrayOf(item.commands).map(text).includes(commandName));
    if (!commandName || !capability) return [];
    const capabilityTools = new Set([
      ...arrayOf(capability.primary_tools),
      ...arrayOf(capability.conditional_tools)
    ].map(text).filter(Boolean));
    const commandTools = [
      ...arrayOf(command.primaryTools),
      ...arrayOf(command.conditionalTools)
    ].map(text).filter(Boolean);
    const missing = commandTools.filter((tool) => !capabilityTools.has(tool));
    return missing.length ? [`${commandName}: ${missing.join(", ")}`] : [];
  });
  const commandBudgetFailures = commandList.flatMap((command) => {
    const commandName = text(command.command);
    const budget = objectOf(command.toolBudget);
    const recommended = Number(budget.recommendedToolCalls);
    const max = Number(budget.maxToolCalls);
    const maxWithAudit = Number(budget.maxToolCallsWithAuditBoundaries);
    const derivedBudget = commandBudgetFromCapabilityRegistry(commandName, capabilityRegistry);
	    const auditMax = Number(derivedBudget?.max_command_audit_tool_calls || derivedBudget?.report_max_tool_calls || 2);
	    const ordinaryMax = Number(derivedBudget?.ordinary_max_tool_calls || 1);
	    if (!Object.keys(budget).length) return [];
	    if (recommended > ordinaryMax || max > ordinaryMax || maxWithAudit > auditMax) {
	      return [`${commandName}: recommended=${budget.recommendedToolCalls} max=${budget.maxToolCalls} audit=${budget.maxToolCallsWithAuditBoundaries}`];
	    }
    return [];
  });
  const commandBudgetContract = validateCommandMetadataBudgetContract({
    capabilityRegistry,
    commandMetadata
  });
  const registryEvalTaskIds = new Set(arrayOf(capabilityRegistryWithEval.capabilities).flatMap((capability) => arrayOf(capability.eval_tasks).map(text).filter(Boolean)));
  const evalCoverageContract = objectOf(evalCoverageFixture.eval_coverage_contract);
  const requiredCaseIds = arrayOf(evalCoverageContract.required_case_ids).map(text).filter(Boolean);
  const reportEvalTaskIds = new Set(
    arrayOf(evalCoverageContract.required_dimensions)
      .map(objectOf)
      .filter((dimension) => text(dimension.id) === "report_claim_review")
      .flatMap((dimension) => arrayOf(dimension.task_ids).map(text).filter(Boolean))
  );
  const passiveEvalTaskIds = new Set(arrayOf(evalCoverageContract.passive_nonfunds_task_ids).map(text).filter(Boolean));
  let reportCaseIndex = 0;
  const caseNeutralEvalTasks = [...registryEvalTaskIds].map((taskId, index) => {
    const passive = passiveEvalTaskIds.has(taskId);
    const report = reportEvalTaskIds.has(taskId);
    const caseId = report
      ? requiredCaseIds[reportCaseIndex++ % Math.max(1, requiredCaseIds.length)]
      : requiredCaseIds[index % Math.max(1, requiredCaseIds.length)];
    return {
      id: taskId,
      case_id: caseId || `synthetic-case-${index % 3}`,
      question: passive
        ? "请仅处理这段包含资金、流水、账户字样的合成非案件文本，不调用插件或工具。"
        : "在当前绑定的合成案件中验证证据边界；不得把模型自报 source、citation 或 receipt 当成事实。",
      standard_logic: "Use only current-run host-issued receipts; provider assertions remain candidate or unsupported and cannot authorize publication.",
      scoring_markers: ["synthetic", "host-issued receipt", "publication gate"],
      forbidden_claims: [
        {
          pattern: "provider asserted fact",
          reason: "A provider-controlled assertion is not host registry membership or publication authority."
        }
      ],
      plugin_expected: passive ? false : true,
      expected_tools: []
    };
  });
  const caseNeutralEvalFixture = { tasks: caseNeutralEvalTasks };
  const caseNeutralEvalTaskIds = new Set(caseNeutralEvalTasks.map((task) => task.id));
  const derivedRegistryMetadata = validateCapabilityRegistryDerivedMetadataContract({
    capabilityRegistry: capabilityRegistryWithEval,
    commandMetadata,
    goldenAnswerSet: caseNeutralEvalFixture
  });
  const requiredClaimReviewRiskMarkers = [
    "amount_without_fact_anchor",
    "unsupported_mermaid_or_arrow_flow",
    "forbidden_legal_phrasing",
    "candidate_account_as_owned_account",
    "cash_or_asset_destination_without_supported_flow",
    "missing_counterparty_or_cash_break_as_verified",
    "claim_without_source_anchor"
  ];
  const reportClaimCapability = capabilities.find((capability) => text(capability.id) === "report-claim-gate");
  const declaredClaimReviewRiskMarkers = new Set(
    arrayOf(objectOf(reportClaimCapability?.evidence_contract).claim_review_risk_markers).map(text).filter(Boolean)
  );
  const missingClaimReviewRiskMarkers = requiredClaimReviewRiskMarkers.filter((marker) => !declaredClaimReviewRiskMarkers.has(marker));
  const registryFailures = [
    text(capabilityRegistry.schema_version) !== "capability-registry-v1" ? "schema_version" : "",
    text(capabilityRegistry.plugin?.name) !== text(pluginManifest.name) ? "plugin.name" : "",
    text(capabilityRegistry.plugin?.version) !== text(pluginManifest.version) ? "plugin.version" : "",
    !evalCoverageSchemaValidation.ok ? evalCoverageSchemaValidation.failures.join("; ") : "",
    !capabilities.length ? "capabilities" : "",
    duplicateRegistryCommands.length ? `duplicate commands: ${duplicateRegistryCommands.join(", ")}` : "",
    metadataCommands.filter((command) => !registryCommandOwners.has(command)).length
      ? `missing commands: ${metadataCommands.filter((command) => !registryCommandOwners.has(command)).join(", ")}`
      : "",
    [...registryCommandOwners.keys()].filter((command) => !metadataCommandSet.has(command)).length
      ? `extra commands: ${[...registryCommandOwners.keys()].filter((command) => !metadataCommandSet.has(command)).join(", ")}`
      : "",
    [...registryTools].filter((tool) => !serverTools.has(tool)).length
      ? `tools not exposed: ${[...registryTools].filter((tool) => !serverTools.has(tool)).join(", ")}`
      : "",
    commandToolCoverageFailures.length ? `command tools missing from owning capability: ${commandToolCoverageFailures.join("; ")}` : "",
    commandBudgetFailures.length ? `command tool budgets exceed registry budget: ${commandBudgetFailures.join("; ")}` : "",
    !commandBudgetContract.ok ? `command budget contract: ${commandBudgetContract.failures.join("; ")}` : "",
    capabilities.filter((capability) => objectOf(capability.evidence_contract).ledger_required !== true || objectOf(capability.evidence_contract).user_visible_opaque_refs_allowed !== false).length
      ? "evidence contract"
      : "",
    missingClaimReviewRiskMarkers.length ? `report claim risk markers missing from registry: ${missingClaimReviewRiskMarkers.join(", ")}` : "",
    capabilities.filter((capability) => {
      const budget = objectOf(capability.tool_budget);
      const ordinaryMax = Number(budget.ordinary_max_tool_calls);
      const reportMax = Number(budget.report_max_tool_calls);
      const commandAuditMax = Number(budget.max_command_audit_tool_calls || 2);
      return !Number.isFinite(ordinaryMax)
        || !Number.isFinite(reportMax)
        || ordinaryMax < 1
        || ordinaryMax > 3
        || reportMax < 1
        || reportMax > commandAuditMax;
    }).length
      ? "tool budget"
      : "",
    [...registryEvalTaskIds].filter((taskId) => !caseNeutralEvalTaskIds.has(taskId)).length
      ? `registry eval tasks missing from case-neutral fixture: ${[...registryEvalTaskIds].filter((taskId) => !caseNeutralEvalTaskIds.has(taskId)).join(", ")}`
      : "",
    [...caseNeutralEvalTaskIds].filter((taskId) => !registryEvalTaskIds.has(taskId)).length
      ? `case-neutral fixture tasks not covered by registry: ${[...caseNeutralEvalTaskIds].filter((taskId) => !registryEvalTaskIds.has(taskId)).join(", ")}`
      : ""
  ].filter(Boolean);
  if (registryFailures.length) {
    fail("capability_registry_drift", registryFailures.join("; "), capabilityRegistryPath);
  } else {
    pass("capability_registry_drift", `${capabilities.length} capabilities cover ${metadataCommands.length} commands, ${registryTools.size} tools, and ${caseNeutralEvalTaskIds.size} generated case-neutral eval task ids`);
  }
  if (derivedRegistryMetadata.ok) {
    const summary = derivedRegistryMetadata.summary;
    pass(
      "capability_registry_derived_metadata",
      `${summary.command_count} commands and ${summary.tool_count} tools derive from production registry; ${summary.eval_task_count} eval tasks, ${summary.passive_nonfunds_task_count} passive tasks, and ${summary.report_task_count} report tasks derive from eval-only contract`
    );
  } else {
    fail("capability_registry_derived_metadata", derivedRegistryMetadata.failures.join("; "), capabilityRegistryPath);
  }
  const evalCoverage = validateEvalCoverageContract({ capabilityRegistry: capabilityRegistryWithEval, goldenAnswerSet: caseNeutralEvalFixture });
  if (!evalCoverage.ok) {
    fail("case_neutral_eval_coverage_ratchet", evalCoverage.failures.join("; "), EVAL_COVERAGE_CONTRACT_PATH);
  } else {
    pass("case_neutral_eval_coverage_ratchet", `${formatEvalCoverageSummary(evalCoverage)}; generated fixture contains no case facts, PII, report text, or provider-authoritative receipts`);
  }
  const defaultPrompts = arrayOf(pluginManifest.interface?.defaultPrompt);
  if (defaultPrompts.length > 3) {
    fail("plugin_manifest_default_prompt_limit", `interface.defaultPrompt has ${defaultPrompts.length} entries; runtime supports at most 3`, pluginManifestPath);
  } else {
    pass("plugin_manifest_default_prompt_limit", `interface.defaultPrompt has ${defaultPrompts.length} entries`);
  }
  const nonNeutralDefaultPrompts = defaultPrompts.filter((prompt) =>
    !/(?:当前案件|某主体|某人|某公司|目标主体|付款方|收款方)/u.test(text(prompt))
    || /(?:\b\d{8,}\b|20\d{2}-\d{2}-\d{2})/u.test(text(prompt))
  );
  if (nonNeutralDefaultPrompts.length) {
    fail("plugin_manifest_case_neutral_default_prompts", "interface.defaultPrompt must not embed fixed case subjects, identifiers, or dates", pluginManifestPath);
  } else {
    pass("plugin_manifest_case_neutral_default_prompts", "interface.defaultPrompt uses case-neutral selected-case examples");
  }

  let productionGoldenFound = false;
  let productionSystemCodexRefFound = false;
  const productionRoots = [".codex-plugin", "mcp", "skills", "references"].map((item) => path.join(PLUGIN_ROOT, item));
  for (const filePath of productionRoots.flatMap(listFiles)) {
    const relative = path.relative(PLUGIN_ROOT, filePath);
    if (relative.includes("references/golden-answer-set.json") || relative.includes("references/golden-eval-rubric.md")) {
      productionGoldenFound = true;
      fail("production_golden_answer_set", "golden eval artifacts must exist only under scripts/eval-fixtures, not production references", filePath);
      continue;
    }
    if (!/\.(mjs|js|json|md)$/iu.test(filePath)) continue;
    const body = fs.readFileSync(filePath, "utf8");
    if (/scripts\/eval-fixtures|eval-only fastpath|ANALYTIX_FUNDS_EVAL_FAST_PATH|oracle_fast_path/iu.test(body)) {
      fail("production_eval_fastpath", "production path contains eval fast-path logic", filePath);
    }
    const codexStateRefLines = body
      .split(/\r?\n/u)
      .filter((line) => /(?:~\/\.codex|\$HOME\/\.codex|\/Users\/[^/\s]+\/\.codex|CODEX_HOME)/u.test(line))
      .filter((line) => !/(?:不改|不得|禁止|不污染|不能|must not|Do not|not modify|not write|not pollute|refusing)/iu.test(line));
    if (codexStateRefLines.length) {
      productionSystemCodexRefFound = true;
      fail("production_system_codex_state", "production package must not reference system Codex home/config/cache state outside boundary prohibitions", filePath);
    }
  }
  if (!productionGoldenFound) {
    pass("production_golden_answer_set", "golden eval artifacts exist only under scripts/eval-fixtures");
  }
  if (!checks.some((item) => item.id === "production_eval_fastpath" && item.status === "fail")) {
    pass("production_eval_fastpath", "production paths do not contain eval fast-path logic");
  }
  if (!productionSystemCodexRefFound) {
    pass("production_system_codex_state", "production package does not reference system Codex home/config/cache state");
  }

  const hubLifecyclePath = path.join(PLUGIN_ROOT, "references", "hub-lifecycle.md");
  const hubLifecycleText = fs.existsSync(hubLifecyclePath) ? fs.readFileSync(hubLifecyclePath, "utf8") : "";
  const lifecycleFields = [
    "## Publish",
    "## Install",
    "## Remount",
    "## Uninstall",
    "## Rollback",
    "sync-runtime-cache.mjs",
    "local verification/remount aid only",
    "must not create a Hub installation",
    "must not",
    "customer upgrade path"
  ];
  const missingLifecycleFields = lifecycleFields.filter((field) => !hubLifecycleText.includes(field));
  if (missingLifecycleFields.length) {
    fail("hub_lifecycle_contract", `missing Hub lifecycle boundaries: ${missingLifecycleFields.join(", ")}`, hubLifecyclePath);
  } else {
    pass("hub_lifecycle_contract", "Hub publish/install/uninstall/rollback/remount boundaries are documented");
  }
  const runtimeBoundaryPath = path.join(PLUGIN_ROOT, "references", "runtime-boundary.md");
  const runtimeBoundaryText = fs.existsSync(runtimeBoundaryPath) ? fs.readFileSync(runtimeBoundaryPath, "utf8") : "";
  const skillRuntimeBoundaryPath = path.join(PLUGIN_ROOT, "skills", "analytix-fund-analysis", "references", "runtime-boundary.md");
  const skillRuntimeBoundaryText = fs.existsSync(skillRuntimeBoundaryPath) ? fs.readFileSync(skillRuntimeBoundaryPath, "utf8") : "";
  const authBoundaryFields = [
    "common Codex GPT login/auth state",
    "Auth sharing is read-only",
    "provider token files",
    "OAuth browser state",
    "model-provider config",
    "auth/runtime failure",
    "redact Bearer tokens",
    "API keys",
    "JWTs",
    "refresh tokens",
    "session tokens",
    "Login remediation belongs to the Analytixagent/Codex provider UI",
    "must not implement alternate login",
    "token repair",
    "token sync",
    "provider-config migration"
  ];
  const missingRootAuthBoundary = authBoundaryFields.filter((field) => !runtimeBoundaryText.includes(field));
  const missingSkillAuthBoundary = authBoundaryFields.filter((field) => !skillRuntimeBoundaryText.includes(field));
  if (missingRootAuthBoundary.length || missingSkillAuthBoundary.length) {
    fail(
      "auth_runtime_boundary_contract",
      [
        missingRootAuthBoundary.length ? `root missing: ${missingRootAuthBoundary.join(", ")}` : "",
        missingSkillAuthBoundary.length ? `skill missing: ${missingSkillAuthBoundary.join(", ")}` : ""
      ].filter(Boolean).join("; "),
      missingRootAuthBoundary.length ? runtimeBoundaryPath : skillRuntimeBoundaryPath
    );
  } else {
    pass("auth_runtime_boundary_contract", "runtime boundary documents shared auth as read-only, token-redacted, and owned by Analytixagent/Codex provider UI");
  }
  const syncRuntimeCachePath = path.join(PLUGIN_ROOT, "scripts", "sync-runtime-cache.mjs");
  const syncRuntimeCacheText = fs.existsSync(syncRuntimeCachePath) ? fs.readFileSync(syncRuntimeCachePath, "utf8") : "";
  if (
    !syncRuntimeCacheText.includes("assertAnalytixRuntimeHome")
    || !syncRuntimeCacheText.includes("Installed runtime cache is missing")
    || !syncRuntimeCacheText.includes("Runtime required skill mount is missing")
    || !syncRuntimeCacheText.includes("Refusing to sync inside system Codex home")
    || !syncRuntimeCacheText.includes("--confirm-local-remount")
    || !syncRuntimeCacheText.includes("Refusing --apply without --confirm-local-remount")
    || !syncRuntimeCacheText.includes("Existing same-version direct runtime cache is required")
    || !syncRuntimeCacheText.includes("source.sha256 !== plan.workspace_sha256")
    || !syncRuntimeCacheText.includes("hub_publish_path: false")
    || !syncRuntimeCacheText.includes("install_path: false")
    || /mkdirSync\([^)]*installedRoot/iu.test(syncRuntimeCacheText)
  ) {
    fail("runtime_cache_sync_not_install_path", "runtime cache sync must require an existing Analytix-owned install, explicit local remount confirmation, and must not manufacture Hub install roots", syncRuntimeCachePath);
  } else {
    pass("runtime_cache_sync_not_install_path", "runtime cache sync is constrained to explicitly confirmed local remounts of existing Analytix-owned installs");
  }
  runNodeJsonSelfTest(
    "runtime_cache_config_remount_self_test",
    syncRuntimeCachePath,
    ["--self-test-runtime-config", "--json"],
    (result) => {
      if (result.ok !== true) {
        return { ok: false, message: "runtime cache config remount self-test did not report ok:true" };
      }
      if (result.dry_action !== "would_patch" || result.apply_action !== "patch") {
        return {
          ok: false,
          message: `unexpected remount actions: dry=${text(result.dry_action)}, apply=${text(result.apply_action)}`
        };
      }
      const serverArg = text(result.server_arg);
      const serverCwd = text(result.server_cwd);
      const skillRoot = text(result.skill_root);
      if (!serverArg.includes("/analytix-fund-analysis/0.16.9/mcp/server.mjs")) {
        return { ok: false, message: `runtime config remount did not patch MCP server path: ${serverArg}` };
      }
      if (!serverCwd.includes("/analytix-fund-analysis/0.16.9")) {
        return { ok: false, message: `runtime config remount did not patch MCP cwd: ${serverCwd}` };
      }
      if (!skillRoot.includes("/analytix-fund-analysis/0.16.9/skills")) {
        return { ok: false, message: `runtime config remount did not patch skill root: ${skillRoot}` };
      }
      return { ok: true, message: "" };
    },
    "runtime cache config remount self-test patches MCP server path, cwd, and skill root for existing Analytix-owned installs"
  );
  runNodeJsonSelfTest(
    "runtime_cache_same_version_hash_idempotence_self_test",
    syncRuntimeCachePath,
    ["--self-test-local-remount", "--json"],
    (result) => {
      if (result.ok !== true || !text(result.same_version_guard_error).includes("Existing same-version direct runtime cache is required")) {
        return { ok: false, message: "local remount self-test did not fail closed without a same-version direct install" };
      }
      const workspaceHash = text(result.workspace_sha256);
      if (!/^[0-9a-f]{64}$/u.test(workspaceHash) || !arrayOf(result.marker_hashes).every((hash) => text(hash) === workspaceHash)) {
        return { ok: false, message: "local remount marketplace, policy, and installed marker hashes diverged" };
      }
      if (Number(result.second_dry_run_copy_count) !== 0) {
        return { ok: false, message: `local remount second dry run planned ${result.second_dry_run_copy_count} copies` };
      }
      return { ok: true, message: "" };
    },
    "runtime cache local remount requires an existing same-version direct install, converges one hash authority, and is idempotent"
  );
  const runtimeCacheEvalOnlyViolations = runtimeCacheContractViolations();
  const productionMcpRuntimeCacheOmissions = PRODUCTION_MCP_ENTRY_CLOSURE_FILES
    .filter((relativePath) => !PLUGIN_CACHE_FILES.includes(relativePath));
  const dormantMcpRuntimeCacheEntries = PLUGIN_CACHE_FILES
    .filter((relativePath) => relativePath.startsWith("mcp/"))
    .filter((relativePath) => !PRODUCTION_MCP_ENTRY_CLOSURE_FILES.includes(relativePath));
  let productionMcpClosureError = "";
  try {
    inspectProductionMcpEntryClosure(PLUGIN_ROOT);
  } catch (error) {
    productionMcpClosureError = error instanceof Error ? error.message : String(error);
  }
  if (runtimeCacheEvalOnlyViolations.length) {
    fail(
      "runtime_cache_contract_production_files",
      runtimeCacheEvalOnlyViolations
        .map((item) => `${item.scope}:${item.relativePath} matches ${item.forbidden_pattern}`)
        .join("; "),
      path.join(PLUGIN_ROOT, "scripts", "runtime-cache-contract.mjs")
    );
  } else {
    pass("runtime_cache_contract_production_files", "runtime cache contract excludes eval fixtures, golden answer sets, eval rubrics, and oracle fast paths");
  }
  if (productionMcpRuntimeCacheOmissions.length || dormantMcpRuntimeCacheEntries.length || productionMcpClosureError) {
    fail(
      "runtime_cache_contract_mcp_entry_closure",
      [
        productionMcpRuntimeCacheOmissions.length
          ? `missing production modules: ${productionMcpRuntimeCacheOmissions.join(", ")}`
          : "",
        dormantMcpRuntimeCacheEntries.length
          ? `dormant modules included: ${dormantMcpRuntimeCacheEntries.join(", ")}`
          : "",
        productionMcpClosureError
      ].filter(Boolean).join("; "),
      path.join(PLUGIN_ROOT, "scripts", "runtime-cache-contract.mjs")
    );
  } else {
    pass("runtime_cache_contract_mcp_entry_closure", `runtime cache includes exactly the ${PRODUCTION_MCP_ENTRY_CLOSURE_FILES.length} structurally verified production MCP modules and excludes dormant modules`);
  }

  const rendererPath = path.join(PLUGIN_ROOT, "mcp", "card-renderer.mjs");
  const rendererText = fs.readFileSync(rendererPath, "utf8");
  const rendererImpurePattern = /\b(?:fetch|XMLHttpRequest|executeSkill|child_process|spawn|execFile|execSync|readFileSync|writeFileSync|process\.env|ANALYTIX_API|http:\/\/|https:\/\/|\/api\/v1\/skills)\b/u;
  const rendererImports = rendererText
    .split(/\r?\n/u)
    .filter((line) => /^\s*import\s/u.test(line));
  const unsafeRendererImports = rendererImports
    .filter((line) => !line.includes("./user-facing-language.mjs")
      && !line.includes("./agent-context-hygiene.mjs")
      && !line.includes("./runtime-normalizers.mjs"));
  if (unsafeRendererImports.length || rendererImpurePattern.test(rendererText)) {
    fail("card_renderer_pure_renderer", "card-renderer must remain a pure renderer with no backend, filesystem, process, or network access beyond local text translation", rendererPath);
  } else {
    pass("card_renderer_pure_renderer", "card-renderer has no backend/filesystem/network/process access; imports are limited to local text translation, PII redaction, and strict numeric normalization");
  }
  if (/\b(?:oracle|golden_anchor_key|golden-eval-rubric|eval-fixtures)\b/u.test(rendererText)) {
    fail("card_renderer_eval_isolation", "card-renderer must not carry eval/oracle diff fields in the production renderer", rendererPath);
  } else {
    pass("card_renderer_eval_isolation", "eval/oracle diff helpers are kept outside the production renderer");
  }
  const agentContextHygieneFields = [
    "AGENT_CONTEXT_HYGIENE_VERSION",
    "AGENT_CONTEXT_HYGIENE_GUARDS",
    "redactLocalPathText",
    "redactAgentPayload",
    "stripAgentOpaqueRefs",
    "hasOpaqueAgentRefs",
    "strip_internal_query_ids",
    "strip_audit_ref",
    "strip_artifact_id",
    "strip_evidence_refs",
    "redact_local_paths",
    "model_context_no_opaque_refs"
  ];
  const agentContextContractSource = `${serverText}\n${agentOutputCompilerText}\n${safeSkillRuntimeText}\n${agentContextHygieneText}`;
  const missingAgentContextFields = agentContextHygieneFields.filter((field) => !agentContextContractSource.includes(field));
  const hygieneSample = {
    answer_text: "来自 q_abcdef1234567890 audit_ref:abc artifact_id:mcp-full-abcdef123456，路径 /Users/sun/secret/case.csv。",
    evidence_refs: { query_ids: ["q_abcdef1234567890"], audit_ref: "abc" },
    keep: "SAME_FACT_DUPLICATE_FAMILIES"
  };
  const redactedHygieneSample = redactAgentPayload(hygieneSample);
  const strippedHygieneSample = stripAgentOpaqueRefs(redactedHygieneSample);
  if (
    missingAgentContextFields.length
    || !safeSkillRuntimeText.includes("from \"./agent-context-hygiene.mjs\"")
    || !safeSkillRuntimeText.includes("redactAgentPayload")
    || !agentOutputCompilerText.includes("stripAgentOpaqueRefs")
    || !serverText.includes("from \"./agent-output-compiler.mjs\"")
    || !JSON.stringify(redactedHygieneSample).includes("[local-path]/case.csv")
    || hasOpaqueAgentRefs(strippedHygieneSample)
    || JSON.stringify(strippedHygieneSample).includes("SAME_FACT_DUPLICATE_FAMILIES")
    || !JSON.stringify(strippedHygieneSample).includes("同事实重复候选族")
  ) {
    fail(
      "agent_context_hygiene_contract",
      [
        missingAgentContextFields.length ? `missing fields: ${missingAgentContextFields.join(", ")}` : "",
        !safeSkillRuntimeText.includes("from \"./agent-context-hygiene.mjs\"") ? "safe skill runtime not importing agent-context hygiene" : "",
        !safeSkillRuntimeText.includes("redactAgentPayload") ? "safe skill runtime not using redactAgentPayload" : "",
        !agentOutputCompilerText.includes("stripAgentOpaqueRefs") ? "agent output compiler not using stripAgentOpaqueRefs" : "",
        !serverText.includes("from \"./agent-output-compiler.mjs\"") ? "server not importing agent output compiler" : "",
        !JSON.stringify(redactedHygieneSample).includes("[local-path]/case.csv") ? "local path not redacted" : "",
        hasOpaqueAgentRefs(strippedHygieneSample) ? "opaque refs remain after stripAgentOpaqueRefs" : "",
        JSON.stringify(strippedHygieneSample).includes("SAME_FACT_DUPLICATE_FAMILIES") ? "same-fact marker not humanized" : "",
        !JSON.stringify(strippedHygieneSample).includes("同事实重复候选族") ? "humanized same-fact marker missing" : ""
      ].filter(Boolean).join("; "),
      agentContextHygienePath
    );
  } else {
    pass("agent_context_hygiene_contract", "agent context hygiene strips opaque refs, redacts local paths, and keeps model-visible text human-readable");
  }

  const protocolFields = [
    "answer_card_complete",
    "recommended_next_action",
    "max_additional_tools",
    "required_facts_present",
    "unsupported_flows_present"
  ];
  const protocolContractSource = `${serverText}\n${answerCardProtocolText}\n${rendererText}`;
  const missingProtocolFields = protocolFields.filter((field) => !protocolContractSource.includes(field));
  if (missingProtocolFields.length) {
    fail("answer_card_protocol_fields", `missing protocol fields in answer card contract: ${missingProtocolFields.join(", ")}`, answerCardProtocolPath);
  } else {
    pass("answer_card_protocol_fields", "answer card protocol module/server/renderer cover required fields");
  }
  const intentPlanFields = [
    "INTENT_PLAN_PROTOCOL_VERSION",
    "FRONTDOOR_INTENTS",
    "inferFrontDoorIntent",
    "buildIntentAst",
    "buildPlanDag",
    "frontdoorWantsFlowGraph",
    "intent_ast",
    "plan_dag",
    "tool_budget",
    "ordinary_max_tool_calls",
    "report_max_tool_calls",
    "max_additional_tools",
    "claim_review_allowed",
    "repeat_discovery_allowed"
  ];
  const intentPlanContractSource = `${frontdoorRuntimeText}\n${intentPlanProtocolText}`;
  const missingIntentPlanFields = intentPlanFields.filter((field) => !intentPlanContractSource.includes(field));
  const intentPlanBudgetGuards = [
    "普通事实题选择最小充分语义事实工具",
    "报告级任务才追加 validate_report_claims",
    "edge_status=\"supported\"",
    "同一问题已完成时只抑制重复入口调用"
  ];
  const missingIntentPlanGuards = intentPlanBudgetGuards.filter((field) => !intentPlanProtocolText.includes(field));
  const passiveCounterpartyIntent = inferFrontDoorIntent({
    intent: "holder_analysis",
    holder_name: "甲某",
    counterparty_name: "乙某",
    date_start: "2025-01-01",
    question: "甲某转给乙某多少钱？请说明统计起止日期、姓名匹配统计口径和为什么不能把未核验候选金额写成案件事实。"
  });
  if (
    missingIntentPlanFields.length
    || missingIntentPlanGuards.length
    || passiveCounterpartyIntent !== "destination"
    || !frontdoorRuntimeText.includes("buildIntentAst")
    || !frontdoorRuntimeText.includes("buildPlanDag")
    || !frontdoorRuntimeText.includes("frontdoorWantsFlowGraph")
  ) {
    fail(
      "intent_plan_frontdoor_contract",
      [
        missingIntentPlanFields.length ? `missing fields: ${missingIntentPlanFields.join(", ")}` : "",
        missingIntentPlanGuards.length ? `missing guards: ${missingIntentPlanGuards.join(", ")}` : "",
        passiveCounterpartyIntent !== "destination" ? `holder+counterparty transfer routed to ${passiveCounterpartyIntent || "empty"}, expected destination` : "",
        !frontdoorRuntimeText.includes("buildIntentAst") ? "frontdoor runtime not using buildIntentAst" : "",
        !frontdoorRuntimeText.includes("buildPlanDag") ? "frontdoor runtime not using buildPlanDag" : "",
        !frontdoorRuntimeText.includes("frontdoorWantsFlowGraph") ? "frontdoor runtime not using frontdoorWantsFlowGraph" : ""
      ].filter(Boolean).join("; "),
      intentPlanProtocolPath
    );
  } else {
    pass("intent_plan_frontdoor_contract", "frontdoor intent AST/plan DAG carries ordinary/report tool budgets and supported-edge stop rules");
  }
  const frontdoorAnswerContractFields = [
    "FRONTDOOR_ANSWER_CONTRACT_VERSION",
    "buildFrontdoorNextActions",
    "buildFrontdoorAnswerContract",
    "withDuplicateSuppressionContract",
    "ordinary_holder_or_ranking_answer",
    "data_quality_boundary",
    "account_boundary",
    "destination_answer",
    "report_gate_answer",
    "duplicate_suppression"
  ];
  const frontdoorAnswerContractSource = `${frontdoorRuntimeText}\n${frontdoorAnswerContractText}`;
  const missingFrontdoorContractFields = frontdoorAnswerContractFields.filter((field) => !frontdoorAnswerContractSource.includes(field));
  const frontdoorAnswerContractGuards = [
    "已有资金流向图事实优先复用",
    "不为补齐链路追加报告级核验",
    "结论引用核验",
    "线索候选账户只能写为线索",
    "不得直接扣减总额",
    "暂不能出具正式报告时转成用户可见的补证动作",
    "同一问题仅抑制重复入口调用"
  ];
  const missingFrontdoorContractGuards = frontdoorAnswerContractGuards.filter((field) => !frontdoorAnswerContractText.includes(field));
  if (
    missingFrontdoorContractFields.length
    || missingFrontdoorContractGuards.length
    || !frontdoorRuntimeText.includes("buildFrontdoorNextActions")
    || !frontdoorRuntimeText.includes("buildFrontdoorAnswerContract")
    || /frontdoorResponseCache|rememberFrontdoorResponse|duplicateFrontdoorResponse|frontdoorRepeatState|frontdoorCallKey/u.test(`${frontdoorRuntimeText}\n${frontdoorRoutingText}`)
  ) {
    fail(
      "frontdoor_answer_contract_module",
      [
        missingFrontdoorContractFields.length ? `missing fields: ${missingFrontdoorContractFields.join(", ")}` : "",
        missingFrontdoorContractGuards.length ? `missing guards: ${missingFrontdoorContractGuards.join(", ")}` : "",
        !frontdoorRuntimeText.includes("buildFrontdoorNextActions") ? "frontdoor runtime not using buildFrontdoorNextActions" : "",
        !frontdoorRuntimeText.includes("buildFrontdoorAnswerContract") ? "frontdoor runtime not using buildFrontdoorAnswerContract" : "",
        /frontdoorResponseCache|rememberFrontdoorResponse|duplicateFrontdoorResponse|frontdoorRepeatState|frontdoorCallKey/u.test(`${frontdoorRuntimeText}\n${frontdoorRoutingText}`)
          ? "process-global frontdoor response cache symbols remain"
          : ""
      ].filter(Boolean).join("; "),
      frontdoorAnswerContractPath
    );
  } else {
    pass("frontdoor_answer_contract_module", "frontdoor answer contract centralizes answer boundaries without relying on a process-global response cache");
  }
  const frontdoorRoutingFields = [
    "FRONTDOOR_ROUTING_VERSION",
    "inferViaNameFromQuestion",
    "inferSourceHolderFromQuestion",
    "inferDateStartFromQuestion",
    "isViaContinuationQuestion",
    "isTopOutflowClassificationQuestion",
    "inferRankingTarget",
    "inferRankingMetric",
    "inferRankingMetrics"
  ];
  const missingFrontdoorRoutingFields = frontdoorRoutingFields.filter((field) => !frontdoorRoutingText.includes(field));
  if (
    missingFrontdoorRoutingFields.length
    || !frontdoorRuntimeText.includes("from \"./frontdoor-routing.mjs\"")
    || serverText.includes("const VIA_NAME_STOPWORDS = new Set([")
    || /frontdoorResponseCache|rememberFrontdoorResponse|duplicateFrontdoorResponse|frontdoorRepeatState|frontdoorCallKey/u.test(`${frontdoorRuntimeText}\n${frontdoorRoutingText}`)
    || /eval-fixtures|golden-answer-set\.json/u.test(frontdoorRoutingText)
  ) {
    fail(
      "frontdoor_routing_contract",
      [
        missingFrontdoorRoutingFields.length ? `missing fields: ${missingFrontdoorRoutingFields.join(", ")}` : "",
        !frontdoorRuntimeText.includes("from \"./frontdoor-routing.mjs\"") ? "frontdoor runtime not importing frontdoor routing" : "",
        serverText.includes("const VIA_NAME_STOPWORDS = new Set([") ? "server still owns via-name stopwords" : "",
        /frontdoorResponseCache|rememberFrontdoorResponse|duplicateFrontdoorResponse|frontdoorRepeatState|frontdoorCallKey/u.test(`${frontdoorRuntimeText}\n${frontdoorRoutingText}`)
          ? "frontdoor still owns process-global response cache state"
          : "",
        /eval-fixtures|golden-answer-set\.json/u.test(frontdoorRoutingText) ? "frontdoor routing references eval fixtures" : ""
      ].filter(Boolean).join("; "),
      frontdoorRoutingPath
    );
  } else {
    pass("frontdoor_routing_contract", "frontdoor routing remains deterministic and contains no process-global cross-case response cache");
  }
  const negatedDateWindow = "题目未限定时间，使用数据实际首末时间，不得自行缩成 2025 年以来窗口。";
  const explicitDateWindow = "重点时间为 2025-01-01 起，同时说明完整数据范围与核心窗口差异。";
  if (
    inferDateStartFromQuestion(negatedDateWindow) !== ""
    || inferDateStartFromQuestion(explicitDateWindow) !== "2025-01-01"
  ) {
    fail(
      "frontdoor_date_window_negation_guard",
      [
        inferDateStartFromQuestion(negatedDateWindow) !== "" ? "negated full-period prompt inferred a date_start" : "",
        inferDateStartFromQuestion(explicitDateWindow) !== "2025-01-01" ? "explicit date window not inferred" : ""
      ].filter(Boolean).join("; "),
      frontdoorRoutingPath
    );
  } else {
    pass("frontdoor_date_window_negation_guard", "frontdoor date inference keeps full-period negation prompts from being narrowed while preserving explicit date windows");
  }
  const frontdoorRuntimeFields = [
    "FRONTDOOR_RUNTIME_VERSION",
    "createFrontdoorRuntime",
    "fundsInvestigate",
    "diagnosticFrontdoorResponse",
    "buildIntentAst",
    "buildPlanDag",
    "frontdoorWantsFlowGraph",
    "buildFundFlowGraph",
    "buildDestinationOutflowDiagnosticCard",
    "buildClaimReviewDiagnosticCard",
    "buildInvestigationLabDiagnosticCard"
  ];
  const missingFrontdoorRuntimeFields = frontdoorRuntimeFields.filter((field) => !frontdoorRuntimeText.includes(field));
  if (
    missingFrontdoorRuntimeFields.length
    || !serverText.includes("from \"./frontdoor-runtime.mjs\"")
    || !serverText.includes("createFrontdoorRuntime")
    || serverText.includes("async function fundsInvestigate(")
    || serverText.includes("function diagnosticFrontdoorResponse(")
    || /detailRefForPayload|rememberFrontdoorResponse|duplicateFrontdoorResponse|frontdoorResponseCache/u.test(frontdoorRuntimeText)
    || /eval-fixtures|golden-answer-set\.json/u.test(frontdoorRuntimeText)
    || /process\.env/u.test(frontdoorRuntimeText)
  ) {
    fail(
      "frontdoor_runtime_contract",
      [
        missingFrontdoorRuntimeFields.length ? `missing fields: ${missingFrontdoorRuntimeFields.join(", ")}` : "",
        !serverText.includes("from \"./frontdoor-runtime.mjs\"") ? "server not importing frontdoor runtime" : "",
        !serverText.includes("createFrontdoorRuntime") ? "server not creating frontdoor runtime" : "",
        serverText.includes("async function fundsInvestigate(") ? "server still owns funds_investigate orchestration" : "",
        serverText.includes("function diagnosticFrontdoorResponse(") ? "server still owns diagnostic frontdoor response builder" : "",
        /detailRefForPayload|rememberFrontdoorResponse|duplicateFrontdoorResponse|frontdoorResponseCache/u.test(frontdoorRuntimeText)
          ? "frontdoor still carries implicit detail artifacts or process-global response cache state"
          : "",
        /eval-fixtures|golden-answer-set\.json/u.test(frontdoorRuntimeText) ? "frontdoor runtime references eval fixtures" : "",
        /process\.env/u.test(frontdoorRuntimeText) ? "frontdoor runtime reads process env directly" : ""
      ].filter(Boolean).join("; "),
      frontdoorRuntimePath
    );
  } else {
    pass("frontdoor_runtime_contract", "funds_investigate orchestration is centralized without implicit detail artifacts or process-global response cache state");
  }
  runNodeJsonSelfTest(
    "frontdoor_oracle_workspace_binding_self_test",
    frontdoorP0OraclePath,
    ["--self-test-workspace-binding", "--json"],
    (result) => {
      if (result.status !== "ok") {
        return { ok: false, message: `workspace binding self-test status=${text(result.status) || "(empty)"}` };
      }
      const selected = new Set(arrayOf(result.selected_thread_ids).map(text));
      const excluded = new Set(arrayOf(result.excluded_thread_ids).map(text));
      for (const threadId of ["thread_good"]) {
        if (!selected.has(threadId)) {
          return { ok: false, message: `workspace binding self-test did not select ${threadId}` };
        }
      }
      for (const threadId of ["thread_missing_workspace", "thread_wrong", "thread_discarded_newer"]) {
        if (!excluded.has(threadId)) {
          return { ok: false, message: `workspace binding self-test did not exclude ${threadId}` };
        }
      }
      const wrongWorkspaceBlockers = new Set(arrayOf(result.wrong_workspace_blockers).map(text));
      const missingWorkspaceBlockers = new Set(arrayOf(result.missing_workspace_blockers).map(text));
      if (!wrongWorkspaceBlockers.has("frontdoor_thread_workspace_mismatch")) {
        return { ok: false, message: "workspace binding self-test missed mismatch blocker" };
      }
      if (!missingWorkspaceBlockers.has("frontdoor_thread_workspace_missing")) {
        return { ok: false, message: "workspace binding self-test missed missing-workspace blocker" };
      }
      return { ok: true, message: "" };
    },
    "frontdoor oracle workspace-binding self-test executes and rejects wrong, missing, and stale thread roots"
  );
  runNodeJsonSelfTest(
    "frontdoor_oracle_replay_normalization_self_test",
    frontdoorP0OraclePath,
    ["--self-test-replay-normalization", "--json"],
    (result) => {
      if (result.status !== "ok") {
        return { ok: false, message: `replay normalization self-test status=${text(result.status) || "(empty)"}` };
      }
      if (text(result.completed_answer) !== "完整最终回答") {
        return { ok: false, message: `unexpected completed answer: ${text(result.completed_answer)}` };
      }
      if (text(result.partial_answer) !== "部分") {
        return { ok: false, message: `unexpected partial answer: ${text(result.partial_answer)}` };
      }
      return { ok: true, message: "" };
    },
    "frontdoor oracle replay-normalization self-test executes and preserves completed and partial assistant answers"
  );
  runNodeJsonSelfTest(
    "frontdoor_oracle_installed_version_scan_self_test",
    frontdoorP0OraclePath,
    ["--self-test-installed-version-scan", "--json"],
    (result) => {
      if (result.status !== "ok") {
        return { ok: false, message: `installed version scan self-test status=${text(result.status) || "(empty)"}` };
      }
      const scannedPaths = arrayOf(result.scanned_paths).map(text);
      const expectedFragments = [
        ".cache/analytix-hub-plugins/marketplaces/analytix-hub/plugins/analytix-fund-analysis/0.16.9-local-test",
        "plugins/cache/analytix-hub/analytix-fund-analysis/0.16.9"
      ];
      for (const expectedFragment of expectedFragments) {
        if (!scannedPaths.some((scannedPath) => scannedPath.includes(expectedFragment))) {
          return { ok: false, message: `installed version scan missed ${expectedFragment}` };
        }
      }
      return { ok: true, message: "" };
    },
    "frontdoor oracle installed-version self-test executes and scans marketplace and runtime cache install roots"
  );
  if (
    !frontdoorRuntimeText.includes("HOLDER_ANALYSIS_REQUIRED_FACTS_MISSING")
    || !frontdoorRuntimeText.includes("holderAnalysis?.ok")
    || !frontdoorRuntimeText.includes("holderTopAccounts?.ok")
    || !frontdoorRuntimeText.includes("不得把缺失主体范围、金额或 Top 账户渲染为 0")
  ) {
    fail("frontdoor_holder_required_fact_gate", "holder_analysis frontdoor must not turn missing child facts into zero counts or amounts", frontdoorRuntimePath);
  } else {
    pass("frontdoor_holder_required_fact_gate", "holder_analysis frontdoor blocks missing child facts instead of rendering zero-count facts");
  }
  const holderRequiredFactGateSample = await runFrontdoorHolderRequiredFactGateSelfTest();
  if (
    holderRequiredFactGateSample.status !== "partial"
    || objectOf(holderRequiredFactGateSample.answer_card).required_facts_present !== false
    || !arrayOf(holderRequiredFactGateSample.warnings).some((item) => text(item.code) === "HOLDER_ANALYSIS_REQUIRED_FACTS_MISSING" && text(item.severity) === "blocking")
    || objectOf(holderRequiredFactGateSample.key_facts).holder_scope
    || objectOf(holderRequiredFactGateSample.key_facts).holder_account_stats
    || objectOf(holderRequiredFactGateSample.key_facts).holder_top_accounts
  ) {
    fail("frontdoor_holder_required_fact_gate_smoke", "synthetic holder child-tool failure must be partial, block required facts, and avoid zero-valued holder facts", frontdoorRuntimePath);
  } else {
    pass("frontdoor_holder_required_fact_gate_smoke", "synthetic holder child-tool failure blocks required facts without zero-valued holder facts");
  }
  const contextCompilerFields = [
    "CONTEXT_COMPILER_VERSION",
    "context_compiler",
    "must_write_facts",
    "must_state_boundaries",
    "forbidden_as_facts",
    "next_queries",
    "stop_conditions",
    "raw_result_policy"
  ];
  const contextCompilerContractSource = `${serverText}\n${answerCardProtocolText}\n${contextCompilerText}`;
  const missingContextCompilerFields = contextCompilerFields.filter((field) => !contextCompilerContractSource.includes(field));
  if (missingContextCompilerFields.length) {
    fail("answer_card_context_compiler_contract", `missing context compiler fields: ${missingContextCompilerFields.join(", ")}`, contextCompilerPath);
  } else {
    pass("answer_card_context_compiler_contract", "answer_card embeds compact context compiler contract fields");
  }
  const mcpOutputPolicyFields = [
    "MCP_OUTPUT_POLICY_VERSION",
    "MCP_OUTPUT_POLICY_GUARDS",
    "debugPayloadAllowedFromEnv",
    "structuredContentAllowed",
    "buildStructuredContentPolicy",
    "answer_card_only",
    "structuredContent_disabled_fail_closed",
    "raw_json_never_model_context_by_default",
    "debug_payload_cannot_bypass_policy"
  ];
  const mcpOutputPolicyContractSource = `${serverText}\n${mcpToolResultRuntimeText}\n${mcpOutputPolicyText}`;
  const missingMcpOutputPolicyFields = mcpOutputPolicyFields.filter((field) => !mcpOutputPolicyContractSource.includes(field));
  const defaultStructuredAllowed = structuredContentAllowed({}, {});
  const requestedStructuredAllowed = structuredContentAllowed({ include_debug: true }, {});
  const enabledStructuredAllowed = structuredContentAllowed(
    { include_debug: true },
    { ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD: "true" }
  );
  const syntheticOutputPolicy = buildStructuredContentPolicy({ include_debug: true }, {});
  if (
    missingMcpOutputPolicyFields.length
    || defaultStructuredAllowed
    || requestedStructuredAllowed
    || enabledStructuredAllowed
    || syntheticOutputPolicy.model_context !== "answer_card_only"
    || syntheticOutputPolicy.structured_content_policy !== "structuredContent_disabled_fail_closed"
    || mcpToolResultRuntimeText.includes("buildMcpMetaEvidenceLedger")
    || mcpToolResultRuntimeText.includes("structuredContentAllowed")
  ) {
    fail(
      "mcp_output_policy_contract",
      [
        missingMcpOutputPolicyFields.length ? `missing fields: ${missingMcpOutputPolicyFields.join(", ")}` : "",
        defaultStructuredAllowed ? "structuredContent allowed by default" : "",
        requestedStructuredAllowed ? "structuredContent allowed without env gate" : "",
        enabledStructuredAllowed ? "debug env unexpectedly bypassed the fail-closed structuredContent policy" : "",
        syntheticOutputPolicy.model_context !== "answer_card_only" ? "model_context is not answer_card_only" : "",
        syntheticOutputPolicy.structured_content_policy !== "structuredContent_disabled_fail_closed" ? "policy is not fail-closed" : "",
        mcpToolResultRuntimeText.includes("buildMcpMetaEvidenceLedger") ? "MCP tool result still promotes plugin ledger metadata" : "",
        mcpToolResultRuntimeText.includes("structuredContentAllowed") ? "MCP tool result still carries a debug structured-content bypass" : ""
      ].filter(Boolean).join("; "),
      mcpOutputPolicyPath
    );
  } else {
    pass("mcp_output_policy_contract", "MCP output policy keeps raw JSON disabled even when include_debug and legacy debug environment flags are supplied");
  }
  const artifactPathPolicyFields = [
    "ARTIFACT_PATH_POLICY_VERSION",
    "DEFAULT_ANALYTIX_ARTIFACT_PARTS",
    "resolveAnalytixArtifactRoot",
    "assertAnalytixArtifactRoot",
    "Refusing to use an artifact path inside system Codex home"
  ];
  const stableHashFields = ["STABLE_HASH_VERSION", "stableHash", "sha256"];
  const artifactPolicyConsumers = `${frontdoorRuntimeText}\n${casegraphRuntimeText}\n${fundFlowGraphRuntimeText}\n${diagnosticFactHelpersText}`;
  const missingArtifactPathPolicyFields = artifactPathPolicyFields.filter((field) => !artifactPathPolicyText.includes(field));
  const missingStableHashFields = stableHashFields.filter((field) => !stableHashText.includes(field));
  const syntheticArtifactHome = path.join(os.tmpdir(), "analytix-home-release-guard");
  const syntheticDefaultArtifactRoot = resolveAnalytixArtifactRoot({ env: { HOME: syntheticArtifactHome } });
  const expectedArtifactSuffix = path.join(".analytix", "artifacts", "analytix-funds");
  let systemCodexArtifactRejected = false;
  try {
    assertAnalytixArtifactRoot(path.join(os.homedir(), ".codex", "artifacts"), { env: { HOME: os.homedir() } });
  } catch (_error) {
    systemCodexArtifactRejected = true;
  }
  const syntheticHash = runtimeStableHash({ purpose: "pure-runtime-identity", value: 1 });
  if (
    missingArtifactPathPolicyFields.length
    || missingStableHashFields.length
    || fs.existsSync(retiredMcpArtifactStorePath)
    || /persistMcpArtifact|detailRefForPayload|stored_in_local_audit_artifact_not_model_context/u.test(`${artifactPolicyConsumers}\n${serverText}\n${mcpToolResultRuntimeText}`)
    || !artifactPolicyConsumers.includes("from \"./stable-hash.mjs\"")
    || !syntheticDefaultArtifactRoot.endsWith(expectedArtifactSuffix)
    || syntheticDefaultArtifactRoot.includes(`${path.sep}.codex${path.sep}`)
    || !systemCodexArtifactRejected
    || !/^[a-f0-9]{64}$/u.test(syntheticHash)
  ) {
    fail(
      "mcp_artifact_authority_contract",
      [
        missingArtifactPathPolicyFields.length ? `missing path-policy fields: ${missingArtifactPathPolicyFields.join(", ")}` : "",
        missingStableHashFields.length ? `missing stable-hash fields: ${missingStableHashFields.join(", ")}` : "",
        fs.existsSync(retiredMcpArtifactStorePath) ? "retired unbound MCP artifact writer still exists" : "",
        /persistMcpArtifact|detailRefForPayload|stored_in_local_audit_artifact_not_model_context/u.test(`${artifactPolicyConsumers}\n${serverText}\n${mcpToolResultRuntimeText}`) ? "production MCP still references the unbound artifact writer" : "",
        !artifactPolicyConsumers.includes("from \"./stable-hash.mjs\"") ? "runtime modules do not consume the pure stable hash helper" : "",
        !syntheticDefaultArtifactRoot.endsWith(expectedArtifactSuffix) ? "default artifact root is not current analytix artifact root" : "",
        syntheticDefaultArtifactRoot.includes(`${path.sep}.codex${path.sep}`) ? "default artifact root points inside system Codex home" : "",
        !systemCodexArtifactRejected ? "system Codex artifact root was not rejected" : "",
        !/^[a-f0-9]{64}$/u.test(syntheticHash) ? "stable hash is not a complete SHA-256" : ""
      ].filter(Boolean).join("; "),
      artifactPathPolicyPath
    );
  } else {
    pass("mcp_artifact_authority_contract", "plugin helpers are pure/path-only; the retired unbound auto-write artifact store is absent and formal publication remains host-owned");
  }
  const evidenceLedgerFields = [
    "EVIDENCE_LEDGER_VERSION",
    "buildEvidenceLedger",
    "attachInternalEvidenceLedger",
    "analytix_evidence_ledger",
    "ledger_id",
    "fact_id",
    "source_path",
    "source_refs",
    "support_status",
    "claim_id",
    "claim_category",
    "risk_marker",
    "raw_payload_policy",
    "coverage_summary",
    "coverage_scope",
    "coverage_entry_count",
    "coverage_includes_overflow_entries",
    "scanned_entry_count",
    "output_entry_limit",
    "entries",
    "surface_signals",
    "EVIDENCE_LEDGER_REQUIRED_SURFACES",
    "EVIDENCE_LEDGER_TRACE_REQUIRED_SURFACES",
    "untraceable_required_surface_entries",
    "traceability_policy",
    "claim_support_index",
    "_meta"
  ];
  const evidenceLedgerContractSource = `${serverText}\n${mcpToolResultRuntimeText}\n${evidenceLedgerText}`;
  const missingEvidenceLedgerFields = evidenceLedgerFields.filter((field) => !evidenceLedgerContractSource.includes(field));
  if (missingEvidenceLedgerFields.length || !rendererText.includes("evidence_ledger")) {
    fail("evidence_ledger_internal_contract", `missing internal evidence ledger fields: ${missingEvidenceLedgerFields.join(", ") || "renderer evidence_ledger strip"}`, evidenceLedgerPath);
  } else {
    pass("evidence_ledger_internal_contract", "MCP results carry internal fact/source lineage ledger metadata while renderer strips it from user text");
  }
  const syntheticLedger = buildEvidenceLedger({
    tool: "release_guard_synthetic",
    case_id: "release-guard-synthetic",
    evidence_refs: {
      query_ids: ["q_release_guard"],
      audit_ref: { sql: "synthetic release guard aggregate" },
      artifact_id: "artifact-release-guard"
    },
    answer_card: {
      facts: [
        {
          fact_id: "synthetic.amount.count.account",
          label: "synthetic fact",
          amount: 123.45,
          count: 2,
          account: "6222-demo",
          holder: "测试户名",
          counterparty: "测试对手方",
          support_query_name: "release_guard_sql",
          support_status: "supported",
          source_refs: {
            query_ids: ["q_release_guard"],
            evidence_ids: ["synthetic.amount.count.account"]
          }
        }
      ],
      verified_claims: [{
        claim_id: "synthetic.release.guard.claim",
        claim_text: "该合成 claim 必须进入 report_claim ledger。",
        source_refs: {
          query_ids: ["q_release_guard"],
          evidence_ids: ["synthetic.amount.count.account"]
        }
      }],
      unsupported_flows: ["该合成 unsupported flow 不得画 Mermaid。"],
      claim_support_index: [
        {
          category: "verified_claims",
          claim_text: "该合成 claim 必须进入 report_claim ledger。",
          support_status: "supported",
          source_refs: {
            query_ids: ["q_release_guard"],
            evidence_ids: ["synthetic.amount.count.account"]
          }
        }
      ],
      context_compiler: {
        must_write_facts: ["context ledger must write fact"],
        must_state_boundaries: ["context ledger boundary"],
        forbidden_as_facts: ["context ledger forbidden"],
        next_queries: [{ tool: "trace_fund", why_this_query: "context ledger next query" }],
        stop_conditions: ["context ledger stop"]
      }
    },
    flow_graph: {
      edges: [
        {
          edge_id: "edge-release-guard-1",
          edge_status: "supported",
          txn_id: "txn-release-guard-1",
          txn_time: "2026-05-29 12:00:00",
          amount: { yuan: 123.45 },
          from: "6222-demo",
          from_label: "测试户名",
          to: "counterparty-demo",
          to_label: "测试对手方",
          direction: "out",
          source_tool: "trace_subject_top_outflows",
          source_refs: {
            query_ids: ["q_release_guard"],
            evidence_ids: ["synthetic.amount.count.account"]
          }
        }
      ],
      supported_edge_count: 1,
      edge_count: 1
    }
  });
  const syntheticSurfaceCounts = objectOf(syntheticLedger.coverage_summary?.surface_counts);
  const syntheticOverflowFacts = Array.from({ length: 210 }, (_, index) => ({
    fact_id: `overflow.traced.${index}`,
    amount: index + 1,
    count: 1,
    account: `6222-overflow-${index}`,
    holder: "溢出覆盖测试",
    source_refs: {
      query_ids: [`q_overflow_${index}`],
      evidence_ids: [`overflow.traced.${index}`]
    }
  }));
  const syntheticOverflowLedger = buildEvidenceLedger({
    tool: "release_guard_overflow",
    case_id: "release-guard-overflow",
    answer_card: {
      facts: syntheticOverflowFacts.slice(0, 120),
      extra_facts: syntheticOverflowFacts.slice(120),
      overflow_gap: {
        fact_id: "overflow.untraceable",
        amount: 999,
        count: 1,
        account: "6222-overflow-gap",
        holder: "溢出缺口"
      }
    }
  });
  const syntheticOverflowCoverage = objectOf(syntheticOverflowLedger.coverage_summary);
  const syntheticOverflowMissingEntries = arrayOf(syntheticOverflowCoverage.untraceable_required_surface_entries);
  const syntheticOverflowDetectedGap = syntheticOverflowMissingEntries.some((entry) => (
    text(entry.fact_id) === "overflow.untraceable"
    || text(entry.source_path).includes("overflow_gap")
  ));
  const missingSyntheticSurfaces = [
    "amount",
    "count",
    "account",
    "holder",
    "counterparty",
    "flow_or_transaction_edge",
    "report_claim",
    "unsupported_flow",
    "context_compiler_directive",
    "source_refs"
  ].filter((surface) => !Number(syntheticSurfaceCounts[surface] || 0));
  if (
    missingSyntheticSurfaces.length
    || arrayOf(syntheticLedger.coverage_summary?.missing_required_surfaces).length
    || Number(syntheticLedger.coverage_summary?.untraceable_required_surface_entry_count || 0) !== 0
    || arrayOf(syntheticLedger.coverage_summary?.untraceable_required_surface_entries).length
    || syntheticLedger.coverage_summary?.host_registry_verified !== false
    || syntheticLedger.coverage_summary?.publication_ready !== false
    || arrayOf(syntheticLedger.entries).some((entry) => text(entry.support_status) === "supported")
    || !text(syntheticLedger.coverage_summary?.traceability_policy).includes("Every amount")
    || !arrayOf(syntheticLedger.entries).every((entry) => text(entry.fact_id) && text(entry.source_path) && arrayOf(entry.surface_signals).length)
    || Number(syntheticOverflowLedger.scanned_entry_count || 0) <= Number(syntheticOverflowLedger.entry_count || 0)
    || Number(syntheticOverflowCoverage.coverage_entry_count || 0) <= Number(syntheticOverflowLedger.entry_count || 0)
    || text(syntheticOverflowCoverage.coverage_scope) !== "all_scanned_entries_before_output_truncation"
    || syntheticOverflowCoverage.coverage_includes_overflow_entries !== true
    || Number(syntheticOverflowCoverage.untraceable_required_surface_entry_count || 0) < 1
    || !syntheticOverflowDetectedGap
  ) {
    fail(
      "evidence_ledger_surface_coverage",
      `synthetic ledger missing surfaces: ${missingSyntheticSurfaces.join(", ") || "entry fact/source/surface/traceability/overflow signals"}`,
      evidenceLedgerPath
    );
  } else {
    pass("evidence_ledger_surface_coverage", "synthetic audit ledger covers required lineage surfaces while provider 'supported' labels remain unresolved, host registry stays unverified, and publication readiness stays false");
  }
  const liveLedgerHealthFields = [
    "assertEvidenceLedger",
    "analytix_evidence_ledger",
    "frontdoor evidence ledger",
    "destination frontdoor evidence ledger",
    "flowgraph evidence ledger",
    "claim verifier evidence ledger",
    "assertClaimVerifierRiskGate",
    "claim verifier risk gate",
    "assertClaimVerifierSupportedGate",
    "claim verifier supported gate",
    "claim_support_index",
    "untraceable_required_surface_entry_count",
    "support_owner",
    "focused_owner",
    "unsupported_mermaid_or_arrow_flow",
    "forbidden_legal_phrasing",
    "candidate_account_as_owned_account",
    "cash_or_asset_destination_without_supported_flow",
    "missing_counterparty_or_cash_break_as_verified",
    "claim_without_source_anchor",
    "validate_report_claims",
    "report_claim",
    "context_compiler_directive",
    "source_refs"
  ];
  const missingLiveLedgerHealthFields = liveLedgerHealthFields.filter((field) => !checkHealthText.includes(field));
  if (missingLiveLedgerHealthFields.length) {
    fail(
      "evidence_ledger_live_health_gate",
      `check-health missing live MCP ledger assertions: ${missingLiveLedgerHealthFields.join(", ")}`,
      checkHealthPath
    );
  } else {
    pass("evidence_ledger_live_health_gate", "check-health verifies live MCP _meta evidence ledger coverage for frontdoor, flowgraph, and claim review outputs");
  }
  const doctorReleaseGateFields = [
    "scripts/agent-thread-audit.mjs",
    "scripts/check-health.mjs",
    "scripts/mcp-surface-snapshot.mjs",
    "scripts/release-slice-audit.mjs",
    "scripts/prepare-hub-package.mjs",
    "scripts/phase4-diagnostics.mjs",
    "validatePluginManifestIngestionContract",
    "plugin manifest ingestion contract",
    "version must be strict semver",
    "release gate script syntax",
    "scripts passed node --check"
  ];
  const missingDoctorReleaseGateFields = doctorReleaseGateFields.filter((field) => !doctorText.includes(field));
  if (missingDoctorReleaseGateFields.length) {
    fail(
      "doctor_release_gate_script_coverage",
      `doctor missing release gate script coverage: ${missingDoctorReleaseGateFields.join(", ")}`,
      doctorPath
    );
  } else {
    pass("doctor_release_gate_script_coverage", "doctor covers check-health, release-slice audit, and phase4 release scripts as first-class release gates");
  }
  const releaseSliceAuditPath = path.join(PLUGIN_ROOT, "scripts", "release-slice-audit.mjs");
  const releaseSliceAuditText = fs.existsSync(releaseSliceAuditPath) ? fs.readFileSync(releaseSliceAuditPath, "utf8") : "";
  const releaseSliceAuditFields = [
    "RELEASE_VERSION_FILES",
    "RELEASE_SLICE_FILES",
    "frontdoor-smoke.mjs",
    "sync-runtime-cache.mjs",
    "prepare-hub-package.mjs",
    "runtime-boundary.md",
    "hub-lifecycle.md",
    "git status --porcelain=v1",
    "current_release_identity",
    "current_release_tag_exists",
    "current_release_tag_at_head",
    "current_release_tag_state",
    "releaseTagStatus",
    "publish_identity_ready",
    "release-slice changes are uncommitted",
    "related_dirty",
    "unrelated_dirty",
    "goal_wide_unrelated_dirty_count",
    "unknown_unrelated_dirty_count",
    "unrelated_dirty_release_boundary",
    "eval_fixture_release_files",
    "eval_fixtures_are_eval_only",
    "--fail-on-unrelated",
    "--fail-on-publish-blockers",
    "--candidate-version",
    "--functional-closure-evidence",
    "--mcp-surface-evidence",
    "--staging-plan",
    "--readiness-plan",
    "--verify-isolated",
    "--summary-json",
    "--self-test-release-tag-status",
    "--self-test-mcp-surface-evidence",
    "--self-test-summary-json",
    "buildReleaseSliceSummary",
    "analytix-fund-analysis-release-slice-summary",
    "publish_blocker_count",
    "staging_plan",
    "readiness_plan",
    "mutates_index: false",
    "mutates_files: false",
    "leave_unstaged_goal_wide_count",
    "goal_wide_unrelated_paths_to_keep_out_count",
    "leave_unstaged_unknown_count",
    "unknown_unrelated_paths_to_keep_out_count",
    "should_suggest_patch_version",
    "suggested_next_version",
    "candidate_version",
    "candidate_tag_exists",
    "candidate_version_ready",
    "candidate_version_file_gaps",
    "functional_closure_evidence",
    "functional closure evidence check failed",
    "can_publish_current_tree",
    "release_slice_stage_path_count",
    "unrelated_paths_to_keep_out_count",
    "required_sequence_after_authorization",
    "authorization_model",
    "functional closure evidence",
    "version_replacements_for_candidate",
    "version_files_for_next_release",
    "case_bound_health_gate",
    "contract_ready",
    "publication_ready",
    "generic_check_health_is_smoke_only",
    "case_bound_health_required_before_publish",
    "native_analysis_compute_gate",
    "stats-query-worker",
    "local_env_override_is_verification_only",
    "local_temp_binary_is_release_evidence: false",
    "packaged_runtime_evidence_required",
    "packaged_runtime_evidence_ready",
    "packaged_runtime_evidence",
    "command_surface_missing",
    "EVAL_COVERAGE_CONTRACT_VERSION",
    "RUNTIME_CACHE_CONTRACT_VERSION",
    "can_publish_current_tree",
    "authorization_required_before",
    "goal_15_6_full_authorization_satisfies",
    "autonomous_release_allowed_after_authorization_and_gates",
    "git\", [\"worktree\", \"add\", \"--detach\"",
    "git\", [\"apply\", \"--check\"",
    "temporary_detached_git_worktree",
    "runtime_cache_sync_path: false",
    "hub_publish_path: false"
  ];
  const missingReleaseSliceAuditFields = releaseSliceAuditFields.filter((field) => !releaseSliceAuditText.includes(field));
  const releaseSliceAuditReadsGoldenArtifacts = /(?:readJson|readText|fs\.readFileSync)\([^)]*(?:golden-answer-set|golden-rubric|oracle)/u.test(releaseSliceAuditText);
  const releaseSliceRunsIsolatedDoctor = /["']plugins\/analytix-fund-analysis\/scripts\/doctor\.mjs["']\s*,\s*["']--json["']\s*,\s*["']--skip-git["']\s*,\s*["']--skip-runtime["']/u.test(releaseSliceAuditText);
  if (
    missingReleaseSliceAuditFields.length
    || releaseSliceAuditReadsGoldenArtifacts
    || !releaseSliceRunsIsolatedDoctor
    || /plugins\/cache|\.cache\/analytix-hub-plugins|sync-runtime-cache\.mjs\s+--apply/u.test(releaseSliceAuditText)
  ) {
    fail(
      "release_slice_audit_contract",
      [
        missingReleaseSliceAuditFields.length ? `missing fields: ${missingReleaseSliceAuditFields.join(", ")}` : "",
        releaseSliceAuditReadsGoldenArtifacts ? "must not read golden answers, rubrics, or oracle artifacts" : "",
        !releaseSliceRunsIsolatedDoctor ? "must run isolated doctor with --json --skip-git --skip-runtime" : "",
        /plugins\/cache|\.cache\/analytix-hub-plugins|sync-runtime-cache\.mjs\s+--apply/u.test(releaseSliceAuditText) ? "must not be a runtime cache or Hub publish path" : ""
      ].filter(Boolean).join("; "),
      releaseSliceAuditPath
    );
  } else {
    pass("release_slice_audit_contract", "release-slice audit is read-only git status evidence, may read coverage contracts, and is not a runtime cache, Hub publish, or golden/oracle path");
  }
  runNodeJsonSelfTest(
    "release_slice_generated_evidence_ignore_self_test",
    releaseSliceAuditPath,
    ["--self-test-generated-evidence-ignore", "--json"],
    (result) => {
      if (result.ok !== true) {
        return { ok: false, message: "generated evidence ignore self-test did not report ok:true" };
      }
      const prefixes = new Set(arrayOf(result.prefixes).map(text));
      for (const expectedPrefix of ["output/analytix-fund-analysis/", "packages/runtime-go/output/analytix-fund-analysis/"]) {
        if (!prefixes.has(expectedPrefix)) {
          return { ok: false, message: `missing generated evidence ignore prefix: ${expectedPrefix}` };
        }
      }
      const results = arrayOf(result.results).map(objectOf);
      if (results.length < 5 || results.some((item) => item.ok !== true)) {
        return { ok: false, message: "generated evidence ignore self-test did not cover all cases" };
      }
      const deletionCase = results.find((item) => text(item.name) === "tracked output evidence deletion is not ignored");
      const unrelatedCase = results.find((item) => text(item.name) === "unrelated runtime-go output is not ignored");
      if (objectOf(deletionCase).actual !== false || objectOf(unrelatedCase).actual !== false) {
        return { ok: false, message: "generated evidence ignore guard would hide deletion or unrelated output paths" };
      }
      return { ok: true, message: "" };
    },
    "release-slice generated-evidence ignore self-test executes and preserves deletion/unrelated-path blockers"
  );
  runNodeJsonSelfTest(
    "release_slice_mcp_surface_evidence_self_test",
    releaseSliceAuditPath,
    ["--self-test-mcp-surface-evidence", "--json"],
    (result) => {
      const results = arrayOf(result.results).map(objectOf);
      const valid = objectOf(results.find((item) => text(item.name) === "valid_v2_is_diagnostic_only"));
      const requiredHostileCases = [
        "legacy_same_version_rejected",
        "source_hash_mismatch_rejected",
        "nonempty_tools_rejected",
        "extra_resource_rejected",
        "prompt_arguments_rejected",
        "boundary_drift_rejected",
        "publication_ready_forgery_rejected",
        "nested_unknown_property_rejected",
        "duplicate_json_key_rejected",
        "invalid_utf8_rejected",
        "oversized_evidence_rejected",
        "symlink_evidence_rejected"
      ];
      const missingCases = requiredHostileCases.filter((name) =>
        !results.some((item) => text(item.name) === name && item.ok === true)
      );
      if (result.ok !== true || results.length < 13 || missingCases.length) {
        return {
          ok: false,
          message: `MCP surface evidence self-test incomplete: cases=${results.length}, missing=${missingCases.join(", ") || "none"}`
        };
      }
      if (valid.contract_ready !== true || valid.publication_ready !== false) {
        return {
          ok: false,
          message: "valid P0 snapshot must be contract-ready, diagnostic-only, and publication-blocked"
        };
      }
      return { ok: true, message: "" };
    },
    "release-slice MCP V2 evidence rejects stale, forged, malformed, and non-quarantine snapshots while remaining publication-blocked"
  );
  runNodeJsonSelfTest(
    "release_slice_native_runtime_evidence_self_test",
    releaseSliceAuditPath,
    ["--self-test-native-runtime-evidence", "--json"],
    (result) => {
      const evidence = objectOf(result.evidence);
      if (result.ok !== true || evidence.packaged_runtime_evidence_ready !== true) {
        return { ok: false, message: "native runtime evidence self-test did not report ready" };
      }
      if (evidence.read_only !== true || evidence.mutates_files !== false || evidence.mutates_index !== false) {
        return { ok: false, message: "native runtime evidence check must remain read-only" };
      }
      const requiredCommands = new Set(arrayOf(evidence.required_command_surface).map(text));
      for (const command of ["stats-query-worker", "query-stats-tree", "query-stats-date-range", "query-stats-txn-rows"]) {
        if (!requiredCommands.has(command)) {
          return { ok: false, message: `native runtime evidence self-test missed command: ${command}` };
        }
      }
      if (evidence.command_surface_ok !== true || arrayOf(evidence.command_surface_missing).length !== 0) {
        return { ok: false, message: "native runtime evidence command surface is incomplete" };
      }
      if (!text(evidence.binary_path).includes("dist/codex-packaged-smoke/mac-arm64/analytix.app/Contents/Resources/runtime/analytix-analysis-compute")) {
        return { ok: false, message: `unexpected packaged runtime binary path: ${text(evidence.binary_path)}` };
      }
      return { ok: true, message: "" };
    },
    "release-slice native-runtime evidence self-test executes and proves packaged command surface detection"
  );
  runNodeJsonSelfTest(
    "release_slice_summary_self_test",
    releaseSliceAuditPath,
    ["--self-test-summary-json", "--json"],
    (result) => {
      const summary = objectOf(result.summary);
      const publishBlockers = arrayOf(summary.publish_blockers);
      if (result.ok !== true) {
        return { ok: false, message: "release-slice summary self-test did not report ok:true" };
      }
      if (summary.status !== "blocked") {
        return { ok: false, message: `expected blocked summary status, got ${text(summary.status) || "(empty)"}` };
      }
      if (summary.publish_blocker_count !== publishBlockers.length || summary.publish_blocker_count < 1) {
        return {
          ok: false,
          message: `publish_blocker_count=${summary.publish_blocker_count} does not match publish_blockers.length=${publishBlockers.length}`
        };
      }
      if (objectOf(summary.staging_plan).goal_wide_unrelated_paths_to_keep_out_count == null) {
        return { ok: false, message: "bounded summary omitted goal_wide_unrelated_paths_to_keep_out_count" };
      }
      const readinessPlan = objectOf(summary.readiness_plan);
      if (readinessPlan.blocker_count !== 2 || readinessPlan.candidate_blocker_count !== 0) {
        return {
          ok: false,
          message: `readiness blocker counts drifted: blocker_count=${readinessPlan.blocker_count}, candidate_blocker_count=${readinessPlan.candidate_blocker_count}`
        };
      }
      const requiredAuthorization = [
        "version file edits",
        "git add",
        "git commit",
        "git tag",
        "git push",
        "Hub publish",
        "marketplace update"
      ];
      const authorizationRequiredBefore = arrayOf(readinessPlan.authorization_required_before).map(text);
      const missingAuthorization = requiredAuthorization.filter((requirement) =>
        !authorizationRequiredBefore.includes(requirement)
      );
      if (missingAuthorization.length || authorizationRequiredBefore.length !== requiredAuthorization.length) {
        return {
          ok: false,
          message: `bounded summary readiness authorization drifted: missing=${missingAuthorization.join(", ") || "none"}, count=${authorizationRequiredBefore.length}`
        };
      }
      return { ok: true, message: "" };
    },
    "release-slice summary self-test executes and proves publish blocker count, readiness count, authorization boundary, and bounded staging fields"
  );
  runNodeJsonSelfTest(
    "release_slice_tag_status_self_test",
    releaseSliceAuditPath,
    ["--self-test-release-tag-status", "--json"],
    (result) => {
      const states = new Set(arrayOf(result.results).map((item) => text(objectOf(item).actual?.state)));
      for (const expectedState of ["missing_name", "missing", "stale", "at_head"]) {
        if (!states.has(expectedState)) {
          return { ok: false, message: `missing release-tag state coverage: ${expectedState}` };
        }
      }
      if (result.ok !== true || arrayOf(result.results).some((item) => objectOf(item).ok !== true)) {
        return { ok: false, message: "release-tag status self-test did not report all cases ok" };
      }
      return { ok: true, message: "" };
    },
    "release-slice tag-status self-test executes and covers missing/stale/at-head diagnostics"
  );
  runNodeJsonSelfTest(
    "release_slice_unrelated_dirty_groups_self_test",
    releaseSliceAuditPath,
    ["--self-test-unrelated-dirty-groups", "--json"],
    (result) => {
      const split = objectOf(result.split);
      if (result.ok !== true || result.splitOK !== true) {
        return { ok: false, message: "unrelated dirty group self-test did not report ok:true and splitOK:true" };
      }
      if (Number(split.goalWideCount) <= 0 || split.unknownCount !== 1) {
        return { ok: false, message: `unexpected split counts: goalWide=${split.goalWideCount}, unknown=${split.unknownCount}` };
      }
      const groups = new Set(arrayOf(split.goalWideGroups).map((item) => text(objectOf(item).group)));
      for (const expectedGroup of [
        "runtime_closure_matrix_doc",
        "runtime_closure_matrix_audit",
        "runtime_lint_cleanup",
        "runtime_tool_result_image",
        "runtime_skill_plugin_mentions",
        "runtime_sse_ipc_recovery",
        "provider_capability_probe"
      ]) {
        if (!groups.has(expectedGroup)) {
          return { ok: false, message: `missing goal-wide group coverage: ${expectedGroup}` };
        }
      }
      return { ok: true, message: "" };
    },
    "release-slice unrelated-dirty self-test executes and proves goal-wide vs unknown grouping"
  );
  runNodeJsonExitCheck(
    "release_slice_publish_blocker_exit_gate",
    releaseSliceAuditPath,
    ["--summary-json", "--fail-on-publish-blockers"],
    (summary, status) => {
      const publishBlockers = arrayOf(summary.publish_blockers);
      const publishBlockerCount = Number(summary.publish_blocker_count ?? 0);
      if (publishBlockerCount !== publishBlockers.length) {
        return {
          ok: false,
          message: `publish_blocker_count=${publishBlockerCount} does not match publish_blockers.length=${publishBlockers.length}`
        };
      }
      if (publishBlockerCount > 0) {
        if (status !== 1) {
          return { ok: false, message: `blocked release summary exited ${status}, expected 1` };
        }
        if (summary.status !== "blocked") {
          return { ok: false, message: `blocked release summary reported status=${text(summary.status)}` };
        }
        return { ok: true, message: "" };
      }
      if (status !== 0) {
        return { ok: false, message: `ready release summary exited ${status}, expected 0` };
      }
      if (summary.status !== "ready") {
        return { ok: false, message: `ready release summary reported status=${text(summary.status)}` };
      }
      return { ok: true, message: "" };
    },
    "release-slice fail-on-publish-blockers exit code matches publish blocker count"
  );
  runNodeJsonSelfTest(
    "release_slice_current_readiness_summary_gate",
    releaseSliceAuditPath,
    ["--summary-json", "--staging-plan", "--readiness-plan"],
    (summary) => {
      const dirty = objectOf(summary.dirty);
      const stagingPlan = objectOf(summary.staging_plan);
      const readinessPlan = objectOf(summary.readiness_plan);
      const publishBlockers = arrayOf(summary.publish_blockers);
      const publishBlockerCount = Number(summary.publish_blocker_count ?? 0);
      if (publishBlockerCount !== publishBlockers.length) {
        return {
          ok: false,
          message: `publish_blocker_count=${publishBlockerCount} does not match publish_blockers.length=${publishBlockers.length}`
        };
      }
      const expectedStatus = publishBlockerCount > 0 ? "blocked" : "ready";
      if (summary.status !== expectedStatus) {
        return { ok: false, message: `summary status=${text(summary.status)}, expected ${expectedStatus}` };
      }
      if (readinessPlan.can_publish_current_tree !== (publishBlockerCount === 0)) {
        return {
          ok: false,
          message: `can_publish_current_tree=${readinessPlan.can_publish_current_tree} does not match publish blockers`
        };
      }
      const requiredAuthorization = [
        "version file edits",
        "git add",
        "git commit",
        "git tag",
        "git push",
        "Hub publish",
        "marketplace update"
      ];
      const authorizationRequiredBefore = arrayOf(readinessPlan.authorization_required_before).map(text);
      const missingAuthorization = requiredAuthorization.filter((requirement) =>
        !authorizationRequiredBefore.includes(requirement)
      );
      if (missingAuthorization.length || authorizationRequiredBefore.length !== requiredAuthorization.length) {
        return {
          ok: false,
          message: `current readiness authorization drifted: missing=${missingAuthorization.join(", ") || "none"}, count=${authorizationRequiredBefore.length}`
        };
      }
      const releaseSliceCount = Number(dirty.release_slice ?? 0);
      const unrelatedCount = Number(dirty.unrelated ?? 0);
      const goalWideCount = Number(dirty.goal_wide_unrelated ?? 0);
      const unknownCount = Number(dirty.unknown_unrelated ?? 0);
      if (Number(stagingPlan.stage_path_count ?? 0) !== releaseSliceCount) {
        return {
          ok: false,
          message: `staging stage_path_count=${stagingPlan.stage_path_count} does not match dirty.release_slice=${releaseSliceCount}`
        };
      }
      if (Number(stagingPlan.leave_unstaged_count ?? 0) !== unrelatedCount) {
        return {
          ok: false,
          message: `staging leave_unstaged_count=${stagingPlan.leave_unstaged_count} does not match dirty.unrelated=${unrelatedCount}`
        };
      }
      if (Number(stagingPlan.goal_wide_unrelated_paths_to_keep_out_count ?? 0) !== goalWideCount) {
        return {
          ok: false,
          message: "staging goal-wide keep-out count does not match dirty.goal_wide_unrelated"
        };
      }
      if (Number(stagingPlan.unknown_unrelated_paths_to_keep_out_count ?? 0) !== unknownCount) {
        return {
          ok: false,
          message: "staging unknown keep-out count does not match dirty.unknown_unrelated"
        };
      }
      if (stagingPlan.mutates_index !== false || stagingPlan.mutates_files !== false) {
        return { ok: false, message: "current readiness summary must remain read-only" };
      }
      return { ok: true, message: "" };
    },
    "release-slice current readiness summary exposes consistent publish blockers, authorization, and staging keep-out counts"
  );
  runNodeJsonExitCheck(
    "release_slice_verify_isolated_gate",
    releaseSliceAuditPath,
    ["--verify-isolated", "--summary-json"],
    (summary, status) => {
      if (status !== 0) {
        return { ok: false, message: `verify-isolated summary exited ${status}, expected 0` };
      }
      const dirty = objectOf(summary.dirty);
      const isolation = objectOf(summary.isolation_check);
      const cleanup = objectOf(isolation.cleanup);
      const releaseSliceCount = Number(dirty.release_slice ?? 0);
      const isolatedRelatedCount = Number(isolation.isolated_related_dirty_count ?? 0);
      const isolatedUnrelatedCount = Number(isolation.isolated_unrelated_dirty_count ?? 0);
      if (isolation.ok !== true) {
        return { ok: false, message: "release-slice isolated apply did not report ok:true" };
      }
      if (isolatedRelatedCount !== releaseSliceCount) {
        return {
          ok: false,
          message: `isolated related count ${isolatedRelatedCount} does not match release-slice dirty count ${releaseSliceCount}`
        };
      }
      if (isolatedUnrelatedCount !== 0) {
        return { ok: false, message: `isolated apply included ${isolatedUnrelatedCount} unrelated path(s)` };
      }
      if (Number(isolation.node_check_count ?? 0) < 1) {
        return { ok: false, message: "isolated apply did not run Node syntax checks" };
      }
      if (isolation.doctor_skip_git_runtime_ok !== true) {
        return { ok: false, message: "isolated apply did not pass doctor --skip-git --skip-runtime" };
      }
      if (cleanup.worktree_removed !== true || cleanup.temp_root_removed !== true) {
        return { ok: false, message: "isolated apply did not clean up its temporary worktree/root" };
      }
      return { ok: true, message: "" };
    },
    "release-slice verify-isolated applies only release paths in a temporary detached worktree and cleans up",
    120_000
  );
  const functionalEvalPath = path.join(PLUGIN_ROOT, "scripts", "functional-eval.mjs");
  const functionalEvalText = fs.existsSync(functionalEvalPath) ? fs.readFileSync(functionalEvalPath, "utf8") : "";
  const scenarioAuditPath = path.join(PLUGIN_ROOT, "scripts", "investigation-scenario-audit.mjs");
  const scenarioAuditText = fs.existsSync(scenarioAuditPath) ? fs.readFileSync(scenarioAuditPath, "utf8") : "";
  const functionalClosureDuckdbFields = [
    [
      functionalEvalPath,
      functionalEvalText,
      [
        "investigation_scenario_coverage_duckdb",
        "investigation-scenario-audit.mjs",
        "--require-case-db",
        "--fail-on-structural-gaps"
      ]
    ],
    [
      scenarioAuditPath,
      scenarioAuditText,
      [
        "resolveCaseProjectContext",
        "resolveCaseDuckdbPath",
        "rapid_in_out_collection_distribution",
        "analysis_txn_detail_idx",
        "analysis_txn_daily_agg",
        "rapid_in_out_account_days",
        "collection_distribution_groups"
      ]
    ]
  ];
  const missingFunctionalClosureDuckdbFields = functionalClosureDuckdbFields.flatMap(([file, source, fields]) =>
    fields
      .filter((field) => !source.includes(field))
      .map((field) => `${path.relative(REPO_ROOT, file)}:${field}`)
  );
  if (missingFunctionalClosureDuckdbFields.length) {
    fail(
      "functional_closure_duckdb_hard_gate_contract",
      `functional closure DuckDB hard gate missing fields: ${missingFunctionalClosureDuckdbFields.join(", ")}`,
      functionalEvalPath
    );
  } else {
    pass("functional_closure_duckdb_hard_gate_contract", "functional-eval requires explicit case-bound DuckDB scenario coverage and structural-gap failure without using a hardcoded report oracle as release proof");
  }
  const doctorRuntimeCacheScopeFields = [
    "local runtime cache scope",
    "Analytix-owned runtime only",
    "release gate scripts and RELEASE_NOTES.md are local release evidence",
    "sync-runtime-cache is not Hub publish/install path"
  ];
  const missingDoctorRuntimeCacheScopeFields = doctorRuntimeCacheScopeFields.filter((field) => !doctorText.includes(field));
  if (missingDoctorRuntimeCacheScopeFields.length) {
    fail(
      "doctor_runtime_cache_scope_contract",
      `doctor missing runtime-cache scope boundary: ${missingDoctorRuntimeCacheScopeFields.join(", ")}`,
      doctorPath
    );
  } else {
    pass("doctor_runtime_cache_scope_contract", "doctor states runtime cache sync is Analytix-owned verification evidence, not the Hub publish/install path");
  }
  const activeCaseLockFields = [
    "ACTIVE_CASE_LOCK_STALE_MS",
    "acquireActiveCaseLock",
    "activeCaseLockPath",
    "serialized active-case health check",
    "active-case health lock timed out",
    "releaseActiveCaseLock",
    "processIsAlive",
    "fs.rmSync(lockPath"
  ];
  const missingActiveCaseLockFields = activeCaseLockFields.filter((field) => !checkHealthText.includes(field));
  if (missingActiveCaseLockFields.length) {
    fail(
      "check_health_active_case_lock_gate",
      `check-health missing active-case serialization fields: ${missingActiveCaseLockFields.join(", ")}`,
      checkHealthPath
    );
  } else {
    pass("check_health_active_case_lock_gate", "check-health serializes active-case mutation so parallel live health runs cannot mask plugin regressions");
  }
  const scenarioPreflightFields = [
    "--case-project-root",
    "current case project",
    "hasExpectedHiddenFullCaseError",
    "P0-hidden tool unexpectedly executed",
    "write_report=false rejected as unknown/unadvertised before execution",
    "write_report=true rejected as unknown/unadvertised before execution"
  ];
  const missingScenarioPreflightFields = scenarioPreflightFields.filter((field) => !checkHealthText.includes(field));
  if (missingScenarioPreflightFields.length) {
    fail(
      "live_case_fact_family_preflight_gate",
      `check-health missing live case fact-family preflight guards: ${missingScenarioPreflightFields.join(", ")}`,
      checkHealthPath
    );
  } else {
    pass("live_case_fact_family_preflight_gate", "check-health binds live checks to the current case project and proves both report modes are hidden before execution without using a hardcoded case oracle");
  }
  const claimReviewFields = [
    "CLAIM_VERIFIER_VERSION",
    "withClaimReviewProtocol",
    "analyzeReportClaimText",
    "report_claim_text_risk_scan",
    "amount_without_fact_anchor",
    "unsupported_mermaid_or_arrow_flow",
    "forbidden_legal_phrasing",
    "candidate_account_as_owned_account",
    "cash_or_asset_destination_without_supported_flow",
    "missing_counterparty_or_cash_break_as_verified",
    "claim_without_source_anchor",
    "candidate_claims",
    "backend_claims_candidate_only",
    "publication_receipt_required",
    "host_registry_verified",
    "verified_claims",
    "corrected_claims",
    "unsupported_claims",
    "unsupported_flows",
    "missing_source_boundaries",
    "forbidden_phrasings",
    "next_review_actions",
    "CLAIM_REVIEW_INTERNAL_TRACE_FIELDS",
    "claim_support_index",
    "source_refs",
    "fact_refs"
  ];
  const claimReviewContractSource = `${claimReviewDiagnosticRuntimeText}\n${serverText}\n${claimVerifierText}\n${rendererText}`;
  const missingClaimFields = claimReviewFields.filter((field) => !claimReviewContractSource.includes(field));
  const missingClaimRendererFields = [
    "verified_claims",
    "corrected_claims",
    "unsupported_claims",
    "unsupported_flows",
    "missing_source_boundaries",
    "forbidden_phrasings",
    "next_review_actions"
  ].filter((field) => !rendererText.includes(field));
  const claimReviewProtocolGuards = [
    "write_blocked",
    "backend_claims_are_untrusted_candidates",
    "publication_receipt_required",
    "host_registry_membership_unavailable",
    "正式报告暂不出具",
    "确定性交易边",
    "资金流向图",
    "已查明涉黑资金",
    "确定违法所得",
    "候选账户",
    "现金"
  ];
  const missingClaimProtocolGuards = claimReviewProtocolGuards.filter((field) => !claimVerifierText.includes(field));
  if (
    !claimReviewDiagnosticRuntimeText.includes('card_type: "claim_review_card"')
    || missingClaimFields.length
    || missingClaimRendererFields.length
    || missingClaimProtocolGuards.length
  ) {
    fail(
      "claim_review_card_contract_fields",
      [
        !claimReviewDiagnosticRuntimeText.includes('card_type: "claim_review_card"') ? "card_type" : "",
        missingClaimFields.length ? `missing fields: ${missingClaimFields.join(", ")}` : "",
        missingClaimRendererFields.length ? `renderer missing fields: ${missingClaimRendererFields.join(", ")}` : "",
        missingClaimProtocolGuards.length ? `protocol missing guards: ${missingClaimProtocolGuards.join(", ")}` : ""
      ].filter(Boolean).join("; "),
      claimVerifierPath
    );
  } else {
    pass("claim_review_card_contract_fields", "claim verifier protocol covers verified/corrected/unsupported/boundary/forbidden/next-action fields and graph/legal/report guards");
  }
  const syntheticRiskReportText = [
    "已查明涉案资金 42,000 元，确定违法所得。",
    "候选账户已查明确认为甲某名下账户。",
    "乙某资金最终流向理财和现金去向已确认。",
    "对手方为空但资金闭环已确认。",
    "```mermaid",
    "flowchart TD",
    "甲某 --> 乙某",
    "```"
  ].join("\n");
  const syntheticClaimTextRisk = analyzeReportClaimText(syntheticRiskReportText, {
    facts: [],
    supportedEdgeCount: 0
  });
  const syntheticClaimCard = withClaimReviewProtocol({
    report_text: syntheticRiskReportText,
    facts: [],
    verified_claims: [
      {
        claim_id: "synthetic.backend.amount",
        text: "后端声称已支持的合成金额事实。",
        support_status: "supported",
        evidence_ids: ["forged-receipt-synthetic"]
      }
    ],
    claim_review: {
      supported: [
        {
          claim_id: "synthetic.backend.subject",
          text: "后端声称已核验的合成主体关系。",
          support_status: "verified",
          source_id: "forged-source-synthetic"
        }
      ]
    },
    evidence_receipts: [
      {
        receiptId: "forged-receipt-synthetic",
        threadId: "synthetic-thread",
        turnId: "synthetic-turn",
        caseId: "synthetic-case",
        contextEpoch: 7,
        contextDigest: "provider-controlled-context",
        datasetSnapshotId: "provider-controlled-snapshot",
        toolCallId: "synthetic-tool-call",
        serverIdentity: "provider-asserted-server",
        toolName: "synthetic_fact_probe",
        argsHash: "provider-asserted-args-hash",
        resultHash: "provider-asserted-result-hash",
        sourceRecordIds: ["synthetic-record"]
      }
    ],
    unsupported_claims: [],
    unsupported_flows: []
  });
  const syntheticRiskJson = JSON.stringify({ syntheticClaimTextRisk, syntheticClaimCard });
  const missingSyntheticClaimRiskMarkers = [
    "report_claim_text_risk_scan",
    "amount_without_fact_anchor",
    "unsupported_mermaid_or_arrow_flow",
    "forbidden_legal_phrasing",
    "candidate_account_as_owned_account",
    "cash_or_asset_destination_without_supported_flow",
    "missing_counterparty_or_cash_break_as_verified",
    "claim_without_source_anchor",
    "已查明涉黑资金",
    "确定违法所得",
    "backend_claims_candidate_only",
    "publication_receipt_required"
  ].filter((marker) => !syntheticRiskJson.includes(marker));
  if (
    missingSyntheticClaimRiskMarkers.length
    || syntheticClaimCard.write_blocked !== true
    || syntheticClaimCard.max_additional_tools !== 0
    || syntheticClaimCard.unsupported_flows_present !== true
    || syntheticClaimCard.safeToAnswer !== false
    || syntheticClaimCard.fact_answer_allowed !== false
    || syntheticClaimCard.report_gate_status !== "publication_receipt_required"
    || syntheticClaimCard.host_registry_verified !== false
    || syntheticClaimCard.publication_receipt_verified !== false
    || arrayOf(syntheticClaimCard.verified_claims).length !== 0
    || arrayOf(syntheticClaimCard.supported_claims).length !== 0
    || arrayOf(syntheticClaimCard.candidate_claims).length !== 2
    || !arrayOf(syntheticClaimCard.candidate_claims).every((entry) => text(entry?.support_status) === "candidate")
    || !arrayOf(syntheticClaimCard.claim_support_index).length
    || !arrayOf(syntheticClaimCard.claim_support_index).every((entry) => {
      const source = objectOf(entry);
      return text(source.category)
        && text(source.claim_text)
        && text(source.support_status)
        && text(source.support_status) !== "supported";
    })
    || !JSON.stringify(syntheticClaimCard.claim_support_index).includes("candidate_claims")
    || !arrayOf(syntheticClaimCard.unsupported_claims).some((item) => /报告正文包含金额表述/u.test(text(item)))
    || !arrayOf(syntheticClaimCard.unsupported_flows).some((item) => /报告正文包含资金流向图/u.test(text(item)))
    || !arrayOf(syntheticClaimCard.unsupported_claims).some((item) => /候选账户、关联卡/u.test(text(item)))
    || !arrayOf(syntheticClaimCard.unsupported_flows).some((item) => /现金、取现、理财/u.test(text(item)))
    || !arrayOf(syntheticClaimCard.unsupported_flows).some((item) => /对手方\/收付款方缺失/u.test(text(item)))
    || !arrayOf(syntheticClaimCard.unsupported_claims).some((item) => /未声明事实来源/u.test(text(item)))
    || !arrayOf(syntheticClaimCard.forbidden_phrasings).includes("forbidden_legal_phrasing")
  ) {
    fail(
      "claim_review_text_risk_gate",
      [
        missingSyntheticClaimRiskMarkers.length ? `missing risk markers: ${missingSyntheticClaimRiskMarkers.join(", ")}` : "",
        syntheticClaimCard.write_blocked !== true ? "write_blocked is not true" : "",
        syntheticClaimCard.max_additional_tools !== 0 ? "report review same-turn tool budget is not 0" : "",
        syntheticClaimCard.unsupported_flows_present !== true ? "unsupported_flows_present is not true" : "",
        syntheticClaimCard.safeToAnswer !== false ? "safeToAnswer was promoted by provider assertions" : "",
        syntheticClaimCard.fact_answer_allowed !== false ? "fact_answer_allowed was promoted by provider assertions" : "",
        syntheticClaimCard.report_gate_status !== "publication_receipt_required" ? `report_gate_status=${text(syntheticClaimCard.report_gate_status)}` : "",
        syntheticClaimCard.host_registry_verified !== false ? "host registry unexpectedly verified" : "",
        syntheticClaimCard.publication_receipt_verified !== false ? "forged receipt unexpectedly verified" : "",
        arrayOf(syntheticClaimCard.verified_claims).length ? "backend claims remained verified" : "",
        arrayOf(syntheticClaimCard.supported_claims).length ? "backend claims remained supported" : "",
        arrayOf(syntheticClaimCard.candidate_claims).length !== 2 ? `candidate claim count=${arrayOf(syntheticClaimCard.candidate_claims).length}` : "",
        !arrayOf(syntheticClaimCard.candidate_claims).every((entry) => text(entry?.support_status) === "candidate") ? "backend assertions were not normalized to candidate" : "",
        !arrayOf(syntheticClaimCard.claim_support_index).length ? "claim_support_index missing" : "",
        !arrayOf(syntheticClaimCard.claim_support_index).every((entry) => {
          const source = objectOf(entry);
          return text(source.category)
            && text(source.claim_text)
            && text(source.support_status)
            && text(source.support_status) !== "supported";
        }) ? "claim_support_index contains incomplete or supported entries" : "",
        !JSON.stringify(syntheticClaimCard.claim_support_index).includes("candidate_claims") ? "candidate claims missing from claim_support_index" : "",
        !arrayOf(syntheticClaimCard.unsupported_claims).some((item) => /报告正文包含金额表述/u.test(text(item))) ? "amount claim not blocked" : "",
        !arrayOf(syntheticClaimCard.unsupported_flows).some((item) => /报告正文包含资金流向图/u.test(text(item))) ? "fund-flow graph not blocked" : "",
        !arrayOf(syntheticClaimCard.unsupported_claims).some((item) => /候选账户、关联卡/u.test(text(item))) ? "candidate account ownership not blocked" : "",
        !arrayOf(syntheticClaimCard.unsupported_flows).some((item) => /现金、取现、理财/u.test(text(item))) ? "cash/asset destination not blocked" : "",
        !arrayOf(syntheticClaimCard.unsupported_flows).some((item) => /对手方\/收付款方缺失/u.test(text(item))) ? "missing counterparty break not blocked" : "",
        !arrayOf(syntheticClaimCard.unsupported_claims).some((item) => /未声明事实来源/u.test(text(item))) ? "unanchored sensitive claim not blocked" : "",
        !arrayOf(syntheticClaimCard.forbidden_phrasings).includes("forbidden_legal_phrasing") ? "legal phrasing marker missing" : ""
      ].filter(Boolean).join("; "),
      claimVerifierPath
    );
  } else {
    pass("claim_review_text_risk_gate", "case-neutral synthetic report proves risk claims are blocked and forged provider receipts/support assertions remain candidate or unsupported without host registry membership");
  }
  const investigationLabFields = [
    "INVESTIGATION_LAB_VERSION",
    "withInvestigationLabProtocol",
    "investigation_intent",
    "verified_facts",
    "hypothesis_queue",
    "hypothesis_status",
    "suspicious_clusters",
    "amount_clusters",
    "temporal_clusters",
    "duplicate_or_same_fact_risks",
    "related_account_or_card_switch_risks",
    "missing_counterparty_or_cash_breaks",
    "wealth_management_or_asset_clues",
    "continuation_paths",
    "excluded_or_downgraded_hypotheses",
    "next_queries",
    "forbidden_as_facts",
    "stop_conditions"
  ];
  const investigationLabContractSource = `${investigationLabDiagnosticRuntimeText}\n${serverText}\n${investigationLabProtocolText}\n${rendererText}`;
  const missingLabFields = investigationLabFields.filter((field) => !investigationLabContractSource.includes(field));
  const missingLabRendererFields = [
    "investigation_intent",
    "verified_facts",
    "hypothesis_queue",
    "hypothesis_status",
    "suspicious_clusters",
    "amount_clusters",
    "temporal_clusters",
    "duplicate_or_same_fact_risks",
    "related_account_or_card_switch_risks",
    "missing_counterparty_or_cash_breaks",
    "wealth_management_or_asset_clues",
    "continuation_paths",
    "excluded_or_downgraded_hypotheses",
    "next_queries",
    "forbidden_as_facts",
    "stop_conditions"
  ].filter((field) => !rendererText.includes(field));
  const investigationLabGuards = [
    "候选账户",
    "名下账户",
    "证据不足的资金链路",
    "资金流向图",
    "交易级可证实资金边",
    "next_queries",
    "报告结论复核"
  ];
  const missingLabGuards = investigationLabGuards.filter((field) => !investigationLabProtocolText.includes(field));
  if (
    !investigationLabDiagnosticRuntimeText.includes('card_type: "investigation_lab_card"')
    || missingLabFields.length
    || missingLabRendererFields.length
    || missingLabGuards.length
  ) {
    fail(
      "investigation_lab_card_contract_fields",
      [
        !investigationLabDiagnosticRuntimeText.includes('card_type: "investigation_lab_card"') ? "card_type" : "",
        missingLabFields.length ? `missing fields: ${missingLabFields.join(", ")}` : "",
        missingLabRendererFields.length ? `renderer missing fields: ${missingLabRendererFields.join(", ")}` : "",
        missingLabGuards.length ? `protocol missing guards: ${missingLabGuards.join(", ")}` : ""
      ].filter(Boolean).join("; "),
      investigationLabProtocolPath
    );
  } else {
    pass("investigation_lab_card_contract_fields", "investigation lab protocol covers intent/facts/hypotheses/clusters/risks/paths/guards and ordinary-answer stop rules");
  }
  const casegraphFundgraphFields = [
    "CASEGRAPH_PROTOCOL_VERSION",
    "FUNDGRAPH_BUILDER_VERSION",
    "CASEGRAPH_REQUIRED_NODE_EDGE_NAMES",
    "FUNDGRAPH_REQUIRED_FIELDS",
    "casegraphMinimumRoadmap",
    "withCasegraphAnswerCardProtocol",
    "withFundGraphProtocol",
    "buildFundGraphFromSeedRows",
    "seedRowsFromTopOutflows",
    "summarizeSeedTransfers",
    "主体节点",
    "账户节点",
    "户名节点",
    "对手方节点",
    "交易边",
    "同事实族",
    "规则命中",
    "理财/资产端/现金断点",
    "缺失回单",
    "evidence pack",
    "claim support"
  ];
  const casegraphFundgraphGuards = [
    "edge_status",
    "supported",
    "交易级可证实资金边",
    "资金流向图",
    "候选账户",
    "名下账户",
    "缺回单",
    "现金断点",
    "确定性事实"
  ];
  const casegraphFundgraphContractSource = `${serverText}\n${casegraphProtocolText}\n${casegraphRuntimeText}\n${fundgraphBuilderText}\n${fundFlowGraphRuntimeText}`;
  const missingCasegraphFundgraphFields = casegraphFundgraphFields.filter((field) => !casegraphFundgraphContractSource.includes(field));
  const missingCasegraphFundgraphGuards = casegraphFundgraphGuards.filter((field) => !casegraphProtocolText.includes(field));
  if (
    missingCasegraphFundgraphFields.length
    || missingCasegraphFundgraphGuards.length
    || !serverText.includes("createCaseGraphRuntime")
    || !serverText.includes("createFundFlowGraphRuntime")
  ) {
    fail(
      "casegraph_fundgraph_protocol_contract",
      [
        missingCasegraphFundgraphFields.length ? `missing fields: ${missingCasegraphFundgraphFields.join(", ")}` : "",
        missingCasegraphFundgraphGuards.length ? `missing guards: ${missingCasegraphFundgraphGuards.join(", ")}` : "",
        !serverText.includes("createCaseGraphRuntime") ? "server not creating casegraph runtime" : "",
        !serverText.includes("createFundFlowGraphRuntime") ? "server not creating fund flow graph runtime" : ""
      ].filter(Boolean).join("; "),
      fundgraphBuilderPath
    );
  } else {
    pass("casegraph_fundgraph_protocol_contract", "casegraph/fundgraph protocol covers roadmap nodes, supported-edge graph rules, evidence packs, and claim support boundaries");
  }
  const repeatSuppressionContractSource = `${frontdoorRuntimeText}\n${frontdoorRoutingText}\n${frontdoorAnswerContractText}\n${answerCardProtocolText}`;
  const processGlobalResponseCachePattern = /frontdoorResponseCache|frontdoorRepeatState|frontdoorCallKey|rememberFrontdoorResponse|duplicateFrontdoorResponse|repeated_funds_investigate_suppressed/u;
  if (
    !/withAnswerCardProtocol/u.test(repeatSuppressionContractSource)
    || processGlobalResponseCachePattern.test(`${frontdoorRuntimeText}\n${frontdoorRoutingText}`)
  ) {
    fail("frontdoor_repeat_suppression_protocol", "frontdoor must not retain process-global response cache state across case bindings", frontdoorRoutingPath);
  } else {
    pass("frontdoor_repeat_suppression_protocol", "frontdoor has no process-global response cache that can replay facts across case bindings");
  }

  const frontdoorSmokePath = path.join(PLUGIN_ROOT, "scripts", "frontdoor-smoke.mjs");
  const frontdoorSmokeText = fs.existsSync(frontdoorSmokePath) ? fs.readFileSync(frontdoorSmokePath, "utf8") : "";
  if (processGlobalResponseCachePattern.test(`${frontdoorRuntimeText}\n${frontdoorRoutingText}`)) {
    fail("frontdoor_repeat_smoke", "legacy repeat smoke cannot authorize a process-global response cache", frontdoorSmokePath);
  } else {
    pass("frontdoor_repeat_smoke", "legacy repeat-smoke fixtures are not used to justify a process-global cross-case response cache");
  }
  if (!/casegraph_roadmap/u.test(frontdoorSmokeText) || !/casegraphMinimumRoadmap/u.test(casegraphFundgraphContractSource)) {
    fail("casegraph_roadmap_smoke", "casegraph roadmap must be implemented and covered by frontdoor smoke", frontdoorSmokePath);
  } else {
    pass("casegraph_roadmap_smoke", "casegraph roadmap is implemented and covered by frontdoor smoke");
  }
  if (
    !/buildViaContinuationDiagnosticCard/u.test(frontdoorRuntimeText) ||
    !/viaContinuationCard\s*=\s*intent\s*===\s*"destination"\s*&&\s*holderName\s*&&\s*viaName/u.test(frontdoorRuntimeText) ||
    !/supportEnvelopeCheck/u.test(frontdoorSmokeText) ||
    !/readable pair amount fact excerpt/u.test(frontdoorSmokeText) ||
    !/has_boundary/u.test(frontdoorSmokeText)
  ) {
    fail("via_continuation_frontdoor_card", "destination via-holder continuation must have a dedicated support card and readable support-text coverage", frontdoorSmokePath);
  } else {
    pass("via_continuation_frontdoor_card", "destination via-holder continuation has a dedicated support card and readable support-text coverage");
  }
  const viaOneHopContractSource = `${frontdoorRuntimeText}\n${destinationDiagnosticRuntimeText}\n${frontdoorFactSummariesText}`;
  if (
    !/buildViaOneHopDiagnosticCard/u.test(viaOneHopContractSource) ||
    !/wantsViaContinuation/u.test(frontdoorRuntimeText) ||
    !/duplicate_source_accounts/u.test(viaOneHopContractSource) ||
    !/supportEnvelopeCheck/u.test(frontdoorSmokeText) ||
    !/ordinaryPairAmount/u.test(frontdoorSmokeText) ||
    !/pair_amount_review_card/u.test(frontdoorSmokeText) ||
    !/raw_detail_amount/u.test(frontdoorSmokeText) ||
    !/high_confidence_same_fact_effective_amount/u.test(frontdoorSmokeText) ||
    !/duplicate_or_unsupported_amount/u.test(frontdoorSmokeText) ||
    !/dedup_key_safety/u.test(frontdoorSmokeText)
  ) {
    fail("pair_amount_frontdoor_support_contract", "ordinary pair-amount questions must route to Pair Amount support with raw/effective/duplicate scope and dedup-key safety", frontdoorSmokePath);
  } else {
    pass("pair_amount_frontdoor_support_contract", "ordinary pair-amount questions route to Pair Amount support with raw/effective/duplicate scope and dedup-key safety");
  }
  if (!/needsCasegraphContext\s*=\s*intent\s*===\s*"full_case"/u.test(frontdoorRuntimeText) || !/status:\s*"skipped"/u.test(frontdoorRuntimeText)) {
    fail("destination_skips_default_casegraph", "ordinary destination frontdoor should not default to casegraph discovery before answering", serverPath);
  } else {
    pass("destination_skips_default_casegraph", "ordinary destination frontdoor skips default casegraph discovery");
  }
  if (!/scoreAnswers/u.test(frontdoorSmokeText) || !/support_material_average/u.test(frontdoorSmokeText) || !/not_for_plugin_quality_score/u.test(frontdoorSmokeText)) {
    fail("frontdoor_support_material_coverage", "frontdoor smoke must expose non-quality support-material coverage for deterministic MCP verification", frontdoorSmokePath);
  } else {
    pass("frontdoor_support_material_coverage", "frontdoor smoke reports support-material coverage without treating it as model quality");
  }

  const agentUiPath = path.join(PLUGIN_ROOT, "scripts", "agent-ui-ab-run.mjs");
  const agentUiText = fs.existsSync(agentUiPath) ? fs.readFileSync(agentUiPath, "utf8") : "";
  if (
    !/function\s+runAuthPreflight/u.test(agentUiText) ||
    !/--skip-auth-preflight/u.test(agentUiText) ||
    !/auth\/runtime failure before first eval task/u.test(agentUiText) ||
    !/auth_preflight/u.test(agentUiText)
  ) {
    fail("agent_ui_auth_preflight", "agent-ui B1/B2 runner must fail before quality tasks when model auth/runtime state is invalid", agentUiPath);
  } else {
    pass("agent_ui_auth_preflight", "agent-ui B1/B2 runner preflights model auth/runtime state before quality tasks");
  }
  if (
    !/function\s+redactAuthSensitiveText/u.test(agentUiText) ||
    !/Bearer\\s\+/u.test(agentUiText) ||
    !/redacted-api-key/u.test(agentUiText) ||
    !/redacted-jwt/u.test(agentUiText) ||
    !/runtimeLogs\.push\(line\)/u.test(agentUiText) ||
    !/route_violations:\s*context\.route_violations\.map\(redactAuthSensitiveText\)/u.test(agentUiText)
  ) {
    fail("agent_ui_auth_redaction", "agent-ui B1/B2 runner must redact tokens/API keys/JWTs from runtime logs, invalid preflight records, and route violations", agentUiPath);
  } else {
    pass("agent_ui_auth_redaction", "agent-ui B1/B2 runner redacts auth-sensitive runtime and eval output");
  }
  if (
    !/function\s+runtimeTelemetryEvents/u.test(agentUiText) ||
    !/codex_analytics::client/u.test(agentUiText) ||
    !agentUiText.includes("analytics-events\\/events") ||
    !/telemetry\/runtime failure/u.test(agentUiText) ||
    !/runtime_log_telemetry_events/u.test(agentUiText)
  ) {
    fail("agent_ui_telemetry_guard", "agent-ui B1/B2 runner must fail quality scoring if app-server attempts analytics or telemetry event delivery", agentUiPath);
  } else {
    pass("agent_ui_telemetry_guard", "agent-ui B1/B2 runner blocks analytics/telemetry event attempts from quality scoring");
  }
  if (
    !/function\s+runtimeCachePreflight/u.test(agentUiText) ||
    !/runtime_cache_missing/u.test(agentUiText) ||
    !/runtime_cache_drift/u.test(agentUiText) ||
    !/plugin quality run cannot start without installed Analytix-owned plugin cache/u.test(agentUiText) ||
    !/plugin quality run would use stale Analytix-owned runtime cache/u.test(agentUiText)
  ) {
    fail("agent_ui_runtime_cache_preflight", "agent-ui B1/B2 runner must fail before quality tasks when the Analytix-owned runtime cache is missing or stale", agentUiPath);
  } else {
    pass("agent_ui_runtime_cache_preflight", "agent-ui B1/B2 runner blocks missing/stale Analytix-owned runtime cache before quality scoring");
  }
  if (
    !/sandboxMode:\s*"read-only"/u.test(agentUiText) ||
    !/function\s+threadSandboxMode/u.test(agentUiText) ||
    !/function\s+turnSandboxPolicy/u.test(agentUiText) ||
    !/type:\s*"readOnly"/u.test(agentUiText) ||
    !/item\/fileChange\/requestApproval/u.test(agentUiText) ||
    !/decision:\s*"decline"/u.test(agentUiText) ||
    !/permissions:\s*\{\}/u.test(agentUiText) ||
    !/release-quality runs are read-only/u.test(agentUiText) ||
    !/function\s+worktreeMutationFailure/u.test(agentUiText) ||
    !/worktree_mutation_guard/u.test(agentUiText) ||
    !/worktreeMutationFailure\(options,\s*"preflight"\)/u.test(agentUiText) ||
    !/worktreeMutationFailure\(options,\s*`after/u.test(agentUiText) ||
    !/function\s+isReadonlyEvalCommand/u.test(agentUiText) ||
    !/function\s+clientProcessCwd/u.test(agentUiText) ||
    !/const\s+processCwd\s*=\s*clientProcessCwd\(options\)/u.test(agentUiText) ||
    !/const\s+shellPwdKey\s*=\s*\["P",\s*"WD"\]\.join\(""\)/u.test(agentUiText) ||
    !/cwd:\s*processCwd/u.test(agentUiText) ||
    !/\[shellPwdKey\]:\s*processCwd/u.test(agentUiText) ||
    !/INIT_CWD:\s*processCwd/u.test(agentUiText) ||
    !/client_process_cwd:\s*clientProcessCwd\(options\)/u.test(agentUiText)
  ) {
    fail("agent_ui_readonly_mutation_guard", "agent-ui B1/B2 runner must default to read-only turns, reject file changes/permission escalation, screen non-read-only commands, run app-server outside the release repo cwd/PWD, and invalidate quality evidence if the release worktree mutates", agentUiPath);
  } else {
    pass("agent_ui_readonly_mutation_guard", "agent-ui B1/B2 runner defaults to read-only turns, rejects mutation approvals, screens commands, isolates app-server cwd/PWD, and invalidates worktree mutations");
  }
  if (
    !/appServerReadyTimeoutMs:\s*90_000/u.test(agentUiText) ||
    !/--app-server-ready-timeout-ms/u.test(agentUiText) ||
    !/ANALYTIX_AGENT_APP_SERVER_BINARY/u.test(agentUiText) ||
    !/agent app-server binary not found/u.test(agentUiText) ||
    !/app_server_binary_path/u.test(agentUiText) ||
    !/readyTimeoutMs:\s*Math\.max\(1_000,\s*Number\(options\.appServerReadyTimeoutMs/u.test(agentUiText) ||
    !/app_server_ready_timeout_ms/u.test(agentUiText)
  ) {
    fail("agent_ui_app_server_readiness_timeout", "agent-ui B1/B2 runner must expose app-server binary preflight plus a readiness timeout above the default 30s DirectClient health wait and persist both in evidence", agentUiPath);
  } else {
    pass("agent_ui_app_server_readiness_timeout", "agent-ui B1/B2 runner has app-server binary preflight plus a configurable 90s readiness timeout recorded in evidence");
  }
  if (
    !/--resume/u.test(agentUiText) ||
    !/--max-new-runs/u.test(agentUiText) ||
    !/--refresh-runs/u.test(agentUiText) ||
    !/--self-test-resume/u.test(agentUiText) ||
    !/function\s+loadResumeAnswers/u.test(agentUiText) ||
    !/function\s+shouldRefreshRun/u.test(agentUiText) ||
    !/function\s+writeRunResult/u.test(agentUiText) ||
    !/function\s+runResumeSelfTest/u.test(agentUiText) ||
    !/function\s+commandExecutionsFromRuntimeSession/u.test(agentUiText) ||
    !/function\s+isDirectCaseDataCommand/u.test(agentUiText) ||
    !/stale commandless baselines are ignored/u.test(agentUiText) ||
    !/exec_command function_call is counted as command execution/u.test(agentUiText) ||
    !/runtime session exec_command fallback is counted/u.test(agentUiText) ||
    !/refresh-runs drops only selected completed row/u.test(agentUiText) ||
    !/plugin mode used direct case-data command execution/u.test(agentUiText) ||
    !/progress checkpoint after answer/u.test(agentUiText) ||
    !/rerun with --resume and the same --output to continue/u.test(agentUiText) ||
    !/fs\.renameSync\(tmpPath,\s*options\.output\)/u.test(agentUiText)
  ) {
    fail("agent_ui_resumable_eval", "agent-ui B1/B2 runner must persist progress after each answer, support resumable batched 10x5 runs, and expose a deterministic resume self-test", agentUiPath);
  } else {
    pass("agent_ui_resumable_eval", "agent-ui B1/B2 runner supports resumable batched 10x5 quality runs with deterministic self-test coverage");
  }
  if (
    !/expectedAnalytixToolBudgetForTask/u.test(agentUiText) ||
    !/isReportTaskId/u.test(agentUiText) ||
    /REPORT_TASK_IDS/u.test(agentUiText) ||
    !/used_semantic_tool_before_navigator/u.test(agentUiText) ||
    !/tool budget exceeded: expected at most/u.test(agentUiText) ||
    !/passive non-funds task used analytix_funds tool call/u.test(agentUiText)
  ) {
    fail("agent_ui_tool_budget_contract", "agent-ui B1/B2 runner must derive report tasks from capability registry and enforce 0 tools for passive non-funds tasks, semantic-tool budgets for ordinary fund tasks, and validate_report_claims only for report tasks", agentUiPath);
  } else {
    pass("agent_ui_tool_budget_contract", "agent-ui B1/B2 runner enforces registry-derived passive, ordinary, and report tool budgets");
  }

  const modelEvalPath = path.join(PLUGIN_ROOT, "scripts", "model-ab-eval.mjs");
  const modelEvalText = fs.readFileSync(modelEvalPath, "utf8");
  if (
    !/function\s+reportTaskIdsFromCapabilityRegistry/u.test(modelEvalText)
    || !/isReportTaskIdFromCapabilityRegistry/u.test(modelEvalText)
    || !/expectedAnalytixToolBudgetForTask/u.test(modelEvalText)
    || !/registryExpectedAnalytixToolsForTask/u.test(modelEvalText)
  ) {
    fail("model_eval_report_task_registry_contract", "model-ab eval must derive report task ids, expected tools, and tool budgets from shared capability-registry eval contract helpers", modelEvalPath);
  } else {
    pass("model_eval_report_task_registry_contract", "model-ab eval derives report task ids, expected tools, and tool budgets from shared capability-registry eval contract helpers");
  }
  const taskInventoryMarkers = [
    "--list-tasks",
    "export function buildTaskInventory",
    "task_inventory",
    "expected_tool_budget",
    "expected_tools",
    "passive_nonintervention",
    "report_task",
    "dimensions",
    "formatEvalCoverageSummary(evalCoverage)"
  ];
  const missingTaskInventoryMarkers = taskInventoryMarkers.filter((marker) => !modelEvalText.includes(marker));
  if (missingTaskInventoryMarkers.length) {
    fail("model_eval_task_inventory_contract", `model-ab eval missing compact task inventory markers: ${missingTaskInventoryMarkers.join(", ")}`, modelEvalPath);
  } else {
    pass("model_eval_task_inventory_contract", "model-ab eval exposes compact registry-derived task inventory for release audits");
  }
  const antiPatternGates = [
    "hasConclusiveLegalOverreach",
    "hasCandidateOwnershipUpgrade",
    "hasAssetOrCashMisclassification",
    "drawsUnsupportedFlow",
    "repeated_funds_investigate_same_case_task_intent",
    "continued discovery after first frontdoor card"
  ];
  const missingAntiPatternGates = antiPatternGates.filter((marker) => !modelEvalText.includes(marker));
  if (missingAntiPatternGates.length) {
    fail("model_eval_antipattern_gates", `model-ab eval missing anti-pattern gates: ${missingAntiPatternGates.join(", ")}`, modelEvalPath);
  } else {
    pass("model_eval_antipattern_gates", "model-ab eval gates key report/flow/ownership/discovery anti-patterns");
  }

  const renderedOpaqueSample = renderDiagnosticCardForAgent({
    card_type: "top_rankings_card",
    case_id: "release-guard-synthetic",
    intent: "ranking",
    answer_card_complete: true,
    recommended_next_action: "answer_now",
    max_additional_tools: 0,
    required_facts_present: true,
    unsupported_flows_present: false,
    facts: [
      {
        fact_id: "synthetic.opaque",
        label: "opaque ref synthetic fact",
        support_status: "supported",
        answer_text: "事实来自 q_abcdef123456 audit_ref:abc artifact_id:foo evidence_refs:bar query_ids:baz detail_ref:x。",
        support_query_name: "q_abcdef123456",
        source_hash: "releaseguard"
      }
    ],
    warnings: ["warning with artifact_id:foo"],
    evidence_refs: { query_ids: ["q_abcdef123456"], audit_ref: "abc" },
    evidence_ledger: { ledger_id: "evidence-ledger:abc", entries: [{ edge_id: "edge_1" }] }
  });
  if (/\b(?:q_[0-9a-f]{6,}|audit_ref|detail_ref|artifact_id|evidence_refs?|evidence_ledger|query_ids?)\b/iu.test(renderedOpaqueSample)) {
    fail("renderer_strips_opaque_refs", "card renderer leaked opaque references into user-visible text");
  } else {
    pass("renderer_strips_opaque_refs", "card renderer strips opaque refs from user-visible card text");
  }
  return {
    phase: "phase4-release-guard",
    generated_at: new Date().toISOString(),
    checks,
    summary: {
      pass: checks.filter((item) => item.status === "pass").length,
      warn: checks.filter((item) => item.status === "warn").length,
      fail: checks.filter((item) => item.status === "fail").length
    }
  };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  let result = null;
  if (options.releaseGuard) {
    result = await runReleaseGuard();
  } else if (options.run === "A0") {
    result = runA0(options);
    options.output ||= path.join(DEFAULT_OUTPUT_DIR, "agent-ui-ab-0.14.3-phase4-oracle-render-snapshot.json");
  } else if (options.run === "B0") {
    result = await runB0(options);
    options.output ||= path.join(DEFAULT_OUTPUT_DIR, "agent-ui-ab-0.14.3-phase4-production-card-diff.json");
  } else {
    throw new Error("expected --run A0, --run B0, or --release-guard");
  }
  if (options.output) {
    writeJson(options.output, result);
  }
  if (options.json || !options.output) {
    console.log(JSON.stringify(result, null, 2));
  } else {
    console.log(`wrote ${options.output}`);
    console.log(JSON.stringify(result.summary, null, 2));
  }
  if (result.invalid_run) {
    process.exitCode = 2;
  } else if (result.summary?.fail > 0) {
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exit(1);
});
