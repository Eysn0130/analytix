import {
  duplicateCandidateSummary,
  duplicateSeedCandidate,
  makeDiagnosticFact,
  probePatternSummary,
  unitDriftCandidate,
  diagnosticSkillWarnings
} from "./diagnostic-fact-helpers.mjs";
import { withInvestigationLabProtocol } from "./investigation-lab-protocol.mjs";
import {
  arrayOf,
  clampInt,
  intValue,
  objectOf,
  text
} from "./runtime-normalizers.mjs";

export const INVESTIGATION_LAB_DIAGNOSTIC_RUNTIME_VERSION = "0.14.3-investigation-lab-diagnostic-runtime";

function countText(value) {
  const next = intValue(value);
  return next ? next.toLocaleString("en-US") : "未知";
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

function scopePayloadFor(holderName) {
  const normalizedHolder = text(holderName);
  return {
    holder_name: normalizedHolder,
    include_candidate_accounts: Boolean(normalizedHolder)
  };
}

function subjectLabelFor({ holderName = "", focusKeywords = [], question = "" } = {}) {
  const explicitHolder = text(holderName);
  if (explicitHolder) return explicitHolder;
  const q = text(question);
  const namedFocus = uniqueTexts(
    arrayOf(focusKeywords).filter((keyword) => {
      const value = text(keyword);
      return value && /[\u4e00-\u9fa5]{2,12}/u.test(value) && (!q || q.includes(value));
    }),
    4
  );
  if (namedFocus.length) return namedFocus.join("、");
  return "当前案件主体";
}

export function createInvestigationLabDiagnosticRuntime({ safeSkill, moneyText, duplicateCandidateRiskText }) {
  async function buildInvestigationLabDiagnosticCard(caseId, args = {}, { signal } = {}) {
    const holderName = text(args.holder_name);
    const focusKeywords = arrayOf(args.focus_keywords).map(text).filter(Boolean);
    const questionText = text(args.question);
    const subjectLabel = subjectLabelFor({ holderName, focusKeywords, question: questionText });
    const defaultKeywords = ["理财", "缺失对手", "重复交易号", "金额集中", "换卡"];
    const keywords = uniqueTexts(focusKeywords.length ? [...focusKeywords, ...defaultKeywords] : defaultKeywords, 12);
    const dateStart = text(args.date_start);
    const scopedHolderPayload = scopePayloadFor(holderName);
    const labResult = await safeSkill("run_investigation_lab", {
      case_id: caseId,
      ...scopedHolderPayload,
      analysis_goal: questionText || "open-ended investigative discovery",
      focus_keywords: keywords,
      date_start: dateStart,
      max_hypotheses: clampInt(args.max_cards, 8, 1, 12),
      include_top_outflows: true,
      include_full_case_rankings: false
    }, { signal });
    const probeResult = await safeSkill("hypothesis_probe", {
      case_id: caseId,
      ...scopedHolderPayload,
      probe_type: "investigative_patterns",
      keywords,
      limit: 20
    }, { signal });
    const reportProbeResult = await safeSkill("hypothesis_probe", {
      case_id: caseId,
      probe_type: "report_claim_review",
      keywords: ["双空", "空户名", "空账号", "缺失对手"],
      limit: 20
    }, { signal });
    const missingResult = await safeSkill("classify_missing_counterparty_business", {
      case_id: caseId,
      ...scopedHolderPayload,
      missing_kind: "both",
      limit: 20
    }, { signal });
    const duplicateResult = await safeSkill("resolve_duplicate_families", {
      case_id: caseId,
      holder_name: holderName,
      date_start: dateStart,
      scope_mode: "same_holder_accounts",
      limit: 5
    }, { signal });
    const lab = objectOf(labResult.data);
    const probe = objectOf(probeResult.data);
    const reportProbe = objectOf(reportProbeResult.data);
    const missing = objectOf(missingResult.data);
    const blankCounterpartyBoth = objectOf(probePatternSummary(reportProbe).blank_counterparty_both_name_and_account_missing);
    const blankCounterpartyBothCount = intValue(blankCounterpartyBoth.txn_count);
    const blankCounterpartyBothAmount = blankCounterpartyBoth.turnover_total;
    const blankCounterpartyBothSupported = blankCounterpartyBothCount > 0 || text(blankCounterpartyBothAmount);
    const blankCounterpartyBothText = blankCounterpartyBothSupported
      ? `双空对手方统计：对手方名称和对手方账号均为空 ${countText(blankCounterpartyBothCount)} 笔，资金流量 ${moneyText(blankCounterpartyBothAmount)}；这是缺失对手方/现金断点补调入口，不是交易级可证实资金边，不得画资金流向图或写成资金闭环。`
      : "";
    const duplicateCandidate = duplicateCandidateSummary(duplicateResult);
    const duplicateRiskText = duplicateCandidateRiskText(duplicateCandidate);
    const unit = unitDriftCandidate(probe);
    const duplicateSeed = duplicateSeedCandidate(probe, unit.txn_id);
    const allText = JSON.stringify({
      lab_cards: [...arrayOf(lab.mandatory_review_cards), ...arrayOf(lab.hypothesis_cards)],
      probe_findings: arrayOf(probe.findings),
      missing,
      duplicate_candidate: duplicateCandidate
    });
    const has = (pattern) => pattern.test(allText);
    const defaultLeadWords = ["理财", "缺失对手", "重复交易号", "金额集中", "换卡"];
    const leadWords = uniqueTexts([...focusKeywords, ...defaultLeadWords], 10);
    const calls = [labResult, probeResult, reportProbeResult, missingResult, duplicateResult];
    const suspiciousClusters = leadWords.filter((lead) => allText.includes(lead)).map((lead) => `${lead}：production lab/probe 已返回相关线索。`);
    const temporalClusters = arrayOf(probePatternSummary(probe).temporal_clusters).slice(0, 5).map((item) => text(item.summary || item.date || JSON.stringify(item))).filter(Boolean);
    const continuationPaths = arrayOf(lab.continuation_paths || probePatternSummary(probe).continuation_paths).slice(0, 5);
    const duplicateSeedRequirement = duplicateSeed.txn_id
      ? `关键重复交易号 ${duplicateSeed.txn_id} 可作为复核种子。`
      : "重复交易号未稳定返回时，不得补写固定样例交易号。";
    const unitRequirement = unit.amount
      ? `单位纠偏 ${moneyText(unit.amount)} = ${unit.amount_wanyuan} 万元。`
      : "单位纠偏未稳定返回时，不得补写固定样例金额。";
    return withInvestigationLabProtocol({
      card_type: "investigation_lab_card",
      case_id: caseId,
      intent: "old_thread_patterns",
      title: "开放式深挖核验材料",
      fact_source: "production_backend",
      investigation_intent: `开放式深挖：围绕${subjectLabel}、围绕主体，重点复核 ${keywords.join("、")} 等侦查假设；${questionText || `${subjectLabel} 开放式深挖`}`,
      verified_facts: [
        blankCounterpartyBothText,
        unit.amount ? `命中单位纠偏候选：${moneyText(unit.amount)}，折合 ${unit.amount_wanyuan} 万元。` : "",
        duplicateSeed.txn_id ? `命中重复交易号种子：${duplicateSeed.txn_id}。` : ""
      ].filter(Boolean),
      confirmed_facts: [
        blankCounterpartyBothText,
        unit.amount ? `命中单位纠偏候选：${moneyText(unit.amount)}，折合 ${unit.amount_wanyuan} 万元。` : "",
        duplicateSeed.txn_id ? `命中重复交易号种子：${duplicateSeed.txn_id}。` : ""
      ].filter(Boolean),
      hypothesis_queue: [
        blankCounterpartyBothSupported
          ? `双空对手方/现金断点：${countText(blankCounterpartyBothCount)} 笔、${moneyText(blankCounterpartyBothAmount)}，进入待检假设；未取得交易级可证实资金边前只能作为补调入口。`
          : "",
        ...leadWords.map((lead) => `${lead}：进入待检假设，继续回到底层交易和资金去向核验。`)
      ].filter(Boolean),
      hypothesis_status: leadWords.map((lead) => ({
        hypothesis: lead,
        status: allText.includes(lead) ? "high_confidence_lead" : "needs_query",
        support_status: allText.includes(lead) ? "lead" : "missing"
      })),
      suspicious_groups: suspiciousClusters.length ? suspiciousClusters : ["可疑线索集合未稳定返回：只能作为待复核假设，不得写成已查明事实。"],
      amount_concentrations: unit.amount ? [`${moneyText(unit.amount)} / ${unit.amount_wanyuan} 万元 / ${Number(unit.amount).toFixed(2).replace(/,/gu, "")}，需防止元/万元单位误读。`] : ["金额集中未稳定返回：只能列下一步查询，不能补写成事实金额。"],
      time_concentrations: temporalClusters.length
        ? temporalClusters
        : [dateStart
            ? `时间集中未稳定返回：围绕 ${dateStart} 后和已命中事实种子继续复核。`
            : "时间集中未稳定返回：按用户问题和全案时间范围继续复核，不能编造核心日期。"],
      duplicate_or_same_fact_risks: [
        duplicateRiskText,
        (has(/重复|同事实|换卡|补卡/u) || duplicateSeed.txn_id) ? "production lab/probe 返回重复、同事实、换卡或补卡风险线索。" : ""
      ].filter(Boolean),
      related_account_or_card_switch_risks: has(/换卡|补卡|关联账户/u) ? ["发现换卡/补卡/关联账户风险，不能直接做全流水去重。"] : ["关联账户/换卡风险未稳定返回：不得把候选账户写成名下账户，需账户归集和回单复核。"],
      card_switch_or_related_account_risks: has(/换卡|补卡|关联账户/u) ? ["发现换卡/补卡/关联账户风险，不能直接做全流水去重。"] : ["换卡/补卡风险未稳定返回：只能作为待复核方向。"],
      missing_counterparty_or_cash_breaks: [
        blankCounterpartyBothText,
        has(/缺失对手|missing|现金|断点/u) ? "发现缺失对手方、现金或断点线索；列为补调方向。" : "缺失对手方/现金断点未稳定返回：不得补写最终流向。"
      ].filter(Boolean),
      wealth_management_or_asset_clues: has(/理财|认申购|赎回/u) ? ["发现理财、认申购、赎回或资产转换线索；不能误写成现金去向。"] : ["理财/资产线索未稳定返回：不得把转存、贷款、还款或理财误写成现金去向。"],
      continuation_paths: continuationPaths.length ? continuationPaths : ["追查路径待补：先核验假设，再补确定性资金边，最后复核后续承接清单。"],
      excluded_or_downgraded_hypotheses: [
        "没有交易级可证实资金边，不得画成确定性资金流向图箭头。",
        "没有主体关系证据的关联人线索，只能写为需复核线索。"
      ],
      next_queries: [
        { tool: "hypothesis_probe", why_this_query: "继续验证每个开放式假设是否有底层交易或规则命中支撑。" },
        { tool: "build_fund_flow_graph", why_this_query: "只有锁定交易边后，才允许生成资金流向图。" },
        { tool: "validate_continuation_list", why_this_query: "只有形成具体后续承接清单时再验证，不用于本普通深挖题继续绕路。" }
      ],
      next_investigation_queue: [
        "next investigation queue: 将假设队列按 supported fact、lead、gap、forbidden-as-fact 分层，下一轮只追新的对象、口径、种子交易或待验证假设。"
      ],
      forbidden_as_facts: [
        "把重复放大、同事实候选或未复核金额范围写成事实金额。",
        "把理财/转存/贷款/还款误写成现金去向。",
        "把证据不足的资金链路画成确定性资金流向图。",
        "把涉黑、违法所得、代持、实际控制写成已查明。"
      ],
      stop_conditions: [
        "到达现金、空户名、缺失对手方、缺回单、现有流水无法证明时，停止确定性表述并列补证方向。",
        "支持事实已覆盖事实、边界和事实化风险项时，停止重复取数，由 investigation-lab owner 形成研判。"
      ],
      facts: [
        makeDiagnosticFact({
          factId: "blank_counterparty.both_blank.investigation_lab",
          label: "双空对手方统计",
          count: blankCounterpartyBoth.txn_count,
          amount: blankCounterpartyBoth.turnover_total,
          unit: "yuan",
          supportStatus: blankCounterpartyBothSupported ? "supported" : "missing",
          answerText: blankCounterpartyBothSupported
            ? blankCounterpartyBothText
            : "hypothesis_probe 未稳定返回双空对手方统计，不能补写金额或笔数。",
          supportQueryName: "hypothesis_probe.report_claim_review.blank_counterparty_both_name_and_account_missing",
          sourcePayload: { reportProbeResult, blankCounterpartyBoth }
        }),
        makeDiagnosticFact({
          factId: "unit_drift.candidate",
          label: "单位纠偏",
          amount: unit.amount,
          unit: "yuan",
          supportStatus: unit.amount ? "supported" : "missing",
          answerText: unit.amount
            ? `单位纠偏值 ${moneyText(unit.amount)}，折合 ${unit.amount_wanyuan} 万元。`
            : "production lab/probe 未稳定返回单位纠偏候选，是 facts 缺口。",
          supportQueryName: "hypothesis_probe.old_thread.unit_drift_candidates",
          sourcePayload: { probeResult, unit }
        }),
        makeDiagnosticFact({
          factId: "old_thread.transaction_seed",
          label: "重复交易号种子",
          value: duplicateSeed.txn_id,
          supportStatus: duplicateSeed.txn_id ? "supported" : "missing",
          answerText: duplicateSeed.txn_id
            ? `关键交易号 ${duplicateSeed.txn_id} 应作为重复交易号和金额集中复核种子。`
            : "production lab/probe 未稳定返回关键重复交易号，是 facts 缺口。",
          supportQueryName: "hypothesis_probe.old_thread.duplicate_txn_id_groups",
          sourcePayload: { probeResult, duplicateSeed }
        }),
        makeDiagnosticFact({
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
        ...leadWords.slice(0, 5).map((lead, index) => makeDiagnosticFact({
          factId: `old_thread.lead.${index + 1}`,
          label: "开放式深挖线索",
          value: lead,
          supportStatus: allText.includes(lead) ? "lead" : "gap_disclosed",
          answerText: allText.includes(lead)
            ? `${lead} 已在 production lab/probe 中出现，仍需继续回到底层交易或工具复核。`
            : `${lead} 未在 production lab/probe 中稳定出现，是 facts 缺口，事实层只能写成待复核假设。`,
          supportQueryName: "run_investigation_lab+hypothesis_probe",
          sourcePayload: { labResult, probeResult, missingResult, lead }
        }))
      ],
      warnings: [
        "这些是侦查线索和可疑特征，不是法律性质最终认定。",
        ...diagnosticSkillWarnings(calls)
      ],
      unsupported_claims: [
        "不得把插件输出写成说明书；应给实质研判结果和下一步复核路径。",
        `缺失对手方、理财、${subjectLabel} 等只能先写为可疑特征或补调方向；没有交易级可证实资金边不得写成确定性闭环。`,
        `没有交易级可证实资金边时，${subjectLabel}、理财、缺失对手等不能写成确定性资金闭环。`,
        "自由假设不得直接进入事实结论，应标注为已证实、高可信线索、未证实、已排除、需补证或禁止写成事实。"
      ],
      answer_constraints: [
        "输出自然语言研判结果，不输出状态机字段名。",
        blankCounterpartyBothSupported
          ? `双空对手方统计已返回：${countText(blankCounterpartyBothCount)} 笔、${moneyText(blankCounterpartyBothAmount)}，并说明交易级可证实资金边边界。`
          : "",
        "不得输出确定性资金流向图边。",
        `说明开放式深挖围绕 ${subjectLabel} 的重点复核方向。`,
        "每个可疑线索集合都要标明支持程度；没有证据边的链路进入证据不足事项。",
        unitRequirement,
        duplicateSeedRequirement,
        "单位纠偏只列正确换算值，不列举错误万元值示例；防止读者把反例数字当事实传播。",
        "输出侦查意图、已证实事实、假设队列、可疑线索集合、证据缺口、事实层状态和下一步追查队列。",
        "理财线索用认申购、赎回、理财等当前返回词说明资产转换复核方向，并明确不能写成现金去向或已证实事实。",
        "有证据写清楚，没证据不硬写，可疑点给追查队列，错误口径全部拦截。",
        "普通问答拿到本卡后直接形成研判；同一问题不要重复调用 navigator 扩圈，未证实路径只写补证方向。",
        "本题不是正式报告正文复核；本卡只抑制同一问题的重复复核，不阻断用户继续追一层、改口径、指定新对象或提出新假设。"
      ]
    });
  }

  return { buildInvestigationLabDiagnosticCard };
}
