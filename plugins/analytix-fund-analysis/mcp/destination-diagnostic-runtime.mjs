import {
  compactFinancialProductLeads,
  diagnosticCallSummaries,
  diagnosticSkillWarnings,
  makeDiagnosticFact
} from "./diagnostic-fact-helpers.mjs";
import { compactRankingRows } from "./agent-payload-compiler.mjs";
import { withAnswerCardProtocol } from "./answer-card-protocol.mjs";
import {
  arrayOf,
  intOrUndefined,
  numberOrUndefined,
  objectOf,
  pruneEmpty,
  text,
  uniqueTexts
} from "./runtime-normalizers.mjs";

export const DESTINATION_DIAGNOSTIC_RUNTIME_VERSION = "0.14.3-destination-diagnostic-runtime";

function humanNameText(value) {
  const raw = text(value);
  if (!raw || raw === "__unknown__" || /^counterparty:unknown$/iu.test(raw)) return "对手字段缺失";
  return raw;
}

function moneyNumber(value) {
  if (value && typeof value === "object") {
    const source = objectOf(value);
    return moneyNumber(source.yuan ?? source.amount_yuan ?? source.amount ?? source.value);
  }
  return numberOrUndefined(value);
}

function optionalIntNumber(value) {
  const next = intOrUndefined(value);
  return next !== undefined && next >= 0 ? next : undefined;
}

function transferSummaryComplete(source = {}, { dateStart = "", dateEnd = "", requireCounterpartyAccount = false } = {}) {
  const item = objectOf(source);
  const first = text(dateStart || item.first_txn_at || item.first_txn_time || item.min_txn_time || item.date_start);
  const last = text(dateEnd || item.last_txn_at || item.last_txn_time || item.max_txn_time || item.date_end);
  const counterpartyAccounts = uniqueTexts(item.counterparty_accounts, 4);
  return moneyNumber(item.amount) > 0
    && optionalIntNumber(item.txn_count) > 0
    && Boolean(first)
    && Boolean(last)
    && (!requireCounterpartyAccount || counterpartyAccounts.length > 0);
}

function summarySupportStatus(source, options = {}) {
  if (transferSummaryComplete(source, options)) return "supported";
  return Object.keys(objectOf(source)).length ? "needs_review" : "missing";
}

function dateRangeLabel(source = {}, fallback = "本轮可核验数据范围") {
  const item = objectOf(source);
  const firstDate = text(item.first_txn_at || item.first_txn_time || item.min_txn_time || item.date_start);
  const lastDate = text(item.last_txn_at || item.last_txn_time || item.max_txn_time || item.date_end);
  return firstDate || lastDate ? `${firstDate || "未知起"} 至 ${lastDate || "未知止"}` : fallback;
}

function scopedDateRangeLabel({ dateStart = "", dateEnd = "", scoped = {}, fallback = "本轮可核验数据范围" } = {}) {
  const item = objectOf(scoped);
  return dateRangeLabel({
    first_txn_at: text(dateStart) || item.first_txn_at || item.first_txn_time || item.min_txn_time,
    last_txn_at: text(dateEnd) || item.last_txn_at || item.last_txn_time || item.max_txn_time
  }, text(dateStart) ? `${text(dateStart)} 以来` : fallback);
}

export function destinationRowName(row) {
  const item = objectOf(row);
  return humanNameText(item.display_name || item.counterparty_name || item.counterparty_key || item.counterparty_account || item.account_key || "");
}

export function destinationRowAmount(row) {
  const item = objectOf(row);
  return moneyNumber(item.outflow ?? item.outflow_total ?? item.turnover ?? item.amount);
}

function destinationRowBoundary(row, dateStart) {
  const item = objectOf(row);
  const name = destinationRowName(item);
  const first = text(item.first_txn_at);
  const last = text(item.last_txn_at);
  const account = text(item.counterparty_account || item.counterparty_key || item.account_key);
  if (/对手字段缺失|空户名|__unknown__/u.test(name)) {
    return {
      category: "未匹配终点",
      boundary: "对手字段缺失/空户名只能写需复核，不能写成最终流向。"
    };
  }
  if (account && (!text(item.counterparty_name) || /^counterparty:/iu.test(name) || /^\d{8,}$/u.test(name))) {
    return {
      category: "账号型终点，户名待核",
      boundary: "需补调开户信息、收款账户流水、银行回单和账户归集，不能按空户名盲归属。"
    };
  }
  if (dateStart && last && last.slice(0, 10) < dateStart.slice(0, 10)) {
    return {
      category: name.includes("公司") ? "历史直接对手（单位）" : "历史直接对手（自然人）",
      boundary: `所查数据范围内的历史出账对象，不得直接写成 ${dateStart} 以来主要终点。`
    };
  }
  return {
    category: name.includes("公司") ? "直接对手（单位）" : "直接对手（自然人）",
    boundary: first || last
      ? "只能说明直接出账对手和时间范围；没有收款侧明细前不能确认最终流向。"
      : "只能说明直接出账对手；需补调后确认时间和下游。"
  };
}

function isSelfCounterparty(row, holderName) {
  const name = text(objectOf(row).display_name || objectOf(row).counterparty_name);
  return !!holderName && !!name && name === holderName;
}

export function createDestinationDiagnosticRuntime({ moneyText }) {
  const visibleMoney = (value) => {
    const amount = moneyNumber(value);
    return amount === undefined ? "金额未返回" : moneyText(amount);
  };
  const visibleCount = (value) => {
    const count = optionalIntNumber(value);
    return count === undefined ? "笔数未返回" : `${count} 笔`;
  };

  function buildDestinationOutflowDiagnosticCard({ caseId, holderName, dateStart = "", dateEnd = "", topN = 5, traceResult, counterpartyRankResult, calls = [] }) {
    const traceData = objectOf(traceResult?.data);
    const scopeStats = objectOf(traceData.scope_stats);
    const requestedTopN = Math.max(1, Math.min(Number(topN) || 5, 50));
    const rankRows = arrayOf(counterpartyRankResult?.data?.rankings)
      .filter((row) => !isSelfCounterparty(row, holderName))
      .slice(0, requestedTopN);
    const compactRows = compactRankingRows(rankRows, requestedTopN);
    const terminalClassification = compactRows.map((row) => {
      const name = destinationRowName(row);
      const boundary = destinationRowBoundary(row, dateStart);
      return pruneEmpty({
        name,
        category: boundary.category,
        amount: destinationRowAmount(row),
        txn_count: optionalIntNumber(row.txn_count),
        first_txn_at: text(row.first_txn_at),
        last_txn_at: text(row.last_txn_at),
        boundary: boundary.boundary
      }) || {};
    });
    const dateLabel = dateStart ? `${dateStart} 以来` : "本轮可核验数据范围";
    const classificationLine = terminalClassification
      .map((item) => `${text(item.name)}=${text(item.category)}`)
      .filter(Boolean)
      .join("；");
    const continuationTargets = terminalClassification.slice(0, Math.min(requestedTopN, 20)).map((item) => {
      const category = text(item.category);
      const target = text(item.name) || "未匹配终点";
      const reason = /未匹配|户名待核/u.test(category)
        ? "优先补调原始银行明细、开户信息、收款账户流水、银行回单和账户归集，先还原终点身份。"
        : /历史/u.test(category)
          ? `作为历史直接对手补交易背景、银行回单、账户归集和用途材料，不直接并入 ${dateLabel} 核心窗口。`
          : "补收款账户完整流水、银行回单、开户信息、账户归集和用途材料，核验是否存在下一跳和逐笔余额承接。";
      return { target, reason };
    });
    const qualityGuards = [
      "同事实：跨账户、重复交易号、换卡/补卡候选只能作为风险，未经回单或账户映射确认不能扣减或重复放大。",
      "空户名：对手字段缺失/空户名只能写未匹配终点或需复核，不能写成最终流向。",
      "换卡：候选账户、本人/同名账户、账号型终点必须区分直接归属和换卡线索。",
      "不能全流水去重：不得为了得到更顺的故事对全案或全流水盲去重。"
    ];
    const requiredFields = [
      "开户信息",
      "收款账户流水",
      "完整流水",
      "银行回单",
      "原始银行反馈",
      "账户归集",
      "关系材料",
      "交易用途材料",
      "对手户名/账号原始字段"
    ];
    const outAmount = scopeStats.out_amount ?? scopeStats.outflow_total;
    const txnCount = scopeStats.txn_count ?? scopeStats.directed_txn_count;
    const normalizedOutAmount = moneyNumber(outAmount);
    const normalizedTxnCount = optionalIntNumber(txnCount);
    const coreWindowComplete = normalizedOutAmount > 0 && normalizedTxnCount > 0;
    return withAnswerCardProtocol({
      card_type: "destination_outflow_card",
      case_id: caseId,
      intent: "destination",
      title: "Top 出账终点分类与补调核验材料",
      fact_source: "production_backend",
      holder_name: holderName,
      date_start: dateStart,
      date_end: dateEnd,
      terminal_classification: terminalClassification,
      continuation_targets: continuationTargets,
      required_subpoena_fields: requiredFields,
      quality_guards: qualityGuards,
      next_queries: [
        {
          tool: "validate_continuation_list",
          why_this_query: "仅在形成正式附件或补调清单前校验补调对象；同一问题不要重复校验，用户继续追一层、改口径、指定新对象或提出新假设时走对应语义事实工具。"
        }
      ],
      facts: [
        makeDiagnosticFact({
          factId: "destination.core_window.scope",
          label: "核心窗口出账口径",
          holder: holderName,
          count: normalizedTxnCount,
          amount: normalizedOutAmount,
          unit: "yuan",
          timeRange: dateLabel,
          supportStatus: coreWindowComplete ? "supported" : "missing",
          answerText: coreWindowComplete
            ? `核心窗口 ${dateLabel}：${holderName || "主体"}来源交易 ${normalizedTxnCount} 笔，出账 ${visibleMoney(normalizedOutAmount)}，折合 ${(normalizedOutAmount / 10000).toFixed(6)} 万元；该数字是出账发生额口径，不等于最终流向或非法所得。`
            : `核心窗口 ${dateLabel} 的金额或笔数未完整返回；不得将缺失值写成 0、无交易或无异常。`,
          supportQueryName: "trace_subject_top_outflows.scope_stats",
          sourcePayload: traceResult
        }),
        makeDiagnosticFact({
          factId: "destination.terminal_classification.required",
          label: "终点分类",
          value: classificationLine,
          answerText: `终点分类：${classificationLine || "按返回 Top 出账逐项分类"}；未匹配终点不能写成最终流向。`,
          supportQueryName: "rank_counterparties.outflow+trace_subject_top_outflows",
          sourcePayload: { trace: traceResult, counterparty_rank: counterpartyRankResult }
        }),
        makeDiagnosticFact({
          factId: "destination.continuation.required",
          label: "补调对象",
          value: "补调对象/开户信息/收款账户流水/完整流水/银行回单/原始银行反馈/账户归集/关系材料",
          answerText: "补调对象：开户信息、收款账户流水、完整流水、银行回单、原始银行反馈、账户归集、关系材料；形成正式附件前再用 validate_continuation_list 校验补调清单。",
          supportQueryName: "trace_subject_top_outflows.followup_requests",
          sourcePayload: traceResult
        }),
        makeDiagnosticFact({
          factId: "destination.quality_guards.required",
          label: "质量护栏",
          value: "同事实/空户名/换卡/不能全流水去重",
          answerText: "质量护栏：同事实、空户名、换卡、不能全流水去重；不得把候选账户、账号型终点或未匹配终点写成确定归属。",
          supportQueryName: "trace_subject_top_outflows+rank_counterparties.boundary",
          sourcePayload: { trace: traceResult, counterparty_rank: counterpartyRankResult }
        }),
        ...terminalClassification.map((item, index) => {
          const complete = moneyNumber(item.amount) !== undefined && optionalIntNumber(item.txn_count) !== undefined;
          return makeDiagnosticFact({
          factId: `destination.terminal.rank${index + 1}`,
          label: "终点分类",
          counterparty: text(item.name),
          count: optionalIntNumber(item.txn_count),
          amount: moneyNumber(item.amount),
          unit: "yuan",
          timeRange: text(item.first_txn_at) || text(item.last_txn_at) ? `${text(item.first_txn_at) || "未知起"}~${text(item.last_txn_at) || "未知止"}` : "",
          supportStatus: complete ? "supported" : "missing",
          answerText: complete
            ? `${index + 1}. ${text(item.name)}：${text(item.category)}，出账 ${visibleMoney(item.amount)}，${visibleCount(item.txn_count)}；${text(item.boundary)}`
            : `${index + 1}. ${text(item.name)}：${text(item.category)}；金额或笔数未完整返回，只能列为待补证边界，不得写成 0。${text(item.boundary)}`,
          supportQueryName: "rank_counterparties.outflow",
          sourcePayload: counterpartyRankResult
          });
        })
      ],
      warnings: [
        ...diagnosticSkillWarnings(calls)
      ],
      unsupported_claims: [
        "未匹配终点、空户名、账号型终点不得写成最终流向。",
        "所查数据范围 Top 不等于指定时间窗口 Top；未返回窗口分拆金额时不得套用历史范围金额。",
        "没有收款账户流水、银行回单、账户归集和逐笔余额承接时，不得画确定性资金链。",
        "质量护栏应说明同事实、空户名、换卡和不能全流水去重。"
      ],
      forbidden_as_facts: [
        "把未匹配终点写成最终流向。",
        "把候选账户写成名下账户。",
        "把同事实/换卡候选直接全流水去重。",
        "把历史直接对手写成 2025 年以来主要终点。",
        "把补调对象写成已查明最终收款人。"
      ],
      answer_constraints: [
        "直接回答终点分类、补调对象、质量护栏和需复核边界。",
        "补调对象需覆盖开户信息、收款账户流水、完整流水、银行回单、原始银行反馈、账户归集和关系材料。",
        "边界需说明不能写成最终流向、不能确认、补调、未匹配和需复核。",
        "普通出账题拿到本卡即可作答；形成正式附件或用户继续追一层时再追加后续核验。"
      ]
    }, {
      recommendedNextAction: "answer_now",
      maxAdditionalTools: 0,
      requiredFactsPresent: coreWindowComplete && terminalClassification.some(
        (item) => moneyNumber(item.amount) > 0 && optionalIntNumber(item.txn_count) > 0
      ),
      unsupportedFlowsPresent: true
    });
  }

  function buildViaOneHopDiagnosticCard({
    caseId,
    holderName,
    viaName,
    dateStart = "",
    dateEnd = "",
    sourceToViaSummary = {},
    sourceToViaScopedSummary = {},
    sourceToViaCoreAccountSummary = {},
    calls = []
  }) {
    const fullPeriod = objectOf(sourceToViaSummary);
    const scoped = objectOf(sourceToViaScopedSummary);
    const coreAccount = objectOf(sourceToViaCoreAccountSummary);
    const fullPeriodDateLabel = dateRangeLabel(fullPeriod, "完整数据范围内");
    const scopedDateLabel = scopedDateRangeLabel({ dateStart, dateEnd, scoped });
    const coreAccountDateLabel = dateRangeLabel(coreAccount, scopedDateLabel);
    const scopedCluster = objectOf(scoped.largest_date_cluster);
    const fullCluster = objectOf(fullPeriod.largest_date_cluster);
    const coreCluster = objectOf(coreAccount.largest_date_cluster);
    const coreAccounts = uniqueTexts(coreAccount.counterparty_accounts, 4);
    const fullSourceAccounts = uniqueTexts(fullPeriod.source_accounts, 4);
    const scopedSourceAccounts = uniqueTexts(scoped.source_accounts, 4);
    const coreSourceAccounts = uniqueTexts(coreAccount.source_accounts, 4);
    const coreDuplicateSourceAccounts = uniqueTexts(coreAccount.duplicate_source_accounts, 4);
    const sourceAccountText = (accounts, label = "付款账号") => accounts.length ? `${label} ${accounts.join("/")}` : "";
    const duplicateSourceText = (accounts) => accounts.length ? `同事实重复候选付款账号 ${accounts.join("/")}` : "";
    const fullAmount = moneyNumber(fullPeriod.amount);
    const scopedAmount = moneyNumber(scoped.amount);
    const coreAmount = moneyNumber(coreAccount.amount);
    const returnedAmounts = [fullAmount, scopedAmount, coreAmount].filter((item) => item > 0);
    const amountsDiffer = new Set(returnedAmounts.map((item) => item.toFixed(2))).size > 1;
    const duplicateBoundaryText = [
      amountsDiffer ? "收款人姓名匹配、时间窗口和重点收款账户统计范围不一致，需作为竞争金额业务解释分列，不得写成单一事实金额。" : "",
      duplicateSourceText(coreDuplicateSourceAccounts)
        ? `${duplicateSourceText(coreDuplicateSourceAccounts)}只能写成重复放大风险，不能累计为新增确定性交易边。`
        : "如存在同事实或换卡候选，只能写成重复放大风险，不能直接累计或扣减。"
    ].filter(Boolean).join("；") || "不同金额观察范围需回到交易号、时间、账户和对手方逐项复核。";
    const facts = [
      makeDiagnosticFact({
        factId: "via_one_hop.full_period",
        label: "收款人姓名匹配结果",
        holder: holderName,
        counterparty: viaName,
        count: optionalIntNumber(fullPeriod.txn_count),
        amount: moneyNumber(fullPeriod.amount),
        unit: "yuan",
        timeRange: fullPeriodDateLabel,
        supportStatus: summarySupportStatus(fullPeriod),
        answerText: Object.keys(fullPeriod).length
          ? `${holderName || "来源主体"}向${viaName || "对手方"}转账，${fullPeriodDateLabel}，收款人姓名匹配结果 ${visibleCount(fullPeriod.txn_count)}，合计 ${visibleMoney(fullPeriod.amount)}；主要集中日 ${text(fullCluster.date) || "未知日期"} ${visibleCount(fullCluster.txn_count)}/${visibleMoney(fullCluster.amount)}${sourceAccountText(fullSourceAccounts) ? `；${sourceAccountText(fullSourceAccounts)}` : ""}。`
          : `${holderName || "来源主体"}向${viaName || "对手方"}转账的收款人姓名匹配结果未稳定返回，不能写成已查明金额。`,
        supportQueryName: "rank_counterparties.outflow.source_to_via.full_period",
        sourcePayload: { sourceToViaSummary }
      }),
      makeDiagnosticFact({
        factId: "via_one_hop.date_window",
        label: "时间窗口核验",
        holder: holderName,
        counterparty: viaName,
        count: optionalIntNumber(scoped.txn_count),
        amount: moneyNumber(scoped.amount),
        unit: "yuan",
        timeRange: scopedDateLabel,
        supportStatus: summarySupportStatus(scoped, { dateStart, dateEnd }),
        answerText: Object.keys(scoped).length
          ? `时间窗口核验：${holderName || "来源主体"}向${viaName || "对手方"}转账，${scopedDateLabel}，按有效交易号去重 ${visibleCount(scoped.txn_count)}，合计 ${visibleMoney(scoped.amount)}；集中日 ${text(scopedCluster.date) || "未知日期"} ${visibleCount(scopedCluster.txn_count)}/${visibleMoney(scopedCluster.amount)}${sourceAccountText(scopedSourceAccounts) ? `；${sourceAccountText(scopedSourceAccounts)}` : ""}。`
          : `${scopedDateLabel} 的 ${holderName || "来源主体"}向${viaName || "对手方"} 时间窗口聚合未稳定返回，只能说明需复核。`,
        supportQueryName: "rank_counterparties.outflow.source_to_via.date_window",
        sourcePayload: { sourceToViaScopedSummary }
      }),
      makeDiagnosticFact({
        factId: "via_one_hop.core_account",
        label: "重点收款账户流水",
        holder: holderName,
        counterparty: viaName,
        count: optionalIntNumber(coreAccount.txn_count),
        amount: moneyNumber(coreAccount.amount),
        unit: "yuan",
        timeRange: coreAccountDateLabel,
        supportStatus: summarySupportStatus(coreAccount, { dateStart, dateEnd, requireCounterpartyAccount: true }),
        answerText: Object.keys(coreAccount).length
          ? `${holderName || "来源主体"}向${viaName || "对手方"}转账，${sourceAccountText(coreSourceAccounts, "付款账号") ? `${sourceAccountText(coreSourceAccounts, "付款账号")}，` : ""}收款账号 ${coreAccounts.join("/") || "待核"}，按收款账号和有效交易号核验 ${visibleCount(coreAccount.txn_count)}，合计 ${visibleMoney(coreAccount.amount)}；主要集中日 ${text(coreCluster.date) || "未知日期"} ${visibleCount(coreCluster.txn_count)}/${visibleMoney(coreCluster.amount)}；${duplicateSourceText(coreDuplicateSourceAccounts) ? `${duplicateSourceText(coreDuplicateSourceAccounts)}作为重复放大风险单列，不计入新增交易边；` : ""}该结果只能证明一跳收款事实，尚不能证明后续资金闭环。`
          : "重点收款账户流水统计未稳定返回；不得把收款人姓名匹配结果直接写成最终穿透。",
        supportQueryName: "rank_counterparties.outflow.source_to_via.core_account",
        sourcePayload: { sourceToViaCoreAccountSummary }
      }),
      makeDiagnosticFact({
        factId: "via_one_hop.duplicate_guard",
        label: "金额差异与重复放大风险",
        holder: holderName,
        counterparty: viaName,
        amount: returnedAmounts.length ? Math.max(...returnedAmounts) : undefined,
        unit: "yuan",
        supportStatus: amountsDiffer || coreDuplicateSourceAccounts.length ? "needs_review" : "unresolved",
        answerText: duplicateBoundaryText,
        supportQueryName: "dedupe_guard.source_to_via",
        sourcePayload: { fullPeriod, scoped, coreAccount }
      })
    ];
    return withAnswerCardProtocol({
      card_type: "source_to_counterparty_amount_card",
      case_id: caseId,
      intent: "destination",
      title: `${holderName || "来源主体"}至${viaName || "对手方"}一跳金额核验材料`,
      fact_source: "production_backend",
      holder_name: holderName,
      via_holder_name: viaName,
      date_start: dateStart,
      date_end: dateEnd,
      answer_sections: ["可核金额事实", "时间/账户集中", "重复放大风险", "边界"],
      facts,
      source_to_via_summary: fullPeriod,
      source_to_via_date_window_summary: scoped,
      source_to_via_core_account_summary: coreAccount,
      unsupported_flows: [
        amountsDiffer ? "不同观察范围金额不一致时，不能把最大 raw/聚合值直接写成已查明事实金额。" : "",
        `${duplicateSourceText(coreDuplicateSourceAccounts) || "同事实重复候选付款账号待复核"}只能写成重复放大风险，不得写成第二条确定性付款边。`,
        "重点收款账户只证明一跳收款事实；未核后续账户流水、银行回单和余额承接前，不能写成最终资金闭环。",
        "本题只问一跳金额时，不得自行扩展到后续去向、案件关系图或资金流向图。"
      ].filter(Boolean),
      quality_guards: [
        "收款人姓名匹配结果、集中日期和重点收款账户是三个不同观察角度，应简明解释差异。",
        `${sourceAccountText(coreSourceAccounts.length ? coreSourceAccounts : fullSourceAccounts, "付款账号") || "付款账号待从生产事实复核"}；收款账号 ${coreAccounts.join("/") || "待核"}。`,
        "同事实去重只用于防止重复放大，不能对全案或全流水盲去重。",
        "候选卡/同名户名聚合不得升级为名下确定账户。"
      ],
      stop_conditions: [
        "普通一跳金额题到此先作答。",
        `只有用户继续问 ${viaName || "对手方"} 后续/下游/又转给谁，才进入 continuation card。`
      ],
      next_queries: [
        {
          query: `如果后续追问 ${viaName || "对手方"} 又给谁，再以 via_holder_name=${viaName || "续查对象"} 调用 funds_investigate(destination)。`,
          reason: "后续去向是另一类任务，不能和本次一跳金额核验混写。"
        }
      ],
      source_queries: diagnosticCallSummaries(calls),
      answer_constraints: [
        "直接回答实际统计期间、时间窗口和核心账号已经返回的金额、笔数和账号。",
        "用经侦材料口吻说明金额结论、集中日期、主要账号、大额交易、异常或待核点、资金意义和下一步核查。",
        "如观察范围不一致，解释差异来源和重复放大风险，不能把未支持范围写成已查明事实。",
        "不要输出 q_xxx、audit_ref、artifact_id、evidence_refs。"
      ]
    }, {
      recommendedNextAction: "answer_now",
      maxAdditionalTools: 0,
      requiredFactsPresent: ["via_one_hop.full_period", "via_one_hop.core_account"].every(
        (factId) => text(facts.find((fact) => fact.fact_id === factId)?.support_status) === "supported"
      ),
      unsupportedFlowsPresent: true
    });
  }

  function buildViaContinuationDiagnosticCard({
    caseId,
    holderName,
    viaName,
    dateStart = "",
    dateEnd = "",
    sourceToViaSummary = {},
    sourceToViaScopedSummary = {},
    sourceToViaCoreAccountSummary = {},
    viaCounterpartyRank,
    viaFinancialProbe,
    inlineFlowGraph,
    calls = []
  }) {
    const fullPeriod = objectOf(sourceToViaSummary);
    const scoped = objectOf(sourceToViaScopedSummary);
    const coreAccount = objectOf(sourceToViaCoreAccountSummary);
    const viaRows = compactRankingRows(viaCounterpartyRank?.data?.rankings || [], 8)
      .filter((row) => !isSelfCounterparty(row, viaName));
    const viaClues = compactFinancialProductLeads(viaFinancialProbe?.data || {}, 6);
    const graphEdges = arrayOf(inlineFlowGraph?.flow_graph?.edges).slice(0, 8);
    const fullPeriodDateLabel = dateRangeLabel(fullPeriod, "完整数据范围内");
    const scopedDateLabel = scopedDateRangeLabel({ dateStart, dateEnd, scoped });
    const coreAccountDateLabel = dateRangeLabel(coreAccount, scopedDateLabel);
    const sourceToViaFacts = [
      makeDiagnosticFact({
        factId: "via_continuation.source_to_via.full_period",
        label: "来源主体至续查对象姓名匹配结果",
        holder: holderName,
        counterparty: viaName,
        count: optionalIntNumber(fullPeriod.txn_count),
        amount: moneyNumber(fullPeriod.amount),
        unit: "yuan",
        timeRange: fullPeriodDateLabel,
        supportStatus: summarySupportStatus(fullPeriod),
        answerText: Object.keys(fullPeriod).length
          ? `${holderName || "来源主体"}向${viaName || "续查对象"}转账，${fullPeriodDateLabel}，收款人姓名匹配结果 ${visibleCount(fullPeriod.txn_count)}，合计 ${visibleMoney(fullPeriod.amount)}；主要集中日 ${text(objectOf(fullPeriod.largest_date_cluster).date) || "未知日期"} ${visibleCount(objectOf(fullPeriod.largest_date_cluster).txn_count)}/${visibleMoney(objectOf(fullPeriod.largest_date_cluster).amount)}。`
          : `${holderName || "来源主体"}向${viaName || "续查对象"}转账的收款人姓名匹配结果未稳定返回，不能写成已查明穿透链路。`,
        supportQueryName: "rank_counterparties.outflow.source_to_via.full_period",
        sourcePayload: { sourceToViaSummary }
      }),
      makeDiagnosticFact({
        factId: "via_continuation.source_to_via.date_window",
        label: "来源主体至续查对象时间窗口核验",
        holder: holderName,
        counterparty: viaName,
        count: optionalIntNumber(scoped.txn_count),
        amount: moneyNumber(scoped.amount),
        unit: "yuan",
        timeRange: scopedDateLabel,
        supportStatus: summarySupportStatus(scoped, { dateStart, dateEnd }),
        answerText: Object.keys(scoped).length
          ? `时间窗口核验：${holderName || "来源主体"}向${viaName || "续查对象"}转账，${scopedDateLabel}，按有效交易号去重 ${visibleCount(scoped.txn_count)}，合计 ${visibleMoney(scoped.amount)}；集中日 ${text(objectOf(scoped.largest_date_cluster).date) || "未知日期"} ${visibleCount(objectOf(scoped.largest_date_cluster).txn_count)}/${visibleMoney(objectOf(scoped.largest_date_cluster).amount)}。`
          : `${scopedDateLabel} 的 ${holderName || "来源主体"}向${viaName || "续查对象"} 时间窗口聚合未稳定返回，只能列需补调。`,
        supportQueryName: "rank_counterparties.outflow.source_to_via.date_window",
        sourcePayload: { sourceToViaScopedSummary }
      }),
      makeDiagnosticFact({
        factId: "via_continuation.source_to_via.core_account",
        label: "重点收款账户流水",
        holder: holderName,
        counterparty: viaName,
        count: optionalIntNumber(coreAccount.txn_count),
        amount: moneyNumber(coreAccount.amount),
        unit: "yuan",
        timeRange: coreAccountDateLabel,
        supportStatus: summarySupportStatus(coreAccount, { dateStart, dateEnd, requireCounterpartyAccount: true }),
        answerText: Object.keys(coreAccount).length
          ? `${holderName || "来源主体"}向${viaName || "续查对象"}转账，收款账号 ${uniqueTexts(coreAccount.counterparty_accounts, 4).join("/") || "待核"}，按有效交易号核验 ${visibleCount(coreAccount.txn_count)}，合计 ${visibleMoney(coreAccount.amount)}；该账号聚合只证明一跳收款事实，不证明最终资金闭环。`
          : "重点收款账户流水统计未稳定返回；不得把收款人姓名匹配结果直接写成最终穿透。",
        supportQueryName: "rank_counterparties.outflow.source_to_via.core_account",
        sourcePayload: { sourceToViaCoreAccountSummary }
      })
    ];
    const viaRankFacts = viaRows.slice(0, 5).map((row, index) => {
      const amount = destinationRowAmount(row);
      const count = optionalIntNumber(row.txn_count);
      const complete = amount > 0 && count > 0 && Boolean(text(row.first_txn_at)) && Boolean(text(row.last_txn_at));
      return makeDiagnosticFact({
        factId: `via_continuation.via_outflow.rank${index + 1}`,
        label: "续查对象后续出账",
        holder: viaName,
        counterparty: destinationRowName(row),
        count,
        amount,
        unit: "yuan",
        timeRange: text(row.first_txn_at) || text(row.last_txn_at) ? `${text(row.first_txn_at) || "未知起"}~${text(row.last_txn_at) || "未知止"}` : "",
        supportStatus: complete ? "supported" : "missing",
        answerText: complete
          ? `${index + 1}. ${viaName || "续查对象"} 后续出账对手 ${destinationRowName(row)}：出账 ${visibleMoney(amount)}，${visibleCount(count)}，${text(row.first_txn_at) || "未知起"} 至 ${text(row.last_txn_at) || "未知止"}；这是一跳后续对手方统计，未做逐笔余额承接前不能写成 ${holderName || "来源主体"} 原资金确定穿透。`
          : `${index + 1}. ${viaName || "续查对象"} 后续出账对手 ${destinationRowName(row)} 的金额或笔数未完整返回；不得写成 0 或确定穿透。`,
        supportQueryName: "rank_counterparties.outflow.via_holder",
        sourcePayload: viaCounterpartyRank
      });
    });
    const financialFacts = viaClues.slice(0, 4).map((clue, index) => {
      const amount = moneyNumber(clue.outflow_total ?? clue.inflow_total);
      const direction = moneyNumber(clue.outflow_total) !== undefined ? "出账" : "入账";
      return makeDiagnosticFact({
        factId: `via_continuation.financial_product.${index + 1}`,
        label: "理财/资产转换线索",
        holder: viaName,
        counterparty: text(clue.counterparty_name) || "理财/资产端",
        count: clue.txn_count,
        amount,
        direction,
        unit: "yuan",
        timeRange: text(clue.first_txn_at) && text(clue.first_txn_at) === text(clue.last_txn_at)
          ? text(clue.first_txn_at)
          : text(clue.first_txn_at) || text(clue.last_txn_at) ? `${text(clue.first_txn_at) || "未知起"}~${text(clue.last_txn_at) || "未知止"}` : "",
        supportStatus: amount === undefined ? "needs_evidence" : "lead",
        answerText: amount === undefined
          ? `${text(clue.counterparty_name) || "理财/资产端"} 的金额未返回，只能列为待核线索，不得写成 0。`
          : `${text(clue.counterparty_name) || "理财/资产端"}：${text(clue.first_txn_at) || "时间待核"}，${direction} ${visibleMoney(amount)}；摘要/产品 ${uniqueTexts(clue.business_texts, 3).join("/") || "待核"}；只能写为理财/资产转换线索，不能直接写成确定性资金边或现金去向。`,
        supportQueryName: "hypothesis_probe.via_holder.financial_product",
        sourcePayload: viaFinancialProbe
      });
    });
    const graphFacts = graphEdges.slice(0, 4).map((edge, index) => {
      const item = objectOf(edge);
      return makeDiagnosticFact({
        factId: `via_continuation.supported_edge.${index + 1}`,
        label: "可解释交易边",
        holder: text(item.from_label),
        counterparty: text(item.to_label),
        count: 1,
        amount: moneyNumber(item.amount),
        unit: "yuan",
        timeRange: text(item.txn_time),
        supportStatus: text(item.edge_status) === "supported"
          && moneyNumber(item.amount) > 0
          && Boolean(text(item.from_label))
          && Boolean(text(item.to_label))
          && Boolean(text(item.txn_time))
          ? "supported"
          : "needs_evidence",
        answerText: moneyNumber(item.amount) === undefined
          ? `${text(item.from_label) || holderName} -> ${text(item.to_label) || viaName} 的金额未返回；不得补画叙事箭头或写成 0。`
          : `${text(item.from_label) || holderName} -> ${text(item.to_label) || viaName}：${visibleMoney(item.amount)}，${text(item.txn_time) || "时间待核"}；只允许解释本条返回交易边，不得补画叙事箭头。`,
        supportQueryName: "build_fund_flow_graph.edge_preview",
        sourcePayload: inlineFlowGraph
      });
    });
    const allFacts = [
      ...sourceToViaFacts,
      ...viaRankFacts,
      ...financialFacts,
      ...graphFacts
    ];
    const continuationPathLines = [
      `${holderName || "来源主体"} -> ${viaName || "续查对象"}：先按收款人姓名匹配结果、时间窗口和重点收款账户三种观察角度复核一跳金额，防止重复放大。`,
      ...viaClues.slice(0, 3).map((item) => {
        const amount = moneyNumber(item.outflow_total ?? item.inflow_total);
        return amount === undefined
          ? `${viaName || "续查对象"} -> ${text(item.counterparty_name) || "理财/资产端"}：金额未返回，只能列为待核线索，不得写成 0。`
          : `${viaName || "续查对象"} -> ${text(item.counterparty_name) || "理财/资产端"}：${text(item.first_txn_at) || "时间待核"}，${moneyNumber(item.outflow_total) !== undefined ? "出账" : "入账"} ${visibleMoney(amount)}；作为资产转换线索，需补产品凭证、回单和余额承接。`;
      }),
      ...viaRows.slice(0, 4).map((row) =>
        destinationRowAmount(row) > 0 && optionalIntNumber(row.txn_count) > 0 && text(row.first_txn_at) && text(row.last_txn_at)
          ? `${viaName || "续查对象"} -> ${destinationRowName(row)}：后续出账 ${visibleMoney(destinationRowAmount(row))}，${visibleCount(row.txn_count)}；未逐笔匹配前不能写成原资金确定穿透。`
          : `${viaName || "续查对象"} -> ${destinationRowName(row)}：金额或笔数未完整返回，只能列为补证边界，不得写成 0。`
      )
    ];
    const continuationTargetRows = [
      { target: viaName || "续查对象", reason: "补收款账户完整流水、银行回单、开户信息、账户归集和用途材料，核验一跳收款和后续出账是否存在逐笔余额承接。" },
      ...viaClues.slice(0, 3).map((item) => ({
        target: text(item.counterparty_name) || "理财/资产端",
        reason: "补产品合同、申购/赎回确认、资金回单和产品资金去向；不能把资产转换线索写成现金去向。"
      })),
      ...viaRows.slice(0, 4).map((row) => ({
        target: destinationRowName(row),
        reason: "补对手账户流水、开户信息、银行回单和交易背景；未核验前只写线索。"
      }))
    ];
    const hasFinancialLeads = viaClues.length > 0;
    const hasViaRankRows = viaRows.length > 0;
    return withAnswerCardProtocol({
      card_type: "destination_outflow_card",
      case_id: caseId,
      intent: "destination",
      title: `${viaName || "续查对象"}后续去向复核材料`,
      fact_source: "production_backend",
      holder_name: holderName,
      via_holder_name: viaName,
      date_start: dateStart,
      date_end: dateEnd,
      answer_sections: ["一跳核验", "后续去向", "资产转换线索", "不能确认边界", "补调清单"],
      facts: allFacts,
      continuation_paths: continuationPathLines,
      wealth_management_or_asset_clues: viaClues.map((item) =>
        `${text(item.counterparty_name) || "理财/资产端"}：${text(item.first_txn_at) || "时间待核"}，${item.outflow_total ? "出账" : "入账"} ${moneyText(item.outflow_total || item.inflow_total)}；只能写理财/资产转换线索。`
      ),
      continuation_targets: continuationTargetRows,
      required_subpoena_fields: ["开户信息", "收款账户流水", "完整流水", "银行回单", "原始银行反馈", "账户归集", "关系材料", "产品合同/申购赎回确认", "交易用途材料"],
      quality_guards: [
        "同事实：按有效交易号、时间、金额和对手去重，不能把重复放大口径写成事实。",
        "空户名：空户名或缺失对手只能写需复核，不能补成最终流向。",
        "换卡：重点收款账户和收款人姓名匹配结果需区分，不能把候选卡写成名下确定账户。",
        "不能全流水去重：不得为了得到顺畅链路对全案或全流水盲去重。"
      ],
      unsupported_flows: [
        `${viaName || "续查对象"}后续线索未做逐笔余额承接，不能直接证明 ${holderName || "来源主体"} 原资金已穿透。`,
        "时间邻近、金额接近只能提示高优先级复核，不能直接写成确定性资金边。",
        "未返回可证实交易边的链路不得画资金流向图或箭头。"
      ],
      forbidden_as_facts: [
        "把理财/认申购/赎回写成现金去向。",
        "把候选账户写成名下账户。",
        "把重复放大口径写成事实金额。",
        "把后续对手统计写成已查明最终资金流向。"
      ],
      next_queries: [
        { tool: "validate_continuation_list", why_this_query: "只有形成正式补调附件前再校验续查清单；本普通问答拿到本卡后应直接作答。" },
        { tool: "trace_fund_next_hop", why_this_query: "用户指定某一条交易或回单后，再做下一跳逐笔核验。" }
      ],
      answer_constraints: [
        `先写 ${holderName || "来源主体"} -> ${viaName || "续查对象"} 的一跳核验，再写 ${viaName || "续查对象"} 的后续去向。`,
        "只点名本卡返回的后续出账、资产转换或可证实交易边线索，并明确不能直接证明原资金穿透。",
        "说明不能直接证明、不能直接写成确定性资金边、不能直接认定、逐笔余额承接、银行回单、收款账户流水。",
        "补调清单形成正式附件前才使用 validate_continuation_list；用户继续指定下一跳时可重新进入前门。"
      ],
      warnings: [
        hasFinancialLeads ? "" : "理财/资产转换线索未稳定返回；只能说明事实缺口和补调方向。",
        hasViaRankRows ? "" : "后续对手排名未稳定返回；不得硬写为事实。",
        ...diagnosticSkillWarnings(calls)
      ].filter(Boolean)
    }, {
      recommendedNextAction: "answer_now",
      maxAdditionalTools: 0,
      requiredFactsPresent: sourceToViaFacts.some((fact) => text(fact.support_status) === "supported")
        && [...viaRankFacts, ...graphFacts].some((fact) => text(fact.support_status) === "supported"),
      unsupportedFlowsPresent: true
    });
  }

  return {
    buildDestinationOutflowDiagnosticCard,
    buildViaOneHopDiagnosticCard,
    buildViaContinuationDiagnosticCard
  };
}
