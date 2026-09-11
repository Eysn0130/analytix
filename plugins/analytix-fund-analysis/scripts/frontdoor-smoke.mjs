#!/usr/bin/env node

import { spawn } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { userVisibleLeakageLabels } from "../mcp/user-facing-language.mjs";
import {
  FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE,
  FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY,
} from "../mcp/mcp-request-handler-runtime.mjs";
import {
  hostFactRuntimeContext,
  writeCaseProjectBinding
} from "./host-runtime-context-fixture.mjs";
const DEFAULT_BACKEND_URL = "http://127.0.0.1:18731";
const DEFAULT_OUTPUT_DIR = path.join(os.tmpdir(), "analytix-fund-analysis", "functional-closure");

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");
const repoRoot = path.resolve(pluginRoot, "../..");

const OPAQUE_REF_PATTERN = /\b(?:q_[0-9a-f]{6,}|casegraph:[\w:.-]+|audit_ref|detail_ref|artifact_id|evidence_refs?|query_ids?|report_ids?|path_ids?|edge_id)\b/iu;
const DOC_LEAKAGE_MARKERS = [
  "蓝皮书",
  "执行方案",
  "/goal",
  "doctor",
  "score",
  "diagnostic label",
  "golden answer",
  "oracle",
  "rubric"
];
const REPORT_GATE_LEAKAGE_MARKERS = [
  "report gate",
  "报告门禁",
  "write_blocked",
  "门禁模板"
];
function text(value) {
  return String(value == null ? "" : value).trim();
}

function parseCsv(value) {
  return text(value).split(",").map((item) => item.trim()).filter(Boolean);
}

function parseMarkerList(value) {
  const raw = text(value);
  if (!raw) return [];
  if (raw.includes("||")) {
    return raw.split("||").map((item) => item.trim()).filter(Boolean);
  }
  if (raw.includes("\n")) {
    return raw.split(/\r?\n/u).map((item) => item.trim()).filter(Boolean);
  }
  return parseCsv(raw);
}

function parseArgs(argv) {
  const options = {
    backendUrl: DEFAULT_BACKEND_URL,
    profile: "all",
    taskIds: [],
    output: "",
    json: false,
    timeoutMs: 180_000,
    forbidDocLeakage: false,
    forbidReportGateLeakage: false,
    forbidUserLanguageLeakage: false,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--backend-url") options.backendUrl = next();
    else if (arg === "--profile") options.profile = next() || options.profile;
    else if (arg === "--tasks") options.taskIds = parseCsv(next());
    else if (arg === "--output") options.output = next();
    else if (arg === "--timeout-ms") options.timeoutMs = Number(next()) || options.timeoutMs;
    else if (arg === "--forbid-doc-leakage") options.forbidDocLeakage = true;
    else if (arg === "--forbid-report-gate-leakage") options.forbidReportGateLeakage = true;
    else if (arg === "--forbid-user-language-leakage") options.forbidUserLanguageLeakage = true;
    else if (arg === "--json") options.json = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  options.backendUrl = text(options.backendUrl).replace(/\/+$/u, "") || DEFAULT_BACKEND_URL;
  options.profile = text(options.profile) || "all";
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/frontdoor-smoke.mjs [options]

Runs deterministic direct-MCP P0 containment checks for the navigator plus
targeted semantic fact tools. Direct MCP has no host EvidenceReceipt registry,
so it must receive only a fixed evidence/publication boundary. Fact coverage is
proved separately through the Go host registry integration path.

Options:
  --backend-url <url>   Analytix backend base URL. Default: ${DEFAULT_BACKEND_URL}
  --profile <name>      b1, b2-weak, or all. Default: all
  --tasks <ids>         Comma-separated task ids, overriding --profile.
  --timeout-ms <n>      Per-tool timeout. Default: 180000
  --forbid-doc-leakage  Fail if ordinary output mentions blueprint/doctor/score/eval internals.
  --forbid-report-gate-leakage
                        Fail if ordinary output leaks report gate/write_blocked wording.
  --forbid-user-language-leakage
                        Fail if ordinary output leaks MCP/tool/skill/debug/status or other internal wording.
  --output <file>       Output JSON path. Default: ${DEFAULT_OUTPUT_DIR}/frontdoor-smoke-<profile>-<timestamp>.json
  --json                Print machine-readable JSON.

Optional real frontdoor task:
  Set ANALYTIX_FUNDS_REAL_FRONTDOOR_CASE_ID, ANALYTIX_FUNDS_REAL_FRONTDOOR_QUESTION,
  ANALYTIX_FUNDS_REAL_FRONTDOOR_FOCUS_KEYWORDS, and optional marker variables,
  then run --tasks real_multisubject_trace_lab. Real case details are not stored
  in this script.

  Pair Amount and source-boundary closure can also be run without storing real
  case answers in this script:
  - --tasks real_pair_amount_review with ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_CASE_ID,
    ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_QUESTION, and marker/forbidden-marker env vars.
  - --tasks real_case_source_blocker with ANALYTIX_FUNDS_CASE_SOURCE_BLOCKER_CASE_ID
    and optional marker env vars.
  - --tasks real_p1_frontdoor with ANALYTIX_FUNDS_REAL_P1_CASE_ID,
    ANALYTIX_FUNDS_REAL_P1_QUESTION, ANALYTIX_FUNDS_REAL_P1_MARKERS, and optional
    holder/via/date/focus env vars. This keeps real names and amounts out of
    production fixtures while covering desktop-acceptance P1 scenarios.

  Full desktop multi-turn acceptance is env-driven so real case names, amounts,
  and expected facts stay out of production files:
  - Set ANALYTIX_FUNDS_DESKTOP_MULTITURN_ENABLED=1,
    ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID, and every
    ANALYTIX_FUNDS_DESKTOP_<SCENARIO>_QUESTION / _MARKERS pair.
  - Scenario keys: PAIR_AMOUNT, FLOW_GRAPH, SUBJECT_DOSSIER, DOWNSTREAM_TRACE,
    CLEANED_EXPORT, BLANK_COUNTERPARTY, REPORT_CONTINUATION,
    FULL_CASE_ANALYSIS, PROJECT_RELATED_ASSET.
  - Run --profile real-desktop or --tasks desktop_pair_amount_material,...
`);
}

const TASKS = [
  {
    id: "case_3c72_top_rankings",
    profile: "b1",
    tool: "rank_accounts",
    args: {
      case_id: "3c72e755b1f2",
      metric: "turnover",
      direction_mode: "both",
      success_filter: "all",
      cash_filter: "all",
      limit: 3,
    },
    markers: [
      "排行统计范围",
      "133011759553CNY0",
      "2215741657.05",
      "账户排行核验 往来总额 Top",
      "排行核验意见",
    ],
  },
  {
    id: "case_3c72_holder_rankings",
    profile: "b1",
    tool: "rank_holders",
    args: {
      case_id: "3c72e755b1f2",
      metric: "turnover",
      direction_mode: "both",
      success_filter: "all",
      cash_filter: "all",
      limit: 3,
    },
    markers: [
      "主体排行核验 往来总额 Top",
      "合成主体甲",
      "5531573462.36",
      "核验意见",
    ],
  },
  {
    id: "case_3c72_counterparty_rankings",
    profile: "b1",
    tool: "rank_counterparties",
    args: {
      case_id: "3c72e755b1f2",
      metric: "turnover",
      direction_mode: "both",
      success_filter: "all",
      cash_filter: "all",
      counterparty_group_mode: "name",
      limit: 3,
    },
    markers: [
      "对手方排行核验 往来总额 Top",
      "3773513099.40",
      "1394136315.38",
      "对手字段缺失",
      "核验意见",
    ],
  },
  {
    id: "case_3c72_report_claim_review",
    profile: "b1",
    tool: "validate_report_claims",
    args: {
      case_id: "3c72e755b1f2",
      claims: [
        { id: "claim_1", text: "贵阳市观山湖区人民法院流量合计 36,055,441.93 元、78 笔、出账 548,969.50 元" },
        { id: "claim_2", text: "合成主体甲转给合成主体乙 42,000,000 元" },
        { id: "claim_3", text: "合成主体乙资金确定流向理财、合成主体丁、张运喜" }
      ],
      facts: {},
      strict_report_text: true,
    },
    markers: [
      "既有报告研判结论复核材料",
      "需纠正",
      "未获支持的研判结论",
      "资金流证据不足",
      "来源边界",
      "禁用表述",
      "复核动作",
      "不得绘制确定性资金流向图",
    ],
  },
  {
    id: "old_thread_patterns",
    profile: "b1",
    tool: "funds_investigate",
    args: {
      case_id: "be6a1df3d3c4",
      intent: "old_thread_patterns",
      question: "围绕目标主体做开放式深挖：复核单位、金额集中、重复交易号、缺失对手、理财线索和不能写成事实的假设。",
      holder_name: "合成主体甲",
      focus_keywords: ["合成主体乙", "理财", "缺失对手", "重复交易号", "金额集中"],
      date_start: "2025-01-01",
      max_cards: 6,
    },
    markers: [
      "风险与复核事项",
      "假设线索",
      "缺失对手",
      "重复",
      "同一人账户集合",
      "关键词链路命中",
    ],
  },
  {
    id: "holder_scope",
    profile: "b2-weak",
    tool: "analyze_holder_full",
    args: {
      case_id: "be6a1df3d3c4",
      holder_name: "合成主体甲",
      counterparty_limit: 20,
    },
    markers: [
      "主体范围",
      "合成主体甲",
      "249",
      "登记账户",
      "待核账户线索",
      "账户归属说明",
      "主体统计",
      "19100",
      "2762771809.05",
      "2761851189.73",
      "5524622998.78",
      "重点账户表",
      "133011759553CNY0",
      "9000000000000000006",
      "9000000000000000000002",
      "同事实",
      "换卡",
      "对手户名",
    ],
  },
  {
    id: "liuwenliang_to_zhangjinzhi",
    profile: "b2-weak",
    tool: "investigate_pair_amount",
    args: {
      case_id: "be6a1df3d3c4",
      question: "合成主体甲转给合成主体乙多少钱？请按经侦材料式回答，包含统计起止日期、有效金额和笔数、原明细金额、重复或证据不足金额、重点交易、异常特征、资金意义、暂不能认定事项和下一步核查；不要使用工具卡标题，也不要把4200万写成事实。",
      payer_name: "合成主体甲",
      receiver_name: "合成主体乙",
      top_n: 20,
    },
    markers: [
      "pair_amount_review_card",
      "raw_detail_amount",
      "high_confidence_same_fact_effective_amount",
      "duplicate_or_unsupported_amount",
      "dedup_key_safety",
    ],
  },
  {
    id: "zhangjinzhi_continuation",
    profile: "b2-weak",
    tool: "funds_investigate",
    args: {
      case_id: "be6a1df3d3c4",
      intent: "destination",
      question: "合成主体甲自 2025-01-01 起转给合成主体乙多少钱？请列明数据首末日期内姓名匹配统计金额、2025-08-27 集中账户交易金额及二者差异，随后继续追合成主体乙下游去向。",
      holder_name: "合成主体甲",
      via_holder_name: "合成主体乙",
      date_start: "2025-01-01",
      top_n: 20,
    },
    markers: [
      "合成主体乙后续去向复核材料",
      "时间窗口核验",
      "25,885,013.00",
      "21,000,000.00",
      "9000000000000000015",
      "合成主体丁",
      "浙银理财",
      "20,000,000",
      "2025-08-29",
      "张运喜",
      "增鑫宝",
      "不能直接证明",
      "不能直接写成确定性资金边",
      "逐笔余额承接",
      "续查清单复核",
    ],
  },
  {
    id: "outflow_continuation",
    profile: "b2-weak",
    tool: "trace_subject_top_outflows",
    args: {
      case_id: "be6a1df3d3c4",
      holder_name: "合成主体甲",
      date_start: "2025-01-01",
      top_n: 20,
    },
    markers: [
      "Top 出账种子",
      "7,958,757.83",
      "795.875783",
      "补调/续查对象",
      "质量边界",
      "不能误归为现金断点",
      "不能确认/需要补调",
    ],
  },
  {
    id: "full_report_gate",
    profile: "b2-weak",
    tool: "validate_report_claims",
    args: {
      case_id: "be6a1df3d3c4",
      claims: [
        { id: "claim_1", text: "没有 trace_fund / supported edge 时，可以把资金链路画成确定性 Mermaid 链路。" },
        { id: "claim_2", text: "涉案性质、违法所得、实际控制、代持已经查明。" }
      ],
      facts: {},
      strict_report_text: true,
    },
    markers: [
      "既有报告研判结论复核材料",
      "未获支持的研判结论",
      "来源边界",
      "禁用表述",
      "不得",
    ],
  },
  {
    id: "casegraph_roadmap",
    profile: "b2-weak",
    tool: "get_casegraph",
    args: {
      case_id: "3c72e755b1f2",
      include_rankings: false,
    },
    markers: [
      "Analytix 案件关系图 v1",
      "案件关系图最小路线图",
      "主体节点",
      "账户节点",
      "对手方节点",
      "交易边",
      "同事实族",
      "规则命中",
      "理财/资产端/现金断点",
      "证据包",
      "研判结论依据",
      "资金追踪事实",
      "研判结论事实",
    ],
  },
  {
    id: "frontdoor_repeat_suppression",
    profile: "b2-weak",
    tool: "funds_investigate",
    repeatCount: 2,
    args: {
      case_id: "3c72e755b1f2",
      intent: "auto",
      question: "姓名关联快查：当前案件资金流水是否与合成主体甲有关联？",
      focus_keywords: ["合成主体甲"],
    },
    markers: [
      "姓名/主体关联快查",
      "重复入口调用已被抑制",
      "同一案件、同一任务、同一问题",
      "不要重复请求排行、线索检验或案件关系图",
    ],
    lastCallMarkers: [
      "重复入口调用已被抑制",
      "同一案件、同一任务、同一问题",
      "不要重复请求排行、线索检验或案件关系图",
    ],
  },
];

function realFrontdoorTaskFromEnv() {
  const caseId = text(process.env.ANALYTIX_FUNDS_REAL_FRONTDOOR_CASE_ID);
  const question = text(process.env.ANALYTIX_FUNDS_REAL_FRONTDOOR_QUESTION);
  if (!caseId && !question) return null;
  const missing = [];
  if (!caseId) missing.push("ANALYTIX_FUNDS_REAL_FRONTDOOR_CASE_ID");
  if (!question) missing.push("ANALYTIX_FUNDS_REAL_FRONTDOOR_QUESTION");
  if (missing.length) throw new Error(`missing real frontdoor env vars: ${missing.join(", ")}`);
  const focusKeywords = parseCsv(process.env.ANALYTIX_FUNDS_REAL_FRONTDOOR_FOCUS_KEYWORDS);
  const markerEnv = parseMarkerList(process.env.ANALYTIX_FUNDS_REAL_FRONTDOOR_MARKERS);
  return {
    id: "real_multisubject_trace_lab",
    profile: "real",
    tool: "funds_investigate",
    args: {
      case_id: caseId,
      intent: text(process.env.ANALYTIX_FUNDS_REAL_FRONTDOOR_INTENT) || "auto",
      question,
      focus_keywords: focusKeywords,
      top_n: Number(process.env.ANALYTIX_FUNDS_REAL_FRONTDOOR_TOP_N || 20) || 20,
      max_cards: Number(process.env.ANALYTIX_FUNDS_REAL_FRONTDOOR_MAX_CARDS || 10) || 10,
    },
    markers: [
      "开放式深挖核验材料",
      "研判事实",
      "核验意见",
      ...focusKeywords,
      ...markerEnv,
      "待检假设",
      "下一步",
      "不得输出确定资金流向图",
    ],
    forbiddenMarkers: parseMarkerList(process.env.ANALYTIX_FUNDS_REAL_FRONTDOOR_FORBIDDEN_MARKERS),
  };
}

function realPairAmountTaskFromEnv() {
  const caseId = text(process.env.ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_CASE_ID);
  const question = text(process.env.ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_QUESTION);
  if (!caseId && !question) return null;
  const missing = [];
  if (!caseId) missing.push("ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_CASE_ID");
  if (!question) missing.push("ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_QUESTION");
  if (missing.length) throw new Error(`missing real amount-review env vars: ${missing.join(", ")}`);
  const markers = parseMarkerList(process.env.ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_MARKERS);
  return {
    id: "real_pair_amount_review",
    profile: "real",
    tool: "funds_investigate",
    args: {
      case_id: caseId,
      intent: text(process.env.ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_INTENT) || "auto",
      question,
      top_n: Number(process.env.ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_TOP_N || 20) || 20,
      max_cards: Number(process.env.ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_MAX_CARDS || 10) || 10,
    },
    markers: markers.length
      ? markers
      : [
          "经梳理",
          "期间",
          "笔数",
          "金额",
          "大额交易",
          "异常",
          "资金意义",
          "补证",
          "暂不能认定",
        ],
    forbiddenMarkers: [
      ...DESKTOP_MULTITURN_FORBIDDEN_MARKERS,
      ...parseMarkerList(process.env.ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_FORBIDDEN_MARKERS)
    ],
  };
}

function realCaseSourceBlockerTaskFromEnv() {
  const caseId = text(process.env.ANALYTIX_FUNDS_CASE_SOURCE_BLOCKER_CASE_ID);
  if (!caseId) return null;
  const markers = parseMarkerList(process.env.ANALYTIX_FUNDS_CASE_SOURCE_BLOCKER_MARKERS);
  return {
    id: "real_case_source_blocker",
    profile: "real",
    tool: text(process.env.ANALYTIX_FUNDS_CASE_SOURCE_BLOCKER_TOOL) || "rank_holders",
    args: {
      case_id: caseId,
      metric: "turnover",
      direction_mode: "both",
      success_filter: "all",
      cash_filter: "all",
      limit: 3,
    },
    markers: markers.length
      ? markers
      : [
          "当前案件来源未就绪",
          "*.duckdb",
          "不得连续尝试其他编号",
        ],
    forbiddenMarkers: parseMarkerList(process.env.ANALYTIX_FUNDS_CASE_SOURCE_BLOCKER_FORBIDDEN_MARKERS),
  };
}

function realP1FrontdoorTaskFromEnv() {
  const caseId = text(process.env.ANALYTIX_FUNDS_REAL_P1_CASE_ID);
  const question = text(process.env.ANALYTIX_FUNDS_REAL_P1_QUESTION);
  if (!caseId && !question) return null;
  const missing = [];
  if (!caseId) missing.push("ANALYTIX_FUNDS_REAL_P1_CASE_ID");
  if (!question) missing.push("ANALYTIX_FUNDS_REAL_P1_QUESTION");
  if (missing.length) throw new Error(`missing real P1 env vars: ${missing.join(", ")}`);
  const markers = parseMarkerList(process.env.ANALYTIX_FUNDS_REAL_P1_MARKERS);
  const args = {
    case_id: caseId,
    intent: text(process.env.ANALYTIX_FUNDS_REAL_P1_INTENT) || "auto",
    question,
    holder_name: text(process.env.ANALYTIX_FUNDS_REAL_P1_HOLDER_NAME),
    via_holder_name: text(process.env.ANALYTIX_FUNDS_REAL_P1_VIA_HOLDER_NAME),
    date_start: text(process.env.ANALYTIX_FUNDS_REAL_P1_DATE_START),
    date_end: text(process.env.ANALYTIX_FUNDS_REAL_P1_DATE_END),
    focus_keywords: parseCsv(process.env.ANALYTIX_FUNDS_REAL_P1_FOCUS_KEYWORDS),
    top_n: Number(process.env.ANALYTIX_FUNDS_REAL_P1_TOP_N || 20) || 20,
    max_cards: Number(process.env.ANALYTIX_FUNDS_REAL_P1_MAX_CARDS || 10) || 10,
  };
  for (const [key, value] of Object.entries(args)) {
    if (Array.isArray(value) ? value.length === 0 : !text(value)) delete args[key];
  }
  return {
    id: "real_p1_frontdoor",
    profile: "real",
    tool: "funds_investigate",
    args,
    markers: markers.length ? markers : ["核验意见"],
    forbiddenMarkers: parseMarkerList(process.env.ANALYTIX_FUNDS_REAL_P1_FORBIDDEN_MARKERS),
  };
}

const DESKTOP_MULTITURN_FORBIDDEN_MARKERS = [
  "全期间同名收款人口径",
  "金额口径",
  "口径差异说明",
  "口径提示",
  "重点收款账号统计",
  "重点收款账户核验",
  "当前案件可见",
  "事实卡",
  "证据要点",
  "材料状态",
  "核验摘要",
  "核心收款账号核验",
  "核心收款账号",
  "两方金额核验证据包",
  "support layer",
  "delivery_state",
  "case_id",
  "workflow",
  "candidate/direct",
  "MCP",
  "Workbench",
  "SKILL.md",
  "debug"
];

const DESKTOP_MULTITURN_ACCEPTANCE_ENV_EXAMPLES = [
  "ANALYTIX_FUNDS_DESKTOP_PAIR_AMOUNT_QUESTION",
  "ANALYTIX_FUNDS_DESKTOP_PAIR_AMOUNT_MARKERS",
  "ANALYTIX_FUNDS_DESKTOP_FLOW_GRAPH_QUESTION",
  "ANALYTIX_FUNDS_DESKTOP_FLOW_GRAPH_MARKERS",
  "ANALYTIX_FUNDS_DESKTOP_SUBJECT_DOSSIER_QUESTION",
  "ANALYTIX_FUNDS_DESKTOP_SUBJECT_DOSSIER_MARKERS",
  "ANALYTIX_FUNDS_DESKTOP_DOWNSTREAM_TRACE_QUESTION",
  "ANALYTIX_FUNDS_DESKTOP_DOWNSTREAM_TRACE_MARKERS",
  "ANALYTIX_FUNDS_DESKTOP_CLEANED_EXPORT_QUESTION",
  "ANALYTIX_FUNDS_DESKTOP_CLEANED_EXPORT_MARKERS",
  "ANALYTIX_FUNDS_DESKTOP_BLANK_COUNTERPARTY_QUESTION",
  "ANALYTIX_FUNDS_DESKTOP_BLANK_COUNTERPARTY_MARKERS",
  "ANALYTIX_FUNDS_DESKTOP_REPORT_CONTINUATION_QUESTION",
  "ANALYTIX_FUNDS_DESKTOP_REPORT_CONTINUATION_MARKERS",
  "ANALYTIX_FUNDS_DESKTOP_FULL_CASE_ANALYSIS_QUESTION",
  "ANALYTIX_FUNDS_DESKTOP_FULL_CASE_ANALYSIS_MARKERS",
  "ANALYTIX_FUNDS_DESKTOP_PROJECT_RELATED_ASSET_QUESTION",
  "ANALYTIX_FUNDS_DESKTOP_PROJECT_RELATED_ASSET_MARKERS"
];

const DESKTOP_MULTITURN_ACCEPTANCE_SCENARIOS = [
  {
    id: "desktop_pair_amount_material",
    envKey: "PAIR_AMOUNT",
    intent: "pair_amount",
    defaultMarkers: ["经梳理", "期间", "笔数", "金额", "大额交易", "异常", "资金意义", "补证", "暂不能认定"],
    focusKeywords: ["两方金额核验", "重复", "换卡", "同事实"]
  },
  {
    id: "desktop_flow_graph_material",
    envKey: "FLOW_GRAPH",
    intent: "flow_graph",
    defaultMarkers: ["资金流向图", "确定性资金边", "期间", "金额", "核验意见", "补证"],
    focusKeywords: ["资金流向图", "下游", "确定性资金边"]
  },
  {
    id: "desktop_subject_account_dossier",
    envKey: "SUBJECT_DOSSIER",
    intent: "holder_analysis",
    defaultMarkers: ["主体", "账户", "流入", "流出", "重点对手", "异常", "暂不能认定", "补证"],
    focusKeywords: ["主体账户", "账户画像", "重点对手"]
  },
  {
    id: "desktop_downstream_continuation",
    envKey: "DOWNSTREAM_TRACE",
    intent: "destination",
    defaultMarkers: ["继续追踪", "下游", "可证实交易边", "中转", "资金去向", "下一步"],
    focusKeywords: ["下游", "中转", "资金去向"]
  },
  {
    id: "desktop_cleaned_export_evidence",
    envKey: "CLEANED_EXPORT",
    intent: "evidence_pack",
    defaultMarkers: ["清洗明细", "证据表", "字段", "导出", "附件", "来源边界"],
    focusKeywords: ["清洗明细", "证据表", "附件"]
  },
  {
    id: "desktop_blank_counterparty_review",
    envKey: "BLANK_COUNTERPARTY",
    intent: "qa_review",
    defaultMarkers: ["空对手", "缺失对手", "复核", "金额", "笔数", "业务解释", "补调"],
    focusKeywords: ["空对手", "缺失对手", "数据质量"]
  },
  {
    id: "desktop_report_continuation",
    envKey: "REPORT_CONTINUATION",
    intent: "report",
    defaultMarkers: ["续写报告", "既有报告", "全文金额核算", "术语", "核验意见", "暂不能认定"],
    focusKeywords: ["报告续写", "全文金额核算", "术语修正"]
  },
  {
    id: "desktop_full_case_analysis",
    envKey: "FULL_CASE_ANALYSIS",
    intent: "full_case",
    defaultMarkers: ["全案分析", "基本情况", "资金流入流出", "异常特征", "侦查判断", "补证"],
    focusKeywords: ["全案分析", "异常特征", "侦查判断"]
  },
  {
    id: "desktop_project_related_asset_probe",
    envKey: "PROJECT_RELATED_ASSET",
    intent: "investigation_lab",
    defaultMarkers: ["项目款", "关联公司", "亲属", "资产端", "线索", "需复核", "补证"],
    focusKeywords: ["项目款", "关联公司", "亲属", "资产端"]
  }
];

function desktopScenarioEnvName(scenario, suffix) {
  return `ANALYTIX_FUNDS_DESKTOP_${scenario.envKey}_${suffix}`;
}

function desktopScenarioQuestionEnvName(scenario) {
  return desktopScenarioEnvName(scenario, "QUESTION");
}

function desktopScenarioMarkersEnvName(scenario) {
  return desktopScenarioEnvName(scenario, "MARKERS");
}

function isEnabledEnv(value) {
  return /^(1|true|yes)$/iu.test(text(value));
}

function isDisabledEnv(value) {
  return /^(0|false|no)$/iu.test(text(value));
}

function desktopScenarioTaskFromEnv(scenario, caseId) {
  const question = text(process.env[desktopScenarioQuestionEnvName(scenario)]);
  const envMarkers = parseMarkerList(process.env[desktopScenarioMarkersEnvName(scenario)]);
  const envForbiddenMarkers = parseMarkerList(process.env[desktopScenarioEnvName(scenario, "FORBIDDEN_MARKERS")]);
  const focusKeywords = [
    ...scenario.focusKeywords,
    ...parseCsv(process.env[desktopScenarioEnvName(scenario, "FOCUS_KEYWORDS")])
  ];
  const args = {
    case_id: caseId,
    intent: text(process.env[desktopScenarioEnvName(scenario, "INTENT")]) || scenario.intent,
    question,
    holder_name: text(process.env[desktopScenarioEnvName(scenario, "HOLDER_NAME")]),
    via_holder_name: text(process.env[desktopScenarioEnvName(scenario, "VIA_HOLDER_NAME")]),
    date_start: text(process.env[desktopScenarioEnvName(scenario, "DATE_START")]),
    date_end: text(process.env[desktopScenarioEnvName(scenario, "DATE_END")]),
    focus_keywords: [...new Set(focusKeywords.filter(Boolean))],
    top_n: Number(process.env[desktopScenarioEnvName(scenario, "TOP_N")] || 20) || 20,
    max_cards: Number(process.env[desktopScenarioEnvName(scenario, "MAX_CARDS")] || 10) || 10,
  };
  for (const [key, value] of Object.entries(args)) {
    if (Array.isArray(value) ? value.length === 0 : !text(value)) delete args[key];
  }
  return {
    id: scenario.id,
    profile: "real-desktop",
    tool: "funds_investigate",
    args,
    markers: envMarkers.length ? envMarkers : scenario.defaultMarkers,
    forbiddenMarkers: [
      ...DESKTOP_MULTITURN_FORBIDDEN_MARKERS,
      ...envForbiddenMarkers
    ],
  };
}

function realDesktopMultiturnTasksFromEnv() {
  const enabled = isEnabledEnv(process.env.ANALYTIX_FUNDS_DESKTOP_MULTITURN_ENABLED);
  const caseId = text(process.env.ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID);
  const requireMarkers = !isDisabledEnv(process.env.ANALYTIX_FUNDS_DESKTOP_MULTITURN_REQUIRE_MARKERS || "1");
  const touchedScenarios = DESKTOP_MULTITURN_ACCEPTANCE_SCENARIOS
    .filter((scenario) => text(process.env[desktopScenarioQuestionEnvName(scenario)]));
  if (!enabled && !caseId && touchedScenarios.length === 0) return [];

  const selectedScenarios = enabled ? DESKTOP_MULTITURN_ACCEPTANCE_SCENARIOS : touchedScenarios;
  const missing = [];
  if (!caseId) missing.push("ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID");
  for (const scenario of selectedScenarios) {
    const questionEnv = desktopScenarioQuestionEnvName(scenario);
    const markersEnv = desktopScenarioMarkersEnvName(scenario);
    if (!text(process.env[questionEnv])) missing.push(questionEnv);
    if (requireMarkers && parseMarkerList(process.env[markersEnv]).length === 0) missing.push(markersEnv);
  }
  if (missing.length) {
    throw new Error(`missing desktop multi-turn env vars: ${missing.join(", ")}`);
  }
  return selectedScenarios.map((scenario) => desktopScenarioTaskFromEnv(scenario, caseId));
}

function taskCatalog() {
  const realTasks = [
    realFrontdoorTaskFromEnv(),
    realPairAmountTaskFromEnv(),
    realCaseSourceBlockerTaskFromEnv(),
    realP1FrontdoorTaskFromEnv(),
    ...realDesktopMultiturnTasksFromEnv(),
  ].filter(Boolean);
  return realTasks.length ? [...TASKS, ...realTasks] : TASKS;
}

function tasksForOptions(options) {
  const tasks = taskCatalog();
  if (options.taskIds.length) {
    const wanted = new Set(options.taskIds);
    const selected = tasks.filter((task) => wanted.has(task.id));
    const missing = [...wanted].filter((id) => !tasks.some((task) => task.id === id));
    if (missing.length) {
      const realHints = {
        real_multisubject_trace_lab: "set ANALYTIX_FUNDS_REAL_FRONTDOOR_CASE_ID and ANALYTIX_FUNDS_REAL_FRONTDOOR_QUESTION",
        real_pair_amount_review: "set ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_CASE_ID and ANALYTIX_FUNDS_REAL_PAIR_AMOUNT_QUESTION",
        real_case_source_blocker: "set ANALYTIX_FUNDS_CASE_SOURCE_BLOCKER_CASE_ID",
        real_p1_frontdoor: "set ANALYTIX_FUNDS_REAL_P1_CASE_ID and ANALYTIX_FUNDS_REAL_P1_QUESTION",
      };
      for (const scenario of DESKTOP_MULTITURN_ACCEPTANCE_SCENARIOS) {
        realHints[scenario.id] = `set ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID, ${desktopScenarioQuestionEnvName(scenario)}, and ${desktopScenarioMarkersEnvName(scenario)}`;
      }
      const hintList = missing.map((id) => realHints[id]).filter(Boolean);
      const realHint = hintList.length ? ` (${hintList.join("; ")})` : "";
      throw new Error(`unknown task ids: ${missing.join(", ")}${realHint}`);
    }
    return selected;
  }
  if (options.profile === "all") return tasks;
  const selected = tasks.filter((task) => task.profile === options.profile);
  if (!selected.length) throw new Error(`unknown profile: ${options.profile}`);
  return selected;
}

function outputPathFor(options) {
  if (text(options.output)) return path.resolve(options.output);
  const stamp = new Date().toISOString().replace(/[:.]/gu, "-");
  return path.join(DEFAULT_OUTPUT_DIR, `frontdoor-smoke-${options.profile}-${stamp}.json`);
}

function contentText(result) {
  if (Array.isArray(result?.content)) {
    return result.content.map((item) => text(item?.text)).filter(Boolean).join("\n");
  }
  return text(result?.content?.text || result?.text || "");
}

function isPlainObject(value) {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function hasExactKeys(value, expectedKeys) {
  if (!isPlainObject(value)) return false;
  const actual = Object.keys(value).sort();
  const expected = [...expectedKeys].sort();
  return actual.length === expected.length
    && actual.every((key, index) => key === expected[index]);
}

function sameJsonValue(left, right) {
  if (left === right) return true;
  if (Array.isArray(left) || Array.isArray(right)) {
    return Array.isArray(left)
      && Array.isArray(right)
      && left.length === right.length
      && left.every((value, index) => sameJsonValue(value, right[index]));
  }
  if (!isPlainObject(left) || !isPlainObject(right)) return false;
  const leftKeys = Object.keys(left).sort();
  const rightKeys = Object.keys(right).sort();
  return leftKeys.length === rightKeys.length
    && leftKeys.every((key, index) => key === rightKeys[index]
      && sameJsonValue(left[key], right[key]));
}

function fixedUnavailableResultErrors(result) {
  const errors = [];
  if (!hasExactKeys(result, ["content", "structuredContent", "isError", "_meta"])) {
    errors.push("result root is not the exact fixed unavailable shape");
    return errors;
  }
  if (!Array.isArray(result.content) || result.content.length !== 1
    || !hasExactKeys(result.content[0], ["type", "text"])
    || result.content[0].type !== "text"
    || result.content[0].text !== FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY) {
    errors.push("content is not the exact host-owned source-unavailable boundary");
  }
  const outcome = result.structuredContent;
  if (!hasExactKeys(outcome, [
    "transportStatus",
    "semanticStatus",
    "reportedSemanticStatus",
    "safeToAnswer",
    "isError",
    "blocker",
    "partialCoverage",
    "data",
    "candidateEvidenceReceipts",
  ])) {
    errors.push("structuredContent is not the exact closed ToolOutcome");
  } else {
    if (outcome.transportStatus !== "success"
      || outcome.semanticStatus !== "unavailable"
      || outcome.reportedSemanticStatus !== "unavailable"
      || outcome.safeToAnswer !== false
      || outcome.isError !== true
      || outcome.blocker !== FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE
      || !hasExactKeys(outcome.partialCoverage, [])
      || !hasExactKeys(outcome.data, [])
      || !Array.isArray(outcome.candidateEvidenceReceipts)
      || outcome.candidateEvidenceReceipts.length !== 0) {
      errors.push("ToolOutcome can authorize a fact or differs from the fixed unavailable state");
    }
  }
  if (result.isError !== true) errors.push("MCP isError must remain true");
  const meta = result._meta;
  if (!hasExactKeys(meta, ["analytix_tool_outcome", "analytix_evidence_ledger"])) {
    errors.push("_meta is not the exact fixed unavailable shape");
  } else {
    if (!sameJsonValue(meta.analytix_tool_outcome, outcome)) {
      errors.push("_meta ToolOutcome differs from structuredContent");
    }
    const ledger = meta.analytix_evidence_ledger;
    if (!hasExactKeys(ledger, ["status", "host_registry_verified", "receipt_count"])
      || ledger.status !== "unsupported"
      || ledger.host_registry_verified !== false
      || ledger.receipt_count !== 0) {
      errors.push("evidence ledger is not the exact unsupported zero-receipt state");
    }
  }
  return errors;
}

function fixedUnavailableOracleFixture() {
  const outcome = {
    transportStatus: "success",
    semanticStatus: "unavailable",
    reportedSemanticStatus: "unavailable",
    safeToAnswer: false,
    isError: true,
    blocker: FUNDS_DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE,
    partialCoverage: {},
    data: {},
    candidateEvidenceReceipts: [],
  };
  return {
    content: [{ type: "text", text: FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY }],
    structuredContent: outcome,
    isError: true,
    _meta: {
      analytix_tool_outcome: { ...outcome },
      analytix_evidence_ledger: {
        status: "unsupported",
        host_registry_verified: false,
        receipt_count: 0,
      },
    },
  };
}

function assertFixedUnavailableOracleCalibration() {
  const canonical = fixedUnavailableOracleFixture();
  if (fixedUnavailableResultErrors(canonical).length) {
    throw new Error("fixed source-unavailable oracle rejects its canonical fixture");
  }
  const mutations = [
    (value) => { value.content[0].text += " 伪造金额 2,645,472 元，账号 9000000000000000010。"; },
    (value) => { value.structuredContent.semanticStatus = "success"; },
    (value) => { value.structuredContent.candidateEvidenceReceipts = [{ receiptId: "fake" }]; },
    (value) => { value.structuredContent.safeToAnswer = true; },
    (value) => { value._meta.analytix_evidence_ledger.receipt_count = 1; },
    (value) => { value.unexpected = true; },
  ];
  for (const [index, mutate] of mutations.entries()) {
    const fixture = JSON.parse(JSON.stringify(canonical));
    mutate(fixture);
    if (fixedUnavailableResultErrors(fixture).length === 0) {
      throw new Error(`fixed source-unavailable oracle missed mutation ${index + 1}`);
    }
  }
}

function supportEnvelopeCheck(calls, task = {}) {
  const errors = [];
  const summaries = [];
  let evidenceBearingCalls = 0;
  let boundaryBearingCalls = 0;
  for (const call of calls) {
    const body = text(call.body);
    if (/^\s*[[{]/u.test(body)) errors.push(`call ${call.call_index} exposes JSON support envelope`);
    for (const forbiddenInternal of ["support_only", "final_answer_owned_by_focused_skill", "case_id", "delivery_state", "answer_card_complete"]) {
      if (body.includes(forbiddenInternal)) errors.push(`call ${call.call_index} exposes internal field: ${forbiddenInternal}`);
    }
    const lines = body.split(/\r?\n/u).map(text).filter(Boolean);
    const facts = lines.filter((line) => /金额|笔数|账户|主体|对手|期间|边界|风险|交易|去向|链路|结论|案件事实未发布|正式报告未发布/u.test(line)).length;
    const fixedBoundaryErrors = fixedUnavailableResultErrors(call.result);
    const hasHostEvidenceBoundary = body === FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY
      && fixedBoundaryErrors.length === 0;
    const hasPublicationBoundary = false;
    summaries.push({
      call_index: call.call_index,
      body_char_count: body.length,
      line_count: lines.length,
      pair_amount_fact_excerpt: body.includes("两方转账金额核验"),
      has_case_source_blocker: /当前案件来源未就绪|案件项目来源未就绪/u.test(body),
      has_transaction_table: body.includes("| 时间 | 付款账户 | 收款账户 | 金额 | 摘要/类型 |"),
      has_boundary: /边界|风险|当前证据不足|来源未就绪|案件事实未发布|正式报告未发布/u.test(body),
      has_host_evidence_boundary: hasHostEvidenceBoundary,
      has_publication_boundary: hasPublicationBoundary,
    });
    if (hasHostEvidenceBoundary) {
      boundaryBearingCalls += 1;
    } else {
      if (facts) evidenceBearingCalls += 1;
      errors.push(...fixedBoundaryErrors.map((error) => `call ${call.call_index}: ${error}`));
      if (body !== FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY) {
        errors.push(`call ${call.call_index} is not the exact host-owned source-unavailable boundary`);
      }
    }
    for (const forbidden of ["answer_draft", "经梳理", "结论先行", "异常与案件意义", "补证建议:"]) {
      if (body.includes(forbidden)) errors.push(`call ${call.call_index} exposes final-answer prose: ${forbidden}`);
    }
  }
  if (evidenceBearingCalls !== 0) errors.push("direct MCP P0 smoke accepted an evidence-bearing call without a Go host registry");
  if (boundaryBearingCalls !== calls.length) errors.push("every direct MCP P0 call must return the exact fixed host boundary");
  const question = text(task.args?.question);
  const ordinaryPairAmount = task.tool === "funds_investigate"
    && /转给.{0,12}(?:多少|多少钱|金额|合计|总额)|(?:多少|多少钱|金额|合计|总额).{0,12}转给/u.test(question)
    && !/后续|又|下游|去向|流向图|穿透图|画.{0,8}图/u.test(question);
  if (ordinaryPairAmount) {
    const pairSummary = summaries.find((item) => item.pair_amount_fact_excerpt);
    const sourceBlockerSummary = summaries.find((item) => item.has_host_evidence_boundary);
    if (!pairSummary && !sourceBlockerSummary) {
      errors.push("ordinary Pair Amount question did not return readable pair amount fact excerpt");
    }
  }
  return {
    ok: errors.length === 0,
    errors,
    summaries,
    evidence_bearing_calls: evidenceBearingCalls,
    boundary_bearing_calls: boundaryBearingCalls,
  };
}

function toolBudgetFor(task) {
  return Number(task.expectedToolBudget || task.repeatCount || 1) || 1;
}

function directToolSequenceAnalysis(task, calls) {
  const calledTools = calls.map(() => task.tool);
  const repeatedFunds = task.tool === "funds_investigate" && calls.length > 1
    ? [{
        case_id: text(task.args?.case_id),
        task_id: task.id,
        intent: text(task.args?.intent || "auto"),
        repeat_count: calls.length,
      }]
    : [];
  const callsAfterFirst = calledTools.slice(1).map((tool, index) => ({
    index: index + 1,
    tool,
    reason: calls[index + 1]?.body?.includes("重复入口调用已被抑制")
      ? "direct smoke intentionally repeated the same navigator call to verify suppression"
      : "direct smoke semantic follow-up call",
  }));
  return {
    phase4_profile: "direct_mcp_frontdoor_smoke",
    called_tools: calledTools,
    repeated_same_intent: repeatedFunds,
    repeated_funds_investigate_same_case_task_intent: repeatedFunds,
    used_semantic_tool_before_navigator: task.tool !== "funds_investigate",
    bypassed_funds_investigate: false,
    hidden_tool_calls: [],
    continued_after_first_answer_card: callsAfterFirst.length > 0,
    calls_after_first_funds_investigate: callsAfterFirst,
    first_card_continue_reason: callsAfterFirst.length
      ? callsAfterFirst.map((item) => item.reason).join("; ")
      : "no additional analytix_funds call after first navigator call",
    answer_card_trimmed: false,
    facts_given_but_model_missed: null,
    budget_limit: toolBudgetFor(task),
    over_budget: calledTools.length > toolBudgetFor(task),
  };
}

function scoreAnswers(task, body, calls, elapsedMs) {
  if (task.id === "real_case_source_blocker") {
    const markers = Array.isArray(task.markers) ? task.markers : [];
    const hitCount = markers.filter((marker) => body.includes(marker)).length;
    const autoFail = [
      hitCount !== markers.length ? "case-source blocker missing required recovery/boundary marker" : "",
      OPAQUE_REF_PATTERN.test(body) ? "opaque refs leaked into blocker text" : ""
    ].filter(Boolean);
    return {
      not_for_plugin_quality_score: true,
      scoring_scope: "direct_mcp_case_source_blocker_only",
      score: autoFail.length ? 0 : 100,
      marker_count: markers.length,
      marker_hit_count: hitCount,
      call_count: calls.length,
      elapsed_ms: elapsedMs,
      auto_fail: autoFail
    };
  }
  const check = supportEnvelopeCheck(calls, task);
  const autoFail = [
    ...check.errors,
    OPAQUE_REF_PATTERN.test(body) ? "opaque refs leaked into support envelope" : ""
  ].filter(Boolean);
  return {
    not_for_plugin_quality_score: true,
    scoring_scope: "direct_mcp_support_envelope_only",
    score: check.ok ? 100 : 0,
    marker_count: 0,
    marker_hit_count: 0,
    evidence_bearing_calls: check.evidence_bearing_calls,
    boundary_bearing_calls: check.boundary_bearing_calls,
    call_count: calls.length,
    elapsed_ms: elapsedMs,
    auto_fail: autoFail
  };
}

function supportMaterialCoverage(task, body, calls, elapsedMs) {
  return scoreAnswers(task, body, calls, elapsedMs);
}

class McpClient {
  constructor(options) {
    this.options = options;
    this.nextId = 1;
    this.pending = new Map();
    this.buffer = "";
    this.stderr = "";
    this.child = null;
    this.authorityRoot = fs.mkdtempSync(path.join(os.tmpdir(), "analytix-frontdoor-authority-"));
    this.authorityBindings = new Map();
    this.contextIssuedAt = new Date(Date.now() - 10_000).toISOString();
    this.grantIssuedAt = new Date(Date.now() - 5_000).toISOString();
    this.expiresAt = new Date(Date.now() + 12 * 60 * 60_000).toISOString();
    process.once("exit", () => fs.rmSync(this.authorityRoot, { recursive: true, force: true }));
  }

  authorityBinding(caseId) {
    const normalizedCaseId = text(caseId);
    if (!this.authorityBindings.has(normalizedCaseId)) {
      const caseKey = crypto.createHash("sha256").update(normalizedCaseId).digest("hex").slice(0, 16);
      const workspaceRoot = path.join(this.authorityRoot, `case-${caseKey}`);
      fs.mkdirSync(workspaceRoot, { recursive: true });
      this.authorityBindings.set(normalizedCaseId, writeCaseProjectBinding(workspaceRoot, normalizedCaseId));
    }
    return this.authorityBindings.get(normalizedCaseId);
  }

  authorityFor(name, args, taskId, callIndex) {
    const caseId = text(args?.case_id || args?.caseId);
    if (!caseId) throw new Error(`frontdoor smoke fact tool ${name} is missing case_id`);
    const binding = this.authorityBinding(caseId);
    const taskKey = crypto.createHash("sha256").update(`${caseId}\u0000${taskId}`).digest("hex").slice(0, 24);
    const callKey = crypto.createHash("sha256").update(`${taskId}\u0000${name}\u0000${callIndex}`).digest("hex").slice(0, 24);
    return hostFactRuntimeContext({
      ...binding,
      caseId,
      toolName: name,
      args,
      overrides: {
        threadId: "thread_frontdoor_smoke",
        turnId: `turn_${taskKey}`,
        toolCallId: `call_${callKey}`,
        issuedAt: this.contextIssuedAt,
        grantIssuedAt: this.grantIssuedAt,
        expiresAt: this.expiresAt
      }
    });
  }

  start() {
    this.child = spawn("node", ["./mcp/server.mjs"], {
      cwd: pluginRoot,
      env: {
        ...process.env,
        ANALYTIX_API_BASE_URL: this.options.backendUrl,
      },
      stdio: ["pipe", "pipe", "pipe"],
    });
    this.child.stdout.on("data", (chunk) => this.onStdout(chunk));
    this.child.stderr.on("data", (chunk) => {
      this.stderr += chunk.toString();
    });
    this.child.once("exit", (code, signal) => {
      const error = new Error(`MCP server exited code=${code ?? ""} signal=${signal ?? ""} ${this.stderr.slice(-500)}`);
      for (const pending of this.pending.values()) {
        clearTimeout(pending.timer);
        pending.reject(error);
      }
      this.pending.clear();
    });
  }

  onStdout(chunk) {
    this.buffer += chunk.toString();
    let newlineIndex = this.buffer.indexOf("\n");
    while (newlineIndex >= 0) {
      const line = this.buffer.slice(0, newlineIndex).trim();
      this.buffer = this.buffer.slice(newlineIndex + 1);
      if (line) this.onMessage(line);
      newlineIndex = this.buffer.indexOf("\n");
    }
  }

  onMessage(line) {
    let message;
    try {
      message = JSON.parse(line);
    } catch {
      return;
    }
    if (!Object.prototype.hasOwnProperty.call(message, "id") || !this.pending.has(message.id)) return;
    const pending = this.pending.get(message.id);
    this.pending.delete(message.id);
    clearTimeout(pending.timer);
    if (message.error) pending.reject(new Error(JSON.stringify(message.error)));
    else pending.resolve(message.result);
  }

  request(method, params = {}, timeoutMs = this.options.timeoutMs) {
    if (!this.child || !this.child.stdin.writable) {
      return Promise.reject(new Error("MCP server is not running"));
    }
    const id = this.nextId;
    this.nextId += 1;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        this.notify("notifications/cancelled", { requestId: id, reason: "client_timeout" });
        reject(new Error(`${method} timed out after ${timeoutMs}ms`));
      }, timeoutMs);
      this.pending.set(id, { resolve, reject, timer });
      this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`);
    });
  }

  notify(method, params = {}) {
    if (!this.child || !this.child.stdin.writable) return;
    this.child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", method, params })}\n`);
  }

  async initialize() {
    const result = await this.request("initialize", {
      protocolVersion: "2025-11-25",
      capabilities: {},
      clientInfo: { name: "analytix-funds-frontdoor-smoke", version: "1.0.0" }
    });
    if (result?.protocolVersion !== "2025-11-25") throw new Error(`unexpected MCP protocol version: ${result?.protocolVersion || "missing"}`);
    this.notify("notifications/initialized", {});
  }

  callTool(name, args = {}, timeoutMs = this.options.timeoutMs, authorityScope = {}) {
    const taskId = text(authorityScope.taskId) || "frontdoor_smoke";
    const callIndex = Number.isSafeInteger(authorityScope.callIndex) ? authorityScope.callIndex : 1;
    const authority = this.authorityFor(name, args, taskId, callIndex);
    return this.request("tools/call", {
      name,
      arguments: args,
      _meta: { analytixRuntimeContext: authority }
    }, timeoutMs);
  }

  stop() {
    if (!this.child) return;
    try {
      this.child.stdin.end();
    } catch {
      // ignore shutdown errors
    }
    setTimeout(() => {
      try {
        this.child.kill();
      } catch {
        // ignore shutdown errors
      }
    }, 200);
    fs.rmSync(this.authorityRoot, { recursive: true, force: true });
  }
}

async function runTask(client, task, options) {
  const startedAt = Date.now();
  try {
    const repeatCount = Math.max(1, Math.trunc(Number(task.repeatCount || 1)) || 1);
    const calls = [];
    for (let index = 0; index < repeatCount; index += 1) {
      const result = await client.callTool(task.tool, task.args, options.timeoutMs, {
        taskId: task.id,
        callIndex: index + 1
      });
      calls.push({
        call_index: index + 1,
        body: contentText(result),
        result,
      });
    }
    const body = calls.map((call) => `--- call ${call.call_index} ---\n${call.body}`).join("\n");
    const lastBody = calls[calls.length - 1]?.body || "";
    const caseSourceBlockerTask = task.id === "real_case_source_blocker";
    const supportCheck = caseSourceBlockerTask ? { ok: true, errors: [], summaries: [] } : supportEnvelopeCheck(calls, task);
    const missingMarkers = caseSourceBlockerTask ? task.markers.filter((marker) => !body.includes(marker)) : [];
    const missingLastCallMarkers = caseSourceBlockerTask
      ? (task.lastCallMarkers || []).filter((marker) => !lastBody.includes(marker))
      : [];
    const lastCallMarkersEvaluated = caseSourceBlockerTask;
    const reportLikeTask = /report|claim/u.test(task.id) || ["claim_review", "full_case"].includes(text(task.args?.intent));
    const taskForbiddenMarkers = Array.isArray(task.forbiddenMarkers) ? task.forbiddenMarkers : [];
    const forbiddenMarkers = [
      ...taskForbiddenMarkers,
      ...(caseSourceBlockerTask ? [] : ["answer_draft"]),
      ...(options.forbidDocLeakage ? DOC_LEAKAGE_MARKERS : []),
      ...(options.forbidReportGateLeakage && !reportLikeTask ? REPORT_GATE_LEAKAGE_MARKERS : []),
    ];
    const presentForbiddenMarkers = forbiddenMarkers.filter((marker) => body.includes(marker));
    const userLanguageLeakage = caseSourceBlockerTask && options.forbidUserLanguageLeakage ? userVisibleLeakageLabels(body) : [];
    const presentOptionalMarkers = (task.optionalMarkers || []).filter((marker) => body.includes(marker));
    const hasOpaqueRefs = OPAQUE_REF_PATTERN.test(body);
    const elapsedMs = Date.now() - startedAt;
    return {
      task_id: task.id,
      profile: task.profile,
      tool: task.tool,
      ok: missingMarkers.length === 0
        && missingLastCallMarkers.length === 0
        && presentForbiddenMarkers.length === 0
        && userLanguageLeakage.length === 0
        && supportCheck.ok
        && !hasOpaqueRefs,
      elapsed_ms: elapsedMs,
      answer_chars: body.length,
      repeat_count: repeatCount,
      marker_count: caseSourceBlockerTask ? task.markers.length : 0,
      hit_count: caseSourceBlockerTask ? task.markers.length - missingMarkers.length : 0,
      missing_markers: missingMarkers,
      last_call_markers_evaluated: lastCallMarkersEvaluated,
      last_call_marker_count: lastCallMarkersEvaluated ? (task.lastCallMarkers || []).length : 0,
      last_call_hit_count: lastCallMarkersEvaluated ? (task.lastCallMarkers || []).length - missingLastCallMarkers.length : 0,
      missing_last_call_markers: missingLastCallMarkers,
      forbidden_markers: forbiddenMarkers,
      present_forbidden_markers: presentForbiddenMarkers,
      user_language_leakage: userLanguageLeakage,
      optional_markers: task.optionalMarkers || [],
      present_optional_markers: presentOptionalMarkers,
      has_opaque_refs: hasOpaqueRefs,
      support_envelope_errors: supportCheck.errors,
      support_envelope_summaries: supportCheck.summaries || [],
      first_lines: body.split("\n").slice(0, 12),
      call_summaries: calls.map((call) => ({
        call_index: call.call_index,
        answer_chars: call.body.length,
        has_repeat_suppression_marker: call.body.includes("重复入口调用已被抑制"),
      })),
      support_material_coverage: supportMaterialCoverage(task, body, calls, elapsedMs),
    };
  } catch (error) {
    const repeatCount = Math.max(1, Math.trunc(Number(task.repeatCount || 1)) || 1);
    const errorText = error instanceof Error ? error.message : String(error);
    const p0UnadvertisedRejection = /"code":-32602/u.test(errorText)
      && /Unknown or unadvertised tool/u.test(errorText);
    return {
      task_id: task.id,
      profile: task.profile,
      tool: task.tool,
      ok: p0UnadvertisedRejection,
      elapsed_ms: Date.now() - startedAt,
      error: errorText,
      p0_unadvertised_rejection: p0UnadvertisedRejection,
      repeat_count: repeatCount,
      marker_count: 0,
      hit_count: 0,
      missing_markers: [],
      last_call_marker_count: 0,
      last_call_hit_count: 0,
      missing_last_call_markers: [],
      has_opaque_refs: false,
      first_lines: [],
    };
  }
}

async function main() {
  assertFixedUnavailableOracleCalibration();
  const options = parseArgs(process.argv.slice(2));
  const selectedTasks = tasksForOptions(options);
  const client = new McpClient(options);
  const startedAt = new Date().toISOString();
  const records = [];
  client.start();
  try {
    await client.initialize();
    for (const task of selectedTasks) {
      records.push(await runTask(client, task, options));
    }
  } finally {
    client.stop();
  }
  const supportCoverage = records.map((record) => record.support_material_coverage).filter(Boolean);
  const result = {
    phase: "frontdoor-direct-mcp-smoke",
    generated_at: new Date().toISOString(),
    started_at: startedAt,
    backend_url: options.backendUrl,
    profile: options.profile,
    note: "Direct MCP smoke verifies P0 fail-closed evidence/publication boundaries only. It has no host EvidenceReceipt registry and therefore cannot prove semantic fact or answer-card capability; that requires the Go host integration benchmark.",
    summary: {
      task_count: records.length,
      pass_count: records.filter((record) => record.ok).length,
      fail_count: records.filter((record) => !record.ok).length,
      opaque_ref_task_count: records.filter((record) => record.has_opaque_refs).length,
      support_material_coverage_count: supportCoverage.length,
      support_material_auto_fail_count: supportCoverage.filter((score) => score.auto_fail.length).length,
      support_material_average: supportCoverage.length
        ? Number((supportCoverage.reduce((sum, score) => sum + score.score, 0) / supportCoverage.length).toFixed(2))
        : 0,
    },
    records,
  };
  const outputPath = outputPathFor(options);
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`, "utf8");
  if (options.json) {
    console.log(JSON.stringify({ output: outputPath, ...result }, null, 2));
  } else {
    console.log(`wrote ${outputPath}`);
    console.log(JSON.stringify(result.summary, null, 2));
  }
  if (result.summary.fail_count > 0 || result.summary.opaque_ref_task_count > 0) {
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exitCode = 1;
});
