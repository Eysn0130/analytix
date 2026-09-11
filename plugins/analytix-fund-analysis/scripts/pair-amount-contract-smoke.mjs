#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { CaseSourceError } from "../mcp/backend-api-client.mjs";
import { createAgentOutputCompiler } from "../mcp/agent-output-compiler.mjs";
import { frontdoorWantsFlowGraph, inferFrontDoorIntent } from "../mcp/intent-plan-protocol.mjs";
import {
  inferSourceHolderFromQuestion,
  inferViaNameFromQuestion,
  isExplicitTransferAmountQuestion
} from "../mcp/frontdoor-routing.mjs";
import { tools as MCP_TOOL_SCHEMAS } from "../mcp/tool-schemas.mjs";
import { createToolCallRuntime } from "../mcp/tool-call-runtime.mjs";
import { userVisibleLeakageLabels } from "../mcp/user-facing-language.mjs";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const compiler = createAgentOutputCompiler({
  moneyText(value) {
    const number = Number(value);
    return Number.isFinite(number) ? `${number.toFixed(2)} 元` : "";
  }
});

const disputedPairQuestion = "王明转给李华多少钱？请复核重复、换卡和同事实风险。";
const disputedPairIntent = inferFrontDoorIntent({ intent: "auto", question: disputedPairQuestion });
const disputedPairVia = inferViaNameFromQuestion(disputedPairQuestion, { holderName: "王明" });
const disputedPairHolder = inferSourceHolderFromQuestion(disputedPairQuestion, { viaName: disputedPairVia });
assert(disputedPairIntent === "destination", "explicit disputed Pair Amount question must route to destination support lane");
assert(isExplicitTransferAmountQuestion(disputedPairQuestion, { holderName: disputedPairHolder, viaName: disputedPairVia }), "explicit disputed Pair Amount question must be recognized");
assert(frontdoorWantsFlowGraph({ intent: disputedPairIntent, questionText: disputedPairQuestion }) === false, "ordinary Pair Amount question must not be diverted into flow-graph support");
assert(frontdoorWantsFlowGraph({ intent: "destination", questionText: "画王明转给李华的资金流向图" }) === true, "explicit graph request must keep flow-graph support");

const rankingText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    title: "rank_counterparties completed",
    summary: "已返回本工具可支持的事实摘要；是否足够作答取决于当前问题类型、source boundary 和 validation state。"
  },
  key_facts: {
    ranking_context: {
      scope_type: "holder",
      holder_name: "示例主体甲",
      metric: "outflow",
      direction_mode: "out",
      success_filter: "all",
      cash_filter: "all",
      txn_count: 15082,
      account_count: 244,
      counterparty_count: 871,
      first_txn_at: "2005-04-28",
      last_txn_at: "2026-05-16",
      boundary: "本卡只控制普通排行/Top 问题；若用户问 A 转给 B 多少钱、金额争议、去重/换卡/同事实等金额核验，本卡仅是排行线索/线索候选，不能单独作为最终金额，必须补定向有来源支撑聚合或专项资金核算核验原明细/高置信去重金额、时间窗和核验意见。"
    },
    rankings: [{
      target: "rank_counterparties",
      metric: "outflow",
      rows: [{
        display_name: "示例对手乙",
        outflow: 1234567.89,
        turnover: 1234567.89,
        txn_count: 10,
        first_txn_at: "2026-01-02 10:00:00",
        last_txn_at: "2026-01-02 10:30:00"
      }]
    }]
  },
  next_actions: []
});

assert(!/^\s*[[{]/u.test(rankingText), "rank output must be concise readable facts, not a JSON support envelope");
assert(rankingText.includes("案件事实摘录"), "rank output must identify itself as a fact excerpt");
assert(rankingText.includes("金额核验"), "rank support must preserve amount-review boundary");
assert(rankingText.includes("排行线索") || rankingText.includes("线索候选"), "rank support must mark rank result as lead-only for amount review");
assert(!/support_only|final_answer_owned_by_focused_skill|case_id|delivery_state|answer_card_complete/u.test(rankingText), "rank output must not expose support-envelope internals");
assert(!rankingText.includes("可直接作答"), "rank support must not say 可直接作答");
assert(!rankingText.includes("仅用户追问"), "rank support must not defer Pair Amount validation to 仅用户追问");

const quickFactSkill = fs.readFileSync(path.join(PLUGIN_ROOT, "skills", "quick-fact", "SKILL.md"), "utf8");
const pairAmountSkill = fs.readFileSync(path.join(PLUGIN_ROOT, "skills", "pair-amount-investigation", "SKILL.md"), "utf8");
const pairAmountReconciliation = fs.readFileSync(path.join(PLUGIN_ROOT, "skills", "pair-amount-investigation", "references", "amount-reconciliation.md"), "utf8");
const pairAmountWriting = fs.readFileSync(path.join(PLUGIN_ROOT, "skills", "pair-amount-investigation", "references", "investigative-writing.md"), "utf8");
const pairAmountGuidance = [pairAmountSkill, pairAmountReconciliation, pairAmountWriting].join("\n");
const rootAgentPolicy = fs.readFileSync(path.join(PLUGIN_ROOT, "agents", "openai.yaml"), "utf8");
const pairAmountAgentPolicy = fs.readFileSync(path.join(PLUGIN_ROOT, "skills", "pair-amount-investigation", "agents", "openai.yaml"), "utf8");
const graphVisualizationSkill = fs.readFileSync(path.join(PLUGIN_ROOT, "skills", "graph-visualization", "SKILL.md"), "utf8");
const fundTracingSkill = fs.readFileSync(path.join(PLUGIN_ROOT, "skills", "fund-tracing", "SKILL.md"), "utf8");
const deliveryQcSkill = fs.readFileSync(path.join(PLUGIN_ROOT, "skills", "delivery-qc", "SKILL.md"), "utf8");
const commandMetadata = fs.readFileSync(path.join(PLUGIN_ROOT, "references", "command-metadata.json"), "utf8");
const commandMetadataJson = JSON.parse(commandMetadata);
const capabilityRegistry = JSON.parse(fs.readFileSync(path.join(PLUGIN_ROOT, "references", "capability-registry.json"), "utf8"));
const packagedCapabilityRegistry = JSON.parse(fs.readFileSync(path.join(PLUGIN_ROOT, "skills", "analytix-fund-analysis", "references", "capability-registry.json"), "utf8"));
assert(pairAmountSkill.length < 9000, "pair-amount SKILL.md must stay concise and rely on progressive disclosure references");
assert(pairAmountSkill.includes("负责最终成稿") || pairAmountSkill.includes("最终成稿 owner"), "pair-amount-investigation must own Pair Amount final drafting");
assert(pairAmountSkill.includes("amount-reconciliation.md") && pairAmountSkill.includes("investigative-writing.md"), "pair-amount skill must expose one-hop references for detailed amount logic and writing guidance");
assert(pairAmountSkill.includes("首条可见消息默认静默") && pairAmountSkill.includes("正在核验当前案件事实。"), "pair-amount owner must override generic visible skill announcements");
assert(pairAmountSkill.includes("不外露工程词") && pairAmountSkill.includes("内部字段"), "pair-amount completion gate must fail visible implementation leakage");
assert(rootAgentPolicy.includes("通用 skill 公告规则的显式例外") && rootAgentPolicy.includes("不能说明将使用哪个 skill、流程或工具"), "root agent policy must suppress visible skill/tool announcement before first tool use");
assert(pairAmountAgentPolicy.includes("选择本能力必须静默") && pairAmountAgentPolicy.includes("只能逐字写“正在核验当前案件事实。”"), "pair-amount agent policy must make skill selection silent in user-visible progress");
assert(pairAmountAgentPolicy.includes("investigative_row_reading") && pairAmountAgentPolicy.includes("备注/类型线索"), "pair-amount agent policy must tell the model to consume row-reading support within loader limits");
assert(pairAmountAgentPolicy.includes("精确数") && pairAmountAgentPolicy.includes("不得四舍五入"), "pair-amount agent policy must preserve exact visible continuation facts");
assert(pairAmountAgentPolicy.includes("2-3段连贯研判") && pairAmountAgentPolicy.includes("证明力固定/外部补证短尾"), "pair-amount agent policy must prefer compact investigative narrative and short proof-value tail over section templates");
assert(pairAmountGuidance.includes("本次依据") && pairAmountGuidance.includes("统计范围") && pairAmountGuidance.includes("来源边界") && pairAmountGuidance.includes("当前不能认定"), "pair-amount guidance must force amount-challenge source and evidence boundaries");
assert(pairAmountAgentPolicy.includes("金额挑战、自定义范围和复核后有效追问必须") && pairAmountAgentPolicy.includes("已有流水支持") && pairAmountAgentPolicy.includes("核验意见"), "pair-amount runtime prompt must force visible amount-challenge source boundary wording");
assert(pairAmountAgentPolicy.includes("只统计/复核后有效") && pairAmountAgentPolicy.includes("**核验意见：**") && pairAmountAgentPolicy.includes("暂不能认定"), "pair-amount runtime prompt must force source boundary wording for custom rescope followups");
assert(pairAmountAgentPolicy.includes("复核后有效转账”默认指双方有效总额") && pairAmountAgentPolicy.includes("不得缩成单一收款账号"), "pair-amount runtime prompt must keep custom effective-transfer rescope on the two-party effective total");
assert(pairAmountGuidance.includes("原明细金额") && pairAmountGuidance.includes("复核后有效金额"), "pair-amount guidance must require raw and reviewed amount bases");
assert(pairAmountGuidance.includes("账户集合") && pairAmountGuidance.includes("时间集中"), "pair-amount guidance must map cluster concepts to economic-investigation wording");
assert(pairAmountGuidance.includes("交易号为空") && pairAmountGuidance.includes("不能单独作为去重键"), "pair-amount guidance must preserve dedupe-key safety");
assert(pairAmountGuidance.includes("已确认重复记录，已剔除，不作为新增转账"), "pair-amount guidance must classify confirmed repeated records");
assert(pairAmountGuidance.includes("investigate_pair_amount"), "pair-amount owner must prefer first-class Pair Amount tool");
assert(pairAmountGuidance.includes("这不是 MCP-only") && pairAmountGuidance.includes("case-workbench"), "pair-amount owner must allow controlled workbench fallback without making SQL first choice");
assert(pairAmountGuidance.includes("不用 shell") && pairAmountGuidance.includes("旧报告"), "pair-amount owner must forbid local/history fact sourcing");
assert(pairAmountGuidance.includes("主要集中转账") && pairAmountGuidance.includes("集中交易以外逐笔往来"), "pair-amount guidance must explain concentrated transfers and outside-set rows");
assert(pairAmountGuidance.includes("复核后有效交易 20 笔以内") && pairAmountGuidance.includes("逐笔列出"), "pair-amount guidance must require complete small row tables");
assert(pairAmountGuidance.includes("已见承接/分流") && pairAmountGuidance.includes("不要把已在案事实写成"), "pair-amount guidance must surface in-case receiver continuation facts when available");
assert(pairAmountGuidance.includes("下一步核查建议") && pairAmountGuidance.includes("余额承接"), "pair-amount guidance must require explicit next investigative steps");
assert(pairAmountGuidance.includes("两三段连贯研判") && pairAmountGuidance.includes("不要写 `簇`"), "pair-amount guidance must prefer compact investigative narrative and ban data-science wording");
assert(!pairAmountSkill.includes("account clusters"), "pair-amount SKILL.md must not carry detailed English cluster instructions in the core body");
assert(!pairAmountAgentPolicy.includes("簇外"), "pair amount agent metadata must not teach the visible word 簇外");
assert(commandMetadata.includes("a capability_gap is not evidence and must not be used to compose a number"), "command metadata must block capability gaps as Pair Amount evidence");
assert(commandMetadata.includes("stop with an evidence boundary instead of using local files or history"), "command metadata must prevent local/history fact sourcing");
assert(commandMetadata.includes("Separate already-imported current-case流水 review/export actions from external missing materials"), "command metadata must split current-case export from external missing materials");
assert(commandMetadata.includes("do not tell the user to 调取双方完整流水"), "command metadata must block generic complete-flow retrieval");
assert(commandMetadata.includes("one bounded trace_subject_top_outflows continuation check must be used"), "command metadata must require a bounded in-case continuation check for material Pair Amount concentration");
assert(commandMetadata.includes("cover at least 30 days after the concentration date") && commandMetadata.includes("never default to next-day/24-hour only"), "command metadata must block too-narrow receiver continuation windows");
assert(commandMetadata.includes("已见承接/分流 paragraph"), "command metadata must require receiver-side continuation facts in user-facing material when available");
assert(commandMetadata.includes("do not list already visible transactions as if they still need to be found"), "command metadata must block missing-data wording for current-case facts");
assert(commandMetadata.includes("20 rows or fewer") && commandMetadata.includes("complete returned row-level transfer table"), "command metadata must require complete row tables for small current-case pair amounts");
assert(commandMetadata.includes("其余X笔") && commandMetadata.includes("小额转账合计"), "command metadata must forbid folded small-row shortcuts");
assert(commandMetadata.includes("compact economic-investigation material") && commandMetadata.includes("集中交易以外逐笔往来"), "command metadata must require report-style transfer-structure judgment for natural Pair Amount answers");
assert(commandMetadata.includes("compact economic-investigation material") && commandMetadata.includes("not a heading checklist"), "command metadata must require report-style natural Pair Amount answers");
assert(commandMetadata.includes("Read returned rows like case流水"), "command metadata must require case-row interpretation before external proof requests");
assert(commandMetadata.includes("investigative_row_reading") && commandMetadata.includes("cue-bucket"), "command metadata must require runtime row-reading facts as the natural answer spine");
assert(commandMetadata.includes("full detail aggregate is the controlling answer"), "command metadata must keep detail aggregate above rank/focus/old-report amounts");
assert(commandMetadata.includes("early, low-value, terminal, batch, repayment, remittance"), "command metadata must preserve low-value/early/outside-set rows");
assert(commandMetadata.includes("residual-bucket phrases") && commandMetadata.includes("全期间扣除该集中链路后"), "command metadata must forbid residual-bucket wording for outside focused concentration rows");
assert(commandMetadata.includes("导出核对该N笔明细") && commandMetadata.includes("proof-value fixation"), "command metadata must keep export/check wording from replacing complete small tables");
assert(commandMetadata.includes("do not write 需补取/调取收款账户后续出账明细"), "command metadata must block returned continuation facts being described as missing data");
const amountCommand = commandMetadataJson.commands?.find((item) => item.command === "/analytix amount");
assert(amountCommand?.toolBudget?.recommendedToolCalls >= 2, "Pair Amount command metadata must allow the bounded continuation fact call when needed");
assert(amountCommand?.toolBudget?.maxToolCalls >= 2, "Pair Amount command metadata must not force a one-tool-call stop when a high-share concentration needs continuation facts");
assert(amountCommand?.toolBudget?.supportFlow?.some((item) => String(item).includes("current answer evidence table")), "Pair Amount metadata must state that small returned rows are answer evidence, not export placeholders");
assert(quickFactSkill.includes("does not own Pair Amount"), "quick-fact must explicitly reject Pair Amount ownership");
assert(quickFactSkill.includes("Pair Amount turns are not complete in quick-fact"), "quick-fact completion gate must reject Pair Amount finalization");
assert(!quickFactSkill.includes("First call `funds_investigate` as the amount-review mini-review navigator"), "quick-fact must not instruct first-call funds_investigate for Pair Amount");
assert(graphVisualizationSkill.includes("three-line summary") || graphVisualizationSkill.includes("three-row"), "graph visualization must reject shallow three-row summaries");
assert(graphVisualizationSkill.includes("来源溯源") && graphVisualizationSkill.includes("赎回后分流"), "graph visualization must require layered source/downstream expansion");
assert(fundTracingSkill.includes("Default graph") && fundTracingSkill.includes("接续去向/未调取端点"), "fund tracing must require default graph depth and endpoints");
assert(fundTracingSkill.includes("本窗口") && fundTracingSkill.includes("核验窗口"), "fund tracing must forbid tool-window wording in user-visible downstream answers");
assert(fundTracingSkill.includes("If current-case data already contains downstream outflows") && fundTracingSkill.includes("Do not move already visible transactions into `下一步取证`"), "fund tracing must analyze visible downstream facts before proof/continuation next steps");
assert(fundTracingSkill.includes("Keep `下一步` short and subordinate"), "fund tracing must keep next-step wording subordinate to current-case analysis");
assert(deliveryQcSkill.includes("already visible current-case transaction facts"), "delivery QC must block treating visible current-case facts as missing");
assert(deliveryQcSkill.includes("已见承接/分流"), "delivery QC must require in-case continuation facts when available");
assert(deliveryQcSkill.includes("20 笔以内") && deliveryQcSkill.includes("complete returned"), "delivery QC must block truncating small pair-amount detail tables");
assert(deliveryQcSkill.includes("其余 X 笔") && deliveryQcSkill.includes("小额转账"), "delivery QC must block folded row shortcuts");
assert(deliveryQcSkill.includes("interchangeable boilerplate") && deliveryQcSkill.includes("主要集中转账"), "delivery QC must block template headings without case-specific depth");
assert(deliveryQcSkill.includes("headings as the main answer"), "delivery QC must block heading-led answers without case narrative");
assert(deliveryQcSkill.includes("more than four top-level headings") && deliveryQcSkill.includes("strong rewrite trigger"), "delivery QC must block long heading skeletons for natural pair amount answers");
assert(deliveryQcSkill.includes("short proof-value tail") && deliveryQcSkill.includes("not a substitute for the current-case研判"), "delivery QC must keep next-step evidence requests subordinate to current-case analysis");
assert(deliveryQcSkill.includes("work plan") && deliveryQcSkill.includes("current case facts"), "delivery QC must block next-step plans that replace current-case analysis");
assert(deliveryQcSkill.includes("transfer-relationship analysis") && deliveryQcSkill.includes("集中交易以外逐笔往来"), "delivery QC must block correct-total but shallow pair amount answers");
assert(deliveryQcSkill.includes("investigative_row_reading") && deliveryQcSkill.includes("low-value-row warning"), "delivery QC must block ignoring runtime row-reading support");
assert(deliveryQcSkill.includes("basic accounting level") && deliveryQcSkill.includes("account spread"), "delivery QC must block correct but basic accounting-level Pair Amount answers");
assert(deliveryQcSkill.includes("current-case detail aggregate controls the answer"), "delivery QC must block shrinking to rank/focus/old-report amounts");
assert(deliveryQcSkill.includes("early, low-value, terminal, batch, repayment, remittance"), "delivery QC must block dropping old/low/outside focused concentration rows");
assert(deliveryQcSkill.includes("全期间扣除该集中链路后") && deliveryQcSkill.includes("零散历史往来"), "delivery QC must block residual-bucket wording");
assert(deliveryQcSkill.includes("后续出账明细") && deliveryQcSkill.includes("prove承接关系"), "delivery QC must block generic missing-data wording after continuation facts are returned");

for (const [label, registry] of [["source registry", capabilityRegistry], ["packaged registry", packagedCapabilityRegistry]]) {
  const priorityTools = registry.tool_discovery_contract?.priority_tools || [];
  const defaultTools = registry.tool_discovery_contract?.default_visible_tools || [];
  const pairPriorityIndex = priorityTools.indexOf("investigate_pair_amount");
  const rankPriorityIndex = priorityTools.indexOf("rank_counterparties");
  const fundsPriorityIndex = priorityTools.indexOf("funds_investigate");
  const pairDefaultIndex = defaultTools.indexOf("investigate_pair_amount");
  const rankDefaultIndex = defaultTools.indexOf("rank_counterparties");
  const fundsDefaultIndex = defaultTools.indexOf("funds_investigate");
  assert(pairPriorityIndex >= 0, `${label} must include investigate_pair_amount in priority tools`);
  assert(pairDefaultIndex >= 0, `${label} must include investigate_pair_amount in default visible tools`);
  assert(pairPriorityIndex < rankPriorityIndex && pairPriorityIndex < fundsPriorityIndex, `${label} must prioritize investigate_pair_amount before rank/funds support`);
  assert(pairDefaultIndex < rankDefaultIndex && pairDefaultIndex < fundsDefaultIndex, `${label} must show investigate_pair_amount before rank/funds support in default tools`);
  assert((registry.tool_discovery_contract?.max_default_visible_tools || 0) >= defaultTools.length, `${label} max_default_visible_tools must not hide Pair Amount from the default surface`);
}

const frontdoorRuntime = fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", "frontdoor-runtime.mjs"), "utf8");
const toolSchemas = fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", "tool-schemas.mjs"), "utf8");
const toolCallRuntime = fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", "tool-call-runtime.mjs"), "utf8");
const pairAmountToolSchema = MCP_TOOL_SCHEMAS.find((tool) => tool.name === "investigate_pair_amount");
const traceSubjectOutflowsToolSchema = MCP_TOOL_SCHEMAS.find((tool) => tool.name === "trace_subject_top_outflows");
assert(pairAmountToolSchema, "MCP surface must expose investigate_pair_amount");
assert(traceSubjectOutflowsToolSchema, "MCP surface must expose trace_subject_top_outflows");
const pairAmountToolIndex = MCP_TOOL_SCHEMAS.findIndex((tool) => tool.name === "investigate_pair_amount");
const fundsInvestigateIndex = MCP_TOOL_SCHEMAS.findIndex((tool) => tool.name === "funds_investigate");
assert(pairAmountToolIndex >= 0 && fundsInvestigateIndex >= 0 && pairAmountToolIndex < fundsInvestigateIndex, "MCP surface must list investigate_pair_amount before funds_investigate");
assert(pairAmountToolSchema.description.includes("特定双方资金往来核验一等事实入口"), "investigate_pair_amount must be first-class Pair Amount support");
assert(pairAmountToolSchema.description.includes("普通双方资金往来金额首答优先调用本入口"), "investigate_pair_amount must be preferred for first answers");
assert(pairAmountToolSchema.description.includes("不得为补账号、补逐笔字段、扩大 top_n 或改写问题再次调用"), "investigate_pair_amount schema must block duplicate same-fact enrichment calls");
assert(pairAmountToolSchema.title === "特定双方资金往来核验", "investigate_pair_amount must expose a Chinese user-visible title");
assert(pairAmountToolSchema.annotations?.title === "特定双方资金往来核验", "investigate_pair_amount annotations must expose a Chinese title for MCP clients");
assert(pairAmountToolSchema._meta?.["openai/toolInvocation/invoking"] === "正在核验双方资金往来", "investigate_pair_amount must expose Chinese tool invocation text");
assert(MCP_TOOL_SCHEMAS.every((tool) => tool.title && tool.annotations?.title && tool._meta?.["openai/toolInvocation/invoking"]), "all MCP tools must expose Chinese user-visible invocation metadata");
assert(traceSubjectOutflowsToolSchema.description.includes("30 calendar days") && traceSubjectOutflowsToolSchema.description.includes("24-hour"), "trace_subject_top_outflows schema must discourage next-day Pair Amount continuation windows");
assert(toolSchemas.includes("investigate_pair_amount"), "tool schemas source must include investigate_pair_amount");
assert(toolCallRuntime.includes('name === "investigate_pair_amount"'), "tool runtime must route investigate_pair_amount");
assert(toolCallRuntime.includes("normalizeFullCaseAnalysisArgs") && toolCallRuntime.includes("include_internal_playbooks = false"), "tool runtime must keep first full-case/report fact package bounded by default");
assert(toolCallRuntime.includes("normalizePairAmountSupportQuestion") && toolCallRuntime.includes("full_two_party_effective_total"), "tool runtime must normalize unrestricted effective-amount rescope followups to the full two-party total");
assert(toolCallRuntime.includes("looksLikePairAmountRequest"), "tool runtime must detect Pair Amount calls sent through the broad navigator");
assert(toolCallRuntime.includes("routed_from_tool") && toolCallRuntime.includes('"funds_investigate"'), "tool runtime must mark Pair Amount calls redirected away from funds_investigate ownership");
assert(toolCallRuntime.includes('intent: "destination"'), "investigate_pair_amount must route to destination Pair Amount support lane");
assert(frontdoorRuntime.includes("pair_amount_review_card"), "frontdoor must expose a Pair Amount review card");
assert(frontdoorRuntime.includes("run_case_sql"), "frontdoor Pair Amount path must use targeted Controlled Case Workbench");
assert(frontdoorRuntime.includes("executeLocalDuckdbRunCaseSql"), "frontdoor Pair Amount path must use local read-only DuckDB fallback when backend SQL parser fails closed");
assert(frontdoorRuntime.includes("pairAmountWorkbenchNeedsLocalFallback"), "frontdoor Pair Amount path must detect parser-gap workbench results instead of silently using rank amounts");
assert(frontdoorRuntime.includes("runLocalPairAmountWorkbench"), "frontdoor Pair Amount path must re-run the same controlled SQL through the local DuckDB workbench fallback");
assert(frontdoorRuntime.includes("dominantPairAmountFocusCluster"), "frontdoor must identify a dominant current-case concentrated transfer");
assert(frontdoorRuntime.includes("date_counterparty_clusters"), "frontdoor must expose date/counterparty clusters");
assert(frontdoorRuntime.includes("source_account_count"), "frontdoor must expose payer account count");
assert(frontdoorRuntime.includes("counterparty_account_count"), "frontdoor must expose receiver account count");
assert(frontdoorRuntime.includes("full_period_pair_amount_with_focus_cluster"), "frontdoor must keep full pair amount as control even when a focused concentration exists");
assert(frontdoorRuntime.includes("PAIR_AMOUNT_FOCUS_CLUSTER_NOT_TOTAL"), "frontdoor must warn that focused concentrations are not unrestricted totals");
assert(!frontdoorRuntime.includes("dominant_current_case_transaction_cluster"), "frontdoor must not promote focus_cluster to the ordinary validation state");
assert(!frontdoorRuntime.includes("workbenchEffective = focusCluster"), "frontdoor must not use focused concentration as the default workbench amount");
assert(frontdoorRuntime.includes("expanded_same_name_scope"), "frontdoor must expose full pair detail scope for unrestricted questions");
assert(frontdoorRuntime.includes("ranking_candidate_only_not_final_pair_amount"), "frontdoor must keep rank_counterparties candidate-only fallback for Pair Amount");
assert(frontdoorRuntime.includes("ranking_candidate_cross_check_not_final_pair_amount"), "frontdoor must keep ranking as cross-check when detail support is available");
assert(frontdoorRuntime.includes("PAIR_AMOUNT_DETAIL_AGGREGATE_DISAGREEMENT"), "frontdoor must flag rank/detail disagreement without shrinking to a focused concentration");
assert(frontdoorRuntime.includes("rank_cross_check_amount"), "frontdoor must preserve rank cross-check amount separately from controlling pair detail");
assert(frontdoorRuntime.includes("查无信息") && frontdoorRuntime.includes("forbidden_single_keys"), "frontdoor must preserve placeholder txn_id guard");
assert(frontdoorRuntime.includes("required_joint_fields"), "frontdoor must require joint fields for dedup review");
assert(frontdoorRuntime.includes("small_amount_candidate_amount"), "frontdoor must expose optional fee/principal candidate");
assert(frontdoorRuntime.includes("dedup_key_safety"), "frontdoor must expose structured dedup-key safety");
assert(frontdoorRuntime.includes("top_transactions"), "frontdoor must expose Pair Amount Top transaction candidates");
assert(!frontdoorRuntime.includes("answer_draft"), "frontdoor must not generate answer drafts");
assert(!frontdoorRuntime.includes('title: "金额核验小研判"'), "frontdoor must not expose old mini-review title as card title");
assert(!frontdoorRuntime.includes('"金额核验小研判"'), "frontdoor must not preserve old mini-review title");

const outputCompiler = fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", "agent-output-compiler.mjs"), "utf8");
assert(outputCompiler.includes("transactionLimit = 20"), "Pair Amount support compiler must default to retaining a complete small row table");
assert(outputCompiler.includes("renderSupportEnvelopeForAgent"), "agent output compiler must render concise non-JSON support text for model-visible content");
assert(outputCompiler.includes("pair_amount_fact_pack"), "Pair Amount support compiler must expose factual pair amount pack instead of prompt instructions");
assert(outputCompiler.includes("other_verified_transfer_rows"), "Pair Amount support compiler must expose other verified transfer rows for relationship analysis");
assert(outputCompiler.includes("buildSupportEvidenceEnvelope"), "agent output compiler must build support envelope");
assert(outputCompiler.includes("support_only: true"), "agent output compiler must mark ordinary output support-only");
assert(outputCompiler.includes("final_answer_owned_by_focused_skill: true"), "agent output compiler must leave final answer to focused owner");
assert(outputCompiler.includes('"duplicate_or_unsupported_amount"'), "support envelope must preserve duplicate/evidence-insufficient amount");
assert(outputCompiler.includes("confirmed_same_fact_duplicate_amount"), "support envelope must preserve confirmed repeated-record amount");
assert(outputCompiler.includes("unresolved_duplicate_or_unsupported_amount"), "support envelope must preserve unresolved duplicate/evidence-insufficient amount");
assert(outputCompiler.includes('"dedup_key_safety"'), "support envelope must preserve dedup-key safety");
assert(outputCompiler.includes('"top_transactions"'), "support envelope must preserve Pair Amount Top transaction candidates");
assert(outputCompiler.includes("publicPairAmountSupportKey"), "support envelope must translate internal cluster keys before exposing facts to the model");
assert(outputCompiler.includes('"major_concentrated_transfer"'), "support envelope must expose focused transfer with business-readable wording");
assert(outputCompiler.includes('"major_concentrated_transfer_role"'), "support envelope must expose focused transfer role with business-readable wording");
assert(outputCompiler.includes('"major_concentrated_transfer_share"'), "support envelope must expose focused transfer share with business-readable wording");
assert(outputCompiler.includes('"other_verified_transfer_amount"'), "support envelope must expose other verified transfer amount with business-readable wording");
assert(outputCompiler.includes('"other_verified_transfer_count"'), "support envelope must expose other verified transfer count with business-readable wording");
assert(outputCompiler.includes('"receiver_account_groups"') && outputCompiler.includes('"time_concentrations"'), "support envelope must expose account/time groupings with economic-investigation wording");
assert(outputCompiler.includes('"full_period_effective_amount"'), "support envelope must preserve full pair amount scalar");
assert(outputCompiler.includes('"full_period_effective_count"'), "support envelope must preserve full pair count scalar");
assert(outputCompiler.includes('"first_txn_at"') && outputCompiler.includes('"last_txn_at"'), "support envelope must preserve full pair statistical period scalars");
assert(!outputCompiler.includes('"rank_cross_check_amount"'), "support envelope must not expose rank cross-check amount as a competing Pair Amount conclusion");
assert(outputCompiler.includes("COMPETING_PAIR_RANK_KEY_PATTERN"), "support envelope must strip competing rank Pair Amount fields from compact evidence");
assert(outputCompiler.includes("transactionLimit: 20"), "support envelope must keep enough row candidates for full-detail pair answers");
assert(frontdoorRuntime.includes("slice(0, 20)"), "frontdoor Pair Amount review must expose enough row candidates for small full-detail pair answers");
for (const forbiddenSupportPrompt of [
  "opening_sentence_must_cover",
  "natural_pair_answer_depth_contract",
  "natural_pair_visible_shape_contract",
  "investigative_row_reading",
  "complete_transfer_table",
  "receiver_continuation_check",
  "must_list_all_returned_rows",
  "failed_answer_shapes",
  "case_user_review_lens",
  "must_include_transfer_structure_judgment",
  "must_not_use_vague_scope_words",
  "raw_transaction_candidates"
]) {
  assert(!outputCompiler.includes(forbiddenSupportPrompt), `agent-readable support must not carry prompt/control field: ${forbiddenSupportPrompt}`);
}
assert(outputCompiler.includes("row_cue_buckets"), "support envelope must expose factual row cue buckets");
assert(outputCompiler.includes("对手方排名|候选重复金额|折叠额外行"), "support envelope must filter rank diagnostic warnings from full-detail Pair Amount support");
assert(outputCompiler.includes('"source_accounts"'), "support envelope must preserve payer accounts");
assert(outputCompiler.includes('"counterparty_accounts"'), "support envelope must preserve receiver accounts");
assert(outputCompiler.includes('"expanded_same_name_scope"'), "support envelope must preserve broader same-name boundary");
assert(outputCompiler.includes('"effective_stat_basis"'), "support envelope must preserve Pair Amount effective-stat basis");
assert(outputCompiler.includes("差异说明") && outputCompiler.includes("差异原因") && outputCompiler.includes("当前不能认定"), "support envelope must carry amount-challenge difference explanation and current proof limits");
assert(outputCompiler.includes("交易明细列示") && outputCompiler.includes("折叠占位行"), "support envelope must block folded transaction-table placeholders");
assert(outputCompiler.includes("重点交易说明") && outputCompiler.includes("主要集中转账") && outputCompiler.includes("不得替代完整两方总额"), "support envelope must prevent focused transfer clusters from replacing the two-party effective total");
assert(!outputCompiler.includes("renderPairAmountReviewCardForAgent"), "agent output compiler must not contain Pair Amount prose renderer");
assert(!outputCompiler.includes("appendDraftSection"), "agent output compiler must not replay answer draft sections");
assert(!outputCompiler.includes("appendDraftPrefixLines"), "agent output compiler must not replay answer draft prefixes");

const pairAmountSupportText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    card_type: "pair_amount_review_card",
    fact_source: "production_backend_controlled_workbench",
    validation_state: "full_period_pair_amount_with_focus_cluster",
    holder_name: "示例付款人",
    via_holder_name: "示例收款人",
    first_txn_at: "2020-01-01 09:00:00",
    last_txn_at: "2025-01-03 10:00:00",
    raw_detail_amount: 230000,
    raw_detail_count: 4,
    full_period_effective_amount: 130000,
    full_period_effective_count: 3,
    high_confidence_same_fact_effective_amount: 130000,
    high_confidence_same_fact_count: 3,
    duplicate_or_unsupported_amount: 100000,
    confirmed_same_fact_duplicate_amount: 100000,
    confirmed_same_fact_duplicate_count: 1,
    duplicate_review_status: "confirmed_same_holder_cross_account_same_fact",
    focus_cluster: {
      date: "2025-01-03",
      counterparty_acct: "2002",
      first_txn_at: "2025-01-03 09:00:00",
      last_txn_at: "2025-01-03 10:00:00",
      effective_amount: 100000,
      effective_count: 1,
      share_of_effective_amount: 0.769231
    },
    outside_focus_cluster_effective_amount: 30000,
    outside_focus_cluster_effective_count: 2,
    source_account_count: 2,
    source_accounts: ["1001", "1002"],
    counterparty_account_count: 2,
    counterparty_accounts: ["2001", "2002"],
    top_transactions: [
      { txn_time: "2025-01-03 09:00:00", payer_account: "1002", receiver_account: "2002", amount: 100000, txn_type: "跨行汇出" },
      { txn_time: "2020-01-01 09:00:00", payer_account: "1001", receiver_account: "2001", amount: 20000, remark: "还款", txn_type: "转账" },
      { txn_time: "2020-01-02 09:00:00", payer_account: "1001", receiver_account: "2001", amount: 10000, txn_type: "自助终端" }
    ]
  }
});
assert(!/^\s*[[{]/u.test(pairAmountSupportText), "compiled Pair Amount support must be readable text, not JSON");
assert(pairAmountSupportText.includes("案件事实摘录: 两方转账金额核验"), "compiled Pair Amount support must identify the fact excerpt");
assert(pairAmountSupportText.includes("130,000.00 元") && pairAmountSupportText.includes("3"), "compiled Pair Amount support must preserve amount and count");
assert(pairAmountSupportText.includes("已确定不同卡号/账号重复记录 100,000.00 元") && pairAmountSupportText.includes("不作为新增转账"), "compiled Pair Amount support must identify confirmed repeated records");
assert(!pairAmountSupportText.includes("疑似重复或证据不足金额 100,000.00 元"), "confirmed repeated records must not be described as evidence-insufficient");
assert((pairAmountSupportText.match(/\[account-redacted\]/gu) || []).length >= 4, "ordinary agent context must retain account-list cardinality while applying the restricted PII projection");
assert(!/[12]00[12]/u.test(pairAmountSupportText), "ordinary agent context must not expose raw payer or receiver accounts");
assert(pairAmountSupportText.includes("2020-01-01 09:00:00") && pairAmountSupportText.includes("2025-01-03 10:00:00"), "compiled Pair Amount support must preserve full period");
assert(pairAmountSupportText.includes("| 时间 | 付款账户 | 收款账户 | 金额 | 摘要/类型 |"), "compiled Pair Amount support must render row-level transaction table");
assert(pairAmountSupportText.includes("还款") && pairAmountSupportText.includes("自助终端"), "compiled Pair Amount support must preserve row cue facts");
assert(pairAmountSupportText.includes("交易明细列示") && pairAmountSupportText.includes("20笔以内"), "compiled Pair Amount support must preserve full small-table rendering boundary");
assert(!/support_only|final_answer_owned_by_focused_skill|case_id|delivery_state|answer_card_complete|raw_transaction_candidates/u.test(pairAmountSupportText), "compiled Pair Amount support must hide internal support fields");
assert(!userVisibleLeakageLabels(pairAmountSupportText).length, `compiled Pair Amount support leaked internal language\n${pairAmountSupportText}`);

const pairAmountExecutionCases = [];
const noPairAmountCacheRuntime = createToolCallRuntime({
  fundsInvestigate: async (args) => {
    pairAmountExecutionCases.push(args.case_id);
    return {
      case_id: args.case_id,
      answer_card: {
        card_type: "pair_amount_review_card",
        case_id: args.case_id
      }
    };
  }
});
const samePairAmountQuestion = {
  question: "示例付款人转给示例收款人多少钱？",
  payer_name: "示例付款人",
  receiver_name: "示例收款人"
};
const caseAFirst = await noPairAmountCacheRuntime.callTool("investigate_pair_amount", {
  ...samePairAmountQuestion,
  case_id: "case-a"
});
const caseB = await noPairAmountCacheRuntime.callTool("investigate_pair_amount", {
  ...samePairAmountQuestion,
  case_id: "case-b"
});
const caseASecond = await noPairAmountCacheRuntime.callTool("investigate_pair_amount", {
  ...samePairAmountQuestion,
  case_id: "case-a"
});
assert(pairAmountExecutionCases.join(",") === "case-a,case-b,case-a", `PairAmountCacheNeverCrossesCaseBinding: expected three live executions, got ${pairAmountExecutionCases.join(",")}`);
assert(caseAFirst.case_id === "case-a" && caseB.case_id === "case-b" && caseASecond.case_id === "case-a", "PairAmountCacheNeverCrossesCaseBinding: response case binding was reused across cases");

const cardRenderer = fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", "card-renderer.mjs"), "utf8");
const destinationRuntime = fs.readFileSync(path.join(PLUGIN_ROOT, "mcp", "destination-diagnostic-runtime.mjs"), "utf8");
for (const [label, source] of [["card renderer", cardRenderer], ["destination runtime", destinationRuntime], ["agent output compiler", outputCompiler]]) {
  assert(!source.includes("金额核验小研判"), `${label} must not expose old support-card title`);
  assert(!source.includes("口径差异说明"), `${label} must not expose old difference heading`);
  assert(!source.includes("全期间同名收款人统计"), `${label} must not expose old full-period support label`);
  assert(!source.includes("重点收款账号统计"), `${label} must not expose old core-account support label`);
}

const caseSourceError = new CaseSourceError({
  code: "CASE_SOURCE_BLOCKER",
  message: "Unable to resolve active Analytix case via backend active-case API: active case is not set",
  stage: "backend_active_case",
  details: { backend_status: 404, backend_error_code: "CASE_NOT_FOUND" }
});

const invalidCaseError = new CaseSourceError({
  code: "INVALID_CASE_SOURCE",
  message: "Explicit Analytix case_id does not exist: missing-case",
  caseId: "missing-case",
  stage: "explicit_case_lookup",
  details: { backend_status: 404, backend_error_code: "CASE_NOT_FOUND" }
});

function createCaseSourceRuntime({ invalid = false } = {}) {
  const error = invalid ? invalidCaseError : caseSourceError;
  return createToolCallRuntime({
    fundsInvestigate: async () => { throw error; },
    getCaseGraph: async () => { throw error; },
    buildFundFlowGraph: async () => { throw error; },
    resolveCase: async () => { throw error; },
    compactCaseRecord: (value) => value,
    compactSkillsRecord: (value) => value,
    httpJson: async () => { throw error; },
    getImportOverview: async () => { throw error; },
    getCleaningOverview: async () => { throw error; },
    getCaseDataPipelineOverview: async () => { throw error; },
    getCaseScopeMap: async () => { throw error; },
    getStatsMeta: async () => { throw error; },
    getStatsTree: async () => { throw error; },
    queryStatsRows: async () => { throw error; },
    queryStatsTxnRows: async () => { throw error; },
    queryAccountTxnRows: async () => { throw error; },
    getAnalysisDashboard: async () => { throw error; },
    executeSkill: async () => { throw error; }
  });
}

const activeBlocker = await createCaseSourceRuntime().callTool("get_current_case", {});
assert(activeBlocker.status === "blocked", "get_current_case must return blocked Case Source Blocker");
assert(activeBlocker.error_code === "CASE_SOURCE_BLOCKER", "active missing case must use CASE_SOURCE_BLOCKER");
assert(activeBlocker.answer_card?.card_type === "case_source_blocker", "case source blocker must be rendered as answer card");
const blockerText = compiler.agentReadableToolText(activeBlocker);
assert(blockerText.includes("当前案件来源未就绪"), "blocker text must name case-source blocker in user-facing language");
assert(blockerText.includes("不得搜索本机数据库文件"), "blocker text must forbid local database-file search");
assert(blockerText.includes("不得连续尝试其他编号"), "blocker text must forbid id guessing loops");
assert(userVisibleLeakageLabels(blockerText).length === 0, `blocker output leaked internal wording: ${userVisibleLeakageLabels(blockerText).join(", ")}`);

const invalidBlocker = await createCaseSourceRuntime({ invalid: true }).callTool("rank_holders", { case_id: "missing-case" });
assert(invalidBlocker.status === "blocked", "explicit invalid case_id must return blocker");
assert(invalidBlocker.error_code === "INVALID_CASE_SOURCE", "explicit invalid case_id must not downgrade to active-case lookup");
assert(invalidBlocker.case_source_blocker.explicit_case_id === "missing-case", "invalid blocker must preserve explicit case_id");

console.log(JSON.stringify({
  status: "ok",
  skipped_asset_bundle_checks: [],
  checked: [
    "rank support envelope avoids direct-answer lure",
    "rank support envelope marks amount review as lead-only",
    "ordinary Pair Amount routing is not diverted into flow-graph support",
    "pair-amount-investigation owns Pair Amount finalization",
    "quick-fact rejects Pair Amount ownership",
    "frontdoor returns Pair Amount review cards with deduped counterparty aggregation controlling effective facts",
    "Pair Amount support envelope preserves original-detail/dedup/principal/duplicate/top-transaction facts",
    "PairAmountCacheNeverCrossesCaseBinding",
    "active-case source blocker forbids local case discovery",
    "invalid explicit case_id does not trigger id guessing"
  ]
}, null, 2));
