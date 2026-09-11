#!/usr/bin/env node

import fs from "node:fs";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import os from "node:os";
import path from "node:path";
import { createAgentOutputCompiler } from "../mcp/agent-output-compiler.mjs";
import {
  renderDiagnosticCardForAgent,
  renderDiagnosticCardForAudit,
} from "../mcp/card-renderer.mjs";
import { createMcpToolResultRuntime } from "../mcp/mcp-tool-result-runtime.mjs";
import { createToolCallRuntime } from "../mcp/tool-call-runtime.mjs";
import { supportsLocalDuckdbWorkbenchSkill } from "../mcp/duckdb-workbench-runtime.mjs";
import {
  assertNoUserVisibleLeakage,
  translateUserVisibleText,
  userVisibleLeakageLabels,
} from "../mcp/user-facing-language.mjs";
import { normalizeSkillArgs } from "../mcp/runtime-normalizers.mjs";
import { tools as MCP_TOOL_SCHEMAS } from "../mcp/tool-schemas.mjs";
import { PUBLICATION_RECEIPT_REQUIRED_TEXT } from "../mcp/report-publication-guard.mjs";
import { createMcpRequestHandlerRuntime } from "../mcp/mcp-request-handler-runtime.mjs";
import {
  FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY,
  FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
  FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI,
} from "../mcp/source-unavailable-boundary.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function moneyText(value) {
  const number = Number(value);
  return Number.isFinite(number) ? `${number.toFixed(2)} 元` : "";
}

function assertClean(label, value, requiredMarkers = []) {
  const rendered = String(value || "");
  const leaks = userVisibleLeakageLabels(rendered);
  if (leaks.length) {
    throw new Error(
      `${label} leaked internal language: ${leaks.join(", ")}\n${rendered}`,
    );
  }
  for (const marker of requiredMarkers) {
    assert(
      rendered.includes(marker),
      `${label} missing marker: ${marker}\n${rendered}`,
    );
  }
  return rendered;
}

function assertLeaks(label, value, expectedLabels = []) {
  const rendered = String(value || "");
  const leaks = userVisibleLeakageLabels(rendered);
  assert(
    leaks.length > 0,
    `${label} unexpectedly passed leakage scan\n${rendered}`,
  );
  for (const expected of expectedLabels) {
    assert(
      leaks.includes(expected),
      `${label} missing expected leak label: ${expected}; got ${leaks.join(", ")}\n${rendered}`,
    );
  }
  return leaks;
}

function valueAtPath(source, path) {
  return String(path || "")
    .split(".")
    .filter(Boolean)
    .reduce((value, key) => value?.[key], source);
}

function assertSupportEnvelope(label, value, requiredPaths = []) {
  const rendered = String(value || "");
  assert(
    !/^\s*[[{]/u.test(rendered),
    `${label} must be readable fact text, not a JSON support envelope\n${rendered}`,
  );
  assert(
    rendered.includes("案件事实摘录") || rendered.includes("研判状态:"),
    `${label} must identify a concise fact/source excerpt\n${rendered}`,
  );
  assert(
    rendered.length < 6000,
    `${label} is too large for model-visible support text: ${rendered.length}`,
  );
  for (const forbidden of [
    "answer_draft",
    "support_only",
    "final_answer_owned_by_focused_skill",
    "answer_card_complete",
    "delivery_state",
    "case_id",
    "经梳理",
    "结论先行",
    "一跳转账核验如下",
    "异常特征:",
    "异常与案件意义",
    "补证建议:",
    "核查建议:",
    "研判结论:",
    "特定双方资金往来核验事实材料",
    "既有报告研判结论复核材料",
  ]) {
    assert(
      !rendered.includes(forbidden),
      `${label} exposes final-answer prose: ${forbidden}\n${rendered}`,
    );
  }
  if (requiredPaths.length) {
    assert(
      rendered.split(/\r?\n/u).filter(Boolean).length >= 2,
      `${label} did not retain any visible fact lines for requested paths ${requiredPaths.join(", ")}\n${rendered}`,
    );
  }
  return rendered;
}

function readPluginText(relativePath) {
  return fs.readFileSync(
    new URL(`../${relativePath}`, import.meta.url),
    "utf8",
  );
}

function assertRuntimeConfigRemountSelfTest() {
  const scriptPath = fileURLToPath(
    new URL("./sync-runtime-cache.mjs", import.meta.url),
  );
  const output = execFileSync(
    process.execPath,
    [scriptPath, "--self-test-runtime-config", "--json"],
    {
      encoding: "utf8",
      maxBuffer: 1024 * 1024,
    },
  );
  const report = JSON.parse(output);
  assert(
    report.ok === true,
    `runtime config remount self-test failed\n${output}`,
  );
  assert(
    String(report.server_arg || "").includes(
      "/analytix-fund-analysis/0.16.9/mcp/server.mjs",
    ),
    `runtime config remount self-test did not patch MCP server path\n${output}`,
  );
  assert(
    String(report.skill_root || "").includes(
      "/analytix-fund-analysis/0.16.9/skills",
    ),
    `runtime config remount self-test did not patch skill root\n${output}`,
  );
}

function assertNoVisibleSourcePhrases(
  label,
  relativePath,
  forbiddenPhrases = [],
) {
  const source = readPluginText(relativePath);
  for (const phrase of forbiddenPhrases) {
    assert(
      !source.includes(phrase),
      `${label} exposes user-visible internal phrase: ${phrase}`,
    );
  }
}

function frontmatterDescription(relativePath) {
  const source = readPluginText(relativePath);
  const match = source.match(/^description:\s*([^\n#]+?)\s*$/mu);
  return String(match?.[1] || "").trim();
}

function quotedYamlValue(source, key) {
  const match = source.match(
    new RegExp(`^\\s*${key}:\\s*["']?([^"'\\n]+)["']?\\s*$`, "mu"),
  );
  return String(match?.[1] || "").trim();
}

function agentInterfaceText(relativePath) {
  const source = readPluginText(relativePath);
  return [
    quotedYamlValue(source, "display_name"),
    quotedYamlValue(source, "short_description"),
    quotedYamlValue(source, "default_prompt"),
  ]
    .filter(Boolean)
    .join(" ");
}

function assertDiscoveryText(label, value, requiredMarkers = []) {
  const rendered = assertClean(label, value, requiredMarkers);
  for (const forbidden of [
    "Quick Fact",
    "Workbench",
    "MCP",
    "Mermaid",
    "case_id",
    "claim",
    "debug",
    "JSON",
    "Shell",
    "SKILL.md",
  ]) {
    assert(
      !rendered.includes(forbidden),
      `${label} exposes discovery-internal term: ${forbidden}\n${rendered}`,
    );
  }
  return rendered;
}

function assertDesktopRoutingInstruction(
  label,
  relativePath,
  requiredMarkers = [],
) {
  const rendered = readPluginText(relativePath);
  for (const marker of requiredMarkers) {
    assert(
      rendered.includes(marker),
      `${label} missing desktop routing marker: ${marker}`,
    );
  }
  assert(
    !rendered.includes("我将使用 quick-fact"),
    `${label} must not teach visible quick-fact progress`,
  );
  assert(
    !rendered.includes("我将使用“quick-fact”"),
    `${label} must not teach visible quick-fact progress`,
  );
}

async function assertP0ProductionSurface() {
  const mcpManifest = JSON.parse(readPluginText(".mcp.json"));
  assert(
    mcpManifest?.mcpServers?.analytix_funds?.disabled === true,
    "funds production manifest must disable process resolution while source authority is unavailable",
  );
  const runtime = createMcpRequestHandlerRuntime({
    serverName: "analytix_funds",
    serverVersion: "0.16.16",
    pluginRoot: fileURLToPath(new URL("..", import.meta.url)),
    env: { ANALYTIX_FUNDS_EXPOSE_ALL_TOOLS: "true" },
  });
  await runtime.handleRequest({
    method: "initialize",
    params: {
      protocolVersion: "2025-11-25",
      capabilities: {},
      clientInfo: { name: "user-visible-p0-contract", version: "1" },
    },
  });
  await runtime.handleNotification({
    method: "notifications/initialized",
    params: {},
  });

  const toolList = await runtime.handleRequest({ method: "tools/list", params: {} });
  const toolNames = (toolList?.tools || []).map((tool) => tool.name);
  assert(
    JSON.stringify(toolNames) ===
      JSON.stringify(["count_case_rows", "analyze_account_flows"]),
    "disabled plugin protocol closure must remain exactly the two host-capture schemas even when full discovery is requested",
  );

  const resourceList = await runtime.handleRequest({ method: "resources/list", params: {} });
  assert(
    JSON.stringify((resourceList?.resources || []).map((item) => item.uri)) ===
      JSON.stringify([FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI]),
    "production resources/list must expose only the fixed source-unavailable boundary",
  );
  const resource = await runtime.handleRequest({
    method: "resources/read",
    params: { uri: FUNDS_SOURCE_UNAVAILABLE_RESOURCE_URI },
  });
  const resourceText = String(resource?.contents?.[0]?.text || "");
  assertClean("source-unavailable resource", resourceText, [
    FUNDS_FIXED_SOURCE_UNAVAILABLE_BOUNDARY,
    "能力缺口",
    "未检查范围",
    "补证动作",
  ]);

  const promptList = await runtime.handleRequest({ method: "prompts/list", params: {} });
  assert(
    JSON.stringify((promptList?.prompts || []).map((item) => item.name)) ===
      JSON.stringify([FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME]),
    "production prompts/list must expose only the fixed boundary prompt",
  );

  for (const [label, message, expected] of [
    [
      "unadvertised fact tool",
      { method: "tools/call", params: { name: "get_current_case", arguments: {} } },
      /Unknown or unadvertised tool/u,
    ],
    [
      "boundary prompt arguments",
      {
        method: "prompts/get",
        params: {
          name: FUNDS_SOURCE_UNAVAILABLE_PROMPT_NAME,
          arguments: { case_id: "must-not-be-consumed" },
        },
      },
      /does not accept arguments/u,
    ],
  ]) {
    let rejected = false;
    try {
      await runtime.handleRequest(message);
    } catch (error) {
      rejected = Number(error?.code) === -32602 && expected.test(String(error?.message || error));
    }
    assert(rejected, `${label} must fail closed with JSON-RPC -32602`);
  }
  await runtime.close();
}

assertNoVisibleSourcePhrases(
  "claim review MCP card source",
  "mcp/tool-call-runtime.mjs",
  ["既有报告 claim 复核事实卡"],
);
assertNoVisibleSourcePhrases(
  "claim review diagnostic source",
  "mcp/claim-review-diagnostic-runtime.mjs",
  ["用户 claim 复核"],
);
assertNoVisibleSourcePhrases(
  "claim verifier protocol source",
  "mcp/claim-verifier-protocol.mjs",
  ["claim 复核", "每条 claim", "pass / block", "verified / corrected"],
);
assertNoVisibleSourcePhrases(
  "agent payload source",
  "mcp/agent-payload-compiler.mjs",
  ["claim 仍需"],
);
assertNoVisibleSourcePhrases(
  "case scope graph source",
  "mcp/case-scope-map-runtime.mjs",
  ["claim validation"],
);

const manifest = JSON.parse(readPluginText(".codex-plugin/plugin.json"));
assertDiscoveryText(
  "plugin marketplace long description",
  manifest.interface?.longDescription,
  [
    "同一案件",
    "同一轮次",
    "同一上下文版本",
    "同一数据快照",
    "不分析或发布金额",
    "未检查范围",
    "补证建议",
  ],
);
assertDiscoveryText(
  "quick-fact discovery description",
  frontmatterDescription("skills/quick-fact/SKILL.md"),
  [
    "姓名/主体是否和案件资金有关联",
    "严格命中",
    "模糊命中",
    "排除对象",
    "不能确认",
  ],
);
assertDiscoveryText(
  "fund-tracing discovery description",
  frontmatterDescription("skills/fund-tracing/SKILL.md"),
  ["资金来源", "资金穿透图", "未调取端点", "资金断点"],
);
assertDiscoveryText(
  "case-workbench discovery description",
  frontmatterDescription("skills/case-workbench/SKILL.md"),
  ["专项资金核算", "对公转个人重算", "摘要缺失纳入总额", "无法识别性质"],
);
assertDiscoveryText(
  "report-builder discovery description",
  frontmatterDescription("skills/report-builder/SKILL.md"),
  ["基本情况", "资金流入流出", "重点对手方", "异常特征", "资金去向"],
);

const agentInterfaceChecks = [
  [
    "root agent interface",
    "agents/openai.yaml",
    ["涉案资金研判", "核验意见", "补证建议"],
  ],
  [
    "index agent interface",
    "skills/index/agents/openai.yaml",
    ["案件资金研判总入口", "经侦"],
  ],
  [
    "quick-fact agent interface",
    "skills/quick-fact/agents/openai.yaml",
    ["资金事实快查", "严格命中", "不能确认"],
  ],
  [
    "pair-amount agent interface",
    "skills/pair-amount-investigation/agents/openai.yaml",
    ["特定双方资金往来核验", "金额争议", "取证"],
  ],
  [
    "delivery-qc agent interface",
    "skills/delivery-qc/agents/openai.yaml",
    ["研判材料交付复核", "核验意见", "补证动作"],
  ],
  [
    "fund-tracing agent interface",
    "skills/fund-tracing/agents/openai.yaml",
    ["资金来源去向追踪", "未调取端点", "资金断点"],
  ],
  [
    "case-workbench agent interface",
    "skills/case-workbench/agents/openai.yaml",
    ["专项资金核算", "只读", "补证建议"],
  ],
  [
    "visual-evidence agent interface",
    "skills/visual-evidence/agents/openai.yaml",
    ["证据图表附件", "经侦"],
  ],
  [
    "report-builder agent interface",
    "skills/report-builder/agents/openai.yaml",
    ["经侦研判报告", "能力当前不可用", "补证动作"],
  ],
  [
    "claim-review agent interface",
    "skills/claim-review/agents/openai.yaml",
    ["报告事实结论复核", "金额统计范围", "资金路径"],
  ],
  [
    "analysis-critique agent interface",
    "skills/analysis-critique/agents/openai.yaml",
    ["研判完整性复核", "证据不足"],
  ],
  [
    "graph-visualization agent interface",
    "skills/graph-visualization/agents/openai.yaml",
    ["资金流向图谱", "资金穿透图", "未调取端点"],
  ],
  [
    "account-dossier agent interface",
    "skills/account-dossier/agents/openai.yaml",
    ["账户资金画像", "异常特征"],
  ],
  [
    "subject-dossier agent interface",
    "skills/subject-dossier/agents/openai.yaml",
    ["主体资金画像", "资金流入流出"],
  ],
  [
    "counterparty-analysis agent interface",
    "skills/counterparty-analysis/agents/openai.yaml",
    ["重点对手方研判", "补证优先级"],
  ],
  [
    "data-quality agent interface",
    "skills/data-quality/agents/openai.yaml",
    ["数据质量与口径复核", "金额统计范围"],
  ],
  [
    "evidence-request agent interface",
    "skills/evidence-request/agents/openai.yaml",
    ["补充取证清单", "调证优先级"],
  ],
  [
    "full-case-analysis agent interface",
    "skills/full-case-analysis/agents/openai.yaml",
    ["全案资金研判", "证据缺口"],
  ],
  [
    "investigation-lab agent interface",
    "skills/investigation-lab/agents/openai.yaml",
    ["异常资金线索研判", "补证建议"],
  ],
  [
    "case-context agent interface",
    "skills/case-context/agents/openai.yaml",
    ["案件数据范围核验", "数据覆盖"],
  ],
];
for (const [label, relativePath, markers] of agentInterfaceChecks) {
  assertDiscoveryText(label, agentInterfaceText(relativePath), markers);
}

assertDesktopRoutingInstruction(
  "root skill desktop first action",
  "skills/analytix-fund-analysis/SKILL.md",
  [
    "桌面端入口边界",
    "自然语言 shortcut /",
    "不得替代",
    "普通案件任务默认不要发送可见进度",
    "首条可见 assistant 消息不得是计划、承诺、技能公告或流程说明",
    "只允许逐字写 `正在核验当前案件事实。`",
    "不得向普通办案用户展示能力名、技能文件名、MCP、Shell、命令、路径、资源清单",
  ],
);
assertDesktopRoutingInstruction(
  "quick-fact desktop progress contract",
  "skills/quick-fact/SKILL.md",
  [
    "Desktop first-response rule",
    "`正在核验当前案件事实。`",
    "does not own Pair Amount",
  ],
);
for (const [label, relativePath, markers] of [
  [
    "graph desktop source boundary",
    "skills/graph-visualization/SKILL.md",
    [
      "Desktop/source boundary",
      "`正在核验当前案件事实。`",
      "investigate_pair_amount",
      "trace_subject_top_outflows",
      "never search local `*.duckdb`",
    ],
  ],
  [
    "subject desktop source boundary",
    "skills/subject-dossier/SKILL.md",
    [
      "Desktop/source boundary",
      "`正在核验当前案件事实。`",
      "analyze_holder_full",
      "local DuckDB",
    ],
  ],
  [
    "fund tracing desktop source boundary",
    "skills/fund-tracing/SKILL.md",
    [
      "Desktop/source boundary",
      "`正在核验当前案件事实。`",
      "trace_subject_top_outflows",
      "local DuckDB",
    ],
  ],
  [
    "full-case desktop source boundary",
    "skills/full-case-analysis/SKILL.md",
    [
      "Desktop/source boundary",
      "`正在核验当前案件事实。`",
      "run_full_case_analysis",
      "local DuckDB",
    ],
  ],
  [
    "report desktop source boundary",
    "skills/report-builder/SKILL.md",
    [
      "Desktop/source boundary",
      "`正在核验当前案件事实。`",
      "validate_report_claims",
      "local DuckDB",
    ],
  ],
]) {
  assertDesktopRoutingInstruction(label, relativePath, markers);
}

const fundsInvestigateDescription = String(
  MCP_TOOL_SCHEMAS.find((tool) => tool.name === "funds_investigate")
    ?.description || "",
);
const pairAmountToolIndex = MCP_TOOL_SCHEMAS.findIndex(
  (tool) => tool.name === "investigate_pair_amount",
);
const fundsInvestigateIndex = MCP_TOOL_SCHEMAS.findIndex(
  (tool) => tool.name === "funds_investigate",
);
assert(
  pairAmountToolIndex >= 0 && fundsInvestigateIndex >= 0,
  "Pair Amount and funds_investigate schemas must both remain in the dormant development inventory",
);
assert(
  pairAmountToolIndex < fundsInvestigateIndex,
  "dormant investigate_pair_amount schema must precede funds_investigate for any future reviewed activation",
);
assert(
  fundsInvestigateDescription.includes("自然语言 shortcut / navigator"),
  "funds_investigate description must be navigator/support language",
);
assert(
  fundsInvestigateDescription.includes(
    "不得成为普通问法的唯一首跳或最终成稿 owner",
  ),
  "funds_investigate description must reject final ownership",
);
assert(
  fundsInvestigateDescription.includes("不要用于“A 转给 B 多少钱”"),
  "funds_investigate description must reject Pair Amount first-call ownership",
);
assert(
  !fundsInvestigateDescription.includes("普通桌面问法应优先使用本工具"),
  "funds_investigate description must not prioritize ordinary desktop questions",
);
assert(
  fundsInvestigateDescription.includes("不得为了当前案件事实先搜索工作区文件"),
  "funds_investigate description must block workspace-file exploration",
);
const pairAmountToolDescription = String(
  MCP_TOOL_SCHEMAS.find((tool) => tool.name === "investigate_pair_amount")
    ?.description || "",
);
assert(
  pairAmountToolDescription.includes("特定双方资金往来核验一等事实入口"),
  "investigate_pair_amount description must identify first-class Pair Amount support",
);
assert(
  pairAmountToolDescription.includes("普通双方资金往来金额首答优先调用本入口"),
  "investigate_pair_amount description must be the ordinary first Pair Amount support call",
);
assert(
  pairAmountToolDescription.includes("主要集中转账"),
  "investigate_pair_amount description must include focused transfer concentration support",
);
assertDiscoveryText(
  "investigate_pair_amount tool description",
  pairAmountToolDescription,
  ["特定双方资金往来核验", "主要集中转账", "逐笔交易候选", "公安经侦材料"],
);

const compiler = createAgentOutputCompiler({ moneyText });
await assertP0ProductionSurface();
assertRuntimeConfigRemountSelfTest();
const frontdoorRuntimeSource = readPluginText("mcp/frontdoor-runtime.mjs");
assert(
  frontdoorRuntimeSource.includes(
    "withInternalRuntimeContext(initialRuntimeArgs, input)",
  ),
  "frontdoor runtime must pass runtime context into internal safeSkill calls",
);
assert(
  (frontdoorRuntimeSource.match(/\bsafeSkill\(/gu) || []).length === 1,
  "frontdoor runtime must route internal skill calls through frontdoorSafeSkill context wrapper",
);
assert(
  (frontdoorRuntimeSource.match(/\bfrontdoorSafeSkill\(/gu) || []).length > 5,
  "frontdoor runtime context wrapper must cover internal skill calls",
);
const normalizedFullCaseArgs = normalizeSkillArgs("run_full_case_analysis", {
  write_report: true,
  _analytix: {
    workspaceRealPath: "/Users/sun/Projects/江苏航案件分析",
    threadId: "thr_case",
    turnId: "turn_case",
  },
});
assert(
  normalizedFullCaseArgs.write_report === true,
  "run_full_case_analysis normalizer must preserve write_report",
);
assert(
  normalizedFullCaseArgs._analytix?.workspaceRealPath ===
    "/Users/sun/Projects/江苏航案件分析",
  "run_full_case_analysis normalizer must preserve runtime workspace context",
);

const directTranslation = translateUserVisibleText(
  "quick-fact SKILL.md List MCP resources Shell Codex cat /Users/example/.analytix/runtime grep /Users/example/.analytix follow-up Controlled Workbench run_case_sql create_case_notebook export_cleaned_case_data case_id Mermaid supported seed supported transaction edge supported support needs_review candidate blocked partial validation state delivery contract source-of-truth explicit_case_lookup backend_active_case failed after transient retry semantic MCP tool runtime context source envelope artifact runtime claim evidence ledger graph node graph edge raw rows JSON debug Context must write claim_1 status=ok targeted Mini-Review duplicate evidence pack deterministic fact result continuation rescope hypothesis analysis_txn_detail_idx holder_analysis ranking high-confidence same-fact txn_id compare_analysis_scopes account cluster time cluster amount cluster focus_cluster outside-cluster 可疑簇 本工具可支持 模型应 前门",
);
assertClean("direct translation", directTranslation, [
  "资金事实快查",
  "专项规则",
  "读取可用核验能力",
  "执行过程",
  "研判助手",
  "内部检索过程已隐藏",
  "下一步追查",
  "专项资金核算",
  "清洗明细导出",
  "案件编号",
  "资金流向图",
  "可证实资金链路",
  "已有数据支持",
  "支持依据",
  "需补证",
  "线索",
  "当前证据不足",
  "部分可见",
  "核验状态",
  "交付要求",
  "权威来源",
  "显式案件编号核验",
  "当前案件确认",
  "重试后仍未完成",
  "案件事实核验环节",
  "核验上下文",
  "本次依据与统计范围",
  "交付材料",
  "研判结论",
  "证据记录",
  "研判依据",
  "第1项",
  "核验状态：已完成",
  "定向",
  "简要核验材料",
  "重复",
  "证据包",
  "确定性事实",
  "核验结果",
  "续查",
  "重算范围",
  "假设",
  "清洗/分析索引",
  "主体研判",
  "排行",
  "高置信同事实",
  "交易号",
  "对比核验",
  "相关账户（列明账号）",
  "时间集中",
  "金额集中",
  "主要集中转账",
  "集中交易以外逐笔往来",
  "可疑线索集合",
  "本次核验可支持",
]);

assertLeaks("visible case id value", "案件ID: 38dc50241996", ["case id value"]);
const redactedCaseIdText = translateUserVisibleText(
  "案件编号：38dc50241996；case_id: 38dc50241996。",
);
assertClean("case id value translation", redactedCaseIdText, ["当前案件"]);
assert(
  !/[0-9a-f]{8,}/iu.test(redactedCaseIdText),
  `case id value translation retained opaque id\n${redactedCaseIdText}`,
);

assertLeaks(
  "candidate edge visible leak",
  "Candidate Edge: A -> B；Confirmed Edge: B -> C；候选边已确认。",
  ["candidate/confirmed edge"],
);
const translatedEdgeStatusText = translateUserVisibleText(
  "Candidate Edge: A -> B；Confirmed Edge: B -> C；候选边已确认。",
);
assertClean("candidate edge status translation", translatedEdgeStatusText, [
  "待复核链路",
  "已有流水支持的链路",
  "相关链路需继续补证固定",
]);
assert(
  !/candidate\s+edge|confirmed\s+edge|候选边[^。；\n]{0,12}确认/iu.test(
    translatedEdgeStatusText,
  ),
  `candidate edge status translation retained internal status\n${translatedEdgeStatusText}`,
);

const clusterTranslation = translateUserVisibleText(
  "账户簇、时间簇、金额簇、交易簇、资金簇、focus_cluster、outside-cluster、account clusters、time clusters、amount clusters、可疑簇、未知簇",
);
assertClean("cluster wording translation", clusterTranslation, [
  "相关账户（列明账号）",
  "时间集中",
  "金额集中",
  "交易组",
  "资金集中流向",
  "主要集中转账",
  "集中交易以外逐笔往来",
  "可疑线索集合",
  "未知集合",
]);
assertLeaks(
  "cluster wording leak sample",
  "账户簇 focus_cluster outside-cluster amount cluster",
  ["经侦禁用术语-簇", "cluster"],
);

const legacySupportCardTranslation = translateUserVisibleText(
  "金额核验小研判：当前案件可见。金额口径、口径差异说明、口径提示、全期间同名收款人统计、全期间同名对手户名聚合口径、重点收款账号统计、核心窗口口径。",
);
assertClean(
  "legacy support-card wording translation",
  legacySupportCardTranslation,
  [
    "经梳理",
    "已调取流水显示",
    "金额核验情况",
    "竞争金额业务解释",
    "补证方向",
    "同名收款人核验结果",
    "重点收款账户流水",
    "集中交易窗口",
  ],
);
for (const forbidden of [
  "金额核验小研判",
  "金额口径",
  "金额核算说明",
  "口径差异说明",
  "口径提示",
  "全期间",
  "全期间同名收款人统计",
  "同名收款人匹配结果",
  "重点收款账号统计",
  "重点收款账户情况",
  "重点收款账户核验",
  "当前案件可见",
  "当前已调取数据中",
  "核心窗口口径",
  "核心交易窗口",
]) {
  assert(
    !legacySupportCardTranslation.includes(forbidden),
    `legacy support-card wording survived translation: ${forbidden}\n${legacySupportCardTranslation}`,
  );
}

assertLeaks(
  "source boundary label leak sample",
  "来源边界：仅按当前已调取流水核验。",
  ["来源边界"],
);
const sourceBoundaryTranslation =
  translateUserVisibleText("来源边界：仅按当前已调取流水核验。");
assertClean("source boundary translation", sourceBoundaryTranslation, [
  "本次依据与统计范围",
]);
assert(
  !sourceBoundaryTranslation.includes("来源边界"),
  `source boundary label survived translation\n${sourceBoundaryTranslation}`,
);

assertLeaks(
  "ai slop opener closer leak sample",
  "结论先说：值得注意的是，本案资金关系具有重要意义。综上所述，不难发现该事实提供了有力支撑。",
  ["AI套话开场", "AI套话收尾", "空泛案件意义"],
);

const aiSlopTranslation = translateUserVisibleText(
  "结论先说：值得注意的是，本案资金关系具有重要意义，并提供了有力支撑、奠定基础。总的来说，不难发现该线索形成闭环。",
);
assertClean("ai slop translation", aiSlopTranslation, [
  "对固定资金关系有意义",
  "可作为后续核查依据",
  "可作为后续核查起点",
  "形成可复核链条",
]);
for (const forbidden of [
  "结论先说",
  "直接说结论",
  "值得注意的是",
  "总的来说",
  "不难发现",
  "具有重要意义",
  "有力支撑",
  "奠定基础",
  "形成闭环",
]) {
  assert(
    !aiSlopTranslation.includes(forbidden),
    `ai slop wording survived translation: ${forbidden}\n${aiSlopTranslation}`,
  );
}

assertLeaks(
  "ai binary uplift leak sample",
  "这不仅是普通转账，更是隐藏资金链条的重要闭环。",
  ["AI二元拔高"],
);
assertLeaks(
  "ai mechanical tricolon leak sample",
  "首先核验金额，其次分析路径，最后形成结论。",
  ["AI三段式机械列举"],
);
assertLeaks(
  "unsourced authority leak sample",
  "业内人士认为该资金链条已经坐实。",
  ["无源权威铺垫"],
);

assertLeaks(
  "ai meta tail leak sample",
  "下面我会梳理本案事实。希望这对你有帮助，如需我可以继续展开。",
  ["AI元评论尾巴"],
);
assertLeaks(
  "self media consulting leak sample",
  "该资金链路深刻揭示了案件底层逻辑，是关键抓手，可实现降本增效和全链路闭环。",
  ["自媒体洞察腔", "咨询化泛化"],
);

const caseRegisterTranslation = translateUserVisibleText(
  "下面我会梳理本案事实。该资金链路深刻揭示了案件底层逻辑，是关键抓手，可实现降本增效和全链路闭环。希望这对你有帮助。",
);
assertClean("case register translation", caseRegisterTranslation, [
  "资金链路",
  "提示",
  "资金关系",
  "重点核查方向",
  "提高核查效率",
]);
for (const forbidden of [
  "下面我会",
  "希望这对你有帮助",
  "深刻揭示",
  "底层逻辑",
  "关键抓手",
  "降本增效",
  "全链路闭环",
]) {
  assert(
    !caseRegisterTranslation.includes(forbidden),
    `case register wording survived translation: ${forbidden}\n${caseRegisterTranslation}`,
  );
}

assertLeaks(
  "public security empty officialese leak sample",
  "本项工作应高度重视、压实责任、形成合力、纵深推进，取得实效并夯实坚实基础。",
  ["公安公文空话", "咨询化泛化"],
);
const officialeseTranslation = translateUserVisibleText(
  "本项工作应高度重视、压实责任、形成合力、纵深推进，取得实效并形成强力支撑。",
);
assertClean("public security officialese translation", officialeseTranslation, [
  "需围绕证据核查",
  "明确补证责任",
  "协同核查",
  "继续核查",
  "形成可复核结果",
  "核查依据",
]);
for (const forbidden of [
  "高度重视",
  "压实责任",
  "形成合力",
  "纵深推进",
  "取得实效",
  "强力支撑",
]) {
  assert(
    !officialeseTranslation.includes(forbidden),
    `officialese survived translation: ${forbidden}\n${officialeseTranslation}`,
  );
}

assertLeaks(
  "legal overclaim leak sample",
  "银行流水已坐实违法所得，并证明张某实际控制该账户，最终归属已锁定。",
  ["法律定性越界"],
);
assertClean(
  "legal downgrade safe wording",
  "现有材料尚不能认定实际控制，不能证明最终归属，需补证固定。",
  ["尚不能认定实际控制", "不能证明最终归属"],
);
const legalOverclaimTranslation = translateUserVisibleText(
  "银行流水已坐实违法所得，并证明张某实际控制该账户，最终归属已锁定。洗钱事实成立。",
);
assertClean("legal overclaim translation", legalOverclaimTranslation, [
  "需结合证据固定",
  "资金性质待补证",
  "相关控制/归属关系需补证固定",
  "相关法律评价需另行结合证据判断",
]);
for (const forbidden of [
  "坐实",
  "违法所得",
  "实际控制",
  "最终归属",
  "锁定",
  "洗钱事实成立",
]) {
  assert(
    !legalOverclaimTranslation.includes(forbidden),
    `legal overclaim survived translation: ${forbidden}\n${legalOverclaimTranslation}`,
  );
}

assertLeaks(
  "desktop progress raw leak sample",
  "我将使用“quick-fact”案件资金快查流程。Read SKILL.md. List MCP resources. Shell: grep /Users/example/.analytix",
  ["quick-fact", "SKILL.md", "List MCP resources", "Shell", "local path"],
);

assertLeaks(
  "desktop report artifact absolute path leak sample",
  [
    "报告文件：`/Users/example/Analytix交付区/公安经侦资金研判报告.docx`",
    "资金流向图：`/var/folders/tmp/资金流向图.png`",
  ].join("\n"),
  ["local path", "absolute artifact path"],
);

const reportArtifactPathTranslation = translateUserVisibleText(
  [
    "报告文件：`/Users/example/Analytix交付区/公安经侦资金研判报告.docx`",
    "附件表：`/var/folders/tmp/资金研判附件表.xlsx`",
  ].join("\n"),
);
assertClean(
  "desktop report artifact path translation",
  reportArtifactPathTranslation,
  ["已生成至 Analytix 交付区"],
);
assert(
  !reportArtifactPathTranslation.includes("/Users/") &&
    !reportArtifactPathTranslation.includes("/var/"),
  `artifact path survived translation:\n${reportArtifactPathTranslation}`,
);

assertLeaks(
  "desktop pair amount skill announcement leak sample",
  "我会使用“pair-amount-investigation”资金往来核验流程，核对合成主体甲与合成主体乙之间的转账金额口径。",
  ["双方资金往来核验流程公告泄漏"],
);

assertLeaks(
  "desktop SQL executor progress leak sample",
  "该环境的专项 SQL 执行器未返回可用明细结果；我改用已返回的定向核验事实、排行交叉校验和重复风险复核来成稿。",
  ["SQL", "执行器", "专项明细查询"],
);

assertLeaks(
  "desktop progress scope wording leak sample",
  "我会按“清洗明细导出/证据表交付”口径先读取对应技能说明。",
  ["进度口径泄漏", "读取技能说明泄漏"],
);

assertLeaks(
  "desktop first progress plan leak sample",
  "我会按当前案件资金研判流程先读取案件数据口径与可用交易明细，再做两方金额核验并整理成经侦材料式结论。",
  ["计划式进度泄漏", "进度口径泄漏", "读取技能说明泄漏"],
);

assertLeaks(
  "desktop focused skill announcement leak sample",
  "我将使用 `pair-amount-investigation`（两方金额核验）技能，先定位当前案件数据与统计口径，再直接给出经侦研判结论。",
  ["计划式进度泄漏"],
);

assertLeaks(
  "real ui pair amount workflow progress leak sample",
  "我将使用“两方金额核验”流程，先核对当前案件数据中“示例付款人→示例收款人”的转账明细与统计口径，再给出金额结论和证据边界。",
  ["计划式进度泄漏", "双方资金往来核验流程公告泄漏", "先核对统计口径泄漏"],
);

assertLeaks(
  "desktop focused workflow announcement leak sample",
  "我会使用“pair-amount-investigation”流程做两方金额核验，并只基于当前案件可复核数据给出经侦研判结论。\nitem-2\ncommentary",
  ["计划式进度泄漏"],
);

assertLeaks(
  "visual artifact state-word leak sample",
  [
    "示例付款人 -> 示例收款人 2100万元来源去向图",
    "supported : 可解释确定交易边",
    "needs_review : 缺端点/需补证",
    "candidate / edge_status / delivery_state",
  ].join("\n"),
  ["英文图谱状态词"],
);

assertLeaks(
  "mechanical pair amount wording leak sample",
  [
    "2025-08-27 16:05–16:20 示例付款人账户组向示例收款人转账。",
    "该金额属于本轮命中的同事实去重后的金额和高可信追踪种子。",
    "已有规范明细/分析图支持；证据状态：支持。",
  ].join("\n"),
  ["裸账户组表述", "机械命中表述", "机械同事实表述"],
);

assertLeaks(
  "mechanical pair amount detail-table fallback leak sample",
  [
    "金额核验表：原始命中记录 10 笔，原命中明细 8 笔，原始匹配明细 8 笔。",
    "重点交易表：当前已支持该组交易集中发生于 2025-08-27 16:05—16:20，",
    "但本轮未展开逐笔明细表，另有字段未回传。",
  ].join("\n"),
  ["机械明细表述"],
);

assertLeaks(
  "real ui subject internal-return wording leak sample",
  [
    "当前摘要未回传的开户资料、联系方式、地址、用途凭证均属于下一步需补证事项。",
  ].join("\n"),
  ["机械明细表述", "内部摘要表述"],
);

assertLeaks(
  "real ui pair amount current-fact-summary wording leak sample",
  [
    "当前事实摘要已明确有效金额与差异金额，但未展开16笔有效转账的逐笔时间点、付款账号和收款账号。",
  ].join("\n"),
  ["机械明细表述", "内部摘要表述"],
);

assertLeaks(
  "real ui pair amount unexpanded-account wording leak sample",
  ["现有已调取流水摘要未逐笔展开具体账号、单笔时间和摘要字段。"].join("\n"),
  ["机械明细表述"],
);

assertLeaks(
  "real ui subject round-material wording leak sample",
  [
    "本轮材料已明确展示的账号包括若干项，但本轮材料未展开完整账号，本轮不宜直接认定主要收付款对象。",
    "补齐本轮未展开的登记账户信息。",
  ].join("\n"),
  ["轮次材料表述"],
);

assertLeaks(
  "real ui downstream internal-summary wording leak sample",
  ["本轮已核到：发生出账账户14张。当前摘要未逐笔列明全部对手账号。"].join("\n"),
  ["内部摘要表述"],
);

assertLeaks(
  "real ui nonstandard progress leak sample",
  "正在继续核验主体对手方事实。",
  ["非标准进度句"],
);

assertLeaks(
  "real ui pair amount shallow-answer leak sample",
  [
    "当前已支持该组交易集中发生于 2025-08-27 16:05—16:20，最大单笔 500 万元；但本轮未展开逐笔明细表。",
    "完整名下账户数仍需以开户资料、主体账户清单补证。",
    "若需制作附件，应导出该 5 笔的交易时间、交易号、付款账号、收款账号、金额、摘要/备注、余额连续性。",
    "下一步取证：调取该 5 笔银行回单、双方完整流水、交易流水号、余额连续性。",
  ].join("\n"),
  ["机械明细表述", "机械账户补证套话", "机械附件制作套话", "重复调取流水建议"],
);

assertLeaks(
  "real ui pair amount full-scope mechanical leak sample",
  [
    "当前流水可见合成主体甲向合成主体乙相关收款端转账 25 笔，合计 5900 万元。",
    "付款方：合成主体甲主体账户集合。",
    "下一步取证：调取合成主体甲相关付款账户 2025-08-27 全量流水。",
  ].join("\n"),
  ["机械收款端表述", "重复调取流水建议"],
);

assertLeaks(
  "real ui pair amount vague-scope leak sample",
  [
    "全区间、多账户显示示例付款人与示例收款人存在完整往来。",
    "重点交易表：示例付款人多账户向示例收款人多账户转账 18 笔。",
    "异常特征：金额大、时间集中。下一步取证：继续调取收款账户后续出账明细。",
  ].join("\n"),
  ["笼统范围表述", "已见后续出账仍泛化取证"],
);

assertLeaks(
  "real ui pair amount full-period aggregate label leak sample",
  [
    "示例付款人向示例收款人的全期间转账为 16 笔。",
    "金额核验表：全期间汇总金额为若干元，其中集中转账最突出。",
    "重点交易表：全期间往来汇总。",
  ].join("\n"),
  ["笼统范围表述"],
);

assertLeaks(
  "real ui pair amount folded-row leak sample",
  ["| 其余 8 笔 | 多个账户 | 多个账户 | 合计 364,013 | 小额转账 |"].join("\n"),
  ["折叠明细行表述"],
);

assertLeaks(
  "real ui graph folded-link leak sample",
  ["**其余链路：** 合成主体乙名下其余数笔大额出账需复核。"].join("\n"),
  ["折叠明细行表述"],
);

assertLeaks(
  "real ui pair amount residual-bucket leak sample",
  [
    "全期间扣除该集中链路后 13 笔合计 4,885,013.00 元，属零散历史往来，需结合用途材料复核。",
    "| 2017-03-09 | 9000000000000000010 | 9000000000000000007 | 651,000.00 | 跨行转出 | 与2017年大额入账相邻，需结合用途材料复核 |",
    "下一步：导出核对该 18 笔明细。",
  ].join("\n"),
  ["残差桶表述", "完整小表未展开"],
);

assertLeaks(
  "real ui report evidence-status heading leak sample",
  [
    "| 合成主体乙后续出账对手方 | 金额 | 时间 | 与合成主体甲款项关系 | 证据状态 |",
    "| 其他代理业务资金-浙银理财认申购户 | 20,000,000.00元 | 2025-08-29 | 时间接近、金额高度对应 | 理财申购承接线索，需补证 |",
  ].join("\n"),
  ["机械同事实表述"],
);

assertLeaks(
  "real ui pair amount already-visible continuation as missing leak sample",
  [
    "已见合成主体乙账户存在理财申购和第三方转出线索。",
    "下一步取证：建议重点核对合成主体乙收款账户后续转出明细、双方借据和相关银行回单。",
  ].join("\n"),
  ["已见后续出账仍泛化取证"],
);

const pairAmountMechanicalTranslation = translateUserVisibleText(
  [
    "示例付款人账户组 → 示例收款人账户组",
    "本轮命中的同事实去重代表，高可信追踪种子，已有规范明细/分析图支持，证据状态：支持",
    "原始命中记录；本轮未展开逐笔明细表；调取双方完整流水；收款端；主体账户集合；调取示例主体全量流水",
  ].join("\n"),
);
assertClean(
  "mechanical pair amount wording translation",
  pairAmountMechanicalTranslation,
  [
    "相关账户（需列明账号）",
    "已清洗流水显示",
    "重复风险复核",
    "可作为后续追查起点",
    "已有流水支持",
    "核验意见",
    "初筛流水记录",
    "重点交易明细需逐笔列明",
    "在已调取流水中核对缺失字段",
  ],
);

const shallowPairAnswerTranslation = translateUserVisibleText(
  [
    "完整名下账户数仍需以开户资料、主体账户清单补证。",
    "全区间显示双方存在多账户往来。",
    "全期间扣除该集中链路后属零散历史往来，需结合用途材料复核。",
    "导出核对该 18 笔明细。",
    "若需制作附件，应导出该 5 笔明细。",
    "调取双方完整流水。",
    "建议重点核对收款账户后续出账明细。",
  ].join("\n"),
);
assertClean(
  "shallow pair answer wording translation",
  shallowPairAnswerTranslation,
  [
    "本次转账涉及账户已在上表列明",
    "统计期间内",
    "集中交易之外的逐笔往来",
    "摘要/类型提示用途线索",
    "已返回的小表应先逐笔列明",
    "在已调取流水中导出",
    "在已调取流水中核对缺失字段",
    "先用当前案件已导入的收款方流水核验承接/分流",
  ],
);

const subjectInternalReturnTranslation = translateUserVisibleText(
  "当前摘要未回传的开户资料、联系方式和用途凭证均属于下一步需补证事项。",
);
assertClean(
  "subject internal-return wording translation",
  subjectInternalReturnTranslation,
  ["当前材料尚未取得"],
);

const currentFactSummaryTranslation = translateUserVisibleText(
  "当前事实摘要已明确有效金额与差异金额，但未展开16笔有效转账的逐笔时间点。",
);
assertClean(
  "current fact-summary wording translation",
  currentFactSummaryTranslation,
  ["当前材料", "相关16笔有效转账的逐笔时间点需在附件中逐笔列明"],
);
assert(
  !currentFactSummaryTranslation.includes("当前事实摘要"),
  `current fact summary survived translation\n${currentFactSummaryTranslation}`,
);
assert(
  !currentFactSummaryTranslation.includes("未展开"),
  `unexpanded wording survived translation\n${currentFactSummaryTranslation}`,
);

const unexpandedAccountTranslation = translateUserVisibleText(
  "现有已调取流水摘要未逐笔展开具体账号、单笔时间和摘要字段。",
);
assertClean(
  "unexpanded account wording translation",
  unexpandedAccountTranslation,
  ["需结合回单、流水号和账户明细逐笔固定"],
);
assert(
  !unexpandedAccountTranslation.includes("未逐笔展开"),
  `unexpanded account wording survived translation\n${unexpandedAccountTranslation}`,
);

const subjectRoundMaterialTranslation = translateUserVisibleText(
  "本轮材料已明确展示账号，但本轮材料未展开完整账号，本轮不宜认定，需补齐本轮未展开的登记账户信息。",
);
assertClean(
  "subject round-material wording translation",
  subjectRoundMaterialTranslation,
  ["现有材料已列明", "需结合开户资料逐号补齐登记账户信息", "现阶段不宜"],
);

const downstreamInternalSummaryTranslation = translateUserVisibleText(
  "本轮已核到出账账户14张，当前摘要未逐笔列明全部对手账号。",
);
assertClean(
  "downstream internal-summary wording translation",
  downstreamInternalSummaryTranslation,
  ["现有材料已核实", "当前材料尚未固定逐笔明细"],
);

const graphFoldedLinkTranslation = translateUserVisibleText(
  "其余链路：合成主体乙名下其余数笔大额出账需复核。",
);
assertClean(
  "graph folded-link wording translation",
  graphFoldedLinkTranslation,
  ["尚未固定的后续去向", "尚未固定的数笔"],
);

const visualArtifactTranslation = translateUserVisibleText(
  [
    "图例：supported edge / needs_review / candidate",
    "edge_status=supported；delivery_state=supported；case_id=abc",
  ].join("\n"),
);
assertClean(
  "visual artifact state-word translation",
  visualArtifactTranslation,
  ["已有数据支持", "需补证", "线索", "链路状态", "交付状态", "案件编号"],
);

const progressTranslation = translateUserVisibleText(
  "我会按“清洗明细导出/证据表交付”口径先读取对应技能说明。",
);
assertClean("desktop progress scope wording translation", progressTranslation, [
  "正在核验当前案件事实。",
]);
assert(
  !progressTranslation.includes("口径"),
  `progress scope wording survived translation\n${progressTranslation}`,
);

const firstProgressTranslation = translateUserVisibleText(
  "我会按当前案件资金研判流程先读取案件数据口径与可用交易明细，再做两方金额核验并整理成经侦材料式结论。",
);
assertClean(
  "desktop first progress plan translation",
  firstProgressTranslation,
  ["正在核验当前案件事实。"],
);
assert(
  !firstProgressTranslation.includes("我会"),
  `first progress plan wording survived translation\n${firstProgressTranslation}`,
);

assertLeaks(
  "mechanical delivery heading sample",
  [
    "按当前案件、未限定时间和账户口径核验。",
    "全期间同名对手户名聚合口径",
    "核心收款账号确定口径",
    "口径与来源边界",
    "数据事实",
    "已有数据支持的资金边（资金流向图依据表）",
    "没有已有数据支持的交易级资金边时停止画图",
    "资金流向图（仅使用上表已有数据支持的资金边）",
    "Subject Dossier",
    "归属边界（direct / candidate）",
    "候选线索账户 0 个",
    "必须保留",
    "不要合并或改名",
    "报告复核作答骨架: 第一行保留",
  ].join("\n"),
  [
    "机械开头",
    "内部金额统计标题",
    "内部图谱标题",
    "审计层资金边表述",
    "资金边",
    "Subject Dossier",
    "direct/candidate",
    "候选账户零值标题",
    "指令型标题合同",
    "指令型报告骨架",
  ],
);

assertLeaks(
  "real ui report instruction-contract phrase leak sample",
  [
    "46,885,013.00 元与 25,885,013.00 元的差异是本报告必须保留的金额争议点。",
    "原始明细总额和有效去重金额不要合并或改名。",
  ].join("\n"),
  ["指令型标题合同"],
);

const reportInstructionContractTranslation = translateUserVisibleText(
  [
    "46,885,013.00 元与 25,885,013.00 元的差异是本报告必须保留的金额争议点。",
    "原始明细总额和有效去重金额不要合并或改名。",
  ].join("\n"),
);
assertClean(
  "real ui report instruction-contract phrase translation",
  reportInstructionContractTranslation,
  ["需在材料中列明", "不得混同"],
);

const claimReviewText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    card_type: "claim_review_card",
    required_facts_present: false,
    verified_claims: [{ claim: "资金流入流出合计已按当前案件核验。" }],
    corrected_claims: [{ claim: "原报告金额需按核验范围纠正。" }],
    unsupported_claims: [{ claim: "股票明细当前不能确认。" }],
    unsupported_flows: [{ text: "Mermaid path has no supported edge." }],
    forbidden_phrasings: ["claim pass / blocked / needs_review / candidate"],
    next_review_actions: ["follow-up source envelope and evidence ledger"],
  },
});
assertSupportEnvelope("claim review card", claimReviewText, [
  "support_facts.card_type",
  "support_facts.verified_claims",
  "support_facts.corrected_claims",
]);

const pairAmountText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    card_type: "pair_amount_review_card",
    delivery_state: "supported",
    validation_state: "bounded_workbench_executed",
    holder_name: "示例主体甲",
    via_holder_name: "示例对手乙",
    raw_detail_amount: 1200,
    raw_detail_count: 3,
    high_confidence_same_fact_effective_amount: 1000,
    high_confidence_same_fact_count: 2,
    duplicate_or_unsupported_amount: 200,
    answer_draft: [
      "Pair Amount source boundary validation state raw/effective/dedup",
      "rank_counterparties only ranking_only/candidate and cannot be final claim",
      "Workbench JSON debug output is blocked",
    ],
  },
});
assertSupportEnvelope("pair amount review card", pairAmountText, [
  "support_facts.card_type",
  "support_facts.raw_detail_amount",
  "support_facts.high_confidence_same_fact_effective_amount",
  "support_facts.duplicate_or_unsupported_amount",
]);

const downstreamTraceText = compiler.compactToolText({
  tool: "trace_subject_top_outflows",
  status: "ok",
  response: {
    skill_id: "trace_subject_top_outflows",
    status: "ok",
    data: {
      scope_stats: {
        account_open_name: "示例收款人",
        txn_count: 62,
        out_amount: 51018573.8,
        first_txn_at: "2025-08-29 01:08:36",
        last_txn_at: "2025-09-24 11:40:37",
      },
      top_outflows: [
        {
          rank: 1,
          seed_txn: {
            txn_id: "t1",
            txn_time: "2025-09-10 10:28:22",
            account_key: "9000000000000000013",
            account_open_name: "示例收款人",
            counterparty_key: "9000000000000000014",
            counterparty_name: "示例下游人",
            amount: { yuan: 5000000 },
            direction: "out",
            txn_type: "e转出",
          },
          terminal_category: "target_account_no_matched_downstream",
          terminal_category_label:
            "对手账号在案内存在，但本次核验窗口内未匹配到后续出账",
          target_account_in_case: true,
        },
      ],
      terminal_summary: ["本次核验窗口内未匹配到后续出账"],
      followup_requests: ["案内账户存在，但本窗口未见继续出账"],
    },
  },
});
const downstreamTraceSupport = assertSupportEnvelope(
  "downstream trace support",
  downstreamTraceText,
  ["compact_evidence.top_outflows"],
);
const downstreamTraceRendered = downstreamTraceSupport;
assert(
  !/本窗口|核验窗口|本次核验窗口/u.test(downstreamTraceRendered),
  `downstream trace support leaked window wording\n${downstreamTraceRendered}`,
);
assert(
  downstreamTraceRendered.includes("已调取流水范围内未匹配到接续出账"),
  `downstream trace support must use case-facing stop wording\n${downstreamTraceRendered}`,
);

const sourceToCounterpartyAmountText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    card_type: "source_to_counterparty_amount_card",
    holder_name: "示例付款人",
    via_holder_name: "示例收款人",
    source_to_via_summary: {
      txn_count: 18,
      amount: 25885013,
      source_accounts: ["9000000000000000008"],
      largest_date_cluster: {
        date: "2025-08-27",
        txn_count: 5,
        amount: 21000000,
      },
    },
    source_to_via_date_window_summary: {},
    source_to_via_core_account_summary: {
      txn_count: 5,
      amount: 21000000,
      source_accounts: ["9000000000000000008"],
      counterparty_accounts: ["9000000000000000013"],
      duplicate_source_accounts: ["9000000000000000017"],
      largest_date_cluster: {
        date: "2025-08-27",
        txn_count: 5,
        amount: 21000000,
      },
    },
    answer_draft: [
      "核验依据：示例付款人 -> 示例收款人 全期间同名对手户名聚合：按有效交易号去重 16 笔。",
      "核心窗口口径未稳定返回。",
      "按核心收款账号核验：旧诊断行。",
    ],
  },
});
assertSupportEnvelope(
  "source-to-counterparty amount card",
  sourceToCounterpartyAmountText,
  [
    "support_facts.card_type",
    "support_facts.source_to_via_summary",
    "support_facts.source_to_via_core_account_summary",
  ],
);
for (const forbidden of [
  "核验依据",
  "全期间同名对手户名聚合",
  "核心窗口口径",
  "按核心收款账号核验",
  "金额口径",
  "口径差异说明",
  "同名收款人聚合支持",
  "核心收款账号支持",
  "核心收款账号核验",
  "重点收款账户核验",
  "核心收款账号",
]) {
  assert(
    !sourceToCounterpartyAmountText.includes(forbidden),
    `source-to-counterparty amount card exposes mechanical wording: ${forbidden}\n${sourceToCounterpartyAmountText}`,
  );
}

const flowGraphText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    title: "build_fund_flow_graph completed",
    holder_name: "示例付款人",
    via_holder_name: "示例收款人",
    summary: "supported seed and candidate path",
  },
  key_facts: {
    graph_delivery_contract: {
      output_order: [
        "结论",
        "资金来源",
        "主要资金链路",
        "下游去向",
        "资金断点",
        "待补证事项",
      ],
      final_answer_sections: [
        "结论",
        "资金来源",
        "主要资金链路",
        "下游去向",
        "资金断点",
        "待补证事项",
      ],
      edge_boundary:
        "图中只画端点完整且金额、时间、付款端、收款端均可核验的交易",
    },
    flow_graph: {
      supported_edge_count: 1,
      incomplete_edge_count: 3,
      edge_status_counts: {
        supported: 1,
        candidate: 1,
        needs_review: 1,
        missing: 1,
      },
      edges: [
        {
          edge_status: "supported",
          from_label: "示例付款人",
          to_label: "示例收款人",
          amount: 21000000,
          txn_time: "2025-01-01",
          summary: "转账",
        },
        {
          edge_status: "candidate",
          from_label: "示例收款人",
          to_label: "未调取端点",
          amount: 5000000,
          missing_fields: ["counterparty_account"],
          boundary: "candidate needs_review",
        },
      ],
    },
  },
});
assertSupportEnvelope("flow graph card", flowGraphText, [
  "compact_evidence.flow_graph",
]);

const nameQuickFactText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    card_type: "name_association_quick_fact_card",
    title: "姓名/主体关联快查",
    strict_rows: [
      { target: "示例甲", strict_txn_count: 2, strict_amount: 300 },
    ],
    excluded_near_names: [
      { target: "示例甲", matched_name: "示例乙", txn_count: 1, amount: 100 },
    ],
    cannot_confirm: ["示例丙"],
    answer_draft: [
      "姓名/主体关联快查",
      "严格命中:",
      "- 示例甲：严格命中 2 笔，300.00 元。",
      "模糊命中:",
      "- 本轮未发现可直接并入目标主体的模糊命中。",
      "排除对象:",
      "- 示例乙：近名排除，不能并入目标。",
      "不能确认:",
      "- 示例丙：严格字段未命中，不能确认其与当前案件资金有关联。",
      "证据边界：轻量快查，不扩大为全案深挖。",
    ],
  },
});
assertSupportEnvelope("name association quick fact card", nameQuickFactText, [
  "support_facts.strict_rows",
  "support_facts.excluded_near_names",
]);

const financialTopicText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    card_type: "financial_product_topic_card",
    title: "主体基金/证券/股票专题核算",
    subject_account_count: 2,
    buckets: [
      { bucket: "基金申赎", direction: "出账", txn_count: 14, amount: 7600000 },
      {
        bucket: "证券三方存管",
        direction: "入账",
        txn_count: 27,
        amount: 1517861.88,
      },
    ],
    stock_detail_visible: false,
    answer_draft: [
      "主体基金/证券/股票专题核算",
      "主体账户全集：按户名及同证件号账户核验，共 2 个账户。",
      "基金申赎：出账 14 笔，7,600,000.00 元；进账 4 笔，4,090,623.63 元。",
      "证券三方存管：出账 25 笔，10,625,008.00 元；进账 27 笔，1,517,861.88 元。",
      "股票明细：当前可见数据未提供股票成交、持仓或证券流水明细；需补调证券流水或持仓明细。",
      "证据边界：不得只抓单一证券公司对手方代替主体账户全集。",
    ],
  },
});
assertSupportEnvelope("financial product topic card", financialTopicText, [
  "support_facts.subject_account_count",
  "support_facts.buckets",
  "support_facts.stock_detail_visible",
]);

const companyToPersonText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    card_type: "company_to_person_recompute_card",
    title: "对公转个人资金重算",
    total_txn_count: 10,
    total_amount: 1000,
    summary_missing_txn_count: 3,
    summary_missing_amount: 300,
    nature_buckets: [
      { bucket: "可识别薪资工资/劳务/报销", txn_count: 2, amount: 200 },
      { bucket: "无法识别性质", txn_count: 8, amount: 800 },
    ],
    answer_draft: [
      "对公转个人资金重算",
      "口径定义：对公主体指组织特征付款账户；个人主体指个人端。",
      "摘要缺失定义：交易摘要字段为空或仅空白；该部分纳入总额。",
      "总额：10 笔，1,000.00 元。",
      "可识别薪资工资/劳务/报销：2 笔，200.00 元。",
      "无法识别性质：8 笔，800.00 元。",
      "摘要缺失纳入金额：3 笔，300.00 元；已计入上述总额。",
      "证据边界：暂不出具最终确认表述。",
    ],
  },
});
assertSupportEnvelope("company to person recompute card", companyToPersonText, [
  "support_facts.total_txn_count",
  "support_facts.total_amount",
  "support_facts.summary_missing_amount",
  "support_facts.nature_buckets",
]);

const flowGraphDeliveryText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    card_type: "fund_flow_graph_delivery_card",
    title: "资金穿透图",
    supported_edges: [
      {
        from_label: "示例甲",
        to_label: "示例乙",
        edge_status: "supported",
        amount: 5000000,
        txn_time: "2025-01-01",
        boundary: "supported",
      },
    ],
    boundary_edges: [
      {
        from_label: "示例乙",
        to_label: "未调取端点",
        edge_status: "needs_evidence",
        amount: 20000000,
        missing_fields: ["收款侧流水"],
        boundary: "needs evidence",
      },
    ],
    answer_draft: [
      "资金穿透图",
      "主链核验：示例甲 → 示例乙，本轮按可见交易核验 5 笔，21,000,000.00 元。",
      "可画入图的交易边：3 条；仅这些交易边进入下方资金流向图。",
      "资金流向图:",
      "- 示例甲 → 示例乙：5,000,000.00 元。",
      "旁路线索:",
      "- 示例乙 → 理财产品：20,000,000.00 元；需做余额承接后才能写成原资金穿透。",
      "现金/理财/证券/基金去向:",
      "- 理财产品：20,000,000.00 元；列为资产/产品去向线索。",
      "未调取端点/资金断点:",
      "- 未调取端点需补证。",
      "补证建议：补调核心收款账号流水。",
    ],
  },
});
assertSupportEnvelope("fund flow graph delivery card", flowGraphDeliveryText, [
  "support_facts.supported_edges",
  "support_facts.boundary_edges",
]);

const broadGraphTableText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    card_type: "broad_graph_table_card",
    title: "当前案件资金图谱和重点对手方表格",
    answer_draft: [
      "当前案件资金图谱和重点对手方表格",
      "资金图谱节点: 重点账户 9000000000000000012；重点主体 示例甲。",
      "重点对手方表格:",
      "| 1 | __unknown__ | 1,000.00 元 | 2 | 当前案件规范明细索引统计；不是确定性资金穿透边 |",
      "证据边界: 本表是全案统计图谱和重点对手方排行，不等同于逐笔资金穿透。",
      "补证建议: 对重点对手方补调交易级流水、银行回单、开户资料、用途材料和余额承接。",
    ],
  },
});
assertSupportEnvelope("broad graph table card", broadGraphTableText, [
  "support_facts.card_type",
]);

const accountAnalysisText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    intent: "account_analysis",
    title: "Analytix 资金研判入口",
  },
  key_facts: {
    account_key: "9000000000000000012",
    account_stats: {
      txn_count: 44,
      inflow: 54196609.36,
      outflow: 51058000,
      turnover: 105254609.36,
    },
    counterparty_rankings: [
      { display_name: "示例对手方", turnover: 1000000, txn_count: 3 },
    ],
  },
});
assertSupportEnvelope("account analysis card", accountAnalysisText, [
  "compact_evidence.account_key",
  "compact_evidence.account_stats",
  "compact_evidence.counterparty_rankings",
]);

const genericCardText = compiler.agentReadableToolText({
  status: "ok",
  answer_card: {
    title: "rank_accounts completed",
    case_id: "case-demo",
    summary:
      "已返回本工具可支持的事实摘要；targeted source boundary validation state",
  },
  key_facts: {
    summary: "普通排行卡",
  },
});
assertSupportEnvelope("generic answer card", genericCardText);

const dbDiagnosticSupportText = compiler.compactToolText({
  data: {
    skill_id: "case_sql_recipes",
    status: "ok",
    data: {
      recipe_contract: "case_sql_recipes_v1",
      recipes: [
        {
          recipe_id: "pair_amount_one_hop",
          title: "两主体一跳金额核算",
          category: "pair_amount",
          required_parameters: ["payer_name", "receiver_name"],
          template_sql:
            "SELECT * FROM fc_transaction_norm WHERE account_open_name = :payer_name",
          source_scope: [
            "current_case",
            "cleaned fc_*_norm",
            "approved analysis_*",
          ],
          replayable: true,
        },
      ],
      items: [
        {
          tool_name: "run_case_sql",
          query_digest: "abcd1234",
          purpose: "audited aggregate",
          base_tables: ["fc_transaction_norm"],
          raw_sql: "SELECT account_open_name FROM fc_transaction_norm",
          local_path: "/Users/sun/private/case.duckdb",
        },
      ],
      preview_contract: "case_row_preview_v1",
      sample_notice:
        "样本仅用于字段和记录形态核验，不得据此汇总金额、笔数或资金链路。",
      records: [
        {
          account_open_name: "张三",
          counterparty_acct_norm: "9000000000000011",
          amount_val: 100,
        },
      ],
      row_count: 1,
      row_limit: 20,
      raw_rows_exposed: false,
      query_id: "query-demo",
      evidence_id: "evidence-demo",
    },
    citations: {
      query_ids: ["query-demo"],
      evidence_ids: ["evidence-demo"],
    },
  },
});
assertSupportEnvelope(
  "database-site diagnostic support",
  dbDiagnosticSupportText,
  ["compact_evidence.workbench"],
);
for (const forbidden of [
  "template_sql",
  "SELECT *",
  "SELECT account_open_name",
  "/Users/",
  "9000000000000011",
  "raw_sql",
  "local_path",
]) {
  assert(
    !dbDiagnosticSupportText.includes(forbidden),
    `database-site diagnostic support leaked ${forbidden}\n${dbDiagnosticSupportText}`,
  );
}
assert(
  dbDiagnosticSupportText.includes("样本仅用于字段和记录形态核验"),
  `database-site diagnostic support lost sample boundary\n${dbDiagnosticSupportText}`,
);

const workbenchInternalPayloadText = compiler.compactToolText({
  tool: "profile_case_schema",
  status: "ok",
  response: {
    skill_id: "profile_case_schema",
    status: "ok",
    data: {
      profile_contract: "case_schema_profile_v1",
      workbench_contract: "local_duckdb_readonly",
      execution_status: "executed",
      schema_status: "ok",
      allowed_view_policy: "cleaned_and_analysis_only",
      result_mode: "aggregate",
      row_limit: 20,
      row_count: 1,
      raw_rows_exposed: false,
      source_hash: "sha256:internal",
      query_id: "query-internal",
      metric_scope: {
        purpose: "audited aggregate",
        result_mode: "aggregate",
        row_count: 1,
        truncated: false,
      },
      sql_policy: {
        policy_engine: "DuckDB parser/binder",
        status: "passed",
      },
      validation_state: {
        current_case_only: true,
        readonly: true,
        cleaned_analysis_scope_only: true,
        raw_rows_exposed: false,
      },
      tables: [
        {
          table_name: "analysis_txn_detail_idx",
          sql_name: "analysis_txn_detail_idx",
          role: "approved analysis table",
          row_count: 12,
          columns: [
            {
              column_name: "counterparty_name",
              sql_name: "counterparty_name",
              data_type: "VARCHAR",
            },
            {
              column_name: "counterparty_acct_norm",
              sql_name: "counterparty_acct_norm",
              data_type: "VARCHAR",
            },
            {
              column_name: "amount_val",
              sql_name: "amount_val",
              data_type: "DOUBLE",
            },
          ],
          field_coverage: {
            counterparty_name: {
              column: "counterparty_name",
              coverage_rate: 0.8,
              distinct_count: 3,
            },
          },
        },
      ],
      items: [
        {
          tool_name: "run_case_sql",
          query_digest: "abcd1234",
          purpose: "DuckDB audited aggregate",
          base_tables: ["analysis_txn_detail_idx"],
          validation_state: "parser_binder_validated",
        },
      ],
    },
    citations: {
      query_ids: ["query-internal"],
    },
  },
});
assertSupportEnvelope(
  "workbench internal payload support",
  workbenchInternalPayloadText,
  ["compact_evidence.workbench"],
);
for (const forbidden of [
  "DuckDB",
  "analysis_txn_detail_idx",
  "fc_transaction_norm",
  "source_hash",
  "query_id",
  "query_ids",
  "metric_scope",
  "raw_rows_exposed",
  "current_case_only",
  "cleaned_analysis_scope_only",
  "parser_binder_validated",
  "row_limit",
  "counterparty_name",
  "counterparty_acct_norm",
  "run_case_sql",
]) {
  assert(
    !workbenchInternalPayloadText.includes(forbidden),
    `workbench internal payload leaked ${forbidden}\n${workbenchInternalPayloadText}`,
  );
}
for (const required of ["清洗后的交易明细", "最终答复改写约束"]) {
  assert(
    workbenchInternalPayloadText.includes(required),
    `workbench internal payload missing safe marker ${required}\n${workbenchInternalPayloadText}`,
  );
}

const reportDeliveryPayload = {
  tool: "run_full_case_analysis",
  response: {
    data: {
      skill_id: "run_full_case_analysis",
      status: "ok",
      data: {
        workbench_contract: "local_duckdb_readonly",
        execution_status: "report_written",
        result_mode: "report_artifact",
        case_id: "case-demo",
        report_path: "/Users/sun/Projects/analytix/output/report-demo.md",
        manifest_path: "/Users/sun/Projects/analytix/output/report-demo.json",
        report_artifact: {
          path: "/Users/sun/Projects/analytix/output/report-demo.md",
          manifest_path: "/Users/sun/Projects/analytix/output/report-demo.json",
          inspection_status: "passed",
          files: [
            {
              path: "/Users/sun/Projects/analytix/output/report-demo.md",
              size: 1200,
              sha256: "0123456789abcdef0123456789abcdef",
              inspection_status: "passed",
            },
            {
              path: "/Users/sun/Projects/analytix/output/report-demo.json",
              size: 300,
              sha256: "abcdef0123456789abcdef0123456789",
              inspection_status: "passed",
            },
          ],
        },
        full_case_package: {
          coverage: {
            transaction_detail: {
              row_count: 12,
              txn_count: 12,
              account_count: 2,
              counterparty_name_count: 3,
              turnover_amount: 100,
              inflow_amount: 40,
              outflow_amount: 60,
              first_txn_at: "2024-01-01",
              last_txn_at: "2024-01-31",
            },
          },
          holder_rankings: [
            {
              rank: 1,
              name: "测试主体",
              amount_yuan: "100.00 元",
              txn_count: "12",
              first_txn_at: "2024-01-01",
              last_txn_at: "2024-01-31",
            },
          ],
        },
        validation_state: {
          status: "passed",
          current_case_only: true,
          readonly: true,
          cleaned_analysis_scope_only: true,
          artifact_written: true,
        },
      },
    },
  },
};
const reportDeliveryText = compiler.compactToolText(reportDeliveryPayload, {
  write_report: true,
});
assert(
  reportDeliveryText.includes("正式报告未发布"),
  `ReportRequiresPublicationReceipt: compiler blocker is missing\n${reportDeliveryText}`,
);
assert(
  !reportDeliveryText.includes("报告交付"),
  `ReportRequiresPublicationReceipt: compiler accepted an unverified delivery\n${reportDeliveryText}`,
);
assert(
  !reportDeliveryText.includes("检查状态 passed"),
  `ReportRequiresPublicationReceipt: file inspection was treated as publication proof\n${reportDeliveryText}`,
);
assert(
  !reportDeliveryText.includes("report-demo"),
  `ReportRequiresPublicationReceipt: compiler leaked an unverified report path\n${reportDeliveryText}`,
);

let reportWriteExecutionCount = 0;
const reportWriteRuntime = createToolCallRuntime({
  executeSkill: async () => {
    reportWriteExecutionCount += 1;
    return { status: "ok" };
  },
});
const reportWriteBlocked = await reportWriteRuntime.callTool(
  "run_full_case_analysis",
  {
    case_id: "case-demo",
    write_report: true,
    publication_receipt_id: "model-supplied-receipt",
  },
);
assert(
  reportWriteExecutionCount === 0,
  `ReportRequiresPublicationReceipt: write_report=true reached executeSkill ${reportWriteExecutionCount} time(s)`,
);
assert(
  reportWriteBlocked.error_code === "PUBLICATION_RECEIPT_REQUIRED",
  `ReportRequiresPublicationReceipt: wrong blocker\n${JSON.stringify(reportWriteBlocked, null, 2)}`,
);
assert(
  reportWriteBlocked.isError === true,
  `ReportRequiresPublicationReceipt: tool payload must be semantic failure\n${JSON.stringify(reportWriteBlocked, null, 2)}`,
);
const reportReadOnlyBlocked = await reportWriteRuntime.callTool(
  "run_full_case_analysis",
  {
    case_id: "case-demo",
    write_report: false,
  },
);
assert(
  reportWriteExecutionCount === 0,
  `WriteReportFalseProducesNoFile: P0-quarantined analysis reached executeSkill ${reportWriteExecutionCount} time(s)`,
);
assert(
  reportReadOnlyBlocked.error_code === "PUBLICATION_RECEIPT_REQUIRED",
  `WriteReportFalseProducesNoFile: analysis-only call must fail closed during P0\n${JSON.stringify(reportReadOnlyBlocked, null, 2)}`,
);
assert(
  reportReadOnlyBlocked.isError === true,
  `WriteReportFalseProducesNoFile: P0 blocker must be a semantic failure\n${JSON.stringify(reportReadOnlyBlocked, null, 2)}`,
);
assert(
  supportsLocalDuckdbWorkbenchSkill("run_full_case_analysis") === false,
  "ReportRequiresPublicationReceipt: local DuckDB runtime still advertises report fallback",
);
const reportBackendFailureRuntime = createToolCallRuntime({
  executeSkill: async () => {
    throw new Error("backend report service unavailable");
  },
});
const reportBackendFailure = await reportBackendFailureRuntime.callTool(
  "run_full_case_analysis",
  {
    case_id: "case-demo",
    write_report: false,
  },
);
const reportBackendFailureSerialized = JSON.stringify(reportBackendFailure);
assert(
  reportBackendFailure.error_code === "PUBLICATION_RECEIPT_REQUIRED",
  `ReportRequiresPublicationReceipt: P0 quarantine must block before the unavailable backend\n${reportBackendFailureSerialized}`,
);
assert(
  reportBackendFailure.isError === true,
  `ReportRequiresPublicationReceipt: P0 quarantine must remain semantic failure\n${reportBackendFailureSerialized}`,
);
assert(
  !/report[_A-Z]?path|manifest[_A-Z]?path|local_duckdb_fallback|已生成/u.test(
    reportBackendFailureSerialized,
  ),
  `ReportRequiresPublicationReceipt: backend failure entered report fallback\n${reportBackendFailureSerialized}`,
);
const { toolResult: reportDeliveryToolResult } = createMcpToolResultRuntime({
  serverName: "analytix-funds-smoke",
  serverVersion: "0.16.9",
  compactToolText: compiler.compactToolText,
  compactStructuredContent: compiler.compactStructuredContent,
  env: {},
});
const reportDeliveryResult = reportDeliveryToolResult(reportDeliveryPayload, {
  write_report: true,
});
const reportDeliverySerialized = JSON.stringify(reportDeliveryResult);
assert(
  !reportDeliverySerialized.includes("report-demo.md"),
  `ReportRequiresPublicationReceipt: unverified report path escaped into MCP result\n${JSON.stringify(reportDeliveryResult, null, 2)}`,
);
assert(
  !reportDeliverySerialized.includes("report-demo.json"),
  `ReportRequiresPublicationReceipt: unverified manifest path escaped into MCP result\n${JSON.stringify(reportDeliveryResult, null, 2)}`,
);
assert(
  !reportDeliveryResult.structuredContent?.delivery_artifact,
  `ReportRequiresPublicationReceipt: unverified delivery_artifact was accepted\n${JSON.stringify(reportDeliveryResult, null, 2)}`,
);
assert(
  reportDeliveryResult.isError === true,
  `ReportRequiresPublicationReceipt: MCP top-level isError must be true\n${JSON.stringify(reportDeliveryResult, null, 2)}`,
);
assert(
  String(reportDeliveryResult.content?.[0]?.text || "").includes("正式报告"),
  `ReportRequiresPublicationReceipt: blocker text is missing\n${JSON.stringify(reportDeliveryResult, null, 2)}`,
);
for (const forbidden of ["0.00 元", "事实材料已覆盖全案", "支撑全案初步研判"]) {
  assert(
    !reportDeliverySerialized.includes(forbidden),
    `ReportRequiresPublicationReceipt: blocker was laundered into a fact package (${forbidden})\n${JSON.stringify(reportDeliveryResult, null, 2)}`,
  );
}

const { toolResult: adversarialReportToolResult } = createMcpToolResultRuntime({
  serverName: "analytix-funds-smoke",
  serverVersion: "0.16.9",
  compactToolText: compiler.compactToolText,
  compactStructuredContent: compiler.compactStructuredContent,
  env: {
    ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD: "true",
  },
});
for (const [label, injected] of [
  [
    "camel paths",
    {
      reportPath: "/tmp/fake-report.md",
      manifestPath: "/tmp/fake-manifest.json",
    },
  ],
  ["string receipt", { publicationReceipt: "fake-publication-receipt" }],
  [
    "generic artifact",
    { artifact: { path: "/tmp/fake-artifact.md", inspectionStatus: "passed" } },
  ],
  ["camel artifact", { reportArtifact: { path: "/tmp/fake-camel-report.md" } }],
  [
    "delivery artifact",
    { deliveryArtifact: { path: "/tmp/fake-delivery.md" } },
  ],
]) {
  const adversarialResult = adversarialReportToolResult(
    {
      tool: "run_full_case_analysis",
      skill_id: "run_full_case_analysis",
      status: "ok",
      response: injected,
    },
    { write_report: false, include_debug: true },
  );
  const serialized = JSON.stringify(adversarialResult);
  assert(
    adversarialResult.isError === true,
    `ReportRequiresPublicationReceipt (${label}): debug mode must fail closed\n${serialized}`,
  );
  assert(
    !serialized.includes("/tmp/fake"),
    `ReportRequiresPublicationReceipt (${label}): debug mode leaked a path\n${serialized}`,
  );
  assert(
    !serialized.includes("fake-publication-receipt"),
    `ReportRequiresPublicationReceipt (${label}): debug mode leaked a fake receipt\n${serialized}`,
  );
}

const writeReportFalseTemp = fs.mkdtempSync(
  path.join(os.tmpdir(), "analytix-write-report-false-"),
);
const writeReportFalseArtifactRoot = path.join(
  writeReportFalseTemp,
  "artifacts",
);
const { toolResult: writeReportFalseToolResult } = createMcpToolResultRuntime({
  serverName: "analytix-funds-smoke",
  serverVersion: "0.16.9",
  compactToolText: compiler.compactToolText,
  compactStructuredContent: compiler.compactStructuredContent,
  env: {
    ANALYTIX_FUNDS_ARTIFACT_DIR: writeReportFalseArtifactRoot,
  },
});
writeReportFalseToolResult(
  {
    tool: "run_full_case_analysis",
    skill_id: "run_full_case_analysis",
    status: "ok",
    response: {
      data: {
        status: "ok",
        full_case_package: {
          evidence_gaps: ["仅返回分析材料，不生成正式报告。"],
        },
      },
    },
  },
  { write_report: false },
);
assert(
  !fs.existsSync(writeReportFalseArtifactRoot),
  `WriteReportFalseProducesNoFile: MCP detail artifact was written under ${writeReportFalseArtifactRoot}`,
);
fs.rmSync(writeReportFalseTemp, { recursive: true, force: true });

const partialFullCaseText = compiler.compactToolText(
  {
    tool: "run_full_case_analysis",
    response: {
      data: {
        skill_id: "run_full_case_analysis",
        status: "partial",
        data: {
          full_case_package: {
            coverage: {
              transaction_detail: {
                txn_count: 2,
                account_count: 1,
                first_txn_at: "2024-01-01",
                last_txn_at: "2024-01-31",
              },
            },
            holder_rankings: [{ rank: 1, name: "仅覆盖主体甲", txn_count: 2 }],
            evidence_gaps: ["仅覆盖一家主体和一个月。"],
          },
        },
      },
    },
  },
  { write_report: false },
);
assert(
  partialFullCaseText === PUBLICATION_RECEIPT_REQUIRED_TEXT,
  `PartialCoverageCannotBecomeWholeCaseConclusion: full-case payload did not remain in PublicationReceipt quarantine\n${partialFullCaseText}`,
);
assert(
  !partialFullCaseText.includes("仅覆盖主体甲"),
  `PartialCoverageCannotBecomeWholeCaseConclusion: quarantined partial fact leaked\n${partialFullCaseText}`,
);
for (const forbidden of ["支撑全案", "已覆盖全案", "0.00 元"]) {
  assert(
    !partialFullCaseText.includes(forbidden),
    `PartialCoverageCannotBecomeWholeCaseConclusion: partial package was upgraded (${forbidden})\n${partialFullCaseText}`,
  );
}

const caseSourceText = compiler.agentReadableToolText({
  status: "blocked",
  answer_card: {
    card_type: "case_source_blocker",
    reason:
      "主体排行核验 failed after transient retry: Explicit Analytix case_id does not exist: missing-case",
    failed_stage: "explicit_case_lookup",
    explicit_case_id: "missing-case",
    source_chain: [
      "Analytix explicit case selection",
      "backend explicit case lookup",
      "analytixagent runtime context",
      "analytix_funds MCP semantic tool",
    ],
    recovery_actions: [
      "Select the exact Analytix case explicitly, then retry the same question with that case_id.",
      "After the source-of-truth is restored, rerun the semantic MCP tool instead of answering from local files or history.",
    ],
    forbidden_actions: ["do not scan JSON or SKILL.md"],
  },
});
assertClean("case source blocker", caseSourceText, [
  "当前案件来源未就绪",
  "显式案件编号",
  "显式案件编号核验",
  "恢复动作",
  "不得连续尝试其他编号",
]);
for (const forbidden of [
  "failed after transient retry",
  "explicit_case_lookup",
  "source-of-truth",
  "semantic MCP tool",
  "backend explicit case lookup",
  "runtime context",
  "Wait for Analytix",
  "SKILL.md",
  "JSON",
]) {
  assert(
    !caseSourceText.includes(forbidden),
    `case source blocker exposes internal wording: ${forbidden}\n${caseSourceText}`,
  );
}

const renderedCardText = renderDiagnosticCardForAgent({
  card_type: "claim_review_card",
  title: "Visual Evidence claim-review debug card",
  required_facts_present: true,
  write_blocked: false,
  answer_draft: [
    "Mermaid supported edge before claim pass",
    "Workbench artifact with raw rows JSON debug",
  ],
  facts: [{ answer_text: "rank_* coverage data_quality facts are supported." }],
  context_compiler: {
    raw_result_policy: "answer-card-only",
    must_write_facts: ["Context must write：claim_2 status=ok targeted fact"],
  },
  unsupported_claims: ["candidate claim without source envelope"],
  unsupported_flows: ["needs_review Mermaid arrow"],
  next_review_actions: ["follow-up with evidence ledger"],
});
assert(
  renderedCardText.includes("案件事实未发布"),
  `diagnostic card renderer must fail closed before the host Final Evidence Gate\n${renderedCardText}`,
);
assert(
  !/rank_\* coverage|claim_2 status|Mermaid supported edge|raw rows JSON/u.test(
    renderedCardText,
  ),
  `diagnostic card renderer leaked a pre-gate fact draft\n${renderedCardText}`,
);

const auditRetentionBoundary = renderDiagnosticCardForAudit({
  card_type: "generic_diagnostic_card",
  context_compiler: { raw_result_policy: "answer-card-only" },
  continuation_paths: [{}],
});
assert(
  /未返回可读摘要/u.test(auditRetentionBoundary)
    && /须以宿主证据登记为准/u.test(auditRetentionBoundary),
  `audit renderer must describe an unverified retention boundary\n${auditRetentionBoundary}`,
);
for (const forbidden of ["明细证据保留", "已留存在审计材料", "完整载荷保留", "full_payload_retained"]) {
  assert(
    !auditRetentionBoundary.includes(forbidden),
    `audit renderer made an unverified retention claim: ${forbidden}\n${auditRetentionBoundary}`,
  );
}

const emptyAuditBoundary = renderDiagnosticCardForAudit({
  card_type: "generic_diagnostic_card",
  title: "空结果边界",
});
assert(
  /本轮未取得可验证事实/u.test(emptyAuditBoundary)
    && /未返回不等于不存在、为零或无异常/u.test(emptyAuditBoundary)
    && /本轮未取得可验证核验结果/u.test(emptyAuditBoundary),
  `empty audit sections must remain unknown instead of becoming none\n${emptyAuditBoundary}`,
);
assert(
  !/(?:^|\n)- 无(?:\n|$)/u.test(emptyAuditBoundary),
  `empty audit sections were rendered as a verified absence\n${emptyAuditBoundary}`,
);

console.log(
  JSON.stringify(
    {
      status: "ok",
      checked: [
        "direct user-facing term translation",
        "plugin and skill discovery descriptions",
        "agent interface discovery descriptions",
        "desktop first-action routing instructions",
        "runtime config remount self-test",
        "frontdoor safeSkill runtime context propagation",
        "full-case runtime context normalization",
        "funds_investigate Chinese fact-verifier description",
        "report-conclusion review card",
        "pre-gate diagnostic fact drafts are replaced by a fixed host boundary",
        "pair amount review card",
        "fund-flow graph card",
        "name-association quick fact card",
        "financial product topic card",
        "company-to-person recompute card",
        "fund-flow graph delivery card",
        "broad graph table card",
        "account analysis card",
        "generic answer card",
        "database-site diagnostic support",
        "workbench internal payload support",
        "ReportRequiresPublicationReceipt",
        "WriteReportFalseProducesNoFile",
        "PartialCoverageCannotBecomeWholeCaseConclusion",
        "case source blocker",
        "diagnostic card renderer",
        "audit artifact retention requires host evidence registry confirmation",
        "empty diagnostic sections remain unknown instead of none",
      ],
    },
    null,
    2,
  ),
);
