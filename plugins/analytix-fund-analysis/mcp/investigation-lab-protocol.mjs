import { withAnswerCardProtocol } from "./answer-card-protocol.mjs";

export const INVESTIGATION_LAB_VERSION = "0.14.3-investigation-lab-protocol";

export const INVESTIGATION_LAB_REQUIRED_FIELDS = [
  "investigation_intent",
  "verified_facts",
  "hypothesis_queue",
  "hypothesis_status",
  "suspicious_groups",
  "amount_concentrations",
  "time_concentrations",
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

const DEFAULT_ANSWER_SECTIONS = [
  "侦查意图",
  "已证实事实",
  "假设队列",
  "支持程度与需复核边界",
  "下一步追查队列",
  "禁止写成事实"
];

const DEFAULT_STOP_CONDITIONS = [
  "本轮开放式深挖支持材料已经给出事实、假设、证据缺口和事实化风险项；同一问题不重复走入口，用户继续追一层或调整统计范围时可进入对应核验流程。",
  "到达现金、空户名、缺失对手方、缺回单、现有流水无法证明时，停止确定性表述并列补证方向。",
  "没有交易级可证实资金边，不得画成确定性资金流向图箭头。",
  "没有证据的开放式假设只能写为线索、需复核或下一步查询，不能写成已查明事实。"
];

const DEFAULT_FORBIDDEN_AS_FACTS = [
  "把重复放大、同事实候选或未复核金额范围写成事实金额。",
  "把候选账户、关联账户或换卡线索写成名下账户。",
  "把理财/转存/贷款/还款误写成现金去向。",
  "把证据不足的资金链路画成确定性资金流向图。",
  "把涉黑、违法所得、代持、实际控制写成已查明。"
];

const DEFAULT_EXCLUDED_OR_DOWNGRADED = [
  "没有交易级可证实资金边，不得画成确定性资金流向图箭头。",
  "没有主体关系证据的关联人线索，只能写为需复核线索。",
  "缺少回单、缺失对手方或现金断点时，只能进入补证队列。"
];

const DEFAULT_NEXT_QUERIES = [
  { tool: "hypothesis_probe", why_this_query: "继续验证每个开放式假设是否有底层交易、规则命中或证据缺口支撑。" },
  { tool: "build_fund_flow_graph", why_this_query: "只有锁定交易级可证实资金边后，才允许生成资金流向图。" },
  { tool: "validate_continuation_list", why_this_query: "只有形成具体后续承接清单时再验证，不用于普通深挖题继续绕路。" }
];

const DEFAULT_ANSWER_CONSTRAINTS = [
  "区分已证实事实、高可信线索、未证实假设、已排除/降级假设和禁止写成事实。",
  "每个可疑线索集合都要标明支持程度；没有证据边的链路进入证据不足或禁止写成事实事项。",
  "普通深挖题拿到本卡后直接形成本轮研判；用户继续追一层或改口径时可选择追踪、排行、假设或质量核验，不追加报告事实结论复核。",
  "报告结论复核只用于报告级输出门禁；普通开放式深挖不得把待复核线索升级为报告结论。",
  "单位纠偏只列正确换算值，不列举错误万元值示例，防止反例数字当事实传播。"
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

function uniqueTexts(values, limit = 24) {
  const seen = new Set();
  const output = [];
  for (const value of values.map(text).filter(Boolean)) {
    if (seen.has(value)) continue;
    seen.add(value);
    output.push(value);
    if (output.length >= limit) break;
  }
  return output;
}

function uniqueItems(values, limit = 16) {
  const seen = new Set();
  const output = [];
  for (const value of values) {
    const key = typeof value === "string" ? value : JSON.stringify(value);
    if (!key || seen.has(key)) continue;
    seen.add(key);
    output.push(value);
    if (output.length >= limit) break;
  }
  return output;
}

function mergeTextList(source, defaults, limit = 24) {
  return uniqueTexts([...arrayOf(source), ...defaults], limit);
}

function mergeItemList(source, defaults, limit = 16) {
  return uniqueItems([...arrayOf(source), ...defaults], limit);
}

export function withInvestigationLabProtocol(card, options = {}) {
  const source = objectOf(card);
  const verifiedFacts = arrayOf(source.verified_facts).map(text).filter(Boolean);
  const hypothesisQueue = arrayOf(source.hypothesis_queue).map(text).filter(Boolean);
  const merged = {
    ...source,
    card_type: "investigation_lab_card",
    intent: text(source.intent) || "old_thread_patterns",
    investigation_intent: text(source.investigation_intent) || "开放式深挖：先提假设，再回到底层事实复核。",
    answer_sections: mergeTextList(source.answer_sections, DEFAULT_ANSWER_SECTIONS, 12),
    verified_facts: verifiedFacts,
    confirmed_facts: arrayOf(source.confirmed_facts).length ? arrayOf(source.confirmed_facts).map(text).filter(Boolean) : verifiedFacts,
    hypothesis_queue: hypothesisQueue,
    hypothesis_status: mergeItemList(source.hypothesis_status, hypothesisQueue.map((hypothesis) => ({
      hypothesis,
      status: "needs_evidence",
      support_status: "lead"
    })), 24),
    suspicious_groups: mergeTextList(
      [...arrayOf(source.suspicious_groups), ...arrayOf(source.suspicious_clusters)],
      ["未稳定返回的可疑线索集合只能写为待复核假设，不得写成已查明事实。"],
      16
    ),
    amount_concentrations: mergeTextList(
      [...arrayOf(source.amount_concentrations), ...arrayOf(source.amount_clusters)],
      ["金额集中缺口只能列下一步查询，不能补写成事实金额。"],
      16
    ),
    time_concentrations: mergeTextList(
      [...arrayOf(source.time_concentrations), ...arrayOf(source.temporal_clusters)],
      ["时间集中缺口只能列复核窗口，不能编造交易时间。"],
      16
    ),
    duplicate_or_same_fact_risks: mergeTextList(source.duplicate_or_same_fact_risks, ["重复交易号、同事实、换卡/补卡风险必须在相关账户内审慎复核，禁止全案盲去重。"], 16),
    related_account_or_card_switch_risks: mergeTextList(source.related_account_or_card_switch_risks, ["关联账户/换卡风险未取得开户、回单或归集证据前，不得写成名下账户。"], 16),
    missing_counterparty_or_cash_breaks: mergeTextList(source.missing_counterparty_or_cash_breaks, ["缺失对手方、现金断点或缺回单只能写为补证方向，不能写成最终流向。"], 16),
    wealth_management_or_asset_clues: mergeTextList(source.wealth_management_or_asset_clues, ["理财、认申购、赎回或资产转换线索不能误写成现金去向。"], 16),
    continuation_paths: mergeItemList(source.continuation_paths, ["追查路径待补：hypothesis_probe -> trace_funds/build_fund_flow_graph -> validate_continuation_list。"], 12),
    excluded_or_downgraded_hypotheses: mergeTextList(source.excluded_or_downgraded_hypotheses, DEFAULT_EXCLUDED_OR_DOWNGRADED, 16),
    next_queries: mergeItemList(source.next_queries, DEFAULT_NEXT_QUERIES, 12),
    forbidden_as_facts: mergeTextList(source.forbidden_as_facts, DEFAULT_FORBIDDEN_AS_FACTS, 18),
    stop_conditions: mergeTextList(source.stop_conditions, DEFAULT_STOP_CONDITIONS, 18),
    unsupported_claims: mergeTextList(source.unsupported_claims, [
      "自由假设不得直接进入事实结论，必须标注为已证实、高可信线索、未证实、已排除、需补证或禁止写成事实。",
      "缺失对手方、理财、关联账户、换卡、现金断点等只能先写为可疑特征或补调方向。"
    ], 18),
    answer_constraints: mergeTextList(source.answer_constraints, DEFAULT_ANSWER_CONSTRAINTS, 18)
  };
  return withAnswerCardProtocol(merged, {
    recommendedNextAction: text(options.recommendedNextAction) || "answer_now",
    maxAdditionalTools: Object.prototype.hasOwnProperty.call(options, "maxAdditionalTools")
      ? Number(options.maxAdditionalTools)
      : 0,
    requiredFactsPresent: Object.prototype.hasOwnProperty.call(options, "requiredFactsPresent")
      ? Boolean(options.requiredFactsPresent)
      : arrayOf(merged.facts).some((fact) => text(objectOf(fact).support_status) === "supported"),
    unsupportedFlowsPresent: Object.prototype.hasOwnProperty.call(options, "unsupportedFlowsPresent")
      ? Boolean(options.unsupportedFlowsPresent)
      : true
  });
}
