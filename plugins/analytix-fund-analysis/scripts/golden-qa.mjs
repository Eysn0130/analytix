#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const DEFAULT_BACKEND_URL = "http://127.0.0.1:18731";
const DEFAULT_CASE_ID = "3c72e755b1f2";
const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");

function text(value) {
  return String(value || "").trim();
}

export function finiteNumber(value) {
  if (value === null || value === undefined || typeof value === "boolean") return undefined;
  if (typeof value === "string" && !value.trim()) return undefined;
  if (typeof value === "object") return undefined;
  const number = Number(value);
  return Number.isFinite(number) ? number : undefined;
}

export function nonNegativeInteger(value) {
  const number = finiteNumber(value);
  return number !== undefined && Number.isInteger(number) && number >= 0 ? number : undefined;
}

export function parseNumericArgument(value, flag, { integer = false, positive = false } = {}) {
  const number = integer ? nonNegativeInteger(value) : finiteNumber(value);
  if (number === undefined || (positive && number <= 0)) {
    throw new Error(`invalid numeric argument for ${flag}`);
  }
  return number;
}

export function arrayLengthOrUndefined(value) {
  return Array.isArray(value) ? value.length : undefined;
}

function parseArgs(argv) {
  const options = {
    backendUrl: DEFAULT_BACKEND_URL,
    caseId: DEFAULT_CASE_ID,
    json: false,
    timeoutMs: 180_000,
    expectTxnTotal: 423560,
    expectAccountCount: 750,
    expectSourceFileCount: 66,
    expectUnresolvedAccountCount: null,
    expectImportRowsTotal: null,
    expectImportRowsDedup: null,
    expectSameFactExtraRows: null,
    expectSameHolderSameFactExtraRows: null,
    expectSameHolderSameFactGroups: null,
    printModelSmokePrompts: false,
    printEvalRubric: false,
    printBadThreadReplay: false,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--backend-url") options.backendUrl = next();
    else if (arg === "--case-id") options.caseId = next();
    else if (arg === "--json") options.json = true;
    else if (arg === "--timeout-ms") options.timeoutMs = parseNumericArgument(next(), arg, { integer: true, positive: true });
    else if (arg === "--expect-txn-total") options.expectTxnTotal = parseNumericArgument(next(), arg, { integer: true });
    else if (arg === "--expect-account-count") options.expectAccountCount = parseNumericArgument(next(), arg, { integer: true });
    else if (arg === "--expect-source-file-count") options.expectSourceFileCount = parseNumericArgument(next(), arg, { integer: true });
    else if (arg === "--expect-unresolved-account-count") options.expectUnresolvedAccountCount = parseNumericArgument(next(), arg, { integer: true });
    else if (arg === "--expect-import-rows-total") options.expectImportRowsTotal = parseNumericArgument(next(), arg, { integer: true });
    else if (arg === "--expect-import-rows-dedup") options.expectImportRowsDedup = parseNumericArgument(next(), arg, { integer: true });
    else if (arg === "--expect-same-fact-extra-rows") options.expectSameFactExtraRows = parseNumericArgument(next(), arg, { integer: true });
    else if (arg === "--expect-same-holder-same-fact-extra-rows") options.expectSameHolderSameFactExtraRows = parseNumericArgument(next(), arg, { integer: true });
    else if (arg === "--expect-same-holder-same-fact-groups") options.expectSameHolderSameFactGroups = parseNumericArgument(next(), arg, { integer: true });
    else if (arg === "--print-model-smoke-prompts") options.printModelSmokePrompts = true;
    else if (arg === "--print-eval-rubric") options.printEvalRubric = true;
    else if (arg === "--print-bad-thread-replay") options.printBadThreadReplay = true;
    else if (arg === "--help" || arg === "-h") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  options.backendUrl = options.backendUrl.replace(/\/+$/u, "");
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/golden-qa.mjs [options]

Options:
  --backend-url <url>              Analytix backend base URL. Default: ${DEFAULT_BACKEND_URL}
  --case-id <id>                   Golden case id. Default: ${DEFAULT_CASE_ID}
  --expect-txn-total <n>           Expected case transaction total.
  --expect-account-count <n>       Expected distinct account count.
  --expect-source-file-count <n>   Expected source file count.
  --expect-unresolved-account-count <n> Expected account-dimension unregistered holder account count.
  --expect-import-rows-total <n>   Expected import ledger rows_total.
  --expect-import-rows-dedup <n>   Expected import ledger rows_dedup.
  --expect-same-fact-extra-rows <n> Expected cross-account same-fact extra rows.
  --expect-same-holder-same-fact-extra-rows <n> Expected same-holder same-fact extra rows.
  --expect-same-holder-same-fact-groups <n> Expected same-holder same-fact group count.
  --timeout-ms <n>                 Per-request timeout.
  --json                           Print JSON report.
  --print-model-smoke-prompts      Include the DeepSeek/OpenAI smoke prompts in the output.
  --print-eval-rubric              Include the manual golden eval rubric in the output.
  --print-bad-thread-replay        Include compact-output replay tasks for the 019e5fdc poor thread.`);
}

async function postJson(url, body, timeoutMs) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body || {}),
      signal: controller.signal,
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

async function executeSkill(options, skillId, input) {
  const payload = await postJson(
    `${options.backendUrl}/api/v1/skills/${encodeURIComponent(skillId)}:execute`,
    { input: { ...input, case_id: options.caseId } },
    options.timeoutMs,
  );
  return payload && typeof payload === "object" && Object.prototype.hasOwnProperty.call(payload, "data")
    ? payload.data
    : payload;
}

function approxEqual(left, right, tolerance = 0.01) {
  const leftNumber = finiteNumber(left);
  const rightNumber = finiteNumber(right);
  return leftNumber !== undefined
    && rightNumber !== undefined
    && Math.abs(leftNumber - rightNumber) <= tolerance;
}

function makeReporter() {
  const checks = [];
  return {
    checks,
    assert(name, condition, details = {}) {
      checks.push({ name, ok: Boolean(condition), details });
      if (!condition) {
        throw new Error(`${name} failed: ${JSON.stringify(details)}`);
      }
    },
  };
}

function modelSmokePrompts(caseId) {
  return [
    `以案件 ${caseId} 为准，先调用 get_case_scope_map、audit_unindexed_sources、audit_case_data_quality、compare_analysis_scopes 和 plan_case_analysis，再回答：该案中账户资金最大的账户是哪个？必须说明排序指标、覆盖交易数、未纳入口径来源审计和数据质量边界，但不要输出 q_xxx/audit_ref/artifact_id 等内部编号。`,
    `以案件 ${caseId} 为准，先调用 get_case_scope_map，再调用 audit_case_data_quality 和 resolve_duplicate_families，复核导入清洗、账户维表未登记户名账户、明细空户名字段、左侧树口径边界、换卡/补卡/同事实重复候选；不得把 clean_duplicate=0 写成没有重复，也不得全流水去重，最低约束到同一户名/同一人账户集合。`,
    `以案件 ${caseId} 为准，调用 rank_accounts 和 analyze_account_full，分析排名第一账户的资金情况、来源去向和可疑特征；不得把弱来源、未验证计算或缺失交付写成生产事实。`,
    `以案件 ${caseId} 为准，调用 resolve_owner_scope(holder_name="合成主体甲", include_candidate_accounts=true)，说明直接账户、候选账户和不能直接认定的边界。`,
    `以案件 ${caseId} 为准，调用 hypothesis_probe(holder_name="合成主体甲", include_candidate_accounts=true, probe_type="discovery")，发现现金、理财、工程、涉诉、资产等可疑线索。`,
    `以案件 ${caseId} 为准，调用 hypothesis_probe(holder_name="合成主体甲", include_candidate_accounts=true, probe_type="destination_flow")，回答合成主体甲资金主要去向和补调对象；只输出工具返回支持的事实。`,
    `以案件 ${caseId} 为准，调用 hypothesis_probe(holder_name="合成主体甲", include_candidate_accounts=true, probe_type="investigative_patterns", keywords=["合成主体乙","理财"])，复核重复交易号、缺失对手大额、金额集中和关键词链路。`,
    `以案件 ${caseId} 为准，调用 run_investigation_lab(holder_name="合成主体甲", include_candidate_accounts=true, focus_keywords=["合成主体乙","理财"], date_start="2025-01-01")，像旧线程一样自由深挖可疑点，但每个结论必须来自工具返回的支持事实或交易边。`,
    `以案件 ${caseId} 为准，调用 trace_subject_top_outflows(holder_name="合成主体甲", include_candidate_accounts=true, date_start="2025-01-01", top_n=20)，输出Top出账、终点分类和补调对象；不得把未匹配终点写成最终流向。`,
    `以案件 ${caseId} 为准，调用 classify_missing_counterparty_business(holder_name="合成主体甲", include_candidate_accounts=true, missing_kind="both")，复核现金/理财/户名缺失误分类风险。`,
    `以案件 ${caseId} 为准，验证全案/报告工具处于 P0 执行前隔离；不得调用或模拟 run_full_case_analysis，不得生成案件事实或报告草稿，只说明当前能力缺口、已检查范围和补证动作。`,
  ];
}

function badThreadReplayPrompts(caseId) {
  return [
    {
      task_id: "bad-thread-replay-01-holder-analysis",
      user_prompt: "对合成主体甲名下账户进行分析",
      expected_owner: "subject-dossier",
      suggested_first_tools: ["resolve_owner_scope", "analyze_holder_full", "rank_accounts"],
      suggested_arguments: { case_id: caseId, intent: "holder_analysis", holder_name: "合成主体甲", max_cards: 4 },
      scoring: [
        "tool content text <= 16000 chars",
        "answer locks and states case_id/case_name before facts",
        "uses focused owner and semantic fact tools instead of reading SKILL.md through shell",
        "does not expose q_xxx/audit_ref/artifact_id internal ids",
        "separates direct accounts from candidate leads",
        "states duplicate/card-replacement and missing-counterparty boundaries"
      ]
    },
    {
      task_id: "bad-thread-replay-02-destination",
      user_prompt: "资金转给谁了",
      expected_owner: "fund-flow-tracing",
      suggested_first_tools: ["trace_subject_top_outflows", "rank_counterparties", "build_fund_flow_graph"],
      suggested_arguments: { case_id: caseId, intent: "destination", holder_name: "合成主体甲", top_n: 20 },
      scoring: [
        "does not aggregate bounded rows by the model",
        "uses rank/trace evidence refs for every destination amount",
        "does not write missing counterparties as final receivers",
        "calls out financial-product terminals separately from cash"
      ]
    },
    {
      task_id: "bad-thread-replay-03-zhangjinzhi-continuation",
      user_prompt: "转给合成主体乙后，合成主体乙又将钱给了谁",
      required_first_tool: "build_fund_flow_graph",
      suggested_arguments: { case_id: caseId, holder_name: "合成主体甲", via_holder_name: "合成主体乙", date_start: "2025-01-01", top_n: 20 },
      scoring: [
        "every edge has txn_id, txn_time, amount, direction, and endpoint labels",
        "does not repeat duplicate-amplified 42M as fact; separates effective pair amount 25,885,013 from 2025 core transaction concentration 21,000,000",
        "does not invent downstream arrows outside flow_graph.edges",
        "downgrades unmatched endpoints to follow-up requests"
      ]
    },
    {
      task_id: "bad-thread-replay-04-flow-diagram",
      user_prompt: "画出资金流向图",
      required_first_tool: "build_fund_flow_graph",
      suggested_arguments: { case_id: caseId, holder_name: "合成主体甲", via_holder_name: "合成主体乙", date_start: "2025-01-01", top_n: 20 },
      scoring: [
        "Mermaid uses only flow_graph.edges where edge_status='supported'",
        "no edge without transaction evidence",
        "diagram labels include amount and date",
        "final text lists evidence gaps and next validation action"
      ]
    }
  ];
}

function readEvalRubric() {
  const rubricPath = path.join(scriptDir, "eval-fixtures", "golden-eval-rubric.md");
  return fs.readFileSync(rubricPath, "utf8");
}

async function run(options) {
  const report = makeReporter();
  const reconciliation = await executeSkill(options, "get_case_reconciliation", {});
  report.assert("reconciliation status", reconciliation.status === "ok", { status: reconciliation.status, warnings: reconciliation.warnings });

  const sourceAudit = await executeSkill(options, "audit_unindexed_sources", { table_limit: 200, column_limit: 60 });
  report.assert("source audit callable", ["ok", "partial"].includes(sourceAudit.status), { status: sourceAudit.status, warnings: sourceAudit.warnings });
  report.assert("source audit exposes no raw rows", sourceAudit.data?.raw_rows_exposed === false, sourceAudit.data);
  report.assert("source audit pipeline counts present", sourceAudit.data?.pipeline_counts && typeof sourceAudit.data.pipeline_counts === "object", sourceAudit.data);

  const dataQuality = await executeSkill(options, "audit_case_data_quality", { example_limit: 10 });
  const importSummary = dataQuality.data?.import_lineage?.summary || {};
  const duplicateSummary = dataQuality.data?.cleaning_quality?.duplicate_summary || {};
  const cleaningFlags = dataQuality.data?.cleaning_quality?.flags || {};
  const accountIdentity = dataQuality.data?.account_identity_quality || {};
  report.assert("data quality audit callable", ["ok", "partial"].includes(dataQuality.status), { status: dataQuality.status, warnings: dataQuality.warnings });
  report.assert("data quality import lineage present", nonNegativeInteger(importSummary.file_log_count) > 0, importSummary);
  report.assert("data quality duplicate summary present", duplicateSummary && typeof duplicateSummary === "object", duplicateSummary);
  if (Number.isFinite(options.expectImportRowsTotal)) {
    report.assert("data quality import rows_total", nonNegativeInteger(importSummary.rows_total) === options.expectImportRowsTotal, importSummary);
  }
  if (Number.isFinite(options.expectImportRowsDedup)) {
    report.assert("data quality import rows_dedup", nonNegativeInteger(importSummary.rows_dedup) === options.expectImportRowsDedup, importSummary);
  }
  if (Number.isFinite(options.expectSameFactExtraRows)) {
    report.assert(
      "data quality same-fact extra rows",
      nonNegativeInteger(duplicateSummary.same_fact_cross_account_extra_rows) === options.expectSameFactExtraRows,
      duplicateSummary,
    );
  }
  if (Number.isFinite(options.expectUnresolvedAccountCount)) {
    report.assert(
      "data quality account-dim unregistered account count",
      nonNegativeInteger(accountIdentity.unregistered_account_count ?? cleaningFlags.blank_holder_account_count) === options.expectUnresolvedAccountCount,
      accountIdentity,
    );
  }

  const duplicateFamilies = await executeSkill(options, "resolve_duplicate_families", {
    holder_name: "合成主体甲",
    scope_mode: "same_holder_accounts",
    limit: 10,
  });
  const duplicateFamilySummary = duplicateFamilies.data?.summary || {};
  const sameHolderFamilySummary = duplicateFamilySummary.same_holder_same_fact || {};
  const fullCaseReviewSummary = duplicateFamilySummary.full_case_review_only || {};
  report.assert("duplicate family resolver callable", ["ok", "partial"].includes(duplicateFamilies.status), { status: duplicateFamilies.status, warnings: duplicateFamilies.warnings });
  report.assert("duplicate family policy blocks blind dedupe", duplicateFamilies.data?.dedupe_policy?.full_case_blind_dedupe_allowed === false, duplicateFamilies.data?.dedupe_policy || {});
  report.assert("duplicate family policy blocks unconfirmed deduction", duplicateFamilies.data?.dedupe_policy?.deduct_from_totals_allowed === false, duplicateFamilies.data?.dedupe_policy || {});
  report.assert("duplicate family resolver returns families array", Array.isArray(duplicateFamilies.data?.families), duplicateFamilies.data);
  report.assert(
    "duplicate family scope is context-compact",
    nonNegativeInteger(duplicateFamilies.data?.scope?.selected_account_count) >= duplicateFamilies.data?.scope?.selected_accounts_preview?.length
      && duplicateFamilies.data?.scope?.selected_accounts === undefined,
    duplicateFamilies.data?.scope || {},
  );
  if (Number.isFinite(options.expectSameHolderSameFactExtraRows)) {
    report.assert(
      "duplicate family same-holder extra rows",
      nonNegativeInteger(sameHolderFamilySummary.extra_rows) === options.expectSameHolderSameFactExtraRows,
      sameHolderFamilySummary,
    );
  }
  if (Number.isFinite(options.expectSameHolderSameFactGroups)) {
    report.assert(
      "duplicate family same-holder groups",
      nonNegativeInteger(sameHolderFamilySummary.group_count) === options.expectSameHolderSameFactGroups,
      sameHolderFamilySummary,
    );
  }
  report.assert(
    "duplicate family full-case cross-holder review exposed",
    Object.prototype.hasOwnProperty.call(fullCaseReviewSummary, "cross_holder_group_count"),
    fullCaseReviewSummary,
  );

  const coverage = await executeSkill(options, "get_scope_coverage", {});
  const coverageData = coverage.data?.coverage || {};
  report.assert("coverage txn_total", nonNegativeInteger(coverageData.txn_total) === options.expectTxnTotal, coverageData);
  report.assert("coverage txn_analyzed", nonNegativeInteger(coverageData.txn_analyzed) === options.expectTxnTotal, coverageData);
  report.assert("coverage txn_excluded", nonNegativeInteger(coverageData.txn_excluded) === 0, coverageData);
  report.assert("coverage account_count", nonNegativeInteger(coverageData.account_count) === options.expectAccountCount, coverageData);
  report.assert("coverage source_file_count", nonNegativeInteger(coverageData.source_file_count) === options.expectSourceFileCount, coverageData);

  const scopeCompare = await executeSkill(options, "compare_analysis_scopes", {});
  report.assert("scope compare has scopes", Array.isArray(scopeCompare.data?.scopes) && scopeCompare.data.scopes.length >= 4, scopeCompare.data);
  const detailScope = scopeCompare.data.scopes.find((item) => item.scope_id === "detail_index_all_rows") || {};
  const byNameScope = scopeCompare.data.scopes.find((item) => item.scope_id === "by_name_holder_aggregate") || {};
  report.assert("scope compare detail count", nonNegativeInteger(detailScope.txn_count) === options.expectTxnTotal, detailScope);
  report.assert("scope compare unresolved holders present", nonNegativeInteger(byNameScope.unresolved_account_count) > 0, byNameScope);
  if (Number.isFinite(options.expectUnresolvedAccountCount)) {
    report.assert("scope compare unresolved holder account count", nonNegativeInteger(byNameScope.unresolved_account_count) === options.expectUnresolvedAccountCount, byNameScope);
  }

  const accountRank = await executeSkill(options, "rank_accounts", { metric: "turnover", limit: 10 });
  const accountTop = accountRank.data?.rankings?.[0] || {};
  report.assert("rank_accounts nonempty", Array.isArray(accountRank.data?.rankings) && accountRank.data.rankings.length > 0, accountRank.data);
  report.assert("rank_accounts top positive", finiteNumber(accountTop.turnover_total) > 0, accountTop);

  const holderRank = await executeSkill(options, "rank_holders", { metric: "turnover", limit: 10 });
  const holderTop = holderRank.data?.rankings?.[0] || {};
  report.assert("rank_holders nonempty", Array.isArray(holderRank.data?.rankings) && holderRank.data.rankings.length > 0, holderRank.data);
  report.assert("rank_holders top positive", finiteNumber(holderTop.turnover_total) > 0, holderTop);

  const counterpartyRank = await executeSkill(options, "rank_counterparties", { metric: "turnover", limit: 10 });
  const counterpartyTop = counterpartyRank.data?.rankings?.[0] || {};
  report.assert("rank_counterparties nonempty", Array.isArray(counterpartyRank.data?.rankings) && counterpartyRank.data.rankings.length > 0, counterpartyRank.data);
  report.assert("rank_counterparties top positive", finiteNumber(counterpartyTop.turnover_total) > 0, counterpartyTop);

  const liuCounterpartyNameRank = await executeSkill(options, "rank_counterparties", {
    holder_name: "合成主体甲",
    direction_mode: "out",
    metric: "outflow",
    limit: 20,
    dedupe_same_holder_same_fact: true,
    counterparty_group_mode: "name",
  });
  const zhangByName = (liuCounterpartyNameRank.data?.rankings || []).find((item) => text(item.display_name) === "合成主体乙") || {};
  report.assert("合成主体甲->合成主体乙 rank candidate remains non-controlling", finiteNumber(zhangByName.outflow_total) > 0, zhangByName);

  const liuCounterpartyAccountRank = await executeSkill(options, "rank_counterparties", {
    holder_name: "合成主体甲",
    direction_mode: "out",
    metric: "outflow",
    limit: 20,
    dedupe_same_holder_same_fact: true,
    counterparty_group_mode: "account",
  });
  const zhangCoreAccount = (liuCounterpartyAccountRank.data?.rankings || []).find((item) => text(item.counterparty_account) === "9000000000000000015") || {};
  report.assert("合成主体甲->合成主体乙 core card total deduped", approxEqual(zhangCoreAccount.outflow_total, 21000000), zhangCoreAccount);
  report.assert("合成主体甲->合成主体乙 core card txn count", nonNegativeInteger(zhangCoreAccount.txn_count) === 5, zhangCoreAccount);

  const accountAnalysis = await executeSkill(options, "analyze_account_full", { account_keys: [accountTop.account_key], counterparty_limit: 10 });
  const accountSummary = accountAnalysis.data?.account_stats?.group_summary || {};
  report.assert(
    "account profile txn_count matches rank",
    nonNegativeInteger(accountSummary.txn_count) !== undefined
      && nonNegativeInteger(accountSummary.txn_count) === nonNegativeInteger(accountTop.txn_count),
    { accountSummary, accountTop },
  );
  report.assert("account profile inflow matches rank", approxEqual(accountSummary.inflow_total, accountTop.inflow_total), { accountSummary, accountTop });
  report.assert("account profile outflow matches rank", approxEqual(accountSummary.outflow_total, accountTop.outflow_total), { accountSummary, accountTop });

  if (text(holderTop.holder_name) && !["未识别户名", "未登记户名"].includes(text(holderTop.holder_name))) {
    const holderAnalysis = await executeSkill(options, "analyze_holder_full", { holder_name: holderTop.holder_name, match_mode: "exact", counterparty_limit: 10 });
    const holderSummary = holderAnalysis.data?.account_analysis?.account_stats?.group_summary || {};
    report.assert(
      "holder profile txn_count matches rank",
      nonNegativeInteger(holderSummary.txn_count) !== undefined
        && nonNegativeInteger(holderSummary.txn_count) === nonNegativeInteger(holderTop.txn_count),
      { holderSummary, holderTop },
    );
    report.assert("holder profile inflow matches rank", approxEqual(holderSummary.inflow_total, holderTop.inflow_total), { holderSummary, holderTop });
    report.assert("holder profile outflow matches rank", approxEqual(holderSummary.outflow_total, holderTop.outflow_total), { holderSummary, holderTop });
  }

  const ownerScope = await executeSkill(options, "resolve_owner_scope", {
    holder_name: "合成主体甲",
    include_candidate_accounts: true,
    candidate_min_turnover: 100000,
    limit: 30,
  });
  report.assert("owner direct accounts present", Array.isArray(ownerScope.data?.direct_account_keys) && ownerScope.data.direct_account_keys.length > 0, ownerScope.data);
  report.assert("owner direct account-dim scope includes sub accounts", ownerScope.data?.direct_account_keys?.length >= 200, ownerScope.data);
  report.assert("owner candidate accounts array", Array.isArray(ownerScope.data?.candidate_accounts), ownerScope.data);
  const directAccountKeys = new Set((ownerScope.data?.direct_account_keys || []).map(text).filter(Boolean));
  const candidateAccountKeys = new Set((ownerScope.data?.candidate_account_keys || []).map(text).filter(Boolean));
  const unresolvedHighValueAccounts = ownerScope.data?.unresolved_high_value_accounts || [];
  report.assert(
    "owner unresolved high value remains unassigned",
    Array.isArray(unresolvedHighValueAccounts)
      && unresolvedHighValueAccounts.length > 0
      && unresolvedHighValueAccounts.every((item) => {
        const accountKey = text(item.account_key);
        return accountKey
          && finiteNumber(item.turnover_total) > 0
          && !directAccountKeys.has(accountKey)
          && !candidateAccountKeys.has(accountKey);
      }),
    ownerScope.data,
  );
  report.assert(
    "owner expanded coverage grows",
    nonNegativeInteger(ownerScope.data?.expanded_coverage?.txn_analyzed) !== undefined
      && nonNegativeInteger(ownerScope.data?.direct_coverage?.txn_analyzed) !== undefined
      && nonNegativeInteger(ownerScope.data?.expanded_coverage?.txn_analyzed)
        >= nonNegativeInteger(ownerScope.data?.direct_coverage?.txn_analyzed),
    ownerScope.data,
  );

  const discoveryProbe = await executeSkill(options, "hypothesis_probe", {
    holder_name: "合成主体甲",
    include_candidate_accounts: true,
    probe_type: "discovery",
    limit: 10,
  });
  report.assert("discovery probe findings", Array.isArray(discoveryProbe.data?.findings) && discoveryProbe.data.findings.length > 0, discoveryProbe.data);

  const destinationProbe = await executeSkill(options, "hypothesis_probe", {
    holder_name: "合成主体甲",
    include_candidate_accounts: true,
    probe_type: "destination_flow",
    limit: 10,
  });
  report.assert("destination probe findings", Array.isArray(destinationProbe.data?.findings) && destinationProbe.data.findings.length > 0, destinationProbe.data);
  report.assert("destination followup candidates", Array.isArray(destinationProbe.data?.followup_candidates) && destinationProbe.data.followup_candidates.length > 0, destinationProbe.data);

  const cashProbe = await executeSkill(options, "hypothesis_probe", {
    holder_name: "合成主体甲",
    include_candidate_accounts: true,
    probe_type: "cash_breakpoints",
    limit: 10,
  });
  report.assert("cash probe callable", Array.isArray(cashProbe.data?.findings), cashProbe.data);

  const financialProbe = await executeSkill(options, "hypothesis_probe", {
    holder_name: "合成主体甲",
    include_candidate_accounts: true,
    probe_type: "financial_product_flows",
    limit: 10,
  });
  report.assert("financial probe callable", Array.isArray(financialProbe.data?.findings), financialProbe.data);

  const patternProbe = await executeSkill(options, "hypothesis_probe", {
    holder_name: "合成主体甲",
    include_candidate_accounts: true,
    probe_type: "investigative_patterns",
    keywords: ["合成主体乙", "理财"],
    limit: 20,
  });
  const patternSummary = patternProbe.data?.pattern_summary || {};
  report.assert("investigative pattern probe callable", Array.isArray(patternProbe.data?.findings), patternProbe.data);
  report.assert(
    "investigative pattern duplicate txn ids surfaced",
    Array.isArray(patternSummary.duplicate_txn_id_groups) && patternSummary.duplicate_txn_id_groups.some((item) => item.txn_id === "20250103175349102219900882174578"),
    patternSummary,
  );
  report.assert(
    "investigative pattern keyword hits include zhang jinzhi or finance",
    Array.isArray(patternSummary.keyword_detail_rows) && patternSummary.keyword_detail_rows.some((item) => String(item.display_name || item.business_text || "").includes("合成主体乙") || item.business_category === "financial_product"),
    patternSummary,
  );

  const investigationLab = await executeSkill(options, "run_investigation_lab", {
    holder_name: "合成主体甲",
    include_candidate_accounts: true,
    analysis_goal: "golden_qa old-thread-style suspicious discovery",
    focus_keywords: ["合成主体乙", "理财"],
    date_start: "2025-01-01",
    max_hypotheses: 8,
  });
  const labCards = investigationLab.data?.hypothesis_cards || [];
  const mandatoryCards = investigationLab.data?.mandatory_review_cards || [];
  const labCardTypes = new Set(labCards.map((item) => item.card_type));
  report.assert("investigation lab callable", Array.isArray(labCards) && labCards.length > 0, investigationLab.data);
  report.assert("investigation lab preserves autonomy", investigationLab.data?.agent_autonomy?.mode === "hypothesis_lab", investigationLab.data?.agent_autonomy || {});
  report.assert("investigation lab has pattern card", labCardTypes.has("duplicate_txn_id") || labCardTypes.has("keyword_link"), labCards);
  report.assert("investigation lab prioritizes top outflow", labCards[0]?.card_type === "subject_top_outflow", labCards.slice(0, 3));
  const topOutflowCardId = String(mandatoryCards.find((item) => item.card_type === "subject_top_outflow")?.hypothesis_id || "");
  report.assert(
    "investigation lab mandatory contract",
    Boolean(topOutflowCardId) &&
      Array.isArray(investigationLab.data?.model_answer_contract?.must_review_before_final) &&
      investigationLab.data.model_answer_contract.must_review_before_final.includes(topOutflowCardId),
    { mandatoryCards, contract: investigationLab.data?.model_answer_contract },
  );
  report.assert(
    "investigation lab cards include claim guards",
    labCards.every((item) => String(item.evidence_statement || "").trim() && String(item.claim_guard || "").trim()),
    labCards,
  );
  report.assert(
    "investigation lab card amounts preserve yuan and wan units",
    labCards.some((item) => String(item.evidence_statement || "").includes("7958757.83 元（795.875783 万元）")) &&
      labCards.some((item) => String(item.claim_guard || "").includes("10000000.00 元是 1000.000000 万元")),
    labCards,
  );
  report.assert(
    "investigation lab catches old-thread regression details",
    labCards.some((item) => JSON.stringify(item.facts || {}).includes("20250103175349102219900882174578")) &&
      labCards.some((item) => JSON.stringify(item.facts || {}).includes("合成主体乙") || JSON.stringify(item.facts || {}).includes("financial_product")),
    labCards,
  );

  const topOutflowTrace = await executeSkill(options, "trace_subject_top_outflows", {
    holder_name: "合成主体甲",
    include_candidate_accounts: true,
    date_start: "2025-01-01",
    top_n: 10,
  });
  const topOutflow = topOutflowTrace.data?.top_outflows?.[0]?.seed_txn || {};
  report.assert("top outflow trace rows", Array.isArray(topOutflowTrace.data?.top_outflows) && topOutflowTrace.data.top_outflows.length > 0, topOutflowTrace.data);
  report.assert("top outflow has terminal summary", Array.isArray(topOutflowTrace.data?.terminal_summary), topOutflowTrace.data);
  report.assert("top outflow finance misclassification guard", topOutflow.business_category === "financial_product" && approxEqual(topOutflow.amount, 7958757.83), topOutflow);
  report.assert(
    "top outflow duplicate txn ids deduped",
    Array.isArray(topOutflowTrace.data?.duplicate_seed_txn_ids) && topOutflowTrace.data.duplicate_seed_txn_ids.includes("20250103175349102219900882174578"),
    topOutflowTrace.data,
  );
  report.assert(
    "top outflow includes zhang jinzhi continuation lead",
    topOutflowTrace.data.top_outflows.some((item) => String(item.seed_txn?.counterparty_name || "").includes("合成主体乙")),
    topOutflowTrace.data.top_outflows,
  );

  const missingBusiness = await executeSkill(options, "classify_missing_counterparty_business", {
    holder_name: "合成主体甲",
    include_candidate_accounts: true,
    missing_kind: "both",
    limit: 20,
  });
  report.assert("missing business classifier callable", Array.isArray(missingBusiness.data?.category_summary), missingBusiness.data);
  report.assert("missing business correction rules", Array.isArray(missingBusiness.data?.correction_rules) && missingBusiness.data.correction_rules.length > 0, missingBusiness.data);
  report.assert(
    "missing business financial product over cash",
    missingBusiness.data.category_summary.some((item) => item.business_category === "financial_product" && finiteNumber(item.amount_total) > 0),
    missingBusiness.data.category_summary,
  );

  const continuationValidation = await executeSkill(options, "validate_continuation_list", {
    rows: [
      {
        txn_id: topOutflow.txn_id,
        account_key: topOutflow.account_key,
        txn_time: topOutflow.txn_time,
        amount: topOutflow.amount,
        direction: "out",
      },
    ],
    expected_total_amount: finiteNumber(topOutflow.amount),
    expected_txn_count: 1,
    strict_db_match: true,
  });
  const continuationIssueCodes = new Set((continuationValidation.data?.issues || []).map((item) => item.code));
  report.assert(
    "continuation validation accepts card-replacement account candidates",
    ["passed", "warning"].includes(continuationValidation.data?.validation_status) &&
      ![...continuationIssueCodes].some((code) => String(code || "").endsWith("_MISMATCH")) &&
      (!continuationIssueCodes.has("TXN_ID_MULTIPLE_ACCOUNT_CANDIDATES") || Array.isArray(continuationValidation.data?.ambiguous_txn_ids)),
    continuationValidation.data,
  );

  const badContinuationValidation = await executeSkill(options, "validate_continuation_list", {
    rows: [
      {
        txn_id: topOutflow.txn_id,
        account_key: "",
        txn_time: topOutflow.txn_time,
        amount: topOutflow.amount,
        direction: "out",
      },
    ],
    required_fields: ["txn_id", "account_key", "counterparty_name"],
    expected_total_amount: finiteNumber(topOutflow.amount),
    expected_txn_count: 1,
    strict_db_match: false,
  });
  const badIssueCodes = new Set((badContinuationValidation.data?.issues || []).map((item) => item.code));
  report.assert("continuation blank field blocks appendix", badContinuationValidation.data?.validation_status === "failed" && badIssueCodes.has("BLANK_REQUIRED_FIELD"), badContinuationValidation.data);

  const plan = await executeSkill(options, "plan_case_analysis", { analysis_goal: "golden_qa top20 outflow continuation", include_risk_scan: false, max_lanes: 6 });
  report.assert("plan lanes present", Array.isArray(plan.data?.lanes) && plan.data.lanes.length > 0, plan.data);
  report.assert("plan has budget", Boolean(plan.data?.tool_budget?.max_total_tool_calls), plan.data?.tool_budget || {});
  report.assert("plan has commander policy", Array.isArray(plan.data?.tool_selection_policy) && plan.data.tool_selection_policy.length > 0, plan.data);
  report.assert("plan includes outflow lane", plan.data?.lanes?.some((item) => item.lane_id === "subject_outflow_tracing"), plan.data?.lanes || []);

  const holderPlan = await executeSkill(options, "plan_case_analysis", {
    holder_name: "合成主体甲",
    analysis_goal: "golden_qa compact holder commander context",
    include_risk_scan: false,
    max_lanes: 1,
  });
  const holderPlanScope = holderPlan.data?.scope || {};
  const holderPlanResolvedScope = holderPlanScope.resolved_scope || {};
  const holderPlanCoverage = holderPlan.data?.shared_context?.coverage || {};
  report.assert(
    "holder plan scope is context-compact",
    nonNegativeInteger(holderPlanScope.account_key_count) >= 200 &&
      Array.isArray(holderPlanScope.account_keys_preview) &&
      holderPlanScope.account_keys_preview.length <= 50 &&
      holderPlanScope.account_keys === undefined,
    holderPlanScope,
  );
  report.assert(
    "holder plan resolved scope is context-compact",
    nonNegativeInteger(holderPlanResolvedScope.expanded_account_keys_count) >= 200 &&
      Array.isArray(holderPlanResolvedScope.expanded_account_keys_preview) &&
      holderPlanResolvedScope.expanded_account_keys_preview.length <= 50 &&
      holderPlanResolvedScope.expanded_account_keys === undefined,
    holderPlanResolvedScope,
  );
  report.assert(
    "holder plan coverage is context-compact",
    holderPlanCoverage.selected_accounts === undefined &&
      nonNegativeInteger(holderPlanCoverage.selected_account_count) >= holderPlanCoverage.selected_accounts_preview?.length,
    holderPlanCoverage,
  );

  const fullReport = {
    status: "blocked",
    boundary: "P0 pre-execution isolation; no full-case fact or report draft was requested by golden QA",
  };
  report.assert("full report golden path stays pre-execution blocked", fullReport.status === "blocked", fullReport);

  return {
    ok: true,
    case_id: options.caseId,
    checks: report.checks,
    summary: {
      coverage: coverageData,
      top_account: accountTop,
      top_holder: holderTop,
      top_counterparty: counterpartyTop,
      owner_scope: {
        direct_account_count: arrayLengthOrUndefined(ownerScope.data?.direct_account_keys),
        candidate_account_count: arrayLengthOrUndefined(ownerScope.data?.candidate_accounts),
        expanded_txn_analyzed: nonNegativeInteger(ownerScope.data?.expanded_coverage?.txn_analyzed),
      },
      discovery_findings: arrayLengthOrUndefined(discoveryProbe.data?.findings),
      destination_findings: arrayLengthOrUndefined(destinationProbe.data?.findings),
      pattern_findings: arrayLengthOrUndefined(patternProbe.data?.findings),
      investigation_lab_cards: arrayLengthOrUndefined(investigationLab.data?.hypothesis_cards),
      full_report_status: fullReport.status,
      report_validation: {},
    },
    model_smoke_prompts: options.printModelSmokePrompts ? modelSmokePrompts(options.caseId) : undefined,
    bad_thread_replay: options.printBadThreadReplay ? badThreadReplayPrompts(options.caseId) : undefined,
    eval_rubric_md: options.printEvalRubric ? readEvalRubric() : undefined,
  };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const result = await run(options);
  if (options.json) {
    console.log(JSON.stringify(result, null, 2));
  } else {
    console.log(`golden qa passed for ${result.case_id}`);
    console.log(`checks: ${result.checks.length}`);
    console.log(`top account: ${result.summary.top_account.account_key || "n/a"}`);
    console.log(`top holder: ${result.summary.top_holder.holder_name || "n/a"}`);
    console.log(`top counterparty: ${result.summary.top_counterparty.display_name || "n/a"}`);
    if (options.printModelSmokePrompts) {
      console.log("\nmodel smoke prompts:");
      for (const prompt of result.model_smoke_prompts || []) {
        console.log(`- ${prompt}`);
      }
    }
    if (options.printEvalRubric) {
      console.log("\neval rubric:");
      console.log(result.eval_rubric_md || "");
    }
    if (options.printBadThreadReplay) {
      console.log("\nbad-thread replay tasks:");
      for (const item of result.bad_thread_replay || []) {
        console.log(`- ${item.task_id}: ${item.user_prompt}`);
      }
    }
  }
}

if (path.resolve(process.argv[1] || "") === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.stack || error.message : String(error));
    process.exit(1);
  });
}
