export const FRONTDOOR_ANSWER_CONTRACT_VERSION = "0.14.3-frontdoor-answer-contract";
export const FRONTDOOR_ANSWER_GUARD_MARKERS = [
  "已有资金流向图事实优先复用",
  "不为补齐链路追加报告级核验",
  "结论引用核验",
  "暂不能出具正式报告时转成补证动作",
  "同一问题仅抑制重复入口调用"
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

export function buildFrontdoorNextActions({
  inlineFlowGraph = null,
  viaName = "",
  wantsFlowGraph = false,
  reportGradeIntent = false,
  caseId = ""
} = {}) {
  const nextActions = [];
  if (objectOf(inlineFlowGraph).flow_graph) {
    nextActions.push({
      reason: `已内联返回${viaName ? `“${viaName}”` : "续查对象"}相关资金流向图谱事实；本轮可直接输出主要去向、下一跳和核验意见。若用户继续追一层、改口径或提出新假设，可再选择对应语义事实工具。`
    });
  } else if (wantsFlowGraph) {
    nextActions.push({
      reason: "本次未识别到明确续查对象，先列出主要去向、重点出账起点和“需补调/不能确认”的边界；若用户指定续查对象或种子交易，再进入资金来源去向核验。"
    });
  }
  if (reportGradeIntent) {
    nextActions.push({
      reason: "只有正式材料发布前才补做结论引用核验，重点避免把待核线索、名下事实和资金链路混写。"
    });
  }
  if (!nextActions.length) {
    nextActions.push({
      reason: "本轮已有可回答的事实边界，应先回答结论、统计范围和风险。新目标、新统计范围或新假设可另行核验。"
    });
  }
  return nextActions;
}

export function buildFrontdoorAnswerContract() {
  return {
    ordinary_holder_or_ranking_answer: "若入口已返回主体范围、账户统计、重点账户或风险复核卡，可先给事实结论和边界；普通问答不要因质量提示反复进入报告事实结论复核。若用户要求完整清单、更多排名、改口径或继续追查，可进入对应排行、画像、资金去向或假设核验。",
    data_quality_boundary: "clean_duplicate=0 只能说明清洗标记未命中，不等于没有同事实/换卡/补卡重复风险；重复候选不得直接扣减总额。",
    account_boundary: "线索候选账户只能写为线索；未登记户名账户不能归入某人名下，除非现有资料已确认。",
    pair_amount_answer: "明确 A->B 金额题由特定双方资金往来核验专项成稿；入口卡片、对手方排行、重复复核和专项资金核算只提供支撑证据。成稿要分列原明细、重复风险复核后的高可信金额、可选本金/手续费、重复或证据不足、相关账户（列明账号）、时间集中、重复组、差异原因、异常特征、案件意义、暂不能认定事项和下一步核验。对手方排行核验只能写为排行线索/线索候选，不得作为最终金额结论。",
    destination_answer: "资金去向题用重点对手方、重点出账、资金链路预览和资金流向图事实作答；同步说明同事实、空户名/字段缺失、换卡风险和不能全流水去重边界。用户继续追一层或改统计范围时，再进入来源去向、资金流向图或对手方核验。",
    report_gate_answer: "报告级任务先说明结论引用核验结果。本入口只给复核版事实摘要；来源审计、数据质量、必需复核卡和报告事实结论复核未全部通过前，只能输出待复核草稿边界。暂不能出具正式报告时转成用户可见的补证动作；只纠正本次实际返回的错误口径或未获支持的研判结论，不套用固定案件金额。",
    graph_boundary: "已有数据支持的资金流向图事实返回后沿用该图谱事实；同一问题已完成时只抑制重复入口调用，不影响继续追一层、改统计范围或指定新对象。"
  };
}

export function withDuplicateSuppressionContract(answerContract = {}) {
  return {
    ...objectOf(answerContract),
    duplicate_suppression: "同一案件、同一任务和同一问题已完成时，沿用首次完整支持事实；只抑制重复入口调用，不影响新续查、新统计范围或新线索核验。"
  };
}
