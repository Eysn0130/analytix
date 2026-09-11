#!/usr/bin/env node

import fs from "node:fs";
import http from "node:http";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { userVisibleLeakageLabels } from "../mcp/user-facing-language.mjs";

const require = createRequire(import.meta.url);

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(scriptDir, "..");
const repoRoot = path.resolve(pluginRoot, "../..");
const desktopAppDir = path.join(repoRoot, "apps", "desktop");
const webAppDir = path.join(repoRoot, "apps", "web");
const desktopRequire = createRequire(path.join(desktopAppDir, "package.json"));
const webRequire = createRequire(path.join(webAppDir, "package.json"));
const electronBinaryPath = desktopRequire("electron");
const { _electron: electron } = webRequire("playwright");
const { sanitizeAnalytixManagedCacheEnv } = desktopRequire("./src/main/managed-cache-env");

const DEFAULT_OUTPUT_DIR = path.join(repoRoot, "output", "analytix-fund-analysis", "desktop-acceptance");
const DEFAULT_CWD = repoRoot;
const DEFAULT_MODEL = "gpt-5.5";
const DEFAULT_MODEL_PROVIDER = "openai";

const SCENARIOS = [
  {
    id: "desktop_pair_amount_material",
    envKey: "PAIR_AMOUNT",
    requiredKinds: ["period", "amount", "count", "table", "anomaly", "meaning", "next_step", "cannot_identify"],
  },
  {
    id: "desktop_amount_challenge",
    envKey: "AMOUNT_CHALLENGE",
    requiredKinds: ["amount", "challenge", "explanation", "source_boundary", "cannot_identify", "next_step"],
  },
  {
    id: "desktop_rescope_followup",
    envKey: "RESCOPE_FOLLOWUP",
    requiredKinds: ["rescope", "amount", "explanation", "source_boundary", "cannot_identify"],
  },
  {
    id: "desktop_flow_graph_material",
    envKey: "FLOW_GRAPH",
    requiredKinds: ["graph", "period", "amount", "edge", "next_step", "cannot_identify"],
  },
  {
    id: "desktop_subject_account_dossier",
    envKey: "SUBJECT_DOSSIER",
    requiredKinds: ["account", "inflow", "outflow", "counterparty", "anomaly", "next_step"],
  },
  {
    id: "desktop_downstream_continuation",
    envKey: "DOWNSTREAM_TRACE",
    requiredKinds: ["downstream", "edge", "amount", "next_step", "cannot_identify"],
  },
  {
    id: "desktop_cleaned_export_evidence",
    envKey: "CLEANED_EXPORT",
    requiredKinds: ["cleaned_export", "table", "field", "source_boundary"],
  },
  {
    id: "desktop_blank_counterparty_review",
    envKey: "BLANK_COUNTERPARTY",
    requiredKinds: ["blank_counterparty", "amount", "count", "explanation", "next_step"],
  },
  {
    id: "desktop_report_continuation",
    envKey: "REPORT_CONTINUATION",
    requiredKinds: ["report", "continuation", "amount_check", "source_boundary", "cannot_identify"],
  },
  {
    id: "desktop_full_case_analysis",
    envKey: "FULL_CASE_ANALYSIS",
    requiredKinds: ["full_case", "inflow", "outflow", "anomaly", "judgment", "next_step"],
  },
  {
    id: "desktop_project_related_asset_probe",
    envKey: "PROJECT_RELATED_ASSET",
    requiredKinds: ["project", "company", "relative", "asset", "lead", "next_step"],
  },
];

const REQUIRED_KIND_PATTERNS = {
  account: /账户/u,
  amount: /(?:金额|[0-9][0-9,.]*\s*元|万元|亿元)/u,
  amount_check: /(?:金额核算|金额核对|全文金额|复核金额)/u,
  anomaly: /(?:异常|集中|分拆|可疑|特征)/u,
  asset: /(?:资产端|房产|车辆|理财|证券|基金|保险|大额消费)/u,
  blank_counterparty: /(?:空对手|缺失对手|对手方缺失|无对手)/u,
  cannot_identify: /(?:暂不能认定|不能认定|不能直接|不能替代|不能用|不作为新增|需复核|证据不足)/u,
  challenge: /(?:质疑|挑战|不是|为什么|核对|复核|差异)/u,
  cleaned_export: /(?:清洗明细|证据表|导出|附件)/u,
  company: /(?:关联公司|公司|企业|单位)/u,
  continuation: /(?:下一步侦查建议|暂不能认定事项|补调|补证|核查|复核|穿透|调取|比对|核对)/u,
  count: /(?:笔数|[0-9]+\s*笔)/u,
  counterparty: /(?:重点对手|对手方|交易对手)/u,
  downstream: /(?:下游|去向|后续转出|继续追踪|穿透)/u,
  edge: /(?:资金链路|交易链路|链路|流向|路径|转入|转出)/u,
  explanation: /(?:业务解释|原因|成因|说明|使用边界|来源|用途|不能把|不能直接)/u,
  field: /(?:字段|交易日期|交易金额|对手方|账号|摘要)/u,
  full_case: /(?:全案|整体|综合研判|案件研判)/u,
  graph: /(?:资金流向图|流向图|穿透图|图谱|PNG|JPG|图片)/u,
  inflow: /(?:流入|收入|进账|入账)/u,
  judgment: /(?:研判认为|侦查判断|案件意义|判断)/u,
  lead: /(?:线索|需复核|建议补调|补证)/u,
  meaning: /(?:案件意义|资金意义|研判认为|资金性质|角色|目的|重点核查|线索意义|用途线索|异常线索)/u,
  next_step: /(?:下一步|建议补调|补调|核查建议|侦查建议|调取)/u,
  outflow: /(?:流出|支出|出账|转出)/u,
  period: /(?:期间|起止|[12][0-9]{3}[-/年][0-9]{1,2}[-/月][0-9]{1,2}|[12][0-9]{3}年[0-9]{1,2}月[0-9]{1,2}日)/u,
  project: /(?:项目款|工程款|主材|劳务|合同|票税|成本)/u,
  relative: /(?:亲属|近亲属|家属|关联人员)/u,
  report: /(?:报告|研判材料|文书|初稿|续写)/u,
  rescope: /(?:改为|重算|重新计算|统计范围|核验范围|原始明细|有效去重|不得混用|不能混用)/u,
  source_boundary: /(?:来源边界|已有流水支持|需复核|暂不能认定|核验意见)/u,
  table: /\|[^\n]+\|[^\n]*\n\|[-:| ]+\||(?:表|序号).{0,20}(?:金额|笔数|时间)/u,
};

const FORBIDDEN_VISIBLE_PATTERNS = [
  ["旧同名口径", /全期间同名/u],
  ["旧口径标题", /(?:金额口径|口径提示|口径差异说明)/u],
  ["计划式进度泄漏", /(?:^|\n)\s*(?:我先|我会|我将)[^。\n；]{0,80}(?:核验|读取|整理|流程|材料|交易明细|口径)/u],
  ["进度口径泄漏", /(?:我会|将|正在|先)?[^。\n；]{0,20}按[^。\n；]{0,40}口径/u],
  ["读取技能说明泄漏", /(?:先)?读取[^。\n；]{0,20}技能说明|技能文件|先看[^。\n；]{0,20}文件|读取案件数据口径|可用交易明细/u],
  ["旧金额小研判", /金额核验\s*小研判/u],
  ["旧统计标题", /(?:重点收款账号统计|重点收款账户情况|同名收款人匹配结果)/u],
  ["待核旧词", /待复核/u],
  ["工具卡语气", /(?:事实卡|证据要点|核验摘要|材料状态|风险\/复核卡)/u],
  ["工程实现泄漏", /(?:DuckDB|here-doc|沙箱|命令行|本地路径|执行命令|grep|cat|shell|Shell|SQL|执行器|专项明细查询|明细字段与交易行)/u],
  ["内部路由泄漏", /(?:MCP|Workbench|SKILL\.md|case_id|workflow|delivery_state|support layer|candidate\/direct)/iu],
  ["审计层资金边表述", /资金边/u],
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

const CONTINUATION_ACTION_PATTERN = /(?:补调|补证|核查|复核|穿透|调取|比对|核对|查明|追踪|追查|排查|验证|确认|梳理)/u;
const CONTINUATION_OBJECT_PATTERN = /(?:账户|账号|回单|流水|交易明细|理财|基金|申赎资料|重点收款对象|Top\s*20|主体|对手方|余额承接|收款对象|付款账号|收款账号|下一层流水)/iu;
const CONTINUATION_PURPOSE_PATTERN = /(?:资金去向|最终受益人|实际控制|控制关系|暂不能认定|不能认定|关系认定|资金性质|资金用途|余额承接|来源去向|证明|排除|确认|边界)/u;

function sectionFromMarker(body, markerPattern, maxLength = 1400) {
  const match = markerPattern.exec(body);
  if (!match) return "";
  return body.slice(match.index, match.index + maxLength);
}

function hasContinuationParts(body) {
  return CONTINUATION_ACTION_PATTERN.test(body)
    && CONTINUATION_OBJECT_PATTERN.test(body)
    && CONTINUATION_PURPOSE_PATTERN.test(body);
}

function hasReportContinuationQuality(answer) {
  const content = text(answer);
  if (!content) return false;
  const investigationPlan = sectionFromMarker(content, /(?:下一步侦查建议|下一步核查|补证建议|核查建议)/u);
  if (investigationPlan && hasContinuationParts(investigationPlan)) return true;

  const unresolvedMatters = sectionFromMarker(content, /(?:暂不能认定事项|暂不能认定|不能认定)/u);
  if (unresolvedMatters && hasContinuationParts(unresolvedMatters)) return true;

  const disposition = sectionFromMarker(content, /(?:资金去向|关系认定|最终受益人|实际控制关系|控制关系)/u);
  if (disposition && hasContinuationParts(disposition)) return true;

  const candidateSentences = content
    .split(/(?<=[。；;!?！？\n])\s*/u)
    .map((line) => line.trim())
    .filter(Boolean);
  return candidateSentences.some((line) => hasContinuationParts(line));
}

function hasRequiredKind(kind, answer) {
  if (kind === "continuation") return hasReportContinuationQuality(answer);
  return REQUIRED_KIND_PATTERNS[kind]?.test(answer);
}

const FULL_CASE_PRIORITY_LABEL_PATTERN = /(?:重点主体|重点账户|重点对手方|排行|排名|优先核查对象|资金规模靠前主体|重点核查对象|主要主体|主要账户|主要对手方)/u;
const FULL_CASE_PRIORITY_STRONG_LABEL_PATTERN = /(?:重点主体|重点账户|重点对手方)/gu;
const FULL_CASE_AMOUNT_FIELD_PATTERN = /(?:入账|出账|往来合计|交易金额|资金规模|金额|流入|流出|[0-9][0-9,.]*\s*元|万元|亿元)/u;
const FULL_CASE_AUDIT_FIELD_PATTERN = /(?:笔数|期间|交易[0-9,，]*\s*笔|[0-9]+\s*笔|[12][0-9]{3}[-/年][0-9]{1,2}|至)/u;
const FULL_CASE_INVESTIGATIVE_CONTEXT_PATTERN = /(?:数据规模|统计期间|异常特征|暂不能认定|下一步侦查建议|核查建议|需复核|需补证|资金链路)/u;

function hasTableOrListStructure(body) {
  return REQUIRED_KIND_PATTERNS.table.test(body)
    || /(?:^|\n)\s*(?:[-*]|\d+[.、])\s*\S+/u.test(body)
    || /(?:排名|排行|重点主体|重点账户|重点对手方).{0,80}(?:\n|：|:)/u.test(body);
}

function hasFullCasePriorityRankingQuality(answer) {
  const content = text(answer);
  if (!content) return false;
  const labelMatches = [...content.matchAll(FULL_CASE_PRIORITY_STRONG_LABEL_PATTERN)]
    .map((match) => match[0]);
  const strongLabelCount = new Set(labelMatches).size;
  const hasPriorityMeaning = FULL_CASE_PRIORITY_LABEL_PATTERN.test(content);
  const hasStructure = hasTableOrListStructure(content);
  const hasAmount = FULL_CASE_AMOUNT_FIELD_PATTERN.test(content);
  const hasAuditField = FULL_CASE_AUDIT_FIELD_PATTERN.test(content);
  const hasContext = FULL_CASE_INVESTIGATIVE_CONTEXT_PATTERN.test(content);
  if (strongLabelCount >= 2 && hasStructure && hasAmount && hasAuditField) return true;
  return hasPriorityMeaning && hasStructure && hasAmount && hasAuditField && hasContext;
}

function hasScenarioMarker(scenario, marker, answer, looseAnswer) {
  const literal = text(marker);
  if (!literal) return true;
  if (scenario.id === "desktop_full_case_analysis" && /^(?:Top\s*20|Top20)$/iu.test(literal)) {
    return hasFullCasePriorityRankingQuality(answer);
  }
  return answer.includes(literal) || looseAnswer.includes(markerText(literal));
}

function markerText(value) {
  return text(value).replace(/\s+/gu, "");
}

function moneyMarker(value) {
  return text(value).replace(/[,\s，]/gu, "");
}

function parseCsv(value) {
  return text(value).split(",").map((item) => item.trim()).filter(Boolean);
}

function parseList(value) {
  const raw = text(value);
  if (!raw) return [];
  if (raw.includes("||")) return raw.split("||").map((item) => item.trim()).filter(Boolean);
  if (raw.includes("\n")) return raw.split(/\r?\n/u).map((item) => item.trim()).filter(Boolean);
  return parseCsv(raw);
}

function isEnabled(value) {
  return /^(1|true|yes)$/iu.test(text(value));
}

function parseArgs(argv) {
  const options = {
    backendUrl: text(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_BACKEND_URL),
    cwd: text(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_CWD) || DEFAULT_CWD,
    model: text(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_MODEL) || DEFAULT_MODEL,
    modelProvider: text(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_MODEL_PROVIDER) || DEFAULT_MODEL_PROVIDER,
    output: "",
    timeoutMs: Number(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_TIMEOUT_MS || 900_000) || 900_000,
    pollMs: Number(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_POLL_MS || 3_000) || 3_000,
    limit: Number(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_LIMIT || 0) || 0,
    sandboxMode: text(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_SANDBOX) || "danger-full-access",
    manageBackend: !text(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_BACKEND_URL),
    requireMarkers: !/^(0|false|no)$/iu.test(text(process.env.ANALYTIX_FUNDS_DESKTOP_MULTITURN_REQUIRE_MARKERS || "1")),
    activeCaseSyncCheck: isEnabled(process.env.ANALYTIX_FUNDS_DESKTOP_ACTIVE_CASE_SYNC_CHECK),
    useLocalRcHub: isEnabled(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_USE_LOCAL_RC_HUB),
    userDataDir: text(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_USER_DATA_DIR),
    json: false,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const next = () => argv[++index] || "";
    if (arg === "--backend-url") {
      options.backendUrl = next();
      options.manageBackend = false;
    } else if (arg === "--cwd") options.cwd = next();
    else if (arg === "--model") options.model = next();
    else if (arg === "--model-provider") options.modelProvider = next();
    else if (arg === "--output") options.output = next();
    else if (arg === "--timeout-ms") options.timeoutMs = Number(next()) || options.timeoutMs;
    else if (arg === "--poll-ms") options.pollMs = Number(next()) || options.pollMs;
    else if (arg === "--limit") options.limit = Number(next()) || 0;
    else if (arg === "--sandbox") options.sandboxMode = next() || options.sandboxMode;
    else if (arg === "--user-data-dir") options.userDataDir = next();
    else if (arg === "--no-manage-backend") options.manageBackend = false;
    else if (arg === "--no-require-markers") options.requireMarkers = false;
    else if (arg === "--active-case-sync-check") options.activeCaseSyncCheck = true;
    else if (arg === "--use-local-rc-hub") options.useLocalRcHub = true;
    else if (arg === "--json") options.json = true;
    else if (arg === "-h" || arg === "--help") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  if (!options.output) {
    const stamp = new Date().toISOString().replace(/[:.]/gu, "-");
    options.output = path.join(DEFAULT_OUTPUT_DIR, `desktop-multiturn-acceptance-${stamp}.json`);
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/desktop-multiturn-acceptance.mjs [options]

Runs a real Electron + embedded analytixagent multi-turn acceptance check.
Real case names, amounts, expected facts, and markers are supplied through env
vars and are written only to the local output evidence file.

Required env:
  ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID
  ANALYTIX_FUNDS_DESKTOP_<SCENARIO>_QUESTION
  ANALYTIX_FUNDS_DESKTOP_<SCENARIO>_MARKERS
  ANALYTIX_FUNDS_DESKTOP_PAIR_AMOUNT_QUESTION
  ANALYTIX_FUNDS_DESKTOP_AMOUNT_CHALLENGE_QUESTION
  ANALYTIX_FUNDS_DESKTOP_RESCOPE_FOLLOWUP_QUESTION
  ANALYTIX_FUNDS_DESKTOP_FLOW_GRAPH_QUESTION
  ANALYTIX_FUNDS_DESKTOP_SUBJECT_DOSSIER_QUESTION
  ANALYTIX_FUNDS_DESKTOP_DOWNSTREAM_TRACE_QUESTION
  ANALYTIX_FUNDS_DESKTOP_CLEANED_EXPORT_QUESTION
  ANALYTIX_FUNDS_DESKTOP_BLANK_COUNTERPARTY_QUESTION
  ANALYTIX_FUNDS_DESKTOP_REPORT_CONTINUATION_QUESTION
  ANALYTIX_FUNDS_DESKTOP_FULL_CASE_ANALYSIS_QUESTION
  ANALYTIX_FUNDS_DESKTOP_PROJECT_RELATED_ASSET_QUESTION
  ANALYTIX_FUNDS_DESKTOP_ACTIVE_CASE_SYNC_CHECK
  ANALYTIX_FUNDS_DESKTOP_SCENARIO_ORDER

Scenario keys:
  PAIR_AMOUNT, AMOUNT_CHALLENGE, RESCOPE_FOLLOWUP, FLOW_GRAPH, SUBJECT_DOSSIER, DOWNSTREAM_TRACE,
  CLEANED_EXPORT, BLANK_COUNTERPARTY, REPORT_CONTINUATION,
  FULL_CASE_ANALYSIS, PROJECT_RELATED_ASSET

Runtime path:
  agentThreadStart creates the real desktop thread; turns are sent through
  vscode://codex/turn/start and verified with thread/read.

Options:
  --backend-url <url>      Use an existing backend instead of Electron-managed backend.
  --limit <n>              Run only the first n configured scenarios.
  --active-case-sync-check Verify encoded explicit case lookups around switch and restore.
  --use-local-rc-hub      Pin Hub marketplace, skill catalog, and install-policy to the Analytix-owned local RC runtime cache.
  --no-require-markers     Allow scenarios without explicit env marker lists.
  --json                   Print machine-readable result.
`);
}

function loadEnvFile(filePath) {
  if (!fs.existsSync(filePath)) return;
  const source = fs.readFileSync(filePath, "utf8");
  for (const line of source.split(/\r?\n/u)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#") || !trimmed.includes("=")) continue;
    const index = trimmed.indexOf("=");
    const key = trimmed.slice(0, index).trim();
    let value = trimmed.slice(index + 1).trim();
    if (!key || process.env[key] !== undefined) continue;
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }
    process.env[key] = value;
  }
}

function loadLocalEnvFiles() {
  for (const relative of [".env", ".env.local", "apps/desktop/.env", "apps/desktop/.env.local", "backend/.env", "backend/.env.local"]) {
    loadEnvFile(path.join(repoRoot, relative));
  }
}

function defaultAnalytixRuntimeHome() {
  return path.join(os.homedir(), ".analytix");
}

function localRcHubManifestPaths() {
  const runtimeHome = text(process.env.ANALYTIX_AGENT_RUNTIME_HOME)
    || text(process.env.ANALYTIX_AGENT_RUNTIME_CODEX_HOME)
    || defaultAnalytixRuntimeHome();
  return {
    runtime_home: runtimeHome,
    marketplace_manifest: path.join(
      runtimeHome,
      ".cache",
      "analytix-hub-plugins",
      "marketplaces",
      "analytix-hub",
      ".agents",
      "plugins",
      "marketplace.json"
    ),
    skill_catalog_manifest: path.join(
      runtimeHome,
      ".cache",
      "analytix-hub-plugins",
      "skills",
      "catalog.json"
    ),
    install_policy_manifest: path.join(
      runtimeHome,
      ".cache",
      "analytix-hub-plugins",
      "install-policy.json"
    ),
  };
}

function readJsonFile(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function assertLocalRcHubReady(paths) {
  const manifest = readJsonFile(path.join(pluginRoot, ".codex-plugin", "plugin.json"));
  const version = text(manifest.version);
  const marketplace = readJsonFile(paths.marketplace_manifest);
  const plugin = Array.isArray(marketplace.plugins)
    ? marketplace.plugins.find((entry) => text(entry?.name) === "analytix-fund-analysis")
    : null;
  const sourcePath = text(plugin?.source?.path);
  if (!plugin || text(plugin.version) !== version || !sourcePath.includes(`/analytix-fund-analysis/${version}`)) {
    throw new Error(`local RC Hub marketplace is not mounted for analytix-fund-analysis@${version}: ${paths.marketplace_manifest}`);
  }
  const installPolicy = readJsonFile(paths.install_policy_manifest);
  const policyPlugin = Array.isArray(installPolicy.requiredPlugins)
    ? installPolicy.requiredPlugins.find((entry) => text(entry?.pluginName) === "analytix-fund-analysis")
    : null;
  if (!policyPlugin || text(policyPlugin.version) !== version) {
    throw new Error(`local RC Hub install-policy is not mounted for analytix-fund-analysis@${version}: ${paths.install_policy_manifest}`);
  }
  const directRoot = path.join(paths.runtime_home, "plugins", "cache", "analytix-hub", "analytix-fund-analysis", version);
  if (!fs.existsSync(path.join(directRoot, ".codex-plugin", "plugin.json"))) {
    throw new Error(`local RC Hub direct plugin cache is missing analytix-fund-analysis@${version}: ${directRoot}`);
  }
  return {
    plugin_version: version,
    marketplace_source_path: sourcePath,
    direct_plugin_cache: directRoot,
  };
}

function applyLocalRcHubEnv(env, options) {
  if (!options.useLocalRcHub) return null;
  const paths = localRcHubManifestPaths();
  for (const value of [paths.marketplace_manifest, paths.skill_catalog_manifest, paths.install_policy_manifest]) {
    if (!fs.existsSync(value)) {
      throw new Error(`local RC Hub manifest missing: ${value}`);
    }
  }
  const state = assertLocalRcHubReady(paths);
  delete env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_MANIFEST;
  delete env.ANALYTIX_AGENT_PLUGIN_SKILL_CATALOG_MANIFEST;
  delete env.ANALYTIX_AGENT_PLUGIN_INSTALL_POLICY_MANIFEST;
  delete env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_BASE_URL;
  delete env.ANALYTIX_HUB_API_BASE_URL;
  delete env.VITE_ANALYTIX_HUB_API_BASE_URL;
  env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_FALLBACK_DISABLED = "1";
  env.ANALYTIX_AGENT_PLUGIN_MARKETPLACE_URL = "http://127.0.0.1:9/analytix-local-rc-hub-disabled";
  return {
    ...paths,
    ...state,
    remote_sync_mode: "preserve_existing_marketplace_after_local_connection_refused",
  };
}

async function findFreePort(host = "127.0.0.1") {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(0, host, () => {
      const address = server.address();
      const port = typeof address === "object" && address ? address.port : 0;
      server.close((error) => error ? reject(error) : resolve(port));
    });
  });
}

function scenarioEnvName(scenario, suffix) {
  return `ANALYTIX_FUNDS_DESKTOP_${scenario.envKey}_${suffix}`;
}

function scenarioForbiddenMarkers(scenario) {
  return [...new Set([
    ...parseList(process.env[scenarioEnvName(scenario, "FORBIDDEN_MARKERS")]),
    ...parseList(process.env[scenarioEnvName(scenario, "FORBIDDEN")]),
  ])];
}

function applyScenarioOrder(selected) {
  const order = parseList(process.env.ANALYTIX_FUNDS_DESKTOP_SCENARIO_ORDER);
  if (!order.length) return selected;
  const selectedByToken = new Map();
  for (const scenario of selected) {
    selectedByToken.set(scenario.envKey, scenario);
    selectedByToken.set(scenario.id, scenario);
  }
  const ordered = [];
  const used = new Set();
  for (const token of order) {
    const scenario = selectedByToken.get(token) || selectedByToken.get(token.toUpperCase());
    if (!scenario) {
      throw new Error(`unknown or unconfigured desktop acceptance scenario in ANALYTIX_FUNDS_DESKTOP_SCENARIO_ORDER: ${token}`);
    }
    if (!used.has(scenario.id)) {
      ordered.push(scenario);
      used.add(scenario.id);
    }
  }
  for (const scenario of selected) {
    if (!used.has(scenario.id)) ordered.push(scenario);
  }
  return ordered;
}

function configuredScenarios(options) {
  const enabled = isEnabled(process.env.ANALYTIX_FUNDS_DESKTOP_MULTITURN_ENABLED);
  const touched = SCENARIOS.filter((scenario) => text(process.env[scenarioEnvName(scenario, "QUESTION")]));
  let selected = enabled ? SCENARIOS : touched;
  selected = applyScenarioOrder(selected);
  if (options.limit > 0) selected = selected.slice(0, options.limit);
  const missing = [];
  if (!text(process.env.ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID)) {
    missing.push("ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID");
  }
  for (const scenario of selected) {
    if (!text(process.env[scenarioEnvName(scenario, "QUESTION")])) missing.push(scenarioEnvName(scenario, "QUESTION"));
    if (options.requireMarkers && parseList(process.env[scenarioEnvName(scenario, "MARKERS")]).length === 0) {
      missing.push(scenarioEnvName(scenario, "MARKERS"));
    }
  }
  if (!selected.length) {
    if (!options.activeCaseSyncCheck) {
      throw new Error("no desktop acceptance scenario configured; set ANALYTIX_FUNDS_DESKTOP_MULTITURN_ENABLED=1 or at least one scenario question env var");
    }
  }
  if (missing.length) {
    throw new Error(`missing desktop multi-turn env vars: ${missing.join(", ")}`);
  }
  return selected.map((scenario) => ({
    ...scenario,
    question: text(process.env[scenarioEnvName(scenario, "QUESTION")]),
    markers: parseList(process.env[scenarioEnvName(scenario, "MARKERS")]),
    expectedEffectiveAmount: text(process.env[scenarioEnvName(scenario, "EXPECTED_EFFECTIVE_AMOUNT")]),
    forbiddenMarkers: scenarioForbiddenMarkers(scenario),
  }));
}

function buildElectronArgs(userDataDir) {
  const args = process.platform === "linux" ? ["--no-sandbox"] : [];
  if (userDataDir) args.push(`--user-data-dir=${userDataDir}`);
  args.push(desktopAppDir);
  return args;
}

async function launchDesktop(options) {
  const port = options.backendUrl ? Number(new URL(options.backendUrl).port || 0) : await findFreePort();
  const backendUrl = options.backendUrl || `http://127.0.0.1:${port}`;
  const userDataDir = options.userDataDir || path.join(DEFAULT_OUTPUT_DIR, "electron-user-data-multiturn");
  fs.mkdirSync(userDataDir, { recursive: true });
  const env = {
    ...process.env,
    ANALYTIX_MANAGE_BACKEND: options.manageBackend ? "1" : "0",
    BACKEND_HOST: "127.0.0.1",
    BACKEND_PORT: String(port),
    VITE_ANALYTIX_DEV_AUTH_BYPASS: process.env.VITE_ANALYTIX_DEV_AUTH_BYPASS || "1",
    ANALYTIX_DEV_AUTH_BYPASS: process.env.ANALYTIX_DEV_AUTH_BYPASS || "1",
  };
  const localRcHub = applyLocalRcHubEnv(env, options);
  sanitizeAnalytixManagedCacheEnv(env, { repoRoot, populateMissingDefaults: true });
  const app = await electron.launch({
    executablePath: electronBinaryPath,
    args: buildElectronArgs(userDataDir),
    cwd: desktopAppDir,
    env,
  });
  const electronOutput = [];
  const child = app.process();
  child.stdout?.on("data", (chunk) => electronOutput.push(String(chunk)));
  child.stderr?.on("data", (chunk) => electronOutput.push(String(chunk)));
  return { app, backendUrl, port, userDataDir, electronOutput, localRcHub };
}

async function waitForShellPage(app, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    for (const page of app.windows()) {
      const hasBridge = await page.evaluate(() => Boolean(window.analytixDesktop)).catch(() => false);
      if (hasBridge) return page;
    }
    try {
      const page = await app.waitForEvent("window", { timeout: Math.min(1_000, Math.max(100, deadline - Date.now())) });
      const hasBridge = await page.evaluate(() => Boolean(window.analytixDesktop)).catch(() => false);
      if (hasBridge) return page;
    } catch {
      // keep polling
    }
  }
  throw new Error("Analytix shell window did not expose analytixDesktop");
}

async function waitForBackendReady(backendUrl, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let last = "";
  while (Date.now() < deadline) {
    try {
      const response = await httpGetText(`${backendUrl}/health`, 2_000);
      if (response.statusCode >= 200 && response.statusCode < 300) {
        return { ok: true, status: response.statusCode, body: response.body };
      }
      last = `http ${response.statusCode} ${response.body}`;
    } catch (error) {
      last = error instanceof Error ? error.message : String(error);
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`backend did not become ready at ${backendUrl}: ${last}`);
}

function httpGetText(url, timeoutMs) {
  return httpRequestText(url, timeoutMs, "GET");
}

function httpRequestText(url, timeoutMs, method = "GET") {
  return new Promise((resolve, reject) => {
    const request = http.request(url, { method, timeout: timeoutMs }, (response) => {
      const chunks = [];
      response.on("data", (chunk) => chunks.push(Buffer.from(chunk)));
      response.on("end", () => {
        resolve({
          statusCode: response.statusCode || 0,
          body: Buffer.concat(chunks).toString("utf8"),
        });
      });
    });
    request.on("timeout", () => {
      request.destroy(new Error(`http timeout after ${timeoutMs}ms`));
    });
    request.on("error", reject);
    request.end();
  });
}

function parseJsonText(value) {
  try {
    return JSON.parse(text(value));
  } catch {
    return { body: text(value) };
  }
}

function caseIdFromPayload(value) {
  const direct = text(
    value?.case_id ||
    value?.caseId ||
    value?.id ||
    value?.data?.case_id ||
    value?.data?.caseId ||
    value?.data?.id
  );
  if (direct) return direct;
  const matches = findObjects(value, (item) => text(item.case_id || item.caseId || item.id));
  for (const item of matches) {
    const candidate = text(item.case_id || item.caseId || item.id);
    if (candidate) return candidate;
  }
  return "";
}

function summarizeCaseItem(item) {
  return {
    case_id: caseIdFromPayload(item),
    case_name: text(item.case_name || item.caseName || item.name),
    case_number: text(item.case_number || item.caseNumber),
  };
}

function summarizeExplicitCaseSelection(payload, expectedCaseId = "") {
  const selectedCaseId = caseIdFromPayload(payload);
  return {
    ok: payload?.ok !== false && Boolean(selectedCaseId) && (!expectedCaseId || selectedCaseId === expectedCaseId),
    expected_case_id: expectedCaseId || undefined,
    selected_case_id: selectedCaseId,
    case_id_matches_expected: expectedCaseId ? selectedCaseId === expectedCaseId : undefined,
    status: payload?.status,
    error: payload?.error,
  };
}

async function prepareShell(shell, backendUrl) {
  await shell.addInitScript((context) => {
    window.__ANALYTIX_API_BASE__ = context.backendUrl;
    window.__ANALYTIX_WS_BASE__ = context.backendUrl.replace(/^http/u, "ws").replace(/\/+$/u, "") + "/ws/events";
    window.localStorage.setItem("analytix:theme-mode", "light");
    window.localStorage.setItem("analytix:auth-bypass:v1", JSON.stringify({
      token: "acceptance-token",
      expiresAt: "2027-01-01T00:00:00.000Z",
      checkedAt: "2026-01-01T00:00:00.000Z",
      user: { id: "desktop-acceptance", email: "desktop-acceptance@example.test", plan: "professional" },
    }));
    window.sessionStorage.setItem("analytix:desktop-startup-gate:completed", "1");
    window.location.hash = "#/agent";
  }, { backendUrl });
  await shell.evaluate((context) => {
    window.__ANALYTIX_API_BASE__ = context.backendUrl;
    window.__ANALYTIX_WS_BASE__ = context.backendUrl.replace(/^http/u, "ws").replace(/\/+$/u, "") + "/ws/events";
    window.localStorage.setItem("analytix:theme-mode", "light");
    window.localStorage.setItem("analytix:auth-bypass:v1", JSON.stringify({
      token: "acceptance-token",
      expiresAt: "2027-01-01T00:00:00.000Z",
      checkedAt: "2026-01-01T00:00:00.000Z",
      user: { id: "desktop-acceptance", email: "desktop-acceptance@example.test", plan: "professional" },
    }));
    window.sessionStorage.setItem("analytix:desktop-startup-gate:completed", "1");
    window.location.hash = "#/agent";
  }, { backendUrl });
  await shell.evaluate(() => {
    window.__ANALYTIX_AGENT_ELECTRON_NOTIFICATIONS__ = [];
    window.analytixDesktop?.onAgentRuntimeNotification?.((notification) => {
      window.__ANALYTIX_AGENT_ELECTRON_NOTIFICATIONS__.push(notification);
    });
  });
  await shell.evaluate(() => window.analytixDesktop?.agentDesktopNativeHostAttach?.({ surface: "agent", waitForLoad: true }));
}

async function activateAcceptanceCase(shell, backendUrl, caseId) {
  if (!caseId) return { ok: false, error: "missing case id" };
  const response = await httpRequestText(
    `${backendUrl}/api/v1/cases/${encodeURIComponent(caseId)}/activate`,
    10_000,
    "POST"
  );
  const body = parseJsonText(response.body);
  const activeCaseId = text(body?.data?.case_id || body?.case_id);
  if (response.statusCode >= 200 && response.statusCode < 300 && activeCaseId === caseId) {
    await shell.evaluate((selectedCaseId) => {
      window.localStorage.setItem("analytix:active-case-id", selectedCaseId);
    }, caseId);
    return { ok: true, case_id: activeCaseId };
  }
  return {
    ok: false,
    status: response.statusCode,
    expected_case_id: caseId,
    case_id: activeCaseId,
    body,
  };
}

async function readExplicitCaseSelection(backendUrl, caseId) {
  const explicitCaseId = text(caseId);
  if (!explicitCaseId) {
    throw new Error("explicit case_id is required before case selection lookup");
  }
  const query = new URLSearchParams({ case_id: explicitCaseId });
  const response = await httpGetText(`${backendUrl}/api/v1/cases/active?${query.toString()}`, 10_000);
  const body = parseJsonText(response.body);
  return response.statusCode >= 200 && response.statusCode < 300
    ? body
    : { ok: false, status: response.statusCode, body };
}

async function listCases(backendUrl) {
  const response = await httpGetText(`${backendUrl}/api/v1/cases?page=1&page_size=50&include_deleted=false`, 10_000);
  const body = parseJsonText(response.body);
  const items = Array.isArray(body?.data?.items)
    ? body.data.items
    : Array.isArray(body?.items)
      ? body.items
      : [];
  return {
    ok: response.statusCode >= 200 && response.statusCode < 300,
    status: response.statusCode,
    count: items.length,
    items: items.map(summarizeCaseItem).filter((item) => item.case_id),
    body: response.statusCode >= 200 && response.statusCode < 300 ? undefined : body,
  };
}

async function activateAndReadExplicitCase(shell, backendUrl, caseId) {
  const activation = await activateAcceptanceCase(shell, backendUrl, caseId)
    .catch((error) => ({ ok: false, error: text(error?.message || error) }));
  const selectionPayload = await readExplicitCaseSelection(backendUrl, caseId)
    .catch((error) => ({ ok: false, error: text(error?.message || error) }));
  return {
    activation,
    explicit_selection: summarizeExplicitCaseSelection(selectionPayload, caseId),
  };
}

async function runActiveCaseSyncCheck(shell, backendUrl, caseId, beforeActivation = null) {
  const explicitCaseId = text(caseId);
  if (!explicitCaseId) {
    throw new Error("explicit case_id is required before case selection sync check");
  }
  const beforePayload = beforeActivation || await readExplicitCaseSelection(backendUrl, explicitCaseId)
    .catch((error) => ({ ok: false, error: text(error?.message || error) }));
  const cases = await listCases(backendUrl)
    .catch((error) => ({ ok: false, error: text(error?.message || error), count: 0, items: [] }));
  const requestedSwitchId = text(process.env.ANALYTIX_FUNDS_DESKTOP_ACTIVE_CASE_SWITCH_ID);
  const switchTarget = requestedSwitchId
    ? cases.items.find((item) => item.case_id === requestedSwitchId)
    : cases.items.find((item) => item.case_id && item.case_id !== caseId);
  const result = {
    ok: false,
    before_activation: beforePayload.selected_case_id
      ? beforePayload
      : summarizeExplicitCaseSelection(beforePayload, explicitCaseId),
    cases,
    requested_case_id: caseId,
    requested_switch_case_id: requestedSwitchId || undefined,
    switch_target: switchTarget || null,
  };
  if (!cases.ok) {
    result.error = "failed to list cases for active-case switch verification";
    return result;
  }
  if (!switchTarget?.case_id) {
    result.error = requestedSwitchId
      ? "requested switch case was not found"
      : "no alternate case was available for active-case switch verification";
    return result;
  }
  result.switch = await activateAndReadExplicitCase(shell, backendUrl, switchTarget.case_id);
  result.restore = await activateAndReadExplicitCase(shell, backendUrl, explicitCaseId);
  result.ok = Boolean(
    result.switch.activation.ok &&
    result.switch.explicit_selection.ok &&
    result.restore.activation.ok &&
    result.restore.explicit_selection.ok
  );
  return result;
}

async function nativeSnapshot(app) {
  return app.evaluate(async ({ webContents }) => {
    const view = webContents.getAllWebContents().find((item) => String(item.getURL()).startsWith("app://-/"));
    if (!view) return { id: 0, url: "", text: "" };
    let textContent = "";
    try {
      textContent = await view.executeJavaScript("document.body ? document.body.innerText : ''", true);
    } catch {
      textContent = "";
    }
    return { id: view.id, url: view.getURL(), text: textContent };
  });
}

async function waitForNativeView(app, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let last = { id: 0, url: "", text: "" };
  while (Date.now() < deadline) {
    last = await nativeSnapshot(app);
    if (last.id) return last;
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`embedded analytixagent view did not appear; last=${JSON.stringify(last)}`);
}

async function nativeCodexFetch(app, method, body, timeoutMs = 20_000) {
  const snapshot = await waitForNativeView(app, timeoutMs);
  const response = await app.evaluate(async ({ webContents }, args) => {
    const view = webContents.fromId(args.id);
    return view.executeJavaScript(
      `(async () => {
        const requestMethod = ${JSON.stringify(args.method)};
        const requestBody = ${JSON.stringify(args.bodyJson)};
        const requestTimeoutMs = ${Number(args.timeoutMs)};
        const requestId = requestMethod + "-" + Date.now() + "-" + Math.random().toString(16).slice(2);
        return await new Promise((resolve) => {
          const timer = setTimeout(() => {
            window.removeEventListener("message", listener);
            resolve({ timeout: true });
          }, requestTimeoutMs);
          const listener = (event) => {
            if (event.data && event.data.type === "fetch-response" && event.data.requestId === requestId) {
              clearTimeout(timer);
              window.removeEventListener("message", listener);
              resolve(event.data);
            }
          };
          window.addEventListener("message", listener);
          window.electronBridge.sendMessageFromView({
            type: "fetch",
            requestId,
            method: "POST",
            url: "vscode://codex/" + requestMethod,
            body: requestBody
          });
        });
      })()`,
      true
    );
  }, { id: snapshot.id, method, bodyJson: JSON.stringify(body), timeoutMs });
  if (response?.timeout) throw new Error(`timed out waiting for vscode://codex/${method}`);
  return JSON.parse(String(response?.bodyJsonString || "{}"));
}

function sandboxPolicy(mode) {
  return text(mode) === "danger-full-access"
    ? { type: "dangerFullAccess" }
    : { type: "readOnly", networkAccess: false };
}

function threadIdFromStart(result) {
  const thread = result?.thread && typeof result.thread === "object" ? result.thread : result;
  return text(thread?.id || thread?.threadId || thread?.thread_id || thread?.sessionId);
}

async function startThread(shell, options) {
  const result = await shell.evaluate(async (params) => window.analytixDesktop?.agentThreadStart?.(params), {
    cwd: options.cwd,
    model: options.model,
    modelProvider: options.modelProvider,
    approvalPolicy: "never",
    approvalsReviewer: "user",
    sandbox: options.sandboxMode,
    persistExtendedHistory: true,
    ephemeral: false,
  });
  const threadId = threadIdFromStart(result);
  if (!threadId) throw new Error(`agentThreadStart returned no thread id: ${JSON.stringify(result)}`);
  return { threadId, result };
}

async function sendTurn(app, threadId, prompt, options) {
  return nativeCodexFetch(app, "turn/start", {
    threadId,
    input: [{ type: "text", text: prompt, text_elements: [] }],
    cwd: options.cwd,
    approvalPolicy: "never",
    approvalsReviewer: "user",
    sandboxPolicy: sandboxPolicy(options.sandboxMode),
    model: options.model,
    modelProvider: options.modelProvider,
    effort: "xhigh",
    serviceTier: null,
    summary: "none",
    personality: null,
    collaborationMode: null,
  }, 30_000);
}

function findObjects(value, predicate, results = []) {
  if (!value || typeof value !== "object") return results;
  if (predicate(value)) results.push(value);
  if (Array.isArray(value)) {
    for (const item of value) findObjects(item, predicate, results);
  } else {
    for (const item of Object.values(value)) findObjects(item, predicate, results);
  }
  return results;
}

function turnIdFromStart(result) {
  return text(result?.turn?.id || result?.turnId || result?.turn_id || result?.id);
}

function turnsFromThreadRead(read) {
  return findObjects(read, (item) => text(item.id || item.turnId || item.turn_id) && text(item.status) && Array.isArray(item.items));
}

function isTerminalTurnStatus(status) {
  return /^(completed|failed|cancelled|canceled|interrupted)$/iu.test(text(status));
}

async function readThreadForTurnWait(app, threadId) {
  return nativeCodexFetch(app, "thread/read", { threadId, includeTurns: true }, 30_000).catch((error) => {
    if (/not materialized yet|includeTurns is unavailable before first user message|rollout .* is empty/iu.test(text(error?.message || error))) {
      return null;
    }
    throw error;
  });
}

function threadStatusType(read) {
  return text(read?.thread?.status?.type || read?.status?.type || read?.status);
}

async function waitForTurn(app, threadId, turnId, options) {
  const deadline = Date.now() + options.timeoutMs;
  let lastRead = null;
  while (Date.now() < deadline) {
    lastRead = await readThreadForTurnWait(app, threadId);
    if (lastRead) {
      const turns = turnsFromThreadRead(lastRead);
      const targetTurn = turns.find((turn) => text(turn.id || turn.turnId || turn.turn_id) === turnId) || turns.at(-1);
      if (targetTurn && isTerminalTurnStatus(targetTurn.status)) {
        return { read: lastRead, turn: targetTurn, timeout: false };
      }
      const statusType = threadStatusType(lastRead);
      if (!turnId && /idle|completed/iu.test(statusType)) {
        return { read: lastRead, turn: targetTurn || null, timeout: false };
      }
    }
    await new Promise((resolve) => setTimeout(resolve, options.pollMs));
  }
  return { read: lastRead, turn: null, timeout: true };
}

function extractTextFragments(value, fragments = []) {
  if (typeof value === "string") {
    if (value.trim()) fragments.push(value);
    return fragments;
  }
  if (!value || typeof value !== "object") return fragments;
  if (Array.isArray(value)) {
    for (const item of value) extractTextFragments(item, fragments);
    return fragments;
  }
  for (const key of ["text", "markdown", "content", "message", "body"]) {
    if (typeof value[key] === "string" && value[key].trim()) fragments.push(value[key]);
  }
  for (const item of Object.values(value)) extractTextFragments(item, fragments);
  return fragments;
}

function collectAgentMessages(read, options = {}) {
  const dedupe = options.dedupe !== false;
  const seen = new Set();
  const messages = [];
  findObjects(read, (item) => {
    const type = text(item.type || item.kind);
    const role = text(item.role);
    return /agentMessage|assistant/iu.test(type) || role === "assistant";
  }).forEach((item) => {
    const combined = [...new Set(extractTextFragments(item))]
      .filter((part) => !/^(agentMessage|assistant|completed|inProgress|item-\d+|commentary|analysis|final|final_answer)$/iu.test(text(part)))
      .join("\n")
      .trim();
    if (combined && (!dedupe || !seen.has(combined))) {
      seen.add(combined);
      messages.push({ type: text(item.type || item.kind || item.role), text: combined });
    }
  });
  return messages;
}

const ANALYTIX_TOOL_NAME_PATTERN = /^(resolve_current_case|inspect_case_schema|profile_case_schema|explain_case_sql|diagnose_case_sql|preview_case_rows|inspect_workbench_history|case_sql_recipes|run_case_sql|investigate_pair_amount|rank_counterparties|funds_investigate|trace_subject_top_outflows|build_fund_flow_graph|analyze_holder_full|analyze_account_full|run_full_case_analysis|validate_report_claims|create_case_notebook)$/u;

function collectToolCallSummary(read, turn = null) {
  const scope = turn || read;
  const calls = [];
  const seen = new Set();
  findObjects(scope, (item) => {
    const source = item || {};
    const name = text(
      source.toolName
      || source.tool_name
      || source.tool
      || source.name
      || source.functionName
      || source.function_name
      || source.action
    );
    if (!ANALYTIX_TOOL_NAME_PATTERN.test(name)) return false;
    const type = text(source.type || source.kind || source.role);
    return /tool|mcp|function|call|item|result|completed|started/iu.test(type)
      || Boolean(source.arguments || source.input || source.output || source.result || source.status);
  }).forEach((item) => {
    const name = text(item.toolName || item.tool_name || item.tool || item.name || item.functionName || item.function_name || item.action);
    const status = text(item.status || item.state || item.type || item.kind);
    const key = `${name}::${status}::${text(item.id || item.callId || item.call_id || item.item_id)}`;
    if (seen.has(key)) return;
    seen.add(key);
    calls.push({ name, status });
  });
  const counts = {};
  for (const call of calls) {
    counts[call.name] = (counts[call.name] || 0) + 1;
  }
  return {
    calls: calls.slice(0, 40),
    counts,
    last_tool: calls.at(-1)?.name || "",
    workbench_tool_count: calls.filter((call) => /^(profile_case_schema|explain_case_sql|diagnose_case_sql|preview_case_rows|inspect_workbench_history|case_sql_recipes|run_case_sql)$/u.test(call.name)).length
  };
}

async function waitForNewAssistantMessage(app, threadId, baselineMessageCount, options) {
  const deadline = Date.now() + options.timeoutMs;
  let lastRead = null;
  while (Date.now() < deadline) {
    lastRead = await readThreadForTurnWait(app, threadId);
    if (lastRead) {
      const messages = collectAgentMessages(lastRead, { dedupe: false });
      const statusType = threadStatusType(lastRead);
      if (messages.length > baselineMessageCount && /idle|completed/iu.test(statusType)) {
        return { read: lastRead, timeout: false };
      }
    }
    await new Promise((resolve) => setTimeout(resolve, options.pollMs));
  }
  return { read: lastRead, timeout: true };
}

function checkVisibleText(scenario, answer, messages) {
  const looseAnswer = markerText(answer);
  const missingMarkers = scenario.markers.filter((marker) => {
    return !hasScenarioMarker(scenario, marker, answer, looseAnswer);
  });
  const missingKinds = scenario.requiredKinds
    .filter((kind) => !hasRequiredKind(kind, answer));
  const amountQualityFailures = [];
  const expectedEffectiveAmount = moneyMarker(scenario.expectedEffectiveAmount);
  if (expectedEffectiveAmount) {
    const effectiveMentions = [...answer.matchAll(/复核后(?:有效|已有流水支持)(?:金额|转账)?[^。\n|]{0,80}?([0-9][0-9,，,]*\.\d{2})\s*元/gu)]
      .map((match) => moneyMarker(match[1]))
      .filter(Boolean);
    if (!effectiveMentions.includes(expectedEffectiveAmount)) {
      amountQualityFailures.push(`复核后有效金额未按预期 ${scenario.expectedEffectiveAmount} 表述`);
    } else if (effectiveMentions[0] && effectiveMentions[0] !== expectedEffectiveAmount) {
      amountQualityFailures.push(`首个复核后有效金额为 ${effectiveMentions[0]}，不是预期 ${expectedEffectiveAmount}`);
    }
  }
  const leakage = [];
  const visibleBodies = messages.map((message) => message.text);
  for (const [index, body] of visibleBodies.entries()) {
    const labels = userVisibleLeakageLabels(body);
    for (const label of labels) leakage.push({ message_index: index + 1, label });
    for (const [label, pattern] of FORBIDDEN_VISIBLE_PATTERNS) {
      if (pattern.test(body)) leakage.push({ message_index: index + 1, label });
    }
    for (const marker of scenario.forbiddenMarkers) {
      if (body.includes(marker)) leakage.push({ message_index: index + 1, label: `forbidden marker: ${marker}` });
    }
  }
  return {
    ok: missingMarkers.length === 0 && missingKinds.length === 0 && amountQualityFailures.length === 0 && leakage.length === 0,
    missing_markers: missingMarkers,
    missing_required_kinds: missingKinds,
    amount_quality_failures: amountQualityFailures,
    user_visible_leakage: leakage,
  };
}

async function runScenario(app, threadId, scenario, options) {
  const startedAt = Date.now();
  const baselineRead = await readThreadForTurnWait(app, threadId);
  const baselineMessages = baselineRead ? collectAgentMessages(baselineRead, { dedupe: false }) : [];
  const turnStart = await sendTurn(app, threadId, scenario.question, options);
  const turnId = turnIdFromStart(turnStart);
  let waited = turnId
    ? await waitForTurn(app, threadId, turnId, options)
    : await waitForNewAssistantMessage(app, threadId, baselineMessages.length, options);
  if (!waited.timeout) {
    const records = waited.read ? collectAgentMessages(waited.read, { dedupe: false }) : [];
    if (records.length <= baselineMessages.length) {
      waited = await waitForNewAssistantMessage(app, threadId, baselineMessages.length, options);
    }
  }
  if (waited.timeout) {
    const summary = collectToolCallSummary(waited.read, waited.turn);
    return {
      id: scenario.id,
      ok: false,
      elapsed_ms: Date.now() - startedAt,
      turn_id: turnId,
      error: "turn timed out",
      tool_call_summary: summary,
    };
  }
  const messageRecords = collectAgentMessages(waited.read, { dedupe: false });
  const newMessages = messageRecords.slice(baselineMessages.length);
  const messages = collectAgentMessages(waited.read);
  const answer = newMessages.at(-1)?.text || messageRecords.at(-1)?.text || "";
  const quality = checkVisibleText(scenario, answer, messages);
  const summary = collectToolCallSummary(waited.read, waited.turn);
  return {
    id: scenario.id,
    ok: quality.ok,
    elapsed_ms: Date.now() - startedAt,
    turn_id: turnId,
    answer_chars: answer.length,
    message_count: messages.length,
    message_record_count: messageRecords.length,
    new_message_record_count: newMessages.length,
    markers: scenario.markers,
    required_kinds: scenario.requiredKinds,
    tool_call_summary: summary,
    ...quality,
    answer_preview: answer.slice(0, 800),
    visible_message_previews: messages.map((message, index) => ({
      index: index + 1,
      chars: message.text.length,
      preview: message.text.slice(0, 240),
    })),
  };
}

async function main() {
  loadLocalEnvFiles();
  const options = parseArgs(process.argv.slice(2));
  const scenarios = configuredScenarios(options);
  const caseId = text(process.env.ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID);
  const result = {
    ok: false,
    started_at: new Date().toISOString(),
    plugin_version: JSON.parse(fs.readFileSync(path.join(pluginRoot, ".codex-plugin", "plugin.json"), "utf8")).version,
    case_id_present: Boolean(caseId),
    scenario_count: scenarios.length,
    scenarios: [],
    output: path.resolve(options.output),
  };
  fs.mkdirSync(path.dirname(result.output), { recursive: true });
  let launched = null;
  try {
    if (!caseId) {
      throw new Error("ANALYTIX_FUNDS_DESKTOP_MULTITURN_CASE_ID must contain an explicit case_id");
    }
    launched = await launchDesktop(options);
    result.backend_url = launched.backendUrl;
    result.electron_user_data_dir = launched.userDataDir;
    if (launched.localRcHub) result.local_rc_hub = launched.localRcHub;
    const shell = await waitForShellPage(launched.app, 60_000);
    await prepareShell(shell, launched.backendUrl);
    await waitForBackendReady(launched.backendUrl, 90_000);
    const explicitCaseBeforeActivate = await readExplicitCaseSelection(launched.backendUrl, caseId)
      .catch((error) => ({ ok: false, error: text(error?.message || error) }));
    result.explicit_case_before_activate = summarizeExplicitCaseSelection(explicitCaseBeforeActivate, caseId);
    result.activate_case = await activateAcceptanceCase(shell, launched.backendUrl, caseId)
      .catch((error) => ({ ok: false, error: text(error?.message || error) }));
    if (!result.activate_case.ok) {
      throw new Error(`desktop acceptance failed to activate case ${caseId}: ${JSON.stringify(result.activate_case)}`);
    }
    const explicitCase = await readExplicitCaseSelection(launched.backendUrl, caseId)
      .catch((error) => ({ ok: false, error: text(error?.message || error) }));
    result.explicit_case_selection = summarizeExplicitCaseSelection(explicitCase, caseId);
    if (!result.explicit_case_selection.ok) {
      throw new Error(`desktop acceptance explicit case lookup mismatch: ${JSON.stringify(result.explicit_case_selection)}`);
    }
    if (options.activeCaseSyncCheck) {
      result.active_case_sync = await runActiveCaseSyncCheck(
        shell,
        launched.backendUrl,
        caseId,
        result.explicit_case_before_activate
      );
      if (!result.active_case_sync.ok) {
        throw new Error(`desktop active-case sync check failed: ${JSON.stringify(result.active_case_sync)}`);
      }
    }
    await waitForNativeView(launched.app, 90_000);
    if (scenarios.length) {
      const thread = await startThread(shell, options);
      result.thread_id = thread.threadId;
      for (const scenario of scenarios) {
        const scenarioResult = await runScenario(launched.app, thread.threadId, scenario, options);
        result.scenarios.push(scenarioResult);
        fs.writeFileSync(result.output, `${JSON.stringify({ ...result, ok: false, partial: true }, null, 2)}\n`);
        if (!scenarioResult.ok && !isEnabled(process.env.ANALYTIX_FUNDS_DESKTOP_ACCEPTANCE_CONTINUE_ON_FAIL)) {
          break;
        }
      }
    }
    result.ok = result.scenarios.length === scenarios.length &&
      result.scenarios.every((item) => item.ok) &&
      (!options.activeCaseSyncCheck || result.active_case_sync?.ok === true);
    result.finished_at = new Date().toISOString();
    fs.writeFileSync(result.output, `${JSON.stringify(result, null, 2)}\n`);
    if (options.json) console.log(JSON.stringify(result, null, 2));
    else console.log(`desktop multi-turn acceptance ${result.ok ? "passed" : "failed"}: ${result.output}`);
    if (!result.ok) process.exitCode = 1;
  } catch (error) {
    result.ok = false;
    result.error = error instanceof Error ? error.message : String(error);
    result.finished_at = new Date().toISOString();
    if (launched?.electronOutput) {
      result.electron_output_tail = launched.electronOutput.join("").split(/\r?\n/u).slice(-120);
    }
    fs.writeFileSync(result.output, `${JSON.stringify(result, null, 2)}\n`);
    if (options.json) console.log(JSON.stringify(result, null, 2));
    else console.error(`desktop multi-turn acceptance failed: ${result.output}\n${result.error}`);
    process.exitCode = 1;
  } finally {
    if (launched?.app) {
      await Promise.race([
        launched.app.close().catch(() => undefined),
        new Promise((resolve) => setTimeout(resolve, 5_000)),
      ]);
    }
  }
}

main().catch((error) => {
  const payload = {
    ok: false,
    error: error instanceof Error ? error.message : String(error),
    finished_at: new Date().toISOString(),
  };
  if (process.argv.includes("--json")) console.log(JSON.stringify(payload, null, 2));
  else console.error(payload.error);
  process.exitCode = 1;
});
