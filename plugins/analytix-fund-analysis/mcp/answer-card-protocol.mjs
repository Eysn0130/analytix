import { answerCardContextCompiler } from "./context-compiler.mjs";

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function uniqueTexts(values, limit = 12) {
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

function clampInt(value, fallback, min, max) {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.max(min, Math.min(max, Math.trunc(number)));
}

export function requiredFactsPresentForCard(card) {
  const facts = arrayOf(objectOf(card).facts);
  if (!facts.length) return false;
  // Only the host Final Evidence Gate can attest fact readiness. A card or
  // model-provided support label is not sufficient evidence.
  return false;
}

export function unsupportedFlowsPresentForCard(card) {
  const source = objectOf(card);
  const flowText = JSON.stringify({
    unsupported_claims: source.unsupported_claims,
    unsupported_flows: source.unsupported_flows,
    claim_review_unsupported: objectOf(source.claim_review).unsupported,
    forbidden_as_facts: source.forbidden_as_facts
  });
  return /(unsupported\s*flow|Mermaid|flowchart|资金链路|资金流向|确定性|supported edge|交易边|箭头|闭环)/iu.test(flowText);
}

export function answerCardProtocolDefaults(card) {
  const cardType = text(objectOf(card).card_type);
  if (cardType === "claim_review_card") {
    return {
      recommendedNextAction: "answer_now_then_validate_report_claims_only_if_formal_report_text_is_required",
      maxAdditionalTools: 1
    };
  }
  return {
    recommendedNextAction: "answer_now",
    maxAdditionalTools: 0
  };
}

export function withAnswerCardProtocol(card, options = {}) {
  const source = objectOf(card);
  const defaults = answerCardProtocolDefaults(source);
  const requiredFactsPresent = Object.prototype.hasOwnProperty.call(options, "requiredFactsPresent")
    ? Boolean(options.requiredFactsPresent)
    : requiredFactsPresentForCard(source);
  const unsupportedFlowsPresent = Object.prototype.hasOwnProperty.call(options, "unsupportedFlowsPresent")
    ? Boolean(options.unsupportedFlowsPresent)
    : unsupportedFlowsPresentForCard(source);
  const factAnswerAllowed = false;
  const maxAdditionalTools = clampInt(
    options.maxAdditionalTools ?? defaults.maxAdditionalTools,
    defaults.maxAdditionalTools,
    0,
    3
  );
  const recommendedNextAction = text(options.recommendedNextAction || defaults.recommendedNextAction);
  const completionStopCondition = requiredFactsPresent
    ? (maxAdditionalTools <= 0
      ? "本卡事实、边界和事实化风险项已经足够当前答复；只抑制同一问题的重复查询，不阻断继续追一层、改口径、新对象或新假设的语义工具追查。"
      : "报告级正式文本如需发布，先做一次研判结论复核；该限制只约束同一报告文本的重复复核，不阻断普通研判、继续追一层、改口径、新对象或新假设。")
    : "当前事实工具失败或必需事实缺口未补齐，只能说明缺口和重试建议，禁止把缺失字段渲染成 0 或写成已查明事实。";
  const stopConditions = [
    ...arrayOf(source.stop_conditions).map(text).filter(Boolean),
    completionStopCondition
  ];
  const contextCompiler = answerCardContextCompiler({
    ...source,
    must_write_facts: [],
    facts: [],
    verified_claims: [],
    confirmed_facts: [],
    verified_facts: [],
    context_compiler: undefined
  }, stopConditions);
  return {
    ...source,
    context_compiler: contextCompiler,
    answer_card_complete: true,
    fact_answer_allowed: factAnswerAllowed,
    recommended_next_action: recommendedNextAction,
    max_additional_tools: maxAdditionalTools,
    required_facts_present: requiredFactsPresent,
    unsupported_flows_present: unsupportedFlowsPresent,
    stop_conditions: uniqueTexts(stopConditions, 12)
  };
}
