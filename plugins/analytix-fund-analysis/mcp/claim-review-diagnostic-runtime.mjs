import { withClaimReviewProtocol } from "./claim-verifier-protocol.mjs";
import {
  diagnosticCallSummaries,
  diagnosticSkillWarnings,
  makeDiagnosticFact
} from "./diagnostic-fact-helpers.mjs";
import {
  arrayOf,
  objectOf,
  text,
  uniqueTexts
} from "./runtime-normalizers.mjs";
import { translateUserVisibleText } from "./user-facing-language.mjs";

export const CLAIM_REVIEW_DIAGNOSTIC_RUNTIME_VERSION = "0.14.5-claim-review-diagnostic-runtime";

const CLAIM_REVIEW_STATUS_CLASSIFICATION = new Map([
  ["supported", "supported"],
  ["verified", "supported"],
  ["matched", "supported"],
  ["corrected", "corrected"],
  ["correction", "corrected"],
  ["correct", "corrected"],
  ["mismatch", "corrected"],
  ["mismatched", "corrected"],
  ["adjusted", "corrected"],
  ["adjust", "corrected"],
  ["revised", "corrected"],
  ["revise", "corrected"],
  ["纠正", "corrected"],
  ["已纠正", "corrected"],
  ["需纠正", "corrected"],
  ["unsupported", "unsupported"],
  ["not_supported", "unsupported"],
  ["unmatched", "unsupported"],
  ["missing", "unsupported"],
  ["failed", "unsupported"],
  ["failure", "unsupported"],
  ["insufficient", "unsupported"],
  ["unverified", "unsupported"],
  ["unresolved", "unsupported"],
  ["partial", "unsupported"],
  ["needs_evidence", "unsupported"],
  ["need_evidence", "unsupported"],
  ["needs_review", "unsupported"],
  ["need_review", "unsupported"],
  ["refuted", "unsupported"],
  ["rejected", "unsupported"],
  ["forbidden", "unsupported"],
  ["boundary", "unsupported"],
  ["downgraded", "unsupported"],
  ["lead", "unsupported"],
  ["candidate", "unsupported"],
  ["unknown", "unsupported"],
  ["未支持", "unsupported"],
  ["缺失", "unsupported"]
]);

function normalizedClaimReviewStatus(row) {
  const source = objectOf(row);
  return text(source.status || source.support_status || source.finding)
    .trim()
    .toLowerCase()
    .replace(/[\s-]+/gu, "_");
}

function claimReviewStatusClass(row) {
  return CLAIM_REVIEW_STATUS_CLASSIFICATION.get(normalizedClaimReviewStatus(row)) || "unsupported";
}

export function classifyClaimReviewRows(rows = []) {
  return arrayOf(rows).reduce((result, row) => {
    result[claimReviewStatusClass(row)].push(row);
    return result;
  }, { supported: [], corrected: [], unsupported: [] });
}

function claimTextsFromArgs(args = {}) {
  const source = objectOf(args);
  const explicit = [
    ...arrayOf(source.claim_texts),
    ...arrayOf(source.claims).map((item) => typeof item === "string" ? item : text(objectOf(item).text))
  ].map(text).filter(Boolean);
  if (explicit.length) return uniqueTexts(explicit, 12);

  const raw = text(source.report_text || source.question);
  if (!raw) return [];
  return uniqueTexts(raw
    .split(/[\n；;。]/u)
    .map((line) => line.replace(/^(?:请|帮我|复核|校验|纠正|报告|claim)[:：\s]*/u, "").trim())
    .filter((line) => line.length >= 8)
    .filter((line) => /(?:元|万|笔|转|流向|去向|入账|出账|资金|账户|户名|对手|报告|claim)/u.test(line)), 12);
}

function validationRows(validationResult) {
  const data = objectOf(validationResult?.data || validationResult?.envelope?.data);
  return [
    ...arrayOf(data.claims),
    ...arrayOf(data.results),
    ...arrayOf(data.validated_claims)
  ];
}

export function createClaimReviewDiagnosticRuntime({ safeSkill, moneyText }) {
  async function buildClaimReviewDiagnosticCard(caseId, args = {}, { signal } = {}) {
    const sourceArgs = objectOf(args);
    const coverageResult = await safeSkill("get_scope_coverage", { case_id: caseId }, { signal });
    const claimTexts = claimTextsFromArgs(sourceArgs);
    const validationResult = claimTexts.length
      ? await safeSkill("validate_report_claims", {
          case_id: caseId,
          claims: claimTexts.map((claim, index) => ({ id: `user_claim_${index + 1}`, text: claim })),
          facts: {},
          strict_report_text: true
        }, { signal })
      : null;
    const coverage = objectOf(coverageResult.data.coverage || coverageResult.data);
    const validation = objectOf(validationResult?.data || validationResult?.envelope?.data);
    const rows = validationRows(validationResult);
    const calls = [coverageResult, validationResult].filter(Boolean);
    const classifiedRows = classifyClaimReviewRows(rows);
    const supportedRows = classifiedRows.supported;
    const correctedRows = classifiedRows.corrected;
    const unsupportedRows = classifiedRows.unsupported;

    return withClaimReviewProtocol({
      card_type: "claim_review_card",
      case_id: caseId,
      intent: "claim_review",
      title: "既有报告研判结论复核材料",
      fact_source: "production_backend",
      write_blocked: true,
      report_gate_status: claimTexts.length
        ? "claim_review_required_until_source_boundaries_pass"
        : "claim_review_waiting_for_user_claims",
      answer_sections: ["已复核事实", "需纠正", "未支持/不能确认", "来源边界", "需复核边界", "禁用表述", "下一步复核"],
      stop_conditions: [
        "报告级任务在研判结论、来源审计、数据质量和来源边界逐项通过前，只能输出待复核草稿边界。",
        "未支持/不能确认的链路只能写为线索、需核实或需补调；没有确定性资金边时不得画确定性资金流向图。",
        "普通研判题不要因报告级边界转入报告复核；只有报告或研判结论复核请求使用本卡。"
      ],
      claim_review: {
        supported: supportedRows.map((row) => text(objectOf(row).text || objectOf(row).claim || objectOf(row).summary)).filter(Boolean),
        corrected: correctedRows.map((row) => text(objectOf(row).correction || objectOf(row).finding || objectOf(row).text)).filter(Boolean),
        unsupported: unsupportedRows.map((row) => text(objectOf(row).text || objectOf(row).claim || objectOf(row).finding)).filter(Boolean),
        downgraded: [
          "证据不足的法律评价或最终归属判断只能写线索/需复核，不能写已查明或确定。"
        ]
      },
      verified_claims: supportedRows.map((row) => text(objectOf(row).text || objectOf(row).claim || objectOf(row).summary)).filter(Boolean),
      corrected_claims: correctedRows.map((row) => text(objectOf(row).correction || objectOf(row).finding || objectOf(row).text)).filter(Boolean),
      unsupported_flows: [
        ...unsupportedRows.map((row) => text(objectOf(row).text || objectOf(row).claim || objectOf(row).finding)).filter(Boolean),
        !claimTexts.length ? "未提供待复核报告研判结论，不能生成正式报告级结论。" : ""
      ].filter(Boolean),
      missing_source_boundaries: [
        "报告中的聚合片段必须回到确定性工具事实，不能模型侧补成资金闭环。",
        "来源审计、数据质量、必需核验材料或报告事实结论复核任一未逐项通过时，只能输出待复核草稿边界。",
        !claimTexts.length ? "请提供报告文本、待复核研判结论或明确待复核金额/链路后再做逐项校验。" : ""
      ].filter(Boolean),
      forbidden_phrasings: [
        "未触发报告阻断",
        "门禁通过",
        "可直接出具正式报告",
        "已查明涉案资金最终归属",
        "确定违法所得",
        "实际控制/代持已坐实",
        "最终资金归属已确认"
      ],
      next_review_actions: [
        claimTexts.length
          ? "对未支持或需纠正的研判结论补确定性交易边、回单、账户流水、导入/清洗口径和来源锚点。"
          : "先提供待复核报告文本或研判结论，再做逐项报告事实结论复核。",
        "正式报告级文本形成后，只允许围绕当前研判结论再做一次复核。",
        "对证据不足的资金链路先补确定性资金边，再决定是否画图。"
      ],
      facts: [
        makeDiagnosticFact({
          factId: "report.source.status",
          label: "报告来源状态",
          value: translateUserVisibleText(text(validation.validation_status || "需报告事实结论复核")),
          answerText: claimTexts.length
            ? `已接收 ${claimTexts.length} 条待复核研判结论；正式复用前必须通过报告事实结论复核和逐项核验材料复核。`
            : "未接收具体待复核研判结论；当前只能说明报告级复核边界，不能产出正式报告结论。",
          supportQueryName: claimTexts.length ? "validate_report_claims" : "claim_review.input_boundary",
          sourcePayload: validationResult || { claim_count: 0 }
        }),
        makeDiagnosticFact({
          factId: "case.total.turnover",
          label: "全案 directed 交易总量",
          count: coverage.directed_transaction_rows ?? coverage.txn_analyzed ?? coverage.txn_count,
          amount: coverage.turnover_yuan ?? coverage.turnover_total,
          unit: "yuan",
          answerText: `全案具备进/出方向的交易 ${coverage.directed_transaction_rows ?? coverage.txn_analyzed ?? coverage.txn_count ?? "未知"} 笔，资金流量 ${moneyText(coverage.turnover_yuan ?? coverage.turnover_total)}。`,
          supportQueryName: "get_scope_coverage",
          sourcePayload: coverageResult
        }),
        ...rows.slice(0, 8).map((row, index) => {
          const item = objectOf(row);
          const rowStatus = translateUserVisibleText(text(item.status || item.support_status || item.finding));
          return makeDiagnosticFact({
            factId: `claim_review.user_claim.${index + 1}`,
            label: "用户研判结论复核",
            value: rowStatus,
            answerText: [
              text(item.text || item.claim || item.summary) || `研判结论 ${index + 1}`,
              rowStatus ? `状态：${rowStatus}` : "",
              text(item.correction) ? `纠正：${text(item.correction)}` : "",
              text(item.boundary || item.reason) ? `边界：${text(item.boundary || item.reason)}` : ""
            ].filter(Boolean).join("；"),
            supportQueryName: "validate_report_claims",
            sourcePayload: validationResult
          });
        })
      ],
      warnings: [
        !claimTexts.length ? "没有用户提供的研判结论或报告文本，本卡不得补造固定报告事实。" : "",
        "报告中的可疑特征只能写成统计特征、线索、需核实或需复核，不能直接写成违法事实、实际控制、代持或最终资金归属。",
        "未形成交易级可证实资金边时，不能绘制确定性资金流向图链路。",
        ...diagnosticSkillWarnings(calls)
      ].filter(Boolean),
      unsupported_claims: [
        !claimTexts.length ? "未提供待复核研判结论，不能输出正式报告结论。" : "",
        "没有交易级可证实资金边时，资金链路片段只能作为聚合线索；证据不足或不能确认时，不得画资金流向图、流程图、箭头或确定性资金边。",
        "涉案性质、违法所得、实际控制、代持等法律评价只能写线索/需核实/需复核，不能写已查明或确定。"
      ].filter(Boolean),
      answer_constraints: [
        "报告级结论必须先做报告事实结论复核；证据不足的内容只能写成线索、需核实、需复核或建议补调。",
        "每个金额、笔数、主体和链路必须回到确定性工具事实；证据不足时写需核实或需补调。",
        "面向用户只输出复核结论、核验意见和下一步补调方向。"
      ],
      source_queries: diagnosticCallSummaries(calls)
    });
  }

  return { buildClaimReviewDiagnosticCard };
}
