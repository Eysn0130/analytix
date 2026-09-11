import {
  amountFromDiagnosticRow,
  diagnosticCounterpartyName,
  diagnosticRowName,
  diagnosticRowTimeRange,
  diagnosticSkillWarnings,
  makeDiagnosticFact
} from "./diagnostic-fact-helpers.mjs";
import {
  arrayOf,
  intValue,
  objectOf,
  text
} from "./runtime-normalizers.mjs";

export const TOP_RANKINGS_DIAGNOSTIC_RUNTIME_VERSION = "0.14.3-top-rankings-diagnostic-runtime";

function topRankingMetricLabel(metric) {
  const value = text(metric);
  if (value === "inflow") return "入账";
  if (value === "outflow") return "出账";
  if (value === "txn_count") return "笔数";
  if (value === "max_single_amount") return "最大单笔";
  return "往来总额";
}

export function createTopRankingsDiagnosticRuntime({ safeSkill, moneyText }) {
  function diagnosticMetricRankFact(metric, rankResult) {
    const row = objectOf(arrayOf(objectOf(rankResult).data?.rankings)[0]);
    const metricLabel = topRankingMetricLabel(metric);
    const amountValue = metric === "txn_count"
      ? intValue(row.txn_count)
      : amountFromDiagnosticRow(row, metric);
    const amountText = metric === "txn_count" ? `${amountValue || "未知"} 笔` : moneyText(amountValue);
    const accountName = diagnosticRowName(row, ["account_key", "acct_display", "account", "card_no", "acct_no"]) || "未知账户";
    const holderName = diagnosticRowName(row, ["holder_name", "account_open_name", "open_name", "display_name"]) || "未知户名";
    const metricField = metric === "max_single_amount" ? "max_single" : metric;
    return makeDiagnosticFact({
      factId: `top_account.metric.${metric}.rank1`,
      label: `${metricLabel} Top`,
      account: accountName,
      holder: holderName,
      count: row.txn_count ?? row.count,
      amount: metric === "txn_count" ? amountFromDiagnosticRow(row, "turnover") : amountValue,
      unit: metric === "txn_count" ? "rows" : "yuan",
      timeRange: diagnosticRowTimeRange(row),
      answerText: `${metricLabel} Top：账户 ${accountName}，户名 ${holderName}，${metricLabel} ${amountText}，交易 ${row.txn_count ?? "未知"} 笔，入账 ${moneyText(amountFromDiagnosticRow(row, "inflow"))}，出账 ${moneyText(amountFromDiagnosticRow(row, "outflow"))}，往来总额 ${moneyText(amountFromDiagnosticRow(row, "turnover"))}，最大单笔 ${moneyText(amountFromDiagnosticRow(row, "max_single_amount"))}；本块由 rank_accounts(metric="${metric}") 独立排序，不能复用其他指标 Top。`,
      supportQueryName: `rank_accounts.${metricField}`,
      sourcePayload: rankResult
    });
  }

  async function buildTopRankingsDiagnosticCard(caseId, { signal } = {}) {
    const coverageResult = await safeSkill("get_scope_coverage", { case_id: caseId }, { signal });
    const dataQualityResult = await safeSkill("audit_case_data_quality", { case_id: caseId, example_limit: 3 }, { signal });
    const accountRank = await safeSkill("rank_accounts", { case_id: caseId, metric: "turnover", direction_mode: "both", success_filter: "all", cash_filter: "all", limit: 3 }, { signal });
    const holderRank = await safeSkill("rank_holders", { case_id: caseId, metric: "turnover", direction_mode: "both", success_filter: "all", cash_filter: "all", limit: 3 }, { signal });
    const counterpartyRank = await safeSkill("rank_counterparties", { case_id: caseId, metric: "turnover", direction_mode: "both", success_filter: "all", cash_filter: "all", counterparty_group_mode: "name", limit: 3 }, { signal });
    const accountInflowRank = await safeSkill("rank_accounts", { case_id: caseId, metric: "inflow", direction_mode: "in", success_filter: "all", cash_filter: "all", limit: 3 }, { signal });
    const accountOutflowRank = await safeSkill("rank_accounts", { case_id: caseId, metric: "outflow", direction_mode: "out", success_filter: "all", cash_filter: "all", limit: 3 }, { signal });
    const accountTxnCountRank = await safeSkill("rank_accounts", { case_id: caseId, metric: "txn_count", direction_mode: "both", success_filter: "all", cash_filter: "all", limit: 3 }, { signal });
    const accountMaxSingleRank = await safeSkill("rank_accounts", { case_id: caseId, metric: "max_single_amount", direction_mode: "both", success_filter: "all", cash_filter: "all", limit: 3 }, { signal });
    const coverage = objectOf(coverageResult.data.coverage || coverageResult.data);
    const importLineage = objectOf(objectOf(dataQualityResult.data).import_lineage);
    const importSummary = objectOf(importLineage.summary);
    const account = objectOf(arrayOf(accountRank.data.rankings)[0]);
    const holder = objectOf(arrayOf(holderRank.data.rankings)[0]);
    const holderSummary = objectOf(holderRank.data.group_summary);
    const counterparties = arrayOf(counterpartyRank.data.rankings);
    const first = objectOf(counterparties[0]);
    const second = objectOf(counterparties[1]);
    const third = objectOf(counterparties[2]);
    const topAccountHolderName = diagnosticRowName(account, ["holder_name", "account_open_name", "open_name", "display_name"]);
    const topHolderName = diagnosticRowName(holder, ["holder_name", "open_name", "display_name"]);
    const topCounterpartyName = diagnosticCounterpartyName(first);
    const sharedTopSubjectFinding = topAccountHolderName && topHolderName && topCounterpartyName
      && topAccountHolderName === topHolderName && topAccountHolderName === topCounterpartyName
        ? {
            status: "needs_review",
            top_account_holder_name: topAccountHolderName,
            top_holder_name: topHolderName,
            top_counterparty_name: topCounterpartyName,
            shared_top_subject: true,
            blank_counterparty_review_required: true
          }
        : {
            status: "needs_review",
            top_account_holder_name: topAccountHolderName || undefined,
            top_holder_name: topHolderName || undefined,
            top_counterparty_name: topCounterpartyName || undefined,
            shared_top_subject: false,
            blank_counterparty_review_required: true
          };
    const metricFacts = [
      diagnosticMetricRankFact("turnover", accountRank),
      diagnosticMetricRankFact("inflow", accountInflowRank),
      diagnosticMetricRankFact("outflow", accountOutflowRank),
      diagnosticMetricRankFact("txn_count", accountTxnCountRank),
      diagnosticMetricRankFact("max_single_amount", accountMaxSingleRank)
    ];
    const calls = [coverageResult, dataQualityResult, accountRank, holderRank, counterpartyRank, accountInflowRank, accountOutflowRank, accountTxnCountRank, accountMaxSingleRank];
    return {
      card_type: "top_rankings_card",
      case_id: caseId,
      intent: "top_rankings",
      title: "全案 Top 排名核验材料",
      fact_source: "production_backend",
      coverage,
      import_lineage_summary: importSummary,
      top_accounts: arrayOf(accountRank.data.rankings),
      top_holders: arrayOf(holderRank.data.rankings),
      top_counterparties: counterparties,
      metric_rank_facts: metricFacts,
      investigative_findings: [sharedTopSubjectFinding],
      facts: [
        makeDiagnosticFact({
          factId: "coverage.directed_transactions",
          label: "全案覆盖范围",
          count: coverage.directed_transaction_rows ?? coverage.txn_analyzed ?? coverage.txn_count,
          amount: coverage.turnover_yuan ?? coverage.turnover_total,
          unit: "rows/yuan",
          timeRange: "本轮可核验数据范围",
          answerText: `规范/可分析交易 ${coverage.transaction_rows ?? coverage.txn_total ?? coverage.txn_analyzed ?? "未知"} 条，账户维度覆盖 ${coverage.account_count ?? "未知"} 个，其中具备进/出方向 ${coverage.directed_transaction_rows ?? coverage.txn_analyzed ?? "未知"} 条，方向缺失 ${coverage.undirected_transaction_rows ?? "未知"} 条，全案资金流量 ${moneyText(coverage.turnover_yuan ?? coverage.turnover_total)}。`,
          supportQueryName: "get_scope_coverage",
          sourcePayload: coverageResult
        }),
        makeDiagnosticFact({
          factId: "source_audit.import_lineage",
          label: "来源审计与规范入库",
          count: importSummary.rows_imported_norm ?? coverage.txn_analyzed,
          amount: importSummary.rows_total,
          unit: "rows",
          timeRange: "导入至分析索引范围",
          answerText: `导入与规范化情况：导入台账 ${importSummary.file_log_count ?? "未知"} 个，原始行 ${importSummary.rows_total ?? "未知"}，规范入库 ${importSummary.rows_imported_norm ?? "未知"}，导入阶段去重/跳过重复 ${importSummary.rows_dedup ?? "未知"}，错误 ${importSummary.rows_error ?? "未知"}；Top 排名以规范明细/分析索引为准，原始行或去重候选不直接作为资金统计。`,
          supportQueryName: "audit_case_data_quality.import_lineage",
          sourcePayload: dataQualityResult
        }),
        ...metricFacts,
        makeDiagnosticFact({
          factId: "top_account.turnover.rank1",
          label: "账户 turnover 第一",
          account: diagnosticRowName(account, ["account_key", "acct_display", "account", "card_no", "acct_no"]),
          holder: diagnosticRowName(account, ["holder_name", "account_open_name", "open_name", "display_name"]),
          idNo: diagnosticRowName(account, ["id_no", "holder_id_no", "idNo"]),
          count: account.txn_count ?? account.count,
          amount: amountFromDiagnosticRow(account, "turnover"),
          unit: "yuan",
          timeRange: diagnosticRowTimeRange(account),
          answerText: `账户第一为 ${diagnosticRowName(account, ["account_key", "acct_display", "account"]) || "未知"}，户名 ${diagnosticRowName(account, ["holder_name", "account_open_name", "open_name", "display_name"]) || "未知"}，证件号 ${diagnosticRowName(account, ["id_no", "holder_id_no", "idNo"]) || "未登记"}，交易 ${account.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromDiagnosticRow(account, "turnover"))}。`,
          supportQueryName: "rank_accounts.turnover",
          sourcePayload: accountRank
        }),
        makeDiagnosticFact({
          factId: "top_holder.turnover.rank1",
          label: "户名主体 turnover 第一",
          holder: diagnosticRowName(holder, ["holder_name", "open_name", "display_name"]),
          idNo: diagnosticRowName(holder, ["id_no", "holder_id_no", "idNo"]),
          count: holder.txn_count ?? holder.count,
          amount: amountFromDiagnosticRow(holder, "turnover"),
          unit: "yuan",
          timeRange: diagnosticRowTimeRange(holder),
          answerText: `主体第一为 ${diagnosticRowName(holder, ["holder_name", "open_name", "display_name"]) || "未知"} / ${diagnosticRowName(holder, ["id_no", "holder_id_no", "idNo"]) || "未登记证件"}，open_name + id_no（户名+证件号）口径，账户 ${holder.account_count ?? "未知"} 个，交易 ${holder.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromDiagnosticRow(holder, "turnover"))}。`,
          supportQueryName: "rank_holders.turnover",
          sourcePayload: holderRank
        }),
        makeDiagnosticFact({
          factId: "top_counterparty.turnover.rank1",
          label: "对手方 turnover 第一",
          counterparty: diagnosticCounterpartyName(first),
          count: first.txn_count ?? first.count,
          amount: amountFromDiagnosticRow(first, "turnover"),
          unit: "yuan",
          timeRange: diagnosticRowTimeRange(first),
          answerText: `对手方第一为 ${diagnosticCounterpartyName(first) || "未知"}，具备方向交易 ${first.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromDiagnosticRow(first, "turnover"))}。`,
          supportQueryName: "rank_counterparties.turnover",
          sourcePayload: counterpartyRank
        }),
        makeDiagnosticFact({
          factId: "blank_counterparty.both_blank.rank2",
          label: "双空对手方 turnover 第二",
          counterparty: diagnosticCounterpartyName(second),
          count: second.txn_count ?? second.count,
          amount: amountFromDiagnosticRow(second, "turnover"),
          unit: "yuan",
          timeRange: diagnosticRowTimeRange(second),
          answerText: `第二对手方为 ${diagnosticCounterpartyName(second) || "未知"}；报告口径按 counterparty_name 和 counterparty_acct_norm 均为空统计，具备方向交易 ${second.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromDiagnosticRow(second, "turnover"))}。`,
          supportQueryName: "rank_counterparties.turnover",
          sourcePayload: counterpartyRank
        }),
        makeDiagnosticFact({
          factId: "top_counterparty.turnover.rank3",
          label: "对手方 turnover 第三",
          counterparty: diagnosticCounterpartyName(third),
          count: third.txn_count ?? third.count,
          amount: amountFromDiagnosticRow(third, "turnover"),
          unit: "yuan",
          answerText: `第三对手方为 ${diagnosticCounterpartyName(third) || "未知"}，具备方向交易 ${third.txn_count ?? "未知"} 笔，资金流量 ${moneyText(amountFromDiagnosticRow(third, "turnover"))}。`,
          supportQueryName: "rank_counterparties.turnover",
          sourcePayload: counterpartyRank
        })
      ],
      warnings: [
        "户名主体排名必须说明 open_name + id_no 口径；只按 open_name 会把多证照同名企业合并。",
        "空户名 Top 只指 counterparty_name 和 counterparty_acct_norm 均为空；不能把户名空但账号不空的记录盲并入双空对手方。",
        "若按入账、出账、笔数或最大单笔排序，Top 结果需另行计算，不得复用 turnover Top。",
        "Top 排名需区分统计结论、异常特征和待核线索；同事实/换卡/补卡候选只作为下一步补调或复核方向。",
        ...diagnosticSkillWarnings(calls)
      ],
      unsupported_claims: [
        "不能只按 open_name 合并为报告级主体口径；报告级主体必须使用 open_name + id_no。",
        "不能把方向缺失交易混入进/出方向统计。",
        "不能把所有户名空记录写成双空对手方；双空必须同时缺 counterparty_name 和 counterparty_acct_norm。"
      ],
      answer_constraints: [
        `当前案件编号 ${caseId}；首段先给 Top 结论，再补覆盖范围和排序说明。`,
        "建议按覆盖范围、Top 账户、Top 户名主体、Top 对手方、排序差异和待核事项组织。",
        "先说明覆盖范围，再分别列 Top 账户、Top 户名主体、Top 对手方，避免把不同排序结果混成一个口径。",
        `账户维度覆盖数为 ${coverage.account_count ?? "get_scope_coverage 未返回"} 个。`,
        `导入与规范化数字：原始行 ${importSummary.rows_total ?? "未知"}、规范入库 ${importSummary.rows_imported_norm ?? "未知"}、导入阶段去重/跳过重复 ${importSummary.rows_dedup ?? "未知"}。`,
        `户名主体组为 open_name + id_no（户名+证件号）/未登记账户口径 ${holderSummary.holder_count ?? "rank_holders 未返回"} 组；仅按 open_name 合并不能作为报告级主体口径。`,
        "若用户只问最大账户，可用一句话补充 Top 户名主体账户数、交易笔数、资金流量和双空对手方金额，帮助判断主体口径和数据质量。",
        "排序指标为往来总额；入账、出账、笔数、最大单笔是不同排序口径，改问时需要重算。",
        "来源边界可短写为：统计基于规范明细/分析索引；同事实、换卡/补卡候选和需补证事项不能混入确定金额。",
        "普通问答拿到本卡即可作答，不要再次扩大 rank 工具。"
      ]
    };
  }

  return { buildTopRankingsDiagnosticCard };
}
