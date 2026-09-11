export const INTENT_PLAN_PROTOCOL_VERSION = "0.14.3-intent-plan-protocol";

export const FRONTDOOR_INTENTS = new Set([
  "casegraph",
  "claim_review",
  "company_to_person_recompute",
  "financial_product_topic",
  "flow_graph",
  "name_association_quick_fact",
  "destination",
  "full_case",
  "old_thread_patterns",
  "ranking",
  "risk_discovery",
  "qa_review",
  "holder_analysis",
  "account_analysis"
]);

const REPORT_INTENTS = new Set(["claim_review", "full_case"]);
const SUPPORTED_EDGE_GUARD_LITERAL = 'edge_status="supported"';
const INTENT_PLAN_GUARD_MARKERS = [
  "报告级任务才追加 validate_report_claims",
  "同一问题已完成时只抑制重复入口调用"
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
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

export function inferFrontDoorIntent(args = {}) {
  const source = objectOf(args);
  const explicit = text(source.intent);
  const signal = [
    source.question,
    source.analysis_goal,
    source.holder_name,
    source.via_holder_name,
    source.counterparty_name,
    ...arrayOf(source.focus_keywords)
  ].map(text).filter(Boolean).join(" ");
  const reportClaimSignal = /claim|既有报告|(?:既有|原|已有|此前|上次|旧).{0,4}报告|报告.*(?:复核|校验|纠正|降级|门禁|阻断)|报告级.*(?:复核|门禁|阻断)|(?:来源|证据|金额|笔数).{0,12}(?:纠正|复核|校验)|无证据|unsupported|降级/u;
  const broadExplicit = new Set(["auto", "risk_discovery", "full_case"]);
  const counterpartyTransferSignal =
    /[\u4e00-\u9fa5]{2,4}.{0,8}(?:转给|汇给|打给|给(?!出|我|出具)).{0,8}[\u4e00-\u9fa5]{2,4}/u;
  const formalReportSignal = /(?:生成|出具|形成|写|撰写|编制|输出|更新).{0,10}(?:报告|研判报告|正式报告)|报告(?:正文|草稿|门禁|复核|校验|claim)/u;
  const fullCaseDraftSignal = /(?:生成|出具|形成|写|撰写|编制|输出).{0,16}(?:全案|整体|当前案件|资金研判|研判报告|报告草稿|报告正文|报告材料|材料)|(?:全案|整体|当前案件|资金研判).{0,16}(?:报告|草稿|材料)/u;
  const openTraceInvestigationSignal =
    /(?:全面)?资金(?:梳理|追踪|穿透|链路|路径|关联通道|通道)|共同交易对手|中转|回流|拆分|关联人|关联公司|围绕.{0,24}(?:关联|通道|追踪|穿透|链路)/u;
  const q = signal;
  if (explicit === "full_case") return "full_case";
  if (/对公转个人|公转私|单位转个人|公司转个人|摘要缺失纳入|薪资|工资|劳务|报销/u.test(q)) {
    return "company_to_person_recompute";
  }
  if (/(?:基金|证券|股票|三方存管|申购|赎回)/u.test(q) && /(?:购买|买|情况|专题|核算|统计|明细)/u.test(q)) {
    return "financial_product_topic";
  }
  if (/(?:是否|有无|有没有).{0,18}(?:案件资金|资金|流水|交易).{0,12}(?:关联|有关|关系)|(?:关联快查|姓名.{0,4}快查|主体关联快查)/u.test(q)) {
    return "name_association_quick_fact";
  }
  if (/(?:资金流向图|资金穿透图|来源去向图|去向图|流向图|关系图|导出.{0,8}图|画.{0,8}图)/u.test(q)) {
    return "flow_graph";
  }
  if (
    counterpartyTransferSignal.test(signal) &&
    (!explicit || explicit === "auto" || explicit === "holder_analysis" || explicit === "account_analysis" || explicit === "risk_discovery")
  ) {
    return "destination";
  }
  if (
    openTraceInvestigationSignal.test(signal) &&
    !formalReportSignal.test(signal) &&
    (!explicit || explicit === "auto" || explicit === "full_case" || explicit === "risk_discovery")
  ) {
    return "old_thread_patterns";
  }
  if (fullCaseDraftSignal.test(signal) && !reportClaimSignal.test(signal)) {
    return "full_case";
  }
  if (explicit && !broadExplicit.has(explicit) && FRONTDOOR_INTENTS.has(explicit)) {
    return explicit;
  }
  if (reportClaimSignal.test(signal)) {
    return "claim_review";
  }
  if (/全案|报告|研判报告/u.test(q) || formalReportSignal.test(signal)) {
    return "full_case";
  }
  if (arrayOf(source.account_keys).length) return "account_analysis";
  if (counterpartyTransferSignal.test(signal)) {
    return "destination";
  }
  if (/转给.{0,12}后|收到.{0,12}后|[\u4e00-\u9fa5]{2,4}.*(?:又|后续|下游|去向|流向|穿透|转给谁)|(?:又|后续).*(?:转给谁|给谁|下游|去向|流向)/u.test(signal)) {
    return "destination";
  }
  if (/旧线程|开放式深挖|自由深挖|深挖复核|单位纠偏|金额集中|重复交易号|理财线索|围绕.{0,24}(?:深挖|复核|穿透|关联)|[\u4e00-\u9fa5]{2,4}.*(?:深挖|理财线索)/u.test(signal)) {
    return "old_thread_patterns";
  }
  if (reportClaimSignal.test(signal)) {
    return "claim_review";
  }
  if (explicit && explicit !== "auto" && FRONTDOOR_INTENTS.has(explicit)) {
    return explicit;
  }
  if (/画|图|流向图|mermaid|箭头/u.test(q)) return "flow_graph";
  if (text(source.via_holder_name || source.counterparty_name) && /转|给|金额|合计|多少钱|出账|入账|补充数字|核验/u.test(q)) return "destination";
  if (/clean_duplicate|同事实|重复|换卡|补卡|空户名|户名空|未登记户名|字段缺失|清洗|去重|误读/u.test(q)) return "qa_review";
  if (/(?:Top|top|前\s*\d+).{0,12}出账/u.test(q) && /终点|补调|分类|去向|流向/u.test(q)) return "destination";
  if (/(?:名下账户|名下|户名|主体资金画像|主体资金研判|对[\u4e00-\u9fa5]{2,4}.{0,8}(?:账户|资金).{0,8}(?:研判|分析|画像|核查)|[\u4e00-\u9fa5]{2,4}名下账户)/u.test(q)) {
    return "holder_analysis";
  }
  if (arrayOf(source.account_keys).length || /卡号|账号|账户/u.test(q)) return "account_analysis";
  if (/转给谁|给了谁|去向|下游|后续|流向|穿透/u.test(q)) return "destination";
  if (/全案|报告|研判报告/u.test(q)) return "full_case";
  if (/排名|排行|top|Top|最大|最多|最高|第一|前\s*\d+/u.test(q)) return "ranking";
  if (text(source.holder_name) || /名下|户名|某人/u.test(q)) return "holder_analysis";
  if (arrayOf(source.account_keys).length || /卡号|账号|账户/u.test(q)) return "account_analysis";
  if (/可疑|异常|深挖|自由|不规律|重复|换卡|补卡/u.test(q)) return "risk_discovery";
  return "casegraph";
}

export function buildIntentAst(input = {}) {
  const source = objectOf(input);
  const intent = text(source.intent);
  const requestedIntent = text(source.requestedIntent || intent);
  const reportGradeIntent = REPORT_INTENTS.has(intent) || source.reportGradeIntent === true;
  const wantsFlowGraph = source.wantsFlowGraph === true || requestedIntent === "flow_graph" || intent === "destination";
  const openInvestigationIntent = ["old_thread_patterns", "destination", "flow_graph", "risk_discovery"].includes(intent);
  const maxAdditionalTools = reportGradeIntent ? 1 : openInvestigationIntent ? 1 : 0;
  return pruneEmpty({
    version: INTENT_PLAN_PROTOCOL_VERSION,
    requested_intent: requestedIntent,
    normalized_intent: intent,
    lane: reportGradeIntent ? "report_gate" : intent === "destination" ? "trace_or_destination" : intent,
    subject: {
      holder_name: text(source.holderName),
      via_holder_name: text(source.viaName),
      has_account_keys: arrayOf(source.accountKeys).length > 0
    },
    window: {
      date_start: text(source.dateStart),
      date_end: text(source.dateEnd)
    },
    flags: {
      wants_flow_graph: wantsFlowGraph,
      wants_via_continuation: source.wantsViaContinuation === true,
      report_grade_intent: reportGradeIntent,
      passive_allowed_without_plugin: source.pluginExpected === false
    },
    tool_budget: {
      ordinary_max_tool_calls: 1,
      report_max_tool_calls: 2,
      max_additional_tools: maxAdditionalTools,
      budget_policy: "soft_same_question_budget; do not repeat the same metric/tool loop, but do not block new hypotheses, rescope, continuation, or evidence conflicts",
      claim_review_allowed: reportGradeIntent,
      repeat_discovery_allowed: openInvestigationIntent
    }
  }) || {};
}

export function buildPlanDag(intentAst = {}) {
  const ast = objectOf(intentAst);
  const intent = text(ast.normalized_intent);
  const reportGrade = objectOf(ast.flags).report_grade_intent === true;
  const wantsFlowGraph = objectOf(ast.flags).wants_flow_graph === true;
  const nodes = [
    {
      id: "frontdoor",
      tool: "funds_investigate",
      role: "single_default_entry",
      status: "required"
    }
  ];
  if (wantsFlowGraph) {
    nodes.push({
      id: "supported_edge_gate",
      tool: "build_fund_flow_graph",
      role: "conditional_internal_or_explicit_graph_edge_builder",
      status: "conditional",
      guard: "只有交易级可证实资金边才能成为资金流向图箭头。"
    });
  }
  if (reportGrade) {
    nodes.push({
      id: "claim_review_gate",
      tool: "validate_report_claims",
      role: "report_level_hard_gate",
      status: "allowed_once_after_frontdoor"
    });
  }
  nodes.push({
    id: "answer_card",
    role: "compact_model_context",
    status: "terminal",
    guard: reportGrade
      ? "报告事实结论复核和来源边界未通过前，不出具正式报告。"
      : ["old_thread_patterns", "destination", "flow_graph", "risk_discovery"].includes(intent)
        ? "本轮研判卡已经回答当前问题；用户继续追一层或改口径时，可进入新的核验流程。"
        : "同一普通事实问题已完成时，只抑制重复入口调用。"
  });
  return {
    version: INTENT_PLAN_PROTOCOL_VERSION,
    intent,
    nodes,
    edges: nodes.slice(1).map((node) => ({ from: "frontdoor", to: node.id })),
    stop_conditions: [
      reportGrade
        ? "报告级任务在正式文本前先完成结论引用核验；未通过时用自然语言说明暂不能出具正式报告和需补证事项。"
        : ["old_thread_patterns", "destination", "flow_graph", "risk_discovery"].includes(intent)
          ? "开放深挖/去向/穿透任务先基于本轮核验材料作答；用户继续追一层、改口径或指定新对象时可以选择对应语义事实工具。"
          : "普通事实题选择最小充分语义事实工具后直接作答，不追加报告事实结论复核。",
      "无交易级可证实资金边时不得画资金流向图或写确定性资金链。",
      "候选账户、缺失对手方、现金断点和缺失回单只能写线索/需复核。"
    ]
  };
}

export function frontdoorWantsFlowGraph({ intent = "", requestedGraph = false, questionText = "" } = {}) {
  if (requestedGraph === true || text(intent) === "flow_graph") return true;
  if (["qa_review", "risk_discovery"].includes(text(intent))) return false;
  return /资金流向图|资金穿透图|来源去向图|去向图|流向图|关系图|(?:导出|生成|画|绘制).{0,8}图|确定性资金链|资金链路图|资金路径图/u.test(text(questionText));
}
