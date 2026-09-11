import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {
  isCaseSourceError
} from "./backend-api-client.mjs";
import { abortError, throwIfAborted } from "./abort-runtime.mjs";
import {
  resolveCaseProjectContext
} from "./case-project-context.mjs";
import {
  MCP_VALIDATION_ERROR_KEY,
  arrayOf,
  intOrUndefined,
  normalizeSkillArgs,
  objectOf,
  text,
  validationWarning
} from "./runtime-normalizers.mjs";
import {
  skillEnvelopeData,
  unwrapSkillEnvelope
} from "./agent-payload-compiler.mjs";
import { withClaimReviewProtocol } from "./claim-verifier-protocol.mjs";
import {
  INTERNAL_DATASET_SNAPSHOT_PROBE_TOOL,
  executeLocalDuckdbWorkbenchSkill,
  supportsLocalDuckdbWorkbenchSkill
} from "./duckdb-workbench-runtime.mjs";
import { analyzeDuckdbReadOnlySql } from "./duckdb-sql-policy.mjs";
import {
  isDuckdbDiagnosticError,
  publicDuckdbDiagnostic
} from "./duckdb-diagnostic.mjs";
import {
  buildFundGraphFromSeedRows,
  seedMatchesName,
  seedRowsFromTopOutflows,
  summarizeSeedTransfers
} from "./fundgraph-builder.mjs";
import {
  probeToolTypes,
  skillToolMap
} from "./tool-runtime-routing.mjs";
import { isPreExecutionBlockedToolName } from "./pre-execution-tool-policy.mjs";
import { unavailableSkillsSurfaceState } from "./case-pipeline-runtime.mjs";

export const TOOL_CALL_RUNTIME_VERSION = "0.16.16";

const TRANSIENT_TOOL_ERROR_PATTERN = /(INTERNAL_ERROR|retryable|fetch failed|ECONNRESET|ETIMEDOUT|stream disconnected|database is locked|DuckDB.*occupied)/iu;
const RANKING_TOOLS = new Set(["rank_accounts", "rank_holders", "rank_counterparties"]);
const CASE_SOURCE_RECOVERY_ACTIONS = [
  "确认当前对话工作区是 analytix 案件项目目录，且存在 .analytix/case-project.json。",
  "如已输入案件编号，只能与该案件项目绑定的 caseId 一致；不一致时不得跨案读取。",
  "案件项目绑定恢复后，重新执行案件事实核验；不得改用本机文件、历史输出或测试样例作答。"
];
const CASE_SOURCE_FORBIDDEN_ACTIONS = [
  "不得列本机目录。",
  "不得搜索本机数据库文件。",
  "不得扫描下载目录或相邻案件文件夹。",
  "不得从历史输出、测试样例、缓存文本或失败核验中猜案件编号。",
  "显式案件编号无效后，不得用猜测编号连续重试排行或研判核验。"
];

function looksLikePairAmountRequest(args = {}) {
  const source = objectOf(args);
  const question = text(source.question || source.query || source.prompt || source.analysis_goal || source.intent);
  const payerName = text(source.payer_name || source.holder_name);
  const receiverName = text(source.receiver_name || source.via_holder_name || source.counterparty_name);
  const requestedIntent = text(source.intent || source.requested_intent);
  if (payerName && receiverName && (requestedIntent === "destination" || /(?:转给|转向|给|打给|汇给|转账|往来|金额|多少钱|多少|几笔)/u.test(question))) {
    return true;
  }
  return /[\p{Script=Han}A-Za-z0-9]{1,24}(?:转给|转向|打给|汇给|转账给)[\p{Script=Han}A-Za-z0-9]{1,24}(?:多少钱|多少|金额|几笔|合计)|[\p{Script=Han}A-Za-z0-9]{1,24}(?:给)[\p{Script=Han}A-Za-z0-9]{1,24}(?:打了|转了|付了|汇了)?(?:多少钱|多少|几笔)/u.test(question);
}

function looksLikeReportPublicationRequest(args = {}) {
  const source = objectOf(args);
  const intent = text(source.intent || source.requested_intent);
  const body = [source.question, source.prompt, source.report_text, source.analysis_goal]
    .map(text)
    .filter(Boolean)
    .join("\n");
  return intent === "full_case"
    || source.write_report === true
    || /(?:正式)?(?:报告|材料|简报|附件|附表|导出|发布)/u.test(body);
}

function normalizePairAmountSupportQuestion(args = {}) {
  const source = objectOf(args);
  const question = text(source.question || source.query || source.prompt || source.analysis_goal);
  const payerName = text(source.payer_name || source.holder_name);
  const receiverName = text(source.receiver_name || source.via_holder_name || source.counterparty_name);
  const asksEffectiveRescope = /(?:自定义范围|自定义口径|只统计|重算|复核后有效|初筛)/u.test(question);
  const explicitlyNarrowsScope = /\b\d{10,}\b/u.test(question)
    || /(?:19|20)\d{2}[-年/]\d{1,2}(?:[-月/]\d{1,2})?/u.test(question)
    || /(?:这笔|该笔|这组|该组|该账户|收款账户|付款账户|账号)/u.test(question);
  if (!asksEffectiveRescope || explicitlyNarrowsScope || !payerName || !receiverName) {
    return source;
  }
  return {
    ...source,
    question: `${payerName}转给${receiverName}多少钱？请按完整两方复核后有效转账金额回答，并说明初筛流水记录差异。`,
    requested_pair_amount_scope: "full_two_party_effective_total"
  };
}

async function routePairAmountThroughFocusedSupport(fundsInvestigate, args = {}, { signal } = {}) {
  throwIfAborted(signal);
  const normalizedArgs = normalizePairAmountSupportQuestion(normalizeSkillArgs("investigate_pair_amount", args));
  const payerName = text(normalizedArgs.payer_name || normalizedArgs.holder_name);
  const receiverName = text(normalizedArgs.receiver_name || normalizedArgs.via_holder_name || normalizedArgs.counterparty_name);
  const response = await fundsInvestigate({
    ...normalizedArgs,
    intent: "destination",
    question: text(normalizedArgs.question) || [payerName, receiverName].filter(Boolean).join("转给"),
    holder_name: payerName || text(normalizedArgs.holder_name),
    via_holder_name: receiverName || text(normalizedArgs.via_holder_name || normalizedArgs.counterparty_name),
    counterparty_name: receiverName || text(normalizedArgs.counterparty_name || normalizedArgs.via_holder_name),
    top_n: normalizedArgs.top_n || 20,
    include_debug: false
  }, { signal });
  throwIfAborted(signal);
  return {
    ...response,
    tool: "investigate_pair_amount",
    skill_id: "investigate_pair_amount",
    routed_from_tool: "funds_investigate",
    answer_card: response?.answer_card
	      ? {
	          ...response.answer_card,
	          support_owner: "pair-amount-investigation",
	          focused_owner: "pair-amount-investigation",
	          funds_investigate_support_only: true
	        }
      : response?.answer_card
  };
}

function transientToolError(error) {
  const message = text(error?.message || error);
  return TRANSIENT_TOOL_ERROR_PATTERN.test(message);
}

function caseSourceBlockerPayload({ name, args = {}, error }) {
  const explicitCaseId = text(args.case_id || args.caseId || error?.caseId);
  const code = text(error?.code) || "CASE_SOURCE_BLOCKER";
  const invalidExplicit = code === "INVALID_CASE_SOURCE";
  const reason = invalidExplicit
    ? "指定 Analytix 案件编号不属于当前案件项目或不存在。"
    : "Analytix 案件项目上下文尚未就绪。";
  const blocker = {
    source_guardrail: "required_case_source_of_truth_missing",
    status: "blocked",
    error_code: invalidExplicit ? "INVALID_CASE_SOURCE" : "CASE_SOURCE_BLOCKER",
	    reason,
	    source_chain: [
	      "当前对话工作区",
	      ".analytix/case-project.json",
	      "插件运行环境上下文",
	      "数据服务案件接口",
	      "案件事实核验环节"
	    ],
    failed_stage: text(error?.stage) || (invalidExplicit ? "case_project_scope_guard" : "case_project_context"),
    explicit_case_id: explicitCaseId || undefined,
    backend_details: {
      status: Number(error?.details?.backend_status || 0) || undefined,
      code: text(error?.details?.backend_error_code) || undefined
    },
    recovery_actions: CASE_SOURCE_RECOVERY_ACTIONS,
    forbidden_actions: CASE_SOURCE_FORBIDDEN_ACTIONS
  };
  return {
    tool: name,
    skill_id: skillToolMap.get(name) || name,
    case_id: explicitCaseId || undefined,
    status: "blocked",
    error_code: blocker.error_code,
    source_guardrail: blocker.source_guardrail,
    case_source_blocker: blocker,
    key_facts: {
      case_source_blocker: blocker
    },
	    warnings: [
	      "当前案件项目权威来源缺失；不得从本机文件、历史输出、测试样例或猜测案件编号作答。"
	    ],
	    answer_card: {
	      card_type: "case_source_blocker",
	      title: "案件项目来源未就绪",
      tool: name,
      status: "blocked",
      ...blocker
    }
  };
}

function sleep(ms, signal) {
  throwIfAborted(signal);
  return new Promise((resolve, reject) => {
    let settled = false;
    const finish = (operation) => {
      if (settled) return;
      settled = true;
      signal?.removeEventListener("abort", onAbort);
      operation();
    };
    const timer = setTimeout(() => finish(resolve), ms);
    const onAbort = () => {
      clearTimeout(timer);
      finish(() => reject(abortError(signal, "tool retry cancelled")));
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    if (signal?.aborted) onAbort();
  });
}

async function withTransientRetry(label, operation, attempts = 2, signal) {
  let lastError;
  for (let index = 0; index < attempts; index += 1) {
    throwIfAborted(signal);
    try {
      const result = await operation();
      throwIfAborted(signal);
      return result;
    } catch (error) {
      throwIfAborted(signal);
      lastError = error;
      if (index + 1 >= attempts || !transientToolError(error)) break;
      await sleep(250 * (index + 1), signal);
    }
  }
	  const message = text(lastError?.message || lastError);
	  if (message && lastError && typeof lastError === "object") {
	    lastError.message = `${label} 重试后仍未完成：${message}`;
	    throw lastError;
	  }
	  throw new Error(message ? `${label} 重试后仍未完成：${message}` : `${label} 重试后仍未完成`);
	}

function reviewEntryText(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return text(value);
  const source = objectOf(value);
  const id = text(source.claim_id || source.claimId || source.id);
  const body = text(source.text || source.claim || source.statement || source.summary);
  const status = text(source.status);
  const reason = text(source.reason || source.message);
  const factRefs = arrayOf(source.fact_refs).map(text).filter(Boolean);
  const missingFactRefs = arrayOf(source.missing_fact_refs).map(text).filter(Boolean);
  const prefix = [id, body].filter(Boolean).join("：");
  const details = [
    status ? `status=${status}` : "",
    factRefs.length ? `fact_refs=${factRefs.join(",")}` : "",
    missingFactRefs.length ? `missing_fact_refs=${missingFactRefs.join(",")}` : "",
    reason
  ].filter(Boolean).join("；");
  if (prefix && details) return `${prefix}；${details}`;
  return prefix || details;
}

function reviewEntryTexts(value) {
  return arrayOf(value).map(reviewEntryText).filter(Boolean);
}

function backendClaimCandidate(value, origin, index) {
  const source = objectOf(value);
  const body = reviewEntryText(value);
  if (!body) return undefined;
  return {
    ...source,
    claim_id: text(source.claim_id || source.claimId || source.id) || `${origin}.${index + 1}`,
    text: text(source.text || source.claim || source.statement || source.summary) || body,
    asserted_support_status: text(source.support_status || source.status) || undefined,
    status: "candidate",
    support_status: "candidate",
    candidate_origin: origin
  };
}

function backendClaimCandidates(data) {
  const source = objectOf(data);
  const groups = [
    ["backend_verified_assertion", source.verified_claims],
    ["backend_supported_assertion", source.supported_claims],
    ["backend_checked_supported_assertion", arrayOf(source.checked_claims)
      .filter((claim) => text(objectOf(claim).status || objectOf(claim).support_status) === "supported")]
  ];
  return groups.flatMap(([origin, values]) => arrayOf(values)
    .map((value, index) => backendClaimCandidate(value, origin, index))
    .filter(Boolean));
}

function unsupportedNumberClaims(data) {
  return arrayOf(objectOf(data).unsupported_numbers)
    .map(text)
    .filter(Boolean)
    .map((number) => `unsupported_number: ${number} 未在 facts 中出现，不能写成报告事实。`);
}

function reportReviewBody(args = {}, claims = []) {
  return [
    text(args.report_text || args.reportText || args.question),
    ...claims.map((claim) => {
      const source = objectOf(claim);
      return text(source.text || source.claim || source.statement || claim);
    })
  ].filter(Boolean).join("\n");
}

function buildClaimReviewDeterministicAugments({ caseId, body }) {
  const correctedClaims = [];
  const unsupportedFlows = [];
  const missingSourceBoundaries = [];
  const nextReviewActions = [];
  const warnings = [];
  if (!caseId || !body) {
    return { correctedClaims, unsupportedFlows, missingSourceBoundaries, nextReviewActions, warnings };
  }

  if (/fetch failed|run_full_case_analysis\s*\(\s*write_report\s*=\s*true\s*\)/iu.test(body)) {
    missingSourceBoundaries.push("既有报告自述 fetch failed；未形成确定性交易边前，不能把既有报告当完整报告级结论。");
    unsupportedFlows.push("既有报告中的资金链路片段只能作为聚合线索；没有已有数据支持的确定性资金边时，不得画资金流向图或写成确定路径。");
  }

  if (correctedClaims.length || unsupportedFlows.length || missingSourceBoundaries.length) {
    nextReviewActions.push("报告复用前先补齐事实来源、金额统计范围、核验状态和交易级可证实资金边；未取得确定性交易边的路径只能写线索和补证动作。");
  }

  return { correctedClaims, unsupportedFlows, missingSourceBoundaries, nextReviewActions, warnings };
}

function localClaimReviewRiskWarnings(reportText) {
  const body = text(reportText);
  const warnings = [];
  if (/```mermaid|flowchart|graph\s+(?:TD|LR|RL|BT)|-->|→|=>/iu.test(body)) {
    warnings.push("unsupported_mermaid_or_arrow_flow: 报告含资金流向图、流程图或箭头链路，但没有确定性资金边时不得写成已确认流向。");
  }
  return warnings;
}

const CONTROLLED_CASE_WORKBENCH_TOOLS = new Set([
  "inspect_case_schema",
  "run_case_sql",
  "explain_case_sql",
  "diagnose_case_sql",
  "count_case_rows",
  "profile_case_schema",
  "preview_case_rows",
  "inspect_workbench_history",
  "case_sql_recipes",
  "create_case_notebook"
]);
const CASE_WORKBENCH_PURPOSE_REQUIRED_TOOLS = new Set([
  "run_case_sql",
  "create_case_notebook",
  "explain_case_sql",
  "diagnose_case_sql",
  "preview_case_rows"
]);
const CASE_WORKBENCH_SQL_REQUIRED_TOOLS = new Set([
  "run_case_sql",
  "explain_case_sql"
]);
const FORBIDDEN_CASE_SQL_CHECKS = [
  {
    test: (sql) => /\b(drop|delete|insert|update|alter|create|attach|detach|copy|export|pragma|install|load|vacuum|truncate|merge|replace)\b/iu.test(sql),
    reason: "DDL/DML/extension/export statements are not allowed"
  },
  {
    test: (sql) => /\bselect\s+\*/iu.test(sql) && !/\blimit\s+\d+\b/iu.test(sql),
    reason: "unbounded SELECT * is not allowed; request explicit columns, aggregates, or a bounded preview"
  },
  {
    test: (sql) => /\bfc_[a-z0-9_]*_raw\b/iu.test(sql),
    reason: "raw/source tables are not allowed in ordinary fund-analysis workbench queries"
  },
  {
    test: (sql) => /\b(read_csv|read_parquet|read_json|read_text|httpfs|secret|token|s3_|http_|https?:|file:)\b/iu.test(sql),
    reason: "external file/network/secret access is not allowed"
  },
  {
    test: (sql) => /(?:\.\.\.|…)/u.test(sql),
    reason: "SQL must be complete; ellipsis/placeholders are not executable"
  }
];

function caseWorkbenchPurpose(name, args) {
  if (name === "create_case_notebook") return text(args.analysis_goal || args.analysisGoal || args.title);
  if (name === "profile_case_schema") return text(args.purpose) || "字段覆盖、空值率和取值范围现场勘查";
  if (name === "count_case_rows") return text(args.purpose) || "统计当前案件指定清洗/分析表记录数";
  if (name === "inspect_workbench_history") return text(args.purpose) || "读取最近专项资金核算历史摘要";
  if (name === "case_sql_recipes") return text(args.purpose) || "读取经审核专项资金核算模板目录";
  return text(args.purpose || args.query_request || args.queryRequest);
}

function caseWorkbenchSqlTexts(name, args) {
  const values = [];
  const directSql = text(args.sql);
  if (directSql) values.push(directSql);
  if (name === "create_case_notebook") {
    for (const cell of arrayOf(args.cells)) {
      const sql = text(objectOf(cell).sql);
      if (sql) values.push(sql);
    }
  }
  return values;
}

function normalizeFullCaseAnalysisArgs(args = {}) {
  const effectiveArgs = { ...objectOf(args) };
  if (effectiveArgs.include_internal_playbooks === undefined) {
    effectiveArgs.include_internal_playbooks = false;
  }
  return effectiveArgs;
}

function publicationReceiptRequiredPayload({ name, skillId }) {
  const reason = "正式报告发布已被阻断：当前宿主尚未为本轮同案、同快照且逐项通过 claim gate 的报告签发有效 PublicationReceipt。";
  return {
    tool: name,
    skill_id: skillId,
    status: "blocked",
    semantic_status: "blocked",
    error_code: "PUBLICATION_RECEIPT_REQUIRED",
    reason,
    error_message: reason,
    isError: true,
    safeToAnswer: false,
    safe_to_answer_current_task: false,
    write_blocked: true,
    report_gate_status: "publication_receipt_required",
    prohibited_claims: [
      "不得把报告文件存在、可打开、已渲染或模型自报的 receipt ID 当作正式发布许可。",
      "不得在缺少宿主权威 PublicationReceipt 时返回正式报告路径、manifest、下载链接或已出具表述。"
    ],
    warnings: [reason],
    answer_card: {
      card_type: "report_publication_blocker",
      title: "正式报告暂不发布",
      tool: name,
      status: "blocked",
      error_code: "PUBLICATION_RECEIPT_REQUIRED",
      write_blocked: true,
      safe_to_answer_current_task: false,
      reason,
      next_actions: [
        "当前 P0 期间不执行 full-case 报告分析；仅使用已广告的只读核验工具补齐来源、范围和逐项证据。",
        "由宿主逐 claim 验证证据、数据快照、PII 投影和渲染结果后，再走原子发布流程。"
      ]
    }
  };
}

function normalizeCaseTitleFragment(value) {
  return text(value)
    .replace(/\s+/gu, "")
    .replace(/(?:资金)?案件(?:项目)?分析$/u, "")
    .replace(/案件(?:项目)?$/u, "")
    .replace(/分析$/u, "");
}

function caseProjectTitleFragments(args = {}, env = {}) {
  const projectContext = resolveCaseProjectContext(args, env);
  const workspaceRoot = text(projectContext?.workspace_root || projectContext?.workspaceRoot || env.ANALYTIX_CASE_PROJECT_ROOT || env.ANALYTIX_WORKSPACE_ROOT);
  const baseName = workspaceRoot ? path.basename(workspaceRoot) : "";
  const normalizedBase = normalizeCaseTitleFragment(baseName);
  return [...new Set([
    text(baseName).replace(/\s+/gu, ""),
    normalizedBase,
    normalizedBase ? `${normalizedBase}案件` : "",
    normalizedBase ? `${normalizedBase}案件分析` : ""
  ].filter((item) => item && item.length >= 2))];
}

function stripCaseProjectTitleHolderFilter(name, args = {}, env = {}) {
  if (!RANKING_TOOLS.has(name)) return args;
  const holderName = text(args.holder_name || args.holderName).replace(/\s+/gu, "");
  if (!holderName) return args;
  const fragments = caseProjectTitleFragments(args, env);
  const isCaseTitleFragment = fragments.some((fragment) => (
    holderName === fragment ||
    normalizeCaseTitleFragment(holderName) === normalizeCaseTitleFragment(fragment)
  ));
  if (!isCaseTitleFragment) return args;
  const next = { ...args };
  delete next.holder_name;
  delete next.holderName;
  if (text(next.match_mode || next.matchMode) === "contains") {
    delete next.match_mode;
    delete next.matchMode;
  }
  return next;
}

function runtimeContextFromArgs(args = {}) {
  const source = objectOf(args);
  return objectOf(source._analytix || source.__analytix || source.analytix_runtime_context);
}

function runtimeDataRoot(env = {}) {
  const configured = text(env.ANALYTIX_DATA_DIR || env.ANALYTIX_RUNTIME_DATA_DIR);
  if (configured) return path.resolve(configured);
  const home = text(env.HOME) || os.homedir();
  return path.join(home, ".analytix", "data");
}

function readCurrentTurnUserMessage(args = {}, env = {}) {
  const context = runtimeContextFromArgs(args);
  const threadId = text(context.threadId || context.thread_id);
  const turnId = text(context.turnId || context.turn_id);
  if (!/^[A-Za-z0-9_-]{4,120}$/u.test(threadId) || !/^[A-Za-z0-9_-]{4,120}$/u.test(turnId)) return "";
  const eventsPath = path.join(runtimeDataRoot(env), "threads", threadId, "events.jsonl");
  try {
    const lines = fs.readFileSync(eventsPath, "utf8").trim().split(/\r?\n/u);
    for (const line of lines) {
      if (!line) continue;
      const event = JSON.parse(line);
      const item = objectOf(event.item);
      if (event.turnId === turnId && item.kind === "user_message") {
        return text(item.text || event.prompt);
      }
    }
  } catch {
    return "";
  }
  return "";
}

function explicitTurnoverIntent(value) {
  return /(?:交易总额|往来总额|双向|入账\+出账|进出合计|不区分(?:流入|流出|进出|方向)|总流量|turnover)/iu.test(text(value));
}

function outflowCounterpartyIntent(value) {
  const question = text(value);
  if (!question || explicitTurnoverIntent(question)) return false;
  return /(?:资金)?(?:流向|去向|流出|转出|出账|付款|支付|付给|转给|打给|汇给).{0,24}(?:前\s*\d+|top\s*\d+|前十|前十五|前二十|对手方|收款方|对象|户名|是谁|谁)|(?:前\s*\d+|top\s*\d+|前十|前十五|前二十).{0,24}(?:资金)?(?:流向|去向|出账|付款|收款方|对手方)/iu.test(question);
}

function normalizeRankingIntentArgs(name, args = {}, env = {}) {
  if (name !== "rank_counterparties") return args;
  const directQuestion = text(args.question || args.query || args.prompt || args.analysis_goal);
  const runtimeQuestion = readCurrentTurnUserMessage(args, env);
  const question = directQuestion || runtimeQuestion;
  if (!outflowCounterpartyIntent(question)) return args;
  return {
    ...args,
    metric: "outflow",
    direction_mode: "out",
    counterparty_group_mode: text(args.counterparty_group_mode || args.counterpartyGroupMode) || "name"
  };
}

function normalizeRuntimeToolArgs(name, args = {}, env = {}) {
  return normalizeRankingIntentArgs(
    name,
    stripCaseProjectTitleHolderFilter(name, normalizeSkillArgs(name, args), env),
    env
  );
}

function forbiddenCaseSqlReason(sqlTexts) {
  for (const sql of sqlTexts) {
    try {
      analyzeDuckdbReadOnlySql(sql);
    } catch {
      return "SQL 未通过当前案件只读正向安全策略。";
    }
    for (const { test, reason } of FORBIDDEN_CASE_SQL_CHECKS) {
      if (test(sql)) return reason;
    }
  }
  return "";
}

function forbiddenPreviewWhereReason(whereSql) {
  const clause = text(whereSql)
    .replace(/\/\*[\s\S]*?\*\//gu, " ")
    .replace(/--[^\r\n]*/gu, " ")
    .trim();
  if (!clause || /^(?:1\s*=\s*1|true)$/iu.test(clause)) {
    return "preview_case_rows requires a narrow where_sql filter; unfiltered samples are not allowed.";
  }
  if (
    /\b(drop|delete|insert|update|alter|create|attach|detach|copy|export|pragma|install|load|vacuum|truncate|merge|replace)\b/iu.test(clause) ||
    /;\s*\S/u.test(clause) ||
    /(?:\.\.\.|…)/u.test(clause)
  ) {
    return "preview_case_rows where_sql must be a safe single filtered predicate.";
  }
  return "";
}

function workbenchCard({ name, args, status, reason, warnings = [], nextActions = [] }) {
  const purpose = caseWorkbenchPurpose(name, args);
  return {
    card_type: "case_workbench_card",
    intent: "controlled_case_workbench",
    tool: name,
    status,
    requested_purpose: purpose || undefined,
    case_id: text(args.case_id || args.caseId) || undefined,
    fact_source: "analytix_funds_mcp_controlled_case_workbench",
    key_facts: {
      boundary: [
        "current Analytix case only",
        "read-only controlled backend execution or Analytix-owned local DuckDB fallback only",
        "cleaned fc_*_norm and analysis_* scope only",
        "bounded rows; row_limit <= 500",
        "validated evidence boundary required"
      ],
      reason
    },
    warnings,
    next_actions: nextActions
  };
}

function stableHash(value) {
  return crypto
    .createHash("sha256")
    .update(JSON.stringify(value || {}, (_key, item) => (typeof item === "bigint" ? String(item) : item)))
    .digest("hex");
}

function runCaseSqlSourceScope(sql) {
  const source = text(sql).replace(/\/\*[\s\S]*?\*\//gu, " ").replace(/--[^\r\n]*/gu, " ");
  const output = [];
  const seen = new Set();
  for (const match of source.matchAll(/\b(?:from|join)\s+(?:"([^"]+)"|([A-Za-z_][A-Za-z0-9_.]*))/giu)) {
    const table = text(match[1] || match[2]).replace(/"/gu, "").split(".").pop().toLowerCase();
    if (!table || seen.has(table)) continue;
    if (!/^analysis_[a-z0-9_]*$/u.test(table) && !/^fc_[a-z0-9_]*_norm$/u.test(table)) continue;
    seen.add(table);
    output.push(table);
  }
  return output;
}

function runCaseSqlEvidenceCard({ args = {}, payload = {} }) {
  const source = objectOf(args);
  const data = skillEnvelopeData(payload);
  const response = objectOf(payload);
  const sql = text(source.sql);
  const queryId = text(data.query_id || response.query_id);
  const sourceScope = arrayOf(data.source_scope).length ? arrayOf(data.source_scope).map(text).filter(Boolean) : runCaseSqlSourceScope(sql);
  const rowLimit = Number(source.row_limit || data.row_limit || 100) || 100;
  const rowCount = intOrUndefined(data.row_count ?? response.row_count);
  const truncatedValue = data.truncated ?? response.truncated;
  const truncated = typeof truncatedValue === "boolean" ? truncatedValue : undefined;
  const sourceHash = text(data.source_hash || response.source_hash);
  const missingFields = [
    !queryId ? "query_id" : "",
    !sourceScope.length ? "source_scope" : "",
    rowCount === undefined || rowCount < 0 ? "row_count" : "",
    truncated === undefined ? "pagination_completeness" : "",
    !/^[0-9a-f]{64}$/iu.test(sourceHash) ? "source_sha256" : "",
    "host_evidence_receipt"
  ].filter(Boolean);
  const numericRowCount = rowCount !== undefined && rowCount >= 0 ? rowCount : undefined;
  return {
    card_type: "case_workbench_evidence_card",
    fact_id: queryId ? `workbench:${queryId.split(":").pop()}` : undefined,
    support_status: "unsupported",
    fact_answer_allowed: false,
    missing_fields: missingFields,
    support_query_name: "run_case_sql",
    source_level: "controlled_case_workbench",
    source_hash: /^[0-9a-f]{64}$/iu.test(sourceHash) ? sourceHash : undefined,
    source_refs: queryId ? { query_ids: [queryId] } : undefined,
    query_id: queryId || undefined,
    metric_scope: {
      purpose: caseWorkbenchPurpose("run_case_sql", source),
      source_scope: sourceScope,
      allowed_view_policy: "cleaned_and_analysis_only",
      result_mode: text(source.result_mode || source.resultMode || data.result_mode || "preview"),
      row_limit: rowLimit,
      row_count: numericRowCount,
      truncated,
      raw_rows_exposed: false
    },
    validation_state: {
      status: truncated === true ? "partial" : "candidate",
      current_case_only: true,
      readonly: true,
      cleaned_analysis_scope_only: true,
      bounded_row_limit: true,
      raw_rows_exposed: false,
      report_grade_claim_requires_owner_review: true,
      host_evidence_receipt_required: true
    },
    evidence_boundary: "专项资金核算结果只支持本次 SQL 明确限定的当前案件、清洗/分析范围；写入报告前仍需 focused owner 复核口径、表后研判和不能认定事项。",
    next_review_actions: [
      "将该结果与语义事实或既有核验结果做 source-of-truth selection。",
      "如用于报告级金额、链路或图表，补充异常特征、证明价值、暂不能认定事项和下一步调取材料。"
    ]
  };
}

function workbenchRecoveryActions() {
  return [
    "先用 inspect_case_schema 查看当前案件允许的 fc_*_norm 与 analysis_* 表名。",
    "行数问题优先用 count_case_rows，并传入允许表名，例如 analysis_txn_detail_idx。",
    "字段覆盖、空值率和取值范围先用 profile_case_schema 做现场勘查。",
    "自定义口径先用 case_sql_recipes 选择经审核模板，必要时用 explain_case_sql 做 parser/binder 预检。",
    "自定义聚合用 run_case_sql，SQL 只能访问当前案件清洗表或分析索引。",
    "如 SQL 或表名被拒绝，改用 diagnose_case_sql 修正口径后重试。",
    "需要可回放附件时，最后用 create_case_notebook 记录同一口径、核验意见和交付边界。"
  ];
}

function workbenchRecommendedTools() {
  return [
    "get_scope_coverage",
    "inspect_case_schema",
    "count_case_rows",
    "profile_case_schema",
    "case_sql_recipes",
    "explain_case_sql",
    "diagnose_case_sql",
    "run_case_sql",
    "create_case_notebook"
  ];
}

function workbenchRecommendedLadder() {
  return [
    "get_scope_coverage",
    "inspect_case_schema",
    "count_case_rows",
    "profile_case_schema",
    "case_sql_recipes",
    "explain_case_sql",
    "run_case_sql",
    "diagnose_case_sql",
    "create_case_notebook"
  ];
}

function semanticInsufficientProtocol({ name, args = {}, error, missingCapability = "" }) {
  const skillId = skillToolMap.get(name) || name;
  const message = "当前能力未返回可由宿主验证的案件事实结果。";
  return {
    failed_surface: name,
    missing_capability: missingCapability || skillId,
    prohibited_claims: [
      "不得编造语义工具未返回的金额、笔数、排行、画像、链路、图谱、报告或法律定性。",
      "不得使用历史输出、旧报告、旧 cache、本机文件扫描或其他案件结果替代当前案件 DuckDB 现场。",
      "不得把 preview、排行候选、dashboard metadata 或 capability gap 文本写成最终金额事实。"
    ],
    recommended_ladder: workbenchRecommendedLadder(),
    safe_to_answer_current_task: false,
    productization_target: `若 ${skillId} 多次缺口，应产品化为 Analytix 当前案件语义核验能力，并补充 MCP oracle/front-door canary。`,
    reason: message,
    case_id: text(args.case_id || args.caseId) || undefined
  };
}

function topNRuntimeContract({ requestedLimit, resolvedLimit, returnedCount, filteredCount }) {
  const requestedValue = intOrUndefined(requestedLimit);
  const resolvedValue = intOrUndefined(resolvedLimit);
  const returnedValue = intOrUndefined(returnedCount);
  if (requestedValue === undefined || requestedValue < 1 || resolvedValue === undefined || resolvedValue < 1 || returnedValue === undefined || returnedValue < 0) {
    return undefined;
  }
  const requested = requestedValue;
  const resolved = resolvedValue;
  const returned = returnedValue;
  const filtered = intOrUndefined(filteredCount);
  const dataExhausted = returned < requested;
  return {
    requested_limit: requested,
    resolved_limit: resolved,
    returned_count: returned,
    visible_count: returned,
    filtered_count: filtered !== undefined && filtered >= 0 ? filtered : undefined,
    data_exhausted: dataExhausted,
    truncation_reason: dataExhausted ? "source_data_exhausted_before_requested_limit" : "none",
    compact_text_row_count: returned,
    final_answer_expected_min_rows: Math.min(requested, returned)
  };
}

function blockedCaseWorkbenchPayload({ name, args, reason, code = "CONTROLLED_CASE_WORKBENCH_BLOCKED" }) {
  const recoveryActions = workbenchRecoveryActions();
  const recommendedTools = workbenchRecommendedTools();
  const insufficient = semanticInsufficientProtocol({
    name,
    args,
    error: reason,
    missingCapability: "controlled_case_workbench_policy"
  });
  return {
    tool: name,
    skill_id: name,
    case_id: text(args.case_id || args.caseId) || undefined,
    status: "blocked",
    error_code: code,
    reason,
    error_message: reason,
    insufficient_protocol: insufficient,
    recommended_ladder: insufficient.recommended_ladder,
    safe_to_answer_current_task: false,
    key_facts: {
      workbench_gap: {
        status: "blocked",
        error_code: code,
        error_message: reason,
        recovery_actions: recoveryActions,
        recommended_workbench_tools: recommendedTools,
        ...insufficient
      }
    },
    answer_card: workbenchCard({
      name,
      args,
      status: "blocked",
      reason,
      warnings: [reason],
      nextActions: [
        "优先使用已覆盖该问题的语义核验能力。",
        "如需自定义分析，应限定在当前案件、只读、清洗表或分析索引范围内重新表述。",
        "返回数据来源与口径边界、核验状态、核验意见和未解决的能力缺口，不输出弱来源事实。"
      ]
    })
  };
}

function capabilityGapCaseWorkbenchPayload({ name, args, error }) {
  const diagnostic = publicDuckdbDiagnostic(error, { sql: text(args.sql), stage: "execute" });
  const reason = `专项资金核算能力暂不可用：${diagnostic.publicMessage}`;
  const recoveryActions = workbenchRecoveryActions();
  const recommendedTools = workbenchRecommendedTools();
  const insufficient = semanticInsufficientProtocol({
    name,
    args,
    error: diagnostic.publicMessage,
    missingCapability: "controlled_case_workbench"
  });
  return {
    tool: name,
    skill_id: name,
    case_id: text(args.case_id || args.caseId) || undefined,
    status: "capability_gap",
    error_code: "CONTROLLED_CASE_WORKBENCH_UNAVAILABLE",
    reason,
    error_message: reason,
    duckdb_diagnostic: diagnostic,
    insufficient_protocol: insufficient,
    recommended_ladder: insufficient.recommended_ladder,
    safe_to_answer_current_task: false,
    key_facts: {
      workbench_gap: {
        status: "capability_gap",
        error_code: "CONTROLLED_CASE_WORKBENCH_UNAVAILABLE",
        error_message: reason,
        recovery_actions: recoveryActions,
        recommended_workbench_tools: recommendedTools,
        ...insufficient
      }
    },
    answer_card: workbenchCard({
      name,
      args,
      status: "capability_gap",
      reason,
      warnings: [
        "不得编造自定义查询结果。",
        "不得把弱来源、来源明细行、未复核计算或缺少交付物的内容写成确定事实。"
      ],
      nextActions: [
        "向用户说明缺少的语义核验能力和核验意见。",
        "重复出现的自定义分析应产品化为新的后端语义核验能力，并补测试和证据合同覆盖。"
      ]
    })
  };
}

function blockedLocalDuckdbPolicyPayload({ name, skillId, args = {}, error }) {
  const diagnostic = publicDuckdbDiagnostic(error, { sql: text(args.sql), stage: "policy" });
  const reason = diagnostic.publicMessage;
  const sqlPolicy = objectOf(error?.sql_policy);
  const insufficient = semanticInsufficientProtocol({
    name,
    args,
    error: reason,
    missingCapability: "sql_policy_preflight"
  });
  return {
    tool: name,
    skill_id: skillId,
    case_id: text(args.case_id || args.caseId) || undefined,
    status: "blocked",
    error_code: text(error?.code) || "LOCAL_DUCKDB_SQL_POLICY_BLOCKED",
    backend_status: "local_duckdb_policy_blocked",
    reason,
    duckdb_diagnostic: diagnostic,
    sql_policy: sqlPolicy,
    insufficient_protocol: insufficient,
    recommended_ladder: insufficient.recommended_ladder,
    safe_to_answer_current_task: false,
    key_facts: {
      workbench_blocked: {
        status: "blocked",
        error_code: text(error?.code) || "LOCAL_DUCKDB_SQL_POLICY_BLOCKED",
        sql_policy: sqlPolicy,
        ...insufficient,
        recovery_actions: [
          "先用 inspect_case_schema/profile_case_schema 复核 SQL-safe 表名和字段名。",
          "字段或表名修正后，再重新执行 run_case_sql 或 explain_case_sql。",
          "不得把被 parser/binder 拦截的 SQL 写成案件金额、笔数、排行或资金链路事实。"
        ]
      }
    },
    warnings: [
      reason,
      "该结果表示 SQL 被当前案件只读策略或 DuckDB parser/binder 拦截，不是数据服务能力缺失。"
    ],
    answer_card: workbenchCard({
      name,
      args,
      status: "blocked",
      reason,
      warnings: [
        "SQL 未通过当前案件 DuckDB parser/binder 预检。",
        "不得编造自定义查询结果。"
      ],
      nextActions: [
        "用 inspect_case_schema/profile_case_schema 核对 SQL-safe 名称。",
        "修正字段、表名、过滤条件或聚合口径后重试。"
      ]
    }),
    response: {
      status: "blocked",
      error_code: text(error?.code) || "LOCAL_DUCKDB_SQL_POLICY_BLOCKED",
      data: {
        execution_status: "blocked",
        sql_policy: sqlPolicy,
        validation_state: {
          status: "blocked",
          current_case_only: true,
          readonly: true,
          cleaned_analysis_scope_only: true,
          sql_policy: sqlPolicy,
          report_grade_claim_requires_owner_review: true
        },
        raw_rows_exposed: false
      },
      warnings: [reason],
      error: reason
    }
  };
}

function typedLocalDuckdbFailurePayload({ name, skillId = name, args = {}, error }) {
  const diagnostic = publicDuckdbDiagnostic(error, { sql: text(args.sql), stage: "execute" });
  if (["sql_policy_rejected", "sql_parse_rejected", "sql_bind_rejected"].includes(diagnostic.code)) {
    return blockedLocalDuckdbPolicyPayload({ name, skillId, args, error });
  }
  return capabilityGapCaseWorkbenchPayload({ name, args, error });
}

function semanticToolCapabilityGapPayload({ name, args = {}, error }) {
  const skillId = skillToolMap.get(name) || name;
  const message = "语义事实服务暂不可用；错误详情未进入模型、聊天或报告。";
  const insufficient = semanticInsufficientProtocol({ name, args, error: message, missingCapability: skillId });
  const recommendedTools = workbenchRecommendedTools();
  return {
    tool: name,
    skill_id: skillId,
    case_id: text(args.case_id || args.caseId) || undefined,
    status: "capability_gap",
    error_code: "SEMANTIC_TOOL_UNAVAILABLE",
    failed_surface: insufficient.failed_surface,
    missing_capability: insufficient.missing_capability,
    prohibited_claims: insufficient.prohibited_claims,
    recommended_ladder: insufficient.recommended_ladder,
    safe_to_answer_current_task: false,
    productization_target: insufficient.productization_target,
    key_facts: {
      semantic_tool_unavailable: true,
      failed_tool: name,
      failed_skill: skillId,
      current_case_duckdb_workbench_available: true,
      recommended_workbench_tools: recommendedTools,
      ...insufficient
    },
    warnings: [
      `语义事实工具 ${name} 暂不可用：${message}`,
      "不得编造该语义工具本应返回的画像、链路、证据包或报告结论。",
      "当前案件 DuckDB 仍可通过受控 Workbench 做 schema、row count、profile、EXPLAIN、只读 SQL 和 history 核验。"
    ],
    answer_card: {
      card_type: "semantic_tool_capability_gap_card",
      title: "语义事实工具暂不可用",
      tool: name,
      skill_id: skillId,
      status: "capability_gap",
      failed_reason: message,
      fact_source: "analytix_funds_mcp_semantic_gap",
      current_case_duckdb_workbench_available: true,
      recommended_workbench_tools: recommendedTools,
      recommended_ladder: insufficient.recommended_ladder,
      safe_to_answer_current_task: false,
      prohibited_claims: insufficient.prohibited_claims,
      productization_target: insufficient.productization_target,
      next_actions: [
        "先用 get_scope_coverage 或 inspect_case_schema 确认当前案件数据范围。",
        "行数、排行、金额或有界口径问题可用 count_case_rows、rank_*、case_sql_recipes 和 run_case_sql 复核。",
        "图谱、画像、报告级结论在语义工具恢复或专项核算补齐前只能写来源边界和补证动作。"
      ],
      evidence_boundary: "本卡只说明语义工具不可用及可替代核验路径，不支持任何案件金额、链路、画像或法律结论。"
    }
  };
}

export function createToolCallRuntime({
  fundsInvestigate,
  getCaseGraph,
  buildFundFlowGraph,
  resolveCase,
  compactCaseRecord,
  compactSkillsRecord,
  httpJson,
  getImportOverview,
  getCleaningOverview,
  getCaseDataPipelineOverview,
  getCaseScopeMap,
  getStatsMeta,
  getStatsTree,
  queryStatsRows,
  queryStatsTxnRows,
  queryAccountTxnRows,
  getAnalysisDashboard,
  exportCleanedCaseData,
  executeSkill,
  env = {}
}) {
  function preExecutionWriteQuarantinePayload(name) {
    return {
      tool: name,
      skill_id: skillToolMap.get(name) || name,
      status: "blocked",
      semantic_status: "blocked",
      error_code: "P0_PRE_EXECUTION_BLOCKED",
      blocker: "pre_execution_write_quarantine",
      isError: true,
      safeToAnswer: false,
      safe_to_answer_current_task: false,
      write_blocked: true,
      data: {},
      warnings: [
        "当前写入能力在宿主 PublicationReceipt 链完成前保持隔离。",
        "不得执行后端、本地 fallback、staging、附件或文件写入。"
      ]
    };
  }

  async function localDuckdbFallbackPayload({ name, skillId, args = {}, error, controlled = false, signal }) {
    throwIfAborted(signal);
    if (isCaseSourceError(error)) return null;
    if (skillId === "run_full_case_analysis") return null;
    if (text(env.ANALYTIX_FUNDS_DISABLE_LOCAL_DUCKDB_FALLBACK) === "1") return null;
    if (!supportsLocalDuckdbWorkbenchSkill(skillId)) return null;
    const effectiveArgs = normalizeRuntimeToolArgs(name, args, env);
    try {
      const payload = await executeLocalDuckdbWorkbenchSkill(skillId, effectiveArgs, { env, signal });
      throwIfAborted(signal);
      const data = skillEnvelopeData(payload);
      const caseId = text(effectiveArgs.case_id || effectiveArgs.caseId || data.case_id || payload.case_id);
      const reason = `${controlled ? "专项资金核算后端" : "语义事实后端"}不可用，已使用当前案件 DuckDB 本地只读 fallback。`;
      const fallbackWarning = `${reason} 后端错误详情未进入模型、聊天或报告。`;
      if (controlled) {
        const evidenceCard = name === "run_case_sql"
          ? runCaseSqlEvidenceCard({ args: effectiveArgs, payload })
          : undefined;
        return {
          tool: name,
          skill_id: skillId,
          case_id: caseId || undefined,
          status: "ok",
          backend_status: "local_duckdb_fallback",
          answer_card: workbenchCard({
            name,
            args: effectiveArgs,
            status: "ok",
            reason,
            warnings: [fallbackWarning],
            nextActions: [
              "需要图表或表格时，交给证据图表附件流程。",
              "写入报告级表述前，先做报告事实结论复核。"
            ]
          }),
          evidence_card: evidenceCard,
          warnings: [fallbackWarning],
          response: evidenceCard ? {
            ...objectOf(payload),
            evidence_card: objectOf(payload).evidence_card || evidenceCard,
            data: {
              ...objectOf(objectOf(payload).data),
              evidence_card: objectOf(objectOf(payload).data).evidence_card || evidenceCard,
              source_hash: text(objectOf(objectOf(payload).data).source_hash) || evidenceCard.source_hash,
              metric_scope: objectOf(objectOf(payload).data).metric_scope || evidenceCard.metric_scope,
              validation_state: objectOf(objectOf(payload).data).validation_state || evidenceCard.validation_state
            }
          } : payload
        };
      }
      return {
        tool: name,
        skill_id: skillId,
        case_id: caseId || undefined,
        status: "ok",
        backend_status: "local_duckdb_fallback",
        warnings: [fallbackWarning],
        key_facts: {
          local_duckdb_fallback: true,
          current_case_only: true,
          allowed_view_policy: "cleaned_and_analysis_only",
          query_id: text(data.query_id)
        },
        response: payload
      };
    } catch (localError) {
      throwIfAborted(signal);
      if (isDuckdbDiagnosticError(localError) || localError?.sql_policy) {
        return typedLocalDuckdbFailurePayload({ name, skillId, args: effectiveArgs, error: localError });
      }
      const diagnostic = publicDuckdbDiagnostic(localError, { sql: text(effectiveArgs.sql), stage: "execute" });
      const reason = `${controlled ? "专项资金核算" : "语义事实"}后端不可用，且当前案件 DuckDB fallback 未成功：${diagnostic.publicMessage}`;
      const insufficient = semanticInsufficientProtocol({
        name,
        args: effectiveArgs,
        error: diagnostic.publicMessage,
        missingCapability: controlled ? "controlled_case_workbench_local_duckdb_fallback" : skillId
      });
      return {
        tool: name,
        skill_id: skillId,
        case_id: text(effectiveArgs.case_id || effectiveArgs.caseId) || undefined,
        status: "capability_gap",
        error_code: "LOCAL_DUCKDB_FALLBACK_FAILED",
        backend_status: "capability_gap",
        reason,
        duckdb_diagnostic: diagnostic,
        insufficient_protocol: insufficient,
        recommended_ladder: insufficient.recommended_ladder,
        safe_to_answer_current_task: false,
        warnings: [
          "后端错误详情未进入模型、聊天或报告。",
          reason,
          "不得编造 DuckDB 未返回的资金事实、链路或统计结果。"
        ],
        answer_card: controlled ? workbenchCard({
          name,
          args: effectiveArgs,
          status: "capability_gap",
          reason,
          warnings: [reason],
          nextActions: workbenchRecoveryActions()
        }) : {
          card_type: "local_duckdb_fallback_failed_card",
          title: "当前案件 DuckDB fallback 未成功",
          tool: name,
          skill_id: skillId,
          status: "capability_gap",
          failed_reason: reason,
          next_actions: workbenchRecoveryActions()
        }
      };
    }
  }

  async function localBuildFundFlowGraphFallback(args = {}, error, signal) {
    throwIfAborted(signal);
    const effectiveArgs = normalizeSkillArgs("build_fund_flow_graph", args);
    const caseId = text(effectiveArgs.case_id || effectiveArgs.caseId);
    try {
      const holderName = text(effectiveArgs.holder_name);
      const viaName = text(effectiveArgs.via_holder_name || effectiveArgs.counterparty_name);
      const topN = Math.max(1, Math.min(Number(effectiveArgs.top_n || 20) || 20, 100));
      const sourceTopN = viaName ? Math.max(topN, 50) : topN;
      const sourcePayload = await executeLocalDuckdbWorkbenchSkill("trace_subject_top_outflows", {
        ...effectiveArgs,
        holder_name: holderName,
        top_n: sourceTopN,
        trace_depth: 1
      }, { env, signal });
      const sourceData = skillEnvelopeData(sourcePayload);
      const sourceSeeds = seedRowsFromTopOutflows(sourceData.top_outflows)
        .filter((item) => seedMatchesName(objectOf(item).seed, viaName));
      let viaPayload = null;
      let viaData = {};
      let viaSeeds = [];
      if (viaName) {
        viaPayload = await executeLocalDuckdbWorkbenchSkill("trace_subject_top_outflows", {
          ...effectiveArgs,
          holder_name: viaName,
          top_n: topN,
          trace_depth: 1
        }, { env, signal });
        viaData = skillEnvelopeData(viaPayload);
        viaSeeds = seedRowsFromTopOutflows(viaData.top_outflows);
      }
      const { flowGraph, edges, incompleteEdges } = buildFundGraphFromSeedRows({
        sourceSeeds,
        viaSeeds,
        topN,
        holderName,
        viaName
      });
      const topContract = topNRuntimeContract({
        requestedLimit: topN,
        resolvedLimit: topN,
        returnedCount: edges.length,
        filteredCount: incompleteEdges.length
      });
      const queryIds = [
        ...arrayOf(sourcePayload.citations?.query_ids),
        ...arrayOf(viaPayload?.citations?.query_ids)
      ].map(text).filter(Boolean);
      return {
        tool: "build_fund_flow_graph",
        skill_id: "build_fund_flow_graph",
        case_id: caseId || text(sourceData.case_id || viaData.case_id) || undefined,
        status: incompleteEdges.length ? "partial" : "ok",
        backend_status: "local_duckdb_fallback",
        answer_card: {
          card_type: "fund_flow_graph_card",
          title: "资金流向图",
          status: incompleteEdges.length ? "partial" : "ok",
          source_holder: holderName || undefined,
          via_holder: viaName || undefined,
          ...topContract,
          edge_count: edges.length,
          incomplete_edge_count: incompleteEdges.length,
          summary: edges.length
            ? "已按当前案件 DuckDB 可见一跳交易整理资金链路；端点缺失或同名待核的交易只作为资金断点/续查线索。"
            : "当前案件 DuckDB fallback 未形成可绘制交易边，只能列资金断点和补证事项。"
        },
        key_facts: {
          graph_delivery_contract: {
            source_cleaning_boundary: "来源为当前案件清洗明细/分析索引；同事实、换卡/补卡、重复交易号、缺失对手和金额集中均需在补证事项中说明。",
            edge_boundary: "图中只画对象完整且金额、时间、付款对象、收款对象均可核验的交易；缺对象、同名待核或聚合对象写入资金断点和待核列表。"
          },
          top_n_contract: topContract,
          source_scope_stats: sourceData.scope_stats || {},
          via_scope_stats: viaPayload ? viaData.scope_stats || {} : {},
          source_seed_count: sourceSeeds.length,
          downstream_seed_count: viaSeeds.length,
          source_to_via_summary: viaName
            ? summarizeSeedTransfers(sourceSeeds, { fromLabel: holderName, toLabel: viaName })
            : undefined,
          flow_graph: flowGraph,
          fund_flow_fact_pack: {
            source_holder: holderName || undefined,
            via_holder: viaName || undefined,
            source_scope_stats: sourceData.scope_stats || {},
            via_scope_stats: viaPayload ? viaData.scope_stats || {} : {},
            edge_count: edges.length,
            supported_edge_count: edges.filter((edge) => text(edge.edge_status) === "supported").length,
            incomplete_edge_count: incompleteEdges.length,
            top_n_contract: topContract,
            current_answer_sufficiency: edges.length
              ? "已返回当前案件 DuckDB 可见的一跳资金链路、后续线索和边界；普通图谱题应先据此作答。"
              : "未返回可画图资金链路；只能说明证据缺口并建议指定交易或补调流水。"
          }
        },
        flow_graph: flowGraph,
        warnings: [
          "资金图后端不可用，已使用当前案件 DuckDB 本地只读 fallback；后端错误详情未进入模型、聊天或报告。",
          ...arrayOf(sourcePayload.warnings),
          ...arrayOf(viaPayload?.warnings)
        ],
        evidence_refs: {
          query_ids: queryIds,
          audit_ref: { graph_id: `fund-flow-local:${stableHash({ edges, queryIds })}` }
        }
      };
    } catch (localError) {
      throwIfAborted(signal);
      const diagnostic = publicDuckdbDiagnostic(localError, { stage: "execute" });
      const reason = `资金图后端不可用，且当前案件 DuckDB fallback 未成功：${diagnostic.publicMessage}`;
      const insufficient = semanticInsufficientProtocol({
        name: "build_fund_flow_graph",
        args: effectiveArgs,
        error: reason,
        missingCapability: "fund_flow_graph_local_duckdb_fallback"
      });
      return {
        tool: "build_fund_flow_graph",
        skill_id: "build_fund_flow_graph",
        case_id: caseId || undefined,
        status: "capability_gap",
        error_code: "LOCAL_DUCKDB_FALLBACK_FAILED",
        backend_status: "capability_gap",
        reason,
        duckdb_diagnostic: diagnostic,
        insufficient_protocol: insufficient,
        recommended_ladder: insufficient.recommended_ladder,
        safe_to_answer_current_task: false,
        warnings: [
          "后端错误详情未进入模型、聊天或报告。",
          reason,
          "不得编造 DuckDB 未返回的资金图、链路或统计结果。"
        ],
        answer_card: {
          card_type: "local_duckdb_fallback_failed_card",
          title: "资金图 fallback 未成功",
          status: "capability_gap",
          failed_reason: reason,
          next_actions: workbenchRecoveryActions()
        }
      };
    }
  }

  async function callControlledCaseWorkbenchTool(name, args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const effectiveArgs = normalizeRuntimeToolArgs(name, args, env);
    if (
      name === "count_case_rows"
      && effectiveArgs.table_name === "analysis_txn_detail_idx"
      && !Object.prototype.hasOwnProperty.call(effectiveArgs, "where_sql")
      && !Object.prototype.hasOwnProperty.call(effectiveArgs, "whereSql")
    ) {
      return {
        tool: name,
        ...await executeLocalDuckdbWorkbenchSkill(name, effectiveArgs, { env, signal })
      };
    }
    const purpose = caseWorkbenchPurpose(name, effectiveArgs);
    if (CASE_WORKBENCH_PURPOSE_REQUIRED_TOOLS.has(name) && !purpose) {
      return blockedCaseWorkbenchPayload({
        name,
        args: effectiveArgs,
        reason: "使用专项资金核算前必须说明用途或分析目标。"
      });
    }
    if (CASE_WORKBENCH_SQL_REQUIRED_TOOLS.has(name) && !text(effectiveArgs.sql)) {
      const sqlRequiredReason = name === "run_case_sql"
        ? "run_case_sql requires executable sql; query_request is planning context only."
        : `${name} requires executable sql; query_request is planning context only.`;
      return blockedCaseWorkbenchPayload({
        name,
        args: effectiveArgs,
        reason: sqlRequiredReason
      });
    }
    if (name === "preview_case_rows" && (!text(effectiveArgs.table_name) || !text(effectiveArgs.where_sql))) {
      return blockedCaseWorkbenchPayload({
        name,
        args: effectiveArgs,
        reason: "preview_case_rows requires table_name and a narrow where_sql filter; unfiltered samples are not allowed."
      });
    }
    if (name === "preview_case_rows") {
      const previewWhereReason = forbiddenPreviewWhereReason(effectiveArgs.where_sql || effectiveArgs.whereSql);
      if (previewWhereReason) {
        return blockedCaseWorkbenchPayload({
          name,
          args: effectiveArgs,
          reason: previewWhereReason
        });
      }
    }
    const forbiddenReason = forbiddenCaseSqlReason(caseWorkbenchSqlTexts(name, effectiveArgs));
    if (forbiddenReason) {
      return blockedCaseWorkbenchPayload({
        name,
        args: effectiveArgs,
        reason: forbiddenReason
      });
    }
    if (name === "run_case_sql" || name === "create_case_notebook") {
      effectiveArgs.allowed_view_policy = "cleaned_and_analysis_only";
    }
    if (name === "run_case_sql") {
      const rowLimit = Number(effectiveArgs.row_limit || effectiveArgs.rowLimit || 100);
      effectiveArgs.row_limit = Math.max(1, Math.min(Number.isFinite(rowLimit) ? rowLimit : 100, 500));
    }
    if (name === "explain_case_sql" || name === "diagnose_case_sql") {
      const rowLimit = Number(effectiveArgs.row_limit || effectiveArgs.rowLimit || 100);
      effectiveArgs.row_limit = Math.max(1, Math.min(Number.isFinite(rowLimit) ? rowLimit : 100, 500));
    }
    if (name === "profile_case_schema") {
      const tableLimit = Number(effectiveArgs.table_limit || effectiveArgs.tableLimit || 8);
      const columnLimit = Number(effectiveArgs.column_limit || effectiveArgs.columnLimit || 24);
      effectiveArgs.table_limit = Math.max(1, Math.min(Number.isFinite(tableLimit) ? tableLimit : 8, 20));
      effectiveArgs.column_limit = Math.max(1, Math.min(Number.isFinite(columnLimit) ? columnLimit : 24, 80));
    }
    if (name === "preview_case_rows") {
      const rowLimit = Number(effectiveArgs.row_limit || effectiveArgs.rowLimit || 20);
      effectiveArgs.row_limit = Math.max(1, Math.min(Number.isFinite(rowLimit) ? rowLimit : 20, 20));
    }
    if (name === "inspect_workbench_history") {
      const limit = Number(effectiveArgs.limit || 20);
      effectiveArgs.limit = Math.max(1, Math.min(Number.isFinite(limit) ? limit : 20, 50));
    }
    if (name === "case_sql_recipes") {
      const limit = Number(effectiveArgs.limit || 50);
      effectiveArgs.limit = Math.max(1, Math.min(Number.isFinite(limit) ? limit : 50, 100));
    }
    if (name === "create_case_notebook") {
      const maxRows = Number(effectiveArgs.max_rows_per_query || effectiveArgs.maxRowsPerQuery || 100);
      effectiveArgs.max_rows_per_query = Math.max(1, Math.min(Number.isFinite(maxRows) ? maxRows : 100, 500));
    }
    try {
      const payload = await executeSkill(name, effectiveArgs, { signal });
      const evidenceCard = name === "run_case_sql"
        ? runCaseSqlEvidenceCard({ args: effectiveArgs, payload })
        : undefined;
      return {
        tool: name,
        skill_id: name,
        case_id: text(effectiveArgs.case_id || effectiveArgs.caseId) || undefined,
        status: "ok",
        answer_card: workbenchCard({
          name,
          args: effectiveArgs,
          status: "ok",
          reason: "专项资金核算已返回有界后端结果。",
          nextActions: [
            "需要图表或表格时，交给证据图表附件流程。",
            "写入报告级表述前，先做报告事实结论复核。"
          ]
        }),
        evidence_card: evidenceCard,
        response: evidenceCard ? {
          ...objectOf(payload),
          evidence_card: objectOf(payload).evidence_card || evidenceCard,
          data: {
            ...objectOf(objectOf(payload).data),
            evidence_card: objectOf(objectOf(payload).data).evidence_card || evidenceCard,
            source_hash: text(objectOf(objectOf(payload).data).source_hash) || evidenceCard.source_hash,
            metric_scope: objectOf(objectOf(payload).data).metric_scope || evidenceCard.metric_scope,
            validation_state: objectOf(objectOf(payload).data).validation_state || evidenceCard.validation_state
          }
        } : payload
      };
    } catch (error) {
      throwIfAborted(signal);
      if (isCaseSourceError(error)) {
        return caseSourceBlockerPayload({ name, args: effectiveArgs, error });
      }
      if (isDuckdbDiagnosticError(error)) {
        return typedLocalDuckdbFailurePayload({ name, skillId: name, args: effectiveArgs, error });
      }
      const fallback = await localDuckdbFallbackPayload({
        name,
        skillId: name,
        args: effectiveArgs,
        error,
        controlled: true,
        signal
      });
      if (fallback) return fallback;
      return capabilityGapCaseWorkbenchPayload({ name, args: effectiveArgs, error });
    }
  }

  async function callToolUnchecked(name, args = {}, { signal } = {}) {
    throwIfAborted(signal);
    if (name === INTERNAL_DATASET_SNAPSHOT_PROBE_TOOL) {
      throw new Error("DATASET_SNAPSHOT_AUTHORITY_UNAVAILABLE");
    }
    if (name === "funds_investigate") {
      if (looksLikeReportPublicationRequest(args)) {
        return publicationReceiptRequiredPayload({ name, skillId: name, args });
      }
      if (looksLikePairAmountRequest(args)) {
		return routePairAmountThroughFocusedSupport(fundsInvestigate, args, { signal });
      }
      return fundsInvestigate(args, { signal });
    }
    if (name === "investigate_pair_amount") {
	  return routePairAmountThroughFocusedSupport(fundsInvestigate, args, { signal });
    }
    if (name === "get_casegraph") {
      return getCaseGraph(args, { signal });
    }
    if (name === "build_fund_flow_graph") {
      try {
        return await withTransientRetry(name, () => buildFundFlowGraph(args, { signal }), 2, signal);
      } catch (error) {
        throwIfAborted(signal);
        return localBuildFundFlowGraphFallback(args, error, signal);
      }
    }
    if (name === "get_current_case") {
      const resolved = await resolveCase(args, { signal });
      return {
        case_id: resolved.case_id,
        source: resolved.source,
        case: compactCaseRecord(resolved.case, { includeImports: args.include_imports === true })
      };
    }
    if (name === "get_case_status") {
      const includeImports = args.include_imports === true;
      const resolved = await resolveCase(args, { signal });
      const [caseDetail, skills] = await Promise.all([
        httpJson(`/cases/${encodeURIComponent(resolved.case_id)}`, { signal }).catch((error) => {
          throwIfAborted(signal);
          return { error: error.message, data: resolved.case };
        }),
        httpJson("/skills?visibility=all", { signal }).then(
          (payload) => compactSkillsRecord(payload),
          () => {
            throwIfAborted(signal);
            return unavailableSkillsSurfaceState();
          }
        )
      ]);
      return {
        case_id: resolved.case_id,
        source: resolved.source,
        case: compactCaseRecord(caseDetail.data || caseDetail, { includeImports }),
        skills
      };
    }
    if (name === "get_import_overview") {
      return getImportOverview(args, { signal });
    }
    if (name === "get_cleaning_overview") {
      return getCleaningOverview(args, { signal });
    }
    if (name === "get_case_data_pipeline_overview") {
      return getCaseDataPipelineOverview(args, { signal });
    }
    if (name === "get_case_scope_map") {
      return getCaseScopeMap(args, { signal });
    }
    if (name === "get_stats_meta") {
      return getStatsMeta(args, { signal });
    }
    if (name === "get_stats_tree") {
      return getStatsTree(args, { signal });
    }
    if (name === "query_stats_rows") {
      return queryStatsRows(args, { signal });
    }
    if (name === "query_stats_txn_rows") {
      return queryStatsTxnRows(args, { signal });
    }
    if (name === "query_account_txn_rows") {
      return queryAccountTxnRows(args, { signal });
    }
    if (name === "get_analysis_dashboard") {
      return getAnalysisDashboard(args, { signal });
    }
    if (name === "export_cleaned_case_data") {
      return exportCleanedCaseData(args, { signal });
    }
    if (CONTROLLED_CASE_WORKBENCH_TOOLS.has(name)) {
      return callControlledCaseWorkbenchTool(name, args, { signal });
    }
    const skillId = skillToolMap.get(name);
    if (!skillId) {
      throw new Error(`Unknown tool: ${name}`);
    }
    const effectiveArgs = normalizeRuntimeToolArgs(name, args, env);
    const validationError = text(effectiveArgs[MCP_VALIDATION_ERROR_KEY]);
    delete effectiveArgs[MCP_VALIDATION_ERROR_KEY];
    if (validationError) {
      return {
        tool: name,
        skill_id: skillId,
        case_id: text(effectiveArgs.case_id || effectiveArgs.caseId) || undefined,
        ...validationWarning("INVALID_TOLERANCE_RATIO", validationError)
      };
    }
    const forcedProbeType = probeToolTypes.get(name);
    if (forcedProbeType && !text(effectiveArgs.probe_type)) {
      effectiveArgs.probe_type = forcedProbeType;
    }
    if (skillId === "run_full_case_analysis") {
      Object.assign(effectiveArgs, normalizeFullCaseAnalysisArgs(effectiveArgs));
      return publicationReceiptRequiredPayload({ name, skillId, args: effectiveArgs });
    }
    if (skillId === "hypothesis_probe") {
      const probeKeywords = [
        ...arrayOf(effectiveArgs.keywords).map(text).filter(Boolean),
        text(effectiveArgs.via_holder_name),
        text(effectiveArgs.counterparty_name)
      ].filter(Boolean);
      if (probeKeywords.length) {
        effectiveArgs.keywords = [...new Set(probeKeywords)];
      }
      delete effectiveArgs.via_holder_name;
      delete effectiveArgs.counterparty_name;
    }
    let payload = {};
    if (skillId === "validate_report_claims") {
      try {
        // A report-claim review is publication-blocked until the host issues a
        // PublicationReceipt. A transient backend failure must not schedule a
        // delayed retry that can outlive the terminal blocked response.
        payload = await withTransientRetry(skillId, () => executeSkill(skillId, effectiveArgs, { signal }), 1, signal);
      } catch (error) {
        throwIfAborted(signal);
        const warning = "后端报告复核服务暂不可用；错误详情未进入模型、聊天或报告。";
        const claims = arrayOf(effectiveArgs.claims);
        return {
          tool: name,
          skill_id: skillId,
          case_id: text(effectiveArgs.case_id || effectiveArgs.caseId) || undefined,
          status: "blocked",
          blocker: "report_claim_validation_unavailable",
          answer_card: withClaimReviewProtocol({
            card_type: "claim_review_card",
            case_id: text(effectiveArgs.case_id || effectiveArgs.caseId) || undefined,
            intent: "claim_review",
            title: "报告研判结论复核不可用",
            fact_source: "host_boundary",
            write_blocked: true,
            report_gate_status: "claim_validation_unavailable",
            claims,
            verified_claims: [],
            unsupported_claims: claims.map(reviewEntryText).filter(Boolean),
            warnings: [warning, "未签发 PublicationReceipt；不得执行本地报告复核 fallback 或发布正式报告。"]
          }, {
            recommendedNextAction: "stop",
            maxAdditionalTools: 0,
            requiredFactsPresent: false,
            unsupportedFlowsPresent: true
          }),
          warnings: [warning, "validate_report_claims 未执行本地 fallback。"]
        };
      }
    } else {
      try {
        payload = await withTransientRetry(skillId, () => executeSkill(skillId, effectiveArgs, { signal }), 2, signal);
      } catch (error) {
        throwIfAborted(signal);
        const fallback = await localDuckdbFallbackPayload({
          name,
          skillId,
          args: effectiveArgs,
          error,
          signal
        });
        if (fallback) return fallback;
        throw error;
      }
    }
    if (skillId === "validate_report_claims") {
      const data = skillEnvelopeData(payload);
      const envelope = unwrapSkillEnvelope(payload);
      const claims = arrayOf(effectiveArgs.claims);
      const reportText = reportReviewBody(effectiveArgs, claims);
      const deterministicAugments = await buildClaimReviewDeterministicAugments({
        caseId: text(effectiveArgs.case_id || effectiveArgs.caseId),
        body: reportText
      });
      const localRiskWarnings = localClaimReviewRiskWarnings(reportText);
      const candidateClaims = backendClaimCandidates(data);
      const unsupportedClaims = [
        ...reviewEntryTexts(data.unsupported_claims),
        ...unsupportedNumberClaims(data)
      ];
      const correctedClaims = [
        ...reviewEntryTexts(data.corrected_claims),
        ...deterministicAugments.correctedClaims
      ];
      const unsupportedFlows = [
        ...reviewEntryTexts(data.unsupported_flows),
        ...deterministicAugments.unsupportedFlows
      ];
      const missingSourceBoundaries = [
        ...reviewEntryTexts(data.missing_source_boundaries),
        ...deterministicAugments.missingSourceBoundaries
      ];
      const forbiddenPhrasings = reviewEntryTexts(data.forbidden_phrasings);
      const warnings = [
        ...reviewEntryTexts(data.warnings || envelope.warnings),
        ...deterministicAugments.warnings,
        ...localRiskWarnings,
        "后端返回的 verified/supported claim、citation 和 receipt 均只作为候选；当前没有宿主 registry membership 或 PublicationReceipt，不能授权案件事实或正式报告。"
      ];
      const backendReviewHasIssues = [
        ...correctedClaims,
        ...unsupportedClaims,
        ...unsupportedFlows,
        ...missingSourceBoundaries,
        ...forbiddenPhrasings,
        ...warnings
      ].length > 0;
      if (backendReviewHasIssues) {
        warnings.push("write_blocked=true: 正式报告暂不出具，复核或来源边界仍有缺口。");
      }
      const claimReviewCard = withClaimReviewProtocol({
        card_type: "claim_review_card",
        case_id: text(effectiveArgs.case_id || effectiveArgs.caseId) || undefined,
        intent: "claim_review",
        title: "既有报告研判结论复核材料",
        fact_source: "backend_candidate_review_plus_host_boundary",
        status: "blocked",
        semantic_status: "blocked",
        safeToAnswer: false,
        safe_to_answer_current_task: false,
        write_blocked: true,
        report_gate_status: "publication_receipt_required",
        report_text: reportText,
        claims,
        facts: effectiveArgs.facts,
        final_investigation_answer: effectiveArgs.final_investigation_answer || effectiveArgs.investigation_answer,
        flow_graph: effectiveArgs.flow_graph || effectiveArgs.flowGraph,
        supported_edge_count: effectiveArgs.supported_edge_count,
        candidate_claims: candidateClaims,
        verified_claims: [],
        supported_claims: [],
        corrected_claims: correctedClaims,
        unsupported_claims: unsupportedClaims,
        unsupported_flows: unsupportedFlows,
        missing_source_boundaries: missingSourceBoundaries,
        forbidden_phrasings: forbiddenPhrasings,
        next_review_actions: [
          ...reviewEntryTexts(data.next_review_actions),
          ...deterministicAugments.nextReviewActions
        ],
        warnings
      }, {
        recommendedNextAction: "stop",
        maxAdditionalTools: 0,
        requiredFactsPresent: false,
        unsupportedFlowsPresent: true
      });
      return {
        tool: name,
        skill_id: skillId,
        case_id: text(effectiveArgs.case_id || effectiveArgs.caseId) || undefined,
        status: "blocked",
        semantic_status: "blocked",
        blocker: "publication_receipt_required",
        isError: true,
        safeToAnswer: false,
        safe_to_answer_current_task: false,
        write_blocked: true,
        report_gate_status: "publication_receipt_required",
        answer_card: claimReviewCard,
        backend_review: {
          asserted_status: text(envelope.status || data.status || payload.status) || "unknown",
          candidate_claim_count: candidateClaims.length,
          claims_are_authoritative: false,
          host_registry_verified: false,
          publication_receipt_verified: false
        },
        warnings
      };
    }
    return {
      tool: name,
      skill_id: skillId,
      case_id: text(effectiveArgs.case_id || effectiveArgs.caseId) || undefined,
      response: payload
    };
  }

  async function callTool(name, args = {}, { signal } = {}) {
    throwIfAborted(signal);
    if (isPreExecutionBlockedToolName(name)) {
      if (name === "run_full_case_analysis") {
        return publicationReceiptRequiredPayload({ name, skillId: name, args });
      }
      return preExecutionWriteQuarantinePayload(name);
    }
    try {
      const result = await callToolUnchecked(name, args, { signal });
      throwIfAborted(signal);
      return result;
    } catch (error) {
      throwIfAborted(signal);
      if (isCaseSourceError(error)) {
        return caseSourceBlockerPayload({ name, args, error });
      }
      const skillId = skillToolMap.get(name) || name;
      if (isDuckdbDiagnosticError(error)) {
        return typedLocalDuckdbFailurePayload({ name, skillId, args: normalizeSkillArgs(name, args), error });
      }
      const fallback = await localDuckdbFallbackPayload({
        name,
        skillId,
        args: normalizeSkillArgs(name, args),
        error,
        controlled: CONTROLLED_CASE_WORKBENCH_TOOLS.has(name),
        signal
      });
      if (fallback) return fallback;
      if (!text(error?.message || error).startsWith("Unknown tool:")) {
        return semanticToolCapabilityGapPayload({ name, args, error });
      }
      throw error;
    }
  }

  return { callTool };
}
